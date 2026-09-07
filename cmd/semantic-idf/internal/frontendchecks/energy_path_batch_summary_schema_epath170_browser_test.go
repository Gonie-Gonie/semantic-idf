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

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

// The numbers are explicitly synthetic, but every summary below is produced by
// the real typed-v2 read/normalization/summary boundary, not a JavaScript facsimile.
func epath170CanonicalBatchRun(t *testing.T, prefix string, scale, lighting float64, loadBasis, loadStatus string) simulation.SimulationRunResult {
	t.Helper()
	id := func(name string) string { return prefix + "." + name }
	node := func(name, level, kind, domain, basis string, value float64) simulation.EnergyExplanationNode {
		return simulation.EnergyExplanationNode{
			ID: id(name), Level: level, Kind: kind, Label: prefix + " " + name,
			Value: value * scale, DisplayValue: value * scale, RawValue: value * scale,
			EffectiveValue: value * scale, AllocatedValue: value * scale, Multiplier: 1,
			Unit: "kWh", ScaleDomain: domain, Basis: basis, AggregationBasis: "model_total",
			Period: "annual", SourceIDs: []string{id("source." + name)},
		}
	}
	walls := node("walls", "driver", "driver.surface.exterior_walls", "thermal", "heat_balance_share", 60)
	walls.DriverCategory, walls.ServiceKind, walls.AllocationApplied = "surface.exterior_walls", "cooling", true
	people := node("people", "driver", "driver.internal.people", "thermal", "heat_balance_share", 40)
	people.DriverCategory, people.ServiceKind, people.AllocationApplied = "internal.people", "cooling", true
	roofs := node("roofs", "driver", "driver.surface.roofs", "thermal", "heat_balance_share", 80)
	roofs.DriverCategory, roofs.ServiceKind, roofs.AllocationApplied = "surface.roofs", "heating", true
	coolingLoad := node("cooling.load", "load", "load.cooling", "thermal", loadBasis, 100)
	coolingLoad.ServiceKind = "cooling"
	heatingLoad := node("heating.load", "load", "load.heating", "thermal", loadBasis, 80)
	heatingLoad.ServiceKind = "heating"
	cooling := node("cooling", "end_use", "energy.cooling", "site", "reported_meter", 25)
	cooling.EndUse, cooling.ServiceKind = "cooling", "cooling"
	heating := node("heating", "end_use", "energy.heating", "site", "reported_meter", 100)
	heating.EndUse, heating.ServiceKind = "heating", "heating"
	lights := node("lighting", "end_use", "energy.lighting", "site", "reported_meter", lighting)
	lights.EndUse = "lighting"
	electricity := node("electricity", "carrier", "energy.electricity", "site", "reported_meter", 40)
	electricity.Carrier, electricity.EndUse, electricity.MeterHierarchyLevel = "electricity", "total", "facility_total"
	gas := node("gas", "carrier", "energy.natural_gas", "site", "reported_meter", 100)
	gas.Carrier, gas.EndUse, gas.MeterHierarchyLevel = "natural_gas", "total", "facility_total"
	zero := node("zero.equipment", "end_use", "energy.equipment", "site", "reported_meter", 0)
	zero.EndUse = "equipment"
	link := func(name, from, to, relation, basis, service string, fromValue, toValue float64) simulation.EnergyPathLink {
		return simulation.EnergyPathLink{
			ID: id("link." + name), FromID: id(from), ToID: id(to), Relation: relation, Basis: basis,
			FromValue: fromValue * scale, ToValue: toValue * scale, FromUnit: "kWh", ToUnit: "kWh",
			Period: "annual", ServiceKind: service, SourceIDs: []string{id("source." + from), id("source." + to)},
		}
	}
	loadFound := 2
	if loadStatus != "complete" {
		loadFound = 1
	}
	explanation := simulation.EnergyExplanationResult{
		Schema: "semantic-idf.energy-explanation/v2", Scope: simulation.EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
		Nodes: []simulation.EnergyExplanationNode{walls, people, roofs, coolingLoad, heatingLoad, cooling, heating, lights, electricity, gas, zero},
		Links: []simulation.EnergyPathLink{
			link("walls", "walls", "cooling.load", "driver_to_load", "heat_balance_share", "cooling", 60, 60),
			link("people", "people", "cooling.load", "driver_to_load", "heat_balance_share", "cooling", 40, 40),
			link("roofs", "roofs", "heating.load", "driver_to_load", "heat_balance_share", "heating", 80, 80),
			link("cooling.conversion", "cooling.load", "cooling", "load_to_end_use", loadBasis, "cooling", 100, 25),
			link("heating.conversion", "heating.load", "heating", "load_to_end_use", loadBasis, "heating", 80, 100),
			link("cooling.carrier", "cooling", "electricity", "end_use_to_carrier", "reported_meter", "cooling", 25, 25),
			link("heating.carrier", "heating", "gas", "end_use_to_carrier", "reported_meter", "heating", 100, 100),
			link("lighting.carrier", "lighting", "electricity", "end_use_to_carrier", "reported_meter", "", lighting, lighting),
		},
		Completeness: simulation.EnergyCompleteness{
			Status: "complete", HeatDrivers: simulation.EnergyCompletenessLevel{Level: "heat", Status: "complete", Found: 3, Total: 3},
			DeliveredLoad: simulation.EnergyCompletenessLevel{Level: "load", Status: loadStatus, Found: loadFound, Total: 2},
			EnergyUse:     simulation.EnergyCompletenessLevel{Level: "energy", Status: "complete", Found: 5, Total: 5},
		},
	}
	for _, item := range explanation.Nodes {
		name := item.Label
		isMeter := item.ScaleDomain == "site"
		sourceType := "sql_variable"
		if isMeter {
			sourceType = "sql_meter"
		}
		switch item.ID {
		case electricity.ID:
			name = "Electricity:Facility"
		case gas.ID:
			name = "NaturalGas:Facility"
		case cooling.ID:
			name = "Cooling:Electricity"
		case heating.ID:
			name = "Heating:NaturalGas"
		case lights.ID:
			name = "InteriorLights:Electricity"
		}
		explanation.Sources = append(explanation.Sources, simulation.EnergyDataSource{
			ID: item.SourceIDs[0], SourceType: sourceType, Name: name, IsMeter: isMeter,
			RawValue: item.Value, EffectiveValue: item.Value, NormalizedUnit: "kWh", SourceUnit: "J",
			ReportingFrequency: "Annual",
		})
	}
	encoded, err := json.Marshal(struct {
		Explanation simulation.EnergyExplanationResult `json:"energyExplanation"`
	}{Explanation: explanation})
	if err != nil {
		t.Fatal(err)
	}
	var bundle simulation.PurposeResultBundle
	if err := json.Unmarshal(encoded, &bundle); err != nil {
		t.Fatal(err)
	}
	return simulation.SimulationRunResult{RunID: prefix, Filename: prefix + ".idf", Status: "succeeded", PurposeResults: &bundle}
}

