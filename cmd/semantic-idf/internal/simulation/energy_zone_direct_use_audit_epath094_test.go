package simulation

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestEPATH094AuditZoneDirectBranchesUseOnlyObservedSources(t *testing.T) {
	input := epath094AuditSelectedZoneFixture()
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(input)
	after, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("v1 input mutated while projecting direct zone energy:\nbefore=%s\nafter=%s", before, after)
	}

	wantNodes := map[string]float64{
		"end_use.lighting.office":         11,
		"end_use.equipment.office":        12,
		"end_use.water_systems.office":    4,
		"end_use.other.office":            5,
		"end_use.cooling.office":          10,
		"carrier.electricity.office":      29,
		"carrier.natural_gas.office":      7,
		"carrier.propane.office":          2,
		"carrier.district_cooling.office": 4,
	}
	for id, want := range wantNodes {
		node := epath094AuditNode(result.Nodes, id)
		if node == nil || node.Value != want || node.Basis != "direct_zone_energy" || node.ZoneName != "Office" {
			t.Errorf("direct zone node %q = %#v, want value %.3f and direct_zone_energy", id, node, want)
		}
		if stringSliceContains(node.SourceIDs, "facility.electricity") || stringSliceContains(node.SourceIDs, "facility.gas") {
			t.Errorf("direct zone node %q claims facility provenance: %#v", id, node.SourceIDs)
		}
	}

	branches := []struct {
		from     string
		to       string
		value    float64
		sourceID string
		relation string
		otherIDs []string
	}{
		{"end_use.lighting.office", "carrier.electricity.office", 11, "zone.office.lights", "direct_end_use_to_carrier", []string{"zone.office.equipment.electric", "zone.office.hvac.cooling"}},
		{"end_use.equipment.office", "carrier.electricity.office", 7, "zone.office.equipment.electric", "direct_end_use_to_carrier", []string{"zone.office.equipment.gas", "zone.office.equipment.other"}},
		{"end_use.equipment.office", "carrier.natural_gas.office", 3, "zone.office.equipment.gas", "direct_end_use_to_carrier", []string{"zone.office.equipment.electric", "zone.office.equipment.other"}},
		{"end_use.equipment.office", "carrier.propane.office", 2, "zone.office.equipment.other", "direct_end_use_to_carrier", []string{"zone.office.equipment.electric", "zone.office.equipment.gas"}},
		{"end_use.water_systems.office", "carrier.natural_gas.office", 4, "zone.office.water", "direct_end_use_to_carrier", nil},
		{"end_use.other.office", "carrier.electricity.office", 5, "zone.office.process", "direct_end_use_to_carrier", nil},
		{"end_use.cooling.office", "carrier.electricity.office", 6, "zone.office.hvac.cooling", "end_use_to_carrier", []string{"zone.office.hvac.cooling.district"}},
		{"end_use.cooling.office", "carrier.district_cooling.office", 4, "zone.office.hvac.cooling.district", "end_use_to_carrier", []string{"zone.office.hvac.cooling"}},
	}
	for _, want := range branches {
		link := epath094AuditLink(result.Links, want.from, want.to)
		if link == nil || link.Relation != want.relation || link.Basis != "direct_zone_energy" || link.FromValue != want.value || link.ToValue != want.value || link.ZoneName != "Office" {
			t.Errorf("direct branch %s -> %s = %#v, want %s %.3f direct_zone_energy", want.from, want.to, link, want.relation, want.value)
			continue
		}
		if !stringSliceContains(link.SourceIDs, want.sourceID) {
			t.Errorf("direct branch %s -> %s lost exact source %q: %#v", want.from, want.to, want.sourceID, link.SourceIDs)
		}
		for _, otherID := range want.otherIDs {
			if stringSliceContains(link.SourceIDs, otherID) {
				t.Errorf("direct branch %s -> %s claims sibling source %q: %#v", want.from, want.to, otherID, link.SourceIDs)
			}
		}
	}

	conversion := epath094AuditLink(result.Links, "load.cooling.office", "end_use.cooling.office")
	if conversion == nil || conversion.Relation != "load_to_end_use" || conversion.Basis != "direct_zone_energy" ||
		conversion.FromValue != 18 || conversion.ToValue != 10 || conversion.Ratio != 1.8 || conversion.RatioKind != "load_to_site_energy" ||
		!stringSliceContains(conversion.SourceIDs, "zone.office.hvac.cooling") || !stringSliceContains(conversion.SourceIDs, "zone.office.load.cooling") {
		t.Errorf("exact-zone HVAC conversion = %#v, want 18 thermal -> 10 mixed-carrier site from direct component evidence", conversion)
	}
	for _, link := range result.Links {
		if link.Basis == "zone_load_allocation" || link.Basis == "service_path_allocation" {
			t.Errorf("EPATH-100 allocation leaked into direct-source EPATH-094 branch: %#v", link)
		}
	}
	correspondence := epath094AuditLink(result.Links, "driver.internal.lighting.cooling.office", "end_use.lighting.office")
	if correspondence == nil || correspondence.Relation != energyPathRelationSourceCorrespondence || correspondence.Basis != "direct_zone_energy" ||
		correspondence.FromValue != 8 || correspondence.ToValue != 11 ||
		!stringSliceContains(correspondence.SourceIDs, "zone.office.lights") || !stringSliceContains(correspondence.SourceIDs, "zone.office.heat.lights") ||
		stringSliceContains(correspondence.SourceIDs, "zone.office.equipment.electric") {
		t.Errorf("runtime direct-use/thermal correspondence = %#v", correspondence)
	}

	summary := buildEnergyExplanationSummary(result)
	for id, want := range map[string]float64{
		"end_use.lighting.office":         11,
		"end_use.equipment.office":        12,
		"carrier.electricity.office":      29,
		"carrier.natural_gas.office":      7,
		"carrier.propane.office":          2,
		"carrier.district_cooling.office": 4,
	} {
		item := epath094AuditSummaryItem(append(append([]EnergyExplanationSummaryItem{}, summary.EndUses...), summary.Carriers...), id)
		if item == nil || item.Value != want || item.Basis != "direct_zone_energy" {
			t.Errorf("direct zone summary %q = %#v, want %.3f direct_zone_energy", id, item, want)
		}
	}

	if result.Completeness.Status != "partial" || result.Completeness.EnergyUse.Status != "partial" || strings.TrimSpace(result.Completeness.EnergyUse.Message) == "" {
		t.Errorf("zone direct-use subtotal advertised as complete: %#v", result.Completeness)
	}
	if !epath094AuditHasWarning(result.Warnings, "zone_direct_energy_partial_coverage") {
		t.Errorf("missing partial direct-zone coverage warning: %#v", result.Warnings)
	}
	for carrier, want := range map[string]float64{"electricity": 29, "natural_gas": 7, "propane": 2, "district_cooling": 4} {
		reconciliation := epath094AuditCarrierReconciliation(result.Reconciliation, carrier)
		if reconciliation == nil || reconciliation.Status != "partial" || reconciliation.Basis != "direct_zone_energy" ||
			reconciliation.ExpectedValue != want || reconciliation.ExplainedValue != want || reconciliation.ResidualValue != 0 {
			t.Errorf("partial carrier reconciliation %q = %#v, want observed subtotal %.3f", carrier, reconciliation, want)
		}
		if stringSliceContains(reconciliation.SourceIDs, "facility.electricity") || stringSliceContains(reconciliation.SourceIDs, "facility.gas") {
			t.Errorf("zone carrier reconciliation %q claims a facility total: %#v", carrier, reconciliation.SourceIDs)
		}
	}
}

func TestEPATH094AuditMissingDirectUseCannotAllocateBuildingMeters(t *testing.T) {
	input := epath094AuditMissingDirectFixture()
	result := UpgradeEnergyExplanationV1(input)

	if len(result.AvailableZones) != 2 || result.AvailableZones[0] != "Lab" || result.AvailableZones[1] != "Office" {
		t.Fatalf("direct-only zone inventory = %#v, want Lab and Office from scoped thermal evidence", result.AvailableZones)
	}
	for _, id := range []string{
		"end_use.lighting.office",
		"end_use.equipment.office",
		"carrier.electricity.office",
	} {
		if node := epath094AuditNode(result.Nodes, id); node != nil {
			t.Errorf("missing direct source was fabricated from a building meter: %#v", node)
		}
	}
	for _, link := range result.Links {
		if strings.HasPrefix(link.FromID, "end_use.") || strings.HasPrefix(link.ToID, "carrier.") ||
			link.Basis == "zone_load_allocation" || link.Basis == "service_path_allocation" {
			t.Errorf("building site energy leaked into selected zone without a direct source: %#v", link)
		}
	}
	for _, sourceID := range []string{"facility.electricity", "building.lighting", "building.equipment"} {
		for _, node := range result.Nodes {
			if stringSliceContains(node.SourceIDs, sourceID) {
				t.Errorf("selected-zone graph retained unassignable building source %q on %#v", sourceID, node)
			}
		}
	}
	if result.Completeness.Status == "complete" || result.Completeness.EnergyUse.Status == "complete" {
		t.Errorf("zone with no direct site-energy evidence inherited complete Building coverage: %#v", result.Completeness)
	}
}

