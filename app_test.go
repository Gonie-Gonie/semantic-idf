package main

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
	"github.com/Gonie-Gonie/semantic-idf/internal/simulation"
)

const appSummaryIDF = `
Version,
  24.1;                    !- Version Identifier

Zone,
  Office;                  !- Name
`

func TestAnalyzeInputTextIncludesSummary(t *testing.T) {
	app := NewApp()
	result, err := app.AnalyzeInputText(appSummaryIDF)
	if err != nil {
		t.Fatalf("AnalyzeInputText() error = %v", err)
	}
	if result.Report == nil {
		t.Fatalf("AnalyzeInputText() report = nil")
	}
	if result.Report.Summary.MetricCount != 59 {
		t.Fatalf("summary metric count = %d, want 59", result.Report.Summary.MetricCount)
	}
	if len(result.Report.Summary.Categories) != 6 {
		t.Fatalf("summary category count = %d, want 6", len(result.Report.Summary.Categories))
	}
	if result.Report.Geometry.ZoneCount != 1 {
		t.Fatalf("geometry zone count = %d, want 1", result.Report.Geometry.ZoneCount)
	}
}

func TestAnalyzeInputTextUsesCacheForSameInput(t *testing.T) {
	app := NewApp()
	first, err := app.AnalyzeInputText(appSummaryIDF)
	if err != nil {
		t.Fatalf("first AnalyzeInputText() error = %v", err)
	}
	if first.AnalysisKey == "" {
		t.Fatalf("first analysis key is empty")
	}
	if first.Timing == nil {
		t.Fatalf("first timing = nil")
	}
	if first.Timing.CacheHit {
		t.Fatalf("first analysis unexpectedly reported cache hit")
	}
	_, hasSummaryTiming := first.Timing.Stages["summary"]
	_, hasCoreTiming := first.Timing.Stages["core"]
	if !hasSummaryTiming || !hasCoreTiming {
		t.Fatalf("first analysis did not report stage timings: %+v", first.Timing)
	}

	second, err := app.AnalyzeInputText(appSummaryIDF)
	if err != nil {
		t.Fatalf("second AnalyzeInputText() error = %v", err)
	}
	if second.AnalysisKey != first.AnalysisKey {
		t.Fatalf("analysis key changed: %q != %q", second.AnalysisKey, first.AnalysisKey)
	}
	if second.Timing == nil || !second.Timing.CacheHit {
		t.Fatalf("second analysis timing = %+v, want cache hit", second.Timing)
	}
	if second.Report == nil || second.Report.Summary.MetricCount != first.Report.Summary.MetricCount {
		t.Fatalf("cached report summary = %+v, want metric count %d", second.Report, first.Report.Summary.MetricCount)
	}
}

func TestGetCachedAnalysisAssemblesCompletedStageResults(t *testing.T) {
	app := NewApp()
	quick, err := app.AnalyzeInputQuickText(appSummaryIDF)
	if err != nil {
		t.Fatalf("AnalyzeInputQuickText() error = %v", err)
	}
	if quick.AnalysisKey == "" {
		t.Fatalf("quick analysis key is empty")
	}
	if quick.Report == nil || quick.Report.Geometry.ZoneCount != 0 || len(quick.Report.Diagnostics) != 0 {
		t.Fatalf("quick report should not include heavy stages: %+v", quick.Report)
	}

	for _, stage := range []string{"profile", "hvac", "output", "diagnostics", "geometry"} {
		if _, err := app.AnalyzeInputStageText(appSummaryIDF, stage); err != nil {
			t.Fatalf("AnalyzeInputStageText(%q) error = %v", stage, err)
		}
	}
	cached, err := app.GetCachedAnalysis(quick.AnalysisKey)
	if err != nil {
		t.Fatalf("GetCachedAnalysis() error = %v", err)
	}
	if cached == nil {
		t.Fatalf("GetCachedAnalysis() = nil, want assembled result")
	}
	if cached.Timing == nil || cached.Timing.Mode != "full" || !cached.Timing.CacheHit {
		t.Fatalf("cached timing = %+v, want full cache hit", cached.Timing)
	}
	if cached.Report == nil || cached.Report.Geometry.ZoneCount != 1 {
		t.Fatalf("cached geometry zone count = %+v, want 1", cached.Report)
	}
	if cached.Report.Summary.MetricCount != quick.Report.Summary.MetricCount {
		t.Fatalf("cached summary metric count = %d, want %d", cached.Report.Summary.MetricCount, quick.Report.Summary.MetricCount)
	}
}

