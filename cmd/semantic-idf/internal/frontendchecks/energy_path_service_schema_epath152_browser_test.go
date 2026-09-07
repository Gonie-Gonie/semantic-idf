package frontendchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEPATH152ServiceDestinationsFromAnalyzedOfficeBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser actual office HVAC destination verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	input, err := os.ReadFile(repoPath("frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := idf.Parse(string(input))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"hvac":               idf.AnalyzeHVAC(doc),
		"semanticNavigation": idf.BuildSemanticModel(doc, idf.SemanticYAMLMetadata{}).Navigation,
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/fixture.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	})
	mux.HandleFunc("/epath152-analyzed", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath152ServiceSchemaHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=16000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath152-analyzed").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH152 analyzed office browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath152-analyzed="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH152 analyzed office destinations: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH152 analyzed office destinations:\n%s", output)
	}
}

const epath152ServiceSchemaHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH152 analyzed office</title></head>
<body data-epath152-analyzed="pending"><pre id="result">pending</pre><script type="module">
try {
  const {energyPathServiceDestinations:resolve}=await import('/src/js/energy-path-service-destinations.js');
  const fixture=await (await fetch('/fixture.json')).json(), before=JSON.stringify(fixture),failures=[];
  const check=(condition,message)=>{if(!condition)failures.push(message);};
  const paths=(fixture.hvac.serviceModel?.zoneServices||[]).flatMap(zone=>zone.paths||[]);
  const path=paths.find(path=>path.serviceKind==='cooling'&&path.zoneName&&path.airLoop&&path.plantLoop);
  if(!path)throw Error('Vendored Office must expose a real cooling path with both AirLoop and PlantLoop');
  const node={id:'end_use.cooling',level:'end_use',endUse:'cooling',serviceKind:'cooling',period:'M1',scaleDomain:'site',unit:'kWh',value:10,relatedPathIds:[path.id],sourceIds:[]};
  const options={...fixture,nodes:[node],links:[],sources:[],scope:{kind:'building'},period:'M1',service:'cooling'};
  const all=result=>result.groups.flatMap(group=>group.candidates).filter(candidate=>candidate.kind==='hvac');
  let result=resolve(node,options),candidates=all(result);
  check(new Set(candidates.map(candidate=>candidate.target.targetId)).size===candidates.length,'Actual physical HVAC destinations repeat across semantic entity/field routes: '+JSON.stringify(candidates.map(candidate=>({entityId:candidate.entityId,target:candidate.target,label:candidate.label}))));
  check(candidates.some(candidate=>['service-path','hvac-path'].includes(candidate.target.targetKind)&&candidate.target.targetId===path.id),'Actual Office service path target missing: '+JSON.stringify({path,result}));
  for(const reference of[path.airLoop,path.plantLoop]) {
    const loop=fixture.hvac.loops.find(loop=>loop.name===reference.name&&loop.type===reference.type);
    check(Boolean(loop),'Office fixture actual loop ref is unresolved '+reference.name);
    check(candidates.some(candidate=>candidate.target.targetKind==='hvac-loop'&&candidate.label.includes(reference.name)),'Explicit actual loop target missing '+reference.name+': '+JSON.stringify(candidates));
  }
  for(const candidate of candidates) {
    const entity=fixture.semanticNavigation.entities.find(entity=>entity.id===candidate.entityId);
    const occurrence=fixture.semanticNavigation.occurrences.find(occurrence=>occurrence.occurrenceId===candidate.occurrenceId);
    check(Boolean(entity)&&(!candidate.occurrenceId||occurrence?.entityId===entity.id),'Actual semantic entity/occurrence mismatch');
    check([...(entity?.viewTargets||[]),...(occurrence?.viewTargets||[])].some(target=>target.view===candidate.view&&target.targetKind===candidate.target.targetKind&&target.targetId===candidate.target.targetId),'Physical destination invented a semantic target');
    check(candidate.pathIds.length>0&&candidate.pathIds.every(id=>id===path.id),'Explicit selected path acquired unrelated service ownership');
  }
  const zoneNode={...node,zoneName:path.zoneName};
  result=resolve(zoneNode,{...options,nodes:[zoneNode],scope:{kind:'zone',zoneName:path.zoneName}});
  check(all(result).length>0&&all(result).every(candidate=>candidate.zoneName===path.zoneName),'Actual Zone context was lost');
  const component=candidates.find(candidate=>candidate.target.targetKind==='hvac-component');
  check(Boolean(component),'Actual Office path must expose a connected component');
  if(component) {
    const componentNode={...zoneNode,relatedPathIds:[],sourceIds:['component-observation']};
    const componentSource={id:'component-observation',zoneName:path.zoneName,relatedEntityIds:[component.entityId]};
    const viaComponent=all(resolve(componentNode,{...options,nodes:[componentNode],sources:[componentSource],scope:{kind:'zone',zoneName:path.zoneName}}));
    check(viaComponent.some(candidate=>candidate.target.targetId===path.id),'Exact semantic component source did not resolve its actual report HVAC path');
    check(viaComponent.every(candidate=>candidate.zoneName===path.zoneName),'Shared component source escaped selected Zone');
  }
  const other=paths.find(other=>other.zoneName!==path.zoneName&&other.serviceKind==='cooling');
  if(!other)throw Error('Vendored Office must expose a second cooled Zone');
  const wrongZone={...zoneNode,relatedPathIds:[other.id]};
  check(all(resolve(wrongZone,{...options,nodes:[wrongZone],scope:{kind:'zone',zoneName:path.zoneName}})).length===0,'Actual related path from a different Zone was accepted');
  const heating={...node,endUse:'heating',serviceKind:'heating'};
  check(all(resolve(heating,{...options,nodes:[heating],service:'heating'})).length===0,'Actual cooling path was reused for heating');
  check(JSON.stringify(fixture)===before,'Service resolver mutated analyzed Office payload');
  if(failures.length)throw Error(failures.join('\n'));
  document.body.dataset.epath152Analyzed='passed';document.querySelector('#result').textContent='PASS';
} catch(error) {document.body.dataset.epath152Analyzed='failed';document.querySelector('#result').textContent=error.stack||String(error);}
</script></body></html>`
