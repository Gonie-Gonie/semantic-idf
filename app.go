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
	"time"

	"github.com/Gonie-Gonie/idf-analyzer/internal/epinput"
	"github.com/Gonie-Gonie/idf-analyzer/internal/idf"
	"github.com/Gonie-Gonie/idf-analyzer/internal/simulation"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx           context.Context
	analysisCache *AnalysisCache
}

type TextEditResult struct {
	Text     string                      `json:"text"`
	Format   string                      `json:"format"`
	Version  string                      `json:"version,omitempty"`
	Semantic *idf.SemanticYAMLProjection `json:"semantic,omitempty"`
	Report   *idf.Report                 `json:"report"`
	Warnings []string                    `json:"warnings,omitempty"`
}

type InputAnalysisResult struct {
	Text        string                      `json:"text,omitempty"`
	AnalysisKey string                      `json:"analysisKey,omitempty"`
	Format      string                      `json:"format"`
	Version     string                      `json:"version,omitempty"`
	Model       *epinput.Model              `json:"model"`
	EPJSON      string                      `json:"epjson,omitempty"`
	Semantic    *idf.SemanticYAMLProjection `json:"semantic,omitempty"`
	Report      *idf.Report                 `json:"report"`
	Timing      *AnalysisTiming             `json:"timing,omitempty"`
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

type SimulationSettings = simulation.SimulationSettings
type EnergyPlusInstallSetting = simulation.EnergyPlusInstallSetting

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
	return &App{analysisCache: NewAnalysisCache(defaultAnalysisCacheEntries)}
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
	return a.AnalyzeInputQuickText(text)
}

func (a *App) AnalyzeInputQuickText(text string) (*InputAnalysisResult, error) {
	return a.analyzeInputText(text, "quick", false)
}

func (a *App) AnalyzeInputText(text string) (*InputAnalysisResult, error) {
	return a.analyzeInputText(text, "full", true)
}

func (a *App) AnalyzeInputStageText(text string, stage string) (*InputAnalysisResult, error) {
	mode := normalizeAnalysisStage(stage)
	if mode == "" {
		return nil, fmt.Errorf("unsupported analysis stage %q", stage)
	}
	return a.analyzeInputStageText(text, mode)
}

func (a *App) GetCachedAnalysis(textHash string) (*InputAnalysisResult, error) {
	if a.analysisCache == nil || strings.TrimSpace(textHash) == "" {
		return nil, nil
	}
	for _, mode := range []string{"full"} {
		if result, ok := a.analysisCache.LookupTextMode(textHash, mode); ok {
			cached := cloneInputAnalysisResult(result)
			if cached.Timing == nil {
				cached.Timing = &AnalysisTiming{Mode: mode}
			}
			cached.Timing.CacheHit = true
			cached.Timing.TotalMS = 0
			cached.Timing.QueueWaitMS = 0
			return cached, nil
		}
	}
	if assembled := a.cachedCompletedStageAnalysis(textHash); assembled != nil {
		return assembled, nil
	}
	return nil, nil
}

func (a *App) AnalyzeInputDiagnosticsText(text string) ([]idf.Diagnostic, error) {
	_, doc, err := parseInputDocument(text)
	if err != nil {
		return nil, err
	}
	return idf.AnalyzeDiagnostics(doc), nil
}

func (a *App) AnalyzeInputGeometryText(text string) (*idf.GeometryReport, error) {
	_, doc, err := parseInputDocument(text)
	if err != nil {
		return nil, err
	}
	geometry := idf.AnalyzeGeometry(doc)
	return &geometry, nil
}

func (a *App) AnalyzeInputProfileText(text string) (*idf.ProfileReport, error) {
	_, doc, err := parseInputDocument(text)
	if err != nil {
		return nil, err
	}
	profile := idf.AnalyzeProfile(doc)
	return &profile, nil
}

