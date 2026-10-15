package frontendchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestHVACOperationAvailabilityAndSequenceBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless HVAC operating availability and control sequence acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	index := readTestFile(t, "frontend/src/index.html")
	hvacMain := sliceBetween(index, `<section class="hvac-main"`, `<div id="hvacApplyDialog"`)
	harness := strings.Replace(hvacOperationBrowserHTML, "{{HVAC_MAIN}}", hvacMain, 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/hvac-operation", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, harness)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome,
		"--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check",
		"--window-size=1360,1000", "--virtual-time-budget=15000", "--user-data-dir="+t.TempDir(),
		"--dump-dom", server.URL+"/hvac-operation").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-hvac-operation-status="passed"`) {
		failure := string(output)
		if start := strings.Index(failure, `<pre id="result">`); start >= 0 {
			result := failure[start+len(`<pre id="result">`):]
			if end := strings.Index(result, `</pre>`); end >= 0 {
				failure = html.UnescapeString(result[:end])
			}
		}
		t.Fatalf("HVAC operation availability/sequence browser acceptance failed: %v\n%s", err, failure)
	}
}

// Carry actual parsed object identities, schedules and control relationships
// through Go -> JSON -> the selected-equipment UI. Hand-written profiles alone
// cannot detect missing source references or dropped transport fields.
func TestHVACOperationParsedModelBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("parsed HVAC operating profile browser acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	fixtures := map[string]any{}
	for _, fixture := range []struct{ key, path, schedules string }{
		{"plant", "frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf", `
Schedule:Compact, Review Dispatch Hours, Fraction, Through: 12/31, For: AllDays, Until: 08:00, 0, Until: 20:00, 1, Until: 24:00, 0;
`},
		{"furnace", "internal/simulation/testdata/energy_path_real_models/models/25.1/Furnace.idf", `
