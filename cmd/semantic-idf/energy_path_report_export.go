package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/tabular"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const energyPathReportSchema = "semantic-idf.energy-path-report/v2"

type EnergyPathReportXLSXExportRequest struct {
	Report             EnergyPathReportDTO `json:"report"`
	IncludeTraceSheets bool                `json:"includeTraceSheets"`
}

type EnergyPathReportContext struct {
	RunID          string `json:"runId"`
	Filename       string `json:"filename"`
	InputPath      string `json:"inputPath"`
	FinishedAt     string `json:"finishedAt"`
	ScopeKind      string `json:"scopeKind"`
	ZoneName       string `json:"zoneName"`
	Period         string `json:"period"`
	Service        string `json:"service"`
	SummaryService string `json:"summaryService"`
	ModelContext   string `json:"modelContext"`
}

type EnergyPathReportTrace struct {
	Nodes          []json.RawMessage `json:"nodes"`
	Links          []json.RawMessage `json:"links"`
	Sources        []json.RawMessage `json:"sources"`
	Reconciliation []json.RawMessage `json:"reconciliation"`
	Warnings       []json.RawMessage `json:"warnings"`
}

// Raw values deliberately bypass PurposeResultBundle.UnmarshalJSON: exporting
// the displayed snapshot must not normalize, repair, or reaggregate it.
type EnergyPathReportDTO struct {
	Schema      string                       `json:"schema"`
	Context     EnergyPathReportContext      `json:"context"`
	Summary     json.RawMessage              `json:"summary"`
	Quality     json.RawMessage              `json:"quality"`
	QualityRows []EnergyPathReportQualityRow `json:"qualityRows"`
	Trace       EnergyPathReportTrace        `json:"trace"`
	RawResult   string                       `json:"rawResult"`
}

type EnergyPathReportQualityRow struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Status  string   `json:"status"`
	Value   *float64 `json:"value"`
	Unit    string   `json:"unit"`
	Found   *float64 `json:"found"`
	Total   *float64 `json:"total"`
	Message string   `json:"message"`
}

func (a *App) SaveEnergyPathXLSX(request EnergyPathReportXLSXExportRequest) (*SaveFileResult, error) {
	sheets, err := buildEnergyPathReportWorkbook(request)
	if err != nil {
		return nil, err
	}
	if a.ctx == nil {
		return nil, fmt.Errorf("desktop runtime is not ready")
	}
	var out bytes.Buffer
	if err := tabular.WriteWorkbookXLSX(&out, sheets); err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(filepath.Base(request.Report.Context.Filename), filepath.Ext(request.Report.Context.Filename))
	if base == "" || base == "." {
		base = "simulation"
	}
	base = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '-'
		}
		return r
	}, base)
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{Title: "Save Energy Path workbook", DefaultFilename: base + "-energy-path.xlsx", Filters: []wailsruntime.FileFilter{{DisplayName: "Excel workbook (*.xlsx)", Pattern: "*.xlsx"}}})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return &SaveFileResult{Canceled: true}, nil
	}
	if err := validateEnergyPathReportOutputPath(request.Report, path); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, out.Bytes(), 0644); err != nil {
		return nil, err
	}
	return &SaveFileResult{Path: path, Filename: filepath.Base(path)}, nil
}

func validateEnergyPathReportOutputPath(report EnergyPathReportDTO, output string) error {
	target, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	raw := energyPathReportObject(json.RawMessage(report.RawResult))
	directory := energyPathReportText(raw["outputDirectory"])
	protected := []string{report.Context.InputPath, energyPathReportText(raw["inputPath"])}
	for _, entry := range energyPathReportRows(raw["files"]) {
		file := energyPathReportObject(entry)
		path := energyPathReportText(file["path"])
		if path == "" {
			path = energyPathReportText(file["name"])
		}
		if path != "" && !filepath.IsAbs(path) && directory != "" {
			path = filepath.Join(directory, path)
		}
		protected = append(protected, path)
	}
	if directory != "" {
		protected = append(protected, filepath.Join(directory, "semantic-idf-run.json"), filepath.Join(directory, "semantic-idf-run-plan.json"))
	}
	actual, actualErr := os.Stat(target)
	for _, path := range protected {
		if strings.TrimSpace(path) == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if filepath.Clean(absolute) == filepath.Clean(target) {
			return fmt.Errorf("Energy Path workbook cannot overwrite an input or simulation artifact")
		}
		original, err := os.Stat(absolute)
		if actualErr == nil && err == nil && os.SameFile(actual, original) {
			return fmt.Errorf("Energy Path workbook cannot overwrite an alias of an input or simulation artifact")
		}
	}
	return nil
}

