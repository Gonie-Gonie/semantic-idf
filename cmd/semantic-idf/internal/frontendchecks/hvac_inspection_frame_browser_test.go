package frontendchecks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

type hvacRuntimeBrowserFixture struct {
	Family    string                    `json:"family"`
	Component idf.HVACOperatingSource   `json:"component"`
	Profile   idf.HVACOperatingProfile  `json:"profile"`
	Topology  idf.HVACLoop              `json:"topology"`
	Runtime   map[string]any            `json:"runtime"`
	Sources   []idf.HVACOperatingSource `json:"sources,omitempty"`
}

func hvacRuntimeBrowserResult(t *testing.T, input string, fixture hvacRuntimeBrowserFixture) map[string]any {
	t.Helper()
	request := simulation.SimulationPurposeRequest{Purposes: []simulation.SimulationPurposeID{simulation.SimulationPurposeHVACLoopCheck}, Scope: simulation.SimulationPurposeScope{LoopMode: "selected", ComponentIDs: []string{fixture.Component.ObjectType + ":" + fixture.Component.ObjectName}}}
	switch fixture.Topology.Type {
	case "AirLoopHVAC":
		request.Scope.AirLoopNames = []string{fixture.Topology.Name}
	case "PlantLoop":
		request.Scope.PlantLoopNames = []string{fixture.Topology.Name}
	case "CondenserLoop":
		request.Scope.CondenserLoopNames = []string{fixture.Topology.Name}
	}
	readLoop := func(series []simulation.SimulationSeries) map[string]any {
		bundle := simulation.BuildPurposeResultBundle(&simulation.SimulationRunResult{InputPath: input, Series: series}, request)
		for _, loop := range bundle.HVACLoops {
			if loop.Name != fixture.Topology.Name || loop.LoopType != fixture.Topology.Type {
				continue
			}
			data, err := json.Marshal(loop)
			if err != nil {
				t.Fatal(err)
			}
			var result map[string]any
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			return result
		}
		t.Fatalf("executed runtime loop %s is missing", fixture.Topology.Name)
		return nil
	}
	// Ask the real result builder for source-backed bindings, then supply
	// deterministic reported values through its ordinary Series input.
	missing := readLoop(nil)
	series := []simulation.SimulationSeries{}
	seen := map[string]bool{}
	bindings, _ := missing["controlSignals"].([]any)
	for _, item := range bindings {
		binding := item.(map[string]any)
		key, _ := binding["keyValue"].(string)
		name, _ := binding["variableName"].(string)
		frequency, _ := binding["reportingFrequency"].(string)
		if key == "" || name == "" || frequency == "" {
			continue
		}
		signature := strings.ToLower(key + "\x00" + name + "\x00" + frequency)
		if seen[signature] {
			continue
		}
		seen[signature] = true
		role, _ := binding["role"].(string)
		value, unit := 1.0, ""
		switch role {
		case "manager_status", "loop_status":
			value = 0
		case "sensor", "setpoint":
			value, unit = 20, "C"
		case "actuator", "actuator_request":
			unit = "kg/s"
		case "demand", "dispatch", "activity":
			value, unit = 1000, "W"
		}
		lower := strings.ToLower(name)
		if strings.Contains(lower, "ratio") || strings.Contains(lower, "fraction") {
			value, unit = 1, ""
		}
		if strings.Contains(lower, "temperature") {
			value, unit = 20, "C"
		}
		if lower == "schedule value" {
			unit = ""
		}
		if strings.Contains(lower, "mass flow rate") {
			value, unit = 1, "kg/s"
		}
		if strings.Contains(lower, "thermostat control type") {
			value, unit = 4, ""
		}
		points := []simulation.SimulationPoint{}
		intervals := []simulation.SimulationNativeInterval{}
		for index, label := range []string{"01-01 00:15", "01-01 00:30", "01-01 00:45", "01-01 01:00"} {
			points = append(points, simulation.SimulationPoint{X: index + 1, Label: label, Value: value})
			timeIndex := int64(index + 1)
			if strings.EqualFold(frequency, "Detailed") {
				timeIndex += 1000
			}
			intervals = append(intervals, simulation.SimulationNativeInterval{TimeIndex: timeIndex, EnvironmentPeriodIndex: 1, IntervalMinutes: 15, SimulationDays: 1, Year: 2026})
		}
		series = append(series, simulation.SimulationSeries{SourceID: fmt.Sprintf("runtime-%s-%d", fixture.Family, len(series)), KeyValue: key, Name: name, File: "eplusout.sql", Column: key + ":" + name + " [" + unit + "](" + frequency + ")", ReportingFrequency: frequency, Points: points, NativeIntervals: intervals})
	}
	return readLoop(series)
}

// Keep runtime inspection grounded in the same source graph as the static
// HVAC view; hand-authored control trees would conceal identity/wire drift.
func hvacRuntimeBrowserFixtures(t *testing.T) []hvacRuntimeBrowserFixture {
	t.Helper()
	input, err := os.ReadFile(repoPath("frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := idf.Parse(string(input))
	if err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(t.TempDir(), "runtime.idf")
	if err := os.WriteFile(inputPath, input, 0600); err != nil {
		t.Fatal(err)
	}
	report := idf.AnalyzeHVAC(doc)
	loops := map[string]idf.HVACLoop{}
	for _, loop := range report.Loops {
		loops[loop.ID] = loop
	}
	fixtures := []hvacRuntimeBrowserFixture{}
	for _, family := range []struct{ name, objectType, objectName, loopType string }{
		{"fan", "Fan:VariableVolume", "VAV_5_Fan", "AirLoopHVAC"},
		{"coil", "Coil:Heating:Water", "VAV_5_HeatC", "AirLoopHVAC"},
		{"chiller", "Chiller:", "", "PlantLoop"},
		{"waterheater", "WaterHeater:Mixed", "SWHSys1 Water Heater", "PlantLoop"},
		{"local", "", "", "ZoneHVAC"},
	} {
		found := false
		for _, profile := range report.OperatingProfiles {
			loop := loops[profile.LoopID]
			if profile.LoopID == "" {
				loop = idf.HVACLoop{Name: "Zone / local equipment", Type: "ZoneHVAC"}
			}
			if profile.ComponentIndex < 0 || profile.ComponentIndex >= len(doc.Objects) || loop.Type != family.loopType {
				continue
			}
			object := doc.Objects[profile.ComponentIndex]
			if !strings.HasPrefix(object.Type, family.objectType) || len(object.Fields) == 0 || family.objectName != "" && object.Fields[0].Value != family.objectName {
				continue
			}
			component := idf.HVACOperatingSource{ObjectIndex: profile.ComponentIndex, ObjectType: object.Type, ObjectName: object.Fields[0].Value}
			fixture := hvacRuntimeBrowserFixture{Family: family.name, Component: component, Profile: profile, Topology: loop}
			fixture.Runtime = hvacRuntimeBrowserResult(t, inputPath, fixture)
			fixtures = append(fixtures, fixture)
			t.Logf("runtime inspection native %s: %s in %s", family.name, component.ObjectName, loop.Name)
			found = true
			break
		}
		if !found {
			t.Fatalf("bundled %s runtime control profile is missing", family.name)
		}
	}
	// These small authored models pass through parsing, HVAC analysis and the
	// ordinary purpose-result builder, retaining real wrapper/tank identities.
	object := func(typ, name string, count int, values map[int]string) idf.Object {
		item := idf.Object{Type: typ, Fields: make([]idf.Field, count)}
		item.Fields[0].Value = name
		for index, value := range values {
			item.Fields[index].Value = value
		}
		labels := map[int]string{}
		if typ == "WaterHeater:Stratified" {
			labels = map[int]string{8: "Heater 1 Setpoint Temperature Schedule Name", 11: "Heater 1 Height", 12: "Heater 2 Setpoint Temperature Schedule Name", 15: "Heater 2 Height", 51: "Number of Nodes"}
		} else if strings.HasPrefix(typ, "WaterHeater:HeatPump:") {
			labels = map[int]string{1: "Availability Schedule Name", 2: "Compressor Setpoint Temperature Schedule Name", 3: "Dead Band Temperature Difference"}
		}
		for index, label := range labels {
			item.Fields[index].Comment = label
		}
		return item
	}
	appendTank := func(family, typ, name string, objects []idf.Object) {
		t.Helper()
		for index := range objects {
			objects[index].Index = index
		}
		text := (idf.Document{Objects: objects}).String()
		authored, err := idf.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), family+".idf")
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		for _, profile := range idf.AnalyzeHVAC(authored).OperatingProfiles {
			candidate := authored.Objects[profile.ComponentIndex]
			if candidate.Type != typ || candidate.Fields[0].Value != name || profile.LoopID != "" {
				continue
			}
			fixture := hvacRuntimeBrowserFixture{Family: family, Component: idf.HVACOperatingSource{ObjectIndex: candidate.Index, ObjectType: candidate.Type, ObjectName: name}, Profile: profile, Topology: idf.HVACLoop{Name: "Zone / local equipment", Type: "ZoneHVAC"}}
			for _, source := range authored.Objects {
				fixture.Sources = append(fixture.Sources, idf.HVACOperatingSource{ObjectIndex: source.Index, ObjectType: source.Type, ObjectName: source.Fields[0].Value})
			}
			fixture.Runtime = hvacRuntimeBrowserResult(t, path, fixture)
			fixtures = append(fixtures, fixture)
			return
		}
		t.Fatalf("authored %s lacks its native runtime profile", family)
	}
	constant := func(name, value string) idf.Object {
		return object("Schedule:Constant", name, 3, map[int]string{2: value})
	}
	appendTank("waterheater-stratified", "WaterHeater:Stratified", "Runtime Layered Tank", []idf.Object{
		constant("Runtime Upper Target", "60"), constant("Runtime Lower Target", "55"),
		object("WaterHeater:Stratified", "Runtime Layered Tank", 52, map[int]string{2: "0.2", 3: "2", 4: "VerticalCylinder", 8: "Runtime Upper Target", 9: "2", 10: "2000", 11: "1.5", 12: "Runtime Lower Target", 13: "2", 14: "1500", 15: "0", 16: "Electricity", 17: "1", 51: "4.0"}),
	})
	for _, variant := range []struct {
		family, typ, name    string
		typeIndex, nameIndex int
	}{
		{"waterheater-hpwh-pumped", "WaterHeater:HeatPump:PumpedCondenser", "Runtime HPWH Pumped", 16, 17},
		{"waterheater-hpwh-wrapped", "WaterHeater:HeatPump:WrappedCondenser", "Runtime HPWH Wrapped", 15, 16},
	} {
		appendTank(variant.family, variant.typ, variant.name, []idf.Object{
			constant("Runtime Always On", "1"), constant("Runtime Compressor Target", "60"), constant("Runtime Backup Target", "50"),
			object("WaterHeater:Mixed", "Runtime Backup Tank", 33, map[int]string{1: "0.2", 2: "Runtime Backup Target", 3: "2", 5: "Cycle", 6: "2000", 10: "Electricity", 11: "1"}),
			object(variant.typ, variant.name, 39, map[int]string{1: "Runtime Always On", 2: "Runtime Compressor Target", 3: "2", variant.typeIndex: "WaterHeater:Mixed", variant.nameIndex: "Runtime Backup Tank"}),
		})
	}
	return fixtures
}

func TestHVACInspectionSharedFrameActualAppBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("actual app HVAC frame controls")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	page := readTestFile(t, "frontend/src/index.html")
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		page = strings.Replace(page, script, "", 1)
	}
	page = strings.Replace(page, "</body>", hvacInspectionFrameHTML+"</body>", 1)
	mux := http.NewServeMux()
	fixtures := hvacRuntimeBrowserFixtures(t)
	mux.HandleFunc("/runtime-fixtures", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fixtures)
	})
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/frame.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	browser := epath201FreshBrowser(t, ctx, chrome)
	simulationBrowserReady(t, ctx, browser, server.URL+"/src/frame.html", "hvacFrameStatus", "hvac-frame-result")
	if directory := os.Getenv("HVAC_RUNTIME_CAPTURE_DIR"); directory != "" {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
		for _, capture := range []struct{ name, query, viewport string }{
			{"fan-loop-dark.png", "?review=fan", "1900,1300"},
			{"coil-control-dark.png", "?review=coil&focus=control", "2560,1500"},
			{"coil-control-narrow.png", "?review=coil&focus=control", "1360,1200"},
			{"chiller-onoff-dispatch.png", "?review=chiller&focus=onoff", "1900,1400"},
			{"waterheater-thermostat.png", "?review=waterheater&focus=onoff", "1900,1400"},
			{"chiller-availability-ko-light.png", "?review=chiller&focus=availability&language=ko&theme=light", "1900,1400"},
			{"missing-required.png", "?review=error&focus=error", "1600,1200"},
			{"fan-availability-narrow.png", "?review=fan&focus=availability", "1360,1200"},
			{"fan-cycleon-override.png", "?review=fan&focus=availability&override=1", "1900,1400"},
			{"fan-native-substeps.png", "?review=fan&focus=availability&transitions=1", "1900,1400"},
			{"coil-legend-highlight.png", "?review=coil&focus=history&legend=1", "2560,1500"},
		} {
			if filter := os.Getenv("HVAC_RUNTIME_CAPTURE_FILTER"); filter != "" && !strings.Contains(","+filter+",", ","+capture.name+",") {
				continue
			}
			t.Run("capture/"+capture.name, func(t *testing.T) {
				path, err := filepath.Abs(filepath.Join(directory, capture.name))
				if err != nil {
					t.Fatal(err)
				}
				var width, height int
				if _, err := fmt.Sscanf(capture.viewport, "%d,%d", &width, &height); err != nil {
					t.Fatal(err)
				}
				captureContext, stop := context.WithTimeout(context.Background(), 30*time.Second)
				defer stop()
				captureBrowser := epath201FreshBrowser(t, captureContext, chrome)
				captureBrowser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": width, "height": height, "deviceScaleFactor": 1, "mobile": false}, nil)
				simulationBrowserReady(t, captureContext, captureBrowser, server.URL+"/src/frame.html"+capture.query, "hvacFrameStatus", "hvac-frame-result")
				simulationBrowserScreenshot(t, captureBrowser, path)
				t.Logf("runtime HVAC visual review: %s", path)
			})
		}
	}
}

