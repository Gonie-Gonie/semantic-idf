package main

import (
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

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

const appMetricsIDF = `
Version,
  24.1;                    !- Version Identifier

Zone,
  Office;                  !- Name
`

func TestAnalyzeInputTextIncludesMetrics(t *testing.T) {
	app := NewApp()
	result, err := app.AnalyzeInputText(appMetricsIDF)
	if err != nil {
		t.Fatalf("AnalyzeInputText() error = %v", err)
	}
	if result.Report == nil {
		t.Fatalf("AnalyzeInputText() report = nil")
	}
	if result.Report.Metrics.MetricCount != 59 {
		t.Fatalf("metrics metric count = %d, want 59", result.Report.Metrics.MetricCount)
	}
	if len(result.Report.Metrics.Categories) != 6 {
		t.Fatalf("metrics category count = %d, want 6", len(result.Report.Metrics.Categories))
	}
	if result.Report.Geometry.ZoneCount != 1 {
		t.Fatalf("geometry zone count = %d, want 1", result.Report.Geometry.ZoneCount)
	}
}

func TestAnalyzeInputTextUsesCacheForSameInput(t *testing.T) {
	app := NewApp()
	first, err := app.AnalyzeInputText(appMetricsIDF)
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
	_, hasMetricsTiming := first.Timing.Stages["metrics"]
	_, hasCoreTiming := first.Timing.Stages["core"]
	if !hasMetricsTiming || !hasCoreTiming {
		t.Fatalf("first analysis did not report stage timings: %+v", first.Timing)
	}

	second, err := app.AnalyzeInputText(appMetricsIDF)
	if err != nil {
		t.Fatalf("second AnalyzeInputText() error = %v", err)
	}
	if second.AnalysisKey != first.AnalysisKey {
		t.Fatalf("analysis key changed: %q != %q", second.AnalysisKey, first.AnalysisKey)
	}
	if second.Timing == nil || !second.Timing.CacheHit {
		t.Fatalf("second analysis timing = %+v, want cache hit", second.Timing)
	}
	if second.Report == nil || second.Report.Metrics.MetricCount != first.Report.Metrics.MetricCount {
		t.Fatalf("cached report metrics = %+v, want metric count %d", second.Report, first.Report.Metrics.MetricCount)
	}
}

func TestGetCachedAnalysisAssemblesCompletedStageResults(t *testing.T) {
	app := NewApp()
	quick, err := app.AnalyzeInputQuickText(appMetricsIDF)
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
		if _, err := app.AnalyzeInputStageText(appMetricsIDF, stage); err != nil {
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
	if cached.Report.Metrics.MetricCount != quick.Report.Metrics.MetricCount {
		t.Fatalf("cached metrics metric count = %d, want %d", cached.Report.Metrics.MetricCount, quick.Report.Metrics.MetricCount)
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

func TestParseCachedBatchInputReusesContentHashCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.idf")
	if err := os.WriteFile(path, []byte(appMetricsIDF), 0o644); err != nil {
		t.Fatal(err)
	}
	firstModel, firstDoc, err := parseCachedBatchInput(path)
	if err != nil {
		t.Fatalf("first parseCachedBatchInput() error = %v", err)
	}
	secondModel, secondDoc, err := parseCachedBatchInput(path)
	if err != nil {
		t.Fatalf("second parseCachedBatchInput() error = %v", err)
	}
	if firstModel != secondModel {
		t.Fatalf("parseCachedBatchInput() did not reuse cached model")
	}
	if len(firstDoc.Objects) != len(secondDoc.Objects) {
		t.Fatalf("cached doc object count = %d, want %d", len(secondDoc.Objects), len(firstDoc.Objects))
	}
}

func TestThrottleBatchMetricsProgressKeepsFinalEvent(t *testing.T) {
	var emitted []BatchMetricsProgress
	throttled := throttleBatchMetricsProgress(func(progress BatchMetricsProgress) {
		emitted = append(emitted, progress)
	})
	throttled(BatchMetricsProgress{RunID: "batch", Total: 3, Completed: 1})
	throttled(BatchMetricsProgress{RunID: "batch", Total: 3, Completed: 2})
	throttled(BatchMetricsProgress{RunID: "batch", Total: 3, Completed: 3})
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

func TestBatchMetricsWorkbookSheetsIncludeDelta(t *testing.T) {
	result := BatchMetricsResult{
		Metrics: []BatchMetricsMetric{{ID: "total_wwr", CSVName: "total_wwr [%]", Category: "Envelope", Unit: "%"}},
		Files: []BatchMetricsFile{
			{Index: 0, Filename: "a.idf", Label: "a.idf", Status: "ok", MetricValues: map[string]BatchMetricsValue{"total_wwr": {DisplayValue: "10", Status: "ok"}}},
			{Index: 1, Filename: "b.idf", Label: "b.idf", Status: "ok", MetricValues: map[string]BatchMetricsValue{"total_wwr": {DisplayValue: "15", Status: "ok"}}},
		},
	}
	sheets := batchMetricsWorkbookSheets(BatchMetricsXLSXExportRequest{
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
	sourceObjectIndex := 12
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
						ID:               "kpi.cooling_cop",
						Level:            "derived_kpi",
						Label:            "Cooling COP",
						Value:            2,
						ServiceKind:      "cooling",
						PathType:         "zone",
						Basis:            "derived_kpi",
						Formula:          "delivered_load / electric_end_use_energy",
						NumeratorLabel:   "Cooling load",
						NumeratorValue:   8,
						NumeratorUnit:    "kWh",
						DenominatorLabel: "Cooling electricity",
						DenominatorValue: 4,
						DenominatorUnit:  "kWh",
						SourceIDs:        []string{"sql-rdd-1"},
					}},
				},
				EnergyExplanation: simulation.EnergyExplanationResult{
					Schema: "semantic-idf.energy-explanation/v1",
					Warnings: []simulation.EnergyWarning{{
						Severity: "warning",
						Code:     "heat_balance_deviation_large",
						Message:  "Annual heat-balance deviation is large.",
						Period:   "annual",
					}},
					Sources: []simulation.EnergyDataSource{{
						ID:             "sql-rdd-1",
						SourceType:     "sql_report_data",
						IsMeter:        true,
						Name:           "Cooling:Electricity",
						TableName:      "ReportData",
						RowName:        "Cooling:Electricity",
						ColumnName:     "Value [J]",
						SourceUnit:     "J",
						NormalizedUnit: "kWh",
						ObjectIndex:    &sourceObjectIndex,
					}},
					Periods: []simulation.EnergyPeriod{{
						ID:   "annual",
						Kind: "annual",
						Warnings: []simulation.EnergyWarning{{
							Severity: "warning",
							Code:     "heat_balance_deviation_large",
							Message:  "Annual heat-balance deviation is large.",
							Period:   "annual",
						}},
						Edges: []simulation.EnergyExplanationEdge{{
							ID:        "edge-1",
							FromID:    "energy.carrier.electricity",
							ToID:      "energy.end_use.cooling.electricity",
							Value:     1,
							Unit:      "kWh",
							Relation:  "meter_enduse",
							Basis:     "measured_meter",
							SourceIDs: []string{"sql-rdd-1"},
						}},
						Reconciliation: []simulation.EnergyReconciliation{{
							ID:             "reconcile.energy.electricity.annual",
							Level:          "energy",
							Period:         "annual",
							Label:          "Electricity total basis",
							Status:         "residual",
							ExpectedValue:  2,
							ExplainedValue: 1,
							ResidualValue:  1,
							Unit:           "kWh",
							Basis:          "meter_reconciliation",
							SourceIDs:      []string{"sql-rdd-1"},
						}},
					}},
				},
			},
		}},
	}
	sheets := batchSimulationWorkbookSheets(BatchSimulationXLSXExportRequest{Result: result})
	if len(sheets) != 6 || sheets[0].Name != "Purpose Metrics" || sheets[1].Name != "Energy Summary" || sheets[4].Name != "Reconciliation" || sheets[5].Name != "Energy Warnings" {
		t.Fatalf("simulation workbook sheets = %#v", sheets)
	}
	if rows := sheets[0].Sections[0].Rows; len(rows) != 1 || rows[0][3] != "energy_explanation.kpi.cooling_cop" {
		t.Fatalf("purpose metric rows = %#v", rows)
	}
	if rows := sheets[1].Sections[0].Rows; len(rows) != 1 || rows[0][3] != "derived_kpi" || rows[0][10] != "zone" || rows[0][15] != "derived_kpi" || rows[0][18] != "8" || rows[0][21] != "4" || rows[0][23] != "sql-rdd-1" || rows[0][24] != "12" || rows[0][25] != "ReportData" || rows[0][26] != "Cooling:Electricity" || rows[0][27] != "Value [J]" || rows[0][28] != "J" || rows[0][29] != "kWh" {
		t.Fatalf("energy summary rows = %#v", rows)
	}
	if rows := sheets[3].Sections[0].Rows; len(rows) != 1 || rows[0][15] != "sql-rdd-1" || rows[0][16] != "12" || rows[0][18] != "ReportData" || rows[0][19] != "Cooling:Electricity" || rows[0][20] != "Value [J]" || rows[0][21] != "J" || rows[0][22] != "kWh" {
		t.Fatalf("energy edge rows = %#v", rows)
	}
	if rows := sheets[4].Sections[0].Rows; len(rows) != 1 || rows[0][7] != "residual" || rows[0][16] != "sql-rdd-1" || rows[0][17] != "12" || rows[0][18] != "ReportData" || rows[0][19] != "Cooling:Electricity" || rows[0][20] != "Value [J]" || rows[0][21] != "J" || rows[0][22] != "kWh" {
		t.Fatalf("reconciliation rows = %#v", rows)
	}
	if rows := sheets[5].Sections[0].Rows; len(rows) != 1 || rows[0][3] != "annual" || rows[0][5] != "heat_balance_deviation_large" {
		t.Fatalf("warning rows = %#v", rows)
	}
}

