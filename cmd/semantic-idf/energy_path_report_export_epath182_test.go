package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/tabular"
)

func epath182BackendFixture(t *testing.T) EnergyPathReportXLSXExportRequest {
	t.Helper()
	quality := json.RawMessage(`{"drivers":{"status":"partial","found":0,"total":2},"loads":{"status":"complete","found":2,"total":2},"endUses":{"status":"unavailable","found":0,"total":0},"carriers":{"status":"not_requested","found":0,"total":0},"ratios":{"status":"unavailable","found":0,"total":0},"driverToLoadClosedPct":0,"endUseToCarrierClosedPct":null,"zoneAllocatedPct":null,"unassignedPct":null,"driverToLoadStatus":"partial","endUseToCarrierStatus":"unavailable","zoneAllocationStatus":"unavailable"}`)
	summary := json.RawMessage(`{"schema":"semantic-idf.energy-explanation-summary/v2","period":"annual","scope":{"kind":"building"},"drivers":[{"id":"PRIVATE_DRIVER","label":"Exterior walls","value":0,"rawValue":0,"unit":"kWh thermal","basis":"heat_balance_share"}],"loads":[{"id":"PRIVATE_LOAD","label":"Cooling load","value":400,"unit":"kWh thermal","basis":"reported_variable","serviceKind":"cooling"}],"endUses":[{"id":"PRIVATE_END_USE","label":"Cooling","value":100,"unit":"kWh","basis":"reported_meter"},{"id":"PRIVATE_UNKNOWN","label":"Unknown use","value":null,"unit":"kWh","basis":"reported_meter"}],"carriers":[],"ratios":[],"residuals":[]}`)
	trace := EnergyPathReportTrace{
		Nodes:          []json.RawMessage{json.RawMessage(`{"id":"PRIVATE_LOAD","level":"load","period":"annual","value":400,"rawValue":null,"unit":"kWh thermal","unknownNodeMetadata":{"keep":null}}`), json.RawMessage(`{"id":"PRIVATE_END_USE","level":"end_use","period":"annual","value":100,"unit":"kWh"}`)},
		Links:          []json.RawMessage{json.RawMessage(`{"id":"PRIVATE_LINK","fromId":"PRIVATE_LOAD","toId":"PRIVATE_END_USE","relation":"load_to_end_use","fromValue":400,"fromUnit":"kWh thermal","toValue":100,"toUnit":"kWh","ratio":4,"ratioKind":"cop","period":"annual","sourceIds":["PRIVATE_SOURCE"],"unknownLink":null}`)},
		Sources:        []json.RawMessage{json.RawMessage(`{"id":"PRIVATE_SOURCE","name":"Cooling meter","sourceType":"sql_meter","sourceUnit":"J","normalizedUnit":"kWh","rawValue":null,"effectiveValue":0,"unknownSource":false}`)},
		Reconciliation: []json.RawMessage{json.RawMessage(`{"id":"PRIVATE_ACCOUNTING","level":"allocation","label":"Zone HVAC coverage","period":"annual","directValue":0,"allocatedValue":10,"unassignedValue":null,"expectedValue":10,"unit":"kWh","basis":"by_service_path_load_share","status":"partial"}`)},
		Warnings:       []json.RawMessage{json.RawMessage(`{"code":"PRIVATE_WARNING","severity":"warning","message":"Unknown accounting context"}`)},
	}
	graph := map[string]any{"schema": "semantic-idf.energy-explanation/v2", "scope": map[string]string{"kind": "building"}, "nodes": trace.Nodes, "links": trace.Links, "sources": trace.Sources, "reconciliation": trace.Reconciliation, "warnings": trace.Warnings, "quality": quality}
	raw := map[string]any{"runId": "run182", "filename": "saved model.idf", "inputPath": "C:/saved/model.idf", "finishedAt": "2026-01-01T00:00:00Z", "purposeResults": map[string]any{"energyExplanation": graph, "energyExplanationSummary": summary}, "unknownOriginalField": strings.Repeat("한😀<&>\r\n", 6500)}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	rows := []EnergyPathReportQualityRow{}
	for _, stage := range []struct {
		key, status  string
		found, total *float64
	}{{"drivers", "partial", epath171Number(0), epath171Number(2)}, {"loads", "complete", epath171Number(2), epath171Number(2)}, {"endUses", "unavailable", nil, nil}, {"carriers", "not_requested", nil, nil}, {"ratios", "unavailable", nil, nil}} {
		rows = append(rows, EnergyPathReportQualityRow{Key: stage.key, Label: stage.key, Status: stage.status, Found: stage.found, Total: stage.total})
	}
	for _, metric := range []struct {
		key, status string
		value       *float64
	}{{"driver_to_load_closed_pct", "partial", epath171Number(0)}, {"end_use_to_carrier_closed_pct", "unavailable", nil}, {"zone_allocated_pct", "unavailable", nil}, {"unassigned_pct", "unavailable", nil}} {
		rows = append(rows, EnergyPathReportQualityRow{Key: metric.key, Label: metric.key, Status: metric.status, Value: metric.value, Unit: "%"})
	}
	return EnergyPathReportXLSXExportRequest{Report: EnergyPathReportDTO{Schema: energyPathReportSchema, Context: EnergyPathReportContext{RunID: "run182", Filename: "saved model.idf", InputPath: "C:/saved/model.idf", FinishedAt: "2026-01-01T00:00:00Z", ScopeKind: "building", Period: "annual", Service: "cooling", SummaryService: "all", ModelContext: "simulation_result_snapshot"}, Summary: summary, Quality: quality, QualityRows: rows, Trace: trace, RawResult: strings.ReplaceAll(string(data), "\n", "\r\n")}}
}

