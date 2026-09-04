package simulation

import (
	"strings"
	"testing"
)

func TestEPATH070ReviewSelectedLoadOwnsGraphAndAnnualProvenance(t *testing.T) {
	series := []energyExplanationSeries{
		review070Load("zone-sensible-energy", "Zone Air System Sensible Cooling Energy", "load.zone_cooling", "cooling", "zone", "Office", "sensible", 10, map[int]float64{1: 0, 2: 10}, false, "Monthly"),
		review070Load("zone-sensible-rate", "Zone Air System Sensible Cooling Rate", "load.zone_cooling", "cooling", "zone", "Office", "sensible", 18, map[int]float64{1: 9, 2: 9}, true, "Monthly"),
		review070Load("zone-latent-rate", "Zone Air System Latent Cooling Rate", "load.zone_latent_cooling", "cooling", "zone", "Office", "latent", 5, map[int]float64{1: 2, 2: 3}, true, "Monthly"),
		review070Load("ideal-total", "Zone Ideal Loads Zone Total Cooling Energy", "load.zone_cooling", "cooling", "zone", "Office", "combined", 50, map[int]float64{3: 50}, false, "Monthly"),
		review070Load("radiant", "Zone Radiant HVAC Cooling Energy", "load.zone_radiant_cooling", "cooling", "zone", "Office", "combined", 30, map[int]float64{1: 30}, false, "Monthly"),
		review070Load("coil", "Cooling Coil Total Cooling Energy", "load.system_cooling", "cooling", "system", "", "combined", 100, map[int]float64{1: 100}, false, "Monthly"),
		review070Load("plant", "Plant Loop Cooling Demand Energy", "load.plant_cooling", "cooling", "plant", "", "combined", 200, map[int]float64{1: 200}, false, "Monthly"),
	}
	result := review070BuildResult(series)
	wanted := []string{"zone-sensible-energy", "zone-latent-rate"}
	excluded := []string{"zone-sensible-rate", "ideal-total", "radiant", "coil", "plant"}

	annual := energyPathV2NodeByID(result.Nodes, "load.cooling.building")
	if annual == nil || annual.Value != 15 || annual.ThermalComponent != "combined" {
		t.Fatalf("annual canonical cooling load = %#v, want 15 kWh combined", annual)
	}
	review070RequireIDs(t, "annual load", annual.SourceIDs, wanted...)
	review070RejectIDs(t, "annual load", annual.SourceIDs, excluded...)

	link := energyPathV2LinkByIDs(result.Links, "load.cooling.building", "end_use.cooling.building")
	if link == nil {
		t.Fatalf("missing canonical load->end-use link: %#v", result.Links)
	}
	review070RequireIDs(t, "load link", link.SourceIDs, wanted...)
	review070RejectIDs(t, "load link", link.SourceIDs, excluded...)

	for _, expectation := range []struct {
		period string
		value  float64
	}{{period: "M1", value: 2}, {period: "M2", value: 13}} {
		period := review070Period(result.Periods, expectation.period)
		if period == nil {
			t.Fatalf("missing %s period", expectation.period)
		}
		node := energyPathV2NodeByID(period.Nodes, "load.cooling.building")
		if node == nil || node.Value != expectation.value {
			t.Fatalf("%s canonical cooling load = %#v, want %g", expectation.period, node, expectation.value)
		}
		review070RequireIDs(t, expectation.period+" load", node.SourceIDs, wanted...)
		review070RejectIDs(t, expectation.period+" load", node.SourceIDs, excluded...)
	}
	if period := review070Period(result.Periods, "M3"); period != nil && energyPathV2NodeByID(period.Nodes, "load.cooling.building") != nil {
		t.Fatalf("lower-priority Ideal Loads source filled an uncovered selected-family month: %#v", period.Nodes)
	}
	for _, item := range result.Reconciliation {
		if item.Level != "heat" || item.ServiceKind != "cooling" {
			continue
		}
		if item.ExpectedValue != 15 {
			t.Fatalf("cooling reconciliation basis = %g, want canonical load 15", item.ExpectedValue)
		}
		review070RejectIDs(t, "cooling reconciliation", item.SourceIDs, excluded...)
	}
}

