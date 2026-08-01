import {
  elements,
  escapeHTML,
  normalizeThermalTopologyAreaBasis,
  normalizeThermalTopologyGraphLevel,
  normalizeThermalTopologyLayout,
  normalizeThermalTopologyMetric,
  normalizeThermalTopologyScope,
  state,
} from "../state.js";
import { t } from "../i18n.js";
import { recordViewHistory } from "../view-history.js";
import {
  computeThermalTopologyLayout,
  createThermalTopologyLayoutModel,
  THERMAL_NODE_HEIGHT,
  THERMAL_NODE_WIDTH,
  thermalTopologyLayoutCacheKey,
} from "./thermal-topology-layout.js";
import { renderThermalTopologyInspector } from "./thermal-topology-inspector.js";

let currentGeometry = null;
let currentHelpers = null;
let currentLayout = null;
let currentModel = null;
let resizeObserver = null;
let resizeFrame = 0;
let observedWidth = 0;
let observedHeight = 0;
let compactSelection = null;

window.addEventListener("idfAnalyzer:thermalTopologyFit", () => fitThermalTopology());

export function renderThermalTopology(geometry, helpers = {}) {
  if (!elements.thermalTopologyGraph) {
    return;
  }
  currentGeometry = geometry;
  currentHelpers = helpers;
  syncThermalTopologyControls();
  ensureResizeObserver();

  const viewport = graphViewport();
  const options = thermalTopologyOptions();
  const cacheKey = thermalTopologyLayoutCacheKey(geometry, options, viewport);
  currentModel = createThermalTopologyLayoutModel(geometry, options);
  currentLayout = state.thermalTopologyLayoutCache.get(cacheKey) || computeThermalTopologyLayout(currentModel, viewport);
  if (!state.thermalTopologyLayoutCache.has(cacheKey)) {
    state.thermalTopologyLayoutCache.set(cacheKey, currentLayout);
  }
  renderThermalTopologySVG(currentModel, currentLayout);
  elements.thermalTopologyMatrix.hidden = true;
  renderThermalTopologyInspector(geometry, helpers.navigationAttributes);
}

export function fitThermalTopology() {
  if (!currentLayout || !elements.thermalTopologyGraph) {
    state.thermalTopologyPanX = 0;
    state.thermalTopologyPanY = 0;
    state.thermalTopologyScale = 1;
    return;
  }
  const values = Object.values(currentLayout.positions || {});
  if (!values.length) {
    return;
  }
  const viewport = graphViewport();
  const minX = Math.min(...values.map((position) => position.x)) - THERMAL_NODE_WIDTH / 2;
  const maxX = Math.max(...values.map((position) => position.x)) + THERMAL_NODE_WIDTH / 2;
  const minY = Math.min(...values.map((position) => position.y)) - THERMAL_NODE_HEIGHT / 2;
  const maxY = Math.max(...values.map((position) => position.y)) + THERMAL_NODE_HEIGHT / 2;
  const scale = clamp(Math.min((viewport.width - 36) / Math.max(1, maxX - minX), (viewport.height - 36) / Math.max(1, maxY - minY)), 0.1, 3.5);
  state.thermalTopologyScale = scale;
  state.thermalTopologyPanX = viewport.width / 2 - ((minX + maxX) / 2) * scale;
  state.thermalTopologyPanY = viewport.height / 2 - ((minY + maxY) / 2) * scale;
  applyGraphTransform();
}

function renderThermalTopologySVG(model, layout) {
  const selectedID = state.thermalTopologySelectedEntityId || state.selectedGeometryId;
  const edges = [...layout.edges].sort((left, right) => Number(left.id === selectedID) - Number(right.id === selectedID));
  const nodes = [...model.nodes].sort((left, right) => String(left.id).localeCompare(String(right.id)));
  const backButton = state.thermalTopologyGraphLevel === "boundary"
    ? `<button class="thermal-topology-back" type="button" data-topology-back>${escapeHTML(t("action.back", {}, "Back"))}</button>`
    : "";
  elements.thermalTopologyGraph.innerHTML = `${backButton}
    <svg class="thermal-topology-svg" viewBox="0 0 ${layout.width} ${layout.height}" preserveAspectRatio="xMidYMid meet" tabindex="0" aria-label="${escapeHTML(t("topology.thermalTooltip"))}">
      <defs>
        <marker id="thermalTopologyArrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
          <path d="M 0 0 L 10 5 L 0 10 z"></path>
        </marker>
        <marker id="thermalTopologyAirArrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
          <path d="M 0 0 L 10 5 L 0 10 z"></path>
        </marker>
      </defs>
      <g class="thermal-topology-panzoom" transform="${graphTransform()}">
        <g class="thermal-topology-edges">${edges.map((edge) => renderEdge(edge, model, selectedID)).join("")}</g>
        <g class="thermal-topology-nodes">${nodes.map((node) => renderNode(node, layout.positions[node.id], selectedID)).join("")}</g>
      </g>
    </svg>`;
  bindGraphInteractions();
}

