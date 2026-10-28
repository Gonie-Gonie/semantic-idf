package simulation

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"testing"
	"time"
)

// RunSimulation launches this test executable as a no-op engine against a
// synthetic output fixture. This covers the runner's plan handoff as well as
// parsing; it does not require an installed EnergyPlus engine.
func TestMain(m *testing.M) {
	if os.Getenv("SEMANTIC_IDF_TEST_PURPOSE_ENGINE") == "1" && len(os.Args) > 2 && os.Args[1] == "-d" {
		if path := os.Getenv("SEMANTIC_IDF_TEST_ENGINE_ARGUMENTS_PATH"); path != "" {
			payload, _ := json.Marshal(os.Args[1:])
			_ = os.WriteFile(path, payload, 0o644)
		}
		if expanded := os.Getenv("SEMANTIC_IDF_TEST_ENGINE_EXPANDED_INPUT"); expanded != "" {
			_ = os.WriteFile(filepath.Join(os.Args[2], "eplusout.expidf"), []byte(expanded), 0o644)
		}
		if code, err := strconv.Atoi(os.Getenv("SEMANTIC_IDF_TEST_ENGINE_EXIT_CODE")); err == nil {
			os.Exit(code)
		}
		os.Exit(0)
	}
	// Cross-process storage tests deliberately share the parent's isolated app
	// data root. All other tests, including older runner fixtures, must keep new
	// ownership/session/root-index metadata away from the user's real profile.
	if os.Getenv("SEMANTIC_IDF_STORAGE_INSTANCE_CHILD") != "" {
		os.Exit(m.Run())
	}
	os.Exit(runSimulationTestsWithIsolatedAppData(m))
}

func runSimulationTestsWithIsolatedAppData(m *testing.M) int {
	root, err := os.MkdirTemp("", "semantic-idf-simulation-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	root, err = filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	temporaryRoot, err := filepath.Abs(os.TempDir())
	if err != nil || storagePathKey(filepath.Dir(root)) != storagePathKey(temporaryRoot) {
		fmt.Fprintln(os.Stderr, "test app-data path is outside the temporary root", err)
		return 1
	}
	exitCode := 1
	defer func() {
		// root was created atomically above and its absolute parent was verified.
		if err := os.RemoveAll(root); err != nil {
			fmt.Fprintln(os.Stderr, "remove synthetic test app data:", err)
		}
	}()
	for key, directory := range map[string]string{"LOCALAPPDATA": "local", "APPDATA": "roaming"} {
		if err := os.Setenv(key, filepath.Join(root, directory)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitCode
		}
	}
	return m.Run()
}

type purposeSeriesFixtureColumn struct {
	key, name, unit, frequency string
	value                      float64
}

func purposeSeriesFixtureColumns() []purposeSeriesFixtureColumn {
	return []purposeSeriesFixtureColumn{
		{"SUPPLY OUTLET", "System Node Temperature", "C", "Hourly", 23},
		{"SUPPLY OUTLET", "System Node Mass Flow Rate", "kg/s", "Hourly", 0.5},
		{"SUPPLY FAN", "Fan Electricity Rate", "W", "Hourly", 100},
		{"OFFICE", "Zone Mean Air Temperature", "C", "Hourly", 22},
		{"OFFICE", "Zone Air Relative Humidity", "%", "Hourly", 45},
		{"UNSELECTED ZONE", "Zone Mean Air Temperature", "C", "Hourly", 99},
		{"UNSELECTED NODE", "System Node Temperature", "C", "Hourly", 100},
		{"SUPPLY OUTLET", "System Node Temperature", "C", "Daily", 26},
		{"UNSELECTED PUMP", "Pump Electricity Rate", "W", "Hourly", 500},
		{"UNSELECTED CHILLER", "Chiller Electricity Rate", "W", "Hourly", 800},
	}
}

func purposeSeriesFixturePlan(purposes ...SimulationPurposeID) PurposeRunPlan {
	plan := PurposeRunPlan{Purposes: purposes}
	for _, column := range purposeSeriesFixtureColumns()[:5] {
		plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{
			ObjectType: "Output:Variable", KeyValue: column.key,
			VariableName: column.name, ReportingFrequency: "Hourly",
		})
	}
	return plan
}

