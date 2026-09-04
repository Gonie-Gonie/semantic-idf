import { t } from "../i18n.js";
import { escapeHTML } from "../state.js";
import {
  energyPathSummaryGroups,
  energyPathSummaryKPIValues,
  isEnergyPathSummaryV2,
} from "../energy-path-summary.js";

export const ENERGY_PATH_SCHEMA_V2 = "semantic-idf.energy-explanation/v2";

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
    label: "Thermal Loads",
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

export function isEnergyPathV2(explanation = {}) {
  return String(explanation?.schema || "").toLowerCase() === ENERGY_PATH_SCHEMA_V2;
}

export function energyPathHasPayload(explanation = {}) {
  return energyPathAllNodes(explanation).length > 0 ||
    energyPathAllLinks(explanation).length > 0 ||
    energyPathZoneResults(explanation).some((result) => (
      energyPathAllNodes(result).length > 0 || energyPathAllLinks(result).length > 0
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

export function renderEnergyPathView(explanation = {}, viewState = {}) {
  normalizeEnergyPathViewState(viewState, explanation);
  const graph = energyPathGraphForState(explanation, viewState);
  const selectedID = String(viewState.simulationEnergySelection || "");
  const stages = ENERGY_PATH_STAGES.map((stage, index) => renderEnergyPathStage(stage, graph.nodes, selectedID, index)).join("");
  return `
    <section class="energy-path-view" data-energy-path-schema="${escapeHTML(ENERGY_PATH_SCHEMA_V2)}">
      ${renderEnergyPathHeader(explanation, viewState)}
      ${renderEnergyPathWarnings(graph.warnings)}
      <div class="energy-path-stage-grid" role="group" aria-label="${escapeHTML(t("simulation.energyPathDirection", {}, "Load drivers → Thermal loads → End-use energy → Energy sources"))}">
        ${stages}
      </div>
      ${renderEnergyPathNodeInspector(explanation, graph.nodes, selectedID, viewState)}
      <div class="energy-path-domain-legend" aria-label="${escapeHTML(t("simulation.energyPathScaleDomains", {}, "Thermal and site-energy scale domains"))}">
        <span>${escapeHTML(t("simulation.energyPathThermalDomain", {}, "Thermal domain"))} · ${escapeHTML(t("simulation.energyPathThermalUnit", {}, "kWh thermal"))}</span>
        <strong>${escapeHTML(t("simulation.energyPathConversion", {}, "Equipment conversion"))}</strong>
        <span>${escapeHTML(t("simulation.energyPathSiteDomain", {}, "Site energy domain"))} · ${escapeHTML(t("simulation.energyPathSiteUnit", {}, "kWh site"))}</span>
      </div>
    </section>`;
}

export function renderEnergyPathNodeInspector(explanation = {}, nodes = [], selectedID = "", viewState = {}) {
  const node = (nodes || []).find((item) => item?.id && item.id === selectedID);
  if (!node) {
    return "";
  }
  const sourceDetails = energyPathInspectorSources(explanation, node, viewState);
  const raw = energyPathInspectorNumber(node, "rawValue", energyPathSumSourceField(sourceDetails, "rawValue"));
  const effective = energyPathInspectorNumber(node, "effectiveValue", energyPathSumSourceField(sourceDetails, "effectiveValue"));
  const allocated = energyPathInspectorNumber(node, "allocatedValue", node.value);
  const multiplier = energyPathInspectorMultiplier(node, sourceDetails, raw, effective);
  const applications = energyPathUniqueValues(sourceDetails.map((source) => source.multiplierApplication));
  const unit = node.unit || (node.scaleDomain === "site" ? "kWh site" : "kWh thermal");
  const allocatedDriver = node.level === "driver" && node.allocationApplied === true;
  const rawInspectorValue = allocatedDriver ? energyPathSignedDriverValue(node, raw) : raw;
  const effectiveInspectorValue = allocatedDriver ? energyPathSignedDriverValue(node, effective) : effective;
  const values = [
    ["raw", allocatedDriver
      ? t("simulation.energyPathRawHeatGainLoss", {}, "Raw heat gain / loss")
      : t("simulation.energyPathRawReported", {}, "Raw reported value"), energyPathSummaryValueLabel(rawInspectorValue, unit)],
    ["multiplier", t("simulation.energyPathEffectiveMultiplier", {}, "Effective multiplier"), multiplier.toLocaleString(undefined, { maximumFractionDigits: 4 })],
    ["effective", allocatedDriver
      ? t("simulation.energyPathSignedPressure", {}, "Signed pressure after multiplier")
      : t("simulation.energyPathEffectiveContribution", {}, "Effective contribution"), energyPathSummaryValueLabel(effectiveInspectorValue, unit)],
    ["allocated", allocatedDriver
      ? t("simulation.energyPathAllocatedContribution", {}, "Allocated contribution")
      : t("simulation.energyPathAllocated", {}, "Allocated"), energyPathSummaryValueLabel(allocated, unit)],
    ["application", t("simulation.energyPathMultiplierApplication", {}, "Multiplier application"), applications.join(", ") || "unknown"],
  ];
  const loadBreakdown = renderEnergyPathLoadBreakdown(node, unit);
  const allocationExplanation = renderEnergyPathAllocationExplanation(node);
  const offsetEffects = renderEnergyPathOffsetEffects(node, unit);
  const simultaneousLoad = renderEnergyPathSimultaneousLoad(node, unit);
  const sourceInspector = renderEnergyPathSourceDetails(sourceDetails, viewState);
  return `
    <aside class="energy-path-node-inspector" data-energy-path-inspector="${escapeHTML(node.id)}">
      <header>
        <strong>${escapeHTML(t("simulation.energyPathInspector", {}, "Energy Path detail"))}</strong>
        <span>${escapeHTML(node.label || node.kind || node.id)}</span>
      </header>
      <dl>
        ${values.map(([key, label, value]) => `
          <div data-energy-path-inspector-value="${key}">
            <dt>${escapeHTML(label)}</dt>
            <dd>${escapeHTML(value)}</dd>
          </div>`).join("")}
      </dl>
      ${allocationExplanation}
      ${loadBreakdown}
      ${offsetEffects}
      ${simultaneousLoad}
      ${sourceInspector}
    </aside>`;
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
    <code>heat_balance_share</code>
    <span>${escapeHTML(explanation)}</span>
  </p>`;
}

function renderEnergyPathOffsetEffects(node = {}, unit = "kWh thermal") {
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
        "Opposite-sign heat-balance effects are context only and never create reverse main ribbons.",
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
            <small>${escapeHTML(effect.explanation)} <code>${escapeHTML(effect.basis)}</code></small>
          </div>`;
        }).join("")}
      </dl>
    </section>`;
}

function renderEnergyPathSimultaneousLoad(node = {}, unit = "kWh thermal") {
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
      <code>${escapeHTML(basis)}</code>
    </section>`;
}

