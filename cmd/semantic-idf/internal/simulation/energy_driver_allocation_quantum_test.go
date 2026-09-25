package simulation

import (
	"fmt"
	"math"
	"reflect"
	"testing"
)

// Integer pressure units make the independent one-quantum bound exact:
// abs(allocatedMilli*sumPressure - budgetMilli*pressure) < sumPressure.
// This does not reproduce the production largest-remainder algorithm.
func TestEPATH080EveryDriverOwnQuantumAndExactBudget(t *testing.T) {
	for _, test := range []struct {
		name     string
		budget   int64
		pressure []int64
	}{
		// Old last-remainder code gives the seventh driver .145 instead of a
		// floor/ceil of 1.003/7, accumulating six other branches' errors.
		{"seven equal", 1003, []int64{1000, 1000, 1000, 1000, 1000, 1000, 1000}},
		{"tiny budget", 2, []int64{1000, 1000, 1000, 1000, 1000, 1000, 1000}},
		// Observed 3dp pressure inputs reproduce the three diagnostic shapes.
		// They are allocation-unit inputs only, not accepted fixture expectations;
		// every expected bound is calculated independently from the integer quota.
		{"six unequal", 2702, []int64{8283, 95198, 137978, 12781, 135260, 29871}},
		{"seven unequal", 4507, []int64{8212, 109, 74030, 107299, 9960, 107547, 21842}},
		{"nine unequal", 417434, []int64{170668, 69110, 67244, 93303, 9009, 106985, 45569, 39716, 5406}},
	} {
		for _, service := range []string{"cooling", "heating"} {
			t.Run(test.name+"/"+service, func(t *testing.T) {
				var pressureSum int64
				for _, value := range test.pressure {
					pressureSum += value
				}
				var baseline map[string]float64
				for shift := range test.pressure {
					for _, reverse := range []bool{false, true} {
						nodes, loads, identities := epath080QuantumNodes(test.budget, test.pressure, service, shift, reverse)
						before := map[string]EnergyExplanationNode{}
						for id, entry := range nodes {
							before[id] = entry.node
						}
						if !allocateCanonicalEnergyDriverNodes(nodes, loads, true) {
							t.Fatal("canonical allocation was not applied")
						}
						var sum int64
						values := map[string]float64{}
						for index, id := range identities {
							node := nodes[id].node
							milli := int64(math.Round(node.Value * 1000))
							if node.Value != float64(milli)/1000 || milli < 0 {
								t.Fatalf("not a nonnegative exact 3dp allocation: %#v", node)
							}
							deviation := milli*pressureSum - test.budget*test.pressure[index]
							if deviation <= -pressureSum || deviation >= pressureSum {
								t.Fatalf("%s accumulated another branch's rounding: milli=%d quota=%d/%d deviation=%d/%d", id, milli, test.budget*test.pressure[index], pressureSum, deviation, pressureSum)
							}
							original := before[id]
							if node.RawValue != original.RawValue || node.EffectiveValue != original.EffectiveValue || node.SignedValue != original.SignedValue || node.ServiceKind != original.ServiceKind || node.Unit != original.Unit || !reflect.DeepEqual(node.SourceIDs, original.SourceIDs) {
								t.Fatalf("allocation changed signed raw/effective/source identity: before=%#v after=%#v", original, node)
							}
							if !node.AllocationApplied || node.AllocatedValue != node.Value || node.DisplayValue != node.Value || node.Basis != "heat_balance_share" || node.AllocationExplanation != energyDriverAllocationExplanation {
								t.Fatalf("allocation contract changed: %#v", node)
							}
							if test.name == "seven equal" {
								want := int64(143)
								if index < 2 {
									want++ // tied residual units belong to the two lowest IDs
								}
								if milli != want {
									t.Fatalf("stable semantic tie: %s=%d, want%d", id, milli, want)
								}
							}
							sum += milli
							values[id] = node.Value
						}
						if sum != test.budget || !reflect.DeepEqual(nodes["load"].node.SourceIDs, before["load"].SourceIDs) || nodes["load"].node.Value != before["load"].Value {
							t.Fatalf("load budget changed: allocated=%d budget=%d", sum, test.budget)
						}
						for _, id := range []string{"opposite", "zero", "synthetic", "unrelated"} {
							if nodes[id].node.AllocatedValue != 0 || nodes[id].node.SignedValue != before[id].SignedValue {
								t.Fatalf("ineligible %s entered denominator or lost its sign", id)
							}
						}
						if baseline == nil {
							baseline = values
						} else if !reflect.DeepEqual(values, baseline) {
							t.Fatalf("input/map order changed allocation: %v versus %v", values, baseline)
						}
					}
				}
			})
		}
	}
}

