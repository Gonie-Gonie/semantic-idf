import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";
import { heatFlowPresentationExtents, heatFlowPresentationRatio, formatHeatFlowWatts, heatFlowExactWatts } from "../heat-flow-data.js";

// Rate bars share one fixed logarithmic scale across every zone/timestamp.
// Measured interval energies never enter this rate scale.
export function heatFlowLedgerMaximum(dataset) {
  return heatFlowPresentationExtents(dataset).rateMax;
}

export function renderHeatFlowLedgerAxis(maximum) {
  if (!Number.isFinite(maximum)) return `<div class="heatflow-ledger-axis">${escapeHTML(t("simulation.heatFlowUnavailable", {}, "Unavailable"))}</div>`;
  return `<div class="heatflow-ledger-axis" aria-hidden="true"><span>${escapeHTML(formatHeatFlowWatts(-maximum))}</span><span>${escapeHTML(formatHeatFlowWatts(0, { signed: false }))}</span><span>${escapeHTML(formatHeatFlowWatts(maximum))}</span></div>`;
}

export function renderHeatFlowLedgerRow({ id, label, value, maximum, color, aggregate = false, kpi = "", sourceTooltip = "" }) {
  const observed = Number.isFinite(value);
  const direction = !observed ? "unavailable" : value > 0 ? "incoming" : value < 0 ? "outgoing" : "neutral";
  const width = observed && maximum > 0 ? Math.min(1, Math.abs(heatFlowPresentationRatio(value, maximum))) * 50 : 0;
  const left = value < 0 ? 50 - width : 50;
  const directionLabel = !observed ? t("simulation.heatFlowUnavailable", {}, "Unavailable")
    : value === 0 ? t("simulation.heatFlowNoTransfer", {}, "No net transfer")
    : value > 0 ? t("simulation.heatFlowIntoZone", {}, "Into zone") : t("simulation.heatFlowOutOfZone", {}, "Out of zone");
  return `<div class="heatflow-ledger-row heatflow-bar-row ${direction}" data-heatflow-ledger="${escapeHTML(id)}" data-value="${observed ? value : ""}" data-unit="W" data-observed="${observed}" data-bar-max="${Number.isFinite(maximum) ? maximum : ""}" data-direction="${direction}" ${aggregate ? `data-heatflow-aggregate="${escapeHTML(id)}"` : ""}>
    <div class="heatflow-bar-label"><span>${color ? `<i style="--legend-color: ${escapeHTML(color)}"></i>` : ""}${escapeHTML(label)}</span>
      <strong ${kpi ? `data-heatflow-kpi="${escapeHTML(kpi)}"` : ""} title="${escapeHTML(sourceTooltip || heatFlowExactWatts(value))}">${escapeHTML(formatHeatFlowWatts(value))}</strong></div>
    <div class="heatflow-value-bar" aria-label="${escapeHTML(directionLabel)}">${width > 0 ? `<i data-heatflow-bar-fill style="left: ${left}%; width: ${width}%"></i>` : ""}${!observed ? `<span class="heatflow-bar-unavailable">${escapeHTML(directionLabel)}</span>` : ""}</div>
  </div>`;
}
