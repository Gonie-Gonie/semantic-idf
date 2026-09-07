package simulation

import (
	"math"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergySurfaceCategoryIndexUsesGeometryTopologyAndCaseInsensitiveKeys(t *testing.T) {
	report := idf.GeometryReport{
		Surfaces: []idf.GeometrySurface{
			{ID: "surface.wall", Name: "North Wall", Type: "BuildingSurface:Detailed", SurfaceType: "Wall", ZoneName: "Office", SpaceName: "Open Office", OutsideBoundary: "Outdoors", ZoneMultiplier: 2, SurfaceMultiplier: 3},
			{ID: "surface.roof", Name: "Roof One", Type: "BuildingSurface:Detailed", SurfaceType: "Roof", ZoneName: "Office", OutsideBoundary: "OtherSideCoefficients"},
			{ID: "surface.floor", Name: "Ground Slab", Type: "BuildingSurface:Detailed", SurfaceType: "Floor", ZoneName: "Office", OutsideBoundary: "Ground"},
			{ID: "surface.party", Name: "Party Wall", Type: "BuildingSurface:Detailed", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Surface"},
			{ID: "surface.adiabatic", Name: "Adiabatic Wall", Type: "BuildingSurface:Detailed", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Adiabatic"},
		},
		Windows: []idf.GeometryWindow{
			{ID: "window.north", Name: "North Window", Type: "FenestrationSurface:Detailed", SurfaceType: "Window", BaseSurfaceID: "surface.wall", ZoneName: "Office", ZoneMultiplier: 2, SurfaceMultiplier: 4},
		},
		Topology: idf.ThermalTopologyReport{
			Boundaries: []idf.ThermalBoundaryRecord{
				{ID: "boundary.wall", SurfaceID: "surface.wall", SurfaceEntityID: "entity.wall", SurfaceName: "North Wall", BoundaryCondition: "outdoors", RelationKind: "exterior"},
				{ID: "boundary.roof", SurfaceID: "surface.roof", SurfaceEntityID: "entity.roof", SurfaceName: "Roof One", RelationKind: "other_side_coefficients"},
				{ID: "boundary.floor", SurfaceID: "surface.floor", SurfaceEntityID: "entity.floor", SurfaceName: "Ground Slab", RelationKind: "ground"},
				{ID: "boundary.party", SurfaceID: "surface.party", SurfaceEntityID: "entity.party", SurfaceName: "Party Wall", RelationKind: "interzone_explicit_surface"},
				{ID: "boundary.adiabatic", SurfaceID: "surface.adiabatic", SurfaceEntityID: "entity.adiabatic", SurfaceName: "Adiabatic Wall", RelationKind: "adiabatic_explicit"},
			},
			Openings: []idf.ThermalOpeningRecord{
				{ID: "opening.north", WindowID: "window.north", EntityID: "entity.window.north", Name: "North Window", BaseSurfaceID: "surface.wall"},
			},
			Connections: []idf.ThermalConnectionAggregate{
				{ID: "connection.exterior", BoundaryIDs: []string{"boundary.wall", "boundary.roof"}, OpeningIDs: []string{"opening.north"}},
				{ID: "connection.ground", BoundaryIDs: []string{"boundary.floor"}},
				{ID: "connection.interzone", BoundaryIDs: []string{"boundary.party"}},
			},
		},
	}

	index := buildEnergySurfaceCategoryIndex(report)
	tests := map[string]string{
		"NORTH WALL":     energyDriverCategoryExteriorWalls,
		"roof one":       energyDriverCategoryRoofs,
		"GROUND SLAB":    energyDriverCategoryGroundFloors,
		"party wall":     energyDriverCategoryInterzoneSurfaces,
		"adiabatic wall": energyDriverCategoryStorageOther,
		"north window":   energyDriverCategoryWindowsDoors,
	}
	for key, want := range tests {
		got, warning := index.resolve(key)
		if warning != nil || got.Category != want {
			t.Fatalf("resolve(%q) = %#v, warning %#v; want %q", key, got, warning, want)
		}
	}

	wall, _ := index.resolve("  north   wall ")
	if wall.SurfaceID != "surface.wall" || wall.EntityID != "entity.wall" || wall.ZoneName != "Office" || wall.SpaceName != "Open Office" || wall.EffectiveMultiplier != 6 {
		t.Fatalf("wall context = %#v", wall)
	}
	if !stringSliceContains(wall.RelatedEntityIDs, "surface.wall") || !stringSliceContains(wall.RelatedEntityIDs, "connection.exterior") {
		t.Fatalf("wall related entities = %#v", wall.RelatedEntityIDs)
	}
	window, _ := index.resolve("ENTITY.WINDOW.NORTH")
	if window.SpaceName != "Open Office" || window.EffectiveMultiplier != 8 || !stringSliceContains(window.RelatedEntityIDs, "connection.exterior") {
		t.Fatalf("window context = %#v", window)
	}
}

func TestEnergySurfaceCategoryIndexRetainsUnresolvedValueInStorageOther(t *testing.T) {
	index := buildEnergySurfaceCategoryIndex(idf.GeometryReport{})
	category, warning := index.resolve("Unknown Surface")
	if category.Category != energyDriverCategoryStorageOther || category.EffectiveMultiplier != 1 || warning == nil || warning.Code != "energy_driver_surface_unresolved" {
		t.Fatalf("unresolved = %#v, warning %#v", category, warning)
	}
	if !stringSliceContains(category.RelatedEntityIDs, "Unknown Surface") {
		t.Fatalf("unresolved related entities = %#v", category.RelatedEntityIDs)
	}
}

func TestEnergySurfaceDriverCategoryMapsInternalMassAndOtherSideSurfaces(t *testing.T) {
	tests := []struct {
		name            string
		surfaceType     string
		objectType      string
		boundaryKind    string
		outsideBoundary string
		want            string
	}{
		{name: "internal mass", objectType: "InternalMass", want: energyDriverCategoryStorageOther},
		{name: "other-side wall", surfaceType: "Wall", boundaryKind: "other_side_coefficients", want: energyDriverCategoryExteriorWalls},
		{name: "other-side roof", surfaceType: "Roof", outsideBoundary: "OtherSideConditionsModel", want: energyDriverCategoryRoofs},
		{name: "other-side floor", surfaceType: "Floor", outsideBoundary: "OtherSideCoefficients", want: energyDriverCategoryGroundFloors},
		{name: "implicit zone", surfaceType: "Wall", boundaryKind: "interzone_implicit_zone", want: energyDriverCategoryInterzoneSurfaces},
		{name: "implicit space", surfaceType: "Wall", boundaryKind: "interspace_implicit", want: energyDriverCategoryInterzoneSurfaces},
	}
	for _, test := range tests {
		if got := energySurfaceDriverCategory(test.surfaceType, test.objectType, test.boundaryKind, test.outsideBoundary); got != test.want {
			t.Errorf("%s category = %q; want %q", test.name, got, test.want)
		}
	}

	// GeometryReport currently has no InternalMass collection. A SQL key for
	// one therefore follows the explicit unresolved path: it is still retained
	// in storage/other with a warning rather than silently disappearing.
	category, warning := buildEnergySurfaceCategoryIndex(idf.GeometryReport{}).resolve("Mass Partition")
	if category.Category != energyDriverCategoryStorageOther || warning == nil {
		t.Fatalf("InternalMass fallback = %#v, warning %#v", category, warning)
	}
}

func TestEnergySurfaceCategoryAggregationPreservesRawSum(t *testing.T) {
	index := buildEnergySurfaceCategoryIndex(idf.GeometryReport{Surfaces: []idf.GeometrySurface{
		{ID: "wall.a", Name: "Wall A", SurfaceType: "Wall", OutsideBoundary: "Outdoors"},
		{ID: "wall.b", Name: "Wall B", SurfaceType: "Wall", OutsideBoundary: "Outdoors"},
	}})
	raw := map[string]float64{"Wall A": 12.5, "WALL B": -3.25, "Missing": 4}
	byCategory := map[string]float64{}
	total := 0.0
	for key, value := range raw {
		category, _ := index.resolve(key)
		byCategory[category.Category] += value
		total += value
	}
	aggregated := 0.0
	for _, value := range byCategory {
		aggregated += value
	}
	if math.Abs(aggregated-total) > 1e-9 {
		t.Fatalf("aggregated raw sum = %g; want %g", aggregated, total)
	}
}

func TestEnergyDriverSourcePolicyKeepsOnlyZoneAirAdditiveTermsInMainFlow(t *testing.T) {
	tests := []struct {
		name string
		kind string
		role string
	}{
		{name: "Surface Inside Face Convection Heat Gain Energy", kind: "heat.surface_inside_face_convection", role: energyDriverSourceRoleMainFlow},
		{name: "Zone Lights Convective Heating Energy", kind: "heat.lighting", role: energyDriverSourceRoleMainFlow},
		{name: "Zone Infiltration Sensible Heat Loss Energy", kind: "heat.infiltration", role: energyDriverSourceRoleMainFlow},
		{name: "Zone Infiltration Latent Heat Gain Energy", kind: "heat.infiltration", role: energyDriverSourceRoleMainFlow},
		{name: "Zone Ventilation Latent Heat Loss Energy", kind: "heat.ventilation", role: energyDriverSourceRoleMainFlow},
		{name: "Zone Cross Mixing Latent Heat Gain Energy", kind: "heat.mixing", role: energyDriverSourceRoleMainFlow},
		{name: "Zone Windows Total Transmitted Solar Radiation Energy", kind: "heat.solar_window", role: energyDriverSourceRoleContext},
		{name: "Surface Inside Face Conduction Heat Transfer Energy", kind: "heat.surface_conduction", role: energyDriverSourceRoleContext},
		{name: "Zone Lights Radiant Heating Energy", kind: "heat.lighting_radiant", role: energyDriverSourceRoleContext},
		{name: "Zone Air Heat Balance Surface Convection Rate", kind: "heat.surface_convection", role: energyDriverSourceRoleReconciliation},
		{name: "Zone Air Heat Balance Internal Convective Heat Gain Rate", kind: "heat.internal_convective", role: energyDriverSourceRoleReconciliation},
	}
	for _, test := range tests {
		policy := energyDriverSourcePolicyFor(test.name, test.kind)
		if policy.Role != test.role {
			t.Errorf("policy(%q) role = %q; want %q", test.name, policy.Role, test.role)
		}
		if stringsContainsFold(test.name, "Surface Inside Face Convection") && policy.Explanation != energyDriverSurfaceExplanation {
			t.Errorf("surface explanation = %q", policy.Explanation)
		}
	}
}

func TestEnergyDriverContextualBuilderCombinesSensibleLatentCrossMixingWithoutAggregate(t *testing.T) {
	for _, name := range []string{
		"Zone Infiltration Latent Heat Gain Energy",
		"Zone Ventilation Latent Heat Loss Rate",
		"Zone Cross Mixing Sensible Heat Gain Energy",
		"Zone CrossMixing Latent Heat Loss Energy",
	} {
		definition, ok := energyHeatAliasDefinitionForName(name)
		if !ok {
			t.Fatalf("parser alias %q is missing", name)
		}
		if stringsContainsFold(name, "Cross Mixing") || stringsContainsFold(name, "CrossMixing") {
			if definition.Kind != "heat.mixing" {
				t.Fatalf("cross-mixing alias %q kind = %q", name, definition.Kind)
			}
		}
	}
	series := []energyExplanationSeries{
		{Level: "load", Kind: "load.zone_cooling", Label: "Cooling", Unit: "kWh", ServiceKind: "cooling", ZoneName: "Office", Total: 10, SourceIDs: []string{"load"}},
		{Level: "heat", Kind: "heat.mixing", Label: "Mixing", Unit: "kWh", ZoneName: "Office", ThermalComponent: "sensible", Total: 6, SourceIDs: []string{"mixing-sensible"}, sourceName: "Zone Cross Mixing Sensible Heat Gain Energy"},
		{Level: "heat", Kind: "heat.mixing", Label: "Mixing", Unit: "kWh", ZoneName: "Office", ThermalComponent: "latent", Total: 4, SourceIDs: []string{"mixing-latent"}, sourceName: "Zone Cross Mixing Latent Heat Gain Energy"},
		{Level: "heat", Kind: "heat.interzone_air", Label: "Aggregate", Unit: "kWh", ZoneName: "Office", ThermalComponent: "combined", Total: 10, SourceIDs: []string{"mixing-aggregate"}, sourceName: "Zone Air Heat Balance Interzone Air Transfer Rate"},
	}
	sources := []EnergyDataSource{{ID: "load"}, {ID: "mixing-sensible"}, {ID: "mixing-latent"}, {ID: "mixing-aggregate"}}
	legacy := buildEnergyExplanationResultWithDriverContext(energyDriverMonthlyFixtureSeries(series), sources, &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.GeometryReport{}))
	legacy.scope = EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}
	result := UpgradeEnergyExplanationV1(legacy)
	node := energyPathV2NodeByID(result.Nodes, "driver.air.interzone.cooling.office")
	if node == nil || node.Value != 10 || node.ThermalComponent != "combined" ||
		!stringSliceContains(node.SourceIDs, "mixing-sensible") || !stringSliceContains(node.SourceIDs, "mixing-latent") || stringSliceContains(node.SourceIDs, "mixing-aggregate") {
		t.Fatalf("interzone sensible/latent driver = %#v", node)
	}
	aggregate := energyExplanationSourceByID(result.Sources, "mixing-aggregate")
	if aggregate == nil || aggregate.DriverRole != energyDriverSourceRoleReconciliation || aggregate.InspectorSection != energyDriverInspectorSectionContext || aggregate.RawValue != 10 {
		t.Fatalf("interzone aggregate source = %#v", aggregate)
	}
}

func TestEnergyDriverCanonicalTaxonomyIsExactAndOrdered(t *testing.T) {
	want := map[string]energyDriverCategoryDefinition{
		energyDriverCategoryExteriorWalls:         {Category: energyDriverCategoryExteriorWalls, Label: "Wall heat exchange", Order: 0},
		energyDriverCategoryRoofs:                 {Category: energyDriverCategoryRoofs, Label: "Roof heat exchange", Order: 1},
		energyDriverCategoryGroundFloors:          {Category: energyDriverCategoryGroundFloors, Label: "Ground / floor heat exchange", Order: 2},
		energyDriverCategoryWindowsDoors:          {Category: energyDriverCategoryWindowsDoors, Label: "Window / door heat exchange", Order: 3},
		energyDriverCategoryInterzoneSurfaces:     {Category: energyDriverCategoryInterzoneSurfaces, Label: "Interzone surfaces", Order: 4},
		energyDriverCategoryInfiltration:          {Category: energyDriverCategoryInfiltration, Label: "Infiltration", Order: 5},
		energyDriverCategoryMechanicalVentilation: {Category: energyDriverCategoryMechanicalVentilation, Label: "Mechanical ventilation", Order: 6},
		energyDriverCategoryInterzoneAir:          {Category: energyDriverCategoryInterzoneAir, Label: "Interzone air", Order: 7},
		energyDriverCategoryPeople:                {Category: energyDriverCategoryPeople, Label: "People", Order: 8},
		energyDriverCategoryLighting:              {Category: energyDriverCategoryLighting, Label: "Lighting", Order: 9},
		energyDriverCategoryEquipment:             {Category: energyDriverCategoryEquipment, Label: "Equipment", Order: 10},
		energyDriverCategoryInternalOther:         {Category: energyDriverCategoryInternalOther, Label: "Other / storage", Order: 12},
		energyDriverCategoryStorageOther:          {Category: energyDriverCategoryStorageOther, Label: "Other / storage", Order: 12},
	}
	got := map[string]energyDriverCategoryDefinition{}
	for _, definition := range energyDriverCategoryDefinitions {
		if definition.Category == energyDriverDisplayCategoryInterzoneTransfer {
			continue
		}
		got[definition.Category] = definition
	}
	if len(got) != len(want) {
		t.Fatalf("canonical taxonomy count = %d; want %d: %#v", len(got), len(want), got)
	}
	for category, expected := range want {
		actual, ok := got[category]
		if !ok || actual != expected {
			t.Errorf("taxonomy[%q] = %#v, %v; want %#v", category, actual, ok, expected)
		}
	}
}

func stringsContainsFold(value string, fragment string) bool {
	return normalizeEnergyOutputName(value) == normalizeEnergyOutputName(fragment) ||
		len(fragment) > 0 && len(value) >= len(fragment) && containsFold(value, fragment)
}

func containsFold(value string, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if equalFoldASCII(value[index:index+len(fragment)], fragment) {
			return true
		}
	}
	return false
}