func TestMaxAnalysisWorkersIsCapped(t *testing.T) {
	workers := idf.MaxAnalysisWorkers()
	if workers < 1 || workers > 4 {
		t.Fatalf("MaxAnalysisWorkers() = %d, want 1..4", workers)
	}
}

func TestAnalysisCacheSharesInFlightComputation(t *testing.T) {
	cache := NewAnalysisCache(4)
	key := analysisCacheKey{TextHash: "same", Format: "idf", EnergyPlusVersion: "24.1", AnalyzerVersion: "test", Mode: "full", SettingsHash: "default"}
	var calls int32
	compute := func() (*InputAnalysisResult, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(20 * time.Millisecond)
		return &InputAnalysisResult{
			AnalysisKey: key.TextHash,
			Format:      key.Format,
			Version:     key.EnergyPlusVersion,
			Report:      &idf.Report{},
		}, nil
	}

	var wg sync.WaitGroup
	results := make([]*InputAnalysisResult, 2)
	for index := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, _, _, err := cache.GetOrCompute(key, compute)
			if err != nil {
				t.Errorf("GetOrCompute() error = %v", err)
			}
			results[index] = result
		}(index)
	}
	wg.Wait()

	if calls != 1 {
		t.Fatalf("compute calls = %d, want 1", calls)
	}
	if results[0] == nil || results[1] == nil || results[0].AnalysisKey != "same" || results[1].AnalysisKey != "same" {
		t.Fatalf("shared results = %#v", results)
	}
}

func TestParseBatchInputReusesContentHashCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.idf")
	if err := os.WriteFile(path, []byte(appSummaryIDF), 0o644); err != nil {
		t.Fatal(err)
	}
	firstModel, firstDoc, err := parseBatchInput(path)
	if err != nil {
		t.Fatalf("first parseBatchInput() error = %v", err)
	}
	secondModel, secondDoc, err := parseBatchInput(path)
	if err != nil {
		t.Fatalf("second parseBatchInput() error = %v", err)
	}
	if firstModel != secondModel {
		t.Fatalf("parseBatchInput() did not reuse cached model")
	}
	if len(firstDoc.Objects) != len(secondDoc.Objects) {
		t.Fatalf("cached doc object count = %d, want %d", len(secondDoc.Objects), len(firstDoc.Objects))
	}
}

func TestThrottleMultiSummaryProgressKeepsFinalEvent(t *testing.T) {
	var emitted []MultiSummaryProgress
	throttled := throttleMultiSummaryProgress(func(progress MultiSummaryProgress) {
		emitted = append(emitted, progress)
	})
	throttled(MultiSummaryProgress{RunID: "batch", Total: 3, Completed: 1})
	throttled(MultiSummaryProgress{RunID: "batch", Total: 3, Completed: 2})
	throttled(MultiSummaryProgress{RunID: "batch", Total: 3, Completed: 3})
	if len(emitted) != 2 {
		t.Fatalf("emitted progress count = %d, want first and final events", len(emitted))
	}
	if emitted[len(emitted)-1].Completed != 3 {
		t.Fatalf("final progress event = %+v, want completed=3", emitted[len(emitted)-1])
	}
}

