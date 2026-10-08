package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/epinput"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx                            context.Context
	analysisCache                  *AnalysisCache
	analysisCacheOnce              sync.Once
	simulationWorkspaceCache       simulationWorkspaceCache
	simulationProgressCache        simulationProgressCache
	storageMu                      sync.RWMutex
	storageReferencesMu            sync.Mutex
	storageBatchDirectories        []string
	storageBatchInputDirectories   []string
	storageLoadedDirectory         string
	storageLoadedDirectories       []string
	storageSingleDirectory         string
	storageLoadedInputDirectory    string
	storageSingleInputDirectory    string
	storageSingleSourceDirectory   string
	storageSingleSourceDirectories []string
	storageInputDirectory          string
	storageInputDirectories        []string
	storageInputInitialized        bool
	storageLastCleanup             *StorageCleanupSummary
	storageInstanceError           error
	simulationResultHTTPAvailable  atomic.Bool
}

type TextEditResult struct {
	Text     string                      `json:"text"`
	Format   string                      `json:"format"`
	Version  string                      `json:"version,omitempty"`
	Semantic *idf.SemanticYAMLProjection `json:"semantic,omitempty"`
	Report   *idf.Report                 `json:"report"`
	Warnings []string                    `json:"warnings,omitempty"`
}

type MetricsExportResult struct {
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
	Text     string                      `json:"text"`
	Format   string                      `json:"format,omitempty"`
	Version  string                      `json:"version,omitempty"`
	Model    *epinput.Model              `json:"model,omitempty"`
	EPJSON   string                      `json:"epjson,omitempty"`
	Semantic *idf.SemanticYAMLProjection `json:"semantic,omitempty"`
	Report   *idf.Report                 `json:"report"`
	Preview  idf.ProfileApplyPreview     `json:"preview"`
}

type HVACApplyTextResult struct {
	Text     string                      `json:"text"`
	Format   string                      `json:"format,omitempty"`
	Version  string                      `json:"version,omitempty"`
	Model    *epinput.Model              `json:"model,omitempty"`
	EPJSON   string                      `json:"epjson,omitempty"`
	Semantic *idf.SemanticYAMLProjection `json:"semantic,omitempty"`
	Report   *idf.Report                 `json:"report"`
	Preview  idf.HVACApplyPreview        `json:"preview"`
}

type OutputApplyTextResult struct {
	Text     string                      `json:"text"`
	Format   string                      `json:"format,omitempty"`
	Version  string                      `json:"version,omitempty"`
	Model    *epinput.Model              `json:"model,omitempty"`
	EPJSON   string                      `json:"epjson,omitempty"`
	Semantic *idf.SemanticYAMLProjection `json:"semantic,omitempty"`
	Report   *idf.Report                 `json:"report"`
	Preview  idf.OutputApplyPreview      `json:"preview"`
}

type ModelPatchResult = InputAnalysisResult

func NewApp() *App {
	return &App{analysisCache: NewAnalysisCache(defaultAnalysisCacheEntries)}
}

func (a *App) GetAppInfo() AppInfo {
	return currentAppInfo()
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.storageInstanceError = simulation.RegisterStorageInstance()
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

	textHash := analysisTextHash(resultText)
	result := &ModelPatchResult{
		Text:        resultText,
		AnalysisKey: textHash,
		Format:      string(model.Format),
		Version:     model.Version.Raw,
		Model:       model,
		EPJSON:      epjsonText,
		Semantic:    semanticProjectionForModelDoc(model, doc),
		Report:      &report,
		Timing:      &AnalysisTiming{Mode: "full", CacheHit: false},
	}
	if a.analysisCache != nil {
		a.analysisCache.Store(analysisCacheKey{
			TextHash:          textHash,
			Format:            string(model.Format),
			EnergyPlusVersion: model.Version.Raw,
			AnalyzerVersion:   currentAppInfo().Version,
			Mode:              "full",
			SettingsHash:      defaultAnalysisSettingsHash,
		}, result)
	}
	return result, nil
}

func (a *App) OpenIDF(path string) (*idf.Report, error) {
	if err := a.ensureStorageInstance(); err != nil {
		return nil, err
	}
	a.storageMu.RLock()
	defer a.storageMu.RUnlock()
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := a.setStorageInputPath(path); err != nil {
		return nil, err
	}
	return a.AnalyzeIDFText(string(content))
}

func (a *App) OpenInputFile() (*InputFileResult, error) {
	if err := a.ensureStorageInstance(); err != nil {
		return nil, err
	}
	a.storageMu.RLock()
	defer a.storageMu.RUnlock()
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
	if err := a.setStorageInputPath(path); err != nil {
		return nil, err
	}
	return &InputFileResult{
		Path:     path,
		Filename: filepath.Base(path),
		Text:     string(content),
	}, nil
}

