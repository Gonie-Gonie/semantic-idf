package simulation

import "testing"

func TestEPATH080PreAllocationClosureIdentityIsOrderIndependent(t *testing.T) {
	makeSeries := func(reverse bool) []energyExplanationSeries {
		load := canonicalEnergyExplanationSeries(energyExplanationSeries{
			Stage: "load", Level: "load", Kind: "load.zone_cooling", CanonicalKind: "load.zone_cooling",
			Label: "Cooling load", Unit: "kWh", ServiceKind: "cooling", ZoneName: "Office", PathType: "zone",
			DriverCategory: "load.cooling", ThermalComponent: "sensible", SourceIDs: []string{"load"},
			Monthly: map[int]float64{1: 100}, RawMonthly: map[int]float64{1: 100}, Total: 100, RawTotal: 100,
			EffectiveMultiplier: 1, multiplierApplied: true, canonicalLoadMetadata: true,
		})
		driver := func(kind, category, sourceID string, value float64) energyExplanationSeries {
			return canonicalEnergyExplanationSeries(energyExplanationSeries{
				Stage: "driver", Level: "heat", Kind: kind, CanonicalKind: kind,
				Label: energyDriverCategoryLabel(category), Unit: "kWh", ZoneName: "Office",
				DriverSourceRole: energyDriverSourceRoleMainFlow, DriverCategory: category,
				ThermalComponent: "sensible", SourceIDs: []string{sourceID},
				Monthly: map[int]float64{1: value}, RawMonthly: map[int]float64{1: value}, Total: value, RawTotal: value,
				EffectiveMultiplier: 1, multiplierApplied: true, heatSignMultiplier: 1,
			})
		}
		physical := driver("heat.internal_people", energyDriverCategoryPeople, "people", 10)
		other := driver("heat.surface_reconciliation", energyDriverCategoryStorageOther, "surface-gap", 2)
		closure := driver("heat.unmapped_zone_balance", energyDriverCategoryStorageOther, "preallocation-closure", 88)
		if reverse {
			return []energyExplanationSeries{load, closure, other, physical}
		}
		return []energyExplanationSeries{load, physical, other, closure}
	}
	build := func(input []energyExplanationSeries) energyExplanationGraph {
		return buildEnergyExplanationGraphForPeriod("M1", input, PurposeAllocationPolicyDirectOnly, func(item energyExplanationSeries) float64 {
			return item.Monthly[1]
		})
	}
	forward := build(makeSeries(false))
	reverse := build(makeSeries(true))
	values := func(graph energyExplanationGraph) map[string]float64 {
		out := map[string]float64{}
		for _, node := range graph.Nodes {
			for _, sourceID := range node.SourceIDs {
				if sourceID == "people" || sourceID == "surface-gap" || sourceID == "preallocation-closure" {
					out[sourceID] = node.AllocatedValue
				}
			}
		}
		return out
	}
	forwardValues, reverseValues := values(forward), values(reverse)
	for sourceID, want := range map[string]float64{"people": 83.333, "surface-gap": 16.667, "preallocation-closure": 0} {
		if forwardValues[sourceID] != want || reverseValues[sourceID] != want {
			t.Fatalf("%s allocation changed with input order: forward=%#v reverse=%#v, want %g", sourceID, forwardValues, reverseValues, want)
		}
	}
	for _, graph := range []energyExplanationGraph{forward, reverse} {
		total := 0.0
		for _, edge := range graph.Edges {
			if edge.Relation == "heat_driver" {
				total = roundedEnergyNumber(total + edge.Value)
			}
		}
		if total != 100 {
			t.Fatalf("allocated ribbons = %g, want exact load closure 100", total)
		}
	}
}

func TestEPATH080AllZeroCanonicalLoadsKeepRawDriverAsExplicitZeroAllocation(t *testing.T) {
	definition, ok := energyHeatAliasDefinitionForName("Zone People Convective Heating Energy")
	if !ok {
		t.Fatal("People convective alias is unavailable")
	}
	load := canonicalEnergyExplanationSeries(energyExplanationSeries{
		Stage:            "load",
		CanonicalKind:    "load.zone_cooling",
		Level:            "load",
		Kind:             "load.zone_cooling",
		Label:            "Zone cooling load",
		Unit:             "kWh",
		ServiceKind:      "cooling",
		PathType:         "zone",
		ZoneName:         "Office",
		ThermalComponent: "sensible",
		SourceIDs:        []string{"zero-cooling-load"},
		Monthly:          map[int]float64{1: 0},
		SourceName:       "Zone Air System Sensible Cooling Energy",
		sourceName:       "Zone Air System Sensible Cooling Energy",
		sourceKeyValue:   "Office",
		sourceFrequency:  "Monthly",
	})
	driver := canonicalEnergyExplanationSeries(energyExplanationSeriesForBuilder(&energyExplanationSeriesBuilder{
		dictionary: energyExplanationDictionary{
			row:                sqlOutputDictionaryRow{keyValue: "Office", name: "Zone People Convective Heating Energy", units: "J"},
			reportingFrequency: "Monthly",
			heat:               &definition,
		},
		unit:    "kWh",
		total:   5,
		monthly: map[int]float64{1: 5},
	}, "people-pressure"))
	multipliers := energyEffectiveMultiplierIndex{
		Enabled:          true,
		Zones:            map[string]energyZoneMultiplierRecord{normalizePurposeToken("Office"): {ZoneName: "Office", ZoneMultiplier: 2, GroupMultiplier: 1}},
		SpaceZones:       map[string]string{},
		OutputKeyZones:   map[string]string{},
		GroupMultipliers: map[string]float64{},
	}
	sources := []EnergyDataSource{
		{ID: "zero-cooling-load", SourceType: "test", KeyValue: "Office", Name: load.SourceName, Units: "kWh", ZoneName: "Office"},
		{ID: "people-pressure", SourceType: "test", KeyValue: "Office", Name: driver.SourceName, Units: "kWh", ZoneName: "Office"},
	}
	legacy := buildEnergyExplanationResultWithDriverContext(
		[]energyExplanationSeries{load, driver},
		sources,
		&PurposeRunPlan{},
		energyDriverBuildContext{Enabled: true, Multipliers: multipliers},
	)
	result := UpgradeEnergyExplanationV1(legacy)

	for _, node := range result.Nodes {
		if node.Level == "driver" && node.DriverCategory == energyDriverCategoryPeople {
			t.Fatalf("raw driver with zero actual load reappeared in the main graph: %#v", node)
		}
	}
	for _, link := range result.Links {
		if link.Relation == "driver_to_load" {
			t.Fatalf("zero actual load produced a main driver ribbon: %#v", link)
		}
	}
	var source *EnergyDataSource
	for index := range result.Sources {
		if result.Sources[index].ID == "people-pressure" {
			source = &result.Sources[index]
			break
		}
	}
	if source == nil || source.RawValue != 5 || source.EffectiveValue != 10 || source.AllocatedValue != 0 || !source.AllocationApplied || source.AllocationFactor != 0 {
		t.Fatalf("allocated-zero raw driver provenance = %#v, want raw/effective/allocated 5/10/0", source)
	}
}
