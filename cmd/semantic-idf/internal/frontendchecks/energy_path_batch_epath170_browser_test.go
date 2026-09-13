package frontendchecks

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestEPATH170ActualToolsAnnualSummaryComparisonBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser actual Tools annual Batch comparison acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	page := readTestFile(t, "frontend/src/tools.html")
	marker := `<script type="module" src="./js/tools.js"></script>`
	if !strings.Contains(page, marker) {
		t.Fatal("actual Tools bootstrap changed")
	}
	page = strings.Replace(page, marker, epath170BatchSetupHTML+marker+epath170BatchAssertionsHTML, 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/epath170-batch.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	width, height := 1600, 900
	run := func(action, query string) ([]byte, error) {
		return exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--force-device-scale-factor=1", fmt.Sprintf("--window-size=%d,%d", width, height), "--virtual-time-budget=16000", "--user-data-dir="+t.TempDir(), action, server.URL+"/src/epath170-batch.html"+query+"#batch-simulation").CombinedOutput()
	}
	output, err := run("--dump-dom", "")
	if viewport := regexp.MustCompile(`data-epath170-viewport="(\d+)x(\d+)"`).FindSubmatch(output); err == nil && len(viewport) == 3 {
		actualWidth, _ := strconv.Atoi(string(viewport[1]))
		actualHeight, _ := strconv.Atoi(string(viewport[2]))
		if actualWidth != 1600 || actualHeight != 900 {
			if actualWidth < 1200 || actualWidth > 1800 || actualHeight < 600 || actualHeight > 1000 {
				t.Fatalf("unexpected headless viewport %dx%d", actualWidth, actualHeight)
			}
			width += 1600 - actualWidth
			height += 900 - actualHeight
			output, err = run("--dump-dom", "")
		}
	}
	if err != nil {
		t.Fatalf("EPATH170 browser: %v\n%s", err, output)
	}
	comfort, comfortErr := run("--dump-dom", "?scenario=comfort")
	if comfortErr != nil || !strings.Contains(string(comfort), `data-epath170-status="passed"`) {
		diagnostic := regexp.MustCompile(`(?s)<pre id="epath170-result"[^>]*>(.*?)</pre>`).FindSubmatch(comfort)
		if len(diagnostic) == 2 {
			t.Fatalf("EPATH170 Comfort-only route: %v\n%s", comfortErr, diagnostic[1])
		}
		t.Fatalf("EPATH170 Comfort-only route: %v\n%s", comfortErr, comfort)
	}
	for _, mode := range []string{"compare", "detail"} {
		if path := os.Getenv("EPATH170_SCREENSHOT_" + strings.ToUpper(mode)); path != "" {
			epath170CaptureFreshViewport(t, ctx, chrome, server.URL+"/src/epath170-batch.html?manual=1&mode="+mode+"#batch-simulation", mode, path)
		}
	}
	diagnostic := regexp.MustCompile(`(?s)<pre id="epath170-result"[^>]*>(.*?)</pre>`).FindStringSubmatch(string(output))
	if !strings.Contains(string(output), `data-epath170-status="passed"`) {
		if len(diagnostic) == 2 {
			t.Fatalf("EPATH170 failed: %s", diagnostic[1])
		}
		t.Fatalf("EPATH170 failed:\n%s", output)
	}
	if len(diagnostic) == 2 {
		t.Log(diagnostic[1])
	}
}

