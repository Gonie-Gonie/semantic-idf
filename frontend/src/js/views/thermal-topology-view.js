import {
  elements,
  escapeHTML,
  normalizeThermalTopologyAreaComponent,
  normalizeThermalTopologyAreaBasis,
  normalizeThermalTopologyDisplay,
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
  if (state.thermalTopologyDisplay === "matrix") {
    elements.thermalTopologyGraph.hidden = true;
    elements.thermalTopologyMatrix.hidden = false;
    renderThermalTopologyMatrix(currentModel);
  } else {
    elements.thermalTopologyGraph.hidden = false;
    elements.thermalTopologyMatrix.hidden = true;
    renderThermalTopologySVG(currentModel, currentLayout);
  }
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
  const edges = [...layout.edges].sort((left, right) => Number(edgeMatchesThermalSelection(left, model)) - Number(edgeMatchesThermalSelection(right, model)));
  const nodes = [...model.nodes].sort((left, right) => String(left.id).localeCompare(String(right.id)));
  const metricContext = createMetricContext(model);
  const backButton = state.thermalTopologyGraphLevel === "boundary"
    ? `<button class="thermal-topology-back" type="button" data-topology-back>${escapeHTML(t("action.back", {}, "Back"))}</button>`
    : "";
  elements.thermalTopologyGraph.innerHTML = `${backButton}${renderMetricLegend(metricContext)}
    <svg class="thermal-topology-svg" viewBox="0 0 ${layout.width} ${layout.height}" preserveAspectRatio="xMidYMid meet" tabindex="0" aria-label="${escapeHTML(t("topology.thermalTooltip"))}">
      <defs>
        <marker id="thermalTopologyArrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
          <path d="M 0 0 L 10 5 L 0 10 z"></path>
        </marker>
        <marker id="thermalTopologyAirArrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
          <path d="M 0 0 L 10 5 L 0 10 z"></path>
        </marker>
        <marker id="thermalTopologyHeatArrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
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
  const attributes = currentHelpers?.navigationAttributes?.(targetKind, targetID) || "";
  const tooltip = connectionTooltip(edge, model);
  const markerStart = presentation.markerStart ? `marker-start="url(#thermalTopologyHeatArrow)"` : "";
  const markerEnd = presentation.markerEnd ? `marker-end="url(#thermalTopologyHeatArrow)"` : model.metric === "simulated_heat" ? "" : `marker-end="url(#${air ? "thermalTopologyAirArrow" : "thermalTopologyArrow"})"`;
  return `<g class="thermal-edge-group navigable-row" data-thermal-target-kind="${targetKind}" data-thermal-target-id="${escapeHTML(targetID)}" ${attributes}>
    <title>${escapeHTML(tooltip)}</title>
    <path class="${classes}" style="--thermal-edge-width:${presentation.width}" d="${edge.route.path}" ${markerStart} ${markerEnd}></path>
    <path class="thermal-edge-hit" d="${edge.route.path}" tabindex="0" role="button" aria-label="${escapeHTML(label)}"></path>
    ${state.thermalTopologyShowLabels ? `<text class="thermal-edge-label${selected ? " selected" : ""}" x="${edge.route.labelX}" y="${edge.route.labelY}">${escapeHTML(label)}</text>` : ""}
  </g>`;
}

function renderNode(node, position, selectedID, model, metricContext) {
  if (!position) return "";
  const external = !["zone", "space", "thermal_boundary", "thermal_interface", "window", "thermal_boundary_group"].includes(node.kind);
  const selected = node.id === selectedID || node.entityId === selectedID || node.sourceId === selectedID;
  const targetKind = external ? "thermal_environment" : node.kind;
  const targetID = node.sourceId || node.entityId || node.id;
  const attributes = currentHelpers?.navigationAttributes?.(targetKind, targetID) || "";
  const label = trimLabel(node.label || node.objectName || node.id, 25);
  const subtitle = nodeMetricSubtitle(node, model, metricContext);
  const nodeIssues = (node.diagnosticIds || []).length;
  const classes = ["thermal-node", external ? "external" : cssToken(node.kind), selected ? "selected" : "", model.metric === "qa" && !nodeIssues ? "qa-muted" : ""].filter(Boolean).join(" ");
  return `<g class="${classes} navigable-row" transform="translate(${position.x} ${position.y})" data-thermal-target-kind="${escapeHTML(targetKind)}" data-thermal-target-id="${escapeHTML(targetID)}" ${attributes}>
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

function renderThermalTopologyMatrix(model) {
  const query = String(state.thermalTopologyMatrixQuery || "").trim().toLowerCase();
  const matchingNodes = [...model.nodes]
    .filter((node) => !query || `${node.label || ""} ${node.id || ""} ${node.kind || ""}`.toLowerCase().includes(query))
    .sort((left, right) => String(left.label || left.id).localeCompare(String(right.label || right.id)));
  const nodes = matchingNodes.slice(0, 120);
  const rowNodes = nodes.filter((node) => node.kind === "zone" || node.kind === "space");
  const connectionByPair = new Map();
  const metricContext = createMetricContext(model);
  for (const connection of model.connections) {
    connectionByPair.set(`${connection.fromNodeId}|${connection.toNodeId}`, connection);
    connectionByPair.set(`${connection.toNodeId}|${connection.fromNodeId}`, connection);
  }
  if (!rowNodes.length || !nodes.length) {
    elements.thermalTopologyMatrix.innerHTML = `<div class="empty">${escapeHTML(t("topology.noConnections"))}</div>`;
    return;
  }
  const limitNote = matchingNodes.length > nodes.length ? `<div class="thermal-matrix-limit">Showing 120 of ${matchingNodes.length} nodes · filter to narrow</div>` : "";
  elements.thermalTopologyMatrix.innerHTML = `${limitNote}<table class="thermal-matrix-table">
    <thead><tr><th class="thermal-matrix-corner">${escapeHTML(t("topology.matrixTab", {}, "Matrix"))}</th>${nodes.map((node) => matrixHeader(node, "column")).join("")}</tr></thead>
    <tbody>${rowNodes.map((row) => `<tr>${matrixHeader(row, "row")}${nodes.map((column) => matrixCell(row, column, connectionByPair.get(`${row.id}|${column.id}`), model, metricContext)).join("")}</tr>`).join("")}</tbody>
  </table>`;
  elements.thermalTopologyMatrix.querySelectorAll("[data-thermal-target-id]").forEach((element) => {
    element.addEventListener("click", () => activateGraphTarget(element.dataset.thermalTargetKind, element.dataset.thermalTargetId));
    element.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" && event.key !== " ") return;
      event.preventDefault();
      activateGraphTarget(element.dataset.thermalTargetKind, element.dataset.thermalTargetId);
    });
    bindThermalHover(element);
  });
}

