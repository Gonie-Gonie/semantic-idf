package simulation

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The raw pressure totals in every zone-month are deliberately much larger
// than the delivered loads. Allocating after Building aggregation would split
// annual cooling 150/150 between People and Lighting. Correct zone-month-first
// allocation produces 68/232 after Office's 2x effective multiplier.
func TestEPATH080AcceptanceAllocatesSignedPressurePerZoneMonthBeforeBuilding(t *testing.T) {
	result := epath080BuildSignedFixture(t, false)

	expected := []struct {
		zone      string
		period    string
		service   string
		category  string
		raw       float64
		effective float64
		allocated float64
	}{
		{zone: "Office", period: "M1", service: "cooling", category: energyDriverCategoryPeople, raw: 90, effective: 180, allocated: 18},
		{zone: "Office", period: "M1", service: "cooling", category: energyDriverCategoryLighting, raw: 10, effective: 20, allocated: 2},
		{zone: "Office", period: "M2", service: "cooling", category: energyDriverCategoryPeople, raw: 10, effective: 20, allocated: 18},
		{zone: "Office", period: "M2", service: "cooling", category: energyDriverCategoryLighting, raw: 90, effective: 180, allocated: 162},
		{zone: "Lab", period: "M1", service: "cooling", category: energyDriverCategoryPeople, raw: 80, effective: 80, allocated: 16},
		{zone: "Lab", period: "M1", service: "cooling", category: energyDriverCategoryLighting, raw: 20, effective: 20, allocated: 4},
		{zone: "Lab", period: "M2", service: "cooling", category: energyDriverCategoryPeople, raw: 20, effective: 20, allocated: 16},
		{zone: "Lab", period: "M2", service: "cooling", category: energyDriverCategoryLighting, raw: 80, effective: 80, allocated: 64},
		{zone: "Office", period: "M3", service: "heating", category: energyDriverCategoryInfiltration, raw: 60, effective: 120, allocated: 7.5},
		{zone: "Office", period: "M3", service: "heating", category: energyDriverCategoryMechanicalVentilation, raw: 36, effective: 72, allocated: 4.5},
		{zone: "Office", period: "M4", service: "heating", category: energyDriverCategoryInfiltration, raw: 20, effective: 40, allocated: 26.667},
		{zone: "Office", period: "M4", service: "heating", category: energyDriverCategoryMechanicalVentilation, raw: 10, effective: 20, allocated: 13.333},
		{zone: "Lab", period: "M3", service: "heating", category: energyDriverCategoryInfiltration, raw: 20, effective: 20, allocated: 0.952},
		{zone: "Lab", period: "M3", service: "heating", category: energyDriverCategoryMechanicalVentilation, raw: 64, effective: 64, allocated: 3.048},
		{zone: "Lab", period: "M4", service: "heating", category: energyDriverCategoryInfiltration, raw: 30, effective: 30, allocated: 18},
		{zone: "Lab", period: "M4", service: "heating", category: energyDriverCategoryMechanicalVentilation, raw: 20, effective: 20, allocated: 12},
	}
	for _, want := range expected {
		epath080AssertContribution(t, result, want.zone, want.period, want.service, want.category, want.raw, want.effective, want.allocated)
	}

	for _, want := range []struct {
		zone    string
		period  string
		service string
		load    float64
	}{
		{zone: "Office", period: "M1", service: "cooling", load: 20},
		{zone: "Office", period: "M2", service: "cooling", load: 180},
		{zone: "Office", period: "M3", service: "heating", load: 12},
		{zone: "Office", period: "M4", service: "heating", load: 40},
		{zone: "Lab", period: "M1", service: "cooling", load: 20},
		{zone: "Lab", period: "M2", service: "cooling", load: 80},
		{zone: "Lab", period: "M3", service: "heating", load: 4},
		{zone: "Lab", period: "M4", service: "heating", load: 30},
		{period: "M1", service: "cooling", load: 40},
		{period: "M2", service: "cooling", load: 260},
		{period: "M3", service: "heating", load: 16},
		{period: "M4", service: "heating", load: 70},
		{period: "annual", service: "cooling", load: 300},
		{period: "annual", service: "heating", load: 86},
	} {
		epath080AssertExactClosure(t, result, want.zone, want.period, want.service, want.load)
	}

	for _, want := range []struct {
		service   string
		category  string
		allocated float64
	}{
		{service: "cooling", category: energyDriverCategoryPeople, allocated: 68},
		{service: "cooling", category: energyDriverCategoryLighting, allocated: 232},
		{service: "heating", category: energyDriverCategoryInfiltration, allocated: 53.119},
		{service: "heating", category: energyDriverCategoryMechanicalVentilation, allocated: 32.881},
	} {
		nodeID := epath080DriverNodeID(want.category, want.service, "")
		node := epath080NodeByID(result.Nodes, nodeID)
		if node == nil || node.Value != want.allocated || node.AllocatedValue != want.allocated {
			t.Errorf("annual Building zone-month contribution %s = %#v, want allocated width %g", nodeID, node, want.allocated)
		}
	}
	if node := epath080NodeByID(result.Nodes, epath080DriverNodeID(energyDriverCategoryPeople, "cooling", "")); node != nil && node.Value == 150 {
		t.Errorf("People cooling contribution used aggregate-first 150/150 split: %#v", node)
	}

}

