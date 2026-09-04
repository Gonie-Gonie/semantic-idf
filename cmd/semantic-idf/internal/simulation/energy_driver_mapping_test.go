package simulation

import (
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyDriverSurfaceDoesNotDoubleCountFenestrationOrWindowTotals(t *testing.T) {
	report := idf.GeometryReport{
		Surfaces: []idf.GeometrySurface{{ID: "wall.1", Name: "Wall 1", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Outdoors"}},
		Windows:  []idf.GeometryWindow{{ID: "window.1", Name: "Window 1", SurfaceType: "Window", ZoneName: "Office", BaseSurfaceID: "wall.1"}},
	}
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "heating", 10, "load"),
		reviewEnergyHeatSeries(t, "Wall 1", "Surface Inside Face Convection Heat Gain Energy", 8, "wall"),
		reviewEnergyHeatSeries(t, "Window 1", "Surface Inside Face Convection Heat Gain Energy", 2, "window"),
		reviewEnergyHeatSeries(t, "Office", "Zone Windows Total Heat Gain Energy", 50, "window-total"),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(report))
	result := UpgradeEnergyExplanationV1(legacy)
	wall := energyPathV2NodeByID(result.Nodes, "driver.surface.exterior_walls.heating.building")
	window := energyPathV2NodeByID(result.Nodes, "driver.surface.windows_doors.heating.building")
	if wall == nil || wall.Value != 8 || window == nil || window.Value != 2 {
		t.Fatalf("surface/fenestration source totals = wall %#v, window %#v; nodes=%#v", wall, window, result.Nodes)
	}
	for _, node := range result.Nodes {
		if node.Level == "driver" && stringSliceContains(node.SourceIDs, "window-total") {
			t.Fatalf("aggregate window total leaked into additive flow: %#v", node)
		}
	}
	source := energyExplanationSourceByID(result.Sources, "window-total")
	if source == nil || source.DriverRole != energyDriverSourceRoleContext {
		t.Fatalf("window total context provenance = %#v", source)
	}
}

func TestEnergyDriverOutdoorAirMismatchIsNamedStorageComponent(t *testing.T) {
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 55, "load"),
		reviewEnergyHeatSeries(t, "Office", "Zone Infiltration Sensible Heat Gain Energy", 30, "infiltration"),
		reviewEnergyHeatSeries(t, "Office", "Zone Ventilation Sensible Heat Gain Energy", 20, "ventilation"),
		reviewEnergyHeatSeries(t, "Office", "Zone Combined Outdoor Air Sensible Heat Gain Energy", 55, "combined"),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.GeometryReport{}))
	result := UpgradeEnergyExplanationV1(legacy)
	storage := energyPathV2NodeByID(result.Nodes, "driver.balance.storage_other.cooling.building")
	if storage == nil || storage.Value != 5 {
		t.Fatalf("outdoor-air reconciliation storage = %#v", storage)
	}
	derived := reviewEnergySourceWithFormula(result.Sources, "signed outdoor-air aggregate - signed infiltration and mechanical-ventilation drivers")
	if derived == nil || derived.DriverComponent != "outdoor_air.reconciliation_difference.sensible" || derived.RawValue != 5 ||
		!stringSliceContains(derived.InputSourceIDs, "combined") || !stringSliceContains(derived.InputSourceIDs, "infiltration") || !stringSliceContains(derived.InputSourceIDs, "ventilation") {
		t.Fatalf("outdoor-air reconciliation provenance = %#v", derived)
	}
	combined := energyExplanationSourceByID(result.Sources, "combined")
	if combined == nil || combined.DriverRole != energyDriverSourceRoleReconciliation {
		t.Fatalf("outdoor-air aggregate role = %#v", combined)
	}
	if !stringSliceContains(storage.SourceIDs, derived.ID) || !stringSliceContains(storage.SourceIDs, "combined") {
		t.Fatalf("storage node did not retain derived and input provenance: %#v", storage)
	}
	for _, nodeID := range []string{"driver.air.infiltration.cooling.building", "driver.air.mechanical_ventilation.cooling.building"} {
		node := energyPathV2NodeByID(result.Nodes, nodeID)
		if node == nil || stringSliceContains(node.SourceIDs, "combined") {
			t.Fatalf("outdoor-air aggregate was consumed by direct driver %q: %#v", nodeID, node)
		}
	}
	for _, link := range result.Links {
		if link.Relation == "driver_to_load" && stringSliceContains(link.SourceIDs, "combined") && link.FromID != storage.ID {
			t.Fatalf("outdoor-air aggregate was consumed outside its named derived balance link: %#v", link)
		}
	}
}