function renderEnergyPathLoadBreakdown(node = {}, unit = "kWh thermal") {
  if (node.level !== "load") return "";
  const components = (node.loadBreakdown || [])
    .map((component) => ({
      component: energyPathToken(component?.component),
      value: Number(component?.value),
      share: Number(component?.share),
    }))
    .filter((component) => component.component && Number.isFinite(component.value));
  const latentShare = energyPathLoadLatentShare(node, components);
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
          const share = Number.isFinite(component.share) && component.share >= 0
            ? component.share
            : Math.abs(component.value) / Math.max(Math.abs(Number(node.value) || 0), 1e-9);
          const emphasized = component.component === "latent" && significant;
          const label = component.component === "latent"
            ? t("simulation.energyPathLatentLoad", {}, "Latent")
            : component.component === "sensible"
              ? t("simulation.energyPathSensibleLoad", {}, "Sensible")
              : component.component;
          return `<div data-energy-path-load-breakdown-component="${escapeHTML(component.component)}" data-energy-path-load-breakdown-emphasized="${emphasized ? "true" : "false"}">
            <dt>${escapeHTML(label)}</dt>
            <dd>${escapeHTML(energyPathSummaryValueLabel(component.value, unit))} · ${escapeHTML(energyPathPercentLabel(share))}</dd>
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
  const direction = String(source.heatDirection || "").trim();
  const formula = String(source.formula || "").trim();
  const allocationFormula = source.allocationApplied ? String(source.allocationFormula || "").trim() : "";
  const allocationExplanation = source.allocationApplied ? String(source.allocationExplanation || "").trim() : "";
  const inputSourceIDs = energyPathUniqueValues(source.inputSourceIds).join(", ");
  const relatedEntityIDs = energyPathUniqueValues(source.relatedEntityIds);
  const status = energyPathSourceDerivationStatus(source);
  const fields = [
    ["driverComponent", t("simulation.energyPathSourceComponent", {}, "Component"), component],
    ["heatDirection", t("simulation.energyPathSourceHeatDirection", {}, "Heat direction"), direction],
    ["formula", t("simulation.energyPathSourceFormula", {}, "Formula"), formula],
    ["allocationFormula", t("simulation.energyPathSourceAllocationFormula", {}, "Allocation formula"), allocationFormula],
    ["allocationExplanation", t("simulation.energyPathSourceAllocationExplanation", {}, "Allocation note"), allocationExplanation],
    ["inputSourceIds", t("simulation.energyPathSourceInputs", {}, "Input sources"), inputSourceIDs],
    ["relatedEntityIds", t("simulation.energyPathSourceEntities", {}, "Related entities"), relatedEntityIDs],
  ].filter(([, , value]) => Array.isArray(value) ? value.length > 0 : Boolean(value));
  return `
    <article class="energy-path-inspector-source" data-energy-path-source="${escapeHTML(source.id || "")}" data-energy-path-source-status="${status}">
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
    unique.push({ severity, code, message });
  }
  if (!unique.length) return "";
  return `
    <section class="energy-path-warnings" data-energy-path-warnings role="status">
      <h4>${escapeHTML(t("simulation.energyPathQualityWarnings", {}, "Energy Path quality warnings"))}</h4>
      <ul>${unique.map((warning) => `
        <li data-energy-path-warning-severity="${warning.severity}">
          ${warning.code ? `<code>${escapeHTML(warning.code)}</code>` : ""}
          <span>${escapeHTML(warning.message)}</span>
        </li>`).join("")}</ul>
    </section>`;
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