func equalFoldASCII(left string, right string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		lc, rc := left[index], right[index]
		if lc >= 'A' && lc <= 'Z' {
			lc += 'a' - 'A'
		}
		if rc >= 'A' && rc <= 'Z' {
			rc += 'a' - 'A'
		}
		if lc != rc {
			return false
		}
	}
	return true
}

func TestEnergyDriverPresentationMergesBuildingInterzoneAtFivePercent(t *testing.T) {
	nodes := []EnergyExplanationNode{
		{Level: "load", ServiceKind: "cooling", ZoneName: "Office", Value: 100},
		{Level: "heat", ServiceKind: "cooling", DriverCategory: energyDriverCategoryInterzoneSurfaces, Value: 2},
		{Level: "heat", ServiceKind: "cooling", DriverCategory: energyDriverCategoryInterzoneAir, Value: 2.99},
		{Level: "heat", ServiceKind: "cooling", DriverCategory: energyDriverCategoryInternalOther, Value: 1},
	}
	below := buildEnergyDriverPresentationPlan(nodes, EnergyExplanationScope{Kind: "building"})
	for _, category := range []string{energyDriverCategoryInterzoneSurfaces, energyDriverCategoryInterzoneAir, energyDriverCategoryInternalOther} {
		got := below.apply(EnergyExplanationNode{Level: "heat", ServiceKind: "cooling", DriverCategory: category})
		if got.DriverCategory != energyDriverCategoryStorageOther {
			t.Fatalf("below threshold %q mapped to %q", category, got.DriverCategory)
		}
	}

	nodes[2].Value = 3
	atThreshold := buildEnergyDriverPresentationPlan(nodes, EnergyExplanationScope{Kind: "building"})
	for _, category := range []string{energyDriverCategoryInterzoneSurfaces, energyDriverCategoryInterzoneAir} {
		got := atThreshold.apply(EnergyExplanationNode{Level: "heat", ServiceKind: "cooling", DriverCategory: category})
		if got.DriverCategory != energyDriverDisplayCategoryInterzoneTransfer || got.Label != "Interzone transfer" {
			t.Fatalf("threshold %q = %#v", category, got)
		}
	}
}

