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
	RemovedCount int    `json:"removedCount"`
}

type AppSettings struct {
	Version int `json:"version"`
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

func (a *App) ScanCleanupInputFile() (*CleanupFileResult, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("desktop runtime is not ready")
	}
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   "Open EnergyPlus input for cleanup",
		Filters: inputFileFilters(),
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return &CleanupFileResult{Canceled: true}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	result, err := cleanupFileResultFromText(string(content), path)
	if err != nil {
		return nil, err
	}
	return result, nil
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

func (a *App) PreviewCleanupText(text string, ruleIDs []string) (*CleanupPreviewResult, error) {
	return previewCleanupText(text, ruleIDs)
}

func (a *App) ExportCleanupCopy(text string, suggestedFilename string, ruleIDs []string) (*CleanupApplyResult, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("desktop runtime is not ready")
	}
	preview, err := previewCleanupText(text, ruleIDs)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(suggestedFilename) == "" {
		suggestedFilename = "cleaned.idf"
	}
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Export cleaned EnergyPlus input",
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
	return &CleanupApplyResult{Path: path, Filename: filepath.Base(path), RemovedCount: preview.RemovedCount}, nil
}

func (a *App) ApplyCleanupToFile(path string, text string, ruleIDs []string) (*CleanupApplyResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("cleanup apply requires an original file path")
	}
	preview, err := previewCleanupText(text, ruleIDs)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(preview.Text), 0o644); err != nil {
		return nil, err
	}
	return &CleanupApplyResult{Path: path, Filename: filepath.Base(path), RemovedCount: preview.RemovedCount}, nil
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

func cleanupFileResultFromText(text string, path string) (*CleanupFileResult, error) {
	model, err := epinput.Parse(path, []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	return &CleanupFileResult{
		Path:     path,
		Filename: filepath.Base(path),
		Text:     text,
		Format:   string(model.Format),
		Version:  model.Version.Raw,
		Scan:     idf.ScanCleanup(doc),
	}, nil
}

func previewCleanupText(text string, ruleIDs []string) (*CleanupPreviewResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	updated, preview := idf.ApplyCleanup(doc, ruleIDs)
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
	if settings.Version == 0 {
		settings.Version = defaultAppSettings().Version
	}
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
	return AppSettings{Version: 1}
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
	if settings.Version == 0 {
		settings.Version = defaultAppSettings().Version
	}
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