func TestEPATH080AcceptancePreservesRawEffectiveAllocatedAndInspectorContract(t *testing.T) {
	result := epath080BuildSignedFixture(t, false)
	node := epath080NodeByID(result.Nodes, epath080DriverNodeID(energyDriverCategoryPeople, "cooling", ""))
	if node == nil {
		t.Fatal("annual Building People cooling driver is missing")
	}
	if node.RawValue != 200 || node.EffectiveValue != 300 || node.AllocatedValue != 68 || node.Value != 68 {
		t.Errorf("Building People accounting = raw/effective/allocated/width %g/%g/%g/%g, want 200/300/68/68", node.RawValue, node.EffectiveValue, node.AllocatedValue, node.Value)
	}
	if !node.AllocationApplied || node.Basis != "heat_balance_share" || node.ScaleDomain != "thermal" {
		t.Errorf("Building People basis/domain = %q/%q, want heat_balance_share/thermal", node.Basis, node.ScaleDomain)
	}
	if !epath080ExplainsNonCausalShare(node.AllocationExplanation) {
		t.Errorf("Building People allocation explanation %q must identify a non-causal signed heat-balance share", node.AllocationExplanation)
	}
	link := epath080LinkByIDs(result.Links, node.ID, "load.cooling.building")
	if link == nil || link.FromValue != 68 || link.ToValue != 68 || link.Basis != "heat_balance_share" || link.ServiceKind != "cooling" {
		t.Errorf("Building People main ribbon = %#v, want allocated 68 on both sides with heat_balance_share", link)
	}

	source := epath080SourceByID(result.Sources, "people-office")
	if source == nil {
		t.Fatal("Office People inspector source is missing")
	}
	if source.RawValue != 100 || source.EffectiveValue != 200 || source.AllocatedValue != 36 || !source.AllocationApplied {
		t.Errorf("Office People source raw/effective/contribution = %g/%g/%g, want 100/200/36", source.RawValue, source.EffectiveValue, source.AllocatedValue)
	}
	if source.DriverRole != energyDriverSourceRoleMainFlow || source.InspectorSection != energyDriverInspectorSectionBreakdown {
		t.Errorf("Office People inspector placement = role %q section %q", source.DriverRole, source.InspectorSection)
	}
	if !epath080ExplainsNonCausalShare(source.Explanation) {
		t.Errorf("Office People explanation %q must explicitly say non-causal signed heat-balance share", source.Explanation)
	}
	formula := strings.ToLower(source.Formula)
	if !strings.Contains(formula, "actual") || !strings.Contains(formula, "pressure") {
		t.Errorf("Office People inspector formula %q does not expose actual-load × signed-pressure share", source.Formula)
	}

	rule := epath080RuleByID(result.RelationshipRules, energyRelationshipRuleHeatDriverBalance)
	if rule == nil || rule.Basis != "heat_balance_share" || !epath080ExplainsNonCausalShare(rule.Formula) {
		t.Errorf("driver-to-load relationship rule = %#v, want heat_balance_share pressure formula", rule)
	}
}

