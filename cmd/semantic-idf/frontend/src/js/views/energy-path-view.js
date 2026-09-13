import { t } from "../i18n.js";
import { escapeHTML } from "../state.js";
import {
  energyPathGroupSmallNodes,
  energyPathWithOriginalMembers,
  mergeEnergyPathOriginalMembers,
} from "../energy-path-grouping.js";
export { energyPathGroupSmallNodes } from "../energy-path-grouping.js";
import { renderEnergyPathQualityLine, renderEnergyPathDataDetails } from "../energy-path-details.js";
export { energyPathQualityForState, renderEnergyPathQualityLine, renderEnergyPathDataDetails } from "../energy-path-details.js";
import {
  energyPathSummaryGroups,
  isEnergyPathSummaryV2,
} from "../energy-path-summary.js";
import { energyPathKPIItems } from "../energy-path-kpis.js";
import { energyPathLayout } from "../energy-path-layout.js";
import { energyPathRibbons } from "../energy-path-ribbons.js";
import { energyPathNodeAppearance, energyPathLinkAppearance } from "../energy-path-appearance.js";
import { energyPathCompareNodes, energyPathOrderLinks, energyPathFocus } from "../energy-path-focus.js";
import { energyPathInspectorModel } from "../energy-path-inspector.js";

export const ENERGY_PATH_SCHEMA_V2 = "semantic-idf.energy-explanation/v2";

export const ENERGY_PATH_CARRIER_PRESENTATION = Object.freeze({
  electricity: Object.freeze({ label: "Electricity", unit: "kWh", scaleDomain: "site" }),
  natural_gas: Object.freeze({ label: "Natural gas", unit: "kWh", scaleDomain: "site" }),
  district_cooling: Object.freeze({ label: "District cooling", unit: "kWh", scaleDomain: "site" }),
  district_heating: Object.freeze({ label: "District heating", unit: "kWh", scaleDomain: "site" }),
  steam: Object.freeze({ label: "Steam", unit: "kWh", scaleDomain: "site" }),
  propane: Object.freeze({ label: "Propane", unit: "kWh", scaleDomain: "site" }),
  fuel_oil_1: Object.freeze({ label: "Fuel oil #1", unit: "kWh", scaleDomain: "site" }),
  fuel_oil_2: Object.freeze({ label: "Fuel oil #2", unit: "kWh", scaleDomain: "site" }),
  coal: Object.freeze({ label: "Coal", unit: "kWh", scaleDomain: "site" }),
  diesel: Object.freeze({ label: "Diesel", unit: "kWh", scaleDomain: "site" }),
  gasoline: Object.freeze({ label: "Gasoline", unit: "kWh", scaleDomain: "site" }),
  other_fuel_1: Object.freeze({ label: "Other fuel 1", unit: "kWh", scaleDomain: "site" }),
  other_fuel_2: Object.freeze({ label: "Other fuel 2", unit: "kWh", scaleDomain: "site" }),
  water: Object.freeze({ label: "Water", unit: "m3", scaleDomain: "context" }),
});

const ENERGY_PATH_CARRIER_ALIASES = Object.freeze({
  electricity: "electricity",
  naturalgas: "natural_gas",
  gas: "natural_gas",
  districtcooling: "district_cooling",
  districtheating: "district_heating",
  districtheatingwater: "district_heating",
  steam: "steam",
  districtheatingsteam: "steam",
  propane: "propane",
  fueloil1: "fuel_oil_1",
  fueloilno1: "fuel_oil_1",
  fueloil2: "fuel_oil_2",
  fueloilno2: "fuel_oil_2",
  coal: "coal",
  diesel: "diesel",
  gasoline: "gasoline",
  otherfuel1: "other_fuel_1",
  otherfuel2: "other_fuel_2",
  water: "water",
});

const ENERGY_PATH_COMBUSTION_CARRIERS = new Set([
  "natural_gas", "propane", "fuel_oil_1", "fuel_oil_2", "coal", "diesel", "gasoline", "other_fuel_1", "other_fuel_2",
]);

const ENERGY_PATH_PURCHASED_DISTRICT_CARRIERS = new Set(["district_cooling", "district_heating", "steam"]);

export const ENERGY_PATH_STAGES = Object.freeze([
  Object.freeze({
    level: "driver",
    labelKey: "simulation.energyPathStageDrivers",
    label: "Load Drivers",
    descriptionKey: "simulation.energyPathStageDriversDescription",
    description: "Factors that add or remove heat in the zone air heat balance",
    scaleDomain: "thermal",
    unitLabel: "kWh thermal",
  }),
  Object.freeze({
    level: "load",
    labelKey: "simulation.energyPathStageLoads",
    label: "Thermal Load",
    descriptionKey: "simulation.energyPathStageLoadsDescription",
    description: "Heating and cooling delivered to or removed from zones",
    scaleDomain: "thermal",
    unitLabel: "kWh thermal",
  }),
  Object.freeze({
    level: "end_use",
    labelKey: "simulation.energyPathStageEndUses",
    label: "End-use Energy",
    descriptionKey: "simulation.energyPathStageEndUsesDescription",
    description: "HVAC equipment and direct end-use site energy",
    scaleDomain: "site",
    unitLabel: "kWh site",
  }),
  Object.freeze({
    level: "carrier",
    labelKey: "simulation.energyPathStageSources",
    label: "Energy Source",
    descriptionKey: "simulation.energyPathStageSourcesDescription",
    description: "Electricity, natural gas, district energy, steam, and other sources",
    scaleDomain: "site",
    unitLabel: "kWh site",
  }),
]);

export const ENERGY_PATH_PERIODS = Object.freeze([
  Object.freeze({ value: "annual", labelKey: "simulation.periodAnnual", label: "Annual" }),
  ...["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]
    .map((label, index) => Object.freeze({ value: `M${index + 1}`, labelKey: `simulation.month.${index + 1}`, label })),
]);

const ENERGY_PATH_SCOPES = Object.freeze([
  Object.freeze({ value: "building", labelKey: "simulation.energyPathScopeBuilding", label: "Building" }),
  Object.freeze({ value: "zone", labelKey: "simulation.energyPathScopeZone", label: "Zone" }),
]);

const ENERGY_PATH_SERVICES = Object.freeze([
  Object.freeze({ value: "all", labelKey: "common.all", label: "All" }),
  Object.freeze({ value: "cooling", labelKey: "simulation.cooling", label: "Cooling" }),
  Object.freeze({ value: "heating", labelKey: "simulation.heating", label: "Heating" }),
]);

const ENERGY_PATH_DRIVER_ORDER = Object.freeze({
  "surface.exterior_walls": 0,
  "surface.roofs": 1,
  "surface.ground_floors": 2,
  "surface.windows_doors": 3,
  "surface.interzone": 4,
  "air.infiltration": 5,
  "air.mechanical_ventilation": 6,
  "air.interzone": 7,
  "internal.people": 8,
  "internal.lighting": 9,
  "internal.equipment": 10,
  "interzone.transfer": 11,
  "internal.other": 12,
  "balance.storage_other": 12,
});

const ENERGY_PATH_END_USE_ORDER = Object.freeze({
  cooling: 0,
  heating: 1,
  fans_pumps: 2,
  hvac_auxiliaries: 3,
  lighting: 4,
  equipment: 5,
  water_systems: 6,
  refrigeration: 7,
  other: 8,
});

const ENERGY_PATH_END_USE_PRESENTATION = Object.freeze({
  cooling: Object.freeze({ key: "cooling", labelKey: "simulation.energyPathEndUseCoolingEquipment", label: "Cooling equipment" }),
  heating: Object.freeze({ key: "heating", labelKey: "simulation.energyPathEndUseHeatingEquipment", label: "Heating equipment" }),
  fans: Object.freeze({ key: "fans_pumps", labelKey: "simulation.energyPathEndUseFansPumps", label: "Fans & pumps" }),
  pumps: Object.freeze({ key: "fans_pumps", labelKey: "simulation.energyPathEndUseFansPumps", label: "Fans & pumps" }),
  heat_rejection: Object.freeze({ key: "hvac_auxiliaries", labelKey: "simulation.energyPathEndUseHVACAuxiliaries", label: "HVAC auxiliaries" }),
  humidification: Object.freeze({ key: "hvac_auxiliaries", labelKey: "simulation.energyPathEndUseHVACAuxiliaries", label: "HVAC auxiliaries" }),
  heat_recovery: Object.freeze({ key: "hvac_auxiliaries", labelKey: "simulation.energyPathEndUseHVACAuxiliaries", label: "HVAC auxiliaries" }),
  lighting: Object.freeze({ key: "lighting", labelKey: "simulation.energyPathEndUseLighting", label: "Lighting" }),
  equipment: Object.freeze({ key: "equipment", labelKey: "simulation.energyPathEndUseEquipment", label: "Equipment" }),
  water_systems: Object.freeze({ key: "water_systems", labelKey: "simulation.energyPathEndUseWaterSystems", label: "Water systems" }),
  refrigeration: Object.freeze({ key: "refrigeration", labelKey: "simulation.energyPathEndUseRefrigeration", label: "Refrigeration" }),
  other: Object.freeze({ key: "other", labelKey: "simulation.energyPathEndUseOther", label: "Other" }),
});

const ENERGY_PATH_CONVERSION_RATIOS = Object.freeze({
  coefficient_of_performance: Object.freeze({
    labelKey: "simulation.energyPathRatioCOP",
    label: "COP",
    services: Object.freeze(["cooling"]),
  }),
  efficiency: Object.freeze({
    labelKey: "simulation.energyPathRatioEfficiency",
    label: "Efficiency",
    services: Object.freeze(["heating"]),
  }),
  load_to_fuel: Object.freeze({
    labelKey: "simulation.energyPathRatioLoadFuel",
    label: "Load / fuel",
    services: Object.freeze(["heating"]),
  }),
  load_to_site_energy: Object.freeze({
    labelKey: "simulation.energyPathRatioLoadSiteEnergy",
    label: "Load / site energy",
    services: Object.freeze(["cooling", "heating"]),
  }),
  load_to_purchased_energy: Object.freeze({
    labelKey: "simulation.energyPathRatioLoadPurchasedEnergy",
    label: "Load / purchased energy",
    services: Object.freeze(["cooling", "heating"]),
  }),
});

const ENERGY_PATH_AUXILIARY_END_USES = Object.freeze(new Set(["fans_pumps", "hvac_auxiliaries"]));
const ENERGY_PATH_SUPPLY_PRESENTATION = Object.freeze({
  purchased: Object.freeze({
    labelKey: "simulation.energyPathSupplyPurchased",
    label: "Purchased electricity",
    order: 0,
  }),
  produced: Object.freeze({
    labelKey: "simulation.energyPathSupplyProduced",
    label: "Onsite production",
    order: 1,
  }),
  sold: Object.freeze({
    labelKey: "simulation.energyPathSupplySold",
    label: "Sold electricity",
    order: 2,
  }),
  storage: Object.freeze({
    labelKey: "simulation.energyPathSupplyStorage",
    label: "Storage discharge",
    order: 4,
  }),
  storage_charge: Object.freeze({
    labelKey: "simulation.energyPathSupplyStorageCharge",
    label: "Storage charge",
    order: 3,
  }),
});
const ENERGY_PATH_SUPPORT_STRIP_TRIGGER_KINDS = new Set(["produced", "storage", "storage_charge"]);
const ENERGY_PATH_UNASSIGNED_BUILDING_HVAC = "unassigned_building_hvac_energy";
const ENERGY_PATH_UNASSIGNED_BUILDING_HVAC_AUXILIARY = "unassigned_building_hvac_auxiliary_energy";
const ENERGY_PATH_AUXILIARY_ALLOCATION_RECONCILIATION_PREFIX = "reconcile.zone_auxiliary_allocation.";

export function isEnergyPathV2(explanation = {}) {
  return String(explanation?.schema || "").toLowerCase() === ENERGY_PATH_SCHEMA_V2;
}

export function energyPathZoneDirectCoverage(payload = {}) {
  const scope = payload.scope || {};
  if (energyPathToken(scope.kind) !== "zone") {
    return { limited: false, status: "", message: "", found: 0, total: 0 };
  }
  const completeness = payload.completeness || {};
  const energyUse = completeness.energyUse || (completeness.items || [])
    .find((item) => ["energy", "energy_use", "end_use"].includes(energyPathToken(item?.level))) || {};
  const status = energyPathToken(energyUse.status);
  const warningLimited = (payload.warnings || [])
    .some((warning) => energyPathToken(warning?.code) === "zone_direct_energy_partial_coverage");
  const reconciliationLimited = (payload.reconciliation || []).some((item) => (
    energyPathToken(item?.status) === "partial" && energyPathToken(item?.basis) === "direct_zone_energy"
  ));
  return {
    limited: status === "partial" || warningLimited || reconciliationLimited,
    status,
    message: String(energyUse.message || "").trim(),
    found: Math.max(0, Number(energyUse.found) || 0),
    total: Math.max(0, Number(energyUse.total) || 0),
  };
}

export function energyPathAuxiliaryAllocationQuality(explanation = {}, viewState = {}) {
  const empty = {
    available: false,
    period: viewState.simulationEnergyPeriod || "annual",
    expectedValue: 0,
    directValue: 0,
    allocatedValue: 0,
    unassignedValue: 0,
    directRatio: 0,
    allocatedRatio: 0,
    unassignedRatio: 0,
    unit: "",
  };
  if ((viewState.simulationEnergyScopeKind || "building") !== "zone") {
    return empty;
  }
  const periodID = viewState.simulationEnergyPeriod || "annual";
  const period = (explanation.periods || [])
    .find((item) => energyPathToken(item?.id) === energyPathToken(periodID));
  const reconciliation = energyPathToken(periodID) === "annual"
    ? (explanation.reconciliation || period?.reconciliation || [])
    : (period?.reconciliation || []);
  const rows = reconciliation.filter((row) => (
    energyPathToken(row?.level) === "allocation" &&
    energyPathToken(row?.id).startsWith(ENERGY_PATH_AUXILIARY_ALLOCATION_RECONCILIATION_PREFIX) &&
    (!row?.period || energyPathToken(row.period) === energyPathToken(periodID))
  ));
  const componentFields = ["directValue", "allocatedValue", "unassignedValue"];
  if (!rows.length || !rows.some((row) => componentFields.some((field) => Object.prototype.hasOwnProperty.call(row, field)))) {
    return empty;
  }
  const total = rows.reduce((quality, row) => {
    quality.expectedValue += Math.max(0, Number(row.expectedValue) || 0);
    quality.directValue += Math.max(0, Number(row.directValue) || 0);
    quality.allocatedValue += Math.max(0, Number(row.allocatedValue) || 0);
    quality.unassignedValue += Math.max(0, Number(row.unassignedValue) || 0);
    quality.unit ||= String(row.unit || "").trim();
    return quality;
  }, { ...empty, period: periodID });
  if (!(total.expectedValue > 0)) {
    return empty;
  }
  total.available = true;
  total.directRatio = total.directValue / total.expectedValue;
  total.allocatedRatio = total.allocatedValue / total.expectedValue;
  total.unassignedRatio = total.unassignedValue / total.expectedValue;
  return total;
}

export function renderEnergyPathAuxiliaryAllocationQuality(quality = {}) {
  if (!quality.available || !(Number(quality.expectedValue) > 0)) return "";
  const items = [
    ["direct", t("simulation.energyPathAllocationDirect", {}, "Direct"), Number(quality.directValue) || 0, Number(quality.directRatio) || 0],
    ["allocated", t("simulation.energyPathAllocationAllocated", {}, "Allocated"), Number(quality.allocatedValue) || 0, Number(quality.allocatedRatio) || 0],
    ["unassigned", t("simulation.energyPathAllocationUnassigned", {}, "Unassigned"), Number(quality.unassignedValue) || 0, Number(quality.unassignedRatio) || 0],
  ];
  const expected = Number(quality.expectedValue) || 0;
  const unit = String(quality.unit || "").trim();
  return `
    <section class="energy-path-auxiliary-allocation-quality" data-energy-path-auxiliary-allocation-quality data-energy-path-period="${escapeHTML(quality.period || "annual")}" data-energy-path-expected-value="${escapeHTML(String(expected))}" role="status">
      <header>
        <strong>${escapeHTML(t("simulation.energyPathAuxiliaryAllocationCoverage", {}, "HVAC auxiliary allocation coverage"))}</strong>
        <span>${escapeHTML(t("simulation.energyPathAuxiliaryAllocationScope", {}, "Building allocation used by this Zone view"))}</span>
      </header>
      <div class="energy-path-auxiliary-allocation-ratios">
        ${items.map(([kind, label, value, ratio]) => `
          <div data-energy-path-auxiliary-allocation-ratio="${kind}" data-energy-path-ratio-value="${escapeHTML(String(ratio))}" data-energy-path-allocation-value="${escapeHTML(String(value))}">
            <span>${escapeHTML(label)}</span>
            <strong>${escapeHTML(`${(ratio * 100).toLocaleString(undefined, { maximumFractionDigits: 1 })}%`)}</strong>
            <small>${escapeHTML(`${energyPathSummaryValueLabel(value, unit)} / ${energyPathSummaryValueLabel(expected, unit)}`)}</small>
          </div>`).join("")}
      </div>
      <p>${escapeHTML(t(
        "simulation.energyPathAuxiliaryAllocationContext",
        {},
        "These are Building-wide auxiliary allocation shares for the selected period. Unassigned energy remains quality context and is never added to the selected Zone.",
      ))}</p>
    </section>`;
}

export function energyPathHasPayload(explanation = {}) {
  return Boolean(explanation.quality || explanation.summary?.quality || (explanation.periods || []).some((period) => period.quality)) ||
    energyPathAllNodes(explanation).length > 0 ||
    energyPathAllLinks(explanation).length > 0 ||
    energyPathZoneResults(explanation).some((result) => (
      result.quality || result.summary?.quality || energyPathAllNodes(result).length > 0 || energyPathAllLinks(result).length > 0
    ));
}

export function normalizeEnergyPathViewState(viewState = {}, explanation = {}) {
  const rootIsZoneOnly = energyPathToken(explanation.scope?.kind) === "zone" && !energyPathZoneResults(explanation).length;
  viewState.simulationEnergyScopeKind = rootIsZoneOnly
    ? "zone"
    : ["building", "zone"].includes(viewState.simulationEnergyScopeKind)
      ? viewState.simulationEnergyScopeKind
      : "building";
  viewState.simulationEnergyPeriod = ENERGY_PATH_PERIODS.some((period) => period.value === viewState.simulationEnergyPeriod)
    ? viewState.simulationEnergyPeriod
    : "annual";

  const zones = energyPathZoneNames(explanation);
  const selectedZone = zones.find((zone) => energyPathToken(zone) === energyPathToken(viewState.simulationEnergyZoneName));
  viewState.simulationEnergyZoneName = selectedZone || zones[0] || "";
  if (viewState.simulationEnergyScopeKind === "zone" && !zones.length) {
    viewState.simulationEnergyScopeKind = "building";
  }

  const services = energyPathServiceOptions(explanation, viewState);
  viewState.simulationEnergyService = services.some((service) => service.value === viewState.simulationEnergyService)
    ? viewState.simulationEnergyService
    : "all";
  return viewState;
}

let energyPathSceneSequence = 0;
const energyPathMountedScenes = new WeakMap();

export function prepareEnergyPathScene(explanation = {}, viewState = {}, options = {}) {
  // A supplied projection is already bound to the caller's normalized context.
  // Re-normalizing here would project an all-service graph a second time.
  if (!options.graph) normalizeEnergyPathViewState(viewState, explanation);
  const graph = options.graph || energyPathGraphForState(explanation, viewState);
  const allServiceGraph = options.allServiceGraph || ((viewState.simulationEnergyService || "all") === "all"
    ? graph : energyPathGraphForState(explanation, { ...viewState, simulationEnergyService: "all" }));
  const allGraphNodes = graph.nodes;
  const zoneCoverage = energyPathZoneDirectCoverageForState(explanation, viewState);
  const auxiliaryAllocationQuality = energyPathAuxiliaryAllocationQuality(explanation, viewState);
  const mainNodes = energyPathMainStageNodes(allGraphNodes, graph.links, explanation, viewState).map((node) => {
    const carrierQuality = node.level === "carrier"
      ? energyPathCarrierReconciliation(explanation, node, viewState)
      : null;
    return {
      ...node,
      ...(zoneCoverage.limited && node.level === "carrier" ? { presentationCoverage: "partial" } : {}),
      ...(carrierQuality ? { carrierQuality } : {}),
    };
  });
  const visibleNodes = ENERGY_PATH_STAGES.flatMap((stage) => mainNodes
    .filter((node) => (node.presentationLevel || node.level) === stage.level &&
      (stage.level !== "driver" || Math.abs(Number(node.value) || 0) > 0))
    .sort((left, right) => compareEnergyPathStageNodes(stage, left, right)));
  const period = viewState.simulationEnergyPeriod || "annual";
  const layout = energyPathLayout(visibleNodes, energyPathOrderLinks(visibleNodes, graph.links), { width: 1000, height: 420 });
  const drawing = energyPathRibbons(layout, { period });
  return {
    token: String(++energyPathSceneSequence),
    explanation,
    context: {
      scopeKind: viewState.simulationEnergyScopeKind || "building",
      zoneName: viewState.simulationEnergyZoneName || "",
      period,
      service: viewState.simulationEnergyService || "all",
    },
    graph, allServiceGraph, allGraphNodes, visibleNodes, layout, drawing,
    controlOptions: {
      zones: energyPathZoneNames(explanation),
      services: energyPathServiceOptions(explanation, viewState, allServiceGraph),
    },
    ratioQuality: energyPathRatioQualityForState(explanation, viewState),
    zoneCoverage, auxiliaryAllocationQuality,
  };
}

function energyPathSceneSelection(scene, viewState) {
  const requestedSelection = String(viewState.simulationEnergySelection || "");
  const selectedID = scene.allGraphNodes.some((node) => node.id === requestedSelection)
    ? requestedSelection
    : scene.allGraphNodes.find((node) => (node.originalNodeIds || []).includes(requestedSelection))?.id || requestedSelection;
  const relatedNodeIDs = new Set(
    energyPathCorrespondenceCounterparts(scene.allGraphNodes, scene.graph.relations, selectedID)
      .map((node) => node.id),
  );
  const focus = energyPathFocus(scene.layout, scene.drawing, selectedID, relatedNodeIDs);
  return { selectedID, relatedNodeIDs, focus };
}

export function renderEnergyPathView(explanation = {}, viewState = {}, options = {}) {
  const scene = options.scene || prepareEnergyPathScene(explanation, viewState);
  const { selectedID } = energyPathSceneSelection(scene, viewState);
  return `
    <section class="energy-path-view" data-energy-path-scene="${scene.token}" data-energy-path-selection="${escapeHTML(selectedID)}" data-energy-path-schema="${escapeHTML(ENERGY_PATH_SCHEMA_V2)}" data-energy-path-zone-coverage="${scene.zoneCoverage.limited ? "partial" : "complete_or_unreported"}">
      ${renderEnergyPathHeader(explanation, viewState, { scene, fixedContext: options.fixedContext })}
      ${renderEnergyPathContextMetrics(explanation, viewState)}
      ${renderEnergyPathSupportStrip(scene.allGraphNodes, scene.graph.links, selectedID, scene.graph.supplyActivities)}
      ${renderEnergyPathGraph(scene, viewState)}
      ${renderEnergyPathQuality(explanation, viewState)}
      <div class="energy-path-render-slot" data-energy-path-inspector-slot>${renderEnergyPathInspector(scene, viewState, options)}</div>
      ${renderEnergyPathGraphLegend()}
      <div class="energy-path-render-slot" data-energy-path-details-slot>${renderEnergyPathDetails(scene, viewState, options)}</div>
    </section>`;
}

export function renderEnergyPathInspector(scene, viewState = {}, options = {}) {
  const { selectedID, focus } = energyPathSceneSelection(scene, viewState);
  const selectedLink = scene.drawing.ribbons.find((ribbon) => ribbon.id === focus.selectedLinkID);
  return selectedLink
    ? renderEnergyPathLinkInspector(scene.explanation, selectedLink, scene.visibleNodes, scene.graph.links, viewState, scene.ratioQuality, options)
    : renderEnergyPathNodeInspector(scene.explanation, scene.allGraphNodes, selectedID, viewState, scene.graph.relations, scene.graph.links, scene.graph.supplyActivities, options);
}

export function renderEnergyPathQuality(explanation = {}, viewState = {}) {
  return renderEnergyPathQualityLine(explanation, viewState);
}

export function renderEnergyPathDetails(scene, viewState = {}, options = {}) {
  return renderEnergyPathDataDetails(scene.explanation, viewState, {
    ...options,
    diagnosticsHTML: renderEnergyPathWarnings(scene.graph.warnings) +
      renderEnergyPathAuxiliaryAllocationQuality(scene.auxiliaryAllocationQuality) +
      renderEnergyPathZoneCoverageNotice(scene.zoneCoverage),
  });
}

function energyPathMountedScene(host, scene, viewState) {
  const root = host?.matches?.("[data-energy-path-scene]") ? host : host?.querySelector?.("[data-energy-path-scene]");
  const context = scene?.context;
  if (!root || root.dataset.energyPathScene !== scene?.token || !context ||
    context.scopeKind !== (viewState.simulationEnergyScopeKind || "building") ||
    context.zoneName !== (viewState.simulationEnergyZoneName || "") ||
    context.period !== (viewState.simulationEnergyPeriod || "annual") ||
    context.service !== (viewState.simulationEnergyService || "all")) return null;
  const previous = energyPathMountedScenes.get(root);
  if (previous?.scene === scene) return previous;
  const mounted = {
    root, scene,
    selectedID: root.dataset.energyPathSelection || "",
    inspector: root.querySelector("[data-energy-path-inspector-slot]"),
    details: root.querySelector("[data-energy-path-details-slot]"),
    nodes: [...root.querySelectorAll("[data-energy-path-layout-node]")],
    ribbons: [...root.querySelectorAll("[data-energy-path-ribbon]")],
    bars: [...root.querySelectorAll("[data-energy-path-bar]")],
    ratios: [...root.querySelectorAll("[data-energy-path-bridge-ratio]")],
    hits: [...root.querySelectorAll("[data-energy-path-link-hit]")],
    supports: [...root.querySelectorAll("[data-energy-path-support-kind]")],
  };
  if (!mounted.inspector || !mounted.details) return null;
  energyPathMountedScenes.set(root, mounted);
  return mounted;
}

export function updateEnergyPathSelection(host, scene, viewState = {}, options = {}) {
  const mounted = energyPathMountedScene(host, scene, viewState);
  if (!mounted) return false;
  const { selectedID, relatedNodeIDs, focus } = energyPathSceneSelection(scene, viewState);
  if (mounted.selectedID === selectedID && options.refreshInspector !== true) return true;
  for (const node of mounted.nodes) {
    const id = node.dataset.energyPathLayoutNode;
    const related = relatedNodeIDs.has(id);
    node.classList.toggle("selected", id === selectedID);
    node.classList.toggle("related", related);
    node.setAttribute("aria-pressed", String(id === selectedID));
    if (related) node.dataset.energyPathRelated = "true";
    else delete node.dataset.energyPathRelated;
    node.dataset.energyPathFocus = energyPathFocusKind(focus, "node", id, true);
  }
  for (const ribbon of mounted.ribbons) ribbon.dataset.energyPathFocus = energyPathFocusKind(focus, "link", ribbon.dataset.energyPathRibbon);
  for (const bar of mounted.bars) bar.dataset.energyPathFocus = energyPathFocusKind(focus, "node", bar.dataset.energyPathBar);
  for (const ratio of mounted.ratios) {
    ratio.dataset.energyPathFocus = energyPathFocusKind(focus, "link", ratio.dataset.energyPathBridgeRatio);
    ratio.setAttribute("aria-pressed", String(focus.selectedLinkID === ratio.dataset.energyPathBridgeRatio));
  }
  for (const hit of mounted.hits) {
    const selected = focus.selectedLinkID === hit.dataset.energyPathLinkHit;
    hit.setAttribute("aria-pressed", String(selected));
    hit.nextElementSibling?.classList.toggle("selected", selected);
  }
  const activities = scene.graph.supplyActivities || energyPathSupplyActivities(scene.allGraphNodes, scene.graph.links);
  for (const support of mounted.supports) {
    const activity = activities.find((item) => item.kind === support.dataset.energyPathSupportKind && item.carrier === support.dataset.energyPathSupportCarrier);
    const selected = Boolean(activity?.nodeIds.includes(selectedID));
    support.classList.toggle("selected", selected);
    support.setAttribute("aria-pressed", String(selected));
  }
  mounted.inspector.innerHTML = renderEnergyPathInspector(scene, viewState, options);
  mounted.selectedID = selectedID;
  mounted.root.dataset.energyPathSelection = selectedID;
  return true;
}

export function updateEnergyPathDetails(host, scene, viewState = {}, options = {}) {
  const mounted = energyPathMountedScene(host, scene, viewState);
  if (!mounted) return false;
  mounted.details.innerHTML = renderEnergyPathDetails(scene, viewState, options);
  const expanded = String(viewState.simulationEnergyDetailsOpen === true);
  for (const control of host.querySelectorAll('[aria-controls="energyPathDataDetails"][aria-expanded]')) control.setAttribute("aria-expanded", expanded);
  return true;
}

export function renderEnergyPathContextMetrics(explanation = {}, viewState = {}) {
  if ((viewState.simulationEnergyScopeKind || "building") !== "building" ||
    energyPathToken(viewState.simulationEnergyPeriod || "annual") !== "annual") return "";
  const metrics = new Map();
  for (const source of explanation.sources || []) {
    const unit = String(source.normalizedUnit || source.units || source.sourceUnit || "").trim();
    const identity = energyPathToken([source.name, source.keyValue, source.id].filter(Boolean).join(" "));
    if (energyPathToken(source.inspectorSection) !== "context" || !identity.includes("water") || unit.toLowerCase() !== "m3") continue;
    const value = [source.effectiveValue, source.rawValue, source.allocatedValue]
      .map(Number)
      .find((candidate) => Number.isFinite(candidate) && candidate !== 0);
    if (!Number.isFinite(value)) continue;
    const key = identity.includes("facility") ? "water_facility" : `water_${metrics.size + 1}`;
    const current = metrics.get(key);
    if (!current || Math.abs(value) > Math.abs(current.value)) {
      metrics.set(key, { key, label: source.name || source.keyValue || "Water use", value, unit: "m3" });
    }
  }
  if (!metrics.size) return "";
  return `<section class="energy-path-context-metrics" data-energy-path-context-metrics>
    <header>
      <strong>${escapeHTML(t("simulation.energyPathContextMetrics", {}, "Utility context"))}</strong>
      <span>${escapeHTML(t("simulation.energyPathWaterContextNote", {}, "Water volume is not included in the site-energy scale."))}</span>
    </header>
    <div>${[...metrics.values()].map((metric) => `<article data-energy-path-context-metric="water">
      <span>${escapeHTML(metric.label)}</span>
      <strong>${escapeHTML(`${Number(metric.value).toLocaleString(undefined, { maximumFractionDigits: 2 })} ${metric.unit}`)}</strong>
    </article>`).join("")}</div>
  </section>`;
}

export function energyPathSupplyActivities(nodes = [], links = [], carrierNode = {}) {
  const nodeByID = new Map((nodes || []).filter((node) => node?.id).map((node) => [node.id, node]));
  const supplyLinks = (links || []).filter((link) => [
    "support_supply", "end_use_to_carrier", "direct_end_use_to_carrier",
  ].includes(energyPathToken(link?.relation)));
  const wantedCarrier = energyPathCanonicalCarrier(
    typeof carrierNode === "string"
      ? carrierNode
      : carrierNode?.carrier || energyPathCarrierFromNodeID(carrierNode?.id),
  );
  const chargeContextCarriers = new Set((nodes || [])
    .filter((node) => energyPathToken(node?.level) === "support" && energyPathSupportKind(node) === "storage_charge")
    .map((node) => energyPathCanonicalCarrier(node.carrier) || "electricity"));
  const grouped = new Map();
  for (const node of nodes || []) {
    const kind = energyPathSupportKind(node);
    if (!kind) continue;
    const consumptionCharge = kind === "storage_charge" && energyPathToken(node?.level) === "end_use";
    if (energyPathToken(node?.level) !== "support" && !consumptionCharge) continue;
    const nodeLinks = supplyLinks.filter((link) => (link.fromId === node.id || link.toId === node.id) &&
      (consumptionCharge || energyPathToken(link.relation) === "support_supply"));
    const linkedCarrierNode = nodeLinks
      .map((link) => nodeByID.get(link.fromId === node.id ? link.toId : link.fromId))
      .find((candidate) => energyPathToken(candidate?.level) === "carrier");
    const carrier = energyPathCanonicalCarrier(
      node.carrier || linkedCarrierNode?.carrier || energyPathCarrierFromNodeID(linkedCarrierNode?.id),
    ) || "electricity";
    if (wantedCarrier && carrier !== wantedCarrier) continue;
    if (consumptionCharge && chargeContextCarriers.has(carrier)) continue;
    const linkValue = nodeLinks
      .flatMap((link) => [Number(link.fromValue), Number(link.toValue)])
      .find((value) => Number.isFinite(value) && Math.abs(value) > 0);
    const value = Math.abs(Number.isFinite(Number(node.value)) ? Number(node.value) : linkValue || 0);
    if (!(value > 1e-9)) continue;
    const presentation = ENERGY_PATH_SUPPLY_PRESENTATION[kind];
    const key = `${carrier}|${kind}`;
    const current = grouped.get(key);
    if (!current) {
      grouped.set(key, {
        kind,
        carrier,
        carrierLabel: linkedCarrierNode?.label || ENERGY_PATH_CARRIER_PRESENTATION[carrier]?.label || carrier,
        label: t(presentation.labelKey, {}, presentation.label),
        value,
        unit: energyPathFlowUnit(node.unit || nodeLinks[0]?.fromUnit || nodeLinks[0]?.toUnit, "site"),
        nodeIds: [consumptionCharge ? linkedCarrierNode?.id : node.id].filter(Boolean),
        sourceIds: energyPathUniqueValues(node.sourceIds),
      });
      continue;
    }
    current.value += value;
    current.nodeIds = energyPathUniqueValues([...current.nodeIds, consumptionCharge ? linkedCarrierNode?.id : node.id]);
    current.sourceIds = energyPathUniqueValues([...current.sourceIds, ...(node.sourceIds || [])]);
  }
  return [...grouped.values()].sort((left, right) => (
    String(left.carrier).localeCompare(String(right.carrier)) ||
    ENERGY_PATH_SUPPLY_PRESENTATION[left.kind].order - ENERGY_PATH_SUPPLY_PRESENTATION[right.kind].order
  ));
}

export function renderEnergyPathSupportStrip(nodes = [], links = [], selectedID = "", suppliedActivities = null) {
  const activities = suppliedActivities || energyPathSupplyActivities(nodes, links);
  if (!activities.some((activity) => ENERGY_PATH_SUPPORT_STRIP_TRIGGER_KINDS.has(activity.kind))) return "";
  return `
    <section class="energy-path-support-strip" data-energy-path-support-strip role="region" aria-label="${escapeHTML(t("simulation.energyPathSupplyActivity", {}, "Supply and storage activity"))}">
      <header>
        <strong>${escapeHTML(t("simulation.energyPathSupplyActivity", {}, "Supply and storage activity"))}</strong>
        <span>${escapeHTML(t(
          "simulation.energyPathSupplyActivityDescription",
          {},
          "Supply and storage details do not add to the consumption total. Storage charge is already included in end uses.",
        ))}</span>
      </header>
      <div class="energy-path-support-items">
        ${activities.map((activity) => {
          const nodeID = activity.nodeIds[0] || "";
          const selected = activity.nodeIds.includes(selectedID);
          const valueLabel = energyPathSummaryValueLabel(activity.value, activity.unit);
          const accessibleLabel = `${activity.label}: ${valueLabel}; ${activity.carrierLabel}`;
          return `<button
            class="energy-path-support-item${selected ? " selected" : ""}"
            type="button"
            data-energy-explanation-node="${escapeHTML(nodeID)}"
            data-energy-path-support-kind="${escapeHTML(activity.kind)}"
            data-energy-path-support-carrier="${escapeHTML(activity.carrier)}"
            data-energy-path-support-value="${escapeHTML(String(activity.value))}"
            aria-label="${escapeHTML(accessibleLabel)}"
            aria-pressed="${selected ? "true" : "false"}"
          >
            <span>${escapeHTML(activity.label)}<small>${escapeHTML(activity.carrierLabel)}</small></span>
            <strong>${escapeHTML(valueLabel)}</strong>
          </button>`;
        }).join("")}
      </div>
    </section>`;
}

function energyPathSupportKind(node = {}) {
  const semantic = [node.endUse, node.kind, node.id, ...(node.badges || [])]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_");
  const canonicalTokens = ["electricity_purchased", "electricity_sold", "generators", "storage_discharge", "storage_charge"];
  const exact = canonicalTokens.find((token) => semantic.split("_").join(" ").includes(token.split("_").join(" ")));
  if (exact === "electricity_purchased") return "purchased";
  if (exact === "electricity_sold") return "sold";
  if (exact === "generators") return "produced";
  if (exact === "storage_discharge") return "storage";
  if (exact === "storage_charge") return "storage_charge";
  if (semantic.includes("electricity_purchased") || /(^|_)purchased?(_|$)/.test(semantic) || semantic.includes("grid_purchase")) {
    return "purchased";
  }
  if (semantic.includes("electricity_sold") || /(^|_)sold(_|$)/.test(semantic) || semantic.includes("surplus_sold") || semantic.includes("grid_export")) {
    return "sold";
  }
  if (semantic.includes("storage_discharge") || semantic.includes("storage_supply")) {
    return "storage";
  }
  if (semantic.includes("onsite_production") || semantic.includes("electricity_produced") || /(^|_)generators?(_|$)/.test(semantic) || /(^|_)produced?(_|$)/.test(semantic)) {
    return "produced";
  }
  return "";
}

function energyPathZoneDirectCoverageForState(explanation = {}, viewState = {}) {
  const scopedResult = energyPathResultForState(explanation, viewState);
  if (!scopedResult) return energyPathZoneDirectCoverage();
  const periodID = viewState.simulationEnergyPeriod || "annual";
  const period = (scopedResult.periods || [])
    .find((item) => energyPathToken(item?.id) === energyPathToken(periodID));
  const monthly = energyPathToken(periodID) !== "annual";
  return energyPathZoneDirectCoverage({
    scope: period?.summary?.scope || scopedResult.scope,
    completeness: monthly
      ? period?.summary?.completeness || period?.completeness || {}
      : scopedResult.summary?.completeness || scopedResult.completeness || {},
    warnings: monthly ? period?.warnings || [] : scopedResult.warnings || [],
    reconciliation: monthly ? period?.reconciliation || [] : scopedResult.reconciliation || [],
  });
}

function renderEnergyPathZoneCoverageNotice(coverage = {}) {
  if (!coverage.limited) return "";
  const count = coverage.total > 0 ? ` (${coverage.found}/${coverage.total})` : "";
  return `
    <p class="energy-path-zone-coverage-notice" data-energy-path-zone-coverage-notice="partial" role="status">
      <strong>${escapeHTML(t("simulation.energyPathPartialZoneEnergyCoverage", {}, "Partial Zone energy coverage"))}</strong>
      <span>${escapeHTML(t(
        "simulation.energyPathObservedZoneEnergySubtotalExplanation",
        {},
        "Exact zone observations and explicitly allocated HVAC energy are included; energy-source values remain subtotals, not complete zone totals.",
      ))}${escapeHTML(count)}</span>
    </p>`;
}

export function energyPathCarrierReconciliation(explanation = {}, carrierNode = {}, viewState = {}) {
  const carrier = energyPathCanonicalCarrier(
    carrierNode?.carrier || energyPathCarrierFromNodeID(carrierNode?.id),
  );
  if (!carrier) return null;
  const rows = energyPathReconciliationForState(explanation, viewState)
    .filter((row) => energyPathReconciliationCarrier(row) === carrier)
    .filter((row) => ["energy", "carrier"].includes(energyPathToken(row?.level)) || energyPathToken(row?.basis) === "carrier_residual")
    .sort((left, right) => Math.abs(Number(right.expectedValue) || 0) - Math.abs(Number(left.expectedValue) || 0));
  if (!rows.length) return null;
  const row = { ...rows[0] };
  const normalization = energyPathEnergyUnitNormalization(row.unit);
  if (normalization) {
    for (const field of ["expectedValue", "explainedValue", "residualValue", "overmappedValue"]) {
      row[field] = energyPathScaledNumber(row[field], normalization.factor);
    }
    row.unit = "kWh";
  }
  const expectedValue = Number(row.expectedValue) || 0;
  const explainedValue = Number(row.explainedValue) || 0;
  const explicitResidual = Number(row.residualValue);
  const residualValue = Number.isFinite(explicitResidual) ? explicitResidual : expectedValue - explainedValue;
  const denominator = Math.max(Math.abs(expectedValue), 1e-9);
  const residualRatio = Math.abs(residualValue) / denominator;
  const reportedStatus = energyPathToken(row.status);
  const tolerance = 1e-9 * Math.max(1, Math.abs(expectedValue), Math.abs(explainedValue));
  const status = reportedStatus === "overmapped" || residualValue < -tolerance || Number(row.overmappedValue) > tolerance
    ? "overmapped"
    : reportedStatus === "balanced" || Math.abs(residualValue) <= tolerance
      ? "balanced"
      : "residual";
  return {
    ...row,
    carrier,
    expectedValue,
    explainedValue,
    residualValue,
    residualRatio,
    status,
    qualityBasis: "carrier_residual",
    unit: energyPathFlowUnit(row.unit || carrierNode.unit, "site"),
  };
}

function energyPathReconciliationForState(explanation = {}, viewState = {}) {
  const scopedResult = energyPathResultForState(explanation, viewState);
  if (!scopedResult) return [];
  const periodID = viewState.simulationEnergyPeriod || "annual";
  const period = (scopedResult.periods || [])
    .find((item) => energyPathToken(item?.id) === energyPathToken(periodID));
  if (energyPathToken(periodID) !== "annual") return period?.reconciliation || [];
  return (scopedResult.reconciliation || []).length
    ? scopedResult.reconciliation
    : period?.reconciliation || [];
}

function energyPathReconciliationCarrier(row = {}) {
  const explicit = energyPathCanonicalCarrier(row.carrier);
  if (explicit) return explicit;
  const identity = [row.id, row.label]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "");
  const candidates = Object.keys(ENERGY_PATH_CARRIER_PRESENTATION)
    .filter((carrier) => carrier !== "water")
    .sort((left, right) => right.length - left.length);
  return candidates.find((carrier) => {
    const token = carrier.replace(/[^a-z0-9]+/g, "");
    const label = ENERGY_PATH_CARRIER_PRESENTATION[carrier].label.toLowerCase().replace(/[^a-z0-9]+/g, "");
    return identity.includes(token) || identity.includes(label);
  }) || "";
}

