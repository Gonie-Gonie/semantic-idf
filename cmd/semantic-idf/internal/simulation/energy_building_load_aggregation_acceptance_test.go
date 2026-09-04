package simulation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEPATH071AcceptanceZoneMonthMultipliersApplyOnceAndServicesStaySeparate(t *testing.T) {
	series := []energyExplanationSeries{
		epath071Load("office-cooling", "Office", "cooling", "sensible", 15, map[int]float64{1: 10, 2: 5}),
		epath071Load("office-heating", "Office", "heating", "sensible", 5, map[int]float64{1: 4, 2: 1}),
		epath071Load("lab-cooling", "Lab", "cooling", "sensible", 10, map[int]float64{1: 7, 2: 3}),
		epath071Load("lab-heating", "Lab", "heating", "sensible", 7, map[int]float64{1: 2, 2: 5}),
	}
	index := energyEffectiveMultiplierIndex{
		Enabled: true,
		Zones: map[string]energyZoneMultiplierRecord{
			normalizePurposeToken("Office"): {ZoneName: "Office", ZoneMultiplier: 2, GroupMultiplier: 3},
			normalizePurposeToken("Lab"):    {ZoneName: "Lab", ZoneMultiplier: 2, GroupMultiplier: 1},
		},
		SpaceZones:       map[string]string{},
		OutputKeyZones:   map[string]string{},
		GroupMultipliers: map[string]float64{},
	}
	result := epath071BuildCanonicalResult(series, index)

	for _, expected := range []struct {
		period  string
		service string
		value   float64
	}{
		{period: "M1", service: "cooling", value: 74},
		{period: "M2", service: "cooling", value: 36},
		{period: "annual", service: "cooling", value: 110},
		{period: "M1", service: "heating", value: 28},
		{period: "M2", service: "heating", value: 16},
		{period: "annual", service: "heating", value: 44},
	} {
		node := epath071LoadNode(result, expected.period, expected.service)
		if node == nil || node.Value != expected.value {
			t.Errorf("%s %s Building load = %#v, want %g after one multiplier application", expected.period, expected.service, node, expected.value)
		}
	}

	for sourceID, expected := range map[string]struct {
		raw        float64
		effective  float64
		multiplier float64
	}{
		"office-cooling": {raw: 15, effective: 90, multiplier: 6},
		"office-heating": {raw: 5, effective: 30, multiplier: 6},
		"lab-cooling":    {raw: 10, effective: 20, multiplier: 2},
		"lab-heating":    {raw: 7, effective: 14, multiplier: 2},
	} {
		source := epath071SourceByID(result.Sources, sourceID)
		if source == nil || source.RawValue != expected.raw || source.EffectiveValue != expected.effective || source.EffectiveMultiplier != expected.multiplier {
			t.Errorf("source %q accounting = %#v, want raw=%g effective=%g multiplier=%g", sourceID, source, expected.raw, expected.effective, expected.multiplier)
		}
	}

	epath071AssertZoneClosure(t, result, "annual", "cooling")
	epath071AssertZoneClosure(t, result, "annual", "heating")
	epath071AssertZoneClosure(t, result, "M1", "cooling")
	epath071AssertZoneClosure(t, result, "M1", "heating")
	epath071AssertZoneClosure(t, result, "M2", "cooling")
	epath071AssertZoneClosure(t, result, "M2", "heating")
}

func TestEPATH071AcceptanceActualZoneLoadsAreAuthoritativeOverInterzoneTransfer(t *testing.T) {
	series := []energyExplanationSeries{
		epath071Load("office-load", "Office", "cooling", "sensible", 60, map[int]float64{1: 60}),
		epath071Load("lab-load", "Lab", "heating", "sensible", 50, map[int]float64{1: 50}),
		epath071Interzone("office-interzone", "Office", "cooling", 20),
		epath071Interzone("lab-interzone", "Lab", "heating", 20),
	}
	result := epath071BuildCanonicalResult(series, epath071UnitMultiplierIndex("Office", "Lab"))
	cooling := epath071LoadNode(result, "annual", "cooling")
	heating := epath071LoadNode(result, "annual", "heating")
	if cooling == nil || cooling.Value != 60 || heating == nil || heating.Value != 50 {
		t.Fatalf("Building loads = cooling %#v / heating %#v; interzone transfer must not be added to actual delivered load", cooling, heating)
	}
	for _, node := range []*EnergyExplanationNode{cooling, heating} {
		if epath071Contains(node.SourceIDs, "office-interzone") || epath071Contains(node.SourceIDs, "lab-interzone") {
			t.Errorf("interzone source contaminated authoritative load provenance: %#v", node)
		}
	}
	epath071AssertZoneClosure(t, result, "annual", "cooling")
	epath071AssertZoneClosure(t, result, "annual", "heating")
}

