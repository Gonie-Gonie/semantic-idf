package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/epinput"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

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
	if strings.TrimSpace(textHash) == "" {
		return nil, nil
	}
	a.initializedAnalysisCache()
	if result, ok := a.analysisCache.LookupTextMode(textHash, "full"); ok {
		cached := cloneInputAnalysisResult(result)
		if cached.Timing == nil {
			cached.Timing = &AnalysisTiming{Mode: "full"}
		}
		cached.Timing.CacheHit = true
		cached.Timing.TotalMS = 0
		cached.Timing.QueueWaitMS = 0
		return cached, nil
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
	result, err := a.analyzeInputStageText(text, "geometry")
	if err != nil {
		return nil, err
	}
	geometry := result.Report.Geometry
	return &geometry, nil
}

// AnalyzeInputTopologyText returns the same cached topology report used by the Topology view.
func (a *App) AnalyzeInputTopologyText(text string) (*idf.ThermalTopologyReport, error) {
	geometry, err := a.AnalyzeInputGeometryText(text)
	if err != nil {
		return nil, err
	}
	topology := geometry.Topology
	return &topology, nil
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
	a.initializedAnalysisCache()
	requestStart := time.Now()
	textHash := analysisTextHash(text)
	if cached, ok := a.analysisCache.LookupTextMode(textHash, mode); ok {
		return cachedAnalysisResult(cached, requestStart, mode), nil
	}

	parseStart := time.Now()
	input, err := a.analysisInputForText(textHash, text)
	if err != nil {
		return nil, err
	}
	model, doc := input.model, input.doc
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
			report = idf.AnalyzeQuickFromIndex(input.documentIndex(), stageTimer)
		case "overview":
			report = idf.AnalyzeOverviewTimed(doc, stageTimer)
		default:
			report = idf.AnalyzeTimed(doc, stageTimer)
		}
		slimReportForMode(&report, mode)
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
	a.initializedAnalysisCache()
	requestStart := time.Now()
	textHash := analysisTextHash(text)
	if cached, ok := a.analysisCache.LookupTextMode(textHash, mode); ok {
		return cachedAnalysisResult(cached, requestStart, mode), nil
	}

	parseStart := time.Now()
	input, err := a.analysisInputForText(textHash, text)
	if err != nil {
		return nil, err
	}
	model := input.model
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
		stageDetails := map[string]int64{}
		index := input.documentIndex()
		switch mode {
		case "profile":
			report.Profile = idf.AnalyzeProfileFromIndex(index)
		case "hvac":
			report.HVAC = idf.AnalyzeHVACFromIndex(index)
		case "hvac-debug":
			report.HVAC = idf.AnalyzeHVACFromIndex(index)
		case "output":
			report.Output = idf.AnalyzeOutputFromIndex(index)
		case "diagnostics":
			report.Diagnostics = idf.AnalyzeDiagnosticsFromIndex(index)
		case "geometry":
			stageTimer, stageSnapshot := analysisStageRecorder()
			geometry := idf.AnalyzeGeometryFromIndexTimed(index, stageTimer)
			report.Geometry = geometry
			for name, duration := range stageSnapshot() {
				stageDetails[name] = duration
			}
		default:
			return nil, fmt.Errorf("unsupported analysis stage %q", mode)
		}
		slimReportForMode(&report, mode)
		stageMS := analysisDurationMS(time.Since(stageStart))
		stageDetails[stageName] = stageMS

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
				Stages:    stageDetails,
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
	requiredStages := []string{"profile", "hvac", "geometry"}
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
	case "hvac-debug", "debug-hvac", "rule-graph":
		return "hvac-debug"
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

func slimReportForMode(report *idf.Report, mode string) {
	if report == nil || mode == "hvac-debug" {
		return
	}
	report.HVAC.RuleGraph = idf.HVACRuleGraph{}
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
