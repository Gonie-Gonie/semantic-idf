package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/tabular"
)

func epath171Number(value float64) *float64 { return &value }

func epath171SummarySection(t *testing.T, sheets []tabular.WorkbookSheet) tabular.Section {
	t.Helper()
	for _, section := range sheets[0].Sections {
		if section.Title == "energy_path_summary" {
			return section
		}
	}
	t.Fatal("Energy Path Summary sheet has no summary section")
	return tabular.Section{}
}

// The browser integration test exercises actual comparator output. This small
// fixture isolates backend transport validation and ZIP formatting only.
func epath171BackendRequest() BatchSimulationXLSXExportRequest {
	side := func(value float64) BatchEnergyPathExportSide {
		valid := true
		return BatchEnergyPathExportSide{Value: epath171Number(value), Unit: "kWh", Basis: "reported_variable", AggregationBasis: "model_total", UnitValid: &valid, ScaleDomain: "thermal", UnitIdentity: `["kwh","thermal"]`, Quality: BatchEnergyPathExportQuality{Status: "complete", Found: epath171Number(2), Total: epath171Number(2), Percent: epath171Number(100)}}
	}
	row := func(value float64) BatchEnergyPathExportRow {
		return BatchEnergyPathExportRow{Key: `["loads","cooling",""]`, Kind: "summary", Stage: "loads", StageLabel: "Loads", Category: "Cooling load", CategoryID: "cooling", Service: "cooling", Baseline: side(value), Target: side(value), Unit: "kWh", DeltaUnit: "kWh", Delta: epath171Number(0), DirectlyComparable: true}
	}
	request := BatchSimulationXLSXExportRequest{Comparison: BatchSimulationComparisonXLSXContext{BaselineRowID: "left", TargetRowID: "right"}, Result: simulation.MultiSimulationResult{RunID: "batch", Results: []simulation.SimulationRunResult{}}}
	projection := &BatchEnergyPathExportProjection{Schema: batchEnergyPathExportSchema, Scope: "building", Period: "annual"}
	for index, id := range []string{"left", "right"} {
		request.Result.Results = append(request.Result.Results, simulation.SimulationRunResult{RunID: id, Filename: id + ".idf", Status: "completed", PurposeResults: &simulation.PurposeResultBundle{EnergyExplanation: simulation.EnergyExplanationResult{Schema: "semantic-idf.energy-explanation/v2"}, EnergyExplanationSummary: simulation.EnergyExplanationSummary{Schema: "semantic-idf.energy-explanation-summary/v2", Period: "annual", Scope: simulation.EnergyExplanationScope{Kind: "building"}}}})
		projection.Runs = append(projection.Runs, BatchEnergyPathExportRun{ResultIndex: index, RowID: id, Status: "ready", Rows: []BatchEnergyPathExportRow{row(float64(index) * 10)}})
	}
	delta := row(0)
	delta.Target = side(10)
	delta.Delta = epath171Number(10)
	projection.Comparison = BatchEnergyPathExportComparison{BaselineRowID: "left", TargetRowID: "right", Status: "ready", Rows: []BatchEnergyPathExportRow{delta}}
	request.EnergyPath = projection
	return request
}

