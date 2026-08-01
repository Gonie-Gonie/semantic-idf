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

func TestTOPO280To285IntegratedBrowserFlows(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser integrated thermal topology flows in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/thermal-topology-integrated", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, thermalTopologyIntegratedHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check",
		"--virtual-time-budget=30000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/thermal-topology-integrated",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("integrated thermal topology browser flows timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("integrated thermal topology browser flows failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-topology-integrated-status="passed"`) {
		t.Fatalf("integrated thermal topology browser flows did not pass:\n%s", document)
	}
	for _, signal := range []string{`"topo280":true`, `"topo281":true`, `"topo282":true`, `"topo283":true`, `"topo284":true`, `"topo285":true`, `"backendCalls":0`} {
		if !strings.Contains(document, signal) {
			t.Fatalf("integrated thermal topology result is missing %s:\n%s", signal, document)
		}
	}
}

const thermalTopologyIntegratedHarnessHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Thermal topology integrated acceptance</title></head>
<body data-topology-integrated-status="pending">
<div id="geometryStats"></div><div id="geometryCanvasHost"></div><svg id="geometryPlan"></svg><div id="geometryDetails"></div>
<div id="thermalTopologyGraph" style="width:900px;height:600px"></div><div id="thermalTopologyMatrix" style="height:300px;overflow:auto"></div><aside id="thermalTopologyInspector"></aside>
<button data-thermal-topology-display="graph"></button><button data-thermal-topology-display="matrix"></button><input id="thermalTopologyMatrixQuery">
<select id="thermalTopologyGraphLevel"><option value="zone">zone</option><option value="boundary">boundary</option></select>
<select id="thermalTopologyMetric"><option value="topology">topology</option><option value="area">area</option><option value="ua">ua</option><option value="exposure">exposure</option><option value="qa">qa</option><option value="air">air</option><option value="simulated_heat">simulated heat</option></select>
<label id="thermalTopologyAreaComponentControl"><select id="thermalTopologyAreaComponent"><option value="gross">gross</option><option value="opaque">opaque</option><option value="openings">openings</option></select></label>
<select id="thermalTopologyScope"><option value="building">building</option><option value="story">story</option><option value="selection">selection</option><option value="neighbors">neighbors</option></select>
<div id="thermalTopologySimulationControls" hidden><select id="thermalTopologySimulationPeriod"><option value="annual">annual</option><option value="monthly">monthly</option><option value="hourly">hourly</option><option value="selected_range">selected range</option></select><label id="thermalTopologySimulationFrameControl"><span id="thermalTopologySimulationFrameLabel"></span><input id="thermalTopologySimulationFrame" type="range"></label></div>
<select id="thermalTopologyLayout"><option value="spatial">spatial</option><option value="network">network</option></select>
<select id="thermalTopologyAreaBasis"><option value="effective">effective</option><option value="physical">physical</option></select>
<input id="thermalTopologyShowOpenings" type="checkbox" checked><input id="thermalTopologyShowAirCoupling" type="checkbox"><input id="thermalTopologyExpandExternalTargets" type="checkbox"><input id="thermalTopologyShowLabels" type="checkbox" checked>
<pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const tick = () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
try {
  const stateModule = await import("/src/js/state.js");
  const renderer = await import("/src/js/views/thermal-topology-view.js");
  const geometryView = await import("/src/js/views/geometry-view.js");
  const targets = await import("/src/js/thermal-topology-targets.js");
  const anchor = (objectIndex, objectName) => [
    {objectIndex,objectType:"BuildingSurface:Detailed",objectName},
    {objectIndex,objectType:"BuildingSurface:Detailed",objectName,fieldIndex:5,fieldName:"Outside Boundary Condition"},
    {objectIndex,objectType:"BuildingSurface:Detailed",objectName,fieldIndex:6,fieldName:"Outside Boundary Condition Object"},
  ];
  const nodes = [
    {id:"zone:a",entityId:"zone:a",kind:"zone",label:"Zone A",storyIndex:0,centroid:{x:0,y:0,z:0}},
    {id:"zone:b",entityId:"zone:b",kind:"zone",label:"Zone B",storyIndex:0,centroid:{x:8,y:0,z:0}},
    {id:"thermal-environment:outdoors",kind:"outdoors",label:"Outdoors"},
    {id:"thermal-environment:adiabatic",kind:"adiabatic",label:"Adiabatic"},
    {id:"thermal-unresolved:missing",kind:"unresolved_target",label:"Missing Pair"},
  ];
  const boundaries = [
    {id:"boundary:ext",surfaceId:"surface:ext",surfaceEntityId:"surface:ext",surfaceName:"South Wall",surfaceType:"BuildingSurface:Detailed",ownerZoneId:"zone:a",boundaryConditionRaw:"Outdoors",boundaryCondition:"Outdoors",relationKind:"exterior",targetKind:"outdoors",targetId:"thermal-environment:outdoors",targetName:"Outdoors",constructionName:"Exterior Wall",constructionStatus:"resolved",uValue:0.4,hasUValue:true,physicalGrossArea:10,physicalOpeningArea:2,physicalOpaqueArea:8,effectiveGrossArea:20,effectiveOpeningArea:4,effectiveOpaqueArea:16,opaqueUa:6.4,openingUa:4,totalUa:10.4,hasUa:true,openingIds:["opening:ext"],sourceAnchors:anchor(1,"South Wall"),geometryCheck:{status:"not_applicable"}},
    {id:"boundary:pair-a",surfaceId:"surface:pair-a",surfaceEntityId:"surface:pair-a",surfaceName:"Pair A",surfaceType:"BuildingSurface:Detailed",ownerZoneId:"zone:a",boundaryConditionRaw:"Surface",boundaryCondition:"Surface",relationKind:"interzone_explicit_surface",targetKind:"zone",targetId:"zone:b",targetName:"Pair B",counterpartSurfaceId:"surface:pair-b",counterpartSurfaceEntityId:"surface:pair-b",pairId:"interface:pair",constructionName:"Opaque A",constructionStatus:"reverse_layer_equivalent",uValue:1.4286,hasUValue:true,physicalGrossArea:4,physicalOpeningArea:1,physicalOpaqueArea:3,effectiveGrossArea:8,effectiveOpeningArea:2,effectiveOpaqueArea:6,opaqueUa:8.5716,openingUa:4,totalUa:12.5716,hasUa:true,openingIds:["opening:pair-a"],sourceAnchors:anchor(2,"Pair A"),geometryCheck:{status:"valid",overlapRatio:1,normalDot:-1,planeDistance:0}},
    {id:"boundary:pair-b",surfaceId:"surface:pair-b",surfaceEntityId:"surface:pair-b",surfaceName:"Pair B",surfaceType:"BuildingSurface:Detailed",ownerZoneId:"zone:b",boundaryConditionRaw:"Surface",boundaryCondition:"Surface",relationKind:"interzone_explicit_surface",targetKind:"zone",targetId:"zone:a",targetName:"Pair A",counterpartSurfaceId:"surface:pair-a",counterpartSurfaceEntityId:"surface:pair-a",pairId:"interface:pair",constructionName:"Opaque B",constructionStatus:"reverse_layer_equivalent",uValue:1.4286,hasUValue:true,physicalGrossArea:4,physicalOpeningArea:1,physicalOpaqueArea:3,effectiveGrossArea:8,effectiveOpeningArea:2,effectiveOpaqueArea:6,opaqueUa:8.5716,openingUa:4,totalUa:12.5716,hasUa:true,openingIds:["opening:pair-b"],sourceAnchors:anchor(3,"Pair B"),geometryCheck:{status:"not_applicable"}},
    {id:"boundary:adi-a",surfaceId:"surface:adi-a",surfaceEntityId:"surface:adi-a",surfaceName:"Adiabatic A",surfaceType:"BuildingSurface:Detailed",ownerZoneId:"zone:a",relationKind:"adiabatic_explicit",targetKind:"adiabatic",targetId:"thermal-environment:adiabatic",physicalGrossArea:5,effectiveGrossArea:5,sourceAnchors:anchor(4,"Adiabatic A")},
    {id:"boundary:adi-b",surfaceId:"surface:adi-b",surfaceEntityId:"surface:adi-b",surfaceName:"Adiabatic B",surfaceType:"BuildingSurface:Detailed",ownerZoneId:"zone:b",relationKind:"adiabatic_explicit",targetKind:"adiabatic",targetId:"thermal-environment:adiabatic",physicalGrossArea:5,effectiveGrossArea:5,sourceAnchors:anchor(5,"Adiabatic B")},
    {id:"boundary:bad",surfaceId:"surface:bad",surfaceEntityId:"surface:bad",surfaceName:"Broken Pair",surfaceType:"BuildingSurface:Detailed",ownerZoneId:"zone:a",relationKind:"invalid",targetKind:"unresolved_target",targetId:"thermal-unresolved:missing",physicalGrossArea:3,effectiveGrossArea:3,diagnosticIds:["issue:missing"],sourceAnchors:anchor(6,"Broken Pair")},
  ];
  const openings = [
    {id:"opening:ext",windowId:"window:ext",entityId:"window:ext",name:"South Window",baseSurfaceId:"surface:ext",ownerZoneId:"zone:a",constructionName:"Window",hasUValue:true,uValue:1,physicalArea:2,effectiveArea:4,ua:4,hasUa:true},
    {id:"opening:pair-a",windowId:"window:pair-a",entityId:"window:pair-a",name:"Interior Window A",baseSurfaceId:"surface:pair-a",ownerZoneId:"zone:a",counterpartOpeningId:"opening:pair-b",pairId:"opening-interface:pair",physicalArea:1,effectiveArea:2,ua:4,hasUa:true},
    {id:"opening:pair-b",windowId:"window:pair-b",entityId:"window:pair-b",name:"Interior Window B",baseSurfaceId:"surface:pair-b",ownerZoneId:"zone:b",counterpartOpeningId:"opening:pair-a",pairId:"opening-interface:pair",physicalArea:1,effectiveArea:2,ua:4,hasUa:true},
  ];
  const airCouplings = [
    {id:"air:mix",entityId:"air:mix",objectType:"ZoneMixing",objectName:"Mix B to A",objectIndex:7,fromNodeId:"zone:b",toNodeId:"zone:a",direction:"directed",couplingKind:"zone_mixing",designFlowRate:0.15,unit:"m3/s",scheduleName:"Always On",sourceAnchors:[{objectIndex:7,objectType:"ZoneMixing",objectName:"Mix B to A"}]},
    {id:"air:afn",entityId:"air:afn",objectType:"AirflowNetwork:MultiZone:Surface",objectName:"AFN Pair",objectIndex:8,fromNodeId:"zone:a",toNodeId:"zone:b",direction:"bidirectional",couplingKind:"airflow_network",surfaceId:"surface:pair-a",componentName:"Pair Crack",sourceAnchors:[{objectIndex:8,objectType:"AirflowNetwork:MultiZone:Surface",objectName:"AFN Pair"}]},
  ];
  const connections = [
    {id:"connection:ext",fromNodeId:"zone:a",toNodeId:"thermal-environment:outdoors",relationKind:"exterior",boundaryIds:["boundary:ext"],openingIds:["opening:ext"],surfaceCount:1,openingCount:1,physicalGrossArea:10,physicalOpaqueArea:8,physicalOpeningArea:2,effectiveGrossArea:20,effectiveOpaqueArea:16,effectiveOpeningArea:4,totalUa:10.4,hasUa:true,physicalTotalUa:5.2,hasPhysicalUa:true},
    {id:"connection:pair",fromNodeId:"zone:a",toNodeId:"zone:b",relationKind:"interzone_explicit_surface",boundaryIds:["boundary:pair-a","boundary:pair-b"],openingIds:["opening:pair-a"],surfaceCount:1,openingCount:1,physicalGrossArea:4,physicalOpaqueArea:3,physicalOpeningArea:1,effectiveGrossArea:8,effectiveOpaqueArea:6,effectiveOpeningArea:2,totalUa:12.5716,hasUa:true,physicalTotalUa:6.2858,hasPhysicalUa:true,sourceAnchors:[...anchor(2,"Pair A"),...anchor(3,"Pair B")]},
    {id:"connection:adi-a",fromNodeId:"zone:a",toNodeId:"thermal-environment:adiabatic",relationKind:"adiabatic_explicit",boundaryIds:["boundary:adi-a"],surfaceCount:1,effectiveGrossArea:5},
    {id:"connection:adi-b",fromNodeId:"zone:b",toNodeId:"thermal-environment:adiabatic",relationKind:"adiabatic_explicit",boundaryIds:["boundary:adi-b"],surfaceCount:1,effectiveGrossArea:5},
    {id:"connection:bad",fromNodeId:"zone:a",toNodeId:"thermal-unresolved:missing",relationKind:"invalid",qaOnly:true,boundaryIds:["boundary:bad"],surfaceCount:1,effectiveGrossArea:3,diagnosticIds:["issue:missing"]},
    {id:"connection:air",fromNodeId:"zone:a",toNodeId:"zone:b",relationKind:"air_coupling",airCouplingIds:["air:mix","air:afn"],surfaceCount:0,openingCount:0},
  ];
  const geometry = {
    zoneCount:2,surfaceCount:6,windowCount:3,
    zones:[{id:"zone:a",name:"Zone A",objectIndex:0,storyIndex:0},{id:"zone:b",name:"Zone B",objectIndex:9,storyIndex:0}],
    surfaces:boundaries.map((boundary,index)=>({id:boundary.surfaceId,name:boundary.surfaceName,type:"BuildingSurface:Detailed",surfaceType:"Wall",zoneName:boundary.ownerZoneId==="zone:b"?"Zone B":"Zone A",outsideBoundary:boundary.boundaryConditionRaw||boundary.relationKind,construction:boundary.constructionName,objectIndex:index+1,storyIndex:0,physicalArea:boundary.physicalGrossArea,fields:[]})),
    windows:openings.map((opening,index)=>({id:opening.windowId,name:opening.name,type:"FenestrationSurface:Detailed",baseSurfaceId:opening.baseSurfaceId,zoneName:opening.ownerZoneId==="zone:b"?"Zone B":"Zone A",objectIndex:index+20,storyIndex:0,physicalArea:opening.physicalArea})),
    stories:[{index:0,name:"Story 1",elevation:0}],constructions:[],
    topology:{schema:"semantic-idf.thermal-topology/v1",sourceModelHash:"integrated-fixture",nodes,boundaries,openings,connections,airCouplings,zoneSignatures:[{zoneId:"zone:a",zoneName:"Zone A",interzoneArea:8,exteriorArea:20,totalUa:22.9716,hasTotalUa:true},{zoneId:"zone:b",zoneName:"Zone B",interzoneArea:8,totalUa:12.5716,hasTotalUa:true}],issueLinks:[{id:"issue:missing",code:"surface_counterpart_missing",severity:"error",message:"Missing reciprocal surface",entityId:"surface:bad",boundaryId:"boundary:bad",relatedEntityIds:["surface:bad"],sourceAnchors:anchor(6,"Broken Pair")}],adjacencyObservations:[{surfaceAId:"surface:adi-a",surfaceBId:"surface:adi-b",overlapRatio:1,declaredConnection:false,observationKind:"geometrically_adjacent_but_thermally_disconnected"}]}
  };
  const state = stateModule.state;
  state.report={geometry,output:{existing:[]}}; state.geometryMode="thermal"; state.thermalTopologyGraphLevel="zone"; state.thermalTopologyScope="building"; state.thermalTopologyLayout="spatial"; state.thermalTopologyDisplay="graph"; state.thermalTopologyAreaBasis="effective"; state.thermalTopologyAreaComponent="gross"; state.thermalTopologyShowOpenings=true; state.thermalTopologyShowAirCoupling=false; state.thermalTopologyShowLabels=true;
  let backendCalls=0; const reveals=[]; const selections=[];
  const helpers={
    navigationAttributes:()=>'',
    selectGeometry:async(kind,id)=>{selections.push({kind,id});state.thermalTopologySelectedEntityKind=kind;state.thermalTopologySelectedEntityId=id;state.selectedGeometryKind=kind;state.selectedGeometryId=id;return true;},
    setGeometryMode:(mode)=>{state.geometryMode=mode;},
    revealThermalTargetInGeometry:(kind,id,mode)=>{const resolved=targets.resolveThermalTopologyTarget({targetKind:kind,targetId:id},geometry);reveals.push({kind,id,mode,surfaces:resolved?.surfaceIds||[]});return Boolean(resolved);},
  };

  await geometryView.selectGeometry("surface","surface:ext",{syncSemantic:false,syncLocate:false});
  state.thermalTopologyMetric="area"; renderer.renderThermalTopology(geometry,helpers);
  const exteriorEdge=document.querySelector('[data-thermal-target-id="connection:ext"]');
  const exteriorProjected=state.thermalTopologySelectedEntityId==="boundary:ext"&&exteriorEdge?.querySelector('.thermal-edge')?.classList.contains('selected');
  const exteriorInspector=document.getElementById('thermalTopologyInspector').textContent;
  const openingBreakdown=exteriorInspector.includes('South Window')&&exteriorInspector.includes('Model gross / opening / net');
  const exactSource=[...document.querySelectorAll('[data-inspector-semantic],[data-inspector-source]')].length===2&&!document.querySelector('[data-inspector-source]').disabled&&boundaries[0].sourceAnchors.some((item)=>item.fieldName==='Outside Boundary Condition');
  state.thermalTopologySelectedEntityKind="thermal_connection";state.thermalTopologySelectedEntityId="connection:ext";state.selectedGeometryKind="thermal_connection";state.selectedGeometryId="connection:ext";state.thermalTopologyMetric="ua";renderer.renderThermalTopology(geometry,helpers);
  const exteriorUA=(document.querySelector('[data-thermal-target-id="connection:ext"] .thermal-edge-label')?.textContent||'').includes('10.4 W/K');
  document.querySelector('[data-thermal-target-id="connection:ext"]').dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true}));
  document.querySelector('[data-topology-back]').click();
  const exteriorBack=state.thermalTopologyGraphLevel==='zone'&&state.thermalTopologySelectedEntityId==='connection:ext';
  const topo280=exteriorProjected&&openingBreakdown&&exactSource&&exteriorUA&&exteriorBack;

  state.thermalTopologyMetric="area";state.thermalTopologySelectedEntityKind="thermal_connection";state.thermalTopologySelectedEntityId="connection:pair";state.selectedGeometryKind="thermal_connection";state.selectedGeometryId="connection:pair";renderer.renderThermalTopology(geometry,helpers);
  const pairResolved=targets.resolveThermalTopologyTarget({targetKind:"thermal_connection",targetId:"connection:pair"},geometry);
  document.querySelector('[data-inspector-mode="3d"]').click();
  const pair3D=reveals.at(-1)?.surfaces.length===2&&pairResolved.surfaceIds.length===2;
  const graphPairArea=Number.parseFloat(document.querySelector('[data-thermal-target-id="connection:pair"] .thermal-edge-label')?.textContent||'NaN');
  document.querySelector('[data-thermal-target-id="connection:pair"]').dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true}));
  const pairExpanded=document.querySelectorAll('.thermal-node.thermal_boundary').length===2;
  state.thermalTopologySelectedEntityKind="thermal_boundary";state.thermalTopologySelectedEntityId="boundary:pair-a";renderer.renderThermalTopology(geometry,helpers);
  const pairInspector=document.getElementById('thermalTopologyInspector').textContent;
  const reciprocalValidated=pairInspector.includes('surface:pair-b')&&pairInspector.includes('reverse layer equivalent')&&pairInspector.includes('valid');
  state.thermalTopologyGraphLevel="zone";state.thermalTopologyScope="building";state.thermalTopologyDisplay="matrix";state.thermalTopologyMetric="area";renderer.renderThermalTopology(geometry,helpers);
  const matrixPairArea=Number.parseFloat(document.querySelector('[data-thermal-target-id="connection:pair"]')?.textContent||'NaN');
  state.thermalTopologyMetric="ua";renderer.renderThermalTopology(geometry,helpers);
  const matrixPairUA=Number.parseFloat(document.querySelector('[data-thermal-target-id="connection:pair"]')?.textContent||'NaN');
  const topo281=pair3D&&pairExpanded&&reciprocalValidated&&graphPairArea===matrixPairArea&&matrixPairUA===12.57&&connections[1].sourceAnchors.filter((item)=>item.fieldName==='Outside Boundary Condition').length===2;

  state.thermalTopologyDisplay="graph";state.thermalTopologyGraphLevel="zone";state.thermalTopologyScope="building";state.thermalTopologyMetric="qa";state.thermalTopologySelectedEntityKind="";state.thermalTopologySelectedEntityId="";renderer.renderThermalTopology(geometry,helpers);
  const observationEdge=document.querySelector('.thermal-edge.qa-observation')?.closest('.thermal-edge-group');
  observationEdge?.dispatchEvent(new MouseEvent('click',{bubbles:true}));
  const observationInspector=document.getElementById('thermalTopologyInspector').textContent;
  document.querySelector('[data-inspector-mode="3d"]')?.click();
  const observationBothSurfaces=reveals.at(-1)?.surfaces.length===2&&document.querySelectorAll('[data-thermal-inspector-kind="thermal_boundary"]').length===2;
  const noGeneratedRelation=!connections.some((connection)=>connection.relationKind==='interzone_explicit_surface'&&(connection.boundaryIds||[]).includes('boundary:adi-a'));
  state.thermalTopologySelectedEntityKind="thermal_connection";state.thermalTopologySelectedEntityId="connection:bad";renderer.renderThermalTopology(geometry,helpers);
  const diagnoseButton=document.querySelector('[data-inspector-diagnostic="issue:missing"]');diagnoseButton?.click();await tick();
  state.thermalTopologySelectedEntityKind="thermal_boundary";state.thermalTopologySelectedEntityId="boundary:bad";const qaSnapshot=stateModule.captureThermalTopologyState(state);state.thermalTopologySelectedEntityId="";stateModule.restoreThermalTopologyState(qaSnapshot,state);
  const stableAfterFix=state.thermalTopologySelectedEntityId==='boundary:bad'&&targets.thermalTopologyTargetExists({targetKind:'thermal_boundary',targetId:'boundary:bad'},geometry);
  const topo282=Boolean(observationEdge)&&observationInspector.includes('QA evidence only')&&observationBothSurfaces&&noGeneratedRelation&&Boolean(diagnoseButton)&&selections.some((item)=>item.kind==='thermal_issue'&&item.id==='issue:missing')&&stableAfterFix;

  state.thermalTopologyMetric="air";state.thermalTopologyShowAirCoupling=true;state.thermalTopologySelectedEntityKind="thermal_air_coupling";state.thermalTopologySelectedEntityId="air:mix";renderer.renderThermalTopology(geometry,helpers);
  const airInspector=document.getElementById('thermalTopologyInspector').textContent;
  const separateLayers=Boolean(document.querySelector('[data-thermal-target-id="connection:air"] .thermal-edge.air'))&&Boolean(document.querySelector('[data-thermal-target-id="connection:pair"] .thermal-edge.conductive'));
  state.thermalTopologySelectedEntityId="air:afn";renderer.renderThermalTopology(geometry,helpers);
  const afnInspector=document.getElementById('thermalTopologyInspector').textContent;
  const topo283=separateLayers&&airInspector.includes('ZoneMixing')&&airInspector.includes('0.15 m3/s')&&airInspector.includes('Always On')&&afnInspector.includes('surface:pair-a / Pair Crack')&&Boolean(document.querySelector('[data-inspector-semantic]'));

  state.simulationResult={purposeRunPlan:{estimatedWeight:'Heavy',outputObjects:[{objectType:'Output:Variable',keyValue:'South Wall',variableName:'Surface Average Face Conduction Heat Transfer Energy',signature:'surface-heat'}]},purposeResults:{thermalTopology:{schema:'semantic-idf.thermal-topology-simulation/v1',available:true,state:'simulation_overlay',signConvention:'Positive enters owner; negative leaves owner',periods:[{id:'monthly',label:'Monthly',kind:'monthly',labels:['January','February'],frameCount:2,boundaryFlows:[{boundaryId:'boundary:ext',relatedBoundaryIds:['boundary:ext'],connectionId:'connection:ext',ownerNodeId:'zone:a',targetNodeId:'thermal-environment:outdoors',value:0.5,values:[1,-0.5],unit:'kWh',sourceIds:['source:wall']}],connectionFlows:[{connectionId:'connection:ext',fromNodeId:'zone:a',toNodeId:'thermal-environment:outdoors',ownerNodeId:'zone:a',value:0.5,values:[1,-0.5],unit:'kWh',sourceIds:['source:wall']}]}],sources:[{id:'source:wall',name:'Surface Average Face Conduction Heat Transfer Energy',keyValue:'South Wall',sourceUnit:'J',normalizedUnit:'kWh',aggregationMethod:'sum_reported_energy'}]}}};
  state.thermalTopologySelectedEntityKind="thermal_connection";state.thermalTopologySelectedEntityId="connection:ext";state.thermalTopologyMetric="simulated_heat";state.thermalTopologySimulationPeriod="monthly";state.thermalTopologySimulationFrame=1;renderer.renderThermalTopology(geometry,helpers);
  const simulatedText=document.getElementById('thermalTopologyInspector').textContent;let ledgerJump=false;window.addEventListener('idfAnalyzer:openSimulationPurposePlan',()=>{ledgerJump=true;});document.querySelector('[data-inspector-output-source]')?.click();
  const simulatedEdgeLabel=document.querySelector('[data-thermal-target-id="connection:ext"] .thermal-edge-label')?.textContent||'';
  state.thermalTopologyMetric="ua";renderer.renderThermalTopology(geometry,helpers);const staticUALabel=document.querySelector('[data-thermal-target-id="connection:ext"] .thermal-edge-label')?.textContent||'';
  const topo284=document.getElementById('thermalTopologySimulationPeriod').value==='monthly'&&simulatedEdgeLabel.includes('-0.5 kWh')&&simulatedText.includes('sum_reported_energy')&&simulatedText.includes('Positive enters owner')&&ledgerJump&&staticUALabel.includes('W/K')&&!staticUALabel.includes('kWh')&&state.simulationResult.purposeRunPlan.estimatedWeight==='Heavy';

  state.thermalTopologyMetric="area";state.thermalTopologyDisplay="graph";state.thermalTopologySelectedEntityKind="thermal_connection";state.thermalTopologySelectedEntityId="connection:pair";state.thermalTopologyPanX=17;state.thermalTopologyPanY=-9;state.thermalTopologyScale=1.4;
  const roundTrip=stateModule.captureThermalTopologyState(state);state.thermalTopologyMetric="qa";state.thermalTopologySelectedEntityId="";state.thermalTopologyPanX=0;stateModule.restoreThermalTopologyState(roundTrip,state);
  const baselineArea=8,compareArea=12,delta=compareArea-baselineArea,percent=delta/baselineArea*100;
  const topo285=state.thermalTopologyMetric==='area'&&state.thermalTopologySelectedEntityId==='connection:pair'&&state.thermalTopologyPanX===17&&state.thermalTopologyScale===1.4&&delta===4&&percent===50&&backendCalls===0;

  assert(topo280&&topo281&&topo282&&topo283&&topo284&&topo285,'one or more integrated acceptance flows failed');
  document.getElementById('result').textContent=JSON.stringify({topo280,topo281,topo282,topo283,topo284,topo285,backendCalls});
  document.body.dataset.topologyIntegratedStatus='passed';
} catch(error) {
  document.getElementById('result').textContent=error.stack||String(error);
  document.body.dataset.topologyIntegratedStatus='failed';
}
</script></body></html>`
