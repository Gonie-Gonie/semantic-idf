package frontendchecks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestEnergyPathActualAppSmallPositiveRatioBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("actual Energy Path small-ratio presentation")
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
	page = strings.Replace(page, "</body>", epath142LayoutHTML+epathSmallRatioHTML+"</body>", 1)
	type browserResult struct {
		Failures []string `json:"failures"`
		Evidence []string `json:"evidence"`
	}
	done := make(chan browserResult, 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/epath-small-ratio.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	mux.HandleFunc("/epath-small-ratio/done", func(w http.ResponseWriter, r *http.Request) {
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
	profile := t.TempDir()
	command := exec.Command(chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0", "--window-size=1600,900", "--user-data-dir="+profile, server.URL+"/src/epath-small-ratio.html?manual=1")
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
		if len(result.Failures) > 0 {
			t.Fatalf("small positive ratio: %s", strings.Join(result.Failures, "\n"))
		}
	case <-time.After(60 * time.Second):
		stop()
		t.Fatal("actual small-ratio browser timed out")
	}
}

// The reported LargeOffice June pair is injected into the existing balanced
// application fixture; no real engine, file loader, or mocked renderer is used.
const epathSmallRatioHTML = `<script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
try{
 for(let attempt=0;document.body.dataset.epath142Status!=="manual"&&attempt<160;attempt++)await new Promise(resolve=>setTimeout(resolve,25));
 if(document.body.dataset.epath142Status!=="manual")throw new Error("actual application bootstrap failed");
 const [{state},simulation,view]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/views/energy-path-view.js")]);
 const original=JSON.stringify(state.simulationResult),host=document.getElementById("simulationEnergyDashboard");
 let runs=0,analyses=0;
 window.go.main.App.RunSimulation=async()=>{runs++;throw new Error("unexpected Run");};
 for(const name of["AnalyzeIDF","AnalyzeInputDiagnosticsText","AnalyzeIDFText"])window.go.main.App[name]=async()=>{analyses++;throw new Error("unexpected Analyze");};
 const tiny=.157/1501.071,tinyCOP=.002/20,smallLabel="<"+(.01).toLocaleString(undefined,{maximumFractionDigits:2});
 check(tiny>0&&tiny<.001,"actual June ratio fixture is not tiny and positive");
 for(const[locale,expected]of[["en-US","<0.01"],["de-DE","<0,01"],["ko-KR","<0.01"]]){
  check(view.energyPathRatioValueLabel(tiny,locale)===expected,"small-ratio decimal is not locale-aware: "+locale);
  check(view.energyPathRatioValueLabel(.85,locale)===(.85).toLocaleString(locale,{maximumFractionDigits:2}),"normal efficiency changed: "+locale);
  check(view.energyPathRatioValueLabel(4,locale)==="4","normal COP changed: "+locale);
 }
 check(view.energyPathRatioValueLabel(.01,"en-US")==="0.01"&&view.energyPathRatioValueLabel(.00999,"en-US")==="<0.01","display threshold is not strict");
 const unavailable=view.energyPathRatioValueLabel(null);
 for(const value of[undefined,null,"",false,0,-1,NaN,Infinity,-Infinity])check(view.energyPathRatioValueLabel(value)===unavailable,"invalid ratio gained numeric presentation: "+String(value));
 const install=(small,qualityStatus="complete")=>{
  const result=JSON.parse(original),explanation=result.purposeResults.energyExplanation,payload=explanation.periods[0];
  payload.id=payload.period="M6";
  const values={cooling:{load:small ? .002 : 80,site:20},heating:{load:small ? .157 : 85,site:small?1501.071:100}};
  for(const node of payload.nodes){
   node.period="M6";const service=node.serviceKind;
   let value=node.value;
   if(node.level==="load")value=values[service].load;
   if(node.level==="driver")value=values[service].load/payload.nodes.filter(item=>item.level==="driver"&&item.serviceKind===service).length;
   if(node.level==="end_use"&&values[node.endUse])value=values[node.endUse].site;
   if(node.level==="carrier")value=node.carrier==="natural_gas"?values.heating.site+5:values.cooling.site+55;
   Object.assign(node,{value,rawValue:value,effectiveValue:value,allocatedValue:value});
  }
  const nodes=new Map(payload.nodes.map(node=>[node.id,node]));
  for(const link of payload.links){
   link.period="M6";const from=nodes.get(link.fromId),to=nodes.get(link.toId);
   if(link.relation==="driver_to_load")link.fromValue=link.toValue=from.value;
   if(link.relation==="load_to_end_use")Object.assign(link,{fromValue:from.value,toValue:to.value,ratio:from.value/to.value,ratioKind:link.serviceKind==="cooling"?"coefficient_of_performance":"efficiency"});
   if(link.relation==="end_use_to_carrier"&&values[from.endUse])link.fromValue=link.toValue=from.value;
  }
  payload.summary.period="M6";payload.quality.ratios.status=qualityStatus;payload.summary.quality.ratios.status=qualityStatus;
  for(const[group,level]of[["drivers","driver"],["loads","load"],["endUses","end_use"],["carriers","carrier"]])payload.summary[group]=payload.nodes.filter(node=>node.level===level);
  result.runId="small-ratio-"+small+"-"+qualityStatus;
  Object.assign(state,{simulationResult:freeze(result),simulationRunning:false});
  simulation.restoreSimulationEnergyWorkspaceContext({simulationEnergyScopeKind:"building",simulationEnergyZoneName:"",simulationEnergyPeriod:"M6",simulationEnergyService:"heating",simulationEnergySelection:"",simulationEnergyDetailsOpen:false,energyDrawer:{tab:"data",stage:"",outputSource:""}});
  simulation.renderSimulation();return{result,payload,snapshot:JSON.stringify(result)};
 };
 const service=value=>{const control=host.querySelector("[data-simulation-energy-service]");check(Boolean(control),"missing actual service control");control.value=value;control.dispatchEvent(new Event("change",{bubbles:true}));};
 const runSample=small=>{
  const fixture=install(small);
  for(const kind of["heating","cooling"]){
   if(state.simulationEnergyService!==kind)service(kind);
   const graph=view.energyPathGraphForState(fixture.result.purposeResults.energyExplanation,state),link=graph.links.find(item=>item.relation==="load_to_end_use"&&item.serviceKind===kind);
   const expected=small?smallLabel:view.energyPathRatioValueLabel(kind==="heating" ? .85 : 4);
   const button=host.querySelector("[data-energy-path-bridge-ratio]"),strong=button?.querySelector("strong"),ratio=kind==="heating"?(small?tiny:.85):(small?tinyCOP:4);
   check(button?.dataset.energyPathRatioKind===(kind==="heating"?"efficiency":"coefficient_of_performance"),"typed named ratio disappeared: "+kind);
   check(strong?.textContent===expected,"bridge tiny ratio rounded to zero or normal changed: "+kind+" "+strong?.textContent);
   check(Number(button?.dataset.energyPathRatioValue)===ratio,"bridge raw numeric ratio changed: "+kind);
   check(button?.title.includes(expected)&&button?.getAttribute("aria-label").includes(expected)&&button?.querySelector("[data-energy-path-ratio-tooltip]")?.textContent.includes(expected),"tooltip/accessibility ratio differs: "+kind);
   if(small)check(strong?.innerHTML.startsWith("&lt;")&&button.querySelector("[data-energy-path-ratio-tooltip]").innerHTML.includes("&lt;"),"small-ratio text was not HTML escaped");
   const kpi=host.querySelector('[data-energy-path-kpi-ratio="'+kind+'"]');
   check(kpi?.textContent.includes(expected)&&Number(kpi?.dataset.energyPathKpiRatioValue)===ratio,"KPI ratio rounded to zero or mutated: "+kind);
   const geometry=()=>JSON.stringify([...host.querySelectorAll("[data-energy-path-graph-underlay] path,[data-energy-path-graph-underlay] rect")].map(element=>["d","x","y","width","height"].map(key=>element.getAttribute(key))));
   const canvas=host.querySelector("[data-energy-path-canvas]"),beforeGeometry=geometry();
   button.click();
   check(state.simulationEnergySelection===link.id,"ratio native click lost link identity");
   check(host.querySelector('[data-energy-path-link-value="ratio"]')?.textContent.includes(expected),"selected-link inspector ratio differs");
   check(host.querySelector("[data-energy-path-canvas]")===canvas&&geometry()===beforeGeometry,"ratio selection redrew quantitative geometry");
   const endUse=graph.nodes.find(node=>node.level==="end_use"&&node.endUse===kind);
   [...host.querySelectorAll("[data-energy-path-layout-node]")].find(node=>node.dataset.energyPathLayoutNode===endUse.id).click();
   check(state.simulationEnergySelection===endUse.id&&host.querySelector('[data-energy-path-detail-breakdown="ratioRows"] dd')?.textContent.includes(expected),"node inspector ratio differs");
   check(state.simulationEnergyPeriod==="M6"&&state.simulationEnergyScopeKind==="building"&&state.simulationEnergyService===kind&&!state.simulationRunning,"native selection/navigation changed simulation context");
   check(JSON.stringify(fixture.result)===fixture.snapshot,"presentation mutated canonical raw ratio or dual quantities");
  }
  return fixture;
 };
 const small=runSample(true);runSample(false);
 const raw=small.payload.links.find(link=>link.relation==="load_to_end_use"&&link.serviceKind==="heating");
 check(view.energyPathConversionRatio(raw)?.value===tiny,"named tiny ratio numeric validation lost precision");
 for(const update of[{ratio:0},{ratio:NaN},{ratio:Infinity},{ratioKind:""},{ratioKind:undefined},{fromValue:0},{toValue:0}])check(view.energyPathConversionRatio({...raw,...update})===null,"invalid/missing named ratio became available: "+JSON.stringify(update));
 const summaryHost=document.createElement("div");
 summaryHost.innerHTML=view.renderEnergyPathSummaryOverview({...small.payload.summary,ratios:[{label:"June efficiency",value:tiny},{label:"Normal COP",value:4}]});
 const summaryValues=[...summaryHost.querySelectorAll('[data-energy-path-summary-group="ratios"] strong')].map(node=>node.textContent);
 check(summaryValues.includes(smallLabel)&&summaryValues.includes("4"),"summary ratio formatter differs");
 const unknown=install(true,"unavailable"),bridge=host.querySelector("[data-energy-path-bridge-ratio]");
 check(bridge?.dataset.energyPathRatioKind==="unavailable"&&!bridge.hasAttribute("data-energy-path-ratio-value")&&!host.querySelector('[data-energy-path-kpi-ratio="heating"]')?.hasAttribute("data-energy-path-kpi-ratio-value"),"explicit unavailable ratio quality gained a numeric value");
 bridge.click();check(!host.querySelector('[data-energy-path-link-value="ratio"]')?.textContent.includes(smallLabel),"unavailable selected-link ratio showed a tiny value");
 check(JSON.stringify(unknown.result)===unknown.snapshot,"unavailable presentation mutated raw payload");
 check(runs===0&&analyses===0,"ratio selection/navigation invoked Run or Analyze");
 evidence.push("Actual June .157 / 1501.071 = "+tiny+" and tiny COP = "+tinyCOP+": bridge, tooltip, KPI, link/node inspector, summary escaped below-threshold labels; .85/4 unchanged; three locales, immutable values, no Run/Analyze.");
}catch(error){failures.push(String(error?.stack||error));}
await fetch("/epath-small-ratio/done",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({failures,evidence})});
</script>`
