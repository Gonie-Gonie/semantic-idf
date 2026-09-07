package simulation

import (
	"math"
	"reflect"
	"testing"
)

func TestEnergyPathFanPoolEveryZoneStaysWithinItsOwnRoundingQuantum(t *testing.T) {
	for _, total := range []float64{.0025, 1.003, 1000.003} {
		original := []energyPathZoneAuxiliaryTarget{
			{NodeID: "a", ZoneName: "A", Value: 1},
			{NodeID: "b", ZoneName: "B", Value: 1},
			{NodeID: "c", ZoneName: "C", Value: 1},
			{NodeID: "d", ZoneName: "D", Value: 1},
			{NodeID: "e", ZoneName: "E", Value: 1},
			{NodeID: "zero", ZoneName: "Zero", Value: 0},
		}
		budget := math.Round(total * 1000)
		var baseline map[string]float64
		// Rotate and reverse tied recipients: rounding ownership must not depend
		// on query, map or Zone iteration order.
		for offset := 0; offset < len(original); offset++ {
			for _, reverse := range []bool{false, true} {
				targets := append(append([]energyPathZoneAuxiliaryTarget(nil), original[offset:]...), original[:offset]...)
				if reverse {
					for left, right := 0, len(targets)-1; left < right; left, right = left+1, right-1 {
						targets[left], targets[right] = targets[right], targets[left]
					}
				}
				before := append([]energyPathZoneAuxiliaryTarget(nil), targets...)
				values := energyPathFanPoolShares(total, targets)
				if len(values) != len(targets) {
					t.Fatal("valid positive pool rejected")
				}
				got, sum := map[string]float64{}, 0.0
				for index, value := range values {
					target := targets[index]
					quota := budget * target.Value / 5
					quanta := math.Round(value * 1000)
					if value < 0 || (quanta != math.Floor(quota) && quanta != math.Ceil(quota)) {
						t.Fatalf("pool=%g Zone=%s allocation=%g outside own quota [%g,%g]; another Zone's rounding error accumulated here", total, target.ZoneName, value, math.Floor(quota)/1000, math.Ceil(quota)/1000)
					}
					got[target.NodeID] = value
					sum += quanta
				}
				if sum != budget || !reflect.DeepEqual(before, targets) {
					t.Fatal("pool budget escaped or input recipients mutated")
				}
				if baseline == nil {
					baseline = got
				} else if !reflect.DeepEqual(baseline, got) {
					t.Fatalf("tie ownership depends on recipient order: %v versus %v", baseline, got)
				}
			}
		}
	}
}
