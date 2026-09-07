package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/tabular"
)

// The actual Tools event handler owns both exports. Only the native Save bridge
// is stubbed; its exact JSON is decoded by the production Go DTO and formatter.
func TestEPATH171ActualToolsTraceOptionToWorkbookBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("actual Tools export browser acceptance")
	}
	chrome := epath171ExportChrome()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	page, err := os.ReadFile("frontend/src/tools.html")
	if err != nil {
		t.Fatal(err)
	}
	setup := epath171ReadTestString(t, "internal/frontendchecks/energy_path_batch_epath170_browser_test.go", "epath170BatchSetupHTML")
	marker := `<script type="module" src="./js/tools.js"></script>`
	if !strings.Contains(string(page), marker) {
		t.Fatal("actual Tools bootstrap changed")
	}
	pageText := strings.Replace(string(page), marker, setup+epath171ExportBridgeHTML+marker+epath171ExportAssertionsHTML, 1)
	requests := make(chan []byte, 3)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir("frontend/src"))))
	mux.HandleFunc("/src/epath171-tools.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, pageText)
	})
	mux.HandleFunc("/epath171-save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		select {
		case requests <- data:
		default:
			http.Error(w, "unexpected additional save", 409)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"filename":"captured.xlsx"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--window-size=1600,900", "--virtual-time-budget=18000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/src/epath171-tools.html#batch-simulation").CombinedOutput()
	if err != nil {
		t.Fatalf("actual export browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath171-status="passed"`) {
		proof := regexp.MustCompile(`(?s)<pre id="epath171-result"[^>]*>(.*?)</pre>`).FindSubmatch(output)
		if len(proof) == 2 {
			t.Fatalf("actual export browser: %s", proof[1])
		}
		t.Fatalf("actual export browser:\n%s", output)
	}
	if len(requests) != 3 {
		t.Fatalf("captured Save requests=%d, want unchecked, checked, and explicit unit-mismatch exports", len(requests))
	}
	var originalDefaults []tabular.WorkbookSheet
	for exportIndex := 0; exportIndex < 3; exportIndex++ {
		wire := <-requests
		var request BatchSimulationXLSXExportRequest
		if err := json.Unmarshal(wire, &request); err != nil {
			t.Fatalf("production request JSON decode: %v", err)
		}
		if request.IncludeTraceSheets != (exportIndex == 1) {
			t.Fatalf("checkbox option did not cross Go JSON boundary: %#v", request.IncludeTraceSheets)
		}
		if len(request.Result.Results) != 100 {
			t.Fatalf("actual100-row result lost during export: %d", len(request.Result.Results))
		}
		sheets, err := buildBatchSimulationExportWorkbook(request)
		if err != nil {
			t.Fatalf("production workbook formatter export%d: %v", exportIndex, err)
		}
		want := []string{"Energy Path Summary", "Energy Path Delta", "Data Quality", "Runs"}
		if len(sheets) < 4 {
			t.Fatalf("workbook missing default sheets: %#v", sheets)
		}
		for index, name := range want {
			if sheets[index].Name != name {
				t.Fatalf("default sheet%d=%s, want%s", index, sheets[index].Name, name)
			}
		}
		if exportIndex != 1 && len(sheets) != 4 {
			t.Fatalf("default workbook leaked trace sheets: %v", epath171SheetNames(sheets))
		}
		if exportIndex == 0 {
			originalDefaults = sheets
		} else if exportIndex == 1 && !reflect.DeepEqual(originalDefaults, sheets[:4]) {
			t.Fatal("trace option changed summary/delta/quality/run values")
		}
		var workbook bytes.Buffer
		if err := tabular.WriteWorkbookXLSX(&workbook, sheets); err != nil {
			t.Fatal(err)
		}
		archive, err := zip.NewReader(bytes.NewReader(workbook.Bytes()), int64(workbook.Len()))
		if err != nil {
			t.Fatal(err)
		}
		xmlFiles := map[string][]byte{}
		for _, file := range archive.File {
			reader, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(reader)
			_ = reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			xmlFiles[file.Name] = data
		}
		var workbookXML struct {
			Sheets []struct {
				Name string `xml:"name,attr"`
			} `xml:"sheets>sheet"`
		}
		if err := xml.Unmarshal(xmlFiles["xl/workbook.xml"], &workbookXML); err != nil {
			t.Fatal(err)
		}
		var actualNames []string
		for _, sheet := range workbookXML.Sheets {
			actualNames = append(actualNames, sheet.Name)
		}
		if !reflect.DeepEqual(actualNames, epath171SheetNames(sheets)) {
			t.Fatalf("actual ZIP workbook names differ: %v", actualNames)
		}
		for sheetIndex, sheet := range sheets {
			var sheetXML struct {
				Rows []struct {
					Cells []struct {
						Value string `xml:"is>t"`
					} `xml:"c"`
				} `xml:"sheetData>row"`
			}
			if err := xml.Unmarshal(xmlFiles[fmt.Sprintf("xl/worksheets/sheet%d.xml", sheetIndex+1)], &sheetXML); err != nil {
				t.Fatal(err)
			}
			actualRow := 0
			for _, section := range sheet.Sections {
				for _, expected := range append([][]string{{"[" + section.Title + "]"}, section.Headers}, section.Rows...) {
					if actualRow >= len(sheetXML.Rows) {
						t.Fatalf("%s ZIP rows truncated", sheet.Name)
					}
					cells := sheetXML.Rows[actualRow].Cells
					if len(cells) < len(expected) {
						t.Fatalf("%s ZIP row%d cells truncated", sheet.Name, actualRow)
					}
					for column, value := range expected {
						if cells[column].Value != value {
							t.Fatalf("%s ZIP row%d column%d=%q want%q", sheet.Name, actualRow, column, cells[column].Value, value)
						}
					}
					actualRow++
				}
			}
		}
		allXML := string(bytes.Join(epath171XMLValues(xmlFiles), nil))
		if exportIndex != 1 {
			for _, canary := range []string{"NODE_PRIVATE_170_", "SOURCE_PRIVATE_170_", "EDGE_PRIVATE_170_"} {
				if strings.Contains(allXML, canary) {
					t.Fatalf("default XLSX leaked raw trace%s", canary)
				}
			}
		} else {
			for _, name := range []string{"Energy Nodes", "Energy Sources", "Energy Edges", "Energy Path Link Delta", "Energy Path JSON"} {
				if !strings.Contains(strings.Join(actualNames, "|"), name) {
					t.Fatalf("trace option omitted %s: %v", name, actualNames)
				}
			}
			for _, canary := range []string{"NODE_PRIVATE_170_0_", "SOURCE_PRIVATE_170_0", "EDGE_PRIVATE_170_0_"} {
				if !strings.Contains(allXML, canary) {
					t.Fatalf("checked XLSX lost reproducibility trace%s", canary)
				}
			}
			epath171AssertOriginalTraceJSON(t, sheets, wire)
		}
		// Assert semantic values from the transmitted rows, not a second graph
		// aggregation, before the ZIP-level exact-cell proof above.
		epath171AssertWorkbookComparison(t, sheets, wire)
	}
}

func epath171AssertOriginalTraceJSON(t *testing.T, sheets []tabular.WorkbookSheet, wire []byte) {
	t.Helper()
	var original struct {
		Result struct {
			Results []struct {
				PurposeResults map[string]json.RawMessage `json:"purposeResults"`
			} `json:"results"`
		} `json:"result"`
	}
	if err := json.Unmarshal(wire, &original); err != nil {
		t.Fatal(err)
	}
	chunks := map[string][]string{}
	for _, sheet := range sheets {
		if sheet.Name != "Energy Path JSON" {
			continue
		}
		for _, section := range sheet.Sections {
			for _, row := range section.Rows {
				if len(row) != 7 || row[3] != "original_submitted_json" {
					t.Fatalf("actual captured trace lost original representation: %v", row)
				}
				key := row[0] + "/" + row[2]
				index, indexErr := strconv.Atoi(row[4])
				count, countErr := strconv.Atoi(row[5])
				if indexErr != nil || countErr != nil || index < 0 || index >= count {
					t.Fatalf("invalid trace chunk indices: %v", row[:6])
				}
				if _, exists := chunks[key]; !exists {
					chunks[key] = make([]string, count)
				}
				if len(chunks[key]) != count || chunks[key][index] != "" {
					t.Fatalf("inconsistent/duplicate trace chunk for%s", key)
				}
				chunks[key][index] = row[6]
			}
		}
	}
	for index, run := range original.Result.Results {
		for _, part := range []string{"energyExplanation", "energyExplanationSummary"} {
			raw, exists := run.PurposeResults[part]
			if !exists {
				continue
			}
			key := strconv.Itoa(index) + "/" + part
			parts := chunks[key]
			if len(parts) == 0 {
				t.Fatalf("original trace%s missing", key)
			}
			for _, chunk := range parts {
				if chunk == "" {
					t.Fatalf("original trace%s has missing chunk", key)
				}
			}
			if !bytes.Equal([]byte(strings.Join(parts, "")), raw) {
				t.Fatalf("original JSON trace%s changed nullable values, optional fields or provenance", key)
			}
		}
	}
}

func epath171AssertWorkbookComparison(t *testing.T, sheets []tabular.WorkbookSheet, wire []byte) {
	t.Helper()
	var request BatchSimulationXLSXExportRequest
	if err := json.Unmarshal(wire, &request); err != nil {
		t.Fatal(err)
	}
	if request.EnergyPath == nil || len(request.EnergyPath.Comparison.Rows) == 0 {
		t.Fatal("captured export has no semantic comparison projection")
	}
	metadata := map[string]string{}
	for _, section := range sheets[3].Sections {
		if section.Title == "energy_path_context" {
			for _, row := range section.Rows {
				if len(row) >= 2 {
					metadata[row[0]] = row[1]
				}
			}
		}
	}
	for key, value := range map[string]string{"scope": "building", "period": "annual", "baseline_row_id": request.EnergyPath.Comparison.BaselineRowID, "target_row_id": request.EnergyPath.Comparison.TargetRowID, "comparison_status": "ready"} {
		if metadata[key] != value {
			t.Fatalf("workbook context%s=%q, want%q", key, metadata[key], value)
		}
	}
	sections := map[string][]map[string]string{}
	for _, sheet := range sheets[:3] {
		for _, section := range sheet.Sections {
			for _, cells := range section.Rows {
				row := map[string]string{}
				for index, header := range section.Headers {
					if index < len(cells) {
						row[header] = cells[index]
					}
				}
				sections[sheet.Name] = append(sections[sheet.Name], row)
			}
		}
	}
	number := func(value *float64) string {
		if value == nil {
			return ""
		}
		return strconv.FormatFloat(*value, 'g', -1, 64)
	}
	find := func(sheet string, match map[string]string) map[string]string {
		t.Helper()
		var matches []map[string]string
		for _, candidate := range sections[sheet] {
			ok := true
			for key, value := range match {
				if candidate[key] != value {
					ok = false
					break
				}
			}
			if ok {
				matches = append(matches, candidate)
			}
		}
		if len(matches) != 1 {
			t.Fatalf("%s expected one row%v, got%d", sheet, match, len(matches))
		}
		return matches[0]
	}
	verify := func(actual, expected map[string]string) {
		t.Helper()
		for key, value := range expected {
			if actual[key] != value {
				t.Fatalf("worksheet%s/%s field%s=%q, want captured%q", actual["stage"], actual["category"], key, actual[key], value)
			}
		}
	}
	for _, row := range request.EnergyPath.Comparison.Rows {
		sheet := "Energy Path Delta"
		match := map[string]string{"stage": row.StageLabel, "category": row.Category}
		if row.Kind == "quality" {
			sheet = "Data Quality"
			match["context"] = "comparison"
		}
		actual := find(sheet, match)
		expected := map[string]string{"baseline": number(row.Baseline.Value), "target": number(row.Target.Value), "unit": row.Unit, "delta": number(row.Delta), "delta_unit": row.DeltaUnit, "delta_percent": number(row.DeltaPercent), "warnings": strings.Join(row.WarningCodes, "; ")}
		if row.Kind == "quality" {
			for prefix, side := range map[string]BatchEnergyPathExportSide{"baseline": row.Baseline, "target": row.Target} {
				expected[prefix+"_status"] = side.Quality.Status
				expected[prefix+"_found"] = number(side.Quality.Found)
				expected[prefix+"_total"] = number(side.Quality.Total)
				expected[prefix+"_percent"] = number(side.Quality.Percent)
			}
		} else {
			if row.UnitMismatch {
				expected["unit"] = ""
			}
			expected["baseline_basis"] = row.Baseline.Basis
			expected["target_basis"] = row.Target.Basis
			expected["baseline_aggregation_basis"] = row.Baseline.AggregationBasis
			expected["target_aggregation_basis"] = row.Target.AggregationBasis
			for key, value := range map[string]bool{"basis_mismatch": row.BasisMismatch, "coverage_mismatch": row.CoverageMismatch, "unit_mismatch": row.UnitMismatch, "directly_comparable": row.DirectlyComparable} {
				expected[key] = strconv.FormatBool(value)
			}
			expected["baseline_unit"] = row.Baseline.Unit
			expected["target_unit"] = row.Target.Unit
		}
		verify(actual, expected)
		if row.BasisMismatch && !strings.Contains(actual["warning_text"], "Not directly comparable") {
			t.Fatal("basis mismatch workbook warning is not readable")
		}
		if row.CoverageMismatch && !strings.Contains(actual["warning_text"], "Source coverage differs") {
			t.Fatal("coverage workbook warning lost visible meaning")
		}
		if row.UnitMismatch && !strings.Contains(actual["warning_text"], "no delta") {
			t.Fatal("unit mismatch workbook warning does not explain blank delta")
		}
	}
	for _, run := range request.EnergyPath.Runs {
		for _, row := range run.Rows {
			if row.Kind == "quality" {
				continue
			}
			side := row.Baseline
			actual := find("Energy Path Summary", map[string]string{"result_index": strconv.Itoa(run.ResultIndex), "stage": row.StageLabel, "category": row.Category})
			verify(actual, map[string]string{"value": number(side.Value), "unit": side.Unit, "basis": side.Basis, "aggregation_basis": side.AggregationBasis, "missing": strconv.FormatBool(side.Missing), "ambiguous": strconv.FormatBool(side.Ambiguous), "invalid": strconv.FormatBool(side.Invalid), "quality_status": side.Quality.Status, "quality_found": number(side.Quality.Found), "quality_total": number(side.Quality.Total), "quality_percent": number(side.Quality.Percent), "warnings": strings.Join(row.WarningCodes, "; ")})
		}
	}
	// Literal fixture checks independently anchor the source/DTO/cell chain.
	for _, row := range request.EnergyPath.Comparison.Rows {
		if row.Stage == "carriers" && row.CategoryID == "electricity" && request.EnergyPath.Comparison.TargetRowID == "private170-run-002" {
			if row.Baseline.Unit != "kWh" || row.Target.Unit != "MJ" || !row.UnitMismatch || row.Delta != nil || row.DeltaPercent != nil {
				t.Fatal("actual selected unit-mismatch export fabricated arithmetic or relabelled the target unit")
			}
		}
		if row.Stage == "endUses" && row.CategoryID == "cooling" {
			if number(row.Baseline.Value) != "25" || number(row.Target.Value) != "20" || number(row.Delta) != "-5" || number(row.DeltaPercent) != "-20" || !row.BasisMismatch || !row.CoverageMismatch {
				t.Fatalf("measured/allocated cooling evidence changed: %#v", row)
			}
		}
		if row.Stage == "endUses" && row.CategoryID == "refrigeration" {
			if number(row.Baseline.Value) != "0" || number(row.Target.Value) != "5" || number(row.Delta) != "5" || row.DeltaPercent != nil {
				t.Fatal("reported0/zero-baseline percentage changed at Go boundary")
			}
		}
		if row.Stage == "endUses" && (row.CategoryID == "equipment" || row.CategoryID == "water_systems") {
			if row.Target.Value != nil || row.Delta != nil || row.DeltaPercent != nil {
				t.Fatal("missing/null category became0 in exported workbook")
			}
		}
		if row.Kind == "quality" && row.CategoryID == "endUses_coverage" {
			if number(row.Baseline.Value) != "100" || number(row.Target.Value) != "50" || number(row.Delta) != "-50" || row.DeltaUnit != "pp" {
				t.Fatal("source coverage or percentage-point delta changed")
			}
		}
	}
}

func epath171SheetNames(sheets []tabular.WorkbookSheet) []string {
	var names []string
	for _, sheet := range sheets {
		names = append(names, sheet.Name)
	}
	return names
}
func epath171XMLValues(files map[string][]byte) [][]byte {
	var values [][]byte
	for _, data := range files {
		values = append(values, data)
	}
	return values
}

func epath171ReadTestString(t *testing.T, path, name string) string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, specification := range general.Specs {
			value, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, id := range value.Names {
				if id.Name != name || index >= len(value.Values) {
					continue
				}
				literal, ok := value.Values[index].(*ast.BasicLit)
				if !ok {
					t.Fatal("test fixture is not a string literal")
				}
				text, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatal(err)
				}
				return text
			}
		}
	}
	t.Fatalf("test fixture%s not found", name)
	return ""
}

