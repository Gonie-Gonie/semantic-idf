package simulation

import (
	"reflect"
	"sort"
	"testing"
)

func TestEPATH121GeneratedDualValueConversionsAndTracesSurviveTwoReloads(t *testing.T) {
	result := epath121GeneratedConversion(t, "cooling", "electricity", 100, 25)
	heating := epath121GeneratedConversion(t, "heating", "natural_gas", 85, 100)
	result.Nodes = append(result.Nodes, heating.Nodes...)
	result.Links = append(result.Links, heating.Links...)
	result.Sources = append(result.Sources, heating.Sources...)
	for _, phase := range []string{"generated", "first reload", "second reload"} {
		if phase != "generated" {
			result = epath120Reload(t, result)
		}
		t.Run(phase, func(t *testing.T) {
			for _, want := range []struct {
				service, carrier, kind string
				thermal, site, ratio   float64
			}{
				{"cooling", "electricity", "coefficient_of_performance", 100, 25, 4},
				{"heating", "natural_gas", "efficiency", 85, 100, .85},
			} {
				link := energyPathV2LinkByIDs(result.Links, "load."+want.service+".building", "end_use."+want.service+".building")
				if link == nil || link.Relation != "load_to_end_use" || link.ServiceKind != want.service || link.FromValue != want.thermal || link.ToValue != want.site || link.Ratio != want.ratio || link.RatioKind != want.kind {
					t.Errorf("%s conversion lost its dual values/classification: %#v", want.service, link)
					continue
				}
				load := energyPathV2NodeByID(result.Nodes, link.FromID)
				endUse := energyPathV2NodeByID(result.Nodes, link.ToID)
				if load.ScaleDomain != "thermal" || endUse.ScaleDomain != "site" || link.FromUnit != "kWh thermal" || link.ToUnit != "kWh" {
					t.Errorf("%s conversion lost thermal/site units: %#v", want.service, link)
				}
				for _, sourceID := range []string{"load." + want.service, "meter." + want.service + "." + want.carrier} {
					if !stringSliceContains(link.SourceIDs, sourceID) || epath092AuditSourceByID(result.Sources, sourceID) == nil {
						t.Errorf("%s ratio lost inspectable source %s", want.service, sourceID)
					}
				}
			}
			if got := len(epath092AuditLinksByRelation(result.Links, "load_to_end_use")); got != 2 {
				t.Errorf("expected exactly two service conversion crossings, got %d", got)
			}
		})
	}
}

func TestEPATH121StoredRatioUsesDualValuesAndActualCarrierEvidence(t *testing.T) {
	for _, test := range []struct {
		service, carrier, expectedKind string
		thermal, site, ratio           float64
	}{
		{"cooling", "electricity", "coefficient_of_performance", 100, 25, 4},
		{"heating", "natural_gas", "efficiency", 85, 100, .85},
	} {
		t.Run(test.service, func(t *testing.T) {
			result := epath121GeneratedConversion(t, test.service, test.carrier, test.thermal, test.site)
			link := energyPathV2LinkByIDs(result.Links, "load."+test.service+".building", "end_use."+test.service+".building")
			link.Ratio, link.RatioKind, link.RatioLabel = 999, "load_to_fuel", "stale imported ratio"
			trace := append([]string{}, link.SourceIDs...)
			for pass := 1; pass <= 2; pass++ {
				result = epath120Reload(t, result)
				link = energyPathV2LinkByIDs(result.Links, "load."+test.service+".building", "end_use."+test.service+".building")
				if link == nil || link.Ratio != test.ratio || link.RatioKind != test.expectedKind || link.RatioLabel == "stale imported ratio" {
					t.Errorf("reload %d trusted stale ratio instead of dual values/carrier evidence: %#v", pass, link)
				}
				if link != nil && !reflect.DeepEqual(link.SourceIDs, trace) {
					t.Errorf("reload %d ratio refresh changed source trace: got=%#v want=%#v", pass, link.SourceIDs, trace)
				}
			}
		})
	}
}

