import { elements, escapeHTML, state } from "../state.js";
import { t } from "../i18n.js";
import { resolveThermalTopologyTarget } from "../thermal-topology-targets.js";

let activeHelpers = null;
let thermalDetailsInteractionsBound = false;

export function renderThermalTopologyDetails(geometry, helpers = {}) {
  const details = elements.topologyDetails;
  if (!details) return;
  activeHelpers = helpers;
  const activeSelection = resolveThermalDetailSelection(geometry);
  if (!activeSelection?.item) {
    details.removeAttribute("aria-labelledby");
    details.innerHTML = `<div class="empty">${escapeHTML(t("topology.networkDetailsEmpty", {}, "Select a network node or connection"))}</div>`;
    return;
  }
  details.setAttribute("aria-labelledby", "topologyDetailsHeading");
  details.innerHTML = `
    <div class="topology-detail-head">
      <div>
        <h3 id="topologyDetailsHeading">${escapeHTML(selectionTitle(activeSelection))}</h3>
        <span>${escapeHTML(selectionSubtitle(activeSelection))}</span>
      </div>
      <span class="topology-sync-note">${state.topologySyncLocate ? t("topology.syncOn") : t("topology.syncOff")}</span>
    </div>
    <div class="topology-detail-grid thermal-topology-detail-grid">
      ${renderSelectionDetails(activeSelection, geometry)}
    </div>`;
  bindThermalDetailsInteractions();
}

function resolveThermalDetailSelection(geometry) {
  const id = state.selectedTopologyEntityId || state.thermalTopologySelectedEntityId;
  const kind = state.selectedTopologyEntityKind || state.thermalTopologySelectedEntityKind;
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
  return details;
}

function renderZoneDetails(node, geometry) {
  const topology = geometry?.topology || {};
  const signature = (topology.zoneSignatures || []).find((item) => item.zoneId === node.id || item.zoneName === node.zoneName || item.zoneName === node.label) || {};
  const exposure = zonePhysicalExposure(node, topology);
  const adjacent = [...new Set([...(signature.adjacentZoneIds || []), ...(signature.airCoupledZoneIds || [])])];
  return [
    detailSection("Geometry", renderRows([
      ["Floor area", area(node.floorArea)],
      ["Volume", volume(node.volume)],
      ["Story", Number(node.storyIndex) + 1],
      ["Shell", signature.closedShell ? "Closed" : `Open · ${signature.openEdgeCount || 0} edges`],
    ])),
    detailSection("Thermal exposure", renderRows([
      ["Outdoors", area(exposure.exteriorArea)],
      ["Ground", area(exposure.groundArea)],
      ["Adjacent-zone", area(exposure.interzoneArea)],
      ["Adiabatic", area(exposure.adiabaticArea)],
      ["Opening ratio", percent(exposure.exteriorWwr)],
      ["Exterior UA", wattsPerKelvin(exposure.exteriorUa, exposure.hasExteriorUA)],
      ["Ground UA", wattsPerKelvin(exposure.groundUa, exposure.hasGroundUA)],
      ["Total UA", wattsPerKelvin(exposure.totalUa, exposure.hasTotalUA)],
    ])),
    detailSection("Adjacent zones", adjacent.length ? `<div class="thermal-detail-chips">${adjacent.map(chip).join("")}</div>` : emptyValue()),
    renderZoneHVACSummary(node),
  ].join("");
}

