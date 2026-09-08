package simulation

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func energyPathVRFDocument(t *testing.T) idf.Document {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "DOAToVRF.idf"))
	if err != nil {
		t.Fatal(err)
	}
	return parsePurposePlanFixture(t, string(data))
}

// Literal names and keys are from the original DOAToVRF input and its MTD:
// crankcase electricity belongs to Cooling, while defrost belongs to Heating.
func energyPathVRFExpectedRequests() map[string]string {
	out := map[string]string{}
	for zone := 1; zone <= 5; zone++ {
		for _, service := range []string{"Cooling", "Heating"} {
			out[fmt.Sprintf("TU%d|Zone VRF Air Terminal %s Electricity Energy", zone, service)] = fmt.Sprintf("SPACE%d-1", zone)
		}
	}
	for _, name := range []string{
		"VRF Heat Pump Cooling Electricity Energy", "VRF Heat Pump Crankcase Heater Electricity Energy",
		"VRF Heat Pump Heating Electricity Energy", "VRF Heat Pump Defrost Electricity Energy",
	} {
		out["VRF HEAT PUMP|"+name] = ""
	}
	return out
}

func TestEnergyPathVRFOriginalMonthlyRequests(t *testing.T) {
	doc := energyPathVRFDocument(t)
	before := doc.String()
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	expected := energyPathVRFExpectedRequests()
	seen := map[string]int{}
	for _, output := range plan.OutputObjects {
		key := strings.ToUpper(strings.TrimSpace(output.KeyValue)) + "|" + output.VariableName
		owner, wanted := expected[key]
		if !wanted {
			continue
		}
		if output.ObjectType != "Output:Variable" || output.ReportingFrequency != "Monthly" || output.ScopeZoneName != owner || !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) {
			t.Errorf("incorrect VRF request: %#v", output)
		}
		seen[key]++
	}
	if len(seen) != 14 {
		t.Fatalf("VRF exact Monthly consumption requests=%d, want14 (five local pairs plus four shared outdoor constituents)", len(seen))
	}
	for key := range expected {
		if seen[key] != 1 {
			t.Errorf("%s count=%d, want1", key, seen[key])
		}
	}
	if before != doc.String() {
		t.Fatal("output planning mutated the original VRF document")
	}
}

func TestEnergyPathVRFOriginalTypedOwnership(t *testing.T) {
	doc := energyPathVRFDocument(t)
	before := doc.String()
	systems := energyPathVRFSystems(doc)
	if len(systems) != 1 || len(systems[0].Terminals) != 5 || len(systems[0].Targets) != 14 {
		t.Fatalf("original VRF system/owners/constituents differ: %#v", systems)
	}
	system := systems[0]
	if system.OutdoorUnit.ObjectType != "AirConditioner:VariableRefrigerantFlow" || system.OutdoorUnit.ObjectName != "VRF Heat Pump" ||
		system.TerminalUnitList.ObjectType != "ZoneTerminalUnitList" || system.TerminalUnitList.ObjectName != "VRF Heat Pump TU List" {
		t.Fatalf("wrong original outdoor/list identity: %#v", system)
	}
	for i, terminal := range system.Terminals {
		number := i + 1
		if terminal.ZoneName != fmt.Sprintf("SPACE%d-1", number) || terminal.Terminal.ObjectName != fmt.Sprintf("TU%d", number) ||
			!strings.EqualFold(terminal.Terminal.ObjectType, "ZoneHVAC:TerminalUnit:VariableRefrigerantFlow") ||
			terminal.CoolingCoil.ObjectName != fmt.Sprintf("TU%d VRF DX Cooling Coil", number) || !strings.EqualFold(terminal.CoolingCoil.ObjectType, "Coil:Cooling:DX:VariableRefrigerantFlow") ||
			terminal.HeatingCoil.ObjectName != fmt.Sprintf("TU%d VRF DX Heating Coil", number) || !strings.EqualFold(terminal.HeatingCoil.ObjectType, "Coil:Heating:DX:VariableRefrigerantFlow") ||
			terminal.Fan.ObjectName != fmt.Sprintf("TU%d VRF Supply Fan", number) || terminal.Fan.ObjectType != "Fan:ConstantVolume" {
			t.Fatalf("wrong typed owner: %#v", terminal)
		}
		for _, ref := range []idf.ComponentRef{terminal.Terminal, terminal.CoolingCoil, terminal.HeatingCoil, terminal.Fan} {
			object := directHVACFixtureObject(t, &doc, ref.ObjectType, ref.ObjectName)
			if ref.ObjectIndex != object.Index || ref.ID != fmt.Sprintf("component:%d", object.Index) || ref.InletNode == "" || ref.OutletNode == "" {
				t.Fatalf("unbound original component: %#v", ref)
			}
		}
	}
	expected := energyPathVRFExpectedRequests()
	for _, target := range system.Targets {
		if len(target.Definition.Energy.Aliases) != 1 || target.Definition.Energy.Carrier != "electricity" || target.Definition.Energy.FacilityTotal {
			t.Fatalf("incorrect constituent definition %#v", target)
		}
		name := target.Definition.Energy.Aliases[0]
		key := strings.ToUpper(target.KeyValue) + "|" + name
		owner, ok := expected[key]
		if !ok || owner != target.ZoneName || target.Definition.Shared != (owner == "") {
			t.Fatalf("missing literal source/owner binding %#v", target)
		}
		service := "heating"
		if strings.Contains(name, "Cooling") || strings.Contains(name, "Crankcase") {
			service = "cooling"
		}
		if target.Definition.Energy.EndUse != service || !strings.HasPrefix(target.Definition.ID, service+".vrf.") {
			t.Fatalf("MTD carrier role changed %#v", target)
		}
		object := directHVACFixtureObject(t, &doc, target.Definition.ObjectType, target.KeyValue)
		if target.ObjectIndex != object.Index {
			t.Fatalf("target index not original object: %#v", target)
		}
	}
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	if !reflect.DeepEqual(context.VRFSystems, systems) || len(context.DirectHVACComponents) != 0 || before != doc.String() {
		t.Fatal("VRF context changed owners/input or leaked into PTAC/PTHP component cohort")
	}
}

