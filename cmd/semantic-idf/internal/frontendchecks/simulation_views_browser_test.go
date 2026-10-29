package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	page := readTestFile(t, "frontend/src/index.html")
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		page = strings.Replace(page, script, "", 1)
	}
	script := strings.TrimPrefix(sliceBetween(simulationViewsModuleHarnessHTML, `<script type="module">`, `</script>`), `<script type="module">`)
	page = strings.Replace(page, "</head>", `<style>.simulation-pane{height:400px;max-height:400px;flex:none}.simulation-section{flex:none}</style></head>`, 1)
	page = strings.Replace(page, "</body>", `<pre id="result">pending</pre><script type="module">`+script+`</script></body>`, 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/simulation-module.html", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	browser := epath201FreshBrowser(t, ctx, chrome)
	simulationBrowserReady(t, ctx, browser, server.URL+"/src/simulation-module.html", "simulationModuleStatus", "result")
}

const simulationViewsModuleHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Simulation module harness</title></head>
<body data-simulation-module-status="pending">
<pre id="result">pending</pre>
<script type="module">
document.body.dataset.simulationModuleStatus='pending';
try {
  const module = await import("/src/js/views/simulation-views.js");
  if (typeof module.renderSimulation !== "function" || typeof module.initializeSimulationControls !== "function") {
    throw new Error("Simulation exports are unavailable");
  }
  const {openSimulationResultParts,simulationResultStore}=await import('/src/js/simulation-result-parts.js');
  const check=(condition,message)=>{if(!condition)throw Error(message);};
  const observed=(id,name)=>({file:'eplusout.sql',column:name,sourceId:id,reportingFrequency:'Hourly',points:null});
  const root={schema:'semantic-idf.simulation-transfer/v1',result:{runId:'comparison-resources',series:[observed('weather','Environment:Site Outdoor Air Drybulb Temperature [C]'),observed('hvac','NODE:System Node Temperature [C]')],purposeResults:{
    comfort:{zones:[{zoneName:'Office',metrics:[{name:'Zone Mean Air Temperature',unit:'C',keyValue:'Office',reportingFrequency:'Hourly',points:null}]}]},
    hvacLoops:[{name:'Loop',series:[observed('hvac','NODE:System Node Temperature [C]')]}],
    zoneHeatFlow:{unit:'W',frameCount:2,labels:['01-01 01:00','01-02 01:00'],categories:[{id:'internalConvective',label:'Internal convective gains',color:'#ffaa00'}],zones:[{name:'Office',values:null,temperatures:[22,24]}]},
    energyExplanation:{sources:[{id:'energy-source',hourlyEnergy:{basis:'reported_source',unit:'kWh',labels:['01-01 01:00','01-01 02:00'],values:null}}]}
  }},timelines:[{x:[10,40],labels:['01-01 01:00','01-02 01:00']}],pointSets:[{timeline:0,values:null},{timeline:0,values:null},{timeline:0,values:null}],pointBindings:[
    {path:['series',0,'points'],data:0},{path:['purposeResults','comfort','zones',0,'metrics',0,'points'],data:1},
    {path:['series',1,'points'],data:2},{path:['purposeResults','hvacLoops',0,'series',0,'points'],data:2}
  ]};
  const scope={kind:'building',aggregationBasis:'model_total'};
  const node=(id,level,value,extra={})=>({id,level,value,rawValue:value,effectiveValue:value,allocatedValue:value,unit:'kWh',period:'annual',aggregationBasis:'model_total',sourceIds:['energy-source'],...extra});
  Object.assign(root.result.purposeResults.energyExplanation,{schema:'semantic-idf.energy-explanation/v2',scope,period:'annual',nodes:[
    node('driver.internal.people.cooling','driver',20,{driverCategory:'internal.people',serviceKind:'cooling',scaleDomain:'thermal'}),
    node('load.cooling','load',20,{serviceKind:'cooling',scaleDomain:'thermal'}),
    node('end_use.cooling','end_use',5,{endUse:'cooling',serviceKind:'cooling',scaleDomain:'site'}),
    node('carrier.electricity','carrier',5,{carrier:'electricity',scaleDomain:'site'})
  ],links:[{id:'driver-load',fromId:'driver.internal.people.cooling',toId:'load.cooling',fromValue:20,toValue:20,fromUnit:'kWh',toUnit:'kWh',relation:'driver_to_load',serviceKind:'cooling',period:'annual'}],periods:[],zoneResults:[]});
  Object.assign(root.result.purposeResults.energyExplanation.sources[0],{name:'Zone People Convective Heating Energy',keyValue:'Office',zoneName:'Office',sourceType:'sql_report_data',sourceUnit:'J',normalizedUnit:'kWh',reportingFrequency:'Hourly'});
  root.result.status='succeeded';root.result.purposeRunPlan={purposes:['basic_energy','zone_heat_flow','hvac_loop_check','comfort_check'],outputObjects:[]};
  const payloads=new Map(),part=(id,path,value)=>{const body=JSON.stringify([value]);payloads.set(id,body);return {id,byteLength:new TextEncoder().encode(body).length,bindings:[{path}]};};
  const manifest={schema:'semantic-idf.simulation-parts/v1',runId:'comparison-resources',textHash:'comparison-input',snapshot:'comparison-snapshot',root,parts:[
    part('weather',['pointSets',0,'values'],[-5,0]),part('comfort',['pointSets',1,'values'],[22,24]),
    part('hvac',['pointSets',2,'values'],[14,16]),part('heatflow',['result','purposeResults','zoneHeatFlow','zones',0,'values'],[[0,500]]),
    part('energy-source',['result','purposeResults','energyExplanation','sources',0,'hourlyEnergy','values'],[7,11])
  ]};
  const rawManifest=JSON.stringify(manifest);
  // Exercise the supported main-thread parser first; the later replacement
  // uses the production worker in an active browser with a real animation clock.
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
    const {state}=await import('/src/js/state.js');
    const {setLanguage}=await import('/src/js/i18n.js');
    const tick=()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
    const geometry={zones:[{name:'Office',storyIndex:0}],stories:[{index:0,name:'Level 1'}],surfaces:[{name:'Office Floor',surfaceType:'Floor',zoneName:'Office',storyIndex:0,vertices:[{x:0,y:0,z:0},{x:6,y:0,z:0},{x:6,y:5,z:0},{x:0,y:5,z:0}]}]};
    state.activeResultTab='simulation';state.simulationResult=result;state.simulationActiveRunID=result.runId;state.simulationActiveResultView='energy';state.simulationRunning=false;
    state.report={geometry};state.simulationEnvironment={installations:[],weatherFolders:[]};state.simulationComfortZone='Office';state.simulationHeatFlowSelectedZone='Office';
    state.simulationHeatFlowRangeStart=0;state.simulationHeatFlowRangeEnd=-1;state.simulationHeatFlowFrameIndex=0;state.simulationHVACInspection=null;
    document.querySelectorAll('.result-pane').forEach(pane=>pane.classList.toggle('active',pane.id==='simulationPane'));
    module.initializeSimulationControls();await module.renderSimulation();await tick();
    const host=view=>document.getElementById({energy:'simulationEnergyDashboard',zone_heat_flow:'simulationHeatFlow',hvac_loops:'simulationHVACLoopResults',comfort:'simulationComfortResults'}[view]);
    const pane=document.querySelector('#simulationPane > .simulation-pane');
    const click=view=>document.querySelector('[data-simulation-result-view-button="'+view+'"]').click();
    const views=['energy','zone_heat_flow','hvac_loops','comfort'],mounted=new Map(),requestsBeforeTabs=requests.length;
    let fetches=0;const originalFetch=window.fetch;window.fetch=(...args)=>{fetches++;return originalFetch(...args);};
    try{
      for(const view of views){
        click(view);await tick();
        const root=host(view).firstElementChild;check(root,'actual Simulation shell did not mount '+view);
        const svg=['hvac_loops','comfort'].includes(view)?host(view).querySelector('svg[data-time-series-chart]'):null;
        if(svg){
          const before=Number(svg.dataset.timeSeriesHigh)-Number(svg.dataset.timeSeriesLow),box=svg.getBoundingClientRect();
          svg.dispatchEvent(new WheelEvent('wheel',{bubbles:true,cancelable:true,deltaY:-500,clientX:box.left+box.width/2}));await tick();
          check(Number(svg.dataset.timeSeriesHigh)-Number(svg.dataset.timeSeriesLow)<before,'fixture failed to zoom '+view);
        }
        pane.scrollTop=60;const scroll=pane.scrollTop;check(scroll>0,'actual shell has no scrollable '+view+' result');
        mounted.set(view,{root,svg,low:svg?.dataset.timeSeriesLow,high:svg?.dataset.timeSeriesHigh,scroll});
      }
      for(let pass=0;pass<2;pass++)for(const view of views){
        click(view);await tick();const previous=mounted.get(view);
        check(host(view).firstElementChild===previous.root,'warm tab rebuilt '+view+' DOM');
        if(previous.svg)check(host(view).contains(previous.svg)&&previous.svg.dataset.timeSeriesLow===previous.low&&previous.svg.dataset.timeSeriesHigh===previous.high,'warm tab discarded '+view+' chart viewport');
        check(Math.abs(pane.scrollTop-previous.scroll)<1,'warm tab lost '+view+' scroll');
      }
      check(requests.length===requestsBeforeTabs&&fetches===0,'warm view switching reread HTTP resources');
      click('energy');await tick();const hiddenHVAC=host('hvac_loops').firstElementChild;
      const context=state.simulationHVACInspection;context.frameIndex=1;context.frameX=undefined;
      click('hvac_loops');await tick();
      check(host('hvac_loops').firstElementChild!==hiddenHVAC&&host('hvac_loops').querySelector('[data-simulation-hvac-frame]').value==='1','hidden shared-frame change reused stale HVAC controls');
      click('energy');await tick();const oldComfort=host('comfort').firstElementChild;
      document.documentElement.style.setProperty('--graph-label-font-size','16px');click('comfort');await tick();
      check(host('comfort').firstElementChild!==oldComfort,'chart font change reused stale panel');
      click('energy');await tick();const oldLocalizedComfort=host('comfort').firstElementChild;setLanguage('ko');click('comfort');await tick();
      check(host('comfort').firstElementChild!==oldLocalizedComfort,'language change reused stale panel');setLanguage('en');
      const nextManifest=JSON.parse(rawManifest);nextManifest.runId='pending-replacement';nextManifest.root.result.runId='pending-replacement';nextManifest.snapshot='pending-snapshot';
      let release;const gate=new Promise(resolve=>{release=resolve;});const nextRequests=[];
      const next=openSimulationResultParts(nextManifest,{backgroundBytes:0,fetcher:async url=>{const id=new URL(url,location.href).searchParams.get('partId');nextRequests.push(id);await gate;return new Response(payloads.get(id));}});
      const nextStore=simulationResultStore(next),oldRoots=views.map(view=>host(view).firstElementChild);
      try{
        state.simulationResult=next;state.simulationActiveRunID=next.runId;state.simulationActiveResultView='comfort';
        const pending=module.renderSimulation();click('hvac_loops');click('energy');
        check(oldRoots.every(root=>!root.isConnected),'result replacement retained old mounted result panels');
        release();await pending;await nextStore.ensureAll();await tick();
        check(state.simulationActiveResultView==='energy'&&!document.querySelector('[data-simulation-result-view="energy"]').hidden&&host('energy').querySelector('.energy-path-view'),'rapid pending tab changes lost latest Energy intent');
        check(!host('comfort').firstElementChild&&!host('hvac_loops').firstElementChild,'stale pending tab completion remounted a hidden result');
        check(new Set(nextRequests).size===nextRequests.length,'rapid pending tabs duplicated a resource request');
      }finally{release();nextStore.dispose();}
    }finally{window.fetch=originalFetch;}
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
