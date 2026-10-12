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

func TestEnergyPathContextRibbonGeometryBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser measured association ribbon verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/context-ribbons", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathContextRibbonGeometryHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--window-size=1600,1000", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/context-ribbons").CombinedOutput()
	if err != nil {
		t.Fatalf("measured association ribbon browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-context-ribbons="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("measured association ribbon geometry failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("measured association ribbon geometry failed:\n%s", output)
	}
}

const energyPathContextRibbonGeometryHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Measured association ribbons</title></head>
<body data-context-ribbons="pending"><pre id="result">pending</pre><script type="module">
const check=(condition,message)=>{if(!condition)throw new Error(message);};
const near=(left,right)=>Math.abs(left-right)<=1e-8*Math.max(1,Math.abs(left),Math.abs(right));
const freeze=value=>{if(value&&typeof value==='object'){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
try{
 const [{energyPathLayout:layout},{energyPathRibbons:flow},{energyPathContextConnectors:context,energyPathGroupInputs:groupInputs,synchronizeEnergyPathContextValues:synchronize},{energyPathLinkAppearance:paint},{energyPathFocus:focus,energyPathCompareNodes:compare}]=await Promise.all([
  import('/src/js/energy-path-layout.js'),import('/src/js/energy-path-ribbons.js'),import('/src/js/energy-path-context.js'),import('/src/js/energy-path-appearance.js'),import('/src/js/energy-path-focus.js')]);
 const make=(id,level,value,details={})=>({id,level,value,period:'annual',unit:'kWh',scaleDomain:level==='input'?'boundary':['driver','load'].includes(level)?'thermal':'site',...details});
 const nodes=[make('solar','input',1000,{kind:'input.solar_incident',signedValue:1000}),make('convection','input',30,{kind:'input.exterior_convection',signedValue:-30}),make('people','input',40,{kind:'input.people',signedValue:40}),
  make('wall','driver',70,{driverCategory:'surface.exterior_walls',serviceKind:'cooling'}),make('internal','driver',10,{driverCategory:'internal.people',serviceKind:'cooling'}),
  make('cooling','load',80,{serviceKind:'cooling'}),make('cooling.energy','end_use',20,{endUse:'cooling',serviceKind:'cooling'}),make('auxiliary','end_use',15,{endUse:'fans_pumps',serviceKind:'all'}),make('electricity','carrier',35,{carrier:'electricity'})];
 const link=(id,relation,fromId,toId,fromValue,toValue,details={})=>({id,relation,fromId,toId,fromValue,toValue,fromUnit:'kWh',toUnit:'kWh',period:'annual',sourceIds:['source.'+id],...details});
 const canonical=[link('wall.load','driver_to_load','wall','cooling',70,70,{serviceKind:'cooling'}),link('internal.load','driver_to_load','internal','cooling',10,10,{serviceKind:'cooling'}),link('conversion','load_to_end_use','cooling','cooling.energy',80,20,{serviceKind:'cooling',ratio:4,ratioKind:'coefficient_of_performance'}),link('cooling.carrier','end_use_to_carrier','cooling.energy','electricity',20,20),link('aux.carrier','direct_end_use_to_carrier','auxiliary','electricity',15,15)];
 const associations=[link('solar.wall','input_to_driver','solar','wall',1000,70),link('convection.wall','input_to_driver','convection','wall',-30,70),link('people.internal','input_to_driver','people','internal',40,10),
  link('fan.cooling','load_to_auxiliary','cooling','auxiliary',60,6,{serviceKind:'cooling',relatedPathIds:['fan.path'],basis:'service_path_allocation'}),link('pump.cooling','load_to_auxiliary','cooling','auxiliary',40,1,{serviceKind:'cooling',relatedPathIds:['pump.path'],basis:'service_path_allocation'})];
 const before=JSON.stringify({nodes,canonical,associations});freeze(nodes);freeze(canonical);freeze(associations);
 const geometry=layout(nodes,[...canonical,...associations]),ribbons=context(geometry,associations),accounting=flow(geometry);
 check(ribbons.length===5,'valid input/auxiliary associations were lost');
 check(JSON.stringify(accounting)===JSON.stringify(flow(layout(nodes,canonical))),'context changed accounting widths, ports or COP');
 const nodeByID=new Map(geometry.nodes.map(node=>[node.id,node]));
 for(const ribbon of ribbons){
  check(ribbon.path.endsWith(' Z')&&!('arrowPath'in ribbon)&&!/NaN|Infinity/.test(ribbon.path),'association did not become a finite closed ribbon without an arrowhead');
  for(const side of['from','to']){
   const port=ribbon[side+'Port'],width=ribbon[side+'Width'],value=ribbon[side+'Value'],slot=nodeByID.get(ribbon[side+'Id']);
   check(near(width,Math.abs(value)*ribbon[side+'PixelsPerKWh'])&&near(port.y1-port.y0,width),'endpoint magnitude lost its own linear domain scale');
   check(port.y0>=slot.y-1e-8&&port.y1<=slot.y+slot.height+1e-8,'context reference ribbon escaped its visual node lane');
  }
  check(ribbon.link===associations.find(item=>item.id===ribbon.id),'association lost original source/quantity identity');
 }
 const negative=ribbons.find(item=>item.id==='convection.wall');check(negative.fromValue===-30&&negative.fromWidth>0,'negative observation lost its sign or magnitude geometry');
 const fan=ribbons.find(item=>item.id==='fan.cooling'),pump=ribbons.find(item=>item.id==='pump.cooling');
 check(fan.fromValue===60&&pump.fromValue===40&&fan.toValue===6&&pump.toValue===1&&fan.path!==pump.path,'grouped fan/pump relation acquired another owner reference');
 check(fan.fromPort.y1<pump.fromPort.y0&&fan.toPort.y1<pump.toPort.y0,'fan and pump visual reference ports overlap');
 check(near(fan.fromWidth/pump.fromWidth,1.5)&&near(fan.toWidth/pump.toWidth,6),'independent thermal/site reference magnitude ratios were altered');
 const svg=document.createElementNS('http://www.w3.org/2000/svg','svg');svg.setAttribute('width',geometry.width);svg.setAttribute('height',geometry.height);svg.setAttribute('viewBox','0 0 '+geometry.width+' '+geometry.height);document.body.append(svg);
 for(const ribbon of ribbons){const path=document.createElementNS(svg.namespaceURI,'path');path.setAttribute('d',ribbon.path);path.setAttribute('fill','#5ba9ca');path.style.pointerEvents='fill';path.dataset.id=ribbon.id;svg.append(path);}
 for(const ribbon of[fan,pump]){
  const center=document.createElementNS(svg.namespaceURI,'path');center.setAttribute('d',ribbon.centerPath);center.style.pointerEvents='none';center.setAttribute('fill','none');svg.append(center);
  const point=center.getPointAtLength(center.getTotalLength()/2),matrix=center.getScreenCTM();
  const hit=document.elementFromPoint(point.x*matrix.a+point.y*matrix.c+matrix.e,point.x*matrix.b+point.y*matrix.d+matrix.f);
  check(hit?.dataset.id===ribbon.id,'native filled ribbon target is hidden by overlapping fan/pump hit areas');
 }
 const selected=focus(geometry,{...accounting,connectors:ribbons},fan.id);check(selected.active&&selected.selectedLinkID===fan.id&&!selected.linkIDs.has(pump.id),'selecting fan relation selected the overlapping pump owner');
 check(paint(associations[2],nodes).colorKey==='driver-internal'&&paint(associations[3],nodes).colorKey==='cooling','association did not retain physical input/service paint');
 const boundaryOrder=['input.solar_incident','input.exterior_convection','input.exterior_longwave','input.people','input.lighting','input.equipment'];
 check(JSON.stringify([...boundaryOrder].reverse().map((kind,index)=>({id:String(index),level:'input',kind})).sort((a,b)=>compare('input',a,b)).map(node=>node.kind))===JSON.stringify(boundaryOrder),'boundary input order is value/label dependent');
 const grouped=groupInputs([nodes[2],{...nodes[2],id:'people.second',value:2,signedValue:2,sourceIds:['people.second']}],[]);check(grouped.nodes.length===1&&grouped.nodes[0].signedValue===42&&grouped.nodes[0].originalNodeIds.length===2,'new People input lost physical-quantity grouping/member identity');
 const equipment=groupInputs([make('electric','input',8,{kind:'input.equipment.electric',signedValue:8}),make('gas','input',3,{kind:'input.equipment.gas',signedValue:-3})],[]);check(equipment.nodes.length===1&&equipment.nodes[0].kind==='input.equipment'&&equipment.nodes[0].signedValue===5&&equipment.nodes[0].value===5&&equipment.nodes[0].groupedMembers.length===2,'native Equipment families duplicated the boundary input or lost signed membership');
 const peopleDriver=make('people.cooling','driver',12,{driverCategory:'internal.people'}),lightDriver=make('lighting.cooling','driver',8,{driverCategory:'internal.lighting'}),heatDriver=make('people.heating','driver',3,{driverCategory:'internal.people'});
 const internalGroup={...peopleDriver,id:'internal.gains',driverCategory:'internal.gains',value:23,groupedMembers:[peopleDriver,lightDriver,heatDriver],originalNodeIds:[peopleDriver.id,lightDriver.id,heatDriver.id]};
 const peopleInput=make('people.input','input',40,{kind:'input.people',signedValue:40}),lightInput=make('lighting.input','input',20,{kind:'input.lighting',signedValue:20});
 const peopleLinks=[link('people.sourceA','input_to_driver',peopleInput.id,peopleDriver.id,20,12),link('people.sourceB','input_to_driver',peopleInput.id,peopleDriver.id,20,12),link('people.heat','input_to_driver',peopleInput.id,heatDriver.id,40,3)];
 const matched=synchronize([peopleInput,lightInput,internalGroup],[{...peopleLinks[0],id:'people.group',toId:internalGroup.id,groupedMembers:peopleLinks},{...link('lighting.source','input_to_driver',lightInput.id,lightDriver.id,20,8),toId:internalGroup.id,groupedMembers:[link('lighting.original','input_to_driver',lightInput.id,lightDriver.id,20,8)]}]).links;
 check(matched[0].fromValue===40&&matched[0].toValue===15&&matched[1].fromValue===20&&matched[1].toValue===8,'People/Lighting associations reused the full Internal gains card or double counted a repeated original driver');
 check(synchronize([peopleInput,internalGroup],[{...matched[0],groupedMembers:[{...peopleLinks[0],toId:'foreign.driver'}]}]).links[0].toValue===null,'foreign original driver acquired the grouped Internal gains quantity');
 check(synchronize([peopleInput,internalGroup],[{...matched[0],groupedMembers:[peopleLinks[0],{...peopleLinks[0],toValue:11}]}]).links[0].toValue===null,'conflicting duplicate target quantity was silently accepted');
 for(const mutate of[link=>({...link,fromValue:null}),link=>({...link,toValue:-1}),link=>({...link,sourceIds:[]}),link=>({...link,period:'M2'}),link=>({...link,fromUnit:'m3'}),link=>({...link,zoneName:'foreign'}),link=>({...link,serviceKind:'heating'}),link=>({...link,relatedPathIds:[]})]){
  const bad=mutate(associations[3]);if(bad.zoneName==='foreign')bad.zoneName='foreign';
  if(bad.zoneName==='foreign'){const scopedNodes=nodes.map(node=>({...node,zoneName:'Office'}));check(context(layout(scopedNodes,canonical),[bad]).length===0,'foreign Zone association rendered');}
  else check(context(geometry,[bad]).length===0,'invalid scope/period/unit/source/auxiliary service evidence rendered');
 }
 const duplicate=[associations[3],{...associations[3],toValue:7}];check(context(geometry,duplicate).length===0,'conflicting duplicate association picked a first valid owner');
 const zero=context(geometry,[{...associations[0],fromValue:0,toValue:0}])[0];check(zero.fromWidth===0&&zero.toWidth===0&&!/NaN|Infinity/.test(zero.path),'reported zero acquired fake reference volume');
 const overflow=context(geometry,[{...associations[3],fromValue:1e308},{...associations[4],fromValue:1e308}]);check(overflow.length===0,'overflowing shared visual reference capacity created nonfinite geometry');
 const monthlyNodes=nodes.map(node=>({...node,period:'M2'})),monthly=associations.map(link=>({...link,period:'M2'}));check(context(layout(monthlyNodes,canonical),monthly,{period:'M2'}).length===5&&context(geometry,monthly,{period:'M2'}).length===0,'Monthly references borrowed Annual node observations');
 check(JSON.stringify({nodes,canonical,associations})===before,'ribbon grouping/layout/focus mutated the source graph');
 document.body.dataset.contextRibbons='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.contextRibbons='failed';document.getElementById('result').textContent=String(error.stack||error);}
</script></body></html>`
