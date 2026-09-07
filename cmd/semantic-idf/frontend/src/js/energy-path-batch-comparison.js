import { isEnergyPathSummaryV2 } from "./energy-path-summary.js";

export const ENERGY_PATH_BATCH_STAGES = Object.freeze([
  { key: "drivers", label: "Drivers" }, { key: "loads", label: "Loads" },
  { key: "endUses", label: "End uses" }, { key: "carriers", label: "Carriers" },
  { key: "ratios", label: "Ratios" }, { key: "residuals", label: "Residual / coverage" },
].map(Object.freeze));

const DRIVERS = Object.freeze({
  "surface.exterior_walls": "Exterior walls", "surface.roofs": "Roofs",
  "surface.ground_floors": "Ground / floors", "surface.windows_doors": "Windows / doors",
  "surface.interzone": "Interzone surfaces", "air.infiltration": "Infiltration",
  "air.mechanical_ventilation": "Mechanical ventilation", "air.interzone": "Interzone air",
  "internal.people": "People", "internal.lighting": "Lighting heat", "internal.equipment": "Equipment heat",
  "interzone.transfer": "Interzone transfer", "internal.other": "Other internal gains", "balance.storage_other": "Other / storage",
});
const END_USES = Object.freeze({
  cooling: "Cooling equipment", heating: "Heating equipment", fans: "Fans", pumps: "Pumps",
  fans_pumps: "Fans & pumps", heat_rejection: "Heat rejection", humidification: "Humidification",
  heat_recovery: "Heat recovery", hvac_auxiliaries: "HVAC auxiliaries", lighting: "Lighting",
  equipment: "Equipment", water_systems: "Water systems", refrigeration: "Refrigeration",
  storage_charge: "Storage charge", other: "Other end uses",
});
const CARRIERS = Object.freeze({
  electricity: "Electricity", natural_gas: "Natural gas", district_cooling: "District cooling",
  district_heating: "District heating", steam: "Steam", propane: "Propane", fuel_oil_1: "Fuel oil #1",
  fuel_oil_2: "Fuel oil #2", coal: "Coal", diesel: "Diesel", gasoline: "Gasoline",
  other_fuel_1: "Other fuel 1", other_fuel_2: "Other fuel 2", water: "Water",
});
const RATIOS = Object.freeze({
  coefficient_of_performance: "COP", efficiency: "Efficiency", load_to_site_energy: "Load / site energy",
  load_to_purchased_energy: "Load / purchased energy", load_to_fuel: "Load / fuel",
});
const BASES = new Set(["reported_meter", "reported_variable", "reported_end_use_subtotal", "integrated_rate",
  "heat_balance_share", "service_path_allocation", "zone_load_allocation", "direct_zone_energy", "derived_ratio", "residual"]);
const AVAILABLE = new Set(["complete", "partial", "overmapped"]);
const ENERGY_UNITS = new Set(["j", "kj", "mj", "gj", "wh", "kwh", "mwh", "btu", "kbtu", "mbtu", "therm", "therms"]);
const list = (value) => Array.isArray(value) ? value : [];
const token = (value) => typeof value === "string" ? value.trim().toLowerCase() : "";
const finite = (value) => (typeof value === "number" || typeof value === "string" && value.trim() !== "") && Number.isFinite(Number(value)) ? Number(value) : null;
const canonicalService = (value) => ["cooling", "heating"].includes(token(value)) ? token(value) : "";
const serviceLabel = (value) => value === "cooling" ? "Cooling" : value === "heating" ? "Heating" : "";
const labelFrom = (labels, value, fallback) => Object.hasOwn(labels, value) ? labels[value] : fallback;

/** Batch comparison consumes the existing v2 annual summary, never a new energy aggregation. */
export function energyPathBatchSummary(run = {}) {
  const summary = run?.purposeResults?.energyExplanationSummary || run?.purposeResults?.energyExplanation?.summary;
  if (!summary || typeof summary !== "object") return { status: "unavailable", reason: "missing_summary", summary: null };
  if (!isEnergyPathSummaryV2(summary)) return { status: "unavailable", reason: "unsupported_schema", summary: null };
  if (token(summary.scope?.kind) !== "building" || token(summary.period) !== "annual" || token(summary.scope?.zoneName)) {
    return { status: "unavailable", reason: "building_annual_only", summary: null };
  }
  return { status: "ready", reason: "", summary };
}

