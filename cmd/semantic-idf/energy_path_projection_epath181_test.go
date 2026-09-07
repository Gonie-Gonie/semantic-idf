package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	appcli "github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/cli"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

// The Python stdio client launches this test executable as a real CLI process.
// This narrowly gated test-only dispatcher avoids building a second Wails app;
// the command parser, loader, projection, formatter and exit code are production.
func init() {
	if os.Getenv("EPATH181_CLI_HELPER") != "1" {
		return
	}
	if len(os.Args) < 2 || os.Args[1] != "energy-path" {
		os.Exit(97)
	}
	handled, code := appcli.MaybeRun(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, "epath181-test")
	if !handled {
		os.Exit(98)
	}
	os.Exit(code)
}

func TestEPATH181StoredSQLMatchesGUIAppHTTPCLIAndPython(t *testing.T) {
	runDirectory, sqlPath, inputPath := epath181CreateStoredRun(t)
	before := epath181FileSnapshot(t, runDirectory)
	file, err := os.Stat(sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	run := simulation.SimulationRunResult{InputPath: inputPath, OutputDirectory: runDirectory,
		Files: []simulation.SimulationFileInfo{{Name: filepath.Base(sqlPath), Path: sqlPath, Kind: "sqlite", Size: file.Size()}}}
	gui := simulation.BuildPurposeResultBundle(&run, simulation.SimulationPurposeRequest{
		Purposes: []simulation.SimulationPurposeID{simulation.SimulationPurposeBasicEnergy}, BasicEnergyDetail: simulation.PurposeBasicEnergyDetailEnergyPath,
	})
	guiBytes := epath181JSON(t, gui)
	if gui.EnergyExplanation.Schema != "semantic-idf.energy-explanation/v2" || len(gui.EnergyExplanation.Links) == 0 || len(gui.EnergyExplanation.ZoneResults) == 0 {
		t.Fatalf("actual stored SQL fixture must yield a canonical graph and scoped Zone result: %+v", gui.EnergyExplanation)
	}
	app := NewApp()
	app.rememberSimulationResult("existing user document", &simulation.SimulationRunResult{RunID: "existing-user-run", Status: "completed"})
	cacheBefore, err := app.GetCachedSimulationResult(analysisTextHash("existing user document"), "existing-user-run")
	if err != nil {
		t.Fatal(err)
	}
	requestSequenceBefore := app.simulationWorkspaceCache.requestSequence
	server := httptest.NewServer(appAssetHandler(app))
	defer server.Close()
	type expectedCase struct {
		Request map[string]any  `json:"request"`
		JSON    json.RawMessage `json:"json"`
		CSV     string          `json:"csv"`
		Trace   string          `json:"trace"`
	}
	cases := []expectedCase{
		{Request: map[string]any{"resultPath": runDirectory, "inputPath": inputPath, "scope": "building", "period": "annual", "service": "all"}},
		{Request: map[string]any{"resultPath": sqlPath, "inputPath": inputPath, "scope": "building", "period": "M1", "service": "cooling"}},
		{Request: map[string]any{"resultPath": runDirectory, "inputPath": inputPath, "scope": "zone", "zone": "Office", "period": "M2", "service": "all"}},
	}
	for index := range cases {
		item := &cases[index]
		var request simulation.EnergyPathProjectionRequest
		if err := json.Unmarshal(epath181JSON(t, item.Request), &request); err != nil {
			t.Fatal(err)
		}
		projection, err := app.LoadEnergyPath(request)
		if err != nil {
			t.Fatalf("App.LoadEnergyPath case %d: %v", index, err)
		}
		item.JSON = epath181JSON(t, projection)
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(item.JSON, &envelope); err != nil {
			t.Fatal(err)
		}
		epath181EqualJSON(t, fmt.Sprintf("case %d GUI purposeResults", index), guiBytes, envelope["purposeResults"])
		epath181AssertCanonicalQuality(t, gui.EnergyExplanation, item.Request, projection.View.Quality)
		epath181AssertSelectedView(t, item.JSON, item.Request)
		item.CSV, err = simulation.EnergyPathProjectionCSV(projection, false)
		if err != nil {
			t.Fatal(err)
		}
		item.Trace, err = simulation.EnergyPathProjectionCSV(projection, true)
		if err != nil {
			t.Fatal(err)
		}
		epath181AssertSummaryAndTraceCSV(t, gui.EnergyExplanation, projection, item.CSV, item.Trace)
		for _, format := range []string{"json", "csv"} {
			for _, trace := range []bool{false, true} {
				body := map[string]any{}
				for key, value := range item.Request {
					body[key] = value
				}
				body["format"], body["includeTrace"] = format, trace
				response := epath181HTTP(t, server.URL, http.MethodPost, epath181JSON(t, body))
				if response.Code != http.StatusOK {
					t.Fatalf("HTTP case %d %s trace=%t: %d %s", index, format, trace, response.Code, response.Body)
				}
				args := []string{"energy-path", item.Request["resultPath"].(string), "--input", inputPath, "--scope", item.Request["scope"].(string), "--period", item.Request["period"].(string), "--service", item.Request["service"].(string), "--format", format}
				if zone, _ := item.Request["zone"].(string); zone != "" {
					args = append(args, "--zone", zone)
				}
				if trace {
					args = append(args, "--include-trace")
				}
				var stdout, stderr bytes.Buffer
				handled, code := appcli.MaybeRun(args, strings.NewReader(""), &stdout, &stderr, "epath181-test")
				if !handled || code != 0 {
					t.Fatalf("actual CLI case %d: handled=%t code=%d stderr=%s", index, handled, code, stderr.String())
				}
				if format == "json" {
					epath181EqualJSON(t, "HTTP JSON", item.JSON, response.Body.Bytes())
					epath181EqualJSON(t, "CLI JSON", item.JSON, stdout.Bytes())
				} else {
					want := item.CSV
					if trace {
						want = item.Trace
					}
					if response.Body.String() != want || stdout.String() != want {
						t.Fatalf("CSV bytes differ across shared formatter/HTTP/CLI case %d trace=%t", index, trace)
					}
					if !strings.HasPrefix(response.Header().Get("Content-Type"), "text/csv") {
						t.Fatalf("CSV HTTP content type=%s", response.Header().Get("Content-Type"))
					}
				}
			}
		}
	}
	t.Run("checked-in Python API and actual stdio subprocess", func(t *testing.T) {
		python := epath181Python(t)
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		module, err := filepath.Abs(filepath.Join("..", "..", "clients", "python", "semantic_idf_client.py"))
		if err != nil {
			t.Fatal(err)
		}
		// Register the external client as a Go test-cache dependency as well as
		// importing and executing it in Python below.
		moduleBytes, err := os.ReadFile(module)
		if err != nil || len(moduleBytes) == 0 {
			t.Fatalf("checked-in Python client unavailable: %v", err)
		}
		input := map[string]any{"url": server.URL, "executable": executable, "module": module, "cases": cases}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, python, "-X", "utf8", "-c", epath181PythonCrossSurface)
		command.Env = append(os.Environ(), "EPATH181_CLI_HELPER=1", "PYTHONDONTWRITEBYTECODE=1")
		command.Stdin = bytes.NewReader(epath181JSON(t, input))
		output, err := command.CombinedOutput()
		if err != nil || strings.TrimSpace(string(output)) != "EPATH181 Python HTTP/stdio PASS" {
			t.Fatalf("checked-in Python client parity failed: %v\n%s", err, output)
		}
	})
	if !reflect.DeepEqual(before, epath181FileSnapshot(t, runDirectory)) {
		t.Fatal("read-only projection changed the saved SQL/IDF or created run output files")
	}
	cacheAfter, err := app.GetCachedSimulationResult(analysisTextHash("existing user document"), "existing-user-run")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cacheBefore, cacheAfter) || app.simulationWorkspaceCache.requestSequence != requestSequenceBefore {
		t.Fatal("read-only App projection changed the simulation workspace cache")
	}
	if !bytes.Equal(guiBytes, epath181JSON(t, gui)) {
		t.Fatal("projection changed the GUI builder payload")
	}
}

