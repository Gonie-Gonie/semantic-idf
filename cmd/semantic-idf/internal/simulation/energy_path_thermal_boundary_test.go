package simulation

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestEnergyPathThermalBoundaryMergeIsExplicitAndOrderIndependent(t *testing.T) {
	const active, mixed = "active_surface_source", "mixed_thermal_boundaries"
	for _, test := range []struct{ left, right, want string }{
		{"", "", ""}, {active, active, active}, {mixed, mixed, mixed},
		{active, "", mixed}, {"", active, mixed}, {mixed, active, mixed},
		{active, mixed, mixed}, {mixed, "", mixed}, {"", mixed, mixed},
	} {
		if got := mergeEnergyPathThermalBoundary(test.left, test.right); got != test.want {
			t.Errorf("merge(%q,%q)=%q, want %q", test.left, test.right, got, test.want)
		}
	}
}

func TestEnergyPathThermalBoundarySelectionDoesNotChangeAuthorityOrQuantity(t *testing.T) {
	candidates := epath070HierarchyCandidates("cooling", false)
	candidates[2].ThermalBoundary = "active_surface_source"
	for start := range candidates {
		selected := selectCanonicalEnergyExplanationLoads(candidates[start:])
		if len(selected) != 1 || selected[0].Total != candidates[start].Total || !reflect.DeepEqual(selected[0].SourceIDs, candidates[start].SourceIDs) || selected[0].ThermalBoundary != candidates[start].ThermalBoundary {
			t.Fatalf("boundary metadata changed authority or selected quantity at tier %d: %#v", start, selected)
		}
	}
	active := candidates[2]
	active.SourceKey, active.sourceKeyValue = "Floor A", "Floor A"
	other := epath070Load("baseboard", "Zone Baseboard Total Cooling Energy", "load.zone_equipment_cooling", "cooling", "zone", "Office", "combined", 7, map[int]float64{1: 7}, false, "Monthly")
	other.SourceKey, other.sourceKeyValue = "Baseboard B", "Baseboard B"
	beforeSelection, _ := json.Marshal([]energyExplanationSeries{active, other})
	for _, items := range [][]energyExplanationSeries{{active, other}, {other, active}} {
		selected := selectCanonicalEnergyExplanationLoads(items)
		if len(selected) != 1 || selected[0].ThermalBoundary != "mixed_thermal_boundaries" || selected[0].Total != 37 || selected[0].Monthly[1] != 37 || len(selected[0].SourceIDs) != 2 {
			t.Fatalf("additive selected physical families lost boundary/quantity: %#v", selected)
		}
		afterSelection, _ := json.Marshal([]energyExplanationSeries{active, other})
		if string(beforeSelection) != string(afterSelection) {
			t.Fatal("canonical load selection mutated original source maps/slices")
		}
	}
	second := active
	second.SourceKey, second.sourceKeyValue, second.SourceIDs = "Floor B", "Floor B", []string{"second-floor"}
	selected := selectCanonicalEnergyExplanationLoads([]energyExplanationSeries{active, second})
	if len(selected) != 1 || selected[0].Total != 60 || selected[0].ThermalBoundary != "active_surface_source" {
		t.Fatalf("two same-boundary sources became mixed or changed quantity: %#v", selected)
	}
	// Inspector-only breakdown/latent context is not an additive thermal
	// authority and must not turn an active selected load into a mixed load.
	for _, withContext := range []energyExplanationSeries{
		appendEnergyLoadBreakdownProvenance(active, other), appendEnergyLoadDetailProvenance(active, other),
	} {
		if withContext.ThermalBoundary != "active_surface_source" || withContext.Total != active.Total {
			t.Fatal("non-additive provenance changed the selected thermal boundary")
		}
	}
}

