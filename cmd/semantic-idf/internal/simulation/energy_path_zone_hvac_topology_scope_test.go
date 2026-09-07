package simulation

import (
	"encoding/json"
	"math"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// These tests deliberately use existing entrypoints. The private topology
// argument must be propagated through both the runtime allocation pass and the
// V1->V2 projection; testing a new planner directly would miss either omission.
func TestEnergyPathZoneHVACTopologyKnownRecipientsCannotEscape(t *testing.T) {
	index, served := epathHVACTopologyLargeOffice(t)
	for _, retainZeroNodes := range []bool{true, false} {
		name := "reported_zero_recipient_nodes"
		if !retainZeroNodes {
			name = "no_current_recipient_nodes_must_not_invent_zero_or_lose_topology"
		}
		t.Run(name, func(t *testing.T) {
			// M7/M8 meter amounts are the observed LargeOffice values at the
			// canonical monthly 0.001-kWh precision. M6 uses deliberate effective
			// test loads of 1 each, so its 80-kWh control allocation is 5 per Zone.
			// The graph can omit an observed zero; it can also have no observation.
			// Neither absence proves that the known topology has disappeared.
			monthly := []EnergyPeriod{
				epathHVACTopologyPeriod(index, served, "M6", 80, 1, true, []float64{10, 10, 10}),
				epathHVACTopologyPeriod(index, served, "M7", 375.594, 0, retainZeroNodes, []float64{9.568, 139.335, 41.997}),
				epathHVACTopologyPeriod(index, served, "M8", 1393.570, 0, retainZeroNodes, []float64{7.996, 108.865, 82.260}),
			}
			input := epathHVACTopologyMonthlyFixture(index, monthly)
			before, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			allocated := applyEnergyExplanationV1ServicePathLoadShareAllocation(input)
			result := UpgradeEnergyExplanationV1(allocated)
			const total = 1849.164
			const unassigned = 1769.164
			epathHVACTopologyAssertLedger(t, result.Reconciliation, "annual", total, 80, unassigned)
			for _, period := range result.Periods {
				switch period.ID {
				case "M6":
					epathHVACTopologyAssertLedger(t, period.Reconciliation, "M6", 80, 80, 0)
				case "M7":
					epathHVACTopologyAssertLedger(t, period.Reconciliation, "M7", 375.594, 0, 375.594)
				case "M8":
					epathHVACTopologyAssertLedger(t, period.Reconciliation, "M8", 1393.570, 0, 1393.570)
				}
			}
			for _, id := range []string{"end_use.heating.building", "carrier.natural_gas.building"} {
				if node := epath094AuditNode(result.Nodes, id); node == nil || math.Abs(node.Value-total) > 1e-8 {
					t.Errorf("Building truth changed while retaining unassigned energy: %s = %#v", id, node)
				}
			}
			if !epath100AuditHasWarning(result.Warnings, "unassigned_building_hvac_energy") {
				t.Error("known connected zero-load months lost the Building unassigned warning")
			}
			for _, zoneName := range append(append([]string(nil), served...), epathHVACTopologyPlenums...) {
				zone := epath100AuditZoneResult(result.ZoneResults, zoneName)
				if zone == nil {
					t.Errorf("missing Zone scope %s", zoneName)
					continue
				}
				isServed := !stringSliceContains(epathHVACTopologyPlenums, zoneName)
				annual := epath094AuditNode(zone.Nodes, "end_use.heating."+metricID(zoneName))
				if isServed {
					if annual == nil || annual.Value != 5 {
						t.Errorf("%s annual allocation must be M6 5 only, not a new annual-share redistribution: %#v", zoneName, annual)
					}
				} else if annual != nil {
					t.Errorf("unserved plenum received annual central heating: %s value=%g basis=%s", zoneName, annual.Value, annual.Basis)
				}
				for _, period := range zone.Periods {
					if period.ID != "M7" && period.ID != "M8" {
						continue
					}
					if node := epath094AuditNode(period.Nodes, "end_use.heating."+metricID(zoneName)); node != nil {
						t.Errorf("%s %s must not receive central heating or a fabricated zero end-use node: value=%g basis=%s", zoneName, period.ID, node.Value, node.Basis)
					}
					if isServed && !retainZeroNodes && epath094AuditNode(period.Nodes, "load.heating."+metricID(zoneName)) != nil {
						t.Errorf("%s %s topology metadata fabricated a zero load observation", zoneName, period.ID)
					}
				}
				if epath100AuditHasWarning(zone.Warnings, "unassigned_building_hvac_energy") {
					t.Errorf("Building warning was relabeled as local Zone consumption: %s", zoneName)
				}
			}
			// Exercise direct selected-scope projection as well as the nested
			// ZoneResults adapter; the unserved Zone keeps its thermal data only.
			selectedInput := input
			selectedInput.scope = EnergyExplanationScope{Kind: "zone", ZoneName: epathHVACTopologyPlenums[0], AggregationBasis: "model_total"}
			selected := UpgradeEnergyExplanationV1(applyEnergyExplanationV1ServicePathLoadShareAllocation(selectedInput))
			if node := epath094AuditNode(selected.Nodes, "end_use.heating."+metricID(selected.Scope.ZoneName)); node != nil {
				t.Errorf("direct selected Zone projection escaped topology: value=%g basis=%s", node.Value, node.Basis)
			}
			after, err := json.Marshal(input)
			if err != nil || string(before) != string(after) {
				t.Errorf("allocation mutated the original input: %v", err)
			}
		})
	}
}

func TestEnergyPathZoneHVACTopologyExplicitSourcePathsRemainAuthoritative(t *testing.T) {
	for _, value := range []float64{0, 20} {
		name := "known_path_zero"
		if value > 0 {
			name = "known_path_positive"
		}
		t.Run(name, func(t *testing.T) {
			// Even without a runtime topology index, a source's explicit path
			// restricts its denominator. An unrelated positive target is not a
			// substitute when that source's only known recipient reports zero.
			nodes := []EnergyExplanationNode{
				epath100AuditCarrierNode("natural_gas", 80, "annual", "facility.gas"),
				epath100AuditEndUseNode("heating", "natural_gas", 80, "annual", "meter.gas", []string{"heating.office"}),
				epath100AuditLoadNode("Office", "heating", value, "annual", "load.office", []string{"heating.office"}),
				epath100AuditLoadNode("Unserved", "heating", 1000, "annual", "load.unserved", []string{"heating.unrelated"}),
			}
			input := epathHVACTopologyAnnualFixture(energyServicePathIndex{}, nodes)
			result := UpgradeEnergyExplanationV1(applyEnergyExplanationV1ServicePathLoadShareAllocation(input))
			allocated, unassigned := 0.0, 80.0
			if value > 0 {
				allocated, unassigned = 80, 0
			}
			epathHVACTopologyAssertLedger(t, result.Reconciliation, "annual", 80, allocated, unassigned)
			for _, zone := range result.ZoneResults {
				node := epath094AuditNode(zone.Nodes, "end_use.heating."+metricID(zone.Scope.ZoneName))
				if zone.Scope.ZoneName == "Office" && value > 0 {
					if node == nil || node.Value != 80 || node.Basis != "service_path_allocation" {
						t.Errorf("positive exact-path recipient changed: %#v", node)
					}
				} else if node != nil {
					t.Errorf("source path restriction escaped to %s: value=%g basis=%s", zone.Scope.ZoneName, node.Value, node.Basis)
				}
			}
		})
	}
}

func TestEnergyPathZoneHVACTopologyMissingStillAllowsDocumentedFallback(t *testing.T) {
	nodes := []EnergyExplanationNode{
		epath100AuditCarrierNode("natural_gas", 50, "annual", "facility.gas"),
		epath100AuditEndUseNode("heating", "natural_gas", 50, "annual", "meter.gas", nil),
		epath100AuditLoadNode("Office", "heating", 10, "annual", "load.office", nil),
		epath100AuditLoadNode("Lab", "heating", 40, "annual", "load.lab", nil),
		epath100AuditLoadNode("CoolingOnly", "cooling", 950, "annual", "load.cooling", nil),
	}
	result := UpgradeEnergyExplanationV1(applyEnergyExplanationV1ServicePathLoadShareAllocation(epathHVACTopologyAnnualFixture(energyServicePathIndex{}, nodes)))
	epathHVACTopologyAssertLedger(t, result.Reconciliation, "annual", 50, 50, 0)
	for _, item := range []struct {
		zone string
		want float64
	}{{"Office", 10}, {"Lab", 40}} {
		zone := epath100AuditZoneResult(result.ZoneResults, item.zone)
		if zone == nil {
			t.Fatalf("missing fallback Zone %s", item.zone)
		}
		node := epath094AuditNode(zone.Nodes, "end_use.heating."+metricID(item.zone))
		if node == nil || node.Value != item.want || node.Basis != "zone_load_allocation" {
			t.Errorf("genuinely missing topology lost documented same-service fallback: %s = %#v", item.zone, node)
		}
	}
	if zone := epath100AuditZoneResult(result.ZoneResults, "CoolingOnly"); zone != nil && epath094AuditNode(zone.Nodes, "end_use.heating.coolingonly") != nil {
		t.Error("missing topology allowed cross-service allocation")
	}
}

func TestEnergyPathZoneHVACTopologyMixedDeliveryIsNotVentilationFallback(t *testing.T) {
	index := energyServicePathIndex{
		byZoneService: map[string][]string{
			"office|mixed": {"office.mixed"}, "office|ventilation": {"office.vent"},
			"ventonly|ventilation": {"ventonly.vent"},
		},
		byZone: map[string][]string{"office": {"office.mixed", "office.vent"}, "ventonly": {"ventonly.vent"}},
	}
	for _, service := range []string{"cooling", "heating"} {
		for _, value := range []float64{0, 20} {
			name := service + "/known_zero"
			if value > 0 {
				name = service + "/known_positive"
			}
			t.Run(name, func(t *testing.T) {
				nodes := []EnergyExplanationNode{
					epath100AuditCarrierNode("electricity", 40, "annual", "facility"),
					epath100AuditEndUseNode(service, "electricity", 40, "annual", "meter", nil),
					epath100AuditLoadNode("Office", service, value, "annual", "office.load", []string{"office.mixed", "office.vent"}),
					epath100AuditLoadNode("VentOnly", service, 1000, "annual", "ventonly.load", []string{"ventonly.vent"}),
				}
				edges := []EnergyExplanationEdge{epath100AuditMeterEdge("annual", nodes[0], nodes[1])}
				for _, load := range nodes[2:] {
					edges = append(edges, epath100AuditDeliveredEdge("annual", nodes[1], load, service, load.RelatedPathIDs))
				}
				plan := buildEnergyPathZoneHVACAllocationPlan(nodes, edges, nil, "annual", "annual", false, index)
				if len(plan.Records) != 1 {
					t.Fatalf("missing allocation record: %#v", plan)
				}
				if value == 0 {
					if len(plan.Edges) != 0 || plan.Records[0].AllocatedValue != 0 || plan.Records[0].UnassignedValue != 40 {
						t.Fatal("zero mixed recipient escaped into a ventilation-only Zone")
					}
				} else if len(plan.Edges) != 1 || plan.Edges[0].ToID != nodes[2].ID || plan.Edges[0].Value != 40 || !reflect.DeepEqual(plan.Edges[0].RelatedPathIDs, []string{"office.mixed"}) || plan.Records[0].UnassignedValue != 0 {
					t.Fatalf("known mixed delivery lost its typed service allocation: %#v", plan)
				}
			})
		}
	}
}

var epathHVACTopologyPlenums = []string{"GroundFloor_Plenum", "MidFloor_Plenum", "TopFloor_Plenum"}

func epathHVACTopologyLargeOffice(t *testing.T) (energyServicePathIndex, []string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate checked-in model")
	}
	index := buildEnergyServicePathIndex(filepath.Join(filepath.Dir(file), "testdata", "energy_path_real_models", "models", "25.1", "RefBldgLargeOfficeNew2004_Chicago.idf"))
	served := []string{"Basement", "Core_bottom", "Core_mid", "Core_top"}
	for _, floor := range []string{"bot", "mid", "top"} {
		for _, number := range []string{"1", "2", "3", "4"} {
			served = append(served, "Perimeter_"+floor+"_ZN_"+number)
		}
	}
	wanted := map[string]bool{}
	for _, zone := range served {
		key := normalizePurposeToken(zone) + "|heating"
		wanted[key] = true
		if len(index.byZoneService[key]) == 0 {
			t.Fatalf("actual model lacks expected connected heating recipient %s", zone)
		}
	}
	actual := map[string]bool{}
	for key := range index.byZoneService {
		if strings.HasSuffix(key, "|heating") {
			actual[key] = true
		}
	}
	if len(served) != 16 || !reflect.DeepEqual(actual, wanted) {
		t.Fatalf("actual heating recipient set changed: got=%v want=%v", actual, wanted)
	}
	for _, plenum := range epathHVACTopologyPlenums {
		if len(index.byZoneService[normalizePurposeToken(plenum)+"|heating"]) != 0 {
			t.Fatalf("return plenum unexpectedly became a heating service recipient: %s", plenum)
		}
	}
	return index, served
}

