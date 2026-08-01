import { elements, escapeHTML, state } from "../state.js";
import { t } from "../i18n.js";
import { openSelectionInView, revealSelectionSource, selectionTargetsForView } from "../selection-controller.js";
import { resolveThermalTopologyTarget } from "../thermal-topology-targets.js";
import { recordViewHistory } from "../view-history.js";

let activeGeometry = null;
let activeHelpers = null;
let activeSelection = null;

export function renderThermalTopologyInspector(geometry, helpers = {}) {
  if (!elements.thermalTopologyInspector) return;
  activeGeometry = geometry;
  activeHelpers = helpers;
  activeSelection = resolveInspectorSelection(geometry);
  if (!activeSelection?.item) {
    elements.thermalTopologyInspector.removeAttribute("aria-labelledby");
    elements.thermalTopologyInspector.innerHTML = `<div class="empty">${escapeHTML(t("topology.inspectorEmpty", {}, "Select a thermal node or connection"))}</div>`;
    return;
  }
  const { kind, id, item } = activeSelection;
  const attributes = helpers.navigationAttributes?.(kind, id) || "";
  elements.thermalTopologyInspector.setAttribute("aria-labelledby", "thermalTopologyInspectorHeading");
  elements.thermalTopologyInspector.innerHTML = `
    <div class="thermal-inspector-head navigable-row" ${attributes}>
      <div>
        <h3 id="thermalTopologyInspectorHeading">${escapeHTML(selectionTitle(activeSelection))}</h3>
        <span>${escapeHTML(selectionSubtitle(activeSelection))}</span>
      </div>
      <button type="button" class="geometry-mode-button" data-panel-action-menu aria-label="Navigation actions">•••</button>
    </div>
    <div class="thermal-inspector-content">
      ${renderSelectionDetails(activeSelection, geometry)}
      ${renderInspectorActions(activeSelection, geometry)}
    </div>`;
  bindInspectorActions();
}

function resolveInspectorSelection(geometry) {
  const id = state.thermalTopologySelectedEntityId || state.selectedGeometryId;
  const kind = state.thermalTopologySelectedEntityKind || state.selectedGeometryKind;
  if (!id) return null;
  const thermal = resolveThermalTopologyTarget({ targetKind: kind, targetId: id }, geometry);
  if (thermal?.item) return { ...thermal, item: thermal.item };
  const topology = geometry?.topology || {};
  if (kind === "zone" || kind === "space") {
    const item = (topology.nodes || []).find((node) => node.id === id || node.entityId === id);
    return item ? { kind, id: item.entityId || item.id, item } : null;
  }
  if (kind === "window" || kind === "fenestration") {
    const item = (topology.openings || []).find((opening) => opening.id === id || opening.entityId === id || opening.windowId === id);
    return item ? { kind: "window", id: item.entityId || item.windowId, item, opening: item } : null;
  }
  if (kind === "surface") {
    const item = (topology.boundaries || []).find((boundary) => boundary.surfaceId === id || boundary.surfaceEntityId === id);
    return item ? { kind: "thermal_boundary", id: item.id, item } : null;
  }
  return null;
}

function renderSelectionDetails(selection, geometry) {
  let details = "";
  if (selection.kind === "zone" || selection.kind === "space") details = renderZoneDetails(selection.item, geometry);
  else if (selection.kind === "thermal_connection") details = renderConnectionDetails(selection.item, geometry);
  else if (selection.kind === "thermal_boundary") details = renderBoundaryDetails(selection.item, geometry);
  else if (selection.kind === "window") details = renderOpeningDetails(selection.item, geometry);
  else if (selection.kind === "thermal_air_coupling") details = renderAirCouplingDetails(selection.item, geometry);
  else if (selection.kind === "thermal_environment") details = renderEnvironmentDetails(selection.item, geometry);
  else if (selection.kind === "thermal_interface") details = renderInterfaceDetails(selection.item, geometry);
  else if (selection.kind === "thermal_issue") details = renderIssueDetails(selection.item);
  else if (selection.kind === "thermal_observation") details = renderObservationDetails(selection.item, geometry);
  else details = renderRows([["ID", selection.id], ["Kind", selection.kind]]);
  return details + renderSimulationHeatFlowLedger(selection);
}