func createPurposeSeriesSQLFixture(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY, KeyValue TEXT, Name TEXT, Units TEXT, ReportingFrequency TEXT, IsMeter INTEGER)`,
		`CREATE TABLE "Time" (TimeIndex INTEGER PRIMARY KEY, Month INTEGER, Day INTEGER, Hour INTEGER, Minute INTEGER)`,
		`CREATE TABLE ReportData (ReportDataIndex INTEGER PRIMARY KEY, TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`,
		`INSERT INTO "Time" VALUES (1, 1, 1, 1, 0), (2, 1, 1, 2, 0)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	columns := make([]purposeSeriesFixtureColumn, 0, maxSQLSeriesColumns+8)
	for index := 0; index < maxSQLSeriesColumns; index++ {
		columns = append(columns, purposeSeriesFixtureColumn{"UNRELATED", fmt.Sprintf("Unrelated output %d", index), "W", "Hourly", 1})
	}
	columns = append(columns, purposeSeriesFixtureColumns()...)
	for index, column := range columns {
		id := index + 1
		if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES (?, ?, ?, ?, ?, 0)`, id, column.key, column.name, column.unit, column.frequency); err != nil {
			t.Fatal(err)
		}
		for frame := 1; frame <= 2; frame++ {
			if _, err := tx.Exec(`INSERT INTO ReportData (TimeIndex, ReportDataDictionaryIndex, Value) VALUES (?, ?, ?)`, frame, id, column.value+float64(frame-1)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestPurposeSQLSeriesBypassPreviewLimitForSelectedPanels(t *testing.T) {
	t.Run("environment Hourly full weather axis", func(t *testing.T) {
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
		exec(`CREATE TABLE Time(TimeIndex INTEGER PRIMARY KEY,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,EnvironmentPeriodIndex INTEGER,IntervalType INTEGER,WarmupFlag INTEGER)`)
		exec(`CREATE TABLE ReportData(TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`)
		exec(`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER,EnvironmentType INTEGER)`)
		exec(`INSERT INTO EnvironmentPeriods VALUES(1,1),(2,3)`)
		exec(`INSERT INTO Time VALUES(1,1,1,1,0,1,1,0),(2,1,1,1,0,2,1,1)`)
		for index := 1; index <= maxSQLSeriesColumns; index++ {
			exec(`INSERT INTO ReportDataDictionary VALUES(?,'OTHER',?,'W','Hourly',0)`, index, fmt.Sprintf("Unrelated %d", index))
			exec(`INSERT INTO ReportData VALUES(1,?,7)`, index)
		}
		names := simulationEnvironmentPlotVariables()
		units := []string{"C", "C", "%", "Pa", "W/m2", "W/m2", "m/s", "deg"}
		for index, name := range names {
			id := maxSQLSeriesColumns + index + 1
			exec(`INSERT INTO ReportDataDictionary VALUES(?,'Environment',?,?,'Hourly',0)`, id, name, units[index])
			exec(`INSERT INTO ReportData VALUES(1,?,999),(2,?,999)`, id, id)
		}
		exec(`INSERT INTO ReportDataDictionary VALUES(?,'OTHER',?,'C','Hourly',0)`, maxSQLSeriesColumns+9, names[0])
		const frames = maxCSVSeriesPoints + 103
		for frame := 0; frame < frames; frame++ {
			stamp := time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC).Add(time.Duration(frame) * time.Hour)
			exec(`INSERT INTO Time VALUES(?,?,?,?,0,2,1,0)`, 1000+frame, int(stamp.Month()), stamp.Day(), stamp.Hour())
			for index := range names {
				if index == 1 && frame == 250 {
					continue
				}
				var value any = float64(frame)
				if index == 2 && frame == 251 {
					value = nil
				}
				exec(`INSERT INTO ReportData VALUES(?,?,?)`, 1000+frame, maxSQLSeriesColumns+index+1, value)
			}
			exec(`INSERT INTO ReportData VALUES(?,?,999)`, 1000+frame, maxSQLSeriesColumns+9)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		for _, purpose := range []SimulationPurposeID{SimulationPurposeBasicEnergy, SimulationPurposeZoneHeatFlow, SimulationPurposeComfort} {
			plan := BuildPurposeRunPlan(parsePurposePlanFixture(t, "Version,24.2;\nZone,Office;"), SimulationPurposeRequest{Purposes: []SimulationPurposeID{purpose}})
			series, err := parseSimulationSQLSeriesForPlan(path, plan)
			if err != nil {
				t.Fatal(err)
			}
			if len(series) != maxSQLSeriesColumns+8 {
				t.Fatal("environment observations were capped or an unrequested source escaped", purpose, len(series))
			}
			var base SimulationSeries
			for index, item := range series[maxSQLSeriesColumns:] {
				want := frames
				if index == 1 || index == 2 {
					want--
				}
				if len(item.Points) != want || item.ReportingFrequency != "Hourly" || item.KeyValue != "Environment" || item.Name != names[index] || unitFromSeriesColumn(item.Column) != units[index] || item.SourceID == "" || item.IsMeter == nil || *item.IsMeter || item.Points[0].Value != 0 {
					t.Fatal("native environment identity, cadence, units or knownzero changed", purpose, index, item.Column, len(item.Points))
				}
				if index == 0 {
					base = item
				}
				if index == 1 && (item.Points[250].X != base.Points[251].X || item.Points[250].Label != base.Points[251].Label) {
					t.Fatal("missing environment observation shifted native coordinates")
				}
			}
		}
	})
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	createPurposeSeriesSQLFixture(t, path)
	for _, test := range []struct {
		name     string
		purposes []SimulationPurposeID
		extra    int
	}{
		{"generic preview", nil, 0},
		{"HVAC only", []SimulationPurposeID{SimulationPurposeHVACLoopCheck}, 4},
		{"Comfort only", []SimulationPurposeID{SimulationPurposeComfort}, 2},
		{"both panels", []SimulationPurposeID{SimulationPurposeHVACLoopCheck, SimulationPurposeComfort}, 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := parseSimulationSQL(path, purposeSeriesFixturePlan(test.purposes...))
			if err != nil {
				t.Fatal(err)
			}
			if len(parsed.Series) != maxSQLSeriesColumns+test.extra {
				t.Fatalf("series count = %d, want %d", len(parsed.Series), maxSQLSeriesColumns+test.extra)
			}
			for _, series := range parsed.Series {
				if series.KeyValue == "UNSELECTED ZONE" || series.KeyValue == "UNSELECTED NODE" || series.KeyValue == "UNSELECTED PUMP" || series.KeyValue == "UNSELECTED CHILLER" {
					t.Fatalf("series outside planned keys bypassed preview limit: %s", series.Column)
				}
			}
		})
	}
}

func TestPurposeSQLSeriesWithoutOutputPlanUsesLegacyPurposeScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	createPurposeSeriesSQLFixture(t, path)
	series, err := parseSimulationSQLSeriesForPlan(path, PurposeRunPlan{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != maxSQLSeriesColumns+7 {
		t.Fatalf("legacy HVAC purpose series = %d, want %d", len(series), maxSQLSeriesColumns+7)
	}
	components := map[string]bool{}
	for _, item := range series {
		components[item.KeyValue] = true
	}
	if !components["UNSELECTED PUMP"] || !components["UNSELECTED CHILLER"] {
		t.Fatal("an empty legacy output plan must preserve the unrestricted HVAC component fallback")
	}
}

func TestRunSimulationReadsSelectedPanelsBeforeBuildingResults(t *testing.T) {
	dir := t.TempDir()
	createPurposeSeriesSQLFixture(t, filepath.Join(dir, "eplusout.sql"))
	if err := os.WriteFile(filepath.Join(dir, "eplusout.err"), []byte("EnergyPlus Completed Successfully"), 0o644); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SEMANTIC_IDF_TEST_PURPOSE_ENGINE", "1")
	plan := purposeSeriesFixturePlan(SimulationPurposeHVACLoopCheck, SimulationPurposeComfort)
	request := SimulationRunRequest{
		RunID: "purpose-series-regression", Filename: "input.idf", Text: "Version,25.1;",
		OutputDirectory: dir, EnergyPlusExecutablePath: executable,
		PurposeRunPlan: &plan, PurposeRequest: &SimulationPurposeRequest{Purposes: plan.Purposes},
	}
	settings := SimulationSettings{EnergyPlusInstallations: []EnergyPlusInstallSetting{{ExecutablePath: executable, Version: "25.1"}}}
	result, err := RunSimulation(request, nil, settings)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || result.PurposeResults == nil {
		t.Fatalf("run status = %s, error = %s, stderr = %s", result.Status, result.Error, result.Stderr)
	}
	assertPurposeSeriesPanelResults(t, result.PurposeResults)
	if len(result.Series) != maxSQLSeriesColumns+6 || len(result.ResultSources) != 1 || result.ResultSources[0] != "sql" {
		t.Fatalf("series = %d, sources = %v", len(result.Series), result.ResultSources)
	}
}

func TestPurposeCSVFallbackBypassesPreviewLimitForSelectedPanels(t *testing.T) {
	t.Run("environment Hourly CSV retention", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "environment.csv")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		writer := csv.NewWriter(file)
		header := []string{"Date/Time"}
		for index := 0; index < maxCSVSeriesColumns; index++ {
			header = append(header, fmt.Sprintf("OTHER:Other %d [W](Hourly)", index))
		}
		names := simulationEnvironmentPlotVariables()
		units := []string{"C", "C", "%", "Pa", "W/m2", "W/m2", "m/s", "deg"}
		for index, name := range names {
			header = append(header, "Environment:"+name+" ["+units[index]+"](Hourly)")
		}
		if err := writer.Write(header); err != nil {
			t.Fatal(err)
		}
		const frames = maxCSVSeriesPoints + 103
		for frame := 0; frame < frames; frame++ {
			row := []string{time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC).Add(time.Duration(frame) * time.Hour).Format("01/02 15:04:05")}
			for index := 0; index < maxCSVSeriesColumns; index++ {
				row = append(row, "1")
			}
			for index := range names {
				value := strconv.Itoa(frame)
				if index == 1 && frame == 250 {
					value = ""
				}
				row = append(row, value)
			}
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
		for _, purpose := range []SimulationPurposeID{SimulationPurposeBasicEnergy, SimulationPurposeZoneHeatFlow, SimulationPurposeComfort} {
			plan := BuildPurposeRunPlan(parsePurposePlanFixture(t, "Version,24.2;\nZone,Office;"), SimulationPurposeRequest{Purposes: []SimulationPurposeID{purpose}})
			_, series, err := parseSimulationCSVForPlan(path, plan)
			if err != nil {
				t.Fatal(err)
			}
			if len(series) != maxCSVSeriesColumns+8 || len(series[0].Points) != maxCSVSeriesPoints {
				t.Fatal("environment CSV selection removed generic bounds or lost requested columns", purpose)
			}
			for index, item := range series[maxCSVSeriesColumns:] {
				key, name := splitPurposeSeriesColumn(item.Column)
				want := frames
				if index == 1 {
					want--
				}
				if key != "Environment" || name != names[index] || purposeSeriesFrequency(item) != "Hourly" || unitFromSeriesColumn(item.Column) != units[index] || len(item.Points) != want || item.Points[0].Value != 0 || item.File != "environment.csv" {
					t.Fatal("environment CSV source/cadence/units/samples changed", purpose, index, item.Column, len(item.Points))
				}
				if index == 1 && (item.Points[250].X != series[maxCSVSeriesColumns].Points[251].X || item.Points[250].Label != series[maxCSVSeriesColumns].Points[251].Label) {
					t.Fatal("missing CSV environment observation changed row identity")
				}
			}
		}
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.csv")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := csv.NewWriter(file)
	header := []string{"Date/Time"}
	values := []string{"01/01 01:00:00"}
	for index := 0; index < maxCSVSeriesColumns; index++ {
		header = append(header, fmt.Sprintf("UNRELATED:Unrelated output %d [W](Hourly)", index))
		values = append(values, "1")
	}
	for _, column := range purposeSeriesFixtureColumns() {
		header = append(header, fmt.Sprintf("%s:%s [%s](%s)", column.key, column.name, column.unit, column.frequency))
		values = append(values, strconv.FormatFloat(column.value, 'f', -1, 64))
	}
	if err := writer.WriteAll([][]string{header, values}); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, preview, err := parseSimulationCSV(path)
	if err != nil || len(preview) != maxCSVSeriesColumns {
		t.Fatalf("generic CSV preview = %d, error = %v", len(preview), err)
	}
	plan := purposeSeriesFixturePlan(SimulationPurposeHVACLoopCheck, SimulationPurposeComfort)
	result := &SimulationRunResult{OutputDirectory: dir, PurposeRunPlan: &plan}
	readSimulationOutputs(result)
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{Purposes: plan.Purposes})
	assertPurposeSeriesPanelResults(t, &bundle)
	if len(result.Series) != maxCSVSeriesColumns+6 || len(result.ResultSources) != 1 || result.ResultSources[0] != "csv" {
		t.Fatalf("series = %d, sources = %v", len(result.Series), result.ResultSources)
	}
}

func TestInitialSQLReadKeepsSeriesWhenHeatFlowIsMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createPurposeSeriesSQLFixture(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES (1000, 'OFFICE', 'Zone Air Heat Balance Internal Convective Heat Gain Rate', 'W', 'Hourly', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData (TimeIndex, ReportDataDictionaryIndex, Value) VALUES (1, 1000, 'invalid numeric observation')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := parseSimulationHeatFlowSQL(path); err == nil {
		t.Fatal("fixture must fail HeatFlow parsing independently of the valid preview series")
	}
	result := &SimulationRunResult{OutputDirectory: dir}
	readSimulationOutputs(result)
	if len(result.Series) != maxSQLSeriesColumns || len(result.HeatFlow.Zones) != 0 || len(result.ResultSources) != 1 || result.ResultSources[0] != "sql" {
		t.Fatalf("valid SQL series were lost after another section failed: series=%d, HeatFlow zones=%d, sources=%v", len(result.Series), len(result.HeatFlow.Zones), result.ResultSources)
	}
	t.Run("heat flow survives series failure", func(t *testing.T) {
		path := initialCompactSQLFixture(t, "INTEGER")
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE ReportData SET Value='invalid preview observation' WHERE ReportDataDictionaryIndex=10`); err != nil {
			db.Close()
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := parseSimulationSQLSeries(path); err == nil {
			t.Fatal("fixture must fail preview series independently of valid heat flow")
		}
		for _, processors := range []int{1, 2} {
			previous := goruntime.GOMAXPROCS(processors)
			result := &SimulationRunResult{OutputDirectory: filepath.Dir(path)}
			readSimulationOutputs(result)
			goruntime.GOMAXPROCS(previous)
			if len(result.Series) != 0 || len(result.HeatFlow.Zones) != 2 || len(result.ResultSources) != 1 || result.ResultSources[0] != "sql" {
				t.Fatalf("a failed series reader abandoned the independent heat-flow read at %d processors", processors)
			}
		}
	})
}