func TestEPATH094AuditZeroDirectObservationDoesNotCreateOrCompleteCarrier(t *testing.T) {
	input := EnergyExplanationV1{
		Schema:       energyExplanationV1Schema,
		Purpose:      string(SimulationPurposeBasicEnergy),
		Frequency:    "annual",
		Nodes:        []EnergyExplanationNode{{ID: "load.office.cooling", Level: "load", Kind: "load.zone_cooling", Label: "Cooling load", Value: 5, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"load.office"}}},
		Sources:      []EnergyDataSource{{ID: "zero.office.lights", SourceType: "sql_variable", Name: "Zone Lights Electricity Energy", ZoneName: "Office"}},
		Completeness: EnergyCompleteness{Status: "complete", EnergyUse: EnergyCompletenessLevel{Level: "energy", Status: "complete", Found: 1, Total: 1}},
		scope:        EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"},
		zoneDirectUseSeries: []energyExplanationSeries{
			epath094AuditDirectSeries("Office", "interior_lighting", "electricity", 0, "zero.office.lights"),
		},
	}
	result := UpgradeEnergyExplanationV1(input)
	for _, node := range result.Nodes {
		if node.Level == "end_use" || node.Level == "carrier" {
			t.Errorf("zero direct observation created a visible site-energy node: %#v", node)
		}
	}
	if result.Completeness.Status == "complete" || result.Completeness.EnergyUse.Status == "complete" {
		t.Errorf("zero-only direct observations advertised a complete zone carrier total: %#v", result.Completeness)
	}
}

func TestEPATH094AuditAnnualOnlyDirectSourceSurvivesMonthlyCanonicalFallback(t *testing.T) {
	input := EnergyExplanationV1{
		Schema:                energyExplanationV1Schema,
		Purpose:               string(SimulationPurposeBasicEnergy),
		Frequency:             "monthly",
		Periods:               []EnergyPeriod{{ID: "M2", Label: "M2", Kind: "monthly"}, {ID: "M1", Label: "M1", Kind: "monthly"}},
		Sources:               []EnergyDataSource{{ID: "annual.office.lights", SourceType: "sql_variable", Name: "Zone Lights Electricity Energy", ZoneName: "Office"}},
		Completeness:          EnergyCompleteness{Status: "complete", EnergyUse: EnergyCompletenessLevel{Level: "energy", Status: "complete", Found: 1, Total: 1}},
		scope:                 EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"},
		canonicalMonthlyBasis: true,
		zoneDirectUseSeries: []energyExplanationSeries{
			epath094AuditDirectSeries("Office", "interior_lighting", "electricity", 9, "annual.office.lights"),
		},
	}
	result := UpgradeEnergyExplanationV1(input)
	epath094AuditAssertZoneValue(t, result.Nodes, "end_use.lighting.office", 9)
	epath094AuditAssertZoneValue(t, result.Nodes, "carrier.electricity.office", 9)
	if link := epath094AuditLink(result.Links, "end_use.lighting.office", "carrier.electricity.office"); link == nil || link.FromValue != 9 || link.Basis != "direct_zone_energy" || !stringSliceContains(link.SourceIDs, "annual.office.lights") {
		t.Errorf("annual-only direct fallback link = %#v", link)
	}
	for _, period := range result.Periods {
		if period.ID == "M1" || period.ID == "M2" {
			for _, node := range period.Nodes {
				if node.Level == "end_use" || node.Level == "carrier" {
					t.Errorf("annual-only direct value was fabricated into %s: %#v", period.ID, node)
				}
			}
		}
	}
	if len(result.Periods) != 2 || result.Periods[0].ID != "M1" || result.Periods[1].ID != "M2" {
		t.Errorf("annual-only fallback changed deterministic interactive periods: %#v", result.Periods)
	}
}

func TestEPATH094AuditPartialMonthlyHVACUsesOnlyObservedTemporalOverlap(t *testing.T) {
	direct := epath094AuditDirectMonthlySeries("Office", "cooling", "electricity", 4, map[int]float64{1: 4}, "direct.office.cooling.partial")
	direct.ServiceKind = "cooling"
	direct.RelatedEntityIDs = []string{"zone.office", "component.office.cooling"}
	load := func(period string, value float64, sourceID string) EnergyExplanationNode {
		return EnergyExplanationNode{
			ID: "load.office.cooling", Level: "load", Kind: "load.zone_cooling", Label: "Office cooling load",
			Value: value, Unit: "kWh", Period: period, ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{sourceID},
		}
	}
	input := EnergyExplanationV1{
		Schema:                energyExplanationV1Schema,
		Purpose:               string(SimulationPurposeBasicEnergy),
		Frequency:             "monthly",
		Nodes:                 []EnergyExplanationNode{load("annual", 30, "load.office.annual")},
		Periods:               []EnergyPeriod{{ID: "M2", Label: "M2", Kind: "monthly", Nodes: []EnergyExplanationNode{load("M2", 20, "load.office.m2")}}, {ID: "M1", Label: "M1", Kind: "monthly", Nodes: []EnergyExplanationNode{load("M1", 10, "load.office.m1")}}},
		Sources:               []EnergyDataSource{{ID: "load.office.annual", ZoneName: "Office"}, {ID: "load.office.m1", ZoneName: "Office"}, {ID: "load.office.m2", ZoneName: "Office"}, {ID: "direct.office.cooling.partial", ZoneName: "Office"}},
		Completeness:          EnergyCompleteness{Status: "complete", EnergyUse: EnergyCompletenessLevel{Level: "energy", Status: "complete", Found: 1, Total: 1}},
		scope:                 EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"},
		canonicalMonthlyBasis: true,
		zoneDirectUseSeries:   []energyExplanationSeries{direct},
	}
	result := UpgradeEnergyExplanationV1(input)
	annual := epath094AuditLink(result.Links, "load.cooling.office", "end_use.cooling.office")
	if annual == nil || annual.FromValue != 10 || annual.ToValue != 4 || annual.Ratio != 2.5 ||
		annual.Basis != "direct_zone_energy" || !strings.Contains(annual.Explanation, "partial temporal coverage") ||
		!stringSliceContains(annual.SourceIDs, "load.office.m1") || stringSliceContains(annual.SourceIDs, "load.office.m2") {
		t.Errorf("partial-month annual HVAC conversion = %#v, want observed overlap 10 thermal -> 4 site", annual)
	}
	m1 := energyExplanationPeriodByID(result.Periods, "M1")
	m2 := energyExplanationPeriodByID(result.Periods, "M2")
	if m1 == nil || m2 == nil {
		t.Fatalf("partial-month periods missing: M1=%#v M2=%#v", m1, m2)
	}
	if link := epath094AuditLink(m1.Links, "load.cooling.office", "end_use.cooling.office"); link == nil || link.FromValue != 10 || link.ToValue != 4 || link.Ratio != 2.5 {
		t.Errorf("observed M1 HVAC conversion = %#v", link)
	}
	if node := epath094AuditNode(m2.Nodes, "end_use.cooling.office"); node != nil {
		t.Errorf("missing M2 direct observation fabricated an HVAC end use: %#v", node)
	}
	if link := epath094AuditLink(m2.Links, "load.cooling.office", "end_use.cooling.office"); link != nil {
		t.Errorf("missing M2 direct observation fabricated an HVAC conversion: %#v", link)
	}
	if node := epath094AuditNode(result.Nodes, "load.cooling.office"); node == nil || node.Value != 30 {
		t.Errorf("partial direct coverage changed the full annual load endpoint: %#v", node)
	}
}

func TestEPATH094AuditStoredZoneQualifiedEnergyIsPromotedInZoneScope(t *testing.T) {
	input := EnergyExplanationV1{
		Schema:    energyExplanationV1Schema,
		Purpose:   string(SimulationPurposeBasicEnergy),
		Frequency: "annual",
		Nodes: []EnergyExplanationNode{
			{ID: "stored.zone.carrier", Level: "energy", Kind: "energy.electricity.direct_zone_subtotal", Label: "Observed electricity", Value: 5, Unit: "kWh", ZoneName: "Office", Carrier: "electricity", EndUse: "total", MeterHierarchyLevel: "zone_direct_subtotal", SourceIDs: []string{"stored.office.lights"}},
			{ID: "stored.zone.lights", Level: "energy", Kind: "energy.interior_lighting", Label: "Office lights", Value: 5, Unit: "kWh", ZoneName: "Office", Carrier: "electricity", EndUse: "interior_lighting", MeterHierarchyLevel: "zone_direct_use", SourceIDs: []string{"stored.office.lights"}},
		},
		Edges:   []EnergyExplanationEdge{{ID: "stored.zone.edge", FromID: "stored.zone.carrier", ToID: "stored.zone.lights", Value: 5, Unit: "kWh", Relation: "energy_variable", Basis: "measured_energy_variable", RuleID: energyRelationshipRuleMeasuredEnergyVariable, SourceIDs: []string{"stored.office.lights"}, ZoneName: "Office"}},
		Sources: []EnergyDataSource{{ID: "stored.office.lights", SourceType: "stored_v1", Name: "Zone Lights Electricity Energy", ZoneName: "Office"}},
		scope:   EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"},
	}
	zone := UpgradeEnergyExplanationV1(input)
	epath094AuditAssertZoneValue(t, zone.Nodes, "end_use.lighting.office", 5)
	epath094AuditAssertZoneValue(t, zone.Nodes, "carrier.electricity.office", 5)
	if link := epath094AuditLink(zone.Links, "end_use.lighting.office", "carrier.electricity.office"); link == nil || link.Relation != "direct_end_use_to_carrier" || link.Basis != "direct_zone_energy" {
		t.Errorf("stored direct-zone link = %#v", link)
	}
}

