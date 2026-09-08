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

// This literal roster comes from the reviewed DOAToPTHP component types and
// reported consumption roles, not from the production definition catalog.
var pthpDirectRoles = []struct{ id, objectType, suffix, name, service, carrier string }{
	{"cooling.coil.electricity", "Coil:Cooling:DX:SingleSpeed", " HP Cooling Mode", "Cooling Coil Electricity Energy", "cooling", "electricity"},
	{"heating.coil.dx_electricity", "Coil:Heating:DX:SingleSpeed", " HP Heating Mode", "Heating Coil Electricity Energy", "heating", "electricity"},
	{"heating.coil.defrost_electricity", "Coil:Heating:DX:SingleSpeed", " HP Heating Mode", "Heating Coil Defrost Electricity Energy", "heating", "electricity"},
	{"heating.coil.crankcase_electricity", "Coil:Heating:DX:SingleSpeed", " HP Heating Mode", "Heating Coil Crankcase Heater Electricity Energy", "heating", "electricity"},
	{"heating.coil.natural_gas", "Coil:Heating:Fuel", " HP Supp Coil", "Heating Coil NaturalGas Energy", "heating", "natural_gas"},
	{"heating.coil.ancillary_natural_gas", "Coil:Heating:Fuel", " HP Supp Coil", "Heating Coil Ancillary NaturalGas Energy", "heating", "natural_gas"},
	{"heating.coil.electricity", "Coil:Heating:Fuel", " HP Supp Coil", "Heating Coil Electricity Energy", "heating", "electricity"},
}

func pthpDirectDocument(t *testing.T) idf.Document {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "DOAToPTHP.idf"))
	if err != nil {
		t.Fatal(err)
	}
	return parsePurposePlanFixture(t, string(data))
}

func pthpDirectPlan(doc idf.Document, scope SimulationPurposeScope) PurposeRunPlan {
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, Scope: scope})
	plan.AllocationPolicy = PurposeAllocationPolicyDirectOnly
	return plan
}

func pthpDirectName(name string) bool {
	for _, role := range pthpDirectRoles {
		if strings.EqualFold(strings.TrimSpace(name), role.name) {
			return true
		}
	}
	return false
}

func TestEnergyPathDirectHVACPTHPOriginalOwnershipAndRequests(t *testing.T) {
	doc := pthpDirectDocument(t)
	before := doc.String()
	zones, connections := 0, 0
	for _, object := range doc.Objects {
		switch strings.ToLower(object.Type) {
		case "zone":
			zones++
			if len(object.Fields) < 7 || strings.TrimSpace(object.Fields[6].Value) != "1" {
				t.Fatalf("original Zone multiplier is not the reviewed literal one: %#v", object.Fields)
			}
		case "zonelist", "zonegroup":
			t.Fatalf("unexpected original multiplier/list object %s", object.Type)
		case "zonehvac:equipmentconnections":
			connections++
			if strings.EqualFold(object.Fields[0].Value, "PLENUM-1") {
				t.Fatal("original PLENUM has equipment ownership")
			}
		}
	}
	if zones != 6 || connections != 5 {
		t.Fatalf("original topology is %d Zones/%d owners, want6/5", zones, connections)
	}
	targets := energyPathDirectHVACComponentTargets(doc)
	if len(targets) != 35 {
		t.Fatalf("PTHP targets=%d want35 (not PTAC's crankcase cohort)", len(targets))
	}
	for zone := 1; zone <= 5; zone++ {
		owner := fmt.Sprintf("SPACE%d-1", zone)
		for _, role := range pthpDirectRoles {
			count := 0
			for _, target := range targets {
				if target.ZoneName != owner || target.Definition.ID != role.id {
					continue
				}
				count++
				definition := target.Definition
				if target.KeyValue != owner+role.suffix || definition.ObjectType != role.objectType ||
					!reflect.DeepEqual(definition.Energy.Aliases, []string{role.name}) || definition.Energy.EndUse != role.service ||
					definition.Energy.Carrier != role.carrier || definition.Energy.HierarchyLevel != "zone_direct_use" || definition.Energy.FacilityTotal {
					t.Errorf("incorrect typed owner/role: %#v", target)
				}
			}
			if count != 1 {
				t.Errorf("%s/%s count%d want1", owner, role.id, count)
			}
		}
	}
	for _, test := range []struct {
		name  string
		scope SimulationPurposeScope
		want  int
	}{
		{"all", SimulationPurposeScope{}, 35},
		{"selected", SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"space1-1"}}, 7},
		{"plenum", SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"PLENUM-1"}}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan := pthpDirectPlan(doc, test.scope)
			seen := map[string]bool{}
			for _, output := range plan.OutputObjects {
				if !pthpDirectName(output.VariableName) {
					continue
				}
				key := output.KeyValue + "\x00" + output.VariableName
				if seen[key] || output.ObjectType != "Output:Variable" || output.ReportingFrequency != "Monthly" || output.ScopeZoneName == "" ||
					output.KeyValue == "*" || output.KeyValue == output.ScopeZoneName || !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) ||
					test.name == "selected" && output.ScopeZoneName != "SPACE1-1" {
					t.Fatalf("ambiguous/unscoped request %#v", output)
				}
				seen[key] = true
			}
			if len(seen) != test.want {
				t.Fatalf("exact requests%d want%d", len(seen), test.want)
			}
		})
	}
	if before != doc.String() || !reflect.DeepEqual(targets, energyPathDirectHVACComponentTargets(doc)) {
		t.Fatal("ownership/request generation mutated original document or is unstable")
	}
}

