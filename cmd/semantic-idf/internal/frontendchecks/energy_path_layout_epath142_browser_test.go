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

func TestEPATH142ActualAppFrameEnergyPathLayoutBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser actual app frame acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	// Use the actual index and complete stylesheet cascade. Only the desktop
	// bootstrap is replaced; the balanced workspace and run setup remain real.
	page := readTestFile(t, "frontend/src/index.html")
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		if !strings.Contains(page, script) {
			t.Fatalf("actual app bootstrap changed: %s", script)
		}
		page = strings.Replace(page, script, "", 1)
	}
	page = strings.Replace(page, "</body>", epath142LayoutHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/epath142-layout.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	windowWidth, windowHeight := 1600, 900
	runBrowser := func(action, url string) ([]byte, error) {
		return exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--force-device-scale-factor=1", fmt.Sprintf("--window-size=%d,%d", windowWidth, windowHeight), "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), action, url).CombinedOutput()
	}
	output, err := runBrowser("--dump-dom", server.URL+"/src/epath142-layout.html")
	// Windows Chrome subtracts native window furniture even in headless mode.
	// Calibrate the real content viewport, never the app's CSS or panel width.
	if viewport := regexp.MustCompile(`data-epath142-viewport="(\d+)x(\d+)"`).FindSubmatch(output); err == nil && len(viewport) == 3 {
		actualWidth, _ := strconv.Atoi(string(viewport[1]))
		actualHeight, _ := strconv.Atoi(string(viewport[2]))
		if actualWidth != 1600 || actualHeight != 900 {
			if actualWidth < 1200 || actualWidth > 1800 || actualHeight < 600 || actualHeight > 1000 {
				t.Fatalf("unexpected headless viewport cannot be safely calibrated: %dx%d", actualWidth, actualHeight)
			}
			windowWidth += 1600 - actualWidth
			windowHeight += 900 - actualHeight
			output, err = runBrowser("--dump-dom", server.URL+"/src/epath142-layout.html")
		}
	}
	if err != nil {
		t.Fatalf("EPATH142 browser failed: %v\n%s", err, output)
	}
	document := string(output)
	if screenshot := os.Getenv("EPATH142_SCREENSHOT"); screenshot != "" {
		// Capture applies its own final viewport after script execution, unlike
		// dump-dom. This image supplements the calibrated DOM assertions; it
		// never substitutes for their clipping / viewport evidence.
		windowWidth, windowHeight = 1600, 900
		shot, shotErr := runBrowser("--screenshot="+screenshot, server.URL+"/src/epath142-layout.html?manual=1")
		if shotErr != nil {
			t.Fatalf("EPATH142 screenshot failed: %v\n%s", shotErr, shot)
		}
	}
	if !strings.Contains(document, `data-epath142-status="passed"`) {
		if start := strings.Index(document, `<pre id="epath142-result"`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH142 failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH142 failed:\n%s", output)
	}
	if diagnostics := regexp.MustCompile(`(?s)<pre id="epath142-result"[^>]*>(.*?)</pre>`).FindStringSubmatch(document); len(diagnostics) == 2 {
		t.Log(diagnostics[1])
	}
}