function renderZoneDetails(node, geometry) {
  const topology = geometry?.topology || {};
  const signature = (topology.zoneSignatures || []).find((item) => item.zoneId === node.id || item.zoneName === node.zoneName || item.zoneName === node.label) || {};
  const adjacent = [...new Set([...(signature.adjacentZoneIds || []), ...(signature.airCoupledZoneIds || [])])];
  return [
    inspectorSection("Geometry", renderRows([
      ["Floor area", area(node.floorArea)],
      ["Volume", volume(node.volume)],
      ["Story", Number(node.storyIndex) + 1],
      ["Shell", signature.closedShell ? "Closed" : `Open · ${signature.openEdgeCount || 0} edges`],
    ])),
    inspectorSection("Thermal exposure", renderRows([
      ["Outdoors", area(signature.exteriorArea)],
      ["Ground", area(signature.groundArea)],
      ["Adjacent-zone", area(signature.interzoneArea)],
      ["Adiabatic", area(signature.adiabaticArea)],
      ["Opening ratio", percent(signature.exteriorWwr)],
      ["Exterior UA", wattsPerKelvin(signature.exteriorUa, signature.hasTotalUa)],
      ["Ground UA", wattsPerKelvin(signature.groundUa, signature.hasTotalUa)],
      ["Total UA", wattsPerKelvin(signature.totalUa, signature.hasTotalUa)],
    ])),
    inspectorSection("Adjacent zones", adjacent.length ? `<div class="thermal-inspector-chips">${adjacent.map(chip).join("")}</div>` : emptyValue()),
    renderZoneProfileSummary(node),
    renderZoneHVACSummary(node),
    renderZoneOutputSummary(node),
    renderDiagnostics(node.diagnosticIds, topology),
  ].join("");
}

function renderConnectionDetails(connection, geometry) {
  const topology = geometry?.topology || {};
  const nodeByID = new Map((topology.nodes || []).map((node) => [node.id, node]));
  const boundaries = (topology.boundaries || []).filter((boundary) => (connection.boundaryIds || []).includes(boundary.id));
  const constructions = dominantValues(boundaries.map((boundary) => boundary.constructionName));
  return [
    inspectorSection("Connection", renderRows([
      ["From", nodeByID.get(connection.fromNodeId)?.label || connection.fromNodeId],
      ["To", nodeByID.get(connection.toNodeId)?.label || connection.toNodeId],
      ["Relation", humanize(connection.relationKind)],
      ["Surfaces / openings", `${connection.surfaceCount || 0} / ${connection.openingCount || 0}`],
    ])),
    inspectorSection("Physical", renderRows([
      ["Gross / opaque / openings", `${area(connection.physicalGrossArea)} / ${area(connection.physicalOpaqueArea)} / ${area(connection.physicalOpeningArea)}`],
      ["Opaque / opening / total UA", `${wattsPerKelvin(connection.physicalOpaqueUa, connection.hasPhysicalUa)} / ${wattsPerKelvin(connection.physicalOpeningUa, connection.hasPhysicalUa)} / ${wattsPerKelvin(connection.physicalTotalUa, connection.hasPhysicalUa)}`],
    ])),
    inspectorSection("Model total", renderRows([
      ["Gross / opaque / openings", `${area(connection.effectiveGrossArea)} / ${area(connection.effectiveOpaqueArea)} / ${area(connection.effectiveOpeningArea)}`],
      ["Opaque / opening / total UA", `${wattsPerKelvin(connection.opaqueUa, connection.hasUa)} / ${wattsPerKelvin(connection.openingUa, connection.hasUa)} / ${wattsPerKelvin(connection.totalUa, connection.hasUa)}`],
      ["Dominant constructions", constructions.join(", ") || "N/A"],
    ])),
    inspectorSection("Source boundaries", boundaries.length ? `<div class="thermal-inspector-source-list">${boundaries.map((boundary) => sourceButton("thermal_boundary", boundary.id, boundary.surfaceName)).join("")}</div>` : emptyValue()),
    renderDiagnostics(connection.diagnosticIds, topology),
  ].join("");
}

