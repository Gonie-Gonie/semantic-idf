package frontendchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEPATH204HistoryPreservesEnergyWithRetainedAnalyzedZoneBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser actual analyzed semantic selection history")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	const input = `Version, 25.1;
Zone, Office, 0,0,0,0,1,1;
Material:NoMass, Insulation, Rough, 2;
Construction, Wall Construction, Insulation;
BuildingSurface:Detailed, Office Wall, Wall, Wall Construction, Office, , Outdoors, , SunExposed, WindExposed, 0.5, 4,
  0,0,0, 0,0,3, 4,0,3, 4,0,0;
`
	doc, err := idf.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"input": input,
		"report": map[string]any{
			"geometry": idf.AnalyzeGeometry(doc), "profile": idf.AnalyzeProfile(doc), "hvac": idf.AnalyzeHVAC(doc), "output": map[string]any{"existing": []any{}},
		},
		"navigation": idf.BuildSemanticModel(doc, idf.SemanticYAMLMetadata{}).Navigation,
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
	page = strings.Replace(page, "</body>", epath142LayoutHTML+epath204SemanticHistoryHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/epath204-semantic.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	mux.HandleFunc("/epath204-analyzed.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--window-size=1600,900", "--virtual-time-budget=15000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/src/epath204-semantic.html?manual=1").CombinedOutput()
	if err != nil {
		t.Fatalf("semantic history browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath204-semantic="passed"`) {
		diagnostic := regexp.MustCompile(`(?s)<pre id="epath204-semantic-result"[^>]*>(.*?)</pre>`).FindStringSubmatch(string(output))
		if len(diagnostic) == 2 {
			t.Fatalf("retained semantic Zone history: %s", diagnostic[1])
		}
		t.Fatalf("retained semantic Zone history:\n%s", output)
	}
}

