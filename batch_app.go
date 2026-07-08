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
