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
  const carriers = groups.get("carriers") || [];
  return [
    {
      id: "total_site_energy",
      label: "Total site energy",
      value: energyPathSummaryTotal(carriers),
      unit: energyPathSummaryUnit(carriers, "kWh site"),
    },
    {
      id: "cooling_load",
      label: "Cooling load",
      value: energyPathSummaryTotal(loads.filter((item) => energyPathSummaryService(item) === "cooling")),
      unit: "kWh thermal",
    },
    {
      id: "heating_load",
      label: "Heating load",
      value: energyPathSummaryTotal(loads.filter((item) => energyPathSummaryService(item) === "heating")),
      unit: "kWh thermal",
    },
    {
      id: "coverage",
      label: "Coverage",
      value: Number(summary.completeness?.mappedPercent),
      unit: "%",
      status: summary.completeness?.status || "",
    },
  ];
}

function energyPathSummaryTotal(items = []) {
  return (items || []).reduce((sum, item) => {
    const value = Number(item?.value);
    return Number.isFinite(value) ? sum + value : sum;
  }, 0);
}

function energyPathSummaryUnit(items = [], fallback = "") {
  return (items || []).find((item) => item?.unit)?.unit || fallback;
}

function energyPathSummaryService(item = {}) {
  const value = String(item.serviceKind || item.kind || item.id || "").toLowerCase();
  if (value.includes("cool")) {
    return "cooling";
  }
  if (value.includes("heat")) {
    return "heating";
  }
  return "";
}