function renderEdge(edge, model, selectedID) {
  const selected = edge.id === selectedID;
  const air = edge.relationKind === "air_coupling" || (edge.airCouplingIds || []).length > 0;
  const invalid = edge.qaOnly || edge.relationKind === "invalid";
  const classes = ["thermal-edge", air ? "air" : "conductive", invalid ? "invalid" : "", selected ? "selected" : ""].filter(Boolean).join(" ");
  const label = connectionLabel(edge, model);
  const targetKind = "thermal_connection";
  const attributes = currentHelpers?.navigationAttributes?.(targetKind, baseExpandedID(edge.id)) || "";
  return `<g class="thermal-edge-group navigable-row" data-thermal-target-kind="${targetKind}" data-thermal-target-id="${escapeHTML(baseExpandedID(edge.id))}" ${attributes}>
    <path class="${classes}" d="${edge.route.path}" marker-end="url(#${air ? "thermalTopologyAirArrow" : "thermalTopologyArrow"})"></path>
    <path class="thermal-edge-hit" d="${edge.route.path}" tabindex="0" role="button" aria-label="${escapeHTML(label)}"></path>
    ${state.thermalTopologyShowLabels ? `<text class="thermal-edge-label${selected ? " selected" : ""}" x="${edge.route.labelX}" y="${edge.route.labelY}">${escapeHTML(label)}</text>` : ""}
  </g>`;
}

function renderNode(node, position, selectedID) {
  if (!position) return "";
  const external = node.kind !== "zone" && node.kind !== "space";
  const selected = node.id === selectedID || node.entityId === selectedID;
  const targetKind = external ? "thermal_environment" : node.kind;
  const targetID = node.sourceId || node.entityId || node.id;
  const attributes = currentHelpers?.navigationAttributes?.(targetKind, targetID) || "";
  const label = trimLabel(node.label || node.objectName || node.id, 25);
  const subtitle = nodeSubtitle(node);
  return `<g class="thermal-node ${external ? "external" : "zone"}${selected ? " selected" : ""} navigable-row" transform="translate(${position.x} ${position.y})" data-thermal-target-kind="${escapeHTML(targetKind)}" data-thermal-target-id="${escapeHTML(targetID)}" ${attributes}>
    <rect x="${-THERMAL_NODE_WIDTH / 2}" y="${-THERMAL_NODE_HEIGHT / 2}" width="${THERMAL_NODE_WIDTH}" height="${THERMAL_NODE_HEIGHT}" rx="9"></rect>
    <circle class="thermal-node-port left" cx="${-THERMAL_NODE_WIDTH / 2}" cy="0" r="3"></circle>
    <circle class="thermal-node-port right" cx="${THERMAL_NODE_WIDTH / 2}" cy="0" r="3"></circle>
    <circle class="thermal-node-port top" cx="0" cy="${-THERMAL_NODE_HEIGHT / 2}" r="3"></circle>
    <circle class="thermal-node-port bottom" cx="0" cy="${THERMAL_NODE_HEIGHT / 2}" r="3"></circle>
    <text class="thermal-node-label" text-anchor="middle" y="-4">${escapeHTML(label)}</text>
    <text class="thermal-node-subtitle" text-anchor="middle" y="14">${escapeHTML(subtitle)}</text>
  </g>`;
}

