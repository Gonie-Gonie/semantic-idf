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

func TestEPATH160PureStateMigrationBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser state migration verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath160-state", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath160PureStateHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath160-state").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH160 pure state browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath160-state="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH160 pure state failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH160 pure state failed:\n%s", output)
	}
}

const epath160PureStateHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH160 state migration</title></head>
<body data-epath160-state="pending"><pre id="result">pending</pre><script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const json=JSON.stringify;
const freeze=value=>{if(value&&typeof value==='object'&&!Object.isFrozen(value)){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
try {
  const {migrateEnergyPathState:migrate}=await import('/src/js/energy-path-state.js');
  const {state}=await import('/src/js/state.js');
  const defaults={simulationEnergyScopeKind:'building',simulationEnergyZoneName:'',simulationEnergyPeriod:'annual',simulationEnergyService:'all',simulationEnergySelection:'',simulationEnergyDetailsOpen:false};
  const keys=Object.keys(defaults).sort();
  const emptyDrawer={tab:'data',stage:'',outputSource:''};
  check(json(migrate())===json({primary:defaults,drawer:emptyDrawer}),'default migration differs from exact six primary defaults');
  check(json(Object.keys(state).filter(key=>key.startsWith('simulationEnergy')).sort())===json(keys),'state.js has extra primary Energy or drawer/focus fields');
  check(json(Object.fromEntries(Object.keys(defaults).map(key=>[key,state[key]])))===json(defaults),'actual state.js defaults drifted from migration defaults');
  for(const input of [null,undefined,false,17,'cached text',[],{simulationEnergyPeriod:NaN}]){
    check(json(migrate(input))===json({primary:defaults,drawer:emptyDrawer}),'invalid cache container/period invented state '+String(input));
  }
  const current={energyScopeKind:'zone',energyZoneName:'Office',energyPeriod:'M2',energyService:'cooling',energySelection:'load.cooling.office',energyDetailsOpen:true,energyDrawer:{tab:'output',stage:'loads',outputSource:'sql-rdd-11'}};
  const snapshot=json(current);freeze(current);
  let result=migrate(current);
  check(result.primary.simulationEnergyScopeKind==='zone'&&result.primary.simulationEnergyZoneName==='Office'&&result.primary.simulationEnergyPeriod==='M2'&&result.primary.simulationEnergyService==='cooling'&&result.primary.simulationEnergySelection==='load.cooling.office'&&result.primary.simulationEnergyDetailsOpen,'canonical context lost one of six primary fields');
  check(json(result.drawer)===json(current.energyDrawer),'nested local drawer state was not restored');
  check(json(current)===snapshot,'migration mutated cached workspace');
  check(json(migrate(result))===json(result)&&json(migrate(JSON.parse(json(result))))===json(result),'migration is not idempotent across JSON roundtrip');
  result.primary.simulationEnergyPeriod='M8';result.drawer.tab='data';
  check(migrate(current).primary.simulationEnergyPeriod==='M2'&&migrate(current).drawer.tab==='output','returned objects alias cached objects');
  for(const oldView of ['sources','reconciliation']){
    result=migrate({energyScopeKind:undefined,energyZoneName:undefined,energyFocusMode:'zone',energyZoneFocus:'Office',energyView:oldView,energyPeriod:'m3',energyService:'HEATING',energySelection:'original.node'});
    check(result.primary.simulationEnergyScopeKind==='zone'&&result.primary.simulationEnergyZoneName==='Office'&&result.primary.simulationEnergyPeriod==='M3'&&result.primary.simulationEnergyService==='heating'&&result.primary.simulationEnergyDetailsOpen&&result.drawer.tab==='data','legacy Zone/'+oldView+' failed migration');
  }
  const obsolete={simulationEnergyView:'sankey',simulationEnergySankeyMode:'compact',simulationEnergySignMode:'signed',simulationEnergyNodeLimit:40,simulationEnergyFocusMode:'service_path',simulationEnergyServicePathFocus:'some.path',simulationEnergyLoopFocus:'some.loop',energyFocusMode:'loop',energyServicePathFocus:'another.path',energyLoopFocus:'another.loop'};
  check(json(migrate(obsolete))===json({primary:defaults,drawer:emptyDrawer}),'obsolete view/render/path/loop focus became new active state or guessed a node');
  const oldState={simulationEnergyFocusMode:'zone',simulationEnergyZoneFocus:'Lab',simulationEnergyView:'sources',simulationEnergyDetailsTab:'output',simulationEnergyDetailsStage:'endUses',simulationEnergyOutputSource:'source.lab'};
  result=migrate(oldState);
  check(result.primary.simulationEnergyZoneName==='Lab'&&result.primary.simulationEnergyDetailsOpen&&result.drawer.tab==='output'&&result.drawer.stage==='endUses'&&result.drawer.outputSource==='source.lab','old direct state fields lost read compatibility');
  result=migrate({...current,simulationEnergyScopeKind:'building',simulationEnergyZoneName:'',simulationEnergyPeriod:'annual',simulationEnergyService:'all',simulationEnergySelection:'',simulationEnergyDetailsOpen:false});
  check(json(result.primary)===json(defaults),'direct current six state fields did not override snapshot aliases, including false/empty values');
  result=migrate({energyScopeKind:null,energyFocusMode:'zone',energyZoneName:null,energyZoneFocus:'Office',energyPeriod:'M13',energyService:4,energySelection:8,energyDetailsOpen:'true',energyView:'sources',energyDrawer:null,energyDetailsTab:'output',energyOutputSource:'stale'});
  check(json(result)===json({primary:defaults,drawer:emptyDrawer}),'explicit invalid modern values revived stale legacy fields or coerced booleans/IDs');
  result=migrate({...current,energyDrawer:{tab:null,stage:'sources',outputSource:42},energyDetailsTab:'output',energyDetailsStage:'drivers',energyOutputSource:'stale'});
  check(json(result.drawer)===json(emptyDrawer),'present malformed nested drawer fell back to stale flat drawer state');
  check(migrate({energyScopeKind:'zone',energyZoneName:''}).primary.simulationEnergyScopeKind==='building','empty Zone produced an active unnamed Zone scope');
  result=migrate({energyScopeKind:'zone',energyZoneName:'Not loaded yet',energySelection:'pending.exact.node',energyDrawer:{outputSource:'pending.source'}});
  check(result.primary.simulationEnergyScopeKind==='zone'&&result.primary.simulationEnergyZoneName==='Not loaded yet'&&result.primary.simulationEnergySelection==='pending.exact.node'&&result.drawer.outputSource==='pending.source','data-independent migration erased IDs before the real result could rehydrate');
  for(let month=1;month<=12;month++)check(migrate({energyPeriod:'m'+month}).primary.simulationEnergyPeriod==='M'+month,'valid explicit month lost '+month);
  for(const invalid of ['monthly','M0','M01','M13',true,{},null])check(migrate({energyPeriod:invalid}).primary.simulationEnergyPeriod==='annual','invalid period claimed a reported month '+String(invalid));
  const inherited=Object.create({simulationEnergyPeriod:'M7',simulationEnergyDetailsOpen:true,energyDrawer:{tab:'output'},primary:{simulationEnergyPeriod:'M8'}});
  check(json(migrate(inherited))===json({primary:defaults,drawer:emptyDrawer}),'inherited fields escaped the explicit cache allowlist');
  check(json(migrate({energyDrawer:Object.create({tab:'output',stage:'drivers',outputSource:'inherited'})}).drawer)===json(emptyDrawer),'inherited local drawer fields escaped the allowlist');
  for(const stage of ['__proto__','constructor','toString'])check(migrate({energyDrawer:{stage}}).drawer.stage==='','unknown stage resolved through Object prototype '+stage);
  const hostile=JSON.parse('{"__proto__":{"polluted":true},"constructor":{"prototype":{"polluted":true}},"energyScopeKind":"building","unrelated":99}');
  check(json(migrate(hostile))===json({primary:defaults,drawer:emptyDrawer})&&!{}.polluted,'unknown keys leaked into primary state or prototype');
  for(const value of [current,oldState,obsolete,hostile,result]){
    const migrated=migrate(value);check(json(Object.keys(migrated.primary).sort())===json(keys)&&json(Object.keys(migrated.drawer).sort())===json(['outputSource','stage','tab']),'migration output field allowlist expanded');
    check(json(migrate(migrated))===json(migrated),'repeated migration changed normalized state');
  }
  if(failures.length)throw Error(failures.join('\n'));
  document.body.dataset.epath160State='passed';document.querySelector('#result').textContent='PASS';
}catch(error){document.body.dataset.epath160State='failed';document.querySelector('#result').textContent=error.stack||String(error);}
</script></body></html>`
