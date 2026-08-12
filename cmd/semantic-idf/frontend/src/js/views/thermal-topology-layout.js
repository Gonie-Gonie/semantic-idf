import { normalizeThermalTopologyLayout } from "../state.js";
import { thermalTopologyObservationID } from "../thermal-topology-targets.js";

export const THERMAL_NODE_WIDTH = 148;
export const THERMAL_NODE_HEIGHT = 56;
export const THERMAL_ENDPOINT_RADIUS = 7;
const LAYOUT_PADDING = 92;
const STORY_LANE_GAP = 150;

export function thermalTopologyLayoutCacheKey(geometry, options = {}, viewport = {}) {
  const topology = geometry?.topology || {};
  const topologyHash = topology.sourceModelHash || [
    topology.schema || "thermal-topology",
    ...(topology.nodes || []).map((node) => node.id),
    ...(topology.connections || []).map((connection) => connection.id),
  ].join(":");
  return [
    topologyHash,
    normalizeThermalTopologyLayout(options.layout),
    options.metric || "topology",
    options.storyIndex ?? "all",
    Boolean(options.showAirCoupling),
    Math.round((Number(viewport.width) || 900) / 50) * 50,
    Math.round((Number(viewport.height) || 600) / 50) * 50,
  ].join("|");
}

export function createThermalTopologyLayoutModel(geometry, options = {}) {
  const topology = geometry?.topology || {};
  const base = {
    schema: topology.schema || "",
    layout: normalizeThermalTopologyLayout(options.layout),
    storyIndex: options.storyIndex ?? "all",
    metric: String(options.metric || "topology"),
    nodes: [...(topology.nodes || [])],
    connections: (topology.connections || []).filter((connection) => options.showAirCoupling || connection.relationKind !== "air_coupling"),
    boundaries: [...(topology.boundaries || [])],
    allOpenings: [...(topology.openings || [])],
    airCouplings: options.showAirCoupling ? [...(topology.airCouplings || [])] : [],
    issueLinks: [...(topology.issueLinks || [])],
    zoneSignatures: [...(topology.zoneSignatures || [])],
  };
  if (base.metric === "qa") {
    base.connections.push(...qaObservationConnections(topology, base.boundaries));
  }
  const scoped = applyThermalTopologyLevel(base, options.storyIndex);
  const projected = projectAdiabaticBoundaries(scoped);
  return {
    ...projected,
    cacheKey: thermalTopologyLayoutCacheKey(geometry, options),
  };
}

export function computeThermalTopologyLayout(model, viewport = {}) {
  const width = Math.max(360, Number(viewport.width) || 900);
  const height = Math.max(280, Number(viewport.height) || 600);
  const positions = model.layout === "network"
    ? computeNetworkLayout(model, { width, height })
    : computeSpatialLayout(model, { width, height });
  resolveNodeCollisions(positions, model.nodes, { width, height });
  const layout = { width, height, positions, edges: [] };
  rerouteThermalTopologyEdges(model, layout);
  return layout;
}

export function rerouteThermalTopologyEdges(model, layout) {
  const parallelCounts = new Map();
  const nodeByID = new Map(model.nodes.map((node) => [node.id, node]));
  layout.edges = (model.connections || []).map((connection) => {
    const pairKey = connection.presentationKind === "adiabatic_stub"
      ? `${connection.fromNodeId}|adiabatic|${thermalConnectionSide(connection)}`
      : sortedPairKey(connection.fromNodeId, connection.toNodeId);
    const lane = parallelCounts.get(pairKey) || 0;
    parallelCounts.set(pairKey, lane + 1);
    return {
      ...connection,
      route: routeThermalEdgeWithNodeIndex(
        connection,
        layout.positions[connection.fromNodeId],
        layout.positions[connection.toNodeId],
        lane,
        nodeByID,
      ),
    };
  }).filter((edge) => edge.route);
  return layout.edges;
}