// This is an injection fragment, not a substitute app shell. The manual QA
// server applies the same two-script replacement to the checked-in index.
const epath142LayoutHTML = `<pre id="epath142-result" hidden>pending</pre>
<script>window.go={main:{App:{GetSimulationEnvironment:async()=>({installations:[{version:"25.1",executablePath:"C:/fixtures/EnergyPlus/energyplus.exe"}],weatherFolders:[]})}}};window.runtime={EventsOn(){}};</script>
<script type="module">
const failures=[],evidence=[];
document.body.dataset.epath142Viewport=innerWidth+"x"+innerHeight;
const check=(condition,message)=>{if(!condition)failures.push(message);};
const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
const level={status:"complete",found:12,total:12};
const quality={drivers:level,loads:level,endUses:level,carriers:level,ratios:level,driverToLoadClosedPct:100,endUseToCarrierClosedPct:100,driverToLoadStatus:"complete",endUseToCarrierStatus:"complete",zoneAllocatedPct:100,unassignedPct:0,zoneAllocationStatus:"complete"};
const categories=["surface.exterior_walls","surface.roofs","surface.ground_floors","surface.windows_doors","surface.interzone","air.infiltration","air.mechanical_ventilation","air.interzone","internal.people","internal.lighting","internal.equipment","interzone.transfer"];
const longLabel="Cooling demand from occupied perimeter rooms and adjoining internal zones with unusually long equipment descriptions";
const sourceRecords=new Map();
const makeGraph=(scope,period,scale)=>{
 const suffix=scope.kind==="zone"?"office":"building",nodes=[],links=[];
 const add=(id,level,value,extra={})=>{
  const sourceID="source."+id+"."+suffix;
  sourceRecords.set(sourceID,{id:sourceID,sourceType:"sql_variable",name:id+" Energy",keyValue:"Office",zoneName:"Office",reportingFrequency:"Monthly",sourceUnit:"J",normalizedUnit:"kWh"});
  const node={id:id+"."+suffix,level,label:id,value:value*scale,rawValue:value*scale,effectiveValue:value*scale,allocatedValue:value*scale,unit:"kWh",period,aggregationBasis:"model_total",sourceIds:[sourceID],...(scope.kind==="zone"?{zoneName:"Office"}:{}),...extra};nodes.push(node);return node;
 };
 const link=(from,to,value,relation,extra={})=>{const item={id:relation+"."+from.id+"."+to.id,fromId:from.id,toId:to.id,fromValue:value*scale,toValue:value*scale,fromUnit:"kWh",toUnit:"kWh",relation,period,sourceIds:[...new Set([...from.sourceIds,...to.sourceIds])],...extra};links.push(item);return item;};
 const drivers=categories.map((category,index)=>add("driver."+category,"driver",10,{driverCategory:category,serviceKind:index<8?"cooling":"heating",scaleDomain:"thermal",basis:"heat_balance_share",allocationApplied:true}));
 const cooling=add("load.cooling","load",80,{label:longLabel,serviceKind:"cooling",scaleDomain:"thermal",latentShare:.2});
 const heating=add("load.heating","load",40,{label:"Heating load",serviceKind:"heating",scaleDomain:"thermal"});
 drivers.forEach((node,index)=>link(node,index<8?cooling:heating,10,"driver_to_load",{serviceKind:node.serviceKind,basis:"heat_balance_share"}));
 const coolUse=add("end_use.cooling","end_use",20,{label:"Cooling equipment",endUse:"cooling",serviceKind:"cooling",scaleDomain:"site"});
 const heatUse=add("end_use.heating","end_use",50,{label:"Heating equipment",endUse:"heating",serviceKind:"heating",scaleDomain:"site"});
 link(cooling,coolUse,80,"load_to_end_use",{toValue:20*scale,serviceKind:"cooling",ratio:4,ratioKind:"coefficient_of_performance"});
 link(heating,heatUse,40,"load_to_end_use",{toValue:50*scale,serviceKind:"heating",ratio:0.8,ratioKind:"equipment_efficiency"});
 const direct=["fans","lighting","equipment","water_systems","refrigeration","other"].map(endUse=>add("end_use."+endUse,"end_use",10,{endUse,serviceKind:endUse==="fans"?"cooling":"",scaleDomain:"site",basis:scope.kind==="zone"?"direct_zone_energy":"reported_variable",meterHierarchyLevel:scope.kind==="zone"?"zone_direct_use":"end_use_total"}));
 const electricity=add("carrier.electricity","carrier",75,{label:"Electricity",carrier:"electricity",scaleDomain:"site",basis:"reported_meter",meterHierarchyLevel:scope.kind==="zone"?"zone_total":"facility_total"});
 const gas=add("carrier.natural_gas","carrier",55,{label:"Natural gas",carrier:"natural_gas",scaleDomain:"site",basis:"reported_meter",meterHierarchyLevel:scope.kind==="zone"?"zone_total":"facility_total"});
 link(coolUse,electricity,20,"end_use_to_carrier",{serviceKind:"cooling",basis:"reported_variable"});link(heatUse,gas,50,"end_use_to_carrier",{serviceKind:"heating",basis:"reported_variable"});
 direct.forEach(node=>{link(node,electricity,node.endUse==="water_systems"?5:10,"end_use_to_carrier",{basis:"reported_variable"});if(node.endUse==="water_systems")link(node,gas,5,"end_use_to_carrier",{basis:"reported_variable"});});
 for(const endUse of["lighting","equipment"]){const driver=drivers.find(n=>n.driverCategory==="internal."+endUse),use=direct.find(n=>n.endUse===endUse);link(driver,use,10,"source_correspondence",{basis:"reported_correspondence",serviceKind:"heating"});}
 const completeness={heatBalance:level,load:level,energyUse:level,mappedPercent:100};
 const summary={schema:"semantic-idf.energy-explanation-summary/v2",scope,period,quality,completeness,drivers:nodes.filter(n=>n.level==="driver"),loads:nodes.filter(n=>n.level==="load"),endUses:nodes.filter(n=>n.level==="end_use"),carriers:nodes.filter(n=>n.level==="carrier")};
 return {id:period,period,nodes,links,quality,summary,completeness};
};
const buildingScope={kind:"building",aggregationBasis:"model_total"},zoneScope={kind:"zone",zoneName:"Office",aggregationBasis:"model_total"};
const building=makeGraph(buildingScope,"annual",1),zone=makeGraph(zoneScope,"annual",.5);
const periods=[makeGraph(buildingScope,"M1",.25)],zonePeriods=[makeGraph(zoneScope,"M1",.125)];
const explanation={schema:"semantic-idf.energy-explanation/v2",scope:buildingScope,...building,periods,zoneResults:[{scope:zoneScope,...zone,periods:zonePeriods}],sources:[...sourceRecords.values()]};
const result=freeze({runId:"epath142-completed",status:"succeeded",purposeResults:{energyExplanation:explanation,energyExplanationSummary:building.summary},purposeRunPlan:{outputObjects:[]},series:[]});
const originalJSON=JSON.stringify(result);
try{
 const [store,simulation,layout,navigation,view]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/layout.js"),import("/src/js/navigation.js"),import("/src/js/views/energy-path-view.js")]);
 const {state}=store;
 store.setDocumentText("Version, 25.1;\nBuilding, EPATH142;");
 layout.initializeWorkspaceSplitter();simulation.initializeSimulationControls();
 await new Promise(resolve=>setTimeout(resolve,0));
 Object.assign(state,{report:null,simulationRunning:false,simulationResult:result,simulationProgress:{status:"succeeded",percent:100,message:"Completed"},simulationActiveResultView:"energy"});
 simulation.restoreSimulationEnergyWorkspaceContext({simulationEnergyScopeKind:"building",simulationEnergyZoneName:"",simulationEnergyPeriod:"annual",simulationEnergyService:"all",simulationEnergySelection:"",simulationEnergyDetailsOpen:false,energyDrawer:{tab:"data",stage:"",outputSource:""}});
 navigation.switchResultTab("simulation",{recordHistory:false});simulation.renderSimulation();
 const host=document.getElementById("simulationEnergyDashboard"),setup=document.getElementById("simulationRunSetup");
 const change=(selector,value)=>{const control=host.querySelector(selector);check(Boolean(control),"missing control "+selector);if(control){control.focus();control.value=value;control.dispatchEvent(new Event("change",{bubbles:true}));}};
 const graph=()=>view.energyPathGraphForState(explanation,state);
 const button=id=>[...host.querySelectorAll("[data-energy-path-layout-node]")].find(node=>node.dataset.energyPathLayoutNode===id);
 const key=node=>[node.level,node.driverCategory||node.endUse||node.carrier||node.serviceKind].join(":");
 const geometry=()=>{const canvas=host.querySelector("[data-energy-path-canvas]").getBoundingClientRect();return Object.fromEntries(graph().nodes.map(node=>{const r=button(node.id)?.getBoundingClientRect();return[key(node),r?[r.x-canvas.x,r.y-canvas.y,r.width,r.height]:null];}));};
 if(new URLSearchParams(location.search).get("manual")==="1"){
  document.body.dataset.epath142Status="manual";document.getElementById("epath142-result").textContent="Actual index, balanced workspace, completed run. Inspect Building / Zone geometry, direct lane, long load title, and run setup disclosure.";
 }else{
 check(innerWidth===1600&&innerHeight===900,"fixture did not use real 1600x900 application viewport: "+innerWidth+"x"+innerHeight);
 const workspace=document.querySelector(".workspace"),analysis=document.querySelector(".analysis-panel");
 check(analysis.getBoundingClientRect().width>=700&&analysis.getBoundingClientRect().width<=850,"actual default balanced analysis pane was bypassed: "+analysis.getBoundingClientRect().width);
 check(workspace.querySelector(".editor-panel")&&document.getElementById("workspaceSplitter"),"real editor / splitter app stack missing");
 check(setup&&!setup.open&&document.querySelector(".simulation-progress-card")?.getBoundingClientRect().height>0,"completed setup did not collapse while retaining progress");
 check(host.querySelectorAll("[data-energy-path-stage]").length===4&&host.querySelectorAll("[data-energy-path-layout]").length===1,"real dashboard lacks one four-column canvas");
 check(host.querySelectorAll("[data-energy-path-kpi]").length===4,"real result stack lost its four summary cards");
 check(!host.querySelector(".energy-path-conversion-flow,.energy-path-auxiliary-flow"),"default graph still appends duplicate conversion / auxiliary card lists");
 const projected=graph(),counts=Object.fromEntries(["driver","load","end_use","carrier"].map(level=>[level,projected.nodes.filter(n=>n.level===level).length]));
 check(JSON.stringify(counts)===JSON.stringify({driver:12,load:2,end_use:8,carrier:2}),"material categories disappeared from real graph: "+JSON.stringify(counts));
 check(host.querySelectorAll("[data-energy-path-layout-node]").length===24,"canvas omitted or duplicated graph nodes");
 const canvas=host.querySelector("[data-energy-path-canvas]"),canvasRect=canvas.getBoundingClientRect(),band=host.querySelector('[data-energy-path-lane-band="direct"]'),bandRect=band?.getBoundingClientRect();
 check(band&&bandRect.height>0,"direct / auxiliary lower lane is absent");
 for(const node of projected.nodes){
  const element=button(node.id),rect=element?.getBoundingClientRect();
  check(element?.tagName==="BUTTON"&&element.type==="button","node is not a native keyboard button: "+node.id);
  if(!rect)continue;
  const expectedLane=node.level==="driver"?"thermal":node.level==="carrier"?"shared":node.level==="load"||["cooling","heating"].includes(node.endUse)?"main":"direct";
  check(element.dataset.energyPathLane===expectedLane,"node assigned wrong lane: "+node.id+" "+element.dataset.energyPathLane);
  check(rect.width>0&&rect.height>0&&rect.left>=canvasRect.left-1&&rect.right<=canvasRect.right+1&&rect.top>=canvasRect.top-1&&rect.bottom<=canvasRect.bottom+1,"node escaped canvas: "+node.id+" "+JSON.stringify(rect.toJSON()));
  check(rect.left>=0&&rect.right<=innerWidth+1&&rect.top>=0&&rect.bottom<=innerHeight+1,"default node clipped below real screen: "+node.id+" bottom="+rect.bottom+" viewport="+innerHeight);
  const hit=document.elementFromPoint(rect.left+rect.width/2,rect.top+rect.height/2);
  check(Boolean(hit&&element.contains(hit)),"node center hidden by an ancestor / overlapping content: "+node.id);
  if(expectedLane==="direct")check(rect.top>=bandRect.top-1&&rect.left>=bandRect.left-1,"direct node was forced through thermal lane: "+node.id);
  if(node.level==="end_use"&&expectedLane==="main")check(rect.bottom<=bandRect.top+1,"HVAC conversion end use overlaps direct lane: "+node.id);
  const label=element.querySelector(".energy-path-node-label"),style=label&&getComputedStyle(label);
  check(label&&parseInt(style.webkitLineClamp,10)===2&&label.getBoundingClientRect().height<=2*parseFloat(style.lineHeight)+1,"node label is not limited to two lines: "+node.id);
  check(element.title.includes(node.label||node.kind||node.id)&&element.title.includes("kWh")&&element.getAttribute("aria-label")===element.title,"full label and energy unit missing from tooltip / accessible name: "+node.id);
  if(node.level==="load"&&node.serviceKind==="cooling")check(element.getAttribute("aria-label").includes("Latent 20%")&&element.querySelector("[data-energy-path-load-latent-badge]"),"explicit accessible label hides important latent-load badge");
 }
 for(const element of[document.documentElement,workspace,analysis,document.getElementById("simulationPane"),host,canvas])check(element.scrollWidth<=element.clientWidth+1,"horizontal overflow in "+(element.id||element.className||element.tagName)+": "+element.scrollWidth+"/"+element.clientWidth);
 const pane=document.querySelector("#simulationPane > .simulation-pane");
 check(pane.scrollHeight<=pane.clientHeight+1,"completed default Energy view still requires vertical scroll: "+pane.scrollHeight+"/"+pane.clientHeight);
 for(const selector of[".energy-path-view","[data-energy-path-quality-line]",".energy-path-domain-legend"]){const element=host.querySelector(selector),r=element?.getBoundingClientRect();check(r&&r.top>=0&&r.bottom<=innerHeight+1,"default graph / quality / legend clipped outside viewport: "+selector+" bottom="+r?.bottom);}
 const columns=[...host.querySelectorAll("[data-energy-path-stage]")].map(n=>n.getBoundingClientRect()),divider=host.querySelector("[data-energy-path-divider]");
 check(columns.every((rect,index)=>index===0||rect.left>columns[index-1].left),"four columns lost left-to-right order");
 check(divider&&/Equipment conversion/i.test(divider.textContent)&&divider.getBoundingClientRect().left>=columns[1].right-1&&divider.getBoundingClientRect().left<=columns[2].left+1,"equipment boundary not between thermal load and end-use columns");
 check(bandRect.left>=columns[2].left-1,"direct band incorrectly starts in a thermal column");
 const buildingGeometry=geometry();
 evidence.push("viewport "+innerWidth+"x"+innerHeight+", analysis width "+analysis.getBoundingClientRect().width+", canvas "+Math.round(canvasRect.width)+"x"+canvasRect.height+", bottom "+canvasRect.bottom);
 evidence.push([".topbar",".analysis-panel > .tabs",".simulation-run-setup",".simulation-progress-card","#simulationResultTabs","#simulationEnergyDashboard",".energy-path-kpis",".energy-path-controls","[data-energy-path-layout]"].map(selector=>{const r=document.querySelector(selector)?.getBoundingClientRect();return selector+":"+(r?Math.round(r.top)+"+"+Math.round(r.height):"missing");}).join("; "));
 check(projected.relations?.length===2&&!projected.links.some(link=>link.relation==="source_correspondence"),"lighting/equipment correspondence became additive energy flow");
 const lightingDriver=projected.nodes.find(n=>n.driverCategory==="internal.lighting"),lightingUse=projected.nodes.find(n=>n.endUse==="lighting"),equipmentUse=projected.nodes.find(n=>n.endUse==="equipment");
 button(lightingDriver.id)?.focus();button(lightingDriver.id)?.click();
 check(document.activeElement?.dataset.energyPathLayoutNode===lightingDriver.id&&host.querySelector('[data-energy-path-inspector="'+lightingDriver.id+'"]'),"native focused node activation did not retain focus and open inspector");
 check(button(lightingUse.id)?.dataset.energyPathRelated==="true"&&button(equipmentUse.id)?.dataset.energyPathRelated!=="true","counterpart selection did not highlight only matching lighting source");
 check(host.querySelectorAll("[data-energy-path-layout-node]").length===24,"source correspondence selection changed graph node count");
 const cooling=projected.nodes.find(n=>n.level==="load"&&n.serviceKind==="cooling");button(cooling.id)?.click();
 check(host.querySelector('[data-energy-path-inspector="'+cooling.id+'"]')?.textContent.includes(longLabel),"inspector lost full untruncated long load label");
 change("[data-simulation-energy-scope]","zone");change("[data-simulation-energy-zone-name]","Office");
 const zoneGeometry=geometry();
 check(Object.keys(zoneGeometry).length===24,"Zone dropped equivalent direct categories");
 for(const [id,rect]of Object.entries(buildingGeometry))check(rect&&zoneGeometry[id]&&rect.every((value,index)=>Math.abs(value-zoneGeometry[id][index])<1),"Zone changed category-equivalent geometry: "+id+" "+JSON.stringify([rect,zoneGeometry[id]]));
 check(pane.scrollHeight<=pane.clientHeight+1,"equivalent Zone view requires vertical scroll: "+pane.scrollHeight+"/"+pane.clientHeight);
 evidence.push("Zone "+[".energy-path-kpis",".energy-path-controls","[data-energy-path-layout]","[data-energy-path-quality-line]","[data-energy-path-inspector]",".energy-path-domain-legend"].map(selector=>{const r=host.querySelector(selector)?.getBoundingClientRect();return selector+":"+(r?Math.round(r.top)+"+"+Math.round(r.height):"absent");}).join("; "));
 for(const node of graph().nodes){const r=button(node.id)?.getBoundingClientRect();check(r&&r.bottom<=innerHeight+1,"Zone node clipped outside viewport: "+node.id);}
 check(!setup.open,"scope selection reopened completed setup");
 change("[data-simulation-energy-path-period]","M1");check(!setup.open,"period selection reopened completed setup");
 const summary=setup.querySelector("summary");summary.focus();summary.click();
 check(setup.open&&document.activeElement===summary,"native setup summary failed to open with keyboard focus");
 check(document.getElementById("simulationWeatherSelect")?.getBoundingClientRect().height>0&&document.getElementById("simulationRunButton")?.getBoundingClientRect().height>0&&setup.querySelectorAll('input[type="checkbox"]').length>=4,"open setup does not expose actual Weather / Run / purpose controls");
 simulation.renderSimulation();check(setup.open,"same-result rerender discarded user-open setup choice");
 summary.click();simulation.renderSimulation();check(!setup.open,"same-result rerender discarded user-closed setup choice");
 state.simulationRunning=true;simulation.renderSimulation();check(setup.open,"new running state did not reopen setup");
 state.simulationRunning=false;simulation.renderSimulation();summary.click();
 const weather=document.getElementById("simulationWeatherSelect");weather.focus();
 const newResult=freeze({...result,runId:"epath142-next-completed"});state.simulationResult=newResult;simulation.renderSimulation();
 check(!setup.open,"new completed result did not collapse setup once");
 check(document.activeElement===summary,"hiding a focused setup control did not return focus to summary");
 summary.click();simulation.renderSimulation();check(setup.open,"new completed result kept forcing setup closed after user reopened it");
 summary.click();state.simulationProgress={status:"failed",percent:100,message:"Run failed"};simulation.renderSimulation();
 check(setup.open,"failed progress with retained successful result hid retry setup");
 state.simulationResult=freeze({...result,runId:"epath142-failed",status:"failed"});simulation.renderSimulation();check(setup.open,"failed result hid retry setup");
 summary.click();simulation.renderSimulation();check(!setup.open,"same failure discarded user-closed setup choice");
 check(JSON.stringify(result)===originalJSON,"layout, correspondence, scope or setup interaction mutated raw result / export input");
 document.body.dataset.epath142Status=failures.length?"failed":"passed";document.getElementById("epath142-result").textContent=(failures.length?failures.join("\n"):"passed")+"\n"+evidence.join("\n");
 }
}catch(error){document.body.dataset.epath142Status="failed";document.getElementById("epath142-result").textContent=failures.join("\n")+"\n"+error.stack;}
</script>`
