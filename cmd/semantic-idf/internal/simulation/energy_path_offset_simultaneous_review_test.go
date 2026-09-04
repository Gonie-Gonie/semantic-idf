package simulation

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"testing"
)

// The fixture is deliberately hostile to aggregate-first arithmetic. In M1,
// Office is cooling-heavy while Lab is heating-heavy; summing the loads first
// would manufacture a 100% simultaneous ratio. In M2 the two services occur in
// different zones, so the correct Building numerator remains zero.
func TestEPATH081ReviewSimultaneousRatioIsZoneMonthFirst(t *testing.T) {
	forward := review081BuildFixture(t, false)
	reversed := review081BuildFixture(t, true)

	for _, result := range []EnergyExplanationResult{forward, reversed} {
		checks := []struct {
			zone        string
			period      string
			numerator   float64
			denominator float64
			ratio       float64
			services    []string
		}{
			{zone: "Office", period: "M1", numerator: 10, denominator: 100, ratio: 0.1, services: []string{"cooling", "heating"}},
			{zone: "Lab", period: "M1", numerator: 10, denominator: 100, ratio: 0.1, services: []string{"cooling", "heating"}},
			{zone: "Office", period: "M2", numerator: 0, denominator: 40, ratio: 0, services: []string{"cooling"}},
			{zone: "Lab", period: "M2", numerator: 0, denominator: 20, ratio: 0, services: []string{"heating"}},
			{zone: "", period: "M1", numerator: 20, denominator: 200, ratio: 0.1, services: []string{"cooling", "heating"}},
			// Aggregate-first min(40,20)/max(40,20) would be 0.5. There is no
			// simultaneous load in either zone during M2, so this must be zero.
			{zone: "", period: "M2", numerator: 0, denominator: 60, ratio: 0, services: []string{"cooling", "heating"}},
			{zone: "Office", period: "annual", numerator: 10, denominator: 140, ratio: 0.071429, services: []string{"cooling", "heating"}},
			{zone: "Lab", period: "annual", numerator: 10, denominator: 120, ratio: 0.083333, services: []string{"cooling", "heating"}},
			{zone: "", period: "annual", numerator: 20, denominator: 260, ratio: 0.076923, services: []string{"cooling", "heating"}},
		}
		for _, want := range checks {
			for _, service := range want.services {
				node := review081LoadNode(t, result, want.zone, want.period, service)
				review081AssertSimultaneous(t, node, want.numerator, want.denominator, want.ratio)
			}
		}

		// The same zone-month tuple must be attached to both primary load nodes.
		for _, scopePeriod := range []struct{ zone, period string }{
			{zone: "Office", period: "M1"}, {zone: "Lab", period: "M1"},
			{zone: "", period: "M1"}, {zone: "", period: "M2"},
			{zone: "Office", period: "annual"}, {zone: "Lab", period: "annual"}, {zone: "", period: "annual"},
		} {
			cooling := review081LoadNode(t, result, scopePeriod.zone, scopePeriod.period, "cooling")
			heating := review081LoadNode(t, result, scopePeriod.zone, scopePeriod.period, "heating")
			if !reflect.DeepEqual(cooling.SimultaneousLoad, heating.SimultaneousLoad) {
				t.Errorf("%s/%s cooling and heating simultaneous metadata differ: cooling=%#v heating=%#v", scopePeriod.zone, scopePeriod.period, cooling.SimultaneousLoad, heating.SimultaneousLoad)
			}
		}
	}

	if got, want := review081MetadataSignature(forward), review081MetadataSignature(reversed); !reflect.DeepEqual(got, want) {
		t.Fatalf("081 metadata changed with input order\nforward=%#v\nreverse=%#v", got, want)
	}
}

