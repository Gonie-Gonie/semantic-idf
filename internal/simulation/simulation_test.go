package simulation

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestParseERRFileCountsIssues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.err")
	content := `Program Version,EnergyPlus
   ** Warning ** Missing schedule fallback
   ** Severe  ** Node connection problem
   ** Fatal  ** Simulation stopped
EnergyPlus Terminated--Fatal Error Detected. 1 Warning; 1 Severe Errors; Elapsed Time=00hr 00min 01.00sec
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	summary := parseERRFile(path)
	if summary.Warnings != 1 || summary.Severe != 1 || summary.Fatal != 1 {
		t.Fatalf("counts = warnings %d severe %d fatal %d", summary.Warnings, summary.Severe, summary.Fatal)
	}
	if len(summary.Issues) != 3 {
		t.Fatalf("issue count = %d, want 3", len(summary.Issues))
	}
	if summary.Completed {
		t.Fatalf("fatal run should not be marked completed")
	}
}

func TestParseSimulationCSVBuildsSummariesAndSeries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.csv")
	content := `Date/Time,ZONE ONE:Zone Air Temperature [C](Hourly),Electricity:Facility [J](Hourly)
 01/01  01:00:00,20.0,100
 01/01  02:00:00,21.5,150
 01/01  03:00:00,19.5,125
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	summary, series, err := parseSimulationCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if summary.RowCount != 3 {
		t.Fatalf("row count = %d, want 3", summary.RowCount)
	}
	if len(summary.ColumnInfo) != 2 {
		t.Fatalf("column summary count = %d, want 2", len(summary.ColumnInfo))
	}
	first := summary.ColumnInfo[0]
	if first.Min != 19.5 || first.Max != 21.5 || first.Last != 19.5 {
		t.Fatalf("temperature summary = %+v", first)
	}
	if len(series) != 2 || len(series[0].Points) != 3 {
		t.Fatalf("series = %+v", series)
	}
}

func TestParseSimulationHeatFlowCSVBuildsDataset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.csv")
	content := `Date/Time,ZONE ONE:Zone Mean Air Temperature [C](Hourly),ZONE ONE:Zone Air Heat Balance Internal Convective Heat Gain Rate [W](Hourly),ZONE ONE:Zone Air Heat Balance Surface Convection Rate [W](Hourly),ZONE TWO:Zone Air Heat Balance Outdoor Air Transfer Rate [W](Hourly)
 01/01  01:00:00,20.0,100,-30,-12
 01/01  02:00:00,21.5,150,-45,-18
 01/01  03:00:00,19.5,125,-20,-8
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	dataset, err := parseSimulationHeatFlowCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if dataset.FrameCount != 3 || dataset.OriginalFrameCount != 3 {
		t.Fatalf("frame counts = %d/%d, want 3/3", dataset.FrameCount, dataset.OriginalFrameCount)
	}
	if len(dataset.Categories) != 3 {
		t.Fatalf("category count = %d, want 3: %#v", len(dataset.Categories), dataset.Categories)
	}
	if len(dataset.Zones) != 2 {
		t.Fatalf("zone count = %d, want 2: %#v", len(dataset.Zones), dataset.Zones)
	}
	if dataset.Zones[0].Name != "ZONE ONE" || dataset.Zones[0].Temperature[1] != 21.5 {
		t.Fatalf("first zone = %#v", dataset.Zones[0])
	}
	if dataset.MaxAbs != 150 {
		t.Fatalf("max abs = %v, want 150", dataset.MaxAbs)
	}
}

func TestParseSimulationHeatFlowESOBuildsDataset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.eso")
	content := `Program Version,EnergyPlus
