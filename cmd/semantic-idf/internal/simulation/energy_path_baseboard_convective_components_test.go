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

func energyPathConvectiveBaseboardOriginal(t *testing.T) (idf.Document, string) {
	t.Helper()
	path := filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "MultiStory.idf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return parsePurposePlanFixture(t, string(data)), path
}

func energyPathConvectiveBaseboardHand(t *testing.T) idf.Document {
	t.Helper()
	doc := energyPathBaseboardHand(t)
	baseboard := directHVACFixtureObject(t, &doc, energyPathBaseboardElectricType, "Baseboard")
	baseboard.Type = energyPathBaseboardConvectiveElectricType
	baseboard.Fields = baseboard.Fields[:7]
	list := directHVACFixtureObject(t, &doc, "ZoneHVAC:EquipmentList", "List")
	list.Fields[2].Value = energyPathBaseboardConvectiveElectricType
	return doc
}

func energyPathConvectiveBaseboardZoneFactors() map[string]float64 {
	return map[string]float64{
		"Gnd West Zone": 1, "Gnd Center Zone": 4, "Gnd East Zone": 1,
		"Mid West Zone": 8, "Mid Center Zone": 32, "Mid East Zone": 8,
		"Top West Zone": 1, "Top Center Zone": 4, "Top East Zone": 1,
	}
}

func TestEnergyPathConvectiveBaseboardOriginalNineExactOwnersAndFactors(t *testing.T) {
	doc, _ := energyPathConvectiveBaseboardOriginal(t)
	before := doc.String()
	targets := energyPathBaseboardTargets(doc)
	if !energyPathHasNativeBaseboard(doc) || len(targets) != 9 {
		t.Fatalf("native roster=%+v", targets)
	}
	want := energyPathConvectiveBaseboardZoneFactors()
	index := buildEnergyEffectiveMultiplierIndex(doc)
	seen := map[string]bool{}
	for _, target := range targets {
		factor, exists := want[target.ZoneName]
		key := strings.TrimSuffix(target.ZoneName, " Zone") + " Baseboard"
		if !exists || seen[target.ZoneName] || target.KeyValue != key || target.Component.ObjectType != energyPathBaseboardConvectiveElectricType || target.Component.ObjectName != key || target.Component.ID != fmt.Sprintf("component:%d", target.Component.ObjectIndex) || target.RadiantFraction != 0 || target.PeopleFraction != 0 || len(target.Recipients) != 0 {
			t.Fatalf("convective owner acquired wrong type, key or radiation: %+v", target)
		}
		seen[target.ZoneName] = true
		if got := index.Zones[normalizePurposeToken(target.ZoneName)].effectiveMultiplier(); got != factor {
			t.Errorf("%s factor=%g want=%g", target.ZoneName, got, factor)
		}
		definition, ok := energyPathBaseboardElectricityDefinitionForType(target.Component.ObjectType)
		if !ok || definition.ObjectType != target.Component.ObjectType || definition.ID != "heating.baseboard.electricity" || !reflect.DeepEqual(definition.Energy.Aliases, []string{"Baseboard Electricity Energy"}) {
			t.Fatalf("native direct definition=%+v", definition)
		}
	}
	if len(energyPathBaseboardElectricityDefinitions()) != 2 || energyPathBaseboardElectricityDefinition().ObjectType != energyPathBaseboardElectricType {
		t.Fatal("existing radiant definition contract changed")
	}
	if len(energyPathSharedHeatingElectricTargets(doc)) != 0 || len(energyPathBaseboardHeatRejectionTargets(doc)) != 0 {
		t.Fatal("standalone baseboards invented shared boiler/tower targets")
	}
	if doc.String() != before {
		t.Fatal("typed inspection changed original model")
	}
}