export function renderEnergyPathCarrierResidualBadge(quality = null) {
  if (!quality) return "";
  const status = quality.status || "residual";
  const label = energyPathCarrierResidualBadgeLabel(quality);
  return `<small
    class="energy-path-carrier-residual-badge ${escapeHTML(status)}"
    data-energy-path-carrier-residual-badge="${escapeHTML(status)}"
    data-energy-path-quality-basis="carrier_residual"
    aria-label="${escapeHTML(label)}"
  >${escapeHTML(label)}</small>`;
}

function energyPathCarrierResidualBadgeLabel(quality = {}) {
  return quality.status === "balanced"
    ? t("simulation.energyPathCarrierBalanced", {}, "Balanced")
    : quality.status === "overmapped"
      ? t(
        "simulation.energyPathCarrierOvermappedBadge",
        { share: energyPathPercentLabel(quality.residualRatio) },
        `Overmapped ${energyPathPercentLabel(quality.residualRatio)}`,
      )
      : t(
        "simulation.energyPathCarrierResidualBadge",
        { share: energyPathPercentLabel(quality.residualRatio) },
        `Unclassified ${energyPathPercentLabel(quality.residualRatio)}`,
      );
}

export function energyPathConversionRatio(link = {}, nodes = [], links = []) {
  if (energyPathToken(link.relation) !== "load_to_end_use") return null;
  const kind = energyPathToken(link.ratioKind);
  const presentation = ENERGY_PATH_CONVERSION_RATIOS[kind];
  const service = energyPathItemService(link);
  const fromValue = Number(link.fromValue);
  const toValue = Number(link.toValue);
  const ratio = Number(link.ratio);
  const downstreamCarriers = energyPathConversionDownstreamCarriers(link, nodes, links);
  if (
    (kind === "load_to_fuel" && downstreamCarriers.some((carrier) => !ENERGY_PATH_COMBUSTION_CARRIERS.has(carrier))) ||
    (kind === "efficiency" && downstreamCarriers.some((carrier) => !ENERGY_PATH_COMBUSTION_CARRIERS.has(carrier))) ||
    (kind === "load_to_purchased_energy" && downstreamCarriers.some((carrier) => !ENERGY_PATH_PURCHASED_DISTRICT_CARRIERS.has(carrier)))
  ) {
    return null;
  }
  if (
    !presentation ||
    !presentation.services.includes(service) ||
    !Number.isFinite(fromValue) || fromValue <= 0 ||
    !Number.isFinite(toValue) || toValue <= 0 ||
    !Number.isFinite(ratio) || ratio <= 0 ||
    !energyPathRatioUnitsCompatible(link.fromUnit, link.toUnit) ||
    (kind === "efficiency" && ratio > 1)
  ) {
    return null;
  }
  const derivedRatio = fromValue / toValue;
  const tolerance = 0.0005 + Number.EPSILON * Math.max(1, Math.abs(derivedRatio));
  if (Math.abs(ratio - derivedRatio) > tolerance) return null;
  return {
    kind,
    label: t(presentation.labelKey, {}, presentation.label),
    value: ratio,
  };
}

function energyPathConversionDownstreamCarriers(link = {}, nodes = [], links = []) {
  const nodeByID = new Map((nodes || []).filter((node) => node?.id).map((node) => [node.id, node]));
  const directCarrier = energyPathCanonicalCarrier(link.carrier);
  const carriers = directCarrier ? [directCarrier] : [];
  for (const branch of links || []) {
    if (
      branch?.fromId !== link.toId ||
      !["end_use_to_carrier", "direct_end_use_to_carrier"].includes(energyPathToken(branch?.relation))
    ) continue;
    const carrierNode = nodeByID.get(branch.toId);
    const carrier = energyPathCanonicalCarrier(carrierNode?.carrier || energyPathCarrierFromNodeID(branch.toId));
    if (carrier && !carriers.includes(carrier)) carriers.push(carrier);
  }
  return carriers;
}

export function energyPathConversionFlows(nodes = [], links = []) {
  const nodeByID = new Map((nodes || []).filter((node) => node?.id).map((node) => [node.id, node]));
  return (links || []).flatMap((link) => {
    if (energyPathToken(link?.relation) !== "load_to_end_use") return [];
    const fromNode = nodeByID.get(link.fromId);
    const toNode = nodeByID.get(link.toId);
    if (fromNode?.level !== "load" || toNode?.level !== "end_use") return [];
    const service = energyPathToken(toNode.endUse);
    if (!(["cooling", "heating"].includes(service)) || energyPathItemService(fromNode) !== service) return [];
    const fromValue = Number(link.fromValue);
    const toValue = Number(link.toValue);
    if (!Number.isFinite(fromValue) || fromValue <= 0 || !Number.isFinite(toValue) || toValue <= 0) return [];
    return [{
      id: link.id || energyPathPresentationLinkID(link),
      service,
      fromNode,
      toNode,
      fromValue,
      toValue,
      fromUnit: energyPathFlowUnit(link.fromUnit, "thermal"),
      toUnit: energyPathFlowUnit(link.toUnit, "site"),
      ratio: energyPathConversionRatio(link, nodes, links),
    }];
  }).sort((left, right) => {
    const serviceOrder = { cooling: 0, heating: 1 };
    return serviceOrder[left.service] - serviceOrder[right.service] || String(left.id).localeCompare(String(right.id));
  });
}

export function energyPathAuxiliaryFlows(nodes = [], links = []) {
  const nodeByID = new Map((nodes || []).filter((node) => node?.id).map((node) => [node.id, node]));
  return (links || []).flatMap((link) => {
    const relation = energyPathToken(link?.relation);
    const auxiliaryRelations = ["direct_end_use_to_carrier", "end_use_to_carrier"];
    if (energyPathToken(link?.relation) !== "direct_end_use_to_carrier" &&
      !(auxiliaryRelations.includes(relation) && relation === "end_use_to_carrier" && energyPathToken(link?.basis) === "service_path_allocation")) return [];
    const fromNode = nodeByID.get(link.fromId);
    const toNode = nodeByID.get(link.toId);
    const endUse = energyPathToken(fromNode?.endUse);
    if (fromNode?.level !== "end_use" || toNode?.level !== "carrier" || !ENERGY_PATH_AUXILIARY_END_USES.has(endUse)) return [];
    const value = Number(link.fromValue);
    if (!Number.isFinite(value) || value <= 0 || Number(link.toValue) !== value) return [];
    return [{
      id: link.id || energyPathPresentationLinkID(link),
      endUse,
      fromNode,
      toNode,
      value,
      unit: energyPathFlowUnit(link.fromUnit || link.toUnit, "site"),
    }];
  }).sort((left, right) => {
    const endUseOrder = ENERGY_PATH_END_USE_ORDER[left.endUse] ?? Number.MAX_SAFE_INTEGER;
    const rightEndUseOrder = ENERGY_PATH_END_USE_ORDER[right.endUse] ?? Number.MAX_SAFE_INTEGER;
    return endUseOrder - rightEndUseOrder || String(left.toNode.id).localeCompare(String(right.toNode.id));
  });
}

