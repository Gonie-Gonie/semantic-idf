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

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

func TestSimulationRunInspectHVACAndComfortBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("actual Run & Inspect purpose-result browser regression")
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
	page = strings.Replace(page, "</body>", epath142LayoutHTML+simulationPurposeResultsBrowserHTML+"</body>", 1)
	type browserResult struct {
		Failures []string `json:"failures"`
		Evidence []string `json:"evidence"`
	}
	done := make(chan browserResult, 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/purpose-results.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	mux.HandleFunc("/purpose-results/run", func(w http.ResponseWriter, r *http.Request) {
		var request simulation.SimulationRunRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.PurposeRequest == nil {
			http.Error(w, "invalid purpose request", http.StatusBadRequest)
			return
		}
		result := simulationPurposeResultsBrowserFixture(request.RunID, *request.PurposeRequest)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("/purpose-results/done", func(w http.ResponseWriter, r *http.Request) {
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
	command := exec.Command(chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0", "--window-size=1600,900", "--user-data-dir="+profile, server.URL+"/src/purpose-results.html?manual=1")
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
			t.Fatalf("Run & Inspect purpose results: %s", strings.Join(result.Failures, "\n"))
		}
	case <-ctx.Done():
		stop()
		t.Fatal("Run & Inspect purpose browser did not report within its deadline")
	}
}

// Keep the wire contract and summary construction real: only the EnergyPlus
// series are fixtures. The browser sends its native request and receives the
// production purpose builder's HVAC/Comfort bundle through JSON.
func simulationPurposeResultsBrowserFixture(runID string, request simulation.SimulationPurposeRequest) simulation.SimulationRunResult {
	series := func(key, name, unit string, first, middle, last float64) simulation.SimulationSeries {
		return simulation.SimulationSeries{
			File: "eplusout.sql", Column: key + ":" + name + " [" + unit + "]",
			KeyValue: key, Name: name, ReportingFrequency: "Hourly", RowCount: 3,
			Min: first, Max: last, Average: (first + middle + last) / 3,
			Points: []simulation.SimulationPoint{
				{X: 0, Label: "01/21 01:00:00", Value: first},
				{X: 1, Label: "01/21 02:00:00", Value: middle},
				{X: 2, Label: "01/21 03:00:00", Value: last},
			},
		}
	}
	result := simulation.SimulationRunResult{
		RunID: runID, Status: "succeeded", Filename: "purpose-results.idf",
		PurposeRunPlan: &simulation.PurposeRunPlan{Purposes: request.Purposes},
		Series: []simulation.SimulationSeries{
			series("SUPPLY OUTLET", "System Node Temperature", "C", 14, 16, 18),
			series("SUPPLY OUTLET", "System Node Mass Flow Rate", "kg/s", 0.5, 1, 1.5),
			series("SUPPLY OUTLET", "System Node Setpoint Temperature", "C", 16, 16, 16),
			series("SUPPLY FAN", "Fan Electricity Rate", "W", 300, 400, 500),
			series("OFFICE", "Zone Mean Air Temperature", "C", 22, 24, 26),
			series("OFFICE", "Zone Thermostat Heating Setpoint Temperature", "C", 20, 20, 20),
			series("OFFICE", "Zone Thermostat Cooling Setpoint Temperature", "C", 25, 25, 25),
			series("OFFICE", "Zone Air Relative Humidity", "%", 45, 50, 55),
		},
	}
	bundle := simulation.BuildPurposeResultBundle(&result, request)
	result.PurposeResults = &bundle
	return result
}

const simulationPurposeResultsBrowserHTML = `<pre id="purpose-results-evidence" hidden>pending</pre><script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const tick=()=>new Promise(resolve=>setTimeout(resolve,0));
try{
 for(let n=0;document.body.dataset.epath142Status!=="manual"&&n<200;n++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath142Status!=="manual")throw Error("actual fixture not ready: "+document.getElementById("epath142-result")?.textContent);
 const {state}=await import("/src/js/state.js"),i18n=await import("/src/js/i18n.js");
 i18n.setLanguage("en");
 const original=state.simulationResult,originalJSON=JSON.stringify(original),requests=[];
 const run=document.getElementById("simulationRunButton");
 const tab=view=>document.querySelector('[data-simulation-result-view-button="'+view+'"]');
 check(tab("hvac_loops").disabled&&tab("comfort").disabled,"previous energy-only result unexpectedly enables HVAC/Comfort");
 for(const purpose of ["hvac_loop_check","comfort_check"]){
  const input=document.querySelector('[data-simulation-purpose="'+purpose+'"]');
  check(input&&!input.checked,"purpose should start unchecked: "+purpose);
  input.click();
  check(input.checked&&state.simulationSelectedPurposes.includes(purpose),"native checkbox did not preserve selected purpose: "+purpose);
 }
 window.go.main.App.RunPurposeSimulationText=async request=>{
  requests.push(request);
  const response=await fetch("/purpose-results/run",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(request)});
  if(!response.ok)throw Error(await response.text());
  return response.json();
 };
 check(!run.disabled,"Run & Inspect is disabled after purpose selection: "+run.title);
 run.click();
 for(let n=0;(requests.length===0||state.simulationRunning)&&n<500;n++)await new Promise(resolve=>setTimeout(resolve,10));
 check(requests.length===1,"native Run must send exactly one request");
 const request=requests[0];
 for(const purpose of ["basic_energy","zone_heat_flow","hvac_loop_check","comfort_check"]){
  check(request?.purposeRequest?.purposes.includes(purpose),"run request omitted purpose: "+purpose);
 }
 check(request?.resultMode==="sql_first"&&request?.purposeRequest?.sqlMode==="sql_first","run lost SQL-first result extraction");
 check(!state.simulationRunning&&state.simulationProgress?.status==="succeeded","purpose run failed or did not finish: "+document.getElementById("simulationStatus").textContent);
 check(state.simulationResult!==original&&state.simulationResult?.runId===request?.runId,"native completion did not install new result");
 check(!tab("hvac_loops").disabled&&!tab("comfort").disabled,"returned purpose data did not enable both result tabs");
 check(state.simulationResult?.purposeResults?.hvacLoops?.[0]?.series?.length===3,"production HVAC builder did not return node series");
 check(state.simulationResult?.purposeResults?.comfort?.zones?.[0]?.metrics?.length===4,"production Comfort builder did not return zone metrics");

 tab("hvac_loops").click();await tick();
 const hvac=document.getElementById("simulationHVACLoopResults");
 check(state.simulationActiveResultView==="hvac_loops"&&!hvac.closest('[data-simulation-result-view]').hidden,"HVAC tab did not open its result section");
 const nodeRow=[...hvac.querySelectorAll("tbody tr")].find(row=>row.cells[0]?.textContent.trim()==="SUPPLY OUTLET"&&row.cells[1]?.textContent.trim()==="System Node Temperature");
 check(nodeRow&&[4,5,6,7].map(index=>parseFloat(nodeRow.cells[index].textContent)).join(",")==="14,18,16,3","HVAC node temperature min/max/average/points missing or incorrect: "+nodeRow?.textContent);
 check(hvac.textContent.includes("SUPPLY FAN")&&hvac.textContent.includes("Fan Electricity Rate"),"HVAC component operation result missing");
 check(hvac.querySelector(".simulation-hvac-node-card")?.textContent.includes("14"),"HVAC first-frame snapshot missing");
 const frame=hvac.querySelector("[data-simulation-hvac-frame]");
 check(frame?.max==="2","HVAC frame slider did not use returned series length");
 if(frame){frame.value="2";frame.dispatchEvent(new Event("input",{bubbles:true}));}
 check(state.simulationHVACFrameIndex===2&&hvac.querySelector(".simulation-hvac-node-card")?.textContent.includes("18"),"HVAC frame interaction did not render the returned last value");

 tab("comfort").click();await tick();
 const comfort=document.getElementById("simulationComfortResults");
 check(state.simulationActiveResultView==="comfort"&&!comfort.closest('[data-simulation-result-view]').hidden,"Comfort tab did not open its result section");
 const comfortRow=[...comfort.querySelectorAll("tbody tr")].find(row=>row.cells[0]?.textContent.trim()==="OFFICE"&&row.cells[1]?.textContent.trim()==="Zone Mean Air Temperature");
 check(comfortRow&&[3,4,5,8].map(index=>Number(comfortRow.cells[index].textContent)).join(",")==="22,26,24,3","Comfort temperature min/max/average/points missing or incorrect");
 check(comfort.querySelector(".comfort-temperature-line")?.getAttribute("points").trim().split(/\s+/).length===3,"Comfort temperature timeline lost returned samples");
 check(Boolean(comfort.querySelector(".comfort-setpoint-band")),"Comfort heating/cooling setpoint band missing");
 check(Boolean(comfort.querySelector(".comfort-humidity-line")),"Comfort humidity timeline missing");
 check(JSON.stringify(original)===originalJSON,"Run or purpose tabs mutated previous result");
 evidence.push("Native checkbox changes preserve all four purposes in one SQL-first Run & Inspect request; production Go bundle enables HVAC/Comfort; node and zone statistics, component results, frame slider, temperature/setpoint/humidity timelines render.");
}catch(error){failures.push(error.stack||String(error));}
document.body.dataset.purposeResultsStatus=failures.length?"failed":"passed";
document.getElementById("purpose-results-evidence").textContent=JSON.stringify({failures,evidence});
await fetch("/purpose-results/done",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({failures,evidence})});
</script>`