1,5,Environment Title[],Latitude[deg],Longitude[deg],Time Zone[],Elevation[m]
2,8,Day of Simulation[],Month[],Day of Month[],DST Indicator[1=yes 0=no],Hour[],StartMinute[],EndMinute[],DayType
10,1,ZONE ONE,Zone Mean Air Temperature [C] !Hourly
11,1,ZONE ONE,Zone Air Heat Balance Internal Convective Heat Gain Rate [W] !Hourly
12,1,ZONE ONE,Zone Air Heat Balance Surface Convection Rate [W] !Hourly
13,1,ZONE TWO,Zone Air Heat Balance Outdoor Air Transfer Rate [W] !Hourly
End of Data Dictionary
1,RUN PERIOD,  0.00, 0.00,   0.00,  0.00
2,1, 1, 1, 0, 1, 0.00,60.00,Monday
10,20.0
11,100.0
12,-30.0
13,-12.0
2,1, 1, 1, 0, 2, 0.00,60.00,Monday
10,21.5
11,150.0
12,-45.0
13,-18.0
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	dataset, err := parseSimulationHeatFlowESO(path)
	if err != nil {
		t.Fatal(err)
	}
	if dataset.FrameCount != 2 || dataset.OriginalFrameCount != 2 {
		t.Fatalf("frame counts = %d/%d, want 2/2", dataset.FrameCount, dataset.OriginalFrameCount)
	}
	if len(dataset.Categories) != 3 {
		t.Fatalf("category count = %d, want 3: %#v", len(dataset.Categories), dataset.Categories)
	}
	if len(dataset.Zones) != 2 {
		t.Fatalf("zone count = %d, want 2: %#v", len(dataset.Zones), dataset.Zones)
	}
	if dataset.Zones[0].Name != "ZONE ONE" || dataset.Zones[0].Temperature[1] != 21.5 {
		t.Fatalf("first zone = %#v", dataset.Zones[0])
	}
	if dataset.Labels[1] != "01-01 02:00" {
		t.Fatalf("labels = %#v", dataset.Labels)
	}
}

func TestParseSimulationHeatFlowSQLBuildsDataset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergyPlusSQL(t, path)

	dataset, err := parseSimulationHeatFlowSQL(path)
	if err != nil {
		t.Fatal(err)
	}
	if dataset.FrameCount != 2 || dataset.OriginalFrameCount != 2 {
		t.Fatalf("frame counts = %d/%d, want 2/2", dataset.FrameCount, dataset.OriginalFrameCount)
	}
	if len(dataset.Categories) != 3 {
		t.Fatalf("category count = %d, want 3: %#v", len(dataset.Categories), dataset.Categories)
	}
	if len(dataset.Zones) != 2 {
		t.Fatalf("zone count = %d, want 2: %#v", len(dataset.Zones), dataset.Zones)
	}
	if dataset.Zones[0].Name != "ZONE ONE" || dataset.Zones[0].Temperature[1] != 21.5 {
		t.Fatalf("first zone = %#v", dataset.Zones[0])
	}
	if dataset.Labels[1] != "01-01 02:00" {
		t.Fatalf("labels = %#v", dataset.Labels)
	}
	if dataset.SourceFile != "eplusout.sql" {
		t.Fatalf("source file = %q, want eplusout.sql", dataset.SourceFile)
	}
}

