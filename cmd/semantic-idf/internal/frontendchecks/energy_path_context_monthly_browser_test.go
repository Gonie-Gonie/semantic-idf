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

func TestEnergyPathMonthlyInputContextServiceTurnoverBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser monthly input-reference verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/monthly-context", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathContextMonthlyHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/monthly-context").CombinedOutput()
	if err != nil {
		t.Fatalf("monthly input-reference browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-monthly-context="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("monthly input-reference verification failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("monthly input-reference verification failed:\n%s", output)
	}
}

const energyPathContextMonthlyHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Monthly input reference</title></head>
<body data-monthly-context="pending"><pre id="result">pending</pre><div id="mount"></div><script type="module">
const check=(condition,message)=>{if(!condition)throw new Error(message);};
try{
 const [{energyPathGraphForState:project,renderEnergyPathView:renderView,ENERGY_PATH_SCHEMA_V2:schema},{energyPathMonthlyChartSeries:monthly,energyPathLinkRatioChartSeries:ratios,renderEnergyPathComponentChart:render}]=await Promise.all([
  import('/src/js/views/energy-path-view.js'),import('/src/js/energy-path-chart.js')]);
 const zoneName='Perimeter_bot_ZN_3',scope={kind:'zone',zoneName},windowCategory='surface.windows_doors';
 const inputID='input:input.solar_incident:'+windowCategory+':perimeter_bot_zn_3';
 const nativeGraph=(period,solar,drivers)=>{
  const nodes=[{id:inputID,level:'input',kind:'input.solar_incident',label:'Incident solar',scaleDomain:'boundary',unit:'kWh',period,zoneName,value:Math.abs(solar),signedValue:solar,sourceIds:['solar.native']}],links=[];
  for(const [serviceKind,value]of drivers){
   const driver={id:'driver.surface_windows_doors.'+serviceKind+'.perimeter_bot_zn_3',level:'driver',kind:'driver.'+windowCategory,driverCategory:windowCategory,label:'Window / door heat exchange',scaleDomain:'thermal',unit:'kWh',period,zoneName,serviceKind,value,sourceIds:['window.native.'+serviceKind]};
   nodes.push(driver,{id:'load.'+serviceKind+'.perimeter_bot_zn_3',level:'load',serviceKind,label:serviceKind+' load',scaleDomain:'thermal',unit:'kWh',period,zoneName,value,sourceIds:[serviceKind+'.native']});
   links.push({id:'input-context:'+inputID+':'+driver.id,relation:'input_to_driver',basis:'boundary_context',fromId:inputID,toId:driver.id,fromValue:solar,toValue:value,fromUnit:'kWh',toUnit:'kWh',period,zoneName,sourceIds:['solar.native',...driver.sourceIds]});
  }
  return{nodes,links};
 };
 const annual=nativeGraph('Annual',281.99,[['cooling',4.11],['heating',1]]);
 const nativeMonths=[nativeGraph('M1',20,[['heating',.3]]),nativeGraph('M2',22,[['cooling',.7]]),nativeGraph('M3',24,[['cooling',.6],['heating',.2]]),nativeGraph('M4',26,[]),nativeGraph('M5',0,[['cooling',0]]),...Array.from({length:7},(_,index)=>nativeGraph('M'+(index+6),30+index,[]))];
 const explanation={schema,scope,...annual,periods:nativeMonths.map((graph,index)=>({id:'M'+(index+1),kind:'monthly',...graph}))};
 const before=JSON.stringify(explanation),state={simulationEnergyScopeKind:'zone',simulationEnergyZoneName:zoneName,simulationEnergyService:'all',simulationEnergyPeriod:'annual'};
 const selected=project(explanation,state).relations.find(link=>link.relation==='input_to_driver');
 check(selected&&selected.groupedMembers.length===2&&selected.fromValue===281.99&&selected.toValue===5.11,'native-shaped Annual context did not preserve both heating/cooling references');
 const graphs=Array.from({length:12},(_,month)=>project(explanation,{...state,simulationEnergyPeriod:'M'+(month+1)}));
 const sourceOptions={fromNode:project(explanation,state).nodes.find(node=>node.id===selected.fromId)};
 const series=monthly(selected,'link',graphs,sourceOptions),from=series[0].points,to=series[1].points;
 check(from.slice(0,5).map(point=>point.value).join(',')==='20,22,24,26,0','observed monthly inputs disappeared with turnover or absent room drivers: '+JSON.stringify(from));
 check(to.slice(0,5).map(point=>point.value).join(',')==='0.3,0.7,0.8,,0','monthly driver references lost service turnover, explicit zero or missing-vs-zero: '+JSON.stringify(to));
 check(from.every(point=>typeof point.value==='number')&&to.slice(5).every(point=>point.value===null),'complete native input months disappeared with absent driver references');
 const missingMonth=[...graphs];missingMonth[11]={};
 check(monthly(selected,'link',missingMonth).every(trace=>trace.points[11].value===null),'a missing native month was populated from Annual values');
 const ratio=ratios(selected,series)[0].points;
 check(Math.abs(ratio[0].value-20/.3)<1e-10&&ratio[3].value===null&&ratio[4].value===null,'input relation lost paired ratio or invented missing/zero-denominator ratios');
 const foreign=structuredClone(graphs[0]),foreignLink=foreign.relations.find(link=>link.id===selected.id);
 foreignLink.groupedMembers.push({...foreignLink.groupedMembers[0],id:'foreign-original',fromValue:999,toValue:999});
 check(monthly(selected,'link',[foreign]).every(trace=>trace.points[0].value===null),'an unrelated original relation contaminated the selected pair');
 const duplicate=structuredClone(graphs[0]);duplicate.relations.push(structuredClone(duplicate.relations[0]));
 check(monthly(selected,'link',[duplicate]).every(trace=>trace.points[0].value===null),'duplicate physical references were silently selected');
 const otherZone=structuredClone(graphs[0]);otherZone.relations[0].zoneName='Other zone';otherZone.nodes.forEach(node=>node.zoneName='Other zone');
 check(monthly(selected,'link',[otherZone]).every(trace=>trace.points[0].value===null),'monthly reference crossed Zone scope');
 const buildingGraph=graph=>({nodes:graph.nodes.map(node=>({...node,zoneName:''})),links:graph.links.map(link=>({...link,zoneName:''}))});
 const building={schema,scope:{kind:'building'},...buildingGraph(annual),periods:nativeMonths.map((graph,index)=>({id:'M'+(index+1),kind:'monthly',...buildingGraph(graph)}))},buildingState={...state,simulationEnergyScopeKind:'building'};
 const buildingSelected=project(building,buildingState).relations.find(link=>link.relation==='input_to_driver'),buildingMonths=Array.from({length:12},(_,month)=>project(building,{...buildingState,simulationEnergyPeriod:'M'+(month+1)}));
 const buildingSeries=monthly(buildingSelected,'link',buildingMonths);
 check(buildingSeries[0].points.every((point,index)=>point.value===from[index].value)&&buildingSeries[1].points.every((point,index)=>point.value===to[index].value),'All Building references lost month values or service-turnover identity');
 const withWallInput=(graph,value)=>({...graph,nodes:[...graph.nodes,{...graph.nodes[0],id:inputID.replace(windowCategory,'surface.exterior_walls'),driverCategory:'surface.exterior_walls',value:Math.abs(value),signedValue:value,sourceIds:['solar.wall.native']}]});
 const rosterExplanation={schema,scope,...withWallInput(annual,400),periods:[{id:'M1',kind:'monthly',...nativeMonths[0]}]};
 const rosterAnnual=project(rosterExplanation,state),rosterLink=rosterAnnual.relations.find(link=>link.relation==='input_to_driver'),rosterOptions={fromNode:rosterAnnual.nodes.find(node=>node.id===rosterLink.fromId)};
 const rosterMonth=(index,value)=>project({schema,scope,...withWallInput(nativeMonths[index],value)},{...state,simulationEnergyPeriod:'M'+(index+1)});
 const rosterMonths=[project(rosterExplanation,{...state,simulationEnergyPeriod:'M1'}),rosterMonth(1,40),rosterMonth(4,0),rosterMonth(3,42)];
 const rosterSeries=monthly(rosterLink,'link',rosterMonths,rosterOptions);
 check(rosterLink.fromValue===681.99&&rosterOptions.fromNode.groupedMembers.length===2,'selected reference lost full Solar input-card quantity or roster');
 check(rosterSeries[0].points.slice(0,4).map(point=>point.value).join(',')===',62,0,68','incomplete monthly input roster was accepted or complete/known-zero/independent input sum changed');
 check(rosterSeries[1].points.slice(0,4).map(point=>point.value).join(',')==='0.3,0.7,0,','source roster guard erased an independently observed driver or invented a missing driver');
 const rosterHost=document.createElement('div');rosterHost.innerHTML=renderView(rosterExplanation,{...state,simulationEnergySelection:rosterLink.id});
 check([...rosterHost.querySelectorAll('[data-energy-path-chart-value]')].map(mark=>mark.dataset.energyPathChartValue).join(',')==='0.3','actual inspector did not pass the selected input roster or discarded its independent target');
 const mount=document.getElementById('mount');mount.innerHTML=render({item:selected,kind:'link',monthly:series,endpointLabels:{from:'Incident solar',to:'Window / door heat exchange'},display:{perArea:true,areaM2:1}});
 check(mount.querySelectorAll('[data-energy-path-chart-value]').length===16&&!mount.querySelector('[data-energy-path-chart-empty]'),'native monthly context observations failed to render paired quantity bars');
 check(JSON.stringify(explanation)===before,'monthly presentation changed native observations/source evidence');
 document.body.dataset.monthlyContext='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.monthlyContext='failed';document.getElementById('result').textContent=String(error.stack||error);}
</script></body></html>`
