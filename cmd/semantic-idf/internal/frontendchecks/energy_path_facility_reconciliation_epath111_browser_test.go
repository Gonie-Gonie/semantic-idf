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

func TestEPATH111FacilityReconciliationAndSupplyBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser EPATH-111 facility reconciliation harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-facility-reconciliation", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathFacilityReconciliationHarnessHTML)
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
		server.URL+"/energy-path-facility-reconciliation",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("EPATH-111 facility reconciliation browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("EPATH-111 facility reconciliation browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-facility-reconciliation-status="passed"`) {
		t.Fatalf("EPATH-111 facility reconciliation frontend contract failed:\n%s", document)
	}
}

const energyPathFacilityReconciliationHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>EPATH-111 facility carrier reconciliation</title></head>
<body data-energy-path-facility-reconciliation-status="pending">
<div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const failures = [];
const check = (condition, message) => { if (!condition) failures.push(message); };
const state = {
  simulationEnergyScopeKind: "building",
  simulationEnergyPeriod: "annual",
  simulationEnergyService: "all",
  simulationEnergySelection: "carrier.electricity.building",
};
const carrier = {
  id: "carrier.electricity.building", level: "carrier", kind: "carrier.electricity",
  label: "Electricity", value: 100, unit: "kWh", scaleDomain: "site",
  carrier: "electricity", badges: ["carrier_residual"], sourceIds: ["facility"],
};
const cooling = {
  id: "end_use.cooling.building", level: "end_use", kind: "energy.cooling",
  label: "Cooling", value: 99.99, unit: "kWh", scaleDomain: "site", endUse: "cooling",
  sourceIds: ["cooling"],
};
const consumptionLink = {
  id: "cooling", fromId: cooling.id, toId: carrier.id,
  relation: "end_use_to_carrier", basis: "measured_meter",
  fromValue: 99.99, fromUnit: "kWh", toValue: 99.99, toUnit: "kWh", sourceIds: ["cooling"],
};
const supportDefinitions = [
  { kind: "purchased", token: "electricity_purchased", label: "Purchased electricity", value: 120 },
  { kind: "produced", token: "generators", label: "Onsite production", value: 25 },
  { kind: "sold", token: "electricity_sold", label: "Sold electricity", value: 15 },
  { kind: "storage", token: "storage_discharge", label: "Storage discharge", value: 8 },
];
const supportNode = (definition, value = definition.value) => ({
  id: "support." + definition.token + ".building", level: "support",
  kind: "energy." + definition.token, label: definition.label, endUse: definition.token,
  carrier: "electricity", value, displayValue: Math.abs(value), unit: "kWh", scaleDomain: "site",
  sourceIds: [definition.kind],
});
const supportLink = (definition, value = definition.value) => ({
  id: "support-link." + definition.kind,
  fromId: "support." + definition.token + ".building", toId: carrier.id,
  relation: "support_supply", basis: "measured_meter",
  fromValue: Math.abs(value), fromUnit: "kWh", toValue: Math.abs(value), toUnit: "kWh",
  sourceIds: [definition.kind],
});
const reconciliation = (expected, explained, residual, status) => ({
  id: "reconcile.energy.electricity.annual", level: "energy", period: "annual",
  label: "Electricity total basis", status, expectedValue: expected,
  explainedValue: explained, residualValue: residual, unit: "kWh", basis: "carrier_residual",
  formula: "facility carrier total - mapped carrier-qualified end-use meters",
});
const explanationFor = (definitions = [], options = {}) => {
  const mapped = Number.isFinite(options.mapped) ? options.mapped : 99.99;
  const residual = Number.isFinite(options.residual) ? options.residual : 0.01;
  const status = options.status || (residual < 0 ? "overmapped" : residual === 0 ? "balanced" : "residual");
  const localCarrier = { ...carrier, badges: residual === 0 ? [] : ["carrier_residual"] };
  const localCooling = { ...cooling, value: mapped };
  const nodes = [localCarrier, localCooling, ...definitions.map((item) => supportNode(item, item.value))];
  const links = [{ ...consumptionLink, fromValue: mapped, toValue: mapped }, ...definitions.map((item) => supportLink(item, item.value))];
  if (options.residualNode) {
    nodes.push({
      id: "residual.site_electricity.building", level: "residual", kind: "energy.residual",
      label: "Unclassified energy", carrier: "electricity",
      value: Math.abs(residual), unit: "kWh", scaleDomain: "site",
      basis: "residual", badges: ["unclassified_energy"],
    });
    links.push({
      id: "residual-link", fromId: "residual.site_electricity.building", toId: localCarrier.id,
      relation: "residual", basis: "residual", fromValue: Math.abs(residual), fromUnit: "kWh",
      toValue: Math.abs(residual), toUnit: "kWh",
    });
  }
  return {
    schema: "semantic-idf.energy-explanation/v2",
    scope: { kind: "building", aggregationBasis: "model_total" },
    nodes, links,
    reconciliation: [reconciliation(100, mapped, residual, status)],
    periods: [
      { id: "M1", kind: "monthly", reconciliation: [{ ...reconciliation(100, 90, 10, "residual"), id: "reconcile.energy.electricity.M1", period: "M1" }] },
      { id: "M2", kind: "monthly", reconciliation: [{ ...reconciliation(100, 110, -10, "overmapped"), id: "reconcile.energy.electricity.M2", period: "M2" }] },
    ],
    sources: [
      { id: "facility", sourceType: "sql_meter", isMeter: true, name: "Electricity:Facility", sourceUnit: "J", normalizedUnit: "kWh" },
      { id: "cooling", sourceType: "sql_meter", isMeter: true, name: "Cooling:Electricity", sourceUnit: "J", normalizedUnit: "kWh" },
      ...definitions.map((item) => ({ id: item.kind, sourceType: "sql_meter", isMeter: true, name: item.label, sourceUnit: "J", normalizedUnit: "kWh" })),
    ],
  };
};

try {
  const view = await import("/src/js/views/energy-path-view.js");

  const noSupply = explanationFor([]);
  check(view.energyPathSupplyActivities(noSupply.nodes, noSupply.links, carrier).length === 0, "no-support graph reported supply activity");
  check(view.renderEnergyPathSupportStrip(noSupply.nodes, noSupply.links) === "", "support strip rendered without supply activity");
  document.getElementById("mount").innerHTML = view.renderEnergyPathView(noSupply, { ...state });
  check(!document.querySelector("[data-energy-path-support-strip]"), "full view rendered an empty support strip");

  const purchasedOnly = explanationFor([supportDefinitions[0]]);
  const purchasedActivities = view.energyPathSupplyActivities(purchasedOnly.nodes, purchasedOnly.links, carrier);
  check(purchasedActivities.length === 1 && purchasedActivities[0].kind === "purchased" && purchasedActivities[0].value === 120, "purchased electricity was not retained as supply context");
  check(view.renderEnergyPathSupportStrip(purchasedOnly.nodes, purchasedOnly.links) === "", "purchase-only data incorrectly triggered the onsite/storage support strip");
  document.getElementById("mount").innerHTML = view.renderEnergyPathView(purchasedOnly, { ...state });
  check(!document.querySelector("[data-energy-path-support-strip]"), "purchase-only full view rendered the onsite/storage support strip");
  check(document.querySelector('[data-energy-path-supply-breakdown] [data-energy-path-supply-kind="purchased"]'), "purchase-only supply context was not available in the carrier inspector");

  const soldOnly = explanationFor([supportDefinitions[2]]);
  check(view.renderEnergyPathSupportStrip(soldOnly.nodes, soldOnly.links) === "", "sold-only data incorrectly triggered the onsite/storage support strip");
  document.getElementById("mount").innerHTML = view.renderEnergyPathView(soldOnly, { ...state });
  check(!document.querySelector("[data-energy-path-support-strip]"), "sold-only full view rendered the onsite/storage support strip");
  check(document.querySelector('[data-energy-path-supply-breakdown] [data-energy-path-supply-kind="sold"]'), "sold-only supply context was not available in the carrier inspector");

  const zeroProduced = { ...supportDefinitions[1], value: 0 };
  const zeroActivity = explanationFor([supportDefinitions[0], zeroProduced]);
  check(view.renderEnergyPathSupportStrip(zeroActivity.nodes, zeroActivity.links) === "", "zero onsite production triggered the support strip");

  const allSupply = explanationFor(supportDefinitions);
  const activities = view.energyPathSupplyActivities(allSupply.nodes, allSupply.links, carrier);
  check(JSON.stringify(activities.map((item) => item.kind)) === JSON.stringify(["purchased", "produced", "sold", "storage"]), "supply categories/order are not purchased, produced, sold, storage");
  check(JSON.stringify(activities.map((item) => item.value)) === JSON.stringify([120, 25, 15, 8]), "supply activity values were netted or changed");
  check(activities.every((item) => item.carrier === "electricity"), "supply activity created a non-Electricity carrier identity");

  document.getElementById("mount").innerHTML = view.renderEnergyPathView(allSupply, { ...state });
  const carrierStage = document.querySelector('[data-energy-path-stage="carrier"]');
  check(carrierStage?.querySelectorAll("[data-energy-explanation-node]").length === 1, "main graph did not retain one Electricity carrier");
  check(carrierStage?.querySelector('[data-energy-explanation-node="carrier.electricity.building"]'), "single Electricity carrier is missing");
  check(!document.querySelector('[data-energy-path-stage="support"]'), "support activity became a fifth main graph stage");
  const supportStrip = document.querySelector("[data-energy-path-support-strip]");
  check(supportStrip?.querySelectorAll("[data-energy-path-support-kind]").length === 4, "conditional support strip did not show all available supply activities");
  for (const definition of supportDefinitions) {
    const stripItem = supportStrip?.querySelector('[data-energy-path-support-kind="' + definition.kind + '"]');
    check(stripItem && Number(stripItem.dataset.energyPathSupportValue) === definition.value, "support-strip value missing for " + definition.kind);
    const inspectorItem = document.querySelector('[data-energy-path-supply-breakdown] [data-energy-path-supply-kind="' + definition.kind + '"]');
    check(inspectorItem && Number(inspectorItem.dataset.energyPathSupplyValue) === definition.value, "Supply breakdown value missing for " + definition.kind);
  }
  check(document.querySelector('[data-energy-path-carrier-residual-badge="residual"][data-energy-path-quality-basis="carrier_residual"]'), "carrier node/inspector has no residual quality badge");
  const inspectorReconciliation = document.querySelector('[data-energy-path-carrier-reconciliation="electricity"]');
  check(inspectorReconciliation, "carrier inspector has no reconciliation section");
  check(inspectorReconciliation?.querySelector('[data-energy-path-carrier-reconciliation-term="expected"]')?.textContent.includes("100"), "carrier inspector lost facility total");
  check(inspectorReconciliation?.querySelector('[data-energy-path-carrier-reconciliation-term="explained"]')?.textContent.includes("99.99"), "carrier inspector lost mapped end-use sum");
  check(inspectorReconciliation?.querySelector('[data-energy-path-carrier-reconciliation-term="residual"]')?.textContent.includes("0.01"), "carrier inspector lost residual");
  check(!document.querySelector("[data-energy-path-unclassified-energy]"), "residual at the absolute threshold and below 2% was rendered as an Unclassified graph node");

  const aboveThreshold = explanationFor(supportDefinitions, { mapped: 97, residual: 3, residualNode: true });
  document.getElementById("mount").innerHTML = view.renderEnergyPathView(aboveThreshold, { ...state });
  const unclassified = document.querySelector('[data-energy-path-unclassified-energy="electricity"]');
  check(unclassified && unclassified.textContent.includes("Unclassified energy") && unclassified.textContent.includes("3"), "backend-qualified residual did not render as a clear Unclassified energy node");
  check(document.querySelector('[data-energy-path-stage="carrier"]')?.querySelectorAll("[data-energy-explanation-node]").length === 1, "Unclassified energy created another carrier");

  const overmapped = explanationFor(supportDefinitions, { mapped: 110, residual: -10, status: "overmapped" });
  document.getElementById("mount").innerHTML = view.renderEnergyPathView(overmapped, { ...state });
  check(!document.querySelector("[data-energy-path-unclassified-energy]"), "negative residual incorrectly rendered a positive Unclassified energy supply");
  check(document.querySelector('[data-energy-path-carrier-residual-badge="overmapped"]'), "overmapping lost its carrier quality badge");
  check(document.querySelector('[data-energy-path-carrier-reconciliation-term="residual"]')?.textContent.includes("-10"), "inspector did not preserve the signed overmapping residual");

  for (const [name, mutate] of [
    ["negative node", (node) => { node.value = -3; }],
    ["negative signed residual", (node) => { node.signedValue = -3; }],
    ["reversed link", (_, link) => { [link.fromId, link.toId] = [link.toId, link.fromId]; }],
    ["negative link", (_, link) => { link.fromValue = -3; link.toValue = -3; }],
    ["missing badge", (node) => { node.badges = []; }],
    ["unqualified node basis", (node) => { node.basis = "reported_meter"; }],
    ["unqualified link basis", (_, link) => { link.basis = "reported_meter"; }],
    ["thermal residual", (node) => { node.scaleDomain = "thermal"; node.unit = "kWh thermal"; }],
  ]) {
    const invalid = explanationFor(supportDefinitions, { mapped: 97, residual: 3, residualNode: true });
    mutate(invalid.nodes.find((node) => node.level === "residual"), invalid.links.find((link) => link.relation === "residual"));
    document.getElementById("mount").innerHTML = view.renderEnergyPathView(invalid, { ...state });
    check(!document.querySelector("[data-energy-path-unclassified-energy]"), name + " was admitted as Unclassified energy");
  }

  const chargeOnly = explanationFor([]);
  chargeOnly.nodes.find((node) => node.id === cooling.id).value -= 5;
  chargeOnly.links[0].fromValue -= 5;
  chargeOnly.links[0].toValue -= 5;
  const charge = {
    id: "end_use.storage_charge.building", level: "end_use", kind: "energy.storage_charge",
    label: "Storage charge", endUse: "storage_charge", carrier: "electricity", value: 5,
    unit: "kWh", scaleDomain: "site", basis: "reported_variable", sourceIds: ["charge"],
  };
  chargeOnly.nodes.push(charge);
  chargeOnly.links.push({
    id: "charge-consumption", fromId: charge.id, toId: carrier.id, relation: "direct_end_use_to_carrier",
    basis: "reported_variable", fromValue: 5, fromUnit: "kWh", toValue: 5, toUnit: "kWh", sourceIds: ["charge"],
  });
  const chargeGraph = view.energyPathGraphForState(chargeOnly, { ...state });
  check(chargeGraph.supplyActivities.some((item) => item.kind === "storage_charge" && item.value === 5), "charging-only data lost its distinct storage activity");
  check(Math.abs(chargeGraph.links.filter((link) => ["end_use_to_carrier", "direct_end_use_to_carrier"].includes(link.relation)).reduce((sum, link) => sum + link.toValue, 0) - 99.99) < 1e-8, "storage charge stopped being consumption or was double-counted");
  check(!chargeGraph.nodes.some((node) => node.level === "support" && node.endUse === "storage_charge"), "storage charge was duplicated into a support node");
  document.getElementById("mount").innerHTML = view.renderEnergyPathView(chargeOnly, { ...state });
  check(document.querySelector('[data-energy-path-support-strip] [data-energy-path-support-kind="storage_charge"]'), "charging-only storage did not trigger a support strip");
  check(document.querySelector('[data-energy-path-supply-breakdown] [data-energy-path-supply-kind="storage_charge"]'), "storage charge absent from carrier Supply breakdown");

  const canonicalCharge = structuredClone(chargeOnly);
  const canonicalOther = canonicalCharge.nodes.find((node) => node.id === charge.id);
  Object.assign(canonicalOther, { id: "end_use.other.building", kind: "end_use.other", endUse: "other", label: "Other" });
  canonicalCharge.links.find((link) => link.id === "charge-consumption").fromId = canonicalOther.id;
  canonicalCharge.nodes.push({ ...charge, id: "support.storage_charge.building", level: "support" });
  const canonicalChargeGraph = view.energyPathGraphForState(canonicalCharge, { ...state });
  check(canonicalChargeGraph.supplyActivities.filter((item) => item.kind === "storage_charge").length === 1 && canonicalChargeGraph.supplyActivities.find((item) => item.kind === "storage_charge")?.value === 5, "canonical Other consumption lost exact isolated storage-charge context");
  check(Math.abs(canonicalChargeGraph.links.filter((link) => ["end_use_to_carrier", "direct_end_use_to_carrier"].includes(link.relation)).reduce((sum, link) => sum + link.toValue, 0) - 99.99) < 1e-8, "canonical charge context inflated or reduced main consumption");
  document.getElementById("mount").innerHTML = view.renderEnergyPathView(canonicalCharge, { ...state });
  check(Number(document.querySelector('[data-energy-path-support-kind="storage_charge"]')?.dataset.energyPathSupportValue) === 5, "canonical storage context absent from support strip");
  const mixedCharge = structuredClone(chargeOnly);
  mixedCharge.nodes.push({ ...charge, id: "support.storage_charge.building", level: "support" });
  const mixedActivities = view.energyPathSupplyActivities(mixedCharge.nodes, mixedCharge.links, carrier);
  check(mixedActivities.filter((item) => item.kind === "storage_charge").length === 1 && mixedActivities.find((item) => item.kind === "storage_charge")?.value === 5, "authoritative support context and legacy charge end use were double-counted");

  const scoped = explanationFor(supportDefinitions);
  const january = explanationFor([{ ...supportDefinitions[1], value: 2 }]);
  const zoneAnnual = explanationFor([{ ...supportDefinitions[1], value: 7 }]);
  const zoneJanuary = explanationFor([{ ...supportDefinitions[1], value: 0.7 }]);
  scoped.periods[0] = { ...january, id: "M1", kind: "monthly", periods: [] };
  scoped.availableZones = ["Zone Alpha", "Zone Beta"];
  scoped.zoneResults = [
    { ...zoneAnnual, scope: { kind: "zone", zoneName: "Zone Alpha", aggregationBasis: "model_total" }, periods: [{ ...zoneJanuary, id: "M1", kind: "monthly", periods: [] }] },
    { ...explanationFor([]), scope: { kind: "zone", zoneName: "Zone Beta", aggregationBasis: "model_total" }, periods: [] },
  ];
  for (const [label, selectedState, expectedProduced] of [
    ["Building annual", { ...state }, 25],
    ["Building January", { ...state, simulationEnergyPeriod: "M1" }, 2],
    ["Zone annual", { ...state, simulationEnergyScopeKind: "zone", simulationEnergyZoneName: "Zone Alpha" }, 7],
    ["Zone January", { ...state, simulationEnergyScopeKind: "zone", simulationEnergyZoneName: "Zone Alpha", simulationEnergyPeriod: "M1" }, 0.7],
  ]) {
    document.getElementById("mount").innerHTML = view.renderEnergyPathView(scoped, selectedState);
    const item = document.querySelector('[data-energy-path-support-kind="produced"]');
    check(item && Number(item.dataset.energyPathSupportValue) === expectedProduced, label + " leaked a different scope/period's supply value");
  }
  document.getElementById("mount").innerHTML = view.renderEnergyPathView(scoped, { ...state, simulationEnergyScopeKind: "zone", simulationEnergyZoneName: "Zone Beta" });
  check(!document.querySelector("[data-energy-path-support-strip]"), "zone with no supply inherited Building/another zone's support strip");

  const annualQuality = view.energyPathCarrierReconciliation(allSupply, carrier, { ...state });
  const januaryQuality = view.energyPathCarrierReconciliation(allSupply, carrier, { ...state, simulationEnergyPeriod: "M1" });
  const februaryQuality = view.energyPathCarrierReconciliation(allSupply, carrier, { ...state, simulationEnergyPeriod: "M2" });
  check(annualQuality?.residualValue === 0.01 && annualQuality?.status === "residual", "annual reconciliation selection is wrong");
  check(januaryQuality?.residualValue === 10 && januaryQuality?.status === "residual", "positive monthly residual was lost");
  check(februaryQuality?.residualValue === -10 && februaryQuality?.status === "overmapped", "negative monthly residual was absolutized or lost");
  check(januaryQuality.residualValue + februaryQuality.residualValue === 0, "opposite monthly residuals did not retain signed cancellation semantics");

  if (failures.length) throw new Error(failures.join(" | "));
  document.body.dataset.energyPathFacilityReconciliationStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathFacilityReconciliationStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
