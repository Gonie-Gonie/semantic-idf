package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// EPATH-060 requires source preference to be resolved for every physical
// surface and every period tier before category aggregation. In particular,
// an Hourly row for Wall A must not be added to Wall A's selected Monthly row,
// while Wall B must still be allowed to fall back to its Hourly-only row.
func TestEPATH060ReviewSurfacePeriodTierExactlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
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
			(1, 'Wall A', 'Surface Inside Face Convection Heat Gain Energy', 'J', 0, 'Monthly', 'Surface'),
			(2, 'Wall A', 'Surface Inside Face Convection Heat Gain Energy', 'J', 0, 'Hourly', 'Surface'),
			(3, 'Wall B', 'Surface Inside Face Convection Heat Gain Energy', 'J', 0, 'Hourly', 'Surface')`,
		`INSERT INTO "Time" VALUES
			(1, 1, 31, 24, 0),
			(2, 1, 1, 1, 0),
			(3, 1, 1, 2, 0)`,
		`INSERT INTO ReportData VALUES
			(1, 1, 1, 36000000.0),
			(2, 2, 2, 144000000.0),
			(3, 3, 2, 216000000.0),
			(4, 2, 3, 10800000.0),
			(5, 3, 3, 14400000.0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			t.Fatalf("surface period-tier SQL fixture failed: %v\n%s", err, statement)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	report := idf.GeometryReport{Surfaces: []idf.GeometrySurface{
		{ID: "wall.a", Name: "Wall A", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Outdoors"},
		{ID: "wall.b", Name: "Wall B", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Outdoors"},
	}}
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(
		path,
		&PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath},
		newEnergyDriverBuildContext(report),
	)
	if err != nil {
		t.Fatal(err)
	}

	var aggregate *energyExplanationSeries
	for index := range parsed.Series {
		item := &parsed.Series[index]
		if item.parseCategoryAggregate && item.DriverCategory == energyDriverCategoryExteriorWalls {
			if aggregate != nil {
				t.Fatalf("multiple exterior-wall category aggregates: %#v and %#v", aggregate, item)
			}
			aggregate = item
		}
	}
	if aggregate == nil {
		t.Fatalf("missing exterior-wall category aggregate in %#v", parsed.Series)
	}
	if aggregate.Total != 17 || aggregate.Monthly[1] != 17 {
		t.Fatalf("annual/monthly surface aggregation = total %g, M1 %g; want Wall A Monthly 10 + Wall B Hourly fallback 7 exactly once", aggregate.Total, aggregate.Monthly[1])
	}
	hourlyTotal := 0.0
	for _, value := range aggregate.Hourly {
		hourlyTotal += value
	}
	if math.Abs(hourlyTotal-107) > 1e-9 {
		t.Fatalf("hourly surface aggregation = %g; want Wall A Hourly 100 + Wall B Hourly 7", hourlyTotal)
	}
	for _, sourceID := range []string{"sql-rdd-1", "sql-rdd-3"} {
		if !stringSliceContains(aggregate.AnnualSourceIDs, sourceID) || !stringSliceContains(aggregate.MonthlySourceIDs, sourceID) {
			t.Fatalf("annual/monthly provenance missing %q: annual=%#v monthly=%#v", sourceID, aggregate.AnnualSourceIDs, aggregate.MonthlySourceIDs)
		}
	}
	if stringSliceContains(aggregate.AnnualSourceIDs, "sql-rdd-2") || stringSliceContains(aggregate.MonthlySourceIDs, "sql-rdd-2") {
		t.Fatalf("unselected Wall A Hourly source leaked into annual/monthly provenance: annual=%#v monthly=%#v", aggregate.AnnualSourceIDs, aggregate.MonthlySourceIDs)
	}
	for _, sourceID := range []string{"sql-rdd-2", "sql-rdd-3"} {
		if !stringSliceContains(aggregate.HourlySourceIDs, sourceID) {
			t.Fatalf("hourly provenance missing %q: %#v", sourceID, aggregate.HourlySourceIDs)
		}
	}
}

// People sensible heat contains both convective and radiant heat. It must not
// win source preference over the narrower convective quantity used by the
// additive zone-air balance merely because it appeared first in the RDD.
func TestEPATH061ReviewPeopleConvectiveBeatsSensibleContext(t *testing.T) {
	makeHeat := func(name string, total float64, sourceID string) energyExplanationSeries {
		t.Helper()
		return reviewEnergyHeatSeries(t, "Office", name, total, sourceID)
	}
	series := []energyExplanationSeries{
		{Level: "load", Kind: "load.zone_cooling", Label: "Cooling", Unit: "kWh", ServiceKind: "cooling", ZoneName: "Office", Total: 100, Monthly: map[int]float64{1: 100}, SourceIDs: []string{"load"}, sourceFrequency: "Monthly"},
		makeHeat("Zone People Sensible Heating Energy", 90, "people-sensible"),
		makeHeat("Zone People Convective Heating Energy", 10, "people-convective"),
		makeHeat("Zone People Latent Gain Energy", 3, "people-latent"),
	}
	sources := []EnergyDataSource{
		{ID: "load", Name: "Zone Air System Sensible Cooling Energy"},
		{ID: "people-sensible", Name: "Zone People Sensible Heating Energy"},
		{ID: "people-convective", Name: "Zone People Convective Heating Energy"},
		{ID: "people-latent", Name: "Zone People Latent Gain Energy"},
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, sources, &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.GeometryReport{}))
	result := UpgradeEnergyExplanationV1(legacy)
	people := energyPathV2NodeByID(result.Nodes, "driver.internal.people.cooling.building")
	if people == nil {
		t.Fatalf("missing people driver: nodes=%#v sources=%#v", result.Nodes, result.Sources)
	}
	if people.Value != 13 || !stringSliceContains(people.SourceIDs, "people-convective") || !stringSliceContains(people.SourceIDs, "people-latent") || stringSliceContains(people.SourceIDs, "people-sensible") {
		t.Fatalf("people driver = %#v; want convective 10 + latent 3 only", people)
	}
	sensible := energyExplanationSourceByID(result.Sources, "people-sensible")
	if sensible == nil || sensible.DriverRole != energyDriverSourceRoleContext || sensible.InspectorSection != energyDriverInspectorSectionContext {
		t.Fatalf("people sensible source must remain context: %#v", sensible)
	}
}