export function computeSpatialLayout(model, viewport = {}) {
  const width = Math.max(360, Number(viewport.width) || 900);
  const height = Math.max(280, Number(viewport.height) || 600);
  const internal = model.nodes.filter((node) => !isExternalNode(node));
  const external = model.nodes.filter(isExternalNode);
  const stories = [...new Set(internal.map((node) => Number(node.storyIndex) || 0))].sort((a, b) => a - b);
  const storyLaneByIndex = new Map(stories.map((story, index) => [story, index]));
  const centroids = internal.map((node) => node.centroid || {}).filter((point) => Number.isFinite(Number(point.x)) && Number.isFinite(Number(point.y)));
  const xValues = centroids.map((point) => Number(point.x));
  const yValues = centroids.map((point) => Number(point.y));
  const minX = Math.min(...xValues, 0);
  const maxX = Math.max(...xValues, 1);
  const minY = Math.min(...yValues, 0);
  const maxY = Math.max(...yValues, 1);
  const laneHeight = model.storyIndex !== "all" || stories.length <= 1
    ? height - LAYOUT_PADDING * 2
    : Math.max(STORY_LANE_GAP, (height - LAYOUT_PADDING * 2) / Math.max(stories.length, 1));
  const positions = {};
  internal
    .sort(compareNodes)
    .forEach((node, index) => {
      const storyLane = storyLaneByIndex.get(Number(node.storyIndex) || 0) || 0;
      const centroid = node.centroid || {};
      const normalizedX = normalizeRange(Number(centroid.x), minX, maxX, (index + 1) / (internal.length + 1));
      const normalizedY = normalizeRange(Number(centroid.y), minY, maxY, ((index * 7) % Math.max(1, internal.length)) / Math.max(1, internal.length));
      const yStart = model.storyIndex !== "all" || stories.length <= 1 ? LAYOUT_PADDING : LAYOUT_PADDING + storyLane * laneHeight;
      positions[node.id] = {
        x: LAYOUT_PADDING + normalizedX * Math.max(1, width - LAYOUT_PADDING * 2),
        y: yStart + (1 - normalizedY) * Math.max(1, laneHeight - THERMAL_NODE_HEIGHT),
      };
    });
  placeExternalNodes(external, positions, model.connections, { width, height });
  return positions;
}

export function computeNetworkLayout(model, viewport = {}) {
  const width = Math.max(360, Number(viewport.width) || 900);
  const height = Math.max(280, Number(viewport.height) || 600);
  const degree = nodeDegrees(model.connections);
  const neighborsByNode = connectionNeighbors(model.connections);
  const internal = model.nodes.filter((node) => !isExternalNode(node));
  const external = model.nodes.filter(isExternalNode);
  const storyGroups = new Map();
  for (const node of internal) {
    const story = Number(node.storyIndex) || 0;
    if (!storyGroups.has(story)) storyGroups.set(story, []);
    storyGroups.get(story).push(node);
  }
  const stories = [...storyGroups.keys()].sort((a, b) => a - b);
  const positions = {};
  stories.forEach((story, columnIndex) => {
    const nodes = storyGroups.get(story).sort((left, right) => (
      (degree.get(right.id) || 0) - (degree.get(left.id) || 0) || compareNodes(left, right)
    ));
    const x = stories.length <= 1
      ? width / 2
      : LAYOUT_PADDING + columnIndex * ((width - LAYOUT_PADDING * 2) / Math.max(1, stories.length - 1));
    const ordered = barycentricOrder(nodes, neighborsByNode, positions);
    ordered.forEach((node, rowIndex) => {
      positions[node.id] = {
        x,
        y: LAYOUT_PADDING + (rowIndex + 1) * ((height - LAYOUT_PADDING * 2) / (ordered.length + 1)),
      };
    });
  });
  placeExternalNodes(external, positions, model.connections, { width, height });
  return positions;
}

export function routeThermalEdge(connection, source, target, parallelIndex = 0, nodes = []) {
  const nodeByID = nodes instanceof Map ? nodes : new Map(nodes.map((node) => [node.id, node]));
  return routeThermalEdgeWithNodeIndex(connection, source, target, parallelIndex, nodeByID);
}

