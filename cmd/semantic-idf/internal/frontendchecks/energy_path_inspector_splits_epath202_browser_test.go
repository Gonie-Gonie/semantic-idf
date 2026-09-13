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

func TestEPATH202ActualAppInspectorCarrierAndEndUseSplitsBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser actual Energy Path inspector split acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	// Existing actual-index fixtures supply a complete synthetic reported graph,
	// its period/Zone variants, residual and independent supply context. No model
	// formatter or split helper is used to calculate this test's expected rows.
	page := readTestFile(t, "frontend/src/index.html")
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		if !strings.Contains(page, script) {
			t.Fatalf("actual app bootstrap changed: %s", script)
		}
		page = strings.Replace(page, script, "", 1)
	}
	// The older provenance fixture's generic "storage" label does not establish
	// discharge. This test needs an explicitly typed supply to prove exclusion
	// from consumption; do not teach production to guess an unknown direction.
	const storageFixture = `["storage",5,3]`
	if strings.Count(epath150InspectorHTML, storageFixture) != 1 {
		t.Fatal("expected one existing storage fixture tuple")
	}
	inspectorFixture := strings.Replace(epath150InspectorHTML, storageFixture, `["storage_discharge",5,3]`, 1)
	page = strings.Replace(page, "</body>", epath142LayoutHTML+epath143RibbonsHTML+epath144ColorsHTML+inspectorFixture+epath202InspectorSplitsHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/epath202-inspector-splits.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	width, height := 1600, 900
	run := func() ([]byte, error) {
		return exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--force-device-scale-factor=1", fmt.Sprintf("--window-size=%d,%d", width, height), "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/src/epath202-inspector-splits.html?manual=1").CombinedOutput()
	}
	output, err := run()
	if viewport := regexp.MustCompile(`data-epath202-viewport="(\d+)x(\d+)"`).FindSubmatch(output); err == nil && len(viewport) == 3 {
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
		t.Fatalf("EPATH202 browser: %v\n%s", err, output)
	}
	diagnostic := regexp.MustCompile(`(?s)<pre id="epath202-result"[^>]*>(.*?)</pre>`).FindStringSubmatch(string(output))
	if !strings.Contains(string(output), `data-epath202-status="passed"`) {
		if len(diagnostic) == 2 {
			t.Fatalf("EPATH202 failed: %s", diagnostic[1])
		}
		t.Fatalf("EPATH202 failed:\n%s", output)
	}
	if len(diagnostic) == 2 {
		t.Log(diagnostic[1])
	}
}

