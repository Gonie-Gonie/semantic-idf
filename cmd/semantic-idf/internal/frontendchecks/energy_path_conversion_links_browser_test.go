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

func TestEPATH092ConversionLinksBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser EPATH-092 conversion-link harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-conversion-links", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathConversionLinksHarnessHTML)
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
		server.URL+"/energy-path-conversion-links",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("EPATH-092 conversion-link browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("EPATH-092 conversion-link browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-conversion-links-status="passed"`) {
		t.Fatalf("EPATH-092 conversion-link frontend contract failed:\n%s", document)
	}
}

const energyPathConversionLinksHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>EPATH-092 conversion links</title></head>
<body data-energy-path-conversion-links-status="pending">
<div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };

try {
  const module = await import("/src/js/views/energy-path-view.js");
  const i18n = await import("/src/js/i18n.js");
  const coolingLoadID = "load.cooling.building";
  const heatingLoadID = "load.heating.building";
  const coolingEndUseID = "end_use.cooling.building";
  const heatingEndUseID = "end_use.heating.building";
  const electricityID = "carrier.electricity.building";
  const gasID = "carrier.natural_gas.building";
  const nodes = [
    { id: coolingLoadID, level: "load", kind: "load.cooling", label: "Cooling load", value: 100, unit: "kWh", scaleDomain: "thermal", serviceKind: "cooling", sourceIds: ["cooling-load", "dehumidification-detail"] },
    { id: heatingLoadID, level: "load", kind: "load.heating", label: "Heating load", value: 80, unit: "kWh", scaleDomain: "thermal", serviceKind: "heating", sourceIds: ["heating-load", "humidification-detail"] },
    { id: coolingEndUseID, level: "end_use", kind: "end_use.cooling", label: "Cooling", value: 25, unit: "kWh", scaleDomain: "site", serviceKind: "cooling", endUse: "cooling", sourceIds: ["meter.cooling"] },
    { id: heatingEndUseID, level: "end_use", kind: "end_use.heating", label: "Heating", value: 100, unit: "kWh", scaleDomain: "site", serviceKind: "heating", endUse: "heating", sourceIds: ["meter.heating"] },
    { id: "end_use.fans.building", level: "end_use", kind: "end_use.fans", label: "Fans", value: 6, unit: "kWh", scaleDomain: "site", endUse: "fans", sourceIds: ["meter.fans"] },
    { id: "end_use.pumps.building", level: "end_use", kind: "end_use.pumps", label: "Pumps", value: 4, unit: "kWh", scaleDomain: "site", endUse: "pumps", sourceIds: ["meter.pumps"] },
    { id: "end_use.heat_rejection.building", level: "end_use", kind: "end_use.heat_rejection", label: "Heat rejection", value: 3, unit: "kWh", scaleDomain: "site", endUse: "heat_rejection", sourceIds: ["meter.heat_rejection"] },
    { id: electricityID, level: "carrier", kind: "carrier.electricity", label: "Electricity", value: 38, unit: "kWh", scaleDomain: "site", carrier: "electricity" },
    { id: gasID, level: "carrier", kind: "carrier.natural_gas", label: "Natural gas", value: 100, unit: "kWh", scaleDomain: "site", carrier: "natural_gas" },
  ];
  const links = [
    { id: "conversion.cooling", fromId: coolingLoadID, toId: coolingEndUseID, relation: "load_to_end_use", basis: "derived_ratio", fromValue: 100, fromUnit: "kWh", toValue: 25, toUnit: "kWh", ratio: 4, ratioKind: "coefficient_of_performance", ratioLabel: "untrusted label", serviceKind: "cooling" },
    { id: "conversion.heating", fromId: heatingLoadID, toId: heatingEndUseID, relation: "load_to_end_use", basis: "derived_ratio", fromValue: 80, fromUnit: "kWh", toValue: 100, toUnit: "kWh", ratio: 0.8, ratioKind: "efficiency", ratioLabel: "untrusted label", serviceKind: "heating" },
    { id: "carrier.cooling", fromId: coolingEndUseID, toId: electricityID, relation: "end_use_to_carrier", basis: "reported_meter", fromValue: 25, fromUnit: "kWh", toValue: 25, toUnit: "kWh", serviceKind: "cooling" },
    { id: "carrier.heating", fromId: heatingEndUseID, toId: gasID, relation: "end_use_to_carrier", basis: "reported_meter", fromValue: 100, fromUnit: "kWh", toValue: 100, toUnit: "kWh", serviceKind: "heating" },
    { id: "aux.fans", fromId: "end_use.fans.building", toId: electricityID, relation: "direct_end_use_to_carrier", basis: "reported_meter", fromValue: 6, fromUnit: "kWh", toValue: 6, toUnit: "kWh", sourceIds: ["meter.fans"] },
    { id: "aux.pumps", fromId: "end_use.pumps.building", toId: electricityID, relation: "direct_end_use_to_carrier", basis: "reported_meter", fromValue: 4, fromUnit: "kWh", toValue: 4, toUnit: "kWh", sourceIds: ["meter.pumps"] },
    { id: "aux.heat_rejection", fromId: "end_use.heat_rejection.building", toId: electricityID, relation: "direct_end_use_to_carrier", basis: "reported_meter", fromValue: 3, fromUnit: "kWh", toValue: 3, toUnit: "kWh", sourceIds: ["meter.heat_rejection"] },
  ];
  const sources = [
    { id: "cooling-load", name: "Zone cooling load", sourceType: "sql_variable", driverCategory: "load.cooling", driverComponent: "load.delivered.total", inspectorSection: "Breakdown" },
    { id: "dehumidification-detail", name: "Zone dehumidification load", sourceType: "sql_variable", driverCategory: "load.cooling", driverComponent: "load.dehumidification", inspectorSection: "Breakdown", effectiveValue: 10 },
    { id: "heating-load", name: "Zone heating load", sourceType: "sql_variable", driverCategory: "load.heating", driverComponent: "load.delivered.total", inspectorSection: "Breakdown" },
    { id: "humidification-detail", name: "Zone humidification load", sourceType: "sql_variable", driverCategory: "load.heating", driverComponent: "load.humidification", inspectorSection: "Breakdown", effectiveValue: 8 },
  ];
  const explanation = {
    schema: module.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    nodes, links, sources,
  };
  const state = { simulationEnergyScopeKind: "building", simulationEnergyPeriod: "annual", simulationEnergyService: "all", simulationEnergySelection: "" };
  const graph = module.energyPathGraphForState(explanation, state);
  const conversions = module.energyPathConversionFlows(graph.nodes, graph.links);
  const auxiliaries = module.energyPathAuxiliaryFlows(graph.nodes, graph.links);

  assert(conversions.length === 2, "cooling/heating conversions are not the only main conversion links");
  assert(conversions[0].service === "cooling" && conversions[0].fromValue === 100 && conversions[0].toValue === 25 && conversions[0].ratio?.label === "COP" && conversions[0].ratio?.value === 4, "cooling conversion is unclear or includes auxiliary energy");
  assert(conversions[1].service === "heating" && conversions[1].fromValue === 80 && conversions[1].toValue === 100 && conversions[1].ratio?.label === "Efficiency" && conversions[1].ratio?.value === 0.8, "heating conversion is unclear");
  assert(!graph.links.some((link) => link.relation === "load_to_end_use" && ["fans_pumps", "hvac_auxiliaries"].includes(graph.nodes.find((node) => node.id === link.toId)?.endUse)), "an auxiliary end use was forced through a thermal load");
  assert(auxiliaries.length === 2, "direct fan/pump and heat-rejection branches did not form the lower auxiliary lane");
  assert(auxiliaries.find((flow) => flow.endUse === "fans_pumps")?.value === 10, "fan/pump auxiliary total is wrong");
  assert(auxiliaries.find((flow) => flow.endUse === "hvac_auxiliaries")?.value === 3, "heat-rejection auxiliary total is wrong");

  const ratioCases = [
    ["coefficient_of_performance", "cooling", 100, 25, 4, "COP"],
    ["efficiency", "heating", 80, 100, 0.8, "Efficiency"],
    ["load_to_fuel", "heating", 90, 100, 0.9, "Load / fuel"],
    ["load_to_site_energy", "heating", 90, 30, 3, "Load / site energy"],
    ["load_to_purchased_energy", "cooling", 60, 20, 3, "Load / purchased energy"],
  ];
  for (const [ratioKind, serviceKind, fromValue, toValue, ratio, label] of ratioCases) {
    const result = module.energyPathConversionRatio({ relation: "load_to_end_use", ratioKind, ratioLabel: "do not trust me", serviceKind, fromValue, fromUnit: "kWh", toValue, toUnit: "kWh", ratio });
    assert(result?.label === label && result?.value === ratio, ratioKind + " did not use its exact canonical label");
  }
  const invalidCases = [
    { ratioKind: "coefficient_of_performance", serviceKind: "cooling", fromValue: 100, toValue: 0, ratio: 4 },
    { ratioKind: "coefficient_of_performance", serviceKind: "cooling", fromValue: 100, toValue: 25, ratio: 0 },
    { ratioKind: "coefficient_of_performance", serviceKind: "cooling", fromValue: 100, toValue: 25, ratio: Number.NaN },
    { ratioKind: "coefficient_of_performance", serviceKind: "cooling", fromValue: 100, toValue: 25, ratio: 3 },
    { ratioKind: "efficiency", serviceKind: "cooling", fromValue: 80, toValue: 100, ratio: 0.8 },
    { ratioKind: "efficiency", serviceKind: "heating", fromValue: 110, toValue: 100, ratio: 1.1 },
    { ratioKind: "unknown", serviceKind: "heating", fromValue: 80, toValue: 100, ratio: 0.8 },
  ];
  for (const invalid of invalidCases) {
    assert(module.energyPathConversionRatio({ relation: "load_to_end_use", fromUnit: "kWh", toUnit: "kWh", ...invalid }) === null, "invalid or uninterpretable ratio label was exposed: " + JSON.stringify(invalid));
  }
  assert(module.energyPathConversionRatio({ relation: "load_to_end_use", ratioKind: "coefficient_of_performance", serviceKind: "cooling", fromValue: 160, fromUnit: "kWh thermal", toValue: 45, toUnit: "kWh site", ratio: 3.556 })?.label === "COP", "valid backend three-decimal ratio rounding was rejected");
  assert(module.energyPathConversionRatio({ relation: "load_to_end_use", ratioKind: "coefficient_of_performance", serviceKind: "cooling", fromValue: 100, fromUnit: "kWh thermal", toValue: 25, toUnit: "MJ site", ratio: 4 }) === null, "base-unit mismatch exposed a ratio label");
  assert(module.energyPathConversionRatio({ relation: "load_to_end_use", ratioKind: "coefficient_of_performance", serviceKind: "cooling", fromValue: 100, fromUnit: "widgets", toValue: 25, toUnit: "widgets", ratio: 4 }) === null, "unknown non-energy units exposed a ratio label");
  for (const [fromUnit, toUnit] of [
    ["TJ thermal", "TJ site"],
    ["GWh (thermal)", "GWh [site]"],
    ["MBtu_load", "MBtu_energy"],
    ["therm fuel", "therm delivered"],
    ["ton-hour thermal", "ton-hour site"],
  ]) {
    assert(module.energyPathConversionRatio({ relation: "load_to_end_use", ratioKind: "coefficient_of_performance", serviceKind: "cooling", fromValue: 100, fromUnit, toValue: 25, toUnit, ratio: 4 })?.label === "COP", "backend-supported energy unit was hidden: " + fromUnit + " / " + toUnit);
  }
  for (const unit of ["kWhWidgets", "widgets-kWh", "kWh/site", "kWh thermal extra"]) {
    assert(module.energyPathConversionRatio({ relation: "load_to_end_use", ratioKind: "coefficient_of_performance", serviceKind: "cooling", fromValue: 100, fromUnit: unit, toValue: 25, toUnit: unit, ratio: 4 }) === null, "non-exact energy-unit token was accepted: " + unit);
  }

  const mount = document.getElementById("mount");
  // Retain compatibility coverage for the standalone ratio renderer. It is
  // no longer appended to the default EPATH-142 graph canvas.
  mount.innerHTML = module.renderEnergyPathFlowLanes(graph.nodes, graph.links);
  const conversionLane = mount.querySelector("[data-energy-path-conversion-lane]");
  const auxiliaryLane = mount.querySelector("[data-energy-path-auxiliary-lane]");
  assert(conversionLane?.textContent.includes("Cooling load → Cooling equipment energy"), "cooling equipment conversion title is missing");
  assert(conversionLane?.textContent.includes("Heating load → Heating equipment energy"), "heating equipment conversion title is missing");
  assert(conversionLane?.textContent.includes("100 kWh thermal") && conversionLane?.textContent.includes("25 kWh site"), "thermal and site values are not distinguished across cooling conversion");
  assert(conversionLane?.querySelector('[data-energy-path-ratio-kind="coefficient_of_performance"]')?.dataset.energyPathRatioLabel === "COP", "COP metadata is not rendered");
  assert(conversionLane?.querySelector('[data-energy-path-ratio-kind="efficiency"]')?.dataset.energyPathRatioLabel === "Efficiency", "Efficiency metadata is not rendered");
  assert(auxiliaryLane?.querySelectorAll("[data-energy-path-auxiliary-link]").length === 2, "lower auxiliary lane does not show direct carrier branches");
  assert(!auxiliaryLane?.querySelector("[data-energy-path-ratio-kind]"), "auxiliary energy entered a conversion-ratio denominator");
  mount.innerHTML = module.renderEnergyPathView(explanation, state);
  assert(!mount.querySelector('[data-energy-path-conversion-lane], [data-energy-path-auxiliary-lane]'), "default canvas appended obsolete duplicate card lists");
  const mainEndUseStage = mount.querySelector('[data-energy-path-stage="end_use"]');
  for (const id of ["end_use.fans_pumps.building", "end_use.hvac_auxiliaries.building"]) {
    assert(mainEndUseStage?.querySelectorAll('[data-energy-path-lane="direct"][data-energy-explanation-node="' + id + '"]').length === 1, "auxiliary must occur once in the end-use column's direct lane: " + id);
    assert(!mainEndUseStage?.querySelector('[data-energy-path-lane="main"][data-energy-explanation-node="' + id + '"]'), "auxiliary was forced through the thermal conversion lane");
  }
  const fanPumpButton = mainEndUseStage?.querySelector('[data-energy-path-lane="direct"][data-energy-explanation-node="end_use.fans_pumps.building"]');
  assert(fanPumpButton?.getAttribute("aria-pressed") === "false", "lower-lane auxiliary is not an interactive selectable node");

  mount.innerHTML = module.renderEnergyPathView(explanation, { ...state, simulationEnergySelection: coolingLoadID });
  const coolingInspector = mount.querySelector('[data-energy-path-inspector="' + coolingLoadID + '"]');
  assert(coolingInspector && !coolingInspector.querySelector('[data-energy-path-detail-section="sources"], [data-energy-path-source], [data-energy-path-humidity-detail]'), "Cooling inspector retained Source data");
  const coolingSources = module.energyPathInspectorSources(explanation, graph.nodes.find((node) => node.id === coolingLoadID), state);
  assert(coolingSources.length === 2 && coolingSources.some((source) => source.id === "cooling-load") && coolingSources.some((source) => source.id === "dehumidification-detail"), "Cooling source provenance lost or broadened its source membership");
  const sourceDetailsMount = document.createElement("div");
  sourceDetailsMount.innerHTML = module.renderEnergyPathSourceDetails(coolingSources, state);
  assert(sourceDetailsMount.querySelector('[data-energy-path-humidity-detail="dehumidification"]')?.textContent.includes("Dehumidification detail"), "standalone Cooling sources lost the dehumidification detail label");
  assert(!sourceDetailsMount.querySelector('[data-energy-path-humidity-detail="humidification"]'), "humidification was attached to Cooling sources");
  mount.innerHTML = module.renderEnergyPathView(explanation, { ...state, simulationEnergySelection: heatingLoadID });
  const heatingInspector = mount.querySelector('[data-energy-path-inspector="' + heatingLoadID + '"]');
  assert(heatingInspector && !heatingInspector.querySelector('[data-energy-path-detail-section="sources"], [data-energy-path-source], [data-energy-path-humidity-detail]'), "Heating inspector retained Source data");
  const heatingSources = module.energyPathInspectorSources(explanation, graph.nodes.find((node) => node.id === heatingLoadID), state);
  assert(heatingSources.length === 2 && heatingSources.some((source) => source.id === "heating-load") && heatingSources.some((source) => source.id === "humidification-detail"), "Heating source provenance lost or broadened its source membership");
  sourceDetailsMount.innerHTML = module.renderEnergyPathSourceDetails(heatingSources, state);
  assert(sourceDetailsMount.querySelector('[data-energy-path-humidity-detail="humidification"]')?.textContent.includes("Humidification detail"), "standalone Heating sources lost the humidification detail label");
  assert(!sourceDetailsMount.querySelector('[data-energy-path-humidity-detail="dehumidification"]'), "dehumidification was attached to Heating sources");

  mount.innerHTML = module.renderEnergyPathView(explanation, { ...state, simulationEnergySelection: "end_use.fans_pumps.building" });
  assert(mount.querySelector('[data-energy-path-lane="direct"][data-energy-explanation-node="end_use.fans_pumps.building"]')?.getAttribute("aria-pressed") === "true", "lower-lane auxiliary selection state is not visible");
  assert(mount.querySelector('[data-energy-path-inspector="end_use.fans_pumps.building"]'), "lower-lane auxiliary selection cannot open its inspector");

  const invalidLink = { ...links[0], id: "invalid-zero-denominator", toValue: 0, ratio: 0 };
  mount.innerHTML = module.renderEnergyPathFlowLanes(graph.nodes, [invalidLink]);
  assert(!mount.querySelector("[data-energy-path-ratio-kind]"), "zero-denominator conversion rendered a ratio label");

  i18n.setLanguage("ko");
  mount.innerHTML = module.renderEnergyPathFlowLanes(graph.nodes, graph.links);
  assert(mount.querySelector("[data-energy-path-conversion-lane]")?.textContent.includes("냉방 부하 → 냉방 설비 에너지"), "Korean cooling conversion label is missing");
  assert(mount.querySelector("[data-energy-path-auxiliary-lane]")?.textContent.includes("보조 설비 에너지"), "Korean auxiliary-lane label is missing");
  mount.innerHTML = module.renderEnergyPathView(explanation, { ...state, simulationEnergySelection: coolingLoadID });
  assert(mount.querySelector('[data-energy-path-lane-band="direct"]')?.textContent.includes("직접·보조 에너지"), "Korean canvas direct / auxiliary label is missing");
  assert(!mount.querySelector('[data-energy-path-inspector] [data-energy-path-source]'), "Korean Cooling inspector retained Source data");
  sourceDetailsMount.innerHTML = module.renderEnergyPathSourceDetails(coolingSources, state);
  assert(sourceDetailsMount.querySelector('[data-energy-path-humidity-detail="dehumidification"]')?.textContent.includes("제습 상세"), "Korean standalone dehumidification detail label is missing");
  i18n.setLanguage("en");

  document.body.dataset.energyPathConversionLinksStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathConversionLinksStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
