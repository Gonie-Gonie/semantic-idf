package simulation

import (
	"database/sql"
	"encoding/json"
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

func TestParseSimulationSQLSeriesBuildsTimedSeries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergyPlusSQL(t, path)

	series, err := parseSimulationSQLSeries(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 4 {
		t.Fatalf("series count = %d, want 4: %#v", len(series), series)
	}
	first := series[0]
	if first.File != "eplusout.sql" || first.Column != "ZONE ONE:Zone Mean Air Temperature [C]" {
		t.Fatalf("first series identity = %+v", first)
	}
	if first.Min != 20 || first.Max != 21.5 || first.Average != 20.75 || first.RowCount != 2 {
		t.Fatalf("first series stats = %+v", first)
	}
	if len(first.Points) != 2 || first.Points[0].Label != "01-01 01:00" || first.Points[1].Label != "01-01 02:00" {
		t.Fatalf("first series points = %#v", first.Points)
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

func TestParseSimulationEnergySQLBuildsDashboard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)

	result, err := parseSimulationEnergySQL(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.FacilityMonthly) != 1 {
		t.Fatalf("facility series count = %d, want 1: %#v", len(result.FacilityMonthly), result.FacilityMonthly)
	}
	if len(result.EndUseMonthly) != 1 {
		t.Fatalf("end-use series count = %d, want 1: %#v", len(result.EndUseMonthly), result.EndUseMonthly)
	}
	if len(result.ZoneMonthly) != 1 {
		t.Fatalf("zone series count = %d, want 1: %#v", len(result.ZoneMonthly), result.ZoneMonthly)
	}
	if result.FacilityMonthly[0].Unit != "kWh" || result.FacilityMonthly[0].Total != 3 {
		t.Fatalf("facility energy = %+v, want 3 kWh", result.FacilityMonthly[0])
	}
	if len(result.FacilityMonthly[0].Points) != 2 || result.FacilityMonthly[0].Points[0].Label != "M1" || result.FacilityMonthly[0].Points[1].Label != "M2" {
		t.Fatalf("facility monthly points = %#v", result.FacilityMonthly[0].Points)
	}
	if result.ZoneMonthly[0].ZoneName != "ZONE ONE" || result.ZoneMonthly[0].Total != 0.5 {
		t.Fatalf("zone energy = %+v, want ZONE ONE 0.5 kWh", result.ZoneMonthly[0])
	}
}

func TestParseSimulationEnergySQLAggregatesRowsByMonth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO "Time" VALUES (3, 1, 15, 1, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES (7, 3, 20, 3600000.0)`); err != nil {
		t.Fatal(err)
	}

	result, err := parseSimulationEnergySQL(path)
	if err != nil {
		t.Fatal(err)
	}
	facility := result.FacilityMonthly[0]
	if facility.Total != 4 {
		t.Fatalf("facility total = %+v, want 4 kWh", facility)
	}
	if len(facility.Points) != 2 || facility.Points[0].Label != "M1" || facility.Points[0].Value != 2 || facility.Points[1].Value != 2 {
		t.Fatalf("aggregated monthly points = %#v", facility.Points)
	}
}

func TestPurposeResultBundleUsesSQLEnergyDashboard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)

	result := &SimulationRunResult{
		Status: "succeeded",
		Files: []SimulationFileInfo{{
			Name: "eplusout.sql",
			Path: path,
			Kind: "sqlite",
		}},
	}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy},
	})

	if len(bundle.Energy.FacilityMonthly) != 1 || bundle.Energy.FacilityMonthly[0].Source != "eplusout.sql" {
		t.Fatalf("bundle energy = %#v", bundle.Energy)
	}
	if len(bundle.Completeness) != 1 || !bundle.Completeness[0].Found || bundle.Completeness[0].Source != "sql" {
		t.Fatalf("bundle completeness = %#v", bundle.Completeness)
	}
}

func TestParseSimulationIntegritySQLBuildsDiagnosticsAndTabular(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestIntegritySQL(t, path)

	result, err := parseSimulationIntegritySQL(path)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasErrorsTable || len(result.Issues) != 2 {
		t.Fatalf("sql issues = %#v", result)
	}
	if result.Issues[1].Severity != "severe" || result.Issues[1].Count != 1 {
		t.Fatalf("severe issue = %#v", result.Issues[1])
	}
	if !result.HasTabularData || len(result.TabularReports) != 1 {
		t.Fatalf("tabular reports = %#v", result.TabularReports)
	}
	report := result.TabularReports[0]
	if report.ReportName != "AnnualBuildingUtilityPerformanceSummary" || report.TableName != "Site and Source Energy" {
		t.Fatalf("report identity = %#v", report)
	}
	if len(report.Columns) != 2 || report.Rows[0].Values["Total Energy [GJ]"] != "12.5" {
		t.Fatalf("report values = %#v", report)
	}
}

func TestPurposeResultBundleUsesSQLIntegrityResult(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestIntegritySQL(t, path)

	result := &SimulationRunResult{
		Status: "succeeded",
		Files: []SimulationFileInfo{{
			Name: "eplusout.sql",
			Path: path,
			Kind: "sqlite",
		}},
	}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeIntegrity},
	})

	if len(bundle.Integrity.SQLIssues) != 2 || len(bundle.Integrity.TabularReports) != 1 {
		t.Fatalf("integrity bundle = %#v", bundle.Integrity)
	}
	if len(bundle.Completeness) != 3 || !bundle.Completeness[1].Found || !bundle.Completeness[2].Found {
		t.Fatalf("integrity completeness = %#v", bundle.Completeness)
	}
}

func TestPurposeResultBundleBuildsHVACLoopSeries(t *testing.T) {
	result := &SimulationRunResult{
		Status: "succeeded",
		Series: []SimulationSeries{
			{File: "eplusout.sql", Column: "Air Supply Inlet:System Node Temperature [C]", Min: 20, Max: 22, Average: 21, Points: []SimulationPoint{{X: 1, Value: 20}, {X: 2, Value: 22}}},
			{File: "eplusout.sql", Column: "Air Supply Inlet:System Node Setpoint Temperature [C]", Min: 21, Max: 21, Average: 21, Points: []SimulationPoint{{X: 1, Value: 21}, {X: 2, Value: 21}}},
			{File: "eplusout.sql", Column: "Air Supply Inlet:System Node Mass Flow Rate [kg/s]", Min: 0.2, Max: 0.4, Average: 0.3, Points: []SimulationPoint{{X: 1, Value: 0.2}, {X: 2, Value: 0.4}}},
			{File: "eplusout.sql", Column: "Fan Outlet:System Node Mass Flow Rate [kg/s]", Min: 0, Max: 0, Average: 0, Points: []SimulationPoint{{X: 1, Value: 0}, {X: 2, Value: 0}}},
			{File: "eplusout.sql", Column: "Supply Fan:Fan Electricity Rate [W]", Min: 120, Max: 260, Average: 190, Points: []SimulationPoint{{X: 1, Value: 120}, {X: 2, Value: 260}}},
			{File: "eplusout.sql", Column: "Supply Fan:Fan Electricity Energy [J]", Min: 432000, Max: 936000, Average: 684000, Points: []SimulationPoint{{X: 1, Value: 432000}, {X: 2, Value: 936000}}},
			{File: "eplusout.sql", Column: "Office:Zone Mean Air Temperature [C]", Min: 21, Max: 21, Average: 21, Points: []SimulationPoint{{X: 1, Value: 21}}},
		},
	}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck},
		Scope: SimulationPurposeScope{
			AirLoopNames: []string{"Main Air Loop"},
		},
	})

	if len(bundle.HVACLoops) != 1 || bundle.HVACLoops[0].Name != "Main Air Loop" {
		t.Fatalf("hvac loop result = %#v", bundle.HVACLoops)
	}
	if len(bundle.HVACLoops[0].Series) != 4 {
		t.Fatalf("hvac series = %#v", bundle.HVACLoops[0].Series)
	}
	if len(bundle.HVACLoops[0].NodeSummaries) != 2 {
		t.Fatalf("node summaries = %#v", bundle.HVACLoops[0].NodeSummaries)
	}
	if len(bundle.HVACLoops[0].Components) != 1 || bundle.HVACLoops[0].Components[0].ComponentName != "Supply Fan" {
		t.Fatalf("component summaries = %#v", bundle.HVACLoops[0].Components)
	}
	if len(bundle.HVACLoops[0].Components[0].Metrics) != 2 {
		t.Fatalf("component metrics = %#v", bundle.HVACLoops[0].Components[0].Metrics)
	}
	if len(bundle.HVACLoops[0].DerivedMetrics) == 0 {
		t.Fatalf("derived metrics missing: %#v", bundle.HVACLoops[0])
	}
	if !hvacLoopAlertExists(bundle.HVACLoops[0].Alerts, "no_detected_mass_flow") {
		t.Fatalf("expected zero-flow alert: %#v", bundle.HVACLoops[0].Alerts)
	}
	if len(bundle.Completeness) != 2 || !bundle.Completeness[0].Found || !bundle.Completeness[1].Found || bundle.Completeness[0].Source != "sql" {
		t.Fatalf("hvac completeness = %#v", bundle.Completeness)
	}
	if len(bundle.HVACLoops[0].Completeness) != len(hvacLoopCheckNodeVariables())+1 {
		t.Fatalf("node completeness = %#v", bundle.HVACLoops[0].Completeness)
	}
}

func TestPurposeResultBundleBuildsComfortResult(t *testing.T) {
	result := &SimulationRunResult{
		Status: "succeeded",
		Series: []SimulationSeries{
			{File: "eplusout.sql", Column: "Office:Zone Mean Air Temperature [C]", Min: 20, Max: 24, Average: 22, Points: []SimulationPoint{{X: 1, Value: 22}}},
			{File: "eplusout.sql", Column: "Office:Zone Thermostat Cooling Setpoint Temperature [C]", Min: 26, Max: 26, Average: 26, Points: []SimulationPoint{{X: 1, Value: 26}}},
			{File: "eplusout.sql", Column: "Air Supply Inlet:System Node Temperature [C]", Min: 12, Max: 14, Average: 13, Points: []SimulationPoint{{X: 1, Value: 13}}},
		},
	}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeComfort},
	})

	if len(bundle.Comfort.Zones) != 1 || bundle.Comfort.Zones[0].ZoneName != "Office" {
		t.Fatalf("comfort zones = %#v", bundle.Comfort.Zones)
	}
	if len(bundle.Comfort.Series) != 2 || len(bundle.Comfort.Zones[0].Metrics) != 2 {
		t.Fatalf("comfort series = %#v", bundle.Comfort)
	}
	if len(bundle.Completeness) != 1 || !bundle.Completeness[0].Found || bundle.Completeness[0].Source != "sql" {
		t.Fatalf("comfort completeness = %#v", bundle.Completeness)
	}
}

func hvacLoopAlertExists(alerts []HVACLoopAlert, code string) bool {
	for _, alert := range alerts {
		if alert.Code == code {
			return true
		}
	}
	return false
}

func TestWriteSimulationRunManifest(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "in.idf")
	if err := os.WriteFile(inputPath, []byte("Version, 24.1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := &SimulationRunResult{
		RunID:           "sim-test",
		Status:          "succeeded",
		InputPath:       inputPath,
		Filename:        "in.idf",
		OutputDirectory: dir,
		StartedAt:       "2026-06-10T00:00:00Z",
		FinishedAt:      "2026-06-10T00:00:01Z",
		Files: []SimulationFileInfo{{
			Name: "eplusout.sql",
			Path: filepath.Join(dir, "eplusout.sql"),
			Kind: "sqlite",
			Size: 120,
		}},
	}
	plan := &PurposeRunPlan{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, EstimatedWeight: "Light"}
	request := SimulationRunRequest{
		PurposeRequest: &SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}},
		PurposeRunPlan: plan,
		ResultMode:     "sql_first",
	}

	writeSimulationRunManifest(result, request)

	path := filepath.Join(dir, "idf-analyzer-run.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest SimulationRunManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.RunID != "sim-test" || manifest.Status != "succeeded" || manifest.InputHash == "" {
		t.Fatalf("manifest core fields = %+v", manifest)
	}
	if len(manifest.Purposes) != 1 || manifest.Purposes[0] != SimulationPurposeBasicEnergy {
		t.Fatalf("manifest purposes = %#v", manifest.Purposes)
	}
	if manifest.OutputPlan == nil || manifest.OutputPlan.EstimatedWeight != "Light" {
		t.Fatalf("manifest output plan = %#v", manifest.OutputPlan)
	}
	if len(result.Files) != 2 || !simulationResultHasFileKind(result.Files, "manifest") {
		t.Fatalf("result files = %#v", result.Files)
	}
}

func TestWritePurposeRunArtifacts(t *testing.T) {
	dir := t.TempDir()
	request := SimulationRunRequest{
		PurposeRunPlan:      &PurposeRunPlan{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, EstimatedWeight: "Light"},
		TemporaryOutputDiff: "--- original.idf\n+++ purpose-run-copy.idf\n+Output:SQLite,\n+  SimpleAndTabular;  !- Option Type\n",
	}

	writePurposeRunArtifacts(dir, request)

	if _, err := os.Stat(filepath.Join(dir, "idf-analyzer-run-plan.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "temporary_outputs.diff")); err != nil {
		t.Fatal(err)
	}
	files := collectSimulationFiles(dir)
	if !simulationResultHasFileKind(files, "run_plan") || !simulationResultHasFileKind(files, "temporary_output_diff") {
		t.Fatalf("artifact file kinds = %#v", files)
	}
}

func TestReadSimulationOutputsEmitsDetailedProgressPhases(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "eplusout.err"), []byte("EnergyPlus Completed Successfully"), 0o644); err != nil {
		t.Fatal(err)
	}
	var phases []string
	result := &SimulationRunResult{OutputDirectory: dir}

	readSimulationOutputsWithProgress(result, "sim-progress", func(progress SimulationProgress) {
		phases = append(phases, progress.Phase)
	}, "")

	if !stringSliceContains(phases, "parse_sql") || !stringSliceContains(phases, "parse_fallback") {
		t.Fatalf("progress phases = %#v", phases)
	}
}

func simulationResultHasFileKind(files []SimulationFileInfo, kind string) bool {
	for _, file := range files {
		if file.Kind == kind {
			return true
		}
	}
	return false
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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

func createTestEnergySQL(t *testing.T, path string) {
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
			(20, '', 'Electricity:Facility', 'J'),
			(21, '', 'Electricity:Cooling', 'J'),
			(22, 'ZONE ONE', 'Zone Lights Electricity Energy', 'J')`,
		`INSERT INTO "Time" VALUES
			(1, 1, 31, 24, 0),
			(2, 2, 28, 24, 0)`,
		`INSERT INTO ReportData VALUES
			(1, 1, 20, 3600000.0),
			(2, 2, 20, 7200000.0),
			(3, 1, 21, 1800000.0),
			(4, 2, 21, 1800000.0),
			(5, 1, 22, 900000.0),
			(6, 2, 22, 900000.0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("sql fixture statement failed: %v\n%s", err, statement)
		}
	}
}

func createTestIntegritySQL(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE Errors (
			ErrorIndex INTEGER PRIMARY KEY,
			ErrorType TEXT,
			ErrorMessage TEXT,
			Count INTEGER
		)`,
		`CREATE TABLE TabularDataWithStrings (
			ReportName TEXT,
			ReportForString TEXT,
			TableName TEXT,
			RowName TEXT,
			ColumnName TEXT,
			Units TEXT,
			RowId INTEGER,
			ColumnId INTEGER,
			Value TEXT
		)`,
		`INSERT INTO Errors VALUES
			(1, 'Warning', 'Calculated design day warning', 2),
			(2, 'Severe', 'Node connection problem', 1)`,
		`INSERT INTO TabularDataWithStrings VALUES
			('AnnualBuildingUtilityPerformanceSummary', 'Entire Facility', 'Site and Source Energy', 'Total Site Energy', 'Total Energy', 'GJ', 1, 1, '12.5'),
			('AnnualBuildingUtilityPerformanceSummary', 'Entire Facility', 'Site and Source Energy', 'Total Site Energy', 'Energy Per Total Building Area', 'MJ/m2', 1, 2, '85.1')`,
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
