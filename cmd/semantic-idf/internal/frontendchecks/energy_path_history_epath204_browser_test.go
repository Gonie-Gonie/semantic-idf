package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEPATH204ActualAppControlHistoryAndExactFocusBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser actual Energy Path history acceptance")
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
	page = strings.Replace(page, "</body>", epath142LayoutHTML+epath204HistoryHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/epath204-history.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	// Response-only forwarding wrappers observe real production computations.
	// No snapshot cache, history implementation or clock is mocked.
	for path, names := range map[string][]string{
		"views/energy-path-view.js": {"prepareEnergyPathScene", "energyPathGraphForState"},
		"energy-path-layout.js":     {"energyPathLayout"},
		"energy-path-ribbons.js":    {"energyPathRibbons"},
	} {
		source := readTestFile(t, "frontend/src/js/"+path)
		for _, name := range names {
			marker := "export function " + name + "("
			if strings.Count(source, marker) != 1 {
				t.Fatalf("expected one exported computation: %s", name)
			}
			original := "epath204Original_" + name
			source = strings.Replace(source, marker, "function "+original+"(", 1)
			source += fmt.Sprintf("\nexport function %s(...args) { const counts = globalThis.epath204Computations ||= {}; counts[%q] = (counts[%q] || 0) + 1; return %s(...args); }\n", name, name, name, original)
		}
		mux.HandleFunc("/src/js/"+path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = fmt.Fprint(w, source)
		})
	}
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	width, height := 1600, 900
	run := func() ([]byte, error) {
		return exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--force-device-scale-factor=1", fmt.Sprintf("--window-size=%d,%d", width, height), "--virtual-time-budget=15000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/src/epath204-history.html?manual=1").CombinedOutput()
	}
	output, err := run()
	if viewport := regexp.MustCompile(`data-epath204-viewport="(\d+)x(\d+)"`).FindSubmatch(output); err == nil && len(viewport) == 3 {
		actualWidth, _ := strconv.Atoi(string(viewport[1]))
		actualHeight, _ := strconv.Atoi(string(viewport[2]))
		if actualWidth != 1600 || actualHeight != 900 {
			if actualWidth < 1200 || actualWidth > 1800 || actualHeight < 600 || actualHeight > 1000 {
				t.Fatalf("unexpected headless viewport: %dx%d", actualWidth, actualHeight)
			}
			width += 1600 - actualWidth
			height += 900 - actualHeight
			output, err = run()
		}
	}
	if err != nil {
		t.Fatalf("EPATH204 browser: %v\n%s", err, output)
	}
	diagnostic := regexp.MustCompile(`(?s)<pre id="epath204-result"[^>]*>(.*?)</pre>`).FindStringSubmatch(string(output))
	if !strings.Contains(string(output), `data-epath204-status="passed"`) {
		if len(diagnostic) == 2 {
			t.Fatalf("EPATH204 failed: %s", diagnostic[1])
		}
		t.Fatalf("EPATH204 failed:\n%s", output)
	}
	if len(diagnostic) == 2 {
		t.Log(diagnostic[1])
	}
}

