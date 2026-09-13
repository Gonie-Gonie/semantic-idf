import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";
import { renderHVACEquipmentIcon } from "./hvac-views.js";

const normalized = (value) => String(value || "").trim().toLowerCase();
const label = (key, fallback) => { const value = t(key); return !value || value === key ? fallback : value; };
const finite = (value) => typeof value === "number" && Number.isFinite(value);

// Only parsed ports, connectors and demand-path edges establish connectivity.
// Branch order and the set of available observations never invent an edge.
export function buildHVACInspectionTopology(loop = {}, observations = {}) {
  const vertices = new Map(), edges = new Map();
  const node = (name, side = "") => {
    if (!normalized(name)) return "";
    const id = `node:${normalized(name)}`;
    if (!vertices.has(id)) vertices.set(id, { id, kind: "node", name, side });
    return id;
  };
  const edge = (from, to, side = "", role = "") => {
    if (!from || !to || from === to) return;
    const id = `${from}\u0000${to}`;
    if (!edges.has(id)) edges.set(id, { from, to, side, ...(role ? { role } : {}) });
  };
  const component = (item, id, side) => {
    if (!item || !(item.objectName || item.name)) return "";
    vertices.set(id, { ...item, id, kind: "component", name: item.objectName || item.name, type: item.objectType || item.type || "", side });
    return id;
  };
  for (const [side, source] of [["supply", loop.supplySide], ["demand", loop.demandSide]]) {
    if (!source) continue;
    node(source.inletNode, side); node(source.outletNode, side);
    const branches = new Map();
    for (const [index, branch] of (source.branches || []).entries()) {
      const items = branch.components || [];
      const inlet = branch.inletNode || items[0]?.inletNode;
      const outlet = branch.outletNode || items.at(-1)?.outletNode;
      branches.set(normalized(branch.name), { inlet, outlet });
      node(inlet, side); node(outlet, side);
      for (const [position, item] of items.entries()) {
        const id = component(item, `component:${side}:${index}:${position}`, side);
        if (!id) continue;
        edge(node(item.inletNode, side), id, side);
        edge(id, node(item.outletNode, side), side);
      }
    }
    for (const connector of source.connectors || []) {
      const kind = normalized(connector.type);
      if (kind.includes("splitter")) {
        const inlet = branches.get(normalized(connector.inletBranchName))?.outlet;
        for (const name of connector.branchNames || []) edge(node(inlet, side), node(branches.get(normalized(name))?.inlet, side), side);
      }
      if (kind.includes("mixer")) {
        const outlet = branches.get(normalized(connector.outletBranchName))?.inlet;
        for (const name of connector.branchNames || []) edge(node(branches.get(normalized(name))?.outlet, side), node(outlet, side), side);
      }
    }
  }
  const demand = loop.demandGraph;
  for (const item of demand?.nodes || []) node(item.nodeName, "demand");
  const demandComponents = new Map();
  for (const path of [demand?.supplyPath, demand?.returnPath]) {
    for (const item of path?.components || []) {
      const key = `${normalized(item.objectType)}\u0000${normalized(item.objectName)}`;
      if (!demandComponents.has(key)) demandComponents.set(key, item);
    }
  }
  for (const [index, [key, item]] of [...demandComponents].entries()) {
    const id = component(item, `component:demand-path:${index}`, "demand");
    demandComponents.set(key, { ...item, id });
    for (const name of item.inletNodes || []) edge(node(name, "demand"), id, "demand");
    for (const name of item.outletNodes || []) edge(id, node(name, "demand"), "demand");
  }
  for (const item of demand?.edges || []) {
    const owner = demandComponents.get(`${normalized(item.objectType)}\u0000${normalized(item.objectName)}`);
    const from = node(item.fromNode, "demand"), to = node(item.toNode, "demand");
    // An explicit demand component already describes its own inlet/outlet path.
    if (owner?.id && owner.inletNodes?.some((name) => normalized(name) === normalized(item.fromNode)) && owner.outletNodes?.some((name) => normalized(name) === normalized(item.toNode))) continue;
    edge(from, to, "demand");
  }
  const equipmentMatch = (item, nameKey = "name", typeKey = "type", requireUnique = true) => {
    const matches = [...vertices.values()].filter((vertex) => vertex.kind === "component" && normalized(vertex.name) === normalized(item[nameKey]) && normalized(vertex.type) === normalized(item[typeKey]));
    return matches.length === 1 || !requireUnique && matches.length ? matches[0] : null;
  };
  const expandedParents = new Map();
  for (const item of observations.components || []) {
    let vertex = equipmentMatch(item, "name", "type", false);
    if (!vertex) {
      const parent = equipmentMatch(item, "parentComponentName", "parentComponentType");
      const id = `observed:${item.id || `${normalized(item.type)}:${normalized(item.name)}`}`;
      component({ objectName: item.name, objectType: item.type }, id, parent?.side || "");
      vertex = vertices.get(id);
      if (!vertex) continue;
      if (parent) {
        if (!expandedParents.has(parent.id)) expandedParents.set(parent.id, new Set());
        expandedParents.get(parent.id).add(id);
      }
      // Port names and roles come from the executed, typed component. In
      // particular, adjacent observations do not establish any connection.
      const ports = item.nodePorts?.length ? item.nodePorts : [
        ...(item.inletNodes || []).map((nodeName) => ({ nodeName, role: "inlet" })),
        ...(item.outletNodes || []).map((nodeName) => ({ nodeName, role: "outlet" })),
      ];
      for (const port of ports) {
        const role = normalized(port.role), point = node(port.nodeName, vertex.side);
        if (role.includes("inlet")) edge(point, id, vertex.side, role);
        if (role.includes("outlet")) edge(id, point, vertex.side, role);
      }
    }
  }
  for (const item of observations.nodes || []) node(item.name);
  for (const [parentID, children] of expandedParents) {
    const parent = vertices.get(parentID);
    const start = `node:${normalized(parent.inletNode)}`, finish = `node:${normalized(parent.outletNode)}`;
    if (!parent.inletNode || !parent.outletNode) continue;
    const visited = new Set([start]), pending = [start];
    while (pending.length) {
      const current = pending.shift();
      for (const item of edges.values()) {
        if (item.from !== current || visited.has(item.to)) continue;
        if (!children.has(item.from) && !children.has(item.to)) continue;
        visited.add(item.to); pending.push(item.to);
      }
    }
    // Expand a wrapper only when its typed children prove the full inlet to
    // outlet path. Retain the equipment's frame values as a separate card.
    if (visited.has(finish)) {
      for (const [key, item] of edges) if (item.from === parentID || item.to === parentID) edges.delete(key);
      parent.expanded = true;
    }
  }
  return { vertices: [...vertices.values()], edges: [...edges.values()] };
}

