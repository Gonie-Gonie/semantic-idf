import { escapeHTML } from "../state.js";
import { getLanguage, t } from "../i18n.js";
import { heatFlowCategoryValue, heatFlowCategoryLabel, formatHeatFlowWatts, heatFlowExactWatts, heatFlowFrameTime } from "../heat-flow-data.js";

const indexes = new WeakMap();
const key = value => String(value || "").trim().toLowerCase().replace(/\s+/g, " ");
const round = value => Number(value).toFixed(2);

function exchangeIndex(overlay) {
  if (!overlay || typeof overlay !== "object") return null;
  if (indexes.has(overlay)) return indexes.get(overlay);
  const period = overlay.periods?.find(item => item.id === "hourly" && item.kind === "hourly");
  const labels = new Map();
  for (const [index, label] of (period?.labels || []).entries()) labels.set(label, labels.has(label) ? -1 : index);
  const nodes = new Map((overlay.planGeometry?.topology?.nodes || []).map(node => [node.id, node]));
  const byZone = new Map();
  for (const flow of period?.boundaryFlows || []) {
    if (flow.unit !== "kWh" || flow.signConvention !== "positive enters owner" || !flow.observed) continue;
    for (const nodeID of new Set([flow.ownerNodeId, flow.targetNodeId])) {
      const node = nodes.get(nodeID);
      if (node?.kind !== "zone") continue;
      const zoneKey = key(node.zoneName || node.label);
      const flows = byZone.get(zoneKey) || [];
      flows.push(flow);
      byZone.set(zoneKey, flows);
    }
  }
  const index = { labels, nodes, byZone };
  indexes.set(overlay, index);
  return index;
}

export function heatFlowMeasuredExchanges(overlay, zoneName, frameLabel) {
  if (!Number.isFinite(heatFlowFrameTime(frameLabel))) return [];
  const index = exchangeIndex(overlay);
  const frame = index?.labels.get(frameLabel);
  if (!Number.isInteger(frame) || frame < 0) return [];
  const groups = new Map();
  for (const flow of index.byZone.get(key(zoneName)) || []) {
    const value = flow.values?.[frame];
    if (flow.observed?.[frame] !== true || typeof value !== "number" || !Number.isFinite(value)) continue;
    const owner = index.nodes.get(flow.ownerNodeId);
    const selectedOwner = key(owner?.zoneName || owner?.label) === key(zoneName);
    const peerID = selectedOwner ? flow.targetNodeId : flow.ownerNodeId;
    const peer = index.nodes.get(peerID);
    if (!peer || peerID === (selectedOwner ? flow.ownerNodeId : flow.targetNodeId)) continue;
    const inward = selectedOwner ? value : -value;
    const group = groups.get(peerID) || { peerID, peerName: peer.zoneName || peer.label || peerID,
      peerKind: peer.kind, incoming: 0, outgoing: 0, boundaryIDs: [], sourceIDs: [] };
    group[inward >= 0 ? "incoming" : "outgoing"] += Math.abs(inward);
    group.boundaryIDs.push(flow.boundaryId);
    group.sourceIDs.push(...(flow.sourceIds || []));
    groups.set(peerID, group);
  }
  return [...groups.values()].sort((a, b) => a.peerName.localeCompare(b.peerName));
}

export function heatFlowAggregateExchanges(dataset, zone, frame) {
  return (dataset?.categories || []).flatMap((category, index) => {
    if (!["interzoneAir", "outdoorAir", "surfaceConvection"].includes(category.id)) return [];
    const value = heatFlowCategoryValue(zone, index, frame);
    return [{ id: category.id, label: heatFlowCategoryLabel(category), value }];
  });
}

function energy(value) {
  return `${new Intl.NumberFormat(getLanguage(), { minimumFractionDigits: 3, maximumFractionDigits: 3 }).format(value)} kWh`;
}

