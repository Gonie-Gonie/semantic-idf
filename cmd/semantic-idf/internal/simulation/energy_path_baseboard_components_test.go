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

func energyPathBaseboardOriginal(t *testing.T) idf.Document {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "5ZoneElectricBaseboard.idf"))
	if err != nil {
		t.Fatal(err)
	}
	return parsePurposePlanFixture(t, string(data))
}

func energyPathBaseboardHand(t *testing.T) idf.Document {
	t.Helper()
	return parsePurposePlanFixture(t, `
Version,25.1;
Zone,Office,0,0,0,0,1,7;
Zone,Other,0,0,0,0,1,1;
BuildingSurface:Detailed,Floor,Floor,Slab,Office,,Ground,,NoSun,NoWind,1,3,0,0,0,1,0,0,0,1,0;
BuildingSurface:Detailed,Other Floor,Floor,Slab,Other,,Ground,,NoSun,NoWind,1,3,0,0,0,1,0,0,0,1,0;
ZoneHVAC:EquipmentConnections,Office,List,,,Office Air,;
ZoneHVAC:EquipmentList,List,SequentialLoad,ZoneHVAC:Baseboard:RadiantConvective:Electric,Baseboard,1,1,,;
ZoneHVAC:Baseboard:RadiantConvective:Electric,Baseboard,Always,HeatingDesignCapacity,Autosize,,,.97,.2,.3,Floor,.7;
Output:Variable,*,Baseboard Total Heating Energy,Hourly;
Output:Variable,*,Baseboard Total Heating Energy,Hourly;
Output:Variable,*,Baseboard Total Heating Rate,Hourly;
Output:Variable,*,Baseboard Total Heating Rate,Hourly;
`)
}

func energyPathBaseboardDuplicate(doc *idf.Document, object idf.Object) {
	object.Fields = append([]idf.Field(nil), object.Fields...)
	object.Index = len(doc.Objects)
	doc.Objects = append(doc.Objects, object)
}

func TestEnergyPathBaseboardTypedOriginalAndNonUnitOwner(t *testing.T) {
	for _, tc := range []struct {
		name       string
		doc        idf.Document
		owners     map[string]string
		recipients int
	}{
		{"original", energyPathBaseboardOriginal(t), map[string]string{"SPACE2-1 Baseboard": "SPACE2-1", "SPACE4-1 Baseboard": "SPACE4-1"}, 10},
		{"commentless multiplier7", energyPathBaseboardHand(t), map[string]string{"Baseboard": "Office"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.doc.String()
			targets := energyPathBaseboardTargets(tc.doc)
			if len(targets) != len(tc.owners) {
				t.Fatalf("typed owners=%+v, want %v", targets, tc.owners)
			}
			count := 0
			for _, target := range targets {
				object := directHVACFixtureObject(t, &tc.doc, energyPathBaseboardElectricType, target.KeyValue)
				if tc.owners[target.KeyValue] != target.ZoneName || target.Component.ObjectIndex != object.Index || target.Component.ObjectType != energyPathBaseboardElectricType || target.Component.ID != fmt.Sprintf("component:%d", object.Index) || target.RadiantFraction != .2 || target.PeopleFraction != .3 {
					t.Fatalf("target changed native identity/fractions: %+v", target)
				}
				for _, recipient := range target.Recipients {
					surface := directHVACFixtureObject(t, &tc.doc, "BuildingSurface:Detailed", recipient.SurfaceName)
					if recipient.ZoneName != target.ZoneName || recipient.SurfaceObjectIndex != surface.Index || recipient.Fraction <= 0 {
						t.Fatalf("recipient lost exact original surface: %+v", recipient)
					}
					count++
				}
			}
			if count != tc.recipients || before != tc.doc.String() {
				t.Fatalf("recipient count/mutation: count=%d, want %d", count, tc.recipients)
			}
			context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(tc.doc), tc.doc)
			if !context.HasNativeBaseboard || len(context.BaseboardTargets) != len(targets) || !reflect.DeepEqual(context.DirectHVACComponents, energyPathBaseboardDirectTargets(targets)) {
				t.Fatal("new typed targets did not register with direct consumption context")
			}
			if energyPathDirectHVACParentSupported(energyPathBaseboardElectricType) || len(energyPathDirectHVACParentDefinitions(energyPathBaseboardElectricType)) != 0 {
				t.Fatal("baseboard broadened PTAC/PTHP parent assumptions")
			}
		})
	}
}

