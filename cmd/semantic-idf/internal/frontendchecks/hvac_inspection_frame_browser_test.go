package frontendchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

type hvacRuntimeBrowserFixture struct {
	Family    string                   `json:"family"`
	Component idf.HVACOperatingSource  `json:"component"`
	Profile   idf.HVACOperatingProfile `json:"profile"`
	Topology  idf.HVACLoop             `json:"topology"`
	Runtime   map[string]any           `json:"runtime"`
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
		case "demand", "activity":
			value, unit = 1000, "W"
		}
		lower := strings.ToLower(name)
		if strings.Contains(lower, "ratio") || strings.Contains(lower, "fraction") {
			value, unit = 1, ""
		}
		if strings.Contains(lower, "temperature") {
			value, unit = 20, "C"
		}
		if strings.Contains(lower, "mass flow rate") {
			value, unit = 1, "kg/s"
		}
		if strings.Contains(lower, "thermostat control type") {
			value, unit = 4, ""
		}
		points := []simulation.SimulationPoint{}
		for index, label := range []string{"01-01 00:15", "01-01 00:30", "01-01 00:45", "01-01 01:00"} {
			points = append(points, simulation.SimulationPoint{X: index + 1, Label: label, Value: value})
		}
		series = append(series, simulation.SimulationSeries{SourceID: fmt.Sprintf("runtime-%s-%d", fixture.Family, len(series)), KeyValue: key, Name: name, File: "eplusout.sql", Column: key + ":" + name + " [" + unit + "](" + frequency + ")", ReportingFrequency: frequency, Points: points})
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
	} {
		found := false
		for _, profile := range report.OperatingProfiles {
			loop := loops[profile.LoopID]
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
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/src/frame.html").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-hvac-frame-status="passed"`) {
		if _, detail, ok := strings.Cut(string(output), `<pre id="hvac-frame-result" hidden="">`); ok {
			message, _, _ := strings.Cut(detail, "</pre>")
			t.Fatalf("HVAC shared frame: %v\n%s", err, message)
		}
		t.Fatalf("HVAC shared frame: %v\n%s", err, output)
	}
	if directory := os.Getenv("HVAC_RUNTIME_CAPTURE_DIR"); directory != "" {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
		for _, capture := range []struct{ name, query, viewport string }{
			{"fan-logic-dark.png", "?review=fan", "1900,1300"},
			{"coil-control-dark.png", "?review=coil&focus=control", "2560,1500"},
			{"chiller-logic-ko-light.png", "?review=chiller&language=ko&theme=light", "1900,1400"},
			{"missing-required.png", "?review=error", "1600,1200"},
			{"manager-source-dark.png", "?review=fan&source=manager&focus=source", "2560,1700"},
		} {
			path, err := filepath.Abs(filepath.Join(directory, capture.name))
			if err != nil {
				t.Fatal(err)
			}
			captureContext, stop := context.WithTimeout(context.Background(), 30*time.Second)
			output, err := exec.CommandContext(captureContext, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--force-device-scale-factor=1", "--window-size="+capture.viewport, "--user-data-dir="+t.TempDir(), "--screenshot="+path, server.URL+"/src/frame.html"+capture.query).CombinedOutput()
			stop()
			if err != nil {
				t.Fatalf("runtime HVAC capture %s failed: %v\n%s", capture.name, err, output)
			}
			t.Logf("runtime HVAC visual review: %s", path)
		}
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
 check(!document.querySelector('#simulationHVACLoopStats')&&!host.closest('.simulation-section').querySelector('.profile-section-head'),'removed loop heading/count still visible');
 frame(2);check(label()==='01-01 03:00','frame slider did not change selected hour');
 select('Second');check(slider().value==='1'&&label()==='01-01 03:00','loop switch reset timestamp or used old array index');
 select('Empty');select('First');check(slider().value==='2'&&label()==='01-01 03:00','empty loop discarded shared frame');
 select('Second');frame(0);select('First');check(slider().value==='0'&&label()==='01-01 01:00','frame selected in second loop was not shared');
 frame(2);state.simulationResult={...result,runId:'frames-2'};simulation.renderSimulation();check(slider().value==='0','new simulation retained previous run timestamp');

 // Exercise the new operating-logic view through the real app event path.
 // Hourly loop values and Timestep diagnostic rows retain separate frames.
 const fixtures=await(await fetch('/runtime-fixtures')).json(),fixturesBefore=JSON.stringify(fixtures),fanFixture=fixtures.find(fixture=>fixture.family==='fan'),coilFixture=fixtures.find(fixture=>fixture.family==='coil'),chillerFixture=fixtures.find(fixture=>fixture.family==='chiller');
 const addHourly=(captured,nodeName)=>{captured.series=[{name:'System Node Temperature',keyValue:nodeName,file:'eplusout.sql',column:nodeName+':System Node Temperature [C](Hourly)',reportingFrequency:'Hourly',points:[1,2,3].map(x=>({x,value:20+x,label:'01-01 0'+x+':00'}))}];return captured;};
 const runtimeFan=addHourly(runtimeClone(fixtures,'fan'),fanFixture.topology.supplySide?.inletNode||'Native fan inlet');
 const runtimeChiller=addHourly(runtimeClone(fixtures,'chiller'),chillerFixture.topology.supplySide?.inletNode||'Native chiller inlet');
 const dispatchDemand=runtimeChiller.controlSignals.find(signal=>signal.graph==='loop'&&signal.role==='demand');check(dispatchDemand,'native chiller lacks reported loop demand for dispatch context');setReported(runtimeChiller,signal=>signal.id===dispatchDemand.id,[1000,2000,3000,4000]);
 const managerBinding=runtimeFan.controlSignals.find(signal=>signal.role==='manager_status'),managerContext=runtimeFan.controlSignals.find(signal=>signal.role==='sensor'&&signal.source?.objectIndex===managerBinding?.source?.objectIndex&&/temperature/i.test(signal.variableName));
 check(managerBinding&&managerContext,'native manager reason-context inputs were not collected');setReported(runtimeFan,signal=>signal.id===managerContext.id,[18,19,20,21]);
 const runtimeResult={...result,runId:'runtime-1',purposeResults:{hvacLoops:[runtimeFan,runtimeChiller]}};
 // A later editor report must never replace this saved run's definitions.
 state.report={hvac:{operatingProfiles:[{componentIndex:fanFixture.component.objectIndex,loopId:fanFixture.profile.loopId,parameterControl:{nodes:[{id:'editor-only',source:{objectName:'EDITOR MODEL MUST NOT APPEAR'}}],edges:[]}}]}};
 state.simulationResult=runtimeResult;simulation.renderSimulation();
 const view=mode=>{const control=host.querySelector('[data-hvac-inspect-view]');check(control,'inspection view selector missing');if(control.value===mode)return;control.value=mode;control.dispatchEvent(new Event('change',{bubbles:true}));};
 const runtimeSlider=()=>host.querySelector('[data-hvac-runtime-frame]'),runtimeLabel=()=>host.querySelector('[data-hvac-runtime-frame-label]')?.textContent;
 const runtimeFrame=index=>{const control=runtimeSlider();check(control&&!control.disabled,'runtime frame control is not usable');control.value=index;control.dispatchEvent(new Event('input',{bubbles:true}));control.dispatchEvent(new Event('change',{bubbles:true}));};
 const chooseProfile=fixture=>{const control=host.querySelector('[data-hvac-runtime-profile]');check(control&&[...control.options].some(option=>option.value===String(fixture.component.objectIndex)),'executed component profile missing from runtime picker');if(control.value===String(fixture.component.objectIndex))return;control.value=String(fixture.component.objectIndex);control.dispatchEvent(new Event('change',{bubbles:true}));};
 frame(2);view('logic');chooseProfile(fanFixture);
 check(host.querySelector('[data-hvac-runtime]')&&runtimeSlider().max==='3'&&runtimeLabel().includes('00:15'),'logic view did not keep its native four-frame Timestep calendar');
 check(!host.textContent.includes('EDITOR MODEL MUST NOT APPEAR')&&host.textContent.includes(fanFixture.component.objectName),'runtime component selection used a changed editor model');
 check(!host.querySelector('[data-hvac-runtime-error]')&&host.querySelector('[data-hvac-runtime-summary]')&&host.querySelector('[data-hvac-runtime-availability]')&&host.querySelector('[data-hvac-runtime-control]'),'collected runtime evidence did not expose availability/control summary');
 const fanControlCard=()=>[...host.querySelectorAll('.hvac-parameter-flow-stage.controls [data-hvac-flow-card]')].find(card=>card.querySelector('strong')?.textContent==='Match airflow');
 check(fanControlCard()&&!fanControlCard().querySelector('[data-hvac-runtime-card-value]'),'fan airflow controller borrowed an unrelated temperature sensor from the same equipment source');
 check(host.querySelector('[data-hvac-runtime-decisions] [data-hvac-runtime-operator="'+CSS.escape(fanFixture.profile.finalAvailability.op)+'"]'),'native run-permission AND/OR logic was omitted');
 const managerButton=host.querySelector('[data-hvac-runtime-source="'+CSS.escape(managerBinding.id)+'"]');check(managerButton,'availability owner is not inspectable');managerButton.click();
 const sourceDetail=()=>host.querySelector('[data-hvac-runtime-source-detail]'),sourceInput=(key,name)=>[...sourceDetail().querySelectorAll('dl > div')].find(row=>row.querySelector('dt')?.textContent.trim()===key+' · '+name)?.querySelector('dd')?.textContent;
 check(sourceDetail()?.querySelector('h4')?.textContent===managerBinding.source.objectName&&sourceDetail().textContent.includes(managerBinding.source.objectType),'manager source detail lost the executed object identity');
 const managerDefinition=fanFixture.profile.availabilityDefinitions.nodes.find(node=>node.code==='availability_manager'&&node.source.objectIndex===managerBinding.source.objectIndex);
 const originalManagerFields=(managerDefinition?.parameters||[]).filter(field=>field.fieldIndex>=0&&field.objectIndex===managerBinding.source.objectIndex);
 check(originalManagerFields.length&&originalManagerFields.every(field=>[...sourceDetail().querySelectorAll('dl > div')].some(row=>row.querySelector('dt')?.textContent.trim()===field.name&&row.querySelector('dd')?.textContent.trim()===field.value)),'original manager fields were replaced by an inferred reason');
 const nativeSourceFields=[fanFixture.profile.availabilityDefinitions,fanFixture.profile.onOff,fanFixture.profile.parameterControl].flatMap(graph=>graph?.nodes||[]).filter(node=>node.source.objectIndex===managerBinding.source.objectIndex).flatMap(node=>node.parameters||[]).filter(field=>field.fieldIndex>=0&&field.objectIndex===managerBinding.source.objectIndex);
 check([...sourceDetail().querySelector('dl').children].every(row=>nativeSourceFields.some(field=>row.querySelector('dt')?.textContent.trim()===field.name&&row.querySelector('dd')?.textContent.trim()===field.value)),'internal analysis metadata was presented as an original IDF field');
 check(sourceInput(managerContext.keyValue,managerContext.variableName)==='18 °C','manager detail did not show current collected reason-context values');
 runtimeFrame(1);check(sourceInput(managerContext.keyValue,managerContext.variableName)==='19 °C'&&host.querySelector('[data-hvac-runtime-source="'+CSS.escape(managerBinding.id)+'"]')?.getAttribute('aria-pressed')==='true','selected source context did not follow the shared logic frame');
 check(!fanControlCard().querySelector('[data-hvac-runtime-card-value]'),'runtime frame update attached an unrelated equipment temperature to the airflow controller');
 runtimeFrame(2);check(runtimeLabel().includes('00:45'),'runtime frame input/change did not select the exact Timestep');
 view('loop');check(slider().value==='2'&&label()==='01-01 03:00','runtime frame overwrote the Hourly loop snapshot');
 view('logic');check(runtimeSlider().value==='2'&&runtimeLabel().includes('00:45'),'view switch discarded runtime diagnostic position');
 select(runtimeChiller.name);view('logic');chooseProfile(chillerFixture);runtimeFrame(1);
 check(runtimeLabel().includes('00:30')&&host.textContent.includes(chillerFixture.component.objectName)&&!host.querySelector('[data-hvac-runtime-error]'),'another circuit borrowed the fan profile/frame');
 const dispatchContext=()=>host.querySelector('[data-hvac-runtime-dispatch]'),dispatchValue=()=>[...dispatchContext().querySelectorAll('dl > div')].find(row=>row.querySelector('dt')?.textContent.trim()===dispatchDemand.keyValue+' · '+dispatchDemand.variableName)?.querySelector('dd')?.textContent;
 const dispatchDefinitions=[chillerFixture.profile.onOff,chillerFixture.profile.parameterControl].flatMap(graph=>graph.nodes||[]).filter(node=>node.code==='plant_dispatch'),dispatchFields=dispatchDefinitions.flatMap(node=>node.parameters||[]).filter(field=>field.fieldIndex>=0);
 check(dispatchDefinitions.length&&dispatchFields.some(field=>/Lower Limit/.test(field.name))&&dispatchFields.some(field=>/Equipment List Name/.test(field.name)),'native chiller no longer covers source-backed range/list dispatch');
 check(dispatchContext()&&dispatchDefinitions.every(node=>dispatchContext().textContent.includes(node.source.objectName)&&dispatchContext().textContent.includes(node.source.objectType))&&dispatchFields.every(field=>[...dispatchContext().querySelectorAll('dl > div')].some(row=>row.querySelector('dt')?.textContent.trim()===field.name&&row.querySelector('dd')?.textContent.trim()===field.value)),'plant dispatch omitted original operation source/ranges/equipment list');
 const chillerDecisions=host.querySelector('[data-hvac-runtime-decisions]');
 check(chillerDecisions&&!chillerDecisions.querySelector('[data-hvac-runtime-condition="unknown"]')&&!chillerDecisions.textContent.includes('Runtime decision input')&&chillerDecisions.querySelector('[data-hvac-runtime-state="allowed"]'),'source-less runtime dispatch remained an unresolved Availability condition');
 check(dispatchValue()==='2.00 kW','dispatch did not show native reported demand on the selected Timestep: '+JSON.stringify({value:dispatchValue(),key:dispatchDemand.keyValue,variable:dispatchDemand.variableName}));runtimeFrame(2);check(dispatchValue()==='3.00 kW','dispatch context did not update with the shared control frame');runtimeFrame(1);
 select(runtimeFan.name);view('logic');chooseProfile(fanFixture);
 check(runtimeSlider().value==='2'&&runtimeLabel().includes('00:45'),'switching loops discarded the selected occurrence runtime frame');
 state.simulationResult={...runtimeResult,runId:'runtime-2'};simulation.renderSimulation();view('logic');chooseProfile(fanFixture);
 check(runtimeSlider().value==='0'&&runtimeLabel().includes('00:15'),'new simulation retained the prior run runtime timestamp');

 const runtimeCoil=addHourly(runtimeClone(fixtures,'coil'),coilFixture.topology.supplySide?.inletNode||'Native coil inlet');
 setReported(runtimeCoil,signal=>signal.role==='sensor'&&/temperature/i.test(signal.variableName),[22,23,24,25]);setReported(runtimeCoil,signal=>signal.role==='setpoint',[20]);
 setReported(runtimeCoil,signal=>signal.role==='actuator_request',[.8,.7,.6,.5]);setReported(runtimeCoil,signal=>signal.role==='actuator',[.3,.2,.1,0]);
 state.simulationResult={...runtimeResult,runId:'runtime-coil-cards',purposeResults:{hvacLoops:[runtimeCoil]}};simulation.renderSimulation();view('logic');chooseProfile(coilFixture);
 const commandCard=()=>host.querySelector('.hvac-parameter-flow-stage.commands [data-hvac-flow-card]'),requestCardValue=()=>commandCard()?.querySelector('[data-hvac-runtime-card-value]')?.textContent,actualCardValue=()=>commandCard()?.querySelector('[data-hvac-runtime-card-actual]')?.textContent;
 check(requestCardValue()==='Requested: 0.8 kg/s'&&actualCardValue()==='Actual: 0.3 kg/s','command card conflated requested and actual actuator values');
 const originalCommandCard=commandCard();runtimeFrame(1);
 check(commandCard()===originalCommandCard&&requestCardValue()==='Requested: 0.7 kg/s'&&actualCardValue()==='Actual: 0.2 kg/s','runtime frame rebuilt the control graph or failed to update distinct actuator values');

 const missingRuntime=structuredClone(runtimeFan),required=missingRuntime.controlSignals.find(signal=>signal.required&&signal.role==='manager_status');
 check(required,'native fan fixture needs a reported manager for acquisition regression');missingRuntime.controlSeries=missingRuntime.controlSeries.filter(series=>series.sourceId!==required.seriesId);
 state.simulationResult={...runtimeResult,runId:'runtime-missing',purposeResults:{hvacLoops:[missingRuntime]}};simulation.renderSimulation();view('logic');chooseProfile(fanFixture);
 const acquisition=host.querySelector('[data-hvac-runtime-error]');
 check(acquisition&&acquisition.textContent.trim()&&host.textContent.includes(required.variableName),'missing required control output became an ordinary successful unknown row');
 view('loop');check(slider()&&!slider().disabled&&label()==='01-01 01:00','control acquisition failure destroyed retained Hourly loop values');
 const invalidRuntime=structuredClone(runtimeChiller),invalidProfile=invalidRuntime.operatingProfiles.find(profile=>profile.componentIndex===chillerFixture.component.objectIndex),invalidLayer=invalidProfile.availabilityLayers.find(layer=>layer.role==='plant_dispatch');
 for(const layer of [invalidLayer,...invalidProfile.configuredAvailabilityLayers.filter(layer=>layer.id===invalidLayer.id)])layer.expression={op:'unknown',reason:'missing_operation_schemes',source:layer.owner};
 state.simulationResult={...runtimeResult,runId:'runtime-definition-error',purposeResults:{hvacLoops:[invalidRuntime]}};simulation.renderSimulation();view('logic');chooseProfile(chillerFixture);
 const definitionError=host.querySelector('[data-hvac-runtime-error]');check(definitionError?.querySelector('h4')?.textContent==='Control definition error'&&definitionError.textContent.includes('missing_operation_schemes')&&!host.querySelector('[data-hvac-runtime-decisions]'),'invalid source definition was displayed as a successful unknown runtime condition');
 state.simulationResult={...result,runId:'legacy-logic'};simulation.renderSimulation();view('logic');
 check(host.querySelector('[data-hvac-runtime-error]')&&!runtimeSlider(),'old result silently synthesized operating logic from the editor');
 view('loop');check(slider()&&!slider().disabled,'legacy result lost its original loop inspection');
 check(JSON.stringify(fixtures)===fixturesBefore,'runtime UI mutated original executed fixtures');
 const review=new URLSearchParams(location.search);
 if(review.has('review')){
  const family=review.get('review')==='error'?'fan':review.get('review'),fixture=fixtures.find(item=>item.family===family),captured=addHourly(runtimeClone(fixtures,family),fixture.topology.supplySide?.inletNode||'Native inlet');
  if(review.get('review')==='error'){
   const source=captured.controlSignals.find(signal=>signal.required&&signal.role==='manager_status');captured.controlSeries=captured.controlSeries.filter(series=>series.sourceId!==source.seriesId);
  }
  if(family==='coil'){
   setReported(captured,signal=>signal.role==='sensor'&&/temperature/i.test(signal.variableName),[22]);setReported(captured,signal=>signal.role==='setpoint',[20]);setReported(captured,signal=>signal.role==='actuator_request',[.8]);setReported(captured,signal=>signal.role==='actuator',[.3]);
  }
  if(review.get('source')==='manager'){
   const manager=captured.controlSignals.find(signal=>signal.role==='manager_status'),context=captured.controlSignals.find(signal=>signal.role==='sensor'&&signal.source?.objectIndex===manager.source.objectIndex&&/temperature/i.test(signal.variableName));
   setReported(captured,signal=>signal.id===context.id,[18,19,20,21]);
  }
  const {setLanguage}=await import('/src/js/i18n.js');setLanguage(review.get('language')||'en');document.documentElement.dataset.theme=review.get('theme')||'dark';
  state.simulationResult={...runtimeResult,runId:'runtime-review-'+family,purposeResults:{hvacLoops:[captured]}};simulation.renderSimulation();view('logic');chooseProfile(fixture);
  if(review.get('review')!=='error')runtimeFrame(1);
  if(review.get('source')==='manager'){
   const manager=captured.controlSignals.find(signal=>signal.role==='manager_status');host.querySelector('[data-hvac-runtime-source="'+CSS.escape(manager.id)+'"]').click();
  }
  if(review.get('focus')==='control')host.querySelector('[data-hvac-runtime-control]')?.scrollIntoView({block:'start'});
  if(review.get('focus')==='source')host.querySelector('[data-hvac-runtime-source-detail]')?.scrollIntoView({block:'start'});
 }
 document.body.dataset.hvacFrameStatus='passed';document.getElementById('hvac-frame-result').textContent='passed';
}catch(error){document.body.dataset.hvacFrameStatus='failed';document.getElementById('hvac-frame-result').textContent=error.stack;}
</script>`