function semanticCategory(stage, item) {
  const kind = token(item.kind), service = canonicalService(item.serviceKind) || canonicalService(kind.replace(/^load\./, ""));
  const expectedLevel = { drivers: "driver", loads: "load", endUses: "end_use", carriers: "carrier", ratios: "ratio", residuals: "residual" }[stage];
  const levelValid = !token(item.level) || token(item.level) === expectedLevel;
  const requiredServiceValid = !["drivers", "loads", "ratios"].includes(stage) || !token(item.serviceKind) || Boolean(canonicalService(item.serviceKind));
  let category = "", label = "", key = "", known = true;
  if (stage === "drivers") {
    category = kind.startsWith("driver.") ? kind.slice(7) : "";
    known = Object.hasOwn(DRIVERS, category) && Boolean(service);
    label = labelFrom(DRIVERS, category, "Unclassified driver");
    key = known ? `simulation.energyPathDriverCategory.${category}` : "";
  } else if (stage === "loads") {
    category = service;
    const kindService = canonicalService(kind.replace(/^load\./, ""));
    known = Boolean(category) && (!kindService || !token(item.serviceKind) || kindService === canonicalService(item.serviceKind));
    label = service ? `${serviceLabel(service)} load` : "Unclassified load";
  } else if (stage === "endUses") {
    category = token(item.endUse) || (kind.startsWith("energy.") ? kind.slice(7) : "");
    const kindEndUse = kind.startsWith("energy.") ? kind.slice(7) : "";
    known = Object.hasOwn(END_USES, category) && (!Object.hasOwn(END_USES, kindEndUse) || !token(item.endUse) || category === kindEndUse);
    label = labelFrom(END_USES, category, "Unclassified end use");
  } else if (stage === "carriers") {
    category = token(item.carrier);
    const kindCarrier = kind.match(/^(?:energy|carrier)\.([^.]+)(?:\.(?:total|observed_end_use_subtotal|direct_zone_subtotal))?$/)?.[1];
    known = Object.hasOwn(CARRIERS, category) && (!Object.hasOwn(CARRIERS, kindCarrier) || category === kindCarrier);
    label = labelFrom(CARRIERS, category, "Unclassified energy source");
  } else if (stage === "ratios") {
    category = kind.startsWith("ratio.") ? kind.slice(6) : /^kpi\.(cooling|heating)_cop$/.test(kind) ? "coefficient_of_performance" : "";
    const ratioService = service || canonicalService(kind.match(/^kpi\.(cooling|heating)_cop$/)?.[1]);
    const kindService = canonicalService(kind.match(/^kpi\.(cooling|heating)_cop$/)?.[1]);
    known = Object.hasOwn(RATIOS, category) && Boolean(ratioService) && (!kindService || !token(item.serviceKind) || kindService === ratioService);
    label = labelFrom(RATIOS, category, "Unclassified ratio");
    return { id: category || "unknown", label, key, service: ratioService, known: known && levelValid && requiredServiceValid, identity: [stage, category || "unknown", ratioService] };
  } else {
    category = kind;
    const domain = token(item.scaleDomain), carrier = token(item.carrier);
    known = Boolean(kind) && ["thermal", "site"].includes(domain) && (domain === "thermal" ? Boolean(service) : Object.hasOwn(CARRIERS, carrier));
    label = domain === "thermal" ? `${serviceLabel(service) || "Thermal"} residual` : `${labelFrom(CARRIERS, carrier, "Site energy")} residual`;
    return { id: category || "unknown", label, key, service, known: known && levelValid, identity: [stage, domain, domain === "site" ? carrier : service, category || "unknown"] };
  }
  return { id: category || "unknown", label, key, service, known: known && levelValid && requiredServiceValid,
    identity: [stage, category || "unknown", stage === "drivers" ? service : ""] };
}

function unitIdentity(unit, domain = "") {
  const text = token(unit).replaceAll("ₜₕ", " thermal").replace(/[_\s-]+/g, " ");
  const explicitDomain = /\bthermal\b/.test(text) ? "thermal" : /\bsite\b/.test(text) ? "site" : "";
  return { base: text.replace(/\b(thermal|site)\b/g, "").trim(), domain: token(domain) || explicitDomain,
    valid: !explicitDomain || !domain || explicitDomain === token(domain) };
}

function availability(quality) {
  const status = token(quality?.status) || "unavailable", found = finite(quality?.found), total = finite(quality?.total);
  const valid = Number.isInteger(found) && Number.isInteger(total) && found >= 0 && total > 0 && found <= total;
  return { status, found, total, percent: valid && (AVAILABLE.has(status) || status === "missing") ? found / total * 100 : null };
}