function energyPathMainStageNodes(nodes = [], links = [], explanation = {}, viewState = {}) {
  const residualCarrierByID = new Map();
  const nodeByID = new Map((nodes || []).filter((node) => node?.id).map((node) => [node.id, node]));
  for (const link of links || []) {
    if (energyPathToken(link?.relation) !== "residual") continue;
    const fromNode = nodeByID.get(link.fromId);
    const toNode = nodeByID.get(link.toId);
    if (fromNode?.level !== "residual" || toNode?.level !== "carrier" ||
      energyPathToken(link.basis) !== "residual" ||
      !(Number(link.fromValue) > 0) || !(Number(link.toValue) > 0)) continue;
    residualCarrierByID.set(fromNode.id, toNode);
  }
  return (nodes || []).flatMap((node) => {
    if (node.level !== "residual") return [node];
    const carrierNode = residualCarrierByID.get(node.id);
    if (!carrierNode || energyPathToken(node.basis) !== "residual" ||
      !(node.badges || []).some((badge) => energyPathToken(badge) === "unclassified_energy") ||
      energyPathToken(node.scaleDomain) !== "site" ||
      !(Number(node.value) > 1e-9) || Number(node.signedValue) < 0) return [];
    const quality = energyPathCarrierReconciliation(explanation, carrierNode, viewState);
    if (quality && (quality.status === "overmapped" || !(quality.residualValue > 0))) return [];
    const carrier = energyPathCanonicalCarrier(node.carrier || carrierNode.carrier || energyPathCarrierFromNodeID(carrierNode.id));
    return [{
      ...node,
      carrier,
      label: t("simulation.energyPathUnclassifiedEnergy", {}, "Unclassified energy"),
      presentationLevel: "end_use",
      presentationKind: "unclassified_energy",
    }];
  });
}

export function renderEnergyPathFlowLanes(nodes = [], links = [], selectedID = "") {
  const conversions = energyPathConversionFlows(nodes, links);
  const auxiliaries = energyPathAuxiliaryFlows(nodes, links);
  if (!conversions.length && !auxiliaries.length) return "";
  return `
    <section class="energy-path-flow-lanes" data-energy-path-flow-lanes>
      ${conversions.length ? `
        <section class="energy-path-conversion-lane" data-energy-path-conversion-lane>
          <header>
            <strong>${escapeHTML(t("simulation.energyPathMainConversions", {}, "Cooling and heating conversion"))}</strong>
            <span>${escapeHTML(t("simulation.energyPathMainConversionsDescription", {}, "Thermal load is converted to equipment site energy."))}</span>
          </header>
          <div class="energy-path-flow-lane-items">
            ${conversions.map(renderEnergyPathConversionFlow).join("")}
          </div>
        </section>` : ""}
      ${auxiliaries.length ? `
        <section class="energy-path-auxiliary-lane" data-energy-path-auxiliary-lane>
          <header>
            <strong>${escapeHTML(t("simulation.energyPathAuxiliaryLane", {}, "Auxiliary energy"))}</strong>
            <span>${escapeHTML(t("simulation.energyPathAuxiliaryLaneDescription", {}, "Fans, pumps, and heat rejection connect directly to energy sources and are excluded from conversion ratios."))}</span>
          </header>
          <div class="energy-path-flow-lane-items">
            ${auxiliaries.map((flow) => renderEnergyPathAuxiliaryFlow(flow, selectedID)).join("")}
          </div>
        </section>` : ""}
    </section>`;
}

function renderEnergyPathConversionFlow(flow = {}) {
  const title = flow.service === "cooling"
    ? t("simulation.energyPathCoolingConversion", {}, "Cooling load → Cooling equipment energy")
    : t("simulation.energyPathHeatingConversion", {}, "Heating load → Heating equipment energy");
  const ratio = flow.ratio
    ? `<span class="energy-path-conversion-ratio" data-energy-path-ratio-kind="${escapeHTML(flow.ratio.kind)}" data-energy-path-ratio-label="${escapeHTML(flow.ratio.label)}" data-energy-path-ratio-value="${escapeHTML(String(flow.ratio.value))}">
        <strong>${escapeHTML(flow.ratio.label)}</strong>
        <span>${escapeHTML(energyPathRatioValueLabel(flow.ratio.value))}</span>
      </span>`
    : "";
  return `
    <article class="energy-path-conversion-flow" data-energy-path-conversion-link="${escapeHTML(flow.id)}" data-energy-path-service="${escapeHTML(flow.service)}">
      <header><strong>${escapeHTML(title)}</strong>${ratio}</header>
      <div class="energy-path-conversion-values">
        <span>${escapeHTML(energyPathSummaryValueLabel(flow.fromValue, flow.fromUnit))}</span>
        <b aria-hidden="true">→</b>
        <span>${escapeHTML(energyPathSummaryValueLabel(flow.toValue, flow.toUnit))}</span>
      </div>
    </article>`;
}

function renderEnergyPathAuxiliaryFlow(flow = {}, selectedID = "") {
  const selected = Boolean(flow.fromNode.id && flow.fromNode.id === selectedID);
  return `
    <button class="energy-path-auxiliary-flow${selected ? " selected" : ""}" type="button" data-energy-explanation-node="${escapeHTML(flow.fromNode.id || "")}" data-energy-path-auxiliary-link="${escapeHTML(flow.id)}" data-energy-path-auxiliary-end-use="${escapeHTML(flow.endUse)}" aria-pressed="${selected ? "true" : "false"}">
      <span>${escapeHTML(flow.fromNode.label || flow.fromNode.id || "")}</span>
      <b aria-hidden="true">→</b>
      <span>${escapeHTML(flow.toNode.label || flow.toNode.id || "")}</span>
      <strong>${escapeHTML(energyPathSummaryValueLabel(flow.value, flow.unit))}</strong>
    </button>`;
}

function energyPathRatioUnitsCompatible(fromUnit = "", toUnit = "") {
  const fromBase = energyPathRatioUnitBase(fromUnit);
  const toBase = energyPathRatioUnitBase(toUnit);
  return Boolean(fromBase && toBase && fromBase === toBase);
}

function energyPathRatioUnitBase(unit = "") {
  const normalized = energyPathToken(String(unit || "").replace(/[()\[\]_]/g, " "))
    .split(/\s+/)
    .filter(Boolean)
    .filter((part) => !["thermal", "site", "purchased", "fuel", "delivered", "load", "energy"].includes(part))
    .join("")
    .replace(/[_-]/g, "");
  const supported = new Set([
    "j", "kj", "mj", "gj", "tj",
    "wh", "kwh", "mwh", "gwh",
    "btu", "kbtu", "mbtu", "mmbtu",
    "therm", "therms", "tonhour", "tonhours",
  ]);
  return supported.has(normalized) ? normalized : "";
}

function energyPathFlowUnit(unit = "", scaleDomain = "") {
  const value = String(unit || "").trim();
  if (energyPathToken(value) === "kwh") {
    return t(
      scaleDomain === "thermal" ? "simulation.energyPathThermalUnit" : "simulation.energyPathSiteUnit",
      {},
      scaleDomain === "thermal" ? "kWh thermal" : "kWh site",
    );
  }
  return value;
}

export function energyPathRatioValueLabel(value, locales = undefined) {
  const number = energyPathNodeNumber(value);
  if (number === null || number <= 0) return t("common.notAvailable", {}, "—");
  const format = (number) => number.toLocaleString(locales, { maximumFractionDigits: 2 });
  // Preserve a known positive ratio without implying exact zero at display precision.
  return number < 0.01 ? `<${format(0.01)}` : format(number);
}

export function renderEnergyPathNodeInspector(explanation = {}, nodes = [], selectedID = "", viewState = {}, relations = [], links = [], suppliedActivities = null, options = {}) {
  const node = (nodes || []).find((item) => item?.id && item.id === selectedID);
  if (!node) {
    return "";
  }
  const sourceDetails = energyPathInspectorSources(explanation, node, viewState);
  const inspectorActions = typeof options.inspectorActionsForNode === "function"
    ? options.inspectorActionsForNode(node, sourceDetails, viewState)
    : {};
  const carrierQuality = node.level === "carrier"
    ? energyPathCarrierReconciliation(explanation, node, viewState)
    : null;
  const model = energyPathInspectorModel(node, energyPathInspectorContext(explanation, nodes, links, sourceDetails, viewState, {
    carrierQuality, supplyActivities: suppliedActivities || [],
  }));
  const driverNavigation = node.level === "driver" && typeof options.driverNavigationForNode === "function"
    ? options.driverNavigationForNode(node, sourceDetails, viewState, model)
    : null;
  const hasServiceNavigation = ["load", "end_use", "carrier"].includes(node.level) && typeof options.serviceNavigationForNode === "function";
  const serviceNavigation = hasServiceNavigation ? options.serviceNavigationForNode(node, sourceDetails, viewState, model) : null;
  const unit = model.valueRows.find((row) => row.key === "total")?.unit ||
    (node.scaleDomain === "site" ? "kWh site" : "kWh thermal");
  const rowValue = (key) => model.valueRows.find((row) => row.key === key)?.value ?? null;
  const rowDirection = (key) => model.valueRows.find((row) => row.key === key)?.direction;
  const raw = rowValue("raw"), effective = rowValue("effective"), allocated = rowValue("allocated"), multiplier = rowValue("multiplier");
  const allocatedDriver = node.level === "driver" && node.allocationApplied === true;
  // The model applies sign metadata to each original driver member before
  // aggregation. A combined node's net sign must never flip a gross sum.
  const rawInspectorValue = raw;
  const effectiveInspectorValue = effective;
  const values = [
    ["total", t("simulation.energyPathInspectorTotal", {}, "Displayed total"), energyPathInspectorValueLabel(rowValue("total"), unit)],
    ["raw", allocatedDriver
      ? rowDirection("raw") === "magnitude"
        ? t("simulation.energyPathRawMagnitude", {}, "Raw magnitude · direction unavailable")
        : t("simulation.energyPathRawHeatGainLoss", {}, "Raw heat gain / loss")
      : t("simulation.energyPathRawReported", {}, "Raw reported value"), energyPathInspectorValueLabel(rawInspectorValue, unit)],
    ["multiplier", t("simulation.energyPathEffectiveMultiplier", {}, "Effective multiplier"), energyPathInspectorValueLabel(multiplier, "", 4)],
    ["effective", allocatedDriver
      ? rowDirection("effective") === "magnitude"
        ? t("simulation.energyPathEffectiveMagnitude", {}, "Magnitude after multiplier · direction unavailable")
        : t("simulation.energyPathSignedPressure", {}, "Signed pressure after multiplier")
      : t("simulation.energyPathEffectiveContribution", {}, "Effective contribution"), energyPathInspectorValueLabel(effectiveInspectorValue, unit)],
    ["allocated", allocatedDriver
      ? t("simulation.energyPathAllocatedContribution", {}, "Allocated contribution")
      : t("simulation.energyPathAllocated", {}, "Allocated"), energyPathInspectorValueLabel(allocated, unit)],
  ];
  const loadBreakdown = renderEnergyPathLoadBreakdown(node, unit, model.breakdown?.componentRows, rowValue("total") !== null);
  const allocationExplanation = renderEnergyPathAllocationExplanation(node);
  const offsetEffects = renderEnergyPathOffsetEffects(node, unit, { hideTechnical: true });
  const simultaneousLoad = renderEnergyPathSimultaneousLoad(node, unit, { hideTechnical: true });
  const correspondenceActions = renderEnergyPathCorrespondenceActions(nodes, relations, node);
  const supplyBreakdown = node.level === "carrier"
    ? renderEnergyPathSupplyBreakdown(suppliedActivities
      ? suppliedActivities.filter((activity) => activity.carrier === energyPathCanonicalCarrier(node.carrier || energyPathCarrierFromNodeID(node.id)))
      : energyPathSupplyActivities(nodes, links, node))
    : "";
  const carrierReconciliation = renderEnergyPathCarrierReconciliation(carrierQuality ? { ...carrierQuality, basis: "", formula: "", observedSubtotal: viewState.simulationEnergyScopeKind === "zone" } : null);
  const skipContextKeys = new Set([
    ...(carrierReconciliation ? ["facility_total", "observed_subtotal", "classified", "residual"] : []),
    ...(supplyBreakdown ? ["purchased", "sold", "produced", "storage", "storage_charge"] : []),
  ]);
  const breakdown = loadBreakdown + renderEnergyPathInspectorBreakdown(model, { skipComponents: Boolean(loadBreakdown), skipContextKeys }) +
    offsetEffects + simultaneousLoad + renderEnergyPathGroupedMembers(node) + supplyBreakdown + carrierReconciliation;
  const actions = renderEnergyPathDriverNavigation(node, driverNavigation, model) +
    renderEnergyPathInspectorActions(node, inspectorActions, { suppressHVAC: hasServiceNavigation }) +
    renderEnergyPathServiceNavigation(node, serviceNavigation, model) + correspondenceActions;
  return `
    <aside class="energy-path-node-inspector" data-energy-path-inspector="${escapeHTML(node.id)}">
      <header>
        <strong>${escapeHTML(t("simulation.energyPathInspector", {}, "Energy Path detail"))}</strong>
        <span>${escapeHTML(energyPathInspectorSafeLabel(node.label || node.kind, model))}</span>
      </header>
      ${renderEnergyPathDetailSection("represents", renderEnergyPathRepresentation(node, model))}
      ${renderEnergyPathDetailSection("value", renderEnergyPathInspectorValues(values))}
      ${renderEnergyPathDetailSection("breakdown", breakdown)}
      ${renderEnergyPathDetailSection("basis", renderEnergyPathInspectorBasis(node, model) + allocationExplanation)}
      ${renderEnergyPathDetailSection("actions", actions)}
    </aside>`;
}

function energyPathInspectorContext(explanation, nodes, links, sources, viewState, extra = {}) {
  const period = viewState.simulationEnergyPeriod || "annual";
  const scopedResult = energyPathResultForState(explanation, viewState);
  const wrapper = (scopedResult?.periods || []).find((candidate) => energyPathToken(candidate.id) === energyPathToken(period));
  const wrapperNodeIDs = new Set((wrapper?.nodes || []).map((node) => node.id));
  const wrapperBacked = nodes.length > 0 && nodes.every((node) => [node.id, ...(node.originalNodeIds || [])].some((id) => wrapperNodeIDs.has(id)));
  return {
    kind: "node", nodes, links, sources,
    period, periodContext: wrapperBacked ? period : "",
    scope: { kind: viewState.simulationEnergyScopeKind || "building", zoneName: viewState.simulationEnergyZoneName || "" },
    zoneResults: energyPathZoneResults(explanation),
    ratioQuality: energyPathRatioQualityForState(explanation, viewState),
    ...extra,
  };
}

function energyPathInspectorValueLabel(value, unit = "", digits = 2) {
  if (typeof value !== "number" || !Number.isFinite(value)) return t("common.notAvailable", {}, "—");
  return [value.toLocaleString(undefined, { maximumFractionDigits: digits }), energyPathFlowUnit(unit, unit.includes("thermal") ? "thermal" : "site")].filter(Boolean).join(" ");
}

function energyPathInspectorSafeLabel(value, model = {}, fallback = "") {
  const label = String(value || "").trim();
  const ids = [...(model.sourceIds || []), ...(model.ruleIds || [])].filter(Boolean);
  return label && !ids.some((id) => label.includes(id)) ? label
    : fallback || t("simulation.energyPathInspectorItem", {}, "Energy-path item");
}

function renderEnergyPathDetailSection(key, content = "") {
  const labels = {
    represents: ["simulation.energyPathDetailRepresents", "What this represents"],
    value: ["simulation.energyPathDetailValue", "Value"],
    breakdown: ["simulation.energyPathInspectorBreakdown", "Breakdown"],
    basis: ["simulation.energyPathDetailBasis", "Calculation / allocation basis"],
    actions: ["simulation.energyPathDetailActions", "Actions"],
  };
  const [labelKey, label] = labels[key];
  const empty = key === "actions"
    ? t("simulation.energyPathInspectorNoActions", {}, "No actions are available for this selection.")
    : t("simulation.energyPathInspectorNoBreakdown", {}, "No additional breakdown is reported for this selection and period.");
  return `<section class="energy-path-detail-section" data-energy-path-detail-section="${key}"><h5>${escapeHTML(t(labelKey, {}, label))}</h5>${content || `<p class="energy-path-detail-empty">${escapeHTML(empty)}</p>`}</section>`;
}

function renderEnergyPathInspectorValues(values, attribute = "data-energy-path-inspector-value") {
  return `<dl>${values.map(([key, label, value]) => `<div ${attribute}="${escapeHTML(key)}"><dt>${escapeHTML(label)}</dt><dd>${escapeHTML(value)}</dd></div>`).join("")}</dl>`;
}

function energyPathThermalBoundary(node) {
  // This is an explicit backend measurement contract, never inferred from
  // output names, equipment labels, or another service's source dictionary.
  return node?.level === "load" && ["active_surface_source", "mixed_thermal_boundaries"].includes(node.thermalBoundary)
    ? node.thermalBoundary : "";
}

function renderEnergyPathRepresentation(item, model, thermalLoad = item) {
  const stage = model.representation?.stage || item.level || item.relation;
  const descriptions = {
    driver: ["simulation.energyPathRepresentsDriver", "Contribution of a heat gain or loss to the selected thermal load. Allocated contributions are not a direct causal decomposition."],
    load: ["simulation.energyPathRepresentsLoad", "Thermal energy delivered to meet cooling or heating demand, separate from the equipment's site-energy use."],
    end_use: ["simulation.energyPathRepresentsEndUse", "Site energy consumed by this end use. It is not added to the related thermal load."],
    carrier: ["simulation.energyPathRepresentsCarrier", "Site-energy consumption supplied by this energy source. Supply and storage activity are shown separately as context."],
    residual: ["simulation.energyPathRepresentsResidual", "A positive, unclassified energy gap between the reported energy-source total and mapped consumption."],
    load_to_end_use: ["simulation.energyPathRepresentsConversion", "A matched equipment conversion from thermal load to site-energy use. The two sides retain their own units and values."],
    driver_to_load: ["simulation.energyPathRepresentsDriverLink", "The contribution assigned from this load driver to the connected thermal load."],
    end_use_to_carrier: ["simulation.energyPathRepresentsCarrierLink", "The reported or allocated share of this end use supplied by the connected energy source."],
  };
  const [key, fallback] = descriptions[item.relation] || descriptions[stage] || ["simulation.energyPathRepresentsItem", "Reported energy-path information for the selected item and period."];
  const service = energyPathToken(item.serviceKind);
  const boundary = energyPathThermalBoundary(thermalLoad);
  const description = boundary ? `<div data-energy-path-thermal-boundary="${boundary}">
    <strong>${escapeHTML(t("simulation.energyPathThermalBoundary", {}, "Thermal measurement boundary"))}</strong>
    <p>${escapeHTML(boundary === "active_surface_source"
      ? t("simulation.energyPathBoundaryActiveSurface", {}, "Heat added to or removed from the active radiant surface by the radiant fluid circuit. Model multipliers are already included. This is not heat delivered to Zone air in the same period.")
      : t("simulation.energyPathBoundaryMixed", {}, "Combined air-system delivery and active-surface source/sink heat. This total does not represent a single Zone-air measurement boundary."))}</p>
    <p>${escapeHTML(t("simulation.energyPathBoundaryComparison", {}, "Any ratio is a load/site-energy comparison, not equipment COP or efficiency. Surface heat storage and exchange with other surfaces can shift Zone-air effects between periods. Do not add active-surface heat again."))}</p>
  </div>` : `<p>${escapeHTML(t(key, {}, fallback))}</p>`;
  return `${description}${["cooling", "heating"].includes(service) ? `<small>${escapeHTML(t("simulation.service", {}, "Service"))}: ${escapeHTML(t(service === "cooling" ? "simulation.cooling" : "simulation.heating", {}, service === "cooling" ? "Cooling" : "Heating"))}</small>` : ""}`;
}

function renderEnergyPathInspectorBreakdown(model, options = {}) {
  const groups = [
    ["componentRows", "simulation.energyPathLoadBreakdown", "Sensible / latent breakdown"],
    ["zoneRows", "simulation.energyPathSummaryTopZones", "Top zones"],
    ["carrierRows", "simulation.energyPathInspectorCarrierSplit", "Energy-source split"],
    ["endUseRows", "simulation.energyPathInspectorEndUseSplit", "End-use breakdown"],
    ["allocationRows", "simulation.energyPathInspectorAllocationSplit", "Direct / allocated breakdown"],
    ["contextRows", "simulation.energyPathInspectorContext", "Context"],
    ["ratioRows", "simulation.energyPathKPILoadSiteRatio", "Load/site ratio"],
  ];
  return groups.map(([groupKey, labelKey, fallback]) => {
    if (groupKey === "componentRows" && options.skipComponents) return "";
    if (groupKey === "ratioRows" && options.skipRatios) return "";
    const rows = (model.breakdown?.[groupKey] || []).filter((row) => groupKey !== "contextRows" || !options.skipContextKeys?.has(row.key));
    if (!rows.length) return "";
    const rowLabel = (row) => {
      const labels = {
        sensible: ["simulation.energyPathSensibleLoad", "Sensible"], latent: ["simulation.energyPathLatentLoad", "Latent"],
        direct: ["simulation.energyPathInspectorDirect", "Direct / reported"], allocated: ["simulation.energyPathAllocated", "Allocated"], unknown: ["simulation.energyPathInspectorUnknownBasis", "Unspecified basis"],
        cooling: ["simulation.energyPathCoolingLoad", "Cooling"], heating: ["simulation.energyPathHeatingLoad", "Heating"],
        facility_total: ["simulation.energyPathCarrierTotal", "Facility carrier total"], observed_subtotal: ["simulation.energyPathObservedDirectUseSubtotal", "Observed direct-use subtotal"],
        classified: ["simulation.energyPathClassifiedEndUses", "Mapped end uses"], residual: ["simulation.energyPathUnclassifiedResidual", "Residual"],
        "load.predicted.sensible": ["simulation.energyPathInspectorPredictedLoad", "Predicted sensible load"],
        "load.predicted_vs_delivered.sensible": ["simulation.energyPathInspectorPredictedGap", "Predicted minus delivered sensible load"],
      };
      const known = labels[row.key];
      if (groupKey === "carrierRows" && ENERGY_PATH_CARRIER_PRESENTATION[row.key]) {
        const carrier = ENERGY_PATH_CARRIER_PRESENTATION[row.key];
        return t(carrier.labelKey, {}, carrier.label);
      }
      if (groupKey === "endUseRows" && ENERGY_PATH_END_USE_PRESENTATION[row.key]) {
        const endUse = ENERGY_PATH_END_USE_PRESENTATION[row.key];
        return t(endUse.labelKey, {}, endUse.label);
      }
      return known ? t(known[0], {}, known[1]) : energyPathInspectorSafeLabel(row.label, model, t(labelKey, {}, fallback));
    };
    return `<section class="energy-path-detail-breakdown" data-energy-path-detail-breakdown="${groupKey}"><h6>${escapeHTML(t(labelKey, {}, fallback))}</h6><dl>${rows.map((row) => `<div data-energy-path-detail-row="${escapeHTML(row.key || "")}"><dt>${escapeHTML(rowLabel(row))}</dt><dd>${escapeHTML(groupKey === "ratioRows" ? energyPathRatioValueLabel(row.value) : energyPathInspectorValueLabel(row.value, row.unit || ""))}${row.partial ? ` · ${escapeHTML(t("simulation.energyPathKPIMatchedConversions", {}, "Partial overlap · matched conversions only"))}` : ""}</dd>${groupKey === "ratioRows" ? `<small>${escapeHTML(t("simulation.energyPathBridgeFrom", {}, "From"))}: ${escapeHTML(energyPathInspectorValueLabel(row.fromValue, row.fromUnit))} · ${escapeHTML(t("simulation.energyPathBridgeTo", {}, "To"))}: ${escapeHTML(energyPathInspectorValueLabel(row.toValue, row.toUnit))}</small>` : ""}</div>`).join("")}</dl></section>`;
  }).join("");
}

function renderEnergyPathInspectorBasis(item, model, attribute = "data-energy-path-inspector-value") {
  const kind = energyPathToken(model.basis?.kind || item.basis);
  const allocationNote = energyPathToken(item.allocationExplanation || item.explanation);
  const airflowAllocation = kind === "service_path_allocation" && allocationNote.includes("airloop") &&
    (allocationNote.includes("supply_air") || allocationNote.includes("supply-air"));
  const labels = {
    direct_zone_energy: ["simulation.energyPathDirectZoneEnergy", "Direct zone energy"],
    service_path_allocation: airflowAllocation
      ? ["simulation.energyPathAirflowAllocation", "Allocated by related AirLoop supply-air volume share"]
      : /load[ _-]+share/.test(allocationNote)
        ? ["simulation.energyPathServicePathAllocation", "Allocated by HVAC service-path load share"]
        : ["simulation.energyPathRelatedServiceAllocation", "Allocated using the related HVAC service path"],
    zone_load_allocation: ["simulation.energyPathInspectorZoneAllocation", "Allocated by zone load share"],
    heat_balance_share: ["simulation.energyPathInspectorHeatBalanceBasis", "Signed heat-balance share allocation"],
    grouped_presentation: ["simulation.energyPathInspectorGroupedBasis", "Grouped original contributions; each member retains its own calculation basis"],
    reported: ["simulation.energyPathInspectorReportedBasis", "Reported simulation value"],
    measured_meter: ["simulation.energyPathInspectorReportedBasis", "Reported simulation value"],
    measured_energy_variable: ["simulation.energyPathInspectorReportedBasis", "Reported simulation value"],
    direct_meter: ["simulation.energyPathInspectorReportedBasis", "Reported simulation value"],
    reported_meter: ["simulation.energyPathInspectorReportedBasis", "Reported simulation value"],
    reported_variable: ["simulation.energyPathInspectorReportedBasis", "Reported simulation value"],
    reported_end_use_subtotal: ["simulation.energyPathInspectorSubtotalBasis", "Subtotal of observed end uses; not a complete facility total"],
    allocated: ["simulation.energyPathInspectorAllocatedBasis", "Explicitly allocated contribution"],
    residual: ["simulation.energyPathInspectorResidualBasis", "Difference between the reported source total and classified uses"],
    integrated_rate: ["simulation.energyPathInspectorIntegratedBasis", "Reported rate integrated over time"],
    derived_ratio: ["simulation.energyPathInspectorDerivedBasis", "Calculated from matched reported values"],
  };
  const named = labels[kind];
  const label = named ? t(named[0], {}, named[1]) : t("simulation.energyPathInspectorUnknownBasis", {}, "Unspecified basis");
  const values = [["basis", t("simulation.energyPathBasis", {}, "Basis"), label]];
  const applicationLabels = {
    already_model_total: ["simulation.energyPathMultiplierModelTotal", "Already included in model total"],
    requires_zone_multiplier: ["simulation.energyPathMultiplierZone", "Zone multiplier required"],
    requires_group_multiplier: ["simulation.energyPathMultiplierGroup", "Zone-group multiplier required"],
    unknown: ["simulation.energyPathInspectorUnknownBasis", "Unspecified basis"],
  };
  const applications = energyPathUniqueValues(String(model.basis?.application || "unknown").split(",").map((value) => {
    const [key, fallback] = applicationLabels[energyPathToken(value)] || applicationLabels.unknown;
    return t(key, {}, fallback);
  }));
  values.push(["application", t("simulation.energyPathMultiplierApplication", {}, "Multiplier application"), applications.join(" · ")]);
  return renderEnergyPathInspectorValues(values, attribute);
}

