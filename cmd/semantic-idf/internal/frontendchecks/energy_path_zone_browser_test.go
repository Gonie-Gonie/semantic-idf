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

func TestEnergyPathPrecomputedZoneResultBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser Energy Path zone-result harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-zone", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathZoneHarnessHTML)
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
		server.URL+"/energy-path-zone",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("Energy Path zone-result browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("Energy Path zone-result browser harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-energy-path-zone-status="passed"`) {
		t.Fatalf("Energy Path precomputed zone-result contract failed in browser:\n%s", document)
	}
}

const energyPathZoneHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Energy Path zone-result harness</title></head>
<body data-energy-path-zone-status="pending">
<div id="runtimeStatus"></div>
<pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => {
  if (!condition) throw new Error(message);
};
const pathGraph = (prefix, zoneName, period) => {
  const nodes = [];
  const links = [];
  for (const service of ["cooling", "heating"]) {
    const ids = ["driver", "load", "end_use", "carrier"].map((level) => prefix + "." + service + "." + level);
    ["driver", "load", "end_use", "carrier"].forEach((level, index) => {
      const node = {
        id: ids[index],
        level,
        label: prefix + " " + service + " " + level,
        value: index + 1,
        unit: level === "driver" || level === "load" ? "kWh thermal" : "kWh",
        scaleDomain: level === "driver" || level === "load" ? "thermal" : "site",
        serviceKind: service,
        zoneName,
        period,
      };
      if (level === "carrier") {
        node.kind = "carrier.electricity";
        node.carrier = "electricity";
      }
      nodes.push(node);
    });
    for (let index = 0; index < ids.length - 1; index += 1) {
      links.push({
        id: prefix + "." + service + ".link." + index,
        fromId: ids[index],
        toId: ids[index + 1],
        serviceKind: service,
        zoneName,
        period,
      });
    }
  }
  return { nodes, links };
};
const pathSummary = (scope, period, value) => ({
  schema: "semantic-idf.energy-explanation-summary/v2",
  period,
  scope,
  drivers: [{ id: "driver." + period, label: "Driver " + period, value, unit: "kWh thermal" }],
  loads: [{ id: "load.cooling." + period, label: "Cooling " + period, value, serviceKind: "cooling", unit: "kWh thermal" }],
  endUses: [{ id: "end_use.cooling." + period, label: "Cooling energy " + period, value, serviceKind: "cooling", unit: "kWh site" }],
  carriers: [{ id: "carrier.electricity." + period, label: "Electricity " + period, value, unit: "kWh site" }],
  ratios: [],
  residuals: [],
  topZones: [],
  completeness: { mappedPercent: value, status: "complete" },
});
const zoneResult = (name, token) => {
  const annual = pathGraph(token + ".annual", name, "annual");
  const january = pathGraph(token + ".m1", name, "M1");
  if (token === "beta") {
    Object.assign(january.nodes.find((node) => node.level === "load" && node.serviceKind === "heating"), {
      rawValue: 1,
      effectiveValue: 10,
      allocatedValue: 2,
      sourceIds: ["beta-heating-load", "beta-heating-context", "beta-heating-fallback"],
    });
  }
  const scope = { kind: "zone", zoneName: name, aggregationBasis: "model_total" };
  return {
    scope,
    summary: pathSummary(scope, "annual", token === "alpha" ? 10 : 20),
    completeness: { mappedPercent: token === "alpha" ? 10 : 20, status: "complete" },
    nodes: annual.nodes,
    links: annual.links,
    periods: [{
      id: "M1",
      kind: "monthly",
      summary: pathSummary(scope, "M1", token === "alpha" ? 1 : 2),
      nodes: january.nodes,
      links: january.links,
      warnings: [{ severity: "warning", code: token + "-monthly-balance", message: token + " monthly balance warning" }],
    }],
    reconciliation: [],
    warnings: [{ severity: "warning", code: token + "-annual-balance", message: token + " annual balance warning" }],
  };
};
try {
  const module = await import("/src/js/views/energy-path-view.js");
  const mergedDrivers = module.energyPathMergeAllServiceDrivers([
    { id: "driver.air.infiltration.cooling.building", level: "driver", driverCategory: "air.infiltration", serviceKind: "cooling", value: 10, rawValue: 1, effectiveValue: 10, allocatedValue: 5 },
    { id: "driver.air.infiltration.heating.building", level: "driver", driverCategory: "air.infiltration", serviceKind: "heating", value: 20, rawValue: 2, effectiveValue: 20, allocatedValue: 10 },
  ], []);
  assert(mergedDrivers.nodes.length === 1 && mergedDrivers.nodes[0].rawValue === 3 && mergedDrivers.nodes[0].effectiveValue === 30 && mergedDrivers.nodes[0].allocatedValue === 15 && mergedDrivers.nodes[0].multiplier === 10, "All-service projection lost raw/effective/allocated multiplier accounting");
  let backendCalls = 0;
  window.fetch = () => {
    backendCalls += 1;
    throw new Error("Energy Path controls must not call the backend");
  };

  const building = pathGraph("building.annual", "", "annual");
  const explanation = {
    schema: "semantic-idf.energy-explanation/v2",
    scope: { kind: "building", aggregationBasis: "model_total" },
    availableZones: ["Beta Zone", "Alpha Zone"],
    zoneResults: [zoneResult("Alpha Zone", "alpha"), zoneResult("Beta Zone", "beta")],
    sources: [{
      id: "beta-heating-load",
      name: "Zone People Convective Heating Energy",
      rawValue: 40,
      effectiveValue: 40,
      effectiveMultiplier: 1,
      multiplierApplication: "already_model_total",
      driverComponent: "internal.people.sensible.gain",
      heatDirection: "gain",
      inspectorSection: "Breakdown",
      relatedEntityIds: ["people.office"],
      scopeDetails: [{
        scope: { kind: "zone", zoneName: "Beta Zone", aggregationBasis: "model_total" },
        rawValue: 1,
        effectiveValue: 10,
        effectiveMultiplier: 10,
        multiplierApplication: "requires_zone_multiplier",
        allocationFactor: 0.2,
        allocatedValue: 2,
      }],
    }, {
      id: "beta-heating-context",
      name: "Zone Ideal Loads Outdoor Air Total Heating Energy",
      driverComponent: "mechanical_ventilation.ideal_loads_context.combined.loss",
      heatDirection: "loss",
      inspectorSection: "Context",
      relatedEntityIds: ["ideal-loads.office"],
    }, {
      id: "beta-heating-fallback",
      name: "Mechanical ventilation fallback",
      driverComponent: "mechanical_ventilation.fallback_residual.sensible",
      heatDirection: "loss",
      inspectorSection: "Balance",
      formula: "signed outdoor-air aggregate - signed infiltration",
      inputSourceIds: ["outdoor-air", "infiltration"],
      relatedEntityIds: ["zone.office", "air:mix", "air:missing"],
    }, {
      id: "beta-driver-context",
      name: "Zone Ideal Loads Heat Recovery Sensible Heating Energy",
      zoneName: "Beta Zone",
      driverCategory: "air.mechanical_ventilation",
      driverComponent: "mechanical_ventilation.heat_recovery_context.sensible.loss",
      heatDirection: "loss",
      inspectorSection: "Context",
      relatedEntityIds: ["ideal-loads.beta"],
    }],
    nodes: building.nodes,
    links: building.links,
    periods: [],
  };
  const state = {
    simulationEnergyScopeKind: "building",
    simulationEnergyZoneName: "",
    simulationEnergyPeriod: "annual",
    simulationEnergyService: "all",
    simulationEnergySelection: "",
    report: { geometry: { topology: { airCouplings: [{ id: "air:mix" }] } } },
  };
  const buildingSummary = pathSummary(explanation.scope, "annual", 100);

  assert(JSON.stringify(module.energyPathZoneNames(explanation)) === JSON.stringify(["Alpha Zone", "Beta Zone"]), "Building payload did not expose both precomputed zones");
  assert(module.energyPathGraphForState(explanation, state).nodes.every((node) => node.id.startsWith("building.annual")), "Building default did not use the top-level graph");
  assert(module.energyPathSummaryForState(explanation, buildingSummary, state).carriers[0].value === 100, "Building graph did not use the Building annual summary");

  const scope = document.createElement("select");
  scope.setAttribute("data-simulation-energy-scope", "");
  scope.innerHTML = '<option value="building">Building</option><option value="zone">Zone</option>';
  scope.value = "zone";
  const scopeUpdate = module.updateEnergyPathControlState({ type: "change", target: scope }, state, explanation);
  assert(scopeUpdate.handled && scopeUpdate.render, "Zone scope change was not handled locally");
  assert(state.simulationEnergyZoneName === "Alpha Zone", "Zone scope did not select the first available zone result");
  let graph = module.energyPathGraphForState(explanation, state);
  assert(graph.nodes.length > 0 && graph.nodes.every((node) => node.id.startsWith("alpha.annual")), "Zone scope filtered the Building graph instead of selecting Alpha zoneResult");
  assert(module.energyPathSummaryForState(explanation, buildingSummary, state).carriers[0].value === 10, "Alpha zone graph retained the Building KPI summary");

  const zone = document.createElement("input");
  zone.setAttribute("data-simulation-energy-zone-name", "");
  zone.value = "Beta Zone";
  const zoneUpdate = module.updateEnergyPathControlState({ type: "change", target: zone }, state, explanation);
  assert(zoneUpdate.handled && zoneUpdate.render, "Zone selection was not handled locally");
  graph = module.energyPathGraphForState(explanation, state);
  assert(graph.nodes.length > 0 && graph.nodes.every((node) => node.id.startsWith("beta.annual")), "Zone selection leaked another scope graph");
  assert(module.energyPathSummaryForState(explanation, buildingSummary, state).carriers[0].value === 20, "Beta zone graph retained another scope KPI summary");

  const period = document.createElement("select");
  period.setAttribute("data-simulation-energy-path-period", "");
  period.innerHTML = '<option value="annual">Annual</option><option value="M1">M1</option>';
  period.value = "M1";
  const periodUpdate = module.updateEnergyPathControlState({ type: "change", target: period }, state, explanation);
  assert(periodUpdate.handled && periodUpdate.render, "M1 selection was not handled locally");
  graph = module.energyPathGraphForState(explanation, state);
  assert(graph.nodes.length > 0 && graph.nodes.every((node) => node.id.startsWith("beta.m1")), "M1 did not use the selected zoneResult period graph");
  assert(graph.warnings.length === 1 && graph.warnings[0].code === "beta-monthly-balance", "M1 did not select its own quality warnings");
  assert(!graph.nodes.some((node) => node.id.includes("annual") || node.id.startsWith("alpha") || node.id.startsWith("building")), "M1 leaked annual or another scope graph");
  const januarySummary = module.energyPathSummaryForState(explanation, buildingSummary, state);
  assert(januarySummary.carriers[0].value === 2 && januarySummary.period === "M1" && januarySummary.scope.zoneName === "Beta Zone", "M1 graph did not select the matching Zone/period KPI summary");

  const service = document.createElement("select");
  service.setAttribute("data-simulation-energy-service", "");
  service.innerHTML = '<option value="all">All</option><option value="cooling">Cooling</option><option value="heating">Heating</option>';
  service.value = "heating";
  const serviceUpdate = module.updateEnergyPathControlState({ type: "change", target: service }, state, explanation);
  assert(serviceUpdate.handled && serviceUpdate.render, "Service selection was not handled locally");
  graph = module.energyPathGraphForState(explanation, state);
  assert(graph.nodes.length === 4 && graph.nodes.every((node) => node.id.includes(".heating.")), "Service filtering did not apply to the selected zone and M1 graph");
  state.simulationEnergySelection = graph.nodes.find((node) => node.level === "load").id;
  const rendered = document.createElement("div");
  rendered.innerHTML = module.renderEnergyPathView(explanation, state);
  const inspector = rendered.querySelector("[data-energy-path-inspector]");
  assert(inspector, "Selected Zone node did not render an inspector");
  assert(inspector.querySelector('[data-energy-path-inspector-value="raw"] dd').textContent.includes("1"), "Inspector did not show the raw reported value");
  assert(inspector.querySelector('[data-energy-path-inspector-value="multiplier"] dd').textContent === "10", "Inspector did not use the selected Zone multiplier detail");
  assert(inspector.querySelector('[data-energy-path-inspector-value="effective"] dd').textContent.includes("10"), "Inspector did not show the effective contribution");
  assert(inspector.querySelector('[data-energy-path-inspector-value="allocated"] dd').textContent.includes("2"), "Inspector did not keep allocation separate from effective contribution");
  assert(inspector.querySelector('[data-energy-path-inspector-value="application"] dd').textContent === "Zone multiplier required", "Inspector did not explain the multiplier application in plain language");
  assert(!inspector.querySelector('[data-energy-path-detail-section="sources"], [data-energy-path-detail-section="entities"], [data-energy-path-source]'), "Zone inspector still renders removed Source data or Related model entities");
  const selectedSources = module.energyPathInspectorSources(explanation, graph.nodes.find((node) => node.id === state.simulationEnergySelection), state);
  assert(selectedSources.find((source) => source.id === "beta-heating-load")?.multiplierApplication === "requires_zone_multiplier", "selected Zone calculation source lost its exact multiplier application");
  const sourceDetails = document.createElement("div");
  sourceDetails.innerHTML = module.renderEnergyPathSourceDetails(selectedSources, state);
  assert(sourceDetails.querySelector('[data-energy-path-inspector-section="breakdown"]'), "Standalone source details did not group additive sources as Breakdown");
  assert(sourceDetails.querySelector('[data-energy-path-inspector-section="context"]'), "Standalone source details did not retain context-only sources");
  assert(sourceDetails.querySelector('[data-energy-path-inspector-section="balance"]'), "Standalone source details did not group reconciliation sources as Balance");
  assert(sourceDetails.querySelector('[data-energy-path-source="beta-heating-load"] [data-energy-path-source-field="driverComponent"] dd').textContent === "internal.people.sensible.gain", "Standalone source details lost the sensible/latent component dimension");
  assert(sourceDetails.querySelector('[data-energy-path-source="beta-heating-load"] [data-energy-path-source-field="heatDirection"] dd').textContent === "gain", "Standalone source details lost the gain/loss direction");
  const fallbackSource = sourceDetails.querySelector('[data-energy-path-source="beta-heating-fallback"]');
  assert(fallbackSource?.dataset.energyPathSourceStatus === "fallback", "Standalone source details did not flag a fallback derivation");
  assert(fallbackSource.querySelector('[data-energy-path-source-field="formula"] dd').textContent.includes("outdoor-air aggregate"), "Standalone source details lost the fallback formula");
  assert(fallbackSource.querySelector('[data-energy-path-source-field="inputSourceIds"] dd').textContent.includes("outdoor-air") && fallbackSource.querySelector('[data-energy-path-source-field="inputSourceIds"] dd').textContent.includes("infiltration"), "Standalone source details lost fallback input source IDs");
  assert(fallbackSource.querySelector('[data-energy-path-source-field="relatedEntityIds"] dd').textContent.includes("zone.office"), "Standalone source details lost related topology entities");
  const airCouplingAction = fallbackSource.querySelector('[data-energy-path-topology-air-coupling-id="air:mix"]');
  assert(airCouplingAction?.dataset.entityKind === "thermal_air_coupling" && airCouplingAction.dataset.panelTargetId === "air:mix", "Standalone source details lost the valid Topology Air coupling action");
  assert(!fallbackSource.querySelector('[data-energy-path-topology-air-coupling-id="air:missing"]'), "Standalone source details made an unavailable related entity actionable");
  const warningPanel = rendered.querySelector("[data-energy-path-warnings]");
  assert(warningPanel?.textContent.includes("beta monthly balance warning"), "Selected-period Energy Path quality warning was not rendered");
  assert(!warningPanel.textContent.includes("annual balance warning"), "Selected-period warning panel leaked the annual warning");
  const driverInspector = document.createElement("div");
  const driverNode = {
    id: "driver.air.mechanical_ventilation.heating.beta",
    level: "driver",
    label: "Mechanical ventilation",
    value: 1,
    driverCategory: "air.mechanical_ventilation",
    serviceKind: "heating",
    sourceIds: ["beta-heating-fallback"],
  };
  driverInspector.innerHTML = module.renderEnergyPathNodeInspector(explanation, [driverNode], driverNode.id, state);
  assert(!driverInspector.querySelector('[data-energy-path-source]'), "Driver inspector still renders source details");
  assert(module.energyPathInspectorSources(explanation, driverNode, state).some((source) => source.id === "beta-driver-context"), "Driver calculation sources lost matching context-only provenance");

  const [{ state: applicationState }, selectionController, registry, simulationView] = await Promise.all([
    import("/src/js/state.js"),
    import("/src/js/selection-controller.js"),
    import("/src/js/panel-navigation-registry.js"),
    import("/src/js/views/simulation-views.js"),
  ]);
  const navigation = {
    entities: [{
      id: "air:mix",
      kind: "thermal_air_coupling",
      label: "Mix B to A",
      viewTargets: [{ view: "topology", targetKind: "thermal_air_coupling", targetId: "air:mix", priority: 120 }],
    }],
    occurrences: [],
  };
  let openedView = "";
  let revealedTarget = "";
  let completeNavigation;
  const navigationComplete = new Promise((resolve) => { completeNavigation = resolve; });
  registry.registerPanelNavigationAdapter("topology", {
    canReveal: (selection) => selection.viewTarget?.targetKind === "thermal_air_coupling" && selection.viewTarget.targetId === "air:mix",
    reveal: (selection) => {
      revealedTarget = selection.viewTarget?.targetId || "";
      completeNavigation();
      return true;
    },
    selectFromElement: () => null,
    captureContext: () => ({}),
    restoreContext: () => true,
    preferredSemanticOccurrence: () => "",
  });
  Object.assign(applicationState, {
    report: state.report,
    simulationEnergyScopeKind: "zone",
    simulationEnergyZoneName: "Beta Zone",
    semanticProjection: { navigation },
    analysisReady: { ...(applicationState.analysisReady || {}), topology: true },
  });
  selectionController.configureSelectionController({
    state: applicationState,
    getNavigationIndex: () => navigation,
    isAnalysisCurrent: () => true,
    isViewReady: () => true,
    openView: (view) => { openedView = view; },
  });
  sourceDetails.addEventListener("click", simulationView.handleSimulationSeriesInspectClick);
  airCouplingAction.click();
  await Promise.race([
    navigationComplete,
    new Promise((_, reject) => window.setTimeout(() => reject(new Error("Topology Air navigation timed out")), 1000)),
  ]);
  assert(openedView === "topology" && revealedTarget === "air:mix", "Click did not navigate to the exact related Topology Air edge");
  const forgedUnavailable = document.createElement("button");
  forgedUnavailable.dataset.energyPathTopologyAirCouplingId = "air:missing";
  assert(await simulationView.openSimulationEnergyPathTopologyAirCoupling(forgedUnavailable) === false, "Unavailable Topology entity did not fail gracefully");
  assert(document.getElementById("runtimeStatus").textContent.includes("unavailable"), "Unavailable Topology entity did not expose a status message");

  const unavailable = {
    schema: "semantic-idf.energy-explanation/v2",
    scope: { kind: "building", aggregationBasis: "model_total" },
    availableZones: ["Ghost Zone"],
    zoneResults: [],
    nodes: building.nodes,
    links: building.links,
  };
  const unavailableState = { simulationEnergyScopeKind: "zone", simulationEnergyPeriod: "annual", simulationEnergyService: "all" };
  module.normalizeEnergyPathViewState(unavailableState, unavailable);
  assert(module.energyPathZoneNames(unavailable).length === 0 && unavailableState.simulationEnergyScopeKind === "building", "Zone scope was offered without a precomputed zoneResult");

  const directZoneGraph = pathGraph("direct.annual", "Direct Zone", "annual");
  const directZone = {
    schema: "semantic-idf.energy-explanation/v2",
    scope: { kind: "zone", zoneName: "Direct Zone", aggregationBasis: "model_total" },
    nodes: directZoneGraph.nodes,
    links: directZoneGraph.links,
    periods: [],
  };
  const directState = { simulationEnergyScopeKind: "building", simulationEnergyPeriod: "annual", simulationEnergyService: "all" };
  module.normalizeEnergyPathViewState(directState, directZone);
  assert(directState.simulationEnergyScopeKind === "zone" && directState.simulationEnergyZoneName === "Direct Zone", "A top-level Zone payload was mislabeled as Building scope");
  assert(module.energyPathGraphForState(directZone, directState).nodes.every((node) => node.id.startsWith("direct.annual")), "A top-level Zone payload lost its graph during default normalization");
  assert(backendCalls === 0, "Scope, zone, period, or service selection called the backend");

  document.body.dataset.energyPathZoneStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathZoneStatus = "failed";
  document.getElementById("result").textContent = error?.stack || error?.message || String(error);
}
</script>
</body>
</html>`