func TestEnergyPathThermalBoundaryCombinationOwnsPeriodMapsAndBreakdowns(t *testing.T) {
	backing := []string{"source-a", "untouched-1", "untouched-2", "untouched-3"}
	items := make([]energyExplanationSeries, 3)
	for i := range items {
		value := float64(i + 1)
		items[i] = epath070Load("source-"+string(rune('a'+i)), "Zone Radiant HVAC Cooling Energy", "load.zone_radiant_cooling", "cooling", "zone", "Office", "combined", value*10, map[int]float64{1: value * 10}, false, "Monthly")
		items[i].RawTotal, items[i].RawMonthly = value*2, map[int]float64{1: value * 2}
		items[i].Daily, items[i].RawDaily = map[int]float64{1: value * 10}, map[int]float64{1: value * 2}
		items[i].Hourly, items[i].RawHourly = map[int]float64{1: value * 10}, map[int]float64{1: value * 2}
		items[i].multiplierApplied = true
		component := "latent"
		if i == 0 {
			component = "sensible"
		}
		items[i].loadBreakdown = []energyLoadBreakdownSeries{{Component: component, Total: value, Monthly: map[int]float64{1: value}, Daily: map[int]float64{1: value}, Hourly: map[int]float64{1: value}, SourceIDs: []string{items[i].SourceIDs[0]}}}
	}
	items[0].SourceIDs = backing[:1]
	snapshot := func() string {
		parts := make([][]energyLoadBreakdownSeries, len(items))
		for i := range items {
			parts[i] = items[i].loadBreakdown
		}
		data, _ := json.Marshal(struct {
			Series []energyExplanationSeries
			Parts  [][]energyLoadBreakdownSeries
		}{items, parts})
		return string(data)
	}
	before := snapshot()
	for _, order := range [][]energyExplanationSeries{items, {items[2], items[1], items[0]}} {
		result, ok := combineCanonicalEnergyLoadSeries(order)
		if !ok || result.Total != 60 || result.RawTotal != 12 || result.Monthly[1] != 60 || result.RawMonthly[1] != 12 || result.Daily[1] != 60 || result.RawDaily[1] != 12 || result.Hourly[1] != 60 || result.RawHourly[1] != 12 {
			t.Fatalf("sum or original raw/effective period arithmetic changed: %#v", result)
		}
		for _, part := range result.loadBreakdown {
			want := 1.0
			if part.Component == "latent" {
				want = 5
			}
			if part.Total != want || part.Monthly[1] != want || part.Daily[1] != want || part.Hourly[1] != want {
				t.Fatalf("component merge lost independent source sum: %#v", part)
			}
		}
		if before != snapshot() || !reflect.DeepEqual(backing, []string{"source-a", "untouched-1", "untouched-2", "untouched-3"}) {
			t.Fatal("combination mutated original maps, appended-component maps, or backing slices")
		}
	}
}

func energyPathThermalBoundaryPair(service, boundary string, carriers []string) (EnergyExplanationNode, EnergyExplanationNode, EnergyPathLink) {
	load := EnergyExplanationNode{ID: "load." + service + ".building", Level: "load", ServiceKind: service, ThermalBoundary: boundary, Value: 100, EffectiveValue: 100, Unit: "kWh"}
	endUse := EnergyExplanationNode{ID: "end_use." + service + ".building", Level: "end_use", EndUse: service, Value: 25, EffectiveValue: 25, Unit: "kWh", endUseCarriers: append([]string(nil), carriers...)}
	link := EnergyPathLink{FromID: load.ID, ToID: endUse.ID, Relation: "load_to_end_use", ServiceKind: service, FromValue: 100, ToValue: 25, FromUnit: "kWh", ToUnit: "kWh", RatioKind: "stale", RatioLabel: "COP", Ratio: 999}
	return load, endUse, link
}

