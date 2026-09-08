package simulation

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathRadiantServicePathOriginal(t *testing.T) (energyServicePathIndex, idf.HVACReport) {
	t.Helper()
	path := filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "RadLoTempCFloHeatCool.idf")
	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(input)); got != "c4988a4211b37a9508dd0265ee19cb1f4f7174c57e214a38ec3935e2f918388e" {
		t.Fatalf("original radiant input changed: %s", got)
	}
	t.Cleanup(func() {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(input, after) {
			t.Error("service-path regression changed the immutable original model")
		}
	})
	return buildEnergyServicePathIndex(path), idf.AnalyzeHVAC(parsePurposePlanFixture(t, string(input)))
}

func epathRadiantServicePathZones() []string {
	return []string{"West Zone", "EAST ZONE", "NORTH ZONE"}
}

func TestEnergyPathRadiantServicePathsSeparateOriginalWaterPorts(t *testing.T) {
	topology, report := epathRadiantServicePathOriginal(t)
	seen, pathIDs := map[string]int{}, map[string]bool{}
	for _, path := range topology.auxiliaryPaths {
		if path.ID == "" || pathIDs[path.ID] {
			t.Fatalf("empty or duplicated original path identity: %#v", path)
		}
		pathIDs[path.ID] = true
		if path.AirLoopName != "" {
			t.Fatalf("radiant water port acquired an air-loop owner: %#v", path)
		}
		wantLoop := map[string]string{"cooling": "Chilled Water Loop", "heating": "Hot Water Loop"}[path.ServiceKind]
		if wantLoop == "" || path.PlantLoopName != wantLoop {
			t.Fatalf("wrong original water-port service/plant: %#v", path)
		}
		wantCondenser := ""
		if path.ServiceKind == "cooling" {
			wantCondenser = "Chilled Water Condenser Loop"
		}
		if path.CondenserLoopName != wantCondenser {
			t.Fatalf("condenser relation escaped the original electric-chiller cooling chain: %#v", path)
		}
		seen[path.ZoneName+"|"+path.ServiceKind]++
	}
	if len(topology.auxiliaryPaths) != 6 || len(seen) != 6 {
		t.Fatalf("want exactly three cooling and three heating original paths: %v", seen)
	}
	for _, zone := range epathRadiantServicePathZones() {
		for _, service := range []string{"cooling", "heating"} {
			if seen[zone+"|"+service] != 1 || len(topology.byZoneService[normalizePurposeToken(zone)+"|"+service]) != 1 {
				t.Fatalf("missing/duplicated original %s %s delivery: %v", zone, service, seen)
			}
		}
	}
	for _, test := range []struct{ loop, allowed, forbidden string }{
		{"Chilled Water Loop", "cooling", "heating"},
		{"Hot Water Loop", "heating", "cooling"},
	} {
		key := normalizePurposeToken(test.loop) + "|"
		if len(topology.byLoopService[key+test.allowed]) != 3 || len(topology.byLoopService[key+test.forbidden]) != 0 {
			t.Fatalf("loop %s crossed service ports: %v", test.loop, topology.byLoopService)
		}
	}
	if len(topology.byLoopService[normalizePurposeToken("Chilled Water Condenser Loop")+"|heating"]) != 0 {
		t.Fatal("false CHW heating contaminated the condenser index")
	}
	eligible := energyPathZoneAuxiliaryEligiblePaths("heat_rejection", nil, topology.auxiliaryPaths)
	owners := map[string]int{}
	for _, path := range eligible {
		if path.ServiceKind != "cooling" || path.PlantLoopName != "Chilled Water Loop" || path.CondenserLoopName != "Chilled Water Condenser Loop" {
			t.Fatalf("tower eligibility included an unrelated original service: %#v", path)
		}
		owners[path.ZoneName]++
	}
	if len(eligible) != 3 || !topology.auxiliaryResolvable["heat_rejection"] || topology.auxiliaryResolvable["pumps"] || topology.auxiliaryResolvable["fans"] {
		t.Fatalf("tower and mixed-pump/Fan absence boundaries changed: eligible=%v resolvable=%v", eligible, topology.auxiliaryResolvable)
	}
	for _, zone := range epathRadiantServicePathZones() {
		if owners[zone] != 1 {
			t.Fatalf("tower lacks exact original downstream Zone %s: %v", zone, owners)
		}
	}
	pumps, towers, parents := map[string]bool{}, map[string]bool{}, 0
	for _, item := range report.ServiceModel.Components {
		switch item.Component.ObjectType {
		case "Pump:VariableSpeed":
			pumps[item.Component.ObjectName] = true
		case "CoolingTower:SingleSpeed":
			towers[item.Component.ObjectName] = true
			if !energyPathAuxiliaryComponentResolved("heat_rejection", item, eligible) {
				t.Fatal("original tower was not connected to its validated condenser path")
			}
		case "ZoneHVAC:LowTemperatureRadiant:ConstantFlow":
			parents++
		}
	}
	if len(pumps) != 3 || !pumps["Circ Pump"] || !pumps["Cond Circ Pump"] || !pumps["HW Circ Pump"] || len(towers) != 1 || !towers["Big Tower"] || parents != 3 {
		t.Fatalf("original constituent inventory was dropped: pumps=%v towers=%v radiant=%d", pumps, towers, parents)
	}
}

