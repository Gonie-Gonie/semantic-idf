package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathBaseboardRecipientOriginalRosterPreservesAllSurfaceValues(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "5ZoneElectricBaseboard.idf"))
	if err != nil {
		t.Fatal(err)
	}
	doc := parsePurposePlanFixture(t, string(data))
	before := doc.String()
	geometry := idf.AnalyzeGeometry(doc)
	context := newEnergyDriverBuildContext(geometry, doc)
	if len(context.BaseboardTargets) != 2 || len(geometry.Surfaces) != 42 {
		t.Fatalf("original equipment/surface roster changed: targets=%d surfaces=%d", len(context.BaseboardTargets), len(geometry.Surfaces))
	}
	// AnalyzeGeometry also includes the two original overhangs. They are
	// geometry, not inside-face convection observations or radiant recipients.
	originalSurfaces, originalShading := map[int]idf.Object{}, map[int]idf.Object{}
	for _, object := range doc.Objects {
		switch object.Type {
		case "BuildingSurface:Detailed":
			originalSurfaces[object.Index] = object
		case "Shading:Zone:Detailed":
			originalShading[object.Index] = object
		}
	}
	if len(originalSurfaces) != 40 || len(originalShading) != 2 {
		t.Fatalf("original typed inventory changed: building=%d shading=%d", len(originalSurfaces), len(originalShading))
	}
	wantShading := map[string]bool{"Main South Overhang": true, "South Door Overhang": true}
	seenGeometry := map[int]bool{}
	var convectionSurfaces []idf.GeometrySurface
	for _, surface := range geometry.Surfaces {
		if seenGeometry[surface.ObjectIndex] {
			t.Fatalf("duplicate original geometry object %d", surface.ObjectIndex)
		}
		seenGeometry[surface.ObjectIndex] = true
		if surface.IsShading {
			object, found := originalShading[surface.ObjectIndex]
			if !found || surface.Type != "Shading:Zone:Detailed" || !wantShading[surface.Name] || object.Fields[0].Value != surface.Name || object.Fields[1].Value != "FRONT-1" ||
				len(energyPathBaseboardSurfaceReferences(context, surface.Name, surface.ZoneName)) != 0 {
				t.Fatalf("overhang acquired an incorrect original identity or recipient qualification: %#v", surface)
			}
			delete(wantShading, surface.Name)
			continue
		}
		object, found := originalSurfaces[surface.ObjectIndex]
		if !found || surface.Type != "BuildingSurface:Detailed" || object.Fields[0].Value != surface.Name {
			t.Fatalf("convection surface lacks an exact original building-surface identity: %#v", surface)
		}
		convectionSurfaces = append(convectionSurfaces, surface)
	}
	if len(convectionSurfaces) != 40 || len(wantShading) != 0 {
		t.Fatalf("geometry lost original building/overhang members: convection=%d missingShading=%v", len(convectionSurfaces), wantShading)
	}
	want := map[string]struct {
		zone     string
		fraction float64
	}{
		"RIGHT-1": {"SPACE2-1", .3}, "C2-1": {"SPACE2-1", .1}, "SB25": {"SPACE2-1", .1}, "SB23": {"SPACE2-1", .1}, "SB21": {"SPACE2-1", .1},
		"LEFT-1": {"SPACE4-1", .3}, "C4-1": {"SPACE4-1", .1}, "SB45": {"SPACE4-1", .1}, "SB41": {"SPACE4-1", .1}, "SB43": {"SPACE4-1", .1},
	}
	var input []energyExplanationSeries
	var inputSources []EnergyDataSource
	for index, surface := range convectionSurfaces {
		value := float64(index%5 - 2) // Includes actual zero and both directions.
		sourceID := fmt.Sprintf("surface-%d", index)
		input = append(input, energyExplanationSeries{Level: "heat", Kind: "heat.surface_inside_face_convection", SurfaceScoped: true,
			sourceName: "Surface Inside Face Convection Heat Gain Energy", sourceKeyValue: surface.Name, Unit: "kWh", Total: value, SourceIDs: []string{sourceID},
			Monthly: map[int]float64{1: value}, Daily: map[int]float64{0: value}, Hourly: map[int]float64{0: value}, SelectedRange: value, HasSelectedRange: true})
		inputSources = append(inputSources, EnergyDataSource{ID: sourceID, SourceType: "sql_report_data", Name: "Surface Inside Face Convection Heat Gain Energy", KeyValue: surface.Name,
			InputSourceIDs: []string{"unchanged-input"}, Formula: "unchanged formula"})
	}
	plain := context
	plain.BaseboardTargets = nil
	baseline, baselineSources, baselineWarnings := prepareEnergyDriverSeries(input, append([]EnergyDataSource(nil), inputSources...), plain)
	got, sources, warnings := prepareEnergyDriverSeries(input, append([]EnergyDataSource(nil), inputSources...), context)
	if !reflect.DeepEqual(warnings, baselineWarnings) || len(got) != 40 || len(sources) != 40 {
		t.Fatal("qualification changed surface membership or geometry warnings")
	}
	qualified := 0
	for index, surface := range convectionSurfaces {
		expected, recipient := want[strings.ToUpper(surface.Name)]
		references := energyPathBaseboardSurfaceReferences(context, "  "+strings.ToLower(surface.Name)+"  ", surface.ZoneName)
		if (len(references) > 0) != recipient || energyPathRadiantSurfaceIsContext(context, surface.Name) {
			t.Fatalf("%q acquired an incorrect recipient or CF exclusion boundary", surface.Name)
		}
		if recipient {
			qualified++
			if len(references) != 1 || references[0].target.ZoneName != expected.zone || references[0].recipient.Fraction != expected.fraction || references[0].target.RadiantFraction != .2 {
				t.Fatalf("%q recipient provenance changed: %#v", surface.Name, references)
			}
			for _, text := range []string{energyPathBaseboardRecipientExplanation, references[0].target.Component.ID, fmt.Sprintf("surface %q [object %d]", surface.Name, references[0].recipient.SurfaceObjectIndex), fmt.Sprintf("recipient share of radiant output=%g", expected.fraction)} {
				if !strings.Contains(sources[index].Explanation, text) {
					t.Fatalf("%q missing source qualification %q: %s", surface.Name, text, sources[index].Explanation)
				}
			}
			if !strings.Contains(got[index].DriverExplanation, energyPathBaseboardRecipientExplanation) || !stringSliceContains(sources[index].RelatedEntityIDs, references[0].target.Component.ID) {
				t.Fatalf("%q lost recipient explanation or original parent trace", surface.Name)
			}
		} else if !reflect.DeepEqual(got[index], baseline[index]) || !reflect.DeepEqual(sources[index], baselineSources[index]) {
			t.Fatalf("nonrecipient/counterpart %q changed", surface.Name)
		}
		// Only these metadata fields may change. Every raw/effective/temporal
		// scalar, category, role, multiplier, source set and formula must match.
		got[index].DriverExplanation = baseline[index].DriverExplanation
		sources[index].Explanation = baselineSources[index].Explanation
		sources[index].RelatedEntityIDs = baselineSources[index].RelatedEntityIDs
		if !reflect.DeepEqual(got[index], baseline[index]) || !reflect.DeepEqual(sources[index], baselineSources[index]) {
			t.Fatalf("%q qualification changed numeric, source, or geometry contracts", surface.Name)
		}
	}
	if qualified != 10 || doc.String() != before {
		t.Fatalf("original recipient count/input changed: count=%d", qualified)
	}
}