func epath171ExportChrome() string {
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome", "msedge"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	if runtime.GOOS == "windows" {
		for _, path := range []string{filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"), filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"), filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"), filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe")} {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path
			}
		}
	}
	return ""
}

const epath171ExportBridgeHTML = `<script>
window.epath171Requests=[];
window.go.main.App.SaveBatchSimulationXLSX=async request=>{const response=await fetch('/epath171-save',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(request)});if(!response.ok)throw new Error(await response.text());window.epath171Requests.push(JSON.parse(JSON.stringify(request)));return response.json();};
</script>`

const epath171ExportAssertionsHTML = `<pre id="epath171-result" hidden>pending</pre><script type="module">
const wait=async(predicate,label)=>{for(let i=0;i<300;i++){if(predicate())return;await new Promise(resolve=>setTimeout(resolve,20));}throw new Error('Timed out: '+label);};
const check=(value,message)=>{if(!value)throw new Error(message);};
try{
 await wait(()=>document.querySelector('[data-tools-tab="batch-simulation"]')?.classList.contains('active'),'actual Tools Batch tab');
 document.getElementById('multiSimulationSelectFiles').click();await wait(()=>!document.getElementById('multiSimulationRun').disabled,'selected input files');document.getElementById('multiSimulationRun').click();
 await wait(()=>document.querySelector('[data-batch-energy-summary-table]'),'annual summary table');
 const {energyPathBatchComparison}=await import('/src/js/energy-path-batch-comparison.js');
 const fixture=window.epath170,baseline=document.getElementById('multiSimulationCompareBaseline'),target=document.getElementById('multiSimulationCompareTarget');
 baseline.value=fixture.payload.results[0].runId;baseline.dispatchEvent(new Event('change',{bubbles:true}));target.value=fixture.payload.results[1].runId;target.dispatchEvent(new Event('change',{bubbles:true}));
 const checkbox=document.getElementById('multiSimulationIncludeTraceSheets'),button=document.getElementById('multiSimulationExportXLSX');
 check(checkbox?.type==='checkbox'&&!checkbox.checked,'trace sheets checkbox must default unchecked');check(checkbox.labels?.[0]?.textContent.includes('Include trace sheets'),'trace option has no actual accessible label');
 const rows=energyPathBatchComparison(fixture.payload.results[0],fixture.payload.results[1]).rows;
 const mainBefore=sessionStorage.getItem('idfAnalyzer.currentDocument'),resultBefore=JSON.stringify(fixture.payload);
 button.click();await wait(()=>window.epath171Requests.length===1,'default actual Save bridge');
 checkbox.click();check(checkbox.checked,'native checkbox click did not select trace sheets');button.click();await wait(()=>window.epath171Requests.length===2,'trace actual Save bridge');
 target.value=fixture.payload.results[2].runId;target.dispatchEvent(new Event('change',{bubbles:true}));checkbox.click();check(!checkbox.checked,'native trace toggle did not return to default');button.click();await wait(()=>window.epath171Requests.length===3,'unit-mismatch actual Save bridge');
 for(const[index,request]of window.epath171Requests.entries()){
  const comparedRows=index===2?energyPathBatchComparison(fixture.payload.results[0],fixture.payload.results[2]).rows:rows;
  check(request.includeTraceSheets===(index===1),'checkbox option missing from actual Save request');check(request.energyPath?.schema==='semantic-idf.energy-path-batch-export/v2','actual Save request omitted typed Energy Path projection');check(request.energyPath.comparison.baselineRowId===fixture.payload.results[0].runId&&request.energyPath.comparison.targetRowId===fixture.payload.results[index===2?2:1].runId,'export changed selected comparison pair');
  check(request.energyPath.comparison.rows.length===comparedRows.length,'export dropped/added semantic summary comparison rows');
  for(const expected of comparedRows){const row=request.energyPath.comparison.rows.find(row=>row.key===expected.key);check(Boolean(row),'export rematched category by raw IDs');for(const key of['kind','stage','category','delta','deltaPercent','basisMismatch','coverageMismatch','directlyComparable','warningCodes'])check(JSON.stringify(row[key])===JSON.stringify(expected[key]),'export differs from actual170 comparator '+key);for(const side of['baseline','target']){for(const key of['value','unit','basis','aggregationBasis','missing','ambiguous','invalid'])check(JSON.stringify(row[side][key])===JSON.stringify(expected[side][key]),'export changed nullable source evidence '+side+'.'+key);for(const key of['status','found','total','percent'])check(JSON.stringify(row[side].quality[key])===JSON.stringify(expected[side].quality[key]),'export changed source quality '+side+'.'+key);}}
 }
 check(!/NODE_PRIVATE_170_|SOURCE_PRIVATE_170_|EDGE_PRIVATE_170_|Sankey Edge Delta|Largest Energy Explanation Changes/.test(document.getElementById('multiSimulationChart').innerText),'raw trace/edge delta remains in primary UI');
 check(fixture.calls.run===1&&fixture.calls.select===1&&fixture.calls.forbidden===0,'export triggered additional Analyze/Run');check(JSON.stringify(fixture.payload)===resultBefore&&sessionStorage.getItem('idfAnalyzer.currentDocument')===mainBefore,'export mutated raw results or main workspace');
 document.body.dataset.epath171Status='passed';document.getElementById('epath171-result').textContent='Actual Tools unchecked/checked exports match the170 comparator and preserve zero/null/basis/coverage; no extra Analyze/Run.';
}catch(error){document.body.dataset.epath171Status='failed';document.getElementById('epath171-result').textContent=error.stack||String(error);}
</script>`
