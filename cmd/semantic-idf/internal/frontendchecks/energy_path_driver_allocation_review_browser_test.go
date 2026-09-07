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

func TestEnergyPathDriverAllocationReviewBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser Energy Path driver-allocation review harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-driver-allocation-review", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathDriverAllocationReviewHarnessHTML)
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
		server.URL+"/energy-path-driver-allocation-review",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("Energy Path driver-allocation browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("Energy Path driver-allocation browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-driver-allocation-review-status="passed"`) {
		t.Fatalf("Energy Path driver-allocation browser contract failed:\n%s", document)
	}
}

const energyPathDriverAllocationReviewHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Energy Path driver allocation review</title></head>
<body data-energy-path-driver-allocation-review-status="pending">
<div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const allocationExplanation = "Deterministic signed heat-balance share allocation; not a direct causal decomposition.";
const driver = (id, label, category, raw, effective, allocated, sourceIds, period = "annual") => ({
  id, level: "driver", kind: "driver." + category, label,
  value: allocated, rawValue: raw, effectiveValue: effective, allocatedValue: allocated,
  allocationApplied: true, allocationExplanation, displayValue: allocated,
  unit: "kWh", scaleDomain: "thermal", period, serviceKind: "cooling",
  driverCategory: category, thermalComponent: "sensible", basis: "heat_balance_share",
  aggregationBasis: "model_total", multiplier: raw ? effective / raw : 1, sourceIds,
});
const load = (id, value, period = "annual") => ({
  id, level: "load", kind: "load.zone_cooling", label: "Cooling load", value,
  rawValue: value, effectiveValue: value, allocatedValue: value, unit: "kWh",
  scaleDomain: "thermal", period, serviceKind: "cooling", driverCategory: "load.cooling",
  sourceIds: ["cooling-load"],
});
const link = (fromId, toId, value, period = "annual") => ({
  id: "link." + period + "." + fromId, fromId, toId, relation: "driver_to_load",
  basis: "heat_balance_share", explanation: allocationExplanation,
  fromValue: value, fromUnit: "kWh", toValue: value, toUnit: "kWh",
  period, serviceKind: "cooling",
});
const period = (id, peopleRaw, peopleEffective, peopleAllocated, storageAllocated, loadValue) => {
  const people = driver("driver.internal.people.cooling.building", "People", "internal.people", peopleRaw, peopleEffective, peopleAllocated, ["office-people", "lab-people"], id);
  const storage = driver("driver.balance.storage_other.cooling.building", "Other / storage", "balance.storage_other", 0, 0, storageAllocated, ["cooling-load"], id);
  const cooling = load("load.cooling.building", loadValue, id);
  return { id, kind: "monthly", nodes: [people, storage, cooling], links: [link(people.id, cooling.id, peopleAllocated, id), link(storage.id, cooling.id, storageAllocated, id)], warnings: [] };
};

try {
  const module = await import("/src/js/views/energy-path-view.js");
  const annualPeople = driver("driver.internal.people.cooling.building", "People", "internal.people", 30, 60, 12, ["office-people", "lab-people"]);
  const annualStorage = driver("driver.balance.storage_other.cooling.building", "Other / storage", "balance.storage_other", 0, 0, 3, ["cooling-load"]);
  const annualLoad = load("load.cooling.building", 15);
  const officePeople = driver("driver.internal.people.cooling.office", "People", "internal.people", 10, 20, 4, ["office-people"]);
  officePeople.zoneName = "Office";
  const officeStorage = driver("driver.balance.storage_other.cooling.office", "Other / storage", "balance.storage_other", 0, 0, 1, ["office-load"]);
  officeStorage.zoneName = "Office";
  const officeLoad = load("load.cooling.office", 5);
  officeLoad.zoneName = "Office";
  officeLoad.sourceIds = ["office-load"];
  const labPeople = driver("driver.internal.people.cooling.lab", "People", "internal.people", 20, 40, 8, ["lab-people"]);
  labPeople.zoneName = "Lab";
  const labLoad = load("load.cooling.lab", 8);
  labLoad.zoneName = "Lab";
  labLoad.sourceIds = ["lab-load"];

  const explanation = {
    schema: module.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    availableZones: ["Office", "Lab"],
    nodes: [annualPeople, annualStorage, annualLoad],
    links: [link(annualPeople.id, annualLoad.id, 12), link(annualStorage.id, annualLoad.id, 3)],
    periods: [period("M1", 10, 20, 4, 1, 5), period("M2", 20, 40, 8, 2, 10)],
    zoneResults: [
      { scope: { kind: "zone", zoneName: "Office", aggregationBasis: "model_total" }, nodes: [officePeople, officeStorage, officeLoad], links: [link(officePeople.id, officeLoad.id, 4), link(officeStorage.id, officeLoad.id, 1)], periods: [] },
      { scope: { kind: "zone", zoneName: "Lab", aggregationBasis: "model_total" }, nodes: [labPeople, labLoad], links: [link(labPeople.id, labLoad.id, 8)], periods: [] },
    ],
    sources: [
      { id: "office-people", name: "Office People pressure", zoneName: "Office", rawValue: 10, effectiveValue: 20, allocatedValue: 4, allocationApplied: true, allocationFactor: 0.2, effectiveMultiplier: 2, multiplierApplication: "requires_zone_multiplier", driverCategory: "internal.people", driverComponent: "people.sensible", driverRole: "main_flow", inspectorSection: "breakdown", explanation: allocationExplanation, formula: "actual cooling load * positive signed pressure / total positive signed pressure" },
      { id: "lab-people", name: "Lab People pressure", zoneName: "Lab", rawValue: 20, effectiveValue: 40, allocatedValue: 8, allocationApplied: true, allocationFactor: 0.2, effectiveMultiplier: 2, multiplierApplication: "requires_zone_multiplier", driverCategory: "internal.people", driverComponent: "people.sensible", driverRole: "main_flow", inspectorSection: "breakdown", explanation: allocationExplanation, formula: "actual cooling load * positive signed pressure / total positive signed pressure" },
      { id: "office-infiltration", name: "Office infiltration pressure", zoneName: "Office", rawValue: 99, effectiveValue: 99, allocatedValue: 99, allocationApplied: true, driverCategory: "air.infiltration", driverRole: "main_flow", inspectorSection: "breakdown" },
      { id: "office-load", name: "Office cooling load", zoneName: "Office", driverCategory: "load.cooling", driverRole: "main_flow", inspectorSection: "breakdown" },
      { id: "lab-load", name: "Lab cooling load", zoneName: "Lab", driverCategory: "load.cooling", driverRole: "main_flow", inspectorSection: "breakdown" },
      { id: "cooling-load", name: "Building cooling load", driverCategory: "load.cooling", driverRole: "main_flow", inspectorSection: "breakdown" },
    ],
  };
  const mount = document.getElementById("mount");
  const render = (state) => {
    mount.innerHTML = module.renderEnergyPathView(explanation, state);
    return mount;
  };
  const valueText = (inspector, key) => inspector.querySelector('[data-energy-path-inspector-value="' + key + '"] dd')?.textContent || "";
  const button = (root, id) => root.querySelector('[data-energy-explanation-node="' + id + '"]');

  const annualState = { simulationEnergyScopeKind: "building", simulationEnergyPeriod: "annual", simulationEnergyService: "cooling", simulationEnergySelection: annualPeople.id };
  let root = render(annualState);
  let selectedButton = button(root, annualPeople.id);
  assert(selectedButton && selectedButton.querySelector("strong").textContent.includes("12"), "annual main node did not use AllocatedValue 12");
  assert(!selectedButton.querySelector("strong").textContent.includes("60"), "annual main node displayed EffectiveValue as width");
  let inspector = root.querySelector('[data-energy-path-inspector="' + annualPeople.id + '"]');
  assert(valueText(inspector, "raw").includes("30"), "inspector raw value is not distinct");
  assert(valueText(inspector, "effective").includes("60"), "inspector effective value is not distinct");
  assert(valueText(inspector, "allocated").includes("12"), "inspector allocated value is not distinct");
  let basisNote = inspector.querySelector('[data-energy-path-allocation-explanation="heat_balance_share"]');
  assert(basisNote, "allocated driver inspector has no heat_balance_share explanation hook");
  assert(!basisNote.textContent.includes("heat_balance_share") && basisNote.textContent.toLowerCase().includes("heat-balance"), "allocation note must explain heat-balance allocation without exposing a technical basis token");
  const sourceBasis = inspector.querySelector('[data-energy-path-detail-section="sources"] [data-energy-path-source-metadata="basis"] dd');
  assert(sourceBasis?.textContent === "heat_balance_share" && !sourceBasis.closest("details").open, "exact heat_balance_share provenance is not retained inside collapsed Source data");
  assert(basisNote.textContent.toLowerCase().includes("not a direct causal"), "allocation note does not explain non-causal interpretation");

  root = render({ ...annualState, simulationEnergyPeriod: "M1", simulationEnergySelection: annualPeople.id });
  selectedButton = button(root, annualPeople.id);
  inspector = root.querySelector('[data-energy-path-inspector="' + annualPeople.id + '"]');
  assert(selectedButton.querySelector("strong").textContent.includes("4"), "M1 main node leaked annual allocated value");
  assert(valueText(inspector, "raw").includes("10") && valueText(inspector, "effective").includes("20") && valueText(inspector, "allocated").includes("4"), "M1 inspector leaked annual raw/effective/allocated metadata");

  root = render({ ...annualState, simulationEnergyPeriod: "M1", simulationEnergySelection: "driver.balance.storage_other.cooling.building" });
  selectedButton = button(root, "driver.balance.storage_other.cooling.building");
  inspector = root.querySelector('[data-energy-path-inspector="driver.balance.storage_other.cooling.building"]');
  assert(selectedButton && selectedButton.textContent.includes("Other / storage") && selectedButton.querySelector("strong").textContent.includes("1"), "zero-pressure Other/storage fallback is not visible with allocated value");
  assert(inspector.querySelector('[data-energy-path-allocation-explanation="heat_balance_share"]'), "Other/storage fallback lacks allocation explanation");

  const officeState = { simulationEnergyScopeKind: "zone", simulationEnergyZoneName: "Office", simulationEnergyPeriod: "annual", simulationEnergyService: "cooling", simulationEnergySelection: officePeople.id };
  root = render(officeState);
  inspector = root.querySelector('[data-energy-path-inspector="' + officePeople.id + '"]');
  assert(inspector && inspector.textContent.includes("Office People pressure"), "Office source provenance is not reachable");
  assert(!inspector.textContent.includes("Lab People pressure"), "Lab source leaked into Office driver inspector");
  assert(!inspector.textContent.includes("Office infiltration pressure"), "unrelated driver category leaked into People inspector");
  assert(button(root, officePeople.id) && !button(root, labPeople.id), "zone graph did not isolate Office from Lab");

  document.body.dataset.energyPathDriverAllocationReviewStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathDriverAllocationReviewStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
