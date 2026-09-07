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

func TestEPATH143PureRibbonGeometryBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser quantitative ribbon verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath143-ribbon-geometry", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath143PureRibbonGeometryHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath143-ribbon-geometry").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH143 pure ribbon browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath143-ribbon-geometry="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH143 pure ribbon failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH143 pure ribbon failed:\n%s", output)
	}
}

const epath143PureRibbonGeometryHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH143 pure ribbons</title></head>
<body data-epath143-ribbon-geometry="pending"><pre id="result">pending</pre><script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const clone=value=>JSON.parse(JSON.stringify(value));
const near=(left,right)=>Math.abs(left-right)<=1e-8*Math.max(Number.MIN_VALUE,Math.abs(left),Math.abs(right));
try {
  const {energyPathLayout:layout}=await import('/src/js/energy-path-layout.js');
  const {energyPathRibbons:ribbons}=await import('/src/js/energy-path-ribbons.js');
  const make=(cooling=null,heating=null,direct=0)=>{
    const nodes=[],links=[];
    for(const [service,pair] of [['cooling',cooling],['heating',heating]]){
      if(!pair)continue;
      const [load,energy]=pair,carrier=service==='cooling'?'electricity':'natural_gas';
      nodes.push(
        {id:'driver.'+service,level:'driver',serviceKind:service,value:load,unit:'kWh thermal',scaleDomain:'thermal',period:'annual',sourceIds:['source.driver.'+service]},
        {id:'load.'+service,level:'load',serviceKind:service,value:load,unit:'kWh thermal',scaleDomain:'thermal',period:'annual'},
        {id:'end_use.'+service,level:'end_use',endUse:service,serviceKind:service,value:energy,unit:'kWh site',scaleDomain:'site',period:'annual'},
        {id:'carrier.'+carrier,level:'carrier',carrier,value:energy,unit:'kWh',scaleDomain:'site',period:'annual'}
      );
      links.push(
        {id:'driver.'+service,relation:'driver_to_load',fromId:'driver.'+service,toId:'load.'+service,fromValue:load,toValue:load,fromUnit:'kWh thermal',toUnit:'kWh thermal',serviceKind:service,period:'annual'},
        {id:'conversion.'+service,relation:'load_to_end_use',fromId:'load.'+service,toId:'end_use.'+service,fromValue:load,toValue:energy,fromUnit:'kWh thermal',toUnit:'kWh site',serviceKind:service,period:'annual',ratio:load/energy,ratioKind:service==='cooling'?'coefficient_of_performance':'efficiency',sourceIds:['source.load.'+service,'source.energy.'+service]},
        {id:'carrier.'+service,relation:'end_use_to_carrier',fromId:'end_use.'+service,toId:'carrier.'+carrier,fromValue:energy,toValue:energy,fromUnit:'kWh site',toUnit:'kWh',serviceKind:service,period:'annual'}
      );
    }
    if(direct){nodes.push({id:'end_use.equipment',level:'end_use',endUse:'equipment',value:direct,unit:'kWh',scaleDomain:'site',period:'annual'},{id:'carrier.propane',level:'carrier',carrier:'propane',value:direct,unit:'kWh',scaleDomain:'site',period:'annual'});links.push({id:'direct.equipment',relation:'direct_end_use_to_carrier',fromId:'end_use.equipment',toId:'carrier.propane',fromValue:direct,toValue:direct,fromUnit:'kWh',toUnit:'kWh',period:'annual'});}
    return {nodes,links};
  };
  const compute=(graph,options={})=>ribbons(layout(graph.nodes,graph.links),options);
  const find=(result,id)=>result.ribbons.find(item=>item.id===id);
  const checkGeometry=(graph,result)=>{
    const slots=new Map(layout(graph.nodes,graph.links).nodes.map(node=>[node.id,node]));
    for(const scale of Object.values(result.scales))check(Object.values(scale).every(value=>value===null||Number.isFinite(value))&&scale.pixelsPerKWh<=scale.fitPixelsPerKWh*(1+1e-12),'nonfinite metadata or enlarged domain fit');
    for(const bar of result.bars){const slot=slots.get(bar.nodeId);check(bar.node===graph.nodes.find(node=>node.id===bar.nodeId),'bar lost original node reference');check(near(bar.height,bar.value*result.scales[bar.domain].pixelsPerKWh),'bar used porttotal/rawvalue instead of reportednode value');check(bar.height>=0&&bar.y>=slot.y-1e-8&&bar.y+bar.height<=slot.y+slot.height+1e-8,'reportedbar escaped reservedslot');check(bar.side==='incoming'?bar.x+bar.width<=slot.x+1e-8:bar.x>=slot.x+slot.width-1e-8,'quantitativebar hidden behind opaque labelcard');}
    for(const extent of result.portExtents){const slot=slots.get(extent.nodeId);check(Number.isFinite(extent.height)&&extent.y>=slot.y-1e-8&&extent.y+extent.height<=slot.y+slot.height+1e-8,'portsum escaped slot or overflowed');}
    for(const item of result.ribbons){check(item.link===graph.links.find(link=>link.id===item.id),'ribbon lost original link reference');check(!/NaN|Infinity/.test(item.path)&&item.path.endsWith(' Z'),'invalid closed SVG ribbon');check(near(item.fromPort.y1-item.fromPort.y0,item.fromWidth)&&near(item.toPort.y1-item.toPort.y0,item.toWidth),'path port span diverged from pairedwidth');check(item.fromPort.x<item.toPort.x&&item.gap.width===item.toPort.x-item.fromPort.x,'ribbon reversed or lost exterior gap');check(near(item.fromWidth,item.fromValue*result.scales[item.domain==='conversion'?'thermal':item.domain].pixelsPerKWh)&&near(item.toWidth,item.toValue*result.scales[item.domain==='conversion'?'site':item.domain].pixelsPerKWh),'ribbon invented a minimumwidth or mixed same-domain scales');}
  };
  for(const graph of [make([400,100]),make(null,[85,100]),make([400,100],[85,100]),make([400,100],[85,100],1e9)]){
    const before=JSON.stringify(graph),result=compute(graph);
    const cool=find(result,'conversion.cooling'),heat=find(result,'conversion.heating');
    if(cool)check(cool.fromWidth>cool.toWidth&&cool.fromValue===400&&cool.toValue===100&&cool.link.ratio===4,'COP4 did not narrow or rawpairedvalues changed');
    if(heat)check(heat.fromWidth<heat.toWidth&&heat.fromValue===85&&heat.toValue===100&&heat.link.ratio===.85,'boiler efficiency did not widen or reportedratio changed');
    checkGeometry(graph,result);check(JSON.stringify(graph)===before,'ribbon derivation mutated canonical values/provenance');
  }
  const equal=make([100,100],[85,100]);
  let result=compute(equal);
  check(near(result.scales.thermal.pixelsPerKWh,result.scales.site.pixelsPerKWh)&&near(find(result,'conversion.cooling').fromWidth,find(result,'conversion.cooling').toWidth),'exactratio1 acquired fake taper');
  const partial=make([100,25]);partial.links[1].fromValue=40;partial.links[1].toValue=20;partial.links[1].ratio=2;
  result=compute(partial);
  check(find(result,'conversion.cooling').fromValue===40&&find(result,'conversion.cooling').toValue===20,'partial paired values stretched to annual totals');
  check(result.bars.find(bar=>bar.nodeId==='load.cooling'&&bar.side==='outgoing').height>find(result,'conversion.cooling').fromWidth,'unpairedload barportion was erased');
  check(result.bars.find(bar=>bar.nodeId==='end_use.cooling'&&bar.side==='incoming').height>find(result,'conversion.cooling').toWidth,'unpairedsite barportion was erased');
  const other=make([7,1.75]);other.nodes[0].rawValue=0;other.nodes[0].effectiveValue=0;other.nodes[0].allocatedValue=7;other.nodes[0].allocationApplied=true;
  check(compute(other).bars.find(bar=>bar.nodeId==='driver.cooling').value===7,'explicit Other/storage rawzero displaced canonicalallocatedValue');
  const merged=make([100,25],[40,50]);const originals=merged.nodes.filter(node=>node.level==='driver');
  merged.nodes=merged.nodes.filter(node=>node.level!=='driver');merged.nodes.unshift({...originals[0],id:'driver.all',serviceKind:'all',value:140,groupedMembers:originals});merged.links.filter(link=>link.relation==='driver_to_load').forEach(link=>link.fromId='driver.all');
  result=compute(merged);check(result.ribbons.filter(item=>item.relation==='driver_to_load').length===2,'proven all-service driver members lost cooling/heating ribbons');
  merged.nodes[0].groupedMembers=[];check(compute(merged).ribbons.every(item=>item.relation!=='driver_to_load'),'wordall without originalmember evidence faked servicecoverage');
  const over=make(null,null,70);over.nodes[1].value=100;over.nodes.push({...over.nodes[0],id:'end_use.lighting',endUse:'lighting',value:40});over.links.push({...over.links[0],id:'direct.lighting',fromId:'end_use.lighting',fromValue:40,toValue:40});
  result=compute(over);check(result.scales.site.maxValue===100&&result.scales.site.capacityMaxValue===110,'reported domainmax conflated with overmappedportcapacity');
  const reported=result.bars.find(bar=>bar.nodeId==='carrier.propane'),extent=result.portExtents.find(item=>item.nodeId==='carrier.propane');
  check(reported.value===100&&extent.totalValue===110&&extent.overmapped&&extent.height>reported.height,'overmappedincoming inflatedreportedbar or hid excess');checkGeometry(over,result);
  const incoming=result.ribbons.filter(item=>item.toId==='carrier.propane').sort((a,b)=>a.toPort.y0-b.toPort.y0);
  check(incoming.length===2&&near(incoming[0].toPort.y1,incoming[1].toPort.y0),'same-side ports overlapped or didnotaccumulate');
  const tiny=make(null,null,1e-7);tiny.links[0].toValue=9e-7;
  check(compute(tiny).ribbons.length===0,'absolute1 tolerance accepted9times unequal tiny-domain values');
  tiny.links[0].toValue=1e-7;result=compute(tiny);check(result.ribbons.length===1&&result.ribbons[0].fromWidth===result.ribbons[0].toWidth,'exact tiny-domain equality lost linear same-domain ribbon');
  const zero=make([100,25]);zero.nodes[0].value=0;zero.links[0].fromValue=0;zero.links[0].toValue=0;
  result=compute(zero);check(!find(result,'driver.cooling')&&result.bars.filter(bar=>bar.nodeId==='driver.cooling').every(bar=>bar.height===0),'zero energy acquired artificial visiblewidth');
  for(const mutate of [
    graph=>{graph.links[1].fromValue=null;},graph=>{graph.links[1].toValue=false;},graph=>{graph.links[1].toValue='';},graph=>{graph.links[1].toValue=-1;},graph=>{graph.links[1].toValue=Infinity;},
    graph=>{graph.links[1].fromUnit='';},graph=>{graph.links[1].toUnit='kWh thermal';},graph=>{graph.nodes[2].scaleDomain='thermal';},graph=>{graph.nodes[2].unit='m3';},
    graph=>{graph.links[1].serviceKind='heating';},graph=>{graph.nodes[2].endUse='fans_pumps';},graph=>{graph.links[1].sourceIds=[];},graph=>{graph.nodes[1].zoneName='Office';},graph=>{graph.links[1].zoneName='Other';graph.nodes[1].zoneName='Office';graph.nodes[2].zoneName='Office';},
    graph=>{graph.links[1].period='M1';},
  ]){const graph=make([400,100]);mutate(graph);check(!find(compute(graph),'conversion.cooling'),'invalidconversion type/period/zone/value/source evidence rendered');}
  const monthly=make([12,3]);monthly.nodes.forEach(node=>node.period='M2');monthly.links.forEach(link=>link.period='M2');
  check(find(compute(monthly,{period:'M2'}),'conversion.cooling')?.fromValue===12,'valid monthlypaired values disappeared');
  check(compute(monthly,{period:'M1'}).ribbons.length===0,'differentmonth reused selectedperiod data');
  monthly.nodes.forEach(node=>delete node.period);monthly.links.forEach(link=>delete link.period);
  check(compute(monthly,{period:'M2'}).ribbons.length===0,'unqualified annual observations were reinterpreted asmonthly');
  const duplicate=make([400,100]);duplicate.links.push(clone(duplicate.links[1]));
  check(compute(duplicate).ribbons.filter(item=>item.id==='conversion.cooling').length===1,'identicalduplicate link doubledportenergy');
  for(const modify of [link=>{link.toValue=0;},link=>{link.toValue=101;},link=>{link.fromUnit='m3';},link=>{link.period='M2';},link=>{link.serviceKind='heating';}]){const graph=clone(duplicate);modify(graph.links[3]);result=compute(graph);check(!find(result,'conversion.cooling')&&result.excluded.some(item=>item.id==='conversion.cooling'&&item.reason==='conflicting_duplicate'),'badduplicate variant allowed firstvalidsameID towin');}
  const overflow=make(null,null,1e308);overflow.nodes.push({...overflow.nodes[0],id:'end_use.other',endUse:'other'});overflow.links.push({...overflow.links[0],id:'direct.other',fromId:'end_use.other'});
  result=compute(overflow);check(result.ribbons.length===0&&result.excluded.filter(item=>item.reason==='aggregate_overflow').length===2,'finitepositiveSUM overflow didnotexplicitlyexclude affectedribbons');checkGeometry(overflow,result);
  const dense=make([120,30],[52,65]);dense.nodes=dense.nodes.filter(node=>node.level!=='driver');dense.links=dense.links.filter(link=>link.relation!=='driver_to_load');
  for(let index=0;index<12;index++){const value=index===0?65:107/11;const node={id:'driver.dense.'+index,level:'driver',serviceKind:'all',value,unit:'kWh',scaleDomain:'thermal',period:'annual',groupedMembers:[{level:'driver',serviceKind:'cooling',unit:'kWh',scaleDomain:'thermal'},{level:'driver',serviceKind:'heating',unit:'kWh',scaleDomain:'thermal'}]};dense.nodes.push(node);for(const service of ['cooling','heating'])dense.links.push({id:node.id+'.'+service,relation:'driver_to_load',fromId:node.id,toId:'load.'+service,serviceKind:service,fromValue:value/2,toValue:value/2,fromUnit:'kWh',toUnit:'kWh',period:'annual'});}
  for(const [index,endUse] of ['fans_pumps','hvac_auxiliaries','lighting','equipment','water_systems','refrigeration','other','storage_charge','additional'].entries()){dense.nodes.push({id:'direct.'+index,level:'end_use',endUse,value:5,unit:'kWh',scaleDomain:'site',period:'annual'});dense.links.push({id:'direct-link.'+index,relation:'direct_end_use_to_carrier',fromId:'direct.'+index,toId:'carrier.electricity',fromValue:5,toValue:5,fromUnit:'kWh',toUnit:'kWh',period:'annual'});}
  result=compute(dense);checkGeometry(dense,result);check(result.ribbons.filter(item=>item.relation==='driver_to_load').length===24,'dense split-driver ports disappeared');
  const bySide=new Map();for(const item of result.ribbons)for(const [side,port] of [['outgoing',item.fromPort],['incoming',item.toPort]]){const key=(side==='outgoing'?item.fromId:item.toId)+'|'+side;bySide.set(key,[...(bySide.get(key)||[]),port]);}
  for(const ports of bySide.values()){ports.sort((a,b)=>a.y0-b.y0);check(ports.every((port,index)=>!index||port.y0>=ports[index-1].y1-1e-8),'densecumulativeports overlapped');}
  const residual=make(null,null,10);residual.nodes[0]={...residual.nodes[0],level:'residual',presentationLevel:'end_use',presentationKind:'unclassified_energy',basis:'residual',signedValue:10};residual.links[0].relation='residual';residual.links[0].basis='residual';
  check(compute(residual).ribbons.length===1,'positivevetted unclassifiedresidual lostsite ribbon');residual.nodes[0].signedValue=-10;check(compute(residual).ribbons.length===0,'negativeovermapped residual became positiveflow');
  const empty=ribbons(layout([],[]));check(empty.ribbons.length===0&&empty.bars.length===0&&empty.scales.thermal.maxValue===0&&empty.scales.site.kWhPerPixel===null,'emptygraph invented quantitativeenergy');
  if(failures.length)throw new Error(failures.join(' | '));
  document.body.dataset.epath143RibbonGeometry='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.epath143RibbonGeometry='failed';document.getElementById('result').textContent=String(error?.stack||error);}
</script></body></html>`