function accounting(quality, field, statusField) {
  const status = token(quality?.[statusField]) || "unavailable", value = finite(quality?.[field]);
  return { status, found: null, total: null, percent: value !== null && value >= 0 && AVAILABLE.has(status) ? value : null };
}

function itemQuality(summary, stage, item = {}) {
  if (stage !== "residuals") return availability(summary.quality?.[stage]);
  return token(item.scaleDomain) === "thermal"
    ? accounting(summary.quality, "driverToLoadClosedPct", "driverToLoadStatus")
    : accounting(summary.quality, "endUseToCarrierClosedPct", "endUseToCarrierStatus");
}

function indexSummary(summary, stage) {
  const entries = new Map();
  for (const item of list(summary[stage])) {
    if (!item || typeof item !== "object" || Array.isArray(item)) continue;
    // Raw water is utility context, not site energy. Retain only an explicitly
    // derived energy equivalent with a genuine energy unit (the v2 contract).
    if (stage === "carriers" && token(item.carrier) === "water" &&
      (token(item.basis) !== "derived_ratio" || !ENERGY_UNITS.has(unitIdentity(item.unit).base))) continue;
    const category = semanticCategory(stage, item), key = JSON.stringify(category.identity);
    if (!entries.has(key)) entries.set(key, { category, items: [] });
    // A repeated record is still ambiguous: summary generation already performs
    // legitimate aggregation. This consumer never sums or chooses a first row.
    entries.get(key).items.push(item);
  }
  return entries;
}

function summarySide(entry, summary, stage) {
  const items = entry?.items || [], item = items.length === 1 ? items[0] : null;
  const expectedDomain = ["drivers", "loads"].includes(stage) ? "thermal" : ["endUses", "carriers"].includes(stage) ? "site" : "";
  const domain = token(item?.scaleDomain) || expectedDomain;
  const unit = unitIdentity(item?.unit, domain), value = finite(item?.value);
  const ratioUnits = stage === "ratios" ? [unitIdentity(item?.numeratorUnit, "thermal"), unitIdentity(item?.denominatorUnit, "site")] : [];
  const unitValid = unit.valid && (!expectedDomain || domain === expectedDomain) &&
    (stage === "ratios" || ENERGY_UNITS.has(unit.base)) && ratioUnits.every((part) => ENERGY_UNITS.has(part.base) && part.valid);
  const invalid = Boolean(item) && (value === null || !entry.category.known || !unitValid);
  return {
    value: item && !invalid ? value : null, unit: typeof item?.unit === "string" ? item.unit : "",
    basis: token(item?.basis), aggregationBasis: token(item?.aggregationBasis) || token(summary.scope?.aggregationBasis),
    missing: items.length === 0, ambiguous: items.length > 1, invalid, unitValid,
    sign: token(item?.sign), scaleDomain: domain, quality: itemQuality(summary, stage, item || {}),
    unitIdentity: JSON.stringify([unit.base, unit.domain, ...ratioUnits.map((part) => [part.base, part.domain])]),
  };
}

function compareSides(baseline, target, { quality = false } = {}) {
  const present = !baseline.missing && !target.missing && !baseline.ambiguous && !target.ambiguous;
  const unitMismatch = present && (baseline.unitIdentity !== target.unitIdentity || baseline.unitValid === false || target.unitValid === false);
  const basisMismatch = present && (!baseline.basis || !target.basis || baseline.basis !== target.basis ||
    baseline.aggregationBasis !== target.aggregationBasis || !quality && (!BASES.has(baseline.basis) || !BASES.has(target.basis)));
  const coverageMismatch = baseline.quality.status !== target.quality.status || baseline.quality.percent !== target.quality.percent ||
    baseline.quality.found !== target.quality.found || baseline.quality.total !== target.quality.total;
  const numeric = present && !unitMismatch && !baseline.invalid && !target.invalid && baseline.value !== null && target.value !== null;
  const difference = numeric ? target.value - baseline.value : null;
  const delta = difference !== null && Number.isFinite(difference) ? difference || 0 : null;
  const percent = delta !== null && baseline.value !== 0 ? delta / baseline.value * 100 : null;
  const deltaPercent = percent !== null && Number.isFinite(percent) ? percent || 0 : null;
  const warningCodes = [
    baseline.missing ? "missing_baseline" : "", target.missing ? "missing_target" : "",
    baseline.ambiguous || target.ambiguous ? "ambiguous_category" : "",
    baseline.invalid || target.invalid ? "invalid_value_or_category" : "",
    unitMismatch ? "unit_or_domain_mismatch" : "", basisMismatch ? "basis_mismatch" : "",
    coverageMismatch ? "coverage_mismatch" : "",
    numeric && delta === null ? "numeric_overflow" : "",
  ].filter(Boolean);
  return { delta, deltaPercent, basisMismatch, coverageMismatch, unitMismatch,
    directlyComparable: delta !== null && !basisMismatch && !coverageMismatch, warningCodes };
}

