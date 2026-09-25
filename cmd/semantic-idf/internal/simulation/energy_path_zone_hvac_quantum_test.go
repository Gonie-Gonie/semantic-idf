package simulation

import (
	"fmt"
	"math"
	"reflect"
	"testing"
)

// Integer quotas independently constrain each recipient without reproducing
// the production apportioner. The last Zone may not absorb everyone else's
// rounding error, even when the overall total happens to close.
func TestEnergyPathZoneHVACProportionalOwnQuantum(t *testing.T) {
	for _, tc := range []struct {
		name    string
		budget  int64
		weights []int64
	}{
		{"sixteen equal", 1000, []int64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}},
		{"tiny positive pool", 4, []int64{1, 1, 1, 1, 1, 1, 1}},
		{"zero weight holes", 1003, []int64{0, 1, 1, 1, 1, 0, 1, 1, 1, 0}},
		{"unequal", 2702, []int64{8283, 95198, 137978, 12781, 135260, 29871}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			weights := make([]float64, len(tc.weights))
			var denominator int64
			for i, w := range tc.weights {
				weights[i] = float64(w)
				denominator += w
			}
			got := energyPathZoneHVACProportionalValues(float64(tc.budget)/1000, weights)
			if len(got) != len(weights) {
				t.Fatalf("lost positive pool: %v", got)
			}
			var sum int64
			for i, value := range got {
				milli := int64(math.Round(value * 1000))
				if !energyPathFinite(value) || value < 0 || value != float64(milli)/1000 {
					t.Fatalf("invalid recipient %d: %g", i, value)
				}
				if tc.weights[i] == 0 && milli != 0 {
					t.Fatalf("zero weight received %g", value)
				}
				deviation := milli*denominator - tc.budget*tc.weights[i]
				if deviation <= -denominator || deviation >= denominator {
					t.Fatalf("recipient %d absorbed another Zone's rounding: %g, deviation %d/%d milli-kWh", i, value, deviation, denominator)
				}
				sum += milli
			}
			if sum != tc.budget {
				t.Fatalf("allocated %d, budget %d milli-kWh", sum, tc.budget)
			}
		})
	}
}

func TestEnergyPathZoneHVACQuantumServicePlanStableRecipientsAndSources(t *testing.T) {
	for _, service := range []string{"cooling", "heating"} {
		t.Run(service, func(t *testing.T) {
			nodes := []EnergyExplanationNode{{ID: "meter", Level: "energy", Kind: "energy." + service, EndUse: service, Carrier: "electricity", Value: 1, EffectiveValue: 1, Unit: "kWh", SourceIDs: []string{"paid"}, RelatedPathIDs: []string{"served"}}}
			for i := 0; i < 16; i++ {
				zone := fmt.Sprintf("Zone %02d", i)
				nodes = append(nodes, EnergyExplanationNode{ID: fmt.Sprintf("load.%02d", i), Level: "load", Kind: "load." + service, ServiceKind: service, ZoneName: zone, Value: 1, EffectiveValue: 1, Unit: "kWh", SourceIDs: []string{fmt.Sprintf("source.%02d", i)}, RelatedPathIDs: []string{"served"}})
			}
			// A large, physically unserved load must not dilute these quotas.
			nodes = append(nodes, EnergyExplanationNode{ID: "unserved", Level: "load", Kind: "load." + service, ServiceKind: service, ZoneName: "Unserved", Value: 10000, EffectiveValue: 10000, Unit: "kWh", RelatedPathIDs: []string{"other"}, SourceIDs: []string{"foreign"}})
			before := append([]EnergyExplanationNode(nil), nodes...)
			first := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, nil, "M1", "monthly", true)
			if !reflect.DeepEqual(nodes, before) {
				t.Fatal("allocation changed raw input nodes")
			}
			for i, j := 0, len(nodes)-1; i < j; i, j = i+1, j-1 {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
			second := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, nil, "M1", "monthly", true)
			if !reflect.DeepEqual(first, second) {
				t.Fatal("input order changed stable semantic allocation")
			}
			if len(first.Edges) != 16 || len(first.Records) != 1 {
				t.Fatalf("wrong recipient/ledger census: %d/%d", len(first.Edges), len(first.Records))
			}
			var sum int64
			for _, edge := range first.Edges {
				milli := int64(math.Round(edge.Value * 1000))
				if milli != 62 && milli != 63 {
					t.Fatalf("recipient %s got %g, outside its own 1/16 quota", edge.ZoneName, edge.Value)
				}
				if edge.ToID == "unserved" || edge.Basis != "service_path_allocation" || edge.Relation != "allocation" || !reflect.DeepEqual(edge.RelatedPathIDs, []string{"served"}) || !stringSliceContains(edge.SourceIDs, "paid") || stringSliceContains(edge.SourceIDs, "foreign") {
					t.Fatalf("changed service/source boundary: %#v", edge)
				}
				sum += milli
			}
			r := first.Records[0]
			if sum != 1000 || r.ExpectedValue != 1 || r.DirectValue != 0 || r.AllocatedValue != 1 || r.UnassignedValue != 0 || r.OvermappedValue != 0 {
				t.Fatalf("native budget no longer closes: %d %#v", sum, r)
			}
		})
	}
}