func TestBatchSimulationWorkbookSheetsIncludeRunContext(t *testing.T) {
	result := simulation.MultiSimulationResult{
		RunID:     "sim-run-context",
		Total:     2,
		Completed: 2,
		Succeeded: 1,
		Failed:    1,
	}
	sheets := batchSimulationWorkbookSheets(BatchSimulationXLSXExportRequest{
		Result: result,
		Context: BatchSimulationXLSXExportContext{
			SelectedPaths:  []string{"a.idf", "b.idf"},
			RootDirectory:  "C:/models",
			SelectedRowIDs: []string{"baseline-run", "target-run"},
			Metric:         "energy_explanation.kpi.cooling_cop",
			Sort:           "filename",
			ViewMode:       "purpose",
			WeatherMode:    "same",
			WeatherPath:    "weather.epw",
			WorkerCount:    3,
			PurposeRequest: simulation.SimulationPurposeRequest{
				Purposes:          []simulation.SimulationPurposeID{simulation.SimulationPurposeBasicEnergy},
				FrequencyPolicy:   simulation.PurposeFrequencyPolicyHighestResolution,
				SQLMode:           simulation.PurposeSQLModeSQLFirst,
				AllocationPolicy:  simulation.PurposeAllocationPolicyByZoneLoadShare,
				BasicEnergyDetail: simulation.PurposeBasicEnergyDetailHeatDrivers,
				OutputApplyMode:   simulation.PurposeOutputApplyModeAddMissingOnly,
				Scope: simulation.SimulationPurposeScope{
					ZoneMode:         "selected",
					ZoneNames:        []string{"Core"},
					PeriodMode:       "custom",
					PeriodStart:      "01-01",
					PeriodEnd:        "01-31",
					LoopMode:         "selected",
					AirLoopNames:     []string{"AHU-1"},
					OutputSignatures: []string{"Output:Variable|*|Zone Air Temperature|Hourly"},
				},
			},
			Comparison: BatchSimulationComparisonXLSXContext{
				BaselineRowID: "baseline-run",
				TargetRowID:   "target-run",
			},
		},
	})
	if len(sheets) != 2 || sheets[0].Name != "Purpose Metrics" || sheets[1].Name != "Run Context" {
		t.Fatalf("simulation context workbook sheets = %#v", sheets)
	}
	rows := map[string]string{}
	for _, row := range sheets[1].Sections[0].Rows {
		if len(row) == 2 {
			rows[row[0]] = row[1]
		}
	}
	for key, want := range map[string]string{
		"run_id":                     "sim-run-context",
		"selected_paths":             "a.idf; b.idf",
		"selected_row_ids":           "baseline-run; target-run",
		"worker_count":               "3",
		"comparison_baseline_row_id": "baseline-run",
		"comparison_target_row_id":   "target-run",
		"purpose_ids":                "basic_energy",
		"frequency_policy":           simulation.PurposeFrequencyPolicyHighestResolution,
		"allocation_policy":          simulation.PurposeAllocationPolicyByZoneLoadShare,
		"basic_energy_detail":        simulation.PurposeBasicEnergyDetailHeatDrivers,
		"sql_mode":                   simulation.PurposeSQLModeSQLFirst,
		"output_apply_mode":          simulation.PurposeOutputApplyModeAddMissingOnly,
		"scope_zone_names":           "Core",
		"scope_period_start":         "01-01",
		"scope_air_loop_names":       "AHU-1",
		"scope_output_signatures":    "Output:Variable|*|Zone Air Temperature|Hourly",
	} {
		if rows[key] != want {
			t.Fatalf("run context row %q = %q, want %q (rows %#v)", key, rows[key], want, rows)
		}
	}
}

