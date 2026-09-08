package frontendchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
	"github.com/gorilla/websocket"
)

func TestEPATH161ActualAppSelectionWithoutGraphRedrawBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("native-time actual Energy Path redraw / latency acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	input, err := os.ReadFile(repoPath("frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := idf.Parse(string(input))
	if err != nil {
		t.Fatal(err)
	}
	// The balanced Energy graph is explicitly synthetic acceptance data. Its
	// expensive model context uses the actual, untrimmed 19-Zone Office analysis.
	model, err := json.Marshal(map[string]any{
		"geometry": idf.AnalyzeGeometry(document), "profile": idf.AnalyzeProfile(document), "hvac": idf.AnalyzeHVAC(document),
		"semanticNavigation": idf.BuildSemanticModel(document, idf.SemanticYAMLMetadata{}).Navigation,
	})
	if err != nil {
		t.Fatal(err)
	}
	page := readTestFile(t, "frontend/src/index.html")
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		if !strings.Contains(page, script) {
			t.Fatalf("actual app bootstrap changed: %s", script)
		}
		page = strings.Replace(page, script, "", 1)
	}
	page = strings.Replace(page, "</body>", epath142LayoutHTML+epath143RibbonsHTML+epath161RedrawHTML+"</body>", 1)
	// Response-only forwarding wrappers count real computation calls. Production
	// code has no test hook, no altered imports, and no synthetic performance clock.
	instrument := func(path string, functions ...string) string {
		source := readTestFile(t, "frontend/src/js/"+path)
		for _, name := range functions {
			marker := "export function " + name + "("
			if strings.Count(source, marker) != 1 {
				t.Fatalf("expected one synchronous exported computation %s in %s", name, path)
			}
			original := "epath161Original_" + name
			source = strings.Replace(source, marker, "function "+original+"(", 1)
			source += fmt.Sprintf("\nexport function %s(...args) { const counters = globalThis.epath161Computations ||= {}; counters[%q] = (counters[%q] || 0) + 1; return %s(...args); }\n", name, name, name, original)
		}
		return source
	}
	profile := func(path string, functions ...string) string {
		source := readTestFile(t, "frontend/src/js/"+path)
		if path == "views/energy-path-view.js" {
			source = instrument(path, "prepareEnergyPathScene", "energyPathGraphForState")
		}
		for _, name := range functions {
			marker := "export function " + name + "("
			export := "export "
			if !strings.Contains(source, marker) {
				marker = "function " + name + "("
				export = ""
			}
			if strings.Count(source, marker) != 1 {
				t.Fatalf("expected one synchronous inspector helper %s in %s", name, path)
			}
			original := "epath161Profiled_" + name
			source = strings.Replace(source, marker, "function "+original+"(", 1)
			source += fmt.Sprintf("\n%sfunction %s(...args) { const calls = globalThis.epath161InspectorCalls ||= {}; calls[%q] = (calls[%q] || 0) + 1; const started = performance.now(); try { return %s(...args); } finally { const timings = globalThis.epath161InspectorTimings ||= {}; timings[%q] = (timings[%q] || 0) + performance.now() - started; } }\n", export, name, name, name, original, name, name)
		}
		return source
	}
	modules := map[string]string{
		"views/energy-path-view.js":           profile("views/energy-path-view.js", "renderEnergyPathNodeInspector", "renderEnergyPathLinkInspector", "updateEnergyPathSelection"),
		"energy-path-layout.js":               instrument("energy-path-layout.js", "energyPathLayout"),
		"energy-path-ribbons.js":              instrument("energy-path-ribbons.js", "energyPathRibbons"),
		"views/simulation-views.js":           profile("views/simulation-views.js", "simulationEnergyInspectorActions", "simulationEnergyInspectorRelatedEntities", "simulationEnergyDriverNavigation", "simulationEnergyServiceNavigation", "prepareSimulationEnergyModelNavigation", "simulationSemanticNavigationCache", "simulationEnergyGraphFocusTarget"),
		"energy-path-driver-destinations.js":  profile("energy-path-driver-destinations.js", "energyPathDriverDestinations", "prepareEnergyPathDriverNavigation"),
		"energy-path-service-destinations.js": profile("energy-path-service-destinations.js", "energyPathServiceDestinations"),
		"energy-path-inspector.js":            profile("energy-path-inspector.js", "energyPathInspectorModel"),
	}
	type browserResult struct {
		Failures []string `json:"failures"`
		Evidence []string `json:"evidence"`
		Width    int      `json:"width"`
		Height   int      `json:"height"`
	}
	done := make(chan browserResult, 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	for path, source := range modules {
		mux.HandleFunc("/src/js/"+path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = fmt.Fprint(w, source)
		})
	}
	mux.HandleFunc("/epath161-model.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(model)
	})
	mux.HandleFunc("/src/epath161-redraw.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	mux.HandleFunc("/epath161/done", func(w http.ResponseWriter, r *http.Request) {
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
	width, height := 1600, 900
	for attempt := 0; attempt < 2; attempt++ {
		profile := t.TempDir()
		// Do not let CommandContext kill Chrome before its crash-reporting children
		// close their files. Every exit path below uses this owned profile's CDP
		// Browser.close endpoint; timing/assertion code is unchanged.
		command := exec.Command(chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0", "--force-device-scale-factor=1", fmt.Sprintf("--window-size=%d,%d", width, height), "--user-data-dir="+profile, server.URL+"/src/epath161-redraw.html?manual=1&run161=1")
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		stop := epath161OwnedChromeCleanup(t, command, profile)
		t.Cleanup(stop)
		var result browserResult
		select {
		case result = <-done:
		case <-ctx.Done():
			stop()
			t.Fatal("native-time redraw acceptance timed out")
		}
		stop()
		if result.Width != 1600 || result.Height != 900 {
			if attempt != 0 || result.Width < 1200 || result.Width > 1800 || result.Height < 600 || result.Height > 1000 {
				t.Fatalf("unexpected browser content viewport %dx%d", result.Width, result.Height)
			}
			width += 1600 - result.Width
			height += 900 - result.Height
			continue
		}
		for _, line := range result.Evidence {
			t.Log(line)
		}
		if len(result.Failures) > 0 {
			t.Fatalf("EPATH161 actual redraw: %s", strings.Join(result.Failures, "\n"))
		}
		return
	}
}

// The profile comes only from the immediately preceding t.TempDir call. Never
// attach to an existing browser or terminate processes by name/system-wide.
func epath161OwnedChromeCleanup(t *testing.T, command *exec.Cmd, profile string) func() {
	t.Helper()
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	var once sync.Once
	return func() {
		once.Do(func() {
			closed := false
			select {
			case <-wait:
				closed = true
			default:
			}
			if !closed {
				if err := epath161CloseOwnedBrowser(profile); err != nil {
					t.Logf("test-owned Browser.close: %v", err)
				}
				select {
				case <-wait:
					closed = true
				case <-time.After(5 * time.Second):
					// Last resort is exactly the exec.Cmd process this test started.
					// No taskkill tree, broad Chrome search, or user-browser endpoint.
					_ = command.Process.Kill()
					select {
					case <-wait:
						closed = true
					case <-time.After(2 * time.Second):
					}
				}
			}
			if !closed {
				t.Error("test-owned Chrome did not exit after bounded shutdown")
				return
			}
			// Crashpad may finish closing handles just after the browser exits. Retry
			// only this validated test-owned directory, before t.TempDir's cleanup.
			absolute, err := filepath.Abs(profile)
			if err != nil {
				t.Error(err)
				return
			}
			resolved, err := filepath.EvalSymlinks(profile)
			if os.IsNotExist(err) {
				return
			}
			// Windows may canonicalize a legitimate 8.3 TEMP parent name. Resolve
			// that parent too, while rejecting a substituted profile junction.
			parent, parentErr := filepath.EvalSymlinks(filepath.Dir(absolute))
			if err != nil || parentErr != nil || !strings.EqualFold(filepath.Clean(resolved), filepath.Join(parent, filepath.Base(absolute))) || filepath.Dir(absolute) == absolute {
				t.Errorf("refusing cleanup of changed test profile: %q / %v", profile, err)
				return
			}
			for deadline := time.Now().Add(3 * time.Second); ; {
				if err = os.RemoveAll(resolved); err == nil {
					return
				}
				if time.Now().After(deadline) {
					t.Errorf("test-owned Chrome profile still locked after graceful exit: %v", err)
					return
				}
				time.Sleep(25 * time.Millisecond)
			}
		})
	}
}

func epath161CloseOwnedBrowser(profile string) error {
	data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		return fmt.Errorf("invalid owned DevTools endpoint")
	}
	port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	path := strings.TrimSpace(lines[1])
	if err != nil || port < 1 || port > 65535 || !strings.HasPrefix(path, "/devtools/browser/") || strings.ContainsAny(path, "?# \t\r\n") {
		return fmt.Errorf("invalid owned DevTools endpoint")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	socket, _, err := websocket.DefaultDialer.DialContext(ctx, fmt.Sprintf("ws://127.0.0.1:%d%s", port, path), nil)
	if err != nil {
		return err
	}
	defer socket.Close()
	_ = socket.SetWriteDeadline(time.Now().Add(2 * time.Second))
	return socket.WriteJSON(map[string]any{"id": 1, "method": "Browser.close"})
}

const epath161RedrawHTML = `<pre id="epath161-result" hidden>pending</pre><script type="module">
const failures=[],evidence=[],latencies=[];
const check=(value,message)=>{if(!value)failures.push(message);};
const sleep=ms=>new Promise(resolve=>setTimeout(resolve,ms));
const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
const report=async()=>{document.body.dataset.epath161Status=failures.length?"failed":"passed";document.getElementById("epath161-result").textContent=failures.concat(evidence).join("\n");await fetch("/epath161/done",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({failures,evidence,width:innerWidth,height:innerHeight})});};
try{
 if(innerWidth!==1600||innerHeight!==900){await report();}
 else{
  for(let attempt=0;document.body.dataset.epath143Status!=="manual"&&attempt<160;attempt++)await sleep(25);
  if(document.body.dataset.epath143Status!=="manual")throw new Error("actual142/143 bootstrap did not finish");
  const [store,simulation,view,history,navigation,adapters,hvacViews,controller]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/views/energy-path-view.js"),import("/src/js/view-history.js"),import("/src/js/navigation.js"),import("/src/js/panel-navigation-adapters.js"),import("/src/js/views/hvac-views.js"),import("/src/js/selection-controller.js")]);
  for(const name of["renderEnergyPathHeader","renderEnergyPathKPI","renderEnergyPathGraph","renderEnergyPathInspector","renderEnergyPathQuality","renderEnergyPathDetails"])check(typeof view[name]==="function","missing separated rendering stage "+name);
  const {state}=store,fixture=await(await fetch("/epath161-model.json")).json(),modelJSON=JSON.stringify(fixture);
  const result=JSON.parse(JSON.stringify(state.simulationResult)),explanation=result.purposeResults.energyExplanation,originalZone=explanation.zoneResults[0];
  const zones=fixture.geometry.zones.map(zone=>zone.name),paths=(fixture.hvac.serviceModel?.zoneServices||[]).flatMap(zone=>zone.paths||[]);
  check(zones.length===19&&fixture.semanticNavigation.entities.length>1000,"real untrimmed LargeOffice metadata is missing: "+JSON.stringify({zones:zones.length,entities:fixture.semanticNavigation.entities.length}));
  const sourceRecords=new Map(),profileDimensions={"internal.people":"occupancy","internal.lighting":"lighting","internal.equipment":"equipment","air.infiltration":"infiltration","air.mechanical_ventilation":"ventilation"};
  const profileEntities=new Map();
  for(const entity of fixture.semanticNavigation.entities)for(const target of entity.viewTargets||[])if(target.view==="profile"&&target.targetKind==="profile-item")profileEntities.set(target.targetId,entity.id);
  const bind=(graph,zoneName="")=>{
   for(const node of graph.nodes){
    const matchingZones=zoneName?[zoneName]:zones,category=node.driverCategory||node.endUse||node.carrier||node.serviceKind;
    node.sourceIds=[];node.relatedEntityIds=[];
    for(const name of matchingZones){
     const id="source.epath161."+category+"."+name;
     const entities=(fixture.profile.zoneProfiles||[]).find(zone=>zone.zoneName===name)?.items?.filter(item=>item.dimension===profileDimensions[node.driverCategory]).map(item=>profileEntities.get(item.id)).filter(Boolean)||[];
     sourceRecords.set(id,{id,sourceType:"sql_variable",name:category+" reported energy",zoneName:name,keyValue:name,reportingFrequency:"Monthly",sourceUnit:"J",normalizedUnit:"kWh",relatedEntityIds:entities});
     node.sourceIds.push(id);node.relatedEntityIds.push(...entities);
    }
    const service=node.serviceKind||node.endUse;
    if(node.level==="load"||node.level==="end_use"&&["cooling","heating"].includes(node.endUse))node.relatedPathIds=paths.filter(path=>(!zoneName||path.zoneName===zoneName)&&path.serviceKind===service).map(path=>path.id);
   }
   const byID=new Map(graph.nodes.map(node=>[node.id,node]));for(const link of graph.links)link.sourceIds=[...new Set([...(byID.get(link.fromId)?.sourceIds||[]),...(byID.get(link.toId)?.sourceIds||[])])];
   if(graph.summary)for(const[group,level]of[["drivers","driver"],["loads","load"],["endUses","end_use"],["carriers","carrier"]])graph.summary[group]=graph.nodes.filter(node=>node.level===level);
  };
  explanation.zoneResults=zones.map((name,index)=>{
   const zone=JSON.parse(JSON.stringify(originalZone).replaceAll('"Office"',JSON.stringify(name)).replaceAll(".office",".actualzone"+index));
   for(const graph of[zone,...zone.periods]){
    for(const node of graph.nodes)for(const key of["value","rawValue","effectiveValue","allocatedValue","displayValue","signedValue"])if(typeof node[key]==="number")node[key]*=2/zones.length;
    for(const link of graph.links)for(const key of["value","fromValue","toValue"])if(typeof link[key]==="number")link[key]*=2/zones.length;
    bind(graph,name);
   }
   return zone;
  });
  bind(explanation);explanation.periods.forEach(graph=>bind(graph));explanation.sources=[...sourceRecords.values()];
  result.runId="epath161-real-office-context";result.purposeResults.energyExplanationSummary=explanation.summary;
  state.report={geometry:fixture.geometry,profile:fixture.profile,hvac:fixture.hvac,metrics:{categories:[]}};state.semanticProjection={navigation:fixture.semanticNavigation};
  state.reportAnalyzedText=store.getDocumentText();state.reportAnalysisKey=state.analysisKey="epath161-office-context";state.analysisDirty={metrics:false,topology:false,profile:false,hvac:false,simulation:false};state.analysisReady={topology:true,profile:true,hvac:true,simulation:true};state.geometryReady=true;
  adapters.initializeResultPanelNavigationAdapters();hvacViews.initializeHVACControls();controller.configureSelectionController({state,getNavigationIndex:()=>state.semanticProjection.navigation,getCurrentText:store.getDocumentText,isAnalysisCurrent:()=>true,getActivePanelView:()=>state.activeResultTab,recordHistory:payload=>{const snapshot=history.captureViewSnapshot();if(payload.previous)snapshot.globalSelection=payload.previous;history.recordViewHistory(snapshot);},openView:async(target,options)=>navigation.switchResultTab(target,{...options,recordHistory:false}),onSelectionChange:detail=>window.dispatchEvent(new CustomEvent("idfAnalyzer:semanticSelectionChanged",{detail}))});
  state.simulationResult=freeze(result);state.simulationActiveResultView="energy";simulation.restoreSimulationEnergyWorkspaceContext({energyScopeKind:"building",energyZoneName:"",energyPeriod:"annual",energyService:"all",energySelection:"",energyDetailsOpen:false,energyDrawer:{tab:"data",stage:"",outputSource:""}});simulation.renderSimulation();
  const rawJSON=JSON.stringify(result),host=document.getElementById("simulationEnergyDashboard"),pane=document.querySelector("#simulationPane > .simulation-pane");
  const graph=()=>view.energyPathGraphForState(state.simulationResult.purposeResults.energyExplanation,state),initialGraph=graph();
  const node=id=>host.querySelector('[data-energy-path-layout-node="'+CSS.escape(id)+'"]'),hit=id=>host.querySelector('[data-energy-path-link-hit="'+CSS.escape(id)+'"]'),ratio=id=>host.querySelector('[data-energy-path-bridge-ratio="'+CSS.escape(id)+'"]');
  const counters=()=>({...globalThis.epath161Computations});
  const changed=(before,after)=>Object.fromEntries(Object.keys(after).filter(key=>after[key]!==before[key]).map(key=>[key,after[key]-(before[key]||0)]));
  const graphSelector="[data-energy-path-canvas],[data-energy-path-layout-node],[data-energy-path-bar],[data-energy-path-ribbon],[data-energy-path-bridge-ratio],[data-energy-path-ratio-tooltip],[data-energy-path-link-hit],[data-energy-path-hit-layer],.energy-path-controls,[data-energy-path-kpi],[data-energy-path-quality-line]";
  const retain=()=>{const canvas=host.querySelector("[data-energy-path-canvas]"),r=canvas.getBoundingClientRect();return[...host.querySelectorAll(graphSelector)].map(element=>({element,geometry:JSON.stringify([...["d","x","y","width","height","data-from-x","data-from-y0","data-from-y1","data-to-x","data-to-y0","data-to-y1","data-from-width","data-to-width"].map(key=>element.getAttribute(key)),element.style.left,element.style.top,element.style.width,element.style.height]),rect:element.hasAttribute("data-energy-path-layout-node")?(()=>{const b=element.getBoundingClientRect();return[b.x-r.x,b.y-r.y,b.width,b.height];})():null}));};
  const assertRetained=(before,label)=>{const current=retain();check(current.length===before.length&&current.every((item,index)=>item.element===before[index].element&&item.geometry===before[index].geometry&&JSON.stringify(item.rect)===JSON.stringify(before[index].rect)),label+" replaced or moved graph/header/KPI/quality DOM");};
  const assertNoWork=(before,label)=>check(Object.keys(changed(before,counters())).length===0,label+" recomputed scene/projection/layout/ribbons: "+JSON.stringify(changed(before,counters())));
  const sectionNames=["represents","value","breakdown","basis","entities","sources","actions"];
  const measure=(label,activate,expectedID,{link=false,keepInspector=false}={})=>{
   const retained=retain(),before=counters(),oldInspector=host.querySelector("[data-energy-path-inspector],[data-energy-path-link-inspector]"),details=host.querySelector("[data-energy-path-data-details]");
   const profileBefore={...globalThis.epath161InspectorTimings},started=performance.now();activate();const activated=performance.now();
   const inspector=host.querySelector(link?"[data-energy-path-link-inspector]":"[data-energy-path-inspector]");
   const rect=inspector?.getBoundingClientRect(),style=inspector&&getComputedStyle(inspector);
   const elapsed=performance.now()-started;latencies.push({label,ms:elapsed});
   if(label==="node surface.exterior_walls"||label.includes("fresh report metadata")||elapsed>50){const timings=Object.fromEntries(Object.entries(globalThis.epath161InspectorTimings||{}).map(([name,time])=>[name,+(time-(profileBefore[name]||0)).toFixed(2)]));evidence.push(label+" profile: handler="+(activated-started).toFixed(2)+"ms, post-handler forced layout/style="+(elapsed-activated+started).toFixed(2)+"ms, inclusive helpers="+JSON.stringify(timings));}
   check(elapsed<=50,label+" exceeded50ms including current inspector/layout: "+elapsed.toFixed(2)+"ms");
   if(expectedID){check(state.simulationEnergySelection===expectedID&&inspector?.getAttribute(link?"data-energy-path-link-inspector":"data-energy-path-inspector")===expectedID,label+" did not synchronously update selected inspector");check(rect?.width>0&&rect.height>0&&style.display!=="none"&&style.visibility!=="hidden",label+" measured a deferred/hidden inspector");check(JSON.stringify([...inspector?.querySelectorAll(":scope > [data-energy-path-detail-section]")||[]].map(item=>item.dataset.energyPathDetailSection))===JSON.stringify(sectionNames),label+" deferred one of7 inspector sections");}
   if(keepInspector)check(inspector===oldInspector,label+" replaced unchanged current inspector");
   assertRetained(retained,label);assertNoWork(before,label);if(details&&expectedID)check(host.querySelector("[data-energy-path-data-details]")===details,label+" replaced independent Data drawer");
   return inspector;
  };
  const activateNode=(item,label=item.driverCategory||item.endUse||item.carrier||item.serviceKind)=>measure("node "+label,()=>{node(item.id).focus({preventScroll:true});node(item.id).click();},item.id);
  const change=(selector,value)=>{const target=host.querySelector(selector);if(!target)throw new Error("missing real context control "+selector);target.value=value;target.dispatchEvent(new Event("change",{bubbles:true}));};
  check(host.querySelectorAll("[data-energy-path-layout-node]").length===24,"dense scene lost its24 material nodes");check(pane.scrollWidth<=pane.clientWidth+1&&pane.scrollHeight<=pane.clientHeight+1,"actual dense initial frame overflows");
  evidence.push("Actual24-node frame, "+zones.length+" analyzed Office Zones, "+fixture.semanticNavigation.entities.length+" semantic entities, "+paths.length+" actual service paths");
  await sleep(40);
  if(new URLSearchParams(location.search).get("run161")!=="1"){document.body.dataset.epath161Status="manual";}
  else{
   const lightingDriver=initialGraph.nodes.find(item=>item.driverCategory==="internal.lighting"),lightingUse=initialGraph.nodes.find(item=>item.endUse==="lighting"),cooling=initialGraph.nodes.find(item=>item.level==="load"&&item.serviceKind==="cooling");
   for(const predicate of[item=>item.driverCategory==="surface.exterior_walls",item=>item.driverCategory==="internal.people",item=>item.id===lightingDriver.id,item=>item.id===cooling.id,item=>item.endUse==="cooling",item=>item.level==="carrier"&&item.carrier==="electricity"]){const item=initialGraph.nodes.find(predicate),inspector=activateNode(item);if(["surface.exterior_walls","internal.people"].includes(item.driverCategory))check(inspector.querySelector("[data-energy-path-driver-destination]"),"real Office model context action was not built synchronously for "+item.driverCategory);if(item.endUse==="cooling")check(inspector.querySelector('[data-energy-path-service-kind="hvac"] [data-energy-path-service-destination]'),"real Office HVAC actions were omitted from timed inspector");}
   activateNode(lightingDriver,"lighting counterpart setup");await sleep(180);check(node(lightingUse.id).dataset.energyPathFocus==="counterpart"&&getComputedStyle(node(lightingUse.id)).opacity==="1","same-DOM selection lost counterpart card distinction");check([...host.querySelectorAll('[data-energy-path-bar="'+CSS.escape(lightingUse.id)+'"]')].every(bar=>Number(getComputedStyle(bar).opacity)===.25),"counterpart physical bars became selected flow");
   activateNode(lightingUse,"counterpart card activation");
   for(const relation of["driver_to_load","load_to_end_use","end_use_to_carrier"]){const link=initialGraph.links.find(item=>item.relation===relation&&hit(item.id));if(!link)throw new Error("missing actual ribbon "+relation);const target=relation==="load_to_end_use"?ratio(link.id):hit(link.id),tooltip=target.querySelector("[data-energy-path-ratio-tooltip]");measure("edge "+relation,()=>{target.focus({preventScroll:true});if(target.tagName==="BUTTON")target.click();else target.dispatchEvent(new KeyboardEvent("keydown",{key:"Enter",bubbles:true,cancelable:true}));},link.id,{link:true});check(document.activeElement===target&&(!tooltip||target.querySelector("[data-energy-path-ratio-tooltip]")===tooltip),"edge selection replaced its focused control/tooltip");if(relation!=="load_to_end_use")measure("edge Space "+relation,()=>target.dispatchEvent(new KeyboardEvent("keydown",{key:" ",code:"Space",bubbles:true,cancelable:true})),link.id,{link:true});}
   const kpi=host.querySelector('[data-energy-path-kpi="cooling_load"] [data-energy-path-kpi-node]');measure("visible Cooling KPI",()=>kpi.click(),cooling.id);
   const inspector=host.querySelector("[data-energy-path-inspector]"),sources=inspector.querySelector('[data-energy-path-detail-section="sources"]');sources.open=true;
   const beforeDetails=counters(),retained=retain();host.querySelector('[data-energy-path-quality-stage="drivers"]').click();check(host.querySelector("[data-energy-path-inspector]")===inspector&&sources.open,"drawer open replaced inspector or collapsed native Sources");assertRetained(retained,"Data drawer open");assertNoWork(beforeDetails,"Data drawer open");
   for(const tab of["output","data"]){const before=counters(),scene=retain();host.querySelector('[data-energy-path-details-tab="'+tab+'"]').click();check(host.querySelector("[data-energy-path-inspector]")===inspector&&sources.open,"drawer tab replaced inspector/native Sources");assertRetained(scene,"drawer "+tab);assertNoWork(before,"drawer "+tab);}
   const dataDrawer=host.querySelector("[data-energy-path-data-details]");activateNode(lightingDriver,"selection with independent Data drawer");check(host.querySelector("[data-energy-path-data-details]")===dataDrawer,"node selection replaced open independent Data drawer");
   const beforeClear=counters(),clearScene=retain(),focused=node(lightingDriver.id);focused.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true}));check(state.simulationEnergySelection===""&&!host.querySelector("[data-energy-path-inspector],[data-energy-path-link-inspector]"),"Escape did not clear selection/inspector");assertRetained(clearScene,"Escape clear");assertNoWork(beforeClear,"Escape clear");
   activateNode(cooling,"blank-clear setup");const blankBefore=counters(),blankScene=retain(),canvas=host.querySelector("[data-energy-path-canvas]");canvas.dispatchEvent(new MouseEvent("click",{bubbles:true,cancelable:true}));check(state.simulationEnergySelection===""&&document.activeElement===canvas,"blank canvas clear did not retain focus");assertRetained(blankScene,"blank clear");assertNoWork(blankBefore,"blank clear");
   const people=initialGraph.nodes.find(item=>item.driverCategory==="internal.people");activateNode(people,"metadata refresh setup");const previousInspector=host.querySelector("[data-energy-path-inspector]"),navigationBuilds=()=>globalThis.epath161InspectorCalls?.prepareEnergyPathDriverNavigation||0,beforeWrapper=navigationBuilds();state.report={...state.report};activateNode(people,"same selection fresh report metadata");check(host.querySelector("[data-energy-path-inspector]")!==previousInspector,"fresh report did not refresh same-node inspector");check(navigationBuilds()===beforeWrapper,"report wrapper replacement rebuilt unchanged model navigation indexes");evidence.push("Report wrapper navigation rebuilds="+(navigationBuilds()-beforeWrapper));
   const modelChanges=[
    ["geometry",()=>{state.report={...state.report,geometry:{...state.report.geometry}};}],
    ["profile",()=>{state.report={...state.report,profile:{...state.report.profile}};}],
    ["HVAC",()=>{state.report={...state.report,hvac:{...state.report.hvac}};}],
    ["projection",()=>{state.semanticProjection={...state.semanticProjection};}],
    ["navigation",()=>{state.semanticProjection={...state.semanticProjection,navigation:{...state.semanticProjection.navigation}};}],
    ["entities",()=>{state.semanticProjection={...state.semanticProjection,navigation:{...state.semanticProjection.navigation,entities:[...state.semanticProjection.navigation.entities]}};}],
    ["occurrences",()=>{state.semanticProjection={...state.semanticProjection,navigation:{...state.semanticProjection.navigation,occurrences:[...state.semanticProjection.navigation.occurrences]}};}],
    ["analysis key",()=>{state.reportAnalysisKey+="-metadata161";}],
    ["last analyzed key",()=>{state.lastAnalyzedKey="epath161-new-last-analyzed-key";}]
   ];
   for(const[label,changeModel]of modelChanges){const beforeBuilds=navigationBuilds(),beforeWork=counters(),priorInspector=host.querySelector("[data-energy-path-inspector]");changeModel();simulation.renderSimulationEnergyDashboard(state.simulationResult);check(navigationBuilds()===beforeBuilds+1,label+" replacement did not invalidate model navigation exactly once");check(host.querySelector("[data-energy-path-inspector]")!==priorInspector&&state.simulationEnergySelection===people.id,label+" replacement lost current inspector/selection");assertNoWork(beforeWork,label+" model-only invalidation");const currentInspector=host.querySelector("[data-energy-path-inspector]");node(people.id).click();check(navigationBuilds()===beforeBuilds+1&&host.querySelector("[data-energy-path-inspector]")===currentInspector,label+" replacement failed to retain the newly prepared index/inspector");}
   evidence.push("Actual model/projection/navigation/key replacements invalidated navigation exactly once; following same-node clicks retained fresh inspector/index");
   const workspace=simulation.captureSimulationEnergyWorkspaceContext(),beforeRestore=counters();simulation.restoreSimulationEnergyWorkspaceContext({...workspace,energyDrawer:{tab:"data",stage:"drivers",outputSource:""}});simulation.renderSimulationEnergyDashboard(state.simulationResult);assertNoWork(beforeRestore,"same-context workspace drawer restore");
   const snapshot=history.captureViewSnapshot().panelContexts.simulation,beforeHistory=counters();await simulation.restoreSimulationNavigationContext(snapshot,{genericRestoreContext:async()=>true});const historyDelta=changed(beforeHistory,counters());check(!historyDelta.prepareEnergyPathScene&&!historyDelta.energyPathLayout&&!historyDelta.energyPathRibbons,"same-context adapter history recomputed scene/layout/ribbons: "+JSON.stringify(historyDelta));
   const invalidate=(label,action)=>{const old=host.querySelector("[data-energy-path-canvas]"),before=counters();action();check(host.querySelector("[data-energy-path-canvas]")!==old,label+" failed to invalidate actual graph DOM");const delta=changed(before,counters());check(delta.prepareEnergyPathScene>0&&delta.energyPathLayout>0&&delta.energyPathRibbons>0,label+" reused stale graph geometry: "+JSON.stringify(delta));};
   invalidate("scope",()=>change("[data-simulation-energy-scope]","zone"));if(state.simulationEnergyZoneName!==zones[0])invalidate("Zone",()=>change("[data-simulation-energy-zone-name]",zones[0]));
   invalidate("period",()=>change("[data-simulation-energy-path-period]","M1"));invalidate("service",()=>change("[data-simulation-energy-service]","cooling"));
   const scoped=graph().nodes.find(item=>item.level==="load"&&item.serviceKind==="cooling");activateNode(scoped,"actual Office Zone M1 cooling");
   const next=JSON.parse(rawJSON);next.runId="epath161-replaced-result";invalidate("new result identity",()=>{state.simulationResult=freeze(next);simulation.renderSimulationEnergyDashboard(state.simulationResult);});
   check(JSON.stringify(result)===rawJSON&&JSON.stringify(fixture)===modelJSON,"selection or scene invalidation mutated raw Energy / actual Office metadata");
   const sorted=latencies.map(item=>item.ms).sort((a,b)=>a-b);evidence.push("Native-clock selection latency n="+latencies.length+", p95="+sorted[Math.floor((sorted.length-1)*.95)].toFixed(2)+"ms, max="+sorted.at(-1).toFixed(2)+"ms; "+latencies.map(item=>item.label+"="+item.ms.toFixed(2)).join("; "));
   evidence.push("Graph/header/KPI/quality/ribbon/hit/ratio DOM retained, no projection/layout work on selections or drawer actions; context/result invalidation rebuilt geometry");await report();
  }
 }
}catch(error){failures.push(error.stack||String(error));await report();}
</script>`