func TestEPATH094AuditMonthlyZonesAreOrderInvariantAndBuildingTotalsStayFrozen(t *testing.T) {
	forwardInput := epath094AuditBuildingMonthlyFixture()
	baselineInput := epath094AuditWithoutZoneDirect(forwardInput)
	before, err := json.Marshal(forwardInput)
	if err != nil {
		t.Fatal(err)
	}
	forward := UpgradeEnergyExplanationV1(forwardInput)
	after, err := json.Marshal(forwardInput)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("v1 JSON changed during zone projection:\nbefore=%s\nafter=%s", before, after)
	}
	baseline := UpgradeEnergyExplanationV1(baselineInput)
	if got, want := epath094AuditBuildingSnapshot(forward), epath094AuditBuildingSnapshot(baseline); !reflect.DeepEqual(got, want) {
		t.Fatalf("zone-direct observations changed the frozen Building graph:\nwith direct=%#v\nbaseline=%#v", got, want)
	}
	if forward.Completeness.Status != "complete" || forward.Completeness.MappedPercent != 100 ||
		forward.Completeness.EnergyUse.Status != "complete" || forward.Completeness.EnergyUse.Found != 1 || forward.Completeness.EnergyUse.Total != 1 {
		t.Fatalf("Building completeness changed while deriving zone direct-use views: %#v", forward.Completeness)
	}
	for _, sourceID := range []string{"direct.office.lights", "direct.office.equipment.electric", "direct.office.equipment.gas", "direct.lab.lights", "direct.lab.equipment.electric", "direct.lab.equipment.gas"} {
		if epath094AuditResultClaimsSource(forward.Nodes, forward.Links, forward.Reconciliation, sourceID) {
			t.Errorf("Building graph double-counts zone direct source %q", sourceID)
		}
	}

	reverseInput := epath094AuditBuildingMonthlyFixture()
	epath094AuditReverse(reverseInput.Nodes)
	epath094AuditReverse(reverseInput.Edges)
	epath094AuditReverse(reverseInput.Periods)
	epath094AuditReverse(reverseInput.Sources)
	epath094AuditReverse(reverseInput.zoneDirectUseSeries)
	for index := range reverseInput.zoneDirectUseSeries {
		epath094AuditReverse(reverseInput.zoneDirectUseSeries[index].SourceIDs)
		epath094AuditReverse(reverseInput.zoneDirectUseSeries[index].AnnualSourceIDs)
		epath094AuditReverse(reverseInput.zoneDirectUseSeries[index].MonthlySourceIDs)
		epath094AuditReverse(reverseInput.zoneDirectUseSeries[index].RelatedEntityIDs)
	}
	for index := range reverseInput.Periods {
		epath094AuditReverse(reverseInput.Periods[index].Nodes)
		epath094AuditReverse(reverseInput.Periods[index].Edges)
	}
	reverse := UpgradeEnergyExplanationV1(reverseInput)
	if got, want := epath094AuditZoneSnapshot(forward), epath094AuditZoneSnapshot(reverse); !reflect.DeepEqual(got, want) {
		t.Fatalf("zone direct projection depends on source/order input:\nforward=%#v\nreverse=%#v", got, want)
	}
	for _, result := range []EnergyExplanationResult{forward, reverse} {
		for _, zone := range result.ZoneResults {
			epath094AuditAssertDirectZoneCompleteness(t, zone.Scope.ZoneName+" annual", zone.Completeness)
			periodIDs := make([]string, 0, len(zone.Periods))
			for _, period := range zone.Periods {
				periodIDs = append(periodIDs, period.ID)
				if period.Summary == nil {
					t.Errorf("%s %s summary missing", zone.Scope.ZoneName, period.ID)
				} else {
					epath094AuditAssertDirectZoneCompleteness(t, zone.Scope.ZoneName+" "+period.ID, period.Summary.Completeness)
				}
			}
			if !reflect.DeepEqual(periodIDs, []string{"M1", "M2"}) {
				t.Errorf("%s zone period order = %#v, want deterministic M1/M2", zone.Scope.ZoneName, periodIDs)
			}
			epath094AuditAssertSortedProvenance(t, zone.Nodes, zone.Links, zone.Reconciliation)
			for _, period := range zone.Periods {
				epath094AuditAssertSortedProvenance(t, period.Nodes, period.Links, period.Reconciliation)
			}
		}
	}

	if !reflect.DeepEqual(forward.AvailableZones, []string{"Lab", "Office"}) || len(forward.ZoneResults) != 2 {
		t.Fatalf("direct-only zones not exposed for scope switching: zones=%#v results=%#v", forward.AvailableZones, forward.ZoneResults)
	}
	wants := map[string]struct {
		lighting    float64
		equipment   float64
		electricity float64
		gas         float64
	}{
		"Office": {lighting: 3, equipment: 12, electricity: 10, gas: 5},
		"Lab":    {lighting: 7, equipment: 22, electricity: 20, gas: 9},
	}
	for _, zone := range forward.ZoneResults {
		want, ok := wants[zone.Scope.ZoneName]
		if !ok {
			t.Errorf("unexpected zone result %#v", zone.Scope)
			continue
		}
		scopeToken := metricID(zone.Scope.ZoneName)
		epath094AuditAssertZoneValue(t, zone.Nodes, "end_use.lighting."+scopeToken, want.lighting)
		epath094AuditAssertZoneValue(t, zone.Nodes, "end_use.equipment."+scopeToken, want.equipment)
		epath094AuditAssertZoneValue(t, zone.Nodes, "carrier.electricity."+scopeToken, want.electricity)
		epath094AuditAssertZoneValue(t, zone.Nodes, "carrier.natural_gas."+scopeToken, want.gas)
		for _, month := range []struct {
			id          string
			lighting    float64
			electricity float64
			gas         float64
		}{
			{id: "M1", lighting: map[string]float64{"Office": 1, "Lab": 3}[zone.Scope.ZoneName], electricity: map[string]float64{"Office": 4, "Lab": 9}[zone.Scope.ZoneName], gas: map[string]float64{"Office": 2, "Lab": 3}[zone.Scope.ZoneName]},
			{id: "M2", lighting: map[string]float64{"Office": 2, "Lab": 4}[zone.Scope.ZoneName], electricity: map[string]float64{"Office": 6, "Lab": 11}[zone.Scope.ZoneName], gas: map[string]float64{"Office": 3, "Lab": 6}[zone.Scope.ZoneName]},
		} {
			period := energyExplanationPeriodByID(zone.Periods, month.id)
			if period == nil {
				t.Errorf("%s missing %s", zone.Scope.ZoneName, month.id)
				continue
			}
			epath094AuditAssertZoneValue(t, period.Nodes, "end_use.lighting."+scopeToken, month.lighting)
			epath094AuditAssertZoneValue(t, period.Nodes, "carrier.electricity."+scopeToken, month.electricity)
			epath094AuditAssertZoneValue(t, period.Nodes, "carrier.natural_gas."+scopeToken, month.gas)
		}
		annualElectricity := epath094AuditNode(zone.Nodes, "carrier.electricity."+scopeToken)
		m1Electricity := epath094AuditNode(energyExplanationPeriodByID(zone.Periods, "M1").Nodes, "carrier.electricity."+scopeToken)
		m2Electricity := epath094AuditNode(energyExplanationPeriodByID(zone.Periods, "M2").Nodes, "carrier.electricity."+scopeToken)
		if annualElectricity == nil || m1Electricity == nil || m2Electricity == nil || annualElectricity.Value != m1Electricity.Value+m2Electricity.Value {
			t.Errorf("%s annual direct electricity does not equal monthly sum: annual=%#v M1=%#v M2=%#v", zone.Scope.ZoneName, annualElectricity, m1Electricity, m2Electricity)
		}
	}
}

