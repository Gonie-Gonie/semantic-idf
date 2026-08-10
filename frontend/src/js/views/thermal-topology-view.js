import {
  elements,
  escapeHTML,
  normalizeThermalTopologyAreaComponent,
  normalizeThermalTopologyAreaBasis,
  normalizeThermalTopologyGraphLevel,
  normalizeThermalTopologyLayout,
  normalizeThermalTopologyMetric,
  normalizeThermalTopologyScope,
  state,
} from "../state.js";
import { t } from "../i18n.js";
import { clearSemanticHover, hoverSemanticEntity } from "../selection-controller.js";
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

const THERMAL_LAYOUT_CACHE_LIMIT = 24;

window.addEventListener("idfAnalyzer:thermalTopologyFit", () => fitThermalTopology());
window.addEventListener("idfAnalyzer:thermalTopologyExport", () => exportThermalTopologyJSON());

export function thermalTopologyExportPayload(geometry, areaBasis = "effective") {
  const topology = geometry?.topology;
  if (!topology) return null;
  return {
    ...topology,
    areaBasis: normalizeThermalTopologyAreaBasis(areaBasis),
  };
}

export function exportThermalTopologyJSON() {
  const payload = thermalTopologyExportPayload(currentGeometry, state.thermalTopologyAreaBasis);
  if (!payload) return false;
  const blob = new Blob([`${JSON.stringify(payload, null, 2)}\n`], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  const sourceName = String(state.currentFilename || "model.idf").replace(/\.(idf|json|epjson)$/i, "");
  link.href = url;
  link.download = `${sourceName}.thermal-topology.json`;
  link.click();
  URL.revokeObjectURL(url);
  return true;
}

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
  currentModel = createThermalTopologyLayoutModel(geometry, options);
  const cacheKey = thermalTopologyLayoutCacheKey(geometry, options, viewport);
  currentLayout = state.thermalTopologyLayoutCache.get(cacheKey) || computeThermalTopologyLayout(currentModel, viewport);
  if (!state.thermalTopologyLayoutCache.has(cacheKey)) {
    rememberThermalTopologyLayout(cacheKey, currentLayout);
  }
  renderThermalTopologySVG(currentModel, currentLayout);
  renderThermalTopologyInspector(geometry, helpers);
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
  const edges = [...layout.edges].sort((left, right) => String(left.targetId || left.id).localeCompare(String(right.targetId || right.id)));
  const nodes = [...model.nodes].sort((left, right) => String(left.id).localeCompare(String(right.id)));
  const metricContext = createMetricContext(model);
  const backButton = state.thermalTopologyGraphLevel === "boundary"
    ? `<button class="thermal-topology-back" type="button" data-topology-back>${escapeHTML(t("action.back", {}, "Back"))}</button>`
    : "";
  elements.thermalTopologyGraph.innerHTML = `${backButton}${renderMetricLegend()}
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
        <g class="thermal-topology-edges">${edges.map((edge) => renderEdge(edge, model, selectedID, metricContext)).join("")}</g>
        <g class="thermal-topology-nodes">${nodes.map((node) => renderNode(node, layout.positions[node.id], selectedID, model, metricContext)).join("")}</g>
      </g>
    </svg>`;
  bindGraphInteractions();
}

function renderEdge(edge, model, selectedID, metricContext) {
  const selected = edgeMatchesThermalSelection(edge, model);
  const air = edge.relationKind === "air_coupling" || (edge.airCouplingIds || []).length > 0;
  const invalid = edge.qaOnly || edge.relationKind === "invalid";
  const presentation = edgeMetricPresentation(edge, model, metricContext);
  const classes = ["thermal-edge", `relation-${cssToken(edge.relationKind)}`, air ? "air" : "conductive", invalid ? "invalid" : "", ...presentation.classes, selected ? "selected" : ""].filter(Boolean).join(" ");
  const label = connectionLabel(edge, model, metricContext);
  const targetKind = edge.targetKind || "thermal_connection";
  const targetID = edge.targetId || baseExpandedID(edge.id);
  const attributes = currentHelpers?.navigationAttributes?.(targetKind, targetID, {}, { tabindex: false }) || "";
  const tooltip = connectionTooltip(edge, model);
  const ariaLabel = connectionAriaLabel(edge, model, targetID, label);
  const markerEnd = `marker-end="url(#${air ? "thermalTopologyAirArrow" : "thermalTopologyArrow"})"`;
  return `<g class="thermal-edge-group navigable-row" tabindex="0" role="button" data-thermal-target-kind="${targetKind}" data-thermal-target-id="${escapeHTML(targetID)}" aria-label="${escapeHTML(ariaLabel)}" ${attributes}>
    <title>${escapeHTML(tooltip)}</title>
    <path class="${classes}" style="--thermal-edge-width:${presentation.width}" d="${edge.route.path}" ${markerEnd}></path>
    <path class="thermal-edge-hit" d="${edge.route.path}" aria-hidden="true"></path>
  </g>`;
}

