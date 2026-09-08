package simulation

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEPATH101PTACMixedFanInventoryRemainsUnassigned(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate original PTAC model")
	}
	path := filepath.Join(filepath.Dir(filename), "testdata", "energy_path_real_models", "models", "25.1", "DOAToPTAC.idf")
	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := parsePurposePlanFixture(t, string(input))
	counts := map[string]int{}
	for _, object := range doc.Objects {
		counts[strings.ToLower(object.Type)]++
	}
	for kind, want := range map[string]int{"zonehvac:packagedterminalairconditioner": 5, "fan:onoff": 5, "fan:variablevolume": 1, "airloophvac": 1} {
		if counts[kind] != want {
			t.Fatalf("original mixed inventory changed: %s=%d, want%d", kind, counts[kind], want)
		}
	}
	report := idf.AnalyzeHVAC(doc)
	index := buildEnergyServicePathIndex(path)
	eligible := energyPathZoneAuxiliaryEligiblePaths("fans", nil, index.auxiliaryPaths)
	owners, pathIDs := map[string]bool{}, []string{}
	for _, path := range eligible {
		if path.AirLoopName != "DOAS" || path.ZoneName == "PLENUM-1" {
			t.Fatalf("fan AirLoop ownership escaped original DOAS recipients: %#v", path)
		}
		owners[path.ZoneName] = true
		pathIDs = append(pathIDs, path.ID)
	}
	if len(owners) != 5 {
		t.Fatalf("the actual DOAS five-Zone path evidence disappeared: %v", owners)
	}
	for i := 1; i <= 5; i++ {
		if !owners[fmt.Sprintf("SPACE%d-1", i)] {
			t.Fatal("missing exact original DOAS recipient")
		}
	}
	local, central := 0, 0
	for _, item := range report.ServiceModel.Components {
		if strings.EqualFold(item.Component.ObjectType, "ZoneHVAC:PackagedTerminalAirConditioner") {
			local++
			fans := 0
			for _, ref := range item.InternalRefs {
				if strings.EqualFold(ref.ObjectType, "Fan:OnOff") {
					fans++
				}
			}
			if fans != 1 || energyPathAuxiliaryComponentResolved("fans", item, eligible) {
				t.Fatalf("a local PTAC fan was lost or relabelled as the central measured AirLoop pool: %#v", item)
			}
		}
		if item.Component.ObjectType == "Fan:VariableVolume" && item.Component.ObjectName == "DOAS Supply Fan" {
			central++
			if !energyPathAuxiliaryComponentResolved("fans", item, eligible) {
				t.Fatal("the actual central fan's DOAS ownership was lost")
			}
		}
	}
	if local != 5 || central != 1 || index.auxiliaryResolvable["fans"] {
		t.Fatalf("broad Fans inventory must retain5local+1central ambiguity: local%d central%d resolvable%v", local, central, index.auxiliaryResolvable)
	}
	// The default request remains lightweight. A dictionary's availability or
	// a PTAC's Part Load Ratio is not a direct fan-electricity observation.
	request := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	requestedMeter := false
	for _, output := range request.OutputObjects {
		if strings.EqualFold(output.ObjectType, "Output:Meter") && purposeFieldValue(output.Fields, "Key Name") == "Fans:Electricity" {
			requestedMeter = true
		}
		if strings.EqualFold(output.ObjectType, "Output:Variable") && strings.Contains(strings.ToLower(output.VariableName), "fan electricity") {
			t.Fatalf("the lightweight plan silently added heavy component fan energy: %s", output.VariableName)
		}
	}
	if !requestedMeter {
		t.Fatal("broad observed fan meter request disappeared")
	}
	for _, restricted := range []bool{false, true} {
		t.Run(fmt.Sprintf("DOAS_metadata_%v", restricted), func(t *testing.T) {
			paths := []string(nil)
			if restricted {
				paths = pathIDs
			}
			nodes := []EnergyExplanationNode{epath101AuditAuxiliary("fans", 100, "broad.fans.only", paths)}
			for i := 1; i <= 5; i++ {
				zone := fmt.Sprintf("SPACE%d-1", i)
				for _, service := range []string{"cooling", "heating"} {
					// Unequal known positive loads cannot legitimize either an
					// equal-Zone fallback or assigning the entire pool to DOAS.
					nodes = append(nodes, epath101AuditLoad(zone, service, float64(i*10), "load."+zone+"."+service, index.byZone[normalizePurposeToken(zone)]))
				}
			}
			plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, index, "M1", "monthly", true)
			if len(plan.Edges) != 0 || len(plan.FanSourceAllocations) != 0 || len(plan.Records) != 1 {
				t.Fatalf("unmeasured mixed fan pool produced direct/source-local/Zone allocation: %#v", plan)
			}
			record := plan.Records[0]
			if record.ExpectedValue != 100 || record.DirectValue != 0 || record.AllocatedValue != 0 || record.UnassignedValue != 100 || record.OvermappedValue != 0 || record.Method != "unassigned" || record.Carrier != "electricity" || len(record.SourceIDs) != 1 || record.SourceIDs[0] != "broad.fans.only" {
				t.Fatalf("mixed inventory lost truthful broad100/direct0/allocated0/unassigned100 accounting: %#v", record)
			}
		})
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(input, after) {
		t.Fatal("read-only topology/request/allocator test changed the original model")
	}
}
