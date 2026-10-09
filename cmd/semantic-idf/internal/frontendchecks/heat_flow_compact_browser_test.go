package frontendchecks

import (
	"context"
	"crypto/sha256"
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

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

func TestHeatFlowLargeOfficePlenumCompactBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("actual LargeOffice geometry Heat Flow compact browser regression")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	inputPath := repoPath("frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf")
	input, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	inputHash := sha256.Sum256(input)
	document, err := idf.Parse(string(input))
	if err != nil {
		t.Fatal(err)
	}
	geometry := idf.AnalyzeGeometry(document)
	result := heatFlowLargeOfficeResult(inputPath, geometry)
	geometryJSON, _ := json.Marshal(geometry)
	fixture, err := json.Marshal(map[string]any{"geometry": geometry, "result": result})
	if err != nil {
		t.Fatal(err)
	}
	floorPieces := map[string]int{}
	for _, surface := range geometry.Surfaces {
		if strings.EqualFold(surface.SurfaceType, "Floor") {
			floorPieces[surface.ZoneName]++
		}
	}
	if len(geometry.Zones) != 19 || len(geometry.Stories) != 7 || floorPieces["GroundFloor_Plenum"] != 5 || floorPieces["MidFloor_Plenum"] != 5 || floorPieces["TopFloor_Plenum"] != 5 {
		t.Fatalf("actual LargeOffice/plenum fixture changed: zones %d stories %d floor pieces %+v", len(geometry.Zones), len(geometry.Stories), floorPieces)
	}
	t.Logf("production LargeOffice: 19 zones, 7 elevations, floor pieces %+v", floorPieces)
	page := readTestFile(t, "frontend/src/index.html")
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		if !strings.Contains(page, script) {
			t.Fatalf("actual bootstrap changed: %s", script)
		}
		page = strings.Replace(page, script, "", 1)
	}
	page = strings.Replace(page, "</body>", epath142LayoutHTML+heatFlowCompactHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/heat-flow-compact.json", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(fixture)
	})
	mux.HandleFunc("/src/heat-flow-compact.html", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	browser := epath201FreshBrowser(t, ctx, chrome)
	browser.call("Page.enable", map[string]any{}, nil)
	browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 1600, "height": 1100, "deviceScaleFactor": 1, "mobile": false}, nil)
	browser.call("Page.navigate", map[string]any{"url": server.URL + "/src/heat-flow-compact.html?manual=1"}, nil)
	for deadline := time.Now().Add(45 * time.Second); ; {
		status := string(browser.evaluate(`document.body?.dataset.heatFlowCompactStatus`))
		if status == `"passed"` {
			t.Log(string(browser.evaluate(`document.getElementById('heat-flow-compact-result').textContent`)))
			for _, width := range []int{1600, 1100} {
				browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": width, "height": 1100, "deviceScaleFactor": 1, "mobile": false}, nil)
				for _, theme := range []string{"dark", "light"} {
					var routeFailures []string
					if err := json.Unmarshal(browser.evaluate(fmt.Sprintf(`window.heatFlowCompactValidateRouteLayout(%q)`, theme)), &routeFailures); err != nil {
						t.Fatal(err)
					}
					if len(routeFailures) != 0 {
						t.Fatalf("actual LargeOffice bar/route layout at %dpx viewport %s theme: %v", width, theme, routeFailures)
					}
				}
				t.Logf("fixed native floor plans, signed-rate bars and unrelated-badge clearance pass at %dpx viewport in dark/light", width)
			}
			if os.Getenv("HEAT_FLOW_COMPACT_REVIEW") != "" {
				directory, err := os.MkdirTemp("", "idf-heat-flow-compact-review-")
				if err != nil {
					t.Fatal(err)
				}
				for _, capture := range []struct {
					name, zone string
					width      int
				}{{"large-office-plenum-wide", "GroundFloor_Plenum", 1600}, {"large-office-plenum-narrow", "GroundFloor_Plenum", 1100}} {
					if os.Getenv("HEAT_FLOW_COMPACT_REVIEW") == "wide" && capture.width != 1600 {
						continue
					}
					browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": capture.width, "height": 1100, "deviceScaleFactor": 1, "mobile": false}, nil)
					t.Log(string(browser.evaluate(fmt.Sprintf(`window.heatFlowCompactReview(%q)`, capture.zone))))
					var screenshot struct {
						Data string `json:"data"`
					}
					browser.call("Page.captureScreenshot", map[string]any{"format": "png", "fromSurface": true, "captureBeyondViewport": false}, &screenshot)
					data, err := base64.StdEncoding.DecodeString(screenshot.Data)
					if err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(directory, capture.name+".png")
					if err := os.WriteFile(path, data, 0o644); err != nil {
						t.Fatal(err)
					}
					t.Logf("temporary actual LargeOffice Heat Flow screenshot: %s", path)
				}
			}
			currentGeometry, _ := json.Marshal(geometry)
			if string(currentGeometry) != string(geometryJSON) {
				t.Fatal("LargeOffice fixture modified shared production geometry")
			}
			currentInput, err := os.ReadFile(inputPath)
			if err != nil || sha256.Sum256(currentInput) != inputHash {
				t.Fatal("LargeOffice UI fixture changed bundled original input")
			}
			return
		}
		if status == `"failed"` || time.Now().After(deadline) {
			t.Fatalf("LargeOffice compact browser %s: %s", status, browser.evaluate(`document.getElementById('heat-flow-compact-result')?.textContent`))
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// The original model and production geometry/flow identities are real. Bounded
// native observations exercise UI arithmetic without launching EnergyPlus or
// treating an old capture's absent observation masks as verified new evidence.
func heatFlowLargeOfficeResult(inputPath string, geometry idf.GeometryReport) simulation.SimulationRunResult {
	ids := []string{"internalConvective", "surfaceConvection", "interzoneAir", "outdoorAir", "systemAir", "systemConvective", "airStorage", "deviation"}
	colors := []string{"#f59e0b", "#ef4444", "#a855f7", "#14b8a6", "#3b82f6", "#64748b", "#e5e7eb", "#94a3b8"}
	dataset := simulation.HeatFlowDataset{FrameCount: 3, OriginalFrameCount: 3, Labels: []string{"01-01 01:00", "01-01 02:00", "01-01 03:00"}, Unit: "W", TemperatureUnit: "C", MinTemperature: 21, MaxTemperature: 25, MaxAbs: 587000}
	for index, id := range ids {
		dataset.Categories = append(dataset.Categories, simulation.HeatFlowCategory{ID: id, Label: id, Unit: "W", Color: colors[index]})
	}
	for index, zone := range geometry.Zones {
		values := []float64{5000 + float64(index)*100, 1000, 0, -700, -4000, 0, 1300 + float64(index)*100, 10}
		item := simulation.HeatFlowZoneSeries{Name: zone.Name, Temperature: []float64{22, 23, 24}, TemperatureObserved: []bool{true, true, true}}
		for _, value := range values {
			item.Values = append(item.Values, []float64{value, value, value})
			item.Observed = append(item.Observed, []bool{true, true, true})
		}
		if index == 0 {
			item.Values[0][2] = 587000
		}
		dataset.Zones = append(dataset.Zones, item)
	}
	result := simulation.SimulationRunResult{RunID: "large-office-compact-fixture", Status: "succeeded", InputPath: inputPath, HeatFlow: dataset}
	for index, surface := range geometry.Surfaces {
		item := simulation.SimulationSeries{File: "eplusout.csv", Column: surface.Name + ":Surface Average Face Conduction Heat Transfer Energy [J]", ReportingFrequency: "Hourly"}
		value := float64(index%7-3) * .15
		if value == 0 {
			value = .15
		}
		for frame, label := range dataset.Labels {
			item.Points = append(item.Points, simulation.SimulationPoint{X: frame + 1, Label: label, Value: value * 3_600_000})
		}
		result.Series = append(result.Series, item)
	}
	bundle := simulation.BuildPurposeResultBundle(&result, simulation.SimulationPurposeRequest{Purposes: []simulation.SimulationPurposeID{simulation.SimulationPurposeZoneHeatFlow}, ZoneHeatFlowDetail: simulation.PurposeZoneHeatFlowDetailSurface})
	result.PurposeResults = &bundle
	return result
}

const heatFlowCompactHTML = `<pre id="heat-flow-compact-result" hidden>pending</pre><script type="module">
const failures=[],evidence=[];const check=(value,message)=>{if(!value)failures.push(message);};
const tick=()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
const freeze=value=>{if(value&&typeof value==='object'){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
try{
 for(let n=0;document.body.dataset.epath142Status!=='manual'&&n<400;n++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath142Status!=='manual')throw Error('actual shell failed to bootstrap');
 const {state}=await import('/src/js/state.js'),simulation=await import('/src/js/views/simulation-views.js'),{setLanguage}=await import('/src/js/i18n.js');
 const fixture=freeze(await(await fetch('/heat-flow-compact.json')).json()),before=JSON.stringify(fixture);
 setLanguage('en');document.documentElement.dataset.theme='dark';document.querySelector('[data-layout-preset="analysis"]').click();
 Object.assign(state,{report:{geometry:fixture.geometry},simulationResult:fixture.result,simulationRunning:false,simulationActiveResultView:'zone_heat_flow',simulationHeatFlowSelectedZone:'GroundFloor_Plenum',simulationHeatFlowFrameIndex:0,simulationHeatFlowRangeStart:0,simulationHeatFlowRangeEnd:-1,simulationHeatFlowStory:'all',simulationHeatFlowInspectorCollapsed:false,simulationHeatFlowPlanScale:1,simulationHeatFlowPlanPanX:0,simulationHeatFlowPlanPanY:0});
 simulation.renderSimulation();await tick();
 const host=document.getElementById('simulationHeatFlow');
 const render=async zone=>{state.simulationHeatFlowSelectedZone=zone;simulation.renderSimulation();await tick();};
 const planGeometry=()=>JSON.stringify([...host.querySelectorAll('[data-heatflow-plan-content]')].map(item=>item.outerHTML));
 const planBounds=()=>JSON.stringify([...host.querySelectorAll('[data-heatflow-plan] polygon')].map(item=>{const bounds=item.getBoundingClientRect();return [bounds.left,bounds.top,bounds.width,bounds.height].map(value=>Number(value.toFixed(3)));}));
 const fixedGeometry=planGeometry(),fixedBounds=planBounds();
 Object.assign(state,{simulationHeatFlowPlanScale:3.75,simulationHeatFlowPlanPanX:-180,simulationHeatFlowPlanPanY:110});await render('GroundFloor_Plenum');
 check(!host.querySelector('[data-heatflow-plan-zoom],.heatflow-viewport-actions')&&!host.querySelector('[data-heatflow-plan-content][transform]'),'LargeOffice fixed floor plans retain camera controls/transform');
 check(planGeometry()===fixedGeometry&&planBounds()===fixedBounds,'stale LargeOffice scale/pan moved fixed floor geometry');
 const fixedPlan=host.querySelector('[data-heatflow-plan]'),bounds=fixedPlan.getBoundingClientRect(),x=bounds.left+bounds.width/2,y=bounds.top+bounds.height/2;
 const planWheel=new WheelEvent('wheel',{deltaY:120,clientX:x,clientY:y,shiftKey:true,bubbles:true,cancelable:true});fixedPlan.dispatchEvent(planWheel);
 check(!planWheel.defaultPrevented,'LargeOffice floor plan prevents native page scrolling');
 fixedPlan.dispatchEvent(new PointerEvent('pointerdown',{pointerId:902,pointerType:'mouse',button:0,buttons:1,clientX:x,clientY:y,bubbles:true,cancelable:true}));
 window.dispatchEvent(new PointerEvent('pointermove',{pointerId:902,pointerType:'mouse',buttons:1,clientX:x-60,clientY:y+90,bubbles:true}));
 window.dispatchEvent(new PointerEvent('pointerup',{pointerId:902,pointerType:'mouse',button:0,clientX:x-60,clientY:y+90,bubbles:true}));
 fixedPlan.dispatchEvent(new MouseEvent('dblclick',{clientX:x,clientY:y,bubbles:true,cancelable:true}));await tick();
 check(planGeometry()===fixedGeometry&&planBounds()===fixedBounds&&state.simulationHeatFlowFrameIndex===0,'LargeOffice wheel/drag/double-click moved floor geometry or time selection');
 check(host.querySelectorAll('.heatflow-floor-card').length===7,'LargeOffice story separation changed or floors were omitted');
 const fixedScale=host.querySelector('[data-heatflow-scale="net"]').textContent;
 check(fixedScale.includes('583.30 kW')&&/fixed|all frames/i.test(fixedScale)&&/log/i.test(fixedScale),'LargeOffice historical +587kW observation is omitted from its disclosed fixed all-time log map scale');
 const largestMini=Math.max(...[...host.querySelectorAll('[data-heatflow-local-category="internalConvective"]')].map(item=>Number(item.getAttribute('height'))));
 check(largestMini>=30&&Math.abs(largestMini-46*Math.log1p(6800)/Math.log1p(587000))<.02,'normal LargeOffice gains remain faint or have incorrect log size under a fixed annual outlier');
 const originalPlenumMini=host.querySelector('g[data-heat-zone="GroundFloor_Plenum"] .heatflow-mini-stack').outerHTML,originalPlenumColor=host.querySelector('g[data-heat-zone="GroundFloor_Plenum"] polygon').getAttribute('style');
 check([...host.querySelectorAll('.heatflow-inspector [data-heatflow-ledger]')].every(row=>row.dataset.barMax==='587000'),'LargeOffice inspector uses an independent frame/zone scale instead of fixed global W maximum');
 for(const name of['GroundFloor_Plenum','MidFloor_Plenum','TopFloor_Plenum']){
  const shapes=[...host.querySelectorAll('g[data-heat-zone]')].filter(item=>item.dataset.heatZone===name);
  const polygonCount=shapes.reduce((sum,item)=>sum+item.querySelectorAll('polygon').length,0),badgeCount=shapes.reduce((sum,item)=>sum+item.querySelectorAll('.heatflow-zone-number').length,0);
  check(polygonCount===5&&badgeCount===1,'plenum split surfaces duplicate badges or lost five native floor polygons '+JSON.stringify({name,polygonCount,badgeCount}));
 }
 const badges=[...host.querySelectorAll('.heatflow-zone-number')];
 check(badges.length===19&&new Set(badges.map(item=>item.textContent)).size===19,'zone numbering is not unique across all visible stories');
 check(!host.querySelector('[data-heatflow-summary-story][open]')&&!host.querySelector('.heatflow-surface-details[open]'),'compact map opens detailed numeric tables by default');
 for(const badge of badges){const bounds=badge.getBoundingClientRect(),plan=badge.closest('svg').getBoundingClientRect();check(bounds.left>=plan.left-1&&bounds.right<=plan.right+1&&bounds.top>=plan.top-1&&bounds.bottom<=plan.bottom+1,'zone badge clipped outside its compact floor plan');}
 for(const zone of['GroundFloor_Plenum','Core_bottom','TopFloor_Plenum']){
  await render(zone);
  const zoneShape=[...host.querySelectorAll('g[data-heat-zone]')].find(item=>item.dataset.heatZone===zone);
  const names=new Set([...host.querySelectorAll('g[data-heat-zone]')].map(item=>item.dataset.heatZone.toLowerCase()));
  const arrows=[...host.querySelectorAll('[data-heatflow-arrow]')],outside=arrows.filter(item=>item.dataset.peer==='outdoors');
  check(outside.length<=1,'Outside directions or peer rails were drawn separately for '+zone);
  check(!host.querySelector('.heatflow-external-label')&&!arrows.some(item=>/adiabatic|thermal-environment:ground/.test(item.dataset.peer)),'adiabatic/ground boundaries became exterior labels/arrows');
  const peers=arrows.filter(item=>item.dataset.peer!=='outdoors');
  check(new Set(peers.map(item=>item.dataset.peer)).size===peers.length,'visible pair produced multiple direction paths instead of one signed net');
  check(peers.every(item=>names.has(item.dataset.peer.replace(/^zone:/,''))),'arrow assigned an absent zone to the visible floor grid');
  if(zone.endsWith('_Plenum')){
   const suffix=zone==='GroundFloor_Plenum'?'bot':'top',core=zone==='GroundFloor_Plenum'?'bottom':'top';
   const expected=['zone:core_'+core,...[1,2,3,4].map(index=>'zone:perimeter_'+suffix+'_zn_'+index)];
   check(peers.length===5&&expected.every(peer=>peers.some(item=>item.dataset.peer===peer)),'real plenum must connect all five occupied zone badge centers on the other story '+JSON.stringify({zone,peers:peers.map(item=>item.dataset.peer)}));
  }
  check(arrows.every(item=>item.dataset.unit==='kWh'&&Math.abs(Number(item.dataset.value)-(Number(item.dataset.grossIn)-Number(item.dataset.grossOut)))<1e-10&&JSON.parse(item.dataset.boundaries).length&&JSON.parse(item.dataset.sources).length),'compact spatial arrow lost signed interval-energy arithmetic or source provenance');
  check(host.querySelectorAll('[data-heatflow-peer]').length>0,'compact arrows removed precise peer ledger');
 }
 const badgeFor=name=>[...host.querySelectorAll('[data-heatflow-zone-badge]')].find(item=>item.dataset.heatflowZoneBadge.toLowerCase()===name.toLowerCase());
 const assertBadgeAnchors=()=>{
  const center=badge=>{const bounds=badge.getBoundingClientRect();return {x:bounds.left+bounds.width/2,y:bounds.top+bounds.height/2,radius:bounds.width/2};};
  const selected=center(badgeFor(state.simulationHeatFlowSelectedZone));
  const peers=[...host.querySelectorAll('[data-heatflow-arrow][data-peer^="zone:"]')];
  check(peers.length===5,'details layout update removed actual selected plenum pair paths');
  for(const arrow of peers){
   const peer=center(badgeFor(arrow.dataset.peer.replace(/^zone:/,''))),path=arrow.querySelector('path'),coordinates=path.getAttribute('d').match(/-?\d+(?:\.\d+)?/g).map(Number),matrix=path.getScreenCTM();
   const incoming=arrow.dataset.heatflowArrow==='incoming',from=incoming?peer:selected,to=incoming?selected:peer;
   const start=new DOMPoint(coordinates[0],coordinates[1]).matrixTransform(matrix),end=new DOMPoint(coordinates.at(-2),coordinates.at(-1)).matrixTransform(matrix);
   const error=Math.max(Math.abs(Math.hypot(start.x-from.x,start.y-from.y)-(from.radius+4)),Math.abs(Math.hypot(end.x-to.x,end.y-to.y)-(to.radius+4)));
   check(error<.15,'shared arrow endpoints no longer track actual badge centers after details layout '+JSON.stringify({peer:arrow.dataset.peer,error}));
  }
 };
 await render('TopFloor_Plenum');assertBadgeAnchors();
 const selectedTopBefore=badgeFor('TopFloor_Plenum').getBoundingClientRect().top;
 const lowerStory=fixture.result.purposeResults.thermalTopology.planGeometry.zones.find(zone=>zone.name==='Core_bottom').storyIndex,detailsSelector='[data-heatflow-summary-story="'+lowerStory+'"]';
 host.querySelector(detailsSelector+' > summary').click();await tick();
 check(host.querySelector(detailsSelector).open&&badgeFor('TopFloor_Plenum').getBoundingClientRect().top-selectedTopBefore>20,'native details opening did not exercise lower-card position change');
 assertBadgeAnchors();state.simulationHeatFlowFrameIndex=1;await render('TopFloor_Plenum');
 check(host.querySelector(detailsSelector).open,'frame render discarded native zone-values open state');assertBadgeAnchors();
 check(host.querySelector('[data-heatflow-scale="net"]').textContent===fixedScale&&host.querySelector('g[data-heat-zone="GroundFloor_Plenum"] .heatflow-mini-stack').outerHTML===originalPlenumMini&&host.querySelector('g[data-heat-zone="GroundFloor_Plenum"] polygon').getAttribute('style')===originalPlenumColor,'same plenum W values change map colour/mini size after a frame change');
 host.querySelector(detailsSelector+' > summary').click();await tick();assertBadgeAnchors();
 await render('GroundFloor_Plenum');
 const unfilteredMini=host.querySelector('g[data-heat-zone="GroundFloor_Plenum"] .heatflow-mini-stack').outerHTML,unfilteredColor=host.querySelector('g[data-heat-zone="GroundFloor_Plenum"] polygon').getAttribute('style');
 const selector=document.getElementById('simulationHeatFlowStory'),plenumStory=fixture.result.purposeResults.thermalTopology.planGeometry.zones.find(zone=>zone.name==='GroundFloor_Plenum').storyIndex;
 selector.value=String(plenumStory);selector.dispatchEvent(new Event('change',{bubbles:true}));await tick();
 check(host.querySelectorAll('.heatflow-floor-card').length===1&&!host.querySelector('[data-heatflow-arrow][data-peer^="zone:"]'),'story filter still draws paths to absent zone badges');
 check(host.querySelector('[data-heatflow-scale="net"]').textContent===fixedScale&&host.querySelector('g[data-heat-zone="GroundFloor_Plenum"] .heatflow-mini-stack').outerHTML===unfilteredMini&&host.querySelector('g[data-heat-zone="GroundFloor_Plenum"] polygon').getAttribute('style')===unfilteredColor,'story filter changes the fixed global log scale or gives one zone a misleading independent visual magnitude');
 selector.value='all';selector.dispatchEvent(new Event('change',{bubbles:true}));await tick();
 window.heatFlowCompactValidateRouteLayout=async(theme='dark')=>{
  document.documentElement.dataset.theme=theme;
  await render('GroundFloor_Plenum');
  const errors=[],badges=[...host.querySelectorAll('[data-heatflow-zone-badge]')];
  if(host.querySelector('[data-heatflow-plan-zoom],.heatflow-viewport-actions,[data-heatflow-plan-content][transform]'))errors.push('fixed plans expose camera controls or transforms');
  const rateRows=[...host.querySelectorAll('.heatflow-inspector [data-heatflow-ledger]')];
  if(rateRows.length!==9)errors.push('inspector omits one or more signed W category/diagnostic rows');
  for(const row of rateRows){
   const bounds=row.getBoundingClientRect(),value=row.querySelector('strong'),valueBounds=value?.getBoundingClientRect(),fill=row.querySelector('[data-heatflow-bar-fill]');
   if(row.dataset.unit!=='W'||valueBounds?.left<bounds.left-1||valueBounds?.right>bounds.right+1||parseFloat(getComputedStyle(value).fontSize)<12)errors.push('explicit W value is unreadable or clips '+row.dataset.heatflowLedger);
   if(Number(row.dataset.value)!==0&&row.dataset.observed==='true'&&!(fill?.getBoundingClientRect().width>0))errors.push('observed nonzero signed rate lacks a visible bar '+row.dataset.heatflowLedger);
  }
  const arrows=[...host.querySelectorAll('[data-heatflow-arrow]')];
  for(const arrow of arrows){
   const path=arrow.querySelector('path'),length=path.getTotalLength(),matrix=path.getScreenCTM(),peer=arrow.dataset.peer.replace(/^zone:/,''),unrelated=badges.filter(item=>![state.simulationHeatFlowSelectedZone.toLowerCase(),peer].includes(item.dataset.heatflowZoneBadge.toLowerCase())).map(item=>{const bounds=item.getBoundingClientRect();return {name:item.dataset.heatflowZoneBadge,x:bounds.left+bounds.width/2,y:bounds.top+bounds.height/2,radius:bounds.width/2};});
   for(let step=0,count=Math.ceil(length/2);step<=count;step++){
    const point=path.getPointAtLength(length*step/count),screen=new DOMPoint(point.x,point.y).matrixTransform(matrix);
    const badge=unrelated.find(item=>Math.hypot(screen.x-item.x,screen.y-item.y)<item.radius+1);
    if(badge){errors.push(arrow.dataset.peer+' crosses unrelated zone '+badge.name);break;}
   }
  }
  return errors;
 };
 check(JSON.stringify(fixture)===before,'compact map modified canonical LargeOffice numeric/source geometry payload');
 window.heatFlowCompactReview=async zone=>{setLanguage('ko');document.documentElement.dataset.theme='dark';document.documentElement.style.setProperty('--graph-label-font-size','12px');await render(zone);host.scrollIntoView({block:'start'});await tick();return {zone,mainWidth:document.querySelector('.analysis-panel').getBoundingClientRect().width,floorCards:host.querySelectorAll('.heatflow-floor-card').length,arrows:[...host.querySelectorAll('[data-heatflow-arrow]')].map(item=>({peer:item.dataset.peer,net:Number(item.dataset.value),grossIn:Number(item.dataset.grossIn),grossOut:Number(item.dataset.grossOut)}))};};
 evidence.push('actual bundled LargeOffice production geometry: 19 zones / 7 stories / 31 native floor pieces; each 5-piece plenum retains one badge; fixed plans ignore stale camera state and wheel/drag/double-click; cross-story peers connect visible zone badges; one signed pair net and one Outside with gross ledger preserved; native details opening/closing repositions arrow endpoints and remains open after frame changes');
}catch(error){failures.push(error.stack||String(error));}
document.body.dataset.heatFlowCompactStatus=failures.length?'failed':'passed';document.getElementById('heat-flow-compact-result').textContent=JSON.stringify({failures,evidence});
</script>`
