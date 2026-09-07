package simulation

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEPATH194SurfaceSourcesUseActualAnalyzedTopology(t *testing.T) {
	doc, err := idf.Parse(epath194SurfaceIDF)
	if err != nil {
		t.Fatal(err)
	}
	geometry := idf.AnalyzeGeometry(doc)
	context := newEnergyDriverBuildContext(geometry, doc)
	cases := []struct {
		name, category, boundary, openingType string
	}{
		{"Office Wall", energyDriverCategoryExteriorWalls, "exterior", ""},
		{"Office Roof", energyDriverCategoryRoofs, "exterior", ""},
		{"Office Floor", energyDriverCategoryGroundFloors, "ground", ""},
		{"Office Partition", energyDriverCategoryInterzoneSurfaces, "interzone_explicit_surface", ""},
		{"Office Adiabatic", energyDriverCategoryStorageOther, "adiabatic_explicit", ""},
		{"Other Side Wall", energyDriverCategoryExteriorWalls, "other_side_coefficients", ""},
		{"Other Side Roof", energyDriverCategoryRoofs, "other_side_conditions_model", ""},
		{"Other Side Floor", energyDriverCategoryGroundFloors, "other_side_coefficients", ""},
		{"Office Window", energyDriverCategoryWindowsDoors, "exterior", "Window"},
		{"Office Door", energyDriverCategoryWindowsDoors, "exterior", "Door"},
		{"Office Glass Door", energyDriverCategoryWindowsDoors, "exterior", "GlassDoor"},
		{"Office Mass", energyDriverCategoryStorageOther, "internal_mass", ""},
		{"Missing SQL Surface", energyDriverCategoryStorageOther, "", ""},
	}
	keys := make([]string, len(cases))
	identities := make(map[string][]string, len(cases))
	for index, test := range cases {
		keys[index] = test.name
		t.Run(test.name+" analyzed identity", func(t *testing.T) {
			category, warning := context.SurfaceCategories.resolve(test.name)
			if category.Category != test.category {
				t.Fatalf("actual source category = %q, want %q", category.Category, test.category)
			}
			if test.name == "Missing SQL Surface" {
				if warning == nil || warning.Code != "energy_driver_surface_unresolved" || category.ZoneName != "" {
					t.Fatalf("unresolved source borrowed a real owner/category: %#v / %#v", category, warning)
				}
				return
			}
			if warning != nil || category.ZoneName != "Office" || category.BoundaryKind != test.boundary {
				t.Fatalf("actual surface context = %#v / %#v", category, warning)
			}
			folded, foldedWarning := context.SurfaceCategories.resolve("  " + strings.ToUpper(test.name) + "  ")
			if foldedWarning != nil || folded.SurfaceID != category.SurfaceID || folded.EntityID != category.EntityID {
				t.Fatalf("SQL case folding changed actual surface identity: %#v / %#v", category, folded)
			}
			var expected []string
			if test.openingType != "" {
				var window *idf.GeometryWindow
				var opening *idf.ThermalOpeningRecord
				for index := range geometry.Windows {
					if geometry.Windows[index].Name == test.name {
						window = &geometry.Windows[index]
					}
				}
				for index := range geometry.Topology.Openings {
					if geometry.Topology.Openings[index].Name == test.name {
						opening = &geometry.Topology.Openings[index]
					}
				}
				if window == nil || opening == nil || window.SurfaceType != test.openingType || opening.SurfaceType != test.openingType || opening.WindowID != window.ID || opening.EntityID == "" || window.BaseSurfaceID == "" {
					t.Fatalf("actual %s geometry/opening bridge missing: window=%#v opening=%#v", test.openingType, window, opening)
				}
				if category.SurfaceID != window.ID || category.EntityID != opening.EntityID {
					t.Fatalf("%s source resolves another opening: %#v", test.openingType, category)
				}
				expected = []string{window.ID, opening.EntityID, window.BaseSurfaceID}
				connections := 0
				for _, connection := range geometry.Topology.Connections {
					if stringSliceContains(connection.OpeningIDs, opening.ID) {
						expected = append(expected, connection.ID)
						connections++
					}
				}
				if connections != 1 {
					t.Fatalf("%s must belong to one actual opening connection, got %d", test.name, connections)
				}
			} else if test.name == "Office Mass" {
				massID := ""
				for _, object := range doc.Objects {
					if strings.EqualFold(object.Type, "InternalMass") && object.Fields[0].Value == test.name {
						massID = fmt.Sprintf("internal-mass-%d", object.Index)
					}
				}
				if massID == "" || category.SurfaceID != massID || category.EntityID != massID {
					t.Fatalf("InternalMass must use actual parsed object identity, not an invented topology surface: %#v", category)
				}
				expected = []string{massID}
			} else {
				var boundary *idf.ThermalBoundaryRecord
				for index := range geometry.Topology.Boundaries {
					if geometry.Topology.Boundaries[index].SurfaceName == test.name {
						boundary = &geometry.Topology.Boundaries[index]
					}
				}
				if boundary == nil || boundary.RelationKind != test.boundary || boundary.SurfaceEntityID == "" || category.SurfaceID != boundary.SurfaceID || category.EntityID != boundary.SurfaceEntityID {
					t.Fatalf("actual analyzed boundary not linked to source category: %#v / %#v", boundary, category)
				}
				expected = []string{boundary.SurfaceID, boundary.SurfaceEntityID}
				for _, connection := range geometry.Topology.Connections {
					if stringSliceContains(connection.BoundaryIDs, boundary.ID) {
						expected = append(expected, connection.ID)
					}
				}
				if len(expected) != 3 {
					t.Fatalf("%s must map one actual boundary connection, got %#v", test.name, expected)
				}
			}
			for _, id := range expected {
				if !stringSliceContains(category.RelatedEntityIDs, id) {
					t.Errorf("source %s lacks actual topology/geometry identity %s: %#v", test.name, id, category.RelatedEntityIDs)
				}
			}
			identities[test.name] = expected
		})
	}
	path := epath194SurfaceSQL(t, keys)
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}, context)
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(legacy)
	for _, phase := range []string{"generated", "JSON reload"} {
		if phase != "generated" {
			result = epath120Reload(t, result)
		}
		t.Run(phase+" canonical sources", func(t *testing.T) {
			for index, test := range cases {
				id := fmt.Sprintf("sql-rdd-%d", index+1)
				source := energyExplanationSourceByID(result.Sources, id)
				if source == nil || source.KeyValue != test.name || source.DriverCategory != test.category || source.RawValue != -float64(index+1) || source.SourceUnit != "J" || source.NormalizedUnit != "kWh" {
					t.Errorf("physical source %s category/raw/unit changed or merged: %#v", test.name, source)
					continue
				}
				count := 0
				for _, candidate := range result.Sources {
					if candidate.ID == id {
						count++
					}
				}
				if count != 1 {
					t.Errorf("physical source %s occurs %d times, want once", test.name, count)
				}
				for _, identity := range identities[test.name] {
					if !stringSliceContains(source.RelatedEntityIDs, identity) {
						t.Errorf("canonical source %s lost actual related identity %s", test.name, identity)
					}
				}
				if test.name != "Missing SQL Surface" && source.ZoneName != "Office" {
					t.Errorf("known surface %s lost actual Office owner: %q", test.name, source.ZoneName)
				}
			}
			unresolved := 0
			for _, warning := range result.Warnings {
				if warning.Code == "energy_driver_surface_unresolved" {
					unresolved++
					if !strings.Contains(warning.Message, "Missing SQL Surface") {
						t.Errorf("real resolvable surface produced unresolved warning: %#v", warning)
					}
				}
			}
			if unresolved == 0 {
				t.Error("unresolvable SQL source must retain an explicit warning, not silently disappear")
			}
		})
	}
}