func TestEPATH080AcceptanceZeroPressureUsesOtherAndNeverSplicesAcrossMonths(t *testing.T) {
	series := []energyExplanationSeries{
		epath080Load("zero-cooling-load", "No Pressure", "cooling", map[int]float64{1: 7, 2: 0}),
		epath080Load("zero-heating-load", "No Pressure", "heating", map[int]float64{1: 0, 2: 5}),
		epath080Heat(t, "loss-without-heating", "No Pressure", "Zone Infiltration Sensible Heat Loss Energy", map[int]float64{1: 3, 2: 0}),
		epath080Heat(t, "gain-without-cooling", "No Pressure", "Zone People Convective Heating Energy", map[int]float64{1: 0, 2: 4}),
	}
	result := epath080Build(series, epath080UnitMultipliers("No Pressure"))

	for _, want := range []struct {
		period  string
		service string
		value   float64
	}{
		{period: "M1", service: "cooling", value: 7},
		{period: "M2", service: "heating", value: 5},
		{period: "annual", service: "cooling", value: 7},
		{period: "annual", service: "heating", value: 5},
	} {
		nodes, links := epath080Graph(t, result, "No Pressure", want.period)
		storageID := epath080DriverNodeID(energyDriverCategoryStorageOther, want.service, "No Pressure")
		loadID := "load." + want.service + ".no_pressure"
		storage := epath080NodeByID(nodes, storageID)
		link := epath080LinkByIDs(links, storageID, loadID)
		if storage == nil || storage.Value != want.value || storage.AllocatedValue != want.value || !storage.AllocationApplied {
			t.Errorf("%s zero-denominator %s Other/storage = %#v, want allocated %g", want.period, want.service, storage, want.value)
		}
		if link == nil || link.FromValue != want.value || link.ToValue != want.value || link.Basis != "heat_balance_share" {
			t.Errorf("%s zero-denominator %s ribbon = %#v, want exact Other/storage closure %g", want.period, want.service, link, want.value)
		}
		epath080AssertExactClosure(t, result, "No Pressure", want.period, want.service, want.value)
	}

	// These raw drivers have no actual matching-service load in their own month.
	// They must remain inspectable with an explicit zero allocation and must not
	// be resurrected by annual aggregation or v2 firstNonZero fallbacks.
	for _, want := range []struct {
		sourceID string
		raw      float64
		service  string
		category string
	}{
		{sourceID: "loss-without-heating", raw: 3, service: "heating", category: energyDriverCategoryInfiltration},
		{sourceID: "gain-without-cooling", raw: 4, service: "cooling", category: energyDriverCategoryPeople},
	} {
		source := epath080SourceByID(result.Sources, want.sourceID)
		if source == nil || source.RawValue != want.raw || source.EffectiveValue != want.raw || source.AllocatedValue != 0 || source.AllocationFactor != 0 || !source.AllocationApplied {
			t.Errorf("raw-only source %q = %#v, want raw/effective %g and explicit zero allocation", want.sourceID, source, want.raw)
		}
		nodeID := epath080DriverNodeID(want.category, want.service, "No Pressure")
		if node := epath080NodeByID(result.Nodes, nodeID); node != nil && (node.Value != 0 || node.AllocatedValue != 0) {
			t.Errorf("raw-only source %q reappeared as annual main node via fallback: %#v", want.sourceID, node)
		}
		if link := epath080LinkByIDs(result.Links, nodeID, "load."+want.service+".no_pressure"); link != nil && (link.FromValue != 0 || link.ToValue != 0) {
			t.Errorf("raw-only source %q reappeared as annual main ribbon via fallback: %#v", want.sourceID, link)
		}
	}
}