func TestEnergyPathBaseboardOwnerAndRecipientAmbiguityFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, *idf.Document)
	}{
		{"missing equipment", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentList", "List").Fields[3].Value = "Missing"
		}},
		{"wrong typed list", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentList", "List").Fields[2].Value = "ZoneHVAC:Baseboard:Convective:Electric"
		}},
		{"missing list", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentConnections", "Office").Fields[1].Value = "Missing"
		}},
		{"duplicate equipment", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, energyPathBaseboardElectricType, "Baseboard"))
		}},
		{"unsupported same reporting key", func(t *testing.T, d *idf.Document) {
			p := *directHVACFixtureObject(t, d, energyPathBaseboardElectricType, "Baseboard")
			p.Type = "ZoneHVAC:Baseboard:Convective:Electric"
			energyPathBaseboardDuplicate(d, p)
		}},
		{"duplicate Zone", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "Zone", "Office"))
		}},
		{"duplicate list", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentList", "List"))
		}},
		{"disconnected second list", func(t *testing.T, d *idf.Document) {
			p := *directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentList", "List")
			energyPathBaseboardDuplicate(d, p)
			d.Objects[len(d.Objects)-1].Fields[0].Value = "Unconnected List"
		}},
		{"duplicate connection", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentConnections", "Office"))
		}},
		{"other Zone connection", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentConnections", "Office"))
			d.Objects[len(d.Objects)-1].Fields[0].Value = "Other"
		}},
		{"unknown typed wrapper", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, idf.Object{Type: "Unsupported:Wrapper", Fields: []idf.Field{{Value: "Wrapper"}, {Value: energyPathBaseboardElectricType}, {Value: "Baseboard"}}})
		}},
		{"foreign surface Zone", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "BuildingSurface:Detailed", "Floor").Fields[3].Value = "Other"
		}},
		{"foreign surface Space", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "BuildingSurface:Detailed", "Floor").Fields[4].Value = "Other Space"
			energyPathBaseboardDuplicate(d, idf.Object{Type: "Space", Fields: []idf.Field{{Value: "Other Space"}, {Value: "Other"}}})
		}},
		{"duplicate surface", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "BuildingSurface:Detailed", "Floor"))
		}},
		{"missing surface", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathBaseboardElectricType, "Baseboard").Fields[9].Value = "Missing"
		}},
		{"duplicate recipient", func(t *testing.T, d *idf.Document) {
			p := directHVACFixtureObject(t, d, energyPathBaseboardElectricType, "Baseboard")
			p.Fields = append(p.Fields, idf.Field{Value: "Floor"}, idf.Field{Value: "0"})
		}},
		{"invalid sum", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathBaseboardElectricType, "Baseboard").Fields[10].Value = ".5"
		}},
		{"NaN radiation", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathBaseboardElectricType, "Baseboard").Fields[7].Value = "NaN"
		}},
		{"infinite fraction", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathBaseboardElectricType, "Baseboard").Fields[10].Value = "+Inf"
		}},
		{"negative people", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathBaseboardElectricType, "Baseboard").Fields[8].Value = "-.1"
		}},
		{"zero efficiency", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathBaseboardElectricType, "Baseboard").Fields[6].Value = "0"
		}},
		{"legacy inline schema", func(t *testing.T, d *idf.Document) {
			p := directHVACFixtureObject(t, d, energyPathBaseboardElectricType, "Baseboard")
			p.Fields = append(p.Fields[:2], p.Fields[3:]...)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := energyPathBaseboardHand(t)
			tc.mutate(t, &doc)
			if !energyPathHasNativeBaseboard(doc) || len(energyPathBaseboardTargets(doc)) != 0 {
				t.Fatal("ambiguous original model became direct target or hid native presence")
			}
			plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, Scope: SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"Office"}}})
			for _, output := range plan.OutputObjects {
				if strings.HasPrefix(normalizeEnergyOutputName(output.VariableName), "baseboard ") || strings.HasPrefix(normalizeEnergyOutputName(output.VariableName), "zone baseboard total ") {
					t.Fatalf("selected scope laundered native owner or fake alias: %+v", output)
				}
			}
		})
	}
}

