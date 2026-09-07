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

func TestEPATH161PreparedDriverNavigationBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser prepared driver navigation verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	// Apply every existing category, source, scope, duplicate and occurrence
	// assertion to both the ordinary resolver and the explicit prepared path.
	page := strings.Replace(epath151PureDestinationsHTML,
		`const {energyPathDriverDestinations:destinations}=await import('/src/js/energy-path-driver-destinations.js');`,
		`const {energyPathDriverDestinations:ordinaryDestinations,prepareEnergyPathDriverNavigation:prepare}=await import('/src/js/energy-path-driver-destinations.js');
  let firstPrepared;
  const destinations=(node,options)=>{
    firstPrepared ||= prepare(options);
    const expected=ordinaryDestinations(node,options);
    const prepared=ordinaryDestinations(node,{...options,preparedNavigation:firstPrepared});
    const fresh=ordinaryDestinations(node,{...options,preparedNavigation:prepare(options)});
    check(JSON.stringify(expected)===JSON.stringify(prepared),'reused/stale prepared token changed validated destinations');
    check(JSON.stringify(expected)===JSON.stringify(fresh),'fresh prepared token changed validated destinations');
    return prepared;
  };`, 1)
	page = strings.Replace(page, `  if(failures.length)throw Error(failures.join('\n'));`, epath161PreparedDriverAssertions+`  if(failures.length)throw Error(failures.join('\n'));`, 1)
	if !strings.Contains(page, "let firstPrepared;") || !strings.Contains(page, "routeReads") {
		t.Fatal("EPATH151 fixture integration anchors changed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath161-driver-index", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath161-driver-index").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH161 prepared driver browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath151-destinations="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH161 prepared driver failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH161 prepared driver failed:\n%s", output)
	}
}

const epath161PreparedDriverAssertions = `
  const indexedOptions=clone(options);let routeReads=0;
  const firstEntity=indexedOptions.semanticNavigation.entities[0], originalTargets=firstEntity.viewTargets;
  Object.defineProperty(firstEntity,'viewTargets',{enumerable:true,get(){routeReads++;return originalTargets;}});
  const indexedToken=prepare(indexedOptions), readsAfterPreparation=routeReads;
  check(Object.isFrozen(indexedToken)&&Object.keys(indexedToken).length===0,'prepared token exposes mutable validation indexes');
  const firstIndexed=ordinaryDestinations(node,{...indexedOptions,preparedNavigation:indexedToken});
  for(let selection=0;selection<25;selection++)ordinaryDestinations({...node,value:selection},{...indexedOptions,preparedNavigation:indexedToken,zoneRows:[]});
  check(routeReads===readsAfterPreparation,'prepared selection revisited semantic entity viewTargets');
  check(targets(firstIndexed).includes('wall.office'),'prepared lookup lost verified exterior wall');
  const noSource=ordinaryDestinations({...node,sourceIds:[]},{...indexedOptions,preparedNavigation:indexedToken});
  check(all(noSource).filter(item=>item.target.targetId==='wall.office').every(item=>item.evidenceKind==='category_context'),'prepared navigation cached selected source attribution');
  const scopedIndexed=ordinaryDestinations(node,{...indexedOptions,preparedNavigation:indexedToken,scope:{kind:'zone',zoneName:'Lab'}});
  check(!targets(scopedIndexed).includes('wall.office')&&targets(scopedIndexed).includes('wall.lab'),'prepared navigation cached selected Zone scope');
  check(ordinaryDestinations(node,{...indexedOptions,preparedNavigation:indexedToken,period:'M2'}).status==='unavailable','prepared navigation bypassed period check');
  indexedOptions.semanticNavigation.entities=[];
  check(ordinaryDestinations(node,{...indexedOptions,preparedNavigation:indexedToken}).status==='unavailable','replaced entity collection reused stale prepared routes');

  const changedTopology=clone(options), topologyToken=prepare(changedTopology);
  changedTopology.geometry.topology.boundaries=[];
  check(ordinaryDestinations(node,{...changedTopology,preparedNavigation:topologyToken}).status==='unavailable','replaced topology boundary array reused stale verified records');
  const changedProfile=clone(options), profileToken=prepare(changedProfile);
  changedProfile.profile.zoneProfiles=[];
  check(ordinaryDestinations({...node,driverCategory:'internal.people'},{...changedProfile,preparedNavigation:profileToken}).status==='unavailable','replaced Profile collection reused old item routes');
  const changedOA=clone(options), oaToken=prepare(changedOA);
  changedOA.hvac.serviceModel.zoneServices=[];
  check(!targets(ordinaryDestinations({...node,driverCategory:'air.mechanical_ventilation'},{...changedOA,preparedNavigation:oaToken})).includes('oa.path'),'replaced HVAC service collection reused old outdoor-air route');
  const forged=ordinaryDestinations(node,{...options,preparedNavigation:{routesByTarget:new Map()}});
  check(JSON.stringify(forged)===JSON.stringify(ordinaryDestinations(node,options)),'caller-controlled token replaced validation indexes');
`
