package frontendchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBatchSimulationLiveProgressAndNoRetryBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("actual Tools batch progress regression")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	for _, path := range []string{"frontend/src/js/batch/batch-simulation.js", "frontend/src/js/simulation-progress.js", "frontend/src/js/tools.js", "frontend/src/js/locales/en.js", "frontend/src/js/locales/ko.js"} {
		_ = readTestFile(t, path)
	}
	page := readTestFile(t, "frontend/src/tools.html")
	marker := `<script type="module" src="./js/tools.js"></script>`
	if !strings.Contains(page, marker) {
		t.Fatal("actual Tools bootstrap changed")
	}
	page = strings.Replace(page, marker, batchSimulationProgressSetupHTML+marker+batchSimulationProgressAssertionsHTML, 1)
	type result struct {
		Failures []string       `json:"failures"`
		Evidence map[string]any `json:"evidence"`
	}
	done := make(chan result, 1)
	releaseRun := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseRun) })
	var mu sync.Mutex
	var runID string
	runCalls, alternateCalls, pollCalls, pollActive, maxPollActive := 0, 0, 0, 0, 0
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/batch-progress.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	mux.HandleFunc("/api/batch-simulation-run", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			RunID string `json:"runId"`
		}
		if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&request) != nil || request.RunID == "" {
			http.Error(w, "invalid request", 400)
			return
		}
		mu.Lock()
		runCalls++
		count := runCalls
		runID = request.RunID
		mu.Unlock()
		if count > 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"broken":`)
			return
		}
		select {
		case <-releaseRun:
			http.Error(w, "HTTP transfer failed after batch execution started", 502)
		case <-r.Context().Done():
		}
	})
	mux.HandleFunc("/api/multi-simulation-run", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		alternateCalls++
		mu.Unlock()
		http.Error(w, "alternate run must not execute", 500)
	})
	mux.HandleFunc("/api/simulation/progress", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		pollCalls++
		pollActive++
		if pollActive > maxPollActive {
			maxPollActive = pollActive
		}
		count, id := pollCalls, runID
		mu.Unlock()
		defer func() { mu.Lock(); pollActive--; mu.Unlock() }()
		if r.URL.Query().Get("runId") != id {
			http.Error(w, "wrong run", 400)
			return
		}
		if count > 1 {
			<-r.Context().Done()
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"runId": id, "sequence": 10, "phase": "item_progress", "itemRunId": "http-item", "itemPhase": "sql_heat_flow", "itemPath": "C:/fixtures/Alpha.idf", "status": "running", "completed": 0, "total": 3, "active": 1, "queued": 2, "progressKind": "work", "workCompleted": 200, "workTotal": 0, "workUnit": "rows", "percent": 89})
	})
	mux.HandleFunc("/batch-progress/status", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"runId": runID, "runs": runCalls, "alternate": alternateCalls, "polls": pollCalls, "pollActive": pollActive, "maxPollActive": maxPollActive})
	})
	mux.HandleFunc("/batch-progress/release", func(w http.ResponseWriter, _ *http.Request) {
		releaseOnce.Do(func() { close(releaseRun) })
		w.WriteHeader(204)
	})
	mux.HandleFunc("/batch-progress/done", func(w http.ResponseWriter, r *http.Request) {
		var value result
		if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		select {
		case done <- value:
		default:
		}
		w.WriteHeader(204)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	profile := t.TempDir()
	command := exec.Command(chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0", "--window-size=1600,900", "--user-data-dir="+profile, server.URL+"/src/batch-progress.html#batch-simulation")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	stop := epath161OwnedChromeCleanup(t, command, profile)
	t.Cleanup(stop)
	select {
	case value := <-done:
		stop()
		evidence, _ := json.Marshal(value.Evidence)
		t.Log(string(evidence))
		if len(value.Failures) > 0 {
			t.Fatal(strings.Join(value.Failures, "\n"))
		}
	case <-ctx.Done():
		stop()
		t.Fatal("actual batch progress browser timed out")
	}
}

const batchSimulationProgressSetupHTML = `<script>
window.__batchProgress={native:[],handlers:new Map(),errors:[]};
window.addEventListener("error",event=>window.__batchProgress.errors.push(String(event.error||event.message)));
window.addEventListener("unhandledrejection",event=>window.__batchProgress.errors.push(String(event.reason)));
window.go={main:{App:{GetSettings:async()=>({settings:{appearance:{language:"en",theme:"light"}}}),GetAppInfo:async()=>({name:"SemanticIDF",version:"progress"}),GetSimulationEnvironment:async()=>({installations:[{version:"25.1",executablePath:"C:/fixtures/EnergyPlus.exe"}],weatherFolders:[],defaultWorkerCount:2}),SelectSimulationInputFiles:async()=>({paths:["C:/fixtures/Alpha.idf","C:/fixtures/Beta.idf","C:/fixtures/Gamma.idf"]}),RunMultipleSimulations:request=>new Promise(resolve=>window.__batchProgress.native.push({request,resolve}))}}};
window.runtime={EventsOn:(name,handler)=>{window.__batchProgress.handlers.set(name,handler);return()=>{};},EventsOnMultiple:(name,handler)=>{window.__batchProgress.handlers.set(name,handler);return()=>{};}};
</script>`

const batchSimulationProgressAssertionsHTML = `<script type="module">
const failures=[],evidence={};
const check=(value,message)=>{if(!value)failures.push(message);};
const tick=()=>new Promise(resolve=>setTimeout(resolve,0));
const until=async(condition,label)=>{for(let n=0;n<1200;n++){if(await condition())return;await new Promise(resolve=>setTimeout(resolve,10));}throw Error("Timed out: "+label);};
try{
 const fixture=window.__batchProgress,api=window.go.main.App,i18n=await import("/src/js/i18n.js"),run=document.getElementById("multiSimulationRun"),status=document.getElementById("multiSimulationStatus"),percent=document.getElementById("multiSimulationPercent"),files=document.getElementById("multiSimulationFiles");
 const serverState=async()=>await(await fetch("/batch-progress/status")).json();
 await until(()=>fixture.handlers.has("idfAnalyzer:multiSimulationProgress"),"actual Tools handlers");
 document.getElementById("multiSimulationSelectFiles").click();await until(()=>!run.disabled,"selection");run.click();await until(()=>fixture.native.length===1,"explicit native run");
 const first=fixture.native[0],emit=fields=>fixture.handlers.get("idfAnalyzer:multiSimulationProgress")({runId:first.request.runId,status:"running",phase:"item_progress",completed:0,total:3,active:2,queued:1,...fields});
 emit({sequence:1,itemRunId:"alpha",itemPath:"C:/fixtures/Alpha.idf",itemPhase:"sql_series",progressKind:"work",workCompleted:10,workTotal:100,workUnit:"rows",percent:89});
 const alpha=files.querySelector('[data-simulation-file-path="C:/fixtures/Alpha.idf"]'),beta=files.querySelector('[data-simulation-file-path="C:/fixtures/Beta.idf"]');
 check(percent.textContent==="0%"&&status.textContent.includes("0 / 3 files complete")&&status.textContent.includes("2 active")&&status.textContent.includes("1 queued"),"Batch displayed child/legacy percent as whole batch progress");
 check(alpha.textContent.includes("Reading SQL time series")&&alpha.textContent.includes("10 / 100 rows")&&alpha.textContent.includes("10% of this step"),"active child row omitted stage-local actual work");
 emit({sequence:2,itemRunId:"beta",itemPath:"C:/fixtures/Beta.idf",itemPhase:"execute",message:"Warming up {2}",progressKind:"indeterminate"});
 check(alpha.textContent.includes("10 / 100 rows")&&beta.textContent.includes("Warming up {2}")&&run.disabled,"parallel child row erased another worker or enabled rerun");
 const newest=status.textContent;emit({sequence:1,itemPhase:"complete",message:"stale child",completed:3});check(status.textContent===newest,"stale batch sequence overwrote live worker phase");
 i18n.setLanguage("ko");await tick();check(alpha.textContent.includes("SQL 시계열")&&alpha.textContent.includes("현재 작업 10%")&&status.textContent.includes("2 실행 중")&&status.textContent.includes("1 대기 중"),"language switch did not translate all retained active worker rows");
 emit({sequence:3,phase:"execute",completed:1,total:3,active:1,queued:1,workUnit:"files",message:"Alpha.idf",status:"failed"});
 check(percent.textContent==="33%"&&status.classList.contains("status-loading")&&run.disabled,"one failed item stopped whole batch progress or changed measured file count");
 emit({sequence:4,phase:"complete",completed:3,total:3,status:"complete"});
 check(status.textContent.includes("결과 수신 중")&&run.disabled,"backend complete claimed installed Batch result before response");
 first.resolve({runId:first.request.runId,total:3,completed:3,succeeded:0,failed:3,results:[]});await until(()=>!run.disabled,"native typed response");i18n.setLanguage("en");await tick();
 const completedStatus=status.textContent;emit({sequence:5,message:"late progress"});check(status.textContent===completedStatus,"late batch event overwrote final typed result");
 check((await serverState()).polls===0,"native batch started unnecessary HTTP polling");
 api.RunMultipleSimulations=undefined;run.click();await until(async()=>(await serverState()).polls>=1,"HTTP-only child telemetry");
 await until(()=>status.textContent.includes("200 rows processed"),"actual unknown-total SQL work");
 check(percent.textContent==="0%"&&alpha.textContent.includes("Reading SQL heat-flow data")&&alpha.textContent.includes("200 rows processed")&&!status.textContent.includes("remaining"),"HTTP child unknown total became numeric near-finished batch ETA");
 await until(async()=>(await serverState()).polls===2,"in-flight poll to abort");await fetch("/batch-progress/release");await until(()=>!run.disabled,"HTTP transfer failure");
 check(status.textContent.includes("HTTP transfer failed after batch execution started"),"batch transfer failure claimed success or hid error");
 await until(async()=>(await serverState()).pollActive===0,"failed-run query abortion");
 run.click();await until(async()=>(await serverState()).runs===2,"explicit rerun after transfer failure");await until(()=>!run.disabled,"invalid JSON response");
 const current=await serverState();check(current.runs===2&&current.alternate===0&&fixture.native.length===1,"500/JSON failures automatically reran through native or alternate HTTP endpoint");
 check(current.polls===2&&current.maxPollActive===1,"settled batch leaked/overlapped progress polling");
 check(!status.textContent.includes("Batch simulation complete"),"invalid JSON response reported batch completion");
 check(fixture.errors.length===0,"unhandled actual Tools errors: "+fixture.errors.join("; "));
 evidence.transport=current;evidence.nativeRuns=fixture.native.length;evidence.checks="actual Tools; parallel child rows; files vs work percentages; EN/KO live change; sequence/run/response guards; HTTP-only polling and abort; 500 and malformed JSON never rerun";
}catch(error){failures.push(error.stack||String(error));}
await fetch("/batch-progress/done",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({failures,evidence})});
</script>`