function qualityRows(baselineSummary, targetSummary) {
  const definitions = [
    ...["drivers", "loads", "endUses", "carriers"].map((stage) => ({
      id: `${stage}_coverage`, label: `${ENERGY_PATH_BATCH_STAGES.find((item) => item.key === stage).label} source coverage`,
      read: (summary) => availability(summary.quality?.[stage]), basis: "source_availability",
    })),
    ...[
      ["driverToLoadClosedPct", "driverToLoadStatus", "Driver-to-load closure"],
      ["endUseToCarrierClosedPct", "endUseToCarrierStatus", "End-use-to-carrier closure"],
      ["zoneAllocatedPct", "zoneAllocationStatus", "Assigned Zone HVAC share"],
      ["unassignedPct", "zoneAllocationStatus", "Unassigned HVAC share"],
    ].map(([field, status, label]) => ({ id: field, label, read: (summary) => accounting(summary.quality, field, status), basis: "accounting_quality" })),
  ];
  return definitions.map((definition) => {
    const side = (summary) => {
      const quality = definition.read(summary);
      return { value: quality.percent, unit: "%", unitIdentity: "%", basis: definition.basis,
        aggregationBasis: token(summary.scope?.aggregationBasis), quality, missing: false, ambiguous: false, invalid: false };
    };
    const baseline = side(baselineSummary), target = side(targetSummary);
    return { key: JSON.stringify(["quality", definition.id]), kind: "quality", categoryId: definition.id,
      stage: "residuals", stageLabel: "Residual / coverage", category: definition.label, categoryLabel: definition.label,
      categoryKey: "", service: "", baseline, target, unit: "%", deltaUnit: "pp", ...compareSides(baseline, target, { quality: true }) };
  });
}

/** Nullable arithmetic and typed semantic matching; no raw node/source ID joins. */
export function energyPathBatchComparison(baselineRun = {}, targetRun = {}) {
  const baselineResult = energyPathBatchSummary(baselineRun), targetResult = energyPathBatchSummary(targetRun);
  if (baselineResult.status !== "ready" || targetResult.status !== "ready") return {
    status: "unavailable", reason: baselineResult.reason || targetResult.reason, rows: [], stages: ENERGY_PATH_BATCH_STAGES,
  };
  const rows = [];
  for (const stage of ENERGY_PATH_BATCH_STAGES) {
    const left = indexSummary(baselineResult.summary, stage.key), right = indexSummary(targetResult.summary, stage.key);
    const order = Object.keys({ drivers: DRIVERS, endUses: END_USES, carriers: CARRIERS, ratios: RATIOS }[stage.key] || {});
    const rank = (key) => {
      const index = order.indexOf((left.get(key) || right.get(key)).category.id);
      return index < 0 ? order.length : index;
    };
    const keys = [...new Set([...left.keys(), ...right.keys()])].sort((a, b) => rank(a) - rank(b) || a.localeCompare(b));
    for (const key of keys) {
      const category = (left.get(key) || right.get(key)).category;
      const baseline = summarySide(left.get(key), baselineResult.summary, stage.key);
      const target = summarySide(right.get(key), targetResult.summary, stage.key);
      const suffix = ["drivers", "ratios"].includes(stage.key) ? serviceLabel(category.service) : "";
      rows.push({ key, kind: "summary", stage: stage.key, stageLabel: stage.label,
        category: [category.label, suffix].filter(Boolean).join(" · "), categoryLabel: category.label,
        categoryKey: category.key, categoryId: category.id, service: category.service,
        baseline, target, unit: target.unit || baseline.unit, deltaUnit: target.unit || baseline.unit,
        ...compareSides(baseline, target) });
    }
  }
  rows.push(...qualityRows(baselineResult.summary, targetResult.summary));
  return { status: "ready", reason: "", scope: "building", period: "annual", rows, stages: ENERGY_PATH_BATCH_STAGES };
}
