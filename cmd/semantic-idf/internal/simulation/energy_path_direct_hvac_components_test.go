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

func directHVACComponentFixture(t *testing.T) idf.Document {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "DOAToPTAC.idf"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := idf.Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func directHVACFixtureObject(t *testing.T, doc *idf.Document, objectType, name string) *idf.Object {
	t.Helper()
	for index := range doc.Objects {
		object := &doc.Objects[index]
		if strings.EqualFold(object.Type, objectType) && len(object.Fields) > 0 && strings.EqualFold(object.Fields[0].Value, name) {
			return object
		}
	}
	t.Fatalf("missing fixture object %s/%s", objectType, name)
	return nil
}

func TestEnergyPathDirectHVACComponentsOriginalPTAC(t *testing.T) {
	doc := directHVACComponentFixture(t)
	before := doc.String()
	targets := energyPathDirectHVACComponentTargets(doc)
	if len(targets) != 25 {
		t.Fatalf("targets=%d, want five independently metered constituents for each of five native PTACs: %#v", len(targets), targets)
	}
	expected := map[string]string{
		"cooling.coil.electricity":           "Cooling Coil Electricity Energy",
		"cooling.coil.crankcase_electricity": "Cooling Coil Crankcase Heater Electricity Energy",
		"heating.coil.natural_gas":           "Heating Coil NaturalGas Energy",
		"heating.coil.ancillary_natural_gas": "Heating Coil Ancillary NaturalGas Energy",
		"heating.coil.electricity":           "Heating Coil Electricity Energy",
	}
	seen := map[string]bool{}
	for _, target := range targets {
		name, ok := expected[target.Definition.ID]
		if !ok || !reflect.DeepEqual(target.Definition.Energy.Aliases, []string{name}) {
			t.Fatalf("unexpected constituent %#v", target)
		}
		if target.Definition.Energy.HierarchyLevel != "zone_direct_use" || target.Definition.Energy.FacilityTotal {
			t.Fatalf("incorrect direct scope %#v", target)
		}
		key := target.ZoneName + "/" + target.Definition.ID
		if seen[key] {
			t.Fatalf("duplicate constituent %s", key)
		}
		seen[key] = true
		if strings.HasPrefix(target.Definition.ID, "cooling.") {
			if target.KeyValue != target.ZoneName+" PTAC CCoil" || target.Definition.ObjectType != "Coil:Cooling:DX:SingleSpeed" || target.Definition.Energy.Carrier != "electricity" {
				t.Fatalf("wrong cooling owner %#v", target)
			}
		} else if target.KeyValue != target.ZoneName+" Heating Coil" || target.Definition.ObjectType != "Coil:Heating:Fuel" {
			t.Fatalf("wrong heating owner %#v", target)
		}
	}
	for zone := 1; zone <= 5; zone++ {
		for id := range expected {
			if !seen[fmt.Sprintf("SPACE%d-1/%s", zone, id)] {
				t.Fatalf("missing Zone%d constituent %s", zone, id)
			}
		}
	}
	if !reflect.DeepEqual(targets, energyPathDirectHVACComponentTargets(doc)) || before != doc.String() {
		t.Fatal("ownership is nondeterministic or changed original input")
	}
}

func TestEnergyPathDirectHVACComponentsFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *idf.Document)
	}{
		{"missing coil", func(t *testing.T, doc *idf.Document) {
			directHVACFixtureObject(t, doc, "Coil:Cooling:DX:SingleSpeed", "SPACE1-1 PTAC CCoil").Fields[0].Value = "Detached CCoil"
		}},
		{"missing fan", func(t *testing.T, doc *idf.Document) {
			directHVACFixtureObject(t, doc, "Fan:OnOff", "SPACE1-1 Supply Fan").Fields[0].Value = "Detached Fan"
		}},
		{"missing equipment list", func(t *testing.T, doc *idf.Document) {
			directHVACFixtureObject(t, doc, "ZoneHVAC:EquipmentConnections", "SPACE1-1").Fields[1].Value = "Missing List"
		}},
		{"missing Zone", func(t *testing.T, doc *idf.Document) {
			directHVACFixtureObject(t, doc, "Zone", "SPACE1-1").Fields[0].Value = "Not The Owner"
		}},
		{"wrong fuel", func(t *testing.T, doc *idf.Document) {
			directHVACFixtureObject(t, doc, "Coil:Heating:Fuel", "SPACE1-1 Heating Coil").Fields[2].Value = "Propane"
		}},
		{"duplicate coil", func(t *testing.T, doc *idf.Document) {
			item := *directHVACFixtureObject(t, doc, "Coil:Cooling:DX:SingleSpeed", "SPACE1-1 PTAC CCoil")
			item.Index = len(doc.Objects)
			doc.Objects = append(doc.Objects, item)
		}},
		{"duplicate parent", func(t *testing.T, doc *idf.Document) {
			item := *directHVACFixtureObject(t, doc, "ZoneHVAC:PackagedTerminalAirConditioner", "SPACE1-1 PTAC")
			item.Index = len(doc.Objects)
			doc.Objects = append(doc.Objects, item)
		}},
		{"duplicate connection", func(t *testing.T, doc *idf.Document) {
			item := *directHVACFixtureObject(t, doc, "ZoneHVAC:EquipmentConnections", "SPACE1-1")
			item.Index = len(doc.Objects)
			doc.Objects = append(doc.Objects, item)
		}},
		{"duplicate list", func(t *testing.T, doc *idf.Document) {
			connection := directHVACFixtureObject(t, doc, "ZoneHVAC:EquipmentConnections", "SPACE1-1")
			item := *directHVACFixtureObject(t, doc, "ZoneHVAC:EquipmentList", connection.Fields[1].Value)
			item.Index = len(doc.Objects)
			doc.Objects = append(doc.Objects, item)
		}},
		{"duplicate mixer", func(t *testing.T, doc *idf.Document) {
			item := *directHVACFixtureObject(t, doc, "AirTerminal:SingleDuct:Mixer", "SPACE1-1 DOAS Air Terminal")
			item.Index = len(doc.Objects)
			doc.Objects = append(doc.Objects, item)
		}},
		{"mixer wrong node", func(t *testing.T, doc *idf.Document) {
			directHVACFixtureObject(t, doc, "AirTerminal:SingleDuct:Mixer", "SPACE1-1 DOAS Air Terminal").Fields[3].Value = "Another PTAC inlet"
		}},
		{"mixer blank nodes", func(t *testing.T, doc *idf.Document) {
			directHVACFixtureObject(t, doc, "AirTerminal:SingleDuct:Mixer", "SPACE1-1 DOAS Air Terminal").Fields[3].Value = ""
			directHVACFixtureObject(t, doc, "ZoneHVAC:PackagedTerminalAirConditioner", "SPACE1-1 PTAC").Fields[2].Value = ""
		}},
		{"shared coil outside selected Zone", func(t *testing.T, doc *idf.Document) {
			directHVACFixtureObject(t, doc, "ZoneHVAC:PackagedTerminalAirConditioner", "SPACE2-1 PTAC").Fields[18].Value = "SPACE1-1 PTAC CCoil"
		}},
		{"shared list", func(t *testing.T, doc *idf.Document) {
			first := directHVACFixtureObject(t, doc, "ZoneHVAC:EquipmentConnections", "SPACE1-1")
			directHVACFixtureObject(t, doc, "ZoneHVAC:EquipmentConnections", "SPACE2-1").Fields[1].Value = first.Fields[1].Value
		}},
		{"central or disconnected reference", func(t *testing.T, doc *idf.Document) {
			doc.Objects = append(doc.Objects, idf.Object{Index: len(doc.Objects), Type: "Branch", Fields: []idf.Field{{Value: "Central supply"}, {}, {Value: "Coil:Cooling:DX:SingleSpeed"}, {Value: "SPACE1-1 PTAC CCoil"}, {Value: "Central in"}, {Value: "Central out"}}})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := directHVACComponentFixture(t)
			test.mutate(t, &doc)
			for _, target := range energyPathDirectHVACComponentTargets(doc) {
				if strings.EqualFold(target.ZoneName, "SPACE1-1") {
					t.Fatalf("ambiguous/disconnected component escaped ownership guard: %#v", target)
				}
			}
			// Selecting only the first Zone must not hide the second owner.
			plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath,
				Scope: SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"SPACE1-1"}}})
			for _, output := range plan.OutputObjects {
				if _, ok := energyPathDirectHVACComponentDefinitionForName(output.VariableName); ok {
					t.Fatalf("scoped plan restored rejected owner: %#v", output)
				}
			}
		})
	}
}

