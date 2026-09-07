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

func TestEPATH151PureDriverDestinationsBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser driver destination verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath151-driver-destinations", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath151PureDestinationsHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath151-driver-destinations").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH151 pure destinations browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath151-destinations="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH151 pure destinations failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH151 pure destinations failed:\n%s", output)
	}
}

const epath151PureDestinationsHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH151 pure destinations</title></head>
<body data-epath151-destinations="pending"><pre id="result">pending</pre><script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const clone=value=>JSON.parse(JSON.stringify(value));
const freeze=value=>{if(value&&typeof value==='object'&&!Object.isFrozen(value)){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
try {
  const {energyPathDriverDestinations:destinations}=await import('/src/js/energy-path-driver-destinations.js');
  const navigation={entities:[],occurrences:[]};
  const semantic=(id,kind,view,targetKind,targetId,extra={})=>{
    const target={view,targetKind,targetId,label:'Target '+id};
    const entity={id,kind,label:'Model '+id,viewTargets:[target],...extra};
    navigation.entities.push(entity);
    navigation.occurrences.push({occurrenceId:'occurrence.'+id,entityId:id,path:'zones/Office/'+id,viewTargets:[target],sourceAnchor:extra.sourceAnchors?.[0]});
    return entity;
  };
  const geometry={topology:{nodes:[{id:'zone.office',entityId:'entity.office',kind:'zone',zoneName:'Office',label:'Office'},{id:'zone.lab',entityId:'entity.lab',kind:'zone',zoneName:'Lab',label:'Lab'},{id:'outdoors',kind:'outdoors',label:'Outdoors'}],boundaries:[],openings:[],connections:[],airCouplings:[]}};
  const boundary=(id,type,relation,zone='Office')=>{
    const record={id,surfaceId:'surface.'+id,surfaceEntityId:'entity.'+id,surfaceName:'Surface '+id,surfaceType:type,relationKind:relation,ownerZoneId:'zone.'+zone.toLowerCase()};
    geometry.topology.boundaries.push(record);semantic(id,'thermal_boundary','topology','thermal_boundary',id);return record;
  };
  boundary('wall.office','Wall','exterior');boundary('wall.lab','Wall','exterior','Lab');boundary('roof','Roof','exterior');boundary('floor','Floor','ground');boundary('outdoor.floor','Floor','exterior');boundary('interzone','Wall','interzone_explicit_surface');boundary('adiabatic','Wall','adiabatic_explicit');boundary('invalid','Wall','invalid');boundary('other.side','Wall','other_side_coefficients');
  boundary('roof.ceiling','RoofCeiling','exterior');
  const opening={id:'opening',windowId:'window',entityId:'window.entity',name:'Office window',surfaceType:'Window',ownerZoneId:'zone.office',baseSurfaceId:'surface.wall.office'};
  geometry.topology.openings.push(opening);semantic('window.entity','fenestration','topology','fenestration','window');
  geometry.topology.connections.push({id:'mixed.connection',fromNodeId:'zone.office',toNodeId:'outdoors',boundaryIds:['wall.office','roof'],openingIds:['opening']});semantic('mixed.connection','thermal_connection','topology','thermal_connection','mixed.connection');
  const coupling=(id,kind,from,to)=>{geometry.topology.airCouplings.push({id,entityId:id,objectName:'Coupling '+id,couplingKind:kind,fromNodeId:from,toNodeId:to});semantic(id,'thermal_air_coupling','topology','thermal_air_coupling',id);};
  coupling('mix','zone_mixing','zone.office','zone.lab');coupling('afn.internal','airflow_network','zone.office','zone.lab');coupling('afn.external','airflow_network','zone.office','outdoors');coupling('ventilation','outdoor_ventilation','outdoors','zone.office');coupling('broken.mix','zone_mixing','zone.office','missing.zone');
  semantic('entity.office','zone','topology','zone','zone.office');semantic('entity.lab','zone','topology','zone','zone.lab');
  const profile={zoneProfiles:[],groups:[]};
  let index=0;
  for(const zoneName of ['Office','Lab']){
    const zone={zoneName,items:[],dimensions:[]};profile.zoneProfiles.push(zone);
    for(const dimension of ['occupancy','lighting','equipment','infiltration','ventilation','outdoor_air']){
      const item={id:'item.'+zoneName+'.'+dimension,zoneName,dimension,objectIndex:index++,objectType:dimension==='occupancy'?'People':'Typed:'+dimension,objectName:zoneName+' '+dimension};zone.items.push(item);zone.dimensions.push({dimension,label:dimension,itemIds:[item.id]});
      semantic('entity.'+item.id,'profile-item','profile','profile-item',item.id,{sourceAnchors:[{objectId:'object.'+item.id,objectIndex:item.objectIndex,objectType:item.objectType,objectName:item.objectName}]});
      semantic('dimension.'+zoneName+'.'+dimension,'zone','profile','zone-dimension','profile-zone-dimension:'+zoneName.toLowerCase()+':'+dimension);
    }
  }
  const hvac={serviceModel:{zoneServices:[{zoneName:'Office',paths:[{id:'oa.path',zoneName:'Office',serviceKind:'ventilation',delivery:{id:'oa.unit',objectType:'ZoneHVAC:OutdoorAirUnit'}},{id:'cooling.path',zoneName:'Office',serviceKind:'cooling',delivery:{id:'cool.coil',objectType:'Coil:Cooling:Water'}},{id:'cooling.with.oa',zoneName:'Office',serviceKind:'cooling',conditioning:[{id:'oa.mixer',objectType:'OutdoorAir:Mixer'}]}]}]}};
  for(const path of hvac.serviceModel.zoneServices[0].paths)semantic('entity.'+path.id,'hvac-path','hvac','service-path',path.id);
  const source={id:'wall.source',zoneName:'Office',relatedEntityIds:['wall.office'],inputSourceIds:['wall.input']};
  const sources=[source,{id:'wall.input',inputSourceIds:['wall.source'],relatedEntityIds:['wall.office']},{id:'unrelated.source',relatedEntityIds:['wall.lab']}];
  const node={id:'driver.wall',level:'driver',driverCategory:'surface.exterior_walls',period:'M1',value:10,sourceIds:['wall.source']};
  const options={geometry,profile,hvac,semanticNavigation:navigation,sources,scope:{kind:'building'},period:'M1',zoneRows:[{key:'Office',value:6,unit:'kWh thermal'},{key:'Lab',value:4,unit:'kWh thermal'}]};
  const snapshot=JSON.stringify({node,options});freeze(node);freeze(options);
  const all=result=>result.groups.flatMap(group=>group.candidates);
  const targets=result=>all(result).map(candidate=>candidate.target.targetId);
  let result=destinations(node,options);
  check(result.status==='available'&&targets(result).includes('wall.office')&&targets(result).includes('wall.lab')&&!targets(result).includes('roof')&&!targets(result).includes('interzone')&&!targets(result).includes('adiabatic'),'exterior category matched roof/interzone/adiabatic or omitted walls');
  check(all(result).find(item=>item.target.targetId==='wall.office')?.evidenceKind==='exact_source'&&all(result).find(item=>item.target.targetId==='wall.lab')?.evidenceKind==='category_context','source identity confused with broad category context');
  check(result.groups.find(group=>group.zoneName==='Office')?.value===6,'strict supplied Zone value was not retained for chooser display');
  check(all(result).find(item=>item.target.targetId==='mixed.connection')?.evidenceKind==='category_context','mixed connection was presented as exact category attribution');
  check(targets(destinations({...node,driverCategory:'surface.roofs'},options)).includes('roof.ceiling'),'actual RoofCeiling type was omitted from roof context');
  for(const [category,wanted,unwanted] of [['surface.roofs','roof','wall.office'],['surface.ground_floors','floor','roof'],['surface.windows_doors','window','wall.office'],['surface.interzone','interzone','wall.office']]){
    const ids=targets(destinations({...node,driverCategory:category},options));check(ids.includes(wanted)&&!ids.includes(unwanted),'surface taxonomy destination mismatch '+category);
  }
  for(const [category,dimension] of [['internal.people','occupancy'],['internal.lighting','lighting'],['internal.equipment','equipment'],['air.infiltration','infiltration']]){
    result=destinations({...node,driverCategory:category},options);
    check(targets(result).includes('item.Office.'+dimension)&&!targets(result).includes('item.Office.'+(dimension==='lighting'?'equipment':'lighting')),'Profile dimension target mismatch '+category);
  }
  const profileSource={id:'profile.source',zoneName:'Office',relatedEntityIds:['object.item.Office.occupancy']};
  result=destinations({...node,driverCategory:'internal.people',sourceIds:['profile.source']},{...options,sources:[profileSource]});
  check(all(result).find(item=>item.target.targetId==='item.Office.occupancy')?.evidenceKind==='exact_source','exact Profile source object anchor was not recognized');
  result=destinations(node,{...options,sources:[{...source,zoneName:'Lab',inputSourceIds:[]}]});
  check(all(result).find(item=>item.target.targetId==='wall.office')?.evidenceKind==='category_context','wrong-zone source was promoted to exact target provenance');
  result=destinations({...node,driverCategory:'air.interzone'},options);
  check(targets(result).includes('mix')&&targets(result).includes('afn.internal')&&!targets(result).includes('afn.external')&&!targets(result).includes('ventilation')&&!targets(result).includes('broken.mix'),'air coupling classification used kind without valid endpoints');
  check(result.groups.every(group=>!group.zoneName&&group.value===null&&group.label.includes('Office')&&group.label.includes('Lab')),'Building shared air coupling was assigned one arbitrary owner Zone/value');
  result=destinations({...node,driverCategory:'air.interzone'},{...options,scope:{kind:'zone',zoneName:'Office'}});
  check(result.groups.length===1&&result.groups[0].zoneName==='Office'&&result.groups[0].value===6,'explicit Zone coupling context lost its own selected Zone contribution');
  result=destinations({...node,driverCategory:'air.infiltration'},options);
  check(targets(result).includes('afn.external')&&!targets(result).includes('afn.internal'),'infiltration borrowed interzone AFN coupling');
  result=destinations({...node,driverCategory:'air.mechanical_ventilation'},options);
  check(targets(result).includes('oa.path')&&targets(result).includes('cooling.with.oa')&&!targets(result).includes('cooling.path')&&targets(result).includes('item.Office.outdoor_air')&&targets(result).includes('ventilation'),'ventilation did not retain verified OA service/Profile or guessed unrelated cooling service');
  result=destinations(node,{...options,scope:{kind:'zone',zoneName:'Office'}});
  check(all(result).every(item=>item.zoneName==='Office')&&!targets(result).includes('wall.lab'),'Zone driver navigated another Zone');
  for(const bad of [{level:'load'},{driverCategory:'balance.storage_other'},{driverCategory:'wall.office'},{period:'annual'},{zoneName:'Lab'}]){
    check(destinations({...node,...bad},{...options,scope:{kind:'zone',zoneName:'Office'}}).status==='unavailable','unsupported/mismatched driver had navigation '+JSON.stringify(bad));
  }
  check(destinations(node,{...options,semanticNavigation:{}}).status==='unavailable','report record alone invented global semantic target');
  const badProfile=clone(options);badProfile.semanticNavigation.entities.find(item=>item.id==='entity.item.Office.occupancy').sourceAnchors[0].objectName='Wrong source';badProfile.semanticNavigation.occurrences.find(item=>item.entityId==='entity.item.Office.occupancy').sourceAnchor.objectName='Wrong source';
  check(!targets(destinations({...node,driverCategory:'internal.people'},badProfile)).includes('item.Office.occupancy'),'stale Profile source index was accepted despite name mismatch');
  const conflict=clone(options);conflict.geometry.topology.boundaries.push({...conflict.geometry.topology.boundaries[0],surfaceType:'Roof'});
  check(!targets(destinations(node,conflict)).includes('wall.office'),'conflicting duplicate report identity chose first record');
  const entityConflict=clone(options);entityConflict.semanticNavigation.entities.push({...entityConflict.semanticNavigation.entities[0],kind:'other'});
  check(!targets(destinations(node,entityConflict)).includes('wall.office'),'conflicting duplicate semantic entity chose first record');
  const surfaceRoutes=clone(options), surfaceTarget={view:'topology',targetKind:'surface',targetId:'surface.wall.office',label:'Office wall'};
  surfaceRoutes.semanticNavigation.entities.push({id:'entity.wall.office',kind:'surface',label:'Office wall',viewTargets:[surfaceTarget]});
  result=destinations(node,surfaceRoutes);
  check(!targets(result).includes('surface.wall.office')&&targets(result).includes('wall.office'),'same physical surface appeared twice despite a verified thermal boundary route');
  surfaceRoutes.semanticNavigation.entities=surfaceRoutes.semanticNavigation.entities.filter(item=>item.id!=='wall.office');
  surfaceRoutes.semanticNavigation.occurrences=surfaceRoutes.semanticNavigation.occurrences.filter(item=>item.entityId!=='wall.office');
  check(targets(destinations(node,surfaceRoutes)).includes('surface.wall.office'),'surface fallback disappeared when boundary semantic route was unavailable');
  const multipleContexts=clone(options), originalOccurrence=multipleContexts.semanticNavigation.occurrences.find(item=>item.entityId==='wall.office');
  Object.assign(originalOccurrence,{sourceAnchor:{objectId:'wall.object',objectIndex:20,objectName:'Office wall',objectType:'BuildingSurface:Detailed',fieldIndex:1,fieldName:'Surface Type'},contextKind:'zone_geometry'});
  multipleContexts.semanticNavigation.occurrences.push({...originalOccurrence,occurrenceId:'wall.construction.context',sourceAnchor:{...originalOccurrence.sourceAnchor,fieldIndex:2,fieldName:'Construction Name'}});
  result=destinations(node,multipleContexts);
  check(all(result).filter(item=>item.target.targetId==='wall.office').length===1&&all(result).find(item=>item.target.targetId==='wall.office').occurrenceId==='','verified physical entity target chose/duplicated arbitrary field occurrences');
  multipleContexts.semanticNavigation.entities.find(item=>item.id==='wall.office').viewTargets=[];
  result=destinations(node,multipleContexts);
  const contextual=all(result).filter(item=>item.target.targetId==='wall.office');
  check(contextual.length===2&&new Set(contextual.map(item=>item.contextLabel)).size===2&&contextual.every(item=>item.occurrenceId),'occurrence-only source field routes lost explicit distinguishable contexts');
  const staleEntity=clone(options);staleEntity.semanticNavigation.entities.push({id:'wrong.entity',kind:'surface',label:'Wrong source',viewTargets:[{view:'topology',targetKind:'thermal_boundary',targetId:'wall.office'}]});
  result=destinations({...node,sourceIds:[],relatedEntityIds:['wrong.entity']},staleEntity);
  check(all(result).filter(item=>item.entityId==='wrong.entity').every(item=>item.evidenceKind==='category_context'),'stale entity target alone falsely established exact source attribution');
  const stable=all(destinations(node,options)).map(item=>item.id).sort();
  const reversed=clone(options);reversed.geometry.topology.boundaries.reverse();reversed.semanticNavigation.entities.reverse();reversed.semanticNavigation.occurrences.reverse();reversed.sources.reverse();reversed.zoneRows=[{key:'Office',value:900,unit:'kWh thermal'},{key:'Lab',value:1000,unit:'kWh thermal'}];
  check(JSON.stringify(stable)===JSON.stringify(all(destinations({...node,value:999},reversed)).map(item=>item.id).sort()),'candidate IDs depend on values/Zone ranking/input order');
  check(JSON.stringify(stable)===JSON.stringify(all(destinations(node,{...options,zoneRows:[]})).map(item=>item.id).sort()),'click revalidation without Zone values changed candidate identity');
  check(JSON.stringify({node,options})===snapshot,'destination model mutated original graph/source/report');
  if(failures.length)throw Error(failures.join('\n'));
  document.body.dataset.epath151Destinations='passed';document.querySelector('#result').textContent='PASS';
}catch(error){document.body.dataset.epath151Destinations='failed';document.querySelector('#result').textContent=error.stack||String(error);}
</script></body></html>`