func (a *App) SaveIDF(path string, text string) error {
	if err := a.ensureStorageInstance(); err != nil {
		return err
	}
	a.storageMu.RLock()
	defer a.storageMu.RUnlock()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return err
	}
	return a.setStorageInputPath(path)
}

func (a *App) SaveInputFile(path string, text string) (*SaveFileResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return a.SaveInputFileAs(text, "")
	}
	if err := a.ensureStorageInstance(); err != nil {
		return nil, err
	}
	a.storageMu.RLock()
	defer a.storageMu.RUnlock()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return nil, err
	}
	if err := a.setStorageInputPath(path); err != nil {
		return nil, err
	}
	return &SaveFileResult{Path: path, Filename: filepath.Base(path)}, nil
}

func (a *App) SaveInputFileAs(text string, suggestedFilename string) (*SaveFileResult, error) {
	if err := a.ensureStorageInstance(); err != nil {
		return nil, err
	}
	a.storageMu.RLock()
	defer a.storageMu.RUnlock()
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
	if err := a.setStorageInputPath(path); err != nil {
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
		Text:     resultText,
		Format:   string(model.Format),
		Version:  model.Version.Raw,
		Semantic: semanticProjectionForModelDoc(model, updated),
		Report:   &report,
	}, nil
}

func (a *App) ApplySemanticDuplicateNameFixText(text string) (*TextEditResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	updated, fixes := idf.ApplySemanticDuplicateNameFixes(doc)
	resultText := writeDocumentInOriginalFormat(updated, model)
	report := idf.Analyze(updated)
	warnings := make([]string, 0, len(fixes))
	for _, fix := range fixes {
		warnings = append(warnings, fmt.Sprintf("Renamed %s #%d from %q to %q.", fix.ObjectType, fix.ObjectIndex, fix.Before, fix.After))
	}
	return &TextEditResult{
		Text:     resultText,
		Format:   string(model.Format),
		Version:  model.Version.Raw,
		Semantic: semanticProjectionForModelDoc(model, updated),
		Report:   &report,
		Warnings: warnings,
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
		Text:     resultText,
		Format:   string(model.Format),
		Version:  model.Version.Raw,
		Semantic: semanticProjectionForModelDoc(model, updated),
		Report:   &report,
	}, nil
}

func (a *App) ExportMetricsText(text string, format string) (*MetricsExportResult, error) {
	_, doc, err := parseInputDocument(text)
	if err != nil {
		return nil, err
	}
	metrics := idf.AnalyzeMetrics(doc)

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		output, err := idf.ExportMetricsJSON(metrics)
		if err != nil {
			return nil, err
		}
		return &MetricsExportResult{
			Text:     output,
			Format:   "json",
			Filename: "metrics.json",
			MIME:     "application/json",
		}, nil
	case "csv":
		output, err := idf.ExportMetricsCSV(metrics)
		if err != nil {
			return nil, err
		}
		return &MetricsExportResult{
			Text:     output,
			Format:   "csv",
			Filename: "metrics.csv",
			MIME:     "text/csv",
		}, nil
	default:
		return nil, fmt.Errorf("unsupported metrics export format %q; use json or csv", format)
	}
}

// ExportSummaryText is retained for older Wails clients. New clients use ExportMetricsText.
func (a *App) ExportSummaryText(text string, format string) (*MetricsExportResult, error) {
	return a.ExportMetricsText(text, format)
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
	if err := a.ensureStorageInstance(); err != nil {
		return nil, err
	}
	a.storageMu.RLock()
	defer a.storageMu.RUnlock()
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
	if err := a.setStorageInputPath(path); err != nil {
		return nil, err
	}
	return &CleanupApplyResult{Path: path, Filename: filepath.Base(path), Text: preview.Text, RemovedCount: preview.RemovedCount}, nil
}

