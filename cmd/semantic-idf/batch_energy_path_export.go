package main

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/tabular"
)

const batchEnergyPathExportSchema = "semantic-idf.energy-path-batch-export/v2"

// These are presentation snapshots of the shared browser comparison, not a
// second summary model. Nullable numbers must survive the request boundary.
type BatchEnergyPathExportQuality struct {
	Status  string   `json:"status"`
	Found   *float64 `json:"found"`
	Total   *float64 `json:"total"`
	Percent *float64 `json:"percent"`
}

type BatchEnergyPathExportSide struct {
	Value            *float64                     `json:"value"`
	Unit             string                       `json:"unit"`
	Basis            string                       `json:"basis"`
	AggregationBasis string                       `json:"aggregationBasis"`
	Missing          bool                         `json:"missing"`
	Ambiguous        bool                         `json:"ambiguous"`
	Invalid          bool                         `json:"invalid"`
	UnitValid        *bool                        `json:"unitValid,omitempty"`
	Sign             string                       `json:"sign,omitempty"`
	ScaleDomain      string                       `json:"scaleDomain,omitempty"`
	UnitIdentity     string                       `json:"unitIdentity"`
	Quality          BatchEnergyPathExportQuality `json:"quality"`
}

type BatchEnergyPathExportRow struct {
	Key                string                    `json:"key"`
	Kind               string                    `json:"kind"`
	Stage              string                    `json:"stage"`
	StageLabel         string                    `json:"stageLabel"`
	Category           string                    `json:"category"`
	CategoryID         string                    `json:"categoryId"`
	Service            string                    `json:"service"`
	Baseline           BatchEnergyPathExportSide `json:"baseline"`
	Target             BatchEnergyPathExportSide `json:"target"`
	Unit               string                    `json:"unit"`
	DeltaUnit          string                    `json:"deltaUnit"`
	Delta              *float64                  `json:"delta"`
	DeltaPercent       *float64                  `json:"deltaPercent"`
	BasisMismatch      bool                      `json:"basisMismatch"`
	CoverageMismatch   bool                      `json:"coverageMismatch"`
	UnitMismatch       bool                      `json:"unitMismatch"`
	DirectlyComparable bool                      `json:"directlyComparable"`
	WarningCodes       []string                  `json:"warningCodes"`
}

type BatchEnergyPathExportRun struct {
	ResultIndex int                        `json:"resultIndex"`
	RowID       string                     `json:"rowId"`
	Status      string                     `json:"status"`
	Reason      string                     `json:"reason"`
	Rows        []BatchEnergyPathExportRow `json:"rows"`
}

type BatchEnergyPathExportComparison struct {
	BaselineRowID string                     `json:"baselineRowId"`
	TargetRowID   string                     `json:"targetRowId"`
	Status        string                     `json:"status"`
	Reason        string                     `json:"reason"`
	Rows          []BatchEnergyPathExportRow `json:"rows"`
}

type BatchEnergyPathExportProjection struct {
	Schema     string                          `json:"schema"`
	Scope      string                          `json:"scope"`
	Period     string                          `json:"period"`
	Runs       []BatchEnergyPathExportRun      `json:"runs"`
	Comparison BatchEnergyPathExportComparison `json:"comparison"`
}

// Retain the submitted schema and trace before the general purpose-result read
// adapter upgrades legacy data or normalizes sparse optional numeric fields.
func (request *BatchSimulationXLSXExportRequest) UnmarshalJSON(data []byte) error {
	type plain BatchSimulationXLSXExportRequest
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var wire struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*request = BatchSimulationXLSXExportRequest(decoded)
	request.rawResult = append([]byte(nil), wire.Result...)
	return nil
}

type batchEnergyPathRawResult struct {
	Results []struct {
		PurposeResults struct {
			EnergyExplanation        json.RawMessage `json:"energyExplanation"`
			EnergyExplanationSummary json.RawMessage `json:"energyExplanationSummary"`
		} `json:"purposeResults"`
	} `json:"results"`
}

