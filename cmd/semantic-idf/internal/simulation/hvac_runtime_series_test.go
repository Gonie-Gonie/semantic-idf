package simulation

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func hvacRuntimeCadenceTestPlan() PurposeRunPlan {
	return PurposeRunPlan{
		Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck},
		OutputObjects: []PurposeOutputObject{
			{ObjectType: "Output:Variable", KeyValue: "NODE A", VariableName: "System Node Temperature", ReportingFrequency: "Hourly", PurposeIDs: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}},
			{ObjectType: "Output:Variable", KeyValue: "SCHED A", VariableName: "Schedule Value", ReportingFrequency: "Timestep", PurposeIDs: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}},
			{ObjectType: "Output:Variable", KeyValue: "MANAGER A", VariableName: "Availability Manager Night Cycle Control Status", ReportingFrequency: "Timestep", PurposeIDs: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}},
			{ObjectType: "Output:Variable", KeyValue: "OTHER PURPOSE", VariableName: "Other diagnostic", ReportingFrequency: "Timestep", PurposeIDs: []SimulationPurposeID{SimulationPurposeCustomOutputs}},
		},
	}
}

func TestHVACRuntimeTimestepSQLCadenceRetention(t *testing.T) {
	t.Run("Detailed discrete native intervals", func(t *testing.T) {
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
		exec := func(query string, args ...any) {
			t.Helper()
			if _, err := tx.Exec(query, args...); err != nil {
				t.Fatal(err)
			}
		}
		exec(`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,KeyValue TEXT,Name TEXT,Units TEXT,ReportingFrequency TEXT,IsMeter INTEGER)`)
		exec(`CREATE TABLE "Time"(TimeIndex INTEGER PRIMARY KEY,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,Year INTEGER,EnvironmentPeriodIndex INTEGER,Interval REAL,SimulationDays INTEGER,IntervalType INTEGER,WarmupFlag INTEGER)`)
		exec(`CREATE TABLE ReportData(TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`)
		exec(`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER,EnvironmentType INTEGER)`)
		exec(`INSERT INTO EnvironmentPeriods VALUES(1,1),(2,3)`)
		exec(`INSERT INTO "Time" VALUES(1,1,1,0,10,2025,1,10,1,-1,0),(2,1,1,0,10,2025,2,10,1,-1,1),(3,1,1,1,0,2025,2,60,1,1,0)`)
		for index := 1; index <= maxSQLSeriesColumns; index++ {
			exec(`INSERT INTO ReportDataDictionary VALUES(?, 'UNRELATED', ?, 'W', 'Hourly', 0)`, index, fmt.Sprintf("Other %d", index))
			exec(`INSERT INTO ReportData VALUES(3,?,999)`, index)
		}
		const scheduleID, managerID, secondManagerID, wrongFrequencyID = maxSQLSeriesColumns + 1, maxSQLSeriesColumns + 2, maxSQLSeriesColumns + 3, maxSQLSeriesColumns + 4
		for _, output := range []struct {
			id                   int
			key, name, frequency string
		}{
			{scheduleID, "SCHED A", "Schedule Value", "Zone Timestep"},
			{managerID, "MANAGER A", "Availability Manager Night Cycle Control Status", "HVAC System Timestep"},
			{secondManagerID, "MANAGER B", "Availability Manager Night Cycle Control Status", "Detailed"},
			{wrongFrequencyID, "MANAGER A", "Availability Manager Night Cycle Control Status", "Zone Timestep"},
		} {
			exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,'',?,0)`, output.id, output.key, output.name, output.frequency)
			exec(`INSERT INTO ReportData VALUES(1,?,1),(2,?,1)`, output.id, output.id)
		}
		const frames = maxCSVSeriesPoints + 102
		elapsed := 0
		for i := 0; i < frames; i++ {
			duration := []int{2, 3, 5}[i%3]
			elapsed += duration
			stamp := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(elapsed) * time.Minute)
			exec(`INSERT INTO "Time" VALUES(?,?,?,?,?,2025,2,?,?, -1,0)`, 1000+i, int(stamp.Month()), stamp.Day(), stamp.Hour(), stamp.Minute(), duration, elapsed/(24*60)+1)
			value := (i % 2) * 2
			exec(`INSERT INTO ReportData VALUES(?,?,?),(?,?,?)`, 1000+i, managerID, value, 1000+i, secondManagerID, 2-value)
			if i%3 == 2 {
				exec(`INSERT INTO ReportData VALUES(?,?,1)`, 1000+i, scheduleID)
			}
			exec(`INSERT INTO ReportData VALUES(?,?,0.5)`, 1000+i, wrongFrequencyID)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		plan := hvacRuntimeCadenceTestPlan()
		plan.OutputObjects[2].ReportingFrequency = "Detailed"
		second := plan.OutputObjects[2]
		second.KeyValue = "MANAGER B"
		plan.OutputObjects = append(plan.OutputObjects, second)
		series, err := parseSimulationSQLSeriesForPlan(path, plan)
		if err != nil {
			t.Fatal(err)
		}
		if len(series) != maxSQLSeriesColumns+3 {
			t.Fatalf("unplanned status cadence bypassed preview: %d", len(series))
		}
		schedule, manager, other := series[maxSQLSeriesColumns], series[maxSQLSeriesColumns+1], series[maxSQLSeriesColumns+2]
		if len(manager.Points) != frames || len(schedule.Points) != frames/3 || len(manager.NativeIntervals) != frames || other.NativeIntervalsSourceID != manager.SourceID || len(other.NativeIntervals) != 0 || len(schedule.NativeIntervals) != frames/3 {
			t.Fatalf("native Detailed points, gaps or shared metadata axis changed: manager=%d/%d schedule=%d/%d ref=%q", len(manager.Points), len(manager.NativeIntervals), len(schedule.Points), len(schedule.NativeIntervals), other.NativeIntervalsSourceID)
		}
		if manager.Points[0].Value != 0 || manager.Points[1].Value != 2 || manager.NativeIntervals[0].TimeIndex != 1000 || manager.NativeIntervals[1].IntervalMinutes != 3 || manager.NativeIntervals[0].EnvironmentPeriodIndex != 2 || manager.NativeIntervals[0].Year != 2025 || manager.NativeIntervals[0].SimulationDays != 1 {
			t.Fatal("native status/metadata was averaged, rounded or selected from sizing/warmup")
		}
		// Component selection can exclude the global metadata owner. The
		// delivered loop must rebase its exact axis without a dangling source.
		profile := idf.HVACOperatingProfile{ComponentIndex: 8, AvailabilityDefinitions: idf.HVACControlSequence{Nodes: []idf.HVACControlNode{{ID: "manager", Code: "availability_manager", Source: idf.HVACOperatingSource{ObjectIndex: 8, ObjectType: "AvailabilityManager:NightCycle", ObjectName: "MANAGER B"}}}}}
		result := HVACLoopRunResult{}
		attachHVACRuntimeControls(&result, []idf.HVACOperatingProfile{profile}, idf.HVACLoop{}, nil, series)
		if result.ControlCoverage.Status != "complete" || len(result.ControlSeries) != 1 || len(result.ControlSeries[0].NativeIntervals) != frames || result.ControlSeries[0].NativeIntervalsSourceID != "" {
			t.Fatalf("selected equipment native metadata escaped the local pool: %+v", result.ControlCoverage)
		}
	})
	for _, test := range []struct {
		name       string
		interval   int
		weather    bool
		nativeOnly bool
		empty      bool
	}{
		{"zone cadence", 0, true, false, false},
		{"HVAC cadence", -1, true, false, false},
		{"legacy environment metadata", 0, false, false, false},
		{"design-only native cadence", 0, false, true, false},
		{"empty native environments", 0, false, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "eplusout.sql")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var tx *sql.Tx
			exec := func(query string, args ...any) {
				t.Helper()
				var err error
				if tx == nil {
					_, err = db.Exec(query, args...)
				} else {
					_, err = tx.Exec(query, args...)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			exec(`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY, KeyValue TEXT, Name TEXT, Units TEXT, ReportingFrequency TEXT, IsMeter INTEGER)`)
			exec(`CREATE TABLE "Time" (TimeIndex INTEGER PRIMARY KEY,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,EnvironmentPeriodIndex INTEGER,IntervalType INTEGER,WarmupFlag INTEGER)`)
			exec(`CREATE TABLE ReportData (TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`)
			tx, err = db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if test.weather || test.nativeOnly {
				exec(`CREATE TABLE EnvironmentPeriods (EnvironmentPeriodIndex INTEGER,EnvironmentType INTEGER)`)
				if test.weather {
					exec(`INSERT INTO EnvironmentPeriods VALUES(1,1),(2,3)`)
				} else if !test.empty {
					exec(`INSERT INTO EnvironmentPeriods VALUES(1,1),(2,2)`)
				}
			}
			exec(`INSERT INTO "Time" VALUES(1,1,1,0,10,1,?,0),(2,1,1,0,10,2,?,1),(3,1,1,1,0,2,1,0)`, test.interval, test.interval)
			for index := 1; index <= maxSQLSeriesColumns; index++ {
				key, name, frequency := "UNRELATED", fmt.Sprintf("Unrelated output %d", index), "Hourly"
				if index == 1 {
					key, name = "NODE A", "System Node Temperature"
				} else if index == 2 {
					key, name, frequency = "SCHED A", "Schedule Value", "Detailed"
				}
				exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,?,?,0)`, index, key, name, "", frequency)
				exec(`INSERT INTO ReportData VALUES(?,?,999)`, 1, index)
			}
			for offset, item := range []struct{ key, variable, frequency string }{
				{"SCHED A", "Schedule Value", "Timestep"},
				{"MANAGER A", "Availability Manager Night Cycle Control Status", "Zone Timestep"},
				{"OTHER SCHEDULE", "Schedule Value", "Timestep"},
				{"SCHED A", "Schedule Value", "Hourly"},
				{"SCHED A", "Schedule Value", ""},
				{"OTHER PURPOSE", "Other diagnostic", "Timestep"},
			} {
				id := maxSQLSeriesColumns + offset + 1
				exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,?,?,0)`, id, item.key, item.variable, "", item.frequency)
				exec(`INSERT INTO ReportData VALUES(1,?,999),(2,?,999),(3,?,888)`, id, id, id)
			}
			exec(`INSERT INTO ReportData VALUES(3,1,21)`)
			const frames = maxCSVSeriesPoints + 101
			for index := 1; index <= frames; index++ {
				stamp := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(index) * 10 * time.Minute)
				if _, err := tx.Exec(`INSERT INTO "Time" VALUES(?,?,?,?,?,2,?,0)`, index+3, int(stamp.Month()), stamp.Day(), stamp.Hour(), stamp.Minute(), test.interval); err != nil {
					t.Fatal(err)
				}
				for _, id := range []int{2, maxSQLSeriesColumns + 1, maxSQLSeriesColumns + 2} {
					if id == maxSQLSeriesColumns+2 && index == 650 {
						continue
					}
					value := float64((index - 1) % 2)
					if id == maxSQLSeriesColumns+2 {
						value = float64((index-1)%4) / 2
					}
					if _, err := tx.Exec(`INSERT INTO ReportData VALUES(?,?,?)`, index+3, id, value); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			series, err := parseSimulationSQLSeriesForPlan(path, hvacRuntimeCadenceTestPlan())
			if err != nil {
				t.Fatal(err)
			}
			wantSeries := maxSQLSeriesColumns + 2
			if test.empty {
				wantSeries = maxSQLSeriesColumns
			}
			if len(series) != wantSeries {
				t.Fatalf("runtime diagnostics were lost or unplanned keys/frequencies bypassed preview: %d series", len(series))
			}
			if test.empty {
				return
			}
			schedule, manager := series[maxSQLSeriesColumns], series[maxSQLSeriesColumns+1]
			extra := 0
			if test.nativeOnly {
				extra = 1
			} else if !test.weather {
				extra = 3
			}
			if len(schedule.Points) != frames+extra || len(manager.Points) != frames-1+extra || len(series[1].Points) != maxCSVSeriesPoints {
				t.Fatalf("cadence retention or generic bound changed: %v", hvacTestPointCounts(series))
			}
			if test.weather {
				if len(series[0].Points) != 1 || series[0].Points[0].Value != 21 || schedule.Points[0].Value != 0 || manager.Points[0].Value != 0 || manager.Points[1].Value != .5 {
					t.Fatal("cadence, sizing/warmup, known zero or mixed-status evidence changed")
				}
				if manager.Points[649].X != schedule.Points[650].X || manager.Points[649].Label != schedule.Points[650].Label {
					t.Fatal("missing manager observation shifted the shared native time axis")
				}
			}
			if test.nativeOnly && (len(series[0].Points) != 2 || schedule.Points[0].Value != 999 || schedule.Points[1].Value != 0 || manager.Points[1].Value != 0 || manager.Points[2].Value != .5) {
				t.Fatal("design-only Timestep must retain its native non-warmup axis without changing legacy Hourly behavior")
			}
			if schedule.ReportingFrequency != "Timestep" || manager.ReportingFrequency != "Zone Timestep" || schedule.SourceID == manager.SourceID {
				t.Fatal("native cadence or series identity was rewritten")
			}
		})
	}
}

func TestHVACRuntimeTimestepCSVRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.csv")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := csv.NewWriter(file)
	header := []string{"Date/Time"}
	for index := 1; index <= maxCSVSeriesColumns; index++ {
		header = append(header, fmt.Sprintf("UNRELATED:Unrelated output %d [W](Hourly)", index))
	}
	header = append(header, "SCHED A:Schedule Value [](Timestep)", "MANAGER A:Availability Manager Night Cycle Control Status [](Zone Timestep)", "OTHER:Schedule Value [](Timestep)", "SCHED A:Schedule Value [](Detailed)", "SCHED A:Schedule Value [](Hourly)", "SCHED A:Schedule Value []", "OTHER PURPOSE:Other diagnostic [](Timestep)", "MANAGER A:Availability Manager Night Cycle Control Status [](Detailed)")
	if err := writer.Write(header); err != nil {
		t.Fatal(err)
	}
	const frames = maxCSVSeriesPoints + 101
	for index := 1; index <= frames; index++ {
		stamp := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(index) * 10 * time.Minute)
		row := []string{stamp.Format("01/02 15:04:05")}
		for column := 1; column <= maxCSVSeriesColumns; column++ {
			row = append(row, strconv.Itoa(index))
		}
		manager := strconv.FormatFloat(float64((index-1)%4)/2, 'f', 1, 64)
		if index == 650 {
			manager = ""
		}
		row = append(row, strconv.Itoa((index-1)%2), manager, "1", "1", "1", "1", "1", strconv.Itoa(((index-1)%2)*2))
		if err := writer.Write(row); err != nil {
			t.Fatal(err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, series, err := parseSimulationCSVForPlan(path, hvacRuntimeCadenceTestPlan())
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != maxCSVSeriesColumns+2 {
		t.Fatalf("CSV admitted unplanned runtime keys/cadences: %d series", len(series))
	}
	schedule, manager := series[maxCSVSeriesColumns], series[maxCSVSeriesColumns+1]
	if len(series[0].Points) != maxCSVSeriesPoints || len(schedule.Points) != frames || len(manager.Points) != frames-1 || schedule.Points[0].Value != 0 || manager.Points[1].Value != .5 {
		t.Fatal("CSV runtime observations were sampled or changed, or generic preview became unbounded")
	}
	if manager.Points[649].X != schedule.Points[650].X || manager.Points[649].Label != schedule.Points[650].Label {
		t.Fatal("CSV missing observation shifted shared source row coordinates")
	}
	detailedPlan := hvacRuntimeCadenceTestPlan()
	detailedPlan.OutputObjects[2].ReportingFrequency = "Detailed"
	_, detailed, err := parseSimulationCSVForPlan(path, detailedPlan)
	if err != nil || len(detailed) != maxCSVSeriesColumns+2 || len(detailed[len(detailed)-1].Points) != frames || detailed[len(detailed)-1].Points[0].Value != 0 || detailed[len(detailed)-1].Points[1].Value != 2 || !purposeDetailedFrequency(purposeSeriesFrequency(detailed[len(detailed)-1])) {
		t.Fatal("CSV Detailed source selection changed discrete values, cadence or sample count", err)
	}
	plan := hvacRuntimeCadenceTestPlan()
	plan.Purposes = []SimulationPurposeID{SimulationPurposeCustomOutputs}
	_, generic, err := parseSimulationCSVForPlan(path, plan)
	if err != nil || len(generic) != maxCSVSeriesColumns {
		t.Fatal("a non-HVAC purpose retained unbounded runtime diagnostics")
	}
}