func TestEPATH094AuditSanitizedZoneTokenCollisionStaysIsolated(t *testing.T) {
	if metricID("Office-A") != metricID("Office A") {
		t.Fatal("fixture no longer exercises a sanitized zone-token collision")
	}
	input := EnergyExplanationV1{
		Schema:    energyExplanationV1Schema,
		Purpose:   string(SimulationPurposeBasicEnergy),
		Frequency: "annual",
		Sources: []EnergyDataSource{
			{ID: "direct.office-dash.lights", SourceType: "fixture", Name: "Zone Lights Electricity Energy", ZoneName: "Office-A"},
			{ID: "direct.office-space.lights", SourceType: "fixture", Name: "Zone Lights Electricity Energy", ZoneName: "Office A"},
		},
		zoneDirectUseSeries: []energyExplanationSeries{
			epath094AuditDirectSeries("Office-A", "interior_lighting", "electricity", 5, "direct.office-dash.lights"),
			epath094AuditDirectSeries("Office A", "interior_lighting", "electricity", 9, "direct.office-space.lights"),
		},
	}
	result := UpgradeEnergyExplanationV1(input)
	if !reflect.DeepEqual(result.AvailableZones, []string{"Office A", "Office-A"}) || len(result.ZoneResults) != 2 {
		t.Fatalf("colliding zone inventory = %#v / %d results", result.AvailableZones, len(result.ZoneResults))
	}
	wants := map[string]struct {
		value   float64
		source  string
		sibling string
	}{
		"Office-A": {value: 5, source: "direct.office-dash.lights", sibling: "direct.office-space.lights"},
		"Office A": {value: 9, source: "direct.office-space.lights", sibling: "direct.office-dash.lights"},
	}
	for _, zone := range result.ZoneResults {
		want, ok := wants[zone.Scope.ZoneName]
		if !ok {
			t.Fatalf("unexpected colliding zone result: %#v", zone.Scope)
		}
		scopeToken := metricID(zone.Scope.ZoneName)
		for _, id := range []string{"end_use.lighting." + scopeToken, "carrier.electricity." + scopeToken} {
			node := epath094AuditNode(zone.Nodes, id)
			if node == nil || node.Value != want.value || !reflect.DeepEqual(node.SourceIDs, []string{want.source}) || stringSliceContains(node.SourceIDs, want.sibling) {
				t.Errorf("zone-token collision contaminated %q (%s): %#v", id, zone.Scope.ZoneName, node)
			}
		}
		link := epath094AuditLink(zone.Links, "end_use.lighting."+scopeToken, "carrier.electricity."+scopeToken)
		if link == nil || link.FromValue != want.value || !reflect.DeepEqual(link.SourceIDs, []string{want.source}) || stringSliceContains(link.SourceIDs, want.sibling) {
			t.Errorf("zone-token collision contaminated link for %s: %#v", zone.Scope.ZoneName, link)
		}
	}
	for _, sourceID := range []string{"direct.office-dash.lights", "direct.office-space.lights"} {
		if epath094AuditResultClaimsSource(result.Nodes, result.Links, result.Reconciliation, sourceID) {
			t.Errorf("runtime sidecar source %q leaked into Building graph", sourceID)
		}
	}
}

func TestEPATH094AuditZoneDirectVariablesUseEffectiveZoneMultiplier(t *testing.T) {
	base := energyExplanationSeries{
		Stage:               "end_use",
		CanonicalKind:       "energy.interior_lighting",
		Level:               "energy",
		Kind:                "energy.interior_lighting",
		Label:               "Zone interior lighting",
		Unit:                "kWh",
		Carrier:             "electricity",
		EndUse:              "interior_lighting",
		MeterHierarchyLevel: "zone_direct_use",
		ZoneName:            "Office",
		SourceKey:           "Office",
		SourceName:          "Zone Lights Electricity Energy",
		SourceIDs:           []string{"direct.office.lights"},
		RawTotal:            2,
		RawMonthly:          map[int]float64{1: 0.5, 2: 1.5},
		Total:               2,
		Monthly:             map[int]float64{1: 0.5, 2: 1.5},
	}
	sources := []EnergyDataSource{{ID: "direct.office.lights", Name: "Zone Lights Electricity Energy", KeyValue: "Office", ZoneName: "Office"}}
	index := energyEffectiveMultiplierIndex{
		Enabled:          true,
		Zones:            map[string]energyZoneMultiplierRecord{normalizePurposeToken("Office"): {ZoneName: "Office", ZoneMultiplier: 2, GroupMultiplier: 3}},
		SpaceZones:       map[string]string{},
		OutputKeyZones:   map[string]string{},
		GroupMultipliers: map[string]float64{},
	}

	for _, test := range []struct {
		name      string
		hierarchy string
		basis     string
	}{
		{name: "case-insensitive hierarchy", hierarchy: "ZoNe_DiReCt_UsE"},
		{name: "basis-only direct evidence", basis: "direct_zone_energy"},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := base
			item.MeterHierarchyLevel = test.hierarchy
			item.Basis = test.basis
			got, gotSources, warnings := applyEnergyExplanationMultipliers([]energyExplanationSeries{item}, sources, index)
			if len(got) != 1 || got[0].Total != 12 || got[0].Monthly[1] != 3 || got[0].Monthly[2] != 9 ||
				got[0].EffectiveMultiplier != 6 || got[0].MultiplierApplication != energyMultiplierRequiresZone {
				t.Fatalf("zone direct-use multiplier result = %#v, want raw 2 x effective multiplier 6", got)
			}
			if len(warnings) != 0 {
				t.Errorf("resolved direct-use multiplier emitted warnings: %#v", warnings)
			}
			if len(gotSources) != 1 || gotSources[0].RawValue != 2 || gotSources[0].EffectiveValue != 12 ||
				gotSources[0].EffectiveMultiplier != 6 || gotSources[0].MultiplierApplication != energyMultiplierRequiresZone {
				t.Errorf("zone direct-use source multiplier trace = %#v", gotSources)
			}
		})
	}
}

func TestEPATH094AuditEnergyPlusZoneDirectAliasesRemainExplicit(t *testing.T) {
	tests := []struct {
		name    string
		carrier string
		endUse  string
	}{
		{name: "Zone Lights Electricity Energy", carrier: "electricity", endUse: "interior_lighting"},
		{name: "Zone Lights Electric Energy", carrier: "electricity", endUse: "interior_lighting"},
		{name: "Zone Electric Equipment Electricity Energy", carrier: "electricity", endUse: "interior_equipment"},
		{name: "Zone Gas Equipment NaturalGas Energy", carrier: "natural_gas", endUse: "interior_equipment"},
		{name: "Zone Gas Equipment Gas Energy", carrier: "natural_gas", endUse: "interior_equipment"},
		{name: "Zone Other Equipment Fuel Energy", carrier: "other", endUse: "interior_equipment"},
		{name: "Zone Hot Water Equipment District Heating Energy", carrier: "district_heating", endUse: "interior_equipment"},
		{name: "Zone Steam Equipment District Heating Energy", carrier: "district_heating", endUse: "interior_equipment"},
	}
	for _, test := range tests {
		definition, ok := energyVariableAliasDefinitionForName(test.name)
		if !ok || definition.HierarchyLevel != "zone_direct_use" || definition.Carrier != test.carrier || definition.EndUse != test.endUse {
			t.Errorf("zone direct-use alias %q = %#v / found=%t", test.name, definition, ok)
		}
	}
}

func TestEPATH094AuditSQLZoneOutputsReachSelectedZoneV2Graph(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	epath094AuditCreateZoneDirectSQL(t, path)
	plan := PurposeRunPlan{
		BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath,
		ZoneMode:          "selected",
		ZoneNames:         []string{"Office"},
		AllocationPolicy:  PurposeAllocationPolicyDirectOnly,
	}
	legacy, err := parseSimulationEnergyExplanationSQL(path, &plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.zoneDirectUseSeries) != 8 {
		t.Fatalf("SQL direct-use capture = %d series, want 4 categories in each of two zones: %#v", len(legacy.zoneDirectUseSeries), legacy.zoneDirectUseSeries)
	}
	for _, node := range legacy.Nodes {
		if node.MeterHierarchyLevel == "zone_direct_use" {
			t.Fatalf("runtime direct series altered the frozen v1 graph: %#v", node)
		}
	}

	result := UpgradeEnergyExplanationV1(legacy)
	for id, want := range map[string]float64{
		"end_use.lighting.office":    3,
		"end_use.equipment.office":   14,
		"carrier.electricity.office": 10,
		"carrier.natural_gas.office": 5,
		"carrier.other.office":       2,
	} {
		epath094AuditAssertZoneValue(t, result.Nodes, id, want)
	}
	for _, node := range result.Nodes {
		if strings.EqualFold(node.ZoneName, "Lab") || strings.Contains(node.ID, ".lab") {
			t.Errorf("Lab SQL direct source leaked into selected Office graph: %#v", node)
		}
		if stringSliceContains(node.SourceIDs, "sql-rdd-1") || stringSliceContains(node.SourceIDs, "sql-rdd-2") {
			t.Errorf("building facility/broad meter leaked into direct Office subtotal: %#v", node)
		}
	}
	for _, month := range []struct {
		id          string
		lighting    float64
		equipment   float64
		electricity float64
		gas         float64
		other       float64
	}{
		{id: "M1", lighting: 1, equipment: 6, electricity: 4, gas: 2, other: 1},
		{id: "M2", lighting: 2, equipment: 8, electricity: 6, gas: 3, other: 1},
	} {
		period := energyExplanationPeriodByID(result.Periods, month.id)
		if period == nil {
			t.Errorf("missing SQL-backed %s period", month.id)
			continue
		}
		epath094AuditAssertZoneValue(t, period.Nodes, "end_use.lighting.office", month.lighting)
		epath094AuditAssertZoneValue(t, period.Nodes, "end_use.equipment.office", month.equipment)
		epath094AuditAssertZoneValue(t, period.Nodes, "carrier.electricity.office", month.electricity)
		epath094AuditAssertZoneValue(t, period.Nodes, "carrier.natural_gas.office", month.gas)
		epath094AuditAssertZoneValue(t, period.Nodes, "carrier.other.office", month.other)
	}
	if !reflect.DeepEqual(result.AvailableZones, []string{"Lab", "Office"}) {
		t.Errorf("SQL direct-use zone inventory = %#v, want Lab and Office", result.AvailableZones)
	}
}