func TestEPATH061ReviewInternalFamiliesExcludeTotalsAndExposeAggregateGap(t *testing.T) {
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 30, "load"),
		reviewEnergyHeatSeries(t, "Office", "Zone People Convective Heating Energy", 10, "people-convective"),
		reviewEnergyHeatSeries(t, "Office", "Zone People Latent Gain Energy", 2, "people-latent"),
		reviewEnergyHeatSeries(t, "Office", "Zone Lights Total Heating Energy", 50, "lights-total"),
		reviewEnergyHeatSeries(t, "Office", "Zone Lights Convective Heating Energy", 5, "lights-convective"),
		reviewEnergyHeatSeries(t, "Office", "Zone Lights Radiant Heating Energy", 25, "lights-radiant"),
		reviewEnergyHeatSeries(t, "Office", "Zone Lights Visible Radiation Heating Energy", 10, "lights-visible"),
		reviewEnergyHeatSeries(t, "Office", "Zone Lights Return Air Heating Energy", 10, "lights-return"),
		reviewEnergyHeatSeries(t, "Office", "Zone Electric Equipment Convective Heating Energy", 3, "equipment-convective"),
		reviewEnergyHeatSeries(t, "Office", "Zone Electric Equipment Latent Gain Energy", 1, "equipment-latent"),
		reviewEnergyHeatSeries(t, "Office", "Zone Electric Equipment Lost Heat Energy", 40, "equipment-lost"),
		reviewEnergyHeatSeries(t, "Office", "Zone Total Internal Convective Heating Energy", 20, "internal-convective-total"),
		reviewEnergyHeatSeries(t, "Office", "Zone Total Internal Latent Gain Energy", 4, "internal-latent-total"),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.GeometryReport{}))
	result := UpgradeEnergyExplanationV1(legacy)

	checks := []struct {
		id      string
		value   float64
		sources []string
	}{
		{id: "driver.internal.people.cooling.building", value: 12, sources: []string{"people-convective", "people-latent"}},
		{id: "driver.internal.lighting.cooling.building", value: 5, sources: []string{"lights-convective"}},
		{id: "driver.internal.equipment.cooling.building", value: 4, sources: []string{"equipment-convective", "equipment-latent"}},
	}
	for _, check := range checks {
		node := energyPathV2NodeByID(result.Nodes, check.id)
		if node == nil || node.Value != check.value {
			t.Fatalf("%s = %#v; want %g", check.id, node, check.value)
		}
		for _, sourceID := range check.sources {
			if !stringSliceContains(node.SourceIDs, sourceID) {
				t.Fatalf("%s missing source %q: %#v", check.id, sourceID, node)
			}
		}
	}
	lighting := energyPathV2NodeByID(result.Nodes, "driver.internal.lighting.cooling.building")
	for _, forbidden := range []string{"lights-total", "lights-radiant", "lights-visible", "lights-return"} {
		if lighting != nil && stringSliceContains(lighting.SourceIDs, forbidden) {
			t.Fatalf("lighting context source %q leaked into main flow: %#v", forbidden, lighting)
		}
	}
	equipment := energyPathV2NodeByID(result.Nodes, "driver.internal.equipment.cooling.building")
	if equipment != nil && stringSliceContains(equipment.SourceIDs, "equipment-lost") {
		t.Fatalf("equipment lost-heat context leaked into main flow: %#v", equipment)
	}
	other := energyPathV2NodeByID(result.Nodes, "driver.balance.storage_other.cooling.building")
	if other == nil || other.Value < 3 {
		t.Fatalf("internal reconciliation gap did not reach Other / storage: %#v", other)
	}
	for _, component := range []string{"internal.other.reconciliation_gap.sensible", "internal.other.reconciliation_gap.latent"} {
		found := false
		for _, source := range result.Sources {
			if source.DriverComponent == component {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing inspector component %q: %#v", component, result.Sources)
		}
	}
	for _, sourceID := range []string{"people-convective", "people-latent", "lights-convective", "equipment-convective", "equipment-latent"} {
		source := energyExplanationSourceByID(result.Sources, sourceID)
		if source == nil || source.HeatDirection != "gain" {
			t.Errorf("internal source %q lacks explicit gain semantics: %#v", sourceID, source)
		}
	}
}

// Equipment objects are additive source families, not aliases for one another.
// The canonical graph may merge them into one Equipment node only after every
// independently reported family has survived Energy/Rate preference selection.
func TestEPATH061ReviewEquipmentFamiliesAreSummedNotAliasDeduped(t *testing.T) {
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 21, "load"),
		reviewEnergyHeatSeries(t, "Office", "Zone Electric Equipment Convective Heating Energy", 1, "electric"),
		reviewEnergyHeatSeries(t, "Office", "Zone Gas Equipment Convective Heating Energy", 2, "gas"),
		reviewEnergyHeatSeries(t, "Office", "Zone Other Equipment Convective Heating Energy", 3, "other"),
		reviewEnergyHeatSeries(t, "Office", "Zone Hot Water Equipment Convective Heating Energy", 4, "hot-water"),
		reviewEnergyHeatSeries(t, "Office", "Zone Steam Equipment Convective Heating Energy", 5, "steam"),
		reviewEnergyHeatSeries(t, "Office", "Zone Electric Equipment Latent Gain Energy", 6, "electric-latent"),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.GeometryReport{}))
	result := UpgradeEnergyExplanationV1(legacy)
	equipment := energyPathV2NodeByID(result.Nodes, "driver.internal.equipment.cooling.building")
	if equipment == nil || equipment.Value != 21 {
		t.Fatalf("equipment family sum = %#v; want all five convective families plus latent contribution", equipment)
	}
	for _, sourceID := range []string{"electric", "gas", "other", "hot-water", "steam", "electric-latent"} {
		if !stringSliceContains(equipment.SourceIDs, sourceID) {
			t.Errorf("equipment family sum is missing source %q: %#v", sourceID, equipment)
		}
	}
}