function layoutTopology(graph) {
  const connectedIDs = new Set(graph.edges.flatMap((item) => [item.from, item.to]));
  const connected = graph.vertices.filter((item) => connectedIDs.has(item.id));
  const detached = graph.vertices.filter((item) => !connectedIDs.has(item.id));
  const outgoing = new Map(connected.map((item) => [item.id, []]));
  const indegree = new Map(connected.map((item) => [item.id, 0]));
  for (const edge of graph.edges) {
    outgoing.get(edge.from)?.push(edge.to);
    indegree.set(edge.to, (indegree.get(edge.to) || 0) + 1);
  }
  const pending = new Set(connected.map((item) => item.id));
  const rank = new Map();
  // Break a closed cycle for positioning only; the original edge remains drawn.
  while (pending.size) {
    const ready = [...pending].filter((id) => !indegree.get(id));
    if (!ready.length) ready.push(pending.values().next().value);
    for (const id of ready) {
      pending.delete(id);
      if (!rank.has(id)) rank.set(id, 0);
      for (const next of outgoing.get(id) || []) {
        if (!pending.has(next)) continue;
        rank.set(next, Math.max(rank.get(next) || 0, rank.get(id) + 1));
        indegree.set(next, Math.max(0, (indegree.get(next) || 0) - 1));
      }
    }
  }
  const levels = [];
  for (const vertex of connected) (levels[rank.get(vertex.id) || 0] ||= []).push(vertex);
  const rows = Math.max(1, ...levels.map((items) => items.length));
  const width = Math.max(680, levels.length * 220 + 40), networkHeight = Math.max(240, rows * 190 + 50);
  const detachedColumns = Math.min(4, Math.floor((width - 40) / 220));
  const height = networkHeight + (detached.length ? 50 + Math.ceil(detached.length / detachedColumns) * 190 : 0);
  const positions = new Map();
  for (const [column, items] of levels.entries()) {
    for (const [row, item] of items.entries()) positions.set(item.id, { x: 130 + column * 220, y: 48 + row * 190 + (rows - items.length) * 95 });
  }
  for (const [index, item] of detached.entries()) positions.set(item.id, { x: 130 + index % detachedColumns * 220, y: networkHeight + 68 + Math.floor(index / detachedColumns) * 190 });
  return { width, height, positions, detachedTop: detached.length ? networkHeight : null };
}

