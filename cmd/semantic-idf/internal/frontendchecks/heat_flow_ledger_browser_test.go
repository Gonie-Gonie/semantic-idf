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
 const render=(next=result)=>{state.simulationResult=freeze(next);simulation.renderSimulation();};
 check(host.querySelectorAll('g[data-heat-zone]').length===3&&summary('C')?.querySelector('[data-heatflow-local-gain]').getAttribute('data-heatflow-local-gain')==='','geometry-only zone became a reported zero or disappeared');
 check(summary('A')?.querySelector('[data-heatflow-local-gain]').dataset.heatflowLocalGain==='25600'&&summary('A').querySelector('[data-heatflow-local-loss]').dataset.heatflowLocalLoss==='-24600','zone local gain/loss numbers are not separate native W quantities');
 check(kpi('net')?.textContent==='+3.600 kW'&&kpi('net').title==='+3,600.000000 W'&&kpi('residual')?.textContent==='+1.400 kW','storage/deviation were counted as extra heat transfer or precision changed');
 check([...host.querySelectorAll('[data-heatflow-local-category]')].every(bar=>['internalConvective','systemAir','systemConvective'].includes(bar.dataset.heatflowLocalCategory)),'exchange or storage still appears in local map bars');
 check(arrows().length===3&&arrows().find(arrow=>arrow.dataset.peer==='zone:b'&&arrow.dataset.heatflowArrow==='incoming')?.dataset.value==='2'&&arrows().find(arrow=>arrow.dataset.peer==='zone:b'&&arrow.dataset.heatflowArrow==='outgoing')?.dataset.value==='0.5','canonical pair heat arrows netted simultaneous opposite flows or have wrong owner/sign/value');
 check(arrows().every(arrow=>arrow.dataset.unit==='kWh'&&arrow.querySelector('path').getAttribute('marker-end')),'interval energy was relabelled W or lacks arrowheads');
 check(host.querySelector('[data-heatflow-peer="zone:b"] [data-heatflow-pair-in="2"]')&&host.querySelector('[data-heatflow-peer="zone:b"] [data-heatflow-pair-out="0.5"]'),'measured pair ledger lost its separate simultaneous signed directions');
 check(host.querySelector('[data-heatflow-aggregate="outdoorAir"]').dataset.value==='1000'&&host.querySelector('[data-heatflow-aggregate="outdoorAir"]').classList.contains('incoming'),'outdoor zone-air transfer lacks explicit direction');
 check(host.querySelector('[data-heatflow-history]')&&!host.querySelector('.heatflow-inspector [data-heatflow-history]'),'history remains squeezed into narrow inspector');
 const history=host.querySelector('[data-heatflow-history]');const chartHit=history.querySelector('[data-heatflow-chart]');const hitBounds=chartHit.getBoundingClientRect();
 chartHit.dispatchEvent(new PointerEvent('pointermove',{clientX:hitBounds.left+hitBounds.width*.9,clientY:hitBounds.top+hitBounds.height/2,bubbles:true}));
 check(state.simulationHeatFlowFrameIndex===0,'moving pointer over graph unexpectedly changed inspected frame');
 chartHit.dispatchEvent(new MouseEvent('click',{clientX:hitBounds.left+hitBounds.width*.5,clientY:hitBounds.top+hitBounds.height/2,bubbles:true}));
 check(state.simulationHeatFlowFrameIndex===1,'clicking center frame bin did not select exact observation');
 state.simulationHeatFlowFrameIndex=0;render();
 const netScale=host.querySelector('[data-heatflow-scale="net"]')?.textContent;
 check(netScale?.includes('-17.200 kW')&&netScale.includes('/ 0 /')&&netScale.includes('+17.200 kW'),'net map legend lacks numeric physical balance bounds');
 const netFill=host.querySelector('g[data-heat-zone="A"] polygon').getAttribute('style');
 const overlayControl=document.getElementById('simulationHeatFlowOverlay');overlayControl.value='temperature';overlayControl.dispatchEvent(new Event('change',{bubbles:true}));
 const temperatureScale=host.querySelector('[data-heatflow-scale="temperature"]');
 check(temperatureScale&&/21(?:\.0)? degC.*25(?:\.0)? degC/.test(temperatureScale.textContent)&&host.querySelector('g[data-heat-zone="A"] polygon').getAttribute('style')!==netFill,'temperature overlay did not switch map fill and numeric legend units');
 overlayControl.value='net';overlayControl.dispatchEvent(new Event('change',{bubbles:true}));
 check(host.querySelector('[data-heatflow-scale="net"]').textContent===netScale&&host.querySelector('g[data-heat-zone="A"] polygon').getAttribute('style')===netFill,'returning to net overlay changed its fixed numerical scale');
 const geometryBefore=host.querySelector('[data-heatflow-plan]').outerHTML;
 const editable=clone(fixture.geometry);editable.surfaces.forEach(surface=>surface.vertices.forEach(point=>point.x+=1000));editable.zones[0].name='Edited A';
 state.report={geometry:editable};render();check(host.querySelector('[data-heatflow-plan]').outerHTML===geometryBefore,'editing current analysis moved executed result geometry');
 const slider=document.getElementById('simulationHeatFlowSlider');slider.value='1';slider.dispatchEvent(new Event('input',{bubbles:true}));
 check(state.simulationHeatFlowFrameIndex===1&&kpi('net').textContent==='-17.200 kW'&&kpi('residual').textContent==='0.000 kW','native slider did not update physical balance/residual');
 check(arrows().find(arrow=>arrow.dataset.peer==='zone:b'&&arrow.dataset.heatflowArrow==='outgoing')?.dataset.value==='3'&&arrows().find(arrow=>arrow.dataset.peer==='zone:b'&&arrow.dataset.heatflowArrow==='incoming')?.dataset.value==='1','sampled frame index was used instead of exact native timestamp');
 check(host.querySelector('[data-heatflow-peer="zone:b"] [data-heatflow-pair-out="3"]'),'sampled-frame pair ledger failed to match exact native timestamp');
 const b=host.querySelector('g[data-heat-zone="B"]');b.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));
 check(state.simulationHeatFlowSelectedZone==='B'&&host.querySelector('g[data-heat-zone="B"]').getAttribute('aria-pressed')==='true','keyboard activation did not select plan zone');
 check(kpi('net').textContent.endsWith('*')&&host.querySelector('.heatflow-partial-note')&&kpi('residual').textContent==='—','unobserved zone-air term became zero or complete residual');
 const unavailable=host.querySelector('[data-heatflow-aggregate="outdoorAir"]');check(unavailable.dataset.value===''&&unavailable.classList.contains('unavailable')&&!/[←→]/.test(unavailable.querySelector('b').textContent),'unobserved outdoor exchange became a zero/direction');
 const bBarHeight=host.querySelector('g[data-heat-zone="B"] [data-heatflow-local-category="systemAir"]').getAttribute('height');
 state.simulationHeatFlowFrameIndex=2;render();
 check(host.querySelector('g[data-heat-zone="B"] [data-heatflow-local-category="systemAir"]').getAttribute('height')===bBarHeight&&Number(bBarHeight)>0,'equal -2000 W local values changed map bar height when other frame magnitudes changed');
 check(host.querySelector('[data-heatflow-scale="net"]').textContent===netScale,'map color bounds changed with current frame');
 state.simulationHeatFlowFrameIndex=1;render();
 host.querySelector('[data-heatflow-summary="A"] button').click();check(state.simulationHeatFlowSelectedZone==='A','zone summary button lost native selection');
 const transform=host.querySelector('[data-heatflow-plan-content]').getAttribute('transform');host.querySelector('[data-heatflow-plan-zoom="in"]').click();
 check(host.querySelector('[data-heatflow-plan-content]').getAttribute('transform')!==transform&&state.simulationHeatFlowFrameIndex===1,'plan zoom lost transform or changed frame');
 host.querySelector('[data-heatflow-plan-zoom="reset"]').click();
 const chartBefore=host.querySelector('[data-heatflow-history]').outerHTML;host.querySelector('[data-heatflow-inspector-toggle]').click();
 check(state.simulationHeatFlowInspectorCollapsed&&host.querySelector('[data-heatflow-history]').outerHTML===chartBefore,'collapsing ledger hid or changed full-width graph');
 host.querySelector('[data-heatflow-inspector-toggle]').click();
 for(const [name,mutate]of[
  ['missing execution snapshot',next=>delete next.purposeResults.thermalTopology.planGeometry],
  ['old overlay without observed flags',next=>next.purposeResults.thermalTopology.periods.forEach(period=>period.boundaryFlows.forEach(flow=>delete flow.observed))],
  ['missing native timestamp',next=>next.purposeResults.zoneHeatFlow.labels[1]='02-01 03:00'],
  ['duplicate native timestamp',next=>{const period=next.purposeResults.thermalTopology.periods.find(item=>item.id==='hourly');period.labels[1]=period.labels[2];}],
  ['masked native surface observation',next=>next.purposeResults.thermalTopology.periods.find(item=>item.id==='hourly').boundaryFlows.forEach(flow=>flow.observed[2]=false)]
 ]){
  const next=clone(result);mutate(next);state.report={geometry:fixture.geometry};render(next);check(arrows().length===0,name+' manufactured measured pair arrows');
  check(host.querySelector('[data-heatflow-aggregate="outdoorAir"]').dataset.value==='-1000',name+' removed independent aggregate zone observations');
 }
 render();state.simulationHeatFlowFrameIndex=2;render();
 check(arrows().length===0&&kpi('net').textContent==='0.000 kW'&&!kpi('net').textContent.includes('*'),'reported zero heat was treated as missing or created transfer arrow');
 check(host.querySelector('[data-heatflow-aggregate="interzoneAir"]').classList.contains('neutral'),'reported zero interzone transfer lacks neutral state');
 const absent=clone(dataset.zones[0]);absent.values[0][0]=null;const balance=data.heatFlowBalance(dataset,absent,0);
 check(!balance.complete&&Number.isNaN(balance.residual)&&Number.isNaN(data.heatFlowCategoryValue(absent,0,0)),'null numeric data became complete heat balance');
 state.simulationHeatFlowFrameIndex=0;render();
 const preset=(labels,frame,name)=>{
  const next=clone(result),series=next.purposeResults.zoneHeatFlow;series.labels=labels;series.frameCount=series.originalFrameCount=labels.length;
  series.zones.forEach(zone=>{zone.values=zone.values.map(values=>labels.map(()=>values[0]));zone.observed=zone.observed.map(()=>labels.map(()=>true));zone.temperature=labels.map(()=>22);zone.temperatureObserved=labels.map(()=>true);});
  state.simulationHeatFlowFrameIndex=frame;state.simulationHeatFlowRangeStart=0;state.simulationHeatFlowRangeEnd=-1;render(next);
  const button=host.querySelector('[data-heatflow-range-preset="'+name+'"]');button.click();
  return {start:state.simulationHeatFlowRangeStart,end:state.simulationHeatFlowRangeEnd,disabled:button.disabled};
 };
 const sampled=Array.from({length:9},(_,index)=>{const date=new Date(Date.UTC(2000,0,1,1+index*13));return '01-'+String(date.getUTCDate()).padStart(2,'0')+' '+String(date.getUTCHours()).padStart(2,'0')+':00';});
 let range=preset(sampled,4,'day');check(range.start===4&&range.end===4,'24-hour preset used 24 sampled indices instead of source calendar time: '+JSON.stringify(range));
 range=preset(sampled,4,'week');check(range.start===0&&range.end===8,'week preset did not use elapsed calendar hours');
 range=preset(['01-01 12:00','01-01 24:00','01-02 01:00','01-02 12:00'],2,'day');check(range.start===1&&range.end===3,'24:00 calendar rollover shifted or discarded observed time: '+JSON.stringify(range));
 range=preset(['01-01 01:00','01-01 14:00','07-21 03:00','07-21 16:00','01-01 01:00','01-01 14:00'],3,'week');check(range.start===2&&range.end===3,'range crossed backwards design-day/run sequence boundary: '+JSON.stringify(range));
 range=preset(['01-01 01:00','01-01 02:00','01-01 02:00','01-01 03:00'],1,'day');check(range.start===0&&range.end===1,'range crossed ambiguous duplicate timestamps');
 range=preset(['01-01 01:00','02-31 01:00','01-03 01:00'],1,'day');check(range.disabled&&range.start===0&&range.end===2,'invalid calendar time enabled elapsed-time preset');
 check(data.heatFlowFrameTime('01-01 24:00')===data.heatFlowFrameTime('01-02 00:00')&&Number.isNaN(data.heatFlowFrameTime('01-01 24:01')),'calendar parser changed EnergyPlus 24:00 meaning');
 state.simulationHeatFlowFrameIndex=0;state.simulationHeatFlowRangeStart=0;state.simulationHeatFlowRangeEnd=-1;render();
 for(const language of['en','ko'])for(const font of[11,18]){
  i18n.setLanguage(language);document.documentElement.style.setProperty('--graph-label-font-size',font+'px');render();await tick();
  for(const item of host.querySelectorAll('.heatflow-chart-tick')){const size=parseFloat(getComputedStyle(item).fontSize)*item.getScreenCTM().a;check(Math.abs(size-Math.max(12,font))<.04,'graph font is unreadable or ignores setting '+JSON.stringify({language,font,size}));}
  check(host.querySelector('[data-heatflow-summary="A"] button').textContent.includes('A'),'translated UI renamed source zone');
 }
 i18n.setLanguage('en');document.documentElement.style.setProperty('--graph-label-font-size','11px');render();
 window.heatFlowLedgerReview=()=>{state.report={geometry:fixture.geometry};state.simulationResult=result;state.simulationHeatFlowFrameIndex=0;state.simulationHeatFlowRangeStart=0;state.simulationHeatFlowRangeEnd=-1;state.simulationHeatFlowSelectedZone='A';state.simulationHeatFlowInspectorCollapsed=false;simulation.renderSimulation();};
 check(JSON.stringify(fixture)===before,'view mutated executed source geometry/numeric result');
 evidence.push('real IDF geometry + production canonical pair flow builder; exact sampled/native timestamps; kW transfer vs kWh interval; storage/deviation excluded; known zero and missing masks; snapshot ownership; keyboard/selection/zoom; EN/KO + configured physical graph fonts');
}catch(error){failures.push(error.stack||String(error));}
document.body.dataset.heatFlowLedgerStatus=failures.length?'failed':'passed';document.getElementById('heat-flow-ledger-result').textContent=JSON.stringify({failures,evidence});
</script>`