func TestSlimReportForModeDefersHVACRuleGraphExceptDebug(t *testing.T) {
	report := &idf.Report{
		HVAC: idf.HVACReport{
			RuleGraph: idf.HVACRuleGraph{
				Nodes: []idf.HVACRuleNode{{ID: "node", Label: "Node"}},
				Edges: []idf.HVACRuleEdge{{ID: "edge", RuleID: "rule", FromID: "node", ToID: "other", EdgeKind: "test"}},
			},
		},
	}
	slimReportForMode(report, "hvac")
	if len(report.HVAC.RuleGraph.Nodes) != 0 || len(report.HVAC.RuleGraph.Edges) != 0 {
		t.Fatalf("default slim report kept rule graph: %+v", report.HVAC.RuleGraph)
	}

	debugReport := &idf.Report{
		HVAC: idf.HVACReport{
			RuleGraph: idf.HVACRuleGraph{
				Nodes: []idf.HVACRuleNode{{ID: "node", Label: "Node"}},
				Edges: []idf.HVACRuleEdge{{ID: "edge", RuleID: "rule", FromID: "node", ToID: "other", EdgeKind: "test"}},
			},
		},
	}
	slimReportForMode(debugReport, "hvac-debug")
	if len(debugReport.HVAC.RuleGraph.Nodes) != 1 || len(debugReport.HVAC.RuleGraph.Edges) != 1 {
		t.Fatalf("debug slim report removed rule graph: %+v", debugReport.HVAC.RuleGraph)
	}
}

func TestAnalyzeBatchDiagnosePathsKeepsParseFailuresInResult(t *testing.T) {
	dir := t.TempDir()
	okPath := filepath.Join(dir, "ok.idf")
	badPath := filepath.Join(dir, "bad.epjson")
	if err := os.WriteFile(okPath, []byte(appSummaryIDF), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badPath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := AnalyzeBatchDiagnosePaths(BatchJobRequest{RunID: "diagnose-test", InputPaths: []string{okPath, badPath}})
	if result.Total != 2 || result.Completed != 2 || result.Succeeded != 1 || result.Failed != 1 {
		t.Fatalf("batch diagnose counts = %+v", result)
	}
	if len(result.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(result.Files))
	}
}

func TestAnalyzeBatchOutputQAReportsInventoryAndReadiness(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outputs.idf")
	text := appSummaryIDF + `
Output:SQLite,
  SimpleAndTabular;

Output:VariableDictionary,
  Regular;

Output:Variable,
  *,
  Zone Mean Air Temperature,
  Detailed;

Output:Variable,
  *,
  Zone Mean Air Temperature,
  Detailed;
`
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	result := AnalyzeBatchOutputQAPaths(BatchJobRequest{RunID: "output-test", InputPaths: []string{path}})
	if result.Succeeded != 1 || len(result.Files) != 1 {
		t.Fatalf("output qa result = %+v", result)
	}
	file := result.Files[0]
	if !file.SQLitePresent || !file.VariableDictionary || file.OutputVariableCount != 2 || file.DuplicateOutputCount != 1 || file.DetailedOrTimestepCount != 2 {
		t.Fatalf("output qa file = %+v", file)
	}
	if file.PurposeReadiness["basic_energy"] {
		t.Fatalf("basic energy readiness should report missing purpose outputs: %+v", file.PurposeReadiness)
	}
}

func TestConvertExportBatchRenamePolicyPreservesExistingOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.idf")
	if err := os.WriteFile(path, []byte(appSummaryIDF), 0o644); err != nil {
		t.Fatal(err)
	}
	request := BatchConvertExportRequest{
		BatchJobRequest: BatchJobRequest{RunID: "convert-test", InputPaths: []string{path}},
		TargetFormat:    "epjson",
		OutputDirectory: dir,
		OverwritePolicy: "rename",
	}
	first := ConvertExportBatch(request)
	second := ConvertExportBatch(request)
	if first.Succeeded != 1 || second.Succeeded != 1 {
		t.Fatalf("convert results = %+v / %+v", first, second)
	}
	if first.Files[0].OutputPath == second.Files[0].OutputPath {
		t.Fatalf("rename policy reused output path %q", first.Files[0].OutputPath)
	}
}