type energyPathRawObject map[string]json.RawMessage

func energyPathReportObject(raw json.RawMessage) energyPathRawObject {
	var object energyPathRawObject
	_ = json.Unmarshal(raw, &object)
	return object
}

func energyPathReportText(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func energyPathReportRows(raw json.RawMessage) []json.RawMessage {
	var rows []json.RawMessage
	_ = json.Unmarshal(raw, &rows)
	return rows
}

func energyPathReportNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null"
}

// Match the UI's fallback semantics without replacing explicit empty objects.
func energyPathReportFirstPresent(values ...json.RawMessage) json.RawMessage {
	for _, raw := range values {
		if energyPathReportNull(raw) {
			continue
		}
		value := bytes.TrimSpace(raw)
		if string(value) == "false" || string(value) == `""` {
			continue
		}
		if len(value) > 0 && value[0] != '"' && value[0] != '{' && value[0] != '[' && string(value) != "true" && energyPathReportNumber(value) == "0" {
			continue
		}
		return raw
	}
	return nil
}

func energyPathReportEqual(left, right json.RawMessage) bool {
	decode := func(raw json.RawMessage) any {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			return nil
		}
		return value
	}
	return reflect.DeepEqual(decode(left), decode(right))
}

func energyPathReportToken(raw json.RawMessage) string {
	return strings.ToLower(strings.TrimSpace(energyPathReportText(raw)))
}

func energyPathReportPeriodContext(report EnergyPathReportDTO) (energyPathRawObject, energyPathRawObject, energyPathRawObject, error) {
	raw := energyPathReportObject(json.RawMessage(report.RawResult))
	if raw == nil {
		return nil, nil, nil, fmt.Errorf("Energy Path report requires the original result JSON object")
	}
	for _, field := range []struct{ key, value string }{{"runId", report.Context.RunID}, {"filename", report.Context.Filename}, {"inputPath", report.Context.InputPath}, {"finishedAt", report.Context.FinishedAt}} {
		if energyPathReportText(raw[field.key]) != field.value {
			return nil, nil, nil, fmt.Errorf("Energy Path snapshot %s does not match its context", field.key)
		}
	}
	purpose := energyPathReportObject(raw["purposeResults"])
	explanation := energyPathReportObject(purpose["energyExplanation"])
	if energyPathReportText(explanation["schema"]) != "semantic-idf.energy-explanation/v2" {
		return nil, nil, nil, fmt.Errorf("Energy Path export requires a canonical v2 snapshot")
	}
	selected := explanation
	if report.Context.ScopeKind == "zone" {
		matches := 0
		for _, candidate := range append([]json.RawMessage{purpose["energyExplanation"]}, energyPathReportRows(explanation["zoneResults"])...) {
			object := energyPathReportObject(candidate)
			scope := energyPathReportObject(object["scope"])
			if energyPathReportToken(scope["kind"]) == "zone" && strings.EqualFold(strings.TrimSpace(energyPathReportText(scope["zoneName"])), strings.TrimSpace(report.Context.ZoneName)) {
				selected = object
				matches++
			}
		}
		if matches != 1 {
			return nil, nil, nil, fmt.Errorf("Energy Path Zone is missing or ambiguous")
		}
	} else if energyPathReportToken(energyPathReportObject(selected["scope"])["kind"]) != "building" {
		return nil, nil, nil, fmt.Errorf("Energy Path Building scope is unavailable")
	}
	var period energyPathRawObject
	matches := 0
	for _, candidate := range energyPathReportRows(selected["periods"]) {
		object := energyPathReportObject(candidate)
		if strings.EqualFold(energyPathReportText(object["id"]), report.Context.Period) {
			period = object
			matches++
		}
	}
	if matches > 1 {
		return nil, nil, nil, fmt.Errorf("Energy Path period is ambiguous")
	}
	if report.Context.Period != "annual" && period == nil {
		nodes := energyPathReportRows(selected["nodes"])
		exact := len(nodes) > 0
		for _, node := range nodes {
			exact = exact && strings.EqualFold(energyPathReportText(energyPathReportObject(node)["period"]), report.Context.Period)
		}
		if !exact {
			return nil, nil, nil, fmt.Errorf("Energy Path period is unavailable; annual fallback is forbidden")
		}
		period = selected
	}
	return explanation, selected, period, nil
}