func TestEnergyPathDirectHVACPTHPInvalidOwnershipCannotBeScopedDirect(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *idf.Document)
	}{
		{"shared DX heating coil", func(t *testing.T, doc *idf.Document) {
			parent := directHVACFixtureObject(t, doc, "ZoneHVAC:PackagedTerminalHeatPump", "SPACE2-1 Heat Pump")
			changed := false
			for i := range parent.Fields {
				if parent.Fields[i].Value == "SPACE2-1 HP Heating Mode" {
					parent.Fields[i].Value = "SPACE1-1 HP Heating Mode"
					changed = true
				}
			}
			if !changed {
				t.Fatal("missing original typed heating reference")
			}
		}},
		{"wrong DX object type", func(t *testing.T, doc *idf.Document) {
			directHVACFixtureObject(t, doc, "Coil:Heating:DX:SingleSpeed", "SPACE1-1 HP Heating Mode").Type = "Coil:Heating:Fuel"
		}},
		{"duplicate DX object", func(t *testing.T, doc *idf.Document) {
			object := *directHVACFixtureObject(t, doc, "Coil:Heating:DX:SingleSpeed", "SPACE1-1 HP Heating Mode")
			object.Index = len(doc.Objects)
			doc.Objects = append(doc.Objects, object)
		}},
		{"DX and Fuel share indistinguishable SQL name and key", func(t *testing.T, doc *idf.Document) {
			for i := range doc.Objects {
				for j := range doc.Objects[i].Fields {
					if doc.Objects[i].Fields[j].Value == "SPACE1-1 HP Supp Coil" {
						doc.Objects[i].Fields[j].Value = "SPACE1-1 HP Heating Mode"
					}
				}
			}
		}},
		{"disconnected Fuel shares DX output key", func(t *testing.T, doc *idf.Document) {
			object := *directHVACFixtureObject(t, doc, "Coil:Heating:Fuel", "SPACE1-1 HP Supp Coil")
			object.Index = len(doc.Objects)
			object.Fields = append([]idf.Field(nil), object.Fields...)
			object.Fields[0].Value = "SPACE1-1 HP Heating Mode"
			doc.Objects = append(doc.Objects, object)
		}},
		{"missing supplemental coil", func(t *testing.T, doc *idf.Document) {
			directHVACFixtureObject(t, doc, "Coil:Heating:Fuel", "SPACE1-1 HP Supp Coil").Fields[0].Value = "Detached supplemental coil"
		}},
		{"wrong supplemental fuel", func(t *testing.T, doc *idf.Document) {
			directHVACFixtureObject(t, doc, "Coil:Heating:Fuel", "SPACE1-1 HP Supp Coil").Fields[2].Value = "Propane"
		}},
		{"duplicate equipment owner", func(t *testing.T, doc *idf.Document) {
			object := *directHVACFixtureObject(t, doc, "ZoneHVAC:EquipmentConnections", "SPACE1-1")
			object.Index = len(doc.Objects)
			doc.Objects = append(doc.Objects, object)
		}},
		{"disconnected wrapper reference", func(t *testing.T, doc *idf.Document) {
			doc.Objects = append(doc.Objects, idf.Object{Index: len(doc.Objects), Type: "Branch", Fields: []idf.Field{{Value: "Other supply"}, {}, {Value: "Coil:Heating:DX:SingleSpeed"}, {Value: "SPACE1-1 HP Heating Mode"}, {Value: "In"}, {Value: "Out"}}})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc := pthpDirectDocument(t)
			test.mutate(t, &doc)
			targets := energyPathDirectHVACComponentTargets(doc)
			for _, target := range targets {
				if strings.EqualFold(target.ZoneName, "SPACE1-1") {
					t.Fatalf("invalid owner promoted to direct: %#v", target)
				}
			}
			if len(targets) < 21 {
				t.Fatalf("unrelated SPACE3..5 owners discarded: %d targets", len(targets))
			}
			plan := pthpDirectPlan(doc, SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"SPACE1-1"}})
			for _, output := range plan.OutputObjects {
				if pthpDirectName(output.VariableName) {
					t.Fatalf("selected scope laundered rejected ownership: %#v", output)
				}
			}
		})
	}
}
