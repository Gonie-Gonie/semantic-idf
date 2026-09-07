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

func TestEPATH150RelatedEntityLabelsBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser related entity verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath150-entities", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath150EntitiesHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath150-entities").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH150 entities browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath150-entities="passed"`) {
		t.Fatalf("EPATH150 entities failed:\n%s", output)
	}
}

const epath150EntitiesHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH150 related entities</title></head>
<body data-epath150-entities="pending"><pre id="result">pending</pre><script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
try {
  const {simulationEnergyInspectorRelatedEntities:resolve}=await import('/src/js/views/simulation-views.js');
  const path={id:'hvac.path.exact',zoneName:'Office',serviceKind:'cooling',delivery:{id:'coil.exact',objectType:'Coil:Cooling:Water',objectName:'Office cooling coil'},airLoop:{id:'loop.exact',name:'Office air loop'}};
  const report={geometry:{surfaces:[{id:'surface.exact',name:'Geometry wall name'}],topology:{nodes:[{id:'zone.office',label:'Office'},{id:'zone.lab',label:'Lab'}],airCouplings:[{id:'air.exact',objectName:'Office–Lab transfer',fromNodeId:'zone.office',toNodeId:'zone.lab'}],connections:[{id:'connection.exact',fromNodeId:'zone.office',toNodeId:'zone.lab'}]}},hvac:{serviceModel:{zoneServices:[{paths:[path,{id:'hvac.path.unrelated',delivery:{id:'coil.unrelated',objectName:'Unrelated coil'}}]}]}}};
  const view={report,simulationEnergyScopeKind:'building',simulationEnergyPeriod:'annual',semanticProjection:{navigation:{entities:[{id:'surface.exact',kind:'surface',label:'Semantic exterior wall'},{id:'source-like.entity',kind:'surface',label:'sql-rdd-hidden'}]}}};
  const item={id:'driver.test',level:'driver',relatedEntityIds:['surface.exact','air.exact','connection.exact','unknown.entity','source-like.entity'],relatedPathIds:[path.id],sourceIds:['sql-rdd-hidden']};
  const sources=[{id:'sql-rdd-hidden',relatedEntityIds:['surface.exact','source-only.unknown']}];
  const before=JSON.stringify({item,sources,view});
  const entities=resolve(item,sources,view),byID=new Map(entities.map(entity=>[entity.id,entity]));
  check(byID.get('surface.exact')?.label==='Semantic exterior wall','exact semantic label did not override geometry label');
  check(byID.get('air.exact')?.label==='Office–Lab transfer','actual air coupling objectName not resolved');
  check(byID.get('connection.exact')?.label.includes('Office / Lab'),'connection did not use resolved endpoint names');
  check(byID.get('loop.exact')?.label==='Office air loop'&&byID.get('coil.exact')?.label==='Office cooling coil','explicit related path lost loop/component names');
  check(!byID.has('coil.unrelated')&&!byID.has('hvac.path.unrelated'),'unrelated same-service HVAC path was inferred');
  check(entities.filter(entity=>entity.id==='surface.exact').length===1,'repeated source entity was not deduplicated');
  check(entities.every(entity=>entity.label!==entity.id&&entity.label!=='sql-rdd-hidden'),'raw entity/source ID leaked as human label');
  check(entities.every(entity=>!entity.existingAirCouplingAction),'Building aggregate acquired arbitrary air-coupling jump');
  const zone=resolve(item,sources,{...view,simulationEnergyScopeKind:'zone'});
  check(zone.filter(entity=>entity.existingAirCouplingAction).map(entity=>entity.id).join(',')==='air.exact','existing Zone air-coupling action was not exact');
  const fallback=resolve(item,sources,{...view,semanticProjection:null});
  check(fallback.find(entity=>entity.id==='surface.exact')?.label==='Geometry wall name','exact report label unavailable without semantic projection');
  check(JSON.stringify({item,sources,view})===before,'related entity lookup mutated source/model payload');
} catch(error) {failures.push(error.stack||String(error));}
document.body.dataset.epath150Entities=failures.length?'failed':'passed';
document.getElementById('result').textContent=failures.length?failures.join('\n'):'passed';
</script></body></html>`