function renderBoundaryDetails(boundary, geometry) {
  const topology = geometry?.topology || {};
  const openings = (topology.openings || []).filter((opening) => (boundary.openingIds || []).includes(opening.id));
  const check = boundary.geometryCheck || {};
  return [
    inspectorSection("Source boundary", renderRows([
      ["Object", boundary.surfaceName],
      ["Owner zone / space", [boundary.ownerZoneId, boundary.ownerSpaceId].filter(Boolean).join(" / ")],
      ["OBC", `${boundary.boundaryConditionRaw || boundary.boundaryCondition} → ${boundary.targetName || boundary.targetId}`],
      ["Counterpart", boundary.counterpartSurfaceEntityId || (boundary.virtualCounterpart ? "Virtual" : "N/A")],
      ["Relation", humanize(boundary.relationKind)],
      ["Construction", `${boundary.constructionName || "N/A"} · ${boundary.hasUValue ? `${number(boundary.uValue)} W/m²K` : "U N/A"}`],
      ["Construction validation", humanize(boundary.constructionStatus || "not checked")],
    ])),
    inspectorSection("Area & UA", renderRows([
      ["Physical gross / opening / net", `${area(boundary.physicalGrossArea)} / ${area(boundary.physicalOpeningArea)} / ${area(boundary.physicalOpaqueArea)}`],
      ["Model gross / opening / net", `${area(boundary.effectiveGrossArea)} / ${area(boundary.effectiveOpeningArea)} / ${area(boundary.effectiveOpaqueArea)}`],
      ["Opaque / opening / total UA", `${wattsPerKelvin(boundary.opaqueUa, boundary.hasUa)} / ${wattsPerKelvin(boundary.openingUa, boundary.hasUa)} / ${wattsPerKelvin(boundary.totalUa, boundary.hasUa)}`],
    ])),
    inspectorSection("Exposure & geometry", renderRows([
      ["Orientation / azimuth", `${boundary.orientation || "N/A"} / ${number(boundary.azimuth)}°`],
      ["Sun / wind", `${boundary.sunExposure || "N/A"} / ${boundary.windExposure || "N/A"}`],
      ["Validation", `${check.status || "not checked"}${check.message ? ` · ${check.message}` : ""}`],
      ["Overlap / plane", `${percent(check.overlapRatio)} / ${number(check.planeDistance)} m`],
    ])),
    inspectorSection("Openings", openings.length ? `<div class="thermal-inspector-source-list">${openings.map((opening) => sourceButton("window", opening.entityId || opening.windowId, `${opening.name} · ${area(opening.effectiveArea)}`)).join("")}</div>` : emptyValue()),
    renderDiagnostics(boundary.diagnosticIds, topology),
  ].join("");
}

function renderOpeningDetails(opening, geometry) {
  return [
    inspectorSection("Opening", renderRows([
      ["Base surface", opening.baseSurfaceId],
      ["Counterpart", opening.counterpartOpeningId || "N/A"],
      ["Construction", opening.constructionName || "N/A"],
      ["U-value", opening.hasUValue ? `${number(opening.uValue)} W/m²K` : "N/A"],
      ["Physical / model area", `${area(opening.physicalArea)} / ${area(opening.effectiveArea)}`],
      ["UA", wattsPerKelvin(opening.ua, opening.hasUa)],
    ])),
    renderDiagnostics(opening.diagnosticIds, geometry?.topology || {}),
  ].join("");
}