function energyPathDriverDestinationLabel(item = {}, model = {}, fallback = "") {
  const kinds = {
    connection_context: ["simulation.energyPathDriverConnectionContext", "Related connection context"],
    zone_context: ["simulation.energyPathDriverZoneContext", "Zone topology context"],
    air_coupling: ["simulation.energyPathRelatedAirCoupling", "Air coupling"],
    outdoor_air_service: ["simulation.energyPathDriverOutdoorAirService", "Outdoor-air service"],
    building_source_group: ["simulation.energyPathDriverBuildingGroup", "Building source group"],
    profile_source_group: ["simulation.energyPathDriverProfileGroup", "Profile source group"],
    model_context: ["simulation.energyPathDriverRelatedContext", "Related model context"],
    profile_occupancy: ["simulation.energyPathDriverProfileOccupancy", "Occupancy"],
    profile_lighting: ["simulation.energyPathDriverProfileLighting", "Lighting"],
    profile_equipment: ["simulation.energyPathDriverProfileEquipment", "Equipment"],
    profile_infiltration: ["simulation.energyPathDriverProfileInfiltration", "Infiltration"],
    profile_ventilation: ["simulation.energyPathDriverProfileVentilation", "Ventilation"],
    profile_outdoor_air: ["simulation.energyPathDriverProfileOutdoorAir", "Outdoor air"],
  };
  const kind = kinds[item.labelKind];
  const label = kind ? t(kind[0], {}, kind[1]) : energyPathInspectorSafeLabel(item.label || item.target?.label || item.zoneName, model, fallback);
  const context = String(item.contextLabel || "").trim() ? energyPathInspectorSafeLabel(item.contextLabel, model, "") : "";
  return [label, context].filter(Boolean).join(" · ");
}

export function renderEnergyPathDriverNavigation(node = {}, navigation = null, model = {}) {
  if (node.level !== "driver" || !navigation) return "";
  const category = energyPathToken(node.driverCategory || navigation.category);
  const categoryLabels = {
    "surface.exterior_walls": "Exterior walls", "surface.roofs": "Roofs",
    "surface.ground_floors": "Ground / floors", "surface.windows_doors": "Windows / doors",
    "surface.interzone": "Interzone surfaces", "air.infiltration": "Infiltration",
    "air.mechanical_ventilation": "Mechanical ventilation", "air.interzone": "Interzone air",
    "internal.people": "People", "internal.lighting": "Lighting", "internal.equipment": "Equipment",
    "interzone.transfer": "Interzone transfer", "internal.other": "Other internal gains",
    "balance.storage_other": "Other / storage",
  };
  const categoryLabel = Object.prototype.hasOwnProperty.call(categoryLabels, category)
    ? t(`simulation.energyPathDriverCategory.${category}`, {}, categoryLabels[category])
    : t("simulation.energyPathStageDrivers", {}, "Load Drivers");
  const groups = (Array.isArray(navigation.groups) ? navigation.groups : []).filter((group) => group && Array.isArray(group.candidates));
  const safeModel = { ...model, sourceIds: energyPathUniqueValues([...(model.sourceIds || []), ...groups.flatMap((group) => group.sourceIds || [])]) };
  const views = [
    ["topology", "tab.topology", "Topology"],
    ["profile", "tab.profile", "Profile"],
    ["hvac", "tab.hvac", "HVAC"],
  ];
  const sections = views.map(([view, labelKey, fallback]) => {
    const viewLabel = t(labelKey, {}, fallback);
    const viewGroups = groups.map((group) => ({ ...group, candidates: group.candidates.filter((candidate) => candidate?.id && candidate.view === view) }))
      .filter((group) => group.candidates.length);
    if (!viewGroups.length) return "";
    const count = viewGroups.reduce((total, group) => total + group.candidates.length, 0);
    const content = `<ul class="energy-path-driver-groups">${viewGroups.map((group) => {
      const groupLabel = energyPathDriverDestinationLabel(group, safeModel, t("simulation.energyPathDriverSourceGroup", {}, "Source group"));
      const contribution = typeof group.value === "number" && Number.isFinite(group.value)
        ? `${t("simulation.energyPathDriverZoneContribution", {}, "Zone driver contribution")}: ${energyPathInspectorValueLabel(group.value, group.unit || "kWh thermal")}` : "";
      return `<li data-energy-path-driver-group="${escapeHTML(group.id || "")}" data-energy-path-driver-zone="${escapeHTML(group.zoneName || "")}">
        <header><strong>${escapeHTML(groupLabel)}</strong>${contribution ? `<small data-energy-path-driver-zone-contribution>${escapeHTML(contribution)}</small>` : ""}</header>
        <ul class="energy-path-driver-candidates">${group.candidates.map((candidate) => {
          const contextOnly = candidate.evidenceKind !== "exact_source";
          const label = energyPathDriverDestinationLabel(candidate, safeModel, viewLabel);
          const evidenceLabel = contextOnly
            ? t("simulation.energyPathDriverCategoryContext", {}, "Model context only · no contribution is assigned to this target")
            : t("simulation.energyPathDriverExactSource", {}, "Linked source context");
          return `<li><button class="energy-path-inspector-action energy-path-driver-destination" type="button" data-energy-path-driver-node="${escapeHTML(node.id)}" data-energy-path-driver-destination="${escapeHTML(candidate.id)}" data-energy-path-driver-evidence="${contextOnly ? "category_context" : "exact_source"}">
            <span>${escapeHTML(t("simulation.energyPathDriverOpenView", { view: viewLabel }, `Open ${viewLabel}`))}</span><strong>${escapeHTML(label)}</strong><small>${escapeHTML(evidenceLabel)}</small>
          </button></li>`;
        }).join("")}</ul>
      </li>`;
    }).join("")}</ul>`;
    return `<section class="energy-path-driver-view" data-energy-path-driver-view="${view}"><h6>${escapeHTML(viewLabel)}</h6>${count > 1
      ? `<details data-energy-path-driver-chooser="${view}"><summary>${escapeHTML(t("simulation.energyPathDriverChooseTarget", {}, "Choose a Zone or source group"))} · ${count}</summary>${content}</details>`
      : content}</section>`;
  }).join("");
  return `<section class="energy-path-driver-navigation" data-energy-path-driver-navigation="${escapeHTML(navigation.status || "unavailable")}" data-energy-path-driver-category="${escapeHTML(navigation.category || "")}">
    <h6>${escapeHTML(t("simulation.energyPathDriverModelContext", {}, "Explore related model context"))} · ${escapeHTML(categoryLabel)}</h6>
    ${sections || `<button class="energy-path-inspector-action" type="button" disabled>${escapeHTML(t("simulation.energyPathDriverModelContext", {}, "Explore related model context"))}</button><p class="energy-path-detail-empty">${escapeHTML(t("simulation.energyPathDriverNavigationUnavailable", {}, "No verified Topology, Profile, or outdoor-air service target is available for this driver."))}</p>`}
  </section>`;
}

function energyPathServiceNavigationLabel(item = {}, model = {}, fallback = "") {
  const kinds = {
    service_path: ["simulation.energyPathServicePath", "HVAC service path"],
    air_loop: ["simulation.energyPathServiceAirLoop", "Air loop"],
    plant_loop: ["simulation.energyPathServicePlantLoop", "Plant loop"],
    system: ["simulation.energyPathServiceSystem", "Connected system"],
    component: ["simulation.energyPathServiceComponent", "Connected component"],
    heat_flow_ledger: ["simulation.heatFlow", "Heat-Flow Ledger"],
    output_request: ["simulation.energyPathOutputRequest", "Output request"],
    output_input: ["simulation.energyPathServiceOutputInput", "Input-source Output request"],
    building_source_group: ["simulation.energyPathDriverBuildingGroup", "Building source group"],
  };
  const kind = kinds[item.labelKind];
  const label = kind ? t(kind[0], {}, kind[1]) : energyPathInspectorSafeLabel(item.label || item.target?.label, model, fallback);
  const context = String(item.contextLabel || "").trim() ? energyPathInspectorSafeLabel(item.contextLabel, model, "") : "";
  return [label, context].filter(Boolean).join(" · ");
}

function energyPathServiceUnavailableReason(reason = "", kind = "") {
  const reasons = {
    no_explicit_hvac_path: ["simulation.energyPathServiceHVACUnavailable", "No verified HVAC path, loop, or connected system is available for this selection."],
    scope_mismatch: ["simulation.energyPathServiceScopeUnavailable", "No matching destination is verified for the selected Zone."],
    service_mismatch: ["simulation.energyPathServiceKindUnavailable", "No matching destination is verified for the selected service."],
    period_mismatch: ["simulation.energyPathServicePeriodUnavailable", "No matching destination is verified for the selected period."],
    unsupported_domain: ["simulation.energyPathServiceDomainUnavailable", "The reported unit and energy domain do not support this destination."],
    heat_flow_unavailable: ["simulation.energyPathServiceLedgerUnavailable", "No matching Zone Heat-Flow Ledger is available in this result."],
    heat_flow_period_unavailable: ["simulation.energyPathServiceLedgerPeriodUnavailable", "The Zone ledger has no usable frames for the selected period."],
    output_unavailable: ["simulation.energyPathOutputUnavailable", "No exact request in this run plan"],
    output_ambiguous: ["simulation.energyPathOutputAmbiguous", "More than one matching request"],
    output_derived: ["simulation.energyPathOutputDerived", "Calculated from input sources"],
    output_tabular: ["simulation.energyPathOutputTabular", "Reported in an annual summary table"],
    facility_meter_unavailable: ["simulation.energyPathServiceFacilityUnavailable", "No exact facility-meter Output request is verified for this carrier."],
  };
  const fallback = kind === "hvac" ? reasons.no_explicit_hvac_path : kind === "heat_flow" ? reasons.heat_flow_unavailable : reasons.output_unavailable;
  const [key, label] = reasons[reason] || fallback;
  return t(key, {}, label);
}

export function renderEnergyPathServiceNavigation(node = {}, navigation = null, model = {}) {
  if (!node.id || !["load", "end_use", "carrier"].includes(node.level) || !navigation) return "";
  const groups = (Array.isArray(navigation.groups) ? navigation.groups : []).filter((group) => group && Array.isArray(group.candidates));
  const reasons = navigation.unavailableReasons || {};
  const candidates = groups.flatMap((group) => group.candidates);
  const safeModel = { ...model, sourceIds: energyPathUniqueValues([
    ...(model.sourceIds || []), ...groups.flatMap((group) => [group.id, ...(group.sourceIds || [])]),
    ...candidates.flatMap((candidate) => [candidate.id, candidate.sourceId]),
  ]) };
  const kinds = [
    { kind: "hvac", title: t("simulation.energyPathRelatedHVAC", {}, "Related HVAC"), action: t("simulation.energyPathOpenHVAC", {}, "Open HVAC"),
      choose: t("simulation.energyPathServiceChooseHVAC", {}, "Choose a service path, loop, or system"),
      note: t("simulation.energyPathServiceModelContext", {}, "Verified model relationships are shown here; no energy contribution is assigned to an individual target.") },
    { kind: "heat_flow", title: t("simulation.heatFlow", {}, "Heat-Flow Ledger"), action: t("simulation.energyPathServiceOpenLedger", {}, "Open Zone ledger"),
      choose: t("simulation.energyPathServiceChooseZone", {}, "Choose a Zone ledger"),
      note: t("simulation.energyPathServiceLedgerContext", {}, "Zone heat-balance context for this period, not the source of the displayed load total.") },
    { kind: "output", title: t("simulation.energyPathServiceOutput", {}, "Output requests"), action: t("simulation.energyPathServiceOpenOutput", {}, "Open exact Output request"),
      choose: t("simulation.energyPathServiceChooseOutput", {}, "Choose an Output request"), note: "" },
  ];
  return `<div class="energy-path-service-navigation" data-energy-path-service-navigation>${kinds.map((definition) => {
    const ownGroups = groups.filter((group) => group.kind === definition.kind).map((group) => ({ ...group,
      candidates: group.candidates.filter((candidate) => candidate?.id && candidate.kind === definition.kind),
    })).filter((group) => group.candidates.length);
    if (!ownGroups.length && !Object.prototype.hasOwnProperty.call(reasons, definition.kind)) return "";
    const count = ownGroups.reduce((sum, group) => sum + group.candidates.length, 0);
    const reasonID = `energyPathService${definition.kind}Unavailable`;
    const content = `<ul class="energy-path-service-groups">${ownGroups.map((group) => {
      const label = energyPathServiceNavigationLabel(group, safeModel, group.zoneName || definition.title);
      const contribution = group.zoneName && typeof group.value === "number" && Number.isFinite(group.value)
        ? `${t("simulation.energyPathServiceZoneContribution", {}, "Zone contribution")}: ${energyPathInspectorValueLabel(group.value, group.unit || "kWh thermal")}` : "";
      return `<li data-energy-path-service-group="${escapeHTML(group.id || "")}" data-energy-path-service-zone="${escapeHTML(group.zoneName || "")}">
        <header><strong>${escapeHTML(label)}</strong>${contribution ? `<small data-energy-path-service-zone-contribution>${escapeHTML(contribution)}</small>` : ""}</header>
        <ul class="energy-path-service-candidates">${group.candidates.map((candidate) => {
          const targetLabel = energyPathServiceNavigationLabel(candidate, safeModel, definition.title);
          const input = candidate.evidenceKind === "derived_input" ? t("simulation.energyPathServiceDerivedInput", {}, "Exact request for an input source; the displayed total is calculated.") : "";
          const fields = candidate.kind === "output" ? candidate.requestFields || {} : {};
          const requestIdentity = [fields.objectType, fields.keyValue, fields.variableName, fields.reportingFrequency]
            .filter((value) => typeof value === "string" && value.trim())
            .map((value) => energyPathInspectorSafeLabel(value, safeModel));
          const serviceLabels = {
            cooling: ["simulation.cooling", "Cooling"], heating: ["simulation.heating", "Heating"],
            ventilation: ["simulation.energyPathDriverProfileVentilation", "Ventilation"], exhaust: ["simulation.energyPathServiceExhaust", "Exhaust"],
          };
          const serviceLabel = serviceLabels[candidate.serviceKind];
          const routeKind = candidate.routeKind || (["service-path", "hvac-path"].includes(candidate.target?.targetKind) ? "service_path" : "");
          const modelIdentity = candidate.kind === "hvac" ? [
            routeKind ? energyPathServiceNavigationLabel({ labelKind: routeKind }, safeModel, definition.title) : "",
            serviceLabel ? t(serviceLabel[0], {}, serviceLabel[1]) : "",
          ].filter(Boolean) : [];
          return `<li><button class="energy-path-inspector-action energy-path-service-destination" type="button" data-energy-path-service-node="${escapeHTML(node.id)}" data-energy-path-service-destination="${escapeHTML(candidate.id)}" data-energy-path-service-evidence="${escapeHTML(candidate.evidenceKind || "")}">
            <span>${escapeHTML(definition.action)}</span><strong>${escapeHTML(targetLabel)}</strong>${modelIdentity.length ? `<small data-energy-path-service-model-identity>${escapeHTML(modelIdentity.join(" · "))}</small>` : ""}${requestIdentity.length ? `<small data-energy-path-service-request-identity>${escapeHTML(requestIdentity.join(" · "))}</small>` : ""}${input ? `<small>${escapeHTML(input)}</small>` : ""}
          </button></li>`;
        }).join("")}</ul></li>`;
    }).join("")}</ul>`;
    return `<section class="energy-path-inspector-action-group" data-energy-path-service-kind="${definition.kind}"><h5>${escapeHTML(definition.title)}</h5>
      ${definition.note ? `<p>${escapeHTML(definition.note)}</p>` : ""}
      ${count > 1 ? `<details data-energy-path-service-chooser="${definition.kind}"><summary>${escapeHTML(definition.choose)} · ${count}</summary>${content}</details>`
        : count === 1 ? content : `<button class="energy-path-inspector-action" type="button" disabled aria-describedby="${reasonID}">${escapeHTML(definition.action)}</button><small id="${reasonID}" class="energy-path-action-unavailable">${escapeHTML(energyPathServiceUnavailableReason(reasons[definition.kind], definition.kind))}</small>`}
    </section>`;
  }).join("")}</div>`;
}

export function renderEnergyPathInspectorActions(node = {}, actions = {}, options = {}) {
  if (!node.id) return "";
  const context = actions || {};
  const groups = [{
    kind: "series",
    title: t("simulation.energyPathSourceSeries", {}, "Source series"),
    action: t("simulation.energyPathOpenSeries", {}, "Open Series"),
    choose: t("simulation.energyPathChooseSeries", {}, "Choose source series"),
    targets: context.series,
    reason: context.seriesUnavailableReason || t("simulation.energyPathSeriesUnavailable", {}, "No matching source series is available for this selection."),
  }];
  if (["load", "end_use"].includes(node.level) && !options.suppressHVAC) groups.push({
    kind: "hvac",
    title: t("simulation.energyPathRelatedHVAC", {}, "Related HVAC"),
    action: t("simulation.energyPathOpenHVAC", {}, "Open HVAC"),
    choose: t("simulation.energyPathChooseHVACPath", {}, "Choose HVAC path"),
    targets: context.hvacPaths,
    reason: context.hvacUnavailableReason || t("simulation.energyPathHVACUnavailable", {}, "No explicit HVAC service path is available for this selection."),
  });
  return `<div class="energy-path-inspector-actions" data-energy-path-inspector-actions>
    ${groups.map((group) => {
      const targets = (Array.isArray(group.targets) ? group.targets : []).filter((target) => target && String(target.id || "").trim());
      const reasonID = `energyPath${group.kind === "series" ? "Series" : "HVAC"}Unavailable`;
      const button = (target) => {
        const period = String(target.period || "annual");
        const periodOption = ENERGY_PATH_PERIODS.find((option) => option.value === period);
        const periodLabel = periodOption ? t(periodOption.labelKey, {}, periodOption.label) : period;
        const detail = group.kind === "series"
          ? [target.label || group.title, target.frequency, periodLabel].filter(Boolean).join(" · ")
          : target.label || group.title;
        return `<button class="energy-path-inspector-action" type="button"
          ${group.kind === "series"
            ? `data-energy-path-series-id="${escapeHTML(target.id)}" data-energy-path-series-period="${escapeHTML(period)}" data-energy-path-series-source="${escapeHTML(target.sourceId || "")}"`
            : `data-energy-path-hvac-path-id="${escapeHTML(target.id)}"`}>
          <span>${escapeHTML(group.action)}</span><small>${escapeHTML(detail)}</small>
        </button>`;
      };
      return `<section class="energy-path-inspector-action-group" data-energy-path-${group.kind}-actions>
        <h5>${escapeHTML(group.title)}</h5>
        ${targets.length > 1 ? `<details data-energy-path-action-chooser="${group.kind}">
          <summary>${escapeHTML(group.choose)} · ${targets.length}</summary>
          <ul>${targets.map((target) => `<li>${button(target)}</li>`).join("")}</ul>
        </details>` : targets.length === 1 ? button(targets[0]) : `<button class="energy-path-inspector-action" type="button" disabled aria-describedby="${reasonID}">${escapeHTML(group.action)}</button>
          <small id="${reasonID}" class="energy-path-action-unavailable">${escapeHTML(group.reason)}</small>`}
      </section>`;
    }).join("")}
  </div>`;
}

function renderEnergyPathGroupedMembers(node = {}) {
  const members = node.groupedMembers || [];
  if (members.length < 2) return "";
  return `<details class="energy-path-group-members" data-energy-path-group-members>
    <summary>${escapeHTML(t("simulation.energyPathExpandMembers", {}, "Expand"))} · ${escapeHTML(String(members.length))}</summary>
    <p>${escapeHTML(t("simulation.energyPathGroupedMembersDescription", {}, "Original contributions remain available here and in exports."))}</p>
    <ul>${members.map((member) => `<li data-energy-path-group-member="${escapeHTML(member.id || "")}">
      <strong>${escapeHTML(energyPathInspectorSafeLabel(member.label || member.kind, { sourceIds: member.sourceIds || [], ruleIds: [member.ruleId].filter(Boolean) }, t("simulation.energyPathInspectorContribution", {}, "Original contribution")))}</strong>
      <span>${escapeHTML(energyPathInspectorValueLabel(energyPathNodeNumber(member.value), energyPathFlowUnit(member.unit || node.unit, member.scaleDomain || node.scaleDomain)))}</span>
      <small>${escapeHTML([member.serviceKind, member.period, member.zoneName].filter(Boolean).join(" · "))}</small>
    </li>`).join("")}</ul>
  </details>`;
}

export function renderEnergyPathSupplyBreakdown(activities = []) {
  if (!(activities || []).length) return "";
  return `
    <section class="energy-path-supply-breakdown" data-energy-path-supply-breakdown role="region" aria-label="${escapeHTML(t("simulation.energyPathSupplyBreakdown", {}, "Supply breakdown"))}">
      <header>
        <strong>${escapeHTML(t("simulation.energyPathSupplyBreakdown", {}, "Supply breakdown"))}</strong>
        <span>${escapeHTML(t("simulation.energyPathAsAvailable", {}, "Reported values only"))}</span>
      </header>
      <p>${escapeHTML(t(
        "simulation.energyPathSupplyBreakdownDescription",
        {},
        "Supply, export, and storage values provide context. Storage charge remains in consumption and is not added again.",
      ))}</p>
      <dl>
        ${(activities || []).map((activity) => `<div
          data-energy-path-supply-kind="${escapeHTML(activity.kind)}"
          data-energy-path-supply-value="${escapeHTML(String(activity.value))}"
        >
          <dt>${escapeHTML(activity.label)}</dt>
          <dd>${escapeHTML(energyPathSummaryValueLabel(activity.value, activity.unit))}</dd>
        </div>`).join("")}
      </dl>
    </section>`;
}

export function renderEnergyPathCarrierReconciliation(quality = null) {
  if (!quality) return "";
  const formula = String(quality.formula || "").trim();
  const detail = [quality.basis, formula].filter(Boolean).join(" · ");
  return `
    <section
      class="energy-path-carrier-reconciliation ${escapeHTML(quality.status)}"
      data-energy-path-carrier-reconciliation="${escapeHTML(quality.carrier)}"
      data-energy-path-quality-basis="carrier_residual"
      role="region"
      aria-label="${escapeHTML(t("simulation.energyPathCarrierReconciliation", {}, "Carrier reconciliation"))}"
    >
      <header>
        <strong>${escapeHTML(t("simulation.energyPathCarrierReconciliation", {}, "Carrier reconciliation"))}</strong>
        ${renderEnergyPathCarrierResidualBadge(quality)}
      </header>
      <dl>
        <div data-energy-path-carrier-reconciliation-term="expected">
          <dt>${escapeHTML(quality.observedSubtotal
            ? t("simulation.energyPathObservedDirectUseSubtotal", {}, "Observed direct-use subtotal")
            : t("simulation.energyPathCarrierTotal", {}, "Facility carrier total"))}</dt>
          <dd>${escapeHTML(energyPathSummaryValueLabel(quality.expectedValue, quality.unit))}</dd>
        </div>
        <div data-energy-path-carrier-reconciliation-term="explained">
          <dt>${escapeHTML(t("simulation.energyPathClassifiedEndUses", {}, "Mapped end uses"))}</dt>
          <dd>${escapeHTML(energyPathSummaryValueLabel(quality.explainedValue, quality.unit))}</dd>
        </div>
        <div data-energy-path-carrier-reconciliation-term="residual">
          <dt>${escapeHTML(t("simulation.energyPathUnclassifiedResidual", {}, "Residual"))}</dt>
          <dd>${escapeHTML(energyPathSummaryValueLabel(quality.residualValue, quality.unit))}</dd>
        </div>
      </dl>
      <p>${escapeHTML(t(
        "simulation.energyPathCarrierResidualDescription",
        {},
        "Unclassified energy appears when a positive gap exceeds 2% of the energy-source total or the absolute reporting threshold.",
      ))}</p>
      ${detail ? `<small>${escapeHTML(detail)}</small>` : ""}
    </section>`;
}

