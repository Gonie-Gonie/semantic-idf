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

func TestEPATH182ReportSnapshotAndHTMLBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless report projection verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath182-report", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath182ReportBrowserHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath182-report").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-epath182-report="passed"`) {
		if start := strings.Index(string(output), `<pre id="result">`); start >= 0 {
			if end := strings.Index(string(output)[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH182 report: %v\n%s", err, output[start:start+end])
			}
		}
		t.Fatalf("EPATH182 report: %v\n%s", err, output)
	}
}

const epath182ReportBrowserHTML = `<!doctype html><html><head><meta charset="utf-8"></head><body><pre id="result">pending</pre><script type="module">
const errors=[],check=(value,message)=>{if(!value)errors.push(message);};
const copy=value=>JSON.parse(JSON.stringify(value));
const equal=(left,right)=>JSON.stringify(left)===JSON.stringify(right);
try {
 const {buildEnergyPathReport:build,renderEnergyPathReportHTML:render}=await import('/src/js/energy-path-report.js');
 const {energyPathSummaryForState}=await import('/src/js/views/energy-path-view.js');
 const {energyPathQualityForState}=await import('/src/js/energy-path-details.js');
 let calls=0;window.fetch=()=>{calls++;throw Error('report must not fetch');};
 const quality={drivers:{status:'partial',found:0,total:2},loads:{status:'complete',found:1,total:1},endUses:{status:'partial',found:2,total:3},carriers:{status:'complete',found:1,total:1},ratios:{status:'partial',found:1,total:2},driverToLoadClosedPct:0,driverToLoadStatus:'partial',endUseToCarrierClosedPct:100,endUseToCarrierStatus:'complete',zoneAllocatedPct:30,unassignedPct:70,zoneAllocationStatus:'partial'};
 const scope={kind:'building',valueBasis:'model_total'};
 const item=(id,label,value,unit='kWh')=>({id,label,value,unit,basis:'reported_variable',sourceIds:['SOURCE_PRIVATE_182']});
 const summary={schema:'semantic-idf.energy-explanation-summary/v2',scope,period:'annual',drivers:[item('NODE_PRIVATE_182_wall','Exterior walls',30)],loads:[{...item('load.cooling','Cooling load',100),serviceKind:'cooling'},{...item('load.heating','Heating load',85),serviceKind:'heating'}],endUses:[item('use.cooling','Cooling',25),item('use.zero','Lighting',0),item('use.null','Missing equipment',null),{id:'use.absent',label:'Absent equipment',unit:'kWh'}],carriers:[item('carrier.electricity','Electricity',50)],ratios:[item('ratio.cooling','Cooling COP',4,'ratio')],residuals:[],quality};
 const nodes=[{id:'NODE_PRIVATE_182_wall',level:'driver',kind:'driver.exterior_wall',period:'annual',value:30,unit:'kWh',scaleDomain:'thermal'}, {id:'load.cooling',level:'load',serviceKind:'cooling',period:'annual',value:100,unit:'kWh',scaleDomain:'thermal'}, {id:'use.cooling',level:'end_use',endUse:'cooling',period:'annual',value:25,unit:'kWh',scaleDomain:'site'}, {id:'carrier.electricity',level:'carrier',carrier:'electricity',period:'annual',value:50,unit:'kWh',scaleDomain:'site',basis:'reported_meter',meterHierarchyLevel:'facility_total'}];
 const links=[{id:'LINK_PRIVATE_182',relation:'load_to_end_use',fromId:'load.cooling',toId:'use.cooling',period:'annual',fromValue:100,fromUnit:'kWh',toValue:25,toUnit:'kWh',ratio:4,ratioKind:'cop',sourceIds:['SOURCE_PRIVATE_182']}];
 const sources=[{id:'SOURCE_PRIVATE_182',name:'Energy <script>bad()<\/script>',sourceUnit:'J',normalizedUnit:'kWh',rawValue:null,futureField:'future\r\n☀'}];
 const explanation={schema:'semantic-idf.energy-explanation/v2',scope,nodes,links,sources,quality,periods:[],zoneResults:[]};
 const result={runId:'run182',filename:'Office <script>bad()<\/script>.idf',inputPath:'C:/run/Office.idf',finishedAt:'2026-01-01',purposeResults:{energyExplanation:explanation,energyExplanationSummary:summary},unknown:null};
 const state={simulationEnergyScopeKind:'building',simulationEnergyPeriod:'annual',simulationEnergyService:'cooling',simulationEnergySelection:{kind:'node',id:'load.cooling'},simulationEnergyDetailsOpen:true};
 const before=JSON.stringify(result),stateBefore=JSON.stringify(state),report=build(result,state);
 check(report.context.summaryService==='all'&&report.context.service==='cooling','graph emphasis was mistaken for summary filtering');
 check(equal(report.summary,energyPathSummaryForState(explanation,summary,state)),'report recomputed the UI summary');
 check(equal(report.quality,energyPathQualityForState(explanation,state)),'report rewrote UI quality');
 check(report.rawResult===before&&JSON.stringify(result)===before&&JSON.stringify(state)===stateBefore,'report changed original result/state or raw snapshot');
 check(equal(report.trace.nodes,nodes)&&equal(report.trace.links,links)&&equal(report.trace.sources,sources),'trace rows were re-normalized');
 check(report.qualityRows.find(row=>row.key==='drivers').found===0&&report.qualityRows.find(row=>row.key==='driver_to_load_closed_pct').value===0,'known zero converted to unknown');
 check(report.qualityRows.find(row=>row.key==='end_use_to_carrier_closed_pct').value===100,'reported facility closure lost');
 const page=new DOMParser().parseFromString(render(report),'text/html');
 check(page.querySelector('[data-energy-path-report] h1')?.textContent==='Energy Path','missing report heading');
 check([...page.querySelectorAll('[data-energy-path-report-stage]')].map(node=>node.dataset.energyPathReportStage).join(',')==='drivers,loads,endUses,carriers,ratios,residuals','summary stage order changed');
 check(!page.querySelector('[data-energy-path-report-trace]').open&&page.querySelectorAll('[data-energy-path-report-trace] table').length===2,'trace not separate/collapsed');
 check(!page.querySelector('script')&&!page.querySelector('img'),'unescaped result/source text injected markup');
 check(page.body.textContent.includes('Summary: all services')&&page.body.textContent.includes('Graph selection: cooling'),'service context is misleading');
 const primary=page.querySelector('[data-energy-path-report]').cloneNode(true);primary.querySelector('[data-energy-path-report-trace]').remove();
 check(!/SOURCE_PRIVATE_182|NODE_PRIVATE_182|LINK_PRIVATE_182/.test(primary.textContent),'raw IDs leaked into primary report');
 const rows=[...page.querySelectorAll('[data-energy-path-report-stage="endUses"] tbody tr')].map(row=>[...row.cells].map(cell=>cell.textContent));
 check(rows.find(row=>row[0]==='Lighting')[2]==='0'&&rows.find(row=>row[0]==='Missing equipment')[2]==='—'&&rows.find(row=>row[0]==='Absent equipment')[2]==='—','HTML null/missing/zero distinction lost');
 check(page.querySelector('[data-energy-path-report-trace]').textContent.includes('J')&&page.querySelector('[data-energy-path-report-trace]').textContent.includes('kWh'),'source/normalized units lost');
 report.summary.loads[0].value=999;report.trace.sources[0].futureField='mutated';
 check(JSON.stringify(result)===before,'report aliases mutable source arrays');
 const zone=copy(explanation);zone.scope={kind:'zone',zoneName:'Office',valueBasis:'model_total'};zone.nodes=nodes.map(node=>({...node,zoneName:'Office',basis:node.level==='carrier'?'direct_zone_energy':node.basis,period:'M1'}));zone.links=links.map(link=>({...link,zoneName:'Office',period:'M1'}));zone.quality={...quality,endUseToCarrierClosedPct:0,endUseToCarrierStatus:'partial'};zone.summary={...copy(summary),scope:zone.scope,period:'annual',completeness:{energyUse:{status:'partial'}}};
 const monthlySummary={...copy(zone.summary),period:'M1'};
 zone.periods=[{id:'M1',nodes:zone.nodes,links:zone.links,summary:monthlySummary,quality:zone.quality},{id:'M3',nodes:[],links:[]}];
 explanation.zoneResults=[zone];
 const zoneState={...state,simulationEnergyScopeKind:'zone',simulationEnergyZoneName:'Office',simulationEnergyPeriod:'M1'};
 const selected=build(result,zoneState);
 check(selected.context.zoneName==='Office'&&selected.context.period==='M1'&&selected.qualityRows.find(row=>row.key==='end_use_to_carrier_closed_pct').value===null,'Zone subtotal pretends measured zero closure');
 const empty=build(result,{...zoneState,simulationEnergyPeriod:'M3'});
 check(empty.summary===null&&empty.trace.nodes.length===0&&!render(empty).includes('>100</td>'),'explicit empty month fell back to annual');
 const mustReject=(candidate,selectedState,message)=>{let rejected=false;try{build(candidate,selectedState);}catch{rejected=true;}check(rejected,message);};
 mustReject(result,{...zoneState,simulationEnergyPeriod:'M2'},'missing month accepted');
 mustReject(result,{...zoneState,simulationEnergyZoneName:'Missing'},'missing Zone accepted');
 const duplicate=copy(result);duplicate.purposeResults.energyExplanation.zoneResults.push(copy(zone));
 mustReject(duplicate,zoneState,'ambiguous Zone accepted');
 const duplicateMonth=copy(result);duplicateMonth.purposeResults.energyExplanation.zoneResults[0].periods.push(copy(zone.periods[0]));
 mustReject(duplicateMonth,zoneState,'ambiguous month accepted');
 check(build({purposeResults:{energyExplanation:{schema:'semantic-idf.energy-explanation/v1'}}},state)===null&&calls===0,'legacy masquerade or backend call');
} catch(error){errors.push(error.stack||String(error));}
document.getElementById('result').textContent=JSON.stringify({errors});document.body.dataset.epath182Report=errors.length?'failed':'passed';
</script></body></html>`