func (a *App) SaveCleanupToFile(path string, text string, ruleIDs []string, excludedCandidateKeys []string) (*CleanupApplyResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("cleanup save requires an original file path")
	}
	if err := a.ensureStorageInstance(); err != nil {
		return nil, err
	}
	a.storageMu.RLock()
	defer a.storageMu.RUnlock()
	preview, err := previewCleanupText(text, ruleIDs, excludedCandidateKeys)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(preview.Text), 0o644); err != nil {
		return nil, err
	}
	if err := a.setStorageInputPath(path); err != nil {
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
		Text:     resultText,
		Format:   string(updatedModel.Format),
		Version:  updatedModel.Version.Raw,
		Model:    updatedModel,
		EPJSON:   epjsonText,
		Semantic: semanticProjectionForModelDoc(updatedModel, updatedDoc),
		Report:   &report,
		Preview:  preview,
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
		Text:     resultText,
		Format:   string(updatedModel.Format),
		Version:  updatedModel.Version.Raw,
		Model:    updatedModel,
		EPJSON:   epjsonText,
		Semantic: semanticProjectionForModelDoc(updatedModel, updatedDoc),
		Report:   &report,
		Preview:  preview,
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
	resultText := writeOutputDocumentInOriginalFormat(text, updated, model)
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
		Text:     resultText,
		Format:   string(updatedModel.Format),
		Version:  updatedModel.Version.Raw,
		Model:    updatedModel,
		EPJSON:   epjsonText,
		Semantic: semanticProjectionForModelDoc(updatedModel, updatedDoc),
		Report:   &report,
		Preview:  preview,
	}, nil
}

func (a *App) GetMetricGuides() []idf.MetricGuide {
	return idf.MetricGuides()
}

// GetSummaryMetricGuides is retained for older Wails clients. New clients use GetMetricGuides.
func (a *App) GetSummaryMetricGuides() []idf.MetricGuide {
	return a.GetMetricGuides()
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

func writeDocumentInOriginalFormat(doc idf.Document, original *epinput.Model) string {
	return epinput.WriteDocumentLikeOriginal(doc, original)
}

func writeOutputDocumentInOriginalFormat(originalText string, doc idf.Document, original *epinput.Model) string {
	if original != nil && original.Format == epinput.FormatEPJSON {
		return writeDocumentInOriginalFormat(doc, original)
	}
	return replaceOutputManagementObjects(originalText, doc)
}

func replaceOutputManagementObjects(originalText string, doc idf.Document) string {
	lines := strings.Split(strings.ReplaceAll(originalText, "\r\n", "\n"), "\n")
	remove := make([]bool, len(lines))
	for _, span := range idfObjectSpans(lines) {
		if !isOutputManagementTypeName(span.objectType) {
			continue
		}
		for index := span.startLine; index <= span.endLine && index < len(remove); index++ {
			if index >= 0 {
				remove[index] = true
			}
		}
	}
	kept := make([]string, 0, len(lines))
	for index, line := range lines {
		if !remove[index] {
			kept = append(kept, line)
		}
	}
	output := outputManagementDocument(doc).String()
	result := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if strings.TrimSpace(output) == "" {
		return result + "\n"
	}
	return result + "\n\n!- Managed output requests\n\n" + output + "\n"
}

type idfObjectSpan struct {
	objectType string
	startLine  int
	endLine    int
}

func idfObjectSpans(lines []string) []idfObjectSpan {
	var spans []idfObjectSpan
	var token strings.Builder
	inObject := false
	startLine := 0
	pendingStartLine := 0
	for lineIndex, rawLine := range lines {
		code, _, _ := strings.Cut(strings.TrimRight(rawLine, "\r"), "!")
		if !inObject && strings.TrimSpace(code) == "" {
			token.Reset()
			continue
		}
		if !inObject && strings.TrimSpace(token.String()) == "" {
			token.Reset()
			pendingStartLine = lineIndex
		}
		for _, r := range code {
			if r != ',' && r != ';' {
				token.WriteRune(r)
				continue
			}
			value := strings.TrimSpace(token.String())
			token.Reset()
			if !inObject {
				if value == "" {
					continue
				}
				startLine = pendingStartLine
				spans = append(spans, idfObjectSpan{objectType: value, startLine: startLine, endLine: lineIndex})
				inObject = true
			}
			if r == ';' && inObject {
				spans[len(spans)-1].endLine = lineIndex
				inObject = false
			}
		}
	}
	return spans
}

func outputManagementDocument(doc idf.Document) idf.Document {
	out := idf.Document{}
	for _, obj := range doc.Objects {
		if !isOutputManagementTypeName(obj.Type) {
			continue
		}
		obj.Index = len(out.Objects)
		out.Objects = append(out.Objects, obj)
	}
	return out
}

func isOutputManagementTypeName(objectType string) bool {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	return strings.HasPrefix(lower, "output:") || strings.HasPrefix(lower, "outputcontrol:")
}

func inputFileFilters() []wailsruntime.FileFilter {
	return []wailsruntime.FileFilter{
		{DisplayName: "EnergyPlus input", Pattern: "*.idf;*.imf;*.epjson;*.json;*.txt"},
		{DisplayName: "All files", Pattern: "*.*"},
	}
}
