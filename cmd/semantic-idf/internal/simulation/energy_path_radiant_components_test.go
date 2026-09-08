package simulation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func energyPathRadiantDocument(t *testing.T) idf.Document {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "RadLoTempCFloHeatCool.idf"))
	if err != nil {
		t.Fatal(err)
	}
	return parsePurposePlanFixture(t, string(data))
}

func TestEnergyPathRadiantOriginalMonthlyRequestKeys(t *testing.T) {
	doc := energyPathRadiantDocument(t)
	before := doc.String()
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	owners := map[string]string{"WEST ZONE RADIANT FLOOR": "West Zone", "EAST ZONE RADIANT FLOOR": "EAST ZONE", "NORTH ZONE RADIANT FLOOR": "NORTH ZONE"}
	aliases := map[string]bool{"Zone Radiant HVAC Cooling Energy": true, "Zone Radiant HVAC Cooling Rate": true, "Zone Radiant HVAC Heating Energy": true, "Zone Radiant HVAC Heating Rate": true}
	seen := map[string]bool{}
	for _, output := range plan.OutputObjects {
		if !aliases[output.VariableName] || output.ReportingFrequency != "Monthly" {
			continue
		}
		owner, found := owners[strings.ToUpper(output.KeyValue)]
		if !found || output.ObjectType != "Output:Variable" || output.ScopeZoneName != owner || !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) {
			t.Errorf("radiant Monthly request lacks exact original equipment/owner: %#v", output)
			continue
		}
		key := strings.ToUpper(output.KeyValue) + "|" + output.VariableName
		if seen[key] {
			t.Errorf("duplicate radiant request %s", key)
		}
		seen[key] = true
	}
	if len(seen) != 12 {
		t.Fatalf("exact radiant Monthly requests=%d, want 12 (three equipment owners, four J/W load aliases)", len(seen))
	}
	if before != doc.String() {
		t.Fatal("planning mutated original geometry, equipment, or wildcard Timestep outputs")
	}
}

func energyPathRadiantHandDocument(t *testing.T) idf.Document {
	t.Helper()
	// Commentless Design-Object schema; the two Branches reference distinct
	// water ports of ONE Zone-owned radiant parent, not two equipment owners.
	return parsePurposePlanFixture(t, `
Version,25.1;
Zone,Office,0,0,0,0,1,7;
Zone,Other,0,0,0,0,1,1;
BuildingSurface:Detailed,Floor,Floor,Slab,Office,,Ground,,NoSun,NoWind,1,3,0,0,0,1,0,0,0,1,0;
BuildingSurface:Detailed,Other Floor,Floor,Slab,Other,,Ground,,NoSun,NoWind,1,3,0,0,0,1,0,0,0,1,0;
ZoneHVAC:EquipmentConnections,Office,List,,,Office Air,;
ZoneHVAC:EquipmentList,List,SequentialLoad,ZoneHVAC:LowTemperatureRadiant:ConstantFlow,Radiant,1,1,,;
ZoneHVAC:LowTemperatureRadiant:ConstantFlow,Radiant,Design,Always,Office,Floor,400,.0004,,75000,50,HW In,HW Out,H1,H2,H3,H4,CW In,CW Out,C1,C2,C3,C4,,;
ZoneHVAC:LowTemperatureRadiant:ConstantFlow:Design,Design,ConvectionOnly,.012,.016,.35,MeanAirTemperature,.8,.87,.1,SimpleOff,1;
Branch,Heating Branch,,ZoneHVAC:LowTemperatureRadiant:ConstantFlow,Radiant,HW In,HW Out;
Branch,Cooling Branch,,ZoneHVAC:LowTemperatureRadiant:ConstantFlow,Radiant,CW In,CW Out;
Output:Variable,*,Zone Radiant HVAC Cooling Energy,Timestep;
Output:Variable,*,Zone Radiant HVAC Cooling Rate,Timestep;
Output:Variable,*,Zone Radiant HVAC Heating Energy,Timestep;
Output:Variable,*,Zone Radiant HVAC Heating Rate,Timestep;
Output:Variable,*,Zone Radiant HVAC Pump Electricity Energy,Timestep;
`)
}