func TestEnergyPathBaseboardZeroRadiationAndRecipientRemainNonRecipients(t *testing.T) {
	doc := energyPathBaseboardHand(t)
	directHVACFixtureObject(t, &doc, energyPathBaseboardElectricType, "Baseboard").Fields[7].Value = "0"
	if targets := energyPathBaseboardTargets(doc); len(targets) != 1 || len(targets[0].Recipients) != 0 {
		t.Fatalf("zero radiation invented recipients or lost electric owner: %+v", targets)
	}
	doc = energyPathBaseboardHand(t)
	parent := directHVACFixtureObject(t, &doc, energyPathBaseboardElectricType, "Baseboard")
	parent.Fields[8].Value = "1"
	parent.Fields[10].Value = "0"
	if targets := energyPathBaseboardTargets(doc); len(targets) != 1 || len(targets[0].Recipients) != 0 {
		t.Fatalf("zero fraction invented recipient: %+v", targets)
	}
}

func TestEnergyPathBaseboardNativeOutputPairsAndOriginalDuplicatePreservation(t *testing.T) {
	doc := energyPathBaseboardOriginal(t)
	before := doc.String()
	for _, tc := range []struct {
		name   string
		scope  SimulationPurposeScope
		owners map[string]string
	}{
		{"all", SimulationPurposeScope{ZoneMode: "all"}, map[string]string{"SPACE2-1 Baseboard": "SPACE2-1", "SPACE4-1 Baseboard": "SPACE4-1"}},
		{"selected", SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"SPACE2-1"}}, map[string]string{"SPACE2-1 Baseboard": "SPACE2-1"}},
		{"no local baseboard", SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"SPACE1-1"}}, map[string]string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, Scope: tc.scope})
			assertEnergyPathMonthlyHourlyRequestPairs(t, plan)
			seen := map[string]bool{}
			for _, output := range plan.OutputObjects {
				if strings.HasPrefix(normalizeEnergyOutputName(output.VariableName), "zone baseboard total ") {
					t.Fatalf("nonexistent Zone alias requested: %+v", output)
				}
				if !strings.HasPrefix(normalizeEnergyOutputName(output.VariableName), "baseboard ") {
					continue
				}
				owner, exists := tc.owners[output.KeyValue]
				if !exists || owner != output.ScopeZoneName || output.ObjectType != "Output:Variable" || !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) || (output.ReportingFrequency != "Monthly" && output.ReportingFrequency != "Hourly") {
					t.Fatalf("native key/owner/frequency missing: %+v", output)
				}
				switch output.VariableName {
				case "Baseboard Electricity Energy", "Baseboard Electricity Rate", "Baseboard Total Heating Energy", "Baseboard Total Heating Rate":
				default:
					t.Fatalf("unsupported native output: %+v", output)
				}
				key := output.KeyValue + "|" + output.VariableName + "|" + output.ReportingFrequency
				if seen[key] {
					t.Fatalf("duplicate exact request %s", key)
				}
				seen[key] = true
			}
			if len(seen) != len(tc.owners)*8 {
				t.Fatalf("native request count=%d want=%d", len(seen), len(tc.owners)*8)
			}
			request := PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeKeepExistingAdd)
			if len(request.Updates) != 0 || len(request.RemoveObjectIndexes) != 0 {
				t.Fatal("native request rewrites original manual outputs")
			}
			applied, preview := idf.ApplyOutput(doc, request)
			if !preview.CanApply {
				t.Fatalf("cannot apply additive outputs: %+v", preview)
			}
			duplicates := map[string]int{}
			for position, original := range doc.Objects {
				if !reflect.DeepEqual(original, applied.Objects[position]) {
					t.Fatalf("original object %d changed index/fields/metadata", original.Index)
				}
				if strings.EqualFold(original.Type, "Output:Variable") && energyPathBaseboardField(original, 0) == "*" && strings.EqualFold(energyPathBaseboardField(original, 2), "Hourly") && (energyPathBaseboardField(original, 1) == "Baseboard Total Heating Energy" || energyPathBaseboardField(original, 1) == "Baseboard Total Heating Rate") {
					duplicates[energyPathBaseboardField(original, 1)]++
				}
			}
			if duplicates["Baseboard Total Heating Energy"] != 2 || duplicates["Baseboard Total Heating Rate"] != 2 {
				t.Fatalf("original duplicate native wildcard roster changed: %v", duplicates)
			}
			if before != doc.String() {
				t.Fatal("planning/apply changed original input")
			}
		})
	}
}
