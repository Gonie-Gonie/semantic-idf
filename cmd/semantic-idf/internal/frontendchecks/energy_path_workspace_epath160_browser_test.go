package frontendchecks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// This acceptance intentionally keeps main.js and both auxiliary pages intact.
// It uses fresh documents, not a synthetic restore call or BFCache state. Only
// read-only backend responses are fixtures; all navigation and restore code is real.
func TestEPATH160ColdSettingsBatchWorkspaceReturnBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser cold Settings / Batch workspace acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	const input = "Version,25.1;\nBuilding,EPATH160;\nZone,Office;\n"
	digest := sha256.Sum256([]byte(input))
	bridge := strings.ReplaceAll(epath160WorkspaceBridgeHTML, "EPATH160_TEXT_HASH", hex.EncodeToString(digest[:]))
	pages := map[string]string{}
	for _, name := range []string{"index.html", "settings.html", "tools.html"} {
		page := readTestFile(t, "frontend/src/"+name)
		if name == "index.html" && !strings.Contains(page, `<script type="module" src="./js/main.js"></script>`) {
			t.Fatal("actual main bootstrap is required for this cold-return acceptance")
		}
		page = strings.Replace(page, "<head>", "<head>"+bridge, 1)
		pages[name] = strings.Replace(page, "</body>", epath160WorkspaceAutomationHTML+"</body>", 1)
	}
	type browserResult struct {
		Failures []string       `json:"failures"`
		Evidence []string       `json:"evidence"`
		Counts   map[string]int `json:"counts"`
	}
	var mu sync.Mutex
	requests := map[string]int{}
	done := make(chan browserResult, 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	for name, page := range pages {
		mux.HandleFunc("/src/"+name, func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			requests[name]++
			mu.Unlock()
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, page)
		})
	}
	mux.HandleFunc("/epath160/done", func(w http.ResponseWriter, r *http.Request) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// No virtual-time budget: native page navigations and main's asynchronous
	// cache restoration must actually complete. This isolated CI Chrome is killed
	// only after the final in-page assertion report (or the bounded timeout).
	command := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--disable-features=BackForwardCache", "--force-device-scale-factor=1", "--window-size=1600,900", "--user-data-dir="+t.TempDir(), server.URL+"/src/index.html")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	select {
	case result := <-done:
		for _, line := range result.Evidence {
			t.Log(line)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(result.Failures) != 0 {
			t.Fatalf("cold workspace acceptance failed: %s; requests=%v counts=%v", strings.Join(result.Failures, "\n"), requests, result.Counts)
		}
		if requests["index.html"] < 8 || requests["settings.html"] != 1 || requests["tools.html"] != 1 {
			t.Fatalf("did not traverse actual cold main / Settings / Tools pages: %v", requests)
		}
		if result.Counts["mainBoots"] != requests["index.html"] || result.Counts["forbidden"] != 0 || result.Counts["simulationLookups"] < 8 || result.Counts["autoRunNegativeProbe"] != 1 {
			t.Fatalf("cold restore skipped a document or ran analysis / simulation: requests=%v counts=%v", requests, result.Counts)
		}
	case <-ctx.Done():
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("cold navigation timed out: requests=%v", requests)
	}
}

