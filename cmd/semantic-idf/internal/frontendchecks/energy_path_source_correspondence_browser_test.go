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

func TestEPATH093SourceCorrespondenceBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser EPATH-093 source-correspondence harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-source-correspondence", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathSourceCorrespondenceHarnessHTML)
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
		server.URL+"/energy-path-source-correspondence",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("EPATH-093 source-correspondence browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("EPATH-093 source-correspondence browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-source-correspondence-status="passed"`) {
		t.Fatalf("EPATH-093 source-correspondence frontend contract failed:\n%s", document)
	}
}

const energyPathSourceCorrespondenceHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>EPATH-093 source correspondence</title></head>
<body data-energy-path-source-correspondence-status="pending">
<div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };

const fixture = (scopeToken = "building", zoneName = "", period = "annual") => {
  const suffix = scopeToken;
  const driver = (kind, value) => ({
    id: "driver.internal." + kind + ".cooling." + suffix,
    level: "driver", kind: "driver.internal." + kind, driverCategory: "internal." + kind,
    label: kind[0].toUpperCase() + kind.slice(1) + " heat", value, rawValue: value,
    effectiveValue: value, allocatedValue: value, unit: "kWh", scaleDomain: "thermal",
    serviceKind: "cooling", period, zoneName,
  });
  const endUse = (kind, rawKind, value) => ({
    id: "end_use." + rawKind + "." + suffix,
    level: "end_use", kind: "end_use." + rawKind, endUse: rawKind,
    label: kind[0].toUpperCase() + kind.slice(1) + " electricity", value,
    rawValue: value, effectiveValue: value, allocatedValue: value,
    unit: "kWh", scaleDomain: "site", period, zoneName,
  });
  const lightingDriver = driver("lighting", 18);
  const equipmentDriver = driver("equipment", 12);
  const peopleDriver = driver("people", 9);
  const lightingEndUse = endUse("lighting", "interior_lighting", 20);
  const equipmentEndUse = endUse("equipment", "interior_equipment", 14);
  const carrier = {
    id: "carrier.electricity." + suffix, level: "carrier", kind: "carrier.electricity",
    carrier: "electricity", label: "Electricity", value: 34, unit: "kWh",
    scaleDomain: "site", period, zoneName,
  };
  const nodes = [lightingDriver, equipmentDriver, peopleDriver, lightingEndUse, equipmentEndUse, carrier];
  const correspondence = (id, fromId, toId, serviceKind = "cooling") => ({
    id, fromId, toId, relation: "source_correspondence", basis: "derived_ratio",
    fromValue: 1, toValue: 1, fromUnit: "kWh", toUnit: "kWh", serviceKind, period,
  });
  const links = [
    correspondence("correspondence.lighting." + suffix, lightingDriver.id, lightingEndUse.id),
    correspondence("correspondence.equipment." + suffix, equipmentDriver.id, equipmentEndUse.id),
    // Deliberately malformed: People must never acquire a direct-energy counterpart in the UI.
    correspondence("correspondence.people.trap." + suffix, peopleDriver.id, lightingEndUse.id),
    { id: "carrier.lighting." + suffix, fromId: lightingEndUse.id, toId: carrier.id, relation: "direct_end_use_to_carrier", basis: "reported_meter", fromValue: 20, toValue: 20, fromUnit: "kWh", toUnit: "kWh", period },
    { id: "carrier.equipment." + suffix, fromId: equipmentEndUse.id, toId: carrier.id, relation: "direct_end_use_to_carrier", basis: "reported_meter", fromValue: 14, toValue: 14, fromUnit: "kWh", toUnit: "kWh", period },
  ];
  return { nodes, links };
};

try {
  const module = await import("/src/js/views/energy-path-view.js");
  const i18n = await import("/src/js/i18n.js");
  const annual = fixture();
  const monthly = fixture("building", "", "M1");
  const office = fixture("office", "Office", "annual");
  const explanation = {
    schema: module.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    nodes: annual.nodes,
    links: annual.links,
    periods: [{ id: "M1", nodes: monthly.nodes, links: monthly.links }],
    availableZones: ["Office"],
    zoneResults: [{
      schema: module.ENERGY_PATH_SCHEMA_V2,
      scope: { kind: "zone", zoneName: "Office", aggregationBasis: "zone_total" },
      nodes: office.nodes, links: office.links,
    }],
  };
  const baseState = {
    simulationEnergyScopeKind: "building", simulationEnergyPeriod: "annual",
    simulationEnergyService: "all", simulationEnergySelection: "",
  };
  const graph = module.energyPathGraphForState(explanation, baseState);
  const pairs = module.energyPathCorrespondencePairs(graph.nodes, graph.relations);
  const lightingEndUse = graph.nodes.find((node) => node.level === "end_use" && node.endUse === "lighting");
  const equipmentEndUse = graph.nodes.find((node) => node.level === "end_use" && node.endUse === "equipment");
  const lightingDriver = graph.nodes.find((node) => node.level === "driver" && node.driverCategory === "internal.lighting");
  const equipmentDriver = graph.nodes.find((node) => node.level === "driver" && node.driverCategory === "internal.equipment");
  const peopleDriver = graph.nodes.find((node) => node.level === "driver" && node.driverCategory === "internal.people");

  assert(graph.links.every((link) => link.relation !== "source_correspondence"), "a non-flow relation remained in graph.links");
  assert(graph.relations.length === 3 && graph.relations.every((link) => link.relation === "source_correspondence"), "payload correspondence links were not structurally isolated");
  assert(pairs.length === 2, "only Lighting and Equipment correspondence pairs should be actionable");
  assert(lightingEndUse?.id === "end_use.lighting.building" && equipmentEndUse?.id === "end_use.equipment.building", "correspondence endpoints did not survive end-use taxonomy projection");
  assert(lightingDriver?.id === "driver.internal.lighting.all.building" && equipmentDriver?.id === "driver.internal.equipment.all.building", "correspondence driver endpoints did not survive all-service projection");
  assert(module.energyPathCorrespondenceCounterparts(graph.nodes, graph.relations, peopleDriver.id).length === 0, "People acquired a direct site-energy presentation");

  const stageOrder = { driver: 0, load: 1, end_use: 2, carrier: 3 };
  const nodeByID = new Map(graph.nodes.map((node) => [node.id, node]));
  assert(graph.links.every((link) => stageOrder[nodeByID.get(link.fromId)?.level] < stageOrder[nodeByID.get(link.toId)?.level]), "flow graph gained a cycle or reverse link");

  const mount = document.getElementById("mount");
  mount.innerHTML = module.renderEnergyPathView(explanation, { ...baseState, simulationEnergySelection: lightingEndUse.id });
  const selectedLighting = mount.querySelector('[data-energy-explanation-node="' + lightingEndUse.id + '"]');
  const relatedLightingHeat = mount.querySelector('[data-energy-explanation-node="' + lightingDriver.id + '"][data-energy-path-related="true"]');
  const thermalAction = mount.querySelector('[data-energy-path-correspondence-action="related_thermal_effect"]');
  assert(selectedLighting?.classList.contains("selected") && relatedLightingHeat?.classList.contains("related"), "selecting Lighting energy did not outline Lighting heat");
  assert(thermalAction?.tagName === "BUTTON" && thermalAction.type === "button" && thermalAction.tabIndex === 0, "related thermal-effect action is not keyboard-accessible");
  assert(thermalAction.dataset.energyExplanationNode === lightingDriver.id && thermalAction.textContent.includes("Related thermal effect"), "related thermal-effect action does not use the existing node-selection target");
  assert(!mount.querySelector("[data-energy-path-correspondence-link]"), "source correspondence was drawn as a default ribbon or lane");
  assert(!mount.querySelector("[data-energy-path-flow-lanes]")?.textContent.includes("source_correspondence"), "source correspondence leaked into flow lanes");

  let selectedThroughExistingMechanism = "";
  thermalAction.addEventListener("click", (event) => {
    selectedThroughExistingMechanism = event.target.closest("[data-energy-explanation-node]")?.dataset.energyExplanationNode || "";
  });
  thermalAction.focus();
  assert(document.activeElement === thermalAction, "related action cannot receive keyboard focus");
  thermalAction.click();
  assert(selectedThroughExistingMechanism === lightingDriver.id, "related action bypassed the existing energy-node selection mechanism");

  mount.innerHTML = module.renderEnergyPathView(explanation, { ...baseState, simulationEnergySelection: equipmentDriver.id });
  const relatedEquipmentEnergy = mount.querySelector('[data-energy-explanation-node="' + equipmentEndUse.id + '"][data-energy-path-related="true"]');
  const energyUseAction = mount.querySelector('[data-energy-path-correspondence-action="related_energy_use"]');
  assert(relatedEquipmentEnergy?.classList.contains("related"), "selecting Equipment heat did not outline Equipment energy");
  assert(energyUseAction?.dataset.energyExplanationNode === equipmentEndUse.id && energyUseAction.textContent.includes("Related energy use"), "reverse counterpart action is missing");

  const coolingGraph = module.energyPathGraphForState(explanation, { ...baseState, simulationEnergyService: "cooling" });
  const coolingLighting = coolingGraph.nodes.find((node) => node.level === "end_use" && node.endUse === "lighting");
  const coolingDriver = coolingGraph.nodes.find((node) => node.driverCategory === "internal.lighting");
  assert(coolingLighting && coolingDriver && module.energyPathCorrespondenceCounterparts(coolingGraph.nodes, coolingGraph.relations, coolingLighting.id)[0]?.id === coolingDriver.id, "correspondence was lost through service filtering");

  const monthlyGraph = module.energyPathGraphForState(explanation, { ...baseState, simulationEnergyPeriod: "M1" });
  const monthlyLighting = monthlyGraph.nodes.find((node) => node.level === "end_use" && node.endUse === "lighting");
  assert(monthlyLighting && module.energyPathCorrespondenceCounterparts(monthlyGraph.nodes, monthlyGraph.relations, monthlyLighting.id).length === 1, "correspondence was lost through period filtering");

  const zoneState = { ...baseState, simulationEnergyScopeKind: "zone", simulationEnergyZoneName: "Office" };
  const zoneGraph = module.energyPathGraphForState(explanation, zoneState);
  const zoneLighting = zoneGraph.nodes.find((node) => node.level === "end_use" && node.endUse === "lighting");
  assert(zoneLighting?.id === "end_use.lighting.office" && module.energyPathCorrespondenceCounterparts(zoneGraph.nodes, zoneGraph.relations, zoneLighting.id).length === 1, "correspondence was lost through zone scope filtering");

  i18n.setLanguage("ko");
  mount.innerHTML = module.renderEnergyPathView(explanation, { ...baseState, simulationEnergySelection: lightingEndUse.id });
  assert(mount.querySelector('[data-energy-path-correspondence-action="related_thermal_effect"]')?.textContent.includes("관련 열 영향"), "Korean related thermal-effect action is missing");
  mount.innerHTML = module.renderEnergyPathView(explanation, { ...baseState, simulationEnergySelection: lightingDriver.id });
  assert(mount.querySelector('[data-energy-path-correspondence-action="related_energy_use"]')?.textContent.includes("관련 에너지 사용"), "Korean related energy-use action is missing");
  i18n.setLanguage("en");

  document.body.dataset.energyPathSourceCorrespondenceStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathSourceCorrespondenceStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