func TestConvertExportBatchWritesXLSXTables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.idf")
	if err := os.WriteFile(path, []byte(appSummaryIDF), 0o644); err != nil {
		t.Fatal(err)
	}
	result := ConvertExportBatch(BatchConvertExportRequest{
		BatchJobRequest: BatchJobRequest{RunID: "convert-xlsx-test", InputPaths: []string{path}},
		TargetFormat:    "xlsx",
		OutputDirectory: dir,
		OverwritePolicy: "fail",
	})
	if result.Succeeded != 1 || len(result.Files) != 1 {
		t.Fatalf("xlsx convert result = %+v", result)
	}
	if filepath.Ext(result.Files[0].OutputPath) != ".xlsx" {
		t.Fatalf("xlsx output path = %q", result.Files[0].OutputPath)
	}
	archive, err := zip.OpenReader(result.Files[0].OutputPath)
	if err != nil {
		t.Fatalf("open xlsx archive: %v", err)
	}
	defer archive.Close()
	foundSheet := false
	for _, file := range archive.File {
		if file.Name == "xl/worksheets/sheet1.xml" {
			foundSheet = true
			break
		}
	}
	if !foundSheet {
		t.Fatalf("xlsx archive missing sheet1.xml")
	}
}

func TestBatchSummaryWorkbookSheetsIncludeDelta(t *testing.T) {
	result := MultiSummaryResult{
		Metrics: []MultiSummaryMetric{{ID: "total_wwr", CSVName: "total_wwr [%]", Category: "Envelope", Unit: "%"}},
		Files: []MultiSummaryFile{
			{Index: 0, Filename: "a.idf", Label: "a.idf", Status: "ok", MetricValues: map[string]MultiSummaryValue{"total_wwr": {DisplayValue: "10", Status: "ok"}}},
			{Index: 1, Filename: "b.idf", Label: "b.idf", Status: "ok", MetricValues: map[string]MultiSummaryValue{"total_wwr": {DisplayValue: "15", Status: "ok"}}},
		},
	}
	sheets := batchSummaryWorkbookSheets(BatchSummaryXLSXExportRequest{
		Result:        result,
		Orientation:   "metrics",
		BaselineIndex: 0,
		CompareIndex:  1,
	})
	if len(sheets) != 2 || sheets[0].Name != "Raw" || sheets[1].Name != "Delta" {
		t.Fatalf("workbook sheets = %#v", sheets)
	}
	deltaRows := sheets[1].Sections[0].Rows
	if len(deltaRows) < 3 || deltaRows[2][5] != "+5 pt" || deltaRows[2][6] != "50%" {
		t.Fatalf("delta rows = %#v", deltaRows)
	}
}

func TestBatchSimulationWorkbookSheetsIncludesPurposeAndEnergySheets(t *testing.T) {
	result := simulation.MultiSimulationResult{
		RunID:     "sim-xlsx-test",
		Total:     1,
		Completed: 1,
		Succeeded: 1,
		Results: []simulation.SimulationRunResult{{
			RunID:    "run-a",
			Status:   "succeeded",
			Filename: "a.idf",
			PurposeMetrics: []simulation.PurposeMetric{{
				ID:           "energy_explanation.kpi.cooling_cop",
				Label:        "Derived KPI: Cooling COP",
				Value:        2,
				DisplayValue: "2",
				Status:       "ok",
			}},
			PurposeResults: &simulation.PurposeResultBundle{
				EnergyExplanationSummary: simulation.EnergyExplanationSummary{
					Schema: "semantic-idf.energy-explanation-summary/v1",
					DerivedKPIs: []simulation.EnergyExplanationSummaryItem{{
						ID:          "kpi.cooling_cop",
						Level:       "derived_kpi",
						Label:       "Cooling COP",
						Value:       2,
						ServiceKind: "cooling",
						PathType:    "zone",
					}},
				},
				EnergyExplanation: simulation.EnergyExplanationResult{
					Schema: "semantic-idf.energy-explanation/v1",
					Sources: []simulation.EnergyDataSource{{
						ID:             "sql-rdd-1",
						SourceType:     "sql_report_data",
						IsMeter:        true,
						Name:           "Cooling:Electricity",
						SourceUnit:     "J",
						NormalizedUnit: "kWh",
					}},
					Periods: []simulation.EnergyPeriod{{
						ID:   "annual",
						Kind: "annual",
						Edges: []simulation.EnergyExplanationEdge{{
							ID:       "edge-1",
							FromID:   "energy.carrier.electricity",
							ToID:     "energy.end_use.cooling.electricity",
							Value:    1,
							Unit:     "kWh",
							Relation: "meter_enduse",
							Basis:    "measured_meter",
						}},
						Reconciliation: []simulation.EnergyReconciliation{{
							ID:             "reconcile.energy.electricity.annual",
							Level:          "energy",
							Period:         "annual",
							Label:          "Electricity total basis",
							ExpectedValue:  2,
							ExplainedValue: 1,
							ResidualValue:  1,
							Unit:           "kWh",
							Basis:          "meter_reconciliation",
						}},
					}},
				},
			},
		}},
	}
	sheets := batchSimulationWorkbookSheets(BatchSimulationXLSXExportRequest{Result: result})
	if len(sheets) != 5 || sheets[0].Name != "Purpose Metrics" || sheets[1].Name != "Energy Summary" || sheets[4].Name != "Reconciliation" {
		t.Fatalf("simulation workbook sheets = %#v", sheets)
	}
	if rows := sheets[0].Sections[0].Rows; len(rows) != 1 || rows[0][3] != "energy_explanation.kpi.cooling_cop" {
		t.Fatalf("purpose metric rows = %#v", rows)
	}
	if rows := sheets[1].Sections[0].Rows; len(rows) != 1 || rows[0][3] != "derived_kpi" || rows[0][10] != "zone" {
		t.Fatalf("energy summary rows = %#v", rows)
	}
}

