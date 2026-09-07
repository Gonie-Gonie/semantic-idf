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

func TestEPATH150PureInspectorModelBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser period-safe inspector verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath150-inspector", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath150PureInspectorHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath150-inspector").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH150 pure inspector browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath150-inspector="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH150 pure inspector failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH150 pure inspector failed:\n%s", output)
	}
}

const epath150PureInspectorHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH150 pure inspector</title></head>
<body data-epath150-inspector="pending"><pre id="result">pending</pre><script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const clone=value=>JSON.parse(JSON.stringify(value));
const freeze=value=>{if(value&&typeof value==='object'&&!Object.isFrozen(value)){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
const value=(model,key)=>model.valueRows.find(row=>row.key===key)?.value;
const breakdown=(model,group,key)=>model.breakdown[group].find(row=>row.key===key);
try {
  const {energyPathInspectorModel:model}=await import('/src/js/energy-path-inspector.js');
  const thermal=(id,level,value,extra={})=>({id,level,value,unit:'kWh',scaleDomain:'thermal',period:'M1',...extra});
  const site=(id,level,value,extra={})=>({id,level,value,unit:'kWh',scaleDomain:'site',period:'M1',...extra});
  const driver=thermal('driver.wall','driver',5,{driverCategory:'surface.exterior_walls',serviceKind:'heating',rawValue:-12,effectiveValue:-24,allocatedValue:5,multiplier:2,allocationApplied:true,basis:'heat_balance_share',allocationExplanation:'PRIVATE_SOURCE_ID / PRIVATE_RULE_ID',sourceIds:['PRIVATE_SOURCE_ID'],offsetEffects:[{effectKind:'heating_reduction',rawValue:1,effectiveValue:2,sourceIds:['offset.source']}],simultaneousLoad:{available:true,numerator:1,denominator:2,ratio:.5,sourceIds:['simultaneous.source']}});
  const load=thermal('load.cooling','load',100,{serviceKind:'cooling',loadBreakdown:[{component:'sensible',value:80,unit:'kWh',sourceIds:['sensible.source']},{component:'latent',value:20,unit:'kWh',sourceIds:['latent.source']}],sourceIds:['load.source']});
  const use=site('use.cooling','end_use',25,{endUse:'cooling',serviceKind:'cooling',basis:'service_path_allocation',sourceIds:['use.source']});
  const electricity=site('carrier.electricity','carrier',30,{carrier:'electricity',basis:'reported_meter',meterHierarchyLevel:'facility_total',effectiveValue:null,allocatedValue:null,sourceIds:['facility.source']});
  const gas=site('carrier.gas','carrier',12,{carrier:'natural_gas',basis:'reported_meter'});
  const conversion={id:'conversion',fromId:load.id,toId:use.id,relation:'load_to_end_use',serviceKind:'cooling',period:'M1',fromValue:40,toValue:20,fromUnit:'kWh thermal',toUnit:'kWh site',basis:'derived_ratio',ruleId:'PRIVATE_RULE_ID',sourceIds:['load.source','use.source']};
  const split={id:'split',fromId:use.id,toId:electricity.id,relation:'end_use_to_carrier',period:'M1',fromValue:25,toValue:25,fromUnit:'kWh site',toUnit:'kWh',basis:'service_path_allocation',sourceIds:['use.source']};
  const sources=[{id:'PRIVATE_SOURCE_ID',rawValue:987654,effectiveValue:1975308,normalizedUnit:'kWh',reportingFrequency:'Monthly',scopeDetails:[{scope:{kind:'building'},rawValue:987654,effectiveValue:1975308}],multiplierApplication:'requires_zone_multiplier',relatedEntityIds:['model.wall']},
    {id:'prediction.source',driverCategory:'load.cooling',driverComponent:'load.predicted.sensible',driverRole:'context',inspectorSection:'context',effectiveValue:900,normalizedUnit:'kWh',reportingFrequency:'Monthly'},
    {id:'use.source',inputSourceIds:['meter.input'],effectiveValue:1234567},{id:'meter.input',inputSourceIds:['use.source']},
  ];
  const zone=(zoneName,driverValue,loadValue)=>({scope:{kind:'zone',zoneName},nodes:[{...driver,id:'annual.wall.'+zoneName,zoneName,period:'annual',value:10000}],periods:[{id:'M1',nodes:[
    {...driver,id:'zone.wall.'+zoneName,zoneName,value:driverValue},{...driver,id:'zone.wrong.category.'+zoneName,driverCategory:'surface.roofs',zoneName,value:9000},
    {...driver,id:'zone.wrong.service.'+zoneName,zoneName,serviceKind:'cooling',value:8000},{...load,id:'zone.load.'+zoneName,zoneName,value:loadValue},
  ]}]});
  const zones=[zone('Office',3,20),zone('Lab',2,30),{scope:{kind:'zone',zoneName:'Annual only'},nodes:[{...driver,zoneName:'Annual only',period:'annual',value:7777}]}];
  const options={period:'M1',scope:{kind:'building'},nodes:[driver,load,use,electricity,gas],links:[conversion,split],sources,zoneResults:zones,ratioQuality:{status:'complete'}};
  const snapshot=JSON.stringify({driver,load,use,electricity,options});freeze(driver);freeze(load);freeze(use);freeze(electricity);freeze(options);
  let result=model(driver,options);
  check(value(result,'total')===5&&value(result,'raw')===-12&&value(result,'effective')===-24&&value(result,'allocated')===5&&value(result,'multiplier')===2,'driver lost independent signed raw/effective/allocated values');
  check(result.basis.allocationLabelKind==='allocated'&&!result.basis.explanation.includes('PRIVATE_'),'friendly basis leaked source/rule identifiers');
  check(['PRIVATE_SOURCE_ID','offset.source','simultaneous.source'].every(id=>result.sourceIds.includes(id))&&result.relatedEntityIds.includes('model.wall'),'hidden driver component/context/entity provenance omitted');
  check(result.breakdown.zoneRows.length===2&&breakdown(result,'zoneRows','Office').value===3&&breakdown(result,'zoneRows','Lab').value===2,'top Zones mixed wrong category/service or annual-only values');
  check(result.breakdown.componentRows.every(row=>row.value===null),'combined driver fabricated sensible/latent split');
  result=model({...driver,rawValue:0,effectiveValue:0,allocatedValue:0,value:0},options);
  check(['total','raw','effective','allocated'].every(key=>value(result,key)===0),'explicit allocated zero became missing/nonzero');
  for(const missing of [undefined,null,false,'',NaN,Infinity]){
    result=model({...driver,rawValue:missing,effectiveValue:missing,allocatedValue:missing,multiplier:missing},options);
    check(['raw','effective','allocated','multiplier'].every(key=>value(result,key)===null),'unknown optional number became0/1 or annual source fallback: '+String(missing));
  }
  const missingMultiplier={...driver,rawValue:1,effectiveValue:10};delete missingMultiplier.multiplier;
  check(value(model(missingMultiplier,options),'multiplier')===10,'missing multiplier was not derived from exact same-period raw/effective values');
  check(value(model({...missingMultiplier,rawValue:-1,effectiveValue:-10},options),'multiplier')===10,'same-sign negative raw/effective pair lost its positive factor');
  for(const invalid of [undefined,null,false,'',NaN,Infinity,0,-1]){
    check(value(model({...missingMultiplier,multiplier:invalid},options),'multiplier')===null,'explicit invalid multiplier was replaced by derived factor: '+String(invalid));
  }
  for(const contradictory of [{period:'annual'},{rawValue:0},{effectiveValue:0},{rawValue:null},{effectiveValue:null},{rawValue:-1},{unit:'m3'},{rawValue:1e-308,effectiveValue:1e308}]){
    check(value(model({...missingMultiplier,...contradictory},options),'multiplier')===null,'invalid/nonlocal accounting produced a multiplier: '+JSON.stringify(contradictory));
  }
  const uniformMultipliers={...driver,multiplier:999,groupedMembers:[missingMultiplier,{...missingMultiplier,id:'same.factor',rawValue:2,effectiveValue:20}]};
  check(value(model(uniformMultipliers,options),'multiplier')===10,'uniform original multipliers used stale outer aggregate factor');
  const mixedMultipliers={...uniformMultipliers,multiplier:2.5,groupedMembers:[{...missingMultiplier,rawValue:1,effectiveValue:2},{...missingMultiplier,id:'different.factor',rawValue:1,effectiveValue:3}]};
  check(value(model(mixedMultipliers,options),'multiplier')===null,'mixed original factors displayed weighted outer ratio as uniform multiplier');
  check(value(model({...uniformMultipliers,groupedMembers:[missingMultiplier,{...missingMultiplier,id:'invalid.factor',multiplier:null}]},options),'multiplier')===null,'grouped explicit unknown multiplier borrowed raw/effective fallback');
  check(value(model({...uniformMultipliers,groupedMembers:[missingMultiplier,{...missingMultiplier,id:'annual.factor',period:'annual'}]},options),'multiplier')===null,'grouped multiplier borrowed annual original factor');
  const member={...driver,id:'original',rawValue:null,effectiveValue:null,allocatedValue:null};
  result=model({...driver,rawValue:0,effectiveValue:0,allocatedValue:0,groupedMembers:[member]},options);
  check(['raw','effective','allocated'].every(key=>value(result,key)===null),'singleton original missing values were replaced by synthesized outer0');
  const grouped={...driver,rawValue:999,effectiveValue:999,allocatedValue:999,groupedMembers:[{...driver,id:'sensible',thermalComponent:'sensible',value:3,rawValue:-10,effectiveValue:-20,allocatedValue:3},{...driver,id:'latent',thermalComponent:'latent',value:2,rawValue:-2,effectiveValue:-4,allocatedValue:2}]};
  result=model(grouped,options);
  check(value(result,'raw')===-12&&value(result,'effective')===-24&&value(result,'allocated')===5&&breakdown(result,'componentRows','sensible').value===3&&breakdown(result,'componentRows','latent').value===2,'grouped exact accounting/components used outer stale fields');
  const opposed={...grouped,rawValue:8,effectiveValue:16,signedValue:2,sign:'positive',groupedMembers:[{...driver,id:'gain',rawValue:5,effectiveValue:10,signedValue:10,sign:'positive'},{...driver,id:'loss',rawValue:3,effectiveValue:6,signedValue:-6,sign:'negative'}]};
  result=model(opposed,options);
  check(value(result,'raw')===2&&value(result,'effective')===4,'opposing original signed pressures used overall sign times gross magnitudes');
  const sparseDirection={...driver,rawValue:12,effectiveValue:24};delete sparseDirection.sign;delete sparseDirection.signedValue;
  result=model(sparseDirection,options);
  check(value(result,'raw')===12&&result.valueRows.find(row=>row.key==='raw').direction==='magnitude','heating service alone fabricated negative raw direction');
  result=model({...opposed,groupedMembers:[{...sparseDirection,rawValue:5,effectiveValue:10},{...driver,rawValue:-3,effectiveValue:-6}]},options);
  check(value(result,'raw')===null&&value(result,'effective')===null,'mixed known/unknown pressure direction fabricated signed net2');
  result=model({...opposed,groupedMembers:[{...sparseDirection,rawValue:5,effectiveValue:10},{...sparseDirection,rawValue:0,effectiveValue:0}]},options);
  check(value(result,'raw')===5&&result.valueRows.find(row=>row.key==='raw').direction==='magnitude','explicit zero falsely forced unknown sign for magnitude-only group');
  for(const conflicting of [{period:'annual'},{scaleDomain:'site'},{unit:'m3'}]){
    const altered={...grouped,groupedMembers:[grouped.groupedMembers[0],{...grouped.groupedMembers[1],...conflicting}]};
    result=model(altered,options);
    check(value(result,'raw')===null&&breakdown(result,'componentRows','latent').value===null,'grouped original wrong period/domain/unit entered selected values');
  }
  const noPeriod={...driver};delete noPeriod.period;
  check(value(model(noPeriod,options),'total')===null,'unqualified missing monthly node period borrowed annual value');
  check(value(model(noPeriod,{...options,periodContext:'M1'}),'total')===5,'known monthly wrapper failed to qualify period-omitted node');
  check(value(model({...noPeriod,period:'annual'},{...options,periodContext:'M1'}),'total')===null,'trusted wrapper overrode explicit annual node period');
  result=model(load,options);
  check(value(result,'total')===100&&value(result,'raw')===null&&breakdown(result,'componentRows','sensible').value===80&&breakdown(result,'componentRows','latent').value===20,'load total or explicit period components incorrect');
  check(breakdown(result,'contextRows','load.predicted.sensible').value===null&&breakdown(result,'contextRows','load.predicted.sensible').scope==='run','monthly reportingFrequency promoted annual predicted source scalar into month');
  check(result.sourceIds.includes('prediction.source')&&result.sourceIds.includes('sensible.source')&&result.sourceIds.includes('latent.source'),'load context/component source evidence omitted');
  result=model({...load,period:'annual'},{...options,period:'annual'});
  check(breakdown(result,'contextRows','load.predicted.sensible').value===900,'annual run-level predicted context lost explicit reported value');
  result=model({...load,loadBreakdown:[{component:'sensible',value:0,unit:'kWh'}]},options);
  check(breakdown(result,'componentRows','sensible').value===0&&breakdown(result,'componentRows','latent').value===null,'missing latent became zero or sensible zero disappeared');
  const duplicateNodeZones=clone(zones);duplicateNodeZones[0].periods[0].nodes.push({...duplicateNodeZones[0].periods[0].nodes[0],value:99});
  check(breakdown(model(driver,{...options,zoneResults:duplicateNodeZones}),'zoneRows','Office').value===null,'conflicting Zone node duplicate used first/last value');
  const duplicatePeriodZones=clone(zones);duplicatePeriodZones[0].periods.push({...duplicatePeriodZones[0].periods[0],nodes:[{...driver,zoneName:'Office',value:99}]});
  check(breakdown(model(driver,{...options,zoneResults:duplicatePeriodZones}),'zoneRows','Office').value===null,'conflicting Zone month wrappers used first match');
  check(breakdown(model(driver,{...options,zoneResults:[...zones,zone('Office',99,20)]}),'zoneRows','Office').value===null,'conflicting Zone result variants used first match');
  result=model(use,options);
  check(breakdown(result,'carrierRows','electricity').value===25&&breakdown(result,'allocationRows','allocated').value===25&&breakdown(result,'allocationRows','direct').value===0,'end-use own carrier/allocated split incorrect');
  check(result.sourceIds.includes('meter.input'),'derived source input provenance was not retained or recursion failed');
  const ratio=breakdown(result,'ratioRows','cooling');
  check(ratio.value===2&&ratio.fromValue===40&&ratio.toValue===20&&ratio.partial&&ratio.linkIds[0]==='conversion','ratio replaced actual partial paired values with node totals');
  check(breakdown(model(use,{...options,ratioQuality:{status:'not_requested'}}),'ratioRows','cooling').value===null,'explicit unavailable ratio quality contradicted displayed quotient');
  const invalidConversion={...conversion,fromUnit:'kWh site'};
  check(model(use,{...options,links:[invalidConversion,split]}).breakdown.ratioRows.length===0,'wrong-domain conversion produced a ratio');
  check(model(use,{...options,links:[conversion,{...conversion,toValue:0},split]}).breakdown.ratioRows.length===0,'conflicting duplicate link identity picked one ratio');
  const memberLink=(id,toValue,basis)=>({...split,id,fromValue:toValue,toValue,basis});
  const mixedLink={...split,basis:'grouped_presentation',groupedMembers:[memberLink('direct',10,'direct_zone_energy'),memberLink('allocated',15,'service_path_allocation')]};
  result=model(use,{...options,links:[mixedLink]});
  check(breakdown(result,'allocationRows','direct').value===10&&breakdown(result,'allocationRows','allocated').value===15&&breakdown(result,'allocationRows','unknown').value===0,'mixed original link allocation partition failed');
  result=model(use,{...options,links:[{...mixedLink,groupedMembers:[memberLink('direct',10,'direct_zone_energy'),memberLink('allocated',999,'service_path_allocation')]}]});
  check(breakdown(result,'allocationRows','unknown').value===25&&breakdown(result,'allocationRows','direct').value===0&&breakdown(result,'allocationRows','allocated').value===0,'nonclosing leaf amounts invented known allocation split');
  result=model(use,{...options,links:[{...split,basis:'reported_meter'}],nodes:options.nodes.map(node=>node.id===use.id?{...node,allocationApplied:true}:node)});
  check(breakdown(result,'allocationRows','direct').value===25&&breakdown(result,'allocationRows','allocated').value===0,'reported branch borrowed endpoint allocation flag');
  const carrierOptions={...options,carrierQuality:{carrier:'electricity',period:'M1',expectedValue:30,explainedValue:25,residualValue:5,unit:'kWh',sourceIds:['reconciliation.source']},supplyActivities:[{kind:'purchased',carrier:'electricity',value:7,unit:'kWh',sourceIds:['purchased.source']},{kind:'produced',carrier:'electricity',period:'M1',value:6,unit:'kWh'},{kind:'storage',carrier:'electricity',period:'annual',value:98765,unit:'kWh'},{kind:'purchased',carrier:'natural_gas',value:555,unit:'kWh'}]};
  result=model(electricity,carrierOptions);
  check(value(result,'total')===30&&value(result,'effective')===null&&value(result,'allocated')===null,'carrier optional unknowns became reported0');
  check(breakdown(result,'endUseRows','cooling').value===25&&breakdown(result,'contextRows','facility_total').value===30&&breakdown(result,'contextRows','residual').value===5,'carrier actual enduses/facility/residual values incorrect');
  check(breakdown(result,'contextRows','purchased').value===7&&breakdown(result,'contextRows','produced').value===6&&!breakdown(result,'contextRows','storage'),'supply context mixed carrier/annual values into month');
  check(result.sourceIds.includes('reconciliation.source')&&result.sourceIds.includes('purchased.source'),'carrier hidden quality/supply source IDs omitted');
  result=model(electricity,{...carrierOptions,scope:{kind:'zone',zoneName:'Office'}});
  check(Boolean(breakdown(result,'contextRows','observed_subtotal'))&&!breakdown(result,'contextRows','facility_total'),'Zone source subtotal mislabeled complete facility total');
  result=model(conversion,{...options,kind:'link'});
  check(value(result,'from')===40&&value(result,'to')===20&&result.ruleIds.includes('PRIVATE_RULE_ID')&&result.representation.serviceKind==='cooling','selected conversion lost exact dual-domain values/service/rule evidence');
  check(!result.basis.explanation.includes('PRIVATE_'),'selected link friendly basis leaked technical IDs');
  const heatingLoad={...load,id:'load.heating',serviceKind:'heating',value:5};
  const driverLink={id:'driver.allocation',fromId:driver.id,toId:heatingLoad.id,relation:'driver_to_load',period:'M1',serviceKind:'heating',fromValue:5,toValue:5,fromUnit:'kWh thermal',toUnit:'kWh thermal',ruleId:'allocation.rule',sourceIds:['allocation.proof']};
  result=model(driver,{...options,nodes:[...options.nodes,heatingLoad],links:[...options.links,driverLink]});
  check(result.ruleIds.includes('allocation.rule')&&result.sourceIds.includes('allocation.proof')&&result.breakdown.ratioRows.length===0,'driver allocation trace was dropped or misclassified as a conversion ratio');
  check(JSON.stringify({driver,load,use,electricity,options})===snapshot,'inspector model mutated canonical graph/scopes/source metadata');
  check(value(model(null,{period:'M1'}),'total')===null,'empty inspector fabricated a zero');
  if(failures.length)throw new Error(failures.join(' | '));
  document.body.dataset.epath150Inspector='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.epath150Inspector='failed';document.getElementById('result').textContent=String(error?.stack||error);}
</script></body></html>`
