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
Schedule:Compact, Review Parent Hours, Fraction, Through: 5/31, For: AllDays, Until: 10:00, 0, Until: 18:00, 1, Until: 24:00, 0, Through: 8/31, For: AllDays, Until: 24:00, 0, Through: 12/31, For: AllDays, Until: 10:00, 0, Until: 18:00, 1, Until: 24:00, 0;
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
			{"boiler-wide.png", "?capture=wide", "1480,1800"},
			{"boiler-narrow.png", "?capture=narrow", "480,1800"},
			{"boiler-wide-ko.png", "?capture=wide&lang=ko", "1480,1800"},
			{"boiler-narrow-ko.png", "?capture=narrow&lang=ko", "480,1800"},
			{"water-coil-wide.png", "?capture=wide&equipment=coil", "1480,1500"},
			{"water-coil-narrow.png", "?capture=narrow&equipment=coil", "480,2600"},
			{"water-coil-wide-ko.png", "?capture=wide&equipment=coil&lang=ko", "1480,1500"},
			{"water-coil-narrow-ko.png", "?capture=narrow&equipment=coil&lang=ko", "480,2600"},
			{"furnace-annual-wide.png", "?capture=wide&equipment=furnace&time=year", "1480,1400"},
			{"furnace-annual-narrow.png", "?capture=narrow&equipment=furnace&time=year", "480,1600"},
			{"furnace-annual-wide-ko.png", "?capture=wide&equipment=furnace&time=year&lang=ko", "1480,1400"},
			{"furnace-annual-narrow-ko.png", "?capture=narrow&equipment=furnace&time=year&lang=ko", "480,1600"},
			{"vav-availability-wide.png", "?capture=wide&equipment=vav&time=year", "1480,700"},
			{"vav-availability-narrow-ko.png", "?capture=narrow&equipment=vav&time=year&lang=ko", "480,800"},
			{"chiller-availability-wide.png", "?capture=wide&equipment=chiller&time=year", "1480,700"},
			{"chiller-availability-wide-ko.png", "?capture=wide&equipment=chiller&time=year&lang=ko", "1480,700"},
			{"chiller-availability-narrow-ko.png", "?capture=narrow&equipment=chiller&time=year&lang=ko", "480,800"},
		} {
			if filter := os.Getenv("HVAC_OPERATION_CAPTURE_FILTER"); filter != "" {
				matched := false
				for _, name := range strings.Split(filter, ",") {
					matched = matched || strings.Contains(capture.name, strings.TrimSpace(name))
				}
				if !matched {
					continue
				}
			}
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
 const definitions=(equipment,names)=>({nodes:[...names.map((name,index)=>({id:'owner:'+index,kind:'definition',code:'availability_owner',source:source(index===0?(equipment===fan?air:plant):equipment),parameters:[{name:'Availability Schedule Name',value:name}]})),...names.map((name,index)=>({id:'schedule:'+index,kind:'schedule',code:'schedule_value',source:{objectIndex:60+index,objectType:'Schedule:Compact',objectName:name}}))],edges:names.map((_,index)=>({from:'owner:'+index,to:'schedule:'+index,relation:'declares_schedule'}))});
 const onOff=equipment=>({nodes:[{id:'permission',kind:'decision',code:'availability_permission',source:source(equipment)},{id:'decision',kind:'decision',code:'boiler_on_off',source:source(equipment)},{id:'on',kind:'outcome',code:'on_command',boundary:'output',source:source(equipment)},{id:'off',kind:'outcome',code:'off_command',boundary:'output',source:source(equipment)}],edges:[{from:'permission',to:'decision',relation:'permits',condition:'yes'},{from:'permission',to:'off',relation:'blocked',condition:'no'},{from:'decision',to:'on',relation:'requested',condition:'yes'},{from:'decision',to:'off',relation:'no_request',condition:'no'}]});
 const parameters=equipment=>({nodes:[{id:'input',kind:'external_input',code:'inlet_state',boundary:'input',source:source(plant)},{id:'setpoint',kind:'setpoint',code:'setpoint_manager',source:{objectIndex:50,objectType:'SetpointManager:Scheduled',objectName:'Supply <target> & reset'}},{id:'controller',kind:'controller',code:'feedback_controller',source:source(equipment),parameters:[{name:'Action',value:'Reverse'}]},{id:'command',kind:'control_variable',code:'water_flow_command',source:source(equipment)},{id:'equipment',kind:'equipment',code:'equipment_response',source:source(equipment),parameters:[{name:'Flow mode',value:'LeavingSetpointModulated'}]},{id:'state',kind:'state_variable',code:'outlet_state',source:source(equipment)},{id:'output',kind:'external_output',code:'outlet_state',boundary:'output',source:source(equipment)}],edges:[{from:'input',to:'equipment',relation:'inlet_conditions'},{from:'setpoint',to:'controller',relation:'sets_target',polarity:'negative'},{from:'controller',to:'command',relation:'determines_command'},{from:'command',to:'equipment',relation:'actuates'},{from:'equipment',to:'state',relation:'determines_state'},{from:'state',to:'controller',relation:'feedback',polarity:'unknown'},{from:'state',to:'output',relation:'outputs'}]});
 for(const [item,profile,names] of [[boiler,boilerProfile,['Loop hours','Boiler hours']],[pump,pumpProfile,['Loop hours','On']],[fan,fanProfile,['On','External file']]])Object.assign(profile,{availabilityDefinitions:definitions(item,names),configuredAvailabilityLayers:profile.availabilityLayers,configuredAvailability:profile.finalAvailability,onOff:onOff(item),parameterControl:parameters(item)});
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
 const availability=()=>detail()?.querySelector('[data-hvac-operation-availability]');
 const stack=()=>availability().querySelector('[data-hvac-timeline-id]');
 const rows=()=>[...stack().querySelectorAll('[data-hvac-timeline-row]')];
 const sourceRow=name=>rows().find(row=>row.querySelector('strong').textContent===name);
 const schedulePlot=name=>sourceRow(name)?.querySelector('svg');
 const segments=plot=>[...plot.querySelectorAll('[data-hvac-patch-status]')].map(patch=>[Number(patch.dataset.startMinute),Number(patch.dataset.endMinute),patch.dataset.hvacPatchStatus]);
 const expectSegments=(plot,expected,message)=>check(plot&&JSON.stringify(segments(plot))===JSON.stringify(expected),message+': '+JSON.stringify(plot&&segments(plot)));
 const finalRow=()=>rows().at(-1),finalPlot=()=>finalRow().querySelector('svg');
 await choose(boiler);
 check(detail()?.dataset.componentIndex==='10','equipment click failed to open its operating detail');
 check(detail().querySelectorAll('.hvac-operation-panel').length===3&&detail().querySelectorAll('[data-hvac-operation-graph]').length===3,'equipment must have exactly three separate graphs');
 check(!graph.querySelector('.hvac-loop-service-overview,.hvac-loop-related-paths,.hvac-supporting-assets,.hvac-detail-grid,.hvac-cross-loop-map'),'obsolete selected-equipment output remains below the loop');
 check(detail().textContent.includes(boiler.objectName)&&detail().textContent.includes('Supply <target> & reset'),'source names were lost while escaping');
 check(!detail().querySelector('img,target')&&!window.__unsafe,'source name was interpreted as markup');
 check(!detail().querySelector('details'),'source/parameter disclosure widgets remain in operating detail');
 check(availability().querySelectorAll('select').length===1&&!availability().querySelector('input,button,p,legend,.hvac-availability-legend,.hvac-availability-presence,b'),'availability contains calendar/reset/descriptive/numeric clutter');
 check(!detail().querySelector('[data-hvac-operation-calendar="month"],[data-hvac-operation-calendar="day"],[data-hvac-operation-calendar="dayType"],[data-hvac-operation-calendar="year"]'),'removed date controls remain visible');
 check(rows().length===3&&finalRow().dataset.hvacAvailabilityKind==='final','related availability inputs and final row are not stacked in order');
 check(sourceRow('Loop hours').querySelector('[data-hvac-availability-owner]')?.textContent===plant.name&&sourceRow('Boiler hours').querySelector('[data-hvac-availability-owner]')?.textContent===boiler.objectName,'leftmost owner names do not match source declarations');
 check(sourceRow('Loop hours').querySelector('[data-hvac-availability-owner]').dataset.objectIndex==='1'&&sourceRow('Boiler hours').querySelector('[data-hvac-availability-owner]').dataset.objectIndex==='10','owner source identities changed');
 check(finalRow().querySelector('[data-hvac-availability-owner]')?.dataset.objectIndex==='10','final configured availability lost selected equipment owner');
 expectSegments(schedulePlot('Loop hours'),[[0,480,'off'],[480,1200,'on'],[1200,1440,'off']],'parent availability patches wrong');
 expectSegments(schedulePlot('Boiler hours'),[[0,600,'off'],[600,1080,'on'],[1080,1440,'off']],'equipment availability patches wrong');
 expectSegments(finalPlot(),[[0,600,'off'],[600,1080,'on'],[1080,1440,'off']],'final availability is not the native combined permission');
 const hoverAt=(plot,minute,total=Number(stack().dataset.timelineTotal))=>{const bounds=plot.getBoundingClientRect();plot.dispatchEvent(new PointerEvent('pointermove',{clientX:bounds.left+2+(plot.clientWidth-4)*minute/total,bubbles:true}));};
 hoverAt(schedulePlot('Loop hours'),540);
 check(stack().querySelector('[data-hvac-timeline-hover]').textContent==='09:00','shared hover time wrong');
 check(rows().map(row=>row.dataset.hvacHoverValue).join('|')==='1|0|0','shared hover did not resolve every row at the same instant');
 check(rows().map(row=>row.querySelector('[data-hvac-row-status]').textContent).join('|')==='ON|OFF|Unavailable','hover statuses do not distinguish source and final permission');
 check(new Set([...stack().querySelectorAll('[data-hvac-crosshair]')].map(line=>line.getAttribute('x1'))).size===1&&[...stack().querySelectorAll('[data-hvac-crosshair]')].every(line=>line.getAttribute('visibility')==='visible'),'availability crosshair is not synchronized');
 const onoff=()=>detail().querySelector('[data-hvac-operation-onoff]');
 const parameterPanel=()=>detail().querySelector('[data-hvac-operation-parameters]');
 check(onoff().querySelectorAll('[data-condition="yes"]').length===2&&onoff().querySelectorAll('[data-condition="no"]').length===2,'on/off branches were not explicitly labeled');
 check(!onoff().querySelector('[data-hvac-node-kind="control_variable"],[data-hvac-node-kind="state_variable"]'),'modulation leaked into on/off decisions');
 check(['external_input','controller','setpoint','control_variable','state_variable','external_output'].every(kind=>parameterPanel().querySelector('[data-hvac-node-kind="'+kind+'"]')),'parameter graph lost input/control/state/output roles');
 check(parameterPanel().querySelector('[data-control-value="LeavingSetpointModulated"]')&&parameterPanel().querySelector('[data-control-value="Reverse"]'),'equipment/controller fields are missing');
 check(parameterPanel().querySelector('[data-polarity="negative"] > text')?.textContent==='\u2212'&&parameterPanel().querySelector('[data-polarity="unknown"] > text')?.textContent==='?','proven and unresolved signs are not distinguished');
 const checkRoutes=()=>{
  for(const operationGraph of detail().querySelectorAll('[data-hvac-operation-graph]')){
   const boxes=[...operationGraph.querySelectorAll('[data-hvac-control-node]')].map(node=>({id:node.dataset.hvacControlNode,rect:node.getBoundingClientRect()}));
   for(const edge of operationGraph.querySelectorAll('[data-hvac-control-edge]')){
    const path=edge.querySelector('path');check(path?.getAttribute('d'),'relationship has no rendered connector');
    const length=path.getTotalLength(),matrix=path.getScreenCTM();
    for(let sample=1;sample<80;sample++){
     const point=path.getPointAtLength(length*sample/80).matrixTransform(matrix);
     const obstacle=boxes.find(({id,rect})=>id!==edge.dataset.from&&id!==edge.dataset.to&&point.x>rect.left+2&&point.x<rect.right-2&&point.y>rect.top+2&&point.y<rect.bottom-2);
     check(!obstacle,'connector crossed unrelated node '+obstacle?.id+' for '+edge.dataset.from+' to '+edge.dataset.to);
    }
   }
  }
 };
 checkRoutes();
 let left=availability().getBoundingClientRect(),right=onoff().getBoundingClientRect();
 check(left.width>900&&right.width>900&&right.top>=left.bottom-1,'availability does not occupy the full readable width above control graphs');
 const changeView=async value=>{const input=detail().querySelector('[data-hvac-operation-calendar="timelineView"]');input.value=value;input.dispatchEvent(new Event('change',{bubbles:true}));await settle();};
 const setCalendar=async options=>{state.hvacOperationCalendar={year:2026,month:1,day:1,dayType:'Monday',timelineView:'day',...options};view.renderHVAC(hvac);await settle();};
 await setCalendar({month:6});
 expectSegments(finalPlot(),[[0,720,'off'],[720,960,'on'],[960,1440,'off']],'programmatic calendar retained stale availability');
 await setCalendar({dayType:'Sunday'});expectSegments(finalPlot(),[[0,1440,'off']],'programmatic day type retained weekday availability');
 await setCalendar({});await changeView('week');
 check(Number(stack().dataset.timelineTotal)===10080,'representative week is not seven exact days');
 check(stack().querySelector('.hvac-availability-time-axis').textContent.includes('Monday'),'representative week axis lost selected effective weekday');
 await changeView('year');
 check(Number(stack().dataset.timelineTotal)===525600&&!stack().querySelector('canvas,.heatmap'),'annual view is not a linear365-day timeline');
 let annualSVG=stack().querySelector('svg'),annualBounds=annualSVG.getBoundingClientRect();
 annualSVG.dispatchEvent(new WheelEvent('wheel',{deltaY:-450,clientX:annualBounds.left+annualBounds.width/2,bubbles:true,cancelable:true}));
 const zoomedSpan=Number(stack().dataset.timelineEnd)-Number(stack().dataset.timelineStart);
 check(zoomedSpan<525600&&zoomedSpan>10080,'annual wheel zoom did not change time range');
 const startBefore=Number(stack().dataset.timelineStart),plotHost=stack().querySelector('.hvac-availability-timelines');
 annualSVG.dispatchEvent(new PointerEvent('pointerdown',{pointerId:1,button:0,clientX:annualBounds.left+250,bubbles:true}));
 // Real pointer capture retargets moves to this host, outside the SVG subtree.
 plotHost.dispatchEvent(new PointerEvent('pointermove',{pointerId:1,clientX:annualBounds.left+180,bubbles:true}));
 plotHost.dispatchEvent(new PointerEvent('pointerup',{pointerId:1,clientX:annualBounds.left+180,bubbles:true}));
 check(Number(stack().dataset.timelineStart)>startBefore,'captured-pointer drag did not pan annual viewport');
 check(new Set([...stack().querySelectorAll('[data-hvac-crosshair]')].map(line=>line.getAttribute('x1'))).size===1,'pan lost shared time cursor');
 plotHost.dispatchEvent(new KeyboardEvent('keydown',{key:'Home',bubbles:true,cancelable:true}));
 check(Number(stack().dataset.timelineStart)===0&&Number(stack().dataset.timelineEnd)===525600,'keyboard reset did not restore annual range');
 await changeView('day');await choose(pump,true);
 check(detail()?.dataset.componentIndex==='11'&&!detail().textContent.includes(boiler.objectName),'keyboard selection retained previous equipment');
 expectSegments(schedulePlot('On'),[[0,1440,'on']],'pump selection retained boiler availability');
 for(const element of [document.querySelector('.hvac-layout'),document.querySelector('.hvac-main'),document.querySelector('#hvacSummary')])element.style.width='690px';
 await settle();connect(graph);checkRoutes();
 check(getComputedStyle(parameterPanel().querySelector('[data-hvac-operation-graph]')).gridAutoFlow!=='column','parameter graph did not use narrow layout near breakpoint');
 for(const element of [document.querySelector('.hvac-layout'),document.querySelector('.hvac-main'),document.querySelector('#hvacSummary')])element.style.width='400px';
 await settle();left=availability().getBoundingClientRect();right=onoff().getBoundingClientRect();
 check(right.top>=left.bottom-1&&Math.abs(left.left-right.left)<2,'narrow view does not stack graphs');
 check(detail().scrollWidth<=detail().clientWidth+1&&graph.scrollWidth<=graph.clientWidth+1,'narrow graphs overflow horizontally');
 connect(graph);checkRoutes();
 view.navigateHVAC({view:'loop',loopID:'air',graphKey:'',context:{}},{pushHistory:true});await settle();await choose(fan);
 check(detail()?.dataset.componentIndex==='20','fan did not get shared operation detail');
 expectSegments(schedulePlot('External file'),[[0,1440,'unknown']],'external schedule fabricated binary availability');
 const legacy={...fanProfile};delete legacy.availabilityDefinitions;delete legacy.onOff;delete legacy.parameterControl;
 const {renderHVACOperationDetail:renderDetail}=await import('/src/js/views/hvac-operation-view.js');
 const legacyDOM=document.createElement('div');legacyDOM.innerHTML=renderDetail(legacy,schedules,{component:fan,calendar});
 check(legacyDOM.querySelectorAll('[data-hvac-graph-unavailable]').length===2&&!legacyDOM.querySelector('[data-hvac-operation-graph="onoff"],[data-hvac-operation-graph="parameters"]'),'legacy mixed control was relabeled as a precise new graph');
 const opaque={...boilerProfile,finalAvailability:{op:'manager_unknown',reason:'runtime_manager'}};delete opaque.configuredAvailability;delete opaque.configuredAvailabilityLayers;
 const opaqueDOM=document.createElement('div');opaqueDOM.innerHTML=renderDetail(opaque,schedules,{component:boiler,calendar});connect(opaqueDOM);
 const opaqueFinal=opaqueDOM.querySelector('[data-hvac-availability-kind="final"]');
 check(opaqueFinal.title.includes('Analyze')&&opaqueFinal.querySelector('[data-hvac-patch-status="unknown"]')&&!opaqueFinal.querySelector('[data-hvac-patch-status="on"],[data-hvac-patch-status="off"]'),'legacy runtime-only report guessed configured availability or omitted reanalysis hover');
 const denseSchedule={name:'Minute values',objectType:'Schedule:Compact',kind:'compact',status:'supported',rules:[{startDay:1,endDay:366,selector:'AllDays',intervals:Array.from({length:1440},(_,index)=>({untilMinute:index+1,value:index%2?.5:0}))}]};
 const denseProfile={componentIndex:fan.objectIndex,availabilityDefinitions:{nodes:[{id:'dense',kind:'schedule',code:'schedule_value',source:{objectType:'Schedule:Compact',objectName:'Minute values'}}],edges:[]},configuredAvailabilityLayers:[{id:'dense',role:'equipment',owner:source(fan),expression:positive('Minute values')}],configuredAvailability:positive('Minute values'),finalAvailability:{op:'unknown'},onOff:fanProfile.onOff,parameterControl:fanProfile.parameterControl};
 const denseHost=document.createElement('div');denseHost.style.width='1000px';document.body.append(denseHost);
 denseHost.innerHTML=renderDetail(denseProfile,[denseSchedule],{component:fan,calendar:{year:2026,timelineView:'year'}});connect(denseHost);await settle();
 const denseGraph=denseHost.querySelector('[data-hvac-timeline-id]'),denseSVG=denseGraph.querySelector('svg');
 check(Number(denseSVG.dataset.sourceSegments)===525600,'minute-exact annual source evidence was reduced');
 check(Number(denseSVG.dataset.renderedSegments)<=denseSVG.clientWidth&&denseSVG.querySelectorAll('*').length<denseSVG.clientWidth*4+20,'dense annual patches create unbounded DOM');
 check(denseSVG.querySelector('[data-hvac-patch-status="mixed"][data-has-on="true"]'),'dense annual summary lost short positive intervals');
 const denseBounds=denseSVG.getBoundingClientRect();
 for(let index=0;index<12;index++)denseSVG.dispatchEvent(new WheelEvent('wheel',{deltaY:-450,clientX:denseBounds.left+2,bubbles:true,cancelable:true}));
 check(Number(denseGraph.dataset.timelineEnd)-Number(denseGraph.dataset.timelineStart)===60,'dense annual timeline cannot zoom to native minute details');
 denseSVG.dispatchEvent(new PointerEvent('pointermove',{clientX:denseBounds.left+2+(denseSVG.clientWidth-4)*1.5/60,bubbles:true}));
 check(denseGraph.querySelector('[data-hvac-timeline-row]').dataset.hvacHoverValue==='1'&&denseGraph.querySelector('[data-hvac-row-status]').textContent==='ON','shared hover lost exact native minute status');
 const tinySchedules=[
  {name:'Tiny positive',objectType:'Schedule:Compact',kind:'compact',status:'supported',rules:[{startDay:1,endDay:366,selector:'AllDays',intervals:[{untilMinute:720,value:0},{untilMinute:1440,value:.0001}]}]},
  constant('Known zero',0),
  {name:'Missing values',objectType:'Schedule:File',kind:'unsupported',status:'unknown'},
  {name:'Mixed signed values',objectType:'Schedule:Compact',kind:'compact',status:'supported',rules:[{startDay:1,endDay:366,selector:'AllDays',intervals:[{untilMinute:720,value:-100},{untilMinute:1440,value:.0001}]}]},
  {name:'Partial zero',objectType:'Schedule:Compact',kind:'compact',status:'supported',rules:[{startDay:1,endDay:366,selector:'Weekdays',intervals:[{untilMinute:1440,value:0}]}]},
 ];
 const tinyExpression=all(...tinySchedules.map(schedule=>positive(schedule.name)));
 const tinyProfile={...denseProfile,configuredAvailabilityLayers:[{id:'tiny',role:'equipment',owner:source(fan),expression:tinyExpression}],configuredAvailability:tinyExpression,availabilityDefinitions:{nodes:tinySchedules.map((schedule,index)=>({id:'tiny:'+index,kind:'schedule',code:'schedule_value',source:{objectType:schedule.objectType,objectName:schedule.name}})),edges:[]}};
 denseHost.innerHTML=renderDetail(tinyProfile,tinySchedules,{component:fan,calendar:{year:2026,dayType:'Monday',timelineView:'week'}});connect(denseHost);await settle();
 const tinyGraph=denseHost.querySelector('[data-hvac-timeline-id]'),tinyRows=[...tinyGraph.querySelectorAll('[data-hvac-timeline-row]')];
 const tinyRow=name=>tinyRows.find(row=>row.querySelector('strong').textContent===name),tinySVG=tinyRow('Tiny positive').querySelector('svg');
 const tinyBounds=tinySVG.getBoundingClientRect();
 tinySVG.dispatchEvent(new PointerEvent('pointermove',{clientX:tinyBounds.left+2+(tinySVG.clientWidth-4)*800/10080,bubbles:true}));
 check(tinyRow('Tiny positive').dataset.hvacHoverValue==='1'&&tinyRow('Mixed signed values').dataset.hvacHoverValue==='1','tiny positive source was treated as OFF');
 check(tinyRow('Known zero').dataset.hvacHoverValue==='0'&&tinyRow('Missing values').dataset.hvacHoverValue==='unknown','known zero and absent source merged');
 tinySVG.dispatchEvent(new PointerEvent('pointermove',{clientX:tinyBounds.left+2+(tinySVG.clientWidth-4)*(6*1440+800)/10080,bubbles:true}));
 check(tinyRow('Partial zero').dataset.hvacHoverValue==='unknown','partially unknown source became OFF');
 check(!denseHost.querySelector('details,input,button,.hvac-availability-presence,.hvac-availability-legend'),'minimal availability regained removed clutter');
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
 const [{state},view,{evaluateHVACOperation:evaluate},{renderHVACOperationDetail:renderDetail,connectHVACOperationDetail:connect},{setLanguage,t}]=await Promise.all([
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
 const availability=()=>detail().querySelector('[data-hvac-operation-availability]');
 const parameters=()=>detail().querySelector('[data-hvac-operation-parameters]');
 const onoff=()=>detail().querySelector('[data-hvac-operation-onoff]');
 const valueAt=(plot,minute)=>({on:'1',off:'0',unknown:'unknown'})[[...plot.querySelectorAll('[data-hvac-patch-status]')].find(item=>Number(item.dataset.startMinute)<=minute&&Number(item.dataset.endMinute)>minute)?.dataset.hvacPatchStatus];
 const scheduleCard=(panel,name)=>[...panel.querySelectorAll('[data-hvac-timeline-row]')].find(node=>node.querySelector('strong')?.textContent===name&&node.dataset.hvacAvailabilityKind==='schedule');
 const checkBranchLabels=()=>{
  const labels=[...onoff().querySelectorAll('[data-hvac-control-edge] > text')].map(node=>({text:node.textContent,rect:node.getBoundingClientRect()}));
  check(labels.length>=3&&labels.every(label=>label.rect.width>0&&label.rect.height>0),'branch labels missing/invisible');
  for(let first=0;first<labels.length;first++)for(let next=first+1;next<labels.length;next++){
   const a=labels[first],b=labels[next];
   check(Math.min(a.rect.right,b.rect.right)-Math.max(a.rect.left,b.rect.left)<.5||Math.min(a.rect.bottom,b.rect.bottom)-Math.max(a.rect.top,b.rect.top)<.5,'branch labels overlap: '+a.text+' / '+b.text);
  }
 };
 const checkThreeGraphs=profile=>{
  check(detail().querySelectorAll('.hvac-operation-panel').length===3&&detail().querySelectorAll('[data-hvac-operation-graph]').length===3,'parsed source must render exactly three graphs');
  for(const [key,mode] of [['onOff','onoff'],['parameterControl','parameters']]){
   const panel=detail().querySelector('[data-hvac-operation-'+mode+']');
   check(profile[key].nodes.every(node=>panel.querySelector('[data-hvac-control-node="'+CSS.escape(node.id)+'"]')),'parsed '+key+' node vanished');
   check(profile[key].edges.length===panel.querySelectorAll('[data-hvac-control-edge]').length,'parsed '+key+' relationship vanished');
   for(const node of profile[key].nodes)if(node.source.objectName)check(panel.querySelector('[data-hvac-control-node="'+CSS.escape(node.id)+'"] .hvac-control-source')?.textContent===node.source.objectName,'native source identity lost in '+key+': '+node.source.objectName);
  }
  for(const node of profile.parameterControl.nodes.filter(node=>node.code==='node_setpoint'))for(const parameter of node.parameters.filter(parameter=>/node.*name/i.test(parameter.name)))check(parameters().querySelector('[data-hvac-control-node="'+CSS.escape(node.id)+'"] [data-control-value="'+CSS.escape(parameter.value)+'"]'),'setpoint reference node identity lost from inline control fields');
  check(profile.configuredAvailability&&profile.configuredAvailabilityLayers,'parsed configured availability projection missing');
  const referenceNames=new Set();
  const references=expression=>{if(expression?.scheduleName)referenceNames.add(expression.scheduleName.trim().toLowerCase());for(const child of expression?.children||[])references(child);};
  for(const layer of profile.configuredAvailabilityLayers)references(layer.expression);
  const displayed=[...availability().querySelectorAll('[data-hvac-availability-kind="schedule"]')];
  check([...referenceNames].every(name=>displayed.some(row=>row.querySelector('strong').textContent.trim().toLowerCase()===name)),'configured availability schedule identity vanished');
  check(displayed.every(row=>referenceNames.has(row.querySelector('strong').textContent.trim().toLowerCase())),'runtime/control schedule leaked into configured availability');
  for(const owner of availability().querySelectorAll('[data-hvac-availability-owner]')){
   const native=state.report.objects.find(object=>String(object.index)===owner.dataset.objectIndex);
   check(native&&owner.textContent===native.name&&owner.dataset.objectType===native.type,'availability declaring owner lost native identity');
  }
  const final=[...availability().querySelectorAll('[data-hvac-timeline-row]')].at(-1);
  check(final.dataset.hvacAvailabilityKind==='final'&&final.querySelector('[data-hvac-availability-owner]')?.dataset.objectIndex===String(profile.componentIndex),'final configured availability is not last or lost equipment owner');
  const expected=evaluate({...profile,finalAvailability:profile.configuredAvailability},state.report.hvac.operatingSchedules,{...calendar,...state.hvacOperationCalendar}).final.values;
  for(const minute of [0,420,540,720,1140,1260])check(valueAt(final.querySelector('svg'),minute)===(expected[minute]===null?'unknown':String(expected[minute])),'displayed final differs from configured minimum at '+minute);
  check(availability().querySelectorAll('select').length===1&&!detail().querySelector('details,[data-hvac-operation-calendar="month"],[data-hvac-operation-calendar="year"]'),'removed disclosure/calendar controls returned');
  check([...detail().querySelectorAll('[data-hvac-control-edge] path')].every(path=>path.getAttribute('d')?.startsWith('M')),'parsed graph did not draw all relationships');
 };
 const checkParameterRoutes=()=>{
  const nodes=[...parameters().querySelectorAll('[data-hvac-control-node]')].map(node=>({id:node.dataset.hvacControlNode,rect:node.getBoundingClientRect()}));
  for(const edge of parameters().querySelectorAll('[data-hvac-control-edge]')){
   const path=edge.querySelector('path'),length=path.getTotalLength(),matrix=path.getScreenCTM();
   for(let index=1;index<100;index++){
    const point=path.getPointAtLength(length*index/100).matrixTransform(matrix);
    const obstacle=nodes.find(({id,rect})=>id!==edge.dataset.from&&id!==edge.dataset.to&&point.x>rect.left+2&&point.x<rect.right-2&&point.y>rect.top+2&&point.y<rect.bottom-2);
    check(!obstacle,'parsed feedback relationship crosses '+obstacle?.id+' for '+edge.dataset.from+' to '+edge.dataset.to);
   }
  }
 };
 const plant=fixtures.plant,water=plant.hvac.loops.find(loop=>loop.name==='HeatSys1');check(water,'real hot-water loop missing');
 const boiler=source(plant,'Boiler:HotWater','HeatSys1 Boiler');check(boiler,'real boiler source missing');
 const boilerProfile=profile(plant,boiler,water);check(boilerProfile,'real boiler operating profile missing');
 check(!boilerProfile.availabilityDefinitions.nodes.some(node=>['decision','controller','dispatch'].includes(node.kind)),'runtime algorithms leaked into availability definitions');
 check(!JSON.stringify(boilerProfile.configuredAvailability).includes('runtime_dispatch')&&!JSON.stringify(boilerProfile.configuredAvailability).includes('runtime_manager'),'runtime outcome polluted configured minimum availability');
 check(boilerProfile.onOff.nodes.some(node=>node.source.objectName==='HeatSys1 Operation Scheme'),'real plant dispatch lost its on/off role');
 const targetSchedule=boilerProfile.parameterControl.nodes.find(node=>node.kind==='schedule'&&node.source.objectName==='HW-Loop-Temp-Schedule');
 check(targetSchedule,'real boiler target schedule absent');
 view.initializeHVACControls();await setup(plant,water);await select(boiler.name);checkThreeGraphs(boilerProfile);
 check(detail().dataset.componentIndex===String(boiler.index),'parsed object identity changed at selection');
 const targetValue=()=>parameters().querySelector('[data-hvac-control-node="'+CSS.escape(targetSchedule.id)+'"] .hvac-control-schedule-value')?.textContent;
 check(/82[.,]2\s*°C/.test(targetValue()||''),'boiler schedule lost actual winter 82.2 °C target');
 const selectMonth=async(month)=>{state.hvacOperationCalendar={...calendar,year:2026,month};view.renderHVAC(plant.hvac);await settle();};
 await selectMonth(6);check(/65\s*°C/.test(targetValue()||''),'summer date did not update target');
 await selectMonth(1);check(/82[.,]2\s*°C/.test(targetValue()||''),'winter date retained summer target');
 checkBranchLabels();setLanguage('ko');view.renderHVAC(plant.hvac);await settle();checkBranchLabels();
 check([...onoff().querySelectorAll('[data-hvac-control-edge] > text')].some(node=>node.textContent==='예'),'Korean decision branches not localized');
 setLanguage(locale);view.renderHVAC(plant.hvac);await settle();
 const pump=water.supplySide.branches.flatMap(branch=>branch.components).find(component=>component.objectType.startsWith('Pump:'));
 check(pump,'real pump missing');await select(pump.objectName);
 check(!detail().textContent.includes('HeatSys1 Operation Scheme'),'pump inherited boiler-only dispatch');
 const chiller=plant.objects.find(object=>object.type.startsWith('Chiller:'));check(chiller,'real chiller source missing');
 const chillerOccurrence=plant.hvac.operatingProfiles.find(item=>item.componentIndex===chiller.index&&plant.hvac.loops.some(loop=>loop.id===item.loopId&&loop.type==='PlantLoop'));
 check(chillerOccurrence,'real chiller supply occurrence missing');
 const chillerLoop=plant.hvac.loops.find(loop=>loop.id===chillerOccurrence.loopId);
 await setup(plant,chillerLoop);await select(chiller.name);checkThreeGraphs(chillerOccurrence);
 const ownDefault=[...availability().querySelectorAll('[data-hvac-availability-kind="layer"]')].find(row=>row.querySelector('[data-hvac-availability-owner]')?.dataset.objectIndex===String(chiller.index)&&row.querySelector('strong').textContent===t('hvac.operation.noSeparateSchedule'));
 check(ownDefault,'chiller own availability definition silently vanished instead of showing its unrestricted default');
 check(valueAt(ownDefault.querySelector('svg'),720)==='1','known chiller no-separate-schedule default became unknown');
 const chillerFinal=[...availability().querySelectorAll('[data-hvac-timeline-row]')].at(-1);
 check(valueAt(chillerFinal.querySelector('svg'),720)==='1','runtime chiller load dispatch polluted configured minimum availability');

 const coilOccurrence=plant.hvac.operatingProfiles.find(item=>item.parameterControl.nodes.some(node=>node.code==='feedback_controller'&&node.source.objectType==='Controller:WaterCoil')&&plant.hvac.loops.some(loop=>loop.id===item.loopId&&loop.type==='AirLoopHVAC'));
 check(coilOccurrence,'actual Controller:WaterCoil occurrence missing');
 const coilLoop=plant.hvac.loops.find(loop=>loop.id===coilOccurrence.loopId),coil=plant.objects.find(object=>object.index===coilOccurrence.componentIndex);
 await setup(plant,coilLoop);await select(coil.name);checkThreeGraphs(coilOccurrence);
 checkParameterRoutes();
 check(parameters().querySelector('[data-hvac-node-kind="control_variable"]')&&parameters().querySelector('[data-hvac-node-kind="state_variable"]')&&parameters().querySelector('[data-hvac-node-kind="external_input"]')&&parameters().querySelector('[data-hvac-node-kind="external_output"]'),'water coil lost command/state or external I/O');
 check(parameters().querySelector('[data-polarity="positive"] > text')?.textContent==='+'&&parameters().querySelector('[data-polarity="negative"] > text')?.textContent==='−','native water-coil comparator signs missing');
 check(parameters().querySelector('[data-control-parameter="Action"]'),'water-coil Action evidence missing');
 check(parameters().textContent.includes('Controller:WaterCoil')&&parameters().querySelector('[data-control-parameter="Sensor Node Name"]')&&parameters().querySelector('[data-control-parameter="Actuator Node Name"]'),'water-coil actual source and inline ports missing');
 for(const width of ['690px','400px']){
  for(const node of [document.querySelector('.hvac-layout'),document.querySelector('.hvac-main'),document.querySelector('#hvacSummary')])node.style.width=width;
  await settle();connect(graph);checkParameterRoutes();
  check(detail().scrollWidth<=detail().clientWidth+1,'parsed water-coil graph overflows at '+width);
 }
 for(const node of [document.querySelector('.hvac-layout'),document.querySelector('.hvac-main'),document.querySelector('#hvacSummary')])node.style.width='1400px';
 const furnace=fixtures.furnace,air=furnace.hvac.loops.find(loop=>loop.name==='Typical Terminal Reheat 1');
 const unit=source(furnace,'AirLoopHVAC:Unitary:Furnace:HeatOnly','Gas Furnace 1'),fan=source(furnace,'Fan:OnOff','Supply Fan 1');
 const fanProfile=profile(furnace,fan,air);check(fanProfile,'nested fan profile missing');
 await setup(furnace,air);await select(unit.name);
 const parentSchedule=scheduleCard(availability(),'Review Parent Hours');
 check(parentSchedule&&valueAt(parentSchedule.querySelector('[data-hvac-schedule-plot]'),540)==='0'&&valueAt(parentSchedule.querySelector('[data-hvac-schedule-plot]'),720)==='1','unitary raw parent availability schedule missing');
 graph.innerHTML=renderDetail(fanProfile,furnace.hvac.operatingSchedules,{component:{objectIndex:fan.index,objectType:fan.type,objectName:fan.name},calendar});connect(graph);await settle();checkThreeGraphs(fanProfile);
 const rawFan=scheduleCard(availability(),'Review Fan Off');check(rawFan&&valueAt(rawFan.querySelector('[data-hvac-schedule-plot]'),720)==='0','availability definitions overrode literal fan schedule with manager algorithm');
 check(JSON.stringify(fixtures)===original,'real-source inspection mutated report');
 const captureParams=new URLSearchParams(location.search),capture=captureParams.get('capture'),capturingCoil=captureParams.get('equipment')==='coil',capturingFurnace=captureParams.get('equipment')==='furnace',capturingVAV=captureParams.get('equipment')==='vav',capturingChiller=captureParams.get('equipment')==='chiller';
 if(capture){
  state.hvacOperationCalendar={...calendar,year:2026,timelineView:captureParams.get('time')||'day'};
  await setup(capturingFurnace?furnace:plant,capturingFurnace?air:capturingCoil||capturingVAV?coilLoop:capturingChiller?chillerLoop:water);await select(capturingFurnace?unit.name:capturingCoil||capturingVAV?coil.name:capturingChiller?chiller.name:boiler.name);
  const width=capture==='narrow'?'400px':'1400px';
  for(const node of [document.querySelector('.hvac-layout'),document.querySelector('.hvac-main'),document.querySelector('#hvacSummary')])node.style.width=width;
  await settle();
  const inspection=detail(),result=document.querySelector('#result');
  document.body.replaceChildren(inspection,result);inspection.style.width=width;connect(document.body);await settle();
  if(capturingCoil){
   const parameterPanel=inspection.querySelector('[data-hvac-operation-parameters]');
   inspection.querySelector('.hvac-operation-columns').replaceChildren(parameterPanel);connect(document.body);await settle();
  }
  if(capturingFurnace||capturingVAV||capturingChiller){
   const availabilityPanel=inspection.querySelector('[data-hvac-operation-availability]');
   inspection.querySelector('.hvac-operation-columns').replaceChildren(availabilityPanel);connect(document.body);await settle();
   const strip=availabilityPanel.querySelector('.hvac-availability-timeline-plot'),bounds=strip.getBoundingClientRect();
   strip.dispatchEvent(new PointerEvent('pointermove',{clientX:bounds.left+2+(strip.clientWidth-4)*237600/525600,bubbles:true}));await settle();
  }
  window.scrollTo(0,0);await settle();
 }
 document.body.dataset.hvacParsedOperationStatus='passed';document.querySelector('#result').textContent='passed';
}catch(error){document.body.dataset.hvacParsedOperationStatus='failed';document.querySelector('#result').textContent=error.stack||String(error)}
</script></body></html>`
