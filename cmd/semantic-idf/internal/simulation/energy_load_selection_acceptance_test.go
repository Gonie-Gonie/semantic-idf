package simulation

import "testing"

// EPATH-070 consumes these names at the canonical boundary. Keeping this list
// explicit prevents a correct selector from being bypassed because an
// authoritative or diagnostic EnergyPlus output was never classified.
func TestEPATH070AcceptanceCanonicalLoadAliasesAreDiscoverable(t *testing.T) {
	tests := []struct {
		name    string
		service string
	}{
		{name: "Zone Air System Sensible Cooling Energy", service: "cooling"},
		{name: "Zone Air System Latent Cooling Energy", service: "cooling"},
		{name: "Zone Air System Sensible Heating Energy", service: "heating"},
		{name: "Zone Air System Latent Heating Energy", service: "heating"},
		{name: "Zone Ideal Loads Zone Total Cooling Energy", service: "cooling"},
		{name: "Zone Ideal Loads Zone Total Heating Energy", service: "heating"},
		{name: "Zone Radiant HVAC Cooling Energy", service: "cooling"},
		{name: "Zone Radiant HVAC Heating Energy", service: "heating"},
		{name: "Cooling Coil Total Cooling Energy", service: "cooling"},
		{name: "Heating Coil Heating Energy", service: "heating"},
		{name: "Plant Loop Cooling Demand Energy", service: "cooling"},
		{name: "Plant Loop Heating Demand Energy", service: "heating"},
		{name: "Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate", service: "cooling"},
		{name: "Zone Predicted Sensible Load to Heating Setpoint Heat Transfer Rate", service: "heating"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition, ok := energyLoadAliasDefinitionForName(test.name)
			if !ok {
				t.Fatalf("EPATH-070 load alias %q is not classified", test.name)
			}
			if definition.ServiceKind != test.service {
				t.Fatalf("EPATH-070 load alias %q service = %q, want %q", test.name, definition.ServiceKind, test.service)
			}
		})
	}
}

// The hierarchy is evaluated for one normalized zone/service target. Coverage
// is held equal so this test isolates physical authority rather than reporting
// frequency: actual zone air > Ideal Loads total > direct zone equipment >
// system/coil > plant.
func TestEPATH070AcceptanceCanonicalLoadHierarchy(t *testing.T) {
	for _, service := range []string{"cooling", "heating"} {
		candidates := epath070HierarchyCandidates(service, true)
		for start := range candidates {
			start := start
			t.Run(service+"/"+candidates[start].SourceIDs[0]+"-fallback", func(t *testing.T) {
				selected := selectCanonicalEnergyExplanationLoads(append([]energyExplanationSeries(nil), candidates[start:]...))
				if len(selected) != 1 {
					t.Fatalf("selected %d canonical %s load families, want one authoritative family: %#v", len(selected), service, selected)
				}
				wantSource := candidates[start].SourceIDs[0]
				if !epath070Contains(selected[0].SourceIDs, wantSource) {
					t.Fatalf("selected %s source IDs = %#v, want highest available tier %q", service, selected[0].SourceIDs, wantSource)
				}
				for _, lower := range candidates[start+1:] {
					if epath070Contains(selected[0].SourceIDs, lower.SourceIDs[0]) {
						t.Fatalf("lower-authority source %q was spliced into selected %s family: %#v", lower.SourceIDs[0], service, selected[0])
					}
				}
			})
		}
	}
}

// Real aliases have different legacy Kind and scope fields. Those differences
// must not cause every physical level to survive preference selection and then
// be summed into the one v2 load node. Each suffix also verifies that direct
// zone equipment and central/plant sources remain valid fallbacks when all
// higher tiers are absent.
func TestEPATH070AcceptanceHierarchyPreventsCrossLevelDoubleCounting(t *testing.T) {
	for _, service := range []string{"cooling", "heating"} {
		candidates := epath070HierarchyCandidates(service, false)
		for start := range candidates {
			start := start
			t.Run(service+"/"+candidates[start].SourceIDs[0]+"-primary", func(t *testing.T) {
				result := epath070BuildResult(candidates[start:])
				node := epath070NodeByID(result.Nodes, "load."+service+".building")
				if node == nil {
					t.Fatalf("highest available %s tier %q did not produce a canonical Building load: %#v", service, candidates[start].SourceIDs[0], result.Nodes)
				}
				wantValue := candidates[start].Total
				if node.Value != wantValue {
					t.Fatalf("canonical Building %s load = %g, want authoritative tier %q value %g without lower-level double count; node=%#v", service, node.Value, candidates[start].SourceIDs[0], wantValue, node)
				}
				if !epath070Contains(node.SourceIDs, candidates[start].SourceIDs[0]) {
					t.Fatalf("canonical Building %s load lost authoritative source %q: %#v", service, candidates[start].SourceIDs[0], node)
				}
			})
		}
	}
}

