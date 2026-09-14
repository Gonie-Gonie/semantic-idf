package simulation

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func energyPathPoolExpectedRequests() map[[3]string]bool {
	want := map[[3]string]bool{}
	for _, pair := range [][3]string{
		{"Test Pool", "Indoor Pool Water Heating Energy", "Indoor Pool Water Heating Rate"},
		{"Central Boiler", "Boiler Heating Energy", "Boiler Heating Rate"},
		{"Central Boiler", "Boiler NaturalGas Energy", "Boiler NaturalGas Rate"},
		{"Central Boiler", "Boiler Ancillary NaturalGas Energy", "Boiler Ancillary NaturalGas Rate"},
		{"Central Boiler", "Boiler Ancillary Electricity Energy", "Boiler Ancillary Electricity Rate"},
		{"HW Circ Pump", "Pump Electricity Energy", "Pump Electricity Rate"},
		{"CW Circ Pump", "Pump Electricity Energy", "Pump Electricity Rate"},
	} {
		for _, name := range pair[1:] {
			for _, frequency := range []string{"Monthly", "Hourly"} {
				want[[3]string{pair[0], name, frequency}] = true
			}
		}
	}
	return want
}

func energyPathPoolRequestTuple(output PurposeOutputObject) [3]string {
	return [3]string{purposeOutputKeyValue(output.Fields), purposeOutputVariableName(output.Fields), purposeOutputFrequency(output.ObjectType, output.Fields)}
}

func TestEnergyPathPoolOutputIntentsAreSevenExactNativePairsAtBothFrequencies(t *testing.T) {
	doc := energyPathPoolOriginal(t)
	before := doc.String()
	requests := energyPathPoolOutputRequests(doc)
	want := energyPathPoolExpectedRequests()
	if len(requests) != 28 || len(want) != 28 {
		t.Fatalf("seven keyed native pairs imply 28 M/H name intents, not 28 physical additions: %d/%d", len(requests), len(want))
	}
	seen := map[[3]string]bool{}
	for _, output := range requests {
		key := energyPathPoolRequestTuple(output)
		if !want[key] || seen[key] || output.ObjectType != "Output:Variable" || output.Reason != "Basic Energy Path" || !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) {
			t.Fatalf("unexpected/duplicate native intent: %+v", output)
		}
		seen[key] = true
		if key[2] == "Hourly" && (output.Weight != "heavy" || !strings.Contains(output.Description, "never a second additive budget")) {
			t.Fatalf("Hourly companion became an additive budget: %+v", output)
		}
		if strings.HasSuffix(key[1], "Rate") && !strings.Contains(output.Description, "not a replacement for an unavailable Monthly Energy") {
			t.Fatalf("rate fallback can fabricate missing purchased Energy: %+v", output)
		}
		thermal := strings.HasPrefix(key[1], "Indoor Pool Water Heating ") || strings.HasPrefix(key[1], "Boiler Heating ")
		if thermal {
			if !strings.Contains(output.Description, "non-additive to purchased energy and Zone-air delivered load") || strings.Contains(output.Description, "source-local budget authority") {
				t.Fatalf("thermal response crossed the purchased/Zone-air boundary: %+v", output)
			}
		} else if !strings.Contains(output.Description, "Monthly Energy is the source-local budget authority") || !strings.Contains(output.Description, "remains unassigned") {
			t.Fatalf("native consuming source lost its explicit budget/unknown contract: %+v", output)
		}
		zoneName := ""
		if key[0] == "Test Pool" {
			zoneName = "SPACE1-1"
		}
		if output.ScopeZoneName != zoneName {
			t.Fatalf("invented pool/plant recipient from output name: %+v", output)
		}
	}
	if before != doc.String() {
		t.Fatal("intent discovery mutated original physical input")
	}
}