func TestEPATH080AcceptanceAllocationOrderAndRemainderAreDeterministic(t *testing.T) {
	forward := epath080BuildSignedFixture(t, false)
	reversed := epath080BuildSignedFixture(t, true)
	if got, want := epath080ResultSignature(forward), epath080ResultSignature(reversed); !reflect.DeepEqual(got, want) {
		t.Errorf("reversing canonical input changed ordered node/link allocation\nforward=%#v\nreverse=%#v", got, want)
	}

	// Lab M3 heating is 4 × (20/84, 64/84). The final thousandth must
	// deterministically carry the rounding remainder and close exactly.
	epath080AssertContribution(t, forward, "Lab", "M3", "heating", energyDriverCategoryInfiltration, 20, 20, 0.952)
	epath080AssertContribution(t, forward, "Lab", "M3", "heating", energyDriverCategoryMechanicalVentilation, 64, 64, 3.048)
	epath080AssertExactClosure(t, forward, "Lab", "M3", "heating", 4)
}

// EPATH-080 is applied only by the canonical runtime path. The direct v1
// adapter remains a frozen compatibility contract and retains raw heat-driver
// widths and the historical load-to-heat edge direction.
func TestEPATH080AcceptanceFrozenV1DriverShapeIsUnchanged(t *testing.T) {
	series := []energyExplanationSeries{
		epath080Load("legacy-load", "Office", "cooling", map[int]float64{1: 100}),
		epath080Heat(t, "legacy-driver", "Office", "Zone Infiltration Sensible Heat Gain Energy", map[int]float64{1: 30}),
	}
	legacy := buildEnergyExplanationResult(series, epath080Sources(series), &PurposeRunPlan{})
	if legacy.Schema != energyExplanationV1Schema {
		t.Fatalf("frozen adapter schema = %q, want %q", legacy.Schema, energyExplanationV1Schema)
	}
	var driver *EnergyExplanationNode
	for index := range legacy.Nodes {
		if epath080Contains(legacy.Nodes[index].SourceIDs, "legacy-driver") {
			driver = &legacy.Nodes[index]
			break
		}
	}
	if driver == nil || driver.Level != "heat" || driver.Value != 30 || driver.AllocatedValue != 0 {
		t.Errorf("frozen v1 driver = %#v, want historical raw 30 kWh heat node without v2 allocation fields", driver)
	}
	var edge *EnergyExplanationEdge
	for index := range legacy.Edges {
		if legacy.Edges[index].Relation == "heat_driver" && epath080Contains(legacy.Edges[index].SourceIDs, "legacy-driver") {
			edge = &legacy.Edges[index]
			break
		}
	}
	if edge == nil || edge.Value != 30 || edge.Basis != "derived_balance" || driver == nil || edge.ToID != driver.ID {
		t.Errorf("frozen v1 heat-driver edge = %#v, want historical load->heat raw width 30", edge)
	}
}