function renderAirCouplingDetails(coupling, geometry) {
  const nodes = new Map((geometry?.topology?.nodes || []).map((node) => [node.id, node]));
  return [
    inspectorSection("Air coupling", renderRows([
      ["From / to", `${nodes.get(coupling.fromNodeId)?.label || coupling.fromNodeId} → ${nodes.get(coupling.toNodeId)?.label || coupling.toNodeId}`],
      ["Object", `${coupling.objectType} · ${coupling.objectName || ""}`],
      ["Kind / direction", `${humanize(coupling.couplingKind)} / ${coupling.direction || "N/A"}`],
      ["Design flow", coupling.designFlowRate ? `${number(coupling.designFlowRate)} ${coupling.unit || "m³/s"}` : "N/A"],
      ["Schedule", coupling.scheduleName || "N/A"],
      ["AFN surface / component", `${coupling.surfaceId || "N/A"} / ${coupling.componentName || "N/A"}`],
    ])),
    renderDiagnostics(coupling.diagnosticIds, geometry?.topology || {}),
  ].join("");
}

function renderObservationDetails(observation, geometry) {
  const topology = geometry?.topology || {};
  const boundaries = (topology.boundaries || []).filter((boundary) => (observation.boundaryIds || []).includes(boundary.id));
  return [
    inspectorSection("Geometric adjacency QA", renderRows([
      ["Observation", humanize(observation.observationKind)],
      ["Overlap", percent(observation.overlapRatio)],
      ["Declared connection", observation.declaredConnection ? "Yes" : "No"],
      ["Thermal relation", "Not created · QA evidence only"],
    ])),
    inspectorSection("Adjacent source surfaces", boundaries.length
      ? `<div class="thermal-inspector-source-list">${boundaries.map((boundary) => sourceButton("thermal_boundary", boundary.id, boundary.surfaceName)).join("")}</div>`
      : emptyValue()),
  ].join("");
}

function renderEnvironmentDetails(node, geometry) {
  const topology = geometry?.topology || {};
  const connections = (topology.connections || []).filter((connection) => connection.fromNodeId === node.id || connection.toNodeId === node.id);
  const families = dominantValues(connections.map((connection) => humanize(connection.relationKind)));
  return inspectorSection("External target", renderRows([
    ["Connected zones", new Set(connections.flatMap((connection) => [connection.fromNodeId, connection.toNodeId]).filter((id) => id !== node.id)).size],
    ["Model total area", area(connections.reduce((sum, connection) => sum + (Number(connection.effectiveGrossArea) || 0), 0))],
    ["Total UA", wattsPerKelvin(connections.reduce((sum, connection) => sum + (Number(connection.totalUa) || 0), 0), connections.some((connection) => connection.hasUa))],
    ["Boundary families", families.join(", ") || "N/A"],
  ]));
}

function renderInterfaceDetails(item) {
  return inspectorSection("Reciprocal interface", renderRows([
    ["ID", item.id],
    ["Boundaries", (item.boundaries || []).map((boundary) => boundary.surfaceName).join(" ↔ ")],
  ]));
}

function renderIssueDetails(issue) {
  return inspectorSection("Topology issue", renderRows([
    ["Severity", issue.severity],
    ["Code", issue.code],
    ["Message", issue.message],
  ]));
}

function renderDiagnostics(ids = [], topology = {}) {
  const idSet = new Set(ids || []);
  const issues = (topology.issueLinks || []).filter((issue) => idSet.has(issue.id));
  return inspectorSection("Diagnostics", issues.length
    ? `<div class="thermal-inspector-source-list">${issues.map((issue) => `<button type="button" data-inspector-diagnostic="${escapeHTML(issue.id)}"><strong>${escapeHTML(issue.severity || "warning")}</strong><span>${escapeHTML(issue.code)} · ${escapeHTML(issue.message)}</span></button>`).join("")}</div>`
    : emptyValue("No linked issues"));
}

function renderZoneProfileSummary(node) {
  const zoneName = node.zoneName || node.label || node.objectName || "";
  const profile = (state.report?.profile?.zoneProfiles || []).find((item) => sameThermalTopologyName(item.zoneName, zoneName));
  const wanted = new Set(["occupancy", "lighting", "equipment", "infiltration", "ventilation"]);
  const dimensions = (profile?.dimensions || []).filter((item) => wanted.has(item.dimension));
  return inspectorSection("Profile summary", dimensions.length
    ? renderRows(dimensions.map((item) => [item.label || humanize(item.dimension), item.displayValue || `${number(item.value)} ${item.unit || ""}`.trim()]))
    : emptyValue("No linked occupancy, lighting, equipment, infiltration, or ventilation profile"));
}

