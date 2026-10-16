package frontendchecks

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestHVACControlExpressionModelBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("compact HVAC expression and signal relationship acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/expressions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, hvacControlExpressionModelHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/expressions").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-expression-status="passed"`) {
		failure := string(output)
		if _, result, ok := strings.Cut(failure, `<pre id="result">`); ok {
			message, _, _ := strings.Cut(result, `</pre>`)
			failure = html.UnescapeString(message)
		}
		t.Fatalf("HVAC control expression acceptance failed: %v: %s", err, failure)
	}
}

const hvacControlExpressionModelHTML = `<!doctype html><html><body data-expression-status="pending"><pre id="result"></pre><script type="module">
try {
 const {buildHVACControlExpressions:build}=await import('/src/js/hvac-control-expressions.js');
 const check=(condition,message)=>{if(!condition)throw Error(message);};
 const owner={objectIndex:1,objectType:'Fan:VariableVolume',objectName:'Supply fan'};
 const manager={objectIndex:2,objectType:'AvailabilityManager:Scheduled',objectName:'Loop manager'};
 const source=(objectIndex,objectType,objectName)=>({objectIndex,objectType,objectName});
 const positive=scheduleName=>({op:'schedule_positive',scheduleName,source:owner});
 const all=(...children)=>({op:'all',children}),any=(...children)=>({op:'any',children});
 const node=(id,kind,code,source=owner,parameters=[])=>({id,kind,code,source,parameters});
 const decision=node('decision','decision','fan_on_off');
 const demand={...node('demand','external_input','airflow_request'),boundary:'input'};
 const dispatch=node('dispatch','dispatch','zone_equipment_priority',source(3,'ZoneHVAC:EquipmentList','Zone list'));
 const thermostat=node('thermostat','controller','zone_thermostat',source(4,'ZoneControl:Thermostat','Thermostat'));
 const expression=all({op:'always'},all(positive('A'),any(positive('B'),positive('C'))));
 const profile={componentIndex:1,availabilityLayers:[{role:'equipment',owner}],finalAvailability:expression,
  onOffExpression:all(expression,{op:'runtime_condition',referenceId:'decision',source:owner}),
  onOff:{nodes:[decision,demand,dispatch,thermostat],edges:[{from:'demand',to:'decision',relation:'requests'},{from:'dispatch',to:'decision',relation:'dispatches'},{from:'thermostat',to:'decision',relation:'controls'},{from:'thermostat',to:'dispatch',relation:'context'},{from:'dispatch',to:'thermostat',relation:'context'}]}};
 const before=JSON.stringify(profile),model=build(profile),permission=model.onOff[0].expression,command=model.onOff[1].expression;
 check(model.onOff.map(item=>item.id).join('|')==='availability|command'&&command.children[0].op==='reference'&&command.children[0].referenceId==='availability','Named availability definition was duplicated or missing');
 check(permission.op==='all'&&permission.children.length===2&&permission.children[1].op==='any','Boolean nesting or neutral-default compression changed grouping');
 const evaluate=(tree,values)=>{if(tree.op==='schedule_positive')return values[tree.scheduleName];if(tree.op==='always')return tree.booleanValue?1:0;const children=tree.children.map(child=>evaluate(child,values));return tree.op==='all'?children.includes(0)?0:children.includes(null)?null:1:children.includes(1)?1:children.includes(null)?null:0;};
 for(const A of [0,1,null])for(const B of [0,1,null])for(const C of [0,1,null]){
  const alternative=B===1||C===1?1:B===null||C===null?null:0;
  const expected=A===0||alternative===0?0:A===null||alternative===null?null:1;
  check(evaluate(permission,{A,B,C})===expected,'Compressed Boolean grouping changed three-valued truth table');
 }
 const activation=command.children[1];
 check(activation.node===decision&&activation.dependencies.nodes.length===3&&activation.dependencies.edges.length===5&&!activation.children.length,'Runtime dependency closure became Boolean AND/OR or lost cyclic context');
 check(activation.dependencies.nodes.some(item=>item===dispatch)&&activation.dependencies.nodes.some(item=>item===thermostat),'Actual dispatch/thermostat source context vanished');
 const statusTree={op:'fan_gate',target:'central_fan',children:[{op:'plant_managers',children:[{op:'always',value:0,source:manager},{op:'always',value:1,source:manager}]},{op:'always',value:0,source:owner}]};
 const statuses=build({...profile,finalAvailability:statusTree,onOffExpression:all(statusTree,{op:'runtime_condition',referenceId:'decision'})}).onOff[0].expression;
 check(statuses.op==='fan_gate'&&statuses.children[0].op==='plant_managers'&&statuses.children[0].children.map(child=>child.managerStatus).join('|')==='NoAction|ForceOff'&&statuses.children[1].booleanValue===false,'Manager status domain/order was flattened into Boolean false/true');
 for(const onOff of [{nodes:[],edges:[]},{nodes:[decision,{...decision}],edges:[]}]) {
  check(build({...profile,onOff}).onOff[1].expression.children[1].op==='unknown','Missing or ambiguous same-profile rule reference selected an arbitrary node');
 }
 check(build({...profile,onOffExpression:undefined}).onOff[1].expression.reason==='on_off_expression_requires_reanalysis','Legacy graph edges were used to invent activation logic');
 const flow={...node('flow','external_input','upstream_flow'),boundary:'input'};
 const passive=build({...profile,onOff:{nodes:[flow],edges:[]},onOffExpression:{op:'passive_flow',referenceId:'flow'}});
 check(passive.onOff.length===1&&passive.onOff[0].expression.op==='passive_flow'&&passive.onOff[0].expression.node===flow,'Passive flow acquired a fake ON command');
 const cycle={op:'all',children:[]};cycle.children.push(cycle);
 check(build({...profile,finalAvailability:cycle,onOffExpression:undefined}).onOff[0].expression.op==='unknown','Cyclic expression was expanded indefinitely');
 const unsupported=build({...profile,finalAvailability:{op:'xor',children:[positive('A')]}});
 check(unsupported.onOff[0].expression.op==='unknown'&&!unsupported.onOff[0].expression.children,'Unsupported operator became a false Boolean formula');
 const emptyOr=build({...profile,finalAvailability:any(),onOffExpression:undefined}).onOff[0].expression;
 check(emptyOr.op==='always'&&emptyOr.booleanValue===false,'Empty OR was rendered as the neutral TRUE of AND');
 const wide=all(...Array.from({length:12000},()=>positive('A'))),wideCopy={...wide,children:[...wide.children]};
 const bounded=build({...profile,finalAvailability:wide,onOffExpression:all(wideCopy,{op:'runtime_condition',referenceId:'decision'})});
 const flattened=[];for(const formula of bounded.onOff){const pending=[formula.expression];while(pending.length){const current=pending.pop();flattened.push(current);pending.push(...current.children||[]);}}
 check(flattened.length<8300&&flattened.some(item=>item.reason==='expression_truncated'),'Wide malformed expression exceeded the normalization budget');
 const spm=source(10,'SetpointManager:Scheduled','Supply target'),schedule=source(11,'Schedule:Compact','Supply temperature'),controller=source(12,'Controller:WaterCoil','Coil controller');
 const parameterNodes=[
  {...node('inlet','external_input','inlet_state'),boundary:'input'},node('schedule','schedule','schedule_value',schedule),
  node('provider','setpoint','setpoint_manager',spm),node('target','setpoint','node_setpoint',spm),
  {...node('sensor','external_input','node_measurement',controller,[{name:'Sensor Node Name',value:'Outlet air'}]),boundary:'input'},
  node('residual','comparison','sensed_minus_setpoint',controller),node('controller','controller','feedback_controller',controller,[{name:'Action',value:'Reverse'}]),
  node('command','control_variable','water_flow_command',controller),node('equipment','equipment','equipment_response'),node('state','state_variable','outlet_state'),
  {...node('output','external_output','outlet_state'),boundary:'output'}];
 const parameterEdges=[{from:'schedule',to:'provider',relation:'sets_value'},{from:'provider',to:'target',relation:'sets_target'},
  {from:'sensor',to:'residual',relation:'adds_measurement',polarity:'positive'},{from:'target',to:'residual',relation:'subtracts_target',polarity:'negative'},
  {from:'residual',to:'controller',relation:'compares'},{from:'controller',to:'command',relation:'controls',polarity:'unknown'},
  {from:'command',to:'equipment',relation:'actuates'},{from:'equipment',to:'state',relation:'determines_state'},
  {from:'state',to:'sensor',relation:'feedback',polarity:'unknown'},{from:'state',to:'output',relation:'outputs'}];
 const parameterGraph={nodes:parameterNodes,edges:parameterEdges},parameterBefore=JSON.stringify(parameterGraph);
 const compact=build({...profile,parameterControl:parameterGraph}).parameters,allMembers=compact.groups.flatMap(group=>group.cards.flatMap(card=>card.members));
 check(allMembers.length===parameterNodes.length&&new Set(allMembers.map(item=>item.id)).size===parameterNodes.length&&parameterNodes.every(item=>allMembers.includes(item)),'Grouped parameter nodes lost identity, duplicated members or mutated originals');
 check(compact.groups.map(group=>group.id).join('|')==='inputs|controls|commands|response','External input/control/command/physical-response roles collapsed');
 const providerCard=compact.groups[0].cards.find(card=>card.members.some(item=>item.id==='provider'));
 check(providerCard.members.some(item=>item.id==='schedule'&&item.source===schedule)&&providerCard.source===spm,'Schedule folding lost the actual schedule source or provider ownership');
 const controllerCard=compact.groups.find(group=>group.id==='controls').cards.find(card=>card.source===controller);
 check(['sensor','residual','controller'].every(id=>controllerCard.members.some(item=>item.id===id))&&controllerCard.relationships.some(edge=>edge.polarity==='positive')&&controllerCard.relationships.some(edge=>edge.polarity==='negative'),'Native feedback residual signs or actual sensed node were lost');
 check(compact.relationships===parameterEdges&&parameterEdges.every(edge=>compact.groups.some(group=>group.cards.some(card=>card.relationships.includes(edge)))),'Native port/feedback relationship was invented or dropped');
 check(compact.groups.find(group=>group.id==='commands').cards[0].members[0].id==='command'&&compact.groups.at(-1).cards.some(card=>card.members.some(item=>item.boundary==='output')),'Manipulated command and physical state/output were confused');
 const sharedProvider=node('provider2','setpoint','setpoint_manager',source(13,'SetpointManager:Scheduled','Other target'));
 const shared=build({...profile,parameterControl:{nodes:[...parameterNodes,sharedProvider],edges:[...parameterEdges,{from:'schedule',to:'provider2',relation:'sets_value'}]}}).parameters;
 check(shared.groups[0].cards.some(card=>card.source===schedule&&card.members.some(item=>item.id==='schedule')),'Shared schedule was assigned to one arbitrary provider');
 check(JSON.stringify(profile)===before&&JSON.stringify(parameterGraph)===parameterBefore,'Compact adapter mutated source model');
 document.body.dataset.expressionStatus='passed';document.querySelector('#result').textContent='passed';
}catch(error){document.body.dataset.expressionStatus='failed';document.querySelector('#result').textContent=error.stack||String(error);}
</script></body></html>`
