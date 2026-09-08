package frontendchecks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Opt-in: consume an already materialized, unchanged PurposeResultBundle and
// its original IDF. This measures browser transport/decode/display only; it
// does not run EnergyPlus, rebuild a result, or accept numerical oracle values.
func TestSimulationActualLargePayloadResponseAndViewsBrowser(t *testing.T) {
	payloadPath := os.Getenv("SIMULATION_UI_BUNDLE_PATH")
	if payloadPath == "" {
		t.Skip("set SIMULATION_UI_BUNDLE_PATH and SIMULATION_UI_BUNDLE_INPUT for saved actual bundle display timing")
	}
	inputPath := os.Getenv("SIMULATION_UI_BUNDLE_INPUT")
	if inputPath == "" {
		t.Fatal("SIMULATION_UI_BUNDLE_INPUT must name the original captured model")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Fatal("explicit actual-payload test requires Chrome/Chromium/Edge")
	}
	payloadPath, err := filepath.Abs(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(payloadPath)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("actual bundle must be a readable regular file: %v", err)
	}
	hashFile := func() string {
		file, err := os.Open(payloadPath)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			t.Fatal(err)
		}
		return hex.EncodeToString(hash.Sum(nil))
	}
	originalHash := hashFile()
	input, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	document, err := idf.Parse(string(input))
	if err != nil {
		t.Fatal(err)
	}
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
			t.Fatalf("actual bootstrap changed: %s", script)
		}
		page = strings.Replace(page, script, "", 1)
	}
	page = strings.Replace(page, "</body>", epath142LayoutHTML+simulationActualPayloadHTML+"</body>", 1)
	type browserResult struct {
		Failures []string       `json:"failures"`
		Evidence map[string]any `json:"evidence"`
	}
	done := make(chan browserResult, 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/actual-bundle.json", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, payloadPath) })
	mux.HandleFunc("/original-model.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(model)
	})
	mux.HandleFunc("/src/progress-actual.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	mux.HandleFunc("/actual-payload/done", func(w http.ResponseWriter, r *http.Request) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	profile := t.TempDir()
	command := exec.Command(chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0", "--force-device-scale-factor=1", "--window-size=1600,900", "--user-data-dir="+profile, server.URL+"/src/progress-actual.html?manual=1")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	stop := epath161OwnedChromeCleanup(t, command, profile)
	t.Cleanup(stop)
	select {
	case result := <-done:
		stop()
		evidence, _ := json.Marshal(result.Evidence)
		t.Logf("actual bundle bytes=%d sha256=%s native browser=%s", info.Size(), originalHash, evidence)
		if hashFile() != originalHash {
			t.Fatal("saved actual bundle changed during read-only display test")
		}
		if len(result.Failures) != 0 {
			t.Fatal(strings.Join(result.Failures, "\n"))
		}
	case <-ctx.Done():
		stop()
		t.Fatal("native-time actual-payload response/display timed out")
	}
}

const simulationActualPayloadHTML = `<script type="module">
const failures=[],evidence={};
const check=(value,message)=>{if(!value)failures.push(message);};
const tick=()=>new Promise(resolve=>setTimeout(resolve,0));
try{
 for(let n=0;document.body.dataset.epath142Status!=="manual"&&n<200;n++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath142Status!=="manual")throw Error("actual shell bootstrap failed");
 const store=await import("/src/js/state.js"),simulation=await import("/src/js/views/simulation-views.js"),{state}=store;
 const model=await(await fetch("/original-model.json")).json();
 state.report=model;state.semanticProjection={navigation:model.semanticNavigation};
 state.reportAnalyzedText=store.getDocumentText();state.reportAnalysisKey=state.analysisKey="actual-payload-model";
 state.analysisDirty={metrics:false,topology:false,profile:false,hvac:false,simulation:false};state.geometryReady=true;
 const status=document.getElementById("simulationStatus"),host=document.getElementById("simulationEnergyDashboard");
 let calls=0,responseReady=0,actualBundle;
 window.go.main.App.RunPurposeSimulationText=async request=>{
  calls++;window.dispatchEvent(new CustomEvent("idfAnalyzer:simulationProgress",{detail:{runId:request.runId,phase:"complete",status:"succeeded",percent:100,completed:9,total:9,message:"Backend build completed"}}));
  check(status.dataset.simulationProgressPhase==="receiving_results","actual payload response was prematurely marked complete");
  const started=performance.now(),response=await fetch("/actual-bundle.json");
  let text=await response.text();evidence.receiveTextMS=performance.now()-started;evidence.payloadCharacters=text.length;
  const decoding=performance.now();actualBundle=JSON.parse(text);evidence.jsonDecodeMS=performance.now()-decoding;text=null;
  check(actualBundle.energyExplanation?.schema==="semantic-idf.energy-explanation/v2","file is not an original canonical PurposeResultBundle");
  check(actualBundle.zoneHeatFlow?.zones?.length>0,"actual combined Zone Heat Flow payload absent");
  responseReady=performance.now();
  return {runId:request.runId,status:"succeeded",filename:"Actual captured Large Office",purposeResults:actualBundle,purposeRunPlan:{outputObjects:[]},series:[]};
 };
 document.getElementById("simulationRunButton").click();
 for(let n=0;(!responseReady||state.simulationRunning)&&n<12000;n++)await new Promise(resolve=>setTimeout(resolve,5));
 if(!responseReady||state.simulationRunning)throw Error("actual response/display did not finish");
 const canvas=host.querySelector("[data-energy-path-canvas]"),canvasBounds=canvas?.getBoundingClientRect();
 evidence.responseToEnergyLayoutMS=performance.now()-responseReady;
 check(state.simulationProgress.status==="succeeded"&&canvasBounds?.width>0&&canvasBounds?.height>0,"actual Energy Path failed to render");
 check(state.simulationResult.purposeResults===actualBundle,"completed handler replaced actual bundle");
 evidence.energyNodes=host.querySelectorAll("[data-energy-path-layout-node]").length;
 const heatStarted=performance.now();document.querySelector('[data-simulation-result-view-button="zone_heat_flow"]').click();
 const heat=document.getElementById("simulationHeatFlow"),plan=heat.querySelector(".heatflow-floor-grid svg"),inspector=heat.querySelector(".heatflow-inspector");
 const planBounds=plan?.getBoundingClientRect(),inspectorBounds=inspector?.getBoundingClientRect();
 evidence.heatFlowHandlerAndLayoutMS=performance.now()-heatStarted;
 evidence.heatFlowZones=actualBundle.zoneHeatFlow.zones.length;evidence.heatFlowFrames=actualBundle.zoneHeatFlow.frameCount;
 evidence.modelZones=model.geometry.zones.length;
 check(state.simulationActiveResultView==="zone_heat_flow"&&planBounds?.width>0&&inspectorBounds?.width>0,"actual HeatFlow plan/inspector failed to render");
 check(calls===1,"display caused extra backend execution");
 evidence.runCalls=calls;evidence.clock="native performance.now; no virtual time";
}catch(error){failures.push(error.stack||String(error));}
await fetch("/actual-payload/done",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({failures,evidence})});
</script>`
