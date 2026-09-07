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

func TestEPATH170PureBatchComparisonBoundariesBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless Energy Path comparison boundary verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath170-comparison", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath170PureComparisonHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath170-comparison").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH170 pure comparison browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath170-comparison="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH170 pure comparison failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH170 pure comparison failed:\n%s", output)
	}
}

const epath170PureComparisonHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH170 comparison boundaries</title></head>
<body data-epath170-comparison="pending"><pre id="result">pending</pre><script type="module">
const failures=[];const check=(ok,message)=>{if(!ok)failures.push(message);};
const clone=value=>structuredClone(value),json=JSON.stringify;
const freeze=value=>{if(value&&typeof value==='object'&&!Object.isFrozen(value)){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
try{
 const {ENERGY_PATH_BATCH_STAGES:stages,energyPathBatchSummary:read,energyPathBatchComparison:compare}=await import('/src/js/energy-path-batch-comparison.js');
 const source={schema:'semantic-idf.energy-explanation-summary/v2',period:'annual',scope:{kind:'building',aggregationBasis:'model_total'},
  drivers:[{id:'raw.driver.id',level:'driver',kind:'driver.surface.exterior_walls',serviceKind:'cooling',value:100,unit:'kWh thermal',scaleDomain:'thermal',basis:'heat_balance_share',label:'DO_NOT_SHOW_SOURCE_ID'}],
  loads:[{id:'raw.load.id',level:'load',kind:'load.cooling',serviceKind:'cooling',value:100,unit:'kWh thermal',scaleDomain:'thermal',basis:'reported_variable'}],
  endUses:[{id:'raw.enduse.id',level:'end_use',kind:'energy.cooling',endUse:'cooling',value:25,unit:'kWh',scaleDomain:'site',basis:'reported_meter'}],
  carriers:[{id:'raw.carrier.id',level:'carrier',kind:'energy.electricity.total',carrier:'electricity',value:25,unit:'kWh site',scaleDomain:'site',basis:'reported_meter'}],
  ratios:[{id:'raw.ratio.id',level:'ratio',kind:'kpi.cooling_cop',serviceKind:'cooling',value:4,basis:'derived_ratio',numeratorUnit:'kWh thermal',denominatorUnit:'kWh site',numeratorLabel:'RAW_FROM_ID',denominatorLabel:'RAW_TO_ID'}],
  residuals:[{id:'raw.residual.id',level:'residual',kind:'energy.unclassified',carrier:'electricity',value:-5,unit:'kWh',scaleDomain:'site',basis:'residual'}],
  quality:Object.fromEntries(['drivers','loads','endUses','carriers','ratios'].map(key=>[key,{status:'complete',found:2,total:2}]))};
 Object.assign(source.quality,{driverToLoadClosedPct:100,driverToLoadStatus:'complete',endUseToCarrierClosedPct:100,endUseToCarrierStatus:'complete',zoneAllocatedPct:75,unassignedPct:25,zoneAllocationStatus:'partial'});
 const run=summary=>({runId:'raw.run.id',purposeResults:{energyExplanationSummary:summary}});
 const first=run(source),other=clone(first);other.runId='DIFFERENT_RUN';
 for(const stage of stages)for(const item of other.purposeResults.energyExplanationSummary[stage.key]){item.id='unrelated.'+item.id;item.label='another arbitrary label';item.sourceIds=['OTHER_SOURCE_ID'];item.value*=2;}
 const before=json([first,other]);freeze(first);freeze(other);
 let rows=compare(first,other).rows,summaryRows=rows.filter(row=>row.kind==='summary');
 check(json(stages.map(stage=>stage.key))===json(['drivers','loads','endUses','carriers','ratios','residuals']),'stage order/top-Zone scope changed');
 check(summaryRows.length===6&&summaryRows.every(row=>row.delta===row.baseline.value&&row.deltaPercent===100),'semantic identity depended on run/node/source IDs or labels');
 check(summaryRows.every(row=>row.directlyComparable)&&!json(rows.map(row=>row.category)).includes('RAW_')&&!json(rows.map(row=>row.category)).includes('DO_NOT_SHOW'),'safe labels/basis semantics lost');
 check(json([first,other])===before,'comparison mutated frozen source data');
 const changed=clone(first),summary=changed.purposeResults.energyExplanationSummary;
 const valueCase=(value)=>{summary.loads[0].value=value;return compare(first,changed).rows.find(row=>row.stage==='loads');};
 for(const value of[null,undefined,'',false,true,NaN,Infinity,-Infinity,{},[]]){const row=valueCase(value);check(row.target.value===null&&row.delta===null&&!row.directlyComparable,'invalid numeric value invented energy '+String(value));}
 check(valueCase(0).delta===-100&&valueCase('0').target.value===0,'explicit measured zero was lost');
 summary.loads[0].value=100;
 summary.loads[0].unit='J';check(compare(first,changed).rows.find(row=>row.stage==='loads').delta===null,'different energy units were silently converted');
 summary.loads[0].unit='kWh site';check(compare(first,changed).rows.find(row=>row.stage==='loads').delta===null,'explicit thermal/site unit contradiction passed');
 summary.loads[0].unit='kWh thermal';summary.loads[0].scaleDomain='site';check(compare(first,changed).rows.find(row=>row.stage==='loads').delta===null,'wrong scale domain passed');
 summary.loads[0].scaleDomain='thermal';summary.loads[0].unit='kWhₜₕ';check(compare(first,changed).rows.find(row=>row.stage==='loads').delta===0,'equivalent thermal unit notation split comparison');
 summary.loads[0].unit='kWh thermal';summary.loads[0].basis='service_path_allocation';
 let row=compare(first,changed).rows.find(row=>row.stage==='loads');check(row.delta===0&&row.basisMismatch&&!row.directlyComparable,'different basis became missing rows or directly comparable');
 summary.loads[0].basis='reported_variable';summary.loads.push({...summary.loads[0],id:'different-contributor',value:999});row=compare(first,changed).rows.find(row=>row.stage==='loads');check(row.target.ambiguous&&row.target.value===null&&row.delta===null,'ambiguous canonical category summed or chose one record');summary.loads.pop();
 for(const name of['constructor','__proto__','toString']){summary.drivers[0].kind='driver.'+name;const invalid=compare(changed,changed).rows.find(row=>row.stage==='drivers');check(invalid.delta===null&&invalid.category.startsWith('Unclassified driver'),'prototype name resolved as known taxonomy '+name);}
 summary.drivers=clone(source.drivers);summary.carriers[0].kind='energy.natural_gas';check(compare(first,changed).rows.find(row=>row.stage==='carriers').delta===null,'carrier kind contradiction passed');summary.carriers=clone(source.carriers);
 summary.carriers.push({id:'water',level:'carrier',kind:'energy.water.total',carrier:'water',value:100,unit:'m3',scaleDomain:'site',basis:'reported_meter'});
 check(compare(changed,changed).rows.filter(row=>row.stage==='carriers').length===1,'raw water volume became site-energy comparison');
 summary.carriers.at(-1).unit='kWh';summary.carriers.at(-1).basis='derived_ratio';check(compare(changed,changed).rows.some(row=>row.stage==='carriers'&&row.categoryId==='water'&&row.delta===0),'explicit derived water energy equivalent was dropped');
 summary.ratios[0].numeratorUnit='kWh site';check(compare(first,changed).rows.find(row=>row.stage==='ratios').delta===null,'COP numerator domain was ignored');summary.ratios=clone(source.ratios);
 summary.quality.loads={status:'complete',found:4,total:4};row=compare(first,changed).rows.find(row=>row.stage==='loads');check(row.coverageMismatch&&row.baseline.quality.percent===100&&row.target.quality.percent===100,'different source groups hidden by equal percentages');
 summary.quality.loads={status:'not_requested',found:0,total:0};row=compare(first,changed).rows.find(row=>row.categoryId==='loads_coverage');check(row.target.value===null&&row.delta===null&&row.coverageMismatch,'not-requested coverage fabricated0%');
 summary.quality.loads={status:'missing',found:0,total:2};row=compare(first,changed).rows.find(row=>row.categoryId==='loads_coverage');check(row.target.value===0&&row.delta===-100&&row.target.quality.status==='missing','requested but missing outputs lost known0/2 coverage or became not-requested');
 summary.quality.loads={status:'partial',found:1,total:2};row=compare(first,changed).rows.find(row=>row.categoryId==='loads_coverage');check(row.delta===-50&&row.deltaUnit==='pp'&&row.deltaPercent===-50,'coverage arithmetic lost percentage-point unit');
 for(const [field,status]of[['driverToLoadClosedPct','driverToLoadStatus'],['unassignedPct','zoneAllocationStatus']]){summary.quality[field]=0;summary.quality[status]='unavailable';row=compare(first,changed).rows.find(row=>row.categoryId===field);check(row.target.value===null,'unavailable accounting denominator looked like measured0');}
 summary.scope={kind:'zone',zoneName:'Office'};check(read(changed).status==='unavailable','Zone summary silently became Building');summary.scope={kind:'building'};summary.period='M2';check(read(changed).status==='unavailable','month summary silently became annual');summary.period='annual';summary.schema='semantic-idf.energy-explanation-summary/v1';check(read(changed).status==='unavailable','legacy summary used in primary v2 comparison');
 const hugeA=clone(first),hugeB=clone(first);hugeA.purposeResults.energyExplanationSummary.residuals[0].value=-Number.MAX_VALUE;hugeB.purposeResults.energyExplanationSummary.residuals[0].value=Number.MAX_VALUE;
 row=compare(hugeA,hugeB).rows.find(row=>row.kind==='summary'&&row.stage==='residuals');check(row.delta===null&&row.deltaPercent===null&&!row.directlyComparable&&row.warningCodes.includes('numeric_overflow'),'finite inputs overflowed into a valid comparison');
 const ordered=clone(first);ordered.purposeResults.energyExplanationSummary.drivers.push({...ordered.purposeResults.energyExplanationSummary.drivers[0],id:'air',kind:'driver.air.infiltration',value:999});
 const order=compare(ordered,ordered).rows.filter(row=>row.stage==='drivers').map(row=>row.key);ordered.purposeResults.energyExplanationSummary.drivers.reverse();check(json(compare(ordered,ordered).rows.filter(row=>row.stage==='drivers').map(row=>row.key))===json(order)&&compare(ordered,ordered).rows.find(row=>row.stage==='drivers').categoryId==='surface.exterior_walls','input/value order displaced stable category taxonomy');
 if(failures.length)throw Error(failures.join('\n'));
 document.body.dataset.epath170Comparison='passed';document.querySelector('#result').textContent='PASS';
}catch(error){document.body.dataset.epath170Comparison='failed';document.querySelector('#result').textContent=error.stack||String(error);}
</script></body></html>`
