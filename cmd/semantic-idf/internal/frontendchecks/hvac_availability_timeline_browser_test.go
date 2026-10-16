package frontendchecks

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestHVACAvailabilitySourceTimelineBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("exact HVAC availability source timeline acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/timeline", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, hvacAvailabilityTimelineBrowserHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/timeline").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-timeline-status="passed"`) {
		failure := string(output)
		if _, result, ok := strings.Cut(failure, `<pre id="result">`); ok {
			message, _, _ := strings.Cut(result, `</pre>`)
			failure = html.UnescapeString(message)
		}
		t.Fatalf("HVAC availability timeline acceptance failed: %v\n%s", err, failure)
	}
}

const hvacAvailabilityTimelineBrowserHTML = `<!doctype html><html><body data-timeline-status="pending"><pre id="result"></pre><script type="module">
try {
  const {buildHVACScheduleTimeline:build,buildHVACAvailabilityStack:stack}=await import('/src/js/hvac-availability-timeline.js');
  const {normalizeHVACOperationCalendar:normalize,evaluateHVACOperation:evaluate}=await import('/src/js/hvac-operation.js');
  const check=(condition,message)=>{if(!condition)throw new Error(message);};
  const node=(name,type='Schedule:Constant')=>({kind:'schedule',source:{objectName:name,objectType:type}});
  const constant=(name,value)=>({name,objectType:'Schedule:Constant',kind:'constant',status:'supported',value});
  const all=(value)=>[{untilMinute:1440,value}];
  const schedules=[constant('Off',0),constant('Negative',-2),
    {name:'Pulse',objectType:'Schedule:Compact',kind:'compact',status:'supported',rules:[{startDay:1,endDay:366,selector:'AllDays',intervals:[{untilMinute:367,value:0},{untilMinute:368,value:.5},{untilMinute:1440,value:0}]}]},
    {name:'Season',objectType:'Schedule:Compact',kind:'compact',status:'supported',rules:[{startDay:1,endDay:182,selector:'AllDays',intervals:all(0)},{startDay:183,endDay:366,selector:'AllDays',intervals:all(.75)}]},
    {name:'Week',objectType:'Schedule:Compact',kind:'compact',status:'supported',rules:[{startDay:1,endDay:366,selector:'Weekdays',intervals:all(1)},{startDay:1,endDay:366,selector:'Weekends',intervals:all(0)},{startDay:1,endDay:366,selector:'Holiday',intervals:all(.25)}]},
    {name:'Pulse',objectType:'Schedule:Day:Interval',kind:'day',status:'supported',intervals:all(0)},
    constant('Duplicate',0),constant('Duplicate',1),
    {name:'Missing weekends',objectType:'Schedule:Compact',kind:'compact',status:'supported',rules:[{startDay:1,endDay:366,selector:'Weekdays',intervals:all(0)}]}];
  const before=JSON.stringify(schedules), calendar={year:2025,month:1,day:1,dayType:'Monday'};
  const zero=build(node('Off'),schedules,calendar,{view:'year'});
  check(zero.totalMinutes===365*1440&&zero.segments.length===1&&zero.allOff===true&&zero.status==='known','Nonleap all-off evidence or RLE changed');
  check(build(node('Off'),schedules,{...calendar,year:2024},{view:'year'}).totalMinutes===366*1440,'Leap year lost Feb29');
  const pulse=build(node('Pulse','Schedule:Compact'),schedules,calendar,{view:'year'});
  check(pulse.positiveMinutes===365&&pulse.allOff===false&&pulse.maximum===.5&&pulse.resolutionMinutes===1,'Annual sampling erased a one-minute availability pulse or raw fraction');
  check(pulse.segments[1].startMinute===367&&pulse.segments[1].endMinute===368&&pulse.segments.length===731,'Native minute pulse boundaries shifted');
  check(build(node('Pulse','Schedule:Day:Interval'),schedules,calendar,{view:'day'}).allOff===true,'Day and annual schedule namespaces crossed');
  const negative=build(node('Negative'),schedules,calendar,{view:'week'});
  check(negative.minimum===-2&&negative.maximum===-2&&negative.allOff===true,'Raw negative schedule was normalized or treated as positive');
  for(const name of ['Missing','Duplicate']) {
    const missing=build(node(name),schedules,calendar,{view:'year'});
    check(missing.status==='unknown'&&missing.allOff===null&&missing.unknownMinutes===missing.totalMinutes,'Missing/duplicate became known all-off: '+name);
  }
  const partial=build(node('Missing weekends','Schedule:Compact'),schedules,calendar,{view:'year'});
  check(partial.status==='partial'&&partial.allOff===null&&partial.unknownMinutes>0,'Missing weekend evidence became all-off');
  const season=build(node('Season','Schedule:Compact'),schedules,calendar,{view:'year'});
  check(season.segments[1].startMinute===181*1440&&season.segments[1].value===.75,'Nonleap annual incorrectly used leap ordinal for July boundary');
  const week=build(node('Week','Schedule:Compact'),schedules,{...calendar,month:6,day:29},{view:'week'});
  check(week.totalMinutes===7*1440&&week.positiveMinutes===5*1440&&week.ticks[0].dayType==='Monday'&&week.ticks[1].dayType==='Tuesday'&&week.ticks[2].month===7&&week.ticks[2].day===1,'Representative week did not advance date and selected weekday together');
  const holiday=build(node('Week','Schedule:Compact'),schedules,{...calendar,dayType:'Holiday'},{view:'week'});
  check(holiday.segments[0].value===.25&&holiday.segments[0].endMinute===1440&&holiday.ticks[0].dayType==='Holiday','Special day escaped its selected first-day scope');
  check(build(node('Off'),schedules,calendar,{view:'year'})===zero,'Immutable schedule timeline cache missed');
  check(JSON.stringify(schedules)===before,'Timeline builder mutated source definitions');
  check(normalize({...calendar,month:2,day:31,timelineView:'year'}).day===28&&normalize({...calendar,year:2024,month:2,day:31}).day===29,'Calendar bounds ignore selected year');
  const owner={objectIndex:7,objectType:'Fan:VariableVolume',objectName:'Supply Fan'};
  const positive=scheduleName=>({op:'schedule_positive',scheduleName});
  const together=(...children)=>({op:'all',children});
  const input=(name,id=name)=>({id,kind:'schedule',source:{objectName:name,objectType:'Schedule:Constant'}});
  const profile=(expression,names=['Off'],layers)=>({availabilityDefinitions:{nodes:names.map(name=>input(name)),edges:[]},
    availabilityLayers:layers||[{id:'equipment',owner,role:'equipment',expression}],finalAvailability:expression});
  const configured=(report,expression=report.finalAvailability,layers=report.availabilityLayers)=>({...report,configuredAvailability:expression,configuredAvailabilityLayers:layers});
  const last=result=>result.rows.at(-1).timeline;
  const only=(timeline,value)=>timeline.segments.length===1&&timeline.segments[0].value===value;
  const extra=[...schedules,constant('On',1),constant('Tiny',.0001)];
  const rawStates=stack(profile(together(positive('Tiny'),positive('Negative')),['Tiny','Negative']),extra,calendar);
  check(rawStates.rows.length===3&&rawStates.rows.at(-1).kind==='final','Inputs not followed by one final row');
  check(only(rawStates.rows[0].timeline,1)&&only(rawStates.rows[1].timeline,0)&&only(last(rawStates),0),'Source predicates were not binary or final AND lost knownzero');
  const cycleOn={op:'manager_scheduled_on',scheduleName:'On'};
  const forceOff={op:'manager_scheduled_off',scheduleName:'Off'};
  const fanOverride=profile({op:'fan_gate',target:'central_fan',children:[cycleOn,positive('Off')]},['On','Off']);
  const immutable=JSON.stringify(fanOverride), override=stack(fanOverride,extra,calendar,{view:'week'});
  check(only(override.rows[1].timeline,0)&&only(last(override),1),'Final incorrectly ANDed source rows instead of manager fan override');
  check(override.rows.every(row=>row.timeline.totalMinutes===7*1440&&JSON.stringify(row.timeline.startDate)===JSON.stringify(override.startDate)),'Stack row calendars drifted');
  check(stack(fanOverride,extra,calendar,{view:'week'})===override&&JSON.stringify(fanOverride)===immutable,'Stack cache missed or mutated profile');
  const negativeOff=stack(profile({op:'manager_scheduled_off',scheduleName:'Negative'},['Negative']),extra,calendar);
  check(only(negativeOff.rows[0].timeline,0)&&only(last(negativeOff),1),'ScheduledOff treated negative nonzero source as ForceOff');
  for(const [op,wanted] of [['air_managers',0],['plant_managers',1]]) {
    const expression={op:'manager_gate',children:[{op,children:[cycleOn,forceOff]}]};
    check(only(last(stack(profile(expression,['On','Off']),extra,calendar)),wanted),'Stack lost native manager priority: '+op);
  }
  const runtime={op:'manager_unknown',reason:'runtime_manager',source:{objectIndex:8,objectType:'AvailabilityManager:NightCycle',objectName:'Night Cycle'}};
  const unknown=stack(profile({op:'manager_gate',children:[runtime]},['On']),extra,calendar);
  check(unknown.requiresReanalysis&&unknown.rows.length===1&&only(last(unknown),null),'Legacy runtime outcomes were reused or incomplete configuration silently accepted');
  const scheduledOwner={objectIndex:9,objectType:'AvailabilityManager:Scheduled',objectName:'Scheduled Manager'};
  const staticOwn={...positive('Off'),source:owner};
  const baseline={op:'fan_gate',target:'central_fan',children:[{op:'always',value:0,source:runtime.source},staticOwn]};
  const runtimeProfile=profile({op:'fan_gate',target:'central_fan',children:[runtime,staticOwn]},['On','Off']);
  const runtimeConfigured=configured(runtimeProfile,baseline,[{id:'equipment',owner,role:'equipment',expression:baseline}]);
  const beforeRuntime=JSON.stringify(runtimeConfigured),minimum=stack(runtimeConfigured,extra,calendar);
  check(minimum.rows.length===2&&minimum.rows[0].name==='Off'&&only(last(minimum),0)&&!minimum.requiresReanalysis,'Runtime-only manager/input polluted configured minimum permission');
  check(minimum.rows[0].owners.length===1&&minimum.rows[0].owners[0].objectIndex===owner.objectIndex&&minimum.rows.at(-1).owners[0].objectIndex===owner.objectIndex,'Configured input/final owner lost');
  check(evaluate(runtimeConfigured,extra,calendar).final.values.every(value=>value===null)&&JSON.stringify(runtimeConfigured)===beforeRuntime,'Configured stack changed native runtime semantics or source report');
  const partialConfigured={...runtimeConfigured};delete partialConfigured.configuredAvailabilityLayers;
  const partialMinimum=stack(partialConfigured,extra,calendar);
  check(partialMinimum.rows.length===2&&partialMinimum.rows[0].name==='Off'&&only(last(partialMinimum),0)&&partialMinimum.rows.at(-1).owners[0].objectIndex===owner.objectIndex,'Partial configured report mixed legacy runtime layers into static prerequisites');
  for(const reason of ['missing_manager','missing_manager_list','missing_schedule','unsupported_manager','unresolved_availability_schema']) {
    const expression={op:'manager_unknown',reason,source:scheduledOwner};
    const failure=stack(configured(profile(expression,[])),extra,calendar);
    check(failure.rows.length===2&&only(failure.rows[0].timeline,null)&&only(last(failure),null)&&failure.rows[0].owners[0].objectIndex===scheduledOwner.objectIndex,'Actual configuration error was excluded: '+reason);
  }
  const sharedExpression=together({...positive('On'),source:owner},{...positive('On'),source:scheduledOwner});
  const sharedProfile=configured(profile(sharedExpression,['On']));
  sharedProfile.availabilityDefinitions.nodes.push({id:'runtimeOwner',kind:'definition',source:runtime.source});
  sharedProfile.availabilityDefinitions.edges.push({from:'runtimeOwner',to:'On'});
  const shared=stack(sharedProfile,extra,calendar);
  check(shared.rows[0].owners.map(source=>source.objectIndex).join('|')==='7|9'&&shared.rows.length===2,'Shared source lost surviving owners or reattached excluded runtime owner');
  const dispatch={op:'always',value:0,reason:'not_in_operation_scheme'};
  const dispatched=stack(profile(together(positive('On'),dispatch),['On'],[{id:'equipment',owner,role:'equipment',expression:positive('On')},{id:'dispatch',owner,role:'plant_dispatch',expression:dispatch}]),extra,calendar);
  check(dispatched.rows[1].kind==='layer'&&only(dispatched.rows[1].timeline,0)&&only(last(dispatched),0),'Explicit zero dispatch membership became missing or unknown');
  const chiller={objectIndex:10,objectType:'Chiller:Electric:ReformulatedEIR',objectName:'Chiller 1'};
  const scheme={objectIndex:11,objectType:'PlantEquipmentOperationSchemes',objectName:'Cooling operation schemes'};
  const activation={...positive('On'),source:scheme};
  const chillerReport=configured(profile(together(activation,{op:'unknown',reason:'runtime_dispatch'}),['On']),activation,[
    {id:'dispatch',owner:scheme,role:'plant_dispatch',expression:activation},
    {id:'equipment',owner:chiller,role:'equipment',expression:{op:'always',source:chiller}},
  ]);
  chillerReport.availabilityDefinitions.nodes.push({id:'chillerDefault',kind:'definition',code:'no_availability_schedule',source:chiller});
  const chillerStack=stack(chillerReport,extra,calendar);
  check(chillerStack.rows.length===3&&chillerStack.rows[0].owners[0].objectIndex===scheme.objectIndex&&chillerStack.rows[1].labelCode==='noSeparateSchedule'&&chillerStack.rows[1].owners[0].objectIndex===chiller.objectIndex&&only(chillerStack.rows[1].timeline,1)&&only(last(chillerStack),1),'Chiller default or Plant activation schedule vanished into runtime dispatch');
  const unresolvedProfile=profile(positive('Missing'),[]);
  unresolvedProfile.availabilityDefinitions.nodes=[{id:'missing',kind:'unknown',code:'unresolved_schedule_reference',source:owner,parameters:[{name:'Schedule Name',value:'Missing'}]}];
  const unresolved=stack(unresolvedProfile,extra,calendar);
  check(unresolved.rows[0].name==='Missing'&&only(unresolved.rows[0].timeline,null)&&only(last(unresolved),null),'Missing schedule was omitted or resolved from owner name');
  const nested=profile(positive('Pulse'),['Pulse']);
  nested.availabilityDefinitions.nodes.push({id:'dayPulse',...node('Pulse','Schedule:Day:Interval')});
  nested.availabilityDefinitions.edges.push({from:'Pulse',to:'dayPulse'});
  nested.onOff={nodes:[input('On','unrelatedControl')],edges:[]};
  const annual=stack(nested,extra,calendar,{view:'year'});
  check(annual.rows.length===2&&last(annual).positiveMinutes===365&&last(annual).segments.length===731&&last(annual).segments[1].startMinute===367,'Annual stack sampled away minute pulses, included nested day or unrelated control schedules');
  const leap=stack(nested,extra,{...calendar,year:2024},{view:'year'});
  check(last(leap).positiveMinutes===366&&last(leap).totalMinutes===366*1440,'Final availability calendar lost leapday');
  const weekly=stack(profile(positive('Week'),['Week']),extra,{...calendar,month:6,day:29},{view:'week'});
  check(last(weekly).positiveMinutes===5*1440,'Final week does not advance weekday with source rows');
  const partialFinal=last(stack(profile(positive('Missing weekends'),['Missing weekends']),extra,calendar,{view:'year'}));
  check(partialFinal.status==='partial'&&partialFinal.allOff===null&&partialFinal.unknownMinutes>0,'Final unknown calendar interval was rendered as known OFF');
  const legacy=stack({finalAvailability:positive('Tiny')},extra,calendar);
  check(legacy.rows[0].name==='Tiny'&&legacy.rows[0].source.objectType===extra.find(schedule=>schedule.name==='Tiny').objectType&&only(last(legacy),1),'Legacy exact references or actual compiled schedule identity were lost');
  check(only(last(stack({},extra,calendar)),null),'Absent final availability became unrestricted');
  check(JSON.stringify(schedules)===before,'Availability stack mutated original schedules');
  document.body.dataset.timelineStatus='passed';document.querySelector('#result').textContent='passed';
} catch(error) { document.body.dataset.timelineStatus='failed';document.querySelector('#result').textContent=error.stack||String(error); }
</script></body></html>`