func TestEPATH170CanonicalBatchSummaryFixture(t *testing.T) {
	run := epath170CanonicalBatchRun(t, "baseline", 1, 10, "direct_zone_energy", "complete")
	summary := run.PurposeResults.EnergyExplanationSummary
	if summary.Schema != "semantic-idf.energy-explanation-summary/v2" || summary.Scope.Kind != "building" || summary.Period != "annual" {
		t.Fatalf("real summary identity = %#v", summary)
	}
	if len(summary.Drivers) != 3 || len(summary.Loads) != 2 || len(summary.EndUses) != 3 || len(summary.Carriers) != 2 || len(summary.Ratios) != 2 || len(summary.Residuals) != 1 {
		t.Fatalf("real summary groups = %#v", summary)
	}
	if summary.Quality == nil || summary.Quality.Loads.Status != "complete" || summary.Quality.Loads.Found != 2 || summary.Quality.Loads.Total != 2 {
		t.Fatalf("real requested-output quality = %#v", summary.Quality)
	}
	for _, item := range summary.Drivers {
		if !strings.HasPrefix(item.Kind, "driver.") || item.ServiceKind == "" || item.Basis != "heat_balance_share" {
			t.Fatalf("actual driver semantic fields = %#v", item)
		}
	}
	for _, item := range summary.EndUses {
		if item.Carrier != "" || item.EndUse == "equipment" {
			t.Fatalf("carrier-neutral / pruned-zero summary contract = %#v", item)
		}
	}
}

