package simulation

import (
	"math"
	"strings"
	"testing"
)

// East is cooling-heavy (90/10) while West is heating-heavy (20/80).
// The approved simultaneous ratio is therefore (10+20)/(90+80) = 30/170,
// not min(Building cooling 110, heating 90)/max(...) = 90/110.
func TestEPATH081AcceptanceSimultaneousLoadsAggregateCompletedZoneMonths(t *testing.T) {
	result := epath081BuildFixture(t)

	for _, want := range []struct {
		zone        string
		cooling     float64
		heating     float64
		numerator   float64
		denominator float64
	}{
		{zone: "East", cooling: 90, heating: 10, numerator: 10, denominator: 90},
		{zone: "West", cooling: 20, heating: 80, numerator: 20, denominator: 80},
	} {
		nodes, links := epath081Graph(t, result, want.zone, "M1")
		epath081AssertLoadAndClosure(t, nodes, links, want.zone, "cooling", want.cooling)
		epath081AssertLoadAndClosure(t, nodes, links, want.zone, "heating", want.heating)
		for _, service := range []string{"cooling", "heating"} {
			load := epath081NodeByID(nodes, epath081LoadID(service, want.zone))
			epath081AssertSimultaneousMetric(t, load, want.numerator, want.denominator)
		}
	}

	for _, period := range []string{"M1", "annual"} {
		nodes, links := epath081Graph(t, result, "", period)
		epath081AssertLoadAndClosure(t, nodes, links, "", "cooling", 110)
		epath081AssertLoadAndClosure(t, nodes, links, "", "heating", 90)
		for _, service := range []string{"cooling", "heating"} {
			load := epath081NodeByID(nodes, epath081LoadID(service, ""))
			epath081AssertSimultaneousMetric(t, load, 30, 170)
		}
	}

	// The same physical category is a gain in East and a loss in West. Both
	// service-specific contributions must survive Building aggregation instead
	// of cancelling as one signed Building total.
	for _, want := range []struct {
		service string
		value   float64
	}{
		{service: "cooling", value: 90},
		{service: "heating", value: 80},
	} {
		node := epath081NodeByID(result.Nodes, epath081DriverID(energyDriverCategoryInfiltration, want.service, ""))
		if node == nil || node.Value != want.value || node.AllocatedValue != want.value {
			t.Errorf("Building infiltration %s contribution = %#v, want %g without cross-zone cancellation", want.service, node, want.value)
		}
	}
}

