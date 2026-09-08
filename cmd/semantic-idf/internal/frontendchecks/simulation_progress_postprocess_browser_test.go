package frontendchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSimulationPostprocessProgressPreservesResultDOMBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("actual simulation progress browser regression")
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
	page = strings.Replace(page, "</body>", epath142LayoutHTML+simulationPostprocessProgressHTML+"</body>", 1)
	type browserResult struct {
		Failures []string `json:"failures"`
		Evidence []string `json:"evidence"`
	}
	done := make(chan browserResult, 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/progress-postprocess.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	mux.HandleFunc("/progress-postprocess/done", func(w http.ResponseWriter, r *http.Request) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	profile := t.TempDir()
	// Elapsed UI uses performance.now and a real one-second interval. Do not
	// couple its observation to Chrome's virtual-time budget or a fixed sleep.
	command := exec.Command(chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0", "--window-size=1600,900", "--user-data-dir="+profile, server.URL+"/src/progress-postprocess.html?manual=1")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	stop := epath161OwnedChromeCleanup(t, command, profile)
	t.Cleanup(stop)
	select {
	case result := <-done:
		stop()
		for _, line := range result.Evidence {
			t.Log(line)
		}
		if len(result.Failures) != 0 {
			t.Fatalf("progress regression: %s", strings.Join(result.Failures, "\n"))
		}
	case <-ctx.Done():
		stop()
		t.Fatal("native-time progress browser did not report within its deadline")
	}
}

// Actual index, real native Run button and module handlers. Only the backend
// response is deferred; no simulation, analysis, synthetic progress timer or
// altered renderer is used to make the acceptance pass.
const simulationPostprocessProgressHTML = `<pre id="progress-postprocess-result" hidden>pending</pre><script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const tick=()=>new Promise(resolve=>setTimeout(resolve,0));
try{
 for(let n=0;document.body.dataset.epath142Status!=="manual"&&n<200;n++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath142Status!=="manual")throw Error("actual fixture not ready: "+document.getElementById("epath142-result")?.textContent);
 const {state}=await import("/src/js/state.js"),simulation=await import("/src/js/views/simulation-views.js"),i18n=await import("/src/js/i18n.js");
 const host=document.getElementById("simulationEnergyDashboard"),button=document.getElementById("simulationRunButton"),status=document.getElementById("simulationStatus"),percent=document.getElementById("simulationPercent");
 const original=state.simulationResult,originalJSON=JSON.stringify(original),pending=[];
 const activeTimers=new Set(),timerTicks=[],nativeSetInterval=window.setInterval,nativeClearInterval=window.clearInterval;
 window.setInterval=(callback,delay,...args)=>{const scheduledAt=performance.now();const id=nativeSetInterval((...values)=>{timerTicks.push({delay,scheduledAt,firedAt:performance.now()});return callback(...values);},delay,...args);activeTimers.add(id);return id;};
 window.clearInterval=id=>{activeTimers.delete(id);return nativeClearInterval(id);};
 window.go.main.App.RunPurposeSimulationText=request=>new Promise((resolve,reject)=>pending.push({request,resolve,reject}));
 const emit=(runId,phase,message="Actual backend "+phase,code="running",value=800/9)=>window.dispatchEvent(new CustomEvent("idfAnalyzer:simulationProgress",{detail:{runId,phase,message,status:code,percent:value,completed:phase==="complete"?9:8,total:9}}));
 const start=async()=>{const count=pending.length;button.click();for(let n=0;pending.length===count&&n<100;n++)await tick();if(pending.length!==count+1)throw Error("native Run did not reach backend once: "+status.textContent);return pending.at(-1);};
 const firstStartedAt=performance.now(),first=await start(),canvas=host.querySelector("[data-energy-path-canvas]"),node=host.querySelector("[data-energy-path-layout-node]");
 check(Boolean(canvas&&node),"previous result disappeared at run start");
 let mutations=0;const observer=new MutationObserver(rows=>mutations+=rows.length);observer.observe(host,{subtree:true,childList:true});
 node.focus();
 for(const phase of ["build_purpose_results","energy_geometry","energy_dashboard","energy_drivers","energy_service_paths","energy_path","zone_heat_flow","thermal_topology"]){
  emit(first.request.runId,phase);await tick();
  check(host.querySelector("[data-energy-path-canvas]")===canvas&&document.activeElement===node,"progress replaced/focused existing graph at "+phase);
  check(status.dataset.simulationProgressPhase===phase&&status.title==="Actual backend "+phase,"actual backend phase/message lost at "+phase);
  check(percent.textContent==="89%"&&status.textContent.includes("not a time estimate"),"89% incorrectly treated as a time estimate at "+phase);
  check(state.simulationResult===original&&state.simulationRunning&&button.disabled,"pending response changed result or enabled duplicate run");
 }
 check(mutations===0,"progress mutated previous result DOM "+mutations+" times");
 check(activeTimers.size===1,"progress did not own exactly one elapsed timer");
 const elapsedBefore=Number(status.dataset.simulationProgressElapsed),tickCountBefore=timerTicks.length,unchangedPhase=state.simulationProgress,observationStarted=performance.now();
 await new Promise((resolve,reject)=>{
  const changed=()=>Number(status.dataset.simulationProgressElapsed)>elapsedBefore&&timerTicks.length>tickCountBefore;
  const elapsedObserver=new MutationObserver(()=>{if(changed()){elapsedObserver.disconnect();clearTimeout(deadline);resolve();}});
  elapsedObserver.observe(status,{attributes:true,attributeFilter:["data-simulation-progress-elapsed"]});
  const deadline=setTimeout(()=>{elapsedObserver.disconnect();reject(Error("elapsed did not advance after an observed native interval within 5s: "+JSON.stringify({elapsedBefore,elapsedNow:status.dataset.simulationProgressElapsed,ticks:timerTicks,hidden:document.hidden,activeTimers:activeTimers.size,phase:state.simulationProgress.phase,waitMS:performance.now()-observationStarted})));},5000);
 });
 check(Number(status.dataset.simulationProgressElapsed)>elapsedBefore&&status.textContent.includes("Elapsed"),"elapsed clock did not advance while backend phase was unchanged");
 check(state.simulationProgress===unchangedPhase&&timerTicks.slice(tickCountBefore).some(tick=>tick.delay===1000)&&performance.now()-firstStartedAt>=1000,"elapsed display advanced without an actual one-second interval on the unchanged phase");
 evidence.push("native elapsed observation "+JSON.stringify({before:elapsedBefore,after:Number(status.dataset.simulationProgressElapsed),intervalCallbacks:timerTicks.length-tickCountBefore,waitMS:performance.now()-observationStarted,ticks:timerTicks}));
 check(host.querySelector("[data-energy-path-canvas]")===canvas&&document.activeElement===node&&mutations===0,"elapsed tick touched the previous result DOM/focus");
 const beforeStale=state.simulationProgress;emit("unrelated-run","energy_path");check(state.simulationProgress===beforeStale,"unrelated run progress accepted");
 i18n.setLanguage("ko");emit(first.request.runId,"energy_path");
 check(status.textContent.includes("Energy Path 구성 중")&&status.textContent.includes("시간 예측이 아닙니다"),"Korean phase or stage disclaimer missing");
 emit(first.request.runId,"complete","Simulation completed","succeeded",100);await tick();
 check(status.dataset.simulationProgressPhase==="receiving_results"&&status.textContent.includes("결과 수신 대기")&&!status.textContent.includes("Simulation completed"),"backend complete claimed displayed completion before response");
 check(percent.textContent==="…"&&state.simulationRunning&&button.disabled&&state.simulationResult===original,"pending complete lost response wait state");
 check(activeTimers.size===1,"backend complete prematurely stopped response-wait elapsed time");
 check(host.querySelector("[data-energy-path-canvas]")===canvas&&mutations===0,"complete event repainted previous result");
 observer.disconnect();i18n.setLanguage("en");
 const result={...original,runId:first.request.runId};first.resolve(result);for(let n=0;state.simulationRunning&&n<100;n++)await tick();await tick();
 check(state.simulationResult===result&&state.simulationProgress.status==="succeeded"&&!button.disabled&&percent.textContent==="100%","actual response did not complete display");
 check(activeTimers.size===0&&status.dataset.simulationProgressElapsed==="","response did not clean up its elapsed timer");
 const finalProgress=state.simulationProgress,completedCanvas=host.querySelector("[data-energy-path-canvas]");emit(first.request.runId,"energy_drivers");
 check(state.simulationProgress===finalProgress&&host.querySelector("[data-energy-path-canvas]")===completedCanvas,"late event overwrote completed result state");
 const second=await start();
 const broken={...original,runId:second.request.runId};Object.defineProperty(broken,"purposeResults",{get(){throw Error("deliberate display boundary failure");}});
 second.resolve(broken);for(let n=0;state.simulationRunning&&n<100;n++)await tick();await tick();
 check(state.simulationResult===broken&&broken.status==="succeeded"&&state.simulationProgress.status==="display_failed"&&status.textContent.includes("Results received, but display failed"),"display exception misreported as EnergyPlus failure or discarded result");
 check(!button.disabled,"display failure left Run disabled");
 check(activeTimers.size===0,"display failure leaked elapsed timer");
 state.simulationResult=original;simulation.renderSimulation();
 const third=await start(),sentinel={runId:"newer-document-run",phase:"execute",percent:44,status:"running",message:"Newer run"};
 state.simulationActiveRunID=sentinel.runId;state.simulationProgress=sentinel;state.simulationRunning=true;third.reject(Error("old request failed"));await tick();await tick();
 check(state.simulationProgress===sentinel&&state.simulationRunning,"stale request failure overwrote newer active run");
 check(activeTimers.size===0,"stale request failure leaked old elapsed timer");
 check(pending.length===3&&JSON.stringify(original)===originalJSON,"extra Run request or original result mutation");
 window.setInterval=nativeSetInterval;window.clearInterval=nativeClearInterval;
 evidence.push("8 real backend phases; stable previous graph/focus including 1s elapsed tick; exactly one timer with response/error cleanup; EN/KO stage note; complete-before-response wait; result/display failure separation; stale error and late event guards; 3 explicit Run requests");
}catch(error){failures.push(error.stack||String(error));}
document.body.dataset.progressPostprocessStatus=failures.length?"failed":"passed";
document.getElementById("progress-postprocess-result").textContent=JSON.stringify({failures,evidence});
await fetch("/progress-postprocess/done",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({failures,evidence})});
</script>`
