package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/Gonie-Gonie/idf-analyzer/internal/epinput"
	"github.com/Gonie-Gonie/idf-analyzer/internal/idf"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
}

type TextEditResult struct {
	Text     string      `json:"text"`
	Format   string      `json:"format"`
	Version  string      `json:"version,omitempty"`
	Report   *idf.Report `json:"report"`
	Warnings []string    `json:"warnings,omitempty"`
}

type InputAnalysisResult struct {
	Text    string         `json:"text,omitempty"`
	Format  string         `json:"format"`
	Version string         `json:"version,omitempty"`
	Model   *epinput.Model `json:"model"`
	EPJSON  string         `json:"epjson,omitempty"`
	Report  *idf.Report    `json:"report"`
}

type SummaryExportResult struct {
	Text     string `json:"text"`
	Format   string `json:"format"`
	Filename string `json:"filename"`
	MIME     string `json:"mime"`
}

type InputFileResult struct {
	Canceled bool   `json:"canceled,omitempty"`
	Path     string `json:"path,omitempty"`
	Filename string `json:"filename,omitempty"`
	Text     string `json:"text,omitempty"`
}

type SaveFileResult struct {
	Canceled bool   `json:"canceled,omitempty"`
	Path     string `json:"path,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type CleanupFileResult struct {
	Canceled bool            `json:"canceled,omitempty"`
	Path     string          `json:"path,omitempty"`
	Filename string          `json:"filename,omitempty"`
	Text     string          `json:"text,omitempty"`
	Format   string          `json:"format,omitempty"`
	Version  string          `json:"version,omitempty"`
	Scan     idf.CleanupScan `json:"scan"`
}

type CleanupPreviewResult struct {
	Text              string                 `json:"text"`
	RemovedCandidates []idf.CleanupCandidate `json:"removedCandidates"`
	RemovedCount      int                    `json:"removedCount"`
	ObjectCount       int                    `json:"objectCount"`
}

type CleanupApplyResult struct {
	Canceled     bool   `json:"canceled,omitempty"`
	Path         string `json:"path,omitempty"`
	Filename     string `json:"filename,omitempty"`
	Text         string `json:"text,omitempty"`
	RemovedCount int    `json:"removedCount"`
}

type ProfileApplyTextResult struct {
	Text    string                  `json:"text"`
	Format  string                  `json:"format,omitempty"`
	Version string                  `json:"version,omitempty"`
	Model   *epinput.Model          `json:"model,omitempty"`
	EPJSON  string                  `json:"epjson,omitempty"`
	Report  *idf.Report             `json:"report"`
	Preview idf.ProfileApplyPreview `json:"preview"`
}

type HVACApplyTextResult struct {
	Text    string               `json:"text"`
	Format  string               `json:"format,omitempty"`
	Version string               `json:"version,omitempty"`
	Model   *epinput.Model       `json:"model,omitempty"`
	EPJSON  string               `json:"epjson,omitempty"`
	Report  *idf.Report          `json:"report"`
	Preview idf.HVACApplyPreview `json:"preview"`
}

type OutputApplyTextResult struct {
	Text    string                 `json:"text"`
	Format  string                 `json:"format,omitempty"`
	Version string                 `json:"version,omitempty"`
	Model   *epinput.Model         `json:"model,omitempty"`
	EPJSON  string                 `json:"epjson,omitempty"`
	Report  *idf.Report            `json:"report"`
	Preview idf.OutputApplyPreview `json:"preview"`
}

type AppSettings struct {
	Version     int                         `json:"version"`
	Appearance  AppearanceSettings          `json:"appearance"`
	Behavior    BehaviorSettings            `json:"behavior"`
	Interaction InteractionSettings         `json:"interaction"`
	Profile     idf.ProfileAnalysisSettings `json:"profile"`
	Simulation  SimulationSettings          `json:"simulation"`
}

type AppearanceSettings struct {
	Theme            string                     `json:"theme"`
	Language         string                     `json:"language"`
	AnalysisTabOrder []string                   `json:"analysisTabOrder"`
	Geometry         GeometryAppearanceSettings `json:"geometry"`
}

type GeometryAppearanceSettings struct {
	Background string `json:"background"`
	Zone       string `json:"zone"`
	Wall       string `json:"wall"`
	Roof       string `json:"roof"`
	Window     string `json:"window"`
	Selected   string `json:"selected"`
}

type BehaviorSettings struct {
	AutoAnalyzeDelayMS int `json:"autoAnalyzeDelayMs"`
}

type SimulationSettings struct {
	EnergyPlusInstallations []EnergyPlusInstallSetting `json:"energyPlusInstallations"`
	ExtraWeatherDataPaths   []string                   `json:"extraWeatherDataPaths"`
	RunDirectory            string                     `json:"runDirectory"`
	WorkerFraction          float64                    `json:"workerFraction"`
	MaxWorkers              int                        `json:"maxWorkers"`
	AutoRunOnOpen           bool                       `json:"autoRunOnOpen"`
}

type EnergyPlusInstallSetting struct {
	ID              string `json:"id"`
	Version         string `json:"version"`
	Name            string `json:"name"`
	ExecutablePath  string `json:"executablePath"`
	RootPath        string `json:"rootPath"`
	WeatherDataPath string `json:"weatherDataPath"`
	AutoDetected    bool   `json:"autoDetected"`
}

type InteractionSettings struct {
	SyncRawTextPosition bool              `json:"syncRawTextPosition"`
	GeometrySyncLocate  bool              `json:"geometrySyncLocate"`
	Shortcuts           map[string]string `json:"shortcuts"`
}

type SettingsResult struct {
	Path     string      `json:"path"`
	Settings AppSettings `json:"settings"`
}

type MultiSummaryResult struct {
	Canceled    bool                 `json:"canceled,omitempty"`
	RunID       string               `json:"runId,omitempty"`
	Total       int                  `json:"total"`
	Completed   int                  `json:"completed"`
	Succeeded   int                  `json:"succeeded"`
	Failed      int                  `json:"failed"`
	Concurrency int                  `json:"concurrency"`
	Metrics     []MultiSummaryMetric `json:"metrics"`
	Files       []MultiSummaryFile   `json:"files"`
}

type MultiSummaryMetric struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Unit     string `json:"unit"`
	CSVName  string `json:"csvName"`
}

type MultiSummaryFile struct {
	Index        int                          `json:"index"`
	Path         string                       `json:"path"`
	Filename     string                       `json:"filename"`
	Label        string                       `json:"label"`
	Format       string                       `json:"format,omitempty"`
	Version      string                       `json:"version,omitempty"`
	Status       string                       `json:"status"`
	Error        string                       `json:"error,omitempty"`
	ObjectCount  int                          `json:"objectCount,omitempty"`
	MetricValues map[string]MultiSummaryValue `json:"metricValues,omitempty"`
}

type MultiSummaryValue struct {
	DisplayValue string `json:"displayValue"`
	Status       string `json:"status"`
}

type MultiSummaryProgress struct {
	RunID     string           `json:"runId"`
	Total     int              `json:"total"`
	Completed int              `json:"completed"`
	Succeeded int              `json:"succeeded"`
	Failed    int              `json:"failed"`
	File      MultiSummaryFile `json:"file"`
}

type ModelPatchResult = InputAnalysisResult

func NewApp() *App {
	return &App{}
}

func (a *App) GetAppInfo() AppInfo {
	return currentAppInfo()
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) AnalyzeIDFText(text string) (*idf.Report, error) {
	result, err := a.AnalyzeInputText(text)
	if err != nil {
		return nil, err
	}
	return result.Report, nil
}

func (a *App) AnalyzeInputOverviewText(text string) (*InputAnalysisResult, error) {
	return analyzeInputText(text, idf.AnalyzeOverview, false)
}

func (a *App) AnalyzeInputText(text string) (*InputAnalysisResult, error) {
	return analyzeInputText(text, idf.Analyze, true)
}

func (a *App) AnalyzeInputDiagnosticsText(text string) ([]idf.Diagnostic, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	return idf.AnalyzeDiagnostics(doc), nil
}

func (a *App) AnalyzeInputGeometryText(text string) (*idf.GeometryReport, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	geometry := idf.AnalyzeGeometry(epinput.ToIDFDocument(model))
	return &geometry, nil
}

func (a *App) AnalyzeInputProfileText(text string) (*idf.ProfileReport, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	profile := idf.AnalyzeProfile(epinput.ToIDFDocument(model))
	return &profile, nil
}

func (a *App) AnalyzeInputHVACText(text string) (*idf.HVACReport, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	hvac := idf.AnalyzeHVAC(epinput.ToIDFDocument(model))
	return &hvac, nil
}

func (a *App) AnalyzeInputOutputText(text string) (*idf.OutputReport, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	output := idf.AnalyzeOutput(epinput.ToIDFDocument(model))
	return &output, nil
}

func analyzeInputText(text string, analyze func(idf.Document) idf.Report, includeEPJSON bool) (*InputAnalysisResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	report := analyze(doc)
	epjsonText := ""
	if includeEPJSON {
		epjsonText, err = epinput.Write(model, epinput.FormatEPJSON)
		if err != nil {
			return nil, err
		}
	}

	return &InputAnalysisResult{
		Text:    text,
		Format:  string(model.Format),
		Version: model.Version.Raw,
		Model:   model,
		EPJSON:  epjsonText,
		Report:  &report,
	}, nil
}

func (a *App) PatchModelValueText(text string, objectIndex int, fieldIndex int, jsonPath []string, rawValue string) (*ModelPatchResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	if err := epinput.PatchFieldValue(model, objectIndex, fieldIndex, jsonPath, rawValue); err != nil {
		return nil, err
	}

	resultText, err := epinput.Write(model, model.Format)
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	report := idf.Analyze(doc)
	epjsonText, err := epinput.Write(model, epinput.FormatEPJSON)
	if err != nil {
		return nil, err
	}

	return &ModelPatchResult{
		Text:    resultText,
		Format:  string(model.Format),
		Version: model.Version.Raw,
		Model:   model,
		EPJSON:  epjsonText,
		Report:  &report,
	}, nil
}

func (a *App) OpenIDF(path string) (*idf.Report, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return a.AnalyzeIDFText(string(content))
}

func (a *App) OpenInputFile() (*InputFileResult, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   "Open EnergyPlus input",
		Filters: inputFileFilters(),
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return &InputFileResult{Canceled: true}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &InputFileResult{
		Path:     path,
		Filename: filepath.Base(path),
		Text:     string(content),
	}, nil
}

func (a *App) AnalyzeMultiIDFSummary(runID string) (*MultiSummaryResult, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("desktop runtime is not ready")
	}
	paths, err := wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   "Open EnergyPlus inputs for Multi-IDF Summary",
		Filters: inputFileFilters(),
	})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return &MultiSummaryResult{Canceled: true, RunID: runID}, nil
	}

	return analyzeMultiSummaryPaths(paths, runID, func(progress MultiSummaryProgress) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "idfAnalyzer:multiSummaryProgress", progress)
		}
	}), nil
}

func (a *App) SaveIDF(path string, text string) error {
	return os.WriteFile(path, []byte(text), 0o644)
}

func (a *App) SaveInputFile(path string, text string) (*SaveFileResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return a.SaveInputFileAs(text, "")
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return nil, err
	}
	return &SaveFileResult{Path: path, Filename: filepath.Base(path)}, nil
}

func (a *App) SaveInputFileAs(text string, suggestedFilename string) (*SaveFileResult, error) {
	suggestedFilename = strings.TrimSpace(suggestedFilename)
	if suggestedFilename == "" {
		suggestedFilename = "model.idf"
	}
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Save EnergyPlus input",
		DefaultFilename: suggestedFilename,
		Filters:         inputFileFilters(),
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return &SaveFileResult{Canceled: true}, nil
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return nil, err
	}
	return &SaveFileResult{Path: path, Filename: filepath.Base(path)}, nil
}

func (a *App) UpdateFieldText(text string, objectIndex int, fieldIndex int, value string) (*TextEditResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	updated, err := idf.UpdateField(doc, objectIndex, fieldIndex, value)
	if err != nil {
		return nil, err
	}
	resultText := writeDocumentInOriginalFormat(updated, model)
	report := idf.Analyze(updated)
	return &TextEditResult{
		Text:    resultText,
		Format:  string(model.Format),
		Version: model.Version.Raw,
		Report:  &report,
	}, nil
}

func (a *App) SuggestFieldValuesText(text string, objectIndex int, fieldIndex int) ([]idf.FieldValueSuggestion, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	return idf.SuggestFieldValues(doc, objectIndex, fieldIndex), nil
}

func (a *App) RemoveUnusedObjectsText(text string) (*TextEditResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	updated, _ := idf.RemoveUnusedObjects(doc)
	resultText := writeDocumentInOriginalFormat(updated, model)
	report := idf.Analyze(updated)
	return &TextEditResult{
		Text:    resultText,
		Format:  string(model.Format),
		Version: model.Version.Raw,
		Report:  &report,
	}, nil
}

func (a *App) ExportSummaryText(text string, format string) (*SummaryExportResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	summary := idf.AnalyzeSummary(epinput.ToIDFDocument(model))

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		output, err := idf.ExportSummaryJSON(summary)
		if err != nil {
			return nil, err
		}
		return &SummaryExportResult{
			Text:     output,
			Format:   "json",
			Filename: "summary.json",
			MIME:     "application/json",
		}, nil
	case "csv":
		output, err := idf.ExportSummaryCSV(summary)
		if err != nil {
			return nil, err
		}
		return &SummaryExportResult{
			Text:     output,
			Format:   "csv",
			Filename: "summary.csv",
			MIME:     "text/csv",
		}, nil
	default:
		return nil, fmt.Errorf("unsupported summary export format %q; use json or csv", format)
	}
}

func (a *App) ScanCleanupText(text string, path string, filename string) (*CleanupFileResult, error) {
	return cleanupFileResultFromText(text, path, filename)
}

func (a *App) PreviewCleanupText(text string, ruleIDs []string, excludedCandidateKeys []string) (*CleanupPreviewResult, error) {
	return previewCleanupText(text, ruleIDs, excludedCandidateKeys)
}

func (a *App) SaveCleanupAs(text string, suggestedFilename string, ruleIDs []string, excludedCandidateKeys []string) (*CleanupApplyResult, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("desktop runtime is not ready")
	}
	preview, err := previewCleanupText(text, ruleIDs, excludedCandidateKeys)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(suggestedFilename) == "" {
		suggestedFilename = "cleaned.idf"
	}
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Save cleaned EnergyPlus input as",
		DefaultFilename: suggestedFilename,
		Filters:         inputFileFilters(),
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return &CleanupApplyResult{Canceled: true, RemovedCount: preview.RemovedCount}, nil
	}
	if err := os.WriteFile(path, []byte(preview.Text), 0o644); err != nil {
		return nil, err
	}
	return &CleanupApplyResult{Path: path, Filename: filepath.Base(path), Text: preview.Text, RemovedCount: preview.RemovedCount}, nil
}

func (a *App) SaveCleanupToFile(path string, text string, ruleIDs []string, excludedCandidateKeys []string) (*CleanupApplyResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("cleanup save requires an original file path")
	}
	preview, err := previewCleanupText(text, ruleIDs, excludedCandidateKeys)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(preview.Text), 0o644); err != nil {
		return nil, err
	}
	return &CleanupApplyResult{Path: path, Filename: filepath.Base(path), Text: preview.Text, RemovedCount: preview.RemovedCount}, nil
}

func (a *App) PreviewProfileApplyText(text string, request idf.ProfileApplyRequest) (*idf.ProfileApplyPreview, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	preview := idf.PreviewApplyProfile(doc, request)
	return &preview, nil
}

func (a *App) ApplyProfileText(text string, request idf.ProfileApplyRequest) (*ProfileApplyTextResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	updated, preview := idf.ApplyProfile(doc, request)
	resultText := writeDocumentInOriginalFormat(updated, model)
	updatedModel, err := epinput.Parse("", []byte(resultText))
	if err != nil {
		return nil, err
	}
	updatedDoc := epinput.ToIDFDocument(updatedModel)
	report := idf.Analyze(updatedDoc)
	epjsonText, err := epinput.Write(updatedModel, epinput.FormatEPJSON)
	if err != nil {
		return nil, err
	}
	return &ProfileApplyTextResult{
		Text:    resultText,
		Format:  string(updatedModel.Format),
		Version: updatedModel.Version.Raw,
		Model:   updatedModel,
		EPJSON:  epjsonText,
		Report:  &report,
		Preview: preview,
	}, nil
}

func (a *App) PreviewHVACApplyText(text string, request idf.HVACApplyRequest) (*idf.HVACApplyPreview, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	preview := idf.PreviewApplyHVAC(doc, request)
	return &preview, nil
}

func (a *App) ApplyHVACText(text string, request idf.HVACApplyRequest) (*HVACApplyTextResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	updated, preview := idf.ApplyHVAC(doc, request)
	if !preview.CanApply {
		return nil, fmt.Errorf("HVAC preview has blocking warnings")
	}
	resultText := writeDocumentInOriginalFormat(updated, model)
	updatedModel, err := epinput.Parse("", []byte(resultText))
	if err != nil {
		return nil, err
	}
	updatedDoc := epinput.ToIDFDocument(updatedModel)
	report := idf.Analyze(updatedDoc)
	epjsonText, err := epinput.Write(updatedModel, epinput.FormatEPJSON)
	if err != nil {
		return nil, err
	}
	return &HVACApplyTextResult{
		Text:    resultText,
		Format:  string(updatedModel.Format),
		Version: updatedModel.Version.Raw,
		Model:   updatedModel,
		EPJSON:  epjsonText,
		Report:  &report,
		Preview: preview,
	}, nil
}

func (a *App) PreviewOutputApplyText(text string, request idf.OutputApplyRequest) (*idf.OutputApplyPreview, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	preview := idf.PreviewApplyOutput(doc, request)
	return &preview, nil
}

func (a *App) ApplyOutputText(text string, request idf.OutputApplyRequest) (*OutputApplyTextResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	updated, preview := idf.ApplyOutput(doc, request)
	if !preview.CanApply {
		return nil, fmt.Errorf("output preview has blocking warnings")
	}
	resultText := writeDocumentInOriginalFormat(updated, model)
	updatedModel, err := epinput.Parse("", []byte(resultText))
	if err != nil {
		return nil, err
	}
	updatedDoc := epinput.ToIDFDocument(updatedModel)
	report := idf.Analyze(updatedDoc)
	epjsonText, err := epinput.Write(updatedModel, epinput.FormatEPJSON)
	if err != nil {
		return nil, err
	}
	return &OutputApplyTextResult{
		Text:    resultText,
		Format:  string(updatedModel.Format),
		Version: updatedModel.Version.Raw,
		Model:   updatedModel,
		EPJSON:  epjsonText,
		Report:  &report,
		Preview: preview,
	}, nil
}

func (a *App) GetSummaryMetricGuides() []idf.SummaryGuide {
	return idf.SummaryGuides()
}

func analyzeMultiSummaryPaths(paths []string, runID string, emit func(MultiSummaryProgress)) *MultiSummaryResult {
	result := &MultiSummaryResult{
		RunID:       runID,
		Total:       len(paths),
		Completed:   0,
		Concurrency: multiSummaryConcurrency(len(paths)),
		Metrics:     multiSummaryMetrics(idf.AnalyzeSummary(idf.Document{})),
		Files:       make([]MultiSummaryFile, len(paths)),
	}
	if len(paths) == 0 {
		return result
	}

	type job struct {
		index int
		path  string
	}

	jobs := make(chan job)
	results := make(chan MultiSummaryFile)
	var wg sync.WaitGroup
	for worker := 0; worker < result.Concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				results <- analyzeMultiSummaryFile(item.index, item.path)
			}
		}()
	}

	go func() {
		for index, path := range paths {
			jobs <- job{index: index, path: path}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	for file := range results {
		result.Files[file.Index] = file
		result.Completed++
		if file.Status == "ok" {
			result.Succeeded++
		} else {
			result.Failed++
		}
		if emit != nil {
			emit(MultiSummaryProgress{
				RunID:     runID,
				Total:     result.Total,
				Completed: result.Completed,
				Succeeded: result.Succeeded,
				Failed:    result.Failed,
				File:      file,
			})
		}
	}

	ensureUniqueMultiSummaryLabels(result.Files)
	return result
}

func analyzeMultiSummaryFile(index int, path string) MultiSummaryFile {
	file := MultiSummaryFile{
		Index:    index,
		Path:     path,
		Filename: filepath.Base(path),
		Label:    filepath.Base(path),
		Status:   "ok",
	}

	content, err := os.ReadFile(path)
	if err != nil {
		file.Status = "error"
		file.Error = err.Error()
		return file
	}

	model, err := epinput.Parse(path, content)
	if err != nil {
		file.Status = "error"
		file.Error = err.Error()
		return file
	}

	doc := epinput.ToIDFDocument(model)
	summary := idf.AnalyzeSummary(doc)
	file.Format = string(model.Format)
	file.Version = model.Version.Raw
	file.ObjectCount = len(doc.Objects)
	file.MetricValues = map[string]MultiSummaryValue{}
	for _, category := range summary.Categories {
		for _, metric := range category.Metrics {
			file.MetricValues[metric.ID] = MultiSummaryValue{
				DisplayValue: metric.DisplayValue,
				Status:       metric.Status,
			}
		}
	}
	if buildingName := strings.TrimSpace(file.MetricValues["building_name"].DisplayValue); buildingName != "" && !strings.EqualFold(buildingName, "N/A") {
		file.Label = buildingName
	}
	return file
}

func multiSummaryMetrics(summary idf.SummaryReport) []MultiSummaryMetric {
	names := idf.SummaryCSVMetricNames(summary)
	metrics := make([]MultiSummaryMetric, 0, summary.MetricCount)
	for _, category := range summary.Categories {
		for _, metric := range category.Metrics {
			metrics = append(metrics, MultiSummaryMetric{
				ID:       metric.ID,
				Name:     metric.Name,
				Category: metric.Category,
				Unit:     metric.Unit,
				CSVName:  names[metric.ID],
			})
		}
	}
	return metrics
}

func multiSummaryConcurrency(count int) int {
	if count <= 0 {
		return 0
	}
	limit := runtime.NumCPU()
	if limit < 1 {
		limit = 1
	}
	if limit > 8 {
		limit = 8
	}
	if limit > count {
		limit = count
	}
	return limit
}

func ensureUniqueMultiSummaryLabels(files []MultiSummaryFile) {
	seen := map[string]int{}
	for index := range files {
		label := strings.TrimSpace(files[index].Label)
		if label == "" {
			label = files[index].Filename
		}
		if label == "" {
			label = fmt.Sprintf("File %d", files[index].Index+1)
		}
		seen[label]++
		if seen[label] > 1 {
			label = fmt.Sprintf("%s (%d)", label, seen[label])
		}
		files[index].Label = label
	}
}

func cleanupFileResultFromText(text string, path string, filename string) (*CleanupFileResult, error) {
	path = strings.TrimSpace(path)
	filename = strings.TrimSpace(filename)
	parseName := path
	if parseName == "" {
		parseName = filename
	}
	model, err := epinput.Parse(parseName, []byte(text))
	if err != nil {
		return nil, err
	}
	if filename == "" && path != "" {
		filename = filepath.Base(path)
	}
	if filename == "" {
		filename = "Current input"
	}
	doc := epinput.ToIDFDocument(model)
	return &CleanupFileResult{
		Path:     path,
		Filename: filename,
		Text:     text,
		Format:   string(model.Format),
		Version:  model.Version.Raw,
		Scan:     idf.ScanCleanup(doc),
	}, nil
}

func previewCleanupText(text string, ruleIDs []string, excludedCandidateKeys []string) (*CleanupPreviewResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	updated, preview := idf.ApplyCleanup(doc, ruleIDs, excludedCandidateKeys)
	output := text
	if preview.RemovedCount > 0 || idf.CleanupCompacts(ruleIDs) {
		output = cleanupOutputText(updated, model)
	}
	return &CleanupPreviewResult{
		Text:              output,
		RemovedCandidates: preview.RemovedCandidates,
		RemovedCount:      preview.RemovedCount,
		ObjectCount:       len(updated.Objects),
	}, nil
}

func cleanupOutputText(doc idf.Document, original *epinput.Model) string {
	if original != nil && original.Format == epinput.FormatEPJSON {
		return writeDocumentInOriginalFormat(doc, original)
	}
	return doc.String()
}

func (a *App) GetSettings() (*SettingsResult, error) {
	path, settings, err := loadAppSettings()
	if err != nil {
		return nil, err
	}
	return &SettingsResult{Path: path, Settings: settings}, nil
}

func (a *App) SaveSettings(settings AppSettings) (*SettingsResult, error) {
	settings = normalizeAppSettings(settings)
	path, err := appSettingsPath()
	if err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		return nil, err
	}
	return &SettingsResult{Path: path, Settings: settings}, nil
}

func writeDocumentInOriginalFormat(doc idf.Document, original *epinput.Model) string {
	if original != nil && original.Format == epinput.FormatEPJSON {
		model := epinput.FromIDFDocument(doc, epinput.FormatEPJSON)
		output, err := epinput.Write(model, epinput.FormatEPJSON)
		if err == nil {
			return output
		}
	}
	return doc.String()
}

func inputFileFilters() []wailsruntime.FileFilter {
	return []wailsruntime.FileFilter{
		{DisplayName: "EnergyPlus input", Pattern: "*.idf;*.imf;*.epjson;*.json;*.txt"},
		{DisplayName: "All files", Pattern: "*.*"},
	}
}

func defaultAppSettings() AppSettings {
	return AppSettings{
		Version: 1,
		Appearance: AppearanceSettings{
			Theme:            "system",
			Language:         "en",
			AnalysisTabOrder: []string{"summary", "profile", "hvac", "output", "simulation", "diagnose", "geometry"},
			Geometry: GeometryAppearanceSettings{
				Background: "#f7fafc",
				Zone:       "#b8d7b0",
				Wall:       "#7b9cbc",
				Roof:       "#b8b0a1",
				Window:     "#3fb6d4",
				Selected:   "#f0a202",
			},
		},
		Behavior: BehaviorSettings{
			AutoAnalyzeDelayMS: 900,
		},
		Interaction: InteractionSettings{
			SyncRawTextPosition: true,
			GeometrySyncLocate:  true,
			Shortcuts: map[string]string{
				"save":           "Ctrl+S",
				"open":           "Ctrl+O",
				"undoView":       "Ctrl+Z",
				"redoView":       "Ctrl+Y, Ctrl+Shift+Z",
				"jumpDefinition": "F12",
				"jumpReferences": "Shift+F12",
				"inputText":      "Ctrl+1",
				"inputJson":      "Ctrl+2",
				"inputTable":     "Ctrl+3",
				"tabSummary":     "Ctrl+Alt+1",
				"tabProfile":     "Ctrl+Alt+2",
				"tabHVAC":        "Ctrl+Alt+3",
				"tabOutput":      "Ctrl+Alt+4",
				"tabSimulation":  "Ctrl+Alt+5",
				"tabDiagnose":    "Ctrl+Alt+6",
				"tabGeometry":    "Ctrl+Alt+7",
			},
		},
		Profile:    idf.DefaultProfileAnalysisSettings(),
		Simulation: defaultSimulationSettings(),
	}
}

func normalizeAppSettings(settings AppSettings) AppSettings {
	defaults := defaultAppSettings()
	if settings.Version == 0 {
		settings.Version = defaults.Version
	}
	switch strings.ToLower(strings.TrimSpace(settings.Appearance.Theme)) {
	case "light", "dark", "system":
		settings.Appearance.Theme = strings.ToLower(strings.TrimSpace(settings.Appearance.Theme))
	default:
		settings.Appearance.Theme = defaults.Appearance.Theme
	}
	settings.Appearance.Language = normalizeAppLanguage(settings.Appearance.Language, defaults.Appearance.Language)
	settings.Appearance.AnalysisTabOrder = normalizeAnalysisTabOrder(settings.Appearance.AnalysisTabOrder, defaults.Appearance.AnalysisTabOrder)
	settings.Appearance.Geometry.Background = normalizeHexColor(settings.Appearance.Geometry.Background, defaults.Appearance.Geometry.Background)
	settings.Appearance.Geometry.Zone = normalizeHexColor(settings.Appearance.Geometry.Zone, defaults.Appearance.Geometry.Zone)
	settings.Appearance.Geometry.Wall = normalizeHexColor(settings.Appearance.Geometry.Wall, defaults.Appearance.Geometry.Wall)
	settings.Appearance.Geometry.Roof = normalizeHexColor(settings.Appearance.Geometry.Roof, defaults.Appearance.Geometry.Roof)
	settings.Appearance.Geometry.Window = normalizeHexColor(settings.Appearance.Geometry.Window, defaults.Appearance.Geometry.Window)
	settings.Appearance.Geometry.Selected = normalizeHexColor(settings.Appearance.Geometry.Selected, defaults.Appearance.Geometry.Selected)
	if settings.Behavior.AutoAnalyzeDelayMS == 0 {
		settings.Behavior.AutoAnalyzeDelayMS = defaults.Behavior.AutoAnalyzeDelayMS
	}
	if settings.Behavior.AutoAnalyzeDelayMS < 150 {
		settings.Behavior.AutoAnalyzeDelayMS = 150
	}
	if settings.Behavior.AutoAnalyzeDelayMS > 5000 {
		settings.Behavior.AutoAnalyzeDelayMS = 5000
	}
	settings.Interaction.Shortcuts = normalizeShortcutSettings(settings.Interaction.Shortcuts, defaults.Interaction.Shortcuts)
	settings.Profile = normalizeProfileSettings(settings.Profile, defaults.Profile)
	settings.Simulation = normalizeSimulationSettings(settings.Simulation, defaults.Simulation)
	return settings
}

func normalizeShortcutSettings(values map[string]string, defaults map[string]string) map[string]string {
	out := make(map[string]string, len(defaults))
	for key, fallback := range defaults {
		value := strings.TrimSpace(values[key])
		if value == "" {
			value = fallback
		}
		out[key] = value
	}
	return out
}

func normalizeAppLanguage(value string, fallback string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if index := strings.IndexAny(normalized, "-_"); index >= 0 {
		normalized = normalized[:index]
	}
	switch normalized {
	case "en", "ko", "ja", "hi", "es", "fr":
		return normalized
	case "kr":
		return "ko"
	case "jp":
		return "ja"
	default:
		return fallback
	}
}

func normalizeAnalysisTabOrder(values []string, fallback []string) []string {
	allowed := map[string]bool{
		"summary":    true,
		"profile":    true,
		"hvac":       true,
		"output":     true,
		"simulation": true,
		"diagnose":   true,
		"geometry":   true,
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(allowed))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if !allowed[normalized] || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	source := fallback
	if len(source) == 0 {
		source = []string{"summary", "profile", "hvac", "output", "simulation", "diagnose", "geometry"}
	}
	for _, value := range source {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if allowed[normalized] && !seen[normalized] {
			seen[normalized] = true
			out = append(out, normalized)
		}
	}
	return out
}

func normalizeProfileSettings(settings idf.ProfileAnalysisSettings, defaults idf.ProfileAnalysisSettings) idf.ProfileAnalysisSettings {
	allowedDimensions := map[string]bool{
		idf.ProfileDimensionOccupancy:    true,
		idf.ProfileDimensionLighting:     true,
		idf.ProfileDimensionEquipment:    true,
		idf.ProfileDimensionInfiltration: true,
		idf.ProfileDimensionVentilation:  true,
		idf.ProfileDimensionOutdoorAir:   true,
	}
	settings.EnabledDimensions = normalizeProfileChoiceList(settings.EnabledDimensions, defaults.EnabledDimensions, allowedDimensions)
	if settings.DisplayMetrics == nil {
		settings.DisplayMetrics = defaults.DisplayMetrics
	}
	if settings.GroupingMetrics == nil {
		settings.GroupingMetrics = defaults.GroupingMetrics
	}
	for dimension, metric := range defaults.DisplayMetrics {
		if strings.TrimSpace(settings.DisplayMetrics[dimension]) == "" {
			settings.DisplayMetrics[dimension] = metric
		}
	}
	for dimension, metric := range defaults.GroupingMetrics {
		if strings.TrimSpace(settings.GroupingMetrics[dimension]) == "" {
			settings.GroupingMetrics[dimension] = metric
		}
	}
	if settings.NumericTolerance <= 0 {
		settings.NumericTolerance = defaults.NumericTolerance
	}
	settings.ScheduleCompareMode = normalizeProfileChoice(settings.ScheduleCompareMode, []string{"none", "name", "resolved"}, defaults.ScheduleCompareMode)
	settings.GraphMode = normalizeProfileChoice(settings.GraphMode, []string{"multiplier", "actual_value"}, defaults.GraphMode)
	settings.ScheduleSummaryMode = normalizeProfileChoice(settings.ScheduleSummaryMode, []string{
		"representative_day",
		"representative_week",
		"monthly_average",
		"hourly_average_by_daytype",
		"load_duration",
		"annual_heatmap",
	}, defaults.ScheduleSummaryMode)
	settings.ApplyBehavior.DefaultMode = normalizeProfileChoice(settings.ApplyBehavior.DefaultMode, []string{"clone", "shared"}, defaults.ApplyBehavior.DefaultMode)
	if strings.TrimSpace(settings.ApplyBehavior.NameSuffix) == "" {
		settings.ApplyBehavior.NameSuffix = defaults.ApplyBehavior.NameSuffix
	}
	settings.ApplyBehavior.ReplaceExistingPolicy = normalizeProfileChoice(settings.ApplyBehavior.ReplaceExistingPolicy, []string{"replace", "keep", "duplicate"}, defaults.ApplyBehavior.ReplaceExistingPolicy)
	return settings
}

func normalizeProfileChoice(value string, allowed []string, fallback string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, choice := range allowed {
		if normalized == choice {
			return normalized
		}
	}
	return fallback
}

func normalizeProfileChoiceList(values []string, fallback []string, allowed map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if !allowed[normalized] || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
}

func normalizeHexColor(value string, fallback string) string {
	color := strings.ToLower(strings.TrimSpace(value))
	if color == "" {
		return fallback
	}
	if !strings.HasPrefix(color, "#") {
		color = "#" + color
	}
	if len(color) == 4 && isHexColor(color[1:]) {
		return "#" + string(color[1]) + string(color[1]) + string(color[2]) + string(color[2]) + string(color[3]) + string(color[3])
	}
	if len(color) == 7 && isHexColor(color[1:]) {
		return color
	}
	return fallback
}

func isHexColor(value string) bool {
	for _, char := range value {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F') {
			continue
		}
		return false
	}
	return len(value) > 0
}

func loadAppSettings() (string, AppSettings, error) {
	path, err := appSettingsPath()
	if err != nil {
		return "", AppSettings{}, err
	}
	settings := defaultAppSettings()
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			payload, marshalErr := json.MarshalIndent(settings, "", "  ")
			if marshalErr != nil {
				return "", AppSettings{}, marshalErr
			}
			if writeErr := os.WriteFile(path, append(payload, '\n'), 0o644); writeErr != nil {
				return "", AppSettings{}, writeErr
			}
			return path, settings, nil
		}
		return "", AppSettings{}, err
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return path, settings, nil
	}
	if err := json.Unmarshal(content, &settings); err != nil {
		return "", AppSettings{}, err
	}
	settings = normalizeAppSettings(settings)
	return path, settings, nil
}

func appSettingsPath() (string, error) {
	root := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if root == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		root = configDir
	}
	dir := filepath.Join(root, "IDF Analyzer")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}
