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
const thermalTopologyLookupCache = new WeakMap();
const EMPTY_TOPOLOGY_ITEMS = Object.freeze([]);

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

  const lookup = createThermalTopologyLookup(topology);
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
    result.item = lookup.boundaryByID.get(id) || null;
    addBoundary(result, result.item, lookup);
  } else if (kind === "thermal_interface") {
    const pairedBoundaries = [...(lookup.boundariesByPairID.get(id) || [])];
    result.item = pairedBoundaries.length ? { id, boundaries: pairedBoundaries } : null;
    pairedBoundaries.forEach((boundary) => addBoundary(result, boundary, lookup));
  } else if (kind === "thermal_connection") {
    result.item = lookup.connectionByID.get(id) || null;
    if (result.item) {
      result.nodeIds.push(result.item.fromNodeId, result.item.toNodeId);
      for (const boundaryId of result.item.boundaryIds || []) {
        addBoundary(result, lookup.boundaryByID.get(boundaryId), lookup);
      }
      for (const openingId of result.item.openingIds || []) {
        addOpening(result, lookup.openingByID.get(openingId));
      }
      for (const couplingId of result.item.airCouplingIds || []) {
        addAirCoupling(result, lookup.airCouplingByID.get(couplingId));
      }
    }
  } else if (kind === "thermal_environment") {
    result.item = lookup.nodeByID.get(id) || null;
    if (result.item) result.nodeIds.push(result.item.id);
  } else if (kind === "thermal_air_coupling") {
    result.item = lookup.airCouplingByID.get(id) || null;
    addAirCoupling(result, result.item);
  } else if (kind === "thermal_issue") {
    result.item = lookup.issueByID.get(id) || null;
    addIssueRelations(result, result.item, lookup);
  } else if (kind === "thermal_observation") {
    const observation = lookup.observationByID.get(id) || null;
    if (observation) {
      const relatedBoundaries = [observation.surfaceAId, observation.surfaceBId]
        .map((surfaceID) => lookup.boundaryBySurfaceID.get(surfaceID))
        .filter(Boolean);
      result.item = {
        ...observation,
        id,
        boundaryIds: relatedBoundaries.map((boundary) => boundary.id),
        sourceAnchors: relatedBoundaries.flatMap((boundary) => boundary.sourceAnchors || []),
      };
      relatedBoundaries.forEach((boundary) => addBoundary(result, boundary, lookup));
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

function createThermalTopologyLookup(topology) {
  const collections = {
    boundaries: topology.boundaries || EMPTY_TOPOLOGY_ITEMS,
    openings: topology.openings || EMPTY_TOPOLOGY_ITEMS,
    airCouplings: topology.airCouplings || EMPTY_TOPOLOGY_ITEMS,
    connections: topology.connections || EMPTY_TOPOLOGY_ITEMS,
    nodes: topology.nodes || EMPTY_TOPOLOGY_ITEMS,
    issueLinks: topology.issueLinks || EMPTY_TOPOLOGY_ITEMS,
    adjacencyObservations: topology.adjacencyObservations || EMPTY_TOPOLOGY_ITEMS,
  };
  const cached = thermalTopologyLookupCache.get(topology);
  if (
    cached &&
    cached.sourceModelHash === topology.sourceModelHash &&
    Object.entries(collections).every(([key, values]) => cached.collections[key] === values && cached.lengths[key] === values.length)
  ) {
    return cached.lookup;
  }
  const boundariesByPairID = new Map();
  for (const boundary of collections.boundaries) {
    if (!boundary.pairId) continue;
    const paired = boundariesByPairID.get(boundary.pairId) || [];
    paired.push(boundary);
    boundariesByPairID.set(boundary.pairId, paired);
  }
  const lookup = {
    boundaryByID: indexFirst(collections.boundaries, (boundary) => boundary.id),
    boundaryBySurfaceID: indexFirst(collections.boundaries, (boundary) => boundary.surfaceId),
    boundariesByPairID,
    openingByID: indexFirst(collections.openings, (opening) => opening.id),
    airCouplingByID: indexFirst(collections.airCouplings, (coupling) => coupling.id),
    connectionByID: indexFirst(collections.connections, (connection) => connection.id),
    nodeByID: indexFirst(collections.nodes, (node) => node.id),
    issueByID: indexFirst(collections.issueLinks, (issue) => issue.id),
    observationByID: indexFirst(collections.adjacencyObservations, thermalTopologyObservationID),
  };
  thermalTopologyLookupCache.set(topology, {
    sourceModelHash: topology.sourceModelHash,
    collections,
    lengths: Object.fromEntries(Object.entries(collections).map(([key, values]) => [key, values.length])),
    lookup,
  });
  return lookup;
}

function indexFirst(values, keyForValue) {
  const index = new Map();
  for (const value of values) {
    const key = keyForValue(value);
    if (key && !index.has(key)) index.set(key, value);
  }
  return index;
}

function addBoundary(result, boundary, lookup) {
  if (!boundary) return;
  result.boundaryIds.push(boundary.id);
  result.surfaceIds.push(boundary.surfaceId);
  result.nodeIds.push(boundary.ownerSpaceId, boundary.ownerZoneId, boundary.targetId);
  for (const openingId of boundary.openingIds || []) {
    addOpening(result, lookup.openingByID.get(openingId));
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

function addIssueRelations(result, issue, lookup) {
  if (!issue) return;
  addBoundary(result, lookup.boundaryByID.get(issue.boundaryId), lookup);
  addOpening(result, lookup.openingByID.get(issue.openingId));
  addAirCoupling(result, lookup.airCouplingByID.get(issue.airCouplingId));
  for (const relatedId of issue.relatedEntityIds || []) {
    addBoundary(result, lookup.boundaryByID.get(relatedId), lookup);
    addOpening(result, lookup.openingByID.get(relatedId));
    addAirCoupling(result, lookup.airCouplingByID.get(relatedId));
    if (lookup.nodeByID.has(relatedId)) {
      result.nodeIds.push(relatedId);
    }
  }
}

function uniqueStrings(values) {
  return [...new Set(values.map((value) => String(value || "").trim()).filter(Boolean))];
}
