package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEPATH152ActualAppServiceLedgerOutputNavigationBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser actual Energy Path service navigation acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	page := readTestFile(t, "frontend/src/index.html")
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		if !strings.Contains(page, script) {
			t.Fatalf("actual app bootstrap changed: %s", script)
		}
		page = strings.Replace(page, script, "", 1)
	}
	page = strings.Replace(page, "</body>", epath142LayoutHTML+epath143RibbonsHTML+epath144ColorsHTML+epath151DriverNavigationHTML+epath152ServiceNavigationHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/epath152-service-navigation.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	windowWidth, windowHeight := 1600, 900
	runBrowser := func(action, query string) ([]byte, error) {
		return exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--force-device-scale-factor=1", fmt.Sprintf("--window-size=%d,%d", windowWidth, windowHeight), "--virtual-time-budget=18000", "--user-data-dir="+t.TempDir(), action, server.URL+"/src/epath152-service-navigation.html?manual=1"+query).CombinedOutput()
	}
	output, err := runBrowser("--dump-dom", "&run152=1")
	if viewport := regexp.MustCompile(`data-epath152-viewport="(\d+)x(\d+)"`).FindSubmatch(output); err == nil && len(viewport) == 3 {
		actualWidth, _ := strconv.Atoi(string(viewport[1]))
		actualHeight, _ := strconv.Atoi(string(viewport[2]))
		if actualWidth != 1600 || actualHeight != 900 {
			if actualWidth < 1200 || actualWidth > 1800 || actualHeight < 600 || actualHeight > 1000 {
				t.Fatalf("unexpected headless viewport: %dx%d", actualWidth, actualHeight)
			}
			windowWidth += 1600 - actualWidth
			windowHeight += 900 - actualHeight
			output, err = runBrowser("--dump-dom", "&run152=1")
		}
	}
	if err != nil {
		t.Fatalf("EPATH152 browser failed: %v\n%s", err, output)
	}
	for _, mode := range []string{"chooser", "ledger", "output"} {
		if screenshot := os.Getenv("EPATH152_SCREENSHOT_" + strings.ToUpper(mode)); screenshot != "" {
			windowWidth, windowHeight = 1600, 900
			if shot, shotErr := runBrowser("--screenshot="+screenshot, "&destination="+mode); shotErr != nil {
				t.Fatalf("EPATH152 %s screenshot failed: %v\n%s", mode, shotErr, shot)
			}
		}
	}
	document := string(output)
	diagnostic := regexp.MustCompile(`(?s)<pre id="epath152-result"[^>]*>(.*?)</pre>`).FindStringSubmatch(document)
	if !strings.Contains(document, `data-epath152-status="passed"`) {
		if len(diagnostic) == 2 {
			t.Fatalf("EPATH152 failed: %s", diagnostic[1])
		}
		t.Fatalf("EPATH152 failed:\n%s", output)
	}
	if len(diagnostic) == 2 {
		t.Log(diagnostic[1])
	}
}

