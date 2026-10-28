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
		directory, captures := os.Getenv("HVAC_INSPECTION_SCREENSHOT_DIR"), []string{"topology", "graphs"}
		if comparisonDirectory := os.Getenv("SIMULATION_COMPARISON_SCREENSHOT_DIR"); comparisonDirectory != "" {
			directory, captures = comparisonDirectory, []string{"comparison"}
		}
		if directory != "" {
			for _, capture := range captures {
				filename := "hvac-inspection-" + capture + ".png"
				if capture == "comparison" {
					filename = "simulation-comparison-comfort.png"
				}
				path, err := filepath.Abs(filepath.Join(directory, filename))
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
		HeatFlow: simulation.HeatFlowDataset{
			SourceFile: "eplusout.sql", Unit: "W", TemperatureUnit: "C", FrameCount: 3, OriginalFrameCount: 3,
			Labels: []string{"01/21 01:00:00", "01/21 02:00:00", "01/21 03:00:00"},
			Categories: []simulation.HeatFlowCategory{
				{ID: "internalConvective", Label: "Internal convective gains", Unit: "W"},
				{ID: "surfaceConvection", Label: "Surface convection", Unit: "W"},
				{ID: "interzoneAir", Label: "Interzone air transfer", Unit: "W"},
				{ID: "outdoorAir", Label: "Outdoor air transfer", Unit: "W"},
				{ID: "systemAir", Label: "HVAC system air transfer", Unit: "W"},
				{ID: "systemConvective", Label: "HVAC/system convective gains", Unit: "W"},
				{ID: "airStorage", Label: "Air energy storage", Unit: "W"},
				{ID: "deviation", Label: "Heat balance deviation", Unit: "W"},
			},
			Zones: []simulation.HeatFlowZoneSeries{{Name: "OFFICE", Temperature: []float64{22, 24, 26},
				Values:   [][]float64{{0, 1000, 2000}, {100, 200, 300}, {0, 0, 0}, {-500, -400, -300}, {0, 0, -500}, {0, 0, 0}, {0, 100, 200}, {0, 0, 0}},
				Observed: [][]bool{{true, false, true}, {true, true, true}, {true, true, true}, {true, true, true}, {true, true, true}, {true, true, true}, {true, true, true}, {true, true, true}},
			}},
		},
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
			series("Environment", "Site Outdoor Air Drybulb Temperature", "C", -5, 0, 8),
			series("Environment", "Site Outdoor Air Relative Humidity", "%", 30, 45, 60),
			series("Environment", "Site Outdoor Air Barometric Pressure", "Pa", 100000, 100500, 101000),
		},
	}
	for index := range result.Series {
		item := &result.Series[index]
		item.SourceID = fmt.Sprintf("fixture-sql-%d", index)
		purposes := []simulation.SimulationPurposeID{simulation.SimulationPurposeHVACLoopCheck}
		if item.KeyValue == "OFFICE" {
			purposes = []simulation.SimulationPurposeID{simulation.SimulationPurposeComfort, simulation.SimulationPurposeZoneHeatFlow}
		} else if item.KeyValue == "Environment" {
			purposes = []simulation.SimulationPurposeID{simulation.SimulationPurposeBasicEnergy, simulation.SimulationPurposeZoneHeatFlow, simulation.SimulationPurposeComfort}
		}
		result.PurposeRunPlan.OutputObjects = append(result.PurposeRunPlan.OutputObjects, simulation.PurposeOutputObject{
			ObjectType: "Output:Variable", PurposeIDs: purposes, KeyValue: item.KeyValue, VariableName: item.Name, ReportingFrequency: item.ReportingFrequency,
		})
	}
	for index, name := range []string{
		"Zone Air Heat Balance Internal Convective Heat Gain Rate", "Zone Air Heat Balance Surface Convection Rate",
		"Zone Air Heat Balance Interzone Air Transfer Rate", "Zone Air Heat Balance Outdoor Air Transfer Rate",
		"Zone Air Heat Balance System Air Transfer Rate", "Zone Air Heat Balance System Convective Heat Gain Rate",
		"Zone Air Heat Balance Air Energy Storage Rate", "Zone Air Heat Balance Deviation Rate",
	} {
		result.HeatFlow.Categories[index].VariableName = name
		result.PurposeRunPlan.OutputObjects = append(result.PurposeRunPlan.OutputObjects, simulation.PurposeOutputObject{
			ObjectType: "Output:Variable", PurposeIDs: []simulation.SimulationPurposeID{simulation.SimulationPurposeZoneHeatFlow}, KeyValue: "OFFICE", VariableName: name, ReportingFrequency: "Hourly",
		})
	}
	bundle := simulation.BuildPurposeResultBundle(&result, request)
	result.PurposeResults = &bundle
	return result
}