function routeThermalEdgeWithNodeIndex(connection, source, target, parallelIndex, nodeByID) {
  if (connection.presentationKind === "adiabatic_stub") {
    return routeAdiabaticStub(connection, source, parallelIndex, nodeByID.get(connection.fromNodeId));
  }
  if (!source || !target) {
    return null;
  }
  if (connection.fromNodeId === connection.toNodeId) {
    const x = source.x + THERMAL_NODE_WIDTH / 2;
    const y = source.y - THERMAL_NODE_HEIGHT / 2;
    return {
      path: `M ${x} ${y} C ${x + 62} ${y - 58}, ${x - 62} ${y - 58}, ${x} ${y}`,
      labelX: x,
      labelY: y - 52,
      sourcePort: "top",
      targetPort: "top",
      selfLoop: true,
    };
  }
  const sourceNode = nodeByID.get(connection.fromNodeId);
  const targetNode = nodeByID.get(connection.toNodeId);
  const ports = chooseThermalPorts(source, target, sourceNode, targetNode);
  const start = portPoint(source, ports.sourcePort, sourceNode);
  const end = portPoint(target, ports.targetPort, targetNode);
  const dx = end.x - start.x;
  const dy = end.y - start.y;
  const length = Math.max(1, Math.hypot(dx, dy));
  const isAir = connection.relationKind === "air_coupling" || (connection.airCouplingIds || []).length > 0;
  const laneOffset = (parallelIndex + (isAir ? 1 : 0)) * 12;
  const offsetX = (-dy / length) * laneOffset;
  const offsetY = (dx / length) * laneOffset;
  const sx = start.x + offsetX;
  const sy = start.y + offsetY;
  const tx = end.x + offsetX;
  const ty = end.y + offsetY;
  const horizontal = ports.sourcePort === "left" || ports.sourcePort === "right";
  const path = horizontal
    ? `M ${sx} ${sy} C ${(sx + tx) / 2} ${sy}, ${(sx + tx) / 2} ${ty}, ${tx} ${ty}`
    : `M ${sx} ${sy} C ${sx} ${(sy + ty) / 2}, ${tx} ${(sy + ty) / 2}, ${tx} ${ty}`;
  return {
    path,
    labelX: (sx + tx) / 2 + (-dy / length) * 14,
    labelY: (sy + ty) / 2 + (dx / length) * 14,
    ...ports,
    selfLoop: false,
  };
}

function routeAdiabaticStub(connection, source, parallelIndex, sourceNode) {
  if (!source) return null;
  const side = thermalConnectionSide(connection);
  const direction = sideVector(side);
  const perpendicular = { x: -direction.y, y: direction.x };
  const sourceSize = thermalNodeSize(sourceNode);
  const halfExtent = side === "left" || side === "right" ? sourceSize.width / 2 : sourceSize.height / 2;
  const laneOffset = alternatingLaneOffset(parallelIndex) * 11;
  const gap = 9;
  const length = 30;
  const start = {
    x: source.x + direction.x * (halfExtent + gap) + perpendicular.x * laneOffset,
    y: source.y + direction.y * (halfExtent + gap) + perpendicular.y * laneOffset,
  };
  const end = { x: start.x + direction.x * length, y: start.y + direction.y * length };
  const capHalf = 5;
  return {
    path: `M ${start.x} ${start.y} L ${end.x} ${end.y}`,
    capPath: `M ${end.x - perpendicular.x * capHalf} ${end.y - perpendicular.y * capHalf} L ${end.x + perpendicular.x * capHalf} ${end.y + perpendicular.y * capHalf}`,
    labelX: end.x + direction.x * 8,
    labelY: end.y + direction.y * 8,
    sourcePort: side,
    targetPort: side,
    selfLoop: false,
    adiabaticStub: true,
  };
}

export function applyThermalTopologyLevel(model, storyIndex = "all") {
  if (storyIndex === "all") {
    return model;
  }
  const connections = model.connections || [];
  const includedConnections = new Set();
  const included = new Set();
  for (const node of model.nodes) {
    if (!isExternalNode(node) && Number(node.storyIndex) === Number(storyIndex)) {
      included.add(node.id);
    }
  }
  const levelNodes = new Set(included);
  const nodeByID = new Map((model.nodes || []).map((node) => [node.id, node]));
  for (const connection of connections) {
    const fromOnLevel = levelNodes.has(connection.fromNodeId);
    const toOnLevel = levelNodes.has(connection.toNodeId);
    const fromExternal = isExternalNode(nodeByID.get(connection.fromNodeId));
    const toExternal = isExternalNode(nodeByID.get(connection.toNodeId));
    if ((fromOnLevel && (toOnLevel || toExternal)) || (toOnLevel && (fromOnLevel || fromExternal))) {
      includedConnections.add(connection);
      included.add(connection.fromNodeId);
      included.add(connection.toNodeId);
    }
  }
  const scopedConnections = connections.filter((connection) => includedConnections.has(connection));
  return {
    ...model,
    nodes: model.nodes.filter((node) => included.has(node.id)),
    connections: scopedConnections,
  };
}

