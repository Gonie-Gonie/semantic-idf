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

func TestEPATH144PureAppearanceBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser semantic appearance verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath144-appearance", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath144PureAppearanceHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath144-appearance").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH144 pure appearance browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath144-appearance="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH144 pure appearance failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH144 pure appearance failed:\n%s", output)
	}
}

const epath144PureAppearanceHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH144 pure appearance</title></head>
<body data-epath144-appearance="pending"><pre id="result">pending</pre><script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const clone=value=>JSON.parse(JSON.stringify(value));
const freeze=(value,seen=new Set())=>{if(value&&typeof value==='object'&&!seen.has(value)){seen.add(value);Object.values(value).forEach(item=>freeze(item,seen));Object.freeze(value);}return value;};
try {
  const {energyPathNodeAppearance:nodeStyle,energyPathLinkAppearance:linkStyle}=await import('/src/js/energy-path-appearance.js');
  const neutral=nodeStyle({}).colorKey;
  const cooling={id:'load.cooling',level:'load',serviceKind:'cooling',value:100};
  const heating={id:'load.heating',level:'load',serviceKind:'heating',value:85};
  const coolColor=nodeStyle(cooling).colorKey,heatColor=nodeStyle(heating).colorKey;
  check(coolColor!==heatColor&&coolColor!==neutral&&heatColor!==neutral,'Cooling/Heating/unknown did not have distinct typed appearances');
  for(const serviceKind of ['all','','unknown'])check(nodeStyle({...cooling,serviceKind}).colorKey===neutral,'unknown/all load service inferred Cooling from ID');
  check(nodeStyle({id:'load.cooling',label:'Cooling electricity',kind:'load.cooling'}).colorKey===neutral,'untyped node guessed color from ID/label/kind');
  check(nodeStyle({level:'carrier',id:'carrier.electricity',label:'Electricity',carrier:'unrecognized'}).colorKey===neutral,'unknown typed carrier guessed Electricity from label/ID');
  check(nodeStyle({level:'driver',driverCategory:'unknown',label:'Cooling wall',serviceKind:'cooling'}).colorKey===neutral,'unknown driver acquired service color');
  for(const driverCategory of ['surface.exterior_walls','surface.roofs','surface.ground_floors','surface.windows_doors','surface.interzone','air.infiltration','air.mechanical_ventilation','air.interzone','internal.people','internal.lighting','internal.equipment','balance.storage_other']){
    const direct={level:'driver',driverCategory,value:20};
    check(nodeStyle({...direct,serviceKind:'cooling'}).colorKey===nodeStyle({...direct,serviceKind:'heating'}).colorKey,'driver category color depended on service: '+driverCategory);
    check(!nodeStyle(direct).residual,'driver Other/storage or category misrepresented as carrier residual: '+driverCategory);
  }
  for(const [alias,canonical] of [['Electricity','electricity'],['gas','natural_gas'],['NaturalGas','natural_gas'],['DistrictCooling','district_cooling'],['DistrictHeatingWater','district_heating'],['DistrictHeatingSteam','steam'],['FuelOilNo1','fuel_oil_1'],['FuelOilNo2','fuel_oil_2'],['OtherFuel1','other_fuel_1'],['OtherFuel2','other_fuel_2']]){
    check(nodeStyle({level:'carrier',carrier:alias}).colorKey===nodeStyle({level:'carrier',carrier:canonical}).colorKey,'carrier alias changed fixed appearance: '+alias);
  }
  for(const carrier of ['electricity','natural_gas','district_cooling','district_heating','steam','propane','fuel_oil_1','fuel_oil_2','coal','diesel','gasoline','other_fuel_1','other_fuel_2','water']){
    check(nodeStyle({level:'carrier',carrier,serviceKind:'cooling'}).colorKey===nodeStyle({level:'carrier',carrier,serviceKind:'heating'}).colorKey,'carrier color varied by selected service: '+carrier);
  }
  check(nodeStyle({level:'end_use',endUse:'cooling',serviceKind:'heating'}).colorKey===coolColor,'explicit Cooling endUse lost precedence to contradictory service');
  check(nodeStyle({level:'end_use',endUse:'heating',serviceKind:'cooling'}).colorKey===heatColor,'explicit Heating endUse lost precedence to contradictory service');
  for(const endUse of ['fans','pumps','fans_pumps','heat_rejection','humidification','heat_recovery','hvac_auxiliaries','lighting','equipment','water_systems','refrigeration','other']){
    check(nodeStyle({level:'end_use',endUse,serviceKind:'cooling'}).colorKey===nodeStyle({level:'end_use',endUse,serviceKind:'heating'}).colorKey,'direct/aux category inherited allocated service color: '+endUse);
  }
  for(const [alias,canonical] of [['fans','fans_pumps'],['pumps','fans_pumps'],['heat_rejection','hvac_auxiliaries'],['humidification','hvac_auxiliaries'],['heat_recovery','hvac_auxiliaries']]){
    check(nodeStyle({level:'end_use',endUse:alias}).colorKey===nodeStyle({level:'end_use',endUse:canonical}).colorKey,'end-use alias diverged from canonical category: '+alias);
  }
  const energy={id:'use.cooling',level:'end_use',endUse:'cooling',value:25,sourceIds:['meter.cooling']};
  for(const evidence of [{allocationApplied:true},{basis:'heat_balance_share'},{basis:'service_path_allocation'},{basis:'zone_load_allocation'},{basis:'allocated'},{badges:['allocated']}]){
    const appearance=nodeStyle({...energy,...evidence});
    check(appearance.allocated===true&&appearance.allocationLabelKind==='allocated'&&appearance.colorKey===nodeStyle(energy).colorKey,'explicit allocation lost same semantic color or allocation label: '+JSON.stringify(evidence));
  }
  for(const evidence of [{allocatedValue:25},{allocationFactor:.5},{allocationApplied:false},{allocationApplied:'true'},{basis:'derived_ratio'},{basis:'direct_zone_energy'},{basis:'reported_meter'},{basis:'reported_variable'},{basis:'reported_end_use_subtotal'},{basis:'integrated_rate'},{basis:'unknown'},{badges:['partial','filtered_carrier_splits']},{label:'Allocated cooling'}]){
    const appearance=nodeStyle({...energy,...evidence});
    check(appearance.allocated===false&&appearance.allocationLabelKind==='','non-allocation evidence invented hatch/badge: '+JSON.stringify(evidence));
  }
  const allocated={...energy,id:'member.allocated',basis:'service_path_allocation',allocationApplied:true};
  const reported={...energy,id:'member.reported',basis:'direct_zone_energy',allocationApplied:false};
  const mixed={...allocated,id:'group.mixed',basis:'grouped_presentation',groupedMembers:[allocated,reported],originalNodeIds:[allocated.id,reported.id]};
  check(nodeStyle(mixed).allocated&&nodeStyle(mixed).allocationLabelKind==='includes_allocated','mixed member provenance mislabeled wholly Allocated from first member flag');
  check(nodeStyle({...mixed,allocationApplied:false,groupedMembers:[reported,allocated]}).allocationLabelKind==='includes_allocated','mixed grouping became order-dependent or ignored allocated member');
  check(nodeStyle({...mixed,groupedMembers:[allocated,{...allocated,id:'member.second'}]}).allocationLabelKind==='allocated','all-allocated group lost exact allocation evidence');
  check(!nodeStyle({...mixed,groupedMembers:[reported,{...reported,id:'member.second'}]}).allocated,'all-reported group inherited stale outer allocation flag');
  check(nodeStyle({...mixed,groupedMembers:[allocated,{id:'member.unknown'}]}).allocationLabelKind==='includes_allocated','unknown member implied wholly allocated group');
  const nested={id:'nested',level:'end_use',endUse:'cooling',groupedMembers:[mixed,allocated]};
  check(nodeStyle(nested).allocationLabelKind==='includes_allocated','nested grouped provenance lost mixed evidence');
  const cycle={id:'cycle',level:'end_use',endUse:'cooling'};cycle.groupedMembers=[cycle,allocated];
  check(nodeStyle(cycle).allocationLabelKind==='includes_allocated','cyclic grouped provenance fabricated complete allocation or failed safe traversal');
  const residual={id:'residual.electricity',level:'residual',presentationLevel:'end_use',presentationKind:'unclassified_energy',basis:'residual',badges:['unclassified_energy'],value:5,signedValue:5,carrier:'electricity',unit:'kWh site',scaleDomain:'site'};
  const residualStyle=nodeStyle(residual);
  check(residualStyle.residual===true&&residualStyle.allocated===false,'qualified positive residual lost separate hatch semantics');
  check(nodeStyle({...residual,carrier:'natural_gas'}).colorKey===residualStyle.colorKey,'residual inherited carrier hue instead of neutral gray');
  for(const invalid of [{...energy,basis:'residual',label:'Residual'},{...energy,presentationKind:'unclassified_energy'},{...residual,value:-5,signedValue:-5},{...residual,value:5,signedValue:-5},{...residual,value:0}])check(!nodeStyle(invalid).residual,'unqualified/negative/zero residual received main-flow residual styling');
  const other=nodeStyle({level:'driver',driverCategory:'balance.storage_other',basis:'heat_balance_share',allocationApplied:true,rawValue:0,value:7});
  check(other.allocated&&!other.residual,'allocated Other/storage confused with residual or raw zero');
  const driver={id:'driver.people',level:'driver',driverCategory:'internal.people',serviceKind:'all',basis:'heat_balance_share',allocationApplied:true};
  const carrier={id:'carrier.electricity',level:'carrier',carrier:'electricity',basis:'reported_meter'};
  const nodes=[driver,cooling,heating,energy,carrier,residual];
  const paths=[
    {id:'driver.cooling',relation:'driver_to_load',fromId:driver.id,toId:cooling.id,serviceKind:'cooling',basis:'heat_balance_share'},
    {id:'conversion.cooling',relation:'load_to_end_use',fromId:cooling.id,toId:energy.id,serviceKind:'cooling',basis:'derived_ratio'},
    {id:'consumption',relation:'end_use_to_carrier',fromId:energy.id,toId:carrier.id,basis:'reported_meter'},
    {id:'residual',relation:'residual',fromId:residual.id,toId:carrier.id,basis:'residual',fromValue:5,toValue:5},
  ];
  check(linkStyle(paths[0],nodes).colorKey===coolColor&&linkStyle(paths[0],nodes).allocated,'driver-load did not use target service plus link allocation evidence');
  check(linkStyle(paths[1],nodes).colorKey===coolColor&&!linkStyle(paths[1],nodes).allocated,'conversion borrowed upstream driver allocation or lost service color');
  check(linkStyle(paths[2],nodes).colorKey===nodeStyle(carrier).colorKey,'end-use carrier ribbon lost fixed target carrier identity');
  check(linkStyle(paths[3],nodes).residual&&linkStyle(paths[3],nodes).colorKey===residualStyle.colorKey,'qualified residual ribbon lost gray hatch');
  check(!linkStyle({...paths[2],allocatedValue:25},nodes.map(node=>node.id===energy.id?allocated:node)).allocated,'reported carrier split inherited allocated endpoint/allocatedValue');
  check(linkStyle({...paths[2],basis:'service_path_allocation'},nodes).allocated,'allocated carrier split lost its own basis evidence');
  check(!linkStyle({...paths[3],fromId:energy.id},nodes).residual,'residual relation invented residual source identity');
  for(const invalid of [{...paths[0],relation:'unknown'},{...paths[2],toId:'missing'},{...paths[2],relation:'source_correspondence'},{...paths[2],fromId:carrier.id,toId:energy.id}])check(linkStyle(invalid,nodes).colorKey===neutral,'invalid/unknown/non-flow relation acquired physical color');
  const snapshot=JSON.stringify({nodes,paths,mixed});freeze(nodes);freeze(paths);freeze(mixed);
  for(const node of nodes)nodeStyle(node);for(const path of paths)linkStyle(path,nodes);nodeStyle(mixed);
  check(JSON.stringify({nodes,paths,mixed})===snapshot,'appearance mutated canonical values, IDs, or grouping provenance');
  check(JSON.stringify(nodeStyle(clone(mixed)))===JSON.stringify(nodeStyle(mixed)),'appearance depends on object identity rather than immutable metadata');
  const {energyPathGraphForState}=await import('/src/js/views/energy-path-view.js');
  for(const [bases,expected] of [[['direct_zone_energy','service_path_allocation'],'includes_allocated'],[['zone_load_allocation','service_path_allocation'],'allocated'],[['direct_zone_energy','reported_meter'],'']]){
    const input={schema:'semantic-idf.energy-explanation/v2',scope:{kind:'building',aggregationBasis:'model_total'},nodes:[
      {id:'end_use.fans.building',level:'end_use',endUse:'fans',serviceKind:'cooling',carrier:'electricity',value:12,rawValue:12,unit:'kWh',scaleDomain:'site',basis:bases[0],sourceIds:['fan.meter']},
      {id:'end_use.pumps.building',level:'end_use',endUse:'pumps',serviceKind:'cooling',carrier:'electricity',value:8,rawValue:80,unit:'kWh',scaleDomain:'site',basis:bases[1],sourceIds:['pump.meter','pump.allocation']},
      {...carrier,value:20,rawValue:20,unit:'kWh',scaleDomain:'site',sourceIds:['facility.meter']},
    ],links:[
      {id:'fan.branch',fromId:'end_use.fans.building',toId:carrier.id,relation:'end_use_to_carrier',fromValue:12,toValue:12,fromUnit:'kWh',toUnit:'kWh',serviceKind:'cooling',period:'annual',basis:bases[0],sourceIds:['fan.meter']},
      {id:'pump.branch',fromId:'end_use.pumps.building',toId:carrier.id,relation:'end_use_to_carrier',fromValue:8,toValue:8,fromUnit:'kWh',toUnit:'kWh',serviceKind:'cooling',period:'annual',basis:bases[1],sourceIds:['pump.meter','pump.allocation']},
    ],sources:[{id:'fan.meter'},{id:'pump.meter'},{id:'pump.allocation'},{id:'facility.meter'}]};
    const before=JSON.stringify(input);freeze(input);
    const graph=energyPathGraphForState(input,{simulationEnergyScopeKind:'building',simulationEnergyPeriod:'annual',simulationEnergyService:'all'});
    const fanPump=graph.nodes.find(node=>node.endUse==='fans_pumps'),branch=graph.links.find(link=>link.fromId===fanPump?.id&&link.toId===carrier.id);
    check(Boolean(fanPump)&&Boolean(branch)&&graph.links.length===1,'actual fan/pump projection failed to merge one exact carrier branch');
    check(fanPump?.value===20&&branch?.fromValue===20&&branch?.toValue===20,'appearance projection changed reported paired energy sums');
    check(nodeStyle(fanPump).allocationLabelKind===expected&&linkStyle(branch,graph.nodes).allocationLabelKind===expected,'actual graph projection lost own mixed/direct/allocated link evidence: '+bases.join('+'));
    check(branch?.sourceIds.includes('fan.meter')&&branch?.sourceIds.includes('pump.meter')&&branch?.sourceIds.includes('pump.allocation'),'appearance projection dropped exact branch provenance');
    check(JSON.stringify(input)===before,'actual graph appearance mutated canonical JSON/raw values/IDs/sources');
  }
  const {energyPathGroupSmallNodes}=await import('/src/js/energy-path-grouping.js');
  for(const secondBasis of ['reported_meter','service_path_allocation']){
    const input={nodes:[
      {id:'end_use.cooling.building',level:'end_use',endUse:'cooling',serviceKind:'cooling',value:1000,unit:'kWh',scaleDomain:'site'},
      {id:'end_use.lighting.building',level:'end_use',endUse:'lighting',serviceKind:'all',value:1,rawValue:10,unit:'kWh',scaleDomain:'site',sourceIds:['light.a','light.b']},
      {id:'end_use.equipment.building',level:'end_use',endUse:'equipment',serviceKind:'all',value:2,rawValue:20,unit:'kWh',scaleDomain:'site',sourceIds:['equipment.a','equipment.b']},
      {...carrier,value:1003,unit:'kWh',scaleDomain:'site'},
    ],links:[
      {id:'light.projected',fromId:'end_use.lighting.building',toId:carrier.id,relation:'end_use_to_carrier',basis:'grouped_presentation',serviceKind:'all',period:'annual',fromValue:1,toValue:1,fromUnit:'kWh',toUnit:'kWh',sourceIds:['light.a','light.b'],groupedMembers:[{id:'light.a',basis:'zone_load_allocation',sourceIds:['light.a']},{id:'light.b',basis:'service_path_allocation',sourceIds:['light.b']}]},
      {id:'equipment.projected',fromId:'end_use.equipment.building',toId:carrier.id,relation:'end_use_to_carrier',basis:'grouped_presentation',serviceKind:'all',period:'annual',fromValue:2,toValue:2,fromUnit:'kWh',toUnit:'kWh',sourceIds:['equipment.a','equipment.b'],groupedMembers:[{id:'equipment.a',basis:'direct_zone_energy',sourceIds:['equipment.a']},{id:'equipment.b',basis:secondBasis,sourceIds:['equipment.b']}]},
    ]};
    const before=JSON.stringify(input);freeze(input);
    for(const reverse of [false,true]){
      const result=energyPathGroupSmallNodes(reverse?[...input.nodes].reverse():input.nodes,reverse?[...input.links].reverse():input.links);
      const other=result.nodes.find(node=>node.automaticOther),branch=result.links.find(link=>link.fromId===other?.id&&link.toId===carrier.id);
      check(Boolean(other)&&other.value===3&&result.links.length===1&&branch?.fromValue===3&&branch?.toValue===3,'automatic Other projection lost exact summed link values');
      check(linkStyle(branch,result.nodes).allocationLabelKind==='includes_allocated','automatic Other link lost mixed original allocation evidence or depended on input order');
      check(branch?.groupedMembers?.length===4&&branch.sourceIds.length===4,'automatic Other link dropped original allocation members/source IDs');
    }
    check(JSON.stringify(input)===before,'automatic Other appearance merge mutated canonical IDs/raw values/source JSON');
  }
  if(failures.length)throw new Error(failures.join(' | '));
  document.body.dataset.epath144Appearance='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.epath144Appearance='failed';document.getElementById('result').textContent=String(error?.stack||error);}
</script></body></html>`