function renderZoneHVACSummary(node) {
  const zoneName = node.zoneName || node.label || node.objectName || "";
  const summaries = (state.report?.hvac?.serviceModel?.zoneServices || []).filter((item) => sameThermalTopologyName(item.zoneName, zoneName));
  const paths = summaries.flatMap((item) => item.paths || []);
  const services = dominantValues(paths.map((path) => humanize(path.serviceKind)));
  const systems = dominantValues(paths.flatMap((path) => [path.airLoop?.name, path.plantLoop?.name, path.sourceSystem?.name]).filter(Boolean));
  return inspectorSection("HVAC service", paths.length
    ? renderRows([
      ["Service paths", paths.length],
      ["Services", services.join(", ") || "N/A"],
      ["Systems / loops", systems.join(", ") || "N/A"],
    ])
    : emptyValue("No HVAC service path targets this zone"));
}

function renderZoneOutputSummary(node) {
  const zoneName = node.zoneName || node.label || node.objectName || "";
  const requests = (state.report?.output?.existing || []).filter((item) => (
    item.objectType === "Output:Variable" && (item.keyValue === "*" || sameThermalTopologyName(item.keyValue, zoneName))
  ));
  return inspectorSection("Output requests", requests.length
    ? renderRows([
      ["Applicable requests", requests.length],
      ["Examples", requests.slice(0, 3).map((item) => item.variableName).filter(Boolean).join(", ")],
    ])
    : emptyValue("No applicable Output:Variable request"));
}

function renderSimulationHeatFlowLedger(selection) {
  const overlay = state.simulationResult?.purposeResults?.thermalTopology;
  if (!overlay?.available) {
    const reason = overlay?.unavailableReason || "Simulation overlay is not loaded.";
    return inspectorSection("Heat-flow ledger", `<div class="empty compact">${escapeHTML(reason)}</div><button type="button" data-inspector-purpose-plan>Open purpose plan</button>`);
  }
  const periodSelection = thermalTopologyInspectorPeriod(overlay);
  const flows = thermalTopologyInspectorFlows(selection, periodSelection.period);
  if (!flows.length) {
    return inspectorSection("Heat-flow ledger", `<div class="empty compact">No mapped surface result for this topology entity.</div><button type="button" data-inspector-purpose-plan>Open purpose plan</button>`);
  }
  const sourceIDs = [...new Set(flows.flatMap((flow) => [
    ...(flow.sourceIds || []),
    ...(flow.traces || []).flatMap((trace) => trace.sourceIds || []),
  ]))];
  const sources = sourceIDs.map((id) => (overlay.sources || []).find((source) => source.id === id)).filter(Boolean);
  const value = flows.reduce((sum, flow) => sum + thermalTopologyInspectorFlowValue(flow, periodSelection), 0);
  return inspectorSection("Heat-flow ledger", [
    renderRows([
      ["Period", periodSelection.label],
      ["Signed energy", `${value > 0 ? "+" : ""}${number(value)} kWh`],
      ["Direction", value > 0 ? "Gain to owner" : value < 0 ? "Loss from owner" : "Balanced"],
      ["Convention", overlay.signConvention || "Positive enters the owning zone"],
      ["Sources", sources.length],
    ]),
    sources.length
      ? `<div class="thermal-inspector-source-list">${sources.map((source) => `<button type="button" data-inspector-output-source="${escapeHTML(source.id)}"><strong>${escapeHTML(source.name || source.id)}</strong><span>${escapeHTML(source.keyValue || "*")} · ${escapeHTML(source.sourceUnit || source.units || "")} → ${escapeHTML(source.normalizedUnit || "kWh")} · ${escapeHTML(source.aggregationMethod || "reported")}</span></button>`).join("")}</div>`
      : emptyValue("No source provenance"),
  ].join(""));
}