func TestEPATH071AcceptancePrimaryLoadStageAndLatentDetails(t *testing.T) {
	series := []energyExplanationSeries{
		epath071Load("cooling-sensible", "Office", "cooling", "sensible", 90, map[int]float64{1: 90}),
		epath071Load("cooling-latent", "Office", "cooling", "latent", 10, map[int]float64{1: 10}),
		epath071Load("heating-sensible", "Office", "heating", "sensible", 91, map[int]float64{1: 91}),
		epath071Load("heating-latent", "Office", "heating", "latent", 9, map[int]float64{1: 9}),
		epath071DetailLoad("dehumidification-detail", "Office", "dehumidification", 10),
		epath071DetailLoad("humidification-detail", "Office", "humidification", 9),
	}
	result := epath071BuildCanonicalResult(series, epath071UnitMultiplierIndex("Office"))

	loadNodes := []EnergyExplanationNode{}
	for _, node := range result.Nodes {
		if node.Level == "load" {
			loadNodes = append(loadNodes, node)
		}
	}
	if len(loadNodes) != 2 || epath071NodeByID(loadNodes, "load.cooling.building") == nil || epath071NodeByID(loadNodes, "load.heating.building") == nil {
		t.Errorf("default v2 Load stage = %#v, want exactly Cooling and Heating primary nodes", loadNodes)
	}
	for _, unwanted := range []string{"load.humidification.building", "load.dehumidification.building"} {
		if node := epath071NodeByID(result.Nodes, unwanted); node != nil {
			t.Errorf("humidification/dehumidification escaped inspector breakdown as primary node: %#v", node)
		}
	}

	cooling := epath071NodeByID(result.Nodes, "load.cooling.building")
	heating := epath071NodeByID(result.Nodes, "load.heating.building")
	if cooling == nil || cooling.Value != 100 || cooling.ThermalComponent != "combined" ||
		heating == nil || heating.Value != 100 || heating.ThermalComponent != "combined" {
		t.Fatalf("primary sensible+latent totals = cooling %#v / heating %#v, want 100 kWh combined each", cooling, heating)
	}
	for _, expected := range []struct {
		node      *EnergyExplanationNode
		sourceID  string
		component string
	}{
		{node: cooling, sourceID: "cooling-latent", component: "load.delivered.latent"},
		{node: heating, sourceID: "heating-latent", component: "load.delivered.latent"},
		{node: cooling, sourceID: "dehumidification-detail", component: "load.dehumidification"},
		{node: heating, sourceID: "humidification-detail", component: "load.humidification"},
	} {
		if expected.node == nil || !epath071Contains(expected.node.SourceIDs, expected.sourceID) {
			t.Errorf("load node lost inspector breakdown source %q: %#v", expected.sourceID, expected.node)
		}
		source := epath071SourceByID(result.Sources, expected.sourceID)
		if source == nil || source.InspectorSection != energyDriverInspectorSectionBreakdown || source.DriverComponent != expected.component {
			t.Errorf("inspector breakdown source %q = %#v, want section=%q component=%q", expected.sourceID, source, energyDriverInspectorSectionBreakdown, expected.component)
		}
	}

	dehumidification := epath071SourceByID(result.Sources, "dehumidification-detail")
	if !epath071HasEmphasisMetadata(cooling, dehumidification, "dehumid") {
		t.Errorf("10%% dehumidification has no badge/emphasis metadata on load or inspector source: node=%#v source=%#v", cooling, dehumidification)
	}
	humidification := epath071SourceByID(result.Sources, "humidification-detail")
	if epath071HasEmphasisMetadata(heating, humidification, "humid") {
		t.Errorf("9%% humidification was emphasized below the 10%% threshold: node=%#v source=%#v", heating, humidification)
	}
}

func TestEPATH071AcceptanceZoneBreakdownClosesAfterDeterministicRoundingAndZeros(t *testing.T) {
	series := []energyExplanationSeries{
		epath071Load("a-cooling", "Zone A", "cooling", "sensible", 0.3334, map[int]float64{1: 0.3334, 2: 0}),
		epath071Load("b-cooling", "Zone B", "cooling", "sensible", 0.6668, map[int]float64{1: 0.3334, 2: 0.3334}),
		epath071Load("zero-cooling", "Zero Zone", "cooling", "sensible", 0, map[int]float64{1: 0, 2: 0}),
		epath071Load("zero-heating", "Zero Zone", "heating", "sensible", 0, map[int]float64{1: 0, 2: 0}),
	}
	result := epath071BuildCanonicalResult(series, epath071UnitMultiplierIndex("Zone A", "Zone B", "Zero Zone"))

	for _, expected := range []struct {
		period string
		value  float64
	}{
		{period: "M1", value: 0.666},
		{period: "M2", value: 0.333},
		{period: "annual", value: 0.999},
	} {
		node := epath071LoadNode(result, expected.period, "cooling")
		if node == nil || node.Value != expected.value {
			t.Errorf("rounded %s Building cooling = %#v, want %g", expected.period, node, expected.value)
		}
		epath071AssertZoneClosure(t, result, expected.period, "cooling")
	}
	for _, period := range []string{"M1", "M2", "annual"} {
		if node := epath071LoadNode(result, period, "heating"); node != nil {
			t.Errorf("zero-only %s heating source created a primary node: %#v", period, node)
		}
	}
}