function renderConnectionDetails(connection, geometry) {
  const topology = geometry?.topology || {};
  const nodeByID = new Map((topology.nodes || []).map((node) => [node.id, node]));
  const boundaries = (topology.boundaries || []).filter((boundary) => (connection.boundaryIds || []).includes(boundary.id));
  const constructions = dominantValues(boundaries.map((boundary) => boundary.constructionName));
  const multiplier = areaMultiplier(connection, "physicalGrossArea", "effectiveGrossArea");
  return [
    detailSection("Connection", renderRows([
      ["From", nodeByID.get(connection.fromNodeId)?.label || connection.fromNodeId],
      ["To", nodeByID.get(connection.toNodeId)?.label || connection.toNodeId],
      ["Relation", humanize(connection.relationKind)],
      ["Surface count", connection.surfaceCount || 0],
      ["Opening count", connection.openingCount || 0],
      ["Dominant constructions", constructions.join(", ") || "—"],
    ])),
    detailSection("Area & UA", renderVariableTable([
      ["Multiplier", number(multiplier), "×"],
      ["Gross area", number(connection.physicalGrossArea), "m²"],
      ["Opaque area", number(connection.physicalOpaqueArea), "m²"],
      ["Opening area", number(connection.physicalOpeningArea), "m²"],
      ["Opaque UA", metricNumber(connection.physicalOpaqueUa, connection.hasPhysicalUa), "W/K"],
      ["Opening UA", metricNumber(connection.physicalOpeningUa, connection.hasPhysicalUa), "W/K"],
      ["Total UA", metricNumber(connection.physicalTotalUa, connection.hasPhysicalUa), "W/K"],
    ])),
    detailSection("Source boundaries", boundaries.length ? `<div class="thermal-detail-source-list">${boundaries.map((boundary) => sourceButton("thermal_boundary", boundary.id, boundary.surfaceName)).join("")}</div>` : emptyValue()),
  ].join("");
}

function renderBoundaryDetails(boundary, geometry) {
  const topology = geometry?.topology || {};
  const openings = (topology.openings || []).filter((opening) => (boundary.openingIds || []).includes(opening.id));
  const check = boundary.geometryCheck || {};
  const multiplier = areaMultiplier(boundary, "physicalGrossArea", "effectiveGrossArea");
  return [
    detailSection("Source boundary", renderRows([
      ["Object", boundary.surfaceName],
      ["Owner zone", boundary.ownerZoneId],
      ["Owner space", boundary.ownerSpaceId],
      ["OBC", `${boundary.boundaryConditionRaw || boundary.boundaryCondition} → ${boundary.targetName || boundary.targetId}`],
      ["Counterpart", boundary.counterpartSurfaceEntityId || (boundary.virtualCounterpart ? "Virtual" : "—")],
      ["Relation", humanize(boundary.relationKind)],
      ["Construction", `${boundary.constructionName || "—"} · ${boundary.hasUValue ? `${number(boundary.uValue)} W/m²K` : "U —"}`],
      ["Construction validation", humanize(boundary.constructionStatus || "not checked")],
    ])),
    detailSection("Area & UA", renderVariableTable([
      ["Multiplier", number(multiplier), "×"],
      ["Gross area", number(boundary.physicalGrossArea), "m²"],
      ["Opaque area", number(boundary.physicalOpaqueArea), "m²"],
      ["Opening area", number(boundary.physicalOpeningArea), "m²"],
      ["Opaque UA", metricNumber(unscaledMetric(boundary.opaqueUa, multiplier), boundary.hasUa), "W/K"],
      ["Opening UA", metricNumber(unscaledMetric(boundary.openingUa, multiplier), boundary.hasUa), "W/K"],
      ["Total UA", metricNumber(unscaledMetric(boundary.totalUa, multiplier), boundary.hasUa), "W/K"],
    ])),
    detailSection("Exposure & geometry", renderRows([
      ["Orientation", boundary.orientation || "—"],
      ["Azimuth", `${number(boundary.azimuth)}°`],
      ["Sun exposure", boundary.sunExposure || "—"],
      ["Wind exposure", boundary.windExposure || "—"],
      ["Validation", `${check.status || "not checked"}${check.message ? ` · ${check.message}` : ""}`],
      ["Overlap", percent(check.overlapRatio)],
      ["Plane distance", `${number(check.planeDistance)} m`],
    ])),
    detailSection("Openings", openings.length ? `<div class="thermal-detail-source-list">${openings.map((opening) => sourceButton("window", opening.entityId || opening.windowId, `${opening.name} · ${area(opening.physicalArea)}`)).join("")}</div>` : emptyValue()),
  ].join("");
}