const epath202InspectorSplitsHTML = `<pre id="epath202-result" hidden>pending</pre><script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
document.body.dataset.epath202Viewport=innerWidth+"x"+innerHeight;
try{
 for(let attempt=0;document.body.dataset.epath150Status!=="manual"&&attempt<250;attempt++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath150Status!=="manual")throw new Error("actual common inspector bootstrap failed: "+document.getElementById("epath150-result")?.textContent);
 const {state}=await import("/src/js/state.js");
 const host=document.getElementById("simulationEnergyDashboard"),result=state.simulationResult,rawJSON=JSON.stringify(result),reportJSON=JSON.stringify(state.report);
 const api=window.go.main.App,originalAPI={...api};let forbiddenCalls=0;
 for(const method of["AnalyzeInputText","AnalyzeInputDiagnosticsText","RunSimulation","RunSimulationText","RunPurposeSimulationText"]){api[method]=()=>{forbiddenCalls++;throw new Error("inspector invoked backend "+method);};}
 const sourceIDs=result.purposeResults.energyExplanation.sources.map(source=>source.id);
 const inspector=()=>host.querySelector("[data-energy-path-inspector]");
 const section=key=>inspector()?.querySelector('[data-energy-path-detail-section="'+key+'"]');
 const button=id=>[...host.querySelectorAll("[data-energy-path-layout-node]")].find(node=>node.dataset.energyPathLayoutNode===id);
 const change=(attribute,value)=>{const target=host.querySelector('['+attribute+']');if(!target)throw new Error("missing actual control "+attribute);target.value=value;target.dispatchEvent(new Event("change",{bubbles:true}));check(host.querySelector('['+attribute+']')?.value===value,"context control failed "+attribute+"="+value);};
 const select=id=>{const target=button(id);if(!target)throw new Error("missing actual native node "+id);target.focus();target.click();check(state.simulationEnergySelection===id&&inspector()?.dataset.energyPathInspector===id,"selection opened wrong inspector: "+id);check(document.activeElement===target,"native node focus lost: "+id);};
 const formatted=value=>value.toLocaleString(undefined,{maximumFractionDigits:2})+" kWh site";
 const labels={electricity:"Electricity",natural_gas:"Natural gas",cooling:"Cooling equipment",heating:"Heating equipment",fans_pumps:"Fans & pumps",lighting:"Lighting",equipment:"Equipment",water_systems:"Water systems",refrigeration:"Refrigeration",other:"Other"};
 const assertInspectorSections=label=>{
  const text=inspector()?.innerText||"";for(const id of[...sourceIDs,"rule.private.inspector150"])check(!text.includes(id),label+": inspector leaks technical source/rule ID: "+id);
  check(!text.includes("987654"),label+": run-level source sentinel leaked into selected-period values");
  check([...inspector().querySelectorAll("[data-energy-path-detail-section]")].map(element=>element.dataset.energyPathDetailSection).join("|")==="represents|value|breakdown|basis|actions",label+": five common sections changed");
 };
 const assertRows=(groupKey,expected,label)=>{
  const groups=section("breakdown")?.querySelectorAll('[data-energy-path-detail-breakdown="'+groupKey+'"]')||[];
  check(groups.length===1,label+": expected exactly one "+groupKey+" group, found "+groups.length);
  const group=groups[0],rows=[...(group?.querySelectorAll("[data-energy-path-detail-row]")||[])],keys=rows.map(row=>row.dataset.energyPathDetailRow);
  check(JSON.stringify([...keys].sort())===JSON.stringify(Object.keys(expected).sort()),label+": wrong / duplicate / missing split keys "+JSON.stringify(keys));
  for(const[key,value]of Object.entries(expected)){
   const matches=rows.filter(row=>row.dataset.energyPathDetailRow===key),row=matches[0];
   check(matches.length===1,label+": split row is not unique: "+key);
   check(row?.querySelector("dt")?.textContent.trim()===labels[key],label+": split label is not friendly typed taxonomy for "+key+": "+row?.querySelector("dt")?.textContent);
   check(row?.querySelector("dd")?.textContent.trim()===formatted(value),label+": wrong exact value/unit for "+key+": "+row?.querySelector("dd")?.textContent+" expected "+formatted(value));
   check(row?.getClientRects().length&&getComputedStyle(row).visibility!=="hidden",label+": split row is hidden: "+key);
  }
  check(group?.querySelector("h6")?.textContent.trim()===(groupKey==="carrierRows"?"Energy-source split":"End-use breakdown"),label+": split heading misidentifies the accounting direction");
  assertInspectorSections(label);
 };
 check(innerWidth===1600&&innerHeight===900,"actual content viewport is not1600x900");
 const cases=[{scope:"building",period:"annual",factor:1},{scope:"building",period:"M1",factor:.25},{scope:"zone",period:"annual",factor:.5},{scope:"zone",period:"M1",factor:.125}];
 for(const fixture of cases){
  change("data-simulation-energy-scope",fixture.scope);if(fixture.scope==="zone")change("data-simulation-energy-zone-name","Office");change("data-simulation-energy-path-period",fixture.period);change("data-simulation-energy-service","all");
  const suffix=fixture.scope==="zone"?"office":"building",factor=fixture.factor,label=fixture.scope+"/"+fixture.period;
  check(state.simulationEnergyScopeKind===fixture.scope&&state.simulationEnergyPeriod===fixture.period&&state.simulationEnergyService==="all",label+": selected context mismatch");
  const canvas=host.querySelector("[data-energy-path-canvas]"),paths=JSON.stringify([...host.querySelectorAll("[data-energy-path-ribbon]")].map(path=>[path.dataset.energyPathRibbon,path.getAttribute("d")]));
  select("end_use.water_systems."+suffix);
  assertRows("carrierRows",{electricity:5*factor,natural_gas:5*factor},label+" water systems");
  check(!section("breakdown")?.querySelector('[data-energy-path-detail-breakdown="endUseRows"]'),label+": end-use inspector incorrectly contains reverse end-use breakdown");
  check(inspector()?.querySelector('[data-energy-path-inspector-value="total"] dd')?.textContent.trim()===formatted(10*factor),label+": water total is not the reported two-carrier subtotal");
  select("carrier.electricity."+suffix);
  assertRows("endUseRows",{cooling:10*factor,fans_pumps:10*factor,lighting:10*factor,equipment:10*factor,water_systems:5*factor,refrigeration:10*factor,other:10*factor},label+" electricity");
  check(!section("breakdown")?.querySelector('[data-energy-path-detail-breakdown="carrierRows"]'),label+": carrier inspector incorrectly contains reverse carrier split");
  if(fixture.scope==="building"){
   const annual=fixture.period==="annual";
   for(const[kind,value]of Object.entries(annual?{purchased:7,produced:6,storage:5,storage_charge:4}:{purchased:1,produced:2,storage:3,storage_charge:4})){
    const supply=section("breakdown")?.querySelector('[data-energy-path-supply-kind="'+kind+'"]');
    check(supply?.dataset.energyPathSupplyValue===String(value),label+": separate supply context lost "+kind+"="+value);
    check(!supply?.closest('[data-energy-path-detail-breakdown="endUseRows"]'),label+": supply incorrectly became consumption branch "+kind);
   }
   if(annual){
    for(const[key,value]of[["expected",68],["explained",65],["residual",3]]){
     const term=section("breakdown")?.querySelector('[data-energy-path-carrier-reconciliation-term="'+key+'"]');
     check(term?.querySelector("dd")?.textContent.trim()===formatted(value),label+": separate carrier reconciliation "+key+" lost exact value: "+term?.textContent);
     check(!term?.closest('[data-energy-path-detail-breakdown="endUseRows"]'),label+": reconciliation term became end-use consumption: "+key);
    }
   }
  }
  select("carrier.natural_gas."+suffix);
  assertRows("endUseRows",{heating:100*factor,water_systems:5*factor},label+" natural gas");
  check(host.querySelector("[data-energy-path-canvas]")===canvas&&JSON.stringify([...host.querySelectorAll("[data-energy-path-ribbon]")].map(path=>[path.dataset.energyPathRibbon,path.getAttribute("d")]))===paths,label+": selecting breakdowns replaced / changed quantitative graph");
  evidence.push(label+": water source split "+(5*factor)+"/"+(5*factor)+", electricity7 end uses total "+(65*factor)+", gas2 end uses total "+(105*factor));
 }
 check(state.simulationResult===result&&JSON.stringify(result)===rawJSON&&JSON.stringify(state.report)===reportJSON,"inspector changed original result / report metadata");
 check(forbiddenCalls===0,"inspector split selection analyzed or reran the model");
 for(const key of Object.keys(api))if(!(key in originalAPI))delete api[key];Object.assign(api,originalAPI);
 evidence.push("Exact visible typed row sets/friendly labels/site units; residual and supply remain separate; graph DOM retained; immutable inputs and0 Analyze/Run calls");
}catch(error){failures.push(error.stack||String(error));}
document.body.dataset.epath202Status=failures.length?"failed":"passed";
document.getElementById("epath202-result").textContent=JSON.stringify({failures,evidence});
</script>`