func TestEPATH080QuantumRejectedExtremeRetainsPriorAllocation(t *testing.T) {
	// The existing apportioner deliberately refuses budgets above 2^53
	// milli-units. That must not silently erase an otherwise positive driver.
	// No one-quantum precision claim is made beyond that representable range.
	const budget int64 = 1<<53 + 2
	for _, service := range []string{"cooling", "heating"} {
		nodes, loads, ids := epath080QuantumNodes(budget, []int64{1000}, service, 0, false)
		load := nodes["load"].node.Value
		if energyPathFanPoolShares(load, []energyPathZoneAuxiliaryTarget{{NodeID: ids[0], Value: 1}}) != nil {
			t.Fatal("fixture no longer exercises the existing apportioner's upper bound")
		}
		allocateCanonicalEnergyDriverNodes(nodes, loads, true)
		if got := nodes[ids[0]].node.AllocatedValue; got <= 0 || got != roundedEnergyNumber(load) {
			t.Fatalf("rejected extreme silently lost its old allocation: got%g load%g", got, load)
		}
	}
}

func TestEPATH080QuantumAllocationKeepsZeroAndNoPressureDistinct(t *testing.T) {
	for _, service := range []string{"cooling", "heating"} {
		for _, budget := range []int64{0, 7} {
			nodes, loads, ids := epath080QuantumNodes(budget, []int64{1000}, service, 0, false)
			if budget > 0 {
				// Opposite-sign pressure is not permission to invent a matched
				// pressure. Only the actual positive load creates the fallback.
				nodes[ids[0]].node.SignedValue *= -1
			}
			allocateCanonicalEnergyDriverNodes(nodes, loads, true)
			if nodes[ids[0]].node.AllocatedValue != 0 || nodes[ids[0]].node.RawValue != .5 || nodes[ids[0]].node.EffectiveValue != 1 {
				t.Fatal("zero/missing pressure changed inspectable raw evidence")
			}
			fallbacks := 0
			for _, entry := range nodes {
				if entry.node.Kind != "heat.allocation_fallback_storage" {
					continue
				}
				fallbacks++
				if budget == 0 || entry.node.AllocatedValue != float64(budget)/1000 || entry.node.ServiceKind != service || !reflect.DeepEqual(entry.node.SourceIDs, []string{"load-source"}) {
					t.Fatalf("fallback invented pressure/source/budget: %#v", entry.node)
				}
			}
			if (budget == 0 && fallbacks != 0) || (budget > 0 && fallbacks != 1) {
				t.Fatalf("budget%d fallback count%d", budget, fallbacks)
			}
		}
	}
}

func epath080QuantumNodes(budget int64, pressures []int64, service string, shift int, reverse bool) (map[string]*energyExplanationNodeAccumulator, map[string][]string, []string) {
	const zone = "Quantum Office"
	sign := 1.0
	if service == "heating" {
		sign = -1
	}
	nodes := map[string]*energyExplanationNodeAccumulator{
		"load": {node: EnergyExplanationNode{ID: "load", Level: "load", Kind: "load.zone_" + service, ServiceKind: service, ZoneName: zone, Period: "M3", Unit: "kWh", Value: float64(budget) / 1000, SourceIDs: []string{"load-source"}}},
	}
	ids := make([]string, len(pressures))
	for index := range pressures {
		ids[index] = fmt.Sprintf("heat.driver.%02d", index)
	}
	for offset := range pressures {
		index := (shift + offset) % len(pressures)
		if reverse {
			index = len(pressures) - 1 - index
		}
		id := ids[index]
		pressure := float64(pressures[index]) / 1000
		nodes[id] = &energyExplanationNodeAccumulator{node: EnergyExplanationNode{
			ID: id, Level: "heat", Kind: "heat.internal_people", DriverCategory: energyDriverCategoryPeople,
			ZoneName: zone, Period: "M3", ServiceKind: service, Unit: "kWh", RawValue: pressure / 2,
			EffectiveValue: pressure, SignedValue: sign * pressure, Value: pressure, SourceIDs: []string{"source-" + id},
		}}
	}
	for id, pressure := range map[string]float64{"opposite": -13, "zero": 0, "synthetic": 99, "unrelated": 101} {
		node := EnergyExplanationNode{ID: id, Level: "heat", Kind: "heat.internal_people", DriverCategory: energyDriverCategoryPeople, ZoneName: zone, Period: "M3", ServiceKind: service, Unit: "kWh", SignedValue: sign * pressure, RawValue: math.Abs(pressure), SourceIDs: []string{id + "-source"}}
		if id == "synthetic" {
			node.Kind = "heat.unmapped_zone_balance"
		}
		if id == "unrelated" {
			node.ZoneName = "Unrelated Office"
		}
		nodes[id] = &energyExplanationNodeAccumulator{node: node}
	}
	return nodes, map[string][]string{energyExplanationZoneServiceKey(zone, service): {"load"}}, ids
}