function matchedObservation(vertex, observations) {
  const name = normalized(vertex.name);
  const exact = observations.filter((item) => normalized(item.name) === name && (vertex.kind === "node" || !item.type || normalized(item.type) === normalized(vertex.type)));
  return exact.length === 1 ? exact[0] : null;
}

function metricValue(metric) {
  if (metric.formattedValue != null) return String(metric.formattedValue);
  if (!finite(metric.value)) return "—";
  const digits = Math.abs(metric.value) > 0 && Math.abs(metric.value) < .01 ? 4 : 2;
  return `${metric.value.toLocaleString("en-US", { minimumFractionDigits: digits, maximumFractionDigits: digits })}${metric.unit ? ` ${metric.unit}` : ""}`;
}

function splitLabel(value, limit = 24) {
  const text = String(value || "");
  if (text.length <= limit) return [text];
  let split = text.lastIndexOf(" ", limit);
  if (split < limit / 2) split = limit;
  const remaining = text.slice(split).trim();
  return [text.slice(0, split), remaining.length > limit ? `${remaining.slice(0, limit - 1)}…` : remaining];
}

function renderVertex(vertex, point, observation, selected) {
  const isNode = vertex.kind === "node";
  const id = observation?.id || (isNode ? vertex.name : vertex.id);
  const metrics = observation?.metrics || [];
  const active = isNode && Boolean(observation && observation.active !== false);
  const status = ["on", "off"].includes(observation?.status) ? observation.status : "unknown";
  const stateLabel = status === "on" ? label("simulation.hvacStatusOn", "On") : status === "off" ? label("simulation.hvacStatusOff", "Off") : "";
  const metricLines = metrics.map((metric) => `${metric.label || metric.id}: ${metricValue(metric)}`);
  const title = [vertex.name, stateLabel, ...metricLines].filter(Boolean).join("\n");
  const lines = splitLabel(vertex.name);
  const nameY = isNode ? 28 : 36;
  const metricY = nameY + lines.length * 16 + 8;
  const cardHeight = Math.max(128, metricY + Math.min(4, metrics.length) * 17 + 10);
  const attr = isNode ? "data-hvac-inspect-node" : "data-hvac-inspect-component";
  return `<g class="hvac-inspect-vertex ${isNode ? "node" : "equipment"} ${active ? "measured" : ""} ${selected ? "selected" : ""} ${escapeHTML(vertex.side)}" ${attr}="${escapeHTML(id)}" data-hvac-inspect-topology-id="${escapeHTML(vertex.id)}" data-hvac-inspect-point-name="${escapeHTML(vertex.name)}" role="button" tabindex="0" aria-pressed="${Boolean(selected)}" aria-label="${escapeHTML(title)}" transform="translate(${point.x} ${point.y})">
    <title>${escapeHTML(title)}</title>
    <rect class="hvac-inspect-card" x="-97" y="-29" width="194" height="${cardHeight + 29}" rx="9"></rect>
    ${isNode ? `<circle class="hvac-inspect-node-ring" cx="0" cy="0" r="${active ? 13 : 9}"></circle><circle class="hvac-inspect-node-core" cx="0" cy="0" r="4"></circle>` : renderHVACEquipmentIcon(vertex, 0, 0)}
    ${stateLabel ? `<text class="hvac-inspect-state ${status}" x="85" y="-9" text-anchor="end">${escapeHTML(stateLabel)}</text>` : ""}
    ${lines.map((line, index) => `<text class="hvac-inspect-point-label" x="0" y="${nameY + index * 16}" text-anchor="middle" ${line.length > 26 ? `textLength="180" lengthAdjust="spacingAndGlyphs"` : ""}>${escapeHTML(line)}</text>`).join("")}
    ${metrics.slice(0, 4).map((metric, index) => `<text class="hvac-inspect-metric" x="0" y="${metricY + index * 17}" text-anchor="middle" data-hvac-inspect-metric="${escapeHTML(metric.id || metric.label)}"><tspan class="hvac-inspect-metric-label">${escapeHTML(metric.label || metric.id)} </tspan><tspan class="hvac-inspect-metric-value">${escapeHTML(metricValue(metric))}</tspan></text>`).join("")}
  </g>`;
}

