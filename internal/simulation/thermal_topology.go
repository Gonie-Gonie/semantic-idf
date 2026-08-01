package simulation

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/internal/epinput"
	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
)

const thermalTopologySimulationSchema = "semantic-idf.thermal-topology-simulation/v1"

type ThermalTopologySimulationResult struct {
	Schema            string                            `json:"schema"`
	Available         bool                              `json:"available"`
	UnavailableReason string                            `json:"unavailableReason,omitempty"`
	State             string                            `json:"state"`
	SignConvention    string                            `json:"signConvention"`
	Periods           []ThermalTopologySimulationPeriod `json:"periods,omitempty"`
	Sources           []EnergyDataSource                `json:"sources,omitempty"`
	Completeness      []PurposeCompletenessItem         `json:"completeness,omitempty"`
	Reconciliation    []EnergyReconciliation            `json:"reconciliation,omitempty"`
	OutputWeight      ThermalTopologyOutputWeight       `json:"outputWeight"`
}

type ThermalTopologySimulationPeriod struct {
	ID              string                          `json:"id"`
	Label           string                          `json:"label"`
	Kind            string                          `json:"kind"`
	Labels          []string                        `json:"labels,omitempty"`
	FrameCount      int                             `json:"frameCount"`
	BoundaryFlows   []ThermalTopologyBoundaryFlow   `json:"boundaryFlows,omitempty"`
	ConnectionFlows []ThermalTopologyConnectionFlow `json:"connectionFlows,omitempty"`
}

type ThermalTopologyBoundaryFlow struct {
	ID                 string                     `json:"id"`
	BoundaryID         string                     `json:"boundaryId"`
	RelatedBoundaryIDs []string                   `json:"relatedBoundaryIds,omitempty"`
	ConnectionID       string                     `json:"connectionId,omitempty"`
	OwnerNodeID        string                     `json:"ownerNodeId"`
	TargetNodeID       string                     `json:"targetNodeId"`
	Value              float64                    `json:"value"`
	Values             []float64                  `json:"values,omitempty"`
	Unit               string                     `json:"unit"`
	Direction          string                     `json:"direction"`
	SignConvention     string                     `json:"signConvention"`
	AggregationMethod  string                     `json:"aggregationMethod"`
	SourceIDs          []string                   `json:"sourceIds,omitempty"`
	Traces             []ThermalTopologyFlowTrace `json:"traces,omitempty"`
}

type ThermalTopologyConnectionFlow struct {
	ID                string    `json:"id"`
	ConnectionID      string    `json:"connectionId"`
	FromNodeID        string    `json:"fromNodeId"`
	ToNodeID          string    `json:"toNodeId"`
	OwnerNodeID       string    `json:"ownerNodeId,omitempty"`
	BoundaryIDs       []string  `json:"boundaryIds,omitempty"`
	Value             float64   `json:"value"`
	Values            []float64 `json:"values,omitempty"`
	Unit              string    `json:"unit"`
	Direction         string    `json:"direction"`
	SignConvention    string    `json:"signConvention"`
	AggregationMethod string    `json:"aggregationMethod"`
	SourceIDs         []string  `json:"sourceIds,omitempty"`
}

type ThermalTopologyFlowTrace struct {
	Family    string    `json:"family"`
	Value     float64   `json:"value"`
	Values    []float64 `json:"values,omitempty"`
	Unit      string    `json:"unit"`
	SourceIDs []string  `json:"sourceIds,omitempty"`
}

type ThermalTopologyOutputWeight struct {
	Series int    `json:"series"`
	Frames int    `json:"frames"`
	Values int64  `json:"values"`
	Rating string `json:"rating"`
}

type thermalTopologyRawSeries struct {
	keyValue           string
	variableName       string
	unit               string
	reportingFrequency string
	file               string
	sourceType         string
	indexGroup         string
	points             []SimulationPoint
	family             string
	rate               bool
	source             EnergyDataSource
}

type thermalTopologyRawFlow struct {
	boundary           idf.ThermalBoundaryRecord
	relatedBoundaryIDs []string
	connectionID       string
	labels             []string
	values             []float64
	sourceIDs          []string
	aggregationMethods []string
	traces             map[string]thermalTopologyTraceValues
}

type thermalTopologyTraceValues struct {
	values    []float64
	sourceIDs []string
}

type thermalTopologyPeriodDefinition struct {
	id      string
	label   string
	kind    string
	labels  []string
	buckets [][]int
}