function renderOpeningDetails(opening, geometry) {
  const multiplier = areaMultiplier(opening, "physicalArea", "effectiveArea");
  const physicalUA = Number(opening.physicalArea) * Number(opening.uValue);
  return [
    detailSection("Opening", renderRows([
      ["Base surface", opening.baseSurfaceId],
      ["Counterpart", opening.counterpartOpeningId || "—"],
      ["Construction", opening.constructionName || "—"],
    ])),
    detailSection("Area & UA", renderVariableTable([
      ["Multiplier", number(multiplier), "×"],
      ["Area", number(opening.physicalArea), "m²"],
      ["U-value", metricNumber(opening.uValue, opening.hasUValue), "W/m²K"],
      ["UA", metricNumber(physicalUA, opening.hasUValue), "W/K"],
    ])),
  ].join("");
}

function renderAirCouplingDetails(coupling, geometry) {
  const nodes = new Map((geometry?.topology?.nodes || []).map((node) => [node.id, node]));
  return [
    detailSection("Air coupling", renderRows([
      ["From", nodes.get(coupling.fromNodeId)?.label || coupling.fromNodeId],
      ["To", nodes.get(coupling.toNodeId)?.label || coupling.toNodeId],
      ["Object type", coupling.objectType],
      ["Object name", coupling.objectName],
      ["Kind", humanize(coupling.couplingKind)],
      ["Direction", coupling.direction || "—"],
      ["Design flow", coupling.designFlowRate ? `${number(coupling.designFlowRate)} ${coupling.unit || "m³/s"}` : "—"],
      ["Schedule", coupling.scheduleName || "—"],
      ["AFN surface", coupling.surfaceId || "—"],
      ["Component", coupling.componentName || "—"],
    ])),
  ].join("");
}

function renderObservationDetails(observation, geometry) {
  const topology = geometry?.topology || {};
  const boundaries = (topology.boundaries || []).filter((boundary) => (observation.boundaryIds || []).includes(boundary.id));
  return [
    detailSection("Geometric adjacency QA", renderRows([
      ["Observation", humanize(observation.observationKind)],
      ["Overlap", percent(observation.overlapRatio)],
      ["Declared connection", observation.declaredConnection ? "Yes" : "No"],
      ["Thermal relation", "Not created · QA evidence only"],
    ])),
    detailSection("Adjacent source surfaces", boundaries.length
      ? `<div class="thermal-detail-source-list">${boundaries.map((boundary) => sourceButton("thermal_boundary", boundary.id, boundary.surfaceName)).join("")}</div>`
      : emptyValue()),
  ].join("");
}

function renderEnvironmentDetails(node, geometry) {
  const topology = geometry?.topology || {};
  const connections = (topology.connections || []).filter((connection) => connection.fromNodeId === node.id || connection.toNodeId === node.id);
  const families = dominantValues(connections.map((connection) => humanize(connection.relationKind)));
  const physicalGrossArea = connections.reduce((sum, connection) => sum + (Number(connection.physicalGrossArea) || 0), 0);
  const effectiveGrossArea = connections.reduce((sum, connection) => sum + (Number(connection.effectiveGrossArea) || 0), 0);
  const totalUA = connections.reduce((sum, connection) => sum + (Number(connection.physicalTotalUa) || 0), 0);
  return detailSection("External target", [
    renderRows([
      ["Connected zones", new Set(connections.flatMap((connection) => [connection.fromNodeId, connection.toNodeId]).filter((id) => id !== node.id)).size],
      ["Boundary families", families.join(", ") || "—"],
    ]),
    renderVariableTable([
      ["Multiplier", number(areaRatio(physicalGrossArea, effectiveGrossArea)), "×"],
      ["Gross area", number(physicalGrossArea), "m²"],
      ["Total UA", metricNumber(totalUA, connections.some((connection) => connection.hasPhysicalUa)), "W/K"],
    ]),
  ].join(""));
}