// The two zone-level internal-convective outputs are alternate aggregate
// reconciliation sources. Energy Path requests both for version/availability
// coverage, but must select one instead of adding equivalent totals together.
func TestEPATH061ReviewInternalAggregateAliasesAreNotDoubleCounted(t *testing.T) {
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 10, "load"),
		reviewEnergyHeatSeries(t, "Office", "Zone People Convective Heating Energy", 10, "people"),
		reviewEnergyHeatSeries(t, "Office", "Zone Total Internal Convective Heating Energy", 10, "internal-total"),
		reviewEnergyHeatSeries(t, "Office", "Zone Air Heat Balance Internal Convective Heat Gain Rate", 10, "internal-balance"),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.GeometryReport{}))
	result := UpgradeEnergyExplanationV1(legacy)
	for _, source := range result.Sources {
		if source.DriverComponent == "internal.other.reconciliation_gap.sensible" && source.RawValue != 0 {
			t.Fatalf("equivalent internal aggregate aliases produced a false reconciliation gap: %#v", source)
		}
	}
	for _, item := range result.Reconciliation {
		if item.Label == "Internal gain reconciliation - Office" && (item.ExpectedValue != 10 || item.ExplainedValue != 10 || item.ResidualValue != 0) {
			t.Fatalf("internal aggregate alias reconciliation = %#v; want one 10 kWh aggregate", item)
		}
	}
}

func TestEPATH062ReviewSeasonalInfiltrationGainAndLossSurviveAnnual(t *testing.T) {
	heating := reviewEnergyLoadSeries("Office", "heating", 7, "heating-load")
	heating.Monthly = map[int]float64{1: 7}
	cooling := reviewEnergyLoadSeries("Office", "cooling", 11, "cooling-load")
	cooling.Monthly = map[int]float64{7: 11}
	sensibleLoss := reviewEnergyHeatSeries(t, "Office", "Zone Infiltration Sensible Heat Loss Energy", 5, "infiltration-sensible-loss")
	sensibleLoss.Monthly = map[int]float64{1: 5}
	latentLoss := reviewEnergyHeatSeries(t, "Office", "Zone Infiltration Latent Heat Loss Energy", 2, "infiltration-latent-loss")
	latentLoss.Monthly = map[int]float64{1: 2}
	sensibleGain := reviewEnergyHeatSeries(t, "Office", "Zone Infiltration Sensible Heat Gain Energy", 8, "infiltration-sensible-gain")
	sensibleGain.Monthly = map[int]float64{7: 8}
	latentGain := reviewEnergyHeatSeries(t, "Office", "Zone Infiltration Latent Heat Gain Energy", 3, "infiltration-latent-gain")
	latentGain.Monthly = map[int]float64{7: 3}
	outdoorAggregate := reviewEnergyHeatSeries(t, "Office", "Zone Air Heat Balance Outdoor Air Transfer Rate", 3, "outdoor-air-aggregate")
	outdoorAggregate.Monthly = map[int]float64{1: -5, 7: 8}
	series := []energyExplanationSeries{heating, cooling, sensibleLoss, latentLoss, sensibleGain, latentGain, outdoorAggregate}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.GeometryReport{}))
	result := UpgradeEnergyExplanationV1(legacy)

	heatingDriver := energyPathV2NodeByID(result.Nodes, "driver.air.infiltration.heating.building")
	coolingDriver := energyPathV2NodeByID(result.Nodes, "driver.air.infiltration.cooling.building")
	if heatingDriver == nil || heatingDriver.Value != 7 || heatingDriver.SignedValue != -7 ||
		!stringSliceContains(heatingDriver.SourceIDs, "infiltration-sensible-loss") || !stringSliceContains(heatingDriver.SourceIDs, "infiltration-latent-loss") {
		t.Fatalf("annual infiltration loss = %#v", heatingDriver)
	}
	if coolingDriver == nil || coolingDriver.Value != 11 || coolingDriver.SignedValue != 11 ||
		!stringSliceContains(coolingDriver.SourceIDs, "infiltration-sensible-gain") || !stringSliceContains(coolingDriver.SourceIDs, "infiltration-latent-gain") {
		t.Fatalf("annual infiltration gain = %#v", coolingDriver)
	}
	for _, node := range result.Nodes {
		if node.Level == "driver" && stringSliceContains(node.SourceIDs, "outdoor-air-aggregate") {
			t.Fatalf("outdoor-air aggregate was added as a second main driver: %#v", node)
		}
	}
	aggregate := energyExplanationSourceByID(result.Sources, "outdoor-air-aggregate")
	if aggregate == nil || aggregate.DriverRole != energyDriverSourceRoleReconciliation {
		t.Fatalf("outdoor-air aggregate provenance = %#v", aggregate)
	}
	foundOutdoorCheck := false
	for _, item := range result.Reconciliation {
		if item.Label == "Outdoor-air reconciliation - Office" {
			foundOutdoorCheck = item.ExpectedValue == 3 && item.ExplainedValue == 3 && item.ResidualValue == 0 && item.Status == "balanced"
		}
	}
	if !foundOutdoorCheck {
		t.Fatalf("missing sensible outdoor-air reconciliation: %#v", result.Reconciliation)
	}
	for _, sourceID := range []string{"infiltration-sensible-loss", "infiltration-latent-loss", "infiltration-sensible-gain", "infiltration-latent-gain"} {
		source := energyExplanationSourceByID(result.Sources, sourceID)
		if source == nil || source.DriverComponent == "" || source.HeatDirection == "" {
			t.Fatalf("infiltration dimension provenance %q = %#v", sourceID, source)
		}
	}
}