function matrixHeader(node, axis) {
  const external = node.kind !== "zone" && node.kind !== "space";
  const kind = external ? "thermal_environment" : node.kind;
  const id = node.sourceId || node.entityId || node.id;
  const attributes = currentHelpers?.navigationAttributes?.(kind, id) || "";
  return `<th scope="${axis === "row" ? "row" : "col"}" class="thermal-matrix-header ${axis}" data-thermal-target-kind="${escapeHTML(kind)}" data-thermal-target-id="${escapeHTML(id)}" ${attributes}>${escapeHTML(trimLabel(node.label || node.id, 18))}</th>`;
}

function matrixCell(row, column, connection, model, metricContext) {
  if (!connection) return `<td class="thermal-matrix-cell empty-cell" aria-label="No connection">—</td>`;
  const label = connectionLabel(connection, model, metricContext);
  const attributes = currentHelpers?.navigationAttributes?.("thermal_connection", connection.id) || "";
  return `<td class="thermal-matrix-cell navigable-row" data-thermal-target-kind="thermal_connection" data-thermal-target-id="${escapeHTML(connection.id)}" ${attributes} title="${escapeHTML(connectionTooltip(connection, model))}">${escapeHTML(matrixCellValue(connection, model, label))}</td>`;
}

function matrixCellValue(connection, model, fallback) {
  if (model.metric === "topology") return String(connection.surfaceCount || (connection.boundaryIds || []).length || "•");
  if (model.metric === "area") return formatNumber(connectionAreaValue(connection, model));
  if (model.metric === "ua") {
    const ua = connectionUAValue(connection, model);
    return ua.available ? formatNumber(ua.value) : "N/A";
  }
  if (model.metric === "qa") return String((connection.diagnosticIds || []).length || (connection.observationKind ? "!" : "—"));
  if (model.metric === "air") return airCouplingsForConnection(connection, model).length ? fallback : "—";
  if (model.metric === "simulated_heat") {
    const flow = thermalTopologyFlowForConnection(connection);
    return flow ? signedThermalTopologyValue(thermalTopologySelectedFlowValue(flow)) : "N/A";
  }
  return fallback;
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
  elements.thermalTopologyGraph.querySelectorAll(".thermal-node.selected, .thermal-edge.selected, .thermal-edge-label.selected").forEach((element) => element.classList.remove("selected"));
  const target = [...elements.thermalTopologyGraph.querySelectorAll("[data-thermal-target-id]")]
    .find((element) => element.dataset.thermalTargetKind === kind && element.dataset.thermalTargetId === id);
  target?.classList.add("selected");
  target?.querySelector(".thermal-edge")?.classList.add("selected");
  target?.querySelector(".thermal-edge-label")?.classList.add("selected");
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
  elements.thermalTopologyShowLabels.checked = Boolean(state.thermalTopologyShowLabels);
  elements.thermalTopologyDisplayButtons.forEach((button) => button.classList.toggle("active", button.dataset.thermalTopologyDisplay === normalizeThermalTopologyDisplay(state.thermalTopologyDisplay)));
  elements.thermalTopologyMatrixQuery.value = state.thermalTopologyMatrixQuery || "";
  elements.thermalTopologyMatrixQuery.hidden = state.thermalTopologyDisplay !== "matrix";
  const overlay = thermalTopologySimulationResult();
  const simulatedOption = [...elements.thermalTopologyMetric.options].find((option) => option.value === "simulated_heat");
  if (simulatedOption) {
    simulatedOption.disabled = !overlay.available;
    simulatedOption.title = overlay.available ? "Simulation surface heat-flow overlay" : overlay.unavailableReason || "Run Zone Heat Flow with Surface detail.";
  }
  if (elements.thermalTopologySimulationControls) {
    elements.thermalTopologySimulationControls.hidden = state.thermalTopologyMetric !== "simulated_heat";
  }
  const selection = thermalTopologySimulationSelection();
  if (elements.thermalTopologySimulationPeriod) {
    elements.thermalTopologySimulationPeriod.value = selection.requestedPeriod;
    elements.thermalTopologySimulationPeriod.disabled = !overlay.available;
  }
  if (elements.thermalTopologySimulationFrameControl) {
    elements.thermalTopologySimulationFrameControl.hidden = !overlay.available || selection.maximumFrame <= 0;
  }
  if (elements.thermalTopologySimulationFrame) {
    elements.thermalTopologySimulationFrame.max = String(selection.maximumFrame);
    elements.thermalTopologySimulationFrame.value = String(selection.frame);
    elements.thermalTopologySimulationFrame.disabled = !overlay.available;
  }
  if (elements.thermalTopologySimulationFrameLabel) {
    elements.thermalTopologySimulationFrameLabel.textContent = selection.frameLabel;
  }
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
  if (model.metric === "simulated_heat") {
    const flow = thermalTopologyFlowForConnection(connection);
    const value = thermalTopologySelectedFlowValue(flow);
    parts.push(flow ? `${signedThermalTopologyValue(value)} kWh` : "No result");
    if (flow) parts.push(thermalTopologyFlowDirectionLabel(value));
  } else if (model.metric === "area") parts.push(formatArea(area));
  else if (model.metric === "ua") parts.push(connectionUAValue(connection, model).available ? `${formatNumber(connectionUAValue(connection, model).value)} W/K` : "N/A");
  else if (model.metric === "qa") parts.push((connection.diagnosticIds || []).length ? `${connection.diagnosticIds.length} issues` : connection.observationKind || "OK");
  else if (model.metric === "air") {
    const couplings = airCouplingsForConnection(connection, model);
    const flow = couplings.reduce((sum, coupling) => sum + (Number(coupling.designFlowRate) || 0), 0);
    parts.push(flow > 0 ? `${formatNumber(flow)} ${couplings[0]?.unit || "m³/s"}` : couplings[0]?.scheduleName || "Air coupling");
  } else {
    parts.push(`${Number(connection.surfaceCount) || (connection.boundaryIds || []).length} surfaces`);
    parts.push(String(connection.relationKind || "connection").replaceAll("_", " "));
  }
  if (model.metric === "area" && openingRatio > 0) parts.push(`${Math.round(openingRatio * 100)}% open`);
  return parts.join(" · ");
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
    if (model.metric === "simulated_heat") return Math.abs(thermalTopologySelectedFlowValue(thermalTopologyFlowForConnection(connection)));
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
  } else if (model.metric === "simulated_heat") {
    const flow = thermalTopologyFlowForConnection(connection);
    const value = thermalTopologySelectedFlowValue(flow);
    width = flow ? 1.75 + 8.25 * Math.sqrt(Math.abs(value) / context.maximum) : 1.5;
    classes.push("metric-simulated-heat", flow ? (value >= 0 ? "metric-gain" : "metric-loss") : "metric-na");
    const marker = thermalTopologyFlowMarker(connection, flow, value);
    return { width: Math.max(1.5, Number(width) || 1.5).toFixed(2), classes: classes.filter(Boolean), ...marker };
  } else {
    classes.push("metric-connectivity");
  }
  return { width: Math.max(1.5, Number(width) || 1.5).toFixed(2), classes: classes.filter(Boolean) };
}