func TestCreateBatchSafeCleanupCopiesWritesCleanedCopyOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cleanup.idf")
	text := appSummaryIDF + `
Schedule:Constant,
  Unused,
  ,
  1;
`
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	result := CreateBatchSafeCleanupCopies(BatchConvertExportRequest{
		BatchJobRequest: BatchJobRequest{RunID: "cleanup-copy-test", InputPaths: []string{path}},
		OutputDirectory: dir,
		OverwritePolicy: "rename",
	})
	if result.Succeeded != 1 || len(result.Files) != 1 || result.Files[0].OutputPath == "" {
		t.Fatalf("cleanup copy result = %+v", result)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cleaned, err := os.ReadFile(result.Files[0].OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(original), "Unused") {
		t.Fatalf("original was unexpectedly changed:\n%s", string(original))
	}
	if strings.Contains(string(cleaned), "Unused") {
		t.Fatalf("cleaned copy still contains unused schedule:\n%s", string(cleaned))
	}
}

func TestAnalyzeBatchSimulationPlanReportsPurposeWeight(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.idf")
	if err := os.WriteFile(path, []byte(appSummaryIDF), 0o644); err != nil {
		t.Fatal(err)
	}
	purpose := simulation.NormalizeSimulationPurposeRequest(&simulation.SimulationPurposeRequest{
		Purposes: []simulation.SimulationPurposeID{simulation.SimulationPurposeBasicEnergy},
	})
	result := AnalyzeBatchSimulationPlan(simulation.MultiSimulationRequest{
		InputPaths:     []string{path},
		WorkerCount:    1,
		WeatherMode:    "same",
		PurposeRequest: &purpose,
	})
	if result.Total != 1 || result.Succeeded != 1 || len(result.Files) != 1 {
		t.Fatalf("plan preview result = %+v", result)
	}
	file := result.Files[0]
	if file.OutputCount == 0 || file.TemporaryOutputCount == 0 || result.CommonOutputCount == 0 {
		t.Fatalf("plan preview file = %+v common=%d", file, result.CommonOutputCount)
	}
}

func TestDefaultEnergyPlusSampleAnalyzes(t *testing.T) {
	content, err := os.ReadFile("frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf")
	if err != nil {
		t.Fatalf("ReadFile(default sample) error = %v", err)
	}
	result, err := NewApp().AnalyzeInputText(string(content))
	if err != nil {
		t.Fatalf("AnalyzeInputText(default sample) error = %v", err)
	}
	if result.Report.ObjectCount < 100 {
		t.Fatalf("default sample object count = %d, want a complex example", result.Report.ObjectCount)
	}
	if result.Report.Geometry.ZoneCount < 10 || result.Report.Geometry.SurfaceCount < 50 || result.Report.Geometry.WindowCount < 10 {
		t.Fatalf("default sample geometry too small: zones=%d surfaces=%d windows=%d", result.Report.Geometry.ZoneCount, result.Report.Geometry.SurfaceCount, result.Report.Geometry.WindowCount)
	}
}

