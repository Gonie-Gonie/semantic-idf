package simulation

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathInternalBoundaryInputsSeparateSourcesAndRoomResponse(t *testing.T) {
	f := newThermalInputFixture(t)
	values, sources, months := f.read(t)
	for kind, category := range map[string]string{"input.people": energyDriverCategoryPeople, "input.lighting": energyDriverCategoryLighting, "input.equipment.electric": energyDriverCategoryEquipment, "input.equipment.gas": energyDriverCategoryEquipment, "input.equipment.other": energyDriverCategoryEquipment, "input.equipment.hot_water": energyDriverCategoryEquipment, "input.equipment.steam": energyDriverCategoryEquipment} {
		thermalInputAssertValue(t, values, months, "", "Annual", kind, category, 182, true)
	}
	driver := EnergyExplanationNode{ID: "people", Level: "driver", DriverCategory: energyDriverCategoryPeople, Value: 7, Unit: "kWh", ScaleDomain: "thermal", Period: "Annual", SourceIDs: []string{"people-source"}}
	result := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Nodes: []EnergyExplanationNode{driver}, Sources: sources}
	appendEnergyPathThermalInputGraphs(&result, values, months)
	peopleLinks := 0
	for _, node := range result.Nodes {
		if node.Level == "input" && energyPathThermalInputIsResponseContext(node.Kind) {
			t.Fatalf("building response was represented as an imposed input: %+v", node)
		}
	}
	for _, link := range result.Links {
		if link.ToID != driver.ID || link.ToValue != 7 || link.FromValue != 182 || !strings.Contains(link.FromID, "input.people") {
			t.Fatalf("input crossed internal families or changed actual driver allocation: %+v", link)
		}
		peopleLinks++
	}
	if peopleLinks != 1 || result.Nodes[0].Value != driver.Value {
		t.Fatalf("expected one prescribed-People → allocated-internal-gain relation: %+v", result.Links)
	}
	responseSources := 0
	for _, source := range result.Sources {
		if source.Name == "Surface Heat Storage Energy" || source.Name == "Surface Outside Face Solar Radiation Heat Gain Energy" || source.Name == "Zone Total Internal Total Heating Energy" {
			responseSources++
		}
	}
	if responseSources != 6 {
		t.Fatalf("building-response source provenance was discarded: %d", responseSources)
	}
}

