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

func TestSimulationEnergyAreaDisplayBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("actual Energy area-unit browser regression")
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
	// Keep the shared layout fixture and canonical kWh data intact. Different
	// executed-model areas expose accidental use of the building denominator
	// for zone results, while annual/monthly values share one scope area.
	fixture := epath142LayoutHTML
	for _, replacement := range [][2]string{
		{`buildingScope={kind:"building",aggregationBasis:"model_total",floorAreaM2:1}`, `buildingScope={kind:"building",aggregationBasis:"model_total",floorAreaM2:100}`},
		{`zoneScope={kind:"zone",zoneName:"Office",aggregationBasis:"model_total",floorAreaM2:1}`, `zoneScope={kind:"zone",zoneName:"Office",aggregationBasis:"model_total",floorAreaM2:25}`},
	} {
		if strings.Count(fixture, replacement[0]) != 1 {
			t.Fatalf("scope fixture changed: %s", replacement[0])
		}
		fixture = strings.Replace(fixture, replacement[0], replacement[1], 1)
	}
	page = strings.Replace(page, "</body>", fixture+energyPathAreaDisplayBrowserHTML+"</body>", 1)
	type browserResult struct {
		Failures []string `json:"failures"`
		Evidence []string `json:"evidence"`
	}
	done := make(chan browserResult, 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/energy-area-display.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	mux.HandleFunc("/energy-area-display/done", func(w http.ResponseWriter, r *http.Request) {
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
	command := exec.Command(chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0", "--window-size=1600,900", "--user-data-dir="+profile, server.URL+"/src/energy-area-display.html?manual=1")
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
			t.Fatalf("Energy area display: %s", strings.Join(result.Failures, "\n"))
		}
	case <-ctx.Done():
		stop()
		t.Fatal("Energy area browser did not report within its deadline")
	}
}

const energyPathAreaDisplayBrowserHTML = `<pre id="energy-area-evidence" hidden>pending</pre><script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
try{
 for(let n=0;document.body.dataset.epath142Status!=="manual"&&n<200;n++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath142Status!=="manual")throw Error("actual Energy fixture not ready: "+document.getElementById("epath142-result")?.textContent);
 const [{state},simulation,display,view,i18n]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/energy-path-display.js"),import("/src/js/views/energy-path-view.js"),import("/src/js/i18n.js")]);
 i18n.setLanguage("en");
 const {energyPathDisplayContext:context,formatEnergyPathDisplayValue:format,energyPathDisplayUnit:unit}=display;
 const original=state.simulationResult,originalJSON=JSON.stringify(original),explanation=original.purposeResults.energyExplanation;
 const host=document.getElementById("simulationEnergyDashboard");
 const change=(selector,value)=>{const control=host.querySelector(selector);if(!control)throw Error("missing native Energy control "+selector);control.value=value;control.dispatchEvent(new Event("change",{bubbles:true}));};
 const node=id=>host.querySelector('[data-energy-path-layout-node="'+id+'"]');
 const kpi=id=>host.querySelector('[data-energy-path-kpi="'+id+'"] strong')?.textContent.trim();
 const number=id=>node(id)?.querySelector("strong")?.textContent.trim();
 const row=(kind,key)=>host.querySelector('[data-energy-path-'+kind+'-value="'+key+'"] dd')?.textContent.trim();
 const energy=(text,value,label)=>check(text?.startsWith(value+" kWh/m²"),label+": "+text+"; expected "+value+" kWh/m²");
 const scope=()=>context(explanation,state);
 simulation.renderSimulationEnergyDashboard(original);

 check(scope().perArea===true&&scope().areaM2===100,"building area must come from executed result scope");
 for(const [source,value] of [["J",3600000],["kJ",3600],["MJ",3.6],["GJ",.0036],["Wh",1000],["kWh",1],["MWh",.001]]){
  check(format(value,source,{perArea:true,areaM2:25})==="0.04 kWh/m²","source energy unit was not converted before area division: "+source);
 }
 check(format(0,"kWh",scope())==="0.00 kWh/m²"&&format(10,"kWh",scope())==="0.10 kWh/m²","energy formatting did not keep exactly two decimal places");
 check(format(-.01,"kWh",scope())==="0.00 kWh/m²","rounded energy has a misleading negative zero");
 check(unit("kWh thermal",scope())==="kWh/m² thermal"&&unit("GJ site",scope())==="kWh/m² site","thermal/site unit suffix lost");
 for(const [value,source,expected] of [[4,"COP","4 COP"],[37.5,"%","37.5 %"],[12,"m3","12 m3"]]){
  check(format(value,source,scope())===expected&&unit(source,scope())===source,"nonenergy value was divided by area: "+source);
 }
 for(const area of [undefined,null,0,-1,Infinity,"25"]){
  const invalid=context({scope:{kind:"building",floorAreaM2:area}},{});
  check(invalid.areaM2===null&&format(25,"kWh",invalid).startsWith("—"),"missing/invalid area fabricated a numeric intensity: "+area);
 }
 check(context(explanation,{simulationEnergyScopeKind:"zone",simulationEnergyZoneName:"OFFICE"}).areaM2===25,"zone lookup lost case-insensitive scope identity");
 check(context(explanation,{simulationEnergyScopeKind:"zone",simulationEnergyZoneName:"Missing"}).areaM2===null,"unknown zone reused building area");

 energy(kpi("total_site_energy"),"1.30","building annual site KPI");
 energy(kpi("cooling_load"),"0.80","building annual cooling KPI");
 energy(kpi("heating_load"),"0.40","building annual heating KPI");
 check(number("load.cooling.building")==="0.80","building chart and KPI use different area/precision: "+number("load.cooling.building"));
 energy(node("load.cooling.building")?.title.split(": ").at(-1),"0.80","chart accessible energy value");
 check([...host.querySelectorAll("[data-energy-path-stage] > header")].every(header=>header.textContent.includes("kWh/m²")),"column headers did not display area units");
 check(host.querySelector("[data-energy-path-legend]")?.textContent.includes("kWh/m²"),"graph legend still displays total-energy units");

 change("[data-simulation-energy-service]","cooling");
 const canvas=host.querySelector("[data-energy-path-canvas]"),scene=host.querySelector("[data-energy-path-scene]");
 const cooling=node("load.cooling.building"),coolingKPI=host.querySelector('[data-energy-path-kpi="cooling_load"]');
 const bridge=[...host.querySelectorAll("[data-energy-path-bridge-ratio]")].find(item=>item.dataset.energyPathRatioValue==="4");
 check(Boolean(bridge)&&host.querySelector('[data-energy-path-kpi-ratio="cooling"]')?.dataset.energyPathKpiRatioValue==="4","normalization changed the paired80/20 conversion ratio");
 cooling.click();
 check(state.simulationEnergySelection==="load.cooling.building","native chart selection failed");
 energy(row("inspector","total"),"0.80","selection-updated node inspector total");
 energy(row("inspector","raw"),"0.80","selection-updated raw observation display");
 check(host.querySelector("[data-energy-path-canvas]")===canvas&&host.querySelector("[data-energy-path-scene]")===scene&&host.querySelector('[data-energy-path-kpi="cooling_load"]')===coolingKPI,"node selection rebuilt the cached graph/KPI scene");
 if(bridge){
  bridge.click();
  energy(row("link","from"),"0.80","selection-updated link thermal value");
  energy(row("link","to"),"0.20","selection-updated link site value");
  check(row("link","ratio")?.includes("4")&&!row("link","ratio")?.includes("/m²"),"link ratio was normalized as energy");
  check(bridge.title.includes("0.80 kWh/m²")&&bridge.title.includes("0.20 kWh/m²"),"conversion tooltip retained raw kWh values");
  check(host.querySelector("[data-energy-path-canvas]")===canvas&&host.querySelector("[data-energy-path-scene]")===scene,"link selection rebuilt cached graph scene");
 }
 change("[data-simulation-energy-service]","all");
 change("[data-simulation-energy-path-period]","M1");
 energy(kpi("total_site_energy"),"0.33","building monthly site rounding");
 energy(kpi("cooling_load"),"0.20","building monthly cooling");
 check(number("load.cooling.building")==="0.20"&&scope().areaM2===100,"monthly selection changed building area or retained annual chart values");

 change("[data-simulation-energy-scope]","zone");
 change("[data-simulation-energy-zone-name]","Office");
 change("[data-simulation-energy-path-period]","annual");
 check(scope().areaM2===25,"zone view reused building area");
 energy(kpi("total_site_energy"),"2.60","zone annual site KPI");
 energy(kpi("cooling_load"),"1.60","zone annual cooling KPI");
 check(number("load.cooling.office")==="1.60","zone annual chart did not use zone area: "+number("load.cooling.office"));
 node("load.cooling.office").click();
 energy(row("inspector","total"),"1.60","zone annual selection inspector");
 change("[data-simulation-energy-path-period]","M1");
 energy(kpi("total_site_energy"),"0.65","zone monthly site KPI");
 energy(kpi("cooling_load"),"0.40","zone monthly cooling KPI");
 check(number("load.cooling.office")==="0.40","zone monthly chart value inconsistent with KPI");
 node("load.cooling.office").click();
 energy(row("inspector","total"),"0.40","zone monthly selection inspector");

 // Old captures have no denominator. A live report must never supply an
 // unrelated area, nor may a zone silently borrow the building's area.
 const missingArea=JSON.parse(originalJSON);
 missingArea.runId="energy-area-missing-zone";
 delete missingArea.purposeResults.energyExplanation.zoneResults[0].scope.floorAreaM2;
 state.simulationResult=freeze(missingArea);
 state.report={geometry:{floorArea:999,floorAreaM2:999,zones:[{name:"Office",floorArea:999,floorAreaM2:999}]}};
 simulation.renderSimulationEnergyDashboard(state.simulationResult);
 check(kpi("cooling_load")?.startsWith("—")&&number("load.cooling.office")==="—","legacy zone without area invented kWh/m² from building or current report");
 node("load.cooling.office").click();
 check(row("inspector","total")?.startsWith("—"),"selection restored numeric energy despite missing zone area");
 check(JSON.stringify(original)===originalJSON,"area formatting or selection mutated raw simulation energy values");
 check(view.energyPathGraphForState(explanation,{simulationEnergyScopeKind:"building",simulationEnergyPeriod:"annual",simulationEnergyService:"all"}).nodes.find(item=>item.id==="load.cooling.building")?.value===80,"display normalization leaked into graph computation");
 evidence.push("Executed building100m² / zone25m² areas normalize annual and monthly KPI, chart, tooltip and node/link inspector values with two decimals. Native selections retain graph DOM and ratio4; raw results stay unchanged. Missing zone area stays unavailable, independent of live model area.");
 evidence.push("J/kJ/MJ/GJ/Wh/kWh/MWh convert to kWh before area division; thermal/site suffixes and nonenergy COP/%/m3 values retain their meaning.");
}catch(error){failures.push(error.stack||String(error));}
document.body.dataset.energyAreaStatus=failures.length?"failed":"passed";
document.getElementById("energy-area-evidence").textContent=JSON.stringify({failures,evidence});
await fetch("/energy-area-display/done",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({failures,evidence})});
</script>`