function renderInterfaceDetails(item) {
  return detailSection("Reciprocal interface", renderRows([
    ["ID", item.id],
    ["Boundaries", (item.boundaries || []).map((boundary) => boundary.surfaceName).join(" ↔ ")],
  ]));
}

function renderIssueDetails(issue) {
  return detailSection("Topology issue", renderRows([
    ["Severity", issue.severity],
    ["Code", issue.code],
    ["Message", issue.message],
  ]));
}

function renderZoneHVACSummary(node) {
  const zoneName = node.zoneName || node.label || node.objectName || "";
  const summaries = (state.report?.hvac?.serviceModel?.zoneServices || []).filter((item) => sameThermalTopologyName(item.zoneName, zoneName));
  const paths = summaries.flatMap((item) => item.paths || []);
  const services = dominantValues(paths.map((path) => humanize(path.serviceKind)));
  const systems = dominantValues(paths.flatMap((path) => [path.airLoop?.name, path.plantLoop?.name, path.sourceSystem?.name]).filter(Boolean));
  return detailSection("HVAC service", paths.length
    ? renderRows([
      ["Service paths", paths.length],
      ["Services", services.join(", ") || "—"],
      ["Systems and loops", systems.join(", ") || "—"],
    ])
    : emptyValue("No HVAC service path targets this zone"));
}

function sameThermalTopologyName(left, right) {
  return String(left || "").trim().toLowerCase() === String(right || "").trim().toLowerCase();
}