func TestEPATH181ProjectionErrorsAreHonestAcrossBoundaries(t *testing.T) {
	runDirectory, sqlPath, inputPath := epath181CreateStoredRun(t)
	before := epath181FileSnapshot(t, runDirectory)
	server := httptest.NewServer(appAssetHandler(NewApp()))
	defer server.Close()
	valid := map[string]any{"resultPath": sqlPath, "inputPath": inputPath, "scope": "building", "period": "annual", "service": "all"}
	for _, test := range []struct {
		name, method string
		body         []byte
		status       int
	}{
		{"wrong method", http.MethodGet, nil, http.StatusMethodNotAllowed},
		{"malformed", http.MethodPost, []byte(`{`), http.StatusBadRequest},
		{"unknown field", http.MethodPost, []byte(`{"resultPath":"x","invented":true}`), http.StatusBadRequest},
		{"trailing object", http.MethodPost, append(epath181JSON(t, valid), []byte(` {}`)...), http.StatusBadRequest},
		{"oversize", http.MethodPost, []byte(`{"resultPath":"` + strings.Repeat("x", 70*1024) + `"}`), http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := epath181HTTP(t, server.URL, test.method, test.body)
			if response.Code != test.status {
				t.Fatalf("status=%d want%d: %s", response.Code, test.status, response.Body)
			}
		})
	}
	for _, test := range []struct{ name, key, value string }{
		{"missing SQL", "resultPath", filepath.Join(runDirectory, "absent.sql")},
		{"absent month", "period", "M3"},
		{"invalid month", "period", "M13"},
		{"missing Zone", "zone", "Missing Zone"},
		{"unsupported format", "format", "xlsx"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := map[string]any{}
			for key, value := range valid {
				request[key] = value
			}
			request[test.key] = test.value
			if test.key == "zone" {
				request["scope"] = "zone"
			}
			response := epath181HTTP(t, server.URL, http.MethodPost, epath181JSON(t, request))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want400: %s", response.Code, response.Body)
			}
			args := []string{"energy-path", request["resultPath"].(string), "--input", inputPath, "--scope", request["scope"].(string), "--period", request["period"].(string)}
			if zone, _ := request["zone"].(string); zone != "" {
				args = append(args, "--zone", zone)
			}
			if format, _ := request["format"].(string); format != "" {
				args = append(args, "--format", format)
			}
			var stdout, stderr bytes.Buffer
			handled, code := appcli.MaybeRun(args, strings.NewReader(""), &stdout, &stderr, "test")
			if !handled || code == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
				t.Fatalf("CLI must reject without success payload: handled=%t code=%d stdout=%s stderr=%s", handled, code, &stdout, &stderr)
			}
		})
	}
	if !reflect.DeepEqual(before, epath181FileSnapshot(t, runDirectory)) {
		t.Fatal("invalid requests created or modified run files")
	}
}