func TestReplaceOutputManagementObjectsPreservesNonOutputText(t *testing.T) {
	original := `
Version,
  24.2;

Building,
  Test Building,
  0,
  Suburbs,
  ,
  ,
  MinimalShadowing;

GlobalGeometryRules,
  UpperLeftCorner,
  CounterClockWise,
  World;

Zone,
  Office;

Output:Variable,
  *,
  Zone Mean Air Temperature,
  Hourly;
`
	doc, err := idf.Parse(original)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	updated, preview := idf.ApplyOutput(doc, idf.OutputApplyRequest{
		AddRecommendations:  []string{"sqlite-simple-tabular"},
		RemoveObjectIndexes: []int{4},
	})
	if !preview.CanApply {
		t.Fatalf("preview blocked: %#v", preview.Warnings)
	}
	patched := replaceOutputManagementObjects(original, updated)
	if !strings.Contains(patched, "Building,\n  Test Building") {
		t.Fatalf("patched text lost Building object:\n%s", patched)
	}
	if !strings.Contains(patched, "GlobalGeometryRules,") {
		t.Fatalf("patched text lost GlobalGeometryRules:\n%s", patched)
	}
	if strings.Contains(patched, "Zone Mean Air Temperature") {
		t.Fatalf("patched text kept removed output variable:\n%s", patched)
	}
	if !strings.Contains(patched, "Output:SQLite,") {
		t.Fatalf("patched text did not append Output:SQLite:\n%s", patched)
	}
}

func TestPreparePurposeSimulationRequestUsesRunCopy(t *testing.T) {
	original := appSummaryIDF + `
OutputControl:Table:Style,
  Comma,                   !- Column Separator
  JtoKWH;                  !- Unit Conversion
`
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.idf")
	if err := os.WriteFile(inputPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write input fixture: %v", err)
	}

	purposeRequest := simulation.NormalizeSimulationPurposeRequest(&simulation.SimulationPurposeRequest{
		Purposes: []simulation.SimulationPurposeID{simulation.SimulationPurposeBasicEnergy},
	})
	prepared, err := preparePurposeSimulationRequest(simulation.SimulationRunRequest{
		InputPath:       inputPath,
		Filename:        "input.idf",
		PurposeRequest:  &purposeRequest,
		UseReadVarsESO:  false,
		StandardOutput:  false,
		OutputDirectory: tempDir,
	})
	if err != nil {
		t.Fatalf("preparePurposeSimulationRequest() error = %v", err)
	}
	content, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read original input: %v", err)
	}
	if string(content) != original {
		t.Fatalf("preparePurposeSimulationRequest mutated original file:\n%s", string(content))
	}
	if prepared.Text == "" || prepared.Text == original {
		t.Fatalf("prepared run copy text was not expanded:\n%s", prepared.Text)
	}
	if !strings.Contains(prepared.Text, "Output:SQLite") || !strings.Contains(prepared.Text, "Zone Lights Electricity Energy") {
		t.Fatalf("prepared run copy is missing purpose outputs:\n%s", prepared.Text)
	}
	if strings.Contains(prepared.Text, "OutputControl:Table:Style 1") {
		t.Fatalf("prepared run copy inserted a synthetic OutputControl name:\n%s", prepared.Text)
	}
	if prepared.PurposeRunPlan == nil || len(prepared.PurposeRunPlan.OutputObjects) == 0 {
		t.Fatalf("prepared run plan was not attached: %#v", prepared.PurposeRunPlan)
	}
	if !strings.Contains(prepared.TemporaryOutputDiff, "purpose-run-copy.idf") {
		t.Fatalf("temporary output diff missing run-copy marker:\n%s", prepared.TemporaryOutputDiff)
	}
	if prepared.ResultMode != "sql_first" {
		t.Fatalf("prepared result mode = %q, want sql_first", prepared.ResultMode)
	}
}

