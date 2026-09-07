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

func TestEPATH171PureBatchExportProjectionBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless Energy Path workbook projection verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath171-projection", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, "<!doctype html><html><body>"+epath170BatchSetupHTML+epath171ExportProjectionHTML+"</body></html>")
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath171-projection").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-epath171-projection="passed"`) {
		if start := strings.Index(string(output), `<pre id="result">`); start >= 0 {
			if end := strings.Index(string(output)[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH171 pure projection: %v\n%s", err, output[start:start+end])
			}
		}
		t.Fatalf("EPATH171 pure projection: %v\n%s", err, output)
	}
}

const epath171ExportProjectionHTML = `<pre id="result">pending</pre><script type="module">
import { energyPathBatchExport, ENERGY_PATH_BATCH_EXPORT_SCHEMA } from "/src/js/energy-path-batch-export.js";
import { energyPathBatchComparison } from "/src/js/energy-path-batch-comparison.js";
const errors=[],check=(value,message)=>{if(!value)errors.push(message);};
const canonical=value=>Array.isArray(value)?value.map(canonical):value&&typeof value==="object"?Object.fromEntries(Object.keys(value).sort().map(key=>[key,canonical(value[key])])):value;
const equal=(left,right)=>JSON.stringify(canonical(left))===JSON.stringify(canonical(right));
try {
 const fixture=window.epath170,result=fixture.payload,runs=result.results;
 const pair={baselineRowId:runs[0].runId,targetRowId:runs[1].runId};
 const snapshot=energyPathBatchExport(result,pair),actual=energyPathBatchComparison(runs[0],runs[1]);
 check(snapshot.schema===ENERGY_PATH_BATCH_EXPORT_SCHEMA&&snapshot.scope==="building"&&snapshot.period==="annual","wrong export schema/scope/period");
 check(snapshot.runs.length===100&&snapshot.runs.every((run,index)=>run.resultIndex===index&&run.rowId===runs[index].runId),"run order/index/identity lost");
 check(snapshot.comparison.status===actual.status&&snapshot.comparison.reason===actual.reason&&snapshot.comparison.rows.length===actual.rows.length,"comparison status/rows changed");
 for(let i=0;i<actual.rows.length;i++){
  const projected=snapshot.comparison.rows[i],visible=actual.rows[i];
  for(const key of Object.keys(projected))check(equal(projected[key],visible[key]),"export recomputed or lost UI field "+visible.key+":"+key);
 }
 for(let index=0;index<runs.length;index++){
  const own=energyPathBatchComparison(runs[index],runs[index]),exported=snapshot.runs[index];
  check(exported.status===own.status&&exported.reason===own.reason&&exported.rows.length===own.rows.length,"per-run availability/row count changed "+index);
  own.rows.forEach((row,i)=>check(equal(row.baseline,exported.rows[i].baseline),"per-run nullable evidence changed "+index+":"+row.key));
 }
 const serialized=JSON.stringify(snapshot),decoded=JSON.parse(serialized);
 check(!/SOURCE_PRIVATE|NODE_PRIVATE|EDGE_PRIVATE|ZONE_PRIVATE/.test(serialized),"raw graph/source/zone identity leaked into default projection");
 const row=(category,stage="endUses")=>decoded.comparison.rows.find(item=>item.categoryId===category&&item.stage===stage);
 check(row("refrigeration").baseline.value===0&&row("refrigeration").delta===5&&row("refrigeration").deltaPercent===null,"known zero/undefined relative change lost across JSON");
 check(row("water_systems").target.value===null&&row("water_systems").target.invalid&&row("water_systems").delta===null,"invalid nullable value became zero");
 check(row("equipment").target.missing&&row("equipment").target.value===null&&row("equipment").delta===null,"missing category was synthesized");
 check(row("cooling").basisMismatch&&row("cooling").coverageMismatch&&!row("cooling").directlyComparable&&row("cooling").delta===-5,"measured/allocated warning or visible arithmetic lost");
 const coverage=decoded.comparison.rows.find(item=>item.kind==="quality"&&item.categoryId==="endUses_coverage");
 check(coverage.delta===-50&&coverage.deltaUnit==="pp"&&coverage.baseline.quality.found===2&&coverage.target.quality.found===1,"coverage count/percentage-point delta lost");
 check(snapshot.runs[3].status==="unavailable"&&snapshot.runs[3].reason==="building_annual_only"&&!snapshot.runs[3].rows.length,"Zone summary was reaggregated for export");
 check(snapshot.runs[4].status==="unavailable"&&snapshot.runs[5].status==="unavailable","monthly/legacy summary silently fell back");
 const unitPair=energyPathBatchExport(result,{...pair,targetRowId:runs[2].runId});
 check(unitPair.comparison.rows.find(item=>item.stage==="carriers"&&item.categoryId==="electricity").delta===null,"incompatible units exported a delta");
 const duplicate=energyPathBatchExport(result,{...pair,targetRowId:runs[98].runId});
 check(duplicate.runs.length===100&&duplicate.runs[98].status==="ready"&&duplicate.runs[99].status==="ready","duplicate IDs prevented independently indexed summary exports");
 check(duplicate.comparison.status==="unavailable"&&duplicate.comparison.reason==="ambiguous_comparison"&&!duplicate.comparison.rows.length,"duplicate comparison ID selected an arbitrary run");
 for(const [input,reason] of [[{},"missing_comparison"],[{baselineRowId:pair.baselineRowId},"missing_comparison"],[{...pair,targetRowId:pair.baselineRowId},"same_comparison_run"],[{...pair,targetRowId:"stale-run"},"missing_comparison_run"]]){
  const absent=energyPathBatchExport(result,input);check(absent.comparison.status==="unavailable"&&absent.comparison.reason===reason&&!absent.comparison.rows.length,"missing/stale selection silently chose first pair: "+reason);
 }
 check(energyPathBatchExport({results:[]})===null&&energyPathBatchExport({results:[{purposeResults:{energyExplanationSummary:{},energyExplanation:{}}}]})===null,"non-Energy export changed route");
 check(energyPathBatchExport({results:[{purposeResults:{energyExplanationSummary:{schema:"semantic-idf.energy-explanation-summary/v1"},energyExplanation:{schema:"semantic-idf.energy-explanation/v1"}}}]})===null,"genuine v1 export masqueraded as v2");
 const graphOnly=energyPathBatchExport({results:[{runId:"graph-only",purposeResults:{energyExplanation:{schema:"semantic-idf.energy-explanation/v2"}}}]});
 check(graphOnly.runs[0].status==="unavailable"&&!graphOnly.runs[0].rows.length,"graph-only result fabricated a summary");
 const emptyProjection=energyPathBatchExport({results:[null,runs[0]]});check(emptyProjection.runs[0].status==="unavailable"&&emptyProjection.runs[1].resultIndex===1,"empty result lost ordered identity");
 const original=JSON.stringify(snapshot.runs[0]);snapshot.comparison.rows[0].baseline.quality.status="changed";snapshot.comparison.rows[0].warningCodes.push("changed");
 check(JSON.stringify(snapshot.runs[0])===original&&JSON.stringify(result)===fixture.rawJSON,"projection aliases another run or immutable source");
 check(fixture.calls.select===0&&fixture.calls.run===0&&fixture.calls.forbidden===0,"export projection triggered backend work");
 if(errors.length)throw new Error(errors.slice(0,12).join("\n")+"\nTotal failures: "+errors.length);
 document.body.dataset.epath171Projection="passed";document.getElementById("result").textContent="100 indexed runs; exact shared UI arithmetic; zero/null/missing/invalid/coverage/basis/unit fidelity; duplicate/stale selection rejection; no backend work or mutation.";
}catch(error){document.body.dataset.epath171Projection="failed";document.getElementById("result").textContent=String(error.stack||error);}
</script>`