func TestEPATH060ReviewSurfaceAggregateMismatchIsSeparateBalanceTerm(t *testing.T) {
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 100, "load"),
		reviewEnergyHeatSeries(t, "Wall A", "Surface Inside Face Convection Heat Gain Energy", -10, "surface-detail"),
		reviewEnergyHeatSeries(t, "Office", "Zone Air Heat Balance Surface Convection Rate", 12, "surface-aggregate"),
	}
	sources := reviewEnergySources(series)
	report := idf.GeometryReport{Surfaces: []idf.GeometrySurface{
		{ID: "wall.a", Name: "Wall A", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Outdoors"},
	}}
	legacy := buildEnergyExplanationResultWithDriverContext(series, sources, &PurposeRunPlan{}, newEnergyDriverBuildContext(report))
	result := UpgradeEnergyExplanationV1(legacy)

	wall := energyPathV2NodeByID(result.Nodes, "driver.surface.exterior_walls.cooling.building")
	if wall == nil || wall.Value != 10 || !stringSliceContains(wall.SourceIDs, "surface-detail") || stringSliceContains(wall.SourceIDs, "surface-aggregate") {
		t.Fatalf("source-level surface driver must remain 10 kWh without aggregate duplication: %#v", wall)
	}
	var check *EnergyReconciliation
	for index := range result.Reconciliation {
		if result.Reconciliation[index].Label == "Surface reconciliation - Office" {
			check = &result.Reconciliation[index]
			break
		}
	}
	if check == nil || check.ExpectedValue != 12 || check.ExplainedValue != 10 || check.ResidualValue != 2 || check.Status == "balanced" ||
		!stringSliceContains(check.SourceIDs, "surface-detail") || !stringSliceContains(check.SourceIDs, "surface-aggregate") {
		t.Fatalf("surface reconciliation = %#v; all=%#v", check, result.Reconciliation)
	}
	derived := reviewEnergySourceWithFormula(result.Sources, "signed zone surface-convection aggregate - signed selected surface-source sum")
	if derived == nil || derived.DriverCategory != energyDriverCategoryStorageOther || derived.RawValue != 2 ||
		!stringSliceContains(derived.InputSourceIDs, "surface-detail") || !stringSliceContains(derived.InputSourceIDs, "surface-aggregate") {
		t.Fatalf("surface mismatch balance source = %#v; sources=%#v", derived, result.Sources)
	}
	if !reviewHasWarning(result.Warnings, "energy_driver_surface_reconciliation_gap") {
		t.Fatalf("missing above-tolerance surface warning: %#v", result.Warnings)
	}
}

func TestEPATH060ReviewBaseSurfaceAndFenestrationStayDistinctExactlyOnce(t *testing.T) {
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 15, "load"),
		reviewEnergyHeatSeries(t, "North Wall", "Surface Inside Face Convection Heat Gain Energy", -10, "wall"),
		reviewEnergyHeatSeries(t, "North Window", "Surface Inside Face Convection Heat Gain Energy", -5, "window"),
	}
	report := idf.GeometryReport{
		Surfaces: []idf.GeometrySurface{
			{ID: "surface.wall", Name: "North Wall", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Outdoors"},
		},
		Windows: []idf.GeometryWindow{
			{ID: "window.north", Name: "North Window", SurfaceType: "Window", BaseSurfaceID: "surface.wall", ZoneName: "Office"},
		},
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(report))
	result := UpgradeEnergyExplanationV1(legacy)
	wall := energyPathV2NodeByID(result.Nodes, "driver.surface.exterior_walls.cooling.building")
	window := energyPathV2NodeByID(result.Nodes, "driver.surface.windows_doors.cooling.building")
	if wall == nil || wall.Value != 10 || !stringSliceContains(wall.SourceIDs, "wall") || stringSliceContains(wall.SourceIDs, "window") {
		t.Fatalf("base-surface driver = %#v", wall)
	}
	if window == nil || window.Value != 5 || !stringSliceContains(window.SourceIDs, "window") || stringSliceContains(window.SourceIDs, "wall") {
		t.Fatalf("fenestration driver = %#v", window)
	}
	if wall.Value+window.Value != 15 {
		t.Fatalf("base + fenestration = %g; want each physical output once", wall.Value+window.Value)
	}
}