func TestEPATH081AcceptanceOffsetEffectsNeverBecomeReverseMainRibbons(t *testing.T) {
	result := epath081BuildFixture(t)
	graphs := []struct {
		zone   string
		period string
	}{
		{period: "annual"},
		{period: "M1"},
		{zone: "East", period: "annual"},
		{zone: "East", period: "M1"},
		{zone: "West", period: "annual"},
		{zone: "West", period: "M1"},
	}
	for _, graph := range graphs {
		nodes, links := epath081Graph(t, result, graph.zone, graph.period)
		nodeByID := map[string]EnergyExplanationNode{}
		for _, node := range nodes {
			nodeByID[node.ID] = node
			identity := strings.ToLower(strings.Join([]string{node.ID, node.Kind, node.Level}, " "))
			if strings.Contains(identity, "offset") {
				t.Errorf("%s/%s exposed Offset effects as a primary graph node: %#v", graph.zone, graph.period, node)
			}
		}
		for _, link := range links {
			identity := strings.ToLower(strings.Join([]string{link.ID, link.Relation, link.Basis}, " "))
			if strings.Contains(identity, "offset") {
				t.Errorf("%s/%s exposed Offset effects as a main link: %#v", graph.zone, graph.period, link)
			}
			if link.Relation != "driver_to_load" {
				continue
			}
			from, fromOK := nodeByID[link.FromID]
			to, toOK := nodeByID[link.ToID]
			if !fromOK || !toOK || from.Level != "driver" || to.Level != "load" {
				t.Errorf("%s/%s main thermal ribbon is not forward driver -> load: %#v", graph.zone, graph.period, link)
				continue
			}
			if energyCanonicalServiceKind(from.ServiceKind) != energyCanonicalServiceKind(to.ServiceKind) || energyCanonicalServiceKind(link.ServiceKind) != energyCanonicalServiceKind(to.ServiceKind) {
				t.Errorf("%s/%s opposing pressure crossed into a reverse-service main ribbon: %#v", graph.zone, graph.period, link)
			}
		}
	}

	for _, want := range []struct {
		zone       string
		period     string
		service    string
		effective  float64
		effectKind string
		sourceIDs  []string
	}{
		{zone: "East", period: "M1", service: "cooling", effective: 10, effectKind: "reduces_cooling", sourceIDs: []string{"east-ventilation-loss"}},
		{zone: "East", period: "M1", service: "heating", effective: 90, effectKind: "reduces_heating", sourceIDs: []string{"east-infiltration-gain"}},
		{zone: "West", period: "M1", service: "cooling", effective: 80, effectKind: "reduces_cooling", sourceIDs: []string{"west-infiltration-loss"}},
		{zone: "West", period: "M1", service: "heating", effective: 20, effectKind: "reduces_heating", sourceIDs: []string{"west-people-gain"}},
		{period: "M1", service: "cooling", effective: 90, effectKind: "reduces_cooling", sourceIDs: []string{"east-ventilation-loss", "west-infiltration-loss"}},
		{period: "M1", service: "heating", effective: 110, effectKind: "reduces_heating", sourceIDs: []string{"east-infiltration-gain", "west-people-gain"}},
		{period: "annual", service: "cooling", effective: 90, effectKind: "reduces_cooling", sourceIDs: []string{"east-ventilation-loss", "west-infiltration-loss"}},
		{period: "annual", service: "heating", effective: 110, effectKind: "reduces_heating", sourceIDs: []string{"east-infiltration-gain", "west-people-gain"}},
	} {
		nodes, _ := epath081Graph(t, result, want.zone, want.period)
		load := epath081NodeByID(nodes, epath081LoadID(want.service, want.zone))
		if load == nil {
			t.Errorf("%s/%s %s load is missing", want.zone, want.period, want.service)
			continue
		}
		epath081AssertOffsetSummary(t, load.OffsetEffects, want.service, want.effectKind, want.effective, want.sourceIDs)
	}

	// Explicit pairs that would visualize the opposing pressures are forbidden.
	for _, forbidden := range []struct {
		zone   string
		fromID string
		toID   string
	}{
		{zone: "East", fromID: epath081DriverID(energyDriverCategoryInfiltration, "cooling", "East"), toID: epath081LoadID("heating", "East")},
		{zone: "East", fromID: epath081DriverID(energyDriverCategoryMechanicalVentilation, "heating", "East"), toID: epath081LoadID("cooling", "East")},
		{zone: "West", fromID: epath081DriverID(energyDriverCategoryInfiltration, "heating", "West"), toID: epath081LoadID("cooling", "West")},
		{zone: "West", fromID: epath081DriverID(energyDriverCategoryPeople, "cooling", "West"), toID: epath081LoadID("heating", "West")},
	} {
		_, links := epath081Graph(t, result, forbidden.zone, "M1")
		if link := epath081LinkByIDs(links, forbidden.fromID, forbidden.toID); link != nil {
			t.Errorf("Offset effect escaped inspector as reverse main ribbon: %#v", link)
		}
	}
}