func TestBatchSimulationWorkbookSheetsIncludeSourceAvailability(t *testing.T) {
	sourceObjectIndex := 4
	result := simulation.MultiSimulationResult{
		Results: []simulation.SimulationRunResult{{
			RunID:    "run-source-availability",
			Filename: "availability.idf",
			Status:   "succeeded",
			PurposeResults: &simulation.PurposeResultBundle{
				EnergyExplanation: simulation.EnergyExplanationResult{
					Sources: []simulation.EnergyDataSource{
						{
							ID:             "sql-rdd-20",
							SourceType:     "sql_report_data",
							IsMeter:        true,
							Name:           "Electricity:Facility",
							SourceUnit:     "J",
							NormalizedUnit: "kWh",
							TableName:      "ReportData",
							RowName:        "Electricity:Facility",
							ColumnName:     "Value [J]",
							ObjectIndex:    &sourceObjectIndex,
						},
					},
					Completeness: simulation.EnergyCompleteness{
						SourceAvailability: []simulation.EnergySourceAvailabilityEntry{
							{Level: "energy", Name: "Electricity:Facility", Status: "found", SourceIDs: []string{"sql-rdd-20"}},
							{Level: "load", Name: "Zone Air System Sensible Cooling Energy", Status: "missing"},
						},
					},
				},
			},
		}},
	}
	sheets := batchSimulationWorkbookSheets(BatchSimulationXLSXExportRequest{Result: result})
	if len(sheets) != 3 || sheets[1].Name != "Energy Sources" || sheets[2].Name != "Source Availability" {
		t.Fatalf("source availability sheets = %#v", sheets)
	}
	if rows := sheets[2].Sections[0].Rows; len(rows) != 2 || rows[0][5] != "found" || rows[0][6] != "sql-rdd-20" || rows[0][7] != "4" || rows[0][8] != "ReportData" || rows[0][9] != "Electricity:Facility" || rows[0][10] != "Value [J]" || rows[0][11] != "J" || rows[0][12] != "kWh" || rows[1][4] != "Zone Air System Sensible Cooling Energy" || rows[1][5] != "missing" {
		t.Fatalf("source availability rows = %#v", rows)
	}
}

func TestBatchSimulationWorkbookSheetsIncludeEnergyNodes(t *testing.T) {
	sourceObjectIndex := 7
	result := simulation.MultiSimulationResult{
		Results: []simulation.SimulationRunResult{{
			RunID:    "run-energy-nodes",
			Filename: "nodes.idf",
			Status:   "succeeded",
			PurposeResults: &simulation.PurposeResultBundle{
				EnergyExplanation: simulation.EnergyExplanationResult{
					Sources: []simulation.EnergyDataSource{{
						ID:             "sql-rdd-7",
						SourceType:     "sql_report_data",
						Name:           "Zone Air System Sensible Cooling Energy",
						SourceUnit:     "J",
						NormalizedUnit: "kWh",
						TableName:      "ReportData",
						RowName:        "Office",
						ColumnName:     "Zone Air System Sensible Cooling Energy",
						ObjectIndex:    &sourceObjectIndex,
					}},
					Periods: []simulation.EnergyPeriod{{
						ID:   "annual",
						Kind: "annual",
						Nodes: []simulation.EnergyExplanationNode{{
							ID:             "load.cooling.office",
							Level:          "load",
							Kind:           "load.zone_cooling",
							Label:          "Office cooling",
							Value:          8,
							Unit:           "kWh",
							Period:         "annual",
							ZoneName:       "Office",
							ServiceKind:    "cooling",
							PathType:       "zone",
							Basis:          "measured_variable",
							SourceIDs:      []string{"sql-rdd-7"},
							RelatedPathIDs: []string{"path.office.cooling"},
						}},
					}},
				},
			},
		}},
	}
	sheets := batchSimulationWorkbookSheets(BatchSimulationXLSXExportRequest{Result: result})
	if len(sheets) != 3 || sheets[1].Name != "Energy Nodes" || sheets[2].Name != "Energy Sources" {
		t.Fatalf("energy node sheets = %#v", sheets)
	}
	if rows := sheets[1].Sections[0].Rows; len(rows) != 1 || rows[0][3] != "annual" || rows[0][4] != "load.cooling.office" || rows[0][10] != "Office" || rows[0][11] != "cooling" || rows[0][12] != "zone" || rows[0][18] != "measured_variable" || rows[0][19] != "sql-rdd-7" || rows[0][20] != "7" || rows[0][21] != "path.office.cooling" || rows[0][22] != "ReportData" || rows[0][23] != "Office" || rows[0][24] != "Zone Air System Sensible Cooling Energy" || rows[0][25] != "J" || rows[0][26] != "kWh" {
		t.Fatalf("energy node rows = %#v", rows)
	}
}

