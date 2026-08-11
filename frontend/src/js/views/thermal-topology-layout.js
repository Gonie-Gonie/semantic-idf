import { normalizeThermalTopologyLayout, normalizeThermalTopologyScope } from "../state.js";
import { thermalTopologyObservationID } from "../thermal-topology-targets.js";

export const THERMAL_NODE_WIDTH = 148;
export const THERMAL_NODE_HEIGHT = 56;
const LAYOUT_PADDING = 92;
const STORY_LANE_GAP = 150;

export function thermalTopologyLayoutCacheKey(geometry, options = {}, viewport = {}) {
  const topology = geometry?.topology || {};
  const scope = normalizeThermalTopologyScope(options.scope);
  const selectionAffectsScope = scope === "selection" || scope === "neighbors";
  const topologyHash = topology.sourceModelHash || [
    topology.schema || "thermal-topology",
    ...(topology.nodes || []).map((node) => node.id),
    ...(topology.connections || []).map((connection) => connection.id),
  ].join(":");
  return [
    topologyHash,
    normalizeThermalTopologyLayout(options.layout),
    scope,
    options.metric || "topology",
    options.storyIndex ?? "all",
    selectionAffectsScope ? options.selectedEntityId || "" : "",
    selectionAffectsScope ? options.selectedEntityKind || "" : "",
    Boolean(options.showAirCoupling),
    Boolean(options.expandExternalTargets),
    Math.round((Number(viewport.width) || 900) / 50) * 50,
    Math.round((Number(viewport.height) || 600) / 50) * 50,
  ].join("|");
}

