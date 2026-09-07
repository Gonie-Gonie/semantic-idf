package simulation

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathDerivedDriverNativeRawMultiplier(t *testing.T) {
	for _, test := range []struct {
		name        string
		zone, group float64
	}{
		{"Zone", 10, 1},
		{"ZoneGroup", 1, 10},
		{"both", 2, 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			series := []energyExplanationSeries{
				reviewEnergyLoadSeries("Office", "cooling", 100, "load"),
				reviewEnergyHeatSeries(t, "Wall A", "Surface Inside Face Convection Heat Gain Energy", -10, "surface-detail"),
				reviewEnergyHeatSeries(t, "Office", "Zone Air Heat Balance Surface Convection Rate", 12, "surface-aggregate"),
			}
			report := idf.GeometryReport{Surfaces: []idf.GeometrySurface{{ID: "wall.a", Name: "Wall A", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Outdoors"}}}
			context := newEnergyDriverBuildContext(report)
			context.Multipliers = energyEffectiveMultiplierIndex{Enabled: true, Zones: map[string]energyZoneMultiplierRecord{"office": {ZoneName: "Office", ZoneMultiplier: test.zone, GroupMultiplier: test.group}}}
			legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, context)
			result := UpgradeEnergyExplanationV1(legacy)
			assert := func(result EnergyExplanationResult) {
				t.Helper()
				wall := energyPathV2NodeByID(result.Nodes, "driver.surface.exterior_walls.cooling.building")
				storage := energyPathV2NodeByID(result.Nodes, "driver.balance.storage_other.cooling.building")
				if wall == nil || storage == nil || wall.RawValue != 10 || wall.EffectiveValue != 100 || wall.Value != 833.333 || storage.RawValue != 2 || storage.EffectiveValue != 20 || storage.Value != 166.667 || wall.Value+storage.Value != 1000 {
					t.Fatalf("raw-only correction changed effective pressure/allocation: wall=%#v storage=%#v", wall, storage)
				}
				source := reviewEnergySourceWithFormula(result.Sources, "signed zone surface-convection aggregate - signed selected surface-source sum")
				if source == nil || source.SourceType != "derived_formula" || source.RawValue != 2 || source.EffectiveValue != 20 || source.EffectiveMultiplier != 10 || source.AllocatedValue != 166.667 || !strings.Contains(source.Explanation, "not a separate SQL measurement") {
					t.Fatalf("derived raw/effective/source semantics lost: %#v", source)
				}
				for _, observed := range []struct {
					id             string
					raw, effective float64
				}{{"surface-detail", -10, -100}, {"surface-aggregate", 12, 120}} {
					s := energyExplanationSourceByID(result.Sources, observed.id)
					if s == nil || s.RawValue != observed.raw || s.EffectiveValue != observed.effective {
						t.Fatalf("original SQL observation changed: %#v", s)
					}
				}
			}
			assert(result)
			for repeat := 0; repeat < 2; repeat++ {
				data, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, &result); err != nil {
					t.Fatal(err)
				}
				assert(result)
			}
		})
	}
}

