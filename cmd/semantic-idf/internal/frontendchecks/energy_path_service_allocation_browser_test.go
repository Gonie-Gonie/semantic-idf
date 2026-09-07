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

func TestEPATH100ServicePathAllocationBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser EPATH-100 service allocation harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-service-allocation", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathServiceAllocationHarnessHTML)
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
		server.URL+"/energy-path-service-allocation",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("EPATH-100 service allocation browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("EPATH-100 service allocation browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-service-allocation-status="passed"`) {
		t.Fatalf("EPATH-100 service allocation frontend contract failed:\n%s", document)
	}
}

const energyPathServiceAllocationHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>EPATH-100 service-path allocation</title></head>
<body data-energy-path-service-allocation-status="pending">
<div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };

const partialCompleteness = {
  status: "partial", mappedPercent: 0,
  energyUse: { level: "energy", status: "partial", found: 0, total: 0, message: "Observed direct-use and allocated HVAC subtotal; full Zone carrier coverage is unknown" },
  deliveredLoad: { level: "load", status: "complete", found: 1, total: 1 },
  heatDrivers: { level: "heat", status: "complete", found: 1, total: 1 },
};
const nodes = [
  { id: "load.cooling.office", level: "load", kind: "load.cooling", label: "Office cooling load", value: 40, rawValue: 40, effectiveValue: 40, allocatedValue: 40, unit: "kWh", scaleDomain: "thermal", period: "annual", zoneName: "Office", serviceKind: "cooling" },
  { id: "end_use.cooling.office", level: "end_use", kind: "end_use.cooling", label: "Allocated cooling energy", value: 10, rawValue: 10, effectiveValue: 10, allocatedValue: 10, unit: "kWh", scaleDomain: "site", period: "annual", zoneName: "Office", serviceKind: "cooling", endUse: "cooling", basis: "service_path_allocation", allocationExplanation: "Allocated by the related HVAC service-path load share", sourceIds: ["meter.cooling"] },
  { id: "carrier.electricity.office", level: "carrier", kind: "carrier.electricity", label: "Electricity", value: 10, unit: "kWh", scaleDomain: "site", period: "annual", zoneName: "Office", carrier: "electricity", basis: "service_path_allocation" },
  { id: "residual.unassigned_building_hvac_energy.office", level: "end_use", kind: "unassigned_building_hvac_energy", label: "Unassigned building HVAC energy", value: 12, unit: "kWh", scaleDomain: "site", period: "annual", zoneName: "Office", endUse: "cooling", basis: "residual", sourceIds: ["meter.cooling"] },
];
const links = [
  { id: "link.cooling.allocation", fromId: nodes[0].id, toId: nodes[1].id, relation: "load_to_end_use", basis: "service_path_allocation", fromValue: 40, toValue: 10, fromUnit: "kWh", toUnit: "kWh", ratio: 4, ratioKind: "coefficient_of_performance", period: "annual", zoneName: "Office", serviceKind: "cooling", sourceIds: ["meter.cooling"] },
  { id: "link.cooling.carrier", fromId: nodes[1].id, toId: nodes[2].id, relation: "end_use_to_carrier", basis: "service_path_allocation", fromValue: 10, toValue: 10, fromUnit: "kWh", toUnit: "kWh", period: "annual", zoneName: "Office", serviceKind: "cooling", sourceIds: ["meter.cooling"] },
  { id: "link.unassigned_building_hvac_energy.office", fromId: nodes[3].id, toId: nodes[2].id, relation: "residual", basis: "residual", fromValue: 12, toValue: 12, fromUnit: "kWh", toUnit: "kWh", period: "annual", zoneName: "Office", serviceKind: "cooling", sourceIds: ["meter.cooling"] },
];
const summary = {
  schema: "semantic-idf.energy-explanation-summary/v2", period: "annual",
  scope: { kind: "zone", zoneName: "Office", aggregationBasis: "model_total_contribution" },
  drivers: [], loads: [{ id: nodes[0].id, level: "load", label: nodes[0].label, value: 40, unit: "kWh thermal", serviceKind: "cooling" }],
  endUses: [
    { id: nodes[1].id, level: "end_use", label: nodes[1].label, value: 10, unit: "kWh site", endUse: "cooling", serviceKind: "cooling", basis: "service_path_allocation" },
    { id: nodes[3].id, level: "end_use", label: nodes[3].label, value: 12, unit: "kWh site", endUse: "cooling", serviceKind: "cooling", basis: "residual" },
  ],
  carriers: [
    { id: nodes[2].id, level: "carrier", label: "Electricity", value: 10, unit: "kWh site", carrier: "electricity", basis: "service_path_allocation" },
    { id: "carrier.unassigned_building_hvac_energy", level: "carrier", label: "Unassigned building HVAC energy", value: 12, unit: "kWh site", carrier: "electricity", basis: "residual" },
  ],
  ratios: [],
  residuals: [{ id: "unassigned_building_hvac_energy", level: "residual", label: "Unassigned building HVAC energy", value: 12, unit: "kWh site", basis: "residual" }],
  topZones: [], completeness: partialCompleteness,
};
const zoneResult = {
  scope: summary.scope, summary, completeness: partialCompleteness, nodes, links,
  warnings: [{ severity: "warning", code: "zone_direct_energy_partial_coverage", message: "Known Zone energy is a subtotal." }],
};
const explanation = {
  schema: "semantic-idf.energy-explanation/v2", scope: { kind: "building", aggregationBasis: "model_total" },
  availableZones: ["Office"], zoneResults: [zoneResult], nodes: [], links: [],
  sources: [{ id: "meter.cooling", sourceType: "sql_meter", isMeter: true, name: "Cooling:Electricity", keyValue: "Cooling:Electricity" }],
  warnings: [{ severity: "warning", code: "unassigned_building_hvac_energy", message: "Unassigned building HVAC energy: 12 kWh remains after service-path allocation." }],
};
const state = {
  simulationEnergyScopeKind: "zone", simulationEnergyZoneName: "Office",
  simulationEnergyPeriod: "annual", simulationEnergyService: "all", simulationEnergySelection: "",
};

try {
  const module = await import("/src/js/views/energy-path-view.js");
  const i18n = await import("/src/js/i18n.js");
  const mount = document.getElementById("mount");

  mount.innerHTML = module.renderEnergyPathWarnings(explanation.warnings);
  const quality = mount.querySelector('[data-energy-path-quality-detail="unassigned_building_hvac_energy"]');
  assert(quality?.textContent.includes("Unassigned building HVAC energy"), "quality detail omitted the unassigned Building HVAC label");
  assert(quality?.textContent.includes("12 kWh"), "quality detail omitted the unassigned Building HVAC amount/message");

  const graph = module.energyPathGraphForState(explanation, state);
  const allocated = graph.nodes.find((node) => node.id === "end_use.cooling.office");
  assert(allocated?.basis === "service_path_allocation" && allocated.value === 10, "service-path allocated Zone energy was lost");
  assert(!graph.nodes.some(module.isEnergyPathUnassignedBuildingHVACItem), "unassigned Building energy leaked into selected Zone nodes");
  assert(!graph.links.some(module.isEnergyPathUnassignedBuildingHVACItem), "unassigned Building energy leaked into selected Zone links");
  assert(graph.links.length === 2, "valid service-path allocation links were removed");

  const coolingGraph = module.energyPathGraphForState(explanation, { ...state, simulationEnergyService: "cooling" });
  assert(coolingGraph.nodes.some((node) => node.id === allocated.id) && coolingGraph.links.length === 2, "cooling service filter hid the allocated Zone path");
  assert(!coolingGraph.nodes.some(module.isEnergyPathUnassignedBuildingHVACItem), "service filter restored unassigned Building energy");

  const scopedSummary = module.energyPathSummaryForState(explanation, {}, state);
  assert(scopedSummary.endUses.length === 1 && scopedSummary.carriers.length === 1 && scopedSummary.residuals.length === 0, "unassigned Building energy leaked into selected Zone summary values");
  mount.innerHTML = module.renderEnergyPathKPI(scopedSummary) + module.renderEnergyPathSummaryOverview(scopedSummary);
  assert(mount.querySelector('[data-energy-path-kpi="total_site_energy"]')?.textContent.includes("Known zone site energy"), "EPATH-094 Known-only Zone KPI was not preserved");
  assert(mount.querySelectorAll('[data-energy-path-kpi]').length === 4 && mount.querySelectorAll('[data-energy-path-kpi="coverage"] [data-energy-path-kpi-boundary]').length === 2 && !mount.querySelector('[data-energy-path-kpi="coverage"] [data-energy-path-kpi-boundary-value]'), "unknown partial Zone coverage must remain explicit and nonnumeric in the fourth KPI card");
  assert(!mount.textContent.includes("Unassigned building HVAC energy"), "selected Zone summary displayed unassigned Building energy");

  mount.innerHTML = module.renderEnergyPathView(explanation, { ...state, simulationEnergySelection: allocated.id });
  const basis = mount.querySelector('[data-energy-path-inspector-value="basis"] dd')?.textContent || "";
  assert(basis.includes("Allocated by HVAC service-path load share") && !basis.includes("service_path_allocation"), "inspector did not explain the service-path allocation basis in plain language");
  const sourceDetails = mount.querySelector('[data-energy-path-detail-section="sources"]');
  assert(sourceDetails?.tagName === "DETAILS" && !sourceDetails.open && sourceDetails.querySelector('[data-energy-path-source-metadata="basis"] dd')?.textContent === "service_path_allocation", "exact allocation basis was not preserved inside collapsed Source data");
  assert(mount.querySelector('[data-energy-path-zone-coverage-notice="partial"]'), "EPATH-094 partial Zone notice was lost");
  assert(mount.querySelectorAll(".energy-path-partial-coverage-badge").length === 1, "EPATH-094 Known-only carrier marker was lost");
  assert(!mount.textContent.includes("Unassigned building HVAC energy"), "selected Zone graph displayed unassigned Building energy");
  assert(!mount.querySelector("[data-simulation-energy-allocation-policy]"), "an allocation-policy selector was added to the main UI");

  i18n.setLanguage("ko");
  mount.innerHTML = module.renderEnergyPathNodeInspector(explanation, graph.nodes, allocated.id, state, graph.relations) + module.renderEnergyPathWarnings(explanation.warnings);
  assert(mount.querySelector('[data-energy-path-inspector-value="basis"] dd')?.textContent.includes("HVAC 서비스 경로 부하 비율로 배분"), "Korean service-path allocation basis is missing");
  assert(mount.querySelector('[data-energy-path-quality-detail="unassigned_building_hvac_energy"]')?.textContent.includes("미배정 건물 HVAC 에너지"), "Korean unassigned Building HVAC quality label is missing");
  i18n.setLanguage("en");

  document.body.dataset.energyPathServiceAllocationStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathServiceAllocationStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
