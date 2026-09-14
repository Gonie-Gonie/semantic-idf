package simulation

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEnergyPathPoolBoundaryProducerRetainsObservedQuantitiesAndActualConsumers(t *testing.T) {
	inventory := energyPathNativePoolInventory(energyPathPoolOriginal(t))
	evidence := energyPathPoolEvidence{Inventory: inventory}
	for _, definition := range energyPathPoolOutputDefinitions() {
		if definition.EndUse != "heating" && definition.EndUse != "pumps" {
			continue
		}
		for _, source := range inventory.Sources {
			if !definition.matchesOriginal(source.Component.ObjectType, source.FuelType) {
				continue
			}
			id := "sql-rdd-20"
			if source.Component.ObjectName == "CW Circ Pump" {
				id = "sql-rdd-21"
			}
			evidence.Observations = append(evidence.Observations, energyPathPoolObservation{Definition: definition, Component: source.Component, Loop: source.Loop,
				Valid: true, Series: energyExplanationSeries{SourceIDs: []string{id}}})
		}
	}
	nodes := []EnergyExplanationNode{
		{ID: "gas", Level: "energy", Kind: "energy.heating", EndUse: "heating", Carrier: "natural_gas", Value: 100, SourceIDs: []string{"sql-rdd-1"}},
		{ID: "zero", Level: "energy", Kind: "energy.heating", EndUse: "heating", Carrier: "electricity", Value: 0, SourceIDs: []string{"sql-rdd-2"}},
		{ID: "pumps", Level: "energy", Kind: "energy.pumps", EndUse: "pumps", Carrier: "electricity", Value: 120, SourceIDs: []string{"sql-rdd-3"}},
		{ID: "cooling", Level: "energy", Kind: "energy.cooling", EndUse: "cooling", Carrier: "electricity", Value: 40, SourceIDs: []string{"sql-rdd-4"}},
		{ID: "local", Level: "energy", Kind: "energy.heating", EndUse: "heating", Carrier: "electricity", ZoneName: "SPACE2-1", Value: 10, SourceIDs: []string{"sql-rdd-5"}},
	}
	before, _ := json.Marshal(nodes)
	got := energyPathPoolRestrictedNodes(nodes, evidence)
	for i := 0; i < 3; i++ {
		if len(got[i].serviceBoundaryRestrictions) != 1 {
			t.Fatalf("missing original non-Zone demand for %s", got[i].ID)
		}
		r := got[i].serviceBoundaryRestrictions[0]
		if err := validateEnergyPathServiceBoundaryRestriction(r); err != nil {
			t.Fatalf("invalid native metadata: %v %+v", err, r)
		}
		if r.PlantLoopName != "hot water loop" || r.DemandObjectName != "test pool" || r.Reason != energyPathBoundaryNonZoneDemand || !reflect.DeepEqual(r.ConsumerSourceIDs, []string{"sql-rdd-20"}) {
			t.Fatalf("invented or omitted actual HW source: %+v", r)
		}
		if got[i].Value != nodes[i].Value || !reflect.DeepEqual(got[i].SourceIDs, nodes[i].SourceIDs) {
			t.Fatal("restriction changed an observed budget")
		}
	}
	if len(got[3].serviceBoundaryRestrictions)+len(got[4].serviceBoundaryRestrictions) != 0 {
		t.Fatal("unrelated Cooling or exact local heater was globally blocked")
	}
	after, _ := json.Marshal(nodes)
	if string(before) != string(after) {
		t.Fatal("producer mutated caller nodes")
	}
	// Exact CW source does not inherit the broad Pump node's HW restriction.
	if r := projectEnergyPathServiceBoundaryRestrictions(got[2].serviceBoundaryRestrictions, []string{"sql-rdd-21"}, true); len(r) != 0 {
		t.Fatalf("CW inherits unrelated HW demand: %+v", r)
	}
	if r := projectEnergyPathServiceBoundaryRestrictions(got[2].serviceBoundaryRestrictions, []string{"sql-rdd-20"}, true); len(r) != 1 {
		t.Fatal("HW proof illegally removed its own non-Zone demand")
	}
}

