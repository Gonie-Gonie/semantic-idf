export function createTopologyFocusContext(geometry, selection = {}) {
  const topology = geometry?.topology;
  const selectedID = String(selection.id || "");
  const selectedKind = normalizeKind(selection.kind);
  const context = emptyFocusContext();
  if (!topology || !selectedID || selectedKind === "story") return context;

  const nodes = topology.nodes || [];
  const connections = topology.connections || [];
  const boundaries = topology.boundaries || [];
  const openings = topology.openings || [];
  const airCouplings = topology.airCouplings || [];
  const issues = topology.issueLinks || [];
  const nodeByID = new Map(nodes.map((node) => [node.id, node]));
  const boundaryByID = new Map(boundaries.map((boundary) => [boundary.id, boundary]));
  const openingByID = new Map(openings.map((opening) => [opening.id, opening]));

  const seedNodeIDs = new Set(nodes
    .filter((node) => node.id === selectedID || node.entityId === selectedID)
    .map((node) => node.id));
  const selectedBoundaries = boundaries.filter((boundary) => boundaryMatches(boundary, selectedID));
  const selectedOpenings = openings.filter((opening) => openingMatches(opening, selectedID));
  const selectedAirCouplings = airCouplings.filter((coupling) => coupling.id === selectedID || coupling.entityId === selectedID);
  const selectedIssues = issues.filter((issue) => issue.id === selectedID || issue.entityId === selectedID);

  if (selectedKind === "zone" || nodes.some((node) => node.id === selectedID && node.kind === "zone")) {
    seedNodeIDs.add(selectedID);
    context.primaryNodeIds.add(selectedID);
    context.primaryZoneIds.add(selectedID);
    for (const signature of topology.zoneSignatures || []) {
      if (signature.zoneId === selectedID) {
        (signature.spaceIds || []).forEach((id) => seedNodeIDs.add(id));
      }
    }
    boundaries
      .filter((boundary) => boundary.ownerZoneId === selectedID)
      .forEach((boundary) => {
        if (boundary.ownerSpaceId) seedNodeIDs.add(boundary.ownerSpaceId);
      });
  }

  // Thermal connection endpoints are compacted to their parent zones even
  // when the selected topology node is a Space. Add those parent zones before
  // resolving one-hop edges so a Space selection retains its adjacent zones.
  addParentZoneSeeds(seedNodeIDs, topology);

  const selectedEdgeIDs = new Set(connections
    .filter((connection) => connectionMatches(connection, selectedID))
    .map((connection) => connection.id));
  selectedBoundaries.forEach((boundary) => {
    context.primaryBoundaryIds.add(boundary.id);
    if (boundary.surfaceId) context.primarySurfaceIds.add(boundary.surfaceId);
    if (boundary.surfaceEntityId) context.primarySurfaceIds.add(boundary.surfaceEntityId);
    connections
      .filter((connection) => (connection.boundaryIds || []).includes(boundary.id))
      .forEach((connection) => selectedEdgeIDs.add(connection.id));
  });
  selectedOpenings.forEach((opening) => {
    context.primaryOpeningIds.add(opening.id);
    if (opening.windowId) context.primaryWindowIds.add(opening.windowId);
    if (opening.entityId) context.primaryWindowIds.add(opening.entityId);
    connections
      .filter((connection) => (connection.openingIds || []).includes(opening.id))
      .forEach((connection) => selectedEdgeIDs.add(connection.id));
    const baseBoundary = boundaries.find((boundary) => boundary.surfaceId === opening.baseSurfaceId || boundary.id === opening.baseSurfaceId);
    if (baseBoundary) {
      connections
        .filter((connection) => (connection.boundaryIds || []).includes(baseBoundary.id))
        .forEach((connection) => selectedEdgeIDs.add(connection.id));
    }
  });
  selectedAirCouplings.forEach((coupling) => {
    connections
      .filter((connection) => (connection.airCouplingIds || []).includes(coupling.id))
      .forEach((connection) => selectedEdgeIDs.add(connection.id));
  });
  selectedIssues.forEach((issue) => {
    connections
      .filter((connection) => (connection.diagnosticIds || []).includes(issue.id)
        || (issue.boundaryId && (connection.boundaryIds || []).includes(issue.boundaryId))
        || (issue.openingId && (connection.openingIds || []).includes(issue.openingId))
        || (issue.airCouplingId && (connection.airCouplingIds || []).includes(issue.airCouplingId)))
      .forEach((connection) => selectedEdgeIDs.add(connection.id));
  });

  const relatedConnections = selectedEdgeIDs.size
    ? connections.filter((connection) => selectedEdgeIDs.has(connection.id))
    : seedNodeIDs.size
      ? connections.filter((connection) => seedNodeIDs.has(connection.fromNodeId) || seedNodeIDs.has(connection.toNodeId))
      : [];

  seedNodeIDs.forEach((id) => context.nodeIds.add(id));
  nodes
    .filter((node) => node.id === selectedID || node.entityId === selectedID)
    .forEach((node) => {
      context.primaryNodeIds.add(node.id);
      if (node.kind === "zone") context.primaryZoneIds.add(node.id);
    });
  selectedEdgeIDs.forEach((id) => context.primaryConnectionIds.add(id));
  relatedConnections.forEach((connection) => addConnectionToFocus(context, connection, boundaryByID, openingByID));
  selectedBoundaries.forEach((boundary) => addBoundaryToFocus(context, boundary));
  selectedOpenings.forEach((opening) => addOpeningToFocus(context, opening));
  selectedAirCouplings.forEach((coupling) => context.airCouplingIds.add(coupling.id));

  addParentZones(context, topology, nodeByID);
  context.active = context.nodeIds.size > 0
    || context.connectionIds.size > 0
    || context.surfaceIds.size > 0
    || context.windowIds.size > 0;
  return context;
}

