export const ENERGY_PATH_SUMMARY_SCHEMA_V1 = "semantic-idf.energy-explanation-summary/v1";
export const ENERGY_PATH_SUMMARY_SCHEMA_V2 = "semantic-idf.energy-explanation-summary/v2";

const ENERGY_PATH_SUMMARY_V2_GROUPS = Object.freeze([
  Object.freeze({ key: "drivers", label: "Drivers", type: "energy_explanation.drivers", compare: true }),
  Object.freeze({ key: "loads", label: "Loads", type: "energy_explanation.loads", compare: true }),
  Object.freeze({ key: "endUses", label: "End Uses", type: "energy_explanation.end_uses", compare: true }),
  Object.freeze({ key: "carriers", label: "Carriers", type: "energy_explanation.carriers", compare: true }),
  Object.freeze({ key: "ratios", label: "Ratios", type: "energy_explanation.ratios", compare: true }),
  Object.freeze({ key: "residuals", label: "Residuals", type: "energy_explanation.residuals", compare: true }),
  Object.freeze({ key: "topZones", label: "Top Zones", type: "energy_explanation.top_zones", compare: true }),
]);

const ENERGY_PATH_SUMMARY_V1_GROUPS = Object.freeze([
  Object.freeze({ key: "energyByCarrier", label: "Energy by carrier", type: "energy_explanation.energy_by_carrier", compare: false }),
  Object.freeze({ key: "energyByEndUse", label: "Energy Use", type: "energy_explanation.energy_by_end_use", compare: true }),
  Object.freeze({ key: "deliveredLoadByService", label: "Delivered Load", type: "energy_explanation.delivered_load_by_service", compare: true }),
  Object.freeze({ key: "derivedKpis", label: "Derived KPI", type: "energy_explanation.derived_kpi", compare: true }),
  Object.freeze({ key: "heatDrivers", label: "Heat Drivers", type: "energy_explanation.heat_drivers", compare: true }),
  Object.freeze({ key: "residuals", label: "Residual", type: "energy_explanation.residuals", compare: true }),
  Object.freeze({ key: "topHeatDrivers", label: "Top heat drivers", type: "energy_explanation.top_heat_drivers", compare: false }),
  Object.freeze({ key: "topZones", label: "Top zones", type: "energy_explanation.top_zones", compare: false }),
]);

const ENERGY_PATH_SITE_CARRIERS = new Set([
  "electricity", "natural_gas", "district_cooling", "district_heating", "steam", "propane",
  "fuel_oil_1", "fuel_oil_2", "coal", "diesel", "gasoline", "other_fuel_1", "other_fuel_2", "water",
]);

export function isEnergyPathSummaryV2(summary = {}) {
  return String(summary?.schema || "").toLowerCase() === ENERGY_PATH_SUMMARY_SCHEMA_V2;
}

export function energyPathSummaryGroups(summary = {}, { comparison = false } = {}) {
  const definitions = isEnergyPathSummaryV2(summary)
    ? ENERGY_PATH_SUMMARY_V2_GROUPS
    : ENERGY_PATH_SUMMARY_V1_GROUPS;
  return definitions
    .filter((definition) => !comparison || definition.compare)
    .map((definition) => ({
      ...definition,
      items: Array.isArray(summary?.[definition.key]) ? summary[definition.key] : [],
    }));
}

export function energyPathLegacyDerivedKPIItems(summary = {}) {
  if (isEnergyPathSummaryV2(summary)) {
    return [];
  }
  return energyPathSummaryGroups(summary)
    .find((group) => group.type === "energy_explanation.derived_kpi")?.items || [];
}

export function energyPathSummaryKPIValues(summary = {}) {
  if (!isEnergyPathSummaryV2(summary)) {
    return [];
  }
  const groups = new Map(energyPathSummaryGroups(summary).map((group) => [group.key, group.items]));
  const loads = groups.get("loads") || [];
  const carriers = (groups.get("carriers") || []).filter(energyPathSummaryIsSiteEnergyCarrier);
  const cooling = loads.filter((item) => energyPathSummaryService(item) === "cooling");
  const heating = loads.filter((item) => energyPathSummaryService(item) === "heating");
  return [
    {
      id: "total_site_energy",
      label: "Total site energy",
      value: energyPathSummaryTotal(carriers),
      unit: energyPathSummaryUnit(carriers, "kWh site"),
      nodeIds: carriers.map((item) => item.id || ""),
      nodeValues: carriers.map((item) => item.value),
    },
    {
      id: "cooling_load",
      label: "Cooling load",
      value: energyPathSummaryTotal(cooling),
      unit: "kWh thermal",
      nodeIds: cooling.map((item) => item.id || ""),
      nodeValues: cooling.map((item) => item.value),
    },
    {
      id: "heating_load",
      label: "Heating load",
      value: energyPathSummaryTotal(heating),
      unit: "kWh thermal",
      nodeIds: heating.map((item) => item.id || ""),
      nodeValues: heating.map((item) => item.value),
    },
    {
      id: "coverage",
      label: "Energy-path coverage",
      // Availability and the two accounting boundaries have different
      // denominators. They are never collapsed into a legacy scalar score.
      value: null,
      unit: "",
      nodeIds: [],
      nodeValues: [],
    },
  ];
}

function energyPathSummaryTotal(items = []) {
  if (!items.length) return null;
  let total = 0;
  for (const item of items) {
    const input = item?.value;
    if (!(typeof input === "number" || typeof input === "string" && input.trim() !== "") || !Number.isFinite(Number(input))) return null;
    total += Number(input);
  }
  return Number.isFinite(total) ? total : null;
}

function energyPathSummaryUnit(items = [], fallback = "") {
  return (items || []).find((item) => item?.unit)?.unit || fallback;
}

function energyPathSummaryIsSiteEnergyCarrier(item = {}) {
  const carrier = energyPathSummaryCarrier(item);
  const unit = String(item.unit || "").trim().toLowerCase().replace(/[\s_-]+/g, "");
  if (!ENERGY_PATH_SITE_CARRIERS.has(carrier) || (unit !== "kwh" && unit !== "kwhsite")) return false;
  if (carrier !== "water") return true;
  return String(item.basis || "").trim().toLowerCase() === "derived_ratio";
}

function energyPathSummaryCarrier(item = {}) {
  if (item.carrier) return String(item.carrier).trim().toLowerCase();
  const identity = String(item.id || item.kind || "").trim().toLowerCase();
  const parts = identity.split(".");
  const carrierIndex = parts.indexOf("carrier");
  if (carrierIndex >= 0 && parts[carrierIndex + 1]) return parts[carrierIndex + 1];
  return "";
}

function energyPathSummaryService(item = {}) {
  const explicit = String(item.serviceKind || "").trim().toLowerCase();
  if (explicit) return ["cooling", "heating"].includes(explicit) ? explicit : "";
  return String(item.kind || item.id || "").toLowerCase().match(/(?:^|[._:-])(cooling|heating)(?:$|[._:-])/)?.[1] || "";
}