func TestApplyPurposeOutputsTextUsesOutputPipeline(t *testing.T) {
	result, err := NewApp().ApplyPurposeOutputsText(appSummaryIDF, simulation.SimulationPurposeRequest{
		Purposes: []simulation.SimulationPurposeID{simulation.SimulationPurposeZoneHeatFlow},
	})
	if err != nil {
		t.Fatalf("ApplyPurposeOutputsText() error = %v", err)
	}
	if !result.Preview.CanApply {
		t.Fatalf("purpose output preview blocked: %#v", result.Preview.Warnings)
	}
	if !outputApplyPreviewHasAction(result.Preview.Changes, "add_output", "Output:Variable") {
		t.Fatalf("preview changes do not include Output:Variable add: %#v", result.Preview.Changes)
	}
	if !strings.Contains(result.Text, "Output:SQLite") || !strings.Contains(result.Text, "Zone Mean Air Temperature") {
		t.Fatalf("applied text is missing purpose outputs:\n%s", result.Text)
	}
	if result.Report == nil || result.Report.Output.VariableCount == 0 || result.Report.Output.ObjectCount == 0 {
		t.Fatalf("applied report output summary not populated: %#v", result.Report)
	}
}

func TestAppAssetHandlerServesSummaryMetricGuides(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/summary-metric-guides", nil)
	response := httptest.NewRecorder()

	appAssetHandler(NewApp()).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("summary metric guide API status = %d, want %d", response.Code, http.StatusOK)
	}
	var guides []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&guides); err != nil {
		t.Fatalf("summary metric guide API did not return JSON: %v", err)
	}
	if len(guides) != 59 {
		t.Fatalf("summary metric guide API returned %d guides, want 59", len(guides))
	}
}

func outputApplyPreviewHasAction(changes []idf.OutputApplyChange, action string, objectType string) bool {
	for _, change := range changes {
		if change.Action == action && change.ObjectType == objectType {
			return true
		}
	}
	return false
}

func TestAnalyzeMultiSummaryPaths(t *testing.T) {
	tempDir := t.TempDir()
	first := filepath.Join(tempDir, "first.idf")
	second := filepath.Join(tempDir, "second.idf")
	if err := os.WriteFile(first, []byte(`Version, 24.1;
Building, Alpha;
Zone, Office;
`), 0o644); err != nil {
		t.Fatalf("write first fixture: %v", err)
	}
	if err := os.WriteFile(second, []byte(`Version, 24.1;
Building, Beta;
Zone, Core;
Zone, Perimeter;
`), 0o644); err != nil {
		t.Fatalf("write second fixture: %v", err)
	}

	var progress []MultiSummaryProgress
	result := analyzeMultiSummaryPaths([]string{first, second}, "test-run", func(item MultiSummaryProgress) {
		progress = append(progress, item)
	})

	if result.Total != 2 || result.Completed != 2 || result.Succeeded != 2 || result.Failed != 0 {
		t.Fatalf("multi summary counts = total:%d completed:%d succeeded:%d failed:%d", result.Total, result.Completed, result.Succeeded, result.Failed)
	}
	if len(progress) != 2 {
		t.Fatalf("progress events = %d, want 2", len(progress))
	}
	if len(result.Metrics) != 59 {
		t.Fatalf("multi summary metrics = %d, want 59", len(result.Metrics))
	}
	if result.Files[0].Label != "Alpha" || result.Files[1].Label != "Beta" {
		t.Fatalf("multi summary labels = %q, %q; want Alpha, Beta", result.Files[0].Label, result.Files[1].Label)
	}
	if got := result.Files[1].MetricValues["zone_count"].DisplayValue; got != "2" {
		t.Fatalf("second zone_count = %q, want 2", got)
	}
	if result.Metrics[0].CSVName != "energyplus_version [-]" {
		t.Fatalf("first CSV metric name = %q, want energyplus_version [-]", result.Metrics[0].CSVName)
	}
}