function renderEnergyPathCorrespondenceActions(nodes = [], relations = [], selectedNode = {}) {
  const counterparts = energyPathCorrespondenceCounterparts(nodes, relations, selectedNode.id);
  if (!counterparts.length) return "";
  const selectedIsEndUse = selectedNode.level === "end_use";
  const action = selectedIsEndUse ? "related_thermal_effect" : "related_energy_use";
  const label = selectedIsEndUse
    ? t("simulation.energyPathRelatedThermalEffect", {}, "Related thermal effect")
    : t("simulation.energyPathRelatedEnergyUse", {}, "Related energy use");
  return `
    <section class="energy-path-correspondence-actions" data-energy-path-correspondence-actions>
      <h5>${escapeHTML(t("simulation.energyPathRelated", {}, "Related"))}</h5>
      ${counterparts.map((counterpart) => `
        <button
          class="energy-path-correspondence-action"
          type="button"
          data-energy-explanation-node="${escapeHTML(counterpart.id || "")}"
          data-energy-path-correspondence-action="${action}"
          data-energy-path-correspondence-target="${escapeHTML(counterpart.id || "")}"
        >
          <span>${escapeHTML(label)}</span>
          <strong>${escapeHTML(counterpart.label || counterpart.kind || counterpart.id || "")}</strong>
        </button>`).join("")}
    </section>`;
}

export function energyPathCorrespondenceCounterparts(nodes = [], relations = [], selectedID = "") {
  const byID = new Map();
  for (const pair of energyPathCorrespondencePairs(nodes, relations)) {
    if (pair.driverNode.id === selectedID) {
      byID.set(pair.endUseNode.id, pair.endUseNode);
    } else if (pair.endUseNode.id === selectedID) {
      byID.set(pair.driverNode.id, pair.driverNode);
    }
  }
  return [...byID.values()].sort((left, right) => String(left.id || "").localeCompare(String(right.id || "")));
}

export function energyPathCorrespondencePairs(nodes = [], relations = []) {
  const nodeByID = new Map((nodes || []).filter((node) => node?.id).map((node) => [node.id, node]));
  return (relations || []).flatMap((relation) => {
    if (energyPathToken(relation?.relation) !== "source_correspondence") return [];
    const fromNode = nodeByID.get(relation.fromId);
    const toNode = nodeByID.get(relation.toId);
    if (!fromNode || !toNode || fromNode.id === toNode.id) return [];
    const driverNode = fromNode.level === "driver" ? fromNode : toNode.level === "driver" ? toNode : null;
    const endUseNode = fromNode.level === "end_use" ? fromNode : toNode.level === "end_use" ? toNode : null;
    if (!driverNode || !endUseNode) return [];
    const driverCategory = energyPathToken(driverNode.driverCategory || driverNode.kind || "").replace(/^driver[._-]/, "");
    const endUse = energyPathCanonicalEndUse(endUseNode);
    const expectedEndUse = driverCategory === "internal.lighting"
      ? "lighting"
      : driverCategory === "internal.equipment"
        ? "equipment"
        : "";
    if (!expectedEndUse || endUse !== expectedEndUse) return [];
    return [{ relation, driverNode, endUseNode }];
  });
}

function energyPathSignedDriverValue(node = {}, magnitude = 0) {
  const sign = Number(node.signedValue) < 0 || energyPathToken(node.sign) === "negative" || energyPathToken(node.serviceKind) === "heating"
    ? -1
    : 1;
  return sign * Math.abs(Number(magnitude) || 0);
}

function renderEnergyPathAllocationExplanation(node = {}) {
  if (node.level !== "driver" || node.allocationApplied !== true) return "";
  const explanation = t(
    "simulation.energyPathAllocationExplanation",
    {},
    "Deterministic signed heat-balance share allocation (non-causal; not a direct causal decomposition).",
  );
  return `<p class="energy-path-allocation-explanation" data-energy-path-allocation-explanation="heat_balance_share">
    <strong>${escapeHTML(t("simulation.energyPathAllocationBasis", {}, "Allocation basis"))}</strong>
    <span>${escapeHTML(explanation)}</span>
  </p>`;
}

function renderEnergyPathOffsetEffects(node = {}, unit = "kWh thermal", options = {}) {
  const effects = (node.offsetEffects || [])
    .map((effect) => ({
      effectKind: energyPathToken(effect?.effectKind),
      targetService: energyPathToken(effect?.targetService),
      driverCategory: String(effect?.driverCategory || "").trim(),
      label: String(effect?.label || "").trim(),
      heatDirection: energyPathToken(effect?.heatDirection),
      rawValue: Math.abs(Number(effect?.rawValue) || 0),
      effectiveValue: Math.abs(Number(effect?.effectiveValue) || 0),
      unit: String(effect?.unit || unit),
      basis: String(effect?.basis || "signed_heat_balance_offset"),
      explanation: String(effect?.explanation || "").trim(),
    }))
    .filter((effect) => effect.effectKind && effect.targetService && (effect.rawValue > 0 || effect.effectiveValue > 0));
  if (!effects.length) return "";
  return `
    <section class="energy-path-offset-effects" data-energy-path-offset-effects>
      <header>
        <strong>${escapeHTML(t("simulation.energyPathOffsetEffects", {}, "Offset effects"))}</strong>
      </header>
      <p>${escapeHTML(t(
        "simulation.energyPathOffsetEffectsExplanation",
        {},
        "Opposite-sign heat-balance effects are non-additive, non-causal context, not avoided-load quantities. They are not added to the total and never create reverse main ribbons.",
      ))}</p>
      <dl>
        ${effects.map((effect) => {
          const targetLabel = effect.targetService === "heating"
            ? t("simulation.energyPathReducesHeating", {}, "Reduces heating")
            : t("simulation.energyPathReducesCooling", {}, "Reduces cooling");
          const category = effect.label || effect.driverCategory || effect.heatDirection || effect.effectKind;
          return `<div data-energy-path-offset-effect="${escapeHTML(effect.effectKind)}" data-energy-path-offset-target="${escapeHTML(effect.targetService)}" data-energy-path-offset-category="${escapeHTML(effect.driverCategory)}">
            <dt>${escapeHTML(`${category} · ${targetLabel}`)}</dt>
            <dd>
              ${escapeHTML(t("simulation.energyPathOffsetRaw", {}, "Raw"))}: ${escapeHTML(energyPathSummaryValueLabel(effect.rawValue, effect.unit))}
              · ${escapeHTML(t("simulation.energyPathOffsetEffective", {}, "After multiplier"))}: ${escapeHTML(energyPathSummaryValueLabel(effect.effectiveValue, effect.unit))}
            </dd>
            ${options.hideTechnical ? "" : `<small>${escapeHTML(effect.explanation)} <code>${escapeHTML(effect.basis)}</code></small>`}
          </div>`;
        }).join("")}
      </dl>
    </section>`;
}

function renderEnergyPathSimultaneousLoad(node = {}, unit = "kWh thermal", options = {}) {
  const metric = node?.simultaneousLoad;
  if (node.level !== "load" || metric?.available !== true) return "";
  const numerator = Math.max(0, Number(metric.numerator) || 0);
  const denominator = Math.max(0, Number(metric.denominator) || 0);
  const ratio = denominator > 0
    ? Math.max(0, Math.min(1, Number.isFinite(Number(metric.ratio)) ? Number(metric.ratio) : numerator / denominator))
    : 0;
  const metricUnit = String(metric.unit || unit);
  const basis = String(metric.basis || "simultaneous_min_over_max");
  return `
    <section
      class="energy-path-simultaneous-load"
      data-energy-path-simultaneous-load-ratio="${escapeHTML(basis)}"
      data-energy-path-simultaneous-load-numerator="${escapeHTML(String(numerator))}"
      data-energy-path-simultaneous-load-denominator="${escapeHTML(String(denominator))}"
      data-energy-path-simultaneous-load-value="${escapeHTML(String(ratio))}"
    >
      <header>
        <strong>${escapeHTML(t("simulation.energyPathSimultaneousLoadRatio", {}, "Simultaneous heating / cooling ratio"))}</strong>
        <span>${escapeHTML(energyPathPercentLabel(ratio))}</span>
      </header>
      <p>${escapeHTML(t(
        "simulation.energyPathSimultaneousLoadDefinition",
        {},
        "Sum of zone-month minimum cooling/heating loads divided by the sum of their maximum loads.",
      ))}</p>
      <dl>
        <div data-energy-path-simultaneous-load-term="numerator">
          <dt>${escapeHTML(t("simulation.energyPathSimultaneousLoadNumerator", {}, "Simultaneous load"))}</dt>
          <dd>${escapeHTML(energyPathSummaryValueLabel(numerator, metricUnit))}</dd>
        </div>
        <div data-energy-path-simultaneous-load-term="denominator">
          <dt>${escapeHTML(t("simulation.energyPathSimultaneousLoadDenominator", {}, "Larger service load"))}</dt>
          <dd>${escapeHTML(energyPathSummaryValueLabel(denominator, metricUnit))}</dd>
        </div>
      </dl>
      ${options.hideTechnical ? "" : `<code>${escapeHTML(basis)}</code>`}
    </section>`;
}

function renderEnergyPathLoadBreakdown(node = {}, unit = "kWh thermal", validatedRows = null, allowLatentShare = true) {
  if (node.level !== "load") return "";
  const components = (validatedRows || node.loadBreakdown || [])
    .map((component) => ({
      component: energyPathToken(component?.component || component?.key),
      value: energyPathNodeNumber(component?.value),
      share: energyPathNodeNumber(component?.share),
    }))
    .filter((component) => component.component);
  const latentShare = allowLatentShare ? energyPathLoadLatentShare(node, components.filter((component) => component.value !== null)) : 0;
  if (!components.length && latentShare <= 0) return "";
  const significant = latentShare + 1e-9 >= 0.1;
  const badge = significant ? renderEnergyPathLatentBadge(latentShare, "inspector") : "";
  return `
    <section class="energy-path-load-breakdown" data-energy-path-load-breakdown>
      <header>
        <strong>${escapeHTML(t("simulation.energyPathLoadBreakdown", {}, "Sensible / latent breakdown"))}</strong>
        ${badge}
      </header>
      <dl>
        ${components.map((component) => {
          const share = component.share !== null && component.share >= 0
            ? component.share
            : component.value !== null && energyPathNodeNumber(node.value) > 0
              ? Math.abs(component.value) / Math.abs(Number(node.value)) : null;
          const emphasized = component.component === "latent" && significant;
          const label = component.component === "latent"
            ? t("simulation.energyPathLatentLoad", {}, "Latent")
            : component.component === "sensible"
              ? t("simulation.energyPathSensibleLoad", {}, "Sensible")
              : component.component;
          return `<div data-energy-path-load-breakdown-component="${escapeHTML(component.component)}" data-energy-path-load-breakdown-emphasized="${emphasized ? "true" : "false"}">
            <dt>${escapeHTML(label)}</dt>
            <dd>${escapeHTML(energyPathInspectorValueLabel(component.value, unit))}${share !== null ? ` · ${escapeHTML(energyPathPercentLabel(share))}` : ""}</dd>
          </div>`;
        }).join("")}
      </dl>
    </section>`;
}

function energyPathLoadLatentShare(node = {}, components = node.loadBreakdown || []) {
  const explicit = Number(node.latentShare);
  if (Number.isFinite(explicit) && explicit > 0) return explicit;
  const latent = (components || []).find((component) => energyPathToken(component?.component) === "latent");
  const total = Math.abs(Number(node.value) || 0);
  return latent && total > 0 && Number.isFinite(Number(latent.value))
    ? Math.abs(Number(latent.value)) / total
    : 0;
}

function energyPathPercentLabel(value) {
  const percent = Math.max(0, Number(value) || 0) * 100;
  return `${percent.toLocaleString(undefined, { maximumFractionDigits: 1 })}%`;
}

function renderEnergyPathLatentBadge(latentShare, location = "node") {
  const label = t(
    "simulation.energyPathLatentShareSignificant",
    { share: energyPathPercentLabel(latentShare) },
    `Latent ${energyPathPercentLabel(latentShare)}`,
  );
  return `<small class="energy-path-load-latent-badge" data-energy-path-load-latent-badge="${escapeHTML(location)}" data-energy-path-load-latent-share="${escapeHTML(String(latentShare))}">${escapeHTML(label)}</small>`;
}

export function energyPathInspectorSources(explanation = {}, node = {}, viewState = {}) {
  const directIDs = new Set(node.sourceIds || []);
  const category = energyPathToken(node.driverCategory || (
    node.level === "load" && node.serviceKind ? `load.${node.serviceKind}` : ""
  ));
  const zoneScope = (viewState.simulationEnergyScopeKind || "building") === "zone";
  const wantedZone = energyPathToken(viewState.simulationEnergyZoneName);
  const selected = [];
  const seen = new Set();
  for (const source of explanation.sources || []) {
    if (!source?.id || seen.has(source.id)) {
      continue;
    }
    const section = energyPathSourceInspectorSection(source);
    const attachableSection = node.level === "driver"
      ? section === "context"
      : node.level === "load" && (section === "context" || section === "balance");
    const sourceZone = energyPathToken(source.zoneName);
    const zoneMatches = !zoneScope || sourceZone === wantedZone || (node.level === "driver" && !sourceZone);
    const contextMatch = attachableSection &&
      category && energyPathToken(source.driverCategory) === category &&
      zoneMatches;
    if (!directIDs.has(source.id) && !contextMatch) {
      continue;
    }
    selected.push(energyPathSourceForScope(source, viewState));
    seen.add(source.id);
  }
  return selected;
}

export function renderEnergyPathSourceDetails(sources = [], viewState = {}) {
  const sectionDefinitions = [
    ["breakdown", "simulation.energyPathInspectorBreakdown", "Breakdown"],
    ["context", "simulation.energyPathInspectorContext", "Context"],
    ["balance", "simulation.energyPathInspectorBalance", "Balance"],
  ];
  const grouped = new Map(sectionDefinitions.map(([section]) => [section, []]));
  for (const source of sources || []) {
    const section = energyPathSourceInspectorSection(source);
    grouped.get(section).push(source);
  }
  const groups = sectionDefinitions
    .map(([section, labelKey, label]) => ({ section, label: t(labelKey, {}, label), sources: grouped.get(section) }))
    .filter((group) => group.sources.length);
  if (!groups.length) {
    return "";
  }
  return `
    <section class="energy-path-inspector-sources" data-energy-path-inspector-sources>
      <h5>${escapeHTML(t("simulation.energyPathInspectorSources", {}, "Source details"))}</h5>
      ${groups.map((group) => `
        <section class="energy-path-inspector-source-group" data-energy-path-inspector-section="${group.section}">
          <header>
            <strong>${escapeHTML(group.label)}</strong>
            <span>${escapeHTML(String(group.sources.length))}</span>
          </header>
          <div class="energy-path-inspector-source-grid">
            ${group.sources.map((source) => renderEnergyPathInspectorSource(source, viewState)).join("")}
          </div>
        </section>`).join("")}
    </section>`;
}

function renderEnergyPathInspectorSource(source = {}, viewState = {}) {
  const component = String(source.driverComponent || "").trim();
  const componentLabel = energyPathSourceComponentLabel(component);
  const humidityDetail = energyPathHumidityDetailKind(component);
  const direction = String(source.heatDirection || "").trim();
  const formula = String(source.formula || "").trim();
  const allocationFormula = source.allocationApplied ? String(source.allocationFormula || "").trim() : "";
  const allocationExplanation = source.allocationApplied ? String(source.allocationExplanation || "").trim() : "";
  const inputSourceIDs = energyPathUniqueValues(source.inputSourceIds).join(", ");
  const relatedEntityIDs = energyPathUniqueValues(source.relatedEntityIds);
  const status = energyPathSourceDerivationStatus(source);
  const fields = [
    ["driverComponent", t("simulation.energyPathSourceComponent", {}, "Component"), componentLabel],
    ["heatDirection", t("simulation.energyPathSourceHeatDirection", {}, "Heat direction"), direction],
    ["formula", t("simulation.energyPathSourceFormula", {}, "Formula"), formula],
    ["allocationFormula", t("simulation.energyPathSourceAllocationFormula", {}, "Allocation formula"), allocationFormula],
    ["allocationExplanation", t("simulation.energyPathSourceAllocationExplanation", {}, "Allocation note"), allocationExplanation],
    ["multiplierApplication", t("simulation.energyPathMultiplierApplication", {}, "Multiplier application"), source.multiplierApplication],
    ["inputSourceIds", t("simulation.energyPathSourceInputs", {}, "Input sources"), inputSourceIDs],
    ["relatedEntityIds", t("simulation.energyPathSourceEntities", {}, "Related entities"), relatedEntityIDs],
  ].filter(([, , value]) => Array.isArray(value) ? value.length > 0 : Boolean(value));
  return `
    <article class="energy-path-inspector-source" data-energy-path-source="${escapeHTML(source.id || "")}" data-energy-path-source-status="${status}"${humidityDetail ? ` data-energy-path-humidity-detail="${humidityDetail}"` : ""}>
      <header>
        <strong>${escapeHTML(source.name || source.keyValue || source.id || t("simulation.energyPathSource", {}, "Source"))}</strong>
        ${status === "reported" ? "" : `<span>${escapeHTML(t(
          status === "fallback" ? "simulation.energyPathSourceFallback" : "simulation.energyPathSourceDerived",
          {},
          status === "fallback" ? "Fallback" : "Derived",
        ))}</span>`}
      </header>
      ${fields.length ? `<dl>${fields.map(([key, label, value]) => `
        <div data-energy-path-source-field="${key}">
          <dt>${escapeHTML(label)}</dt>
          <dd>${key === "relatedEntityIds"
            ? renderEnergyPathRelatedEntities(value, viewState)
            : escapeHTML(value)}</dd>
        </div>`).join("")}</dl>` : ""}
    </article>`;
}

function energyPathHumidityDetailKind(component = "") {
  const token = energyPathToken(component).replace(/[\s:/-]+/g, ".");
  if (token === "load.humidification" || token.endsWith(".humidification")) return "humidification";
  if (token === "load.dehumidification" || token.endsWith(".dehumidification")) return "dehumidification";
  return "";
}

function energyPathSourceComponentLabel(component = "") {
  switch (energyPathHumidityDetailKind(component)) {
    case "humidification":
      return t("simulation.energyPathHumidificationDetail", {}, "Humidification detail");
    case "dehumidification":
      return t("simulation.energyPathDehumidificationDetail", {}, "Dehumidification detail");
    default:
      return component;
  }
}

function renderEnergyPathRelatedEntities(entityIDs = [], viewState = {}) {
  const airCouplingIDs = new Set(
    (viewState.report?.geometry?.topology?.airCouplings || [])
      .map((coupling) => String(coupling?.id || "").trim())
      .filter(Boolean),
  );
  const zoneScope = viewState.simulationEnergyScopeKind === "zone";
  return energyPathUniqueValues(entityIDs).map((entityID) => {
    if (!zoneScope || !airCouplingIDs.has(entityID)) {
      return `<span class="energy-path-related-entity">${escapeHTML(entityID)}</span>`;
    }
    const label = t(
      "simulation.energyPathOpenTopologyAirCoupling",
      { id: entityID },
      `Open Topology air coupling ${entityID}`,
    );
    return `<button
      class="energy-path-related-entity energy-path-related-entity-action"
      type="button"
      data-energy-path-topology-air-coupling-id="${escapeHTML(entityID)}"
      data-entity-id="${escapeHTML(entityID)}"
      data-entity-kind="thermal_air_coupling"
      data-panel-target-id="${escapeHTML(entityID)}"
      aria-label="${escapeHTML(label)}"
    >${escapeHTML(entityID)}</button>`;
  }).join("");
}

export function renderEnergyPathWarnings(warnings = []) {
  const unique = [];
  const seen = new Set();
  for (const warning of warnings || []) {
    const message = String(warning?.message || warning || "").trim();
    if (!message) continue;
    const severity = ["error", "warning", "info"].includes(energyPathToken(warning?.severity))
      ? energyPathToken(warning.severity)
      : "warning";
    const code = String(warning?.code || "").trim();
    const key = `${severity}\u0000${code}\u0000${message}`;
    if (seen.has(key)) continue;
    seen.add(key);
    const unassignedBuildingHVACAuxiliary = isEnergyPathUnassignedBuildingHVACAuxiliaryItem(warning);
    const unassignedBuildingHVAC = isEnergyPathUnassignedBuildingHVACItem(warning);
    unique.push({
      severity,
      code,
      message: unassignedBuildingHVAC
        ? message.replace(/^unassigned building hvac(?: auxiliary)? energy\s*(?::|·|—|-)?\s*/i, "")
        : message,
      unassignedBuildingHVAC,
      unassignedBuildingHVACAuxiliary,
    });
  }
  if (!unique.length) return "";
  return `
    <section class="energy-path-warnings" data-energy-path-warnings role="status">
      <h4>${escapeHTML(t("simulation.energyPathQualityWarnings", {}, "Energy Path quality warnings"))}</h4>
      <ul>${unique.map((warning) => `
        <li data-energy-path-warning-severity="${warning.severity}"${warning.unassignedBuildingHVACAuxiliary ? ` data-energy-path-quality-detail="${ENERGY_PATH_UNASSIGNED_BUILDING_HVAC_AUXILIARY}"` : warning.unassignedBuildingHVAC ? ` data-energy-path-quality-detail="${ENERGY_PATH_UNASSIGNED_BUILDING_HVAC}"` : ""}>
          ${warning.code ? `<code>${escapeHTML(warning.code)}</code>` : ""}
          ${warning.unassignedBuildingHVACAuxiliary
            ? `<strong>${escapeHTML(t("simulation.energyPathUnassignedBuildingHVACAuxiliaryEnergy", {}, "Unassigned building HVAC auxiliary energy"))}</strong>`
            : warning.unassignedBuildingHVAC ? `<strong>${escapeHTML(t("simulation.energyPathUnassignedBuildingHVACEnergy", {}, "Unassigned building HVAC energy"))}</strong>` : ""}
          ${warning.message ? `<span>${escapeHTML(warning.message)}</span>` : ""}
        </li>`).join("")}</ul>
    </section>`;
}

export function isEnergyPathUnassignedBuildingHVACItem(item = {}) {
  return [item.code, item.id, item.kind, item.label, item.fromId, item.toId]
    .filter(Boolean)
    .map((value) => energyPathToken(value).replace(/[^a-z0-9]+/g, "_"))
    .some((value) => value.includes(ENERGY_PATH_UNASSIGNED_BUILDING_HVAC) || value.includes(ENERGY_PATH_UNASSIGNED_BUILDING_HVAC_AUXILIARY));
}

export function isEnergyPathUnassignedBuildingHVACAuxiliaryItem(item = {}) {
  return [item.code, item.id, item.kind, item.label, item.fromId, item.toId]
    .filter(Boolean)
    .map((value) => energyPathToken(value).replace(/[^a-z0-9]+/g, "_"))
    .some((value) => value.includes(ENERGY_PATH_UNASSIGNED_BUILDING_HVAC_AUXILIARY));
}

function energyPathSourceInspectorSection(source = {}) {
  const section = energyPathToken(source.inspectorSection);
  return section === "context" || section === "balance" ? section : "breakdown";
}

function energyPathSourceDerivationStatus(source = {}) {
  const text = energyPathToken([source.driverComponent, source.formula, source.explanation].filter(Boolean).join(" "));
  if (text.includes("fallback") || text.includes("unsplit") || text.includes("outdoor_air_minus_infiltration")) {
    return "fallback";
  }
  return source.sourceType === "derived_formula" || (!source.allocationApplied && source.formula) || (source.inputSourceIds || []).length ? "derived" : "reported";
}

function energyPathSourceForScope(source = {}, viewState = {}) {
  const wantedKind = (viewState.simulationEnergyScopeKind || "building") === "zone" ? "zone" : "building";
  const wantedZone = energyPathToken(viewState.simulationEnergyZoneName);
  const detail = (source.scopeDetails || []).find((candidate) => {
    const scope = candidate?.scope || {};
    if (energyPathToken(scope.kind) !== wantedKind) {
      return false;
    }
    return wantedKind !== "zone" || energyPathToken(scope.zoneName) === wantedZone;
  });
  return detail ? { ...source, ...detail } : source;
}

function energyPathSumSourceField(sources = [], field = "") {
  let found = false;
  const value = sources.reduce((total, source) => {
    if (!Object.prototype.hasOwnProperty.call(source, field) || !Number.isFinite(Number(source[field]))) {
      return total;
    }
    found = true;
    return total + Number(source[field]);
  }, 0);
  return found ? value : Number.NaN;
}

function energyPathInspectorNumber(item = {}, field = "", fallback = Number.NaN) {
  if (Object.prototype.hasOwnProperty.call(item, field) && Number.isFinite(Number(item[field]))) {
    return Number(item[field]);
  }
  if (Number.isFinite(Number(fallback))) {
    return Number(fallback);
  }
  return Number(item.value) || 0;
}

function energyPathInspectorMultiplier(node = {}, sources = [], raw = 0, effective = 0) {
  if (Object.prototype.hasOwnProperty.call(node, "multiplier") && Number.isFinite(Number(node.multiplier)) && Number(node.multiplier) > 0) {
    return Number(node.multiplier);
  }
  const source = sources.find((item) => Number.isFinite(Number(item.effectiveMultiplier)) && Number(item.effectiveMultiplier) > 0);
  if (source) {
    return Number(source.effectiveMultiplier);
  }
  return raw ? effective / raw : 1;
}