func TestBatchSimulationWorkbookSheetsIncludeHeatDriverSummarySign(t *testing.T) {
	result := simulation.MultiSimulationResult{
		Results: []simulation.SimulationRunResult{{
			RunID:    "run-heat-sign",
			Filename: "heat-sign.idf",
			Status:   "succeeded",
			PurposeResults: &simulation.PurposeResultBundle{
				EnergyExplanationSummary: simulation.EnergyExplanationSummary{
					Schema: "semantic-idf.energy-explanation-summary/v1",
					HeatDrivers: []simulation.EnergyExplanationSummaryItem{{
						ID:           "heat.infiltration.negative",
						Level:        "heat",
						Label:        "Infiltration heat loss",
						Value:        1.25,
						Unit:         "kWh",
						HeatCategory: "air_exchange",
						Sign:         "negative",
						Basis:        "measured_energy_variable",
					}},
				},
			},
		}},
	}
	sheets := batchSimulationWorkbookSheets(BatchSimulationXLSXExportRequest{Result: result})
	if len(sheets) != 2 || sheets[1].Name != "Energy Summary" {
		t.Fatalf("energy summary sheets = %#v", sheets)
	}
	if rows := sheets[1].Sections[0].Rows; len(rows) != 1 || rows[0][13] != "air_exchange" || rows[0][14] != "negative" || rows[0][15] != "measured_energy_variable" {
		t.Fatalf("energy summary heat sign rows = %#v", rows)
	}
}

func TestBatchSimulationWorkbookSheetsIncludeCompletenessDelta(t *testing.T) {
	result := simulation.MultiSimulationResult{
		Results: []simulation.SimulationRunResult{
			{
				RunID:    "baseline-run",
				Filename: "baseline.idf",
				Status:   "succeeded",
				PurposeResults: &simulation.PurposeResultBundle{
					EnergyExplanationSummary: simulation.EnergyExplanationSummary{
						Schema: "semantic-idf.energy-explanation-summary/v1",
						Completeness: simulation.EnergyCompleteness{
							Status:        "complete",
							MappedPercent: 90,
							SourceAvailability: []simulation.EnergySourceAvailabilityEntry{
								{Level: "load", Name: "Zone Air System Sensible Cooling Energy", Status: "found"},
							},
						},
					},
				},
			},
			{
				RunID:    "target-run",
				Filename: "target.idf",
				Status:   "succeeded",
				PurposeResults: &simulation.PurposeResultBundle{
					EnergyExplanationSummary: simulation.EnergyExplanationSummary{
						Schema: "semantic-idf.energy-explanation-summary/v1",
						Completeness: simulation.EnergyCompleteness{
							Status:            "partial",
							MappedPercent:     65,
							MissingCategories: []string{"load: Zone Air System Sensible Cooling Energy"},
							SourceAvailability: []simulation.EnergySourceAvailabilityEntry{
								{Level: "load", Name: "Zone Air System Sensible Cooling Energy", Status: "missing"},
								{Level: "heat", Name: "Zone Fan Heat Gain Energy", Status: "not_applicable"},
							},
						},
					},
				},
			},
		},
	}
	sheets := batchSimulationWorkbookSheets(BatchSimulationXLSXExportRequest{
		Result: result,
		Comparison: BatchSimulationComparisonXLSXContext{
			BaselineRowID: "baseline-run",
			TargetRowID:   "target-run",
		},
	})
	if len(sheets) != 3 || sheets[2].Name != "Completeness Delta" {
		t.Fatalf("completeness delta sheets = %#v", sheets)
	}
	rows := sheets[2].Sections[0].Rows
	if len(rows) != 5 || rows[0][0] != "status" || rows[3][0] != "missing_source_outputs" || !strings.Contains(rows[3][4], "Zone Air System Sensible Cooling Energy") || rows[4][0] != "not_applicable_source_outputs" {
		t.Fatalf("completeness delta rows = %#v", rows)
	}
}