func buildThermalTopologySimulationResult(result *SimulationRunResult, request SimulationPurposeRequest) ThermalTopologySimulationResult {
	out := ThermalTopologySimulationResult{
		Schema:         thermalTopologySimulationSchema,
		State:          "static_topology",
		SignConvention: "positive enters the owning zone; negative leaves the owning zone",
	}
	if request.ZoneHeatFlowDetail != PurposeZoneHeatFlowDetailSurface {
		out.UnavailableReason = "Run Zone Heat Flow with Surface detail to load a simulation overlay."
		out.Completeness = []PurposeCompletenessItem{thermalTopologyCompleteness(false, "purpose plan", out.UnavailableReason)}
		return out
	}

	topology, err := thermalTopologyFromSimulationInput(result.InputPath)
	if err != nil {
		out.UnavailableReason = "The simulation input could not be mapped to thermal topology: " + err.Error()
		out.Completeness = []PurposeCompletenessItem{thermalTopologyCompleteness(false, "simulation input", out.UnavailableReason)}
		return out
	}
	series := collectThermalTopologyRawSeries(result)
	if len(series) == 0 {
		out.UnavailableReason = "No compatible surface heat-flow outputs were found. Open the purpose plan and rerun with Surface detail."
		out.Completeness = []PurposeCompletenessItem{thermalTopologyCompleteness(false, "simulation results", out.UnavailableReason)}
		return out
	}

	flows, sources, reconciliations := buildThermalTopologyRawFlows(topology, series)
	if len(flows) == 0 {
		out.UnavailableReason = "Surface heat-flow outputs were present but none matched a topology boundary."
		out.Completeness = []PurposeCompletenessItem{thermalTopologyCompleteness(false, "surface-to-boundary mapping", out.UnavailableReason)}
		return out
	}

	frameLabels := longestThermalTopologyLabels(flows)
	periodDefinitions := thermalTopologyPeriodDefinitions(frameLabels)
	for _, definition := range periodDefinitions {
		out.Periods = append(out.Periods, buildThermalTopologySimulationPeriod(topology, flows, definition))
	}
	out.Available = len(out.Periods) > 0
	out.State = "simulation_overlay"
	out.Sources = sources
	out.Reconciliation = reconciliations
	out.Completeness = []PurposeCompletenessItem{
		thermalTopologyCompleteness(true, thermalTopologySourcesLabel(sources), "Surface heat-flow outputs are mapped to stable thermal boundary IDs."),
	}
	frameCount := len(frameLabels)
	out.OutputWeight = ThermalTopologyOutputWeight{
		Series: len(sources),
		Frames: frameCount,
		Values: int64(len(sources)) * int64(frameCount),
		Rating: thermalTopologyWeightRating(len(sources), frameCount),
	}
	return out
}

func thermalTopologyCompleteness(found bool, source string, message string) PurposeCompletenessItem {
	item := purposeCompleteness(SimulationPurposeZoneHeatFlow, "Thermal topology surface heat flow", found, source)
	item.Message = message
	return item
}

func thermalTopologyFromSimulationInput(path string) (idf.ThermalTopologyReport, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return idf.ThermalTopologyReport{}, fmt.Errorf("input path is missing")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return idf.ThermalTopologyReport{}, err
	}
	if epinput.DetectFormat(path, content) == epinput.FormatIDF {
		doc, parseErr := idf.Parse(string(content))
		if parseErr != nil {
			return idf.ThermalTopologyReport{}, parseErr
		}
		return idf.AnalyzeGeometry(doc).Topology, nil
	}
	model, err := epinput.Parse(path, content)
	if err != nil {
		return idf.ThermalTopologyReport{}, err
	}
	doc := epinput.ToIDFDocument(model)
	return idf.AnalyzeGeometry(doc).Topology, nil
}

func collectThermalTopologyRawSeries(result *SimulationRunResult) []thermalTopologyRawSeries {
	for _, file := range result.Files {
		if file.Kind != "sqlite" {
			continue
		}
		series, err := loadThermalTopologyRawSeriesFromSQL(file.Path)
		if err == nil && len(series) > 0 {
			return series
		}
	}
	return loadThermalTopologyRawSeriesFromFallback(result.Series)
}

