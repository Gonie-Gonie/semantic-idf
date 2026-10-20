package frontendchecks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Use the multi-manager water coil that exposed tangled return links, plus
// other real equipment families. Native identities and signs remain authoritative
// while the display routes dependencies around cards and permits tracing.
func TestHVACParameterFlowTraceBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("parsed HVAC parameter flow geometry and interaction acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	input, err := os.ReadFile(repoPath("frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := idf.Parse(string(input))
	if err != nil {
		t.Fatal(err)
	}
	report := idf.AnalyzeHVAC(doc)
	loopTypes := map[string]string{}
	for _, loop := range report.Loops {
		loopTypes[loop.ID] = loop.Type
	}
	fixtures := []any{}
	for _, family := range []struct{ name, objectType, objectName, loopType string }{
		{"coil", "Coil:Heating:Water", "VAV_5_HeatC", "AirLoopHVAC"},
		{"fan", "Fan:VariableVolume", "VAV_5_Fan", "AirLoopHVAC"},
		{"chiller", "Chiller:", "", "PlantLoop"},
		{"pump", "Pump:", "", "PlantLoop"},
		{"boiler", "Boiler:", "", "PlantLoop"},
	} {
		found := false
		for index := range report.OperatingProfiles {
			profile := &report.OperatingProfiles[index]
			if profile.ComponentIndex < 0 || profile.ComponentIndex >= len(doc.Objects) || loopTypes[profile.LoopID] != family.loopType {
				continue
			}
			object := doc.Objects[profile.ComponentIndex]
			if !strings.HasPrefix(object.Type, family.objectType) || len(object.Fields) == 0 || family.objectName != "" && object.Fields[0].Value != family.objectName {
				continue
			}
			if len(profile.ParameterControl.Edges) == 0 {
				t.Fatalf("bundled %s parameter graph is missing", family.name)
			}
			component := idf.HVACOperatingSource{ObjectIndex: profile.ComponentIndex, ObjectType: object.Type, ObjectName: object.Fields[0].Value}
			fixtures = append(fixtures, map[string]any{"family": family.name, "profile": profile, "component": component})
			t.Logf("parsed %s: %s, %d native nodes / %d native relationships", family.name, component.ObjectName, len(profile.ParameterControl.Nodes), len(profile.ParameterControl.Edges))
			if family.name == "chiller" {
				// Reparse an owned document before changing one curve reference. The
				// browser must preserve the parser's real unresolved-reference node,
				// rather than a hand-written stand-in or altered bundled fixture.
				missingDoc, err := idf.Parse(string(input))
				if err != nil {
					t.Fatal(err)
				}
				curveName := ""
				for _, node := range profile.ParameterControl.Nodes {
					if node.Code == "performance_curve" {
						curveName = node.Source.ObjectName
						break
					}
				}
				if curveName == "" {
					t.Fatal("bundled chiller has no native performance curve")
				}
				replaced := false
				for field := range missingDoc.Objects[profile.ComponentIndex].Fields {
					if strings.EqualFold(missingDoc.Objects[profile.ComponentIndex].Fields[field].Value, curveName) {
						missingDoc.Objects[profile.ComponentIndex].Fields[field].Value = "Undefined Performance Curve"
						replaced = true
						break
					}
				}
				if !replaced {
					t.Fatal("bundled chiller curve is not referenced by its native equipment fields")
				}
				missingReport := idf.AnalyzeHVAC(missingDoc)
				var missingProfile *idf.HVACOperatingProfile
				for candidate := range missingReport.OperatingProfiles {
					item := &missingReport.OperatingProfiles[candidate]
					if item.ComponentIndex == profile.ComponentIndex && item.LoopID == profile.LoopID {
						missingProfile = item
						break
					}
				}
				if missingProfile == nil {
					t.Fatal("missing-curve chiller profile disappeared")
				}
				unresolved := false
				for _, node := range missingProfile.ParameterControl.Nodes {
					if node.Kind == "unknown" && node.Code == "unresolved_curve_reference" {
						unresolved = true
					}
				}
				if !unresolved {
					t.Fatal("missing curve fixture did not produce native unresolved-reference evidence")
				}
				fixtures = append(fixtures, map[string]any{"family": "chiller-missing-curve", "profile": missingProfile, "component": component})
			}
			found = true
			break
		}
		if !found {
			t.Fatalf("bundled %s operating profile is missing", family.name)
		}
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/fixture", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fixtures)
	})
	mux.HandleFunc("/parameter-flow", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, hvacParameterFlowTraceBrowserHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	browser := epath201FreshBrowser(t, ctx, chrome)
	browser.call("Page.enable", map[string]any{}, nil)
	browser.call("Emulation.setFocusEmulationEnabled", map[string]any{"enabled": true}, nil)
	// Native ResizeObserver frames must complete before inspecting text bounds;
	// a virtual-time DOM dump may finish before a pending layout frame arrives.
	runBrowser := func(t *testing.T, query string, width, height int) {
		t.Helper()
		browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": width, "height": height, "deviceScaleFactor": 1, "mobile": false}, nil)
		// A navigation response can precede document replacement. Only the new
		// page may report completion, including when reloading the same URL.
		browser.evaluate(`if(document.body) document.body.dataset.parameterFlowStatus='navigating'`)
		browser.call("Page.navigate", map[string]any{"url": server.URL + "/parameter-flow" + query}, nil)
		browser.call("Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": 0, "y": 0}, nil)
		for deadline := time.Now().Add(20 * time.Second); ; {
			status := string(browser.evaluate(`document.body?.dataset.parameterFlowStatus`))
			if status == `"passed"` {
				return
			}
			if status == `"failed"` || time.Now().After(deadline) {
				t.Fatalf("parameter flow trace acceptance %s: %s", status, browser.evaluate(`document.getElementById('result')?.textContent`))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	for _, width := range []int{1480, 3200, 690, 400} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			runBrowser(t, fmt.Sprintf("?width=%d", width), width, 1200)
		})
	}
	if captureDir := os.Getenv("HVAC_PARAMETER_CAPTURE_DIR"); captureDir != "" {
		if err := os.MkdirAll(captureDir, 0755); err != nil {
			t.Fatal(err)
		}
		for _, capture := range []struct {
			name, query   string
			width, height int
		}{
			{"coil-wide.png", "?width=1480&capture=overview", 1480, 1100},
			{"coil-expanded.png", "?width=2400&capture=overview", 2400, 1100},
			{"coil-setpoint.png", "?width=1480&capture=setpoint", 1480, 1100},
			{"coil-narrow.png", "?width=400&capture=overview", 400, 2000},
			{"coil-light.png", "?width=1480&capture=overview&theme=light", 1480, 1100},
			{"fan-wide.png", "?width=1480&capture=overview&family=fan", 1480, 1100},
			{"chiller-wide.png", "?width=1480&capture=overview&family=chiller", 1480, 1100},
			{"chiller-performance.png", "?width=1480&capture=performance&family=chiller", 1480, 2000},
			{"chiller-performance-ko-light.png", "?width=1480&capture=performance&family=chiller&language=ko&theme=light", 1480, 2000},
			{"chiller-missing-curve.png", "?width=1480&capture=performance&family=chiller-missing-curve", 1480, 2000},
		} {
			path, err := filepath.Abs(filepath.Join(captureDir, capture.name))
			if err != nil {
				t.Fatal(err)
			}
			runBrowser(t, capture.query, capture.width, capture.height)
			var screenshot struct {
				Data string `json:"data"`
			}
			browser.call("Page.captureScreenshot", map[string]any{"format": "png", "fromSurface": true, "captureBeyondViewport": false}, &screenshot)
			data, err := base64.StdEncoding.DecodeString(screenshot.Data)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			t.Logf("parameter flow visual review: %s", path)
		}
	}
}