const epath160WorkspaceBridgeHTML = `<script>
(()=>{
 const key="epath160",read=()=>JSON.parse(sessionStorage.getItem(key)||"null"),write=value=>sessionStorage.setItem(key,JSON.stringify(value));
 const text="Version,25.1;\nBuilding,EPATH160;\nZone,Office;\n",textHash="EPATH160_TEXT_HASH",runId="epath160-cached-run";
 const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
 const level={found:1,total:1,percent:100,status:"complete"};
 const quality={drivers:level,loads:level,endUses:level,carriers:level,ratios:level,driverToLoadClosedPct:100,endUseToCarrierClosedPct:100,driverToLoadStatus:"complete",endUseToCarrierStatus:"complete",zoneAllocationStatus:"complete",zoneAllocatedPct:100,unassignedPct:0};
 const sourceID="source.epath160.cooling.monthly",pathID="service.epath160.office.cooling",entityID="path:"+pathID;
 function graph(scope,period,factor){
  const suffix="."+(scope.zoneName||"building")+"."+period,sourceIds=[sourceID];
  const add=(id,level,value,extra)=>({id:id+suffix,label:level==="driver"?"Exterior walls":level==="load"?"Cooling load":level==="end_use"?"Cooling equipment":"Electricity",level,value:value*factor,rawValue:value*factor,effectiveValue:value*factor,allocatedValue:value*factor,displayValue:value*factor,unit:"kWh",period,zoneName:scope.zoneName||"",sourceIds,...extra});
  const driver=add("driver.walls","driver",100,{driverCategory:"surface.exterior_walls",serviceKind:"cooling",scaleDomain:"thermal",basis:"heat_balance_share",allocationApplied:true});
  const load=add("load.cooling","load",100,{serviceKind:"cooling",scaleDomain:"thermal",basis:"reported_variable",relatedPathIds:[pathID]});
  const use=add("end_use.cooling","end_use",25,{endUse:"cooling",serviceKind:"cooling",scaleDomain:"site",basis:"reported_meter",relatedPathIds:[pathID]});
  const carrier=add("carrier.electricity","carrier",25,{carrier:"electricity",scaleDomain:"site",basis:"reported_meter",meterHierarchyLevel:scope.kind==="zone"?"zone_total":"facility_total"});
  const link=(from,to,relation)=>({id:relation+suffix,fromId:from.id,toId:to.id,relation,fromValue:from.value,toValue:to.value,value:to.value,fromUnit:"kWh",toUnit:"kWh",unit:"kWh",period,zoneName:scope.zoneName||"",serviceKind:"cooling",sourceIds,basis:relation==="driver_to_load"?"heat_balance_share":"reported_meter",domain:relation==="driver_to_load"?"thermal":"site",...(relation==="load_to_end_use"?{ratio:4,ratioKind:"coefficient_of_performance"}:{})});
  const nodes=[driver,load,use,carrier],links=[link(driver,load,"driver_to_load"),link(load,use,"load_to_end_use"),link(use,carrier,"end_use_to_carrier")];
  const completeness={heatBalance:level,load:level,energyUse:level,mappedPercent:100,sourceAvailability:[{stage:"heat",status:"complete",recordName:"Exterior wall heat transfer",sourceIds},{stage:"energy",status:"complete",recordName:"Cooling:Electricity",sourceIds}]};
  const summary={schema:"semantic-idf.energy-explanation-summary/v2",scope,period,quality,completeness,drivers:[driver],loads:[load],endUses:[use],carriers:[carrier]};
  return {id:period,period,nodes,links,quality,completeness,summary};
 }
 const buildingScope={kind:"building"},zoneScope={kind:"zone",zoneName:"Office"},building=graph(buildingScope,"annual",1),zone=graph(zoneScope,"annual",1);
 const explanation={schema:"semantic-idf.energy-explanation/v2",scope:buildingScope,...building,periods:[graph(buildingScope,"M1",.4),graph(buildingScope,"M2",.6)],zoneResults:[{scope:zoneScope,...zone,periods:[graph(zoneScope,"M1",.4),graph(zoneScope,"M2",.6)]}],sources:[{id:sourceID,name:"Cooling:Electricity",sourceType:"sql_meter",isMeter:true,sourceUnit:"J",normalizedUnit:"kWh",reportingFrequency:"Monthly",objectIndex:5}]};
 const result=freeze({runId,status:"succeeded",purposeResults:{energyExplanation:explanation,energyExplanationSummary:building.summary},purposeRunPlan:{outputObjects:[{objectType:"Output:Meter",keyValue:"Cooling:Electricity",reportingFrequency:"Monthly",objectIndex:5}]},series:[]});
 const path={id:pathID,zoneName:"Office",serviceKind:"cooling",pathType:"zone_equipment",servedSubject:{kind:"zone",name:"Office",zoneName:"Office"},delivery:{id:"terminal.epath160",objectType:"ZoneHVAC:IdealLoadsAirSystem",objectName:"Office ideal cooling",objectIndex:4,displayName:"Office ideal cooling",role:"terminal",mediums:["air"]},conditioning:[]};
 path.deliveryEquipment={component:path.delivery,deliveryType:"ideal_loads",displayFamily:"Ideal loads",requiresAirLoop:false,canUsePlantLoop:false,hasInternalCoils:false,mediums:["air"]};
 const report={metrics:{categories:[]},geometry:{zones:[{name:"Office",storyIndex:0}],stories:[{index:0,name:"Ground floor",elevation:0}],surfaces:[],openings:[],topology:{nodes:[],connections:[],boundaries:[],airCouplings:[]}},profile:{dimensions:[],groups:[],rows:[],warnings:[]},hvac:{loops:[],loopCount:0,airLoopCount:0,plantLoopCount:0,condenserLoopCount:0,zoneRelations:[],nodeUsages:[],warnings:[],serviceModel:{zoneServices:[{id:"service-zone.Office",zoneName:"Office",servedSubject:path.servedSubject,paths:[path]}],couplings:[],navigation:{entities:[]}}},output:{existing:[]}};
 const semantic={navigation:{entities:[{id:entityID,kind:"hvac-path",label:"Office cooling path",viewTargets:[{view:"hvac",targetKind:"service-path",targetId:pathID,label:"Office cooling path"}],sourceAnchors:[]}],occurrences:[]}};
 let test=read();if(!test){test={phase:"initial",failures:[],evidence:[],counts:{mainBoots:0,forbidden:0,simulationLookups:0,analysisLookups:0},expected:null};write(test);sessionStorage.setItem("idfAnalyzer.currentDocument",JSON.stringify({schemaVersion:4,text,textHash,analysisKey:textHash,filename:"epath160.idf",activeResultTab:"simulation",activeInputView:"text",simulationResultRef:{textHash,runId},viewSnapshot:{resultTab:"simulation",inputView:"text",panelContexts:{simulation:{activeResultView:"energy",energyScopeKind:"building",energyZoneName:"",energyPeriod:"annual",energyService:"all",energySelection:"",energyDetailsOpen:false}}}}));}
 const update=callback=>{const item=read();callback(item);write(item);return item;};
 const isMain=location.pathname.endsWith("/index.html");if(isMain)update(item=>item.counts.mainBoots++);
 window.addEventListener("pageshow",event=>{if(event.persisted)update(item=>item.failures.push("BFCache preserved a document instead of a cold restore"));});
 window.addEventListener("error",event=>update(item=>item.failures.push("page error: "+(event.error?.stack||event.message))));
 window.addEventListener("unhandledrejection",event=>update(item=>item.failures.push("unhandled rejection: "+(event.reason?.stack||event.reason))));
 const api={
  GetCachedAnalysis:async key=>{update(item=>item.counts.analysisLookups++);return key===textHash?{text,analysisKey:textHash,report:JSON.parse(JSON.stringify(report)),semantic:JSON.parse(JSON.stringify(semantic)),timing:{mode:"full"}}:null;},
  GetCachedSimulationResult:async(key,id)=>{update(item=>item.counts.simulationLookups++);if(["cache_text_race","cache_run_race"].includes(read().phase))await new Promise(resolve=>{window.epath160.releaseCache=resolve;});else await new Promise(resolve=>setTimeout(resolve,80));return key===textHash&&id===runId?freeze(JSON.parse(JSON.stringify(result))):null;},
  GetSettings:async()=>({settings:{appearance:{theme:"light",language:"en",analysisTabOrder:["simulation","metrics","topology","profile","hvac"]},simulation:{autoRunOnOpen:true}}}),
  GetAppInfo:async()=>({name:"SemanticIDF",version:"160",platform:"windows"}),
  GetSimulationEnvironment:async()=>({energyPlusAvailable:true,installations:[{version:"25.1.0",executablePath:"C:/EPATH160/EnergyPlus/energyplus.exe",weatherDataPath:"C:/EPATH160/Weather"}],weatherFolders:[{source:"test",label:"Fixture weather",files:[{path:"C:/EPATH160/Weather/fixture.epw",name:"fixture.epw"}]}],settings:{autoRunOnOpen:true}})
 };
 window.go={main:{App:new Proxy(api,{get(target,property){if(Object.hasOwn(target,property))return target[property];if(/^(Analyze|Run|StartSimulation|RequestSimulation)/.test(String(property)))return async()=>{update(item=>{item.counts.forbidden++;item.evidence.push("Unexpected "+String(property)+" on "+location.pathname+" during "+item.phase);});throw new Error("forbidden work: "+String(property));};return target[property];}})}};
 window.runtime={EventsOn(){return()=>{};},EventsOnMultiple(){return()=>{};}};
 window.epath160={read,write,update,textHash,runId,sourceID,pathID,entityID,resultJSON:JSON.stringify(result)};
})();
</script>`

