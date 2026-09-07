package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/tabular"
)

func TestEPATH182ActualDataExportsToWorkbookAndHTML(t *testing.T) {
	if testing.Short() {
		t.Skip("actual Data details export browser acceptance")
	}
	chrome := epath171ExportChrome()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge unavailable")
	}
	page, err := os.ReadFile("frontend/src/index.html")
	if err != nil {
		t.Fatal(err)
	}
	markup := string(page)
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		if !strings.Contains(markup, script) {
			t.Fatalf("actual app bootstrap changed: %s", script)
		}
		markup = strings.Replace(markup, script, "", 1)
	}
	bootstrap := epath171ReadTestString(t, "internal/frontendchecks/energy_path_layout_epath142_browser_test.go", "epath142LayoutHTML")
	markup = strings.Replace(markup, "</body>", bootstrap+epath182ReportBrowserHTML+"</body>", 1)
	saves := make(chan []byte, 3)
	downloads := make(chan []byte, 3)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir("frontend/src"))))
	for name, counter := range map[string]string{"energy-path-layout.js": "__epath182LayoutCalls", "energy-path-ribbons.js": "__epath182RibbonCalls"} {
		mux.HandleFunc("/src/js/"+name, func(w http.ResponseWriter, r *http.Request) {
			data, err := os.ReadFile("frontend/src/js/" + name)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			pattern := regexp.MustCompile(`export function energyPath(?:Layout|Ribbons)\([^)]*\)\s*\{`)
			if !pattern.Match(data) {
				http.Error(w, "layout instrumentation boundary changed", 500)
				return
			}
			body := pattern.ReplaceAllStringFunc(string(data), func(match string) string {
				return match + "globalThis." + counter + "=(globalThis." + counter + "||0)+1;"
			})
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = io.WriteString(w, body)
		})
	}
	mux.HandleFunc("/src/epath182-report-export.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, markup)
	})
	for path, queue := range map[string]chan []byte{"/epath182-save": saves, "/epath182-download": downloads} {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "POST required", 405)
				return
			}
			data, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			select {
			case queue <- data:
			default:
				http.Error(w, "unexpected additional export", 409)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"filename":"captured.xlsx"}`)
		})
	}
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--window-size=1600,900", "--virtual-time-budget=22000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/src/epath182-report-export.html?manual=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("actual report browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath182-status="passed"`) {
		proof := regexp.MustCompile(`(?s)<pre id="epath182-result"[^>]*>(.*?)</pre>`).FindSubmatch(output)
		if len(proof) == 2 {
			t.Fatalf("actual report browser: %s", proof[1])
		}
		t.Fatalf("actual report browser:\n%s", output)
	}
	if len(saves) != 3 || len(downloads) != 3 {
		t.Fatalf("actual exports saves=%d downloads=%d, want3 each", len(saves), len(downloads))
	}
	var first tabular.WorkbookSheet
	var reports []EnergyPathReportDTO
	for index := 0; index < 3; index++ {
		wire := <-saves
		var request EnergyPathReportXLSXExportRequest
		if err := json.Unmarshal(wire, &request); err != nil {
			t.Fatalf("actual native Save DTO: %v", err)
		}
		if request.IncludeTraceSheets != (index == 1) {
			t.Fatalf("trace checkbox export%d=%t", index, request.IncludeTraceSheets)
		}
		if index != 1 {
			reports = append(reports, request.Report)
		}
		sheets, err := buildEnergyPathReportWorkbook(request)
		if err != nil {
			t.Fatalf("actual report workbook export%d: %v", index, err)
		}
		if len(sheets) == 0 || sheets[0].Name != "Energy Path" {
			t.Fatalf("default report sheet must be Energy Path: %v", epath171SheetNames(sheets))
		}
		if index != 1 && len(sheets) != 1 {
			t.Fatalf("default report leaked trace sheets: %v", epath171SheetNames(sheets))
		}
		if index == 0 {
			first = sheets[0]
		}
		if index == 1 && !reflect.DeepEqual(first, sheets[0]) {
			t.Fatal("checking trace changed primary summary/quality")
		}
		allXML := epath182AssertWorkbookZIP(t, sheets)
		if index != 1 {
			for _, token := range []string{"NODE_PRIVATE182_", "SOURCE_PRIVATE182_", "EDGE_PRIVATE182_"} {
				if strings.Contains(allXML, token) {
					t.Fatalf("default workbook leaked %s", token)
				}
			}
		} else {
			for _, name := range []string{"Energy Path Sources", "Energy Path Links", "Energy Path Accounting", "Energy Path JSON"} {
				if !strings.Contains(strings.Join(epath171SheetNames(sheets), "|"), name) {
					t.Fatalf("trace workbook omitted %s", name)
				}
			}
			for _, token := range []string{"SOURCE_PRIVATE182_", "EDGE_PRIVATE182_"} {
				if !strings.Contains(allXML, token) {
					t.Fatalf("trace workbook omitted %s", token)
				}
			}
		}
		epath182AssertReportWorkbook(t, sheets, wire, index)
	}
	var htmlDocuments []string
	for index := 0; index < 3; index++ {
		var download struct{ Kind, Filename, Text, Raw string }
		if err := json.Unmarshal(<-downloads, &download); err != nil {
			t.Fatal(err)
		}
		if download.Kind == "html" {
			if !strings.Contains(download.Text, "data-energy-path-report") || !strings.Contains(download.Text, "data-energy-path-report-quality") {
				t.Fatal("actual HTML download omitted new report/quality")
			}
			if strings.Contains(strings.ToLower(download.Text), "sankey") {
				t.Fatal("HTML report retained obsolete Sankey terminology")
			}
			htmlDocuments = append(htmlDocuments, download.Text)
		} else if download.Kind == "json" {
			epath181EqualJSON(t, "Full-run JSON keeps original purpose result", []byte(download.Raw), []byte(download.Text))
		} else {
			t.Fatalf("unexpected actual download kind %q", download.Kind)
		}
	}
	if len(htmlDocuments) != 2 {
		t.Fatalf("HTML documents=%d want Building+Zone", len(htmlDocuments))
	}
	for index, html := range htmlDocuments {
		epath182InspectHTML(t, chrome, html, index, reports[index])
	}
}