func TestEPATH182BackendSummaryFirstAndNullableWirePreserved(t *testing.T) {
	request := epath182BackendFixture(t)
	before, _ := json.Marshal(request)
	sheets, err := buildEnergyPathReportWorkbook(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 1 || sheets[0].Name != "Energy Path" {
		t.Fatalf("default sheets: %+v", sheets)
	}
	titles := []string{}
	for _, section := range sheets[0].Sections {
		titles = append(titles, section.Title)
	}
	if !reflect.DeepEqual(titles, []string{"Scope / period", "Drivers", "Loads", "End uses", "Carriers", "Ratios", "Residuals", "Quality"}) {
		t.Fatalf("summary not first: %v", titles)
	}
	if sheets[0].Sections[1].Rows[0][1] != "0" || sheets[0].Sections[3].Rows[1][1] != "" {
		t.Fatal("real zero and absent energy were conflated")
	}
	quality := sheets[0].Sections[7]
	if quality.Rows[0][4] != "0" || quality.Rows[5][1] != "0" || quality.Rows[6][1] != "" {
		t.Fatalf("nullable quality changed: %v", quality.Rows)
	}
	primary, _ := json.Marshal(sheets)
	if strings.Contains(string(primary), "PRIVATE_") {
		t.Fatal("trace identifiers leaked into first sheet")
	}
	after, _ := json.Marshal(request)
	if string(before) != string(after) {
		t.Fatal("formatter mutated raw result or nullable fields")
	}
}

func TestEPATH182BackendTraceXLSXPreservesUnitsDualValuesAndOriginalJSON(t *testing.T) {
	request := epath182BackendFixture(t)
	request.IncludeTraceSheets = true
	sheets, err := buildEnergyPathReportWorkbook(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 6 {
		t.Fatalf("trace sheet count=%d", len(sheets))
	}
	source := sheets[2].Sections[0].Rows[0]
	if source[4] != "J" || source[5] != "kWh" || source[7] != "" || source[8] != "0" {
		t.Fatalf("source units/null/zero lost: %v", source)
	}
	link := sheets[3].Sections[0].Rows[0]
	if !reflect.DeepEqual(link[4:10], []string{"400", "kWh thermal", "100", "kWh", "4", "cop"}) {
		t.Fatalf("dual link values changed: %v", link)
	}
	accounting := sheets[4].Sections[0].Rows[0]
	if accounting[6] != "0" || accounting[7] != "10" || accounting[8] != "" {
		t.Fatalf("direct/allocated/unassigned collapsed: %v", accounting)
	}
	var rebuilt strings.Builder
	for _, row := range sheets[5].Sections[0].Rows {
		if len(utf16.Encode([]rune(row[2]))) > 32767 {
			t.Fatal("original JSON chunk exceeds Excel cell limit")
		}
		rebuilt.WriteString(row[2])
	}
	if rebuilt.String() != request.Report.RawResult {
		t.Fatal("original nullable/unknown JSON altered")
	}
	var out bytes.Buffer
	if err := tabular.WriteWorkbookXLSX(&out, sheets); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if file.Name != "xl/worksheets/sheet6.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		var sheet struct {
			Rows []struct {
				Cells []struct {
					Text string `xml:"is>t"`
				} `xml:"c"`
			} `xml:"sheetData>row"`
		}
		if err := xml.Unmarshal(encoded, &sheet); err != nil {
			t.Fatal(err)
		}
		rebuilt.Reset()
		for _, row := range sheet.Rows[2:] {
			if len(row.Cells) == 3 {
				rebuilt.WriteString(row.Cells[2].Text)
			}
		}
		if rebuilt.String() != request.Report.RawResult {
			t.Fatal("actual XLSX XML normalized CRLF/non-BMP/null JSON")
		}
		return
	}
	t.Fatal("missing original JSON XLSX sheet")
}

