package simulation

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func windowACOriginal(t *testing.T) (idf.Document, string) {
	t.Helper()
	path := filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "MultiStory.idf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "c7165328a5f3a90aa81ac3f95928600079cf9f7a109fd615ccc6686caf518ba2" {
		t.Fatalf("official original hash changed: %s", got)
	}
	return parsePurposePlanFixture(t, string(data)), path
}

func TestEnergyPathWindowACOriginalNineOwnersAndTwentySevenNativeTargets(t *testing.T) {
	doc, path := windowACOriginal(t)
	before := doc.String()
	bytesBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][4]int{
		"Gnd West": {68, 70, 72, 73}, "Gnd Center": {89, 90, 92, 93}, "Gnd East": {110, 111, 113, 114},
		"Mid West": {133, 134, 136, 137}, "Mid Center": {153, 154, 156, 157}, "Mid East": {174, 175, 177, 178},
		"Top West": {197, 198, 200, 201}, "Top Center": {217, 218, 220, 221}, "Top East": {238, 239, 241, 242},
	}
	bindings := energyPathWindowACTargets(doc)
	if len(bindings) != 9 {
		t.Fatalf("native original roster=%+v", bindings)
	}
	expected := map[string]string{}
	for _, target := range bindings {
		prefix := strings.TrimSuffix(target.ZoneName, " Zone")
		positions, ok := want[prefix]
		if !ok || target.Parent.ObjectName != prefix+" Window AC" || target.Mixer.ObjectName != prefix+" OA Mixer" || target.Fan.ObjectName != prefix+" OA Fan" || target.Coil.ObjectName != prefix+" OA DX Coil" || [4]int{target.Parent.ObjectIndex, target.Mixer.ObjectIndex, target.Fan.ObjectIndex, target.Coil.ObjectIndex} != positions {
			t.Fatalf("native original typed roster changed: %+v", target)
		}
		for _, component := range []idf.ComponentRef{target.Parent, target.Mixer, target.Fan, target.Coil} {
			if component.ID != fmt.Sprintf("component:%d", component.ObjectIndex) {
				t.Fatalf("prepared-copy identity borrowed: %+v", component)
			}
		}
		expected[target.ZoneName+"\x00"+target.Coil.ObjectName+"\x00Cooling Coil Electricity Energy"] = "cooling.coil.electricity"
		expected[target.ZoneName+"\x00"+target.Coil.ObjectName+"\x00Cooling Coil Crankcase Heater Electricity Energy"] = "cooling.coil.crankcase_electricity"
		expected[target.ZoneName+"\x00"+target.Fan.ObjectName+"\x00Fan Electricity Energy"] = "fans.zone_equipment.electricity"
	}
	direct := energyPathWindowACDirectTargets(bindings)
	seen := map[string]string{}
	for _, target := range direct {
		if len(target.Definition.Energy.Aliases) != 1 || target.Definition.Energy.Carrier != "electricity" || target.Definition.Energy.HierarchyLevel != "zone_direct_use" {
			t.Fatalf("incorrect native direct family: %+v", target)
		}
		key := target.ZoneName + "\x00" + target.KeyValue + "\x00" + target.Definition.Energy.Aliases[0]
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate native constituent %q", key)
		}
		seen[key] = target.Definition.ID
		wantEndUse := "cooling"
		if target.Definition.ID == "fans.zone_equipment.electricity" {
			wantEndUse = "fans"
		}
		if target.Definition.Energy.EndUse != wantEndUse {
			t.Fatalf("fan and cooling source boundaries mixed: %+v", target)
		}
	}
	if !reflect.DeepEqual(seen, expected) {
		t.Fatalf("native direct roster=%v want%v", seen, expected)
	}
	for _, scope := range []SimulationPurposeScope{{}, {ZoneMode: "selected", ZoneNames: []string{"Mid Center Zone"}}} {
		plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, Scope: scope})
		// The native roster has 27 Monthly accounting identities. The existing
		// addEnergyPathHourlyOutputs pass also requests those same 27 source
		// charts; these separate observations are never Monthly consumption.
		outputs := map[string]map[string]bool{"Monthly": {}, "Hourly": {}}
		for _, output := range plan.OutputObjects {
			name := output.VariableName
			if name != "Cooling Coil Electricity Energy" && name != "Cooling Coil Crankcase Heater Electricity Energy" && name != "Fan Electricity Energy" {
				continue
			}
			key := output.ScopeZoneName + "\x00" + output.KeyValue + "\x00" + name
			frequencyOutputs, supportedFrequency := outputs[output.ReportingFrequency]
			if expected[key] == "" || !supportedFrequency || frequencyOutputs[key] || output.ObjectType != "Output:Variable" || !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) {
				t.Fatalf("invalid/duplicate native output: %+v", output)
			}
			if scope.ZoneMode != "" && output.ScopeZoneName != "Mid Center Zone" {
				t.Fatalf("scope borrowed another original owner: %+v", output)
			}
			if output.ReportingFrequency == "Hourly" && (output.Weight != "heavy" || output.Reason != "Basic Energy Path" || !strings.Contains(output.Description, "source")) {
				t.Fatalf("automatic Hourly companion lost its nonadditive source-chart purpose: %+v", output)
			}
			frequencyOutputs[key] = true
		}
		wantCount := 27
		if scope.ZoneMode != "" {
			wantCount = 3
		}
		for _, frequency := range []string{"Monthly", "Hourly"} {
			if len(outputs[frequency]) != wantCount {
				t.Fatalf("native %s outputs=%d want%d", frequency, len(outputs[frequency]), wantCount)
			}
		}
		if !reflect.DeepEqual(outputs["Monthly"], outputs["Hourly"]) {
			t.Fatalf("Hourly charts borrowed a different native owner/key/family: Monthly=%v Hourly=%v", outputs["Monthly"], outputs["Hourly"])
		}
	}
	bytesAfter, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(bytesBefore, bytesAfter) || doc.String() != before {
		t.Fatal("planning modified original IDF")
	}
}