function renderNode(node, position, selectedID, model, metricContext) {
  if (!position) return "";
  const external = !["zone", "space", "thermal_boundary", "thermal_interface", "window", "thermal_boundary_group"].includes(node.kind);
  const selected = node.id === selectedID || node.entityId === selectedID || node.sourceId === selectedID;
  const targetKind = external ? "thermal_environment" : node.kind;
  const targetID = node.sourceId || node.entityId || node.id;
  const attributes = currentHelpers?.navigationAttributes?.(targetKind, targetID, {}, { tabindex: false }) || "";
  const label = trimLabel(node.label || node.objectName || node.id, 25);
  const subtitle = nodeMetricSubtitle(node, model, metricContext);
  const nodeIssues = (node.diagnosticIds || []).length;
  const ariaLabel = `${label}; entity ${targetKind} ${targetID}; relation node; metric ${subtitle || "not available"}; issues ${nodeIssues}`;
  const classes = ["thermal-node", external ? "external" : cssToken(node.kind), selected ? "selected" : "", model.metric === "qa" && !nodeIssues ? "qa-muted" : ""].filter(Boolean).join(" ");
  return `<g class="${classes} navigable-row" tabindex="0" role="button" transform="translate(${position.x} ${position.y})" data-thermal-target-kind="${escapeHTML(targetKind)}" data-thermal-target-id="${escapeHTML(targetID)}" aria-label="${escapeHTML(ariaLabel)}" ${attributes}>
    <rect x="${-THERMAL_NODE_WIDTH / 2}" y="${-THERMAL_NODE_HEIGHT / 2}" width="${THERMAL_NODE_WIDTH}" height="${THERMAL_NODE_HEIGHT}" rx="9"></rect>
    <circle class="thermal-node-port left" cx="${-THERMAL_NODE_WIDTH / 2}" cy="0" r="3"></circle>
    <circle class="thermal-node-port right" cx="${THERMAL_NODE_WIDTH / 2}" cy="0" r="3"></circle>
    <circle class="thermal-node-port top" cx="0" cy="${-THERMAL_NODE_HEIGHT / 2}" r="3"></circle>
    <circle class="thermal-node-port bottom" cx="0" cy="${THERMAL_NODE_HEIGHT / 2}" r="3"></circle>
    <text class="thermal-node-label" text-anchor="middle" y="-4">${escapeHTML(label)}</text>
    <text class="thermal-node-subtitle" text-anchor="middle" y="14">${escapeHTML(subtitle)}</text>
    ${nodeIssues ? `<text class="thermal-node-issue-badge" x="${THERMAL_NODE_WIDTH / 2 - 8}" y="${-THERMAL_NODE_HEIGHT / 2 + 11}" text-anchor="middle">${nodeIssues}</text>` : ""}
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
    bindThermalHover(element);
  });
  elements.thermalTopologyGraph.querySelector("[data-topology-back]")?.addEventListener("click", collapseBoundaryGraph);
}

function rememberThermalTopologyLayout(cacheKey, layout) {
  const cache = state.thermalTopologyLayoutCache;
  cache.set(cacheKey, layout);
  while (cache.size > THERMAL_LAYOUT_CACHE_LIMIT) {
    cache.delete(cache.keys().next().value);
  }
}

function bindThermalHover(element) {
  element.addEventListener("pointerenter", () => {
    const selection = currentHelpers?.selectionForTarget?.(element.dataset.thermalTargetKind, element.dataset.thermalTargetId);
    if (selection) hoverSemanticEntity(selection, { originView: "geometry", action: "hover", recordHistory: false, follow: false });
  });
  element.addEventListener("pointerleave", () => clearSemanticHover({ originView: "geometry", action: "hover" }));
}

function activateGraphTarget(kind, id) {
  state.thermalTopologySelectedEntityId = id;
  state.thermalTopologySelectedEntityKind = kind;
  state.selectedGeometryKind = kind;
  state.selectedGeometryId = id;
  markGraphTargetSelected(kind, id);
  currentHelpers?.selectGeometry?.(kind, id, { syncLocate: true, syncSemantic: true });
  renderThermalTopologyInspector(currentGeometry, currentHelpers);
}

function markGraphTargetSelected(kind, id) {
  elements.thermalTopologyGraph.querySelectorAll(".thermal-node.selected, .thermal-edge.selected").forEach((element) => element.classList.remove("selected"));
  elements.thermalTopologyGraph.querySelectorAll("[data-thermal-target-id]").forEach((element) => element.setAttribute("aria-selected", "false"));
  const target = [...elements.thermalTopologyGraph.querySelectorAll("[data-thermal-target-id]")]
    .find((element) => element.dataset.thermalTargetKind === kind && element.dataset.thermalTargetId === id);
  target?.classList.add("selected");
  target?.setAttribute("aria-selected", "true");
  target?.querySelector(".thermal-edge")?.classList.add("selected");
}

function expandConnection(id) {
  recordViewHistory();
  compactSelection = { kind: "thermal_connection", id, scope: state.thermalTopologyScope };
  state.thermalTopologyGraphLevel = "boundary";
  state.thermalTopologyScope = "selection";
  state.thermalTopologySelectedEntityId = id;
  state.thermalTopologySelectedEntityKind = "thermal_connection";
  state.selectedGeometryKind = "thermal_connection";
  state.selectedGeometryId = id;
  renderThermalTopology(currentGeometry, currentHelpers);
}

function collapseBoundaryGraph() {
  recordViewHistory();
  state.thermalTopologyGraphLevel = "zone";
  if (compactSelection) {
    state.thermalTopologyScope = compactSelection.scope;
    state.thermalTopologySelectedEntityId = compactSelection.id;
    state.thermalTopologySelectedEntityKind = compactSelection.kind;
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
    metric: state.thermalTopologyMetric,
    areaComponent: state.thermalTopologyAreaComponent,
    layout: state.thermalTopologyLayout,
    scope: state.thermalTopologyScope,
    storyIndex: state.selectedGeometryStory,
    selectedEntityId: state.thermalTopologySelectedEntityId || state.selectedGeometryId,
    selectedEntityKind: state.thermalTopologySelectedEntityKind || state.selectedGeometryKind,
    neighborDepth: state.thermalTopologyNeighborDepth,
    areaBasis: state.thermalTopologyAreaBasis,
    showOpenings: state.thermalTopologyShowOpenings,
    showAirCoupling: state.thermalTopologyShowAirCoupling || state.thermalTopologyMetric === "air",
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
  elements.thermalTopologyAreaComponent.value = normalizeThermalTopologyAreaComponent(state.thermalTopologyAreaComponent);
  elements.thermalTopologyAreaComponentControl.hidden = state.thermalTopologyMetric !== "area";
  elements.thermalTopologyScope.value = normalizeThermalTopologyScope(state.thermalTopologyScope);
  elements.thermalTopologyLayout.value = normalizeThermalTopologyLayout(state.thermalTopologyLayout);
  elements.thermalTopologyAreaBasis.value = normalizeThermalTopologyAreaBasis(state.thermalTopologyAreaBasis);
  elements.thermalTopologyShowOpenings.checked = Boolean(state.thermalTopologyShowOpenings);
  elements.thermalTopologyShowAirCoupling.checked = Boolean(state.thermalTopologyShowAirCoupling);
  elements.thermalTopologyExpandExternalTargets.checked = Boolean(state.thermalTopologyExpandExternalTargets);
}

function connectionLabel(connection, model, metricContext) {
  if (model.graphLevel === "boundary") {
    return String(connection.relationKind || "boundary").replaceAll("_", " ");
  }
  const area = connectionAreaValue(connection, model);
  const gross = Math.max(area, 0.000001);
  const openingField = model.areaBasis === "physical" ? "physicalOpeningArea" : "effectiveOpeningArea";
  const openingRatio = Math.max(0, Number(connection?.[openingField]) || 0) / gross;
  const parts = [];
  if (model.metric === "area") parts.push(formatArea(area));
  else if (model.metric === "ua") parts.push(connectionUAValue(connection, model).available ? `${formatNumber(connectionUAValue(connection, model).value)} W/K` : "N/A");
  else if (model.metric === "qa") parts.push((connection.diagnosticIds || []).length ? `${connection.diagnosticIds.length} issues` : connection.observationKind || "OK");
  else if (model.metric === "air") {
    const couplings = airCouplingsForConnection(connection, model);
    const flow = couplings.reduce((sum, coupling) => sum + (Number(coupling.designFlowRate) || 0), 0);
    parts.push(flow > 0 ? `${formatNumber(flow)} ${couplings[0]?.unit || "m³/s"}` : couplings[0]?.scheduleName || "Air coupling");
  } else {
    parts.push(String(connection.relationKind || "connection").replaceAll("_", " "));
  }
  if (model.metric === "area" && openingRatio > 0) parts.push(`${Math.round(openingRatio * 100)}% open`);
  return parts.join(" · ");
}

function connectionAriaLabel(connection, model, targetID, metricLabel) {
  const relation = String(connection.relationKind || "connection").replaceAll("_", " ");
  const issues = (connection.diagnosticIds || []).length;
  return `${targetID}; entity thermal connection; relation ${relation}; metric ${metricLabel || "not available"}; issues ${issues}`;
}

function nodeMetricSubtitle(node, model) {
  if (node.subtitle) return node.subtitle;
  if (model.metric === "exposure" && (node.kind === "zone" || node.kind === "space")) {
    const signature = model.zoneSignatures.find((item) => item.zoneId === node.id || item.zoneName === node.zoneName || item.zoneName === node.label);
    if (signature) {
      const total = signature.exteriorArea + signature.groundArea + signature.interzoneArea + signature.adiabaticArea + signature.otherBoundaryArea;
      const exterior = total > 0 ? Math.round((signature.exteriorArea / total) * 100) : 0;
      const ground = total > 0 ? Math.round((signature.groundArea / total) * 100) : 0;
      return `${exterior}% exterior · ${ground}% ground`;
    }
  }
  if (node.kind === "zone" || node.kind === "space") {
    return node.storyIndex === undefined ? node.kind : `${node.kind} · story ${Number(node.storyIndex) + 1}`;
  }
  return String(node.kind || "external").replaceAll("_", " ");
}

function createMetricContext(model) {
  const values = model.connections.map((connection) => {
    if (model.metric === "area") return connectionAreaValue(connection, model);
    if (model.metric === "ua") return connectionUAValue(connection, model).value;
    if (model.metric === "air") return airCouplingsForConnection(connection, model).reduce((sum, coupling) => sum + (Number(coupling.designFlowRate) || 0), 0);
    return 1;
  });
  return { maximum: Math.max(...values, 1) };
}

function edgeMetricPresentation(connection, model, context) {
  const classes = [];
  let width = 2;
  if (model.metric === "area") {
    width = 1.75 + 8.25 * Math.sqrt(connectionAreaValue(connection, model) / context.maximum);
    classes.push("metric-area");
  } else if (model.metric === "ua") {
    const ua = connectionUAValue(connection, model);
    width = ua.available ? 1.75 + 8.25 * Math.sqrt(ua.value / context.maximum) : 2.5;
    classes.push("metric-ua", ua.available ? "" : "metric-na");
  } else if (model.metric === "qa") {
    classes.push("metric-qa", (connection.diagnosticIds || []).length || connection.observationKind ? "has-issues" : "qa-muted");
    if (connection.observationKind) classes.push("qa-observation", `observation-${cssToken(connection.observationKind)}`);
    const severity = issueSeverity(connection.diagnosticIds, model.issueLinks);
    if (severity) classes.push(`severity-${severity}`);
  } else if (model.metric === "air") {
    classes.push("metric-air", connection.relationKind === "air_coupling" ? "air-emphasis" : "air-background");
    if (connection.relationKind === "air_coupling") width = 4.5;
  } else if (model.metric === "exposure") {
    classes.push("metric-exposure");
  } else {
    classes.push("metric-connectivity");
  }
  return { width: Math.max(1.5, Number(width) || 1.5).toFixed(2), classes: classes.filter(Boolean) };
}

function renderMetricLegend() {
  const metric = state.thermalTopologyMetric;
  return `<div class="thermal-topology-legend" data-topology-metric="${escapeHTML(metric)}" role="note" aria-label="Network connection types">
    <span class="thermal-legend-item"><i class="thermal-legend-line conductive" aria-hidden="true"></i>Conductive / exterior</span>
    <span class="thermal-legend-item"><i class="thermal-legend-line ground" aria-hidden="true"></i>Ground</span>
    <span class="thermal-legend-item"><i class="thermal-legend-line adiabatic" aria-hidden="true"></i>Adiabatic</span>
    <span class="thermal-legend-item"><i class="thermal-legend-line air" aria-hidden="true"></i>Air / issue pattern</span>
  </div>`;
}

function connectionAreaValue(connection, model) {
  const basis = model.areaBasis === "physical" ? "physical" : "effective";
  const suffix = model.areaComponent === "opaque" ? "OpaqueArea" : model.areaComponent === "openings" ? "OpeningArea" : "GrossArea";
  return Math.max(0, Number(connection?.[`${basis}${suffix}`]) || 0);
}

function connectionUAValue(connection, model) {
  if (model.areaBasis === "physical") {
    return { value: Math.max(0, Number(connection.physicalTotalUa) || 0), available: Boolean(connection.hasPhysicalUa) };
  }
  return { value: Math.max(0, Number(connection.totalUa) || 0), available: Boolean(connection.hasUa) };
}

function airCouplingsForConnection(connection, model) {
  const ids = new Set(connection.airCouplingIds || []);
  return model.airCouplings.filter((coupling) => ids.has(coupling.id));
}

function connectionTooltip(connection, model) {
  const area = model.areaBasis === "physical" ? connection.physicalGrossArea : connection.effectiveGrossArea;
  const ua = connectionUAValue(connection, model);
  const values = [
    String(connection.relationKind || "connection").replaceAll("_", " "),
    `${formatArea(area)}`,
    ua.available ? `${formatNumber(ua.value)} W/K` : "UA N/A",
  ];
  return values.join(" · ");
}

function issueSeverity(diagnosticIDs = [], issueLinks = []) {
  const ids = new Set(diagnosticIDs);
  const severities = issueLinks.filter((issue) => ids.has(issue.id)).map((issue) => String(issue.severity || "").toLowerCase());
  if (severities.includes("error")) return "error";
  if (severities.includes("warning") || severities.includes("warn")) return "warning";
  return severities.length ? "info" : "";
}

function cssToken(value) {
  return String(value || "unknown").toLowerCase().replace(/[^a-z0-9_-]+/g, "-");
}

function baseExpandedID(id) {
  return String(id || "").replace(/:(north|south|east|west|roof|floor)$/i, "");
}

function edgeMatchesThermalSelection(edge, model) {
  const selectedID = state.thermalTopologySelectedEntityId || state.selectedGeometryId;
  const selectedKind = state.thermalTopologySelectedEntityKind || state.selectedGeometryKind;
  if (edge.id === selectedID || edge.targetId === selectedID) return true;
  if (selectedKind === "thermal_boundary" && (edge.boundaryIds || []).includes(selectedID)) return true;
  if (selectedKind === "window") {
    const opening = (currentGeometry?.topology?.openings || []).find((item) => item.entityId === selectedID || item.windowId === selectedID || item.id === selectedID);
    return Boolean(opening && (edge.openingIds || []).includes(opening.id));
  }
  return model.graphLevel === "boundary" && edge.targetId === selectedID;
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
