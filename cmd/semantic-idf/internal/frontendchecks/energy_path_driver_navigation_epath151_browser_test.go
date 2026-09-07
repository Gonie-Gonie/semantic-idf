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

func TestEPATH151ActualAppDriverDestinationsAndHistoryBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser actual Energy Path semantic navigation acceptance")
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
	page = strings.Replace(page, "</body>", epath142LayoutHTML+epath143RibbonsHTML+epath144ColorsHTML+epath151DriverNavigationHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/epath151-driver-navigation.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	windowWidth, windowHeight := 1600, 900
	runBrowser := func(action, query string) ([]byte, error) {
		return exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--force-device-scale-factor=1", fmt.Sprintf("--window-size=%d,%d", windowWidth, windowHeight), "--virtual-time-budget=16000", "--user-data-dir="+t.TempDir(), action, server.URL+"/src/epath151-driver-navigation.html?manual=1"+query).CombinedOutput()
	}
	output, err := runBrowser("--dump-dom", "&run151=1")
	if viewport := regexp.MustCompile(`data-epath151-viewport="(\d+)x(\d+)"`).FindSubmatch(output); err == nil && len(viewport) == 3 {
		actualWidth, _ := strconv.Atoi(string(viewport[1]))
		actualHeight, _ := strconv.Atoi(string(viewport[2]))
		if actualWidth != 1600 || actualHeight != 900 {
			if actualWidth < 1200 || actualWidth > 1800 || actualHeight < 600 || actualHeight > 1000 {
				t.Fatalf("unexpected headless viewport: %dx%d", actualWidth, actualHeight)
			}
			windowWidth += 1600 - actualWidth
			windowHeight += 900 - actualHeight
			output, err = runBrowser("--dump-dom", "&run151=1")
		}
	}
	if err != nil {
		t.Fatalf("EPATH151 browser failed: %v\n%s", err, output)
	}
	for _, mode := range []string{"building", "zone"} {
		if screenshot := os.Getenv("EPATH151_SCREENSHOT_" + strings.ToUpper(mode)); screenshot != "" {
			// Capture supplements the independently calibrated real DOM viewport.
			windowWidth, windowHeight = 1600, 900
			if shot, shotErr := runBrowser("--screenshot="+screenshot, "&scope="+mode); shotErr != nil {
				t.Fatalf("EPATH151 %s screenshot failed: %v\n%s", mode, shotErr, shot)
			}
		}
	}
	document := string(output)
	diagnostic := regexp.MustCompile(`(?s)<pre id="epath151-result"[^>]*>(.*?)</pre>`).FindStringSubmatch(document)
	if !strings.Contains(document, `data-epath151-status="passed"`) {
		if len(diagnostic) == 2 {
			t.Fatalf("EPATH151 failed: %s", diagnostic[1])
		}
		t.Fatalf("EPATH151 failed:\n%s", output)
	}
	if len(diagnostic) == 2 {
		t.Log(diagnostic[1])
	}
}

