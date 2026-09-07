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

func TestEPATH145PureOrderingAndDirectedFocusBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser fixed ordering and directed focus verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath145-focus", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath145PureFocusHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath145-focus").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH145 pure focus browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath145-focus="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH145 pure focus failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH145 pure focus failed:\n%s", output)
	}
}

const epath145PureFocusHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH145 pure focus</title></head>
<body data-epath145-focus="pending"><pre id="result">pending</pre><script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const ordered=value=>[...value].sort();
const same=(left,right)=>JSON.stringify(ordered(left))===JSON.stringify(ordered(right));
const freeze=value=>{if(value&&typeof value==='object'&&!Object.isFrozen(value)){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
try {
  const {energyPathCompareNodes:compare,energyPathFocus:focus,energyPathOrderLinks:orderLinks}=await import('/src/js/energy-path-focus.js');
  const {energyPathLayout:layout}=await import('/src/js/energy-path-layout.js');
  const {energyPathRibbons:ribbons}=await import('/src/js/energy-path-ribbons.js');
  const categories={
    driver:['surface.exterior_walls','surface.roofs','surface.ground_floors','surface.windows_doors','surface.interzone','air.infiltration','air.mechanical_ventilation','air.interzone','internal.people','internal.lighting','internal.equipment','interzone.transfer','balance.storage_other'],
    load:['cooling','heating'],
    end_use:['cooling','heating','fans_pumps','hvac_auxiliaries','lighting','equipment','water_systems','refrigeration','other'],
    carrier:['electricity','natural_gas','district_cooling','district_heating','steam','propane','fuel_oil_1','fuel_oil_2','coal','diesel','gasoline','other_fuel_1','other_fuel_2','water'],
  };
  const field={driver:'driverCategory',load:'serviceKind',end_use:'endUse',carrier:'carrier'};
  for(const [level,kinds] of Object.entries(categories)){
    const nodes=kinds.map((kind,index)=>({id:level+'.'+String(100-index),level,[field[level]]:kind,value:100-index,label:'label '+(100-index)}));
    const unknown={id:'000.unknown',level,value:1e12,label:'Cooling electricity'};
    const expected=[...nodes.map(node=>node.id),unknown.id];
    for(const input of [[...nodes,unknown].reverse(),[unknown,...nodes].map((node,index)=>({...node,value:index%2?-1e10:1e10,label:'changed '+index}))]){
      const before=JSON.stringify(input);freeze(input);
      const sorted=[...input].sort((a,b)=>compare({level},a,b)).map(node=>node.id);
      check(JSON.stringify(sorted)===JSON.stringify(expected),'taxonomy order varied with values/labels/input order: '+level);
      check(JSON.stringify(input)===before,'comparator mutated canonical node values: '+level);
    }
    const tied=[{...nodes[0],id:'z',value:1e9},{...nodes[0],id:'a',value:1}];
    check([...tied].sort((a,b)=>compare(level,a,b))[0].id==='a','same-taxonomy tie used energy magnitude instead of ID: '+level);
  }
  for(const [level,alias,canonical] of [['driver','internal.other','balance.storage_other'],['end_use','fans','fans_pumps'],['end_use','pumps','fans_pumps'],['end_use','heat_rejection','hvac_auxiliaries'],['carrier','gas','natural_gas'],['carrier','DistrictHeatingWater','district_heating'],['carrier','DistrictHeatingSteam','steam'],['carrier','FuelOilNo1','fuel_oil_1']]){
    check(compare(level,{id:'same',level,[field[level]]:alias},{id:'same',level,[field[level]]:canonical})===0,'typed alias changed taxonomy position: '+alias);
  }
  check(compare('end_use',{id:'a',level:'end_use',endUse:'fans_pumps',serviceKind:'heating'},{id:'z',level:'end_use',endUse:'fans_pumps',serviceKind:'cooling'})<0,'allocated auxiliary service overrode category order');
  check(compare('end_use',{id:'a',level:'residual',presentationLevel:'end_use',presentationKind:'unclassified_energy',value:1e9},{id:'z',level:'end_use',endUse:'other',value:1})>0,'residual moved ahead of named end-use taxonomy');
  check(compare('end_use',{id:'z',level:'residual',presentationKind:'unclassified_energy',value:1e9},{id:'a',level:'residual',presentationKind:'unclassified_energy',value:1})>0,'residual ties depended on input/value rather than ID');
  const thermal=(id,level,value,serviceKind,extra={})=>({id,level,value,serviceKind,unit:'kWh thermal',scaleDomain:'thermal',period:'annual',...extra});
  const site=(id,level,value,extra={})=>({id,level,value,unit:'kWh',scaleDomain:'site',period:'annual',...extra});
  const nodes=[
    thermal('driver.cooling','driver',80,'cooling',{driverCategory:'internal.lighting'}),
    thermal('driver.heating','driver',65,'heating',{driverCategory:'surface.roofs'}),
    thermal('driver.common','driver',40,'all',{driverCategory:'surface.exterior_walls',groupedMembers:[thermal('common.cooling','driver',20,'cooling'),thermal('common.heating','driver',20,'heating')]}),
    thermal('load.cooling','load',100,'cooling'),thermal('load.heating','load',85,'heating'),
    site('use.cooling','end_use',25,{endUse:'cooling',serviceKind:'cooling'}),site('use.heating','end_use',100,{endUse:'heating',serviceKind:'heating'}),
    site('use.lighting','end_use',10,{endUse:'lighting'}),site('use.fans','end_use',5,{endUse:'fans_pumps'}),site('use.isolated','end_use',1,{endUse:'other'}),
    site('carrier.electricity','carrier',52,{carrier:'electricity'}),site('carrier.gas','carrier',90,{carrier:'natural_gas'}),
    site('residual','residual',2,{presentationLevel:'end_use',presentationKind:'unclassified_energy',basis:'residual',signedValue:2}),
    site('support.production','support',10),
  ];
  const link=(id,fromId,toId,relation,fromValue,toValue,serviceKind='')=>({id,fromId,toId,relation,fromValue,toValue,serviceKind,period:'annual',fromUnit:relation==='driver_to_load'||relation==='load_to_end_use'?'kWh thermal':'kWh',toUnit:relation==='driver_to_load'?'kWh thermal':'kWh',sourceIds:['source.'+id],...(relation==='residual'?{basis:'residual'}:{})});
  const links=[
    link('driver.cool','driver.cooling','load.cooling','driver_to_load',80,80,'cooling'),link('common.cool','driver.common','load.cooling','driver_to_load',20,20,'cooling'),
    link('driver.heat','driver.heating','load.heating','driver_to_load',65,65,'heating'),link('common.heat','driver.common','load.heating','driver_to_load',20,20,'heating'),
    link('conversion.cool','load.cooling','use.cooling','load_to_end_use',100,25,'cooling'),link('conversion.heat','load.heating','use.heating','load_to_end_use',85,100,'heating'),
    link('cool.electric','use.cooling','carrier.electricity','end_use_to_carrier',25,25,'cooling'),link('heat.electric','use.heating','carrier.electricity','end_use_to_carrier',10,10,'heating'),link('heat.gas','use.heating','carrier.gas','end_use_to_carrier',90,90,'heating'),
    link('light.electric','use.lighting','carrier.electricity','direct_end_use_to_carrier',10,10),link('fans.electric','use.fans','carrier.electricity','direct_end_use_to_carrier',5,5),link('residual.electric','residual','carrier.electricity','residual',2,2),
    link('counterpart','driver.cooling','use.lighting','source_correspondence',10,10),link('support.link','support.production','carrier.electricity','support_supply',10,10),
    link('invalid.cool.fan','load.cooling','use.fans','load_to_end_use',1,1,'cooling'),
  ];
  const before=JSON.stringify({nodes,links});freeze(nodes);freeze(links);
  const geometry=layout([...nodes].sort((a,b)=>compare('',a,b)),links),drawing=ribbons(geometry,{period:'annual'});
  check(drawing.ribbons.length===12&&!drawing.ribbons.some(item=>['counterpart','support.link','invalid.cool.fan'].includes(item.id)),'focus fixture did not use real quantitative validation');
  freeze(geometry);freeze(drawing);
  const expect=(selection,wantedNodes,wantedLinks,counterparts=[])=>{
    const result=focus(geometry,drawing,selection,new Set(counterparts));
    check(result.active&&same(result.nodeIDs,wantedNodes)&&same(result.linkIDs,wantedLinks),'directed focus frontier leaked/missed a physical branch: '+selection);
    check(result.nodeIDs instanceof Set&&result.linkIDs instanceof Set&&result.counterpartIDs instanceof Set,'focus lost Set membership contract');
    return result;
  };
  const coolNodes=['driver.cooling','driver.common','load.cooling','use.cooling','carrier.electricity'];
  const coolLinks=['driver.cool','common.cool','conversion.cool','cool.electric'];
  let result=expect('load.cooling',coolNodes,coolLinks,['use.lighting','missing','load.cooling']);
  check(result.selectedNodeID==='load.cooling'&&!result.selectedLinkID&&same(result.counterpartIDs,['use.lighting'])&&!result.nodeIDs.has('use.lighting'),'counterpart became physical flow or self/missing counterpart survived');
  expect('use.cooling',coolNodes,coolLinks);
  result=expect('cool.electric',coolNodes,coolLinks,['use.lighting']);
  check(result.selectedLinkID==='cool.electric'&&!result.selectedNodeID&&result.counterpartIDs.size===0,'selected link borrowed node correspondence highlights');
  expect('driver.cooling',['driver.cooling','load.cooling','use.cooling','carrier.electricity'],['driver.cool','conversion.cool','cool.electric']);
  expect('driver.common',['driver.common','load.cooling','load.heating','use.cooling','use.heating','carrier.electricity','carrier.gas'],['common.cool','common.heat','conversion.cool','conversion.heat','cool.electric','heat.electric','heat.gas']);
  expect('conversion.heat',['driver.heating','driver.common','load.heating','use.heating','carrier.electricity','carrier.gas'],['driver.heat','common.heat','conversion.heat','heat.electric','heat.gas']);
  expect('carrier.gas',['driver.heating','driver.common','load.heating','use.heating','carrier.gas'],['driver.heat','common.heat','conversion.heat','heat.gas']);
  expect('carrier.electricity',['driver.cooling','driver.heating','driver.common','load.cooling','load.heating','use.cooling','use.heating','use.lighting','use.fans','residual','carrier.electricity'],links.slice(0,12).filter(item=>item.id!=='heat.gas').map(item=>item.id));
  result=expect('use.lighting',['use.lighting','carrier.electricity'],['light.electric'],['driver.cooling']);
  check(same(result.counterpartIDs,['driver.cooling'])&&!result.linkIDs.has('driver.cool')&&!result.nodeIDs.has('driver.cooling'),'lighting correspondence manufactured a thermal demand path');
  expect('residual',['residual','carrier.electricity'],['residual.electric']);
  expect('use.isolated',['use.isolated'],[]);
  for(const selection of ['',null,'unknown','support.production','support.link','counterpart','invalid.cool.fan']){
    const inactive=focus(geometry,drawing,selection,['use.lighting']);
    check(!inactive.active&&!inactive.selectedNodeID&&!inactive.selectedLinkID&&inactive.nodeIDs.size===0&&inactive.linkIDs.size===0&&inactive.counterpartIDs.size===0,'unknown/support/invalid selection dimmed graph or retained stale focus: '+selection);
  }
  const ambiguous=focus(geometry,{ribbons:[...drawing.ribbons,{...drawing.ribbons[0],id:'load.cooling'}]},'load.cooling');
  check(!ambiguous.active,'ambiguous node/link identity used first-match selection');
  const reversed=focus({...geometry,nodes:[...geometry.nodes].reverse()},{...drawing,ribbons:[...drawing.ribbons].reverse()},'load.cooling');
  check(same(reversed.nodeIDs,coolNodes)&&same(reversed.linkIDs,coolLinks),'focus depended on traversal input order');
  const portNodes=[...nodes.map(node=>({...node,rawValue:node.value*10})),site('carrier.aaa_district','carrier',3,{carrier:'district_heating',rawValue:30})].sort((a,b)=>compare('',a,b));
  const portLinks=[...links.slice(0,12),link('district.branch','use.heating','carrier.aaa_district','end_use_to_carrier',3,3,'heating')]
    .map((item,index)=>({...item,id:'misleading.'+String(99-index).padStart(2,'0')}));
  const portBefore=JSON.stringify({portNodes,portLinks});freeze(portNodes);freeze(portLinks);
  const sortedLinks=orderLinks(portNodes,portLinks),reverseSorted=orderLinks(portNodes,[...portLinks].reverse());
  check(sortedLinks!==portLinks&&sortedLinks.every(item=>portLinks.includes(item))&&sortedLinks.length===portLinks.length,'link ordering copied/dropped/replaced original link records');
  check(JSON.stringify(sortedLinks.map(item=>item.id))===JSON.stringify(reverseSorted.map(item=>item.id)),'link order depended on input order');
  const fixedDrawing=ribbons(layout(portNodes,sortedLinks)),reverseDrawing=ribbons(layout(portNodes,reverseSorted));
  const unsortedDrawing=ribbons(layout(portNodes,portLinks));
  check(fixedDrawing.ribbons.length===13,'real multiple-carrier/driver port fixture lost typed physical links');
  const indexByID=new Map(portNodes.map((node,index)=>[node.id,index]));
  for(const [endpoint,otherEndpoint,port] of [['fromId','toId','fromPort'],['toId','fromId','toPort']]){
    const groups=new Map();
    for(const item of fixedDrawing.ribbons)groups.set(item[endpoint],[...(groups.get(item[endpoint])||[]),item]);
    for(const group of groups.values()){
      const actual=[...group].sort((a,b)=>a[port].y0-b[port].y0);
      const expected=[...group].sort((a,b)=>indexByID.get(a[otherEndpoint])-indexByID.get(b[otherEndpoint])||(a.id<b.id?-1:a.id>b.id?1:0));
      check(JSON.stringify(actual.map(item=>item.id))===JSON.stringify(expected.map(item=>item.id)),'ribbon port stack followed link ID/value instead of opposite endpoint taxonomy: '+endpoint);
    }
  }
  for(const item of fixedDrawing.ribbons){
    const reversedItem=reverseDrawing.ribbons.find(other=>other.id===item.id),originalItem=unsortedDrawing.ribbons.find(other=>other.id===item.id);
    check(JSON.stringify([item.fromPort,item.toPort,item.path])===JSON.stringify([reversedItem.fromPort,reversedItem.toPort,reversedItem.path]),'reversing link input changed fixed port coordinates');
    check(item.link===portLinks.find(link=>link.id===item.id)&&item.fromValue===originalItem.fromValue&&item.toValue===originalItem.toValue&&item.fromWidth===originalItem.fromWidth&&item.toWidth===originalItem.toWidth,'port ordering altered original paired values/widths/ID/source reference');
  }
  const heatingBranches=sortedLinks.filter(item=>item.fromId==='use.heating');
  check(JSON.stringify(heatingBranches.map(item=>item.toId))==='["carrier.electricity","carrier.gas","carrier.aaa_district"]','carrier ID lexical order overrode fixed Electricity/Gas/District port order');
  const ties=orderLinks(portNodes,[{id:'z',fromId:'use.heating',toId:'carrier.gas',value:1000},{id:'a',fromId:'use.heating',toId:'carrier.gas',value:1}]);
  check(ties[0].id==='a','same-port tie used energy magnitude instead of link ID');
  const unknownLink={id:'000.unknown',fromId:'missing',toId:'carrier.electricity'};
  check(orderLinks(portNodes,[unknownLink,...portLinks]).at(-1)===unknownLink,'ordering silently dropped or prioritized unknown endpoint');
  check(JSON.stringify({portNodes,portLinks})===portBefore,'fixed edge ordering changed canonical IDs/raw values/source JSON');
  result.nodeIDs.add('external');result.linkIDs.clear();
  check(!focus(geometry,drawing,'use.lighting').nodeIDs.has('external')&&focus(geometry,drawing,'use.lighting').linkIDs.has('light.electric'),'returned Sets leaked mutable state across calls');
  check(JSON.stringify({nodes,links})===before,'focus/order mutated canonical values/source IDs/links');
  check(!focus(null,null,'missing').active,'absent geometry fabricated active selection');
  if(failures.length)throw new Error(failures.join(' | '));
  document.body.dataset.epath145Focus='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.epath145Focus='failed';document.getElementById('result').textContent=String(error?.stack||error);}
</script></body></html>`