func TestEnergyDriverPresentationZoneScopePreservesCanonicalContributors(t *testing.T) {
	categories := []string{
		energyDriverCategoryExteriorWalls,
		energyDriverCategoryRoofs,
		energyDriverCategoryGroundFloors,
		energyDriverCategoryWindowsDoors,
		energyDriverCategoryInterzoneSurfaces,
		energyDriverCategoryInfiltration,
		energyDriverCategoryMechanicalVentilation,
		energyDriverCategoryInterzoneAir,
		energyDriverCategoryPeople,
		energyDriverCategoryLighting,
		energyDriverCategoryEquipment,
		energyDriverCategoryInternalOther,
		energyDriverCategoryStorageOther,
	}
	nodes := []EnergyExplanationNode{{Level: "load", ServiceKind: "cooling", ZoneName: "Office", Value: 100}}
	for index, category := range categories {
		value := float64(index + 2)
		if category == energyDriverCategoryEquipment {
			value = 1
		}
		nodes = append(nodes, EnergyExplanationNode{Level: "heat", ServiceKind: "cooling", ZoneName: "Office", DriverCategory: category, Value: value})
	}
	plan := buildEnergyDriverPresentationPlan(nodes, EnergyExplanationScope{Kind: "zone", ZoneName: "Office"})
	visible := map[string]bool{}
	for _, node := range nodes[1:] {
		visible[plan.apply(node).DriverCategory] = true
	}
	if len(visible) != len(categories)-1 {
		t.Fatalf("visible category count = %d: %#v", len(visible), visible)
	}
	if !visible[energyDriverCategoryInterzoneSurfaces] || !visible[energyDriverCategoryInterzoneAir] {
		t.Fatalf("zone interzone categories were compacted: %#v", visible)
	}
	if !visible[energyDriverCategoryEquipment] || !visible[energyDriverCategoryStorageOther] {
		t.Fatalf("canonical contributor was lost before presentation grouping: %#v", visible)
	}
}