func loadThermalTopologyRawSeriesFromSQL(path string) ([]thermalTopologyRawSeries, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ready, err := sqlHasTables(db, "ReportDataDictionary", "ReportData", "Time")
	if err != nil || !ready {
		return nil, err
	}
	names := thermalTopologySurfaceVariableNames()
	type bucket struct {
		row    SQLSeriesRow
		points []SimulationPoint
	}
	buckets := map[int]*bucket{}
	ordinals := map[int]int{}
	err = walkReportData(db, SQLSeriesQuery{Names: names}, func(row SQLSeriesRow) error {
		if !row.Value.Valid {
			return nil
		}
		item := buckets[row.DictionaryIndex]
		if item == nil {
			item = &bucket{row: row}
			buckets[row.DictionaryIndex] = item
		}
		ordinals[row.DictionaryIndex]++
		item.points = append(item.points, SimulationPoint{
			X:     ordinals[row.DictionaryIndex],
			Label: sqlFrameLabel(row.Month, row.Day, row.Hour, row.Minute),
			Value: row.Value.Float64,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(buckets))
	for id := range buckets {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	out := make([]thermalTopologyRawSeries, 0, len(ids))
	for _, id := range ids {
		item := buckets[id]
		family, rate, ok := classifyThermalTopologySurfaceVariable(item.row.Name)
		if !ok {
			continue
		}
		raw := thermalTopologyRawSeries{
			keyValue:           strings.TrimSpace(item.row.KeyValue),
			variableName:       strings.TrimSpace(item.row.Name),
			unit:               strings.TrimSpace(item.row.Units),
			reportingFrequency: strings.TrimSpace(item.row.ReportingFrequency),
			file:               filepath.Base(path),
			sourceType:         "sql_report_data",
			indexGroup:         strings.TrimSpace(item.row.IndexGroup),
			points:             item.points,
			family:             family,
			rate:               rate,
		}
		raw.source = thermalTopologyEnergySource(raw)
		out = append(out, raw)
	}
	return out, nil
}

func loadThermalTopologyRawSeriesFromFallback(series []SimulationSeries) []thermalTopologyRawSeries {
	out := []thermalTopologyRawSeries{}
	for _, item := range series {
		keyValue, variableName := splitPurposeSeriesColumn(item.Column)
		family, rate, ok := classifyThermalTopologySurfaceVariable(variableName)
		if !ok || keyValue == "" || len(item.Points) == 0 {
			continue
		}
		raw := thermalTopologyRawSeries{
			keyValue:           keyValue,
			variableName:       variableName,
			unit:               unitFromSeriesColumn(item.Column),
			reportingFrequency: "Hourly",
			file:               item.File,
			sourceType:         "simulation_series",
			points:             append([]SimulationPoint(nil), item.Points...),
			family:             family,
			rate:               rate,
		}
		raw.source = thermalTopologyEnergySource(raw)
		out = append(out, raw)
	}
	return out
}

func thermalTopologySurfaceVariableNames() []string {
	values := []string{}
	for _, opening := range []bool{false, true} {
		for _, definition := range surfaceHeatFlowVariableDefinitions(opening) {
			values = append(values, definition.EnergyName)
			values = append(values, definition.EnergyAliases...)
			values = append(values, definition.RateNames...)
		}
	}
	return uniqueSortedThermalTopologyStrings(values)
}

func classifyThermalTopologySurfaceVariable(name string) (string, bool, bool) {
	wanted := normalizePurposeToken(name)
	for _, opening := range []bool{false, true} {
		for _, definition := range surfaceHeatFlowVariableDefinitions(opening) {
			for _, energyName := range append([]string{definition.EnergyName}, definition.EnergyAliases...) {
				if wanted == normalizePurposeToken(energyName) {
					return definition.Family, false, true
				}
			}
			for _, rateName := range definition.RateNames {
				if wanted == normalizePurposeToken(rateName) {
					return definition.Family, true, true
				}
			}
		}
	}
	return "", false, false
}

func thermalTopologyEnergySource(raw thermalTopologyRawSeries) EnergyDataSource {
	method := "sum_reported_energy"
	if raw.rate {
		method = "integrate_reported_rate_" + strings.ToLower(firstThermalTopologyValue(raw.reportingFrequency, "hourly"))
	}
	stableKey := strings.Join([]string{raw.sourceType, raw.file, raw.keyValue, raw.variableName, raw.unit, raw.reportingFrequency}, "\x00")
	return EnergyDataSource{
		ID:                 "thermal-source:" + thermalTopologyStableHash(stableKey, 20),
		SourceType:         raw.sourceType,
		KeyValue:           raw.keyValue,
		Name:               raw.variableName,
		Units:              raw.unit,
		SourceUnit:         raw.unit,
		NormalizedUnit:     "kWh",
		ReportingFrequency: firstThermalTopologyValue(raw.reportingFrequency, "Hourly"),
		AggregationMethod:  method,
		IndexGroup:         raw.indexGroup,
	}
}

func buildThermalTopologyRawFlows(topology idf.ThermalTopologyReport, series []thermalTopologyRawSeries) ([]thermalTopologyRawFlow, []EnergyDataSource, []EnergyReconciliation) {
	seriesByKeyFamily := map[string]map[string][]thermalTopologyRawSeries{}
	for _, item := range series {
		key := normalizePurposeToken(item.keyValue)
		if key == "" {
			continue
		}
		if seriesByKeyFamily[key] == nil {
			seriesByKeyFamily[key] = map[string][]thermalTopologyRawSeries{}
		}
		seriesByKeyFamily[key][item.family] = append(seriesByKeyFamily[key][item.family], item)
	}
	boundaryBySurfaceID := map[string]idf.ThermalBoundaryRecord{}
	for _, boundary := range topology.Boundaries {
		boundaryBySurfaceID[boundary.SurfaceID] = boundary
	}
	openingsByBoundaryID := map[string][]idf.ThermalOpeningRecord{}
	for _, opening := range topology.Openings {
		if boundary, ok := boundaryBySurfaceID[opening.BaseSurfaceID]; ok {
			openingsByBoundaryID[boundary.ID] = append(openingsByBoundaryID[boundary.ID], opening)
		}
	}
	connectionByBoundaryID := map[string]string{}
	for _, connection := range topology.Connections {
		for _, boundaryID := range connection.BoundaryIDs {
			connectionByBoundaryID[boundaryID] = connection.ID
		}
	}

	rawByBoundaryID := map[string]thermalTopologyRawFlow{}
	sourceByID := map[string]EnergyDataSource{}
	reconciliations := []EnergyReconciliation{}
	for _, boundary := range topology.Boundaries {
		familySeries := seriesByKeyFamily[normalizePurposeToken(boundary.SurfaceName)]
		selected := map[string]*thermalTopologyRawSeries{}
		for _, family := range []string{"average_conduction", "inside_conduction", "outside_conduction"} {
			selected[family] = chooseThermalTopologySeries(familySeries[family])
		}
		flow := thermalTopologyRawFlow{
			boundary:     boundary,
			connectionID: connectionByBoundaryID[boundary.ID],
			traces:       map[string]thermalTopologyTraceValues{},
		}
		for family, item := range selected {
			if item == nil {
				continue
			}
			values, labels := normalizeThermalTopologySeries(*item)
			flow.labels = longerThermalTopologyLabels(flow.labels, labels)
			flow.traces[family] = thermalTopologyTraceValues{values: values, sourceIDs: []string{item.source.ID}}
			sourceByID[item.source.ID] = item.source
		}
		flow.values, flow.sourceIDs, flow.aggregationMethods = selectOpaqueThermalTopologyMeasurement(selected, flow.traces)
		for _, opening := range openingsByBoundaryID[boundary.ID] {
			openingFamilies := seriesByKeyFamily[normalizePurposeToken(opening.Name)]
			gain := chooseThermalTopologySeries(openingFamilies["window_gain"])
			loss := chooseThermalTopologySeries(openingFamilies["window_loss"])
			for _, item := range []*thermalTopologyRawSeries{gain, loss} {
				if item == nil {
					continue
				}
				values, labels := normalizeThermalTopologySeries(*item)
				if item.family == "window_loss" {
					values = scaleThermalTopologyValues(values, -1)
				}
				flow.values = addThermalTopologyValues(flow.values, values)
				flow.labels = longerThermalTopologyLabels(flow.labels, labels)
				flow.sourceIDs = appendUniqueThermalTopologyStrings(flow.sourceIDs, item.source.ID)
				flow.aggregationMethods = appendUniqueThermalTopologyStrings(flow.aggregationMethods, item.source.AggregationMethod)
				flow.traces[item.family+":"+opening.ID] = thermalTopologyTraceValues{values: values, sourceIDs: []string{item.source.ID}}
				sourceByID[item.source.ID] = item.source
			}
		}
		if len(flow.values) == 0 {
			continue
		}
		rawByBoundaryID[boundary.ID] = flow
		if reconciliation, ok := reconcileThermalTopologyBoundary(flow); ok {
			reconciliations = append(reconciliations, reconciliation)
		}
	}

	canonicalFlows := []thermalTopologyRawFlow{}
	visitedPairs := map[string]bool{}
	for _, boundary := range topology.Boundaries {
		if boundary.PairID == "" {
			if flow, ok := rawByBoundaryID[boundary.ID]; ok {
				flow.relatedBoundaryIDs = []string{boundary.ID}
				canonicalFlows = append(canonicalFlows, flow)
			}
			continue
		}
		if visitedPairs[boundary.PairID] {
			continue
		}
		visitedPairs[boundary.PairID] = true
		pair := []idf.ThermalBoundaryRecord{boundary}
		for _, candidate := range topology.Boundaries {
			if candidate.ID != boundary.ID && candidate.PairID == boundary.PairID {
				pair = append(pair, candidate)
			}
		}
		sort.Slice(pair, func(i, j int) bool { return pair[i].ID < pair[j].ID })
		canonical := pair[0]
		flow, ok := rawByBoundaryID[canonical.ID]
		if !ok && len(pair) > 1 {
			flow, ok = rawByBoundaryID[pair[1].ID]
			if ok {
				flow.values = scaleThermalTopologyValues(flow.values, -1)
				for family, trace := range flow.traces {
					trace.values = scaleThermalTopologyValues(trace.values, -1)
					flow.traces[family] = trace
				}
				flow.boundary = canonical
			}
		}
		if !ok {
			continue
		}
		flow.boundary = canonical
		flow.connectionID = connectionByBoundaryID[canonical.ID]
		flow.relatedBoundaryIDs = make([]string, 0, len(pair))
		for _, item := range pair {
			flow.relatedBoundaryIDs = append(flow.relatedBoundaryIDs, item.ID)
		}
		canonicalFlows = append(canonicalFlows, flow)
	}
	sort.Slice(canonicalFlows, func(i, j int) bool { return canonicalFlows[i].boundary.ID < canonicalFlows[j].boundary.ID })
	sources := make([]EnergyDataSource, 0, len(sourceByID))
	for _, source := range sourceByID {
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].ID < sources[j].ID })
	sort.Slice(reconciliations, func(i, j int) bool { return reconciliations[i].ID < reconciliations[j].ID })
	return canonicalFlows, sources, reconciliations
}

func chooseThermalTopologySeries(items []thermalTopologyRawSeries) *thermalTopologyRawSeries {
	if len(items) == 0 {
		return nil
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].rate != items[j].rate {
			return !items[i].rate
		}
		if len(items[i].points) != len(items[j].points) {
			return len(items[i].points) > len(items[j].points)
		}
		return items[i].source.ID < items[j].source.ID
	})
	selected := items[0]
	return &selected
}