func epath194SurfaceSQL(t *testing.T, keys []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, statement := range []string{
		`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY, KeyValue TEXT, Name TEXT, Units TEXT, IsMeter INTEGER, ReportingFrequency TEXT, IndexGroup TEXT)`,
		`CREATE TABLE "Time" (TimeIndex INTEGER PRIMARY KEY, Month INTEGER, Day INTEGER, Hour INTEGER, Minute INTEGER)`,
		`CREATE TABLE ReportData (ReportDataIndex INTEGER PRIMARY KEY, TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`,
		`INSERT INTO "Time" VALUES (1, 1, 31, 24, 0)`,
		`INSERT INTO ReportDataDictionary VALUES (100, 'Office', 'Zone Air System Sensible Cooling Energy', 'J', 0, 'Monthly', 'Zone')`,
		`INSERT INTO ReportData VALUES (100, 1, 100, 360000000)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for index, name := range keys {
		if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES (?, ?, 'Surface Inside Face Convection Heat Gain Energy', 'J', 0, 'Monthly', 'Surface')`, index+1, name); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO ReportData VALUES (?, 1, ?, ?)`, index+1, index+1, -float64(index+1)*3_600_000); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

const epath194SurfaceIDF = `Version, 24.2;
Zone, Office, 0,0,0,0,1,1;
Zone, Lab, 0,0,0,0,1,1;
Material:NoMass, Insulation, Rough, 2;
Construction, Wall Construction, Insulation;
WindowMaterial:SimpleGlazingSystem, Glass, 2.5, 0.5, 0.6;
Construction, Glass Construction, Glass;
SurfaceProperty:OtherSideCoefficients, OSC, 0, 10, 1, 0, 0, 0, 0, 0;
SurfaceProperty:OtherSideConditionsModel, OSCM, GapConvectionRadiation;
BuildingSurface:Detailed, Office Wall, Wall, Wall Construction, Office, , Outdoors, , SunExposed, WindExposed, 0.5, 4,
  0,0,0, 0,0,3, 10,0,3, 10,0,0;
BuildingSurface:Detailed, Office Roof, Roof, Wall Construction, Office, , Outdoors, , SunExposed, WindExposed, 0.5, 4,
  0,0,3, 0,4,3, 10,4,3, 10,0,3;
BuildingSurface:Detailed, Office Floor, Floor, Wall Construction, Office, , Ground, , NoSun, NoWind, 0.5, 4,
  0,0,0, 10,0,0, 10,4,0, 0,4,0;
BuildingSurface:Detailed, Office Partition, Wall, Wall Construction, Office, , Surface, Lab Partition, NoSun, NoWind, 0.5, 4,
  10,0,0, 10,0,3, 10,4,3, 10,4,0;
BuildingSurface:Detailed, Lab Partition, Wall, Wall Construction, Lab, , Surface, Office Partition, NoSun, NoWind, 0.5, 4,
  10,4,0, 10,4,3, 10,0,3, 10,0,0;
BuildingSurface:Detailed, Office Adiabatic, Wall, Wall Construction, Office, , Adiabatic, , NoSun, NoWind, 0.5, 4,
  0,4,0, 0,4,3, 0,0,3, 0,0,0;
BuildingSurface:Detailed, Other Side Wall, Wall, Wall Construction, Office, , OtherSideCoefficients, OSC, NoSun, NoWind, 0.5, 4,
  0,4,0, 10,4,0, 10,4,3, 0,4,3;
BuildingSurface:Detailed, Other Side Roof, Roof, Wall Construction, Office, , OtherSideConditionsModel, OSCM, NoSun, NoWind, 0.5, 4,
  0,5,3, 0,6,3, 2,6,3, 2,5,3;
BuildingSurface:Detailed, Other Side Floor, Floor, Wall Construction, Office, , OtherSideCoefficients, OSC, NoSun, NoWind, 0.5, 4,
  0,5,0, 2,5,0, 2,6,0, 0,6,0;
FenestrationSurface:Detailed, Office Window, Window, Glass Construction, Office Wall, , 0.5, , 1, 4,
  1,0,1, 1,0,2, 2,0,2, 2,0,1;
FenestrationSurface:Detailed, Office Door, Door, Wall Construction, Office Wall, , 0.5, , 1, 4,
  3,0,0, 3,0,2, 4,0,2, 4,0,0;
FenestrationSurface:Detailed, Office Glass Door, GlassDoor, Glass Construction, Office Wall, , 0.5, , 1, 4,
  5,0,0, 5,0,2, 6,0,2, 6,0,0;
InternalMass, Office Mass, Wall Construction, Office, , 20;
`