const epath160WorkspaceAutomationHTML = `<script type="module">
const fixture=window.epath160,sleep=ms=>new Promise(resolve=>setTimeout(resolve,ms));
const check=(value,message)=>{if(!value)fixture.update(item=>item.failures.push(message));};
const wait=async(predicate,label)=>{for(let i=0;i<160;i++){if(predicate())return;await sleep(50);}throw new Error("Timed out: "+label+"; "+document.getElementById("runtimeStatus")?.textContent);};
const done=async error=>{if(error)fixture.update(item=>item.failures.push(error?.stack||String(error)));await fetch("/epath160/done",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(fixture.read())});};
const visible=element=>Boolean(element&&element.getBoundingClientRect().width&&element.getBoundingClientRect().height);
const click=element=>{if(!element)throw new Error("required actual navigation control is absent");for(let details=element.closest("details");details;details=details.parentElement?.closest("details"))details.open=true;element.focus();element.click();};
try{
 if(location.pathname.endsWith("/settings.html")){
  await wait(()=>document.querySelector("#analysisTabOrderList"),"real Settings controls");
  check(visible(document.querySelector("#analysisTabOrderList")),"Settings page is a placeholder instead of actual settings UI");
  fixture.update(item=>{item.phase="settings_return";item.evidence.push("Actual Settings controls rendered before real Back to App");});
  click(document.querySelector("a[data-app-return]"));
 }else if(location.pathname.endsWith("/tools.html")){
  await sleep(250);click(document.querySelector('[data-tools-tab="batch-simulation"]'));
  await wait(()=>document.querySelector('[data-tools-panel="batch-simulation"]')?.classList.contains("active"),"actual Tools Batch Simulation tab");
  check(visible(document.querySelector("#multiSimulationRun"))&&document.querySelector("#multiSimulationRun").disabled&&visible(document.querySelector("#multiSimulationWorkers")),"Batch destination does not expose the actual non-running controls");
  fixture.update(item=>{item.phase="tools_return";item.evidence.push("Actual Tools Batch Simulation tab rendered without Run");});
  click(document.querySelector("a[data-app-return]"));
 }else{
 const [store,simulation,view,history,actions]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/views/energy-path-view.js"),import("/src/js/view-history.js"),import("/src/js/actions.js")]);
  const {state}=store;
  const phase=fixture.read().phase;
  if(["cache_text_race","cache_run_race"].includes(phase)){
   await wait(()=>typeof fixture.releaseCache==="function","controlled in-flight result lookup");
   const previousText=store.getDocumentText(),previousPath=state.currentFilePath,analysisBefore=fixture.read().counts.analysisLookups;
   let replacement;
   if(phase==="cache_text_race"){store.setDocumentText(previousText+"\n! Changed while cache read was pending");state.currentFilePath="C:/EPATH160/edited.idf";}
   else{replacement={runId:"newer-user-run",status:"running"};state.simulationResult=replacement;state.simulationActiveRunID=replacement.runId;state.simulationRunning=true;}
   fixture.releaseCache();await sleep(250);
   if(phase==="cache_text_race"){
    check(state.simulationResult===null&&fixture.read().counts.analysisLookups===analysisBefore,"late cache response overwrote edited document or started old-input analysis");
    store.setDocumentText(previousText);state.currentFilePath=previousPath;fixture.update(item=>{item.phase="cache_run_race";item.evidence.push("Delayed cache response rejected after document text/path changed");});location.reload();
   }else{
    check(state.simulationResult===replacement&&state.simulationActiveRunID===replacement.runId&&state.simulationRunning,"late cache response replaced a newer in-flight run");
    check(fixture.read().counts.forbidden===0,"cold restoration performed forbidden work");
    fixture.update(item=>item.evidence.push("Delayed cache response did not replace a newer in-flight run"));
    const sameText=store.getDocumentText();actions.registerLoadedDocument(sameText,{filename:"different.idf",path:"C:/EPATH160/different.idf"});
    check(state.simulationResult===null&&!state.simulationRunning&&state.simulationActiveRunID==="","identical-text different-file registration retained the previous run");
    const fresh=simulation.captureSimulationEnergyWorkspaceContext();
    check(fresh.simulationEnergyScopeKind==="building"&&fresh.simulationEnergyZoneName===""&&fresh.simulationEnergyPeriod==="annual"&&fresh.simulationEnergyService==="all"&&fresh.simulationEnergySelection===""&&!fresh.simulationEnergyDetailsOpen&&JSON.stringify(fresh.energyDrawer)===JSON.stringify({tab:"data",stage:"",outputSource:""}),"new file registration did not reset six Energy defaults and panel-local drawer");
    check(Object.keys(state).filter(key=>key.startsWith("simulationEnergy")).length===6,"new file registration introduced extra Energy primary fields");
    check(await actions.saveWorkspaceSnapshot()===true,"new file snapshot could not be saved");
    const freshSaved=JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument"));check(freshSaved.simulationResultRef===null&&freshSaved.text===sameText&&freshSaved.filename==="different.idf"&&fixture.read().counts.forbidden===0,"new file saved the old result locator or triggered backend work");
    fixture.update(item=>item.evidence.push("Identical-text different-file registration cleared the previous run and saved no result locator"));
    // Negative control: without the saved-input suppression key the exact same
    // usable environment and real lifecycle listener must reach the run API.
    // This local stub records that deliberately requested probe separately.
    state.simulationRunning=false;state.simulationAutoStartedKey="";state.simulationAutoRunOnOpen=true;
    document.getElementById("simulationWeatherSelect").value="C:/EPATH160/Weather/fixture.epw";
    window.go.main.App.RunPurposeSimulationText=async()=>{fixture.update(item=>item.counts.autoRunNegativeProbe=(item.counts.autoRunNegativeProbe||0)+1);return replacement;};
    window.dispatchEvent(new CustomEvent("idfAnalyzer:analysisComplete",{detail:{text:store.getDocumentText(),analysisKey:fixture.textHash,stage:"complete"}}));
    await wait(()=>fixture.read().counts.autoRunNegativeProbe===1,"auto-run negative control");
    fixture.update(item=>item.evidence.push("Negative control reached the fake Run API only after explicitly removing the auto-run suppression key"));await done();
   }
  }else{
  await wait(()=>state.analysisStage==="complete"&&state.reportAnalysisKey===fixture.textHash,"main cached analysis");
  await wait(()=>phase==="cache_miss"?fixture.read().counts.simulationLookups>=5:state.simulationResult?.runId===fixture.runId,"main cached simulation result");
  await sleep(250);
  await wait(()=>state.simulationAutoRunOnOpen&&state.simulationEnvironment?.installations?.length,"usable auto-run settings / environment");
  document.getElementById("simulationWeatherSelect").value="C:/EPATH160/Weather/fixture.epw";
  const runCallsBefore=fixture.read().counts.forbidden;
  window.dispatchEvent(new CustomEvent("idfAnalyzer:analysisComplete",{detail:{text:store.getDocumentText(),analysisKey:fixture.textHash,stage:"complete"}}));await sleep(75);
  check(fixture.read().counts.forbidden===runCallsBefore&&!state.simulationRunning,"restored document auto-ran despite matching install / weather and autoRunOnOpen=true");
  const keys=["simulationEnergyScopeKind","simulationEnergyZoneName","simulationEnergyPeriod","simulationEnergyService","simulationEnergySelection","simulationEnergyDetailsOpen"];
  const primary=()=>Object.fromEntries(keys.map(key=>[key,state[key]]));
  const capture=()=>history.captureViewSnapshot().panelContexts.simulation;
  const host=document.getElementById("simulationEnergyDashboard");
  const assertPrimary=label=>check(JSON.stringify(Object.keys(state).filter(key=>key.startsWith("simulationEnergy")).sort())===JSON.stringify([...keys].sort()),label+" retained extra Energy primary state: "+Object.keys(state).filter(key=>key.startsWith("simulationEnergy")).join(","));
  const assertResult=label=>check(JSON.stringify(state.simulationResult)===fixture.resultJSON,label+" changed the immutable cached result");
  const assertSnapshot=label=>{const saved=JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument")),context=saved.viewSnapshot?.panelContexts?.simulation||{};check(saved.schemaVersion===4,label+" did not write schema4");check(JSON.stringify(saved.simulationResultRef)===JSON.stringify({textHash:fixture.textHash,runId:fixture.runId}),label+" lost exact compact run reference");check(!["energyView","energyFocusMode","energyZoneFocus","energyServicePathFocus","energyDetailsTab","energyDetailsStage","energyOutputSource","energySankeyMode","energySignMode","energyNodeLimit"].some(key=>key in context),label+" serialized obsolete Energy focus / drawer keys");check(JSON.stringify(Object.keys(context.energyDrawer||{}).sort())===JSON.stringify(["outputSource","stage","tab"]),label+" did not keep drawer details panel-local");check(!JSON.stringify(saved).includes('"nodes"')&&!JSON.stringify(saved).includes('"series"')&&!JSON.stringify(saved).includes('"purposeResults"'),label+" serialized simulation payload into workspace snapshot");check(JSON.stringify(saved).length<20000,label+" snapshot is not compact");};
  const change=(selector,value)=>{const element=host.querySelector(selector);if(!element)throw new Error("Missing real Energy control: "+selector);element.value=value;element.focus();element.dispatchEvent(new Event("change",{bubbles:true}));};
  const selected=()=>view.energyPathGraphForState(state.simulationResult.purposeResults.energyExplanation,state).nodes.find(node=>node.id===state.simulationEnergySelection);
  const saveExpected=()=>fixture.update(item=>{item.expected={primary:primary(),drawer:capture().energyDrawer};});
  const assertExpected=label=>{const expected=fixture.read().expected;check(JSON.stringify(primary())===JSON.stringify(expected.primary),label+" changed six keys: "+JSON.stringify(primary()));check(JSON.stringify(capture().energyDrawer)===JSON.stringify(expected.drawer),label+" changed panel-local drawer: "+JSON.stringify(capture().energyDrawer));};
  assertPrimary(phase);
  if(phase==="initial"){
   assertResult("initial cache load");
   change("[data-simulation-energy-scope]","zone");change("[data-simulation-energy-zone-name]","Office");change("[data-simulation-energy-path-period]","M2");change("[data-simulation-energy-service]","cooling");
   const use=view.energyPathGraphForState(state.simulationResult.purposeResults.energyExplanation,state).nodes.find(node=>node.level==="end_use"&&node.endUse==="cooling");
   click(host.querySelector('[data-energy-path-layout-node="'+CSS.escape(use.id)+'"]'));
   check(host.querySelector("[data-energy-path-inspector]"),"real selected node did not open its inspector");
   click(host.querySelector('[data-energy-path-quality-stage="drivers"]'));
   check(capture().energyDrawer?.stage==="drivers","quality stage did not use panel-local drawer state");
   click(host.querySelector('[data-energy-path-service-kind="output"] [data-energy-path-service-destination]'));
   check(capture().energyDrawer?.tab==="output"&&capture().energyDrawer?.outputSource===fixture.sourceID&&host.querySelector('[data-energy-path-output-request-selected="true"]'),"actual Output action did not select its exact request");
   saveExpected();assertPrimary("handlers");await actions.saveWorkspaceSnapshot();assertSnapshot("initial save");
   fixture.update(item=>{item.phase="settings";item.evidence.push("Real scope / period / service / node / Output actions set Office M2 cooling before Settings");});click(document.getElementById("settingsButton"));
  }else if(phase==="settings_return"){
   assertResult("Settings cold return");assertExpected("Settings cold return");assertSnapshot("Settings navigation");
   check(state.activeResultTab==="simulation"&&selected()&&host.querySelector('[data-energy-path-output-request-selected="true"]'),"cold Settings return did not reconstruct the selected Energy view and Output request");
   const restoredRequest=host.querySelector('[data-energy-path-output-request-selected="true"]');restoredRequest.focus();restoredRequest.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true}));
   const restoredOutputAction=host.querySelector('[data-energy-path-service-kind="output"] [data-energy-path-service-destination]');check(!state.simulationEnergyDetailsOpen&&document.activeElement===restoredOutputAction,"cold Output Escape did not derive and focus the current exact source action");
   click(host.querySelector('[data-energy-path-quality-stage="drivers"]'));saveExpected();
   click(host.querySelector('[data-energy-path-service-kind="hvac"] [data-energy-path-service-destination]'));
   await wait(()=>state.activeResultTab==="hvac"&&state.globalSelection?.entityId===fixture.entityID,"real global HVAC destination");
   assertExpected("HVAC dormant Energy");check(visible(document.getElementById("hvacGraph")),"global HVAC target did not reveal actual HVAC pane");
   fixture.update(item=>{item.phase="tools";item.evidence.push("Real global HVAC selection opened while dormant Energy state remained intact");});click(document.getElementById("toolsButton"));
  }else if(phase==="tools_return"){
   assertResult("Tools cold return");assertExpected("Tools dormant Energy return");assertSnapshot("Tools navigation");
   check(state.activeResultTab==="hvac"&&state.globalSelection?.entityId===fixture.entityID,"Tools cold return lost actual HVAC global selection");
   click(document.querySelector('[data-result-tab="simulation"]'));await sleep(100);assertExpected("return from dormant Energy");check(selected()&&host.querySelector("[data-energy-path-inspector]"),"restored Energy selection is not visible after leaving HVAC");
   await actions.saveWorkspaceSnapshot();const cached=JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument"));
   const legacy={activeResultView:"energy",energyFocusMode:"zone",energyZoneFocus:"Office",energyPeriod:"M2",energyService:"cooling",energySelection:state.simulationEnergySelection,energyView:"sources",energyDetailsTab:"data",energyDetailsStage:"drivers",energySankeyMode:"legacy",energySignMode:"absolute",energyNodeLimit:3};
   cached.schemaVersion=3;cached.activeResultTab="simulation";cached.viewSnapshot.resultTab="simulation";cached.viewSnapshot.globalSelection=null;cached.viewSnapshot.panelContexts.simulation=legacy;cached.panelContexts=cached.viewSnapshot.panelContexts;sessionStorage.setItem("idfAnalyzer.currentDocument",JSON.stringify(cached));
   fixture.update(item=>{item.phase="legacy";item.evidence.push("Cold Tools return restored active HVAC and dormant Office M2 cooling selection");});location.reload();
  }else if(phase==="legacy"){
   assertResult("legacy cache load");check(state.simulationEnergyScopeKind==="zone"&&state.simulationEnergyZoneName==="Office"&&state.simulationEnergyPeriod==="M2"&&state.simulationEnergyService==="cooling"&&selected()&&state.simulationEnergyDetailsOpen,"legacy aliases did not normalize into six primary keys: "+JSON.stringify(primary()));
   check(capture().energyDrawer?.tab==="data"&&capture().energyDrawer?.stage==="drivers","legacy source drawer context was discarded");assertPrimary("legacy migration");
   await actions.saveWorkspaceSnapshot();assertSnapshot("legacy normalized save");saveExpected();const saved=JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument"));saved.simulationResultRef.runId="missing-cache-run";sessionStorage.setItem("idfAnalyzer.currentDocument",JSON.stringify(saved));fixture.update(item=>{item.phase="cache_miss";item.evidence.push("Schema3 aliases normalized and rewrote only schema4 compact state");});location.reload();
  }else if(phase==="cache_miss"){
   check(state.simulationResult===null,"cache miss invented or reused another run");assertExpected("cache miss pending selection");assertPrimary("cache miss");
   check(!host.querySelector("[data-energy-path-layout-node]"),"cache miss rendered another run's graph");
   const saved=JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument"));saved.simulationResultRef.runId=fixture.runId;saved.viewSnapshot.panelContexts.simulation.energySelection="missing-node.epath160";saved.viewSnapshot.panelContexts.simulation.energyDrawer={tab:"output",stage:"",outputSource:"missing-source.epath160"};saved.panelContexts=saved.viewSnapshot.panelContexts;sessionStorage.setItem("idfAnalyzer.currentDocument",JSON.stringify(saved));fixture.update(item=>{item.phase="stale_loaded";item.evidence.push("Cache miss kept pending Energy context without rendering a graph or rerunning analysis / simulation");});location.reload();
  }else if(phase==="stale_loaded"){
   assertResult("rehydrated stale selection");check(state.simulationEnergyScopeKind==="zone"&&state.simulationEnergyZoneName==="Office"&&state.simulationEnergyPeriod==="M2"&&state.simulationEnergyService==="cooling","stale selection cleanup changed valid scope / period / service");check(state.simulationEnergySelection===""&&capture().energyDrawer?.outputSource==="","loaded payload did not clear nonexistent node / source: "+JSON.stringify(capture()));assertPrimary("final hydrated state");
   check(fixture.read().counts.forbidden===0,"cold navigation caused Analyze / Run work");fixture.update(item=>item.evidence.push("Valid payload restored; stale node and Output source cleared only after rehydration; exactly six Energy primary keys"));
   const savedBefore=sessionStorage.getItem("idfAnalyzer.currentDocument"),textBefore=store.getDocumentText(),pathBefore=state.currentFilePath;
   for(const race of["text","path"]){
    const digest=crypto.subtle.digest.bind(crypto.subtle);let release;
    Object.defineProperty(crypto.subtle,"digest",{configurable:true,value:(...args)=>new Promise((resolve,reject)=>{release=()=>digest(...args).then(resolve,reject);})});
    try{const pending=actions.saveWorkspaceSnapshot();await wait(()=>typeof release==="function","controlled workspace hash");if(race==="text")store.setDocumentText(textBefore+"\n! edited during navigation hash");else state.currentFilePath="C:/EPATH160/moved.idf";release();check(await pending===false&&sessionStorage.getItem("idfAnalyzer.currentDocument")===savedBefore,"workspace hash-await "+race+" race saved mixed identities instead of canceling");}
    finally{delete crypto.subtle.digest;store.setDocumentText(textBefore);state.currentFilePath=pathBefore;}
   }
   fixture.update(item=>{item.phase="cache_text_race";item.evidence.push("Hash-await text and path races both canceled save without changing the saved workspace");});location.reload();
  }else throw new Error("Unexpected cold main phase: "+phase);
  }
 }
}catch(error){await done(error);}
</script>`