// EPATH-071 is a canonical v2 presentation rule. Direct v1 callers and stored
// manifests retain the historical nodes until the v1-to-v2 boundary.
func TestEPATH071AcceptanceFrozenV1LoadShapeIsUnchanged(t *testing.T) {
	series := []energyExplanationSeries{
		epath071Load("cooling", "Office", "cooling", "sensible", 10, map[int]float64{1: 10}),
		epath071DetailLoad("humidification", "Office", "humidification", 2),
		epath071DetailLoad("dehumidification", "Office", "dehumidification", 3),
	}
	legacy := buildEnergyExplanationResult(series, epath071Sources(series), &PurposeRunPlan{})
	if legacy.Schema != energyExplanationV1Schema {
		t.Fatalf("frozen adapter schema = %q, want %q", legacy.Schema, energyExplanationV1Schema)
	}
	for id, value := range map[string]float64{
		"load.cooling.office":          10,
		"load.humidification.office":   2,
		"load.dehumidification.office": 3,
	} {
		node := epath071NodeByID(legacy.Nodes, id)
		if node == nil || node.Level != "load" || node.Value != value {
			t.Errorf("frozen v1 node %q = %#v, want historical load value %g", id, node, value)
		}
	}
}

func epath071Load(sourceID string, zone string, service string, component string, total float64, monthly map[int]float64) energyExplanationSeries {
	kind := "load.zone_" + service
	name := "Zone Air System Sensible " + strings.Title(service) + " Energy"
	if component == "latent" {
		kind = "load.zone_latent_" + service
		name = "Zone Air System Latent " + strings.Title(service) + " Energy"
	}
	return canonicalEnergyExplanationSeries(energyExplanationSeries{
		Stage:            "load",
		CanonicalKind:    kind,
		Level:            "load",
		Kind:             kind,
		Label:            "Zone " + service + " load",
		Unit:             "kWh",
		ServiceKind:      service,
		PathType:         "zone",
		ZoneName:         zone,
		ThermalComponent: component,
		SourceIDs:        []string{sourceID},
		Total:            total,
		Monthly:          monthly,
		SourceName:       name,
		sourceName:       name,
		sourceKeyValue:   zone,
		sourceFrequency:  "Monthly",
	})
}

func epath071DetailLoad(sourceID string, zone string, service string, value float64) energyExplanationSeries {
	name := "Zone Ideal Loads Zone Latent Heating Energy"
	if service == "dehumidification" {
		name = "Zone Ideal Loads Zone Latent Cooling Energy"
	}
	return canonicalEnergyExplanationSeries(energyExplanationSeries{
		Stage:            "load",
		CanonicalKind:    "load.zone_" + service,
		Level:            "load",
		Kind:             "load.zone_" + service,
		Label:            "Zone " + service + " load",
		Unit:             "kWh",
		ServiceKind:      service,
		PathType:         "zone",
		ZoneName:         zone,
		ThermalComponent: "latent",
		SourceIDs:        []string{sourceID},
		Total:            value,
		Monthly:          map[int]float64{1: value},
		SourceName:       name,
		sourceName:       name,
		sourceKeyValue:   zone,
		sourceFrequency:  "Monthly",
	})
}

func epath071Interzone(sourceID string, zone string, service string, value float64) energyExplanationSeries {
	signed := value
	sign := "positive"
	multiplier := 1.0
	if service == "heating" {
		signed = -value
		sign = "negative"
		multiplier = -1
	}
	return canonicalEnergyExplanationSeries(energyExplanationSeries{
		Stage:                  "driver",
		CanonicalKind:          "heat.interzone_pair",
		Level:                  "heat",
		Kind:                   "heat.interzone_pair",
		Label:                  "Pairwise interzone heat",
		Unit:                   "kWh",
		ZoneName:               zone,
		ServiceKind:            service,
		ThermalComponent:       "sensible",
		DriverCategory:         energyDriverCategoryInterzoneAir,
		DriverSourceRole:       energyDriverSourceRoleMainFlow,
		Sign:                   sign,
		HeatSign:               sign,
		SourceIDs:              []string{sourceID},
		Total:                  value,
		Monthly:                map[int]float64{1: value},
		SourceName:             "Pairwise Interzone Heat Transfer Energy",
		sourceName:             "Pairwise Interzone Heat Transfer Energy",
		sourceKeyValue:         zone,
		sourceFrequency:        "Monthly",
		heatSignMultiplier:     multiplier,
		driverZoneOnly:         true,
		interzonePairID:        "pair.office.lab",
		RawTotal:               value,
		RawMonthly:             map[int]float64{1: value},
		EffectiveMultiplier:    1,
		MultiplierApplication:  energyMultiplierRequiresZone,
		multiplierApplied:      false,
		SelectedRange:          signed,
		RawSelectedRange:       signed,
		HasSelectedRange:       false,
		SelectedRangeSourceIDs: nil,
	})
}