func TestEPATH070ReviewIdealTotalKeepsComponentsButExcludesOtherLevels(t *testing.T) {
	series := []energyExplanationSeries{
		review070Load("ideal-total", "Zone Ideal Loads Zone Total Cooling Energy", "load.zone_cooling", "cooling", "zone", "Office", "combined", 100, map[int]float64{1: 100}, false, "Monthly"),
		review070Load("ideal-sensible", "Zone Ideal Loads Zone Sensible Cooling Energy", "load.zone_cooling", "cooling", "zone", "Office", "sensible", 70, map[int]float64{1: 70}, false, "Monthly"),
		review070Load("ideal-latent", "Zone Ideal Loads Zone Latent Cooling Energy", "load.zone_latent_cooling", "cooling", "zone", "Office", "latent", 30, map[int]float64{1: 30}, false, "Monthly"),
		review070Load("ideal-supply-total", "Zone Ideal Loads Supply Air Total Cooling Energy", "load.zone_cooling", "cooling", "zone", "Office", "combined", 120, map[int]float64{1: 120}, false, "Monthly"),
		review070Load("radiant", "Zone Radiant HVAC Cooling Energy", "load.zone_radiant_cooling", "cooling", "zone", "Office", "combined", 40, map[int]float64{1: 40}, false, "Monthly"),
	}
	result := review070BuildResult(series)
	node := energyPathV2NodeByID(result.Nodes, "load.cooling.building")
	if node == nil || node.Value != 100 || node.ThermalComponent != "combined" {
		t.Fatalf("Ideal Loads canonical total = %#v, want reported Zone Total 100", node)
	}
	review070RequireIDs(t, "Ideal total node", node.SourceIDs, "ideal-total", "ideal-sensible", "ideal-latent")
	review070RejectIDs(t, "Ideal total node", node.SourceIDs, "ideal-supply-total", "radiant")
	link := energyPathV2LinkByIDs(result.Links, "load.cooling.building", "end_use.cooling.building")
	if link == nil {
		t.Fatalf("missing Ideal total load link: %#v", result.Links)
	}
	review070RejectIDs(t, "Ideal total link", link.SourceIDs, "ideal-supply-total", "radiant")

	tests := []struct {
		id        string
		role      string
		section   string
		component string
	}{
		{id: "ideal-total", role: energyDriverSourceRoleMainFlow, section: energyDriverInspectorSectionBreakdown, component: "combined"},
		{id: "ideal-sensible", role: energyDriverSourceRoleContext, section: energyDriverInspectorSectionBreakdown, component: "sensible"},
		{id: "ideal-latent", role: energyDriverSourceRoleContext, section: energyDriverInspectorSectionBreakdown, component: "latent"},
		{id: "ideal-supply-total", role: energyDriverSourceRoleContext, section: energyDriverInspectorSectionContext, component: "combined"},
		{id: "radiant", role: energyDriverSourceRoleContext, section: energyDriverInspectorSectionContext, component: "combined"},
	}
	for _, test := range tests {
		source := energyExplanationSourceByID(result.Sources, test.id)
		if source == nil {
			t.Errorf("missing retained Ideal/load context source %q", test.id)
			continue
		}
		if source.DriverRole != test.role || source.DriverCategory != "load.cooling" || source.InspectorSection != test.section || !strings.Contains(source.DriverComponent, test.component) {
			t.Errorf("source %q metadata = %#v", test.id, source)
		}
		if source.SourceType != "derived_formula" && (source.Formula != "" || len(source.InputSourceIDs) != 0) {
			t.Errorf("reported source %q is falsely marked as a derivation: formula=%q inputs=%#v", test.id, source.Formula, source.InputSourceIDs)
		}
	}
}