func selectOpaqueThermalTopologyMeasurement(selected map[string]*thermalTopologyRawSeries, traces map[string]thermalTopologyTraceValues) ([]float64, []string, []string) {
	if average := selected["average_conduction"]; average != nil {
		return append([]float64(nil), traces["average_conduction"].values...), []string{average.source.ID}, []string{average.source.AggregationMethod}
	}
	inside := selected["inside_conduction"]
	outside := selected["outside_conduction"]
	if inside != nil && outside != nil {
		values := scaleThermalTopologyValues(addThermalTopologyValues(traces["inside_conduction"].values, scaleThermalTopologyValues(traces["outside_conduction"].values, -1)), 0.5)
		return values, []string{inside.source.ID, outside.source.ID}, uniqueSortedThermalTopologyStrings([]string{inside.source.AggregationMethod, outside.source.AggregationMethod})
	}
	if inside != nil {
		return append([]float64(nil), traces["inside_conduction"].values...), []string{inside.source.ID}, []string{inside.source.AggregationMethod}
	}
	if outside != nil {
		return scaleThermalTopologyValues(traces["outside_conduction"].values, -1), []string{outside.source.ID}, []string{outside.source.AggregationMethod}
	}
	return nil, nil, nil
}

func normalizeThermalTopologySeries(item thermalTopologyRawSeries) ([]float64, []string) {
	values := make([]float64, len(item.points))
	labels := make([]string, len(item.points))
	intervalHours := thermalTopologyIntervalHours(item)
	for index, point := range item.points {
		labels[index] = firstThermalTopologyValue(point.Label, fmt.Sprintf("Frame %d", index+1))
		if item.rate {
			values[index] = thermalTopologyRateToKWh(point.Value, item.unit, intervalHours[index])
		} else {
			values[index] = thermalTopologyEnergyToKWh(point.Value, item.unit)
		}
	}
	return values, labels
}