func epath182AssertWorkbookZIP(t *testing.T, sheets []tabular.WorkbookSheet) string {
	t.Helper()
	var workbook bytes.Buffer
	if err := tabular.WriteWorkbookXLSX(&workbook, sheets); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(workbook.Bytes()), int64(workbook.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, file := range archive.File {
		input, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(input)
		_ = input.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[file.Name] = data
	}
	var book struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := xml.Unmarshal(files["xl/workbook.xml"], &book); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, sheet := range book.Sheets {
		names = append(names, sheet.Name)
	}
	if !reflect.DeepEqual(names, epath171SheetNames(sheets)) {
		t.Fatalf("actual XLSX ZIP sheet names=%v", names)
	}
	for index, sheet := range sheets {
		var parsed struct {
			Rows []struct {
				Cells []struct {
					Value string `xml:"is>t"`
				} `xml:"c"`
			} `xml:"sheetData>row"`
		}
		if err := xml.Unmarshal(files[fmt.Sprintf("xl/worksheets/sheet%d.xml", index+1)], &parsed); err != nil {
			t.Fatal(err)
		}
		position := 0
		for _, section := range sheet.Sections {
			for _, expected := range append([][]string{{"[" + section.Title + "]"}, section.Headers}, section.Rows...) {
				if position >= len(parsed.Rows) || len(parsed.Rows[position].Cells) < len(expected) {
					t.Fatalf("actual XLSX %s row%d truncated", sheet.Name, position)
				}
				for column, value := range expected {
					if parsed.Rows[position].Cells[column].Value != value {
						t.Fatalf("actual XLSX %s row%d col%d=%q want%q", sheet.Name, position, column, parsed.Rows[position].Cells[column].Value, value)
					}
				}
				position++
			}
		}
	}
	return string(bytes.Join(epath171XMLValues(files), nil))
}

func epath182AssertReportWorkbook(t *testing.T, sheets []tabular.WorkbookSheet, wire []byte, index int) {
	t.Helper()
	var payload struct {
		Report struct {
			Context     map[string]any             `json:"context"`
			Summary     json.RawMessage            `json:"summary"`
			QualityRows json.RawMessage            `json:"qualityRows"`
			Trace       map[string]json.RawMessage `json:"trace"`
			RawResult   string                     `json:"rawResult"`
		} `json:"report"`
	}
	if err := json.Unmarshal(wire, &payload); err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(payload.Report.RawResult)) {
		t.Fatal("report did not preserve full original run JSON")
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(payload.Report.RawResult), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["runId"] != "epath182-original" {
		t.Fatalf("async current-result switch contaminated captured run: %v", raw["runId"])
	}
	if len(payload.Report.Summary) == 0 || len(payload.Report.QualityRows) == 0 || len(payload.Report.Trace["nodes"]) == 0 {
		t.Fatal("actual report DTO missing summary/quality/raw scoped trace")
	}
	if index == 2 && !strings.Contains(string(payload.Report.Trace["nodes"]), "service_path_allocation") {
		t.Fatal("Zone M1 report lost allocated basis trace")
	}
	sections := map[string]tabular.Section{}
	for _, section := range sheets[0].Sections {
		sections[section.Title] = section
	}
	var summary map[string][]map[string]any
	// Scope/completeness metadata are not arrays; decode groups individually.
	var summaryObject map[string]json.RawMessage
	if err := json.Unmarshal(payload.Report.Summary, &summaryObject); err != nil {
		t.Fatal(err)
	}
	summary = map[string][]map[string]any{}
	zero, unknown := false, false
	for _, group := range []struct{ key, title string }{{"drivers", "Drivers"}, {"loads", "Loads"}, {"endUses", "End uses"}, {"carriers", "Carriers"}, {"ratios", "Ratios"}, {"residuals", "Residuals"}} {
		if raw := summaryObject[group.key]; len(raw) > 0 {
			var rows []map[string]any
			if err := json.Unmarshal(raw, &rows); err != nil {
				t.Fatal(err)
			}
			summary[group.key] = rows
		}
		section, exists := sections[group.title]
		if !exists {
			t.Fatalf("primary sheet missing %s", group.title)
		}
		if len(section.Rows) != len(summary[group.key]) {
			t.Fatalf("%s report summary row count mismatch", group.title)
		}
		for rowIndex, item := range summary[group.key] {
			value := epath182Cell(item["value"])
			if item["value"] == nil {
				unknown = true
			}
			if value == "0" {
				zero = true
			}
			want := []string{epath182Cell(item["label"]), value, epath182Cell(item["unit"]), epath182BasisDisplay(epath182Cell(item["basis"])), epath182Cell(item["aggregationBasis"]), epath182Cell(item["serviceKind"])}
			if !reflect.DeepEqual(section.Rows[rowIndex], want) {
				t.Fatalf("%s row%d=%v want actual DTO %v", group.title, rowIndex, section.Rows[rowIndex], want)
			}
		}
	}
	if !zero || !unknown {
		t.Fatalf("actual report fixture lost explicit zero/null distinction: zero=%t unknown=%t", zero, unknown)
	}
	var quality []map[string]any
	if err := json.Unmarshal(payload.Report.QualityRows, &quality); err != nil {
		t.Fatal(err)
	}
	if len(quality) != 9 || len(sections["Quality"].Rows) != len(quality) {
		t.Fatal("primary Quality must preserve five availability and four boundary rows")
	}
	for rowIndex, item := range quality {
		want := []string{epath182Cell(item["label"]), epath182Cell(item["value"]), epath182Cell(item["unit"]), epath182Cell(item["status"]), epath182Cell(item["found"]), epath182Cell(item["total"]), epath182Cell(item["message"])}
		if !reflect.DeepEqual(sections["Quality"].Rows[rowIndex], want) {
			t.Fatalf("quality row%d differs from nullable display DTO", rowIndex)
		}
	}
	if payload.Report.Context["summaryService"] != "all" || payload.Report.Context["period"] != func() string {
		if index == 2 {
			return "M1"
		}
		return "annual"
	}() {
		t.Fatal("report context changed selected period or invariant all-service summary")
	}
	if index == 2 {
		account := sections["Zone accounting context"]
		if len(account.Rows) != 1 {
			t.Fatal("Zone default report omitted explicit direct/allocated/unassigned accounting context")
		}
		row := account.Rows[0]
		if row[6] != "2" || row[7] != "4" || row[8] != "4" || row[9] != "10" {
			t.Fatalf("Zone accounting source quantities=%v", row)
		}
	}
	if index == 1 {
		var originalLinks []map[string]any
		if err := json.Unmarshal(payload.Report.Trace["links"], &originalLinks); err != nil {
			t.Fatal(err)
		}
		var joined strings.Builder
		for _, sheet := range sheets {
			if sheet.Name == "Energy Path JSON" {
				for _, section := range sheet.Sections {
					for _, row := range section.Rows {
						joined.WriteString(row[2])
					}
				}
			}
			if sheet.Name == "Energy Path Links" {
				rows := sheet.Sections[0].Rows
				if len(rows) != len(originalLinks) {
					t.Fatal("trace XLSX link rows lost exact canonical links")
				}
				for rowIndex, link := range originalLinks {
					row := rows[rowIndex]
					for column, key := range map[int]string{0: "id", 1: "fromId", 2: "toId", 3: "relation", 4: "fromValue", 5: "fromUnit", 6: "toValue", 7: "toUnit", 8: "ratio", 9: "ratioKind", 10: "basis", 11: "period", 12: "zoneName", 13: "serviceKind"} {
						if row[column] != epath182Cell(link[key]) {
							t.Fatalf("trace XLSX link row%d %s=%q want original%q", rowIndex, key, row[column], epath182Cell(link[key]))
						}
					}
				}
			}
		}
		if joined.String() != payload.Report.RawResult {
			t.Fatal("actual XLSX raw JSON chunks changed original null/zero/unknown fields")
		}
		for _, sheet := range sheets {
			if sheet.Name == "Energy Path Sources" {
				for _, row := range sheet.Sections[0].Rows {
					if row[4] != "J" || row[5] != "kWh" {
						t.Fatal("XLSX source raw J / normalized kWh columns lost")
					}
				}
			}
		}
	}
}

func epath182Cell(value any) string {
	if value == nil {
		return ""
	}
	switch item := value.(type) {
	case float64:
		return strconv.FormatFloat(item, 'g', -1, 64)
	case string:
		return item
	default:
		return fmt.Sprint(item)
	}
}

func epath182BasisDisplay(basis string) string {
	key := strings.ToLower(strings.TrimSpace(basis))
	if strings.Contains(key, "unassigned") {
		return "Unassigned · " + basis
	}
	if strings.Contains(key, "allocat") || strings.Contains(key, "load_share") {
		return "Allocated · " + basis
	}
	if key == "direct_zone_energy" || key == "reported_variable" || key == "reported_meter" || key == "integrated_rate" {
		return "Direct / reported · " + basis
	}
	if basis == "" {
		return "Unavailable"
	}
	return basis
}

func epath182InspectHTML(t *testing.T, chrome, html string, index int, report EnergyPathReportDTO) {
	t.Helper()
	expected := epath181JSON(t, map[string]any{"summary": report.Summary, "qualityRows": report.QualityRows, "context": report.Context})
	inspection := `<script type="application/json" id="epath182-html-expected">` + string(expected) + `</script><script>addEventListener('load',()=>{try{
const expected=JSON.parse(document.getElementById('epath182-html-expected').textContent),report=document.querySelector('[data-energy-path-report]'),trace=report?.querySelector('[data-energy-path-report-trace]'),errors=[];
const check=(condition,message)=>{if(!condition)errors.push(message);},display=value=>value===null||value===undefined?'—':String(value),cells=row=>[...row.cells].map(cell=>cell.textContent.trim());
const visible=report?.innerText||'';
check(!!report&&!!trace&&!trace.open,'report/closed native trace missing');check(!/SOURCE_PRIVATE182_|EDGE_PRIVATE182_|NODE_PRIVATE182_/.test(visible),'raw identifiers visible in default HTML');
check(report?.querySelectorAll('[data-energy-path-report-stage]').length===6,'HTML does not expose exactly six summary stages');
for(const key of ['drivers','loads','endUses','carriers','ratios','residuals']){
 const section=report.querySelector('[data-energy-path-report-stage="'+key+'"]'),rows=[...section.querySelectorAll('tbody tr')],items=expected.summary?.[key]||[];
 if(!items.length){check(rows.length===1&&/No data/i.test(rows[0].textContent),'empty stage invented data: '+key);continue;}
 check(rows.length===items.length,'HTML summary row count: '+key);
 items.forEach((item,index)=>{const row=cells(rows[index]);check(JSON.stringify(row.slice(0,4))===JSON.stringify([item.label||'Unlabeled category',item.serviceKind||'',display(item.value),item.unit||'—']),'HTML summary values differ from transmitted workbook DTO '+key+'/'+index);if(item.basis)check(row[4].includes(item.basis),'HTML basis token lost: '+item.basis);});
}
const quality=[...report.querySelectorAll('[data-energy-path-report-quality] tbody tr')];check(quality.length===expected.qualityRows.length,'HTML quality row count');
expected.qualityRows.forEach((item,index)=>check(JSON.stringify(cells(quality[index]))===JSON.stringify([item.label,display(item.value),item.unit,item.status,display(item.found),display(item.total),item.message]),'HTML nullable quality differs from XLSX DTO '+item.key));
const context=report.querySelector('[data-energy-path-report-context]').textContent;check(context.includes(expected.context.period)&&context.includes('Summary: all services')&&context.includes('Graph selection: '+expected.context.service),'HTML scope/period/service context mismatch');
const intersects=element=>{if(!element)return false;const r=element.getBoundingClientRect();return r.width>0&&r.height>0&&r.bottom>0&&r.top<innerHeight&&r.right>0&&r.left<innerWidth;};check(intersects(report.querySelector('h1,h2'))&&intersects(report.querySelector('table')),'primary report heading/table outside actual viewport');check(document.documentElement.scrollWidth<=innerWidth+1,'default report horizontally overflows');
document.body.dataset.epath182Html=errors.length?'failed':'passed';const pre=document.createElement('pre');pre.id='epath182-html-proof';pre.hidden=true;pre.textContent=JSON.stringify({errors,visible:visible.slice(0,220),stages:report?.querySelectorAll('[data-energy-path-report-stage]').length,trace:!!trace,open:trace?.open});document.body.append(pre);
}catch(error){document.body.dataset.epath182Html='failed';const pre=document.createElement('pre');pre.id='epath182-html-proof';pre.hidden=true;pre.textContent=error.stack||String(error);document.body.append(pre);}});</script>`
	page := strings.Replace(html, "</body>", inspection+"</body>", 1)
	if page == html {
		page = html + inspection
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, page)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	args := []string{"--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--window-size=1600,900", "--virtual-time-budget=2500", "--user-data-dir=" + t.TempDir(), "--dump-dom", server.URL}
	output, err := exec.CommandContext(ctx, chrome, args...).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), `data-epath182-html="passed"`) {
		proof := regexp.MustCompile(`(?s)<pre id="epath182-html-proof"[^>]*>(.*?)</pre>`).FindSubmatch(output)
		t.Fatalf("actual exported HTML%d visibility/structure proof=%s", index, proof)
	}
	mode := "BUILDING"
	if index == 1 {
		mode = "ZONE"
	}
	if screenshot := os.Getenv("EPATH182_SCREENSHOT_" + mode); screenshot != "" {
		args[len(args)-2] = "--screenshot=" + screenshot
		args[7] = "--user-data-dir=" + t.TempDir()
		if out, err := exec.CommandContext(ctx, chrome, args...).CombinedOutput(); err != nil {
			t.Fatalf("actual report screenshot: %v %s", err, out)
		}
	}
}