func TestEnergyDriverContextualBuilderCreatesCanonicalMainFlowAndContextSources(t *testing.T) {
	geometry := idf.GeometryReport{
		Surfaces: []idf.GeometrySurface{
			{ID: "surface.wall", Name: "North Wall", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Outdoors", ZoneMultiplier: 2, SurfaceMultiplier: 3},
			{ID: "surface.roof", Name: "Main Roof", SurfaceType: "Roof", ZoneName: "Office", OutsideBoundary: "Outdoors"},
		},
		Topology: idf.ThermalTopologyReport{
			Boundaries: []idf.ThermalBoundaryRecord{
				{ID: "boundary.wall", SurfaceID: "surface.wall", SurfaceEntityID: "entity.wall", SurfaceName: "North Wall", RelationKind: "exterior"},
				{ID: "boundary.roof", SurfaceID: "surface.roof", SurfaceEntityID: "entity.roof", SurfaceName: "Main Roof", RelationKind: "exterior"},
			},
			Connections: []idf.ThermalConnectionAggregate{
				{ID: "connection.exterior", BoundaryIDs: []string{"boundary.wall", "boundary.roof"}},
			},
		},
	}
	series := []energyExplanationSeries{
		{Level: "load", Kind: "load.zone_cooling", Label: "Cooling load", Unit: "kWh", ServiceKind: "cooling", ZoneName: "Office", Total: 100, SourceIDs: []string{"load"}},
		{Level: "heat", Kind: "heat.surface_inside_face_convection", Label: "Surface exchange", Unit: "kWh", Total: 20, SourceIDs: []string{"wall-main"}, SurfaceScoped: true, sourceKeyValue: "NORTH WALL", sourceName: "Surface Inside Face Convection Heat Gain Energy"},
		{Level: "heat", Kind: "heat.surface_inside_face_convection", Label: "Surface exchange", Unit: "kWh", Total: 10, SourceIDs: []string{"roof-main"}, SurfaceScoped: true, sourceKeyValue: "main roof", sourceName: "Surface Inside Face Convection Heat Gain Energy"},
		{Level: "heat", Kind: "heat.surface_conduction", Label: "Conduction", Unit: "kWh", Total: 30, SourceIDs: []string{"conduction-context"}, SurfaceScoped: true, sourceKeyValue: "North Wall", sourceName: "Surface Inside Face Conduction Heat Transfer Energy"},
		{Level: "heat", Kind: "heat.solar_window", Label: "Solar", Unit: "kWh", ZoneName: "Office", Total: 12, SourceIDs: []string{"solar-context"}, sourceName: "Zone Windows Total Transmitted Solar Radiation Energy"},
		{Level: "heat", Kind: "heat.surface_convection", Label: "Aggregate surface", Unit: "kWh", ZoneName: "Office", Total: 31, SourceIDs: []string{"aggregate-context"}, sourceName: "Zone Air Heat Balance Surface Convection Rate"},
		{Level: "heat", Kind: "heat.people", Label: "People", Unit: "kWh", ZoneName: "Office", Total: 15, SourceIDs: []string{"people-main"}, sourceName: "Zone People Convective Heating Energy"},
		{Level: "heat", Kind: "heat.people", Label: "People total", Unit: "kWh", ZoneName: "Office", Total: 18, SourceIDs: []string{"people-context"}, sourceName: "Zone People Total Heating Energy"},
	}
	sources := make([]EnergyDataSource, 0, 8)
	for _, id := range []string{"load", "wall-main", "roof-main", "conduction-context", "solar-context", "aggregate-context", "people-main", "people-context"} {
		sources = append(sources, EnergyDataSource{ID: id, SourceType: "fixture"})
	}
	legacy := buildEnergyExplanationResultWithDriverContext(energyDriverMonthlyFixtureSeries(series), sources, &PurposeRunPlan{}, newEnergyDriverBuildContext(geometry))
	result := UpgradeEnergyExplanationV1(legacy)

	wall := energyPathV2NodeByID(result.Nodes, "driver.surface.exterior_walls.cooling.building")
	if wall == nil || wall.Label != "Wall heat exchange" || !stringSliceContains(wall.SourceIDs, "wall-main") || !stringSliceContains(wall.RelatedEntityIDs, "connection.exterior") {
		t.Fatalf("wall driver = %#v; legacy nodes = %#v", wall, legacy.Nodes)
	}
	roof := energyPathV2NodeByID(result.Nodes, "driver.surface.roofs.cooling.building")
	if roof == nil || roof.Label != "Roof heat exchange" || !stringSliceContains(roof.SourceIDs, "roof-main") {
		t.Fatalf("roof driver = %#v; nodes = %#v", roof, result.Nodes)
	}
	people := energyPathV2NodeByID(result.Nodes, "driver.internal.people.cooling.building")
	if people == nil || people.RawValue != 15 || people.EffectiveValue != 15 || people.SignedValue != 15 ||
		people.Value != 32.609 || people.AllocatedValue != 32.609 || !people.AllocationApplied ||
		!stringSliceContains(people.SourceIDs, "people-main") || stringSliceContains(people.SourceIDs, "people-context") {
		t.Fatalf("people driver = %#v", people)
	}
	sourceByID := make(map[string]EnergyDataSource, len(result.Sources))
	for _, source := range result.Sources {
		sourceByID[source.ID] = source
	}
	for _, contextual := range []string{"solar-context", "conduction-context", "aggregate-context", "people-context"} {
		for _, node := range result.Nodes {
			if node.Level != "driver" || !stringSliceContains(node.SourceIDs, contextual) {
				continue
			}
			if node.DriverCategory != energyDriverCategoryStorageOther {
				t.Fatalf("context source %q became a direct additive category: %#v", contextual, node)
			}
			derivedProvenance := false
			for _, sourceID := range node.SourceIDs {
				source := sourceByID[sourceID]
				if source.SourceType == "derived_formula" && source.Formula != "" && stringSliceContains(source.InputSourceIDs, contextual) {
					derivedProvenance = true
					break
				}
			}
			if !derivedProvenance {
				t.Fatalf("context source %q reached a driver without an explicit derived balance formula: %#v", contextual, node)
			}
		}
		for _, link := range result.Links {
			if link.Relation != "driver_to_load" || !stringSliceContains(link.SourceIDs, contextual) {
				continue
			}
			from := energyPathV2NodeByID(result.Nodes, link.FromID)
			if from == nil || from.DriverCategory != energyDriverCategoryStorageOther {
				t.Fatalf("context source %q was consumed by a non-balance driver link: %#v", contextual, link)
			}
		}
	}
	wallSource := energyExplanationSourceByID(result.Sources, "wall-main")
	if wallSource == nil || wallSource.DriverRole != energyDriverSourceRoleMainFlow || wallSource.Explanation != energyDriverSurfaceExplanation || wallSource.RawValue != 20 || wallSource.EffectiveValue != 20 || wallSource.AllocatedValue != 43.478 || !wallSource.AllocationApplied || wallSource.EffectiveMultiplier != 1 || wallSource.MultiplierApplication != energyMultiplierUnknown || !stringSliceContains(wallSource.RelatedEntityIDs, "connection.exterior") {
		t.Fatalf("wall source = %#v", wallSource)
	}
	for _, id := range []string{"solar-context", "conduction-context", "aggregate-context", "people-context"} {
		source := energyExplanationSourceByID(result.Sources, id)
		if source == nil || source.RawValue == 0 || source.InspectorSection != energyDriverInspectorSectionContext || source.DriverRole == energyDriverSourceRoleMainFlow {
			t.Fatalf("context source %q = %#v", id, source)
		}
	}
}

func TestEnergyDriverContextualBuilderRetainsUnresolvedSurfaceAsStorageWarningAndSource(t *testing.T) {
	series := []energyExplanationSeries{
		{Level: "load", Kind: "load.zone_cooling", Label: "Cooling load", Unit: "kWh", ServiceKind: "cooling", ZoneName: "Office", Total: 25, SourceIDs: []string{"load"}},
		{Level: "heat", Kind: "heat.surface_inside_face_convection", Label: "Surface exchange", Unit: "kWh", Total: 25, SourceIDs: []string{"unknown-surface"}, SurfaceScoped: true, sourceKeyValue: "Missing Wall", sourceName: "Surface Inside Face Convection Heat Gain Energy"},
	}
	legacy := buildEnergyExplanationResultWithDriverContext(energyDriverMonthlyFixtureSeries(series), []EnergyDataSource{{ID: "load"}, {ID: "unknown-surface"}}, &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.GeometryReport{}))
	result := UpgradeEnergyExplanationV1(legacy)
	node := energyPathV2NodeByID(result.Nodes, "driver.balance.storage_other.cooling.building")
	if node == nil || node.Value != 25 || node.AllocatedValue != 25 || !node.AllocationApplied || node.RawValue != 0 || node.EffectiveValue != 0 ||
		!stringSliceContains(node.SourceIDs, "load") || stringSliceContains(node.SourceIDs, "unknown-surface") {
		t.Fatalf("unresolved node = %#v", node)
	}
	foundWarning := false
	for _, warning := range result.Warnings {
		if warning.Code == "energy_driver_surface_unresolved" {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	source := energyExplanationSourceByID(result.Sources, "unknown-surface")
	if source == nil || source.DriverCategory != energyDriverCategoryStorageOther || source.RawValue != 25 || source.EffectiveValue != 25 ||
		source.AllocatedValue != 0 || !source.AllocationApplied {
		t.Fatalf("unresolved source = %#v", source)
	}
}

func TestEnergyDriverSurfaceConvectionUsesZoneAirSignAndSplitsHeatingCooling(t *testing.T) {
	geometry := idf.GeometryReport{Surfaces: []idf.GeometrySurface{
		{ID: "wall.a", Name: "Wall A", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Outdoors"},
		{ID: "wall.b", Name: "Wall B", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Outdoors"},
	}}
	definition, ok := energyHeatAliasDefinitionForName("Surface Inside Face Convection Heat Gain Energy")
	if !ok {
		t.Fatal("surface convection alias is missing")
	}
	makeSurfaceSeries := func(key string, value float64, sourceID string) energyExplanationSeries {
		definitionCopy := definition
		return energyExplanationSeriesForBuilder(&energyExplanationSeriesBuilder{
			dictionary: energyExplanationDictionary{
				row:  sqlOutputDictionaryRow{keyValue: key, name: "Surface Inside Face Convection Heat Gain Energy", units: "J"},
				heat: &definitionCopy,
			},
			unit:  "kWh",
			total: value,
		}, sourceID)
	}
	series := []energyExplanationSeries{
		{Level: "load", Kind: "load.zone_cooling", Label: "Cooling", Unit: "kWh", ServiceKind: "cooling", ZoneName: "Office", Total: 100, SourceIDs: []string{"cooling-load"}},
		{Level: "load", Kind: "load.zone_heating", Label: "Heating", Unit: "kWh", ServiceKind: "heating", ZoneName: "Office", Total: 100, SourceIDs: []string{"heating-load"}},
		makeSurfaceSeries("Wall A", 10, "wall-a"),
		makeSurfaceSeries("Wall B", -7, "wall-b"),
	}
	sources := []EnergyDataSource{{ID: "cooling-load"}, {ID: "heating-load"}, {ID: "wall-a"}, {ID: "wall-b"}}
	legacy := buildEnergyExplanationResultWithDriverContext(energyDriverMonthlyFixtureSeries(series), sources, &PurposeRunPlan{}, newEnergyDriverBuildContext(geometry))
	result := UpgradeEnergyExplanationV1(legacy)
	heating := energyPathV2NodeByID(result.Nodes, "driver.surface.exterior_walls.heating.building")
	cooling := energyPathV2NodeByID(result.Nodes, "driver.surface.exterior_walls.cooling.building")
	if heating == nil || heating.RawValue != 10 || heating.EffectiveValue != 10 || heating.SignedValue != -10 || heating.Value != 100 || heating.AllocatedValue != 100 || !heating.AllocationApplied || !stringSliceContains(heating.SourceIDs, "wall-a") || stringSliceContains(heating.SourceIDs, "wall-b") {
		t.Fatalf("surface heating driver = %#v", heating)
	}
	if cooling == nil || cooling.RawValue != 7 || cooling.EffectiveValue != 7 || cooling.SignedValue != 7 || cooling.Value != 100 || cooling.AllocatedValue != 100 || !cooling.AllocationApplied || !stringSliceContains(cooling.SourceIDs, "wall-b") || stringSliceContains(cooling.SourceIDs, "wall-a") {
		t.Fatalf("surface cooling driver = %#v", cooling)
	}
	heatingLink := energyPathV2LinkByIDs(result.Links, heating.ID, "load.heating.building")
	coolingLink := energyPathV2LinkByIDs(result.Links, cooling.ID, "load.cooling.building")
	if heatingLink == nil || heatingLink.FromValue != 100 || heatingLink.ToValue != 100 || heatingLink.Basis != "heat_balance_share" || !stringSliceContains(heatingLink.SourceIDs, "wall-a") ||
		coolingLink == nil || coolingLink.FromValue != 100 || coolingLink.ToValue != 100 || coolingLink.Basis != "heat_balance_share" || !stringSliceContains(coolingLink.SourceIDs, "wall-b") {
		t.Fatalf("surface driver links = %#v", result.Links)
	}
	wallA := energyExplanationSourceByID(result.Sources, "wall-a")
	wallB := energyExplanationSourceByID(result.Sources, "wall-b")
	if wallA == nil || wallA.RawValue != 10 || wallB == nil || wallB.RawValue != -7 {
		t.Fatalf("surface source provenance: wall-a=%#v wall-b=%#v", wallA, wallB)
	}
	if got, want := roundedEnergyNumber(heating.SignedValue+cooling.SignedValue), roundedEnergyNumber(-(wallA.RawValue + wallB.RawValue)); got != want {
		t.Fatalf("signed driver sum = %v, want inverse raw source sum %v", got, want)
	}
}

func energyDriverMonthlyFixtureSeries(input []energyExplanationSeries) []energyExplanationSeries {
	out := append([]energyExplanationSeries(nil), input...)
	for index := range out {
		out[index].Monthly = map[int]float64{1: out[index].Total}
		out[index].sourceFrequency = "Monthly"
	}
	return out
}

func TestUpgradeEnergyExplanationV1RemovesAggregateWhenDetailedMainFlowExists(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema: energyExplanationV1Schema,
		Nodes: []EnergyExplanationNode{
			{ID: "load.cooling.office", Level: "load", Kind: "load.zone_cooling", Value: 40, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling"},
			{ID: "heat.surface.detail.office", Level: "heat", Kind: "heat.surface_inside_face_convection", Label: "Surface-to-zone-air heat exchange", Value: 40, SignedValue: 40, DisplayValue: 40, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", DriverCategory: energyDriverCategoryExteriorWalls, SourceIDs: []string{"detail"}},
			{ID: "heat.surface.aggregate.office", Level: "heat", Kind: "heat.surface_convection", Label: "Aggregate surface convection", Value: 40, SignedValue: 40, DisplayValue: 40, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"aggregate"}},
			{ID: "heat.solar.office", Level: "heat", Kind: "heat.solar_window", Label: "Window transmitted solar", Value: 20, SignedValue: 20, DisplayValue: 20, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"solar"}},
		},
		Edges: []EnergyExplanationEdge{
			{ID: "detail-link", FromID: "load.cooling.office", ToID: "heat.surface.detail.office", Relation: "heat_driver", Value: 40},
			{ID: "aggregate-link", FromID: "load.cooling.office", ToID: "heat.surface.aggregate.office", Relation: "heat_driver", Value: 40},
			{ID: "solar-link", FromID: "load.cooling.office", ToID: "heat.solar.office", Relation: "heat_driver", Value: 20},
		},
		Sources: []EnergyDataSource{
			{ID: "detail", Name: "Surface Inside Face Convection Heat Gain Energy"},
			{ID: "aggregate", Name: "Zone Air Heat Balance Surface Convection Rate"},
			{ID: "solar", Name: "Zone Windows Total Transmitted Solar Radiation Energy"},
		},
	}
	result := UpgradeEnergyExplanationV1(legacy)
	driver := energyPathV2NodeByID(result.Nodes, "driver.surface.exterior_walls.cooling.building")
	if driver == nil || driver.Value != 40 || !stringSliceContains(driver.SourceIDs, "detail") || stringSliceContains(driver.SourceIDs, "aggregate") || stringSliceContains(driver.SourceIDs, "solar") {
		t.Fatalf("main driver = %#v", driver)
	}
	if len(result.Links) != 1 || result.Links[0].Relation != "driver_to_load" || result.Links[0].FromID != driver.ID {
		t.Fatalf("links = %#v", result.Links)
	}
	for _, id := range []string{"aggregate", "solar"} {
		source := energyExplanationSourceByID(result.Sources, id)
		if source == nil || source.InspectorSection != energyDriverInspectorSectionContext || source.RawValue == 0 {
			t.Fatalf("excluded source %q = %#v", id, source)
		}
	}
}

func TestUpgradeEnergyExplanationV1EnforcesInterzoneThresholdAndZoneMaximum(t *testing.T) {
	buildingFixture := func(interzoneAir float64) EnergyExplanationV1 {
		return EnergyExplanationV1{
			Schema: energyExplanationV1Schema,
			Nodes: []EnergyExplanationNode{
				{ID: "load", Level: "load", Value: 100, ServiceKind: "cooling", ZoneName: "Office"},
				{ID: "surface", Level: "heat", Value: 2, SignedValue: 2, DisplayValue: 2, ServiceKind: "cooling", ZoneName: "Office", DriverCategory: energyDriverCategoryInterzoneSurfaces, SourceIDs: []string{"surface-source"}, RelatedEntityIDs: []string{"surface-entity"}},
				{ID: "air", Level: "heat", Value: interzoneAir, SignedValue: interzoneAir, DisplayValue: interzoneAir, ServiceKind: "cooling", ZoneName: "Office", DriverCategory: energyDriverCategoryInterzoneAir, SourceIDs: []string{"air-source"}, RelatedEntityIDs: []string{"air-entity"}},
			},
			Edges: []EnergyExplanationEdge{
				{ID: "surface-link", FromID: "load", ToID: "surface", Value: 2, Relation: "heat_driver", SourceIDs: []string{"surface-source"}},
				{ID: "air-link", FromID: "load", ToID: "air", Value: interzoneAir, Relation: "heat_driver", SourceIDs: []string{"air-source"}},
			},
		}
	}
	below := UpgradeEnergyExplanationV1(buildingFixture(2.99))
	belowStorage := energyPathV2NodeByID(below.Nodes, "driver.balance.storage_other.cooling.building")
	if energyPathV2NodeByID(below.Nodes, "driver.interzone.transfer.cooling.building") != nil || belowStorage == nil || belowStorage.Value != 4.99 || len(below.Links) != 1 || below.Links[0].FromID != belowStorage.ID || below.Links[0].FromValue != 4.99 {
		t.Fatalf("below threshold nodes = %#v", below.Nodes)
	}
	atThreshold := UpgradeEnergyExplanationV1(buildingFixture(3))
	transfer := energyPathV2NodeByID(atThreshold.Nodes, "driver.interzone.transfer.cooling.building")
	if transfer == nil || transfer.Value != 5 || transfer.Label != "Interzone transfer" ||
		!stringSliceContains(transfer.SourceIDs, "surface-source") || !stringSliceContains(transfer.SourceIDs, "air-source") ||
		!stringSliceContains(transfer.RelatedEntityIDs, "surface-entity") || !stringSliceContains(transfer.RelatedEntityIDs, "air-entity") {
		t.Fatalf("threshold transfer = %#v", transfer)
	}
	if len(atThreshold.Links) != 1 || atThreshold.Links[0].FromID != transfer.ID || atThreshold.Links[0].FromValue != 5 ||
		!stringSliceContains(atThreshold.Links[0].SourceIDs, "surface-source") || !stringSliceContains(atThreshold.Links[0].SourceIDs, "air-source") {
		t.Fatalf("threshold links = %#v", atThreshold.Links)
	}
	nodeIDs := map[string]bool{}
	for _, node := range atThreshold.Nodes {
		nodeIDs[node.ID] = true
	}
	for _, link := range atThreshold.Links {
		if !nodeIDs[link.FromID] || !nodeIDs[link.ToID] {
			t.Fatalf("link endpoint missing from node set: %#v", link)
		}
	}

	zone := buildingFixture(3)
	zone.scope = EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}
	zone.Nodes = zone.Nodes[:1]
	allCategories := []string{
		energyDriverCategoryExteriorWalls, energyDriverCategoryRoofs, energyDriverCategoryGroundFloors,
		energyDriverCategoryWindowsDoors, energyDriverCategoryInterzoneSurfaces, energyDriverCategoryInfiltration,
		energyDriverCategoryMechanicalVentilation, energyDriverCategoryInterzoneAir, energyDriverCategoryPeople,
		energyDriverCategoryLighting, energyDriverCategoryEquipment, energyDriverCategoryInternalOther,
		energyDriverCategoryStorageOther,
	}
	for index, category := range allCategories {
		value := float64(index + 2)
		if category == energyDriverCategoryEquipment {
			value = 1
		}
		zone.Nodes = append(zone.Nodes, EnergyExplanationNode{ID: category, Level: "heat", Value: value, SignedValue: value, DisplayValue: value, ServiceKind: "cooling", ZoneName: "Office", DriverCategory: category})
	}
	zoneResult := UpgradeEnergyExplanationV1(zone)
	driverCount := 0
	for _, node := range zoneResult.Nodes {
		if node.Level == "driver" {
			driverCount++
		}
	}
	if driverCount != len(allCategories)-1 || energyPathV2NodeByID(zoneResult.Nodes, "driver.surface.interzone.cooling.office") == nil || energyPathV2NodeByID(zoneResult.Nodes, "driver.air.interzone.cooling.office") == nil || energyPathV2NodeByID(zoneResult.Nodes, "driver.internal.equipment.cooling.office") == nil {
		t.Fatalf("zone drivers (%d) = %#v", driverCount, zoneResult.Nodes)
	}
}
