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

func TestSimulationViewsBrowserModuleLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser Simulation module harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/simulation-module", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, simulationViewsModuleHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--no-sandbox",
		"--no-first-run",
		"--no-default-browser-check",
		"--virtual-time-budget=10000",
		"--user-data-dir="+t.TempDir(),
		"--dump-dom",
		server.URL+"/simulation-module",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("Simulation module browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("Simulation module browser harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-simulation-module-status="passed"`) {
		t.Fatalf("Simulation module did not load as a native ES module:\n%s", document)
	}
}

const simulationViewsModuleHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Simulation module harness</title></head>
<body data-simulation-module-status="pending">
<pre id="result">pending</pre>
<script type="module">
try {
  const module = await import("/src/js/views/simulation-views.js");
  if (typeof module.renderSimulation !== "function" || typeof module.initializeSimulationControls !== "function") {
    throw new Error("Simulation exports are unavailable");
  }
  const {openSimulationResultParts,simulationResultStore}=await import('/src/js/simulation-result-parts.js');
  const check=(condition,message)=>{if(!condition)throw Error(message);};
  const observed=(id,name)=>({file:'eplusout.sql',column:name,sourceId:id,reportingFrequency:'Hourly',points:null});
  const root={schema:'semantic-idf.simulation-transfer/v1',result:{runId:'comparison-resources',series:[observed('weather','Environment:Site Outdoor Air Drybulb Temperature [C]'),observed('hvac','NODE:System Node Temperature [C]')],purposeResults:{
    comfort:{zones:[{zoneName:'Office',metrics:[{name:'Zone Mean Air Temperature',points:null}]}]},
    hvacLoops:[{name:'Loop',series:[observed('hvac','NODE:System Node Temperature [C]')]}],
    zoneHeatFlow:{zones:[{name:'Office',values:null}]},
    energyExplanation:{sources:[{id:'energy-source',hourlyEnergy:{basis:'reported_source',unit:'kWh',labels:['01-01 01:00','01-01 02:00'],values:null}}]}
  }},timelines:[{x:[10,40],labels:['01-01 01:00','01-01 02:00']}],pointSets:[{timeline:0,values:null},{timeline:0,values:null},{timeline:0,values:null}],pointBindings:[
    {path:['series',0,'points'],data:0},{path:['purposeResults','comfort','zones',0,'metrics',0,'points'],data:1},
    {path:['series',1,'points'],data:2},{path:['purposeResults','hvacLoops',0,'series',0,'points'],data:2}
  ]};
  const payloads=new Map(),part=(id,path,value)=>{const body=JSON.stringify([value]);payloads.set(id,body);return {id,byteLength:new TextEncoder().encode(body).length,bindings:[{path}]};};
  const manifest={schema:'semantic-idf.simulation-parts/v1',runId:'comparison-resources',textHash:'comparison-input',snapshot:'comparison-snapshot',root,parts:[
    part('weather',['pointSets',0,'values'],[-5,0]),part('comfort',['pointSets',1,'values'],[22,24]),
    part('hvac',['pointSets',2,'values'],[14,16]),part('heatflow',['result','purposeResults','zoneHeatFlow','zones',0,'values'],[[0,500]]),
    part('energy-source',['result','purposeResults','energyExplanation','sources',0,'hourlyEnergy','values'],[7,11])
  ]};
  // Route checks use the supported main-thread parser; --dump-dom's virtual
  // clock can finish before a new worker starts even when resource loading works.
  const requests=[],originalWorker=window.Worker;let result;window.Worker=undefined;
  try{result=openSimulationResultParts(manifest,{backgroundBytes:0,fetcher:async url=>{const id=new URL(url,location.href).searchParams.get('partId');requests.push(id);return new Response(payloads.get(id));}});}finally{window.Worker=originalWorker;}
  const store=simulationResultStore(result);
  try{
    const getter=(owner,key)=>typeof Object.getOwnPropertyDescriptor(owner,key)?.get==='function';
    check(requests.length===0&&getter(result.series[0],'points')&&getter(result.series[1],'points'),'comparison metadata eagerly fetched or expanded native observations');
    await store.ensure('comfort',{seriesIDs:['weather']});
    check(JSON.stringify([...requests].sort())===JSON.stringify(['comfort','weather']),'Comfort comparison fetched another purpose or omitted its selected environmental variable');
    check(result.series[0].points[0].value===-5&&result.series[0].points[1].value===0&&getter(result.series[1],'points'),'selected environmental data lost signed/zero values or expanded an unselected HVAC trace');
    await store.ensure('zone_heat_flow',{seriesIDs:['weather']});
    check(requests.filter(id=>id==='weather').length===1&&requests.includes('heatflow')&&!requests.includes('hvac')&&!requests.includes('energy-source'),'Heat Flow comparison reread shared weather or loaded unrelated purpose data');
    await store.ensure('energy',{seriesIDs:['energy-source']});
    check(requests.includes('energy-source')&&!requests.includes('hvac')&&result.purposeResults.energyExplanation.sources[0].hourlyEnergy.values[1]===11,'Energy comparison did not load its exact source or eagerly read a different loop');
    await store.ensure('hvac_loops',{});
    check(requests.filter(id=>id==='hvac').length===1&&result.series[1].points===result.purposeResults.hvacLoops[0].series[0].points,'purpose changes duplicated native observations or their resource fetch');
    await store.ensureAll();
    check(requests.length===5&&new Set(requests).size===5&&JSON.parse(JSON.stringify(result)).purposeResults.zoneHeatFlow.zones[0].values[0][0]===0,'comparison loading changed canonical export values or repeated a resource request');
  }finally{store.dispose();}
  document.body.dataset.simulationModuleStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.simulationModuleStatus = "failed";
  document.getElementById("result").textContent = error?.stack || error?.message || String(error);
}
</script>
</body>
</html>`
