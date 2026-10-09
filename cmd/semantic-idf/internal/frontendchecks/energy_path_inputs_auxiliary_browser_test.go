package frontendchecks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnergyPathInputsAndAuxiliaryActualAppBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless Energy Path inputs and auxiliary acceptance")
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
	page = strings.Replace(page, "</body>", epath142LayoutHTML+energyPathInputsAuxiliaryHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/inputs.html", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	browser := epath201FreshBrowser(t, ctx, chrome)
	browser.call("Page.enable", map[string]any{}, nil)
	browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 1600, "height": 1000, "deviceScaleFactor": 1, "mobile": false}, nil)
	browser.call("Page.navigate", map[string]any{"url": server.URL + "/src/inputs.html?manual=1"}, nil)
	for deadline := time.Now().Add(35 * time.Second); ; {
		status := string(browser.evaluate(`document.body?.dataset.inputsAuxiliaryStatus`))
		if status == `"ready"` {
			break
		}
		if status == `"failed"` || time.Now().After(deadline) {
			t.Fatalf("Energy Path setup %s: %s", status, browser.evaluate(`document.getElementById('inputs-auxiliary-result')?.textContent`))
		}
		time.Sleep(25 * time.Millisecond)
	}
	// Optional visual review uses the same native app/CSS and CDP browser as
	// the acceptance assertions; ordinary runs write no screenshot artifacts.
	directory := ""
	if review := os.Getenv("ENERGY_PATH_INPUTS_REVIEW"); review != "" {
		var err error
		directory, err = os.MkdirTemp(review, "energy-path-inputs-review-")
		if err != nil {
			t.Fatal(err)
		}
	}
	capture := func(name string) {
		if directory == "" {
			return
		}
		var screenshot struct {
			Data string `json:"data"`
		}
		browser.call("Page.captureScreenshot", map[string]any{"format": "png", "fromSurface": true, "captureBeyondViewport": false}, &screenshot)
		data, err := base64.StdEncoding.DecodeString(screenshot.Data)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, name+".png")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("temporary native Energy Path inputs screenshot: %s", path)
	}
	capture("ready-balanced")
	var point struct{ X, Y float64 }
	if err := json.Unmarshal(browser.evaluate(`window.inputsAuxiliaryPointer()`), &point); err != nil || point.X <= 0 || point.Y <= 0 {
		t.Fatalf("context connector has no native pointer target: %s", browser.evaluate(`window.inputsAuxiliaryPointer(true)`))
	}
	browser.call("Input.dispatchMouseEvent", map[string]any{"type": "mousePressed", "x": point.X, "y": point.Y, "button": "left", "clickCount": 1}, nil)
	browser.call("Input.dispatchMouseEvent", map[string]any{"type": "mouseReleased", "x": point.X, "y": point.Y, "button": "left", "clickCount": 1}, nil)
	capture("native-aux-selected")
	browser.evaluate(`window.finishInputsAuxiliary()`)
	if status := string(browser.evaluate(`document.body.dataset.inputsAuxiliaryStatus`)); status != `"passed"` {
		t.Fatalf("Energy Path acceptance %s: %s", status, browser.evaluate(`document.getElementById('inputs-auxiliary-result').textContent`))
	}
	t.Log(string(browser.evaluate(`document.getElementById('inputs-auxiliary-result').textContent`)))
	if directory != "" {
		for _, review := range []struct {
			name, language, theme string
			width                 int
		}{{"wide-final", "en", "light", 1800}, {"narrow-final", "ko", "dark", 1100}} {
			browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": review.width, "height": 1200, "deviceScaleFactor": 1, "mobile": false}, nil)
			t.Log(string(browser.evaluate(fmt.Sprintf(`window.reviewInputsAuxiliary(%q,%q)`, review.language, review.theme))))
			capture(review.name)
		}
	}
}