Schedule:Compact, Review Loop Hours, Fraction, Through: 12/31, For: AllDays, Until: 08:00, 0, Until: 20:00, 1, Until: 24:00, 0;
Schedule:Compact, Review Parent Hours, Fraction, Through: 12/31, For: AllDays, Until: 10:00, 0, Until: 18:00, 1, Until: 24:00, 0;
Schedule:Constant, Review Fan Off, Fraction, 0;
`},
	} {
		input, err := os.ReadFile(repoPath(fixture.path))
		if err != nil {
			t.Fatal(err)
		}
		doc, err := idf.Parse(string(input) + fixture.schedules)
		if err != nil {
			t.Fatal(err)
		}
		changed := 0
		for index := range doc.Objects {
			object := &doc.Objects[index]
			if len(object.Fields) == 0 {
				continue
			}
			name := object.Fields[0].Value
			switch {
			case fixture.key == "plant" && object.Type == "PlantEquipmentOperationSchemes" && name == "HeatSys1 Loop Operation Scheme List":
				object.Fields[3].Value = "Review Dispatch Hours"
				changed++
			case fixture.key == "plant" && object.Type == "Schedule:Compact" && name == "HW-Loop-Temp-Schedule":
				if len(object.Fields) != 6 {
					t.Fatal("real boiler temperature schedule field layout changed")
				}
				object.Fields[2].Value = "Through: 5/31"
				object.Fields = append(object.Fields, idf.Field{Value: "Through: 12/31"}, idf.Field{Value: "For: AllDays"}, idf.Field{Value: "Until: 24:00"}, idf.Field{Value: "65"})
				changed++
			case fixture.key == "furnace" && object.Type == "AvailabilityManager:Scheduled" && name == "Furnace 1 Avail":
				object.Fields[1].Value = "Review Loop Hours"
				changed++
			case fixture.key == "furnace" && object.Type == "AirLoopHVAC:Unitary:Furnace:HeatOnly" && name == "Gas Furnace 1":
				object.Fields[1].Value = "Review Parent Hours"
				changed++
			case fixture.key == "furnace" && object.Type == "Fan:OnOff" && name == "Supply Fan 1":
				object.Fields[1].Value = "Review Fan Off"
				changed++
			}
		}
		wantChanges := 2
		if fixture.key == "furnace" {
			wantChanges = 3
		}
		if changed != wantChanges {
			t.Fatalf("%s source schedule bindings changed: got %d, want %d", fixture.key, changed, wantChanges)
		}
		// Reparse the edited IDF so the tested report comes from the same input
		// boundary as the app, rather than an ad hoc report assembled in JS.
		doc, err = idf.Parse(doc.String())
		if err != nil {
			t.Fatal(err)
		}
		fixtures[fixture.key] = map[string]any{"hvac": idf.AnalyzeHVAC(doc), "objects": idf.AnalyzeOverview(doc).Objects}
	}
	index := readTestFile(t, "frontend/src/index.html")
	hvacMain := sliceBetween(index, `<section class="hvac-main"`, `<div id="hvacApplyDialog"`)
	harness := strings.Replace(hvacOperationParsedBrowserHTML, "{{HVAC_MAIN}}", hvacMain, 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/fixtures", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fixtures)
	})
	mux.HandleFunc("/hvac-parsed-operation", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, harness)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	runBrowser := func(action, query, viewport string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--force-device-scale-factor=1", "--window-size="+viewport, "--virtual-time-budget=15000", "--user-data-dir="+t.TempDir(), action, server.URL+"/hvac-parsed-operation"+query).CombinedOutput()
	}
	output, err := runBrowser("--dump-dom", "", "1480,1100")
	if err != nil || !strings.Contains(string(output), `data-hvac-parsed-operation-status="passed"`) {
		failure := string(output)
		if _, result, ok := strings.Cut(failure, `<pre id="result">`); ok {
			message, _, _ := strings.Cut(result, `</pre>`)
			failure = html.UnescapeString(message)
		}
		t.Fatalf("parsed HVAC operation browser acceptance failed: %v\n%s", err, failure)
	}
	if captureDir := os.Getenv("HVAC_OPERATION_CAPTURE_DIR"); captureDir != "" {
		if err := os.MkdirAll(captureDir, 0755); err != nil {
			t.Fatal(err)
		}
		for _, capture := range []struct{ name, query, viewport string }{
			{"boiler-wide.png", "?capture=wide", "1480,1400"},
			{"boiler-narrow.png", "?capture=narrow", "480,1800"},
			{"boiler-wide-ko.png", "?capture=wide&lang=ko", "1480,1400"},
			{"boiler-narrow-ko.png", "?capture=narrow&lang=ko", "480,1800"},
		} {
			path, err := filepath.Abs(filepath.Join(captureDir, capture.name))
			if err != nil {
				t.Fatal(err)
			}
			if output, err := runBrowser("--screenshot="+path, capture.query, capture.viewport); err != nil {
				t.Fatalf("HVAC operation screenshot %s failed: %v\n%s", path, err, output)
			}
			t.Logf("HVAC operation review: %s", path)
		}
	}
}

const hvacOperationBrowserHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>HVAC equipment operation acceptance</title>
<link rel="stylesheet" href="/src/styles/base.css"><link rel="stylesheet" href="/src/styles/hvac.css">
<style>
body{margin:0;padding:16px}#hvacSummary{width:1120px}.hvac-layout{display:block;width:1120px;height:900px}.hvac-main{width:1120px;height:900px}
</style></head><body data-hvac-operation-status="pending">
<span id="runtimeStatus"></span><div id="hvacSummary" class="hvac-summary"></div>
<div class="hvac-layout">{{HVAC_MAIN}}</div><pre id="result">pending</pre>
<script type="module">
const check=(condition,message)=>{if(!condition)throw Error(message)};
const settle=()=>new Promise(resolve=>setTimeout(resolve,0));
try{
 const {state}=await import('/src/js/state.js');
 const {setLanguage}=await import('/src/js/i18n.js');setLanguage('en');
 const view=await import('/src/js/views/hvac-views.js');
 const {evaluateHVACOperation:evaluate}=await import('/src/js/hvac-operation.js');
 const {connectHVACOperationDetail:connect}=await import('/src/js/views/hvac-operation-view.js');
 const component=(index,name,type,inlet,outlet)=>({objectIndex:index,objectName:name,objectType:type,exists:true,inletNode:inlet,outletNode:outlet});
 const boiler=component(10,'Boiler <img src=x onerror="window.__unsafe=1">','Boiler:HotWater','Pump outlet','Supply outlet');
 const pump=component(11,'Hot-water pump','Pump:VariableSpeed','Supply inlet','Pump outlet');
 const fan=component(20,'Supply fan','Fan:VariableVolume','Air inlet','Air outlet');
 const loop=(id,type,name,components)=>({id,type,name,objectIndex:id==='plant'?1:2,
  supplySide:{name:'Supply side',inletNode:components[0].inletNode,outletNode:components.at(-1).outletNode,connectors:[],branches:[{name:name+' supply',objectIndex:30,inletNode:components[0].inletNode,outletNode:components.at(-1).outletNode,components}]},
  demandSide:{name:'Demand side',inletNode:'Demand inlet',outletNode:'Demand outlet',connectors:[],branches:[]},relatedZones:[],relatedLoops:[],warnings:[]});
 const plant=loop('plant','PlantLoop','Hot water',[pump,boiler]),air=loop('air','AirLoopHVAC','Supply air',[fan]);
 const source=item=>({objectIndex:item.objectIndex,objectType:item.objectType||item.type,objectName:item.objectName||item.name});
 const constant=(name,value)=>({name,kind:'constant',objectType:'Schedule:Constant',status:'supported',value});
 const intervals=(start,end)=>[{untilMinute:start,value:0},{untilMinute:end,value:1},{untilMinute:1440,value:0}];
 const positive=name=>({op:'schedule_positive',scheduleName:name});
 const all=(...children)=>({op:'all',children});
 const unknown={op:'unknown',reason:'Weather-dependent manager requires runtime conditions.'};
 const schedules=[constant('On',1),constant('Off',0),constant('Negative',-1),
  {name:'Loop hours',kind:'compact',objectType:'Schedule:Compact',status:'supported',rules:[{startDay:1,endDay:366,selector:'AllDays',intervals:intervals(480,1200)}]},
  {name:'Boiler hours',kind:'compact',objectType:'Schedule:Compact',status:'supported',rules:[
   {startDay:1,endDay:152,selector:'Weekdays',intervals:intervals(600,1080)},
   {startDay:1,endDay:152,selector:'AllOtherDays',intervals:[{untilMinute:1440,value:0}]},
   {startDay:153,endDay:366,selector:'Weekdays',intervals:intervals(720,960)},
   {startDay:153,endDay:366,selector:'AllOtherDays',intervals:[{untilMinute:1440,value:0}]}]},
  {name:'External file',kind:'unsupported',objectType:'Schedule:File',status:'unknown',reason:'External schedule file is unavailable.'}];
 schedules.push(constant('Shared name',0),
  {name:'Shared name',kind:'day',objectType:'Schedule:Day:Interval',status:'supported',intervals:[{untilMinute:1440,value:1}]},
  {name:'Week',kind:'week',objectType:'Schedule:Week:Daily',status:'supported',weekDays:{monday:'Shared name'}},
  {name:'Annual reference',kind:'year',objectType:'Schedule:Year',status:'supported',periods:[{startDay:1,endDay:366,scheduleName:'Week'}]},
  constant('Duplicate',0),constant('Duplicate',1));
 const layer=(id,owner,role,expression)=>({id,owner:source(owner),role,expression});
 const sequence=(equipment,code)=>({nodes:[
  {id:'availability',kind:'availability',code:'availability',source:source(equipment)},
  {id:'request',kind:'demand',code:'load_request',source:source(plant)},
  {id:'setpoint',kind:'setpoint',code:'scheduled_setpoint',source:{objectIndex:50,objectType:'SetpointManager:Scheduled',objectName:'Supply <target> & reset'},parameters:[{name:'Control Variable',value:'Temperature',objectIndex:50,fieldIndex:1},{name:'Schedule Name',value:'HW setpoint',objectIndex:50,fieldIndex:2}]},
  {id:'equipment',kind:'equipment',code,source:source(equipment),parameters:[{name:'Flow mode',value:'LeavingSetpointModulated',objectIndex:equipment.objectIndex,fieldIndex:7}]}
 ],edges:[{from:'availability',to:'equipment',relation:'permits'},{from:'request',to:'equipment',relation:'requests'},{from:'setpoint',to:'equipment',relation:'modulates'}]});
 const boilerProfile={componentIndex:10,loopId:'plant',availabilityLayers:[layer('loop',plant,'loop',positive('Loop hours')),layer('equipment',boiler,'equipment',positive('Boiler hours'))],finalAvailability:all(positive('Loop hours'),positive('Boiler hours')),control:sequence(boiler,'boiler')};
 const pumpProfile={componentIndex:11,loopId:'plant',availabilityLayers:[layer('loop',plant,'loop',positive('Loop hours')),layer('equipment',pump,'equipment',positive('On'))],finalAvailability:all(positive('Loop hours'),positive('On')),control:sequence(pump,'pump')};
 const fanProfile={componentIndex:20,loopId:'air',availabilityLayers:[layer('loop',air,'loop',positive('On')),layer('equipment',fan,'equipment',positive('External file'))],finalAvailability:all(positive('On'),positive('External file')),control:sequence(fan,'fan')};
 const calendar={month:1,day:1,dayType:'Monday'};
 const evaluated=evaluate(boilerProfile,schedules,calendar);
 const finalValues=expression=>evaluate({...boilerProfile,finalAvailability:expression},schedules,calendar).final.values;
 const every=(values,expected)=>values.length===1440&&values.every(value=>value===expected);
 check(evaluated.layers.length===2&&evaluated.layers[0].id==='loop'&&evaluated.layers[1].id==='equipment','ancestor-to-equipment availability order changed');
 check(evaluated.layers[0].values[540]===1&&evaluated.layers[1].values[540]===0&&evaluated.final.values[540]===0,'local schedule did not gate the parent permission');
 check(evaluated.layers[0].values[1140]===1&&evaluated.layers[1].values[1140]===0&&evaluated.final.values[1140]===0,'late-day local schedule gate ignored');
 check(evaluated.final.values[599]===0&&evaluated.final.values[600]===1&&evaluated.final.values[1079]===1&&evaluated.final.values[1080]===0,'minute-level availability boundaries are shifted');
 check(every(finalValues(all(positive('Off'),positive('On'))),0),'enabled equipment escaped a disabled ancestor gate');
 check(every(finalValues(all(positive('Off'),unknown)),0),'known OFF plus unresolved gate must remain OFF');
 check(every(finalValues(all(positive('On'),unknown)),null),'unresolved gate fabricated availability');
 check(every(finalValues({op:'any',children:[positive('On'),unknown]}),1),'known ON alternative did not dominate an unknown alternative');
 check(every(finalValues({op:'any',children:[positive('Off'),unknown]}),null),'unknown alternative became a known denial');
 check(every(finalValues(positive('Missing schedule')),null)&&every(finalValues(positive('External file')),null),'missing/external schedule silently enabled equipment');
 check(every(finalValues(positive('Negative')),0),'negative schedule value treated as available');
 check(every(finalValues(positive('Shared name')),0)&&every(finalValues(positive('Annual reference')),1),'annual/week/day schedule references crossed namespaces');
 check(every(finalValues(positive('Duplicate')),null),'ambiguous duplicate schedule definitions selected an arbitrary value');
 const cycleOn={op:'manager_scheduled_on',scheduleName:'On'},forceOff={op:'manager_scheduled_off',scheduleName:'Off'};
 const managerList=(op,...children)=>({op,children});
 const permission=manager=>({op:'manager_gate',children:[manager]});
 check(every(finalValues(permission(managerList('air_managers',cycleOn,forceOff))),0),'air-loop ForceOff precedence was replaced by manager order');
 check(every(finalValues(permission(managerList('plant_managers',cycleOn,forceOff))),1),'plant-loop first actionable manager did not win');
 check(every(finalValues(permission(managerList('plant_managers',forceOff,cycleOn))),0),'plant-loop manager order was ignored');
 check(every(finalValues(permission(managerList('air_managers',{op:'manager_unknown'},forceOff))),0),'known air-loop ForceOff did not dominate an unknown manager');
 check(every(finalValues(permission(managerList('plant_managers',{op:'manager_unknown'},forceOff))),null),'unknown first plant manager was skipped as NoAction');
 check(every(finalValues({op:'fan_gate',target:'central_fan',children:[cycleOn,positive('Off')]}),1),'fan CycleOn manager did not override its own OFF schedule');
 check(every(finalValues({op:'fan_gate',target:'central_fan',children:[forceOff,positive('On')]}),0),'fan ForceOff manager did not override its own ON schedule');
 check(every(finalValues({op:'fan_gate',target:'central_fan',children:[{op:'manager_scheduled_off',scheduleName:'Negative'},positive('On')]}),1),'ScheduledOff incorrectly forces off at a negative nonzero value');
 check(every(finalValues({op:'fan_gate',target:'central_fan',children:[{op:'always',value:3},positive('On')]}),0)&&every(finalValues({op:'fan_gate',target:'zone_fan',children:[{op:'always',value:3},positive('Off')]}),1),'CycleOnZoneFansOnly failed to distinguish central and zone fans');
 const summer=evaluate(boilerProfile,schedules,{month:6,day:1,dayType:'Monday'});
 check(summer.final.values[719]===0&&summer.final.values[720]===1&&summer.final.values[960]===0,'seasonal rules lost their actual calendar boundary');
 check(every(evaluate(boilerProfile,schedules,{month:1,day:1,dayType:'Sunday'}).final.values,0),'AllOtherDays did not resolve the selected day type');
 const hvac={loops:[plant,air],airLoopCount:1,plantLoopCount:1,condenserLoopCount:0,nodeUsages:[],nodeOutputMonitors:[],
  operatingProfiles:[boilerProfile,pumpProfile,fanProfile],operatingSchedules:schedules,
  serviceModel:{zoneServices:[],systems:[],components:[],couplings:[],networks:[],navigation:{entities:[],links:[],byLoop:{}}}};
 const original=JSON.stringify(hvac);
 state.report={hvac};state.activeResultTab='hvac';state.activeHVACView='loop';state.activeHVACLoopId='plant';state.activeHVACGraphKey='';state.activeHVACNodeName='';
 state.activeHVACEntity={id:'',kind:'',label:''};state.activeHVACContext={pathId:'',zoneId:'',loopId:'',componentId:'',couplingId:'',previousView:''};state.hvacNavigationStack=[];state.hvacForwardStack=[];
 view.initializeHVACControls();view.renderHVAC(hvac);await settle();
 const graph=document.querySelector('#hvacGraph');
 const findEquipment=item=>[...graph.querySelectorAll('.hvac-loop-equipment[data-hvac-graph-key]')].find(node=>node.textContent.includes(item.objectName));
 const choose=async(item,keyboard=false)=>{const target=findEquipment(item);check(target,'missing diagram equipment '+item.objectName);target.focus();target.dispatchEvent(keyboard?new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}):new MouseEvent('click',{bubbles:true,cancelable:true}));await settle();};
 const detail=()=>graph.querySelector('[data-hvac-operation-detail]');
 const finalPlot=()=>detail()?.querySelector('[data-hvac-availability-final] [data-hvac-availability-plot]');
 const segments=plot=>[...plot.querySelectorAll('[data-availability-value]')].map(rect=>[Number(rect.dataset.startMinute),Number(rect.dataset.endMinute),rect.dataset.availabilityValue]);
 const expectSegments=(plot,expected,message)=>check(plot&&JSON.stringify(segments(plot))===JSON.stringify(expected),message+': '+JSON.stringify(plot&&segments(plot)));
 await choose(boiler);
 check(detail()?.dataset.componentIndex==='10','equipment click failed to open its operating detail');
 check(!graph.querySelector('.hvac-loop-service-overview,.hvac-loop-related-paths,.hvac-supporting-assets,.hvac-detail-grid,.hvac-cross-loop-map'),'obsolete selected-equipment output remains below the loop');
 check(detail().textContent.includes(boiler.objectName)&&detail().textContent.includes('Supply <target> & reset'),'source names were lost while escaping');
 check(!detail().querySelector('img,target')&&!window.__unsafe,'source name was interpreted as markup');
 const rows=[...detail().querySelectorAll('[data-hvac-availability-layer]')];
 check(rows.map(row=>row.dataset.hvacAvailabilityLayer).join('|')==='loop|equipment','availability hierarchy is not top-down');
 const finalRow=detail().querySelector('[data-hvac-availability-final]');
 check(Boolean(rows.at(-1).compareDocumentPosition(finalRow)&Node.DOCUMENT_POSITION_FOLLOWING),'final availability is not last');
 expectSegments(rows[0].querySelector('[data-hvac-availability-plot]'),[[0,480,'0'],[480,1200,'1'],[1200,1440,'0']],'parent plot omitted its schedule');
 expectSegments(rows[1].querySelector('[data-hvac-availability-plot]'),[[0,600,'0'],[600,1080,'1'],[1080,1440,'0']],'equipment plot omitted its schedule');
 expectSegments(finalPlot(),[[0,600,'0'],[600,1080,'1'],[1080,1440,'0']],'combined final plot is wrong');
 const plots=[...detail().querySelectorAll('[data-hvac-availability-plot]')];
 check(plots.length===3&&new Set(plots.map(plot=>plot.getAttribute('viewBox'))).size===1&&plots.every(plot=>segments(plot)[0][0]===0&&segments(plot).at(-1)[1]===1440),'availability plots do not share a complete 24-hour axis');
 const plotAxis=plot=>{const point=plot.createSVGPoint();point.x=36;const start=point.matrixTransform(plot.getScreenCTM()).x;point.x=480;return[start,point.matrixTransform(plot.getScreenCTM()).x]};
 const axes=plots.map(plotAxis);
 check(axes.every(axis=>Math.abs(axis[0]-axes[0][0])<1&&Math.abs(axis[1]-axes[0][1])<1),'final and ancestor plot time axes are not vertically aligned');
 check(detail().querySelectorAll('[data-hvac-control-node]').length===4&&detail().querySelectorAll('[data-hvac-control-edge]').length===3,'control dependencies are not drawn');
 check(detail().querySelector('[data-hvac-control-node="equipment"] [data-control-value="LeavingSetpointModulated"]'),'equipment parameter control missing');
 const checkRoutes=()=>{
  const boxes=[...detail().querySelectorAll('[data-hvac-control-node]')].map(node=>({id:node.dataset.hvacControlNode,rect:node.getBoundingClientRect()}));
  for(const edge of detail().querySelectorAll('[data-hvac-control-edge]')){
   const path=edge.querySelector('path');check(path?.getAttribute('d'),'control relationship has no rendered connector');
   const length=path.getTotalLength(),matrix=path.getScreenCTM();
   for(let sample=1;sample<80;sample++){
    const point=path.getPointAtLength(length*sample/80).matrixTransform(matrix);
    const obstacle=boxes.find(({id,rect})=>id!==edge.dataset.from&&id!==edge.dataset.to&&point.x>rect.left+2&&point.x<rect.right-2&&point.y>rect.top+2&&point.y<rect.bottom-2);
    check(!obstacle,'control arrow crosses '+obstacle?.id+': '+edge.dataset.from+' -> '+edge.dataset.to+' at '+Math.round(point.x)+','+Math.round(point.y)+' path '+path.getAttribute('d'));
   }
  }
 };
 checkRoutes();
 const availability=()=>detail().querySelector('[data-hvac-operation-availability]');
 const control=()=>detail().querySelector('[data-hvac-operation-sequence]');
 let left=availability().getBoundingClientRect(),right=control().getBoundingClientRect();
 check(left.width>300&&right.width>300&&Math.abs(left.top-right.top)<2&&left.right<=right.left+1,'wide loop panel does not show availability left and control sequence right');
 const changeCalendar=async(field,value)=>{const input=detail().querySelector('[data-hvac-operation-calendar="'+field+'"]');check(input,'missing calendar control '+field);input.value=String(value);input.dispatchEvent(new Event('change',{bubbles:true}));await settle();};
 check(['month','day','dayType'].every(field=>detail().querySelector('[data-hvac-operation-calendar="'+field+'"]')),'availability calendar controls missing');
 await changeCalendar('month',6);await changeCalendar('day',1);
 expectSegments(finalPlot(),[[0,720,'0'],[720,960,'1'],[960,1440,'0']],'date selection retained stale availability');
 await changeCalendar('dayType','Sunday');expectSegments(finalPlot(),[[0,1440,'0']],'day type selection retained weekday availability');
 await changeCalendar('month',1);await changeCalendar('dayType','Monday');
 await choose(pump,true);
 check(detail()?.dataset.componentIndex==='11'&&detail().textContent.includes(pump.objectName)&&!detail().textContent.includes(boiler.objectName),'keyboard selection retained the previous component');
 expectSegments(finalPlot(),[[0,480,'0'],[480,1200,'1'],[1200,1440,'0']],'pump selection retained boiler chart');
 for(const element of [document.querySelector('.hvac-layout'),document.querySelector('.hvac-main'),document.querySelector('#hvacSummary')])element.style.width='400px';
 await settle();left=availability().getBoundingClientRect();right=control().getBoundingClientRect();
 check(right.top>=left.bottom-1&&Math.abs(left.left-right.left)<2,'narrow loop panel does not stack availability before control sequence');
 check(detail().scrollWidth<=detail().clientWidth+1&&graph.scrollWidth<=graph.clientWidth+1,'narrow operating detail overflows horizontally');
 // dump-DOM virtual time does not guarantee a paint/rAF after forced resize.
 // Reconnect at the resulting width to verify connector geometry directly.
 connect(graph);checkRoutes();
 view.navigateHVAC({view:'loop',loopID:'air',graphKey:'',context:{}},{pushHistory:true});await settle();await choose(fan);
 check(detail()?.dataset.componentIndex==='20'&&detail().textContent.includes(fan.objectName),'fan family does not get the shared operating detail');
 expectSegments(finalPlot(),[[0,1440,'unknown']],'unavailable fan schedule fabricated a permission plot');
 check(detail().querySelector('[data-hvac-control-node="equipment"]'),'fan has no control sequence');
 check(JSON.stringify(hvac)===original,'operation inspection mutated the analyzed report');
 document.body.dataset.hvacOperationStatus='passed';document.querySelector('#result').textContent='passed';
}catch(error){document.body.dataset.hvacOperationStatus='failed';document.querySelector('#result').textContent=error.stack||String(error)}
</script></body></html>`

