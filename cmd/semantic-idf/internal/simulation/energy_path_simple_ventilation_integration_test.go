package simulation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathSimpleVentilationHourlyExactCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, key, frequency, schedule string
		count, reused                  int
	}{
		{"wildcard", "*", "Hourly", "", 1, 1},
		{"blank wildcard", "", "Hourly", "", 1, 1},
		{"one owner", "Intake", "Hourly", "", 3, 1},
		{"foreign key", "Foreign", "Hourly", "", 3, 0},
		{"scheduled wildcard", "*", "Hourly", "Always", 3, 0},
		{"scheduled owner", "Intake", "Hourly", "Always", 3, 0},
		{"wrong frequency", "*", "Daily", "", 3, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := simpleVentilationHandDocument(t)
			doc := parsePurposePlanFixture(t, base.String()+"\nOutput:Variable,"+tc.key+","+energyPathSimpleVentilationFanName+","+tc.frequency+","+tc.schedule+";")
			before := doc.String()
			opener := doc.Objects[len(doc.Objects)-1].Index
			plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
			count, reused := 0, 0
			for _, output := range plan.OutputObjects {
				if !strings.EqualFold(output.VariableName, energyPathSimpleVentilationFanName) || !strings.EqualFold(output.ReportingFrequency, "Hourly") {
					continue
				}
				count++
				if purposeFieldValue(output.Fields, "Schedule Name") != "" || !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) {
					t.Fatalf("filtered/foreign purpose borrowed by unfiltered chart: %#v", output)
				}
				if output.ObjectIndex != nil {
					reused++
					if *output.ObjectIndex != opener || output.KeyValue != tc.key || output.State != PurposeOutputStateExisting {
						t.Fatalf("lost actual original request: %#v", output)
					}
				} else if output.ScopeZoneName == "" || output.ScopeZoneName != output.KeyValue {
					t.Fatalf("new chart request lost exact owner: %#v", output)
				}
			}
			if count != tc.count || reused != tc.reused || doc.String() != before {
				t.Fatalf("native Hourly coverage: got %d/%d reused; want %d/%d", count, reused, tc.count, tc.reused)
			}
			_, preview := idf.ApplyOutput(doc, PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeKeepExistingAdd))
			if !preview.CanApply {
				t.Fatal("preserving original outputs blocked normal application")
			}
		})
	}
}

func TestEnergyPathSimpleVentilationOriginalRetainsHourlyWildcard(t *testing.T) {
	path := filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "VentilationSimpleTest.idf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if epathRealHash(data) != "e38887ebcbfec13df596bcd77ed1d5965a65a4bd1e0ad026672f449d8fbe1074" {
		t.Fatal("vendored original changed")
	}
	doc := parsePurposePlanFixture(t, string(data))
	before := doc.String()
	opener := -1
	for _, object := range doc.Objects {
		if strings.EqualFold(object.Type, "Output:Variable") && energyPathSimpleVentilationField(object, 0) == "*" && strings.EqualFold(energyPathSimpleVentilationField(object, 1), energyPathSimpleVentilationFanName) && strings.EqualFold(energyPathSimpleVentilationField(object, 2), "Hourly") {
			if opener != -1 {
				t.Fatal("original wildcard is not unique")
			}
			opener = object.Index
		}
	}
	if opener < 0 {
		t.Fatal("original native Hourly wildcard disappeared")
	}
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	monthly := map[string]int{}
	hourly := 0
	for _, output := range plan.OutputObjects {
		if !strings.EqualFold(output.VariableName, energyPathSimpleVentilationFanName) {
			continue
		}
		switch strings.ToLower(output.ReportingFrequency) {
		case "monthly":
			if output.KeyValue != "ZONE 1" && output.KeyValue != "ZONE 2" && output.KeyValue != "ZONE 3" || output.ScopeZoneName != output.KeyValue {
				t.Fatalf("Monthly output lost original aggregate owner: %#v", output)
			}
			monthly[output.KeyValue]++
		case "hourly":
			hourly++
			if output.KeyValue != "*" || output.ObjectIndex == nil || *output.ObjectIndex != opener {
				t.Fatalf("original Hourly wildcard replaced or duplicated by Zone requests: %#v", output)
			}
		default:
			t.Fatalf("unexpected native request frequency: %#v", output)
		}
	}
	if !reflect.DeepEqual(monthly, map[string]int{"ZONE 1": 1, "ZONE 2": 1, "ZONE 3": 1}) || hourly != 1 || doc.String() != before {
		t.Fatalf("original request census/immutability changed: monthly=%v hourly=%d", monthly, hourly)
	}
}