func TestEnergyPathConvectiveBaseboardNativeCapacitySchemaAndAmbiguousOwnersFailClosed(t *testing.T) {
	for _, method := range []string{"HeatingDesignCapacity", "CapacityPerFloorArea", "FractionOfAutosizedHeatingCapacity"} {
		doc := energyPathConvectiveBaseboardHand(t)
		object := directHVACFixtureObject(t, &doc, energyPathBaseboardConvectiveElectricType, "Baseboard")
		object.Fields[2].Value = method
		object.Fields[4].Value, object.Fields[5].Value = "10", "1"
		if got := energyPathBaseboardTargets(doc); len(got) != 1 || len(got[0].Recipients) != 0 {
			t.Fatalf("valid seven-field %s not supported: %+v", method, got)
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*idf.Document)
	}{
		{"missing selected capacity", func(d *idf.Document) {
			o := directHVACFixtureObject(t, d, energyPathBaseboardConvectiveElectricType, "Baseboard")
			o.Fields = o.Fields[:3]
		}},
		{"radiant tail is not convective", func(d *idf.Document) {
			o := directHVACFixtureObject(t, d, energyPathBaseboardConvectiveElectricType, "Baseboard")
			o.Fields = append(o.Fields, idf.Field{Value: "0"}, idf.Field{Value: "0"})
		}},
		{"unknown capacity schema", func(d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathBaseboardConvectiveElectricType, "Baseboard").Fields[2].Value = "Autosize"
		}},
		{"zero efficiency", func(d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathBaseboardConvectiveElectricType, "Baseboard").Fields[6].Value = "0"
		}},
		{"NaN efficiency", func(d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathBaseboardConvectiveElectricType, "Baseboard").Fields[6].Value = "NaN"
		}},
		{"infinite efficiency", func(d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathBaseboardConvectiveElectricType, "Baseboard").Fields[6].Value = "+Inf"
		}},
		{"efficiency above one", func(d *idf.Document) {
			directHVACFixtureObject(t, d, energyPathBaseboardConvectiveElectricType, "Baseboard").Fields[6].Value = "1.01"
		}},
		{"wrong typed equipment list", func(d *idf.Document) {
			directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentList", "List").Fields[2].Value = energyPathBaseboardElectricType
		}},
		{"missing typed owner", func(d *idf.Document) {
			directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentList", "List").Fields[3].Value = "Missing"
		}},
		{"duplicate equipment", func(d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, energyPathBaseboardConvectiveElectricType, "Baseboard"))
		}},
		{"duplicate Zone", func(d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "Zone", "Office"))
		}},
		{"duplicate list", func(d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentList", "List"))
		}},
		{"duplicate connection", func(d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentConnections", "Office"))
		}},
		{"disconnected second owner", func(d *idf.Document) {
			o := *directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentList", "List")
			o.Fields = append([]idf.Field(nil), o.Fields...)
			o.Fields[0].Value = "Unconnected List"
			energyPathBaseboardDuplicate(d, o)
		}},
		{"other Zone connection", func(d *idf.Document) {
			o := *directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentConnections", "Office")
			o.Fields = append([]idf.Field(nil), o.Fields...)
			o.Fields[0].Value = "Other"
			energyPathBaseboardDuplicate(d, o)
		}},
		{"unsupported native name collision", func(d *idf.Document) {
			o := *directHVACFixtureObject(t, d, energyPathBaseboardConvectiveElectricType, "Baseboard")
			o.Type = "ZoneHVAC:Baseboard:Convective:Water"
			energyPathBaseboardDuplicate(d, o)
		}},
		{"radiant native name collision", func(d *idf.Document) {
			o := *directHVACFixtureObject(t, d, energyPathBaseboardConvectiveElectricType, "Baseboard")
			o.Type = energyPathBaseboardElectricType
			energyPathBaseboardDuplicate(d, o)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := energyPathConvectiveBaseboardHand(t)
			tc.mutate(&doc)
			if !energyPathHasNativeBaseboard(doc) || len(energyPathBaseboardTargets(doc)) != 0 {
				t.Fatal("invalid original owner/schema became supported")
			}
			plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, Scope: SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"Office"}}})
			for _, output := range plan.OutputObjects {
				if strings.HasPrefix(output.VariableName, "Baseboard ") || strings.HasPrefix(output.VariableName, "Zone Baseboard ") {
					t.Fatalf("selected scope laundered invalid target: %+v", output)
				}
			}
		})
	}
	for _, objectType := range []string{"ZoneHVAC:Baseboard:Convective:Water", "ZoneHVAC:Baseboard:Unsupported:Electric"} {
		if energyPathBaseboardElectricTypeSupported(objectType) {
			t.Fatal("unsupported native type accepted")
		}
		if _, ok := energyPathBaseboardElectricityDefinitionForType(objectType); ok {
			t.Fatal("unsupported device inferred electric")
		}
	}
}