const hvacOperationParsedBrowserHTML = `<!doctype html>
<html lang="en" data-theme="dark"><head><meta charset="utf-8"><title>Parsed HVAC operating conditions</title>
<link rel="stylesheet" href="/src/styles/base.css"><link rel="stylesheet" href="/src/styles/hvac.css">
<style>body{display:block;margin:0;padding:16px;overflow:auto;height:auto}#hvacSummary{width:1400px}.hvac-layout{display:block;width:1400px;height:1300px}.hvac-main{width:1400px;height:1300px}#result{display:none}</style>
</head><body data-hvac-parsed-operation-status="pending"><span id="runtimeStatus"></span><div id="hvacSummary" class="hvac-summary"></div><div class="hvac-layout">{{HVAC_MAIN}}</div><pre id="result">pending</pre>
<script type="module">
const check=(value,message)=>{if(!value)throw Error(message)};
const settle=()=>new Promise(resolve=>setTimeout(resolve,60));
try{
 const [{state},view,{evaluateHVACOperation:evaluate},{renderHVACOperationDetail:renderDetail,connectHVACOperationDetail:connect},{setLanguage}]=await Promise.all([
  import('/src/js/state.js'),import('/src/js/views/hvac-views.js'),import('/src/js/hvac-operation.js'),import('/src/js/views/hvac-operation-view.js'),import('/src/js/i18n.js')]);
 const locale=new URLSearchParams(location.search).get('lang')||'en';setLanguage(locale);
 const fixtures=await(await fetch('/fixtures')).json(),original=JSON.stringify(fixtures),calendar={month:1,day:1,dayType:'Monday'};
 const graph=document.querySelector('#hvacGraph'),detail=()=>graph.querySelector('[data-hvac-operation-detail]');
 const source=(fixture,type,name)=>fixture.objects.find(object=>object.type===type&&object.name===name);
 const profile=(fixture,object,loop)=>fixture.hvac.operatingProfiles.find(item=>item.componentIndex===object.index&&item.loopId===loop.id);
 const setup=async(fixture,loop)=>{
  state.report=fixture;state.activeResultTab='hvac';state.activeHVACView='loop';state.activeHVACLoopId=loop.id;state.activeHVACGraphKey='';state.activeHVACNodeName='';
  state.activeHVACEntity={id:'',kind:'',label:''};state.activeHVACContext={pathId:'',zoneId:'',loopId:'',componentId:'',couplingId:'',previousView:''};state.hvacNavigationStack=[];state.hvacForwardStack=[];
  view.renderHVAC(fixture.hvac);await settle();
 };
 const select=async(name)=>{
  const node=[...graph.querySelectorAll('.hvac-loop-equipment[data-hvac-graph-key]')].find(item=>item.textContent.includes(name));
  check(node,'parsed equipment is absent from its real loop diagram: '+name);
  node.dispatchEvent(new MouseEvent('click',{bubbles:true,cancelable:true}));await settle();
  check(detail()?.textContent.includes(name),'parsed equipment selection did not render '+name);
 };
 const valueAt=(plot,minute)=>[...plot.querySelectorAll('[data-availability-value]')].find(item=>Number(item.dataset.startMinute)<=minute&&Number(item.dataset.endMinute)>minute)?.dataset.availabilityValue;
 const finalPlot=()=>detail().querySelector('[data-hvac-availability-final] [data-hvac-availability-plot]');
 const checkBranchLabels=()=>{
  const labels=[...detail().querySelectorAll('[data-hvac-control-edge] > text')].map(node=>({text:node.textContent,rect:node.getBoundingClientRect()}));
  check(labels.length>=3&&labels.every(label=>label.rect.width>0&&label.rect.height>0),'boiler branch labels are missing or invisible');
  for(let first=0;first<labels.length;first++)for(let next=first+1;next<labels.length;next++){
   const a=labels[first],b=labels[next];
   check(Math.min(a.rect.right,b.rect.right)-Math.max(a.rect.left,b.rect.left)<.5||Math.min(a.rect.bottom,b.rect.bottom)-Math.max(a.rect.top,b.rect.top)<.5,'boiler branch labels overlap: '+a.text+' / '+b.text);
  }
 };
 const plant=fixtures.plant,water=plant.hvac.loops.find(loop=>loop.name==='HeatSys1');check(water,'real hot-water loop missing');
 const boiler=source(plant,'Boiler:HotWater','HeatSys1 Boiler');check(boiler,'real boiler source missing');
 const boilerProfile=profile(plant,boiler,water);check(boilerProfile,'real boiler operating profile missing');
 const boilerModel=evaluate(boilerProfile,plant.hvac.operatingSchedules,calendar);
 check(boilerProfile.availabilityLayers.some(layer=>layer.role==='plant_dispatch'&&layer.owner.objectName==='HeatSys1 Loop Operation Scheme List'),'actual operation-scheme ancestor was not transported');
 check(boilerModel.final.values[420]===0&&boilerModel.final.values[720]===null&&boilerModel.final.values[1260]===0,'real heating-load dispatch fabricated ON or lost scheduled OFF');
 const setpoint=boilerProfile.control.nodes.find(node=>node.source.objectName==='HeatSys1 Boiler Setpoint Manager');
 const operation=boilerProfile.control.nodes.find(node=>node.source.objectName==='HeatSys1 Operation Scheme');
 const targetSchedule=boilerProfile.control.nodes.find(node=>node.kind==='schedule'&&node.source.objectName==='HW-Loop-Temp-Schedule');
 check(setpoint&&operation&&boilerProfile.control.edges.some(edge=>edge.from===setpoint.id)&&boilerProfile.control.edges.some(edge=>edge.from===operation.id),'real setpoint and operation scheme lost their control relationships');
 check(boilerProfile.control.nodes.some(node=>node.parameters?.some(parameter=>parameter.value==='LeavingSetpointModulated')),'real boiler flow-mode control parameter missing');
 view.initializeHVACControls();await setup(plant,water);await select(boiler.name);
 check(detail().dataset.componentIndex===String(boiler.index),'parsed object identity changed at the selection boundary');
 check(valueAt(finalPlot(),420)==='0'&&valueAt(finalPlot(),720)==='unknown'&&valueAt(finalPlot(),1260)==='0','real-source final permission plot differs from evaluated dispatch');
 check(boilerProfile.control.nodes.every(node=>detail().querySelector('[data-hvac-control-node="'+CSS.escape(node.id)+'"]')),'real control nodes vanished in the UI');
 check(targetSchedule,'real boiler target schedule is absent from its control graph');
 const targetValue=()=>detail().querySelector('[data-hvac-control-node="'+CSS.escape(targetSchedule.id)+'"] .hvac-control-schedule-value')?.textContent;
 check(/82[.,]2\s*°C/.test(targetValue()||''),'boiler schedule card lost the actual 82.2 °C winter target');
 const selectMonth=async(month)=>{const input=detail().querySelector('[data-hvac-operation-calendar="month"]');input.value=month;input.dispatchEvent(new Event('change',{bubbles:true}));await settle();};
 await selectMonth(6);check(/65\s*°C/.test(targetValue()||''),'selected summer date did not update the boiler target to 65 °C');
 await selectMonth(1);check(/82[.,]2\s*°C/.test(targetValue()||''),'returning to winter retained the summer target');
 check([...detail().querySelectorAll('[data-hvac-control-edge] path')].every(path=>path.getAttribute('d')?.startsWith('M')),'real control graph did not draw its dependency arrows');
 checkBranchLabels();
 setLanguage('ko');view.renderHVAC(plant.hvac);await settle();checkBranchLabels();
 const koreanLabels=[...detail().querySelectorAll('[data-hvac-control-edge] > text')].map(node=>node.textContent);
 check(koreanLabels.includes('ON')&&koreanLabels.includes('요청 없음'),'Korean ON / no-request branches are not separately labeled');
 setLanguage(locale);view.renderHVAC(plant.hvac);await settle();
 check(!graph.querySelector('.hvac-loop-service-overview,.hvac-loop-related-paths,.hvac-supporting-assets,.hvac-detail-grid'),'real selection retained obsolete lower output');
 const pump=water.supplySide.branches.flatMap(branch=>branch.components).find(component=>component.objectType.startsWith('Pump:'));
 check(pump,'real boiler-loop pump missing');await select(pump.objectName);
 check(!detail().textContent.includes('HeatSys1 Operation Scheme'),'pump inherited boiler-only load dispatch');
 check(valueAt(finalPlot(),420)==='1','pump incorrectly inherited the source equipment operation schedule');
 const furnace=fixtures.furnace,air=furnace.hvac.loops.find(loop=>loop.name==='Typical Terminal Reheat 1');check(air,'real furnace loop missing');
 const unit=source(furnace,'AirLoopHVAC:Unitary:Furnace:HeatOnly','Gas Furnace 1'),fan=source(furnace,'Fan:OnOff','Supply Fan 1');
 check(unit&&fan,'real furnace wrapper/fan sources missing');
 const fanProfile=profile(furnace,fan,air);check(fanProfile,'nested fan profile missing from real parsed input');
 const fanModel=evaluate(fanProfile,furnace.hvac.operatingSchedules,calendar);
 check(fanModel.layers.map(layer=>layer.owner.objectName).join('|')==='Typical Terminal Reheat 1|Gas Furnace 1|Supply Fan 1','nested fan lost its actual loop→unitary→fan hierarchy');
 check(fanModel.layers[0].values[540]===1&&fanModel.layers[1].values[540]===0&&fanModel.layers[2].values[540]===1&&fanModel.final.values[540]===0,'parent OFF failed to gate a fan forced ON by the air-loop manager');
 check(fanModel.final.values[720]===1&&fanModel.final.values[1140]===0,'real nested fan schedule composition is incorrect');
 await setup(furnace,air);await select(unit.name);
 check(valueAt(finalPlot(),540)==='0'&&valueAt(finalPlot(),720)==='1','unitary wrapper selection ignored its configured availability');
 // The physical loop selects the containing furnace; its internal fan uses
 // the same renderer when inspected with its source-backed occurrence profile.
 graph.innerHTML=renderDetail(fanProfile,furnace.hvac.operatingSchedules,{component:{objectIndex:fan.index,objectType:fan.type,objectName:fan.name},calendar});connect(graph);await settle();
 check(detail().querySelectorAll('[data-hvac-availability-layer]').length===3&&valueAt(finalPlot(),540)==='0'&&valueAt(finalPlot(),720)==='1','nested fan profile could not be rendered as the shared availability graph');
 check(JSON.stringify(fixtures)===original,'real-source operating inspection mutated its report');
 const capture=new URLSearchParams(location.search).get('capture');
 if(capture){
  await setup(plant,water);await select(boiler.name);
  const width=capture==='narrow'?'400px':'1400px';
  for(const node of [document.querySelector('.hvac-layout'),document.querySelector('.hvac-main'),document.querySelector('#hvacSummary')])node.style.width=width;
  if(capture==='narrow'){document.querySelector('.hvac-layout').style.height='1700px';document.querySelector('.hvac-main').style.height='1700px';}
  await settle();detail().scrollIntoView({block:'start'});await settle();
 }
 document.body.dataset.hvacParsedOperationStatus='passed';document.querySelector('#result').textContent='passed';
}catch(error){document.body.dataset.hvacParsedOperationStatus='failed';document.querySelector('#result').textContent=error.stack||String(error)}
</script></body></html>`