func thermalTopologyIntervalHours(item thermalTopologyRawSeries) []float64 {
	hours := make([]float64, len(item.points))
	defaultHours := 1.0
	switch strings.ToLower(strings.TrimSpace(item.reportingFrequency)) {
	case "daily":
		defaultHours = 24
	case "monthly":
		defaultHours = 730
	case "annual", "runperiod", "run period":
		defaultHours = 8760
	}
	for index := range hours {
		hours[index] = defaultHours
	}
	for index := 1; index < len(item.points); index++ {
		previous, previousOK := parseThermalTopologyFrameTime(item.points[index-1].Label)
		current, currentOK := parseThermalTopologyFrameTime(item.points[index].Label)
		if previousOK && currentOK {
			delta := current.Sub(previous).Hours()
			if delta > 0 && delta <= 744 {
				hours[index] = delta
				hours[index-1] = delta
			}
		}
	}
	return hours
}

func thermalTopologyEnergyToKWh(value float64, unit string) float64 {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "j", "joule", "joules", "":
		return value / 3_600_000
	case "kj":
		return value / 3_600
	case "mj":
		return value / 3.6
	case "gj":
		return value * (1000 / 3.6)
	case "wh":
		return value / 1000
	case "kwh":
		return value
	case "mwh":
		return value * 1000
	default:
		return value
	}
}