func TestEPATH060ReviewZeroNetSeasonalSurfaceRawProvenanceIsNotOverwritten(t *testing.T) {
	seasonal := reviewEnergyHeatSeries(t, "Wall A", "Surface Inside Face Convection Heat Gain Energy", 0, "surface-seasonal")
	seasonal.Monthly = map[int]float64{1: 5, 7: -5}
	seasonal.RawMonthly = map[int]float64{1: 5, 7: -5}
	seasonal.parseCategorySource = true
	other := reviewEnergyHeatSeries(t, "Wall B", "Surface Inside Face Convection Heat Gain Energy", 5, "surface-other")
	other.parseCategorySource = true
	aggregate := other
	aggregate.SurfaceScoped = false
	aggregate.SurfaceName = ""
	aggregate.sourceKeyValue = ""
	aggregate.SourceKey = ""
	aggregate.sourceName = "Selected surface inside-face convection by surface"
	aggregate.SourceName = aggregate.sourceName
	aggregate.SourceIDs = []string{"surface-seasonal", "surface-other"}
	aggregate.MonthlySourceIDs = append([]string(nil), aggregate.SourceIDs...)
	aggregate.AnnualSourceIDs = append([]string(nil), aggregate.SourceIDs...)
	aggregate.DriverCategory = energyDriverCategoryExteriorWalls
	aggregate.parseCategoryAggregate = true
	report := idf.GeometryReport{Surfaces: []idf.GeometrySurface{
		{ID: "wall.a", Name: "Wall A", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Outdoors"},
		{ID: "wall.b", Name: "Wall B", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Outdoors"},
	}}
	_, sources, _ := prepareEnergyDriverSeries(
		[]energyExplanationSeries{seasonal, other, aggregate},
		[]EnergyDataSource{{ID: "surface-seasonal"}, {ID: "surface-other"}},
		newEnergyDriverBuildContext(report),
	)
	seasonalSource := energyExplanationSourceByID(sources, "surface-seasonal")
	otherSource := energyExplanationSourceByID(sources, "surface-other")
	if seasonalSource == nil || seasonalSource.RawValue != 0 {
		t.Fatalf("zero-net seasonal surface raw value was overwritten by category total: %#v", seasonalSource)
	}
	if otherSource == nil || otherSource.RawValue != 5 {
		t.Fatalf("other surface raw provenance = %#v", otherSource)
	}
}

func TestEPATH063ReviewCombinedOutdoorAirFallbackHasExplicitProvenance(t *testing.T) {
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 10, "load"),
		reviewEnergyHeatSeries(t, "Office", "Zone Infiltration Sensible Heat Gain Energy", 4, "infiltration"),
		reviewEnergyHeatSeries(t, "Office", "Zone Combined Outdoor Air Sensible Heat Gain Energy", 10, "combined-oa"),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.GeometryReport{}))
	result := UpgradeEnergyExplanationV1(legacy)

	infiltration := energyPathV2NodeByID(result.Nodes, "driver.air.infiltration.cooling.building")
	ventilation := energyPathV2NodeByID(result.Nodes, "driver.air.mechanical_ventilation.cooling.building")
	if infiltration == nil || infiltration.Value != 4 || ventilation == nil || ventilation.Value != 6 {
		t.Fatalf("outdoor-air split = infiltration %#v / mechanical ventilation %#v; sources=%#v", infiltration, ventilation, result.Sources)
	}
	derived := reviewEnergySourceWithFormula(result.Sources, "signed outdoor-air aggregate - signed infiltration")
	if derived == nil || derived.DriverCategory != energyDriverCategoryMechanicalVentilation || derived.RawValue != 6 ||
		!stringSliceContains(derived.InputSourceIDs, "combined-oa") || !stringSliceContains(derived.InputSourceIDs, "infiltration") {
		t.Fatalf("mechanical ventilation fallback source = %#v", derived)
	}
	combined := energyExplanationSourceByID(result.Sources, "combined-oa")
	if combined == nil || combined.DriverRole != energyDriverSourceRoleReconciliation || combined.InspectorSection != energyDriverInspectorSectionContext {
		t.Fatalf("combined outdoor-air aggregate must remain reconciliation-only: %#v", combined)
	}
	if !reviewHasWarning(result.Warnings, "energy_driver_ventilation_fallback") {
		t.Fatalf("fallback provenance flag missing: %#v", result.Warnings)
	}
}

// Aggregate preference is component-local. A latent Combined Outdoor Air row
// must not suppress the sensible Zone Air Heat Balance Outdoor Air Transfer
// row when that is the only available sensible reconciliation source.
func TestEPATH063ReviewOutdoorAirAggregatePreferenceIsPerComponent(t *testing.T) {
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 11, "load"),
		reviewEnergyHeatSeries(t, "Office", "Zone Infiltration Sensible Heat Gain Energy", 4, "infiltration-sensible"),
		reviewEnergyHeatSeries(t, "Office", "Zone Infiltration Latent Heat Gain Energy", 1, "infiltration-latent"),
		reviewEnergyHeatSeries(t, "Office", "Zone Air Heat Balance Outdoor Air Transfer Rate", 10, "outdoor-sensible"),
		reviewEnergyHeatSeries(t, "Office", "Zone Combined Outdoor Air Latent Heat Gain Energy", 2, "outdoor-latent"),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.GeometryReport{}))
	result := UpgradeEnergyExplanationV1(legacy)
	ventilation := energyPathV2NodeByID(result.Nodes, "driver.air.mechanical_ventilation.cooling.building")
	if ventilation == nil || ventilation.Value != 7 {
		t.Fatalf("component-local outdoor-air fallback = %#v; want sensible (10-4) + latent (2-1)", ventilation)
	}
	for _, sourceID := range []string{"outdoor-sensible", "outdoor-latent", "infiltration-sensible", "infiltration-latent"} {
		if !stringSliceContains(ventilation.SourceIDs, sourceID) {
			t.Errorf("component-local outdoor-air fallback is missing source %q: %#v", sourceID, ventilation)
		}
	}
}