function thermalTopologyInspectorPeriod(overlay) {
  const requested = state.thermalTopologySimulationPeriod || "annual";
  const sourceID = requested === "selected_range" ? "hourly" : requested;
  const period = (overlay.periods || []).find((item) => item.id === sourceID) || (overlay.periods || [])[0] || null;
  const frameCount = Math.max(0, Number(period?.frameCount) || period?.labels?.length || 0);
  const windowSize = requested === "selected_range" ? Math.min(24, frameCount) : 1;
  const frame = Math.min(Math.max(0, frameCount - windowSize), Math.max(0, Number(state.thermalTopologySimulationFrame) || 0));
  const start = period?.labels?.[frame] || period?.label || requested;
  const end = period?.labels?.[Math.min(frameCount - 1, frame + windowSize - 1)] || start;
  return { requested, period, frame, windowSize, label: requested === "selected_range" ? `${start} – ${end}` : `${period?.label || requested} · ${start}` };
}

function thermalTopologyInspectorFlows(selection, period) {
  if (!period) return [];
  const topology = activeGeometry?.topology || {};
  if (selection.kind === "thermal_connection") {
    return (period.connectionFlows || []).filter((flow) => flow.connectionId === selection.id || flow.connectionId === selection.item?.id);
  }
  if (selection.kind === "thermal_boundary") {
    return (period.boundaryFlows || []).filter((flow) => flow.boundaryId === selection.id || (flow.relatedBoundaryIds || []).includes(selection.id));
  }
  if (selection.kind === "window") {
    const opening = selection.opening || selection.item;
    const boundary = (topology.boundaries || []).find((item) => item.surfaceId === opening.baseSurfaceId || item.id === opening.baseSurfaceId);
    return boundary ? (period.boundaryFlows || []).filter((flow) => flow.boundaryId === boundary.id || (flow.relatedBoundaryIds || []).includes(boundary.id)) : [];
  }
  if (selection.kind === "zone" || selection.kind === "space" || selection.kind === "thermal_environment") {
    const nodeID = selection.item?.id || selection.id;
    return (period.boundaryFlows || []).filter((flow) => flow.ownerNodeId === nodeID || flow.targetNodeId === nodeID);
  }
  return [];
}

function thermalTopologyInspectorFlowValue(flow, selection) {
  const values = flow?.values || [];
  if (!values.length) return Number(flow?.value) || 0;
  if (selection.requested === "selected_range") {
    return values.slice(selection.frame, selection.frame + selection.windowSize).reduce((sum, value) => sum + (Number(value) || 0), 0);
  }
  return Number(values[Math.min(values.length - 1, selection.frame)]) || 0;
}

function renderInspectorViewAction(view, label, selection) {
  const semanticSelection = activeHelpers?.selectionForTarget?.(selection.kind, selection.id);
  const available = selectionTargetsForView(view, semanticSelection).length > 0;
  const reason = available ? `Open linked ${label}` : `No linked ${label} target for this zone`;
  return `<button type="button" data-inspector-view="${escapeHTML(view)}" ${available ? "" : "disabled"} title="${escapeHTML(reason)}">${escapeHTML(label)}</button>`;
}

function openThermalTopologyOutputSource(sourceID) {
  const overlay = state.simulationResult?.purposeResults?.thermalTopology || {};
  const source = (overlay.sources || []).find((item) => item.id === sourceID);
  if (!source) return;
  const planned = (state.simulationResult?.purposeRunPlan?.outputObjects || []).find((item) => (
    item.objectType === "Output:Variable" &&
    sameThermalTopologyName(item.keyValue, source.keyValue) &&
    sameThermalTopologyName(item.variableName, source.name)
  ));
  const existing = (state.report?.output?.existing || []).find((item) => (
    (planned?.signature && item.signature === planned.signature) ||
    (item.objectType === "Output:Variable" && sameThermalTopologyName(item.keyValue, source.keyValue) && sameThermalTopologyName(item.variableName, source.name))
  ));
  if (existing) {
    state.outputFocusedSignature = existing.signature || "";
    state.outputTemporaryRevealSignature = "";
    [...(elements.resultTabButtons || [])].find((button) => button.dataset.resultTab === "output")?.click();
    return;
  }
  window.dispatchEvent(new CustomEvent("idfAnalyzer:openSimulationPurposePlan", { detail: { sourceID, signature: planned?.signature || "" } }));
}