const epath204SemanticHistoryHTML = `<pre id="epath204-semantic-result" hidden>pending</pre><script type="module">
try {
 const check=(value,message)=>{if(!value)throw Error(message);};
 for(let attempt=0;document.body.dataset.epath142Status!=='manual'&&attempt<300;attempt++)await new Promise(resolve=>setTimeout(resolve,10));
 check(document.body.dataset.epath142Status==='manual','actual app bootstrap failed');
 const [store,simulation,adapters,controller,history,navigation]=await Promise.all([import('/src/js/state.js'),import('/src/js/views/simulation-views.js'),import('/src/js/panel-navigation-adapters.js'),import('/src/js/selection-controller.js'),import('/src/js/view-history.js'),import('/src/js/navigation.js')]);
 const fixture=await(await fetch('/epath204-analyzed.json')).json(),{state}=store;
 store.setDocumentText(fixture.input);
 state.report=fixture.report;state.semanticProjection={navigation:fixture.navigation};
 state.analysisKey=state.reportAnalysisKey='epath204-analyzed-zone';state.reportAnalyzedText=fixture.input;
 state.analysisDirty={metrics:false,topology:false,profile:false,hvac:false,simulation:false};state.analysisReady={topology:true,profile:true,hvac:true,simulation:true};state.geometryReady=true;
 let analyze=0,run=0,queued=0,opened=0;
 for(const name of ['AnalyzeInputText','AnalyzeInputDiagnosticsText'])window.go.main.App[name]=async()=>{analyze++;throw Error('unexpected '+name);};
 for(const name of ['RunSimulation','RunSimulationText','RunPurposeSimulationText'])window.go.main.App[name]=async()=>{run++;throw Error('unexpected '+name);};
 adapters.initializeResultPanelNavigationAdapters();
 controller.configureSelectionController({state,getNavigationIndex:()=>state.semanticProjection.navigation,getCurrentText:store.getDocumentText,getReportAnalysisKey:()=>state.reportAnalysisKey,isAnalysisCurrent:()=>state.reportAnalyzedText===store.getDocumentText(),getActiveInputView:()=>"input-"+(state.activeInputView||"semantic"),getActivePanelView:()=>state.activeResultTab,
  recordHistory:payload=>{const snapshot=history.captureViewSnapshot();if(payload.previous)snapshot.globalSelection=payload.previous;history.recordViewHistory(snapshot);},
  openView:async(view,options)=>{opened++;return navigation.switchResultTab(view,{...options,recordHistory:false});},queueAnalysisTarget:()=>{queued++;},
  onSelectionChange:detail=>window.dispatchEvent(new CustomEvent('idfAnalyzer:semanticSelectionChanged',{detail}))});
 const zones=fixture.navigation.entities.filter(entity=>entity.kind==='zone'&&entity.label==='Office');
 check(zones.length===1,'actual analyzed Office semantic entity is missing/ambiguous');
 await controller.selectSemanticEntity({entityId:zones[0].id,entityKind:zones[0].kind},{originView:'topology',recordHistory:false,follow:false});
 check(state.globalSelection.entityId===zones[0].id&&state.activeResultTab==='simulation'&&opened===0,'nonfollowing actual Zone selection must remain retained without opening another view');
 const host=document.getElementById('simulationEnergyDashboard'),original=JSON.stringify(state.simulationResult);
 simulation.renderSimulationEnergyDashboard(state.simulationResult);
 const context=()=>JSON.stringify(simulation.captureSimulationEnergyWorkspaceContext());
 const change=(attribute,value)=>{const control=host.querySelector('['+attribute+']');check(control,'missing actual Energy control '+attribute);control.focus();control.value=value;control.dispatchEvent(new Event('input',{bubbles:true}));control.dispatchEvent(new Event('change',{bubbles:true}));};
 const select=()=>{const node=host.querySelector('[data-energy-path-layout-node]');check(node,'actual graph has no native node');node.focus();node.click();host.querySelector('[data-energy-path-details-toggle]').click();return state.simulationEnergySelection;};
 const selection=select();
 check(selection&&state.simulationEnergyScopeKind==='building'&&state.simulationEnergyPeriod==='annual','fixture must start selected Building/Annual');
 state.navigationUndoStack=[];state.navigationRedoStack=[];
 const beforeScope=context();
 change('data-simulation-energy-scope','zone');
 const afterScope=context();
 check(state.simulationEnergyScopeKind==='zone'&&state.navigationUndoStack.length===1,'scope input-first transaction must record once');
 await navigation.undoViewNavigation({quiet:true});
 check(context()===beforeScope,'retained semantic Zone overrode restored Building scope/selection/drawer\nexpected '+beforeScope+'\nactual '+context());
 check(state.globalSelection.entityId===zones[0].id,'Back must retain semantic Zone identity without following it');
 check(document.activeElement===host.querySelector('[data-simulation-energy-scope]'),'Back lost actual Scope control focus');
 await navigation.redoViewNavigation({quiet:true});
 check(context()===afterScope&&state.navigationUndoStack.length===1&&state.navigationRedoStack.length===0,'Redo re-followed the retained Zone or changed selected-state transaction');
 await navigation.undoViewNavigation({quiet:true});
 check(context()===beforeScope,'second Back must restore cached Building selection again');
 state.navigationUndoStack=[];state.navigationRedoStack=[];
 const beforeMonth=context();
 change('data-simulation-energy-path-period','M1');
 const afterMonth=context();
 check(state.simulationEnergyPeriod==='M1'&&state.navigationUndoStack.length===1,'month input-first transaction must record once');
 await navigation.undoViewNavigation({quiet:true});
 check(context()===beforeMonth,'retained semantic Zone overrode restored Annual scope/selection/drawer\nexpected '+beforeMonth+'\nactual '+context());
 check(document.activeElement===host.querySelector('[data-simulation-energy-path-period]'),'Annual Back lost Period control focus');
 await navigation.redoViewNavigation({quiet:true});
 check(context()===afterMonth&&state.globalSelection.entityId===zones[0].id,'M1 Redo changed retained Energy/semantic selection');
 check(JSON.stringify(state.simulationResult)===original&&analyze===0&&run===0&&queued===0&&opened===0,'history mutated cached result or performed analysis/run/navigation side effects');
 document.body.dataset.epath204Semantic='passed';document.getElementById('epath204-semantic-result').textContent='PASS: actual analyzed Zone retained while Building/Annual control Back/Redo restores exact Energy state and focus; no Analyze/Run';
}catch(error){document.body.dataset.epath204Semantic='failed';document.getElementById('epath204-semantic-result').textContent=error.stack||String(error);}
</script>`