func thermalTopologyRateToKWh(value float64, unit string, hours float64) float64 {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "w", "watt", "watts", "":
		return value * hours / 1000
	case "kw":
		return value * hours
	case "mw":
		return value * hours * 1000
	default:
		return value * hours
	}
}

func reconcileThermalTopologyBoundary(flow thermalTopologyRawFlow) (EnergyReconciliation, bool) {
	average, hasAverage := flow.traces["average_conduction"]
	inside, hasInside := flow.traces["inside_conduction"]
	outside, hasOutside := flow.traces["outside_conduction"]
	if !hasAverage || !hasInside || !hasOutside {
		return EnergyReconciliation{}, false
	}
	expected := sumThermalTopologyValues(average.values)
	explained := sumThermalTopologyValues(scaleThermalTopologyValues(addThermalTopologyValues(inside.values, scaleThermalTopologyValues(outside.values, -1)), 0.5))
	residual := expected - explained
	status := "ok"
	if math.Abs(residual) > math.Max(0.01, math.Abs(expected)*0.02) {
		status = "mismatch"
	}
	return EnergyReconciliation{
		ID:             "thermal-reconciliation:" + thermalTopologyStableHash(flow.boundary.ID, 20),
		Level:          "boundary",
		Period:         "annual",
		Label:          flow.boundary.SurfaceName + " face-energy reconciliation",
		Status:         status,
		ZoneName:       flow.boundary.OwnerZoneID,
		ExpectedValue:  expected,
		ExplainedValue: explained,
		ResidualValue:  residual,
		Unit:           "kWh",
		Basis:          "surface face conduction",
		Formula:        "average_face = (inside_face - outside_face) / 2",
		SourceIDs:      appendUniqueThermalTopologyStrings(appendUniqueThermalTopologyStrings(average.sourceIDs, inside.sourceIDs...), outside.sourceIDs...),
	}, true
}

func thermalTopologyPeriodDefinitions(labels []string) []thermalTopologyPeriodDefinition {
	if len(labels) == 0 {
		return nil
	}
	definitions := []thermalTopologyPeriodDefinition{
		thermalTopologyGroupedPeriod("annual", "Annual", "annual", labels, func(_ string, _ int) string { return "Annual" }),
		thermalTopologyGroupedPeriod("monthly", "Monthly", "monthly", labels, thermalTopologyMonthBucket),
		thermalTopologyGroupedPeriod("daily", "Daily", "daily", labels, thermalTopologyDayBucket),
		thermalTopologyGroupedPeriod("hourly", "Hourly", "hourly", labels, func(label string, index int) string {
			return firstThermalTopologyValue(label, fmt.Sprintf("Frame %d", index+1))
		}),
	}
	return definitions
}