function sameThermalTopologyName(left, right) {
  return String(left || "").trim().toLowerCase() === String(right || "").trim().toLowerCase();
}

function renderInspectorActions(selection, geometry) {
  const reveal = geometryRevealTarget(selection, geometry);
  const issueIDs = selectionDiagnosticIDs(selection);
  const constructionIndex = selection.item?.constructionObjectIndex;
  return `<section class="thermal-inspector-actions">
    <h4>Actions</h4>
    <div>
      <button type="button" data-inspector-mode="3d" ${reveal ? "" : "disabled"}>${escapeHTML(t("topology.showIn3D", {}, "Show in 3D"))}</button>
      <button type="button" data-inspector-mode="plan" ${reveal ? "" : "disabled"}>${escapeHTML(t("topology.showInPlan", {}, "Show in Plan"))}</button>
      <button type="button" data-inspector-mode="thermal">Thermal</button>
      <button type="button" data-inspector-semantic>${escapeHTML(t("topology.revealSemantic", {}, "Reveal in Semantic"))}</button>
      <button type="button" data-inspector-source ${sourceAnchor(selection) ? "" : "disabled"}>${escapeHTML(t("topology.revealSource", {}, "Reveal source"))}</button>
      ${Number.isInteger(constructionIndex) ? `<button type="button" data-inspector-construction="${constructionIndex}">Open Construction</button>` : ""}
      ${issueIDs.length ? `<button type="button" data-inspector-diagnostic="${escapeHTML(issueIDs[0])}">Open Diagnose issue</button>` : ""}
      ${(selection.kind === "zone" || selection.kind === "space") ? [
        renderInspectorViewAction("profile", "Profile", selection),
        renderInspectorViewAction("hvac", "HVAC", selection),
        renderInspectorViewAction("output", "Output", selection),
      ].join("") : ""}
    </div>
  </section>`;
}

function bindInspectorActions() {
  elements.thermalTopologyInspector.querySelectorAll("[data-thermal-inspector-kind]").forEach((button) => {
    button.addEventListener("click", () => activeHelpers.selectGeometry?.(button.dataset.thermalInspectorKind, button.dataset.thermalInspectorId));
  });
  elements.thermalTopologyInspector.querySelectorAll("[data-inspector-mode]").forEach((button) => {
    button.addEventListener("click", async () => {
      const mode = button.dataset.inspectorMode;
      const reveal = geometryRevealTarget(activeSelection, activeGeometry);
      recordViewHistory();
      if (mode !== "thermal" && activeSelection.kind.startsWith("thermal_") && activeHelpers.revealThermalTargetInGeometry?.(activeSelection.kind, activeSelection.id, mode)) {
        return;
      }
      activeHelpers.setGeometryMode?.(mode);
      if (reveal && mode !== "thermal") await activeHelpers.selectGeometry?.(reveal.kind, reveal.id, { syncLocate: false, recordHistory: false });
    });
  });
  elements.thermalTopologyInspector.querySelector("[data-inspector-semantic]")?.addEventListener("click", () => openSelectionInView("input-semantic", { originView: "geometry", action: "open" }));
  elements.thermalTopologyInspector.querySelector("[data-inspector-source]")?.addEventListener("click", () => revealSelectionSource({ originView: "geometry", action: "reveal_source" }));
  elements.thermalTopologyInspector.querySelector("[data-inspector-construction]")?.addEventListener("click", (event) => {
    window.dispatchEvent(new CustomEvent("idfAnalyzer:geometryLocate", { detail: { objectIndex: Number(event.currentTarget.dataset.inspectorConstruction), objectType: "Construction" } }));
  });
  elements.thermalTopologyInspector.querySelectorAll("[data-inspector-diagnostic]").forEach((button) => {
    button.addEventListener("click", async () => {
      await activeHelpers.selectGeometry?.("thermal_issue", button.dataset.inspectorDiagnostic, { syncLocate: false });
      await openSelectionInView("diagnose", { originView: "geometry", action: "open" });
    });
  });
  elements.thermalTopologyInspector.querySelectorAll("[data-inspector-view]").forEach((button) => {
    button.addEventListener("click", () => openSelectionInView(button.dataset.inspectorView, { originView: "geometry", action: "open" }));
  });
  elements.thermalTopologyInspector.querySelectorAll("[data-inspector-output-source]").forEach((button) => {
    button.addEventListener("click", () => openThermalTopologyOutputSource(button.dataset.inspectorOutputSource));
  });
  elements.thermalTopologyInspector.querySelector("[data-inspector-purpose-plan]")?.addEventListener("click", () => {
    window.dispatchEvent(new CustomEvent("idfAnalyzer:openSimulationPurposePlan"));
  });
}