export function createThermalTopologyLayoutModel(geometry, options = {}) {
  const topology = geometry?.topology || {};
  const base = {
    schema: topology.schema || "",
    layout: normalizeThermalTopologyLayout(options.layout),
    scope: normalizeThermalTopologyScope(options.scope),
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
  const scoped = applyThermalTopologyScope(base, options);
  const expanded = options.expandExternalTargets ? expandExternalTargets(scoped) : scoped;
  return {
    ...expanded,
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
  const parallelCounts = new Map();
  const edges = model.connections.map((connection) => {
    const pairKey = sortedPairKey(connection.fromNodeId, connection.toNodeId);
    const lane = parallelCounts.get(pairKey) || 0;
    parallelCounts.set(pairKey, lane + 1);
    return {
      ...connection,
      route: routeThermalEdge(
        connection,
        positions[connection.fromNodeId],
        positions[connection.toNodeId],
        lane,
        model.nodes,
      ),
    };
  }).filter((edge) => edge.route);
  return { width, height, positions, edges };
}

export function computeSpatialLayout(model, viewport = {}) {
  const width = Math.max(360, Number(viewport.width) || 900);
  const height = Math.max(280, Number(viewport.height) || 600);
  const internal = model.nodes.filter((node) => !isExternalNode(node));
  const external = model.nodes.filter(isExternalNode);
  const stories = [...new Set(internal.map((node) => Number(node.storyIndex) || 0))].sort((a, b) => a - b);
  const centroids = internal.map((node) => node.centroid || {}).filter((point) => Number.isFinite(Number(point.x)) && Number.isFinite(Number(point.y)));
  const xValues = centroids.map((point) => Number(point.x));
  const yValues = centroids.map((point) => Number(point.y));
  const minX = Math.min(...xValues, 0);
  const maxX = Math.max(...xValues, 1);
  const minY = Math.min(...yValues, 0);
  const maxY = Math.max(...yValues, 1);
  const laneHeight = model.scope === "story" || stories.length <= 1
    ? height - LAYOUT_PADDING * 2
    : Math.max(STORY_LANE_GAP, (height - LAYOUT_PADDING * 2) / Math.max(stories.length, 1));
  const positions = {};
  internal
    .sort(compareNodes)
    .forEach((node, index) => {
      const storyLane = Math.max(0, stories.indexOf(Number(node.storyIndex) || 0));
      const centroid = node.centroid || {};
      const normalizedX = normalizeRange(Number(centroid.x), minX, maxX, (index + 1) / (internal.length + 1));
      const normalizedY = normalizeRange(Number(centroid.y), minY, maxY, ((index * 7) % Math.max(1, internal.length)) / Math.max(1, internal.length));
      const yStart = model.scope === "story" || stories.length <= 1 ? LAYOUT_PADDING : LAYOUT_PADDING + storyLane * laneHeight;
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
    const ordered = barycentricOrder(nodes, model.connections, positions);
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
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const sourceNode = byID.get(connection.fromNodeId);
  const targetNode = byID.get(connection.toNodeId);
  const ports = chooseThermalPorts(source, target, sourceNode, targetNode);
  const start = portPoint(source, ports.sourcePort);
  const end = portPoint(target, ports.targetPort);
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

export function applyThermalTopologyScope(model, options = {}) {
  const scope = normalizeThermalTopologyScope(options.scope);
  if (scope === "building") {
    return model;
  }
  const connections = model.connections || [];
  const selectedIDs = selectedNodeIDs(model, options.selectedEntityId);
  let included = new Set();
  if (scope === "story") {
    const story = options.storyIndex;
    for (const node of model.nodes) {
      if (!isExternalNode(node) && (story === "all" || Number(node.storyIndex) === Number(story))) {
        included.add(node.id);
      }
    }
    const storyNodes = new Set(included);
    for (const connection of connections) {
      if (storyNodes.has(connection.fromNodeId) || storyNodes.has(connection.toNodeId)) {
        included.add(connection.fromNodeId);
        included.add(connection.toNodeId);
      }
    }
  } else {
    included = new Set(selectedIDs);
    const frontier = new Set(included);
    for (const connection of connections) {
      if (frontier.has(connection.fromNodeId) || frontier.has(connection.toNodeId)) {
        included.add(connection.fromNodeId);
        included.add(connection.toNodeId);
      }
    }
  }
  selectedIDs.forEach((id) => included.add(id));
  const scopedConnections = connections.filter((connection) => included.has(connection.fromNodeId) && included.has(connection.toNodeId));
  return {
    ...model,
    nodes: model.nodes.filter((node) => included.has(node.id)),
    connections: scopedConnections,
  };
}

function selectedNodeIDs(model, selectedEntityId) {
  const selected = String(selectedEntityId || "");
  if (!selected) return [];
  if (model.nodes.some((node) => node.id === selected)) return [selected];
  const connection = model.connections.find((item) => item.id === selected);
  if (connection) return [connection.fromNodeId, connection.toNodeId];
  const boundary = model.boundaries.find((item) => item.id === selected || item.surfaceId === selected || item.surfaceEntityId === selected);
  if (boundary) return [boundary.ownerSpaceId || boundary.ownerZoneId, boundary.targetId].filter(Boolean);
  const opening = (model.allOpenings || []).find((item) => item.id === selected || item.windowId === selected || item.entityId === selected);
  if (opening) {
    const base = model.boundaries.find((item) => item.surfaceId === opening.baseSurfaceId || item.id === opening.baseSurfaceId);
    if (base) return [base.ownerSpaceId || base.ownerZoneId, base.targetId].filter(Boolean);
  }
  return [];
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

function expandExternalTargets(model) {
  const nodeByID = new Map(model.nodes.map((node) => [node.id, node]));
  const nodes = [...model.nodes];
  const connections = [];
  for (const connection of model.connections) {
    const externalAtFrom = isExternalNode(nodeByID.get(connection.fromNodeId));
    const externalAtTo = isExternalNode(nodeByID.get(connection.toNodeId));
    const orientations = [...new Set(connection.orientations || [])].filter(Boolean).sort();
    if ((!externalAtFrom && !externalAtTo) || orientations.length <= 1) {
      connections.push(connection);
      continue;
    }
    const externalID = externalAtFrom ? connection.fromNodeId : connection.toNodeId;
    const externalNode = nodeByID.get(externalID);
    for (const orientation of orientations) {
      const cloneID = `${externalID}:${String(orientation).toLowerCase()}`;
      if (!nodeByID.has(cloneID)) {
        const clone = { ...externalNode, id: cloneID, sourceId: externalID, label: `${externalNode.label} · ${orientation}`, orientation };
        nodes.push(clone);
        nodeByID.set(cloneID, clone);
      }
      connections.push({
        ...connection,
        id: `${connection.id}:${String(orientation).toLowerCase()}`,
        fromNodeId: externalAtFrom ? cloneID : connection.fromNodeId,
        toNodeId: externalAtTo ? cloneID : connection.toNodeId,
        orientations: [orientation],
      });
    }
  }
  return { ...model, nodes, connections };
}

function placeExternalNodes(external, positions, connections, viewport) {
  const buckets = new Map();
  for (const node of external.sort(compareNodes)) {
    const side = externalSide(node, connections);
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
    .filter((connection) => connection.fromNodeId === node.id || connection.toNodeId === node.id)
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

function portPoint(position, port) {
  if (port === "left") return { x: position.x - THERMAL_NODE_WIDTH / 2, y: position.y };
  if (port === "right") return { x: position.x + THERMAL_NODE_WIDTH / 2, y: position.y };
  if (port === "top") return { x: position.x, y: position.y - THERMAL_NODE_HEIGHT / 2 };
  return { x: position.x, y: position.y + THERMAL_NODE_HEIGHT / 2 };
}

function oppositePort(port) {
  return { left: "right", right: "left", top: "bottom", bottom: "top" }[port] || "left";
}

function resolveNodeCollisions(positions, nodes, viewport) {
  const placed = [];
  for (const node of [...nodes].sort(compareNodes)) {
    const position = positions[node.id];
    if (!position) continue;
    let attempt = 0;
    while (placed.some((other) => rectanglesOverlap(position, other)) && attempt < 80) {
      const ring = Math.floor(attempt / 8) + 1;
      const angle = (attempt % 8) * Math.PI / 4;
      position.x += Math.cos(angle) * ring * 16;
      position.y += Math.sin(angle) * ring * 12;
      position.x = Math.min(viewport.width - THERMAL_NODE_WIDTH / 2 - 8, Math.max(THERMAL_NODE_WIDTH / 2 + 8, position.x));
      position.y = Math.min(viewport.height - THERMAL_NODE_HEIGHT / 2 - 8, Math.max(THERMAL_NODE_HEIGHT / 2 + 8, position.y));
      attempt += 1;
    }
    placed.push(position);
  }
}

function rectanglesOverlap(left, right) {
  return Math.abs(left.x - right.x) < THERMAL_NODE_WIDTH + 12 && Math.abs(left.y - right.y) < THERMAL_NODE_HEIGHT + 12;
}

function barycentricOrder(nodes, connections, positions) {
  return [...nodes].sort((left, right) => {
    const leftValue = neighborBarycenter(left.id, connections, positions);
    const rightValue = neighborBarycenter(right.id, connections, positions);
    return leftValue - rightValue || compareNodes(left, right);
  });
}

function neighborBarycenter(nodeID, connections, positions) {
  const neighbors = connections
    .filter((connection) => connection.fromNodeId === nodeID || connection.toNodeId === nodeID)
    .map((connection) => connection.fromNodeId === nodeID ? connection.toNodeId : connection.fromNodeId)
    .map((id) => positions[id]?.y)
    .filter(Number.isFinite);
  return neighbors.length ? neighbors.reduce((sum, value) => sum + value, 0) / neighbors.length : Number.MAX_SAFE_INTEGER;
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
