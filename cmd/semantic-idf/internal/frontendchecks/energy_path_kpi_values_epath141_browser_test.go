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

func TestEPATH141KPIValuesBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser KPI value verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath141-kpi-values", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath141KPIValuesHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath141-kpi-values").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH141 KPI values browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath141-kpi-values="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH141 KPI values failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH141 KPI values failed:\n%s", output)
	}
}

const epath141KPIValuesHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH141 KPI values</title></head>
<body data-epath141-kpi-values="pending"><pre id="result">pending</pre><script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const clone=(value)=>JSON.parse(JSON.stringify(value));
try {
  const {energyPathKPIItems:items}=await import('/src/js/energy-path-kpis.js');
  const {energyPathSummaryKPIValues:base}=await import('/src/js/energy-path-summary.js');
  const nodes=[
    {id:'load.cooling',level:'load',serviceKind:'cooling',value:100,unit:'kWh thermal',scaleDomain:'thermal',period:'annual'},
    {id:'load.heating',level:'load',serviceKind:'heating',value:40,unit:'kWh thermal',scaleDomain:'thermal',period:'annual'},
    {id:'end_use.cooling',level:'end_use',endUse:'cooling',value:25,unit:'kWh site',scaleDomain:'site',period:'annual'},
    {id:'end_use.heating',level:'end_use',endUse:'heating',value:15,unit:'kWh site',scaleDomain:'site',period:'annual'},
    {id:'carrier.electricity',label:'Electricity',level:'carrier',carrier:'electricity',value:25,unit:'kWh site',scaleDomain:'site',period:'annual'},
    {id:'carrier.natural_gas',label:'Natural gas',level:'carrier',carrier:'natural_gas',value:15,unit:'kWh site',scaleDomain:'site',period:'annual'},
  ];
  const links=[
    {id:'conversion.cooling',relation:'load_to_end_use',fromId:'load.cooling',toId:'end_use.cooling',serviceKind:'cooling',fromValue:40,toValue:20,fromUnit:'kWh thermal',toUnit:'kWh site',period:'annual',sourceIds:['actual.cooling']},
    {id:'conversion.heating',relation:'load_to_end_use',fromId:'load.heating',toId:'end_use.heating',serviceKind:'heating',fromValue:30,toValue:15,fromUnit:'kWh thermal',toUnit:'kWh site',period:'annual',sourceIds:['actual.heating']},
  ];
  const graph={nodes,links};
  const quality={driverToLoadClosedPct:82.5,driverToLoadStatus:'partial',endUseToCarrierClosedPct:90,endUseToCarrierStatus:'overmapped',ratios:{status:'partial',found:1,total:2}};
  const summary={schema:'semantic-idf.energy-explanation-summary/v2',period:'annual',loads:nodes.filter(item=>item.level==='load'),carriers:nodes.filter(item=>item.level==='carrier'),ratios:[{value:999,serviceKind:'cooling'}],completeness:{mappedPercent:100},quality};
  const before=JSON.stringify({summary,graph,quality});
  const byID=(values)=>Object.fromEntries(values.map(item=>[item.id,item]));
  let result=items(summary,graph,{service:'cooling',period:'annual',quality});
  let values=byID(result);
  check(result.length===4&&result.filter(item=>item.emphasized).length===1&&values.cooling_load.emphasized,'four-card selected-service emphasis failed');
  check(values.total_site_energy.value===40&&values.cooling_load.value===100&&values.heating_load.value===40,'service emphasis changed scope/period totals');
  check(values.cooling_load.ratio?.value===2&&values.cooling_load.ratio.fromValue===40&&values.cooling_load.ratio.toValue===20&&values.cooling_load.ratio.partial===true,'ratio used annual node totals or synthetic summary ratio instead of paired40/20');
  check(JSON.stringify(values.cooling_load.ratio.linkIds)==='["conversion.cooling"]','ratio lost actual conversion link provenance');
  check(!values.heating_load.ratio,'unselected service added an extra ratio');
  check(values.coverage.value===null&&values.coverage.unit===''&&values.coverage.targets.length===0,'coverage fabricated a scalar score or energy node');
  check(JSON.stringify(values.coverage.coverage.boundaries.map(item=>[item.id,item.value,item.status]))===JSON.stringify([['driver_to_load',82.5,'partial'],['end_use_to_carrier',90,'overmapped']]),'independent closure boundaries collapsed or legacy mappedPercent won');
  check(values.total_site_energy.targets.length===2&&values.total_site_energy.targets.every(item=>nodes.some(node=>node.id===item.id)),'site total selected only first carrier or fabricated aggregate node');
  check(values.cooling_load.targets[0]?.id==='load.cooling'&&values.cooling_load.targets[0]?.serviceKind==='cooling','cooling card lost canonical graph target');
  values=byID(items(summary,graph,{service:'heating',quality}));
  check(values.heating_load.emphasized&&values.heating_load.ratio?.value===2&&!values.cooling_load.emphasized,'heating emphasis/ratio failed');
  check(items(summary,graph,{service:'all',quality}).every(item=>!item.emphasized&&!item.ratio),'all-service mode invented selected-service emphasis');
  const renamed=clone(graph);renamed.nodes[0].originalNodeIds=['load.cooling'];renamed.nodes[0].id='presentation.cooling';
  values=byID(items(summary,renamed,{quality}));
  check(values.cooling_load.value===100&&values.cooling_load.targets[0]?.id==='presentation.cooling','exact originalNodeIds correspondence lost canonical target');
  const unmatched=clone(graph);unmatched.nodes=unmatched.nodes.filter(node=>node.id!=='load.cooling');
  values=byID(items(summary,unmatched,{quality}));
  check(values.cooling_load.value===null&&values.cooling_load.targets.length===0,'missing graph target guessed a same-service node or retained inconsistent total');
  for(const bad of [null,undefined,false,true,'', ' ',NaN,Infinity]){
    const modified={...summary,loads:[{...nodes[0],value:bad}]};
    check(byID(base(modified)).cooling_load.value===null,'missing/non-numeric load became reported zero');
  }
  check(byID(base({...summary,loads:[{...nodes[0],value:0}]})).cooling_load.value===0,'explicit reported zero became unavailable');
  values=byID(items({...summary,loads:[{...nodes[0],value:0}],carriers:[]},{nodes:[],links:[]},{quality}));
  check(values.cooling_load.value===0&&values.cooling_load.targets.length===0&&values.total_site_energy.value===null,'zero-pruned graph erased reportedzero or invented target/other totals');
  check(byID(base({...summary,loads:[nodes[0],{...nodes[0],id:'load.cooling.unknown',value:null}]})).cooling_load.value===null,'unknown contributor silently entered total aszero');
  values=byID(items({}, {nodes:[],links:[]}, {period:'M3',quality}));
  check(Object.keys(values).length===4&&Object.values(values).every(item=>item.value===null&&!item.ratio),'missing summary lost placeholders or copied annual totals');
  check(values.coverage.coverage.boundaries[0].value===82.5,'missing summary discarded explicitly supplied scoped quality');
  check(base({schema:'semantic-idf.energy-explanation-summary/v1',energyByCarrier:[{value:100}]}).length===0,'V1 adapter contract changed');
  for(const status of ['unavailable','not_requested','not_applicable','missing']){
    values=byID(items(summary,graph,{service:'cooling',quality:{...quality,driverToLoadStatus:status,ratios:{status}}}));
    check(!values.cooling_load.ratio&&values.coverage.coverage.boundaries[0].value===null&&values.coverage.coverage.boundaries[0].status===status,'explicit unavailability was overridden by numeric graph/quality values: '+status);
  }
  values=byID(items(summary,graph,{quality:{...quality,driverToLoadClosedPct:0},knownZoneOnly:true}));
  check(values.coverage.coverage.boundaries[0].value===0&&values.coverage.coverage.boundaries[1].value===null&&values.coverage.coverage.boundaries[1].status==='unavailable','known-only zone exposed fake0/100 carrier closure or hid valid driverzero');
  values=byID(items(summary,graph,{quality:{}}));
  check(values.coverage.coverage.boundaries.every(item=>item.value===null&&item.status==='unavailable'),'absent quality reused legacy mappedPercent');
  const monthly=clone(graph);monthly.nodes.forEach(node=>node.period='M2');monthly.links.forEach(link=>link.period='M2');monthly.links[0].fromValue=6;monthly.links[0].toValue=2;
  check(byID(items({...summary,period:'M2'},monthly,{service:'cooling',period:'M2',quality})).cooling_load.ratio?.value===3,'M2 did not use actual6/2 conversion');
  check(!byID(items({...summary,period:'M2'},graph,{service:'cooling',period:'M2',quality})).cooling_load.ratio,'annual graph supplied a missing monthly ratio');
  for(const mutate of [
    graph=>{graph.links[0].period='M1';},
    graph=>{graph.nodes[2].scaleDomain='thermal';},
    graph=>{graph.links[0].toUnit='kWh thermal';},
    graph=>{graph.links[0].toValue=null;},
    graph=>{graph.links[0].sourceIds=[];},
    graph=>{graph.links[0].serviceKind='heating';},
  ]){
    const changed=clone(monthly);mutate(changed);
    check(!byID(items(summary,changed,{service:'cooling',period:'M2',quality})).cooling_load.ratio,'invalid conversion identity/domain/value/provenance acquired ratio');
  }
  const unqualified=clone(monthly);unqualified.nodes.forEach(node=>delete node.period);unqualified.links.forEach(link=>delete link.period);
  check(!byID(items(summary,unqualified,{service:'cooling',period:'M2',quality})).cooling_load.ratio,'unqualified observations were relabeled as selected month');
  const complete=clone(graph);complete.links[0].fromValue=100;complete.links[0].toValue=25;
  check(byID(items(summary,complete,{service:'cooling',quality})).cooling_load.ratio?.partial===false,'fully paired values retained false partial badge');
  complete.links[0].fromValue=110;
  check(byID(items(summary,complete,{service:'cooling',quality})).cooling_load.ratio?.partial===true,'overmapped discrepancy was presented as full pairing');
  const duplicates=clone(graph);duplicates.links.push(clone(duplicates.links[0]));
  check(byID(items(summary,duplicates,{service:'cooling',quality})).cooling_load.ratio?.fromValue===40,'duplicate link ID doubled observed values');
  duplicates.links[2].fromValue=41;
  check(!byID(items(summary,duplicates,{service:'cooling',quality})).cooling_load.ratio,'contradictory duplicate link selected arbitrary values');
  const water={id:'carrier.water',level:'carrier',carrier:'water',value:5,unit:'kWh',scaleDomain:'site',basis:'derived_ratio'};
  const waterSummary={...summary,carriers:[...summary.carriers,water]};
  check(byID(base(waterSummary)).total_site_energy.value===45,'explicit110 converted-water summary exception was removed');
  check(byID(items(waterSummary,graph,{quality})).total_site_energy.value===null,'unvalidated converted-water summary value remained a complete site total');
  check(byID(items(waterSummary,{...graph,nodes:[...nodes,water]},{quality})).total_site_energy.value===45,'validated converted-water graph contribution was dropped');
  check(JSON.stringify({summary,graph,quality})===before,'KPI derivation mutated canonical payload or sources');
  if(failures.length)throw new Error(failures.join(' | '));
  document.body.dataset.epath141KpiValues='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.epath141KpiValues='failed';document.getElementById('result').textContent=String(error?.stack||error);}
</script></body></html>`
