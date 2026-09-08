package simulation

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func initialCompactSQLFixture(t *testing.T, timeType string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY, KeyValue TEXT, Name TEXT, Units TEXT, ReportingFrequency TEXT, IsMeter INTEGER, IndexGroup TEXT)`,
		`CREATE TABLE Time (TimeIndex ` + timeType + ` PRIMARY KEY, Month INTEGER, Day INTEGER, Hour INTEGER, Minute INTEGER, IntervalType INTEGER)`,
		`CREATE TABLE ReportData (TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`,
		`INSERT INTO ReportDataDictionary VALUES
(0,'Absent','Unreported lower index','C','Hourly',0,'Zone'),
(1,'Office','Zone Mean Air Temperature','C','Hourly',0,'Zone'),
(2,'Office','Zone Air Heat Balance Internal Convective Heat Gain Rate','W','Hourly',0,'Zone'),
(3,'Lab','Zone Air Heat Balance Surface Convection Rate','W','Hourly',0,'Zone'),
(4,'Office','Zone Mean Air Temperature','C','Monthly',0,'Zone'),
(5,'Electricity:Facility','Electricity:Facility','J','Hourly',1,'Facility'),
(6,'Electricity:Facility','Electricity:Facility','J','Monthly',1,'Facility'),
(7,'Sparse','Unknown optional metadata','C',NULL,NULL,NULL)`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	insertTime, err := tx.Prepare(`INSERT INTO Time VALUES (?,?,?,?,0,1)`)
	if err != nil {
		t.Fatal(err)
	}
	defer insertTime.Close()
	insertValue, err := tx.Prepare(`INSERT INTO ReportData VALUES (?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer insertValue.Close()
	for frame := 1; frame <= 1502; frame++ {
		date := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(frame-1) * time.Hour)
		if _, err := insertTime.Exec(frame, int(date.Month()), date.Day(), date.Hour()+1); err != nil {
			t.Fatal(err)
		}
		for _, observation := range []struct {
			id    int
			value any
		}{
			{1, 20 + float64(frame)/1000}, {2, float64(frame - 2)}, {3, float64(-frame)}, {5, 3600000.0},
		} {
			if observation.id == 2 && frame == 4 || observation.id == 5 && frame == 2 {
				observation.value = nil
			}
			if _, err := insertValue.Exec(frame, observation.id, observation.value); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, statement := range []string{
		`INSERT INTO Time VALUES (1503,3,31,24,0,3)`,
		`INSERT INTO ReportData VALUES (1503,4,99),(1503,6,0),(1503,6,NULL),(1,2,999),(1,7,0)`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for id := 10; id <= 269; id++ {
		if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES (?,?,?,'C','Hourly',0,'Zone')`, id, "Filler", fmt.Sprintf("Filler variable %d", id)); err != nil {
			t.Fatal(err)
		}
		if _, err := insertValue.Exec(1, id, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInitialCompactSQLMatchesOriginalWalkerAcrossSamplingAndCaps(t *testing.T) {
	// Changing only Time's declared affinity forces the preserved original
	// walker. Its canonical decimal IDs still join exactly, while ordering is
	// controlled by INTEGER ReportData.TimeIndex on both databases. This tests
	// whole parsers against the actual original implementation, not a mock or
	// another implementation of statistics/downsampling/heat-flow assembly.
	fastPath := initialCompactSQLFixture(t, "INTEGER")
	originalPath := initialCompactSQLFixture(t, "TEXT")
	for _, fixture := range []struct {
		path string
		fast bool
	}{{fastPath, true}, {originalPath, false}} {
		db, err := openSimulationSQLiteReadOnly(fixture.path)
		if err != nil {
			t.Fatal(err)
		}
		fast := compactReportDataIntegerKeyAffinity(db, "Time", []string{"TimeIndex"})
		db.Close()
		if fast != fixture.fast {
			t.Fatal("fixture did not exercise both compact and original walkers")
		}
	}
	plan := PurposeRunPlan{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy, SimulationPurposeZoneHeatFlow}}
	want, err := parseSimulationSQLWithContext(context.Background(), originalPath, plan)
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseSimulationSQLWithContext(context.Background(), fastPath, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("optimized complete SQL parser output differs from original: series=%v energy=%v heatFlow=%v",
			reflect.DeepEqual(got.Series, want.Series), reflect.DeepEqual(got.Energy, want.Energy), reflect.DeepEqual(got.HeatFlow, want.HeatFlow))
	}
	if len(got.Series) != 256 || got.Series[0].SourceID != "sql-rdd-1" || got.Series[len(got.Series)-1].SourceID != "sql-rdd-258" {
		t.Fatal("first 256 observed dictionaries, including zero-only observations, were not preserved")
	}
	byID := map[string]SimulationSeries{}
	for _, series := range got.Series {
		byID[series.SourceID] = series
	}
	hourly, monthly := byID["sql-rdd-1"], byID["sql-rdd-4"]
	if hourly.ReportingFrequency != "Hourly" || monthly.ReportingFrequency != "Monthly" || hourly.Column != monthly.Column || hourly.RowCount != 1503 || monthly.RowCount != 1503 {
		t.Fatal("mixed frequencies or observed frame ordinals changed")
	}
	if len(hourly.Points) != 1200 || hourly.Points[0].X != 1 || hourly.Points[1199].X != 1502 || hourly.Points[0].Label != "01-01 01:00" || hourly.Points[1199].Label != "03-04 14:00" || math.Abs(hourly.Min-20.001) > 1e-12 || math.Abs(hourly.Max-21.502) > 1e-12 {
		t.Fatal("series cap, first/last points, labels or statistics changed")
	}
	if sparse := byID["sql-rdd-7"]; sparse.IsMeter != nil || sparse.ReportingFrequency != "" || len(sparse.Points) != 1 || sparse.Points[0].Value != 0 {
		t.Fatal("unknown optional metadata or explicit zero changed")
	}
	if meter := byID["sql-rdd-6"]; len(meter.Points) != 1 || meter.Points[0].Value != 0 || meter.IsMeter == nil || !*meter.IsMeter {
		t.Fatal("NULL row was treated as an observed zero or known zero was dropped")
	}
	if len(got.Energy.FacilityMonthly) != 2 || got.Energy.FacilityMonthly[0].Total != 1501 || got.Energy.FacilityMonthly[1].Total != 0 {
		t.Fatalf("per-dictionary monthly totals changed: %#v", got.Energy.FacilityMonthly)
	}
	heat := got.HeatFlow
	if heat.OriginalFrameCount != 1503 || heat.FrameCount != 502 || len(heat.Categories) != 2 || len(heat.Zones) != 2 || heat.SourceFile != "eplusout.sql" || heat.Labels[0] != "01-01 01:00" || heat.Labels[1] != "01-01 04:00" || heat.Labels[501] != "03-31 24:00" {
		t.Fatalf("HeatFlow time-first stride/forced last frame changed: frames=%d/%d labels=%v", heat.FrameCount, heat.OriginalFrameCount, heat.Labels)
	}
	var office, lab *HeatFlowZoneSeries
	for index := range heat.Zones {
		switch heat.Zones[index].Name {
		case "Office":
			office = &heat.Zones[index]
		case "Lab":
			lab = &heat.Zones[index]
		}
	}
	if office == nil || lab == nil || heat.Categories[0].ID != "internalConvective" || heat.Categories[1].ID != "surfaceConvection" || office.Values[0][0] != 999 || office.Values[0][1] != 0 || lab.Values[1][0] != -1 || office.Temperature[501] != 99 {
		t.Fatal("duplicate order, NULL filtering, signed category-major values or last monthly temperature changed")
	}
}
