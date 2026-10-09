import { getLanguage, t } from "./i18n.js";

const exchangeCategories = new Set(["surfaceConvection", "interzoneAir", "outdoorAir"]);
const diagnosticCategories = new Set(["airStorage", "deviation"]);
const physicalCategories = ["internalConvective", "surfaceConvection", "interzoneAir", "outdoorAir", "systemAir", "systemConvective"];
const presentationExtents = new WeakMap();
const presentationReferenceWatts = 1;

export function heatFlowCategoryKind(category) {
  return diagnosticCategories.has(category?.id) ? "diagnostic" : exchangeCategories.has(category?.id) ? "exchange" : "local";
}

export function heatFlowCategoryValue(zone, categoryIndex, frameIndex) {
  if (zone?.observed && zone.observed?.[categoryIndex]?.[frameIndex] !== true) return NaN;
  const value = zone?.values?.[categoryIndex]?.[frameIndex];
  return typeof value === "number" && Number.isFinite(value) ? value : NaN;
}

export function heatFlowZoneTemperature(zone, frameIndex) {
  if (zone?.temperatureObserved && zone.temperatureObserved?.[frameIndex] !== true) return NaN;
  const value = zone?.temperature?.[frameIndex];
  return typeof value === "number" && Number.isFinite(value) ? value : NaN;
}

export function heatFlowBalance(dataset, zone, frameIndex) {
  const balance = { gains: 0, losses: 0, localGains: 0, localLosses: 0, exchangeGains: 0, exchangeLosses: 0,
    net: NaN, storage: NaN, deviation: NaN, residual: NaN, complete: false, localComplete: false, exchangeComplete: false, reportedCount: 0, missingCount: 0 };
  const reported = new Set();
  for (const [index, category] of (dataset?.categories || []).entries()) {
    const value = heatFlowCategoryValue(zone, index, frameIndex);
    if (category.id === "airStorage") { balance.storage = value; continue; }
    if (category.id === "deviation") { balance.deviation = value; continue; }
    if (!Number.isFinite(value)) { balance.missingCount++; continue; }
    reported.add(category.id);
    balance.reportedCount++;
    const kind = heatFlowCategoryKind(category);
    balance[value >= 0 ? "gains" : "losses"] += value;
    balance[`${kind}${value >= 0 ? "Gains" : "Losses"}`] += value;
  }
  balance.complete = physicalCategories.every(id => reported.has(id));
  balance.localComplete = physicalCategories.filter(id => !exchangeCategories.has(id)).every(id => reported.has(id));
  balance.exchangeComplete = [...exchangeCategories].every(id => reported.has(id));
  if (![...reported].some(id => !exchangeCategories.has(id))) balance.localGains = balance.localLosses = NaN;
  if (![...reported].some(id => exchangeCategories.has(id))) balance.exchangeGains = balance.exchangeLosses = NaN;
  if (balance.reportedCount) balance.net = balance.gains + balance.losses;
  else balance.gains = balance.losses = balance.localGains = balance.localLosses = balance.exchangeGains = balance.exchangeLosses = NaN;
  if (balance.complete && Number.isFinite(balance.storage)) balance.residual = balance.net - balance.storage;
  return balance;
}

export function heatFlowCategoryLabel(category) {
  return t(`simulation.heatFlowCategory.${category?.id}`, {}, category?.label || category?.id || "");
}

export function heatFlowPresentationExtents(dataset) {
  const extents = { localMax: NaN, netMax: NaN, rateMax: NaN, localCount: 0, netCount: 0, rateCount: 0 };
  if (!dataset || typeof dataset !== "object") return extents;
  if (presentationExtents.has(dataset)) return presentationExtents.get(dataset);
  const frameCount = Math.max(0, Number(dataset.frameCount) || dataset.labels?.length || 0);
  const includeRate = value => {
    if (!Number.isFinite(value)) return;
    extents.rateMax = Math.max(extents.rateCount ? extents.rateMax : 0, Math.abs(value));
    extents.rateCount++;
  };
  // A fixed reference across every zone and supplied frame. Cache the one
  // history scan; selection, Story filters and playback never change the scale.
  for (const zone of dataset.zones || []) {
    for (let frame = 0; frame < frameCount; frame++) {
      const balance = heatFlowBalance(dataset, zone, frame);
      if (Number.isFinite(balance.net)) {
        extents.netMax = Math.max(extents.netCount ? extents.netMax : 0, Math.abs(balance.net));
        extents.netCount++;
      }
      if (Number.isFinite(balance.localGains) && Number.isFinite(balance.localLosses)) {
        extents.localMax = Math.max(extents.localCount ? extents.localMax : 0, balance.localGains, Math.abs(balance.localLosses));
        extents.localCount++;
      }
      for (let categoryIndex = 0; categoryIndex < (dataset.categories?.length || 0); categoryIndex++) {
        includeRate(heatFlowCategoryValue(zone, categoryIndex, frame));
      }
      includeRate(balance.residual);
    }
  }
  presentationExtents.set(dataset, extents);
  return extents;
}

// A disclosed symmetric log scale with one physical 1 W reference. For stacks,
// map each sign's total and distribute its height by the original value shares.
// Keep displayed amounts unchanged; a missing observation is never zero.
export function heatFlowPresentationRatio(value, maximum) {
  if (typeof value !== "number" || !Number.isFinite(value)) return NaN;
  if (value === 0) return 0;
  if (typeof maximum !== "number" || !Number.isFinite(maximum) || maximum <= 0) return NaN;
  return Math.sign(value) * Math.log1p(Math.abs(value) / presentationReferenceWatts)
    / Math.log1p(maximum / presentationReferenceWatts);
}

export function formatHeatFlowWatts(value, { signed = true, unit = "kW", digits = unit === "kW" ? 2 : 3 } = {}) {
  if (typeof value !== "number" || !Number.isFinite(value)) return "—";
  const number = unit === "kW" ? value / 1000 : value;
  // Suppress signed rounded zero in kW labels; exact W tooltips and source
  // values retain the direction and precision of these small transfers.
  const displayNumber = unit === "kW" && Math.abs(number) < .5 * 10 ** -digits ? 0 : number;
  const text = new Intl.NumberFormat(getLanguage(), { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(displayNumber);
  return `${signed && displayNumber > 0 ? "+" : ""}${text} ${unit}`;
}

export function heatFlowExactWatts(value) {
  return formatHeatFlowWatts(value, { unit: "W", digits: 6 });
}

export function heatFlowFrameTime(label) {
  const match = String(label || "").trim().match(/^(?:(\d{4})[-/])?(\d{1,2})[-/](\d{1,2})\s+(\d{1,2}):(\d{2})(?::(\d{2}))?$/);
  if (!match) return NaN;
  const [, rawYear, rawMonth, rawDay, rawHour, rawMinute, rawSecond] = match;
  const year = Number(rawYear || 2000), month = Number(rawMonth), day = Number(rawDay), hour = Number(rawHour), minute = Number(rawMinute), second = Number(rawSecond || 0);
  if (month < 1 || month > 12 || day < 1 || day > 31 || hour > 24 || minute > 59 || second > 59 || hour === 24 && (minute || second)) return NaN;
  const date = new Date(Date.UTC(year, month - 1, day));
  if (date.getUTCMonth() !== month - 1 || date.getUTCDate() !== day) return NaN;
  return date.getTime() + (hour * 3600 + minute * 60 + second) * 1000;
}