function bindGraphInteractions() {
  const svg = elements.thermalTopologyGraph.querySelector(".thermal-topology-svg");
  if (!svg) return;
  let drag = null;
  svg.addEventListener("pointerdown", (event) => {
    if (event.button !== 0 || event.target.closest(".thermal-node, .thermal-edge-group")) return;
    drag = { pointerId: event.pointerId, x: event.clientX, y: event.clientY, panX: state.thermalTopologyPanX, panY: state.thermalTopologyPanY, moved: false };
    svg.setPointerCapture?.(event.pointerId);
    svg.classList.add("panning");
  });
  svg.addEventListener("pointermove", (event) => {
    if (!drag || drag.pointerId !== event.pointerId) return;
    const dx = event.clientX - drag.x;
    const dy = event.clientY - drag.y;
    drag.moved ||= Math.hypot(dx, dy) > 4;
    state.thermalTopologyPanX = drag.panX + dx;
    state.thermalTopologyPanY = drag.panY + dy;
    applyGraphTransform();
  });
  const finishDrag = (event) => {
    if (!drag || drag.pointerId !== event.pointerId) return;
    svg.releasePointerCapture?.(event.pointerId);
    drag = null;
    svg.classList.remove("panning");
  };
  svg.addEventListener("pointerup", finishDrag);
  svg.addEventListener("pointercancel", finishDrag);
  svg.addEventListener("wheel", (event) => {
    event.preventDefault();
    const bounds = svg.getBoundingClientRect();
    const x = (event.clientX - bounds.left) * (currentLayout.width / Math.max(1, bounds.width));
    const y = (event.clientY - bounds.top) * (currentLayout.height / Math.max(1, bounds.height));
    const previous = state.thermalTopologyScale;
    const next = clamp(previous * Math.exp(-event.deltaY * 0.0015), 0.1, 8);
    const worldX = (x - state.thermalTopologyPanX) / previous;
    const worldY = (y - state.thermalTopologyPanY) / previous;
    state.thermalTopologyScale = next;
    state.thermalTopologyPanX = x - worldX * next;
    state.thermalTopologyPanY = y - worldY * next;
    applyGraphTransform();
  }, { passive: false });

  elements.thermalTopologyGraph.querySelectorAll("[data-thermal-target-id]").forEach((element) => {
    element.addEventListener("click", (event) => {
      event.stopPropagation();
      activateGraphTarget(element.dataset.thermalTargetKind, element.dataset.thermalTargetId);
    });
    element.addEventListener("dblclick", (event) => {
      if (element.dataset.thermalTargetKind !== "thermal_connection") return;
      event.preventDefault();
      expandConnection(element.dataset.thermalTargetId);
    });
    element.addEventListener("keydown", (event) => {
      if (event.key === "Enter" && element.dataset.thermalTargetKind === "thermal_connection") {
        event.preventDefault();
        expandConnection(element.dataset.thermalTargetId);
      } else if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        activateGraphTarget(element.dataset.thermalTargetKind, element.dataset.thermalTargetId);
      }
    });
  });
  elements.thermalTopologyGraph.querySelector("[data-topology-back]")?.addEventListener("click", collapseBoundaryGraph);
}

function activateGraphTarget(kind, id) {
  state.thermalTopologySelectedEntityId = id;
  state.selectedGeometryKind = kind;
  state.selectedGeometryId = id;
  markGraphTargetSelected(kind, id);
  currentHelpers?.selectGeometry?.(kind, id, { syncLocate: true, syncSemantic: true });
  renderThermalTopologyInspector(currentGeometry, currentHelpers?.navigationAttributes);
}

function markGraphTargetSelected(kind, id) {
  elements.thermalTopologyGraph.querySelectorAll(".thermal-node.selected, .thermal-edge.selected, .thermal-edge-label.selected").forEach((element) => element.classList.remove("selected"));
  const target = [...elements.thermalTopologyGraph.querySelectorAll("[data-thermal-target-id]")]
    .find((element) => element.dataset.thermalTargetKind === kind && element.dataset.thermalTargetId === id);
  target?.classList.add("selected");
  target?.querySelector(".thermal-edge")?.classList.add("selected");
  target?.querySelector(".thermal-edge-label")?.classList.add("selected");
}

function expandConnection(id) {
  recordViewHistory();
  compactSelection = { kind: "thermal_connection", id };
  state.thermalTopologyGraphLevel = "boundary";
  state.thermalTopologyScope = "selection";
  state.thermalTopologySelectedEntityId = id;
  state.selectedGeometryKind = "thermal_connection";
  state.selectedGeometryId = id;
  renderThermalTopology(currentGeometry, currentHelpers);
}

function collapseBoundaryGraph() {
  recordViewHistory();
  state.thermalTopologyGraphLevel = "zone";
  if (compactSelection) {
    state.thermalTopologySelectedEntityId = compactSelection.id;
    state.selectedGeometryKind = compactSelection.kind;
    state.selectedGeometryId = compactSelection.id;
  }
  renderThermalTopology(currentGeometry, currentHelpers);
}

function applyGraphTransform() {
  const group = elements.thermalTopologyGraph.querySelector(".thermal-topology-panzoom");
  if (group) group.setAttribute("transform", graphTransform());
}

function graphTransform() {
  return `translate(${state.thermalTopologyPanX} ${state.thermalTopologyPanY}) scale(${state.thermalTopologyScale})`;
}