func TestEPATH070AcceptanceSensibleLatentTotalAndBreakdownProvenance(t *testing.T) {
	t.Run("zone air components form one total", func(t *testing.T) {
		series := []energyExplanationSeries{
			epath070Load("actual-sensible", "Zone Air System Sensible Cooling Energy", "load.zone_cooling", "cooling", "zone", "Office", "sensible", 80, map[int]float64{1: 80}, false, "Monthly"),
			epath070Load("actual-latent", "Zone Air System Latent Cooling Energy", "load.zone_latent_cooling", "cooling", "zone", "Office", "latent", 20, map[int]float64{1: 20}, false, "Monthly"),
		}
		epath070AssertCombinedLoad(t, epath070BuildResult(series), "cooling", 100, map[string]float64{
			"actual-sensible": 80,
			"actual-latent":   20,
		})
	})

	t.Run("Ideal Loads total is authoritative but components remain provenance", func(t *testing.T) {
		series := []energyExplanationSeries{
			epath070Load("ideal-total", "Zone Ideal Loads Zone Total Cooling Energy", "load.zone_cooling", "cooling", "zone", "Office", "combined", 100, map[int]float64{1: 100}, false, "Monthly"),
			epath070Load("ideal-sensible", "Zone Ideal Loads Zone Sensible Cooling Energy", "load.zone_cooling", "cooling", "zone", "Office", "sensible", 70, map[int]float64{1: 70}, false, "Monthly"),
			epath070Load("ideal-latent", "Zone Ideal Loads Zone Latent Cooling Energy", "load.zone_cooling", "cooling", "zone", "Office", "latent", 30, map[int]float64{1: 30}, false, "Monthly"),
		}
		epath070AssertCombinedLoad(t, epath070BuildResult(series), "cooling", 100, map[string]float64{
			"ideal-total":    100,
			"ideal-sensible": 70,
			"ideal-latent":   30,
		})
	})

	t.Run("Ideal Loads components are fallback when total is absent", func(t *testing.T) {
		series := []energyExplanationSeries{
			epath070Load("ideal-sensible", "Zone Ideal Loads Zone Sensible Heating Energy", "load.zone_heating", "heating", "zone", "Office", "sensible", 60, map[int]float64{1: 60}, false, "Monthly"),
			epath070Load("ideal-latent", "Zone Ideal Loads Zone Latent Heating Energy", "load.zone_heating", "heating", "zone", "Office", "latent", 15, map[int]float64{1: 15}, false, "Monthly"),
		}
		epath070AssertCombinedLoad(t, epath070BuildResult(series), "heating", 75, map[string]float64{
			"ideal-sensible": 60,
			"ideal-latent":   15,
		})
	})
}