const epath182ReportBrowserHTML = `<pre id="epath182-result" hidden>pending</pre><script type="module">
const failures=[],proof=[],check=(condition,message)=>{if(!condition)failures.push(message);};
const pause=()=>new Promise(resolve=>setTimeout(resolve,10));
const wait=async(test,label)=>{for(let i=0;i<700;i++){if(test())return;await pause();}throw new Error('Timed out: '+label);};
const freeze=value=>{if(value&&typeof value==='object'){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
try{
 await wait(()=>document.body.dataset.epath142Status==='manual','actual index bootstrap');
 const [{state},simulation,view]=await Promise.all([import('/src/js/state.js'),import('/src/js/views/simulation-views.js'),import('/src/js/views/energy-path-view.js')]);
 const result=JSON.parse(JSON.stringify(state.simulationResult));result.runId='epath182-original';result.filename='Office report.idf';result.originalOptional={zero:0,unknown:null,text:'raw Unicode 한글\r\nkept'};
 const explanation=result.purposeResults.energyExplanation;
 const driverLabels={'surface.exterior_walls':'Exterior walls','surface.roofs':'Roofs','surface.ground_floors':'Ground floors','surface.windows_doors':'Windows and doors','surface.interzone':'Interzone surfaces','air.infiltration':'Infiltration','air.mechanical_ventilation':'Mechanical ventilation','air.interzone':'Interzone air exchange','internal.people':'People','internal.lighting':'Lighting heat','internal.equipment':'Equipment heat','interzone.transfer':'Interzone heat transfer'};
 const endUseLabels={cooling:'Cooling equipment',heating:'Heating equipment',fans:'Fans',lighting:'Lighting',equipment:'Equipment',water_systems:'Water systems',refrigeration:'Refrigeration',other:'Other'};
 for(const source of explanation.sources){source.id='SOURCE_PRIVATE182_'+source.id;source.sourceUnit='J';source.normalizedUnit='kWh';source.rawOptional={zero:0,unknown:null};}
 const modify=graph=>{
  const ids=new Map(graph.nodes.map(node=>[node.id,'NODE_PRIVATE182_'+node.id]));
  for(const node of graph.nodes){node.id=ids.get(node.id);node.sourceIds=node.sourceIds.map(id=>'SOURCE_PRIVATE182_'+id);if(node.level==='driver')node.label=driverLabels[node.driverCategory]||node.label;if(node.level==='end_use')node.label=endUseLabels[node.endUse]||node.label;if(node.level==='end_use'&&node.endUse==='cooling'){node.basis='service_path_allocation';node.allocationApplied=true;}if(node.level==='end_use'&&node.endUse==='lighting'){node.basis='direct_zone_energy';node.allocationApplied=false;}if(node.level==='end_use'&&node.endUse==='refrigeration')node.value=0;if(node.level==='end_use'&&node.endUse==='equipment')node.value=null;}
  for(const link of graph.links){link.id='EDGE_PRIVATE182_'+link.id;link.fromId=ids.get(link.fromId);link.toId=ids.get(link.toId);link.sourceIds=link.sourceIds.map(id=>'SOURCE_PRIVATE182_'+id);if(link.ratioKind==='equipment_efficiency')link.ratioKind='efficiency';}
  graph.summary={...graph.summary,drivers:graph.nodes.filter(n=>n.level==='driver'),loads:graph.nodes.filter(n=>n.level==='load'),endUses:graph.nodes.filter(n=>n.level==='end_use'),carriers:graph.nodes.filter(n=>n.level==='carrier')};
 };
 modify(explanation);explanation.periods.forEach(modify);for(const zone of explanation.zoneResults){modify(zone);zone.periods.forEach(modify);}result.purposeResults.energyExplanationSummary=explanation.summary;
 const zoneMonth=explanation.zoneResults[0].periods[0];zoneMonth.quality={...zoneMonth.quality,zoneAllocationStatus:'partial',zoneAllocatedPct:60,unassignedPct:40};zoneMonth.summary.quality=zoneMonth.quality;zoneMonth.reconciliation=[{id:'ACCOUNT_PRIVATE182_M1',level:'allocation',label:'Building-wide HVAC allocation context',period:'M1',serviceKind:'cooling',basis:'service_path_allocation',allocationMethod:'service_path_load_share',directValue:2,allocatedValue:4,unassignedValue:4,expectedValue:10,unit:'kWh',status:'partial',sourceIds:[explanation.sources[0].id]}];
 const original=JSON.stringify(result);freeze(result);state.simulationResult=result;
 const saved=[],downloaded=[],pending=[],downloadWork=[];let analyze=0,run=0;
 window.go.main.App.AnalyzeInputText=async()=>{analyze++;throw Error('unexpected Analyze');};window.go.main.App.RunSimulationText=async()=>{run++;throw Error('unexpected Run');};
 window.go.main.App.SaveEnergyPathXLSX=request=>{const wire=JSON.stringify(request);saved.push(JSON.parse(wire));return new Promise(resolve=>pending.push(async()=>{const response=await fetch('/epath182-save',{method:'POST',headers:{'Content-Type':'application/json'},body:wire});resolve(await response.json());}));};
 const blobs=new Map(),create=URL.createObjectURL.bind(URL),revoke=URL.revokeObjectURL.bind(URL),anchorClick=HTMLAnchorElement.prototype.click;
 URL.createObjectURL=blob=>{const url=create(blob);blobs.set(url,blob);return url;};URL.revokeObjectURL=url=>{revoke(url);};
 HTMLAnchorElement.prototype.click=function(){if(this.download&&blobs.has(this.href)){const blob=blobs.get(this.href),filename=this.download;downloadWork.push(blob.text().then(async text=>{const kind=blob.type.includes('json')?'json':'html';downloaded.push({kind,text,filename});await fetch('/epath182-download',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({kind,filename,text,raw:original})});}));return;}return anchorClick.call(this);};
 const restore=(scope='building',period='annual',service='cooling')=>{state.simulationResult=result;simulation.restoreSimulationEnergyWorkspaceContext({simulationEnergyScopeKind:scope,simulationEnergyZoneName:scope==='zone'?'Office':'',simulationEnergyPeriod:period,simulationEnergyService:service,simulationEnergySelection:'',simulationEnergyDetailsOpen:true,energyDrawer:{tab:'data',stage:'',outputSource:''}});simulation.renderSimulation();};
 const host=document.getElementById('simulationEnergyDashboard');
 const snapshot=()=>({context:JSON.stringify(simulation.captureSimulationEnergyWorkspaceContext()),canvas:host.querySelector('[data-energy-path-canvas]'),paths:[...host.querySelectorAll('[data-energy-path-ribbon]')].map(path=>[path,path.getAttribute('d')]),selected:state.simulationEnergySelection,layout:globalThis.__epath182LayoutCalls,ribbons:globalThis.__epath182RibbonCalls});
 const unchanged=(before,label)=>{check(host.querySelector('[data-energy-path-canvas]')===before.canvas,label+' rebuilt graph canvas');check(before.paths.every(([path,d])=>path.isConnected&&path.getAttribute('d')===d),label+' rebuilt/recomputed quantitative paths');check(JSON.stringify(simulation.captureSimulationEnergyWorkspaceContext())===before.context,label+' changed context/selection/drawer');check(before.layout===globalThis.__epath182LayoutCalls&&before.ribbons===globalThis.__epath182RibbonCalls,label+' recomputed layout/ribbon geometry');};
 const button=format=>host.querySelector('[data-energy-path-export="'+format+'"]');
 restore();
 check(host.querySelectorAll('[data-energy-path-export-section]').length===1,'export capability missing/duplicated in actual Data details');check(!host.querySelector('[data-energy-path-export-trace]').checked,'XLSX trace must default unchecked');
 const node=host.querySelector('[data-energy-explanation-node]');node.click();let before=snapshot();
 button('xlsx').click();check(saved.length===1,'native XLSX button did not synchronously capture report');check(JSON.stringify(saved[0].report.summary)===JSON.stringify(view.energyPathSummaryForState(explanation,result.purposeResults.energyExplanationSummary,state)),'report diverged from existing UI summary helper');check(saved[0].report.summary.loads.some(item=>item.serviceKind==='heating'),'Cooling-selected report lost invariant Heating summary');check(JSON.stringify(saved[0].report.trace.nodes)===JSON.stringify(explanation.nodes)&&JSON.stringify(saved[0].report.trace.links)===JSON.stringify(explanation.links),'Building trace was filtered or reaggregated');unchanged(before,'default XLSX');await pending.shift()();await wait(()=>!button('xlsx').disabled,'default save settled');
 button('html').click();button('json').click();await Promise.all(downloadWork);unchanged(before,'HTML/JSON');
 host.querySelector('[data-energy-path-export-trace]').click();before=snapshot();button('xlsx').click();check(saved.length===2&&saved[1].includeTraceSheets,'trace checkbox did not enter native request');unchanged(before,'trace capture');
 const stale=button('xlsx');state.simulationResult=freeze({...result,runId:'epath182-next',originalOptional:{different:true}});simulation.renderSimulation();await pending.shift()();
 check(JSON.parse(saved[1].report.rawResult).runId==='epath182-original','async save mixed a newly selected run');
 stale.addEventListener('click',simulation.handleSimulationSeriesInspectClick,{once:true});stale.dispatchEvent(new MouseEvent('click',{bubbles:true,cancelable:true}));check(saved.length===2,'disconnected old export button remained actionable');
 restore('zone','M1','all');before=snapshot();check(!host.querySelector('[data-energy-path-export-trace]').checked,'trace option persisted as hidden global state');
 button('xlsx').click();check(saved.length===3&&!saved[2].includeTraceSheets,'Zone M1 default export option incorrect');check(JSON.stringify(saved[2].report.trace.nodes)===JSON.stringify(zoneMonth.nodes)&&JSON.stringify(saved[2].report.trace.links)===JSON.stringify(zoneMonth.links)&&JSON.stringify(saved[2].report.trace.sources)===JSON.stringify(explanation.sources),'Zone trace lost exact month or root source dictionary');await pending.shift()();await wait(()=>!button('xlsx').disabled,'Zone save settled');button('html').click();await Promise.all(downloadWork);unchanged(before,'Zone M1 export');
 check(saved.every(item=>item.report.rawResult===original),'report snapshot changed original full run JSON');
 check(JSON.stringify(result)===original,'export mutated frozen original payload');check(analyze===0&&run===0,'export called Analyze/Run');
 check(!view.renderEnergyPathView(explanation,state).includes('data-energy-path-export-section'),'standalone/Batch capability-free view exposes main export actions');
 proof.push({saves:saved.length,downloads:downloaded.length,unchanged:true,analyze,run});
}catch(error){failures.push(error.stack||String(error));}
document.body.dataset.epath182Status=failures.length?'failed':'passed';document.getElementById('epath182-result').textContent=JSON.stringify({failures,proof});
</script>`