func epath081BuildFixture(t *testing.T) EnergyExplanationResult {
	t.Helper()
	series := []energyExplanationSeries{
		epath081Load("east-cooling-load", "East", "cooling", 90),
		epath081Load("east-heating-load", "East", "heating", 10),
		epath081Load("west-cooling-load", "West", "cooling", 20),
		epath081Load("west-heating-load", "West", "heating", 80),
		epath081Heat(t, "east-infiltration-gain", "East", "Zone Infiltration Sensible Heat Gain Energy", 90),
		epath081Heat(t, "east-ventilation-loss", "East", "Zone Ventilation Sensible Heat Loss Energy", 10),
		epath081Heat(t, "west-people-gain", "West", "Zone People Convective Heating Energy", 20),
		epath081Heat(t, "west-infiltration-loss", "West", "Zone Infiltration Sensible Heat Loss Energy", 80),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(
		series,
		epath081Sources(series),
		&PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath},
		energyDriverBuildContext{Enabled: true, Multipliers: epath081UnitMultipliers("East", "West")},
	)
	return UpgradeEnergyExplanationV1(legacy)
}

func epath081Load(sourceID string, zone string, service string, value float64) energyExplanationSeries {
	label := "Cooling"
	if service == "heating" {
		label = "Heating"
	}
	return canonicalEnergyExplanationSeries(energyExplanationSeries{
		Stage: "load", CanonicalKind: "load.zone_" + service, Level: "load", Kind: "load.zone_" + service,
		Label: label + " load", Unit: "kWh", ServiceKind: service, PathType: "zone", ZoneName: zone,
		ThermalComponent: "sensible", Basis: "reported_variable", SourceIDs: []string{sourceID},
		Total: value, Monthly: map[int]float64{1: value}, SourceName: "Zone Air System Sensible " + label + " Energy",
		sourceName: "Zone Air System Sensible " + label + " Energy", sourceKeyValue: zone, sourceFrequency: "Monthly",
	})
}

func epath081Heat(t *testing.T, sourceID string, zone string, name string, value float64) energyExplanationSeries {
	t.Helper()
	definition, ok := energyHeatAliasDefinitionForName(name)
	if !ok {
		t.Fatalf("EPATH-081 fixture heat alias %q is not classified", name)
	}
	return canonicalEnergyExplanationSeries(energyExplanationSeriesForBuilder(&energyExplanationSeriesBuilder{
		dictionary: energyExplanationDictionary{
			row: sqlOutputDictionaryRow{keyValue: zone, name: name, units: "J"}, reportingFrequency: "Monthly", heat: &definition,
		},
		unit: "kWh", total: value, monthly: map[int]float64{1: value},
	}, sourceID))
}

func epath081Sources(series []energyExplanationSeries) []EnergyDataSource {
	out := make([]EnergyDataSource, 0, len(series))
	for _, item := range series {
		for _, sourceID := range item.SourceIDs {
			out = append(out, EnergyDataSource{
				ID: sourceID, SourceType: "acceptance_fixture", KeyValue: item.sourceKeyValue, Name: item.SourceName,
				Units: item.Unit, SourceUnit: item.Unit, NormalizedUnit: item.Unit, ReportingFrequency: "Monthly", ZoneName: item.ZoneName,
			})
		}
	}
	return out
}

func epath081UnitMultipliers(zones ...string) energyEffectiveMultiplierIndex {
	index := energyEffectiveMultiplierIndex{
		Enabled: true, Zones: map[string]energyZoneMultiplierRecord{}, SpaceZones: map[string]string{},
		OutputKeyZones: map[string]string{}, GroupMultipliers: map[string]float64{},
	}
	for _, zone := range zones {
		index.Zones[normalizePurposeToken(zone)] = energyZoneMultiplierRecord{ZoneName: zone, ZoneMultiplier: 1, GroupMultiplier: 1}
	}
	return index
}

func epath081AssertSimultaneousMetric(t *testing.T, node *EnergyExplanationNode, numerator float64, denominator float64) {
	t.Helper()
	if node == nil || node.SimultaneousLoad == nil {
		t.Errorf("load node has no simultaneous heating/cooling metric: %#v", node)
		return
	}
	metric := node.SimultaneousLoad
	wantRatio := roundSimulationDisplayNumber(numerator / denominator)
	if !metric.Available || metric.Numerator != numerator || metric.Denominator != denominator || math.Abs(metric.Ratio-wantRatio) > 1e-12 || metric.Basis != "simultaneous_min_over_max" {
		t.Errorf("%s simultaneous metric = %#v, want available numerator/denominator/ratio %g/%g/%g with simultaneous_min_over_max", node.ID, metric, numerator, denominator, wantRatio)
	}
}