func TestEnergyPathVRFSelectedScopePreservesWholePoolAndMinimalLoads(t *testing.T) {
	doc := energyPathVRFDocument(t)
	for _, mode := range []string{"selected", "visible", "filtered"} {
		t.Run(mode, func(t *testing.T) {
			plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath,
				Scope: SimulationPurposeScope{ZoneMode: mode, ZoneNames: []string{"space1-1"}}})
			seen, loads := map[string]int{}, map[string]int{}
			for _, output := range plan.OutputObjects {
				key := strings.ToUpper(output.KeyValue) + "|" + output.VariableName
				if owner, ok := energyPathVRFExpectedRequests()[key]; ok {
					seen[key]++
					if output.ReportingFrequency != "Monthly" || output.ScopeZoneName != owner {
						t.Fatalf("scope rewrote original owner: %#v", output)
					}
				}
				if output.VariableName == "Zone Air System Sensible Cooling Energy" || output.VariableName == "Zone Air System Sensible Heating Energy" {
					loads[key]++
					if output.ReportingFrequency != "Monthly" || output.ScopeZoneName != output.KeyValue {
						t.Fatalf("incorrect denominator context: %#v", output)
					}
				}
				if output.ObjectType == "Output:Variable" && output.State != PurposeOutputStateExisting &&
					(output.ReportingFrequency != "Monthly" || strings.Contains(strings.ToLower(output.VariableName), "fan electricity") || strings.Contains(strings.ToLower(output.VariableName), "pump electricity")) {
					t.Fatalf("added heavy/high-frequency output %#v", output)
				}
			}
			if len(seen) != 14 || len(loads) != 10 {
				t.Fatalf("selected scope shrank cohort/denominator: %d/%d", len(seen), len(loads))
			}
			for key, count := range seen {
				if count != 1 {
					t.Errorf("duplicate constituent %s: %d", key, count)
				}
			}
			for zone := 1; zone <= 5; zone++ {
				for _, service := range []string{"Cooling", "Heating"} {
					key := fmt.Sprintf("SPACE%d-1|Zone Air System Sensible %s Energy", zone, service)
					if loads[key] != 1 {
						t.Errorf("incomplete exact denominator %s: %d", key, loads[key])
					}
				}
			}
		})
	}
	for _, zone := range []string{"PLENUM-1", "Unknown Zone", ""} {
		plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath,
			Scope: SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{zone}}})
		for _, output := range plan.OutputObjects {
			if _, ok := energyPathVRFConsumptionDefinitionForName(output.VariableName); ok {
				t.Fatalf("unrelated scope %q acquired pool: %#v", zone, output)
			}
		}
	}
}

func energyPathVRFCloneObject(doc *idf.Document, original idf.Object, name string) {
	copy := original
	copy.Fields = append([]idf.Field(nil), original.Fields...)
	copy.Index = len(doc.Objects)
	if name != "" {
		copy.Fields[0].Value = name
	}
	doc.Objects = append(doc.Objects, copy)
}