func TestEPATH170ComparisonFromCanonicalGoSummaryBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser real Go summary comparison verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	baseline := epath170CanonicalBatchRun(t, "baseline", 1, 10, "direct_zone_energy", "complete")
	prunedZero := epath170CanonicalBatchRun(t, "pruned-zero", 1, 0, "direct_zone_energy", "complete")
	// The producer intentionally omits zero summary contributors. A stored-v2
	// summary may instead explicitly report value:0; that is distinct from an
	// absent category, and the actual Go JSON wire must retain the explicit zero.
	zeroBundle := *prunedZero.PurposeResults
	zeroBundle.EnergyExplanationSummary.EndUses = append(append([]simulation.EnergyExplanationSummaryItem(nil), zeroBundle.EnergyExplanationSummary.EndUses...), simulation.EnergyExplanationSummaryItem{
		ID: "stored.explicit.zero", Level: "end_use", Kind: "energy.lighting", Label: "Reported zero lighting",
		EndUse: "lighting", Value: 0, Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", AggregationBasis: "model_total",
	})
	explicitZero := prunedZero
	explicitZero.PurposeResults = &zeroBundle
	fixtures := map[string]simulation.SimulationRunResult{
		"baseline":   baseline,
		"target":     epath170CanonicalBatchRun(t, "target", 1.5, 10, "direct_zone_energy", "complete"),
		"basis":      epath170CanonicalBatchRun(t, "allocated-load", 1, 10, "service_path_allocation", "complete"),
		"coverage":   epath170CanonicalBatchRun(t, "partial-load", 1, 10, "direct_zone_energy", "partial"),
		"prunedZero": prunedZero, "explicitZero": explicitZero,
	}
	payload, err := json.Marshal(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath170-summary.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	})
	mux.HandleFunc("/epath170-summary", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath170CanonicalSummaryHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=12000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath170-summary").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH170 real summary browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath170-summary="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH170 real summary failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH170 real summary failed:\n%s", output)
	}
}