func epath094AuditSelectedZoneFixture() EnergyExplanationV1 {
	nodes := []EnergyExplanationNode{
		{ID: "load.office.cooling", Level: "load", Kind: "load.zone_cooling", Label: "Office cooling load", Value: 18, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "cooling", Basis: "measured_energy_variable", SourceIDs: []string{"zone.office.load.cooling"}},
		{ID: "facility.electricity", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Value: 100, Unit: "kWh", Carrier: "electricity", EndUse: "total", Basis: "measured_meter", SourceIDs: []string{"facility.electricity"}},
		{ID: "facility.gas", Level: "energy", Kind: "energy.gas.total", Label: "Natural gas", Value: 50, Unit: "kWh", Carrier: "natural_gas", EndUse: "total", Basis: "measured_meter", SourceIDs: []string{"facility.gas"}},
		{ID: "heat.office.lighting", Level: "heat", Kind: "heat.lighting", Label: "Lighting heat", Value: 8, DisplayValue: 8, SignedValue: 8, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "cooling", DriverCategory: energyDriverCategoryLighting, SourceIDs: []string{"zone.office.heat.lights"}},
	}
	return EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Nodes:            nodes,
		Edges: []EnergyExplanationEdge{{
			ID: "heat.office.lighting", FromID: "load.office.cooling", ToID: "heat.office.lighting", Value: 8, DisplayValue: 8, SignedValue: 8,
			Unit: "kWh thermal", Relation: "heat_driver", Basis: "heat_balance_share", RuleID: energyRelationshipRuleHeatDriverBalance,
			ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"zone.office.heat.lights"},
		}},
		Sources: []EnergyDataSource{
			{ID: "zone.office.lights", SourceType: "sql_variable", Name: "Zone Lights Electricity Energy", ZoneName: "Office"},
			{ID: "zone.office.equipment.electric", SourceType: "sql_variable", Name: "Zone Electric Equipment Electricity Energy", ZoneName: "Office"},
			{ID: "zone.office.equipment.gas", SourceType: "sql_variable", Name: "Zone Gas Equipment NaturalGas Energy", ZoneName: "Office"},
			{ID: "zone.office.equipment.other", SourceType: "sql_variable", Name: "Zone Other Equipment Propane Energy", ZoneName: "Office"},
			{ID: "zone.office.water", SourceType: "sql_variable", Name: "Office zone-keyed water heating energy", ZoneName: "Office"},
			{ID: "zone.office.process", SourceType: "sql_variable", Name: "Office zone-keyed process energy", ZoneName: "Office"},
			{ID: "zone.office.hvac.cooling", SourceType: "sql_variable", Name: "Office direct cooling component electricity", ZoneName: "Office", RelatedEntityIDs: []string{"zone.office", "component.office.cooling"}},
			{ID: "zone.office.hvac.cooling.district", SourceType: "sql_variable", Name: "Office direct cooling component district energy", ZoneName: "Office", RelatedEntityIDs: []string{"zone.office", "component.office.cooling"}},
			{ID: "zone.office.load.cooling", SourceType: "sql_variable", Name: "Zone Air System Sensible Cooling Energy", ZoneName: "Office"},
			{ID: "zone.office.heat.lights", SourceType: "sql_variable", Name: "Zone Lights Total Heating Energy", ZoneName: "Office", DriverRole: energyDriverSourceRoleMainFlow, DriverCategory: energyDriverCategoryLighting},
			{ID: "facility.electricity", SourceType: "sql_meter", Name: "Electricity:Facility"},
			{ID: "facility.gas", SourceType: "sql_meter", Name: "NaturalGas:Facility"},
		},
		Completeness: EnergyCompleteness{Status: "complete", EnergyUse: EnergyCompletenessLevel{Level: "energy", Status: "complete", Found: 1, Total: 1}},
		scope:        EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"},
		zoneDirectUseSeries: []energyExplanationSeries{
			epath094AuditDirectSeries("Office", "interior_lighting", "electricity", 11, "zone.office.lights"),
			epath094AuditDirectSeries("Office", "interior_equipment", "electricity", 7, "zone.office.equipment.electric"),
			epath094AuditDirectSeries("Office", "interior_equipment", "natural_gas", 3, "zone.office.equipment.gas"),
			epath094AuditDirectSeries("Office", "interior_equipment", "propane", 2, "zone.office.equipment.other"),
			epath094AuditDirectSeries("Office", "water_systems", "natural_gas", 4, "zone.office.water"),
			epath094AuditDirectSeries("Office", "process", "electricity", 5, "zone.office.process"),
			epath094AuditDirectHVACSeries("Office", "cooling", "electricity", 6, "zone.office.hvac.cooling"),
			epath094AuditDirectHVACSeries("Office", "cooling", "district_cooling", 4, "zone.office.hvac.cooling.district"),
		},
	}
}

func epath094AuditCreateZoneDirectSQL(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE ReportDataDictionary (
			ReportDataDictionaryIndex INTEGER PRIMARY KEY,
			KeyValue TEXT,
			Name TEXT,
			Units TEXT,
			IsMeter INTEGER,
			ReportingFrequency TEXT,
			IndexGroup TEXT
		)`,
		`CREATE TABLE "Time" (
			TimeIndex INTEGER PRIMARY KEY,
			Month INTEGER,
			Day INTEGER,
			Hour INTEGER,
			Minute INTEGER
		)`,
		`CREATE TABLE ReportData (
			ReportDataIndex INTEGER PRIMARY KEY,
			TimeIndex INTEGER,
			ReportDataDictionaryIndex INTEGER,
			Value REAL
		)`,
		`INSERT INTO ReportDataDictionary VALUES
			(1, '', 'Electricity:Facility', 'J', 1, 'Monthly', 'Meter'),
			(2, '', 'InteriorLights:Electricity', 'J', 1, 'Monthly', 'Meter'),
			(3, 'Office', 'Zone Lights Electricity Energy', 'J', 0, 'Monthly', 'Zone'),
			(4, 'Office', 'Zone Electric Equipment Electricity Energy', 'J', 0, 'Monthly', 'Zone'),
			(5, 'Office', 'Zone Gas Equipment NaturalGas Energy', 'J', 0, 'Monthly', 'Zone'),
			(6, 'Office', 'Zone Other Equipment Fuel Energy', 'J', 0, 'Monthly', 'Zone'),
			(7, 'Lab', 'Zone Lights Electric Energy', 'J', 0, 'Monthly', 'Zone'),
			(8, 'Lab', 'Zone Electric Equipment Electric Energy', 'J', 0, 'Monthly', 'Zone'),
			(9, 'Lab', 'Zone Gas Equipment Gas Energy', 'J', 0, 'Monthly', 'Zone'),
			(10, 'Lab', 'Other Equipment Fuel Energy', 'J', 0, 'Monthly', 'Zone')`,
		`INSERT INTO "Time" VALUES
			(1, 1, 31, 24, 0),
			(2, 2, 28, 24, 0)`,
		`INSERT INTO ReportData VALUES
			(1, 1, 1, 360000000), (2, 2, 1, 360000000),
			(3, 1, 2, 144000000), (4, 2, 2, 144000000),
			(5, 1, 3, 3600000), (6, 2, 3, 7200000),
			(7, 1, 4, 10800000), (8, 2, 4, 14400000),
			(9, 1, 5, 7200000), (10, 2, 5, 10800000),
			(11, 1, 6, 3600000), (12, 2, 6, 3600000),
			(13, 1, 7, 3600000000), (14, 2, 7, 3600000000),
			(15, 1, 8, 3600000000), (16, 2, 8, 3600000000),
			(17, 1, 9, 3600000000), (18, 2, 9, 3600000000),
			(19, 1, 10, 3600000000), (20, 2, 10, 3600000000)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("zone direct SQL fixture failed: %v\n%s", err, statement)
		}
	}
}

