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

func TestEPATH094ZoneDirectUseBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser EPATH-094 zone direct-use harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-zone-direct-use", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathZoneDirectUseHarnessHTML)
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
		server.URL+"/energy-path-zone-direct-use",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("EPATH-094 zone direct-use browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("EPATH-094 zone direct-use browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-zone-direct-use-status="passed"`) {
		t.Fatalf("EPATH-094 zone direct-use frontend contract failed:\n%s", document)
	}
}

const energyPathZoneDirectUseHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>EPATH-094 zone direct use</title></head>
<body data-energy-path-zone-direct-use-status="pending">
<div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };

const zoneGraph = (period = "annual", factor = 1) => {
  const nodes = [
    { id: "end_use.interior_lighting.office", level: "end_use", kind: "end_use.interior_lighting", label: "Office lighting", value: 5 * factor, rawValue: 5 * factor, effectiveValue: 5 * factor, allocatedValue: 5 * factor, unit: "kWh", scaleDomain: "site", period, zoneName: "Office", endUse: "interior_lighting", basis: "direct_zone_energy", meterHierarchyLevel: "zone_direct_use", sourceIds: ["variable.zone.lights"] },
    { id: "end_use.interior_equipment.electricity.office", level: "end_use", kind: "end_use.interior_equipment", label: "Office electric equipment", value: 7 * factor, rawValue: 7 * factor, effectiveValue: 7 * factor, allocatedValue: 7 * factor, unit: "kWh", scaleDomain: "site", period, zoneName: "Office", endUse: "interior_equipment", basis: "direct_zone_energy", meterHierarchyLevel: "zone_direct_use", sourceIds: ["variable.zone.equipment.electric"] },
    { id: "end_use.interior_equipment.gas.office", level: "end_use", kind: "end_use.interior_equipment", label: "Office gas equipment", value: 3 * factor, rawValue: 3 * factor, effectiveValue: 3 * factor, allocatedValue: 3 * factor, unit: "kWh", scaleDomain: "site", period, zoneName: "Office", endUse: "interior_equipment", basis: "direct_zone_energy", meterHierarchyLevel: "zone_direct_use", sourceIds: ["variable.zone.equipment.gas"] },
    // Explicit Building-meter fallback traps must never reach the Zone UI.
    { id: "end_use.interior_lighting.fallback.office", level: "end_use", kind: "end_use.interior_lighting", label: "Allocated Building lighting meter", value: 100 * factor, unit: "kWh", scaleDomain: "site", period, zoneName: "Office", endUse: "interior_lighting", basis: "reported_meter", meterHierarchyLevel: "broad_end_use", sourceIds: ["meter.building.lights"] },
    { id: "end_use.interior_equipment.fallback.office", level: "end_use", kind: "end_use.interior_equipment", label: "Allocated Building equipment meter", value: 200 * factor, unit: "kWh", scaleDomain: "site", period, zoneName: "Office", endUse: "interior_equipment", basis: "zone_load_allocation", meterHierarchyLevel: "broad_end_use", sourceIds: ["meter.building.equipment"] },
    { id: "carrier.electricity.office", level: "carrier", kind: "carrier.electricity", label: "Electricity", value: 12 * factor, unit: "kWh", scaleDomain: "site", period, zoneName: "Office", carrier: "electricity", basis: "direct_zone_energy" },
    { id: "carrier.natural_gas.office", level: "carrier", kind: "carrier.natural_gas", label: "Natural gas", value: 3 * factor, unit: "kWh", scaleDomain: "site", period, zoneName: "Office", carrier: "natural_gas", basis: "direct_zone_energy" },
  ];
  const links = [
    { id: "direct.lights", fromId: nodes[0].id, toId: nodes[5].id, relation: "direct_end_use_to_carrier", basis: "direct_zone_energy", fromValue: 5 * factor, toValue: 5 * factor, fromUnit: "kWh", toUnit: "kWh", period, zoneName: "Office", sourceIds: ["variable.zone.lights"] },
    { id: "direct.equipment.electric", fromId: nodes[1].id, toId: nodes[5].id, relation: "direct_end_use_to_carrier", basis: "direct_zone_energy", fromValue: 7 * factor, toValue: 7 * factor, fromUnit: "kWh", toUnit: "kWh", period, zoneName: "Office", sourceIds: ["variable.zone.equipment.electric"] },
    { id: "direct.equipment.gas", fromId: nodes[2].id, toId: nodes[6].id, relation: "direct_end_use_to_carrier", basis: "direct_zone_energy", fromValue: 3 * factor, toValue: 3 * factor, fromUnit: "kWh", toUnit: "kWh", period, zoneName: "Office", sourceIds: ["variable.zone.equipment.gas"] },
    { id: "fallback.lights", fromId: nodes[3].id, toId: nodes[5].id, relation: "end_use_to_carrier", basis: "reported_meter", fromValue: 100 * factor, toValue: 100 * factor, fromUnit: "kWh", toUnit: "kWh", period, zoneName: "Office", sourceIds: ["meter.building.lights"] },
    { id: "fallback.equipment", fromId: nodes[4].id, toId: nodes[5].id, relation: "end_use_to_carrier", basis: "zone_load_allocation", fromValue: 200 * factor, toValue: 200 * factor, fromUnit: "kWh", toUnit: "kWh", period, zoneName: "Office", sourceIds: ["meter.building.equipment"] },
  ];
  return { nodes, links };
};

const partialCompleteness = () => ({
  status: "partial", mappedPercent: 0,
  energyUse: { level: "energy", status: "partial", found: 0, total: 0, message: "Observed direct-use subtotal; full Zone carrier coverage is unknown" },
  deliveredLoad: { level: "load", status: "complete", found: 1, total: 1 },
  heatDrivers: { level: "heat", status: "complete", found: 1, total: 1 },
});

const zoneSummary = (period = "annual", factor = 1, partial = true) => ({
  schema: "semantic-idf.energy-explanation-summary/v2", period,
  scope: { kind: "zone", zoneName: "Office", aggregationBasis: "zone_total" },
  drivers: [], loads: [], ratios: [], residuals: [], topZones: [],
  endUses: [
    { id: "summary.lighting", level: "end_use", label: "Lighting", value: 5 * factor, unit: "kWh site", endUse: "lighting", basis: "direct_zone_energy", meterHierarchyLevel: "zone_direct_use", sourceIds: ["variable.zone.lights"] },
    { id: "summary.equipment", level: "end_use", label: "Equipment", value: 10 * factor, unit: "kWh site", endUse: "equipment", basis: "direct_zone_energy", meterHierarchyLevel: "zone_direct_use", sourceIds: ["variable.zone.equipment.electric", "variable.zone.equipment.gas"] },
    { id: "summary.fallback", level: "end_use", label: "Building meter fallback", value: 300 * factor, unit: "kWh site", endUse: "interior_equipment", basis: "reported_meter", meterHierarchyLevel: "broad_end_use", sourceIds: ["meter.building.equipment"] },
  ],
  carriers: [
    { id: "summary.electricity", level: "carrier", label: "Electricity", value: 12 * factor, unit: "kWh site", carrier: "electricity", basis: "direct_zone_energy" },
    { id: "summary.gas", level: "carrier", label: "Natural gas", value: 3 * factor, unit: "kWh site", carrier: "natural_gas", basis: "direct_zone_energy" },
  ],
  completeness: partial ? partialCompleteness() : {
    status: "complete", mappedPercent: 100,
    energyUse: { level: "energy", status: "complete", found: 6, total: 6 },
  },
});

try {
  const module = await import("/src/js/views/energy-path-view.js");
  const i18n = await import("/src/js/i18n.js");
  const annual = zoneGraph();
  const january = zoneGraph("M1", 0.1);
  const zoneResult = {
    scope: { kind: "zone", zoneName: "Office", aggregationBasis: "zone_total" },
    summary: zoneSummary(), completeness: partialCompleteness(),
    nodes: annual.nodes, links: annual.links,
    reconciliation: [{ id: "zone-carrier-subtotal", level: "carrier", period: "annual", label: "Observed zone carrier subtotal", status: "partial", expectedValue: 15, explainedValue: 15, residualValue: 0, unit: "kWh", basis: "direct_zone_energy" }],
    warnings: [{ severity: "warning", code: "zone_direct_energy_partial_coverage", message: "Observed direct-use subtotal is not a complete zone carrier total." }],
    periods: [{
      id: "M1", kind: "monthly", summary: zoneSummary("M1", 0.1), nodes: january.nodes, links: january.links,
      reconciliation: [{ id: "zone-carrier-subtotal-m1", level: "carrier", period: "M1", label: "Observed zone carrier subtotal", status: "partial", expectedValue: 1.5, explainedValue: 1.5, residualValue: 0, unit: "kWh", basis: "direct_zone_energy" }],
      warnings: [{ severity: "warning", code: "zone_direct_energy_partial_coverage", message: "Monthly observed direct-use subtotal." }],
    }],
  };
  const explanation = {
    schema: module.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    availableZones: ["Office"], zoneResults: [zoneResult], nodes: [], links: [],
    sources: [
      { id: "variable.zone.lights", sourceType: "sql_variable", name: "Zone Lights Electricity Energy", keyValue: "Office", zoneName: "Office" },
      { id: "variable.zone.equipment.electric", sourceType: "sql_variable", name: "Zone Electric Equipment Electricity Energy", keyValue: "Office", zoneName: "Office" },
      { id: "variable.zone.equipment.gas", sourceType: "sql_variable", name: "Zone Gas Equipment NaturalGas Energy", keyValue: "Office", zoneName: "Office" },
      { id: "meter.building.lights", sourceType: "sql_meter", isMeter: true, name: "InteriorLights:Electricity", keyValue: "InteriorLights:Electricity" },
      { id: "meter.building.equipment", sourceType: "sql_meter", isMeter: true, name: "InteriorEquipment:Electricity", keyValue: "InteriorEquipment:Electricity" },
    ],
  };
  const state = {
    simulationEnergyScopeKind: "zone", simulationEnergyZoneName: "Office",
    simulationEnergyPeriod: "annual", simulationEnergyService: "all", simulationEnergySelection: "",
  };

  const graph = module.energyPathGraphForState(explanation, state);
  const lighting = graph.nodes.find((node) => node.level === "end_use" && node.endUse === "lighting");
  const equipment = graph.nodes.find((node) => node.level === "end_use" && node.endUse === "equipment");
  assert(lighting?.value === 5 && equipment?.value === 10, "actual zone lighting/equipment values were not preserved exactly");
  assert(lighting.basis === "direct_zone_energy" && equipment.basis === "direct_zone_energy", "direct zone node basis was not preserved");
  assert(!graph.nodes.some((node) => node.label?.includes("Building") || Number(node.value) >= 100), "a Building meter fallback was presented as Zone energy");
  assert(!graph.links.some((link) => link.sourceIds?.some((id) => id.startsWith("meter.building"))), "a Building meter fallback link reached the Zone graph");
  assert(graph.links.filter((link) => link.relation === "direct_end_use_to_carrier").length === 3, "direct zone carrier branches were lost");
  assert(!module.energyPathZoneDirectUseNodeIsTrusted({
    level: "end_use", endUse: "lighting", basis: "direct_zone_energy", meterHierarchyLevel: "broad_end_use",
    zoneName: "Office", sourceIds: ["meter.building.lights"],
  }, explanation.sources, "Office"), "a contradictory Building meter provenance bypassed the Zone direct-use guard");

  for (const futureNode of [
    { level: "end_use", endUse: "cooling", basis: "service_path_allocation", meterHierarchyLevel: "broad_end_use" },
    { level: "end_use", endUse: "heating", basis: "service_path_allocation", meterHierarchyLevel: "broad_end_use" },
    { level: "end_use", endUse: "fans", basis: "service_path_allocation", meterHierarchyLevel: "broad_end_use" },
    { level: "end_use", endUse: "pumps", basis: "service_path_allocation", meterHierarchyLevel: "broad_end_use" },
  ]) {
    assert(module.energyPathZoneDirectUseNodeIsTrusted(futureNode, [], "Office"), "EPATH-094 filter hid future HVAC allocation: " + futureNode.endUse);
  }

  const summary = module.energyPathSummaryForState(explanation, {}, state);
  assert(summary.endUses.length === 2 && !summary.endUses.some((item) => item.label.includes("fallback")), "Building meter fallback leaked into the Zone summary");
  const coverage = module.energyPathZoneDirectCoverage(summary);
  assert(coverage.limited && coverage.status === "partial" && coverage.found === 0 && coverage.total === 0, "backend partial/unknown completeness signal was not consumed");

  const mount = document.getElementById("mount");
  mount.innerHTML = module.renderEnergyPathKPI(summary);
  const totalKPI = mount.querySelector('[data-energy-path-kpi="total_site_energy"]');
  assert(totalKPI?.dataset.energyPathValueScope === "observed_direct_use_subtotal", "Zone carrier subtotal was presented as a complete KPI");
  assert(totalKPI.textContent.includes("Known zone site energy") && totalKPI.textContent.includes("Known only"), "partial Zone KPI lacks an explicit qualifier");
  assert(!totalKPI.textContent.includes("Total site energy"), "partial Zone KPI still claims to be a total");
  assert(mount.querySelectorAll('[data-energy-path-kpi]').length === 4 && mount.querySelectorAll('[data-energy-path-kpi="coverage"] [data-energy-path-kpi-boundary]').length === 2 && !mount.querySelector('[data-energy-path-kpi="coverage"] [data-energy-path-kpi-boundary-value]'), "unknown Zone coverage must retain the fourth card with explicit nonnumeric boundary states");

  mount.innerHTML = module.renderEnergyPathSummaryOverview(summary);
  const carrierSummary = mount.querySelector('[data-energy-path-summary-group="carriers"]');
  assert(carrierSummary?.dataset.energyPathValueScope === "observed_direct_use_subtotal", "carrier summary was presented as complete");
  assert(carrierSummary.textContent.includes("Known energy sources") && carrierSummary.textContent.includes("Observed direct-use subtotal"), "carrier summary lacks partial-coverage context");

  mount.innerHTML = module.renderEnergyPathView(explanation, { ...state, simulationEnergySelection: lighting.id });
  const coverageNotice = mount.querySelector('[data-energy-path-zone-coverage-notice="partial"]');
  const carrierStage = mount.querySelector('[data-energy-path-stage="carrier"]');
  const inspector = mount.querySelector('[data-energy-path-inspector="' + lighting.id + '"]');
  assert(coverageNotice?.textContent.includes("not complete zone totals"), "Zone view lacks a prominent subtotal explanation");
  assert(carrierStage?.dataset.energyPathValueScope === "observed_direct_use_subtotal" && carrierStage.textContent.includes("Known energy sources"), "carrier stage was presented as a complete total");
  assert(carrierStage.querySelectorAll(".energy-path-partial-coverage-badge").length === 2, "carrier values lack known-only badges");
  const basisValue = inspector?.querySelector('[data-energy-path-inspector-value="basis"] dd')?.textContent || "";
  assert(basisValue.includes("Direct zone energy") && !basisValue.includes("direct_zone_energy"), "direct Zone basis is not explained without exposing a technical token");
  assert(!inspector.querySelector('[data-energy-path-detail-section="sources"], [data-energy-path-detail-section="entities"], [data-energy-path-source]'), "direct Zone inspector still renders removed Source data or Related model entities");
  const lightingSources = module.energyPathInspectorSources(explanation, lighting, state);
  assert(lightingSources.length === 1 && lightingSources[0].id === "variable.zone.lights", "direct Zone lighting lost its underlying source provenance");
  assert(!inspector?.textContent.includes("InteriorLights:Electricity"), "Building fallback meter appeared in the direct Zone inspector");
  assert(!mount.querySelector("[data-simulation-energy-allocation-policy]"), "EPATH-100 allocation-policy UI was introduced early");

  const completeSummary = zoneSummary("annual", 1, false);
  completeSummary.quality = { driverToLoadStatus: "complete", driverToLoadClosedPct: 100, endUseToCarrierStatus: "complete", endUseToCarrierClosedPct: 100 };
  mount.innerHTML = module.renderEnergyPathKPI(completeSummary);
  assert(mount.querySelector('[data-energy-path-kpi="total_site_energy"]')?.textContent.includes("Total site energy"), "complete Zone coverage was incorrectly labelled partial");
  assert(mount.querySelectorAll('[data-energy-path-kpi="coverage"] [data-energy-path-kpi-boundary-value="100"]').length === 2, "known complete Zone accounting boundaries were hidden");
  assert(!mount.querySelector('[data-energy-path-value-scope="observed_direct_use_subtotal"]'), "complete Zone coverage retained a partial marker");

  const buildingSummary = { ...completeSummary, scope: { kind: "building", aggregationBasis: "model_total" }, completeness: partialCompleteness() };
  mount.innerHTML = module.renderEnergyPathKPI(buildingSummary);
  assert(mount.querySelector('[data-energy-path-kpi="total_site_energy"]')?.textContent.includes("Total site energy"), "Zone-only qualifier leaked into Building scope");

  const januaryGraph = module.energyPathGraphForState(explanation, { ...state, simulationEnergyPeriod: "M1" });
  const januaryLighting = januaryGraph.nodes.find((node) => node.level === "end_use" && node.endUse === "lighting");
  assert(januaryLighting?.value === 0.5 && januaryLighting.basis === "direct_zone_energy", "monthly Zone direct-use value/basis was lost");

  i18n.setLanguage("ko");
  mount.innerHTML = module.renderEnergyPathKPI(summary) + module.renderEnergyPathView(explanation, { ...state, simulationEnergySelection: lighting.id });
  assert(mount.querySelector('[data-energy-path-kpi="total_site_energy"]')?.textContent.includes("확인된 Zone site energy"), "Korean partial KPI label is missing");
  assert(mount.querySelector('[data-energy-path-zone-coverage-notice="partial"]')?.textContent.includes("완전한 Zone 합계가 아닌 소계"), "Korean partial-coverage explanation is missing");
  assert(mount.querySelector('[data-energy-path-inspector-value="basis"] dd')?.textContent.includes("Zone 직접 에너지"), "Korean direct-zone basis label is missing");
  i18n.setLanguage("en");

  document.body.dataset.energyPathZoneDirectUseStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathZoneDirectUseStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
