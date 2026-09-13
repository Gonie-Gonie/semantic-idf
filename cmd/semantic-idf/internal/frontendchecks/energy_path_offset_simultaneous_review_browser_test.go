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

func TestEnergyPathOffsetSimultaneousPeriodScopeReviewBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser Energy Path offset/simultaneous period-scope review")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-offset-simultaneous-review", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathOffsetSimultaneousPeriodScopeReviewHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--no-sandbox",
		"--no-first-run",
		"--no-default-browser-check",
		"--virtual-time-budget=10000",
		"--user-data-dir="+t.TempDir(),
		"--dump-dom",
		server.URL+"/energy-path-offset-simultaneous-review",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("Energy Path offset/simultaneous period-scope browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("Energy Path offset/simultaneous period-scope browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-offset-simultaneous-review-status="passed"`) {
		t.Fatalf("Energy Path offset/simultaneous period-scope browser contract failed:\n%s", document)
	}
}

const energyPathOffsetSimultaneousPeriodScopeReviewHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Energy Path offset and simultaneous period/scope review</title></head>
<body data-energy-path-offset-simultaneous-review-status="pending">
<div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const metric = (numerator, denominator, ratio, sourceIds = []) => ({
  available: true, numerator, denominator, ratio, unit: "kWh",
  basis: "simultaneous_min_over_max", sourceIds,
});
const effect = (effectKind, targetService, driverCategory, label, value, sourceIds = []) => ({
  effectKind, targetService, driverCategory, label,
  heatDirection: targetService === "cooling" ? "loss" : "gain",
  rawValue: value, effectiveValue: value, unit: "kWh",
  basis: "signed_heat_balance_offset",
  explanation: "Opposite-sign heat-balance pressure is non-additive, non-causal context and is not an avoided-load quantity.",
  sourceIds,
});
const load = (scope, service, value, simultaneousLoad, offsetEffects = []) => ({
  id: "load." + service + "." + scope, level: "load", kind: "load.zone_" + service,
  label: service === "cooling" ? "Cooling load" : "Heating load",
  value, rawValue: value, effectiveValue: value, allocatedValue: value,
  unit: "kWh", scaleDomain: "thermal", serviceKind: service,
  driverCategory: "load." + service, thermalComponent: "sensible",
  basis: "reported_variable", simultaneousLoad, offsetEffects,
  sourceIds: [scope + "-" + service + "-load"],
});
const period = (id, nodes) => ({ id, kind: "monthly", nodes, links: [], warnings: [] });
const coolingOffset = (label, value, source) => effect("reduces_cooling", "cooling", "air.infiltration", label, value, [source]);
const heatingOffset = (label, value, source) => effect("reduces_heating", "heating", "internal.people", label, value, [source]);

try {
  const module = await import("/src/js/views/energy-path-view.js");
  const buildingAnnualMetric = metric(20, 260, 0.076923, ["office-cooling-load", "office-heating-load", "lab-cooling-load", "lab-heating-load"]);
  const buildingM1Metric = metric(20, 200, 0.1);
  const buildingM2Metric = metric(0, 60, 0);
  const officeAnnualMetric = metric(10, 140, 0.071429);
  const officeM1Metric = metric(10, 100, 0.1);
  const officeM2Metric = metric(0, 40, 0);
  const labAnnualMetric = metric(10, 120, 0.083333);
  const labM1Metric = metric(10, 100, 0.1);
  const labM2Metric = metric(0, 20, 0);

  const buildingAnnualCooling = load("building", "cooling", 150, buildingAnnualMetric, [coolingOffset("Building infiltration", 74, "building-infiltration")]);
  const buildingAnnualHeating = load("building", "heating", 130, buildingAnnualMetric, [heatingOffset("Building people", 41, "building-people")]);
  const buildingM1Cooling = load("building", "cooling", 110, buildingM1Metric, [coolingOffset("M1 infiltration", 70, "m1-infiltration")]);
  const buildingM1Heating = load("building", "heating", 110, buildingM1Metric, [heatingOffset("M1 people", 35, "m1-people")]);
  const buildingM2Cooling = load("building", "cooling", 40, buildingM2Metric, [coolingOffset("Office infiltration", 4, "office-infiltration")]);
  const buildingM2Heating = load("building", "heating", 20, buildingM2Metric, [heatingOffset("Lab people", 6, "lab-people")]);

  const officeAnnualCooling = load("office", "cooling", 140, officeAnnualMetric, [coolingOffset("Office infiltration", 24, "office-infiltration")]);
  officeAnnualCooling.zoneName = "Office";
  const officeAnnualHeating = load("office", "heating", 10, officeAnnualMetric, [heatingOffset("Office people", 30, "office-people")]);
  officeAnnualHeating.zoneName = "Office";
  const officeM1Cooling = load("office", "cooling", 100, officeM1Metric, [coolingOffset("Office infiltration", 20, "office-infiltration")]);
  officeM1Cooling.zoneName = "Office";
  const officeM1Heating = load("office", "heating", 10, officeM1Metric, [heatingOffset("Office people", 30, "office-people")]);
  officeM1Heating.zoneName = "Office";
  const officeM2Cooling = load("office", "cooling", 40, officeM2Metric, [coolingOffset("Office infiltration", 4, "office-infiltration")]);
  officeM2Cooling.zoneName = "Office";

  const labAnnualCooling = load("lab", "cooling", 10, labAnnualMetric, [coolingOffset("Lab infiltration", 50, "lab-infiltration")]);
  labAnnualCooling.zoneName = "Lab";
  const labAnnualHeating = load("lab", "heating", 120, labAnnualMetric, [heatingOffset("Lab people", 11, "lab-people")]);
  labAnnualHeating.zoneName = "Lab";
  const labM1Cooling = load("lab", "cooling", 10, labM1Metric, [coolingOffset("Lab infiltration", 50, "lab-infiltration")]);
  labM1Cooling.zoneName = "Lab";
  const labM1Heating = load("lab", "heating", 100, labM1Metric, [heatingOffset("Lab people", 5, "lab-people")]);
  labM1Heating.zoneName = "Lab";
  const labM2Heating = load("lab", "heating", 20, labM2Metric, [heatingOffset("Lab people", 6, "lab-people")]);
  labM2Heating.zoneName = "Lab";

  const explanation = {
    schema: module.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    availableZones: ["Office", "Lab"],
    nodes: [buildingAnnualCooling, buildingAnnualHeating], links: [],
    periods: [
      period("M1", [buildingM1Cooling, buildingM1Heating]),
      period("M2", [buildingM2Cooling, buildingM2Heating]),
    ],
    zoneResults: [
      {
        scope: { kind: "zone", zoneName: "Office", aggregationBasis: "model_total" },
        nodes: [officeAnnualCooling, officeAnnualHeating], links: [],
        periods: [period("M1", [officeM1Cooling, officeM1Heating]), period("M2", [officeM2Cooling])],
      },
      {
        scope: { kind: "zone", zoneName: "Lab", aggregationBasis: "model_total" },
        nodes: [labAnnualCooling, labAnnualHeating], links: [],
        periods: [period("M1", [labM1Cooling, labM1Heating]), period("M2", [labM2Heating])],
      },
    ],
    sources: [],
  };

  const mount = document.getElementById("mount");
  const render = (state) => {
    mount.innerHTML = module.renderEnergyPathView(explanation, state);
    return mount;
  };
  const inspector = (root, id) => root.querySelector('[data-energy-path-inspector="' + id + '"]');
  const ratio = (root, id) => inspector(root, id)?.querySelector('[data-energy-path-simultaneous-load-ratio="simultaneous_min_over_max"]');
  const offset = (root, id) => inspector(root, id)?.querySelector('[data-energy-path-offset-effects]');
  const assertMetric = (element, numerator, denominator, value, label) => {
    assert(element, label + " ratio section missing");
    assert(Number(element.dataset.energyPathSimultaneousLoadNumerator) === numerator, label + " numerator mismatch");
    assert(Number(element.dataset.energyPathSimultaneousLoadDenominator) === denominator, label + " denominator mismatch");
    assert(Number(element.dataset.energyPathSimultaneousLoadValue) === value, label + " ratio mismatch");
    assert(element.textContent.includes("zone-month"), label + " does not explain zone-month basis");
  };

  let state = {
    simulationEnergyScopeKind: "building", simulationEnergyPeriod: "annual",
    simulationEnergyService: "cooling", simulationEnergySelection: "load.cooling.building",
  };
  let root = render(state);
  assertMetric(ratio(root, "load.cooling.building"), 20, 260, 0.076923, "Building annual cooling");
  let section = offset(root, "load.cooling.building");
  assert(section?.querySelector('[data-energy-path-offset-target="cooling"][data-energy-path-offset-category="air.infiltration"]'), "Building annual cooling offset target/category missing");
  assert(section.textContent.includes("74") && section.textContent.includes("Building infiltration"), "Building annual cooling offset value missing");
  assert(!section.textContent.includes("signed_heat_balance_offset") && section.textContent.toLowerCase().includes("non-additive") && section.textContent.toLowerCase().includes("non-causal"), "offset non-additive/non-causal explanation is missing or exposes a technical token");
  assert(!inspector(root, "load.cooling.building").querySelector('[data-energy-path-detail-section="sources"], [data-energy-path-detail-section="entities"]'), "removed Source data or Related model entities section is still rendered");
  assert(module.energyPathGraphForState(explanation, state).nodes.find((node) => node.id === "load.cooling.building")?.offsetEffects[0]?.basis === "signed_heat_balance_offset", "load graph lost the underlying offset basis");

  root = render({ ...state, simulationEnergyPeriod: "M1" });
  assertMetric(ratio(root, "load.cooling.building"), 20, 200, 0.1, "Building M1 cooling");
  section = offset(root, "load.cooling.building");
  assert(section.textContent.includes("70") && !section.textContent.includes("74"), "Building M1 offset leaked annual value");

  root = render({ ...state, simulationEnergyPeriod: "M2" });
  assertMetric(ratio(root, "load.cooling.building"), 0, 60, 0, "Building M2 cooling");
  section = offset(root, "load.cooling.building");
  assert(section.textContent.includes("Office infiltration") && section.textContent.includes("4"), "Building M2 cooling did not retain Office-local offset");
  assert(!section.textContent.includes("Lab infiltration") && !section.textContent.includes("74"), "Building M2 cooling leaked another zone/period offset");

  root = render({ ...state, simulationEnergyPeriod: "M2", simulationEnergyService: "heating", simulationEnergySelection: "load.heating.building" });
  assertMetric(ratio(root, "load.heating.building"), 0, 60, 0, "Building M2 heating");
  section = offset(root, "load.heating.building");
  assert(section.textContent.includes("Lab people") && section.textContent.includes("6"), "Building M2 heating did not retain Lab-local offset");
  assert(!section.textContent.includes("Office people") && !section.textContent.includes("41"), "Building M2 heating leaked another zone/period offset");

  state = {
    simulationEnergyScopeKind: "zone", simulationEnergyZoneName: "Office", simulationEnergyPeriod: "annual",
    simulationEnergyService: "cooling", simulationEnergySelection: "load.cooling.office",
  };
  root = render(state);
  assertMetric(ratio(root, "load.cooling.office"), 10, 140, 0.071429, "Office annual cooling");
  section = offset(root, "load.cooling.office");
  assert(section.textContent.includes("Office infiltration") && section.textContent.includes("24"), "Office annual offset missing");
  assert(!section.textContent.includes("Lab"), "Lab offset leaked into Office inspector");

  root = render({ ...state, simulationEnergyPeriod: "M2" });
  assertMetric(ratio(root, "load.cooling.office"), 0, 40, 0, "Office M2 cooling");
  section = offset(root, "load.cooling.office");
  assert(section.textContent.includes("4") && !section.textContent.includes("24"), "Office M2 offset leaked annual value");

  state = {
    simulationEnergyScopeKind: "zone", simulationEnergyZoneName: "Lab", simulationEnergyPeriod: "annual",
    simulationEnergyService: "heating", simulationEnergySelection: "load.heating.lab",
  };
  root = render(state);
  assertMetric(ratio(root, "load.heating.lab"), 10, 120, 0.083333, "Lab annual heating");
  section = offset(root, "load.heating.lab");
  assert(section.textContent.includes("Lab people") && section.textContent.includes("11"), "Lab annual heating offset missing");
  assert(!section.textContent.includes("Office"), "Office offset leaked into Lab inspector");

  document.body.dataset.energyPathOffsetSimultaneousReviewStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathOffsetSimultaneousReviewStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