func TestReadSimulationOutputsPrefersSQLHeatFlowAndSeries(t *testing.T) {
	dir := t.TempDir()
	createTestEnergyPlusSQL(t, filepath.Join(dir, "eplusout.sql"))
	content := `Program Version,EnergyPlus
2,8,Day of Simulation[],Month[],Day of Month[],DST Indicator[1=yes 0=no],Hour[],StartMinute[],EndMinute[],DayType
10,1,ZONE ONE,Zone Mean Air Temperature [C] !Hourly
11,1,ZONE ONE,Zone Air Heat Balance Internal Convective Heat Gain Rate [W] !Hourly
End of Data Dictionary
2,1, 1, 1, 0, 1, 0.00,60.00,Monday
10,20.0
11,100.0
`
	if err := os.WriteFile(filepath.Join(dir, "eplusout.eso"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "eplusout.err"), []byte("EnergyPlus Completed Successfully"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := &SimulationRunResult{OutputDirectory: dir}
	readSimulationOutputs(result)
	if len(result.HeatFlow.Zones) != 2 {
		t.Fatalf("heat-flow zones = %d, want 2; files = %#v", len(result.HeatFlow.Zones), result.Files)
	}
	if result.HeatFlow.SourceFile != "eplusout.sql" {
		t.Fatalf("source file = %q, want eplusout.sql", result.HeatFlow.SourceFile)
	}
	if len(result.Series) == 0 || result.Series[0].File != "eplusout.sql" {
		t.Fatalf("sql series were not parsed first: %#v", result.Series)
	}
}

func TestReadSimulationOutputsUsesESOHeatFlowFallback(t *testing.T) {
	dir := t.TempDir()
	content := `Program Version,EnergyPlus
2,8,Day of Simulation[],Month[],Day of Month[],DST Indicator[1=yes 0=no],Hour[],StartMinute[],EndMinute[],DayType
10,1,ZONE ONE,Zone Mean Air Temperature [C] !Hourly
11,1,ZONE ONE,Zone Air Heat Balance Internal Convective Heat Gain Rate [W] !Hourly
End of Data Dictionary
2,1, 1, 1, 0, 1, 0.00,60.00,Monday
10,20.0
11,100.0
`
	if err := os.WriteFile(filepath.Join(dir, "eplusout.eso"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "eplusout.err"), []byte("EnergyPlus Completed Successfully"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := &SimulationRunResult{OutputDirectory: dir}
	readSimulationOutputs(result)
	if len(result.HeatFlow.Zones) != 1 {
		t.Fatalf("heat-flow fallback zones = %d, want 1; files = %#v", len(result.HeatFlow.Zones), result.Files)
	}
	if result.HeatFlow.SourceFile != "eplusout.eso" {
		t.Fatalf("source file = %q, want eplusout.eso", result.HeatFlow.SourceFile)
	}
}

func createTestEnergyPlusSQL(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE ReportDataDictionary (
			ReportDataDictionaryIndex INTEGER PRIMARY KEY,
			KeyValue TEXT,
			Name TEXT,
			Units TEXT
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
			(10, 'ZONE ONE', 'Zone Mean Air Temperature', 'C'),
			(11, 'ZONE ONE', 'Zone Air Heat Balance Internal Convective Heat Gain Rate', 'W'),
			(12, 'ZONE ONE', 'Zone Air Heat Balance Surface Convection Rate', 'W'),
			(13, 'ZONE TWO', 'Zone Air Heat Balance Outdoor Air Transfer Rate', 'W')`,
		`INSERT INTO "Time" VALUES
			(1, 1, 1, 1, 0),
			(2, 1, 1, 2, 0)`,
		`INSERT INTO ReportData VALUES
			(1, 1, 10, 20.0),
			(2, 1, 11, 100.0),
			(3, 1, 12, -30.0),
			(4, 1, 13, -12.0),
			(5, 2, 10, 21.5),
			(6, 2, 11, 150.0),
			(7, 2, 12, -45.0),
			(8, 2, 13, -18.0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("sql fixture statement failed: %v\n%s", err, statement)
		}
	}
}

func TestCollectWeatherFoldersGroupsEPWFiles(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "USA")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "Chicago.epw"), []byte("weather"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte("skip"), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings := []string{}
	folders := collectWeatherFolders(nil, []string{dir}, &warnings)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(folders) != 1 {
		t.Fatalf("folder count = %d, want 1", len(folders))
	}
	if folders[0].Label != "USA" || len(folders[0].Files) != 1 {
		t.Fatalf("folder = %+v", folders[0])
	}
}

func TestDefaultSimulationWorkerCountUsesFractionAndMax(t *testing.T) {
	workers := DefaultWorkerCount(SimulationSettings{WorkerFraction: 0.5, MaxWorkers: 1})
	if workers != 1 {
		t.Fatalf("workers = %d, want 1", workers)
	}
}

func TestEnergyPlusInstallationsSortNewestFirst(t *testing.T) {
	installations := []EnergyPlusInstallSetting{
		{Name: "EnergyPlus 23.2", Version: "23.2", AutoDetected: true},
		{Name: "EnergyPlus 9.6", Version: "9.6", AutoDetected: true},
		{Name: "EnergyPlus 25.1", Version: "25.1", AutoDetected: true},
		{Name: "EnergyPlus 24.2", Version: "24.2", AutoDetected: true},
	}

	sortEnergyPlusInstallations(installations)

	got := []string{installations[0].Version, installations[1].Version, installations[2].Version, installations[3].Version}
	want := []string{"25.1", "24.2", "23.2", "9.6"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("versions = %v, want %v", got, want)
		}
	}
}
