package frontendchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Hand-built navigation fixtures cannot establish that the resolver understands
// the actual analyzer's target IDs, Profile anchors and HVAC service-path schema.
func TestEPATH151DriverDestinationsFromAnalyzedIDFBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser analyzed IDF destination verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	doc, err := idf.Parse(epath151DriverSchemaIDF)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"geometry":           idf.AnalyzeGeometry(doc),
		"profile":            idf.AnalyzeProfile(doc),
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
	mux.HandleFunc("/epath151-analyzed", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath151DriverSchemaHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=12000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath151-analyzed").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH151 analyzed IDF browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath151-analyzed="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH151 analyzed IDF destinations: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH151 analyzed IDF destinations:\n%s", output)
	}
}

const epath151DriverSchemaHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH151 analyzed IDF</title></head>
<body data-epath151-analyzed="pending"><pre id="result">pending</pre><script type="module">
try {
  const {energyPathDriverDestinations:resolve}=await import('/src/js/energy-path-driver-destinations.js');
  const fixture=await (await fetch('/fixture.json')).json();
  const before=JSON.stringify(fixture), failures=[];
  const check=(condition,message)=>{if(!condition)failures.push(message);};
  const options={...fixture,period:'M1',scope:{kind:'building'},sources:[]};
  const destinations=category=>resolve({id:category,level:'driver',driverCategory:category,period:'M1',value:10},options);
  const candidates=result=>result.groups.flatMap(group=>group.candidates);
  const names=result=>candidates(result).map(candidate=>candidate.label).join(' | ');
  for(const [category,name] of [
    ['surface.exterior_walls','Office Wall'],['surface.roofs','Office Roof'],['surface.ground_floors','Office Floor'],
    ['surface.windows_doors','Office Window'],['surface.interzone','Office Partition'],
    ['internal.people','Office People'],['internal.lighting','Office Lights'],['internal.equipment','Office Equipment'],
    ['air.infiltration','Office Infiltration'],['air.mechanical_ventilation','Office Ventilation'],['air.interzone','Office Mixing']
  ]) {
    const result=destinations(category);
    check(result.status==='available'&&names(result).includes(name),category+' lacks actual named source '+name+': '+names(result));
    for(const candidate of candidates(result)) {
      const entity=fixture.semanticNavigation.entities.find(entity=>entity.id===candidate.entityId);
      const occurrence=fixture.semanticNavigation.occurrences.find(occurrence=>occurrence.occurrenceId===candidate.occurrenceId);
      check(Boolean(entity)&&(!candidate.occurrenceId||occurrence?.entityId===entity.id),'candidate lost actual semantic identity');
      check([...(entity?.viewTargets||[]),...(occurrence?.viewTargets||[])].some(target=>target.view===candidate.view&&target.targetKind===candidate.target.targetKind&&target.targetId===candidate.target.targetId),'candidate fabricated semantic view target');
    }
  }
  const walls=candidates(destinations('surface.exterior_walls'));
  check(walls.some(candidate=>candidate.target.targetKind==='thermal_boundary'),'actual surface boundary navigation missing');
  check(!walls.some(candidate=>candidate.label.includes('Roof')||candidate.label.includes('Partition')),'actual wall category borrowed another envelope type');
  const infiltration=candidates(destinations('air.infiltration'));
  check(infiltration.some(candidate=>candidate.view==='profile')&&infiltration.some(candidate=>candidate.view==='topology'),'ordinary infiltration needs both real Profile source and Zone Topology context');
  const ventilation=candidates(destinations('air.mechanical_ventilation'));
  check(ventilation.some(candidate=>candidate.view==='hvac'),'actual OutdoorAirUnit did not produce an HVAC outdoor-air destination: '+JSON.stringify(fixture.hvac.serviceModel.zoneServices));
  check(ventilation.some(candidate=>candidate.view==='profile'),'actual ventilation Profile target missing');
  const zoneOptions={...options,scope:{kind:'zone',zoneName:'Office'}};
  const zone=resolve({id:'wall',level:'driver',driverCategory:'surface.exterior_walls',period:'M1',zoneName:'Office'},zoneOptions);
  check(candidates(zone).length>0&&candidates(zone).every(candidate=>candidate.zoneName==='Office')&&!names(zone).includes('Lab Wall'),'actual Zone route escaped selected owner');
  check(JSON.stringify(fixture)===before,'resolver mutated actual analyzer payload');
  if(failures.length)throw Error(failures.join('\n'));
  document.body.dataset.epath151Analyzed='passed';document.querySelector('#result').textContent='PASS';
} catch(error) {document.body.dataset.epath151Analyzed='failed';document.querySelector('#result').textContent=error.stack||String(error);}
</script></body></html>`

const epath151DriverSchemaIDF = `
Version, 24.2;
Zone, Office, 0, 0, 0, 0, 1, 1;
Zone, Lab, 0, 0, 0, 0, 1, 1;
Schedule:Constant, Always On, , 1;
Material:NoMass, Insulation, Rough, 2;
Construction, Wall Construction, Insulation;
WindowMaterial:SimpleGlazingSystem, Glass, 2.5, 0.5, 0.6;
Construction, Window Construction, Glass;
BuildingSurface:Detailed, Office Wall, Wall, Wall Construction, Office, , Outdoors, , SunExposed, WindExposed, 0.5, 4,
  0,0,0, 0,0,3, 4,0,3, 4,0,0;
