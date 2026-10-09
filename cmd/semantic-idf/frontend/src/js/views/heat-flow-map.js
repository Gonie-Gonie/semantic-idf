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
    <details class="heatflow-surface-details"><summary>${escapeHTML(t("simulation.heatFlowSurfaceExchangeHeading", {}, "Measured surface exchange · kWh / reported interval"))}</summary>
      <p>${escapeHTML(t("simulation.heatFlowSurfaceExchangeNote", {}, "Surface conduction and window exchange use the executed model and exact matching timestamps. These interval energies are separate from the zone-air balance above."))}</p>
      ${pairRows || `<p class="heatflow-unavailable">${escapeHTML(t("simulation.heatFlowPairUnavailable", {}, "No verified surface-exchange observations at this time. A new Surface-detail run provides the required frame and geometry evidence."))}</p>`}
    </details>
  </section>`;
}

function compactArrowExchanges(exchanges, centers) {
  const groups = new Map();
  for (const item of exchanges) {
    const outdoors = item.peerKind === "outdoors" || item.peerKind?.startsWith("outdoors_");
    const zoneKey = key(item.peerName);
    if (!outdoors && (item.peerKind !== "zone" || !centers.has(zoneKey))) continue;
    const groupKey = outdoors ? "outdoors" : `zone:${zoneKey}`;
    const group = groups.get(groupKey) || { peerID: outdoors ? "outdoors" : item.peerID,
      peerName: outdoors ? t("simulation.heatFlowOutside", {}, "Outside") : item.peerName, peerKind: outdoors ? "outdoors" : "zone",
      incoming: 0, outgoing: 0, peerIDs: [], boundaryIDs: [], sourceIDs: [] };
    group.incoming += item.incoming;
    group.outgoing += item.outgoing;
    group.peerIDs.push(item.peerID);
    group.boundaryIDs.push(...(item.boundaryIDs || []));
    group.sourceIDs.push(...(item.sourceIDs || []));
    groups.set(groupKey, group);
  }
  return [...groups.entries()].sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0)
    .map(([, item]) => ({ ...item, peerIDs: [...new Set(item.peerIDs)],
      boundaryIDs: [...new Set(item.boundaryIDs)], sourceIDs: [...new Set(item.sourceIDs)] }))
    .filter(item => item.incoming > 0 || item.outgoing > 0);
}

function trimmedPoint(center, toward, fallbackRadius) {
  const distance = Math.hypot(toward.x - center.x, toward.y - center.y);
  const trim = (center.radius ?? fallbackRadius) + 4;
  return { x: center.x + (toward.x - center.x) / distance * trim,
    y: center.y + (toward.y - center.y) / distance * trim };
}

function arrowRoute(selected, target, controls, badgeRadius) {
  return { start: trimmedPoint(selected, controls[0] || target, badgeRadius),
    end: trimmedPoint(target, controls.at(-1) || selected, badgeRadius), controls };
}

function routePoint(route, t) {
  const u = 1 - t, { start, end, controls } = route;
  if (controls.length === 1) return { x: u * u * start.x + 2 * u * t * controls[0].x + t * t * end.x,
    y: u * u * start.y + 2 * u * t * controls[0].y + t * t * end.y };
  if (controls.length === 2) return { x: u * u * u * start.x + 3 * u * u * t * controls[0].x + 3 * u * t * t * controls[1].x + t * t * t * end.x,
    y: u * u * u * start.y + 3 * u * u * t * controls[0].y + 3 * u * t * t * controls[1].y + t * t * t * end.y };
  return { x: u * start.x + t * end.x, y: u * start.y + t * end.y };
}

function routePenalty(route, circles, rectangles, width, height) {
  const points = [route.start, ...route.controls, route.end];
  const controlLength = points.slice(1).reduce((sum, point, index) => sum + Math.hypot(point.x - points[index].x, point.y - points[index].y), 0);
  const samples = Math.min(256, Math.max(24, Math.ceil(controlLength / 4)));
  let penalty = 0;
  for (let index = 0; index <= samples; index++) {
    const point = routePoint(route, index / samples);
    if (point.x < 2 || point.y < 2 || point.x > width - 2 || point.y > height - 2) penalty += 1;
    for (const circle of circles) {
      const distance = Math.hypot(point.x - circle.x, point.y - circle.y);
      if (distance < circle.clearance) penalty += 100 + (circle.clearance - distance);
    }
    for (const rectangle of rectangles) {
      if (point.x > rectangle.left && point.x < rectangle.right && point.y > rectangle.top && point.y < rectangle.bottom) penalty += 100;
    }
  }
  return penalty / (samples + 1);
}

function chooseArrowRoute(selected, target, centers, rectangleObstacles, width, height, badgeRadius) {
  const dx = target.x - selected.x, dy = target.y - selected.y, length = Math.hypot(dx, dy);
  const nx = -dy / length, ny = dx / length;
  const samePosition = (a, b) => Math.hypot(a.x - b.x, a.y - b.y) < 0.1;
  const circles = [...centers.values()].filter(center => !samePosition(center, selected) && !samePosition(center, target))
    .map(center => ({ ...center, clearance: (center.radius ?? badgeRadius) + 6 }));
  const rectangles = rectangleObstacles.filter(rectangle => [rectangle.left, rectangle.top, rectangle.right, rectangle.bottom].every(Number.isFinite))
    .map(rectangle => ({ left: rectangle.left - 6, top: rectangle.top - 6, right: rectangle.right + 6, bottom: rectangle.bottom + 6 }));
  let best = null, bestPenalty = Infinity;
  const consider = controls => {
    const route = arrowRoute(selected, target, controls, badgeRadius);
    const penalty = routePenalty(route, circles, rectangles, width, height);
    if (penalty < bestPenalty) { best = route; bestPenalty = penalty; }
    return penalty === 0;
  };
  if (consider([])) return best;
  for (const bend of [24, 40, 64, 96, 144, 216, 320, 480]) {
    for (const sign of [1, -1]) {
      if (consider([{ x: (selected.x + target.x) / 2 + nx * bend * sign,
        y: (selected.y + target.y) / 2 + ny * bend * sign }])) return best;
    }
  }
  for (const bend of [64, 128, 256, 384]) {
    for (const sign of [1, -1]) {
      for (const otherSign of [sign, -sign]) {
        if (consider([{ x: selected.x + dx / 3 + nx * bend * sign, y: selected.y + dy / 3 + ny * bend * sign },
          { x: selected.x + dx * 2 / 3 + nx * bend * otherSign, y: selected.y + dy * 2 / 3 + ny * bend * otherSign }])) return best;
      }
    }
  }
  return best;
}

function arrowPath(route, reverse) {
  const start = reverse ? route.end : route.start, end = reverse ? route.start : route.end;
  const controls = reverse ? [...route.controls].reverse() : route.controls;
  const point = value => `${round(value.x)} ${round(value.y)}`;
  const command = controls.length === 2 ? "C" : controls.length === 1 ? "Q" : "L";
  return `M ${point(start)} ${command} ${[...controls, end].map(point).join(" ")}`;
}

export function renderHeatFlowExchangeArrows(exchanges, selectedCenter, centers,
  { width, height, markerID, outsideCenter, badgeRadius = 12, renderOutsideBadge = true }) {
  if (!selectedCenter || !exchanges.length) return "";
  const visible = compactArrowExchanges(exchanges, centers);
  const outside = outsideCenter || { x: width - 32, y: Math.min(28, height / 2), radius: 24 };
  const outsideHalfWidth = Math.max(28, outside.radius ?? 24);
  const outsideObstacle = { left: outside.x - outsideHalfWidth, right: outside.x + outsideHalfWidth,
    top: outside.y - 12, bottom: outside.y + 12 };
  const hasOutside = renderOutsideBadge && visible.some(item => item.peerKind === "outdoors");
  const parts = [];
  for (const item of visible) {
    const peer = item.peerKind === "zone" ? centers.get(key(item.peerName)) : null;
    const target = peer || outside;
    const dx = target.x - selectedCenter.x, dy = target.y - selectedCenter.y;
    const length = Math.hypot(dx, dy);
    const selectedTrim = (selectedCenter.radius ?? badgeRadius) + 4;
    const targetTrim = (target.radius ?? badgeRadius) + 4;
    if (length <= selectedTrim + targetTrim) continue;
    const difference = item.incoming - item.outgoing;
    const net = Math.abs(difference) <= Number.EPSILON * Math.max(1, item.incoming, item.outgoing) * 8 ? 0 : difference;
    const direction = net === 0 ? "neutral" : net > 0 ? "incoming" : "outgoing";
    const route = chooseArrowRoute(selectedCenter, target, centers,
      peer && hasOutside ? [outsideObstacle] : [], width, height, badgeRadius);
    const tooltip = `${item.peerName}: ${t("simulation.heatFlowSurfaceExchangeHeading", {}, "Measured surface exchange · kWh / reported interval")}\n`
      + `${t("simulation.heatFlowTotalNet", {}, "Total net")} ${energy(net)}; `
      + `${t("simulation.heatFlowIn", {}, "In")} ${energy(item.incoming)}; ${t("simulation.heatFlowOut", {}, "Out")} ${energy(item.outgoing)}\n`
      + `Boundary: ${item.boundaryIDs.join(", ")}\nSource: ${item.sourceIDs.join(", ")}`;
    parts.push(`<g class="heatflow-measured-arrow ${direction}" data-heatflow-arrow="${direction}" data-peer="${escapeHTML(item.peerID)}" data-peers="${escapeHTML(JSON.stringify(item.peerIDs))}" data-value="${net}" data-gross-in="${item.incoming}" data-gross-out="${item.outgoing}" data-unit="kWh" data-boundaries="${escapeHTML(JSON.stringify(item.boundaryIDs))}" data-sources="${escapeHTML(JSON.stringify(item.sourceIDs))}">
      <title>${escapeHTML(tooltip)}</title>
      <path d="${arrowPath(route, direction === "incoming")}" stroke-width="2" ${direction === "neutral" ? `marker-start="url(#${markerID}-${direction})"` : ""} marker-end="url(#${markerID}-${direction})"></path>
    </g>`);
    if (!peer && renderOutsideBadge) {
      parts.push(`<g class="heatflow-outside-badge" data-heatflow-outside-badge="1">
        <rect x="${round(target.x - outsideHalfWidth)}" y="${round(target.y - 12)}" width="${round(outsideHalfWidth * 2)}" height="24" rx="8"></rect>
        <text x="${round(target.x)}" y="${round(target.y + 4)}" text-anchor="middle">${escapeHTML(item.peerName)}</text>
      </g>`);
    }
  }
  if (!parts.length) return "";
  return `<defs>${["incoming", "outgoing", "neutral"].map(direction => `<marker id="${markerID}-${direction}" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto-start-reverse" markerUnits="userSpaceOnUse"><path d="M 0 0 L 7 3.5 L 0 7 Z" class="${direction}"></path></marker>`).join("")}</defs><g class="heatflow-measured-arrows">${parts.join("")}</g>`;
}