func TestEnergyPathThermalBoundaryConversionIsNotCOPOrEfficiency(t *testing.T) {
	for _, boundary := range []string{"active_surface_source", "mixed_thermal_boundaries"} {
		for _, service := range []string{"cooling", "heating"} {
			for _, carriers := range [][]string{{"electricity"}, {"natural_gas"}, {"district_cooling"}, {"district_heating"}, {"electricity", "district_cooling"}} {
				load, endUse, link := energyPathThermalBoundaryPair(service, boundary, carriers)
				setEnergyPathConversionRatioKind(&link, &load, &endUse)
				finalizeEnergyPathLinkRatio(&link)
				label := "Active surface source / site energy"
				if boundary == "mixed_thermal_boundaries" {
					label = "Mixed thermal boundaries / site energy"
				}
				if link.RatioKind != "load_to_site_energy" || link.RatioLabel != label || link.Ratio != 4 || link.FromValue != 100 || link.ToValue != 25 {
					t.Fatalf("%s %s %v changed quantity or inferred equipment efficiency: %#v", boundary, service, carriers, link)
				}
			}
		}
	}
	for _, test := range []struct{ service, carrier, kind, label string }{
		{"cooling", "electricity", "coefficient_of_performance", "COP"},
		{"heating", "electricity", "load_to_site_energy", "Load / site energy"},
		{"heating", "natural_gas", "load_to_fuel", "Load / fuel"},
		{"cooling", "district_cooling", "load_to_purchased_energy", "Load / purchased energy"},
	} {
		load, endUse, link := energyPathThermalBoundaryPair(test.service, "", []string{test.carrier})
		setEnergyPathConversionRatioKind(&link, &load, &endUse)
		finalizeEnergyPathLinkRatio(&link)
		if link.RatioKind != test.kind || link.RatioLabel != test.label || link.Ratio != 4 {
			t.Fatalf("unmarked legacy ratio changed: %#v", link)
		}
	}
	load, endUse, link := energyPathThermalBoundaryPair("heating", "", []string{"natural_gas"})
	load.Value, load.EffectiveValue, link.FromValue = 20, 20, 20
	setEnergyPathConversionRatioKind(&link, &load, &endUse)
	finalizeEnergyPathLinkRatio(&link)
	if link.RatioKind != "efficiency" || link.Ratio != .8 {
		t.Fatalf("unmarked combustion efficiency changed: %#v", link)
	}
}

func TestEnergyPathThermalBoundaryKeepsInvalidConversionGuards(t *testing.T) {
	for _, mutate := range []func(*EnergyExplanationNode, *EnergyExplanationNode, *EnergyPathLink){
		func(l, e *EnergyExplanationNode, p *EnergyPathLink) { l.Value, l.EffectiveValue = 0, 0 },
		func(l, e *EnergyExplanationNode, p *EnergyPathLink) { e.Value, e.EffectiveValue = 0, 0 },
		func(l, e *EnergyExplanationNode, p *EnergyPathLink) { l.Value, l.EffectiveValue = -1, -1 },
		func(l, e *EnergyExplanationNode, p *EnergyPathLink) {
			l.Value, l.EffectiveValue = math.NaN(), math.NaN()
		},
		func(l, e *EnergyExplanationNode, p *EnergyPathLink) { p.ToValue = 0 },
		func(l, e *EnergyExplanationNode, p *EnergyPathLink) { p.ToValue = math.Inf(1) },
		func(l, e *EnergyExplanationNode, p *EnergyPathLink) { p.FromUnit = "W" },
		func(l, e *EnergyExplanationNode, p *EnergyPathLink) { e.EndUse = "fans" },
		func(l, e *EnergyExplanationNode, p *EnergyPathLink) { e.endUseCarriers = nil },
	} {
		load, endUse, link := energyPathThermalBoundaryPair("cooling", "active_surface_source", []string{"electricity"})
		mutate(&load, &endUse, &link)
		setEnergyPathConversionRatioKind(&link, &load, &endUse)
		if link.RatioKind != "" || link.RatioLabel != "" || link.Ratio != 0 {
			t.Fatalf("metadata bypassed an existing quantity/identity guard: %#v", link)
		}
	}
}

func TestEnergyPathThermalBoundaryAnnualMergeReclassifiesWithoutChangingSums(t *testing.T) {
	for _, boundaries := range [][]string{{"active_surface_source", ""}, {"", "active_surface_source"}, {"active_surface_source", "active_surface_source"}, {"", ""}} {
		periods := []EnergyPeriod{}
		for i, boundary := range boundaries {
			load, endUse, link := energyPathThermalBoundaryPair("cooling", boundary, []string{"electricity"})
			period := "M1"
			if i == 1 {
				period = "M2"
			}
			load.Period, endUse.Period, link.Period = period, period, period
			setEnergyPathConversionRatioKind(&link, &load, &endUse)
			finalizeEnergyPathLinkRatio(&link)
			periods = append(periods, EnergyPeriod{ID: period, Kind: "monthly", Nodes: []EnergyExplanationNode{load, endUse}, Links: []EnergyPathLink{link}})
		}
		before, _ := json.Marshal(periods)
		nodes, links, _, _ := aggregateEnergyPathV2MonthlyPeriods(periods)
		load := energyPathV2NodeByID(nodes, "load.cooling.building")
		want := boundaries[0]
		if boundaries[0] != boundaries[1] {
			want = "mixed_thermal_boundaries"
		}
		if load == nil || load.ThermalBoundary != want || load.Value != 200 || load.Period != "annual" || len(links) != 1 || links[0].FromValue != 200 || links[0].ToValue != 50 || links[0].Ratio != 4 {
			t.Fatalf("annual node/link lost temporal boundary or source sums: %#v %#v", nodes, links)
		}
		kind := "load_to_site_energy"
		if want == "" {
			kind = "coefficient_of_performance"
		}
		if links[0].RatioKind != kind {
			t.Fatalf("annual ratio retained a first-month-only label: %#v", links[0])
		}
		after, _ := json.Marshal(periods)
		if string(before) != string(after) {
			t.Fatal("annual aggregation mutated monthly nodes/links")
		}
	}
}