func TestEnergyPathRadiantTypedOriginalAndHandOwners(t *testing.T) {
	for _, fixture := range []struct {
		name string
		doc  idf.Document
		want map[string]string
	}{
		{"original", energyPathRadiantDocument(t), map[string]string{"West Zone Radiant Floor": "West Zone", "East Zone Radiant Floor": "EAST ZONE", "North Zone Radiant Floor": "NORTH ZONE"}},
		{"commentless_hand_multiplier7", energyPathRadiantHandDocument(t), map[string]string{"Radiant": "Office"}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			before := fixture.doc.String()
			targets := energyPathRadiantLoadTargets(fixture.doc)
			if len(targets) != len(fixture.want) {
				t.Fatalf("typed radiant owners=%#v, want %v", targets, fixture.want)
			}
			for _, target := range targets {
				object := directHVACFixtureObject(t, &fixture.doc, "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", target.KeyValue)
				surface := directHVACFixtureObject(t, &fixture.doc, "BuildingSurface:Detailed", object.Fields[4].Value)
				if fixture.want[target.KeyValue] != target.ZoneName || target.Component.ObjectType != object.Type || target.Component.ObjectName != target.KeyValue ||
					target.Component.ObjectIndex != object.Index || target.Component.ID != fmt.Sprintf("component:%d", object.Index) || target.SurfaceName != surface.Fields[0].Value {
					t.Fatalf("target lost original typed identity/owner: %#v", target)
				}
			}
			if before != fixture.doc.String() {
				t.Fatal("ownership lookup mutated original input")
			}
		})
	}
}

func TestEnergyPathRadiantOriginalHeatRejectionRequest(t *testing.T) {
	doc := energyPathRadiantDocument(t)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	count := 0
	for _, output := range plan.OutputObjects {
		if strings.EqualFold(output.ObjectType, "Output:Meter") && strings.EqualFold(output.KeyValue, "HeatRejection:Electricity") && strings.EqualFold(output.ReportingFrequency, "Monthly") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("original BIG TOWER needs one exact Monthly heat-rejection meter request, got %d", count)
	}
}