func TestEPATH081ReviewOffsetsAreLoadLocalContextWithoutReverseMainLinks(t *testing.T) {
	result := review081BuildFixture(t, false)

	// Both services are present in the same zone-month and must close
	// independently with the matching pressure sign.
	for _, want := range []struct {
		zone, period, service string
		load                  float64
	}{
		{zone: "Office", period: "M1", service: "cooling", load: 100},
		{zone: "Office", period: "M1", service: "heating", load: 10},
		{zone: "Lab", period: "M1", service: "cooling", load: 10},
		{zone: "Lab", period: "M1", service: "heating", load: 100},
		{zone: "", period: "M1", service: "cooling", load: 110},
		{zone: "", period: "M1", service: "heating", load: 110},
		{zone: "", period: "M2", service: "cooling", load: 40},
		{zone: "", period: "M2", service: "heating", load: 20},
		{zone: "", period: "annual", service: "cooling", load: 150},
		{zone: "", period: "annual", service: "heating", load: 130},
	} {
		epath080AssertExactClosure(t, result, want.zone, want.period, want.service, want.load)
		review081AssertNoReverseDriverLinks(t, result, want.zone, want.period)
	}

	officeM1Cooling := review081LoadNode(t, result, "Office", "M1", "cooling")
	review081AssertOffset(t, officeM1Cooling, "cooling", energyDriverCategoryInfiltration, "loss", 20, []string{"office-infiltration"})
	officeM1Heating := review081LoadNode(t, result, "Office", "M1", "heating")
	review081AssertOffset(t, officeM1Heating, "heating", energyDriverCategoryPeople, "gain", 30, []string{"office-people"})

	// A pressure in a zone-month with no target service is inspectable at its
	// raw source, but is not an offset of a load delivered in another zone.
	buildingM2Cooling := review081LoadNode(t, result, "", "M2", "cooling")
	review081AssertOffset(t, buildingM2Cooling, "cooling", energyDriverCategoryInfiltration, "loss", 4, []string{"office-infiltration"})
	review081RejectOffsetSource(t, buildingM2Cooling, "lab-infiltration")
	buildingM2Heating := review081LoadNode(t, result, "", "M2", "heating")
	review081AssertOffset(t, buildingM2Heating, "heating", energyDriverCategoryPeople, "gain", 6, []string{"lab-people"})
	review081RejectOffsetSource(t, buildingM2Heating, "office-people")

	buildingAnnualCooling := review081LoadNode(t, result, "", "annual", "cooling")
	review081AssertOffset(t, buildingAnnualCooling, "cooling", energyDriverCategoryInfiltration, "loss", 74, []string{
		"office-infiltration", "lab-infiltration",
	})
	buildingAnnualHeating := review081LoadNode(t, result, "", "annual", "heating")
	review081AssertOffset(t, buildingAnnualHeating, "heating", energyDriverCategoryPeople, "gain", 41, []string{
		"office-people", "lab-people",
	})
}

func TestEPATH081ReviewFrozenV1DoesNotGainOffsetOrRatioFields(t *testing.T) {
	series := []energyExplanationSeries{
		epath080Load("legacy-cooling", "Office", "cooling", map[int]float64{1: 20}),
		epath080Load("legacy-heating", "Office", "heating", map[int]float64{1: 10}),
		epath080Heat(t, "legacy-people", "Office", "Zone People Convective Heating Energy", map[int]float64{1: 4}),
		epath080Heat(t, "legacy-infiltration", "Office", "Zone Infiltration Sensible Heat Loss Energy", map[int]float64{1: 3}),
	}
	legacy := buildEnergyExplanationResult(series, epath080Sources(series), &PurposeRunPlan{})
	for _, node := range legacy.Nodes {
		if node.Level == "load" && (len(node.OffsetEffects) != 0 || node.SimultaneousLoad != nil) {
			t.Fatalf("frozen v1 load gained 081 metadata: %#v", node)
		}
	}
	for _, period := range legacy.Periods {
		for _, node := range period.Nodes {
			if node.Level == "load" && (len(node.OffsetEffects) != 0 || node.SimultaneousLoad != nil) {
				t.Fatalf("frozen v1 period load gained 081 metadata: period=%s node=%#v", period.ID, node)
			}
		}
	}
}

func review081BuildFixture(t *testing.T, reverse bool) EnergyExplanationResult {
	t.Helper()
	series := []energyExplanationSeries{
		epath080Load("office-cooling-load", "Office", "cooling", map[int]float64{1: 100, 2: 40}),
		epath080Load("office-heating-load", "Office", "heating", map[int]float64{1: 10}),
		epath080Load("lab-cooling-load", "Lab", "cooling", map[int]float64{1: 10}),
		epath080Load("lab-heating-load", "Lab", "heating", map[int]float64{1: 100, 2: 20}),
		epath080Heat(t, "office-people", "Office", "Zone People Convective Heating Energy", map[int]float64{1: 30, 2: 8}),
		epath080Heat(t, "office-infiltration", "Office", "Zone Infiltration Sensible Heat Loss Energy", map[int]float64{1: 20, 2: 4}),
		epath080Heat(t, "lab-people", "Lab", "Zone People Convective Heating Energy", map[int]float64{1: 5, 2: 6}),
		epath080Heat(t, "lab-infiltration", "Lab", "Zone Infiltration Sensible Heat Loss Energy", map[int]float64{1: 50, 2: 12}),
	}
	if reverse {
		for left, right := 0, len(series)-1; left < right; left, right = left+1, right-1 {
			series[left], series[right] = series[right], series[left]
		}
	}
	return epath080Build(series, epath080UnitMultipliers("Office", "Lab"))
}

func review081LoadNode(t *testing.T, result EnergyExplanationResult, zone, period, service string) *EnergyExplanationNode {
	t.Helper()
	nodes, _ := epath080Graph(t, result, zone, period)
	id := "load." + service + "." + epath080ScopeToken(zone)
	node := epath080NodeByID(nodes, id)
	if node == nil {
		t.Fatalf("%s/%s load %s is missing", zone, period, id)
	}
	return node
}