func TestEnergyPathThermalBoundaryV2ScopeAndWirePreservation(t *testing.T) {
	legacy := energyExplanationV1ConversionFixture()
	for i := range legacy.Nodes {
		if legacy.Nodes[i].Level == "load" {
			legacy.Nodes[i].ThermalBoundary = "active_surface_source"
		}
		if legacy.Nodes[i].Level == "energy" && legacy.Nodes[i].EndUse == "cooling" {
			// This metadata fixture explicitly observes a local site endpoint.
			// The original generic fixture is Building-only under direct_only
			// and correctly has NO Zone conversion without such ownership.
			legacy.Nodes[i].ZoneName = "Office"
			legacy.Nodes[i].MeterHierarchyLevel = "zone_direct_use"
		}
	}
	legacy.Periods[0].Nodes = cloneEnergyExplanationNodes(legacy.Nodes)
	month := legacy.Periods[0]
	month.ID, month.Kind = "M1", "monthly"
	month.Nodes = cloneEnergyExplanationNodes(legacy.Nodes)
	month.Edges = append([]EnergyExplanationEdge(nil), legacy.Edges...)
	for i := range month.Nodes {
		month.Nodes[i].Period = "M1"
	}
	for i := range month.Edges {
		month.Edges[i].Period = "M1"
	}
	legacy.Periods = append(legacy.Periods, month)
	result := UpgradeEnergyExplanationV1(legacy)
	assertGraph := func(nodes []EnergyExplanationNode, links []EnergyPathLink, owner string) {
		t.Helper()
		load := energyPathV2NodeByID(nodes, "load.cooling."+owner)
		link := energyPathV2LinkByRelation(links, "load_to_end_use")
		if load == nil || load.ThermalBoundary != "active_surface_source" || load.Value != 100 || !stringSliceContains(load.SourceIDs, "load") ||
			link == nil || link.RatioKind != "load_to_site_energy" || link.RatioLabel != "Active surface source / site energy" || link.FromValue != 100 || link.ToValue != 25 || link.Ratio != 4 {
			t.Fatalf("%s projection lost boundary or changed values/provenance: %#v %#v", owner, load, link)
		}
	}
	assertGraph(result.Nodes, result.Links, "building")
	monthly := energyExplanationPeriodByID(result.Periods, "M1")
	if monthly == nil || len(result.ZoneResults) != 1 {
		t.Fatal("missing normal Building/Zone monthly contexts")
	}
	assertGraph(monthly.Nodes, monthly.Links, "building")
	zone := result.ZoneResults[0]
	assertGraph(zone.Nodes, zone.Links, "office")
	zoneMonth := energyExplanationPeriodByID(zone.Periods, "M1")
	if zoneMonth == nil {
		t.Fatal("missing Zone monthly context")
	}
	assertGraph(zoneMonth.Nodes, zoneMonth.Links, "office")
	data, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(data), `"thermalBoundary":"active_surface_source"`) {
		t.Fatalf("boundary absent from original V2 wire: %v", err)
	}
	var decoded EnergyExplanationResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	assertGraph(decoded.Nodes, decoded.Links, "building")
	assertGraph(decoded.ZoneResults[0].Nodes, decoded.ZoneResults[0].Links, "office")
	plain, err := json.Marshal(UpgradeEnergyExplanationV1(energyExplanationV1ConversionFixture()))
	if err != nil || strings.Contains(string(plain), `"thermalBoundary"`) {
		t.Fatal("new optional metadata changed the unmarked legacy wire")
	}
}