export function renderEnergyPathKPI(summary = {}, options = {}) {
  if (!isEnergyPathSummaryV2(summary) && !options.graph) {
    return "";
  }
  const knownZoneOnly = options.knownZoneOnly ?? energyPathZoneDirectCoverage(summary).limited;
  const labels = {
    total_site_energy: knownZoneOnly
      ? t("simulation.energyPathKnownZoneSiteEnergy", {}, "Known zone site energy")
      : t("simulation.energyPathTotalSiteEnergy", {}, "Total site energy"),
    cooling_load: t("simulation.energyPathCoolingLoad", {}, "Cooling load"),
    heating_load: t("simulation.energyPathHeatingLoad", {}, "Heating load"),
    coverage: t("simulation.energyPathKPICoverage", {}, "Energy-path coverage"),
  };
  return `<div class="simulation-energy-kpis energy-path-kpis">
    ${energyPathKPIItems(summary, options.graph || {}, { ...options, knownZoneOnly })
      .map((item) => {
        const partialSubtotal = item.id === "total_site_energy" && knownZoneOnly;
        const label = escapeHTML(labels[item.id] || item.label);
        const body = `<span class="energy-path-kpi-label">${label}</span>${item.id === "coverage"
          ? renderEnergyPathKPIBoundaries(item.coverage?.boundaries || item.boundaries)
          : `<strong>${escapeHTML(energyPathKPIFinite(item.value) ? energyPathSummaryValueLabel(item.value, item.unit) : t("common.notAvailable", {}, "—"))}</strong>`}
          ${renderEnergyPathKPIRatio(item)}
          ${partialSubtotal ? `<small class="energy-path-partial-coverage-badge">${escapeHTML(t("simulation.energyPathKnownOnly", {}, "Known only"))}</small>` : ""}`;
        const targets = (item.targets || []).filter((target) => target?.id);
        const targetAttributes = (target) => `data-energy-path-kpi-node="${escapeHTML(target.id)}" data-energy-path-kpi-service="${escapeHTML(target.serviceKind || "")}"`;
        let control;
        if (item.id === "coverage") {
          control = `<button type="button" class="energy-path-kpi-control" data-energy-path-kpi-details aria-controls="energyPathDataDetails" aria-expanded="${options.detailsOpen === true}">${body}</button>`;
        } else if (targets.length > 1) {
          control = `<details data-energy-path-kpi-chooser="${escapeHTML(item.id)}">
            <summary class="energy-path-kpi-control">${body}<small class="energy-path-kpi-choice-label">${escapeHTML(t("simulation.energyPathChooseGraphNode", {}, "Choose graph node"))}</small></summary>
            <ul>${targets.map((target) => {
              const repeatedLabel = targets.filter((candidate) => candidate.label === target.label).length > 1;
              const targetLabel = [target.label || target.id, repeatedLabel && target.label ? target.id : ""].filter(Boolean).join(" · ");
              return `<li><button type="button" ${targetAttributes(target)}>${escapeHTML(targetLabel)}</button></li>`;
            }).join("")}</ul>
          </details>`;
        } else {
          control = `<button type="button" class="energy-path-kpi-control" ${targets.length ? targetAttributes(targets[0]) : "disabled"}>${body}
            ${targets.length ? "" : `<small class="energy-path-kpi-choice-label">${escapeHTML(t("simulation.energyPathGraphNodeUnavailable", {}, "Graph node unavailable"))}</small>`}
          </button>`;
        }
        return `<div data-energy-path-kpi="${escapeHTML(item.id)}" data-energy-path-kpi-emphasized="${item.emphasized === true}"${partialSubtotal ? ' data-energy-path-value-scope="observed_direct_use_subtotal"' : ""}>${control}</div>`;
      })
      .join("")}
  </div>`;
}

function energyPathKPIFinite(value) {
  return typeof value === "number" && Number.isFinite(value);
}

function renderEnergyPathKPIRatio(item = {}) {
  if (!item.emphasized || !["cooling_load", "heating_load"].includes(item.id)) return "";
  const ratio = item.ratio;
  const available = energyPathKPIFinite(ratio?.value);
  return `<small class="energy-path-kpi-ratio" data-energy-path-kpi-ratio="${item.id === "cooling_load" ? "cooling" : "heating"}"
    ${available ? `data-energy-path-kpi-ratio-value="${escapeHTML(String(ratio.value))}"` : ""}
    data-energy-path-kpi-ratio-partial="${ratio?.partial === true}" data-energy-path-kpi-ratio-links="${escapeHTML(JSON.stringify(ratio?.linkIds || []))}">
    ${escapeHTML(t("simulation.energyPathKPILoadSiteRatio", {}, "Load/site ratio"))}: ${escapeHTML(available ? energyPathRatioValueLabel(ratio.value) : t("common.notAvailable", {}, "—"))}
    ${ratio?.partial ? `<span class="energy-path-kpi-ratio-note">${escapeHTML(t("simulation.energyPathKPIMatchedConversions", {}, "Partial overlap · matched conversions only"))}</span>` : ""}
  </small>`;
}

function renderEnergyPathKPIBoundaries(boundaries = []) {
  const labels = {
    driver_to_load: t("simulation.energyPathKPIDriverClosure", {}, "Drivers → loads"),
    end_use_to_carrier: t("simulation.energyPathKPICarrierClosure", {}, "End uses → sources"),
  };
  const statuses = {
    complete: ["QualityComplete", "complete"], partial: ["QualityPartial", "partial"],
    missing: ["QualityMissing", "missing"], unavailable: ["QualityUnavailable", "Unavailable"],
    not_requested: ["QualityNotRequested", "Not requested"], not_applicable: ["QualityNotApplicable", "Not applicable"],
    overmapped: ["QualityOvermapped", "Overmapped"],
  };
  return `<span class="energy-path-kpi-boundaries">${Object.keys(labels).map((id) => {
    const boundary = (boundaries || []).find((item) => item.id === id) || {};
    const status = statuses[boundary.status] ? boundary.status : "unavailable";
    const [statusKey, statusFallback] = statuses[status];
    const statusLabel = t(`simulation.energyPath${statusKey}`, {}, statusFallback);
    const available = ["complete", "partial", "overmapped"].includes(status) && energyPathKPIFinite(boundary.value) && boundary.value >= 0;
    const value = available ? `${boundary.value.toLocaleString(undefined, { maximumFractionDigits: 1 })}%` : t("common.notAvailable", {}, "—");
    return `<span data-energy-path-kpi-boundary="${id}" data-energy-path-kpi-boundary-status="${status}" ${available ? `data-energy-path-kpi-boundary-value="${escapeHTML(String(boundary.value))}"` : ""}>
      <span>${escapeHTML(labels[id])}</span><b>${escapeHTML(`${value} · ${statusLabel}`)}</b>
    </span>`;
  }).join("")}</span>`;
}

export function renderEnergyPathSummaryOverview(summary = {}) {
  if (!isEnergyPathSummaryV2(summary)) {
    return "";
  }
  const groups = energyPathSummaryGroups(summary);
  const zoneCoverage = energyPathZoneDirectCoverage(summary);
  const groupLabels = {
    drivers: t("simulation.energyPathSummaryDrivers", {}, "Drivers"),
    loads: t("simulation.energyPathSummaryLoads", {}, "Loads"),
    endUses: t("simulation.energyPathSummaryEndUses", {}, "End uses"),
    carriers: t("simulation.energyPathSummaryCarriers", {}, "Energy sources"),
    ratios: t("simulation.energyPathSummaryRatios", {}, "Ratios"),
    residuals: t("simulation.energyPathSummaryResiduals", {}, "Residuals"),
    topZones: t("simulation.energyPathSummaryTopZones", {}, "Top zones"),
  };
  const scope = summary.scope || {};
  return `
    <section class="energy-path-summary-overview">
      <div class="simulation-energy-block-head">
        <h4>${escapeHTML(t("simulation.energyPathAnnualSummary", {}, "Energy Path summary"))}</h4>
        <span>${escapeHTML([
          scope.kind || "building",
          scope.zoneName || "",
          summary.period || "annual",
          scope.aggregationBasis || "model_total",
        ].filter(Boolean).join(" · "))}</span>
      </div>
      <div class="energy-path-summary-grid">
        ${groups.map((group) => {
          const partialCarriers = group.key === "carriers" && zoneCoverage.limited;
          return `
          <article data-energy-path-summary-group="${escapeHTML(group.key)}"${partialCarriers ? ' data-energy-path-value-scope="observed_direct_use_subtotal"' : ""}>
            <header>
              <strong>${escapeHTML(partialCarriers
                ? t("simulation.energyPathKnownEnergySources", {}, "Known energy sources")
                : groupLabels[group.key] || group.label)}</strong>
              <span>${escapeHTML(group.items.length)}</span>
            </header>
            ${partialCarriers ? `<p class="energy-path-summary-coverage-note">${escapeHTML(t(
              "simulation.energyPathObservedDirectUseSubtotal",
              {},
              "Observed direct-use subtotal",
            ))}</p>` : ""}
            <div>
              ${group.items.length
                ? group.items.slice(0, 8).map((item) => renderEnergyPathSummaryItem(item, group.key === "ratios")).join("")
                : `<span class="energy-path-summary-empty">${escapeHTML(t("common.notAvailable", {}, "—"))}</span>`}
            </div>
          </article>`;
        }).join("")}
      </div>
    </section>`;
}

export function renderEnergyPathHeader(explanation = {}, viewState = {}, options = {}) {
  if (!options.scene) normalizeEnergyPathViewState(viewState, explanation);
  return `
    <div class="energy-path-header">
      <div class="energy-path-heading">
        <h4 title="${escapeHTML(t("simulation.energyPathDirection", {}, "Load drivers → Thermal load → End-use energy → Energy sources"))}">${escapeHTML(t("simulation.energyPathName", {}, "Energy Path"))}</h4>
      </div>
      ${options.fixedContext ? `<span class="energy-path-fixed-context" data-energy-path-fixed-context>${escapeHTML([
        t("simulation.energyPathScopeBuilding", {}, "Building"),
        t("simulation.periodAnnual", {}, "Annual"),
        t("common.all", {}, "All"),
      ].join(" · "))}</span>` : renderEnergyPathControls(explanation, viewState, options.scene?.controlOptions)}
    </div>`;
}

export function updateEnergyPathControlState(event, viewState = {}, explanation = {}) {
  const target = event?.target;
  if (!(target instanceof Element)) {
    return { handled: false, render: false };
  }
  const scope = target.closest("[data-simulation-energy-scope]");
  if (scope) {
    viewState.simulationEnergyScopeKind = scope.value === "zone" ? "zone" : "building";
    viewState.simulationEnergySelection = "";
    normalizeEnergyPathViewState(viewState, explanation);
    return { handled: true, render: true };
  }
  const zone = target.closest("[data-simulation-energy-zone-name]");
  if (zone) {
    if (event.type === "input") {
      return { handled: true, render: false };
    }
    const match = energyPathZoneNames(explanation)
      .find((name) => energyPathToken(name) === energyPathToken(zone.value));
    if (match) {
      viewState.simulationEnergyZoneName = match;
      viewState.simulationEnergySelection = "";
      normalizeEnergyPathViewState(viewState, explanation);
    }
    return { handled: true, render: true };
  }
  const period = target.closest("[data-simulation-energy-path-period]");
  if (period) {
    viewState.simulationEnergyPeriod = ENERGY_PATH_PERIODS.some((item) => item.value === period.value)
      ? period.value
      : "annual";
    viewState.simulationEnergySelection = "";
    normalizeEnergyPathViewState(viewState, explanation);
    return { handled: true, render: true };
  }
  const service = target.closest("[data-simulation-energy-service]");
  if (service) {
    const options = energyPathServiceOptions(explanation, viewState);
    viewState.simulationEnergyService = options.some((item) => item.value === service.value) ? service.value : "all";
    viewState.simulationEnergySelection = "";
    return { handled: true, render: true };
  }
  return { handled: false, render: false };
}

export function energyPathServiceOptions(explanation = {}, viewState = {}, preparedAllServiceGraph = null) {
  const graph = preparedAllServiceGraph || energyPathGraphForState(explanation, {
    ...viewState,
    simulationEnergyService: "all",
  });
  const present = new Set(
    [...graph.nodes, ...graph.links]
      .map(energyPathItemService)
      .filter(Boolean),
  );
  if (!present.size) {
    return [...ENERGY_PATH_SERVICES];
  }
  return ENERGY_PATH_SERVICES.filter((service) => service.value === "all" || present.has(service.value));
}

export function energyPathZoneNames(explanation = {}) {
  const results = energyPathZoneResults(explanation);
  const resultNames = results
    .map((result) => String(result?.scope?.zoneName || "").trim())
    .filter(Boolean);
  const available = Array.isArray(explanation.availableZones)
    ? explanation.availableZones.map((zone) => String(zone || "").trim()).filter(Boolean)
    : [];
  const resultTokens = new Set(resultNames.map(energyPathToken));
  const usableAvailable = available.filter((zone) => resultTokens.has(energyPathToken(zone)));
  const scoped = energyPathToken(explanation.scope?.kind) === "zone" && explanation.scope?.zoneName
    ? [String(explanation.scope.zoneName).trim()]
    : [];
  return energyPathUniqueNames([...usableAvailable, ...resultNames, ...scoped])
    .sort((left, right) => left.localeCompare(right));
}

export function energyPathGraphForState(explanation = {}, viewState = {}) {
  const scopedResult = energyPathResultForState(explanation, viewState);
  if (!scopedResult) {
    return { nodes: [], links: [], relations: [], warnings: [] };
  }
  const periodID = viewState.simulationEnergyPeriod || "annual";
  const period = (scopedResult.periods || [])
    .find((item) => energyPathToken(item?.id) === energyPathToken(periodID));
  let nodes;
  let links;
  let warnings;
  const periodHasGraph = Boolean((period?.nodes || []).length || (period?.links || []).length);
  if (period && (periodHasGraph || energyPathToken(periodID) !== "annual")) {
    nodes = [...(period.nodes || [])];
    links = [...(period.links || [])];
    warnings = [...(period.warnings || [])];
  } else if (energyPathToken(periodID) === "annual") {
    nodes = [...(scopedResult.nodes || [])]
      .filter((node) => !node.period || energyPathToken(node.period) === "annual");
    links = [...(scopedResult.links || [])]
      .filter((link) => !link.period || energyPathToken(link.period) === "annual");
    warnings = [...(scopedResult.warnings || [])];
  } else {
    nodes = [...(scopedResult.nodes || [])]
      .filter((node) => energyPathToken(node.period) === energyPathToken(periodID));
    links = [...(scopedResult.links || [])]
      .filter((link) => energyPathToken(link.period) === energyPathToken(periodID));
    warnings = [];
  }

  ({ nodes, links } = energyPathApplyCarrierTaxonomy(nodes, links, explanation.sources || []));
  nodes = nodes.map(energyPathWithOriginalMembers);

  const scopeKind = viewState.simulationEnergyScopeKind || "building";
  if (scopeKind !== "zone") {
    nodes = nodes.filter((node) => !node.zoneName || node.aggregationBasis === "model_total");
    const nodeIDs = new Set(nodes.map((node) => node.id));
    links = links.filter((link) => nodeIDs.has(link.fromId) && nodeIDs.has(link.toId));
  } else {
    nodes = nodes.filter((node) => (
      !isEnergyPathUnassignedBuildingHVACItem(node) &&
      energyPathZoneDirectUseNodeIsTrusted(
        node,
        explanation.sources || [],
        viewState.simulationEnergyZoneName,
      )
    ));
    const nodeIDs = new Set(nodes.map((node) => node.id));
    links = links.filter((link) => (
      !isEnergyPathUnassignedBuildingHVACItem(link) &&
      nodeIDs.has(link.fromId) && nodeIDs.has(link.toId)
    ));
  }

  // Capture supply before end-use grouping folds storage charge into Other.
  // This context follows the selected scope and period, independent of service.
  const supplyActivities = energyPathSupplyActivities(nodes, links);
  const service = viewState.simulationEnergyService || "all";
  if (service !== "all") {
    const serviceLinks = links.filter((link) => energyPathItemService(link) === service);
    const linkedIDs = new Set(serviceLinks.flatMap((link) => [link.fromId, link.toId]).filter(Boolean));
    const supportLinks = links.filter((link) => energyPathToken(link.relation) === "support_supply" &&
      (linkedIDs.has(link.fromId) || linkedIDs.has(link.toId)));
    const supportIDs = new Set(supportLinks.flatMap((link) => [link.fromId, link.toId]));
    const visibleCarriers = new Set(nodes
      .filter((node) => node.level === "carrier" && linkedIDs.has(node.id))
      .map((node) => energyPathCanonicalCarrier(node.carrier || energyPathCarrierFromNodeID(node.id))));
    for (const node of nodes) {
      if (node.level === "support" && visibleCarriers.has(energyPathCanonicalCarrier(node.carrier) || "electricity")) {
        supportIDs.add(node.id);
      }
    }
    nodes = nodes.filter((node) => energyPathItemService(node) === service || linkedIDs.has(node.id) || supportIDs.has(node.id));
    const nodeIDs = new Set(nodes.map((node) => node.id));
    links = links.filter((link) => (
      (energyPathItemService(link) === service || supportLinks.includes(link)) &&
      nodeIDs.has(link.fromId) && nodeIDs.has(link.toId)
    ));
	} else {
		({ nodes, links } = energyPathMergeAllServiceDrivers(nodes, links));
  }

  ({ nodes, links } = energyPathProjectEndUsePresentation(nodes, links));
  ({ nodes, links } = energyPathGroupSmallNodes(nodes, links));
  nodes = nodes.map((node) => node.automaticOther ? {
    ...node,
    label: node.level === "driver"
      ? t("simulation.energyPathOtherDrivers", {}, "Other / storage")
      : t("simulation.energyPathEndUseOther", {}, "Other"),
  } : node);

  const nodeIDs = new Set(nodes.map((node) => node.id));
  const connectedLinks = links.filter((link) => nodeIDs.has(link.fromId) && nodeIDs.has(link.toId));
  return {
    nodes,
    links: connectedLinks.filter((link) => !isEnergyPathNonFlowRelation(link)),
    relations: connectedLinks.filter(isEnergyPathNonFlowRelation),
    supplyActivities: supplyActivities.filter((activity) => activity.nodeIds.some((id) => nodeIDs.has(id))),
    warnings,
  };
}

function energyPathApplyCarrierTaxonomy(nodes = [], links = [], sources = []) {
  const copiedNodes = (nodes || []).map((node) => ({ ...node }));
  const copiedLinks = (links || []).map((link) => ({ ...link }));
  const sourceByID = new Map((sources || []).filter((source) => source?.id).map((source) => [source.id, source]));
  const disallowed = new Set();
  const nodeByID = new Map(copiedNodes.filter((node) => node?.id).map((node) => [node.id, node]));

  for (const node of copiedNodes) {
    if (!["end_use", "carrier", "support"].includes(node.level)) continue;
    const normalized = energyPathEnergyUnitNormalization(node.unit);
    if (!normalized) {
      if (String(node.unit || "").trim()) disallowed.add(node.id);
      continue;
    }
    energyPathScaleNode(node, normalized.factor);
    node.unit = "kWh";
  }
  for (const link of copiedLinks) {
    const fromUnit = energyPathEnergyUnitNormalization(link.fromUnit);
    const toUnit = energyPathEnergyUnitNormalization(link.toUnit);
    if (fromUnit) {
      link.fromValue = energyPathScaledNumber(link.fromValue, fromUnit.factor);
      link.fromUnit = "kWh";
    }
    if (toUnit) {
      link.toValue = energyPathScaledNumber(link.toValue, toUnit.factor);
      link.toUnit = "kWh";
    }
  }

  for (const node of copiedNodes) {
    if (node.level !== "carrier") continue;
    const carrierEvidence = node.carrier || energyPathCarrierFromNodeID(node.id);
    if (!carrierEvidence) {
      disallowed.add(node.id);
      continue;
    }
    const carrier = energyPathCanonicalCarrier(carrierEvidence);
    const presentation = ENERGY_PATH_CARRIER_PRESENTATION[carrier];
    if (!presentation) {
      disallowed.add(node.id);
      continue;
    }
    if (carrier === "water" && !energyPathWaterHasExplicitSiteEnergyConversion(node, sourceByID)) {
      disallowed.add(node.id);
      continue;
    }
    if (presentation) {
      node.carrier = carrier;
      node.label = presentation.label;
      node.scaleDomain = "site";
      node.unit = "kWh";
    }
  }

  const carrierLinks = copiedLinks.filter((link) => (
    ["end_use_to_carrier", "direct_end_use_to_carrier"].includes(energyPathToken(link?.relation)) &&
    nodeByID.get(link.fromId)?.level === "end_use" &&
    nodeByID.get(link.toId)?.level === "carrier"
  ));
  const linksByEndUse = new Map();
  for (const link of carrierLinks) {
    const items = linksByEndUse.get(link.fromId) || [];
    items.push(link);
    linksByEndUse.set(link.fromId, items);
  }
  for (const [endUseID, endUseLinks] of linksByEndUse) {
    if (endUseLinks.length && endUseLinks.every((link) => disallowed.has(link.toId))) {
      disallowed.add(endUseID);
    }
  }

  const filteredNodes = copiedNodes
    .filter((node) => !disallowed.has(node.id))
    .map((node) => {
      if (["end_use", "support"].includes(node.level) && energyPathCanonicalUnit(node.unit) === "kWh") {
        node.unit = "kWh";
      }
      return node;
    });
  const visibleNodeIDs = new Set(filteredNodes.map((node) => node.id));
  const filteredLinks = copiedLinks
    .filter((link) => visibleNodeIDs.has(link.fromId) && visibleNodeIDs.has(link.toId))
    .map((link) => {
      const fromNode = nodeByID.get(link.fromId);
      const toNode = nodeByID.get(link.toId);
      if (fromNode?.unit) link.fromUnit = energyPathCanonicalUnit(fromNode.unit);
      if (toNode?.unit) link.toUnit = energyPathCanonicalUnit(toNode.unit);
      return link;
    });
  return { nodes: filteredNodes, links: filteredLinks };
}

function energyPathCarrierFromNodeID(id = "") {
  const parts = String(id || "").split(".");
  return energyPathToken(parts[0]) === "carrier" ? parts[1] || "" : "";
}

function energyPathCanonicalCarrier(value = "") {
  const compact = String(value || "").trim().toLowerCase().replace(/[^a-z0-9]+/g, "");
  return ENERGY_PATH_CARRIER_ALIASES[compact] || "";
}

function energyPathCanonicalUnit(value = "") {
  const trimmed = String(value || "").trim();
  const compact = trimmed.toLowerCase().replace(/[\s_-]+/g, "");
  return compact === "kwh" || compact === "kwhsite" || compact === "kwhthermal" ? "kWh" : trimmed;
}

function energyPathEnergyUnitNormalization(value = "") {
  const compact = String(value || "").trim().toLowerCase().replace(/[\s_-]+/g, "");
  const factors = {
    j: 1 / 3600000,
    kj: 1 / 3600,
    mj: 1 / 3.6,
    gj: 277.7777777778,
    wh: 1 / 1000,
    kwh: 1,
    kwhsite: 1,
  };
  return Object.prototype.hasOwnProperty.call(factors, compact) ? { factor: factors[compact], unit: "kWh" } : null;
}

function energyPathScaleNode(node = {}, factor = 1) {
  for (const field of ["value", "signedValue", "rawValue", "effectiveValue", "allocatedValue", "displayValue"]) {
    if (Object.prototype.hasOwnProperty.call(node, field)) {
      node[field] = energyPathScaledNumber(node[field], factor);
    }
  }
}

function energyPathScaledNumber(value, factor) {
  const number = energyPathNodeNumber(value);
  if (number === null) return value;
  return Math.round(number * factor * 1e9) / 1e9;
}

function energyPathWaterHasExplicitSiteEnergyConversion(node = {}, sourceByID = new Map()) {
  if (energyPathToken(node.basis) !== "derived_ratio" || energyPathCanonicalUnit(node.unit) !== "kWh") return false;
  const sourceIDs = node.sourceIds || [];
  if (!sourceIDs.length) return false;
  return sourceIDs.every((sourceID) => {
    const source = sourceByID.get(sourceID) || {};
    const normalizedUnit = energyPathCanonicalUnit(source.normalizedUnit);
    return Boolean(
      energyPathIsWaterVolumeUnit(source.sourceUnit) && normalizedUnit === "kWh" &&
      String(source.formula || "").trim() && energyPathToken(source.inspectorSection) === "context"
    );
  });
}

function energyPathIsWaterVolumeUnit(value = "") {
  const compact = String(value || "").trim().toLowerCase().replace(/[^a-z0-9³]+/g, "");
  return [
    "m3", "m³", "cubicmeter", "cubicmeters", "cubicmetre", "cubicmetres",
    "ft3", "ft³", "cubicfoot", "cubicfeet", "l", "liter", "liters", "litre", "litres",
  ].includes(compact);
}

export function isEnergyPathNonFlowRelation(link = {}) {
  return energyPathToken(link.relation) === "source_correspondence";
}