func epath080BuildSignedFixture(t *testing.T, reverse bool) EnergyExplanationResult {
	t.Helper()
	series := []energyExplanationSeries{
		epath080Load("cooling-office", "Office", "cooling", map[int]float64{1: 10, 2: 90}),
		epath080Load("heating-office", "Office", "heating", map[int]float64{3: 6, 4: 20}),
		epath080Load("cooling-lab", "Lab", "cooling", map[int]float64{1: 20, 2: 80}),
		epath080Load("heating-lab", "Lab", "heating", map[int]float64{3: 4, 4: 30}),
		epath080Heat(t, "people-office", "Office", "Zone People Convective Heating Energy", map[int]float64{1: 90, 2: 10}),
		epath080Heat(t, "lighting-office", "Office", "Zone Lights Convective Heating Energy", map[int]float64{1: 10, 2: 90}),
		epath080Heat(t, "infiltration-office", "Office", "Zone Infiltration Sensible Heat Loss Energy", map[int]float64{3: 60, 4: 20}),
		epath080Heat(t, "ventilation-office", "Office", "Zone Ventilation Sensible Heat Loss Energy", map[int]float64{3: 36, 4: 10}),
		epath080Heat(t, "people-lab", "Lab", "Zone People Convective Heating Energy", map[int]float64{1: 80, 2: 20}),
		epath080Heat(t, "lighting-lab", "Lab", "Zone Lights Convective Heating Energy", map[int]float64{1: 20, 2: 80}),
		epath080Heat(t, "infiltration-lab", "Lab", "Zone Infiltration Sensible Heat Loss Energy", map[int]float64{3: 20, 4: 30}),
		epath080Heat(t, "ventilation-lab", "Lab", "Zone Ventilation Sensible Heat Loss Energy", map[int]float64{3: 64, 4: 20}),
	}
	if reverse {
		for left, right := 0, len(series)-1; left < right; left, right = left+1, right-1 {
			series[left], series[right] = series[right], series[left]
		}
	}
	multipliers := epath080UnitMultipliers("Office", "Lab")
	multipliers.Zones[normalizePurposeToken("Office")] = energyZoneMultiplierRecord{ZoneName: "Office", ZoneMultiplier: 2, GroupMultiplier: 1}
	return epath080Build(series, multipliers)
}

func epath080Build(series []energyExplanationSeries, multipliers energyEffectiveMultiplierIndex) EnergyExplanationResult {
	legacy := buildEnergyExplanationResultWithDriverContext(
		series,
		epath080Sources(series),
		&PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath},
		energyDriverBuildContext{Enabled: true, Multipliers: multipliers},
	)
	return UpgradeEnergyExplanationV1(legacy)
}

func epath080Load(sourceID string, zone string, service string, monthly map[int]float64) energyExplanationSeries {
	label := "Cooling"
	if service == "heating" {
		label = "Heating"
	}
	return canonicalEnergyExplanationSeries(energyExplanationSeries{
		Stage:            "load",
		CanonicalKind:    "load.zone_" + service,
		Level:            "load",
		Kind:             "load.zone_" + service,
		Label:            label + " load",
		Unit:             "kWh",
		ServiceKind:      service,
		PathType:         "zone",
		ZoneName:         zone,
		ThermalComponent: "sensible",
		Basis:            "reported_variable",
		SourceIDs:        []string{sourceID},
		Total:            epath080Sum(monthly),
		Monthly:          monthly,
		SourceName:       "Zone Air System Sensible " + label + " Energy",
		sourceName:       "Zone Air System Sensible " + label + " Energy",
		sourceKeyValue:   zone,
		sourceFrequency:  "Monthly",
	})
}

func epath080Heat(t *testing.T, sourceID string, zone string, name string, monthly map[int]float64) energyExplanationSeries {
	t.Helper()
	definition, ok := energyHeatAliasDefinitionForName(name)
	if !ok {
		t.Fatalf("EPATH-080 fixture heat alias %q is not classified", name)
	}
	return canonicalEnergyExplanationSeries(energyExplanationSeriesForBuilder(&energyExplanationSeriesBuilder{
		dictionary: energyExplanationDictionary{
			row:                sqlOutputDictionaryRow{keyValue: zone, name: name, units: "J"},
			reportingFrequency: "Monthly",
			heat:               &definition,
		},
		unit:    "kWh",
		total:   epath080Sum(monthly),
		monthly: monthly,
	}, sourceID))
}