func TestEPATH070ReviewPredictedIsDedupedContextWithSensibleDelta(t *testing.T) {
	series := []energyExplanationSeries{
		review070Load("actual-sensible", "Zone Air System Sensible Cooling Energy", "load.zone_cooling", "cooling", "zone", "Office", "sensible", 10, map[int]float64{1: 10}, false, "Monthly"),
		review070Load("actual-latent", "Zone Air System Latent Cooling Energy", "load.zone_latent_cooling", "cooling", "zone", "Office", "latent", 2, map[int]float64{1: 2}, false, "Monthly"),
		review070Load("predicted-zone", "Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate", "load.zone_predicted_cooling", "cooling", "zone", "Office", "sensible", -15, map[int]float64{1: -15}, true, "Monthly"),
		review070Load("predicted-system", "Zone System Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate", "load.zone_predicted_cooling", "cooling", "zone", "Office", "sensible", -15, map[int]float64{1: -15}, true, "Monthly"),
	}
	result := review070BuildResult(series)
	node := energyPathV2NodeByID(result.Nodes, "load.cooling.building")
	if node == nil || node.Value != 12 {
		t.Fatalf("predicted context changed actual delivered load: %#v", node)
	}
	review070RejectIDs(t, "actual load", node.SourceIDs, "predicted-zone", "predicted-system")
	for _, id := range []string{"predicted-zone", "predicted-system"} {
		source := energyExplanationSourceByID(result.Sources, id)
		if source == nil || source.DriverRole != energyDriverSourceRoleContext || source.InspectorSection != energyDriverInspectorSectionContext || source.DriverCategory != "load.cooling" {
			t.Errorf("predicted raw source %q is not retained as context: %#v", id, source)
		}
		if source != nil && source.SourceType != "derived_formula" && source.Formula != "" {
			t.Errorf("predicted reported source %q is falsely labeled derived: %q", id, source.Formula)
		}
	}

	deltas := []*EnergyDataSource{}
	for index := range result.Sources {
		source := &result.Sources[index]
		component := strings.ToLower(source.DriverComponent)
		if source.SourceType == "derived_formula" && source.DriverCategory == "load.cooling" && (strings.Contains(component, "predicted") || strings.Contains(component, "unmet")) {
			deltas = append(deltas, source)
		}
	}
	if len(deltas) != 1 {
		t.Fatalf("predicted-vs-delivered derived detail count = %d, want one deduped comparison: %#v", len(deltas), deltas)
	}
	delta := deltas[0]
	if delta.DriverRole != energyDriverSourceRoleContext || delta.InspectorSection != energyDriverInspectorSectionBalance || delta.Formula == "" || delta.EffectiveValue != 5 {
		t.Fatalf("predicted-vs-delivered delta metadata/value = %#v, want +5 sensible unmet/control context", delta)
	}
	if !review070Contains(delta.InputSourceIDs, "actual-sensible") || review070Contains(delta.InputSourceIDs, "actual-latent") {
		t.Fatalf("predicted comparison is not sensible-to-sensible: %#v", delta.InputSourceIDs)
	}
	predictedInputs := 0
	for _, id := range []string{"predicted-zone", "predicted-system"} {
		if review070Contains(delta.InputSourceIDs, id) {
			predictedInputs++
		}
	}
	if predictedInputs != 1 {
		t.Fatalf("predicted zone/system families were not deduped: %#v", delta.InputSourceIDs)
	}

	predictedOnly := review070BuildResult(series[2:])
	if load := energyPathV2NodeByID(predictedOnly.Nodes, "load.cooling.building"); load != nil {
		t.Fatalf("predicted-only inputs fabricated delivered load: %#v", load)
	}
	for _, source := range predictedOnly.Sources {
		if source.SourceType == "derived_formula" && source.DriverCategory == "load.cooling" && (strings.Contains(strings.ToLower(source.DriverComponent), "predicted") || strings.Contains(strings.ToLower(source.DriverComponent), "unmet")) {
			t.Fatalf("predicted-only input fabricated delivered comparison: %#v", source)
		}
	}
}

func TestEPATH070ReviewSystemFallbackDoesNotAddTotalAndSensible(t *testing.T) {
	series := []energyExplanationSeries{
		review070LoadWithKey("coil-total-energy", "Zone Coil A", "Cooling Coil Total Cooling Energy", "load.system_cooling", "cooling", "system", "", "combined", 100, map[int]float64{1: 100}, false, "Monthly"),
		review070LoadWithKey("coil-sensible-energy", "Zone Coil A", "Cooling Coil Sensible Cooling Energy", "load.system_cooling", "cooling", "system", "", "sensible", 80, map[int]float64{1: 80}, false, "Monthly"),
		review070LoadWithKey("coil-total-rate", "Zone Coil A", "Cooling Coil Total Cooling Rate", "load.system_cooling", "cooling", "system", "", "combined", 90, map[int]float64{1: 90}, true, "Monthly"),
		review070LoadWithKey("plant", "Cooling Loop", "Plant Loop Cooling Demand Energy", "load.plant_cooling", "cooling", "plant", "", "combined", 200, map[int]float64{1: 200}, false, "Monthly"),
	}
	result := review070BuildResult(series)
	node := energyPathV2NodeByID(result.Nodes, "load.cooling.building")
	if node == nil || node.Value != 100 || node.PathType != "system" {
		t.Fatalf("system fallback = %#v, want one total-Energy contribution of 100", node)
	}
	review070RequireIDs(t, "system fallback", node.SourceIDs, "coil-total-energy")
	review070RejectIDs(t, "system fallback", node.SourceIDs, "coil-sensible-energy", "coil-total-rate", "plant")
}