function renderMetricLegend(context) {
  const metric = state.thermalTopologyMetric;
  const topology = currentGeometry?.topology || {};
  let text = "Static topology · relation colors · hover for area and UA";
  if (metric === "area") text = `Static topology · ${state.thermalTopologyAreaComponent} area · ${state.thermalTopologyAreaBasis === "physical" ? "Physical" : "Model total"} · m²`;
  else if (metric === "ua") {
    const valid = (currentModel?.connections || []).filter((connection) => connectionUAValue(connection, currentModel).available).length;
    const total = currentModel?.connections?.length || 0;
    text = `Static topology · Total UA · W/K · coverage ${total ? Math.round((valid / total) * 100) : 0}% · hatch = N/A`;
  } else if (metric === "exposure") text = "Static topology · Exterior · Ground · Adjacent zone · Adiabatic";
  else if (metric === "qa") text = `Static topology · ${topology.issueLinks?.length || 0} issues · solid = declared mismatch · dotted = observation`;
  else if (metric === "air") text = "Static topology · air coupling emphasized · conductive boundaries muted";
  else if (metric === "simulated_heat") {
    const overlay = thermalTopologySimulationResult();
    const selection = thermalTopologySimulationSelection();
    text = overlay.available
      ? `Simulation overlay · ${selection.displayLabel} · red = gain · blue = loss · signed kWh · positive enters owner`
      : `Static topology · ${overlay.unavailableReason || "surface heat-flow results not loaded"}`;
  }
  return `<div class="thermal-topology-legend" data-topology-metric="${escapeHTML(metric)}">${escapeHTML(text)}${context.maximum > 1 && ["area", "ua"].includes(metric) ? ` · max ${escapeHTML(formatNumber(context.maximum))}` : ""}</div>`;
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
  if (model.metric === "simulated_heat") {
    const flow = thermalTopologyFlowForConnection(connection);
    const value = thermalTopologySelectedFlowValue(flow);
    values.push(flow ? `${signedThermalTopologyValue(value)} kWh (${thermalTopologyFlowDirectionLabel(value)})` : "No mapped simulation result");
    values.push("Simulation heat flow is not compared directly with static UA");
  }
  return values.join(" · ");
}