func TestEnergyDriverUnmappedBalanceUsesOnlyActualCanonicalZoneLoad(t *testing.T) {
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 100, "actual-load"),
		canonicalEnergyExplanationSeries(energyExplanationSeries{Level: "load", Kind: "load.zone_radiant_cooling", ServiceKind: "cooling", PathType: "zone", ZoneName: "Office", Total: 40, Monthly: map[int]float64{1: 40}, SourceIDs: []string{"radiant-load"}}),
		canonicalEnergyExplanationSeries(energyExplanationSeries{Level: "load", Kind: "load.system_cooling", ServiceKind: "cooling", PathType: "system", ZoneName: "Office", Total: 300, Monthly: map[int]float64{1: 300}, SourceIDs: []string{"system-load"}}),
		reviewEnergyHeatSeries(t, "Office", "Zone Infiltration Sensible Heat Gain Energy", 30, "infiltration"),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.GeometryReport{}))
	result := UpgradeEnergyExplanationV1(legacy)
	derived := reviewEnergySourceWithFormula(result.Sources, "signed zone cooling load - signed zone heating load - signed mapped driver contributions")
	if derived == nil || derived.RawValue != 70 || !stringSliceContains(derived.InputSourceIDs, "actual-load") || stringSliceContains(derived.InputSourceIDs, "radiant-load") || stringSliceContains(derived.InputSourceIDs, "system-load") {
		t.Fatalf("actual canonical zone-load balance provenance = %#v", derived)
	}
}