func TestEnergyPathDirectHVACComponentsRequestsAndNames(t *testing.T) {
	doc := directHVACComponentFixture(t)
	for _, test := range []struct {
		name, detail string
		scope        SimulationPurposeScope
		want         int
	}{
		{"all", PurposeBasicEnergyDetailEnergyPath, SimulationPurposeScope{}, 25},
		{"selected", PurposeBasicEnergyDetailEnergyPath, SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"space1-1"}}, 5},
		{"unserved", PurposeBasicEnergyDetailEnergyPath, SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"PLENUM-1"}}, 0},
		{"legacy detail", PurposeBasicEnergyDetailHeatDrivers, SimulationPurposeScope{}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: test.detail, Scope: test.scope})
			count := 0
			for _, output := range plan.OutputObjects {
				if _, ok := energyPathDirectHVACComponentDefinitionForName(output.VariableName); !ok {
					continue
				}
				count++
				if output.ReportingFrequency != "Monthly" || output.ScopeZoneName == "" || output.KeyValue == "*" || output.KeyValue == output.ScopeZoneName {
					t.Fatalf("incorrect request binding %#v", output)
				}
				if test.name == "selected" && output.ScopeZoneName != "SPACE1-1" {
					t.Fatalf("out-of-scope request %#v", output)
				}
			}
			if count != test.want {
				t.Fatalf("requests=%d want %d", count, test.want)
			}
		})
	}
	for _, name := range []string{"Zone Packaged Terminal Air Conditioner Electricity Energy", "Fan Electricity Energy", "Heating Coil NaturalGas Rate", "Cooling Coil Total Cooling Energy", "Unknown Cooling Coil Electricity Energy"} {
		if _, ok := energyPathDirectHVACComponentDefinitionForName(name); ok {
			t.Fatalf("unreviewed alias accepted %q", name)
		}
	}
	definition, ok := energyPathDirectHVACComponentDefinitionForName(" cooling coil electricity energy ")
	if !ok {
		t.Fatal("case-insensitive exact name rejected")
	}
	definition.Energy.Aliases[0] = "mutated"
	fresh, ok := energyPathDirectHVACComponentDefinitionForName("Cooling Coil Electricity Energy")
	if !ok || fresh.Energy.Aliases[0] != "Cooling Coil Electricity Energy" {
		t.Fatal("lookup returned shared mutable aliases")
	}
	if got := energyPathDirectHVACComponentTargets(idf.Document{}); got != nil {
		t.Fatalf("non-PTAC fast path=%#v", got)
	}
}