func (a *App) AnalyzeInputHVACText(text string) (*idf.HVACReport, error) {
	_, doc, err := parseInputDocument(text)
	if err != nil {
		return nil, err
	}
	hvac := idf.AnalyzeHVAC(doc)
	return &hvac, nil
}

func (a *App) AnalyzeInputOutputText(text string) (*idf.OutputReport, error) {
	_, doc, err := parseInputDocument(text)
	if err != nil {
		return nil, err
	}
	output := idf.AnalyzeOutput(doc)
	return &output, nil
}

func (a *App) analyzeInputText(text string, mode string, includeEPJSON bool) (*InputAnalysisResult, error) {
	if a.analysisCache == nil {
		a.analysisCache = NewAnalysisCache(defaultAnalysisCacheEntries)
	}
	requestStart := time.Now()
	textHash := analysisTextHash(text)
	if cached, ok := a.analysisCache.LookupTextMode(textHash, mode); ok {
		return cachedAnalysisResult(cached, requestStart, mode), nil
	}

	parseStart := time.Now()
	model, doc, err := parseInputDocument(text)
	if err != nil {
		return nil, err
	}
	parseMS := analysisDurationMS(time.Since(parseStart))
	key := analysisCacheKey{
		TextHash:          textHash,
		Format:            string(model.Format),
		EnergyPlusVersion: model.Version.Raw,
		AnalyzerVersion:   currentAppInfo().Version,
		Mode:              mode,
		SettingsHash:      defaultAnalysisSettingsHash,
	}

	result, cacheHit, queueWait, err := a.analysisCache.GetOrCompute(key, func() (*InputAnalysisResult, error) {
		stageTimer, stageSnapshot := analysisStageRecorder()
		analyzeStart := time.Now()
		var report idf.Report
		switch mode {
		case "quick":
			report = idf.AnalyzeQuickTimed(doc, stageTimer)
		case "overview":
			report = idf.AnalyzeOverviewTimed(doc, stageTimer)
		default:
			report = idf.AnalyzeTimed(doc, stageTimer)
		}
		analyzeMS := analysisDurationMS(time.Since(analyzeStart))

		epjsonStart := time.Now()
		epjsonText := ""
		if includeEPJSON {
			var writeErr error
			epjsonText, writeErr = epinput.Write(model, epinput.FormatEPJSON)
			if writeErr != nil {
				return nil, writeErr
			}
		}
		epjsonMS := analysisDurationMS(time.Since(epjsonStart))

		semanticStart := time.Now()
		semantic := semanticProjectionForModelDoc(model, doc)
		semanticMS := analysisDurationMS(time.Since(semanticStart))

		return &InputAnalysisResult{
			Text:        text,
			AnalysisKey: textHash,
			Format:      string(model.Format),
			Version:     model.Version.Raw,
			Model:       model,
			EPJSON:      epjsonText,
			Semantic:    semantic,
			Report:      &report,
			Timing: &AnalysisTiming{
				Mode:       mode,
				CacheHit:   false,
				TotalMS:    analysisDurationMS(time.Since(requestStart)),
				ParseMS:    parseMS,
				AnalyzeMS:  analyzeMS,
				SemanticMS: semanticMS,
				EPJSONMS:   epjsonMS,
				Stages:     stageSnapshot(),
			},
		}, nil
	})
	if err != nil {
		return nil, err
	}

	return analysisResultForRequest(result, requestStart, mode, cacheHit, queueWait, parseMS), nil
}