func TestEnergyPathConvectiveBaseboardNativeOutputPairsAndOriginalPreservation(t *testing.T) {
	for _, hand := range []bool{false, true} {
		for _, selected := range []bool{false, true} {
			t.Run(fmt.Sprintf("hand=%t/selected=%t", hand, selected), func(t *testing.T) {
				doc, _ := energyPathConvectiveBaseboardOriginal(t)
				wantCount := 9
				zone := "Mid Center Zone"
				if hand {
					doc = energyPathConvectiveBaseboardHand(t)
					wantCount = 1
					zone = "Office"
				}
				before := doc.String()
				scope := SimulationPurposeScope{ZoneMode: "all"}
				if selected {
					scope = SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{zone}}
					wantCount = 1
				}
				plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, Scope: scope})
				assertEnergyPathMonthlyHourlyRequestPairs(t, plan)
				seen := map[string]bool{}
				heatingMeter := 0
				for _, output := range plan.OutputObjects {
					if output.ObjectType == "Output:Meter" && output.KeyValue == "Heating:Electricity" && output.ReportingFrequency == "Monthly" {
						heatingMeter++
					}
					if strings.HasPrefix(output.VariableName, "Zone Baseboard Total ") {
						t.Fatalf("phantom temporary Zone-key alias: %+v", output)
					}
					if !strings.HasPrefix(output.VariableName, "Baseboard ") {
						continue
					}
					if output.KeyValue == "*" || output.ScopeZoneName == "" || selected && output.ScopeZoneName != zone {
						t.Fatalf("native request not exact scoped equipment: %+v", output)
					}
					switch output.VariableName {
					case "Baseboard Electricity Energy", "Baseboard Electricity Rate", "Baseboard Total Heating Energy", "Baseboard Total Heating Rate":
					default:
						t.Fatalf("unsupported native request: %+v", output)
					}
					if output.ReportingFrequency != "Monthly" && output.ReportingFrequency != "Hourly" {
						t.Fatalf("unexpected frequency: %+v", output)
					}
					key := output.KeyValue + "/" + output.VariableName + "/" + output.ReportingFrequency
					if seen[key] {
						t.Fatalf("duplicate planned identity %s", key)
					}
					seen[key] = true
				}
				if len(seen) != wantCount*8 || heatingMeter != 1 {
					t.Fatalf("native request=%d want=%d Heating:Electricity=%d", len(seen), wantCount*8, heatingMeter)
				}
				request := PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeKeepExistingAdd)
				if len(request.Updates) != 0 || len(request.RemoveObjectIndexes) != 0 {
					t.Fatal("original outputs would be rewritten")
				}
				applied, preview := idf.ApplyOutput(doc, request)
				if !preview.CanApply {
					t.Fatalf("cannot append purpose outputs: %+v", preview)
				}
				duplicates := map[string]int{}
				for position, original := range doc.Objects {
					if !reflect.DeepEqual(original, applied.Objects[position]) {
						t.Fatalf("original object %d fields/index changed", original.Index)
					}
					if original.Type == "Output:Variable" && energyPathBaseboardField(original, 0) == "*" && energyPathBaseboardField(original, 2) == "Hourly" {
						duplicates[energyPathBaseboardField(original, 1)]++
					}
				}
				if hand && (duplicates["Baseboard Total Heating Energy"] != 2 || duplicates["Baseboard Total Heating Rate"] != 2) {
					t.Fatal("manual duplicate wildcards removed")
				}
				if doc.String() != before {
					t.Fatal("planning mutated original")
				}
			})
		}
	}
}