function addParentZoneSeeds(seedNodeIDs, topology) {
  const spaceToZone = new Map();
  for (const boundary of topology.boundaries || []) {
    if (boundary.ownerSpaceId && boundary.ownerZoneId) spaceToZone.set(boundary.ownerSpaceId, boundary.ownerZoneId);
  }
  for (const signature of topology.zoneSignatures || []) {
    for (const spaceID of signature.spaceIds || []) spaceToZone.set(spaceID, signature.zoneId);
  }
  for (const nodeID of [...seedNodeIDs]) {
    const parentZoneID = spaceToZone.get(nodeID);
    if (parentZoneID) seedNodeIDs.add(parentZoneID);
  }
}

function emptyFocusContext() {
  return {
    active: false,
    primaryNodeIds: new Set(),
    primaryZoneIds: new Set(),
    primaryConnectionIds: new Set(),
    primaryBoundaryIds: new Set(),
    primarySurfaceIds: new Set(),
    primaryOpeningIds: new Set(),
    primaryWindowIds: new Set(),
    nodeIds: new Set(),
    zoneIds: new Set(),
    connectionIds: new Set(),
    boundaryIds: new Set(),
    surfaceIds: new Set(),
    openingIds: new Set(),
    windowIds: new Set(),
    airCouplingIds: new Set(),
  };
}

function addConnectionToFocus(context, connection, boundaryByID, openingByID) {
  context.connectionIds.add(connection.id);
  if (connection.fromNodeId) context.nodeIds.add(connection.fromNodeId);
  if (connection.toNodeId) context.nodeIds.add(connection.toNodeId);
  for (const boundaryID of connection.boundaryIds || []) {
    context.boundaryIds.add(boundaryID);
    const boundary = boundaryByID.get(boundaryID);
    if (boundary) addBoundaryToFocus(context, boundary);
  }
  for (const openingID of connection.openingIds || []) {
    context.openingIds.add(openingID);
    const opening = openingByID.get(openingID);
    if (opening) addOpeningToFocus(context, opening);
  }
  (connection.airCouplingIds || []).forEach((id) => context.airCouplingIds.add(id));
}

function addBoundaryToFocus(context, boundary) {
  context.boundaryIds.add(boundary.id);
  if (boundary.surfaceId) context.surfaceIds.add(boundary.surfaceId);
  if (boundary.surfaceEntityId) context.surfaceIds.add(boundary.surfaceEntityId);
  if (boundary.counterpartSurfaceId) context.surfaceIds.add(boundary.counterpartSurfaceId);
  if (boundary.counterpartSurfaceEntityId) context.surfaceIds.add(boundary.counterpartSurfaceEntityId);
  if (boundary.ownerZoneId) context.zoneIds.add(boundary.ownerZoneId);
  if (boundary.ownerSpaceId) context.nodeIds.add(boundary.ownerSpaceId);
  if (boundary.targetKind === "zone" || boundary.targetKind === "space") context.nodeIds.add(boundary.targetId);
  (boundary.openingIds || []).forEach((id) => context.openingIds.add(id));
}

function addOpeningToFocus(context, opening) {
  context.openingIds.add(opening.id);
  if (opening.windowId) context.windowIds.add(opening.windowId);
  if (opening.entityId) context.windowIds.add(opening.entityId);
  if (opening.baseSurfaceId) context.surfaceIds.add(opening.baseSurfaceId);
  if (opening.counterpartOpeningId) context.openingIds.add(opening.counterpartOpeningId);
  if (opening.ownerZoneId) context.zoneIds.add(opening.ownerZoneId);
  if (opening.ownerSpaceId) context.nodeIds.add(opening.ownerSpaceId);
}

function addParentZones(context, topology, nodeByID) {
  const spaceToZone = new Map();
  for (const boundary of topology.boundaries || []) {
    if (boundary.ownerSpaceId && boundary.ownerZoneId) spaceToZone.set(boundary.ownerSpaceId, boundary.ownerZoneId);
  }
  for (const signature of topology.zoneSignatures || []) {
    for (const spaceID of signature.spaceIds || []) spaceToZone.set(spaceID, signature.zoneId);
  }
  for (const nodeID of context.nodeIds) {
    const node = nodeByID.get(nodeID);
    if (node?.kind === "zone") context.zoneIds.add(node.id);
    const parentZone = spaceToZone.get(nodeID);
    if (parentZone) context.zoneIds.add(parentZone);
  }
}

function connectionMatches(connection, targetID) {
  return connection.id === targetID
    || (connection.boundaryIds || []).includes(targetID)
    || (connection.openingIds || []).includes(targetID)
    || (connection.airCouplingIds || []).includes(targetID)
    || (connection.diagnosticIds || []).includes(targetID);
}

function boundaryMatches(boundary, targetID) {
  return boundary.id === targetID
    || boundary.surfaceId === targetID
    || boundary.surfaceEntityId === targetID
    || boundary.pairId === targetID;
}

function openingMatches(opening, targetID) {
  return opening.id === targetID
    || opening.windowId === targetID
    || opening.entityId === targetID
    || opening.pairId === targetID;
}

function normalizeKind(kind) {
  const normalized = String(kind || "").trim().toLowerCase();
  return normalized === "fenestration" ? "window" : normalized;
}