// Combined Outdoor Air exposes gain and loss as separate reported quantities.
// The fallback must subtract the matching infiltration direction before graph
// projection so simultaneous monthly gain/loss is not collapsed to a net.
func TestEPATH063ReviewCombinedOutdoorAirFallbackPreservesGainAndLoss(t *testing.T) {
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 6, "cooling-load"),
		reviewEnergyLoadSeries("Office", "heating", 5, "heating-load"),
		reviewEnergyHeatSeries(t, "Office", "Zone Infiltration Sensible Heat Gain Energy", 4, "infiltration-gain"),
		reviewEnergyHeatSeries(t, "Office", "Zone Infiltration Sensible Heat Loss Energy", 3, "infiltration-loss"),
		reviewEnergyHeatSeries(t, "Office", "Zone Combined Outdoor Air Sensible Heat Gain Energy", 10, "outdoor-gain"),
		reviewEnergyHeatSeries(t, "Office", "Zone Combined Outdoor Air Sensible Heat Loss Energy", 8, "outdoor-loss"),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.GeometryReport{}))
	result := UpgradeEnergyExplanationV1(legacy)
	cooling := energyPathV2NodeByID(result.Nodes, "driver.air.mechanical_ventilation.cooling.building")
	heating := energyPathV2NodeByID(result.Nodes, "driver.air.mechanical_ventilation.heating.building")
	if cooling == nil || cooling.Value != 6 || heating == nil || heating.Value != 5 {
		t.Fatalf("directional outdoor-air fallback = cooling %#v / heating %#v; want gain 10-4 and loss 8-3", cooling, heating)
	}
}

func TestEPATH063ReviewIdealLoadsOutdoorAirAndHeatRecoveryAreRequestedContext(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyPathScopeFixtureIDF)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	for _, family := range []struct {
		middle string
		kind   string
	}{
		{middle: "Outdoor Air", kind: "heat.ventilation_ideal_oa"},
		{middle: "Heat Recovery", kind: "heat.ventilation_ideal_heat_recovery"},
	} {
		for _, component := range []string{"Sensible", "Latent", "Total"} {
			for _, mode := range []string{"Heating", "Cooling"} {
				for _, quantity := range []string{"Energy", "Rate"} {
					name := fmt.Sprintf("Zone Ideal Loads %s %s %s %s", family.middle, component, mode, quantity)
					definition, ok := energyHeatAliasDefinitionForName(name)
					if !ok || definition.Kind != family.kind {
						t.Errorf("context alias %q = %#v, %t; want kind %q", name, definition, ok, family.kind)
						continue
					}
					if policy := energyDriverSourcePolicyFor(name, definition.Kind); policy.Role != energyDriverSourceRoleContext {
						t.Errorf("context alias %q role = %q", name, policy.Role)
					}
					output := findPurposeOutput(plan, "Output:Variable", "Office Ideal Loads", name)
					if output == nil || output.ScopeZoneName != "Office" {
						t.Errorf("Energy Path did not request Office Ideal Loads context %q with owner provenance: %#v", name, output)
					}
				}
			}
		}
	}
}

func TestEPATH064ReviewGenericMixingIsZoneOnlyWithTopologyProvenance(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyPathScopeFixtureIDF)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	for _, zoneName := range []string{"Office", "Lab"} {
		if findPurposeOutput(plan, "Output:Variable", zoneName, "Zone Mixing Sensible Heat Gain Energy") == nil {
			t.Errorf("missing official generic Zone Mixing aggregate for %s", zoneName)
		}
		for _, invalid := range []string{
			"Zone Cross Mixing Sensible Heat Gain Energy",
			"Zone CrossMixing Latent Heat Loss Energy",
			"Zone Refrigeration Door Mixing Sensible Heat Gain Energy",
		} {
			if findPurposeOutput(plan, "Output:Variable", zoneName, invalid) != nil {
				t.Errorf("new output plan requested non-authoritative individual mixing output %q for %s", invalid, zoneName)
			}
		}
	}

	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 10, "load"),
		reviewEnergyHeatSeries(t, "Office", "Zone Mixing Sensible Heat Gain Energy", 10, "mixing-zone-aggregate"),
	}
	report := idf.GeometryReport{Topology: idf.ThermalTopologyReport{
		Nodes: []idf.ThermalTopologyNode{
			{ID: "zone.office", EntityID: "entity.office", ZoneName: "Office"},
			{ID: "zone.lab", EntityID: "entity.lab", ZoneName: "Lab"},
		},
		AirCouplings: []idf.ThermalAirCoupling{
			{ID: "air-coupling.mix-1", EntityID: "mixing.1", ObjectType: "ZoneMixing", ObjectName: "Office from Lab", FromNodeID: "zone.lab", ToNodeID: "zone.office", Direction: "directed"},
		},
	}}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(report))
	result := UpgradeEnergyExplanationV1(legacy)
	for _, node := range result.Nodes {
		if node.Level == "driver" && node.DriverCategory == energyDriverCategoryInterzoneAir {
			t.Fatalf("generic receiving-zone mixing aggregate leaked into Building main flow: %#v", node)
		}
	}
	buildingIncoming := 0.0
	for _, link := range result.Links {
		if link.ToID == "load.cooling.building" && link.Relation == "driver_to_load" {
			buildingIncoming += link.ToValue
		}
	}
	if buildingIncoming != 10 {
		t.Errorf("generic-mixing Building closure = %g kWh, want 10 retained outside a major interzone node: nodes=%#v links=%#v", buildingIncoming, result.Nodes, result.Links)
	}
	if len(result.ZoneResults) != 1 {
		t.Fatalf("zone projections = %#v", result.ZoneResults)
	}
	zoneNode := energyPathV2NodeByID(result.ZoneResults[0].Nodes, "driver.air.interzone.cooling.office")
	if zoneNode == nil || zoneNode.Value != 10 || !stringSliceContains(zoneNode.SourceIDs, "mixing-zone-aggregate") || !stringSliceContains(zoneNode.RelatedEntityIDs, "air-coupling.mix-1") {
		t.Fatalf("zone mixing node with topology provenance = %#v", zoneNode)
	}
	source := energyExplanationSourceByID(result.Sources, "mixing-zone-aggregate")
	if source == nil || !stringSliceContains(source.RelatedEntityIDs, "air-coupling.mix-1") {
		t.Fatalf("mixing source topology provenance = %#v", source)
	}
}