export function renderHeatFlowExchangeDetails(dataset, zone, zoneName, frame, overlay) {
  const aggregate = heatFlowAggregateExchanges(dataset, zone, frame);
  const measured = heatFlowMeasuredExchanges(overlay, zoneName, dataset.labels?.[frame]);
  const unavailable = t("simulation.heatFlowUnavailable", {}, "Unavailable");
  const rows = aggregate.map(item => {
    const available = Number.isFinite(item.value);
    const direction = !available ? "unavailable" : item.value === 0 ? "neutral" : item.value > 0 ? "incoming" : "outgoing";
    return `<div class="heatflow-exchange-row ${direction}" data-heatflow-aggregate="${escapeHTML(item.id)}" data-value="${available ? item.value : ""}">
      <span>${escapeHTML(item.label)}</span><b class="heatflow-direction" aria-label="${escapeHTML(!available ? unavailable : item.value === 0 ? t("simulation.heatFlowNoTransfer", {}, "No net transfer") : item.value > 0 ? t("simulation.heatFlowIntoZone", {}, "Into zone") : t("simulation.heatFlowOutOfZone", {}, "Out of zone"))}">${!available ? "—" : item.value === 0 ? "↔" : item.value > 0 ? "→" : "←"}</b>
      <strong title="${escapeHTML(heatFlowExactWatts(item.value))}">${escapeHTML(formatHeatFlowWatts(item.value))}</strong>
    </div>`;
  }).join("");
  const pairRows = measured.map(item => `<div class="heatflow-surface-exchange" data-heatflow-peer="${escapeHTML(item.peerID)}">
    <span>${escapeHTML(item.peerName)}</span>
    <span class="incoming">→ ${escapeHTML(t("simulation.heatFlowIn", {}, "In"))} <strong data-heatflow-pair-in="${item.incoming}">${energy(item.incoming)}</strong></span>
    <span class="outgoing">← ${escapeHTML(t("simulation.heatFlowOut", {}, "Out"))} <strong data-heatflow-pair-out="${item.outgoing}">${energy(item.outgoing)}</strong></span>
  </div>`).join("");
  return `<section class="heatflow-exchanges"><h5>${escapeHTML(t("simulation.heatFlowExchangeHeading", {}, "Exchange with the selected zone"))}</h5>
    <p>${escapeHTML(t("simulation.heatFlowAggregateNote", {}, "Arrows point into or out of this zone. Air and surface-convection values are zone totals (kW), without an assigned neighbouring zone."))}</p>
    ${rows}
    <h5>${escapeHTML(t("simulation.heatFlowSurfaceExchangeHeading", {}, "Measured surface exchange · kWh / reported interval"))}</h5>
    <p>${escapeHTML(t("simulation.heatFlowSurfaceExchangeNote", {}, "Surface conduction and window exchange use the executed model and exact matching timestamps. These interval energies are separate from the zone-air balance above."))}</p>
    ${pairRows || `<p class="heatflow-unavailable">${escapeHTML(t("simulation.heatFlowPairUnavailable", {}, "No verified surface-exchange observations at this time. A new Surface-detail run provides the required frame and geometry evidence."))}</p>`}
  </section>`;
}

export function renderHeatFlowExchangeArrows(exchanges, selectedCenter, centers, { width, height, markerID }) {
  if (!selectedCenter || !exchanges.length) return "";
  const visible = exchanges.filter(item => item.incoming > 0 || item.outgoing > 0);
  const scale = Math.max(0.001, ...visible.map(item => Math.max(item.incoming, item.outgoing)));
  const parts = [];
  for (const [index, item] of visible.entries()) {
    const peer = item.peerKind === "zone" ? centers.get(key(item.peerName)) : null;
    const target = peer || { x: selectedCenter.x < width / 2 ? width - 32 : 32, y: 54 + index * 44 % Math.max(height - 110, 44) };
    const dx = target.x - selectedCenter.x, dy = target.y - selectedCenter.y;
    const length = Math.hypot(dx, dy);
    if (length < 24) continue;
    const nx = -dy / length, ny = dx / length;
    for (const direction of ["incoming", "outgoing"]) {
      const value = item[direction];
      if (value <= 0) continue;
      const offset = direction === "incoming" ? -6 : 6;
      const from = direction === "incoming" ? target : selectedCenter;
      const to = direction === "incoming" ? selectedCenter : target;
      const ux = (to.x - from.x) / length, uy = (to.y - from.y) / length;
      const x1 = from.x + ux * 24 + nx * offset, y1 = from.y + uy * 24 + ny * offset;
      const x2 = to.x - ux * 24 + nx * offset, y2 = to.y - uy * 24 + ny * offset;
      const strokeWidth = 2.5 + 3 * Math.sqrt(value / scale);
      parts.push(`<g class="heatflow-measured-arrow ${direction}" data-heatflow-arrow="${direction}" data-peer="${escapeHTML(item.peerID)}" data-value="${value}" data-unit="kWh">
        <title>${escapeHTML(`${item.peerName} ${direction === "incoming" ? "→" : "←"} ${energy(value)}`)}</title>
        <path d="M ${round(x1)} ${round(y1)} L ${round(x2)} ${round(y2)}" stroke-width="${round(strokeWidth)}" marker-end="url(#${markerID}-${direction})"></path>
      </g>`);
    }
    if (!peer) parts.push(`<text class="heatflow-external-label" x="${round(target.x)}" y="${round(target.y - 10)}" text-anchor="${target.x > width / 2 ? "end" : "start"}">${escapeHTML(item.peerName)}</text>`);
  }
  return `<defs>${["incoming", "outgoing"].map(direction => `<marker id="${markerID}-${direction}" markerWidth="10" markerHeight="10" refX="8" refY="5" orient="auto" markerUnits="userSpaceOnUse"><path d="M 0 0 L 9 5 L 0 10 Z" class="${direction}"></path></marker>`).join("")}</defs><g class="heatflow-measured-arrows">${parts.join("")}</g>`;
}
