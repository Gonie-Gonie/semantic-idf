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

func TestEPATH101AuxiliaryAllocationQualityBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser EPATH-101 auxiliary allocation harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-auxiliary-allocation", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathAuxiliaryAllocationHarnessHTML)
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
		server.URL+"/energy-path-auxiliary-allocation",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("EPATH-101 auxiliary allocation browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("EPATH-101 auxiliary allocation browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-auxiliary-allocation-status="passed"`) {
		t.Fatalf("EPATH-101 auxiliary allocation frontend contract failed:\n%s", document)
	}
}

const energyPathAuxiliaryAllocationHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>EPATH-101 auxiliary allocation</title></head>
<body data-energy-path-auxiliary-allocation-status="pending">
<div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const allocationRow = (id, expectedValue, directValue, allocatedValue, unassignedValue, period = "annual") => ({
  id: "reconcile.zone_auxiliary_allocation." + id + "." + period,
  level: "allocation", period, basis: "service_path_allocation", unit: "kWh site",
  expectedValue, directValue, allocatedValue, unassignedValue,
  explainedValue: directValue + allocatedValue,
  residualValue: unassignedValue,
});
const completeness = { status: "partial", energyUse: { level: "energy", status: "partial", found: 0, total: 0 } };
const nodes = [
  { id: "end_use.fans.office", level: "end_use", kind: "energy.fans", label: "Fans", value: 50, rawValue: 100, effectiveValue: 100, allocatedValue: 50, allocationApplied: true, allocationExplanation: "Allocated by related AirLoop supply-air volume share", unit: "kWh", scaleDomain: "site", period: "annual", zoneName: "Office", endUse: "fans", carrier: "electricity", basis: "service_path_allocation", sourceIds: ["meter.fans", "airflow.office"] },
  { id: "carrier.electricity.office", level: "carrier", kind: "carrier.electricity", label: "Electricity", value: 50, unit: "kWh", scaleDomain: "site", period: "annual", zoneName: "Office", carrier: "electricity", basis: "service_path_allocation" },
  { id: "unassigned_building_hvac_auxiliary_energy.office", level: "end_use", kind: "unassigned_building_hvac_auxiliary_energy", label: "Unassigned building HVAC auxiliary energy", value: 20, unit: "kWh", scaleDomain: "site", period: "annual", zoneName: "Office", endUse: "fans", basis: "residual" },
];
const links = [
  { id: "link.fans.carrier", fromId: nodes[0].id, toId: nodes[1].id, relation: "end_use_to_carrier", basis: "service_path_allocation", fromValue: 50, toValue: 50, unit: "kWh", fromUnit: "kWh", toUnit: "kWh", period: "annual", zoneName: "Office", sourceIds: ["meter.fans", "airflow.office"] },
  { id: "link.unassigned_building_hvac_auxiliary_energy.office", fromId: nodes[2].id, toId: nodes[1].id, relation: "residual", basis: "residual", fromValue: 20, toValue: 20, period: "annual", zoneName: "Office" },
];
const zoneResult = {
  scope: { kind: "zone", zoneName: "Office", aggregationBasis: "model_total" },
  completeness, nodes, links, warnings: [], periods: [],
};
const explanation = {
  schema: "semantic-idf.energy-explanation/v2",
  scope: { kind: "building", aggregationBasis: "model_total" },
  availableZones: ["Office"], zoneResults: [zoneResult], nodes: [], links: [], sources: [],
  reconciliation: [
    allocationRow("fans.electricity", 100, 30, 50, 20),
    allocationRow("pumps.electricity", 50, 20, 30, 0),
    { ...allocationRow("ignored.central_hvac", 500, 500, 0, 0), id: "reconcile.zone_hvac_allocation.cooling.electricity.annual" },
  ],
  periods: [{ id: "M1", kind: "monthly", reconciliation: [allocationRow("fans.electricity", 40, 10, 20, 10, "M1")] }],
};
const state = {
  simulationEnergyScopeKind: "zone", simulationEnergyZoneName: "Office",
  simulationEnergyPeriod: "annual", simulationEnergyService: "all",
  simulationEnergySelection: "end_use.fans_pumps.office",
};

try {
  const module = await import("/src/js/views/energy-path-view.js");
  const i18n = await import("/src/js/i18n.js");
  const mount = document.getElementById("mount");

  const quality = module.energyPathAuxiliaryAllocationQuality(explanation, state);
  assert(quality.available && quality.expectedValue === 150, "quality denominator did not use auxiliary expectedValue only");
  assert(quality.directValue === 50 && quality.allocatedValue === 80 && quality.unassignedValue === 20, "quality component totals are wrong");
  assert(Math.abs(quality.directRatio - 1 / 3) < 1e-9 && Math.abs(quality.allocatedRatio - 8 / 15) < 1e-9 && Math.abs(quality.unassignedRatio - 2 / 15) < 1e-9, "quality ratios do not use expectedValue");
  assert(!module.energyPathAuxiliaryAllocationQuality(explanation, { ...state, simulationEnergyScopeKind: "building" }).available, "Building scope rendered the Zone allocation quality widget");

  const monthly = module.energyPathAuxiliaryAllocationQuality(explanation, { ...state, simulationEnergyPeriod: "M1" });
  assert(monthly.expectedValue === 40 && monthly.directRatio === 0.25 && monthly.allocatedRatio === 0.5 && monthly.unassignedRatio === 0.25, "selected-period allocation quality is wrong");
  const overmapped = module.energyPathAuxiliaryAllocationQuality({ ...explanation, reconciliation: [allocationRow("fans.electricity", 10, 12, 0, 0)] }, state);
  assert(overmapped.directRatio === 1.2, "overmapped direct truth was normalized to 100 percent");

  const graph = module.energyPathGraphForState(explanation, state);
  assert(graph.nodes.some((node) => node.id === "end_use.fans_pumps.office" && node.value === 50), "allocated auxiliary end use was lost");
  assert(graph.links.some((link) => link.relation === "end_use_to_carrier" && link.basis === "service_path_allocation"), "allocated auxiliary carrier ribbon was lost");
  assert(!graph.nodes.some(module.isEnergyPathUnassignedBuildingHVACItem), "unassigned Building auxiliary energy leaked into Zone nodes");
  assert(!graph.links.some(module.isEnergyPathUnassignedBuildingHVACItem), "unassigned Building auxiliary energy leaked into Zone links");

  mount.innerHTML = module.renderEnergyPathView(explanation, state);
  const widget = mount.querySelector("[data-energy-path-auxiliary-allocation-quality]");
  assert(widget?.dataset.energyPathExpectedValue === "150", "Zone view omitted auxiliary allocation quality");
  assert(widget.querySelectorAll("[data-energy-path-auxiliary-allocation-ratio]").length === 3, "Zone quality must show direct, allocated, and unassigned ratios");
  assert(widget.querySelector('[data-energy-path-auxiliary-allocation-ratio="unassigned"]')?.textContent.includes("13.3%"), "unassigned ratio is not readable");
  assert(widget.textContent.includes("Building allocation used by this Zone view"), "Building-wide quality scope is ambiguous");
  assert(widget.textContent.includes("never added to the selected Zone"), "unassigned Building-only behavior is unexplained");
  assert(mount.querySelector('[data-energy-path-auxiliary-end-use="fans_pumps"]'), "allocated auxiliary lane did not accept end_use_to_carrier");
  assert(mount.querySelector('[data-energy-path-inspector-value="basis"] dd')?.textContent.includes("Allocated by related AirLoop supply-air volume share"), "airflow-priority inspector terminology is missing");
  assert(!mount.querySelector("[data-simulation-energy-auxiliary-allocation], [data-simulation-energy-airflow-allocation], [data-simulation-energy-allocation-policy]"), "automatic auxiliary allocation added a toolbar selector");
  assert(!mount.textContent.includes("Unassigned building HVAC auxiliary energy"), "unassigned Building auxiliary energy appeared as a selected Zone value");

  mount.innerHTML = module.renderEnergyPathWarnings([{ severity: "warning", code: "unassigned_building_hvac_auxiliary_energy", message: "Unassigned building HVAC auxiliary energy: 20 kWh remains." }]);
  assert(mount.querySelector('[data-energy-path-quality-detail="unassigned_building_hvac_auxiliary_energy"]')?.textContent.includes("Unassigned building HVAC auxiliary energy"), "Building auxiliary warning label is missing");
  i18n.setLanguage("ko");
  mount.innerHTML = module.renderEnergyPathAuxiliaryAllocationQuality(quality) + module.renderEnergyPathWarnings([{ severity: "warning", code: "unassigned_building_hvac_auxiliary_energy", message: "20 kWh remains." }]);
  assert(mount.textContent.includes("HVAC 보조 에너지 배분 비율") && mount.textContent.includes("미배정 건물 HVAC 보조 에너지"), "Korean auxiliary quality labels are missing");
  i18n.setLanguage("en");

  document.body.dataset.energyPathAuxiliaryAllocationStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathAuxiliaryAllocationStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