func epath181AssertSelectedView(t *testing.T, data []byte, request map[string]any) {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	selection, ok := envelope["selection"].(map[string]any)
	if !ok {
		t.Fatal("projection omitted explicit selection")
	}
	for _, key := range []string{"scope", "period", "service"} {
		if selection[key] != request[key] {
			t.Fatalf("selection %s=%v want%v", key, selection[key], request[key])
		}
	}
	if zone, _ := request["zone"].(string); zone != "" && selection["zone"] != zone {
		t.Fatalf("selected Zone=%v want%v", selection["zone"], zone)
	}
	view, ok := envelope["view"].(map[string]any)
	if !ok {
		t.Fatal("projection omitted selected canonical view")
	}
	if view["period"] != request["period"] || view["service"] != request["service"] {
		t.Fatal("view labels differ from explicit selection")
	}
	nodes, ok := view["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		t.Fatal("fixture selected view is empty")
	}
	loadFound := false
	for _, item := range nodes {
		node := item.(map[string]any)
		if period, _ := node["period"].(string); period != "" && !strings.EqualFold(period, request["period"].(string)) {
			t.Fatalf("selected view contains wrong-period node: %v", node)
		}
		if request["service"] == "cooling" && node["serviceKind"] == "heating" {
			t.Fatalf("cooling projection retained heating node: %v", node)
		}
		if node["level"] == "load" && node["serviceKind"] == "cooling" {
			loadFound = true
			want := map[string]float64{"annual": 300, "M1": 100, "M2": 200}[request["period"].(string)]
			if node["value"] != want {
				t.Fatalf("selected cooling load=%v want%v; annual/month/scope must use actual SQL quantities", node["value"], want)
			}
		}
	}
	if !loadFound {
		t.Fatal("actual selected scope/period lost its reported cooling load")
	}
	if request["scope"] == "building" {
		found := map[string]bool{}
		for _, item := range view["links"].([]any) {
			link := item.(map[string]any)
			if link["relation"] != "load_to_end_use" {
				continue
			}
			service, _ := link["serviceKind"].(string)
			want := map[string]float64{"cooling": 4, "heating": 0.85}[service]
			if want == 0 {
				continue
			}
			found[service] = true
			if link["ratio"] != want {
				t.Fatalf("actual %s paired SQL conversion ratio=%v want%v", service, link["ratio"], want)
			}
		}
		if !found["cooling"] || (request["service"] == "all" && !found["heating"]) {
			t.Fatal("actual Building SQL graph lacks expected cooling/heating conversion pairs")
		}
	}
}

