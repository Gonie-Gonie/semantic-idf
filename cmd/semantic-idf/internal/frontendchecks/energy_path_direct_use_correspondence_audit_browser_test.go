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

func TestEPATH093DirectUseCorrespondenceAuditBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser EPATH-093 direct-use correspondence audit in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-direct-use-correspondence-audit", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathDirectUseCorrespondenceAuditHarnessHTML)
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
		server.URL+"/energy-path-direct-use-correspondence-audit",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("EPATH-093 direct-use correspondence audit timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("EPATH-093 direct-use correspondence audit failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-direct-use-correspondence-audit-status="passed"`) {
		t.Fatalf("EPATH-093 direct-use correspondence frontend audit failed:\n%s", document)
	}
}

const energyPathDirectUseCorrespondenceAuditHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>EPATH-093 direct-use correspondence audit</title></head>
<body data-energy-path-direct-use-correspondence-audit-status="pending">
<div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const close = (left, right) => Math.abs(Number(left) - Number(right)) < 1e-9;

const makeScope = (scopeToken, zoneName, period, scale) => {
  const source = (family, kind) => kind + "." + family + "." + scopeToken + "." + period;
  const driver = (family, service, value) => ({
    id: "driver.internal." + family + "." + service + "." + scopeToken,
    level: "driver", kind: "driver.internal." + family, driverCategory: "internal." + family,
    label: family[0].toUpperCase() + family.slice(1) + " heat", value: value * scale,
    rawValue: value * scale, effectiveValue: value * scale, allocatedValue: value * scale,
    unit: "kWh thermal", scaleDomain: "thermal", serviceKind: service, period, zoneName,
    aggregationBasis: "model_total", sourceIds: [source(family, "heat")],
  });
  const endUse = (family, rawEndUse, value) => ({
    id: "end_use." + rawEndUse + "." + scopeToken,
    level: "end_use", kind: "energy." + rawEndUse, endUse: rawEndUse,
    label: family[0].toUpperCase() + family.slice(1) + " direct energy", value: value * scale,
    rawValue: value * scale, effectiveValue: value * scale, allocatedValue: value * scale,
    unit: "kWh site", scaleDomain: "site", period, zoneName, aggregationBasis: "model_total",
    sourceIds: [source(family, "meter")],
  });
  const load = (service, value) => ({
    id: "load." + service + "." + scopeToken, level: "load", kind: "load." + service,
    label: service + " load", value: value * scale, unit: "kWh thermal", scaleDomain: "thermal",
    serviceKind: service, period, zoneName, aggregationBasis: "model_total",
  });
  const lightingDriver = driver("lighting", "cooling", 8);
  const equipmentDriver = driver("equipment", "heating", 6);
  const peopleDriver = driver("people", "cooling", 2);
  const coolingLoad = load("cooling", 10);
  const heatingLoad = load("heating", 8);
  const lightingEndUse = endUse("lighting", "interior_lights", 10);
  const equipmentEndUse = endUse("equipment", "interior_equipment", 9);
  const carrier = {
    id: "carrier.electricity." + scopeToken, level: "carrier", kind: "carrier.electricity",
    carrier: "electricity", label: "Electricity", value: 19 * scale, unit: "kWh site",
    scaleDomain: "site", period, zoneName, aggregationBasis: "model_total",
  };
  const relation = (id, fromId, toId, serviceKind, fromValue, toValue, sourceIds, sentinel) => ({
    id, fromId, toId, relation: "source_correspondence", basis: "reported_correspondence",
    fromValue: fromValue * scale, fromUnit: "kWh thermal", toValue: toValue * scale, toUnit: "kWh site",
    serviceKind, period, zoneName, sourceIds, explanation: sentinel,
  });
  const links = [
    { id: "flow.lighting." + scopeToken + "." + period, fromId: lightingDriver.id, toId: coolingLoad.id, relation: "driver_to_load", basis: "heat_balance_share", fromValue: 8 * scale, toValue: 8 * scale, fromUnit: "kWh thermal", toUnit: "kWh thermal", serviceKind: "cooling", period },
    { id: "flow.people." + scopeToken + "." + period, fromId: peopleDriver.id, toId: coolingLoad.id, relation: "driver_to_load", basis: "heat_balance_share", fromValue: 2 * scale, toValue: 2 * scale, fromUnit: "kWh thermal", toUnit: "kWh thermal", serviceKind: "cooling", period },
    { id: "flow.equipment." + scopeToken + "." + period, fromId: equipmentDriver.id, toId: heatingLoad.id, relation: "driver_to_load", basis: "heat_balance_share", fromValue: 6 * scale, toValue: 6 * scale, fromUnit: "kWh thermal", toUnit: "kWh thermal", serviceKind: "heating", period },
    { id: "carrier.lighting." + scopeToken + "." + period, fromId: lightingEndUse.id, toId: carrier.id, relation: "direct_end_use_to_carrier", basis: "reported_variable", fromValue: 10 * scale, toValue: 10 * scale, fromUnit: "kWh site", toUnit: "kWh site", period },
    { id: "carrier.equipment." + scopeToken + "." + period, fromId: equipmentEndUse.id, toId: carrier.id, relation: "direct_end_use_to_carrier", basis: "reported_variable", fromValue: 9 * scale, toValue: 9 * scale, fromUnit: "kWh site", toUnit: "kWh site", period },
    relation("relation.lighting." + scopeToken + "." + period, lightingDriver.id, lightingEndUse.id, "cooling", 8, 10, [source("lighting", "heat"), source("lighting", "meter")], "NONFLOW_SENTINEL_LIGHTING"),
    relation("relation.equipment." + scopeToken + "." + period, equipmentDriver.id, equipmentEndUse.id, "heating", 6, 9, [source("equipment", "heat"), source("equipment", "meter")], "NONFLOW_SENTINEL_EQUIPMENT"),
    relation("relation.people.trap." + scopeToken + "." + period, peopleDriver.id, lightingEndUse.id, "cooling", 2, 10, [source("people", "heat"), source("lighting", "meter")], "NONFLOW_SENTINEL_PEOPLE_TRAP"),
    relation("relation.crossed.trap." + scopeToken + "." + period, lightingDriver.id, equipmentEndUse.id, "heating", 8, 9, [source("lighting", "heat"), source("equipment", "meter")], "NONFLOW_SENTINEL_CROSSED_TRAP"),
  ];
  const nodes = [lightingDriver, equipmentDriver, peopleDriver, coolingLoad, heatingLoad, lightingEndUse, equipmentEndUse, carrier];
  const sources = [
    { id: source("lighting", "heat"), sourceType: "sql_variable", name: zoneName + " Lighting thermal source " + period, zoneName, driverCategory: "internal.lighting", inspectorSection: "breakdown" },
    { id: source("lighting", "meter"), sourceType: "sql_variable", name: zoneName + " Lighting site-energy source " + period, zoneName, inspectorSection: "breakdown" },
    { id: source("equipment", "heat"), sourceType: "sql_variable", name: zoneName + " Equipment thermal source " + period, zoneName, driverCategory: "internal.equipment", inspectorSection: "breakdown" },
    { id: source("equipment", "meter"), sourceType: "sql_variable", name: zoneName + " Equipment site-energy source " + period, zoneName, inspectorSection: "breakdown" },
    { id: source("people", "heat"), sourceType: "sql_variable", name: zoneName + " People thermal source " + period, zoneName, driverCategory: "internal.people", inspectorSection: "breakdown" },
  ];
  return { nodes, links, sources };
};

const reverseGraph = (graph) => ({
  ...graph,
  nodes: [...(graph.nodes || [])].reverse(),
  links: [...(graph.links || [])].reverse().map((link) => ({ ...link, sourceIds: [...(link.sourceIds || [])].reverse() })),
});

const pairSignature = (module, graph) => module.energyPathCorrespondencePairs(graph.nodes, graph.relations)
  .map((pair) => ({
    driver: pair.driverNode.id,
    endUse: pair.endUseNode.id,
    fromValue: pair.relation.fromValue,
    toValue: pair.relation.toValue,
    sources: [...(pair.relation.sourceIds || [])].sort(),
  }))
  .sort((left, right) => left.driver.localeCompare(right.driver));

const assertAcyclicForwardFlow = (graph) => {
  const ranks = { driver: 0, load: 1, end_use: 2, carrier: 3 };
  const byID = new Map(graph.nodes.map((node) => [node.id, node]));
  const adjacency = new Map();
  for (const link of graph.links) {
    const from = byID.get(link.fromId);
    const to = byID.get(link.toId);
    assert(from && to, "flow link retained a missing endpoint");
    assert(ranks[from.level] < ranks[to.level], "flow graph retained a reverse or same-stage link: " + link.id);
    adjacency.set(link.fromId, [...(adjacency.get(link.fromId) || []), link.toId]);
  }
  const visiting = new Set();
  const complete = new Set();
  const visit = (id) => {
    if (visiting.has(id)) return true;
    if (complete.has(id)) return false;
    visiting.add(id);
    for (const next of adjacency.get(id) || []) {
      if (visit(next)) return true;
    }
    visiting.delete(id);
    complete.add(id);
    return false;
  };
  for (const node of graph.nodes) assert(!visit(node.id), "flow graph contains a cycle");
};

try {
  const module = await import("/src/js/views/energy-path-view.js");
  const annual = makeScope("building", "", "annual", 1);
  const m1 = makeScope("building", "", "M1", 0.25);
  const m2 = makeScope("building", "", "M2", 0.75);
  const officeAnnual = makeScope("office", "Office", "annual", 0.4);
  const officeM1 = makeScope("office", "Office", "M1", 0.1);
  const explanation = {
    schema: module.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    nodes: annual.nodes,
    links: annual.links,
    periods: [
      { id: "M1", label: "January", kind: "monthly", nodes: m1.nodes, links: m1.links },
      { id: "M2", label: "February", kind: "monthly", nodes: m2.nodes, links: m2.links },
    ],
    availableZones: ["Office"],
    zoneResults: [{
      scope: { kind: "zone", zoneName: "Office", aggregationBasis: "model_total" },
      nodes: officeAnnual.nodes,
      links: officeAnnual.links,
      periods: [{ id: "M1", label: "January", kind: "monthly", nodes: officeM1.nodes, links: officeM1.links }],
    }],
    sources: [...annual.sources, ...m1.sources, ...m2.sources, ...officeAnnual.sources, ...officeM1.sources],
  };
  const originalPayload = JSON.stringify(explanation);
  const baseState = { simulationEnergyScopeKind: "building", simulationEnergyPeriod: "annual", simulationEnergyService: "all", simulationEnergySelection: "" };
  const graph = module.energyPathGraphForState(explanation, baseState);
  const pairs = module.energyPathCorrespondencePairs(graph.nodes, graph.relations);

  assert(graph.links.every((link) => link.relation !== "source_correspondence"), "non-flow correspondence remained in main flow links");
  assert(graph.relations.length === 4 && graph.relations.every((relation) => module.isEnergyPathNonFlowRelation(relation)), "payload non-flow relations were not isolated");
  assert(pairs.length === 2, "malformed People/crossed correspondence became actionable");
  assert(graph.nodes.some((node) => node.id === "end_use.lighting.building" && node.endUse === "lighting"), "raw lighting taxonomy was not projected");
  assert(graph.nodes.some((node) => node.id === "end_use.equipment.building" && node.endUse === "equipment"), "raw equipment taxonomy was not projected");
  assert(!graph.nodes.some((node) => node.level === "end_use" && (node.endUse === "people" || node.id.includes(".people."))), "People became a direct Stage 3 node");
  const people = graph.nodes.find((node) => node.driverCategory === "internal.people");
  assert(people && module.energyPathCorrespondenceCounterparts(graph.nodes, graph.relations, people.id).length === 0, "People acquired a direct-energy counterpart");
  assertAcyclicForwardFlow(graph);

  const lightingPair = pairs.find((pair) => pair.endUseNode.endUse === "lighting");
  const equipmentPair = pairs.find((pair) => pair.endUseNode.endUse === "equipment");
  assert(lightingPair && close(lightingPair.relation.fromValue, 8) && close(lightingPair.relation.toValue, 10), "Lighting two-sided values changed during projection");
  assert(equipmentPair && close(equipmentPair.relation.fromValue, 6) && close(equipmentPair.relation.toValue, 9), "Equipment two-sided values changed during projection");
  assert(JSON.stringify([...(lightingPair.relation.sourceIds || [])].sort()) === JSON.stringify(["heat.lighting.building.annual", "meter.lighting.building.annual"]), "Lighting relation lost exact source provenance");
  assert(JSON.stringify([...(equipmentPair.relation.sourceIds || [])].sort()) === JSON.stringify(["heat.equipment.building.annual", "meter.equipment.building.annual"]), "Equipment relation lost exact source provenance");

  // One direct-use family can affect both cooling and heating. In the default
  // all-service projection those service-specific relations become one
  // actionable correspondence, but the shared site-energy endpoint must not
  // be counted once per service like a flow ribbon.
  const heatingLightingDriver = {
    ...annual.nodes.find((node) => node.driverCategory === "internal.lighting"),
    id: "driver.internal.lighting.heating.building",
    value: 3,
    rawValue: 3,
    effectiveValue: 3,
    allocatedValue: 3,
    serviceKind: "heating",
    sourceIds: ["heat.lighting.heating.building.annual"],
  };
  const multiServiceAnnual = {
    ...annual,
    nodes: [...annual.nodes, heatingLightingDriver],
    links: [
      ...annual.links,
      {
        id: "relation.lighting.heating.building.annual",
        fromId: heatingLightingDriver.id,
        toId: "end_use.interior_lights.building",
        relation: "source_correspondence",
        basis: "reported_correspondence",
        fromValue: 3,
        fromUnit: "kWh thermal",
        toValue: 10,
        toUnit: "kWh site",
        serviceKind: "heating",
        period: "annual",
        sourceIds: ["heat.lighting.heating.building.annual", "meter.lighting.building.annual"],
      },
    ],
  };
  const multiServiceExplanation = {
    ...explanation,
    nodes: multiServiceAnnual.nodes,
    links: multiServiceAnnual.links,
  };
  const multiServiceGraph = module.energyPathGraphForState(multiServiceExplanation, baseState);
  const multiServiceLightingPairs = module.energyPathCorrespondencePairs(
    multiServiceGraph.nodes,
    multiServiceGraph.relations,
  ).filter((pair) => pair.endUseNode.endUse === "lighting");
  assert(multiServiceLightingPairs.length === 1, "all-service projection duplicated the Lighting correspondence action");
  const multiServiceLighting = multiServiceLightingPairs[0];
  assert(close(multiServiceLighting.driverNode.value, 11), "all-service Lighting driver did not merge cooling and heating values");
  assert(close(multiServiceLighting.endUseNode.value, 10), "all-service Lighting end-use value changed");
  assert(close(multiServiceLighting.relation.fromValue, multiServiceLighting.driverNode.value), "non-flow fromValue does not match the projected thermal endpoint");
  assert(close(multiServiceLighting.relation.toValue, multiServiceLighting.endUseNode.value), "non-flow toValue was counted once per service");
  assert(!multiServiceLighting.relation.ratio && !multiServiceLighting.relation.ratioKind && !multiServiceLighting.relation.ratioLabel, "all-service correspondence gained conversion ratio metadata");
  const reversedMultiServiceGraph = module.energyPathGraphForState({
    ...multiServiceExplanation,
    nodes: [...multiServiceExplanation.nodes].reverse(),
    links: [...multiServiceExplanation.links].reverse(),
  }, baseState);
  assert(
    JSON.stringify(pairSignature(module, multiServiceGraph)) === JSON.stringify(pairSignature(module, reversedMultiServiceGraph)),
    "multi-service correspondence depends on payload ordering",
  );

  const mount = document.getElementById("mount");
  mount.innerHTML = module.renderEnergyPathView(explanation, { ...baseState });
  assert(!mount.querySelector("[data-energy-path-correspondence-actions], [data-energy-path-related]"), "default graph exposed a non-flow relation without selection");
  assert(!mount.innerHTML.includes("NONFLOW_SENTINEL_"), "source correspondence was rendered as a default ribbon/line");
  assert(!mount.querySelector("[data-energy-path-flow-lanes]")?.textContent.includes("source_correspondence"), "non-flow relation leaked into a flow lane");

  const coolingState = { ...baseState, simulationEnergyService: "cooling" };
  const coolingGraph = module.energyPathGraphForState(explanation, coolingState);
  const coolingPairs = module.energyPathCorrespondencePairs(coolingGraph.nodes, coolingGraph.relations);
  assert(coolingPairs.length === 1 && coolingPairs[0].endUseNode.endUse === "lighting", "Cooling service did not isolate Lighting correspondence");
  const coolingDriverID = "driver.internal.lighting.cooling.building";
  const lightingEndUseID = "end_use.lighting.building";
  mount.innerHTML = module.renderEnergyPathView(explanation, { ...coolingState, simulationEnergySelection: coolingDriverID });
  assert(mount.querySelector('.energy-path-node.selected[data-energy-explanation-node="' + coolingDriverID + '"]')?.getAttribute("aria-pressed") === "true", "Lighting heat selection is not visible");
  assert(mount.querySelector('.energy-path-node.related[data-energy-path-related="true"][data-energy-explanation-node="' + lightingEndUseID + '"]'), "Lighting energy counterpart was not outline-highlighted");
  const energyAction = mount.querySelector('[data-energy-path-correspondence-action="related_energy_use"]');
  assert(energyAction?.dataset.energyExplanationNode === lightingEndUseID && energyAction.textContent.includes("Related energy use"), "Lighting heat -> energy action is missing");
  const driverInspector = mount.querySelector('[data-energy-path-inspector="' + coolingDriverID + '"]');
  assert(driverInspector?.textContent.includes("Lighting thermal source annual") && !driverInspector.textContent.includes("Lighting site-energy source annual"), "Lighting thermal source provenance is not endpoint-specific");

  let actionTarget = "";
  mount.addEventListener("click", (event) => {
    actionTarget = event.target.closest("[data-energy-explanation-node]")?.dataset.energyExplanationNode || "";
  }, { once: true });
  energyAction.focus();
  assert(document.activeElement === energyAction, "correspondence action cannot receive keyboard focus");
  energyAction.click();
  assert(actionTarget === lightingEndUseID, "correspondence action bypassed the existing node-selection target");

  mount.innerHTML = module.renderEnergyPathView(explanation, { ...coolingState, simulationEnergySelection: lightingEndUseID });
  assert(mount.querySelector('.energy-path-node.related[data-energy-path-related="true"][data-energy-explanation-node="' + coolingDriverID + '"]'), "Lighting heat counterpart was not outline-highlighted from energy selection");
  const thermalAction = mount.querySelector('[data-energy-path-correspondence-action="related_thermal_effect"]');
  assert(thermalAction?.dataset.energyExplanationNode === coolingDriverID && thermalAction.textContent.includes("Related thermal effect"), "Lighting energy -> thermal action is missing");
  const energyInspector = mount.querySelector('[data-energy-path-inspector="' + lightingEndUseID + '"]');
  assert(energyInspector?.textContent.includes("Lighting site-energy source annual") && !energyInspector.textContent.includes("Lighting thermal source annual"), "Lighting site-energy provenance is not endpoint-specific");

  const heatingState = { ...baseState, simulationEnergyService: "heating" };
  const heatingGraph = module.energyPathGraphForState(explanation, heatingState);
  const heatingPairs = module.energyPathCorrespondencePairs(heatingGraph.nodes, heatingGraph.relations);
  assert(heatingPairs.length === 1 && heatingPairs[0].endUseNode.endUse === "equipment", "Heating service did not isolate Equipment correspondence");
  const equipmentDriverID = "driver.internal.equipment.heating.building";
  const equipmentEndUseID = "end_use.equipment.building";
  mount.innerHTML = module.renderEnergyPathView(explanation, { ...heatingState, simulationEnergySelection: equipmentDriverID });
  assert(mount.querySelector('.energy-path-node.related[data-energy-path-related="true"][data-energy-explanation-node="' + equipmentEndUseID + '"]'), "Equipment energy counterpart was not highlighted");
  assert(mount.querySelector('[data-energy-path-correspondence-action="related_energy_use"]')?.dataset.energyExplanationNode === equipmentEndUseID, "Equipment heat -> energy action is missing");
  mount.innerHTML = module.renderEnergyPathView(explanation, { ...heatingState, simulationEnergySelection: equipmentEndUseID });
  assert(mount.querySelector('.energy-path-node.related[data-energy-path-related="true"][data-energy-explanation-node="' + equipmentDriverID + '"]'), "Equipment heat counterpart was not highlighted from energy selection");
  assert(mount.querySelector('[data-energy-path-correspondence-action="related_thermal_effect"]')?.dataset.energyExplanationNode === equipmentDriverID, "Equipment energy -> thermal action is missing");

  const states = [
    baseState,
    { ...baseState, simulationEnergyPeriod: "M1" },
    { ...baseState, simulationEnergyPeriod: "M2" },
    { ...baseState, simulationEnergyScopeKind: "zone", simulationEnergyZoneName: "Office" },
    { ...baseState, simulationEnergyScopeKind: "zone", simulationEnergyZoneName: "Office", simulationEnergyPeriod: "M1" },
  ];
  const reversed = {
    ...explanation,
    ...reverseGraph(explanation),
    periods: [...explanation.periods].reverse().map(reverseGraph),
    zoneResults: [...explanation.zoneResults].reverse().map((zone) => ({ ...zone, ...reverseGraph(zone), periods: [...(zone.periods || [])].reverse().map(reverseGraph) })),
    sources: [...explanation.sources].reverse(),
  };
  for (const state of states) {
    const left = module.energyPathGraphForState(explanation, { ...state });
    const right = module.energyPathGraphForState(reversed, { ...state });
    assert(JSON.stringify(pairSignature(module, left)) === JSON.stringify(pairSignature(module, right)), "correspondence depends on payload ordering for " + JSON.stringify(state));
    assertAcyclicForwardFlow(left);
    const expectedScope = state.simulationEnergyScopeKind === "zone" ? "office" : "building";
    const expectedPeriod = state.simulationEnergyPeriod || "annual";
    const scopedLighting = module.energyPathCorrespondencePairs(left.nodes, left.relations).find((pair) => pair.endUseNode.endUse === "lighting");
    assert(scopedLighting?.driverNode.id.endsWith("." + expectedScope) && scopedLighting?.endUseNode.id === "end_use.lighting." + expectedScope, "scope projection lost correspondence for " + JSON.stringify(state));
    assert((scopedLighting?.relation.sourceIds || []).every((id) => id.endsWith("." + expectedScope + "." + expectedPeriod)), "period/scope source provenance leaked across graphs for " + JSON.stringify(state));
  }
  assert(JSON.stringify(explanation) === originalPayload, "frontend correspondence projection mutated the payload");

  document.body.dataset.energyPathDirectUseCorrespondenceAuditStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathDirectUseCorrespondenceAuditStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