function geometryRevealTarget(selection, geometry) {
  if (!selection) return null;
  if (selection.kind === "zone" || selection.kind === "space") return { kind: selection.kind, id: selection.item.entityId || selection.item.id };
  if (selection.kind === "window") return { kind: "window", id: selection.item.windowId || selection.item.entityId };
  if (selection.kind === "thermal_boundary") return { kind: "surface", id: selection.item.surfaceId };
  const thermal = resolveThermalTopologyTarget({ targetKind: selection.kind, targetId: selection.id }, geometry);
  if (thermal?.windowIds?.[0]) return { kind: "window", id: thermal.windowIds[0] };
  if (thermal?.surfaceIds?.[0]) return { kind: "surface", id: thermal.surfaceIds[0] };
  if (thermal?.nodeIds?.[0]) return { kind: "zone", id: thermal.nodeIds[0] };
  return null;
}

function selectionTitle(selection) {
  const item = selection.item || {};
  if (selection.kind === "thermal_observation") return "Geometric adjacency";
  return item.label || item.surfaceName || item.name || item.objectName || item.id || selection.id;
}

function selectionSubtitle(selection) {
  const item = selection.item || {};
  return humanize(item.relationKind || item.kind || item.surfaceType || selection.kind);
}

function selectionDiagnosticIDs(selection) {
  return [...(selection?.item?.diagnosticIds || [])];
}

function sourceAnchor(selection) {
  return selection?.item?.sourceAnchors?.[0] || selection?.item?.boundaries?.[0]?.sourceAnchors?.[0] || null;
}

function inspectorSection(title, content) {
  return `<section class="thermal-inspector-section"><h4>${escapeHTML(title)}</h4>${content}</section>`;
}

function renderRows(rows) {
  return `<dl class="thermal-inspector-rows">${rows.filter(([, value]) => value !== "" && value !== undefined && value !== null).map(([label, value]) => `<div><dt>${escapeHTML(label)}</dt><dd>${escapeHTML(value)}</dd></div>`).join("")}</dl>`;
}

function sourceButton(kind, id, label) {
  return `<button type="button" data-thermal-inspector-kind="${escapeHTML(kind)}" data-thermal-inspector-id="${escapeHTML(id)}">${escapeHTML(label)}</button>`;
}

function chip(value) {
  return `<span>${escapeHTML(value)}</span>`;
}

function dominantValues(values) {
  const counts = new Map();
  values.filter(Boolean).forEach((value) => counts.set(value, (counts.get(value) || 0) + 1));
  return [...counts.entries()].sort((left, right) => right[1] - left[1] || String(left[0]).localeCompare(String(right[0]))).map(([value]) => value).slice(0, 4);
}

function area(value) { return `${number(value)} m²`; }
function volume(value) { return `${number(value)} m³`; }
function percent(value) { return `${number((Number(value) || 0) * 100)}%`; }
function wattsPerKelvin(value, available) { return available ? `${number(value)} W/K` : "N/A"; }
function number(value) { return Number(value || 0).toLocaleString(undefined, { maximumFractionDigits: 2 }); }
function humanize(value) { return String(value || "N/A").replaceAll("_", " "); }
function emptyValue(text = "None") { return `<div class="empty compact">${escapeHTML(text)}</div>`; }