function ensureResizeObserver() {
  if (resizeObserver || typeof ResizeObserver === "undefined") return;
  resizeObserver = new ResizeObserver((entries) => {
    const bounds = entries[0]?.contentRect;
    if (!bounds || state.geometryMode !== "thermal") return;
    if (Math.abs(bounds.width - observedWidth) < 2 && Math.abs(bounds.height - observedHeight) < 2) return;
    observedWidth = bounds.width;
    observedHeight = bounds.height;
    window.cancelAnimationFrame(resizeFrame);
    resizeFrame = window.requestAnimationFrame(() => {
      if (currentGeometry && state.geometryMode === "thermal") renderThermalTopology(currentGeometry, currentHelpers);
    });
  });
  resizeObserver.observe(elements.thermalTopologyGraph);
}

function thermalTopologyOptions() {
  return {
    graphLevel: state.thermalTopologyGraphLevel,
    layout: state.thermalTopologyLayout,
    scope: state.thermalTopologyScope,
    storyIndex: state.selectedGeometryStory,
    selectedEntityId: state.thermalTopologySelectedEntityId || state.selectedGeometryId,
    neighborDepth: state.thermalTopologyNeighborDepth,
    areaBasis: state.thermalTopologyAreaBasis,
    showOpenings: state.thermalTopologyShowOpenings,
    showAirCoupling: state.thermalTopologyShowAirCoupling,
    expandExternalTargets: state.thermalTopologyExpandExternalTargets,
  };
}

function graphViewport() {
  return {
    width: Math.max(360, elements.thermalTopologyGraph.clientWidth || 900),
    height: Math.max(280, elements.thermalTopologyGraph.clientHeight || 600),
  };
}

function syncThermalTopologyControls() {
  elements.thermalTopologyGraphLevel.value = normalizeThermalTopologyGraphLevel(state.thermalTopologyGraphLevel);
  elements.thermalTopologyMetric.value = normalizeThermalTopologyMetric(state.thermalTopologyMetric);
  elements.thermalTopologyScope.value = normalizeThermalTopologyScope(state.thermalTopologyScope);
  elements.thermalTopologyLayout.value = normalizeThermalTopologyLayout(state.thermalTopologyLayout);
  elements.thermalTopologyAreaBasis.value = normalizeThermalTopologyAreaBasis(state.thermalTopologyAreaBasis);
  elements.thermalTopologyShowOpenings.checked = Boolean(state.thermalTopologyShowOpenings);
  elements.thermalTopologyShowAirCoupling.checked = Boolean(state.thermalTopologyShowAirCoupling);
  elements.thermalTopologyExpandExternalTargets.checked = Boolean(state.thermalTopologyExpandExternalTargets);
  elements.thermalTopologyShowLabels.checked = Boolean(state.thermalTopologyShowLabels);
}

function connectionLabel(connection, model) {
  const area = Number(connection?.[model.areaField]) || 0;
  const gross = Math.max(area, 0.000001);
  const openingField = model.areaBasis === "physical" ? "physicalOpeningArea" : "effectiveOpeningArea";
  const openingRatio = Math.max(0, Number(connection?.[openingField]) || 0) / gross;
  const parts = [`${Number(connection.surfaceCount) || (connection.boundaryIds || []).length} surfaces`];
  if (connection.hasUa) parts.push(`${formatNumber(connection.totalUa)} W/K`);
  else if (area > 0) parts.push(formatArea(area));
  if (openingRatio > 0) parts.push(`${Math.round(openingRatio * 100)}% open`);
  if ((connection.diagnosticIds || []).length) parts.push(`${connection.diagnosticIds.length} issues`);
  return parts.join(" · ");
}

function nodeSubtitle(node) {
  if (node.kind === "zone" || node.kind === "space") {
    return node.storyIndex === undefined ? node.kind : `${node.kind} · story ${Number(node.storyIndex) + 1}`;
  }
  return String(node.kind || "external").replaceAll("_", " ");
}

function baseExpandedID(id) {
  return String(id || "").replace(/:(north|south|east|west|roof|floor)$/i, "");
}

function trimLabel(value, maximum) {
  const text = String(value || "");
  return text.length > maximum ? `${text.slice(0, maximum - 1)}…` : text;
}

function formatArea(value) {
  return `${formatNumber(value)} m²`;
}

function formatNumber(value) {
  return Number(value || 0).toLocaleString(undefined, { maximumFractionDigits: 2 });
}

function clamp(value, minimum, maximum) {
  return Math.min(maximum, Math.max(minimum, Number(value) || minimum));
}