const epath152ServiceNavigationHTML = `<pre id="epath152-result" hidden>pending</pre><script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
const sleep=ms=>new Promise(resolve=>setTimeout(resolve,ms));
document.body.dataset.epath152Viewport=innerWidth+"x"+innerHeight;
try{
 for(let attempt=0;document.body.dataset.epath151Status!=="manual"&&attempt<250;attempt++)await sleep(10);
 if(document.body.dataset.epath151Status!=="manual")throw new Error("actual151 app bootstrap failed: "+document.getElementById("epath151-result")?.textContent);
 const [store,simulation,view,controller,history,navigation,hvacViews,outputResolver]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/views/energy-path-view.js"),import("/src/js/selection-controller.js"),import("/src/js/view-history.js"),import("/src/js/navigation.js"),import("/src/js/views/hvac-views.js"),import("/src/js/energy-path-output-requests.js")]);
 const {state}=store,original=state.simulationResult,originalJSON=JSON.stringify(original),result=JSON.parse(originalJSON),explanation=result.purposeResults.energyExplanation,report=JSON.parse(JSON.stringify(state.report)),semanticNavigation=JSON.parse(JSON.stringify(state.semanticProjection.navigation));
 for(const scope of[explanation,...explanation.zoneResults])scope.periods.push(JSON.parse(JSON.stringify(scope.periods.find(period=>period.id==="M1")).replaceAll("M1","M2")));
 // Actual HeatFlowDataset stores category-major values. Labels, not indexes,
 // identify the selected month; real floor vertices are needed for its plan.
 result.purposeResults.zoneHeatFlow={sourceFile:"eplusout.sql",unit:"W",temperatureUnit:"C",frameCount:4,originalFrameCount:4,labels:["01-01 01:00","01-31 23:00","02-01 01:00","02-28 23:00"],categories:[{id:"internalConvective",label:"Internal convective gains",variableName:"Zone Air Heat Balance Internal Convective Heat Gain Rate",unit:"W",color:"#f59e0b"},{id:"systemAir",label:"HVAC system air transfer",variableName:"Zone Air Heat Balance System Air Transfer Rate",unit:"W",color:"#3b82f6"}],zones:[{name:"Office",values:[[10,20,30,40],[-5,-10,-15,-20]],temperature:[20,21,22,23]},{name:"Lab",values:[[101,102,103,104],[-1,-1,-1,-1]],temperature:[18,19,20,21]}],maxAbs:104,minTemperature:18,maxTemperature:23};
 for(const [zoneName,x]of[["Office",0],["Lab",8]]){
  const existing=report.geometry.surfaces.find(surface=>surface.zoneName===zoneName&&surface.surfaceType==="Floor"),floor=existing||{id:"floor.ledger."+zoneName.toLowerCase(),name:zoneName+" ledger floor",type:"BuildingSurface:Detailed",surfaceType:"Floor",zoneName,storyIndex:0,physicalArea:24};
  floor.vertices=[{x,y:0,z:0},{x:x+6,y:0,z:0},{x:x+6,y:4,z:0},{x,y:4,z:0}];if(!existing)report.geometry.surfaces.push(floor);
 }
 const loops=[
  {id:"loop.raw.air-office",type:"AirLoopHVAC",name:"Office air loop",relatedZones:["Office"]},
  {id:"loop.raw.air-lab",type:"AirLoopHVAC",name:"Lab air loop",relatedZones:["Lab"]},
  {id:"loop.raw.chilled",type:"PlantLoop",name:"Chilled water loop",relatedZones:["Office","Lab"]},
  {id:"loop.raw.hot",type:"PlantLoop",name:"Hot water loop",relatedZones:["Office"]},
  {id:"loop.raw.condenser",type:"CondenserLoop",name:"Condenser water loop",relatedZones:["Office","Lab"]},
 ];
 const loopID=loop=>"loop:"+loop.type.toLowerCase()+":"+loop.name.toLowerCase();
 for(const [index,loop]of loops.entries())Object.assign(loop,{objectIndex:200+index,supplySide:{name:"Supply",branches:[],connectors:[]},demandSide:{name:"Demand",branches:[],connectors:[]},warnings:[]});
 const ref=loop=>({id:loop.id,type:loop.type,name:loop.name,objectIndex:loop.objectIndex});
 const paths=[
  {id:"service.office.cooling.a",zoneName:"Office",serviceKind:"cooling",airLoop:ref(loops[0]),plantLoop:ref(loops[2]),condenserLoop:ref(loops[4])},
  {id:"service.office.cooling.b",zoneName:"Office",serviceKind:"cooling",airLoop:ref(loops[0]),plantLoop:ref(loops[2])},
  {id:"service.lab.cooling",zoneName:"Lab",serviceKind:"cooling",airLoop:ref(loops[1]),plantLoop:ref(loops[2]),condenserLoop:ref(loops[4])},
  {id:"service.office.heating",zoneName:"Office",serviceKind:"heating",airLoop:ref(loops[0]),plantLoop:ref(loops[3])},
  {id:"service.unrelated.cooling",zoneName:"Office",serviceKind:"cooling",airLoop:ref(loops[0])},
 ];
 const targets=new Map();
 const addSemantic=(entityId,kind,targetKind,targetId,label,sourceAnchor)=>{
  const target={view:"hvac",targetKind,targetId,label};
  semanticNavigation.entities.push({id:entityId,kind,label,sourceAnchors:sourceAnchor?[sourceAnchor]:[],viewTargets:[target]});
  targets.set(targetId,{entityId,target,label});
 };
 for(const [index,path]of paths.entries()){
  path.pathType="air_loop";path.servedSubject={kind:"zone",name:path.zoneName,zoneName:path.zoneName};
  path.delivery={id:"delivery."+path.id,objectType:"AirTerminal:SingleDuct:VAV:NoReheat",objectName:path.zoneName+" terminal "+index,objectIndex:220+index,displayName:path.zoneName+" terminal "+index,role:"terminal",mediums:["air"]};
  path.conditioning=[{id:"coil."+path.id,objectType:path.serviceKind==="cooling"?"Coil:Cooling:Water":"Coil:Heating:Water",objectName:path.zoneName+" "+path.serviceKind+" coil "+index,objectIndex:230+index,role:path.serviceKind,mediums:["water","air"]}];
  path.deliveryEquipment={component:path.delivery,deliveryType:"air_terminal",displayFamily:"Air terminal",requiresAirLoop:true,canUsePlantLoop:true,hasInternalCoils:false,mediums:["air"]};
  addSemantic("path:"+path.id,"hvac-path","service-path",path.id,path.zoneName+" "+path.serviceKind+" path "+index);
 }
 for(const loop of loops)addSemantic(loopID(loop),"hvac-loop","hvac-loop",loopID(loop),loop.name,{objectIndex:loop.objectIndex,objectType:loop.type,objectName:loop.name});
 report.hvac={...report.hvac,loops,loopCount:loops.length,airLoopCount:2,plantLoopCount:2,condenserLoopCount:1,zoneRelations:[],nodeUsages:[],warnings:[],serviceModel:{zoneServices:["Office","Lab"].map(zoneName=>({id:"service-zone."+zoneName,zoneName,servedSubject:{kind:"zone",name:zoneName,zoneName},paths:paths.filter(path=>path.zoneName===zoneName)})),couplings:[],navigation:{entities:loops.map(loop=>({id:loopID(loop),kind:"loop",label:loop.name,loopType:loop.type,loopName:loop.name,objectType:loop.type,objectName:loop.name,objectIndex:loop.objectIndex,relatedPathIds:paths.filter(path=>[path.airLoop?.id,path.plantLoop?.id,path.condenserLoop?.id].includes(loop.id)).map(path=>path.id)}))}}};
 const requests=[
  {objectType:"Output:Meter",keyValue:"Electricity:Facility",reportingFrequency:"Monthly",objectIndex:300},
  {objectType:"Output:Meter",keyValue:"Electricity:Facility",reportingFrequency:"Hourly",objectIndex:301},
  {objectType:"Output:Meter",keyValue:"Cooling:Electricity",reportingFrequency:"Monthly",objectIndex:302},
  {objectType:"Output:Meter",keyValue:"Cooling:Electricity",reportingFrequency:"Hourly",objectIndex:303},
  {objectType:"Output:Meter",keyValue:"Heating:NaturalGas",reportingFrequency:"Monthly",objectIndex:304},
  {objectType:"Output:Meter",keyValue:"Fans:Electricity",reportingFrequency:"Monthly",objectIndex:305},
  {objectType:"Output:Meter",keyValue:"Pumps:Electricity",reportingFrequency:"Monthly",objectIndex:306},
  {objectType:"Output:Meter",keyValue:"HeatRejection:Electricity",reportingFrequency:"Monthly",objectIndex:307},
  {objectType:"Output:Meter",keyValue:"NaturalGas:Facility",reportingFrequency:"Monthly",objectIndex:308},
 ];
 result.purposeRunPlan={...result.purposeRunPlan,outputObjects:requests};
 const sourceFor=(id,name,objectIndex)=>({id:"source.private152."+id,sourceType:"sql_meter",isMeter:true,name,reportingFrequency:"Monthly",objectIndex,sourceUnit:"J",normalizedUnit:"kWh"});
 const sources={electricity:sourceFor("facility.electricity","Electricity:Facility",301),gas:sourceFor("facility.gas","NaturalGas:Facility",308),cooling:sourceFor("cooling","Electricity:Cooling",303),cooling_hourly:{...sourceFor("cooling.hourly","Electricity:Cooling",303),reportingFrequency:"Hourly"},heating:sourceFor("heating","NaturalGas:Heating",304),fans:sourceFor("fans","Electricity:Fans",305),pumps:sourceFor("pumps","Electricity:Pumps",306),heat_rejection:sourceFor("heat-rejection","Electricity:HeatRejection",307)};
 explanation.sources.push(...Object.values(sources));
 const graphs=[explanation,...explanation.periods,...explanation.zoneResults.flatMap(zone=>[zone,...zone.periods])];
 for(const payload of graphs){
  const nodeZone=payload.nodes.find(node=>node.zoneName)?.zoneName||"",scopedPaths=paths.filter(path=>!path.id.includes("unrelated")&&(!nodeZone||path.zoneName===nodeZone));
  for(const node of payload.nodes){
   if(node.level==="load"||node.level==="end_use"&&["cooling","heating"].includes(node.endUse))node.relatedPathIds=scopedPaths.filter(path=>path.serviceKind===(node.serviceKind||node.endUse)).map(path=>path.id);
   if(node.level==="end_use"&&sources[node.endUse])node.sourceIds=[sources[node.endUse].id];
   if(node.level==="end_use"&&node.endUse==="cooling")node.sourceIds.push(sources.cooling_hourly.id);
   if(node.level==="carrier")node.sourceIds=[node.carrier==="natural_gas"?sources.gas.id:sources.electricity.id];
   if(node.level==="end_use"&&node.endUse==="fans")node.relatedPathIds=scopedPaths.filter(path=>path.serviceKind==="cooling").map(path=>path.id);
  }
  const fan=payload.nodes.find(node=>node.level==="end_use"&&node.endUse==="fans"),electricity=payload.nodes.find(node=>node.level==="carrier"&&node.carrier==="electricity");
  const originalFanValue=fan?.value;
  if(fan&&electricity)for(const [endUse,value]of[["pumps",originalFanValue*.2],["heat_rejection",originalFanValue*.3]]){
   const node={...fan,id:fan.id.replace("fans",endUse),label:endUse==="pumps"?"Pumps":"Heat rejection",endUse,kind:endUse,sourceIds:[sources[endUse].id],relatedEntityIds:[],relatedPathIds:scopedPaths.filter(path=>path.serviceKind==="cooling").map(path=>path.id),value,displayValue:value,rawValue:value,effectiveValue:value,allocatedValue:value};
   payload.nodes.push(node);payload.links.push({id:"split."+node.id,fromId:node.id,toId:electricity.id,relation:"end_use_to_carrier",domain:"site",basis:"reported_meter",fromValue:value,toValue:value,value,unit:"kWh",fromUnit:"kWh",toUnit:"kWh",period:node.period,sourceIds:node.sourceIds});
  }
  // Split the existing auxiliary total rather than inventing additional
  // facility consumption; the inherited carrier/reconciliation stay coherent.
  if(fan)for(const key of["value","rawValue","effectiveValue","displayValue","allocatedValue"])if(typeof fan[key]==="number")fan[key]*=.5;
  for(const link of payload.links){const from=payload.nodes.find(node=>node.id===link.fromId);if(link.relation==="end_use_to_carrier"&&from){link.sourceIds=from.sourceIds;if(from===fan)Object.assign(link,{fromValue:fan.value,toValue:fan.value,value:fan.value});}}
 }
 state.simulationResult=freeze(result);state.report=report;state.semanticProjection={navigation:semanticNavigation};const rawJSON=JSON.stringify(result),modelJSON=JSON.stringify(report);
 state.analysisDirty={metrics:false,topology:false,profile:false,hvac:false,simulation:false};state.navigationUndoStack=[];state.navigationRedoStack=[];
 let queuedAnalysis=0,backendCalls=0,globalSelections=0;const opened=[];
 window.go.main.App=new Proxy(window.go.main.App,{get(target,key){if(/^(Analyze|RunSimulation|StartSimulation|RequestSimulation)/.test(String(key)))return async()=>{backendCalls++;throw new Error("Unexpected backend "+String(key));};return target[key];}});
 controller.configureSelectionController({state,getNavigationIndex:()=>state.semanticProjection?.navigation||{},getCurrentText:store.getDocumentText,getReportAnalysisKey:()=>state.reportAnalysisKey,isAnalysisCurrent:()=>state.reportAnalyzedText===store.getDocumentText(),getActiveInputView:()=>"input-"+(state.activeInputView||"semantic"),getActivePanelView:()=>state.activeResultTab,
  recordHistory:payload=>{const snapshot=history.captureViewSnapshot();if(payload.previous)snapshot.globalSelection=payload.previous;history.recordViewHistory(snapshot);},
  openView:async(destination,options)=>{opened.push(destination);navigation.switchResultTab(destination,{...options,recordHistory:false});},queueAnalysisTarget:()=>{queuedAnalysis++;},
  onSelectionChange:detail=>{globalSelections++;window.dispatchEvent(new CustomEvent("idfAnalyzer:semanticSelectionChanged",{detail}));}});
 hvacViews.renderHVAC(report.hvac);
 const graph=()=>view.energyPathGraphForState(explanation,state),selected=()=>graph().nodes.find(node=>node.id===state.simulationEnergySelection);
 const model=()=>simulation.simulationEnergyServiceNavigation(selected());
 const candidates=kind=>model().groups.filter(group=>group.kind===kind).flatMap(group=>group.candidates.map(candidate=>({...candidate,groupZoneName:group.zoneName})));
 const buttonFor=candidate=>candidate?document.querySelector('[data-energy-path-service-destination="'+candidate.id+'"]'):null;
 const context=()=>JSON.stringify(history.captureViewSnapshot().panelContexts.simulation);
 const energyContext=()=>JSON.stringify([state.simulationEnergyScopeKind,state.simulationEnergyZoneName,state.simulationEnergyPeriod,state.simulationEnergyService,state.simulationEnergySelection]);
 const revealButton=button=>{for(let ancestor=button?.parentElement;ancestor;ancestor=ancestor.parentElement)if(ancestor.tagName==="DETAILS")ancestor.open=true;button?.focus();};
 const selectNode=async(predicate,{scope="building",period="M2",service="all",details=false}={})=>{
  navigation.switchResultTab("simulation",{recordHistory:false});Object.assign(state,{simulationActiveResultView:"energy",simulationEnergyScopeKind:scope,simulationEnergyZoneName:scope==="zone"?"Office":"",simulationEnergyPeriod:period,simulationEnergyService:service,simulationEnergySelection:"",simulationEnergyDetailsOpen:details,simulationEnergyDetailsTab:"data",simulationEnergyDetailsStage:details?"loads":"",simulationEnergyOutputSource:""});
  const node=graph().nodes.find(predicate);if(!node)throw new Error("Missing actual selected node");state.simulationEnergySelection=node.id;simulation.renderSimulation();await sleep(25);
  check(Boolean(document.querySelector("[data-energy-path-inspector]")),"Actual selected-node inspector missing");return node;
 };
 const activate=async(candidate)=>{
  const button=buttonFor(candidate);check(Boolean(button),"Missing new service destination button "+JSON.stringify(candidate));if(!button)return false;
  check(!button.hasAttribute("data-energy-path-output-source")&&!button.hasAttribute("data-energy-path-hvac-path-id"),"New destination must route through revalidating152 handler, not legacy action");
  revealButton(button);check(document.activeElement===button,"Service destination must be natively focusable");button.click();await sleep(130);return true;
 };
 const back=async(before,depth,label)=>{
  check(state.navigationUndoStack.length===depth+1,"One navigation must record exactly one Energy return point "+label+": "+state.navigationUndoStack.length+" vs "+depth);
  await navigation.undoViewNavigation();await sleep(40);check(state.activeResultTab==="simulation"&&context()===before,"Navigation lost Energy scope/month/service/node/Details context "+label+"\n"+before+"\n"+context());
  check(Boolean(document.querySelector("[data-energy-path-inspector]")),"Returning must restore selected-node inspector "+label);
 };
 const jumpHVAC=async(targetID)=>{
  const candidate=candidates("hvac").find(item=>item.target?.targetId===targetID),before=context(),depth=state.navigationUndoStack.length;
  if(!candidate){check(false,"Missing HVAC target "+targetID+" for "+selected()?.id+" available="+JSON.stringify(candidates("hvac")));return;}
  if(!await activate(candidate))return;
  const expected=targets.get(targetID);check(state.activeResultTab==="hvac"&&state.globalSelection.entityId===expected.entityId,"Real HVAC/global controller did not open exact target "+targetID);
  if(candidate.target.targetKind==="hvac-loop")check(state.activeHVACView==="loop"&&state.activeHVACLoopId===loops.find(loop=>loopID(loop)===targetID)?.id,"Actual loop adapter selected wrong loop "+targetID);
  else check(state.activeHVACContext?.pathId===targetID,"Actual service adapter selected wrong path "+targetID);
  check(document.getElementById("hvacPane").classList.contains("active")&&document.getElementById("hvacGraph").innerHTML.length>100,"Real HVAC graph not rendered");
  await back(before,depth,targetID);evidence.push("HVAC "+targetID);
 };
 const jumpLedger=async(zoneName)=>{
  const candidate=candidates("heat_flow").find(item=>(item.zoneName||item.groupZoneName)===zoneName),before=context(),depth=state.navigationUndoStack.length;
  if(!candidate){check(false,"Missing Ledger Zone "+zoneName+" for "+selected()?.id);return;}
  if(!await activate(candidate))return;
  check(state.activeResultTab==="simulation"&&state.simulationActiveResultView==="zone_heat_flow","Actual Ledger subview not opened");
  check(state.simulationHeatFlowSelectedZone===zoneName&&state.simulationHeatFlowRangeStart===2&&state.simulationHeatFlowRangeEnd===3&&state.simulationHeatFlowFrameIndex===2,"Ledger must select exact Zone and actual February label range, not annual/index-as-month");
  const floor=[...document.querySelectorAll("[data-heat-zone]")].find(element=>element.dataset.heatZone===zoneName);
  check(floor?.classList.contains("selected")&&floor.querySelector("polygon")?.getAttribute("points").split(" ").length===4,"Actual floor-plan Zone geometry not selected");
  const ledger=document.querySelector(".heatflow-inspector"),value=zoneName==="Office"?"30":"103";
  check(ledger?.innerText.includes(zoneName)&&ledger.innerText.includes("02-01 01:00")&&[...ledger.querySelectorAll(".heatflow-ledger-row")].some(row=>row.textContent.includes("Internal convective gains")&&row.textContent.includes(value)),"Ledger did not show category-major February data for "+zoneName);
  const pane=document.querySelector("#simulationPane > .simulation-pane"),chart=ledger?.querySelector(".heatflow-stack-chart");
  const overflow=[...pane.querySelectorAll("*")].filter(element=>element instanceof HTMLElement&&element.getBoundingClientRect().width>0&&element.getBoundingClientRect().right>pane.getBoundingClientRect().right+1).slice(0,5).map(element=>[element.tagName,element.className,Math.round(element.getBoundingClientRect().width),element.scrollWidth]);
  check(pane.scrollWidth<=pane.clientWidth+1&&ledger.scrollWidth<=ledger.clientWidth+1,"Actual Ledger horizontally overflows Simulation/inspector: "+JSON.stringify({pane:[pane.scrollWidth,pane.clientWidth],inspector:[ledger.scrollWidth,ledger.clientWidth],overflow}));
  check(chart&&chart.getBoundingClientRect().right<=ledger.getBoundingClientRect().right+1,"Ledger stack chart exceeds its actual inspector column");
  const focus=getComputedStyle(ledger);check(document.activeElement===ledger&&focus.outlineStyle==="solid"&&parseFloat(focus.outlineWidth)>=1,"Ledger focus must use a deliberate visible solid outline, not irregular UA auto focus");
  await back(before,depth,"ledger "+zoneName);evidence.push("Ledger "+zoneName+" M2=2..3");
 };
 const jumpOutput=async(sourceID,requestIndex)=>{
  const candidate=candidates("output").find(item=>item.sourceId===sourceID),before=energyContext(),depth=state.navigationUndoStack.length;
  if(!candidate){check(false,"Missing Output source "+sourceID+" for "+selected()?.id+" available="+JSON.stringify(candidates("output")));return;}
  if(!await activate(candidate))return;
  check(state.simulationActiveResultView==="energy"&&state.simulationEnergyDetailsOpen&&state.simulationEnergyDetailsTab==="output"&&state.simulationEnergyOutputSource===sourceID,"Output must open actual Energy data drawer at exact source");
  const selectedRow=document.querySelector('[data-energy-path-output-request-selected="true"]');
  check(selectedRow?.dataset.energyPathOutputRequest===outputResolver.energyPathOutputRequestKey(requests[requestIndex],requestIndex),"Output picked wrong meter/frequency/objectIndex "+sourceID);
  check(selectedRow&&selectedRow.getBoundingClientRect().width>0,"Exact Output request is not visible");
  check(document.activeElement===selectedRow||selectedRow?.contains(document.activeElement),"Output focus must reach actual matched request");
  check(state.navigationUndoStack.length===depth&&energyContext()===before,"Output is a local drawer action, not a new global history or Energy selection");
  document.activeElement.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true}));await sleep(25);
  const restoredButton=buttonFor(candidate);check(!state.simulationEnergyDetailsOpen&&energyContext()===before,"Output Escape lost selected Energy scope/month/service/node");
  check(document.activeElement===restoredButton,"Output Escape must focus the exact originating destination button");
  for(let ancestor=restoredButton?.parentElement;ancestor;ancestor=ancestor.parentElement)if(ancestor.tagName==="DETAILS")check(ancestor.open,"Output Escape must reopen containing native multi-source chooser");
  evidence.push("Output "+requests[requestIndex].keyValue+" "+requests[requestIndex].reportingFrequency);
 };
 await selectNode(node=>node.level==="load"&&node.serviceKind==="cooling");
 if(!new URLSearchParams(location.search).has("run152")){
  const destination=new URLSearchParams(location.search).get("destination");
  if(destination==="ledger")await activate(candidates("heat_flow").find(item=>(item.zoneName||item.groupZoneName)==="Office"));
  else if(destination==="output"){
   await selectNode(node=>node.level==="carrier"&&node.carrier==="electricity");await activate(candidates("output").find(item=>item.sourceId===sources.electricity.id));
  }else{
   for(const chooser of document.querySelectorAll("[data-energy-path-service-chooser]"))chooser.open=true;
   document.querySelector("[data-energy-path-service-kind]")?.scrollIntoView({block:"center"});
  }
  document.body.dataset.epath152Status="manual";
 }else{
  check(innerWidth===1600&&innerHeight===900,"Real app content viewport must be1600x900");
  check(opened.length===0,"Selecting a Building load must not choose an arbitrary path");
  const pathCandidates=candidates("hvac").filter(candidate=>candidate.target?.targetKind==="service-path");
  check(pathCandidates.length===3&&!pathCandidates.some(candidate=>candidate.target.targetId.includes("unrelated")||candidate.target.targetId.includes("heating")),"Cooling must use all3 explicit paths, no service/name heuristic");
  const chooser=document.querySelector('[data-energy-path-service-chooser="hvac"]');check(chooser?.tagName==="DETAILS"&&!chooser.open,"Building multi-path navigation requires closed native explicit chooser");
  chooser.open=true;
  const actionText=document.querySelector("[data-energy-path-service-navigation]")?.innerText||"";
  check(actionText&&!/service\.office\.|service\.lab\.|source\.private152\.|loop\.raw\./.test(actionText),"Expanded destination chooser must show readable labels, not internal source/path IDs");
  const pathIdentity=buttonFor(pathCandidates[0])?.querySelector("[data-energy-path-service-model-identity]")?.innerText||"";
  const loopIdentity=buttonFor(candidates("hvac").find(candidate=>candidate.target?.targetId===loopID(loops[0])))?.querySelector("[data-energy-path-service-model-identity]")?.innerText||"";
  check(/service path/i.test(pathIdentity)&&/cooling/i.test(pathIdentity)&&/air loop/i.test(loopIdentity),"HVAC chooser must visibly distinguish typed service paths and AirLoops: "+pathIdentity+" / "+loopIdentity);
  check(document.querySelector("[data-energy-path-inspector]").scrollWidth<=document.querySelector("[data-energy-path-inspector]").clientWidth+1,"Expanded destination choices overflow actual analysis pane");
  chooser.open=false;
  check(new Set(candidates("heat_flow").map(candidate=>candidate.zoneName||candidate.groupZoneName)).size===2,"Building load needs explicit top-Zone Ledger choice");
  await jumpHVAC(paths[1].id);await jumpHVAC(paths[2].id);await jumpLedger("Office");await jumpLedger("Lab");
  await selectNode(node=>node.level==="load"&&node.serviceKind==="cooling",{scope:"zone",service:"cooling",details:true});
  check(candidates("hvac").filter(candidate=>candidate.target?.targetKind==="service-path").every(candidate=>candidate.target.targetId.startsWith("service.office.cooling")),"Zone exact service candidates contain other Zone/service");
  check(candidates("heat_flow").length===1,"Zone load must offer only selected Zone ledger");await jumpHVAC(paths[0].id);await jumpLedger("Office");
  await selectNode(node=>node.level==="load"&&node.serviceKind==="heating",{scope:"zone",service:"heating"});await jumpHVAC(paths[3].id);
  await selectNode(node=>node.level==="end_use"&&node.endUse==="cooling");
  const outputChoices=candidates("output"),monthlyChoice=buttonFor(outputChoices.find(item=>item.sourceId===sources.cooling.id)),hourlyChoice=buttonFor(outputChoices.find(item=>item.sourceId===sources.cooling_hourly.id));
  revealButton(monthlyChoice);
  const monthlyIdentity=monthlyChoice?.querySelector("[data-energy-path-service-request-identity]")?.innerText||"",hourlyIdentity=hourlyChoice?.querySelector("[data-energy-path-service-request-identity]")?.innerText||"";
  check(/Output:Meter/.test(monthlyIdentity)&&/Cooling:Electricity/.test(monthlyIdentity)&&/Monthly/.test(monthlyIdentity)&&/Hourly/.test(hourlyIdentity)&&monthlyIdentity!==hourlyIdentity,"Same-named meter choices must visibly distinguish exact request type/key/frequency: "+monthlyIdentity+" / "+hourlyIdentity);
  await jumpHVAC(loopID(loops[2]));await jumpOutput(sources.cooling.id,2);await jumpOutput(sources.cooling_hourly.id,3);
  await selectNode(node=>node.level==="end_use"&&node.endUse==="heating",{scope:"zone",service:"heating"});await jumpHVAC(loopID(loops[3]));await jumpOutput(sources.heating.id,4);
  await selectNode(node=>node.level==="end_use"&&node.endUse==="fans_pumps");await jumpHVAC(loopID(loops[0]));await jumpHVAC(loopID(loops[2]));await jumpOutput(sources.fans.id,5);await jumpOutput(sources.pumps.id,6);
  await selectNode(node=>node.level==="end_use"&&node.endUse==="hvac_auxiliaries");await jumpHVAC(loopID(loops[4]));await jumpOutput(sources.heat_rejection.id,7);
  await selectNode(node=>node.level==="carrier"&&node.carrier==="electricity");
  check(candidates("hvac").length===0&&candidates("heat_flow").length===0,"Carrier must not fabricate physical service/Zone navigation");
  check(candidates("output").every(candidate=>candidate.sourceId===sources.electricity.id),"Carrier Output must isolate facility meter from end-use meters");await jumpOutput(sources.electricity.id,0);
  const stale=buttonFor(candidates("output")[0]),beforeRejected=state.navigationUndoStack.length;await selectNode(node=>node.level==="load"&&node.serviceKind==="cooling");
  check(await simulation.openSimulationEnergyServiceDestination(stale)===false,"Old other-node Output action must be rejected");
  check(state.navigationUndoStack.length===beforeRejected&&state.simulationActiveResultView==="energy","Rejected action changed history/view");
  // Missing calendar provenance cannot turn a monthly load into annual Ledger.
  const oldLedgerButton=buttonFor(candidates("heat_flow")[0]),uncertain=JSON.parse(rawJSON);uncertain.purposeResults.zoneHeatFlow.labels=["frame0","frame1","frame2","frame3"];state.simulationResult=freeze(uncertain);simulation.renderSimulation();
  check(!candidates("heat_flow").length&&!document.querySelector('[data-energy-path-service-kind="heat_flow"] [data-energy-path-service-destination]'),"Unknown frame labels must disable monthly Ledger action");
  check(await simulation.openSimulationEnergyServiceDestination(oldLedgerButton)===false,"Stale Ledger button must revalidate current calendar evidence");state.simulationResult=result;
  await selectNode(node=>node.level==="carrier"&&node.carrier==="electricity");const exactButton=buttonFor(candidates("output")[0]);
  const duplicate=JSON.parse(rawJSON);duplicate.purposeRunPlan.outputObjects.push({...requests[0],objectIndex:399});duplicate.purposeResults.energyExplanation.sources.find(source=>source.id===sources.electricity.id).objectIndex=null;state.simulationResult=freeze(duplicate);simulation.renderSimulation();
  check(await simulation.openSimulationEnergyServiceDestination(exactButton)===false,"Ambiguous current Output requests must invalidate stale previously-exact button");
  check(!document.querySelector('[data-energy-path-service-kind="output"] [data-energy-path-service-destination]'),"Ambiguous request must not offer enabled Output action");state.simulationResult=result;
  check(queuedAnalysis===0&&backendCalls===0,"Navigation triggered Analyze/Run: "+queuedAnalysis+"/"+backendCalls);
  check(globalSelections>0&&opened.includes("hvac"),"Physical navigation bypassed real global selection controller");
  check(JSON.stringify(result)===rawJSON&&JSON.stringify(original)===originalJSON,"Navigation mutated frozen raw result");
  check(JSON.stringify(report)===modelJSON,"Navigation mutated report model metadata");
  document.body.dataset.epath152Status=failures.length?"failed":"passed";
 }
 document.getElementById("epath152-result").textContent=failures.length?failures.join("\n"):"PASS: "+evidence.length+" real HVAC/Ledger/Output destination roundtrips; exact scope/month/service/source; HVAC/Ledger history and local Output Escape; no Analyze/Run; raw/model immutable. "+evidence.join(" | ");
}catch(error){document.body.dataset.epath152Status="failed";document.getElementById("epath152-result").textContent=String(error?.stack||error);}
</script>`
