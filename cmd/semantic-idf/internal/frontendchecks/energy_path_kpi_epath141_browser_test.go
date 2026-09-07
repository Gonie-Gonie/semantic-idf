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

func TestEPATH141ActualDashboardFourKPIsAndExactSelection(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser four-KPI acceptance")
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
			t.Logf("computed KPI readability: %s", document[start+len("<pre id=\"result\">"):start+end])
		}
	}
}

const epath141KPIHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>EPATH141 compact KPI navigation</title>
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
const buildingScope={kind:"building",aggregationBasis:"model_total"},zoneScope={kind:"zone",zoneName:"Office",aggregationBasis:"model_total"};
const annual=graph(buildingScope,"annual",10,true),january=graph(buildingScope,"M1"),february=graph(buildingScope,"M2",2);
february.links=february.links.filter(link=>link.relation!=="load_to_end_use");
const sources=["people","cooling","heating","electricity","gas"].map(name=>({id:"sql."+name,name:"SOURCE_SENTINEL "+name,sourceType:"sql_variable",sourceUnit:"J",normalizedUnit:"kWh",reportingFrequency:"Monthly"}));
const explanation=freeze({schema:"semantic-idf.energy-explanation/v2",scope:buildingScope,...annual,sources,periods:[january,february],zoneResults:[{scope:zoneScope,...graph(zoneScope,"annual",5,true),periods:[graph(zoneScope,"M1",.5)]}]});
const result=freeze({status:"succeeded",purposeResults:{energyExplanation:explanation,energyExplanationSummary:annual.summary},purposeRunPlan:{outputObjects:[]}}),originalJSON=JSON.stringify(result);
try{
 const[{state},simulation,view]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/views/energy-path-view.js")]);
 simulation.initializeSimulationControls();await new Promise(resolve=>setTimeout(resolve,0));
 const host=document.getElementById("simulationEnergyDashboard"),pane=document.getElementById("simulationPane");
 const mount=(payload=result,extra={})=>{Object.assign(state,{simulationResult:payload,simulationActiveResultView:"energy",simulationEnergyScopeKind:"building",simulationEnergyZoneName:"",simulationEnergyPeriod:"M1",simulationEnergyService:"all",simulationEnergySelection:"",simulationEnergyDetailsOpen:false,simulationEnergyDetailsTab:"data",simulationEnergyDetailsStage:"",simulationEnergyOutputSource:""},extra);simulation.renderSimulationEnergyDashboard(payload);};
 const card=id=>host.querySelector('[data-energy-path-kpi="'+id+'"]');
 const change=(selector,value)=>{const control=host.querySelector(selector);check(Boolean(control),"missing control "+selector);if(control){control.focus();control.value=value;control.dispatchEvent(new Event("change",{bubbles:true}));}};
 const activate=button=>{check(button?.tagName==="BUTTON"&&!button.disabled,"KPI target is not a native enabled button");button?.focus();button?.click();};
 const valueText=id=>card(id)?.querySelector("strong")?.textContent||"";
 mount();
 if(new URLSearchParams(location.search).get("manual")==="1"){
  document.body.dataset.epath141Status="manual";document.getElementById("result").textContent="Manual EPATH-141 fixture: January has a 100 kWh cooling load but matched conversion 40/20=2; total site chooser includes electricity and gas. Coverage shows separate 50% / 95% boundaries.";
 }else{
 check(host.querySelectorAll("[data-energy-path-kpi]").length===4,"default dashboard does not have exactly four compact KPI cards");
 check(valueText("total_site_energy").includes("120")&&valueText("cooling_load").includes("100")&&valueText("heating_load").includes("80"),"January KPI values do not come from the selected-period summary");
 const values=["total_site_energy","cooling_load","heating_load"].map(valueText);
 check(!host.querySelector(".energy-path-kpis")?.textContent.includes("SOURCE_SENTINEL")&&!host.querySelector(".energy-path-kpis")?.textContent.includes("SQL"),"KPI strip exposes source names or SQL counts");
 for(const service of["cooling","heating","all"]){
  change("[data-simulation-energy-service]",service);
  check(JSON.stringify(["total_site_energy","cooling_load","heating_load"].map(valueText))===JSON.stringify(values),"changing Service changed invariant scope/month KPI totals: "+service);
  const emphasized=[...host.querySelectorAll('[data-energy-path-kpi-emphasized="true"]')];
  check(service==="all"?emphasized.length===0:emphasized.length===1&&emphasized[0].dataset.energyPathKpi===service+"_load","wrong selected-service KPI emphasis: "+service);
  if(service==="heating"){
   const ratio=card("heating_load")?.querySelector('[data-energy-path-kpi-ratio="heating"]');
   check(Number(ratio?.dataset.energyPathKpiRatioValue)===.8&&ratio?.dataset.energyPathKpiRatioPartial==="false","Heating emphasis lost its actual80/100 matched load/site ratio");
  }
 }
 change("[data-simulation-energy-service]","cooling");
 const coolingRatio=card("cooling_load")?.querySelector('[data-energy-path-kpi-ratio="cooling"]');
 check(Number(coolingRatio?.dataset.energyPathKpiRatioValue)===2&&coolingRatio?.dataset.energyPathKpiRatioPartial==="true","Cooling KPI used node100/site20 or stale ratio instead of matched40/20 and partial-overlap label");
 check(coolingRatio?.textContent.toLowerCase().includes("partial"),"partial conversion overlap is not explained in the visible KPI");
 const actualCoolingLinkIDs=view.energyPathGraphForState(explanation,{...state,simulationEnergyService:"all"}).links.filter(link=>link.relation==="load_to_end_use"&&link.fromId==="load.cooling.building"&&link.toId==="end_use.cooling.building").map(link=>link.id);
 const ratioLinkIDs=JSON.parse(coolingRatio?.dataset.energyPathKpiRatioLinks||"[]");
 check(actualCoolingLinkIDs.length===1&&ratioLinkIDs.length===1&&ratioLinkIDs[0]===actualCoolingLinkIDs[0],"KPI ratio lost the actual projected conversion-link identity");
 const boundary=(id)=>card("coverage")?.querySelector('[data-energy-path-kpi-boundary="'+id+'"]');
 check(boundary("driver_to_load")?.dataset.energyPathKpiBoundaryStatus==="partial"&&Number(boundary("driver_to_load")?.dataset.energyPathKpiBoundaryValue)===50,"coverage card lost Driver to load boundary status/value");
 check(boundary("end_use_to_carrier")?.dataset.energyPathKpiBoundaryStatus==="partial"&&Number(boundary("end_use_to_carrier")?.dataset.energyPathKpiBoundaryValue)===95,"coverage card lost End use to carrier boundary status/value");
 check(!card("coverage")?.textContent.includes("99%")&&!card("coverage")?.textContent.includes("72.5%"),"coverage card invented mapped/min/average scalar quality");
 const chooser=card("total_site_energy")?.querySelector('[data-energy-path-kpi-chooser="total_site_energy"]');
 check(chooser?.tagName==="DETAILS"&&!chooser.open&&chooser.querySelectorAll("[data-energy-path-kpi-node]").length===2,"total site card guessed a carrier instead of an explicit two-target chooser");
 chooser?.querySelector("summary")?.focus();chooser?.querySelector("summary")?.click();
 check(chooser?.open&&state.simulationEnergySelection==="","opening the carrier chooser selected an arbitrary graph node");
 activate(chooser?.querySelector('[data-energy-path-kpi-node="carrier.natural_gas.building"]'));
 check(state.simulationEnergyService==="all"&&state.simulationEnergySelection==="carrier.natural_gas.building"&&host.querySelector('[data-energy-path-inspector="carrier.natural_gas.building"]'),"selecting gas from Cooling did not reveal the actual all-service carrier");
 check(document.activeElement?.dataset.energyExplanationNode==="carrier.natural_gas.building","carrier KPI selection lost focus on the revealed graph node");
 change("[data-simulation-energy-service]","heating");
 activate(card("cooling_load")?.querySelector('[data-energy-path-kpi-node="load.cooling.building"]'));
 check(state.simulationEnergyService==="cooling"&&state.simulationEnergySelection==="load.cooling.building"&&document.activeElement?.dataset.energyExplanationNode==="load.cooling.building","opposite-service Cooling KPI did not switch and focus its actual load");
 activate(card("heating_load")?.querySelector('[data-energy-path-kpi-node="load.heating.building"]'));
 check(state.simulationEnergyService==="heating"&&state.simulationEnergySelection==="load.heating.building","opposite-service Heating KPI did not reveal its actual load");
 change("[data-simulation-energy-service]","all");activate(card("cooling_load")?.querySelector('[data-energy-path-kpi-node="load.cooling.building"]'));
 check(state.simulationEnergyService==="all","All-service load KPI unnecessarily narrowed the user's service context");
 state.simulationEnergyDetailsStage="drivers";
 activate(card("coverage")?.querySelector("[data-energy-path-kpi-details]"));
 check(state.simulationEnergyDetailsOpen&&state.simulationEnergyDetailsTab==="data"&&state.simulationEnergyDetailsStage===""&&state.simulationEnergySelection==="load.cooling.building","coverage KPI did not open all-stage Data details without inventing an energy-node selection");
 document.activeElement?.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true}));
 check(!state.simulationEnergyDetailsOpen&&document.activeElement?.hasAttribute("data-energy-path-kpi-details"),"Escape did not restore the actual Coverage KPI opener");
 change("[data-simulation-energy-path-period]","M2");change("[data-simulation-energy-service]","cooling");
 const missingRatio=card("cooling_load")?.querySelector('[data-energy-path-kpi-ratio="cooling"]');
 check(missingRatio&&!missingRatio.hasAttribute("data-energy-path-kpi-ratio-value")&&!missingRatio.textContent.includes("88"),"month with no conversion links reused annual/stale ratio or fabricated zero");
 check(valueText("cooling_load").includes("200"),"February KPI fell back to annual cooling total");
 change("[data-simulation-energy-path-period]","M3");
 check(host.querySelectorAll("[data-energy-path-kpi]").length===4&&!/\d/.test(valueText("total_site_energy")),"absent month invented a zero/annual site-energy KPI or dropped the four-card strip");
 mount(result,{simulationEnergyScopeKind:"zone",simulationEnergyZoneName:"Office"});
 check(host.querySelectorAll("[data-energy-path-kpi]").length===4&&valueText("total_site_energy").includes("60")&&card("total_site_energy")?.textContent.includes("Known zone site energy")&&card("total_site_energy")?.textContent.includes("Known only"),"partial Zone lost its known-only 60 kWh qualifier or fourth coverage card");
 check(boundary("end_use_to_carrier")?.dataset.energyPathKpiBoundaryStatus==="unavailable"&&!boundary("end_use_to_carrier")?.hasAttribute("data-energy-path-kpi-boundary-value"),"Zone subtotal presented unknown facility closure as numeric coverage");
 const emptyScope={kind:"building",aggregationBasis:"model_total"};
 const zeroSummary={schema:"semantic-idf.energy-explanation-summary/v2",scope:emptyScope,period:"annual",loads:[{id:"load.cooling.zero",serviceKind:"cooling",value:0,unit:"kWh"}],carriers:[],completeness:{mappedPercent:100}};
 const zeroPayload=freeze({schema:explanation.schema,scope:emptyScope,nodes:[],links:[],sources:[],summary:zeroSummary,quality:{...quality,driverToLoadStatus:"not_requested",endUseToCarrierStatus:"unavailable"}});
 mount(freeze({purposeResults:{energyExplanation:zeroPayload,energyExplanationSummary:zeroSummary}}),{simulationEnergyPeriod:"annual"});
 check(valueText("cooling_load").includes("0")&&!/\d/.test(valueText("heating_load"))&&!/\d/.test(valueText("total_site_energy")),"reported zero and missing energy groups were conflated");
 check(!host.querySelector("[data-energy-path-kpi-node]")&&host.querySelectorAll("[data-energy-path-kpi]").length===4,"no-graph KPI payload fabricated a target or omitted cards");
 check(!card("coverage")?.textContent.includes("100%")&&!card("coverage")?.textContent.includes("0%"),"unavailable/not-requested boundaries became a fake numeric completeness score");
 mount();pane.style.width="360px";
 const strip=host.querySelector(".energy-path-kpis");
 check(strip&&strip.getBoundingClientRect().width<=pane.getBoundingClientRect().width+1&&strip.scrollWidth<=strip.clientWidth+1,"four-card KPI strip overflows a narrow result panel");
 pane.style.width="1100px";
 check(strip&&strip.getBoundingClientRect().height<host.querySelector(".energy-path-stage-grid")?.getBoundingClientRect().height,"default KPI strip visually dominates the Energy Path graph");
 change("[data-simulation-energy-service]","cooling");
 const colorChannels=value=>{const match=value.match(/^rgba?\(([^)]+)\)$/);if(!match)throw new Error("unsupported computed color: "+value);return match[1].split(/[ ,/]+/).filter(Boolean).map(Number);};
 const contrast=element=>{
  const foreground=colorChannels(getComputedStyle(element).color);
  let background=[255,255,255];
  for(let node=element;node;node=node.parentElement){const color=colorChannels(getComputedStyle(node).backgroundColor);if(color.length===3||color[3]>=.999){background=color;break;}}
  const luminance=channels=>channels.slice(0,3).map(value=>{const channel=value/255;return channel<=.04045?channel/12.92:((channel+.055)/1.055)**2.4;}).reduce((sum,value,index)=>sum+value*[.2126,.7152,.0722][index],0);
  const a=luminance(foreground),b=luminance(background);return(Math.max(a,b)+.05)/(Math.min(a,b)+.05);
 };
 const originalTheme=document.documentElement.getAttribute("data-theme");
 const readability=[];
 for(const theme of["light","dark"]){
  document.documentElement.dataset.theme=theme;
  for(const[selector,minFont]of[["[data-energy-path-kpi-boundary] > span",11],["[data-energy-path-kpi-boundary] > b",11],["[data-energy-path-kpi-ratio='cooling']",11],[".energy-path-kpi-ratio-note",10]]){
   const text=host.querySelector(selector),font=text?parseFloat(getComputedStyle(text).fontSize):0,ratio=text?contrast(text):0;
   readability.push({theme,selector,font,contrast:Number(ratio.toFixed(3))});
   check(text&&font>=minFont&&ratio>=4.5,"KPI readability failed "+theme+" "+selector+": "+font+"px / "+ratio.toFixed(3)+":1");
  }
 }
 if(originalTheme===null)document.documentElement.removeAttribute("data-theme");else document.documentElement.setAttribute("data-theme",originalTheme);
 check(JSON.stringify(result)===originalJSON,"KPI scope/period/service, chooser or drawer interactions mutated raw/export inputs");
 if(failures.length)throw new Error(failures.join(" | "));
 document.body.dataset.epath141Status="passed";document.getElementById("result").textContent=JSON.stringify({passed:true,readability});
 }
}catch(error){document.body.dataset.epath141Status="failed";document.getElementById("result").textContent=String(error?.stack||error);}
</script></body></html>`