func TestEnergyPathSimpleVentilationSourceContextChangesOnlyExplanation(t *testing.T) {
	targets := energyPathSimpleVentilationTargets(simpleVentilationHandDocument(t))
	opener := 700
	for _, frequency := range []string{"Monthly", "Hourly"} {
		original := EnergyDataSource{ID: "native", SourceType: "sql_report_data", KeyValue: "Natural", Name: energyPathSimpleVentilationFanName, ZoneName: "Natural", ObjectIndex: &opener, SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: frequency, EffectiveMultiplier: 1, MultiplierApplication: energyMultiplierAlreadyModelTotal, observedValuePresence: energySourceObservedRaw | energySourceObservedEffective}
		sources := []EnergyDataSource{original}
		applyEnergyPathSimpleVentilationSourceContext(sources, targets)
		if !strings.Contains(sources[0].Explanation, "Original members:") || !strings.Contains(sources[0].Explanation, "Natural inlet") {
			t.Fatal("original member identity missing from source explanation")
		}
		sources[0].Explanation = ""
		if !reflect.DeepEqual(sources[0], original) {
			t.Fatalf("metadata changed actual opener, zero knownness or source values: %#v", sources[0])
		}
	}
}

// Projection-only regression. The typed original/SQL binding is independently
// exercised by the sibling tests. Pure native aggregate paths are empty; a
// mixed native/foreign source node must keep its foreign path intact. Both the
// explicit-edge and no-edge direct carrier branches must obey that boundary.
func TestEnergyPathSimpleVentilationDirectGraphNeverBorrowsServicePaths(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		for _, mixed := range []bool{false, true} {
			for _, period := range []string{"annual", "M1"} {
				scope := EnergyExplanationScope{Kind: "zone", ZoneName: "Intake", AggregationBasis: "model_total"}
				source := EnergyDataSource{ID: "native", SourceType: "sql_report_data", Name: energyPathSimpleVentilationFanName, KeyValue: "Intake", ZoneName: "Intake", ReportingFrequency: "Monthly", SourceUnit: "J", NormalizedUnit: "kWh"}
				sources := []EnergyDataSource{source}
				native := EnergyExplanationNode{ID: "native.fans", Level: "energy", EndUse: "fans", Carrier: "electricity", ZoneName: "Intake", Value: 6, Unit: "kWh", Period: period, Basis: "direct_zone_energy", SourceIDs: []string{"native"}, RelatedPathIDs: []string{"foreign-service"}}
				if mixed {
					foreign := source
					foreign.ID, foreign.Name, foreign.KeyValue = "foreign", "Fan Electricity Energy", "Other Fan"
					sources = append(sources, foreign)
					native.SourceIDs = append(native.SourceIDs, "foreign")
				}
				nodes := []EnergyExplanationNode{native, {ID: "electricity", Level: "energy", EndUse: "total", Carrier: "electricity", ZoneName: "Intake", Value: 6, Unit: "kWh", Period: period, Basis: "direct_zone_energy", SourceIDs: []string{"facility"}, RelatedPathIDs: []string{"foreign-service"}}}
				var edges []EnergyExplanationEdge
				if explicit {
					edges = []EnergyExplanationEdge{{ID: "direct", FromID: "electricity", ToID: native.ID, Value: 6, Unit: "kWh", Period: period, Relation: "energy_variable", Basis: "direct_zone_energy", SourceIDs: append([]string(nil), native.SourceIDs...), RelatedPathIDs: []string{"foreign-service"}}}
				}
				built, links := upgradeEnergyExplanationGraph(nodes, edges, sources, scope, PurposeAllocationPolicyByServicePathLoadShare, true, nil)
				result := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: scope, Nodes: built, Links: links, Sources: sources}
				for pass := 0; pass < 3; pass++ {
					node := energyPathV2NodeByID(result.Nodes, "end_use.fans.intake")
					if node == nil || node.Value != 6 || node.Basis != "direct_zone_energy" || !reflect.DeepEqual(node.SourceIDs, sortedSimpleVentilationTestSourceIDs(native.SourceIDs)) || (len(node.RelatedPathIDs) > 0) != mixed {
						t.Fatalf("explicit=%t mixed=%t %s pass%d direct node changed: %#v", explicit, mixed, period, pass, node)
					}
					branches := 0
					for _, link := range result.Links {
						if link.Relation == "load_to_end_use" {
							t.Fatalf("fan invented thermal conversion: %#v", link)
						}
						if link.FromID != node.ID || !energyPathLinkIsCarrierSplit(link) {
							continue
						}
						branches++
						if link.FromValue != 6 || link.ToValue != 6 || link.Basis != "direct_zone_energy" || !reflect.DeepEqual(link.SourceIDs, sortedSimpleVentilationTestSourceIDs(native.SourceIDs)) || (len(link.RelatedPathIDs) > 0) != mixed {
							t.Fatalf("explicit=%t mixed=%t %s pass%d branch borrowed/lost evidence: %#v", explicit, mixed, period, pass, link)
						}
					}
					if branches != 1 {
						t.Fatalf("native carrier branch count=%d", branches)
					}
					if pass < 2 {
						wire, err := json.Marshal(result)
						if err != nil {
							t.Fatal(err)
						}
						var decoded EnergyExplanationResult
						if err := json.Unmarshal(wire, &decoded); err != nil {
							t.Fatal(err)
						}
						result = decoded
					}
				}
			}
		}
	}
}

func sortedSimpleVentilationTestSourceIDs(ids []string) []string {
	if len(ids) == 2 {
		return []string{"foreign", "native"}
	}
	return []string{"native"}
}