func TestEPATH070ReviewAllZeroHigherTierAllowsDirectZoneFallback(t *testing.T) {
	series := []energyExplanationSeries{
		review070Load("zero-zone-air", "Zone Air System Sensible Heating Energy", "load.zone_heating", "heating", "zone", "Office", "sensible", 0, map[int]float64{1: 0}, false, "Monthly"),
		review070Load("baseboard", "Zone Baseboard Total Heating Energy", "load.zone_equipment_heating", "heating", "zone", "Office", "combined", 30, map[int]float64{1: 30}, false, "Monthly"),
	}
	result := review070BuildResult(series)
	node := energyPathV2NodeByID(result.Nodes, "load.heating.building")
	if node == nil || node.Value != 30 || !review070Contains(node.SourceIDs, "baseboard") {
		t.Fatalf("all-zero Zone Air source blocked direct-zone fallback: %#v", node)
	}
}

func review070BuildResult(loads []energyExplanationSeries) EnergyExplanationResult {
	series := append([]energyExplanationSeries(nil), loads...)
	services := map[string]bool{}
	for _, item := range loads {
		service := energyCanonicalServiceKind(item.ServiceKind)
		if service == "cooling" || service == "heating" {
			services[service] = true
		}
	}
	series = append(series, canonicalEnergyExplanationSeries(energyExplanationSeries{
		Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Unit: "kWh", Carrier: "electricity", MeterHierarchyLevel: "facility_total",
		SourceIDs: []string{"review-carrier"}, SourceName: "Electricity:Facility", sourceName: "Electricity:Facility", sourceFrequency: "Monthly", Total: 2000, Monthly: map[int]float64{1: 1000, 2: 1000},
	}))
	for service := range services {
		series = append(series, canonicalEnergyExplanationSeries(energyExplanationSeries{
			Level: "energy", Kind: "energy." + service, Label: service, Unit: "kWh", Carrier: "electricity", EndUse: service, MeterHierarchyLevel: "broad_end_use",
			SourceIDs: []string{"review-end-use-" + service}, SourceName: service + ":Electricity", sourceName: service + ":Electricity", sourceFrequency: "Monthly", Total: 1000, Monthly: map[int]float64{1: 500, 2: 500},
		}))
	}

	sources := make([]EnergyDataSource, 0, len(series))
	seen := map[string]bool{}
	for _, item := range series {
		for _, id := range item.SourceIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			sources = append(sources, EnergyDataSource{
				ID: id, SourceType: "acceptance_fixture", KeyValue: item.sourceKeyValue, Name: firstNonEmpty(item.SourceName, item.sourceName),
				Units: item.Unit, SourceUnit: item.Unit, NormalizedUnit: item.Unit, ReportingFrequency: item.sourceFrequency,
				ZoneName: item.ZoneName, RawValue: item.Total, EffectiveValue: item.Total, EffectiveMultiplier: 1,
			})
		}
	}
	context := energyDriverBuildContext{Enabled: true, Multipliers: unitEnergyMultiplierIndex("Office")}
	legacy := buildEnergyExplanationResultWithDriverContext(series, sources, &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}, context)
	return UpgradeEnergyExplanationV1(legacy)
}

func review070Load(id string, name string, kind string, service string, pathType string, zone string, component string, total float64, monthly map[int]float64, isRate bool, frequency string) energyExplanationSeries {
	return review070LoadWithKey(id, zone, name, kind, service, pathType, zone, component, total, monthly, isRate, frequency)
}

func review070LoadWithKey(id string, key string, name string, kind string, service string, pathType string, zone string, component string, total float64, monthly map[int]float64, isRate bool, frequency string) energyExplanationSeries {
	return canonicalEnergyExplanationSeries(energyExplanationSeries{
		Stage: "load", CanonicalKind: kind, Level: "load", Kind: kind, Label: service + " load", Unit: "kWh",
		ServiceKind: service, PathType: pathType, ZoneName: zone, ThermalComponent: component,
		SourceIDs: []string{id}, SourceName: name, SourceKey: key, Total: total, Monthly: monthly,
		sourceName: name, sourceKeyValue: key, sourceFrequency: frequency, sourceIsRate: isRate,
	})
}

func review070Period(periods []EnergyPeriod, id string) *EnergyPeriod {
	for index := range periods {
		if periods[index].ID == id {
			return &periods[index]
		}
	}
	return nil
}

func review070Contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func review070RequireIDs(t *testing.T, label string, values []string, wanted ...string) {
	t.Helper()
	for _, id := range wanted {
		if !review070Contains(values, id) {
			t.Errorf("%s source IDs = %#v, missing %q", label, values, id)
		}
	}
}

func review070RejectIDs(t *testing.T, label string, values []string, excluded ...string) {
	t.Helper()
	for _, id := range excluded {
		if review070Contains(values, id) {
			t.Errorf("%s source IDs = %#v, unexpectedly contains %q", label, values, id)
		}
	}
}
