package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEPATH150ActualAppComponentChartsBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser actual Energy Path component chart acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	page := readTestFile(t, "frontend/src/index.html")
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		if !strings.Contains(page, script) {
			t.Fatalf("actual app bootstrap changed: %s", script)
		}
		page = strings.Replace(page, script, "", 1)
	}
	page = strings.Replace(page, "</body>", epath142LayoutHTML+epath143RibbonsHTML+epath144ColorsHTML+epath150InspectorHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/epath150-inspector.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	windowWidth, windowHeight := 1600, 900
	runBrowser := func(action, query string) ([]byte, error) {
		return exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--force-device-scale-factor=1", fmt.Sprintf("--window-size=%d,%d", windowWidth, windowHeight), "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), action, server.URL+"/src/epath150-inspector.html?manual=1"+query).CombinedOutput()
	}
	output, err := runBrowser("--dump-dom", "&run150=1")
	if viewport := regexp.MustCompile(`data-epath150-viewport="(\d+)x(\d+)"`).FindSubmatch(output); err == nil && len(viewport) == 3 {
		actualWidth, _ := strconv.Atoi(string(viewport[1]))
		actualHeight, _ := strconv.Atoi(string(viewport[2]))
		if actualWidth != 1600 || actualHeight != 900 {
			if actualWidth < 1200 || actualWidth > 1800 || actualHeight < 600 || actualHeight > 1000 {
				t.Fatalf("unexpected headless viewport: %dx%d", actualWidth, actualHeight)
			}
			windowWidth += 1600 - actualWidth
			windowHeight += 900 - actualHeight
			output, err = runBrowser("--dump-dom", "&run150=1")
		}
	}
	if err != nil {
		t.Fatalf("EPATH150 browser failed: %v\n%s", err, output)
	}
	for _, mode := range []string{"driver", "link"} {
		if screenshot := os.Getenv("EPATH150_SCREENSHOT_" + strings.ToUpper(mode)); screenshot != "" {
			// Screenshot capture applies its own viewport after page layout; DOM
			// acceptance above independently calibrates the real app content size.
			windowWidth, windowHeight = 1600, 900
			if shot, shotErr := runBrowser("--screenshot="+screenshot, "&inspector="+mode); shotErr != nil {
				t.Fatalf("EPATH150 %s screenshot failed: %v\n%s", mode, shotErr, shot)
			}
		}
	}
	document := string(output)
	diagnostic := regexp.MustCompile(`(?s)<pre id="epath150-result"[^>]*>(.*?)</pre>`).FindStringSubmatch(document)
	if !strings.Contains(document, `data-epath150-status="passed"`) {
		if len(diagnostic) == 2 {
			t.Fatalf("EPATH150 failed: %s", diagnostic[1])
		}
		t.Fatalf("EPATH150 failed:\n%s", output)
	}
	if len(diagnostic) == 2 {
		t.Log(diagnostic[1])
	}
}