func TestEnergyPathDerivedDriverRawPreparationIsCopyOnWriteAndIdempotent(t *testing.T) {
	item, sources := appendDerivedEnergyDriverSeries(nil, "surface-reconciliation-sensible", "Office", energyDriverCategoryStorageOther, "heat.surface_reconciliation", "surface.reconciliation_difference", "Surface difference", "Derived context", "signed difference", []string{"observed-a", "observed-b"}, nil, energyDriverVector{total: 0, monthly: map[int]float64{1: 20, 2: -20}, daily: map[int]float64{1: 20, 32: -20}, hourly: map[int]float64{1: 20, 745: -20}, selectedRange: -20, hasSelectedRange: true}, false, energyDriverInspectorSectionBalance)
	series := []energyExplanationSeries{item}
	context := energyDriverBuildContext{Enabled: true, Multipliers: energyEffectiveMultiplierIndex{Enabled: true, Zones: map[string]energyZoneMultiplierRecord{"office": {ZoneName: "Office", ZoneMultiplier: 2, GroupMultiplier: 5}}}}
	before, _ := json.Marshal([]any{series, sources})
	got, gotSources := retainEnergyDriverDerivedRawZoneEquivalent(series, sources, context)
	after, _ := json.Marshal([]any{series, sources})
	if string(before) != string(after) {
		t.Fatal("original series/source modified")
	}
	if got[0].RawTotal != 0 || got[0].RawMonthly[1] != 2 || got[0].RawMonthly[2] != -2 || got[0].RawDaily[32] != -2 || got[0].RawHourly[745] != -2 || got[0].RawSelectedRange != -2 || gotSources[0].RawValue != 0 {
		t.Fatal("signed zero/period native equivalent lost")
	}
	if !reflect.DeepEqual(got[0].Monthly, item.Monthly) || !reflect.DeepEqual(got[0].Daily, item.Daily) || !reflect.DeepEqual(got[0].Hourly, item.Hourly) || got[0].Total != item.Total || got[0].SelectedRange != item.SelectedRange || gotSources[0].EffectiveValue != sources[0].EffectiveValue {
		t.Fatal("effective vectors changed")
	}
	again, againSources := retainEnergyDriverDerivedRawZoneEquivalent(got, gotSources, context)
	if !reflect.DeepEqual(got, again) || !reflect.DeepEqual(gotSources, againSources) {
		t.Fatal("raw preparation repeated factor or explanation")
	}
	reapplied, reappliedSources, _ := applyEnergyExplanationMultipliers(got, gotSources, context.Multipliers)
	if !reflect.DeepEqual(got, reapplied) || !reflect.DeepEqual(gotSources, reappliedSources) {
		t.Fatal("multiplier reapplied to already effective derived vector")
	}
}

func TestEnergyPathDerivedDriverRawRequiresKnownZoneOwnership(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*energyExplanationSeries, *[]EnergyDataSource, *energyDriverBuildContext)
	}{
		{"disabled", func(_ *energyExplanationSeries, _ *[]EnergyDataSource, c *energyDriverBuildContext) {
			c.Enabled = false
		}},
		{"missing Zone", func(_ *energyExplanationSeries, _ *[]EnergyDataSource, c *energyDriverBuildContext) {
			c.Multipliers.Zones = nil
		}},
		{"Building only", func(s *energyExplanationSeries, _ *[]EnergyDataSource, _ *energyDriverBuildContext) {
			s.driverBuildingOnly = true
		}},
		{"unscoped", func(s *energyExplanationSeries, _ *[]EnergyDataSource, _ *energyDriverBuildContext) { s.ZoneName = "" }},
		{"measured source", func(_ *energyExplanationSeries, s *[]EnergyDataSource, _ *energyDriverBuildContext) {
			(*s)[0].SourceType = "sql_variable"
		}},
		{"duplicate source", func(_ *energyExplanationSeries, s *[]EnergyDataSource, _ *energyDriverBuildContext) {
			*s = append(*s, (*s)[0])
		}},
		{"wrong source Zone", func(_ *energyExplanationSeries, s *[]EnergyDataSource, _ *energyDriverBuildContext) {
			(*s)[0].ZoneName = "Other"
		}},
		{"nonfinite factor", func(_ *energyExplanationSeries, _ *[]EnergyDataSource, c *energyDriverBuildContext) {
			c.Multipliers.Zones["office"] = energyZoneMultiplierRecord{ZoneName: "Office", ZoneMultiplier: math.Inf(1), GroupMultiplier: 1}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			item, sources := appendDerivedEnergyDriverSeries(nil, "test", "Office", energyDriverCategoryStorageOther, "heat.surface_reconciliation", "surface.reconciliation_difference", "Difference", "", "difference", []string{"original"}, nil, energyDriverVector{total: 20, monthly: map[int]float64{1: 20}}, false, energyDriverInspectorSectionBalance)
			context := energyDriverBuildContext{Enabled: true, Multipliers: energyEffectiveMultiplierIndex{Enabled: true, Zones: map[string]energyZoneMultiplierRecord{"office": {ZoneName: "Office", ZoneMultiplier: 10, GroupMultiplier: 1}}}}
			test.mutate(&item, &sources, &context)
			series := []energyExplanationSeries{item}
			got, gotSources := retainEnergyDriverDerivedRawZoneEquivalent(series, sources, context)
			if !reflect.DeepEqual(got, series) || !reflect.DeepEqual(gotSources, sources) {
				t.Fatal("unproven native Zone conversion applied")
			}
		})
	}
}
