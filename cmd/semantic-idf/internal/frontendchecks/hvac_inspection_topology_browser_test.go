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

func TestHVACInspectionTopologyBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless HVAC inspection topology acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/topology", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, hvacInspectionTopologyHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/topology").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-inspection-topology-status="passed"`) {
		if _, detail, ok := strings.Cut(string(output), `<pre id="result">`); ok {
			message, _, _ := strings.Cut(detail, "</pre>")
			if message != "" {
				t.Fatalf("HVAC inspection topology acceptance failed: %v\n%s", err, message)
			}
		}
		t.Fatalf("HVAC inspection topology acceptance failed: %v\n%s", err, output)
	}
}

// Shared geometric acceptance uses rendered SVG paths and boxes, rather than
// repeating the placement algorithm's obstacle list in its assertions.
const hvacInspectionBoxGeometryHTML = `
const inspectionPathPoint=(path,length)=>{const point=path.getPointAtLength(length);return new DOMPoint(point.x,point.y).matrixTransform(path.getScreenCTM());};
const inspectionPathSamples=path=>{const length=path.getTotalLength(),count=Math.max(1,Math.ceil(length/3));return Array.from({length:count+1},(_,index)=>inspectionPathPoint(path,length*index/count));};
const assertHVACInfoBoxes=(root,name)=>{
 const svg=root.querySelector('svg.hvac-loop-svg');check(svg,name+' has no native loop SVG');
 const bounds=svg.getBoundingClientRect(),cards=[...svg.querySelectorAll('.hvac-inspect-vertex')].filter(vertex=>vertex.querySelector('.hvac-inspect-annotation')).map(vertex=>({vertex,box:vertex.querySelector('.hvac-inspect-info-box'),annotation:vertex.querySelector('.hvac-inspect-annotation'),leader:vertex.querySelector('.hvac-inspect-leader')}));
 const pipes=[...svg.querySelectorAll('.hvac-loop-path,.hvac-loop-branch-path,.hvac-loop-connector')].filter(path=>typeof path.getTotalLength==='function'&&(path.tagName.toLowerCase()!=='path'||path.getAttribute('d')?.trim())).map(path=>({points:inspectionPathSamples(path),radius:parseFloat(getComputedStyle(path).strokeWidth)/2}));
 const obstacles=[...svg.querySelectorAll('.hvac-loop-icon,.hvac-inspect-node-ring,.hvac-loop-equipment .mini-label,.hvac-loop-cross-indicator circle,.hvac-branch-badge,.hvac-loop-side-note,.hvac-loop-label,.hvac-loop-name,.hvac-loop-endpoint text')].map(item=>({name:item.textContent||item.getAttribute('class')||item.closest('.hvac-loop-cross-indicator')?.getAttribute('class'),rect:item.getBoundingClientRect()})).filter(item=>item.rect.width&&item.rect.height);
 const overlaps=(a,b)=>Math.min(a.right,b.right)-Math.max(a.left,b.left)>1&&Math.min(a.bottom,b.bottom)-Math.max(a.top,b.top)>1;
 const inside=(point,rect,padding=0)=>point.x>rect.left-padding+.1&&point.x<rect.right+padding-.1&&point.y>rect.top-padding+.1&&point.y<rect.bottom+padding-.1;
 for(const card of cards){
  const title=name+' / '+card.vertex.dataset.hvacInspectPointName;
  check(card.box?.tagName.toLowerCase()==='rect'&&card.box.rx.baseVal.value>0,title+' lacks a rounded information box');
  const rect=card.box.getBoundingClientRect();check(rect.width>0&&rect.height>0,title+' information box is empty');
  check(rect.left>=bounds.left-1&&rect.right<=bounds.right+1&&rect.top>=bounds.top-1&&rect.bottom<=bounds.bottom+1,title+' information box is clipped');
  for(const text of card.annotation.querySelectorAll('text')){const content=text.getBoundingClientRect();check(content.left>=rect.left-1&&content.right<=rect.right+1&&content.top>=rect.top-1&&content.bottom<=rect.bottom+1,title+' clips its actual text: '+text.textContent);}
  for(const pipe of pipes)check(!pipe.points.some(point=>inside(point,rect,pipe.radius)),title+' covers a physical pipe segment');
  for(const obstacle of obstacles)check(!overlaps(rect,obstacle.rect),title+' overlaps a physical node, equipment or schematic label: '+obstacle.name);
  check(card.leader&&card.leader.getTotalLength()>0,title+' has no source leader');
  check(getComputedStyle(card.leader).pointerEvents==='stroke',title+' leader cannot participate in its source hover/selection hit target');
  const first=inspectionPathPoint(card.leader,0),last=inspectionPathPoint(card.leader,card.leader.getTotalLength());
  const boxMatrix=card.box.getScreenCTM(),rx=card.box.rx.baseVal.value*Math.hypot(boxMatrix.a,boxMatrix.b),ry=(card.box.ry.baseVal.value||card.box.rx.baseVal.value)*Math.hypot(boxMatrix.c,boxMatrix.d);
  const boxEdge=point=>{if(point.x<rect.left-1||point.x>rect.right+1||point.y<rect.top-1||point.y>rect.bottom+1)return false;const x=point.x<rect.left+rx?rect.left+rx:point.x>rect.right-rx?rect.right-rx:null,y=point.y<rect.top+ry?rect.top+ry:point.y>rect.bottom-ry?rect.bottom-ry:null;return x!==null&&y!==null?Math.abs(Math.hypot((point.x-x)/rx,(point.y-y)/ry)-1)*Math.max(rx,ry)<1:Math.min(Math.abs(point.x-rect.left),Math.abs(point.x-rect.right),Math.abs(point.y-rect.top),Math.abs(point.y-rect.bottom))<1;};
  check(boxEdge(first),title+' leader does not start on the information-box boundary: '+JSON.stringify({first,rect:{left:rect.left,top:rect.top,right:rect.right,bottom:rect.bottom},rx,ry}));
  const ring=card.vertex.querySelector('.hvac-inspect-node-ring');
  if(ring){const center=new DOMPoint(ring.cx.baseVal.value,ring.cy.baseVal.value).matrixTransform(ring.getScreenCTM()),matrix=ring.getScreenCTM(),radius=ring.r.baseVal.value*Math.hypot(matrix.a,matrix.b);check(ring.r.baseVal.value===11,title+' changes node geometry with activity');check(Math.abs(Math.hypot(last.x-center.x,last.y-center.y)-radius)<1,title+' leader does not end on its own node-circle boundary');}
  const leaderPoints=inspectionPathSamples(card.leader).slice(1,-1);
  check(!leaderPoints.some(point=>inside(point,rect)),title+' leader doubles back through its own information box');
  for(const other of cards)if(other!==card){const otherRect=other.box?.getBoundingClientRect();if(otherRect){check(!overlaps(rect,otherRect),title+' overlaps another information box');check(!leaderPoints.some(point=>inside(point,otherRect)),title+' leader crosses another information box');}}
 }
 return cards;
};
`

const hvacInspectionTopologyHTML = `<!doctype html><html data-theme="dark"><head><link rel="stylesheet" href="/src/styles.css"><link rel="stylesheet" href="/src/styles/hvac-inspection-topology.css"></head><body style="display:block;overflow:auto"><div id="mount"></div><pre id="result"></pre>
<script type="module">
const check=(value,message)=>{if(!value)throw new Error(message);};
` + hvacInspectionBoxGeometryHTML + `
try {
 const {buildHVACInspectionTopology:build,buildHVACInspectionLayout:inspectionLayout,renderHVACInspectionTopology:render,updateHVACInspectionTopology:update}=await import('/src/js/views/hvac-inspection-topology.js');
 const {getLanguage,setLanguage}=await import('/src/js/i18n.js');
 const {renderHVACLoopDiagram,buildHVACLoopDiagramLayout:baseLayout}=await import('/src/js/views/hvac-views.js');
 const {renderHVACInspection:renderInspection}=await import('/src/js/views/hvac-inspection-view.js');
 const {prepareHVACInspection:prepare,hvacInspectionSnapshots:snapshots}=await import('/src/js/hvac-inspection-data.js');
 const equipment=(name,inlet,outlet,type='Pump:VariableSpeed')=>({objectType:type,objectName:name,inletNode:inlet,outletNode:outlet,exists:true});
 const loop={id:'plant-1',name:'Chilled water',type:'PlantLoop',supplySide:{inletNode:'Supply inlet',outletNode:'Supply outlet',branches:[
 {name:'Inlet branch',components:[equipment('Supply pump','Supply inlet','Splitter inlet')]},
 {name:'Cooling A',components:[equipment('Chiller A','Chiller A inlet','Chiller A outlet','Chiller:Electric:EIR')]},
 {name:'Cooling B',components:[equipment('Chiller B','Chiller B inlet','Chiller B outlet','Chiller:Electric:EIR')]},
 {name:'Outlet branch',components:[equipment('Outlet pipe','Mixer outlet','Supply outlet','Pipe:Adiabatic')]},
 ],connectors:[{type:'Connector:Splitter',inletBranchName:'Inlet branch',branchNames:['Cooling A','Cooling B']},{type:'Connector:Mixer',outletBranchName:'Outlet branch',branchNames:['Cooling A','Cooling B']}]},demandSide:{}};
 const original=JSON.stringify(loop),graph=build(loop),edge=(from,to)=>graph.edges.some(item=>item.from===from&&item.to===to);
 check(edge('node:splitter inlet','node:chiller a inlet')&&edge('node:splitter inlet','node:chiller b inlet'),'splitter did not branch into both actual branches');
 check(edge('node:chiller a outlet','node:mixer outlet')&&edge('node:chiller b outlet','node:mixer outlet'),'mixer did not join both actual outlets');
 check(!edge('node:chiller a outlet','node:chiller b inlet'),'parallel chillers gained a false serial connection');
 const disconnected=build({supplySide:{branches:[{name:'a',components:[equipment('A','a1','a2')]},{name:'b',components:[equipment('B','b1','b2')]}]}});
 check(disconnected.edges.length===4&&!disconnected.edges.some(item=>item.from==='node:a2'&&item.to==='node:b1'),'branch list order invented connectivity');
 const repeated=build({supplySide:{branches:[{components:[equipment('Two-circuit device','a1','a2')]},{components:[equipment('Two-circuit device','b1','b2')]}]}},{components:[{id:'shared-device',name:'Two-circuit device',type:'Pump:VariableSpeed',metrics:[]}]});
 check(repeated.vertices.filter(item=>item.kind==='component').length===2,'two known circuit occurrences gained a third duplicate observed device');
 const demand=build({demandGraph:{supplyPath:{components:[{objectType:'AirLoopHVAC:ZoneSplitter',objectName:'Air splitter',inletNodes:['Air inlet'],outletNodes:['Zone A inlet','Zone B inlet']}]},edges:[{objectType:'AirLoopHVAC:ZoneSplitter',objectName:'Air splitter',fromNode:'Air inlet',toNode:'Zone A inlet'},{fromNode:'Zone A inlet',toNode:'Zone A outlet',role:'terminal'}]}});
 check(demand.edges.length===4&&demand.vertices.some(item=>item.kind==='component'&&item.name==='Air splitter'),'demand graph dropped explicit components/edges or duplicated component flow');
 const wrapper={id:'unitary-loop',supplySide:{branches:[{name:'Main branch',components:[equipment('Unitary system','Unit inlet','Unit outlet','AirLoopHVAC:UnitarySystem')]}]}};
 const children=[{id:'fan-child',name:'Nested fan',type:'Fan:SystemModel',parentComponentName:'Unitary system',parentComponentType:'AirLoopHVAC:UnitarySystem',nodePorts:[{nodeName:'Unit inlet',role:'air_inlet'},{nodeName:'Coil inlet',role:'air_outlet'}],metrics:[{id:'power',label:'Power',value:200,unit:'W'}]},
 {id:'coil-child',name:'Nested coil',type:'Coil:Cooling:DX:SingleSpeed',parentComponentName:'Unitary system',parentComponentType:'AirLoopHVAC:UnitarySystem',inletNodes:['Coil inlet'],outletNodes:['Unit outlet'],metrics:[{id:'cop',label:'COP',value:3,unit:''}]},
 {id:'unknown-device',name:'Unmapped device',type:'Boiler:HotWater',nodePorts:[{nodeName:'Unit inlet',role:'water_inlet'},{nodeName:'Unmapped device outlet',role:'water_outlet'}],metrics:[{id:'power',label:'Power',value:100,unit:'W'}]},
 {id:'wrong-type',name:'Unitary system',type:'Boiler:HotWater',metrics:[{id:'power',label:'Power',value:50,unit:'W'}]}];
 const extraNodes=[{id:'internal-point',name:'Coil inlet',metrics:[{id:'temperature',label:'Temperature',value:20,unit:'°C'}]},{id:'detached-point',name:'Unmapped point',metrics:[{id:'flow',label:'Flow',value:0,unit:'kg/s'}]}];
 const expanded=build(wrapper,{nodes:extraNodes,components:children});
 check(expanded.edges.some(item=>item.from==='observed:fan-child'&&item.to==='node:coil inlet'&&item.role==='air_outlet')&&expanded.edges.some(item=>item.from==='node:coil inlet'&&item.to==='observed:coil-child'),'typed inner fan/coil ports lost their real connections or medium role');
 check(!expanded.edges.some(item=>item.from==='component:supply:0:0'||item.to==='component:supply:0:0'),'expanded full internal path left a false wrapper bypass');
 check(!expanded.vertices.some(item=>['observed:unknown-device','observed:wrong-type','node:unmapped point','node:unmapped device outlet'].includes(item.id)),'unrelated observations or a same-name wrong-type component became selected-loop members');
 const ambiguousParent=build({supplySide:{branches:[{components:[equipment('Unitary system','Unit inlet','Unit outlet','AirLoopHVAC:UnitarySystem')]},{components:[equipment('Unitary system','Other inlet','Other outlet','AirLoopHVAC:UnitarySystem')]}]}},{components:[children[0]]});
 check(!ambiguousParent.vertices.some(item=>item.id==='observed:fan-child'),'ambiguous wrapper occurrence established child ownership');
 const partial=build(wrapper,{components:[children[0]]});
 check(partial.edges.some(item=>item.from==='node:unit inlet'&&item.to==='component:supply:0:0'),'incomplete child observations erased the actual wrapper topology');
 const nodes=[{id:'inlet-point',name:'SUPPLY INLET',metrics:[{id:'flow',label:'Flow',value:0,unit:'kg/s'},{id:'temperature',label:'Temperature',value:7,unit:'°C'},{id:'humidity',label:'Humidity',value:.0047,unit:'kg/kg'},{id:'setpoint',label:'Setpoint',value:6,unit:'°C'}]},{id:'outlet-point',name:'Supply outlet',metrics:[{id:'flow',label:'Flow',value:2,unit:'kg/s'}]}];
 const components=[{id:'pump-observation',name:'Supply pump',type:'Pump:VariableSpeed',status:'off',metrics:[{id:'power',label:'Power',value:0,unit:'W'}]},{id:'chiller-a-observation',name:'Chiller A',type:'Chiller:Electric:EIR',status:'on',metrics:[{id:'power',label:'Power',value:14000,unit:'W'},{id:'cop',label:'COP',value:4.2,unit:''}]}];
 const mount=document.getElementById('mount');mount.innerHTML=render({loop,nodes,components,selectedNode:'inlet-point'});
 const reference=document.createElement('div');reference.innerHTML=renderHVACLoopDiagram(loop);
 check(mount.querySelector('svg.hvac-loop-svg')&&mount.querySelectorAll('.hvac-loop-side-block').length===2,'inspection must reuse the existing supply/demand loop schematic');
 check(!mount.querySelector('.hvac-inspect-card'),'frame values still replace schematic points with separate cards');
 const equipmentKeys=root=>[...root.querySelectorAll('.hvac-loop-equipment')].map(item=>item.querySelector('title')?.textContent+':'+(Number(item.querySelector('.pipe-port.right').getAttribute('cx'))-Number(item.querySelector('.pipe-port.left').getAttribute('cx')))).join('|');
 check(equipmentKeys(mount)===equipmentKeys(reference),'frame display changed the existing loop equipment ordering or symbols');
 const diagram=mount.querySelector('svg.hvac-loop-svg'),referenceDiagram=reference.querySelector('svg.hvac-loop-svg');
 check(diagram.viewBox.baseVal.width===1480&&referenceDiagram.viewBox.baseVal.width===1120,'inspection did not widen or changed the default HVAC tab width');
 check(Math.abs(diagram.getBoundingClientRect().width-1480)<1,'wider inspection scaled the whole SVG instead of preserving native text/icon size');
 const integrated=inspectionLayout({loop,nodes,components}),nativeBase=baseLayout(loop);
 check(integrated.layout.leftX>nativeBase.leftX&&integrated.layout.width-integrated.layout.rightX>nativeBase.width-nativeBase.rightX,'inspection did not reserve dedicated outboard information gutters');
 const boundaryRecords=integrated.records.filter(record=>record.kind==='node'&&record.anchor.ownerKind==='loop-side');
 check(boundaryRecords.length===2&&boundaryRecords.every(record=>record.anchor.x===integrated.layout.leftX?record.x+record.width<=record.anchor.x-11:record.x>=record.anchor.x+11),'boundary information boxes do not occupy their own outboard gutter beyond the physical node/vertical pipe');
 for(const anchor of integrated.layout.anchors.filter(anchor=>anchor.kind==='component'&&anchor.area==='branch')){
  const block=mount.querySelector('.hvac-loop-side-block.'+anchor.side),panel=block.querySelector('.hvac-loop-side-panel').getBoundingClientRect(),native=[...block.querySelectorAll('.hvac-loop-equipment')].find(item=>item.querySelector('title')?.textContent.split(' - ')[0]===anchor.name);
  check(native,'source-backed branch equipment was lost from its native panel: '+anchor.name);
  for(const symbol of native.querySelectorAll('.hvac-loop-icon,.pipe-port')){const rect=symbol.getBoundingClientRect();check(rect.left>=panel.left-1&&rect.right<=panel.right+1&&rect.top>=panel.top-1&&rect.bottom<=panel.bottom+1,'branch panel no longer contains the native equipment icon/ports: '+anchor.name);}
 }
 const checkAnnotationSpacing=()=>{
  const annotations=[...mount.querySelectorAll('.hvac-inspect-annotation')].map(item=>({item,rect:item.getBoundingClientRect()}));
  for(let i=0;i<annotations.length;i++)for(let j=i+1;j<annotations.length;j++){
   const a=annotations[i].rect,b=annotations[j].rect;
   check(Math.min(a.right,b.right)-Math.max(a.left,b.left)<=1||Math.min(a.bottom,b.bottom)-Math.max(a.top,b.top)<=1,'frame text overlaps: '+annotations[i].item.textContent+' / '+annotations[j].item.textContent);
  }
 };
 check(mount.querySelectorAll('.hvac-inspect-annotation').length>=nodes.length+components.length,'frame measurements must occupy annotation space on the original diagram');
 checkAnnotationSpacing();
 assertHVACInfoBoxes(mount,'parallel chiller information boxes');
 const inlet=mount.querySelector('[data-hvac-inspect-node="inlet-point"]');
 check(inlet?.classList.contains('measured')&&inlet.classList.contains('selected'),'frame point did not match case-insensitive physical node or emphasize selected point');
 const linkedStroke=vertex=>[vertex.querySelector('.hvac-inspect-info-box'),vertex.querySelector('.hvac-inspect-leader'),vertex.querySelector('.hvac-inspect-node-ring')].map(item=>getComputedStyle(item).stroke);
 check(new Set(linkedStroke(inlet)).size===1&&linkedStroke(inlet)[0]!=='none','selection does not emphasize the owning information box, leader and node together');
 const focusPoint=mount.querySelector('[data-hvac-inspect-node="outlet-point"]');focusPoint.focus();
 check(focusPoint.matches(':focus-visible')&&new Set(linkedStroke(focusPoint)).size===1,'keyboard focus does not connect the information box, leader and node');focusPoint.blur();
 check(mount.querySelectorAll('.node.measured').length===2,'all observed node points were not highlighted');
 check(inlet.querySelectorAll('[data-hvac-inspect-metric]').length===3&&inlet.textContent.includes('0.00 kg/s')&&inlet.textContent.includes('0.0047 kg/kg'),'zero flow/humidity precision disappeared or setpoint retained a separate row');
 const compactLabel=id=>inlet.querySelector('[data-hvac-inspect-metric="'+id+'"] .hvac-inspect-metric-label').textContent.trim();
 check(compactLabel('flow')==='ṁ'&&compactLabel('temperature')==='T'&&compactLabel('humidity')==='w','node measurements lost compact physical symbols or confused humidity ratio with relative humidity');
 check(!mount.querySelector('.node .hvac-inspect-point-label')&&inlet.querySelector('title').textContent.startsWith('SUPPLY INLET')&&inlet.getAttribute('aria-label').startsWith('SUPPLY INLET'),'node names must remain in tooltips/accessibility labels only');
 check(mount.querySelector('.equipment .hvac-inspect-point-label')?.textContent==='Supply pump','hiding node names also removed equipment names');
 const valueFont=parseFloat(getComputedStyle(inlet.querySelector('.hvac-inspect-metric-value')).fontSize),labelFont=parseFloat(getComputedStyle(inlet.querySelector('.hvac-inspect-metric-label')).fontSize);
 check(valueFont>labelFont&&valueFont<=labelFont+1,'frame values must stay compact while slightly emphasizing measured values');
 for(const row of mount.querySelectorAll('[data-hvac-inspect-metric]')){
  const symbol=row.querySelector('.hvac-inspect-metric-label'),value=row.querySelector('.hvac-inspect-metric-value');
  check(symbol.getAttribute('x')==='0'&&value.getAttribute('x')==='32','frame measurement fixed column attributes changed');
  check(Math.abs(symbol.getStartPositionOfChar(0).x)<.001&&Math.abs(value.getStartPositionOfChar(0).x-32)<.001,'frame measurement labels and values do not align in fixed columns: '+row.textContent+'; x='+symbol.getStartPositionOfChar(0).x+'/'+value.getStartPositionOfChar(0).x);
  check(symbol.getBBox().x+symbol.getBBox().width<value.getBBox().x,'frame measurement label overlaps its value');
  check(Math.abs(value.getStartPositionOfChar(0).y-Number(row.getAttribute('y')))<.1,'a subscript shifted the measured value off its row');
 }
 const comparison=inlet.querySelector('[data-hvac-setpoint-state="unmet"]');
 check(!inlet.querySelector('[data-hvac-inspect-metric="setpoint"]')&&comparison?.textContent==='7.00 >set 6.00'&&comparison.querySelector('[baseline-shift="sub"]')?.textContent==='set','optional setpoint must be inline with temperature and reflect cooling inequality');
 check(getComputedStyle(comparison).fontWeight==='700'&&getComputedStyle(comparison).fill==='rgb(255, 123, 114)','unmet setpoint comparison is not bold red');
 const unrelatedBoiler={id:'unowned-on-boiler',name:'Unrelated active boiler',type:'Boiler:HotWater',status:'on',nodePorts:[{nodeName:'SUPPLY INLET',role:'water_outlet'}],metrics:[{id:'heating',label:'Heating rate',value:100,unit:'W'}]};
 mount.innerHTML=render({loop,nodes,components:[...components,unrelatedBoiler],selectedNode:'inlet-point'});
 const preservedControl=mount.querySelector('[data-hvac-inspect-node="inlet-point"] [data-hvac-setpoint-state]');
 check(!mount.querySelector('[data-hvac-inspect-component="unowned-on-boiler"]')&&preservedControl?.dataset.hvacSetpointState==='unmet'&&getComputedStyle(preservedControl).fill==='rgb(255, 123, 114)','excluded equipment sharing a port changed the selected-loop cooling control state');
 check(mount.querySelector('[data-hvac-inspect-component="pump-observation"]')?.textContent.includes('Off')&&mount.querySelector('[data-hvac-inspect-component="chiller-a-observation"]')?.textContent.includes('4.20'),'equipment status/power/COP missing');
 check(mount.querySelector('.hvac-loop-icon.pump')&&mount.querySelector('.hvac-loop-icon.chiller'),'equipment icons diverged from HVAC tab');
 check(!mount.querySelector('table,ul,dl')&&!mount.textContent.includes('Source data'),'topology introduced tabular or source/provenance detail');
 const annotationPositions=()=>[...mount.querySelectorAll('.hvac-inspect-annotation')].map(item=>item.getAttribute('transform')).join('|');
 const positions=annotationPositions();
 mount.innerHTML=render({loop,nodes:nodes.map(node=>({...node,metrics:node.metrics.map(metric=>({...metric,value:metric.id==='flow'?8:metric.value}))})),components,selectedComponent:'pump-observation'});
 check(annotationPositions()===positions,'frame change moved physical topology points');
 check(mount.querySelector('[data-hvac-inspect-component="pump-observation"]').classList.contains('selected')&&!mount.querySelector('[data-hvac-inspect-zoom],.hvac-inspect-topology-heading,.hvac-inspect-topology-viewport.fit'),'equipment selection failed or removed zoom/title returned');
 mount.innerHTML=render({loop,nodes:nodes.map(node=>({...node,metrics:node.metrics.map(metric=>({...metric,value:metric.id==='setpoint'?-998.9999999999999:metric.value}))})),components});
 check(!mount.querySelector('[data-hvac-setpoint-state]')&&mount.querySelector('[data-hvac-inspect-node="inlet-point"] [data-hvac-inspect-metric="temperature"] .hvac-inspect-metric-value').textContent==='7.00 °C','SQL roundoff produced a setpoint label/color on an uncontrolled node');
 check(annotationPositions()===positions,'missing setpoint changed topology positions');
 const frameGeometry=()=>[...mount.querySelectorAll('.hvac-inspect-vertex')].filter(item=>item.querySelector('.hvac-inspect-annotation')).map(item=>{
  const box=item.querySelector('.hvac-inspect-info-box'),annotation=item.querySelector('.hvac-inspect-annotation'),ring=item.querySelector('.hvac-inspect-node-ring,.hvac-inspect-equipment-ring');
  return [item.dataset.hvacInspectPointName,annotation.getAttribute('transform'),box.getAttribute('width'),box.getAttribute('height'),item.querySelector('.hvac-inspect-leader').getAttribute('d'),ring.getAttribute('cx'),ring.getAttribute('cy'),ring.getAttribute('r')].join(':');
 }).join('|');
 const longLoop=structuredClone(loop),longSignedNodes=nodes.map(node=>({...node,metrics:node.metrics.map(metric=>({...metric,value:metric.id==='temperature'?-123.45:metric.id==='setpoint'?123.45:metric.id==='flow'?123456.78:metric.value}))}));
 mount.innerHTML=render({loop:longLoop,nodes:longSignedNodes,components});assertHVACInfoBoxes(mount,'signed long node/setpoint values');
 const longGeometry=frameGeometry();
 const longDOM=[...mount.querySelectorAll('svg.hvac-loop-svg,.hvac-inspect-annotation,.hvac-inspect-info-box,.hvac-inspect-leader,.hvac-inspect-node-ring,.hvac-inspect-equipment-ring')];
 const nextLongFrame={loop:longLoop,nodes:longSignedNodes.map(node=>({...node,active:false,metrics:node.metrics.map(metric=>({...metric,value:metric.id==='temperature'?123.45:metric.id==='setpoint'?-98.76:metric.id==='flow'?0:metric.value}))})),components:components.map(item=>({...item,status:'off'})),selectedNode:'outlet-point'};
 check(update(mount,nextLongFrame),'same-shape frame did not use the in-place topology update');
 const nextLongDOM=[...mount.querySelectorAll('svg.hvac-loop-svg,.hvac-inspect-annotation,.hvac-inspect-info-box,.hvac-inspect-leader,.hvac-inspect-node-ring,.hvac-inspect-equipment-ring')];
 check(longDOM.length===nextLongDOM.length&&longDOM.every((item,index)=>item===nextLongDOM[index]&&item.isConnected),'frame update replaced SVG, information boxes, leaders or node rings');
 check(mount.querySelector('[data-hvac-inspect-node="outlet-point"]').classList.contains('selected')&&!mount.querySelector('[data-hvac-inspect-node="inlet-point"]').classList.contains('selected'),'in-place frame update did not refresh selection');
 check(mount.querySelector('[data-hvac-inspect-node="inlet-point"] [data-hvac-inspect-metric="flow"]').textContent.includes('0.00 kg/s')&&mount.querySelector('[data-hvac-setpoint-state="unmet"]')?.textContent.includes('123.45'),'in-place frame update retained stale numeric values or setpoint state');
 assertHVACInfoBoxes(mount,'changed digits and equipment activity');check(frameGeometry()===longGeometry,'frame signs, digit counts or activity changed information-box, leader or node geometry');
 const originalFont=document.body.style.fontFamily;document.body.style.fontFamily='monospace';
 check(!update(mount,nextLongFrame),'font change reused geometry measured with another font');mount.innerHTML=render(nextLongFrame);assertHVACInfoBoxes(mount,'remeasured monospace font');document.body.style.fontFamily=originalFont;
 mount.innerHTML=render(nextLongFrame);const originalLanguage=getLanguage();setLanguage('ko');
 check(!update(mount,nextLongFrame),'language change reused localized geometry');mount.innerHTML=render(nextLongFrame);assertHVACInfoBoxes(mount,'remeasured localized labels');setLanguage(originalLanguage);mount.innerHTML=render(nextLongFrame);
 const fewerMetrics={...nextLongFrame,nodes:nextLongFrame.nodes.map(node=>({...node,metrics:node.metrics.filter(metric=>metric.id!=='humidity')}))};
 check(!update(mount,fewerMetrics),'changed displayed metric rows reused old information-box geometry');
 const dedupNodes=[nodes[0],{...nodes[0],id:'duplicate-inlet',name:' supply inlet '},{...nodes[1],metrics:nodes[0].metrics.map(metric=>({...metric}))}];
 mount.innerHTML=render({loop,nodes:dedupNodes,components});
 check(mount.querySelectorAll('.node[data-hvac-inspect-topology-id="node:supply inlet"]').length===1&&mount.querySelectorAll('.node.measured').length===2,'same actual node was duplicated, or distinct equal-valued nodes were merged');
 assertHVACInfoBoxes(mount,'canonical physical-node deduplication');
 const serial={id:'serial-ports',name:'Serial ports',type:'PlantLoop',supplySide:{inletNode:'A',outletNode:'D',branches:[{name:'Serial branch',components:[equipment('Serial pump','A','B'),equipment('Serial boiler','B','C','Boiler:HotWater'),equipment('Serial valve','C','D','Valve:TwoWay')]}]},demandSide:{}};
 const serialOriginal=JSON.stringify(serial),serialNodes=['A','B','C','D'].map((name,index)=>({id:'serial-'+index,name,metrics:[{id:'temperature',label:'Temperature',value:42,unit:'°C'},{id:'flow',label:'Flow',value:1.25,unit:'kg/s'}]}));
 const serialComponents=serial.supplySide.branches[0].components.map((item,index)=>({id:'serial-equipment-'+index,name:item.objectName,type:item.objectType,status:'on',metrics:[{id:'power',label:'Power',value:100,unit:'W'}]}));
 mount.innerHTML=render({loop:serial,nodes:serialNodes,components:serialComponents});assertHVACInfoBoxes(mount,'serial pump boiler valve ports');
 check(mount.querySelectorAll('.node.measured').length===4,'serial shared ports gained duplicate labels or equal-valued distinct ports were lost');
 const serialPlaced=inspectionLayout({loop:serial,nodes:serialNodes,components:serialComponents});
 for(const [name,role]of [['B','inlet'],['C','outlet']])check(serialPlaced.layout.anchors.some(anchor=>anchor.kind==='node'&&anchor.name===name&&anchor.ownerName==='Serial boiler'&&anchor.ownerType==='Boiler:HotWater'&&anchor.portRole===role),'boiler '+role+' lost exact source port ownership');
 check(JSON.stringify(serial)===serialOriginal,'integrated equipment/port placement mutated native serial connections');
 mount.innerHTML=render({loop:wrapper,nodes:extraNodes,components:children,selectedNode:'internal-point'});
 check(mount.querySelector('[data-hvac-inspect-component="fan-child"]')?.dataset.hvacInspectAnchorKind==='parent'&&mount.querySelector('[data-hvac-inspect-component="coil-child"]'),'owned typed child measurements lost their explicit wrapper anchor');
 check(!mount.querySelector('[data-hvac-inspect-node="internal-point"]'),'typed internal port without a physical schematic anchor gained a detached label');
 check(!mount.querySelector('[data-hvac-inspect-component="unknown-device"],[data-hvac-inspect-component="wrong-type"],[data-hvac-inspect-node="detached-point"],.unanchored,.hvac-inspect-detached-label'),'unrelated or unanchored frame observations still occupy the loop diagram');
 const observationSource=JSON.stringify({nodes:extraNodes,components:children});
 const observationSeries=(key,name,value)=>({file:'fixture.sql',keyValue:key,name,reportingFrequency:'Hourly',column:key+':'+name+' [W](Hourly)',points:[{x:1,label:'01/01 01:00',value},{x:2,label:'01/01 02:00',value}]});
 const investigation={name:'Unitary inspection',loopType:'AirLoopHVAC',topology:wrapper,series:extraNodes.map(item=>observationSeries(item.name,'System Node Temperature',20)),components:children.map(item=>({...item,componentName:item.name,componentType:item.type,series:[observationSeries(item.name,'Boiler Heating Rate',100)]}))};
 const investigationModel=prepare(investigation);mount.innerHTML=renderInspection(investigation,{});
 check(investigationModel.entities.some(item=>item.name==='Unmapped point')&&investigationModel.entities.some(item=>item.name==='Unmapped device')&&investigationModel.entities.some(item=>item.name==='Coil inlet'),'diagram scoping removed original or typed internal observations from Data');
 check([...mount.querySelectorAll('[data-hvac-inspect-entity] option')].some(item=>item.textContent==='Unmapped device')&&!mount.querySelector('[data-hvac-inspect-point-name="Unmapped device"],[data-hvac-inspect-point-name="Unmapped point"]'),'unmapped observation must remain selectable in Data while omitted from topology');
 check(JSON.stringify({nodes:extraNodes,components:children})===observationSource,'diagram filtering mutated raw snapshots');
 const longNodes=['VAV_2_COOLCDEMAND INLET NODE','VAV_2_COOLCDEMAND OUTLET NODE','VAV_2_HEATCDEMAND INLET NODE','VAV_2_HEATCDEMAND OUTLET NODE'].map((name,index)=>({id:'long-node-'+index,name,metrics:nodes[0].metrics}));
 mount.innerHTML=render({loop,nodes:longNodes});check(!mount.querySelector('[data-hvac-inspect-node],.hvac-inspect-detached-label'),'other-loop port names were added by observation alone');checkAnnotationSpacing();
 mount.innerHTML=render({nodes,components});check(mount.querySelector('[data-hvac-inspect-topology-empty]')&&!mount.querySelector('svg'),'missing model topology generated guessed wiring');
 const operatingSource={name:'Chiller labels',loopType:'PlantLoop',topology:loop,series:[],components:[{componentName:'Chiller A',componentType:'Chiller:Electric:EIR',series:[
  ['Chiller Electricity Rate','W',20820],['Chiller COP','',8.69],['Chiller Part Load Ratio','',.20],['Chiller Cycling Ratio','',.41]
 ].map(([name,unit,value])=>({file:'operating.sql',name,keyValue:'Chiller A',column:'Chiller A:'+name+' ['+unit+'](Hourly)',reportingFrequency:'Hourly',points:[{x:1,label:'01/01 01:00',value},{x:2,label:'01/01 02:00',value:0}]}))}]};
 const labelLanguage=getLanguage();
 for(const language of ['en','ko','ja']){
  setLanguage(language);const model=prepare(operatingSource),first=snapshots(model,0),later=snapshots(model,1);
  mount.innerHTML=render({loop,...first});
  const chiller=mount.querySelector('[data-hvac-inspect-point-name="Chiller A"]'),plr=chiller?.querySelector('[data-hvac-inspect-metric-quantity="part_load_ratio"]'),cycling=chiller?.querySelector('[data-hvac-inspect-metric-quantity="cycling_ratio"]');
  check(plr?.querySelector('.hvac-inspect-metric-label').textContent==='PLR'&&cycling?.querySelector('.hvac-inspect-metric-label').textContent==='Cyc','native PLR/cycling symbols depend on translated labels: '+language);
  check(plr?.dataset.hvacInspectMetricName==='Chiller Part Load Ratio'&&cycling?.dataset.hvacInspectMetricName==='Chiller Cycling Ratio'&&plr.querySelector('.hvac-inspect-metric-value').textContent==='0.20'&&cycling.querySelector('.hvac-inspect-metric-value').textContent==='0.41','distinct same-id status observations lost native identity or numeric values');
  check(chiller.querySelector('title').textContent.includes('Chiller Part Load Ratio')&&chiller.querySelector('title').textContent.includes('Chiller Cycling Ratio'),'compact status symbols lack their full native meaning on hover');
  for(const row of [plr,cycling]){const symbol=row.querySelector('.hvac-inspect-metric-label'),value=row.querySelector('.hvac-inspect-metric-value');check(symbol.getBBox().x+symbol.getBBox().width<value.getBBox().x,'operating symbol overlaps the fixed numeric column');}
  const before=frameGeometry();check(update(mount,{loop,...later}),'operating frame update unnecessarily replaced the SVG');
  check(plr.isConnected&&cycling.isConnected&&plr.querySelector('.hvac-inspect-metric-value').textContent==='0.00'&&cycling.querySelector('.hvac-inspect-metric-value').textContent==='0.00'&&frameGeometry()===before,'same-id PLR and cycling rows overwrite each other or move during playback');
  assertHVACInfoBoxes(mount,'localized operating symbols '+language);
  const changed={...later,components:later.components.map(component=>({...component,metrics:component.metrics.map(metric=>metric.quantity==='part_load_ratio'?{...metric,quantity:'runtime_fraction',name:'Fan Runtime Fraction'}:metric)}))};
  check(!update(mount,{loop,...changed}),'changed native operating quantity reused a stale label/schema');
 }
 setLanguage(labelLanguage);
 const runtimeLoop={id:'runtime-fraction',name:'Cycling fan',type:'AirLoopHVAC',supplySide:{branches:[{components:[equipment('Cycling fan','Fan inlet','Fan outlet','Fan:OnOff')]}]},demandSide:{}};
 mount.innerHTML=render({loop:runtimeLoop,components:[{id:'runtime-fan',name:'Cycling fan',type:'Fan:OnOff',status:'on',metrics:[{id:'status',quantity:'runtime_fraction',name:'Fan Runtime Fraction',label:'PLR',unit:'—',value:.5}]}]});
 check(mount.querySelector('[data-hvac-inspect-metric-quantity="runtime_fraction"] .hvac-inspect-metric-label')?.textContent==='RTF','runtime fraction was inferred from a misleading display label');assertHVACInfoBoxes(mount,'runtime-fraction symbol');
 mount.innerHTML=render({loop,nodes,components,selectedNode:'inlet-point'});
 check(JSON.stringify(loop)===original,'inspection altered executed model topology');
 document.body.dataset.inspectionTopologyStatus='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.inspectionTopologyStatus='failed';document.getElementById('result').textContent=error.stack;}
</script></body></html>`