func TestEnergyPathPoolActualResultWireKeepsZeroAndMissingSources(t *testing.T) {
	path, plan, context := energyPathPoolSQLHand(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex=51`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=52`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(legacy)
	for pass := 0; pass < 3; pass++ {
		zero := energyPathPoolSQLSource(t, result.Sources, "sql-rdd-51")
		missing := energyPathPoolSQLSource(t, result.Sources, "sql-rdd-52")
		for _, bit := range []uint8{energySourceObservedRaw, energySourceObservedEffective} {
			if !energyDataSourceValueKnown(zero, bit) || energyDataSourceValueKnown(missing, bit) {
				t.Fatalf("actual result wire changed zero/missing presence on pass%d: zero=%+v missing=%+v", pass, zero, missing)
			}
		}
		if zero.RawValue != 0 || zero.EffectiveValue != 0 || zero.EffectiveMultiplier != 1 {
			t.Fatal("native measured zero acquired a representative multiplier or positive value")
		}
		if pass == 2 {
			break
		}
		wire, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var reopened EnergyExplanationResult
		if err := json.Unmarshal(wire, &reopened); err != nil {
			t.Fatal(err)
		}
		result = reopened
	}
}

func TestEnergyPathPoolSQLActualTopologyKeepsHWUnassignedAndCWScoped(t *testing.T) {
	for _, policy := range []string{PurposeAllocationPolicyDirectOnly, PurposeAllocationPolicyByServicePathLoadShare} {
		for _, selected := range []bool{false, true} {
			path, plan, context := energyPathPoolSQLHand(t)
			plan.AllocationPolicy = policy
			if selected {
				plan.ZoneMode = "selected"
				plan.ZoneNames = []string{"SPACE2-1"}
			}
			legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
			if err != nil {
				t.Fatal(err)
			}
			legacy = enrichEnergyExplanationWithServicePaths(legacy, filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "5ZoneSwimmingPoolZoneMultipliers.idf"))
			result := UpgradeEnergyExplanationV1(legacy)
			for pass := 0; pass < 3; pass++ {
				checkZone := func(zone string, nodes []EnergyExplanationNode, links []EnergyPathLink) {
					t.Helper()
					pumps := 0.0
					for _, node := range nodes {
						if node.Level == "end_use" && node.EndUse == "heating" && node.Value != 0 {
							t.Fatalf("policy=%s selected=%v pass=%d zone=%s acquired unsupported HW heating %g", policy, selected, pass, zone, node.Value)
						}
						if node.Level == "end_use" && node.EndUse == "pumps" {
							pumps += node.Value
						}
						if node.Level == "carrier" && node.Carrier == "natural_gas" && node.Value != 0 {
							t.Fatal("shared boiler gas appeared as Zone input")
						}
					}
					want := 0.0
					if policy == PurposeAllocationPolicyByServicePathLoadShare && zone != "PLENUM-1" {
						want = 4
					}
					if pumps != want {
						t.Fatalf("policy=%s selected=%v pass=%d Zone=%s pumps=%g want%g", policy, selected, pass, zone, pumps, want)
					}
					for _, link := range links {
						if link.Relation == "load_to_end_use" && link.ServiceKind == "heating" {
							t.Fatal("Zone conversion recreated unsupported shared Heating")
						}
					}
				}
				if selected {
					checkZone(result.Scope.ZoneName, result.Nodes, result.Links)
				} else {
					for _, zone := range result.ZoneResults {
						checkZone(zone.Scope.ZoneName, zone.Nodes, zone.Links)
					}
					if len(result.ZoneResults) != 6 {
						t.Fatalf("actual all-Zone graph roster=%d want6", len(result.ZoneResults))
					}
					for _, link := range result.Links {
						if link.Relation == "load_to_end_use" && link.ServiceKind == "heating" {
							t.Fatal("Building ratio includes non-Zone pool consumption")
						}
					}
				}
				if pass == 2 {
					break
				}
				wire, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				var next EnergyExplanationResult
				if err := json.Unmarshal(wire, &next); err != nil {
					t.Fatal(err)
				}
				result = next
			}
		}
	}
}

