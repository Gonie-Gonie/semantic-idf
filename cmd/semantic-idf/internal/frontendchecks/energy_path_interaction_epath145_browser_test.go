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

func TestEPATH145ActualAppEdgeSelectionAndDirectedFocusBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser actual Energy Path interaction acceptance")
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
	page = strings.Replace(page, "</body>", epath142LayoutHTML+epath143RibbonsHTML+epath144ColorsHTML+epath145InteractionHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/epath145-interaction.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	windowWidth, windowHeight := 1600, 900
	runBrowser := func(action, query string) ([]byte, error) {
		return exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--force-device-scale-factor=1", fmt.Sprintf("--window-size=%d,%d", windowWidth, windowHeight), "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), action, server.URL+"/src/epath145-interaction.html?manual=1"+query).CombinedOutput()
	}
	output, err := runBrowser("--dump-dom", "&run145=1")
	if viewport := regexp.MustCompile(`data-epath145-viewport="(\d+)x(\d+)"`).FindSubmatch(output); err == nil && len(viewport) == 3 {
		actualWidth, _ := strconv.Atoi(string(viewport[1]))
		actualHeight, _ := strconv.Atoi(string(viewport[2]))
		if actualWidth != 1600 || actualHeight != 900 {
			if actualWidth < 1200 || actualWidth > 1800 || actualHeight < 600 || actualHeight > 1000 {
				t.Fatalf("unexpected headless viewport: %dx%d", actualWidth, actualHeight)
			}
			windowWidth += 1600 - actualWidth
			windowHeight += 900 - actualHeight
			output, err = runBrowser("--dump-dom", "&run145=1")
		}
	}
	if err != nil {
		t.Fatalf("EPATH145 browser failed: %v\n%s", err, output)
	}
	for _, mode := range []string{"default", "selected"} {
		if screenshot := os.Getenv("EPATH145_SCREENSHOT_" + strings.ToUpper(mode)); screenshot != "" {
			windowWidth, windowHeight = 1600, 900
			if shot, shotErr := runBrowser("--screenshot="+screenshot, "&selection="+mode); shotErr != nil {
				t.Fatalf("EPATH145 %s screenshot failed: %v\n%s", mode, shotErr, shot)
			}
		}
	}
	document := string(output)
	diagnostic := regexp.MustCompile(`(?s)<pre id="epath145-result"[^>]*>(.*?)</pre>`).FindStringSubmatch(document)
	if !strings.Contains(document, `data-epath145-status="passed"`) {
		if len(diagnostic) == 2 {
			t.Fatalf("EPATH145 failed: %s", diagnostic[1])
		}
		t.Fatalf("EPATH145 failed:\n%s", output)
	}
	if len(diagnostic) == 2 {
		t.Log(diagnostic[1])
	}
}

