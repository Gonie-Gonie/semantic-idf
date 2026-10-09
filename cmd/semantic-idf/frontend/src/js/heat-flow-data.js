import { getLanguage, t } from "./i18n.js";

const exchangeCategories = new Set(["surfaceConvection", "interzoneAir", "outdoorAir"]);
const diagnosticCategories = new Set(["airStorage", "deviation"]);
const physicalCategories = ["internalConvective", "surfaceConvection", "interzoneAir", "outdoorAir", "systemAir", "systemConvective"];
const presentationExtents = new WeakMap();

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
  if (!dataset || typeof dataset !== "object") return { localMax: 1, netMax: 1 };
  if (presentationExtents.has(dataset)) return presentationExtents.get(dataset);
  const extents = { localMax: 1, netMax: 1 };
  for (const zone of dataset.zones || []) {
    for (let frame = 0; frame < (Number(dataset.frameCount) || dataset.labels?.length || 0); frame++) {
      const balance = heatFlowBalance(dataset, zone, frame);
      extents.localMax = Math.max(extents.localMax, balance.localGains || 0, Math.abs(balance.localLosses || 0));
      extents.netMax = Math.max(extents.netMax, Math.abs(balance.net || 0));
    }
  }
  presentationExtents.set(dataset, extents);
  return extents;
}

export function formatHeatFlowWatts(value, { signed = true, unit = "kW", digits = 3 } = {}) {
  if (typeof value !== "number" || !Number.isFinite(value)) return "—";
  const number = unit === "kW" ? value / 1000 : value;
  const text = new Intl.NumberFormat(getLanguage(), { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(number);
  return `${signed && value > 0 ? "+" : ""}${text} ${unit}`;
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