func TestEnergyPathZoneHVACQuantumInvalidAndExtremeInputs(t *testing.T) {
	for _, total := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if got := energyPathZoneHVACProportionalValues(total, []float64{1}); got != nil {
			t.Fatalf("invalid budget %g allocated: %v", total, got)
		}
	}
	for _, weights := range [][]float64{nil, {0, -1, math.NaN(), math.Inf(1)}, {math.MaxFloat64, math.MaxFloat64}} {
		if got := energyPathZoneHVACProportionalValues(1, weights); got != nil {
			t.Fatalf("unknown denominator allocated: %v", got)
		}
	}
	if got := energyPathZoneHVACProportionalValues(1, []float64{-1, math.NaN(), math.Inf(1), 0, 1}); !reflect.DeepEqual(got, []float64{0, 0, 0, 0, 1}) {
		t.Fatalf("ineligible weight gained allocation: %v", got)
	}
	// Preserve the existing extreme-range fallback; no milli-unit precision
	// guarantee is made above the shared apportioner's 2^53 budget limit.
	const milli int64 = 1<<53 + 2
	total := float64(milli) / 1000
	if got := energyPathZoneHVACProportionalValues(total, []float64{1}); len(got) != 1 || got[0] != roundedEnergyNumber(total) {
		t.Fatalf("finite extreme silently lost its allocation: %v", got)
	}
}

func TestEnergyPathZoneHVACQuantumTinyLedgerMatchesEmittedEdges(t *testing.T) {
	for _, budget := range []int64{2, 4, 1003} {
		t.Run(fmt.Sprint(budget), func(t *testing.T) {
			total := float64(budget) / 1000
			nodes := []EnergyExplanationNode{{ID: "meter", Level: "energy", Kind: "energy.cooling", EndUse: "cooling", Carrier: "electricity", Value: total, EffectiveValue: total, Unit: "kWh", SourceIDs: []string{"paid"}, RelatedPathIDs: []string{"served"}}}
			for i := 0; i < 7; i++ {
				nodes = append(nodes, EnergyExplanationNode{ID: fmt.Sprintf("load.%d", i), Level: "load", Kind: "load.cooling", ServiceKind: "cooling", ZoneName: fmt.Sprintf("Zone %d", i), Value: 1, EffectiveValue: 1, Unit: "kWh", RelatedPathIDs: []string{"served"}})
			}
			plan := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, nil, "M1", "monthly", true)
			var emitted int64
			for _, edge := range plan.Edges {
				milli := int64(math.Round(edge.Value * 1000))
				deviation := milli*7 - budget
				if edge.Value <= 0 || deviation <= -7 || deviation >= 7 {
					t.Fatalf("invalid positive recipient quota: %#v", edge)
				}
				emitted += milli
			}
			if len(plan.Records) != 1 || emitted != budget || plan.Records[0].AllocatedValue != total || plan.Records[0].UnassignedValue != 0 || plan.Records[0].OvermappedValue != 0 {
				t.Fatalf("positive graph edges and ledger differ: budget %d, emitted %d, records %#v", budget, emitted, plan.Records)
			}
		})
	}
}