func epath080Sources(series []energyExplanationSeries) []EnergyDataSource {
	out := make([]EnergyDataSource, 0, len(series))
	seen := map[string]bool{}
	for _, item := range series {
		for _, sourceID := range item.SourceIDs {
			if seen[sourceID] {
				continue
			}
			seen[sourceID] = true
			out = append(out, EnergyDataSource{
				ID:                 sourceID,
				SourceType:         "acceptance_fixture",
				KeyValue:           item.sourceKeyValue,
				Name:               item.SourceName,
				Units:              item.Unit,
				SourceUnit:         item.Unit,
				NormalizedUnit:     item.Unit,
				ReportingFrequency: "Monthly",
				ZoneName:           item.ZoneName,
			})
		}
	}
	return out
}

func epath080UnitMultipliers(zones ...string) energyEffectiveMultiplierIndex {
	index := energyEffectiveMultiplierIndex{
		Enabled:          true,
		Zones:            map[string]energyZoneMultiplierRecord{},
		SpaceZones:       map[string]string{},
		OutputKeyZones:   map[string]string{},
		GroupMultipliers: map[string]float64{},
	}
	for _, zone := range zones {
		index.Zones[normalizePurposeToken(zone)] = energyZoneMultiplierRecord{ZoneName: zone, ZoneMultiplier: 1, GroupMultiplier: 1}
	}
	return index
}

func epath080AssertContribution(t *testing.T, result EnergyExplanationResult, zone string, period string, service string, category string, raw float64, effective float64, allocated float64) {
	t.Helper()
	nodes, links := epath080Graph(t, result, zone, period)
	nodeID := epath080DriverNodeID(category, service, zone)
	loadID := "load." + service + "." + epath080ScopeToken(zone)
	node := epath080NodeByID(nodes, nodeID)
	if node == nil {
		t.Errorf("%s/%s missing %s", zone, period, nodeID)
		return
	}
	if node.RawValue != raw || node.EffectiveValue != effective || node.AllocatedValue != allocated || node.Value != allocated {
		t.Errorf("%s/%s %s raw/effective/allocated/width = %g/%g/%g/%g, want %g/%g/%g/%g", zone, period, nodeID, node.RawValue, node.EffectiveValue, node.AllocatedValue, node.Value, raw, effective, allocated, allocated)
	}
	if node.Basis != "heat_balance_share" {
		t.Errorf("%s/%s %s basis = %q, want heat_balance_share", zone, period, nodeID, node.Basis)
	}
	link := epath080LinkByIDs(links, nodeID, loadID)
	if link == nil || link.FromValue != allocated || link.ToValue != allocated || link.Basis != "heat_balance_share" || link.ServiceKind != service {
		t.Errorf("%s/%s %s -> %s = %#v, want allocated width %g and heat_balance_share", zone, period, nodeID, loadID, link, allocated)
	}
}

func epath080AssertExactClosure(t *testing.T, result EnergyExplanationResult, zone string, period string, service string, wantLoad float64) {
	t.Helper()
	nodes, links := epath080Graph(t, result, zone, period)
	loadID := "load." + service + "." + epath080ScopeToken(zone)
	load := epath080NodeByID(nodes, loadID)
	if load == nil || load.Value != wantLoad {
		t.Errorf("%s/%s %s = %#v, want load %g", zone, period, loadID, load, wantLoad)
		return
	}
	incoming := 0.0
	for _, link := range links {
		if link.ToID == loadID && link.Relation == "driver_to_load" {
			incoming = roundedEnergyNumber(incoming + link.ToValue)
			if link.FromValue != link.ToValue {
				t.Errorf("%s/%s cross-domain values on same-domain driver link differ: %#v", zone, period, link)
			}
		}
	}
	if incoming != wantLoad {
		t.Errorf("%s/%s %s incoming allocated sum = %g, want exact load closure %g", zone, period, loadID, incoming, wantLoad)
	}
}

