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

func TestEPATH144ActualAppThemeColorsAndNonColorEncodingBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser actual Energy Path theme acceptance")
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
	page = strings.Replace(page, "</body>", epath142LayoutHTML+epath143RibbonsHTML+epath144ColorsHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/epath144-colors.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	windowWidth, windowHeight := 1600, 900
	runBrowser := func(action, query string) ([]byte, error) {
		return exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--force-device-scale-factor=1", fmt.Sprintf("--window-size=%d,%d", windowWidth, windowHeight), "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), action, server.URL+"/src/epath144-colors.html?manual=1"+query).CombinedOutput()
	}
	output, err := runBrowser("--dump-dom", "&run144=1")
	if viewport := regexp.MustCompile(`data-epath144-viewport="(\d+)x(\d+)"`).FindSubmatch(output); err == nil && len(viewport) == 3 {
		actualWidth, _ := strconv.Atoi(string(viewport[1]))
		actualHeight, _ := strconv.Atoi(string(viewport[2]))
		if actualWidth != 1600 || actualHeight != 900 {
			if actualWidth < 1200 || actualWidth > 1800 || actualHeight < 600 || actualHeight > 1000 {
				t.Fatalf("unexpected headless viewport: %dx%d", actualWidth, actualHeight)
			}
			windowWidth += 1600 - actualWidth
			windowHeight += 900 - actualHeight
			output, err = runBrowser("--dump-dom", "&run144=1")
		}
	}
	if err != nil {
		t.Fatalf("EPATH144 browser failed: %v\n%s", err, output)
	}
	for _, theme := range []string{"light", "dark"} {
		if screenshot := os.Getenv("EPATH144_SCREENSHOT_" + strings.ToUpper(theme)); screenshot != "" {
			// Capture performs a later viewport reflow. These supplemental images
			// do not replace the calibrated DOM contrast and clipping assertions.
			windowWidth, windowHeight = 1600, 900
			if shot, shotErr := runBrowser("--screenshot="+screenshot, "&theme="+theme); shotErr != nil {
				t.Fatalf("EPATH144 %s screenshot failed: %v\n%s", theme, shotErr, shot)
			}
		}
	}
	document := string(output)
	diagnostic := regexp.MustCompile(`(?s)<pre id="epath144-result"[^>]*>(.*?)</pre>`).FindStringSubmatch(document)
	if !strings.Contains(document, `data-epath144-status="passed"`) {
		if len(diagnostic) == 2 {
			t.Fatalf("EPATH144 failed: %s", diagnostic[1])
		}
		t.Fatalf("EPATH144 failed:\n%s", output)
	}
	if len(diagnostic) == 2 {
		t.Log(diagnostic[1])
	}
}

