package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type BatchMetricsResult struct {
	Canceled             bool                 `json:"canceled,omitempty"`
	RunID                string               `json:"runId,omitempty"`
	Total                int                  `json:"total"`
	Completed            int                  `json:"completed"`
	Succeeded            int                  `json:"succeeded"`
	Failed               int                  `json:"failed"`
	Concurrency          int                  `json:"concurrency"`
	AreaBasis            string               `json:"areaBasis"`
	IncludesFullTopology bool                 `json:"includesFullTopology,omitempty"`
	Metrics              []BatchMetricsMetric `json:"metrics"`
	Files                []BatchMetricsFile   `json:"files"`
}

type BatchMetricsMetric struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Unit     string `json:"unit"`
	CSVName  string `json:"csvName"`
}

type BatchMetricsFile struct {
	Index        int                          `json:"index"`
	Path         string                       `json:"path"`
	Filename     string                       `json:"filename"`
	Label        string                       `json:"label"`
	Format       string                       `json:"format,omitempty"`
	Version      string                       `json:"version,omitempty"`
	Status       string                       `json:"status"`
	Error        string                       `json:"error,omitempty"`
	ObjectCount  int                          `json:"objectCount,omitempty"`
	MetricValues map[string]BatchMetricsValue `json:"metricValues,omitempty"`
	TopologyData *BatchMetricsTopologyData    `json:"topologyData,omitempty"`
	Topology     *idf.ThermalTopologyReport   `json:"topology,omitempty"`
}

type BatchMetricsValue struct {
	DisplayValue   string  `json:"displayValue"`
	Status         string  `json:"status"`
	AreaBasis      string  `json:"areaBasis,omitempty"`
	Coverage       float64 `json:"coverage,omitempty"`
	HasCoverage    bool    `json:"hasCoverage,omitempty"`
	BasisSensitive bool    `json:"basisSensitive,omitempty"`
}

type BatchMetricsTopologyData struct {
	Summary     idf.ThermalTopologyBatchSummary  `json:"summary"`
	Nodes       []idf.ThermalTopologyNode        `json:"nodes,omitempty"`
	Signatures  []idf.ZoneThermalSignature       `json:"zoneSignatures,omitempty"`
	Connections []idf.ThermalConnectionAggregate `json:"thermalConnections,omitempty"`
	Issues      []idf.ThermalTopologyIssueLink   `json:"boundaryIssues,omitempty"`
}

type BatchMetricsRequest struct {
	RunID               string `json:"runId"`
	AreaBasis           string `json:"areaBasis"`
	IncludeFullTopology bool   `json:"includeFullTopology,omitempty"`
}

type BatchMetricsProgress struct {
	RunID     string           `json:"runId"`
	Total     int              `json:"total"`
	Completed int              `json:"completed"`
	Succeeded int              `json:"succeeded"`
	Failed    int              `json:"failed"`
	File      BatchMetricsFile `json:"file"`
}

// Legacy aliases keep older API clients source-compatible while Batch Metrics is
// the canonical workspace terminology.
type MultiSummaryResult = BatchMetricsResult
type MultiSummaryMetric = BatchMetricsMetric
type MultiSummaryFile = BatchMetricsFile
type MultiSummaryValue = BatchMetricsValue
type MultiSummaryTopologyData = BatchMetricsTopologyData
type MultiSummaryRequest = BatchMetricsRequest
type MultiSummaryProgress = BatchMetricsProgress

func (a *App) AnalyzeMultiIDFSummary(runID string) (*BatchMetricsResult, error) {
	return a.AnalyzeBatchMetrics(BatchMetricsRequest{RunID: runID, AreaBasis: "effective"})
}

func (a *App) AnalyzeMultiIDFSummaryWithOptions(request BatchMetricsRequest) (*BatchMetricsResult, error) {
	return a.AnalyzeBatchMetrics(request)
}

func (a *App) AnalyzeBatchMetrics(request BatchMetricsRequest) (*BatchMetricsResult, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("desktop runtime is not ready")
	}
	paths, err := wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   "Open EnergyPlus inputs for Batch Metrics",
		Filters: inputFileFilters(),
	})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return &BatchMetricsResult{Canceled: true, RunID: request.RunID, AreaBasis: normalizeBatchMetricsAreaBasis(request.AreaBasis)}, nil
	}

	return analyzeBatchMetricsPaths(paths, request, throttleBatchMetricsProgress(func(progress BatchMetricsProgress) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "idfAnalyzer:batchMetricsProgress", progress)
			wailsruntime.EventsEmit(a.ctx, "idfAnalyzer:multiSummaryProgress", progress)
			wailsruntime.EventsEmit(a.ctx, "idfAnalyzer:batchProgress", progress)
		}
	})), nil
}