// RAF-driven production interactions must finish in an active browser. Chrome's
// dump-dom virtual-time budget may expire with the module still awaiting RAF.
func simulationBrowserReady(t *testing.T, ctx context.Context, browser *epath201Browser, url, statusField, evidenceID string) {
	t.Helper()
	browser.call("Page.navigate", map[string]any{"url": url}, nil)
	for {
		status := string(browser.evaluate(fmt.Sprintf(`document.body?.dataset[%q]`, statusField)))
		if status == `"passed"` {
			return
		}
		if status == `"failed"` {
			t.Fatalf("simulation browser %s: %s", status, browser.evaluate(fmt.Sprintf(`document.getElementById(%q)?.textContent`, evidenceID)))
		}
		select {
		case <-ctx.Done():
			t.Fatalf("simulation browser did not report within its deadline; last status %s (%s)", status, evidenceID)
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func simulationBrowserScreenshot(t *testing.T, browser *epath201Browser, path string) {
	t.Helper()
	browser.evaluate(`new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))`)
	var screenshot struct {
		Data string `json:"data"`
	}
	browser.call("Page.captureScreenshot", map[string]any{"format": "png", "fromSurface": true, "captureBeyondViewport": false}, &screenshot)
	data, err := base64.StdEncoding.DecodeString(screenshot.Data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

const hvacRuntimeBrowserHelpersJS = `
const runtimeClone=(fixtures,family)=>structuredClone(fixtures.find(fixture=>fixture.family===family).runtime);
const selectedRuntimeProfile=(model,fixture)=>model.profiles.find(profile=>profile.definition.componentIndex===fixture.component.objectIndex&&profile.definition.loopId===fixture.profile.loopId);
const setReported=(loop,predicate,values)=>{
 const ids=new Set((loop.controlSignals||[]).filter(predicate).map(signal=>signal.seriesId).filter(Boolean));
 check(ids.size>0,'native runtime fixture lacks the requested observation binding');
 const changed=[];
 for(const series of loop.controlSeries||[])if(ids.has(series.sourceId)){
  delete series.displayPoints;delete series.displayColumn;delete series.displayUnit;
  series.points.forEach((point,index)=>point.value=values[index%values.length]);changed.push(series);
 }
 check(changed.length===ids.size,'bound runtime source was not retained exactly once');
 return changed;
};
const expressionNodes=expression=>expression?[expression,...(expression.children||[]).flatMap(expressionNodes)]:[];
`

const hvacInspectionFrameHTML = `<pre id="hvac-frame-result" hidden>pending</pre>
<script>window.go={main:{App:{GetSimulationEnvironment:async()=>({installations:[],weatherFolders:[]})}}};window.runtime={EventsOn(){}};</script>
<script type="module">
const check=(value,message)=>{if(!value)throw Error(message)};
` + hvacRuntimeBrowserHelpersJS + `
try{
 const [store,simulation,navigation]=await Promise.all([import('/src/js/state.js'),import('/src/js/views/simulation-views.js'),import('/src/js/navigation.js')]);
 const {state}=store;store.setDocumentText('Version,25.1;\nBuilding,Frame check;');simulation.initializeSimulationControls();
 await new Promise(resolve=>setTimeout(resolve,0));
 const loop=(name,hours)=>({name,loopType:'AirLoopHVAC',series:hours.length?[{name:'System Node Temperature',keyValue:name+' node',file:'eplusout.sql',column:name+' node:System Node Temperature [C](Hourly)',reportingFrequency:'Hourly',points:hours.map(x=>({x,value:20+x,label:'01-01 0'+x+':00'}))}]:[],components:[]});
 const result={runId:'frames-1',status:'succeeded',series:[],purposeResults:{hvacLoops:[loop('First',[1,2,3]),loop('Second',[1,3]),loop('Empty',[])]},purposeRunPlan:{outputObjects:[]}};
 Object.assign(state,{report:null,simulationRunning:false,simulationResult:result,simulationProgress:{status:'succeeded',percent:100,message:'Completed'},simulationActiveResultView:'hvac_loops'});
 navigation.switchResultTab('simulation',{recordHistory:false});simulation.renderSimulation();
 const host=document.getElementById('simulationHVACLoopResults'),slider=()=>host.querySelector('[data-simulation-hvac-frame]'),label=()=>host.querySelector('[data-hvac-inspect-frame-label]').textContent;
 const select=name=>{const picker=host.querySelector('[data-simulation-hvac-loop]');picker.value=[...picker.options].find(option=>option.textContent===name).value;picker.dispatchEvent(new Event('change',{bubbles:true}));};
 const frame=index=>{const control=slider();control.value=index;control.dispatchEvent(new Event('input',{bubbles:true}));control.dispatchEvent(new Event('change',{bubbles:true}));};
 const step=direction=>host.querySelector('[data-hvac-inspect-step="'+direction+'"]').click();
 const ctrlFrame=(key,target=host,modifiers={})=>{const event=new KeyboardEvent('keydown',{key,ctrlKey:true,bubbles:true,cancelable:true,...modifiers});target.dispatchEvent(event);return event;};
 check(!document.querySelector('#simulationHVACLoopStats')&&!host.closest('.simulation-section').querySelector('.profile-section-head'),'removed loop heading/count still visible');
 const previous=()=>host.querySelector('[data-hvac-inspect-step="-1"]'),next=()=>host.querySelector('[data-hvac-inspect-step="1"]');
 check(previous()&&next()&&previous().disabled&&!next().disabled&&Boolean(host.querySelector('[data-hvac-inspect-frame-label]').compareDocumentPosition(previous())&Node.DOCUMENT_POSITION_FOLLOWING)&&Boolean(previous().compareDocumentPosition(next())&Node.DOCUMENT_POSITION_FOLLOWING),'one-frame buttons are missing, misordered or not on the right of frame output');
 step(1);check(label()==='01-01 02:00','next button skipped a frame');step(-1);check(label()==='01-01 01:00','previous button skipped a frame');
 check(ctrlFrame('ArrowRight').defaultPrevented&&label()==='01-01 02:00','Ctrl+Right did not advance one native frame');
 slider().focus();ctrlFrame('ArrowLeft',slider());check(label()==='01-01 01:00','Ctrl+Left stopped working after using the frame slider');ctrlFrame('ArrowRight',slider());check(label()==='01-01 02:00','Ctrl+Right did not move one frame from the focused slider');slider().blur();
 ctrlFrame('ArrowLeft');ctrlFrame('ArrowLeft');check(label()==='01-01 01:00'&&previous().disabled,'Ctrl+Left crossed the first frame');
 ctrlFrame('ArrowRight');ctrlFrame('ArrowRight');ctrlFrame('ArrowRight');check(label()==='01-01 03:00'&&next().disabled,'Ctrl+Right crossed the last frame');
 frame(1);check(!ctrlFrame('ArrowRight',host,{ctrlKey:false}).defaultPrevented&&label()==='01-01 02:00','plain ArrowRight was hijacked by the HVAC shortcut');
 for(const modifier of ['shiftKey','altKey','metaKey'])check(!ctrlFrame('ArrowRight',host,{[modifier]:true}).defaultPrevented&&label()==='01-01 02:00','HVAC shortcut hijacked modified '+modifier+' activation');
 for(const kind of ['input','textarea','select','editable','editor']){const edit=document.createElement(['editable','editor'].includes(kind)?'div':kind);if(kind==='input')edit.type='text';if(kind==='editable')edit.contentEditable='true';if(kind==='editor'){edit.className='editor-panel';edit.tabIndex=0;}host.append(edit);edit.focus();const event=ctrlFrame('ArrowRight',edit);check(!event.defaultPrevented&&label()==='01-01 02:00','HVAC shortcut hijacked '+kind+' editing');edit.remove();}
 const iframe=document.createElement('iframe');iframe.tabIndex=0;host.append(iframe);iframe.focus();ctrlFrame('ArrowRight');check(label()==='01-01 02:00','HVAC shortcut acted through a focused iframe');iframe.remove();
 const modal=document.createElement('dialog'),modalButton=document.createElement('button');modal.append(modalButton);document.body.append(modal);modal.showModal();modalButton.focus();ctrlFrame('ArrowRight',modalButton);check(label()==='01-01 02:00','HVAC shortcut acted behind an auxiliary dialog');modal.close();modal.remove();
 navigation.switchResultTab('metrics',{recordHistory:false});ctrlFrame('ArrowRight');check(label()==='01-01 02:00','HVAC shortcut acted in another result tab');navigation.switchResultTab('simulation',{recordHistory:false});
 state.simulationActiveResultView='series';ctrlFrame('ArrowRight');check(label()==='01-01 02:00','HVAC shortcut acted in another simulation view');state.simulationActiveResultView='hvac_loops';
 frame(2);check(label()==='01-01 03:00','frame slider did not change selected hour');
 select('Second');check(slider().value==='1'&&label()==='01-01 03:00','loop switch reset timestamp or used old array index');
 select('Empty');check(previous().disabled&&next().disabled&&slider().disabled,'empty loop retained enabled frame controls');step(1);ctrlFrame('ArrowLeft');select('First');check(slider().value==='2'&&label()==='01-01 03:00','empty loop discarded shared frame');
 select('Second');frame(0);step(1);check(label()==='01-01 03:00','one-frame button invented the absent second-loop hour');ctrlFrame('ArrowLeft');check(label()==='01-01 01:00','one-frame shortcut did not use the prior available observation');select('First');check(slider().value==='0'&&label()==='01-01 01:00','frame selected in second loop was not shared');
 frame(2);state.simulationResult={...result,runId:'frames-2'};simulation.renderSimulation();check(slider().value==='0','new simulation retained previous run timestamp');

 // The loop's Hourly cursor is shared; history retains native Timestep data.
 const fixtures=await(await fetch('/runtime-fixtures')).json(),fixturesBefore=JSON.stringify(fixtures),fanFixture=fixtures.find(fixture=>fixture.family==='fan'),coilFixture=fixtures.find(fixture=>fixture.family==='coil'),chillerFixture=fixtures.find(fixture=>fixture.family==='chiller'),waterHeaterFixture=fixtures.find(fixture=>fixture.family==='waterheater'),localFixture=fixtures.find(fixture=>fixture.family==='local');
 const {prepareHVACInspection}=await import('/src/js/hvac-inspection-data.js'),frameAt=(captured,wanted)=>{const index=prepareHVACInspection(captured).frames.findIndex(frame=>frame.label===wanted);check(index>=0,'main result lacks native frame '+wanted);frame(index);};
 const nativeMember=fixture=>[fixture.topology.supplySide,fixture.topology.demandSide].flatMap(side=>side?.branches||[]).flatMap(branch=>branch.components||[]).find(item=>item.objectType===fixture.component.objectType&&item.objectName===fixture.component.objectName);
 const hourlySeries=(fixture,key,name,unit,values)=>({name,keyValue:key,file:'eplusout.sql',column:key+':'+name+' ['+unit+'](Hourly)',reportingFrequency:'Hourly',points:[1,2,3].map((hour,index)=>({x:hour,label:'01-01 0'+hour+':00',value:values[index%values.length]}))});
 const withHourly=(captured,fixture)=>{
  for(const series of captured.controlSeries||[]){const observed=series.points;delete series.displayPoints;delete series.displayColumn;delete series.displayUnit;delete series.nativeIntervalsSourceId;series.points=Array.from({length:12},(_,index)=>{const minutes=(index+1)*15;return {x:index+1,label:'01-01 '+String(Math.floor(minutes/60)).padStart(2,'0')+':'+String(minutes%60).padStart(2,'0'),value:observed[index%observed.length].value};});series.nativeIntervals=series.points.map((point,index)=>({timeIndex:(series.reportingFrequency==='Detailed'?1000:0)+index+1,environmentPeriodIndex:1,intervalMinutes:15,simulationDays:1,year:2026}));}
  const member=nativeMember(fixture);check(member,'native equipment is absent from its typed loop');
  const node=member.inletNode||fixture.topology.supplySide?.inletNode;check(node,'native equipment lacks a connected node');
  captured.series=[hourlySeries(fixture,node,'System Node Temperature','C',[21,22,23]),hourlySeries(fixture,node,'System Node Mass Flow Rate','kg/s',[1,2,3])];
  if(fixture.topology.type==='AirLoopHVAC')captured.series.push(hourlySeries(fixture,node,'System Node Relative Humidity','%',[41,42,43]));
  const activity=captured.controlSignals.find(signal=>signal.role==='activity'&&signal.source.objectIndex===fixture.component.objectIndex);check(activity,'native fixture lacks equipment response');
  captured.components=[{componentName:fixture.component.objectName,componentType:fixture.component.objectType,inletNodes:[member.inletNode].filter(Boolean),outletNodes:[member.outletNode].filter(Boolean),series:[hourlySeries(fixture,fixture.component.objectName,activity.variableName,'W',[1000,0,1500])]}];return captured;
 };
 const mergeEquipment=(base,other)=>{
  const signature=series=>[series.keyValue,series.name,series.reportingFrequency].map(value=>String(value).toLowerCase()).join('\0'),bySource=new Map(base.controlSeries.map(series=>[signature(series),series.sourceId])),aliases=new Map();
  for(const series of other.controlSeries){const key=signature(series);if(bySource.has(key))aliases.set(series.sourceId,bySource.get(key));else{bySource.set(key,series.sourceId);base.controlSeries.push(series);}}
  for(const binding of other.controlSignals){if(aliases.has(binding.seriesId))binding.seriesId=aliases.get(binding.seriesId);base.controlSignals.push(binding);}
  base.operatingProfiles.push(...other.operatingProfiles);base.components.push(...other.components);
  const nodes=new Set(base.series.map(signature));for(const series of other.series)if(!nodes.has(signature(series))){nodes.add(signature(series));base.series.push(series);}return base;
 };
 const runtimeFan=mergeEquipment(withHourly(runtimeClone(fixtures,'fan'),fanFixture),withHourly(runtimeClone(fixtures,'coil'),coilFixture)),runtimeChiller=withHourly(runtimeClone(fixtures,'chiller'),chillerFixture);
 const chillerDispatch=runtimeChiller.controlSignals.find(signal=>signal.role==='dispatch'&&signal.graph==='onOff'&&signal.nodeId==='decision'&&signal.source.objectIndex===chillerFixture.component.objectIndex&&signal.source.objectType===chillerFixture.component.objectType&&signal.source.objectName===chillerFixture.component.objectName);
 check(chillerDispatch?.variableName==='Plant Component Distributed Demand Rate'&&chillerDispatch.keyValue===chillerFixture.component.objectName&&chillerDispatch.reportingFrequency==='Timestep'&&chillerDispatch.required,'plant dispatch is not bound to the exact selected component native assignment');
 setReported(runtimeChiller,signal=>signal.role==='demand',[20000]);setReported(runtimeChiller,signal=>signal.role==='dispatch',[0,0,0,0,-6000,-6000,-6000,-6000,-6000,-6000,-6000,-6000]);
 setReported(runtimeChiller,signal=>signal.role==='activity',[0,0,0,0,0,0,0,0,6000,6000,6000,6000]);
 for(const signal of runtimeChiller.controlSignals.filter(signal=>signal.role==='activity'&&/ratio|fraction/i.test(signal.variableName)))setReported(runtimeChiller,entry=>entry.id===signal.id,[0,0,0,0,0,0,0,0,.6,.6,.6,.6]);
 const managerBinding=runtimeFan.controlSignals.find(signal=>signal.componentIndex===fanFixture.component.objectIndex&&signal.role==='manager_status');check(managerBinding,'native fan lacks its manager');
 const fanGate=expressionNodes(fanFixture.profile.finalAvailability).find(expression=>expression.op==='fan_gate'),ownSchedule=expressionNodes(fanGate?.children?.[1]).find(expression=>expression.op==='schedule_positive')?.scheduleName;check(ownSchedule,'native fan lacks its own schedule');
 setReported(runtimeFan,signal=>signal.role==='schedule'&&signal.keyValue.toLowerCase()===ownSchedule.toLowerCase(),[1,1,1,1,0,0,0,0,1,1,1,1]);
 setReported(runtimeFan,signal=>signal.role==='manager_status'||signal.role==='loop_status',[0,0,0,0,2,0,2,0,2,0,2,2]);
 const nativeStatusTransitions=captured=>{
  const ids=new Set(captured.controlSignals.filter(signal=>signal.role==='manager_status'||signal.role==='loop_status').map(signal=>signal.seriesId));
  for(const series of captured.controlSeries)if(ids.has(series.sourceId)){
   check(series.reportingFrequency==='Detailed','availability status was collected as an interval-averaged output');
   const minutes=[5,...Array.from({length:12},(_,index)=>(index+1)*15)],values=[0,2,0,0,0,2,0,2,0,2,0,2,2];
   series.points=minutes.map((minute,index)=>({x:index+1,label:'01-01 '+String(Math.floor(minute/60)).padStart(2,'0')+':'+String(minute%60).padStart(2,'0'),value:values[index]}));
   series.nativeIntervals=minutes.map((minute,index)=>({timeIndex:1001+index,environmentPeriodIndex:1,intervalMinutes:minute-(minutes[index-1]||0),simulationDays:1,year:2026}));
  }
 };
 nativeStatusTransitions(runtimeFan);
 setReported(runtimeFan,signal=>signal.role==='activity',[1000,1000,1000,1000,0,0,0,0,1500,1500,1500,1500]);
 const runtimeResult={...result,runId:'runtime-1',purposeResults:{hvacLoops:[runtimeFan,runtimeChiller]}};
 state.report={hvac:{operatingProfiles:[{componentIndex:fanFixture.component.objectIndex,loopId:fanFixture.profile.loopId,parameterControl:{nodes:[{id:'editor-only',source:{objectName:'EDITOR MODEL MUST NOT APPEAR'}}],edges:[]}}]}};
 const foreignProfile=structuredClone(runtimeFan.operatingProfiles.find(profile=>profile.componentIndex===fanFixture.component.objectIndex));foreignProfile.loopId='another-native-circuit';foreignProfile.parameterControl.nodes.forEach(node=>node.source.objectName='FOREIGN LOOP MUST NOT APPEAR');runtimeFan.operatingProfiles.unshift(foreignProfile);
 state.simulationResult=runtimeResult;simulation.renderSimulation();
 const componentTarget=fixture=>[...host.querySelectorAll('[data-hvac-inspect-component]')].find(target=>target.dataset.hvacInspectPointName===fixture.component.objectName&&target.dataset.hvacInspectComponent.toLowerCase().includes(fixture.component.objectType.toLowerCase()));
 const chooseEquipment=fixture=>{const target=componentTarget(fixture);check(target,'typed equipment is not clickable: '+fixture.component.objectName);target.dispatchEvent(new MouseEvent('click',{bubbles:true}));};
 const runtimePanel=()=>host.querySelector('[data-hvac-runtime]'),stack=()=>host.querySelector('[data-hvac-runtime-availability]'),finalRow=()=>stack()?.querySelector('[data-hvac-runtime-availability-row="final-availability"]'),onOffPanel=()=>host.querySelector('[data-hvac-runtime-onoff]');
 const bottomFrame=()=>{const control=slider()?.closest('.hvac-inspection-frame-control'),snapshot=host.querySelector('[data-hvac-inspect-snapshot]'),custom=host.querySelector('.hvac-inspection-custom');return control&&snapshot&&custom&&Boolean(snapshot.compareDocumentPosition(control)&Node.DOCUMENT_POSITION_FOLLOWING)&&Boolean(custom.compareDocumentPosition(control)&Node.DOCUMENT_POSITION_FOLLOWING)&&(!runtimePanel()||Boolean(runtimePanel().compareDocumentPosition(control)&Node.DOCUMENT_POSITION_FOLLOWING));};
 check(host.querySelector('[data-hvac-inspect-topology]')&&!runtimePanel(),'default result is not basic loop view');
 check(!host.querySelector('select[data-hvac-inspect-view], select[data-hvac-runtime-profile], [data-hvac-runtime-frame]'),'loop view retained separate view/equipment/time selectors');
 check(bottomFrame(),'shared frame bar is not below loop panels');
 const nodeCharts=host.querySelector('[data-hvac-inspect-basic-charts]'),customControls=host.querySelector('[data-hvac-inspect-custom-controls]'),nodeToggles=[...host.querySelectorAll('[data-hvac-inspect-node-visible]')].map(input=>input.dataset.hvacInspectNodeVisible);
 chooseEquipment(fanFixture);
 check(runtimePanel()&&host.querySelector('[data-hvac-inspect-topology]')&&host.querySelector('[data-hvac-runtime-control]')&&host.querySelector('[data-hvac-runtime-history]'),'equipment selection replaced loop instead of inline control history');
 check(!host.textContent.includes('EDITOR MODEL MUST NOT APPEAR')&&!host.textContent.includes('FOREIGN LOOP MUST NOT APPEAR')&&runtimePanel().textContent.includes(fanFixture.component.objectName),'equipment selection borrowed editor/foreign-loop profile');
 check(!host.querySelector('[data-hvac-runtime-error]')&&bottomFrame(),'exact-time controls failed or frame moved above details');
 const nativeFanConditions=expressionNodes(fanFixture.profile.onOffExpression).filter(expression=>expression.referenceId),fanCommand=onOffPanel()?.querySelector('[data-hvac-boolean-definition="command"]');
 check(onOffPanel()&&onOffPanel().querySelector('[data-hvac-operation-graph="onoff"]')&&nativeFanConditions.length&&nativeFanConditions.every(expression=>fanCommand?.querySelector('[data-hvac-inspect-condition="'+CSS.escape(expression.referenceId)+'"]'))&&fanCommand.querySelector('[data-hvac-expression-reference="availability"]'),'runtime OnOff panel omitted the executed native condition AST or availability operand');
 check(!stack().contains(onOffPanel())&&!onOffPanel().contains(stack())&&onOffPanel().querySelector('[data-hvac-runtime-decision-permission]').textContent==='Allowed'&&onOffPanel().querySelector('[data-hvac-runtime-decision-response]').textContent==='Activity reported','run permission, native decision inputs and reported operation were conflated');
 const resultBounds=host.closest('.result-pane').getBoundingClientRect(),bounded=[stack(),slider().closest('.hvac-inspection-frame-control'),host.querySelector('[data-hvac-inspect-frame-label]')];
 check(bounded.every(element=>{const rect=element.getBoundingClientRect();return rect.left>=resultBounds.left-1&&rect.right<=resultBounds.right+1;}),'loop min-content widened schedule/status/frame beyond visible pane: '+JSON.stringify({pane:[resultBounds.left,resultBounds.right],items:bounded.map(element=>{const rect=element.getBoundingClientRect();return [element.className,rect.left,rect.right];})}));
 check(host.querySelector('[data-hvac-inspect-basic-charts]')===nodeCharts&&host.querySelector('[data-hvac-inspect-custom-controls]')===customControls&&JSON.stringify([...host.querySelectorAll('[data-hvac-inspect-node-visible]')].map(input=>input.dataset.hvacInspectNodeVisible))===JSON.stringify(nodeToggles),'equipment selection changed existing node UI');
 check(!host.querySelector('[data-hvac-runtime-signal-list], .hvac-runtime-table, [data-hvac-runtime-source-detail], [data-hvac-runtime-decisions], select[data-hvac-runtime-profile], [data-hvac-runtime-frame]'),'equipment retained variable/spec tables or duplicate pickers');
 check(label()==='01-01 01:00'&&finalRow()?.dataset.state==='allowed','Hourly1h did not bind exact control1h observation');
 check([...stack().querySelectorAll('[data-hvac-runtime-availability-row]')].at(-1)===finalRow(),'final Availability is not bottom of stack');
 check(stack().querySelector('[data-kind="schedule"]')&&stack().querySelector('[data-kind="manager"]'),'schedule/manager absent from stack');
 const nativeManagerRow=[...stack().querySelectorAll('[data-kind="manager"]')].find(row=>row.querySelector('strong')?.textContent===managerBinding.source.objectName),nativePatches=[...nativeManagerRow.querySelectorAll('.hvac-runtime-patch')];
 check(nativePatches[0]?.classList.contains('is-no_action')&&nativePatches[1]?.classList.contains('is-cycle_on')&&Math.abs(Number(nativePatches[0].getAttribute('width'))-1000*5/180)<1e-6&&Math.abs(Number(nativePatches[1].getAttribute('width'))-1000*10/180)<1e-6,'Detailed NoAction/CycleOn substeps were averaged, binned or drawn at equal widths');
 check(!stack().querySelector('[data-state="mixed"],.is-mixed,[data-hvac-availability-legend="mixed"]'),'exact native availability displayed an ambiguous mixed state');
 check(['allowed','blocked','no_action','cycle_on','zone_fans_only','error','decisive','frame'].every(group=>stack().querySelector('[data-hvac-availability-legend="'+group+'"]')?.textContent.trim()),'Availability legend does not distinguish schedules, native manager actions, acquisition errors, deciding conditions and the current frame');
 const availabilityHover=(group,predicate)=>{
  const section=stack(),legend=section.querySelector('[data-hvac-availability-legend="'+group+'"]'),patches=[...section.querySelectorAll('.hvac-runtime-patch')],matching=patches.filter(predicate);
  check(matching.length>0,'native fixture lacks patches for '+group+' legend review');legend.dispatchEvent(new PointerEvent('pointerover',{bubbles:true}));
  check(section.dataset.hvacAvailabilityHighlight===group&&legend.classList.contains('is-highlighted')&&patches.every(patch=>patch.classList.contains('is-highlighted')===predicate(patch)&&patch.classList.contains('is-muted')!==predicate(patch)),'Availability legend did not isolate '+group+' patches');
  if(group!=='decisive')check(getComputedStyle(matching[0]).fill===getComputedStyle(legend.querySelector('i')).backgroundColor,'Availability legend color differs from its '+group+' patches');
  legend.dispatchEvent(new PointerEvent('pointerout',{bubbles:true,relatedTarget:section}));check(!section.dataset.hvacAvailabilityHighlight&&!section.querySelector('.hvac-runtime-patch.is-muted,.hvac-runtime-patch.is-highlighted'),'Availability legend leave retained dimmed or highlighted patches');
 };
 for(const group of ['allowed','blocked','no_action','cycle_on'])availabilityHover(group,patch=>patch.classList.contains('is-'+group));
 const availabilitySection=stack(),availabilityRows=[...availabilitySection.querySelectorAll('[data-hvac-runtime-availability-row]')],availabilitySVGs=availabilityRows.map(row=>row.querySelector('svg')),availabilityGroups=availabilityRows.map(row=>row.querySelector('[data-hvac-runtime-patches]'));
 const availabilityRange=()=>[Number(stack().dataset.hvacAvailabilityLow),Number(stack().dataset.hvacAvailabilityHigh)],fullAvailabilityRange=availabilityRange(),availabilityLabels=()=>[stack().querySelector('[data-hvac-availability-start]').textContent,stack().querySelector('[data-hvac-availability-end]').textContent],fullAvailabilityLabels=availabilityLabels();
 const availabilitySettle=()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
 const untouchedSVGs=[...nodeCharts.querySelectorAll('svg.hvac-chart-svg'),...runtimePanel().querySelectorAll('[data-hvac-runtime-history] svg.hvac-chart-svg')],untouchedPaths=untouchedSVGs.map(svg=>[...svg.querySelectorAll('path.hvac-chart-line')].map(path=>path.getAttribute('d')));
 const availabilityGeometry=()=>availabilityGroups.map(group=>group.innerHTML),availabilityBefore=availabilityGeometry(),availabilityCursor=()=>Number(availabilitySVGs[0].querySelector('[data-hvac-runtime-cursor]').getAttribute('x1'));
 const availabilityTime=fullAvailabilityRange[0]+availabilityCursor()/1000*(fullAvailabilityRange[1]-fullAvailabilityRange[0]),availabilityPointer=.4,availabilityRect=availabilitySVGs[0].getBoundingClientRect(),availabilityWheel=new WheelEvent('wheel',{bubbles:true,cancelable:true,deltaY:-180,clientX:availabilityRect.left+availabilityRect.width*availabilityPointer,clientY:availabilityRect.top+availabilityRect.height/2});
 availabilitySVGs[0].dispatchEvent(availabilityWheel);await availabilitySettle();
 const zoomAvailabilityRange=availabilityRange(),zoomAvailabilitySpan=zoomAvailabilityRange[1]-zoomAvailabilityRange[0],actualAvailabilityPointer=(availabilityWheel.clientX-availabilityRect.left)/availabilityRect.width,availabilityAnchor=fullAvailabilityRange[0]+actualAvailabilityPointer*(fullAvailabilityRange[1]-fullAvailabilityRange[0]);
 check(availabilityWheel.defaultPrevented&&stack()===availabilitySection&&zoomAvailabilitySpan<fullAvailabilityRange[1]-fullAvailabilityRange[0]&&Math.abs(zoomAvailabilityRange[0]+actualAvailabilityPointer*zoomAvailabilitySpan-availabilityAnchor)<1e-7,'Availability wheel did not narrow its shared horizontal window about the pointer');
 check(availabilityLabels().every((label,index)=>label!==fullAvailabilityLabels[index])&&availabilityGeometry().some((geometry,index)=>geometry!==availabilityBefore[index]),'Availability zoom left stale axis labels or patch geometry');
 const sharedAvailabilityViewport=time=>{
  const visible=time>=zoomAvailabilityRange[0]&&time<=zoomAvailabilityRange[1],expected=(time-zoomAvailabilityRange[0])/zoomAvailabilitySpan*1000;
  return availabilityRows.every((row,index)=>{
   const cursor=row.querySelector('[data-hvac-runtime-cursor]'),aligned=cursor&&(visible?cursor.style.display!=='none'&&Math.abs(Number(cursor.getAttribute('x1'))-expected)<1e-6&&Math.abs(Number(cursor.getAttribute('x2'))-expected)<1e-6:cursor.style.display==='none');
   return row.querySelector('svg')===availabilitySVGs[index]&&row.querySelector('[data-hvac-runtime-patches]')===availabilityGroups[index]&&aligned&&[...row.querySelectorAll('.hvac-runtime-patch')].every(patch=>Number(patch.getAttribute('x'))>=-1e-7&&Number(patch.getAttribute('x'))+Number(patch.getAttribute('width'))<=1000+1e-7);
  });
 };
 check(sharedAvailabilityViewport(availabilityTime),'Availability input/final rows do not share clipped patches and the current-frame cursor after zoom');
 const availabilityZoomGeometry=availabilityGeometry();step(1);
 check(label()==='01-01 02:00'&&availabilityRange().every((value,index)=>Math.abs(value-zoomAvailabilityRange[index])<1e-7)&&sharedAvailabilityViewport(availabilityTime+1)&&availabilityGeometry().every((geometry,index)=>geometry===availabilityZoomGeometry[index]),'frame step reset Availability zoom, shifted its rows independently or redrew patches');
 step(-1);availabilitySVGs[0].dispatchEvent(new MouseEvent('dblclick',{bubbles:true,cancelable:true}));await availabilitySettle();
 check(label()==='01-01 01:00'&&availabilityRange().every((value,index)=>Math.abs(value-fullAvailabilityRange[index])<1e-7)&&availabilityLabels().every((label,index)=>label===fullAvailabilityLabels[index])&&availabilityGeometry().every((geometry,index)=>geometry===availabilityBefore[index]),'Availability reset did not restore its exact full time window and patch geometry');
 check(untouchedSVGs.every((svg,index)=>svg.isConnected&&JSON.stringify([...svg.querySelectorAll('path.hvac-chart-line')].map(path=>path.getAttribute('d')))===JSON.stringify(untouchedPaths[index])),'Availability zoom/frame/reset rebuilt or changed unrelated node and signal graph geometry');
 const fanControlCard=()=>[...host.querySelectorAll('.hvac-parameter-flow-stage.controls [data-hvac-flow-card]')].find(card=>card.querySelector('strong')?.textContent==='Match airflow');
 check(fanControlCard()&&!fanControlCard().querySelector('[data-hvac-runtime-card-value]'),'fan controller borrowed unrelated temperature');
 step(1);check(label()==='01-01 02:00'&&finalRow()?.dataset.state==='blocked'&&stack().querySelector('[data-kind="schedule"][data-decisive="true"]'),'next frame button did not update the OFF schedule and its deciding condition together');
 availabilityHover('decisive',patch=>patch.closest('[data-hvac-runtime-availability-row]').dataset.decisive==='true');
 const frameLegend=stack().querySelector('[data-hvac-availability-legend="frame"]');frameLegend.dispatchEvent(new FocusEvent('focusin',{bubbles:true}));check([...stack().querySelectorAll('[data-hvac-runtime-cursor]')].every(cursor=>cursor.classList.contains('is-highlighted')),'Availability current-frame legend did not highlight the shared time cursors');frameLegend.dispatchEvent(new FocusEvent('focusout',{bubbles:true,relatedTarget:stack()}));check(!stack().querySelector('[data-hvac-runtime-cursor].is-highlighted'),'Availability current-frame legend blur left highlighted cursors');
 ctrlFrame('ArrowLeft');check(label()==='01-01 01:00'&&finalRow()?.dataset.state==='allowed','Ctrl+Left did not update runtime Availability with the shared frame');
 ctrlFrame('ArrowRight');check(label()==='01-01 02:00'&&finalRow()?.dataset.state==='blocked','Ctrl+Right did not update runtime Availability with the shared frame');
 frame(2);check(label()==='01-01 03:00'&&finalRow()?.dataset.state==='allowed','CycleOn did not follow shared cursor');
 const nodeTarget=host.querySelector('[data-hvac-inspect-node]');check(nodeTarget,'loop node is not selectable');nodeTarget.dispatchEvent(new MouseEvent('click',{bubbles:true}));
 check(!runtimePanel()&&host.querySelector('[data-hvac-inspect-basic-charts]')===nodeCharts&&host.querySelector('[data-hvac-inspect-custom-controls]')===customControls&&label()==='01-01 03:00','node selection changed existing UI/current frame');
 chooseEquipment(coilFixture);check(runtimePanel()?.textContent.includes(coilFixture.component.objectName)&&!host.querySelector('[data-hvac-runtime-error]'),'same-loop equipment did not select exact controls');
 select(runtimeChiller.name);check(label()==='01-01 03:00'&&!runtimePanel(),'loop switch retained another circuit equipment');chooseEquipment(chillerFixture);
 check(runtimePanel()?.textContent.includes(chillerFixture.component.objectName)&&!host.querySelector('[data-hvac-runtime-error]'),'chiller borrowed fan/coil profile');
 const chillerDecision=onOffPanel(),decisionReading=binding=>chillerDecision.querySelector('[data-hvac-runtime-value="'+CSS.escape(binding.id)+'"]')?.textContent,chillerDemand=runtimeChiller.controlSignals.find(signal=>signal.role==='demand'&&chillerDecision.querySelector('[data-hvac-runtime-value="'+CSS.escape(signal.id)+'"]')),chillerActivity=runtimeChiller.controlSignals.find(signal=>signal.role==='activity'&&/evaporator cooling rate/i.test(signal.variableName));
 check(chillerActivity&&chillerDecision.querySelector('[data-hvac-inspect-condition="decision"]')&&chillerDecision.querySelector('[data-hvac-boolean-definition="command"]'),'chiller native load-dispatch decision is missing from the runtime AST');
 frameAt(runtimeChiller,'01-01 01:00');check(finalRow()?.dataset.state==='allowed'&&decisionReading(chillerDispatch)==='0.00 kW'&&decisionReading(chillerDemand)==='20.00 kW'&&decisionReading(chillerActivity)==='0.00 kW'&&chillerDecision.querySelector('[data-hvac-runtime-decision-response]').textContent==='No activity reported','positive whole-loop demand was mistaken for component assignment or operation');
 const operationFormula=chillerDecision.querySelector('[data-hvac-boolean-definition="availability"]'),commandFormula=chillerDecision.querySelector('[data-hvac-boolean-definition="command"]'),minimumPermission=chillerDecision.querySelector('[data-hvac-runtime-decision-permission]');
 check(operationFormula?.querySelector('.hvac-expression-lhs')?.textContent==='Operation conditions'&&commandFormula?.querySelector('[data-hvac-expression-reference="availability"]')?.textContent==='Operation conditions'&&minimumPermission.closest('article').querySelector('h5')?.textContent==='Availability'&&minimumPermission.textContent==='Allowed'&&operationFormula.querySelector('[data-hvac-condition-reason="runtime_dispatch"]'),'zero dispatched load conflated native operation conditions with minimum Availability, or On command references a different formula label');
 frameAt(runtimeChiller,'01-01 02:00');check(onOffPanel()===chillerDecision&&decisionReading(chillerDispatch)==='-6.00 kW'&&decisionReading(chillerDemand)==='20.00 kW'&&decisionReading(chillerActivity)==='0.00 kW'&&chillerDecision.querySelector('[data-hvac-runtime-decision-response]').textContent==='No activity reported','signed cooling assignment was lost, inferred actual equipment operation or failed to follow the frame: '+JSON.stringify({samePanel:onOffPanel()===chillerDecision,dispatch:decisionReading(chillerDispatch),demand:decisionReading(chillerDemand),activity:decisionReading(chillerActivity),response:chillerDecision.querySelector('[data-hvac-runtime-decision-response]').textContent,label:label()}));
 frameAt(runtimeChiller,'01-01 03:00');check(onOffPanel()===chillerDecision&&decisionReading(chillerDispatch)==='-6.00 kW'&&decisionReading(chillerActivity)==='6.00 kW'&&chillerDecision.querySelector('[data-hvac-runtime-decision-response]').textContent==='Activity reported','reported operation did not update independently of permission and signed dispatch');
 select(runtimeFan.name);check(label()==='01-01 03:00','loop switch discarded shared time');
 state.simulationResult={...runtimeResult,runId:'runtime-2'};simulation.renderSimulation();check(slider().value==='0'&&label()==='01-01 01:00'&&!runtimePanel(),'new run retained prior time/equipment');
 const coilTemperature=(captured,field,values)=>{
  const definition=captured.operatingProfiles.find(profile=>profile.componentIndex===coilFixture.component.objectIndex&&profile.loopId===coilFixture.profile.loopId),node=definition.parameterControl.nodes.find(node=>node.source.objectIndex===coilFixture.component.objectIndex&&node.source.objectType===coilFixture.component.objectType&&node.source.objectName===coilFixture.component.objectName&&node.parameters?.some(parameter=>parameter.name===field&&parameter.objectIndex===coilFixture.component.objectIndex));
  check(node,'native coil lacks distinct '+field);setReported(captured,signal=>signal.graph==='parameterControl'&&signal.nodeId===node.id&&signal.role==='sensor'&&signal.variableName==='System Node Temperature',values);return node;
 };
 const runtimeCoil=withHourly(runtimeClone(fixtures,'coil'),coilFixture);
 setReported(runtimeCoil,signal=>signal.role==='sensor'&&/temperature/i.test(signal.variableName),[22]);
 const coilAirInlet=coilTemperature(runtimeCoil,'Air Inlet Node Name',[10,10,10,10,0,0,0,0,-10,-10,-10,-10]),coilAirOutlet=coilTemperature(runtimeCoil,'Air Outlet Node Name',[20,20,20,20,0,0,0,0,-7,-7,-7,-7]);coilTemperature(runtimeCoil,'Water Inlet Node Name',[82]);
 setReported(runtimeCoil,signal=>signal.role==='setpoint',[20,20,20,20,0,0,0,0,-5,-5,-5,-5]);
 setReported(runtimeCoil,signal=>signal.role==='actuator_request',[.8,.8,.8,.8,.7,.7,.7,.7,.6,.6,.6,.6]);setReported(runtimeCoil,signal=>signal.role==='actuator',[.3,.3,.3,.3,.2,.2,.2,.2,.1,.1,.1,.1]);
 state.simulationResult={...runtimeResult,runId:'runtime-coil-cards',purposeResults:{hvacLoops:[runtimeCoil]}};simulation.renderSimulation();chooseEquipment(coilFixture);
 const commandCard=()=>host.querySelector('.hvac-parameter-flow-stage.commands [data-hvac-flow-card]'),requestCardValue=()=>commandCard()?.querySelector('[data-hvac-runtime-card-value]')?.textContent,actualCardValue=()=>commandCard()?.querySelector('[data-hvac-runtime-card-actual]')?.textContent;
 check(requestCardValue()==='Requested flow: 0.8 kg/s'&&actualCardValue()==='Actual: 0.3 kg/s','sequence conflated request/actual or joined Hourly row1 to Timestep00:15');
 const controlTerm=term=>host.querySelector('.hvac-parameter-flow-stage.controls [data-hvac-runtime-control-term="'+term+'"] b')?.textContent,controllerCard=host.querySelector('.hvac-parameter-flow-stage.controls [data-hvac-runtime-control-term="tracking-error"]')?.closest('[data-hvac-flow-card]');
 check(controllerCard?.querySelector('strong')?.textContent==='Temperature · Feedback control'&&controlTerm('target-change')==='10.00 K'&&controlTerm('actual-change')==='10.00 K'&&controlTerm('tracking-error')==='0.00 K','converged controller omitted its actual air-stream change/nonzero flow or retained a comparison-first title: '+JSON.stringify({title:controllerCard?.querySelector('strong')?.textContent,target:controlTerm('target-change'),actual:controlTerm('actual-change'),error:controlTerm('tracking-error'),label:label(),controls:host.querySelector('.hvac-parameter-flow-stage.controls')?.textContent}));
 const nativeAirInlet=runtimeCoil.controlSignals.find(signal=>signal.nodeId===coilAirInlet.id&&signal.variableName==='System Node Temperature'),nativeAirOutlet=runtimeCoil.controlSignals.find(signal=>signal.nodeId===coilAirOutlet.id&&signal.variableName==='System Node Temperature'),controllerReadings=controllerCard.querySelector('[data-hvac-runtime-card-value]');
 check(controllerReadings.title.includes(nativeAirInlet.keyValue)&&controllerReadings.title.includes(nativeAirOutlet.keyValue)&&controllerReadings.title.includes('10 °C')&&controllerReadings.title.includes('20 °C')&&!controllerReadings.textContent.includes('82'),'controller explanation lacks its exact reported air sources or borrowed water-inlet temperature');
 const originalCommandCard=commandCard();frame(1);
 check(commandCard()===originalCommandCard&&requestCardValue()==='Requested flow: 0.7 kg/s'&&actualCardValue()==='Actual: 0.2 kg/s','shared frame rebuilt graph or failed to update exact values');
 check(controllerCard===host.querySelector('.hvac-parameter-flow-stage.controls [data-hvac-runtime-control-term="tracking-error"]')?.closest('[data-hvac-flow-card]')&&controlTerm('target-change')==='0.00 K'&&controlTerm('actual-change')==='0.00 K'&&controlTerm('tracking-error')==='0.00 K'&&!controllerReadings.title.includes('10 °C'),'valid zero native temperatures retained stale controller comparison text');
 frame(2);check(controlTerm('target-change')==='5.00 K'&&controlTerm('actual-change')==='3.00 K'&&controlTerm('tracking-error')==='-2.00 K'&&requestCardValue()==='Requested flow: 0.6 kg/s'&&actualCardValue()==='Actual: 0.1 kg/s','negative native temperatures or signed tracking error failed to follow the shared frame');frame(1);
 commandCard().querySelector('button').click();check(host.querySelector('[data-hvac-runtime-history] .hvac-chart-svg')&&!host.querySelector('[data-hvac-parameter-inspector]:not([hidden])'),'sequence card opened raw parameter specs instead of history');
 const requestedBinding=runtimeCoil.controlSignals.find(signal=>signal.role==='actuator_request'),requestedTrace=host.querySelector('[data-hvac-runtime-history] [data-hvac-chart-series="'+CSS.escape(requestedBinding.id)+'"]');
 const {prepareHVACRuntimeInspection,hvacRuntimeTrace}=await import('/src/js/hvac-runtime-data.js'),traceModel=prepareHVACRuntimeInspection(runtimeCoil),nativeRequestedTrace=hvacRuntimeTrace(traceModel,traceModel.signals.find(signal=>signal.id===requestedBinding.id));
 const requestedAtTwo=nativeRequestedTrace.points.find(point=>point.label==='01-01 02:00'),requestedChart=requestedTrace?.closest('[data-hvac-inspection-chart]');
 check(nativeRequestedTrace.points.length===12&&requestedAtTwo?.value===.7&&requestedTrace?.querySelector('path.hvac-chart-line')&&!requestedTrace.querySelector('circle'),'ordinary line history dropped native Timestep values or retained point markers');
 check(requestedChart?.querySelector('[data-hvac-chart-frame-marker]')?.style.display!=='none'&&Number(requestedChart.querySelector('[data-hvac-chart-frame]')?.dataset.hvacChartFrame)===requestedAtTwo.x,'marker-free sequence history lost the shared current-time cursor');
 const requestedLegend=requestedChart.querySelector('[data-hvac-chart-legend="'+CSS.escape(requestedBinding.id)+'"]');
 const actualBinding=runtimeCoil.controlSignals.find(signal=>signal.role==='actuator');
 check(requestedLegend&&requestedChart.querySelector('[data-hvac-chart-legend="'+CSS.escape(actualBinding.id)+'"]')&&requestedChart.querySelectorAll('[data-hvac-chart-series]').length>=2,'selected command lacks separate requested/actual flow legend entries');
 const otherNodeChart=host.querySelector('[data-hvac-inspect-basic-charts] [data-hvac-inspection-chart]');
 requestedLegend.dispatchEvent(new PointerEvent('pointerover',{bubbles:true}));
 check(requestedChart.dataset.hvacChartHighlight===requestedBinding.id&&requestedTrace.classList.contains('is-highlighted')&&[...requestedChart.querySelectorAll('[data-hvac-chart-series]')].filter(series=>series!==requestedTrace).every(series=>series.classList.contains('is-muted'))&&!otherNodeChart.dataset.hvacChartHighlight,'legend hover did not isolate the matching trace or altered another chart');
 requestedLegend.dispatchEvent(new PointerEvent('pointerout',{bubbles:true,relatedTarget:requestedChart}));
 check(!requestedChart.dataset.hvacChartHighlight&&!requestedChart.querySelector('[data-hvac-chart-series].is-muted'),'legend leave did not restore the normal line graph');
 // Native focus/blur is covered by the chart browser with focus emulation;
 // dump-dom can be an unfocused window, so verify the actual app's delegation.
 requestedLegend.dispatchEvent(new FocusEvent('focusin',{bubbles:true}));check(requestedChart.dataset.hvacChartHighlight===requestedBinding.id,'legend focusin did not reach the actual app delegate');requestedLegend.dispatchEvent(new FocusEvent('focusout',{bubbles:true,relatedTarget:requestedChart}));
 check(!requestedChart.dataset.hvacChartHighlight&&!host.querySelector('[data-hvac-inspection-chart="line"] circle'),'legend blur did not restore lines or HVAC data-point markers remain');
 for(const gap of ['inlet','target']){
  const captured=structuredClone(runtimeCoil),targetNode=coilFixture.profile.parameterControl.edges.find(edge=>edge.relation==='subtracts_target').from,binding=captured.controlSignals.find(signal=>gap==='inlet'?signal.id===nativeAirInlet.id:signal.role==='setpoint'&&signal.graph==='parameterControl'&&signal.nodeId===targetNode),series=captured.controlSeries.find(series=>series.sourceId===binding.seriesId),retained=series.points.map((point,index)=>point.label==='01-01 02:00'?-1:index).filter(index=>index>=0);
  series.points=retained.map(index=>series.points[index]);series.nativeIntervals=retained.map(index=>series.nativeIntervals[index]);
  state.simulationResult={...runtimeResult,runId:'runtime-coil-missing-'+gap,purposeResults:{hvacLoops:[captured]}};simulation.renderSimulation();chooseEquipment(coilFixture);
  check(controlTerm('target-change')==='10.00 K'&&controlTerm('actual-change')==='10.00 K','missing-observation regression lacks the preceding complete controller frame');
  frame(1);check(host.querySelector('[data-hvac-runtime-error]')&&!controlTerm('target-change')&&!controlTerm('actual-change')&&controlTerm('tracking-error')===(gap==='inlet'?'0.00 K':undefined)&&requestCardValue()==='Requested flow: 0.7 kg/s','missing '+gap+' timestep retained stale thermal rows or damaged separately reported flow');
  const currentLabels=[...host.querySelectorAll('.hvac-parameter-flow-stage.controls [data-hvac-runtime-card-value]')];check(currentLabels.every(value=>!value.textContent.includes('10.00 K')&&!value.title.includes('10 °C')),'missing '+gap+' timestep retained the preceding comparison text/title');
 }
 const runtimeWaterHeater=withHourly(runtimeClone(fixtures,'waterheater'),waterHeaterFixture),tankTemperature=runtimeWaterHeater.controlSignals.find(signal=>signal.role==='sensor'&&signal.variableName==='Water Heater Tank Temperature'),tankTarget=runtimeWaterHeater.controlSignals.find(signal=>signal.role==='setpoint'&&signal.domain==='temperature_schedule'),tankRuntime=runtimeWaterHeater.controlSignals.find(signal=>signal.role==='activity'&&signal.variableName==='Water Heater Runtime Fraction');
 const {buildHVACControlExpressions}=await import('/src/js/hvac-control-expressions.js');
 const temperatureProvider=(captured,fixture,target)=>{
  const model=prepareHVACRuntimeInspection(captured),profile=selectedRuntimeProfile(model,fixture),typed=profile.signals.find(signal=>signal.id===target.id),providers=profile.signals.filter(signal=>signal.graph==='parameterControl'&&signal.role==='schedule'&&signal.domain==='temperature_schedule'&&signal.seriesId===typed.seriesId&&signal.unit===typed.unit),provider=providers.length===1?providers[0]:null;
  check(provider,'source-backed target does not have one exact parameter provider: '+fixture.family+' '+target.keyValue);
  const cards=buildHVACControlExpressions(profile.definition,{component:profile.source}).parameters.groups.flatMap(group=>group.cards),card=cards.find(card=>card.members.some(node=>node.id===provider.nodeId));
  check(card&&card.members.some(node=>node.id===provider.nodeId&&node.source.objectIndex===provider.source.objectIndex&&node.source.objectType===provider.source.objectType&&node.source.objectName===provider.source.objectName),'provider card lost its actual native schedule source');
  const button=host.querySelector('[data-hvac-inspect-parameter="'+CSS.escape(card.id)+'"]');check(button,'actual provider card is missing for '+fixture.family);return {model,profile,typed,provider,card,button};
 };
 const clickTemperatureProvider=(captured,fixture,target,useProvider=false)=>{
  const selection=temperatureProvider(captured,fixture,target),visible=useProvider?selection.provider:selection.typed,before=JSON.stringify(captured);selection.button.click();
  const history=host.querySelector('[data-hvac-runtime-history]'),toggle=history.querySelector('[data-hvac-runtime-trace="'+CSS.escape(visible.id)+'"]'),trace=history.querySelector('[data-hvac-chart-series="'+CSS.escape(visible.id)+'"]'),legend=history.querySelector('[data-hvac-chart-legend="'+CSS.escape(visible.id)+'"]');
  check(toggle?.checked&&!toggle.disabled&&toggle.closest('label').classList.contains('related')&&trace?.querySelector('path.hvac-chart-line')&&legend&&!host.querySelector('[data-hvac-parameter-inspector]:not([hidden])'),'provider click lost its visible source-backed target checkbox/path/related highlight: '+fixture.family);
  check(useProvider||!history.querySelector('[data-hvac-runtime-trace="'+CSS.escape(selection.provider.id)+'"]'),'canonical typed target provider was displayed a second time after card selection');
  const native=hvacRuntimeTrace(selection.model,visible);check(visible.series===selection.provider.series&&visible.seriesId===selection.provider.seriesId&&native.unit==='°C'&&native.points.length===visible.series.points.length&&native.points.every((point,index)=>point.value===visible.series.points[index].value&&point.label===visible.series.points[index].label),'provider selection substituted another native schedule/source/cadence');
  legend.dispatchEvent(new PointerEvent('pointerover',{bubbles:true}));check(trace.classList.contains('is-highlighted')&&trace.closest('[data-hvac-inspection-chart]').dataset.hvacChartHighlight===visible.id,'canonical provider target legend did not highlight its exact path');legend.dispatchEvent(new PointerEvent('pointerout',{bubbles:true,relatedTarget:history}));
  check(JSON.stringify(captured)===before,'provider click mutated exact captured bindings or native values');return selection;
 };
 check(tankTemperature&&tankTarget&&tankRuntime,'mixed water heater lacks required tank thermostat and native duty observations');
 setReported(runtimeWaterHeater,signal=>signal.role==='sensor'&&/temperature/i.test(signal.variableName),[99]);setReported(runtimeWaterHeater,signal=>signal.id===tankTemperature.id,[55,55,55,55,0,0,0,0,62,62,62,62]);setReported(runtimeWaterHeater,signal=>signal.domain==='temperature_schedule',[60,60,60,60,0,0,0,0,60,60,60,60]);setReported(runtimeWaterHeater,signal=>signal.role==='activity',[1000,1000,1000,1000,0,0,0,0,2000,2000,2000,2000]);setReported(runtimeWaterHeater,signal=>signal.id===tankRuntime.id,[.25,.25,.25,.25,0,0,0,0,.75,.75,.75,.75]);
 state.simulationResult={...runtimeResult,runId:'runtime-water-heater',purposeResults:{hvacLoops:[runtimeWaterHeater]}};simulation.renderSimulation();chooseEquipment(waterHeaterFixture);
 const tankPanel=onOffPanel(),tankReading=binding=>tankPanel.querySelector('[data-hvac-runtime-value="'+CSS.escape(binding.id)+'"]'),tankHistory=host.querySelector('[data-hvac-runtime-history]'),targetAliases=new Set(runtimeWaterHeater.controlSignals.filter(signal=>signal.seriesId===tankTarget.seriesId).map(signal=>signal.id)),targetToggles=[...tankHistory.querySelectorAll('[data-hvac-runtime-trace]')].filter(input=>targetAliases.has(input.dataset.hvacRuntimeTrace));
 check(!host.querySelector('[data-hvac-runtime-error]')&&tankReading(tankTemperature)?.textContent==='55 °C'&&tankReading(tankTemperature).closest('.hvac-runtime-decision-reading').querySelector('strong').textContent==='Tank temperature (interval average)'&&tankReading(tankTarget)?.textContent==='60 °C'&&tankReading(tankRuntime)?.textContent==='0.25'&&tankPanel.querySelector('[data-hvac-runtime-decision-response]').textContent==='Activity reported','native tank thermostat UI substituted a 99°C fluid-port temperature, lost the typed schedule unit or rounded fractional heater duty to an enum');
 check(targetToggles.length===1&&targetToggles[0].dataset.hvacRuntimeTrace===tankTarget.id&&tankHistory.querySelector('[data-hvac-runtime-history-group="°C"] [data-hvac-runtime-trace="'+CSS.escape(tankTarget.id)+'"]'),'typed tank target was plotted twice as a raw schedule or placed outside the temperature group');
 const mixedProvider=clickTemperatureProvider(runtimeWaterHeater,waterHeaterFixture,tankTarget);
 check(![...stack().querySelectorAll('[data-kind="schedule"] strong')].some(label=>label.textContent===tankTarget.keyValue),'tank control-temperature schedule leaked into the Boolean Availability stack');
 frameAt(runtimeWaterHeater,'01-01 02:00');check(onOffPanel()===tankPanel&&tankReading(tankTemperature).textContent==='0 °C'&&tankReading(tankTarget).textContent==='0 °C'&&tankReading(tankRuntime).textContent==='0'&&tankPanel.querySelector('[data-hvac-runtime-decision-response]').textContent==='No activity reported','zero tank temperature, setpoint or heater duty retained a preceding frame or falsely reported activity');
 check(mixedProvider.button.querySelector('[data-hvac-runtime-card-value]')?.textContent==='0 °C'&&host.querySelector('[data-hvac-runtime-trace="'+CSS.escape(tankTarget.id)+'"]')?.checked,'provider-card value/current-frame update stopped following its canonical typed target');
 check(finalRow()?.dataset.state==='allowed','0°C tank target was rendered as an OFF availability condition');
 frameAt(runtimeWaterHeater,'01-01 03:00');check(tankReading(tankTemperature).textContent==='62 °C'&&tankReading(tankTarget).textContent==='60 °C'&&tankReading(tankRuntime).textContent==='0.75'&&tankPanel.querySelector('[data-hvac-runtime-decision-response]').textContent==='Activity reported','heater duty/tank temperature did not follow the same native frame independently of the configured target');
 const chooseLocal=fixture=>{const button=host.querySelector('.hvac-inspection-local-equipment [data-hvac-inspect-native-component="'+fixture.component.objectIndex+'"]');check(button,'authored local tank/wrapper is not selectable');button.click();};
 const layeredFixture=fixtures.find(fixture=>fixture.family==='waterheater-stratified'),runtimeLayered=runtimeClone(fixtures,'waterheater-stratified'),layeredNodes=runtimeLayered.controlSignals.filter(signal=>signal.role==='sensor'&&/^Water Heater Temperature Node [24]$/.test(signal.variableName)),elementDuties=runtimeLayered.controlSignals.filter(signal=>signal.role==='activity'&&/Heater [12] Runtime Fraction/.test(signal.variableName));
 for(const signal of layeredNodes)setReported(runtimeLayered,binding=>binding.id===signal.id,[signal.variableName.endsWith('2')?58:49]);setReported(runtimeLayered,signal=>signal.domain==='temperature_schedule'&&signal.keyValue==='Runtime Upper Target',[60]);setReported(runtimeLayered,signal=>signal.domain==='temperature_schedule'&&signal.keyValue==='Runtime Lower Target',[55]);
 setReported(runtimeLayered,signal=>signal.role==='activity',[0]);for(const signal of elementDuties)setReported(runtimeLayered,binding=>binding.id===signal.id,[signal.variableName.includes('Heater 1')?.375:.25]);
 state.simulationResult={...runtimeResult,runId:'runtime-layered-water-heater',purposeResults:{hvacLoops:[runtimeLayered]}};simulation.renderSimulation();chooseLocal(layeredFixture);
 check(!host.querySelector('[data-hvac-runtime-error]')&&layeredNodes.length===2&&elementDuties.length===2&&layeredNodes.every(signal=>onOffPanel().querySelector('[data-hvac-runtime-value="'+CSS.escape(signal.id)+'"]')?.closest('.hvac-runtime-decision-reading').querySelector('strong').textContent==='Tank node '+signal.variableName.at(-1)+' (interval average)')&&elementDuties.every(signal=>onOffPanel().querySelector('[data-hvac-runtime-value="'+CSS.escape(signal.id)+'"]')?.textContent===(signal.variableName.includes('Heater 1')?'0.38':'0.25')),'stratified UI merged distinct native sensor nodes or failed to present individual continuous heater duties');
 for(const target of runtimeLayered.controlSignals.filter(signal=>signal.role==='setpoint'&&signal.domain==='temperature_schedule'))clickTemperatureProvider(runtimeLayered,layeredFixture,target);
 for(const family of ['waterheater-hpwh-pumped','waterheater-hpwh-wrapped']){
  const fixture=fixtures.find(fixture=>fixture.family===family),captured=runtimeClone(fixtures,family),backup=fixture.sources.find(source=>source.objectType==='WaterHeater:Mixed'),control=captured.controlSignals.find(signal=>signal.variableName==='Water Heater Heat Pump Control Tank Temperature'),compressor=captured.controlSignals.find(signal=>signal.role==='activity'&&signal.variableName==='Water Heater Compressor Part Load Ratio');
  setReported(captured,signal=>signal.id===control.id,[54,60,56,58]);setReported(captured,signal=>signal.domain==='temperature_schedule',[60]);setReported(captured,signal=>signal.id===compressor.id,[.5,0,.25,1]);
  const compressorSeries=captured.controlSeries.find(series=>series.sourceId===compressor.seriesId),backupDutySeries={...structuredClone(compressorSeries),sourceId:family+'-backup-duty',keyValue:backup.objectName,name:'Water Heater Runtime Fraction',column:backup.objectName+':Water Heater Runtime Fraction [](Timestep)'};backupDutySeries.points.forEach(point=>point.value=0);
  const backupDuty={...compressor,id:family+'-backup-duty-binding',source:backup,keyValue:backup.objectName,variableName:backupDutySeries.name,seriesId:backupDutySeries.sourceId};captured.controlSeries.push(backupDutySeries);captured.controlSignals.push(backupDuty);
  state.simulationResult={...runtimeResult,runId:'runtime-'+family,purposeResults:{hvacLoops:[captured]}};simulation.renderSimulation();chooseLocal(fixture);
  const panel=onOffPanel(),reading=binding=>panel.querySelector('[data-hvac-runtime-value="'+CSS.escape(binding.id)+'"]');
  check(!host.querySelector('[data-hvac-runtime-error]')&&reading(control)?.textContent==='54 °C'&&reading(control).closest('.hvac-runtime-decision-reading').querySelector('strong').textContent==='Control tank temperature (interval average)'&&reading(compressor)?.textContent==='0.5'&&reading(backupDuty)?.textContent==='0'&&panel.querySelector('[data-hvac-runtime-decision-response]').textContent==='Activity reported'&&host.querySelector('[data-hvac-runtime-activity]')?.dataset.hvacRuntimeActivity==='active','inactive backup element overrode own HPWH compressor activity/header or replaced its native control temperature: '+family);
  clickTemperatureProvider(captured,fixture,captured.controlSignals.find(signal=>signal.role==='setpoint'&&signal.domain==='temperature_schedule'));
  frame(1);check(onOffPanel()===panel&&reading(control).textContent==='60 °C'&&reading(compressor).textContent==='0'&&panel.querySelector('[data-hvac-runtime-decision-response]').textContent==='No activity reported'&&host.querySelector('[data-hvac-runtime-activity]')?.dataset.hvacRuntimeActivity==='inactive','HPWH own compressor duty/control temperature failed to follow the common native frame: '+family);
  const missing=structuredClone(captured);missing.controlSeries=missing.controlSeries.filter(series=>series.sourceId!==compressor.seriesId);setReported(missing,signal=>signal.id===backupDuty.id,[1]);state.simulationResult={...runtimeResult,runId:'runtime-'+family+'-missing-compressor',purposeResults:{hvacLoops:[missing]}};simulation.renderSimulation();chooseLocal(fixture);
  check(host.querySelector('[data-hvac-runtime-error]')?.title.includes(compressor.variableName)&&onOffPanel().querySelector('[data-hvac-runtime-decision-response]')?.textContent==='—'&&host.querySelector('[data-hvac-runtime-activity]')?.dataset.hvacRuntimeActivity==='error','missing own compressor data borrowed child activity or displayed a raw/false activity state: '+family);
 }
 for(const variant of ['unavailable-typed-target','two-visible-aliases']){
  const captured=structuredClone(runtimeWaterHeater),target=captured.controlSignals.find(signal=>signal.id===tankTarget.id);let second;
  if(variant==='unavailable-typed-target'){target.status='missing';target.message='Required typed temperature target was not collected.';}
  else{second={...target,id:'second-collected-temperature-alias',keyValue:target.keyValue.toUpperCase()};check(second.keyValue!==target.keyValue,'alias-count regression does not contain two distinct visible bindings');captured.controlSignals.push(second);}
  state.simulationResult={...runtimeResult,runId:'runtime-provider-'+variant,purposeResults:{hvacLoops:[captured]}};simulation.renderSimulation();chooseEquipment(waterHeaterFixture);
  const selected=clickTemperatureProvider(captured,waterHeaterFixture,target,true),history=host.querySelector('[data-hvac-runtime-history]'),typedToggle=history.querySelector('[data-hvac-runtime-trace="'+CSS.escape(target.id)+'"]');
  check(!typedToggle.checked&&!history.querySelector('[data-hvac-chart-series="'+CSS.escape(target.id)+'"]')&&!typedToggle.closest('label').classList.contains('related'),'provider fallback arbitrarily selected or highlighted a typed target: '+variant);
  if(second)check(!host.querySelector('[data-hvac-runtime-error]')&&!history.querySelector('[data-hvac-runtime-trace="'+CSS.escape(second.id)+'"]').checked&&!history.querySelector('[data-hvac-chart-series="'+CSS.escape(second.id)+'"]')&&selected.profile.signals.filter(signal=>signal.role==='setpoint'&&signal.domain==='temperature_schedule'&&signal.seriesId===target.seriesId&&signal.status==='collected').length===2,'ambiguous visible aliases were collapsed to an arbitrary target instead of retaining the valid provider');
  else check(typedToggle.disabled&&host.querySelector('[data-hvac-runtime-error]')?.title.includes(target.variableName),'provider fallback hid required typed-target acquisition diagnostics');
 }
 const ambiguousProvider=structuredClone(runtimeWaterHeater),ambiguousNative=ambiguousProvider.controlSeries.find(series=>series.sourceId===tankTarget.seriesId);ambiguousProvider.controlSeries.push(structuredClone(ambiguousNative));state.simulationResult={...runtimeResult,runId:'runtime-provider-ambiguous-source',purposeResults:{hvacLoops:[ambiguousProvider]}};simulation.renderSimulation();chooseEquipment(waterHeaterFixture);
 const ambiguousSelection=temperatureProvider(ambiguousProvider,waterHeaterFixture,tankTarget);ambiguousSelection.button.click();const ambiguousHistory=host.querySelector('[data-hvac-runtime-history]');
 check(ambiguousSelection.provider.status==='ambiguous'&&ambiguousSelection.typed.status==='ambiguous'&&host.querySelector('[data-hvac-runtime-error]')?.title.includes(tankTarget.variableName)&&[ambiguousSelection.provider,ambiguousSelection.typed].every(signal=>ambiguousHistory.querySelector('[data-hvac-runtime-trace="'+CSS.escape(signal.id)+'"]')?.disabled&&!ambiguousHistory.querySelector('[data-hvac-chart-series="'+CSS.escape(signal.id)+'"]')),'ambiguous native source acquired an arbitrary provider/target path or lost required acquisition diagnostics');
 const iconOnly=withHourly(runtimeClone(fixtures,'fan'),fanFixture);iconOnly.components=[];
 state.simulationResult={...runtimeResult,runId:'runtime-native-icon',purposeResults:{hvacLoops:[iconOnly]}};simulation.renderSimulation();
 const nativeIcon=host.querySelector('[data-hvac-inspect-native-component="'+fanFixture.component.objectIndex+'"]');check(nativeIcon,'equipment with no Hourly response lacks clickable native loop icon');
 nativeIcon.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true}));
 check(runtimePanel()?.dataset.hvacRuntimeProfileId===String(fanFixture.component.objectIndex)&&!host.querySelector('[data-hvac-runtime-error]'),'native equipment keyboard selection did not resolve exact occurrence controls');
 const localOnly=runtimeClone(fixtures,'local');check(!localOnly.topology,'native local fixture must not have a physical loop');
 state.simulationResult={...runtimeResult,runId:'runtime-local-only',purposeResults:{hvacLoops:[localOnly]}};simulation.renderSimulation();
 check(slider()?.max==='3'&&!slider().disabled&&label().includes('00:15')&&!host.querySelector('[data-hvac-inspect-topology]'),'control-only local result lost native common frames or fabricated a loop: '+JSON.stringify({max:slider()?.max,disabled:slider()?.disabled,label:label(),topology:host.querySelector('[data-hvac-inspect-topology]')?.outerHTML.slice(0,100),text:host.innerText.slice(0,200)}));
 const localButton=host.querySelector('.hvac-inspection-local-equipment [data-hvac-inspect-native-component="'+localFixture.component.objectIndex+'"]');check(localButton,'local equipment controls became inaccessible without a profile picker');localButton.click();
 check(runtimePanel()?.dataset.hvacRuntimeProfileId===String(localFixture.component.objectIndex)&&!host.querySelector('[data-hvac-runtime-error]'),'local equipment click lost exact source controls');
 frame(2);check(label().includes('00:45')&&!host.querySelector('[data-hvac-runtime-error]'),'local equipment shared frame did not follow collected native Timestep');
 step(1);check(label().includes('01:00'),'local next button did not move one native quarter-hour frame');ctrlFrame('ArrowLeft');check(label().includes('00:45'),'local Ctrl+Left used an Hourly step instead of the prior native Timestep');
 const overriddenFan=withHourly(runtimeClone(fixtures,'fan'),fanFixture);
 setReported(overriddenFan,signal=>signal.role==='schedule'&&signal.keyValue.toLowerCase()===ownSchedule.toLowerCase(),[0]);
 setReported(overriddenFan,signal=>signal.role==='manager_status'||signal.role==='loop_status',[2]);setReported(overriddenFan,signal=>signal.role==='activity',[1000]);
 state.simulationResult={...runtimeResult,runId:'runtime-cycleon-override',purposeResults:{hvacLoops:[overriddenFan]}};simulation.renderSimulation();chooseEquipment(fanFixture);
 const offSchedule=[...stack().querySelectorAll('[data-hvac-runtime-availability-row][data-kind="schedule"]')].find(row=>row.querySelector('strong')?.textContent===ownSchedule);
 const overrideBadge=host.querySelector('[data-hvac-runtime-override]');
 check(offSchedule?.dataset.state==='blocked'&&offSchedule.dataset.decisive==='false'&&finalRow()?.dataset.state==='allowed'&&overrideBadge?.getClientRects().length&&overrideBadge.textContent.includes('CycleOn'),'CycleOn override was hidden, changed the original OFF schedule or failed to explain final permission');
 const noOverrideFan=structuredClone(overriddenFan);setReported(noOverrideFan,signal=>signal.role==='manager_status'||signal.role==='loop_status',[0]);setReported(noOverrideFan,signal=>signal.role==='activity',[0]);
 state.simulationResult={...runtimeResult,runId:'runtime-noaction-schedule-off',purposeResults:{hvacLoops:[noOverrideFan]}};simulation.renderSimulation();chooseEquipment(fanFixture);
 check(finalRow()?.dataset.state==='blocked'&&!host.querySelector('[data-hvac-runtime-override]')?.getClientRects().length,'NoAction retained a CycleOn override or ignored the original OFF schedule');
 const missingRuntime=withHourly(runtimeClone(fixtures,'fan'),fanFixture),required=missingRuntime.controlSignals.find(signal=>signal.required&&signal.role==='manager_status');
 check(required,'fan lacks manager for acquisition regression');missingRuntime.controlSeries=missingRuntime.controlSeries.filter(series=>series.sourceId!==required.seriesId);
 state.simulationResult={...runtimeResult,runId:'runtime-missing',purposeResults:{hvacLoops:[missingRuntime]}};simulation.renderSimulation();chooseEquipment(fanFixture);
 check(host.querySelector('[data-hvac-runtime-error]')?.title.includes(required.variableName)&&host.querySelector('[data-hvac-inspect-topology]')&&slider()&&!slider().disabled,'diagnostics failure removed loop or became ordinary unknown');
 for(const corruption of ['missing','misaligned']){
  const invalidTiming=withHourly(runtimeClone(fixtures,'fan'),fanFixture),binding=invalidTiming.controlSignals.find(signal=>signal.required&&signal.role==='manager_status'),statusIDs=new Set(invalidTiming.controlSignals.filter(signal=>signal.role==='manager_status'||signal.role==='loop_status').map(signal=>signal.seriesId));
  check(binding.reportingFrequency==='Detailed','metadata integrity fixture lacks native Detailed statuses');for(const series of invalidTiming.controlSeries)if(statusIDs.has(series.sourceId)){if(corruption==='missing')delete series.nativeIntervals;else series.nativeIntervals.pop();}
  state.simulationResult={...runtimeResult,runId:'runtime-native-timing-'+corruption,purposeResults:{hvacLoops:[invalidTiming]}};simulation.renderSimulation();chooseEquipment(fanFixture);
  check(host.querySelector('[data-hvac-runtime-error]')?.title.includes(binding.variableName)&&finalRow()?.dataset.state==='error'&&!stack().querySelector('[data-state="mixed"],.is-mixed')&&host.querySelector('[data-hvac-inspect-topology]')&&!slider().disabled,'invalid Detailed '+corruption+' native interval metadata became permission or an ambiguous display: '+JSON.stringify({error:host.querySelector('[data-hvac-runtime-error]')?.title,permission:host.querySelector('[data-hvac-runtime-permission]')?.dataset.hvacRuntimePermission,final:finalRow()?.dataset.state,states:[...stack().querySelectorAll('[data-hvac-runtime-availability-row]')].map(row=>[row.querySelector('strong')?.textContent,row.dataset.state])}));
 }
 const unmatched=withHourly(runtimeClone(fixtures,'coil'),coilFixture);
 // Same-file Hourly row2 and nearby control rows do not establish a2h match.
 for(const series of unmatched.controlSeries){const offsets=series.points.map((point,index)=>point.label!=='01-01 02:00'?index:-1).filter(index=>index>=0);series.points=offsets.map(index=>series.points[index]);series.nativeIntervals=offsets.map(index=>series.nativeIntervals[index]);}
 state.simulationResult={...runtimeResult,runId:'runtime-no-common-frame',purposeResults:{hvacLoops:[unmatched]}};simulation.renderSimulation();chooseEquipment(coilFixture);frame(1);
 check(host.querySelector('[data-hvac-runtime-error]')&&!host.querySelector('[data-hvac-runtime-card-value]')?.textContent.includes('0.8'),'unmatched Hourly frame borrowed nearby runtime sample');
 check(label()==='01-01 02:00'&&host.querySelector('[data-hvac-inspect-metric="temperature"]')?.textContent.includes('22'),'missing control timestep destroyed actual Hourly node snapshot');
 const invalidRuntime=withHourly(runtimeClone(fixtures,'chiller'),chillerFixture),invalidProfile=invalidRuntime.operatingProfiles.find(profile=>profile.componentIndex===chillerFixture.component.objectIndex),invalidLayer=invalidProfile.availabilityLayers.find(layer=>layer.role==='plant_dispatch');
 for(const layer of [invalidLayer,...invalidProfile.configuredAvailabilityLayers.filter(layer=>layer.id===invalidLayer.id)])layer.expression={op:'unknown',reason:'missing_operation_schemes',source:layer.owner};
 state.simulationResult={...runtimeResult,runId:'runtime-definition-error',purposeResults:{hvacLoops:[invalidRuntime]}};simulation.renderSimulation();chooseEquipment(chillerFixture);
 check(host.querySelector('[data-hvac-runtime-error]')?.title.includes('missing_operation_schemes'),'invalid native definition became successful runtime condition');
 state.simulationResult={...result,runId:'legacy-unified'};simulation.renderSimulation();
 check(!runtimePanel()&&slider()&&!slider().disabled&&host.querySelector('[data-hvac-inspect-basic-charts]')&&!host.querySelector('[data-hvac-inspect-view]'),'legacy result lost node inspection or fabricated runtime controls');
 check(JSON.stringify(fixtures)===fixturesBefore,'unified UI mutated native definitions/observations');
 const review=new URLSearchParams(location.search);
 if(review.has('review')){
  const family=review.get('review')==='error'?'fan':review.get('review'),fixture=fixtures.find(item=>item.family===family),captured=withHourly(runtimeClone(fixtures,family),fixture);
  if(review.get('review')==='error'){const source=captured.controlSignals.find(signal=>signal.required&&signal.role==='manager_status');captured.controlSeries=captured.controlSeries.filter(series=>series.sourceId!==source.seriesId);}
  if(review.get('override')==='1'){setReported(captured,signal=>signal.role==='schedule'&&signal.keyValue.toLowerCase()===ownSchedule.toLowerCase(),[0]);setReported(captured,signal=>signal.role==='manager_status'||signal.role==='loop_status',[2]);setReported(captured,signal=>signal.role==='activity',[1000]);}
  if(review.get('transitions')==='1')nativeStatusTransitions(captured);
  if(family==='coil'){setReported(captured,signal=>signal.role==='sensor'&&/temperature/i.test(signal.variableName),[20]);coilTemperature(captured,'Air Inlet Node Name',[10]);coilTemperature(captured,'Air Outlet Node Name',[20]);coilTemperature(captured,'Water Inlet Node Name',[82]);setReported(captured,signal=>signal.role==='setpoint',[20]);setReported(captured,signal=>signal.role==='actuator_request',[.8]);setReported(captured,signal=>signal.role==='actuator',[.3]);}
  if(family==='chiller'){setReported(captured,signal=>signal.role==='demand',[20000]);setReported(captured,signal=>signal.role==='dispatch',[-6000]);setReported(captured,signal=>signal.role==='activity',[0]);}
  if(family==='waterheater'){setReported(captured,signal=>signal.role==='sensor'&&/temperature/i.test(signal.variableName),[99]);setReported(captured,signal=>signal.variableName==='Water Heater Tank Temperature',[58.5]);setReported(captured,signal=>signal.domain==='temperature_schedule',[60]);setReported(captured,signal=>signal.role==='activity',[1000]);setReported(captured,signal=>signal.variableName==='Water Heater Runtime Fraction',[.025]);}
  const {setLanguage}=await import('/src/js/i18n.js');setLanguage(review.get('language')||'en');document.documentElement.dataset.theme=review.get('theme')||'dark';
  state.simulationResult={...runtimeResult,runId:'runtime-review-'+family,purposeResults:{hvacLoops:[captured]}};simulation.renderSimulation();chooseEquipment(fixture);if(review.get('review')!=='error')frame(1);
  if(review.get('focus')==='control')host.querySelector('[data-hvac-runtime-control]')?.scrollIntoView({block:'start'});
  if(review.get('focus')==='availability')host.querySelector('[data-hvac-runtime-availability]')?.scrollIntoView({block:'start'});
  if(review.get('focus')==='onoff')host.querySelector('[data-hvac-runtime-onoff]')?.scrollIntoView({block:'start'});
  if(review.get('focus')==='error')host.querySelector('[data-hvac-runtime-error]')?.scrollIntoView({block:'start'});
  if(review.get('focus')==='history')host.querySelector('[data-hvac-runtime-history]')?.scrollIntoView({block:'start'});
  if(review.get('legend')==='1')host.querySelector('[data-hvac-runtime-history] [data-hvac-chart-legend]')?.dispatchEvent(new PointerEvent('pointerover',{bubbles:true}));
 }
 document.body.dataset.hvacFrameStatus='passed';document.getElementById('hvac-frame-result').textContent='passed';
}catch(error){document.body.dataset.hvacFrameStatus='failed';document.getElementById('hvac-frame-result').textContent=error.stack;}
</script>`
