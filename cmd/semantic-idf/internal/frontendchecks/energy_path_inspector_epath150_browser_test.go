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

func TestEPATH150ActualAppCommonNodeAndLinkInspectorsBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser actual common Energy Path inspector acceptance")
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
const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
document.body.dataset.epath150Viewport=innerWidth+"x"+innerHeight;
try{
 for(let attempt=0;document.body.dataset.epath144Status!=="manual"&&attempt<200;attempt++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath144Status!=="manual")throw new Error("actual app fixture failed: "+document.getElementById("epath144-result")?.textContent);
 const [{state},simulation,view]=await Promise.all([import("/src/js/state.js"),import("/src/js/views/simulation-views.js"),import("/src/js/views/energy-path-view.js")]);
 const previous=state.simulationResult,previousJSON=JSON.stringify(previous),candidate=JSON.parse(previousJSON),explanation=candidate.purposeResults.energyExplanation;
 const privateRule="rule.private.inspector150",annualSentinel=987654;
 for(const source of explanation.sources){source.name="Reported "+source.name;source.rawValue=annualSentinel;source.effectiveValue=annualSentinel*2;source.effectiveMultiplier=2;source.multiplierApplication="requires_zone_multiplier";}
 explanation.sources.push({id:"source.private.predicted.150",name:"Predicted cooling context",sourceType:"sql_variable",driverCategory:"load.cooling",driverRole:"context",driverComponent:"load.predicted.sensible",inspectorSection:"context",zoneName:"Office",rawValue:900,effectiveValue:900,normalizedUnit:"kWh",reportingFrequency:"Monthly"});
 const allGraphs=[explanation,...explanation.periods,...explanation.zoneResults.flatMap(zone=>[zone,...zone.periods])];
 for(const payload of allGraphs){
  for(const link of payload.links)link.ruleId=privateRule;
  const wall=payload.nodes.find(item=>item.driverCategory==="surface.exterior_walls");
  Object.assign(wall,{rawValue:12,effectiveValue:24,allocatedValue:wall.value,multiplier:2,sign:"positive",thermalComponent:"sensible",allocationApplied:true,basis:"heat_balance_share",allocationExplanation:"Allocated according to same-period thermal pressure",relatedEntityIds:["surface.fixture.wall"]});
  Object.assign(payload.nodes.find(item=>item.driverCategory==="internal.people"),{rawValue:-14,effectiveValue:-28,signedValue:-28,sign:"negative"});
  payload.nodes.find(item=>item.driverCategory==="air.infiltration").relatedEntityIds=["air.fixture.mix"];
  const load=payload.nodes.find(item=>item.level==="load"&&item.serviceKind==="cooling");
  load.loadBreakdown=[{component:"sensible",value:load.value*.8,unit:"kWh",sourceIds:load.sourceIds},{component:"latent",value:load.value*.2,unit:"kWh",sourceIds:load.sourceIds}];
  load.relatedPathIds=["path.fixture.cooling"];
  const use=payload.nodes.find(item=>item.level==="end_use"&&item.endUse==="cooling");use.relatedPathIds=["path.fixture.cooling"];
  const other=payload.nodes.find(item=>item.level==="end_use"&&item.endUse==="other");
  other.groupedMembers=[{id:"member.miscellaneous",label:"Miscellaneous equipment",value:other.value*.6,unit:"kWh",sourceIds:other.sourceIds},{id:"member.storage",label:"Storage charging contribution",value:other.value*.4,unit:"kWh",sourceIds:other.sourceIds}];other.originalNodeIds=other.groupedMembers.map(item=>item.id);
 }
 // Supply values are exact selected-graph context, never additional consumption.
 for(const payload of[explanation,...explanation.periods]){
  const period=payload.id||payload.period||"annual";
  for(const[kind,annual,monthly]of[["purchased",7,1],["produced",6,2],["storage",5,3],["storage_charge",4,4]]){
   const id="support."+kind+".fixture",sourceID="source.private."+kind+".150";
   payload.nodes.push({id,level:"support",kind,endUse:kind,carrier:"electricity",label:kind,value:period==="annual"?annual:monthly,unit:"kWh",period,scaleDomain:"site",sourceIds:[sourceID]});
   if(!explanation.sources.some(source=>source.id===sourceID))explanation.sources.push({id:sourceID,name:"Reported "+kind+" electricity",sourceType:"sql_meter",reportingFrequency:"Monthly",rawValue:annualSentinel,effectiveValue:annualSentinel,normalizedUnit:"kWh"});
  }
 }
 const annualWall=explanation.nodes.find(item=>item.driverCategory==="surface.exterior_walls"),annualElectricity=explanation.nodes.find(item=>item.level==="carrier"&&item.carrier==="electricity");
 const month=explanation.periods.find(item=>item.id==="M1"),monthlyElectricity=month.nodes.find(item=>item.level==="carrier"&&item.carrier==="electricity");
 delete monthlyElectricity.rawValue;monthlyElectricity.effectiveValue=null;monthlyElectricity.allocatedValue=null;
 const monthlyGas=month.nodes.find(item=>item.level==="carrier"&&item.carrier==="natural_gas");Object.assign(monthlyGas,{rawValue:0,effectiveValue:0,allocatedValue:0});
 const seriesSource=explanation.sources.find(source=>source.id===explanation.nodes.find(item=>item.level==="load"&&item.serviceKind==="cooling").sourceIds[0]);
 candidate.series=[{file:"eplusout.sql",column:"Office:"+seriesSource.name+" [J]",sourceId:"hourly.distinct.150",name:seriesSource.name,keyValue:"Office",isMeter:false,reportingFrequency:"Hourly",points:[{x:0,value:999,label:"01-01 01:00"}]},{file:"eplusout.sql",column:"Office:"+seriesSource.name+" [J]",sourceId:seriesSource.id,name:seriesSource.name,keyValue:"Office",isMeter:false,reportingFrequency:"Monthly",points:[{x:88,value:111,label:"01-31 24:00"},{x:0,value:222,label:"02-28 24:00"}]}];
 candidate.purposeRunPlan.outputObjects=[{objectType:"Output:Variable",keyValue:"Office",variableName:seriesSource.name,reportingFrequency:"Monthly"}];
 candidate.runId="epath150-common-inspector";candidate.purposeResults.energyExplanationSummary=explanation.summary;
 const report={geometry:{surfaces:[{id:"surface.fixture.wall",name:"Office exterior wall",zoneName:"Office",surfaceType:"Wall"}],topology:{nodes:[{id:"zone.office",label:"Office"},{id:"zone.lab",label:"Laboratory"}],airCouplings:[{id:"air.fixture.mix",objectName:"Office transfer air",fromNodeId:"zone.office",toNodeId:"zone.lab"}]}},hvac:{loops:[],serviceModel:{zoneServices:[{zoneName:"Office",paths:[{id:"path.fixture.cooling",serviceKind:"cooling",zoneName:"Office",servedSubject:{kind:"zone",zoneName:"Office",name:"Office cooling service"}}]}]}}};
 const frozen=freeze(candidate),rawJSON=JSON.stringify(frozen);Object.assign(state,{report,simulationResult:frozen,simulationEnergyScopeKind:"building",simulationEnergyZoneName:"",simulationEnergyPeriod:"annual",simulationEnergyService:"all",simulationEnergySelection:"",simulationEnergyDetailsOpen:false});simulation.renderSimulation();
 const host=document.getElementById("simulationEnergyDashboard"),pane=document.querySelector("#simulationPane > .simulation-pane");
 const graph=()=>view.energyPathGraphForState(explanation,state);
 const wallID=()=>graph().nodes.find(item=>item.driverCategory==="surface.exterior_walls").id;
 const button=id=>[...host.querySelectorAll("[data-energy-path-layout-node]")].find(element=>element.dataset.energyPathLayoutNode===id);
 const edge=id=>[...host.querySelectorAll("[data-energy-explanation-edge]")].find(element=>element.dataset.energyExplanationEdge===id&&element.tabIndex>=0);
 const inspector=()=>host.querySelector("[data-energy-path-inspector],[data-energy-path-link-inspector]");
 const section=key=>inspector()?.querySelector('[data-energy-path-detail-section="'+key+'"]');
 const value=key=>inspector()?.querySelector('[data-energy-path-inspector-value="'+key+'"] dd')?.textContent.trim();
 const click=element=>element?.dispatchEvent(new MouseEvent("click",{bubbles:true,cancelable:true}));
 const select=id=>{click(button(id)||edge(id));check(state.simulationEnergySelection===id,"actual selection handler failed "+id);return inspector();};
 const change=(selector,value)=>{const control=host.querySelector(selector);check(Boolean(control),"missing real control "+selector);if(control){control.value=value;control.dispatchEvent(new Event("change",{bubbles:true}));}};
 const escape=element=>{element.focus();element.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true}));};
 const visibleText=element=>element?.innerText||"";
 const sourceTokens=[...explanation.sources.map(source=>source.id),privateRule];
 const checkStructure=(item,label)=>{
  const panel=inspector();check(Boolean(panel),label+" inspector missing");if(!panel)return;
  const sections=[...panel.querySelectorAll("[data-energy-path-detail-section]")];
  check(sections.map(element=>element.dataset.energyPathDetailSection).join(",")==="represents,value,breakdown,basis,entities,sources,actions",label+" common sections missing/reordered/duplicated");
  check(sections.every(element=>element.parentElement===sections[0]?.parentElement),label+" common sections are not consistent siblings");
  const sources=section("sources");check(sources?.tagName==="DETAILS"&&sources.open===false,label+" Sources is not closed native disclosure");
  const text=visibleText(panel);for(const token of sourceTokens)check(!text.includes(token),label+" leaks source/rule ID before Sources opens: "+token);
  check(!visibleText(section("basis")).includes("heat_balance_share")&&!visibleText(section("basis")).includes("service_path_allocation"),label+" calculation basis exposes raw rule token instead of plain-language explanation");
  check(!text.includes(String(annualSentinel)),label+" annual source scalar leaked into main selected-period values");
  check(pane.scrollWidth<=pane.clientWidth+1,label+" inspector introduces horizontal overflow: pane="+pane.scrollWidth+"/"+pane.clientWidth+" inspector="+panel.scrollWidth+"/"+panel.clientWidth);
  if(pane.scrollWidth>pane.clientWidth+1){const right=pane.getBoundingClientRect().right;evidence.push(label+" overflow "+[...pane.querySelectorAll("*")].map(element=>({element,r:element.getBoundingClientRect()})).filter(({element,r})=>r.width&&r.height&&(r.right>right+2||element.scrollWidth>element.clientWidth+2)).sort((a,b)=>(b.element.scrollWidth-b.element.clientWidth)-(a.element.scrollWidth-a.element.clientWidth)).slice(0,8).map(({element,r})=>element.tagName+"."+element.getAttribute("class")+":"+r.right.toFixed(1)+" scroll"+element.scrollWidth+"/"+element.clientWidth+" ws="+getComputedStyle(element).whiteSpace).join(";"));}
  click(sources?.querySelector("summary"));check(sources?.open===true,label+" native Source disclosure did not open");
  check(pane.scrollWidth<=pane.clientWidth+1,label+" opened Sources introduces horizontal overflow: "+pane.scrollWidth+"/"+pane.clientWidth);
  const exact=(item.sourceIds||[]).find(id=>explanation.sources.some(source=>source.id===id));if(exact)check(visibleText(sources).includes(exact),label+" exact source ID unavailable after opening Sources");
  if(item.ruleId)check(visibleText(sources).includes(item.ruleId),label+" exact rule ID unavailable in opened Sources");
 };
 if(new URLSearchParams(location.search).get("run150")!=="1"){
  const mode=new URLSearchParams(location.search).get("inspector");if(mode==="link")select(graph().links.find(link=>link.relation==="load_to_end_use"&&link.serviceKind==="cooling").id);else select(wallID());
  inspector()?.scrollIntoView({block:"start"});document.body.dataset.epath150Status="manual";document.getElementById("epath150-result").textContent="Actual common inspector; all seven sections; Source data closed. Scope/Period/Service and node/link navigation remain live.";
 }else{
  check(innerWidth===1600&&innerHeight===900,"actual150viewport is not1600x900");
  const originalGraph=graph(),nodeIDs=originalGraph.nodes.filter(item=>button(item.id)).map(item=>item.id),originalPaths=[...host.querySelectorAll("[data-energy-path-ribbon]")].map(path=>[path.dataset.energyPathRibbon,path.getAttribute("d")]);
  for(const id of nodeIDs){const item=originalGraph.nodes.find(node=>node.id===id);select(id);checkStructure(item,item.level+":"+id);}
  for(const relation of["driver_to_load","load_to_end_use","end_use_to_carrier","residual"]){const item=graph().links.find(link=>link.relation===relation&&edge(link.id));select(item.id);checkStructure(item,relation);}
  select(wallID());
  check(value("raw")?.includes("12")&&value("effective")?.includes("24")&&value("allocated")?.includes(String(annualWall.value)),"Driver raw/effective/allocated values were conflated: "+JSON.stringify([value("raw"),value("effective"),value("allocated")]));
  check(value("raw")?.includes("kWh thermal")&&value("allocated")?.includes("kWh thermal"),"thermal Driver inspector values mislabeled as site energy");
  check(visibleText(section("entities")).includes("Office exterior wall")&&!visibleText(section("entities")).includes("surface.fixture.wall"),"related model entity did not use actual report's friendly surface name");
  state.semanticProjection={navigation:{entities:[{id:"surface.fixture.wall",kind:"surface",label:"Canonical office envelope"}]}};simulation.renderSimulationEnergyDashboard(frozen);
  check(visibleText(section("entities")).includes("Canonical office envelope")&&!visibleText(section("entities")).includes("Office exterior wall"),"exact semantic navigation entity label did not outrank fallback report name");state.semanticProjection=null;simulation.renderSimulationEnergyDashboard(frozen);
  check(visibleText(section("breakdown")).includes("Office"),"Driver top zones were not derived from matching nested Zone nodes");
  check(section("breakdown")?.querySelector('[data-energy-path-detail-breakdown="componentRows"] [data-energy-path-detail-row="sensible"] dd')?.textContent.includes(String(annualWall.value)),"Driver sensible component lost actual member contribution");
  const heatingDriver=graph().nodes.find(item=>item.driverCategory==="internal.people");select(heatingDriver.id);check(/[−-]14/.test(value("raw"))&&/[−-]28/.test(value("effective"))&&value("allocated")?.includes(String(heatingDriver.value)),"signed heating pressure was replaced by allocated contribution/magnitude");
  const load=graph().nodes.find(item=>item.level==="load"&&item.serviceKind==="cooling");select(load.id);
  check(visibleText(section("breakdown")).includes("32")&&visibleText(section("breakdown")).includes("8"),"Load sensible/latent breakdown lost exact selected values");
  check(section("breakdown")?.querySelector('[data-energy-path-detail-breakdown="contextRows"] dd')?.textContent.includes("900"),"reported annual predicted context is not inspectable separately from delivered load");
  check(/predicted/i.test(section("breakdown")?.querySelector('[data-energy-path-detail-breakdown="contextRows"] dt')?.textContent||""),"annual prediction is displayed as an unnamed generic context value");
  check(section("actions")?.querySelector('[data-energy-path-hvac-path-id="path.fixture.cooling"]'),"existing exact HVAC navigation action was lost/moved outside common Actions");
  const conversion=graph().links.find(item=>item.relation==="load_to_end_use"&&item.serviceKind==="cooling");select(conversion.id);
  for(const[field,number]of[["from",40],["to",10],["ratio",4]])check(inspector()?.querySelector('[data-energy-path-link-value="'+field+'"]')?.textContent.includes(String(number)),"conversion missing exact selected "+field);
  select(annualElectricity.id);const breakdown=visibleText(section("breakdown"));
  check(breakdown.includes("68")&&breakdown.includes("3"),"Carrier facility total / residual missing from Breakdown");
  check(["Purchased","production","Storage"].every(text=>breakdown.toLowerCase().includes(text.toLowerCase())),"Carrier purchased/produced/storage context missing from Breakdown");
  const other=graph().nodes.find(item=>item.level==="end_use"&&item.endUse==="other");select(other.id);const memberDetails=section("breakdown")?.querySelector("[data-energy-path-group-members]");
  check(memberDetails?.tagName==="DETAILS","Other original contributions lost native Breakdown Expand");click(memberDetails?.querySelector("summary"));check(memberDetails?.open&&visibleText(memberDetails).includes("Miscellaneous equipment")&&visibleText(memberDetails).includes("Storage charging contribution"),"Other Expand lost readable original members");
  check(host.querySelectorAll("[data-energy-path-layout-node]").length===nodeIDs.length,"Other Expand altered quantitative graph node count");
  change("[data-simulation-energy-path-period]","M1");select(monthlyElectricity.id);
  for(const key of["raw","effective","allocated"])check(value(key)==="—", "missing/null monthly "+key+" fabricated zero or annual source value: "+value(key));
  const monthlySupply=section("breakdown")?.querySelector('[data-energy-path-supply-kind="purchased"]');
  check(monthlySupply?.dataset.energyPathSupplyValue==="1","monthly purchased energy missing or reused annual7");
  select(monthlyGas.id);for(const key of["raw","effective","allocated"])check(value(key)?.startsWith("0"),"reported explicit monthly zero was erased as missing: "+key);
  select(wallID());const monthZoneExpected=explanation.zoneResults[0].periods.find(item=>item.id==="M1").nodes.find(item=>item.driverCategory==="surface.exterior_walls").value;
  check(section("breakdown")?.querySelector('[data-energy-path-detail-breakdown="zoneRows"] [data-energy-path-detail-row="Office"] dd')?.textContent.includes(monthZoneExpected.toLocaleString(undefined,{maximumFractionDigits:2})),"monthly Driver top-zone contribution reused annual Zone scalar");
  const monthLoad=graph().nodes.find(item=>item.level==="load"&&item.serviceKind==="cooling");select(monthLoad.id);
  check(section("breakdown")?.querySelector('[data-energy-path-detail-breakdown="contextRows"] dd')?.textContent.trim()==="—"&&!visibleText(section("breakdown")).includes("900"),"monthly predicted context fabricated annual source value");
  check(/predicted/i.test(section("breakdown")?.querySelector('[data-energy-path-detail-breakdown="contextRows"] dt')?.textContent||""),"missing monthly prediction lost its explanatory context label");
  const monthLink=graph().links.find(item=>item.relation==="load_to_end_use"&&item.serviceKind==="cooling");select(monthLink.id);check(inspector()?.querySelector('[data-energy-path-link-value="from"]')?.textContent.includes(String(monthLink.fromValue)),"monthly link detail reused annual thermal value");
  const linkSeries=section("actions")?.querySelector("[data-energy-path-series-id]");check(linkSeries?.dataset.energyPathSeriesId?.endsWith("::"+seriesSource.id),"selected link lost its exact Monthly source Series action");click(linkSeries);
  check(state.simulationActiveResultView==="series"&&state.simulationSeriesRangeStart===0&&state.simulationSeriesRangeEnd===0&&state.simulationSelectedSeries?.endsWith("::"+seriesSource.id),"link Series action did not preserve exact source identity / January label range");
  check(document.getElementById("simulationChart").querySelector("[data-simulation-series-single-point]")?.nextElementSibling?.textContent.includes("111 J"),"link Series navigation did not render the actual January source value");
  click(document.querySelector('[data-simulation-result-view-button="energy"]'));check(state.simulationEnergySelection===monthLink.id&&state.simulationEnergyPeriod==="M1"&&inspector(),"return from link Series lost exact Energy context");
  const focused=edge(monthLink.id);escape(focused);check(state.simulationEnergySelection===""&&!inspector()&&document.activeElement?.dataset.energyExplanationEdge===monthLink.id,"common link inspector broke Escape selection clear / logical focus restore");
  change("[data-simulation-energy-scope]","zone");change("[data-simulation-energy-zone-name]","Office");const zoneLoad=graph().nodes.find(item=>item.level==="load"&&item.serviceKind==="cooling");select(zoneLoad.id);checkStructure(zoneLoad,"ZoneM1load");
  check(visibleText(section("value")).includes(String(zoneLoad.value)),"Zone selected-period load value missing");
  const zoneAir=graph().nodes.find(item=>item.driverCategory==="air.infiltration");select(zoneAir.id);check(visibleText(section("entities")).includes("Office transfer air")&&!visibleText(section("entities")).includes("air.fixture.mix"),"exact topology coupling objectName was not used for related-entity label");select(zoneLoad.id);
  escape(button(zoneLoad.id));check(state.simulationEnergySelection===""&&document.activeElement?.dataset.energyPathLayoutNode===zoneLoad.id,"common node inspector broke Escape focus restore");
  change("[data-simulation-energy-scope]","building");change("[data-simulation-energy-path-period]","annual");
  check(JSON.stringify([...host.querySelectorAll("[data-energy-path-ribbon]")].map(path=>[path.dataset.energyPathRibbon,path.getAttribute("d")]))===JSON.stringify(originalPaths),"inspector disclosure/navigation changed quantitative ribbon geometry");
  check(JSON.stringify(frozen)===rawJSON&&JSON.stringify(previous)===previousJSON,"common inspector mutated raw payload/export inputs");
  evidence.push(nodeIDs.length+" primary nodes + 4 link relations; seven consistent sections; exact source IDs native-collapsed; period-local values and unchanged raw graph");
  document.body.dataset.epath150Status=failures.length?"failed":"passed";document.getElementById("epath150-result").textContent=(failures.length?failures.join("\n"):"passed")+"\n"+evidence.join("\n");
 }
}catch(error){document.body.dataset.epath150Status="failed";document.getElementById("epath150-result").textContent=failures.join("\n")+"\n"+error.stack;}
</script>`
