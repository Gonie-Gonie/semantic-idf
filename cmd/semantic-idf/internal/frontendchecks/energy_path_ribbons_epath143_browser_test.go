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

func TestEPATH143ActualAppFrameDualScaleRibbonsBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser actual dual-scale ribbon acceptance")
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
	page = strings.Replace(page, "</body>", epath142LayoutHTML+epath143RibbonsHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/epath143-ribbons.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	windowWidth, windowHeight := 1600, 900
	runBrowser := func(action, query string) ([]byte, error) {
		return exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--force-device-scale-factor=1", fmt.Sprintf("--window-size=%d,%d", windowWidth, windowHeight), "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), action, server.URL+"/src/epath143-ribbons.html?manual=1"+query).CombinedOutput()
	}
	output, err := runBrowser("--dump-dom", "&run143=1")
	// Match the Wails content viewport, not Chrome's invisible native furniture.
	if viewport := regexp.MustCompile(`data-epath143-viewport="(\d+)x(\d+)"`).FindSubmatch(output); err == nil && len(viewport) == 3 {
		actualWidth, _ := strconv.Atoi(string(viewport[1]))
		actualHeight, _ := strconv.Atoi(string(viewport[2]))
		if actualWidth != 1600 || actualHeight != 900 {
			if actualWidth < 1200 || actualWidth > 1800 || actualHeight < 600 || actualHeight > 1000 {
				t.Fatalf("unexpected headless viewport: %dx%d", actualWidth, actualHeight)
			}
			windowWidth += 1600 - actualWidth
			windowHeight += 900 - actualHeight
			output, err = runBrowser("--dump-dom", "&run143=1")
		}
	}
	if err != nil {
		t.Fatalf("EPATH143 browser failed: %v\n%s", err, output)
	}
	if screenshot := os.Getenv("EPATH143_SCREENSHOT"); screenshot != "" {
		// Capture's later final reflow is supplemental visual evidence; calibrated
		// DOM assertions remain authoritative for viewport and clipping checks.
		windowWidth, windowHeight = 1600, 900
		if shot, shotErr := runBrowser("--screenshot="+screenshot, ""); shotErr != nil {
			t.Fatalf("EPATH143 screenshot failed: %v\n%s", shotErr, shot)
		}
	}
	document := string(output)
	diagnostic := regexp.MustCompile(`(?s)<pre id="epath143-result"[^>]*>(.*?)</pre>`).FindStringSubmatch(document)
	if !strings.Contains(document, `data-epath143-status="passed"`) {
		if len(diagnostic) == 2 {
			t.Fatalf("EPATH143 failed: %s", diagnostic[1])
		}
		t.Fatalf("EPATH143 failed:\n%s", output)
	}
	if len(diagnostic) == 2 {
		t.Log(diagnostic[1])
	}
}