func TestEPATH171BackendDefaultWorkbookAndNulls(t *testing.T) {
	request := epath171BackendRequest()
	before, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	sheets, err := buildBatchSimulationExportWorkbook(request)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Energy Path Summary", "Energy Path Delta", "Data Quality", "Runs"}
	var names []string
	for _, sheet := range sheets {
		names = append(names, sheet.Name)
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("default sheets=%v", names)
	}
	if epath171SummarySection(t, sheets).Rows[0][4] != "0" || sheets[1].Sections[0].Rows[0][2] != "0" || sheets[1].Sections[0].Rows[0][7] != "" {
		t.Fatalf("reported zero or unavailable percent changed: %v", sheets[1].Sections[0].Rows)
	}
	var workbook bytes.Buffer
	if err := tabular.WriteWorkbookXLSX(&workbook, sheets); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(workbook.Bytes()), int64(workbook.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var workbookXML string
	for _, file := range reader.File {
		if file.Name == "xl/workbook.xml" {
			stream, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(stream)
			stream.Close()
			if err != nil {
				t.Fatal(err)
			}
			workbookXML = string(data)
		}
	}
	for _, name := range want {
		if !strings.Contains(workbookXML, `name="`+name+`"`) {
			t.Fatalf("actual XLSX missing %s", name)
		}
	}
	after, _ := json.Marshal(request)
	if !bytes.Equal(before, after) {
		t.Fatal("formatter mutated submitted data")
	}
	request.EnergyPath.Runs[1].Rows[0].Baseline.Value = nil
	request.EnergyPath.Runs[1].Rows[0].Baseline.Invalid = true
	request.EnergyPath.Runs[1].Rows[0].Target = request.EnergyPath.Runs[1].Rows[0].Baseline
	request.EnergyPath.Runs[1].Rows[0].Delta = nil
	request.EnergyPath.Comparison.Rows[0].Target = request.EnergyPath.Runs[1].Rows[0].Baseline
	request.EnergyPath.Comparison.Rows[0].Delta = nil
	sheets, err = buildBatchSimulationExportWorkbook(request)
	if err != nil {
		t.Fatal(err)
	}
	if epath171SummarySection(t, sheets).Rows[1][4] != "" || sheets[1].Sections[0].Rows[0][3] != "" || sheets[1].Sections[0].Rows[0][5] != "" {
		t.Fatal("null resurrected as zero")
	}
}

func TestEPATH171BackendRejectsUnboundProjectionBeforeSave(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*BatchSimulationXLSXExportRequest)
	}{
		{"missing projection", func(r *BatchSimulationXLSXExportRequest) { r.EnergyPath = nil }},
		{"monthly", func(r *BatchSimulationXLSXExportRequest) { r.EnergyPath.Period = "M1" }},
		{"zone", func(r *BatchSimulationXLSXExportRequest) { r.EnergyPath.Scope = "zone" }},
		{"wrong index", func(r *BatchSimulationXLSXExportRequest) { r.EnergyPath.Runs[1].ResultIndex = 0 }},
		{"wrong row ID", func(r *BatchSimulationXLSXExportRequest) { r.EnergyPath.Runs[1].RowID = "stale" }},
		{"missing run", func(r *BatchSimulationXLSXExportRequest) { r.EnergyPath.Runs = r.EnergyPath.Runs[:1] }},
		{"duplicate semantic row", func(r *BatchSimulationXLSXExportRequest) {
			r.EnergyPath.Runs[0].Rows = append(r.EnergyPath.Runs[0].Rows, r.EnergyPath.Runs[0].Rows[0])
		}},
		{"wrong comparison side", func(r *BatchSimulationXLSXExportRequest) {
			r.EnergyPath.Comparison.Rows[0].Target.Value = epath171Number(999)
		}},
		{"missing comparison row", func(r *BatchSimulationXLSXExportRequest) { r.EnergyPath.Comparison.Rows = nil }},
		{"invented selection", func(r *BatchSimulationXLSXExportRequest) { r.EnergyPath.Comparison.TargetRowID = "stale" }},
		{"unknown stage", func(r *BatchSimulationXLSXExportRequest) { r.EnergyPath.Runs[0].Rows[0].Stage = "sources" }},
		{"nan", func(r *BatchSimulationXLSXExportRequest) {
			r.EnergyPath.Comparison.Rows[0].Delta = epath171Number(math.NaN())
		}},
		{"infinity", func(r *BatchSimulationXLSXExportRequest) {
			r.EnergyPath.Runs[0].Rows[0].Baseline.Quality.Percent = epath171Number(math.Inf(1))
		}},
		{"unknown numeric", func(r *BatchSimulationXLSXExportRequest) { r.EnergyPath.Runs[0].Rows[0].Baseline.Missing = true }},
		{"unavailable with rows", func(r *BatchSimulationXLSXExportRequest) {
			r.EnergyPath.Comparison.Status = "unavailable"
			r.EnergyPath.Comparison.Reason = "missing_summary"
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := epath171BackendRequest()
			test.mutate(&request)
			if sheets, err := buildBatchSimulationExportWorkbook(request); err == nil || sheets != nil {
				t.Fatalf("invalid request accepted: %v", err)
			}
			if _, err := (&App{}).SaveBatchSimulationXLSX(request); err == nil || strings.Contains(err.Error(), "desktop runtime") {
				t.Fatalf("validation did not precede desktop dialog: %v", err)
			}
		})
	}
}