const epath151DriverNavigationHTML = `<pre id="epath151-result" hidden>pending</pre><script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
const sleep=ms=>new Promise(resolve=>setTimeout(resolve,ms));
document.body.dataset.epath151Viewport=innerWidth+"x"+innerHeight;
try{
 for(let attempt=0;document.body.dataset.epath144Status!=="manual"&&attempt<200;attempt++)await sleep(10);
 if(document.body.dataset.epath144Status!=="manual")throw new Error("base real-frame fixture failed");
 const [{state},simulation,view,adapters,controller,history,navigation,profileViews,hvacViews,documentState]=await Promise.all([
  import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/views/energy-path-view.js"),
  import("/src/js/panel-navigation-adapters.js"),import("/src/js/selection-controller.js"),import("/src/js/view-history.js"),import("/src/js/navigation.js"),
  import("/src/js/views/profile-views.js"),import("/src/js/views/hvac-views.js"),import("/src/js/state.js")]);
 await import("/src/js/topology-loader.js");
 const i18n=await import("/src/js/i18n.js");
 const original=state.simulationResult,originalJSON=JSON.stringify(original),result=JSON.parse(originalJSON),explanation=result.purposeResults.energyExplanation;
 const office=explanation.zoneResults[0],lab=JSON.parse(JSON.stringify(office).replaceAll("Office","Lab").replaceAll("office","lab"));
 explanation.zoneResults.push(lab);
 for(const graph of[lab,...lab.periods])for(const node of graph.nodes)for(const field of["value","displayValue","rawValue","effectiveValue","allocatedValue"])if(typeof node[field]==="number")node[field]*=.5;
 const semanticNavigation={entities:[],occurrences:[],byViewTarget:{},bySourceObjectIndex:{}};
 const geometry={zoneCount:2,surfaceCount:0,windowCount:0,zones:[{id:"zone:office",name:"Office",objectIndex:0,storyIndex:0},{id:"zone:lab",name:"Lab",objectIndex:1,storyIndex:0}],surfaces:[],windows:[],stories:[{index:0,name:"Ground story",elevation:0}],constructions:[],topology:{schema:"semantic-idf.thermal-topology/v1",sourceModelHash:"epath151",nodes:[{id:"zone:office",entityId:"zone:office",kind:"zone",label:"Office",storyIndex:0,centroid:{x:0,y:0,z:0}},{id:"zone:lab",entityId:"zone:lab",kind:"zone",label:"Lab",storyIndex:0,centroid:{x:8,y:0,z:0}},{id:"thermal-environment:outdoors",kind:"outdoors",label:"Outdoors"},{id:"thermal-environment:ground",kind:"ground",label:"Ground"}],boundaries:[],openings:[],connections:[],airCouplings:[],zoneSignatures:[],issueLinks:[],adjacencyObservations:[]}};
 const categories=new Map(),targets=new Map();let objectIndex=2;
 const addTarget=(category,zoneName,entityID,kind,viewName,targetKind,targetID,label,sourceAnchor)=>{
  const target={view:viewName,targetKind,targetId:targetID,label},occurrenceID="occurrence.private."+entityID;
  const entity={id:entityID,kind,label,sourceAnchors:sourceAnchor?[sourceAnchor]:[],viewTargets:[target]};
  const occurrence={occurrenceId:occurrenceID,entityId:entityID,contextKind:viewName==="profile"?"zone_profile":"model",path:viewName+"/"+zoneName.toLowerCase()+"/"+category,sourceAnchor,viewTargets:[target]};
  semanticNavigation.entities.push(entity);semanticNavigation.occurrences.push(occurrence);semanticNavigation.byViewTarget[viewName+"|"+targetID]=[occurrenceID];
  if(sourceAnchor)semanticNavigation.bySourceObjectIndex[String(sourceAnchor.objectIndex)]=[occurrenceID];
  const record={category,zoneName,entityID,target,occurrenceID,label};targets.set(targetID,record);
  if(!categories.has(category))categories.set(category,[]);categories.get(category).push(record);return record;
 };
 const addBoundary=(key,zoneName,surfaceType,category,relation="exterior")=>{
  const label=zoneName+" "+key.replaceAll("-"," "),surfaceID="surface.private."+zoneName.toLowerCase()+"."+key,id="boundary.private."+zoneName.toLowerCase()+"."+key;
  const anchor={objectIndex:objectIndex++,objectType:"BuildingSurface:Detailed",objectName:label};
  geometry.surfaces.push({id:surfaceID,name:label,type:anchor.objectType,surfaceType,zoneName,outsideBoundary:relation==="ground"?"Ground":relation.startsWith("interzone")?"Surface":"Outdoors",construction:"Test construction",objectIndex:anchor.objectIndex,storyIndex:0,physicalArea:10,fields:[]});
  geometry.topology.boundaries.push({id,surfaceId:surfaceID,surfaceEntityId:surfaceID,surfaceName:label,surfaceType,ownerZoneId:"zone:"+zoneName.toLowerCase(),boundaryCondition:relation==="ground"?"Ground":relation.startsWith("interzone")?"Surface":"Outdoors",relationKind:relation,targetKind:relation==="ground"?"ground":relation.startsWith("interzone")?"zone":"outdoors",targetId:relation==="ground"?"thermal-environment:ground":relation.startsWith("interzone")?"zone:lab":"thermal-environment:outdoors",constructionName:"Test construction",constructionStatus:"resolved",uValue:.4,hasUValue:true,physicalGrossArea:10,physicalOpaqueArea:10,effectiveGrossArea:10,effectiveOpaqueArea:10,totalUa:4,hasUa:true,openingIds:[],sourceAnchors:[anchor],geometryCheck:{status:"not_applicable"}});
  addTarget(category,zoneName,surfaceID,"surface","topology","thermal_boundary",id,label,anchor);return id;
 };
 const wallA=addBoundary("south-wall","Office","Wall","surface.exterior_walls"),wallB=addBoundary("north-wall","Office","Wall","surface.exterior_walls"),wallLab=addBoundary("west-wall","Lab","Wall","surface.exterior_walls");
 const roof=addBoundary("roof","Office","Roof","surface.roofs"),floor=addBoundary("ground-floor","Office","Floor","surface.ground_floors","ground"),interzone=addBoundary("partition","Office","Wall","surface.interzone","interzone_explicit_surface");
 const windowID="window.private.office",openingID="opening.private.office",windowAnchor={objectIndex:objectIndex++,objectType:"FenestrationSurface:Detailed",objectName:"Office south window"};
 geometry.windows.push({id:windowID,name:windowAnchor.objectName,type:windowAnchor.objectType,surfaceType:"Window",baseSurfaceId:targets.get(wallA).entityID,zoneName:"Office",objectIndex:windowAnchor.objectIndex,storyIndex:0,physicalArea:2});
 geometry.topology.openings.push({id:openingID,windowId:windowID,entityId:windowID,name:windowAnchor.objectName,surfaceType:"Window",baseSurfaceId:targets.get(wallA).entityID,ownerZoneId:"zone:office",physicalArea:2,effectiveArea:2,sourceAnchors:[windowAnchor]});
 addTarget("surface.windows_doors","Office",windowID,"window","topology","fenestration",windowID,windowAnchor.objectName,windowAnchor);
 const mixedID="connection.private.mixed";
 geometry.topology.connections.push({id:mixedID,fromNodeId:"zone:office",toNodeId:"thermal-environment:outdoors",relationKind:"exterior",boundaryIds:[wallA,roof],openingIds:[],surfaceCount:2,openingCount:0,physicalGrossArea:20,physicalOpaqueArea:20,effectiveGrossArea:20,effectiveOpaqueArea:20,totalUa:8,hasUa:true});
 addTarget("surface.exterior_walls","Office",mixedID,"thermal_connection","topology","thermal_connection",mixedID,"Office mixed wall and roof connection");
 for(const [category,key,kind,from]of[["air.interzone","mix","zone_mixing","zone:lab"],["air.infiltration","afn","airflow_network","thermal-environment:outdoors"]]){
  const id="air.private."+key,anchor={objectIndex:objectIndex++,objectType:key==="mix"?"ZoneMixing":"AirflowNetwork:MultiZone:Surface",objectName:key==="mix"?"Lab to Office mixing":"Office infiltration connection"};
  geometry.topology.airCouplings.push({id,entityId:id,objectType:anchor.objectType,objectName:anchor.objectName,objectIndex:anchor.objectIndex,fromNodeId:from,toNodeId:"zone:office",direction:"directed",couplingKind:kind,designFlowRate:.15,unit:"m3/s",scheduleName:"Always On",sourceAnchors:[anchor]});
  geometry.topology.connections.push({id:"connection."+id,fromNodeId:from,toNodeId:"zone:office",relationKind:"air_coupling",airCouplingIds:[id],boundaryIds:[],openingIds:[],surfaceCount:0,openingCount:0});
  addTarget(category,"Office",id,"thermal_air_coupling","topology","thermal_air_coupling",id,anchor.objectName,anchor);
 }
 geometry.surfaceCount=geometry.surfaces.length;geometry.windowCount=geometry.windows.length;
 const dimensionRows=[["occupancy","People","internal.people"],["lighting","Lights","internal.lighting"],["equipment","ElectricEquipment","internal.equipment"],["infiltration","ZoneInfiltration:DesignFlowRate","air.infiltration"],["ventilation","ZoneVentilation:DesignFlowRate","air.mechanical_ventilation"]];
 const metricIDs={occupancy:"people_per_area",lighting:"power_per_area",equipment:"power_per_area",infiltration:"air_changes_per_hour",ventilation:"flow_per_person"};
 const profile={zoneCount:2,itemCount:0,zoneProfiles:[],groups:[],dimensions:dimensionRows.map(([id])=>({id,label:id})),matrix:[],schedules:[],warnings:[],defaultSettings:{enabledDimensions:dimensionRows.map(([id])=>id),displayMetrics:metricIDs,groupingMetrics:metricIDs,numericTolerance:.001,scheduleCompareMode:"name",timeView:"week",scaleMode:"shared",applyBehavior:{defaultMode:"clone",replaceExistingPolicy:"replace"}},graphDataset:{series:[]},parameterCandidates:[]};
 for(const zoneName of["Office","Lab"]){
  const items=[];
  for(const [dimension,objectType,category]of dimensionRows)for(let variant=0;variant<(dimension==="occupancy"&&zoneName==="Office"?2:1);variant++){
   const id="profile.private."+zoneName.toLowerCase()+"."+dimension+"."+variant,objectName=zoneName+" "+dimension+(variant?" visitors":" source"),anchor={objectIndex:objectIndex++,objectType,objectName};
   items.push({id,zoneName,dimension,...anchor,scheduleName:"Always On",schedulePattern:"Constant",scheduleHash:"always",rawMethod:"fixture",rawValue:"0.1",normalized:[{id:metricIDs[dimension],label:dimension,unit:"unit",value:.1,displayValue:"0.1",status:"ok"}],warnings:[]});
   profile.graphDataset.series.push({id:"series."+id,label:zoneName,scopeType:"zone",zoneName,dimension,dimensionLabel:dimension,metricId:metricIDs[dimension],metricLabel:dimension,unit:"unit",designValue:.1,scheduleName:"Always On",scheduleHash:"always",schedulePattern:"Constant",dayMultiplierProfile:Array(24).fill(1),weekMultiplierProfile:Array(168).fill(1),monthMultiplierProfile:Array(12).fill(1),annualMultiplierProfile:Array(8760).fill(1),durationMultiplierProfile:Array(8760).fill(1),sourceItemIds:[id],status:"ok",warnings:[]});
   addTarget(category,zoneName,id,"profile-item","profile","profile-item",id,objectName,anchor);
  }
  profile.zoneProfiles.push({zoneName,zoneObjectIndex:zoneName==="Office"?0:1,items,dimensions:dimensionRows.map(([dimension])=>({dimension,label:dimension})),warnings:[]});profile.itemCount+=items.length;
 }
 const outdoorPath={id:"path.private.office.outdoor-air",serviceKind:"ventilation",pathType:"air_loop",zoneName:"Office",servedSubject:{kind:"zone",zoneName:"Office",name:"Office"},sourceSystem:{id:"oa.system.office",displayName:"Office outdoor-air service",objectType:"AirLoopHVAC:OutdoorAirSystem"},delivery:{id:"oa.mixer.office",role:"outdoor_air",objectType:"OutdoorAir:Mixer",name:"Office outdoor-air mixer"},conditioning:[],airLoop:{id:"oa.loop.office",name:"Office outdoor air loop"}};
 const hvac={loops:[],serviceModel:{zoneServices:[{zoneName:"Office",paths:[outdoorPath]}],couplings:[]}};
 addTarget("air.mechanical_ventilation","Office",outdoorPath.id,"hvac-path","hvac","service-path",outdoorPath.id,"Office outdoor-air service");
 // Report-only/unresolvable records must never authorize the controller.
 profile.zoneProfiles[0].items.push({id:"profile.unresolvable",zoneName:"Office",dimension:"occupancy",objectIndex:999,objectType:"People",objectName:"Unavailable occupancy",normalized:[],warnings:[]});
 const sourceRecords=[];
 for(const [category,records]of categories)for(const zoneName of["Office","Lab"]){
  const exact=records.filter(record=>record.zoneName===zoneName&&record.target.targetKind!=="thermal_connection");
  if(exact.length)sourceRecords.push({id:"source.private151."+category+"."+zoneName.toLowerCase(),name:zoneName+" reported "+category,sourceType:"sql_variable",driverCategory:category,zoneName,reportingFrequency:"Monthly",normalizedUnit:"kWh",relatedEntityIds:exact.map(record=>record.entityID)});
 }
 explanation.sources.push(...sourceRecords);
 for(const [scope,graphs]of[["building",[explanation,...explanation.periods]],["Office",[office,...office.periods]],["Lab",[lab,...lab.periods]]])for(const graph of graphs)for(const node of graph.nodes)if(node.level==="driver"){
  const sources=sourceRecords.filter(source=>source.driverCategory===node.driverCategory&&(scope==="building"||source.zoneName===scope));
  node.sourceIds=sources.map(source=>source.id);node.relatedEntityIds=sources.flatMap(source=>source.relatedEntityIds);
  if(node.driverCategory==="air.mechanical_ventilation")node.relatedPathIds=scope==="Lab"?[]:[outdoorPath.id];
 }
 state.simulationResult=freeze(result);const rawJSON=JSON.stringify(result);
 state.report={geometry,profile,hvac,output:{existing:[]}};state.semanticProjection={navigation:semanticNavigation};
 state.reportAnalysisKey=state.analysisKey="epath151-fixture";state.reportAnalyzedText=documentState.getDocumentText();
 state.analysisDirty={metrics:false,topology:false,profile:false,hvac:false,simulation:false};state.analysisReady={topology:true,profile:true,hvac:true,simulation:true};state.geometryReady=true;
 state.topologyMode="thermal";state.thermalTopologyLayout="network";state.selectedTopologyStory="all";
 state.navigationUndoStack=[];state.navigationRedoStack=[];state.profileSettings=null;state.profileViewCache=new Map();
 let queuedAnalysis=0,backendCalls=0,chooserCalls=0;const selectionEvents=[],openedViews=[];
 window.go.main.App=new Proxy(window.go.main.App,{get(target,key){if(/^(Analyze|RunSimulation|StartSimulation|RequestSimulation)/.test(String(key)))return async()=>{backendCalls++;throw new Error("Unexpected backend mutation "+String(key));};return target[key];}});
 adapters.initializeResultPanelNavigationAdapters();profileViews.initializeProfileControls();hvacViews.initializeHVACControls();
 controller.configureSelectionController({state,getNavigationIndex:()=>state.semanticProjection?.navigation||{},getCurrentText:documentState.getDocumentText,getReportAnalysisKey:()=>state.reportAnalysisKey,isAnalysisCurrent:()=>state.reportAnalyzedText===documentState.getDocumentText(),getActiveInputView:()=>"input-"+(state.activeInputView||"semantic"),getActivePanelView:()=>state.activeResultTab,getCurrentSemanticContext:()=>({occurrenceId:state.semanticCurrentOccurrenceId||"",path:state.semanticCurrentPath||""}),
  recordHistory:payload=>{const snapshot=history.captureViewSnapshot();if(payload.previous)snapshot.globalSelection=payload.previous;history.recordViewHistory(snapshot);},
  openView:async(destination,options)=>{openedViews.push(destination);navigation.switchResultTab(destination,{...options,recordHistory:false});},
  queueAnalysisTarget:()=>{queuedAnalysis++;},chooseSemanticOccurrence:()=>{chooserCalls++;return null;},chooseViewTarget:()=>{chooserCalls++;return null;},
  onSelectionChange:detail=>{selectionEvents.push(detail);window.dispatchEvent(new CustomEvent("idfAnalyzer:semanticSelectionChanged",{detail}));}});
 profileViews.renderProfile(profile);hvacViews.renderHVAC(hvac);
 const graph=()=>view.energyPathGraphForState(explanation,state);
 const inspector=()=>document.querySelector("[data-energy-path-inspector]");
 const checkDriverHeading=label=>{
  const heading=document.querySelector("[data-energy-path-driver-navigation] > h6")?.textContent||"";
  check(heading.includes(label)&&!/(driver\.|surface\.|internal\.)/.test(heading),"Actions heading must use the friendly localized category "+label+": "+heading);
 };
 const selectDriver=async(category,{scope="building",period="M1",service="all"}={})=>{
  navigation.switchResultTab("simulation",{recordHistory:false});state.simulationActiveResultView="energy";
  Object.assign(state,{simulationEnergyScopeKind:scope,simulationEnergyZoneName:scope==="zone"?"Office":"",simulationEnergyPeriod:period,simulationEnergyService:service,simulationEnergySelection:"",simulationEnergyDetailsOpen:false,simulationEnergyDetailsTab:"data",simulationEnergyDetailsStage:"",simulationEnergyOutputSource:""});
  const node=graph().nodes.find(item=>item.driverCategory===category);if(!node)throw new Error("Missing driver "+category);
  state.simulationEnergySelection=node.id;simulation.renderSimulationEnergyDashboard(state.simulationResult);await sleep(30);check(Boolean(inspector()),"Actual driver inspector missing "+category);
  if(category==="surface.exterior_walls")checkDriverHeading("Exterior walls");
  if(category==="internal.people")checkDriverHeading("People");
  return node;
 };
 const navModel=()=>simulation.simulationEnergyDriverNavigation(graph().nodes.find(node=>node.id===state.simulationEnergySelection));
 const candidateFor=targetID=>navModel().groups.flatMap(group=>group.candidates).find(candidate=>candidate.target.targetId===targetID);
 const buttonFor=targetID=>{const candidate=candidateFor(targetID);return candidate?document.querySelector('[data-energy-path-driver-destination="'+candidate.id+'"]'):null;};
 const openChooser=button=>{for(let ancestor=button?.parentElement;ancestor;ancestor=ancestor.parentElement)if(ancestor.tagName==="DETAILS")ancestor.open=true;};
 const context=()=>JSON.stringify(history.captureViewSnapshot().panelContexts.simulation);
 const jumpAndReturn=async(targetID)=>{
  const target=targets.get(targetID),candidate=candidateFor(targetID),button=buttonFor(targetID),before=context(),depth=state.navigationUndoStack.length;
  check(Boolean(button),"Missing actual destination button "+targetID);if(!button)return;
  openChooser(button);button.focus();check(document.activeElement===button,"Destination must be natively focusable "+targetID);button.click();
  for(let count=0;count<120&&(state.activeResultTab!==target.target.view||state.globalSelection.entityId!==target.entityID);count++)await sleep(10);await sleep(80);
  check(state.activeResultTab===target.target.view,"Actual panel not opened "+targetID+" -> "+state.activeResultTab);
  check(state.globalSelection.entityId===target.entityID&&state.globalSelection.occurrenceId===(candidate.occurrenceId||""),"Global controller lost chosen entity/occurrence "+targetID+" expected="+candidate.occurrenceId+" actual="+state.globalSelection.occurrenceId);
  check(state.navigationUndoStack.length===depth+1,"Driver jump needs exactly one Energy return history point "+targetID+": "+state.navigationUndoStack.length+" vs "+depth);
  if(target.target.view==="topology")check([state.thermalTopologySelectedEntityId,state.selectedTopologyEntityId].includes(targetID),"Real Topology adapter did not reveal exact target "+targetID+" "+state.thermalTopologySelectedEntityId);
  if(target.target.targetKind==="thermal_air_coupling"){
   const connection=geometry.topology.connections.find(record=>record.airCouplingIds?.includes(targetID));
   const airEdge=[...document.querySelectorAll("#topologyPane [data-thermal-target-id]")].find(element=>element.dataset.thermalTargetId===connection.id);
   const airRect=airEdge?.getBoundingClientRect();
   check(airRect&&Math.hypot(airRect.width,airRect.height)>0&&airEdge.querySelector(".thermal-edge.air")&&(airEdge.classList.contains("selected")||airEdge.classList.contains("connected")),"Exact coupling's actual connection must be visibly emphasized, not only detail/state; metric="+state.thermalTopologyMetric+" target="+targetID);
  }
  if(target.target.view==="profile")check(state.activeProfileZoneName===target.zoneName&&state.profileSelectedDimensions.includes(profile.zoneProfiles.flatMap(zone=>zone.items).find(item=>item.id===targetID).dimension),"Real Profile adapter wrong Zone/dimension "+targetID);
  if(target.target.view==="hvac")check(state.activeHVACContext?.pathId===targetID,"Real HVAC adapter did not reveal OA path");
  const activePane=document.getElementById(target.target.view+"Pane");check(activePane?.classList.contains("active"),"Destination pane is not visible "+targetID);
  const selectedTarget=[...activePane.querySelectorAll("[data-panel-target-id], [data-thermal-target-id]")].find(element=>(element.dataset.panelTargetId===targetID||element.dataset.thermalTargetId===targetID)&&element.getBoundingClientRect().width>0);
  // Thermal boundaries can share one physical connection glyph. Its real
  // detail pane, not a fabricated second graph glyph, must show the exact item.
  const detailHeading=activePane.querySelector("#topologyDetailsHeading");
  const exactThermalDetails=target.target.view==="topology"&&detailHeading?.getBoundingClientRect().width>0&&detailHeading.textContent.includes(target.label);
  check(Boolean(selectedTarget)||exactThermalDetails,"Actual target DOM/detail absent "+targetID+" heading="+detailHeading?.textContent);
  await navigation.undoViewNavigation();await sleep(50);
  check(state.activeResultTab==="simulation"&&context()===before,"Real history failed to restore Energy scope/month/service/node/drawer "+targetID+"\n"+before+"\n"+context());
  evidence.push(target.target.view+":"+target.label);
 };
 await selectDriver("surface.exterior_walls");
 if(!new URLSearchParams(location.search).has("run151")){
  if(new URLSearchParams(location.search).get("scope")==="zone")await selectDriver("internal.people",{scope:"zone",service:"heating"});
  for(const chooser of document.querySelectorAll("[data-energy-path-driver-chooser]"))chooser.open=true;
  document.querySelector("[data-energy-path-driver-navigation]")?.scrollIntoView({block:"center"});document.body.dataset.epath151Status="manual";
 }else{
  check(innerWidth===1600&&innerHeight===900,"Actual app content viewport must be 1600x900");
  i18n.setLanguage("ko");simulation.renderSimulationEnergyDashboard(state.simulationResult);checkDriverHeading("외벽");
  i18n.setLanguage("en");simulation.renderSimulationEnergyDashboard(state.simulationResult);checkDriverHeading("Exterior walls");
  check(state.activeResultTab==="simulation"&&openedViews.length===0&&!state.globalSelection.entityId,"Selecting aggregate driver must not navigate to an arbitrary source");
  const initialButtons=[...document.querySelectorAll("[data-energy-path-driver-destination]")];
  check(initialButtons.length===4,"Building walls need all 3 exact wall choices plus mixed connection context; got "+initialButtons.length);
  const chooser=document.querySelector('[data-energy-path-driver-chooser="topology"]');check(chooser?.tagName==="DETAILS"&&!chooser.open,"Multiple actual targets need a closed native chooser");
  check(new Set([...document.querySelectorAll("[data-energy-path-driver-zone]")].map(element=>element.dataset.energyPathDriverZone)).size===2,"Building chooser must separate Office and Lab groups");
  const mixed=buttonFor(mixedID);check(mixed?.dataset.energyPathDriverEvidence==="category_context"&&/context/i.test(mixed.textContent),"Mixed wall/roof connection must be labelled context, never wall-only attribution");
  openChooser(mixed);const visible=inspector()?.innerText||"";for(const id of["source.private151",wallA,wallB,mixedID,"occurrence.private."])check(!visible.includes(id),"Technical ID leaked into visible actions "+id);
  check(inspector().scrollWidth<=inspector().clientWidth+1,"Expanded source chooser must not horizontally overflow the real analysis pane");
  const zoneContributions=[...document.querySelectorAll("[data-energy-path-driver-zone-contribution]")];check(zoneContributions.length===2,"Building selected-month whole-Zone contributions must accompany groups");
  await jumpAndReturn(wallB);
  const same=candidateFor(wallB);await controller.selectSemanticEntity({entityId:same.entityId,occurrenceId:same.occurrenceId||"",originView:"simulation"},{recordHistory:false,follow:false});
  const sameAnchor=state.globalSelection.sourceAnchor;state.globalSelection={...state.globalSelection,sourceAnchor:{objectName:sameAnchor.objectName,objectType:sameAnchor.objectType,objectIndex:sameAnchor.objectIndex,nonIdentityAnnotation:"preserve identity"}};
  await jumpAndReturn(wallB);await jumpAndReturn(wallLab);await jumpAndReturn(mixedID);
  for(const [category,targetID]of[["surface.roofs",roof],["surface.ground_floors",floor],["surface.windows_doors",windowID],["surface.interzone",interzone]]){await selectDriver(category,{scope:"zone",service:"cooling"});await jumpAndReturn(targetID);}
  for(const [category,targetID]of[["internal.people","profile.private.office.occupancy.1"],["internal.lighting","profile.private.office.lighting.0"],["internal.equipment","profile.private.office.equipment.0"],["air.infiltration","profile.private.office.infiltration.0"],["air.mechanical_ventilation","profile.private.office.ventilation.0"]]){
   const service=category.startsWith("internal")?"heating":"cooling";await selectDriver(category,{scope:"zone",service});
   if(category==="internal.people"){
    i18n.setLanguage("ko");simulation.renderSimulationEnergyDashboard(state.simulationResult);checkDriverHeading("재실자");
    i18n.setLanguage("en");simulation.renderSimulationEnergyDashboard(state.simulationResult);checkDriverHeading("People");
   }
   check([...document.querySelectorAll("[data-energy-path-driver-zone]")].every(element=>element.dataset.energyPathDriverZone==="Office"),"Zone driver chooser contains unrelated Lab sources "+category);
   check(![...document.querySelectorAll("[data-energy-path-driver-destination]")].some(element=>element.textContent.includes("Unavailable occupancy")),"Unresolvable Profile target was offered");
   await jumpAndReturn(targetID);
  }
  await selectDriver("air.interzone",{scope:"zone",service:"cooling"});state.thermalTopologyMetric="topology";await jumpAndReturn("air.private.mix");
  await selectDriver("air.infiltration",{scope:"zone",service:"cooling"});state.thermalTopologyMetric="area";await jumpAndReturn("air.private.afn");
  await selectDriver("air.mechanical_ventilation",{scope:"zone",service:"cooling"});await jumpAndReturn(outdoorPath.id);
  // Old captured buttons cannot authorize another driver or a removed target.
  await selectDriver("surface.exterior_walls");const stale=buttonFor(wallA),beforeView=state.activeResultTab,beforeHistory=state.navigationUndoStack.length;
  await selectDriver("internal.people",{scope:"zone",service:"heating"});check(await simulation.openSimulationEnergyDriverDestination(stale)===false,"Stale other-driver button must fail closed");
  check(state.activeResultTab===beforeView&&state.navigationUndoStack.length===beforeHistory,"Rejected stale destination changed view/history");
  const validPeopleButton=buttonFor("profile.private.office.occupancy.1"),savedProjection=state.semanticProjection;
  state.semanticProjection={navigation:{entities:[],occurrences:[],byViewTarget:{}}};simulation.renderSimulationEnergyDashboard(state.simulationResult);
  check(!document.querySelector("[data-energy-path-driver-destination]"),"Missing semantic index must not fabricate Profile destination");
  check(document.querySelector('[data-energy-path-driver-navigation="unavailable"] button[disabled]'),"Unavailable context needs a native disabled action");
  check(await simulation.openSimulationEnergyDriverDestination(validPeopleButton)===false,"Removed semantic target must fail closed even for the same selected driver");
  check(/unavailable|not available|no matching|no verified[\s\S]*target is available/i.test(inspector()?.innerText||""),"Missing target needs an honest visible reason: "+inspector()?.innerText);state.semanticProjection=savedProjection;
  check(chooserCalls===0,"Explicit destination must not invoke a second arbitrary global chooser");
  check(queuedAnalysis===0&&backendCalls===0,"Navigation must not Analyze or rerun simulation: "+queuedAnalysis+"/"+backendCalls);
  check(JSON.stringify(result)===rawJSON&&JSON.stringify(original)===originalJSON,"Driver navigation mutated raw simulation data");
  check(selectionEvents.some(event=>event.options?.originView==="simulation"),"Real global selection controller was bypassed");
  document.body.dataset.epath151Status=failures.length?"failed":"passed";
 }
 document.getElementById("epath151-result").textContent=failures.length?failures.join("\n"):"PASS: real global controller and Topology/Profile/HVAC adapters; explicit category/Zone/source choices; "+evidence.length+" actual destination/history roundtrips; Analyze/Run=0; frozen raw payload unchanged. "+evidence.join(" | ");
}catch(error){document.body.dataset.epath151Status="failed";document.getElementById("epath151-result").textContent=String(error?.stack||error);}
</script>`
