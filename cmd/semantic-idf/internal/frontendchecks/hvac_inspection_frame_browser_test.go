package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestHVACInspectionSharedFrameActualAppBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("actual app HVAC frame controls")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	page := readTestFile(t, "frontend/src/index.html")
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		page = strings.Replace(page, script, "", 1)
	}
	page = strings.Replace(page, "</body>", hvacInspectionFrameHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/frame.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/src/frame.html").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-hvac-frame-status="passed"`) {
		if _, detail, ok := strings.Cut(string(output), `<pre id="hvac-frame-result" hidden="">`); ok {
			message, _, _ := strings.Cut(detail, "</pre>")
			t.Fatalf("HVAC shared frame: %v\n%s", err, message)
		}
		t.Fatalf("HVAC shared frame: %v\n%s", err, output)
	}
}

const hvacInspectionFrameHTML = `<pre id="hvac-frame-result" hidden>pending</pre>
<script>window.go={main:{App:{GetSimulationEnvironment:async()=>({installations:[],weatherFolders:[]})}}};window.runtime={EventsOn(){}};</script>
<script type="module">
const check=(value,message)=>{if(!value)throw Error(message)};
try{
 const [store,simulation,navigation]=await Promise.all([import('/src/js/state.js'),import('/src/js/views/simulation-views.js'),import('/src/js/navigation.js')]);
 const {state}=store;store.setDocumentText('Version,25.1;\nBuilding,Frame check;');simulation.initializeSimulationControls();
 await new Promise(resolve=>setTimeout(resolve,0));
 const loop=(name,hours)=>({name,loopType:'AirLoopHVAC',series:hours.length?[{name:'System Node Temperature',keyValue:name+' node',file:'eplusout.sql',column:name+' node:System Node Temperature [C](Hourly)',reportingFrequency:'Hourly',points:hours.map(x=>({x,value:20+x,label:'01-01 0'+x+':00'}))}]:[],components:[]});
 const result={runId:'frames-1',status:'succeeded',series:[],purposeResults:{hvacLoops:[loop('First',[1,2,3]),loop('Second',[1,3]),loop('Empty',[])]},purposeRunPlan:{outputObjects:[]}};
 Object.assign(state,{report:null,simulationRunning:false,simulationResult:result,simulationProgress:{status:'succeeded',percent:100,message:'Completed'},simulationActiveResultView:'hvac_loops'});
 navigation.switchResultTab('simulation',{recordHistory:false});simulation.renderSimulation();
 const host=document.getElementById('simulationHVACLoopResults'),slider=()=>host.querySelector('[data-simulation-hvac-frame]'),label=()=>host.querySelector('[data-hvac-inspect-frame-label]').textContent;
 const select=name=>{const picker=host.querySelector('[data-simulation-hvac-loop]');picker.value=[...picker.options].find(option=>option.textContent===name).value;picker.dispatchEvent(new Event('change',{bubbles:true}));};
 const frame=index=>{const control=slider();control.value=index;control.dispatchEvent(new Event('input',{bubbles:true}));control.dispatchEvent(new Event('change',{bubbles:true}));};
 check(!document.querySelector('#simulationHVACLoopStats')&&!host.closest('.simulation-section').querySelector('.profile-section-head'),'removed loop heading/count still visible');
 frame(2);check(label()==='01-01 03:00','frame slider did not change selected hour');
 select('Second');check(slider().value==='1'&&label()==='01-01 03:00','loop switch reset timestamp or used old array index');
 select('Empty');select('First');check(slider().value==='2'&&label()==='01-01 03:00','empty loop discarded shared frame');
 select('Second');frame(0);select('First');check(slider().value==='0'&&label()==='01-01 01:00','frame selected in second loop was not shared');
 frame(2);state.simulationResult={...result,runId:'frames-2'};simulation.renderSimulation();check(slider().value==='0','new simulation retained previous run timestamp');
 document.body.dataset.hvacFrameStatus='passed';document.getElementById('hvac-frame-result').textContent='passed';
}catch(error){document.body.dataset.hvacFrameStatus='failed';document.getElementById('hvac-frame-result').textContent=error.stack;}
</script>`