func epathHVACTopologyPeriod(index energyServicePathIndex, served []string, period string, meter, load float64, retainZero bool, plenumValues []float64) EnergyPeriod {
	nodes := []EnergyExplanationNode{
		epath100AuditCarrierNode("natural_gas", meter, period, "facility.gas"),
		epath100AuditEndUseNode("heating", "natural_gas", meter, period, "meter.gas", nil),
	}
	for _, zone := range served {
		if load > 0 || retainZero {
			nodes = append(nodes, epath100AuditLoadNode(zone, "heating", load, period, "load."+metricID(zone), index.byZoneService[normalizePurposeToken(zone)+"|heating"]))
		}
	}
	for i, zone := range epathHVACTopologyPlenums {
		nodes = append(nodes, epath100AuditLoadNode(zone, "heating", plenumValues[i], period, "load."+metricID(zone), nil))
	}
	fixture := epathHVACTopologyAnnualFixture(index, nodes)
	return EnergyPeriod{ID: period, Kind: "monthly", Nodes: fixture.Nodes, Edges: fixture.Edges}
}

func epathHVACTopologyAnnualFixture(index energyServicePathIndex, nodes []EnergyExplanationNode) EnergyExplanationV1 {
	period := nodes[0].Period
	edges := []EnergyExplanationEdge{epath100AuditMeterEdge(period, nodes[0], nodes[1])}
	sources := []EnergyDataSource{}
	for _, node := range nodes {
		for _, sourceID := range node.SourceIDs {
			sources = append(sources, EnergyDataSource{ID: sourceID, ZoneName: node.ZoneName})
		}
		if node.Level == "load" && node.Value > 0 {
			edges = append(edges, epath100AuditDeliveredEdge(period, nodes[1], node, "heating", node.RelatedPathIDs))
		}
	}
	return EnergyExplanationV1{Schema: energyExplanationV1Schema, Purpose: string(SimulationPurposeBasicEnergy), Frequency: "annual", AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare, Nodes: nodes, Edges: edges, Sources: sources, servicePathIndex: index}
}

