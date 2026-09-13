package frontendchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

func TestSimulationRunInspectHVACAndComfortBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("actual Run & Inspect purpose-result browser regression")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	page := readTestFile(t, "frontend/src/index.html")
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		if !strings.Contains(page, script) {
			t.Fatalf("actual bootstrap changed: %s", script)
		}
		page = strings.Replace(page, script, "", 1)
	}
	page = strings.Replace(page, "</body>", epath142LayoutHTML+simulationPurposeResultsBrowserHTML+"</body>", 1)
	type browserResult struct {
		Failures []string `json:"failures"`
		Evidence []string `json:"evidence"`
	}
	done := make(chan browserResult, 1)
	inputPath := filepath.Join(t.TempDir(), "purpose-results.idf")
	if err := os.WriteFile(inputPath, []byte(simulationPurposeResultsHVACInput), 0o644); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/purpose-results.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	mux.HandleFunc("/purpose-results/run", func(w http.ResponseWriter, r *http.Request) {
		var request simulation.SimulationRunRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.PurposeRequest == nil {
			http.Error(w, "invalid purpose request", http.StatusBadRequest)
			return
		}
		result := simulationPurposeResultsBrowserFixture(request.RunID, inputPath, *request.PurposeRequest)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("/purpose-results/done", func(w http.ResponseWriter, r *http.Request) {
		var result browserResult
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		select {
		case done <- result:
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	profile := t.TempDir()
	command := exec.Command(chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0", "--window-size=1600,900", "--user-data-dir="+profile, server.URL+"/src/purpose-results.html?manual=1")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	stop := epath161OwnedChromeCleanup(t, command, profile)
	t.Cleanup(stop)
	select {
	case result := <-done:
		stop()
		for _, line := range result.Evidence {
			t.Log(line)
		}
		if len(result.Failures) != 0 {
			t.Fatalf("Run & Inspect purpose results: %s", strings.Join(result.Failures, "\n"))
		}
		if directory := os.Getenv("HVAC_INSPECTION_SCREENSHOT_DIR"); directory != "" {
			for _, capture := range []string{"topology", "graphs"} {
				path, err := filepath.Abs(filepath.Join(directory, "hvac-inspection-"+capture+".png"))
				if err != nil {
					t.Fatal(err)
				}
				shot, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--force-device-scale-factor=1", "--window-size=1600,1100", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--screenshot="+path, server.URL+"/src/purpose-results.html?manual=1&capture="+capture).CombinedOutput()
				if err != nil {
					t.Fatalf("HVAC inspection screenshot: %v\n%s", err, shot)
				}
				t.Log("Actual-app screenshot: " + path)
			}
		}
	case <-ctx.Done():
		stop()
		t.Fatal("Run & Inspect purpose browser did not report within its deadline")
	}
}

// Keep the wire contract and summary construction real: only the EnergyPlus
// series are fixtures. The browser sends its native request and receives the
// production purpose builder's HVAC/Comfort bundle through JSON.
func simulationPurposeResultsBrowserFixture(runID, inputPath string, request simulation.SimulationPurposeRequest) simulation.SimulationRunResult {
	series := func(key, name, unit string, first, middle, last float64) simulation.SimulationSeries {
		return simulation.SimulationSeries{
			File: "eplusout.sql", Column: key + ":" + name + " [" + unit + "]",
			KeyValue: key, Name: name, ReportingFrequency: "Hourly", RowCount: 3,
			Min: first, Max: last, Average: (first + middle + last) / 3,
			Points: []simulation.SimulationPoint{
				{X: 0, Label: "01/21 01:00:00", Value: first},
				{X: 1, Label: "01/21 02:00:00", Value: middle},
				{X: 2, Label: "01/21 03:00:00", Value: last},
			},
		}
	}
	result := simulation.SimulationRunResult{
		RunID: runID, Status: "succeeded", Filename: "purpose-results.idf", InputPath: inputPath,
		PurposeRunPlan: &simulation.PurposeRunPlan{Purposes: request.Purposes},
		Series: []simulation.SimulationSeries{
			series("SUPPLY OUTLET", "System Node Temperature", "C", 14, 16, 18),
			series("SUPPLY OUTLET", "System Node Mass Flow Rate", "kg/s", 0.5, 1, 1.5),
			series("SUPPLY OUTLET", "System Node Setpoint Temperature", "C", 16, 16, 16),
			series("SUPPLY OUTLET", "System Node Relative Humidity", "%", 45, 50, 55),
			series("SUPPLY INLET", "System Node Temperature", "C", 24, 25, 26),
			series("SUPPLY INLET", "System Node Mass Flow Rate", "kg/s", 0.5, 1, 1.5),
			series("SUPPLY INLET", "System Node Relative Humidity", "%", 50, 55, 60),
			series("SUPPLY FAN", "Fan Electricity Rate", "W", 0, 400, 500),
			series("RETURN OUTLET", "System Node Temperature", "C", 21, 22, 23),
			series("RETURN OUTLET", "System Node Mass Flow Rate", "kg/s", 2, 3, 4),
			series("RETURN OUTLET", "System Node Relative Humidity", "%", 60, 65, 70),
			series("RETURN FAN", "Fan Electricity Rate", "W", 700, 900, 1100),
			series("OFFICE", "Zone Mean Air Temperature", "C", 22, 24, 26),
			series("OFFICE", "Zone Thermostat Heating Setpoint Temperature", "C", 20, 20, 20),
			series("OFFICE", "Zone Thermostat Cooling Setpoint Temperature", "C", 25, 25, 25),
			series("OFFICE", "Zone Air Relative Humidity", "%", 45, 50, 55),
		},
	}
	bundle := simulation.BuildPurposeResultBundle(&result, request)
	result.PurposeResults = &bundle
	return result
}

const simulationPurposeResultsHVACInput = `Version,25.1;
AirLoopHVAC,SUPPLY LOOP,,,Autosize,SUPPLY BRANCHES,,SUPPLY INLET,SUPPLY RETURN,SUPPLY DEMAND,SUPPLY OUTLET;
BranchList,SUPPLY BRANCHES,SUPPLY BRANCH;
Branch,SUPPLY BRANCH,,Fan:ConstantVolume,SUPPLY FAN,SUPPLY INLET,SUPPLY OUTLET;
Fan:ConstantVolume,SUPPLY FAN,,0.7,600,Autosize,0.9,1,SUPPLY INLET,SUPPLY OUTLET;
AirLoopHVAC,RETURN LOOP,,,Autosize,RETURN BRANCHES,,RETURN INLET,RETURN RETURN,RETURN DEMAND,RETURN OUTLET;
BranchList,RETURN BRANCHES,RETURN BRANCH;
Branch,RETURN BRANCH,,Fan:ConstantVolume,RETURN FAN,RETURN INLET,RETURN OUTLET;
Fan:ConstantVolume,RETURN FAN,,0.7,600,Autosize,0.9,1,RETURN INLET,RETURN OUTLET;
`

const simulationPurposeResultsBrowserHTML = `<pre id="purpose-results-evidence" hidden>pending</pre><script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const tick=()=>new Promise(resolve=>setTimeout(resolve,0));
try{
 for(let n=0;document.body.dataset.epath142Status!=="manual"&&n<200;n++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath142Status!=="manual")throw Error("actual fixture not ready: "+document.getElementById("epath142-result")?.textContent);
 const {state}=await import("/src/js/state.js"),i18n=await import("/src/js/i18n.js");
 i18n.setLanguage("en");
 const original=state.simulationResult,originalJSON=JSON.stringify(original),requests=[];
 const selectedHVACLoop={id:'fixture-supply-loop',type:'AirLoopHVAC',name:'SUPPLY LOOP',supplySide:{branches:[{components:[{objectType:'Fan:ConstantVolume',objectName:'SUPPLY FAN',objectIndex:4}]}]}};
 state.report={...state.report,hvac:{...state.report?.hvac,loops:[selectedHVACLoop]}};
 state.activeHVACLoopId=selectedHVACLoop.id;state.activeHVACGraphKey='component:4';
 const run=document.getElementById("simulationRunButton");
 const tab=view=>document.querySelector('[data-simulation-result-view-button="'+view+'"]');
 check(tab("hvac_loops").disabled&&tab("comfort").disabled,"previous energy-only result unexpectedly enables HVAC/Comfort");
 for(const purpose of ["hvac_loop_check","comfort_check"]){
  const input=document.querySelector('[data-simulation-purpose="'+purpose+'"]');
  check(input&&!input.checked,"purpose should start unchecked: "+purpose);
  input.click();
  check(input.checked&&state.simulationSelectedPurposes.includes(purpose),"native checkbox did not preserve selected purpose: "+purpose);
 }
 window.go.main.App.RunPurposeSimulationText=async request=>{
  requests.push(request);
  const response=await fetch("/purpose-results/run",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(request)});
  if(!response.ok)throw Error(await response.text());
  return response.json();
 };
 check(!run.disabled,"Run & Inspect is disabled after purpose selection: "+run.title);
 run.click();
 for(let n=0;(requests.length===0||state.simulationRunning)&&n<500;n++)await new Promise(resolve=>setTimeout(resolve,10));
 check(requests.length===1,"native Run must send exactly one request");
 const request=requests[0];
 const scope=request?.purposeRequest?.scope||{};
 check(scope.loopMode==='all'&&![scope.airLoopNames,scope.plantLoopNames,scope.condenserLoopNames,scope.componentIds].some(items=>items?.length),'HVAC tab loop/component selection silently narrowed Run & Inspect');
 check(state.activeHVACLoopId===selectedHVACLoop.id&&state.activeHVACGraphKey==='component:4','Run & Inspect changed the HVAC tab selection');
 for(const purpose of ["basic_energy","zone_heat_flow","hvac_loop_check","comfort_check"]){
  check(request?.purposeRequest?.purposes.includes(purpose),"run request omitted purpose: "+purpose);
 }
 check(request?.resultMode==="sql_first"&&request?.purposeRequest?.sqlMode==="sql_first","run lost SQL-first result extraction");
 check(!state.simulationRunning&&state.simulationProgress?.status==="succeeded","purpose run failed or did not finish: "+document.getElementById("simulationStatus").textContent);
 check(state.simulationResult!==original&&state.simulationResult?.runId===request?.runId,"native completion did not install new result");
 check(!tab("hvac_loops").disabled&&!tab("comfort").disabled,"returned purpose data did not enable both result tabs");
 const loops=state.simulationResult?.purposeResults?.hvacLoops||[];
 check(loops.length===2&&loops.find(loop=>loop.name==='SUPPLY LOOP')?.series?.length===7&&loops.every(loop=>loop.topology?.name===loop.name),"production HVAC builder did not return exact executed loop topology and node membership");
 check(state.simulationResult?.purposeResults?.comfort?.zones?.[0]?.metrics?.length===4,"production Comfort builder did not return zone metrics");

 tab("hvac_loops").click();await tick();
 const hvac=document.getElementById("simulationHVACLoopResults");
 check(state.simulationActiveResultView==="hvac_loops"&&!hvac.closest('[data-simulation-result-view]').hidden,"HVAC tab did not open its result section");
 const change=(selector,value)=>{const input=hvac.querySelector(selector);if(!input)throw Error('missing native control '+selector);input.value=value;input.dispatchEvent(new Event('change',{bubbles:true}));};
 const chooseLoop=name=>{const input=hvac.querySelector('[data-simulation-hvac-loop]'),option=[...input.options].find(item=>item.textContent===name);check(Boolean(option),'missing loop option '+name);if(option)change('[data-simulation-hvac-loop]',option.value);};
 const vertex=name=>[...hvac.querySelectorAll('[data-hvac-inspect-point-name]')].find(item=>item.dataset.hvacInspectPointName===name);
 const metric=(name,id)=>vertex(name)?.querySelector('[data-hvac-inspect-metric="'+id+'"] .hvac-inspect-metric-value')?.textContent;
 const chooseEntity=(row,name)=>{const selector='[data-hvac-inspect-entity="'+row+'"]',input=hvac.querySelector(selector),option=[...input.options].find(item=>item.textContent===name);if(!option)throw Error('missing entity '+name);change(selector,option.value);};
 const chooseProperty=(row,name)=>{const selector='[data-hvac-inspect-property="'+row+'"]',input=hvac.querySelector(selector),option=[...input.options].find(item=>item.textContent.startsWith(name+' ('));if(!option)throw Error('missing property '+name);check(!option.disabled,'requested property unexpectedly disabled '+name);change(selector,option.value);};
 const basic=kind=>hvac.querySelector('[data-hvac-inspect-basic-chart="'+kind+'"]');
 const custom=()=>hvac.querySelector('[data-hvac-inspect-custom-chart]');
 chooseLoop('SUPPLY LOOP');
 const installedJSON=JSON.stringify(state.simulationResult);
 check(hvac.querySelector('[data-simulation-hvac-loop]').options.length===2&&hvac.querySelector('[data-hvac-inspect-topology]'),'loop picker or executed topology missing');
 check(!hvac.querySelector('table, .simulation-hvac-node-card, .simulation-hvac-derived-grid')&&!hvac.textContent.includes('Fan Electricity Rate')&&!hvac.textContent.includes('System Node Temperature')&&!hvac.textContent.includes('eplusout.sql'),'HVAC result still lists tables or raw source descriptions');
 check(vertex('SUPPLY OUTLET')&&vertex('SUPPLY FAN')&&!vertex('RETURN FAN'),'selected loop mixed another loop equipment');
 check(metric('SUPPLY OUTLET','temperature')==='14.00 °C'&&metric('SUPPLY OUTLET','flow')==='0.50 kg/s'&&metric('SUPPLY OUTLET','relativeHumidity')==='45.00 %'&&metric('SUPPLY OUTLET','setpoint')==='16.00 °C','first topology frame lost node temperature/flow/humidity/setpoint values');
 check(metric('SUPPLY FAN','power')==='0.00 kW'&&vertex('SUPPLY FAN')?.querySelector('.hvac-inspect-state.off'),'reported equipment zero power did not show Off');
 check(vertex('SUPPLY OUTLET')?.classList.contains('measured'),'frame node point was not emphasized');
 for(const kind of ['flow','temperature','humidity'])check(basic(kind)?.querySelectorAll('[data-hvac-chart-series]').length===2,'default node graph missing two observed points: '+kind);
 const frame=hvac.querySelector("[data-simulation-hvac-frame]");
 check(frame?.max==="2","HVAC frame slider did not use returned series length");
 const graphSVG=basic('temperature').querySelector('svg'),customSVG=custom().querySelector('svg');
 if(frame){frame.value="2";frame.dispatchEvent(new Event("input",{bubbles:true}));}
 check(state.simulationHVACFrameIndex===2&&metric('SUPPLY OUTLET','temperature')==='18.00 °C'&&metric('SUPPLY OUTLET','flow')==='1.50 kg/s'&&metric('SUPPLY OUTLET','relativeHumidity')==='55.00 %','frame interaction did not render exact returned final node values');
 check(metric('SUPPLY FAN','power')==='0.50 kW'&&vertex('SUPPLY FAN')?.querySelector('.hvac-inspect-state.on'),'frame did not update equipment status/power');
 check(hvac.querySelector('[data-simulation-hvac-frame]')===frame&&basic('temperature').querySelector('svg')===graphSVG&&custom().querySelector('svg')===customSVG,'frame slider recreated slider or complete trace graph');
 check(basic('temperature').querySelector('[data-hvac-chart-value="18"].is-frame')&&basic('temperature').querySelector('[data-hvac-chart-value="26"].is-frame'),'frame marker did not highlight exact time-aligned node graph observations');
 vertex('SUPPLY INLET').dispatchEvent(new MouseEvent('click',{bubbles:true}));
 check(vertex('SUPPLY INLET')?.getAttribute('aria-pressed')==='true','topology node selection did not emphasize selected point');
 const outletToggle=hvac.querySelector('[data-hvac-inspect-node-visible="node:supply outlet"]');outletToggle.click();
 for(const kind of ['flow','temperature','humidity'])check(basic(kind)?.querySelectorAll('[data-hvac-chart-series]').length===1&&!basic(kind)?.textContent.includes('SUPPLY OUTLET'),'node checkbox did not remove corresponding trace: '+kind);
 check(vertex('SUPPLY OUTLET')&&custom().querySelector('svg')===customSVG,'node graph visibility removed topology or unrelated custom graph');
 chooseEntity(0,'SUPPLY FAN');
 check(hvac.querySelector('[data-hvac-inspect-property="0"]').value===''&&hvac.querySelector('[data-hvac-inspect-property="0"]').options.length===2,'equipment choice did not reset to its own property choices');
 chooseProperty(0,'Electric power');chooseEntity(1,'SUPPLY OUTLET');chooseProperty(1,'Temperature');
 check(custom().querySelector('[data-hvac-chart-axis="left"]').dataset.hvacChartUnit==='kW'&&custom().querySelector('[data-hvac-chart-axis="right"]').dataset.hvacChartUnit==='°C','native custom property selection failed independent power/temperature Y axes');
 hvac.querySelector('[data-hvac-inspect-add]').click();chooseEntity(2,'SUPPLY INLET');
 const third=hvac.querySelector('[data-hvac-inspect-property="2"]');
 check([...third.options].find(item=>item.textContent.startsWith('Relative humidity ('))?.disabled&&![...third.options].find(item=>item.textContent.startsWith('Temperature ('))?.disabled,'line graph selector allowed third incompatible unit or blocked existing scale');
 change('[data-hvac-inspect-mode]','scatter');
 check(hvac.querySelectorAll('[data-hvac-inspect-entity]').length===2&&!hvac.querySelector('[data-hvac-inspect-add]'),'scatter did not limit custom comparison to two properties');
 check(custom().querySelectorAll('[data-hvac-chart-scatter] circle').length===3&&custom().querySelector('[data-hvac-chart-x-value="0"][data-hvac-chart-y-value="14"]'),'scatter did not compare exact power/temperature observations including zero');
 chooseEntity(0,'SUPPLY OUTLET');chooseProperty(0,'Mass flow');chooseEntity(1,'SUPPLY INLET');chooseProperty(1,'Temperature');
 check(custom().querySelector('[data-hvac-chart-x-value="0.5"][data-hvac-chart-y-value="24"]')&&custom().querySelector('[data-hvac-chart-x-value="1.5"][data-hvac-chart-y-value="26"]'),'two-step node properties did not update exact scatter pairs');
 const supplySelection=state.simulationHVACInspection.selectedLoop;
 chooseLoop('RETURN LOOP');
 check(vertex('RETURN OUTLET')&&vertex('RETURN FAN')&&!vertex('SUPPLY FAN')&&state.simulationHVACFrameIndex===0,'loop change leaked previous topology, equipment or frame');
 check(metric('RETURN OUTLET','temperature')==='21.00 °C'&&metric('RETURN FAN','power')==='0.70 kW','second loop displayed first loop observations');
 const secondFrame=hvac.querySelector('[data-simulation-hvac-frame]');secondFrame.value='1';secondFrame.dispatchEvent(new Event('input',{bubbles:true}));
 chooseLoop('SUPPLY LOOP');
 check(state.simulationHVACInspection.selectedLoop===supplySelection&&state.simulationHVACFrameIndex===2&&hvac.querySelector('[data-hvac-inspect-mode]').value==='scatter'&&!hvac.querySelector('[data-hvac-inspect-node-visible="node:supply outlet"]').checked,'returning to loop lost its frame, graph type or node toggles');
 check(custom().querySelector('[data-hvac-chart-x-value="1.5"][data-hvac-chart-y-value="26"]')&&metric('SUPPLY OUTLET','temperature')==='18.00 °C','returning to loop lost selected custom properties or snapshot');
 chooseLoop('RETURN LOOP');check(state.simulationHVACFrameIndex===1&&metric('RETURN OUTLET','temperature')==='22.00 °C','loop-local second frame was not retained');
 check(JSON.stringify(state.simulationResult)===installedJSON,'HVAC topology/graph interaction mutated returned observations');

 tab("comfort").click();await tick();
 const comfort=document.getElementById("simulationComfortResults");
 check(state.simulationActiveResultView==="comfort"&&!comfort.closest('[data-simulation-result-view]').hidden,"Comfort tab did not open its result section");
 const comfortZone=comfort.querySelector('[data-comfort-inspect-zone]');
 check(comfortZone&&[...comfortZone.options].some(option=>option.value==='OFFICE'),'Comfort result did not expose its reported zone');
 if(comfortZone){comfortZone.value='OFFICE';comfortZone.dispatchEvent(new Event('change',{bubbles:true}));}
 const comfortChart=kind=>comfort.querySelector('[data-comfort-inspect-chart="'+kind+'"]');
 const comfortValue=id=>{const metric=comfort.querySelector('[data-comfort-inspect-metric="'+id+'"]');return Number((metric?.querySelector('[data-comfort-inspect-value]')||metric)?.dataset.comfortInspectValue);};
 check(comfortValue('temperature')===24&&comfortValue('heatingSetpoint')===20&&comfortValue('coolingSetpoint')===25,'Comfort T/Tset indicators lost the actual returned averages');
 const temperature=comfortChart('temperature');
 check(temperature?.querySelectorAll('[data-hvac-chart-series]').length===3&&[22,24,26,20,25].every(value=>temperature.querySelector('[data-hvac-chart-value="'+value+'"]')),'Comfort graph lost observed temperature or heating/cooling setpoints');
 check(comfortChart('humidity')?.querySelectorAll('[data-hvac-chart-series] circle').length===3&&comfortValue('relativeHumidity')===50,'Comfort RH graph or indicator lost the returned samples');
 check(temperature?.querySelector('[data-hvac-chart-axis="x"]')&&temperature?.querySelector('[data-hvac-chart-axis="left"]')&&temperature?.querySelector('.hvac-chart-grid'),'Comfort graph is missing time/value axes or grid');
 check(!comfort.querySelector('table, dl, [data-semantic-id], [data-semantic-ref], [data-hvac-graph-key]')&&!['Source data','Related model entities','eplusout.sql','Zone Mean Air Temperature'].some(label=>comfort.textContent.includes(label)),'Comfort still exposes tables, raw source descriptions or navigation links');
 check(JSON.stringify(original)===originalJSON,"Run or purpose tabs mutated previous result");
 const capture=new URLSearchParams(location.search).get('capture');
 if(capture){
  tab('hvac_loops').click();chooseLoop('SUPPLY LOOP');
  if(capture==='graphs'){
   change('[data-hvac-inspect-mode]','line');chooseEntity(0,'SUPPLY FAN');chooseProperty(0,'Electric power');chooseEntity(1,'SUPPLY OUTLET');chooseProperty(1,'Temperature');
   hvac.querySelector('[data-hvac-inspect-custom-chart]').scrollIntoView({block:'end'});
  }else hvac.querySelector('[data-simulation-hvac-loop]').scrollIntoView({block:'start'});
  await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
 }
 evidence.push("Native purpose checkboxes preserve all four purposes in one SQL-first Run & Inspect request. Production Go builder returns two executed loop topologies with exact membership. Native loop/frame/node/custom property controls render clean topology, status and three node graphs; frame changes retain slider and trace SVG; yyaxes and exactly-two-property scatter use observed values. Loop-local selections survive switching. Comfort zone controls show reported T/Tset/RH graphs and indicators without tables or source links.");
}catch(error){failures.push(error.stack||String(error));}
document.body.dataset.purposeResultsStatus=failures.length?"failed":"passed";
document.getElementById("purpose-results-evidence").textContent=JSON.stringify({failures,evidence});
await fetch("/purpose-results/done",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({failures,evidence})});
</script>`