export function energyPathZoneDirectUseNodeIsTrusted(node = {}, sources = [], zoneName = "") {
  if (node?.level !== "end_use" || !["lighting", "equipment"].includes(energyPathCanonicalEndUse(node))) {
    return true;
  }
  const basis = energyPathToken(node.basis);
  const hierarchy = energyPathToken(node.meterHierarchyLevel);
  const wantedZone = energyPathToken(zoneName || node.zoneName);
  const nodeZone = energyPathToken(node.zoneName);
  const sourceIDs = new Set(node.sourceIds || []);
  const nodeSources = (sources || []).filter((source) => sourceIDs.has(source?.id));
  const hasZoneVariable = nodeSources.some((source) => {
    const sourceZone = energyPathToken(source.zoneName || source.keyValue);
    const sourceType = energyPathToken(source.sourceType);
    return Boolean(wantedZone && sourceZone === wantedZone && !source.isMeter && sourceType.includes("variable"));
  });
  const hasOnlyUnscopedMeters = nodeSources.length > 0 && nodeSources.every((source) => {
    const sourceType = energyPathToken(source.sourceType);
    return (source.isMeter || sourceType.includes("meter")) && !String(source.zoneName || "").trim();
  });
  const contradictoryHierarchy = Boolean(hierarchy && hierarchy !== "zone_direct_use");
  if (basis === "direct_zone_energy" && !contradictoryHierarchy && !hasOnlyUnscopedMeters) {
    return true;
  }
  if (hierarchy === "zone_direct_use" && (hasZoneVariable || nodeZone === wantedZone)) {
    return true;
  }
  if (basis || hierarchy || hasOnlyUnscopedMeters) {
    return false;
  }
  // Keep old precomputed v2 fixtures with no provenance tokens readable. New
  // runtime payloads carry the exact direct_zone_energy/zone_direct_use pair.
  return true;
}

export function energyPathProjectEndUsePresentation(nodes = [], links = []) {
  const sourceNodes = (nodes || []).map(energyPathWithOriginalMembers)
    .sort((left, right) => String(left.id).localeCompare(String(right.id)));
  const sourceLinks = links || [];
  const rawEndUseIDs = new Set(sourceNodes.filter(energyPathUsesCanonicalEndUsePresentation).map((node) => node.id));
  if (!rawEndUseIDs.size) {
    return { nodes: sourceNodes, links: sourceLinks };
  }

  const oldToNewID = new Map();
  const groupedNodes = new Map();
  for (const node of sourceNodes) {
    if (!rawEndUseIDs.has(node?.id)) {
      continue;
    }
    if (Math.abs(Number(node.value) || 0) <= 0) {
      continue;
    }
    const taxonomy = energyPathCanonicalEndUse(node);
    const presentation = ENERGY_PATH_END_USE_PRESENTATION[taxonomy] || ENERGY_PATH_END_USE_PRESENTATION.other;
    const id = `end_use.${presentation.key}.${energyPathEndUseScopeToken(node)}`;
    oldToNewID.set(node.id, id);
    const current = groupedNodes.get(id);
    if (!current) {
      groupedNodes.set(id, {
        ...node,
        id,
        kind: `energy.${presentation.key}`,
        label: t(presentation.labelKey, {}, presentation.label),
        endUse: presentation.key,
        carrier: "",
        sourceIds: energyPathUniqueValues(node.sourceIds).sort(),
        relatedEntityIds: energyPathUniqueValues(node.relatedEntityIds).sort(),
        relatedPathIds: energyPathUniqueValues(node.relatedPathIds).sort(),
        presentationEndUses: [taxonomy],
      });
      continue;
    }
    mergeEnergyPathPresentationNode(current, node, taxonomy);
  }

  const projectedEndUses = [...groupedNodes.values()].sort((left, right) => {
    const leftOrder = ENERGY_PATH_END_USE_ORDER[energyPathToken(left.endUse)] ?? Number.MAX_SAFE_INTEGER;
    const rightOrder = ENERGY_PATH_END_USE_ORDER[energyPathToken(right.endUse)] ?? Number.MAX_SAFE_INTEGER;
    return leftOrder - rightOrder || String(left.id || "").localeCompare(String(right.id || ""));
  });
  const projectedNodes = [
    ...sourceNodes.filter((node) => !rawEndUseIDs.has(node?.id)),
    ...projectedEndUses,
  ];
  const visibleNodeIDs = new Set(projectedNodes.map((node) => node.id));
  const projectedLinks = new Map();
  for (const link of sourceLinks) {
    if (rawEndUseIDs.has(link.fromId) && !oldToNewID.has(link.fromId)) {
      continue;
    }
    if (rawEndUseIDs.has(link.toId) && !oldToNewID.has(link.toId)) {
      continue;
    }
    const projected = {
      ...link,
      fromId: oldToNewID.get(link.fromId) || link.fromId,
      toId: oldToNewID.get(link.toId) || link.toId,
      sourceIds: energyPathUniqueValues(link.sourceIds).sort(),
      relatedPathIds: energyPathUniqueValues(link.relatedPathIds).sort(),
      groupedMembers: energyPathLinkAllocationMembers(link),
    };
    if (!visibleNodeIDs.has(projected.fromId) || !visibleNodeIDs.has(projected.toId)) {
      continue;
    }
    const key = [projected.fromId, projected.toId, projected.relation || "flow"].join("|");
    const current = projectedLinks.get(key);
    if (!current) {
      projected.id = energyPathPresentationLinkID(projected);
      projectedLinks.set(key, projected);
      continue;
    }
    mergeEnergyPathPresentationLink(current, projected);
  }
  const presentationLinks = synchronizeEnergyPathPresentationNonFlowLinks(
    projectedNodes,
    [...projectedLinks.values()],
  );
  return {
    nodes: projectedNodes,
    links: presentationLinks.sort((left, right) => String(left.id || "").localeCompare(String(right.id || ""))),
  };
}

function energyPathUsesCanonicalEndUsePresentation(node = {}) {
  if (node?.level !== "end_use") {
    return false;
  }
  const id = energyPathToken(node.id);
  const kind = energyPathToken(node.kind);
  return Boolean(energyPathToken(node.endUse)) || id.startsWith("end_use.") || kind.startsWith("energy.") || kind.startsWith("end_use.");
}

function energyPathCanonicalEndUse(node = {}) {
  let token = energyPathToken(node.endUse || node.kind || "");
  token = token.replace(/^energy[._-]/, "").replace(/^end_use[._-]/, "");
  switch (token) {
    case "cooling":
    case "heating":
    case "fans":
    case "pumps":
    case "heat_rejection":
    case "humidification":
    case "heat_recovery":
    case "lighting":
    case "equipment":
    case "water_systems":
    case "refrigeration":
    case "other":
      return token;
    case "heatrejection":
      return "heat_rejection";
    case "humidifier":
      return "humidification";
    case "heatrecovery":
      return "heat_recovery";
    case "interior_lighting":
    case "interior_lights":
    case "interiorlighting":
    case "interiorlights":
    case "exterior_lighting":
    case "exterior_lights":
    case "exteriorlighting":
    case "exteriorlights":
    case "lights":
      return "lighting";
    case "interior_equipment":
    case "interiorequipment":
    case "exterior_equipment":
    case "exteriorequipment":
      return "equipment";
    case "watersystems":
    case "dhw":
      return "water_systems";
    default:
      return "other";
  }
}

function energyPathEndUseScopeToken(node = {}) {
  const parts = String(node.id || "").split(".").filter(Boolean);
  if (parts.length >= 3 && energyPathToken(parts[0]) === "end_use") {
    return metricTokenForEnergyPath(parts.at(-1));
  }
  return metricTokenForEnergyPath(node.zoneName || "building");
}

function metricTokenForEnergyPath(value = "") {
  return energyPathToken(value).replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "") || "building";
}

function mergeEnergyPathPresentationNode(current, next, taxonomy) {
  mergeEnergyPathOriginalMembers(current, next);
  for (const field of ["value", "signedValue", "rawValue", "effectiveValue", "allocatedValue", "displayValue"]) {
    if (Object.prototype.hasOwnProperty.call(current, field) || Object.prototype.hasOwnProperty.call(next, field)) {
      current[field] = (Number(current[field]) || 0) + (Number(next[field]) || 0);
    }
  }
  if (Number(current.rawValue)) {
    current.multiplier = Number(current.effectiveValue) / Number(current.rawValue);
  }
  current.sourceIds = energyPathUniqueValues([...(current.sourceIds || []), ...(next.sourceIds || [])]).sort();
  current.relatedEntityIds = energyPathUniqueValues([...(current.relatedEntityIds || []), ...(next.relatedEntityIds || [])]).sort();
  current.relatedPathIds = energyPathUniqueValues([...(current.relatedPathIds || []), ...(next.relatedPathIds || [])]).sort();
  current.presentationEndUses = energyPathUniqueValues([...(current.presentationEndUses || []), taxonomy]).sort();
  if (current.serviceKind !== next.serviceKind) {
    current.serviceKind = "";
  }
  if (current.basis !== next.basis) {
    current.basis = "grouped_presentation";
  }
}

function energyPathLinkAllocationMembers(link) {
  return (link.groupedMembers?.length ? link.groupedMembers : [link]).map((member) => ({
    ...member,
    sourceIds: [...(member.sourceIds || [])],
    badges: [...(member.badges || [])],
  }));
}

function mergeEnergyPathPresentationLink(current, next) {
  current.groupedMembers = [...energyPathLinkAllocationMembers(current), ...energyPathLinkAllocationMembers(next)];
  const nonFlow = isEnergyPathNonFlowRelation(current) && isEnergyPathNonFlowRelation(next);
  if (!nonFlow) {
    for (const field of ["fromValue", "toValue", "value", "signedValue", "displayValue"]) {
      if (Object.prototype.hasOwnProperty.call(current, field) || Object.prototype.hasOwnProperty.call(next, field)) {
        current[field] = (Number(current[field]) || 0) + (Number(next[field]) || 0);
      }
    }
  }
  current.sourceIds = energyPathUniqueValues([...(current.sourceIds || []), ...(next.sourceIds || [])]).sort();
  current.relatedPathIds = energyPathUniqueValues([...(current.relatedPathIds || []), ...(next.relatedPathIds || [])]).sort();
  if (current.serviceKind !== next.serviceKind) {
    current.serviceKind = "";
  }
  if (current.basis !== next.basis) {
    current.basis = "grouped_presentation";
  }
  if (current.ruleId !== next.ruleId) {
    current.ruleId = "";
  }
  if (nonFlow) {
    current.ratio = 0;
    current.ratioKind = "";
    current.ratioLabel = "";
  } else if (current.ratioKind && current.ratioKind === next.ratioKind && Number(current.toValue)) {
    current.ratio = Number(current.fromValue) / Number(current.toValue);
  } else if (current.ratioKind !== next.ratioKind) {
    current.ratio = 0;
    current.ratioKind = "";
    current.ratioLabel = "";
  }
}

function synchronizeEnergyPathPresentationNonFlowLinks(nodes = [], links = []) {
  const nodeByID = new Map((nodes || []).filter((node) => node?.id).map((node) => [node.id, node]));
  for (const link of links || []) {
    if (!isEnergyPathNonFlowRelation(link)) continue;
    const fromNode = nodeByID.get(link.fromId);
    const toNode = nodeByID.get(link.toId);
    if (fromNode) {
      link.fromValue = Math.abs(Number(fromNode.value) || 0);
      link.fromUnit = fromNode.unit || link.fromUnit || "";
    }
    if (toNode) {
      link.toValue = Math.abs(Number(toNode.value) || 0);
      link.toUnit = toNode.unit || link.toUnit || "";
    }
    link.ratio = 0;
    link.ratioKind = "";
    link.ratioLabel = "";
  }
  return links;
}

function energyPathPresentationLinkID(link = {}) {
  return `link.${metricTokenForEnergyPath(link.fromId)}.${metricTokenForEnergyPath(link.toId)}.${metricTokenForEnergyPath(link.relation || "flow")}`;
}

export function energyPathMergeAllServiceDrivers(nodes = [], links = []) {
	const canonicalDrivers = (nodes || []).map(energyPathWithOriginalMembers).filter((node) => (
		node.level === "driver" &&
		Math.abs(Number(node.value) || 0) > 0 &&
		Object.prototype.hasOwnProperty.call(ENERGY_PATH_DRIVER_ORDER, energyPathToken(node.driverCategory))
	)).sort((left, right) => String(left.id).localeCompare(String(right.id)));
	if (!canonicalDrivers.length) {
		return { nodes, links };
	}
	const targetByCategory = new Map([["internal.other", "balance.storage_other"]]);

	const driverByID = new Map();
	const oldToNewID = new Map();
	for (const node of canonicalDrivers) {
		const sourceCategory = energyPathToken(node.driverCategory);
		const category = targetByCategory.get(sourceCategory) || sourceCategory;
		const parts = String(node.id || "").split(".");
		const scopeToken = parts.at(-1) || "building";
		const id = `driver.${category}.all.${scopeToken}`;
		oldToNewID.set(node.id, id);
		const current = driverByID.get(id);
		if (!current) {
			driverByID.set(id, {
				...node,
				id,
				kind: `driver.${category}`,
				driverCategory: category,
				serviceKind: "all",
				label: category === "balance.storage_other" ? "Other / storage" : node.label,
				sourceIds: energyPathUniqueValues(node.sourceIds),
				relatedEntityIds: energyPathUniqueValues(node.relatedEntityIds),
				relatedPathIds: energyPathUniqueValues(node.relatedPathIds),
			});
			continue;
		}
		mergeEnergyPathOriginalMembers(current, node);
		for (const key of ["value", "signedValue", "rawValue", "effectiveValue", "allocatedValue", "displayValue"]) {
			if (Object.prototype.hasOwnProperty.call(node, key) || Object.prototype.hasOwnProperty.call(current, key)) {
				current[key] = (Number(current[key]) || 0) + (Number(node[key]) || 0);
			}
		}
		if (Number(current.rawValue)) {
			current.multiplier = Number(current.effectiveValue) / Number(current.rawValue);
		}
		current.sourceIds = energyPathUniqueValues([...(current.sourceIds || []), ...(node.sourceIds || [])]);
		current.relatedEntityIds = energyPathUniqueValues([...(current.relatedEntityIds || []), ...(node.relatedEntityIds || [])]);
		current.relatedPathIds = energyPathUniqueValues([...(current.relatedPathIds || []), ...(node.relatedPathIds || [])]);
	}

	const canonicalIDs = new Set(canonicalDrivers.map((node) => node.id));
	const projectedNodes = [
		...(nodes || []).filter((node) => !canonicalIDs.has(node.id)),
		...driverByID.values(),
	];
	const linkByKey = new Map();
	for (const link of links || []) {
		const projected = {
			...link,
			fromId: oldToNewID.get(link.fromId) || link.fromId,
			toId: oldToNewID.get(link.toId) || link.toId,
		};
		const key = [projected.fromId, projected.toId, projected.relation || "", projected.basis || "", projected.serviceKind || ""].join("|");
		const current = linkByKey.get(key);
		if (!current) {
			projected.id = `link.${projected.fromId}.${projected.toId}.${projected.serviceKind || "all"}`;
			projected.sourceIds = energyPathUniqueValues(projected.sourceIds);
			projected.relatedPathIds = energyPathUniqueValues(projected.relatedPathIds);
			linkByKey.set(key, projected);
			continue;
		}
		if (!isEnergyPathNonFlowRelation(current) || !isEnergyPathNonFlowRelation(projected)) {
			for (const field of ["fromValue", "toValue", "value", "signedValue", "displayValue"]) {
				if (Object.prototype.hasOwnProperty.call(projected, field) || Object.prototype.hasOwnProperty.call(current, field)) {
					current[field] = (Number(current[field]) || 0) + (Number(projected[field]) || 0);
				}
			}
		}
		current.sourceIds = energyPathUniqueValues([...(current.sourceIds || []), ...(projected.sourceIds || [])]);
		current.relatedPathIds = energyPathUniqueValues([...(current.relatedPathIds || []), ...(projected.relatedPathIds || [])]);
	}
	return { nodes: projectedNodes, links: [...linkByKey.values()] };
}

function energyPathUniqueValues(values = []) {
	return [...new Set((values || []).filter(Boolean))];
}

export function energyPathResultForState(explanation = {}, viewState = {}) {
  if ((viewState.simulationEnergyScopeKind || "building") !== "zone") {
    return explanation;
  }
  const wantedZone = energyPathToken(viewState.simulationEnergyZoneName);
  if (!wantedZone) {
    return null;
  }
  const nested = energyPathZoneResults(explanation)
    .find((result) => energyPathToken(result?.scope?.zoneName) === wantedZone);
  if (nested) {
    return nested;
  }
  if (
    energyPathToken(explanation.scope?.kind) === "zone" &&
    energyPathToken(explanation.scope?.zoneName) === wantedZone
  ) {
    return explanation;
  }
  return null;
}

export function energyPathSummaryForState(explanation = {}, fallbackSummary = {}, viewState = {}) {
  const scopedResult = energyPathResultForState(explanation, viewState);
  if (!scopedResult) {
    return {};
  }
  const periodID = viewState.simulationEnergyPeriod || "annual";
  const period = (scopedResult.periods || [])
    .find((item) => energyPathToken(item?.id) === energyPathToken(periodID));
  const candidate = energyPathToken(periodID) === "annual"
    ? scopedResult.summary || fallbackSummary
    : period?.summary;
  if (!isEnergyPathSummaryV2(candidate)) {
    return {};
  }

  const wantedScopeKind = (viewState.simulationEnergyScopeKind || "building") === "zone" ? "zone" : "building";
  const candidateScope = candidate.scope?.kind ? candidate.scope : scopedResult.scope;
  const candidateScopeKind = energyPathToken(candidateScope?.kind) || "building";
  if (candidateScopeKind !== wantedScopeKind) {
    return {};
  }
  if (
    wantedScopeKind === "zone" &&
    energyPathToken(candidateScope?.zoneName) !== energyPathToken(viewState.simulationEnergyZoneName)
  ) {
    return {};
  }
  const candidatePeriod = candidate.period || periodID;
  if (energyPathToken(candidatePeriod) !== energyPathToken(periodID)) {
    return {};
  }

  const completeness = candidate.completeness && Object.keys(candidate.completeness).length
    ? candidate.completeness
    : period?.completeness || scopedResult.completeness || {};
  const endUses = wantedScopeKind === "zone"
    ? (candidate.endUses || []).filter((item) => (
      !isEnergyPathUnassignedBuildingHVACItem(item) &&
      energyPathZoneDirectUseNodeIsTrusted(
        { ...item, level: item.level || "end_use" },
        explanation.sources || [],
        viewState.simulationEnergyZoneName,
      )
    ))
    : candidate.endUses;
  const zoneSafeItems = (items) => wantedScopeKind === "zone"
    ? (items || []).filter((item) => !isEnergyPathUnassignedBuildingHVACItem(item))
    : items;
  return {
    ...candidate,
    drivers: zoneSafeItems(candidate.drivers),
    loads: zoneSafeItems(candidate.loads),
    endUses,
    carriers: zoneSafeItems(candidate.carriers),
    ratios: zoneSafeItems(candidate.ratios),
    residuals: zoneSafeItems(candidate.residuals),
    topZones: zoneSafeItems(candidate.topZones),
    period: candidatePeriod,
    scope: candidateScope || { kind: wantedScopeKind },
    completeness,
  };
}

export function renderEnergyPathControls(explanation = {}, viewState = {}, preparedOptions = null) {
  const zones = preparedOptions?.zones || energyPathZoneNames(explanation);
  const rootIsZoneOnly = energyPathToken(explanation.scope?.kind) === "zone" && !energyPathZoneResults(explanation).length;
  const scopes = ENERGY_PATH_SCOPES.map((scope) => (
    `<option value="${scope.value}" ${viewState.simulationEnergyScopeKind === scope.value ? "selected" : ""} ${(scope.value === "zone" && !zones.length) || (scope.value === "building" && rootIsZoneOnly) ? "disabled" : ""}>${escapeHTML(t(scope.labelKey, {}, scope.label))}</option>`
  )).join("");
  const periods = ENERGY_PATH_PERIODS.map((period) => (
    `<option value="${period.value}" ${viewState.simulationEnergyPeriod === period.value ? "selected" : ""}>${escapeHTML(t(period.labelKey, {}, period.label))}</option>`
  )).join("");
  const services = (preparedOptions?.services || energyPathServiceOptions(explanation, viewState)).map((service) => (
    `<option value="${service.value}" ${viewState.simulationEnergyService === service.value ? "selected" : ""}>${escapeHTML(t(service.labelKey, {}, service.label))}</option>`
  )).join("");
  const zoneControl = viewState.simulationEnergyScopeKind === "zone"
    ? `<span class="energy-path-zone-control">
        <input type="search" list="simulationEnergyPathZones" value="${escapeHTML(viewState.simulationEnergyZoneName || "")}" data-simulation-energy-zone-name aria-label="${escapeHTML(t("simulation.energyPathZone", {}, "Zone"))}" autocomplete="off" />
        <datalist id="simulationEnergyPathZones">${zones.map((zone) => `<option value="${escapeHTML(zone)}"></option>`).join("")}</datalist>
      </span>`
    : "";
  return `
    <div class="energy-path-controls" aria-label="${escapeHTML(t("simulation.energyPathControls", {}, "Energy Path controls"))}">
      <label>
        <span>${escapeHTML(t("simulation.energyPathScope", {}, "Scope"))}</span>
        <span class="energy-path-control-fields">
          <select data-simulation-energy-scope aria-label="${escapeHTML(t("simulation.energyPathScope", {}, "Scope"))}">${scopes}</select>
          ${zoneControl}
        </span>
      </label>
      <label>
        <span>${escapeHTML(t("common.period", {}, "Period"))}</span>
        <select data-simulation-energy-path-period aria-label="${escapeHTML(t("common.period", {}, "Period"))}">${periods}</select>
      </label>
      <label>
        <span>${escapeHTML(t("simulation.service", {}, "Service"))}</span>
        <select data-simulation-energy-service aria-label="${escapeHTML(t("simulation.service", {}, "Service"))}">${services}</select>
      </label>
    </div>`;
}

export function energyPathRatioQualityForState(explanation = {}, viewState = {}) {
  const result = energyPathResultForState(explanation, viewState) || {};
  const periodID = viewState.simulationEnergyPeriod || "annual";
  const period = (result.periods || []).find((item) => energyPathToken(item.id) === energyPathToken(periodID));
  const quality = energyPathToken(periodID) === "annual"
    ? result.quality || period?.quality || result.summary?.quality || {}
    : period?.quality || period?.summary?.quality || {};
  // Do not turn an absent legacy marker into an explicit unavailable status.
  return quality.ratios;
}

export function renderEnergyPathGraph(scene, viewState = {}) {
  const { visibleNodes, layout, drawing, ratioQuality } = scene;
  const links = scene.graph.links;
  const { selectedID, relatedNodeIDs, focus } = energyPathSceneSelection(scene, viewState);
  const directLane = layout.lanes.find((lane) => lane.id === "direct");
  const directStart = layout.columns.find((column) => column.level === "end_use").x;
  const percent = (value) => `${value / layout.width * 100}%`;
  const stages = ENERGY_PATH_STAGES.map((stage) => {
    const column = layout.columns.find((item) => item.level === stage.level);
    const entries = layout.nodes.filter((item) => item.level === stage.level);
    const partialCarrierSubtotal = stage.level === "carrier" && entries.some((entry) => entry.node.presentationCoverage === "partial");
    const description = partialCarrierSubtotal
      ? t("simulation.energyPathObservedDirectUseSubtotalExplanation", {}, "Only directly observed zone energy uses are included; energy-source values are subtotals, not complete zone totals.")
      : t(stage.descriptionKey, {}, stage.description);
    const unitLabel = t(stage.scaleDomain === "thermal" ? "simulation.energyPathThermalUnit" : "simulation.energyPathSiteUnit", {}, stage.unitLabel);
    return `<article class="energy-path-stage ${stage.scaleDomain === "site" ? "site-domain" : "thermal-domain"}" data-energy-path-stage="${escapeHTML(stage.level)}"${partialCarrierSubtotal ? ' data-energy-path-value-scope="observed_direct_use_subtotal"' : ""} style="left:${percent(column.x)};width:${percent(column.width)}">
      <header title="${escapeHTML(description)}">
        <strong>${escapeHTML(t(stage.labelKey, {}, stage.label))}</strong>
        <span title="${escapeHTML(unitLabel)}">${escapeHTML(partialCarrierSubtotal
          ? t("simulation.energyPathKnownEnergySources", {}, "Known energy sources")
          : unitLabel)}</span>
      </header>
      <div class="energy-path-stage-nodes">${entries.length
        ? entries.map((entry) => renderEnergyPathNode(entry.node, stage, selectedID, relatedNodeIDs, { ...entry, column, focusKind: energyPathFocusKind(focus, "node", entry.id, true) })).join("")
        : `<span class="energy-path-stage-empty">${escapeHTML(t("common.notAvailable", {}, "—"))}</span>`}</div>
    </article>`;
  }).join("");
  return `<div class="energy-path-layout" data-energy-path-layout>
    <div class="energy-path-stage-grid" data-energy-path-canvas role="group" aria-label="${escapeHTML(t("simulation.energyPathDirection", {}, "Load drivers → Thermal load → End-use energy → Energy sources"))}" style="height:${layout.height}px">
      <svg class="energy-path-graph-underlay" data-energy-path-graph-underlay viewBox="0 0 ${layout.width} ${layout.height}" preserveAspectRatio="none" aria-hidden="true" focusable="false">
        ${renderEnergyPathRibbonGeometry(drawing, visibleNodes, focus)}
      </svg>
      ${directLane?.height > 0 ? `<div class="energy-path-direct-lane-band" data-energy-path-lane-band="direct" style="left:${percent(directStart)};width:${percent(layout.width - directStart)};top:${directLane.y}px;height:${directLane.height}px">
        <span>${escapeHTML(t("simulation.energyPathDirectAuxiliaryLane", {}, "Direct & auxiliary energy"))}</span>
      </div>` : ""}
      <div class="energy-path-plot-divider" data-energy-path-divider style="left:${percent(layout.dividerX)}"><span>${escapeHTML(t("simulation.energyPathConversion", {}, "Equipment conversion"))}</span></div>
      ${stages}
      ${renderEnergyPathBridgeRatios(drawing, layout, visibleNodes, links, ratioQuality, focus)}
      ${renderEnergyPathLinkHitLayer(drawing, layout, visibleNodes, focus)}
    </div>
  </div>`;
}