func thermalTopologyGroupedPeriod(id string, label string, kind string, frameLabels []string, key func(string, int) string) thermalTopologyPeriodDefinition {
	definition := thermalTopologyPeriodDefinition{id: id, label: label, kind: kind}
	indexByLabel := map[string]int{}
	for index, frameLabel := range frameLabels {
		bucketLabel := key(frameLabel, index)
		bucketIndex, ok := indexByLabel[bucketLabel]
		if !ok {
			bucketIndex = len(definition.labels)
			indexByLabel[bucketLabel] = bucketIndex
			definition.labels = append(definition.labels, bucketLabel)
			definition.buckets = append(definition.buckets, nil)
		}
		definition.buckets[bucketIndex] = append(definition.buckets[bucketIndex], index)
	}
	return definition
}

func thermalTopologyMonthBucket(label string, index int) string {
	if parsed, ok := parseThermalTopologyFrameTime(label); ok {
		return parsed.Format("Jan")
	}
	month := minThermalTopologyInt(11, index/730)
	return time.Date(2001, time.Month(month+1), 1, 0, 0, 0, 0, time.UTC).Format("Jan")
}

func thermalTopologyDayBucket(label string, index int) string {
	if parsed, ok := parseThermalTopologyFrameTime(label); ok {
		return parsed.Format("Jan 02")
	}
	return fmt.Sprintf("Day %d", index/24+1)
}

func parseThermalTopologyFrameTime(label string) (time.Time, bool) {
	label = strings.TrimSpace(label)
	for _, layout := range []string{"01-02 15:04", "1-2 15:04", "01/02 15:04", "1/2 15:04", "01/02 15:04:05", "2006-01-02 15:04", "2006/01/02 15:04"} {
		value, err := time.Parse(layout, label)
		if err == nil {
			if value.Year() == 0 {
				value = time.Date(2001, value.Month(), value.Day(), value.Hour(), value.Minute(), value.Second(), 0, time.UTC)
			}
			return value, true
		}
	}
	return time.Time{}, false
}

func buildThermalTopologySimulationPeriod(topology idf.ThermalTopologyReport, flows []thermalTopologyRawFlow, definition thermalTopologyPeriodDefinition) ThermalTopologySimulationPeriod {
	period := ThermalTopologySimulationPeriod{
		ID:         definition.id,
		Label:      definition.label,
		Kind:       definition.kind,
		Labels:     append([]string(nil), definition.labels...),
		FrameCount: len(definition.labels),
	}
	connectionByID := map[string]idf.ThermalConnectionAggregate{}
	for _, connection := range topology.Connections {
		connectionByID[connection.ID] = connection
	}
	type connectionAccumulator struct {
		flow ThermalTopologyConnectionFlow
	}
	connectionFlows := map[string]*connectionAccumulator{}
	for _, raw := range flows {
		values := aggregateThermalTopologyValues(raw.values, definition.buckets)
		value := sumThermalTopologyValues(values)
		traces := []ThermalTopologyFlowTrace{}
		traceFamilies := make([]string, 0, len(raw.traces))
		for family := range raw.traces {
			traceFamilies = append(traceFamilies, family)
		}
		sort.Strings(traceFamilies)
		for _, family := range traceFamilies {
			trace := raw.traces[family]
			traceValues := aggregateThermalTopologyValues(trace.values, definition.buckets)
			traces = append(traces, ThermalTopologyFlowTrace{
				Family: family, Value: sumThermalTopologyValues(traceValues), Values: traceValues, Unit: "kWh", SourceIDs: append([]string(nil), trace.sourceIDs...),
			})
		}
		boundaryFlow := ThermalTopologyBoundaryFlow{
			ID:                 "thermal-boundary-flow:" + raw.boundary.ID + ":" + definition.id,
			BoundaryID:         raw.boundary.ID,
			RelatedBoundaryIDs: append([]string(nil), raw.relatedBoundaryIDs...),
			ConnectionID:       raw.connectionID,
			OwnerNodeID:        raw.boundary.OwnerZoneID,
			TargetNodeID:       raw.boundary.TargetID,
			Value:              value,
			Values:             values,
			Unit:               "kWh",
			Direction:          thermalTopologyFlowDirection(value),
			SignConvention:     "positive enters owner",
			AggregationMethod:  strings.Join(uniqueSortedThermalTopologyStrings(raw.aggregationMethods), "+"),
			SourceIDs:          append([]string(nil), raw.sourceIDs...),
			Traces:             traces,
		}
		period.BoundaryFlows = append(period.BoundaryFlows, boundaryFlow)
		if raw.connectionID == "" {
			continue
		}
		accumulator := connectionFlows[raw.connectionID]
		if accumulator == nil {
			connection := connectionByID[raw.connectionID]
			accumulator = &connectionAccumulator{flow: ThermalTopologyConnectionFlow{
				ID:                "thermal-connection-flow:" + raw.connectionID + ":" + definition.id,
				ConnectionID:      raw.connectionID,
				FromNodeID:        connection.FromNodeID,
				ToNodeID:          connection.ToNodeID,
				OwnerNodeID:       raw.boundary.OwnerZoneID,
				Unit:              "kWh",
				SignConvention:    "positive enters owner",
				AggregationMethod: boundaryFlow.AggregationMethod,
			}}
			connectionFlows[raw.connectionID] = accumulator
		}
		if accumulator.flow.OwnerNodeID != raw.boundary.OwnerZoneID {
			accumulator.flow.OwnerNodeID = ""
		}
		accumulator.flow.BoundaryIDs = appendUniqueThermalTopologyStrings(accumulator.flow.BoundaryIDs, raw.relatedBoundaryIDs...)
		accumulator.flow.Values = addThermalTopologyValues(accumulator.flow.Values, values)
		accumulator.flow.SourceIDs = appendUniqueThermalTopologyStrings(accumulator.flow.SourceIDs, raw.sourceIDs...)
	}
	for _, accumulator := range connectionFlows {
		accumulator.flow.Value = sumThermalTopologyValues(accumulator.flow.Values)
		accumulator.flow.Direction = thermalTopologyFlowDirection(accumulator.flow.Value)
		period.ConnectionFlows = append(period.ConnectionFlows, accumulator.flow)
	}
	sort.Slice(period.BoundaryFlows, func(i, j int) bool { return period.BoundaryFlows[i].BoundaryID < period.BoundaryFlows[j].BoundaryID })
	sort.Slice(period.ConnectionFlows, func(i, j int) bool {
		return period.ConnectionFlows[i].ConnectionID < period.ConnectionFlows[j].ConnectionID
	})
	return period
}