func TestEPATH064ReviewExplicitPairConservesAtBuildingWithoutDisablingZoneClosure(t *testing.T) {
	pairSeries := func(key string, sign string, total float64, sourceID string) energyExplanationSeries {
		return canonicalEnergyExplanationSeries(energyExplanationSeries{
			Level:              "heat",
			Kind:               "heat.mixing",
			Label:              "Explicit pairwise mixing",
			Unit:               "kWh",
			ThermalComponent:   "sensible",
			HeatSign:           sign,
			SourceIDs:          []string{sourceID},
			Total:              total,
			Monthly:            map[int]float64{1: total},
			sourceKeyValue:     key,
			sourceName:         "Explicit pairwise mixing heat " + sign,
			sourceFrequency:    "Monthly",
			heatSignMultiplier: energyHeatSignMultiplier(sign),
		})
	}
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 20, "office-load"),
		reviewEnergyLoadSeries("Lab", "heating", 20, "lab-load"),
		pairSeries("Office from Lab", "positive", 10, "pair-office"),
		pairSeries("Lab from Office", "negative", 9, "pair-lab"),
	}
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
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(report))
	result := UpgradeEnergyExplanationV1(legacy)

	pairResidual := reviewEnergySourceWithFormula(result.Sources, "signed pairwise interzone receiving-zone contributions summed at Building scope")
	if pairResidual == nil || pairResidual.DriverCategory != energyDriverCategoryStorageOther || pairResidual.RawValue != 1 ||
		!stringSliceContains(pairResidual.InputSourceIDs, "pair-office") || !stringSliceContains(pairResidual.InputSourceIDs, "pair-lab") {
		t.Fatalf("small Building pair residual = %#v", pairResidual)
	}
	for _, node := range result.Nodes {
		if node.Level == "driver" && node.DriverCategory == energyDriverCategoryInterzoneAir {
			t.Fatalf("small pair net should fold into Other / storage: %#v", node)
		}
	}
	closureZones := map[string]bool{}
	for _, source := range result.Sources {
		if source.Formula == "signed zone cooling load - signed zone heating load - signed mapped driver contributions" {
			closureZones[source.ZoneName] = true
		}
	}
	if !closureZones["Office"] || !closureZones["Lab"] {
		t.Fatalf("Building-only pair residual disabled zone closure: zones=%#v sources=%#v", closureZones, result.Sources)
	}
	foundPairCheck := false
	for _, item := range result.Reconciliation {
		if item.Label == "Interzone pairwise balance" {
			foundPairCheck = item.ExplainedValue == 1 && item.ResidualValue == -1 &&
				stringSliceContains(item.SourceIDs, "pair-office") && stringSliceContains(item.SourceIDs, "pair-lab")
		}
	}
	if !foundPairCheck {
		t.Fatalf("missing pairwise conservation check: %#v", result.Reconciliation)
	}
	for _, service := range []string{"cooling", "heating"} {
		loadID := "load." + service + ".building"
		incoming := 0.0
		for _, link := range result.Links {
			if link.ToID == loadID && link.Relation == "driver_to_load" {
				incoming += link.ToValue
			}
		}
		if incoming != 20 {
			t.Errorf("Building %s driver closure = %g kWh, want 20 after pairwise suppression: nodes=%#v links=%#v", service, incoming, result.Nodes, result.Links)
		}
	}
	for _, node := range result.Nodes {
		if node.Level == "residual" && node.Kind == "heat.residual" {
			t.Errorf("pairwise Building projection reopened a legacy heat residual: %#v", node)
		}
	}
}

func TestEPATH064ReviewStoredV1GenericMixingCannotBecomeBuildingMajor(t *testing.T) {
	fixture := EnergyExplanationV1{
		Schema: energyExplanationV1Schema,
		Nodes: []EnergyExplanationNode{
			{ID: "load", Level: "load", Kind: "load.zone_cooling", Label: "Cooling", Value: 10, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"load-source"}},
			{ID: "mixing", Level: "heat", Kind: "heat.mixing", Label: "Mixing", Value: 10, SignedValue: 10, DisplayValue: 10, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", HeatCategory: "air_exchange", SourceIDs: []string{"mixing-source"}},
		},
		Edges: []EnergyExplanationEdge{
			{ID: "mixing-edge", FromID: "load", ToID: "mixing", Value: 10, DisplayValue: 10, Unit: "kWh", Relation: "heat_driver", SourceIDs: []string{"mixing-source"}},
		},
		Sources: []EnergyDataSource{
			{ID: "load-source", Name: "Zone Air System Sensible Cooling Energy", ZoneName: "Office"},
			{ID: "mixing-source", Name: "Zone Mixing Sensible Heat Gain Energy", ZoneName: "Office"},
		},
	}
	building := UpgradeEnergyExplanationV1(fixture)
	for _, node := range building.Nodes {
		if node.Level == "driver" && node.DriverCategory == energyDriverCategoryInterzoneAir && stringSliceContains(node.SourceIDs, "mixing-source") {
			t.Fatalf("stored v1 receiving-zone aggregate became a Building major node without pairwise provenance: %#v", node)
		}
	}
	storedIncoming := 0.0
	for _, link := range building.Links {
		if link.ToID == "load.cooling.building" && link.Relation == "driver_to_load" {
			storedIncoming += link.ToValue
		}
	}
	if storedIncoming != 10 {
		t.Fatalf("stored v1 generic mixing disappeared from Building closure: incoming=%g nodes=%#v links=%#v", storedIncoming, building.Nodes, building.Links)
	}
	fixture.scope = EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}
	zone := UpgradeEnergyExplanationV1(fixture)
	if node := energyPathV2NodeByID(zone.Nodes, "driver.air.interzone.cooling.office"); node == nil || node.Value != 10 || !stringSliceContains(node.SourceIDs, "mixing-source") {
		t.Fatalf("stored v1 generic mixing was not retained in Zone scope: %#v", node)
	}
}