func TestEnergyPathInternalBoundaryInputOwnersAbsentZeroAndMissingFamilies(t *testing.T) {
	f := newThermalInputFixture(t)
	// Occupants exist in Office only. The unoccupied Lab must neither prevent
	// the complete Building total nor supply a foreign SQL observation.
	for i := range f.doc.Objects {
		if strings.EqualFold(f.doc.Objects[i].Type, "People") {
			f.doc.Objects[i].Fields[1].Value = "Office"
		}
	}
	f.context = newEnergyDriverBuildContext(f.geometry, f.doc)
	values, _, months := f.read(t)
	thermalInputAssertValue(t, values, months, "", "Annual", "input.people", energyDriverCategoryPeople, 180, true)
	thermalInputAssertValue(t, values, months, "Lab", "Annual", "input.people", energyDriverCategoryPeople, 0, false)
	db, err := sql.Open("sqlite", f.path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex IN (SELECT ReportDataDictionaryIndex FROM ReportDataDictionary WHERE Name='Zone People Total Heating Energy' AND KeyValue='Office'); DELETE FROM ReportData WHERE TimeIndex=102 AND ReportDataDictionaryIndex IN (SELECT ReportDataDictionaryIndex FROM ReportDataDictionary WHERE Name='Zone Gas Equipment Total Heating Energy' AND KeyValue='Office')`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	values, _, months = f.read(t)
	thermalInputAssertValue(t, values, months, "", "Annual", "input.people", energyDriverCategoryPeople, 0, true)
	thermalInputAssertValue(t, values, months, "", "Annual", "input.equipment.gas", energyDriverCategoryEquipment, 0, false)
	thermalInputAssertValue(t, values, months, "", "Annual", "input.equipment.electric", energyDriverCategoryEquipment, 182, true)
	thermalInputAssertValue(t, values, months, "", "Annual", "input.lighting", energyDriverCategoryLighting, 182, true)
}

func TestEnergyPathInternalBoundaryInputsRejectDuplicateNativeFamily(t *testing.T) {
	f := newThermalInputFixture(t)
	db, err := sql.Open("sqlite", f.path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO ReportDataDictionary VALUES(70,'Office','Zone Electric Equipment Total Heating Energy','J','Monthly',0); INSERT INTO ReportData VALUES(101,70,3600000),(102,70,3600000)`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	values, _, months := f.read(t)
	thermalInputAssertValue(t, values, months, "", "Annual", "input.equipment.electric", energyDriverCategoryEquipment, 0, false)
	thermalInputAssertValue(t, values, months, "", "Annual", "input.equipment.gas", energyDriverCategoryEquipment, 182, true)
}

func TestEnergyPathInternalBoundaryInputTypedListsAndModelAwareRequests(t *testing.T) {
	doc := parsePurposePlanFixture(t, "Version,25.1; Zone,Office; Zone,Lab; Zone,Plenum; Space,West,Office; Space,East,Lab; SpaceList,Occupied,West,East; People,Occupants,Occupied; Lights,Lights,Office; OtherEquipment,Process,None,Lab;")
	geometry := idf.GeometryReport{Zones: []idf.GeometryZone{{Name: "Office"}, {Name: "Lab"}, {Name: "Plenum"}}, Spaces: []idf.GeometrySpace{{Name: "West", ZoneName: "Office"}, {Name: "East", ZoneName: "Lab"}}}
	owners := energyPathInternalInputOwners(geometry, doc)
	if len(owners["input.people"]) != 2 || !owners["input.people"]["office"] || !owners["input.people"]["lab"] || owners["input.people"]["plenum"] || !owners["input.equipment.other"]["lab"] || len(owners["input.equipment.electric"]) != 0 {
		t.Fatalf("typed native owner expansion=%+v", owners)
	}
	builder := newPurposePlanBuilder(doc, NormalizeSimulationPurposeRequest(&SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}}))
	builder.geometry = geometry
	builder.addEnergyPathThermalInputOutputs([]string{"Office", "Lab", "Plenum"})
	people, lights, equipment := 0, 0, 0
	for _, object := range builder.objects {
		switch object.VariableName {
		case "Zone People Total Heating Energy":
			people++
		case "Zone Lights Total Heating Energy":
			lights++
		case "Zone Other Equipment Total Heating Energy":
			equipment++
		case "Zone Electric Equipment Total Heating Energy", "Zone Gas Equipment Total Heating Energy", "Zone Hot Water Equipment Total Heating Energy", "Zone Steam Equipment Total Heating Energy":
			t.Fatalf("absent equipment family requested: %+v", object)
		}
		if object.KeyValue == "Plenum" && object.VariableName != "Zone Total Internal Total Heating Energy" {
			t.Fatalf("unoccupied plenum acquired prescribed input: %+v", object)
		}
	}
	if people != 2 || lights != 1 || equipment != 1 {
		t.Fatalf("model-aware input roster=%d/%d/%d", people, lights, equipment)
	}
	for _, invalid := range []string{
		"Version,25.1; Zone,Office; ZoneList,WrongList,West; People,Occupants,WrongList;",
		"Version,25.1; Zone,Office; SpaceList,WrongList,Office; People,Occupants,WrongList;",
		"Version,25.1; Zone,Office; ZoneList,Office,Office; People,Occupants,Office;",
	} {
		invalidOwners := energyPathInternalInputOwners(geometry, parsePurposePlanFixture(t, invalid))
		if len(invalidOwners["input.people"]) != 0 {
			t.Fatalf("ambiguous or incorrectly typed list proved ownership: %+v", invalidOwners)
		}
	}
}