func TestEnergyPathPoolPurposePlanRetainsSharedObservationAndManualOriginals(t *testing.T) {
	for _, extraDuplicates := range []bool{false, true} {
		for _, scope := range []SimulationPurposeScope{
			{ZoneMode: "all"},
			{ZoneMode: "selected", ZoneNames: []string{"SPACE2-1"}},
			{ZoneMode: "selected", ZoneNames: []string{"missing Zone"}},
		} {
			t.Run(scope.ZoneMode+"/"+strings.Join(scope.ZoneNames, ",")+map[bool]string{false: "/original", true: "/manual duplicates"}[extraDuplicates], func(t *testing.T) {
				doc := energyPathPoolOriginal(t)
				var originalWildcard *idf.Object
				found := false
				for at := range doc.Objects {
					object := &doc.Objects[at]
					if strings.EqualFold(object.Type, "Output:Variable") && energyPathPoolField(*object, 0) == "*" && energyPathPoolField(*object, 1) == "Indoor Pool Water Heating Rate" && strings.EqualFold(energyPathPoolField(*object, 2), "Hourly") {
						originalWildcard, found = object, true
						break
					}
				}
				if !found || originalWildcard.Index != 320 {
					t.Fatal("lost hash-bound original wildcard Hourly pool-rate opener")
				}
				if extraDuplicates {
					energyPathPoolTestAppend(&doc, *originalWildcard)
					manual := idf.Object{Type: "Output:Variable", Fields: []idf.Field{{Value: "Central Boiler"}, {Value: "Boiler NaturalGas Energy"}, {Value: "Monthly"}}}
					energyPathPoolTestAppend(&doc, manual)
					energyPathPoolTestAppend(&doc, manual)
				}
				before := doc.String()
				plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, Scope: scope})
				want, seen := energyPathPoolExpectedRequests(), map[[3]string]bool{}
				newNative := map[[3]string]bool{}
				for _, output := range plan.OutputObjects {
					if _, _, native := energyPathPoolOutputDefinitionForName(output.VariableName); !native || output.ObjectType != "Output:Variable" {
						continue
					}
					key := energyPathPoolRequestTuple(output)
					if !want[key] || seen[key] {
						t.Fatalf("full builder lost exact native M/H intents: %+v", output)
					}
					seen[key] = true
					if output.State == PurposeOutputStateExisting {
						if output.ObjectIndex == nil {
							t.Fatalf("existing request has no exact original object: %+v", output)
						}
						original := doc.Objects[*output.ObjectIndex]
						if [3]string{energyPathPoolField(original, 0), energyPathPoolField(original, 1), energyPathPoolField(original, 2)} != key {
							t.Fatalf("wildcard/frequency identity substituted for exact original key: %+v", output)
						}
					} else {
						if output.State != PurposeOutputStateTemporary || output.ObjectIndex != nil {
							t.Fatalf("native observation rewrites an original output: %+v", output)
						}
						newNative[key] = true
					}
				}
				if !reflect.DeepEqual(seen, want) {
					t.Fatalf("native shared context incorrectly gated by selected Zone: got %d want %d", len(seen), len(want))
				}
				request := PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeKeepExistingAdd)
				if len(request.Updates) != 0 || len(request.RemoveObjectIndexes) != 0 {
					t.Fatal("pool outputs must not rewrite/remove original objects")
				}
				applied, preview := idf.ApplyOutput(doc, request)
				if !preview.CanApply || len(applied.Objects) < len(doc.Objects) {
					t.Fatalf("native observation run-copy additions blocked: %+v", preview)
				}
				for position, original := range doc.Objects {
					if !reflect.DeepEqual(original, applied.Objects[position]) {
						t.Fatalf("original index %d lost literal fields/comments/duplicates", original.Index)
					}
				}
				for _, object := range applied.Objects[len(doc.Objects):] {
					if !strings.EqualFold(object.Type, "Output:Variable") {
						continue
					}
					if _, _, native := energyPathPoolOutputDefinitionForName(energyPathPoolField(object, 1)); !native {
						continue
					}
					key := [3]string{energyPathPoolField(object, 0), energyPathPoolField(object, 1), energyPathPoolField(object, 2)}
					if !newNative[key] {
						t.Fatalf("unexpected or duplicate physical native addition: %+v", object)
					}
					delete(newNative, key)
				}
				if len(newNative) != 0 {
					t.Fatalf("missing planned native additions: %v", newNative)
				}
				if before != doc.String() {
					t.Fatal("building/applying the run copy mutated the input document")
				}
				// On a second plan every exact native intent is existing; preserve
				// any original wildcard and duplicate objects without adding more.
				again := BuildPurposeRunPlan(applied, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, Scope: scope})
				for _, output := range again.OutputObjects {
					if _, _, native := energyPathPoolOutputDefinitionForName(output.VariableName); native && output.ObjectType == "Output:Variable" && output.State != PurposeOutputStateExisting {
						t.Fatalf("repeated planning adds a duplicate native observation: %+v", output)
					}
				}
			})
		}
	}
}

