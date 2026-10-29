package frontendchecks

import (
	"context"
	"encoding/json"
	"html"
	"io"
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
	fixtureCount := len(fixtures)
	// Keep the reported schedule-OFF / CycleOn example grounded in the exact
	// default model's VAV_3 occurrence, not a renamed sibling or handmade AST.
	input, err := os.ReadFile(repoPath("frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := idf.Parse(string(input))
	if err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(t.TempDir(), "vav3.idf")
	if err := os.WriteFile(inputPath, input, 0600); err != nil {
		t.Fatal(err)
	}
	appendFanFixture := func(doc idf.Document, inputPath, family, fanName, loopName string) {
		t.Helper()
		report := idf.AnalyzeHVAC(doc)
		for _, profile := range report.OperatingProfiles {
			if profile.ComponentIndex < 0 || profile.ComponentIndex >= len(doc.Objects) {
				continue
			}
			object := doc.Objects[profile.ComponentIndex]
			if object.Type != "Fan:VariableVolume" || len(object.Fields) == 0 || object.Fields[0].Value != fanName {
				continue
			}
			for _, loop := range report.Loops {
				if loop.ID != profile.LoopID || loop.Name != loopName {
					continue
				}
				fixture := hvacRuntimeBrowserFixture{Family: family, Component: idf.HVACOperatingSource{ObjectIndex: profile.ComponentIndex, ObjectType: object.Type, ObjectName: object.Fields[0].Value}, Profile: profile, Topology: loop}
				fixture.Runtime = hvacRuntimeBrowserResult(t, inputPath, fixture)
				fixtures = append(fixtures, fixture)
				return
			}
		}
		t.Fatalf("native %s runtime occurrence is missing", family)
	}
	appendFanFixture(doc, inputPath, "fan-vav3", "VAV_3_Fan", "VAV_3")
	// ForceOff is tested with an authored ScheduledOff manager. NightCycle's
	// averaged value 1 is not a native ForceOff command.
	forceDoc, err := idf.Parse(string(input))
	if err != nil {
		t.Fatal(err)
	}
	for i := range forceDoc.Objects {
		object := &forceDoc.Objects[i]
		if len(object.Fields) == 0 {
			continue
		}
		if object.Type == "AvailabilityManagerAssignmentList" && object.Fields[0].Value == "VAV_5 Availability Manager List" {
			object.Fields[1].Value = "AvailabilityManager:ScheduledOff"
		}
		if object.Type == "AvailabilityManager:NightCycle" && object.Fields[0].Value == "VAV_5 Availability Manager" {
			object.Type = "AvailabilityManager:ScheduledOff"
			object.Fields = []idf.Field{{Value: "VAV_5 Availability Manager"}, {Value: "Runtime Force Off"}}
		}
	}
	forceDoc.Objects = append(forceDoc.Objects, idf.Object{Index: len(forceDoc.Objects), Type: "Schedule:Constant", Fields: []idf.Field{{Value: "Runtime Force Off"}, {}, {Value: "0"}}})
	forcePath := filepath.Join(t.TempDir(), "scheduled-off.idf")
	if err := os.WriteFile(forcePath, []byte(forceDoc.String()), 0600); err != nil {
		t.Fatal(err)
	}
	appendFanFixture(forceDoc, forcePath, "fan-forced", "VAV_5_Fan", "VAV_5")
	if len(fixtures) != fixtureCount+2 {
		t.Fatal("native availability fixture count changed")
	}
	mux.HandleFunc("/runtime-fixtures", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fixtures)
	})
	nativeCapture := os.Getenv("HVAC_SCATTER_NATIVE_BUNDLE")
	if nativeCapture != "" {
		if _, err := os.Stat(nativeCapture); err != nil {
			t.Fatal(err)
		}
	}
	mux.HandleFunc("/native-equipment-fixture", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if nativeCapture == "" {
			_, _ = io.WriteString(w, "{}")
			return
		}
		http.ServeFile(w, r, nativeCapture)
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
	if nativeCapture != "" {
		if _, detail, ok := strings.Cut(string(output), `<pre id="result">`); ok {
			message, _, _ := strings.Cut(detail, "</pre>")
			t.Log(html.UnescapeString(message))
		}
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
 const operating={name:'Equipment operation',series:[temp],components:[
  {componentName:'FAN',componentType:'Fan:VariableVolume',series:[series('Fan Electricity Rate','W',[point(0,0),point(1,500),point(2,null)],'FAN'),series('Fan Electricity Energy','J',[point(0,0),point(1,3600000),point(2,null)],'FAN'),series('Fan Air Mass Flow Rate','kg/s',[point(0,0),point(1,2),point(2,null)],'FAN')]},
  {componentName:'PUMP',componentType:'Pump:VariableSpeed',series:[series('Pump Electricity Rate','W',[point(0,0),point(1,500),point(2,null)],'PUMP'),series('Pump Mass Flow Rate','kg/s',[point(0,0),point(1,3),point(2,null)],'PUMP'),series('Pump Shaft Power','W',[point(0,0),point(1,400),point(2,null)],'PUMP')]},
  {componentName:'BOILER',componentType:'Boiler:HotWater',series:[series('Boiler Part Load Ratio','',[point(0,0),point(1,.5),point(2,null)],'BOILER'),series('Boiler Efficiency','',[point(0,0),point(1,.9),point(2,null)],'BOILER'),series('Boiler NaturalGas Rate','W',[point(0,0),point(1,10000),point(2,null)],'BOILER'),series('Boiler NaturalGas Energy','J',[point(0,0),point(1,3600000),point(2,null)],'BOILER'),series('Boiler Heating Energy','J',[point(0,0),point(1,7200000),point(2,null)],'BOILER')]},
  {componentName:'CHILLER',componentType:'Chiller:Electric:EIR',series:[series('Chiller Part Load Ratio','',[point(0,0),point(1,.4),point(2,null)],'CHILLER'),series('Chiller Cycling Ratio','',[point(0,0),point(1,.7),point(2,null)],'CHILLER'),series('Chiller COP','W/W',[point(0,0),point(1,4.5),point(2,null)],'CHILLER'),series('Chiller Electricity Energy','J',[point(0,0),point(1,3600000),point(2,null)],'CHILLER'),series('Chiller Evaporator Cooling Energy','J',[point(0,0),point(1,14400000),point(2,null)],'CHILLER')]},
  {componentName:'EFFICIENCY ONLY',componentType:'Boiler:HotWater',series:[series('Boiler Efficiency','',[point(0,.8),point(1,.9),point(2,null)],'EFFICIENCY ONLY')]},
  {componentName:'ELECTRIC BOILER',componentType:'Boiler:HotWater',series:[series('Boiler Electricity Rate','W',[point(0,0),point(1,10000),point(2,null)],'ELECTRIC BOILER'),series('Boiler Electricity Energy','J',[point(0,0),point(1,3600000),point(2,null)],'ELECTRIC BOILER'),series('Boiler Ancillary Electricity Rate','W',[point(0,0),point(1,100),point(2,null)],'ELECTRIC BOILER')]},
 ]};
 const operatingBefore=JSON.stringify(operating),operatingModel=prepare(operating),equipment=name=>operatingModel.entities.find(entity=>entity.name===name),operatingProperty=(owner,name)=>equipment(owner).properties.find(property=>property.name===name);
 for(const [owner,name,kind,label,unit,value]of [['FAN','Fan Electricity Energy','energy','Electric energy','kWh',1],['FAN','Fan Air Mass Flow Rate','flow','Mass flow','kg/s',2],['PUMP','Pump Shaft Power','shaftPower','Shaft power','kW',.4],['PUMP','Pump Mass Flow Rate','flow','Mass flow','kg/s',3],['BOILER','Boiler Efficiency','efficiency','Efficiency','—',.9],['BOILER','Boiler NaturalGas Rate','fuelPower','Fuel input','kW',10],['BOILER','Boiler NaturalGas Energy','fuelEnergy','Fuel energy','kWh',1],['BOILER','Boiler Heating Energy','heatingEnergy','Heating energy','kWh',2],['CHILLER','Chiller Part Load Ratio','status','PLR','—',.4],['CHILLER','Chiller Cycling Ratio','status','Cycling ratio','—',.7],['CHILLER','Chiller COP','cop','COP','—',4.5],['CHILLER','Chiller Evaporator Cooling Energy','coolingEnergy','Cooling energy','kWh',4],['ELECTRIC BOILER','Boiler Electricity Rate','fuelPower','Electric input','kW',10],['ELECTRIC BOILER','Boiler Electricity Energy','fuelEnergy','Electric input energy','kWh',1],['ELECTRIC BOILER','Boiler Ancillary Electricity Rate','power','Auxiliary electric power','kW',.1]]){
  const property=operatingProperty(owner,name),values=property&&trace(operatingModel,property);check(property?.kind===kind&&property.label===label&&values.unit===unit&&values.points[0].value===0&&Math.abs(values.points[1].value-value)<=2*Number.EPSILON*Math.max(1,Math.abs(value))&&values.points[2].value===null,'equipment operation property lost its native identity, concise meaning, units, real zero or missing value: '+owner+' / '+name+' '+JSON.stringify({kind:property?.kind,label:property?.label,unit:values?.unit,values:values?.points.map(point=>point.value)}));
 }
 check(['FAN','PUMP'].every(name=>!equipment(name).properties.some(property=>property.kind==='efficiency'||property.label==='PLR')),'fan/pump acquired fabricated PLR or operating efficiency');
 check(snapshot(operatingModel,1).components.every(component=>!component.metrics.some(metric=>['shaftPower','efficiency','fuelPower','fuelEnergy','energy','heatingEnergy','coolingEnergy'].includes(metric.id))),'additional Custom properties expanded the loop diagram summaries');
 check(snapshot(operatingModel,1).components.find(component=>component.name==='EFFICIENCY ONLY').status==='unknown','positive efficiency was mistaken for equipment operation');
 check(JSON.stringify(operating)===operatingBefore,'equipment property preparation mutated returned native results');
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
 const {buildHVACRuntimeAvailability:availabilityStack}=await import('/src/js/hvac-runtime-availability.js');
 const sourceIdentity=source=>JSON.stringify([source?.objectIndex,source?.objectType,source?.objectName]),parameterIdentity=parameters=>JSON.stringify((parameters||[]).map(parameter=>[parameter.name,parameter.value,parameter.objectIndex,parameter.fieldIndex]));
 let onOffSchedules=0;
 for(const fixture of fixtures){
  const executed=runtimeClone(fixtures,fixture.family),before=JSON.stringify(executed),prepared=prepareRuntime(executed),profile=selectedRuntimeProfile(prepared,fixture);
  check(profile&&profile.source.objectIndex===fixture.component.objectIndex&&profile.source.objectType===fixture.component.objectType&&profile.source.objectName===fixture.component.objectName,'executed runtime profile/source identity was lost for '+fixture.family);
  check(profile.definition.loopId===fixture.profile.loopId&&profile.definition.parameterControl.nodes.every(node=>fixture.profile.parameterControl.nodes.some(original=>node.id===original.id&&sourceIdentity(node.source)===sourceIdentity(original.source)&&parameterIdentity(node.parameters)===parameterIdentity(original.parameters))),'runtime definition was rebuilt from another component/circuit');
  check(prepared.frames.length===4&&prepared.frames.every(frame=>frame.frequency.toLowerCase()==='timestep')&&Math.abs(prepared.frames[3].x-prepared.frames[0].x-.75)<1e-8,'Timestep observations were sampled, promoted to Hourly or merged with loop-value frames');
  const frame=runtimeSnapshot(prepared,profile.id,0);
  check(profile.signals.some(signal=>signal.required)&&frame.errors.length===0,'native '+fixture.family+' fixture failed required data collection: '+JSON.stringify(frame.errors.map(signal=>[signal.role,signal.variableName,signal.status,signal.message])));
  check(frame.signals.every(signal=>signal.componentIndex===fixture.component.objectIndex&&(!signal.loopId||signal.loopId===fixture.profile.loopId)),'runtime observation borrowed a sibling equipment/circuit binding');
  onOffSchedules+=profile.signals.filter(signal=>signal.graph==='onOff'&&signal.role==='schedule').length;
  check(availabilityStack(prepared,profile.id).rows.filter(row=>row.kind==='schedule').every(row=>row.signalIds.every(id=>profile.signals.find(signal=>signal.id===id)?.graph==='availabilityDefinitions')),'OnOff control-mode/temperature schedule leaked into Boolean Availability rows for '+fixture.family);
  check(JSON.stringify(executed)===before,'runtime projection mutated captured observations or definitions');
 }
 check(onOffSchedules>0,'native fixture no longer exercises OnOff schedules distinct from availabilityDefinitions');
 const waterHeaterFixture=fixtures.find(fixture=>fixture.family==='waterheater'),waterHeater=runtimeClone(fixtures,'waterheater');
 const tankBinding=waterHeater.controlSignals.find(signal=>signal.graph==='onOff'&&signal.nodeId==='demand'&&signal.role==='sensor'&&signal.variableName==='Water Heater Tank Temperature'),tankTarget=waterHeater.controlSignals.find(signal=>signal.graph==='onOff'&&signal.role==='setpoint'&&signal.domain==='temperature_schedule'),tankDuty=waterHeater.controlSignals.find(signal=>signal.role==='activity'&&signal.variableName==='Water Heater Runtime Fraction');
 check(tankBinding&&tankTarget&&tankDuty&&[tankBinding,tankTarget,tankDuty].every(signal=>signal.required&&signal.reportingFrequency==='Timestep'&&sourceIdentity(signal.source)===sourceIdentity(waterHeaterFixture.component))&&tankBinding.keyValue===waterHeaterFixture.component.objectName&&tankDuty.keyValue===waterHeaterFixture.component.objectName,'native tank thermostat temperature, target or fractional heater duty lacks its exact selected tank source');
 check(waterHeater.controlSeries.find(series=>series.sourceId===tankTarget.seriesId).column.includes('[]'),'temperature-schedule fixture incorrectly relies on the raw dimensionless Schedule Value carrying a temperature unit');
 setReported(waterHeater,signal=>signal.role==='sensor'&&/temperature/i.test(signal.variableName),[99]);setReported(waterHeater,signal=>signal.id===tankBinding.id,[55,0,-5,62]);
 setReported(waterHeater,signal=>signal.domain==='temperature_schedule',[60,0,-5,60]);setReported(waterHeater,signal=>signal.role==='activity',[1000,0,1500,2000]);setReported(waterHeater,signal=>signal.id===tankDuty.id,[.25,0,.75,1]);
 const waterHeaterModel=prepareRuntime(waterHeater),waterHeaterProfile=selectedRuntimeProfile(waterHeaterModel,waterHeaterFixture);
 for(const [index,temperature,target,duty,activity]of [[0,55,60,.25,'active'],[1,0,0,0,'inactive'],[2,-5,-5,.75,'active'],[3,62,60,1,'active']]){
  const current=runtimeSnapshot(waterHeaterModel,waterHeaterProfile.id,index),tank=current.signals.find(signal=>signal.id===tankBinding.id),setpoint=current.signals.find(signal=>signal.id===tankTarget.id),runtimeDuty=current.signals.find(signal=>signal.id===tankDuty.id);
  check(current.errors.length===0&&tank.value===temperature&&tank.unit==='°C'&&setpoint.value===target&&setpoint.unit==='°C'&&runtimeDuty.value===duty&&runtimeDuty.unit==='—'&&runtimeDuty.domain==='continuous'&&current.activity===activity,'tank average/typed target/fractional duty was replaced by a fluid-port value, treated as an enum or lost a valid zero/negative observation');
 }
 const tankTrace=runtimeTrace(waterHeaterModel,waterHeaterModel.signals.find(signal=>signal.id===tankDuty.id));check(tankTrace.points.map(point=>point.value).join(',')==='0.25,0,0.75,1','heater duty trace did not retain every fractional native observation');
 check(availabilityStack(waterHeaterModel,waterHeaterProfile.id,waterHeaterModel.frames,1).rows.at(-1).current.state==='allowed','0°C temperature target was interpreted as an Availability OFF schedule');
 const missingDuty=structuredClone(waterHeater);missingDuty.controlSeries=missingDuty.controlSeries.filter(series=>series.sourceId!==tankDuty.seriesId);const dutyModel=prepareRuntime(missingDuty),dutyProfile=selectedRuntimeProfile(dutyModel,waterHeaterFixture),dutyCurrent=runtimeSnapshot(dutyModel,dutyProfile.id,0);check(dutyCurrent.reason==='acquisition_error'&&dutyCurrent.activity==='not_collected'&&dutyCurrent.errors.some(signal=>signal.id===tankDuty.id)&&availabilityStack(dutyModel,dutyProfile.id).activity?.current.state==='error','required native heater duty was replaced by positive heating rate when its observation was missing');
 for(const corruption of ['missing-tank','different-key','duplicate-source','missing-timestep']){
  const captured=structuredClone(waterHeater),series=captured.controlSeries.find(series=>series.sourceId===tankBinding.seriesId);
  if(corruption==='missing-tank')captured.controlSeries=captured.controlSeries.filter(series=>series.sourceId!==tankBinding.seriesId);
  if(corruption==='different-key')series.keyValue='Another water heater';
  if(corruption==='duplicate-source')captured.controlSeries.push(structuredClone(series));
  if(corruption==='missing-timestep'){series.points.shift();series.nativeIntervals?.shift();}
  const prepared=prepareRuntime(captured),profile=selectedRuntimeProfile(prepared,waterHeaterFixture),current=runtimeSnapshot(prepared,profile.id,0);
  check(current.reason==='acquisition_error'&&current.errors.some(signal=>signal.id===tankBinding.id)&&current.signals.find(signal=>signal.id===tankBinding.id).value===null,'missing/ambiguous tank mean borrowed an inlet, neighboring timestep or another tank: '+corruption);
 }
 const layeredFixture=fixtures.find(fixture=>fixture.family==='waterheater-stratified'),layered=runtimeClone(fixtures,'waterheater-stratified'),layeredNodes=layered.controlSignals.filter(signal=>signal.role==='sensor'&&/^Water Heater Temperature Node \d+$/.test(signal.variableName)),layeredDuties=layered.controlSignals.filter(signal=>signal.role==='activity'&&/Heater [12] Runtime Fraction/.test(signal.variableName));
 check(layeredNodes.map(signal=>signal.variableName).sort().join('|')==='Water Heater Temperature Node 2|Water Heater Temperature Node 4'&&layeredDuties.length===2&&layeredNodes.every(signal=>sourceIdentity(signal.source)===sourceIdentity(layeredFixture.component)&&signal.keyValue===layeredFixture.component.objectName)&&!layered.controlSignals.some(signal=>signal.variableName==='Water Heater Tank Temperature'),'stratified thermostat collapsed native element sensor nodes to the tank mean or borrowed another tank');
 setReported(layered,signal=>signal.role==='activity',[0]);setReported(layered,signal=>signal.id===layeredDuties.find(signal=>signal.variableName.includes('Heater 1')).id,[.375,0,.5,1]);setReported(layered,signal=>signal.id===layeredDuties.find(signal=>signal.variableName.includes('Heater 2')).id,[0,.25,0,.5]);
 for(const signal of layeredNodes)setReported(layered,binding=>binding.id===signal.id,[signal.variableName.endsWith('2')?58:49]);
 setReported(layered,signal=>signal.domain==='temperature_schedule'&&signal.keyValue==='Runtime Upper Target',[60]);setReported(layered,signal=>signal.domain==='temperature_schedule'&&signal.keyValue==='Runtime Lower Target',[55]);
 const layeredModel=prepareRuntime(layered),layeredProfile=selectedRuntimeProfile(layeredModel,layeredFixture);
 for(const index of [0,1]){const current=runtimeSnapshot(layeredModel,layeredProfile.id,index);check(!current.errors.length&&current.activity==='active'&&current.signals.find(signal=>signal.variableName==='Water Heater Temperature Node 2').value===58&&current.signals.find(signal=>signal.variableName==='Water Heater Temperature Node 4').value===49&&current.signals.filter(signal=>signal.role==='setpoint'&&signal.domain==='temperature_schedule').every(signal=>signal.unit==='°C'),'separate fractional heater duties or stratified native node/temperature-target values were lost');}
 for(const family of ['waterheater-hpwh-pumped','waterheater-hpwh-wrapped']){
  const fixture=fixtures.find(fixture=>fixture.family===family),captured=runtimeClone(fixtures,family),backup=fixture.sources.find(source=>source.objectType==='WaterHeater:Mixed'),control=captured.controlSignals.find(signal=>signal.variableName==='Water Heater Heat Pump Control Tank Temperature'),compressor=captured.controlSignals.find(signal=>signal.role==='activity'&&signal.variableName==='Water Heater Compressor Part Load Ratio');
  check(backup&&control&&compressor&&[control,compressor].every(signal=>sourceIdentity(signal.source)===sourceIdentity(fixture.component)&&signal.keyValue===fixture.component.objectName)&&!captured.controlSignals.some(signal=>signal.componentIndex===fixture.component.objectIndex&&signal.role==='activity'&&signal.variableName.includes('Runtime Fraction')),'heat-pump wrapper borrowed backup tank mean/duty as its compressor thermostat or operating state: '+family);
  setReported(captured,signal=>signal.id===control.id,[54,60,0,-5]);setReported(captured,signal=>signal.id===compressor.id,[.5,0,.25,1]);setReported(captured,signal=>signal.domain==='temperature_schedule',[60]);
  const ownSeries=captured.controlSeries.find(series=>series.sourceId===compressor.seriesId),backupSeries={...structuredClone(ownSeries),sourceId:family+'-backup-duty',keyValue:backup.objectName,name:'Water Heater Runtime Fraction',column:backup.objectName+':Water Heater Runtime Fraction [](Timestep)'};backupSeries.points.forEach(point=>point.value=0);
  const backupBinding={...compressor,id:family+'-backup-binding',seriesId:backupSeries.sourceId,source:backup,keyValue:backup.objectName,variableName:backupSeries.name};captured.controlSeries.push(backupSeries);captured.controlSignals.push(backupBinding);
  const prepared=prepareRuntime(captured),profile=selectedRuntimeProfile(prepared,fixture);
  for(const [index,activity]of [[0,'active'],[1,'inactive'],[2,'active'],[3,'active']]){const current=runtimeSnapshot(prepared,profile.id,index);check(!current.errors.length&&current.activity===activity&&availabilityStack(prepared,profile.id,prepared.frames,index).activity?.current.state===activity&&current.signals.find(signal=>signal.id===control.id).unit==='°C'&&current.signals.some(signal=>signal.id===backupBinding.id&&signal.value===0),'child backup fraction 0 overrode selected compressor activity/header, or exact native context disappeared: '+family);}
  const indexAlias=structuredClone(captured);indexAlias.controlSignals.find(signal=>signal.id===backupBinding.id).source.objectIndex=fixture.component.objectIndex;const aliasModel=prepareRuntime(indexAlias),aliasProfile=selectedRuntimeProfile(aliasModel,fixture);check(runtimeSnapshot(aliasModel,aliasProfile.id,0).activity==='active'&&availabilityStack(aliasModel,aliasProfile.id).activity?.current.state==='active','same object index with another source type/name acquired selected compressor activity precedence: '+family);
  const missingOwner=structuredClone(captured);missingOwner.controlSeries=missingOwner.controlSeries.filter(series=>series.sourceId!==compressor.seriesId);setReported(missingOwner,signal=>signal.id===backupBinding.id,[1]);const missingModel=prepareRuntime(missingOwner),missingProfile=selectedRuntimeProfile(missingModel,fixture),missingCurrent=runtimeSnapshot(missingModel,missingProfile.id,0);
  check(missingCurrent.reason==='acquisition_error'&&missingCurrent.activity==='not_collected'&&missingCurrent.errors.some(signal=>signal.id===compressor.id)&&availabilityStack(missingModel,missingProfile.id).activity?.current.state==='error','missing own compressor observation borrowed active backup duty or became a successful inactive state: '+family);
  const missingChild=structuredClone(captured);missingChild.controlSeries=missingChild.controlSeries.filter(series=>series.sourceId!==backupBinding.seriesId);const childModel=prepareRuntime(missingChild),childProfile=selectedRuntimeProfile(childModel,fixture),childCurrent=runtimeSnapshot(childModel,childProfile.id,0);
  check(childCurrent.activity==='active'&&childCurrent.errors.some(signal=>signal.id===backupBinding.id)&&availabilityStack(childModel,childProfile.id).activity?.current.state==='active','required missing child context was hidden or overwrote a reported own compressor activity/header: '+family);
 }
 const fanFixture=fixtures.find(fixture=>fixture.family==='fan'),fanLoop=runtimeClone(fixtures,'fan');
 const fanGate=expressionNodes(fanFixture.profile.finalAvailability).find(expression=>expression.op==='fan_gate'),ownSchedule=expressionNodes(fanGate?.children?.[1]).find(expression=>expression.op==='schedule_positive')?.scheduleName;
 check(ownSchedule,'parsed fan no longer exercises own schedule with manager overrides');
 setReported(fanLoop,signal=>signal.role==='activity',[0]);
 setReported(fanLoop,signal=>signal.role==='manager_status'||signal.role==='loop_status',[0]);
 setReported(fanLoop,signal=>signal.role==='schedule'&&signal.keyValue.toLowerCase()===ownSchedule.toLowerCase(),[0]);
 let runtimeModel=prepareRuntime(fanLoop),fanProfile=selectedRuntimeProfile(runtimeModel,fanFixture),observed=runtimeSnapshot(runtimeModel,fanProfile.id,0);
 check(observed.permission.state==='blocked'&&observed.reason==='permission_blocked'&&observed.availability.some(signal=>signal.role==='manager_status'&&signal.state==='no_action'&&signal.value===0),'NoAction=0 was treated as Boolean OFF or allowed the fan schedule to be bypassed');
 const cycle=structuredClone(fanLoop);setReported(cycle,signal=>signal.role==='manager_status'||signal.role==='loop_status',[2]);
 runtimeModel=prepareRuntime(cycle);observed=runtimeSnapshot(runtimeModel,fanProfile.id,0);
 check(observed.permission.state==='allowed'&&observed.availability.some(signal=>signal.state==='cycle_on'),'CycleOn failed to override the fan own schedule');
 const forcedFixture=fixtures.find(fixture=>fixture.family==='fan-forced'),forced=runtimeClone(fixtures,'fan-forced');
 check(forcedFixture.profile.availabilityDefinitions.nodes.some(node=>node.code==='availability_manager'&&node.source.objectType==='AvailabilityManager:ScheduledOff'&&node.parameters.some(parameter=>parameter.name==='Schedule Name'&&parameter.value==='Runtime Force Off')),'ForceOff fixture is not backed by a parsed ScheduledOff definition');
 setReported(forced,signal=>signal.role==='activity',[0]);setReported(forced,signal=>signal.role==='schedule',[1]);
 setReported(forced,signal=>signal.role==='schedule'&&signal.keyValue==='Runtime Force Off',[0]);setReported(forced,signal=>signal.role==='manager_status'||signal.role==='loop_status',[1]);
 const forcedModel=prepareRuntime(forced),forcedProfile=selectedRuntimeProfile(forcedModel,forcedFixture);
 observed=runtimeSnapshot(forcedModel,forcedProfile.id,0);
 check(observed.permission.state==='blocked'&&observed.reason==='permission_blocked'&&observed.availability.some(signal=>signal.state==='force_off'),'reported ForceOff was replaced by a positive source schedule');
 check(forcedProfile.signals.filter(signal=>['manager_status','loop_status'].includes(signal.role)).every(signal=>!signal.possibleStates),'NightCycle-only output restriction leaked into a ScheduledOff manager or its loop');
 for(const invalidCode of [1,1.5]){
  const invalidManager=structuredClone(cycle);setReported(invalidManager,signal=>signal.role==='manager_status',[invalidCode]);
  observed=runtimeSnapshot(prepareRuntime(invalidManager),fanProfile.id,0);
  check(observed.reason==='acquisition_error'&&observed.errors.some(signal=>signal.role==='manager_status')&&observed.availability.some(signal=>signal.role==='manager_status'&&signal.value===invalidCode&&signal.state==='error'),'invalid native NightCycle action was accepted or converted into an ambiguous availability state');
 }
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

 for(const [fixture,captured,expected,kind] of [[fanFixture,fanLoop,'blocked','schedule'],[fanFixture,cycle,'allowed','manager'],[forcedFixture,forced,'blocked','manager']]){
  const before=JSON.stringify(captured),prepared=prepareRuntime(captured),profile=selectedRuntimeProfile(prepared,fixture),stack=availabilityStack(prepared,profile.id,prepared.frames,0);
  const final=stack.rows.at(-1),own=stack.rows.find(row=>row.kind==='schedule'&&row.label.toLowerCase()===ownSchedule.toLowerCase());
  check(final.id==='final-availability'&&final.kind==='permission'&&final.current.state===expected,'stack final disagrees with native manager/fan override');
  check(stack.rows.some(row=>row.kind===kind&&row.decisive)&&stack.rows.every(row=>row.owner&&row.current.label),'stack lost deciding source or owning equipment');
  if(kind==='manager')check(!own?.decisive,'override still attributes decision to ignored own schedule');
  check(stack.rows.filter(row=>row.kind==='schedule').length===new Set(stack.rows.filter(row=>row.kind==='schedule').map(row=>row.label.toLowerCase())).size,'same schedule is repeated instead of retaining owners');
  const later=availabilityStack(prepared,profile.id,prepared.frames,1);
  check(stack.rows.every((row,index)=>row.points===later.rows[index].points&&row.points.length===4),'moving current frame rebuilt/sampled availability history');
  const absent=availabilityStack(prepared,profile.id,prepared.frames,-1);
  check(!absent.frame&&absent.rows.every(row=>row.current.state==='error'&&row.current.value===null&&!row.decisive),'unmatched shared frame borrowed first observation');
  check(!availabilityStack(prepared,'wrong-profile').profile&&!availabilityStack(prepared,'wrong-profile').rows.length,'invalid explicit equipment borrowed another profile');
  check(JSON.stringify(captured)===before,'stack mutated native definitions or observations');
 }
 const constantManagerStack=availabilityStack(prepareRuntime(noOverride),fanProfile.id);
 check(constantManagerStack.rows.at(-1).current.state==='allowed'&&!constantManagerStack.rows.some(row=>row.kind==='condition'&&row.operator==='always'),'manager NoAction0 became Boolean OFF condition');
 check(availabilityStack(prepareRuntime(booleanStop),fanProfile.id).rows.at(-1).current.state==='blocked','stack Boolean0 lost distinct false permission');

 // The default VAV_3 example can run outside its fan schedule. The schedule
 // is an input to a native override rule, not an unconditional AND veto.
 const vav3Fixture=fixtures.find(fixture=>fixture.family==='fan-vav3');
 check(vav3Fixture?.component.objectName==='VAV_3_Fan'&&vav3Fixture.topology.name==='VAV_3','CycleOn acceptance lost the actual default-sample fan occurrence');
 const vav3Manager=vav3Fixture.profile.availabilityDefinitions.nodes.find(node=>node.source?.objectName==='VAV_3 Availability Manager');
 check(vav3Manager?.source.objectType==='AvailabilityManager:NightCycle'&&vav3Manager.parameters.some(parameter=>parameter.name==='Control Type'&&parameter.value==='CycleOnAny')&&vav3Manager.parameters.some(parameter=>parameter.name==='Cycling Run Time'&&Number(parameter.value)===1800),'default VAV_3 manager control/running period changed');
 const vav3Gate=expressionNodes(vav3Fixture.profile.finalAvailability).find(expression=>expression.op==='fan_gate');
 const vav3Schedule=expressionNodes(vav3Gate?.children?.[1]).find(expression=>expression.op==='schedule_positive')?.scheduleName;
 check(vav3Schedule==='HVACOperationSchd','VAV_3 native own fan schedule was not retained');
 for(const [scheduleValue,managerValue,expected,decidingKind] of [[0,2,'allowed','manager'],[0,2.0000000000000004,'allowed','manager'],[0,0,'blocked','schedule']]){
  const captured=runtimeClone(fixtures,'fan-vav3');
  setReported(captured,signal=>signal.role==='schedule'&&signal.keyValue.toLowerCase()===vav3Schedule.toLowerCase(),[scheduleValue]);
  setReported(captured,signal=>signal.role==='manager_status'||signal.role==='loop_status',[managerValue]);
  setReported(captured,signal=>signal.role==='activity',[expected==='allowed'?2443.8340290316523:managerValue===1?1221.9170145158262:0]);
  const prepared=prepareRuntime(captured),profile=selectedRuntimeProfile(prepared,vav3Fixture),current=runtimeSnapshot(prepared,profile.id,0),stack=availabilityStack(prepared,profile.id,prepared.frames,0);
  const scheduleRow=stack.rows.find(row=>row.kind==='schedule'&&row.label===vav3Schedule),fanPermission=stack.rows.find(row=>row.kind==='condition'&&row.operator==='fan_gate');
  check(current.errors.length===0&&current.permission.state===expected&&fanPermission?.current.state===expected&&stack.rows.at(-1).current.state===expected,'VAV_3 final availability incorrectly ANDed the schedule with manager override');
  check(scheduleRow?.current.value===scheduleValue&&stack.rows.some(row=>row.kind===decidingKind&&row.decisive),'VAV_3 schedule input or deciding source was hidden by final permission');
  check(profile.signals.filter(signal=>signal.role==='manager_status').every(signal=>JSON.stringify(signal.possibleStates)==='[0,2]')&&profile.signals.filter(signal=>signal.role==='loop_status').every(signal=>!signal.possibleStates),'native manager action domain was not source-specific or was incorrectly inferred for aggregate loop overrides');
  if(Math.abs(managerValue-2)<1e-7){
   check(scheduleRow.current.state==='blocked'&&!scheduleRow.decisive&&current.activity==='active'&&stack.rows.some(row=>row.kind==='manager'&&row.current.state==='cycle_on'&&row.decisive),'OFF input was changed to ON instead of retaining the actual CycleOn override/activity');
  }
 }
 for(const [controlType,codes,invalid,state] of [['StayOff',[0],0,'no_action'],['CycleOnAny',[0,2],1,'error'],['CycleOnControlZone',[0,2],1,'error'],['CycleOnAnyCoolingZone',[0,2],1,'error'],['CycleOnAnyHeatingZone',[0,2],1,'error'],['CycleOnAnyZoneFansOnly',[0,3],2,'error'],['CycleOnAnyHeatingZoneFansOnly',[0,3],2,'error'],['CycleOnAnyCoolingOrHeatingZone',[0,2,3],2,'cycle_on']]){
  const captured=runtimeClone(fixtures,'fan-vav3'),definition=captured.operatingProfiles.find(profile=>profile.componentIndex===vav3Fixture.component.objectIndex);
  const manager=definition.availabilityDefinitions.nodes.find(node=>sourceIdentity(node.source)===sourceIdentity(vav3Manager.source));
  manager.parameters.find(parameter=>parameter.name==='Control Type').value=controlType;
  const prepared=prepareRuntime(captured),profile=selectedRuntimeProfile(prepared,vav3Fixture),signals=profile.signals.filter(signal=>signal.role==='manager_status');
  check(signals.length===1&&signals.every(signal=>JSON.stringify(signal.possibleStates)===JSON.stringify(codes)&&runtime.hvacRuntimeSignalState(signal,invalid)===state),'NightCycle control-specific action domain was confused with another mode: '+controlType);
 }
 for(const mismatch of ['owning-loop','manager-source','another-manager']){
  const captured=runtimeClone(fixtures,'fan-vav3'),definition=captured.operatingProfiles.find(profile=>profile.componentIndex===vav3Fixture.component.objectIndex),layer=definition.availabilityLayers.find(layer=>layer.role==='loop');
  if(mismatch==='owning-loop')layer.owner.objectIndex++;
  if(mismatch==='manager-source')definition.availabilityDefinitions.nodes.find(node=>sourceIdentity(node.source)===sourceIdentity(vav3Manager.source)).source.objectIndex++;
  if(mismatch==='another-manager')layer.expression.children[0].children.push({op:'manager_unknown',assignmentOwner:layer.owner,source:{objectIndex:999999,objectType:'AvailabilityManager:ScheduledOff',objectName:'Other manager'}});
  const prepared=prepareRuntime(captured),profile=selectedRuntimeProfile(prepared,vav3Fixture),aggregate=profile.signals.find(signal=>signal.role==='loop_status');
  check(aggregate&&!aggregate.possibleStates,'owning-loop action domain was guessed from incomplete or unrelated manager provenance: '+mismatch);
 }
 const vav3Gap=runtimeClone(fixtures,'fan-vav3');
 setReported(vav3Gap,signal=>signal.role==='manager_status'||signal.role==='loop_status',[2]);
 setReported(vav3Gap,signal=>signal.role==='schedule'&&signal.keyValue.toLowerCase()===vav3Schedule.toLowerCase(),[0]);
 const missingVav3Manager=vav3Gap.controlSignals.find(signal=>signal.role==='manager_status');
 const missingVav3Series=vav3Gap.controlSeries.find(series=>series.sourceId===missingVav3Manager.seriesId);
 missingVav3Series.nativeIntervals=structuredClone(missingVav3Series.nativeIntervals||vav3Gap.controlSeries.find(series=>series.sourceId===missingVav3Series.nativeIntervalsSourceId).nativeIntervals);delete missingVav3Series.nativeIntervalsSourceId;
 for(const dependent of vav3Gap.controlSeries.filter(series=>series.nativeIntervalsSourceId===missingVav3Series.sourceId)){dependent.nativeIntervals=structuredClone(missingVav3Series.nativeIntervals);delete dependent.nativeIntervalsSourceId;}
 missingVav3Series.points.splice(1,1);missingVav3Series.nativeIntervals.splice(1,1);
 const vav3GapModel=prepareRuntime(vav3Gap),vav3GapProfile=selectedRuntimeProfile(vav3GapModel,vav3Fixture);
 check(runtimeSnapshot(vav3GapModel,vav3GapProfile.id,0).errors.length===0&&runtimeSnapshot(vav3GapModel,vav3GapProfile.id,1).reason==='acquisition_error'&&availabilityStack(vav3GapModel,vav3GapProfile.id,vav3GapModel.frames,1).rows.some(row=>row.kind==='manager'&&row.current.state==='error'),'missing manager action borrowed another timestep to satisfy OFF schedule or damaged other native frames');

 // Keep native substeps inside a shared zone-timestep frame. Their action
 // codes remain discrete; neither an arithmetic mean nor a last sample from
 // a nearby timestamp is a substitute for the reported interval.
 const substeps=runtimeClone(fixtures,'fan-vav3');
 setReported(substeps,signal=>signal.role==='schedule'&&signal.keyValue===vav3Schedule,[0]);
 const statusIDs=new Set(substeps.controlSignals.filter(signal=>['manager_status','loop_status'].includes(signal.role)).map(signal=>signal.seriesId));
 for(const series of substeps.controlSeries.filter(series=>statusIDs.has(series.sourceId))){
  delete series.displayPoints;delete series.displayColumn;delete series.displayUnit;delete series.nativeIntervalsSourceId;
  series.points=[5,15,30,45,60].map((minute,index)=>({x:100+index,label:'01-01 '+String(Math.floor(minute/60)).padStart(2,'0')+':'+String(minute%60).padStart(2,'0'),value:index===0?0:2}));
  series.nativeIntervals=[5,15,30,45,60].map((minute,index,list)=>({timeIndex:100+index,environmentPeriodIndex:1,simulationDays:1,year:2026,intervalMinutes:minute-(list[index-1]||0)}));
 }
 const substepsBefore=JSON.stringify(substeps),substepModel=prepareRuntime(substeps),substepProfile=selectedRuntimeProfile(substepModel,vav3Fixture),substepStack=availabilityStack(substepModel,substepProfile.id,substepModel.frames,0),substepFinal=substepStack.rows.at(-1);
 check(substepModel.frames.length===4&&substepModel.frames.every(frame=>frame.frequency.toLowerCase()==='timestep'),'Detailed output inserted extra shared frames or changed frame-navigation cadence');
 check(runtimeSnapshot(substepModel,substepProfile.id,0).errors.length===0&&substepFinal.current.state==='allowed','shared-frame end did not use its unique exact native status endpoint');
 check(substepFinal.points.length===5&&substepFinal.points[0].state==='blocked'&&substepFinal.points[1].state==='allowed'&&Math.abs((substepFinal.points[0].endX-substepFinal.points[0].startX)*60-5)<1e-6&&Math.abs((substepFinal.points[1].endX-substepFinal.points[1].startX)*60-10)<1e-6,'native OFF/ON subintervals were averaged, resampled or widened to the whole zone timestep');
 check(substepFinal.points.every(point=>point.state==='allowed'||point.state==='blocked')&&JSON.stringify(substeps)===substepsBefore,'valid native actions gained an ambiguous state or mutated raw output');
 const twoMinute=structuredClone(substeps);
 for(const series of twoMinute.controlSeries.filter(series=>statusIDs.has(series.sourceId))){
  const ends=[2,4,6,8,10,15,30,45,60];
  series.points=ends.map((minute,index)=>({x:200+index,label:'01-01 '+String(Math.floor(minute/60)).padStart(2,'0')+':'+String(minute%60).padStart(2,'0'),value:index%2?2:0}));
  series.nativeIntervals=ends.map((minute,index)=>({timeIndex:200+index,environmentPeriodIndex:1,simulationDays:1,year:2026,intervalMinutes:minute-(ends[index-1]||0)}));
 }
 const twoMinuteModel=prepareRuntime(twoMinute),twoMinuteProfile=selectedRuntimeProfile(twoMinuteModel,vav3Fixture),twoMinuteHistory=availabilityStack(twoMinuteModel,twoMinuteProfile.id,twoMinuteModel.frames,0).rows.at(-1).points;
 check(twoMinuteHistory.length===9&&twoMinuteHistory.every(point=>point.endX-point.startX>1e-7),'floating-point noise at a shared native boundary created extra almost-zero-duration status patches');
 const nativeAggregate=structuredClone(substeps);setReported(nativeAggregate,signal=>signal.role==='loop_status',[1]);
 const aggregateModel=prepareRuntime(nativeAggregate),aggregateProfile=selectedRuntimeProfile(aggregateModel,vav3Fixture),aggregateStack=availabilityStack(aggregateModel,aggregateProfile.id,aggregateModel.frames,0);
 check(runtimeSnapshot(aggregateModel,aggregateProfile.id,0).errors.length===0&&aggregateStack.rows.at(-1).current.state==='blocked'&&aggregateStack.rows.some(row=>row.signals?.some(signal=>signal.role==='loop_status')&&row.current.state==='force_off'&&row.decisive)&&aggregateStack.rows.some(row=>row.signals?.some(signal=>signal.role==='manager_status')&&row.current.state==='cycle_on'&&!row.decisive),'native loop aggregate override was replaced by recomputed assignment priority');
 const foreignAggregate=structuredClone(nativeAggregate);foreignAggregate.controlSignals.filter(signal=>signal.role==='loop_status').forEach(signal=>signal.source.objectIndex++);
 const foreignAggregateModel=prepareRuntime(foreignAggregate),foreignAggregateProfile=selectedRuntimeProfile(foreignAggregateModel,vav3Fixture);
 check(runtimeSnapshot(foreignAggregateModel,foreignAggregateProfile.id,0).permission.state==='allowed','another source object was allowed to replace the owning-loop manager aggregate');
 const aliasedTiming=structuredClone(substeps),aliasedSeries=aliasedTiming.controlSeries.filter(series=>statusIDs.has(series.sourceId));
 aliasedSeries[1].nativeIntervalsSourceId=aliasedSeries[0].sourceId;delete aliasedSeries[1].nativeIntervals;
 const aliasedModel=prepareRuntime(aliasedTiming),aliasedProfile=selectedRuntimeProfile(aliasedModel,vav3Fixture);
 check(runtimeSnapshot(aliasedModel,aliasedProfile.id,0).errors.length===0&&availabilityStack(aliasedModel,aliasedProfile.id,aliasedModel.frames,0).rows.at(-1).points.length===5,'shared native timing metadata was not resolved inside the exact ControlSeries axis');
 for(const invalidTiming of ['different-axis','missing-owner','zero-duration','other-environment','other-year','invalid-year','no-endpoint','fractional-action','wrong-frequency']){
  const captured=structuredClone(aliasedTiming),series=captured.controlSeries.find(series=>series.sourceId===aliasedSeries[1].sourceId),binding=captured.controlSignals.find(signal=>signal.seriesId===series.sourceId);
  if(invalidTiming==='different-axis')series.points[1].x++;
  if(invalidTiming==='missing-owner')series.nativeIntervalsSourceId='not-a-native-timeline';
  if(['zero-duration','other-environment','other-year','invalid-year'].includes(invalidTiming)){
   series.nativeIntervals=structuredClone(aliasedSeries[0].nativeIntervals);delete series.nativeIntervalsSourceId;
   if(invalidTiming==='zero-duration')series.nativeIntervals[1].intervalMinutes=0;
   else if(invalidTiming==='other-environment')series.nativeIntervals.forEach(interval=>interval.environmentPeriodIndex=2);
   else series.nativeIntervals.forEach(interval=>interval.year=invalidTiming==='other-year'?2027:2026.5);
  }
  if(invalidTiming==='no-endpoint'){series.points[1].label='01-01 00:14';delete series.nativeIntervalsSourceId;series.nativeIntervals=structuredClone(aliasedSeries[0].nativeIntervals);}
  if(invalidTiming==='fractional-action')series.points[1].value=1.5;
  if(invalidTiming==='wrong-frequency'){binding.reportingFrequency='Timestep';series.reportingFrequency='Timestep';}
  const prepared=prepareRuntime(captured),profile=selectedRuntimeProfile(prepared,vav3Fixture),current=runtimeSnapshot(prepared,profile.id,0);
  check(current.reason==='acquisition_error'&&current.errors.some(signal=>signal.id===binding.id),'invalid native availability contract was filled, averaged or silently accepted: '+invalidTiming);
  if(invalidTiming==='other-year')check(availabilityStack(prepared,profile.id,prepared.frames,0).rows.find(row=>row.signalIds?.includes(binding.id)).points.every(point=>point.state==='error'),'native history borrowed an interval from another simulation year');
 }

 const coilFixture=fixtures.find(fixture=>fixture.family==='coil'),coilLoop=runtimeClone(fixtures,'coil');
 setReported(coilLoop,signal=>signal.role==='sensor'&&/temperature/i.test(signal.variableName),[22]);setReported(coilLoop,signal=>signal.role==='setpoint',[20]);
 setReported(coilLoop,signal=>signal.role==='actuator_request',[.8]);setReported(coilLoop,signal=>signal.role==='actuator',[.3]);
 runtimeModel=prepareRuntime(coilLoop);const coilProfile=selectedRuntimeProfile(runtimeModel,coilFixture);observed=runtimeSnapshot(runtimeModel,coilProfile.id,0);
 check(observed.errors.length===0&&observed.residuals.some(residual=>residual.value===2&&residual.unit==='°C'),'native sensed-minus-setpoint residual lost its actual measurement/target sign');
 check(observed.signals.some(signal=>signal.role==='actuator_request'&&signal.value===.8)&&observed.signals.some(signal=>signal.role==='actuator'&&signal.value===.3),'requested and actual actuator flows were conflated');
 const coilRuntimeBaseline=JSON.stringify(observed),operatingOnly=structuredClone(coilLoop),optionalBinding={...operatingOnly.controlSignals[0],id:'optional-operating',graph:'equipment',role:'operating',required:false,status:'collected',seriesId:'optional-operating-series',variableName:'Boiler Efficiency',keyValue:'Extra equipment'};
 const optionalSeries={sourceId:optionalBinding.seriesId,file:'other-extra.csv',name:optionalBinding.variableName,keyValue:optionalBinding.keyValue,column:'Extra equipment:Boiler Efficiency [] (Timestep)',reportingFrequency:'Timestep',rowCount:12000};
 Object.defineProperty(optionalSeries,'points',{enumerable:true,get(){throw Error('optional equipment observations were eagerly read by the runtime model');}});
 operatingOnly.controlSignals.push(optionalBinding);operatingOnly.controlSeries.push(optionalSeries);
 const operatingOnlyRuntime=prepareRuntime(operatingOnly);
 check(operatingOnlyRuntime.primary===runtimeModel.primary&&JSON.stringify(operatingOnlyRuntime.frames)===JSON.stringify(runtimeModel.frames)&&JSON.stringify(runtimeSnapshot(operatingOnlyRuntime,coilProfile.id,0))===coilRuntimeBaseline,'optional equipment output changed runtime primary, control frames or the existing availability/control snapshot');
 const optionalAxis=structuredClone(coilLoop),axisTarget=optionalAxis.controlSeries.find(series=>series.reportingFrequency==='Timestep'),axisSource=coilLoop.controlSeries.find(series=>series.sourceId===axisTarget.sourceId),axisOwner={...structuredClone(axisTarget),sourceId:'optional-axis-owner',name:'Shared native axis',column:'Shared native axis [] (Timestep)',nativeIntervals:structuredClone(runtimeModel.nativeIntervalsFor(axisSource))};
 delete axisOwner.nativeIntervalsSourceId;delete axisOwner.displayPoints;
 for(const point of axisOwner.points)Object.defineProperty(point,'value',{enumerable:true,get(){throw Error('axis metadata owner values were read for an excluded operating signal');}});
 for(const series of optionalAxis.controlSeries)if(series.nativeIntervalsSourceId===axisTarget.sourceId)series.nativeIntervalsSourceId=axisOwner.sourceId;
 axisTarget.nativeIntervalsSourceId=axisOwner.sourceId;delete axisTarget.nativeIntervals;
 optionalAxis.controlSeries.push(axisOwner);optionalAxis.controlSignals.push({...optionalBinding,id:'optional-axis-binding',seriesId:axisOwner.sourceId,variableName:axisOwner.name,keyValue:axisOwner.keyValue});
 const optionalAxisRuntime=prepareRuntime(optionalAxis);
 check(JSON.stringify(optionalAxisRuntime.nativeIntervalsFor(axisTarget))===JSON.stringify(axisOwner.nativeIntervals),'excluding an operating channel lost the native interval metadata it owns for required control observations');
 const runtimeEvidence=snapshot=>JSON.stringify(snapshot,(key,value)=>key==='series'?undefined:value);
 check(runtimeEvidence(runtimeSnapshot(optionalAxisRuntime,coilProfile.id,0))===runtimeEvidence(observed),'native metadata ownership alias changed required control values, intervals or decisions');
 const coilDefinition=captured=>captured.operatingProfiles.find(profile=>profile.componentIndex===coilFixture.component.objectIndex&&profile.loopId===coilFixture.profile.loopId),coilGraph=coilDefinition(coilLoop).parameterControl;
 const coilPort=(code,field)=>coilGraph.nodes.find(node=>node.code===code&&sourceIdentity(node.source)===sourceIdentity(coilFixture.component)&&node.parameters?.some(parameter=>parameter.name===field&&parameter.objectIndex===coilFixture.component.objectIndex));
 const coilAirInlet=coilPort('inlet_state','Air Inlet Node Name'),coilAirOutlet=coilPort('outlet_state','Air Outlet Node Name'),coilWaterInlet=coilPort('inlet_state','Water Inlet Node Name');
 check(coilAirInlet&&coilAirOutlet&&coilWaterInlet,'native water coil must retain distinct source-backed air and water ports');
 const portTemperature=(captured,node,value)=>setReported(captured,signal=>signal.graph==='parameterControl'&&signal.nodeId===node.id&&signal.role==='sensor'&&signal.variableName==='System Node Temperature',[value]);
 const convergedCoil=structuredClone(coilLoop);portTemperature(convergedCoil,coilAirInlet,10);portTemperature(convergedCoil,coilAirOutlet,20);portTemperature(convergedCoil,coilWaterInlet,82);
 const convergedSnapshot=runtimeSnapshot(prepareRuntime(convergedCoil),coilProfile.id,0),convergedResidual=convergedSnapshot.residuals.find(residual=>residual.thermal);
 check(convergedSnapshot.errors.length===0&&convergedResidual?.value===0&&convergedResidual.thermal.unit==='K'&&convergedResidual.thermal.inletToTarget===10&&convergedResidual.thermal.inletToOutlet===10,'zero reported tracking error erased the nonzero target/actual air-stream temperature change');
 check(convergedResidual.thermal.inlet.nodeId===coilAirInlet.id&&convergedResidual.thermal.inlet.value===10&&convergedResidual.thermal.outlet.nodeId===coilAirOutlet.id&&convergedResidual.thermal.outlet.value===20&&convergedSnapshot.signals.some(signal=>signal.role==='actuator_request'&&signal.value===.8)&&convergedSnapshot.signals.some(signal=>signal.role==='actuator'&&signal.value===.3),'thermal comparison borrowed water-inlet temperature or treated converged error as zero commanded/actual water flow');
 for(const [inlet,outlet,target,targetChange,actualChange=targetChange,error=0]of [[30,20,20,-10],[-10,0,0,10],[-20,-5,-5,15],[0,0,0,0],[10,18,20,10,8,-2]]){
  const captured=structuredClone(convergedCoil);portTemperature(captured,coilAirInlet,inlet);portTemperature(captured,coilAirOutlet,outlet);setReported(captured,signal=>signal.role==='setpoint',[target]);
  const current=runtimeSnapshot(prepareRuntime(captured),coilProfile.id,0),residual=current.residuals.find(entry=>entry.thermal);
  check(current.errors.length===0&&residual?.value===error&&residual.thermal.inletToTarget===targetChange&&residual.thermal.inletToOutlet===actualChange,'air-stream differences lost cooling/tracking sign or a valid zero/negative native temperature: '+[inlet,outlet,target]);
 }
 const airInletBinding=convergedCoil.controlSignals.find(signal=>signal.graph==='parameterControl'&&signal.nodeId===coilAirInlet.id&&signal.role==='sensor'&&signal.variableName==='System Node Temperature');
 const noThermal=(captured,message)=>check(!runtimeSnapshot(prepareRuntime(captured),coilProfile.id,0).residuals.some(residual=>residual.thermal),message);
 for(const variant of ['missing-inlet','missing-air-port','duplicate-inlet','duplicate-edge','different-source','different-controller','broken-command','different-sensor','missing-native-interval','different-cadence','different-interval','different-environment','different-timestep']){
  const captured=structuredClone(convergedCoil),graph=coilDefinition(captured).parameterControl,binding=captured.controlSignals.find(signal=>signal.id===airInletBinding.id),native=captured.controlSeries.find(series=>series.sourceId===binding.seriesId);
  if(variant==='missing-inlet')captured.controlSignals=captured.controlSignals.filter(signal=>signal.id!==binding.id);
  if(variant==='missing-air-port')graph.nodes=graph.nodes.filter(node=>node.id!==coilAirInlet.id);
  if(variant==='duplicate-inlet')captured.controlSignals.push({...binding,id:binding.id+'-duplicate'});
  if(variant==='duplicate-edge')graph.edges.push({...graph.edges.find(edge=>edge.from===coilAirInlet.id&&edge.relation==='inlet_conditions')});
  if(variant==='different-source')binding.source={...binding.source,objectName:'Another water coil'};
  if(variant==='different-controller')graph.nodes.find(node=>node.code==='water_flow_command').source.objectName='Another controller';
  if(variant==='broken-command')graph.edges=graph.edges.filter(edge=>edge.relation!=='actuates');
  if(variant==='different-sensor')graph.edges.find(edge=>edge.relation==='adds_measurement').from=coilWaterInlet.id;
  if(variant==='missing-native-interval'){delete native.nativeIntervals;delete native.nativeIntervalsSourceId;}
  if(variant==='different-cadence'){binding.reportingFrequency='Detailed';native.reportingFrequency='Detailed';}
  if(['different-interval','different-environment','different-timestep'].includes(variant)){
   native.nativeIntervals=structuredClone(captured.controlSeries.find(series=>series.nativeIntervals)?.nativeIntervals);delete native.nativeIntervalsSourceId;
   check(native.nativeIntervals?.length===4,'thermal timing regression lacks native source metadata');
   for(const interval of native.nativeIntervals){if(variant==='different-interval')interval.intervalMinutes=5;if(variant==='different-environment')interval.environmentPeriodIndex=2;if(variant==='different-timestep')interval.timeIndex+=100;}
  }
  noThermal(captured,'thermal comparison borrowed missing, ambiguous or unrelated air/control/native observations: '+variant);
 }
 const inletGap=structuredClone(convergedCoil),inletGapSeries=inletGap.controlSeries.find(series=>series.sourceId===airInletBinding.seriesId);inletGapSeries.points.shift();
 noThermal(inletGap,'missing inlet timestep borrowed a neighboring air temperature');
 const unpairedTarget=structuredClone(convergedCoil),targetBinding=unpairedTarget.controlSignals.find(signal=>signal.graph==='parameterControl'&&signal.role==='setpoint'&&signal.nodeId===convergedResidual.setpoint.nodeId),targetSeries=unpairedTarget.controlSeries.find(series=>series.sourceId===targetBinding.seriesId);
 delete targetSeries.nativeIntervals;delete targetSeries.nativeIntervalsSourceId;
 const actualOnly=runtimeSnapshot(prepareRuntime(unpairedTarget),coilProfile.id,0).residuals.find(residual=>residual.thermal);
 check(actualOnly?.thermal.inletToOutlet===10&&!('inletToTarget'in actualOnly.thermal),'unproved target interval damaged a valid actual air change or acquired an invented target change');
 const foreignTarget=structuredClone(convergedCoil),foreignTargetBinding=foreignTarget.controlSignals.find(signal=>signal.id===targetBinding.id),foreignTargetSeries=foreignTarget.controlSeries.find(series=>series.sourceId===foreignTargetBinding.seriesId);foreignTargetBinding.keyValue=foreignTargetSeries.keyValue='Another controlled air outlet';
 const foreignTargetResidual=runtimeSnapshot(prepareRuntime(foreignTarget),coilProfile.id,0).residuals.find(residual=>residual.thermal);
 check(foreignTargetResidual?.thermal.inletToOutlet===10&&!('inletToTarget'in foreignTargetResidual.thermal),'another node target was borrowed as the selected coil air-outlet target');
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

 // Reuse available observations only when every existing property is covered.
 const promotion=runtimeClone(fixtures,'fan'),nativeTemp=promotion.controlSignals.find(signal=>signal.graph==='parameterControl'&&signal.role==='sensor'&&signal.variableName==='System Node Temperature'),nativeFlow=promotion.controlSignals.find(signal=>signal.keyValue===nativeTemp?.keyValue&&signal.variableName==='System Node Mass Flow Rate'),nativeActivity=promotion.controlSignals.find(signal=>signal.role==='activity');
 check(nativeTemp&&nativeFlow&&nativeActivity,'native fan lacks complete cadence-promotion fixture');
 const asHourly=binding=>{const original=promotion.controlSeries.find(series=>series.sourceId===binding.seriesId),captured=structuredClone(original);delete captured.displayPoints;delete captured.displayColumn;delete captured.displayUnit;captured.sourceId='legacy-'+binding.seriesId;captured.reportingFrequency='Hourly';captured.column=captured.column.replace(/\([^()]*\)\s*$/,'(Hourly)');captured.points=[{x:1,label:'01-01 01:00',value:17}];return captured;};
 promotion.series=[asHourly(nativeTemp),asHourly(nativeFlow)];promotion.components=[{componentName:fanFixture.component.objectName,componentType:fanFixture.component.objectType,inletNodes:[nativeTemp.keyValue],series:[asHourly(nativeActivity)]}];
 const promoted=prepare(promotion),promotionBefore=JSON.stringify(promotion);
 check(promoted.reportingFrequency==='Timestep'&&promoted.frames.length===4&&promoted.interval===.25,'fully covered legacy properties did not reuse existing native Timestep series');
 check(snapshot(promoted,0).nodes.find(node=>node.name===nativeTemp.keyValue)?.metrics.find(metric=>metric.id==='temperature')?.value===20,'promoted snapshot reused Hourly row1 instead of actual Timestep source');
 const fallback=structuredClone(promotion);fallback.series.push(series('System Node Relative Humidity','%',[point(1,40,'01-01 01:00')],nativeTemp.keyValue));
 const fallbackModel=prepare(fallback);
 check(fallbackModel.reportingFrequency==='Hourly'&&fallbackModel.frames.length===1&&snapshot(fallbackModel,0).nodes[0].metrics.find(metric=>metric.id==='temperature').value===17,'one missing property did not retain whole-loop Hourly semantics');
 const {hvacInspectionCustomTrace:customTrace,hvacInspectionCustomFrequency:customFrequency}=await import('/src/js/hvac-inspection-data.js');
 const customTemperature=fallbackModel.entities.find(entity=>entity.name===nativeTemp.keyValue).properties.find(property=>property.kind==='temperature'),customFlow=fallbackModel.entities.find(entity=>entity.name===nativeTemp.keyValue).properties.find(property=>property.kind==='flow'),customRH=fallbackModel.entities.find(entity=>entity.name===nativeTemp.keyValue).properties.find(property=>property.kind==='relativeHumidity');
 const nativeTemperatureTrace=customTrace(fallbackModel,customTemperature),nativeFlowTrace=customTrace(fallbackModel,customFlow),hourlyHumidityTrace=customTrace(fallbackModel,customRH);
 check(customFrequency(fallbackModel,customTemperature)==='Timestep'&&nativeTemperatureTrace.reportingFrequency==='Timestep'&&nativeTemperatureTrace.points.length===4&&nativeTemperatureTrace.points.every((point,index)=>point.value===fallback.controlSeries.find(series=>series.sourceId===nativeTemp.seriesId).points[index].value),'Custom graph discarded native Timestep values because an unrelated Hourly property was unavailable at that cadence');
 check(trace(fallbackModel,customTemperature).points.length===1&&trace(fallbackModel,customTemperature).points[0].value===17&&customFrequency(fallbackModel,customRH)==='Hourly'&&hourlyHumidityTrace.points.length===1,'Custom cadence changed the existing node graph or fabricated a Timestep humidity observation');
 check(customTrace(fallbackModel,customTemperature)===nativeTemperatureTrace,'Custom frame/mode changes rebuilt complete native observations');
 const {renderHVACInspectionChart:renderCustom}=await import('/src/js/hvac-inspection-charts.js'),customMount=document.createElement('div');
 customMount.innerHTML=renderCustom({mode:'scatter',series:[nativeTemperatureTrace,nativeFlowTrace]});
 check(customMount.querySelectorAll('[data-hvac-chart-scatter] circle').length===4,'same native file/cadence/interval observations did not form complete Custom scatter pairs');
 customMount.innerHTML=renderCustom({mode:'scatter',series:[nativeTemperatureTrace,hourlyHumidityTrace]});
 check(customMount.querySelector('[data-hvac-chart-empty]')&&!customMount.querySelector('circle'),'Custom scatter paired Hourly and Timestep quantities merely because their end timestamp matched');
 customMount.innerHTML=renderCustom({mode:'histogram',series:[nativeTemperatureTrace,hourlyHumidityTrace]});
 check([...customMount.querySelectorAll('[data-hvac-chart-series]')].map(group=>Number(group.dataset.hvacChartObservations)).join(',')==='4,1','Custom histogram reduced native distributions to the shared loop frame count');
 const optionalPerformance=structuredClone(fallback),optionalPerformanceBinding=optionalPerformance.controlSignals.find(signal=>signal.id===nativeActivity.id);
 Object.assign(optionalPerformanceBinding,{graph:'equipment',role:'operating',required:false});
 const optionalPerformanceBefore=JSON.stringify(optionalPerformance),optionalPerformanceModel=prepare(optionalPerformance),optionalPerformanceProperty=optionalPerformanceModel.entities.find(entity=>entity.kind==='component').properties.find(property=>property.kind==='power');
 check(customTrace(optionalPerformanceModel,optionalPerformanceProperty).reportingFrequency==='Timestep'&&customTrace(optionalPerformanceModel,optionalPerformanceProperty).points.length===4&&optionalPerformanceModel.reportingFrequency==='Hourly','collected optional operating channel was unavailable to Custom or forced an incompatible whole-loop cadence');
 check(JSON.stringify(optionalPerformance)===optionalPerformanceBefore,'Custom optional operating projection modified raw source metadata/observations');
 const originalIDs=fallbackModel.entities.map(entity=>[entity.id,entity.properties.filter(property=>property.kind!=='relativeHumidity').map(property=>property.id)]);
 check(JSON.stringify(promoted.entities.map(entity=>[entity.id,entity.properties.map(property=>property.id)]))===JSON.stringify(originalIDs),'cadence promotion changed stable equipment/node/property IDs');
 for(const variant of ['duplicate','hourly','foreign']){
  const captured=structuredClone(promotion);
  if(variant==='duplicate')captured.controlSeries.push(structuredClone(captured.controlSeries.find(series=>series.sourceId===nativeTemp.seriesId)));
  if(variant==='hourly')captured.controlSeries.forEach(series=>series.reportingFrequency='Hourly');
  if(variant==='foreign')captured.controlSignals.forEach(signal=>signal.loopId='another-native-circuit');
  check(prepare(captured).reportingFrequency==='Hourly','cadence promotion borrowed '+variant+' diagnostic observations');
  const retained=prepare(captured),property=retained.entities.find(entity=>entity.name===nativeTemp.keyValue).properties.find(property=>property.kind==='temperature');
  check(customTrace(retained,property).reportingFrequency==='Hourly'&&customTrace(retained,property).points.length===1,'Custom graph borrowed '+variant+' source/cadence evidence rejected by the main inspection');
 }
 const completeHours=structuredClone(promotion);
 for(const series of [...completeHours.series,...completeHours.components.flatMap(component=>component.series)])series.points=[1,2,3].map((hour,index)=>({x:hour,label:'01-01 0'+hour+':00',value:17+index}));
 for(const series of completeHours.controlSeries){
  const source=series.points;delete series.displayPoints;delete series.displayColumn;delete series.displayUnit;
  series.points=Array.from({length:12},(_,index)=>{const minutes=(index+1)*15;return {x:index+1,label:'01-01 '+String(Math.floor(minutes/60)).padStart(2,'0')+':'+String(minutes%60).padStart(2,'0'),value:source[index%source.length].value};});
 }
 check(prepare(completeHours).reportingFrequency==='Timestep'&&prepare(completeHours).frames.length===12,'complete native Timestep counterparts did not preserve every known Hourly observation time');
 for(const missingHour of ['01-01 02:00','01-01 03:00']){
  const captured=structuredClone(completeHours),counterpart=captured.controlSeries.find(series=>series.sourceId===nativeTemp.seriesId);
  counterpart.points=counterpart.points.filter(point=>point.label!==missingHour);
  const retained=prepare(captured),temperature=retained.entities.find(entity=>entity.kind==='node'&&entity.name===nativeTemp.keyValue)?.properties.find(property=>property.kind==='temperature');
  check(retained.reportingFrequency==='Hourly'&&retained.frames.length===3&&trace(retained,temperature).points.map(point=>point.value).join(',')==='17,18,19','partial Timestep counterpart discarded existing middle/last Hourly observation: '+missingHour);
 }
 // These observations are copied from a native 24.2 two-day LargeOffice run.
 // Presentation rows are rebased (51), while SQL interval identities remain
 // original (2359). Requiring their equality rejected every PLR/COP pair.
 const nativeOperatingFixture=boiler=>{
  const rows=[51,52,196,584],labels=['07-21 00:10','07-21 00:20','07-21 08:30','07-22 05:10'];
  const sqlRows=[2359,2360,2504,2892],days=[1,1,1,2],name=boiler?'HEATSYS1 BOILER':'COOLSYS1 CHILLER 1';
  const objectName=boiler?'HeatSys1 Boiler':'CoolSys1 Chiller 1',objectType=boiler?'Boiler:HotWater':'Chiller:Electric:ReformulatedEIR',componentIndex=boiler?585:587,loopId=boiler?'plant:heatsys1':'plant:coolsys1';
  const names=boiler?['Boiler Part Load Ratio','Boiler Efficiency']:['Chiller Part Load Ratio','Chiller COP'];
  const values=boiler?[[0,0,0,3.734493533651071e-6],[0,0,0,.15600000000000003]]:[[0,0,.3953723261782503,.3801105855288277],[0,0,6.172102380243558,7.221135274795966]];
  const source={objectIndex:componentIndex,objectType,objectName},nativeIntervals=sqlRows.map((timeIndex,index)=>({timeIndex,environmentPeriodIndex:3,intervalMinutes:10,simulationDays:days[index],year:2013}));
  const native=names.map((variable,index)=>({sourceId:boiler?'sql-rdd-'+[3442,3444][index]:'sql-rdd-'+[3230,3315][index],file:'eplusout.sql',name:variable,keyValue:name,column:name+':'+variable+' [] (Zone Timestep)',reportingFrequency:'Zone Timestep',nativeIntervalsSourceId:'sql-rdd-2759',points:rows.map((x,offset)=>({x,label:labels[offset],value:values[index][offset]}))}));
  const hourly=names.map((variable,index)=>({...native[index],sourceId:'hourly-'+index,reportingFrequency:'Hourly',column:name+':'+variable+' [] (Hourly)',points:[{x:57,label:'07-21 01:00',value:0},{x:64,label:'07-21 02:00',value:0}]}));hourly.forEach(series=>delete series.nativeIntervalsSourceId);
  // A normal loop also contains Hourly node channels without control bindings.
  // This keeps whole-loop promotion separate from the Custom numeric-column path.
  const ordinary=series('System Node Temperature','C',[{x:57,label:'07-21 01:00',value:20},{x:64,label:'07-21 02:00',value:21}],'NATIVE LOOP INLET');
  return {name:boiler?'HeatSys1':'CoolSys1',loopType:'PlantLoop',topology:{id:loopId},series:[ordinary],components:[{componentName:name,componentType:objectType,series:hourly}],operatingProfiles:[{componentIndex,loopId,parameterControl:{nodes:[{id:'equipment',code:'equipment_response',source}]}}],controlSignals:native.map((series,index)=>({id:'native-operating-'+index,componentIndex,loopId,graph:'equipment',role:'operating',domain:'continuous',source,keyValue:name,variableName:series.name,reportingFrequency:'Timestep',seriesId:series.sourceId,required:false,status:'collected'})),controlSeries:[{sourceId:'sql-rdd-2759',file:'eplusout.sql',name:'Schedule Value',keyValue:'ALWAYS_ON',column:'ALWAYS_ON:Schedule Value [] (Zone Timestep)',reportingFrequency:'Zone Timestep',points:rows.map((x,index)=>({x,label:labels[index],value:1})),nativeIntervals},...native]};
 };
 const nativeOperatingTraces=captured=>{const model=prepare(captured),properties=model.entities.find(entity=>entity.kind==='component').properties;return properties.map(property=>customTrace(model,property));};
 const nativePairCount=traces=>{customMount.innerHTML=renderCustom({mode:'scatter',series:traces});return customMount.querySelectorAll('[data-hvac-chart-scatter] circle').length;};
 const compactOperating=captured=>{
  const packed=structuredClone(captured),envelope={schema:'semantic-idf.simulation-transfer/v1',result:{purposeResults:{hvacLoops:[packed]}},timelines:[],pointSets:[],pointBindings:[],columnSets:[],columnBindings:[]},timelines=new Map();
  const pack=(series,path)=>{
   const points=series.points,signature=JSON.stringify(points.map(point=>[point.x,point.label]));
   let timeline=timelines.get(signature);if(timeline===undefined){timeline=envelope.timelines.length;timelines.set(signature,timeline);envelope.timelines.push({x:points.map(point=>point.x),labels:points.map(point=>point.label)});}
   envelope.pointBindings.push({path:[...path,'points'],data:envelope.pointSets.length});envelope.pointSets.push({timeline,values:points.map(point=>point.value)});series.points=null;
   if(series.nativeIntervals){envelope.columnBindings.push({path:[...path,'nativeIntervals'],data:envelope.columnSets.length});envelope.columnSets.push({kind:'nativeIntervals',values:series.nativeIntervals.flatMap(interval=>[interval.timeIndex,interval.environmentPeriodIndex,interval.intervalMinutes,interval.simulationDays,interval.year||0])});series.nativeIntervals=null;}
  };
  packed.components.forEach((component,index)=>component.series.forEach((series,offset)=>pack(series,['purposeResults','hvacLoops',0,'components',index,'series',offset])));
  packed.controlSeries.forEach((series,index)=>pack(series,['purposeResults','hvacLoops',0,'controlSeries',index]));
  return decodeRuntime(envelope,{deferData:true}).purposeResults.hvacLoops[0];
 };
 for(const boiler of [false,true]){
  const captured=nativeOperatingFixture(boiler),before=JSON.stringify(captured),traces=nativeOperatingTraces(captured);
  check(captured.controlSeries[1].points[0].x===51&&captured.controlSeries[0].nativeIntervals[0].timeIndex===2359,'native regression lost the actual rebased-row/SQL-TimeIndex distinction');
  check(traces.every(trace=>trace.reportingFrequency==='Timestep'&&trace.points.length===4)&&nativePairCount(traces)===4,'native '+(boiler?'boiler PLR/efficiency':'chiller PLR/COP')+' observations failed despite identical source intervals');
  check(traces[0].points.every((point,index)=>point.key===traces[1].points[index].key)&&traces[0].points[0].key.includes('2359'),'native scatter identity used presentation row instead of original SQL TimeIndex');
  for(const [index,source]of captured.controlSeries.slice(1).entries())check(traces[index].points.every((point,offset)=>point.value===source.points[offset].value),'native zero or measured performance value changed during pairing');
  check(JSON.stringify(captured)===before,'native pairing mutated source observations or interval aliases');
  const compact=compactOperating(captured),compactTraces=nativeOperatingTraces(compact);
  check(nativePairCount(compactTraces)===4&&compactTraces.every((trace,index)=>trace.points.every((point,offset)=>point.key===traces[index].points[offset].key&&point.value===traces[index].points[offset].value)),'columnar transfer lost native rebased-row interval identities or values');
  check(compact.controlSeries.every(series=>typeof Object.getOwnPropertyDescriptor(series,'points').get==='function'),'native Custom pairing materialized canonical point objects instead of using shared numeric columns');
  const rebased=structuredClone(captured),right=rebased.controlSeries[2];right.nativeIntervals=structuredClone(rebased.controlSeries[0].nativeIntervals);delete right.nativeIntervalsSourceId;right.points.forEach(point=>point.x+=1000);
  check(nativePairCount(nativeOperatingTraces(rebased))===4,'different presentation rebasing prevented the same original SQL intervals from pairing');
  for(const mismatch of ['native-time','environment','duration','file','missing-metadata']){
   const wrong=structuredClone(captured),other=wrong.controlSeries[2];other.nativeIntervals=structuredClone(wrong.controlSeries[0].nativeIntervals);delete other.nativeIntervalsSourceId;
   if(mismatch==='native-time')other.nativeIntervals.forEach(interval=>interval.timeIndex+=10000);
   if(mismatch==='environment')other.nativeIntervals.forEach(interval=>interval.environmentPeriodIndex++);
   if(mismatch==='duration')other.nativeIntervals.forEach(interval=>interval.intervalMinutes=5);
   if(mismatch==='file')other.file='another-run.sql';
   if(mismatch==='missing-metadata')delete other.nativeIntervals;
   check(nativePairCount(nativeOperatingTraces(wrong))===0,'equal display times fabricated native pairs across '+mismatch);
  }
 }
 const nativePairEvidence=[],nativeCapture=await(await fetch('/native-equipment-fixture')).json();
 if(nativeCapture.hvacLoops?.length){
  for(const [loopName,name,left,right]of [['CoolSys1','COOLSYS1 CHILLER 1','Chiller Part Load Ratio','Chiller COP'],['CoolSys1','COOLSYS1 CHILLER 2','Chiller Part Load Ratio','Chiller COP'],['HeatSys1','HEATSYS1 BOILER','Boiler Part Load Ratio','Boiler Efficiency']]){
   const captured=nativeCapture.hvacLoops.find(loop=>loop.name===loopName),before=JSON.stringify(captured),model=prepare(captured),entity=model.entities.find(entity=>entity.name===name);
   check(entity,'native capture lacks '+name);
   const properties=[left,right].map(name=>entity.properties.find(property=>property.name===name));check(properties.every(Boolean),'native capture lacks operating properties for '+name);
   const traces=properties.map(property=>customTrace(model,property)),expected=captured.controlSeries.find(series=>series.keyValue===name&&series.name===left).points.length,count=nativePairCount(traces);
   check(expected===288&&count===expected&&traces.every(trace=>trace.points.length===expected),'complete native capture lost observed PLR/COP/efficiency pairs for '+name+': '+count+'/'+expected);
   check(JSON.stringify(captured)===before,'complete native capture was mutated by inspection');nativePairEvidence.push({equipment:name,pairs:count});
  }
 }
 const {matchHVACRuntimeFrame:matchRuntimeFrame}=await import('/src/js/hvac-inspection-data.js'),promotionRuntime=prepareRuntime(promotion);
 check(matchRuntimeFrame(promotionRuntime,fallbackModel,0)===3,'same SQL row number was used across Hourly/Timestep axes');
 const noCommon={...fallbackModel,frames:[{...fallbackModel.frames[0],label:'01-01 02:00'}]};
 check(matchRuntimeFrame(promotionRuntime,noCommon,0)===-1&&matchRuntimeFrame(promotionRuntime,fallbackModel,-1)===-1,'missing current timestep was replaced by a nearby/first observation');
 const repeatedRuntime={...promotionRuntime,frames:[...promotionRuntime.frames,{...promotionRuntime.frames[3],key:'another-environment:5',index:4}]};
 check(matchRuntimeFrame(repeatedRuntime,fallbackModel,0)===-1,'repeated runtime timestamp joined an arbitrary environment');
 check(matchRuntimeFrame(promotionRuntime,{...fallbackModel,frames:[...fallbackModel.frames,{...fallbackModel.frames[0],row:99}]},0)===-1,'repeated inspection timestamp joined an arbitrary environment');
 check(matchRuntimeFrame(promotionRuntime,{...fallbackModel,frames:[{...fallbackModel.frames[0],label:'01/01 01:00'}]},0)===3,'equivalent native timestamp formats failed normalized matching');
 check(JSON.stringify(promotion)===promotionBefore,'promotion/alignment mutated original native results');
 const local=runtimeClone(fixtures,'local'),localBefore=JSON.stringify(local),localInspection=prepare(local);
 check(local.loopType==='ZoneHVAC'&&!local.topology&&!localInspection.entities.length&&localInspection.reportingFrequency==='Timestep'&&localInspection.frames.length===4,'control-only native local equipment lost common axis or gained invented loop/entities');
 check(localInspection.frames.every((frame,index)=>matchRuntimeFrame(prepareRuntime(local),localInspection,index)===index),'local common frame diverged from its original native control axis');
 const foreignLocal=structuredClone(local);foreignLocal.controlSignals.forEach(signal=>signal.loopId='another-native-circuit');
 check(!prepare(foreignLocal).frames.length,'control-only local axis borrowed a foreign occurrence');
 check(JSON.stringify(local)===localBefore,'local common axis mutated native input/results');
 check(JSON.stringify(fixtures)===fixturesBefore,'runtime inspection mutated native captured fixtures');

 const {setLanguage,getLanguage}=await import('/src/js/i18n.js');setLanguage('ko');
 const korean=prepare(loop);check(korean!==model&&korean.language===getLanguage()&&korean.language==='ko','language change retained cached presentation');
 check(trace(korean,korean.entities[0].properties.find(p=>p.kind==='temperature')).points.map(p=>p.value).join(',')==='20,21,22','language change altered original observations');
 setLanguage('ja');const japanese=prepare(loop);check(japanese!==korean&&japanese.language==='ja','non-Korean locale reused Korean presentation');
 setLanguage('en');const english=prepare(loop);check(english!==japanese&&english.language==='en','locale cache depends on a translated term instead of locale identity');
 check(JSON.stringify(loop)===original,'inspection mutated original result');
 document.body.dataset.hvacDataStatus='passed';document.getElementById('result').textContent=nativePairEvidence.length?JSON.stringify({status:'passed',nativePairs:nativePairEvidence}):'passed';
}catch(error){document.body.dataset.hvacDataStatus='failed';document.getElementById('result').textContent=error.stack;}
</script>`