func TestEnergyDriverPairwiseInterzoneBuildingNetUsesThresholdAndZoneProvenance(t *testing.T) {
	for _, test := range []struct {
		name            string
		officeGain      float64
		labLoss         float64
		wantPairNet     float64
		wantInterzone   float64
		wantDisplayNode string
	}{
		{name: "small net", officeGain: 60, labLoss: 56, wantPairNet: 4},
		{name: "material net", officeGain: 60, labLoss: 50, wantInterzone: 10, wantDisplayNode: "driver.interzone.transfer.cooling.building"},
	} {
		t.Run(test.name, func(t *testing.T) {
			report := idf.GeometryReport{Topology: idf.ThermalTopologyReport{
				Nodes: []idf.ThermalTopologyNode{
					{ID: "zone.office", EntityID: "entity.office", ZoneName: "Office"},
					{ID: "zone.lab", EntityID: "entity.lab", ZoneName: "Lab"},
				},
				AirCouplings: []idf.ThermalAirCoupling{
					{ID: "coupling.office", EntityID: "mixing.office", ObjectType: "ZoneMixing", ObjectName: "Office from Lab", FromNodeID: "zone.lab", ToNodeID: "zone.office", Direction: "directed"},
					{ID: "coupling.lab", EntityID: "mixing.lab", ObjectType: "ZoneMixing", ObjectName: "Lab from Office", FromNodeID: "zone.office", ToNodeID: "zone.lab", Direction: "directed"},
				},
			}}
			series := []energyExplanationSeries{
				reviewEnergyLoadSeries("Office", "cooling", test.officeGain, "office-load"),
				reviewEnergyLoadSeries("Lab", "heating", test.labLoss, "lab-load"),
				energyDriverPairwiseFixtureSeries("Office from Lab", "positive", test.officeGain, "office-gain"),
				energyDriverPairwiseFixtureSeries("Lab from Office", "negative", test.labLoss, "lab-loss"),
			}
			legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(report))
			result := UpgradeEnergyExplanationV1(legacy)

			office := energyPathV2NodeByID(result.ZoneResults[0].Nodes, "driver.air.interzone.cooling.office")
			var lab *EnergyExplanationNode
			for _, zone := range result.ZoneResults {
				if stringsEqualFold(zone.Scope.ZoneName, "Office") {
					office = energyPathV2NodeByID(zone.Nodes, "driver.air.interzone.cooling.office")
				}
				if stringsEqualFold(zone.Scope.ZoneName, "Lab") {
					lab = energyPathV2NodeByID(zone.Nodes, "driver.air.interzone.heating.lab")
				}
			}
			if office == nil || office.Value != test.officeGain || !stringSliceContains(office.RelatedEntityIDs, "coupling.office") || lab == nil || lab.Value != test.labLoss || !stringSliceContains(lab.RelatedEntityIDs, "coupling.lab") {
				t.Fatalf("pairwise Zone nodes = office %#v, lab %#v; zones=%#v", office, lab, result.ZoneResults)
			}
			if test.wantPairNet > 0 {
				coolingStorage := energyPathV2NodeByID(result.Nodes, "driver.balance.storage_other.cooling.building")
				heatingStorage := energyPathV2NodeByID(result.Nodes, "driver.balance.storage_other.heating.building")
				if coolingStorage == nil || coolingStorage.Value != test.officeGain || heatingStorage == nil || heatingStorage.Value != test.labLoss {
					t.Fatalf("suppressed pair did not preserve closed directional loads in Other / storage: cooling %#v, heating %#v", coolingStorage, heatingStorage)
				}
				if energyPathV2NodeByID(result.Nodes, "driver.interzone.transfer.cooling.building") != nil {
					t.Fatalf("small pair net must not be a prominent Building interzone node: %#v", result.Nodes)
				}
			} else {
				interzone := energyPathV2NodeByID(result.Nodes, test.wantDisplayNode)
				if interzone == nil || interzone.Value != test.wantInterzone {
					t.Fatalf("material Building pair net = %#v", interzone)
				}
			}
			derived := reviewEnergySourceWithFormula(result.Sources, "signed pairwise interzone receiving-zone contributions summed at Building scope")
			if derived == nil || derived.RawValue != test.officeGain-test.labLoss || !stringSliceContains(derived.InputSourceIDs, "office-gain") || !stringSliceContains(derived.InputSourceIDs, "lab-loss") || !stringSliceContains(derived.RelatedEntityIDs, "coupling.office") || !stringSliceContains(derived.RelatedEntityIDs, "coupling.lab") {
				t.Fatalf("pairwise Building residual provenance = %#v", derived)
			}
		})
	}
}

func TestEnergyDriverGenericMixingExactTopologyKeyIsStillZoneAggregate(t *testing.T) {
	report := idf.GeometryReport{Topology: idf.ThermalTopologyReport{
		Nodes: []idf.ThermalTopologyNode{
			{ID: "zone.office", ZoneName: "Office"},
			{ID: "zone.lab", ZoneName: "Lab"},
		},
		AirCouplings: []idf.ThermalAirCoupling{{ID: "coupling.office", ObjectType: "ZoneMixing", ObjectName: "Office from Lab", FromNodeID: "zone.lab", ToNodeID: "zone.office", Direction: "directed"}},
	}}
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 10, "load"),
		reviewEnergyHeatSeries(t, "Office from Lab", "Zone Mixing Sensible Heat Gain Energy", 10, "mixing"),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(report))
	result := UpgradeEnergyExplanationV1(legacy)
	if energyPathV2NodeByID(result.Nodes, "driver.air.interzone.cooling.building") != nil || energyPathV2NodeByID(result.Nodes, "driver.interzone.transfer.cooling.building") != nil {
		t.Fatalf("generic Zone Mixing became pairwise from an exact topology-name match: %#v", result.Nodes)
	}
	if len(result.ZoneResults) != 1 {
		t.Fatalf("generic exact-key zone results = %#v", result.ZoneResults)
	}
	node := energyPathV2NodeByID(result.ZoneResults[0].Nodes, "driver.air.interzone.cooling.office")
	if node == nil || !stringSliceContains(node.RelatedEntityIDs, "coupling.office") {
		t.Fatalf("generic exact-key Zone aggregate/provenance = %#v", node)
	}
}

