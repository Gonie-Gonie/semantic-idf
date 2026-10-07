package frontendchecks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Keep the actual Main and all auxiliary modules intact. Verify the live dialog
// first, then real cold reloads for the independent compact-cache restore cases.
// The bridge contains only fixture responses; no native analysis or simulation runs.
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
	_ = readTranslationSource(t)
	for _, path := range []string{"frontend/src/js/main.js", "frontend/src/js/actions.js", "frontend/src/js/auxiliary-panel.js", "frontend/src/js/auxiliary-context.js", "frontend/src/js/auxiliary-navigation.js", "frontend/src/js/settings.js", "frontend/src/js/settings-client.js", "frontend/src/js/settings-storage.js", "frontend/src/js/tools.js", "frontend/src/js/batch/batch-simulation.js", "frontend/src/js/guide-manual.js", "frontend/src/manual/manifest.json", "frontend/src/manual/metrics.en.md", "frontend/src/styles/auxiliary-panel.css"} {
		readTestFile(t, path)
	}
	pages := map[string]string{}
	for _, name := range []string{"index.html", "settings.html", "guide.html", "tools.html"} {
		page := readTestFile(t, "frontend/src/"+name)
		if name == "index.html" && !strings.Contains(page, `<script type="module" src="./js/main.js"></script>`) {
			t.Fatal("actual main bootstrap is required for this cold-return acceptance")
		}
		if name == "index.html" {
			page = strings.Replace(page, "<head>", "<head>"+bridge, 1)
			page = strings.Replace(page, "</body>", epath160WorkspaceAutomationHTML+"</body>", 1)
		} else {
			// Child bridge/runtime must come from the real host. Only collect errors.
			page = strings.Replace(page, "<head>", "<head>"+epath160AuxiliaryErrorsHTML, 1)
		}
		pages[name] = page
	}
	type browserResult struct {
		Failures []string       `json:"failures"`
		Evidence []string       `json:"evidence"`
		Counts   map[string]int `json:"counts"`
	}
	var mu sync.Mutex
	requests := map[string]int{}
	done := make(chan browserResult, 1)
	type viewportRequest struct {
		Width   int           `json:"width"`
		Page    string        `json:"page"`
		Applied chan struct{} `json:"-"`
	}
	viewports := make(chan viewportRequest)
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
	mux.HandleFunc("/epath160/viewport", func(w http.ResponseWriter, r *http.Request) {
		var request viewportRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || (request.Width != 320 && request.Width != 390 && request.Width != 1600) {
			http.Error(w, "unsupported viewport", http.StatusBadRequest)
			return
		}
		request.Applied = make(chan struct{})
		select {
		case viewports <- request:
		case <-r.Context().Done():
			return
		}
		select {
		case <-request.Applied:
			w.WriteHeader(http.StatusNoContent)
		case <-r.Context().Done():
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// CDP controls the real viewport; Chrome's CLI window size has a minimum
	// that cannot exercise the application's 320/390 px responsive dialog.
	browser := epath201FreshBrowser(t, ctx, chrome, "--enable-unsafe-swiftshader", "--disable-dev-shm-usage", "--disable-features=BackForwardCache")
	browser.call("Page.navigate", map[string]any{"url": server.URL + "/src/index.html"}, nil)
	for {
		select {
		case request := <-viewports:
			browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": request.Width, "height": 900, "deviceScaleFactor": 1, "mobile": false}, nil)
			proof := browser.evaluate(`new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>{const dialog=document.getElementById("auxiliaryPanel"),frame=dialog?.querySelector("iframe:not([hidden])"),doc=frame?.contentDocument;resolve({width:innerWidth,dialogWidth:dialog?.getBoundingClientRect().width,dialogScroll:dialog?.scrollWidth,frameWidth:frame?.clientWidth,contentWidth:doc?.documentElement.scrollWidth,overflow:doc?.documentElement.scrollWidth>frame?.clientWidth?[...doc.body.querySelectorAll("*")].filter(element=>element.getBoundingClientRect().right>frame.clientWidth+1).slice(0,8).map(element=>({tag:element.tagName,id:element.id,class:element.className,right:element.getBoundingClientRect().right})):[]});})))`)
			t.Logf("Actual %s dialog viewport %d: %s", request.Page, request.Width, proof)
			close(request.Applied)
		case result := <-done:
			for _, line := range result.Evidence {
				t.Log(line)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(result.Failures) != 0 {
				t.Fatalf("cold workspace acceptance failed: %s; requests=%v counts=%v", strings.Join(result.Failures, "\n"), requests, result.Counts)
			}
			if requests["index.html"] != 6 || requests["settings.html"] != 1 || requests["guide.html"] != 1 || requests["tools.html"] != 1 {
				t.Fatalf("did not retain cached Settings / Guide / Tools frames or traverse independent cold Main reloads: %v", requests)
			}
			if result.Counts["mainBoots"] != requests["index.html"] || result.Counts["overlayMainBoots"] != 1 || result.Counts["forbidden"] != 0 || result.Counts["simulationLookups"] != 6 || result.Counts["overlayExplicitBatchRuns"] != 1 || result.Counts["autoRunNegativeProbe"] != 1 {
				t.Fatalf("cold restore skipped a document or ran analysis / simulation: requests=%v counts=%v", requests, result.Counts)
			}
			return
		case <-ctx.Done():
			mu.Lock()
			defer mu.Unlock()
			t.Fatalf("cold navigation timed out: requests=%v", requests)
		}
	}
}

const epath160AuxiliaryErrorsHTML = `<script>
window.addEventListener("error",event=>parent.epath160?.update(item=>item.failures.push("auxiliary page error: "+(event.error?.stack||event.message))));
window.addEventListener("unhandledrejection",event=>parent.epath160?.update(item=>item.failures.push("auxiliary rejection: "+(event.reason?.stack||event.reason))));
</script>`

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
  GetSimulationEnvironment:async()=>({energyPlusAvailable:true,installations:[{version:"25.1.0",executablePath:"C:/EPATH160/EnergyPlus/energyplus.exe",weatherDataPath:"C:/EPATH160/Weather"}],weatherFolders:[{source:"test",label:"Fixture weather",files:[{path:"C:/EPATH160/Weather/fixture.epw",name:"fixture.epw"}]}],settings:{autoRunOnOpen:true}}),
  GetStorageUsage:async()=>({scannedAt:"2026-10-08T00:00:00Z",totalBytes:4096,reclaimableBytes:0,protectedBytes:4096,runCount:1,reclaimableRunCount:0,protectedRunCount:1,roots:[],warnings:[]}),
  SetStorageInputPath:async()=>{},
  SelectSimulationInputFiles:async()=>({paths:["C:/EPATH160/a.idf","C:/EPATH160/b.idf","C:/EPATH160/c.idf"]}),
  RunMultipleSimulations:request=>{update(item=>item.counts.overlayExplicitBatchRuns=(item.counts.overlayExplicitBatchRuns||0)+1);window.epath160.batchRequest=request;return new Promise(resolve=>{window.epath160.completeBatch=resolve;});}
 };
 window.go={main:{App:new Proxy(api,{get(target,property){if(Object.hasOwn(target,property))return target[property];if(/^Analyze/.test(String(property))&&window.epath160?.handoffAnalysisAllowed)return()=>{update(item=>item.counts.handoffAnalysis=(item.counts.handoffAnalysis||0)+1);return new Promise(()=>{});};if(/^(Analyze|Run|StartSimulation|RequestSimulation)/.test(String(property)))return async()=>{update(item=>{item.counts.forbidden++;item.evidence.push("Unexpected "+String(property)+" on "+location.pathname+" during "+item.phase);});throw new Error("forbidden work: "+String(property));};return target[property];}})}};
 const listeners=new Map();
 const register=(name,callback)=>{if(!listeners.has(name))listeners.set(name,new Set());listeners.get(name).add(callback);return()=>listeners.get(name)?.delete(callback);};
 window.runtime={EventsOn:register,EventsOnMultiple:register};
 window.epath160={read,write,update,textHash,runId,sourceID,pathID,entityID,resultJSON:JSON.stringify(result),listeners,emit:(name,payload)=>{for(const listener of listeners.get(name)||[])listener(payload);}};
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
    check(fresh.simulationEnergyScopeKind==="building"&&fresh.simulationEnergyZoneName===""&&fresh.simulationEnergyPeriod==="annual"&&fresh.simulationEnergyService==="all"&&fresh.simulationEnergySelection===""&&!fresh.simulationEnergyDetailsOpen&&fresh.energyChartFrequency==="monthly"&&JSON.stringify(fresh.energyDrawer)===JSON.stringify({tab:"data",stage:"",outputSource:""}),"new file registration did not reset Energy defaults and chart frequency and panel-local drawer");
    check(Object.keys(state).filter(key=>key.startsWith("simulationEnergy")).length===7,"new file registration introduced extra Energy primary fields");
    check(await actions.saveWorkspaceSnapshot()===true,"new file snapshot could not be saved");
    const freshSaved=JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument"));check(freshSaved.simulationResultRef===null&&freshSaved.text===sameText&&freshSaved.filename==="different.idf"&&fixture.read().counts.forbidden===0,"new file saved the old result locator or triggered backend work");
    fixture.update(item=>item.evidence.push("Identical-text different-file registration cleared the previous run and saved no result locator"));
    // Negative control: without the saved-input suppression key the exact same
    // usable environment and real lifecycle listener must reach the run API.
    // This local stub records that deliberately requested probe separately.
    state.simulationRunning=false;state.simulationAutoStartedKey="";state.simulationAutoRunOnOpen=true;
    document.getElementById("simulationWeatherSelect").value="C:/EPATH160/Weather/fixture.epw";
    window.go.main.App.RunPurposeSimulationText=()=>{fixture.update(item=>item.counts.autoRunNegativeProbe=(item.counts.autoRunNegativeProbe||0)+1);return new Promise(resolve=>{fixture.finishNegativeRun=resolve;});};
    window.dispatchEvent(new CustomEvent("idfAnalyzer:analysisComplete",{detail:{text:store.getDocumentText(),analysisKey:fixture.textHash,stage:"complete"}}));
    await wait(()=>fixture.read().counts.autoRunNegativeProbe===1,"auto-run negative control");
    fixture.update(item=>item.evidence.push("Negative control reached the fake Run API only after explicitly removing the auto-run suppression key"));
    const auxiliaryHost=window.idfAnalyzerAuxiliaryHost,liveDocument=auxiliaryHost.getDocument(),activeNativeRun=state.simulationActiveRunID,liveReport=state.report,liveResult=state.simulationResult,liveMain=document;
    check(state.simulationRunning&&activeNativeRun&&auxiliaryHost.applyDocument({...liveDocument},{expected:{...liveDocument}})===true&&state.simulationRunning&&state.simulationActiveRunID===activeNativeRun&&state.report===liveReport&&state.simulationResult===liveResult,"no-op Tools handoff changed Main analysis or the pending native run");
    check(auxiliaryHost.applyDocument({...liveDocument,text:"stale replacement"},{expected:{...liveDocument,text:"outdated baseline"}})===false&&store.getDocumentText()===liveDocument.text&&state.simulationActiveRunID===activeNativeRun,"stale Tools expected identity was allowed to replace newer Main input");
    fixture.handoffAnalysisAllowed=true;
    const fixedText=liveDocument.text+"\n! Fixed through the actual Tools host";
    check(auxiliaryHost.applyDocument({...liveDocument,text:fixedText},{expected:{...liveDocument}})===true&&store.getDocumentText()===fixedText&&state.currentFilePath===liveDocument.path&&state.currentFilename===liveDocument.filename&&state.simulationResult===null&&!state.simulationRunning&&state.simulationActiveRunID==="","actual same-file Tools fix did not update Main input and invalidate obsolete simulation state");
    fixture.finishNegativeRun(replacement);await sleep(100);
    check(state.simulationResult===null&&!state.simulationRunning&&state.simulationActiveRunID===""&&document===liveMain,"late native result repopulated the fixed input or rebooted Main");
    await wait(()=>fixture.read().counts.handoffAnalysis>0,"analysis explicitly scheduled by the changed Tools input");
    check(fixture.read().counts.forbidden===0&&fixture.read().counts.autoRunNegativeProbe===1,"Tools input handoff auto-ran simulation or performed work outside its controlled analysis stub");
    fixture.update(item=>item.evidence.push("Actual Main host accepted a no-op without disturbing pending native work, rejected stale expected identity, and invalidated an in-flight run on same-file Tools fix; late native reply stayed rejected"));await done();
   }
  }else{
  await wait(()=>state.analysisStage==="complete"&&state.reportAnalysisKey===fixture.textHash,"main cached analysis");
  await wait(()=>phase==="cache_miss"?fixture.read().counts.simulationLookups>=3:state.simulationResult?.runId===fixture.runId,"main cached simulation result");
  await sleep(250);
  await wait(()=>state.simulationAutoRunOnOpen&&state.simulationEnvironment?.installations?.length,"usable auto-run settings / environment");
  document.getElementById("simulationWeatherSelect").value="C:/EPATH160/Weather/fixture.epw";
  const runCallsBefore=fixture.read().counts.forbidden;
  window.dispatchEvent(new CustomEvent("idfAnalyzer:analysisComplete",{detail:{text:store.getDocumentText(),analysisKey:fixture.textHash,stage:"complete"}}));await sleep(75);
  check(fixture.read().counts.forbidden===runCallsBefore&&!state.simulationRunning,"restored document auto-ran despite matching install / weather and autoRunOnOpen=true");
  const keys=["simulationEnergyScopeKind","simulationEnergyZoneName","simulationEnergyPeriod","simulationEnergyService","simulationEnergySelection","simulationEnergyDetailsOpen","simulationEnergyChartFrequency"];
  const primary=()=>Object.fromEntries(keys.map(key=>[key,state[key]]));
  const capture=()=>history.captureViewSnapshot().panelContexts.simulation;
  const host=document.getElementById("simulationEnergyDashboard");
  const assertPrimary=label=>check(JSON.stringify(Object.keys(state).filter(key=>key.startsWith("simulationEnergy")).sort())===JSON.stringify([...keys].sort()),label+" retained extra Energy primary state: "+Object.keys(state).filter(key=>key.startsWith("simulationEnergy")).join(","));
  const assertResult=label=>check(JSON.stringify(state.simulationResult)===fixture.resultJSON,label+" changed the immutable cached result");
  const assertSnapshot=label=>{const saved=JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument")),context=saved.viewSnapshot?.panelContexts?.simulation||{};check(saved.schemaVersion===4,label+" did not write schema4");check(JSON.stringify(saved.simulationResultRef)===JSON.stringify({textHash:fixture.textHash,runId:fixture.runId}),label+" lost exact compact run reference");check(!["energyView","energyFocusMode","energyZoneFocus","energyServicePathFocus","energyDetailsTab","energyDetailsStage","energyOutputSource","energySankeyMode","energySignMode","energyNodeLimit"].some(key=>key in context),label+" serialized obsolete Energy focus / drawer keys");check(JSON.stringify(Object.keys(context.energyDrawer||{}).sort())===JSON.stringify(["outputSource","stage","tab"]),label+" did not keep drawer details panel-local");check(!JSON.stringify(saved).includes('"nodes"')&&!JSON.stringify(saved).includes('"series"')&&!JSON.stringify(saved).includes('"purposeResults"'),label+" serialized simulation payload into workspace snapshot");check(JSON.stringify(saved).length<20000,label+" snapshot is not compact");};
  const change=(selector,value)=>{const element=host.querySelector(selector);if(!element)throw new Error("Missing real Energy control: "+selector);element.value=value;element.focus();element.dispatchEvent(new Event("change",{bubbles:true}));};
  const selected=()=>view.energyPathGraphForState(state.simulationResult.purposeResults.energyExplanation,state).nodes.find(node=>node.id===state.simulationEnergySelection);
  const saveExpected=()=>fixture.update(item=>{item.expected={primary:primary(),drawer:capture().energyDrawer};});
  const assertExpected=label=>{const expected=fixture.read().expected;check(JSON.stringify(primary())===JSON.stringify(expected.primary),label+" changed Energy keys: "+JSON.stringify(primary()));check(JSON.stringify(capture().energyDrawer)===JSON.stringify(expected.drawer),label+" changed panel-local drawer: "+JSON.stringify(capture().energyDrawer));};
  assertPrimary(phase);
  if(phase==="initial"){
   assertResult("initial cache load");
   change("[data-simulation-energy-scope]","zone");change("[data-simulation-energy-zone-name]","Office");change("[data-simulation-energy-path-period]","M2");
   const use=view.energyPathGraphForState(state.simulationResult.purposeResults.energyExplanation,state).nodes.find(node=>node.level==="end_use"&&node.endUse==="cooling");
   click(host.querySelector('[data-energy-path-layout-node="'+CSS.escape(use.id)+'"]'));
   check(host.querySelector("[data-energy-path-inspector]"),"real selected node did not open its inspector");
   click(host.querySelector('[data-energy-path-quality-stage="drivers"]'));
   check(capture().energyDrawer?.stage==="drivers","quality stage did not use panel-local drawer state");
   click(host.querySelector('[data-energy-path-output-source="'+fixture.sourceID+'"]'));
   check(capture().energyDrawer?.tab==="output"&&capture().energyDrawer?.outputSource===fixture.sourceID&&host.querySelector('[data-energy-path-output-request-selected="true"]'),"actual Output action did not select its exact request");
   saveExpected();assertPrimary("handlers");await actions.saveWorkspaceSnapshot();assertSnapshot("initial save");
   // Observe the actual Three camera without replacing renderer or interaction code.
   const THREE=await import("/src/vendor/three.module.js");
   const project=THREE.PerspectiveCamera.prototype.updateProjectionMatrix;
   THREE.PerspectiveCamera.prototype.updateProjectionMatrix=function(...args){fixture.camera=this;return project.apply(this,args);};
   click(document.querySelector('[data-result-tab="topology"]'));
   await wait(()=>fixture.camera&&document.querySelector(".topology-3d-canvas"),"actual topology renderer and camera");
   const camera=fixture.camera,canvas=document.querySelector(".topology-3d-canvas"),initialCamera=camera.position.toArray();
   canvas.dispatchEvent(new WheelEvent("wheel",{deltaY:100,bubbles:true,cancelable:true}));
   const cameraPosition=JSON.stringify(camera.position.toArray());
   check(cameraPosition!==JSON.stringify(initialCamera),"real wheel action did not establish a nondefault camera");
   const mainDocument=document,editor=document.getElementById("textObjectView"),report=state.report,result=state.simulationResult,energyNode=host.querySelector('[data-energy-path-layout-node="'+CSS.escape(use.id)+'"]'),undoStack=state.navigationUndoStack,globalSelection=state.globalSelection;
   check(editor&&canvas&&camera&&energyNode,"live Main preservation fixture must contain actual editor, renderer and selected Energy DOM");
   const countsBefore={...fixture.read().counts},snapshotBefore=sessionStorage.getItem("idfAnalyzer.currentDocument"),mainURL=location.href;
   const assertLive=label=>{
    check(document===mainDocument&&document.getElementById("textObjectView")===editor&&state.report===report&&state.simulationResult===result&&state.navigationUndoStack===undoStack&&state.globalSelection===globalSelection,label+" replaced Main DOM, report/result, view-history objects or global selection");
    check(document.querySelector(".topology-3d-canvas")===canvas&&fixture.camera===camera&&JSON.stringify(camera.position.toArray())===cameraPosition,label+" reset the actual renderer or zoomed camera");
    check(host.querySelector('[data-energy-path-layout-node="'+CSS.escape(use.id)+'"]')===energyNode,label+" rebuilt the dormant Energy node DOM");
    check(location.href===mainURL&&fixture.read().counts.mainBoots===1&&fixture.read().counts.simulationLookups===countsBefore.simulationLookups&&fixture.read().counts.analysisLookups===countsBefore.analysisLookups&&fixture.read().counts.forbidden===countsBefore.forbidden,label+" navigated Main or requested cache / analysis / simulation work");
    check(sessionStorage.getItem("idfAnalyzer.currentDocument")===snapshotBefore,label+" unexpectedly hashed or rewrote the workspace snapshot");
    assertExpected(label);assertResult(label);
   };
   const frameFor=page=>document.querySelector('iframe[data-auxiliary-page="'+page+'"]');
   const waitFrame=async(page,selector)=>{await wait(()=>document.getElementById("auxiliaryPanel")?.open&&frameFor(page)?.dataset.ready&&frameFor(page).contentDocument.querySelector(selector),"actual "+page+" iframe");const frame=frameFor(page);check(!frame.hidden&&frame.contentDocument.documentElement.dataset.embeddedApp==="true"&&frame.contentWindow.go===window.go&&frame.contentWindow.runtime===window.runtime,page+" did not share the Main callback and progress bridge");return frame;};
   const panelFits=async(frame,label)=>{const panel=document.getElementById("auxiliaryPanel");for(const width of[390,320,1600]){await fetch("/epath160/viewport",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({width,page:label})});await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));check(innerWidth===width,label+" did not use a real "+width+"px viewport");check(panel.getBoundingClientRect().width<=width&&panel.scrollWidth<=panel.clientWidth+1,label+" dialog overflowed at "+width+"px: "+panel.scrollWidth+" / "+panel.clientWidth);check(frame.contentDocument.documentElement.scrollWidth<=frame.clientWidth+1,label+" content overflowed at "+width+"px: "+frame.contentDocument.documentElement.scrollWidth+" / "+frame.clientWidth);}};
   const expectClosed=async(label)=>{await wait(()=>!document.getElementById("auxiliaryPanel").open,label+" closes the dialog");assertLive(label);};
   click(document.getElementById("settingsButton"));
   const settingsFrame=await waitFrame("settings","#settingsForm"),settingsDocument=settingsFrame.contentDocument;
   check(visible(settingsDocument.getElementById("analysisTabOrderList")),"Settings does not expose actual settings controls");
   const font=settingsDocument.getElementById("graphFontSize");font.value="14";font.dispatchEvent(new settingsFrame.contentWindow.Event("input",{bubbles:true}));
   check(settingsDocument.getElementById("settingsForm").dataset.dirty==="true","unsaved Settings edit was not retained as dirty");assertLive("Settings open");
   await panelFits(settingsFrame,"Settings");
   click(settingsDocument.querySelector('a[data-app-auxiliary][href*="guide.html"]'));
   const guideFrame=await waitFrame("guide","#manualChapters a"),guideDocument=guideFrame.contentDocument;
   click(guideDocument.querySelector('a[data-manual-route][href="#metrics"]'));
   await wait(()=>guideDocument.getElementById("metric-catalog"),"actual metrics chapter catalog");
   click(guideDocument.querySelector('a[data-manual-route][href="#metrics/metric-catalog"]'));
   await wait(()=>guideFrame.contentWindow.location.hash==="#metrics/metric-catalog","Guide section route");assertLive("Guide crosslink");
   await panelFits(guideFrame,"Guide");
   const search=guideDocument.getElementById("manualSearch");search.value="cooling";search.dispatchEvent(new guideFrame.contentWindow.Event("input",{bubbles:true}));
   search.dispatchEvent(new guideFrame.contentWindow.KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true}));
   check(document.getElementById("auxiliaryPanel").open&&search.value==="","Guide search Escape should clear its query before closing the dialog");
   guideDocument.body.dispatchEvent(new guideFrame.contentWindow.KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true}));
   await expectClosed("child Escape");
   click(document.getElementById("guideButton"));
   check(await waitFrame("guide","#metric-catalog")===guideFrame&&guideFrame.contentDocument===guideDocument&&guideFrame.contentWindow.location.hash==="#metrics/metric-catalog","cached Guide lost its document identity or current chapter/section");
   click(document.getElementById("auxiliaryPanelClose"));await expectClosed("parent close button");
   click(document.getElementById("settingsButton"));
   check(await waitFrame("settings","#settingsForm")===settingsFrame&&settingsFrame.contentDocument===settingsDocument&&settingsDocument.getElementById("graphFontSize").value==="14"&&settingsDocument.getElementById("settingsForm").dataset.dirty==="true","cached Settings lost its actual unsaved form");
   click(document.querySelector('[data-auxiliary-open="tools"]'));
   const toolsFrame=await waitFrame("tools","#multiSimulationRun"),toolsDocument=toolsFrame.contentDocument;
   click(toolsDocument.querySelector('[data-tools-tab="batch-simulation"]'));
   await wait(()=>toolsDocument.querySelector('[data-tools-panel="batch-simulation"]')?.classList.contains("active"),"actual Tools Batch Simulation tab");
   check(visible(toolsDocument.getElementById("multiSimulationRun"))&&toolsDocument.getElementById("multiSimulationRun").disabled&&visible(toolsDocument.getElementById("multiSimulationWorkers")),"Batch destination does not expose actual non-running controls");
   await wait(()=>fixture.listeners.get("idfAnalyzer:multiSimulationProgress")?.size,"Tools runtime progress registration on parent");assertLive("Tools open");
   await panelFits(toolsFrame,"Tools");
   click(toolsDocument.querySelector("a[data-app-return]"));await expectClosed("child Back to App");
   const documentBefore=window.idfAnalyzerAuxiliaryHost.getDocument();
   store.setDocumentText(documentBefore.text+"\n! Edited while Tools was closed");state.currentFilename="edited-live.idf";state.currentFilePath="C:/EPATH160/edited-live.idf";
   let shown=null;toolsFrame.contentWindow.addEventListener("idfAnalyzer:auxiliaryShown",event=>{shown=event.detail.document;});
   click(document.getElementById("toolsButton"));
   await wait(()=>toolsDocument.getElementById("diagnoseFilename").textContent==="edited-live.idf","cached Tools current Main filename synchronization");
   check(frameFor("tools")===toolsFrame&&toolsFrame.contentDocument===toolsDocument&&shown?.text===store.getDocumentText()&&shown.path===state.currentFilePath&&toolsDocument.getElementById("diagnoseFilename").title===state.currentFilePath,"cached Tools used an old session snapshot instead of current live Main input");
   check(state.simulationResult===result&&fixture.read().counts.forbidden===0,"opening cached Batch Tools analyzed its dormant Diagnose input or replaced current results");
   store.setDocumentText(documentBefore.text);state.currentFilename=documentBefore.filename;state.currentFilePath=documentBefore.path;
   click(document.getElementById("auxiliaryPanelClose"));await expectClosed("cached Tools close");
   click(document.getElementById("toolsButton"));await waitFrame("tools","#multiSimulationRun");
   click(toolsDocument.getElementById("multiSimulationSelectFiles"));await wait(()=>!toolsDocument.getElementById("multiSimulationRun").disabled,"explicit fixture Batch file selection");
   click(toolsDocument.getElementById("multiSimulationRun"));await wait(()=>fixture.batchRequest&&typeof fixture.completeBatch==="function","explicit fake Batch run");
   fixture.emit("idfAnalyzer:multiSimulationProgress",{runId:fixture.batchRequest.runId,completed:1,total:3,message:"fixture-progress-retained",status:"running"});
   check(toolsDocument.getElementById("multiSimulationPercent").textContent==="33%","parent native progress did not reach the actual iframe Batch UI");
   click(document.getElementById("auxiliaryPanelClose"));await expectClosed("running Batch overlay close");
   fixture.emit("idfAnalyzer:multiSimulationProgress",{runId:fixture.batchRequest.runId,completed:2,total:3,message:"fixture-background-progress",status:"running"});
   click(document.getElementById("toolsButton"));await waitFrame("tools","#multiSimulationRun");
   check(toolsDocument.getElementById("multiSimulationPercent").textContent==="67%"&&toolsDocument.getElementById("multiSimulationRun").disabled&&fixture.read().counts.overlayExplicitBatchRuns===1,"cached hidden Batch did not retain progress, or reopening restarted its run");
   fixture.completeBatch({runId:fixture.batchRequest.runId,total:3,completed:3,succeeded:3,failed:0,results:[]});
   await wait(()=>toolsDocument.getElementById("multiSimulationPercent").textContent==="100%"&&!toolsDocument.getElementById("multiSimulationRun").disabled,"explicit fake Batch completion");
   click(document.getElementById("auxiliaryPanelClose"));await expectClosed("completed Batch overlay close");
   check(document.querySelectorAll("#auxiliaryPanel iframe[data-auxiliary-page]").length===3,"host did not retain exactly the three allowed auxiliary frames");
   fixture.update(item=>{item.counts.overlayMainBoots=item.counts.mainBoots;item.evidence.push("Actual Settings/Guide/Tools dialog kept Main DOM, report/result/Energy objects and zoomed THREE camera; cached form/route/progress and current Tools input survived close/reopen without Main boot or Analyze/Run work");});
   THREE.PerspectiveCamera.prototype.updateProjectionMatrix=project;
   click(document.querySelector('[data-result-tab="simulation"]'));await sleep(100);assertExpected("return from dormant Energy");check(selected()&&host.querySelector("[data-energy-path-inspector]"),"live Energy selection is not visible after leaving Topology");
   await actions.saveWorkspaceSnapshot();const cached=JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument"));
   const legacy={activeResultView:"energy",energyFocusMode:"zone",energyZoneFocus:"Office",energyPeriod:"M2",energyService:"cooling",energySelection:state.simulationEnergySelection,energyView:"sources",energyDetailsTab:"data",energyDetailsStage:"drivers",energySankeyMode:"legacy",energySignMode:"absolute",energyNodeLimit:3};
   cached.schemaVersion=3;cached.activeResultTab="simulation";cached.viewSnapshot.resultTab="simulation";cached.viewSnapshot.globalSelection=null;cached.viewSnapshot.panelContexts.simulation=legacy;cached.panelContexts=cached.viewSnapshot.panelContexts;sessionStorage.setItem("idfAnalyzer.currentDocument",JSON.stringify(cached));
   fixture.update(item=>{item.phase="legacy";item.evidence.push("Live auxiliary dialogs retained active Topology camera and dormant Office M2 cooling selection before independent cold restore cases");});location.reload();
  }else if(phase==="legacy"){
   assertResult("legacy cache load");check(state.simulationEnergyScopeKind==="zone"&&state.simulationEnergyZoneName==="Office"&&state.simulationEnergyPeriod==="M2"&&state.simulationEnergyService==="all"&&selected()&&state.simulationEnergyDetailsOpen,"legacy aliases did not normalize into six primary keys: "+JSON.stringify(primary()));
   check(capture().energyDrawer?.tab==="data"&&capture().energyDrawer?.stage==="drivers","legacy source drawer context was discarded");assertPrimary("legacy migration");
   await actions.saveWorkspaceSnapshot();assertSnapshot("legacy normalized save");saveExpected();const saved=JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument"));saved.simulationResultRef.runId="missing-cache-run";sessionStorage.setItem("idfAnalyzer.currentDocument",JSON.stringify(saved));fixture.update(item=>{item.phase="cache_miss";item.evidence.push("Schema3 aliases normalized and rewrote only schema4 compact state");});location.reload();
  }else if(phase==="cache_miss"){
   check(state.simulationResult===null,"cache miss invented or reused another run");assertExpected("cache miss pending selection");assertPrimary("cache miss");
   check(!host.querySelector("[data-energy-path-layout-node]"),"cache miss rendered another run's graph");
   const saved=JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument"));saved.simulationResultRef.runId=fixture.runId;saved.viewSnapshot.panelContexts.simulation.energySelection="missing-node.epath160";saved.viewSnapshot.panelContexts.simulation.energyDrawer={tab:"output",stage:"",outputSource:"missing-source.epath160"};saved.panelContexts=saved.viewSnapshot.panelContexts;sessionStorage.setItem("idfAnalyzer.currentDocument",JSON.stringify(saved));fixture.update(item=>{item.phase="stale_loaded";item.evidence.push("Cache miss kept pending Energy context without rendering a graph or rerunning analysis / simulation");});location.reload();
  }else if(phase==="stale_loaded"){
   assertResult("rehydrated stale selection");check(state.simulationEnergyScopeKind==="zone"&&state.simulationEnergyZoneName==="Office"&&state.simulationEnergyPeriod==="M2"&&state.simulationEnergyService==="all","stale selection cleanup changed valid scope / period / service");check(state.simulationEnergySelection===""&&capture().energyDrawer?.outputSource==="","loaded payload did not clear nonexistent node / source: "+JSON.stringify(capture()));assertPrimary("final hydrated state");
   check(fixture.read().counts.forbidden===0,"cold navigation caused Analyze / Run work");fixture.update(item=>item.evidence.push("Valid payload restored; stale node and Output source cleared only after rehydration; six Energy primary keys plus chart frequency"));
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
}catch(error){await done(error);}
</script>`