func baseboardRecipientQualificationContext() energyDriverBuildContext {
	context := newEnergyDriverBuildContext(idf.GeometryReport{Surfaces: []idf.GeometrySurface{
		{ID: "recipient-wall", Name: "Recipient Wall", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Outdoors"},
		{ID: "peer-wall", Name: "Peer Wall", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Outdoors"},
		{ID: "counterpart", Name: "Counterpart", SurfaceType: "Wall", ZoneName: "Lab", OutsideBoundary: "Outdoors"},
	}})
	context.BaseboardTargets = []energyPathBaseboardTarget{{
		Component: idf.ComponentRef{ID: "component:20", ObjectIndex: 20, ObjectType: energyPathBaseboardElectricType, ObjectName: "Office Baseboard"},
		KeyValue:  "Office Baseboard", ZoneName: "Office", RadiantFraction: .2, PeopleFraction: .3,
		Recipients: []energyPathBaseboardRecipient{{SurfaceName: "Recipient Wall", SurfaceObjectIndex: 10, ZoneName: "Office", Fraction: .7}},
	}}
	return context
}

func TestEnergyPathBaseboardRecipientSQLPreservesCategoryDenominatorAndWireSources(t *testing.T) {
	context := baseboardRecipientQualificationContext()
	plain := context
	plain.BaseboardTargets = nil
	path := epath194SurfaceSQL(t, []string{"Recipient Wall", "Peer Wall", "Counterpart"})
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`ALTER TABLE Time ADD COLUMN Year INTEGER DEFAULT 2017`,
		`ALTER TABLE Time ADD COLUMN Interval REAL DEFAULT 44640`,
		`ALTER TABLE Time ADD COLUMN IntervalType INTEGER DEFAULT 3`,
		`ALTER TABLE Time ADD COLUMN EnvironmentPeriodIndex INTEGER DEFAULT 3`,
		`ALTER TABLE Time ADD COLUMN WarmupFlag INTEGER DEFAULT 0`,
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(3,3)`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if axis := energyPathObservedMonthlyTimeAxis(db); len(axis) != 1 || !axis[1] {
		t.Fatalf("fixture must prove one actual January weather-run Monthly interval: %#v", axis)
	}
	_ = db.Close()
	planValue := BuildPurposeRunPlan(parsePurposePlanFixture(t, "Version,25.1; Zone,Office; Zone,Lab;"), SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath,
	})
	plan := &planValue
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, plan, context)
	if err != nil {
		t.Fatal(err)
	}
	baselineParsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, plan, plain)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed, baselineParsed) {
		t.Fatal("recipient metadata changed streaming categories or monthly precision before preparation")
	}
	prepared, preparedSources, _ := prepareEnergyDriverSeries(parsed.Series, append([]EnergyDataSource(nil), parsed.Sources...), context)
	foundAggregate := false
	for _, item := range prepared {
		if item.parseCategoryAggregate && item.ZoneName == "Office" {
			foundAggregate = true
			if item.Total != -3 || item.Monthly[1] != -3 || item.RawMonthly[1] != -3 || item.heatSignMultiplier != -1 ||
				item.DriverSourceRole != energyDriverSourceRoleMainFlow || !reflect.DeepEqual(item.SourceIDs, []string{"sql-rdd-1", "sql-rdd-2"}) ||
				!strings.Contains(item.DriverExplanation, energyPathBaseboardRecipientAggregateExplanation) {
				t.Fatalf("recipient/peer category lost native total, sign, inputs, role or qualification: %#v", item)
			}
			// The private shadow retains the unrounded J-to-kWh multiply:
			// -3600000*(1/3600000) + -7200000*(1/3600000) is within one
			// floating-point step of -3, unlike the public 3dp period fields.
			// Its source sign is still native; the -1 Zone-air convention is
			// applied only during signed driver preparation/graph allocation.
			shadow := item.driverMonthlyShadow
			if shadow == nil || shadow.invalid || shadow.effective || !energyPathFinite(shadow.values[1]) || math.Abs(shadow.values[1]-(-3)) > 1e-12 {
				t.Fatalf("recipient/peer lost exact observed Monthly precision: shadow=%+v item=%#v", shadow, item)
			}
		}
	}
	if !foundAggregate || len(preparedSources) != len(parsed.Sources) {
		t.Fatal("recipient/peer category or original source membership changed")
	}
	got := UpgradeEnergyExplanationV1(buildEnergyExplanationResultWithDriverContext(parsed.Series, parsed.Sources, plan, context))
	baseline := UpgradeEnergyExplanationV1(buildEnergyExplanationResultWithDriverContext(baselineParsed.Series, baselineParsed.Sources, plan, plain))
	for _, phase := range []string{"native", "wire reload"} {
		if phase == "wire reload" {
			got, baseline = epath120Reload(t, got), epath120Reload(t, baseline)
		}
		nodes, _ := epath080Graph(t, got, "Office", "M1")
		wall := epath080NodeByID(nodes, epath080DriverNodeID(energyDriverCategoryExteriorWalls, "cooling", "Office"))
		if wall == nil || wall.EffectiveValue != 3 || wall.AllocatedValue != 100 || !stringSliceContains(wall.SourceIDs, "sql-rdd-1") || !stringSliceContains(wall.SourceIDs, "sql-rdd-2") {
			t.Fatalf("%s recipient was removed from the original 1+2=3 allocation denominator: %#v", phase, wall)
		}
		comparison := got
		comparison.Sources = append([]EnergyDataSource(nil), got.Sources...)
		for index := range comparison.Sources {
			source := &comparison.Sources[index]
			original := energyExplanationSourceByID(baseline.Sources, source.ID)
			if original == nil {
				t.Fatalf("%s created a source %q from recipient configuration", phase, source.ID)
			}
			if source.ID == "sql-rdd-1" {
				if !strings.Contains(source.Explanation, energyPathBaseboardRecipientExplanation) || !stringSliceContains(source.RelatedEntityIDs, "component:20") || source.RawValue != -1 || source.EffectiveValue != -1 || source.DriverRole != energyDriverSourceRoleMainFlow {
					t.Fatalf("%s recipient original observation/qualification lost: %#v", phase, source)
				}
				source.Explanation, source.RelatedEntityIDs = original.Explanation, original.RelatedEntityIDs
			} else if strings.Contains(source.Explanation, energyPathBaseboardRecipientExplanation) || stringSliceContains(source.RelatedEntityIDs, "component:20") {
				t.Fatalf("%s recipient identity leaked to nonrecipient/derived source %q", phase, source.ID)
			}
		}
		if !reflect.DeepEqual(comparison, baseline) {
			t.Fatalf("%s qualification changed graphs, allocations, formulas, source IDs, or other non-qualification metadata", phase)
		}
	}
}

func TestEnergyPathBaseboardRecipientQualificationIsNonAdditiveConfigurationOnly(t *testing.T) {
	context := baseboardRecipientQualificationContext()
	sources := []EnergyDataSource{
		{ID: "recipient", Name: "Surface Inside Face Convection Heat Gain Rate", KeyValue: " RECIPIENT WALL ", ZoneName: "Office"},
		{ID: "aggregate", Name: "Zone Air Heat Balance Surface Convection Rate", KeyValue: "Office", ZoneName: "Office"},
		{ID: "counterpart", Name: "Surface Inside Face Convection Heat Gain Energy", KeyValue: "Counterpart", ZoneName: "Lab"},
		{ID: "wrong-zone", Name: "Surface Inside Face Convection Heat Gain Energy", KeyValue: "Recipient Wall", ZoneName: "Lab"},
		{ID: "other-output", Name: "Surface Inside Face Conduction Heat Transfer Energy", KeyValue: "Recipient Wall", ZoneName: "Office"},
		{ID: "derived", SourceType: "derived_formula", Name: "Surface Inside Face Convection Heat Gain Energy", KeyValue: "Recipient Wall", ZoneName: "Office"},
	}
	baseline := append([]EnergyDataSource(nil), sources...)
	series := []energyExplanationSeries{
		{Level: "heat", Kind: "heat.surface_convection", ZoneName: "Office", DriverSourceRole: energyDriverSourceRoleReconciliation},
		{Level: "heat", Kind: "heat.surface_convection", ZoneName: "Lab", DriverSourceRole: energyDriverSourceRoleReconciliation},
	}
	applyEnergyPathBaseboardRecipientQualification(series, sources, context)
	if len(sources) != len(baseline) || !strings.Contains(series[0].DriverExplanation, energyPathBaseboardRecipientAggregateExplanation) || series[0].DriverSourceRole != energyDriverSourceRoleReconciliation || series[1].DriverExplanation != "" {
		t.Fatal("aggregate qualification changed roles, added observations, or crossed Zone ownership")
	}
	for index := range sources {
		if index < 2 {
			if !strings.Contains(sources[index].Explanation, "Non-additive recipient configuration:") || sources[index].RawValue != 0 || sources[index].EffectiveValue != 0 || sources[index].observedValuePresence != 0 || len(sources[index].InputSourceIDs) != 0 {
				t.Fatalf("missing native heating/radiation became an observed or additive quantity: %#v", sources[index])
			}
		} else if !reflect.DeepEqual(sources[index], baseline[index]) {
			t.Fatalf("unrelated source %q acquired recipient qualification", sources[index].ID)
		}
	}
	once := append([]EnergyDataSource(nil), sources...)
	applyEnergyPathBaseboardRecipientQualification(series, sources, context)
	if !reflect.DeepEqual(sources, once) {
		t.Fatal("repeated preparation duplicated recipient trace")
	}
	for _, fraction := range []float64{0, -1, math.NaN(), math.Inf(1), 1.1} {
		invalid := baseboardRecipientQualificationContext()
		invalid.BaseboardTargets[0].RadiantFraction = fraction
		if len(energyPathBaseboardSurfaceReferences(invalid, "Recipient Wall", "Office")) != 0 {
			t.Fatalf("invalid/zero radiant fraction %g supplied a recipient", fraction)
		}
		invalid = baseboardRecipientQualificationContext()
		invalid.BaseboardTargets[0].Recipients[0].Fraction = fraction
		if len(energyPathBaseboardSurfaceReferences(invalid, "Recipient Wall", "Office")) != 0 {
			t.Fatalf("invalid/zero recipient fraction %g supplied a recipient", fraction)
		}
	}
	context.Enabled = false
	if len(energyPathBaseboardSurfaceReferences(context, "Recipient Wall", "Office")) != 0 {
		t.Fatal("disabled context supplied a recipient")
	}
}