func epath081AssertOffsetSummary(t *testing.T, effects []EnergyExplanationOffsetEffect, service string, effectKind string, effective float64, sourceIDs []string) {
	t.Helper()
	total := 0.0
	foundSources := []string{}
	for _, effect := range effects {
		if effect.TargetService != service || effect.EffectKind != effectKind {
			continue
		}
		total = roundedEnergyNumber(total + effect.EffectiveValue)
		foundSources = appendUniqueStrings(foundSources, effect.SourceIDs...)
	}
	if total != effective {
		t.Errorf("%s Offset effects total = %g from %#v, want %g", service, total, effects, effective)
	}
	for _, sourceID := range sourceIDs {
		if !epath081Contains(foundSources, sourceID) {
			t.Errorf("%s Offset effects lost source %q: %#v", service, sourceID, effects)
		}
	}
}

func epath081AssertLoadAndClosure(t *testing.T, nodes []EnergyExplanationNode, links []EnergyPathLink, zone string, service string, value float64) {
	t.Helper()
	loadID := epath081LoadID(service, zone)
	load := epath081NodeByID(nodes, loadID)
	if load == nil || load.Value != value {
		t.Errorf("%s %s load = %#v, want %g", zone, service, load, value)
		return
	}
	incoming := 0.0
	for _, link := range links {
		if link.Relation == "driver_to_load" && link.ToID == loadID {
			incoming = roundedEnergyNumber(incoming + link.ToValue)
		}
	}
	if incoming != value {
		t.Errorf("%s %s incoming contribution = %g, want exact closure %g", zone, service, incoming, value)
	}
}

func epath081Graph(t *testing.T, result EnergyExplanationResult, zone string, period string) ([]EnergyExplanationNode, []EnergyPathLink) {
	t.Helper()
	if zone == "" {
		if period == "annual" || period == "" {
			return result.Nodes, result.Links
		}
		if selected := epath081PeriodByID(result.Periods, period); selected != nil {
			return selected.Nodes, selected.Links
		}
		t.Fatalf("Building period %q is missing", period)
	}
	for _, item := range result.ZoneResults {
		if !strings.EqualFold(item.Scope.ZoneName, zone) {
			continue
		}
		if period == "annual" || period == "" {
			return item.Nodes, item.Links
		}
		if selected := epath081PeriodByID(item.Periods, period); selected != nil {
			return selected.Nodes, selected.Links
		}
		t.Fatalf("%s period %q is missing", zone, period)
	}
	t.Fatalf("zone result %q is missing", zone)
	return nil, nil
}

func epath081LoadID(service string, zone string) string {
	return "load." + service + "." + epath081ScopeToken(zone)
}

func epath081DriverID(category string, service string, zone string) string {
	return "driver." + canonicalEnergyPathCategory(category) + "." + service + "." + epath081ScopeToken(zone)
}

func epath081ScopeToken(zone string) string {
	if strings.TrimSpace(zone) == "" {
		return "building"
	}
	return metricID(zone)
}

func epath081NodeByID(nodes []EnergyExplanationNode, id string) *EnergyExplanationNode {
	for index := range nodes {
		if nodes[index].ID == id {
			return &nodes[index]
		}
	}
	return nil
}

func epath081LinkByIDs(links []EnergyPathLink, fromID string, toID string) *EnergyPathLink {
	for index := range links {
		if links[index].FromID == fromID && links[index].ToID == toID {
			return &links[index]
		}
	}
	return nil
}

func epath081PeriodByID(periods []EnergyPeriod, id string) *EnergyPeriod {
	for index := range periods {
		if periods[index].ID == id {
			return &periods[index]
		}
	}
	return nil
}

func epath081Contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