func TestEnergyPathPoolRequestsObserveUniqueSourcesDespiteUnresolvedRecipients(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, *idf.Document)
	}{
		{"unknown pool surface", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "SwimmingPool:Indoor", "Test Pool").Fields[1].Value = "missing Floor"
		}},
		{"pool has no owned water branch", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "Branch", "Swimming Pool Branch").Fields[3].Value = "missing pool"
		}},
		{"bad HW connector", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "Connector:Splitter", "Heating Demand Splitter").Fields[9].Value = "foreign branch"
		}},
		{"unowned CW pump", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "Branch", "CW Pump Branch").Fields[3].Value = "missing pump"
		}},
		{"missing native source water ports", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "Boiler:HotWater", "Central Boiler").Fields[10].Value = ""
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := energyPathPoolOriginal(t)
			tc.mutate(t, &doc)
			inventory := energyPathNativePoolInventory(doc)
			if !inventory.HasNativePool || !inventory.SchemaReviewed {
				t.Fatal("topology failure erased native presence")
			}
			got := map[[3]string]bool{}
			for _, request := range energyPathPoolOutputRequests(doc) {
				key := energyPathPoolRequestTuple(request)
				if got[key] {
					t.Fatalf("duplicate native observation %v", key)
				}
				got[key] = true
				if tc.name == "unknown pool surface" && key[0] == "Test Pool" && request.ScopeZoneName != "" {
					t.Fatalf("unknown surface fabricated a Zone owner: %+v", request)
				}
			}
			if !reflect.DeepEqual(got, energyPathPoolExpectedRequests()) {
				t.Fatalf("exact native observations suppressed by an allocation-proof failure: %v", got)
			}
		})
	}
}

func TestEnergyPathPoolRequestsRejectAmbiguousIdentityFuelAndAbsentOptIn(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mutate     func(*testing.T, *idf.Document)
		blockedKey string
		wantCount  int
	}{
		{"duplicate native pool key", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestAppend(doc, *energyPathPoolTestObject(t, doc, "SwimmingPool:Indoor", "Test Pool"))
		}, "Test Pool", 24},
		{"unreviewed boiler namespace peer", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestAppend(doc, idf.Object{Type: "Boiler:Steam", Fields: []idf.Field{{Value: "Central Boiler"}}})
		}, "Central Boiler", 12},
		{"unreviewed pump namespace peer", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestAppend(doc, idf.Object{Type: "HeaderedPumps:ConstantSpeed", Fields: []idf.Field{{Value: "HW Circ Pump"}}})
		}, "HW Circ Pump", 24},
		{"actual different boiler fuel", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "Boiler:HotWater", "Central Boiler").Fields[1].Value = "Propane"
		}, "Central Boiler", 12},
		{"blank boiler fuel", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "Boiler:HotWater", "Central Boiler").Fields[1].Value = ""
		}, "Central Boiler", 12},
		{"unreviewed version", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "Version", "").Fields[0].Value = "24.2"
		}, "", 0},
		{"native pool absent", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "SwimmingPool:Indoor", "Test Pool").Type = "Unreviewed:Pool"
		}, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := energyPathPoolOriginal(t)
			tc.mutate(t, &doc)
			requests := energyPathPoolOutputRequests(doc)
			if len(requests) != tc.wantCount {
				t.Fatalf("native opt-in/identity/fuel intent count=%d want=%d", len(requests), tc.wantCount)
			}
			for _, request := range requests {
				if energyPathPoolRequestTuple(request)[0] == tc.blockedKey {
					t.Fatalf("unproved native alias was requested: %+v", request)
				}
			}
		})
	}
	for _, detail := range []string{PurposeBasicEnergyDetailLight, PurposeBasicEnergyDetailExplain, PurposeBasicEnergyDetailHeatDrivers} {
		plan := BuildPurposeRunPlan(energyPathPoolOriginal(t), SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: detail})
		for _, output := range plan.OutputObjects {
			if _, _, native := energyPathPoolOutputDefinitionForName(output.VariableName); native && output.ObjectType == "Output:Variable" {
				t.Fatalf("pool backend capture expanded unrelated Basic Energy detail %s: %+v", detail, output)
			}
		}
	}
}