const simulationPurposeResultsHVACInput = `Version,25.1;
GlobalGeometryRules,UpperLeftCorner,Counterclockwise,World;
Zone,OFFICE;
BuildingSurface:Detailed,OFFICE FLOOR,Floor,FLOOR CONSTRUCTION,OFFICE,,Ground,,NoSun,NoWind,1,4,0,0,0,10,0,0,10,5,0,0,5,0;
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
 const checkSupplySetpoint=comparison=>{const point=vertex('SUPPLY OUTLET'),inline=point?.querySelector('[data-hvac-inspect-metric="temperature"] [data-hvac-setpoint-state]');check(inline?.textContent===comparison&&inline.dataset.hvacSetpointState==='unknown'&&inline.querySelector('[baseline-shift="sub"]')?.textContent==='set'&&!point.querySelector('[data-hvac-inspect-metric="setpoint"]'),'fan-only loop lost exact inline setpoint comparison or invented a heating/cooling control state: '+comparison);};
 const checkFrame=(index,label)=>check(state.simulationHVACFrameIndex===index&&hvac.querySelector('[data-simulation-hvac-frame]')?.value===String(index)&&hvac.querySelector('[data-hvac-inspect-frame-label]')?.textContent===label,'shared HVAC frame does not match exact returned timestamp: '+label);
 const chooseEntity=(row,name)=>{const selector='[data-hvac-inspect-entity="'+row+'"]',input=hvac.querySelector(selector),option=[...input.options].find(item=>item.textContent===name);if(!option)throw Error('missing entity '+name);change(selector,option.value);};
 const chooseProperty=(row,name)=>{const selector='[data-hvac-inspect-property="'+row+'"]',input=hvac.querySelector(selector),option=[...input.options].find(item=>item.textContent.startsWith(name+' ('));if(!option)throw Error('missing property '+name);check(!option.disabled,'requested property unexpectedly disabled '+name);change(selector,option.value);};
 const basic=kind=>hvac.querySelector('[data-hvac-inspect-basic-chart="'+kind+'"]');
 const custom=()=>hvac.querySelector('[data-hvac-inspect-custom-chart]');
 chooseLoop('SUPPLY LOOP');
 const installedJSON=JSON.stringify(state.simulationResult);
 check(hvac.querySelector('[data-simulation-hvac-loop]').options.length===2&&hvac.querySelector('[data-hvac-inspect-topology]'),'loop picker or executed topology missing');
 check(!hvac.querySelector('table, .simulation-hvac-node-card, .simulation-hvac-derived-grid')&&!hvac.textContent.includes('Fan Electricity Rate')&&!hvac.textContent.includes('System Node Temperature')&&!hvac.textContent.includes('eplusout.sql'),'HVAC result still lists tables or raw source descriptions');
 check(vertex('SUPPLY OUTLET')&&vertex('SUPPLY FAN')&&!vertex('RETURN FAN'),'selected loop mixed another loop equipment');
 checkFrame(0,'01/21 01:00:00');
 check(metric('SUPPLY OUTLET','temperature')==='14.00 <set 16.00 °C'&&metric('SUPPLY OUTLET','flow')==='0.50 kg/s'&&metric('SUPPLY OUTLET','relativeHumidity')==='45.00 %','first topology frame lost exact node temperature/setpoint/flow/humidity values');
 checkSupplySetpoint('14.00 <set 16.00');
 check(metric('SUPPLY FAN','power')==='0.00 kW'&&vertex('SUPPLY FAN')?.querySelector('.hvac-inspect-state.off'),'reported equipment zero power did not show Off');
 check(vertex('SUPPLY OUTLET')?.classList.contains('measured'),'frame node point was not emphasized');
 for(const kind of ['flow','temperature','humidity'])check(basic(kind)?.querySelectorAll('[data-hvac-chart-series]').length===2,'default node graph missing two observed points: '+kind);
 const frame=hvac.querySelector("[data-simulation-hvac-frame]");
 check(frame?.max==="2","HVAC frame slider did not use returned series length");
 const graphSVG=basic('temperature').querySelector('svg'),customSVG=custom().querySelector('svg');
 if(frame){frame.value="2";frame.dispatchEvent(new Event("input",{bubbles:true}));}
 checkFrame(2,'01/21 03:00:00');
 check(metric('SUPPLY OUTLET','temperature')==='18.00 >set 16.00 °C'&&metric('SUPPLY OUTLET','flow')==='1.50 kg/s'&&metric('SUPPLY OUTLET','relativeHumidity')==='55.00 %','frame interaction did not render exact returned final node values');
 checkSupplySetpoint('18.00 >set 16.00');
 check(metric('SUPPLY FAN','power')==='0.50 kW'&&vertex('SUPPLY FAN')?.querySelector('.hvac-inspect-state.on'),'frame did not update equipment status/power');
 check(hvac.querySelector('[data-simulation-hvac-frame]')===frame&&basic('temperature').querySelector('svg')===graphSVG&&custom().querySelector('svg')===customSVG,'frame slider recreated slider or complete trace graph');
 const {prepareHVACInspection,hvacInspectionTrace}=await import('/src/js/hvac-inspection-data.js');
 const inspected=prepareHVACInspection(loops.find(loop=>loop.name==='SUPPLY LOOP')),at=inspected.frames[2],temperatureTraces=inspected.entities.filter(entity=>entity.kind==='node').flatMap(entity=>entity.properties.filter(property=>property.kind==='temperature').map(property=>hvacInspectionTrace(inspected,property)));
 const cursor=graphSVG.querySelector('[data-hvac-chart-frame]'),axis=graphSVG.querySelector('[data-hvac-chart-axis="left"]'),low=Number(axis.dataset.hvacChartYLow),high=Number(axis.dataset.hvacChartYHigh),top=Number(cursor.getAttribute('y1')),bottom=Number(cursor.getAttribute('y2'));
 check(temperatureTraces.length===2&&temperatureTraces.every(trace=>trace.points.length===3)&&[18,26].every(value=>temperatureTraces.some(trace=>trace.points[2].value===value&&trace.points[2].x===at.x)),'node line graphs lost exact complete source observations');
 check(graphSVG.querySelector('[data-hvac-chart-frame-marker]').style.display!=='none'&&Number(cursor.dataset.hvacChartFrame)===at.x&&Number(cursor.getAttribute('x1'))===Number(cursor.getAttribute('x2'))&&!graphSVG.querySelector('circle'),'marker-free node graph lost its exact current-time vertical cursor');
 for(const trace of temperatureTraces){const path=graphSVG.querySelector('[data-hvac-chart-series="'+CSS.escape(trace.id)+'"] path.hvac-chart-line'),commands=path?.getAttribute('d').match(/[ML][^ML]+/g)||[],end=commands.at(-1)?.slice(1).split(',').map(Number);check(commands.length===3&&end&&Math.abs(end[0]-Number(cursor.getAttribute('x1')))<1e-7&&Math.abs(high-(end[1]-top)/(bottom-top)*(high-low)-trace.points[2].value)<1e-7,'node line path does not preserve the exact time-aligned source endpoint');}
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
 change('[data-hvac-inspect-mode]','histogram');
 check(hvac.querySelector('[data-hvac-inspect-mode]').value==='histogram'&&hvac.querySelector('[data-hvac-inspect-add]')&&![...hvac.querySelector('[data-hvac-inspect-property="2"]').options].find(item=>item.textContent.startsWith('Relative humidity ('))?.disabled,'HVAC histogram mode is unavailable or retained line-only unit restrictions');
 chooseProperty(2,'Relative humidity');
 check(custom().querySelector('[data-hvac-inspection-chart="histogram"]')&&custom().querySelectorAll('[data-hvac-chart-series]').length===3&&custom().querySelectorAll('[data-hvac-chart-histogram-axis]').length===3,'native HVAC custom histogram did not overlay three units with stacked value axes');
 for(const trace of custom().querySelectorAll('[data-hvac-chart-series]'))check([...trace.querySelectorAll('[data-hvac-chart-bin-count]')].reduce((sum,bin)=>sum+Number(bin.dataset.hvacChartBinCount),0)===3,'native HVAC histogram lost complete observations');
 hvac.querySelector('[data-hvac-inspect-add]').click();check(hvac.querySelectorAll('[data-hvac-inspect-entity]').length===4,'HVAC histogram Add did not add one row');hvac.querySelector('[data-hvac-inspect-remove="3"]').click();check(hvac.querySelectorAll('[data-hvac-inspect-entity]').length===3,'HVAC histogram Remove did not remove one row');
 change('[data-hvac-inspect-mode]','scatter');
 check(hvac.querySelectorAll('[data-hvac-inspect-entity]').length===2&&!hvac.querySelector('[data-hvac-inspect-add]'),'scatter did not limit custom comparison to two properties');
 check(custom().querySelectorAll('[data-hvac-chart-scatter] circle').length===3&&custom().querySelector('[data-hvac-chart-x-value="0"][data-hvac-chart-y-value="14"]'),'scatter did not compare exact power/temperature observations including zero');
 chooseEntity(0,'SUPPLY OUTLET');chooseProperty(0,'Mass flow');chooseEntity(1,'SUPPLY INLET');chooseProperty(1,'Temperature');
 check(custom().querySelector('[data-hvac-chart-x-value="0.5"][data-hvac-chart-y-value="24"]')&&custom().querySelector('[data-hvac-chart-x-value="1.5"][data-hvac-chart-y-value="26"]'),'two-step node properties did not update exact scatter pairs');
 const supplySelection=state.simulationHVACInspection.selectedLoop;
 const selectedCustomProperties=()=>JSON.stringify([0,1].map(row=>({entity:hvac.querySelector('[data-hvac-inspect-entity="'+row+'"]').value,property:hvac.querySelector('[data-hvac-inspect-property="'+row+'"]').value})));
 const supplyCustomProperties=selectedCustomProperties();
 chooseLoop('RETURN LOOP');
 check(vertex('RETURN OUTLET')&&vertex('RETURN FAN')&&!vertex('SUPPLY OUTLET')&&!vertex('SUPPLY FAN'),'loop change leaked previous topology or equipment');
 checkFrame(2,'01/21 03:00:00');
 check(metric('RETURN OUTLET','temperature')==='23.00 °C'&&metric('RETURN OUTLET','flow')==='4.00 kg/s'&&metric('RETURN OUTLET','relativeHumidity')==='70.00 %'&&metric('RETURN FAN','power')==='1.10 kW','second loop did not show its own exact observations at the shared final timestamp');
 check(hvac.querySelector('[data-hvac-inspect-mode]').value==='line'&&hvac.querySelector('[data-hvac-inspect-node-visible="node:return outlet"]').checked,'shared frame leaked first-loop graph type or node visibility into the second loop');
 const secondFrame=hvac.querySelector('[data-simulation-hvac-frame]');secondFrame.value='0';secondFrame.dispatchEvent(new Event('input',{bubbles:true}));
 checkFrame(0,'01/21 01:00:00');
 check(metric('RETURN OUTLET','temperature')==='21.00 °C'&&metric('RETURN OUTLET','flow')==='2.00 kg/s'&&metric('RETURN OUTLET','relativeHumidity')==='60.00 %'&&metric('RETURN FAN','power')==='0.70 kW','second loop first timestamp lost its own exact observations');
 secondFrame.value='1';secondFrame.dispatchEvent(new Event('input',{bubbles:true}));
 checkFrame(1,'01/21 02:00:00');
 check(metric('RETURN OUTLET','temperature')==='22.00 °C'&&metric('RETURN OUTLET','flow')==='3.00 kg/s'&&metric('RETURN OUTLET','relativeHumidity')==='65.00 %'&&metric('RETURN FAN','power')==='0.90 kW','second loop middle timestamp lost its own exact observations');
 chooseLoop('SUPPLY LOOP');
 check(state.simulationHVACInspection.selectedLoop===supplySelection&&vertex('SUPPLY OUTLET')&&vertex('SUPPLY FAN')&&!vertex('RETURN OUTLET')&&!vertex('RETURN FAN'),'returning to loop lost selected topology or mixed second-loop equipment');
 checkFrame(1,'01/21 02:00:00');
 check(hvac.querySelector('[data-hvac-inspect-mode]').value==='scatter'&&!hvac.querySelector('[data-hvac-inspect-node-visible="node:supply outlet"]').checked&&vertex('SUPPLY INLET')?.getAttribute('aria-pressed')==='true','returning to loop lost its local graph type, node toggle or topology selection');
 check(selectedCustomProperties()===supplyCustomProperties&&custom().querySelectorAll('[data-hvac-chart-scatter] circle').length===3&&custom().querySelector('[data-hvac-chart-x-value="0.5"][data-hvac-chart-y-value="24"]')&&custom().querySelector('[data-hvac-chart-x-value="1.5"][data-hvac-chart-y-value="26"]'),'returning to loop lost its selected custom properties or exact complete scatter observations');
 check(custom().querySelector('[data-hvac-chart-x-value="1"][data-hvac-chart-y-value="25"].is-frame'),'restored loop-local scatter did not highlight the shared middle timestamp');
 check(metric('SUPPLY OUTLET','temperature')==='16.00 =set 16.00 °C'&&metric('SUPPLY OUTLET','flow')==='1.00 kg/s'&&metric('SUPPLY OUTLET','relativeHumidity')==='50.00 %'&&metric('SUPPLY FAN','power')==='0.40 kW','returning to loop did not render its own exact observations at the shared middle timestamp');
 checkSupplySetpoint('16.00 =set 16.00');
 chooseLoop('RETURN LOOP');checkFrame(1,'01/21 02:00:00');
 check(metric('RETURN OUTLET','temperature')==='22.00 °C'&&metric('RETURN OUTLET','flow')==='3.00 kg/s'&&metric('RETURN OUTLET','relativeHumidity')==='65.00 %'&&metric('RETURN FAN','power')==='0.90 kW','returning to second loop did not retain exact observations at the shared timestamp');
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
 const {prepareComfortInspection}=await import('/src/js/comfort-inspection-data.js'),returnedComfort=state.simulationResult.purposeResults.comfort,office=prepareComfortInspection(returnedComfort).zones.find(zone=>zone.id==='OFFICE');
 check(returnedComfort.zones.find(zone=>zone.zoneName==='OFFICE')?.metrics.every(metric=>metric.reportingFrequency==='Hourly'),'native Comfort purpose metrics lost their exact reported cadence');
 const comfortLine=(kind,category,expected)=>{
  const metric=office?.metrics.find(metric=>metric.kind===kind),chart=comfortChart(category),svg=chart?.querySelector('svg');
  check(metric&&metric.points.length===expected.length&&metric.points.every((point,index)=>point.value===expected[index]),'Comfort adapter lost exact complete native '+kind+' observations');
  if(!svg||!metric)return check(false,'Comfort '+kind+' graph or native metric is missing');
  const yAxis=svg.querySelector('[data-hvac-chart-axis="left"]'),cursor=svg.querySelector('[data-hvac-chart-frame]'),path=svg.querySelector('[data-hvac-chart-series="'+CSS.escape(metric.id)+'"] path.hvac-chart-line'),vertices=[...(path?.getAttribute('d')||'').matchAll(/([ML])([^,\s]+),([^\s]+)/g)];
  const yLow=Number(yAxis?.dataset.hvacChartYLow),yHigh=Number(yAxis?.dataset.hvacChartYHigh),plotTop=Number(cursor?.getAttribute('y1')),plotBottom=Number(cursor?.getAttribute('y2')),xLow=Number(svg.dataset.hvacChartDomainLow),xHigh=Number(svg.dataset.hvacChartDomainHigh),plotLeft=Number(svg.dataset.hvacChartPlotLeft),plotWidth=Number(svg.dataset.hvacChartPlotWidth);
  check(vertices.length===expected.length&&vertices.filter(vertex=>vertex[1]==='M').length===1&&!svg.querySelector('circle'),'Comfort '+kind+' ordinary line lost source vertices, acquired a false gap or retained point markers');
  check(vertices.every((vertex,index)=>Math.abs(yHigh-(Number(vertex[3])-plotTop)/(plotBottom-plotTop)*(yHigh-yLow)-expected[index])<1e-7&&Math.abs(xLow+(Number(vertex[2])-plotLeft)/plotWidth*(xHigh-xLow)-metric.points[index].x)<1e-7),'Comfort '+kind+' rendered line does not preserve exact time-aligned native values');
  check(metric.points.slice(1).every((point,index)=>point.x>metric.points[index].x&&point.x-metric.points[index].x===metric.points[1].x-metric.points[0].x),'consecutive hourly Comfort labels do not have uniform increasing chart times');
 };
 check(temperature?.querySelectorAll('[data-hvac-chart-series]').length===3,'Comfort graph lost observed temperature or heating/cooling setpoints');
 comfortLine('temperature','temperature',[22,24,26]);comfortLine('heatingSetpoint','temperature',[20,20,20]);comfortLine('coolingSetpoint','temperature',[25,25,25]);comfortLine('relativeHumidity','humidity',[45,50,55]);
 check(comfortChart('humidity')?.querySelectorAll('[data-hvac-chart-series]').length===1&&comfortValue('relativeHumidity')===50,'Comfort RH graph or indicator lost the returned samples');
 check(temperature?.querySelector('[data-hvac-chart-axis="x"]')&&temperature?.querySelector('[data-hvac-chart-axis="left"]')&&temperature?.querySelector('.hvac-chart-grid'),'Comfort graph is missing time/value axes or grid');
 check(!comfort.querySelector('table, dl, [data-semantic-id], [data-semantic-ref], [data-hvac-graph-key]')&&!['Source data','Related model entities','eplusout.sql','Zone Mean Air Temperature'].some(label=>comfort.textContent.includes(label)),'Comfort still exposes tables, raw source descriptions or navigation links');
 const {prepareSimulationComparison}=await import('/src/js/simulation-comparison-data.js'),{energyPathSeriesID}=await import('/src/js/energy-path-navigation.js');
 const comparison=view=>document.querySelector('[data-simulation-comparison-host="'+view+'"]');
 const comparisonChange=(view,selector,value)=>{const input=comparison(view)?.querySelector(selector);if(!input)throw Error('missing comparison control '+view+' '+selector);input.value=value;input.dispatchEvent(new Event('change',{bubbles:true}));};
 const comparisonEntries=view=>prepareSimulationComparison(state.simulationResult,view,state).entities.flatMap(entity=>entity.properties.map(property=>({entity,property})));
 const comparisonChoice=(view,row,entry)=>{
  check(Boolean(entry),'missing source-exact comparison property '+view);if(!entry)return;
  const entitySelector='[data-simulation-comparison-entity="'+row+'"]',propertySelector='[data-simulation-comparison-property="'+row+'"]',categorySelector='[data-simulation-comparison-category="'+row+'"]';
  const hasEntity=()=>[...(comparison(view)?.querySelector(entitySelector)?.options||[])].some(option=>option.value===entry.entity.id);
  if(!hasEntity())for(const category of [...(comparison(view)?.querySelector(categorySelector)?.options||[])].filter(option=>!option.disabled).map(option=>option.value)){comparisonChange(view,categorySelector,category);if(hasEntity())break;}
  check(hasEntity(),'comparison category lost related entity '+entry.entity.name);comparisonChange(view,entitySelector,entry.entity.id);comparisonChange(view,propertySelector,entry.property.id);
  check(state.simulationComparisons[view].rows[row]?.entity===entry.entity.id&&state.simulationComparisons[view].rows[row]?.property===entry.property.id,'native comparison selection did not retain exact entity/property '+view);
 };
 const nativeEntry=(view,name)=>{const source=state.simulationResult.series.find(series=>series.name===name),id=source&&energyPathSeriesID(source),matches=comparisonEntries(view).filter(entry=>entry.property.seriesId===id||Boolean(source?.sourceId)&&entry.property.seriesId===source.sourceId);check(matches.length===1,'comparison source must resolve exactly once '+view+' '+name);return matches[0];};
 const drawnComparison=trace=>{const svg=trace.closest('svg[data-hvac-chart-domain-low]'),axis=svg.querySelector('[data-hvac-chart-axis="'+trace.dataset.hvacChartSeriesAxis+'"]'),cursor=svg.querySelector('[data-hvac-chart-frame]'),low=Number(axis.dataset.hvacChartYLow),high=Number(axis.dataset.hvacChartYHigh),top=Number(cursor.getAttribute('y1')),bottom=Number(cursor.getAttribute('y2'));return [...(trace.querySelector('path.hvac-chart-line')?.getAttribute('d')||'').matchAll(/([ML])([^,\s]+),([^\s]+)/g)].map(match=>({command:match[1],value:high-(Number(match[3])-top)/(bottom-top)*(high-low)}));};
 const valuesEqual=(points,values)=>points.length===values.length&&points.every((point,index)=>Math.abs(point.value-values[index])<1e-7);
 check(!tab('series')&&!document.querySelector('[data-simulation-result-view="series"]'),'Run & Inspect retained the removed standalone Series view');
 check(comparison('comfort')===comfort.lastElementChild&&comfort.querySelector('[data-comfort-inspect-chart="temperature"]'),'Comfort comparison did not follow the existing purpose charts');
 comparisonChoice('comfort',0,nativeEntry('comfort','Zone Mean Air Temperature'));comparisonChoice('comfort',1,nativeEntry('comfort','Site Outdoor Air Drybulb Temperature'));
 comparisonChange('comfort','[data-simulation-comparison-mode]','scatter');
 const comfortScatter=comparison('comfort').querySelector('[data-hvac-chart-scatter]');
 check(comfortScatter?.querySelectorAll('circle').length===3&&[22,24,26].every((value,index)=>comfortScatter.querySelector('[data-hvac-chart-x-value="'+value+'"][data-hvac-chart-y-value="'+[-5,0,8][index]+'"]')),'Comfort scatter did not pair complete exact indoor/outdoor observations, including negative and zero temperature');
 comparisonChange('comfort','[data-simulation-comparison-mode]','line');
 const comfortLines=[...comparison('comfort').querySelectorAll('[data-hvac-chart-series]')];
 check(comfortLines.length===2&&!comparison('comfort').querySelector('circle')&&comfortLines.some(trace=>valuesEqual(drawnComparison(trace),[22,24,26]))&&comfortLines.some(trace=>valuesEqual(drawnComparison(trace),[-5,0,8])),'Comfort line comparison omitted native values or restored point markers');
 const comparisonLegend=comparison('comfort').querySelector('[data-hvac-chart-legend]');comparisonLegend?.dispatchEvent(new MouseEvent('pointerover',{bubbles:true}));
 check(comparison('comfort').querySelectorAll('[data-hvac-chart-series].is-highlighted').length===1&&comparison('comfort').querySelectorAll('[data-hvac-chart-series].is-muted').length===1,'comparison legend hover did not highlight only its matching trace');comparisonLegend?.dispatchEvent(new MouseEvent('pointerout',{bubbles:true}));
 comparisonChange('comfort','[data-simulation-comparison-mode]','histogram');
 check(comparison('comfort').querySelector('[data-hvac-inspection-chart="histogram"]')&&comparison('comfort').querySelectorAll('[data-hvac-chart-series]').length===2&&comparison('comfort').querySelectorAll('[data-hvac-chart-histogram-axis]').length===1,'Comfort comparison did not overlay indoor/outdoor histograms on the same temperature axis');
 comparison('comfort').querySelector('[data-simulation-comparison-add]').click();
 comparisonChoice('comfort',2,nativeEntry('comfort','Zone Air Relative Humidity'));
 comparison('comfort').querySelector('[data-simulation-comparison-add]').click();
 comparisonChoice('comfort',3,nativeEntry('comfort','Site Outdoor Air Barometric Pressure'));
 check(comparison('comfort').querySelectorAll('[data-hvac-chart-histogram-axis]').length===3,'purpose histogram retained the two-unit line restriction');
 const comfortHistogramGroups=[...comparison('comfort').querySelectorAll('[data-hvac-chart-series]')];
 check(comfortHistogramGroups.length>=3&&comfortHistogramGroups.every(trace=>[...trace.querySelectorAll('[data-hvac-chart-bin-count]')].reduce((sum,bin)=>sum+Number(bin.dataset.hvacChartBinCount),0)===3),'Comfort histogram did not preserve complete purpose/environment observations');
 comparison('comfort').querySelector('[data-simulation-comparison-remove="3"]').click();comparison('comfort').querySelector('[data-simulation-comparison-remove="2"]').click();
 comparisonChange('comfort','[data-simulation-comparison-mode]','line');
 check(comparison('comfort').querySelectorAll('[data-hvac-chart-series]').length===2&&comparison('comfort').querySelectorAll('[data-simulation-comparison-property]').length===2,'Histogram Remove or return to Line lost the original selected traces');
 const chooseComfortScope=value=>{const input=comfort.querySelector('[data-comfort-inspect-zone]');input.value=value;input.dispatchEvent(new Event('change',{bubbles:true}));};
 chooseComfortScope('__building__');
 check(comparisonEntries('comfort').every(entry=>entry.entity.category==='environment'||entry.entity.name==='Building'),'Building comparison fabricated a facility result from another zone or exposed an unrelated zone target');
 check(state.simulationComparisons.comfort.rows.every(row=>!row.entity||prepareSimulationComparison(state.simulationResult,'comfort',state).entities.some(entity=>entity.id===row.entity)),'Building comparison retained an unavailable previous zone target');
 chooseComfortScope('OFFICE');comparisonChoice('comfort',0,nativeEntry('comfort','Zone Mean Air Temperature'));comparisonChoice('comfort',1,nativeEntry('comfort','Site Outdoor Air Drybulb Temperature'));
 const comfortComparisonRows=JSON.stringify(state.simulationComparisons.comfort.rows);
 tab('zone_heat_flow').click();await tick();const heatFlow=document.getElementById('simulationHeatFlow');
 check(comparison('zone_heat_flow')===heatFlow.lastElementChild,'Heat Flow comparison did not follow the existing ledger and history');
 const gains=comparisonEntries('zone_heat_flow').filter(entry=>entry.property.label==='Internal convective gains');check(gains.length===1,'Heat Flow comparison duplicated or omitted the selected zone heat-gain variable');
 comparisonChoice('zone_heat_flow',0,gains[0]);comparisonChoice('zone_heat_flow',1,nativeEntry('zone_heat_flow','Site Outdoor Air Drybulb Temperature'));
 const heatFlowLines=[...comparison('zone_heat_flow').querySelectorAll('[data-hvac-chart-series]')],gainsLine=heatFlowLines.find(trace=>valuesEqual(drawnComparison(trace),[0,2]));
 check(heatFlowLines.length===2&&gainsLine&&drawnComparison(gainsLine).filter(point=>point.command==='M').length===2&&heatFlowLines.some(trace=>valuesEqual(drawnComparison(trace),[-5,0,8]))&&!comparison('zone_heat_flow').querySelector('circle'),'Heat Flow comparison lost W-to-kW scaling, known zero, observed-mask gap or exact environmental values');
 const heatRows=state.simulationComparisons.zone_heat_flow.rows.length;comparison('zone_heat_flow').querySelector('[data-simulation-comparison-add]')?.click();
 check(state.simulationComparisons.zone_heat_flow.rows.length===heatRows+1,'one native Add click did not add exactly one comparison row');comparison('zone_heat_flow').querySelector('[data-simulation-comparison-remove="'+heatRows+'"]')?.click();check(state.simulationComparisons.zone_heat_flow.rows.length===heatRows,'native Remove did not remove exactly its comparison row');
 const heatSVG=comparison('zone_heat_flow').querySelector('svg'),heatCursor=heatSVG?.querySelector('[data-hvac-chart-frame]'),heatFrame=document.getElementById('simulationHeatFlowSlider');heatFrame.value='2';heatFrame.dispatchEvent(new Event('input',{bubbles:true}));
 check(comparison('zone_heat_flow').querySelector('svg')===heatSVG&&heatCursor&&Math.abs(Number(heatCursor.getAttribute('x1'))-(Number(heatSVG.dataset.hvacChartPlotLeft)+Number(heatSVG.dataset.hvacChartPlotWidth)))<1e-7,'Heat Flow frame did not move the current-time cursor without rebuilding its comparison chart');
 comparisonChange('zone_heat_flow','[data-simulation-comparison-mode]','histogram');
 const heatHistogram=comparison('zone_heat_flow').querySelector('svg'),heatBins=[...heatHistogram.querySelectorAll('[data-hvac-chart-series]')].map(trace=>[...trace.querySelectorAll('[data-hvac-chart-bin-count]')].reduce((sum,bin)=>sum+Number(bin.dataset.hvacChartBinCount),0));
 check(heatBins.length===2&&heatBins.includes(2)&&heatBins.includes(3),'Heat Flow histogram counted unobserved masked values or omitted exact environment observations');
 const heatHistogramHTML=heatHistogram.innerHTML;heatFrame.value='1';heatFrame.dispatchEvent(new Event('input',{bubbles:true}));
 check(comparison('zone_heat_flow').querySelector('svg')===heatHistogram&&heatHistogram.innerHTML===heatHistogramHTML,'Heat Flow current frame rebuilt/rebinned the complete selected-range histogram');
 tab('comfort').click();await tick();check(JSON.stringify(state.simulationComparisons.comfort.rows)===comfortComparisonRows&&comparison('comfort').querySelectorAll('[data-hvac-chart-series]').length===2,'switching purpose tabs lost Comfort comparison choices or mixed Heat Flow selections');
 check(JSON.stringify(state.simulationResult)===installedJSON,'Comfort zone/line rendering mutated exact returned metric metadata or observations');
 check(JSON.stringify(original)===originalJSON,"Run or purpose tabs mutated previous result");
 const capture=new URLSearchParams(location.search).get('capture');
 if(capture){
  if(capture==='comparison'){tab('comfort').click();comparison('comfort').scrollIntoView({block:'center'});}
  else {tab('hvac_loops').click();chooseLoop('SUPPLY LOOP');
  if(capture==='graphs'){
   change('[data-hvac-inspect-mode]','line');chooseEntity(0,'SUPPLY FAN');chooseProperty(0,'Electric power');chooseEntity(1,'SUPPLY OUTLET');chooseProperty(1,'Temperature');
   hvac.querySelector('[data-hvac-inspect-custom-chart]').scrollIntoView({block:'end'});
  }else hvac.querySelector('[data-simulation-hvac-loop]').scrollIntoView({block:'start'});}
  await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
 }
 evidence.push("Native purpose checkboxes preserve all four purposes in one SQL-first Run & Inspect request. Production Go builder returns two executed loop topologies with exact membership. Native loop/frame/node/custom property controls render clean topology, exact inline temperature/setpoint comparisons, status and three node graphs; frame changes retain slider and trace SVG; yyaxes and exactly-two-property scatter use observed values. The timestamp is shared across loops while graph type, node selections/toggles and custom properties remain loop-local. HVAC Custom and Comfort comparison overlay complete distributions with three unit axes stacked below one plot, and retain Add/Remove plus line/scatter choices. Comfort retains its T/Tset/RH charts. Heat Flow comparison preserves observed masks, kW scaling, known zero, separate purpose choices and the shared-frame cursor; its histogram remains unchanged during frame stepping.");
}catch(error){failures.push(error.stack||String(error));}
document.body.dataset.purposeResultsStatus=failures.length?"failed":"passed";
document.getElementById("purpose-results-evidence").textContent=JSON.stringify({failures,evidence});
await fetch("/purpose-results/done",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({failures,evidence})});
</script>`