const epath150InspectorHTML = `<pre id="epath150-result" hidden>pending</pre><script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
document.body.dataset.epath150Viewport=innerWidth+"x"+innerHeight;
try{
 for(let attempt=0;document.body.dataset.epath144Status!=="manual"&&attempt<200;attempt++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath144Status!=="manual")throw new Error("actual app fixture failed: "+document.getElementById("epath144-result")?.textContent);
 const [{state},simulation,view]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/views/energy-path-view.js")]);
 const result=state.simulationResult,rawJSON=JSON.stringify(result),reportJSON=JSON.stringify(state.report);
 Object.assign(state,{simulationEnergyScopeKind:"building",simulationEnergyZoneName:"",simulationEnergyPeriod:"annual",simulationEnergyService:"heating",simulationEnergySelection:"",simulationEnergyDetailsOpen:false,simulationEnergyChartFrequency:"monthly"});simulation.renderSimulation();
 const host=document.getElementById("simulationEnergyDashboard"),pane=document.querySelector("#simulationPane > .simulation-pane");
 const graph=()=>view.energyPathGraphForState(result.purposeResults.energyExplanation,state);
 const button=id=>[...host.querySelectorAll("[data-energy-path-layout-node]")].find(element=>element.dataset.energyPathLayoutNode===id);
 const edge=id=>[...host.querySelectorAll("[data-energy-explanation-edge]")].find(element=>element.dataset.energyExplanationEdge===id&&element.tabIndex>=0);
 const inspector=()=>host.querySelector("[data-energy-path-inspector],[data-energy-path-link-inspector]");
 const select=id=>{const control=button(id)||edge(id);if(!control)throw new Error("missing selected graph item "+id);control.dispatchEvent(new MouseEvent("click",{bubbles:true,cancelable:true}));check(state.simulationEnergySelection===id,"actual selection handler failed "+id);return inspector();};
 const change=(selector,value)=>{const control=host.querySelector(selector);if(!control)throw new Error("missing real control "+selector);control.value=value;control.dispatchEvent(new Event("change",{bubbles:true}));};
 const checkChart=label=>{
  const panel=inspector();check(Boolean(panel),label+" chart missing");if(!panel)return;
  const frequency=panel.querySelector("[data-energy-path-chart-frequency]");
  check(frequency?.tagName==="SELECT"&&[...frequency.options].map(option=>option.value).join(",")==="monthly,hourly",label+" must offer Monthly and Hourly");
  check(!panel.querySelector("[data-energy-path-detail-section],[data-energy-path-inspector-value],[data-energy-path-link-value],[data-energy-path-series-id],[data-energy-path-driver-destination],[data-energy-path-service-destination]"),label+" retains removed component information/actions");
  check(!/What this represents|Calculation \/ allocation basis|Related model entities|Source data/.test(panel.innerText),label+" retains old detail copy");
  check(pane.scrollWidth<=pane.clientWidth+1,label+" chart causes horizontal overflow");
 };
 const nodeIDs=graph().nodes.filter(item=>button(item.id)).map(item=>item.id);
 if(new URLSearchParams(location.search).get("run150")!=="1"){
  const mode=new URLSearchParams(location.search).get("inspector"),id=mode==="link"?graph().links.find(link=>link.relation==="load_to_end_use"&&edge(link.id))?.id:nodeIDs[0];
  select(id);inspector()?.scrollIntoView({block:"start"});document.body.dataset.epath150Status="manual";
 }else{
  check(innerWidth===1600&&innerHeight===900,"actual viewport is not1600x900");
  check(state.simulationEnergyService==="all"&&!host.querySelector("[data-simulation-energy-service],.energy-path-kpis"),"removed controls or hidden persisted service filter remain");
  const canvas=host.querySelector("[data-energy-path-canvas]"),paths=JSON.stringify([...host.querySelectorAll("[data-energy-path-ribbon]")].map(path=>path.getAttribute("d")));
  for(const id of nodeIDs){select(id);checkChart("node "+id);}
  for(const relation of["driver_to_load","load_to_end_use","end_use_to_carrier","residual"]){const item=graph().links.find(link=>link.relation===relation&&edge(link.id));if(item){select(item.id);checkChart("link "+relation);}}
  select(nodeIDs[0]);check(inspector()?.querySelector("[data-energy-path-chart-frequency]")?.value==="monthly","component chart did not default Monthly");
  change("[data-energy-path-chart-frequency]","hourly");checkChart("hourly");check(state.simulationEnergyChartFrequency==="hourly"&&inspector()?.querySelector("[data-energy-path-chart-frequency]")?.value==="hourly","native Hourly selector did not update chart state");
  check(inspector()?.querySelector("[data-energy-path-chart-empty]")&&!inspector()?.querySelector("svg"),"Monthly-only fixture fabricated Hourly energy");
  check(document.activeElement===inspector()?.querySelector("[data-energy-path-chart-frequency]"),"frequency change lost keyboard focus");
  select(nodeIDs[1]);check(inspector()?.querySelector("[data-energy-path-chart-frequency]")?.value==="hourly","component selection lost Hourly preference");
  const snapshot=simulation.captureSimulationEnergyWorkspaceContext();check(snapshot.energyChartFrequency==="hourly","workspace capture lost frequency");
  change("[data-energy-path-chart-frequency]","monthly");simulation.restoreSimulationEnergyWorkspaceContext(snapshot);simulation.renderSimulationEnergyDashboard(result);check(inspector()?.querySelector("[data-energy-path-chart-frequency]")?.value==="hourly","restoring workspace lost frequency");
  change("[data-energy-path-chart-frequency]","monthly");
  const restoredCanvas=host.querySelector("[data-energy-path-canvas]");
  change("[data-energy-path-chart-frequency]","hourly");change("[data-energy-path-chart-frequency]","monthly");check(host.querySelector("[data-energy-path-canvas]")===restoredCanvas,"frequency change rebuilt Energy flow graph");
  check(JSON.stringify([...host.querySelectorAll("[data-energy-path-ribbon]")].map(path=>path.getAttribute("d")))===paths,"chart selection changed quantitative flow paths");
  for(const scope of["building","zone"]){change("[data-simulation-energy-scope]",scope);if(scope==="zone")change("[data-simulation-energy-zone-name]","Office");change("[data-simulation-energy-path-period]","M1");const id=graph().nodes.find(item=>button(item.id)).id;select(id);checkChart(scope+" January");}
  check(JSON.stringify(result)===rawJSON&&JSON.stringify(state.report)===reportJSON,"component chart mutated result/model data");
  evidence.push("Every visible node/link uses Monthly/Hourly graph; old details absent; native frequency control and workspace restore; graph retained; scoped immutable results.");
  if(failures.length)throw new Error(failures.join("\n"));document.body.dataset.epath150Status="passed";
 }
 document.getElementById("epath150-result").textContent=evidence.join("\n");
}catch(error){document.body.dataset.epath150Status="failed";document.getElementById("epath150-result").textContent=(error.stack||String(error))+"\n"+failures.join("\n");}
</script>`
