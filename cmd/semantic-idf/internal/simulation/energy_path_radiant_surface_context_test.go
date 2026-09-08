package simulation

import (
	"database/sql"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func radiantSurfaceContextDocument(t *testing.T) idf.Document {
	t.Helper()
	doc := energyPathRadiantHandDocument(t)
	directHVACFixtureObject(t, &doc, "Zone", "Office").Fields[6].Value = "1"
	return parsePurposePlanFixture(t, doc.String()+`
BuildingSurface:Detailed,Passive Floor,Floor,Slab,Office,,Ground,,NoSun,NoWind,1,3,2,0,0,3,0,0,2,1,0;
`)
}

func TestEnergyPathRadiantSurfaceContextExactOriginalBoundary(t *testing.T) {
	doc := energyPathRadiantDocument(t)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	if len(context.RadiantLoads) != 3 {
		t.Fatalf("original radiant surface ownership missing: %#v", context.RadiantLoads)
	}
	active := map[string]string{"zn001:flr001": "West Zone", "zn002:flr001": "EAST ZONE", "zn003:flr001": "NORTH ZONE"}
	for _, surface := range idf.AnalyzeGeometry(doc).Surfaces {
		zone, want := active[strings.ToLower(surface.Name)]
		target, got := energyPathRadiantSurfaceTarget(context, "  "+strings.ToUpper(surface.Name)+"  ")
		if got != want || got && (target.ZoneName != zone || !strings.EqualFold(target.SurfaceName, surface.Name)) {
			t.Errorf("surface %q context=%v (%#v), want exact active owner %q", surface.Name, got, target, zone)
		}
	}
	for _, key := range []string{"", "Floor", "West Zone", "West Zone Radiant Floor", "Zn001:Flr001 neighbour"} {
		if energyPathRadiantSurfaceIsContext(context, key) {
			t.Errorf("non-surface identity %q acquired active-surface status", key)
		}
	}
}

func TestEnergyPathRadiantSurfaceContextMixedPassiveAndNoStorageInference(t *testing.T) {
	doc := radiantSurfaceContextDocument(t)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	series := energyDriverMonthlyFixtureSeries([]energyExplanationSeries{
		{Level: "heat", Kind: "heat.surface_inside_face_convection", SurfaceScoped: true, sourceName: "Surface Inside Face Convection Heat Gain Energy", sourceKeyValue: "Floor", Unit: "kWh", Total: 7, SourceIDs: []string{"active"}},
		{Level: "heat", Kind: "heat.surface_inside_face_convection", SurfaceScoped: true, sourceName: "Surface Inside Face Convection Heat Gain Energy", sourceKeyValue: "Passive Floor", Unit: "kWh", Total: 3, SourceIDs: []string{"passive"}},
		{Level: "heat", Kind: "heat.surface_convection", sourceName: "Zone Air Heat Balance Surface Convection Rate", ZoneName: "Office", Unit: "kWh", Total: 10, SourceIDs: []string{"aggregate"}},
		{Level: "heat", Kind: "heat.surface_inside_face_convection", SurfaceScoped: true, sourceName: "Surface Inside Face Convection Heat Gain Energy", sourceKeyValue: "Other Floor", Unit: "kWh", Total: 2, SourceIDs: []string{"other"}},
		{Level: "heat", Kind: "heat.surface_convection", sourceName: "Zone Air Heat Balance Surface Convection Rate", ZoneName: "Other", Unit: "kWh", Total: 4, SourceIDs: []string{"other-aggregate"}},
	})
	inputSources := []EnergyDataSource{{ID: "active"}, {ID: "passive"}, {ID: "aggregate"}, {ID: "other"}, {ID: "other-aggregate"}}
	prepared, sources, _ := prepareEnergyDriverSeries(series, append([]EnergyDataSource(nil), inputSources...), context)
	for _, index := range []int{0, 2} {
		if prepared[index].DriverSourceRole != energyDriverSourceRoleContext || sources[index].DriverRole != energyDriverSourceRoleContext || sources[index].InspectorSection != energyDriverInspectorSectionContext ||
			prepared[index].Total != series[index].Total || !reflect.DeepEqual(prepared[index].Monthly, series[index].Monthly) || sources[index].RawValue != series[index].Total || sources[index].ZoneName != "Office" {
			t.Fatalf("active thermal observation lost context/value/Zone: %#v / %#v", prepared[index], sources[index])
		}
	}
	if prepared[0].DriverCategory != energyDriverCategoryGroundFloors || prepared[1].DriverCategory != energyDriverCategoryGroundFloors || prepared[1].DriverSourceRole != energyDriverSourceRoleMainFlow || prepared[3].DriverSourceRole != energyDriverSourceRoleMainFlow {
		t.Fatal("active surface suppressed a passive peer in the same category or another Zone")
	}
	category, _ := context.SurfaceCategories.resolve("Floor")
	if len(category.RelatedEntityIDs) == 0 || !reflect.DeepEqual(sources[0].RelatedEntityIDs, category.RelatedEntityIDs) {
		t.Fatalf("active raw source lost original surface/connection trace: %#v", sources[0])
	}
	final, derived := appendEnergyDriverReconciliationComponents(prepared, sources)
	foundOther := false
	for _, item := range final {
		if item.Kind == "heat.surface_reconciliation" {
			if item.ZoneName == "Office" {
				t.Fatalf("excluded active emission was inferred as storage: %#v", item)
			}
			foundOther = item.ZoneName == "Other" && item.Total == 2
		}
	}
	if !foundOther {
		t.Fatal("unrelated Zone surface reconciliation changed")
	}
	for _, source := range derived {
		if source.SourceType == "derived_formula" && (stringSliceContains(source.InputSourceIDs, "active") || stringSliceContains(source.InputSourceIDs, "aggregate")) {
			t.Fatalf("active context leaked into derived passive/storage provenance: %#v", source)
		}
	}
	rows, _ := appendEnergyDriverPeriodAccounting("M1", prepared, func(item energyExplanationSeries) float64 { return item.Monthly[1] }, nil, nil)
	for _, row := range rows {
		if row.ZoneName == "Office" && strings.HasPrefix(row.ID, "reconcile.driver.surface.") {
			t.Fatalf("active aggregate was compared against a passive-only denominator: %#v", row)
		}
	}
	// With no validated target, preparation must remain exactly the old path.
	plain := context
	plain.RadiantLoads = nil
	baseline, baselineSources, baselineWarnings := prepareEnergyDriverSeries(series, append([]EnergyDataSource(nil), inputSources...), plain)
	geometryOnly := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc))
	got, gotSources, gotWarnings := prepareEnergyDriverSeries(series, append([]EnergyDataSource(nil), inputSources...), geometryOnly)
	if !reflect.DeepEqual(got, baseline) || !reflect.DeepEqual(gotSources, baselineSources) || !reflect.DeepEqual(gotWarnings, baselineWarnings) {
		t.Fatal("documents without radiant targets changed values or metadata")
	}
}