func (a *App) analyzeInputStageText(text string, mode string) (*InputAnalysisResult, error) {
	if a.analysisCache == nil {
		a.analysisCache = NewAnalysisCache(defaultAnalysisCacheEntries)
	}
	requestStart := time.Now()
	textHash := analysisTextHash(text)
	if cached, ok := a.analysisCache.LookupTextMode(textHash, mode); ok {
		return cachedAnalysisResult(cached, requestStart, mode), nil
	}

	parseStart := time.Now()
	model, doc, err := parseInputDocument(text)
	if err != nil {
		return nil, err
	}
	parseMS := analysisDurationMS(time.Since(parseStart))
	key := analysisCacheKey{
		TextHash:          textHash,
		Format:            string(model.Format),
		EnergyPlusVersion: model.Version.Raw,
		AnalyzerVersion:   currentAppInfo().Version,
		Mode:              mode,
		SettingsHash:      defaultAnalysisSettingsHash,
	}

	result, cacheHit, queueWait, err := a.analysisCache.GetOrCompute(key, func() (*InputAnalysisResult, error) {
		stageStart := time.Now()
		report := idf.Report{}
		stageName := mode
		index := idf.NewDocumentIndex(doc)
		switch mode {
		case "profile":
			report.Profile = idf.AnalyzeProfileFromIndex(index)
		case "hvac":
			report.HVAC = idf.AnalyzeHVACFromIndex(index)
		case "output":
			report.Output = idf.AnalyzeOutputFromIndex(index)
		case "diagnostics":
			report.Diagnostics = idf.AnalyzeDiagnosticsFromIndex(index)
		case "geometry":
			geometry := idf.AnalyzeGeometryFromIndex(index)
			report.Geometry = geometry
		default:
			return nil, fmt.Errorf("unsupported analysis stage %q", mode)
		}
		stageMS := analysisDurationMS(time.Since(stageStart))

		return &InputAnalysisResult{
			AnalysisKey: textHash,
			Format:      string(model.Format),
			Version:     model.Version.Raw,
			Report:      &report,
			Timing: &AnalysisTiming{
				Mode:      mode,
				CacheHit:  false,
				TotalMS:   analysisDurationMS(time.Since(requestStart)),
				ParseMS:   parseMS,
				AnalyzeMS: stageMS,
				Stages:    map[string]int64{stageName: stageMS},
			},
		}, nil
	})
	if err != nil {
		return nil, err
	}

	return analysisResultForRequest(result, requestStart, mode, cacheHit, queueWait, parseMS), nil
}

func (a *App) cachedCompletedStageAnalysis(textHash string) *InputAnalysisResult {
	quick, ok := a.analysisCache.LookupTextMode(textHash, "quick")
	if !ok {
		quick, ok = a.analysisCache.LookupTextMode(textHash, "overview")
	}
	if !ok || quick == nil || quick.Report == nil {
		return nil
	}

	assembled := cloneInputAnalysisResult(quick)
	report := *quick.Report
	requiredStages := []string{"profile", "hvac", "output", "diagnostics", "geometry"}
	for _, stage := range requiredStages {
		stageResult, ok := a.analysisCache.LookupTextMode(textHash, stage)
		if !ok || stageResult == nil || stageResult.Report == nil {
			return nil
		}
		mergeStageReport(&report, stage, stageResult.Report)
	}
	assembled.Report = &report
	if assembled.Timing == nil {
		assembled.Timing = &AnalysisTiming{}
	}
	assembled.Timing.Mode = "full"
	assembled.Timing.CacheHit = true
	assembled.Timing.TotalMS = 0
	assembled.Timing.QueueWaitMS = 0
	return assembled
}

func mergeStageReport(target *idf.Report, stage string, source *idf.Report) {
	switch stage {
	case "profile":
		target.Profile = source.Profile
	case "hvac":
		target.HVAC = source.HVAC
	case "output":
		target.Output = source.Output
	case "diagnostics":
		target.Diagnostics = source.Diagnostics
	case "geometry":
		target.Geometry = source.Geometry
	}
}

func normalizeAnalysisStage(stage string) string {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "profile":
		return "profile"
	case "hvac":
		return "hvac"
	case "output", "output-detail", "outputs":
		return "output"
	case "diagnose", "diagnostic", "diagnostics":
		return "diagnostics"
	case "geometry":
		return "geometry"
	default:
		return ""
	}
}