const energyPathInputsAuxiliaryHTML = `<pre id="inputs-auxiliary-result" hidden></pre><script type="module">
const check=(value,message)=>{if(!value)throw new Error(message);};
const report=error=>{document.body.dataset.inputsAuxiliaryStatus='failed';document.getElementById('inputs-auxiliary-result').textContent=error.stack||String(error);};
try{
 for(let attempt=0;document.body.dataset.epath142Status!=='manual'&&attempt<300;attempt++)await new Promise(resolve=>setTimeout(resolve,10));
 check(document.body.dataset.epath142Status==='manual','actual app fixture did not initialize');
 const [{state},simulation,view,chart,batch,{setLanguage}]=await Promise.all([import('/src/js/state.js'),import('/src/js/views/simulation-views.js'),import('/src/js/views/energy-path-view.js'),import('/src/js/energy-path-chart.js'),import('/src/js/batch/batch-energy-path-detail.js'),import('/src/js/i18n.js')]);
 setLanguage('en');
 const result=structuredClone(state.simulationResult),explanation=result.purposeResults.energyExplanation;
 const allGraphs=[explanation,...explanation.periods,...explanation.zoneResults.flatMap(zone=>[zone,...zone.periods])];
 const graphState={...state,simulationEnergyScopeKind:'building',simulationEnergyZoneName:'',simulationEnergyPeriod:'annual',simulationEnergyService:'all',simulationEnergySelection:'',simulationEnergyChartFrequency:'monthly'};
 for(const graph of allGraphs){
  const fan=graph.nodes.find(node=>node.endUse==='fans');fan.serviceKind='hvac';
  const pump={...structuredClone(fan),id:fan.id.replace('end_use.fans.','end_use.pumps.'),label:'Pumps',endUse:'pumps',sourceIds:fan.sourceIds.map(id=>id.replace('fans','pumps'))};
  for(const field of['value','rawValue','effectiveValue','allocatedValue','displayValue'])if(Object.hasOwn(pump,field))pump[field]*=.5;
  graph.nodes.push(pump);
  const fanBranch=graph.links.find(link=>link.fromId===fan.id&&graph.nodes.some(node=>node.id===link.toId&&node.level==='carrier'));
  const pumpBranch={...structuredClone(fanBranch),id:fanBranch.id.replaceAll('fans','pumps'),fromId:pump.id,fromValue:pump.value,toValue:pump.value,sourceIds:fanBranch.sourceIds.map(id=>fan.sourceIds.includes(id)?id.replace('fans','pumps'):id)};
  graph.links.push(pumpBranch);
  const carrier=graph.nodes.find(node=>node.id===pumpBranch.toId);for(const field of['value','rawValue','effectiveValue','allocatedValue','displayValue'])if(Object.hasOwn(carrier,field))carrier[field]+=pump.value;
  for(let index=0;index<pump.sourceIds.length;index++)if(!explanation.sources.some(source=>source.id===pump.sourceIds[index]))explanation.sources.push({...structuredClone(explanation.sources.find(source=>source.id===fan.sourceIds[index])),id:pump.sourceIds[index],name:'Pumps Energy'});
 }
 const baseline=view.prepareEnergyPathScene(explanation,graphState);
 for(const graph of allGraphs){
  const scope=graph.nodes.find(node=>node.level==='load').zoneName||'',suffix=scope?'office':'building',period=graph.id||graph.period||'annual';
  const fan=graph.nodes.find(node=>node.endUse==='fans'),pump=graph.nodes.find(node=>node.endUse==='pumps'),cool=graph.nodes.find(node=>node.level==='load'&&node.serviceKind==='cooling'),heat=graph.nodes.find(node=>node.level==='load'&&node.serviceKind==='heating');
  for(const [owner,served,coolingShare,heatingShare]of[[fan,.75,.6,.4],[pump,.5,.2,.8]])for(const [load,share]of[[cool,coolingShare],[heat,heatingShare]])graph.links.push({id:'aux.'+owner.endUse+'.'+load.serviceKind+'.'+suffix,relation:'load_to_auxiliary',fromId:load.id,toId:owner.id,fromValue:load.value*served,toValue:owner.value*share,fromUnit:'kWh',toUnit:'kWh',period,zoneName:scope,serviceKind:load.serviceKind,basis:'service_path_allocation',sourceIds:owner.sourceIds,relatedPathIds:['path.'+load.serviceKind+'.'+suffix]});
 }
 const auxiliaryScene=view.prepareEnergyPathScene(explanation,graphState);
 check(auxiliaryScene.drawing.connectors.length===4,'separate service fan/pump associations were dropped or fabricated');
 check(JSON.stringify(auxiliaryScene.drawing.ribbons)===JSON.stringify(baseline.drawing.ribbons),'auxiliary context changed quantitative ribbons/ports/COP');
 for(const graph of allGraphs){
  const load=graph.nodes.find(node=>node.level==='load'),scope=load.zoneName||'',suffix=scope?'office':'building',period=graph.id||graph.period||'annual',scale=load.value/80;
  for(const [key,kind,value,category]of[['solar','solar_incident',1000,'surface.exterior_walls'],['conv.wall','exterior_convection',50,'surface.exterior_walls'],['conv.roof','exterior_convection',-20,'surface.roofs'],['internal','internal_gains',30,'internal.people'],['storage','surface_storage',-10,'surface.exterior_walls']]){
   const id='input.'+key+'.'+suffix,sourceID='source.'+id;
   const node={id,level:'input',kind:'input.'+kind,label:key,value:Math.abs(value*scale),signedValue:value*scale,rawValue:value*scale,effectiveValue:value*scale,unit:'kWh',scaleDomain:'boundary',basis:'reported_boundary',period,zoneName:scope,aggregationBasis:'model_total',driverCategory:category,sourceIds:[sourceID]};
   graph.nodes.push(node);
   for(const driver of graph.nodes.filter(node=>node.level==='driver'&&node.driverCategory===category))graph.links.push({id:'context.'+id+'.'+driver.id,relation:'input_to_driver',fromId:id,toId:driver.id,fromValue:node.signedValue,toValue:driver.value,fromUnit:'kWh',toUnit:'kWh',period,zoneName:scope,serviceKind:driver.serviceKind,basis:'reported_boundary',sourceIds:[sourceID]});
   if(!explanation.sources.some(source=>source.id===sourceID))explanation.sources.push({id:sourceID,sourceType:'sql_variable',name:kind+' Energy',keyValue:'Office',reportingFrequency:'Monthly',sourceUnit:'J',normalizedUnit:'kWh'});
  }
 }
 const freeze=value=>{if(value&&typeof value==='object'){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};freeze(result);
 const before=JSON.stringify(result);Object.assign(state,{...graphState,simulationResult:result});simulation.renderSimulation();
 const host=document.getElementById('simulationEnergyDashboard'),scene=view.prepareEnergyPathScene(explanation,state),graph=()=>view.energyPathGraphForState(explanation,state);
 check(host.querySelectorAll('[data-energy-path-stage]').length===5,'reported inputs did not create the first stage');
 check(scene.visibleNodes.filter(node=>node.level==='input').length===4,'surface inputs were not grouped by physical quantity');
 const convection=graph().nodes.find(node=>node.kind==='input.exterior_convection');
 check(convection.signedValue===30,'opposite convection observations lost their signed aggregate');
 const monthly=Array.from({length:12},(_,month)=>view.energyPathGraphForState(explanation,{...state,simulationEnergyPeriod:'M'+(month+1)}));
 check(chart.energyPathMonthlyChartSeries(convection,'node',monthly)[0].points[0].value===7.5,'input Monthly chart lost sign or original identities');
 check(chart.energyPathMonthlyChartSeries(convection,'node',monthly)[0].points[1].value===null,'missing month became Annual/zero input');
 const auxiliary=scene.drawing.connectors.find(item=>item.relation==='load_to_auxiliary'&&item.link.serviceKind==='cooling'&&item.toValue===6);
 check(auxiliary&&auxiliary.fromValue===60&&auxiliary.toValue===6,'served-load reference or auxiliary slice changed');
 const auxMonthly=chart.energyPathMonthlyChartSeries(auxiliary.link,'link',monthly);
 check(auxMonthly[0].points[0].value===15&&auxMonthly[1].points[0].value===1.5,'auxiliary Monthly chart expanded the served load or counted consumption twice');
 const pumpAuxiliary=scene.drawing.connectors.find(item=>item.relation==='load_to_auxiliary'&&item.link.serviceKind==='cooling'&&item.toValue===1);
 check(pumpAuxiliary&&pumpAuxiliary.fromValue===40&&pumpAuxiliary.toId===auxiliary.toId&&pumpAuxiliary.path!==auxiliary.path,'grouped Fans/Pumps merged served references or selectable geometry');
 const pumpMonthly=chart.energyPathMonthlyChartSeries(pumpAuxiliary.link,'link',monthly);
 check(pumpMonthly[0].points[0].value===10&&pumpMonthly[1].points[0].value===.25,'pump Monthly subset acquired the fan reference or whole shared card');
 const inputLink=scene.drawing.connectors.find(item=>item.relation==='input_to_driver'&&item.fromId===convection.id);
 const inputMonthly=chart.energyPathMonthlyChartSeries(inputLink.link,'link',monthly);
 check(inputMonthly[0].points[0].value===7.5,'grouped input association double counted its reference');
 check(scene.drawing.ribbons.length===baseline.drawing.ribbons.length,'inputs became conserved ribbons');
 const canvas=host.querySelector('[data-energy-path-canvas]');
 const edge=id=>[...host.querySelectorAll('[data-energy-explanation-edge]')].find(element=>element.dataset.energyExplanationEdge===id&&element.tabIndex>=0);
 const button=id=>[...host.querySelectorAll('[data-energy-path-layout-node]')].find(element=>element.dataset.energyPathLayoutNode===id);
 const target=edge(auxiliary.id);check(target,'auxiliary native hit path missing');target.scrollIntoView({block:'center'});
 window.reviewInputsAuxiliary=async(language,theme)=>{
  setLanguage(language);document.documentElement.dataset.theme=theme;document.documentElement.style.setProperty('--graph-label-font-size','18px');
  document.querySelector('[data-layout-preset="analysis"]').click();
  await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
  const input=host.querySelector('[data-energy-path-input-kind="input.exterior_convection"]');input.click();
  const frequency=host.querySelector('[data-energy-path-chart-frequency]');frequency.value='monthly';frequency.dispatchEvent(new Event('change',{bubbles:true}));
  const layout=host.querySelector('[data-energy-path-layout]');layout.scrollLeft=0;
  host.querySelector('[data-energy-path-input-context]').scrollIntoView({block:'start',behavior:'instant'});
  await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
  const canvas=host.querySelector('[data-energy-path-canvas]'),r=canvas.getBoundingClientRect();
  check(layout.scrollWidth>=canvas.clientWidth,'five-stage canvas was clipped instead of scrollable');
  check([...host.querySelectorAll('[data-energy-path-layout-node]')].every(node=>{const p=node.getBoundingClientRect();return p.left>=r.left-1&&p.right<=r.right+1&&p.top>=r.top-1&&p.bottom<=r.bottom+1;}),'node escaped native five-stage canvas');
  check(document.documentElement.scrollWidth<=innerWidth+1,'five-stage inner scroll caused page-wide overflow');
  return {language,theme,viewport:innerWidth,layoutWidth:layout.clientWidth,canvasWidth:canvas.clientWidth,scrollWidth:layout.scrollWidth,inputTitle:host.querySelector('[data-energy-path-input-kind="input.exterior_convection"]').title};
 };
 window.inputsAuxiliaryPointer=(debug=false)=>{
  const evidence=[];
  for(const fraction of[.35,.5,.65,.2,.8,.1,.9]){const p=target.getPointAtLength(target.getTotalLength()*fraction),m=target.getScreenCTM(),x=p.x*m.a+p.y*m.c+m.e,y=p.x*m.b+p.y*m.d+m.f,hit=document.elementFromPoint(x,y);if(hit?.closest('[data-energy-explanation-edge]')===target)return {x,y};evidence.push({x,y,hit:hit?.outerHTML.slice(0,300)});}
  return debug?evidence:null;
 };
 window.finishInputsAuxiliary=()=>{try{
  check(state.simulationEnergySelection===auxiliary.id&&host.querySelector('[data-energy-path-link-inspector]')?.dataset.energyPathLinkInspector===auxiliary.id,'native pointer did not select auxiliary connector');
  check(host.querySelector('[data-energy-path-canvas]')===canvas,'connector selection rebuilt graph');
  button(convection.id).click();check(host.querySelector('[data-energy-path-chart-value]')?.dataset.energyPathChartValue==='7.5','input inspector shows wrong Monthly energy');
  const frequency=host.querySelector('[data-energy-path-chart-frequency]');frequency.value='hourly';frequency.dispatchEvent(new Event('change',{bubbles:true}));check(host.querySelector('[data-energy-path-chart-empty]'),'monthly boundary observation fabricated Hourly source');
  const mainSelection=state.simulationEnergySelection,batchHost=document.createElement('section');batchHost.tabIndex=-1;document.body.append(batchHost);
  const controller=batch.createBatchEnergyPathDetail({host:batchHost,resolveRun:()=>result});check(controller.open('run'), 'batch detail did not accept new input graph');
  const batchEdge=[...batchHost.querySelectorAll('[data-energy-explanation-edge]')].find(element=>element.dataset.energyExplanationEdge===auxiliary.id&&element.tabIndex>=0);check(batchEdge,'batch auxiliary hit path missing');batchEdge.focus();batchEdge.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));
  check(batchHost.querySelector('[data-energy-path-link-inspector]')?.dataset.energyPathLinkInspector===auxiliary.id,'batch selection guard dropped context connector');
  check(state.simulationEnergySelection===mainSelection,'batch detail changed main selection');
  batchEdge.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true}));check(!batchHost.querySelector('[data-energy-path-link-inspector]')&&!batchHost.hidden,'first batch Escape did not clear selection');controller.close();batchHost.remove();
  check(JSON.stringify(result)===before,'presentation mutated source graph or source evidence');
  document.body.dataset.inputsAuxiliaryStatus='passed';document.getElementById('inputs-auxiliary-result').textContent='Actual app: optional five-stage inputs, signed grouping and Monthly identity, unchanged accounting ribbons/COP, served-load auxiliary references, native pointer activation, Monthly-only gaps, batch keyboard selection, immutable source data.';
 }catch(error){report(error);}};
 document.body.dataset.inputsAuxiliaryStatus='ready';
}catch(error){report(error);}
</script>`