func TestEPATH065ReviewAirStorageSignAndWarningThreshold(t *testing.T) {
	for _, test := range []struct {
		name         string
		storage      float64
		infiltration float64
		wantWarning  bool
	}{
		{name: "below five percent", storage: 4, infiltration: 96, wantWarning: false},
		{name: "above five percent", storage: 6, infiltration: 94, wantWarning: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			series := []energyExplanationSeries{
				reviewEnergyLoadSeries("Office", "heating", 100, "load"),
				reviewEnergyHeatSeries(t, "Office", "Zone Infiltration Sensible Heat Loss Energy", test.infiltration, "infiltration"),
				reviewEnergyHeatSeries(t, "Office", "Zone Air Heat Balance Air Energy Storage Rate", test.storage, "air-storage"),
			}
			legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.GeometryReport{}))
			result := UpgradeEnergyExplanationV1(legacy)
			storage := energyPathV2NodeByID(result.Nodes, "driver.balance.storage_other.heating.building")
			if storage == nil || storage.Value != test.storage || storage.SignedValue != -test.storage || !stringSliceContains(storage.SourceIDs, "air-storage") {
				t.Fatalf("storage driver = %#v; raw EnergyPlus storage must be inverted for load-pressure convention", storage)
			}
			source := energyExplanationSourceByID(result.Sources, "air-storage")
			if source == nil || source.RawValue != test.storage || source.DriverComponent != "zone_air_storage" {
				t.Fatalf("storage raw provenance = %#v", source)
			}
			if got := reviewHasWarning(result.Warnings, "energy_driver_storage_other_large"); got != test.wantWarning {
				t.Fatalf("storage warning=%t, want %t: %#v", got, test.wantWarning, result.Warnings)
			}
		})
	}
}

func TestEPATH065ReviewRemainingHeatClosesAsNamedStorageWithoutScalingDrivers(t *testing.T) {
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 100, "load"),
		reviewEnergyHeatSeries(t, "Office", "Zone Infiltration Sensible Heat Gain Energy", 30, "infiltration"),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, reviewEnergySources(series), &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.GeometryReport{}))
	result := UpgradeEnergyExplanationV1(legacy)
	infiltration := energyPathV2NodeByID(result.Nodes, "driver.air.infiltration.cooling.building")
	storage := energyPathV2NodeByID(result.Nodes, "driver.balance.storage_other.cooling.building")
	if infiltration == nil || infiltration.Value != 30 || storage == nil || storage.Value != 70 {
		t.Fatalf("closure drivers = infiltration %#v / storage %#v", infiltration, storage)
	}
	if link := energyPathV2LinkByIDs(result.Links, infiltration.ID, "load.cooling.building"); link == nil || link.FromValue != 30 || link.ToValue != 30 {
		t.Fatalf("measured infiltration was prematurely scaled: %#v", link)
	}
	derived := reviewEnergySourceWithFormula(result.Sources, "signed zone cooling load - signed zone heating load - signed mapped driver contributions")
	if derived == nil || derived.RawValue != 70 || !stringSliceContains(derived.InputSourceIDs, "load") || !stringSliceContains(derived.InputSourceIDs, "infiltration") {
		t.Fatalf("unmapped heat-balance source = %#v", derived)
	}
	for _, node := range result.Nodes {
		if node.Level == "residual" && node.Kind == "heat.residual" {
			t.Fatalf("legacy heat residual survived instead of named Other / storage closure: %#v", node)
		}
	}
}

func reviewEnergyHeatSeries(t *testing.T, keyValue string, name string, total float64, sourceID string) energyExplanationSeries {
	t.Helper()
	definition, ok := energyHeatAliasDefinitionForName(name)
	if !ok {
		t.Fatalf("missing heat alias %q", name)
	}
	return canonicalEnergyExplanationSeries(energyExplanationSeriesForBuilder(&energyExplanationSeriesBuilder{
		dictionary: energyExplanationDictionary{
			row:                sqlOutputDictionaryRow{keyValue: keyValue, name: name, units: "J"},
			reportingFrequency: "Monthly",
			heat:               &definition,
		},
		unit:    "kWh",
		total:   total,
		monthly: map[int]float64{1: total},
	}, sourceID))
}

func reviewEnergyLoadSeries(zoneName string, service string, total float64, sourceID string) energyExplanationSeries {
	return canonicalEnergyExplanationSeries(energyExplanationSeries{
		Level:           "load",
		Kind:            "load.zone_" + service,
		Label:           service + " load",
		Unit:            "kWh",
		ServiceKind:     service,
		PathType:        "zone",
		ZoneName:        zoneName,
		SourceIDs:       []string{sourceID},
		Total:           total,
		Monthly:         map[int]float64{1: total},
		sourceName:      "Zone Air System Sensible " + service + " Energy",
		sourceFrequency: "Monthly",
	})
}

func reviewEnergySources(series []energyExplanationSeries) []EnergyDataSource {
	out := make([]EnergyDataSource, 0, len(series))
	for _, item := range series {
		for _, sourceID := range item.SourceIDs {
			out = append(out, EnergyDataSource{ID: sourceID, KeyValue: item.sourceKeyValue, Name: item.sourceName})
		}
	}
	return out
}

func reviewEnergySourceWithFormula(sources []EnergyDataSource, formula string) *EnergyDataSource {
	for index := range sources {
		if sources[index].Formula == formula {
			return &sources[index]
		}
	}
	return nil
}

func reviewHasWarning(warnings []EnergyWarning, code string) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}
