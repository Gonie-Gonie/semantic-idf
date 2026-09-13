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

func TestEPATH141ActualDashboardWithoutSummaryCards(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser summary-card removal acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath141", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath141KPIHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--window-size=1200,900", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath141").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH141 browser failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, "data-epath141-status=\"passed\"") {
		if start := strings.Index(document, "<pre id=\"result\">"); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH141 failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH141 failed:\n%s", output)
	}
	if start := strings.Index(document, "<pre id=\"result\">"); start >= 0 {
		if end := strings.Index(document[start:], "</pre>"); end >= 0 {
			t.Logf("Energy graph without summary cards: %s", document[start+len("<pre id=\"result\">"):start+end])
		}
	}
}

const epath141KPIHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>EPATH141 Energy graph without duplicate summary cards</title>
<link rel="stylesheet" href="/src/styles/base.css"><link rel="stylesheet" href="/src/styles/simulation.css"><style>body{margin:0;overflow:auto}#simulationPane{width:1100px;max-width:100%}button{font:inherit}</style>
<script>window.go={main:{App:{GetSimulationEnvironment:async()=>({installations:[],weatherFolders:[]})}}};window.runtime={EventsOn(){}};</script></head>
<body data-epath141-status="pending"><div id="runtimeStatus"></div><div id="simulationPane"><div id="simulationResultTabs"><button data-simulation-result-view-button="energy">Energy</button></div><section data-simulation-result-view="energy"><div id="simulationEnergyStats"></div><div id="simulationEnergyDashboard"></div></section></div><pre id="result">pending</pre>
<script type="module">
const failures=[],check=(condition,message)=>{if(!condition)failures.push(message);};
const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
const stage={status:"complete",found:1,total:1};
const quality={drivers:stage,loads:stage,endUses:stage,carriers:stage,ratios:stage,driverToLoadStatus:"partial",driverToLoadClosedPct:50,endUseToCarrierStatus:"partial",endUseToCarrierClosedPct:95,zoneAllocationStatus:"unavailable",zoneAllocatedPct:0,unassignedPct:0};
const graph=(scope,period,scale=1,full=false)=>{
 const suffix=scope.kind==="zone"?"office":"building";
 const cooling=100*scale,heating=80*scale,siteCooling=(full?25:20)*scale,siteHeating=100*scale;
 const node=(id,level,value,extra)=>({id:id+"."+suffix,level,value,unit:"kWh",period,aggregationBasis:"model_total",...(scope.kind==="zone"?{zoneName:"Office"}:{}),...extra});
 const nodes=[
  node("driver.internal.people.cooling","driver",cooling,{driverCategory:"internal.people",serviceKind:"cooling",scaleDomain:"thermal",basis:"heat_balance_share",allocatedValue:cooling,allocationApplied:true,sourceIds:["sql.people"]}),
  node("load.cooling","load",cooling,{serviceKind:"cooling",scaleDomain:"thermal",sourceIds:["sql.cooling"]}),
  node("load.heating","load",heating,{serviceKind:"heating",scaleDomain:"thermal",sourceIds:["sql.heating"]}),
  node("end_use.cooling","end_use",siteCooling,{endUse:"cooling",serviceKind:"cooling",scaleDomain:"site",basis:"service_path_allocation",sourceIds:["sql.electricity"]}),
  node("end_use.heating","end_use",siteHeating,{endUse:"heating",serviceKind:"heating",scaleDomain:"site",basis:"service_path_allocation",sourceIds:["sql.gas"]}),
  node("carrier.electricity","carrier",siteCooling,{carrier:"electricity",scaleDomain:"site",sourceIds:["sql.electricity"]}),
  node("carrier.natural_gas","carrier",siteHeating,{carrier:"natural_gas",scaleDomain:"site",sourceIds:["sql.gas"]}),
 ];
 const link=(id,from,to,fromValue,toValue,relation,serviceKind,extra={})=>({id:id+"."+suffix,fromId:from+"."+suffix,toId:to+"."+suffix,fromValue,toValue,fromUnit:"kWh",toUnit:"kWh",relation,serviceKind,period,...extra});
 const links=[
  link("driver.cooling","driver.internal.people.cooling","load.cooling",cooling,cooling,"driver_to_load","cooling",{sourceIds:["sql.people","sql.cooling"]}),
  link("conversion.cooling","load.cooling","end_use.cooling",(full?100:40)*scale,siteCooling,"load_to_end_use","cooling",{ratio:full?4:2,ratioKind:"coefficient_of_performance",sourceIds:["sql.cooling","sql.electricity"]}),
  link("conversion.heating","load.heating","end_use.heating",heating,siteHeating,"load_to_end_use","heating",{ratio:.8,ratioKind:"efficiency",sourceIds:["sql.heating","sql.gas"]}),
  link("carrier.cooling","end_use.cooling","carrier.electricity",siteCooling,siteCooling,"end_use_to_carrier","cooling",{sourceIds:["sql.electricity"]}),
  link("carrier.heating","end_use.heating","carrier.natural_gas",siteHeating,siteHeating,"end_use_to_carrier","heating",{sourceIds:["sql.gas"]}),
 ];
 const completeness={mappedPercent:99,energyUse:{level:"energy",status:scope.kind==="zone"?"partial":"complete",found:1,total:1}};
 const localQuality=scope.kind==="zone"?{...quality,endUseToCarrierStatus:"unavailable",endUseToCarrierClosedPct:99}:quality;
 const summary={schema:"semantic-idf.energy-explanation-summary/v2",scope,period,quality:localQuality,completeness,drivers:nodes.filter(n=>n.level==="driver"),loads:nodes.filter(n=>n.level==="load"),endUses:nodes.filter(n=>n.level==="end_use"),carriers:nodes.filter(n=>n.level==="carrier"),ratios:[{id:"stale-ratio",value:88}]};
 return {id:period,nodes,links,quality:localQuality,completeness,summary};
};
// Unit-area run snapshots preserve this navigation fixture's numeric expectations.
const buildingScope={kind:"building",aggregationBasis:"model_total",floorAreaM2:1},zoneScope={kind:"zone",zoneName:"Office",aggregationBasis:"model_total",floorAreaM2:1};
const annual=graph(buildingScope,"annual",10,true),january=graph(buildingScope,"M1"),february=graph(buildingScope,"M2",2);
february.links=february.links.filter(link=>link.relation!=="load_to_end_use");
const sources=["people","cooling","heating","electricity","gas"].map(name=>({id:"sql."+name,name:"SOURCE_SENTINEL "+name,sourceType:"sql_variable",sourceUnit:"J",normalizedUnit:"kWh",reportingFrequency:"Monthly"}));
const explanation=freeze({schema:"semantic-idf.energy-explanation/v2",scope:buildingScope,...annual,sources,periods:[january,february],zoneResults:[{scope:zoneScope,...graph(zoneScope,"annual",5,true),periods:[graph(zoneScope,"M1",.5)]}]});
const result=freeze({status:"succeeded",purposeResults:{energyExplanation:explanation,energyExplanationSummary:annual.summary},purposeRunPlan:{outputObjects:[]}}),originalJSON=JSON.stringify(result);
try{
 const[{state},simulation]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js")]);
 simulation.initializeSimulationControls();await new Promise(resolve=>setTimeout(resolve,0));
 const host=document.getElementById("simulationEnergyDashboard");
 const drawer=()=>simulation.captureSimulationEnergyWorkspaceContext().energyDrawer;
 const mount=(payload=result,extra={})=>{Object.assign(state,{simulationResult:payload,simulationActiveResultView:"energy"});simulation.restoreSimulationEnergyWorkspaceContext({simulationEnergyScopeKind:"building",simulationEnergyZoneName:"",simulationEnergyPeriod:"M1",simulationEnergyService:"all",simulationEnergySelection:"",simulationEnergyDetailsOpen:false,...extra,energyDrawer:{tab:"data",stage:"",outputSource:"",...extra.energyDrawer}});simulation.renderSimulationEnergyDashboard(payload);};
 const node=id=>host.querySelector('[data-energy-path-layout-node="'+id+'"]');
 const change=(selector,value)=>{const control=host.querySelector(selector);check(Boolean(control),"missing control "+selector);if(control){control.focus();control.value=value;control.dispatchEvent(new Event("change",{bubbles:true}));}};
 const activate=button=>{check(button?.tagName==="BUTTON"&&!button.disabled,"graph/detail target is not a native enabled button");button?.focus();button?.click();};
 const valueText=id=>node(id)?.querySelector("strong")?.textContent.trim()||"";
 const noCards=()=>check(!host.querySelector(".energy-path-kpis,[data-energy-path-kpi]"),"redundant summary cards remain in the actual Energy dashboard");
 mount();
 if(new URLSearchParams(location.search).get("manual")==="1"){
  document.body.dataset.epath141Status="manual";document.getElementById("result").textContent="Manual EPATH-141 fixture: summary cards and Service dropdown are absent. January graph displays a 100.00 kWh/m² cooling load and matched conversion 40/20=2; graph nodes and Data details remain available.";
 }else{
 noCards();
 check(host.querySelectorAll("[data-energy-path-stage]").length===4,"summary removal lost the four-stage energy graph");
 check(valueText("load.cooling.building")==="100.00"&&valueText("load.heating.building")==="80.00","January graph lost selected-period load values and two-decimal precision");
 check(node("load.cooling.building")?.title.includes("100.00 kWh/m²"),"load graph lost accessible area units");
 const qualityLine=host.querySelector("[data-energy-path-quality-line]");
 check(qualityLine?.querySelectorAll("[data-energy-path-quality-stage]").length===4,"graph quality stages disappeared with summary cards");
 check(Boolean(host.querySelector(".energy-path-stage-grid")?.compareDocumentPosition(qualityLine)&Node.DOCUMENT_POSITION_FOLLOWING),"quality line is not below the graph");
 check(!host.querySelector("[data-simulation-energy-service]"),"removed Service dropdown remains visible");
 activate(node("carrier.natural_gas.building"));
 check(state.simulationEnergySelection==="carrier.natural_gas.building","gas remains inaccessible from its graph node after chooser removal");
 check(document.activeElement?.dataset.energyExplanationNode==="carrier.natural_gas.building","graph selection lost native node focus");
 activate(node("load.cooling.building"));
 check(state.simulationEnergyService==="all"&&state.simulationEnergySelection==="load.cooling.building","load graph selection changed the user's service context");
 const bridge=host.querySelector('[data-energy-path-bridge-ratio][data-energy-path-ratio-value="2"]');
 check(Boolean(bridge),"graph lost the matched40/20 conversion ratio with summary cards");
 activate(host.querySelector("[data-energy-path-details-toggle]"));
 check(state.simulationEnergyDetailsOpen&&drawer().tab==="data"&&drawer().stage===""&&state.simulationEnergySelection==="load.cooling.building","Data details toggle lost drawer access or graph selection");
 document.activeElement?.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true}));
 check(!state.simulationEnergyDetailsOpen&&document.activeElement?.hasAttribute("data-energy-path-details-toggle"),"Escape did not restore the Data details opener");
 change("[data-simulation-energy-path-period]","M2");noCards();
 check(valueText("load.cooling.building")==="200.00","February graph reused annual cooling or lost precision");
 check(!host.querySelector("[data-energy-path-bridge-ratio][data-energy-path-ratio-value]"),"month without conversion links reused a stale ratio");
 change("[data-simulation-energy-path-period]","M3");noCards();
 check(!host.querySelector("[data-energy-path-layout-node]"),"absent month reused another period's graph nodes");
 mount(result,{simulationEnergyScopeKind:"zone",simulationEnergyZoneName:"Office"});noCards();
 check(valueText("load.cooling.office")==="50.00"&&valueText("load.heating.office")==="40.00","Zone graph lost selected-period thermal values");
 activate(node("load.cooling.office"));
 check(state.simulationEnergySelection==="load.cooling.office","Zone graph selection failed");
 check(host.querySelector("[data-energy-path-quality-line]")?.textContent.includes("Office"),"Zone quality context disappeared with summary cards");
 const emptyScope={kind:"building",aggregationBasis:"model_total",floorAreaM2:1};
 const zeroSummary={schema:"semantic-idf.energy-explanation-summary/v2",scope:emptyScope,period:"annual",loads:[{id:"load.cooling.zero",serviceKind:"cooling",value:0,unit:"kWh"}],carriers:[],completeness:{mappedPercent:100}};
 const zeroPayload=freeze({schema:explanation.schema,scope:emptyScope,nodes:[],links:[],sources:[],summary:zeroSummary,quality:{...quality,driverToLoadStatus:"not_requested",endUseToCarrierStatus:"unavailable"}});
 mount(freeze({purposeResults:{energyExplanation:zeroPayload,energyExplanationSummary:zeroSummary}}),{simulationEnergyPeriod:"annual"});noCards();
 check(!host.querySelector("[data-energy-path-layout-node]")&&host.querySelector("[data-energy-path-quality-line]"),"empty graph invented nodes or discarded available quality details");
 check(JSON.stringify(result)===originalJSON,"scope/period/service, graph or drawer interactions mutated raw/export inputs");
 if(failures.length)throw new Error(failures.join(" | "));
 document.body.dataset.epath141Status="passed";document.getElementById("result").textContent=JSON.stringify({passed:true,summaryCards:0,graphSelection:true,qualityDetails:true});
 }
}catch(error){document.body.dataset.epath141Status="failed";document.getElementById("result").textContent=String(error?.stack||error);}
</script></body></html>`