func windowACHandDocument(t *testing.T) idf.Document {
	t.Helper()
	var input strings.Builder
	input.WriteString("Version,25.1;\n")
	for i := 1; i <= 4; i++ {
		zone := fmt.Sprintf("Window Zone %d", i)
		factor := 1
		if i == 2 || i == 4 {
			factor = 4
		}
		fmt.Fprintf(&input, "Zone,%s,0,0,0,0,1,%d,3,72,24;\n", zone, factor)
		fmt.Fprintf(&input, "ZoneHVAC:EquipmentConnections,%[1]s,%[1]s Equipment,%[1]s Outlet,%[1]s Inlet,%[1]s Air,;\n", zone)
		fmt.Fprintf(&input, "ZoneHVAC:EquipmentList,%[1]s Equipment,SequentialLoad,ZoneHVAC:WindowAirConditioner,%[1]s Window AC,1,1,,;\n", zone)
		fmt.Fprintf(&input, "ZoneHVAC:WindowAirConditioner,%[1]s Window AC,Always,Autosize,Autosize,%[1]s Inlet,%[1]s Outlet,OutdoorAir:Mixer,%[1]s Mixer,Fan:OnOff,%[1]s Fan,Coil:Cooling:DX:SingleSpeed,%[1]s Coil,Cycling,BlowThrough,.001;\n", zone)
		fmt.Fprintf(&input, "OutdoorAir:Mixer,%[1]s Mixer,%[1]s Mixed,%[1]s Outdoor,%[1]s Relief,%[1]s Inlet;\nOutdoorAir:Node,%[1]s Outdoor;\n", zone)
		fmt.Fprintf(&input, "Fan:OnOff,%[1]s Fan,Always,.7,75,Autosize,.9,1,%[1]s Mixed,%[1]s Fan Outlet;\n", zone)
		fmt.Fprintf(&input, "Coil:Cooling:DX:SingleSpeed,%[1]s Coil,Always,Autosize,.75,3,Autosize,,934.4,%[1]s Fan Outlet,%[1]s Outlet;\n", zone)
	}
	input.WriteString("ZoneList,Repeated Floors,Window Zone 3,Window Zone 4;\nZoneGroup,Repeated Group,Repeated Floors,8;\n")
	return parsePurposePlanFixture(t, input.String())
}