func validateEnergyPathReport(report EnergyPathReportDTO) error {
	if report.Schema != energyPathReportSchema {
		return fmt.Errorf("unsupported Energy Path report schema")
	}
	c := report.Context
	if (c.ScopeKind != "building" && c.ScopeKind != "zone") || (c.ScopeKind == "zone" && strings.TrimSpace(c.ZoneName) == "") || (c.ScopeKind == "building" && c.ZoneName != "") {
		return fmt.Errorf("invalid Energy Path scope")
	}
	if c.SummaryService != "all" || c.ModelContext != "simulation_result_snapshot" || (c.Service != "all" && c.Service != "cooling" && c.Service != "heating") {
		return fmt.Errorf("invalid Energy Path snapshot context")
	}
	if c.Period != "annual" {
		month, err := strconv.Atoi(strings.TrimPrefix(c.Period, "M"))
		if err != nil || month < 1 || month > 12 || c.Period != fmt.Sprintf("M%d", month) {
			return fmt.Errorf("invalid Energy Path period")
		}
	}
	explanation, selected, period, err := energyPathReportPeriodContext(report)
	if err != nil {
		return err
	}
	graph := selected
	if period != nil && (c.Period != "annual" || len(energyPathReportRows(selected["nodes"])) == 0 && len(energyPathReportRows(selected["links"])) == 0) {
		graph = period
	}
	for _, field := range []struct {
		key      string
		actual   []json.RawMessage
		expected []json.RawMessage
	}{{"nodes", report.Trace.Nodes, energyPathReportRows(graph["nodes"])}, {"links", report.Trace.Links, energyPathReportRows(graph["links"])}, {"sources", report.Trace.Sources, energyPathReportRows(explanation["sources"])}, {"reconciliation", report.Trace.Reconciliation, energyPathReportRows(graph["reconciliation"])}, {"warnings", report.Trace.Warnings, energyPathReportRows(graph["warnings"])}} {
		if len(field.actual) != len(field.expected) {
			return fmt.Errorf("Energy Path trace %s does not match the selected snapshot", field.key)
		}
		for index, row := range field.actual {
			if !energyPathReportEqual(row, field.expected[index]) {
				return fmt.Errorf("Energy Path trace %s row %d was changed", field.key, index)
			}
		}
	}
	ids := map[string]bool{}
	for _, raw := range report.Trace.Nodes {
		node := energyPathReportObject(raw)
		id := energyPathReportText(node["id"])
		if id == "" || ids[id] {
			return fmt.Errorf("Energy Path node identity is empty or ambiguous")
		}
		ids[id] = true
		if p := energyPathReportText(node["period"]); p != "" && !strings.EqualFold(p, c.Period) {
			return fmt.Errorf("Energy Path node period mismatch")
		}
		if zone := energyPathReportText(node["zoneName"]); c.ScopeKind == "zone" && zone != "" && !strings.EqualFold(zone, c.ZoneName) {
			return fmt.Errorf("Energy Path node Zone mismatch")
		}
	}
	linkIDs := map[string]bool{}
	for _, raw := range report.Trace.Links {
		link := energyPathReportObject(raw)
		id := energyPathReportText(link["id"])
		if id == "" || linkIDs[id] || !ids[energyPathReportText(link["fromId"])] || !ids[energyPathReportText(link["toId"])] {
			return fmt.Errorf("Energy Path link identity or endpoints are invalid")
		}
		linkIDs[id] = true
		if p := energyPathReportText(link["period"]); p != "" && !strings.EqualFold(p, c.Period) {
			return fmt.Errorf("Energy Path link period mismatch")
		}
		if zone := energyPathReportText(link["zoneName"]); c.ScopeKind == "zone" && zone != "" && !strings.EqualFold(zone, c.ZoneName) {
			return fmt.Errorf("Energy Path link Zone mismatch")
		}
	}
	purpose := energyPathReportObject(energyPathReportObject(json.RawMessage(report.RawResult))["purposeResults"])
	canonicalSummary := energyPathReportFirstPresent(selected["summary"], purpose["energyExplanationSummary"])
	if c.Period != "annual" {
		canonicalSummary = period["summary"]
	}
	base := energyPathReportObject(canonicalSummary)
	validSummary := strings.EqualFold(energyPathReportText(base["schema"]), "semantic-idf.energy-explanation-summary/v2")
	scope := energyPathReportObject(base["scope"])
	if energyPathReportText(scope["kind"]) == "" {
		scope = energyPathReportObject(selected["scope"])
	}
	if kind := energyPathReportToken(scope["kind"]); kind != "" && kind != c.ScopeKind {
		validSummary = false
	}
	if c.ScopeKind == "zone" && !strings.EqualFold(strings.TrimSpace(energyPathReportText(scope["zoneName"])), strings.TrimSpace(c.ZoneName)) {
		validSummary = false
	}
	if p := energyPathReportText(base["period"]); p != "" && !strings.EqualFold(p, c.Period) {
		validSummary = false
	}
	if energyPathReportNull(report.Summary) {
		if validSummary {
			return fmt.Errorf("a valid selected Energy Path summary cannot be omitted")
		}
	} else {
		if !validSummary {
			return fmt.Errorf("Energy Path summary is unavailable for the selected context")
		}
		actual := energyPathReportObject(report.Summary)
		if !strings.EqualFold(energyPathReportText(actual["schema"]), "semantic-idf.energy-explanation-summary/v2") || energyPathReportText(actual["period"]) != c.Period {
			return fmt.Errorf("Energy Path summary context was changed")
		}
		if !energyPathReportEqual(actual["scope"], mustEnergyPathReportJSON(scope)) {
			return fmt.Errorf("Energy Path summary scope was changed")
		}
		for _, key := range []string{"drivers", "loads", "endUses", "carriers", "ratios", "residuals", "topZones"} {
			rows, original := energyPathReportRows(actual[key]), energyPathReportRows(base[key])
			if c.ScopeKind == "building" && len(rows) != len(original) {
				return fmt.Errorf("Energy Path %s rows were omitted or added", key)
			}
			cursor := 0
			for _, row := range rows {
				for cursor < len(original) && !energyPathReportEqual(row, original[cursor]) {
					cursor++
				}
				if cursor == len(original) {
					return fmt.Errorf("Energy Path %s row differs from its selected summary", key)
				}
				cursor++
			}
		}
	}
	// Quality is a field-wise selection of stored run/local quality, not a
	// recomputation. Permit only exact reported fields or unknown placeholders.
	runSummary := energyPathReportObject(selected["summary"])
	runRaw := energyPathReportFirstPresent(selected["quality"], runSummary["quality"])
	localRaw := energyPathReportFirstPresent(selected["quality"], period["quality"], runSummary["quality"])
	if c.Period != "annual" {
		localRaw = energyPathReportFirstPresent(period["quality"], energyPathReportObject(period["summary"])["quality"])
	}
	local, run := energyPathReportObject(localRaw), energyPathReportObject(runRaw)
	quality := energyPathReportObject(report.Quality)
	for _, key := range []string{"drivers", "loads", "endUses", "carriers", "ratios"} {
		expected := energyPathReportFirstPresent(local[key])
		if energyPathReportNull(expected) && key != "ratios" {
			expected = energyPathReportFirstPresent(run[key])
		}
		if energyPathReportNull(expected) {
			expected = json.RawMessage(`{"status":"unavailable","found":0,"total":0}`)
		}
		if !energyPathReportEqual(quality[key], expected) {
			return fmt.Errorf("Energy Path quality %s does not match stored availability", key)
		}
	}
	for _, key := range []string{"driverToLoadClosedPct", "endUseToCarrierClosedPct", "zoneAllocatedPct", "unassignedPct"} {
		value := energyPathReportNumber(local[key])
		if strings.HasPrefix(value, "-") {
			value = ""
		}
		actual := energyPathReportNumber(quality[key])
		if value != actual {
			return fmt.Errorf("Energy Path quality %s was changed", key)
		}
	}
	for _, pair := range []struct {
		status string
		values []string
	}{{"driverToLoadStatus", []string{"driverToLoadClosedPct"}}, {"endUseToCarrierStatus", []string{"endUseToCarrierClosedPct"}}, {"zoneAllocationStatus", []string{"zoneAllocatedPct", "unassignedPct"}}} {
		expected := energyPathReportText(local[pair.status])
		if expected == "" {
			expected = "unavailable"
		}
		if expected == "complete" || expected == "partial" || expected == "overmapped" {
			for _, field := range pair.values {
				if energyPathReportNumber(quality[field]) == "" {
					expected = "unavailable"
				}
			}
		}
		if energyPathReportText(quality[pair.status]) != expected {
			return fmt.Errorf("Energy Path quality status was changed")
		}
	}
	return validateEnergyPathReportQualityRows(report)
}