func aggregateThermalTopologyValues(values []float64, buckets [][]int) []float64 {
	out := make([]float64, len(buckets))
	for bucketIndex, indexes := range buckets {
		for _, index := range indexes {
			if index >= 0 && index < len(values) {
				out[bucketIndex] += values[index]
			}
		}
	}
	return out
}

func longestThermalTopologyLabels(flows []thermalTopologyRawFlow) []string {
	labels := []string{}
	for _, flow := range flows {
		labels = longerThermalTopologyLabels(labels, flow.labels)
	}
	return labels
}

func longerThermalTopologyLabels(left []string, right []string) []string {
	if len(right) > len(left) {
		return append([]string(nil), right...)
	}
	return left
}

func addThermalTopologyValues(left []float64, right []float64) []float64 {
	length := len(left)
	if len(right) > length {
		length = len(right)
	}
	out := make([]float64, length)
	copy(out, left)
	for index, value := range right {
		out[index] += value
	}
	return out
}

func scaleThermalTopologyValues(values []float64, factor float64) []float64 {
	out := make([]float64, len(values))
	for index, value := range values {
		out[index] = value * factor
	}
	return out
}

func sumThermalTopologyValues(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total
}

func thermalTopologyFlowDirection(value float64) string {
	if value > 0 {
		return "gain"
	}
	if value < 0 {
		return "loss"
	}
	return "balanced"
}

func thermalTopologySourcesLabel(sources []EnergyDataSource) string {
	values := []string{}
	for _, source := range sources {
		values = append(values, source.SourceType)
	}
	return strings.Join(uniqueSortedThermalTopologyStrings(values), "+")
}

func thermalTopologyWeightRating(series int, frames int) string {
	values := int64(series) * int64(frames)
	if values >= 2_000_000 {
		return "very_heavy"
	}
	if values >= 500_000 {
		return "heavy"
	}
	if values >= 100_000 {
		return "medium"
	}
	return "light"
}

func uniqueSortedThermalTopologyStrings(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func appendUniqueThermalTopologyStrings(values []string, additions ...string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	return values
}

func thermalTopologyStableHash(value string, length int) string {
	sum := sha1.Sum([]byte(value))
	encoded := hex.EncodeToString(sum[:])
	if length > 0 && length < len(encoded) {
		return encoded[:length]
	}
	return encoded
}

func firstThermalTopologyValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func minThermalTopologyInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