func TestEnergyPathRadiantRejectsAmbiguousOrForeignOwnership(t *testing.T) {
	object := func(t *testing.T, doc *idf.Document, typ, name string) *idf.Object {
		return directHVACFixtureObject(t, doc, typ, name)
	}
	duplicate := func(doc *idf.Document, source idf.Object) {
		source.Fields = append([]idf.Field(nil), source.Fields...)
		source.Index = len(doc.Objects)
		doc.Objects = append(doc.Objects, source)
	}
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *idf.Document)
	}{
		{"explicit_zone_mismatch", func(t *testing.T, d *idf.Document) {
			object(t, d, energyPathRadiantConstantFlowType, "Radiant").Fields[3].Value = "Other"
		}},
		{"surface_foreign_zone", func(t *testing.T, d *idf.Document) {
			object(t, d, "BuildingSurface:Detailed", "Floor").Fields[3].Value = "Other"
		}},
		{"surface_foreign_space", func(t *testing.T, d *idf.Document) {
			object(t, d, "BuildingSurface:Detailed", "Floor").Fields[4].Value = "Foreign Space"
			d.Objects = append(d.Objects, idf.Object{Index: len(d.Objects), Type: "Space", Fields: []idf.Field{{Value: "Foreign Space"}, {Value: "Other"}}})
		}},
		{"equipment_not_listed", func(t *testing.T, d *idf.Document) {
			object(t, d, "ZoneHVAC:EquipmentList", "List").Fields[3].Value = "Missing"
		}},
		{"wrong_equipment_type", func(t *testing.T, d *idf.Document) {
			object(t, d, "ZoneHVAC:EquipmentList", "List").Fields[2].Value = "ZoneHVAC:LowTemperatureRadiant:VariableFlow"
		}},
		{"missing_list_connection", func(t *testing.T, d *idf.Document) {
			object(t, d, "ZoneHVAC:EquipmentConnections", "Office").Fields[1].Value = "Missing"
		}},
		{"duplicate_equipment", func(t *testing.T, d *idf.Document) {
			duplicate(d, *object(t, d, energyPathRadiantConstantFlowType, "Radiant"))
		}},
		{"case_only_duplicate_equipment", func(t *testing.T, d *idf.Document) {
			copy := *object(t, d, energyPathRadiantConstantFlowType, "Radiant")
			duplicate(d, copy)
			d.Objects[len(d.Objects)-1].Fields[0].Value = " radiant "
		}},
		{"cross_type_reporting_name_collision", func(t *testing.T, d *idf.Document) {
			copy := *object(t, d, energyPathRadiantConstantFlowType, "Radiant")
			copy.Type = "ZoneHVAC:LowTemperatureRadiant:VariableFlow"
			duplicate(d, copy)
		}},
		{"duplicate_zone", func(t *testing.T, d *idf.Document) { duplicate(d, *object(t, d, "Zone", "Office")) }},
		{"duplicate_surface", func(t *testing.T, d *idf.Document) { duplicate(d, *object(t, d, "BuildingSurface:Detailed", "Floor")) }},
		{"duplicate_design", func(t *testing.T, d *idf.Document) {
			duplicate(d, *object(t, d, energyPathRadiantConstantFlowType+":Design", "Design"))
		}},
		{"wrong_design_type", func(t *testing.T, d *idf.Document) {
			object(t, d, energyPathRadiantConstantFlowType+":Design", "Design").Type = "ZoneHVAC:LowTemperatureRadiant:VariableFlow:Design"
		}},
		{"duplicate_equipment_list", func(t *testing.T, d *idf.Document) { duplicate(d, *object(t, d, "ZoneHVAC:EquipmentList", "List")) }},
		{"duplicate_connection", func(t *testing.T, d *idf.Document) {
			duplicate(d, *object(t, d, "ZoneHVAC:EquipmentConnections", "Office"))
		}},
		{"two_zone_list_owners", func(t *testing.T, d *idf.Document) {
			duplicate(d, *object(t, d, "ZoneHVAC:EquipmentConnections", "Office"))
			d.Objects[len(d.Objects)-1].Fields[0].Value = "Other"
		}},
		{"second_zone_connection_different_list", func(t *testing.T, d *idf.Document) {
			duplicate(d, *object(t, d, "ZoneHVAC:EquipmentConnections", "Office"))
			d.Objects[len(d.Objects)-1].Fields[1].Value = "Other List"
		}},
		{"disconnected_second_list_reference", func(t *testing.T, d *idf.Document) {
			duplicate(d, *object(t, d, "ZoneHVAC:EquipmentList", "List"))
			d.Objects[len(d.Objects)-1].Fields[0].Value = "Unconnected List"
		}},
		{"shared_surface_different_equipment", func(t *testing.T, d *idf.Document) {
			duplicate(d, *object(t, d, energyPathRadiantConstantFlowType, "Radiant"))
			d.Objects[len(d.Objects)-1].Fields[0].Value = "Other Radiant"
		}},
		{"unsupported_surface_group", func(t *testing.T, d *idf.Document) {
			object(t, d, energyPathRadiantConstantFlowType, "Radiant").Fields[4].Value = "Group"
			d.Objects = append(d.Objects, idf.Object{Index: len(d.Objects), Type: "ZoneHVAC:LowTemperatureRadiant:SurfaceGroup", Fields: []idf.Field{{Value: "Group"}, {Value: "Floor"}, {Value: "1"}}})
		}},
		{"unknown_typed_parent", func(t *testing.T, d *idf.Document) {
			d.Objects = append(d.Objects, idf.Object{Index: len(d.Objects), Type: "Unsupported:Wrapper", Fields: []idf.Field{{Value: "Wrapper"}, {Value: energyPathRadiantConstantFlowType}, {Value: "Radiant"}}})
		}},
		{"crossed_branch_ports", func(t *testing.T, d *idf.Document) {
			object(t, d, "Branch", "Heating Branch").Fields[5].Value = "CW Out"
		}},
		{"duplicated_branch_port_pair", func(t *testing.T, d *idf.Document) {
			duplicate(d, *object(t, d, "Branch", "Heating Branch"))
			d.Objects[len(d.Objects)-1].Fields[0].Value = "Disconnected Duplicate Branch"
		}},
		{"missing_heating_port", func(t *testing.T, d *idf.Document) {
			object(t, d, energyPathRadiantConstantFlowType, "Radiant").Fields[10].Value = ""
		}},
		{"legacy_inline_schema", func(t *testing.T, d *idf.Document) {
			parent := object(t, d, energyPathRadiantConstantFlowType, "Radiant")
			parent.Fields = append(parent.Fields[:1], parent.Fields[2:]...)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc := energyPathRadiantHandDocument(t)
			test.mutate(t, &doc)
			if got := energyPathRadiantLoadTargets(doc); len(got) != 0 {
				t.Fatalf("ambiguous original ownership became output targets: %#v", got)
			}
			// Selecting only the apparently valid owner must not erase the
			// contradictory original references during output planning.
			plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath,
				Scope: SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"Office"}}})
			for _, output := range plan.OutputObjects {
				if energyPathIsRadiantLoadVariable(output.VariableName) && output.ReportingFrequency == "Monthly" && strings.EqualFold(output.KeyValue, "Radiant") {
					t.Fatalf("scope laundered invalid equipment ownership: %#v", output)
				}
			}
		})
	}
}