export function renderHVACInspectionTopology({ loop, nodes = [], components = [], selectedNode = "", selectedComponent = "", zoom = 1 } = {}) {
  const graph = buildHVACInspectionTopology(loop || {}, { nodes, components });
  if (!loop || !graph.edges.length) return `<div class="hvac-inspect-topology-empty" data-hvac-inspect-topology-empty>${escapeHTML(label("simulation.hvacTopologyUnavailable", "Topology is not available for this result."))}</div>`;
  const { width, height, positions, detachedTop } = layoutTopology(graph);
  const scale = zoom === "fit" ? "fit" : [1, 1.5, 2].includes(Number(zoom)) ? Number(zoom) : 1;
  const markerId = `hvac-inspect-arrow-${String(loop.id || loop.name || "loop").replace(/[^a-zA-Z0-9_-]/g, "-")}`;
  const edges = graph.edges.map((edge) => {
    const from = positions.get(edge.from), to = positions.get(edge.to);
    const middle = (from.x + to.x) / 2;
    const d = to.x > from.x ? `M${from.x + 98},${from.y} C${middle},${from.y} ${middle},${to.y} ${to.x - 99},${to.y}` : `M${from.x},${from.y - 30} C${from.x},${from.y - 45} ${to.x},${to.y - 45} ${to.x},${to.y - 30}`;
    return `<path class="hvac-inspect-edge ${escapeHTML(edge.side)}" data-hvac-inspect-edge-from="${escapeHTML(edge.from)}" data-hvac-inspect-edge-to="${escapeHTML(edge.to)}"${edge.role ? ` data-hvac-inspect-port-role="${escapeHTML(edge.role)}"` : ""} d="${d}" marker-end="url(#${markerId})"></path>`;
  }).join("");
  const vertices = graph.vertices.map((vertex) => {
    const observation = matchedObservation(vertex, vertex.kind === "node" ? nodes : components);
    const target = vertex.kind === "node" ? selectedNode : selectedComponent;
    const selected = Boolean(observation?.selected || target && [observation?.id, vertex.id, vertex.name].some((value) => normalized(value) === normalized(target)));
    return renderVertex(vertex, positions.get(vertex.id), observation, selected);
  }).join("");
  return `<section class="hvac-inspect-topology" data-hvac-inspect-topology="${escapeHTML(loop.id || loop.name)}">
    <div class="hvac-inspect-topology-heading"><h4>${escapeHTML(label("simulation.hvacTopology", "Loop topology"))}</h4><label>${escapeHTML(label("simulation.hvacTopologyZoom", "Zoom"))} <select data-hvac-inspect-zoom aria-label="${escapeHTML(label("simulation.hvacTopologyZoom", "Zoom"))}">${[["fit", label("simulation.hvacTopologyFit", "Fit")], [1, "100%"], [1.5, "150%"], [2, "200%"]].map(([value, text]) => `<option value="${value}"${String(scale) === String(value) ? " selected" : ""}>${escapeHTML(text)}</option>`).join("")}</select></label></div>
    <div class="hvac-inspect-topology-viewport" tabindex="0" aria-label="${escapeHTML(loop.name || label("simulation.hvacTopology", "Loop topology"))}" data-hvac-inspect-topology-viewport>
      <svg class="hvac-inspect-topology-svg ${scale === "fit" ? "fit" : ""}" viewBox="0 0 ${width} ${height}" style="width:${scale === "fit" ? "100%" : `${Math.round(width * scale)}px`}" role="group" aria-label="${escapeHTML(loop.name || "HVAC loop")}"><defs><marker id="${markerId}" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 Z"></path></marker></defs>${edges}${detachedTop === null ? "" : `<text class="hvac-inspect-detached-label" x="32" y="${detachedTop + 19}">${escapeHTML(label("simulation.hvacTopologyOtherPoints", "Other equipment / points"))}</text>`}${vertices}</svg>
    </div>
  </section>`;
}