func mustEnergyPathReportJSON(value any) json.RawMessage { data, _ := json.Marshal(value); return data }

func energyPathReportPointer(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'g', -1, 64)
}

func validateEnergyPathReportQualityRows(report EnergyPathReportDTO) error {
	keys := []string{"drivers", "loads", "endUses", "carriers", "ratios", "driver_to_load_closed_pct", "end_use_to_carrier_closed_pct", "zone_allocated_pct", "unassigned_pct"}
	fields := []string{"driverToLoadClosedPct", "endUseToCarrierClosedPct", "zoneAllocatedPct", "unassignedPct"}
	statuses := []string{"driverToLoadStatus", "endUseToCarrierStatus", "zoneAllocationStatus", "zoneAllocationStatus"}
	if len(report.QualityRows) != len(keys) {
		return fmt.Errorf("Energy Path quality display rows are incomplete")
	}
	quality := energyPathReportObject(report.Quality)
	for i, row := range report.QualityRows {
		if row.Key != keys[i] {
			return fmt.Errorf("Energy Path quality row order or identity is invalid")
		}
		for _, number := range []*float64{row.Value, row.Found, row.Total} {
			if number != nil && (math.IsNaN(*number) || math.IsInf(*number, 0) || *number < 0) {
				return fmt.Errorf("Energy Path quality display contains an invalid value")
			}
		}
		if i < 5 {
			level := energyPathReportObject(quality[row.Key])
			status := energyPathReportToken(level["status"])
			if status == "" {
				status = "unavailable"
			}
			if row.Status != status || row.Value != nil || row.Unit != "" {
				return fmt.Errorf("Energy Path stage quality display was changed")
			}
			total, found := energyPathReportNumber(level["total"]), energyPathReportNumber(level["found"])
			if total == "" || total == "0" || strings.HasPrefix(total, "-") || found == "" || strings.HasPrefix(found, "-") || status == "unavailable" || status == "not_requested" || status == "not_applicable" {
				total, found = "", ""
			}
			if energyPathReportPointer(row.Found) != found || energyPathReportPointer(row.Total) != total {
				return fmt.Errorf("Energy Path stage quality counts were changed")
			}
		} else {
			field, statusField := fields[i-5], statuses[i-5]
			status := energyPathReportToken(quality[statusField])
			if status == "" {
				status = "unavailable"
			}
			value := energyPathReportNumber(quality[field])
			if status != "complete" && status != "partial" && status != "overmapped" {
				value = ""
			}
			if row.Key == "end_use_to_carrier_closed_pct" && row.Status == "unavailable" && row.Value == nil {
				value = ""
				status = "unavailable"
			}
			if row.Status != status || energyPathReportPointer(row.Value) != value || row.Unit != "%" || row.Found != nil || row.Total != nil {
				return fmt.Errorf("Energy Path boundary quality display was changed")
			}
		}
	}
	return nil
}