func TestEPATH171BackendDuplicateRunsAndUnavailableSelection(t *testing.T) {
	request := epath171BackendRequest()
	request.Result.Results = append(request.Result.Results, request.Result.Results[1])
	duplicate := request.EnergyPath.Runs[1]
	duplicate.ResultIndex = 2
	request.EnergyPath.Runs = append(request.EnergyPath.Runs, duplicate)
	if _, err := buildBatchSimulationExportWorkbook(request); err == nil {
		t.Fatal("ambiguous selected ID accepted")
	}
	request.EnergyPath.Comparison.Status = "unavailable"
	request.EnergyPath.Comparison.Reason = "ambiguous_comparison"
	request.EnergyPath.Comparison.Rows = nil
	sheets, err := buildBatchSimulationExportWorkbook(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 4 || len(epath171SummarySection(t, sheets).Rows) != 3 || len(sheets[1].Sections[0].Rows) != 0 {
		t.Fatal("duplicate result-index-bound summaries discarded or fallback pair invented")
	}
	if !strings.Contains(strings.Join(sheets[2].Sections[0].Rows[len(sheets[2].Sections[0].Rows)-1], "|"), "ambiguous_comparison") {
		t.Fatal("unavailable comparison reason absent")
	}
}

func TestEPATH171BackendRawTracePreservesUnknownNullAndUnicode(t *testing.T) {
	request := epath171BackendRequest()
	request.IncludeTraceSheets = true
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(wire["result"], &result); err != nil {
		t.Fatal(err)
	}
	var runs []map[string]json.RawMessage
	if err := json.Unmarshal(result["results"], &runs); err != nil {
		t.Fatal(err)
	}
	var purposes map[string]json.RawMessage
	if err := json.Unmarshal(runs[0]["purposeResults"], &purposes); err != nil {
		t.Fatal(err)
	}
	original := `{ "schema": "semantic-idf.energy-explanation-summary/v2", "period":"annual", "scope":{"kind":"building"}, "loads":[{"id":"private-summary-id","value":null}], "unknownFutureTrace": "` + strings.Repeat("😀가", 17000) + `" }`
	purposes["energyExplanationSummary"] = json.RawMessage(original)
	runs[0]["purposeResults"], _ = json.Marshal(purposes)
	result["results"], _ = json.Marshal(runs)
	wire["result"], _ = json.Marshal(result)
	encoded, _ = json.Marshal(wire)
	// encoding/json compacts embedded RawMessage during outer marshal. Capture
	// exactly what crossed the request boundary, not the pre-serialization text.
	var sent batchEnergyPathRawResult
	if err := json.Unmarshal(wire["result"], &sent); err != nil {
		t.Fatal(err)
	}
	want := string(sent.Results[0].PurposeResults.EnergyExplanationSummary)
	var decoded BatchSimulationXLSXExportRequest
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	section, err := batchEnergyPathJSONSection(decoded)
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for _, row := range section.Rows {
		if row[0] == "0" && row[2] == "energyExplanationSummary" {
			if row[3] != "original_submitted_json" {
				t.Fatal("original representation lost")
			}
			if len(utf16.Encode([]rune(row[6]))) > 32767 {
				t.Fatal("Excel cell limit exceeded")
			}
			parts = append(parts, row[6])
		}
	}
	if len(parts) < 2 || strings.Join(parts, "") != want {
		t.Fatal("raw trace did not reproduce exact original JSON")
	}
	if !strings.Contains(want, `"value":null`) || !strings.Contains(want, "unknownFutureTrace") {
		t.Fatal("fixture lost nullable/unknown fields")
	}
	var workbook bytes.Buffer
	if err := tabular.WriteWorkbookXLSX(&workbook, []tabular.WorkbookSheet{{Name: "Energy Path JSON", Sections: []tabular.Section{section}}}); err != nil {
		t.Fatal(err)
	}
	if workbook.Len() == 0 {
		t.Fatal("raw trace workbook empty")
	}
}

func TestEPATH171BackendLegacyRequestUsesEstablishedReadAdapter(t *testing.T) {
	legacy := representativeEnergyPathV1BatchExportRequest()
	sheets, err := buildBatchSimulationExportWorkbook(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sheets, batchSimulationWorkbookSheets(legacy)) {
		t.Fatal("direct genuine V1 golden path changed")
	}
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	// The runtime writer intentionally emits v2. A stored genuine v1 request
	// has v1 headers and passes through the preexisting read adapter instead.
	payload = bytes.ReplaceAll(payload, []byte("semantic-idf.energy-explanation/v2"), []byte("semantic-idf.energy-explanation/v1"))
	payload = bytes.ReplaceAll(payload, []byte("semantic-idf.energy-explanation-summary/v2"), []byte("semantic-idf.energy-explanation-summary/v1"))
	var decoded BatchSimulationXLSXExportRequest
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	sheets, err = buildBatchSimulationExportWorkbook(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sheets, batchSimulationWorkbookSheets(decoded)) {
		t.Fatal("preexisting V1 read-adapter export behavior changed")
	}
}

func TestEPATH171BackendReadinessUsesOriginalSummaryPrecedence(t *testing.T) {
	ready := `{"schema":"SEMANTIC-IDF.ENERGY-EXPLANATION-SUMMARY/V2","period":"ANNUAL","scope":{"kind":"BUILDING"}}`
	for _, test := range []struct{ name, top, nested, status, reason string }{
		{"ready", ready, "null", "ready", ""},
		{"null falls back", "null", ready, "ready", ""},
		{"empty object does not fall back", `{}`, ready, "unavailable", "unsupported_schema"},
		{"missing", `null`, `null`, "unavailable", "missing_summary"},
		{"monthly", strings.ReplaceAll(ready, "ANNUAL", "M1"), ready, "unavailable", "building_annual_only"},
		{"zone", strings.ReplaceAll(ready, "BUILDING", "ZONE"), ready, "unavailable", "building_annual_only"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := epath171BackendRequest()
			run := `{"purposeResults":{"energyExplanation":{"schema":"semantic-idf.energy-explanation/v2","summary":` + test.nested + `},"energyExplanationSummary":` + test.top + `}}`
			request.rawResult = []byte(`{"results":[` + run + `,` + run + `]}`)
			readiness, err := batchEnergyPathSubmittedReadiness(request)
			if err != nil {
				t.Fatal(err)
			}
			if readiness[0] != [2]string{test.status, test.reason} {
				t.Fatalf("readiness=%v", readiness)
			}
			if test.status == "unavailable" {
				if _, err := buildBatchSimulationExportWorkbook(request); err == nil {
					t.Fatal("stale ready projection accepted")
				}
			}
		})
	}
}