function bindThermalDetailsInteractions() {
  const details = elements.topologyDetails;
  if (thermalDetailsInteractionsBound || !details) return;
  thermalDetailsInteractionsBound = true;
  details.addEventListener("click", (event) => {
    const button = event.target.closest?.("[data-thermal-detail-kind]");
    if (!button || !details.contains(button)) return;
    activeHelpers.selectTopologyEntity?.(button.dataset.thermalDetailKind, button.dataset.thermalDetailId);
  });
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

function zonePhysicalExposure(node, topology) {
  const result = {
    exteriorArea: 0,
    groundArea: 0,
    interzoneArea: 0,
    adiabaticArea: 0,
    exteriorOpeningArea: 0,
    exteriorUa: 0,
    groundUa: 0,
    totalUa: 0,
    hasExteriorUA: false,
    hasGroundUA: false,
    hasTotalUA: false,
    exteriorWwr: 0,
  };
  const isSpace = node.kind === "space";
  const boundaries = (topology.boundaries || []).filter((boundary) => (
    isSpace ? boundary.ownerSpaceId === node.id : boundary.ownerZoneId === node.id
  ));
  let totalArea = 0;
  let coveredArea = 0;
  let exteriorWallArea = 0;
  let exteriorWallOpeningArea = 0;
  let exteriorSeen = false;
  let groundSeen = false;
  let exteriorComplete = true;
  let groundComplete = true;
  for (const boundary of boundaries) {
    const grossArea = Number(boundary.physicalGrossArea) || 0;
    const openingArea = Number(boundary.physicalOpeningArea) || 0;
    const multiplier = areaMultiplier(boundary, "physicalGrossArea", "effectiveGrossArea");
    const totalUA = unscaledMetric(boundary.totalUa, multiplier);
    const hasUA = Boolean(boundary.hasUa);
    const relation = String(boundary.relationKind || "").toLowerCase();
    const transferable = !["adiabatic_explicit", "adiabatic_self_reference", "invalid"].includes(relation);
    if (transferable) {
      totalArea += grossArea;
      if (hasUA) {
        coveredArea += grossArea;
        result.totalUa += totalUA;
      }
    }
    if (relation === "exterior" || relation === "outdoors") {
      result.exteriorArea += grossArea;
      exteriorSeen = true;
      exteriorComplete &&= hasUA;
      if (hasUA) result.exteriorUa += totalUA;
      if (String(boundary.surfaceType || "").toLowerCase() === "wall") {
        exteriorWallArea += grossArea;
        exteriorWallOpeningArea += openingArea;
      }
    } else if (["ground", "foundation", "ground_preprocessor"].includes(relation)) {
      result.groundArea += grossArea;
      groundSeen = true;
      groundComplete &&= hasUA;
      if (hasUA) result.groundUa += totalUA;
    } else if (["interzone_explicit_surface", "interzone_implicit_zone", "interspace_implicit", "interzone"].includes(relation)) {
      result.interzoneArea += grossArea;
    } else if (["adiabatic_explicit", "adiabatic_self_reference", "adiabatic"].includes(relation)) {
      result.adiabaticArea += grossArea;
    }
  }
  result.hasExteriorUA = exteriorSeen && exteriorComplete;
  result.hasGroundUA = groundSeen && groundComplete;
  result.hasTotalUA = totalArea > 0 && Math.abs(coveredArea - totalArea) <= 1e-6;
  result.exteriorWwr = exteriorWallArea > 0 ? exteriorWallOpeningArea / exteriorWallArea : 0;
  return result;
}

function detailSection(title, content) {
  return `<section class="thermal-detail-section"><h4>${escapeHTML(title)}</h4>${content}</section>`;
}

function renderRows(rows) {
  return `<dl class="thermal-detail-rows">${rows.filter(([, value]) => value !== "" && value !== undefined && value !== null).map(([label, value]) => `<div><dt>${escapeHTML(label)}</dt><dd>${escapeHTML(value)}</dd></div>`).join("")}</dl>`;
}

function renderVariableTable(rows) {
  const body = rows
    .filter(([, value]) => value !== "" && value !== undefined && value !== null)
    .map(([variable, value, unit]) => `<tr><th scope="row">${escapeHTML(variable)}</th><td>${escapeHTML(value)}</td><td>${escapeHTML(unit || "—")}</td></tr>`)
    .join("");
  return `<div class="thermal-detail-table-wrap"><table class="thermal-detail-table"><thead><tr><th scope="col">Variable</th><th scope="col">Value</th><th scope="col">Unit</th></tr></thead><tbody>${body}</tbody></table></div>`;
}

function sourceButton(kind, id, label) {
  return `<button type="button" data-thermal-detail-kind="${escapeHTML(kind)}" data-thermal-detail-id="${escapeHTML(id)}">${escapeHTML(label)}</button>`;
}

function chip(value) {
  return `<span>${escapeHTML(value)}</span>`;
}

function dominantValues(values) {
  const counts = new Map();
  values.filter(Boolean).forEach((value) => counts.set(value, (counts.get(value) || 0) + 1));
  return [...counts.entries()].sort((left, right) => right[1] - left[1] || String(left[0]).localeCompare(String(right[0]))).map(([value]) => value).slice(0, 4);
}

function areaMultiplier(item, physicalKey, effectiveKey) {
  return areaRatio(item?.[physicalKey], item?.[effectiveKey]);
}

function areaRatio(physicalValue, effectiveValue) {
  const physical = Number(physicalValue) || 0;
  const effective = Number(effectiveValue) || 0;
  return physical > 0 && effective >= 0 ? effective / physical : 1;
}

function unscaledMetric(value, multiplier) {
  const scale = Number(multiplier) || 1;
  return (Number(value) || 0) / scale;
}

function metricNumber(value, available) {
  return available ? number(value) : "—";
}

function area(value) { return `${number(value)} m²`; }
function volume(value) { return `${number(value)} m³`; }
function percent(value) { return `${number((Number(value) || 0) * 100)}%`; }
function wattsPerKelvin(value, available) { return available ? `${number(value)} W/K` : "—"; }
function number(value) { return Number(value || 0).toLocaleString(undefined, { maximumFractionDigits: 2 }); }
function humanize(value) { return String(value || "—").replaceAll("_", " "); }
function emptyValue(text = "None") { return `<div class="empty compact">${escapeHTML(text)}</div>`; }
