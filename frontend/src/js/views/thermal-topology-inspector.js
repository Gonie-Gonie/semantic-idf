import { elements, escapeHTML, state } from "../state.js";
import { t } from "../i18n.js";
import { openSelectionInView, revealSelectionSource } from "../selection-controller.js";
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
    elements.thermalTopologyInspector.innerHTML = `<div class="empty">${escapeHTML(t("topology.inspectorEmpty", {}, "Select a thermal node or connection"))}</div>`;
    return;
  }
  const { kind, id, item } = activeSelection;
  const attributes = helpers.navigationAttributes?.(kind, id) || "";
  elements.thermalTopologyInspector.innerHTML = `
    <div class="thermal-inspector-head navigable-row" ${attributes}>
      <div>
        <h3>${escapeHTML(selectionTitle(activeSelection))}</h3>
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
  const kind = state.selectedGeometryKind;
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
  if (selection.kind === "zone" || selection.kind === "space") return renderZoneDetails(selection.item, geometry);
  if (selection.kind === "thermal_connection") return renderConnectionDetails(selection.item, geometry);
  if (selection.kind === "thermal_boundary") return renderBoundaryDetails(selection.item, geometry);
  if (selection.kind === "window") return renderOpeningDetails(selection.item, geometry);
  if (selection.kind === "thermal_air_coupling") return renderAirCouplingDetails(selection.item, geometry);
  if (selection.kind === "thermal_environment") return renderEnvironmentDetails(selection.item, geometry);
  if (selection.kind === "thermal_interface") return renderInterfaceDetails(selection.item, geometry);
  if (selection.kind === "thermal_issue") return renderIssueDetails(selection.item);
  return renderRows([["ID", selection.id], ["Kind", selection.kind]]);
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
      ${(selection.kind === "zone" || selection.kind === "space") ? `<button type="button" data-inspector-view="profile">Profile</button><button type="button" data-inspector-view="hvac">HVAC</button><button type="button" data-inspector-view="output">Output</button>` : ""}
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
