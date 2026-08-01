export const THERMAL_TOPOLOGY_TARGET_KINDS = Object.freeze([
  "thermal_boundary",
  "thermal_interface",
  "thermal_connection",
  "thermal_environment",
  "thermal_air_coupling",
  "thermal_issue",
  "thermal_observation",
]);

const thermalTopologyTargetKindSet = new Set(THERMAL_TOPOLOGY_TARGET_KINDS);

export function normalizeThermalTopologyTargetKind(kind) {
  return String(kind || "").trim().toLowerCase().replaceAll("-", "_");
}

export function isThermalTopologyTargetKind(kind) {
  return thermalTopologyTargetKindSet.has(normalizeThermalTopologyTargetKind(kind));
}

export function thermalTopologyTargetExists(target, geometry) {
  return Boolean(resolveThermalTopologyTarget(target, geometry));
}

export function thermalTopologyObservationID(observation) {
  const surfaces = [observation?.surfaceAId, observation?.surfaceBId].map((value) => String(value || "")).sort();
  return `thermal-observation:${surfaces[0]}:${surfaces[1]}:${String(observation?.observationKind || "adjacency")}`;
}

export function resolveThermalTopologyTarget(target, geometry) {
  const kind = normalizeThermalTopologyTargetKind(target?.targetKind);
  const id = String(target?.targetId || "");
  const topology = geometry?.topology;
  if (!thermalTopologyTargetKindSet.has(kind) || !id || !topology) {
    return null;
  }

  const boundaries = topology.boundaries || [];
  const openings = topology.openings || [];
  const airCouplings = topology.airCouplings || [];
  const nodes = topology.nodes || [];
  const issueLinks = topology.issueLinks || [];
  const result = {
    kind,
    id,
    item: null,
    boundaryIds: [],
    openingIds: [],
    airCouplingIds: [],
    surfaceIds: [],
    windowIds: [],
    nodeIds: [],
  };

  if (kind === "thermal_boundary") {
    result.item = boundaries.find((boundary) => boundary.id === id) || null;
    addBoundary(result, result.item, openings);
  } else if (kind === "thermal_interface") {
    const pairedBoundaries = boundaries.filter((boundary) => boundary.pairId === id);
    result.item = pairedBoundaries.length ? { id, boundaries: pairedBoundaries } : null;
    pairedBoundaries.forEach((boundary) => addBoundary(result, boundary, openings));
  } else if (kind === "thermal_connection") {
    result.item = (topology.connections || []).find((connection) => connection.id === id) || null;
    if (result.item) {
      result.nodeIds.push(result.item.fromNodeId, result.item.toNodeId);
      for (const boundaryId of result.item.boundaryIds || []) {
        addBoundary(result, boundaries.find((boundary) => boundary.id === boundaryId), openings);
      }
      for (const openingId of result.item.openingIds || []) {
        addOpening(result, openings.find((opening) => opening.id === openingId));
      }
      for (const couplingId of result.item.airCouplingIds || []) {
        addAirCoupling(result, airCouplings.find((coupling) => coupling.id === couplingId));
      }
    }
  } else if (kind === "thermal_environment") {
    result.item = nodes.find((node) => node.id === id) || null;
    if (result.item) result.nodeIds.push(result.item.id);
  } else if (kind === "thermal_air_coupling") {
    result.item = airCouplings.find((coupling) => coupling.id === id) || null;
    addAirCoupling(result, result.item);
  } else if (kind === "thermal_issue") {
    result.item = issueLinks.find((issue) => issue.id === id) || null;
    addIssueRelations(result, result.item, { boundaries, openings, airCouplings, nodes });
  } else if (kind === "thermal_observation") {
    const observation = (topology.adjacencyObservations || []).find((candidate) => thermalTopologyObservationID(candidate) === id) || null;
    if (observation) {
      const relatedBoundaries = [observation.surfaceAId, observation.surfaceBId]
        .map((surfaceID) => boundaries.find((boundary) => boundary.surfaceId === surfaceID))
        .filter(Boolean);
      result.item = {
        ...observation,
        id,
        boundaryIds: relatedBoundaries.map((boundary) => boundary.id),
        sourceAnchors: relatedBoundaries.flatMap((boundary) => boundary.sourceAnchors || []),
      };
      relatedBoundaries.forEach((boundary) => addBoundary(result, boundary, openings));
    }
  }

  if (!result.item) {
    return null;
  }
  for (const key of ["boundaryIds", "openingIds", "airCouplingIds", "surfaceIds", "windowIds", "nodeIds"]) {
    result[key] = uniqueStrings(result[key]);
  }
  return result;
}

function addBoundary(result, boundary, openings) {
  if (!boundary) return;
  result.boundaryIds.push(boundary.id);
  result.surfaceIds.push(boundary.surfaceId);
  result.nodeIds.push(boundary.ownerSpaceId, boundary.ownerZoneId, boundary.targetId);
  for (const openingId of boundary.openingIds || []) {
    addOpening(result, openings.find((opening) => opening.id === openingId));
  }
}

function addOpening(result, opening) {
  if (!opening) return;
  result.openingIds.push(opening.id);
  result.windowIds.push(opening.windowId);
  result.surfaceIds.push(opening.baseSurfaceId);
  result.nodeIds.push(opening.ownerSpaceId, opening.ownerZoneId);
}

function addAirCoupling(result, coupling) {
  if (!coupling) return;
  result.airCouplingIds.push(coupling.id);
  result.nodeIds.push(coupling.fromNodeId, coupling.toNodeId);
  if (coupling.surfaceId) {
    result.surfaceIds.push(coupling.surfaceId);
    result.windowIds.push(coupling.surfaceId);
  }
}

function addIssueRelations(result, issue, topology) {
  if (!issue) return;
  addBoundary(result, topology.boundaries.find((boundary) => boundary.id === issue.boundaryId), topology.openings);
  addOpening(result, topology.openings.find((opening) => opening.id === issue.openingId));
  addAirCoupling(result, topology.airCouplings.find((coupling) => coupling.id === issue.airCouplingId));
  for (const relatedId of issue.relatedEntityIds || []) {
    addBoundary(result, topology.boundaries.find((boundary) => boundary.id === relatedId), topology.openings);
    addOpening(result, topology.openings.find((opening) => opening.id === relatedId));
    addAirCoupling(result, topology.airCouplings.find((coupling) => coupling.id === relatedId));
    if (topology.nodes.some((node) => node.id === relatedId)) {
      result.nodeIds.push(relatedId);
    }
  }
}

function uniqueStrings(values) {
  return [...new Set(values.map((value) => String(value || "").trim()).filter(Boolean))];
}
