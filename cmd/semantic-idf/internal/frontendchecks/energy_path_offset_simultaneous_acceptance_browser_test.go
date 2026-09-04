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

func TestEPATH081AcceptanceOffsetEffectsBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser EPATH-081 acceptance harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-offset-simultaneous-acceptance", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathOffsetSimultaneousAcceptanceHarnessHTML)
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
		server.URL+"/energy-path-offset-simultaneous-acceptance",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("EPATH-081 browser acceptance harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("EPATH-081 browser acceptance harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-offset-simultaneous-acceptance-status="passed"`) {
		t.Fatalf("EPATH-081 browser contract failed:\n%s", document)
	}
}

const energyPathOffsetSimultaneousAcceptanceHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>EPATH-081 offset and simultaneous-load acceptance</title></head>
<body data-energy-path-offset-simultaneous-acceptance-status="pending">
<div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const node = (id, level, label, value, serviceKind, driverCategory, extra = {}) => ({
  id, level, kind: level + "." + driverCategory, label, value,
  rawValue: value, effectiveValue: value, allocatedValue: value,
  unit: "kWh", scaleDomain: "thermal", period: "annual", serviceKind,
  driverCategory, aggregationBasis: "model_total", sourceIds: [id + ".source"],
  ...extra,
});
const effect = (effectKind, targetService, driverCategory, label, heatDirection, value, sourceId) => ({
  effectKind, targetService, driverCategory, label, heatDirection,
  rawValue: value, effectiveValue: value, unit: "kWh",
  basis: "signed_heat_balance_offset",
  explanation: "Opposite-sign heat-balance context only; never a reverse main ribbon.",
  sourceIds: [sourceId],
});
const simultaneous = {
  available: true,
  numerator: 30,
  denominator: 170,
  ratio: 0.176471,
  unit: "kWh",
  basis: "simultaneous_min_over_max",
  sourceIds: ["east-cooling", "east-heating", "west-cooling", "west-heating"],
};
const cooling = node("load.cooling.building", "load", "Cooling load", 110, "cooling", "load.cooling", {
  offsetEffects: [
    effect("reduces_cooling", "cooling", "air.mechanical_ventilation", "Mechanical ventilation", "loss", 10, "east-ventilation-loss"),
    effect("reduces_cooling", "cooling", "surface.exterior_walls", "Exterior walls", "loss", 80, "west-wall-loss"),
  ],
  simultaneousLoad: simultaneous,
});
const heating = node("load.heating.building", "load", "Heating load", 90, "heating", "load.heating", {
  offsetEffects: [
    effect("reduces_heating", "heating", "air.infiltration", "Infiltration", "gain", 90, "east-infiltration-gain"),
    effect("reduces_heating", "heating", "internal.people", "People", "gain", 20, "west-people-gain"),
  ],
  simultaneousLoad: simultaneous,
});
const drivers = [
  node("driver.air.infiltration.cooling.building", "driver", "Infiltration", 90, "cooling", "air.infiltration", { signedValue: 90, sign: "positive", thermalDirection: "gain", allocationApplied: true }),
  node("driver.internal.people.cooling.building", "driver", "People", 20, "cooling", "internal.people", { signedValue: 20, sign: "positive", thermalDirection: "gain", allocationApplied: true }),
  node("driver.air.mechanical_ventilation.heating.building", "driver", "Mechanical ventilation", 10, "heating", "air.mechanical_ventilation", { signedValue: -10, sign: "negative", thermalDirection: "loss", allocationApplied: true }),
  node("driver.surface.exterior_walls.heating.building", "driver", "Exterior walls", 80, "heating", "surface.exterior_walls", { signedValue: -80, sign: "negative", thermalDirection: "loss", allocationApplied: true }),
];
const link = (fromId, toId, value, serviceKind) => ({
  id: "link." + fromId + "." + toId,
  fromId, toId, relation: "driver_to_load", basis: "heat_balance_share",
  fromValue: value, toValue: value, value, period: "annual", serviceKind,
});
const links = [
  link(drivers[0].id, cooling.id, 90, "cooling"),
  link(drivers[1].id, cooling.id, 20, "cooling"),
  link(drivers[2].id, heating.id, 10, "heating"),
  link(drivers[3].id, heating.id, 80, "heating"),
];
const explanation = {
  schema: "semantic-idf.energy-explanation/v2",
  scope: { kind: "building", aggregationBasis: "model_total" },
  nodes: [...drivers, cooling, heating], links, periods: [], zoneResults: [], sources: [], warnings: [],
};

try {
  const module = await import("/src/js/views/energy-path-view.js");
  const state = {
    simulationEnergyScopeKind: "building",
    simulationEnergyPeriod: "annual",
    simulationEnergyService: "all",
    simulationEnergySelection: cooling.id,
  };
  const graph = module.energyPathGraphForState(explanation, { ...state });
  const graphNodes = new Map(graph.nodes.map((item) => [item.id, item]));
  const mainLinks = graph.links.filter((item) => item.relation === "driver_to_load");
  assert(mainLinks.length === 4, "offset context added or removed a main ribbon: " + JSON.stringify(mainLinks));
  for (const item of mainLinks) {
    const from = graphNodes.get(item.fromId);
    const to = graphNodes.get(item.toId);
    assert(from?.level === "driver" && to?.level === "load", "main ribbon direction is not driver to load");
    assert(item.fromValue >= 0 && item.toValue >= 0, "main ribbon width became negative/reverse");
    assert(item.serviceKind === to.serviceKind, "main ribbon crossed into the opposite load service");
    assert(!String(item.id + item.fromId + item.toId + item.basis).toLowerCase().includes("offset"), "offset effect became a main ribbon");
  }
  assert(!mainLinks.some((item) => item.fromId === drivers[0].id && item.toId === heating.id), "positive cooling pressure gained a reverse heating ribbon");
  assert(!mainLinks.some((item) => item.fromId === drivers[2].id && item.toId === cooling.id), "negative heating pressure gained a reverse cooling ribbon");

  const mount = document.getElementById("mount");
  const render = (selection) => {
    mount.innerHTML = module.renderEnergyPathView(explanation, { ...state, simulationEnergySelection: selection });
    return mount;
  };
  const loadButtons = (root) => [...root.querySelectorAll('[data-energy-path-stage="load"] [data-energy-explanation-node]')];
  const inspect = (root, id) => root.querySelector('[data-energy-path-inspector="' + id + '"]');
  const checkRatio = (inspector) => {
    const ratio = inspector?.querySelector('[data-energy-path-simultaneous-load-ratio="simultaneous_min_over_max"]');
    assert(ratio, "load inspector has no simultaneous heating/cooling ratio contract");
    assert(ratio.dataset.energyPathSimultaneousLoadNumerator === "30", "simultaneous numerator is not sum(zone-month min)=30");
    assert(ratio.dataset.energyPathSimultaneousLoadDenominator === "170", "simultaneous denominator is not sum(zone-month max)=170");
    assert(Math.abs(Number(ratio.dataset.energyPathSimultaneousLoadValue) - 30 / 170) < 0.000001, "simultaneous ratio is not 30/170");
    assert(ratio.textContent.includes("17.6%"), "simultaneous ratio is not legible as 17.6%");
    assert(ratio.textContent.includes("Sum of zone-month minimum"), "ratio inspector does not explain zone-month-first aggregation");
  };

  let root = render(cooling.id);
  let loads = loadButtons(root);
  assert(loads.length === 2, "simultaneous operation must keep exactly cooling and heating primary load nodes");
  assert(loads.some((item) => item.dataset.energyExplanationNode === cooling.id), "cooling load disappeared during simultaneous operation");
  assert(loads.some((item) => item.dataset.energyExplanationNode === heating.id), "heating load disappeared during simultaneous operation");
  assert(!root.querySelector('[data-energy-explanation-node*="offset"]'), "Offset effects leaked into a primary node");
  let inspector = inspect(root, cooling.id);
  let offsets = inspector?.querySelector('[data-energy-path-offset-effects]');
  assert(offsets, "cooling load inspector has no stable Offset effects section");
  assert(offsets.querySelector("header strong")?.textContent.trim() === "Offset effects", "Offset effects section heading changed");
  assert(offsets.textContent.includes("never create reverse main ribbons"), "Offset effects section omits the no-reverse-ribbon explanation");
  assert(offsets.querySelector('[data-energy-path-offset-effect="reduces_cooling"][data-energy-path-offset-target="cooling"][data-energy-path-offset-category="air.mechanical_ventilation"]'), "cooling offset lost East ventilation loss");
  assert(offsets.querySelector('[data-energy-path-offset-effect="reduces_cooling"][data-energy-path-offset-target="cooling"][data-energy-path-offset-category="surface.exterior_walls"]'), "cooling offset lost West wall loss instead of aggregating zone-month contributions");
  assert(offsets.textContent.includes("10 kWh") && offsets.textContent.includes("80 kWh"), "cooling offset magnitudes are not inspectable");
  checkRatio(inspector);

  root = render(heating.id);
  loads = loadButtons(root);
  assert(loads.length === 2, "selecting heating hid one of the simultaneous load nodes");
  inspector = inspect(root, heating.id);
  offsets = inspector?.querySelector('[data-energy-path-offset-effects]');
  assert(offsets, "heating load inspector has no stable Offset effects section");
  assert(offsets.querySelector('[data-energy-path-offset-effect="reduces_heating"][data-energy-path-offset-target="heating"][data-energy-path-offset-category="air.infiltration"]'), "heating offset lost East infiltration gain");
  assert(offsets.querySelector('[data-energy-path-offset-effect="reduces_heating"][data-energy-path-offset-target="heating"][data-energy-path-offset-category="internal.people"]'), "heating offset lost West people gain instead of aggregating zone-month contributions");
  assert(offsets.textContent.includes("90 kWh") && offsets.textContent.includes("20 kWh"), "heating offset magnitudes are not inspectable");
  checkRatio(inspector);

  document.body.dataset.energyPathOffsetSimultaneousAcceptanceStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathOffsetSimultaneousAcceptanceStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
