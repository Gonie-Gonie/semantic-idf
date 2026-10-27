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
			{"fan-loop-dark.png", "?review=fan", "1900,1300"},
			{"coil-control-dark.png", "?review=coil&focus=control", "2560,1500"},
			{"chiller-availability-ko-light.png", "?review=chiller&focus=availability&language=ko&theme=light", "1900,1400"},
			{"missing-required.png", "?review=error&focus=error", "1600,1200"},
			{"fan-availability-narrow.png", "?review=fan&focus=availability", "1360,1200"},
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

 // The loop's Hourly cursor is shared; history retains native Timestep data.
 const fixtures=await(await fetch('/runtime-fixtures')).json(),fixturesBefore=JSON.stringify(fixtures),fanFixture=fixtures.find(fixture=>fixture.family==='fan'),coilFixture=fixtures.find(fixture=>fixture.family==='coil'),chillerFixture=fixtures.find(fixture=>fixture.family==='chiller'),localFixture=fixtures.find(fixture=>fixture.family==='local');
 const nativeMember=fixture=>[fixture.topology.supplySide,fixture.topology.demandSide].flatMap(side=>side?.branches||[]).flatMap(branch=>branch.components||[]).find(item=>item.objectType===fixture.component.objectType&&item.objectName===fixture.component.objectName);
 const hourlySeries=(fixture,key,name,unit,values)=>({name,keyValue:key,file:'eplusout.sql',column:key+':'+name+' ['+unit+'](Hourly)',reportingFrequency:'Hourly',points:[1,2,3].map((hour,index)=>({x:hour,label:'01-01 0'+hour+':00',value:values[index%values.length]}))});
 const withHourly=(captured,fixture)=>{
  for(const series of captured.controlSeries||[]){const observed=series.points;delete series.displayPoints;delete series.displayColumn;delete series.displayUnit;series.points=Array.from({length:12},(_,index)=>{const minutes=(index+1)*15;return {x:index+1,label:'01-01 '+String(Math.floor(minutes/60)).padStart(2,'0')+':'+String(minutes%60).padStart(2,'0'),value:observed[index%observed.length].value};});}
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
 const managerBinding=runtimeFan.controlSignals.find(signal=>signal.componentIndex===fanFixture.component.objectIndex&&signal.role==='manager_status');check(managerBinding,'native fan lacks its manager');
 const fanGate=expressionNodes(fanFixture.profile.finalAvailability).find(expression=>expression.op==='fan_gate'),ownSchedule=expressionNodes(fanGate?.children?.[1]).find(expression=>expression.op==='schedule_positive')?.scheduleName;check(ownSchedule,'native fan lacks its own schedule');
 setReported(runtimeFan,signal=>signal.role==='schedule'&&signal.keyValue.toLowerCase()===ownSchedule.toLowerCase(),[1]);
 setReported(runtimeFan,signal=>signal.role==='manager_status',[0,0,0,0,1,1,1,1,2,2,2,2]);
 setReported(runtimeFan,signal=>signal.role==='activity',[1000,1000,1000,1000,0,0,0,0,1500,1500,1500,1500]);
 const runtimeResult={...result,runId:'runtime-1',purposeResults:{hvacLoops:[runtimeFan,runtimeChiller]}};
 state.report={hvac:{operatingProfiles:[{componentIndex:fanFixture.component.objectIndex,loopId:fanFixture.profile.loopId,parameterControl:{nodes:[{id:'editor-only',source:{objectName:'EDITOR MODEL MUST NOT APPEAR'}}],edges:[]}}]}};
 const foreignProfile=structuredClone(runtimeFan.operatingProfiles.find(profile=>profile.componentIndex===fanFixture.component.objectIndex));foreignProfile.loopId='another-native-circuit';foreignProfile.parameterControl.nodes.forEach(node=>node.source.objectName='FOREIGN LOOP MUST NOT APPEAR');runtimeFan.operatingProfiles.unshift(foreignProfile);
 state.simulationResult=runtimeResult;simulation.renderSimulation();
 const componentTarget=fixture=>[...host.querySelectorAll('[data-hvac-inspect-component]')].find(target=>target.dataset.hvacInspectPointName===fixture.component.objectName&&target.dataset.hvacInspectComponent.toLowerCase().includes(fixture.component.objectType.toLowerCase()));
 const chooseEquipment=fixture=>{const target=componentTarget(fixture);check(target,'typed equipment is not clickable: '+fixture.component.objectName);target.dispatchEvent(new MouseEvent('click',{bubbles:true}));};
 const runtimePanel=()=>host.querySelector('[data-hvac-runtime]'),stack=()=>host.querySelector('[data-hvac-runtime-availability]'),finalRow=()=>stack()?.querySelector('[data-hvac-runtime-availability-row="final-availability"]');
 const bottomFrame=()=>{const control=slider()?.closest('.hvac-inspection-frame-control'),snapshot=host.querySelector('[data-hvac-inspect-snapshot]'),custom=host.querySelector('.hvac-inspection-custom');return control&&snapshot&&custom&&Boolean(snapshot.compareDocumentPosition(control)&Node.DOCUMENT_POSITION_FOLLOWING)&&Boolean(custom.compareDocumentPosition(control)&Node.DOCUMENT_POSITION_FOLLOWING)&&(!runtimePanel()||Boolean(runtimePanel().compareDocumentPosition(control)&Node.DOCUMENT_POSITION_FOLLOWING));};
 check(host.querySelector('[data-hvac-inspect-topology]')&&!runtimePanel(),'default result is not basic loop view');
 check(!host.querySelector('select[data-hvac-inspect-view], select[data-hvac-runtime-profile], [data-hvac-runtime-frame]'),'loop view retained separate view/equipment/time selectors');
 check(bottomFrame(),'shared frame bar is not below loop panels');
 const nodeCharts=host.querySelector('[data-hvac-inspect-basic-charts]'),customControls=host.querySelector('[data-hvac-inspect-custom-controls]'),nodeToggles=[...host.querySelectorAll('[data-hvac-inspect-node-visible]')].map(input=>input.dataset.hvacInspectNodeVisible);
 chooseEquipment(fanFixture);
 check(runtimePanel()&&host.querySelector('[data-hvac-inspect-topology]')&&host.querySelector('[data-hvac-runtime-control]')&&host.querySelector('[data-hvac-runtime-history]'),'equipment selection replaced loop instead of inline control history');
 check(!host.textContent.includes('EDITOR MODEL MUST NOT APPEAR')&&!host.textContent.includes('FOREIGN LOOP MUST NOT APPEAR')&&runtimePanel().textContent.includes(fanFixture.component.objectName),'equipment selection borrowed editor/foreign-loop profile');
 check(!host.querySelector('[data-hvac-runtime-error]')&&bottomFrame(),'exact-time controls failed or frame moved above details');
 const resultBounds=host.closest('.result-pane').getBoundingClientRect(),bounded=[stack(),slider().closest('.hvac-inspection-frame-control'),host.querySelector('[data-hvac-inspect-frame-label]')];
 check(bounded.every(element=>{const rect=element.getBoundingClientRect();return rect.left>=resultBounds.left-1&&rect.right<=resultBounds.right+1;}),'loop min-content widened schedule/status/frame beyond visible pane: '+JSON.stringify({pane:[resultBounds.left,resultBounds.right],items:bounded.map(element=>{const rect=element.getBoundingClientRect();return [element.className,rect.left,rect.right];})}));
 check(host.querySelector('[data-hvac-inspect-basic-charts]')===nodeCharts&&host.querySelector('[data-hvac-inspect-custom-controls]')===customControls&&JSON.stringify([...host.querySelectorAll('[data-hvac-inspect-node-visible]')].map(input=>input.dataset.hvacInspectNodeVisible))===JSON.stringify(nodeToggles),'equipment selection changed existing node UI');
 check(!host.querySelector('[data-hvac-runtime-signal-list], .hvac-runtime-table, [data-hvac-runtime-source-detail], [data-hvac-runtime-decisions], select[data-hvac-runtime-profile], [data-hvac-runtime-frame]'),'equipment retained variable/spec tables or duplicate pickers');
 check(label()==='01-01 01:00'&&finalRow()?.dataset.state==='allowed','Hourly1h did not bind exact control1h observation');
 check([...stack().querySelectorAll('[data-hvac-runtime-availability-row]')].at(-1)===finalRow(),'final Availability is not bottom of stack');
 check(stack().querySelector('[data-kind="schedule"]')&&stack().querySelector('[data-kind="manager"]'),'schedule/manager absent from stack');
 const fanControlCard=()=>[...host.querySelectorAll('.hvac-parameter-flow-stage.controls [data-hvac-flow-card]')].find(card=>card.querySelector('strong')?.textContent==='Match airflow');
 check(fanControlCard()&&!fanControlCard().querySelector('[data-hvac-runtime-card-value]'),'fan controller borrowed unrelated temperature');
 frame(1);check(label()==='01-01 02:00'&&finalRow()?.dataset.state==='blocked'&&stack().querySelector('[data-kind="manager"][data-decisive="true"]'),'ForceOff did not block final or mark deciding manager');
 frame(2);check(label()==='01-01 03:00'&&finalRow()?.dataset.state==='allowed','CycleOn did not follow shared cursor');
 const nodeTarget=host.querySelector('[data-hvac-inspect-node]');check(nodeTarget,'loop node is not selectable');nodeTarget.dispatchEvent(new MouseEvent('click',{bubbles:true}));
 check(!runtimePanel()&&host.querySelector('[data-hvac-inspect-basic-charts]')===nodeCharts&&host.querySelector('[data-hvac-inspect-custom-controls]')===customControls&&label()==='01-01 03:00','node selection changed existing UI/current frame');
 chooseEquipment(coilFixture);check(runtimePanel()?.textContent.includes(coilFixture.component.objectName)&&!host.querySelector('[data-hvac-runtime-error]'),'same-loop equipment did not select exact controls');
 select(runtimeChiller.name);check(label()==='01-01 03:00'&&!runtimePanel(),'loop switch retained another circuit equipment');chooseEquipment(chillerFixture);
 check(runtimePanel()?.textContent.includes(chillerFixture.component.objectName)&&!host.querySelector('[data-hvac-runtime-error]'),'chiller borrowed fan/coil profile');
 select(runtimeFan.name);check(label()==='01-01 03:00','loop switch discarded shared time');
 state.simulationResult={...runtimeResult,runId:'runtime-2'};simulation.renderSimulation();check(slider().value==='0'&&label()==='01-01 01:00'&&!runtimePanel(),'new run retained prior time/equipment');
 const runtimeCoil=withHourly(runtimeClone(fixtures,'coil'),coilFixture);
 setReported(runtimeCoil,signal=>signal.role==='sensor'&&/temperature/i.test(signal.variableName),[22,22,22,22,23,23,23,23,24,24,24,24]);setReported(runtimeCoil,signal=>signal.role==='setpoint',[20]);
 setReported(runtimeCoil,signal=>signal.role==='actuator_request',[.8,.8,.8,.8,.7,.7,.7,.7,.6,.6,.6,.6]);setReported(runtimeCoil,signal=>signal.role==='actuator',[.3,.3,.3,.3,.2,.2,.2,.2,.1,.1,.1,.1]);
 state.simulationResult={...runtimeResult,runId:'runtime-coil-cards',purposeResults:{hvacLoops:[runtimeCoil]}};simulation.renderSimulation();chooseEquipment(coilFixture);
 const commandCard=()=>host.querySelector('.hvac-parameter-flow-stage.commands [data-hvac-flow-card]'),requestCardValue=()=>commandCard()?.querySelector('[data-hvac-runtime-card-value]')?.textContent,actualCardValue=()=>commandCard()?.querySelector('[data-hvac-runtime-card-actual]')?.textContent;
 check(requestCardValue()==='Requested flow: 0.8 kg/s'&&actualCardValue()==='Actual: 0.3 kg/s','sequence conflated request/actual or joined Hourly row1 to Timestep00:15');
 const originalCommandCard=commandCard();frame(1);
 check(commandCard()===originalCommandCard&&requestCardValue()==='Requested flow: 0.7 kg/s'&&actualCardValue()==='Actual: 0.2 kg/s','shared frame rebuilt graph or failed to update exact values');
 check([...host.querySelectorAll('.hvac-parameter-flow-stage.controls [data-hvac-runtime-card-value]')].some(value=>value.textContent==='Δ 3 °C'&&value.title.includes('23 °C − 20 °C')),'controller comparison did not follow current timestep');
 commandCard().querySelector('button').click();check(host.querySelector('[data-hvac-runtime-history] .hvac-chart-svg')&&!host.querySelector('[data-hvac-parameter-inspector]:not([hidden])'),'sequence card opened raw parameter specs instead of history');
 const requestedBinding=runtimeCoil.controlSignals.find(signal=>signal.role==='actuator_request'),requestedTrace=host.querySelector('[data-hvac-runtime-history] [data-hvac-chart-series="'+CSS.escape(requestedBinding.id)+'"]');
 check(requestedTrace?.querySelectorAll('[data-hvac-chart-value]').length===12&&requestedTrace.querySelector('.is-frame')?.dataset.hvacChartValue==='0.7','selected sequence history sampled native Timestep data or marked wrong current time');
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
 const missingRuntime=withHourly(runtimeClone(fixtures,'fan'),fanFixture),required=missingRuntime.controlSignals.find(signal=>signal.required&&signal.role==='manager_status');
 check(required,'fan lacks manager for acquisition regression');missingRuntime.controlSeries=missingRuntime.controlSeries.filter(series=>series.sourceId!==required.seriesId);
 state.simulationResult={...runtimeResult,runId:'runtime-missing',purposeResults:{hvacLoops:[missingRuntime]}};simulation.renderSimulation();chooseEquipment(fanFixture);
 check(host.querySelector('[data-hvac-runtime-error]')?.title.includes(required.variableName)&&host.querySelector('[data-hvac-inspect-topology]')&&slider()&&!slider().disabled,'diagnostics failure removed loop or became ordinary unknown');
 const unmatched=withHourly(runtimeClone(fixtures,'coil'),coilFixture);
 // Same-file Hourly row2 and nearby control rows do not establish a2h match.
 for(const series of unmatched.controlSeries)series.points=series.points.filter(point=>point.label!=='01-01 02:00');
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
  if(family==='coil'){setReported(captured,signal=>signal.role==='sensor'&&/temperature/i.test(signal.variableName),[22,23,24,25]);setReported(captured,signal=>signal.role==='setpoint',[20]);setReported(captured,signal=>signal.role==='actuator_request',[.8]);setReported(captured,signal=>signal.role==='actuator',[.3]);}
  const {setLanguage}=await import('/src/js/i18n.js');setLanguage(review.get('language')||'en');document.documentElement.dataset.theme=review.get('theme')||'dark';
  state.simulationResult={...runtimeResult,runId:'runtime-review-'+family,purposeResults:{hvacLoops:[captured]}};simulation.renderSimulation();chooseEquipment(fixture);if(review.get('review')!=='error')frame(1);
  if(review.get('focus')==='control')host.querySelector('[data-hvac-runtime-control]')?.scrollIntoView({block:'start'});
  if(review.get('focus')==='availability')host.querySelector('[data-hvac-runtime-availability]')?.scrollIntoView({block:'start'});
  if(review.get('focus')==='error')host.querySelector('[data-hvac-runtime-error]')?.scrollIntoView({block:'start'});
 }
 document.body.dataset.hvacFrameStatus='passed';document.getElementById('hvac-frame-result').textContent='passed';
}catch(error){document.body.dataset.hvacFrameStatus='failed';document.getElementById('hvac-frame-result').textContent=error.stack;}
</script>`
