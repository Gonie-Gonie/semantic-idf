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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

func TestSimulationHTTPResultTransportBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("native Run & Inspect transport regression")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	page := readTestFile(t, "frontend/src/index.html")
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		if !strings.Contains(page, script) {
			t.Fatalf("actual bootstrap changed: %s", script)
		}
		page = strings.Replace(page, script, "", 1)
	}
	page = strings.Replace(page, "</body>", epath142LayoutHTML+simulationHTTPResultHTML+"</body>", 1)
	inputPath := filepath.Join(t.TempDir(), "http-result.idf")
	if err := os.WriteFile(inputPath, []byte(simulationPurposeResultsHVACInput), 0o644); err != nil {
		t.Fatal(err)
	}
	type browserResult struct {
		Failures []string       `json:"failures"`
		Evidence map[string]any `json:"evidence"`
	}
	done := make(chan browserResult, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var mu sync.Mutex
	var runID string
	runCalls, cacheCalls := 0, 0
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/http-result.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	mux.HandleFunc("/api/simulation-run", func(w http.ResponseWriter, r *http.Request) {
		var request simulation.SimulationRunRequest
		if r.Method != http.MethodPost || r.Header.Get("Accept") != "application/vnd.semantic-idf.simulation-transfer+json" || json.NewDecoder(r.Body).Decode(&request) != nil || request.PurposeRequest == nil {
			http.Error(w, "invalid run request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		runCalls++
		calls := runCalls
		runID = request.RunID
		mu.Unlock()
		if calls > 1 {
			http.Error(w, "transport failure after run started", http.StatusBadGateway)
			return
		}
		result := simulationPurposeResultsBrowserFixture(request.RunID, inputPath, *request.PurposeRequest)
		result.Filename = "HTTP \"quoted\" \\ result </script>.idf"
		// Exercise the production compact encoder and lazy decoder with a full
		// hourly series, including its duplicate points in the purpose bundle.
		for i := range result.Series {
			series := &result.Series[i]
			original := series.Points
			series.Points = make([]simulation.SimulationPoint, 240)
			for frame := range series.Points {
				series.Points[frame] = simulation.SimulationPoint{X: frame, Label: fmt.Sprintf("01/%02d %02d:00:00", frame/24+1, frame%24+1), Value: original[frame%len(original)].Value}
			}
			series.RowCount = len(series.Points)
		}
		bundle := simulation.BuildPurposeResultBundle(&result, *request.PurposeRequest)
		result.PurposeResults = &bundle
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-release:
			_ = json.NewEncoder(w).Encode(simulation.CompactResultTransport(&result))
		case <-r.Context().Done():
		}
	})
	mux.HandleFunc("/api/simulation-result-cache", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			TextHash string `json:"textHash"`
			RunID    string `json:"runId"`
		}
		if r.Method != http.MethodPost || r.Header.Get("Accept") != "application/vnd.semantic-idf.simulation-transfer+json" || json.NewDecoder(r.Body).Decode(&request) != nil {
			http.Error(w, "invalid cache request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		cacheCalls++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if request.TextHash == "exact-hash" && request.RunID == "cached-http" {
			_, _ = fmt.Fprint(w, `{"runId":"cached-http","status":"succeeded","series":[]}`)
		} else {
			_, _ = fmt.Fprint(w, "null")
		}
	})
	mux.HandleFunc("/http-result/status", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"runId": runID, "runCalls": runCalls, "cacheCalls": cacheCalls})
	})
	mux.HandleFunc("/http-result/release", func(w http.ResponseWriter, _ *http.Request) {
		releaseOnce.Do(func() { close(release) })
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/http-result/done", func(w http.ResponseWriter, r *http.Request) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	profile := t.TempDir()
	command := exec.Command(chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0", "--window-size=1600,900", "--user-data-dir="+profile, server.URL+"/src/http-result.html?manual=1")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	stop := epath161OwnedChromeCleanup(t, command, profile)
	t.Cleanup(stop)
	select {
	case result := <-done:
		stop()
		evidence, _ := json.Marshal(result.Evidence)
		t.Log(string(evidence))
		if len(result.Failures) != 0 {
			t.Fatal(strings.Join(result.Failures, "\n"))
		}
	case <-ctx.Done():
		stop()
		t.Fatal("native HTTP result browser did not report within its deadline")
	}
}

const simulationHTTPResultHTML = `<script type="module">
const failures=[],evidence={};
const check=(condition,message)=>{if(!condition)failures.push(message);};
const tick=()=>new Promise(resolve=>setTimeout(resolve,0));
try{
 for(let n=0;document.body.dataset.epath142Status!=="manual"&&n<200;n++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath142Status!=="manual")throw Error("actual fixture not ready");
 const {state}=await import("/src/js/state.js"),simulation=await import("/src/js/views/simulation-views.js");
 const {decodeSimulationResultTransfer:decode}=await import("/src/js/simulation-result-transport.js");
 const ordinary={series:[{points:[]}]};check(decode(ordinary)===ordinary&&decode(null)===null,"ordinary results lost identity");
 const wire=()=>({schema:"semantic-idf.simulation-transfer/v1",result:{series:[{points:null},{points:null},{points:[],displayPoints:null}],purposeResults:{hvacLoops:[{components:[{series:[{points:null}]}]}]}},timelines:[{x:[9,1,9],labels:["","first","duplicate"]}],pointSets:[{timeline:0,values:[0,-3.25,-0]},{timeline:0,values:[4.125,7,8]}],pointBindings:[{path:["series",0,"points"],data:0},{path:["series",0,"displayPoints"],data:0},{path:["series",1,"points"],data:1},{path:["purposeResults","hvacLoops",0,"components",0,"series",0,"points"],data:0}]});
 const envelope=wire(),decoded=decode(envelope),descriptor=(owner,key)=>Object.getOwnPropertyDescriptor(owner,key);
 check(typeof descriptor(decoded.series[0],"points").get==="function"&&typeof descriptor(decoded.series[1],"points").get==="function","decoder eagerly materialized unselected series");
 const points=decoded.series[0].points;
 check(points.length===3&&points[0].x===9&&!Object.hasOwn(points[0],"label")&&points[1].x===1&&points[1].value===-3.25&&points[2].x===9&&Object.is(points[2].value,-0),"decoder changed zeros, signs, decimals, order, duplicate x or optional labels");
 check(decoded.series[0].displayPoints===points&&decoded.purposeResults.hvacLoops[0].components[0].series[0].points===points,"duplicate bindings did not share one point array");
 check(typeof descriptor(decoded.series[1],"points").get==="function"&&envelope.pointSets[0].values===null&&envelope.pointSets[1].values.length===3,"reading one series expanded another series or retained its redundant value column");
 check(Array.isArray(decoded.series[2].points)&&decoded.series[2].points.length===0&&decoded.series[2].displayPoints===null,"empty and null arrays changed");
 check(JSON.parse(JSON.stringify(decoded)).series[1].points[0].value===4.125,"enumerable lazy points were omitted from JSON export");
 for(const change of [
  item=>item.pointBindings[0].path=["__proto__","polluted"],
  item=>item.pointBindings[0].path=["constructor","prototype","polluted"],
  item=>item.pointBindings[0].path=["series",999,"points"],
  item=>item.pointBindings[0].path=["series","0","points"],
  item=>item.pointBindings[0].data=99,
  item=>item.pointSets[0].timeline=-1,
  item=>item.pointSets[0].values.pop(),
  item=>item.timelines[0].labels.pop(),
  item=>item.pointBindings.push({...item.pointBindings[0]}),
 ]){const bad=wire();change(bad);let rejected=false;try{decode(bad);}catch{rejected=true;}check(rejected,"decoder accepted invalid binding or columns");}
 check(Object.prototype.polluted===undefined,"decoder wrote through prototype path");
 const api=window.go.main.App,button=document.getElementById("simulationRunButton"),status=document.getElementById("simulationStatus");
 let bridgeRuns=0,bridgeCache=0;
 api.RunPurposeSimulationText=async()=>{bridgeRuns++;throw Error("unexpected desktop run");};
 api.GetCachedSimulationResult=async()=>{bridgeCache++;throw Error("unexpected desktop cache");};
 state.simulationEnvironment={...state.simulationEnvironment,resultHTTPAvailable:true};
 state.simulationActiveResultView="hvac_loops";
 document.querySelector('[data-simulation-purpose="hvac_loop_check"]').click();
 for(const control of document.querySelectorAll("[data-simulation-purpose]"))if(control.checked&&control.dataset.simulationPurpose!=="hvac_loop_check")control.click();
 const original=state.simulationResult;
 button.click();
 let serverState;
 for(let n=0;n<200;n++){serverState=await(await fetch("/http-result/status")).json();if(serverState.runCalls)break;await tick();}
 check(serverState.runCalls===1&&bridgeRuns===0,"capable desktop did not issue exactly one HTTP run");
 window.dispatchEvent(new CustomEvent("idfAnalyzer:simulationProgress",{detail:{runId:serverState.runId,phase:"complete",status:"succeeded",percent:100,message:"Backend complete"}}));
 await tick();
 check(status.dataset.simulationProgressPhase==="receiving_results"&&state.simulationRunning&&button.disabled&&state.simulationResult===original,"HTTP body pending lost progress/result boundary");
 await fetch("/http-result/release");
 for(let n=0;state.simulationRunning&&n<1000;n++)await tick();
 const received=state.simulationResult;
 check(received!==original&&received.runId===serverState.runId&&state.simulationProgress.status==="succeeded","HTTP result was not installed successfully");
 check(received.filename==='HTTP "quoted" \\ result </'+'script>.idf',"JSON text was changed during transfer");
 check(received.series[0].points.length===240&&received.series[0].points[2].value===18&&received.series[0].points[239].value===18,"original result points were truncated or altered");
 document.querySelector('[data-simulation-result-view-button="hvac_loops"]').click();await tick();
 evidence.hvac={view:state.simulationActiveResultView,loops:received.purposeResults?.hvacLoops?.length,series:received.purposeResults?.hvacLoops?.map(loop=>loop.series?.length),svgs:document.querySelectorAll('#simulationHVACLoopResults svg').length,frame:Boolean(document.querySelector('#simulationHVACLoopResults [data-simulation-hvac-frame]'))};
 check(document.querySelector('#simulationHVACLoopResults [data-simulation-hvac-frame]')&&document.querySelectorAll('#simulationHVACLoopResults svg').length>=4,"HTTP HVAC result did not render native topology and graphs: "+JSON.stringify(evidence.hvac));
 const cached=await simulation.loadCachedSimulationResult("exact-hash","cached-http"),missing=await simulation.loadCachedSimulationResult("other-hash","cached-http");
 check(cached?.runId==="cached-http"&&missing===null&&bridgeCache===0,"HTTP cache restore changed exact-match/miss behavior or used desktop bridge");
 button.click();
 for(let n=0;state.simulationRunning&&n<1000;n++)await tick();
 serverState=await(await fetch("/http-result/status")).json();
 check(serverState.runCalls===2&&bridgeRuns===0&&state.simulationProgress.phase==="request_failed"&&status.textContent.includes("transport failure after run started"),"failed HTTP run retried through bridge or hid its error");
 check(state.simulationResult===received&&!button.disabled,"HTTP failure discarded previous result or prevented explicit rerun");
 state.simulationEnvironment.resultHTTPAvailable=false;
 api.RunPurposeSimulationText=async request=>{bridgeRuns++;return {...received,runId:request.runId};};
 api.GetCachedSimulationResult=async(hash,id)=>{bridgeCache++;return hash==="exact-hash"&&id==="cached-bridge"?{runId:id}:null;};
 button.click();for(let n=0;state.simulationRunning&&n<1000;n++)await tick();
 const legacy=await simulation.loadCachedSimulationResult("exact-hash","cached-bridge");
 serverState=await(await fetch("/http-result/status")).json();
 check(bridgeRuns===1&&bridgeCache===1&&legacy?.runId==="cached-bridge"&&serverState.runCalls===2,"backend-only capability fallback did not preserve typed calls");
 evidence.httpRunCalls=serverState.runCalls;evidence.httpCacheCalls=serverState.cacheCalls;evidence.explicitLegacyBridgeRuns=bridgeRuns;evidence.explicitLegacyCacheReads=bridgeCache;
 evidence.checks="production compact encoder, lazy shared series decoder, native Run button, delayed response progress, HVAC render, full values/escaped text, exact cache hit/miss, no automatic run retry, backend-only compatibility";
}catch(error){failures.push(error.stack||String(error));}
await fetch("/http-result/done",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({failures,evidence})});
</script>`