func TestEPATH070AcceptanceEnergyAndActualSourceAuthority(t *testing.T) {
	t.Run("Energy beats a more complete Rate in the same family", func(t *testing.T) {
		energy := epath070Load("actual-energy", "Zone Air System Sensible Cooling Energy", "load.zone_cooling", "cooling", "zone", "Office", "sensible", 8, map[int]float64{1: 8}, false, "Monthly")
		rate := epath070Load("actual-rate", "Zone Air System Sensible Cooling Rate", "load.zone_cooling", "cooling", "zone", "Office", "sensible", 170, map[int]float64{1: 80, 2: 90}, true, "Monthly")
		selected := selectCanonicalEnergyExplanationLoads([]energyExplanationSeries{rate, energy})
		if len(selected) != 1 || selected[0].Monthly[1] != 8 {
			t.Fatalf("Energy-vs-Rate selection = %#v, want Energy values only", selected)
		}
		if _, spliced := selected[0].Monthly[2]; spliced || epath070Contains(selected[0].SourceIDs, "actual-rate") {
			t.Fatalf("Rate source filled a missing Energy month or entered provenance: %#v", selected[0])
		}
	})

	t.Run("actual delivered beats predicted", func(t *testing.T) {
		actual := epath070Load("actual", "Zone Air System Sensible Cooling Rate", "load.zone_cooling", "cooling", "zone", "Office", "sensible", 40, map[int]float64{1: 40}, true, "Monthly")
		predicted := epath070Load("predicted", "Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate", "load.zone_cooling", "cooling", "zone", "Office", "sensible", 90, map[int]float64{1: 90}, true, "Monthly")
		selected := selectCanonicalEnergyExplanationLoads([]energyExplanationSeries{predicted, actual})
		if len(selected) != 1 || selected[0].Monthly[1] != 40 || !epath070Contains(selected[0].SourceIDs, "actual") || epath070Contains(selected[0].SourceIDs, "predicted") {
			t.Fatalf("actual-vs-predicted selection = %#v, want only actual delivered load in main flow", selected)
		}
	})

	t.Run("monthly eligible family beats annual-only higher tier", func(t *testing.T) {
		annualActual := epath070Load("annual-actual", "Zone Air System Sensible Heating Energy", "load.zone_heating", "heating", "zone", "Office", "combined", 100, nil, false, "Annual")
		monthlyIdeal := epath070Load("monthly-ideal", "Zone Ideal Loads Zone Total Heating Energy", "load.zone_heating", "heating", "zone", "Office", "combined", 15, map[int]float64{1: 7, 2: 8}, false, "Monthly")
		selected := selectCanonicalEnergyExplanationLoads([]energyExplanationSeries{annualActual, monthlyIdeal})
		if len(selected) != 1 || selected[0].SourceClass != "zone_ideal_loads" || selected[0].Monthly[1] != 7 || selected[0].Monthly[2] != 8 || epath070Contains(selected[0].SourceIDs, "annual-actual") {
			t.Fatalf("annual-only higher tier displaced monthly-eligible family: %#v", selected)
		}
	})

	t.Run("authoritative family is locked without month splicing", func(t *testing.T) {
		actual := epath070Load("actual", "Zone Air System Sensible Heating Energy", "load.zone_heating", "heating", "zone", "Office", "combined", 9, map[int]float64{1: 4, 2: 5}, false, "Monthly")
		ideal := epath070Load("ideal", "Zone Ideal Loads Zone Total Heating Energy", "load.zone_heating", "heating", "zone", "Office", "combined", 140, map[int]float64{2: 60, 3: 80}, false, "Monthly")
		selected := selectCanonicalEnergyExplanationLoads([]energyExplanationSeries{ideal, actual})
		if len(selected) != 1 || selected[0].SourceClass != "zone_air_system" || selected[0].Monthly[1] != 4 || selected[0].Monthly[2] != 5 {
			t.Fatalf("analysis-period family lock = %#v, want actual zone-air family", selected)
		}
		if _, spliced := selected[0].Monthly[3]; spliced || epath070Contains(selected[0].SourceIDs, "ideal") {
			t.Fatalf("lower family was spliced into an uncovered month: %#v", selected[0])
		}
	})
}

func epath070HierarchyCandidates(service string, normalizedKind bool) []energyExplanationSeries {
	actualName := "Zone Air System Sensible Cooling Energy"
	idealName := "Zone Ideal Loads Zone Total Cooling Energy"
	directName := "Zone Radiant HVAC Cooling Energy"
	systemName := "Cooling Coil Total Cooling Energy"
	plantName := "Plant Loop Cooling Demand Energy"
	if service == "heating" {
		actualName = "Zone Air System Sensible Heating Energy"
		idealName = "Zone Ideal Loads Zone Total Heating Energy"
		directName = "Zone Radiant HVAC Heating Energy"
		systemName = "Heating Coil Heating Energy"
		plantName = "Plant Loop Heating Demand Energy"
	}
	kinds := []string{
		"load.zone_" + service,
		"load.zone_" + service,
		"load.zone_radiant_" + service,
		"load.system_" + service,
		"load.plant_" + service,
	}
	paths := []string{"zone", "zone", "zone", "system", "plant"}
	zones := []string{"Office", "Office", "Office", "", ""}
	if normalizedKind {
		for index := range kinds {
			kinds[index] = "load.zone_" + service
			zones[index] = "Office"
		}
	}
	names := []string{actualName, idealName, directName, systemName, plantName}
	ids := []string{"actual", "ideal-total", "direct-zone", "system-coil", "plant"}
	values := []float64{50, 40, 30, 20, 10}
	out := make([]energyExplanationSeries, 0, len(ids))
	for index := range ids {
		out = append(out, epath070Load(ids[index], names[index], kinds[index], service, paths[index], zones[index], "combined", values[index], map[int]float64{1: values[index]}, false, "Monthly"))
	}
	return out
}

