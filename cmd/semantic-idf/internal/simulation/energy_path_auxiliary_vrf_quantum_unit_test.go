package simulation

import (
	"fmt"
	"math"
	"reflect"
	"testing"
)

// Integer inequalities test the quota independently of the apportioner.
// Pruned zero edges are still zero recipients, not missing quota obligations.
func epathAuxVRFAssertSevenQuantumShares(t *testing.T, values map[string]float64, budget int64) {
	t.Helper()
	var sum int64
	for i := 0; i < 7; i++ {
		zone := fmt.Sprintf("Zone %02d", i)
		value := values[zone]
		milli := int64(math.Round(value * 1000))
		if !energyPathFinite(value) || value < 0 || value != float64(milli)/1000 {
			t.Fatalf("%s has invalid share %g", zone, value)
		}
		deviation := milli*7 - budget
		if deviation <= -7 || deviation >= 7 {
			t.Fatalf("%s absorbed another recipient's rounding: %g, deviation %d/7 milli-kWh", zone, value, deviation)
		}
		sum += milli
	}
	if sum != budget {
		t.Fatalf("positive graph edges total %d, budget %d milli-kWh", sum, budget)
	}
}

func TestEnergyPathAuxiliaryQuantumCallerBudgetsAndPermutation(t *testing.T) {
	for _, budget := range []int64{4, 2, 1003} {
		for _, mode := range []string{"fans", "fan_airflow", "pumps", "heat_rejection"} {
			t.Run(fmt.Sprintf("%s/%d", mode, budget), func(t *testing.T) {
				endUse, method := mode, "air_loop_load_share"
				switch mode {
				case "fan_airflow":
					endUse, method = "fans", "airflow_share"
				case "pumps":
					method = "plant_loop_load_share"
				case "heat_rejection":
					method = "condenser_loop_load_share"
				}
				var paths []energyPathAuxiliaryServicePath
				var loads []EnergyExplanationNode
				for i := 0; i < 7; i++ {
					zone, path := fmt.Sprintf("Zone %02d", i), fmt.Sprintf("path.%02d", i)
					paths = append(paths, epath101AuditPath(path, zone, "cooling", "Air", "Plant", "Condenser"))
					loads = append(loads, epath101AuditLoad(zone, "cooling", 1, fmt.Sprintf("load.%02d", i), []string{path}))
					if mode == "fan_airflow" {
						loads = append(loads, epath101AuditAirflow(zone, 1, fmt.Sprintf("airflow.%02d", i), []string{path}))
					}
				}
				// The central meter permits only the seven exact paths.
				nodes := append([]EnergyExplanationNode{
					epath101AuditAuxiliary(endUse, float64(budget)/1000, "paid", epath101AuditPathIDs(paths)),
				}, loads...)
				paths = append(paths, epath101AuditPath("rogue", "Rogue", "cooling", "OtherAir", "OtherPlant", "OtherCondenser"))
				nodes = append(nodes, epath101AuditLoad("Rogue", "cooling", 10000, "foreign", []string{"rogue"}))
				before := append([]EnergyExplanationNode(nil), nodes...)
				first := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, epath101AuditTopology(paths), "M1", "monthly", true)
				if !reflect.DeepEqual(nodes, before) {
					t.Fatal("auxiliary allocation changed input observations")
				}
				for i, j := 0, len(nodes)-1; i < j; i, j = i+1, j-1 {
					nodes[i], nodes[j] = nodes[j], nodes[i]
				}
				for i, j := 0, len(paths)-1; i < j; i, j = i+1, j-1 {
					paths[i], paths[j] = paths[j], paths[i]
				}
				second := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, epath101AuditTopology(paths), "M1", "monthly", true)
				if !reflect.DeepEqual(first, second) {
					t.Fatal("node/topology permutation changed semantic auxiliary tie order")
				}
				values := map[string]float64{}
				for _, edge := range first.Edges {
					if edge.ZoneName == "Rogue" || edge.Relation != energyPathAuxiliaryAllocationRelation ||
						edge.RuleID != energyRelationshipRuleAllocatedAuxiliaryServicePath ||
						edge.Basis != "service_path_allocation" || !stringSliceContains(edge.SourceIDs, "paid") ||
						stringSliceContains(edge.SourceIDs, "foreign") || len(edge.RelatedPathIDs) != 1 {
						t.Fatalf("auxiliary recipient/source/path boundary changed: %#v", edge)
					}
					if _, duplicate := values[edge.ZoneName]; duplicate {
						t.Fatal("duplicate physical Zone share")
					}
					values[edge.ZoneName] = edge.Value
				}
				epathAuxVRFAssertSevenQuantumShares(t, values, budget)
				if len(first.Records) != 1 {
					t.Fatalf("unexpected auxiliary ledger census: %d", len(first.Records))
				}
				row := first.Records[0]
				if row.ExpectedValue != float64(budget)/1000 || row.AllocatedValue != row.ExpectedValue ||
					row.DirectValue != 0 || row.UnassignedValue != 0 || row.OvermappedValue != 0 || row.Method != method {
					t.Fatalf("auxiliary graph and ledger do not close: %#v", row)
				}
			})
		}
	}
}