BuildingSurface:Detailed, Lab Wall, Wall, Wall Construction, Lab, , Outdoors, , SunExposed, WindExposed, 0.5, 4,
  5,0,0, 5,0,3, 9,0,3, 9,0,0;
BuildingSurface:Detailed, Office Roof, Roof, Wall Construction, Office, , Outdoors, , SunExposed, WindExposed, 0.5, 4,
  0,0,3, 0,4,3, 4,4,3, 4,0,3;
BuildingSurface:Detailed, Office Floor, Floor, Wall Construction, Office, , Ground, , NoSun, NoWind, 0.5, 4,
  0,0,0, 4,0,0, 4,4,0, 0,4,0;
BuildingSurface:Detailed, Office Partition, Wall, Wall Construction, Office, , Surface, Lab Partition, NoSun, NoWind, 0.5, 4,
  4,0,0, 4,0,3, 4,4,3, 4,4,0;
BuildingSurface:Detailed, Lab Partition, Wall, Wall Construction, Lab, , Surface, Office Partition, NoSun, NoWind, 0.5, 4,
  4,4,0, 4,4,3, 4,0,3, 4,0,0;
FenestrationSurface:Detailed, Office Window, Window, Window Construction, Office Wall, , 0.5, , 1, 4,
  1,0,1, 1,0,2, 2,0,2, 2,0,1;
People, Office People, Office, Always On, People, 5;
Lights, Office Lights, Office, Always On, LightingLevel, 100;
ElectricEquipment, Office Equipment, Office, Always On, EquipmentLevel, 200;
ZoneInfiltration:DesignFlowRate, Office Infiltration, Office, Always On, Flow/Zone, 0.1;
ZoneVentilation:DesignFlowRate, Office Ventilation, Office, Always On, Flow/Zone, 0.2;
ZoneMixing, Office Mixing, Office, Always On, Flow/Zone, 0.1, , , , Lab;
ZoneHVAC:EquipmentConnections, Office, Office HVAC Equipment, Office Supply, Office Exhaust, Office Air Node, Office Return;
ZoneHVAC:EquipmentList, Office HVAC Equipment, SequentialLoad, ZoneHVAC:OutdoorAirUnit, Office OAU, 1, 1, , ;
ZoneHVAC:OutdoorAirUnit, Office OAU, Always On, Office, 0.42, Always On, Office Supply Fan, DrawThrough,
  Office Exhaust Fan, 0.42, Always On, NeutralControl, , , Office Outdoor Air, Office OAU Outlet, Office OAU Inlet, Office Fan Outlet, Office OA Equipment;
Fan:SystemModel, Office Supply Fan;
Fan:ConstantVolume, Office Exhaust Fan;
ZoneHVAC:OutdoorAirUnit:EquipmentList, Office OA Equipment, Coil:Heating:Water, Office Heating Coil;
Coil:Heating:Water, Office Heating Coil;
`