func cachedAnalysisResult(result *InputAnalysisResult, requestStart time.Time, mode string) *InputAnalysisResult {
	return analysisResultForRequest(result, requestStart, mode, true, 0, 0)
}

func analysisResultForRequest(result *InputAnalysisResult, requestStart time.Time, mode string, cacheHit bool, queueWait time.Duration, parseMS int64) *InputAnalysisResult {
	clone := cloneInputAnalysisResult(result)
	if clone == nil {
		return nil
	}
	if clone.Timing == nil {
		clone.Timing = &AnalysisTiming{Mode: mode}
	}
	clone.Timing.Mode = mode
	clone.Timing.CacheHit = cacheHit
	clone.Timing.QueueWaitMS = analysisDurationMS(queueWait)
	clone.Timing.TotalMS = analysisDurationMS(time.Since(requestStart))
	if cacheHit && parseMS > 0 {
		clone.Timing.ParseMS = parseMS
	}
	return clone
}

func analysisStageRecorder() (idf.StageTimer, func() map[string]int64) {
	var mu sync.Mutex
	stages := map[string]int64{}
	timer := func(stage string, elapsed time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		stages[stage] = analysisDurationMS(elapsed)
	}
	snapshot := func() map[string]int64 {
		mu.Lock()
		defer mu.Unlock()
		if len(stages) == 0 {
			return nil
		}
		result := make(map[string]int64, len(stages))
		for key, value := range stages {
			result[key] = value
		}
		return result
	}
	return timer, snapshot
}

func parseInputDocument(text string) (*epinput.Model, idf.Document, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, idf.Document{}, err
	}
	if model.Format == epinput.FormatIDF {
		doc, err := idf.Parse(text)
		if err != nil {
			return nil, idf.Document{}, err
		}
		return model, doc, nil
	}
	return model, epinput.ToIDFDocument(model), nil
}

func semanticProjectionForModelDoc(model *epinput.Model, doc idf.Document) *idf.SemanticYAMLProjection {
	metadata := idf.SemanticYAMLMetadata{}
	if model != nil {
		metadata.EnergyPlusVersion = model.Version.Raw
		metadata.SourceFormat = string(model.Format)
	}
	projection := idf.BuildSemanticYAMLProjection(doc, metadata)
	return &projection
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
		Title:   "Open EnergyPlus inputs for Batch Summary",
		Filters: inputFileFilters(),
	})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return &MultiSummaryResult{Canceled: true, RunID: runID}, nil
	}

	return analyzeMultiSummaryPaths(paths, runID, throttleMultiSummaryProgress(func(progress MultiSummaryProgress) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "idfAnalyzer:multiSummaryProgress", progress)
			wailsruntime.EventsEmit(a.ctx, "idfAnalyzer:batchProgress", progress)
		}
	})), nil
}