// Optional visual QA owns an ephemeral headless process/profile, never the user's
// browser. Set viewport before navigation: Chrome's CLI screenshot mode resizes
// after script execution and miscaptures a scrolled root-document Tools page.
func epath170CaptureFreshViewport(t *testing.T, ctx context.Context, chrome, url, mode, outputPath string) {
	t.Helper()
	profile := t.TempDir()
	command := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--remote-debugging-port=0", "--force-device-scale-factor=1", "--user-data-dir="+profile, "about:blank")
	var processOutput bytes.Buffer
	command.Stdout, command.Stderr = &processOutput, &processOutput
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	var endpoint string
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort")); err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) >= 2 {
				endpoint = "http://127.0.0.1:" + strings.TrimSpace(lines[0])
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if endpoint == "" {
		t.Fatalf("fresh screenshot browser did not start: %s", processOutput.String())
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Get(endpoint + "/json/list")
	if err != nil {
		t.Fatal(err)
	}
	var targets []struct {
		Type         string `json:"type"`
		WebSocketURL string `json:"webSocketDebuggerUrl"`
	}
	err = json.NewDecoder(response.Body).Decode(&targets)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	var socketURL string
	for _, target := range targets {
		if target.Type == "page" {
			socketURL = target.WebSocketURL
			break
		}
	}
	if socketURL == "" {
		t.Fatal("fresh screenshot page target missing")
	}
	connection, _, err := websocket.DefaultDialer.DialContext(ctx, socketURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	sequence := 0
	call := func(method string, params any, result any) {
		t.Helper()
		sequence++
		if err := connection.WriteJSON(map[string]any{"id": sequence, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		_ = connection.SetReadDeadline(time.Now().Add(15 * time.Second))
		for {
			var message struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if err := connection.ReadJSON(&message); err != nil {
				t.Fatalf("%s: %v", method, err)
			}
			if message.ID != sequence {
				continue
			}
			if len(message.Error) > 0 {
				t.Fatalf("%s: %s", method, message.Error)
			}
			if result != nil {
				if err := json.Unmarshal(message.Result, result); err != nil {
					t.Fatal(err)
				}
			}
			return
		}
	}
	call("Page.enable", map[string]any{}, nil)
	call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 1600, "height": 900, "deviceScaleFactor": 1, "mobile": false}, nil)
	call("Page.navigate", map[string]any{"url": url}, nil)
	evaluate := func(expression string) json.RawMessage {
		var result struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
			Exception json.RawMessage `json:"exceptionDetails"`
		}
		call("Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true}, &result)
		if len(result.Exception) > 0 {
			t.Fatalf("screenshot fixture: %s", result.Exception)
		}
		return result.Result.Value
	}
	ready := false
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
		status := string(evaluate(`document.body?.dataset.epath170Status || "pending"`))
		if status == `"manual"` {
			ready = true
			break
		}
		if status == `"failed"` {
			t.Fatalf("screenshot fixture failed: %s", evaluate(`document.getElementById("epath170-result")?.textContent`))
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("%s screenshot fixture never became ready", mode)
	}
	proof := evaluate(`new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>{const detail=new URLSearchParams(location.search).get("mode")==="detail";const host=document.querySelector(detail?"[data-batch-energy-detail]":"[data-batch-energy-comparison]");const intersects=element=>{if(!element)return false;const r=element.getBoundingClientRect();return r.width>0&&r.height>0&&r.bottom>0&&r.top<innerHeight&&r.right>0&&r.left<innerWidth;};resolve({ready:innerWidth===1600&&innerHeight===900&&intersects(host)&&intersects(host?.querySelector(detail?".batch-energy-detail-header":"header"))&&intersects(host?.querySelector(detail?"[data-energy-path-canvas]":"table")),width:innerWidth,height:innerHeight,top:host?.getBoundingClientRect().top,scrollY,canvases:document.querySelectorAll("[data-energy-path-canvas]").length});})))`)
	var captureProof struct {
		Ready bool `json:"ready"`
	}
	if err := json.Unmarshal(proof, &captureProof); err != nil || !captureProof.Ready {
		t.Fatalf("%s screenshot target not visible at native1600x900: %s", mode, proof)
	}
	var screenshot struct {
		Data string `json:"data"`
	}
	call("Page.captureScreenshot", map[string]any{"format": "png", "fromSurface": true, "captureBeyondViewport": false}, &screenshot)
	data, err := base64.StdEncoding.DecodeString(screenshot.Data)
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || configuration.Width != 1600 || configuration.Height != 900 {
		t.Fatalf("invalid screenshot dimensions: %+v, %v", configuration, err)
	}
	if err := os.WriteFile(outputPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s native screenshot ready: %s", mode, proof)
}

const epath170BatchSetupHTML = `<script>
(()=>{
 const freeze=value=>{if(value&&typeof value==="object"){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
 const level=(status="complete",found=2,total=2)=>({status,found,total,percent:total?100*found/total:0});
 const quality=target=>({drivers:level(),loads:level(),endUses:target?level("partial",1,2):level(),carriers:level(),ratios:level(),driverToLoadClosedPct:100,endUseToCarrierClosedPct:100,driverToLoadStatus:"complete",endUseToCarrierStatus:"complete",zoneAllocationStatus:"unavailable",zoneAllocatedPct:0,unassignedPct:0});
 const makeRun=(index,target=false)=>{
  const runId="private170-run-"+String(index).padStart(3,"0"),scope={kind:"building",aggregationBasis:"model_total"},nodes=[],links=[],sources=[],sourceId="SOURCE_PRIVATE_170_"+index;
  const add=(key,level,value,extra={})=>{const node={id:"NODE_PRIVATE_170_"+index+"_"+key,kind:key,level,label:key,value,rawValue:value,effectiveValue:value,allocatedValue:value,displayValue:value,unit:"kWh",period:"annual",aggregationBasis:"model_total",sourceIds:[sourceId],...extra};nodes.push(node);return node;};
  const walls=add("driver.surface.exterior_walls","driver",target?40:50,{label:"Exterior walls",driverCategory:"surface.exterior_walls",serviceKind:"cooling",scaleDomain:"thermal",basis:"heat_balance_share",allocationApplied:true});
  const people=add("driver.internal.people","driver",target?40:50,{label:"People",driverCategory:"internal.people",serviceKind:"cooling",scaleDomain:"thermal",basis:"heat_balance_share",allocationApplied:true});
  const heatingDriver=add("driver.balance.storage_other","driver",80,{label:"Other / Storage",driverCategory:"balance.storage_other",serviceKind:"heating",scaleDomain:"thermal",basis:"heat_balance_share",allocationApplied:true});
  const cooling=add("load.cooling","load",target?80:100,{label:"Cooling load",serviceKind:"cooling",scaleDomain:"thermal",basis:"reported_variable"}),heating=add("load.heating","load",80,{label:"Heating load",serviceKind:"heating",scaleDomain:"thermal",basis:"reported_variable"});
  const coolUse=add("end_use.cooling","end_use",target?20:25,{label:"Cooling equipment",endUse:"cooling",serviceKind:"cooling",scaleDomain:"site",basis:target?"service_path_allocation":"reported_meter"}),heatUse=add("end_use.heating","end_use",100,{label:"Heating equipment",endUse:"heating",serviceKind:"heating",scaleDomain:"site",basis:"reported_meter"});
  const lighting=add("end_use.lighting","end_use",target?12:10,{label:"Lighting",endUse:"lighting",scaleDomain:"site",basis:"reported_meter"});
  const equipment=target?null:add("end_use.equipment","end_use",3,{label:"Equipment",endUse:"equipment",scaleDomain:"site",basis:"reported_meter"});
  const water=target?null:add("end_use.water_systems","end_use",2,{label:"Water systems",endUse:"water_systems",scaleDomain:"site",basis:"reported_meter"});
  const refrigeration=target?add("end_use.refrigeration","end_use",5,{label:"Refrigeration",endUse:"refrigeration",scaleDomain:"site",basis:"reported_meter"}):null;
  const electricity=add("carrier.electricity","carrier",target?37:40,{label:"Electricity",carrier:"electricity",scaleDomain:"site",basis:"reported_meter",meterHierarchyLevel:"facility_total"}),gas=add("carrier.natural_gas","carrier",100,{label:"Natural gas",carrier:"natural_gas",scaleDomain:"site",basis:"reported_meter",meterHierarchyLevel:"facility_total"});
  const link=(from,to,relation,extra={})=>{const item={id:"EDGE_PRIVATE_170_"+index+"_"+links.length,fromId:from.id,toId:to.id,relation,period:"annual",serviceKind:from.serviceKind||to.serviceKind||"",domain:relation==="driver_to_load"?"thermal":"site",basis:relation==="driver_to_load"?"heat_balance_share":"reported_meter",fromValue:from.value,toValue:relation==="driver_to_load"?from.value:relation==="end_use_to_carrier"?from.value:to.value,fromUnit:"kWh",toUnit:"kWh",unit:"kWh",sourceIds:[sourceId],...extra};links.push(item);return item;};
  link(walls,cooling,"driver_to_load");link(people,cooling,"driver_to_load");link(heatingDriver,heating,"driver_to_load");
  link(cooling,coolUse,"load_to_end_use",{ratio:4,ratioKind:"coefficient_of_performance"});link(heating,heatUse,"load_to_end_use",{ratio:.8,ratioKind:"efficiency"});
  for(const node of[coolUse,lighting,equipment,water,refrigeration].filter(Boolean))link(node,electricity,"end_use_to_carrier");link(heatUse,gas,"end_use_to_carrier");
  sources.push({id:sourceId,sourceType:"sql_meter",isMeter:true,name:"Cooling:Electricity",reportingFrequency:"Monthly",sourceUnit:"J",normalizedUnit:"kWh"});
  // Summary fields match EnergyExplanationSummaryItem; node-only driverCategory
  // and effectiveValue are deliberately absent. IDs differ across every run.
  const item=node=>({id:node.id,level:node.level,kind:node.kind,label:node.label,value:node.value,rawValue:node.rawValue,allocatedValue:node.allocatedValue,unit:"kWh",scaleDomain:node.scaleDomain,aggregationBasis:"model_total",serviceKind:node.serviceKind||"",endUse:node.endUse||"",carrier:node.carrier||"",basis:node.basis,sourceIds:node.sourceIds});
  const summary={schema:"semantic-idf.energy-explanation-summary/v2",period:"annual",scope,drivers:nodes.filter(n=>n.level==="driver").map(item),loads:[cooling,heating].map(item),endUses:nodes.filter(n=>n.level==="end_use").map(item),carriers:[electricity,gas].map(item),ratios:[{id:"RATIO_PRIVATE_170_"+index,level:"ratio",kind:"kpi.cooling_cop",label:"Cooling COP",value:4,serviceKind:"cooling",basis:"derived_ratio",numeratorValue:cooling.value,numeratorUnit:"kWh",denominatorValue:coolUse.value,denominatorUnit:"kWh",sourceIds:[sourceId]}],residuals:[{id:"RESIDUAL_PRIVATE_170_"+index,level:"residual",kind:"residual.site_electricity",label:"Unclassified energy",value:0,unit:"kWh",scaleDomain:"site",carrier:"electricity",basis:"residual",sourceIds:[sourceId]}],topZones:[{id:"ZONE_PRIVATE_170",level:"zone",kind:"zone.driver",label:"Unrelated Zone detail must not be a Batch group",value:999999,unit:"kWh"}],quality:quality(target),completeness:{mappedPercent:100,status:"complete"}};
  if(!target)summary.endUses.push({id:"ZERO_PRIVATE_170",level:"end_use",kind:"end_use.refrigeration",label:"Refrigeration",endUse:"refrigeration",value:0,unit:"kWh",scaleDomain:"site",basis:"reported_meter",sourceIds:[sourceId]});
  else summary.endUses.push({id:"NULL_PRIVATE_170",level:"end_use",kind:"end_use.water_systems",label:"Water systems",endUse:"water_systems",value:null,unit:"kWh",scaleDomain:"site",basis:"reported_meter",sourceIds:[sourceId]});
  const explanation={schema:"semantic-idf.energy-explanation/v2",scope,nodes,links,sources,quality:summary.quality,completeness:summary.completeness,summary,zoneResults:[],periods:[]};
  return {runId,status:"succeeded",filename:index===0?"A-baseline.idf":index===1?"B-target.idf":"Model-"+String(index).padStart(3,"0")+".idf",inputPath:"C:/Batch170/model-"+index+".idf",outputDirectory:"C:/Batch170/results/"+index,durationMs:1200,err:{warnings:0,severe:0,fatal:0},series:[],csvs:[],purposeMetrics:[{id:"energy_explanation.energy_use",label:"Site energy",value:electricity.value+100,unit:"kWh",status:"ok"}],purposeResults:{energyExplanationSummary:summary,energyExplanation:explanation},purposeRunPlan:{outputObjects:[{objectType:"Output:Meter",keyValue:"Cooling:Electricity",reportingFrequency:"Monthly"}]}};
 };
 const results=Array.from({length:100},(_,index)=>makeRun(index,index!==0));
 const units=results[2].purposeResults.energyExplanationSummary;units.carriers.find(item=>item.carrier==="electricity").unit="MJ";units.ratios[0].unit="kWh";
 results[3].purposeResults.energyExplanationSummary.scope={kind:"zone",zoneName:"Office"};results[4].purposeResults.energyExplanationSummary.period="M1";
 results[5].purposeResults.energyExplanationSummary.schema="semantic-idf.energy-explanation-summary/v1";
 results[6].purposeResults={energyExplanationSummary:{schema:"semantic-idf.energy-explanation-summary/v2",scope:{kind:"building"},period:"annual",quality:{drivers:level("not_requested",0,0),loads:level("not_requested",0,0),endUses:level("not_requested",0,0),carriers:level("not_requested",0,0),ratios:level("not_requested",0,0),driverToLoadStatus:"unavailable",endUseToCarrierStatus:"unavailable",driverToLoadClosedPct:0,endUseToCarrierClosedPct:0}}};
 const duplicate=results[7].purposeResults.energyExplanationSummary.drivers[0];results[7].purposeResults.energyExplanationSummary.drivers.push({...duplicate,id:"DUPLICATE_PRIVATE_170",value:12});
 results[99].runId=results[98].runId;
 const comfort=new URLSearchParams(location.search).get("scenario")==="comfort";
 const comfortResults=[0,1].map(index=>({runId:"comfort-run-"+index,status:"succeeded",filename:"Comfort-"+index+".idf",inputPath:"C:/Batch170/comfort-"+index+".idf",series:[],csvs:[],err:{warnings:0,severe:0,fatal:0},purposeResults:{energyExplanationSummary:{},energyExplanation:{}},purposeMetrics:[{id:"comfort.uncomfortable_hours",purpose:"comfort",label:"Uncomfortable hours",value:index?4:0,unit:"h",status:"ok"}]}));
 const chosenResults=comfort?comfortResults:results;
 const payload=freeze({runId:"batch-private170",total:chosenResults.length,completed:chosenResults.length,succeeded:chosenResults.length,failed:0,workers:4,results:chosenResults});
 const mainWorkspace={schemaVersion:4,text:"Version,25.1;",analysisKey:"main-analysis",activeResultTab:"simulation",simulationResultRef:{textHash:"a".repeat(64),runId:"main-run"},viewSnapshot:{panelContexts:{simulation:{energyScopeKind:"zone",energyZoneName:"Main Office",energyPeriod:"M2",energyService:"heating",energySelection:"main-selected",energyDetailsOpen:true,energyDrawer:{tab:"data",stage:"drivers",outputSource:""}}}}};
 sessionStorage.setItem("idfAnalyzer.currentDocument",JSON.stringify(mainWorkspace));
 const calls={select:0,run:0,forbidden:0};
 window.go={main:{App:new Proxy({GetSettings:async()=>({settings:{appearance:{language:"en",theme:"light"}}}),GetAppInfo:async()=>({name:"SemanticIDF",version:"170"}),GetSimulationEnvironment:async()=>({installations:[{version:"25.1",executablePath:"C:/fixtures/energyplus.exe"}],weatherFolders:[],defaultWorkerCount:4}),SelectSimulationInputFiles:async()=>{calls.select++;return{paths:payload.results.map(item=>item.inputPath)};},RunMultipleSimulations:async()=>{calls.run++;return payload;}},{get(target,key){if(Object.hasOwn(target,key))return target[key];if(/^(Analyze|Run|ScanCleanup)/.test(String(key)))return async()=>{calls.forbidden++;throw new Error("Unexpected work "+String(key));};return target[key];}})}};
 window.runtime={EventsOn(){return()=>{};},EventsOnMultiple(){return()=>{};}};
 window.epath170={payload,comfort,rawJSON:JSON.stringify(payload),mainJSON:JSON.stringify(mainWorkspace),calls};
})();
</script>`

const epath170BatchAssertionsHTML = `<pre id="epath170-result" hidden>pending</pre><script type="module">
const failures=[],evidence=[],fixture=window.epath170;
document.body.dataset.epath170Viewport=innerWidth+"x"+innerHeight;
const check=(value,message)=>{if(!value)failures.push(message);};
const sleep=ms=>new Promise(resolve=>setTimeout(resolve,ms));
const wait=async(predicate,label)=>{for(let attempt=0;attempt<250;attempt++){if(predicate())return;await sleep(20);}throw new Error("Timed out: "+label);};
const visible=element=>Boolean(element&&element.getBoundingClientRect().width&&element.getBoundingClientRect().height);
const inViewport=element=>{if(!visible(element))return false;const rect=element.getBoundingClientRect();return rect.bottom>0&&rect.top<innerHeight&&rect.right>0&&rect.left<innerWidth;};
try{
 await wait(()=>document.querySelector('[data-tools-tab="batch-simulation"]')?.classList.contains("active"),"actual Tools Batch tab");
 document.getElementById("multiSimulationSelectFiles").click();await wait(()=>!document.getElementById("multiSimulationRun").disabled,"actual file selection");document.getElementById("multiSimulationRun").click();
 if(fixture.comfort){
  await wait(()=>document.querySelectorAll("#multiSimulationTable [data-multi-sim-row]").length===2&&!document.getElementById("multiSimulationRun").disabled,"completed Comfort-only intake");
  const chart=document.getElementById("multiSimulationChart");check(chart.querySelectorAll(".batch-purpose-bar-row").length===2&&!chart.querySelector("[data-batch-energy-summary-table],[data-batch-energy-comparison]"),"empty energy structs diverted Comfort results into unavailable Energy comparison");check([...chart.querySelectorAll(".batch-purpose-bar-row strong")].map(element=>element.textContent.trim()).join("|")==="0|4","Comfort purpose chart lost explicit0/4 result values");check(fixture.calls.select===1&&fixture.calls.run===1&&fixture.calls.forbidden===0,"Comfort-only route performed extra analysis/run");check(JSON.stringify(fixture.payload)===fixture.rawJSON&&sessionStorage.getItem("idfAnalyzer.currentDocument")===fixture.mainJSON,"Comfort-only route mutated result/main workspace");if(failures.length)throw new Error(failures.join("\n"));document.body.dataset.epath170Status="passed";document.getElementById("epath170-result").textContent="Separate cold Tools Comfort-only intake retained actual purpose chart, zero values, and no Energy fallback or extra work.";
 }else{
 await wait(()=>document.querySelector("[data-batch-energy-summary-table]"),"new annual summary comparison");
 const [{state:mainState},compare]=await Promise.all([import("/src/js/state.js"),import("/src/js/energy-path-batch-comparison.js")]);
 Object.assign(mainState,{simulationResult:{runId:"main-state-sentinel"},report:{label:"main report sentinel"},simulationEnergyScopeKind:"zone",simulationEnergyZoneName:"Main Office",simulationEnergyPeriod:"M2",simulationEnergyService:"heating",simulationEnergySelection:"main-selected",simulationEnergyDetailsOpen:true});
 const mainKeys=["simulationResult","report",...Object.keys(mainState).filter(key=>key.startsWith("simulationEnergy"))],mainBefore=JSON.stringify(Object.fromEntries(mainKeys.map(key=>[key,mainState[key]])));
 const baseline=document.getElementById("multiSimulationCompareBaseline"),target=document.getElementById("multiSimulationCompareTarget"),panel=document.querySelector('[data-tools-panel="batch-simulation"]');
 const selectPair=(left,right)=>{baseline.value=fixture.payload.results[left].runId;baseline.dispatchEvent(new Event("change",{bubbles:true}));target.value=fixture.payload.results[right].runId;target.dispatchEvent(new Event("change",{bubbles:true}));};
 const model=(left=0,right=1)=>compare.energyPathBatchComparison(fixture.payload.results[left],fixture.payload.results[right]);
 const rowFor=row=>[...document.querySelectorAll("[data-batch-energy-row]")].find(element=>element.dataset.batchEnergyRow===row.key);
 const cells=row=>[...rowFor(row)?.children||[]].filter(cell=>["TD","TH"].includes(cell.tagName));
 const value=(row,index)=>{const text=cells(row)[index]?.innerText||"";return /^\s*[+−-]?\d/.test(text)?Number(text.match(/[+−-]?[\d,.]+/)[0].replaceAll(",","").replace("−","-")):null;};
 const find=(group,label,left=0,right=1)=>{const rows=model(left,right).rows.filter(row=>row.stage===group&&row.kind!=="quality");return rows.find(row=>row.category.toLowerCase()===label)||rows.find(row=>row.category.toLowerCase().includes(label));};
 const open=(index,origin="target")=>[...document.querySelectorAll("[data-batch-energy-open]")].find(button=>button.dataset.batchEnergyOpen===fixture.payload.results[index].runId&&button.dataset.batchEnergyOpenOrigin===origin);
 const close=()=>document.querySelector("[data-batch-energy-close]");
 const region=()=>document.querySelector("#multiSimulationEnergyPathDetail[data-batch-energy-detail]");
 selectPair(0,1);
 if(new URLSearchParams(location.search).get("manual")==="1"){
  const screenshotTarget=new URLSearchParams(location.search).get("mode")==="detail"?region():document.querySelector("[data-batch-energy-comparison]");
  if(new URLSearchParams(location.search).get("mode")==="detail"){const button=open(1);button.focus();button.click();await sleep(150);region()?.querySelector('[data-energy-path-layout-node]')?.click();}
  screenshotTarget?.scrollIntoView({block:"start",behavior:"instant"});await sleep(180);
  document.body.dataset.epath170Status="manual";document.getElementById("epath170-result").textContent=JSON.stringify({mode:new URLSearchParams(location.search).get("mode"),innerWidth,innerHeight,scrollY,target:screenshotTarget?.getBoundingClientRect().toJSON(),body:document.body.getBoundingClientRect().toJSON(),html:document.documentElement.getBoundingClientRect().toJSON()});
 }else{
  check(innerWidth===1600&&innerHeight===900,"actual Tools viewport is not1600x900");
  check(document.querySelectorAll("#multiSimulationTable [data-multi-sim-row]").length===100,"actual Batch result intake did not retain all100 models");
  const duplicateID=fixture.payload.results[98].runId;check(![...baseline.options,...target.options].some(option=>option.value===duplicateID&&!option.disabled)&&![...document.querySelectorAll("[data-batch-energy-open]")].some(button=>button.dataset.batchEnergyOpen===duplicateID&&!button.disabled),"duplicate result identity exposed arbitrary first/last compare or detail target");
  check(fixture.calls.select===1&&fixture.calls.run===1&&fixture.calls.forbidden===0,"fixture intake bypassed actual selection/run controls or ran hidden analysis");
  const table=document.querySelector("[data-batch-energy-summary-table]");
  check(table&&table.querySelectorAll("thead th").length===6&&[...table.querySelectorAll("thead th")].map(cell=>cell.textContent.trim()).join("|")==="Stage|Category|Baseline|Target|Delta|Delta %","primary comparison is not the six-column summary table");
  check(document.querySelectorAll("[data-batch-energy-summary-table]").length===1&&!document.querySelector("[data-energy-path-canvas]"),"100-model comparison eagerly rendered graphs / multiple summaries");
  check(!panel.querySelector("[data-simulation-energy-scope],[data-simulation-energy-zone-name],[data-simulation-energy-path-period],[data-simulation-energy-service]"),"Batch primary comparison exposes unsupported Zone/month/service controls");
  check(/Building/.test(document.querySelector("[data-batch-energy-comparison]").innerText)&&/Annual/i.test(document.querySelector("[data-batch-energy-comparison]").innerText),"fixed Building/Annual comparison context is not visible");
  check(new Set([...table.querySelectorAll("[data-batch-energy-stage]")].map(row=>row.dataset.batchEnergyStage)).size===6,"primary comparison must contain exactly six allowed stages, withoutTopZones");
  check(!/PRIVATE_170|Sankey Edge Delta|Largest Energy Explanation Changes|Unrelated Zone detail/.test(document.querySelector("[data-batch-energy-comparison]").innerText),"raw trace IDs, old edge rankings or TopZones leaked into primary comparison");
  const walls=find("drivers","exterior"),cooling=find("loads","cooling"),coolUse=find("endUses","cooling"),electricity=find("carriers","electricity"),refrigeration=find("endUses","refrigeration"),equipment=find("endUses","equipment"),water=find("endUses","water");
  for(const[row,expected,label]of[[walls,[50,40,-10,-20],"walls"],[cooling,[100,80,-20,-20],"cooling"],[coolUse,[25,20,-5,-20],"cooling equipment"],[electricity,[40,37,-3,-7.5],"electricity"]]){check(Boolean(row),"missing canonical summary row "+label);if(row)check(expected.every((number,index)=>Math.abs(value(row,index+2)-number)<.001),label+" used graph/ID/Zone fallback instead of exact annual summary arithmetic: "+JSON.stringify(cells(row).map(cell=>cell.innerText)));}
  check(coolUse&&visible(rowFor(coolUse)?.querySelector("[data-batch-energy-basis-warning]"))&&rowFor(coolUse).innerText.includes("Not directly comparable"),"reported vs service-path allocated basis lacks required badge");
  check(coolUse&&cells(coolUse)[4]?.querySelector("[data-batch-energy-coverage-warning]"),"stage source coverage difference lacks warning next to Delta");
  const endUseCoverage=model().rows.find(row=>row.kind==="quality"&&row.categoryId==="endUses_coverage");check(endUseCoverage&&value(endUseCoverage,2)===100&&value(endUseCoverage,3)===50&&value(endUseCoverage,4)===-50&&cells(endUseCoverage)[4].innerText.includes("pp"),"coverage comparison used stale mappedPercent or percent-of-percent delta units");
  check(refrigeration&&value(refrigeration,2)===0&&value(refrigeration,3)===5&&value(refrigeration,4)===5&&value(refrigeration,5)===null,"explicit zero / zero-baseline delta percent was not preserved honestly");
  for(const[row,label]of[[equipment,"missing equipment"],[water,"null water"]])check(row&&value(row,2)>0&&value(row,3)===null&&value(row,4)===null&&value(row,5)===null,label+" was silently treated as zero");
  check(cells(equipment)[3]?.querySelector('[data-batch-energy-value-status="missing"]')&&cells(water)[3]?.querySelector('[data-batch-energy-value-status="invalid"]'),"missing and invalid values share an unexplained emdash");
  const comparison=document.querySelector("[data-batch-energy-comparison]");check(comparison.scrollWidth<=comparison.clientWidth+1&&comparison.getBoundingClientRect().right<=panel.getBoundingClientRect().right+1,"summary table escaped actual Tools content width");
  const order=[...document.querySelectorAll("[data-batch-energy-row]")].map(row=>row.dataset.batchEnergyRow).join("|");selectPair(1,0);check([...document.querySelectorAll("[data-batch-energy-row]")].map(row=>row.dataset.batchEnergyRow).join("|")===order,"summary row order depends on baseline IDs/values");selectPair(0,1);
  const selectedBefore=[baseline.value,target.value].join("|"),opener=open(1);check(opener?.tagName==="BUTTON"&&!opener.disabled,"selected target has no native Energy Path detail action");opener.focus();opener.click();await sleep(80);
  let detail=region();check(visible(detail)&&detail.innerText.includes("B-target.idf")&&detail.querySelectorAll("[data-energy-path-canvas]").length===1&&document.querySelectorAll("[data-energy-path-canvas]").length===1,"single-model detail opened wrong model or dualgraphs");
  check(inViewport(detail.querySelector(".batch-energy-detail-header"))&&inViewport(detail.querySelector("[data-energy-path-canvas]")),"opening selected model left detail header/graph outside actual viewport: "+JSON.stringify(detail.getBoundingClientRect().toJSON()));
  check(detail.querySelector("[data-energy-path-fixed-context]")&&!detail.querySelector("[data-simulation-energy-scope],[data-simulation-energy-zone-name],[data-simulation-energy-path-period],[data-simulation-energy-service]"),"local detail is not pinned to Building/Annual/All");
  const node=detail.querySelector('[data-energy-path-stage="load"] [data-energy-path-layout-node]');node.focus();node.click();check(detail.querySelector("[data-energy-path-inspector]")&&document.activeElement===node,"local detail node did not use actual inspector/focus");
  const canvas=detail.querySelector("[data-energy-path-canvas]"),conversion=detail.querySelector('button[data-energy-path-bridge-ratio]');conversion.focus();conversion.click();check(detail.querySelector("[data-energy-path-link-inspector]")&&document.activeElement===conversion&&detail.querySelector("[data-energy-path-canvas]")===canvas,"local detail link selection rebuilt graph or lost exact focused link");
  const detailToggle=detail.querySelector("[data-energy-path-details-toggle]");detailToggle.click();const outputSource=detail.querySelector("[data-energy-path-data-details] [data-energy-path-output-source]");check(outputSource&&!outputSource.disabled,"local Data drawer has no exact run-specific Output jump");outputSource?.click();const outputRow=detail.querySelector('[data-energy-path-output-request-selected="true"]');check(outputRow&&outputRow.innerText.includes("Cooling:Electricity")&&outputRow.innerText.includes("Monthly")&&document.activeElement===outputRow,"Batch Data Output jump did not reveal its own exact request");document.activeElement.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true}));check(!visible(detail.querySelector("[data-energy-path-data-details]"))&&document.activeElement===detailToggle,"local Output Escape lost drawer-opener focus");
  const kpi=detail.querySelector('[data-energy-path-kpi="cooling_load"] [data-energy-path-kpi-node]');kpi.click();check(detail.querySelector("[data-energy-path-inspector]")?.dataset.energyPathInspector===kpi.dataset.energyPathKpiNode&&detail.querySelector("[data-energy-path-canvas]")===canvas,"Batch KPI selection used main workspace or rebuilt its local graph");
  detail.querySelector('[data-energy-path-quality-stage="drivers"]').click();check(visible(detail.querySelector("[data-energy-path-data-details]")),"local detail quality button did not open Data drawer");
  document.activeElement.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape",bubbles:true,cancelable:true}));check(!visible(detail.querySelector("[data-energy-path-data-details]")),"local Data Escape did not close only drawer");
  close().click();check(!visible(region())&&document.activeElement===opener&&inViewport(opener)&&[baseline.value,target.value].join("|")===selectedBefore,"closing detail lost visible actual opener focus or selected comparison pair");
  const other=open(0,"baseline");other.click();await sleep(50);detail=region();check(detail.innerText.includes("A-baseline.idf")&&document.querySelectorAll("[data-energy-path-canvas]").length===1&&!detail.querySelector("[data-energy-path-inspector]"),"switching selected model reused prior selection/graph");close().click();
  selectPair(0,2);const wrongUnit=find("carriers","electricity",0,2),wrongRatio=find("ratios","cooling",0,2);for(const[row,label]of[[wrongUnit,"carrier"],[wrongRatio,"ratio"]])check(row&&value(row,4)===null&&value(row,5)===null,label+" unit mismatch fabricated comparable arithmetic");
  selectPair(0,7);const ambiguous=find("drivers","exterior",0,7);check(ambiguous&&value(ambiguous,3)===null&&value(ambiguous,4)===null&&cells(ambiguous)[3]?.querySelector('[data-batch-energy-value-status="ambiguous"]'),"duplicate canonical category silently picked first or lacks explicit ambiguity");
  for(const index of[3,4,5]){const allowed=[...target.options].some(option=>option.value===fixture.payload.results[index].runId&&!option.disabled);if(!allowed)continue;selectPair(0,index);const invalid=document.querySelector("[data-batch-energy-comparison]");check(/summary is unavailable/i.test(invalid.innerText)&&[...invalid.querySelectorAll("tbody tr")].every(row=>[...row.children].slice(2).every(cell=>!/[0-9]/.test(cell.innerText))),"wrong scope/period/legacy summary borrowed graph values instead of explicit unavailable comparison: "+index);}
  selectPair(0,6);check(document.querySelector("[data-batch-energy-comparison]").innerText.includes("Not requested")&&!/NaN|Infinity/.test(document.querySelector("[data-batch-energy-comparison]").innerText),"sparse summary invented zero completeness or missing values");
  const sparseOpener=open(6);if(sparseOpener&&!sparseOpener.disabled){sparseOpener.click();check(!region()?.querySelector("[data-energy-path-canvas]")&&/unavailable|not available|no.*graph/i.test(region()?.innerText||""),"summary-only model fabricated another model's detail graph");close()?.click();}
  selectPair(0,1);check(fixture.calls.select===1&&fixture.calls.run===1&&fixture.calls.forbidden===0,"comparison/detail actions called backend work");check(JSON.stringify(fixture.payload)===fixture.rawJSON,"comparison/detail mutated completed Batch payload");check(sessionStorage.getItem("idfAnalyzer.currentDocument")===fixture.mainJSON&&JSON.stringify(Object.fromEntries(mainKeys.map(key=>[key,mainState[key]])))===mainBefore,"local Batch detail mutated main Energy workspace/global result");
  evidence.push("100 actual Tools result rows, one six-column Building/Annual summary; exact arithmetic, missing/zero/basis/coverage/unit/duplicate guards passed");evidence.push("One selected model detail, native node/Data/focus interactions, no extra Run/Analyze and unchanged Batch/main workspace payloads");
  if(failures.length)throw new Error(failures.join("\n"));document.body.dataset.epath170Status="passed";document.getElementById("epath170-result").textContent=evidence.join("\n");
 }
 }
}catch(error){document.body.dataset.epath170Status="failed";document.getElementById("epath170-result").textContent=(error.stack||String(error))+"\n"+failures.join("\n");}
</script>`
