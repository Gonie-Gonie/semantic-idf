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
<body data-epath140-status="pending"><div id="runtimeStatus"></div><div id="simulationPane" class="result-pane active">
<div id="simulationResultTabs"><button data-simulation-result-view-button="energy">Energy</button></div><div id="simulationOutputMessage"></div>
<section data-simulation-result-view="energy"><div id="simulationEnergyStats"></div><div id="simulationEnergyDashboard"></div></section>
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
const result=freeze({status:"succeeded",purposeResults:{energyExplanation:explanation,energyExplanationSummary:building.summary},purposeRunPlan:{outputObjects:[{objectType:"Output:Variable",keyValue:"Office",variableName:sources[0].name,reportingFrequency:"Monthly",purposeIds:['basic_energy']},{objectType:"Output:Variable",keyValue:"Office",variableName:sources[1].name,reportingFrequency:"Hourly",purposeIds:['basic_energy']}]},series});
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
 Object.assign(state,{report,simulationResult:result,simulationActiveResultView:"energy",activeResultTab:"simulation"});
 simulation.restoreSimulationEnergyWorkspaceContext({simulationEnergyScopeKind:"building",simulationEnergyZoneName:"",simulationEnergyPeriod:"annual",simulationEnergyService:"all",simulationEnergySelection:"",simulationEnergyDetailsOpen:false,energyDrawer:{tab:"data",stage:"",outputSource:""}});
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
 const drawer=()=>simulation.captureSimulationEnergyWorkspaceContext().energyDrawer;
 const context=()=>JSON.stringify([state.simulationEnergyScopeKind,state.simulationEnergyZoneName,state.simulationEnergyPeriod,state.simulationEnergyService,state.simulationEnergySelection,state.simulationEnergyDetailsOpen,drawer().tab,drawer().stage,drawer().outputSource]);
 const change=(selector,value)=>{const control=host.querySelector(selector);check(Boolean(control),"missing real Energy control "+selector);if(control){control.value=value;control.dispatchEvent(new Event("change",{bubbles:true}));}};
 const select=id=>{const node=host.querySelector('[data-energy-explanation-node="'+id+'"]');check(Boolean(node),"missing selected graph node "+id);node?.focus();node?.click();check(document.activeElement?.dataset.energyExplanationNode===id,"node activation discarded keyboard focus: "+id);};
 const returnEnergy=()=>document.querySelector('[data-simulation-result-view-button="energy"]')?.click();
 if(new URLSearchParams(location.search).get("manual")==="1"){
  document.body.dataset.epath140Status="manual";document.getElementById("result").textContent="Manual EPATH-140 fixture: use scope and month, then select a component to show its Monthly or Hourly chart.";
 }else{
 check(host.querySelectorAll(".energy-path-view").length===1&&host.querySelectorAll(".energy-path-stage-grid").length===1,"actual Energy dashboard did not render a single Energy Path graph");
 check(!host.querySelector(".energy-path-kpis,[data-energy-path-kpi],[data-simulation-energy-service]"),"actual dashboard retains removed summary cards or Service dropdown");
 check(!host.querySelector(".simulation-energy-subnav,[data-simulation-energy-view],.energy-path-summary-overview,[data-energy-path-summary-group]"),"single Energy dashboard still exposes old subnav or duplicate summary tables");
 check(host.querySelectorAll(".energy-path-controls select").length===2,"default Energy controls are not Scope / Period only");
 check(!host.querySelector("[data-energy-path-inspector-actions]"),"unselected graph rendered unrelated Series/HVAC actions");
 change("[data-simulation-energy-scope]","zone:Office");
 change("[data-simulation-energy-path-period]","M1");
 check(state.simulationEnergyScopeKind==="zone"&&state.simulationEnergyZoneName==="Office"&&state.simulationEnergyPeriod==="M1"&&state.simulationEnergyService==="all","actual control handlers did not retain selected scope / month / service");
 check(!host.querySelector("[data-simulation-energy-zone-name],#simulationEnergyPathZones"),"single-step Scope retained a separate Zone control");
 for(const[selector,label]of[["[data-simulation-energy-scope]","Scope"],["[data-simulation-energy-path-period]","Period"]]){
  check(host.querySelector(selector)?.getAttribute("aria-label")===label,"selected Zone value contaminated accessible control name: "+label);
 }
 check(host.querySelector('[data-energy-explanation-node="load.heating.office"]'),"graph omitted heating after Service dropdown removal");
 select("load.cooling.office");
 check(state.simulationEnergySelection==="load.cooling.office"&&host.querySelector('[data-energy-path-inspector="load.cooling.office"]'),"actual node selection did not open its inspector");
 const monthlyID=navigation.energyPathSeriesID(series[1]);
 host.querySelector("[data-energy-path-details-toggle]")?.click();
 check(state.simulationEnergyDetailsOpen&&!host.querySelector("[data-energy-path-data-details]")?.hidden,"Data details is not available beside a selected node");
 const januaryContext=context();
 let genericRestored=false;
 const historyAdapter={genericCaptureContext:()=>({genericMarker:"retained"}),genericRestoreContext:async snapshot=>{genericRestored=snapshot.genericMarker==="retained";}};
 const navigationSnapshot=simulation.captureSimulationNavigationContext(historyAdapter);
 change("[data-simulation-energy-scope]","building");change("[data-simulation-energy-path-period]","M2");
 const restored=await simulation.restoreSimulationNavigationContext(navigationSnapshot,historyAdapter);
 check(restored&&genericRestored&&context()===januaryContext&&host.querySelector('[data-energy-path-inspector="load.cooling.office"]'),"real navigation-context restore lost scope/month/service/selection/drawer or generic history state");
 host.querySelector('[data-energy-path-output-source="sql-rdd-11"]')?.click();
 const outputContext=context(),outputSnapshot=simulation.captureSimulationNavigationContext(historyAdapter);
 check(drawer().tab==="output"&&drawer().outputSource==="sql-rdd-11","history fixture failed to select exact Output source");
 host.querySelector('[data-energy-path-details-tab="data"]')?.click();
 await simulation.restoreSimulationNavigationContext(outputSnapshot,historyAdapter);
 check(context()===outputContext&&host.querySelector('[data-energy-path-output-request-selected="true"]'),"navigation history did not restore exact Output source and internal tab");
 for(const oldView of["sources","reconciliation"]){
  await simulation.restoreSimulationNavigationContext({...navigationSnapshot,energyScopeKind:undefined,energyZoneName:undefined,energyFocusMode:"zone",energyZoneFocus:"Office",energyDetailsOpen:undefined,energyView:oldView},historyAdapter);
  check(state.simulationEnergyScopeKind==="zone"&&state.simulationEnergyZoneName==="Office"&&state.simulationEnergyDetailsOpen&&drawer().tab==="data"&&!host.querySelector(".simulation-energy-subnav"),"legacy Zone/"+oldView+" history did not migrate to current scope and Data drawer");
 }
 await simulation.restoreSimulationNavigationContext(navigationSnapshot,historyAdapter);
 select(host.querySelector('[data-energy-path-stage="driver"] [data-energy-path-layout-node]').dataset.energyPathLayoutNode);
 check(!host.querySelector("[data-energy-path-inspector-actions],[data-energy-path-service-destination],[data-energy-path-series-actions]"),"removed component detail actions remain visible");
 select("load.heating.office");
 check(host.querySelector('[data-energy-path-inspector="load.heating.office"] [data-energy-path-chart-frequency]'),"heating component selection lost its chart frequency control");
 const {prepareSimulationComparison}=await import('/src/js/simulation-comparison-data.js');
 const comparisonHost=()=>host.querySelector('[data-simulation-comparison-host="energy"]');
 const selectedProperty=()=>{const model=prepareSimulationComparison(state.simulationResult,'energy',state),row=state.simulationComparisons?.energy?.rows?.[0];return model.entities.find(entity=>entity.id===row?.entity)?.properties.find(property=>property.id===row?.property);};
 const inspect=async id=>{const button=document.createElement('button');button.dataset.simulationInspectSeries='1';button.dataset.simulationSeriesId=id;host.append(button);simulation.handleSimulationSeriesInspectClick({target:button,preventDefault(){},stopPropagation(){}});await new Promise(resolve=>setTimeout(resolve,0));};
 check(comparisonHost()===host.lastElementChild&&comparisonHost()?.querySelector('[data-simulation-comparison]'),'Energy comparison is not mounted after the existing diagram and inspector');
 await inspect(monthlyID);
 check(state.simulationActiveResultView==='energy'&&[monthlyID,sources[0].id].includes(selectedProperty()?.seriesId),'source inspection left Energy or selected another reporting frequency');
 check(comparisonHost()?.querySelector('[data-simulation-comparison-property]')?.value===state.simulationComparisons.energy.rows[0].property,'exact source jump did not select the bottom comparison property control');
 const comparisonSnapshot=simulation.captureSimulationNavigationContext(historyAdapter),monthlyRows=JSON.stringify(state.simulationComparisons.energy.rows);
 const hourlyID=navigation.energyPathSeriesID(series[0]);await inspect(hourlyID);
 check([hourlyID,sources[1].id].includes(selectedProperty()?.seriesId)&&JSON.stringify(state.simulationComparisons.energy.rows)!==monthlyRows,'same file/key/name Hourly source was collapsed into Monthly data');
 await simulation.restoreSimulationNavigationContext(comparisonSnapshot,historyAdapter);
 check(JSON.stringify(state.simulationComparisons.energy.rows)===monthlyRows&&[monthlyID,sources[0].id].includes(selectedProperty()?.seriesId),'navigation history did not restore the exact purpose-local comparison source');
 const beforeAmbiguous=JSON.stringify(state.simulationComparisons.energy.rows);await inspect(series[1].file+'::'+series[1].column);
 check(JSON.stringify(state.simulationComparisons.energy.rows)===beforeAmbiguous,'ambiguous file/column source jump silently chose the first reporting frequency');
 check(!document.querySelector('[data-simulation-result-view-button="series"],[data-simulation-result-view="series"]'),'source inspection restored the removed standalone Series view');
 const legacyPayload=freeze({schema:"semantic-idf.energy-explanation/v1",nodes:[{id:"heat.people.office",level:"heat",value:12,unit:"kWh",sourceIds:["legacy.source"]}],sources:[{id:"legacy.source",name:"Legacy people energy"}]});
 const legacyJSON=JSON.stringify(legacyPayload);
 state.simulationResult=freeze({...result,purposeResults:{energyExplanation:legacyPayload}});
 simulation.renderSimulationEnergyDashboard(state.simulationResult);
 check(!host.querySelector(".simulation-energy-subnav,[data-simulation-energy-view]")&&comparisonHost()===host.lastElementChild&&host.textContent.toLowerCase().includes("unavailable"),"unupgraded legacy payload resurrected old subnav or lost its bottom comparison and availability guidance");
 check(JSON.stringify(legacyPayload)===legacyJSON,"legacy compatibility guidance rewrote stored V1 data");
 check(JSON.stringify(result)===originalJSON,"Energy controls, inspectors or navigation changed raw result/export inputs");
 if(failures.length)throw new Error(failures.join(" | "));
 document.body.dataset.epath140Status="passed";document.getElementById("result").textContent="passed";
 }
}catch(error){document.body.dataset.epath140Status="failed";document.getElementById("result").textContent=String(error?.stack||error);}
</script></body></html>`