func TestEnergyPathRadiantSelectedOwnerAndOriginalTimestepPreserved(t *testing.T) {
	doc := energyPathRadiantHandDocument(t)
	for _, zone := range []string{"office", "Other"} {
		t.Run(zone, func(t *testing.T) {
			plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath,
				Scope: SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{zone}}})
			count := 0
			for _, output := range plan.OutputObjects {
				if output.ObjectType == "Output:Variable" && output.State != PurposeOutputStateExisting &&
					(strings.Contains(strings.ToLower(output.VariableName), "pump electricity") || strings.Contains(strings.ToLower(output.VariableName), "fan electricity") || output.ReportingFrequency != "Monthly") {
					t.Fatalf("load-key correction added a heavy output: %#v", output)
				}
				if !energyPathIsRadiantLoadVariable(output.VariableName) || output.ReportingFrequency != "Monthly" {
					continue
				}
				count++
				if output.KeyValue != "Radiant" || output.ScopeZoneName != "Office" || zone != "office" {
					t.Fatalf("selected scope invented an owner: %#v", output)
				}
			}
			want := 0
			if zone == "office" {
				want = 4
			}
			if count != want {
				t.Fatalf("selected %s radiant requests=%d, want %d", zone, count, want)
			}
			updated, _ := idf.ApplyOutput(doc, PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeKeepExistingAdd))
			timestep := 0
			for _, object := range updated.Objects {
				if object.Type == "Output:Variable" && len(object.Fields) >= 3 && object.Fields[0].Value == "*" && object.Fields[2].Value == "Timestep" {
					timestep++
				}
			}
			if timestep != 5 {
				t.Fatalf("original four load and one pump wildcard Timestep requests changed: %d", timestep)
			}
		})
	}
}

func TestEnergyPathRadiantLiteralAliasesAndNonNativeBoundary(t *testing.T) {
	for _, name := range []string{"Zone Radiant HVAC Cooling Energy", "Zone Radiant HVAC Cooling Rate", "Zone Radiant HVAC Heating Energy", "Zone Radiant HVAC Heating Rate"} {
		if !energyPathIsRadiantLoadVariable("  " + strings.ToUpper(name) + " ") {
			t.Fatalf("literal load alias not recognized: %s", name)
		}
	}
	for _, name := range []string{"Zone Radiant HVAC Pump Electricity Energy", "Zone Radiant HVAC Heating Fluid Heat Transfer Energy", "Zone Air System Sensible Heating Energy", "Zone Ideal Loads Supply Air Sensible Cooling Energy", "VRF Heat Pump Cooling Electricity Energy"} {
		if energyPathIsRadiantLoadVariable(name) {
			t.Fatalf("unreviewed/non-additive alias recognized: %s", name)
		}
	}
	for _, typ := range []string{"ZoneHVAC:LowTemperatureRadiant:VariableFlow", "ZoneHVAC:LowTemperatureRadiant:Electric", "ZoneHVAC:IdealLoadsAirSystem", "ZoneHVAC:TerminalUnit:VariableRefrigerantFlow"} {
		doc := energyPathRadiantHandDocument(t)
		directHVACFixtureObject(t, &doc, energyPathRadiantConstantFlowType, "Radiant").Type = typ
		if got := energyPathRadiantLoadTargets(doc); got != nil {
			t.Fatalf("unreviewed variant did not take no-native boundary: %s %#v", typ, got)
		}
	}
}

func TestEnergyPathRadiantNonNativeLegacyZoneRequestsUnchanged(t *testing.T) {
	doc := parsePurposePlanFixture(t, "Zone,Legacy Office,0,0,0,0,1,1;")
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	count := 0
	for _, output := range plan.OutputObjects {
		if !energyPathIsRadiantLoadVariable(output.VariableName) {
			continue
		}
		count++
		if output.KeyValue != "Legacy Office" || output.ScopeZoneName != "Legacy Office" || output.ReportingFrequency != "Monthly" {
			t.Fatalf("native correction changed the pre-existing non-radiant request contract: %#v", output)
		}
	}
	if count != 4 {
		t.Fatalf("legacy no-native radiant aliases=%d, want original four Zone requests", count)
	}
}