func TestEPATH171BackendUnitMismatchAndAlwaysPresentContext(t *testing.T) {
	request := epath171BackendRequest()
	run := &request.EnergyPath.Runs[1].Rows[0]
	run.Baseline.Unit = "MJ"
	run.Baseline.UnitIdentity = `["mj","thermal"]`
	run.Target = run.Baseline
	delta := &request.EnergyPath.Comparison.Rows[0]
	delta.Target = run.Baseline
	delta.Unit = "MJ"
	delta.UnitMismatch = true
	delta.Delta = nil
	delta.DeltaPercent = nil
	delta.DirectlyComparable = false
	delta.WarningCodes = []string{"unit_or_domain_mismatch"}
	sheets, err := buildBatchSimulationExportWorkbook(request)
	if err != nil {
		t.Fatal(err)
	}
	row := sheets[1].Sections[0].Rows[0]
	if row[4] != "" || row[17] != "kWh" || row[18] != "MJ" || !strings.Contains(row[19], "no delta") {
		t.Fatalf("mismatched numeric units mislabeled: %v", row)
	}
	context := sheets[3].Sections[1]
	if context.Title != "energy_path_context" || !reflect.DeepEqual(context.Rows[:4], [][]string{{"scope", "building"}, {"period", "annual"}, {"baseline_row_id", "left"}, {"target_row_id", "right"}}) {
		t.Fatalf("scope or comparison context missing: %v", context)
	}
}

func TestEPATH171BackendSameStageOrderIsBoundToSummaries(t *testing.T) {
	request := epath171BackendRequest()
	for index := range request.EnergyPath.Runs {
		row := request.EnergyPath.Runs[index].Rows[0]
		row.Key = `["loads","heating",""]`
		row.Category = "Heating load"
		row.CategoryID = "heating"
		row.Service = "heating"
		request.EnergyPath.Runs[index].Rows = append(request.EnergyPath.Runs[index].Rows, row)
	}
	row := request.EnergyPath.Comparison.Rows[0]
	row.Key = `["loads","heating",""]`
	row.Category = "Heating load"
	row.CategoryID = "heating"
	row.Service = "heating"
	request.EnergyPath.Comparison.Rows = append(request.EnergyPath.Comparison.Rows, row)
	if _, err := buildBatchSimulationExportWorkbook(request); err != nil {
		t.Fatal(err)
	}
	request.EnergyPath.Comparison.Rows[0], request.EnergyPath.Comparison.Rows[1] = request.EnergyPath.Comparison.Rows[1], request.EnergyPath.Comparison.Rows[0]
	if _, err := buildBatchSimulationExportWorkbook(request); err == nil {
		t.Fatal("reordered same-stage delta rows accepted")
	}
}
