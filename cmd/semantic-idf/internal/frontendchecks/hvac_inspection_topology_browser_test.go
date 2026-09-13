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
		t.Fatalf("HVAC inspection topology acceptance failed: %v\n%s", err, output)
	}
}

const hvacInspectionTopologyHTML = `<!doctype html><html data-theme="dark"><head><link rel="stylesheet" href="/src/styles.css"><link rel="stylesheet" href="/src/styles/hvac-inspection-topology.css"></head><body style="display:block;overflow:auto"><div id="mount"></div><pre id="result"></pre>
<script type="module">
const check=(value,message)=>{if(!value)throw new Error(message);};
try {
 const {buildHVACInspectionTopology:build,renderHVACInspectionTopology:render}=await import('/src/js/views/hvac-inspection-topology.js');
 const {renderHVACLoopDiagram}=await import('/src/js/views/hvac-views.js');
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
 {id:'unknown-device',name:'Unmapped device',type:'Boiler:HotWater',metrics:[{id:'power',label:'Power',value:100,unit:'W'}]}];
 const extraNodes=[{id:'internal-point',name:'Coil inlet',metrics:[{id:'temperature',label:'Temperature',value:20,unit:'°C'}]},{id:'detached-point',name:'Unmapped point',metrics:[{id:'flow',label:'Flow',value:0,unit:'kg/s'}]}];
 const expanded=build(wrapper,{nodes:extraNodes,components:children});
 check(expanded.edges.some(item=>item.from==='observed:fan-child'&&item.to==='node:coil inlet'&&item.role==='air_outlet')&&expanded.edges.some(item=>item.from==='node:coil inlet'&&item.to==='observed:coil-child'),'typed inner fan/coil ports lost their real connections or medium role');
 check(!expanded.edges.some(item=>item.from==='component:supply:0:0'||item.to==='component:supply:0:0'),'expanded full internal path left a false wrapper bypass');
 check(expanded.vertices.some(item=>item.id==='observed:unknown-device')&&expanded.vertices.some(item=>item.id==='node:unmapped point')&&!expanded.edges.some(item=>item.from==='observed:unknown-device'||item.to==='node:unmapped point'),'unknown measured items disappeared or gained invented wires');
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
 check(mount.querySelector('.hvac-loop-connector').getBBox().width>reference.querySelector('.hvac-loop-connector').getBBox().width&&Number(mount.querySelector('.hvac-loop-side-panel').getAttribute('width'))>Number(reference.querySelector('.hvac-loop-side-panel').getAttribute('width')),'extra width did not reach the pipe runs and branch annotation area');
 const checkAnnotationSpacing=()=>{
  const annotations=[...mount.querySelectorAll('.hvac-inspect-annotation')].map(item=>({item,rect:item.getBoundingClientRect()}));
  for(let i=0;i<annotations.length;i++)for(let j=i+1;j<annotations.length;j++){
   const a=annotations[i].rect,b=annotations[j].rect;
   check(Math.min(a.right,b.right)-Math.max(a.left,b.left)<=1||Math.min(a.bottom,b.bottom)-Math.max(a.top,b.top)<=1,'frame text overlaps: '+annotations[i].item.textContent+' / '+annotations[j].item.textContent);
  }
 };
 check(mount.querySelectorAll('.hvac-inspect-annotation').length>=nodes.length+components.length,'frame measurements must occupy annotation space on the original diagram');
 checkAnnotationSpacing();
 const inlet=mount.querySelector('[data-hvac-inspect-node="inlet-point"]');
 check(inlet?.classList.contains('measured')&&inlet.classList.contains('selected'),'frame point did not match case-insensitive physical node or emphasize selected point');
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
  check(symbol.getStartPositionOfChar(0).x===0&&value.getStartPositionOfChar(0).x===32,'frame measurement labels and values do not align in fixed columns');
  check(symbol.getBBox().x+symbol.getBBox().width<value.getBBox().x,'frame measurement label overlaps its value');
  check(Math.abs(value.getStartPositionOfChar(0).y-Number(row.getAttribute('y')))<.1,'a subscript shifted the measured value off its row');
 }
 const comparison=inlet.querySelector('[data-hvac-setpoint-state="unmet"]');
 check(!inlet.querySelector('[data-hvac-inspect-metric="setpoint"]')&&comparison?.textContent==='7.00 >set 6.00'&&comparison.querySelector('[baseline-shift="sub"]')?.textContent==='set','optional setpoint must be inline with temperature and reflect cooling inequality');
 check(getComputedStyle(comparison).fontWeight==='700'&&getComputedStyle(comparison).fill==='rgb(255, 123, 114)','unmet setpoint comparison is not bold red');
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
 mount.innerHTML=render({loop:wrapper,nodes:extraNodes,components:children,selectedNode:'internal-point'});
 check(mount.querySelector('[data-hvac-inspect-node="internal-point"]')?.classList.contains('selected')&&mount.querySelector('[data-hvac-inspect-component="fan-child"]')&&mount.querySelector('[data-hvac-inspect-component="unknown-device"]')&&mount.querySelector('[data-hvac-inspect-node="detached-point"]'),'expanded internal or detached observations are not interactive/visible');
 const longNodes=['VAV_2_COOLCDEMAND INLET NODE','VAV_2_COOLCDEMAND OUTLET NODE','VAV_2_HEATCDEMAND INLET NODE','VAV_2_HEATCDEMAND OUTLET NODE'].map((name,index)=>({id:'long-node-'+index,name,metrics:nodes[0].metrics}));
 mount.innerHTML=render({loop,nodes:longNodes});checkAnnotationSpacing();
 mount.innerHTML=render({nodes,components});check(mount.querySelector('[data-hvac-inspect-topology-empty]')&&!mount.querySelector('svg'),'missing model topology generated guessed wiring');
 mount.innerHTML=render({loop,nodes,components,selectedNode:'inlet-point'});
 check(JSON.stringify(loop)===original,'inspection altered executed model topology');
 document.body.dataset.inspectionTopologyStatus='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.inspectionTopologyStatus='failed';document.getElementById('result').textContent=error.stack;}
</script></body></html>`