func epath094AuditMissingDirectFixture() EnergyExplanationV1 {
	nodes := []EnergyExplanationNode{
		{ID: "energy.carrier.electricity", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Value: 100, Unit: "kWh", Carrier: "electricity", EndUse: "total", Basis: "measured_meter", SourceIDs: []string{"facility.electricity"}},
		{ID: "energy.building.lighting", Level: "energy", Kind: "energy.interior_lighting", Label: "Building lighting", Value: 40, Unit: "kWh", Carrier: "electricity", EndUse: "interior_lighting", Basis: "measured_meter", SourceIDs: []string{"building.lighting"}},
		{ID: "energy.building.equipment", Level: "energy", Kind: "energy.interior_equipment", Label: "Building equipment", Value: 20, Unit: "kWh", Carrier: "electricity", EndUse: "interior_equipment", Basis: "measured_meter", SourceIDs: []string{"building.equipment"}},
		{ID: "load.office.cooling", Level: "load", Kind: "load.zone_cooling", Label: "Office load", Value: 25, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"load.office"}},
		{ID: "load.lab.cooling", Level: "load", Kind: "load.zone_cooling", Label: "Lab load", Value: 75, Unit: "kWh", ZoneName: "Lab", ServiceKind: "cooling", SourceIDs: []string{"load.lab"}},
	}
	return EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyByZoneLoadShare,
		Nodes:            nodes,
		Edges: []EnergyExplanationEdge{
			{ID: "meter.lighting", FromID: nodes[0].ID, ToID: nodes[1].ID, Value: 40, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"building.lighting"}},
			{ID: "meter.equipment", FromID: nodes[0].ID, ToID: nodes[2].ID, Value: 20, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"building.equipment"}},
			// These malformed historical allocation edges deliberately tempt the
			// old generic policy to assign non-HVAC building meters by load share.
			{ID: "allocate.lighting.office", FromID: nodes[1].ID, ToID: nodes[3].ID, Value: 10, Unit: "kWh", Relation: "allocation", Basis: "zone_load_allocation", RuleID: energyRelationshipRuleAllocatedZoneLoad, ServiceKind: "cooling"},
			{ID: "allocate.lighting.lab", FromID: nodes[1].ID, ToID: nodes[4].ID, Value: 30, Unit: "kWh", Relation: "allocation", Basis: "zone_load_allocation", RuleID: energyRelationshipRuleAllocatedZoneLoad, ServiceKind: "cooling"},
			{ID: "allocate.equipment.office", FromID: nodes[2].ID, ToID: nodes[3].ID, Value: 5, Unit: "kWh", Relation: "allocation", Basis: "zone_load_allocation", RuleID: energyRelationshipRuleAllocatedZoneLoad, ServiceKind: "cooling"},
			{ID: "allocate.equipment.lab", FromID: nodes[2].ID, ToID: nodes[4].ID, Value: 15, Unit: "kWh", Relation: "allocation", Basis: "zone_load_allocation", RuleID: energyRelationshipRuleAllocatedZoneLoad, ServiceKind: "cooling"},
		},
		Sources:      []EnergyDataSource{{ID: "facility.electricity"}, {ID: "building.lighting"}, {ID: "building.equipment"}, {ID: "load.office", ZoneName: "Office"}, {ID: "load.lab", ZoneName: "Lab"}},
		Completeness: EnergyCompleteness{Status: "complete", EnergyUse: EnergyCompletenessLevel{Level: "energy", Status: "complete", Found: 1, Total: 1}},
		scope:        EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"},
	}
}

func epath094AuditBuildingMonthlyFixture() EnergyExplanationV1 {
	period := func(id string, values map[string]float64) EnergyPeriod {
		nodes, edges := epath094AuditBuildingPeriod(id, values)
		return EnergyPeriod{ID: id, Label: id, Kind: "monthly", Nodes: nodes, Edges: edges}
	}
	annualValues := map[string]float64{
		"facility.electricity": 200, "facility.gas": 80, "building.lighting": 80, "building.equipment.electric": 40,
		"building.equipment.gas": 20, "building.cooling": 30, "direct.office.lights": 3, "direct.office.equipment.electric": 7,
		"direct.office.equipment.gas": 5, "direct.lab.lights": 7, "direct.lab.equipment.electric": 13, "direct.lab.equipment.gas": 9,
	}
	annualNodes, annualEdges := epath094AuditBuildingPeriod("annual", annualValues)
	return EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "monthly",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Nodes:            annualNodes,
		Edges:            annualEdges,
		Periods: []EnergyPeriod{
			period("M1", map[string]float64{
				"facility.electricity": 90, "facility.gas": 30, "building.lighting": 35, "building.equipment.electric": 15,
				"building.equipment.gas": 8, "building.cooling": 10, "direct.office.lights": 1, "direct.office.equipment.electric": 3,
				"direct.office.equipment.gas": 2, "direct.lab.lights": 3, "direct.lab.equipment.electric": 6, "direct.lab.equipment.gas": 3,
			}),
			period("M2", map[string]float64{
				"facility.electricity": 110, "facility.gas": 50, "building.lighting": 45, "building.equipment.electric": 25,
				"building.equipment.gas": 12, "building.cooling": 20, "direct.office.lights": 2, "direct.office.equipment.electric": 4,
				"direct.office.equipment.gas": 3, "direct.lab.lights": 4, "direct.lab.equipment.electric": 7, "direct.lab.equipment.gas": 6,
			}),
		},
		Sources: epath094AuditBuildingSources(),
		Completeness: EnergyCompleteness{
			Status:        "complete",
			MappedPercent: 100,
			EnergyUse:     EnergyCompletenessLevel{Level: "energy", Status: "complete", Found: 1, Total: 1, Message: "Building facility energy is complete."},
			DeliveredLoad: EnergyCompletenessLevel{Level: "load", Status: "complete", Found: 2, Total: 2, Message: "Load quality must survive zone projection."},
			HeatDrivers:   EnergyCompletenessLevel{Level: "heat", Status: "partial", Found: 1, Total: 2, Message: "Heat quality must survive zone projection."},
			Items: []EnergyCompletenessLevel{
				{Level: "energy", Status: "complete", Found: 1, Total: 1, Message: "Legacy Building energy item."},
				{Level: "load", Status: "complete", Found: 2, Total: 2, Message: "Load quality must survive zone projection."},
				{Level: "heat", Status: "partial", Found: 1, Total: 2, Message: "Heat quality must survive zone projection."},
			},
			MissingCategories: []string{"energy: legacy Building gap", "heat: retained diagnostic gap"},
			SourceAvailability: []EnergySourceAvailabilityEntry{
				{Name: "Facility carrier meters", Level: "energy", Status: "available", SourceIDs: []string{"facility.electricity", "facility.gas"}},
				{Name: "Zone loads", Level: "load", Status: "available", SourceIDs: []string{"building.cooling"}},
				{Name: "Heat drivers", Level: "heat", Status: "partial"},
			},
		},
		canonicalMonthlyBasis: true,
		zoneDirectUseSeries: []energyExplanationSeries{
			epath094AuditDirectMonthlySeries("Office", "interior_lighting", "electricity", 3, map[int]float64{1: 1, 2: 2}, "direct.office.lights"),
			epath094AuditDirectMonthlySeries("Office", "interior_equipment", "electricity", 7, map[int]float64{1: 3, 2: 4}, "direct.office.equipment.electric"),
			epath094AuditDirectMonthlySeries("Office", "interior_equipment", "natural_gas", 5, map[int]float64{1: 2, 2: 3}, "direct.office.equipment.gas"),
			epath094AuditDirectMonthlySeries("Lab", "interior_lighting", "electricity", 7, map[int]float64{1: 3, 2: 4}, "direct.lab.lights"),
			epath094AuditDirectMonthlySeries("Lab", "interior_equipment", "electricity", 13, map[int]float64{1: 6, 2: 7}, "direct.lab.equipment.electric"),
			epath094AuditDirectMonthlySeries("Lab", "interior_equipment", "natural_gas", 9, map[int]float64{1: 3, 2: 6}, "direct.lab.equipment.gas"),
		},
	}
}

func epath094AuditBuildingPeriod(period string, values map[string]float64) ([]EnergyExplanationNode, []EnergyExplanationEdge) {
	nodes := []EnergyExplanationNode{
		{ID: "facility.electricity", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Value: values["facility.electricity"], Unit: "kWh", Period: period, Carrier: "electricity", EndUse: "total", Basis: "measured_meter", SourceIDs: []string{"facility.electricity"}},
		{ID: "facility.gas", Level: "energy", Kind: "energy.gas.total", Label: "Natural gas", Value: values["facility.gas"], Unit: "kWh", Period: period, Carrier: "natural_gas", EndUse: "total", Basis: "measured_meter", SourceIDs: []string{"facility.gas"}},
		{ID: "building.lighting", Level: "energy", Kind: "energy.interior_lighting", Label: "Lighting", Value: values["building.lighting"], Unit: "kWh", Period: period, Carrier: "electricity", EndUse: "interior_lighting", Basis: "measured_meter", SourceIDs: []string{"building.lighting"}},
		{ID: "building.equipment.electric", Level: "energy", Kind: "energy.interior_equipment", Label: "Equipment", Value: values["building.equipment.electric"], Unit: "kWh", Period: period, Carrier: "electricity", EndUse: "interior_equipment", Basis: "measured_meter", SourceIDs: []string{"building.equipment.electric"}},
		{ID: "building.equipment.gas", Level: "energy", Kind: "energy.interior_equipment", Label: "Equipment", Value: values["building.equipment.gas"], Unit: "kWh", Period: period, Carrier: "natural_gas", EndUse: "interior_equipment", Basis: "measured_meter", SourceIDs: []string{"building.equipment.gas"}},
		{ID: "building.cooling", Level: "energy", Kind: "energy.cooling", Label: "Cooling", Value: values["building.cooling"], Unit: "kWh", Period: period, Carrier: "electricity", EndUse: "cooling", Basis: "measured_meter", SourceIDs: []string{"building.cooling"}},
	}
	edges := []EnergyExplanationEdge{
		epath094AuditMeterEdge(period, nodes[0].ID, nodes[2]),
		epath094AuditMeterEdge(period, nodes[0].ID, nodes[3]),
		epath094AuditMeterEdge(period, nodes[1].ID, nodes[4]),
		epath094AuditMeterEdge(period, nodes[0].ID, nodes[5]),
	}
	return nodes, edges
}