func batchEnergyPathSubmittedResult(request BatchSimulationXLSXExportRequest) ([]byte, error) {
	if len(request.rawResult) > 0 {
		return request.rawResult, nil
	}
	return json.Marshal(request.Result)
}

func batchEnergyPathHasSubmittedV2(request BatchSimulationXLSXExportRequest) (bool, error) {
	if len(request.rawResult) == 0 {
		// MarshalJSON intentionally upgrades Go-level legacy compatibility data;
		// inspect its explicit schema before invoking that writer.
		for _, run := range request.Result.Results {
			if run.PurposeResults != nil && (strings.EqualFold(run.PurposeResults.EnergyExplanation.Schema, "semantic-idf.energy-explanation/v2") || strings.EqualFold(run.PurposeResults.EnergyExplanationSummary.Schema, "semantic-idf.energy-explanation-summary/v2")) {
				return true, nil
			}
		}
		return false, nil
	}
	data, err := batchEnergyPathSubmittedResult(request)
	if err != nil {
		return false, err
	}
	var wire batchEnergyPathRawResult
	if err := json.Unmarshal(data, &wire); err != nil {
		return false, err
	}
	for _, run := range wire.Results {
		for _, part := range []json.RawMessage{run.PurposeResults.EnergyExplanation, run.PurposeResults.EnergyExplanationSummary} {
			var header struct {
				Schema  string          `json:"schema"`
				Summary json.RawMessage `json:"summary"`
			}
			if len(part) == 0 {
				continue
			}
			if err := json.Unmarshal(part, &header); err != nil {
				return false, err
			}
			if strings.EqualFold(header.Schema, "semantic-idf.energy-explanation/v2") || strings.EqualFold(header.Schema, "semantic-idf.energy-explanation-summary/v2") {
				return true, nil
			}
			if len(header.Summary) > 0 {
				var summary struct {
					Schema string `json:"schema"`
				}
				if err := json.Unmarshal(header.Summary, &summary); err != nil {
					return false, err
				}
				if strings.EqualFold(summary.Schema, "semantic-idf.energy-explanation-summary/v2") {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func buildBatchSimulationExportWorkbook(request BatchSimulationXLSXExportRequest) ([]tabular.WorkbookSheet, error) {
	hasV2, err := batchEnergyPathHasSubmittedV2(request)
	if err != nil {
		return nil, fmt.Errorf("Energy Path export: read submitted result: %w", err)
	}
	if request.EnergyPath == nil {
		if hasV2 {
			return nil, fmt.Errorf("Energy Path v2 export requires the current Building / Annual comparison projection")
		}
		return batchSimulationWorkbookSheets(request), nil
	}
	if !hasV2 {
		return nil, fmt.Errorf("Energy Path export projection does not belong to a submitted v2 result")
	}
	if err := validateBatchEnergyPathProjection(request); err != nil {
		return nil, fmt.Errorf("Energy Path export: %w", err)
	}
	sheets := batchEnergyPathPrimarySheets(request)
	if request.IncludeTraceSheets {
		sheets = append(sheets, batchEnergyPathTraceSheets(request)...)
		trace, err := batchEnergyPathJSONSection(request)
		if err != nil {
			return nil, err
		}
		sheets = append(sheets, tabular.WorkbookSheet{Name: "Energy Path JSON", Sections: []tabular.Section{trace}})
	}
	return sheets, nil
}

var batchEnergyPathStages = map[string]int{"drivers": 0, "loads": 1, "endUses": 2, "carriers": 3, "ratios": 4, "residuals": 5}

func validateBatchEnergyPathRows(status, reason string, rows []BatchEnergyPathExportRow) error {
	if status != "ready" && status != "unavailable" {
		return fmt.Errorf("invalid projection status %q", status)
	}
	if status == "unavailable" {
		if reason == "" || len(rows) != 0 {
			return fmt.Errorf("unavailable projection must explain its absence and contain no rows")
		}
		return nil
	}
	if reason != "" {
		return fmt.Errorf("ready projection cannot have an unavailable reason")
	}
	seen, previousStage, qualityStarted := map[string]bool{}, -1, false
	for _, row := range rows {
		rank, ok := batchEnergyPathStages[row.Stage]
		if !ok || row.Key == "" || seen[row.Key] || row.Category == "" || row.StageLabel == "" {
			return fmt.Errorf("invalid or duplicate projection row %q", row.Key)
		}
		if row.Kind != "summary" && row.Kind != "quality" {
			return fmt.Errorf("invalid row kind for %q", row.Key)
		}
		if rank < previousStage || (qualityStarted && row.Kind != "quality") || (row.Kind == "quality" && row.Stage != "residuals") {
			return fmt.Errorf("out-of-order projection row %q", row.Key)
		}
		seen[row.Key], previousStage = true, rank
		qualityStarted = qualityStarted || row.Kind == "quality"
		for _, side := range []BatchEnergyPathExportSide{row.Baseline, row.Target} {
			if (side.Missing || side.Ambiguous || side.Invalid) && side.Value != nil {
				return fmt.Errorf("unknown side has a numeric value for %q", row.Key)
			}
			for _, value := range []*float64{side.Value, side.Quality.Found, side.Quality.Total, side.Quality.Percent} {
				if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
					return fmt.Errorf("non-finite value for %q", row.Key)
				}
			}
		}
		for _, value := range []*float64{row.Delta, row.DeltaPercent} {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
				return fmt.Errorf("non-finite delta for %q", row.Key)
			}
		}
		if (row.Baseline.Value == nil || row.Target.Value == nil || row.UnitMismatch) && (row.Delta != nil || row.DeltaPercent != nil) {
			return fmt.Errorf("unavailable comparison has a numeric delta for %q", row.Key)
		}
	}
	return nil
}

func batchEnergyPathRowMap(rows []BatchEnergyPathExportRow) map[string]BatchEnergyPathExportRow {
	indexed := make(map[string]BatchEnergyPathExportRow, len(rows))
	for _, row := range rows {
		indexed[row.Key] = row
	}
	return indexed
}

// Validate only readiness metadata here. Category matching and nullable
// arithmetic remain owned by the shared browser comparator.
func batchEnergyPathSubmittedReadiness(request BatchSimulationXLSXExportRequest) ([][2]string, error) {
	data, err := batchEnergyPathSubmittedResult(request)
	if err != nil {
		return nil, err
	}
	var wire batchEnergyPathRawResult
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	statuses := make([][2]string, 0, len(wire.Results))
	for _, run := range wire.Results {
		summary := run.PurposeResults.EnergyExplanationSummary
		if len(summary) == 0 || string(summary) == "null" {
			var explanation struct {
				Summary json.RawMessage `json:"summary"`
			}
			if len(run.PurposeResults.EnergyExplanation) > 0 {
				if err := json.Unmarshal(run.PurposeResults.EnergyExplanation, &explanation); err != nil {
					return nil, err
				}
			}
			summary = explanation.Summary
		}
		status := [2]string{"unavailable", "missing_summary"}
		if len(summary) > 0 && string(summary) != "null" {
			var header struct {
				Schema string `json:"schema"`
				Period string `json:"period"`
				Scope  struct {
					Kind     string `json:"kind"`
					ZoneName string `json:"zoneName"`
				} `json:"scope"`
			}
			if err := json.Unmarshal(summary, &header); err != nil {
				return nil, err
			}
			status[1] = "unsupported_schema"
			if strings.EqualFold(header.Schema, "semantic-idf.energy-explanation-summary/v2") {
				status[1] = "building_annual_only"
				if strings.EqualFold(strings.TrimSpace(header.Scope.Kind), "building") && strings.EqualFold(strings.TrimSpace(header.Period), "annual") && strings.TrimSpace(header.Scope.ZoneName) == "" {
					status = [2]string{"ready", ""}
				}
			}
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func validateBatchEnergyPathProjection(request BatchSimulationXLSXExportRequest) error {
	projection := request.EnergyPath
	if projection.Schema != batchEnergyPathExportSchema || projection.Scope != "building" || projection.Period != "annual" {
		return fmt.Errorf("unsupported projection schema or scope; Building / Annual is required")
	}
	if len(projection.Runs) != len(request.Result.Results) {
		return fmt.Errorf("projection run count does not match submitted results")
	}
	readiness, err := batchEnergyPathSubmittedReadiness(request)
	if err != nil {
		return fmt.Errorf("read submitted summary metadata: %w", err)
	}
	if len(readiness) != len(projection.Runs) {
		return fmt.Errorf("submitted summary count does not match projection")
	}
	byID := map[string][]int{}
	for index, run := range projection.Runs {
		if run.ResultIndex != index || run.RowID != batchSimulationRowID(request.Result.Results[index]) {
			return fmt.Errorf("projection run %d does not match its submitted result index and row ID", index)
		}
		if err := validateBatchEnergyPathRows(run.Status, run.Reason, run.Rows); err != nil {
			return fmt.Errorf("run %d: %w", index, err)
		}
		if run.Status != readiness[index][0] || run.Reason != readiness[index][1] {
			return fmt.Errorf("run %d readiness does not match its submitted Building / Annual summary", index)
		}
		for _, row := range run.Rows {
			if !reflect.DeepEqual(row.Baseline, row.Target) {
				return fmt.Errorf("run %d contains a non-self comparison row %q", index, row.Key)
			}
		}
		byID[run.RowID] = append(byID[run.RowID], index)
	}
	comparison, requested := projection.Comparison, batchSimulationComparisonContext(request)
	if comparison.BaselineRowID != requested.BaselineRowID || comparison.TargetRowID != requested.TargetRowID {
		return fmt.Errorf("comparison selection does not match the export request")
	}
	if err := validateBatchEnergyPathRows(comparison.Status, comparison.Reason, comparison.Rows); err != nil {
		return err
	}
	left, right := byID[comparison.BaselineRowID], byID[comparison.TargetRowID]
	uniquePair := comparison.BaselineRowID != "" && comparison.TargetRowID != "" && comparison.BaselineRowID != comparison.TargetRowID && len(left) == 1 && len(right) == 1
	if comparison.Status == "unavailable" {
		if uniquePair && projection.Runs[left[0]].Status == "ready" && projection.Runs[right[0]].Status == "ready" {
			return fmt.Errorf("available selected runs cannot have an unavailable comparison")
		}
		return nil
	}
	if !uniquePair {
		return fmt.Errorf("comparison requires two distinct, uniquely identified submitted runs")
	}
	leftRun, rightRun := projection.Runs[left[0]], projection.Runs[right[0]]
	if leftRun.Status != "ready" || rightRun.Status != "ready" {
		return fmt.Errorf("comparison uses an unavailable run summary")
	}
	leftRows, rightRows := batchEnergyPathRowMap(leftRun.Rows), batchEnergyPathRowMap(rightRun.Rows)
	union := map[string]bool{}
	for key := range leftRows {
		union[key] = true
	}
	for key := range rightRows {
		union[key] = true
	}
	if len(comparison.Rows) != len(union) {
		return fmt.Errorf("comparison row count does not match the selected summaries")
	}
	// The shared comparator's union preserves each input's stable category
	// order. Check that property without reimplementing its taxonomy sorting.
	for _, original := range [][]BatchEnergyPathExportRow{leftRun.Rows, rightRun.Rows} {
		positions := map[string]int{}
		for index, row := range original {
			positions[row.Key] = index
		}
		last := -1
		for _, row := range comparison.Rows {
			if index, exists := positions[row.Key]; exists {
				if index <= last {
					return fmt.Errorf("comparison row order does not match its selected summaries")
				}
				last = index
			}
		}
	}
	for _, row := range comparison.Rows {
		if !union[row.Key] {
			return fmt.Errorf("comparison contains an unrelated row %q", row.Key)
		}
		for _, entry := range []struct {
			side BatchEnergyPathExportSide
			rows map[string]BatchEnergyPathExportRow
		}{{row.Baseline, leftRows}, {row.Target, rightRows}} {
			original, exists := entry.rows[row.Key]
			if exists {
				if row.Kind != original.Kind || row.Stage != original.Stage || row.Category != original.Category || row.CategoryID != original.CategoryID || row.Service != original.Service || !reflect.DeepEqual(entry.side, original.Baseline) {
					return fmt.Errorf("comparison row %q does not match its selected run", row.Key)
				}
			} else if !entry.side.Missing || entry.side.Value != nil || entry.side.Ambiguous || entry.side.Invalid {
				return fmt.Errorf("comparison invents a missing selected-run row %q", row.Key)
			}
		}
	}
	return nil
}

func batchEnergyPathNumber(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'g', -1, 64)
}

func batchEnergyPathWarnings(row BatchEnergyPathExportRow) string {
	var messages []string
	if row.BasisMismatch {
		messages = append(messages, "Not directly comparable: reporting or allocation basis differs")
	}
	if row.CoverageMismatch {
		messages = append(messages, "Source coverage differs")
	}
	if row.UnitMismatch {
		messages = append(messages, "Units or accounting domains differ; no delta is available")
	}
	if row.Baseline.Missing || row.Target.Missing {
		messages = append(messages, "A category is missing; no zero was assumed")
	}
	if row.Baseline.Ambiguous || row.Target.Ambiguous {
		messages = append(messages, "Ambiguous category; no value was selected")
	}
	if row.Baseline.Invalid || row.Target.Invalid {
		messages = append(messages, "Value or category is unavailable")
	}
	return strings.Join(messages, "; ")
}

func batchEnergyPathPrimarySheets(request BatchSimulationXLSXExportRequest) []tabular.WorkbookSheet {
	summary := tabular.Section{Title: "energy_path_summary", Headers: []string{"result_index", "file", "stage", "category", "value", "unit", "basis", "aggregation_basis", "missing", "ambiguous", "invalid", "quality_status", "quality_found", "quality_total", "quality_percent", "warnings", "warning_text"}}
	delta := tabular.Section{Title: "energy_path_delta", Headers: []string{"stage", "category", "baseline", "target", "unit", "delta", "delta_unit", "delta_percent", "baseline_basis", "target_basis", "baseline_aggregation_basis", "target_aggregation_basis", "basis_mismatch", "coverage_mismatch", "unit_mismatch", "directly_comparable", "warnings", "baseline_unit", "target_unit", "warning_text"}}
	quality := tabular.Section{Title: "data_quality", Headers: []string{"context", "result_index", "file", "stage", "category", "status", "reason", "baseline_status", "baseline_found", "baseline_total", "baseline_percent", "target_status", "target_found", "target_total", "target_percent", "baseline", "target", "unit", "delta", "delta_unit", "delta_percent", "warnings", "basis_mismatch", "coverage_mismatch", "unit_mismatch", "directly_comparable", "warning_text"}}
	runs := tabular.Section{Title: "runs", Headers: []string{"result_index", "file", "run_id", "status", "input_path", "energy_path_status", "energy_path_reason"}}
	qualityValues := func(side BatchEnergyPathExportSide) []string {
		return []string{side.Quality.Status, batchEnergyPathNumber(side.Quality.Found), batchEnergyPathNumber(side.Quality.Total), batchEnergyPathNumber(side.Quality.Percent)}
	}
	qualityRow := func(context, index, file, status, reason string, row BatchEnergyPathExportRow) []string {
		values := []string{context, index, file, row.StageLabel, row.Category, status, reason}
		values = append(values, qualityValues(row.Baseline)...)
		values = append(values, qualityValues(row.Target)...)
		return append(values, batchEnergyPathNumber(row.Baseline.Value), batchEnergyPathNumber(row.Target.Value), row.Unit, batchEnergyPathNumber(row.Delta), row.DeltaUnit, batchEnergyPathNumber(row.DeltaPercent), strings.Join(row.WarningCodes, "; "), strconv.FormatBool(row.BasisMismatch), strconv.FormatBool(row.CoverageMismatch), strconv.FormatBool(row.UnitMismatch), strconv.FormatBool(row.DirectlyComparable), batchEnergyPathWarnings(row))
	}
	for index, projected := range request.EnergyPath.Runs {
		run := request.Result.Results[index]
		number, file := strconv.Itoa(index), batchSimulationFileLabel(run)
		runs.Rows = append(runs.Rows, []string{number, file, run.RunID, run.Status, run.InputPath, projected.Status, projected.Reason})
		if projected.Status != "ready" {
			quality.Rows = append(quality.Rows, qualityRow("run", number, file, projected.Status, projected.Reason, BatchEnergyPathExportRow{}))
		}
		for _, row := range projected.Rows {
			if row.Kind == "quality" {
				quality.Rows = append(quality.Rows, qualityRow("run", number, file, projected.Status, "", row))
				continue
			}
			side := row.Baseline
			values := []string{number, file, row.StageLabel, row.Category, batchEnergyPathNumber(side.Value), side.Unit, side.Basis, side.AggregationBasis, strconv.FormatBool(side.Missing), strconv.FormatBool(side.Ambiguous), strconv.FormatBool(side.Invalid)}
			values = append(values, qualityValues(side)...)
			values = append(values, strings.Join(row.WarningCodes, "; "), batchEnergyPathWarnings(row))
			summary.Rows = append(summary.Rows, values)
		}
	}
	comparison := request.EnergyPath.Comparison
	if comparison.Status != "ready" {
		quality.Rows = append(quality.Rows, qualityRow("comparison", "", "", comparison.Status, comparison.Reason, BatchEnergyPathExportRow{}))
	}
	for _, row := range comparison.Rows {
		if row.Kind == "quality" {
			quality.Rows = append(quality.Rows, qualityRow("comparison", "", "", comparison.Status, "", row))
			continue
		}
		commonUnit := row.Unit
		if row.UnitMismatch {
			commonUnit = ""
		}
		delta.Rows = append(delta.Rows, []string{row.StageLabel, row.Category, batchEnergyPathNumber(row.Baseline.Value), batchEnergyPathNumber(row.Target.Value), commonUnit, batchEnergyPathNumber(row.Delta), row.DeltaUnit, batchEnergyPathNumber(row.DeltaPercent), row.Baseline.Basis, row.Target.Basis, row.Baseline.AggregationBasis, row.Target.AggregationBasis, strconv.FormatBool(row.BasisMismatch), strconv.FormatBool(row.CoverageMismatch), strconv.FormatBool(row.UnitMismatch), strconv.FormatBool(row.DirectlyComparable), strings.Join(row.WarningCodes, "; "), row.Baseline.Unit, row.Target.Unit, batchEnergyPathWarnings(row)})
	}
	metadata := tabular.Section{Title: "energy_path_context", Headers: []string{"field", "value"}, Rows: [][]string{{"scope", request.EnergyPath.Scope}, {"period", request.EnergyPath.Period}, {"baseline_row_id", comparison.BaselineRowID}, {"target_row_id", comparison.TargetRowID}, {"comparison_status", comparison.Status}, {"comparison_reason", comparison.Reason}}}
	runSections := []tabular.Section{runs, metadata, batchSimulationPurposeMetricSection(request.Result)}
	if context := batchSimulationRunContextSection(request); len(context.Rows) > 0 {
		runSections = append(runSections, context)
	}
	return []tabular.WorkbookSheet{{Name: "Energy Path Summary", Sections: []tabular.Section{summary}}, {Name: "Energy Path Delta", Sections: []tabular.Section{delta}}, {Name: "Data Quality", Sections: []tabular.Section{quality}}, {Name: "Runs", Sections: runSections}}
}

func batchEnergyPathTraceSheets(request BatchSimulationXLSXExportRequest) []tabular.WorkbookSheet {
	var sheets []tabular.WorkbookSheet
	for _, candidate := range []struct {
		name    string
		section tabular.Section
	}{
		{"Energy Nodes", batchSimulationEnergyNodeSection(request.Result)}, {"Energy Sources", batchSimulationEnergySourceSection(request.Result)},
		{"Source Availability", batchSimulationEnergySourceAvailabilitySection(request.Result)}, {"Energy Edges", batchSimulationEnergyEdgeSection(request.Result)},
		{"Reconciliation", batchSimulationEnergyReconciliationSection(request.Result)}, {"Energy Warnings", batchSimulationEnergyWarningSection(request.Result)},
	} {
		if len(candidate.section.Rows) > 0 {
			sheets = append(sheets, tabular.WorkbookSheet{Name: candidate.name, Sections: []tabular.Section{candidate.section}})
		}
	}
	comparison := request.EnergyPath.Comparison
	if comparison.Status == "ready" {
		left, right := -1, -1
		for index, run := range request.EnergyPath.Runs {
			if run.RowID == comparison.BaselineRowID {
				left = index
			}
			if run.RowID == comparison.TargetRowID {
				right = index
			}
		}
		if left >= 0 && right >= 0 {
			section := batchSimulationEnergyEdgeDeltaSection(request.Result.Results[left], request.Result.Results[right])
			if len(section.Rows) > 0 {
				sheets = append(sheets, tabular.WorkbookSheet{Name: "Energy Path Link Delta", Sections: []tabular.Section{section}})
			}
		}
	}
	return sheets
}

func batchEnergyPathJSONSection(request BatchSimulationXLSXExportRequest) (tabular.Section, error) {
	section := tabular.Section{Title: "energy_path_json", Headers: []string{"result_index", "file", "part", "representation", "chunk_index", "chunk_count", "json"}}
	data, err := batchEnergyPathSubmittedResult(request)
	if err != nil {
		return section, err
	}
	var wire batchEnergyPathRawResult
	if err = json.Unmarshal(data, &wire); err != nil {
		return section, err
	}
	representation := "canonical_typed_result"
	if len(request.rawResult) > 0 {
		representation = "original_submitted_json"
	}
	// 15,000 runes stay below Excel's 32,767 UTF-16-code-unit cell limit,
	// including non-BMP text. Chunking preserves the original JSON verbatim.
	const chunkSize = 15000
	for index, run := range wire.Results {
		for _, part := range []struct {
			name string
			data json.RawMessage
		}{{"energyExplanation", run.PurposeResults.EnergyExplanation}, {"energyExplanationSummary", run.PurposeResults.EnergyExplanationSummary}} {
			if len(part.data) == 0 {
				continue
			}
			runes := []rune(string(part.data))
			count := (len(runes) + chunkSize - 1) / chunkSize
			for chunk := 0; chunk < count; chunk++ {
				end := (chunk + 1) * chunkSize
				if end > len(runes) {
					end = len(runes)
				}
				file := ""
				if index < len(request.Result.Results) {
					file = batchSimulationFileLabel(request.Result.Results[index])
				}
				section.Rows = append(section.Rows, []string{strconv.Itoa(index), file, part.name, representation, strconv.Itoa(chunk), strconv.Itoa(count), string(runes[chunk*chunkSize : end])})
			}
		}
	}
	return section, nil
}
