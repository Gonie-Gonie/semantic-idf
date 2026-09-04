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

func TestEPATH091FixedEndUseTaxonomyBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser EPATH-091 fixed end-use taxonomy harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-end-use-taxonomy", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathEndUseTaxonomyHarnessHTML)
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
		server.URL+"/energy-path-end-use-taxonomy",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("EPATH-091 fixed end-use taxonomy browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("EPATH-091 fixed end-use taxonomy browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-end-use-taxonomy-status="passed"`) {
		t.Fatalf("EPATH-091 fixed end-use taxonomy frontend contract failed:\n%s", document)
	}
}

const energyPathEndUseTaxonomyHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>EPATH-091 fixed end-use taxonomy</title></head>
<body data-energy-path-end-use-taxonomy-status="pending">
<div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const electricityID = "carrier.electricity.building";
const gasID = "carrier.natural_gas.building";
const nodes = [
  { id: electricityID, level: "carrier", kind: "carrier.electricity", label: "Electricity", value: 200, unit: "kWh", scaleDomain: "site", carrier: "electricity", sourceIds: ["facility.electricity"] },
  { id: gasID, level: "carrier", kind: "carrier.natural_gas", label: "Natural gas", value: 200, unit: "kWh", scaleDomain: "site", carrier: "natural_gas", sourceIds: ["facility.gas"] },
];
const links = [];
const sources = [
  { id: "facility.electricity", name: "Electricity:Facility", keyValue: "Electricity:Facility", sourceType: "sql_meter", isMeter: true },
  { id: "facility.gas", name: "NaturalGas:Facility", keyValue: "NaturalGas:Facility", sourceType: "sql_meter", isMeter: true },
];
const rawEndUses = [];
const addEndUse = (token, value, carrier, meterName, suffix = token) => {
  const id = "end_use." + token + "." + suffix + ".building";
  const sourceID = "meter." + suffix;
  const carrierID = carrier === "natural_gas" ? gasID : electricityID;
  const node = {
    id, level: "end_use", kind: "end_use." + token,
    label: meterName, value, rawValue: value, effectiveValue: value, allocatedValue: value,
    unit: "kWh", scaleDomain: "site", period: "annual", endUse: token,
    sourceIds: [sourceID],
  };
  nodes.push(node);
  rawEndUses.push(node);
  links.push({
    id: "link." + suffix, fromId: id, toId: carrierID,
    relation: "end_use_to_carrier", basis: "reported_meter",
    fromValue: value, toValue: value, fromUnit: "kWh", toUnit: "kWh",
    period: "annual", sourceIds: [sourceID],
  });
  sources.push({ id: sourceID, name: meterName, keyValue: meterName, sourceType: "sql_meter", isMeter: true });
  return node;
};

addEndUse("cooling", 10, "electricity", "Cooling:Electricity");
addEndUse("heating", 20, "natural_gas", "Heating:NaturalGas");
addEndUse("fans", 3, "electricity", "Fans:Electricity");
addEndUse("pumps", 4, "electricity", "Pumps:Electricity");
addEndUse("heat_rejection", 1, "electricity", "HeatRejection:Electricity");
addEndUse("humidification", 2, "natural_gas", "Humidification:NaturalGas");
addEndUse("heat_recovery", 3, "electricity", "HeatRecovery:Electricity");
addEndUse("lighting", 5, "electricity", "InteriorLights:Electricity");
addEndUse("equipment", 6, "natural_gas", "InteriorEquipment:NaturalGas");
addEndUse("water_systems", 7, "natural_gas", "WaterSystems:NaturalGas");
addEndUse("refrigeration", 8, "electricity", "Refrigeration:Electricity");
addEndUse("other", 9, "natural_gas", "Other:NaturalGas");
for (let index = 1; index <= 10; index += 1) {
  addEndUse("custom_process_" + index, index, "electricity", "Custom Process " + index + ":Electricity", "custom_" + index);
}
addEndUse("zero_trap", 0, "electricity", "ZeroTrap:Electricity");

try {
  const module = await import("/src/js/views/energy-path-view.js");
  const adversarialNodeOrder = [nodes[0], nodes[1], ...nodes.slice(2).reverse()];
  const explanation = {
    schema: module.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    nodes: adversarialNodeOrder, links, sources,
    summary: {
      endUses: rawEndUses.map((node) => ({ id: node.id, label: node.label, value: node.value, endUse: node.endUse })),
    },
  };
  const rawNodesBefore = JSON.stringify(explanation.nodes);
  const rawLinksBefore = JSON.stringify(explanation.links);
  const rawSummaryBefore = JSON.stringify(explanation.summary);
  const baseState = {
    simulationEnergyScopeKind: "building", simulationEnergyPeriod: "annual",
    simulationEnergyService: "all", simulationEnergySelection: "",
  };
  const graph = module.energyPathGraphForState(explanation, baseState);
  const endUses = graph.nodes.filter((node) => node.level === "end_use");
  const carriers = graph.nodes.filter((node) => node.level === "carrier");
  const labels = endUses.map((node) => node.label);
  const expectedLabels = [
    "Cooling equipment", "Heating equipment", "Fans & pumps", "HVAC auxiliaries",
    "Lighting", "Equipment", "Water systems", "Refrigeration", "Other",
  ];

  assert(endUses.length === expectedLabels.length, "fixed taxonomy is unbounded or missing a visible category: " + labels.join(", "));
  assert(expectedLabels.every((label) => labels.includes(label)), "fixed end-use UI labels are incomplete: " + labels.join(", "));
  assert(carriers.length === 2 && carriers.some((node) => node.id === electricityID) && carriers.some((node) => node.id === gasID), "carrier stage was changed while projecting end uses");
  assert(!endUses.some((node) => node.label === "ZeroTrap:Electricity" || node.endUse === "zero_trap" || Number(node.value) === 0), "zero-value end use was not hidden");
  assert(!endUses.some((node) => String(node.endUse || "").startsWith("custom_process_")), "unknown end-use names escaped the bounded taxonomy");

  const auxiliaryTokens = new Set(["heat_rejection", "humidification", "heat_recovery"]);
  const zeroAuxiliaryIDs = new Set(explanation.nodes.filter((node) => auxiliaryTokens.has(node.endUse)).map((node) => node.id));
  const zeroAuxiliaryExplanation = {
    ...explanation,
    nodes: explanation.nodes.map((node) => zeroAuxiliaryIDs.has(node.id)
      ? { ...node, value: 0, rawValue: 0, effectiveValue: 0, allocatedValue: 0 }
      : node),
    links: explanation.links.map((link) => zeroAuxiliaryIDs.has(link.fromId)
      ? { ...link, fromValue: 0, toValue: 0 }
      : link),
  };
  const zeroAuxiliaryGraph = module.energyPathGraphForState(zeroAuxiliaryExplanation, baseState);
  assert(!zeroAuxiliaryGraph.nodes.some((node) => node.level === "end_use" && node.label === "HVAC auxiliaries"), "zero-total HVAC auxiliaries node was rendered");

  const byLabel = (label) => endUses.find((node) => node.label === label);
  const fansAndPumps = byLabel("Fans & pumps");
  assert(fansAndPumps?.value === 7, "Fans & pumps did not sum fan and pump values");
  assert(fansAndPumps.sourceIds.includes("meter.fans") && fansAndPumps.sourceIds.includes("meter.pumps") && fansAndPumps.sourceIds.length === 2, "Fans & pumps node lost or broadened source provenance");
  assert(!endUses.some((node) => node.endUse === "fans" || node.endUse === "pumps"), "raw fans/pumps remained separate in the primary UI graph");
  const fanPumpLinks = graph.links.filter((link) => link.relation === "end_use_to_carrier" && link.fromId === fansAndPumps.id);
  assert(fanPumpLinks.length === 1 && fanPumpLinks[0].toId === electricityID && fanPumpLinks[0].fromValue === 7 && fanPumpLinks[0].toValue === 7, "Fans & pumps carrier link values were not merged");
  assert(fanPumpLinks[0].sourceIds.includes("meter.fans") && fanPumpLinks[0].sourceIds.includes("meter.pumps") && fanPumpLinks[0].sourceIds.length === 2, "Fans & pumps carrier link provenance was not merged exactly");

  const auxiliaries = byLabel("HVAC auxiliaries");
  assert(auxiliaries?.value === 6, "nonzero HVAC auxiliaries were not grouped");
  for (const sourceID of ["meter.heat_rejection", "meter.humidification", "meter.heat_recovery"]) {
    assert(auxiliaries.sourceIds.includes(sourceID), "HVAC auxiliaries lost source " + sourceID);
  }
  const auxiliaryLinks = graph.links.filter((link) => link.relation === "end_use_to_carrier" && link.fromId === auxiliaries.id);
  assert(auxiliaryLinks.length === 2, "HVAC auxiliaries did not retain carrier-specific branches");
  assert(auxiliaryLinks.find((link) => link.toId === electricityID)?.fromValue === 4, "HVAC auxiliary electricity branch is wrong");
  assert(auxiliaryLinks.find((link) => link.toId === gasID)?.fromValue === 2, "HVAC auxiliary gas branch is wrong");

  const other = byLabel("Other");
  const unknownTotal = 55;
  assert(other?.value === 9 + unknownTotal, "unknown end uses were dropped instead of being preserved in Other");
  for (let index = 1; index <= 10; index += 1) {
    assert(other.sourceIds.includes("meter.custom_" + index), "Other node lost unknown source meter " + index);
  }
  const otherLinks = graph.links.filter((link) => link.relation === "end_use_to_carrier" && link.fromId === other.id);
  assert(otherLinks.length === 2, "Other did not preserve the unknown carrier branches");
  assert(otherLinks.find((link) => link.toId === electricityID)?.fromValue === unknownTotal, "unknown electricity contribution was lost from Other carrier link");
  assert(otherLinks.find((link) => link.toId === gasID)?.fromValue === 9, "known Other gas contribution was lost from carrier link");
  assert(graph.links.every((link) => graph.nodes.some((node) => node.id === link.fromId) && graph.nodes.some((node) => node.id === link.toId)), "end-use projection created a dangling carrier link");

  assert(JSON.stringify(explanation.nodes) === rawNodesBefore, "UI projection mutated raw end-use nodes");
  assert(JSON.stringify(explanation.links) === rawLinksBefore, "UI projection mutated raw end-use links");
  assert(JSON.stringify(explanation.summary) === rawSummaryBefore, "UI projection merged the raw fans/pumps summary");
  assert(explanation.summary.endUses.some((item) => item.endUse === "fans") && explanation.summary.endUses.some((item) => item.endUse === "pumps"), "raw summary no longer keeps fans and pumps separate");

  const heatingGraph = module.energyPathGraphForState(explanation, {
    ...baseState, simulationEnergyService: "heating",
  });
  const heatingEndUses = heatingGraph.nodes.filter((node) => node.level === "end_use");
  assert(heatingEndUses.length === 1 && heatingEndUses[0].label === "Heating equipment", "Heating-only filter contains a non-heating end use");
  assert(!heatingEndUses.some((node) => node.label === "HVAC auxiliaries"), "heat_rejection/heat_recovery token text incorrectly pulled neutral auxiliaries into Heating-only filter");
  assert(!heatingGraph.links.some((link) => link.fromId === auxiliaries.id), "Heating-only filter retained a neutral HVAC auxiliary carrier link");

  const mount = document.getElementById("mount");
  const selectedState = { ...baseState, simulationEnergySelection: fansAndPumps.id };
  mount.innerHTML = module.renderEnergyPathView(explanation, selectedState);
  const endUseStage = mount.querySelector('[data-energy-path-stage="end_use"]');
  const carrierStage = mount.querySelector('[data-energy-path-stage="carrier"]');
  assert(endUseStage && carrierStage && endUseStage !== carrierStage, "end-use and carrier stages are no longer semantically separate");
  assert(endUseStage.querySelectorAll("[data-energy-explanation-node]").length === expectedLabels.length, "rendered end-use stage is not bounded to the visible taxonomy");
  const renderedLabels = [...endUseStage.querySelectorAll("[data-energy-explanation-node]")]
    .map((button) => button.querySelector("span")?.textContent.trim() || "");
  assert(JSON.stringify(renderedLabels) === JSON.stringify(expectedLabels), "rendered end uses are not in canonical UI order: " + renderedLabels.join(", "));
  assert(!endUseStage.textContent.includes("ZeroTrap") && !endUseStage.textContent.includes("Custom Process"), "raw zero/unknown meter names leaked into primary node labels");
  const fanPumpButton = endUseStage.querySelector('[data-energy-explanation-node="' + fansAndPumps.id + '"]');
  assert(fanPumpButton?.getAttribute("aria-pressed") === "true" && fanPumpButton.textContent.includes("7"), "merged Fans & pumps node cannot remain selected");
  const inspector = mount.querySelector('[data-energy-path-inspector="' + fansAndPumps.id + '"]');
  assert(inspector?.querySelector('[data-energy-path-source="meter.fans"]')?.textContent.includes("Fans:Electricity"), "Fans original meter name is not inspectable");
  assert(inspector?.querySelector('[data-energy-path-source="meter.pumps"]')?.textContent.includes("Pumps:Electricity"), "Pumps original meter name is not inspectable");
  assert(!inspector?.querySelector('[data-energy-path-source="facility.electricity"]'), "merged end-use inspector leaked facility carrier provenance");

  mount.innerHTML = module.renderEnergyPathView(explanation, { ...baseState, simulationEnergySelection: other.id });
  const otherInspector = mount.querySelector('[data-energy-path-inspector="' + other.id + '"]');
  assert(otherInspector?.querySelector('[data-energy-path-source="meter.custom_10"]')?.textContent.includes("Custom Process 10:Electricity"), "unknown original meter name is not preserved in Other inspector");
  assert(otherInspector?.querySelector('[data-energy-path-source="meter.other"]')?.textContent.includes("Other:NaturalGas"), "known Other meter provenance was lost");

  document.body.dataset.energyPathEndUseTaxonomyStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathEndUseTaxonomyStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