func epath070Load(id string, name string, kind string, service string, pathType string, zone string, component string, total float64, monthly map[int]float64, isRate bool, frequency string) energyExplanationSeries {
	return canonicalEnergyExplanationSeries(energyExplanationSeries{
		Stage:            "load",
		CanonicalKind:    kind,
		Level:            "load",
		Kind:             kind,
		Label:            service + " load",
		Unit:             "kWh",
		ServiceKind:      service,
		PathType:         pathType,
		ZoneName:         zone,
		ThermalComponent: component,
		SourceIDs:        []string{id},
		Total:            total,
		Monthly:          monthly,
		SourceName:       name,
		sourceName:       name,
		sourceKeyValue:   zone,
		sourceFrequency:  frequency,
		sourceIsRate:     isRate,
	})
}

func epath070BuildResult(series []energyExplanationSeries) EnergyExplanationResult {
	sources := make([]EnergyDataSource, 0, len(series))
	seen := map[string]bool{}
	for _, item := range series {
		for _, sourceID := range item.SourceIDs {
			if seen[sourceID] {
				continue
			}
			seen[sourceID] = true
			sources = append(sources, EnergyDataSource{
				ID:                 sourceID,
				SourceType:         "acceptance_fixture",
				KeyValue:           item.sourceKeyValue,
				Name:               item.SourceName,
				Units:              item.Unit,
				ReportingFrequency: item.sourceFrequency,
				ZoneName:           item.ZoneName,
				RawValue:           item.Total,
				EffectiveValue:     item.Total,
			})
		}
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, sources, &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}, energyDriverBuildContext{Enabled: true, Multipliers: unitEnergyMultiplierIndex("Office")})
	return UpgradeEnergyExplanationV1(legacy)
}

func epath070AssertCombinedLoad(t *testing.T, result EnergyExplanationResult, service string, want float64, sourceValues map[string]float64) {
	t.Helper()
	node := epath070NodeByID(result.Nodes, "load."+service+".building")
	if node == nil || node.Value != want || node.ThermalComponent != "combined" {
		t.Fatalf("combined %s load = %#v, want %g kWh with combined component", service, node, want)
	}
	for sourceID, rawValue := range sourceValues {
		if !epath070Contains(node.SourceIDs, sourceID) {
			t.Errorf("combined %s load lost breakdown source %q: %#v", service, sourceID, node.SourceIDs)
		}
		source := epath070SourceByID(result.Sources, sourceID)
		if source == nil || source.RawValue != rawValue {
			t.Errorf("breakdown source %q = %#v, want raw value %g", sourceID, source, rawValue)
		}
	}
	monthly := epath070PeriodByID(result.Periods, "M1")
	if monthly == nil {
		t.Fatal("combined load fixture lost M1 period")
	}
	monthlyNode := epath070NodeByID(monthly.Nodes, "load."+service+".building")
	if monthlyNode == nil || monthlyNode.Value != want {
		t.Errorf("M1 combined %s load = %#v, want %g kWh", service, monthlyNode, want)
	}
}

func epath070NodeByID(nodes []EnergyExplanationNode, id string) *EnergyExplanationNode {
	for index := range nodes {
		if nodes[index].ID == id {
			return &nodes[index]
		}
	}
	return nil
}

func epath070SourceByID(sources []EnergyDataSource, id string) *EnergyDataSource {
	for index := range sources {
		if sources[index].ID == id {
			return &sources[index]
		}
	}
	return nil
}

func epath070PeriodByID(periods []EnergyPeriod, id string) *EnergyPeriod {
	for index := range periods {
		if periods[index].ID == id {
			return &periods[index]
		}
	}
	return nil
}

func epath070Contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