func TestEnergyDriverPairwiseBuildingResidualDoesNotSuppressZoneClosure(t *testing.T) {
	report := idf.GeometryReport{Topology: idf.ThermalTopologyReport{
		Nodes: []idf.ThermalTopologyNode{
			{ID: "zone.office", ZoneName: "Office"},
			{ID: "zone.lab", ZoneName: "Lab"},
		},
		AirCouplings: []idf.ThermalAirCoupling{
			{ID: "coupling.office", ObjectType: "ZoneMixing", ObjectName: "Office from Lab", FromNodeID: "zone.lab", ToNodeID: "zone.office", Direction: "directed"},
			{ID: "coupling.lab", ObjectType: "ZoneMixing", ObjectName: "Lab from Office", FromNodeID: "zone.office", ToNodeID: "zone.lab", Direction: "directed"},
		},
	}}
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 65, "office-load"),
		reviewEnergyLoadSeries("Lab", "heating", 50, "lab-load"),
		energyDriverPairwiseFixtureSeries("Office from Lab", "positive", 60, "office-gain"),
		energyDriverPairwiseFixtureSeries("Lab from Office", "negative", 50, "lab-loss"),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(report))
	result := UpgradeEnergyExplanationV1(legacy)
	source := energyExplanationSourceByID(result.Sources, "derived-driver-unmapped_zone_balance-office")
	if source == nil || source.RawValue != 5 || !stringSliceContains(source.InputSourceIDs, "office-load") || !stringSliceContains(source.InputSourceIDs, "office-gain") {
		t.Fatalf("pairwise Building residual suppressed Office zone closure: %#v", source)
	}
}

func TestEnergyPathRequestsSystemOutdoorAirAndHeatExchangerContextByObject(t *testing.T) {
	doc := parsePurposePlanFixture(t, `
Version, 25.1;
Building, Example;
Zone, Office;
AirLoopHVAC, Main Air Loop;
HeatExchanger:AirToAir:SensibleAndLatent, Main Heat Recovery;
`)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	for _, name := range []string{
		"Air System Outdoor Air Sensible Heating Energy",
		"Air System Outdoor Air Latent Cooling Rate",
	} {
		if output := findPurposeOutput(plan, "Output:Variable", "Main Air Loop", name); output == nil || output.Reason != "Basic Energy Path" {
			t.Errorf("missing Air System outdoor-air context %q: %#v", name, output)
		}
	}
	for _, name := range []string{
		"Heat Exchanger Sensible Heating Energy",
		"Heat Exchanger Latent Cooling Rate",
	} {
		if output := findPurposeOutput(plan, "Output:Variable", "Main Heat Recovery", name); output == nil || output.Reason != "Basic Energy Path" {
			t.Errorf("missing heat-recovery context %q: %#v", name, output)
		}
	}
}

func stringsEqualFold(left string, right string) bool {
	return normalizeEnergyOutputName(left) == normalizeEnergyOutputName(right)
}

func energyDriverPairwiseFixtureSeries(key string, sign string, total float64, sourceID string) energyExplanationSeries {
	multiplier := 1.0
	if sign == "negative" {
		multiplier = -1
	}
	return canonicalEnergyExplanationSeries(energyExplanationSeries{
		Level:              "heat",
		Kind:               "heat.interzone_pair",
		Label:              "Pairwise interzone heat",
		Unit:               "kWh",
		ThermalComponent:   "sensible",
		HeatSign:           sign,
		SourceIDs:          []string{sourceID},
		Total:              total,
		Monthly:            map[int]float64{1: total},
		sourceKeyValue:     key,
		sourceName:         "Pairwise Interzone Heat Transfer Energy",
		sourceFrequency:    "Monthly",
		heatSignMultiplier: multiplier,
	})
}