func TestBatchSimulationWorkbookSheetsIncludeComparisonDelta(t *testing.T) {
	baselineSourceObjectIndex := 21
	targetSourceObjectIndex := 22
	result := simulation.MultiSimulationResult{
		Results: []simulation.SimulationRunResult{
			{
				RunID:    "baseline-run",
				Filename: "baseline.idf",
				Status:   "succeeded",
				PurposeResults: &simulation.PurposeResultBundle{
					EnergyExplanationSummary: simulation.EnergyExplanationSummary{
						Schema: "semantic-idf.energy-explanation-summary/v1",
						EnergyByEndUse: []simulation.EnergyExplanationSummaryItem{{
							ID:        "cooling.electricity",
							Label:     "Cooling electricity",
							Value:     10,
							Unit:      "kWh",
							Level:     "energy",
							Carrier:   "electricity",
							EndUse:    "cooling",
							Basis:     "measured_meter",
							SourceIDs: []string{"baseline-source"},
						}},
						DerivedKPIs: []simulation.EnergyExplanationSummaryItem{{
							ID:               "kpi.cooling_cop",
							Label:            "Cooling COP",
							Value:            2,
							Level:            "derived_kpi",
							ServiceKind:      "cooling",
							PathType:         "zone",
							Basis:            "derived_kpi",
							Formula:          "delivered_load / electric_end_use_energy",
							NumeratorLabel:   "Cooling load",
							NumeratorValue:   8,
							NumeratorUnit:    "kWh",
							DenominatorLabel: "Cooling electricity",
							DenominatorValue: 4,
							DenominatorUnit:  "kWh",
							SourceIDs:        []string{"baseline-source"},
						}},
					},
					EnergyExplanation: simulation.EnergyExplanationResult{
						Schema: "semantic-idf.energy-explanation/v1",
						Sources: []simulation.EnergyDataSource{{
							ID:             "baseline-source",
							SourceType:     "sql_report_data",
							Name:           "Cooling:Electricity",
							ObjectIndex:    &baselineSourceObjectIndex,
							TableName:      "ReportData",
							RowName:        "Baseline Cooling",
							ColumnName:     "Value [J]",
							SourceUnit:     "J",
							NormalizedUnit: "kWh",
						}},
						Periods: []simulation.EnergyPeriod{{
							ID:   "annual",
							Kind: "annual",
							Nodes: []simulation.EnergyExplanationNode{
								{ID: "energy.carrier.electricity", Label: "Electricity"},
								{ID: "energy.end_use.cooling.electricity", Label: "Cooling electricity"},
							},
							Edges: []simulation.EnergyExplanationEdge{{
								ID:             "edge.cooling",
								FromID:         "energy.carrier.electricity",
								ToID:           "energy.end_use.cooling.electricity",
								Value:          10,
								Unit:           "kWh",
								Relation:       "meter_enduse",
								Basis:          "measured_meter",
								RuleID:         "meter.cooling_electricity",
								SourceIDs:      []string{"baseline-source"},
								RelatedPathIDs: []string{"baseline-path"},
							}},
						}},
					},
				},
			},
			{
				RunID:    "target-run",
				Filename: "target.idf",
				Status:   "succeeded",
				PurposeResults: &simulation.PurposeResultBundle{
					EnergyExplanationSummary: simulation.EnergyExplanationSummary{
						Schema: "semantic-idf.energy-explanation-summary/v1",
						EnergyByEndUse: []simulation.EnergyExplanationSummaryItem{{
							ID:        "cooling.electricity",
							Label:     "Cooling electricity",
							Value:     15,
							Unit:      "kWh",
							Level:     "energy",
							Carrier:   "electricity",
							EndUse:    "cooling",
							Basis:     "measured_meter",
							SourceIDs: []string{"target-source"},
						}},
						DerivedKPIs: []simulation.EnergyExplanationSummaryItem{{
							ID:               "kpi.cooling_cop",
							Label:            "Cooling COP",
							Value:            3,
							Level:            "derived_kpi",
							ServiceKind:      "cooling",
							PathType:         "zone",
							Basis:            "derived_kpi",
							Formula:          "delivered_load / electric_end_use_energy",
							NumeratorLabel:   "Cooling load",
							NumeratorValue:   9,
							NumeratorUnit:    "kWh",
							DenominatorLabel: "Cooling electricity",
							DenominatorValue: 3,
							DenominatorUnit:  "kWh",
							SourceIDs:        []string{"target-source"},
						}},
					},
					EnergyExplanation: simulation.EnergyExplanationResult{
						Schema: "semantic-idf.energy-explanation/v1",
						Sources: []simulation.EnergyDataSource{{
							ID:             "target-source",
							SourceType:     "sql_report_data",
							Name:           "Cooling:Electricity",
							ObjectIndex:    &targetSourceObjectIndex,
							TableName:      "ReportData",
							RowName:        "Target Cooling",
							ColumnName:     "Value [J]",
							SourceUnit:     "J",
							NormalizedUnit: "kWh",
						}},
						Periods: []simulation.EnergyPeriod{{
							ID:   "annual",
							Kind: "annual",
							Nodes: []simulation.EnergyExplanationNode{
								{ID: "energy.carrier.electricity", Label: "Electricity"},
								{ID: "energy.end_use.cooling.electricity", Label: "Cooling electricity"},
							},
							Edges: []simulation.EnergyExplanationEdge{{
								ID:             "edge.cooling",
								FromID:         "energy.carrier.electricity",
								ToID:           "energy.end_use.cooling.electricity",
								Value:          15,
								Unit:           "kWh",
								Relation:       "meter_enduse",
								Basis:          "measured_meter",
								RuleID:         "meter.cooling_electricity",
								SourceIDs:      []string{"target-source"},
								RelatedPathIDs: []string{"target-path"},
							}},
						}},
					},
				},
			},
		},
	}
	sheets := batchSimulationWorkbookSheets(BatchSimulationXLSXExportRequest{
		Result: result,
		Comparison: BatchSimulationComparisonXLSXContext{
			BaselineRowID: "baseline-run",
			TargetRowID:   "target-run",
		},
	})
	if len(sheets) != 8 || sheets[1].Name != "Comparison" || sheets[2].Name != "Energy Delta" || sheets[3].Name != "Sankey Edge Delta" || sheets[5].Name != "Energy Nodes" {
		t.Fatalf("simulation comparison workbook sheets = %#v", sheets)
	}
	if rows := sheets[1].Sections[0].Rows; len(rows) != 2 || rows[0][1] != "baseline-run" || rows[1][1] != "target-run" {
		t.Fatalf("comparison rows = %#v", rows)
	}
	if rows := sheets[2].Sections[0].Rows; len(rows) != 2 || rows[0][1] != "cooling.electricity" || rows[0][7] != "5" || rows[0][8] != "50%" || rows[0][26] != "baseline-source" || rows[0][27] != "target-source" || rows[0][28] != "21" || rows[0][29] != "22" || rows[1][1] != "kpi.cooling_cop" || rows[1][17] == "" || rows[1][19] != "8" || rows[1][20] != "9" || rows[1][23] != "4" || rows[1][24] != "3" {
		t.Fatalf("energy delta rows = %#v", rows)
	} else if len(rows[0]) != 40 {
		t.Fatalf("energy delta row width = %d, want 40: %#v", len(rows[0]), rows[0])
	} else {
		want := []string{"ReportData", "ReportData", "Baseline Cooling", "Target Cooling", "Value [J]", "Value [J]", "J", "J", "kWh", "kWh"}
		for i, expected := range want {
			if rows[0][30+i] != expected {
				t.Fatalf("energy delta source metadata column %d = %q, want %q in %#v", 30+i, rows[0][30+i], expected, rows[0])
			}
		}
	}
	if rows := sheets[3].Sections[0].Rows; len(rows) != 1 || rows[0][0] != "meter_enduse" || rows[0][7] != "5" || rows[0][10] != "matched" || rows[0][14] != "baseline-source" || rows[0][15] != "target-source" || rows[0][16] != "21" || rows[0][17] != "22" || rows[0][18] != "baseline-path" || rows[0][19] != "target-path" {
		t.Fatalf("edge delta rows = %#v", rows)
	} else if len(rows[0]) != 30 {
		t.Fatalf("edge delta row width = %d, want 30: %#v", len(rows[0]), rows[0])
	} else {
		want := []string{"ReportData", "ReportData", "Baseline Cooling", "Target Cooling", "Value [J]", "Value [J]", "J", "J", "kWh", "kWh"}
		for i, expected := range want {
			if rows[0][20+i] != expected {
				t.Fatalf("edge delta source metadata column %d = %q, want %q in %#v", 20+i, rows[0][20+i], expected, rows[0])
			}
		}
	}
}