func epathRadiantServicePathNodes(topology energyServicePathIndex, cooling bool) []EnergyExplanationNode {
	nodes := []EnergyExplanationNode{
		epath101AuditCarrier(150),
		epath101AuditAuxiliary("heat_rejection", 50, "meter.tower", nil),
		epath101AuditAuxiliary("pumps", 100, "meter.mixed_pumps", nil),
	}
	for i, zone := range epathRadiantServicePathZones() {
		for _, service := range []string{"cooling", "heating"} {
			// Deliberately different C/H vectors: a false combined-service
			// denominator yields 17.5/12.5/20, not the required 30/15/5.
			value := map[string][]float64{"cooling": {60, 30, 10}, "heating": {10, 20, 70}}[service][i]
			if service == "cooling" && !cooling {
				value = 0
			}
			nodes = append(nodes, epath101AuditLoad(zone, service, value, "load."+service+"."+zone, topology.byZoneService[normalizePurposeToken(zone)+"|"+service]))
		}
	}
	return nodes
}

func TestEnergyPathRadiantServicePathsTowerUsesCoolingOnlyPumpsStayUnassigned(t *testing.T) {
	topology, _ := epathRadiantServicePathOriginal(t)
	for _, period := range []struct{ id, kind string }{{"M1", "monthly"}, {"annual", "annual"}} {
		t.Run(period.id, func(t *testing.T) {
			plan := buildEnergyPathZoneAuxiliaryAllocationPlan(epathRadiantServicePathNodes(topology, true), nil, topology, period.id, period.kind, false)
			epath101AuditAssertRecord(t, plan.Records, "heat_rejection", 50, 0, 50, 0)
			epath101AuditAssertRecord(t, plan.Records, "pumps", 100, 0, 0, 100)
			if len(plan.Records) != 2 || len(plan.Edges) != 3 || len(plan.FanSourceAllocations) != 0 {
				t.Fatalf("invented individual pump/fan pools or dropped tower owners: %#v", plan)
			}
			if epath101AuditRecord(plan.Records, "heat_rejection").Method != "condenser_loop_load_share" || epath101AuditRecord(plan.Records, "pumps").Method != "unassigned" {
				t.Fatalf("modeled condenser share confused with mixed pump metering: %#v", plan.Records)
			}
			for i, zone := range epathRadiantServicePathZones() {
				paths := topology.byZoneService[normalizePurposeToken(zone)+"|cooling"]
				epath101AuditAssertZoneAllocation(t, plan.Edges, zone, []float64{30, 15, 5}[i], "load.cooling."+zone, "meter.tower", paths)
				edge := epath101AuditAllocationEdge(plan.Edges, zone)
				if edge == nil || edge.FromID != "energy.end_use.heat_rejection.electricity" || edge.ServiceKind != "cooling" || edge.Period != period.id || len(edge.SourceIDs) != 2 {
					t.Fatalf("tower branch lost its distinct cooling-only source/service: %#v", edge)
				}
				for _, source := range edge.SourceIDs {
					if strings.Contains(source, "heating") || source == "meter.mixed_pumps" {
						t.Fatalf("tower share included heating or mixed pump evidence: %#v", edge)
					}
				}
			}
			pump := epath101AuditRecord(plan.Records, "pumps")
			if len(pump.SourceIDs) != 1 || pump.SourceIDs[0] != "meter.mixed_pumps" {
				t.Fatalf("unassigned pump ledger invented a measured constituent: %#v", pump)
			}
		})
	}
}

func TestEnergyPathRadiantServicePathsHeatingCannotDriveTower(t *testing.T) {
	topology, _ := epathRadiantServicePathOriginal(t)
	plan := buildEnergyPathZoneAuxiliaryAllocationPlan(epathRadiantServicePathNodes(topology, false), nil, topology, "M1", "monthly", false)
	epath101AuditAssertRecord(t, plan.Records, "heat_rejection", 50, 0, 0, 50)
	epath101AuditAssertRecord(t, plan.Records, "pumps", 100, 0, 0, 100)
	if len(plan.Records) != 2 || len(plan.Edges) != 0 || epath101AuditRecord(plan.Records, "heat_rejection").Method != "unassigned" {
		t.Fatalf("positive heating/zero cooling manufactured tower shares: %#v", plan)
	}
}
