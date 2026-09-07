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

func TestEPATH200ActualAppEnergyPathControlsBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser actual Energy Path controls acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	// Reuse the actual app shell, styles, completed-run fixture and delegated
	// handlers. The graph data is synthetic; this is not an EnergyPlus run claim.
	page := readTestFile(t, "frontend/src/index.html")
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		if !strings.Contains(page, script) {
			t.Fatalf("actual app bootstrap changed: %s", script)
		}
		page = strings.Replace(page, script, "", 1)
	}
	page = strings.Replace(page, "</body>", epath200InitialStateHTML+epath142LayoutHTML+epath200ControlsHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/epath200-controls.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	width, height := 1600, 900
	run := func() ([]byte, error) {
		return exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--force-device-scale-factor=1", fmt.Sprintf("--window-size=%d,%d", width, height), "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/src/epath200-controls.html?manual=1").CombinedOutput()
	}
	output, err := run()
	if viewport := regexp.MustCompile(`data-epath200-viewport="(\d+)x(\d+)"`).FindSubmatch(output); err == nil && len(viewport) == 3 {
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
		t.Fatalf("EPATH200 browser: %v\n%s", err, output)
	}
	diagnostic := regexp.MustCompile(`(?s)<pre id="epath200-result"[^>]*>(.*?)</pre>`).FindStringSubmatch(string(output))
	if !strings.Contains(string(output), `data-epath200-status="passed"`) {
		if len(diagnostic) == 2 {
			t.Fatalf("EPATH200 failed: %s", diagnostic[1])
		}
		t.Fatalf("EPATH200 failed:\n%s", output)
	}
	if len(diagnostic) == 2 {
		t.Log(diagnostic[1])
	}
}

// Capture the real module defaults before the shared completed-run fixture
// applies its explicit setup. Thus default assertions cannot pass solely
// because the fixture itself requested Building / Annual / All.
const epath200InitialStateHTML = `<script type="module">
globalThis.epath200InitialState=import("/src/js/state.js").then(({state})=>({
 scope:state.simulationEnergyScopeKind,period:state.simulationEnergyPeriod,service:state.simulationEnergyService
}));
</script>`