// Missing/null/blank/boolean/nonfinite values stay blank. Only representation
// formatting occurs here; there is no energy matching or aggregation.
func energyPathReportNumber(raw json.RawMessage) string {
	if energyPathReportNull(raw) {
		return ""
	}
	value := strings.TrimSpace(string(raw))
	if len(value) > 0 && value[0] == '"' {
		value = strings.TrimSpace(energyPathReportText(raw))
	}
	if value == "" {
		return ""
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return ""
	}
	return strconv.FormatFloat(number, 'g', -1, 64)
}

func energyPathReportBasis(raw json.RawMessage) string {
	basis := energyPathReportText(raw)
	normalized := strings.ToLower(strings.TrimSpace(basis))
	if strings.Contains(normalized, "unassigned") {
		return "Unassigned · " + basis
	}
	if strings.Contains(normalized, "allocat") || strings.Contains(normalized, "load_share") {
		return "Allocated · " + basis
	}
	if normalized == "direct_zone_energy" || normalized == "reported_variable" || normalized == "reported_meter" || normalized == "integrated_rate" {
		return "Direct / reported · " + basis
	}
	if basis == "" {
		return "Unavailable"
	}
	return basis
}

func buildEnergyPathReportWorkbook(request EnergyPathReportXLSXExportRequest) ([]tabular.WorkbookSheet, error) {
	if err := validateEnergyPathReport(request.Report); err != nil {
		return nil, err
	}
	report := request.Report
	c := report.Context
	context := tabular.Section{Title: "Scope / period", Headers: []string{"Field", "Value"}, Rows: [][]string{{"Scope", c.ScopeKind}, {"Zone", c.ZoneName}, {"Period", c.Period}, {"Selected service", c.Service}, {"Summary service", "all"}, {"Model context", "Saved simulation result snapshot; current editor changes are not included."}, {"Input", c.Filename}, {"Finished", c.FinishedAt}}}
	sections := []tabular.Section{context}
	summary := energyPathReportObject(report.Summary)
	if energyPathReportNull(report.Summary) {
		sections = append(sections, tabular.Section{Title: "Summary unavailable", Headers: []string{"Status"}, Rows: [][]string{{"No valid summary is available for the selected scope and period; values are not assumed to be zero."}}})
	}
	for _, group := range []struct{ key, label string }{{"drivers", "Drivers"}, {"loads", "Loads"}, {"endUses", "End uses"}, {"carriers", "Carriers"}, {"ratios", "Ratios"}, {"residuals", "Residuals"}} {
		section := tabular.Section{Title: group.label, Headers: []string{"Category", "Value", "Unit", "Basis", "Aggregation basis", "Service"}}
		for _, raw := range energyPathReportRows(summary[group.key]) {
			row := energyPathReportObject(raw)
			section.Rows = append(section.Rows, []string{energyPathReportText(row["label"]), energyPathReportNumber(row["value"]), energyPathReportText(row["unit"]), energyPathReportBasis(row["basis"]), energyPathReportText(row["aggregationBasis"]), energyPathReportText(row["serviceKind"])})
		}
		sections = append(sections, section)
	}
	q := tabular.Section{Title: "Quality", Headers: []string{"Metric", "Value", "Unit", "Status", "Found", "Total", "Notes"}}
	for _, row := range report.QualityRows {
		q.Rows = append(q.Rows, []string{row.Label, energyPathReportPointer(row.Value), row.Unit, row.Status, energyPathReportPointer(row.Found), energyPathReportPointer(row.Total), row.Message})
	}
	sections = append(sections, q)
	accounting := energyPathReportAccountingSection(report.Trace.Reconciliation, false)
	if c.ScopeKind == "zone" {
		sections = append(sections, accounting)
	}
	sheets := []tabular.WorkbookSheet{{Name: "Energy Path", Sections: sections}}
	if request.IncludeTraceSheets {
		sheets = append(sheets, energyPathReportTraceSheets(report)...)
	}
	return sheets, nil
}

