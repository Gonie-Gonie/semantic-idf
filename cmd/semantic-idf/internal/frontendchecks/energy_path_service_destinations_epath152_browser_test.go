package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestEPATH152PureServiceDestinationsBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser service destination verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath152-services", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath152PureServicesHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath152-services").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH152 pure services browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath152-services="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH152 pure services failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH152 pure services failed:\n%s", output)
	}
}

const epath152PureServicesHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH152 pure services</title></head>
<body data-epath152-services="pending"><pre id="result">pending</pre><script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const clone=value=>JSON.parse(JSON.stringify(value));
const freeze=value=>{if(value&&typeof value==='object'&&!Object.isFrozen(value)){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
try {
  const {energyPathServiceDestinations:resolve}=await import('/src/js/energy-path-service-destinations.js');
  const {energyPathLabelPeriodRange:labelsRange,energyPathSeriesPeriodRange:seriesRange}=await import('/src/js/energy-path-navigation.js');
  const {energyPathOutputRequestKey:requestKey}=await import('/src/js/energy-path-output-requests.js');
  const all=result=>result.groups.flatMap(group=>group.candidates);
  const targets=(result,kind)=>all(result).filter(item=>!kind||item.kind===kind).map(item=>item.target.targetId);
  const node=(id,level,value,extra={})=>({id,level,value,period:'M1',unit:'kWh',scaleDomain:level==='load'?'thermal':'site',...extra});
  const load=node('load','load',100,{serviceKind:'cooling',relatedPathIds:['office.cooling.a','office.cooling.b','lab.cooling','office.heating'],sourceIds:['load.source']});
  const use=node('use','end_use',25,{serviceKind:'cooling',endUse:'cooling',sourceIds:['cooling.source']});
  const facility=node('facility','carrier',50,{carrier:'electricity',sourceIds:['facility.source','lighting.source','gas.facility']});
  const link={id:'conversion',relation:'load_to_end_use',fromId:load.id,toId:use.id,period:'M1',serviceKind:'cooling',fromValue:40,toValue:20,fromUnit:'kWh thermal',toUnit:'kWh site',relatedPathIds:['office.cooling.a'],sourceIds:['load.source','cooling.source']};
  const air={id:'air.ref',type:'AirLoopHVAC',name:'Office Air',objectIndex:30},plant={id:'plant.ref',type:'PlantLoop',name:'Office Plant',objectIndex:40};
  const path=(id,zoneName,serviceKind,extra={})=>({id,zoneName,serviceKind,delivery:{id:id+'.delivery',objectType:'Coil:Cooling:Water',objectName:id+' coil',objectIndex:20},...extra});
  const paths=[path('office.cooling.a','Office','cooling',{airLoop:air,plantLoop:plant}),path('office.cooling.b','Office','cooling'),path('lab.cooling','Lab','cooling'),path('office.heating','Office','heating',{plantLoop:plant})];
  const hvac={loops:[air,plant],serviceModel:{zoneServices:[{zoneName:'Office',paths:paths.filter(item=>item.zoneName==='Office')},{zoneName:'Lab',paths:paths.filter(item=>item.zoneName==='Lab')}],navigation:{entities:[
    {id:'loop:airloophvac:office air',kind:'loop',label:'Office Air',objectType:'AirLoopHVAC',objectName:'Office Air',objectIndex:30,loopType:'AirLoopHVAC',relatedPathIds:['office.cooling.a']},
    {id:'loop:plantloop:office plant',kind:'loop',label:'Office Plant',objectType:'PlantLoop',objectName:'Office Plant',objectIndex:40,loopType:'PlantLoop',relatedPathIds:['office.cooling.a','office.heating']},
  ]}}};
  const semanticNavigation={entities:[],occurrences:[]};
  for(const path of paths){const target={view:'hvac',targetKind:'service-path',targetId:path.id,label:'Actual '+path.id};semanticNavigation.entities.push({id:'semantic.'+path.id,kind:'hvac-path',viewTargets:[target]});semanticNavigation.occurrences.push({occurrenceId:'occ.'+path.id,entityId:'semantic.'+path.id,viewTargets:[target]});}
  for(const loop of hvac.serviceModel.navigation.entities)semanticNavigation.entities.push({id:'semantic.'+loop.id,kind:'hvac-loop',viewTargets:[{view:'hvac',targetKind:'hvac-loop',targetId:loop.id,label:loop.label}]});
  const heatFlow={sourceFile:'eplusout.sql',frameCount:4,labels:['01-01 01:00','01-31 24:00','02-01 01:00','02-28 24:00'],categories:[{id:'systemAir'},{id:'surfaceConvection'}],zones:[{name:'Office',values:[[10,20,30,40],[-1,-2,-3,-4]]},{name:'Lab',values:[[5,6,7,8],[1,2,3,4]]}]};
  const geometry={zones:[{id:'z1',name:'Office'},{id:'z2',name:'Lab'}],surfaces:['Office','Lab'].map(zoneName=>({id:'floor.'+zoneName,zoneName,surfaceType:'Floor',vertices:[{x:0,y:0,z:0},{x:1,y:0,z:0},{x:1,y:1,z:0}]}))};
  const meter=(id,name,extra={})=>({id,sourceType:'sql_meter',isMeter:true,name,keyValue:name,reportingFrequency:'Monthly',...extra});
  const sources=[{id:'load.source',sourceType:'sql_variable',zoneName:'Office',name:'Zone Air System Sensible Cooling Energy',keyValue:'Office',reportingFrequency:'Monthly'},meter('cooling.source','Cooling:Electricity',{objectIndex:8}),meter('facility.source','Electricity:Facility',{objectIndex:8}),meter('lighting.source','InteriorLights:Electricity'),meter('gas.facility','NaturalGas:Facility')];
  const outputObjects=[{objectType:'Output:Meter',keyValue:'Electricity:Cooling',reportingFrequency:'Hourly',objectIndex:8},{objectType:'Output:Meter',keyValue:'Electricity:Cooling',reportingFrequency:'Monthly',objectIndex:9},{objectType:'Output:Meter',keyValue:'Electricity:Facility',reportingFrequency:'Hourly',objectIndex:8},{objectType:'Output:Meter',keyValue:'Electricity:Facility',reportingFrequency:'Monthly',objectIndex:10},{objectType:'Output:Meter',keyValue:'InteriorLights:Electricity',reportingFrequency:'Monthly'},{objectType:'Output:Meter',keyValue:'NaturalGas:Facility',reportingFrequency:'Monthly'}];
  const options={nodes:[load,use,facility],links:[link],sources,scope:{kind:'building'},period:'M1',hvac,semanticNavigation,heatFlow,geometry,outputObjects,zoneRows:[{key:'Office',value:60,unit:'kWh thermal'},{key:'Lab',value:40,unit:'kWh thermal'}]};
  const snapshot=JSON.stringify({load,options});freeze(load);freeze(options);
  let result=resolve(load,options);
  check(targets(result,'hvac').includes('office.cooling.a')&&targets(result,'hvac').includes('office.cooling.b')&&targets(result,'hvac').includes('lab.cooling')&&!targets(result).includes('office.heating'),'load service paths were not exact service-filtered explicit choices');
  check(targets(result,'hvac').includes('loop:airloophvac:office air')&&targets(result,'hvac').includes('loop:plantloop:office plant'),'explicit connected actual AirLoop/PlantLoop targets missing');
  const officeLedger=all(result).find(item=>item.kind==='heat_flow'&&item.zoneName==='Office');
  check(officeLedger?.range.start===0&&officeLedger?.range.end===1&&officeLedger.evidenceKind==='zone_context'&&officeLedger.target.targetId==='Office','Ledger used frame index as month or claimed load scalar provenance');
  check(result.groups.find(group=>group.kind==='heat_flow'&&group.zoneName==='Office')?.value===60,'top Zone display contribution was lost');
  check(!Object.hasOwn(result.unavailableReasons,'output'),'load got inapplicable equipment Output actions');
  result=resolve({...load,zoneName:'Office'},{...options,scope:{kind:'zone',zoneName:'Office'}});
  check(!targets(result).includes('lab.cooling')&&!targets(result,'heat_flow').includes('Lab'),'Zone navigation escaped exact selected Zone');
  result=resolve(use,options);
  const coolingOutput=all(result).find(item=>item.kind==='output');
  check(coolingOutput?.sourceId==='cooling.source'&&coolingOutput.requestIndex===1&&coolingOutput.requestKey===requestKey(outputObjects[1],1),'stale Hourly object index overrode exact Monthly meter request');
  check(coolingOutput?.requestFields.reportingFrequency==='Monthly'&&coolingOutput.requestFields.keyValue==='Electricity:Cooling'&&coolingOutput.requestFields.objectType==='Output:Meter','exact Output chooser omitted differentiating request display fields');
  check(targets(result,'hvac').includes('office.cooling.a'),'valid selected-period adjacent conversion lost explicit HVAC path');
  check(!all(resolve(use,{...options,service:'heating'})).length,'selected service contradicted node service but navigation remained available');
  check(!all(resolve({...use,serviceKind:'heating',relatedPathIds:['office.heating']},{...options,links:[]})).length,'cooling equipment taxonomy accepted heating own metadata');
  check(!all(resolve({...use,serviceKind:''},{...options,service:'heating'})).length,'explicit cooling equipment taxonomy was overridden by selected heating service');
  for(const invalid of [{period:'annual'},{relation:'source_correspondence'},{fromUnit:'kWh site'},{toValue:NaN},{zoneName:'Lab'}]){
    check(!targets(resolve(use,{...options,links:[{...link,...invalid}]}),'hvac').length,'invalid/support/wrong-period adjacent link authorized HVAC path '+JSON.stringify(invalid));
  }
  result=resolve(facility,options);
  check(all(result).length===1&&all(result)[0].sourceId==='facility.source'&&all(result)[0].requestIndex===3,'Carrier used end-use/gas meter instead of matching Facility request');
  const derived={id:'derived.source',sourceType:'derived_allocation',inputSourceIds:['cooling.source','derived.source']};
  result=resolve({...use,sourceIds:['derived.source']},{...options,links:[],sources:[...sources,derived]});
  check(all(result).filter(item=>item.kind==='output').length===1&&all(result).find(item=>item.kind==='output').evidenceKind==='derived_input','derived source fabricated direct request or lost explicit input/cycle handling');
  const firstDerived={...derived,id:'aaa.derived',inputSourceIds:['cooling.source']};
  for(const roots of [[firstDerived.id,'cooling.source'],['cooling.source',firstDerived.id]]){
    const direct=all(resolve({...use,sourceIds:roots},{...options,links:[],sources:[firstDerived,...sources]})).find(item=>item.kind==='output');
    check(direct?.evidenceKind==='exact_source','direct source became derived because of lexical traversal order');
  }
  const tabular={id:'tabular.source',sourceType:'sql_tabular',name:'Electricity:Facility',isMeter:true,reportingFrequency:'Monthly'};
  check(!all(resolve({...facility,sourceIds:['tabular.source']},{...options,sources:[tabular]})).length,'tabular report fabricated a Facility Output request');
  const ambiguous={...sources[1]};delete ambiguous.objectIndex;
  result=resolve({...use,sourceIds:[ambiguous.id]},{...options,links:[],sources:[ambiguous],outputObjects:[outputObjects[1],{...outputObjects[1],objectIndex:12}]});
  check(result.unavailableReasons.output==='output_ambiguous'&&!all(result).length,'ambiguous duplicate request selected first object');
  for(const mutate of [payload=>payload.labels=['01-01 01:00','02-01 01:00','01-31 24:00','02-28 24:00'],payload=>payload.labels=['2024-01-01','2025-01-01','2025-02-01','2025-02-02'],payload=>payload.labels[1]=payload.labels[0],payload=>payload.labels=['frame1','frame2','frame3','frame4'],payload=>payload.zones[0].values=[[1,2],[3,4],[5,6],[7,8]],payload=>payload.zones[0].values[0][1]=null]){
    const invalid=clone(heatFlow);mutate(invalid);check(!all(resolve({...load,zoneName:'Office'},{...options,heatFlow:invalid,scope:{kind:'zone',zoneName:'Office'}})).some(item=>item.kind==='heat_flow'),'ambiguous/malformed/non-calendar/category-major Ledger data produced enabled range');
  }
  const csv=clone(heatFlow);csv.labels=csv.labels.map(label=>label.replace('-','/'));
  check(all(resolve(load,{...options,heatFlow:csv})).some(item=>item.kind==='heat_flow'&&item.range.end===1),'actual CSV calendar labels were rejected');
  check(!all(resolve(load,{...options,geometry:{...geometry,surfaces:[]}})).some(item=>item.kind==='heat_flow'),'missing floor geometry offered invisible Ledger target');
  check(labelsRange(heatFlow.labels,'M1').end===1&&labelsRange(heatFlow.labels,'annual').end===-1&&labelsRange(heatFlow.labels,'M3')===null,'date-only range changed calendar semantics');
  check(seriesRange({points:heatFlow.labels.map(label=>({label,value:1}))},'M1').end===1&&seriesRange({points:[{label:'01-01 01:00',value:null}]},'M1')===null,'shared date extraction lost strict reported Series values');
  check(seriesRange({points:[{value:1}]},'annual').end===-1,'annual Series accidentally required absent calendar labels');
  const auxiliaryNode=node('auxiliary','end_use',3,{endUse:'fans_pumps',relatedPathIds:paths.map(path=>path.id)});
  result=resolve(auxiliaryNode,{...options,links:[],service:'cooling'});
  check(targets(result,'hvac').includes('office.cooling.a')&&!targets(result).includes('office.heating'),'auxiliary selected service did not constrain explicit broad paths');
  const groupedAux={...auxiliaryNode,relatedPathIds:[],groupedMembers:[{...auxiliaryNode,id:'fans.cool',endUse:'fans',serviceKind:'cooling'}]};
  check(!targets(resolve(groupedAux,{...options,links:[],service:'all'})).includes('office.heating'),'all-service projection discarded original member service evidence');
  const auxiliaryOptions=clone(options);auxiliaryOptions.links=[];
  const ventilationPaths=[path('office.ventilation','Office','ventilation',{airLoop:air}),path('office.exhaust','Office','exhaust',{airLoop:air})];
  auxiliaryOptions.hvac.serviceModel.zoneServices[0].paths.push(...ventilationPaths);
  for(const path of ventilationPaths)auxiliaryOptions.semanticNavigation.entities.push({id:'semantic.'+path.id,kind:'hvac-path',viewTargets:[{view:'hvac',targetKind:'service-path',targetId:path.id}]});
  const ventAux={...auxiliaryNode,relatedPathIds:ventilationPaths.map(path=>path.id)};
  check(targets(resolve(ventAux,auxiliaryOptions),'hvac').includes('office.ventilation')&&targets(resolve(ventAux,auxiliaryOptions),'hvac').includes('office.exhaust'),'explicit auxiliary ventilation/exhaust paths were excluded by cooling/heating physics');
  check(!targets(resolve({...load,relatedPathIds:ventAux.relatedPathIds},{...auxiliaryOptions,links:[]}),'hvac').length,'load service navigation incorrectly reused auxiliary ventilation/exhaust exception');
  const bridgeNode={...use,sourceIds:['related.loop']},bridgeSource={id:'related.loop',relatedEntityIds:['semantic.loop:airloophvac:office air']};
  check(targets(resolve(bridgeNode,{...options,links:[],sources:[bridgeSource]}),'hvac').includes('office.cooling.a'),'canonical semantic loop entity did not bridge its actual report navigation identity');
  const wrongBridge=clone(options);wrongBridge.links=[];wrongBridge.sources=[bridgeSource];
  wrongBridge.semanticNavigation.entities.find(entity=>entity.id===bridgeSource.relatedEntityIds[0]).viewTargets[0].targetKind='hvac-component';
  check(!targets(resolve(bridgeNode,wrongBridge),'hvac').length,'wrong semantic target kind authorized actual report loop paths');
  check(targets(resolve({...use,sourceIds:[],relatedEntityIds:[air.id]},{...options,links:[]}),'hvac').includes('office.cooling.a'),'explicit actual path reference ID was lost');
  const aliases=clone(options), pathEntity=aliases.semanticNavigation.entities.find(entity=>entity.id==='semantic.office.cooling.a');
  pathEntity.viewTargets.push({...pathEntity.viewTargets[0],targetKind:'hvac-path'});
  aliases.semanticNavigation.entities.push({id:'zone.office.context',kind:'zone',viewTargets:[{...pathEntity.viewTargets[0]}]});
  const aliased=all(resolve(load,aliases)).filter(item=>item.kind==='hvac'&&item.target.targetId==='office.cooling.a');
  check(aliased.length===1&&aliased[0].entityId===pathEntity.id&&aliased[0].target.targetKind==='service-path','one physical path appeared as duplicate Zone/entity/route choices');
  check(!targets(resolve({...use,sourceIds:['zone.context.source']},{...aliases,links:[],sources:[{id:'zone.context.source',zoneName:'Office',relatedEntityIds:['zone.office.context']}]}),'hvac').length,'Zone-only source context claimed explicit service path provenance');
  const contextFallback=clone(aliases);contextFallback.semanticNavigation.entities=contextFallback.semanticNavigation.entities.filter(entity=>entity.id!==pathEntity.id);contextFallback.semanticNavigation.occurrences=[];
  check(all(resolve(load,contextFallback)).some(item=>item.target.targetId==='office.cooling.a'&&item.entityId==='zone.office.context'),'independently evidenced path lost its actual Zone navigation alias');
  const conflicting=clone(aliases);conflicting.semanticNavigation.entities.push({...clone(pathEntity),id:'another.path.owner'});
  check(!targets(resolve(load,conflicting),'hvac').includes('office.cooling.a'),'equally specific conflicting physical owners selected a first entity');
  const fallback=clone(options);fallback.semanticNavigation.entities.find(entity=>entity.id===pathEntity.id).viewTargets[0].targetKind='hvac-path';fallback.semanticNavigation.occurrences=[];
  check(all(resolve(load,fallback)).some(item=>item.target.targetId==='office.cooling.a'&&item.target.targetKind==='hvac-path'),'actual hvac-path alias fallback disappeared when service-path route was absent');
  const occurrenceOptions=clone(options);occurrenceOptions.semanticNavigation.entities.find(entity=>entity.id===pathEntity.id).viewTargets=[];
  occurrenceOptions.semanticNavigation.occurrences=occurrenceOptions.semanticNavigation.occurrences.filter(item=>item.entityId!==pathEntity.id);
  for(const fieldName of ['Inlet Node','Outlet Node'])occurrenceOptions.semanticNavigation.occurrences.push({occurrenceId:'field.'+fieldName,entityId:pathEntity.id,sourceAnchor:{objectName:'Actual Coil',fieldName,objectIndex:20},viewTargets:[{...pathEntity.viewTargets[0]}]});
  const occurrenceChoices=all(resolve(load,occurrenceOptions)).filter(item=>item.target.targetId==='office.cooling.a');
  check(occurrenceChoices.length===2&&new Set(occurrenceChoices.map(item=>item.contextLabel)).size===2&&occurrenceChoices.every(item=>item.contextLabel.includes('Actual Coil')),'occurrence-only physical choices lost distinguishing friendly context');
  occurrenceOptions.semanticNavigation.occurrences.forEach(item=>delete item.sourceAnchor);
  check(!targets(resolve(load,occurrenceOptions),'hvac').includes('office.cooling.a'),'indistinguishable occurrence-only choices remained enabled');
  const ranked=resolve(load,{...options,zoneRows:[{key:'Office',value:10,unit:'kWh thermal'},{key:'Lab',value:90,unit:'kWh thermal'}]});
  check(ranked.groups.filter(group=>group.kind==='heat_flow')[0]?.zoneName==='Lab','top-Zone chooser was sorted by opaque identity instead of current contribution');
  const stable=all(resolve(load,options)).map(item=>item.id).sort();
  const reordered=clone(options);reordered.semanticNavigation.entities.reverse();reordered.sources.reverse();reordered.hvac.serviceModel.zoneServices.reverse();reordered.zoneRows=[];
  check(JSON.stringify(stable)===JSON.stringify(all(resolve({...load,value:999},reordered)).map(item=>item.id).sort()),'candidate identity depended on values/order/Zone ranking');
  check(JSON.stringify({load,options})===snapshot,'service resolver mutated canonical graph/model/source data');
  if(failures.length)throw Error(failures.join('\n'));
  document.body.dataset.epath152Services='passed';document.querySelector('#result').textContent='PASS';
}catch(error){document.body.dataset.epath152Services='failed';document.querySelector('#result').textContent=error.stack||String(error);}
</script></body></html>`
