package main

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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Opt-in display diagnosis: an unchanged compact result is partitioned by the
// production encoder and read through the production resource endpoint. No SQL
// is read and no engine or result builder is started by this browser replay.
func TestSimulationActualPartsTabSwitchBrowser(t *testing.T) {
	payloadPath := os.Getenv("SIMULATION_TAB_REPLAY_PAYLOAD")
	if payloadPath == "" {
		t.Skip("set SIMULATION_TAB_REPLAY_PAYLOAD and SIMULATION_TAB_REPLAY_INPUT for actual multipart tab timing")
	}
	inputPath := os.Getenv("SIMULATION_TAB_REPLAY_INPUT")
	if inputPath == "" {
		t.Fatal("explicit executed input is required")
	}
	payload, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	originalHash := sha256.Sum256(payload)
	t.Cleanup(func() {
		file, err := os.Open(payloadPath)
		if err != nil {
			t.Error(err)
			return
		}
		defer file.Close()
		current, statErr := file.Stat()
		hash := sha256.New()
		size, readErr := io.Copy(hash, file)
		if statErr != nil || readErr != nil || size != info.Size() || !current.ModTime().Equal(info.ModTime()) || hex.EncodeToString(hash.Sum(nil)) != hex.EncodeToString(originalHash[:]) {
			t.Error("actual replay changed the preserved payload")
		}
	})
	var identity struct {
		Result struct {
			RunID string `json:"runId"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &identity); err != nil || identity.Result.RunID == "" {
		t.Fatalf("expected complete compact simulation transport: %v", err)
	}
	app := NewApp()
	partsStarted := time.Now()
	parts, err := app.simulationPartsForPayload(payload, identity.Result.RunID, "actual-tab-replay")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("production partition preparation: %s; payload=%d manifest=%d resources=%d", time.Since(partsStarted), len(payload), len(parts.manifest), len(parts.resources))
	input, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := idf.Parse(string(input))
	if err != nil {
		t.Fatal(err)
	}
	model, err := json.Marshal(map[string]any{"geometry": idf.AnalyzeGeometry(doc), "hvac": idf.AnalyzeHVAC(doc)})
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join("frontend", "src")
	if override := os.Getenv("SIMULATION_TAB_REPLAY_SOURCE_OVERRIDES"); override != "" {
		source = override
	}
	page, err := os.ReadFile(filepath.Join(source, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	pageText := string(page)
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		if !strings.Contains(pageText, script) {
			t.Fatalf("actual app bootstrap changed: %s", script)
		}
		pageText = strings.Replace(pageText, script, "", 1)
	}
	pageText = strings.Replace(pageText, "</body>", simulationActualPartsTabsHTML+"</body>", 1)
	rounds := 2
	if value := os.Getenv("SIMULATION_TAB_REPLAY_ROUNDS"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 4 {
			t.Fatal("SIMULATION_TAB_REPLAY_ROUNDS must be between one and four")
		}
		rounds = parsed
	}
	type browserResult struct {
		Failures []string       `json:"failures"`
		Evidence map[string]any `json:"evidence"`
	}
	done := make(chan browserResult, 1)
	var mu sync.Mutex
	counts := map[string]int{}
	var transferred int64
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(source))))
	mux.HandleFunc("/src/actual-tabs.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, pageText)
	})
	mux.HandleFunc("/actual/model", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(model) })
	mux.HandleFunc("/actual/manifest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", simulationResultPartsMediaType)
		_, _ = w.Write(parts.manifest)
	})
	mux.HandleFunc("/actual/config", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"rounds": rounds})
	})
	mux.HandleFunc("/api/simulation-result-part", func(w http.ResponseWriter, r *http.Request) {
		counter := &simulationActualTabCountingWriter{ResponseWriter: w}
		app.serveSimulationResultPart(counter, r)
		mu.Lock()
		counts[r.URL.Query().Get("partId")]++
		transferred += counter.bytes
		mu.Unlock()
	})
	mux.HandleFunc("/actual/stats", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests, duplicates := 0, 0
		for _, count := range counts {
			requests += count
			if count > 1 {
				duplicates += count - 1
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"requests": requests, "unique": len(counts), "duplicates": duplicates, "bytes": transferred})
	})
	mux.HandleFunc("/actual/done", func(w http.ResponseWriter, r *http.Request) {
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
	chrome := simulationActualTabChrome()
	if chrome == "" {
		t.Fatal("actual tab replay requires Chrome/Chromium/Edge")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--enable-precise-memory-info", "--window-size=1600,1000", "--user-data-dir="+t.TempDir(), server.URL+"/src/actual-tabs.html")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			if runtime.GOOS == "windows" {
				_ = exec.Command("taskkill.exe", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F").Run()
			} else {
				_ = command.Process.Kill()
			}
			_ = command.Wait()
		})
	}
	t.Cleanup(stop)
	select {
	case result := <-done:
		stop()
		result.Evidence["payloadBytes"], result.Evidence["payloadSHA256"] = len(payload), hex.EncodeToString(originalHash[:])
		result.Evidence["sourceOverrides"] = os.Getenv("SIMULATION_TAB_REPLAY_SOURCE_OVERRIDES") != ""
		encoded, _ := json.Marshal(result)
		t.Log(string(encoded))
		if output := os.Getenv("SIMULATION_TAB_REPLAY_OUTPUT"); output != "" {
			file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			_, writeErr := file.Write(encoded)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				t.Fatalf("save display evidence: %v / %v", writeErr, closeErr)
			}
		}
		if len(result.Failures) > 0 {
			t.Fatal(strings.Join(result.Failures, "\n"))
		}
	case <-ctx.Done():
		stop()
		t.Fatal("actual multipart tab replay did not report within eight minutes")
	}
}

type simulationActualTabCountingWriter struct {
	http.ResponseWriter
	bytes int64
}

func (w *simulationActualTabCountingWriter) Write(value []byte) (int, error) {
	n, err := w.ResponseWriter.Write(value)
	w.bytes += int64(n)
	return n, err
}

func simulationActualTabChrome() string {
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser", "msedge", "chrome"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	for _, path := range []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
	} {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return ""
}

const simulationActualPartsTabsHTML = `<script>window.go={main:{App:{GetSimulationEnvironment:async()=>({installations:[{version:'24.2',executablePath:'C:/captured/EnergyPlus/energyplus.exe'}],weatherFolders:[]})}}};window.runtime={EventsOn(){}};</script>
<script type="module">
const evidence={steps:[]},failures=[];
const check=(value,message)=>{if(!value)failures.push(message);};
const frame=()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
const tick=()=>new Promise(resolve=>setTimeout(resolve,5));
const heap=()=>performance.memory?.usedJSHeapSize??null;
const stats=()=>fetch('/actual/stats').then(response=>response.json());
try{
 const [storeModule,simulation,layout,navigation,partsModule]=await Promise.all([import('/src/js/state.js'),import('/src/js/views/simulation-views.js'),import('/src/js/layout.js'),import('/src/js/navigation.js'),import('/src/js/simulation-result-parts.js')]);
 const model=await(await fetch('/actual/model')).json(),config=await(await fetch('/actual/config')).json();
 const {state}=storeModule;storeModule.setDocumentText('Version,24.2;\nBuilding,Actual captured result;');
 layout.initializeWorkspaceSplitter();simulation.initializeSimulationControls();await tick();
 const decodeStarted=performance.now(),manifest=await(await fetch('/actual/manifest')).json();
 const result=await partsModule.loadSimulationResultParts(manifest);evidence.manifestDecodeMS=performance.now()-decodeStarted;
 const resources=partsModule.simulationResultStore(result);check(Boolean(resources),'production parts result store absent');
 let pending=0;
 for(const name of ['read','ensure']){const original=resources[name];resources[name]=function(...args){const foreground=name!=='read'||!(args[1]?.priority>0);if(foreground)pending++;try{return Promise.resolve(original.apply(this,args)).finally(()=>{if(foreground)pending--;});}catch(error){if(foreground)pending--;throw error;}};}
 Object.assign(state,{report:model,simulationRunning:false,simulationResult:result,simulationActiveRunID:result.runId,simulationProgress:{status:'succeeded',percent:100,message:'Captured result'},simulationActiveResultView:'energy',analysisDirty:{metrics:false,topology:false,profile:false,hvac:false,simulation:false},geometryReady:true});
 state.reportAnalysisKey=state.analysisKey='actual-tab-model';state.reportAnalyzedText=storeModule.getDocumentText();
 const hosts={energy:'simulationEnergyDashboard',zone_heat_flow:'simulationHeatFlow',hvac_loops:'simulationHVACLoopResults',comfort:'simulationComfortResults'};
 const selector={energy:'[data-energy-path-canvas]',zone_heat_flow:'.heatflow-floor-grid svg',hvac_loops:'svg',comfort:'.simulation-comfort-card,svg'};
 const currentHost=view=>document.getElementById(hosts[view]);
 const settled=async view=>{const started=performance.now();while(pending&&performance.now()-started<120000)await tick();if(pending)throw Error('foreground resources did not settle for '+view);await frame();check(state.simulationActiveResultView===view,'latest view not retained: '+view);const host=currentHost(view);check(host&&!host.closest('[data-simulation-result-view]').hidden&&host.querySelector(selector[view]),'actual view not rendered: '+view);};
 const initialStarted=performance.now();await resources.ensure('energy',state);navigation.switchResultTab('simulation',{recordHistory:false});await simulation.renderSimulation();await settled('energy');
 evidence.initialEnergyMS=performance.now()-initialStarted;evidence.initial=await stats();evidence.heapAfterEnergy=heap();
 const retained=new Map();retained.set('energy',currentHost('energy').firstElementChild);
 const click=async(view,label)=>{const before=await stats(),started=performance.now();document.querySelector('[data-simulation-result-view-button="'+view+'"]').click();const handlerMS=performance.now()-started;await settled(view);const readyMS=performance.now()-started,after=await stats(),host=currentHost(view);evidence.steps.push({label,view,handlerMS,readyMS,resourceRequests:after.requests-before.requests,resourceBytes:after.bytes-before.bytes,reused:retained.get(view)===host.firstElementChild,dom:host.querySelectorAll('*').length,heap:heap()});retained.set(view,host.firstElementChild);};
 for(const view of ['zone_heat_flow','hvac_loops','comfort'])await click(view,'first');
 // Hydrate before warm switches to separate rebuilding from first-use reads.
 const hydrateStarted=performance.now();await resources.ensureAll();await settled(state.simulationActiveResultView);evidence.ensureAllMS=performance.now()-hydrateStarted;evidence.hydrated=await stats();
 for(let round=0;round<config.rounds;round++)for(const view of ['energy','zone_heat_flow','hvac_loops','comfort'])await click(view,'warm-'+round);
 evidence.final=await stats();evidence.heapFinal=heap();evidence.clock='native performance.now, real requestAnimationFrame; no virtual time';
 check(evidence.final.duplicates===0,'a result resource was fetched more than once');
 check(evidence.steps.filter(step=>step.label.startsWith('warm')).every(step=>step.resourceRequests===0),'warm tab switch fetched a result resource');
 resources.dispose();
}catch(error){failures.push(error.stack||String(error));}
await fetch('/actual/done',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({failures,evidence})});
</script>`