func epath080Graph(t *testing.T, result EnergyExplanationResult, zone string, period string) ([]EnergyExplanationNode, []EnergyPathLink) {
	t.Helper()
	if zone == "" {
		if period == "" || period == "annual" {
			return result.Nodes, result.Links
		}
		selected := epath080PeriodByID(result.Periods, period)
		if selected == nil {
			t.Fatalf("Building period %q is missing", period)
		}
		return selected.Nodes, selected.Links
	}
	for _, zoneResult := range result.ZoneResults {
		if !strings.EqualFold(zoneResult.Scope.ZoneName, zone) {
			continue
		}
		if period == "" || period == "annual" {
			return zoneResult.Nodes, zoneResult.Links
		}
		selected := epath080PeriodByID(zoneResult.Periods, period)
		if selected == nil {
			t.Fatalf("%s period %q is missing", zone, period)
		}
		return selected.Nodes, selected.Links
	}
	t.Fatalf("zone result %q is missing", zone)
	return nil, nil
}

func epath080ResultSignature(result EnergyExplanationResult) []string {
	out := []string{}
	appendGraph := func(scope string, period string, nodes []EnergyExplanationNode, links []EnergyPathLink) {
		for _, node := range nodes {
			if node.Level == "driver" || node.Level == "load" {
				out = append(out, fmt.Sprintf("N|%s|%s|%s|%.3f|%.3f|%.3f|%.3f", scope, period, node.ID, node.RawValue, node.EffectiveValue, node.AllocatedValue, node.Value))
			}
		}
		for _, link := range links {
			if link.Relation == "driver_to_load" {
				out = append(out, fmt.Sprintf("L|%s|%s|%s|%s|%.3f|%.3f", scope, period, link.FromID, link.ToID, link.FromValue, link.ToValue))
			}
		}
	}
	appendGraph("building", "annual", result.Nodes, result.Links)
	for _, period := range result.Periods {
		if period.Kind == "monthly" {
			appendGraph("building", period.ID, period.Nodes, period.Links)
		}
	}
	for _, zone := range result.ZoneResults {
		appendGraph(zone.Scope.ZoneName, "annual", zone.Nodes, zone.Links)
		for _, period := range zone.Periods {
			if period.Kind == "monthly" {
				appendGraph(zone.Scope.ZoneName, period.ID, period.Nodes, period.Links)
			}
		}
	}
	return out
}

func epath080DriverNodeID(category string, service string, zone string) string {
	return "driver." + canonicalEnergyPathCategory(category) + "." + service + "." + epath080ScopeToken(zone)
}

func epath080ScopeToken(zone string) string {
	if strings.TrimSpace(zone) == "" {
		return "building"
	}
	return metricID(zone)
}

func epath080NodeByID(nodes []EnergyExplanationNode, id string) *EnergyExplanationNode {
	for index := range nodes {
		if nodes[index].ID == id {
			return &nodes[index]
		}
	}
	return nil
}

func epath080LinkByIDs(links []EnergyPathLink, fromID string, toID string) *EnergyPathLink {
	for index := range links {
		if links[index].FromID == fromID && links[index].ToID == toID && links[index].Relation == "driver_to_load" {
			return &links[index]
		}
	}
	return nil
}

func epath080SourceByID(sources []EnergyDataSource, id string) *EnergyDataSource {
	for index := range sources {
		if sources[index].ID == id {
			return &sources[index]
		}
	}
	return nil
}

func epath080PeriodByID(periods []EnergyPeriod, id string) *EnergyPeriod {
	for index := range periods {
		if periods[index].ID == id {
			return &periods[index]
		}
	}
	return nil
}

func epath080RuleByID(rules []EnergyRelationshipRule, id string) *EnergyRelationshipRule {
	for index := range rules {
		if rules[index].ID == id {
			return &rules[index]
		}
	}
	return nil
}

func epath080Contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func epath080ExplainsNonCausalShare(value string) bool {
	normalized := strings.ToLower(value)
	nonCausal := strings.Contains(normalized, "non-causal") ||
		strings.Contains(normalized, "not a direct causal") ||
		strings.Contains(normalized, "not direct causal")
	return nonCausal && strings.Contains(normalized, "signed heat-balance share")
}

func epath080Sum(values map[int]float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return roundedEnergyNumber(total)
}
