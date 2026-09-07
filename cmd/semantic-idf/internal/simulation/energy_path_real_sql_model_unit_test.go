package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func epathSQLModelUnitFixture(t *testing.T) (string, epathRealSQLModel) {
	t.Helper()
	path := epathOracleUnitSQL(t)
	epathOracleEditSQL(t, path, `CREATE TABLE Zones(ZoneIndex INTEGER,ZoneName TEXT,Multiplier REAL,ListMultiplier REAL);
INSERT INTO Zones VALUES(1,'Office',2,3);
CREATE TABLE Surfaces(SurfaceIndex INTEGER,SurfaceName TEXT,ClassName TEXT,ZoneIndex INTEGER,ExtBoundCond INTEGER,HeatTransferSurf INTEGER);
INSERT INTO Surfaces VALUES(1,'Wall A','Wall',1,0,1),(2,'Wall B','Wall',1,0,1);
INSERT INTO ReportDataDictionary VALUES(10,'Sensible Cooling','Office',0,'Monthly','J','Zone'),(11,'Sensible Heating','Office',0,'Monthly','J','Zone'),(12,'Surface Exchange','Wall A',0,'Monthly','J','Surface'),(13,'Surface Exchange','Wall B',0,'Monthly','J','Surface'),(14,'People Gain','Office',0,'Monthly','J','Zone')`)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for month := 1; month <= 12; month++ {
		for id, value := range map[int]float64{10: 3, 11: 0, 12: -10, 13: 6, 14: 1} {
			if _, err := tx.Exec(`INSERT INTO ReportData VALUES(?,?,?,?)`, 10000+id*20+month, id, month, value*3600000); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	db.Close()
	selector := func(name string, keys ...string) epathRealSQLSelector {
		return epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: name, Unit: "J"}}, Keys: keys}
	}
	model := epathRealSQLModel{Schema: "semantic-idf.energy-path-sql-model/large-office-monthly/v1", Surface: epathRealSQLSurfaceModel{Source: selector("Surface Exchange", "Wall A", "Wall B"), Sign: -1, Mapping: "surface_class_boundary/v1"}, Families: []epathRealSQLFamily{{ID: "people", Keys: []string{"Office"}, Category: "internal.people", Component: "sensible", Terms: []epathRealSQLTerm{{Source: selector("People Gain", "Office"), Sign: 1}}, Role: "pressure", BuildingVisible: true}}, Loads: []epathRealSQLLoad{{Service: "cooling", Component: "sensible", Source: selector("Sensible Cooling", "Office")}, {Service: "heating", Component: "sensible", Source: selector("Sensible Heating", "Office")}}, Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}}
	return path, model
}

func TestEnergyPathRealSQLModelReviewedRecipeFrames(t *testing.T) {
	path := os.Getenv("EPATH_REAL_SQL_MODEL_SQL")
	if path == "" {
		t.Skip("reviewed LargeOffice frame observation requires EPATH_REAL_SQL_MODEL_SQL; not acceptance")
	}
	_, catalog := epathRealDirectories(t)
	recipe, err := epathLoadRealOracleRecipe(filepath.Join(catalog, "oracles", "large-office-25-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if recipe.SQLModel == nil {
		t.Fatal("actual reviewed sqlModel required")
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, *recipe.SQLModel)
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range []string{"cooling", "heating"} {
		total := epathSQLQuantity{}
		for zone := range frames.Zones {
			for month := 1; month <= 12; month++ {
				total = total.add(frames.Loads[epathSQLKey(zone, service, month)])
			}
		}
		t.Logf("INDEPENDENT SQL FRAME ONLY, NOT ACCEPTANCE: %s load %.12g kWh, propagated3dp budget %.12g", service, total.Value, total.Error)
	}
	t.Logf("INDEPENDENT SQL FRAMES ONLY: Zones=%d signed monthly cells=%d; no expected file written", len(frames.Zones), len(frames.Cells))
}

func TestEnergyPathRealSQLModelSurfaceNetBeforePressureAndMultiplierOnce(t *testing.T) {
	path, model := epathSQLModelUnitFixture(t)
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	var annualSurface, annualLoad float64
	for month := 1; month <= 12; month++ {
		cell := frames.Cells[epathSQLKey("Office", "surface:surface.exterior_walls", month)]
		if cell == nil || cell.Raw.Value != 4 || cell.Effective.Value != 24 {
			t.Fatalf("surface rows must net10-6 before pressure, multiplier2*3 once: %#v", cell)
		}
		load := frames.Loads[epathSQLKey("Office", "cooling", month)]
		if load.Value != 18 {
			t.Fatal("Zone load multiplier double-applied or omitted")
		}
		allocated := cell.Allocated["cooling"]
		if math.Abs(allocated.Value-14.4) > 1e-12 {
			t.Fatalf("surface allocation must18*(24/(24+6)), not clipped-source pressure: %#v", allocated)
		}
		if allocated.Error <= 0 || allocated.Error > .1 {
			t.Fatalf("unbounded or absent propagation budget: %#v", allocated)
		}
		annualSurface += allocated.Value
		annualLoad += load.Value
	}
	if math.Abs(annualSurface-172.8) > 1e-10 || annualLoad != 216 {
		t.Fatal("annual is not a sum of completed monthly allocations")
	}
	for _, test := range []struct {
		class           string
		boundary, index int
		want            string
	}{{"Internal Mass", 7, 7, "balance.storage_other"}, {"Wall", -1, 9, "balance.storage_other"}, {"Floor", -1, 10, "surface.ground_floors"}, {"Roof", 3, 4, "surface.interzone"}, {"Window", 0, 5, "surface.windows_doors"}} {
		got, err := epathSQLSurfaceCategory(test.class, test.boundary, test.index)
		if err != nil || got != test.want {
			t.Fatalf("surface classification %v: %s %v", test, got, err)
		}
	}
	if _, err := epathSQLSurfaceCategory("Roof", -9, 1); err == nil {
		t.Fatal("unreviewed boundary silently classified")
	}
}

func TestEnergyPathRealSQLModelMissingKeyAndRemainderSemantics(t *testing.T) {
	path, model := epathSQLModelUnitFixture(t)
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	model.Families = append(model.Families, epathRealSQLFamily{ID: "surface-check", Keys: []string{"Office"}, Category: "balance.storage_other", Component: "sensible", Terms: []epathRealSQLTerm{{Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "People Gain", Unit: "J"}}, Keys: []string{"Office"}}, Sign: 1}}, Subtract: []string{"surface.total"}, Role: "context"})
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	if cell := frames.Cells[epathSQLKey("Office", "surface-check", 1)]; cell.Raw.Value != -3 || len(cell.Allocated) != 0 {
		t.Fatalf("context remainder was allocated or subtracted clipped pressure: %#v", cell)
	}
	model.Families[0].Terms[0].Source.Keys = []string{"Office", "Missing expected key"}
	if _, err := epathCompileSQLModelFrames(path, observed.Sources, model); err == nil {
		t.Fatal("missing expected source key became zero")
	}
	for _, value := range []float64{0, -1, math.NaN()} {
		if _, err := epathSQLShare(epathSQLQuantity{Value: 1}, epathSQLQuantity{Value: 1}, epathSQLQuantity{Value: value}, model.Precision); err == nil {
			t.Errorf("invalid denominator %s accepted", fmt.Sprint(value))
		}
	}
}