func throttleMultiSummaryProgress(emit func(MultiSummaryProgress)) func(MultiSummaryProgress) {
	var mu sync.Mutex
	lastEmit := time.Time{}
	return func(progress MultiSummaryProgress) {
		if emit == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		now := time.Now()
		final := progress.Total > 0 && progress.Completed >= progress.Total
		if !final && !lastEmit.IsZero() && now.Sub(lastEmit) < 150*time.Millisecond {
			return
		}
		lastEmit = now
		emit(progress)
	}
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

func (a *App) ExportSummaryText(text string, format string) (*SummaryExportResult, error) {
	_, doc, err := parseInputDocument(text)
	if err != nil {
		return nil, err
	}
	summary := idf.AnalyzeSummary(doc)

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

func defaultAppSettings() AppSettings {
	return AppSettings{
		Version: 1,
		Appearance: AppearanceSettings{
			Theme:            "system",
			Language:         "en",
			AnalysisTabOrder: []string{"summary", "geometry", "profile", "hvac", "diagnose", "output", "simulation"},
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
				"inputSemantic":  "Ctrl+1",
				"inputText":      "Ctrl+2",
				"inputJson":      "Ctrl+3",
				"inputTable":     "Ctrl+4",
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
		Simulation: simulation.DefaultSettings(),
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
	settings.Simulation = simulation.NormalizeSettings(settings.Simulation, defaults.Simulation)
	return settings
}

func normalizeShortcutSettings(values map[string]string, defaults map[string]string) map[string]string {
	out := make(map[string]string, len(defaults))
	if _, hasSemantic := values["inputSemantic"]; !hasSemantic {
		values = migrateInputViewShortcutDefaults(values)
	}
	for key, fallback := range defaults {
		value := strings.TrimSpace(values[key])
		if value == "" {
			value = fallback
		}
		out[key] = value
	}
	return out
}

func migrateInputViewShortcutDefaults(values map[string]string) map[string]string {
	migrated := make(map[string]string, len(values)+1)
	for key, value := range values {
		migrated[key] = value
	}
	if isBlankOrShortcut(migrated["inputText"], "Ctrl+1") {
		migrated["inputText"] = "Ctrl+2"
	}
	if isBlankOrShortcut(migrated["inputJson"], "Ctrl+2") {
		migrated["inputJson"] = "Ctrl+3"
	}
	if isBlankOrShortcut(migrated["inputTable"], "Ctrl+3") {
		migrated["inputTable"] = "Ctrl+4"
	}
	return migrated
}

func isBlankOrShortcut(value string, expected string) bool {
	value = strings.TrimSpace(value)
	return value == "" || strings.EqualFold(value, expected)
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
		source = []string{"summary", "geometry", "profile", "hvac", "diagnose", "output", "simulation"}
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
	settings.GraphMode = normalizeProfileChoice(settings.GraphMode, []string{"multiplier", "actual_value", "actual", "design", "annual"}, defaults.GraphMode)
	settings.MetricMode = normalizeProfileChoice(settings.MetricMode, []string{"design", "multiplier", "actual", "annual"}, profileMetricModeSetting(settings.GraphMode, defaults.MetricMode))
	settings.TimeView = normalizeProfileChoice(settings.TimeView, []string{"day", "week", "month", "year", "duration", "rules"}, defaults.TimeView)
	settings.CompareMode = normalizeProfileChoice(settings.CompareMode, []string{"single", "overlay", "small_multiples", "ranking", "similarity", "outliers"}, defaults.CompareMode)
	settings.ScaleMode = normalizeProfileChoice(settings.ScaleMode, []string{"auto", "shared", "design_peak", "multiplier_0_1", "percentile"}, defaults.ScaleMode)
	settings.GraphDeck.ScopeType = normalizeProfileChoice(settings.GraphDeck.ScopeType, []string{"group", "zone", "schedule", "dimension", "selection"}, defaults.GraphDeck.ScopeType)
	settings.GraphDeck.MetricMode = normalizeProfileChoice(settings.GraphDeck.MetricMode, []string{"design", "multiplier", "actual", "annual"}, settings.MetricMode)
	settings.GraphDeck.TimeView = normalizeProfileChoice(settings.GraphDeck.TimeView, []string{"day", "week", "month", "year", "duration", "rules"}, settings.TimeView)
	settings.GraphDeck.CompareMode = normalizeProfileChoice(settings.GraphDeck.CompareMode, []string{"single", "overlay", "small_multiples", "ranking", "similarity", "outliers"}, settings.CompareMode)
	settings.GraphDeck.ScaleMode = normalizeProfileChoice(settings.GraphDeck.ScaleMode, []string{"auto", "shared", "design_peak", "multiplier_0_1", "percentile"}, settings.ScaleMode)
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

func profileMetricModeSetting(graphMode string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(graphMode)) {
	case "multiplier":
		return "multiplier"
	case "design":
		return "design"
	case "annual":
		return "annual"
	default:
		return fallback
	}
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