func TestEnergyPathPoolBoundaryPersistsWhenOwnerIsExcludedOrUnresolved(t *testing.T) {
	doc := energyPathPoolOriginal(t)
	energyPathPoolTestObject(t, &doc, "SwimmingPool:Indoor", "Test Pool").Fields[1].Value = "missing surface"
	inventory := energyPathNativePoolInventory(doc)
	result := EnergyExplanationV1{scope: EnergyExplanationScope{Kind: "zone", ZoneName: "SPACE2-1"}, poolEvidence: energyPathPoolEvidence{Inventory: inventory},
		Nodes: []EnergyExplanationNode{{ID: "heating", Level: "energy", EndUse: "heating", Carrier: "natural_gas", SourceIDs: []string{"sql-rdd-1"}}}}
	result.Periods = []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: append([]EnergyExplanationNode(nil), result.Nodes...)}}
	applyEnergyPathPoolResultRestrictions(&result)
	for _, nodes := range [][]EnergyExplanationNode{result.Nodes, result.Periods[0].Nodes} {
		if len(nodes[0].serviceBoundaryRestrictions) != 1 {
			t.Fatal("scope/owner failure erased native presence")
		}
		r := nodes[0].serviceBoundaryRestrictions[0]
		if r.Reason != energyPathBoundaryTopologyIncomplete || r.PlantLoopName != "" || r.DemandObjectName != "test pool" || len(r.ConsumerSourceIDs) != 0 {
			t.Fatalf("unresolved ownership became positive identity/knownness: %+v", r)
		}
		if err := validateEnergyPathServiceBoundaryRestriction(r); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEnergyPathPoolSurfaceQualificationChangesInterpretationOnly(t *testing.T) {
	inventory := energyPathNativePoolInventory(energyPathPoolOriginal(t))
	sources := []EnergyDataSource{{ID: "sql-rdd-1", KeyValue: "F1-1", Name: "Surface Inside Face Conduction Heat Transfer Energy", RawValue: 10, EffectiveValue: 30, EffectiveMultiplier: 3},
		{ID: "sql-rdd-2", KeyValue: "F2-1", Name: "Surface Inside Face Conduction Heat Transfer Energy", RawValue: 8, EffectiveValue: 24, EffectiveMultiplier: 3}}
	got := qualifyEnergyPathPoolSurfaceSources(sources, inventory)
	if !strings.Contains(got[0].Explanation, "not an isolated passive floor") || got[0].RawValue != 10 || got[0].EffectiveValue != 30 || got[0].EffectiveMultiplier != 3 {
		t.Fatal("pool surface qualification altered or misrepresented physical values")
	}
	if !reflect.DeepEqual(got[1], sources[1]) || sources[0].Explanation != "" {
		t.Fatal("unrelated or original source mutated")
	}
}

func TestEnergyPathPoolBoundaryTabularFallbackCannotInventNativeProof(t *testing.T) {
	evidence := energyPathPoolEvidence{Inventory: energyPathNativePoolInventory(energyPathPoolOriginal(t))}
	nodes := []EnergyExplanationNode{{ID: "tabular-heating", Level: "energy", EndUse: "heating", Carrier: "natural_gas", Value: 100, SourceIDs: []string{"actual-tabular-source"}}}
	got := energyPathPoolRestrictedNodes(nodes, evidence)
	if len(got[0].serviceBoundaryRestrictions) != 1 || !energyPathServiceBoundaryDeniesConversion(got[0]) {
		t.Fatal("native Pool disappeared at non-RDD fallback")
	}
	r := got[0].serviceBoundaryRestrictions[0]
	if r.Reason != energyPathBoundaryTopologyIncomplete || len(r.MeterSourceIDs)+len(r.ConsumerSourceIDs) != 0 || got[0].Value != 100 {
		t.Fatalf("fallback fabricated native source or changed quantity: %+v", r)
	}
	if err := validateEnergyPathServiceBoundaryRestriction(r); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got[0].SourceIDs, nodes[0].SourceIDs) {
		t.Fatal("actual fallback provenance changed")
	}
}

func TestEnergyPathPoolUnreviewedFuelRetainsDenialWithoutInventingConsumption(t *testing.T) {
	doc := energyPathPoolOriginal(t)
	energyPathPoolTestObject(t, &doc, "Boiler:HotWater", "Central Boiler").Fields[1].Value = "Propane"
	evidence := energyPathPoolEvidence{Inventory: energyPathNativePoolInventory(doc)}
	for _, value := range []float64{20, 0} {
		for _, native := range []bool{true, false} {
			legacy := epathBoundaryLegacyFixture(value, PurposeAllocationPolicyByServicePathLoadShare)
			for i := range legacy.Nodes {
				if legacy.Nodes[i].Carrier == "natural_gas" {
					legacy.Nodes[i].Carrier = "propane"
				}
				legacy.Nodes[i].serviceBoundaryRestrictions = nil
			}
			if !native {
				legacy.Nodes[3].SourceIDs = []string{"tabular.heating.propane"}
			}
			legacy.Nodes = energyPathPoolRestrictedNodes(legacy.Nodes, evidence)
			r := legacy.Nodes[3].serviceBoundaryRestrictions[0]
			if r.Carrier != "propane" || r.Reason != energyPathBoundaryTopologyIncomplete || len(r.ConsumerSourceIDs) != 0 {
				t.Fatalf("unreviewed fuel borrowed reviewed consumption proof: %+v", r)
			}
			if err := validateEnergyPathServiceBoundaryRestriction(r); err != nil {
				t.Fatal(err)
			}
			legacy = applyEnergyExplanationV1ServicePathLoadShareAllocation(legacy)
			wire, err := json.Marshal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			var result EnergyExplanationResult
			if err := json.Unmarshal(wire, &result); err != nil {
				t.Fatal(err)
			}
			for pass := 0; pass < 3; pass++ {
				propane := 0.0
				for _, node := range result.Nodes {
					if node.Level == "carrier" && node.Carrier == "propane" {
						propane += node.Value
					}
				}
				if propane != value {
					t.Fatalf("unsupported fuel observation changed %g/%g", propane, value)
				}
				for _, link := range result.Links {
					if link.Relation == "load_to_end_use" && link.ServiceKind == "heating" {
						t.Fatal("unknown fuel restored false Heating conversion")
					}
				}
				for _, zone := range result.ZoneResults {
					for _, node := range zone.Nodes {
						if node.Level == "carrier" && node.Carrier == "propane" && node.Value != 0 {
							t.Fatal("unknown pool fuel was allocated to a Zone")
						}
					}
				}
				for _, item := range buildEnergyExplanationSummary(result).Ratios {
					if item.ServiceKind == "heating" {
						t.Fatal("summary restored unknown fuel ratio")
					}
				}
				if pass == 2 {
					break
				}
				wire, err = json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				var next EnergyExplanationResult
				if err := json.Unmarshal(wire, &next); err != nil {
					t.Fatal(err)
				}
				result = next
			}
		}
	}
}