const hvacParameterFlowTraceBrowserHTML = `<!doctype html>
<html lang="en" data-theme="dark"><head><meta charset="utf-8"><title>Water-coil parameter tracing</title>
<link rel="stylesheet" href="/src/styles/base.css"><link rel="stylesheet" href="/src/styles/hvac.css">
<style>body{display:block;margin:0;padding:16px;overflow:auto;height:auto;box-sizing:border-box}#host{width:100%;min-width:0}#result{display:none}</style>
</head><body data-parameter-flow-status="pending"><section id="host" class="hvac-operation-panel parameters"></section><pre id="result">pending</pre>
<script type="module">
const check=(value,message)=>{if(!value)throw Error(message)};
const settle=()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
try{
 const [{buildHVACControlExpressions:build},{renderHVACParameterFlow:render,connectHVACParameterFlow:connect},{setLanguage}]=await Promise.all([
  import('/src/js/hvac-control-expressions.js'),import('/src/js/views/hvac-parameter-flow-view.js'),import('/src/js/i18n.js')]);
 const fixtures=await(await fetch('/fixture')).json(),original=JSON.stringify(fixtures),host=document.querySelector('#host');
 // Preserve the requested content width inside the page's outer padding.
 host.style.maxWidth=(Number(new URLSearchParams(location.search).get('width'))-32)+'px';
 let disconnect;
 const renderFlow=async(language,fixture,model)=>{disconnect?.();setLanguage(language);host.innerHTML=render(model,{component:fixture.component});disconnect=connect(host);await settle();};
 const cardID=card=>card.getAttribute('data-hvac-flow-card');
 const feedbackRelations=new Set(['feedback','adds_measurement']);
 const setpointRelations=new Set(['references_setpoint','sets_target','subtracts_target']);
 const flowKind=relation=>feedbackRelations.has(relation)?'feedback':relation==='measures'?'measurement':setpointRelations.has(relation)?'setpoint':'signal';
 const flowKinds=['signal','setpoint','measurement','feedback'];
 const edgeIdentity=edge=>JSON.stringify([edge.dataset.from,edge.dataset.to,edge.dataset.relation,edge.dataset.polarity||'',edge.dataset.condition||'']);
 const inspect=()=>host.querySelector('[data-hvac-parameter-inspector]');
 const near=(left,right)=>Math.abs(left-right)<.2;
 const assertCardLayout=()=>{
  const svg=host.querySelector('[data-hvac-parameter-flow]'),cards=[...svg.querySelectorAll('[data-hvac-flow-card]')];
  const rects=cards.map(card=>({card,x:Number(card.getAttribute('x')),y:Number(card.getAttribute('y')),w:Number(card.getAttribute('width')),h:Number(card.getAttribute('height'))}));
  check(Math.max(...rects.map(rect=>rect.w))-Math.min(...rects.map(rect=>rect.w))<.2,'cards no longer share one diagram-wide width');
  check(rects.every(rect=>rect.w>0&&rect.w<=360.2),'cards keep stretching beyond a readable content width');
  for(const {card} of rects){
   const button=card.querySelector('button');
   check(button.scrollWidth<=button.clientWidth+1&&button.scrollHeight<=button.clientHeight+1,'card content exceeds its measured button height/width: '+cardID(card));
   check(Math.abs(button.getBoundingClientRect().height-card.getBoundingClientRect().height)<1,'visible button and SVG endpoint boundary disagree after text measurement/resize');
   for(const text of button.querySelectorAll('strong,span')){
    const style=getComputedStyle(text);
    check(style.textOverflow!=='ellipsis'&&['none','0',''].includes(style.webkitLineClamp),'card still truncates its full label/source: '+text.textContent);
    check(text.scrollWidth<=text.clientWidth+1&&text.scrollHeight<=text.clientHeight+1,'label/source text is clipped: '+text.textContent);
   }
  }
  for(let first=0;first<rects.length;first++)for(let second=first+1;second<rects.length;second++){
   const a=rects[first],b=rects[second];
   check(!(a.x<b.x+b.w-.5&&b.x<a.x+a.w-.5&&a.y<b.y+b.h-.5&&b.y<a.y+a.h-.5),'expanded card text overlaps another card');
  }
  check(host.scrollWidth<=host.clientWidth+1&&document.documentElement.scrollWidth<=innerWidth+1,'wrapped parameter diagram overflows its viewport');
  const columns=[...new Set(rects.map(rect=>rect.x))].sort((a,b)=>a-b),cardWidth=rects[0].w;
  return{cardWidth,left:Math.min(...rects.map(rect=>rect.x)),right:svg.viewBox.baseVal.width-Math.max(...rects.map(rect=>rect.x+rect.w)),gap:columns.length>1?Math.min(...columns.slice(1).map((x,index)=>x-columns[index]-cardWidth)):0};
 };
 const assertResizedRoutes=()=>{
  const svg=host.querySelector('[data-hvac-parameter-flow]'),rects=[...svg.querySelectorAll('[data-hvac-flow-card]')].map(card=>({id:cardID(card),x:Number(card.getAttribute('x')),y:Number(card.getAttribute('y')),w:Number(card.getAttribute('width')),h:Number(card.getAttribute('height'))}));
  for(const edge of svg.querySelectorAll('[data-hvac-flow-edge]')){
   const path=edge.querySelector('.hvac-parameter-wire'),length=path.getTotalLength();
   check(length>0,'text measurement/resize collapsed a dependency');
   for(let distance=0;distance<=length;distance+=3){
    const point=path.getPointAtLength(distance);
    for(const rect of rects){
     if([edge.dataset.fromCard,edge.dataset.toCard].includes(rect.id))continue;
     check(!(point.x>rect.x+.7&&point.x<rect.x+rect.w-.7&&point.y>rect.y+.7&&point.y<rect.y+rect.h-.7),'resized route crosses expanded text card '+rect.id);
    }
    check(point.x>=-.2&&point.y>=-.2&&point.x<=svg.viewBox.baseVal.width+.2&&point.y<=svg.viewBox.baseVal.height+.2,'text measurement/resize routed a dependency outside the SVG');
   }
  }
 };
 const assertWideResize=async()=>{
  if(innerWidth<3100)return;
  const snapshots=[];
  for(const width of [1448,2368,3168]){
   host.style.maxWidth=width+'px';
   // ResizeObserver delivery follows rendering frames; the SVG and visible
   // host must share a coordinate frame before measuring cards and paths.
   for(let attempt=0;attempt<20;attempt++){
    await settle();
    const shell=host.querySelector('[data-hvac-parameter-flow-shell]'),svg=shell.querySelector('svg');
    if(Math.abs(svg.viewBox.baseVal.width-Math.round(shell.getBoundingClientRect().width))<1)break;
   }
   const shell=host.querySelector('[data-hvac-parameter-flow-shell]');
   check(Math.abs(shell.querySelector('svg').viewBox.baseVal.width-Math.round(shell.getBoundingClientRect().width))<1,'resize observer did not update the diagram coordinate frame');
   snapshots.push(assertCardLayout());assertResizedRoutes();
  }
  check(near(snapshots[1].cardWidth,snapshots[2].cardWidth),'cards still expand after their common content width is satisfied');
  for(let index=1;index<snapshots.length;index++){
   const before=snapshots[index-1],after=snapshots[index];
   check(after.left>before.left+20&&after.right>before.right+20&&after.gap>before.gap+20,'extra viewport width was not distributed to both outer margins and arrow gutters: '+JSON.stringify(snapshots));
  }
 };
 const isCurve=node=>['performance_curve','unresolved_curve_reference'].includes(node.code);
 const assertNativeMember=(node,member)=>{
  check(member&&member.dataset.hvacNodeKind===node.kind&&member.dataset.hvacSourceName===node.source.objectName&&member.dataset.hvacSourceType===node.source.objectType&&member.dataset.hvacSourceIndex===String(node.source.objectIndex),'source inspector changed native node identity: '+node.id);
  const fields=[...member.querySelectorAll('[data-control-parameter]')];
  check(fields.length===(node.parameters||[]).length,'source inspector dropped or invented native parameter fields: '+node.id);
  for(const parameter of node.parameters||[])check(fields.some(field=>field.dataset.controlParameter===parameter.name&&field.dataset.controlValue===parameter.value&&field.dataset.controlSourceIndex===String(parameter.objectIndex)&&field.dataset.controlFieldIndex===String(parameter.fieldIndex)),'source inspector changed exact native parameter or field identity: '+node.id+' / '+parameter.name);
 };
 const assertPerformanceModels=(fixture,model,language)=>{
  const native=fixture.profile.parameterControl,allCards=model.groups.flatMap(group=>group.cards),curves=native.nodes.filter(isCurve),curveIDs=new Set(curves.map(node=>node.id));
  check(JSON.stringify(model.nodes)===JSON.stringify(native.nodes)&&JSON.stringify(model.relationships)===JSON.stringify(native.edges),'presentation removed native performance evidence instead of separating its display');
  check(allCards.every(card=>card.members.every(node=>!curveIDs.has(node.id))),'a native performance curve still occupies a main control-flow card');
  check([...host.querySelectorAll('[data-hvac-flow-edge]')].every(edge=>edge.dataset.relation!=='modifies_performance'&&!curveIDs.has(edge.dataset.from)&&!curveIDs.has(edge.dataset.to)),'performance-model dependency is still drawn as a command/control wire');
  if(fixture.family==='chiller')check(curves.length>=3,'real chiller no longer exercises separate capacity/temperature/part-load performance curves');
  const byTarget=new Map();
  for(const node of curves){
   const incident=native.edges.filter(edge=>edge.from===node.id||edge.to===node.id);
   check(incident.length>0&&incident.every(edge=>edge.from===node.id&&edge.relation==='modifies_performance'),'parsed curve fixture no longer has model-only native dependencies');
   for(const edge of incident){
    const card=allCards.find(card=>card.members.some(member=>member.id===edge.to));
    check(card&&model.groups.find(group=>group.cards.includes(card)).id==='response','performance curve is not associated with a declared equipment-response owner');
    const entries=byTarget.get(card.id)||[];entries.push({node,edge});byTarget.set(card.id,entries);
   }
  }
  for(const card of allCards){
   const entries=byTarget.get(card.id)||[],models=card.performanceModels||[];
   check(models.length===new Set(entries.map(entry=>entry.node.id)).size,'performance model count does not match the original response owner: '+card.id);
   if(!entries.length)continue;
   for(const {node,edge} of entries)check(models.some(model=>model.node===node&&model.relationships.includes(edge)),'performance association copied or changed its original native curve/relationship references');
   host.querySelector('[data-hvac-inspect-parameter="'+CSS.escape(card.id)+'"]').click();
   const section=inspect().querySelector('[data-hvac-performance-model]');
   check(section&&section.textContent.includes(language==='ko'?'\uc131\ub2a5 \ubaa8\ub378':'Performance model'),'response inspector lacks its localized performance-model section');
   const expectedNodes=[...new Map(entries.map(entry=>[entry.node.id,entry.node])).values()],members=[...section.querySelectorAll('[data-hvac-control-node]')];
   check(members.length===expectedNodes.length&&new Set(members.map(member=>member.dataset.hvacControlNode)).size===expectedNodes.length,'distinct curve definitions were merged, duplicated or assigned to the wrong response');
   for(const node of expectedNodes){
    const member=members.find(member=>member.dataset.hvacControlNode===node.id);assertNativeMember(node,member);
    if(node.code==='unresolved_curve_reference')check(member.textContent.includes(language==='ko'?'\uc131\ub2a5 \uace1\uc120 \ucc38\uc870 \ubbf8\ud655\uc778':'Performance curve reference unresolved')&&member.textContent.includes('Undefined Performance Curve'),'missing curve reference is concealed or presented as a resolved performance definition');
   }
   const relations=[...section.querySelectorAll('[data-hvac-parameter-term]')];
   check(relations.length===entries.length,'performance inspector dropped or invented a native equipment-model dependency');
   for(const {edge} of entries)check(relations.some(relation=>relation.dataset.from===edge.from&&relation.dataset.to===edge.to&&relation.dataset.relation===edge.relation&&(relation.dataset.polarity||'')===(edge.polarity||'')&&(relation.dataset.condition||'')===(edge.condition||'')),'performance inspector changed the original dependency endpoint/relation/sign');
   inspect().querySelector('[data-hvac-close-parameter]').click();
  }
 };
 for(const fixture of fixtures){
 const native=fixture.profile.parameterControl,model=build(fixture.profile,{component:fixture.component}).parameters;
 const nativeCard=new Map(model.groups.flatMap(group=>group.cards.flatMap(card=>card.members.map(node=>[node.id,card.id]))));
 document.body.dataset.family=fixture.family;
 if(fixture.family==='coil')check(model.groups.length===4&&model.groups.find(group=>group.id==='inputs').cards.length>=4,'real multi-manager coil fixture no longer exercises stacked targets');
 for(const language of ['en','ko']){
  await renderFlow(language,fixture,model);
  const shell=host.querySelector('[data-hvac-parameter-flow-shell]'),svg=host.querySelector('[data-hvac-parameter-flow]');
  const cards=[...svg.querySelectorAll('[data-hvac-flow-card]')],edges=[...svg.querySelectorAll('[data-hvac-flow-edge]')];
  const legend=[...shell.querySelectorAll('.hvac-parameter-flow-legend > span')];
  const labels=language==='ko'?['신호','설정값 / 목표값','측정값 입력','제어 피드백']:['Signal','Setpoint / target','Measured input','Control feedback'];
  check(legend.length===flowKinds.length&&legend.every((item,index)=>item.classList.contains(flowKinds[index])&&item.textContent.trim()===labels[index]&&item.title.trim()),'legend does not explain the four source-backed signal meanings');
  assertCardLayout();
  const rects=new Map(cards.map(card=>[cardID(card),{x:Number(card.getAttribute('x')),y:Number(card.getAttribute('y')),w:Number(card.getAttribute('width')),h:Number(card.getAttribute('height'))}]));
  check(edges.length>0,'parsed '+fixture.family+' lost visible dependency links');
  if(fixture.family==='coil')check(edges.length>=8,'multi-manager coil lost visible dependency links');
  check(!shell.hasAttribute('data-flow-focus')&&inspect().hidden,'initial diagram starts with selected-path clutter');
  assertPerformanceModels(fixture,model,language);
  check(host.scrollWidth<=host.clientWidth+1&&document.documentElement.scrollWidth<=innerWidth+1,'parameter diagram overflowed viewport at '+innerWidth);
  const ports=new Map(),longLanes=[],physicalLanes=[];
  for(const edge of edges){
   const wire=edge.querySelector('.hvac-parameter-wire'),hit=edge.querySelector('.hvac-parameter-hit');
   check(wire&&hit&&wire.getAttribute('d')===hit.getAttribute('d'),'visible and accessible edge hit paths diverged');
   check(edge.dataset.flowRoute,'edge has no explicit routed path classification');
   check(edge.getAttribute('tabindex')==='0'&&edge.getAttribute('aria-label'),'edge cannot be traced directly with keyboard');
   const matching=native.edges.find(item=>item.from===edge.dataset.from&&item.to===edge.dataset.to&&item.relation===edge.dataset.relation&&item.polarity===(edge.dataset.polarity||undefined)&&item.condition===(edge.dataset.condition||undefined));
   check(matching,'display edge invented a native relation or changed its exact sign');
   check(nativeCard.get(matching.from)===edge.dataset.fromCard&&nativeCard.get(matching.to)===edge.dataset.toCard,'native edge attached to wrong source card');
   const kind=flowKind(matching.relation);
   check(edge.dataset.flowKind===kind&&flowKinds.every(item=>edge.classList.contains(item)===(item===kind)),'native relationship meaning was inferred from geometry or conflated with another category: '+matching.relation);
   const from=rects.get(edge.dataset.fromCard),to=rects.get(edge.dataset.toCard),length=wire.getTotalLength();
   check(length>0,'edge route collapsed');
   for(let distance=0;distance<=length;distance+=2){
    const point=wire.getPointAtLength(distance);
    for(const [id,rect] of rects){
     if(id===edge.dataset.fromCard||id===edge.dataset.toCard)continue;
     check(!(point.x>rect.x+.7&&point.x<rect.x+rect.w-.7&&point.y>rect.y+.7&&point.y<rect.y+rect.h-.7),'route crosses unrelated card '+id+' via '+edge.dataset.from+' -> '+edge.dataset.to+' at '+point.x+','+point.y);
    }
    check(point.x>=-.2&&point.y>=-.2&&point.x<=svg.viewBox.baseVal.width+.2&&point.y<=svg.viewBox.baseVal.height+.2,'route escaped the visible SVG');
   }
   for(const [id,rect,distance] of [[edge.dataset.fromCard,from,0],[edge.dataset.toCard,to,length]]){
    const point=wire.getPointAtLength(distance),onVertical=(near(point.x,rect.x)||near(point.x,rect.x+rect.w))&&point.y>=rect.y-.2&&point.y<=rect.y+rect.h+.2,onHorizontal=(near(point.y,rect.y)||near(point.y,rect.y+rect.h))&&point.x>=rect.x-.2&&point.x<=rect.x+rect.w+.2;
    check(onVertical||onHorizontal,'edge endpoint does not meet its actual card boundary');
    const key=id+'|'+point.x.toFixed(1)+'|'+point.y.toFixed(1);
    check(!ports.has(key),'multiple dependencies share one ambiguous card port: '+key);ports.set(key,edge);
    check([...edge.querySelectorAll('circle')].some(circle=>near(Number(circle.getAttribute('cx')),point.x)&&near(Number(circle.getAttribute('cy')),point.y)),'edge endpoint lacks a visible connection marker');
   }
   if(!['adjacent','adjacent-return'].includes(edge.dataset.flowRoute)){
    check(edge.dataset.flowLane!==undefined,'long/return dependency has no separate routing lane');longLanes.push(edge.dataset.flowLane);
    const bounds=wire.getBBox(),route=edge.dataset.flowRoute,coordinate=route==='bypass'?bounds.y:route==='return'?bounds.y+bounds.height:route==='vertical'?bounds.x+bounds.width:bounds.x;
    physicalLanes.push(route+'|'+coordinate.toFixed(1));
   }
   const sign=matching.polarity==='positive'?'+':matching.polarity==='negative'?'\u2212':matching.polarity==='unknown'?'?':'';
   check(edge.querySelector('text')?.textContent===sign,'native comparator sign was lost or reinterpreted');
  }
  check(new Set(longLanes).size===longLanes.length,'long/return links overlap the same perimeter lane');
  if(fixture.family==='coil')check(longLanes.length>=4,'multi-manager coil no longer exercises several long/return lanes');
  check(new Set(physicalLanes).size===physicalLanes.length,'different lane IDs conceal coincident long/return path geometry');
  const measuredFeedback=edges.find(edge=>edge.dataset.relation==='adds_measurement'&&edge.dataset.polarity==='positive'),setpointReference=edges.find(edge=>edge.dataset.relation==='references_setpoint'),measuredInput=edges.find(edge=>edge.dataset.relation==='measures');
  if(fixture.family==='coil'){
   check(measuredFeedback&&setpointReference&&measuredInput,'real coil no longer tests separate control feedback, setpoint reference and measured input semantics');
   const samples=flowKinds.map(kind=>edges.find(edge=>edge.dataset.flowKind===kind));
   check(samples.every(Boolean),'real coil no longer exercises all four native relationship categories');
   const styles=samples.map(edge=>getComputedStyle(edge.querySelector('.hvac-parameter-wire')));
   check(new Set(styles.map(style=>style.stroke+'|'+style.strokeDasharray)).size===flowKinds.length,'different native relationship categories lack distinct visible styling');
  }
  // Reorder only an owned presentation clone. Connection meaning must remain
  // native even when the same endpoints move to the opposite screen direction.
  const reordered=JSON.parse(JSON.stringify(model));reordered.groups.reverse();
  const reorderedOriginal=JSON.stringify(reordered),reorderedHost=document.createElement('div');
  reorderedHost.innerHTML=render(reordered,{component:fixture.component});
  const reorderedEdges=[...reorderedHost.querySelectorAll('[data-hvac-flow-edge]')],originalEdges=new Map(edges.map(edge=>[edgeIdentity(edge),edge]));
  check(reorderedEdges.length===edges.length,'changing presentation order removed or invented native relationships');
  for(const edge of reorderedEdges){
   const originalEdge=originalEdges.get(edgeIdentity(edge)),kind=flowKind(edge.dataset.relation);
   check(originalEdge&&edge.dataset.flowKind===originalEdge.dataset.flowKind&&edge.dataset.flowKind===kind&&edge.classList.contains(kind),'screen direction changed a native relationship category');
  }
  check(JSON.stringify(reordered)===reorderedOriginal,'classifying relationships mutated the owned presentation clone');
  const feedback=measuredFeedback||edges[0],reference=setpointReference||measuredInput||edges.at(-1);
  const relatedCards=edge=>cards.filter(card=>[edge.dataset.fromCard,edge.dataset.toCard].includes(cardID(card)));
  const opacity=element=>{let value=1;for(let item=element;item&&item!==shell;item=item.parentElement)value*=Number(getComputedStyle(item).opacity);return value;};
  const assertEdgeFocus=async edge=>{
   await settle();
   check(shell.dataset.flowFocus==='edge'&&edge.classList.contains('is-highlighted'),'edge hover/focus did not isolate its path: '+edge.dataset.from+' -> '+edge.dataset.to+' / '+shell.dataset.flowFocus+' / active '+document.activeElement?.outerHTML?.slice(0,200));
   check(edges.filter(item=>item.classList.contains('is-highlighted')).length===1,'edge trace highlighted unrelated paths');
   check(cards.filter(card=>card.classList.contains('is-related')).length===relatedCards(edge).length,'edge trace did not isolate exact endpoint cards');
   check(cards.find(card=>cardID(card)===edge.dataset.fromCard).classList.contains('is-source')&&cards.find(card=>cardID(card)===edge.dataset.toCard).classList.contains('is-target'),'edge trace lacks directional endpoint emphasis');
   const description=shell.querySelector('[data-hvac-flow-description]');
   check(description&&!description.hidden&&description.textContent.trim()&&description.textContent.includes('\u2192'),'traced edge has no readable endpoint/relation description');
   check(description.textContent.startsWith(labels[flowKinds.indexOf(edge.dataset.flowKind)])&&edge.querySelector('title').textContent===edge.getAttribute('aria-label'),'trace tooltip/keyboard description omits or changes the native relationship category');
   if(edges.length>1)check(opacity(edges.find(item=>item!==edge).querySelector('.hvac-parameter-wire'))<opacity(edge.querySelector('.hvac-parameter-wire')),'unrelated paths are not visually subdued');
  };
  const selected=cards.find(card=>card.querySelector('button')&&model.groups.flatMap(group=>group.cards).find(item=>item.id===cardID(card)).members.some(node=>node.kind==='controller'))||cards[0],button=selected.querySelector('button');
  button.click();check(!inspect().hidden&&button.getAttribute('aria-expanded')==='true','card click lost native source inspector');
  const inspectorBefore=inspect().innerHTML,selectedModel=model.groups.flatMap(group=>group.cards).find(item=>item.id===cardID(selected));
  for(const node of selectedModel.members){
   const member=inspect().querySelector('[data-hvac-control-node="'+CSS.escape(node.id)+'"]');
   check(member&&member.dataset.hvacSourceName===node.source.objectName&&member.dataset.hvacSourceType===node.source.objectType&&member.dataset.hvacSourceIndex===String(node.source.objectIndex),'selected source inspector changed native node identity');
   for(const parameter of node.parameters||[])check([...member.querySelectorAll('[data-control-parameter]')].some(field=>field.dataset.controlParameter===parameter.name&&field.dataset.controlValue===parameter.value&&field.dataset.controlSourceIndex===String(parameter.objectIndex)&&field.dataset.controlFieldIndex===String(parameter.fieldIndex)),'source inspector changed exact native parameter or field identity: '+node.id+' / '+parameter.name);
  }
  if(fixture.family==='coil')check(inspect().textContent.includes('Controller:WaterCoil')&&inspect().querySelector('[data-control-parameter="Action"]')&&inspect().querySelector('[data-control-parameter="Sensor Node Name"]')&&inspect().querySelector('[data-control-parameter="Actuator Node Name"]'),'source inspector lost exact controller I/O ports');
  feedback.dispatchEvent(new PointerEvent('pointerover',{bubbles:true,relatedTarget:svg}));await assertEdgeFocus(feedback);
  check(inspect().innerHTML===inspectorBefore,'temporary edge tracing replaced selected source details');
  feedback.dispatchEvent(new PointerEvent('pointerout',{bubbles:true,relatedTarget:svg}));
  check(shell.dataset.flowFocus==='card'&&inspect().innerHTML===inspectorBefore,'leaving an edge did not restore the selected-card trace and persistent inspector');
  button.dispatchEvent(new PointerEvent('pointerover',{bubbles:true,relatedTarget:svg}));
  check(shell.dataset.flowFocus==='card'&&selected.classList.contains('is-related'),'card hover did not isolate its incident links');
  check(edges.every(edge=>edge.classList.contains('is-highlighted')===[edge.dataset.fromCard,edge.dataset.toCard].includes(cardID(selected))),'card trace highlighted a nonincident dependency');
  button.dispatchEvent(new PointerEvent('pointerout',{bubbles:true,relatedTarget:svg}));
  reference.focus({preventScroll:true});await assertEdgeFocus(reference);
  check(document.activeElement===reference,'raising the traced edge discarded real keyboard focus');
  reference.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true}));
  check(shell.dataset.flowFocus==='card'&&inspect().innerHTML===inspectorBefore,'Escape did not restore selected-card trace without discarding native details');
  reference.blur();
  for(const key of ['Enter',' ']){
   reference.focus({preventScroll:true});reference.dispatchEvent(new KeyboardEvent('keydown',{key,bubbles:true,cancelable:true}));reference.blur();await settle();
   await assertEdgeFocus(reference);check(reference.getAttribute('aria-pressed')==='true','keyboard edge activation did not retain its trace');
   reference.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true}));
   check(shell.dataset.flowFocus==='card'&&reference.getAttribute('aria-pressed')==='false'&&inspect().innerHTML===inspectorBefore,'Escape did not release pinned keyboard trace while preserving native details');
  }
  inspect().querySelector('[data-hvac-close-parameter]').click();
  check(inspect().hidden&&button.getAttribute('aria-expanded')==='false','closing native source inspector left stale selection');
  await assertWideResize();
  check(JSON.stringify(fixtures)===original,'routing/tracing mutated native parsed profile identities or signs');
 }
 }
 const chiller=fixtures.find(item=>item.family==='chiller');
 // An owned graph clone exercises one curve shared by distinct response
 // owners. It must appear in each proven owner's inspector, without changing
 // the original graph or assigning the curve to the control-command stage.
 const sharedProfile=JSON.parse(JSON.stringify(chiller.profile)),sharedGraph=sharedProfile.parameterControl;
 const sharedCurve=sharedGraph.nodes.find(isCurve),sharedEdge=sharedGraph.edges.find(edge=>edge.from===sharedCurve.id&&edge.relation==='modifies_performance');
 const secondary={...sharedGraph.nodes.find(node=>node.id===sharedEdge.to),id:'synthetic-secondary-response',source:{objectIndex:-42,objectType:'Synthetic:EquipmentResponse',objectName:'Secondary modeled response'},parameters:[]};
 sharedGraph.nodes.push(secondary);sharedGraph.edges.push({from:sharedCurve.id,to:secondary.id,relation:'modifies_performance',condition:'Synthetic response selection'});
 const sharedOriginal=JSON.stringify(sharedProfile),sharedFixture={family:'shared-performance',profile:sharedProfile,component:chiller.component},sharedModel=build(sharedProfile,{component:chiller.component}).parameters;
 for(const language of ['en','ko']){
  await renderFlow(language,sharedFixture,sharedModel);assertCardLayout();assertResizedRoutes();assertPerformanceModels(sharedFixture,sharedModel,language);
 }
 check(sharedModel.groups.flatMap(group=>group.cards).filter(card=>(card.performanceModels||[]).some(model=>model.node.id===sharedCurve.id)).length===2,'a shared curve was attached to only one response owner');
 check(JSON.stringify(sharedProfile)===sharedOriginal,'separating shared model dependencies mutated the owned native-shaped graph');
 // A curve without a proven response owner, or with an additional control
 // dependency, must remain explicit rather than disappearing into an unrelated
 // equipment inspector. Only the exclusive native model relationship is moved.
 for(const variant of ['orphan','mixed-role','unknown-owner']){
  const profile=JSON.parse(JSON.stringify(chiller.profile)),graph=profile.parameterControl,curve=graph.nodes.find(isCurve);
  if(variant==='orphan')graph.edges=graph.edges.filter(edge=>edge.from!==curve.id&&edge.to!==curve.id);
  if(variant==='mixed-role')graph.edges.push({from:curve.id,to:graph.nodes.find(node=>node.kind==='control_variable').id,relation:'sets_parameter'});
  if(variant==='unknown-owner')for(const edge of graph.edges)if(edge.from===curve.id)edge.to='unresolved-response-owner';
  const originalProfile=JSON.stringify(profile),model=build(profile,{component:chiller.component}).parameters,card=model.groups.flatMap(group=>group.cards).find(card=>card.members.some(node=>node.id===curve.id));
  check(card&&!model.groups.flatMap(group=>group.cards).some(card=>(card.performanceModels||[]).some(model=>model.node.id===curve.id)),'curve with '+variant+' ownership was silently hidden or assigned to an unproven response');
  await renderFlow('en',chiller,model);assertCardLayout();assertResizedRoutes();
  check(host.querySelector('[data-hvac-flow-card="'+CSS.escape(card.id)+'"]'),'curve with '+variant+' ownership disappeared from the main diagram');
  check(JSON.stringify(profile)===originalProfile,'classifying '+variant+' performance ownership changed the original graph');
 }
 const coil=fixtures.find(item=>item.family==='coil'),coilModel=build(coil.profile,{component:coil.component}).parameters;
 // An owned display-model clone exercises unusually long words and labels
 // without rewriting real source identities or the parsed fixture evidence.
 const longModel=JSON.parse(JSON.stringify(coilModel)),longCard=longModel.groups.find(group=>group.id==='controls').cards[0];
 const longLabel='Detailed temperature comparison and controller operating condition with independently supplied reference and measured feedback';
 const longSource='ControllerSource_WithAnUnusuallyLongUnbrokenEquipmentAndCircuitReference_'.repeat(3);
 longCard.source.objectName=longSource;
 for(const node of longCard.members){
  node.code=longLabel;node.source.objectName=longSource;
  const native=longModel.nodes.find(item=>item.id===node.id);native.code=longLabel;native.source.objectName=longSource;
 }
 const longOriginal=JSON.stringify(longModel);
 await renderFlow('en',coil,longModel);assertCardLayout();assertResizedRoutes();await assertWideResize();
 const expanded=host.querySelector('[data-hvac-flow-card="'+CSS.escape(longCard.id)+'"]'),expandedButton=expanded.querySelector('button');
 check(Number(expanded.getAttribute('height'))>100,'long source text did not expand the card height');
 check(expandedButton.querySelector('strong').textContent.includes(longLabel)&&expandedButton.querySelector('span').textContent===longSource,'wrapping shortened or rewrote full title/source text');
 check(JSON.stringify(longModel)===longOriginal&&JSON.stringify(fixtures)===original,'text measurement mutated the owned display model or native profiles');
 document.documentElement.dataset.theme='light';await renderFlow('en',coil,coilModel);
 const luminance=color=>{
  const rgb=(color.match(/[\d.]+/g)||[]).slice(0,3).map(value=>{const channel=Number(value)/255;return channel<=.04045?channel/12.92:((channel+.055)/1.055)**2.4;});
  return rgb[0]*.2126+rgb[1]*.7152+rgb[2]*.0722;
 };
 const contrast=(first,second)=>{const values=[luminance(first),luminance(second)].sort((left,right)=>right-left);return(values[0]+.05)/(values[1]+.05);};
 const surface=getComputedStyle(host).backgroundColor;
 for(const button of host.querySelectorAll('[data-hvac-inspect-parameter]')){
  const style=getComputedStyle(button);
  check(contrast(style.color,style.backgroundColor)>=4.5,'light theme makes source tile text unreadable');
  check(contrast(getComputedStyle(button.querySelector('span')).color,style.backgroundColor)>=4.5,'light theme makes source identity too faint');
 }
 const lightKinds=new Set();
 for(const edge of host.querySelectorAll('[data-hvac-flow-edge]')){
  lightKinds.add(edge.dataset.flowKind);
  check(contrast(getComputedStyle(edge.querySelector('.hvac-parameter-wire')).stroke,surface)>=3,'light theme makes a '+edge.dataset.flowKind+' path too faint');
 }
 check(flowKinds.every(kind=>lightKinds.has(kind)),'light-theme contrast acceptance missed a relationship category');
 document.documentElement.dataset.theme='dark';
 const capture=new URLSearchParams(location.search).get('capture');
 if(capture){
  const family=new URLSearchParams(location.search).get('family')||'coil',fixture=fixtures.find(item=>item.family===family),model=build(fixture.profile,{component:fixture.component}).parameters;
  document.documentElement.dataset.theme=new URLSearchParams(location.search).get('theme')||'dark';
  await renderFlow(new URLSearchParams(location.search).get('language')||'en',fixture,model);
  if(capture==='setpoint'){
   const edge=host.querySelector('[data-hvac-flow-edge].setpoint[data-relation="references_setpoint"]')||host.querySelector('[data-hvac-flow-edge].setpoint');
   edge.focus({preventScroll:true});edge.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));
  }
  if(capture==='performance'){
   const card=model.groups.flatMap(group=>group.cards).find(card=>card.performanceModels?.length);
   host.querySelector('[data-hvac-inspect-parameter="'+CSS.escape(card.id)+'"]').click();
   inspect().querySelector('[data-hvac-performance-model]').scrollIntoView({block:'start'});window.scrollBy(0,-16);await settle();
  }
 }else disconnect?.();
 document.body.dataset.parameterFlowStatus='passed';document.querySelector('#result').textContent='passed';
}catch(error){document.body.dataset.parameterFlowStatus='failed';document.querySelector('#result').textContent='Family '+document.body.dataset.family+': '+(error.stack||String(error))}
</script></body></html>`
