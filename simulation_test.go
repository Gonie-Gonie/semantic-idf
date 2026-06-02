package main

import (
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
	workers := defaultSimulationWorkerCount(SimulationSettings{WorkerFraction: 0.5, MaxWorkers: 1})
	if workers != 1 {
		t.Fatalf("workers = %d, want 1", workers)
	}
}