func epathHVACTopologyMonthlyFixture(index energyServicePathIndex, periods []EnergyPeriod) EnergyExplanationV1 {
	byID := map[string]EnergyExplanationNode{}
	for _, period := range periods {
		for _, node := range period.Nodes {
			if prior, ok := byID[node.ID]; ok {
				node.Value += prior.Value
			}
			node.RawValue, node.EffectiveValue, node.Period = node.Value, node.Value, "annual"
			byID[node.ID] = node
		}
	}
	// The fixture builder requires the authoritative carrier/end-use first.
	nodes := []EnergyExplanationNode{byID["energy.carrier.natural_gas"], byID["energy.end_use.heating.natural_gas"]}
	keys := []string{}
	for key, node := range byID {
		if node.Level == "load" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		nodes = append(nodes, byID[key])
	}
	input := epathHVACTopologyAnnualFixture(index, nodes)
	input.Frequency, input.Periods, input.canonicalMonthlyBasis = "monthly", periods, true
	return input
}

func epathHVACTopologyAssertLedger(t *testing.T, rows []EnergyReconciliation, period string, expected, allocated, unassigned float64) {
	t.Helper()
	row := epath100AuditAllocationReconciliation(rows, "heating", "natural_gas", period)
	if row == nil {
		t.Errorf("missing %s allocation ledger", period)
		return
	}
	if math.Abs(row.ExpectedValue-expected) > 1e-8 || row.DirectValue != 0 || math.Abs(row.AllocatedValue-allocated) > 1e-8 || math.Abs(row.UnassignedValue-unassigned) > 1e-8 || math.Abs(row.ResidualValue-unassigned) > 1e-8 || math.Abs(row.ExplainedValue-allocated) > 1e-8 {
		t.Errorf("%s topology-bounded ledger: expected/direct/allocated/unassigned/residual/explained=%g/%g/%g/%g/%g/%g; want %g/0/%g/%g/%g/%g", period, row.ExpectedValue, row.DirectValue, row.AllocatedValue, row.UnassignedValue, row.ResidualValue, row.ExplainedValue, expected, allocated, unassigned, unassigned, allocated)
	}
	if unassigned > 0 && row.Status != "partial" || unassigned == 0 && row.Status != "balanced" {
		t.Errorf("%s unassigned coverage status=%s", period, row.Status)
	}
}
