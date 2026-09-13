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

func TestEnergyPathThermalBoundaryInspectorBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser thermal-boundary inspector contract in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-thermal-boundary", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathThermalBoundaryHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox",
		"--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000",
		"--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/energy-path-thermal-boundary")
	output, err := command.CombinedOutput()
	if err != nil || ctx.Err() != nil || !strings.Contains(string(output), `data-thermal-boundary-status="passed"`) {
		t.Fatalf("thermal-boundary inspector contract failed: %v / %v\n%s", err, ctx.Err(), output)
	}
}

// String-render assertions also run directly in Node: no engine, app API,
// fabricated numeric projection, or browser timing threshold is involved.
const energyPathThermalBoundaryHarnessHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Thermal measurement boundaries</title></head>
<body data-thermal-boundary-status="pending"><pre id="result">pending</pre><script type="module">
const check = (condition, message) => { if (!condition) throw new Error(message); };
const freeze = value => { if (value && typeof value === "object") { Object.values(value).forEach(freeze); Object.freeze(value); } return value; };
try {
  const view = await import("/src/js/views/energy-path-view.js");
  const { setLanguage, t } = await import("/src/js/i18n.js");
  const make = (boundary, period = "annual", scope = "building") => {
    const zoneName = scope === "zone" ? "Office" : "";
    const load = { id: "load.cooling", level: "load", label: "Cooling load", serviceKind: "cooling", driverCategory: "load.cooling", value: 80, rawValue: 80, effectiveValue: 80, unit: "kWh", scaleDomain: "thermal", period, zoneName, sourceIds: ["load-source"], ...(boundary === undefined ? {} : { thermalBoundary: boundary }) };
    const use = { id: "end_use.cooling", level: "end_use", label: "Cooling energy", endUse: "cooling", serviceKind: "cooling", value: 20, unit: "kWh", scaleDomain: "site", period, zoneName, sourceIds: ["site-source"] };
    const carrier = { id: "carrier.electricity", level: "carrier", label: "Electricity", carrier: "electricity", value: 20, unit: "kWh", scaleDomain: "site", period, zoneName };
    const conversion = { id: "conversion.cooling", fromId: load.id, toId: use.id, relation: "load_to_end_use", serviceKind: "cooling", fromValue: 80, toValue: 20, fromUnit: "kWh", toUnit: "kWh", ratio: 4, ratioKind: "coefficient_of_performance", period, zoneName, sourceIds: ["load-source", "site-source"] };
    const consumption = { id: "consumption.cooling", fromId: use.id, toId: carrier.id, relation: "end_use_to_carrier", serviceKind: "cooling", fromValue: 20, toValue: 20, fromUnit: "kWh", toUnit: "kWh", period, zoneName, sourceIds: ["site-source"] };
    return { load, use, carrier, conversion, consumption, state: { simulationEnergyScopeKind: scope, simulationEnergyZoneName: zoneName, simulationEnergyPeriod: period, simulationEnergyService: "all" } };
  };
  const render = (fixture, selected) => {
    const nodes = [fixture.load, fixture.use, fixture.carrier];
    const links = [fixture.conversion, fixture.consumption];
    const graph = { nodes, links, relations: [], supplyActivities: [], warnings: [] };
    const explanation = { schema: view.ENERGY_PATH_SCHEMA_V2, nodes, links, sources: [{ id: "load-source", name: "Zone Radiant HVAC Cooling Energy" }], quality: { ratios: { status: fixture.ratioStatus || "complete", found: 1, total: 1 } } };
    const state = { ...fixture.state, simulationEnergySelection: selected };
    const scene = view.prepareEnergyPathScene(explanation, state, { graph, allServiceGraph: graph });
    return view.renderEnergyPathInspector(scene, state);
  };
  const field = (html, attribute, key) => html.match(new RegExp(attribute + '="' + key + '"[^>]*><dt>[^<]*</dt><dd>([\\s\\S]*?)</dd>'))?.[1];
  let samples = 0;
  for (const language of ["en", "ko"]) {
    setLanguage(language);
    for (const boundary of ["active_surface_source", "mixed_thermal_boundaries"]) {
      for (const period of ["annual", "M1"]) for (const scope of ["building", "zone"]) {
        const fixture = freeze(make(boundary, period, scope));
        const original = JSON.stringify(fixture);
        const loadHTML = render(fixture, fixture.load.id);
        const linkHTML = render(fixture, fixture.conversion.id);
        for (const [kind, html] of [["load", loadHTML], ["conversion", linkHTML]]) {
          check(html.includes('data-energy-path-thermal-boundary="' + boundary + '"'), language + "/" + kind + "/" + scope + "/" + period + " missing exact boundary");
          check(html.includes(t(boundary === "active_surface_source" ? "simulation.energyPathBoundaryActiveSurface" : "simulation.energyPathBoundaryMixed")), kind + " lost localized measurement description");
          check(html.includes(t("simulation.energyPathBoundaryComparison")), kind + " lost storage/other-surfaces/COP warning");
          const words = language === "en"
            ? [boundary === "active_surface_source" ? "active radiant surface" : "air-system delivery", "not equipment COP or efficiency", "Surface heat storage", "other surfaces"]
            : [boundary === "active_surface_source" ? "활성 복사 표면" : "공기계통 전달열", "설비 COP나 효율이 아닙니다", "열 저장", "다른 표면"];
          check(words.every(word => html.includes(word)), kind + " lacks explicit translated physical-boundary caveats");
          check(!html.includes(t(kind === "load" ? "simulation.energyPathRepresentsLoad" : "simulation.energyPathRepresentsConversion")), kind + " retained contradictory unqualified delivery/conversion description");
          check([...html.matchAll(/data-energy-path-detail-section="([^"]+)"/g)].map(match => match[1]).join(",") === "represents,value,breakdown,basis,actions", kind + " changed common inspector sections");
        }
        check(field(loadHTML, "data-energy-path-inspector-value", "total") === "80 kWh thermal", "load number changed");
        check(field(linkHTML, "data-energy-path-link-value", "from") === "80 kWh thermal" && field(linkHTML, "data-energy-path-link-value", "to") === "20 kWh site", "conversion endpoint numbers changed");
        check(field(linkHTML, "data-energy-path-link-value", "ratio") === t("simulation.energyPathKPILoadSiteRatio") + ": 4", "radiant ratio still claimed equipment COP or changed number");
        check(JSON.stringify(fixture) === original, "inspector mutated canonical nodes, ratio kind, values, or sources");
        samples++;
      }
    }
    const legacy = make(undefined);
    const legacyLoad = render(legacy, legacy.load.id), legacyLink = render(legacy, legacy.conversion.id);
    check(!legacyLoad.includes("data-energy-path-thermal-boundary") && !legacyLink.includes("data-energy-path-thermal-boundary"), "source name was used to infer boundary");
    check(field(legacyLink, "data-energy-path-link-value", "ratio") === t("simulation.energyPathRatioCOP") + ": 4", "ordinary COP presentation changed");
    for (const unknown of ["", null, "unsupported_boundary"]) {
      const fixture = make(unknown);
      check(render(fixture, fixture.load.id) === legacyLoad && render(fixture, fixture.conversion.id) === legacyLink, "unset/unknown boundary changed existing inspector");
    }
    legacy.use.thermalBoundary = "active_surface_source";
    check(render(legacy, legacy.conversion.id) === legacyLink, "conversion inferred boundary from site endpoint");
    check(!render(legacy, legacy.use.id).includes("data-energy-path-thermal-boundary"), "non-load node acquired thermal-boundary claim");
    const unknownRatio = make("active_surface_source");
    delete unknownRatio.conversion.ratio;
    // The existing bridge can show a load/site comparison without a named
    // equipment ratio; unavailable quality, not a missing name, suppresses it.
    check(field(render(unknownRatio, unknownRatio.conversion.id), "data-energy-path-link-value", "ratio") === t("simulation.energyPathKPILoadSiteRatio") + ": 4", "boundary changed the existing unnamed load/site calculation");
    unknownRatio.ratioStatus = "unavailable";
    check(field(render(unknownRatio, unknownRatio.conversion.id), "data-energy-path-link-value", "ratio") === t("simulation.energyPathBridgeRatioUnavailable"), "boundary fabricated an unknown ratio");
    const missing = make("active_surface_source");
    delete missing.load.rawValue;
    missing.load.effectiveValue = null;
    const missingHTML = render(missing, missing.load.id);
    check(field(missingHTML, "data-energy-path-inspector-value", "raw") === t("common.notAvailable") && field(missingHTML, "data-energy-path-inspector-value", "effective") === t("common.notAvailable"), "boundary healed omitted/null source quantities");
  }
  document.body.dataset.thermalBoundaryStatus = "passed";
  document.getElementById("result").textContent = "passed " + samples + " localized scope/period boundary cases plus absence/unknown/legacy guards";
} catch (error) {
  document.body.dataset.thermalBoundaryStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