export function thermalTopologyFocusContext(model, selectedEntityId = "") {
  const selected = String(selectedEntityId || "");
  const nodeIDs = new Set();
  const edgeIDs = new Set();
  if (!selected) return { active: false, nodeIDs, edgeIDs };

  const selectedNodes = (model.nodes || []).filter((node) => (
    node.id === selected || node.entityId === selected || node.sourceId === selected
  ));
  const exactEdges = (model.connections || []).filter((connection) => thermalConnectionMatchesTarget(connection, selected));
  const relatedEdges = exactEdges.length
    ? exactEdges
    : selectedNodes.length
      ? (model.connections || []).filter((connection) => selectedNodes.some((node) => connection.fromNodeId === node.id || connection.toNodeId === node.id))
      : [];

  selectedNodes.forEach((node) => nodeIDs.add(node.id));
  relatedEdges.forEach((connection) => {
    edgeIDs.add(connection.id);
    if (connection.fromNodeId) nodeIDs.add(connection.fromNodeId);
    if (connection.toNodeId) nodeIDs.add(connection.toNodeId);
  });
  return { active: selectedNodes.length > 0 || exactEdges.length > 0, nodeIDs, edgeIDs };
}

function thermalConnectionMatchesTarget(connection, targetID) {
  return connection.id === targetID
    || connection.targetId === targetID
    || connection.sourceConnectionId === targetID
    || (connection.boundaryIds || []).includes(targetID)
    || (connection.openingIds || []).includes(targetID)
    || (connection.airCouplingIds || []).includes(targetID)
    || (connection.diagnosticIds || []).includes(targetID);
}

function qaObservationConnections(topology, boundaries) {
  const bySurfaceID = new Map(boundaries.map((boundary) => [boundary.surfaceId, boundary]));
  return (topology.adjacencyObservations || []).map((observation) => {
    const left = bySurfaceID.get(observation.surfaceAId);
    const right = bySurfaceID.get(observation.surfaceBId);
    if (!left || !right) return null;
    return {
      id: thermalTopologyObservationID(observation),
      fromNodeId: left.ownerSpaceId || left.ownerZoneId,
      toNodeId: right.ownerSpaceId || right.ownerZoneId,
      targetKind: "thermal_observation",
      targetId: thermalTopologyObservationID(observation),
      relationKind: "qa_observation",
      observationKind: observation.observationKind,
      qaOnly: true,
      boundaryIds: [left.id, right.id],
      diagnosticIds: [],
      surfaceCount: 0,
    };
  }).filter(Boolean);
}

function projectAdiabaticBoundaries(model) {
  const nodeByID = new Map((model.nodes || []).map((node) => [node.id, node]));
  const nodeIDs = new Set(nodeByID.keys());
  const boundaryByID = new Map((model.boundaries || []).map((boundary) => [boundary.id, boundary]));
  const connections = [];
  for (const connection of model.connections || []) {
    if (!isAdiabaticConnection(connection)) {
      connections.push(connection);
      continue;
    }
    const boundaries = (connection.boundaryIds || []).map((id) => boundaryByID.get(id)).filter(Boolean);
    for (const boundary of boundaries) {
      const ownerNodeID = [connection.fromNodeId, connection.toNodeId]
        .find((id) => nodeIDs.has(id) && !isSharedAdiabaticNode(nodeByID.get(id)))
        || boundary.ownerZoneId;
      if (!ownerNodeID || !nodeIDs.has(ownerNodeID)) continue;
      connections.push({
        ...connection,
        id: `${connection.id}:stub:${boundary.id}`,
        sourceConnectionId: connection.id,
        fromNodeId: ownerNodeID,
        toNodeId: "",
        targetKind: "thermal_boundary",
        targetId: boundary.id,
        presentationKind: "adiabatic_stub",
        boundaryIds: [boundary.id],
        openingIds: [...(boundary.openingIds || [])],
        surfaceCount: 1,
        openingCount: (boundary.openingIds || []).length,
        physicalGrossArea: Number(boundary.physicalGrossArea) || 0,
        physicalOpaqueArea: Number(boundary.physicalOpaqueArea) || 0,
        physicalOpeningArea: Number(boundary.physicalOpeningArea) || 0,
        effectiveGrossArea: Number(boundary.effectiveGrossArea) || 0,
        effectiveOpaqueArea: Number(boundary.effectiveOpaqueArea) || 0,
        effectiveOpeningArea: Number(boundary.effectiveOpeningArea) || 0,
        opaqueUa: 0,
        openingUa: 0,
        totalUa: 0,
        hasUa: true,
        physicalOpaqueUa: 0,
        physicalOpeningUa: 0,
        physicalTotalUa: 0,
        hasPhysicalUa: true,
        orientations: boundary.orientation ? [boundary.orientation] : [],
        orientation: boundary.orientation || "",
        surfaceType: boundary.surfaceType || "",
        diagnosticIds: [...(boundary.diagnosticIds || [])],
        sourceAnchors: [...(boundary.sourceAnchors || [])],
      });
    }
  }
  const referencedNodeIDs = new Set(connections.flatMap((connection) => [connection.fromNodeId, connection.toNodeId]).filter(Boolean));
  return {
    ...model,
    nodes: (model.nodes || []).filter((node) => !isSharedAdiabaticNode(node) || referencedNodeIDs.has(node.id)),
    connections,
  };
}