func throttleBatchMetricsProgress(emit func(BatchMetricsProgress)) func(BatchMetricsProgress) {
	var mu sync.Mutex
	lastEmit := time.Time{}
	return func(progress BatchMetricsProgress) {
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

func analyzeBatchMetricsPaths(paths []string, request BatchMetricsRequest, emit func(BatchMetricsProgress)) *BatchMetricsResult {
	areaBasis := normalizeBatchMetricsAreaBasis(request.AreaBasis)
	result := &BatchMetricsResult{
		RunID:                request.RunID,
		Total:                len(paths),
		Completed:            0,
		Concurrency:          batchMetricsConcurrency(len(paths)),
		AreaBasis:            areaBasis,
		IncludesFullTopology: request.IncludeFullTopology,
		Metrics:              batchMetricDefinitions(idf.AnalyzeMetrics(idf.Document{})),
		Files:                make([]BatchMetricsFile, len(paths)),
	}
	result.Metrics = append(result.Metrics, batchTopologyMetricDefinitions()...)
	if len(paths) == 0 {
		return result
	}

	type job struct {
		index int
		path  string
	}

	jobs := make(chan job, len(paths))
	for index, path := range paths {
		jobs <- job{index: index, path: path}
	}
	close(jobs)
	results := make(chan BatchMetricsFile)
	var wg sync.WaitGroup
	for worker := 0; worker < result.Concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				results <- analyzeBatchMetricsFile(item.index, item.path, areaBasis, request.IncludeFullTopology)
			}
		}()
	}

	go func() {
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
			progressFile := file
			progressFile.TopologyData = nil
			progressFile.Topology = nil
			emit(BatchMetricsProgress{
				RunID:     request.RunID,
				Total:     result.Total,
				Completed: result.Completed,
				Succeeded: result.Succeeded,
				Failed:    result.Failed,
				File:      progressFile,
			})
		}
	}

	ensureUniqueBatchMetricsLabels(result.Files)
	return result
}

func analyzeBatchMetricsFile(index int, path string, areaBasis string, includeFullTopology bool) BatchMetricsFile {
	file := BatchMetricsFile{
		Index:    index,
		Path:     path,
		Filename: filepath.Base(path),
		Label:    filepath.Base(path),
		Status:   "ok",
	}

	model, doc, err := parseCachedBatchInput(path)
	if err != nil {
		file.Status = "error"
		file.Error = err.Error()
		return file
	}

	metricsReport := idf.AnalyzeMetrics(doc)
	topology := idf.AnalyzeGeometryFromIndex(idf.NewDocumentIndex(doc)).Topology
	topologySummary := idf.SummarizeThermalTopologyForBatch(topology, areaBasis)
	file.Format = string(model.Format)
	file.Version = model.Version.Raw
	file.ObjectCount = len(doc.Objects)
	file.MetricValues = make(map[string]BatchMetricsValue, metricsReport.MetricCount+len(topologySummary.Metrics))
	for _, category := range metricsReport.Categories {
		for _, metric := range category.Metrics {
			file.MetricValues[metric.ID] = BatchMetricsValue{
				DisplayValue: metric.DisplayValue,
				Status:       metric.Status,
			}
		}
	}
	for metricID, value := range topologySummary.Metrics {
		file.MetricValues[metricID] = BatchMetricsValue{
			DisplayValue: value.DisplayValue, Status: value.Status, AreaBasis: value.AreaBasis,
			Coverage: value.Coverage, HasCoverage: value.HasCoverage, BasisSensitive: value.BasisSensitive,
		}
	}
	file.TopologyData = &BatchMetricsTopologyData{
		Summary:     topologySummary,
		Nodes:       topology.Nodes,
		Signatures:  topology.ZoneSignatures,
		Connections: topology.Connections,
		Issues:      topology.IssueLinks,
	}
	if includeFullTopology {
		copy := topology
		copy.AreaBasis = areaBasis
		file.Topology = &copy
		file.TopologyData.Nodes = append([]idf.ThermalTopologyNode(nil), topology.Nodes...)
		file.TopologyData.Signatures = append([]idf.ZoneThermalSignature(nil), topology.ZoneSignatures...)
		file.TopologyData.Connections = append([]idf.ThermalConnectionAggregate(nil), topology.Connections...)
		file.TopologyData.Issues = append([]idf.ThermalTopologyIssueLink(nil), topology.IssueLinks...)
	}
	if buildingName := strings.TrimSpace(file.MetricValues["building_name"].DisplayValue); buildingName != "" && buildingName != "—" && !strings.EqualFold(buildingName, "N/A") {
		file.Label = buildingName
	}
	return file
}

func batchMetricDefinitions(report idf.MetricsReport) []BatchMetricsMetric {
	names := idf.MetricsCSVNames(report)
	metrics := make([]BatchMetricsMetric, 0, report.MetricCount)
	for _, category := range report.Categories {
		for _, metric := range category.Metrics {
			metrics = append(metrics, BatchMetricsMetric{
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

func batchTopologyMetricDefinitions() []BatchMetricsMetric {
	definitions := idf.ThermalTopologyBatchMetricDefinitions()
	metrics := make([]BatchMetricsMetric, 0, len(definitions))
	for _, definition := range definitions {
		metrics = append(metrics, BatchMetricsMetric{
			ID: definition.ID, Name: definition.Name, Category: "Topology", Unit: definition.Unit, CSVName: definition.ID,
		})
	}
	return metrics
}

func normalizeBatchMetricsAreaBasis(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "physical") {
		return "physical"
	}
	return "effective"
}

func batchMetricsConcurrency(count int) int {
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

func ensureUniqueBatchMetricsLabels(files []BatchMetricsFile) {
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