func TestInitialSQLReadRecognizesDiagnosticAndTabularSources(t *testing.T) {
	for _, test := range []struct {
		name, schema string
		wantSQL      bool
	}{
		{"diagnostics only", `CREATE TABLE Errors (ErrorMessage TEXT)`, true},
		{"tabular only", `CREATE TABLE TabularDataWithStrings (ReportName TEXT, TableName TEXT, RowName TEXT, ColumnName TEXT, Value TEXT)`, true},
		{"unrelated database", `CREATE TABLE Metadata (Name TEXT, Value TEXT)`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			db, err := sql.Open("sqlite", filepath.Join(dir, "eplusout.sql"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(test.schema); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			result := &SimulationRunResult{OutputDirectory: dir}
			readSimulationOutputs(result)
			gotSQL := len(result.ResultSources) == 1 && result.ResultSources[0] == "sql"
			if gotSQL != test.wantSQL || len(result.Series) != 0 || len(result.HeatFlow.Zones) != 0 {
				t.Fatalf("sources=%v, series=%d, HeatFlow zones=%d", result.ResultSources, len(result.Series), len(result.HeatFlow.Zones))
			}
		})
	}
}

func assertPurposeSeriesPanelResults(t *testing.T, bundle *PurposeResultBundle) {
	t.Helper()
	if len(bundle.HVACLoops) != 1 || len(bundle.HVACLoops[0].NodeSummaries) != 1 || len(bundle.HVACLoops[0].Components) != 1 {
		t.Fatalf("HVAC panel omitted planned node or component results: %+v", bundle.HVACLoops)
	}
	if len(bundle.Comfort.Zones) != 1 || bundle.Comfort.Zones[0].ZoneName != "OFFICE" || len(bundle.Comfort.Zones[0].Metrics) != 2 {
		t.Fatalf("Comfort panel omitted planned zone results: %+v", bundle.Comfort.Zones)
	}
}
