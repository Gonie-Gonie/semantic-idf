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

func TestEPATH161PureSceneSlotBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser current scene slot verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath161-scene-slot", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath161PureSceneSlotHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath161-scene-slot").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH161 pure scene slot browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath161-scene-slot="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH161 pure scene slot failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH161 pure scene slot failed:\n%s", output)
	}
}

const epath161PureSceneSlotHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH161 current scene slot</title></head>
<body data-epath161-scene-slot="pending"><pre id="result">pending</pre><script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const json=JSON.stringify;
const freeze=value=>{if(value&&typeof value==='object'&&!Object.isFrozen(value)){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
try {
  const {createEnergyPathSceneSlot}=await import('/src/js/energy-path-scene-slot.js');
  const explanation={schema:'semantic-idf.energy-explanation/v2',nodes:[{id:'load.cooling',value:400}],links:[{id:'conversion',fromValue:400,toValue:100}],periods:[],zoneResults:[],sources:[],quality:{loads:{status:'complete'}}};
  const result={runId:'actual-run',purposeResults:{energyExplanation:explanation}};
  const context={result,explanation,runId:result.runId,schema:explanation.schema,scopeKind:'building',zoneName:'',period:'annual',service:'all',presentationKey:'en'};
  const original=json(context);freeze(context);
  const slot=createEnergyPathSceneSlot();let builds=0;
  const build=()=>({serial:++builds,graph:{nodes:explanation.nodes,links:explanation.links},layout:{nodes:[]},drawing:{ribbons:[]}});
  check(slot.peek()===null&&slot.peek(context)===null,'empty slot exposed a scene');
  const scene=slot.acquire(context,build);
  check(builds===1&&slot.peek()===scene&&slot.peek({...context})===scene,'first acquire/peek did not retain the exact scene');
  check(scene.graph.nodes===explanation.nodes&&scene.graph.links===explanation.links,'slot cloned canonical geometry or source references');
  check(slot.acquire({...context},()=>{throw Error('should not build');})===scene,'equivalent context object invalidated scene');
  for(let selection=0;selection<1000;selection++){
    const next={...context,selection:'node.'+selection,simulationEnergySelection:'node.'+selection,drawer:{tab:selection%2?'output':'data'},detailsOpen:true,report:{},semanticNavigation:{},theme:selection%2?'dark':'light',callbacks:{onSelect(){}}};
    check(slot.acquire(next,build)===scene,'selection/drawer/report/theme/callbacks changed scene');
  }
  check(builds===1,'selection reuse recomputed geometry');
  check(json(context)===original,'slot mutated its context or canonical result');
  const variants={runId:'another-run',schema:'semantic-idf.energy-explanation/v3',scopeKind:'zone',zoneName:'Office',period:'M2',service:'cooling',presentationKey:'ko',result:{...result},explanation:{...explanation}};
  for(const [field,value] of Object.entries(variants)){
    const isolated=createEnergyPathSceneSlot();const first=isolated.acquire(context,build);const changed={...context,[field]:value};
    check(isolated.peek(changed)===null,'peek returned wrong context for '+field);
    const second=isolated.acquire(changed,build);
    check(first!==second&&isolated.peek(context)===null&&isolated.peek(changed)===second,'identity/scalar change failed invalidation '+field);
  }
  const bounded=createEnergyPathSceneSlot();let boundedBuilds=0;const boundedBuild=()=>({serial:++boundedBuilds});
  const annual=bounded.acquire(context,boundedBuild);
  bounded.acquire({...context,period:'M1'},boundedBuild);
  const annualAgain=bounded.acquire(context,boundedBuild);
  check(boundedBuilds===3&&annualAgain!==annual,'single scene slot became a multi-key history cache');
  bounded.clear();check(bounded.peek()===null&&bounded.peek(context)===null,'clear left a mounted scene available');
  bounded.acquire(context,boundedBuild);check(boundedBuilds===4,'clear did not force one new build');

  const mutableContext={...context};const captured=createEnergyPathSceneSlot();const old=captured.acquire(mutableContext,build);
  mutableContext.period='M3';
  check(captured.peek(mutableContext)===null&&captured.peek(context)===old,'key retained mutable context instead of captured values');
  const newPeriod=captured.acquire(mutableContext,build);check(newPeriod!==old,'mutating a context scalar reused stale scene');

  for(const field of ['nodes','links','periods','zoneResults','sources','relations','relationshipRules','scope','reconciliation','completeness','quality','summary','availableZones','zoneContributions','warnings']){
    const payload={...explanation,[field]:[]};const identity={...context,explanation:payload};const replacement=createEnergyPathSceneSlot();const first=replacement.acquire(identity,build);
    payload[field]=[];
    check(replacement.peek(identity)===null&&replacement.acquire(identity,build)!==first,'in-place root collection replacement retained stale '+field);
  }
  const summaryResult={...result,purposeResults:{...result.purposeResults,energyExplanationSummary:{loads:[]}}};
  const summaryContext={...context,result:summaryResult};const summarySlot=createEnergyPathSceneSlot();const summaryScene=summarySlot.acquire(summaryContext,build);
  summaryResult.purposeResults.energyExplanationSummary={loads:[{value:0}]};
  check(summarySlot.peek(summaryContext)===null&&summarySlot.acquire(summaryContext,build)!==summaryScene,'same-result summary replacement retained stale KPI scene');
  // O(1) identity capture does not walk nested payloads, serialize, or read data.
  const opaqueNode={};Object.defineProperty(opaqueNode,'value',{get(){throw Error('nested node traversed');},enumerable:true});
  const opaqueExplanation={nodes:[opaqueNode],toJSON(){throw Error('payload serialized');}};opaqueExplanation.circular=opaqueExplanation;
  const opaqueContext={...context,explanation:opaqueExplanation};const shallow=createEnergyPathSceneSlot();const opaqueScene=shallow.acquire(opaqueContext,()=>({graph:opaqueExplanation}));
  check(shallow.acquire({...opaqueContext,selection:'other'},build)===opaqueScene,'opaque immutable payload was not reused');
  opaqueExplanation.nodes[0]={id:'explicit in-place edit'};
  shallow.clear();check(shallow.peek()===null,'explicit clear after a nested in-place edit retained old scene');

  const failed=createEnergyPathSceneSlot();failed.acquire(context,build);
  let thrown=false;try{failed.acquire({...context,period:'M4'},()=>{throw Error('expected build failure');});}catch(error){thrown=error.message==='expected build failure';}
  check(thrown&&failed.peek()===null&&failed.peek(context)===null,'failed replacement exposed the unrelated previous scene');
  let emptyBuilds=0;
  for(const empty of [null,undefined]){
    check(failed.acquire(context,()=>{emptyBuilds++;return empty;})===null&&failed.peek()===null,'nullish builder result was cached');
  }
  check(emptyBuilds===2,'empty scenes suppressed later actual builds');
  for(const invalid of [()=>false,()=>1,()=>[],()=>Promise.resolve({}),undefined]){
    failed.acquire(context,build);let rejected=false;
    try{failed.acquire({...context,period:'M5'},invalid);}catch(error){rejected=error instanceof TypeError;}
    check(rejected&&failed.peek()===null,'non-synchronous scene or missing builder retained misleading availability');
  }
  const recovered=failed.acquire(context,build);check(recovered&&failed.peek(context)===recovered,'slot did not recover after build errors');
  if(failures.length)throw Error(failures.join('\n'));
  document.body.dataset.epath161SceneSlot='passed';document.querySelector('#result').textContent='PASS';
}catch(error){document.body.dataset.epath161SceneSlot='failed';document.querySelector('#result').textContent=error.stack||String(error);}
</script></body></html>`