func TestEPATH121StoredInvalidCrossingsCannotBecomeConversions(t *testing.T) {
	for _, name := range []string{"lighting", "fans", "pumps", "reversed", "mismatched service", "wrong domain", "volume source unit", "volume target unit"} {
		t.Run(name, func(t *testing.T) {
			result := epath121GeneratedConversion(t, "cooling", "electricity", 100, 25)
			link := energyPathV2LinkByIDs(result.Links, "load.cooling.building", "end_use.cooling.building")
			switch name {
			case "lighting", "fans", "pumps":
				node := *energyPathV2NodeByID(result.Nodes, link.ToID)
				node.ID, node.Kind, node.EndUse, node.ServiceKind = "end_use."+name+".building", "end_use."+name, name, ""
				result.Nodes = append(result.Nodes, node)
				link.ToID = node.ID
			case "reversed":
				link.FromID, link.ToID = link.ToID, link.FromID
			case "mismatched service":
				energyPathV2NodeByID(result.Nodes, link.FromID).ServiceKind = "heating"
			case "wrong domain":
				energyPathV2NodeByID(result.Nodes, link.ToID).ScaleDomain = "thermal"
			case "volume source unit":
				link.FromUnit = "m3"
			case "volume target unit":
				link.ToUnit = "m3"
			}
			for pass := 1; pass <= 2; pass++ {
				result = epath120Reload(t, result)
				for _, got := range epath092AuditLinksByRelation(result.Links, "load_to_end_use") {
					t.Errorf("reload %d retained invalid thermal/site crossing: %#v", pass, *got)
				}
			}
		})
	}
}

func TestEPATH121StoredPartialAnnualConversionRetainsObservedOverlap(t *testing.T) {
	result := epath121GeneratedConversion(t, "cooling", "electricity", 100, 10)
	link := energyPathV2LinkByIDs(result.Links, "load.cooling.building", "end_use.cooling.building")
	link.FromValue, link.ToValue, link.Ratio = 40, 10, 999
	link.Basis = "direct_zone_energy"
	link.Explanation = "Only observed coincident months; partial temporal coverage"
	// The endpoint nodes describe annual values, but this link is justified
	// only by coincident January observations. Refresh must not claim the
	// unobserved remainder by unioning annual endpoint source IDs.
	link.SourceIDs = []string{"observed-load-M1", "observed-energy-M1"}
	result.Sources = append(result.Sources,
		EnergyDataSource{ID: "observed-load-M1", SourceType: "test", Name: "January cooling load", RawValue: 40, NormalizedUnit: "kWh thermal"},
		EnergyDataSource{ID: "observed-energy-M1", SourceType: "test", Name: "January cooling energy", RawValue: 10, NormalizedUnit: "kWh"},
	)
	trace := append([]string{}, link.SourceIDs...)
	sort.Strings(trace)
	for pass := 1; pass <= 2; pass++ {
		result = epath120Reload(t, result)
		link = energyPathV2LinkByIDs(result.Links, "load.cooling.building", "end_use.cooling.building")
		if link == nil || link.FromValue != 40 || link.ToValue != 10 || link.Ratio != 4 || link.RatioKind != "coefficient_of_performance" || link.Explanation != "Only observed coincident months; partial temporal coverage" || !reflect.DeepEqual(link.SourceIDs, trace) {
			t.Errorf("reload %d expanded partial conversion or lost trace: %#v", pass, link)
		}
		if load := energyPathV2NodeByID(result.Nodes, "load.cooling.building"); load == nil || load.Value != 100 {
			t.Errorf("reload %d altered annual load while refreshing partial ratio: %#v", pass, load)
		}
		if link != nil && (stringSliceContains(link.SourceIDs, "load.cooling") || stringSliceContains(link.SourceIDs, "meter.cooling.electricity")) {
			t.Errorf("reload %d partial conversion claimed annual endpoint sources: %#v", pass, link.SourceIDs)
		}
	}
}

func epath121GeneratedConversion(t *testing.T, service, carrier string, thermal, site float64) EnergyExplanationResult {
	t.Helper()
	legacy := epath092AuditConversionFixture(service, []epath092AuditCarrierValue{{carrier, site}}, thermal, "kWh thermal", "kWh")
	for _, sourceID := range []string{"load." + service, "meter." + service + "." + carrier, "facility." + carrier} {
		legacy.Sources = append(legacy.Sources, EnergyDataSource{ID: sourceID, SourceType: "test", Name: sourceID, SourceUnit: "kWh", NormalizedUnit: "kWh"})
	}
	return UpgradeEnergyExplanationV1(legacy)
}
