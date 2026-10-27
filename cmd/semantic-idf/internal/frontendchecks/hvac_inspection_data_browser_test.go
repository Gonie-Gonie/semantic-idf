package frontendchecks

import (
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestHVACInspectionDataBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless HVAC data alignment acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	fixtures := hvacRuntimeBrowserFixtures(t)
	mux.HandleFunc("/runtime-fixtures", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fixtures)
	})
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/data", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, hvacInspectionDataHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/data").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-hvac-data-status="passed"`) {
		if _, detail, ok := strings.Cut(string(output), `<pre id="result">`); ok {
			message, _, _ := strings.Cut(detail, "</pre>")
			t.Fatalf("HVAC data alignment failed: %v\n%s", err, html.UnescapeString(message))
		}
		t.Fatalf("HVAC data alignment failed: %v\n%s", err, output)
	}
}

const hvacInspectionDataHTML = `<!doctype html><body><pre id="result"></pre><script>window.addEventListener('error',event=>document.getElementById('result').textContent=event.message+' line '+event.lineno);</script><script type="module">
const check=(v,m)=>{if(!v)throw Error(m)};
` + hvacRuntimeBrowserHelpersJS + `
try {
 const {prepareHVACInspection:prepare,hvacInspectionTrace:trace,hvacInspectionSnapshots:snapshot,hvacInspectionBasicProperties:basic,hvacInspectionSharedFrame:sharedFrame,rememberHVACInspectionFrame:rememberFrame}=await import('/src/js/hvac-inspection-data.js');
 const {hvacSetpointMode:mode,hvacSetpointComparison:compare}=await import('/src/js/hvac-setpoint.js');
 const point=(x,value,label)=>({x,value,label:label||'01-01 '+String(x+1).padStart(2,'0')+':00'});
 const series=(name,unit,points,key='NODE A',file='eplusout.sql')=>({name,keyValue:key,file,column:key+':'+name+' ['+unit+']',reportingFrequency:'Hourly',points});
 const temp=series('System Node Temperature','C',[point(0,20),point(1,21),point(2,22)]);
 const flow=series('System Node Mass Flow Rate','kg/s',[point(0,0),point(2,3)]);
 const humidity=series('System Node Humidity Ratio','kgWater/kgDryAir',[point(0,.008),point(1,.009),point(2,.010)]);
 const rh=series('System Node Relative Humidity','%',[point(0,40),point(1,45),point(2,50)]);
 const setpoint=series('System Node Setpoint Temperature','C',[point(0,21),point(1,-999),point(2,23)]);
 const power=series('Fan Electricity Rate','W',[point(0,0),point(1,400),point(2,null)],'FAN');
 const loop={name:'Main loop',series:[temp,flow,humidity,rh,setpoint],components:[{componentName:'FAN',componentType:'Fan:VariableVolume',inletNodes:['NODE A'],outletNodes:['NODE B'],series:[power]}]};
 const original=JSON.stringify(loop),model=prepare(loop),node=model.entities.find(e=>e.kind==='node'),fan=model.entities.find(e=>e.kind==='component');
 check(model.frames.length===3&&model.frames[2].x-model.frames[0].x===2,'actual hourly calendar not retained');
 const flowProperty=node.properties.find(p=>p.kind==='flow'),flowTrace=trace(model,flowProperty);
 check(flowTrace.points.map(p=>p.value).join(',')==='0,,3','missing middle sample shifted or acquired zero/previous value');
 check(snapshot(model,1).nodes[0].metrics.find(m=>m.id==='flow').value===null,'snapshot clamped missing sample');
 check(snapshot(model,0).nodes[0].active,'reported zero point lost frame highlighting');
 check(snapshot(model,0).components[0].status==='off'&&snapshot(model,1).components[0].status==='on'&&snapshot(model,2).components[0].status==='unknown','equipment state fabricated from missing/zero power');
 check(trace(model,fan.properties[0]).unit==='kW'&&trace(model,fan.properties[0]).points[1].value===.4,'power unit/value disagrees');
 check(fan.inletNodes[0]==='NODE A'&&fan.outletNodes[0]==='NODE B','exact nested device ports lost');
 const ratio=trace(model,node.properties.find(p=>p.kind==='humidity'));
 check(ratio.unit==='g/kg'&&ratio.points[0].value===8,'humidity ratio scaling mislabeled');
 check(basic(model,'humidity')[0].kind==='relativeHumidity','reported RH should be preferred in default humidity graph');
 check(snapshot(model,0).nodes[0].metrics.length===4&&snapshot(model,0).nodes[0].metrics.some(m=>m.id==='setpoint')&&!snapshot(model,0).nodes[0].metrics.some(m=>m.id==='humidity'),'redundant humidity hid setpoint');
 check(snapshot(model,1).nodes[0].metrics.find(m=>m.id==='setpoint').value===null,'unset setpoint sentinel shown as temperature');
 check(basic(model,'flow',{[node.id]:false}).length===0,'node toggle retained hidden trace');
 const shifted=structuredClone(loop); shifted.series.push(series('System Node Enthalpy','J/kg',[point(101,42000,'01-01 02:00')],'NODE A','eplusout.csv'));
 const shiftedModel=prepare(shifted),enthalpy=shiftedModel.entities[0].properties.find(p=>p.kind==='enthalpy');
 check(trace(shiftedModel,enthalpy).points.map(p=>p.value).join(',')===',42,','cross-file data joined raw row number instead of unique timestamp');
 const ambiguous=structuredClone(shifted); ambiguous.series.at(-1).points.push(point(102,43000,'01-01 02:00'));
 const ambiguousModel=prepare(ambiguous);
 check(trace(ambiguousModel,ambiguousModel.entities[0].properties.find(p=>p.kind==='enthalpy')).points.every(p=>p.value===null),'ambiguous cross-file calendar silently joined');
 const repeated=structuredClone(loop); repeated.series[1].points.push(point(0,4));
 const repeatedModel=prepare(repeated);
 check(trace(repeatedModel,repeatedModel.entities[0].properties.find(p=>p.kind==='flow')).points[0].value===null,'duplicate same-file time silently chosen');
 const mixed=structuredClone(loop); mixed.series.push({...temp,reportingFrequency:'Monthly',points:[point(9,100)]});
 check(prepare(mixed).frames.length===3,'Monthly observations mixed into frame timeline');
 const fuel={...structuredClone(loop),components:[{componentName:'GAS COIL',componentType:'Coil:Heating:Fuel',series:[series('Heating Coil Electricity Rate','W',[point(0,0)],'GAS COIL'),series('Heating Coil Heating Rate','W',[point(0,10000)],'GAS COIL')]}]};
 check(snapshot(prepare(fuel),0).components[0].status==='on','positive fuel heating output marked Off from zero ancillary electric power');
 const blank={name:'Empty',series:[],components:[]};check(prepare(blank).frames.length===0,'empty result invented frames');
 const context={},firstUI={frameIndex:2},secondUI={frameIndex:0};rememberFrame(loop,context,firstUI);
 const shorter={name:'Second',series:[series('System Node Temperature','C',[point(0,8),point(2,9)])]};
 sharedFrame(shorter,context,secondUI);check(secondUI.frameIndex===1,'changing loop retained array index instead of selected timestamp');
 sharedFrame(blank,context,{});sharedFrame(loop,context,firstUI);check(firstUI.frameIndex===2,'visiting an empty loop discarded shared frame');
 secondUI.frameIndex=0;rememberFrame(shorter,context,secondUI);sharedFrame(loop,context,firstUI);check(firstUI.frameIndex===0,'changing frame in second loop did not update first loop');
 const device=(name,type,status='on')=>({name,type,status,inletNodes:['Inlet'],outletNodes:['Outlet']});
 const cooler=device('Cooler','Coil:Cooling:Water'),heater=device('Heater','Coil:Heating:Water'),mixedEquipment={supplySide:{branches:[{components:[cooler,heater].map(item=>({objectName:item.name,objectType:item.type}))}]}};
 check(mode(mixedEquipment,'Outlet',[cooler,{...heater,status:'off'}])==='cooling'&&mode(mixedEquipment,'Outlet',[{...cooler,status:'off'},heater])==='heating','frame operating mode ignores active typed equipment');
 check(mode(mixedEquipment,'Outlet',[cooler,heater])===''&&mode(mixedEquipment,'Outlet',[{...cooler,status:'off'},{...heater,status:'off'}])==='','ambiguous mixed/off loop fabricated operating mode');
 check(mode({supplySide:{branches:[{components:[{objectName:'B',objectType:'Boiler:HotWater'}]}]}},'Water',[])==='heating','hot-water loop lost typed heating mode');
 for(const [actual,target,kind,operator,status] of [[14,15,'cooling','<','satisfied'],[15,15,'cooling','=','satisfied'],[16,15,'cooling','>','unmet'],[16,15,'heating','>','satisfied'],[15,15,'heating','=','satisfied'],[14,15,'heating','<','unmet'],[0,0,'cooling','=','satisfied'],[15,15,'','=','unknown']]){
  const result=compare(actual,target,kind);check(result.operator===operator&&result.state===status,'incorrect setpoint comparison: '+[actual,target,kind]);
 }
 check(compare(15,null,'cooling')===null&&compare(15,-999,'cooling')===null&&compare(null,15,'heating')===null,'missing/sentinel temperature displayed as valid setpoint comparison');
 // Exact values observed in hourly EnergyPlus SQL for an unset node. A strict
 // <= -999 check accepted the final value and exposed a fake setpoint.
 const unsetValues=[-999,-999.0000000000005,-998.9999999999999];
 check(unsetValues.every(value=>compare(20,value,'heating')===null),'averaged unset sentinel became a setpoint comparison');
 const noSetpoint={...structuredClone(loop),series:[...loop.series.filter(item=>item!==setpoint),{...setpoint,points:unsetValues.map((value,x)=>point(x,value))}]},unsetModel=prepare(noSetpoint);
 check(!unsetModel.entities[0].properties.some(property=>property.kind==='setpoint'),'uncontrolled node still offers setpoint as a custom graph property');
 check([0,1,2].every(index=>!snapshot(unsetModel,index).nodes[0].metrics.some(metric=>metric.id==='setpoint')),'unset setpoint remains in frame snapshots');
 check(trace(unsetModel,unsetModel.entities[0].properties.find(property=>property.kind==='temperature')).points.map(item=>item.value).join(',')==='20,21,22','setpoint filtering changed measured temperature');
 const intermittent={name:'Intermittent',series:[temp,{...setpoint,points:[point(0,0),point(1,-998.9999999999999),point(2,-20)]},{...setpoint,keyValue:'OTHER NODE',column:'OTHER NODE:System Node Setpoint Temperature [C]',points:unsetValues.map((value,x)=>point(x,value))}]},intermittentModel=prepare(intermittent);
 const defined=intermittentModel.entities.find(entity=>entity.name==='NODE A').properties.find(property=>property.kind==='setpoint');
 check(trace(intermittentModel,defined).points.map(item=>item.value).join(',')==='0,,-20','valid zero/negative setpoints were lost or missing frame was filled from a neighbor');
 check(!intermittentModel.entities.some(entity=>entity.name==='OTHER NODE'),'another node inherited the defined setpoint');
 for(const loopType of ['PlantLoop','CondenserLoop']){
  const water={...loop,loopType},waterModel=prepare(water);
  check(waterModel.waterLoop&&basic(waterModel,'humidity').length===0,'water loop exposes default humidity graph data');
  check(!waterModel.entities.filter(e=>e.kind==='node').some(e=>e.properties.some(p=>/humidity/i.test(p.name))),'water loop offers humidity in custom property selector');
  check(snapshot(waterModel,0).nodes[0].metrics.length===3&&snapshot(waterModel,0).nodes[0].metrics.find(m=>m.id==='temperature').value===20,'water snapshot retained RH or lost temperature/flow/setpoint');
 }
 // Runtime control inspection has its own native Timestep calendar. Its
 // source-backed definitions come from the executed input, not the editor.
 const runtime=await import('/src/js/hvac-runtime-data.js'),fixtures=await(await fetch('/runtime-fixtures')).json(),fixturesBefore=JSON.stringify(fixtures);
 const prepareRuntime=runtime.prepareHVACRuntimeInspection,runtimeSnapshot=runtime.hvacRuntimeSnapshot,runtimeTrace=runtime.hvacRuntimeTrace;
 const sourceIdentity=source=>JSON.stringify([source?.objectIndex,source?.objectType,source?.objectName]),parameterIdentity=parameters=>JSON.stringify((parameters||[]).map(parameter=>[parameter.name,parameter.value,parameter.objectIndex,parameter.fieldIndex]));
 for(const fixture of fixtures){
  const executed=runtimeClone(fixtures,fixture.family),before=JSON.stringify(executed),prepared=prepareRuntime(executed),profile=selectedRuntimeProfile(prepared,fixture);
  check(profile&&profile.source.objectIndex===fixture.component.objectIndex&&profile.source.objectType===fixture.component.objectType&&profile.source.objectName===fixture.component.objectName,'executed runtime profile/source identity was lost for '+fixture.family);
  check(profile.definition.loopId===fixture.profile.loopId&&profile.definition.parameterControl.nodes.every(node=>fixture.profile.parameterControl.nodes.some(original=>node.id===original.id&&sourceIdentity(node.source)===sourceIdentity(original.source)&&parameterIdentity(node.parameters)===parameterIdentity(original.parameters))),'runtime definition was rebuilt from another component/circuit');
  check(prepared.frames.length===4&&prepared.frames.every(frame=>frame.frequency.toLowerCase()==='timestep')&&Math.abs(prepared.frames[3].x-prepared.frames[0].x-.75)<1e-8,'Timestep observations were sampled, promoted to Hourly or merged with loop-value frames');
  const frame=runtimeSnapshot(prepared,profile.id,0);
  check(profile.signals.some(signal=>signal.required)&&frame.errors.length===0,'native '+fixture.family+' fixture failed required data collection: '+JSON.stringify(frame.errors.map(signal=>[signal.role,signal.variableName,signal.status,signal.message])));
  check(frame.signals.every(signal=>signal.componentIndex===fixture.component.objectIndex&&(!signal.loopId||signal.loopId===fixture.profile.loopId)),'runtime observation borrowed a sibling equipment/circuit binding');
  check(JSON.stringify(executed)===before,'runtime projection mutated captured observations or definitions');
 }
 const fanFixture=fixtures.find(fixture=>fixture.family==='fan'),fanLoop=runtimeClone(fixtures,'fan');
 const fanGate=expressionNodes(fanFixture.profile.finalAvailability).find(expression=>expression.op==='fan_gate'),ownSchedule=expressionNodes(fanGate?.children?.[1]).find(expression=>expression.op==='schedule_positive')?.scheduleName;
 check(ownSchedule,'parsed fan no longer exercises own schedule with manager overrides');
 setReported(fanLoop,signal=>signal.role==='activity',[0]);
 setReported(fanLoop,signal=>signal.role==='manager_status',[0]);
 setReported(fanLoop,signal=>signal.role==='schedule'&&signal.keyValue.toLowerCase()===ownSchedule.toLowerCase(),[0]);
 let runtimeModel=prepareRuntime(fanLoop),fanProfile=selectedRuntimeProfile(runtimeModel,fanFixture),observed=runtimeSnapshot(runtimeModel,fanProfile.id,0);
 check(observed.permission.state==='blocked'&&observed.reason==='permission_blocked'&&observed.availability.some(signal=>signal.role==='manager_status'&&signal.state==='no_action'&&signal.value===0),'NoAction=0 was treated as Boolean OFF or allowed the fan schedule to be bypassed');
 const cycle=structuredClone(fanLoop);setReported(cycle,signal=>signal.role==='manager_status',[2]);
 runtimeModel=prepareRuntime(cycle);observed=runtimeSnapshot(runtimeModel,fanProfile.id,0);
 check(observed.permission.state==='allowed'&&observed.availability.some(signal=>signal.state==='cycle_on'),'CycleOn failed to override the fan own schedule');
 const forced=structuredClone(cycle);setReported(forced,signal=>signal.role==='schedule',[1]);setReported(forced,signal=>signal.role==='manager_status',[1]);
 observed=runtimeSnapshot(prepareRuntime(forced),fanProfile.id,0);
 check(observed.permission.state==='blocked'&&observed.reason==='permission_blocked'&&observed.availability.some(signal=>signal.state==='force_off'),'reported ForceOff was replaced by a positive source schedule');
 const mixedManager=structuredClone(cycle);setReported(mixedManager,signal=>signal.role==='manager_status',[1.5]);
 observed=runtimeSnapshot(prepareRuntime(mixedManager),fanProfile.id,0);
 check(observed.permission.state==='mixed'&&observed.availability.some(signal=>signal.state==='mixed'),'an interval-average manager code was rounded into an invented native action');
 const noOverride=structuredClone(fanLoop);setReported(noOverride,signal=>signal.role==='schedule',[1]);
 const noOverrideDefinition=noOverride.operatingProfiles.find(profile=>profile.componentIndex===fanFixture.component.objectIndex);
 const noOverrideGates=[noOverrideDefinition.finalAvailability,...noOverrideDefinition.availabilityLayers.map(layer=>layer.expression)].flatMap(expressionNodes).filter(expression=>['manager_gate','fan_gate'].includes(expression.op));
 check(noOverrideGates.length>0,'native fan no longer exercises manager-domain gates');
 for(const gate of noOverrideGates)gate.children[0]={op:'always',value:0};
 observed=runtimeSnapshot(prepareRuntime(noOverride),fanProfile.id,0);
 check(observed.permission.state==='allowed','manager-domain constant NoAction=0 became Boolean false instead of preserving its own schedule');
 const booleanStop=structuredClone(noOverride),booleanDefinition=booleanStop.operatingProfiles.find(profile=>profile.componentIndex===fanFixture.component.objectIndex);
 booleanDefinition.finalAvailability={op:'all',children:[{op:'always',value:0},booleanDefinition.finalAvailability]};
 booleanDefinition.availabilityLayers.unshift({id:'explicit-stop',role:'equipment',owner:booleanDefinition.availabilityLayers[0].owner,expression:{op:'always',value:0}});
 observed=runtimeSnapshot(prepareRuntime(booleanStop),fanProfile.id,0);check(observed.permission.state==='blocked','Boolean constant zero inherited manager NoAction semantics');

 const coilFixture=fixtures.find(fixture=>fixture.family==='coil'),coilLoop=runtimeClone(fixtures,'coil');
 setReported(coilLoop,signal=>signal.role==='sensor'&&/temperature/i.test(signal.variableName),[22]);setReported(coilLoop,signal=>signal.role==='setpoint',[20]);
 setReported(coilLoop,signal=>signal.role==='actuator_request',[.8]);setReported(coilLoop,signal=>signal.role==='actuator',[.3]);
 runtimeModel=prepareRuntime(coilLoop);const coilProfile=selectedRuntimeProfile(runtimeModel,coilFixture);observed=runtimeSnapshot(runtimeModel,coilProfile.id,0);
 check(observed.errors.length===0&&observed.residuals.some(residual=>residual.value===2&&residual.unit==='°C'),'native sensed-minus-setpoint residual lost its actual measurement/target sign');
 check(observed.signals.some(signal=>signal.role==='actuator_request'&&signal.value===.8)&&observed.signals.some(signal=>signal.role==='actuator'&&signal.value===.3),'requested and actual actuator flows were conflated');
 const zeroTarget=structuredClone(coilLoop);setReported(zeroTarget,signal=>signal.role==='setpoint',[0]);
 observed=runtimeSnapshot(prepareRuntime(zeroTarget),coilProfile.id,0);check(observed.residuals.some(residual=>residual.value===22)&&observed.errors.length===0,'valid 0 °C target was interpreted as missing data');
 const noTarget=structuredClone(coilLoop);setReported(noTarget,signal=>signal.role==='setpoint',[-998.9999999999999]);
 observed=runtimeSnapshot(prepareRuntime(noTarget),coilProfile.id,0);check(observed.reason==='acquisition_error'&&observed.errors.some(signal=>signal.role==='setpoint')&&!observed.residuals.length,'unset setpoint silently became a successful control comparison');
 const residualDefinition=coilFixture.profile.parameterControl.nodes.find(node=>node.code==='sensed_minus_setpoint'),measuredEdge=coilFixture.profile.parameterControl.edges.find(edge=>edge.to===residualDefinition?.id&&edge.relation==='adds_measurement');
 const missing=structuredClone(coilLoop),missingBinding=missing.controlSignals.find(signal=>signal.role==='sensor'&&signal.graph==='parameterControl'&&signal.nodeId===measuredEdge?.from&&/temperature/i.test(signal.variableName)&&signal.required);
 check(missingBinding,'native coil sensor must be a required observation');missing.controlSeries=missing.controlSeries.filter(series=>series.sourceId!==missingBinding.seriesId);
 observed=runtimeSnapshot(prepareRuntime(missing),coilProfile.id,0);check(observed.reason==='acquisition_error'&&observed.errors.some(signal=>signal.id===missingBinding.id),'required missing observation was shown as normal unknown/inactive instead of acquisition failure');
 const duplicated=structuredClone(coilLoop),bound=duplicated.controlSeries.find(series=>series.sourceId===missingBinding.seriesId);duplicated.controlSeries.push(structuredClone(bound));
 observed=runtimeSnapshot(prepareRuntime(duplicated),coilProfile.id,0);check(observed.reason==='acquisition_error'&&observed.errors.some(signal=>signal.id===missingBinding.id&&signal.status==='ambiguous'),'ambiguous source ID was silently selected');
 const wrongSource=structuredClone(coilLoop),wrong=wrongSource.controlSeries.find(series=>series.sourceId===missingBinding.seriesId);wrong.keyValue='ANOTHER CIRCUIT NODE';
 observed=runtimeSnapshot(prepareRuntime(wrongSource),coilProfile.id,0);check(observed.errors.some(signal=>signal.id===missingBinding.id),'source ID trusted a different native output key');
 const missingFrame=structuredClone(coilLoop),frameSource=missingFrame.controlSeries.find(series=>series.sourceId===missingBinding.seriesId);frameSource.points.splice(1,1);
 runtimeModel=prepareRuntime(missingFrame);observed=runtimeSnapshot(runtimeModel,coilProfile.id,1);
 check(observed.reason==='acquisition_error'&&observed.signals.find(signal=>signal.id===missingBinding.id).value===null&&!observed.residuals.length,'missing frame borrowed the prior sample or a neighbor interval');
 const traceSignal=runtimeModel.signals.find(signal=>signal.id===missingBinding.id);check(runtimeTrace(runtimeModel,traceSignal).points.map(point=>point.value).join(',')==='22,,22,22','runtime time-series trace shifted a missing source frame');
 const crossFile=structuredClone(coilLoop),secondary=crossFile.controlSeries.find(series=>series.sourceId===missingBinding.seriesId);secondary.file='eplusout.csv';secondary.points.forEach(point=>point.x+=100);
 observed=runtimeSnapshot(prepareRuntime(crossFile),coilProfile.id,1);check(observed.errors.length===0&&observed.signals.find(signal=>signal.id===missingBinding.id).value===22,'runtime CSV source used row number instead of a unique native timestamp');
 const ambiguousTime=structuredClone(crossFile),secondaryDuplicate=ambiguousTime.controlSeries.find(series=>series.sourceId===missingBinding.seriesId);secondaryDuplicate.points.push({...secondaryDuplicate.points[1],x:500,value:99});
 observed=runtimeSnapshot(prepareRuntime(ambiguousTime),coilProfile.id,1);check(observed.reason==='acquisition_error'&&observed.signals.find(signal=>signal.id===missingBinding.id).value===null,'repeated secondary timestamp borrowed an arbitrary observation');
 const hourlyAlias=structuredClone(coilLoop),hourlySource=structuredClone(hourlyAlias.controlSeries.find(series=>series.sourceId===missingBinding.seriesId));hourlySource.reportingFrequency='Hourly';hourlySource.points.forEach(point=>point.value=999);hourlyAlias.controlSeries.push(hourlySource);
 runtimeModel=prepareRuntime(hourlyAlias);observed=runtimeSnapshot(runtimeModel,coilProfile.id,0);check(runtimeModel.frames.length===4&&observed.errors.length===0&&observed.signals.find(signal=>signal.id===missingBinding.id).value===22,'Hourly output was borrowed for a Timestep control binding');
 const foreign=structuredClone(coilLoop),foreignSignal={...foreign.controlSignals.find(signal=>signal.id===missingBinding.id),id:'foreign-sensor',loopId:'another-native-circuit',seriesId:'foreign-series'},foreignSeries=structuredClone(foreign.controlSeries.find(series=>series.sourceId===missingBinding.seriesId));foreignSeries.sourceId='foreign-series';foreignSeries.points.forEach(point=>point.value=99);foreign.controlSignals.push(foreignSignal);foreign.controlSeries.push(foreignSeries);
 observed=runtimeSnapshot(prepareRuntime(foreign),coilProfile.id,0);check(!observed.signals.some(signal=>signal.id==='foreign-sensor')&&observed.residuals.some(residual=>residual.value===2),'same object index on another circuit contaminated the selected occurrence');
 const chillerFixture=fixtures.find(fixture=>fixture.family==='chiller'),idle=runtimeClone(fixtures,'chiller');setReported(idle,signal=>signal.role==='demand',[0]);setReported(idle,signal=>signal.role==='activity',[0]);
 const idleModel=prepareRuntime(idle),chillerProfile=selectedRuntimeProfile(idleModel,chillerFixture);
 check(expressionNodes(chillerFixture.profile.finalAvailability).some(expression=>expression.reason==='runtime_dispatch'),'native chiller no longer exercises load-dependent dispatch');
 check(!expressionNodes(chillerProfile.availabilityExpression).some(expression=>expression.reason==='runtime_dispatch')&&expressionNodes(chillerProfile.availabilityExpression).some(expression=>expression.op==='schedule_positive'),'runtime availability lost native configured activation or retained a dispatch algorithm as a minimum permission condition');
 observed=runtimeSnapshot(idleModel,chillerProfile.id,0);check(observed.errors.length===0&&observed.permission.state==='allowed'&&observed.activity==='inactive'&&observed.reason==='zero_reported_demand','allowed equipment with reported zero demand acquired an invented blockage reason');
 const invalidDefinition=runtimeClone(fixtures,'chiller'),invalidProfile=invalidDefinition.operatingProfiles.find(profile=>profile.componentIndex===chillerFixture.component.objectIndex),invalidLayer=invalidProfile.availabilityLayers.find(layer=>layer.role==='plant_dispatch');
 check(invalidLayer,'native chiller must have a dispatch layer for definition-failure acceptance');
 for(const layer of [invalidLayer,...invalidProfile.configuredAvailabilityLayers.filter(layer=>layer.id===invalidLayer.id)])layer.expression={op:'unknown',reason:'missing_operation_schemes',source:layer.owner};
 observed=runtimeSnapshot(prepareRuntime(invalidDefinition),chillerProfile.id,0);
 check(observed.reason==='acquisition_error'&&observed.errors.some(error=>error.id==='availability-definition'&&error.status==='unsupported'&&error.message.includes('missing_operation_schemes')),'unresolved native operation scheme became an ordinary successful unknown availability state');

 // The transfer envelope keeps native columns lazy. Inspecting one coil must
 // not expand every point object or read another equipment profile's values.
 const {decodeSimulationResultTransfer:decodeRuntime}=await import('/src/js/simulation-result-transport.js');
 const compactLoop=structuredClone(coilLoop),otherEquipment=runtimeClone(fixtures,'fan');
 compactLoop.operatingProfiles.push(...otherEquipment.operatingProfiles);
 compactLoop.controlSignals.push(...otherEquipment.controlSignals);
 compactLoop.controlSeries.push(...otherEquipment.controlSeries);
 const compactSources=compactLoop.controlSeries,compactPointSets=[],compactBindings=[],columnReads=[];
 const nativeRows=compactSources[0].points.map(point=>point.x),nativeLabels=compactSources[0].points.map(point=>point.label);
 let rowReads=0;
 const sharedRows=new Proxy(nativeRows,{get(target,key,receiver){if(typeof key==='string'&&/^\d+$/.test(key))rowReads++;return Reflect.get(target,key,receiver);}});
 for(const [index,series] of compactSources.entries()){
  check(series.points.length===nativeRows.length&&series.points.every((point,offset)=>point.x===nativeRows[offset]&&point.label===nativeLabels[offset]),'native compact fixture does not share one reporting axis');
  let values=series.points.map(point=>point.value);columnReads.push(0);
  const set={timeline:0};
  Object.defineProperty(set,'values',{enumerable:true,configurable:true,get(){columnReads[index]++;return values;},set(next){values=next;}});compactPointSets.push(set);
  delete series.displayColumn;delete series.displayUnit;series.points=null;series.displayPoints=null;
  for(const field of ['points','displayPoints'])compactBindings.push({path:['purposeResults','hvacLoops',0,'controlSeries',index,field],data:index});
 }
 const compactEnvelope={schema:'semantic-idf.simulation-transfer/v1',result:{purposeResults:{hvacLoops:[compactLoop]}},timelines:[{x:sharedRows,labels:nativeLabels}],pointSets:compactPointSets,pointBindings:compactBindings};
 const decodedRuntime=decodeRuntime(compactEnvelope,{deferData:true}).purposeResults.hvacLoops[0];
 const lazyPoints=()=>decodedRuntime.controlSeries.every(series=>['points','displayPoints'].every(field=>typeof Object.getOwnPropertyDescriptor(series,field)?.get==='function'));
 check(lazyPoints()&&columnReads.every(reads=>reads===0),'compact runtime decode eagerly read value columns or materialized native points');
 const compactModel=prepareRuntime(decodedRuntime),compactProfile=selectedRuntimeProfile(compactModel,coilFixture);
 check(compactModel.frames.length===4&&columnReads.every(reads=>reads===0)&&lazyPoints(),'runtime preparation eagerly read all source values instead of the shared timeline');
 const compactSnapshot=runtimeSnapshot(compactModel,compactProfile.id,0),compactSignal=compactModel.signals.find(signal=>signal.id===missingBinding.id),compactTrace=runtimeTrace(compactModel,compactSignal);
 check(compactSnapshot.errors.length===0&&compactSnapshot.residuals.some(residual=>residual.value===2)&&compactSnapshot.signals.some(signal=>signal.role==='actuator_request'&&signal.value===.8)&&compactSnapshot.signals.some(signal=>signal.role==='actuator'&&signal.value===.3),'compact native values or control semantics changed while reading lazy columns');
 check(compactTrace.points.map(point=>point.value).join(',')==='22,22,22,22'&&runtimeTrace(compactModel,compactSignal,compactModel.frames.slice(1,3)).points.length===2,'compact selected time series lost native values/window boundaries');
 const selectedSources=new Set(compactProfile.signals.map(signal=>signal.seriesId));
 check(columnReads.every((reads,index)=>selectedSources.has(compactSources[index].sourceId)||reads===0),'opening one runtime profile eagerly read unrelated equipment channels');
 check(rowReads<=nativeRows.length*4,'the shared reporting timeline was rescanned for every runtime binding');
 check(lazyPoints(),'snapshot or trace materialized canonical controlSeries points/displayPoints');
 const oldRuntime=prepareRuntime({name:'Old saved loop',series:loop.series,components:loop.components});check(!oldRuntime.profiles.length&&!oldRuntime.frames.length,'legacy loop values fabricated runtime definitions/observations');
 check(JSON.stringify(fixtures)===fixturesBefore,'runtime inspection mutated native captured fixtures');

 const {setLanguage,getLanguage}=await import('/src/js/i18n.js');setLanguage('ko');
 const korean=prepare(loop);check(korean!==model&&korean.language===getLanguage()&&korean.language==='ko','language change retained cached presentation');
 check(trace(korean,korean.entities[0].properties.find(p=>p.kind==='temperature')).points.map(p=>p.value).join(',')==='20,21,22','language change altered original observations');
 setLanguage('ja');const japanese=prepare(loop);check(japanese!==korean&&japanese.language==='ja','non-Korean locale reused Korean presentation');
 setLanguage('en');const english=prepare(loop);check(english!==japanese&&english.language==='en','locale cache depends on a translated term instead of locale identity');
 check(JSON.stringify(loop)===original,'inspection mutated original result');
 document.body.dataset.hvacDataStatus='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.hvacDataStatus='failed';document.getElementById('result').textContent=error.stack;}
</script>`