func TestEnergyPathVRFInvalidWholeOwnershipCannotShrinkBeforeScope(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, *idf.Document)
	}{
		{"duplicate outdoor", func(t *testing.T, d *idf.Document) {
			energyPathVRFCloneObject(d, *directHVACFixtureObject(t, d, energyPathVRFOutdoorType, "VRF Heat Pump"), "")
		}},
		{"two outdoor owners", func(t *testing.T, d *idf.Document) {
			energyPathVRFCloneObject(d, *directHVACFixtureObject(t, d, energyPathVRFOutdoorType, "VRF Heat Pump"), "Other Outdoor")
		}},
		{"duplicate list", func(t *testing.T, d *idf.Document) {
			energyPathVRFCloneObject(d, *directHVACFixtureObject(t, d, "ZoneTerminalUnitList", "VRF Heat Pump TU List"), "")
		}},
		{"missing list", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathVRFOutdoorType, "VRF Heat Pump").Fields[36].Value = "Missing"
		}},
		{"duplicate member", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "ZoneTerminalUnitList", "VRF Heat Pump TU List").Fields[1].Value = "TU4"
		}},
		{"foreign member", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "ZoneTerminalUnitList", "VRF Heat Pump TU List").Fields[1].Value = "Not A TU"
		}},
		{"unowned second list", func(t *testing.T, d *idf.Document) {
			energyPathVRFCloneObject(d, *directHVACFixtureObject(t, d, "ZoneTerminalUnitList", "VRF Heat Pump TU List"), "Unconnected List")
		}},
		{"duplicate unselected terminal", func(t *testing.T, d *idf.Document) {
			energyPathVRFCloneObject(d, *directHVACFixtureObject(t, d, energyPathVRFTerminalType, "TU5"), "")
		}},
		{"missing unselected coil", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathVRFTerminalType, "TU5").Fields[18].Value = "Missing Coil"
		}},
		{"shared coil", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathVRFTerminalType, "TU5").Fields[18].Value = "TU1 VRF DX Cooling Coil"
		}},
		{"duplicate coil", func(t *testing.T, d *idf.Document) {
			energyPathVRFCloneObject(d, *directHVACFixtureObject(t, d, energyPathVRFCoolingType, "TU5 VRF DX Cooling Coil"), "")
		}},
		{"foreign typed parent", func(t *testing.T, d *idf.Document) {
			extra := parsePurposePlanFixture(t, "Branch, Foreign, , Coil:Cooling:DX:VariableRefrigerantFlow,TU5 VRF DX Cooling Coil,A,B;")
			d.Objects = append(d.Objects, extra.Objects...)
		}},
		{"broken coil node", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathVRFHeatingType, "TU5 VRF DX Heating Coil").Fields[4].Value = "Foreign Node"
		}},
		{"wrong coil type", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathVRFTerminalType, "TU5").Fields[17].Value = "Coil:Cooling:DX:SingleSpeed"
		}},
		{"duplicate Zone owner", func(t *testing.T, d *idf.Document) {
			energyPathVRFCloneObject(d, *directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentConnections", "SPACE5-1"), "SPACE1-1")
		}},
		{"duplicate equipment list", func(t *testing.T, d *idf.Document) {
			energyPathVRFCloneObject(d, *directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentList", "SPACE5-1 Eq"), "")
		}},
		{"shifted terminal reference", func(t *testing.T, d *idf.Document) {
			list := directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentList", "SPACE5-1 Eq")
			list.Fields[12].Value, list.Fields[13].Value = list.Fields[8].Value, list.Fields[9].Value
			list.Fields[8].Value, list.Fields[9].Value = "", ""
		}},
		{"shifted ADU reference", func(t *testing.T, d *idf.Document) {
			list := directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentList", "SPACE5-1 Eq")
			list.Fields[6].Value, list.Fields[7].Value = list.Fields[2].Value, list.Fields[3].Value
			list.Fields[2].Value, list.Fields[3].Value = "", ""
		}},
		{"foreign mixer unit", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "AirTerminal:SingleDuct:Mixer", "SPACE5-1 DOAS Air Terminal").Fields[2].Value = "TU1"
		}},
		{"broken inlet mixer", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "AirTerminal:SingleDuct:Mixer", "SPACE1-1 DOAS Air Terminal").Fields[3].Value = "Other Node"
		}},
		{"broken supply mixer", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "AirTerminal:SingleDuct:Mixer", "SPACE5-1 DOAS Air Terminal").Fields[5].Value = "Other Node"
		}},
		{"foreign ADU", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "ZoneHVAC:AirDistributionUnit", "SPACE5-1 DOAS ATU").Fields[3].Value = "SPACE1-1 DOAS Air Terminal"
		}},
		{"wrong Zone inlet", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentConnections", "SPACE5-1").Fields[2].Value = "TU1 Outlet Node"
		}},
		{"supplemental heater", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathVRFTerminalType, "TU5").Fields[26].Value = "Coil:Heating:Electric"
		}},
		{"other fan variant", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathVRFTerminalType, "TU5").Fields[13].Value = "Fan:OnOff"
		}},
		{"other placement", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathVRFTerminalType, "TU5").Fields[12].Value = "BlowThrough"
		}},
		{"unknown parasitic", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathVRFTerminalType, "TU5").Fields[21].Value = "NaN"
		}},
		{"non-electric outdoor", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathVRFOutdoorType, "VRF Heat Pump").Fields[66].Value = "NaturalGas"
		}},
		{"heat recovery variant", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathVRFOutdoorType, "VRF Heat Pump").Fields[37].Value = "Yes"
		}},
		{"water cooled variant", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathVRFOutdoorType, "VRF Heat Pump").Fields[55].Value = "WaterCooled"
		}},
		{"unsupported outdoor same key", func(t *testing.T, d *idf.Document) {
			item := *directHVACFixtureObject(t, d, energyPathVRFOutdoorType, "VRF Heat Pump")
			item.Type += ":FluidTemperatureControl"
			energyPathVRFCloneObject(d, item, "")
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			doc := energyPathVRFDocument(t)
			test.mutate(t, &doc)
			if got := energyPathVRFSystems(doc); len(got) != 0 {
				t.Fatalf("invalid original retained %d apparently valid VRF pools", len(got))
			}
			plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath,
				Scope: SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"SPACE1-1"}}})
			for _, output := range plan.OutputObjects {
				if _, ok := energyPathVRFConsumptionDefinitionForName(output.VariableName); ok {
					t.Fatalf("selected scope laundered invalid whole ownership: %#v", output)
				}
			}
		})
	}
}