function thermalTopologySimulationResult() {
  return state.simulationResult?.purposeResults?.thermalTopology || {
    available: false,
    unavailableReason: "Run Zone Heat Flow with Surface detail to load a simulation overlay.",
    periods: [],
    sources: [],
  };
}

function thermalTopologySimulationSelection() {
  const overlay = thermalTopologySimulationResult();
  const requestedPeriod = ["annual", "monthly", "daily", "hourly", "selected_range"].includes(state.thermalTopologySimulationPeriod)
    ? state.thermalTopologySimulationPeriod
    : "annual";
  const sourcePeriodID = requestedPeriod === "selected_range" ? "hourly" : requestedPeriod;
  const period = (overlay.periods || []).find((item) => item.id === sourcePeriodID)
    || (overlay.periods || []).find((item) => item.id === "annual")
    || null;
  const frameCount = Math.max(0, Number(period?.frameCount) || period?.labels?.length || 0);
  const windowSize = requestedPeriod === "selected_range" ? Math.min(24, frameCount) : 1;
  const maximumFrame = Math.max(0, frameCount - windowSize);
  const frame = Math.min(maximumFrame, Math.max(0, Number(state.thermalTopologySimulationFrame) || 0));
  const labels = period?.labels || [];
  const frameLabel = requestedPeriod === "selected_range"
    ? `${labels[frame] || `Frame ${frame + 1}`} – ${labels[Math.min(frameCount - 1, frame + windowSize - 1)] || `Frame ${frame + windowSize}`}`
    : labels[frame] || period?.label || "Frame";
  return {
    overlay,
    requestedPeriod,
    sourcePeriodID,
    period,
    frame,
    frameCount,
    windowSize,
    maximumFrame,
    frameLabel,
    displayLabel: requestedPeriod === "selected_range" ? `Selected range ${frameLabel}` : `${period?.label || requestedPeriod} · ${frameLabel}`,
  };
}