const epath200ControlsHTML = `<pre id="epath200-result" hidden>pending</pre><script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
document.body.dataset.epath200Viewport=innerWidth+"x"+innerHeight;
try{
 const defaults=await globalThis.epath200InitialState;
 check(defaults?.scope==="building"&&defaults?.period==="annual"&&defaults?.service==="all","fresh state defaults are not Building / Annual / All: "+JSON.stringify(defaults));
 for(let attempt=0;document.body.dataset.epath142Status!=="manual"&&attempt<200;attempt++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath142Status!=="manual")throw new Error("actual app bootstrap failed: "+document.getElementById("epath142-result")?.textContent);
 const {state}=await import("/src/js/state.js");
 const host=document.getElementById("simulationEnergyDashboard"),result=state.simulationResult,raw=JSON.stringify(result);
 const api=window.go.main.App,originalAPI={...api};let forbiddenCalls=0;
 for(const method of["AnalyzeInputText","AnalyzeInputDiagnosticsText","RunSimulation","RunSimulationText","RunPurposeSimulationText"]){api[method]=()=>{forbiddenCalls++;throw new Error("control interaction called backend "+method);};}
 const selectors={scope:"[data-simulation-energy-scope]",zone:"[data-simulation-energy-zone-name]",period:"[data-simulation-energy-path-period]",service:"[data-simulation-energy-service]"};
 const control=kind=>host.querySelector(selectors[kind]);
 const visible=element=>Boolean(element&&element.getClientRects().length&&getComputedStyle(element).visibility!=="hidden");
 const ids=level=>[...host.querySelectorAll('[data-energy-path-stage="'+level+'"] [data-energy-path-layout-node]')].map(node=>node.dataset.energyPathLayoutNode);
 const change=(kind,value)=>{const element=control(kind);if(!element)throw new Error("missing actual control "+kind);element.focus();element.value=value;element.dispatchEvent(new Event("change",{bubbles:true}));check(control(kind)?.value===value,"handler did not retain "+kind+"="+value);check(document.activeElement===control(kind),"rerender did not return focus to "+kind);};
 const assertControls=(zone,label)=>{
  const controls=host.querySelector(".energy-path-controls");
  check(controls?.querySelectorAll("select").length===3,label+": primary selects are not exactly Scope / Period / Service");
  check(controls?.querySelectorAll("input").length===(zone?1:0),label+": unexpected primary input count");
  check(host.querySelectorAll(selectors.zone).length===(zone?1:0),label+": Zone input presence differs from scope");
  check(host.querySelectorAll("#simulationEnergyPathZones").length===(zone?1:0),label+": Zone datalist presence differs from scope");
  for(const kind of["scope","period","service"]){check(visible(control(kind)),label+": hidden "+kind);check(control(kind)?.getAttribute("aria-label")===({scope:"Scope",period:"Period",service:"Service"})[kind],label+": inaccessible "+kind+" label");}
  if(zone){check(control("zone")?.type==="search"&&control("zone")?.list===host.querySelector("#simulationEnergyPathZones")&&visible(control("zone")),label+": Zone is not a visible search input bound to its datalist");check(control("zone")?.getAttribute("aria-label")==="Zone",label+": Zone has no distinct accessible name");}
  check(!host.querySelector(".simulation-energy-subnav,[data-simulation-energy-view],.energy-path-summary-overview,[data-energy-path-summary-group]"),label+": old Energy subview / summary controls remain");
  for(const token of["sankey-mode","sign-mode","node-limit","focus-mode","allocation-policy","period-kind","period-index","end-use-node-limit"]){check(!host.querySelector('[data-simulation-energy-'+token+']'),label+": legacy control remains: "+token);}
  check(host.querySelectorAll(".energy-path-view").length===1,label+": missing / duplicate primary view");
 };
 check(innerWidth===1600&&innerHeight===900,"actual content viewport is not1600x900");
 check(control("scope")?.value==="building"&&control("period")?.value==="annual"&&control("service")?.value==="all","rendered default selections differ from Building / Annual / All");
 assertControls(false,"default Building");
 check(ids("load").includes("load.cooling.building")&&ids("load").includes("load.heating.building"),"default All omitted a thermal service");
 const expectedPeriods=["annual",...Array.from({length:12},(_,index)=>"M"+(index+1))];
 check(JSON.stringify([...control("period").options].map(option=>option.value))===JSON.stringify(expectedPeriods),"Period does not offer Annual plus each of12 months exactly once");
 check([...control("period").options].every(option=>!option.disabled&&option.textContent.trim()),"Period contains a disabled / unnamed selection");
 check(JSON.stringify([...control("service").options].map(option=>option.value))===JSON.stringify(["all","cooling","heating"]),"Service does not offer exactly All / Cooling / Heating");
 change("scope","zone");assertControls(true,"Zone");
 check([...control("zone").list.options].map(option=>option.value).join("|")==="Office","Zone datalist does not expose the actual available Zone");
 change("zone","Office");
 check(state.simulationEnergyScopeKind==="zone"&&state.simulationEnergyZoneName==="Office"&&ids("load").includes("load.cooling.office")&&!ids("load").includes("load.cooling.building"),"Zone selection did not bind graph to Office");
 change("scope","building");assertControls(false,"returned Building");
 check(ids("load").includes("load.cooling.building")&&!ids("load").includes("load.cooling.office"),"Building return retained Zone graph");
 change("period","M1");
 check(state.simulationEnergyPeriod==="M1","January was not stored in primary state");
 const coolingValue=host.querySelector('[data-energy-path-kpi="cooling_load"]');
 check(coolingValue?.textContent.includes("20"),"January did not render its20kWh cooling load instead of annual80");
 for(const period of expectedPeriods.slice(2)){change("period",period);check(state.simulationEnergyPeriod===period,"month selection failed: "+period);check(ids("load").length===0,"absent "+period+" reused annual / January load nodes");assertControls(false,period);}
 change("period","annual");
 for(const scope of["building","zone"]){
  if(control("scope").value!==scope)change("scope",scope);
  const suffix=scope==="zone"?"office":"building";
  for(const service of["cooling","heating"]){
   change("service",service);assertControls(scope==="zone",scope+" "+service);
   const opposite=service==="cooling"?"heating":"cooling";
   check(state.simulationEnergyService===service&&ids("load").join("|")==="load."+service+"."+suffix,scope+" "+service+": load filter did not select only requested service");
   check(ids("end_use").includes("end_use."+service+"."+suffix)&&!ids("end_use").includes("end_use."+opposite+"."+suffix),scope+" "+service+": end-use filter retained opposite service");
   const carriers=ids("carrier");check(carriers.includes("carrier."+(service==="cooling"?"electricity":"natural_gas")+"."+suffix),scope+" "+service+": connected carrier disappeared");
  }
  change("service","all");check(ids("load").length===2,scope+": All did not restore both services");
 }
 change("scope","building");assertControls(false,"final Building");
 check(state.simulationResult===result&&JSON.stringify(result)===raw,"control changes mutated the completed result");
 check(forbiddenCalls===0,"controls analyzed / reran the model");
 for(const key of Object.keys(api))if(!(key in originalAPI))delete api[key];Object.assign(api,originalAPI);
 evidence.push("Actual index/CSS/delegated handlers: fresh and rendered Building/Annual/All; Zone INPUT+datalist absent→present→absent; Annual+12 months; Building/Zone Cooling/Heating/All; old controls absent; immutable result and0 Analyze/Run calls");
}catch(error){failures.push(error.stack||String(error));}
document.body.dataset.epath200Status=failures.length?"failed":"passed";
document.getElementById("epath200-result").textContent=JSON.stringify({failures,evidence});
</script>`