func TestBatchSimulationEnergyDeltasDistinguishMissingAndZero(t *testing.T) {
	left := simulation.SimulationRunResult{
		RunID:    "baseline-run",
		Filename: "baseline.idf",
		Status:   "succeeded",
		PurposeResults: &simulation.PurposeResultBundle{
			EnergyExplanationSummary: simulation.EnergyExplanationSummary{
				Schema: "semantic-idf.energy-explanation-summary/v1",
				EnergyByEndUse: []simulation.EnergyExplanationSummaryItem{{
					ID:    "lighting.electricity",
					Label: "Lighting electricity",
					Value: 0,
					Unit:  "kWh",
					Level: "energy",
					Basis: "measured_meter",
				}},
			},
			EnergyExplanation: simulation.EnergyExplanationResult{
				Schema: "semantic-idf.energy-explanation/v1",
				Periods: []simulation.EnergyPeriod{{
					ID:   "annual",
					Kind: "annual",
					Nodes: []simulation.EnergyExplanationNode{
						{ID: "energy.carrier.electricity", Label: "Electricity"},
						{ID: "energy.end_use.lighting.electricity", Label: "Lighting electricity"},
					},
					Edges: []simulation.EnergyExplanationEdge{{
						ID:       "edge.lighting",
						FromID:   "energy.carrier.electricity",
						ToID:     "energy.end_use.lighting.electricity",
						Value:    0,
						Unit:     "kWh",
						Relation: "meter_enduse",
						Basis:    "measured_meter",
					}},
				}},
			},
		},
	}
	right := simulation.SimulationRunResult{
		RunID:    "target-run",
		Filename: "target.idf",
		Status:   "succeeded",
		PurposeResults: &simulation.PurposeResultBundle{
			EnergyExplanationSummary: simulation.EnergyExplanationSummary{
				Schema: "semantic-idf.energy-explanation-summary/v1",
				EnergyByEndUse: []simulation.EnergyExplanationSummaryItem{
					{
						ID:    "lighting.electricity",
						Label: "Lighting electricity",
						Value: 5,
						Unit:  "kWh",
						Level: "energy",
						Basis: "measured_meter",
					},
					{
						ID:    "pumps.electricity",
						Label: "Pumps electricity",
						Value: 2,
						Unit:  "kWh",
						Level: "energy",
						Basis: "measured_meter",
					},
				},
			},
			EnergyExplanation: simulation.EnergyExplanationResult{
				Schema: "semantic-idf.energy-explanation/v1",
				Periods: []simulation.EnergyPeriod{{
					ID:   "annual",
					Kind: "annual",
					Nodes: []simulation.EnergyExplanationNode{
						{ID: "energy.carrier.electricity", Label: "Electricity"},
						{ID: "energy.end_use.lighting.electricity", Label: "Lighting electricity"},
						{ID: "energy.end_use.pumps.electricity", Label: "Pumps electricity"},
					},
					Edges: []simulation.EnergyExplanationEdge{
						{
							ID:       "edge.lighting",
							FromID:   "energy.carrier.electricity",
							ToID:     "energy.end_use.lighting.electricity",
							Value:    5,
							Unit:     "kWh",
							Relation: "meter_enduse",
							Basis:    "measured_meter",
						},
						{
							ID:       "edge.pumps",
							FromID:   "energy.carrier.electricity",
							ToID:     "energy.end_use.pumps.electricity",
							Value:    2,
							Unit:     "kWh",
							Relation: "meter_enduse",
							Basis:    "measured_meter",
						},
					},
				}},
			},
		},
	}

	energyRows := batchSimulationEnergyDeltaSection(left, right).Rows
	rowByID := func(rows [][]string, id string) []string {
		for _, row := range rows {
			if len(row) > 1 && row[1] == id {
				return row
			}
		}
		return nil
	}
	if row := rowByID(energyRows, "lighting.electricity"); row == nil || row[5] != "0" || row[6] != "5" || row[8] != "—" || row[10] != "zero baseline" {
		t.Fatalf("zero baseline energy row = %#v; all rows = %#v", row, energyRows)
	}
	if row := rowByID(energyRows, "pumps.electricity"); row == nil || row[5] != "0" || row[6] != "2" || row[8] != "—" || row[10] != "missing in baseline" {
		t.Fatalf("missing baseline energy row = %#v; all rows = %#v", row, energyRows)
	}

	edgeRows := batchSimulationEnergyEdgeDeltaSection(left, right).Rows
	if row := rowByID(edgeRows, "Electricity -> Lighting electricity"); row == nil || row[5] != "0" || row[6] != "5" || row[8] != "—" || row[10] != "zero baseline" {
		t.Fatalf("zero baseline edge row = %#v; all rows = %#v", row, edgeRows)
	}
	if row := rowByID(edgeRows, "Electricity -> Pumps electricity"); row == nil || row[5] != "0" || row[6] != "2" || row[8] != "—" || row[10] != "missing in baseline" {
		t.Fatalf("missing baseline edge row = %#v; all rows = %#v", row, edgeRows)
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
	if result.Version != "24.2" {
		t.Fatalf("default sample version = %q, want 24.2", result.Version)
	}
	if !strings.Contains(string(content), "YES,                     !- Run Simulation for Weather File Run Periods") {
		t.Fatal("default sample must run the annual weather-file RunPeriod")
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
	original := appMetricsIDF + `
Lights,
  Office Lights,
  Office,
  ,
  LightingLevel,
  100;

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
	if !strings.Contains(prepared.Text, "Output:SQLite") || !strings.Contains(prepared.Text, "Electricity:Facility") {
		t.Fatalf("prepared run copy is missing purpose outputs:\n%s", prepared.Text)
	}
	if strings.Contains(prepared.Text, "Zone Lights Electricity Energy") {
		t.Fatalf("default Basic Energy run copy included explain outputs:\n%s", prepared.Text)
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
	result, err := NewApp().ApplyPurposeOutputsText(appMetricsIDF, simulation.SimulationPurposeRequest{
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

func TestAppAssetHandlerServesMetricGuides(t *testing.T) {
	for _, path := range []string{"/api/metric-guides", "/api/summary-metric-guides"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()

			appAssetHandler(NewApp()).ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("metric guide API status = %d, want %d", response.Code, http.StatusOK)
			}
			var guides []struct {
				ID string `json:"id"`
			}
			if err := json.NewDecoder(response.Body).Decode(&guides); err != nil {
				t.Fatalf("metric guide API did not return JSON: %v", err)
			}
			if len(guides) != 59 {
				t.Fatalf("metric guide API returned %d guides, want 59", len(guides))
			}
		})
	}
}

func TestAnalyzeInputTopologyTextUsesGeometryStageReport(t *testing.T) {
	app := NewApp()
	first, err := app.AnalyzeInputTopologyText(appMetricsIDF)
	if err != nil {
		t.Fatalf("AnalyzeInputTopologyText() error = %v", err)
	}
	second, err := app.AnalyzeInputTopologyText(appMetricsIDF)
	if err != nil {
		t.Fatalf("second AnalyzeInputTopologyText() error = %v", err)
	}
	if first.Schema == "" || first.SourceModelHash == "" || first.SourceModelHash != second.SourceModelHash {
		t.Fatalf("cached topology identity was not stable: %#v / %#v", first, second)
	}
}

func TestAppAssetHandlerExportsTopology(t *testing.T) {
	body := `{"text":"Version,24.1; Zone,Office;","format":"json","options":{"areaBasis":"physical"}}`
	request := httptest.NewRequest(http.MethodPost, "/api/topology", strings.NewReader(body))
	response := httptest.NewRecorder()
	appAssetHandler(NewApp()).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("topology API status = %d: %s", response.Code, response.Body.String())
	}
	var topology idf.ThermalTopologyReport
	if err := json.NewDecoder(response.Body).Decode(&topology); err != nil {
		t.Fatalf("topology API did not return report JSON: %v", err)
	}
	if topology.Schema == "" || topology.SourceModelHash == "" || topology.AreaBasis != "physical" {
		t.Fatalf("topology API metadata = %#v", topology)
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

func TestNormalizeProfileSettingsUsesCanonicalTimeViewAndScale(t *testing.T) {
	defaults := idf.DefaultProfileAnalysisSettings()
	settings := normalizeProfileSettings(idf.ProfileAnalysisSettings{TimeView: "duration", ScaleMode: "shared"}, defaults)
	if settings.TimeView != "duration" || settings.ScaleMode != "shared" {
		t.Fatalf("canonical profile settings = %#v", settings)
	}

	settings = normalizeProfileSettings(idf.ProfileAnalysisSettings{TimeView: "rules", ScaleMode: "shared"}, defaults)
	if settings.TimeView != defaults.TimeView || settings.ScaleMode != "shared" {
		t.Fatalf("removed Profile rules view settings = %#v, want time view %q and shared scale", settings, defaults.TimeView)
	}

	settings = normalizeProfileSettings(idf.ProfileAnalysisSettings{TimeView: "legacy", ScaleMode: "legacy"}, defaults)
	if settings.TimeView != defaults.TimeView || settings.ScaleMode != defaults.ScaleMode {
		t.Fatalf("invalid profile settings = %#v, want defaults %#v", settings, defaults)
	}

	payload, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{"graphDeck", "scheduleSummaryMode", "compareMode"} {
		if strings.Contains(string(payload), `"`+removed+`"`) {
			t.Fatalf("profile settings still emit removed field %q: %s", removed, payload)
		}
	}
}

func TestDefaultSettingsRetainThermalTopologyShortcuts(t *testing.T) {
	settings := normalizeAppSettings(AppSettings{})
	want := map[string]string{
		"topology3D": "1", "topologyPlan": "2", "topologyNetwork": "3", "topologyFit": "F",
		"topologyConnectivity": "T", "topologyArea": "A", "topologyUA": "U", "topologyQA": "Q",
		"primaryOpen": "Enter", "availableViews": "Alt+Enter", "clearSelection": "Escape",
	}
	for id, accelerator := range want {
		if settings.Interaction.Shortcuts[id] != accelerator {
			t.Fatalf("shortcut %s = %q, want %q", id, settings.Interaction.Shortcuts[id], accelerator)
		}
	}
	for _, removed := range []string{"topologyDisplay", "topologyNeighbors", "revealSemantic"} {
		if _, exists := settings.Interaction.Shortcuts[removed]; exists {
			t.Fatalf("removed shortcut %s is still configured", removed)
		}
	}
	payload, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{"syncRawTextPosition", "autoAnalyzeDelayMs", "topologySyncLocate", "geometrySyncLocate"} {
		if strings.Contains(string(payload), `"`+removed+`"`) {
			t.Fatalf("default settings still emit removed interaction setting %q: %s", removed, payload)
		}
	}
}

func TestLegacyRemovedInteractionSettingsAreIgnoredAndNotReemitted(t *testing.T) {
	var settings AppSettings
	if err := json.Unmarshal([]byte(`{
		"behavior": {"autoAnalyzeDelayMs": 2500},
		"interaction": {"syncRawTextPosition": false, "topologySyncLocate": false}
	}`), &settings); err != nil {
		t.Fatal(err)
	}
	settings = normalizeAppSettings(settings)
	payload, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{"syncRawTextPosition", "autoAnalyzeDelayMs", "topologySyncLocate"} {
		if strings.Contains(string(payload), `"`+removed+`"`) {
			t.Fatalf("normalized settings re-emitted removed interaction setting %q: %s", removed, payload)
		}
	}
}

func TestLegacyGeometrySyncSettingIsIgnoredWhileShortcutsMigrate(t *testing.T) {
	var settings AppSettings
	if err := json.Unmarshal([]byte(`{
		"interaction": {
			"geometrySyncLocate": true,
			"shortcuts": {
				"geometry3D": "4",
				"geometryPlan": "5",
				"geometryThermal": "6",
				"geometryFit": "Shift+F"
			}
		}
	}`), &settings); err != nil {
		t.Fatal(err)
	}
	settings = normalizeAppSettings(settings)
	for id, want := range map[string]string{
		"topology3D": "4", "topologyPlan": "5", "topologyNetwork": "6", "topologyFit": "Shift+F",
	} {
		if got := settings.Interaction.Shortcuts[id]; got != want {
			t.Fatalf("migrated shortcut %s = %q, want %q", id, got, want)
		}
	}
	payload, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"geometrySyncLocate", "topologySyncLocate", "geometry3D", "geometryPlan", "geometryThermal", "geometryFit"} {
		if strings.Contains(string(payload), `"`+legacy+`"`) {
			t.Fatalf("normalized settings still emit legacy key %q: %s", legacy, payload)
		}
	}
}

func TestGraphFontSizeSettingsDefaultAndClamp(t *testing.T) {
	if got := normalizeAppSettings(AppSettings{}).Appearance.GraphFontSize; got != 11 {
		t.Fatalf("default graph font size = %d, want 11", got)
	}
	settings := AppSettings{Appearance: AppearanceSettings{GraphFontSize: 99}}
	if got := normalizeAppSettings(settings).Appearance.GraphFontSize; got != 18 {
		t.Fatalf("maximum graph font size = %d, want 18", got)
	}
	settings.Appearance.GraphFontSize = 1
	if got := normalizeAppSettings(settings).Appearance.GraphFontSize; got != 9 {
		t.Fatalf("minimum graph font size = %d, want 9", got)
	}
}

func TestAnalyzeBatchMetricsPaths(t *testing.T) {
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

	var progress []BatchMetricsProgress
	result := analyzeBatchMetricsPaths([]string{first, second}, BatchMetricsRequest{RunID: "test-run", AreaBasis: "effective"}, func(item BatchMetricsProgress) {
		progress = append(progress, item)
	})

	if result.Total != 2 || result.Completed != 2 || result.Succeeded != 2 || result.Failed != 0 {
		t.Fatalf("batch metrics counts = total:%d completed:%d succeeded:%d failed:%d", result.Total, result.Completed, result.Succeeded, result.Failed)
	}
	if len(progress) != 2 {
		t.Fatalf("progress events = %d, want 2", len(progress))
	}
	if len(result.Metrics) != 76 {
		t.Fatalf("batch metrics count = %d, want 76", len(result.Metrics))
	}
	if result.Files[0].Label != "Alpha" || result.Files[1].Label != "Beta" {
		t.Fatalf("batch metrics labels = %q, %q; want Alpha, Beta", result.Files[0].Label, result.Files[1].Label)
	}
	if got := result.Files[1].MetricValues["zone_count"].DisplayValue; got != "2" {
		t.Fatalf("second zone_count = %q, want 2", got)
	}
	if got := result.Files[1].MetricValues["topology_zone_count"].DisplayValue; got != "2" {
		t.Fatalf("second topology zone count = %q, want 2", got)
	}
	if result.AreaBasis != "effective" || result.Files[0].TopologyData == nil || result.Files[0].Topology != nil {
		t.Fatalf("default topology batch contract = basis %q data %#v full %#v", result.AreaBasis, result.Files[0].TopologyData, result.Files[0].Topology)
	}
	if result.Metrics[0].CSVName != "energyplus_version [-]" {
		t.Fatalf("first CSV metric name = %q, want energyplus_version [-]", result.Metrics[0].CSVName)
	}
}

func TestBatchTopologyOptionsExportsDetailsAndFullReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "topology.idf")
	text := `Version,24.1;
Zone,Office;
BuildingSurface:Detailed,
  Office Floor, Floor, Floor Construction, Office, , Ground, , NoSun, NoWind, 0.5, 4,
  0,0,0, 10,0,0, 10,10,0, 0,10,0;
`
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write topology fixture: %v", err)
	}
	result := analyzeBatchMetricsPaths([]string{path}, BatchMetricsRequest{RunID: "topology", AreaBasis: "physical", IncludeFullTopology: true}, nil)
	if result.AreaBasis != "physical" || !result.IncludesFullTopology || result.Files[0].Topology == nil {
		t.Fatalf("topology batch options were not retained: %#v", result)
	}
	if result.Files[0].Topology.AreaBasis != "physical" || len(result.Files[0].TopologyData.Connections) == 0 {
		t.Fatalf("full topology/details missing: %#v", result.Files[0])
	}
	sheets := batchMetricsWorkbookSheets(BatchMetricsXLSXExportRequest{Result: *result, BaselineIndex: -1, CompareIndex: -1})
	wantSheets := []string{"Raw", "Topology Summary", "Zone Signatures", "Thermal Connections", "Boundary Issues"}
	if len(sheets) != len(wantSheets) {
		t.Fatalf("topology workbook sheets = %#v", sheets)
	}
	for index, want := range wantSheets {
		if sheets[index].Name != want {
			t.Fatalf("sheet %d = %q, want %q", index, sheets[index].Name, want)
		}
	}
	csvText := batchTopologyNormalizedCSV(*result)
	for _, want := range []string{"row_kind,file,area_basis", "thermal_connection", "source_entity_ids", "source_object_indices"} {
		if !strings.Contains(csvText, want) {
			t.Fatalf("normalized topology CSV missing %q:\n%s", want, csvText)
		}
	}
}

func TestBatchTopologyDeltaRejectsDifferentBasisOrCoverage(t *testing.T) {
	metric := BatchMetricsMetric{ID: "topology_exterior_ua", Category: "Topology", Unit: "W/K"}
	value := func(basis string, coverage float64) BatchMetricsValue {
		return BatchMetricsValue{DisplayValue: "10", Status: "partial", AreaBasis: basis, Coverage: coverage, HasCoverage: true, BasisSensitive: true}
	}
	baseline := BatchMetricsFile{MetricValues: map[string]BatchMetricsValue{metric.ID: value("effective", 0.8)}}
	compare := BatchMetricsFile{MetricValues: map[string]BatchMetricsValue{metric.ID: value("physical", 0.8)}}
	if row := batchMetricsDeltaRow(metric, baseline, compare); row[5] != "not comparable" || !strings.Contains(row[7], "basis") {
		t.Fatalf("different-basis delta row = %#v", row)
	}
	compare.MetricValues[metric.ID] = value("effective", 0.9)
	if row := batchMetricsDeltaRow(metric, baseline, compare); row[5] != "not comparable" || !strings.Contains(row[7], "coverage") {
		t.Fatalf("different-coverage delta row = %#v", row)
	}
}