func epath181AssertCanonicalQuality(t *testing.T, graph simulation.EnergyExplanationResult, request map[string]any, actual *simulation.EnergyPathQuality) {
	t.Helper()
	quality, periods := graph.Quality, graph.Periods
	if request["scope"] == "zone" {
		found := false
		for _, zone := range graph.ZoneResults {
			if zone.Scope.ZoneName == request["zone"] {
				quality, periods = zone.Quality, zone.Periods
				found = true
				break
			}
		}
		if !found {
			t.Fatal("independent GUI fixture lacks selected Zone quality")
		}
	}
	if request["period"] != "annual" {
		found := false
		for _, period := range periods {
			if period.ID == request["period"] {
				quality = period.Quality
				found = true
				break
			}
		}
		if !found {
			t.Fatal("independent GUI fixture lacks selected month quality")
		}
	}
	if quality == nil {
		t.Fatal("actual GUI builder fixture must contain explicit selected-period quality")
	}
	if !reflect.DeepEqual(quality, actual) {
		t.Fatalf("service projection recomputed/dropped unfiltered scope-period quality: want%+v got%+v", quality, actual)
	}
}

func epath181AssertSummaryAndTraceCSV(t *testing.T, gui simulation.EnergyExplanationResult, projection simulation.EnergyPathProjection, summary, trace string) {
	t.Helper()
	read := func(value string) [][]string {
		reader := csv.NewReader(strings.NewReader(value))
		reader.FieldsPerRecord = -1
		rows, err := reader.ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	plain, full := read(summary), read(trace)
	if len(plain) < 2 || len(full) <= len(plain) {
		t.Fatal("CSV must contain default summary rows and opt-in additional trace rows")
	}
	if !reflect.DeepEqual(plain, full[:len(plain)]) {
		t.Fatal("enabling trace altered summary rows")
	}
	columns := map[string]int{}
	for index, name := range plain[0] {
		columns[name] = index
	}
	for _, name := range []string{"row_type", "stage", "category", "value", "unit", "basis", "aggregation_basis", "period", "source_id", "link_id", "from_id", "to_id", "from_value", "from_unit", "to_value", "to_unit", "ratio", "ratio_kind", "trace_json", "status", "found", "total", "source_unit", "normalized_unit", "message"} {
		if _, exists := columns[name]; !exists {
			t.Fatalf("CSV missing %s header", name)
		}
	}
	cell := func(row []string, name string) string { return row[columns[name]] }
	numeric := func(value float64) string { return strconv.FormatFloat(value, 'g', -1, 64) }
	rowIndex := 1
	for _, group := range []struct {
		stage string
		items []simulation.EnergyExplanationSummaryItem
	}{
		{"drivers", projection.View.Summary.Drivers}, {"loads", projection.View.Summary.Loads}, {"endUses", projection.View.Summary.EndUses}, {"carriers", projection.View.Summary.Carriers}, {"ratios", projection.View.Summary.Ratios}, {"residuals", projection.View.Summary.Residuals},
	} {
		for _, item := range group.items {
			if rowIndex >= len(plain) {
				t.Fatal("CSV lost a canonical summary item")
			}
			row := plain[rowIndex]
			rowIndex++
			for name, want := range map[string]string{"row_type": "summary", "stage": group.stage, "category": item.Label, "value": numeric(item.Value), "unit": item.Unit, "basis": item.Basis, "aggregation_basis": item.AggregationBasis, "source_id": "", "link_id": "", "trace_json": ""} {
				if cell(row, name) != want {
					t.Fatalf("summary %s %s=%q want%q", group.stage, name, cell(row, name), want)
				}
			}
		}
	}
	qualityRows := map[string]map[string]string{}
	carrierDenominatorKnown := epath181GUIHasFacilityDenominator(t, gui, projection.Selection)
	if quality := projection.View.Quality; quality != nil {
		for _, stage := range []struct {
			key   string
			level simulation.EnergyCompletenessLevel
		}{
			{"drivers", quality.Drivers}, {"loads", quality.Loads}, {"endUses", quality.EndUses}, {"carriers", quality.Carriers}, {"ratios", quality.Ratios},
		} {
			category := "output_availability"
			if stage.key == "ratios" {
				category = "conversion_availability"
			}
			want := map[string]string{"status": stage.level.Status, "value": "", "found": "", "total": "", "message": stage.level.Message}
			if stage.level.Total > 0 && stage.level.Status != "" && stage.level.Status != "unavailable" && stage.level.Status != "not_requested" && stage.level.Status != "not_applicable" {
				want["found"] = strconv.Itoa(stage.level.Found)
				want["total"] = strconv.Itoa(stage.level.Total)
			}
			qualityRows[stage.key+"/"+category] = want
		}
		for _, metric := range []struct {
			key, status string
			value       float64
		}{
			{"driver_to_load_closed_pct", quality.DriverToLoadStatus, quality.DriverToLoadClosedPct}, {"end_use_to_carrier_closed_pct", quality.EndUseToCarrierStatus, quality.EndUseToCarrierClosedPct}, {"zone_allocated_pct", quality.ZoneAllocationStatus, quality.ZoneAllocatedPct}, {"unassigned_pct", quality.ZoneAllocationStatus, quality.UnassignedPct},
		} {
			value := ""
			if metric.status == "balanced" || metric.status == "complete" || metric.status == "partial" || metric.status == "overmapped" {
				value = numeric(metric.value)
			}
			message := ""
			if metric.key == "end_use_to_carrier_closed_pct" && !carrierDenominatorKnown {
				value = ""
				message = "Facility denominator unavailable; observed or allocated subtotals are not measured facility totals."
			}
			qualityRows["/"+metric.key] = map[string]string{"status": metric.status, "value": value, "unit": "%", "found": "", "total": "", "message": message}
		}
	}
	for _, row := range plain[rowIndex:] {
		key := cell(row, "stage") + "/" + cell(row, "category")
		want, exists := qualityRows[key]
		if cell(row, "row_type") != "quality" || !exists {
			t.Fatalf("default CSV includes unexpected/duplicate non-summary row %v", row)
		}
		delete(qualityRows, key)
		for name, value := range want {
			if cell(row, name) != value {
				t.Fatalf("quality %s %s=%q want%q; unknown is not zero", key, name, cell(row, name), value)
			}
		}
		if cell(row, "source_id") != "" || cell(row, "link_id") != "" || cell(row, "trace_json") != "" {
			t.Fatal("default quality row leaked trace identities")
		}
	}
	if len(qualityRows) != 0 {
		t.Fatal("default CSV omitted actual stage or boundary quality")
	}
	sources := map[string]simulation.EnergyDataSource{}
	for _, source := range projection.View.Sources {
		sources[source.ID] = source
	}
	links := map[string]simulation.EnergyPathLink{}
	for _, link := range projection.View.Links {
		links[link.ID] = link
	}
	for _, row := range full[len(plain):] {
		switch cell(row, "row_type") {
		case "source":
			id := cell(row, "source_id")
			source, exists := sources[id]
			if !exists {
				t.Fatalf("extra/duplicate trace source %q", id)
			}
			delete(sources, id)
			if cell(row, "period") != "" || cell(row, "value") != "" {
				t.Fatal("source trace falsely labels annual source scalar as a selected-month value")
			}
			if cell(row, "source_unit") != source.SourceUnit || cell(row, "normalized_unit") != source.NormalizedUnit {
				t.Fatal("source trace lost raw/normalized units")
			}
			epath181EqualJSON(t, "exact source trace", epath181JSON(t, source), []byte(cell(row, "trace_json")))
		case "link":
			id := cell(row, "link_id")
			link, exists := links[id]
			if !exists {
				t.Fatalf("extra/duplicate trace link %q", id)
			}
			delete(links, id)
			for name, want := range map[string]string{"from_id": link.FromID, "to_id": link.ToID, "from_value": numeric(link.FromValue), "to_value": numeric(link.ToValue), "from_unit": link.FromUnit, "to_unit": link.ToUnit, "ratio_kind": link.RatioKind} {
				if cell(row, name) != want {
					t.Fatalf("trace link %s %s=%q want%q", id, name, cell(row, name), want)
				}
			}
			epath181EqualJSON(t, "exact link trace", epath181JSON(t, link), []byte(cell(row, "trace_json")))
		default:
			t.Fatalf("unexpected opt-in row type %q", cell(row, "row_type"))
		}
	}
	if len(sources) != 0 || len(links) != 0 {
		t.Fatal("trace omitted canonical sources or links")
	}
	if strings.Contains(summary, "sql-rdd-") {
		t.Fatal("default summary CSV leaked raw source IDs")
	}
	if !strings.Contains(trace, "sql-rdd-") {
		t.Fatal("opt-in trace CSV omitted actual SQL source identities")
	}
}

// Inspect the independent GUI builder's full selected scope/period, never the
// service-filtered view. This fixture uses canonical kWh carrier records; Zone
// observed/allocated subtotals cannot supply a measured facility denominator.
func epath181GUIHasFacilityDenominator(t *testing.T, gui simulation.EnergyExplanationResult, selection simulation.EnergyPathSelection) bool {
	t.Helper()
	nodes, periods := gui.Nodes, gui.Periods
	if selection.Scope == "zone" {
		found := false
		for _, zone := range gui.ZoneResults {
			if zone.Scope.ZoneName == selection.Zone {
				nodes, periods, found = zone.Nodes, zone.Periods, true
				break
			}
		}
		if !found {
			t.Fatal("GUI denominator lookup has no selected Zone")
		}
	}
	periodFound := false
	for _, period := range periods {
		if period.ID == selection.Period {
			nodes, periodFound = period.Nodes, true
			break
		}
	}
	if selection.Period != "annual" && !periodFound {
		t.Fatal("GUI denominator lookup has no selected month")
	}
	for _, node := range nodes {
		if node.Level != "carrier" || node.ScaleDomain != "site" || node.ZoneName != "" || math.IsNaN(node.Value) || math.IsInf(node.Value, 0) || math.Abs(node.Value) == 0 {
			continue
		}
		if node.Unit != "kWh" {
			t.Fatalf("independent fixture denominator requires canonical kWh, got %q", node.Unit)
		}
		if node.MeterHierarchyLevel == "observed_end_use_subtotal" || node.MeterHierarchyLevel == "zone_direct_subtotal" {
			continue
		}
		switch node.Basis {
		case "reported_end_use_subtotal", "direct_zone_energy", "service_path_allocation", "zone_load_allocation", "heat_balance_share":
			continue
		}
		if node.MeterHierarchyLevel == "facility_total" || node.Basis == "reported_meter" || node.Basis == "reported_variable" || node.Basis == "integrated_rate" {
			return true
		}
	}
	return false
}

func epath181HTTP(t *testing.T, base, method string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request, err := http.NewRequest(method, base+"/api/energy-path", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	result := httptest.NewRecorder()
	for key, values := range response.Header {
		result.Header()[key] = values
	}
	result.WriteHeader(response.StatusCode)
	if _, err := io.Copy(result.Body, response.Body); err != nil {
		t.Fatal(err)
	}
	return result
}

func epath181JSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func epath181EqualJSON(t *testing.T, label string, want, got []byte) {
	t.Helper()
	var left, right any
	if err := json.Unmarshal(want, &left); err != nil {
		t.Fatalf("%s expected JSON: %v", label, err)
	}
	if err := json.Unmarshal(got, &right); err != nil {
		t.Fatalf("%s actual JSON: %v\n%s", label, err, got)
	}
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("%s structurally differs from shared GUI projection: %s", label, epath181FirstJSONDifference(left, right, "$"))
	}
}

func epath181FirstJSONDifference(left, right any, path string) string {
	if reflect.DeepEqual(left, right) {
		return ""
	}
	if l, ok := left.(map[string]any); ok {
		if r, ok := right.(map[string]any); ok {
			keys := []string{}
			seen := map[string]bool{}
			for key := range l {
				keys = append(keys, key)
				seen[key] = true
			}
			for key := range r {
				if !seen[key] {
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			for _, key := range keys {
				a, aok := l[key]
				b, bok := r[key]
				if aok != bok {
					return fmt.Sprintf("%s.%s presence want=%t got=%t", path, key, aok, bok)
				}
				if diff := epath181FirstJSONDifference(a, b, path+"."+key); diff != "" {
					if l["id"] != nil || r["id"] != nil {
						return fmt.Sprintf("%s (record IDs want=%v got=%v)", diff, l["id"], r["id"])
					}
					return diff
				}
			}
		}
	}
	if l, ok := left.([]any); ok {
		if r, ok := right.([]any); ok {
			if len(l) != len(r) {
				return fmt.Sprintf("%s length want=%d got=%d", path, len(l), len(r))
			}
			for index := range l {
				if diff := epath181FirstJSONDifference(l[index], r[index], fmt.Sprintf("%s[%d]", path, index)); diff != "" {
					return diff
				}
			}
		}
	}
	return fmt.Sprintf("%s want=%v got=%v", path, left, right)
}
func epath181Python(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python", "python3"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	t.Skip("Python3 unavailable for checked-in client acceptance")
	return ""
}
func epath181FileSnapshot(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	result := map[string][32]byte{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[relative] = sha256.Sum256(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func epath181CreateStoredRun(t *testing.T) (string, string, string) {
	t.Helper()
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "Office 한글.idf")
	sqlPath := filepath.Join(directory, "eplusout.sql")
	if err := os.WriteFile(inputPath, []byte(epath181InputIDF), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY, KeyValue TEXT, Name TEXT, Units TEXT, IsMeter INTEGER, ReportingFrequency TEXT, IndexGroup TEXT)`,
		`CREATE TABLE "Time" (TimeIndex INTEGER PRIMARY KEY, Month INTEGER, Day INTEGER, Hour INTEGER, Minute INTEGER)`,
		`CREATE TABLE ReportData (ReportDataIndex INTEGER PRIMARY KEY, TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`,
		`INSERT INTO "Time" VALUES (1,1,31,24,0),(2,2,28,24,0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	entries := []struct {
		key, name string
		meter     int
		value     float64
	}{
		{"Office Wall", "Surface Inside Face Convection Heat Transfer Energy", 0, 80},
		{"Office", "Zone Lights Total Heating Energy", 0, 20},
		{"Office", "Zone Air System Sensible Cooling Energy", 0, 100},
		{"Office", "Zone Air System Sensible Heating Energy", 0, 85},
		{"", "Electricity:Facility", 1, 35},
		{"", "Cooling:Electricity", 1, 25},
		{"", "InteriorLights:Electricity", 1, 10},
		{"", "NaturalGas:Facility", 1, 100},
		{"", "Heating:NaturalGas", 1, 100},
		{"Office", "Zone Lights Electricity Energy", 0, 10},
	}
	for index, entry := range entries {
		if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES (?,?,?,'J',?,'Monthly',?)`, index+1, entry.key, entry.name, entry.meter, func() string {
			if entry.meter == 1 {
				return "Meter"
			}
			return "Zone"
		}()); err != nil {
			db.Close()
			t.Fatal(err)
		}
		for month := 1; month <= 2; month++ {
			if _, err := db.Exec(`INSERT INTO ReportData VALUES (?,?,?,?)`, index*2+month, month, index+1, entry.value*float64(month)*3600000); err != nil {
				db.Close()
				t.Fatal(err)
			}
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return directory, sqlPath, inputPath
}

const epath181InputIDF = `Version,25.1;
Building,Stored Office;
GlobalGeometryRules,UpperLeftCorner,Counterclockwise,World;
Zone,Office,0,0,0,0,1,1,3,300,100;
Material,Wall material,Rough,0.2,1,1000,1000;
Construction,Wall construction,Wall material;
BuildingSurface:Detailed,Office Wall,Wall,Wall construction,Office,,Outdoors,,SunExposed,WindExposed,0.5,4,0,0,3,0,0,0,10,0,0,10,0,3;
BuildingSurface:Detailed,Office Floor,Floor,Wall construction,Office,,Ground,,NoSun,NoWind,1,4,0,0,0,0,10,0,10,10,0,10,0,0;
Schedule:Constant,Always On,,1;
Lights,Office Lights,Office,Always On,LightingLevel,100;
`

const epath181PythonCrossSurface = `import importlib.util, json, pathlib, subprocess, sys
from urllib.error import HTTPError
fixture = json.load(sys.stdin)
spec = importlib.util.spec_from_file_location("semantic_idf_client", fixture["module"])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
client = module.SemanticIDFClient(fixture["url"])
for case in fixture["cases"]:
    request = case["request"]
    options = dict(input_path=pathlib.Path(request["inputPath"]), scope=request["scope"],
                   zone=request.get("zone", ""), period=request["period"], service=request["service"])
    path = pathlib.Path(request["resultPath"])
    for output_format in ("json", "csv"):
        for trace in (False, True):
            settings = dict(options, output_format=output_format, include_trace=trace)
            via_http = client.energy_path(path, **settings)
            via_stdio = module.SemanticIDFClient.energy_path_stdio(pathlib.Path(fixture["executable"]), path, **settings)
            want = case["json"] if output_format == "json" else case["trace" if trace else "csv"]
            assert via_http == want, ("HTTP client mismatch", request, output_format, trace)
            assert via_stdio == want, ("stdio client mismatch", request, output_format, trace)
bad = fixture["cases"][0]["request"]
settings = dict(input_path=bad["inputPath"], period="M3")
try:
    client.energy_path(bad["resultPath"], **settings)
except HTTPError as error:
    assert error.code == 400
else:
    raise AssertionError("Python HTTP accepted a genuinely absent month")
try:
    module.SemanticIDFClient.energy_path_stdio(fixture["executable"], bad["resultPath"], **settings)
except subprocess.CalledProcessError as error:
    assert error.returncode != 0 and error.stdout == b"" and error.stderr
else:
    raise AssertionError("Python stdio accepted a genuinely absent month")
print("EPATH181 Python HTTP/stdio PASS")
`
