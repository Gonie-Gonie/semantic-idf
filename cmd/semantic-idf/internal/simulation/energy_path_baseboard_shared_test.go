package simulation

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathBaseboardOriginalSharedConsumersUseExactNativePlantPaths(t *testing.T) {
	doc := energyPathBaseboardOriginal(t)
	before := doc.String()
	boilers := energyPathSharedHeatingElectricTargets(doc)
	towers := energyPathBaseboardHeatRejectionTargets(doc)
	if len(boilers) != 1 || boilers[0].Component.ObjectType != "Boiler:HotWater" || boilers[0].Component.ObjectName != "Central Boiler" {
		t.Fatalf("native shared boiler not proven: %+v", boilers)
	}
	if len(towers) != 1 || towers[0].Component.ObjectType != "CoolingTower:SingleSpeed" || towers[0].Component.ObjectName != "Central Tower" {
		t.Fatalf("native tower not proven: %+v", towers)
	}
	paths := map[string]idf.ZoneServicePath{}
	for _, summary := range idf.AnalyzeHVAC(doc).ServiceModel.ZoneServices {
		for _, path := range summary.Paths {
			paths[path.ID] = path
		}
	}
	zones := map[string]bool{}
	for _, id := range boilers[0].RelatedPathIDs {
		path, exists := paths[id]
		if !exists || energyCanonicalServiceKind(path.ServiceKind) != "heating" || path.PlantLoop == nil || path.PlantLoop.Name != "Hot Water Loop" || path.SpaceName != "" {
			t.Fatalf("boiler target inherited unrelated whole-service path: %+v", path)
		}
		zones[firstNonEmpty(path.ZoneName, path.ServedSubject.ZoneName)] = true
	}
	wantZones := map[string]bool{"SPACE1-1": true, "SPACE2-1": true, "SPACE3-1": true, "SPACE4-1": true, "SPACE5-1": true}
	if !reflect.DeepEqual(zones, wantZones) {
		t.Fatalf("native central heating paths need all five served Zones, never PLENUM: %v", zones)
	}
	for _, target := range append(boilers, towers...) {
		object := directHVACFixtureObject(t, &doc, target.Component.ObjectType, target.Component.ObjectName)
		if target.Component.ObjectIndex != object.Index {
			t.Fatal("shared consumer lost original object index")
		}
	}
	if before != doc.String() {
		t.Fatal("native shared target inspection mutated original model")
	}
	for _, scope := range []SimulationPurposeScope{{ZoneMode: "all"}, {ZoneMode: "selected", ZoneNames: []string{"SPACE2-1"}}} {
		plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, Scope: scope})
		counts := map[string]int{}
		for _, output := range plan.OutputObjects {
			if strings.HasPrefix(output.VariableName, "Boiler Ancillary Electricity ") {
				if output.KeyValue != "Central Boiler" || output.ScopeZoneName != "" || !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) {
					t.Fatalf("shared ancillary became local or lost exact native key: %+v", output)
				}
				counts[output.VariableName+"|"+output.ReportingFrequency]++
			}
			if output.ObjectType == "Output:Meter" && output.KeyValue == "HeatRejection:Electricity" {
				counts[output.KeyValue+"|"+output.ReportingFrequency]++
			}
		}
		for _, name := range []string{"Boiler Ancillary Electricity Energy", "Boiler Ancillary Electricity Rate", "HeatRejection:Electricity"} {
			for _, frequency := range []string{"Monthly", "Hourly"} {
				if counts[name+"|"+frequency] != 1 {
					t.Fatalf("exact shared native request missing/duplicated: %v", counts)
				}
			}
		}
		if len(counts) != 6 {
			t.Fatalf("unexpected ancillary capture request: %v", counts)
		}
	}
}

func TestEnergyPathBaseboardSharedConsumersRejectOriginalTopologyAmbiguity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, *idf.Document)
	}{
		{"duplicate boiler", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "Boiler:HotWater", "Central Boiler"))
		}},
		{"unsupported samekey boiler", func(t *testing.T, d *idf.Document) {
			p := *directHVACFixtureObject(t, d, "Boiler:HotWater", "Central Boiler")
			p.Type = "Boiler:Steam"
			energyPathBaseboardDuplicate(d, p)
		}},
		{"missing inlet", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "Boiler:HotWater", "Central Boiler").Fields[10].Value = ""
		}},
		{"crossed branch port", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "Branch", "Central Boiler Branch").Fields[5].Value = "Foreign Outlet"
		}},
		{"duplicate branch", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "Branch", "Central Boiler Branch"))
		}},
		{"disconnected second branch", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "Branch", "Central Boiler Branch"))
			d.Objects[len(d.Objects)-1].Fields[0].Value = "Unconnected Boiler Branch"
		}},
		{"unknown wrapper", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, idf.Object{Type: "Unsupported:Wrapper", Fields: []idf.Field{{Value: "Wrapper"}, {Value: "Boiler:HotWater"}, {Value: "Central Boiler"}}})
		}},
		{"duplicate plantloop", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "PlantLoop", "Hot Water Loop"))
		}},
		{"duplicate BranchList", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "BranchList", "Heating Supply Side Branches"))
		}},
		{"repeated branch within list", func(t *testing.T, d *idf.Document) {
			p := directHVACFixtureObject(t, d, "BranchList", "Heating Supply Side Branches")
			p.Fields = append(p.Fields, idf.Field{Value: "Central Boiler Branch"})
		}},
		{"other list repeats branch", func(t *testing.T, d *idf.Document) {
			p := directHVACFixtureObject(t, d, "BranchList", "Cooling Supply Side Branches")
			p.Fields = append(p.Fields, idf.Field{Value: "Central Boiler Branch"})
		}},
		{"other loop reuses list", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "PlantLoop", "Chilled Water Loop").Fields[12].Value = "Heating Supply Side Branches"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := energyPathBaseboardOriginal(t)
			tc.mutate(t, &doc)
			if targets := energyPathSharedHeatingElectricTargets(doc); len(targets) != 0 {
				t.Fatalf("ambiguous original boiler became exact shared target: %+v", targets)
			}
		})
	}
	doc := energyPathBaseboardOriginal(t)
	for position := range doc.Objects {
		if strings.EqualFold(doc.Objects[position].Type, "ZoneHVAC:EquipmentConnections") {
			doc.Objects[position].Type = "Unsupported:DisconnectedZoneEquipment"
		}
	}
	targets := energyPathSharedHeatingElectricTargets(doc)
	if len(targets) != 1 || len(targets[0].RelatedPathIDs) != 0 {
		t.Fatalf("known consuming component with no delivery roster must remain explicitly unassigned: %+v", targets)
	}
	if len(energyPathSharedHeatingElectricTargets(energyPathRadiantDocument(t))) != 0 {
		t.Fatal("bounded mixed capture changed a previously accepted non-baseboard fixture")
	}
}