function thermalTopologyFlowForConnection(connection) {
  const selection = thermalTopologySimulationSelection();
  if (!selection.period) return null;
  const connectionID = baseExpandedID(connection?.id);
  const direct = (selection.period.connectionFlows || []).find((flow) => flow.connectionId === connectionID);
  if (direct) return direct;
  const boundaryIDs = new Set(connection?.boundaryIds || []);
  if (!boundaryIDs.size) return null;
  const matches = (selection.period.boundaryFlows || []).filter((flow) => (
    boundaryIDs.has(flow.boundaryId) || (flow.relatedBoundaryIds || []).some((id) => boundaryIDs.has(id))
  ));
  if (!matches.length) return null;
  if (matches.length === 1) return matches[0];
  return matches.reduce((combined, flow) => ({
    ...combined,
    value: Number(combined.value || 0) + Number(flow.value || 0),
    values: addThermalTopologyFlowValues(combined.values, flow.values),
    sourceIds: [...new Set([...(combined.sourceIds || []), ...(flow.sourceIds || [])])],
    boundaryIds: [...new Set([...(combined.boundaryIds || []), flow.boundaryId, ...(flow.relatedBoundaryIds || [])])],
    ownerNodeId: combined.ownerNodeId === flow.ownerNodeId ? combined.ownerNodeId : "",
  }), { value: 0, values: [], sourceIds: [], boundaryIds: [], ownerNodeId: matches[0].ownerNodeId });
}

function thermalTopologySelectedFlowValue(flow) {
  if (!flow) return 0;
  const selection = thermalTopologySimulationSelection();
  const values = flow.values || [];
  if (!values.length) return Number(flow.value) || 0;
  if (selection.requestedPeriod === "selected_range") {
    return values.slice(selection.frame, selection.frame + selection.windowSize).reduce((sum, value) => sum + (Number(value) || 0), 0);
  }
  return Number(values[Math.min(values.length - 1, selection.frame)]) || 0;
}

function thermalTopologyFlowMarker(connection, flow, value) {
  if (!flow || value === 0) return {};
  const ownerID = flow.ownerNodeId || flow.ownerNodeID || "";
  const destinationID = value > 0
    ? ownerID
    : ownerID === connection.fromNodeId ? connection.toNodeId : connection.fromNodeId;
  if (destinationID === connection.fromNodeId) return { markerStart: true };
  if (destinationID === connection.toNodeId) return { markerEnd: true };
  return value > 0 ? { markerEnd: true } : { markerStart: true };
}

function addThermalTopologyFlowValues(left = [], right = []) {
  const length = Math.max(left.length, right.length);
  return Array.from({ length }, (_, index) => (Number(left[index]) || 0) + (Number(right[index]) || 0));
}

function signedThermalTopologyValue(value) {
  const number = Number(value) || 0;
  return `${number > 0 ? "+" : ""}${formatNumber(number)}`;
}

function thermalTopologyFlowDirectionLabel(value) {
  if (Number(value) > 0) return "gain to owner";
  if (Number(value) < 0) return "loss from owner";
  return "balanced";
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