func energyPathReportAccountingSection(rows []json.RawMessage, trace bool) tabular.Section {
	section := tabular.Section{Title: "Zone accounting context", Headers: []string{"Category", "Period", "Zone", "Service", "Basis", "Method", "Direct", "Allocated", "Unassigned", "Expected", "Unit", "Status"}}
	if trace {
		section.Headers = append(section.Headers, "ID", "Source IDs")
	}
	for _, raw := range rows {
		item := energyPathReportObject(raw)
		if !trace && energyPathReportText(item["level"]) != "allocation" {
			continue
		}
		row := []string{energyPathReportText(item["label"]), energyPathReportText(item["period"]), energyPathReportText(item["zoneName"]), energyPathReportText(item["serviceKind"]), energyPathReportText(item["basis"]), energyPathReportText(item["allocationMethod"]), energyPathReportNumber(item["directValue"]), energyPathReportNumber(item["allocatedValue"]), energyPathReportNumber(item["unassignedValue"]), energyPathReportNumber(item["expectedValue"]), energyPathReportText(item["unit"]), energyPathReportText(item["status"])}
		if trace {
			row = append(row, energyPathReportText(item["id"]), string(item["sourceIds"]))
		}
		section.Rows = append(section.Rows, row)
	}
	return section
}

func energyPathReportTraceSheets(report EnergyPathReportDTO) []tabular.WorkbookSheet {
	nodes := tabular.Section{Title: "Node trace", Headers: []string{"ID", "Level", "Category", "Value", "Raw", "Effective", "Allocated", "Unit", "Basis", "Allocation applied", "Period", "Zone", "Service", "Source IDs"}}
	for _, raw := range report.Trace.Nodes {
		n := energyPathReportObject(raw)
		nodes.Rows = append(nodes.Rows, []string{energyPathReportText(n["id"]), energyPathReportText(n["level"]), energyPathReportText(n["label"]), energyPathReportNumber(n["value"]), energyPathReportNumber(n["rawValue"]), energyPathReportNumber(n["effectiveValue"]), energyPathReportNumber(n["allocatedValue"]), energyPathReportText(n["unit"]), energyPathReportText(n["basis"]), string(n["allocationApplied"]), energyPathReportText(n["period"]), energyPathReportText(n["zoneName"]), energyPathReportText(n["serviceKind"]), string(n["sourceIds"])})
	}
	sources := tabular.Section{Title: "Source trace", Headers: []string{"ID", "Name", "Type", "Key", "Source unit", "Normalized unit", "Frequency", "Raw", "Effective", "Allocated", "Aggregation basis"}}
	for _, raw := range report.Trace.Sources {
		s := energyPathReportObject(raw)
		sources.Rows = append(sources.Rows, []string{energyPathReportText(s["id"]), energyPathReportText(s["name"]), energyPathReportText(s["sourceType"]), energyPathReportText(s["keyValue"]), energyPathReportText(s["sourceUnit"]), energyPathReportText(s["normalizedUnit"]), energyPathReportText(s["reportingFrequency"]), energyPathReportNumber(s["rawValue"]), energyPathReportNumber(s["effectiveValue"]), energyPathReportNumber(s["allocatedValue"]), energyPathReportText(s["aggregationBasis"])})
	}
	links := tabular.Section{Title: "Link trace", Headers: []string{"ID", "From ID", "To ID", "Relation", "From value", "From unit", "To value", "To unit", "Ratio", "Ratio kind", "Basis", "Period", "Zone", "Service", "Source IDs"}}
	for _, raw := range report.Trace.Links {
		l := energyPathReportObject(raw)
		links.Rows = append(links.Rows, []string{energyPathReportText(l["id"]), energyPathReportText(l["fromId"]), energyPathReportText(l["toId"]), energyPathReportText(l["relation"]), energyPathReportNumber(l["fromValue"]), energyPathReportText(l["fromUnit"]), energyPathReportNumber(l["toValue"]), energyPathReportText(l["toUnit"]), energyPathReportNumber(l["ratio"]), energyPathReportText(l["ratioKind"]), energyPathReportText(l["basis"]), energyPathReportText(l["period"]), energyPathReportText(l["zoneName"]), energyPathReportText(l["serviceKind"]), string(l["sourceIds"])})
	}
	warnings := tabular.Section{Title: "Warning trace", Headers: []string{"Severity", "Code", "Period", "Message"}}
	for _, raw := range report.Trace.Warnings {
		w := energyPathReportObject(raw)
		warnings.Rows = append(warnings.Rows, []string{energyPathReportText(w["severity"]), energyPathReportText(w["code"]), energyPathReportText(w["period"]), energyPathReportText(w["message"])})
	}
	original := tabular.Section{Title: "Original result JSON", Headers: []string{"Chunk", "Chunks", "JSON"}}
	runes := []rune(report.RawResult)
	const size = 15000
	count := (len(runes) + size - 1) / size
	for i := 0; i < count; i++ {
		end := (i + 1) * size
		if end > len(runes) {
			end = len(runes)
		}
		original.Rows = append(original.Rows, []string{strconv.Itoa(i), strconv.Itoa(count), string(runes[i*size : end])})
	}
	return []tabular.WorkbookSheet{{Name: "Energy Path Nodes", Sections: []tabular.Section{nodes}}, {Name: "Energy Path Sources", Sections: []tabular.Section{sources}}, {Name: "Energy Path Links", Sections: []tabular.Section{links}}, {Name: "Energy Path Accounting", Sections: []tabular.Section{energyPathReportAccountingSection(report.Trace.Reconciliation, true), warnings}}, {Name: "Energy Path JSON", Sections: []tabular.Section{original}}}
}
