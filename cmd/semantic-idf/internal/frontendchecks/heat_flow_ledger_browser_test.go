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

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

func TestHeatFlowLedgerMeasuredExchangeBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("actual app Heat Flow Ledger browser regression")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	inputPath := filepath.Join(t.TempDir(), "heat-flow-executed.idf")
	if err := os.WriteFile(inputPath, []byte(heatFlowLedgerInput), 0o644); err != nil {
		t.Fatal(err)
	}
	document, err := idf.Parse(heatFlowLedgerInput)
	if err != nil {
		t.Fatal(err)
	}
	geometry := idf.AnalyzeGeometry(document)
	result := heatFlowLedgerResult(inputPath)
	fixture, err := json.Marshal(map[string]any{"geometry": geometry, "result": result})
	if err != nil {
		t.Fatal(err)
	}
	page := readTestFile(t, "frontend/src/index.html")
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		if !strings.Contains(page, script) {
			t.Fatalf("actual bootstrap changed: %s", script)
		}
		page = strings.Replace(page, script, "", 1)
	}
	page = strings.Replace(page, "</body>", epath142LayoutHTML+heatFlowLedgerHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/heat-flow-ledger.json", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(fixture)
	})
	mux.HandleFunc("/src/heat-flow-ledger.html", func(writer http.ResponseWriter, _ *http.Request) {
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
	browser.call("Page.navigate", map[string]any{"url": server.URL + "/src/heat-flow-ledger.html?manual=1"}, nil)
	for deadline := time.Now().Add(45 * time.Second); ; {
		status := string(browser.evaluate(`document.body?.dataset.heatFlowLedgerStatus`))
		if status == `"passed"` {
			t.Log(string(browser.evaluate(`document.getElementById('heat-flow-ledger-result').textContent`)))
			if review := os.Getenv("HEAT_FLOW_LEDGER_REVIEW"); review != "" {
				directory, err := os.MkdirTemp("", "idf-heat-flow-review-")
				if err != nil {
					t.Fatal(err)
				}
				for _, capture := range []struct {
					name, theme, language string
					width                 int
				}{{"dark-ko", "dark", "ko", 1600}, {"light-en", "light", "en", 1100}} {
					if review == "history-final" && capture.theme != "dark" {
						continue
					}
					height := 1000
					if review == "history" || review == "history-final" || review == "inspector" {
						capture.width = 1600
						capture.name += "-" + review
					}
					if review == "inspector" {
						height = 1500
					} else if review == "history-final" {
						height = 2000
					}
					browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": capture.width, "height": height, "deviceScaleFactor": 1, "mobile": false}, nil)
					browser.evaluate(fmt.Sprintf(`(async()=>{document.documentElement.dataset.theme=%q; document.documentElement.style.setProperty('--graph-label-font-size','18px'); const {setLanguage}=await import('/src/js/i18n.js');setLanguage(%q);window.heatFlowLedgerReview();await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));document.getElementById('simulationHeatFlow').scrollIntoView({block:'start'});})()`, capture.theme, capture.language))
					if review == "history" || review == "history-final" || review == "inspector" {
						selector := "[data-heatflow-history]"
						if review == "inspector" {
							selector = ".heatflow-inspector"
						}
						t.Log(string(browser.evaluate(fmt.Sprintf(`(async()=>{document.querySelector('[data-layout-preset="analysis"]').click();await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));document.querySelector(%q).scrollIntoView({block:'start'});await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));return {mainWidth:document.querySelector('.analysis-panel').getBoundingClientRect().width,history:document.querySelector('[data-heatflow-history]').getBoundingClientRect().toJSON(),inspector:document.querySelector('.heatflow-inspector').getBoundingClientRect().toJSON()};})()`, selector))))
					}
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
					t.Logf("temporary actual app Heat Flow review screenshot: %s", path)
				}
			}
			return
		}
		if status == `"failed"` || time.Now().After(deadline) {
			t.Fatalf("Heat Flow Ledger browser %s: %s", status, browser.evaluate(`document.getElementById('heat-flow-ledger-result')?.textContent`))
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func heatFlowLedgerResult(inputPath string) simulation.SimulationRunResult {
	ids := []string{"internalConvective", "surfaceConvection", "interzoneAir", "outdoorAir", "systemAir", "systemConvective", "airStorage", "deviation"}
	colors := []string{"#f59e0b", "#ef4444", "#a855f7", "#14b8a6", "#3b82f6", "#64748b", "#e5e7eb", "#94a3b8"}
	categories := make([]simulation.HeatFlowCategory, len(ids))
	for index, id := range ids {
		categories[index] = simulation.HeatFlowCategory{ID: id, Label: id, Unit: "W", Color: colors[index]}
	}
	mask := func() [][]bool {
		values := make([][]bool, len(ids))
		for index := range values {
			values[index] = []bool{true, true, true}
		}
		return values
	}
	bMask := mask()
	bMask[3][1] = false
	result := simulation.SimulationRunResult{RunID: "heat-flow-fixture", Status: "succeeded", InputPath: inputPath, Filename: filepath.Base(inputPath), HeatFlow: simulation.HeatFlowDataset{
		SourceFile: "eplusout.sql", Unit: "W", TemperatureUnit: "C", FrameCount: 3, OriginalFrameCount: 6,
		Labels: []string{"01-01 01:00", "01-01 03:00", "01-01 06:00"}, Categories: categories, MaxAbs: 30000, MinTemperature: 21, MaxTemperature: 25,
		Zones: []simulation.HeatFlowZoneSeries{
			{Name: "A", Values: [][]float64{{25600, 18000, 1000}, {1100, -4000, 0}, {500, -500, 0}, {1000, -1000, 0}, {-24600, -30000, -1000}, {0, 300, 0}, {2200, -17200, 0}, {-100, 300, 0}}, Observed: mask(), Temperature: []float64{22.5, 23, 24}, TemperatureObserved: []bool{true, true, true}},
			{Name: "B", Values: [][]float64{{3000, 4000, 2000}, {-300, 0, 0}, {-500, 500, 0}, {-1000, 0, 0}, {-1000, -2000, -2000}, {0, 0, 0}, {200, 2500, 0}, {0, 0, 0}}, Observed: bMask, Temperature: []float64{21, 0, 25}, TemperatureObserved: []bool{true, false, true}},
		},
	}}
	add := func(key string, values []float64) {
		item := simulation.SimulationSeries{File: "eplusout.csv", Column: key + ":Surface Average Face Conduction Heat Transfer Energy [J]", ReportingFrequency: "Hourly"}
		for index, value := range values {
			item.Points = append(item.Points, simulation.SimulationPoint{X: index + 1, Label: fmt.Sprintf("01-01 %02d:00", index+1), Value: value * 3_600_000})
		}
		result.Series = append(result.Series, item)
	}
	add("Pair A", []float64{2, 99, -3, 88, 77, 0})
	add("Pair A2", []float64{-.5, 99, 1, 88, 77, 0})
	add("A Outdoors", []float64{-1, -66, 4, -55, -44, 0})
	bundle := simulation.BuildPurposeResultBundle(&result, simulation.SimulationPurposeRequest{Purposes: []simulation.SimulationPurposeID{simulation.SimulationPurposeZoneHeatFlow}, ZoneHeatFlowDetail: simulation.PurposeZoneHeatFlowDetailSurface})
	result.PurposeResults = &bundle
	return result
}

const heatFlowLedgerInput = `Version,25.1;
GlobalGeometryRules,UpperLeftCorner,CounterClockWise,World,World,World;
Material,Wall Layer,MediumRough,0.1,0.8,1800,900;
Construction,Wall Construction,Wall Layer;
Zone,A,0,0,0,0,1,1;
Zone,B,0,0,0,0,1,1;
Zone,C,0,0,0,0,1,1;
BuildingSurface:Detailed,A Floor,Floor,Wall Construction,A,,Ground,,NoSun,NoWind,0.5,4,0,0,0,0,6,0,6,6,0,6,0,0;
BuildingSurface:Detailed,B Floor,Floor,Wall Construction,B,,Ground,,NoSun,NoWind,0.5,4,6,0,0,6,6,0,12,6,0,12,0,0;
BuildingSurface:Detailed,C Floor,Floor,Wall Construction,C,,Ground,,NoSun,NoWind,0.5,4,12,0,0,12,6,0,18,6,0,18,0,0;
BuildingSurface:Detailed,Pair A,Wall,Wall Construction,A,,Surface,Pair B,NoSun,NoWind,0.5,4,6,0,0,6,3,0,6,3,3,6,0,3;
BuildingSurface:Detailed,Pair B,Wall,Wall Construction,B,,Surface,Pair A,NoSun,NoWind,0.5,4,6,0,3,6,3,3,6,3,0,6,0,0;
BuildingSurface:Detailed,Pair A2,Wall,Wall Construction,A,,Surface,Pair B2,NoSun,NoWind,0.5,4,6,3,0,6,6,0,6,6,3,6,3,3;
BuildingSurface:Detailed,Pair B2,Wall,Wall Construction,B,,Surface,Pair A2,NoSun,NoWind,0.5,4,6,3,3,6,6,3,6,6,0,6,3,0;
BuildingSurface:Detailed,A Outdoors,Wall,Wall Construction,A,,Outdoors,,SunExposed,WindExposed,0.5,4,0,0,0,6,0,0,6,0,3,0,0,3;
`

const heatFlowLedgerHTML = `<pre id="heat-flow-ledger-result" hidden>pending</pre><script type="module">
const failures=[],evidence=[];
const check=(value,message)=>{if(!value)failures.push(message);};
const tick=()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
const freeze=value=>{if(value&&typeof value==='object'){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
const clone=value=>JSON.parse(JSON.stringify(value));
try{
 for(let n=0;document.body.dataset.epath142Status!=='manual'&&n<400;n++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath142Status!=='manual')throw Error('actual shell failed to bootstrap');
 const {state}=await import('/src/js/state.js'),simulation=await import('/src/js/views/simulation-views.js'),i18n=await import('/src/js/i18n.js');
 const data=await import('/src/js/heat-flow-data.js'),map=await import('/src/js/views/heat-flow-map.js');
 i18n.setLanguage('en');
 const fixture=freeze(await(await fetch('/heat-flow-ledger.json')).json()),before=JSON.stringify(fixture);
 const result=fixture.result,dataset=result.purposeResults.zoneHeatFlow,overlay=result.purposeResults.thermalTopology;
 check(overlay.planGeometry?.surfaces.length===3&&overlay.planGeometry.sourceModelHash?.length===64,'production result lacks executed compact plan');
 check(overlay.periods.find(period=>period.id==='hourly').boundaryFlows.every(flow=>flow.observed.every(Boolean)),'production native hourly sources did not qualify');
 Object.assign(state,{report:{geometry:fixture.geometry},simulationResult:result,simulationRunning:false,simulationActiveResultView:'zone_heat_flow',simulationHeatFlowSelectedZone:'A',simulationHeatFlowFrameIndex:0,simulationHeatFlowRangeStart:0,simulationHeatFlowRangeEnd:-1,simulationHeatFlowStory:'all',simulationHeatFlowInspectorCollapsed:false,simulationHeatFlowPlanScale:1,simulationHeatFlowPlanPanX:0,simulationHeatFlowPlanPanY:0});
 simulation.renderSimulation();await tick();
 const host=document.getElementById('simulationHeatFlow');
 const summary=name=>host.querySelector('[data-heatflow-summary="'+name+'"]');
 const kpi=name=>host.querySelector('[data-heatflow-kpi="'+name+'"]');
 const arrows=()=>[...host.querySelectorAll('[data-heatflow-arrow]')];
 const render=async(next=result)=>{state.simulationResult=freeze(next);simulation.renderSimulation();await tick();};
 const rateRow=id=>host.querySelector('[data-heatflow-ledger="'+id+'"]');
 const assertMissingMap=name=>{
  const shape=host.querySelector('g[data-heat-zone="'+name+'"]'),polygon=shape?.querySelector('polygon'),fill=polygon?getComputedStyle(polygon).fill:'';
  check(shape?.dataset.heatflowMapObserved==='false'&&shape.classList.contains('missing')&&fill.includes('heatflow-unavailable-')&&shape.closest('svg').querySelector('pattern .heatflow-unavailable-stripe'),'unobserved map value is indistinguishable from known zero '+name);
 };
 const assertRateBar=(id,value,direction,max)=>{
  const row=rateRow(id),observed=Number.isFinite(value),fill=row?.querySelector('[data-heatflow-bar-fill]');
  check(row?.dataset.value===(observed?String(value):'')&&row.dataset.unit==='W'&&row.dataset.observed===String(observed)&&row.dataset.direction===direction&&Number(row.dataset.barMax)===max,'signed rate bar lost exact W/observation/direction/shared-scale metadata '+id);
  check(row?.querySelector('strong')?.title===data.heatFlowExactWatts(value),'signed rate bar lost precise source W tooltip '+id);
  if(!observed||value===0){check(!fill||fill.getBoundingClientRect().width===0,'unknown/zero rate created a visible gain or loss bar '+id);check(observed?row?.querySelector('strong')?.textContent==='0.000 kW':row?.querySelector('strong')?.textContent!=='0.000 kW','unknown and known-zero visible values became indistinguishable '+id);return;}
  const bounds=fill?.getBoundingClientRect(),track=fill?.parentElement,trackBounds=track?.getBoundingClientRect(),width=bounds?.width/track?.clientWidth*100,left=(bounds?.left-trackBounds?.left-track?.clientLeft)/track?.clientWidth*100,expected=50*Math.log1p(Math.abs(value))/Math.log1p(max);
  check(bounds?.width>0&&Math.abs(width-expected)<.4&&Math.abs(left-(value>0?50:50-expected))<.4,'signed rate bar does not use the disclosed fixed log W scale from a common zero center '+JSON.stringify({id,width,left,expected}));
 };
 check(host.querySelectorAll('g[data-heat-zone]').length===3&&summary('C')?.querySelector('[data-heatflow-local-gain]').getAttribute('data-heatflow-local-gain')==='','geometry-only zone became a reported zero or disappeared');
 assertMissingMap('C');check(!host.querySelector('g[data-heat-zone="C"] .heatflow-mini-stack'),'geometry-only missing zone manufactured a known-zero baseline');
 check(summary('A')?.querySelector('[data-heatflow-local-gain]').dataset.heatflowLocalGain==='25600'&&summary('A').querySelector('[data-heatflow-local-loss]').dataset.heatflowLocalLoss==='-24600','zone local gain/loss numbers are not separate native W quantities');
 check(kpi('net')?.textContent==='+3.600 kW'&&kpi('net').title==='+3,600.000000 W'&&kpi('residual')?.textContent==='+1.400 kW','storage/deviation were counted as extra heat transfer or precision changed');
 check([...host.querySelectorAll('[data-heatflow-local-category]')].every(bar=>['internalConvective','systemAir','systemConvective'].includes(bar.dataset.heatflowLocalCategory)),'exchange or storage still appears in local map bars');
 check(arrows().length===2&&arrows().find(arrow=>arrow.dataset.peer==='zone:b')?.dataset.heatflowArrow==='incoming'&&arrows().find(arrow=>arrow.dataset.peer==='zone:b')?.dataset.value==='1.5','compact pair arrow lost signed net value or retained duplicate opposing paths');
 check(arrows().find(arrow=>arrow.dataset.peer==='zone:b')?.dataset.grossIn==='2'&&arrows().find(arrow=>arrow.dataset.peer==='zone:b')?.dataset.grossOut==='0.5','compact arrow lost gross directions in its source attributes');
 check(arrows().filter(arrow=>arrow.dataset.peer==='outdoors').length===1&&!host.querySelector('.heatflow-external-label'),'Outside was split into individual orientation or external name rails');
 const neutralMount=document.createElement('div'),pair=map.heatFlowMeasuredExchanges(overlay,'A',dataset.labels[0]).find(item=>item.peerID==='zone:b');
 neutralMount.innerHTML=map.renderHeatFlowExchangeArrows([{...pair,incoming:.5,outgoing:.5}],{x:80,y:80},new Map([['b',{x:200,y:80}]]),{width:300,height:160,markerID:'neutral-contract'});
 const neutralArrow=neutralMount.querySelector('[data-heatflow-arrow]');
 check(neutralMount.querySelectorAll('[data-heatflow-arrow]').length===1&&neutralArrow?.dataset.heatflowArrow==='neutral'&&neutralArrow.dataset.value==='0'&&neutralArrow.querySelector('path').getAttribute('marker-start')&&neutralArrow.querySelector('path').getAttribute('marker-end'),'equal opposite gross flows did not retain one bidirectional net-zero path');
 check(arrows().every(arrow=>arrow.dataset.unit==='kWh'&&arrow.querySelector('path').getAttribute('marker-end')),'interval energy was relabelled W or lacks arrowheads');
 check(host.querySelector('[data-heatflow-peer="zone:b"] [data-heatflow-pair-in="2"]')&&host.querySelector('[data-heatflow-peer="zone:b"] [data-heatflow-pair-out="0.5"]'),'measured pair ledger lost its separate simultaneous signed directions');
 check(host.querySelector('[data-heatflow-aggregate="outdoorAir"]').dataset.value==='1000'&&host.querySelector('[data-heatflow-aggregate="outdoorAir"]').classList.contains('incoming'),'outdoor zone-air transfer lacks explicit direction');
 for(const [id,value]of[['internalConvective',25600],['surfaceConvection',1100],['interzoneAir',500],['outdoorAir',1000],['systemAir',-24600],['systemConvective',0],['airStorage',2200],['residual',1400],['deviation',-100]])assertRateBar(id,value,value>0?'incoming':value<0?'outgoing':'neutral',30000);
 check(host.querySelectorAll('.heatflow-inspector [data-heatflow-ledger]').length===9&&[...host.querySelectorAll('.heatflow-inspector [data-heatflow-ledger]')].every(row=>row.dataset.unit==='W'),'inspector rate bars mix measured interval-energy kWh with W rates');
 check(host.querySelector('[data-heatflow-history]')&&!host.querySelector('.heatflow-inspector [data-heatflow-history]'),'history remains squeezed into narrow inspector');
 const history=host.querySelector('[data-heatflow-history]');const chartHit=history.querySelector('[data-heatflow-chart]');const hitBounds=chartHit.getBoundingClientRect(),clickHistoryPath=history.querySelector('[data-heatflow-category="internalConvective"]'),clickHistoryD=clickHistoryPath.getAttribute('d'),clickTimelinePath=host.querySelector('.heatflow-timeline-line');
 chartHit.dispatchEvent(new PointerEvent('pointermove',{clientX:hitBounds.left+hitBounds.width*.9,clientY:hitBounds.top+hitBounds.height/2,bubbles:true}));
 check(state.simulationHeatFlowFrameIndex===0,'moving pointer over graph unexpectedly changed inspected frame');
 chartHit.dispatchEvent(new MouseEvent('click',{clientX:hitBounds.left+hitBounds.width*.5,clientY:hitBounds.top+hitBounds.height/2,bubbles:true}));
 check(state.simulationHeatFlowFrameIndex===1,'clicking center frame bin did not select exact observation');
 await tick();check(host.querySelector('[data-heatflow-history]')===history&&history.querySelector('[data-heatflow-category="internalConvective"]')===clickHistoryPath&&clickHistoryPath.getAttribute('d')===clickHistoryD&&host.querySelector('.heatflow-timeline-line')===clickTimelinePath&&history.querySelectorAll('[data-heatflow-chart-frame="1"]').length===3,'graph frame click rebuilt full-history/timeline geometry or retained a stale cursor');
 state.simulationHeatFlowFrameIndex=0;await render();
 const netScale=host.querySelector('[data-heatflow-scale="net"]')?.textContent;
 check(netScale?.includes('-17.200 kW')&&netScale.includes('/ 0 /')&&netScale.includes('+17.200 kW')&&/fixed|all frames/i.test(netScale)&&/log/i.test(netScale),'net map legend does not disclose its fixed all-time numeric bounds and log colour scale');
 check(/fixed|all frames/i.test(host.querySelector('.heatflow-bar-scale')?.textContent)&&/log/i.test(host.querySelector('.heatflow-bar-scale')?.textContent),'mini-stack legend omits its fixed all-time log scale');
 const netFill=host.querySelector('g[data-heat-zone="A"] polygon').getAttribute('style');
 const color=name=>getComputedStyle(host.querySelector('g[data-heat-zone="'+name+'"] polygon')).fill.match(/\d+/g).map(Number);
 const aColor=color('A'),bColor=color('B');check(aColor[0]-aColor[2]>40&&bColor[0]-bColor[2]>20,'normal positive balances remain indistinguishable gray on the fixed annual log scale');
 const aMini=host.querySelector('g[data-heat-zone="A"] [data-heatflow-local-category="internalConvective"]'),bMini=host.querySelector('g[data-heat-zone="B"] [data-heatflow-local-category="internalConvective"]');
 check(Number(aMini.getAttribute('height'))>=40&&Number(aMini.getAttribute('width'))>=14&&Math.abs(Number(aMini.getAttribute('height'))-46*Math.log1p(25600)/Math.log1p(30000))<.02&&Math.abs(Number(bMini.getAttribute('height'))-46*Math.log1p(3000)/Math.log1p(30000))<.02,'mini sign totals are faint or no longer use the shared fixed log W scale');
 const overlayControl=document.getElementById('simulationHeatFlowOverlay');overlayControl.value='temperature';overlayControl.dispatchEvent(new Event('change',{bubbles:true}));
 const temperatureScale=host.querySelector('[data-heatflow-scale="temperature"]');
 check(temperatureScale&&/21(?:\.0)? degC.*25(?:\.0)? degC/.test(temperatureScale.textContent)&&host.querySelector('g[data-heat-zone="A"] polygon').getAttribute('style')!==netFill,'temperature overlay did not switch map fill and numeric legend units');
 overlayControl.value='net';overlayControl.dispatchEvent(new Event('change',{bubbles:true}));
 check(host.querySelector('[data-heatflow-scale="net"]').textContent===netScale&&host.querySelector('g[data-heat-zone="A"] polygon').getAttribute('style')===netFill,'returning to net overlay changed the same-frame numerical scale');
 await tick();
 const outlier=clone(result),outlierData=outlier.purposeResults.zoneHeatFlow;outlierData.zones[0].values[0][2]=587000;outlierData.maxAbs=587000;
 await render(outlier);
 const outlierScale=host.querySelector('[data-heatflow-scale="net"]').textContent,outlierColor=color('A'),outlierHeight=Number(host.querySelector('g[data-heat-zone="A"] [data-heatflow-local-category="internalConvective"]').getAttribute('height'));
 check(outlierScale.includes('586.000 kW')&&outlierScale!==netScale&&outlierColor[0]-outlierColor[2]>40&&outlierHeight>=30&&Math.abs(outlierHeight-46*Math.log1p(25600)/Math.log1p(587000))<.02,'historical +587kW peak is omitted from the fixed scale or still washes out ordinary log-scaled values');
 assertRateBar('internalConvective',25600,'incoming',587000);assertRateBar('systemAir',-24600,'outgoing',587000);
 await render();
 const geometryBefore=host.querySelector('[data-heatflow-plan]').outerHTML;
 const editable=clone(fixture.geometry);editable.surfaces.forEach(surface=>surface.vertices.forEach(point=>point.x+=1000));editable.zones[0].name='Edited A';
 state.report={geometry:editable};await render();check(host.querySelector('[data-heatflow-plan]').outerHTML===geometryBefore,'editing current analysis moved executed result geometry');
 const slider=document.getElementById('simulationHeatFlowSlider');slider.value='1';slider.dispatchEvent(new Event('input',{bubbles:true}));await tick();
 check(state.simulationHeatFlowFrameIndex===1&&kpi('net').textContent==='-17.200 kW'&&kpi('residual').textContent==='0.000 kW','native slider did not update physical balance/residual');
 const lossColor=color('A');check(lossColor[2]-lossColor[0]>40&&host.querySelector('[data-heatflow-scale="net"]').textContent===netScale,'negative balance lacks visible blue fill or changed the fixed all-time bound');
 const internalSegment=Number(host.querySelector('g[data-heat-zone="A"] [data-heatflow-local-category="internalConvective"]').getAttribute('height')),systemSegment=Number(host.querySelector('g[data-heat-zone="A"] [data-heatflow-local-category="systemConvective"]').getAttribute('height'));
 check(Math.abs(internalSegment+systemSegment-46*Math.log1p(18300)/Math.log1p(30000))<.025&&Math.abs(internalSegment/(internalSegment+systemSegment)-18000/18300)<.0005,'mini log-scaled total distorted the original18000:300 positive category composition');
 check(arrows().find(arrow=>arrow.dataset.peer==='zone:b')?.dataset.value==='-2'&&arrows().find(arrow=>arrow.dataset.peer==='zone:b')?.dataset.heatflowArrow==='outgoing','sampled frame index was used instead of exact native timestamp or pair net polarity changed');
 check(host.querySelector('[data-heatflow-peer="zone:b"] [data-heatflow-pair-out="3"]'),'sampled-frame pair ledger failed to match exact native timestamp');
 const b=host.querySelector('g[data-heat-zone="B"]');b.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));
 check(state.simulationHeatFlowSelectedZone==='B'&&host.querySelector('g[data-heat-zone="B"]').getAttribute('aria-pressed')==='true','keyboard activation did not select plan zone');
 check(kpi('net').textContent.endsWith('*')&&host.querySelector('.heatflow-partial-note')&&kpi('residual').textContent==='—','unobserved zone-air term became zero or complete residual');
 const unavailable=host.querySelector('[data-heatflow-aggregate="outdoorAir"]');check(unavailable.dataset.value===''&&unavailable.classList.contains('unavailable')&&!/[←→]/.test(unavailable.querySelector('strong').textContent),'unobserved outdoor exchange became a zero/direction');
 check(host.querySelector('g[data-heat-zone="B"]').dataset.heatflowMapObserved==='true','partial observed net became unavailable solely because one exchange term is masked');
 const temperatureControl=document.getElementById('simulationHeatFlowOverlay');temperatureControl.value='temperature';temperatureControl.dispatchEvent(new Event('change',{bubbles:true}));await tick();assertMissingMap('B');
 const netControl=document.getElementById('simulationHeatFlowOverlay');netControl.value='net';netControl.dispatchEvent(new Event('change',{bubbles:true}));await tick();
 const bBarHeight=host.querySelector('g[data-heat-zone="B"] [data-heatflow-local-category="systemAir"]').getAttribute('height');
 assertRateBar('outdoorAir',NaN,'unavailable',30000);assertRateBar('systemConvective',0,'neutral',30000);assertRateBar('residual',NaN,'unavailable',30000);
 const sameRateWidth=rateRow('systemAir').querySelector('[data-heatflow-bar-fill]').style.width;
 state.simulationHeatFlowFrameIndex=2;await render();
 check(host.querySelector('g[data-heat-zone="B"] [data-heatflow-local-category="systemAir"]').getAttribute('height')===bBarHeight&&Number(bBarHeight)>0&&rateRow('systemAir').querySelector('[data-heatflow-bar-fill]').style.width===sameRateWidth,'same -2000W value changes mini/inspector size between frames on the fixed scale');
 check(host.querySelector('[data-heatflow-scale="net"]').textContent===netScale,'map legend changes its fixed all-time bounds when the frame changes');
 state.simulationHeatFlowFrameIndex=1;await render();
 const summaryButton=host.querySelector('[data-heatflow-summary="A"] button'),summaryDetails=summaryButton.closest('details');if(summaryDetails&&!summaryDetails.open)summaryDetails.querySelector('summary').click();
 summaryButton.click();check(state.simulationHeatFlowSelectedZone==='A','zone summary button lost native selection');
 await tick();
 const planGeometry=()=>JSON.stringify([...host.querySelectorAll('[data-heatflow-plan-content]')].map(item=>item.outerHTML));
 const planBounds=()=>JSON.stringify([...host.querySelectorAll('[data-heatflow-plan] polygon')].map(item=>{const bounds=item.getBoundingClientRect();return [bounds.left,bounds.top,bounds.width,bounds.height].map(value=>Number(value.toFixed(3)));}));
 const fixedGeometry=planGeometry(),fixedBounds=planBounds();
 Object.assign(state,{simulationHeatFlowPlanScale:3.5,simulationHeatFlowPlanPanX:140,simulationHeatFlowPlanPanY:-90});await render();
 check(!host.querySelector('[data-heatflow-plan-zoom],.heatflow-viewport-actions')&&!host.querySelector('[data-heatflow-plan-content][transform]'),'fixed Heat Flow plans still expose camera controls or transform');
 check(planGeometry()===fixedGeometry&&planBounds()===fixedBounds,'obsolete saved scale/pan moved the fixed Heat Flow plan');
 const fixedPlan=host.querySelector('[data-heatflow-plan]'),planRectangle=fixedPlan.getBoundingClientRect(),planX=planRectangle.left+planRectangle.width/2,planY=planRectangle.top+planRectangle.height/2;
 const planWheel=new WheelEvent('wheel',{deltaY:-120,clientX:planX,clientY:planY,bubbles:true,cancelable:true});fixedPlan.dispatchEvent(planWheel);
 check(!planWheel.defaultPrevented,'fixed floor plan captures normal page scrolling');
 fixedPlan.dispatchEvent(new PointerEvent('pointerdown',{pointerId:901,pointerType:'mouse',button:0,buttons:1,clientX:planX,clientY:planY,bubbles:true,cancelable:true}));
 window.dispatchEvent(new PointerEvent('pointermove',{pointerId:901,pointerType:'mouse',buttons:1,clientX:planX+80,clientY:planY+60,bubbles:true}));
 window.dispatchEvent(new PointerEvent('pointerup',{pointerId:901,pointerType:'mouse',button:0,clientX:planX+80,clientY:planY+60,bubbles:true}));
 fixedPlan.dispatchEvent(new MouseEvent('dblclick',{clientX:planX,clientY:planY,bubbles:true,cancelable:true}));await tick();
 check(planGeometry()===fixedGeometry&&planBounds()===fixedBounds&&state.simulationHeatFlowFrameIndex===1,'wheel/drag/double-click moved a fixed plan or changed the selected time');
 const historyMarkup=()=>host.querySelector('[data-heatflow-history]').outerHTML.replace(/ data-heatflow-chart-context="[^"]*"/g,''),chartBefore=historyMarkup();host.querySelector('[data-heatflow-inspector-toggle]').click();
 check(state.simulationHeatFlowInspectorCollapsed&&historyMarkup()===chartBefore,'collapsing ledger hid or changed full-width graph');
 host.querySelector('[data-heatflow-inspector-toggle]').click();
 for(const [name,mutate]of[
  ['missing execution snapshot',next=>delete next.purposeResults.thermalTopology.planGeometry],
  ['old overlay without observed flags',next=>next.purposeResults.thermalTopology.periods.forEach(period=>period.boundaryFlows.forEach(flow=>delete flow.observed))],
  ['missing native timestamp',next=>next.purposeResults.zoneHeatFlow.labels[1]='02-01 03:00'],
  ['duplicate native timestamp',next=>{const period=next.purposeResults.thermalTopology.periods.find(item=>item.id==='hourly');period.labels[1]=period.labels[2];}],
  ['masked native surface observation',next=>next.purposeResults.thermalTopology.periods.find(item=>item.id==='hourly').boundaryFlows.forEach(flow=>flow.observed[2]=false)]
 ]){
  const next=clone(result);mutate(next);state.report={geometry:fixture.geometry};await render(next);check(arrows().length===0,name+' manufactured measured pair arrows');
  check(host.querySelector('[data-heatflow-aggregate="outdoorAir"]').dataset.value==='-1000',name+' removed independent aggregate zone observations');
 }
 await render();state.simulationHeatFlowFrameIndex=2;await render();
 check(arrows().length===0&&kpi('net').textContent==='0.000 kW'&&!kpi('net').textContent.includes('*'),'reported zero heat was treated as missing or created transfer arrow');
 check(host.querySelector('[data-heatflow-aggregate="interzoneAir"]').classList.contains('neutral'),'reported zero interzone transfer lacks neutral state');
 assertRateBar('outdoorAir',0,'neutral',30000);assertRateBar('residual',0,'neutral',30000);
 const allZero=clone(result);allZero.purposeResults.zoneHeatFlow.zones.forEach(zone=>{zone.values.forEach(values=>values.fill(0));zone.observed.forEach(values=>values.fill(true));});await render(allZero);
 check(host.querySelectorAll('[data-heatflow-ledger]').length===9&&[...host.querySelectorAll('[data-heatflow-ledger]')].every(row=>row.dataset.observed==='true'&&row.dataset.value==='0'&&row.dataset.barMax==='0'&&row.dataset.direction==='neutral'&&!row.querySelector('[data-heatflow-bar-fill]')),'all known-zero rates became unavailable or created artificial bars');
 check(host.querySelector('g[data-heat-zone="A"]').dataset.heatflowMapObserved==='true'&&host.querySelector('g[data-heat-zone="A"] .heatflow-mini-stack line')&&!host.querySelector('g[data-heat-zone="A"] .heatflow-mini-stack rect'),'known zero map loses its neutral observed baseline');
 const allMissing=clone(allZero);allMissing.purposeResults.zoneHeatFlow.zones.forEach(zone=>zone.observed.forEach(values=>values.fill(false)));await render(allMissing);
 check(host.querySelectorAll('[data-heatflow-ledger]').length===9&&[...host.querySelectorAll('[data-heatflow-ledger]')].every(row=>row.dataset.observed==='false'&&row.dataset.value===''&&row.dataset.barMax===''&&row.dataset.direction==='unavailable'&&!row.querySelector('[data-heatflow-bar-fill]')),'all missing rates became known-zero values or a manufactured positive scale');
 assertMissingMap('A');check(!host.querySelector('g[data-heat-zone="A"] .heatflow-mini-stack'),'fully unobserved local terms manufacture a known-zero baseline');
 const normalized=clone(result),basisZone=normalized.purposeResults.zoneHeatFlow.zones.find(zone=>zone.name==='B');
 basisZone.rateBasis={effectiveMultiplier:6,systemAir:'modeled_zone',reportedSystemAir:[-6000,-12000,-12000],systemConvective:'energyplus_reported',reportedSystemConvective:[0,300,0]};basisZone.values[5][1]=300;
 state.simulationHeatFlowSelectedZone='B';state.simulationHeatFlowFrameIndex=1;await render(normalized);
 const normalizedAir=rateRow('systemAir'),normalizedTitle=normalizedAir.querySelector('strong').title;
 check(normalizedAir.dataset.value==='-2000'&&normalizedAir.querySelector('strong').textContent==='-2.000 kW'&&normalizedTitle.includes('-2,000.000000 W')&&normalizedTitle.includes('-12,000.000000 W')&&normalizedTitle.includes('multiplier 6'),'corrected HVAC air rate lost displayed -2000W/raw -12000W/multiplier6 source provenance or was divided twice');
 assertRateBar('systemConvective',300,'incoming',30000);
 const mixedWarning=i18n.t('simulation.heatFlowMixedBasisNote'),hasMixedWarning=()=>[...host.querySelectorAll('.heatflow-partial-note')].some(item=>item.textContent===mixedWarning);
 check(hasMixedWarning()&&rateRow('systemConvective').querySelector('strong').textContent==='+0.300 kW','mixed-basis nonzero HVAC convective gain was renormalized or lacks its translated source-basis warning');
 const normalizedZero=clone(normalized),reportedZeroZone=normalizedZero.purposeResults.zoneHeatFlow.zones.find(zone=>zone.name==='B');reportedZeroZone.values[5][1]=0;reportedZeroZone.rateBasis.reportedSystemConvective[1]=0;await render(normalizedZero);
 check(hasMixedWarning()&&rateRow('systemConvective').dataset.value==='0'&&rateRow('systemConvective').dataset.observed==='true'&&rateRow('systemConvective').querySelector('strong').textContent==='0.000 kW'&&!rateRow('systemConvective').querySelector('[data-heatflow-bar-fill]'),'observed mixed-basis zero lost its cancellation warning or was changed into a modeled nonzero/unavailable value');
 const normalizedMissing=clone(normalized);normalizedMissing.purposeResults.zoneHeatFlow.zones.find(zone=>zone.name==='B').observed[5][1]=false;await render(normalizedMissing);
 check(!hasMixedWarning()&&rateRow('systemConvective').dataset.observed==='false'&&!rateRow('systemConvective').querySelector('[data-heatflow-bar-fill]'),'unobserved raw HVAC convective sample became a normalized numeric gain/warning');
 const absent=clone(dataset.zones[0]);absent.values[0][0]=null;const balance=data.heatFlowBalance(dataset,absent,0);
 check(!balance.complete&&Number.isNaN(balance.residual)&&Number.isNaN(data.heatFlowCategoryValue(absent,0,0)),'null numeric data became complete heat balance');
 state.simulationHeatFlowSelectedZone='A';state.simulationHeatFlowFrameIndex=0;await render();
 const preset=async(labels,frame,name)=>{
  const next=clone(result),series=next.purposeResults.zoneHeatFlow;series.labels=labels;series.frameCount=series.originalFrameCount=labels.length;
  series.zones.forEach(zone=>{zone.values=zone.values.map(values=>labels.map(()=>values[0]));zone.observed=zone.observed.map(()=>labels.map(()=>true));zone.temperature=labels.map(()=>22);zone.temperatureObserved=labels.map(()=>true);});
  state.simulationHeatFlowFrameIndex=frame;state.simulationHeatFlowRangeStart=0;state.simulationHeatFlowRangeEnd=-1;await render(next);
  const button=host.querySelector('[data-heatflow-range-preset="'+name+'"]');button.click();
  return {start:state.simulationHeatFlowRangeStart,end:state.simulationHeatFlowRangeEnd,disabled:button.disabled};
 };
 const sampled=Array.from({length:9},(_,index)=>{const date=new Date(Date.UTC(2000,0,1,1+index*13));return '01-'+String(date.getUTCDate()).padStart(2,'0')+' '+String(date.getUTCHours()).padStart(2,'0')+':00';});
 let range=await preset(sampled,4,'day');check(range.start===4&&range.end===4,'24-hour preset used 24 sampled indices instead of source calendar time: '+JSON.stringify(range));
 range=await preset(sampled,4,'week');check(range.start===0&&range.end===8,'week preset did not use elapsed calendar hours');
 const timeChart=host.querySelector('[data-heatflow-chart]'),timeBounds=timeChart.getBoundingClientRect(),timeWheel=new WheelEvent('wheel',{deltaY:-120,clientX:timeBounds.left+timeBounds.width/4,clientY:timeBounds.top+timeBounds.height/2,bubbles:true,cancelable:true});
 timeChart.dispatchEvent(timeWheel);await tick();
 const zoomStart=state.simulationHeatFlowRangeStart,zoomEnd=state.simulationHeatFlowRangeEnd;
 check(timeWheel.defaultPrevented&&zoomEnd-zoomStart+1<sampled.length,'removing plan zoom also disabled history time-range zoom');
 host.querySelector('[data-heatflow-chart]').dispatchEvent(new WheelEvent('wheel',{deltaY:120,shiftKey:true,bubbles:true,cancelable:true}));await tick();
 check(state.simulationHeatFlowRangeStart>zoomStart&&state.simulationHeatFlowRangeEnd-state.simulationHeatFlowRangeStart===zoomEnd-zoomStart,'history Shift-scroll no longer pans the selected time range');
 host.querySelector('[data-heatflow-chart]').dispatchEvent(new MouseEvent('dblclick',{bubbles:true,cancelable:true}));await tick();
 check(state.simulationHeatFlowRangeStart===0&&state.simulationHeatFlowRangeEnd===8,'history double-click no longer restores full time range');
 range=await preset(['01-01 12:00','01-01 24:00','01-02 01:00','01-02 12:00'],2,'day');check(range.start===1&&range.end===3,'24:00 calendar rollover shifted or discarded observed time: '+JSON.stringify(range));
 range=await preset(['01-01 01:00','01-01 14:00','07-21 03:00','07-21 16:00','01-01 01:00','01-01 14:00'],3,'week');check(range.start===2&&range.end===3,'range crossed backwards design-day/run sequence boundary: '+JSON.stringify(range));
 range=await preset(['01-01 01:00','01-01 02:00','01-01 02:00','01-01 03:00'],1,'day');check(range.start===0&&range.end===1,'range crossed ambiguous duplicate timestamps');
 range=await preset(['01-01 01:00','02-31 01:00','01-03 01:00'],1,'day');check(range.disabled&&range.start===0&&range.end===2,'invalid calendar time enabled elapsed-time preset');
 check(data.heatFlowFrameTime('01-01 24:00')===data.heatFlowFrameTime('01-02 00:00')&&Number.isNaN(data.heatFlowFrameTime('01-01 24:01')),'calendar parser changed EnergyPlus 24:00 meaning');
 const playbackResult=clone(result),playback=playbackResult.purposeResults.zoneHeatFlow;
 playback.labels=['01-01 01:00','01-01 01:15','01-01 01:30','01-01 02:00','01-01 02:15'];playback.frameCount=playback.originalFrameCount=playback.labels.length;
 playback.zones.forEach((zone,zoneIndex)=>{
  zone.values=playback.categories.map((_category,category)=>playback.labels.map((_label,frame)=>category===0?(zoneIndex===0?1100:101)*(frame+1):category===4?-(zoneIndex===0?100:1)*(frame+1):category===6?(zoneIndex===0?1000:100)*(frame+1):0));
  zone.observed=playback.categories.map(()=>playback.labels.map(()=>true));zone.temperature=playback.labels.map((_label,frame)=>20+frame);zone.temperatureObserved=playback.labels.map(()=>true);delete zone.rateBasis;
 });
 playback.zones[0].observed[0][2]=false;
 check(playbackResult.heatFlow.frameCount===3&&playback.frameCount===5,'purpose-dataset authority regression lost its distinct legacy frame count');
 const playbackBefore=JSON.stringify(playbackResult),nativeInterval=window.setInterval,nativeClearInterval=window.clearInterval,playTimers=new Map();let nextPlayTimer=900000;
 window.setInterval=(callback,delay,...args)=>{if(![900,420,160].includes(Number(delay)))return nativeInterval.call(window,callback,delay,...args);const timer=nextPlayTimer++;playTimers.set(timer,{delay,callback:()=>callback(...args)});return timer;};
 window.clearInterval=timer=>{if(!playTimers.delete(timer))nativeClearInterval.call(window,timer);};
 try{
  const play=document.getElementById('simulationHeatFlowPlay'),speed=document.getElementById('simulationHeatFlowSpeed'),frameLabel=document.getElementById('simulationHeatFlowFrame');
  const advance=async()=>{check(playTimers.size===1,'playback has no timer or concurrent timers');const timer=[...playTimers.values()][0];if(timer)timer.callback();await tick();};
  const historyPath=()=>host.querySelector('[data-heatflow-category="internalConvective"]'),timelinePath=()=>host.querySelector('.heatflow-timeline-line');
  const assertPlaybackFrame=(frame,series=playback.zones[0])=>{
   const available=series.observed[0][frame],raw=series.values[0][frame],legend=host.querySelector('[data-heatflow-chart-legend="internalConvective"] strong');
   check(state.simulationHeatFlowFrameIndex===frame&&frameLabel.textContent===playback.labels[frame]&&host.querySelector('[data-heatflow-chart-current]').textContent===playback.labels[frame],'Play skipped, reordered or invented a native timestamp '+frame);
   check(legend.dataset.heatflowLegendValue===(available?String(raw):'')&&host.querySelectorAll('[data-heatflow-chart-frame="'+frame+'"]').length===3,'Play current legend/cursors do not describe the exact native source frame '+frame);
   check(host.querySelector('.heatflow-chart-missing').hidden===available&&rateRow('internalConvective').dataset.value===(available?String(raw):'')&&rateRow('internalConvective').dataset.observed===String(available),'Play retained stale numeric/missing status '+frame);
  };
  Object.assign(state,{simulationHeatFlowSelectedZone:'A',simulationHeatFlowFrameIndex:0,simulationHeatFlowRangeStart:0,simulationHeatFlowRangeEnd:-1,simulationActiveResultView:'zone_heat_flow'});speed.value='420';await render(playbackResult);
  check(playback.labels.length===5&&!playback.labels.includes('01-01 01:45')&&host.querySelector('[data-heatflow-chart]').closest('svg').dataset.heatflowChartTimeMode==='elapsed','native cadence fixture lost its explicit unobserved 15-minute gap');
  const fixedHistory=historyPath(),fixedHistoryD=fixedHistory.getAttribute('d'),fixedTimeline=timelinePath(),fixedTimelineD=fixedTimeline.getAttribute('d'),fixedFloor=host.querySelector('[data-heatflow-plan-content]');
  const assertStaticPlayback=()=>check(historyPath()===fixedHistory&&fixedHistory.getAttribute('d')===fixedHistoryD&&timelinePath()===fixedTimeline&&fixedTimeline.getAttribute('d')===fixedTimelineD&&host.querySelector('[data-heatflow-plan-content]')===fixedFloor,'Play/Pause/speed/slider rebuilt static full-history, timeline or floor geometry');
  assertPlaybackFrame(0);play.click();await tick();check(state.simulationHeatFlowPlaying&&playTimers.size===1&&[...playTimers.values()][0].delay===420,'native Play did not start one timer at the selected speed');assertStaticPlayback();
  const cursorX=new Map([[0,Number(host.querySelector('[data-heatflow-chart-frame]').getAttribute('x1'))]]);
  for(const frame of[1,2,3,4,0]){await advance();assertPlaybackFrame(frame);assertStaticPlayback();cursorX.set(frame,Number(host.querySelector('[data-heatflow-chart-frame]').getAttribute('x1')));}
  check(Math.abs((cursorX.get(3)-cursorX.get(2))/(cursorX.get(1)-cursorX.get(0))-2)<.0001,'playback chart interpolated or compressed the unobserved 01:45 source gap');
  for(const delay of[900,420,160]){
   const previous=[...playTimers.keys()][0],frame=state.simulationHeatFlowFrameIndex;speed.value=String(delay);speed.dispatchEvent(new Event('change',{bubbles:true}));await tick();
   check(playTimers.size===1&&!playTimers.has(previous)&&[...playTimers.values()][0].delay===delay&&state.simulationHeatFlowFrameIndex===frame,'speed restart leaks timers or advances/skips a native frame '+delay);assertStaticPlayback();
   await advance();assertPlaybackFrame((frame+1)%5);assertStaticPlayback();
  }
  const pausedFrame=state.simulationHeatFlowFrameIndex;play.click();await tick();check(!state.simulationHeatFlowPlaying&&playTimers.size===0&&state.simulationHeatFlowFrameIndex===pausedFrame,'Pause leaves a timer running or changes the inspected frame');assertStaticPlayback();
  slider.value='4';slider.dispatchEvent(new Event('input',{bubbles:true}));await tick();assertPlaybackFrame(4);assertStaticPlayback();
  const setRange=async(start,end)=>{for(const [id,value]of[['simulationHeatFlowRangeStart',start],['simulationHeatFlowRangeEnd',end]]){const input=document.getElementById(id);input.value=String(value);input.dispatchEvent(new Event('input',{bubbles:true}));await tick();}};
  await setRange(1,3);check(historyPath()!==fixedHistory&&[...host.querySelectorAll('[data-heatflow-chart]')].every(chart=>chart.closest('svg').dataset.heatflowChartStart==='1'&&chart.closest('svg').dataset.heatflowChartEnd==='3'),'native visible-range change reused full-range history geometry');
  slider.value='1';slider.dispatchEvent(new Event('input',{bubbles:true}));await tick();play.click();await tick();
  for(const frame of[2,3,1]){await advance();assertPlaybackFrame(frame);}
  const seriesTab=document.querySelector('[data-simulation-result-view-button="series"]');check(!seriesTab.disabled,'leave-view playback fixture has no available destination');seriesTab.click();await tick();
  check(state.simulationActiveResultView==='series'&&!state.simulationHeatFlowPlaying&&playTimers.size===0,'leaving Heat Flow keeps advancing hidden results');
  document.querySelector('[data-simulation-result-view-button="zone_heat_flow"]').click();await tick();check(!state.simulationHeatFlowPlaying&&playTimers.size===0,'returning to Heat Flow silently resumes playback');
  const aHistory=historyPath();host.querySelector('g[data-heat-zone="B"]').dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));await tick();
  check(historyPath()!==aHistory&&host.querySelector('.heatflow-chart-zone').textContent==='B','zone selection reuses the previous zone history');assertPlaybackFrame(1,playback.zones[1]);
  const englishHistory=historyPath();i18n.setLanguage('ko');await render(playbackResult);check(historyPath()!==englishHistory&&!host.querySelector('.heatflow-chart-heading h4').textContent.includes('Heat-flow history'),'language change retains stale history text');i18n.setLanguage('en');
  Object.assign(state,{simulationHeatFlowSelectedZone:'A',simulationHeatFlowFrameIndex:0,simulationHeatFlowRangeStart:0,simulationHeatFlowRangeEnd:-1});await render(playbackResult);
  const ordinalResult=clone(playbackResult),ordinal=ordinalResult.purposeResults.zoneHeatFlow;ordinal.labels=['07-21 01:00','07-21 01:00','01-21 01:00','01-21 01:15','01-21 01:30'];await render(ordinalResult);
  check(host.querySelector('[data-heatflow-chart]').closest('svg').dataset.heatflowChartTimeMode==='ordinal'&&host.querySelector('.heatflow-chart-sequence-note'),'duplicate/backwards source sequence was sorted or treated as elapsed time');play.click();await tick();
  for(const frame of[1,2,3,4,0]){await advance();check(state.simulationHeatFlowFrameIndex===frame&&frameLabel.textContent===ordinal.labels[frame]&&host.querySelectorAll('[data-heatflow-chart-frame="'+frame+'"]').length===3,'ordinal Play skipped or sorted a recorded native frame '+frame);}
  await setRange(2,2);await advance();check(play.disabled&&!state.simulationHeatFlowPlaying&&playTimers.size===0&&state.simulationHeatFlowFrameIndex===2,'reducing a playing range to one native frame leaves a useless timer or changes that frame');play.click();await tick();check(playTimers.size===0,'disabled single-frame Play starts a timer');
  check(JSON.stringify(playbackResult)===playbackBefore,'incremental Play mutated native labels, values or observation masks');
 }finally{
  if(state.simulationHeatFlowPlaying)document.getElementById('simulationHeatFlowPlay').click();
  playTimers.clear();window.setInterval=nativeInterval;window.clearInterval=nativeClearInterval;
 }
 state.simulationHeatFlowFrameIndex=0;state.simulationHeatFlowRangeStart=0;state.simulationHeatFlowRangeEnd=-1;await render();
 for(const language of['en','ko'])for(const font of[11,18]){
  i18n.setLanguage(language);document.documentElement.style.setProperty('--graph-label-font-size',font+'px');await render();
  for(const item of host.querySelectorAll('.heatflow-chart-tick')){const size=parseFloat(getComputedStyle(item).fontSize)*item.getScreenCTM().a;check(Math.abs(size-Math.max(12,font))<.04,'graph font is unreadable or ignores setting '+JSON.stringify({language,font,size}));}
  check(host.querySelector('[data-heatflow-summary="A"] button').textContent.includes('A'),'translated UI renamed source zone');
 }
 i18n.setLanguage('en');document.documentElement.style.setProperty('--graph-label-font-size','11px');await render();
 window.heatFlowLedgerReview=()=>{state.report={geometry:fixture.geometry};state.simulationResult=result;state.simulationHeatFlowFrameIndex=0;state.simulationHeatFlowRangeStart=0;state.simulationHeatFlowRangeEnd=-1;state.simulationHeatFlowSelectedZone='A';state.simulationHeatFlowInspectorCollapsed=false;simulation.renderSimulation();};
 check(JSON.stringify(fixture)===before,'view mutated executed source geometry/numeric result');
 evidence.push('real IDF geometry + production canonical pair flow builder; exact sampled/native timestamps; fixed all-time log scales retain ordinary-value visibility with historical +587kW peaks; original mini-segment proportions; signed log inspector bars with exact W metadata/values; kW transfer vs kWh interval; storage/deviation excluded; known zero and missing masks; snapshot ownership; keyboard/selection; fixed plan ignores legacy camera state and wheel/drag/double-click; native 15-minute Play visits every recorded frame without interpolating missing times, speed/range/pause/view lifecycle, static history/timeline/floor DOM identity, zone/range/language invalidation and ordinal DesignDay sequence; EN/KO + configured physical graph fonts');
}catch(error){failures.push(error.stack||String(error));}
document.body.dataset.heatFlowLedgerStatus=failures.length?'failed':'passed';document.getElementById('heat-flow-ledger-result').textContent=JSON.stringify({failures,evidence});
</script>`