function energyPathFocusKind(focus, kind, id, includeCounterparts = false) {
  if (!focus?.active) return "inactive";
  if ((kind === "link" ? focus.linkIDs : focus.nodeIDs).has(id)) return "path";
  return includeCounterparts && focus.counterpartIDs.has(id) ? "counterpart" : "dimmed";
}

function renderEnergyPathRibbonGeometry(drawing, nodes, focus) {
  const nodeByID = new Map(nodes.map((node) => [node.id, node]));
  const ribbonPaints = drawing.ribbons.map((ribbon) => energyPathLinkAppearance(ribbon.link, nodes));
  const barPaints = drawing.bars.map((bar) => energyPathNodeAppearance(bar.node));
  const definitions = new Map([...ribbonPaints, ...barPaints]
    .filter((appearance) => appearance.allocated || appearance.residual)
    .map((appearance) => [energyPathPatternID(appearance), appearance]));
  const patterns = `<defs>${[...definitions].map(([id, appearance]) => `<pattern id="${id}" data-energy-path-pattern="${energyPathPaintKind(appearance)}" data-energy-path-color="${appearance.colorKey}" patternUnits="userSpaceOnUse" width="8" height="8">
    <rect class="energy-path-pattern-base" width="8" height="8"/>
    <path class="energy-path-pattern-stroke" d="M-2,2 L2,-2 M0,8 L8,0 M6,10 L10,6"/>
    ${appearance.residual ? '<path class="energy-path-pattern-stroke" d="M-2,6 L2,10 M0,0 L8,8 M6,-2 L10,2"/>' : ""}
  </pattern>`).join("")}</defs>`;
  const paths = drawing.ribbons.map((ribbon, index) => {
    const appearance = ribbonPaints[index];
    const kind = ribbon.domain === "conversion" ? "conversion" : "same_domain";
    const fromDomain = ribbon.domain === "conversion" ? "thermal" : ribbon.domain;
    const toDomain = ribbon.domain === "conversion" ? "site" : ribbon.domain;
    const title = [energyPathRibbonTitle(ribbon, nodeByID), energyPathAppearanceLabel(appearance)].filter(Boolean).join(" · ");
    return `<path class="energy-path-ribbon" data-energy-path-ribbon="${escapeHTML(ribbon.id)}" data-energy-path-ribbon-kind="${kind}" data-energy-path-ribbon-domain="${escapeHTML(ribbon.domain)}" data-energy-path-ribbon-relation="${escapeHTML(ribbon.relation)}" data-energy-path-focus="${energyPathFocusKind(focus, "link", ribbon.id)}"
      ${energyPathAppearanceAttributes(appearance)}${appearance.allocated || appearance.residual ? ` style="fill:url(#${energyPathPatternID(appearance)})"` : ""}
      data-from-value="${ribbon.fromValue}" data-to-value="${ribbon.toValue}" data-from-width="${ribbon.fromWidth}" data-to-width="${ribbon.toWidth}" data-from-domain="${fromDomain}" data-to-domain="${toDomain}"
      data-from-x="${ribbon.fromPort.x}" data-from-y0="${ribbon.fromPort.y0}" data-from-y1="${ribbon.fromPort.y1}" data-to-x="${ribbon.toPort.x}" data-to-y0="${ribbon.toPort.y0}" data-to-y1="${ribbon.toPort.y1}"
      d="${escapeHTML(ribbon.path)}"><title>${escapeHTML(title)}</title></path>`;
  }).join("");
  const bars = drawing.bars.map((bar, index) => {
    const appearance = barPaints[index];
    const title = [`${bar.node.label || bar.nodeId}: ${energyPathValueLabel(bar.value, bar.domain === "thermal" ? "kWh thermal" : "kWh site")}`, energyPathAppearanceLabel(appearance)].filter(Boolean).join(" · ");
    return `<rect class="energy-path-quantitative-bar" data-energy-path-bar="${escapeHTML(bar.nodeId)}" data-energy-path-bar-side="${escapeHTML(bar.side)}" data-energy-path-bar-domain="${escapeHTML(bar.domain)}" data-energy-path-bar-value="${bar.value}" data-energy-path-focus="${energyPathFocusKind(focus, "node", bar.nodeId)}" ${energyPathAppearanceAttributes(appearance)}${appearance.allocated || appearance.residual ? ` style="fill:url(#${energyPathPatternID(appearance)})"` : ""}
      x="${bar.x}" y="${bar.y}" width="${bar.width}" height="${bar.height}"><title>${escapeHTML(title)}</title></rect>`;
  }).join("");
  return patterns + paths + bars;
}

function energyPathPaintKind(appearance) {
  return appearance.residual ? "residual" : appearance.allocated ? "allocated" : "solid";
}

function energyPathPatternID(appearance) {
  return `energyPathPattern-${energyPathPaintKind(appearance)}-${appearance.colorKey}`;
}

function energyPathAppearanceAttributes(appearance) {
  return `data-energy-path-color="${appearance.colorKey}" data-energy-path-paint="${energyPathPaintKind(appearance)}" data-energy-path-allocation-kind="${appearance.allocationLabelKind}"`;
}

function energyPathAppearanceLabel(appearance) {
  if (appearance.residual) return t("simulation.energyPathLegendResidual", {}, "Residual");
  if (appearance.allocationLabelKind === "includes_allocated") return t("simulation.energyPathIncludesAllocated", {}, "Includes allocated");
  return appearance.allocated ? t("simulation.energyPathAllocated", {}, "Allocated") : "";
}

function energyPathRibbonTitle(ribbon, nodeByID) {
  const from = nodeByID.get(ribbon.fromId)?.label || ribbon.fromId;
  const to = nodeByID.get(ribbon.toId)?.label || ribbon.toId;
  const fromUnit = ribbon.domain === "site" ? "kWh site" : "kWh thermal";
  const toUnit = ribbon.domain === "thermal" ? "kWh thermal" : "kWh site";
  return `${from} → ${to} · ${t("simulation.energyPathBridgeFrom", {}, "From")}: ${energyPathValueLabel(ribbon.fromValue, fromUnit)} · ${t("simulation.energyPathBridgeTo", {}, "To")}: ${energyPathValueLabel(ribbon.toValue, toUnit)}`;
}

function energyPathScaleLabel(domain, scale) {
  const value = typeof scale?.kWhPerPixel === "number" && Number.isFinite(scale.kWhPerPixel)
    ? scale.kWhPerPixel.toLocaleString(undefined, { maximumSignificantDigits: 5 })
    : t("common.notAvailable", {}, "—");
  return t(domain === "thermal" ? "simulation.energyPathThermalScale" : "simulation.energyPathSiteScale", { value },
    `${domain === "thermal" ? "Thermal" : "Site"} scale: ${value} kWh per px`);
}

function energyPathBridgeRatio(ribbon, nodes, links, ratioQuality) {
  if (["missing", "unavailable", "not_requested", "not_applicable"].includes(energyPathToken(ratioQuality?.status))) return null;
  return energyPathConversionRatio(ribbon.link, nodes, links) || {
    kind: "load_to_site_energy",
    label: t("simulation.energyPathRatioLoadSiteEnergy", {}, "Load / site energy"),
    value: ribbon.fromValue / ribbon.toValue,
  };
}

function renderEnergyPathBridgeRatios(drawing, layout, nodes, links, ratioQuality, focus) {
  const nodeByID = new Map(nodes.map((node) => [node.id, node]));
  return drawing.ribbons.filter((ribbon) => ribbon.domain === "conversion").map((ribbon, index) => {
    const ratio = energyPathBridgeRatio(ribbon, nodes, links, ratioQuality);
    const shortLabel = !ratio ? t("simulation.energyPathBridgeRatio", {}, "Ratio")
      : ratio.kind === "coefficient_of_performance" ? t("simulation.energyPathRatioCOP", {}, "COP")
        : ratio.kind === "efficiency" ? t("simulation.energyPathBridgeEfficiencyShort", {}, "Eff.")
          : t("simulation.energyPathBridgeLoadSiteShort", {}, "Load/site");
    const valueLabel = ratio ? energyPathRatioValueLabel(ratio.value) : t("common.notAvailable", {}, "—");
    const ratioLabel = ratio ? `${ratio.label}: ${valueLabel}`
      : t("simulation.energyPathBridgeRatioUnavailable", {}, "Ratio is unavailable for this period.");
    const title = [ratioLabel, energyPathRibbonTitle(ribbon, nodeByID),
      energyPathScaleLabel("thermal", drawing.scales.thermal), energyPathScaleLabel("site", drawing.scales.site),
      t("simulation.energyPathIndependentScales", {}, "The two domains use separate scales; the numeric ratio is calculated from the reported values, not pixel widths."),
    ].join(" · ");
    const tooltipID = `energyPathRatioTooltip-${index}`;
    return `<button type="button" class="energy-path-bridge-ratio" data-energy-explanation-edge="${escapeHTML(ribbon.id)}" data-energy-path-bridge-ratio="${escapeHTML(ribbon.id)}" data-energy-path-ratio-kind="${escapeHTML(ratio?.kind || "unavailable")}"${ratio ? ` data-energy-path-ratio-value="${ratio.value}"` : ""} data-energy-path-focus="${energyPathFocusKind(focus, "link", ribbon.id)}" aria-pressed="${focus.selectedLinkID === ribbon.id}"
      style="left:${ribbon.ratioAnchor.x / layout.width * 100}%;top:${ribbon.ratioAnchor.y}px;width:${ribbon.gap.width / layout.width * 100}%" aria-label="${escapeHTML(title)}" aria-describedby="${tooltipID}" title="${escapeHTML(title)}">
      <abbr class="energy-path-bridge-ratio-name" title="${escapeHTML(ratio?.label || ratioLabel)}">${escapeHTML(shortLabel)}</abbr><strong>${escapeHTML(valueLabel)}</strong>
      <span class="energy-path-ratio-tooltip" data-energy-path-ratio-tooltip id="${tooltipID}" role="tooltip">${escapeHTML(title)}</span>
    </button>`;
  }).join("");
}

function renderEnergyPathLinkHitLayer(drawing, layout, nodes, focus) {
  const nodeByID = new Map(nodes.map((node) => [node.id, node]));
  return `<svg class="energy-path-link-hit-layer" data-energy-path-hit-layer viewBox="0 0 ${layout.width} ${layout.height}" preserveAspectRatio="none" role="group" aria-label="${escapeHTML(t("simulation.energyPathSelectableLinks", {}, "Selectable energy links"))}">
    ${drawing.ribbons.map((ribbon) => {
      const title = [energyPathRibbonTitle(ribbon, nodeByID), energyPathAppearanceLabel(energyPathLinkAppearance(ribbon.link, nodes))].filter(Boolean).join(" · ");
      return `<g><path class="energy-path-link-hit" data-energy-path-link-hit="${escapeHTML(ribbon.id)}" data-energy-explanation-edge="${escapeHTML(ribbon.id)}" d="${escapeHTML(ribbon.path)}" tabindex="${ribbon.domain === "conversion" ? -1 : 0}" role="button" aria-pressed="${focus.selectedLinkID === ribbon.id}" aria-label="${escapeHTML(title)}"><title>${escapeHTML(title)}</title></path>
        <path class="energy-path-link-focus-ring${focus.selectedLinkID === ribbon.id ? " selected" : ""}" d="${escapeHTML(ribbon.path)}" aria-hidden="true"/>
      </g>`;
    }).join("")}
  </svg>`;
}

function renderEnergyPathLinkInspector(explanation, ribbon, nodes, links, viewState, ratioQuality, options = {}) {
  const link = ribbon.link;
  const nodeByID = new Map(nodes.map((node) => [node.id, node]));
  const thermalLoad = link.relation === "load_to_end_use" ? nodeByID.get(link.fromId) : null;
  const thermalBoundary = energyPathThermalBoundary(thermalLoad);
  const fromUnit = ribbon.domain === "site" ? "kWh site" : "kWh thermal";
  const toUnit = ribbon.domain === "thermal" ? "kWh thermal" : "kWh site";
  const fields = [
    ["from", t("simulation.energyPathBridgeFrom", {}, "From"), energyPathValueLabel(ribbon.fromValue, fromUnit)],
    ["to", t("simulation.energyPathBridgeTo", {}, "To"), energyPathValueLabel(ribbon.toValue, toUnit)],
  ];
  if (ribbon.domain === "conversion") {
    const ratio = energyPathBridgeRatio(ribbon, nodes, links, ratioQuality);
    fields.push(["ratio", t("simulation.energyPathBridgeRatio", {}, "Ratio"), ratio
      ? `${thermalBoundary ? t("simulation.energyPathKPILoadSiteRatio", {}, "Load/site ratio") : ratio.label}: ${energyPathRatioValueLabel(ratio.value)}`
      : t("simulation.energyPathBridgeRatioUnavailable", {}, "Ratio is unavailable for this period.")]);
  }
  const sourceIDs = energyPathUniqueValues(link.sourceIds);
  const sourceDetails = energyPathInspectorSources(explanation, { sourceIds: sourceIDs }, viewState);
  const model = energyPathInspectorModel(link, energyPathInspectorContext(explanation, nodes, links, sourceDetails, viewState, { kind: "link", ratioQuality }));
  const inspectorActions = typeof options.inspectorActionsForNode === "function" ? options.inspectorActionsForNode(link, sourceDetails, viewState) : {};
  const heading = `${nodeByID.get(ribbon.fromId)?.label || ribbon.fromId} → ${nodeByID.get(ribbon.toId)?.label || ribbon.toId}`;
  return `<aside class="energy-path-node-inspector energy-path-link-inspector" data-energy-path-link-inspector="${escapeHTML(ribbon.id)}">
    <header><strong>${escapeHTML(t("simulation.energyPathLinkDetail", {}, "Energy link detail"))}</strong><span>${escapeHTML(energyPathInspectorSafeLabel(heading, model))}</span></header>
    ${renderEnergyPathDetailSection("represents", renderEnergyPathRepresentation(link, model, thermalLoad))}
    ${renderEnergyPathDetailSection("value", renderEnergyPathInspectorValues(fields, "data-energy-path-link-value"))}
    ${renderEnergyPathDetailSection("breakdown", renderEnergyPathInspectorBreakdown(model, { skipRatios: true }))}
    ${renderEnergyPathDetailSection("basis", renderEnergyPathInspectorBasis(link, model, "data-energy-path-link-value"))}
    ${renderEnergyPathDetailSection("actions", renderEnergyPathInspectorActions(link, inspectorActions))}
  </aside>`;
}

function renderEnergyPathGraphLegend() {
  return `<div class="energy-path-domain-legend" data-energy-path-legend aria-label="${escapeHTML(t("simulation.energyPathScaleDomains", {}, "Thermal and site-energy scale domains"))}">
    <span data-energy-path-legend-item="thermal"><i class="energy-path-legend-swatch thermal" aria-hidden="true"></i>${escapeHTML(t("simulation.energyPathLegendThermal", {}, "Thermal kWh"))}</span>
    <span data-energy-path-legend-item="site"><i class="energy-path-legend-swatch site" aria-hidden="true"></i>${escapeHTML(t("simulation.energyPathLegendSite", {}, "Site kWh"))}</span>
    <span data-energy-path-legend-item="allocated"><i class="energy-path-legend-swatch allocated" aria-hidden="true"></i>${escapeHTML(t("simulation.energyPathAllocated", {}, "Allocated"))}</span>
    <span data-energy-path-legend-item="residual"><i class="energy-path-legend-swatch residual" aria-hidden="true"></i>${escapeHTML(t("simulation.energyPathLegendResidual", {}, "Residual"))}</span>
  </div>`;
}

function renderEnergyPathStage(stage, nodes, selectedID, relatedNodeIDs, index) {
  const stageNodes = (nodes || [])
    .filter((node) => (node.presentationLevel || node.level) === stage.level && (stage.level !== "driver" || Math.abs(Number(node.value) || 0) > 0))
    .sort((left, right) => compareEnergyPathStageNodes(stage, left, right));
  const partialCarrierSubtotal = stage.level === "carrier" && stageNodes.some((node) => node.presentationCoverage === "partial");
  const content = stageNodes.length
    ? stageNodes.map((node) => renderEnergyPathNode(node, stage, selectedID, relatedNodeIDs)).join("")
    : `<span class="energy-path-stage-empty">${escapeHTML(t("common.notAvailable", {}, "—"))}</span>`;
  return `
    <article class="energy-path-stage ${stage.scaleDomain === "site" ? "site-domain" : "thermal-domain"}" data-energy-path-stage="${escapeHTML(stage.level)}"${partialCarrierSubtotal ? ' data-energy-path-value-scope="observed_direct_use_subtotal"' : ""}>
      ${index === 2 ? `<span class="energy-path-conversion-divider">${escapeHTML(t("simulation.energyPathConversion", {}, "Equipment conversion"))}</span>` : ""}
      <header>
        <strong>${escapeHTML(partialCarrierSubtotal
          ? t("simulation.energyPathKnownEnergySources", {}, "Known energy sources")
          : t(stage.labelKey, {}, stage.label))}</strong>
        <span>${escapeHTML(t(stage.scaleDomain === "thermal" ? "simulation.energyPathThermalUnit" : "simulation.energyPathSiteUnit", {}, stage.unitLabel))}</span>
      </header>
      <p>${escapeHTML(partialCarrierSubtotal
        ? t("simulation.energyPathObservedDirectUseSubtotalExplanation", {}, "Only directly observed zone energy uses are included; energy-source values are subtotals, not complete zone totals.")
        : t(stage.descriptionKey, {}, stage.description))}</p>
      <div class="energy-path-stage-nodes">${content}</div>
    </article>`;
}

function compareEnergyPathStageNodes(stage, left, right) {
  return energyPathCompareNodes(stage, left, right);
}

function renderEnergyPathNode(node, stage, selectedID, relatedNodeIDs = new Set(), geometry = null) {
  const appearance = energyPathNodeAppearance(node);
  const selected = node.id && node.id === selectedID;
  const related = Boolean(node.id && relatedNodeIDs.has(node.id));
  const latentShare = stage.level === "load" ? energyPathLoadLatentShare(node) : 0;
  const latentBadge = latentShare + 1e-9 >= 0.1 ? renderEnergyPathLatentBadge(latentShare) : "";
  const partialCoverageBadge = node.presentationCoverage === "partial"
    ? `<small class="energy-path-partial-coverage-badge">${escapeHTML(t("simulation.energyPathKnownOnly", {}, "Known only"))}</small>`
    : "";
  const residualBadge = node.level === "carrier" ? renderEnergyPathCarrierResidualBadge(node.carrierQuality) : "";
  const unclassified = node.presentationKind === "unclassified_energy";
  const label = node.label || node.kind || node.id || "";
  const number = energyPathNodeNumber(node.value);
  const fullValue = number === null ? t("common.notAvailable", {}, "—") : energyPathValueLabel(number, stage.unitLabel);
  const badgeLabels = [
    latentBadge ? t("simulation.energyPathLatentShareSignificant", { share: energyPathPercentLabel(latentShare) }, `Latent ${energyPathPercentLabel(latentShare)}`) : "",
    partialCoverageBadge ? t("simulation.energyPathKnownOnly", {}, "Known only") : "",
    residualBadge ? energyPathCarrierResidualBadgeLabel(node.carrierQuality) : "",
    energyPathAppearanceLabel(appearance),
  ].filter(Boolean);
  const fullTitle = [`${label}: ${fullValue}`, ...badgeLabels].join(" · ");
  const compactValue = number !== null
    ? number.toLocaleString(undefined, { maximumFractionDigits: 2 })
    : t("common.notAvailable", {}, "—");
  const placement = geometry
    ? ` data-energy-path-layout-node="${escapeHTML(node.id || "")}" data-energy-path-lane="${escapeHTML(geometry.lane)}" data-energy-path-focus="${geometry.focusKind}" style="left:${(geometry.x - geometry.column.x) / geometry.column.width * 100}%;top:${geometry.y}px;width:${geometry.width / geometry.column.width * 100}%;height:${geometry.height}px"`
    : "";
  return `
    <button class="energy-path-node${selected ? " selected" : ""}${related ? " related" : ""}${unclassified ? " energy-path-unclassified-energy" : ""}" type="button" data-energy-explanation-node="${escapeHTML(node.id || "")}" ${energyPathAppearanceAttributes(appearance)}${placement}${related ? ' data-energy-path-related="true"' : ""}${node.carrierQuality ? ` data-energy-path-carrier-quality="${escapeHTML(node.carrierQuality.status)}"` : ""}${unclassified ? ` data-energy-path-unclassified-energy="${escapeHTML(node.carrier || "")}"` : ""} aria-pressed="${selected ? "true" : "false"}" title="${escapeHTML(fullTitle)}" aria-label="${escapeHTML(fullTitle)}">
      ${appearance.allocated ? `<i class="energy-path-allocation-mark" data-energy-path-allocation-mark="${appearance.allocationLabelKind}" aria-hidden="true"></i>` : ""}
      <span class="energy-path-node-copy"><span class="energy-path-node-label">${escapeHTML(label)}</span>${latentBadge}${partialCoverageBadge}${residualBadge}</span>
      <strong>${escapeHTML(geometry ? compactValue : fullValue)}</strong>
    </button>`;
}

function energyPathNodeNumber(value) {
  if (typeof value !== "number" && typeof value !== "string") return null;
  if (typeof value === "string" && !value.trim()) return null;
  const number = Number(value);
  return Number.isFinite(number) ? number : null;
}

function energyPathValueLabel(value, unitLabel) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return t("common.notAvailable", {}, "—");
  }
  return `${number.toLocaleString(undefined, { maximumFractionDigits: 2 })} ${t(
    unitLabel === "kWh thermal" ? "simulation.energyPathThermalUnit" : "simulation.energyPathSiteUnit",
    {},
    unitLabel,
  )}`;
}

function renderEnergyPathSummaryItem(item = {}, ratio = false) {
  const detail = [
    energyPathSummaryOptionalValue(t("simulation.energyPathRaw", {}, "Raw"), item, "rawValue"),
    energyPathSummaryOptionalValue(t("simulation.energyPathAllocated", {}, "Allocated"), item, "allocatedValue"),
    item.basis ? `${t("simulation.energyPathBasis", {}, "Basis")}: ${item.basis}` : "",
    item.aggregationBasis
      ? `${t("simulation.energyPathAggregationBasis", {}, "Aggregation")}: ${item.aggregationBasis}`
      : "",
  ].filter(Boolean).join(" · ");
  return `
    <div class="energy-path-summary-item">
      <span>${escapeHTML(item.label || item.id || "")}</span>
      <strong>${escapeHTML(ratio ? [energyPathRatioValueLabel(item.value), item.unit || ""].filter(Boolean).join(" ") : energyPathSummaryValueLabel(item.value, item.unit || ""))}</strong>
      ${detail ? `<small>${escapeHTML(detail)}</small>` : ""}
    </div>`;
}

function energyPathSummaryOptionalValue(label, item, key) {
  if (!Object.prototype.hasOwnProperty.call(item, key) || !Number.isFinite(Number(item[key]))) {
    return "";
  }
  return `${label}: ${energyPathSummaryValueLabel(item[key], item.unit || "")}`;
}

function energyPathSummaryValueLabel(value, unit = "") {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return t("common.notAvailable", {}, "—");
  }
  const formatted = number.toLocaleString(undefined, { maximumFractionDigits: 2 });
  return [formatted, unit].filter(Boolean).join(" ");
}

function energyPathItemService(item = {}) {
  const explicit = energyPathToken(item.serviceKind || item.service || "");
  if (explicit === "cooling" || explicit === "heating") {
    return explicit;
  }
  const endUse = energyPathToken(item.endUse || "");
  if (endUse === "cooling" || endUse === "heating") {
    return endUse;
  }
  const semanticSegments = energyPathToken([item.kind, item.id].filter(Boolean).join("."))
    .split(/[.\s:/-]+/)
    .filter(Boolean);
  if (semanticSegments.includes("cooling")) {
    return "cooling";
  }
  if (semanticSegments.includes("heating")) {
    return "heating";
  }
  return "";
}

function energyPathAllNodes(explanation = {}) {
  return [
    ...(explanation.nodes || []),
    ...(explanation.periods || []).flatMap((period) => period?.nodes || []),
  ];
}

function energyPathAllLinks(explanation = {}) {
  return [
    ...(explanation.links || []),
    ...(explanation.periods || []).flatMap((period) => period?.links || []),
  ];
}

function energyPathZoneResults(explanation = {}) {
  return Array.isArray(explanation.zoneResults) ? explanation.zoneResults : [];
}

function energyPathUniqueNames(values = []) {
  const seen = new Set();
  return (values || []).filter((value) => {
    const token = energyPathToken(value);
    if (!token || seen.has(token)) {
      return false;
    }
    seen.add(token);
    return true;
  });
}

function energyPathToken(value = "") {
  return String(value || "").trim().toLowerCase();
}