func TestEPATH182BackendRejectsChangedSnapshotsBeforeSaveDialog(t *testing.T) {
	mutations := map[string]func(*EnergyPathReportXLSXExportRequest){
		"run": func(r *EnergyPathReportXLSXExportRequest) { r.Report.Context.RunID = "other" },
		"scope": func(r *EnergyPathReportXLSXExportRequest) {
			r.Report.Context.ScopeKind = "zone"
			r.Report.Context.ZoneName = "Lab"
		},
		"missing month":         func(r *EnergyPathReportXLSXExportRequest) { r.Report.Context.Period = "M2" },
		"graph service summary": func(r *EnergyPathReportXLSXExportRequest) { r.Report.Context.SummaryService = "cooling" },
		"summary number": func(r *EnergyPathReportXLSXExportRequest) {
			r.Report.Summary = bytes.Replace(r.Report.Summary, []byte(`"value":400`), []byte(`"value":800`), 1)
		},
		"summary omitted": func(r *EnergyPathReportXLSXExportRequest) { r.Report.Summary = json.RawMessage("null") },
		"trace omitted":   func(r *EnergyPathReportXLSXExportRequest) { r.Report.Trace.Sources = nil },
		"trace unknown field changed": func(r *EnergyPathReportXLSXExportRequest) {
			r.Report.Trace.Sources[0] = bytes.Replace(r.Report.Trace.Sources[0], []byte(`"unknownSource":false`), []byte(`"unknownSource":true`), 1)
		},
		"quality known zero": func(r *EnergyPathReportXLSXExportRequest) { r.Report.QualityRows[5].Value = epath171Number(100) },
		"quality count":      func(r *EnergyPathReportXLSXExportRequest) { r.Report.QualityRows[0].Found = epath171Number(2) },
		"quality key":        func(r *EnergyPathReportXLSXExportRequest) { r.Report.QualityRows[0].Key = "carriers" },
		"quality status": func(r *EnergyPathReportXLSXExportRequest) {
			r.Report.Quality = bytes.Replace(r.Report.Quality, []byte(`"driverToLoadStatus":"partial"`), []byte(`"driverToLoadStatus":"complete"`), 1)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			request := epath182BackendFixture(t)
			mutate(&request)
			if _, err := buildEnergyPathReportWorkbook(request); err == nil {
				t.Fatal("changed report accepted")
			}
			if _, err := (&App{}).SaveEnergyPathXLSX(request); err == nil || strings.Contains(err.Error(), "desktop runtime") {
				t.Fatalf("invalid report reached save dialog: %v", err)
			}
		})
	}
}

func TestEPATH182BackendUnavailableSummaryNeverSynthesizesZeros(t *testing.T) {
	request := epath182BackendFixture(t)
	raw := energyPathReportObject(json.RawMessage(request.Report.RawResult))
	purpose := energyPathReportObject(raw["purposeResults"])
	purpose["energyExplanationSummary"] = json.RawMessage(`{}`)
	raw["purposeResults"] = mustEnergyPathReportJSON(purpose)
	request.Report.RawResult = string(mustEnergyPathReportJSON(raw))
	request.Report.Summary = json.RawMessage("null")
	sheets, err := buildEnergyPathReportWorkbook(request)
	if err != nil {
		t.Fatal(err)
	}
	if sheets[0].Sections[1].Title != "Summary unavailable" {
		t.Fatal("missing explicit unavailable section")
	}
	for _, section := range sheets[0].Sections {
		if section.Title == "Drivers" || section.Title == "Loads" || section.Title == "Carriers" {
			if len(section.Rows) != 0 {
				t.Fatal("unavailable summary synthesized values")
			}
		}
	}
}