const epath144ColorsHTML = `<pre id="epath144-result" hidden>pending</pre><script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
const settlePaint=()=>{const node=document.querySelector("[data-energy-path-layout-node]");if(node){getComputedStyle(node).color;getComputedStyle(node).backgroundColor;}return new Promise(resolve=>setTimeout(resolve,180));};
document.body.dataset.epath144Viewport=innerWidth+"x"+innerHeight;
try{
 for(let attempt=0;document.body.dataset.epath143Status!=="manual"&&attempt<150;attempt++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath143Status!=="manual")throw new Error("actual EPATH-143 bootstrap failed: "+document.getElementById("epath143-result")?.textContent);
 const [{state},simulation,view]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/views/energy-path-view.js")]);
 const previousResult=state.simulationResult,previousJSON=JSON.stringify(previousResult),candidate=JSON.parse(previousJSON),explanation=candidate.purposeResults.energyExplanation;
 const electricity=explanation.nodes.find(node=>node.level==="carrier"&&node.carrier==="electricity"),beforeElectricity=electricity.value;
 for(const key of["value","rawValue","effectiveValue","allocatedValue"])electricity[key]+=3;
 electricity.badges=["carrier_residual"];
 const residual={id:"residual.site_electricity.building",level:"residual",kind:"energy.residual",label:"Unclassified energy",carrier:"electricity",value:3,signedValue:3,rawValue:3,effectiveValue:3,allocatedValue:3,unit:"kWh",period:"annual",scaleDomain:"site",basis:"residual",badges:["unclassified_energy"],sourceIds:electricity.sourceIds};
 explanation.nodes.push(residual);explanation.links.push({id:"residual.electricity.annual",fromId:residual.id,toId:electricity.id,relation:"residual",basis:"residual",fromValue:3,toValue:3,fromUnit:"kWh",toUnit:"kWh",period:"annual",sourceIds:electricity.sourceIds});
 explanation.reconciliation=[...(explanation.reconciliation||[]),{id:"reconcile.energy.electricity.annual",level:"energy",period:"annual",carrier:"electricity",status:"residual",expectedValue:electricity.value,explainedValue:beforeElectricity,residualValue:3,unit:"kWh",basis:"carrier_residual"}];
 const cooling=explanation.nodes.find(node=>node.level==="end_use"&&node.endUse==="cooling");
 Object.assign(cooling,{allocationApplied:true,basis:"service_path_allocation",allocationExplanation:"Explicit HVAC service-path allocation"});
 for(const link of explanation.links)if(link.fromId===cooling.id&&link.relation==="end_use_to_carrier")Object.assign(link,{allocationApplied:true,basis:"service_path_allocation"});
 for(const[group,level]of[["drivers","driver"],["loads","load"],["endUses","end_use"],["carriers","carrier"]])explanation.summary[group]=explanation.nodes.filter(node=>node.level===level);
 candidate.purposeResults.energyExplanationSummary=explanation.summary;candidate.runId="epath144-paint-only-fixture";
 const result=freeze(candidate),fixtureJSON=JSON.stringify(result);
 Object.assign(state,{simulationResult:result,simulationEnergyScopeKind:"building",simulationEnergyZoneName:"",simulationEnergyPeriod:"annual",simulationEnergyService:"all",simulationEnergySelection:"",simulationEnergyDetailsOpen:false});
 document.documentElement.dataset.theme=new URLSearchParams(location.search).get("theme")==="dark"?"dark":"light";
 simulation.renderSimulation();
 const host=document.getElementById("simulationEnergyDashboard"),pane=document.querySelector("#simulationPane > .simulation-pane");
 const graph=()=>view.energyPathGraphForState(explanation,state);
 const button=id=>[...host.querySelectorAll("[data-energy-path-layout-node]")].find(node=>node.dataset.energyPathLayoutNode===id);
 const change=(selector,value)=>{const control=host.querySelector(selector);check(Boolean(control),"missing real control "+selector);if(control){control.value=value;control.dispatchEvent(new Event("change",{bubbles:true}));}};
 const geometry=()=>JSON.stringify({nodes:[...host.querySelectorAll("[data-energy-path-layout-node]")].map(node=>[node.dataset.energyPathLayoutNode,node.getAttribute("style")]),paths:[...host.querySelectorAll("[data-energy-path-ribbon]")].map(path=>[path.dataset.energyPathRibbon,path.getAttribute("d"),path.dataset.fromValue,path.dataset.toValue,path.dataset.fromWidth,path.dataset.toWidth]),bars:[...host.querySelectorAll("[data-energy-path-bar]")].map(bar=>[bar.dataset.energyPathBar,bar.dataset.energyPathBarSide,...["x","y","width","height"].map(name=>bar.getAttribute(name))])});
 if(new URLSearchParams(location.search).get("run144")!=="1"){
  document.body.dataset.epath144Status="manual";document.getElementById("epath144-result").textContent="Actual color fixture: Cooling/Heating, allocated drivers/HVAC, positive3kWh unclassified residual. Theme="+document.documentElement.dataset.theme;
 }else{
 check(innerWidth===1600&&innerHeight===900,"theme acceptance did not use actual1600x900 viewport");
 const originalGeometry=geometry(),paths=[...host.querySelectorAll("[data-energy-path-ribbon]")],nodes=graph().nodes,nodeColorKeys=Object.fromEntries([...host.querySelectorAll("[data-energy-path-layout-node]")].map(node=>[node.dataset.energyPathLayoutNode,node.dataset.energyPathColor]));
 check(paths.length===24&&host.querySelectorAll("[data-energy-path-layout-node]").length===25,"hatch representation duplicated or omitted quantitative nodes/paths");
 const parser=document.createElement("canvas");parser.width=parser.height=1;const context=parser.getContext("2d",{willReadFrequently:true});
 // Canvas normalizes rgb(), color(srgb), color-mix() results and alpha without
 // assuming a particular computed-color serialization or skipping composition.
 const rgba=value=>{if(!value||/^url\(/.test(value))throw new Error("paint is not a scalar CSS color: "+value);context.clearRect(0,0,1,1);context.fillStyle="rgba(0,0,0,0)";context.fillStyle=value;context.fillRect(0,0,1,1);return[...context.getImageData(0,0,1,1).data].map((value,index)=>index===3?value/255:value);};
 const over=(foreground,background)=>{const alpha=foreground[3]??1;return[...foreground.slice(0,3).map((value,index)=>value*alpha+background[index]*(1-alpha)),1];};
 const background=element=>{const ancestors=[];for(let node=element;node;node=node.parentElement)ancestors.unshift(node);return ancestors.reduce((color,node)=>over(rgba(getComputedStyle(node).backgroundColor),color),[255,255,255,1]);};
 const luminance=channels=>channels.slice(0,3).map(value=>{const channel=value/255;return channel<=.04045?channel/12.92:((channel+.055)/1.055)**2.4;}).reduce((sum,value,index)=>sum+value*[.2126,.7152,.0722][index],0);
 const contrast=(first,second)=>{const a=luminance(first),b=luminance(second);return(Math.max(a,b)+.05)/(Math.min(a,b)+.05);};
 const patternFor=element=>{const fill=getComputedStyle(element).fill;if(!/^url\(/.test(fill))return null;const hash=fill.lastIndexOf("#"),id=fill.slice(hash+1).replace(/["')]/g,"");return document.getElementById(id);};
 const shapeBase=element=>{const pattern=patternFor(element),base=pattern?.querySelector("rect");return rgba(getComputedStyle(base||element).fill);};
 const themeColors=new Map();
 for(const theme of["light","dark"]){
  document.documentElement.dataset.theme=theme;
  // Preserve production transitions; sample their settled paint, never a
  // transient mix of the old foreground and the new theme background.
  await settlePaint();
  const textRatios=[],graphicRatios=[],patternRatios=[],badgeRatios=[];
  check(geometry()===originalGeometry,theme+" theme changed quantitative paths/ports or node geometry");
  check(pane.scrollHeight<=pane.clientHeight+1&&pane.scrollWidth<=pane.clientWidth+1,theme+" Building paint/badges introduced actual pane scrolling: "+pane.scrollHeight+"/"+pane.clientHeight);
  for(const node of nodes){
   const element=button(node.id);if(!element)continue;
   check(element.dataset.energyPathColor&&["solid","allocated","residual"].includes(element.dataset.energyPathPaint),"native node lacks stable semantic color/paint token: "+node.id);
   for(const text of[element.querySelector(".energy-path-node-label"),element.querySelector("strong")]){const bg=background(text),ratio=contrast(over(rgba(getComputedStyle(text).color),bg),bg);textRatios.push(ratio);check(ratio>=4.5&&parseFloat(getComputedStyle(text).fontSize)>=11,"node text contrast/font insufficient "+theme+" "+node.id+" "+text.tagName+": "+ratio.toFixed(3)+" foreground="+getComputedStyle(text).color+" background="+bg.join(","));}
   const rect=element.getBoundingClientRect();check(rect.bottom<=innerHeight+1,"node/badge clipped outside screen: "+node.id);
  }
  for(const selector of["[data-energy-path-bridge-ratio]","[data-energy-path-legend-item]"]){for(const element of host.querySelectorAll(selector)){const bg=background(element),ratio=contrast(over(rgba(getComputedStyle(element).color),bg),bg);textRatios.push(ratio);check(ratio>=4.5,"ratio/legend text contrast insufficient "+theme+" "+selector+": "+ratio.toFixed(3));}}
  for(const selector of[".energy-path-load-latent-badge",".energy-path-carrier-residual-badge",".energy-path-partial-coverage-badge"]){for(const badge of host.querySelectorAll(selector)){const r=badge.getBoundingClientRect();if(!r.width||!r.height)continue;const bg=background(badge),ratio=contrast(over(rgba(getComputedStyle(badge).color),bg),bg);badgeRatios.push(selector+"="+ratio.toFixed(3));check(ratio>=4.5,"small badge contrast insufficient "+theme+" "+selector+": "+ratio.toFixed(3)+" foreground="+getComputedStyle(badge).color+" background="+bg.join(","));}}
  for(const element of host.querySelectorAll("[data-energy-path-bar], [data-energy-path-ribbon]")){
   const pattern=patternFor(element),base=shapeBase(element),bg=background(host.querySelector("[data-energy-path-canvas]")),paint=over(base,bg),backgrounds=[bg],band=host.querySelector('[data-energy-path-lane-band="direct"]'),r=element.getBoundingClientRect(),b=band?.getBoundingClientRect();
   if(b&&r.left<b.right&&r.right>b.left&&r.top<b.bottom&&r.bottom>b.top)backgrounds.push(background(band));
   const ratio=Math.min(...backgrounds.map(color=>contrast(over(base,color),color)));graphicRatios.push(ratio);
   const stroke=getComputedStyle(element).stroke,strokeRatio=stroke==="none"?0:Math.min(...backgrounds.map(color=>contrast(over(rgba(stroke),color),color)));
   check(ratio>=3||(strokeRatio>=3&&parseFloat(getComputedStyle(element).strokeWidth)>=.5),"quantitative shape contrast insufficient "+theme+" "+(element.dataset.energyPathRibbon||element.dataset.energyPathBar)+": "+ratio.toFixed(3));
   if(element.dataset.energyPathPaint!=="solid"){
    check(pattern&&pattern.dataset.energyPathPattern===element.dataset.energyPathPaint,"allocated/residual shape has no matching real SVG pattern");
    const hatches=[...pattern?.querySelectorAll("path,line,polyline")||[]];check(hatches.length>0,"pattern metadata has no painted non-color hatch");
    for(const hatch of hatches){const hatchStyle=getComputedStyle(hatch),color=hatchStyle.stroke!=="none"?hatchStyle.stroke:hatchStyle.fill,hatchRatio=contrast(over(rgba(color),paint),paint);patternRatios.push(hatchRatio);check(hatchRatio>=3,"hatch disappears against semantic fill "+theme+": "+hatchRatio.toFixed(3));}
   }
  }
  const coolingLoad=nodes.find(node=>node.level==="load"&&node.serviceKind==="cooling"),heatingLoad=nodes.find(node=>node.level==="load"&&node.serviceKind==="heating");
  const coolingBar=host.querySelector('[data-energy-path-bar="'+coolingLoad.id+'"]'),heatingBar=host.querySelector('[data-energy-path-bar="'+heatingLoad.id+'"]'),cool=shapeBase(coolingBar),heat=shapeBase(heatingBar);
  check(cool[2]>cool[1]&&cool[1]>cool[0],"Cooling is not recognizably blue in "+theme+": "+cool);
  check(heat[0]>heat[1]&&heat[0]>heat[2],"Heating is not recognizably red in "+theme+": "+heat);
  check(/cooling/i.test(button(coolingLoad.id).getAttribute("aria-label"))&&/heating/i.test(button(heatingLoad.id).getAttribute("aria-label")),"Cooling/Heating distinction is color-only");
  const allocated=button(cooling.id);check(allocated?.dataset.energyPathPaint==="allocated"&&allocated.dataset.energyPathColor===button(coolingLoad.id).dataset.energyPathColor,"allocation changed service hue instead of adding a secondary encoding");
  check(allocated?.querySelector("[data-energy-path-allocation-mark]")&&/allocated/i.test(allocated.getAttribute("aria-label"))&&/allocated/i.test(allocated.title),"allocated node lacks non-color mark/full accessible explanation");
  for(const node of nodes.filter(node=>node.level==="end_use"&&["heating","lighting","equipment"].includes(node.endUse)))check(button(node.id)?.dataset.energyPathPaint==="solid"&&!button(node.id)?.querySelector("[data-energy-path-allocation-mark]"),"stored allocatedValue alone mislabeled reported energy as allocated: "+node.id);
  const residualButton=button(residual.id),residualBar=host.querySelector('[data-energy-path-bar="'+residual.id+'"]');
  check(residualButton?.dataset.energyPathPaint==="residual"&&residualBar?.dataset.energyPathPaint==="residual"&&residualButton.textContent.includes("Unclassified energy"),"qualified residual lost its gray/hatch/explicit label encoding");
  if(residualBar){const gray=shapeBase(residualBar);check(Math.max(...gray.slice(0,3))-Math.min(...gray.slice(0,3))<60,"residual is not a muted neutral color");}
  for(const bar of host.querySelectorAll("[data-energy-path-bar]")){const node=nodes.find(item=>item.id===bar.dataset.energyPathBar);if(node?.level==="driver"){const neutral=shapeBase(bar);check(Math.max(...neutral.slice(0,3))-Math.min(...neutral.slice(0,3))<65,"driver category introduced a saturated rainbow hue: "+node.driverCategory);}}
  const carriers=Object.fromEntries(nodes.filter(node=>node.level==="carrier").map(node=>[node.carrier,{key:button(node.id).dataset.energyPathColor,paint:shapeBase(host.querySelector('[data-energy-path-bar="'+node.id+'"]')).slice(0,3)}]));
  check(carriers.electricity.key!==carriers.natural_gas.key,"distinct carriers reuse an indistinguishable semantic color token");themeColors.set(theme,carriers);
  const focused=button(coolingLoad.id);focused.focus();focused.click();await settlePaint();const selected=button(coolingLoad.id),selectedBG=background(selected),selectedContrast=contrast(over(rgba(getComputedStyle(selected).color),selectedBG),selectedBG);check(selectedContrast>=4.5&&document.activeElement===selected,"selected/focused node lost contrast or keyboard focus in "+theme);
  Object.assign(state,{simulationEnergySelection:""});simulation.renderSimulation();check(geometry()===originalGeometry,"node selection changed energy geometry in "+theme);
  evidence.push(theme+" minimum text="+Math.min(...textRatios).toFixed(3)+", shape="+Math.min(...graphicRatios).toFixed(3)+", hatch="+Math.min(...patternRatios).toFixed(3));
  evidence.push(theme+" badges "+badgeRatios.join("; "));
 }
 check(JSON.stringify(themeColors.get("light").electricity.paint)!==JSON.stringify(themeColors.get("dark").electricity.paint),"carrier paint ignored the existing light/dark theme variables");
 check(!host.querySelector('input[type="color"],[data-energy-path-color-control],[data-energy-path-palette-control]'),"fixed palette added user color customization controls");
 check(host.querySelectorAll("[data-energy-path-legend-item]").length===4,"color system expanded the compact four-item legend");
 const hatchShape=kind=>[...host.querySelectorAll('[data-energy-path-pattern="'+kind+'"]')].map(pattern=>[...pattern.querySelectorAll("path,line,polyline")].map(shape=>shape.getAttribute("d")||shape.outerHTML.replace(/stroke="[^"]*"/g,"")).join("|")).sort();
 check(hatchShape("allocated").length>0&&hatchShape("residual").length>0&&JSON.stringify(hatchShape("allocated"))!==JSON.stringify(hatchShape("residual")),"allocation and residual use no distinguishable hatch geometry");
 const checkCarrierColors=contextLabel=>{for(const node of graph().nodes.filter(node=>node.level==="carrier")){const expected=themeColors.get("dark")[node.carrier],paint=shapeBase(host.querySelector('[data-energy-path-bar="'+node.id+'"]')).slice(0,3);check(button(node.id).dataset.energyPathColor===expected.key&&JSON.stringify(paint)===JSON.stringify(expected.paint),contextLabel+" changed fixed carrier palette: "+node.carrier);}};
 for(const service of["cooling","heating","all"]){change("[data-simulation-energy-service]",service);checkCarrierColors("service filter");}
 change("[data-simulation-energy-scope]","zone");change("[data-simulation-energy-zone-name]","Office");
 check(pane.scrollHeight<=pane.clientHeight+1&&pane.scrollWidth<=pane.clientWidth+1,"Zone color encoding introduced scrolling: "+pane.scrollHeight+"/"+pane.clientHeight);
 checkCarrierColors("Zone filter");
 change("[data-simulation-energy-path-period]","M1");checkCarrierColors("month filter");
 const partial=JSON.parse(fixtureJSON),office=partial.purposeResults.energyExplanation.zoneResults.find(zone=>zone.scope.zoneName==="Office");
 office.completeness={...office.completeness,status:"partial",energyUse:{status:"partial",found:6,total:8}};
 office.summary.completeness=office.completeness;const partialResult=freeze(partial),partialJSON=JSON.stringify(partialResult);
 Object.assign(state,{simulationResult:partialResult,simulationEnergyScopeKind:"zone",simulationEnergyZoneName:"Office",simulationEnergyPeriod:"annual",simulationEnergyService:"all",simulationEnergySelection:""});
 for(const theme of["light","dark"]){document.documentElement.dataset.theme=theme;simulation.renderSimulation();await settlePaint();const known=[...host.querySelectorAll(".energy-path-partial-coverage-badge")].filter(badge=>badge.getBoundingClientRect().width>0&&badge.getBoundingClientRect().height>0);check(known.length>0,"real partial Zone did not render Known only badges in "+theme);const ratios=[];for(const badge of known){const bg=background(badge),ratio=contrast(over(rgba(getComputedStyle(badge).color),bg),bg);ratios.push(ratio);check(ratio>=4.5,"Known only badge contrast insufficient "+theme+": "+ratio.toFixed(3));}evidence.push(theme+" partial Zone Known only="+Math.min(...ratios).toFixed(3));}
 check(JSON.stringify(partialResult)===partialJSON,"partial Zone badge rendering mutated raw completeness/source records");
 const shuffled=JSON.parse(fixtureJSON);shuffled.purposeResults.energyExplanation.nodes.reverse();shuffled.purposeResults.energyExplanation.links.reverse();const frozenShuffle=freeze(shuffled),shuffleJSON=JSON.stringify(frozenShuffle);
 Object.assign(state,{simulationResult:frozenShuffle,simulationEnergyScopeKind:"building",simulationEnergyPeriod:"annual",simulationEnergyService:"all",simulationEnergySelection:""});simulation.renderSimulation();
 for(const node of host.querySelectorAll("[data-energy-path-layout-node]"))check(node.dataset.energyPathColor===nodeColorKeys[node.dataset.energyPathLayoutNode],"input order changed semantic node color: "+node.dataset.energyPathLayoutNode);
 check(JSON.stringify(frozenShuffle)===shuffleJSON,"appearance mapping mutated shuffled raw source data");
 check(JSON.stringify(previousResult)===previousJSON&&JSON.stringify(result)===fixtureJSON,"appearance/selection/theme/scope mutated reported data or export input");
 document.body.dataset.epath144Status=failures.length?"failed":"passed";document.getElementById("epath144-result").textContent=(failures.length?failures.join("\n"):"passed")+"\n"+evidence.join("\n");
 }
}catch(error){document.body.dataset.epath144Status="failed";document.getElementById("epath144-result").textContent=failures.join("\n")+"\n"+error.stack;}
</script>`