func epath071BuildCanonicalResult(series []energyExplanationSeries, multipliers energyEffectiveMultiplierIndex) EnergyExplanationResult {
	legacy := buildEnergyExplanationResultWithDriverContext(
		series,
		epath071Sources(series),
		&PurposeRunPlan{},
		energyDriverBuildContext{Enabled: true, Multipliers: multipliers},
	)
	return UpgradeEnergyExplanationV1(legacy)
}

func epath071Sources(series []energyExplanationSeries) []EnergyDataSource {
	out := []EnergyDataSource{}
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
				ReportingFrequency: item.sourceFrequency,
				ZoneName:           item.ZoneName,
			})
		}
	}
	return out
}

func epath071UnitMultiplierIndex(zones ...string) energyEffectiveMultiplierIndex {
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

func epath071LoadNode(result EnergyExplanationResult, period string, service string) *EnergyExplanationNode {
	id := "load." + service + ".building"
	if period == "" || period == "annual" {
		return epath071NodeByID(result.Nodes, id)
	}
	selected := epath071PeriodByID(result.Periods, period)
	if selected == nil {
		return nil
	}
	return epath071NodeByID(selected.Nodes, id)
}

func epath071AssertZoneClosure(t *testing.T, result EnergyExplanationResult, period string, service string) {
	t.Helper()
	building := epath071LoadNode(result, period, service)
	if building == nil {
		t.Errorf("%s Building %s load is missing", period, service)
		return
	}
	zoneTotal := 0.0
	for _, zone := range result.ZoneResults {
		var node *EnergyExplanationNode
		if period == "" || period == "annual" {
			node = epath071NodeByID(zone.Nodes, "load."+service+"."+metricID(zone.Scope.ZoneName))
		} else if selected := epath071PeriodByID(zone.Periods, period); selected != nil {
			node = epath071NodeByID(selected.Nodes, "load."+service+"."+metricID(zone.Scope.ZoneName))
		}
		if node != nil {
			zoneTotal = roundedEnergyNumber(zoneTotal + node.Value)
		}
	}
	if zoneTotal != building.Value {
		t.Errorf("%s %s zone breakdown sum = %g, Building total = %g", period, service, zoneTotal, building.Value)
	}
}

func epath071HasEmphasisMetadata(node *EnergyExplanationNode, source *EnergyDataSource, token string) bool {
	for _, candidate := range []any{node, source} {
		if candidate == nil {
			continue
		}
		encoded, err := json.Marshal(candidate)
		if err != nil {
			continue
		}
		var value any
		if json.Unmarshal(encoded, &value) == nil && epath071ScanEmphasis(value, strings.ToLower(token)) {
			return true
		}
	}
	return false
}

func epath071ScanEmphasis(value any, token string) bool {
	switch typed := value.(type) {
	case map[string]any:
		encoded, _ := json.Marshal(typed)
		contextMatches := strings.Contains(strings.ToLower(string(encoded)), token)
		for key, child := range typed {
			normalizedKey := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			if contextMatches && (strings.Contains(normalizedKey, "emphas") || strings.Contains(normalizedKey, "highlight") || strings.Contains(normalizedKey, "badge")) && epath071Truthy(child) {
				return true
			}
			if epath071ScanEmphasis(child, token) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if epath071ScanEmphasis(child, token) {
				return true
			}
		}
	}
	return false
}

func epath071Truthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.TrimSpace(typed) != "" && !strings.EqualFold(typed, "false")
	case float64:
		return typed != 0
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return false
	}
}

func epath071NodeByID(nodes []EnergyExplanationNode, id string) *EnergyExplanationNode {
	for index := range nodes {
		if nodes[index].ID == id {
			return &nodes[index]
		}
	}
	return nil
}

func epath071SourceByID(sources []EnergyDataSource, id string) *EnergyDataSource {
	for index := range sources {
		if sources[index].ID == id {
			return &sources[index]
		}
	}
	return nil
}

func epath071PeriodByID(periods []EnergyPeriod, id string) *EnergyPeriod {
	for index := range periods {
		if periods[index].ID == id {
			return &periods[index]
		}
	}
	return nil
}

func epath071Contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
