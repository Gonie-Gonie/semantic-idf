package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/internal/epinput"
	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
	"github.com/Gonie-Gonie/semantic-idf/internal/simulation"
	"github.com/Gonie-Gonie/semantic-idf/internal/tabular"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type BatchJobRequest struct {
	RunID         string   `json:"runId"`
	InputPaths    []string `json:"inputPaths"`
	RootDirectory string   `json:"rootDirectory,omitempty"`
	Recursive     bool     `json:"recursive,omitempty"`
	WorkerCount   int      `json:"workerCount,omitempty"`
}

type BatchFileResult struct {
	Index    int    `json:"index"`
	Path     string `json:"path"`
	Filename string `json:"filename"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
	Format   string `json:"format,omitempty"`
	Version  string `json:"version,omitempty"`
}

type BatchDiagnoseResult struct {
	Canceled   bool                    `json:"canceled,omitempty"`
	RunID      string                  `json:"runId,omitempty"`
	Total      int                     `json:"total"`
	Completed  int                     `json:"completed"`
	Succeeded  int                     `json:"succeeded"`
	Failed     int                     `json:"failed"`
	Files      []BatchDiagnoseFile     `json:"files"`
	IssueCodes []BatchIssueCodeSummary `json:"issueCodes"`
}

type BatchDiagnoseFile struct {
	BatchFileResult
	ErrorCount   int              `json:"errorCount"`
	WarningCount int              `json:"warningCount"`
	NoticeCount  int              `json:"noticeCount"`
	Issues       []idf.Diagnostic `json:"issues,omitempty"`
}

type BatchIssueCodeSummary struct {
	Code       string         `json:"code"`
	Severity   string         `json:"severity,omitempty"`
	Category   string         `json:"category,omitempty"`
	Source     string         `json:"source,omitempty"`
	Confidence string         `json:"confidence,omitempty"`
	Count      int            `json:"count"`
	FileCounts map[string]int `json:"fileCounts,omitempty"`
}

type BatchOutputQAResult struct {
	Canceled  bool                `json:"canceled,omitempty"`
	RunID     string              `json:"runId,omitempty"`
	Total     int                 `json:"total"`
	Completed int                 `json:"completed"`
	Succeeded int                 `json:"succeeded"`
	Failed    int                 `json:"failed"`
	Files     []BatchOutputQAFile `json:"files"`
}

type BatchOutputQAFile struct {
	BatchFileResult
	OutputObjectCount        int                 `json:"outputObjectCount"`
	OutputVariableCount      int                 `json:"outputVariableCount"`
	OutputMeterCount         int                 `json:"outputMeterCount"`
	OutputTableCount         int                 `json:"outputTableCount"`
	SQLitePresent            bool                `json:"sqlitePresent"`
	VariableDictionary       bool                `json:"variableDictionary"`
	DetailedOrTimestepCount  int                 `json:"detailedOrTimestepCount"`
	DuplicateOutputCount     int                 `json:"duplicateOutputCount"`
	HeavyWarningCount        int                 `json:"heavyWarningCount"`
	PurposeReadiness         map[string]bool     `json:"purposeReadiness,omitempty"`
	MissingPurposeOutputs    map[string][]string `json:"missingPurposeOutputs,omitempty"`
	OutputWarnings           []idf.Diagnostic    `json:"outputWarnings,omitempty"`
	PurposeOutputPlanWeight  string              `json:"purposeOutputPlanWeight,omitempty"`
	PurposeOutputPlanObjects int                 `json:"purposeOutputPlanObjects,omitempty"`
}

type BatchCleanupReportResult struct {
	Canceled  bool                     `json:"canceled,omitempty"`
	RunID     string                   `json:"runId,omitempty"`
	Total     int                      `json:"total"`
	Completed int                      `json:"completed"`
	Succeeded int                      `json:"succeeded"`
	Failed    int                      `json:"failed"`
	Files     []BatchCleanupReportFile `json:"files"`
	Rules     []idf.CleanupRule        `json:"rules,omitempty"`
}

type BatchCleanupReportFile struct {
	BatchFileResult
	RuleCounts  map[string]int         `json:"ruleCounts,omitempty"`
	Candidates  []idf.CleanupCandidate `json:"candidates,omitempty"`
	Scan        *idf.CleanupScan       `json:"scan,omitempty"`
	SafeCount   int                    `json:"safeCount"`
	ReviewCount int                    `json:"reviewCount"`
	OutputCount int                    `json:"outputCount"`
}

type BatchConvertExportRequest struct {
	BatchJobRequest
	TargetFormat    string `json:"targetFormat"`
	OutputDirectory string `json:"outputDirectory,omitempty"`
	OverwritePolicy string `json:"overwritePolicy,omitempty"`
}

type BatchConvertExportResult struct {
	Canceled  bool                     `json:"canceled,omitempty"`
	RunID     string                   `json:"runId,omitempty"`
	Total     int                      `json:"total"`
	Completed int                      `json:"completed"`
	Succeeded int                      `json:"succeeded"`
	Failed    int                      `json:"failed"`
	Files     []BatchConvertExportFile `json:"files"`
}

type BatchConvertExportFile struct {
	BatchFileResult
	OutputPath string `json:"outputPath,omitempty"`
	MIME       string `json:"mime,omitempty"`
}

type BatchSummaryXLSXExportRequest struct {
	Result        MultiSummaryResult `json:"result"`
	Orientation   string             `json:"orientation"`
	BaselineIndex int                `json:"baselineIndex"`
	CompareIndex  int                `json:"compareIndex"`
}

type BatchSimulationXLSXExportRequest struct {
	Result     simulation.MultiSimulationResult     `json:"result"`
	Context    BatchSimulationXLSXExportContext     `json:"context,omitempty"`
	Comparison BatchSimulationComparisonXLSXContext `json:"comparison,omitempty"`
}

type BatchSimulationXLSXExportContext struct {
	SelectedPaths  []string                             `json:"selectedPaths,omitempty"`
	RootDirectory  string                               `json:"rootDirectory,omitempty"`
	SelectedRowIDs []string                             `json:"selectedRowIds,omitempty"`
	Metric         string                               `json:"metric,omitempty"`
	Sort           string                               `json:"sort,omitempty"`
	ViewMode       string                               `json:"viewMode,omitempty"`
	WeatherMode    string                               `json:"weatherMode,omitempty"`
	WeatherPath    string                               `json:"weatherPath,omitempty"`
	WorkerCount    int                                  `json:"workerCount,omitempty"`
	PurposeRequest simulation.SimulationPurposeRequest  `json:"purposeRequest,omitempty"`
	Comparison     BatchSimulationComparisonXLSXContext `json:"comparison,omitempty"`
}

type BatchSimulationComparisonXLSXContext struct {
	BaselineRowID string `json:"baselineRowId,omitempty"`
	TargetRowID   string `json:"targetRowId,omitempty"`
}

type BatchSimulationPlanPreviewResult struct {
	Total             int                              `json:"total"`
	Completed         int                              `json:"completed"`
	Succeeded         int                              `json:"succeeded"`
	Failed            int                              `json:"failed"`
	Purposes          []simulation.SimulationPurposeID `json:"purposes"`
	WorkerCount       int                              `json:"workerCount"`
	WeatherMode       string                           `json:"weatherMode,omitempty"`
	WeatherPath       string                           `json:"weatherPath,omitempty"`
	CommonOutputCount int                              `json:"commonOutputCount"`
	HeavyFileCount    int                              `json:"heavyFileCount"`
	Files             []BatchSimulationPlanPreviewFile `json:"files"`
}

type BatchSimulationPlanPreviewFile struct {
	BatchFileResult
	OutputCount          int                            `json:"outputCount"`
	TemporaryOutputCount int                            `json:"temporaryOutputCount"`
	ExistingOutputCount  int                            `json:"existingOutputCount"`
	EstimatedWeight      string                         `json:"estimatedWeight,omitempty"`
	EstimatedSeries      int                            `json:"estimatedSeries"`
	EstimatedFrames      int                            `json:"estimatedFrames"`
	RequiresSQL          bool                           `json:"requiresSql"`
	Warnings             []simulation.PurposeRunWarning `json:"warnings,omitempty"`
}

func (a *App) RunBatchDiagnose(runID string) (*BatchDiagnoseResult, error) {
	paths, canceled, err := a.selectBatchInputFiles("Open EnergyPlus inputs for Batch Diagnose")
	if err != nil || canceled {
		return &BatchDiagnoseResult{Canceled: canceled, RunID: runID}, err
	}
	return AnalyzeBatchDiagnosePaths(BatchJobRequest{RunID: runID, InputPaths: paths}), nil
}

func (a *App) RunBatchOutputQA(runID string) (*BatchOutputQAResult, error) {
	paths, canceled, err := a.selectBatchInputFiles("Open EnergyPlus inputs for Batch Output QA")
	if err != nil || canceled {
		return &BatchOutputQAResult{Canceled: canceled, RunID: runID}, err
	}
	return AnalyzeBatchOutputQAPaths(BatchJobRequest{RunID: runID, InputPaths: paths}), nil
}

func (a *App) RunBatchCleanupReport(runID string) (*BatchCleanupReportResult, error) {
	paths, canceled, err := a.selectBatchInputFiles("Open EnergyPlus inputs for Cleanup Report")
	if err != nil || canceled {
		return &BatchCleanupReportResult{Canceled: canceled, RunID: runID}, err
	}
	return AnalyzeBatchCleanupReportPaths(BatchJobRequest{RunID: runID, InputPaths: paths}), nil
}

func (a *App) RunBatchConvertExport(targetFormat string, overwritePolicy string) (*BatchConvertExportResult, error) {
	paths, canceled, err := a.selectBatchInputFiles("Open EnergyPlus inputs for Batch Convert / Export")
	if err != nil || canceled {
		return &BatchConvertExportResult{Canceled: canceled}, err
	}
	outputDirectory, err := a.selectBatchOutputDirectory()
	if err != nil {
		return nil, err
	}
	if outputDirectory == "" {
		return &BatchConvertExportResult{Canceled: true}, nil
	}
	return ConvertExportBatch(BatchConvertExportRequest{
		BatchJobRequest: BatchJobRequest{InputPaths: paths},
		TargetFormat:    targetFormat,
		OutputDirectory: outputDirectory,
		OverwritePolicy: overwritePolicy,
	}), nil
}

func (a *App) SaveBatchSummaryXLSX(request BatchSummaryXLSXExportRequest) (*SaveFileResult, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("desktop runtime is not ready")
	}
	var b bytes.Buffer
	if err := tabular.WriteWorkbookXLSX(&b, batchSummaryWorkbookSheets(request)); err != nil {
		return nil, err
	}
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Save Batch Summary workbook",
		DefaultFilename: "batch-summary.xlsx",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Excel workbook (*.xlsx)", Pattern: "*.xlsx"},
		},
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return &SaveFileResult{Canceled: true}, nil
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		return nil, err
	}
	return &SaveFileResult{Path: path, Filename: filepath.Base(path)}, nil
}

func (a *App) SaveBatchSimulationXLSX(request BatchSimulationXLSXExportRequest) (*SaveFileResult, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("desktop runtime is not ready")
	}
	var b bytes.Buffer
	if err := tabular.WriteWorkbookXLSX(&b, batchSimulationWorkbookSheets(request)); err != nil {
		return nil, err
	}
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Save Batch Simulation workbook",
		DefaultFilename: "batch-simulation-purpose-results.xlsx",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Excel workbook (*.xlsx)", Pattern: "*.xlsx"},
		},
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return &SaveFileResult{Canceled: true}, nil
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		return nil, err
	}
	return &SaveFileResult{Path: path, Filename: filepath.Base(path)}, nil
}

func (a *App) PreviewBatchSimulationPlan(request simulation.MultiSimulationRequest) (*BatchSimulationPlanPreviewResult, error) {
	return AnalyzeBatchSimulationPlan(request), nil
}

func (a *App) CreateBatchSafeCleanedCopies(paths []string) (*BatchConvertExportResult, error) {
	paths = normalizeBatchPaths(paths)
	if len(paths) == 0 {
		selected, canceled, err := a.selectBatchInputFiles("Open EnergyPlus inputs for safe cleanup copies")
		if err != nil || canceled {
			return &BatchConvertExportResult{Canceled: canceled}, err
		}
		paths = selected
	}
	outputDirectory, err := a.selectBatchOutputDirectory()
	if err != nil {
		return nil, err
	}
	if outputDirectory == "" {
		return &BatchConvertExportResult{Canceled: true}, nil
	}
	return CreateBatchSafeCleanupCopies(BatchConvertExportRequest{
		BatchJobRequest: BatchJobRequest{InputPaths: paths},
		OutputDirectory: outputDirectory,
		OverwritePolicy: "rename",
	}), nil
}

func (a *App) selectBatchInputFiles(title string) ([]string, bool, error) {
	if a.ctx == nil {
		return nil, false, fmt.Errorf("desktop runtime is not ready")
	}
	paths, err := wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   title,
		Filters: inputFileFilters(),
	})
	if err != nil {
		return nil, false, err
	}
	return paths, len(paths) == 0, nil
}

func (a *App) selectBatchOutputDirectory() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("desktop runtime is not ready")
	}
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select batch output folder",
	})
}

func AnalyzeBatchDiagnosePaths(request BatchJobRequest) *BatchDiagnoseResult {
	paths := normalizeBatchPaths(request.InputPaths)
	result := &BatchDiagnoseResult{RunID: request.RunID, Total: len(paths)}
	codeCounts := map[string]*BatchIssueCodeSummary{}
	for index, path := range paths {
		file := analyzeBatchDiagnoseFile(index, path)
		result.Files = append(result.Files, file)
		result.Completed++
		if file.Status == "ok" {
			result.Succeeded++
		} else {
			result.Failed++
		}
		for _, issue := range file.Issues {
			key := strings.Join([]string{issue.Code, issue.Severity, issue.Category, issue.Source, issue.Confidence}, "\x00")
			summary := codeCounts[key]
			if summary == nil {
				summary = &BatchIssueCodeSummary{
					Code:       issue.Code,
					Severity:   issue.Severity,
					Category:   issue.Category,
					Source:     issue.Source,
					Confidence: issue.Confidence,
					FileCounts: map[string]int{},
				}
				codeCounts[key] = summary
			}
			summary.Count++
			summary.FileCounts[file.Label]++
		}
	}
	for _, summary := range codeCounts {
		result.IssueCodes = append(result.IssueCodes, *summary)
	}
	sort.Slice(result.IssueCodes, func(i, j int) bool {
		if result.IssueCodes[i].Count != result.IssueCodes[j].Count {
			return result.IssueCodes[i].Count > result.IssueCodes[j].Count
		}
		return result.IssueCodes[i].Code < result.IssueCodes[j].Code
	})
	return result
}

func analyzeBatchDiagnoseFile(index int, path string) BatchDiagnoseFile {
	base := newBatchFileResult(index, path)
	model, doc, err := parseBatchInput(path)
	if err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return BatchDiagnoseFile{BatchFileResult: base}
	}
	base.Status = "ok"
	base.Format = string(model.Format)
	base.Version = model.Version.Raw
	issues := idf.AnalyzeDiagnostics(doc)
	file := BatchDiagnoseFile{BatchFileResult: base, Issues: issues}
	for _, issue := range issues {
		switch issue.Severity {
		case idf.DiagnosticError:
			file.ErrorCount++
		case idf.DiagnosticWarning:
			file.WarningCount++
		default:
			file.NoticeCount++
		}
	}
	return file
}

func AnalyzeBatchOutputQAPaths(request BatchJobRequest) *BatchOutputQAResult {
	paths := normalizeBatchPaths(request.InputPaths)
	result := &BatchOutputQAResult{RunID: request.RunID, Total: len(paths)}
	for index, path := range paths {
		file := analyzeBatchOutputQAFile(index, path)
		result.Files = append(result.Files, file)
		result.Completed++
		if file.Status == "ok" {
			result.Succeeded++
		} else {
			result.Failed++
		}
	}
	return result
}

func AnalyzeBatchSimulationPlan(request simulation.MultiSimulationRequest) *BatchSimulationPlanPreviewResult {
	paths := normalizeBatchPaths(request.InputPaths)
	purposeRequest := simulation.NormalizeSimulationPurposeRequest(request.PurposeRequest)
	result := &BatchSimulationPlanPreviewResult{
		Total:       len(paths),
		Purposes:    purposeRequest.Purposes,
		WorkerCount: request.WorkerCount,
		WeatherMode: request.WeatherMode,
		WeatherPath: request.WeatherPath,
	}
	common := map[string]int{}
	for index, path := range paths {
		file := analyzeBatchSimulationPlanFile(index, path, purposeRequest)
		result.Files = append(result.Files, file)
		result.Completed++
		if file.Status == "ok" {
			result.Succeeded++
			if strings.EqualFold(file.EstimatedWeight, "Heavy") || strings.EqualFold(file.EstimatedWeight, "Very Heavy") || len(file.Warnings) > 0 {
				result.HeavyFileCount++
			}
			model, doc, err := parseBatchInput(path)
			if err == nil {
				_ = model
				plan := simulation.BuildPurposeRunPlan(doc, purposeRequest)
				seen := map[string]bool{}
				for _, object := range plan.OutputObjects {
					if !seen[object.Signature] {
						common[object.Signature]++
						seen[object.Signature] = true
					}
				}
			}
		} else {
			result.Failed++
		}
	}
	for _, count := range common {
		if count == result.Succeeded && result.Succeeded > 0 {
			result.CommonOutputCount++
		}
	}
	return result
}

func analyzeBatchSimulationPlanFile(index int, path string, purposeRequest simulation.SimulationPurposeRequest) BatchSimulationPlanPreviewFile {
	base := newBatchFileResult(index, path)
	model, doc, err := parseBatchInput(path)
	if err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return BatchSimulationPlanPreviewFile{BatchFileResult: base}
	}
	plan := simulation.BuildPurposeRunPlan(doc, purposeRequest)
	file := BatchSimulationPlanPreviewFile{
		BatchFileResult: base,
		OutputCount:     len(plan.OutputObjects),
		EstimatedWeight: plan.EstimatedWeight,
		EstimatedSeries: plan.EstimatedSeries,
		EstimatedFrames: plan.EstimatedFrames,
		RequiresSQL:     plan.RequiresSQL,
		Warnings:        plan.Warnings,
	}
	base.Status = "ok"
	file.BatchFileResult = base
	file.Format = string(model.Format)
	file.Version = model.Version.Raw
	for _, object := range plan.OutputObjects {
		if object.State == simulation.PurposeOutputStateExisting {
			file.ExistingOutputCount++
		} else {
			file.TemporaryOutputCount++
		}
	}
	return file
}

func analyzeBatchOutputQAFile(index int, path string) BatchOutputQAFile {
	base := newBatchFileResult(index, path)
	model, doc, err := parseBatchInput(path)
	if err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return BatchOutputQAFile{BatchFileResult: base}
	}
	base.Status = "ok"
	base.Format = string(model.Format)
	base.Version = model.Version.Raw
	output := idf.AnalyzeOutput(doc)
	file := BatchOutputQAFile{
		BatchFileResult:         base,
		OutputObjectCount:       output.ObjectCount,
		OutputVariableCount:     output.VariableCount,
		OutputMeterCount:        output.MeterCount,
		OutputWarnings:          output.Warnings,
		PurposeReadiness:        map[string]bool{},
		MissingPurposeOutputs:   map[string][]string{},
		PurposeOutputPlanWeight: "",
	}
	for _, item := range output.Existing {
		lower := strings.ToLower(item.ObjectType)
		switch {
		case lower == "output:sqlite":
			file.SQLitePresent = true
		case lower == "output:variabledictionary":
			file.VariableDictionary = true
		case strings.HasPrefix(lower, "output:table:") || lower == "outputcontrol:table:style":
			file.OutputTableCount++
		}
		if item.Duplicate {
			file.DuplicateOutputCount++
		}
		if strings.EqualFold(item.ReportingFrequency, "Detailed") || strings.EqualFold(item.ReportingFrequency, "Timestep") {
			file.DetailedOrTimestepCount++
		}
	}
	for _, warning := range output.Warnings {
		if warning.Code == "high_volume_output" || warning.Code == "duplicate_output_request" {
			file.HeavyWarningCount++
		}
	}
	if output.VariableCount > 200 {
		file.HeavyWarningCount++
	}
	purposeRequest := simulation.NormalizeSimulationPurposeRequest(&simulation.SimulationPurposeRequest{
		Purposes: []simulation.SimulationPurposeID{
			simulation.SimulationPurposeBasicEnergy,
			simulation.SimulationPurposeZoneHeatFlow,
			simulation.SimulationPurposeHVACLoopCheck,
			simulation.SimulationPurposeIntegrity,
		},
	})
	plan := simulation.BuildPurposeRunPlan(doc, purposeRequest)
	file.PurposeOutputPlanWeight = plan.EstimatedWeight
	file.PurposeOutputPlanObjects = len(plan.OutputObjects)
	for _, purposeID := range plan.Purposes {
		file.PurposeReadiness[string(purposeID)] = true
	}
	for _, object := range plan.OutputObjects {
		for _, purposeID := range object.PurposeIDs {
			if object.State != simulation.PurposeOutputStateExisting {
				key := string(purposeID)
				file.PurposeReadiness[key] = false
				file.MissingPurposeOutputs[key] = append(file.MissingPurposeOutputs[key], object.Signature)
			}
		}
	}
	return file
}

func AnalyzeBatchCleanupReportPaths(request BatchJobRequest) *BatchCleanupReportResult {
	paths := normalizeBatchPaths(request.InputPaths)
	result := &BatchCleanupReportResult{RunID: request.RunID, Total: len(paths)}
	for index, path := range paths {
		file := analyzeBatchCleanupReportFile(index, path)
		result.Files = append(result.Files, file)
		result.Completed++
		if file.Status == "ok" {
			result.Succeeded++
		} else {
			result.Failed++
		}
		if len(result.Rules) == 0 && file.Scan != nil {
			result.Rules = file.Scan.Rules
		}
	}
	return result
}

func analyzeBatchCleanupReportFile(index int, path string) BatchCleanupReportFile {
	base := newBatchFileResult(index, path)
	model, doc, err := parseBatchInput(path)
	if err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return BatchCleanupReportFile{BatchFileResult: base}
	}
	base.Status = "ok"
	base.Format = string(model.Format)
	base.Version = model.Version.Raw
	scan := idf.ScanCleanup(doc)
	file := BatchCleanupReportFile{
		BatchFileResult: base,
		RuleCounts:      map[string]int{},
		Candidates:      scan.Candidates,
		Scan:            &scan,
	}
	for _, candidate := range scan.Candidates {
		file.RuleCounts[candidate.RuleID]++
		switch candidate.Source {
		case "output":
			file.OutputCount++
		default:
			if candidate.Risk == "safe" {
				file.SafeCount++
			} else {
				file.ReviewCount++
			}
		}
	}
	return file
}

func ConvertExportBatch(request BatchConvertExportRequest) *BatchConvertExportResult {
	paths := normalizeBatchPaths(request.InputPaths)
	result := &BatchConvertExportResult{RunID: request.RunID, Total: len(paths)}
	for index, path := range paths {
		file := convertExportBatchFile(index, path, request)
		result.Files = append(result.Files, file)
		result.Completed++
		if file.Status == "ok" {
			result.Succeeded++
		} else {
			result.Failed++
		}
	}
	return result
}

func CreateBatchSafeCleanupCopies(request BatchConvertExportRequest) *BatchConvertExportResult {
	paths := normalizeBatchPaths(request.InputPaths)
	result := &BatchConvertExportResult{RunID: request.RunID, Total: len(paths)}
	for index, path := range paths {
		file := createBatchSafeCleanupCopy(index, path, request)
		result.Files = append(result.Files, file)
		result.Completed++
		if file.Status == "ok" || file.Status == "skipped" {
			result.Succeeded++
		} else {
			result.Failed++
		}
	}
	return result
}

func createBatchSafeCleanupCopy(index int, path string, request BatchConvertExportRequest) BatchConvertExportFile {
	base := newBatchFileResult(index, path)
	content, err := os.ReadFile(path)
	if err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return BatchConvertExportFile{BatchFileResult: base}
	}
	model, doc, err := parseBatchInput(path)
	if err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return BatchConvertExportFile{BatchFileResult: base}
	}
	scan := idf.ScanCleanup(doc)
	ruleIDs := safeCleanupRuleIDs(scan)
	if len(ruleIDs) == 0 {
		base.Status = "skipped"
		base.Format = string(model.Format)
		base.Version = model.Version.Raw
		return BatchConvertExportFile{BatchFileResult: base}
	}
	preview, err := previewCleanupText(string(content), ruleIDs, nil)
	if err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return BatchConvertExportFile{BatchFileResult: base}
	}
	extension := filepath.Ext(path)
	if extension == "" {
		extension = ".idf"
	}
	outputDirectory := strings.TrimSpace(request.OutputDirectory)
	if outputDirectory == "" {
		outputDirectory = filepath.Dir(path)
	}
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + "-cleaned" + extension
	outputPath := filepath.Join(outputDirectory, stem)
	outputPath = uniqueBatchOutputPath(outputPath)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return BatchConvertExportFile{BatchFileResult: base}
	}
	if err := os.WriteFile(outputPath, []byte(preview.Text), 0o644); err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return BatchConvertExportFile{BatchFileResult: base}
	}
	base.Status = "ok"
	base.Format = string(model.Format)
	base.Version = model.Version.Raw
	return BatchConvertExportFile{BatchFileResult: base, OutputPath: outputPath, MIME: "text/plain"}
}

func convertExportBatchFile(index int, path string, request BatchConvertExportRequest) BatchConvertExportFile {
	base := newBatchFileResult(index, path)
	model, doc, err := parseBatchInput(path)
	if err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return BatchConvertExportFile{BatchFileResult: base}
	}
	outputContent, extension, mime, err := batchExportContent(model, doc, request.TargetFormat)
	if err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return BatchConvertExportFile{BatchFileResult: base}
	}
	outputDirectory := strings.TrimSpace(request.OutputDirectory)
	if outputDirectory == "" {
		outputDirectory = filepath.Dir(path)
	}
	outputPath, err := resolveBatchOutputPath(outputDirectory, path, extension, request.OverwritePolicy)
	if err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return BatchConvertExportFile{BatchFileResult: base}
	}
	if outputPath == "" {
		base.Status = "skipped"
		return BatchConvertExportFile{BatchFileResult: base}
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return BatchConvertExportFile{BatchFileResult: base}
	}
	if err := os.WriteFile(outputPath, outputContent, 0o644); err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return BatchConvertExportFile{BatchFileResult: base}
	}
	base.Status = "ok"
	base.Format = string(model.Format)
	base.Version = model.Version.Raw
	return BatchConvertExportFile{BatchFileResult: base, OutputPath: outputPath, MIME: mime}
}

func batchExportContent(model *epinput.Model, doc idf.Document, targetFormat string) ([]byte, string, string, error) {
	switch strings.ToLower(strings.TrimSpace(targetFormat)) {
	case "", "idf":
		output, err := epinput.Write(model, epinput.FormatIDF)
		return []byte(output), ".idf", "text/plain", err
	case "epjson", "json":
		output, err := epinput.Write(model, epinput.FormatEPJSON)
		return []byte(output), ".epjson", "application/json", err
	case "semantic-yaml", "semantic_yaml", "yaml", "yml":
		projection := semanticProjectionForModelDoc(model, doc)
		if projection == nil {
			return nil, ".semantic.yaml", "application/x-yaml", fmt.Errorf("semantic projection unavailable")
		}
		return []byte(projection.Text), ".semantic.yaml", "application/x-yaml", nil
	case "xlsx", "table", "tables":
		var b bytes.Buffer
		if err := tabular.WriteOneSheetXLSX(&b, "IDF Tables", idf.ObjectTableSections(doc)); err != nil {
			return nil, ".tables.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", err
		}
		return b.Bytes(), ".tables.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", nil
	case "analysis-json", "analysis_json", "report-json":
		report := idf.Analyze(doc)
		payload, err := json.MarshalIndent(report, "", "  ")
		return append(payload, '\n'), ".analysis.json", "application/json", err
	default:
		return nil, "", "", fmt.Errorf("unsupported batch export format %q", targetFormat)
	}
}

func batchSummaryWorkbookSheets(request BatchSummaryXLSXExportRequest) []tabular.WorkbookSheet {
	raw := batchSummaryRawSection(request.Result, request.Orientation)
	sheets := []tabular.WorkbookSheet{{Name: "Raw", Sections: []tabular.Section{raw}}}
	delta := batchSummaryDeltaSection(request.Result, request.BaselineIndex, request.CompareIndex)
	if len(delta.Rows) > 0 {
		sheets = append(sheets, tabular.WorkbookSheet{Name: "Delta", Sections: []tabular.Section{delta}})
	}
	return sheets
}

func batchSimulationWorkbookSheets(request BatchSimulationXLSXExportRequest) []tabular.WorkbookSheet {
	sections := []tabular.WorkbookSheet{
		{Name: "Purpose Metrics", Sections: []tabular.Section{batchSimulationPurposeMetricSection(request.Result)}},
	}
	if context := batchSimulationRunContextSection(request); len(context.Rows) > 0 {
		sections = append(sections, tabular.WorkbookSheet{Name: "Run Context", Sections: []tabular.Section{context}})
	}
	left, right, ok := batchSimulationComparisonResults(request)
	if ok {
		sections = append(sections, tabular.WorkbookSheet{Name: "Comparison", Sections: []tabular.Section{batchSimulationComparisonSection(left, right)}})
		if completeness := batchSimulationCompletenessDeltaSection(left, right); len(completeness.Rows) > 0 {
			sections = append(sections, tabular.WorkbookSheet{Name: "Completeness Delta", Sections: []tabular.Section{completeness}})
		}
		if delta := batchSimulationEnergyDeltaSection(left, right); len(delta.Rows) > 0 {
			sections = append(sections, tabular.WorkbookSheet{Name: "Energy Delta", Sections: []tabular.Section{delta}})
		}
		if edgeDelta := batchSimulationEnergyEdgeDeltaSection(left, right); len(edgeDelta.Rows) > 0 {
			sections = append(sections, tabular.WorkbookSheet{Name: "Sankey Edge Delta", Sections: []tabular.Section{edgeDelta}})
		}
	}
	if summary := batchSimulationEnergySummarySection(request.Result); len(summary.Rows) > 0 {
		sections = append(sections, tabular.WorkbookSheet{Name: "Energy Summary", Sections: []tabular.Section{summary}})
	}
	if sources := batchSimulationEnergySourceSection(request.Result); len(sources.Rows) > 0 {
		sections = append(sections, tabular.WorkbookSheet{Name: "Energy Sources", Sections: []tabular.Section{sources}})
	}
	if availability := batchSimulationEnergySourceAvailabilitySection(request.Result); len(availability.Rows) > 0 {
		sections = append(sections, tabular.WorkbookSheet{Name: "Source Availability", Sections: []tabular.Section{availability}})
	}
	if edges := batchSimulationEnergyEdgeSection(request.Result); len(edges.Rows) > 0 {
		sections = append(sections, tabular.WorkbookSheet{Name: "Energy Edges", Sections: []tabular.Section{edges}})
	}
	if reconciliation := batchSimulationEnergyReconciliationSection(request.Result); len(reconciliation.Rows) > 0 {
		sections = append(sections, tabular.WorkbookSheet{Name: "Reconciliation", Sections: []tabular.Section{reconciliation}})
	}
	if warnings := batchSimulationEnergyWarningSection(request.Result); len(warnings.Rows) > 0 {
		sections = append(sections, tabular.WorkbookSheet{Name: "Energy Warnings", Sections: []tabular.Section{warnings}})
	}
	return sections
}

func batchSimulationRunContextSection(request BatchSimulationXLSXExportRequest) tabular.Section {
	context := request.Context
	if !hasBatchSimulationRunContext(context) {
		return tabular.Section{}
	}
	purposeRequest := simulation.NormalizeSimulationPurposeRequest(&context.PurposeRequest)
	comparison := batchSimulationComparisonContext(request)
	section := tabular.Section{
		Title:   "run_context",
		Headers: []string{"field", "value"},
		Rows: [][]string{
			{"run_id", request.Result.RunID},
			{"total", fmt.Sprint(request.Result.Total)},
			{"completed", fmt.Sprint(request.Result.Completed)},
			{"succeeded", fmt.Sprint(request.Result.Succeeded)},
			{"failed", fmt.Sprint(request.Result.Failed)},
			{"root_directory", context.RootDirectory},
			{"selected_paths", strings.Join(context.SelectedPaths, "; ")},
			{"selected_row_ids", strings.Join(context.SelectedRowIDs, "; ")},
			{"metric", context.Metric},
			{"sort", context.Sort},
			{"view_mode", context.ViewMode},
			{"weather_mode", context.WeatherMode},
			{"weather_path", context.WeatherPath},
			{"worker_count", fmt.Sprint(context.WorkerCount)},
			{"comparison_baseline_row_id", comparison.BaselineRowID},
			{"comparison_target_row_id", comparison.TargetRowID},
			{"purpose_ids", strings.Join(batchSimulationPurposeIDStrings(purposeRequest.Purposes), "; ")},
			{"frequency_policy", purposeRequest.FrequencyPolicy},
			{"allocation_policy", purposeRequest.AllocationPolicy},
			{"basic_energy_detail", purposeRequest.BasicEnergyDetail},
			{"sql_mode", purposeRequest.SQLMode},
			{"output_apply_mode", purposeRequest.OutputApplyMode},
			{"persist_outputs", fmt.Sprint(purposeRequest.PersistOutputs)},
			{"discovery_allowed", fmt.Sprint(purposeRequest.DiscoveryAllowed)},
			{"scope_zone_mode", purposeRequest.Scope.ZoneMode},
			{"scope_zone_names", strings.Join(purposeRequest.Scope.ZoneNames, "; ")},
			{"scope_period_mode", purposeRequest.Scope.PeriodMode},
			{"scope_period_start", purposeRequest.Scope.PeriodStart},
			{"scope_period_end", purposeRequest.Scope.PeriodEnd},
			{"scope_loop_mode", purposeRequest.Scope.LoopMode},
			{"scope_air_loop_names", strings.Join(purposeRequest.Scope.AirLoopNames, "; ")},
			{"scope_plant_loop_names", strings.Join(purposeRequest.Scope.PlantLoopNames, "; ")},
			{"scope_condenser_loop_names", strings.Join(purposeRequest.Scope.CondenserLoopNames, "; ")},
			{"scope_component_ids", strings.Join(purposeRequest.Scope.ComponentIDs, "; ")},
			{"scope_output_signatures", strings.Join(purposeRequest.Scope.OutputSignatures, "; ")},
			{"scope_custom_output_count", fmt.Sprint(len(purposeRequest.Scope.CustomOutputs))},
		},
	}
	return section
}

func hasBatchSimulationRunContext(context BatchSimulationXLSXExportContext) bool {
	if len(context.SelectedPaths) > 0 || context.RootDirectory != "" || len(context.SelectedRowIDs) > 0 ||
		context.Metric != "" || context.Sort != "" || context.ViewMode != "" || context.WeatherMode != "" ||
		context.WeatherPath != "" || context.WorkerCount != 0 || context.Comparison.BaselineRowID != "" ||
		context.Comparison.TargetRowID != "" {
		return true
	}
	purpose := context.PurposeRequest
	return len(purpose.Purposes) > 0 || purpose.FrequencyPolicy != "" || purpose.SQLMode != "" ||
		purpose.AllocationPolicy != "" || purpose.PersistOutputs || purpose.DiscoveryAllowed ||
		purpose.BasicEnergyDetail != "" || purpose.OutputApplyMode != "" || purpose.Scope.ZoneMode != "" || len(purpose.Scope.ZoneNames) > 0 ||
		purpose.Scope.PeriodMode != "" || purpose.Scope.PeriodStart != "" || purpose.Scope.PeriodEnd != "" ||
		purpose.Scope.LoopMode != "" || len(purpose.Scope.AirLoopNames) > 0 ||
		len(purpose.Scope.PlantLoopNames) > 0 || len(purpose.Scope.CondenserLoopNames) > 0 ||
		len(purpose.Scope.ComponentIDs) > 0 || len(purpose.Scope.OutputSignatures) > 0 ||
		len(purpose.Scope.CustomOutputs) > 0
}

func batchSimulationPurposeIDStrings(purposes []simulation.SimulationPurposeID) []string {
	ids := make([]string, 0, len(purposes))
	for _, purpose := range purposes {
		if purpose != "" {
			ids = append(ids, string(purpose))
		}
	}
	return ids
}

func batchSimulationComparisonContext(request BatchSimulationXLSXExportRequest) BatchSimulationComparisonXLSXContext {
	comparison := request.Comparison
	if comparison.BaselineRowID == "" && comparison.TargetRowID == "" {
		comparison = request.Context.Comparison
	}
	return comparison
}

func batchSimulationComparisonResults(request BatchSimulationXLSXExportRequest) (simulation.SimulationRunResult, simulation.SimulationRunResult, bool) {
	candidates := make([]simulation.SimulationRunResult, 0, len(request.Result.Results))
	byID := map[string]simulation.SimulationRunResult{}
	for _, item := range request.Result.Results {
		if item.PurposeResults == nil || item.PurposeResults.EnergyExplanationSummary.Schema == "" {
			continue
		}
		candidates = append(candidates, item)
		if id := batchSimulationRowID(item); id != "" {
			byID[id] = item
		}
	}
	comparison := batchSimulationComparisonContext(request)
	if comparison.BaselineRowID != "" && comparison.TargetRowID != "" && comparison.BaselineRowID != comparison.TargetRowID {
		left, leftOK := byID[comparison.BaselineRowID]
		right, rightOK := byID[comparison.TargetRowID]
		if leftOK && rightOK {
			return left, right, true
		}
	}
	if len(candidates) >= 2 {
		return candidates[0], candidates[1], true
	}
	return simulation.SimulationRunResult{}, simulation.SimulationRunResult{}, false
}

func batchSimulationComparisonSection(left, right simulation.SimulationRunResult) tabular.Section {
	return tabular.Section{
		Title:   "comparison",
		Headers: []string{"role", "row_id", "file", "run_id", "status", "input_path"},
		Rows: [][]string{
			{"baseline", batchSimulationRowID(left), batchSimulationFileLabel(left), left.RunID, left.Status, left.InputPath},
			{"target", batchSimulationRowID(right), batchSimulationFileLabel(right), right.RunID, right.Status, right.InputPath},
		},
	}
}

func batchSimulationCompletenessDeltaSection(left, right simulation.SimulationRunResult) tabular.Section {
	leftCompleteness := left.PurposeResults.EnergyExplanationSummary.Completeness
	rightCompleteness := right.PurposeResults.EnergyExplanationSummary.Completeness
	section := tabular.Section{
		Title:   "completeness_delta",
		Headers: []string{"metric", "baseline_file", "target_file", "baseline_value", "target_value"},
	}
	rows := []struct {
		label string
		left  string
		right string
	}{
		{label: "status", left: leftCompleteness.Status, right: rightCompleteness.Status},
		{
			label: "mapped_energy_percent",
			left:  formatBatchSimulationOptionalFloat(leftCompleteness.MappedPercent, "", "%"),
			right: formatBatchSimulationOptionalFloat(rightCompleteness.MappedPercent, "", "%"),
		},
		{
			label: "missing_categories",
			left:  batchSimulationStringListSummary(leftCompleteness.MissingCategories),
			right: batchSimulationStringListSummary(rightCompleteness.MissingCategories),
		},
		{
			label: "missing_source_outputs",
			left:  batchSimulationSourceAvailabilitySummary(leftCompleteness.SourceAvailability, "missing"),
			right: batchSimulationSourceAvailabilitySummary(rightCompleteness.SourceAvailability, "missing"),
		},
		{
			label: "not_applicable_source_outputs",
			left:  batchSimulationSourceAvailabilitySummary(leftCompleteness.SourceAvailability, "not_applicable"),
			right: batchSimulationSourceAvailabilitySummary(rightCompleteness.SourceAvailability, "not_applicable"),
		},
	}
	for _, row := range rows {
		if row.left == row.right {
			continue
		}
		section.Rows = append(section.Rows, []string{
			row.label,
			batchSimulationFileLabel(left),
			batchSimulationFileLabel(right),
			row.left,
			row.right,
		})
	}
	return section
}

func batchSimulationStringListSummary(items []string) string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			values = append(values, item)
		}
	}
	sort.Strings(values)
	return batchSimulationPreviewSummary(values)
}

func batchSimulationSourceAvailabilitySummary(items []simulation.EnergySourceAvailabilityEntry, status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	values := []string{}
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item.Status)) != status {
			continue
		}
		values = append(values, batchSimulationAvailabilityLabel(item.Level, item.Name))
	}
	sort.Strings(values)
	return batchSimulationPreviewSummary(values)
}

func batchSimulationPreviewSummary(values []string) string {
	if len(values) == 0 {
		return "0"
	}
	limit := 3
	if len(values) < limit {
		limit = len(values)
	}
	preview := strings.Join(values[:limit], "; ")
	if len(values) > 3 {
		return fmt.Sprintf("%d: %s; ...", len(values), preview)
	}
	return fmt.Sprintf("%d: %s", len(values), preview)
}

func batchSimulationAvailabilityLabel(level string, name string) string {
	values := []string{}
	for _, value := range []string{level, name} {
		value = strings.TrimSpace(value)
		if value != "" {
			values = append(values, value)
		}
	}
	return strings.Join(values, ": ")
}

type batchSimulationDeltaRow struct {
	Group               string
	ID                  string
	Label               string
	LeftValue           float64
	RightValue          float64
	Delta               float64
	Percent             string
	Unit                string
	Status              string
	Level               string
	ServiceKind         string
	PathType            string
	Basis               string
	HeatCategory        string
	Sign                string
	Formula             string
	NumeratorLabel      string
	NumeratorUnit       string
	LeftNumerator       float64
	RightNumerator      float64
	DenominatorLabel    string
	DenominatorUnit     string
	LeftDenominator     float64
	RightDenominator    float64
	LeftRatioPresent    bool
	RightRatioPresent   bool
	Relation            string
	RuleID              string
	FromID              string
	ToID                string
	SourceIDs           []string
	RelatedPathIDs      []string
	LeftSourceIDs       []string
	RightSourceIDs      []string
	LeftRelatedPathIDs  []string
	RightRelatedPathIDs []string
}

func batchSimulationEnergyDeltaSection(left, right simulation.SimulationRunResult) tabular.Section {
	section := tabular.Section{
		Title:   "energy_delta",
		Headers: []string{"type", "id", "label", "baseline_file", "target_file", "baseline_value", "target_value", "delta", "percent", "unit", "status", "level", "service_kind", "path_type", "basis", "heat_category", "sign", "formula", "numerator_label", "baseline_numerator", "target_numerator", "numerator_unit", "denominator_label", "baseline_denominator", "target_denominator", "denominator_unit", "baseline_source_ids", "target_source_ids", "baseline_source_object_index", "target_source_object_index", "baseline_source_table", "target_source_table", "baseline_source_row", "target_source_row", "baseline_source_column", "target_source_column", "baseline_source_unit", "target_source_unit", "baseline_normalized_unit", "target_normalized_unit"},
	}
	leftSummary := left.PurposeResults.EnergyExplanationSummary
	rightSummary := right.PurposeResults.EnergyExplanationSummary
	leftExplanation := left.PurposeResults.EnergyExplanation
	rightExplanation := right.PurposeResults.EnergyExplanation
	rows := []batchSimulationDeltaRow{}
	for _, group := range []struct {
		name  string
		left  []simulation.EnergyExplanationSummaryItem
		right []simulation.EnergyExplanationSummaryItem
	}{
		{name: "energy_by_end_use", left: leftSummary.EnergyByEndUse, right: rightSummary.EnergyByEndUse},
		{name: "delivered_load_by_service", left: leftSummary.DeliveredLoadByService, right: rightSummary.DeliveredLoadByService},
		{name: "derived_kpi", left: leftSummary.DerivedKPIs, right: rightSummary.DerivedKPIs},
		{name: "heat_drivers", left: leftSummary.HeatDrivers, right: rightSummary.HeatDrivers},
		{name: "residuals", left: leftSummary.Residuals, right: rightSummary.Residuals},
	} {
		rows = append(rows, batchSimulationSummaryDeltaRows(group.name, group.left, group.right)...)
	}
	sortBatchSimulationDeltaRows(rows)
	for _, row := range rows {
		values := []string{
			row.Group,
			row.ID,
			row.Label,
			batchSimulationFileLabel(left),
			batchSimulationFileLabel(right),
			formatBatchSimulationFloat(row.LeftValue),
			formatBatchSimulationFloat(row.RightValue),
			formatBatchSimulationFloat(row.Delta),
			row.Percent,
			row.Unit,
			row.Status,
			row.Level,
			row.ServiceKind,
			row.PathType,
			row.Basis,
			row.HeatCategory,
			row.Sign,
			row.Formula,
			row.NumeratorLabel,
			formatBatchSimulationOptionalFloatPresent(row.LeftNumerator, row.LeftRatioPresent),
			formatBatchSimulationOptionalFloatPresent(row.RightNumerator, row.RightRatioPresent),
			row.NumeratorUnit,
			row.DenominatorLabel,
			formatBatchSimulationOptionalFloatPresent(row.LeftDenominator, row.LeftRatioPresent),
			formatBatchSimulationOptionalFloatPresent(row.RightDenominator, row.RightRatioPresent),
			row.DenominatorUnit,
			strings.Join(row.LeftSourceIDs, "; "),
			strings.Join(row.RightSourceIDs, "; "),
			batchSimulationSourceObjectIndexes(leftExplanation, row.LeftSourceIDs),
			batchSimulationSourceObjectIndexes(rightExplanation, row.RightSourceIDs),
		}
		values = append(values, batchSimulationSourceMetadataDeltaFields(leftExplanation, rightExplanation, row.LeftSourceIDs, row.RightSourceIDs)...)
		section.Rows = append(section.Rows, values)
	}
	return section
}

func batchSimulationSummaryDeltaRows(group string, leftItems, rightItems []simulation.EnergyExplanationSummaryItem) []batchSimulationDeltaRow {
	left := map[string]simulation.EnergyExplanationSummaryItem{}
	right := map[string]simulation.EnergyExplanationSummaryItem{}
	ids := map[string]struct{}{}
	for _, item := range leftItems {
		if item.ID == "" {
			continue
		}
		left[item.ID] = item
		ids[item.ID] = struct{}{}
	}
	for _, item := range rightItems {
		if item.ID == "" {
			continue
		}
		right[item.ID] = item
		ids[item.ID] = struct{}{}
	}
	rows := make([]batchSimulationDeltaRow, 0, len(ids))
	for id := range ids {
		leftItem, leftOK := left[id]
		rightItem, rightOK := right[id]
		leftValue := 0.0
		if leftOK {
			leftValue = leftItem.Value
		}
		rightValue := 0.0
		if rightOK {
			rightValue = rightItem.Value
		}
		item := rightItem
		if !rightOK {
			item = leftItem
		}
		rows = append(rows, batchSimulationDeltaRow{
			Group:             group,
			ID:                id,
			Label:             firstNonEmpty(item.Label, id),
			LeftValue:         leftValue,
			RightValue:        rightValue,
			Delta:             rightValue - leftValue,
			Percent:           batchSimulationPercentDelta(leftValue, rightValue-leftValue),
			Unit:              firstNonEmpty(rightItem.Unit, leftItem.Unit),
			Status:            batchSimulationDeltaStatus(leftOK, rightOK, leftValue, rightValue),
			Level:             firstNonEmpty(rightItem.Level, leftItem.Level),
			ServiceKind:       firstNonEmpty(rightItem.ServiceKind, leftItem.ServiceKind),
			PathType:          firstNonEmpty(rightItem.PathType, leftItem.PathType),
			Basis:             firstNonEmpty(rightItem.Basis, leftItem.Basis),
			HeatCategory:      firstNonEmpty(rightItem.HeatCategory, leftItem.HeatCategory),
			Sign:              firstNonEmpty(rightItem.Sign, leftItem.Sign),
			Formula:           firstNonEmpty(rightItem.Formula, leftItem.Formula),
			NumeratorLabel:    firstNonEmpty(rightItem.NumeratorLabel, leftItem.NumeratorLabel),
			NumeratorUnit:     firstNonEmpty(rightItem.NumeratorUnit, leftItem.NumeratorUnit),
			LeftNumerator:     leftItem.NumeratorValue,
			RightNumerator:    rightItem.NumeratorValue,
			DenominatorLabel:  firstNonEmpty(rightItem.DenominatorLabel, leftItem.DenominatorLabel),
			DenominatorUnit:   firstNonEmpty(rightItem.DenominatorUnit, leftItem.DenominatorUnit),
			LeftDenominator:   leftItem.DenominatorValue,
			RightDenominator:  rightItem.DenominatorValue,
			LeftRatioPresent:  leftOK && batchSimulationSummaryRatioPresent(leftItem),
			RightRatioPresent: rightOK && batchSimulationSummaryRatioPresent(rightItem),
			LeftSourceIDs:     stringSliceWhenPresent(leftOK, leftItem.SourceIDs),
			RightSourceIDs:    stringSliceWhenPresent(rightOK, rightItem.SourceIDs),
		})
	}
	return rows
}

func stringSliceWhenPresent(present bool, values []string) []string {
	if !present {
		return nil
	}
	return values
}

func batchSimulationSummaryRatioPresent(item simulation.EnergyExplanationSummaryItem) bool {
	return item.NumeratorValue != 0 || item.DenominatorValue != 0 || item.NumeratorLabel != "" || item.DenominatorLabel != ""
}

func batchSimulationEnergyEdgeDeltaSection(left, right simulation.SimulationRunResult) tabular.Section {
	section := tabular.Section{
		Title:   "sankey_edge_delta",
		Headers: []string{"relation", "edge", "rule_id", "baseline_file", "target_file", "baseline_value", "target_value", "delta", "percent", "unit", "status", "basis", "from_id", "to_id", "baseline_source_ids", "target_source_ids", "baseline_source_object_index", "target_source_object_index", "baseline_related_path_ids", "target_related_path_ids", "baseline_source_table", "target_source_table", "baseline_source_row", "target_source_row", "baseline_source_column", "target_source_column", "baseline_source_unit", "target_source_unit", "baseline_normalized_unit", "target_normalized_unit"},
	}
	leftExplanation := left.PurposeResults.EnergyExplanation
	rightExplanation := right.PurposeResults.EnergyExplanation
	rows := batchSimulationEdgeDeltaRows(leftExplanation, rightExplanation)
	sortBatchSimulationDeltaRows(rows)
	for _, row := range rows {
		values := []string{
			row.Relation,
			row.Label,
			row.RuleID,
			batchSimulationFileLabel(left),
			batchSimulationFileLabel(right),
			formatBatchSimulationFloat(row.LeftValue),
			formatBatchSimulationFloat(row.RightValue),
			formatBatchSimulationFloat(row.Delta),
			row.Percent,
			row.Unit,
			row.Status,
			row.Basis,
			row.FromID,
			row.ToID,
			strings.Join(row.LeftSourceIDs, "; "),
			strings.Join(row.RightSourceIDs, "; "),
			batchSimulationSourceObjectIndexes(leftExplanation, row.LeftSourceIDs),
			batchSimulationSourceObjectIndexes(rightExplanation, row.RightSourceIDs),
			strings.Join(row.LeftRelatedPathIDs, "; "),
			strings.Join(row.RightRelatedPathIDs, "; "),
		}
		values = append(values, batchSimulationSourceMetadataDeltaFields(leftExplanation, rightExplanation, row.LeftSourceIDs, row.RightSourceIDs)...)
		section.Rows = append(section.Rows, values)
	}
	return section
}

func batchSimulationEdgeDeltaRows(leftExplanation, rightExplanation simulation.EnergyExplanationResult) []batchSimulationDeltaRow {
	left := batchSimulationAnnualEdgeMap(leftExplanation)
	right := batchSimulationAnnualEdgeMap(rightExplanation)
	ids := map[string]struct{}{}
	for id := range left {
		ids[id] = struct{}{}
	}
	for id := range right {
		ids[id] = struct{}{}
	}
	rows := make([]batchSimulationDeltaRow, 0, len(ids))
	for id := range ids {
		leftEdge, leftOK := left[id]
		rightEdge, rightOK := right[id]
		leftValue := 0.0
		if leftOK {
			leftValue = leftEdge.LeftValue
		}
		rightValue := 0.0
		if rightOK {
			rightValue = rightEdge.RightValue
		}
		item := rightEdge
		if !rightOK {
			item = leftEdge
		}
		item.ID = id
		item.LeftValue = leftValue
		item.RightValue = rightValue
		item.Delta = rightValue - leftValue
		item.Percent = batchSimulationPercentDelta(leftValue, item.Delta)
		item.Status = batchSimulationDeltaStatus(leftOK, rightOK, leftValue, rightValue)
		item.LeftSourceIDs = stringSliceWhenPresent(leftOK, leftEdge.SourceIDs)
		item.RightSourceIDs = stringSliceWhenPresent(rightOK, rightEdge.SourceIDs)
		item.LeftRelatedPathIDs = stringSliceWhenPresent(leftOK, leftEdge.RelatedPathIDs)
		item.RightRelatedPathIDs = stringSliceWhenPresent(rightOK, rightEdge.RelatedPathIDs)
		rows = append(rows, item)
	}
	return rows
}

func batchSimulationAnnualEdgeMap(explanation simulation.EnergyExplanationResult) map[string]batchSimulationDeltaRow {
	period := batchSimulationAnnualPeriod(explanation)
	nodeLabels := map[string]string{}
	for _, node := range period.Nodes {
		nodeLabels[node.ID] = firstNonEmpty(node.Label, node.Kind, node.ID)
	}
	out := map[string]batchSimulationDeltaRow{}
	for _, edge := range period.Edges {
		id := firstNonEmpty(edge.ID, batchSimulationEdgeFallbackID(edge))
		if id == "" {
			continue
		}
		out[id] = batchSimulationDeltaRow{
			ID:             id,
			Label:          fmt.Sprintf("%s -> %s", firstNonEmpty(nodeLabels[edge.FromID], edge.FromID), firstNonEmpty(nodeLabels[edge.ToID], edge.ToID)),
			LeftValue:      edge.Value,
			RightValue:     edge.Value,
			Unit:           edge.Unit,
			Relation:       edge.Relation,
			RuleID:         edge.RuleID,
			Basis:          edge.Basis,
			FromID:         edge.FromID,
			ToID:           edge.ToID,
			SourceIDs:      edge.SourceIDs,
			RelatedPathIDs: edge.RelatedPathIDs,
		}
	}
	return out
}

func batchSimulationEdgeFallbackID(edge simulation.EnergyExplanationEdge) string {
	parts := []string{}
	for _, part := range []string{edge.Relation, edge.RuleID, edge.FromID, edge.ToID} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "|")
}

func batchSimulationAnnualPeriod(explanation simulation.EnergyExplanationResult) simulation.EnergyPeriod {
	for _, period := range batchSimulationExportPeriods(explanation) {
		if period.ID == "annual" || period.Kind == "annual" {
			return period
		}
	}
	return simulation.EnergyPeriod{
		ID:             "annual",
		Kind:           "annual",
		Nodes:          explanation.Nodes,
		Edges:          explanation.Edges,
		Reconciliation: explanation.Reconciliation,
		Warnings:       explanation.Warnings,
	}
}

func sortBatchSimulationDeltaRows(rows []batchSimulationDeltaRow) {
	sort.Slice(rows, func(i, j int) bool {
		if left, right := absBatchSimulationFloat(rows[i].Delta), absBatchSimulationFloat(rows[j].Delta); left != right {
			return left > right
		}
		if rows[i].Group != rows[j].Group {
			return rows[i].Group < rows[j].Group
		}
		if rows[i].Relation != rows[j].Relation {
			return rows[i].Relation < rows[j].Relation
		}
		return rows[i].Label < rows[j].Label
	})
}

func batchSimulationDeltaStatus(leftOK, rightOK bool, leftValue float64, rightValue float64) string {
	switch {
	case !leftOK && rightOK:
		return "missing in baseline"
	case leftOK && !rightOK:
		return "missing in comparison"
	case leftValue == 0 && rightValue != 0:
		return "zero baseline"
	case leftValue != 0 && rightValue == 0:
		return "zero comparison"
	default:
		return "matched"
	}
}

func batchSimulationPercentDelta(leftValue, delta float64) string {
	if leftValue == 0 {
		return "N/A"
	}
	return formatBatchSimulationFloat((delta/leftValue)*100) + "%"
}

func absBatchSimulationFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func batchSimulationPurposeMetricSection(result simulation.MultiSimulationResult) tabular.Section {
	section := tabular.Section{
		Title:   "purpose_metrics",
		Headers: []string{"file", "status", "run_id", "metric_id", "label", "value", "unit", "display_value", "purpose_id", "metric_status"},
	}
	for _, item := range result.Results {
		file := batchSimulationFileLabel(item)
		for _, metric := range item.PurposeMetrics {
			section.Rows = append(section.Rows, []string{
				file,
				item.Status,
				item.RunID,
				metric.ID,
				metric.Label,
				formatBatchSimulationFloat(metric.Value),
				metric.Unit,
				metric.DisplayValue,
				string(metric.PurposeID),
				metric.Status,
			})
		}
	}
	return section
}

func batchSimulationEnergySummarySection(result simulation.MultiSimulationResult) tabular.Section {
	section := tabular.Section{
		Title:   "energy_summary",
		Headers: []string{"file", "status", "run_id", "type", "id", "label", "value", "unit", "level", "service_kind", "path_type", "carrier", "end_use", "heat_category", "sign", "basis", "formula", "numerator_label", "numerator_value", "numerator_unit", "denominator_label", "denominator_value", "denominator_unit", "source_ids", "source_object_index"},
	}
	for _, item := range result.Results {
		summary := simulation.EnergyExplanationSummary{}
		explanation := simulation.EnergyExplanationResult{}
		if item.PurposeResults != nil {
			summary = item.PurposeResults.EnergyExplanationSummary
			explanation = item.PurposeResults.EnergyExplanation
		}
		file := batchSimulationFileLabel(item)
		for _, group := range batchSimulationSummaryGroups(summary) {
			for _, metric := range group.items {
				section.Rows = append(section.Rows, []string{
					file,
					item.Status,
					item.RunID,
					group.name,
					metric.ID,
					metric.Label,
					formatBatchSimulationFloat(metric.Value),
					metric.Unit,
					metric.Level,
					metric.ServiceKind,
					metric.PathType,
					metric.Carrier,
					metric.EndUse,
					metric.HeatCategory,
					metric.Sign,
					metric.Basis,
					metric.Formula,
					metric.NumeratorLabel,
					formatBatchSimulationOptionalFloat(metric.NumeratorValue, metric.NumeratorLabel, metric.NumeratorUnit),
					metric.NumeratorUnit,
					metric.DenominatorLabel,
					formatBatchSimulationOptionalFloat(metric.DenominatorValue, metric.DenominatorLabel, metric.DenominatorUnit),
					metric.DenominatorUnit,
					strings.Join(metric.SourceIDs, "; "),
					batchSimulationSourceObjectIndexes(explanation, metric.SourceIDs),
				})
			}
		}
	}
	return section
}

func batchSimulationEnergySourceSection(result simulation.MultiSimulationResult) tabular.Section {
	section := tabular.Section{
		Title:   "energy_sources",
		Headers: []string{"file", "status", "run_id", "id", "source_type", "basis", "key", "name", "reporting_frequency", "aggregation", "source_unit", "normalized_unit", "index_group", "object_index", "table", "row", "column"},
	}
	for _, item := range result.Results {
		if item.PurposeResults == nil {
			continue
		}
		file := batchSimulationFileLabel(item)
		for _, source := range item.PurposeResults.EnergyExplanation.Sources {
			objectIndex := ""
			if source.ObjectIndex != nil {
				objectIndex = fmt.Sprintf("%d", *source.ObjectIndex)
			}
			basis := "variable"
			if source.IsMeter {
				basis = "meter"
			}
			section.Rows = append(section.Rows, []string{
				file,
				item.Status,
				item.RunID,
				source.ID,
				source.SourceType,
				basis,
				source.KeyValue,
				source.Name,
				source.ReportingFrequency,
				source.AggregationMethod,
				firstNonEmpty(source.SourceUnit, source.Units),
				source.NormalizedUnit,
				source.IndexGroup,
				objectIndex,
				source.TableName,
				source.RowName,
				source.ColumnName,
			})
		}
	}
	return section
}

func batchSimulationEnergySourceAvailabilitySection(result simulation.MultiSimulationResult) tabular.Section {
	section := tabular.Section{
		Title:   "source_availability",
		Headers: []string{"file", "run_status", "run_id", "level", "output", "availability_status", "source_ids", "source_object_index", "source_table", "source_row", "source_column", "source_unit", "normalized_unit"},
	}
	for _, item := range result.Results {
		if item.PurposeResults == nil {
			continue
		}
		file := batchSimulationFileLabel(item)
		explanation := item.PurposeResults.EnergyExplanation
		for _, availability := range explanation.Completeness.SourceAvailability {
			section.Rows = append(section.Rows, []string{
				file,
				item.Status,
				item.RunID,
				availability.Level,
				availability.Name,
				availability.Status,
				strings.Join(availability.SourceIDs, "; "),
				batchSimulationSourceObjectIndexes(explanation, availability.SourceIDs),
				batchSimulationSourceValueSummary(explanation, availability.SourceIDs, func(source simulation.EnergyDataSource) string { return source.TableName }),
				batchSimulationSourceValueSummary(explanation, availability.SourceIDs, func(source simulation.EnergyDataSource) string { return source.RowName }),
				batchSimulationSourceValueSummary(explanation, availability.SourceIDs, func(source simulation.EnergyDataSource) string { return source.ColumnName }),
				batchSimulationSourceValueSummary(explanation, availability.SourceIDs, func(source simulation.EnergyDataSource) string {
					return firstNonEmpty(source.SourceUnit, source.Units)
				}),
				batchSimulationSourceValueSummary(explanation, availability.SourceIDs, func(source simulation.EnergyDataSource) string { return source.NormalizedUnit }),
			})
		}
	}
	return section
}

func batchSimulationSourceObjectIndexes(explanation simulation.EnergyExplanationResult, sourceIDs []string) string {
	return batchSimulationSourceValueSummary(explanation, sourceIDs, func(source simulation.EnergyDataSource) string {
		if source.ObjectIndex == nil {
			return ""
		}
		return fmt.Sprintf("%d", *source.ObjectIndex)
	})
}

func batchSimulationSourceMetadataDeltaFields(leftExplanation, rightExplanation simulation.EnergyExplanationResult, leftSourceIDs, rightSourceIDs []string) []string {
	return []string{
		batchSimulationSourceValueSummary(leftExplanation, leftSourceIDs, func(source simulation.EnergyDataSource) string { return source.TableName }),
		batchSimulationSourceValueSummary(rightExplanation, rightSourceIDs, func(source simulation.EnergyDataSource) string { return source.TableName }),
		batchSimulationSourceValueSummary(leftExplanation, leftSourceIDs, func(source simulation.EnergyDataSource) string { return source.RowName }),
		batchSimulationSourceValueSummary(rightExplanation, rightSourceIDs, func(source simulation.EnergyDataSource) string { return source.RowName }),
		batchSimulationSourceValueSummary(leftExplanation, leftSourceIDs, func(source simulation.EnergyDataSource) string { return source.ColumnName }),
		batchSimulationSourceValueSummary(rightExplanation, rightSourceIDs, func(source simulation.EnergyDataSource) string { return source.ColumnName }),
		batchSimulationSourceValueSummary(leftExplanation, leftSourceIDs, func(source simulation.EnergyDataSource) string {
			return firstNonEmpty(source.SourceUnit, source.Units)
		}),
		batchSimulationSourceValueSummary(rightExplanation, rightSourceIDs, func(source simulation.EnergyDataSource) string {
			return firstNonEmpty(source.SourceUnit, source.Units)
		}),
		batchSimulationSourceValueSummary(leftExplanation, leftSourceIDs, func(source simulation.EnergyDataSource) string { return source.NormalizedUnit }),
		batchSimulationSourceValueSummary(rightExplanation, rightSourceIDs, func(source simulation.EnergyDataSource) string { return source.NormalizedUnit }),
	}
}

func batchSimulationSourceValueSummary(explanation simulation.EnergyExplanationResult, sourceIDs []string, value func(simulation.EnergyDataSource) string) string {
	sourceByID := map[string]simulation.EnergyDataSource{}
	for _, source := range explanation.Sources {
		sourceByID[source.ID] = source
	}
	seen := map[string]bool{}
	values := []string{}
	for _, sourceID := range sourceIDs {
		source := sourceByID[sourceID]
		field := strings.TrimSpace(value(source))
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		values = append(values, field)
	}
	return strings.Join(values, "; ")
}

func batchSimulationEnergyEdgeSection(result simulation.MultiSimulationResult) tabular.Section {
	section := tabular.Section{
		Title:   "energy_edges",
		Headers: []string{"file", "status", "run_id", "period", "id", "from_id", "to_id", "value", "unit", "relation", "basis", "rule_id", "formula", "zone", "service_kind", "source_ids", "source_object_index", "related_path_ids"},
	}
	for _, item := range result.Results {
		if item.PurposeResults == nil {
			continue
		}
		file := batchSimulationFileLabel(item)
		explanation := item.PurposeResults.EnergyExplanation
		for _, period := range batchSimulationExportPeriods(explanation) {
			for _, edge := range period.Edges {
				section.Rows = append(section.Rows, []string{
					file,
					item.Status,
					item.RunID,
					firstNonEmpty(edge.Period, period.ID),
					edge.ID,
					edge.FromID,
					edge.ToID,
					formatBatchSimulationFloat(edge.Value),
					edge.Unit,
					edge.Relation,
					edge.Basis,
					edge.RuleID,
					edge.Formula,
					edge.ZoneName,
					edge.ServiceKind,
					strings.Join(edge.SourceIDs, "; "),
					batchSimulationSourceObjectIndexes(explanation, edge.SourceIDs),
					strings.Join(edge.RelatedPathIDs, "; "),
				})
			}
		}
	}
	return section
}

func batchSimulationEnergyReconciliationSection(result simulation.MultiSimulationResult) tabular.Section {
	section := tabular.Section{
		Title:   "energy_reconciliation",
		Headers: []string{"file", "status", "run_id", "period", "id", "label", "level", "reconciliation_status", "expected", "explained", "residual", "unit", "basis", "formula", "zone", "service_kind", "source_ids", "source_object_index"},
	}
	for _, item := range result.Results {
		if item.PurposeResults == nil {
			continue
		}
		file := batchSimulationFileLabel(item)
		explanation := item.PurposeResults.EnergyExplanation
		for _, period := range batchSimulationExportPeriods(explanation) {
			for _, row := range period.Reconciliation {
				section.Rows = append(section.Rows, []string{
					file,
					item.Status,
					item.RunID,
					firstNonEmpty(row.Period, period.ID),
					row.ID,
					row.Label,
					row.Level,
					row.Status,
					formatBatchSimulationFloat(row.ExpectedValue),
					formatBatchSimulationFloat(row.ExplainedValue),
					formatBatchSimulationFloat(row.ResidualValue),
					row.Unit,
					row.Basis,
					row.Formula,
					row.ZoneName,
					row.ServiceKind,
					strings.Join(row.SourceIDs, "; "),
					batchSimulationSourceObjectIndexes(explanation, row.SourceIDs),
				})
			}
		}
	}
	return section
}

func batchSimulationEnergyWarningSection(result simulation.MultiSimulationResult) tabular.Section {
	section := tabular.Section{
		Title:   "energy_warnings",
		Headers: []string{"file", "status", "run_id", "period", "severity", "code", "message"},
	}
	for _, item := range result.Results {
		if item.PurposeResults == nil {
			continue
		}
		file := batchSimulationFileLabel(item)
		for _, warning := range batchSimulationEnergyWarningRows(item.PurposeResults.EnergyExplanation) {
			section.Rows = append(section.Rows, []string{
				file,
				item.Status,
				item.RunID,
				warning.Period,
				warning.Severity,
				warning.Code,
				warning.Message,
			})
		}
	}
	return section
}

func batchSimulationEnergyWarningRows(explanation simulation.EnergyExplanationResult) []simulation.EnergyWarning {
	rows := []simulation.EnergyWarning{}
	seen := map[string]bool{}
	add := func(warning simulation.EnergyWarning, period string) {
		if warning.Period == "" {
			warning.Period = period
		}
		if warning.Code == "" && warning.Message == "" {
			return
		}
		key := strings.Join([]string{warning.Severity, warning.Code, warning.Period, warning.Message}, "\x00")
		if seen[key] {
			return
		}
		seen[key] = true
		rows = append(rows, warning)
	}
	for _, warning := range explanation.Warnings {
		add(warning, firstNonEmpty(warning.Period, "annual"))
	}
	for _, period := range batchSimulationExportPeriods(explanation) {
		for _, warning := range period.Warnings {
			add(warning, firstNonEmpty(warning.Period, period.ID))
		}
	}
	return rows
}

type batchSimulationSummaryGroup struct {
	name  string
	items []simulation.EnergyExplanationSummaryItem
}

func batchSimulationSummaryGroups(summary simulation.EnergyExplanationSummary) []batchSimulationSummaryGroup {
	return []batchSimulationSummaryGroup{
		{name: "energy_by_carrier", items: summary.EnergyByCarrier},
		{name: "energy_by_end_use", items: summary.EnergyByEndUse},
		{name: "delivered_load_by_service", items: summary.DeliveredLoadByService},
		{name: "derived_kpi", items: summary.DerivedKPIs},
		{name: "heat_drivers", items: summary.HeatDrivers},
		{name: "residuals", items: summary.Residuals},
		{name: "top_heat_drivers", items: summary.TopHeatDrivers},
		{name: "top_zones", items: summary.TopZones},
	}
}

func batchSimulationExportPeriods(explanation simulation.EnergyExplanationResult) []simulation.EnergyPeriod {
	periods := explanation.Periods
	if len(periods) == 0 && (len(explanation.Edges) > 0 || len(explanation.Reconciliation) > 0 || len(explanation.Warnings) > 0) {
		periods = []simulation.EnergyPeriod{{
			ID:             "annual",
			Kind:           "annual",
			Edges:          explanation.Edges,
			Reconciliation: explanation.Reconciliation,
			Warnings:       explanation.Warnings,
		}}
	}
	out := []simulation.EnergyPeriod{}
	for _, period := range periods {
		kind := strings.ToLower(strings.TrimSpace(period.Kind))
		id := strings.ToLower(strings.TrimSpace(period.ID))
		if kind == "annual" || kind == "monthly" || kind == "selected_range" || id == "annual" || id == "selected_range" {
			out = append(out, period)
		}
	}
	return out
}

func batchSimulationFileLabel(item simulation.SimulationRunResult) string {
	return firstNonEmpty(item.Filename, filepath.Base(item.InputPath), item.RunID)
}

func batchSimulationRowID(item simulation.SimulationRunResult) string {
	return firstNonEmpty(item.RunID, item.InputPath, item.Filename)
}

func formatBatchSimulationFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
}

func formatBatchSimulationOptionalFloat(value float64, label string, unit string) string {
	if value == 0 && strings.TrimSpace(label) == "" && strings.TrimSpace(unit) == "" {
		return ""
	}
	return formatBatchSimulationFloat(value)
}

func formatBatchSimulationOptionalFloatPresent(value float64, present bool) string {
	if !present {
		return ""
	}
	return formatBatchSimulationFloat(value)
}

func batchSummaryRawSection(result MultiSummaryResult, orientation string) tabular.Section {
	orientation = strings.ToLower(strings.TrimSpace(orientation))
	if orientation == "files" {
		headers := []string{"file", "status"}
		for _, metric := range result.Metrics {
			headers = append(headers, firstNonEmpty(metric.CSVName, metric.ID))
		}
		rows := make([][]string, 0, len(result.Files))
		for _, file := range result.Files {
			row := []string{firstNonEmpty(file.Label, file.Filename), file.Status}
			for _, metric := range result.Metrics {
				row = append(row, batchSummaryValue(file, metric.ID))
			}
			rows = append(rows, row)
		}
		return tabular.Section{Title: "raw_files", Headers: headers, Rows: rows}
	}
	headers := []string{"metric", "category", "unit"}
	for _, file := range result.Files {
		headers = append(headers, firstNonEmpty(file.Label, file.Filename))
	}
	rows := make([][]string, 0, len(result.Metrics))
	for _, metric := range result.Metrics {
		row := []string{firstNonEmpty(metric.CSVName, metric.ID), metric.Category, metric.Unit}
		for _, file := range result.Files {
			row = append(row, batchSummaryValue(file, metric.ID))
		}
		rows = append(rows, row)
	}
	return tabular.Section{Title: "raw_metrics", Headers: headers, Rows: rows}
}

func batchSummaryDeltaSection(result MultiSummaryResult, baselineIndex int, compareIndex int) tabular.Section {
	section := tabular.Section{
		Title:   "selected_delta",
		Headers: []string{"metric", "category", "unit", "A", "B", "delta", "percent", "status"},
	}
	baseline, baselineOK := batchSummaryFileByIndex(result.Files, baselineIndex)
	compare, compareOK := batchSummaryFileByIndex(result.Files, compareIndex)
	if !baselineOK || !compareOK {
		return section
	}
	section.Rows = append(section.Rows,
		[]string{"baseline", firstNonEmpty(baseline.Label, baseline.Filename), "", "", "", "", "", ""},
		[]string{"compare", firstNonEmpty(compare.Label, compare.Filename), "", "", "", "", "", ""},
	)
	for _, metric := range result.Metrics {
		row := batchSummaryDeltaRow(metric, baseline, compare)
		section.Rows = append(section.Rows, row)
	}
	return section
}

func batchSummaryDeltaRow(metric MultiSummaryMetric, baseline MultiSummaryFile, compare MultiSummaryFile) []string {
	a := baseline.MetricValues[metric.ID]
	b := compare.MetricValues[metric.ID]
	aNumber, aOK := parseBatchSummaryNumber(a.DisplayValue)
	bNumber, bOK := parseBatchSummaryNumber(b.DisplayValue)
	status := firstNonEmpty(a.Status, "missing") + " -> " + firstNonEmpty(b.Status, "missing")
	if aOK && bOK && batchSummaryUnit(metric, a.DisplayValue) == batchSummaryUnit(metric, b.DisplayValue) {
		delta := bNumber - aNumber
		percent := "N/A"
		if aNumber != 0 {
			percent = formatBatchSummaryNumber((delta/aNumber)*100) + "%"
		}
		return []string{
			firstNonEmpty(metric.CSVName, metric.ID),
			metric.Category,
			metric.Unit,
			a.DisplayValue,
			b.DisplayValue,
			formatBatchSummaryDelta(delta, metric.Unit),
			percent,
			status,
		}
	}
	change := "unchanged"
	if a.DisplayValue != b.DisplayValue {
		change = "changed"
	}
	return []string{firstNonEmpty(metric.CSVName, metric.ID), metric.Category, metric.Unit, a.DisplayValue, b.DisplayValue, change, "N/A", status}
}

func batchSummaryFileByIndex(files []MultiSummaryFile, index int) (MultiSummaryFile, bool) {
	for _, file := range files {
		if file.Index == index {
			return file, true
		}
	}
	return MultiSummaryFile{}, false
}

func batchSummaryValue(file MultiSummaryFile, metricID string) string {
	if file.Status != "ok" {
		return ""
	}
	if value, ok := file.MetricValues[metricID]; ok {
		return value.DisplayValue
	}
	return "N/A"
}

func parseBatchSummaryNumber(value string) (float64, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, false
	}
	end := 0
	for index, r := range text {
		if (r >= '0' && r <= '9') || r == '-' || r == '+' || r == '.' || r == 'e' || r == 'E' {
			end = index + len(string(r))
			continue
		}
		break
	}
	if end == 0 {
		return 0, false
	}
	var valueNumber float64
	if _, err := fmt.Sscanf(text[:end], "%f", &valueNumber); err != nil {
		return 0, false
	}
	return valueNumber, true
}

func batchSummaryUnit(metric MultiSummaryMetric, displayValue string) string {
	if strings.TrimSpace(metric.Unit) != "" {
		return strings.TrimSpace(metric.Unit)
	}
	_, ok := parseBatchSummaryNumber(displayValue)
	if !ok {
		return ""
	}
	return strings.TrimLeft(strings.TrimSpace(displayValue), "+-0123456789.eE ")
}

func formatBatchSummaryDelta(value float64, unit string) string {
	sign := ""
	if value > 0 {
		sign = "+"
	}
	suffix := ""
	if unit == "%" {
		suffix = " pt"
	} else if strings.TrimSpace(unit) != "" && unit != "-" {
		suffix = " " + unit
	}
	return sign + formatBatchSummaryNumber(value) + suffix
}

func formatBatchSummaryNumber(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", value), "0"), ".")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseBatchInput(path string) (*epinput.Model, idf.Document, error) {
	return parseCachedBatchInput(path)
}

func safeCleanupRuleIDs(scan idf.CleanupScan) []string {
	seen := map[string]bool{}
	var out []string
	for _, candidate := range scan.Candidates {
		if candidate.Risk != "safe" {
			continue
		}
		if !seen[candidate.RuleID] {
			seen[candidate.RuleID] = true
			out = append(out, candidate.RuleID)
		}
	}
	sort.Strings(out)
	return out
}

func newBatchFileResult(index int, path string) BatchFileResult {
	filename := filepath.Base(path)
	return BatchFileResult{
		Index:    index,
		Path:     path,
		Filename: filename,
		Label:    filename,
		Status:   "pending",
	}
}

func normalizeBatchPaths(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func resolveBatchOutputPath(outputDirectory string, sourcePath string, extension string, policy string) (string, error) {
	base := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath)) + extension
	target := filepath.Join(outputDirectory, base)
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", "rename":
		return uniqueBatchOutputPath(target), nil
	case "overwrite":
		return target, nil
	case "skip":
		if _, err := os.Stat(target); err == nil {
			return "", nil
		}
		return target, nil
	case "fail":
		if _, err := os.Stat(target); err == nil {
			return "", fmt.Errorf("%s already exists", target)
		}
		return target, nil
	default:
		return "", fmt.Errorf("unsupported overwrite policy %q", policy)
	}
}

func uniqueBatchOutputPath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for index := 2; index < 10000; index++ {
		candidate := fmt.Sprintf("%s-%d%s", stem, index, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return path
}