func epath094AuditMeterEdge(period string, facilityID string, endUse EnergyExplanationNode) EnergyExplanationEdge {
	return EnergyExplanationEdge{
		ID:        "edge." + period + "." + endUse.ID,
		FromID:    facilityID,
		ToID:      endUse.ID,
		Value:     endUse.Value,
		Unit:      endUse.Unit,
		Period:    period,
		Relation:  "meter_enduse",
		Basis:     endUse.Basis,
		RuleID:    energyRelationshipRuleMeasuredEnergyVariable,
		SourceIDs: append([]string(nil), endUse.SourceIDs...),
		ZoneName:  endUse.ZoneName,
	}
}

func epath094AuditDirectNode(id string, zone string, endUse string, carrier string, value float64, sourceID string) EnergyExplanationNode {
	return epath094AuditDirectNodeForPeriodWithSource(id, zone, endUse, carrier, value, "annual", sourceID)
}

func epath094AuditDirectNodeForPeriod(id string, zone string, endUse string, carrier string, value float64, period string) EnergyExplanationNode {
	return epath094AuditDirectNodeForPeriodWithSource(id, zone, endUse, carrier, value, period, id)
}

func epath094AuditDirectNodeForPeriodWithSource(id string, zone string, endUse string, carrier string, value float64, period string, sourceID string) EnergyExplanationNode {
	kind := "energy." + endUse
	switch canonicalEnergyPathEndUse(endUse) {
	case "lighting":
		kind = "energy.interior_lighting"
	case "equipment":
		kind = "energy.interior_equipment"
	}
	return EnergyExplanationNode{
		ID: id, Level: "energy", Kind: kind, Label: id, Value: value, Unit: "kWh", Period: period,
		ZoneName: zone, Carrier: carrier, EndUse: endUse, MeterHierarchyLevel: "zone_direct_use",
		Basis: "measured_energy_variable", SourceIDs: []string{sourceID},
	}
}

func epath094AuditDirectSeries(zone string, endUse string, carrier string, value float64, sourceID string) energyExplanationSeries {
	return epath094AuditDirectMonthlySeries(zone, endUse, carrier, value, nil, sourceID)
}

func epath094AuditDirectHVACSeries(zone string, service string, carrier string, value float64, sourceID string) energyExplanationSeries {
	item := epath094AuditDirectSeries(zone, service, carrier, value, sourceID)
	item.ServiceKind = service
	item.RelatedEntityIDs = []string{"zone." + metricID(zone), "component." + metricID(zone) + "." + canonicalEnergyPathPart(service)}
	return item
}

func epath094AuditDirectMonthlySeries(zone string, endUse string, carrier string, value float64, monthly map[int]float64, sourceID string) energyExplanationSeries {
	canonicalEndUse := canonicalEnergyPathEndUse(endUse)
	kind := "energy." + endUse
	switch canonicalEndUse {
	case "lighting":
		kind = "energy.interior_lighting"
	case "equipment":
		kind = "energy.interior_equipment"
	}
	item := energyExplanationSeries{
		Stage:                 "end_use",
		CanonicalKind:         kind,
		Level:                 "energy",
		Kind:                  kind,
		Label:                 "Zone " + strings.ReplaceAll(endUse, "_", " "),
		Unit:                  "kWh",
		Carrier:               carrier,
		EndUse:                endUse,
		MeterHierarchyLevel:   "zone_direct_use",
		ZoneName:              zone,
		Basis:                 "measured_energy_variable",
		SourceKey:             zone,
		SourceName:            sourceID,
		SourceClass:           "zone_direct_variable",
		SourceIDs:             []string{sourceID},
		AnnualSourceIDs:       []string{sourceID},
		MonthlySourceIDs:      []string{sourceID},
		RelatedEntityIDs:      []string{"zone." + metricID(zone), "source." + metricID(sourceID)},
		RawTotal:              value,
		Total:                 value,
		RawMonthly:            map[int]float64{},
		Monthly:               map[int]float64{},
		EffectiveMultiplier:   1,
		MultiplierApplication: energyMultiplierAlreadyModelTotal,
	}
	for month, monthValue := range monthly {
		item.RawMonthly[month] = monthValue
		item.Monthly[month] = monthValue
	}
	return item
}

func epath094AuditBuildingSources() []EnergyDataSource {
	ids := []string{
		"facility.electricity", "facility.gas", "building.lighting", "building.equipment.electric", "building.equipment.gas", "building.cooling",
		"direct.office.lights", "direct.office.equipment.electric", "direct.office.equipment.gas",
		"direct.lab.lights", "direct.lab.equipment.electric", "direct.lab.equipment.gas",
	}
	out := make([]EnergyDataSource, 0, len(ids))
	for _, id := range ids {
		zone := ""
		if strings.Contains(id, ".office.") {
			zone = "Office"
		} else if strings.Contains(id, ".lab.") {
			zone = "Lab"
		}
		out = append(out, EnergyDataSource{ID: id, SourceType: "fixture", Name: id, ZoneName: zone})
	}
	return out
}

func epath094AuditWithoutZoneDirect(input EnergyExplanationV1) EnergyExplanationV1 {
	out := input
	out.zoneDirectUseSeries = nil
	directNodeIDs := map[string]bool{}
	filterNodes := func(nodes []EnergyExplanationNode) []EnergyExplanationNode {
		filtered := make([]EnergyExplanationNode, 0, len(nodes))
		for _, node := range nodes {
			if node.MeterHierarchyLevel == "zone_direct_use" {
				directNodeIDs[node.ID] = true
				continue
			}
			filtered = append(filtered, node)
		}
		return filtered
	}
	filterEdges := func(edges []EnergyExplanationEdge) []EnergyExplanationEdge {
		filtered := make([]EnergyExplanationEdge, 0, len(edges))
		for _, edge := range edges {
			if directNodeIDs[edge.FromID] || directNodeIDs[edge.ToID] {
				continue
			}
			filtered = append(filtered, edge)
		}
		return filtered
	}
	out.Nodes = filterNodes(out.Nodes)
	for index := range out.Periods {
		out.Periods[index].Nodes = filterNodes(out.Periods[index].Nodes)
	}
	out.Edges = filterEdges(out.Edges)
	for index := range out.Periods {
		out.Periods[index].Edges = filterEdges(out.Periods[index].Edges)
	}
	filteredSources := make([]EnergyDataSource, 0, len(out.Sources))
	for _, source := range out.Sources {
		if !strings.HasPrefix(source.ID, "direct.") {
			filteredSources = append(filteredSources, source)
		}
	}
	out.Sources = filteredSources
	return out
}

type epath094AuditNodeSnapshot struct {
	ID               string
	Value            float64
	Basis            string
	SourceIDs        []string
	RelatedEntityIDs []string
	RelatedPathIDs   []string
}

type epath094AuditLinkSnapshot struct {
	FromID         string
	ToID           string
	Relation       string
	Basis          string
	FromValue      float64
	ToValue        float64
	SourceIDs      []string
	RelatedPathIDs []string
}

type epath094AuditBuildingGraphSnapshot struct {
	Nodes          []epath094AuditNodeSnapshot
	Links          []epath094AuditLinkSnapshot
	Reconciliation []EnergyReconciliation
	Periods        map[string]struct {
		Nodes []epath094AuditNodeSnapshot
		Links []epath094AuditLinkSnapshot
	}
}

func epath094AuditBuildingSnapshot(result EnergyExplanationResult) epath094AuditBuildingGraphSnapshot {
	periods := map[string]struct {
		Nodes []epath094AuditNodeSnapshot
		Links []epath094AuditLinkSnapshot
	}{}
	for _, period := range result.Periods {
		periods[period.ID] = struct {
			Nodes []epath094AuditNodeSnapshot
			Links []epath094AuditLinkSnapshot
		}{epath094AuditNodeSnapshots(period.Nodes), epath094AuditLinkSnapshots(period.Links)}
	}
	return epath094AuditBuildingGraphSnapshot{
		Nodes:          epath094AuditNodeSnapshots(result.Nodes),
		Links:          epath094AuditLinkSnapshots(result.Links),
		Reconciliation: append([]EnergyReconciliation(nil), result.Reconciliation...),
		Periods:        periods,
	}
}