function placeExternalNodes(external, positions, connections, viewport) {
  const buckets = new Map();
  const connectionsByNode = indexConnectionsByNode(connections);
  for (const node of external.sort(compareNodes)) {
    const side = externalSide(node, connectionsByNode.get(node.id) || []);
    if (!buckets.has(side)) buckets.set(side, []);
    buckets.get(side).push(node);
  }
  for (const [side, nodes] of buckets) {
    nodes.forEach((node, index) => {
      const ratio = (index + 1) / (nodes.length + 1);
      if (side === "top") positions[node.id] = { x: ratio * viewport.width, y: 42 };
      else if (side === "bottom") positions[node.id] = { x: ratio * viewport.width, y: viewport.height - 42 };
      else if (side === "left") positions[node.id] = { x: 74, y: ratio * viewport.height };
      else positions[node.id] = { x: viewport.width - 74, y: ratio * viewport.height };
    });
  }
}

function externalSide(node, connections) {
  const value = `${node?.orientation || ""} ${node?.kind || ""} ${node?.id || ""} ${node?.label || ""}`.toLowerCase();
  if (/ground|floor/.test(value)) return "bottom";
  if (/roof|sky|north/.test(value)) return "top";
  if (/west/.test(value)) return "left";
  if (/east/.test(value)) return "right";
  if (/south/.test(value)) return "bottom";
  const relatedOrientations = connections
    .flatMap((connection) => connection.orientations || [])
    .join(" ")
    .toLowerCase();
  if (/north/.test(relatedOrientations)) return "top";
  if (/south/.test(relatedOrientations)) return "bottom";
  if (/west/.test(relatedOrientations)) return "left";
  return "right";
}

function chooseThermalPorts(source, target, sourceNode, targetNode) {
  if (isExternalNode(targetNode)) {
    const targetSide = externalSide(targetNode, []);
    return { sourcePort: targetSide, targetPort: oppositePort(targetSide) };
  }
  if (isExternalNode(sourceNode)) {
    const sourceSide = externalSide(sourceNode, []);
    return { sourcePort: oppositePort(sourceSide), targetPort: sourceSide };
  }
  const dx = target.x - source.x;
  const dy = target.y - source.y;
  if (Math.abs(dx) >= Math.abs(dy)) {
    return dx >= 0
      ? { sourcePort: "right", targetPort: "left" }
      : { sourcePort: "left", targetPort: "right" };
  }
  return dy >= 0
    ? { sourcePort: "bottom", targetPort: "top" }
    : { sourcePort: "top", targetPort: "bottom" };
}

function portPoint(position, port, node) {
  const size = thermalNodeSize(node);
  if (port === "left") return { x: position.x - size.width / 2, y: position.y };
  if (port === "right") return { x: position.x + size.width / 2, y: position.y };
  if (port === "top") return { x: position.x, y: position.y - size.height / 2 };
  return { x: position.x, y: position.y + size.height / 2 };
}

function oppositePort(port) {
  return { left: "right", right: "left", top: "bottom", bottom: "top" }[port] || "left";
}

function resolveNodeCollisions(positions, nodes, viewport) {
  const placed = [];
  for (const node of [...nodes].sort(compareNodes)) {
    const position = positions[node.id];
    if (!position) continue;
    const size = thermalNodeSize(node);
    let attempt = 0;
    while (placed.some((other) => rectanglesOverlap(position, size, other.position, other.size)) && attempt < 80) {
      const ring = Math.floor(attempt / 8) + 1;
      const angle = (attempt % 8) * Math.PI / 4;
      position.x += Math.cos(angle) * ring * 16;
      position.y += Math.sin(angle) * ring * 12;
      position.x = Math.min(viewport.width - size.width / 2 - 8, Math.max(size.width / 2 + 8, position.x));
      position.y = Math.min(viewport.height - size.height / 2 - 8, Math.max(size.height / 2 + 8, position.y));
      attempt += 1;
    }
    placed.push({ position, size });
  }
}