// Appended after the EPATH-142 manual bootstrap in the actual checked-in index.
// ?manual=1 is an interactive fixture; &run143=1 additionally runs acceptance.
const epath143RibbonsHTML = `<pre id="epath143-result" hidden>pending</pre><script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const close=(left,right,tolerance=1e-6)=>Number.isFinite(left)&&Number.isFinite(right)&&Math.abs(left-right)<=tolerance;
const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
const intersects=(a,b,pad=0)=>a.left<b.right-pad&&a.right>b.left+pad&&a.top<b.bottom-pad&&a.bottom>b.top+pad;
document.body.dataset.epath143Viewport=innerWidth+"x"+innerHeight;
try{
 for(let attempt=0;document.body.dataset.epath142Status!=="manual"&&attempt<100;attempt++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath142Status!=="manual")throw new Error("actual EPATH-142 app bootstrap failed: "+document.getElementById("epath142-result")?.textContent);
 const [{state},simulation,view]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/views/energy-path-view.js")]);
 const originalResult=state.simulationResult,originalJSON=JSON.stringify(originalResult);
 const candidate=JSON.parse(originalJSON),explanation=candidate.purposeResults.energyExplanation;
 const revise=payload=>{
  const factor=payload.nodes.find(node=>node.level==="load"&&node.serviceKind==="cooling").value/80;
  const reported=node=>node.level==="driver"?(node.serviceKind==="cooling"?5:20)*factor:node.level==="load"?(node.serviceKind==="cooling"?40:80)*factor:node.level==="end_use"&&node.endUse==="cooling"?10*factor:node.level==="end_use"&&node.endUse==="heating"?100*factor:node.level==="carrier"?(node.carrier==="electricity"?65:105)*factor:node.value;
  payload.nodes.forEach(node=>{const value=reported(node);Object.assign(node,{value,rawValue:value,effectiveValue:value,allocatedValue:value});});
  const byID=new Map(payload.nodes.map(node=>[node.id,node]));
  payload.links.forEach(link=>{const from=byID.get(link.fromId),to=byID.get(link.toId);if(link.relation==="driver_to_load")link.fromValue=link.toValue=from.value;if(link.relation==="load_to_end_use"){link.fromValue=from.value;link.toValue=to.value;link.ratio=from.value/to.value;link.ratioKind=link.serviceKind==="cooling"?"coefficient_of_performance":"efficiency";}if(link.relation==="end_use_to_carrier"&&["cooling","heating"].includes(from.endUse))link.fromValue=link.toValue=from.value;});
  if(payload.summary){for(const[group,level]of[["drivers","driver"],["loads","load"],["endUses","end_use"],["carriers","carrier"]])payload.summary[group]=payload.nodes.filter(node=>node.level===level);}
 };
 revise(explanation);explanation.periods.forEach(revise);for(const zone of explanation.zoneResults){revise(zone);zone.periods.forEach(revise);}
 candidate.purposeResults.energyExplanationSummary=explanation.summary;candidate.runId="epath143-heating-larger-than-cooling";
 const result=freeze(candidate),fixtureJSON=JSON.stringify(result);
 Object.assign(state,{simulationResult:result,simulationRunning:false,simulationEnergyScopeKind:"building",simulationEnergyZoneName:"",simulationEnergyPeriod:"annual",simulationEnergyService:"all",simulationEnergySelection:"",simulationEnergyDetailsOpen:false,simulationEnergyDetailsTab:"data",simulationEnergyDetailsStage:"",simulationEnergyOutputSource:""});
 simulation.renderSimulation();
 const host=document.getElementById("simulationEnergyDashboard"),pane=document.querySelector("#simulationPane > .simulation-pane");
 const nodeButton=id=>[...host.querySelectorAll("[data-energy-path-layout-node]")].find(node=>node.dataset.energyPathLayoutNode===id);
 const change=(selector,value)=>{const control=host.querySelector(selector);check(Boolean(control),"missing real control "+selector);if(control){control.value=value;control.dispatchEvent(new Event("change",{bubbles:true}));}};
 const graph=()=>view.energyPathGraphForState(explanation,state);
 if(new URLSearchParams(location.search).get("run143")!=="1"){
  document.body.dataset.epath143Status="manual";document.getElementById("epath143-result").textContent="Actual EPATH-143 fixture: Heating80/Cooling40, fuel100/cooling10, COP4 and efficiency0.8. Use real scope/month/service and node controls.";
 }else{
 check(innerWidth===1600&&innerHeight===900,"actual application viewport is not 1600x900: "+innerWidth+"x"+innerHeight);
 const inspectBounds=scope=>{
  check(!document.getElementById("simulationRunSetup").open,scope+" completed setup unexpectedly reopened");
  check(pane.scrollHeight<=pane.clientHeight+1,scope+" actual inner pane requires vertical scroll: "+pane.scrollHeight+"/"+pane.clientHeight);
  check(pane.scrollWidth<=pane.clientWidth+1,scope+" actual pane requires horizontal scroll");
  for(const selector of["[data-energy-path-layout]","[data-energy-path-quality-line]","[data-energy-path-legend]"]){const element=host.querySelector(selector),r=element?.getBoundingClientRect();check(r&&r.top>=0&&r.bottom<=innerHeight+1,scope+" graph/quality/legend clipped: "+selector+" bottom="+r?.bottom);}
  for(const node of graph().nodes){const button=nodeButton(node.id),r=button?.getBoundingClientRect();check(r&&r.left>=0&&r.right<=innerWidth+1&&r.top>=0&&r.bottom<=innerHeight+1,scope+" node outside viewport: "+node.id);if(r){const hit=document.elementFromPoint(r.left+r.width/2,r.top+r.height/2);check(hit&&button.contains(hit),scope+" node blocked by ribbon/ratio: "+node.id);}}
 };
 inspectBounds("Building");
 const current=graph(),svg=host.querySelector("[data-energy-path-graph-underlay]"),paths=[...host.querySelectorAll("[data-energy-path-ribbon]")];
 check(svg?.tagName.toLowerCase()==="svg"&&svg.getAttribute("aria-hidden")==="true","decorative ribbon geometry lost aria-hidden SVG separation");
 check(paths.length===23,"actual SVG omitted/duplicated additive branches: "+paths.length);
 check(current.relations.length===2&&!paths.some(path=>current.relations.some(relation=>relation.id===path.dataset.energyPathRibbon)),"source correspondence was drawn as additive energy ribbon");
 // EPATH-145 adds a separate accessible hit layer without changing these
 // quantitative ribbons. Conversion ratio buttons provide the sole tab stop.
 check(!svg.querySelector("[data-energy-explanation-edge]")&&host.querySelectorAll("[data-energy-path-link-hit]").length===paths.length,"edge interaction changed quantitative SVG or duplicated its hit targets");
 const domains=new Map(),widthEvidence=[];
 for(const path of paths){
  const id=path.dataset.energyPathRibbon,link=current.links.find(item=>item.id===id),d=path.getAttribute("d"),bbox=path.getBBox();
  check(link&&d&&!/NaN|Infinity|undefined/.test(d)&&bbox.width>0&&bbox.height>0,"real ribbon path is absent/non-finite: "+id);
  if(!link)continue;
  const fromValue=Number(path.dataset.fromValue),toValue=Number(path.dataset.toValue),fromWidth=Number(path.dataset.fromWidth),toWidth=Number(path.dataset.toWidth);
  check(close(fromValue,link.fromValue)&&close(toValue,link.toValue),"ribbon changed reported dual values: "+id);
  check(fromWidth>0&&toWidth>0,"positive energy ribbon has a zero/nonfinite endpoint width: "+id);
  const fromPort={x:Number(path.dataset.fromX),y0:Number(path.dataset.fromY0),y1:Number(path.dataset.fromY1)},toPort={x:Number(path.dataset.toX),y0:Number(path.dataset.toY0),y1:Number(path.dataset.toY1)};
  check(close(fromPort.y1-fromPort.y0,fromWidth)&&close(toPort.y1-toPort.y0,toWidth)&&toPort.x>fromPort.x,"SVG port coordinates do not match actual declared widths/gap: "+id);
  const epsilon=Math.min((toPort.x-fromPort.x)/10000,.001);
  for(const[port,width,direction]of[[fromPort,fromWidth,1],[toPort,toWidth,-1]]){
   const x=port.x+direction*epsilon,center=(port.y0+port.y1)/2;
   check(path.isPointInFill(new DOMPoint(x,center))&&path.isPointInFill(new DOMPoint(x,center+width*.45))&&path.isPointInFill(new DOMPoint(x,center-width*.45)),"drawn SVG fill is narrower/misplaced than its reported endpoint width: "+id);
   check(!path.isPointInFill(new DOMPoint(x,center+width*.55))&&!path.isPointInFill(new DOMPoint(x,center-width*.55)),"drawn SVG fill is wider than its reported endpoint width: "+id);
  }
  const paint=getComputedStyle(path);check(paint.fill!=="none"&&Number(paint.opacity)>0&&Number(paint.fillOpacity)>0,"quantitative ribbon has invisible fill: "+id);
  const fromScreen=new DOMPoint(fromPort.x,(fromPort.y0+fromPort.y1)/2).matrixTransform(svg.getScreenCTM()),toScreen=new DOMPoint(toPort.x,(toPort.y0+toPort.y1)/2).matrixTransform(svg.getScreenCTM());
  check(fromScreen.x>=nodeButton(link.fromId).getBoundingClientRect().right-.1&&toScreen.x<=nodeButton(link.toId).getBoundingClientRect().left+.1,"ribbon endpoint is hidden beneath an opaque node label: "+id);
  const isConversion=link.relation==="load_to_end_use";
  check(path.dataset.energyPathRibbonKind===(isConversion?"conversion":"same_domain"),"ribbon uses incorrect domain relation: "+id);
  for(const[domain,value,width]of[[path.dataset.fromDomain,fromValue,fromWidth],[path.dataset.toDomain,toValue,toWidth]]){
   check(["thermal","site"].includes(domain),"ribbon endpoint missing thermal/site domain: "+id);
   if(value>0){const coefficient=width/value;if(domains.has(domain))check(close(coefficient,domains.get(domain),1e-6),"same-domain widths lost uniform linear scale: "+domain+" "+id);else domains.set(domain,coefficient);}
  }
  if(!isConversion)check(close(fromWidth,toWidth),"same-domain ribbon tapers despite equal energy: "+id);
  else{const service=current.nodes.find(node=>node.id===link.fromId)?.serviceKind;check(service==="cooling"?fromWidth>toWidth:fromWidth<toWidth,"conversion taper contradicts COP/boiler direction: "+service);widthEvidence.push(service+":"+fromWidth+"→"+toWidth);}
 }
 check(domains.size===2&&[...domains.values()].every(value=>value>0),"ribbons do not expose two positive domain scales");
 const bars=[...host.querySelectorAll("[data-energy-path-bar]")];
 check(bars.length>=current.nodes.length,"quantitative node bars were replaced by nonquantitative HTML card sizes");
 for(const bar of bars){const node=current.nodes.find(node=>node.id===bar.dataset.energyPathBar),box=bar.getBBox(),domain=bar.dataset.energyPathBarDomain;check(node&&close(Number(bar.dataset.energyPathBarValue),node.value),"bar changed reported node total");if(node)check(close(box.height,node.value*domains.get(domain),1e-5),"bar is not scaled from reported node value: "+node.id);const screen=bar.getBoundingClientRect(),card=node&&nodeButton(node.id)?.getBoundingClientRect();check(screen.width>0&&screen.height>0&&card&&!intersects(screen,card,.1),"quantitative bar hidden under opaque node label: "+node?.id);}
 const ratios=[...host.querySelectorAll("[data-energy-path-bridge-ratio]")];
 check(ratios.length===2,"Cooling/Heating conversion ratio labels are missing or duplicated");
 const beforeFocus=state.simulationEnergySelection;
 for(const ratio of ratios){
  const link=current.links.find(item=>item.id===ratio.dataset.energyPathBridgeRatio),service=current.nodes.find(node=>node.id===link?.fromId)?.serviceKind,expected=service==="cooling"?4:.8;
  check(!svg.contains(ratio)&&ratio.tabIndex===0,"ratio keyboard target is hidden inside decorative SVG");
  const name=ratio.getAttribute("aria-label")||"",tooltipID=ratio.getAttribute("aria-describedby"),tooltip=tooltipID&&document.getElementById(tooltipID);
  check(link&&name.includes(String(expected))&&name.includes(String(link.fromValue))&&name.includes(String(link.toValue))&&/thermal/i.test(name)&&/site/i.test(name),"ratio accessible name omits exact ratio / dual-domain values: "+service+" "+name);
  check(service==="cooling"?/COP/i.test(name):/efficiency/i.test(name),"conversion ratio lacks correct typed label: "+service);
  ratio.focus();
  check(document.activeElement===ratio&&tooltip?.getAttribute("role")==="tooltip"&&getComputedStyle(tooltip).visibility!=="hidden"&&getComputedStyle(tooltip).display!=="none","full ratio tooltip is unavailable on keyboard focus: "+service);
  const tooltipRect=tooltip?.getBoundingClientRect(),paneRect=pane.getBoundingClientRect();
  check(tooltipRect&&tooltipRect.width>0&&tooltipRect.height>0&&tooltipRect.left>=paneRect.left&&tooltipRect.right<=paneRect.right&&tooltipRect.top>=paneRect.top&&tooltipRect.bottom<=paneRect.bottom,"keyboard ratio tooltip is clipped by the real app pane: "+service);
  check(tooltip&&/scale|kWh\/px|px\/kWh/i.test(tooltip.textContent),"ratio tooltip hides independent domain-scale explanation: "+service);
  check(state.simulationEnergySelection===beforeFocus&&!host.querySelector("[data-energy-path-inspector]"),"ratio focus triggered unsupported edge/node selection");
  const rect=ratio.getBoundingClientRect();check(rect.width>0&&rect.height>0&&rect.left>=0&&rect.right<=innerWidth&&rect.bottom<=innerHeight,"ratio label not visible inside viewport");
  for(const node of current.nodes)check(!intersects(rect,nodeButton(node.id).getBoundingClientRect(),.5),"ratio label overlaps node label: "+service+"/"+node.id);
 }
 if(ratios.length===2)check(!intersects(ratios[0].getBoundingClientRect(),ratios[1].getBoundingClientRect(),.5),"larger Heating value causes Cooling/Heating ratio labels to collide");
 for(const level of["load","end_use"]){const cooling=current.nodes.find(node=>node.level===level&&(level==="load"?node.serviceKind:node.endUse)==="cooling"),heating=current.nodes.find(node=>node.level===level&&(level==="load"?node.serviceKind:node.endUse)==="heating");check(cooling&&heating&&nodeButton(cooling.id).getBoundingClientRect().top<nodeButton(heating.id).getBoundingClientRect().top,"Cooling-first conversion alignment was replaced by descending load magnitude: "+level);}
 const legend=[...host.querySelectorAll("[data-energy-path-legend-item]")];
 check(legend.length===4&&legend.map(item=>item.dataset.energyPathLegendItem).join(",")==="thermal,site,allocated,residual","legend is not exactly Thermal/Site/Allocated/Residual");
 check(legend.map(item=>item.textContent.trim()).join("|")==="Thermal kWh|Site kWh|Allocated|Residual","legend includes detailed basis/source type clutter or wrong units");
 // Missing request metadata must not be confused with an explicit report that
 // a ratio is unavailable. Both consumers read the same period-local truth.
 const ratioVariant=(status,period="annual")=>{
  const variant=JSON.parse(originalJSON),payload=variant.purposeResults.energyExplanation;
  const selected=period==="annual"?payload:payload.periods.find(item=>item.id===period);
  for(const link of selected.links)if(link.relation==="load_to_end_use"&&link.serviceKind==="heating")link.ratioKind="efficiency";
  for(const container of[selected,selected.summary,...(period==="annual"?[variant.purposeResults.energyExplanationSummary]:[])]){
   if(!container?.quality)continue;if(status===undefined)delete container.quality.ratios;else container.quality.ratios={status,found:0,total:1};
  }
  const frozen=freeze(variant),snapshot=JSON.stringify(frozen);
  Object.assign(state,{simulationResult:frozen,simulationEnergyScopeKind:"building",simulationEnergyPeriod:period,simulationEnergyService:"cooling",simulationEnergySelection:"",simulationEnergyDetailsOpen:false});simulation.renderSimulation();
  return {frozen,snapshot,bridge:host.querySelector("[data-energy-path-bridge-ratio]"),kpi:host.querySelector('[data-energy-path-kpi-ratio="cooling"]'),path:host.querySelector('[data-energy-path-ribbon-kind="conversion"]')};
 };
 const absent=ratioVariant(undefined);
 check(Number(absent.bridge?.dataset.energyPathRatioValue)===4&&Number(absent.kpi?.dataset.energyPathKpiRatioValue)===4,"absent raw ratio-quality marker hid actual80/20 COP4 in bridge or KPI");
 const ratioPath=absent.path?.getAttribute("d");
 for(const status of["unavailable","missing","not_requested","not_applicable"]){
  const sample=ratioVariant(status);
  check(sample.bridge?.dataset.energyPathRatioKind==="unavailable"&&!sample.bridge?.hasAttribute("data-energy-path-ratio-value")&&!sample.kpi?.hasAttribute("data-energy-path-kpi-ratio-value"),"explicit ratio quality contradicts bridge/KPI numeric label: "+status);
  check(sample.bridge?.getAttribute("aria-label")?.includes("unavailable")&&sample.bridge.querySelector("strong")?.textContent.includes("—"),"unavailable bridge ratio is not honestly explained: "+status);
  check(sample.path?.getAttribute("d")===ratioPath,"quality marker changed measured paired ribbon geometry: "+status);
  check(JSON.stringify(sample.frozen)===sample.snapshot,"ratio quality rendering mutated stored fixture: "+status);
 }
 const localMissing=ratioVariant("missing","M1");
 check(localMissing.bridge?.dataset.energyPathRatioKind==="unavailable"&&!localMissing.kpi?.hasAttribute("data-energy-path-kpi-ratio-value"),"selected-month missing ratio reused complete annual ratio metadata");
 check(JSON.stringify(absent.frozen)===absent.snapshot&&JSON.stringify(localMissing.frozen)===localMissing.snapshot,"missing/absent ratio rendering mutated source payload");
 Object.assign(state,{simulationResult:result,simulationEnergyScopeKind:"building",simulationEnergyPeriod:"annual",simulationEnergyService:"all",simulationEnergySelection:""});simulation.renderSimulation();
 const selected=current.nodes.find(node=>node.driverCategory==="internal.lighting");nodeButton(selected.id).focus();nodeButton(selected.id).click();
 check(state.simulationEnergySelection===selected.id&&document.activeElement?.dataset.energyPathLayoutNode===selected.id&&host.querySelector('[data-energy-path-inspector="'+selected.id+'"]'),"SVG ribbons broke native node selection/focus/inspector");
 check(nodeButton(current.nodes.find(node=>node.endUse==="lighting").id)?.dataset.energyPathRelated==="true","SVG rendering removed nonflow lighting counterpart highlight");
 change("[data-simulation-energy-scope]","zone");change("[data-simulation-energy-zone-name]","Office");inspectBounds("Zone");
 check(host.querySelectorAll("[data-energy-path-ribbon]").length===23&&host.querySelectorAll("[data-energy-path-bridge-ratio]").length===2,"Zone equivalent graph lost quantitative ribbons or ratios");
 const finalCanvas=host.querySelector("[data-energy-path-canvas]").getBoundingClientRect();evidence.push("Zone canvas "+finalCanvas.width+"×"+finalCanvas.height+", bottom "+finalCanvas.bottom+", pane "+pane.scrollHeight+"/"+pane.clientHeight);
 check(JSON.stringify(originalResult)===originalJSON&&JSON.stringify(result)===fixtureJSON,"ribbon rendering / tooltip / scope / selection mutated original payload or export input");
 evidence.push("actual viewport "+innerWidth+"×"+innerHeight+", ribbons "+paths.length+", bars "+bars.length+", scales "+JSON.stringify(Object.fromEntries(domains))+", widths "+widthEvidence.join("; "));
 document.body.dataset.epath143Status=failures.length?"failed":"passed";document.getElementById("epath143-result").textContent=(failures.length?failures.join("\n"):"passed")+"\n"+evidence.join("\n");
 }
}catch(error){document.body.dataset.epath143Status="failed";document.getElementById("epath143-result").textContent=failures.join("\n")+"\n"+error.stack;}
</script>`
