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

func TestEPATH110CarrierTaxonomyAndWaterBoundaryBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser EPATH-110 carrier taxonomy harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-carrier-taxonomy", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathCarrierTaxonomyHarnessHTML)
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
		server.URL+"/energy-path-carrier-taxonomy",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("EPATH-110 carrier taxonomy browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("EPATH-110 carrier taxonomy browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-carrier-taxonomy-status="passed"`) {
		t.Fatalf("EPATH-110 carrier taxonomy frontend contract failed:\n%s", document)
	}
}

const energyPathCarrierTaxonomyHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>EPATH-110 carrier taxonomy and water boundary</title></head>
<body data-energy-path-carrier-taxonomy-status="pending">
<div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const failures = [];
const check = (condition, message) => { if (!condition) failures.push(message); };
const state = {
  simulationEnergyScopeKind: "building",
  simulationEnergyPeriod: "annual",
  simulationEnergyService: "all",
  simulationEnergySelection: "",
};
const carrierCases = [
  { token: "electricity", label: "Electricity", className: "site" },
  { token: "natural_gas", label: "Natural gas", className: "fuel" },
  { token: "district_cooling", label: "District cooling", className: "purchased" },
  { token: "district_heating", label: "District heating", className: "purchased" },
  { token: "steam", label: "Steam", className: "purchased" },
  { token: "propane", label: "Propane", className: "fuel" },
  { token: "fuel_oil_1", label: "Fuel oil #1", className: "fuel" },
  { token: "fuel_oil_2", label: "Fuel oil #2", className: "fuel" },
  { token: "coal", label: "Coal", className: "fuel" },
  { token: "diesel", label: "Diesel", className: "fuel" },
  { token: "gasoline", label: "Gasoline", className: "fuel" },
  { token: "other_fuel_1", label: "Other fuel 1", className: "fuel" },
  { token: "other_fuel_2", label: "Other fuel 2", className: "fuel" },
  { token: "water", label: "Water", className: "converted_water" },
];

try {
  const view = await import("/src/js/views/energy-path-view.js");
  const summaryModule = await import("/src/js/energy-path-summary.js");

  const rawWaterExplanation = {
    schema: view.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    nodes: [
      { id: "carrier.electricity.building", level: "carrier", kind: "carrier.electricity", label: "Electricity", value: 10, unit: "kWh", scaleDomain: "site", carrier: "electricity", sourceIds: ["facility.electricity"] },
      { id: "carrier.natural_gas.building", level: "carrier", kind: "carrier.natural_gas", label: "Natural gas", value: 5, unit: "kWh", scaleDomain: "site", carrier: "natural_gas", sourceIds: ["facility.gas"] },
      { id: "carrier.water.building", level: "carrier", kind: "carrier.water", label: "Water total", value: 250, unit: "m3", scaleDomain: "site", carrier: "water", basis: "reported_meter", sourceIds: ["facility.water"] },
      { id: "end_use.cooling.electricity.building", level: "end_use", kind: "energy.cooling", label: "Cooling", value: 10, unit: "kWh", scaleDomain: "site", endUse: "cooling", sourceIds: ["cooling.electricity"] },
      { id: "end_use.water_systems.gas.building", level: "end_use", kind: "energy.water_systems", label: "Gas water systems", value: 5, unit: "kWh", scaleDomain: "site", endUse: "water_systems", sourceIds: ["water_systems.gas"] },
      { id: "end_use.water_systems.water.building", level: "end_use", kind: "energy.water_systems", label: "Water volume by end use", value: 250, unit: "m3", scaleDomain: "site", endUse: "water_systems", basis: "reported_meter", sourceIds: ["water_systems.water"] },
    ],
    links: [
      { id: "cooling-electricity", fromId: "end_use.cooling.electricity.building", toId: "carrier.electricity.building", relation: "end_use_to_carrier", basis: "reported_meter", fromValue: 10, toValue: 10, fromUnit: "kWh", toUnit: "kWh", sourceIds: ["cooling.electricity"] },
      { id: "water-systems-gas", fromId: "end_use.water_systems.gas.building", toId: "carrier.natural_gas.building", relation: "direct_end_use_to_carrier", basis: "reported_meter", fromValue: 5, toValue: 5, fromUnit: "kWh", toUnit: "kWh", sourceIds: ["water_systems.gas"] },
      { id: "water-systems-water", fromId: "end_use.water_systems.water.building", toId: "carrier.water.building", relation: "direct_end_use_to_carrier", basis: "reported_meter", fromValue: 250, toValue: 250, fromUnit: "m3", toUnit: "m3", sourceIds: ["water_systems.water"] },
    ],
    sources: [
      { id: "facility.electricity", sourceType: "sql_meter", isMeter: true, name: "Electricity:Facility", sourceUnit: "J", normalizedUnit: "kWh" },
      { id: "facility.gas", sourceType: "sql_meter", isMeter: true, name: "NaturalGas:Facility", sourceUnit: "J", normalizedUnit: "kWh" },
      { id: "cooling.electricity", sourceType: "sql_meter", isMeter: true, name: "Cooling:Electricity", sourceUnit: "J", normalizedUnit: "kWh" },
      { id: "water_systems.gas", sourceType: "sql_meter", isMeter: true, name: "WaterSystems:NaturalGas", sourceUnit: "J", normalizedUnit: "kWh" },
      { id: "facility.water", sourceType: "sql_meter", isMeter: true, name: "Water:Facility", units: "m3", sourceUnit: "m3", normalizedUnit: "m3", inspectorSection: "context" },
      { id: "water_systems.water", sourceType: "sql_meter", isMeter: true, name: "WaterSystems:Water", units: "m3", sourceUnit: "m3", normalizedUnit: "m3", inspectorSection: "context" },
    ],
  };
  rawWaterExplanation.summary = {
    schema: summaryModule.ENERGY_PATH_SUMMARY_SCHEMA_V2,
    period: "annual",
    scope: rawWaterExplanation.scope,
    carriers: [
      { id: "carrier.electricity.building", carrier: "electricity", label: "Electricity", value: 10, unit: "kWh" },
      { id: "carrier.natural_gas.building", carrier: "natural_gas", label: "Natural gas", value: 5, unit: "kWh" },
      { id: "carrier.water.building", carrier: "water", label: "Water total", value: 250, unit: "m3", basis: "reported_meter", sourceIds: ["facility.water"] },
    ],
    loads: [], endUses: [], drivers: [], ratios: [], residuals: [], topZones: [],
    completeness: { status: "complete", mappedPercent: 100 },
  };

  const rawNodesBefore = JSON.stringify(rawWaterExplanation.nodes);
  const rawLinksBefore = JSON.stringify(rawWaterExplanation.links);
  const rawSourcesBefore = JSON.stringify(rawWaterExplanation.sources);
  const rawGraph = view.energyPathGraphForState(rawWaterExplanation, { ...state });
  check(!rawGraph.nodes.some((node) => node.carrier === "water" || node.id === "carrier.water.building"), "raw m3 Water:Facility remained on the site-energy carrier stage");
  check(!rawGraph.links.some((link) => link.toId === "carrier.water.building" || (link.sourceIds || []).includes("water_systems.water")), "raw water volume remained on a site-energy link");
  const retainedWaterSystems = rawGraph.nodes.find((node) => node.id === "end_use.water_systems.building");
  check(retainedWaterSystems && retainedWaterSystems.value === 5 && retainedWaterSystems.unit === "kWh", "gas/electric Water systems energy was removed or mixed with water volume");
  const retainedWaterSystemsLinks = rawGraph.links.filter((link) => link.fromId === "end_use.water_systems.building");
  check(retainedWaterSystemsLinks.length === 1 && retainedWaterSystemsLinks[0].toId === "carrier.natural_gas.building" && retainedWaterSystemsLinks[0].fromValue === 5, "Water systems did not retain its exact natural-gas energy branch");
  check(rawWaterExplanation.sources.find((source) => source.id === "facility.water")?.inspectorSection === "context", "raw water source context was not retained");
  check(JSON.stringify(rawWaterExplanation.nodes) === rawNodesBefore && JSON.stringify(rawWaterExplanation.links) === rawLinksBefore && JSON.stringify(rawWaterExplanation.sources) === rawSourcesBefore, "water filtering mutated the source payload");

  const rawKPI = summaryModule.energyPathSummaryKPIValues(rawWaterExplanation.summary)
    .find((item) => item.id === "total_site_energy");
  check(rawKPI && rawKPI.value === 15 && rawKPI.unit === "kWh", "Total site energy included 250 m3 of raw water or chose its unit");
  const rawKPIHTML = view.renderEnergyPathKPI(rawWaterExplanation.summary);
  document.getElementById("mount").innerHTML = rawKPIHTML;
  const rawKPINode = document.querySelector('[data-energy-path-kpi="total_site_energy"]');
  check(rawKPINode && rawKPINode.textContent.includes("15") && !rawKPINode.textContent.includes("265") && !rawKPINode.textContent.includes("m3"), "rendered Total site energy mixed water volume with energy");

  const invalidVolumeExplanation = {
    schema: view.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    nodes: [
      { id: "carrier.electricity.building", level: "carrier", kind: "carrier.electricity", label: "Electricity", carrier: "electricity", value: 7, unit: "m3", scaleDomain: "site", sourceIds: ["invalid.electricity.volume"] },
      { id: "end_use.cooling.electricity.building", level: "end_use", kind: "energy.cooling", label: "Cooling", endUse: "cooling", value: 7, unit: "m3", scaleDomain: "site", sourceIds: ["invalid.electricity.volume"] },
    ],
    links: [
      { id: "invalid-volume-link", fromId: "end_use.cooling.electricity.building", toId: "carrier.electricity.building", relation: "end_use_to_carrier", basis: "reported_meter", fromValue: 7, toValue: 7, fromUnit: "m3", toUnit: "m3", sourceIds: ["invalid.electricity.volume"] },
    ],
    sources: [
      { id: "invalid.electricity.volume", sourceType: "sql_meter", isMeter: true, name: "Cooling:Electricity", sourceUnit: "m3", normalizedUnit: "m3" },
    ],
  };
  const invalidVolumeGraph = view.energyPathGraphForState(invalidVolumeExplanation, { ...state });
  check(!invalidVolumeGraph.nodes.some((node) => node.id === "carrier.electricity.building" || node.id === "end_use.cooling.building"), "recognized Electricity with m3 was silently relabeled as kWh");
  check(!invalidVolumeGraph.links.some((link) => link.id === "invalid-volume-link" || link.toId === "carrier.electricity.building"), "m3 Electricity branch survived site-energy validation");

  const orphanWaterExplanation = {
    schema: view.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    nodes: [
      { id: "end_use.water_systems.water.building", level: "end_use", kind: "energy.water_systems", label: "orphan water volume", endUse: "water_systems", value: 99, unit: "m3", scaleDomain: "site", basis: "reported_meter", sourceIds: ["orphan.water"] },
    ],
    links: [],
    sources: [
      { id: "orphan.water", sourceType: "sql_meter", isMeter: true, name: "WaterSystems:Water", sourceUnit: "m3", normalizedUnit: "m3", inspectorSection: "context" },
    ],
  };
  const orphanWaterGraph = view.energyPathGraphForState(orphanWaterExplanation, { ...state });
  check(!orphanWaterGraph.nodes.some((node) => node.id.includes("water_systems") || node.unit === "m3"), "unlinked m3 WaterSystems context rendered as a site-energy end use");
  check(!view.renderEnergyPathView(orphanWaterExplanation, { ...state }).includes("orphan water volume"), "unlinked m3 WaterSystems context leaked into rendered energy flow");

  const jouleExplanation = {
    schema: view.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    nodes: [
      { id: "carrier.electricity.building", level: "carrier", kind: "carrier.electricity", label: "Electricity", carrier: "electricity", value: 3600000, unit: "J", scaleDomain: "site", sourceIds: ["joule.electricity"] },
      { id: "end_use.cooling.electricity.building", level: "end_use", kind: "energy.cooling", label: "Cooling", endUse: "cooling", value: 3600000, unit: "J", scaleDomain: "site", sourceIds: ["joule.electricity"] },
    ],
    links: [
      { id: "joule-link", fromId: "end_use.cooling.electricity.building", toId: "carrier.electricity.building", relation: "end_use_to_carrier", basis: "reported_meter", fromValue: 3600000, toValue: 3600000, fromUnit: "J", toUnit: "J", sourceIds: ["joule.electricity"] },
    ],
    sources: [
      { id: "joule.electricity", sourceType: "sql_meter", isMeter: true, name: "Cooling:Electricity", sourceUnit: "J", normalizedUnit: "kWh" },
    ],
  };
  const jouleBefore = JSON.stringify(jouleExplanation);
  const jouleGraph = view.energyPathGraphForState(jouleExplanation, { ...state });
  const jouleCarrier = jouleGraph.nodes.find((node) => node.id === "carrier.electricity.building");
  const jouleEndUse = jouleGraph.nodes.find((node) => node.id === "end_use.cooling.building");
  const jouleLink = jouleGraph.links.find((link) => link.toId === "carrier.electricity.building");
  const jouleRejected = !jouleCarrier && !jouleEndUse && !jouleLink;
  const jouleConverted = jouleCarrier?.value === 1 && jouleCarrier?.unit === "kWh" && jouleEndUse?.value === 1 && jouleEndUse?.unit === "kWh" && jouleLink?.fromValue === 1 && jouleLink?.toValue === 1 && jouleLink?.fromUnit === "kWh" && jouleLink?.toUnit === "kWh";
  check(jouleRejected || jouleConverted, "3.6e6 J site-energy path was relabeled without coherent value conversion");
  check(JSON.stringify(jouleExplanation) === jouleBefore, "J-to-kWh frontend handling mutated the source payload");

  const taxonomyNodes = [];
  const taxonomyLinks = [];
  const taxonomySources = [];
  let expectedHeating = 0;
  carrierCases.forEach((carrier, index) => {
    const value = index + 1;
    const carrierID = "carrier." + carrier.token + ".building";
    const endUseID = "end_use.heating." + carrier.token + ".building";
    const sourceID = "meter.heating." + carrier.token;
    const inputUnit = index % 2 === 0 ? "KWH" : " kWh ";
    const convertedWater = carrier.token === "water";
    taxonomyNodes.push({
      id: carrierID, level: "carrier", kind: "carrier." + carrier.token,
      label: "legacy " + carrier.token + " total", value, unit: inputUnit,
      scaleDomain: "site", carrier: carrier.token,
      basis: convertedWater ? "derived_ratio" : "reported_meter",
      sourceIds: ["facility." + carrier.token],
    });
    taxonomyNodes.push({
      id: endUseID, level: "end_use", kind: "energy.heating",
      label: "legacy heating " + carrier.token, value, unit: inputUnit,
      scaleDomain: "site", endUse: "heating",
      basis: convertedWater ? "derived_ratio" : "reported_meter",
      sourceIds: [sourceID],
    });
    taxonomyLinks.push({
      id: "link." + carrier.token, fromId: endUseID, toId: carrierID,
      relation: "end_use_to_carrier", basis: convertedWater ? "derived_ratio" : "reported_meter",
      fromValue: value, toValue: value, fromUnit: inputUnit, toUnit: inputUnit,
      serviceKind: "heating", sourceIds: [sourceID],
    });
    taxonomySources.push({
      id: "facility." + carrier.token, sourceType: convertedWater ? "derived_formula" : "sql_meter", isMeter: true,
      name: carrier.token + ":Facility", sourceUnit: convertedWater ? "m3" : "J", normalizedUnit: "kWh",
      inspectorSection: convertedWater ? "context" : "breakdown",
      formula: convertedWater ? "volume * explicit site-energy conversion factor" : "",
    });
    taxonomySources.push({
      id: sourceID, sourceType: convertedWater ? "derived_formula" : "sql_meter", isMeter: true,
      name: "Heating:" + carrier.token, sourceUnit: convertedWater ? "m3" : "J", normalizedUnit: "kWh",
      inspectorSection: convertedWater ? "context" : "breakdown",
      formula: convertedWater ? "volume * explicit site-energy conversion factor" : "",
    });
    expectedHeating += value;
  });
  const taxonomyExplanation = {
    schema: view.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    nodes: taxonomyNodes, links: taxonomyLinks, sources: taxonomySources,
  };
  const taxonomyBefore = JSON.stringify(taxonomyExplanation);
  const taxonomyGraph = view.energyPathGraphForState(taxonomyExplanation, { ...state });
  const taxonomyCarriers = taxonomyGraph.nodes.filter((node) => node.level === "carrier");
  check(taxonomyCarriers.length === carrierCases.length, "fixed carrier graph does not contain exactly 14 canonical carriers after explicit water conversion");
  const heating = taxonomyGraph.nodes.find((node) => node.id === "end_use.heating.building");
  check(heating && heating.value === expectedHeating && heating.unit === "kWh" && !heating.carrier, "carrier-neutral Heating total/unit was not canonicalized");
  carrierCases.forEach((carrier, index) => {
    const value = index + 1;
    const carrierID = "carrier." + carrier.token + ".building";
    const node = taxonomyCarriers.find((candidate) => candidate.id === carrierID);
    check(node && node.carrier === carrier.token && node.label === carrier.label && node.unit === "kWh" && node.scaleDomain === "site", "noncanonical carrier presentation for " + carrier.token);
    const branch = taxonomyGraph.links.find((link) => link.fromId === "end_use.heating.building" && link.toId === carrierID);
    check(branch && branch.fromValue === value && branch.toValue === value && branch.fromUnit === "kWh" && branch.toUnit === "kWh", "Heating mapped to the wrong carrier/value/unit for " + carrier.token);
    check(branch && JSON.stringify(branch.sourceIds) === JSON.stringify(["meter.heating." + carrier.token]), "carrier branch provenance broadened for " + carrier.token);
  });
  check(!taxonomyGraph.nodes.some((node) => node.carrier === "fuel" || node.id.includes("carrier.fuel.")), "generic fuel entered the fixed carrier taxonomy");
  check(JSON.stringify(taxonomyExplanation) === taxonomyBefore, "carrier presentation canonicalization mutated the source payload");

  const invalidWaterConversion = JSON.parse(JSON.stringify(taxonomyExplanation));
  invalidWaterConversion.sources
    .filter((source) => source.id === "facility.water" || source.id === "meter.heating.water")
    .forEach((source) => { source.sourceUnit = "kg"; });
  const invalidWaterGraph = view.energyPathGraphForState(invalidWaterConversion, { ...state });
  check(!invalidWaterGraph.nodes.some((node) => node.carrier === "water"), "non-volumetric source was accepted as converted water energy");
  check(!invalidWaterGraph.links.some((link) => link.toId === "carrier.water.building"), "non-volumetric converted-water branch survived validation");

  const convertedSummary = {
    schema: summaryModule.ENERGY_PATH_SUMMARY_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" }, period: "annual",
    carriers: carrierCases.map((carrier, index) => ({
      id: "carrier." + carrier.token + ".building", carrier: carrier.token, label: carrier.label,
      value: index + 1, unit: "kWh", basis: carrier.token === "water" ? "derived_ratio" : "reported_meter",
      sourceIds: ["facility." + carrier.token],
    })),
    loads: [], endUses: [], drivers: [], ratios: [], residuals: [], topZones: [], completeness: {},
  };
  const convertedKPI = summaryModule.energyPathSummaryKPIValues(convertedSummary)
    .find((item) => item.id === "total_site_energy");
  check(convertedKPI && convertedKPI.value === 105 && convertedKPI.unit === "kWh", "explicitly converted water energy was dropped from Total site energy");

  const genericFuelExplanation = {
    schema: view.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    nodes: [
      { id: "end_use.other.building", level: "end_use", kind: "energy.other", label: "Other", endUse: "other", value: 3, unit: "kWh", scaleDomain: "site", sourceIds: ["mystery"] },
      { id: "carrier.fuel.building", level: "carrier", kind: "carrier.fuel", label: "Fuel", carrier: "fuel", value: 3, unit: "kWh", scaleDomain: "site", sourceIds: ["mystery"] },
    ],
    links: [{ id: "generic-fuel", fromId: "end_use.other.building", toId: "carrier.fuel.building", relation: "direct_end_use_to_carrier", basis: "reported_meter", fromValue: 3, toValue: 3, fromUnit: "kWh", toUnit: "kWh", sourceIds: ["mystery"] }],
    sources: [{ id: "mystery", name: "Mystery:Fuel", sourceType: "sql_meter", isMeter: true }],
  };
  const genericFuelGraph = view.energyPathGraphForState(genericFuelExplanation, { ...state });
  check(!genericFuelGraph.nodes.some((node) => node.carrier === "fuel" || node.id === "carrier.fuel.building"), "arbitrary generic Fuel was accepted as a canonical energy source");
  check(!genericFuelGraph.links.some((link) => link.toId === "carrier.fuel.building"), "link to arbitrary generic Fuel survived carrier validation");

  const conversionFlow = (carrier, ratioKind) => {
	const service = carrier === "district_cooling" ? "cooling" : "heating";
	const loadID = "load." + service + "." + carrier;
	const endUseID = "end_use." + service + "." + carrier;
	const carrierID = "carrier." + carrier;
	const nodes = [
	  { id: loadID, level: "load", kind: "load." + service, label: service + " load", serviceKind: service, value: 10, unit: "kWh thermal", scaleDomain: "thermal" },
	  { id: endUseID, level: "end_use", kind: "energy." + service, label: service, endUse: service, serviceKind: service, value: 5, unit: "kWh site", scaleDomain: "site" },
	  { id: carrierID, level: "carrier", kind: "carrier." + carrier, label: carrier, carrier, value: 5, unit: "kWh site", scaleDomain: "site" },
	];
	const links = [
	  { id: "conversion." + carrier, fromId: loadID, toId: endUseID, relation: "load_to_end_use", basis: "derived_ratio", fromValue: 10, toValue: 5, fromUnit: "kWh thermal", toUnit: "kWh site", ratio: 2, ratioKind, serviceKind: service },
	  { id: "carrier." + carrier, fromId: endUseID, toId: carrierID, relation: "end_use_to_carrier", basis: "reported_meter", fromValue: 5, toValue: 5, fromUnit: "kWh site", toUnit: "kWh site", serviceKind: service },
	];
    return view.energyPathConversionFlows(nodes, links)[0];
  };
  ["electricity", "district_cooling", "district_heating", "steam"].forEach((carrier) => {
    const malicious = conversionFlow(carrier, "load_to_fuel");
    check(malicious && malicious.ratio === null, carrier + " was rendered as generic fuel");
  });
  ["natural_gas", "propane", "fuel_oil_1", "fuel_oil_2", "coal", "diesel", "gasoline", "other_fuel_1", "other_fuel_2"].forEach((carrier) => {
    const fuel = conversionFlow(carrier, "load_to_fuel");
    check(fuel && fuel.ratio?.kind === "load_to_fuel", carrier + " lost its valid combustion-fuel ratio");
  });
  const electricitySiteFlow = conversionFlow("electricity", "load_to_site_energy");
  check(electricitySiteFlow?.ratio?.kind === "load_to_site_energy", "electricity did not retain Load / site energy classification");
  check(electricitySiteFlow?.fromUnit === "kWh thermal" && electricitySiteFlow?.toUnit === "kWh site", "existing thermal/site domain-qualified units were rewritten");
  ["district_cooling", "district_heating", "steam"].forEach((carrier) => {
    check(conversionFlow(carrier, "load_to_purchased_energy")?.ratio?.kind === "load_to_purchased_energy", carrier + " did not retain purchased-energy classification");
  });

  const mount = document.getElementById("mount");
  mount.innerHTML = view.renderEnergyPathView(taxonomyExplanation, { ...state });
  const carrierStage = mount.querySelector('[data-energy-path-stage="carrier"]');
  check(carrierStage && carrierStage.querySelectorAll("[data-energy-explanation-node]").length === 14, "rendered Energy Source stage is not the fixed converted-water taxonomy");
  carrierCases.forEach((carrier) => {
    const rendered = carrierStage?.querySelector('[data-energy-explanation-node="carrier.' + carrier.token + '.building"]');
    check(rendered && rendered.textContent.includes(carrier.label) && rendered.textContent.includes("kWh site"), "rendered carrier label/unit is not canonical for " + carrier.token);
  });
  check(!carrierStage?.textContent.includes("legacy "), "legacy carrier labels leaked into Energy Source nodes");

  if (failures.length) throw new Error(failures.join(" | "));
  document.body.dataset.energyPathCarrierTaxonomyStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathCarrierTaxonomyStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