func TestEPATH182BackendExplicitEmptyQualityDoesNotBorrowSummaryCoverage(t *testing.T) {
	request := epath182BackendFixture(t)
	raw := energyPathReportObject(json.RawMessage(request.Report.RawResult))
	purpose := energyPathReportObject(raw["purposeResults"])
	graph := energyPathReportObject(purpose["energyExplanation"])
	graph["quality"] = json.RawMessage(`{}`)
	summary := energyPathReportObject(request.Report.Summary)
	summary["quality"] = request.Report.Quality
	request.Report.Summary = mustEnergyPathReportJSON(summary)
	purpose["energyExplanation"] = mustEnergyPathReportJSON(graph)
	purpose["energyExplanationSummary"] = request.Report.Summary
	raw["purposeResults"] = mustEnergyPathReportJSON(purpose)
	request.Report.RawResult = string(mustEnergyPathReportJSON(raw))
	unknown := map[string]any{}
	for i := range request.Report.QualityRows {
		row := &request.Report.QualityRows[i]
		row.Status = "unavailable"
		row.Value = nil
		row.Found = nil
		row.Total = nil
		if i < 5 {
			unknown[row.Key] = map[string]any{"status": "unavailable", "found": 0, "total": 0}
		}
	}
	for _, key := range []string{"driverToLoadClosedPct", "endUseToCarrierClosedPct", "zoneAllocatedPct", "unassignedPct"} {
		unknown[key] = nil
	}
	for _, key := range []string{"driverToLoadStatus", "endUseToCarrierStatus", "zoneAllocationStatus"} {
		unknown[key] = "unavailable"
	}
	request.Report.Quality = mustEnergyPathReportJSON(unknown)
	sheets, err := buildEnergyPathReportWorkbook(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range sheets[0].Sections[7].Rows {
		if row[1] != "" || row[4] != "" || row[5] != "" {
			t.Fatalf("empty quality resurrected coverage: %v", row)
		}
	}
}

func TestEPATH182BackendSaveProtectsOriginalInputsAndArtifactAliases(t *testing.T) {
	request := epath182BackendFixture(t)
	dir := t.TempDir()
	input, sqlPath := filepath.Join(dir, "model.idf"), filepath.Join(dir, "eplusout.sql")
	for _, path := range []string{input, sqlPath, filepath.Join(dir, "semantic-idf-run.json"), filepath.Join(dir, "semantic-idf-run-plan.json")} {
		if err := os.WriteFile(path, []byte("original bytes"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	request.Report.Context.InputPath = input
	raw := energyPathReportObject(json.RawMessage(request.Report.RawResult))
	raw["inputPath"] = mustEnergyPathReportJSON(input)
	raw["outputDirectory"] = mustEnergyPathReportJSON(dir)
	raw["files"] = json.RawMessage(`[{"name":"eplusout.sql","kind":"sqlite"}]`)
	request.Report.RawResult = string(mustEnergyPathReportJSON(raw))
	for _, path := range []string{input, sqlPath, filepath.Join(dir, "semantic-idf-run.json"), filepath.Join(dir, "semantic-idf-run-plan.json")} {
		if err := validateEnergyPathReportOutputPath(request.Report, path); err == nil {
			t.Fatalf("protected target accepted: %s", path)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "original bytes" {
			t.Fatal("collision guard changed input")
		}
	}
	alias := filepath.Join(dir, "alias.xlsx")
	if err := os.Link(sqlPath, alias); err != nil {
		t.Fatal(err)
	}
	if err := validateEnergyPathReportOutputPath(request.Report, alias); err == nil {
		t.Fatal("hardlinked SQL could be overwritten")
	}
	if err := validateEnergyPathReportOutputPath(request.Report, filepath.Join(dir, "safe-report.xlsx")); err != nil {
		t.Fatal(err)
	}
}

func TestEPATH182BatchV2NameAndFirstContextDoNotChangeV1Golden(t *testing.T) {
	request := epath171BackendRequest()
	sheets, err := buildBatchSimulationExportWorkbook(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 4 || sheets[0].Sections[0].Title != "energy_path_context" || sheets[0].Sections[1].Title != "energy_path_summary" {
		t.Fatal("Batch first sheet lacks scope/period summary context")
	}
	if !reflect.DeepEqual(sheets[0].Sections[0].Rows[:2], [][]string{{"scope", "building"}, {"period", "annual"}}) {
		t.Fatal("Batch scope/period changed")
	}
	// The compatibility writer is intentionally untouched until EPATH222.
	section := batchSimulationEnergyEdgeDeltaSection(request.Result.Results[0], request.Result.Results[1])
	if section.Title != "sankey_edge_delta" {
		t.Fatal("v1 compatibility trace title changed prematurely")
	}
}