func TestEnergyPathVRFPreservesManualOutputsAndOtherNativeContracts(t *testing.T) {
	doc := energyPathVRFDocument(t)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	request := PurposeRunPlanApplyRequest(plan)
	if len(request.Updates) != 0 || len(request.RemoveObjectIndexes) != 0 {
		t.Fatal("VRF additions rewrite/remove original output requests")
	}
	applied, _ := idf.ApplyOutput(doc, request)
	manualBefore, manualAfter := []string{}, []string{}
	for pass, document := range []idf.Document{doc, applied} {
		for _, object := range document.Objects {
			if !strings.EqualFold(object.Type, "Output:Variable") || len(object.Fields) < 3 || !strings.EqualFold(object.Fields[2].Value, "timestep") {
				continue
			}
			name := object.Fields[1].Value
			if strings.Contains(name, "VRF") || name == "Fan Electricity Rate" {
				key := object.Fields[0].Value + "|" + name + "|" + object.Fields[2].Value
				if pass == 0 {
					manualBefore = append(manualBefore, key)
				} else {
					manualAfter = append(manualAfter, key)
				}
			}
		}
	}
	if len(manualBefore) < 7 || !reflect.DeepEqual(manualBefore, manualAfter) {
		t.Fatalf("original timestep/known TU1 fan W requests changed: %v / %v", manualBefore, manualAfter)
	}
	wantFan := false
	for _, key := range manualAfter {
		wantFan = wantFan || strings.EqualFold(key, "TU1 VRF SUPPLY FAN|Fan Electricity Rate|timestep")
	}
	if !wantFan {
		t.Fatal("lost actual manually requested TU1 fan W")
	}
	if got := energyPathDirectHVACComponentTargets(directHVACComponentFixture(t)); len(got) != 25 {
		t.Fatalf("PTAC direct cohort=%d", len(got))
	}
	if got := energyPathDirectHVACComponentTargets(pthpDirectDocument(t)); len(got) != 35 {
		t.Fatalf("PTHP direct cohort=%d", len(got))
	}
	for _, fixture := range []idf.Document{{}, directHVACComponentFixture(t), pthpDirectDocument(t)} {
		if energyPathVRFSystems(fixture) != nil {
			t.Fatal("non-VRF native fast path is not nil")
		}
	}
	for _, name := range []string{"Fan Electricity Energy", "Fan Electricity Rate", "VRF Heat Pump Cooling Electricity Rate", "Cooling Coil Total Cooling Energy", "Unknown VRF Heat Pump Cooling Electricity Energy"} {
		if _, ok := energyPathVRFConsumptionDefinitionForName(name); ok {
			t.Fatalf("unreviewed alias accepted %q", name)
		}
	}
	definition, ok := energyPathVRFConsumptionDefinitionForName(" vrf heat pump crankcase heater electricity energy ")
	if !ok {
		t.Fatal("exact case-fold lookup failed")
	}
	definition.Energy.Aliases[0] = "changed"
	fresh, _ := energyPathVRFConsumptionDefinitionForName("VRF Heat Pump Crankcase Heater Electricity Energy")
	if fresh.Energy.Aliases[0] == "changed" {
		t.Fatal("mutable alias catalog escaped")
	}
}