func TestEnergyPathRadiantSurfaceContextSQLDoesNotPoisonPassiveCategory(t *testing.T) {
	for _, missingActive := range []bool{false, true} {
		name := "observed active and passive"
		if missingActive {
			name = "missing active does not invalidate passive precision"
		}
		t.Run(name, func(t *testing.T) {
			doc := radiantSurfaceContextDocument(t)
			context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
			path := epath194SurfaceSQL(t, []string{"Floor", "Passive Floor"})
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			for _, query := range []string{
				// The older surface helper has a minimal legacy Time table.
				// Add actual weather/Monthly-axis evidence for the precision
				// assertion; an unknown calendar must correctly stay invalid.
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
			if missingActive {
				if _, err := db.Exec(`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=1`); err != nil {
					t.Fatal(err)
				}
			}
			_ = db.Close()
			plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
			parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, context)
			if err != nil {
				t.Fatal(err)
			}
			aggregates := 0
			for _, item := range parsed.Series {
				if item.parseCategoryAggregate {
					aggregates++
					if item.Total != -2 || !reflect.DeepEqual(item.SourceIDs, []string{"sql-rdd-2"}) || item.driverMonthlyShadow == nil || item.driverMonthlyShadow.invalid || math.Abs(item.driverMonthlyShadow.values[1]-(-2)) > 1e-12 {
						t.Fatalf("active source merged into passive category or invalidated its observed precision: item=%#v shadow=%#v", item, item.driverMonthlyShadow)
					}
				}
			}
			if aggregates != 1 {
				t.Fatalf("passive category count=%d, want 1", aggregates)
			}
			result := UpgradeEnergyExplanationV1(buildEnergyExplanationResultWithDriverContext(parsed.Series, parsed.Sources, &plan, context))
			for _, phase := range []string{"native", "wire reload"} {
				if phase == "wire reload" {
					result = epath120Reload(t, result)
				}
				for _, period := range []string{"M1", "annual"} {
					nodes, links := epath080Graph(t, result, "Office", period)
					// Inside-face convection uses the catalog's source-to-Zone
					// sign -1: the fixture's raw -2 is cooling pressure +2.
					passive := epath080NodeByID(nodes, epath080DriverNodeID(energyDriverCategoryGroundFloors, "cooling", "Office"))
					if passive == nil || !stringSliceContains(passive.SourceIDs, "sql-rdd-2") || stringSliceContains(passive.SourceIDs, "sql-rdd-1") || passive.EffectiveValue != 2 {
						t.Fatalf("%s/%s passive floor was lost or contaminated: %#v", phase, period, passive)
					}
					for _, link := range links {
						if link.Relation == "driver_to_load" && stringSliceContains(link.SourceIDs, "sql-rdd-1") {
							t.Fatalf("active thermal context entered primary allocation: %#v", link)
						}
					}
				}
				active := energyExplanationSourceByID(result.Sources, "sql-rdd-1")
				if !missingActive && (active == nil || active.RawValue != -1 || active.EffectiveValue != -1 || active.ZoneName != "Office" || active.DriverRole != energyDriverSourceRoleContext || active.InspectorSection != energyDriverInspectorSectionContext || len(active.RelatedEntityIDs) == 0) {
					t.Fatalf("%s active raw/source metadata lost: %#v", phase, active)
				}
			}
		})
	}
}
