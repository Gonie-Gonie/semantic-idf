package simulation

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"sync/atomic"
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
	for _, test := range []struct {
		name, table, values      string
		wantFrames, wantOriginal int
		wantError                bool
	}{
		{"NULL and invalid-only value frames", `CREATE TABLE Time(TimeIndex INTEGER PRIMARY KEY,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER)`, `INSERT INTO ReportData VALUES(1,1,0),(2,1,NULL),(3,1,1e999),(4,1,NULL)`, 4, 4, false},
		{"all NULL time keys", `CREATE TABLE Time(TimeIndex INTEGER PRIMARY KEY,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER)`, `INSERT INTO ReportData VALUES(NULL,1,0)`, 0, 0, false},
		{"mixed NULL time key error", `CREATE TABLE Time(TimeIndex INTEGER PRIMARY KEY,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER)`, `INSERT INTO ReportData VALUES(1,1,0),(NULL,1,1)`, 0, 0, true},
		{"orphan BLOB key distinct count", `CREATE TABLE Time(TimeIndex INTEGER PRIMARY KEY,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER)`, `INSERT INTO ReportData VALUES(1,1,0),(x'31',1,7)`, 1, 2, false},
		{"non-rowid integer key distinct count", `CREATE TABLE Time(TimeIndex INTEGER PRIMARY KEY DESC,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER)`, `INSERT INTO ReportData VALUES(1,1,0),(x'31',1,7)`, 1, 2, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, path := compactReportDataFixture(t,
				`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,KeyValue TEXT,Name TEXT,Units TEXT,ReportingFrequency TEXT,IsMeter INTEGER)`,
				test.table,
				`CREATE TABLE ReportData(TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
				`INSERT INTO ReportDataDictionary VALUES(1,' Office ','Zone Air Heat Balance Internal Convective Heat Gain Rate','W','Hourly',0)`,
				`INSERT INTO Time VALUES(1,1,1,1,0),(2,1,1,2,0),(3,1,1,3,0),(4,1,1,4,0)`,
				test.values,
			)
			data, err := parseSimulationHeatFlowSQL(path)
			if (err != nil) != test.wantError {
				t.Fatalf("time key behavior changed: %v", err)
			}
			if test.wantError {
				return
			}
			if data.FrameCount != test.wantFrames || data.OriginalFrameCount != test.wantOriginal {
				t.Fatalf("native or SQL-distinct count changed: %d/%d", data.FrameCount, data.OriginalFrameCount)
			}
			if test.wantFrames == 4 {
				zone := data.Zones[0]
				if !reflect.DeepEqual(zone.Observed[0], []bool{true, false, false, false}) || zone.Values[0][0] != 0 || data.Labels[3] != "01-01 04:00" {
					t.Fatal("known zero or invalid-only native frame was lost", data)
				}
			}
		})
	}
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
	t.Run("bounded parallel initial readers", func(t *testing.T) {
		for _, location := range []string{"series parser", "heat-flow parser", "caller progress"} {
			t.Run(location+" panic joins worker", func(t *testing.T) {
				marker := &struct{ location string }{location}
				started, seriesUnwinding, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
				seriesReader := func() ([]SimulationSeries, error) {
					<-started
					if location == "series parser" {
						defer close(seriesUnwinding)
						panic(marker)
					}
					return nil, nil
				}
				heatFlowReader := func(report func(sqlWorkProgress)) (HeatFlowDataset, error) {
					defer close(finished)
					close(started)
					if location == "series parser" {
						<-seriesUnwinding
						goruntime.Gosched()
					}
					if location == "heat-flow parser" {
						panic(marker)
					}
					report(sqlWorkProgress{Unit: "rows", Completed: 7, Finished: true})
					return HeatFlowDataset{}, nil
				}
				var recovered any
				func() {
					defer func() { recovered = recover() }()
					runInitialSQLReadersInParallel(seriesReader, heatFlowReader, func(sqlWorkProgress) {
						if location == "caller progress" {
							panic(marker)
						}
					}, func() {})
				}()
				if recovered != marker {
					t.Fatal("reader panic did not reach the caller's recovery boundary", recovered)
				}
				select {
				case <-finished:
				default:
					t.Fatal("caller unwound before its independent SQL worker finished")
				}
			})
		}
		capture := func(processors int) (SimulationRunResult, []string, map[string]int64) {
			previous := goruntime.GOMAXPROCS(processors)
			defer goruntime.GOMAXPROCS(previous)
			result := SimulationRunResult{Files: []SimulationFileInfo{{Name: "eplusout.sql", Path: fastPath, Kind: "sqlite"}}, PurposeRunPlan: &plan}
			phases := []string{}
			lastRows, finalRows, finishes := map[string]int64{}, map[string]int64{}, map[string]int{}
			phase := ""
			var busy int32
			enter := func() bool {
				if !atomic.CompareAndSwapInt32(&busy, 0, 1) {
					t.Error("SQL observers were called concurrently")
					return false
				}
				goruntime.Gosched()
				return true
			}
			progress := func(event SimulationProgress) {
				if !enter() {
					return
				}
				defer atomic.StoreInt32(&busy, 0)
				phase = event.Phase
				phases = append(phases, phase)
			}
			work := func(event sqlWorkProgress) {
				if !enter() {
					return
				}
				defer atomic.StoreInt32(&busy, 0)
				if event.Unit != "rows" {
					return
				}
				if phase != "sql_series" && phase != "sql_heat_flow" {
					t.Error("SQL row event escaped its caller phase", phase)
				}
				if event.Completed < lastRows[phase] {
					t.Error("SQL consumed rows decreased", phase)
				}
				lastRows[phase] = event.Completed
				if event.Finished {
					finalRows[phase] = event.Completed
					finishes[phase]++
				}
			}
			if !parseSQLResultsWithProgress(&result, "initial-read", progress, "", work) {
				t.Fatal("initial readers lost both SQL sections")
			}
			for _, section := range []string{"sql_series", "sql_heat_flow"} {
				if finishes[section] != 1 || finalRows[section] <= 4096 {
					t.Fatal("bounded progress lost a final section count", finishes, finalRows)
				}
			}
			return result, phases, finalRows
		}
		serial, serialPhases, serialRows := capture(1)
		parallel, parallelPhases, parallelRows := capture(2)
		if !reflect.DeepEqual(serial, parallel) || !reflect.DeepEqual(serialRows, parallelRows) || !reflect.DeepEqual(serialPhases, []string{"sql_series", "sql_heat_flow"}) || !reflect.DeepEqual(serialPhases, parallelPhases) {
			t.Fatal("parallel readers changed results, final counts or phase order", serialPhases, parallelPhases, serialRows, parallelRows)
		}
	})
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
	if heat.OriginalFrameCount != 1503 || heat.FrameCount != 1503 || len(heat.Categories) != 2 || len(heat.Zones) != 2 || heat.SourceFile != "eplusout.sql" || heat.Labels[0] != "01-01 01:00" || heat.Labels[1] != "01-01 02:00" || heat.Labels[1502] != "03-31 24:00" {
		t.Fatalf("HeatFlow native timestamps changed: frames=%d/%d", heat.FrameCount, heat.OriginalFrameCount)
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
	if office == nil || lab == nil || heat.Categories[0].ID != "internalConvective" || heat.Categories[1].ID != "surfaceConvection" || office.Values[0][0] != 999 || office.Values[0][1] != 0 || office.Observed[0][3] || lab.Values[1][0] != -1 || office.Temperature[1502] != 99 {
		t.Fatal("duplicate order, NULL filtering, signed category-major values or last monthly temperature changed")
	}
}
