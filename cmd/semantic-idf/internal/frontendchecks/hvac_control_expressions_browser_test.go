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
 const {buildHVACOnOffPresentation:present}=await import('/src/js/hvac-onoff-presentation.js');
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
 const loopOwner=source(20,'AirLoopHVAC','Air loop'),listSource=source(21,'AvailabilityManagerAssignmentList','Loop managers');
 const managerList={op:'air_managers',source:listSource,assignmentOwner:loopOwner,children:[{op:'manager_unknown',source:manager,assignmentOwner:loopOwner,reason:'runtime_manager'}]};
 const managerGate=(tree,target='central_fan')=>({op:'manager_gate',target,children:[tree]});
 const fanGate=(tree,target='central_fan',own=positive('Own'))=>({op:'fan_gate',target,children:[tree,own]});
 const managerWrapper=tree=>({op:'air_managers',children:[tree]});
 const presented=(...expressions)=>build({...profile,finalAvailability:all(...expressions),onOffExpression:undefined}).onOffPresentation;
 const nativeFan=all(managerGate(managerList),fanGate(managerWrapper(managerList))),nativeFanBefore=JSON.stringify(nativeFan);
 const named=presented(...nativeFan.children),namedAvailability=named.formulas[0].expression;
 check(namedAvailability.op==='rule_reference'&&named.rules.length===2&&named.rules.filter(rule=>rule.kind==='manager').length===1,'Repeated sourced manager became nested priority wrappers or duplicate permission operands');
 const managerRule=named.rules.find(rule=>rule.kind==='manager'),permissionRule=named.rules.find(rule=>rule.kind==='permission');
 check(managerRule.expression.source===listSource&&managerRule.expression.assignmentOwner===loopOwner&&permissionRule.managerId===managerRule.id&&permissionRule.op==='fan_gate'&&permissionRule.target==='central_fan'&&permissionRule.otherwise.scheduleName==='Own'&&permissionRule.source===owner,'Named override lost exact source, target, or NoAction schedule fallback');
 check(named.formulas[0].id==='availability'&&named.formulas[0].code==='runPermission'&&named.formulas[0].source===owner,'Run permission label changed native formula identity or owner');
 check(JSON.stringify(nativeFan)===nativeFanBefore,'Manager canonicalization mutated the native availability tree');
 // Independent status-mask truth table includes runtime uncertainty, rather
 // than treating the four manager statuses as ordinary Boolean operands.
 const statusValues=mask=>[0,1,2,3].filter(status=>mask&(1<<status));
 const agree=values=>values.every(value=>value===0)?0:values.every(value=>value===1)?1:null;
 const permissionValue=(status,target,otherwise)=>status===1?0:status===2?1:status===3?target==='zone_fan'?1:0:otherwise;
 const evaluateNative=(tree,mask,own)=>{
  if(tree.op==='schedule_positive')return own;
  if(tree.op==='always')return tree.booleanValue===false||tree.value===0?0:1;
  if(tree.op==='manager_gate'||tree.op==='fan_gate')return agree(statusValues(mask).map(status=>permissionValue(status,tree.target,tree.op==='fan_gate'?evaluateNative(tree.children[1],mask,own):1)));
  const values=tree.children.map(child=>evaluateNative(child,mask,own));
  return tree.op==='all'?values.includes(0)?0:values.includes(null)?null:1:values.includes(1)?1:values.includes(null)?null:0;
 };
 const evaluatePresented=(tree,presentation,mask,own)=>{
  if(tree.op==='all'||tree.op==='any'){
   const values=tree.children.map(child=>evaluatePresented(child,presentation,mask,own));
   return tree.op==='all'?values.includes(0)?0:values.includes(null)?null:1:values.includes(1)?1:values.includes(null)?null:0;
  }
  if(tree.op!=='rule_reference')return evaluateNative(tree,mask,own);
  const rule=presentation.rules.find(item=>item.id===tree.referenceId);
  check(rule?.kind==='permission'&&presentation.rules.some(item=>item.id===rule.managerId&&item.kind==='manager'),'Permission reference has no exact named manager definition');
  const fallback=evaluatePresented(rule.otherwise,presentation,mask,own);
  return agree(statusValues(mask).map(status=>permissionValue(status,rule.target,fallback)));
 };
 let truthCases=0;
 for(const target of ['central_fan','zone_fan'])for(let mask=1;mask<16;mask++)for(const own of [0,1,null]){
  const native=all(managerGate(managerList,target),fanGate(managerWrapper(managerList),target));
  const projected=presented(...native.children);
  check(evaluateNative(native,mask,own)===evaluatePresented(projected.formulas[0].expression,projected,mask,own),'Named fan permission changed status-mask truth table: '+[target,mask,own].join('/'));
  truthCases++;
 }
 check(truthCases===90,'Manager override truth table omitted a runtime status mask');
 const retainDistinct=(first,second,message)=>{
  const result=presented(managerGate(first),fanGate(second));
  check(result.formulas[0].expression.op==='all'&&result.formulas[0].expression.children.length===2&&result.rules.filter(rule=>rule.kind==='manager').length===2,message);
 };
 retainDistinct(managerList,{...managerList,source:{...listSource,objectIndex:22}},'Different manager source identity was merged');
 retainDistinct(managerList,{...managerList,assignmentOwner:{...loopOwner,objectIndex:23}},'Same-named manager lists assigned to different owners were merged');
 const ordered={...managerList,op:'plant_managers',children:[{op:'always',value:1,source:manager},{op:'always',value:2,source:{...manager,objectIndex:24}}]};
 retainDistinct(ordered,{...ordered,children:[...ordered.children].reverse()},'Authored plant manager order was discarded');
 retainDistinct(managerList,{op:'air_managers',source:source(25,'AvailabilityManagerAssignmentList','Outer list'),children:[managerList]},'Sourced manager wrapper was silently removed');
 retainDistinct(managerList,{op:'air_managers',assignmentOwner:loopOwner,children:[managerList]},'Manager assignment wrapper was silently removed');
 const differentTarget=presented(managerGate(managerList,'zone_fan'),fanGate(managerList,'central_fan'));
 check(differentTarget.formulas[0].expression.op==='all'&&differentTarget.rules.filter(rule=>rule.kind==='permission').length===2,'Central and zone fan permission were conflated');
 const orProfile=build({...profile,finalAvailability:any(managerGate(managerList),fanGate(managerList)),onOffExpression:undefined}).onOffPresentation;
 check(orProfile.formulas[0].expression.op==='any'&&orProfile.formulas[0].expression.children.length===2,'AND-only redundant gate rule was incorrectly applied to OR');
 const repeated=presented(fanGate(managerList),fanGate(managerWrapper(managerList)));
 check(repeated.rules.filter(rule=>rule.kind==='manager').length===1&&repeated.rules.filter(rule=>rule.kind==='permission').length===1,'Exact repeated override definitions were not shared');
 const managerOnly=presented(managerGate(managerList)).rules.find(rule=>rule.kind==='permission');
 check(managerOnly.otherwise.op==='always'&&managerOnly.otherwise.booleanValue===true,'Manager-only permission lost its allowed NoAction fallback');
 const directCycle={op:'all',domain:'boolean',children:[]};directCycle.children.push(directCycle);
 const directBounded=present([{id:'cycle',code:'cycle',source:owner,expression:directCycle},{id:'wide',code:'wide',source:owner,expression:wide}]);
 const projectedNodes=[];for(const formula of directBounded.formulas){const pending=[formula.expression];while(pending.length){const current=pending.pop();projectedNodes.push(current);pending.push(...current.children||[]);}}
 check(projectedNodes.length<4200&&projectedNodes.some(item=>item.reason==='unresolved_expression')&&projectedNodes.some(item=>item.reason==='expression_truncated'),'Public presentation projection expanded cyclic/wide input without bounds');
 let deepManager={op:'manager_unknown',domain:'manager',source:manager,reason:'runtime_manager'};
 for(let level=0;level<60;level++)deepManager={op:'air_managers',domain:'manager',source:source(100+level,'AvailabilityManagerAssignmentList','Level '+level),children:[deepManager]};
 const deepPresentation=present([{id:'deep',expression:{op:'manager_gate',domain:'boolean',target:'central_fan',children:[deepManager]}}]);
 let deepDepth=0,deepLeaf=deepPresentation.rules.find(rule=>rule.kind==='manager').expression;
 while(deepLeaf.children?.length){deepDepth++;deepLeaf=deepLeaf.children[0];}
 check(deepDepth<=49&&deepLeaf.op==='unknown'&&deepLeaf.reason==='unresolved_expression','Deep sourced manager signatures exceeded their structural budget');
 check(JSON.stringify(profile)===before,'Named-rule projection changed native expressions or source references');
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
