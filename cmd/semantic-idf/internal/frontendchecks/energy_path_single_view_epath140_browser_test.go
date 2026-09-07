package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestEPATH140ActualEnergyDashboardSingleViewAndNavigation(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser single Energy view acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath140", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath140SingleViewHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--window-size=1200,900", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath140").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH140 browser failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, "data-epath140-status=\"passed\"") {
		if start := strings.Index(document, "<pre id=\"result\">"); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH140 failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH140 failed:\n%s", output)
	}
}

const epath140SingleViewHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>EPATH140 actual Energy dashboard</title>
<link rel="stylesheet" href="/src/styles/base.css"><link rel="stylesheet" href="/src/styles/simulation.css"><style>body{margin:0;overflow:auto}#simulationPane{max-width:1100px}button{font:inherit}</style>
<script>window.go={main:{App:{GetSimulationEnvironment:async()=>({installations:[],weatherFolders:[]})}}};window.runtime={EventsOn(){}};</script></head>
<body data-epath140-status="pending"><div id="runtimeStatus"></div><div id="semanticEditor" hidden></div><div id="simulationPane" class="result-pane active">
<div id="simulationResultTabs"><button data-simulation-result-view-button="energy">Energy</button><button data-simulation-result-view-button="series">Series</button></div>
<section data-simulation-result-view="energy"><div id="simulationEnergyStats"></div><div id="simulationEnergyDashboard"></div></section>
<section data-simulation-result-view="series" hidden><div id="simulationSeriesStats"></div><div id="simulationChart"></div></section>
</div><div id="hvacPane" class="result-pane"><div id="hvacSummary"></div><div id="hvacGraph"></div></div><pre id="result">pending</pre><script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
const level={status:"complete",found:1,total:1};
const quality={drivers:level,loads:level,endUses:level,carriers:level,ratios:level,driverToLoadClosedPct:100,endUseToCarrierClosedPct:100,driverToLoadStatus:"complete",endUseToCarrierStatus:"complete",zoneAllocatedPct:100,unassignedPct:0,zoneAllocationStatus:"complete"};
const sources=[
 {id:"sql-rdd-11",sourceType:"sql_variable",name:"Zone Air System Sensible Cooling Energy",keyValue:"Office",zoneName:"Office",reportingFrequency:"Monthly",isMeter:false,sourceUnit:"J",normalizedUnit:"kWh"},
 {id:"sql-rdd-12",sourceType:"sql_variable",name:"Zone Air System Sensible Cooling Energy",keyValue:"Office",zoneName:"Office",reportingFrequency:"Hourly",isMeter:false,sourceUnit:"J",normalizedUnit:"kWh"},
 {id:"sql-rdd-21",sourceType:"sql_variable",name:"Zone People Convective Heating Energy",keyValue:"Office",zoneName:"Office",reportingFrequency:"Monthly",isMeter:false,sourceUnit:"J",normalizedUnit:"kWh"},
 {id:"sql-rdd-31",sourceType:"sql_meter",name:"Cooling:Electricity",keyValue:"",reportingFrequency:"Monthly",isMeter:true,sourceUnit:"J",normalizedUnit:"kWh"},
 {id:"derived.load",sourceType:"derived_load",name:"Office sensible cooling total",inputSourceIds:["sql-rdd-11"],formula:"sum(monthly sensible cooling energy)",normalizedUnit:"kWh"},
 {id:"missing.heating",sourceType:"sql_variable",name:"Zone Air System Sensible Heating Energy",keyValue:"Office",zoneName:"Office",reportingFrequency:"Monthly",isMeter:false,sourceUnit:"J",normalizedUnit:"kWh"},
];
const graph=(scope,period,cooling)=>{
 const suffix=scope.kind==="zone"?"office":"building";
 const node=(id,level,value,extra={})=>({id:id+"."+suffix,level,value,rawValue:value,effectiveValue:value,allocatedValue:value,unit:"kWh",period,aggregationBasis:"model_total",...(scope.kind==="zone"?{zoneName:"Office"}:{}),...extra});
 const nodes=[
  node("driver.internal.people.cooling","driver",cooling,{driverCategory:"internal.people",serviceKind:"cooling",scaleDomain:"thermal",basis:"heat_balance_share",allocationApplied:true,sourceIds:["sql-rdd-21"],relatedPathIds:["path.office.cooling"]}),
  node("load.cooling","load",cooling,{serviceKind:"cooling",scaleDomain:"thermal",sourceIds:["derived.load"],relatedPathIds:["path.office.cooling"]}),
  node("end_use.cooling","end_use",cooling/4,{endUse:"cooling",serviceKind:"cooling",scaleDomain:"site",sourceIds:["sql-rdd-31"],relatedPathIds:["path.office.cooling"]}),
  node("carrier.electricity","carrier",cooling/4,{carrier:"electricity",scaleDomain:"site",sourceIds:["sql-rdd-31"]}),
  node("load.heating","load",10,{serviceKind:"heating",scaleDomain:"thermal",sourceIds:["missing.heating"],relatedPathIds:["path.deleted"]}),
 ];
 const link=(id,from,to,fromValue,toValue,relation,extra={})=>({id:id+"."+suffix,fromId:from+"."+suffix,toId:to+"."+suffix,fromValue,toValue,fromUnit:"kWh",toUnit:"kWh",relation,serviceKind:"cooling",period,...extra});
 const links=[
  link("link.driver","driver.internal.people.cooling","load.cooling",cooling,cooling,"driver_to_load",{sourceIds:["sql-rdd-21","derived.load"]}),
  link("link.conversion","load.cooling","end_use.cooling",cooling,cooling/4,"load_to_end_use",{sourceIds:["derived.load","sql-rdd-31"],ratio:4,ratioKind:"coefficient_of_performance"}),
  link("link.carrier","end_use.cooling","carrier.electricity",cooling/4,cooling/4,"end_use_to_carrier",{sourceIds:["sql-rdd-31"]}),
 ];
 const summary={schema:"semantic-idf.energy-explanation-summary/v2",scope,period,quality,drivers:nodes.filter(n=>n.level==="driver"),loads:nodes.filter(n=>n.level==="load"),endUses:nodes.filter(n=>n.level==="end_use"),carriers:nodes.filter(n=>n.level==="carrier"),completeness:{mappedPercent:100}};
 return {id:period,nodes,links,summary,quality};
};
const buildingScope={kind:"building",aggregationBasis:"model_total"};
const zoneScope={kind:"zone",zoneName:"Office",aggregationBasis:"model_total"};
const building=graph(buildingScope,"annual",300),zone=graph(zoneScope,"annual",150);
const explanation={schema:"semantic-idf.energy-explanation/v2",scope:buildingScope,...building,sources,periods:[graph(buildingScope,"M1",100),graph(buildingScope,"M2",200)],zoneResults:[{scope:zoneScope,...zone,periods:[graph(zoneScope,"M1",50),graph(zoneScope,"M2",100)]}]};
const series=[
 {file:"eplusout.sql",column:"Office:Zone Air System Sensible Cooling Energy [J]",sourceId:"sql-rdd-12",name:sources[1].name,keyValue:"Office",isMeter:false,reportingFrequency:"Hourly",points:[{x:0,value:900,label:"01-01 01:00"},{x:1,value:901,label:"02-01 01:00"}]},
 {file:"eplusout.sql",column:"Office:Zone Air System Sensible Cooling Energy [J]",sourceId:"sql-rdd-11",name:sources[0].name,keyValue:"Office",isMeter:false,reportingFrequency:"Monthly",points:[{x:12,value:100,label:"01-31 24:00"},{x:1,value:200,label:"02-28 24:00"},{x:0,value:300,label:"03-31 24:00"}]},
];
const result=freeze({status:"succeeded",purposeResults:{energyExplanation:explanation,energyExplanationSummary:building.summary},purposeRunPlan:{outputObjects:[{objectType:"Output:Variable",keyValue:"Office",variableName:sources[0].name,reportingFrequency:"Monthly"}]},series});
const originalJSON=JSON.stringify(result);
const report={hvac:{loops:[],serviceModel:{zoneServices:[{zoneName:"Office",paths:[
 {id:"path.office.cooling",serviceKind:"cooling",zoneName:"Office",servedSubject:{kind:"zone",zoneName:"Office",name:"Office"}},
 {id:"path.unrelated.heating",serviceKind:"heating",zoneName:"Office",servedSubject:{kind:"zone",zoneName:"Office",name:"Office"}},
]}],couplings:[]}}};
try{
 const [{state},simulation,navigation,controller,adapters,hvacViews,appNavigation,viewHistory]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/energy-path-navigation.js"),import("/src/js/selection-controller.js"),import("/src/js/panel-navigation-adapters.js"),import("/src/js/views/hvac-views.js"),import("/src/js/navigation.js"),import("/src/js/view-history.js")]);
 simulation.initializeSimulationControls();
 await new Promise(resolve=>setTimeout(resolve,0));
 const host=document.getElementById("simulationEnergyDashboard");
 Object.assign(state,{report,simulationResult:result,simulationActiveResultView:"energy",activeResultTab:"simulation",simulationEnergyScopeKind:"building",simulationEnergyPeriod:"annual",simulationEnergyService:"all",simulationEnergySelection:"",simulationEnergyDetailsOpen:false,simulationEnergyDetailsTab:"data",simulationEnergyDetailsStage:"",simulationEnergyOutputSource:""});
 const pathEntity=id=>({id:"entity."+id,kind:"hvac-path",label:"Office cooling service",viewTargets:[{view:"hvac",targetKind:"service-path",targetId:id,label:"Office cooling service"}]});
 state.semanticProjection={navigation:{entities:[pathEntity("path.office.cooling")]}};
 state.analysisDirty={hvac:false,simulation:false};state.analysisReady={hvac:true,simulation:true};
 adapters.initializeResultPanelNavigationAdapters();hvacViews.initializeHVACControls();
 controller.configureSelectionController({state,getNavigationIndex:()=>state.semanticProjection.navigation,isAnalysisCurrent:()=>true,getActivePanelView:()=>state.activeResultTab,
  recordHistory:payload=>{const snapshot=viewHistory.captureViewSnapshot();if(payload.previous)snapshot.globalSelection=payload.previous;viewHistory.recordViewHistory(snapshot);},
  openView:async(destination,options)=>appNavigation.switchResultTab(destination,{...options,recordHistory:false}),
  onSelectionChange:detail=>window.dispatchEvent(new CustomEvent("idfAnalyzer:semanticSelectionChanged",{detail}))});
 hvacViews.renderHVAC(report.hvac);
 simulation.renderSimulationEnergyDashboard(result);
 const context=()=>JSON.stringify([state.simulationEnergyScopeKind,state.simulationEnergyZoneName,state.simulationEnergyPeriod,state.simulationEnergyService,state.simulationEnergySelection,state.simulationEnergyDetailsOpen,state.simulationEnergyDetailsTab,state.simulationEnergyDetailsStage,state.simulationEnergyOutputSource]);
 const change=(selector,value)=>{const control=host.querySelector(selector);check(Boolean(control),"missing real Energy control "+selector);if(control){control.value=value;control.dispatchEvent(new Event("change",{bubbles:true}));}};
 const select=id=>{const node=host.querySelector('[data-energy-explanation-node="'+id+'"]');check(Boolean(node),"missing selected graph node "+id);node?.focus();node?.click();check(document.activeElement?.dataset.energyExplanationNode===id,"node activation discarded keyboard focus: "+id);};
 const returnEnergy=()=>document.querySelector('[data-simulation-result-view-button="energy"]')?.click();
 const serviceCandidate=targetID=>simulation.simulationEnergyServiceNavigation({id:state.simulationEnergySelection,...(state.simulationResult.purposeResults.energyExplanation.zoneResults[0].periods.find(period=>period.id===state.simulationEnergyPeriod)||state.simulationResult.purposeResults.energyExplanation.zoneResults[0]).nodes.find(node=>node.id===state.simulationEnergySelection)}).groups.flatMap(group=>group.candidates).find(candidate=>candidate.target?.targetId===targetID);
 const serviceButton=targetID=>{const candidate=serviceCandidate(targetID);return candidate?host.querySelector('[data-energy-path-service-destination="'+candidate.id+'"]'):null;};
 if(new URLSearchParams(location.search).get("manual")==="1"){
  document.body.dataset.epath140Status="manual";document.getElementById("result").textContent="Manual EPATH-140 fixture: use scope, month, service and node inspector; load has exact Monthly Series and HVAC targets.";
 }else{
 check(host.querySelectorAll(".energy-path-view").length===1&&host.querySelectorAll(".energy-path-stage-grid").length===1,"actual Energy dashboard did not render a single Energy Path graph");
 check(host.querySelectorAll(".energy-path-kpis").length===1,"actual dashboard lost or duplicated its KPI strip");
 check(!host.querySelector(".simulation-energy-subnav,[data-simulation-energy-view],.energy-path-summary-overview,[data-energy-path-summary-group]"),"single Energy dashboard still exposes old subnav or duplicate summary tables");
 check(host.querySelectorAll(".energy-path-controls select").length===3,"default Energy controls are not Scope / Period / Service only");
 check(!host.querySelector("[data-energy-path-inspector-actions]"),"unselected graph rendered unrelated Series/HVAC actions");
 change("[data-simulation-energy-scope]","zone");
 change("[data-simulation-energy-zone-name]","Office");
 change("[data-simulation-energy-path-period]","M1");
 change("[data-simulation-energy-service]","cooling");
 check(state.simulationEnergyScopeKind==="zone"&&state.simulationEnergyZoneName==="Office"&&state.simulationEnergyPeriod==="M1"&&state.simulationEnergyService==="cooling","actual control handlers did not retain selected scope / month / service");
 for(const[selector,label]of[["[data-simulation-energy-scope]","Scope"],["[data-simulation-energy-path-period]","Period"],["[data-simulation-energy-service]","Service"],["[data-simulation-energy-zone-name]","Zone"]]){
  check(host.querySelector(selector)?.getAttribute("aria-label")===label,"selected Zone value contaminated accessible control name: "+label);
 }
 check(!host.querySelector('[data-energy-explanation-node="load.heating.office"]'),"Cooling service retained unrelated heating load");
 select("load.cooling.office");
 check(state.simulationEnergySelection==="load.cooling.office"&&host.querySelector('[data-energy-path-inspector="load.cooling.office"]'),"actual node selection did not open its inspector");
 const seriesAction=host.querySelector("[data-energy-path-series-id]");
 const monthlyID=navigation.energyPathSeriesID(series[1]);
 check(seriesAction?.dataset.energyPathSeriesId===monthlyID&&seriesAction?.dataset.energyPathSeriesPeriod==="M1","inspector guessed first Hourly series instead of exact derived-input Monthly target");
 check(host.querySelectorAll('[data-energy-path-service-kind="hvac"] [data-energy-path-service-destination]').length===1&&serviceButton("path.office.cooling"),"load inspector inferred unrelated HVAC instead of verified exact relatedPathIds");
 host.querySelector("[data-energy-path-details-toggle]")?.click();
 check(state.simulationEnergyDetailsOpen&&!host.querySelector("[data-energy-path-data-details]")?.hidden,"Data details is not available beside a selected node");
 const januaryContext=context();
 let genericRestored=false;
 const historyAdapter={genericCaptureContext:()=>({genericMarker:"retained"}),genericRestoreContext:async snapshot=>{genericRestored=snapshot.genericMarker==="retained";}};
 const navigationSnapshot=simulation.captureSimulationNavigationContext(historyAdapter);
 change("[data-simulation-energy-scope]","building");change("[data-simulation-energy-path-period]","M2");change("[data-simulation-energy-service]","heating");
 const restored=await simulation.restoreSimulationNavigationContext(navigationSnapshot,historyAdapter);
 check(restored&&genericRestored&&context()===januaryContext&&host.querySelector('[data-energy-path-inspector="load.cooling.office"]'),"real navigation-context restore lost scope/month/service/selection/drawer or generic history state");
 host.querySelector('[data-energy-path-output-source="sql-rdd-11"]')?.click();
 const outputContext=context(),outputSnapshot=simulation.captureSimulationNavigationContext(historyAdapter);
 check(state.simulationEnergyDetailsTab==="output"&&state.simulationEnergyOutputSource==="sql-rdd-11","history fixture failed to select exact Output source");
 host.querySelector('[data-energy-path-details-tab="data"]')?.click();
 await simulation.restoreSimulationNavigationContext(outputSnapshot,historyAdapter);
 check(context()===outputContext&&host.querySelector('[data-energy-path-output-request-selected="true"]'),"navigation history did not restore exact Output source and internal tab");
 for(const oldView of["sources","reconciliation"]){
  await simulation.restoreSimulationNavigationContext({...navigationSnapshot,energyScopeKind:undefined,energyZoneName:undefined,energyFocusMode:"zone",energyZoneFocus:"Office",energyDetailsOpen:undefined,energyView:oldView},historyAdapter);
  check(state.simulationEnergyScopeKind==="zone"&&state.simulationEnergyZoneName==="Office"&&state.simulationEnergyDetailsOpen&&state.simulationEnergyDetailsTab==="data"&&!host.querySelector(".simulation-energy-subnav"),"legacy Zone/"+oldView+" history did not migrate to current scope and Data drawer");
 }
 await simulation.restoreSimulationNavigationContext(navigationSnapshot,historyAdapter);
 host.querySelector("[data-energy-path-series-id]")?.click();
 check(state.simulationActiveResultView==="series"&&state.simulationSelectedSeries===monthlyID,"actual Series action did not select the frequency-qualified Monthly series");
 check(state.simulationSeriesRangeStart===0&&state.simulationSeriesRangeEnd===0,"January Series range was not resolved from January point labels");
 check(document.getElementById("simulationChart").querySelector(".simulation-series-viewport-meta")?.textContent.includes("1-1 / 3"),"actual Series panel retained a stale/full range instead of January");
 const januaryPoint=document.getElementById("simulationChart").querySelector("[data-simulation-series-single-point]");
 check(januaryPoint?.tagName.toLowerCase()==="circle"&&Number(januaryPoint.getAttribute("r"))>0&&januaryPoint.hasAttribute("cx")&&januaryPoint.hasAttribute("cy")&&Number.isFinite(Number(januaryPoint.getAttribute("cx")))&&Number.isFinite(Number(januaryPoint.getAttribute("cy"))),"single January observation is not visibly plotted at finite coordinates");
 check(januaryPoint?.nextElementSibling?.tagName.toLowerCase()==="text"&&januaryPoint.nextElementSibling.textContent.includes("100 J"),"single January observation lacks a visible reported-value label with preserved unit casing");
 check(!/NaN|Infinity/.test(document.getElementById("simulationChart").innerHTML),"actual Monthly chart contains non-finite geometry");
 check(context()===januaryContext,"Series jump changed the Energy scope/month/service/selection/drawer context");
 returnEnergy();
 check(state.simulationActiveResultView==="energy"&&context()===januaryContext&&host.querySelector('[data-energy-path-inspector="load.cooling.office"]'),"return to Energy lost its selection or context");
 change("[data-simulation-energy-path-period]","M2");select("load.cooling.office");
 host.querySelector("[data-energy-path-series-id]")?.click();
 check(state.simulationSeriesRangeStart===1&&state.simulationSeriesRangeEnd===1&&document.getElementById("simulationChart").querySelector(".simulation-series-viewport-meta")?.textContent.includes("2-2 / 3"),"February jump did not replace January's panel-local range using actual labels");
 returnEnergy();change("[data-simulation-energy-path-period]","annual");select("load.cooling.office");
 host.querySelector("[data-energy-path-series-id]")?.click();
 check(state.simulationSeriesRangeStart===0&&(state.simulationSeriesRangeEnd===-1||state.simulationSeriesRangeEnd===2)&&document.getElementById("simulationChart").querySelector(".simulation-series-viewport-meta")?.textContent.includes("1-3 / 3"),"Annual jump did not reset the previous month's panel range");
 returnEnergy();
 const hvacContext=context(),historyBefore=state.navigationUndoStack.length;
 await simulation.openSimulationEnergyServiceDestination(serviceButton("path.office.cooling"));
 check(state.activeResultTab==="hvac"&&state.activeHVACContext?.pathId==="path.office.cooling"&&state.globalSelection.entityId==="entity.path.office.cooling","actual global HVAC action did not navigate its exact service path");
 check(state.navigationUndoStack.length===historyBefore+1,"HVAC action did not preserve and extend global navigation history exactly once");
 check(context()===hvacContext,"HVAC jump changed the Energy context");
 await appNavigation.undoViewNavigation();check(state.activeResultTab==="simulation"&&context()===hvacContext,"actual global HVAC Back did not restore Energy context");returnEnergy();
 select("driver.internal.people.cooling.office");
 check(!host.querySelector("[data-energy-path-hvac-actions]"),"driver heat-source inspector exposed HVAC path inference");
 change("[data-simulation-energy-service]","heating");select("load.heating.office");
 const missingInspector=host.querySelector('[data-energy-path-inspector="load.heating.office"]');
 check(!missingInspector?.querySelector('[data-energy-path-series-id]:not([disabled])')&&!missingInspector?.querySelector('[data-energy-path-service-kind="hvac"] [data-energy-path-service-destination]:not([disabled])'),"missing Series/path evidence fabricated an enabled navigation action");
 check(missingInspector?.querySelector('[data-energy-path-series-actions] button[disabled]')&&missingInspector?.querySelector('[data-energy-path-service-kind="hvac"] button[disabled]'),"unavailable Series/HVAC actions are not visibly disabled");
 check(missingInspector?.querySelector('[data-energy-path-series-actions]')?.textContent.trim().length>6&&missingInspector?.querySelector('[data-energy-path-service-kind="hvac"]')?.textContent.trim().length>4,"unavailable navigation omits an honest explanation");
 const renderCase=(payload,observations,period="annual")=>{
  state.simulationResult=freeze({...result,purposeResults:{...result.purposeResults,energyExplanation:payload},series:observations});
  Object.assign(state,{simulationActiveResultView:"energy",simulationEnergyScopeKind:"zone",simulationEnergyZoneName:"Office",simulationEnergyPeriod:period,simulationEnergyService:"cooling",simulationEnergySelection:"load.cooling.office",simulationEnergyDetailsOpen:false});
  simulation.renderSimulationEnergyDashboard(state.simulationResult);
 };
 renderCase(explanation,[series[1],{...series[1]}]);
 check(!host.querySelector('[data-energy-path-series-id]:not([disabled])')&&host.querySelector('[data-energy-path-series-actions] button[disabled]'),"identical Series identities silently selected the first duplicate");
 const metadataFree={file:series[1].file,column:series[1].column,points:series[1].points};
 renderCase(explanation,[metadataFree]);
 check(!host.querySelector('[data-energy-path-series-id]:not([disabled])'),"old Series without frequency/source identity received an exact jump");
 renderCase(explanation,[{...series[1],points:[{x:1,value:100,label:"Warmup 1"},{x:2,value:200,label:"Warmup 2"}]}],"M1");
 check(!host.querySelector('[data-energy-path-series-id]:not([disabled])'),"unknown timestamps used x/index as a January range");
 const multiple=structuredClone(explanation);
 multiple.zoneResults[0].nodes.find(node=>node.id==="load.cooling.office").sourceIds.push("sql-rdd-31");
 multiple.zoneResults[0].nodes.find(node=>node.id==="load.cooling.office").relatedPathIds.push("path.office.cooling.two");
 state.report={hvac:{...report.hvac,serviceModel:{...report.hvac.serviceModel,zoneServices:[{zoneName:"Office",paths:[...report.hvac.serviceModel.zoneServices[0].paths,{id:"path.office.cooling.two",serviceKind:"cooling",zoneName:"Office",servedSubject:{kind:"zone",zoneName:"Office",name:"Office secondary"}}]}]}}};
 state.semanticProjection={navigation:{entities:[pathEntity("path.office.cooling"),pathEntity("path.office.cooling.two")]}};
 const meterSeries={file:"eplusout.sql",column:"Cooling:Electricity [J]",sourceId:"sql-rdd-31",name:"Cooling:Electricity",keyValue:"",isMeter:true,reportingFrequency:"Monthly",points:[{x:0,value:25,label:"01-31 24:00"}]};
 renderCase(multiple,[...series,meterSeries]);
 for(const kind of["series","hvac"]){
  const chooser=host.querySelector(kind==="hvac"?'[data-energy-path-service-chooser="hvac"]':'[data-energy-path-action-chooser="series"]');
  check(chooser?.tagName==="DETAILS"&&!chooser.open,"multiple exact "+kind+" targets are not an explicit closed chooser");
  check(chooser?.querySelectorAll("button").length===2,"multiple exact "+kind+" choices were lost or guessed");
  chooser?.querySelector("summary")?.click();check(chooser?.open,"native "+kind+" target chooser cannot expand");
 }
 check(state.simulationActiveResultView==="energy","merely rendering/expanding candidates navigated to a guessed target");
 const chosen=host.querySelector('[data-energy-path-series-id="'+navigation.energyPathSeriesID(meterSeries)+'"]');
 chosen?.click();
 check(state.simulationSelectedSeries===navigation.energyPathSeriesID(meterSeries),"explicit second Series choice did not navigate to the selected target");
 const oldSeriesID=series[1].file+"::"+series[1].column;
 state.simulationResult=freeze({...result,series:[series[1]]});state.simulationSelectedSeries=oldSeriesID;
 document.querySelector('[data-simulation-result-view-button="series"]')?.click();
 check(state.simulationSelectedSeries===monthlyID&&document.getElementById("simulationChart").querySelector("select[data-series-variable-index='0']")?.value===monthlyID,"unique historical file::column Series selection did not migrate to exact provenance identity");
 state.simulationResult=result;state.simulationSelectedSeries=oldSeriesID;
 document.querySelector('[data-simulation-result-view-button="series"]')?.click();
 const unresolvedPicker=document.getElementById("simulationChart").querySelector("select[data-series-variable-index='0']");
 check(state.simulationSelectedSeries===oldSeriesID&&unresolvedPicker?.value===""&&unresolvedPicker?.selectedOptions[0]?.disabled,"ambiguous historical Series alias silently chose the first frequency");
 check(unresolvedPicker?.selectedOptions[0]?.textContent.includes("Choose source series"),"ambiguous historical Series selection lacks an explicit unresolved picker prompt");
 check(!document.getElementById("simulationChart").querySelector("polyline,[data-simulation-series-single-point]"),"ambiguous historical Series alias plotted a guessed first observation");
 const legacyPayload=freeze({schema:"semantic-idf.energy-explanation/v1",nodes:[{id:"heat.people.office",level:"heat",value:12,unit:"kWh",sourceIds:["legacy.source"]}],sources:[{id:"legacy.source",name:"Legacy people energy"}]});
 const legacyJSON=JSON.stringify(legacyPayload);
 state.simulationResult=freeze({...result,purposeResults:{energyExplanation:legacyPayload}});
 simulation.renderSimulationEnergyDashboard(state.simulationResult);
 check(!host.querySelector(".simulation-energy-subnav,[data-simulation-energy-view]")&&host.textContent.includes("Series")&&host.textContent.toLowerCase().includes("unavailable"),"unupgraded legacy payload resurrected old subnav or omitted honest availability guidance");
 check(JSON.stringify(legacyPayload)===legacyJSON,"legacy compatibility guidance rewrote stored V1 data");
 check(JSON.stringify(result)===originalJSON,"Energy controls, inspectors or navigation changed raw result/export inputs");
 if(failures.length)throw new Error(failures.join(" | "));
 document.body.dataset.epath140Status="passed";document.getElementById("result").textContent="passed";
 }
}catch(error){document.body.dataset.epath140Status="failed";document.getElementById("result").textContent=String(error?.stack||error);}
</script></body></html>`