const epath204HistoryHTML = `<pre id="epath204-result" hidden>pending</pre><script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
document.body.dataset.epath204Viewport=innerWidth+"x"+innerHeight;
try{
 for(let attempt=0;document.body.dataset.epath142Status!=="manual"&&attempt<200;attempt++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath142Status!=="manual")throw new Error("actual app bootstrap failed");
 const [store,simulation,history,navigation,adapters,controller]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/view-history.js"),import("/src/js/navigation.js"),import("/src/js/panel-navigation-adapters.js"),import("/src/js/selection-controller.js")]);
 const {state}=store,previous=state.simulationResult,previousJSON=JSON.stringify(previous),result=JSON.parse(previousJSON),explanation=result.purposeResults.energyExplanation;
 // A genuinely heating-only selected period exercises an implicit Cooling→All
 // normalization in the same single user history transaction.
 for(const scope of[explanation,...explanation.zoneResults]){
  const base=scope.periods[0],links=base.links.filter(link=>link.serviceKind==="heating"&&link.relation!=="source_correspondence").map(link=>({...link,period:"M2"})),ids=new Set(links.flatMap(link=>[link.fromId,link.toId]));
  const nodes=base.nodes.filter(node=>ids.has(node.id)).map(node=>({...node,period:"M2"}));
  const summary={...base.summary,period:"M2"};for(const[key,level]of[["drivers","driver"],["loads","load"],["endUses","end_use"],["carriers","carrier"]])summary[key]=nodes.filter(node=>node.level===level);
  scope.periods.push({...base,id:"M2",period:"M2",nodes,links,summary});
 }
 const outputSources=[explanation,...explanation.zoneResults].map(scope=>scope.nodes.find(node=>node.level==="load"&&node.serviceKind==="cooling").sourceIds[0]);
 const outputSource=explanation.sources.find(source=>source.id===outputSources[0]);
 result.purposeRunPlan.outputObjects=[{objectType:"Output:Variable",keyValue:outputSource.keyValue,variableName:outputSource.name,reportingFrequency:outputSource.reportingFrequency}];
 result.runId="epath204-history";state.simulationResult=freeze(result);const rawJSON=JSON.stringify(result);
 state.activeInputView="text";state.reportAnalysisKey=state.analysisKey="epath204";state.reportAnalyzedText=store.getDocumentText();
 state.analysisDirty={metrics:false,topology:false,profile:false,hvac:false,simulation:false};
 state.semanticProjection={navigation:{entities:[],occurrences:[],byEntityId:{}}};
 adapters.initializeResultPanelNavigationAdapters();let backendCalls=0,queuedAnalysis=0;
 window.go.main.App=new Proxy(window.go.main.App,{get(target,key){if(/^(Analyze|Run|StartSimulation|RequestSimulation)/.test(String(key)))return async()=>{backendCalls++;throw new Error("unexpected backend "+String(key));};return target[key];}});
 controller.configureSelectionController({state,getNavigationIndex:()=>state.semanticProjection.navigation,getCurrentText:store.getDocumentText,getReportAnalysisKey:()=>state.reportAnalysisKey,isAnalysisCurrent:()=>true,getActiveInputView:()=>"input-text",getActivePanelView:()=>state.activeResultTab,queueAnalysisTarget:()=>{queuedAnalysis++;},recordHistory:()=>history.recordViewHistory(),openView:(target,options)=>navigation.switchResultTab(target,{...options,recordHistory:false}),onSelectionChange:detail=>window.dispatchEvent(new CustomEvent("idfAnalyzer:semanticSelectionChanged",{detail}))});
 const host=document.getElementById("simulationEnergyDashboard"),attributes={scope:"data-simulation-energy-scope",zone:"data-simulation-energy-zone-name",period:"data-simulation-energy-path-period",service:"data-simulation-energy-service"};
 const control=kind=>host.querySelector('['+attributes[kind]+']');
 const primary=()=>JSON.stringify(simulation.captureSimulationEnergyWorkspaceContext());
 const stacks=()=>JSON.stringify([state.navigationUndoStack,state.navigationRedoStack]);
 const counts=()=>JSON.stringify(globalThis.epath204Computations);
 const snapshot=()=>history.captureViewSnapshot().panelContexts.simulation;
 const canvas=()=>host.querySelector("[data-energy-path-canvas]");
 const inspector=()=>host.querySelector("[data-energy-path-inspector],[data-energy-path-link-inspector]");
 const node=id=>[...host.querySelectorAll("[data-energy-path-layout-node]")].find(element=>element.dataset.energyPathLayoutNode===id);
 const choose=id=>{const target=node(id);if(!target)throw new Error("missing node "+id);const oldCanvas=canvas(),before=counts(),oldStacks=stacks();target.focus();target.click();check(state.simulationEnergySelection===id&&document.activeElement===target,"node selection/focus failed "+id);check(canvas()===oldCanvas&&counts()===before&&stacks()===oldStacks,"node selection rebuilt graph / recorded navigation "+id);};
 const emit=(kind,value,type="change")=>{const target=control(kind);if(!target)throw new Error("missing control "+kind);target.focus();target.value=value;target.dispatchEvent(new Event(type,{bubbles:true}));return target;};
 const reset=(scope="building",period="annual",service="all")=>{simulation.restoreSimulationEnergyWorkspaceContext({energyScopeKind:scope,energyZoneName:scope==="zone"?"Office":"",energyPeriod:period,energyService:service,energySelection:"",energyDetailsOpen:false,energyDrawer:{tab:"data",stage:"",outputSource:""}});simulation.renderSimulation();state.navigationUndoStack=[];state.navigationRedoStack=[];};
 const assertOutput=label=>{const row=host.querySelector('[data-energy-path-output-request-selected="true"]');check(row&&row.textContent.includes(outputSource.name)&&row.textContent.includes(outputSource.reportingFrequency),label+": exact declared output request is not visibly selected");};
 const openDrawer=()=>{host.querySelector('[data-energy-path-quality-stage="loads"]').click();const sourceID=outputSources[state.simulationEnergyScopeKind==="zone"?1:0],jump=[...host.querySelectorAll("[data-energy-path-output-source]")].find(button=>button.dataset.energyPathOutputSource===sourceID);if(!jump)throw new Error("missing actual Data output jump "+sourceID);jump.click();check(state.simulationEnergyDetailsOpen&&snapshot().energyDrawer.tab==="output"&&snapshot().energyDrawer.stage==="loads"&&snapshot().energyDrawer.outputSource===sourceID,"drawer setup did not retain all3 local fields via actual source jump");assertOutput("drawer source jump");};
 const assertRestored=(expected,kind,label)=>{check(primary()===expected,label+": six fields / drawer did not restore exactly: "+primary()+" expected "+expected);check(document.activeElement===control(kind),label+": exact control focus not restored");if(state.simulationEnergySelection)check(inspector()&&node(state.simulationEnergySelection),label+": selected inspector missing");check(!host.querySelector("[data-energy-path-data-details]")?.hidden===state.simulationEnergyDetailsOpen,label+": drawer visibility differs from restored state");if(state.simulationEnergyDetailsOpen&&snapshot().energyDrawer.outputSource)assertOutput(label);};
 check(innerWidth===1600&&innerHeight===900,"actual viewport is not1600x900");
 reset();choose("load.cooling.building");openDrawer();control("scope").focus();const building=primary();
 emit("scope","zone","input");check(state.navigationUndoStack.length===1&&state.simulationEnergyScopeKind==="zone"&&state.simulationEnergySelection==="","Building→Zone input did not create one history point / clear selection");const zone=primary();
 check(snapshot().targetId==="energy-path:control:scope","Scope focus was not namespaced in existing targetId");
 await navigation.undoViewNavigation({quiet:true});assertRestored(building,"scope","Building→Zone→Back");check(state.navigationUndoStack.length===0&&state.navigationRedoStack.length===1,"Back added extra history");
 await navigation.redoViewNavigation({quiet:true});assertRestored(zone,"scope","Zone Forward");
 reset();choose("load.cooling.building");openDrawer();control("period").focus();const annual=primary();
 const detached=emit("period","M1","input");check(state.navigationUndoStack.length===1&&state.simulationEnergyPeriod==="M1","Annual→M1 native input did not commit once");
 choose("load.cooling.building");
 const month=primary(),monthCanvas=canvas(),monthInspector=inspector(),monthCounts=counts(),monthStacks=stacks();
 detached.dispatchEvent(new Event("change",{bubbles:true}));emit("period","M1","change");
 check(primary()===month&&canvas()===monthCanvas&&inspector()===monthInspector&&counts()===monthCounts&&stacks()===monthStacks,"following detached/live change erased new selection, graph or history");
 await navigation.undoViewNavigation({quiet:true});assertRestored(annual,"period","Annual→M1→Back");
 await navigation.redoViewNavigation({quiet:true});assertRestored(month,"period","M1 Forward with selected node / drawer");
 reset("zone","M1","cooling");choose("load.cooling.office");openDrawer();control("period").focus();const cooling=primary();
 emit("period","M2","input");check(state.simulationEnergyPeriod==="M2"&&state.simulationEnergyService==="all"&&state.navigationUndoStack.length===1,"heating-only month did not normalize Cooling→All within one history transaction");
 check([...control("service").options].map(option=>option.value).join("|")==="all|heating","M2 fixture is not genuinely heating-only");
 await navigation.undoViewNavigation({quiet:true});assertRestored(cooling,"period","implicit service fallback Back");
 const noChange=(label,action,{typing=false}={})=>{const before=primary(),beforeStacks=stacks(),beforeCounts=counts(),oldCanvas=canvas(),oldInspector=inspector(),oldDrawer=host.querySelector("[data-energy-path-data-details]");action();check(primary()===before&&stacks()===beforeStacks,label+": changed nondefault context / selection / drawer or Undo/Redo");check(counts()===beforeCounts&&canvas()===oldCanvas&&inspector()===oldInspector&&host.querySelector("[data-energy-path-data-details]")===oldDrawer,label+": repainted / reprojected unchanged context");if(!typing)for(const[kind,key]of[["scope","simulationEnergyScopeKind"],["zone","simulationEnergyZoneName"],["period","simulationEnergyPeriod"],["service","simulationEnergyService"]])check(control(kind)?.value===state[key],label+": failed in-place control value restoration: "+kind);};
 for(const[kind,value]of[["scope","zone"],["period","M1"],["service","cooling"],["zone"," office "]])noChange("no-op "+kind,()=>{const target=emit(kind,value);check(document.activeElement===target,"no-op lost control focus "+kind);});
 noChange("Zone typing",()=>{const target=emit("zone","Off","input");check(target.value==="Off"&&document.activeElement===target,"Zone input text/focus was not retained while typing");},{typing:true});
 noChange("invalid Zone commit",()=>{const target=emit("zone","unknown Office");check(document.activeElement===target,"invalid Zone commit lost focus");});
 for(const kind of["scope","period","service"])noChange("invalid "+kind,()=>{const target=emit(kind,"invalid.204");check(document.activeElement===target,"invalid "+kind+" lost focus");});
 for(const[kind,value]of[["scope","building"],["period","M2"],["service","heating"]])noChange("disabled option "+kind,()=>{const option=[...control(kind).options].find(option=>option.value===value);option.disabled=true;emit(kind,value);option.disabled=false;});
 noChange("disabled control",()=>{const target=control("period");target.disabled=true;target.value="M2";target.dispatchEvent(new Event("change",{bubbles:true}));target.disabled=false;});
 check(state.navigationRedoStack.length===1,"no-op / invalid events discarded Redo");
 const branchBefore=primary(),depth=state.navigationUndoStack.length;emit("period","annual");check(state.navigationUndoStack.length===depth+1&&state.navigationRedoStack.length===0,"valid branch after Undo did not replace Forward once");
 await navigation.undoViewNavigation({quiet:true});assertRestored(branchBefore,"period","valid new branch Back");
 // Graph focus uses the existing targetId only when that exact native control
 // is active. The common history APIs remain the sole snapshot/restore path.
 reset();
 for(const[type,selector,attribute]of[["node",'[data-energy-path-layout-node="load.cooling.building"]',"data-energy-path-layout-node"],["ratio","button[data-energy-path-bridge-ratio]","data-energy-path-bridge-ratio"],["edge",'[data-energy-path-link-hit][tabindex="0"]',"data-energy-path-link-hit"]]){
  const target=host.querySelector(selector);if(!target)throw new Error("missing current graph focus target "+type);
  target.focus();const saved=history.captureViewSnapshot(),id=target.getAttribute(attribute);
  check(saved.panelContexts.simulation.targetId==="energy-path:"+type+":"+encodeURIComponent(id),"actual graph focus lacks exact namespace "+type);
  history.recordViewHistory(saved);control("scope").focus();await navigation.undoViewNavigation({quiet:true});
  check(document.activeElement?.getAttribute(attribute)===id,"history did not restore exact native graph focus "+type);
 }
 choose("load.cooling.building");const action=inspector().querySelector("button:not(:disabled)");if(action){action.focus();check(!snapshot().targetId?.startsWith("energy-path:"),"inspector action was incorrectly captured as a graph/control target");}
 const stale={...snapshot(),targetId:"energy-path:node:not-a-real-node"};control("scope").focus();await simulation.restoreSimulationNavigationContext(stale,{genericRestoreContext:async()=>{}});check(!document.activeElement?.matches?.("[data-energy-path-layout-node]"),"unavailable focus target selected an arbitrary first node");
 const expectedKeys=["simulationEnergyScopeKind","simulationEnergyZoneName","simulationEnergyPeriod","simulationEnergyService","simulationEnergySelection","simulationEnergyDetailsOpen"].sort();
 check(JSON.stringify(Object.keys(state).filter(key=>key.startsWith("simulationEnergy")).sort())===JSON.stringify(expectedKeys),"history added persistent Energy primary state");
 check(backendCalls===0&&queuedAnalysis===0,"history/control navigation analyzed or reran the model: "+backendCalls+"/"+queuedAnalysis);
 check(state.simulationResult===result&&JSON.stringify(result)===rawJSON&&JSON.stringify(previous)===previousJSON,"history mutated original / selected result payload");
 evidence.push("Actual Building/Zone and Annual/M1 Back/Forward; native input/change single entry; nondefault invalid/no-op preserve Redo/selection/drawer/DOM; implicit service fallback; exact control/node/ratio/edge focus; six primary fields and0 Analyze/Run");
}catch(error){failures.push(error.stack||String(error));}
document.body.dataset.epath204Status=failures.length?"failed":"passed";
document.getElementById("epath204-result").textContent=JSON.stringify({failures,evidence});
</script>`