export function renderEnergyPathKPI(summary = {}) {
  if (!isEnergyPathSummaryV2(summary)) {
    return "";
  }
  const labels = {
    total_site_energy: t("simulation.energyPathTotalSiteEnergy", {}, "Total site energy"),
    cooling_load: t("simulation.energyPathCoolingLoad", {}, "Cooling load"),
    heating_load: t("simulation.energyPathHeatingLoad", {}, "Heating load"),
    coverage: t("simulation.energyPathCoverage", {}, "Coverage"),
  };
  return `<div class="simulation-energy-kpis energy-path-kpis">
    ${energyPathSummaryKPIValues(summary)
      .map((item) => `
        <div data-energy-path-kpi="${escapeHTML(item.id)}">
          <span>${escapeHTML(labels[item.id] || item.label)}</span>
          <strong>${escapeHTML(energyPathSummaryValueLabel(item.value, item.unit))}</strong>
        </div>`)
      .join("")}
  </div>`;
}

export function renderEnergyPathSummaryOverview(summary = {}) {
  if (!isEnergyPathSummaryV2(summary)) {
    return "";
  }
  const groups = energyPathSummaryGroups(summary);
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
        ${groups.map((group) => `
          <article data-energy-path-summary-group="${escapeHTML(group.key)}">
            <header>
              <strong>${escapeHTML(groupLabels[group.key] || group.label)}</strong>
              <span>${escapeHTML(group.items.length)}</span>
            </header>
            <div>
              ${group.items.length
                ? group.items.slice(0, 8).map(renderEnergyPathSummaryItem).join("")
                : `<span class="energy-path-summary-empty">${escapeHTML(t("common.notAvailable", {}, "—"))}</span>`}
            </div>
          </article>`).join("")}
      </div>
    </section>`;
}

export function renderEnergyPathHeader(explanation = {}, viewState = {}) {
  normalizeEnergyPathViewState(viewState, explanation);
  return `
    <div class="energy-path-header">
      <div class="energy-path-heading">
        <h4>${escapeHTML(t("simulation.energyPathName", {}, "Energy Path"))}</h4>
        <span>${escapeHTML(t("simulation.energyPathDirection", {}, "Load drivers → Thermal loads → End-use energy → Energy sources"))}</span>
      </div>
      ${renderEnergyPathControls(explanation, viewState)}
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

export function energyPathServiceOptions(explanation = {}, viewState = {}) {
  const graph = energyPathGraphForState(explanation, {
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
    return { nodes: [], links: [], warnings: [] };
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

  const scopeKind = viewState.simulationEnergyScopeKind || "building";
  if (scopeKind !== "zone") {
    nodes = nodes.filter((node) => !node.zoneName || node.aggregationBasis === "model_total");
    const nodeIDs = new Set(nodes.map((node) => node.id));
    links = links.filter((link) => nodeIDs.has(link.fromId) && nodeIDs.has(link.toId));
  }

  const service = viewState.simulationEnergyService || "all";
  if (service !== "all") {
    const serviceLinks = links.filter((link) => energyPathItemService(link) === service);
    const linkedIDs = new Set(serviceLinks.flatMap((link) => [link.fromId, link.toId]).filter(Boolean));
    nodes = nodes.filter((node) => energyPathItemService(node) === service || linkedIDs.has(node.id));
    const nodeIDs = new Set(nodes.map((node) => node.id));
    links = links.filter((link) => (
      energyPathItemService(link) === service && nodeIDs.has(link.fromId) && nodeIDs.has(link.toId)
    ));
	} else {
		({ nodes, links } = energyPathMergeAllServiceDrivers(nodes, links));
  }

  const nodeIDs = new Set(nodes.map((node) => node.id));
  return {
    nodes,
    links: links.filter((link) => nodeIDs.has(link.fromId) && nodeIDs.has(link.toId)),
    warnings,
  };
}

export function energyPathMergeAllServiceDrivers(nodes = [], links = []) {
	const canonicalDrivers = (nodes || []).filter((node) => (
		node.level === "driver" &&
		Math.abs(Number(node.value) || 0) > 0 &&
		Object.prototype.hasOwnProperty.call(ENERGY_PATH_DRIVER_ORDER, energyPathToken(node.driverCategory))
	));
	if (!canonicalDrivers.length) {
		return { nodes, links };
	}

	const targetByCategory = new Map([["internal.other", "balance.storage_other"]]);
	const totals = new Map();
	for (const node of canonicalDrivers) {
		const source = energyPathToken(node.driverCategory);
		const target = targetByCategory.get(source) || source;
		totals.set(target, (totals.get(target) || 0) + Math.abs(Number(node.value) || 0));
	}
	if (totals.size > 11) {
		const compactable = ["internal.people", "internal.lighting", "internal.equipment"]
			.filter((category) => totals.has(category))
			.sort((left, right) => {
				const valueOrder = (totals.get(left) || 0) - (totals.get(right) || 0);
				return valueOrder || (ENERGY_PATH_DRIVER_ORDER[right] - ENERGY_PATH_DRIVER_ORDER[left]);
			});
		for (const category of compactable) {
			if (totals.size <= 11) break;
			targetByCategory.set(category, "balance.storage_other");
			totals.set("balance.storage_other", (totals.get("balance.storage_other") || 0) + (totals.get(category) || 0));
			totals.delete(category);
		}
	}

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
		for (const field of ["fromValue", "toValue", "value", "signedValue", "displayValue"]) {
			if (Object.prototype.hasOwnProperty.call(projected, field) || Object.prototype.hasOwnProperty.call(current, field)) {
				current[field] = (Number(current[field]) || 0) + (Number(projected[field]) || 0);
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
  return {
    ...candidate,
    period: candidatePeriod,
    scope: candidateScope || { kind: wantedScopeKind },
    completeness,
  };
}

export function renderEnergyPathControls(explanation = {}, viewState = {}) {
  const zones = energyPathZoneNames(explanation);
  const rootIsZoneOnly = energyPathToken(explanation.scope?.kind) === "zone" && !energyPathZoneResults(explanation).length;
  const scopes = ENERGY_PATH_SCOPES.map((scope) => (
    `<option value="${scope.value}" ${viewState.simulationEnergyScopeKind === scope.value ? "selected" : ""} ${(scope.value === "zone" && !zones.length) || (scope.value === "building" && rootIsZoneOnly) ? "disabled" : ""}>${escapeHTML(t(scope.labelKey, {}, scope.label))}</option>`
  )).join("");
  const periods = ENERGY_PATH_PERIODS.map((period) => (
    `<option value="${period.value}" ${viewState.simulationEnergyPeriod === period.value ? "selected" : ""}>${escapeHTML(t(period.labelKey, {}, period.label))}</option>`
  )).join("");
  const services = energyPathServiceOptions(explanation, viewState).map((service) => (
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
          <select data-simulation-energy-scope>${scopes}</select>
          ${zoneControl}
        </span>
      </label>
      <label>
        <span>${escapeHTML(t("common.period", {}, "Period"))}</span>
        <select data-simulation-energy-path-period>${periods}</select>
      </label>
      <label>
        <span>${escapeHTML(t("simulation.service", {}, "Service"))}</span>
        <select data-simulation-energy-service>${services}</select>
      </label>
    </div>`;
}

function renderEnergyPathStage(stage, nodes, selectedID, index) {
  const stageNodes = (nodes || [])
    .filter((node) => node.level === stage.level && (stage.level !== "driver" || Math.abs(Number(node.value) || 0) > 0))
    .sort((left, right) => compareEnergyPathStageNodes(stage, left, right));
  const content = stageNodes.length
    ? stageNodes.map((node) => renderEnergyPathNode(node, stage, selectedID)).join("")
    : `<span class="energy-path-stage-empty">${escapeHTML(t("common.notAvailable", {}, "—"))}</span>`;
  return `
    <article class="energy-path-stage ${stage.scaleDomain === "site" ? "site-domain" : "thermal-domain"}" data-energy-path-stage="${escapeHTML(stage.level)}">
      ${index === 2 ? `<span class="energy-path-conversion-divider">${escapeHTML(t("simulation.energyPathConversion", {}, "Equipment conversion"))}</span>` : ""}
      <header>
        <strong>${escapeHTML(t(stage.labelKey, {}, stage.label))}</strong>
        <span>${escapeHTML(t(stage.scaleDomain === "thermal" ? "simulation.energyPathThermalUnit" : "simulation.energyPathSiteUnit", {}, stage.unitLabel))}</span>
      </header>
      <p>${escapeHTML(t(stage.descriptionKey, {}, stage.description))}</p>
      <div class="energy-path-stage-nodes">${content}</div>
    </article>`;
}

function compareEnergyPathStageNodes(stage, left, right) {
  if (stage.level === "driver") {
    const leftOrder = ENERGY_PATH_DRIVER_ORDER[energyPathToken(left.driverCategory)] ?? Number.MAX_SAFE_INTEGER;
    const rightOrder = ENERGY_PATH_DRIVER_ORDER[energyPathToken(right.driverCategory)] ?? Number.MAX_SAFE_INTEGER;
    if (leftOrder !== rightOrder) {
      return leftOrder - rightOrder;
    }
  }
  const valueOrder = Math.abs(Number(right.value) || 0) - Math.abs(Number(left.value) || 0);
  if (valueOrder !== 0) {
    return valueOrder;
  }
  return String(left.id || "").localeCompare(String(right.id || ""));
}

function renderEnergyPathNode(node, stage, selectedID) {
  const selected = node.id && node.id === selectedID;
  const latentShare = stage.level === "load" ? energyPathLoadLatentShare(node) : 0;
  const latentBadge = latentShare + 1e-9 >= 0.1 ? renderEnergyPathLatentBadge(latentShare) : "";
  return `
    <button class="energy-path-node${selected ? " selected" : ""}" type="button" data-energy-explanation-node="${escapeHTML(node.id || "")}" aria-pressed="${selected ? "true" : "false"}">
      <span>${escapeHTML(node.label || node.kind || node.id || "")}${latentBadge}</span>
      <strong>${escapeHTML(energyPathValueLabel(node.value, stage.unitLabel))}</strong>
    </button>`;
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

function renderEnergyPathSummaryItem(item = {}) {
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
      <strong>${escapeHTML(energyPathSummaryValueLabel(item.value, item.unit || ""))}</strong>
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
  const token = energyPathToken(item.serviceKind || item.service || item.endUse || item.id || "");
  if (token.includes("cool")) {
    return "cooling";
  }
  if (token.includes("heat")) {
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