function rectanglesOverlap(left, leftSize, right, rightSize) {
  return Math.abs(left.x - right.x) < (leftSize.width + rightSize.width) / 2 + 12
    && Math.abs(left.y - right.y) < (leftSize.height + rightSize.height) / 2 + 12;
}

function barycentricOrder(nodes, neighborsByNode, positions) {
  return [...nodes].sort((left, right) => {
    const leftValue = neighborBarycenter(left.id, neighborsByNode, positions);
    const rightValue = neighborBarycenter(right.id, neighborsByNode, positions);
    return leftValue - rightValue || compareNodes(left, right);
  });
}

function neighborBarycenter(nodeID, neighborsByNode, positions) {
  const neighbors = (neighborsByNode.get(nodeID) || [])
    .map((id) => positions[id]?.y)
    .filter(Number.isFinite);
  return neighbors.length ? neighbors.reduce((sum, value) => sum + value, 0) / neighbors.length : Number.MAX_SAFE_INTEGER;
}

function connectionNeighbors(connections) {
  const neighborsByNode = new Map();
  for (const connection of connections) {
    appendIndexedValue(neighborsByNode, connection.fromNodeId, connection.toNodeId);
    if (connection.toNodeId !== connection.fromNodeId) {
      appendIndexedValue(neighborsByNode, connection.toNodeId, connection.fromNodeId);
    }
  }
  return neighborsByNode;
}

function indexConnectionsByNode(connections) {
  const connectionsByNode = new Map();
  for (const connection of connections) {
    appendIndexedValue(connectionsByNode, connection.fromNodeId, connection);
    if (connection.toNodeId !== connection.fromNodeId) {
      appendIndexedValue(connectionsByNode, connection.toNodeId, connection);
    }
  }
  return connectionsByNode;
}

function appendIndexedValue(index, key, value) {
  if (!key) return;
  const values = index.get(key) || [];
  values.push(value);
  index.set(key, values);
}

function nodeDegrees(connections) {
  const values = new Map();
  for (const connection of connections) {
    values.set(connection.fromNodeId, (values.get(connection.fromNodeId) || 0) + 1);
    values.set(connection.toNodeId, (values.get(connection.toNodeId) || 0) + 1);
  }
  return values;
}

function isExternalNode(node) {
  if (!node) return false;
  return !["zone", "space"].includes(node.kind);
}

export function isThermalPointNode(node) {
  const value = `${node?.kind || ""} ${node?.id || ""}`.toLowerCase();
  return /(^|[\s:])outdoors?(?:_|\b)/.test(value) || node?.kind === "ground";
}

export function thermalNodeSize(node) {
  const diameter = THERMAL_ENDPOINT_RADIUS * 2;
  return isThermalPointNode(node)
    ? { width: diameter, height: diameter }
    : { width: THERMAL_NODE_WIDTH, height: THERMAL_NODE_HEIGHT };
}

function isAdiabaticConnection(connection) {
  return connection?.relationKind === "adiabatic_explicit" || connection?.relationKind === "adiabatic_self_reference";
}

function isSharedAdiabaticNode(node) {
  return node?.kind === "adiabatic" || node?.id === "thermal-environment:adiabatic";
}

function thermalConnectionSide(connection) {
  const value = `${connection?.orientation || ""} ${(connection?.orientations || []).join(" ")} ${connection?.surfaceType || ""}`.toLowerCase();
  if (/roof|ceiling|\bnorth\b|\bn\b/.test(value)) return "top";
  if (/floor|\bsouth\b|\bs\b/.test(value)) return "bottom";
  if (/\bwest\b|\bw\b/.test(value)) return "left";
  return "right";
}

function sideVector(side) {
  if (side === "left") return { x: -1, y: 0 };
  if (side === "top") return { x: 0, y: -1 };
  if (side === "bottom") return { x: 0, y: 1 };
  return { x: 1, y: 0 };
}

function alternatingLaneOffset(index) {
  if (!index) return 0;
  const magnitude = Math.ceil(index / 2);
  return index % 2 ? magnitude : -magnitude;
}

function normalizeRange(value, minimum, maximum, fallback) {
  if (!Number.isFinite(value) || maximum <= minimum) return fallback;
  return (value - minimum) / (maximum - minimum);
}

function sortedPairKey(left, right) {
  return [left, right].sort().join("|");
}

function compareNodes(left, right) {
  return String(left.id || "").localeCompare(String(right.id || ""));
}