func review081AssertSimultaneous(t *testing.T, node *EnergyExplanationNode, numerator, denominator, ratio float64) {
	t.Helper()
	metric := node.SimultaneousLoad
	if metric == nil || !metric.Available || metric.Numerator != numerator || metric.Denominator != denominator ||
		math.Abs(metric.Ratio-ratio) > 5e-7 || metric.Unit != "kWh" || metric.Basis != "simultaneous_min_over_max" {
		t.Errorf("%s simultaneous load = %#v, want available %g/%g = %g kWh with simultaneous_min_over_max", node.ID, metric, numerator, denominator, ratio)
	}
}

func review081AssertOffset(t *testing.T, node *EnergyExplanationNode, targetService, category, direction string, effective float64, sourceIDs []string) {
	t.Helper()
	var found *EnergyExplanationOffsetEffect
	for index := range node.OffsetEffects {
		effect := &node.OffsetEffects[index]
		if effect.TargetService == targetService && effect.DriverCategory == category {
			found = effect
			break
		}
	}
	if found == nil || found.EffectKind != "reduces_"+targetService || found.HeatDirection != direction ||
		found.RawValue != effective || found.EffectiveValue != effective || found.Unit != "kWh" ||
		found.Label == "" || found.Basis != energyDriverOffsetBasis || found.Explanation != energyDriverOffsetExplanation {
		t.Errorf("%s %s offset = %#v, want %s %g kWh", node.ID, category, found, direction, effective)
		return
	}
	for _, sourceID := range sourceIDs {
		if !stringSliceContains(found.SourceIDs, sourceID) {
			t.Errorf("%s %s offset sources = %#v, missing %s", node.ID, category, found.SourceIDs, sourceID)
		}
	}
}

func review081RejectOffsetSource(t *testing.T, node *EnergyExplanationNode, sourceID string) {
	t.Helper()
	for _, effect := range node.OffsetEffects {
		if stringSliceContains(effect.SourceIDs, sourceID) {
			t.Errorf("%s offset leaked source %q from a zone-month without this target load: %#v", node.ID, sourceID, effect)
		}
	}
}

func review081AssertNoReverseDriverLinks(t *testing.T, result EnergyExplanationResult, zone, period string) {
	t.Helper()
	nodes, links := epath080Graph(t, result, zone, period)
	byID := make(map[string]EnergyExplanationNode, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	for _, link := range links {
		if link.Relation != "driver_to_load" {
			continue
		}
		from, fromOK := byID[link.FromID]
		to, toOK := byID[link.ToID]
		if !fromOK || !toOK {
			continue
		}
		if link.FromValue < 0 || link.ToValue < 0 {
			t.Errorf("%s/%s reverse-width main link = %#v", zone, period, link)
		}
		if from.SignedValue > 0 && energyCanonicalServiceKind(to.ServiceKind) != "cooling" {
			t.Errorf("%s/%s positive driver became reverse heating main link: from=%#v to=%#v link=%#v", zone, period, from, to, link)
		}
		if from.SignedValue < 0 && energyCanonicalServiceKind(to.ServiceKind) != "heating" {
			t.Errorf("%s/%s negative driver became reverse cooling main link: from=%#v to=%#v link=%#v", zone, period, from, to, link)
		}
	}
}

func review081MetadataSignature(result EnergyExplanationResult) []string {
	out := []string{}
	appendGraph := func(scope, period string, nodes []EnergyExplanationNode) {
		for _, node := range nodes {
			if node.Level != "load" {
				continue
			}
			metric := node.SimultaneousLoad
			if metric != nil {
				out = append(out, fmt.Sprintf("%s|%s|%s|ratio|%.12g|%.12g|%.12g", scope, period, node.ID, metric.Numerator, metric.Denominator, metric.Ratio))
			}
			for _, effect := range node.OffsetEffects {
				out = append(out, fmt.Sprintf("%s|%s|%s|offset|%s|%s|%.12g|%.12g", scope, period, node.ID, effect.TargetService, effect.DriverCategory, effect.RawValue, effect.EffectiveValue))
			}
		}
	}
	appendGraph("building", "annual", result.Nodes)
	for _, period := range result.Periods {
		if period.Kind == "monthly" {
			appendGraph("building", period.ID, period.Nodes)
		}
	}
	for _, zone := range result.ZoneResults {
		appendGraph(zone.Scope.ZoneName, "annual", zone.Nodes)
		for _, period := range zone.Periods {
			if period.Kind == "monthly" {
				appendGraph(zone.Scope.ZoneName, period.ID, period.Nodes)
			}
		}
	}
	sort.Strings(out)
	return out
}