const epath170CanonicalSummaryHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH170 actual Go summary comparison</title></head>
<body data-epath170-summary="pending"><pre id="result">pending</pre><script type="module">
const failures=[];const check=(ok,message)=>{if(!ok)failures.push(message);};
const close=(left,right)=>typeof left==='number'&&Math.abs(left-right)<1e-8;
const clone=value=>JSON.parse(JSON.stringify(value));
const freeze=value=>{if(value&&typeof value==='object'&&!Object.isFrozen(value)){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
try {
  const {energyPathBatchSummary,energyPathBatchComparison:compare}=await import('/src/js/energy-path-batch-comparison.js');
  const runs=await fetch('/epath170-summary.json').then(response=>response.json());
  const original=JSON.stringify(runs);freeze(runs);
  const baselineSummary=runs.baseline.purposeResults.energyExplanationSummary;
  check(energyPathBatchSummary(runs.baseline).status==='ready','real Go annual Building summary was unavailable');
  check(baselineSummary.drivers.every(item=>!Object.hasOwn(item,'driverCategory')&&item.kind.startsWith('driver.')),'fixture no longer proves actual summary kind-only driver classification');
  const result=compare(runs.baseline,runs.target);
  const stages=['drivers','loads','endUses','carriers','ratios','residuals'];
  const summaryRows=result.rows.filter(row=>row.kind==='summary');
  check(result.status==='ready','real Go summaries were not comparable');
  check(summaryRows.length===13,'semantic categories did not match across independent actual node/link/source IDs: '+summaryRows.length);
  for(const stage of stages){
    const rows=summaryRows.filter(row=>row.stage===stage);
    check(rows.length===baselineSummary[stage].length,'real '+stage+' fields split or merged semantic rows');
    for(const row of rows){
      check(!row.baseline.missing&&!row.target.missing&&!row.baseline.ambiguous&&!row.target.ambiguous,'matched typed row appeared missing or ambiguous '+stage+' '+row.category);
      check(stage==='ratios'?close(row.delta,0):close(row.target.value,row.baseline.value*1.5)&&close(row.delta,row.baseline.value*.5),'wrong actual '+stage+' arithmetic');
      check(!row.basisMismatch&&!row.coverageMismatch&&row.directlyComparable,'identical actual source basis/coverage claimed incompatibility '+stage+' '+row.category);
    }
  }
  const ratioRows=result.rows.filter(row=>row.stage==='ratios');
  check(ratioRows.some(row=>close(row.baseline.value,4))&&ratioRows.some(row=>close(row.baseline.value,.8)),'actual COP/efficiency typed kinds were not preserved independently');
  const basisRows=compare(runs.baseline,runs.basis).rows.filter(row=>row.stage==='loads');
  check(basisRows.length===2&&basisRows.every(row=>row.basisMismatch&&!row.directlyComparable&&close(row.delta,0)),'reported vs service-path load basis split rows or lost comparison badge');
  const coverageRows=compare(runs.baseline,runs.coverage).rows.filter(row=>row.stage==='loads');
  check(coverageRows.length===2&&coverageRows.every(row=>row.coverageMismatch&&row.baseline.quality.status==='complete'&&row.target.quality.status==='partial'&&close(row.baseline.quality.percent,100)&&close(row.target.quality.percent,50)),'actual 130 stage-quality coverage difference was not propagated');
  const missingRows=compare(runs.baseline,runs.prunedZero).rows.filter(row=>row.stage==='endUses'&&close(row.baseline.value,10));
  check(missingRows.length===1&&missingRows[0].target.missing&&missingRows[0].target.value===null&&missingRows[0].delta===null&&missingRows[0].deltaPercent===null,'actual producer-pruned zero was silently invented as a measured zero');
  const zeroRows=compare(runs.baseline,runs.explicitZero).rows.filter(row=>row.stage==='endUses'&&close(row.baseline.value,10));
  check(zeroRows.length===1&&!zeroRows[0].target.missing&&zeroRows[0].target.value===0&&close(zeroRows[0].delta,-10)&&close(zeroRows[0].deltaPercent,-100),'explicit stored Go value:0 was lost or treated as missing');
  const zeroBaseline=compare(runs.explicitZero,runs.baseline).rows.find(row=>row.stage==='endUses'&&row.baseline.value===0);
  check(zeroBaseline?.delta===10&&zeroBaseline.deltaPercent===null,'zero baseline produced infinity or a invented percent');
  const noNumbers=clone(runs.baseline);noNumbers.purposeResults.energyExplanationSummary.loads[0].value=null;
  const nullRows=compare(runs.baseline,noNumbers).rows.filter(row=>row.stage==='loads'&&row.target.value===null);
  check(nullRows.length===1&&nullRows[0].delta===null,'malformed stored null load was coerced to0');
  for(const value of [false,true,'', '  ',undefined]){
    const invalid=clone(runs.baseline);invalid.purposeResults.energyExplanationSummary.loads[0].value=value;
    const rows=compare(runs.baseline,invalid).rows.filter(row=>row.stage==='loads'&&row.target.value===null);
    check(rows.length===1&&rows[0].delta===null&&rows[0].deltaPercent===null,'invalid stored optional value became numeric '+String(value));
  }
  const differentCounts=clone(runs.baseline);differentCounts.purposeResults.energyExplanationSummary.quality.loads.found=4;differentCounts.purposeResults.energyExplanationSummary.quality.loads.total=4;
  check(compare(runs.baseline,differentCounts).rows.filter(row=>row.stage==='loads').every(row=>row.coverageMismatch),'different requested-output counts with equal100% coverage lost warning');
  const units=clone(runs.baseline);units.purposeResults.energyExplanationSummary.loads[0].unit='MJ';
  const unitRows=compare(runs.baseline,units).rows.filter(row=>row.stage==='loads'&&row.target.unit==='MJ');
  check(unitRows.length===1&&unitRows[0].delta===null&&!unitRows[0].directlyComparable&&unitRows[0].warningCodes.includes('unit_or_domain_mismatch'),'different units were subtracted without conversion evidence');
  const duplicate=clone(runs.baseline);duplicate.purposeResults.energyExplanationSummary.loads.push({...duplicate.purposeResults.energyExplanationSummary.loads[0],id:'another.real.load',value:333});
  const duplicateRows=compare(runs.baseline,duplicate).rows.filter(row=>row.stage==='loads'&&row.target.ambiguous);
  check(duplicateRows.length===1&&duplicateRows[0].target.value===null&&duplicateRows[0].delta===null,'conflicting duplicate semantic categories were summed or selected first');
  const wrongLevel=clone(runs.baseline);wrongLevel.purposeResults.energyExplanationSummary.loads.find(item=>item.kind==='load.cooling').level='carrier';
  const levelRows=compare(runs.baseline,wrongLevel).rows.filter(row=>row.stage==='loads');
  check(!levelRows.some(row=>row.target.value===100)&&levelRows.some(row=>row.target.invalid),'known contradictory stage level was accepted as a thermal load');
  const wrongService=clone(runs.baseline);wrongService.purposeResults.energyExplanationSummary.loads.find(item=>item.kind==='load.cooling').serviceKind='heating';
  const serviceRows=compare(runs.baseline,wrongService).rows.filter(row=>row.stage==='loads');
  check(!serviceRows.some(row=>row.target.value===100)&&serviceRows.some(row=>row.target.invalid||row.target.ambiguous),'known cooling kind contradicted by heating service was arbitrarily classified');
  const wrongPeriod=clone(runs.baseline);wrongPeriod.purposeResults.energyExplanationSummary.period='M1';
  check(energyPathBatchSummary(wrongPeriod).status==='unavailable','monthly summary was silently compared as annual');
  const wrongScope=clone(runs.baseline);wrongScope.purposeResults.energyExplanationSummary.scope={kind:'zone',zoneName:'Office'};
  check(energyPathBatchSummary(wrongScope).status==='unavailable','Zone-only summary was silently compared as Building');
  check(JSON.stringify(runs)===original,'comparison mutated actual Go payload/source metadata');
  if(failures.length)throw Error(failures.join('\n'));
  document.body.dataset.epath170Summary='passed';document.querySelector('#result').textContent='PASS';
}catch(error){document.body.dataset.epath170Summary='failed';document.querySelector('#result').textContent=error.stack||String(error);}
</script></body></html>`