func TestEnergyPathVRFReservationQuantumCallerBudgetsAndPermutation(t *testing.T) {
	for _, budget := range []int64{4, 2, 1003} {
		t.Run(fmt.Sprintf("%d", budget), func(t *testing.T) {
			_, nodes, native := vrfReservationFixture()
			// Start from an exactly divisible generic budget: eight equal loads
			// receive 1 each. Native reservation then leaves the hand R budget
			// for seven non-VRF recipients; one native kWh stays unassigned.
			nodes[0].Value, nodes[0].Unit = 8, "kWh"
			nodes[0].SourceIDs, nodes[0].RelatedPathIDs = []string{"broad"}, []string{"served"}
			nodes[1].Value, nodes[1].Unit = 50, "kWh"
			nodes[1].SourceIDs, nodes[1].RelatedPathIDs = []string{"gas-meter"}, []string{"gas-path"}
			for i := 0; i < 7; i++ {
				nodes = append(nodes, epath101AuditLoad(fmt.Sprintf("Zone %02d", i), "cooling", 1, fmt.Sprintf("load.%02d", i), []string{"served"}))
			}
			nodes = append(nodes,
				epath101AuditLoad("VRF", "cooling", 1, "load-vrf", []string{"served"}),
				epath101AuditLoad("Elsewhere", "heating", 50, "load-gas", []string{"gas-path"}))
			native.Services[0].DirectValue = 1
			native.Services[0].SharedValue = 7 - float64(budget)/1000
			native.Services[0].AllocatedValue = 6 - float64(budget)/1000
			beforeNative := native
			beforeNative.Services = append([]energyPathVRFServiceAllocation(nil), native.Services...)
			beforeNative.Zones = append([]energyPathVRFZoneAllocation(nil), native.Zones...)
			build := func(input []EnergyExplanationNode) energyPathZoneHVACAllocationPlan {
				// Use production's caller sequence, not an artificially shuffled
				// reservation plan: both prior boundaries sort semantic edge IDs.
				plan := buildEnergyPathZoneHVACAllocationPlan(input, nil, nil, "M1", "monthly", true)
				plan = reserveEnergyPathHVACConsumptionPools(plan, input, energyServicePathIndex{}, nil, "M1", "monthly", true)
				return reserveEnergyPathVRFAllocation(plan, input, native, "M1")
			}
			first := build(nodes)
			for i, j := 0, len(nodes)-1; i < j; i, j = i+1, j-1 {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
			second := build(nodes)
			if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(native, beforeNative) {
				t.Fatal("input order changed reservation or native source budgets")
			}
			values := map[string]float64{}
			gasSeen := false
			for _, edge := range first.Edges {
				if edge.FromID == "gas" {
					if edge.ZoneName != "Elsewhere" || edge.Value != 50 {
						t.Fatal("unrelated gas group changed")
					}
					gasSeen = true
					continue
				}
				if edge.FromID != "cooling" || edge.ZoneName == "VRF" || edge.Basis != "service_path_allocation" ||
					!stringSliceContains(edge.SourceIDs, "broad") || stringSliceContains(edge.SourceIDs, "local") ||
					stringSliceContains(edge.SourceIDs, "outdoor") || !reflect.DeepEqual(edge.RelatedPathIDs, []string{"served"}) {
					t.Fatalf("VRF-owned/source-local boundary changed: %#v", edge)
				}
				if _, duplicate := values[edge.ZoneName]; duplicate {
					t.Fatal("duplicate non-VRF recipient")
				}
				values[edge.ZoneName] = edge.Value
			}
			epathAuxVRFAssertSevenQuantumShares(t, values, budget)
			if !gasSeen || len(first.Records) != 2 {
				t.Fatal("unrelated group or reservation ledger lost")
			}
			for _, row := range first.Records {
				switch row.ServiceKind {
				case "cooling":
					if row.ExpectedValue != 8 || row.DirectValue != 1 || row.AllocatedValue != 6 ||
						row.UnassignedValue != 1 || row.OvermappedValue != 0 {
						t.Fatalf("owned unassigned native consumption leaked into generic shares: %#v", row)
					}
				case "heating":
					if row.Carrier != "natural_gas" || row.ExpectedValue != 50 || row.AllocatedValue != 50 ||
						row.DirectValue != 0 || row.UnassignedValue != 0 || row.OvermappedValue != 0 {
						t.Fatalf("unrelated gas ledger changed: %#v", row)
					}
				default:
					t.Fatalf("unexpected ledger: %#v", row)
				}
			}
		})
	}
}