func epath094AuditZoneSnapshot(result EnergyExplanationResult) map[string]epath094AuditBuildingGraphSnapshot {
	out := make(map[string]epath094AuditBuildingGraphSnapshot, len(result.ZoneResults))
	for _, zone := range result.ZoneResults {
		periods := map[string]struct {
			Nodes []epath094AuditNodeSnapshot
			Links []epath094AuditLinkSnapshot
		}{}
		for _, period := range zone.Periods {
			periods[period.ID] = struct {
				Nodes []epath094AuditNodeSnapshot
				Links []epath094AuditLinkSnapshot
			}{epath094AuditNodeSnapshots(period.Nodes), epath094AuditLinkSnapshots(period.Links)}
		}
		reconciliation := append([]EnergyReconciliation(nil), zone.Reconciliation...)
		for index := range reconciliation {
			sort.Strings(reconciliation[index].SourceIDs)
		}
		sort.Slice(reconciliation, func(i, j int) bool { return reconciliation[i].ID < reconciliation[j].ID })
		out[zone.Scope.ZoneName] = epath094AuditBuildingGraphSnapshot{
			Nodes:          epath094AuditNodeSnapshots(zone.Nodes),
			Links:          epath094AuditLinkSnapshots(zone.Links),
			Reconciliation: reconciliation,
			Periods:        periods,
		}
	}
	return out
}

func epath094AuditNodeSnapshots(nodes []EnergyExplanationNode) []epath094AuditNodeSnapshot {
	out := make([]epath094AuditNodeSnapshot, 0, len(nodes))
	for _, node := range nodes {
		sources := append([]string(nil), node.SourceIDs...)
		sort.Strings(sources)
		out = append(out, epath094AuditNodeSnapshot{
			ID:               node.ID,
			Value:            node.Value,
			Basis:            node.Basis,
			SourceIDs:        sources,
			RelatedEntityIDs: append([]string(nil), node.RelatedEntityIDs...),
			RelatedPathIDs:   append([]string(nil), node.RelatedPathIDs...),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func epath094AuditLinkSnapshots(links []EnergyPathLink) []epath094AuditLinkSnapshot {
	out := make([]epath094AuditLinkSnapshot, 0, len(links))
	for _, link := range links {
		sources := append([]string(nil), link.SourceIDs...)
		sort.Strings(sources)
		out = append(out, epath094AuditLinkSnapshot{
			FromID:         link.FromID,
			ToID:           link.ToID,
			Relation:       link.Relation,
			Basis:          link.Basis,
			FromValue:      link.FromValue,
			ToValue:        link.ToValue,
			SourceIDs:      sources,
			RelatedPathIDs: append([]string(nil), link.RelatedPathIDs...),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].FromID+"|"+out[i].ToID+"|"+out[i].Relation < out[j].FromID+"|"+out[j].ToID+"|"+out[j].Relation
	})
	return out
}

func epath094AuditNode(nodes []EnergyExplanationNode, id string) *EnergyExplanationNode {
	for index := range nodes {
		if nodes[index].ID == id {
			return &nodes[index]
		}
	}
	return nil
}

func epath094AuditLink(links []EnergyPathLink, fromID string, toID string) *EnergyPathLink {
	for index := range links {
		if links[index].FromID == fromID && links[index].ToID == toID {
			return &links[index]
		}
	}
	return nil
}

func epath094AuditSummaryItem(items []EnergyExplanationSummaryItem, id string) *EnergyExplanationSummaryItem {
	for index := range items {
		if items[index].ID == id {
			return &items[index]
		}
	}
	return nil
}

func epath094AuditHasWarning(warnings []EnergyWarning, code string) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}

func epath094AuditCarrierReconciliation(items []EnergyReconciliation, carrier string) *EnergyReconciliation {
	needle := canonicalEnergyPathPart(carrier)
	for index := range items {
		item := &items[index]
		if item.Level == "energy" && strings.Contains(canonicalEnergyPathPart(item.ID), needle) {
			return item
		}
	}
	return nil
}

func epath094AuditAssertZoneValue(t *testing.T, nodes []EnergyExplanationNode, id string, want float64) {
	t.Helper()
	node := epath094AuditNode(nodes, id)
	if node == nil || node.Value != want || node.Basis != "direct_zone_energy" {
		t.Errorf("zone direct value %q = %#v, want %.3f direct_zone_energy", id, node, want)
	}
}

func epath094AuditAssertDirectZoneCompleteness(t *testing.T, label string, got EnergyCompleteness) {
	t.Helper()
	if got.Status != "partial" || got.MappedPercent != 0 || got.EnergyUse.Status != "partial" ||
		got.EnergyUse.Found != 0 || got.EnergyUse.Total != 0 || strings.TrimSpace(got.EnergyUse.Message) == "" {
		t.Errorf("%s inherited Building energy coverage: %#v", label, got)
	}
	if got.DeliveredLoad.Status != "complete" || got.DeliveredLoad.Found != 2 || got.DeliveredLoad.Total != 2 ||
		got.HeatDrivers.Status != "partial" || got.HeatDrivers.Found != 1 || got.HeatDrivers.Total != 2 {
		t.Errorf("%s lost non-energy completeness quality: load=%#v heat=%#v", label, got.DeliveredLoad, got.HeatDrivers)
	}
	energyItems := 0
	for _, item := range got.Items {
		if !strings.EqualFold(item.Level, "energy") {
			continue
		}
		energyItems++
		if item.Status != "partial" || item.Found != 0 || item.Total != 0 || item.Message != got.EnergyUse.Message {
			t.Errorf("%s retained stale energy completeness item: %#v", label, item)
		}
	}
	if energyItems != 1 {
		t.Errorf("%s energy completeness item count = %d, want one scoped partial item: %#v", label, energyItems, got.Items)
	}
	energyAvailability := 0
	for _, item := range got.SourceAvailability {
		if !strings.EqualFold(item.Level, "energy") {
			continue
		}
		energyAvailability++
		if item.Name != "Complete zone carrier total" || item.Status != "missing" || len(item.SourceIDs) != 0 {
			t.Errorf("%s retained Building energy source availability: %#v", label, item)
		}
	}
	if energyAvailability != 1 {
		t.Errorf("%s energy source-availability count = %d, want one scoped missing entry: %#v", label, energyAvailability, got.SourceAvailability)
	}
	energyMissing := 0
	for _, category := range got.MissingCategories {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(category)), "energy:") {
			continue
		}
		energyMissing++
		if !strings.EqualFold(category, "energy: complete zone carrier total") {
			t.Errorf("%s retained stale Building energy missing-category: %q", label, category)
		}
	}
	if energyMissing != 1 {
		t.Errorf("%s energy missing-category count = %d, want one scoped gap: %#v", label, energyMissing, got.MissingCategories)
	}
}

func epath094AuditResultClaimsSource(nodes []EnergyExplanationNode, links []EnergyPathLink, reconciliation []EnergyReconciliation, sourceID string) bool {
	for _, node := range nodes {
		if stringSliceContains(node.SourceIDs, sourceID) {
			return true
		}
	}
	for _, link := range links {
		if stringSliceContains(link.SourceIDs, sourceID) {
			return true
		}
	}
	for _, item := range reconciliation {
		if stringSliceContains(item.SourceIDs, sourceID) {
			return true
		}
	}
	return false
}

func epath094AuditAssertSortedProvenance(t *testing.T, nodes []EnergyExplanationNode, links []EnergyPathLink, reconciliation []EnergyReconciliation) {
	t.Helper()
	for _, node := range nodes {
		if !sort.StringsAreSorted(node.SourceIDs) {
			t.Errorf("node %q source order is not deterministic: %#v", node.ID, node.SourceIDs)
		}
		if !sort.StringsAreSorted(node.RelatedEntityIDs) {
			t.Errorf("node %q related-entity order is not deterministic: %#v", node.ID, node.RelatedEntityIDs)
		}
		if !sort.StringsAreSorted(node.RelatedPathIDs) {
			t.Errorf("node %q related-path order is not deterministic: %#v", node.ID, node.RelatedPathIDs)
		}
	}
	for _, link := range links {
		if !sort.StringsAreSorted(link.SourceIDs) {
			t.Errorf("link %q source order is not deterministic: %#v", link.ID, link.SourceIDs)
		}
		if !sort.StringsAreSorted(link.RelatedPathIDs) {
			t.Errorf("link %q related-path order is not deterministic: %#v", link.ID, link.RelatedPathIDs)
		}
	}
	for _, item := range reconciliation {
		if !sort.StringsAreSorted(item.SourceIDs) {
			t.Errorf("reconciliation %q source order is not deterministic: %#v", item.ID, item.SourceIDs)
		}
	}
}

func epath094AuditReverse[T any](items []T) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}
