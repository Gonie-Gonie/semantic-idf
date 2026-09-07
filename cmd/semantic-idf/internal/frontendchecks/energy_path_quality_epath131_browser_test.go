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

func TestEPATH131QualityDrawerAndExactOutputNavigationBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser quality drawer verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath131", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath131QualityDrawerHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--window-size=1200,900", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath131").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH131 browser failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, "data-epath131-status=\"passed\"") {
		if start := strings.Index(document, "<pre id=\"result\">"); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH131 failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH131 failed:\n%s", output)
	}
}

const epath131QualityDrawerHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>EPATH131 quality and output drawer</title>
<link rel="stylesheet" href="/src/styles/base.css"><link rel="stylesheet" href="/src/styles/simulation.css"><style>body{margin:0;font:14px sans-serif;overflow:auto}#simulationEnergyDashboard{width:1100px;max-width:100%;box-sizing:border-box}button{font:inherit}</style></head>
<body data-epath131-status="pending"><div id="runtimeStatus"></div><div id="simulationEnergyStats"></div><div id="simulationEnergyDashboard"></div><pre id="result">pending</pre>
<script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const visible=el=>Boolean(el&&el.getClientRects().length&&getComputedStyle(el).display!=="none"&&getComputedStyle(el).visibility!=="hidden");
const requestObjects=[
 {objectType:"Output:Meter",keyValue:"Cooling:Electricity",reportingFrequency:"Monthly",objectIndex:7},
 {objectType:"Output:Meter",keyValue:"Cooling:Electricity",reportingFrequency:"Hourly",objectIndex:8},
 {objectType:"Output:Variable",keyValue:"Office",variableName:"Zone People Convective Heating Energy",reportingFrequency:"Monthly",objectIndex:9},
 {objectType:"Output:Variable",keyValue:"*",variableName:"Zone People Convective Heating Energy",reportingFrequency:"Monthly",objectIndex:10},
 {objectType:"Output:Variable",keyValue:"Office",variableName:"Zone People Convective Heating Energy",reportingFrequency:"Hourly",objectIndex:11},
];
const sources=[
 {id:"meter.monthly",sourceType:"sql_meter",isMeter:true,name:"Electricity:Cooling",reportingFrequency:"Monthly",sourceUnit:"J",normalizedUnit:"kWh",objectIndex:7},
 {id:"people.office",sourceType:"sql_variable",isMeter:false,name:"Zone People Convective Heating Energy",keyValue:"Office",zoneName:"Office",reportingFrequency:"Monthly",sourceUnit:"J",normalizedUnit:"kWh",objectIndex:9},
 {id:"people.wildcard",sourceType:"sql_variable",isMeter:false,name:"Zone People Convective Heating Energy",keyValue:"Lab",zoneName:"Lab",reportingFrequency:"Monthly",sourceUnit:"J",normalizedUnit:"kWh"},
 {id:"people.stale",sourceType:"sql_variable",isMeter:false,name:"Zone People Convective Heating Energy",keyValue:"Office",zoneName:"Office",reportingFrequency:"Monthly",sourceUnit:"J",normalizedUnit:"kWh",objectIndex:11},
 {id:"people.no_frequency",sourceType:"sql_variable",isMeter:false,name:"Zone People Convective Heating Energy",keyValue:"Office",sourceUnit:"J",normalizedUnit:"kWh"},
 {id:"derived.people",sourceType:"derived_heat_balance",name:"Allocated people",inputSourceIds:["people.office"],formula:"load * pressure / sum(pressure)",normalizedUnit:"kWh"},
 {id:"table.total",sourceType:"sql_tabular",name:"Electricity",tableName:"End Uses",columnName:"Electricity",normalizedUnit:"kWh"},
];
const level=(status,found=0,total=0)=>({status,found,total});
const quality={drivers:level("partial",1,2),loads:level("complete",1,1),endUses:level("complete",1,1),carriers:level("complete",1,1),ratios:level("complete",1,1),driverToLoadClosedPct:50,endUseToCarrierClosedPct:100,driverToLoadStatus:"partial",endUseToCarrierStatus:"complete",zoneAllocatedPct:0,unassignedPct:0,zoneAllocationStatus:"unavailable"};
const explanation={
 schema:"semantic-idf.energy-explanation/v2",scope:{kind:"building",aggregationBasis:"model_total"},quality,
 nodes:[
  {id:"driver.internal.people.cooling.building",level:"driver",driverCategory:"internal.people",serviceKind:"cooling",value:50,allocatedValue:50,allocationApplied:true,unit:"kWh",scaleDomain:"thermal",sourceIds:["people.office"]},
  {id:"load.cooling.building",level:"load",serviceKind:"cooling",value:100,unit:"kWh",scaleDomain:"thermal",sourceIds:["people.office"]},
  {id:"end_use.cooling.building",level:"end_use",endUse:"cooling",value:25,unit:"kWh",scaleDomain:"site",sourceIds:["meter.monthly"]},
  {id:"carrier.electricity.building",level:"carrier",carrier:"electricity",value:25,unit:"kWh",scaleDomain:"site"},
 ],
 links:[
  {id:"driver",fromId:"driver.internal.people.cooling.building",toId:"load.cooling.building",relation:"driver_to_load",serviceKind:"cooling",fromValue:50,toValue:50,fromUnit:"kWh",toUnit:"kWh",sourceIds:["people.office"]},
  {id:"conversion",fromId:"load.cooling.building",toId:"end_use.cooling.building",relation:"load_to_end_use",serviceKind:"cooling",fromValue:100,toValue:25,fromUnit:"kWh",toUnit:"kWh",ratio:4,ratioKind:"coefficient_of_performance",sourceIds:["people.office","meter.monthly"]},
  {id:"carrier",fromId:"end_use.cooling.building",toId:"carrier.electricity.building",relation:"end_use_to_carrier",fromValue:25,toValue:25,fromUnit:"kWh",toUnit:"kWh",sourceIds:["meter.monthly"]},
 ],sources,
 completeness:{mappedPercent:100,sourceAvailability:[
  {name:"Zone People Convective Heating Energy",level:"heat",status:"found",sourceIds:["people.office"]},
  {name:"Zone Lights Convective Heating Energy",level:"heat",status:"missing"},
  {name:"Zone Air System Sensible Cooling Energy",level:"load",status:"found",sourceIds:["people.office"]},
  {name:"Cooling:Electricity",level:"energy",status:"found",sourceIds:["meter.monthly"]},
  {name:"Electricity:Facility",level:"energy",status:"found"},
 ]},
 reconciliation:[{id:"reconcile.energy.electricity.annual",level:"energy",period:"annual",expectedValue:25,explainedValue:25,residualValue:0,unit:"kWh"}],
};
try{
 const [view,{state,elements},simulation,resolver]=await Promise.all([
  import("/src/js/views/energy-path-view.js"),import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/energy-path-output-requests.js")
 ]);
 const host=document.getElementById("simulationEnergyDashboard");
 const defaults={simulationEnergyScopeKind:"building",simulationEnergyPeriod:"annual",simulationEnergyService:"all",simulationEnergySelection:"",simulationEnergyDetailsOpen:false,simulationEnergyDetailsTab:"data",simulationEnergyDetailsStage:"",simulationEnergyOutputSource:""};
 const mount=(payload=explanation,objects=requestObjects,extra={})=>{
  Object.assign(state,defaults,extra,{simulationResult:{purposeResults:{energyExplanation:payload},purposeRunPlan:{outputObjects:objects}}});
  host.innerHTML=view.renderEnergyPathView(payload,state,{outputObjects:objects});
 };
 host.addEventListener("click",simulation.handleSimulationSeriesInspectClick);
 host.addEventListener("keydown",simulation.handleSimulationEnergyDetailsKeydown);
 mount();
 if(new URLSearchParams(location.search).get("manual")==="1"){
  document.body.dataset.epath131Status="manual";
  document.getElementById("result").textContent="Manual EPATH-131 fixture: click a quality stage or Data details. The Output drawer uses this run's Monthly and Hourly requests.";
 }else{
 check(elements.simulationEnergyDashboard===host,"actual global handler did not bind the mounted dashboard fixture");
 const line=host.querySelector("[data-energy-path-quality-line]");
 check(line&&line.querySelectorAll("[data-energy-path-quality-stage]").length===4,"quality line does not expose exactly four stages");
 check(line?.querySelector('[data-energy-path-quality-stage="drivers"]')?.textContent.includes("50"),"driver partial quality was replaced by stale global mapped100");
 check(line?.querySelector('[data-energy-path-quality-stage="carriers"]')?.textContent.toLowerCase().includes("complete"),"carrier complete is not distinct from driver partial");
 check(Boolean(host.querySelector(".energy-path-stage-grid")?.compareDocumentPosition(line)&Node.DOCUMENT_POSITION_FOLLOWING),"quality line is not below the graph");
 check(!visible(host.querySelector("[data-energy-path-data-details]")),"Data details drawer is visible by default");
 check(![...host.querySelectorAll("table")].some(visible),"long data/source table visible beside default graph");
 check(!host.querySelector('[data-simulation-energy-view="sources"],[data-simulation-energy-view="reconciliation"]'),"Sources/Reconciliation primary subviews were added");
 line?.querySelector('[data-energy-path-quality-stage="drivers"]')?.click();
 check(state.simulationEnergyDetailsOpen===true&&state.simulationEnergyDetailsTab==="data"&&state.simulationEnergyDetailsStage==="drivers","actual stage click did not open filtered Data drawer state");
 check(visible(host.querySelector("[data-energy-path-data-details]")),"stage click did not reveal drawer");
 check(host.querySelector('[data-energy-path-details-panel="data"]')?.textContent.includes("Zone Lights Convective Heating Energy"),"missing driver request is absent from filtered details");
 check(!host.querySelector('[data-energy-path-details-panel="data"]')?.textContent.includes("Cooling:Electricity"),"driver filter leaked unrelated end-use availability");
 check(!host.querySelector('[data-energy-path-source-availability="Zone Air System Sensible Cooling Energy"]'),"explicit load availability leaked into Drivers through a shared source trace");
 check(host.contains(document.activeElement)&&document.activeElement!==host,"opening drawer lost keyboard focus");
 const dataTab=host.querySelector('[data-energy-path-details-tab="data"]');
 dataTab?.focus();dataTab?.dispatchEvent(new KeyboardEvent("keydown",{key:"ArrowRight",bubbles:true}));
 check(state.simulationEnergyDetailsTab==="output"&&document.activeElement?.dataset.energyPathDetailsTab==="output","ArrowRight did not select and focus the Output tab");
 document.activeElement?.dispatchEvent(new KeyboardEvent("keydown",{key:"Home",bubbles:true}));
 check(state.simulationEnergyDetailsTab==="data"&&document.activeElement?.dataset.energyPathDetailsTab==="data","Home did not select and focus the Data tab");
 document.activeElement?.dispatchEvent(new KeyboardEvent("keydown",{key:"End",bubbles:true}));
 check(state.simulationEnergyDetailsTab==="output"&&document.activeElement?.dataset.energyPathDetailsTab==="output","End did not select and focus the Output tab");
 document.activeElement?.dispatchEvent(new KeyboardEvent("keydown",{key:"ArrowLeft",bubbles:true}));
 check(state.simulationEnergyDetailsTab==="data"&&document.activeElement?.dataset.energyPathDetailsTab==="data","ArrowLeft did not select and focus the Data tab");
 document.activeElement?.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true}));
 check(!state.simulationEnergyDetailsOpen&&!visible(host.querySelector("[data-energy-path-data-details]"))&&document.activeElement?.dataset.energyPathQualityStage==="drivers","Escape did not close the drawer and restore the actual Drivers opener focus");
 host.querySelector("[data-energy-path-details-toggle]")?.click();
 document.activeElement?.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true}));
 check(!state.simulationEnergyDetailsOpen&&document.activeElement?.hasAttribute("data-energy-path-details-toggle"),"toggle-opened drawer did not return Escape focus to its header toggle");
 mount(explanation,requestObjects,{simulationEnergyDetailsOpen:true,simulationEnergyDetailsStage:"drivers"});
 const allStages=host.querySelector('[data-energy-path-quality-stage=""]');
 check(Boolean(allStages),"filtered drawer has no All stages reset action");
 allStages?.click();
 check(state.simulationEnergyDetailsStage===""&&state.simulationEnergyDetailsOpen&&state.simulationEnergyDetailsTab==="data","All stages did not clear only the Data stage filter");
 check(host.querySelectorAll("[data-energy-path-source-availability]").length===explanation.completeness.sourceAvailability.length&&host.querySelector('[data-energy-path-source-availability="Zone Air System Sensible Cooling Energy"]')&&host.querySelector('[data-energy-path-source-availability="Cooling:Electricity"]'),"All stages did not restore full run source availability");
 const storage={...explanation,quality:{...quality,endUses:level("partial",1,2)},completeness:{...explanation.completeness,sourceAvailability:[...explanation.completeness.sourceAvailability,
  {name:"Electric Storage Charge Energy",level:"energy",status:"missing"},
  {name:"Electric Storage Discharge Energy",level:"energy",status:"missing"},
  {name:"ElectricityProduced:Facility",level:"energy",status:"missing"},
 ]}};
 mount(storage,requestObjects,{simulationEnergyDetailsOpen:true,simulationEnergyDetailsStage:"endUses"});
 check(host.querySelector('[data-energy-path-source-availability="Electric Storage Charge Energy"]')?.dataset.energyPathAvailabilityStatus==="missing","missing storage charge consumption was excluded from End uses availability");
 check(!host.querySelector('[data-energy-path-source-availability="Electric Storage Discharge Energy"]')&&!host.querySelector('[data-energy-path-source-availability="ElectricityProduced:Facility"]'),"storage discharge or onsite supply was classified as End uses consumption");

 for(const [sourceID,expectedIndex]of[["meter.monthly",0],["people.office",2],["people.wildcard",3],["people.stale",2]]){
  const source=sources.find(item=>item.id===sourceID);
  const resolved=resolver.resolveEnergyPathOutputRequest(source,requestObjects,sources);
  check(resolved.status==="exact"&&resolved.requestIndex===expectedIndex,"wrong exact output request for "+sourceID);
  mount(explanation,requestObjects,{simulationEnergyDetailsOpen:true,simulationEnergyDetailsTab:"data"});
  let action=host.querySelector('[data-energy-path-output-source="'+sourceID+'"]');
  if(sourceID==="meter.monthly"||sourceID==="people.office")check(Boolean(action),"rendered source row has no Output jump action: "+sourceID);
  // Supplemental raw-source cases use the same production delegated handler.
  if(!action){action=document.createElement("button");action.dataset.energyPathOutputSource=sourceID;host.append(action);}
  action.click();
  check(state.simulationEnergyDetailsOpen&&state.simulationEnergyDetailsTab==="output"&&state.simulationEnergyOutputSource===sourceID,"source jump did not navigate internal Output tab for "+sourceID);
  const selected=host.querySelector('[data-energy-path-output-request-selected="true"]');
  check(selected?.dataset.energyPathOutputRequest===resolver.energyPathOutputRequestKey(requestObjects[expectedIndex],expectedIndex),"Output selection ignored frequency/key/index verification for "+sourceID);
  check(visible(host.querySelector('[data-energy-path-details-panel="output"]')),"Output panel not visible after exact source navigation");
  check(document.activeElement===selected,"exact source navigation did not focus its selected Output request: "+sourceID);
 }
 for(const[sourceID,status]of[["people.no_frequency","ambiguous"],["derived.people","derived"],["table.total","tabular"]]){
  const source=sources.find(item=>item.id===sourceID);
  const resolved=resolver.resolveEnergyPathOutputRequest(source,requestObjects,sources);
  check(resolved.status===status,"nonexact source got wrong resolution status: "+sourceID+"="+resolved.status);
  mount(explanation,requestObjects,{simulationEnergyDetailsOpen:true,simulationEnergyDetailsTab:"output",simulationEnergyOutputSource:sourceID});
  check(!host.querySelector('[data-energy-path-output-request-selected="true"]'),"nonexact source fabricated an exact request selection: "+sourceID);
 }
 check(resolver.resolveEnergyPathOutputRequest(sources[0],[],sources).status==="unavailable","missing run plan fabricated a matching Output request");
 mount(explanation,[],{simulationEnergyDetailsOpen:true,simulationEnergyDetailsTab:"output",simulationEnergyOutputSource:"meter.monthly"});
 check(!host.querySelector('[data-energy-path-output-request-selected="true"]'),"no-plan output drawer selected a request from another run");
 const zone=structuredClone(explanation);
 zone.scope={kind:"zone",zoneName:"Office",aggregationBasis:"model_total"};
 zone.quality={...quality,zoneAllocatedPct:82,unassignedPct:18,zoneAllocationStatus:"partial"};
 // Availability is run-level: a shared row may mention another zone without
 // authorizing its raw source details in the selected Office drawer.
 zone.completeness.sourceAvailability[0].sourceIds.push("people.wildcard");
 zone.nodes[0].sourceIds.push("derived.office","lab.graph");
 zone.periods=[{id:"M1",label:"January",nodes:zone.nodes,links:zone.links,quality:{...quality,zoneAllocatedPct:60,unassignedPct:40,zoneAllocationStatus:"partial"}}];
 const withZone={...explanation,sources:[...sources,
  {id:"office.detail",sourceType:"sql_variable",name:"Equipment pooled source",scopeDetails:[{scope:{kind:"zone",zoneName:"Office"},allocatedValue:5}]},
  {id:"derived.office",sourceType:"derived_heat_balance",name:"Office allocated pressure",inputSourceIds:["lab.input"],formula:"load * pressure / sum(pressure)"},
  {id:"lab.input",sourceType:"sql_variable",name:"Lab pressure input",zoneName:"Lab",keyValue:"Lab"},
  {id:"lab.graph",sourceType:"sql_variable",name:"Shared graph reference",zoneName:"Lab",keyValue:"Lab"},
 ],zoneResults:[zone]};
 mount(withZone,requestObjects,{simulationEnergyScopeKind:"zone",simulationEnergyZoneName:"Office"});
 const annualZoneLine=host.querySelector("[data-energy-path-quality-line]");
 const annualAllocation=annualZoneLine?.querySelector("[data-energy-path-zone-allocation-status]");
 check(annualZoneLine?.textContent.includes("Zone Office")&&annualZoneLine?.textContent.includes("Model total contribution"),"Zone quality line lost selected scope or model-total contribution basis");
 check(annualAllocation?.textContent.includes("82%")&&annualAllocation?.textContent.includes("18%")&&annualAllocation?.textContent.includes("direct/allocated"),"Zone annual direct/allocated and unassigned shares are not explicit");
 host.querySelector("[data-energy-path-details-toggle]")?.click();
 check(!host.querySelector('[data-energy-path-data-source="people.wildcard"]'),"Office Data drawer leaked an unrelated explicit Lab source");
 for(const sourceID of["people.office","office.detail","derived.office","lab.input","lab.graph"]){
  check(Boolean(host.querySelector('[data-energy-path-data-source="'+sourceID+'"]')),"Office Data drawer lost matching scope or graph/derivation trace: "+sourceID);
 }
 host.querySelector('[data-energy-path-details-tab="output"]')?.click();
 check(host.querySelectorAll("[data-energy-path-output-request]").length===requestObjects.length,"Zone source filtering truncated the authoritative run Output plan");
 mount(withZone,requestObjects,{simulationEnergyScopeKind:"zone",simulationEnergyZoneName:"Office",simulationEnergyPeriod:"M1"});
 const monthlyAllocation=host.querySelector("[data-energy-path-zone-allocation-status]");
 check(monthlyAllocation?.textContent.includes("60%")&&monthlyAllocation?.textContent.includes("40%")&&!monthlyAllocation?.textContent.includes("82%"),"Zone January reused annual allocation shares");
 mount(withZone,requestObjects,{simulationEnergyScopeKind:"zone",simulationEnergyZoneName:"Office",simulationEnergyPeriod:"M2",simulationEnergyDetailsOpen:true});
 const absentAllocation=host.querySelector("[data-energy-path-zone-allocation-status]");
 check(absentAllocation?.dataset.energyPathZoneAllocationStatus==="unavailable"&&!absentAllocation?.textContent.includes("%"),"absent Zone month invented allocation shares");
 check([...host.querySelectorAll("[data-energy-path-closure]")].every(item=>item.dataset.energyPathClosureStatus==="unavailable"&&!item.textContent.includes("%")),"absent month reused annual energy conservation percentages");
 check(host.querySelector("[data-energy-path-ratio-availability]")?.dataset.energyPathRatioAvailability==="unavailable","absent month reused annual conversion ratio availability");
 check(host.querySelector('[data-energy-path-quality-stage="carriers"]')?.dataset.energyPathQualityStatus==="complete","run-level source availability was unnecessarily erased for an absent month");
 const missingQuality=structuredClone(explanation);delete missingQuality.quality;
 mount(missingQuality);
 const unknownStages=[...host.querySelectorAll("[data-energy-path-quality-stage]")];
 check(unknownStages.length===4&&unknownStages.every(item=>!item.textContent.includes("100")&&!item.textContent.toLowerCase().includes("complete")),"missing quality payload invented stage completeness from global mapped100");
 const light={schema:explanation.schema,scope:explanation.scope,nodes:[],links:[],sources:[],quality:{...quality,drivers:level("not_requested"),loads:level("not_requested"),endUses:level("not_requested"),carriers:level("not_requested"),ratios:level("not_requested"),driverToLoadStatus:"not_requested",endUseToCarrierStatus:"not_requested"}};
 mount(light,[]);
 check(host.querySelectorAll("[data-energy-path-quality-stage]").length===4&&host.textContent.includes("Not requested"),"quality-only Light result disappeared or became missing");
 const toggle=host.querySelector("[data-energy-path-details-toggle]");toggle?.click();
 check(state.simulationEnergyDetailsOpen&&host.querySelector("[data-energy-path-quality-line]"),"actual global rerender discarded quality-only result");
 mount(explanation);
 host.style.width="360px";
 const narrowLine=host.querySelector("[data-energy-path-quality-line]");
 check(narrowLine&&narrowLine.scrollWidth<=narrowLine.clientWidth+1,"four-stage quality line overflows a360px viewport");
 check(narrowLine&&narrowLine.getBoundingClientRect().width<=host.getBoundingClientRect().width+1,"four-stage quality line escaped the narrow result panel");
 host.querySelector("[data-energy-path-details-toggle]")?.click();
 const narrowDrawer=host.querySelector("[data-energy-path-data-details]");
 check(narrowDrawer&&narrowDrawer.getBoundingClientRect().width<=host.getBoundingClientRect().width+1,"Data details drawer overflows a narrow result panel");
 if(failures.length)throw new Error(failures.join(" | "));
 document.body.dataset.epath131Status="passed";document.getElementById("result").textContent="passed";
 }
}catch(error){document.body.dataset.epath131Status="failed";document.getElementById("result").textContent=String(error?.stack||error);}
</script></body></html>`