func TestEnergyPathDirectHVACComponentsCaseInsensitiveOwnership(t *testing.T) {
	doc := directHVACComponentFixture(t)
	// Object names and typed references are case-insensitive in EnergyPlus;
	// matching must not depend on the human-readable Zone prefix in coil keys.
	for index := range doc.Objects {
		object := &doc.Objects[index]
		for field := range object.Fields {
			if strings.EqualFold(object.Fields[field].Value, "SPACE1-1 PTAC CCoil") {
				object.Fields[field].Value = "  Unrelated coil label  "
			}
		}
	}
	coil := directHVACFixtureObject(t, &doc, "Coil:Cooling:DX:SingleSpeed", "  Unrelated coil label  ")
	coil.Fields[0].Value = "UNRELATED COIL LABEL"
	targets := energyPathDirectHVACComponentTargets(doc)
	if len(targets) != 25 {
		t.Fatalf("case-only reference changed ownership: %d targets", len(targets))
	}
	count := 0
	for _, target := range targets {
		if target.ZoneName == "SPACE1-1" && target.Definition.Energy.EndUse == "cooling" {
			count++
			if !strings.EqualFold(strings.TrimSpace(target.KeyValue), "Unrelated coil label") {
				t.Fatalf("coil key was inferred from Zone name: %#v", target)
			}
		}
	}
	if count != 2 {
		t.Fatalf("renamed cooling coil constituent count=%d", count)
	}
}