const epath145InteractionHTML = `<pre id="epath145-result" hidden>pending</pre><script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
const near=(left,right,tolerance=.01)=>Math.abs(left-right)<=tolerance;
document.body.dataset.epath145Viewport=innerWidth+"x"+innerHeight;
try{
 for(let attempt=0;document.body.dataset.epath144Status!=="manual"&&attempt<200;attempt++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath144Status!=="manual")throw new Error("actual EPATH-144 bootstrap failed: "+document.getElementById("epath144-result")?.textContent);
 const [{state},simulation,view]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/views/energy-path-view.js")]);
 const result=state.simulationResult,originalJSON=JSON.stringify(result),explanation=result.purposeResults.energyExplanation;
 const host=document.getElementById("simulationEnergyDashboard"),pane=document.querySelector("#simulationPane > .simulation-pane");
 const graph=()=>view.energyPathGraphForState(state.simulationResult.purposeResults.energyExplanation,state);
 const node=id=>[...host.querySelectorAll("[data-energy-path-layout-node]")].find(element=>element.dataset.energyPathLayoutNode===id);
 const ribbon=id=>[...host.querySelectorAll("[data-energy-path-ribbon]")].find(element=>element.dataset.energyPathRibbon===id);
 const hit=id=>[...host.querySelectorAll("[data-energy-path-link-hit]")].find(element=>element.dataset.energyPathLinkHit===id);
 const ratio=id=>[...host.querySelectorAll("[data-energy-path-bridge-ratio]")].find(element=>element.dataset.energyPathBridgeRatio===id);
 const change=(selector,value)=>{const control=host.querySelector(selector);check(Boolean(control),"missing real Energy control "+selector);if(control){control.value=value;control.dispatchEvent(new Event("change",{bubbles:true}));}};
 const key=(element,value)=>{const event=new KeyboardEvent("keydown",{key:value,code:value===" "?"Space":value,bubbles:true,cancelable:true});element.focus();element.dispatchEvent(event);return event;};
 const click=element=>element?.dispatchEvent(new MouseEvent("click",{bubbles:true,cancelable:true}));
 const opacity=element=>element?Number(getComputedStyle(element).opacity):NaN;
 const assertOpacity=(element,expected,label)=>check(near(opacity(element),expected),label+" opacity="+opacity(element)+" expected="+expected);
 const barNodes=id=>[...host.querySelectorAll("[data-energy-path-bar]")].filter(element=>element.dataset.energyPathBar===id);
 const positions=()=>JSON.stringify([...host.querySelectorAll("[data-energy-path-layout-node]")].map(element=>[element.dataset.energyPathLayoutNode,element.style.left,element.style.top,element.style.width,element.style.height]));
 const paths=()=>JSON.stringify([...host.querySelectorAll("[data-energy-path-ribbon]")].map(element=>[element.dataset.energyPathRibbon,element.getAttribute("d"),element.dataset.fromWidth,element.dataset.toWidth,element.dataset.fromValue,element.dataset.toValue]));
 const current=graph(),lightingDriver=current.nodes.find(item=>item.driverCategory==="internal.lighting"),lightingUse=current.nodes.find(item=>item.endUse==="lighting");
 if(new URLSearchParams(location.search).get("run145")!=="1"){
  if(new URLSearchParams(location.search).get("selection")==="selected")click(node(lightingDriver.id));
  document.body.dataset.epath145Status="manual";document.getElementById("epath145-result").textContent="Actual145fixture: selectnodesorlinks; Enter/Space onlinks, Escape/blankgraphclear. Lightingdriver highlightsHeating/Gas pluscounterpartcardonly.";
 }else{
 check(innerWidth===1600&&innerHeight===900,"actual145viewport is not1600x900");
 const originalPositions=positions(),originalPaths=paths();
 const physical=current.links.filter(link=>ribbon(link.id));
 check(physical.length===24&&host.querySelectorAll("[data-energy-path-ribbon]").length===24,"interaction overlay changed physical ribbon count");
 check(host.querySelectorAll("[data-energy-path-link-hit]").length===24,"physical links lack one separate accessible hit target each");
 const underlay=host.querySelector("[data-energy-path-graph-underlay]"),hitLayer=host.querySelector("[data-energy-path-hit-layer]");
 check(underlay?.getAttribute("aria-hidden")==="true"&&hitLayer&&!underlay.contains(hitLayer)&&hitLayer.getAttribute("aria-hidden")!=="true","keyboard hit targets are hidden inside quantitative SVG");
 check(!host.querySelector("[data-energy-path-link-inspector]"),"unselected graph rendered link details");
 check(!underlay.querySelector("text")&&!hitLayer.querySelector("text"),"default edges repeat visible value labels instead of ratio-only labels");
 check(host.querySelectorAll("[data-energy-path-bridge-ratio]").length===2,"automatic simplification lost essential conversion ratios");
 for(const link of physical){
  const target=hit(link.id),conversion=link.relation==="load_to_end_use";
  check(target?.dataset.energyExplanationEdge===link.id&&target.getAttribute("role")==="button","hit path has no actual edge identity / button role: "+link.id);
  check(target?.tabIndex===(conversion?-1:0),"incorrect SVG edge keyboard tabstop: "+link.id);
  check(target?.getAttribute("aria-label")&&target.querySelector("title")?.textContent.includes("kWh"),"small edge omitted accessible exact-value tooltip: "+link.id);
  if(conversion){const control=ratio(link.id);check(control?.tagName==="BUTTON"&&control.type==="button"&&control.tabIndex===0&&control.dataset.energyExplanationEdge===link.id,"conversion ratio is not the sole native conversion tabstop");check(Boolean(control?.compareDocumentPosition(hitLayer)&Node.DOCUMENT_POSITION_FOLLOWING),"native conversion controls come after SVG in keyboard DOM order");}
 }
 const taxonomy=level=>[...host.querySelectorAll('[data-energy-path-stage="'+level+'"] [data-energy-path-layout-node]')].map(element=>current.nodes.find(item=>item.id===element.dataset.energyPathLayoutNode));
 check(taxonomy("driver").map(item=>item.driverCategory).join("|")==="surface.exterior_walls|surface.roofs|surface.ground_floors|surface.windows_doors|surface.interzone|air.infiltration|air.mechanical_ventilation|air.interzone|internal.people|internal.lighting|internal.equipment|interzone.transfer","driver taxonomy order depends on values/inputorder");
 check(taxonomy("load").map(item=>item.serviceKind).join(",")==="cooling,heating","load service order is not fixed");
 check(taxonomy("end_use").map(item=>item.level==="residual"?"residual":item.endUse).join(",")==="cooling,heating,fans_pumps,lighting,equipment,water_systems,refrigeration,other,residual","end-use order is not fixed taxonomy / residuallast");
 check(taxonomy("carrier").map(item=>item.carrier).join(",")==="electricity,natural_gas","carrier order follows descending values instead of fixed taxonomy");
 check(pane.scrollHeight<=pane.clientHeight+1&&pane.scrollWidth<=pane.clientWidth+1,"default interaction layer creates app scrolling");
 // This virtual-time fixture checks final paint. Retained buttons now animate,
 // so finish their test-only animation clock; EPATH161 measures natural timing.
 const settleOpacity=()=>{const controls=[...host.querySelectorAll("[data-energy-path-layout-node],[data-energy-path-bridge-ratio]")];controls.forEach(opacity);for(const control of controls)for(const animation of control.getAnimations())animation.finish();controls.forEach(opacity);};
 const assertClear=async label=>{check(state.simulationEnergySelection===""&&!host.querySelector("[data-energy-path-link-inspector]"),label+" did not clear selection/detail");await settleOpacity();for(const element of host.querySelectorAll("[data-energy-path-layout-node],[data-energy-path-bar],[data-energy-path-ribbon],[data-energy-path-bridge-ratio]"))assertOpacity(element,1,label+" restore");check(positions()===originalPositions&&paths()===originalPaths,label+" modified quantitative geometry");};
 const heatingLoad=current.nodes.find(item=>item.level==="load"&&item.serviceKind==="heating"),heatingUse=current.nodes.find(item=>item.endUse==="heating"),gas=current.nodes.find(item=>item.carrier==="natural_gas"&&item.level==="carrier"),electricity=current.nodes.find(item=>item.carrier==="electricity"&&item.level==="carrier");
 click(node(lightingDriver.id));
 check(state.simulationEnergySelection===lightingDriver.id&&document.activeElement?.dataset.energyPathLayoutNode===lightingDriver.id,"native driver selection/focus was broken by overlay");
 await settleOpacity();
 const expectedLinks=physical.filter(link=>link.fromId===lightingDriver.id||link.fromId===heatingLoad.id&&link.toId===heatingUse.id||link.fromId===heatingUse.id&&link.toId===gas.id),expectedIDs=new Set(expectedLinks.map(link=>link.id));
 check(expectedIDs.size===3,"selection fixture does not have its expected three-stage Heating path");
 for(const link of physical)assertOpacity(ribbon(link.id),expectedIDs.has(link.id)?1:.25,"directed Lighting demand "+link.id);
 for(const id of[lightingDriver.id,heatingLoad.id,heatingUse.id,gas.id]){assertOpacity(node(id),1,"selected demand node");for(const bar of barNodes(id))assertOpacity(bar,1,"selected demand bar");}
 assertOpacity(node(lightingUse.id),1,"correspondence counterpart card");check(node(lightingUse.id).dataset.energyPathFocus==="counterpart"&&node(lightingUse.id).dataset.energyPathRelated==="true","correspondence counterpart loses its distinct nonflow encoding");
 for(const bar of barNodes(lightingUse.id))assertOpacity(bar,.25,"counterpart physical bar must not imply causal demand flow");
 assertOpacity(node(electricity.id),.25,"shared Electricity must not flood unrelated branches");
 for(const control of host.querySelectorAll("[data-energy-path-bridge-ratio]"))assertOpacity(control,expectedIDs.has(control.dataset.energyPathBridgeRatio)?1:.25,"conversion ratio focus");
 node(electricity.id).focus();opacity(node(electricity.id));
 // Check the focus transition's final paint without disabling production CSS.
 await settleOpacity();
 check(node(electricity.id).matches(":focus-visible"),"keyboard readability fixture did not establish focus-visible modality");assertOpacity(node(electricity.id),1,"unrelated keyboard-focused card remains readable");for(const bar of barNodes(electricity.id))assertOpacity(bar,.25,"keyboard focus alone must not activate unrelated physical bars");check(state.simulationEnergySelection===lightingDriver.id,"keyboard focus alone changed selected demand path");node(lightingDriver.id).focus();
 key(node(lightingDriver.id),"Escape");await assertClear("Escape");
 const coolingLink=physical.find(link=>link.relation==="load_to_end_use"&&link.serviceKind==="cooling"),coolingControl=ratio(coolingLink.id);
 coolingControl.focus();check(state.simulationEnergySelection==="","ratio focus alone selected a link");coolingControl.click();
 check(state.simulationEnergySelection===coolingLink.id&&document.activeElement?.dataset.energyExplanationEdge===coolingLink.id,"native conversion button did not select exact link/focus");
 const detail=host.querySelector('[data-energy-path-link-inspector="'+coolingLink.id+'"]');
 for(const [field,value]of[["from",coolingLink.fromValue],["to",coolingLink.toValue],["ratio",4]])check(detail?.querySelector('[data-energy-path-link-value="'+field+'"]')?.textContent.includes(String(value)),"selected conversion detail omits exact "+field+" value");
 check(detail?.querySelector('[data-energy-path-link-value="from"]')?.textContent.includes("thermal")&&detail?.querySelector('[data-energy-path-link-value="to"]')?.textContent.includes("site"),"selected link detail conflates thermal and site units");
 check(detail?.querySelector('[data-energy-path-link-value="basis"]'),"selected link omitted calculation basis");
 key(ratio(coolingLink.id),"Escape");await assertClear("conversion Escape");
 const keyboardLink=physical.find(link=>link.fromId===lightingDriver.id);
 for(const pressed of["Enter"," "]){const beforeScroll=pane.scrollTop,event=key(hit(keyboardLink.id),pressed);check(event.defaultPrevented&&pane.scrollTop===beforeScroll,"SVG "+JSON.stringify(pressed)+" did not prevent native scrolling");check(state.simulationEnergySelection===keyboardLink.id&&document.activeElement?.dataset.energyExplanationEdge===keyboardLink.id&&host.querySelector('[data-energy-path-link-inspector="'+keyboardLink.id+'"]'),"SVG keyboard activation did not select exact link and restore focus");key(hit(keyboardLink.id),"Escape");await assertClear("SVG clear");}
 const tiny=physical.find(link=>link.relation==="residual"),tinyPath=ribbon(tiny.id),tinyHit=hit(tiny.id),matrix=tinyHit.getScreenCTM(),inverse=tinyPath.getScreenCTM().inverse();
 check(Number(tinyPath.dataset.fromWidth)<2&&Number(tinyPath.dataset.fromWidth)>0,"tiny-edge fixture does not exercise a true sub2px quantitative ribbon");
 check(getComputedStyle(tinyHit).vectorEffect==="non-scaling-stroke"&&parseFloat(getComputedStyle(tinyHit).strokeWidth)>=8,"tiny edge has no scale-independent accessible hit width");
 let hitEvidence=null;
 for(const fraction of[.15,.25,.4]){const p=tinyHit.getPointAtLength(tinyHit.getTotalLength()*fraction),screen=new DOMPoint(p.x,p.y).matrixTransform(matrix);for(const[dx,dy]of[[0,2.5],[0,-2.5],[2.5,0],[-2.5,0]]){const point=new DOMPoint(screen.x+dx,screen.y+dy),local=point.matrixTransform(inverse),target=document.elementFromPoint(point.x,point.y);if(!tinyPath.isPointInFill(local)&&target?.dataset.energyPathLinkHit===tiny.id){hitEvidence={target,x:point.x,y:point.y};break;}}if(hitEvidence)break;}
 check(Boolean(hitEvidence),"tiny link cannot be hit outside its true fill without inflating quantitative width");
 if(hitEvidence){hitEvidence.target.dispatchEvent(new MouseEvent("click",{bubbles:true,cancelable:true,clientX:hitEvidence.x,clientY:hitEvidence.y}));check(state.simulationEnergySelection===tiny.id&&host.querySelector('[data-energy-path-link-inspector="'+tiny.id+'"]'),"observed tiny-edge hit target does not activate real link detail");}
 check(paths()===originalPaths,"tiny hit area changed the real ribbon path/width");
 const canvas=host.querySelector("[data-energy-path-canvas]"),canvasRect=canvas.getBoundingClientRect(),blank=document.elementFromPoint(canvasRect.left+3,canvasRect.top+3);click(blank);await assertClear("blank graph");
 change("[data-simulation-energy-path-period]","M1");
 const monthlyLink=graph().links.find(link=>link.relation==="load_to_end_use"&&link.serviceKind==="cooling");click(ratio(monthlyLink.id));
 check(host.querySelector('[data-energy-path-link-inspector="'+monthlyLink.id+'"] [data-energy-path-link-value="from"]')?.textContent.includes(String(monthlyLink.fromValue)),"selected monthly link reused annual thermal value");
 const adapter={genericCaptureContext:()=>({marker:"145-history"}),genericRestoreContext:async()=>{}},snapshot=simulation.captureSimulationNavigationContext(adapter);
 change("[data-simulation-energy-scope]","zone");change("[data-simulation-energy-zone-name]","Office");check(state.simulationEnergySelection===""&&!host.querySelector("[data-energy-path-link-inspector]"),"scope change retained an out-of-scope selected edge");
 check(pane.scrollHeight<=pane.clientHeight+1&&pane.scrollWidth<=pane.clientWidth+1,"Zone interaction graph requires scrolling");
 const restored=await simulation.restoreSimulationNavigationContext(snapshot,adapter);check(restored&&state.simulationEnergyScopeKind==="building"&&state.simulationEnergyPeriod==="M1"&&state.simulationEnergySelection===monthlyLink.id&&host.querySelector('[data-energy-path-link-inspector="'+monthlyLink.id+'"] [data-energy-path-link-value="to"]')?.textContent.includes(String(monthlyLink.toValue)),"history restore dropped exact selected monthly link/scope/dual values");
 key(ratio(monthlyLink.id),"Escape");change("[data-simulation-energy-path-period]","annual");assertClear("history clear");
 const varied=JSON.parse(originalJSON),payload=varied.purposeResults.energyExplanation,byID=new Map(payload.nodes.map(item=>[item.id,item]));
 const factor=item=>item.serviceKind==="cooling"?2:item.serviceKind==="heating"?.5:1;
 for(const item of payload.nodes)if(item.level==="driver"||item.level==="load"||item.level==="end_use"&&["cooling","heating"].includes(item.endUse)){for(const field of["value","rawValue","effectiveValue","allocatedValue"])item[field]*=factor(item);}
 for(const link of payload.links){const from=byID.get(link.fromId);if(["driver_to_load","load_to_end_use"].includes(link.relation)||link.relation==="end_use_to_carrier"&&["cooling","heating"].includes(from?.endUse)){link.fromValue*=factor(from);link.toValue*=factor(from);}if(link.relation==="source_correspondence")link.fromValue*=factor(from);}
 for(const carrier of payload.nodes.filter(item=>item.level==="carrier")){const total=payload.links.filter(link=>link.toId===carrier.id).reduce((sum,link)=>sum+link.toValue,0);for(const field of["value","rawValue","effectiveValue","allocatedValue"])carrier[field]=total;}
 for(const row of payload.reconciliation||[]){if(row.carrier==="electricity"){row.expectedValue=payload.nodes.find(item=>item.level==="carrier"&&item.carrier==="electricity").value;row.explainedValue=row.expectedValue-row.residualValue;}}
 for(const[group,level]of[["drivers","driver"],["loads","load"],["endUses","end_use"],["carriers","carrier"]])payload.summary[group]=payload.nodes.filter(item=>item.level===level);
 varied.purposeResults.energyExplanationSummary=payload.summary;payload.nodes.reverse();payload.links.reverse();const frozen=freeze(varied),variedJSON=JSON.stringify(frozen);
 Object.assign(state,{simulationResult:frozen,simulationEnergyScopeKind:"building",simulationEnergyPeriod:"annual",simulationEnergyService:"all",simulationEnergySelection:""});simulation.renderSimulation();
 check(positions()===originalPositions,"valid changed demand magnitudes/reversed input reordered taxonomy or node geometry");
 check(JSON.stringify(result)===originalJSON&&JSON.stringify(frozen)===variedJSON,"selection/hit targets/history/taxonomy mutated raw source/export inputs");
 evidence.push("24quantitative ribbons; true tiny width="+tinyPath.dataset.fromWidth+"px; independent8pxhit; directedLighting→Heating→Gas; exact1600×900");
 document.body.dataset.epath145Status=failures.length?"failed":"passed";document.getElementById("epath145-result").textContent=(failures.length?failures.join("\n"):"passed")+"\n"+evidence.join("\n");
 }
}catch(error){document.body.dataset.epath145Status="failed";document.getElementById("epath145-result").textContent=failures.join("\n")+"\n"+error.stack;}
</script>`
