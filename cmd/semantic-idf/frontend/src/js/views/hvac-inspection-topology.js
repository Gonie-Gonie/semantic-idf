import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";
import { buildHVACLoopDiagramLayout, renderHVACLoopDiagram } from "./hvac-views.js";

const normalized = (value) => String(value || "").trim().toLowerCase();
const label = (key, fallback) => { const value = t(key); return !value || value === key ? fallback : value; };
const finite = (value) => typeof value === "number" && Number.isFinite(value);
const metricRowHeight = 18;

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
    // outlet path. Retain the equipment's frame values as an annotation.
    if (visited.has(finish)) {
      for (const [key, item] of edges) if (item.from === parentID || item.to === parentID) edges.delete(key);
      parent.expanded = true;
    }
  }
  return { vertices: [...vertices.values()], edges: [...edges.values()] };
}

function metricValue(metric) {
  if (metric.formattedValue != null) return String(metric.formattedValue);
  if (!finite(metric.value)) return "—";
  const digits = Math.abs(metric.value) > 0 && Math.abs(metric.value) < .01 ? 4 : 2;
  const unit = metric.unit && !["—", "-", "1"].includes(metric.unit) ? ` ${metric.unit}` : "";
  return `${metric.value.toLocaleString("en-US", { minimumFractionDigits: digits, maximumFractionDigits: digits })}${unit}`;
}

function splitLabel(value, limit = 21) {
  let remaining = String(value || "").trim();
  const lines = [];
  while (remaining.length > limit) {
    if (lines.length === 1) {
      lines.push(`${remaining.slice(0, limit - 1)}…`);
      return lines;
    }
    let split = Math.max(remaining.lastIndexOf(" ", limit), remaining.lastIndexOf("_", limit), remaining.lastIndexOf("-", limit));
    if (split < limit / 2) split = limit;
    lines.push(remaining.slice(0, split));
    remaining = remaining.slice(split).trim();
  }
  if (remaining) lines.push(remaining);
  return lines;
}

function renderMetricSymbol(metric) {
  if (metric.id === "setpoint") return 'T<tspan baseline-shift="sub" font-size="75%">set</tspan>';
  return escapeHTML(({ temperature: "T", relativeHumidity: "RH", humidity: "w", flow: "ṁ", power: "P", cop: "COP", cooling: "Q̇c", heating: "Q̇h", heatTransfer: "Q̇", status: /part load/i.test(metric.label || "") ? "PLR" : "Run" })[metric.id] || metric.label || metric.id);
}

function observationRecords(loop, graph, layout, nodes, components) {
  const componentAnchors = layout.anchors.filter((anchor) => anchor.kind === "component");
  const exactEquipment = (name, type) => componentAnchors.filter((anchor) => normalized(anchor.name) === normalized(name) && normalized(anchor.type) === normalized(type));
  const sideForNode = (name) => {
    const known = graph.vertices.find((vertex) => vertex.kind === "node" && normalized(vertex.name) === normalized(name) && vertex.side);
    if (known) return known.side;
    const owners = componentAnchors.filter(({ component }) => [component.waterInletNode, component.waterOutletNode, ...(component.nodeUsages || []).map((port) => port.nodeName)].some((port) => normalized(port) === normalized(name)));
    return owners.length && owners.every((owner) => owner.side === owners[0].side) ? owners[0].side : "demand";
  };
  return [...nodes.map((item) => ({ item, kind: "node" })), ...components.map((item) => ({ item, kind: "component" }))].map(({ item, kind }) => {
    let anchor = null, parent = "";
    if (kind === "node") {
      anchor = layout.anchors.find((point) => point.kind === "node" && normalized(point.name) === normalized(item.name));
      if (!anchor) {
        // A named zone relationship must be present in the parsed demand graph.
        // Splitter list order and similar-looking names are not a relationship.
        const mapped = (loop.demandGraph?.nodes || []).filter((point) => normalized(point.nodeName) === normalized(item.name) && point.zoneName);
        const owners = new Set(mapped.map((point) => normalized(point.zoneName)));
        if (owners.size === 1) {
          const zone = layout.anchors.find((point) => point.kind === "zone" && normalized(point.name) === [...owners][0]);
          const role = mapped[0].role;
          if (zone && ["zone_inlet", "zone_return"].includes(role)) anchor = { ...zone, kind: "node", name: item.name, x: zone.x + (role === "zone_inlet" ? 42 : -42), role };
        }
      }
    } else {
      const exact = exactEquipment(item.name, item.type);
      if (exact.length === 1) anchor = exact[0];
      if (!anchor) {
        const owners = exactEquipment(item.parentComponentName, item.parentComponentType);
        if (owners.length === 1) { anchor = owners[0]; parent = anchor.name; }
      }
    }
    const metrics = (item.metrics || []).slice(0, 4);
    const nameLines = splitLabel(item.name);
    const parentLines = parent ? splitLabel(parent).map((line, index) => `${index ? "" : "↳ "}${line}`) : [];
    const status = kind === "component" ? ["on", "off"].includes(item.status) ? item.status : "unknown" : "";
    const side = anchor?.side || (kind === "node" ? sideForNode(item.name) : "demand");
    const rows = (side === "supply" ? layout.supplyLayout : layout.demandLayout).rows;
    // Bus equipment between parallel rows keeps its exact icon anchor while
    // sharing a reserved text band with one row, so labels cannot overlap.
    const annotationRowY = anchor ? rows.reduce((best, row) => Math.abs(row.y - anchor.y) < Math.abs(best - anchor.y) ? row.y : best, rows[0]?.y ?? anchor.y) : null;
    return { item, kind, anchor, annotationRowY, parent, side, nameLines, parentLines, metrics, status,
      height: (nameLines.length + parentLines.length) * 16 + metrics.length * metricRowHeight + (status ? 18 : 0) + 12 };
  });
}

const bandColumns = 5;
function reservedBands(records, layout) {
  const result = {};
  for (const side of ["supply", "demand"]) {
    const selected = records.filter((record) => record.side === side);
    const groups = new Map();
    for (const record of selected.filter((entry) => entry.anchor)) {
      const key = `${record.annotationRowY}:${record.kind}`;
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push(record);
    }
    const bandHeight = (kind) => Math.max(0, ...[...groups.values()].filter((group) => group[0].kind === kind).map((group) => Math.ceil(group.length / bandColumns) * (Math.max(...group.map((record) => record.height)) + 14)));
    const detached = selected.filter((record) => !record.anchor);
    const detachedRowHeight = Math.max(0, ...detached.map((record) => record.height)) + 14;
    const above = bandHeight("node"), below = bandHeight("component");
    const rowTopOffset = Math.max(46, above + 52), rowBottomPadding = Math.max(42, below + 52);
    result[side] = { reserveAnnotations: true, rowTopOffset, rowBottomPadding,
      rowGap: Math.max(64, rowTopOffset + rowBottomPadding),
      extraBottom: detached.length ? Math.ceil(detached.length / 4) * detachedRowHeight + 38 : 0,
      detachedRowHeight, above, below };
  }
  return result;
}

function positionAnnotations(records, layout, bands) {
  for (const [side, source] of [["supply", layout.supplyLayout], ["demand", layout.demandLayout]]) {
    const selected = records.filter((record) => record.side === side);
    const groups = new Map();
    for (const record of selected.filter((entry) => entry.anchor)) {
      const key = `${record.annotationRowY}:${record.kind}`;
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push(record);
    }
    for (const group of groups.values()) {
      group.sort((a, b) => a.anchor.x - b.anchor.x || a.item.name.localeCompare(b.item.name));
      const rowHeight = Math.max(...group.map((record) => record.height)) + 14;
      for (let offset = 0; offset < group.length; offset += bandColumns) {
        const row = group.slice(offset, offset + bandColumns);
        const inset = side === "demand" && row.length <= 4;
        const minX = inset ? 224 : 126, maxX = inset ? 896 : 994;
        const xs = row.map((record) => Math.max(minX, Math.min(maxX, record.anchor.x)));
        for (let index = 1; index < xs.length; index++) xs[index] = Math.max(xs[index], xs[index - 1] + 214);
        if (xs.at(-1) > maxX) {
          xs[xs.length - 1] = maxX;
          for (let index = xs.length - 2; index >= 0; index--) xs[index] = Math.min(xs[index], xs[index + 1] - 214);
        }
        row.forEach((record, index) => {
          record.x = xs[index] - 96;
          record.y = record.kind === "node" ? record.annotationRowY - bands[side].above + Math.floor(offset / bandColumns) * rowHeight - 4 : record.annotationRowY + 54 + Math.floor(offset / bandColumns) * rowHeight;
        });
      }
    }
    const detached = selected.filter((record) => !record.anchor);
    const startY = source.top + source.height - bands[side].extraBottom + 32;
    detached.forEach((record, index) => {
      record.x = 176 + index % 4 * 192;
      record.y = startY + Math.floor(index / 4) * bands[side].detachedRowHeight;
    });
  }
}

function renderObservation(record, selectedNode, selectedComponent) {
  const { item, kind, anchor, nameLines, parentLines, metrics, status, x, y, height } = record;
  const target = kind === "node" ? selectedNode : selectedComponent;
  const selected = Boolean(item.selected || target && [item.id, item.name].some((value) => normalized(value) === normalized(target)));
  const active = item.active !== false && metrics.some((metric) => finite(metric.value) || metric.formattedValue != null);
  const id = item.id || item.name;
  const attr = kind === "node" ? "data-hvac-inspect-node" : "data-hvac-inspect-component";
  const title = [item.name, record.parent ? `↳ ${record.parent}` : "", ...metrics.map((metric) => `${metric.label || metric.id}: ${metricValue(metric)}`)].filter(Boolean).join("\n");
  const statusLabel = status === "unknown" ? "—" : status ? label(status === "on" ? "simulation.hvacStatusOn" : "simulation.hvacStatusOff", status === "on" ? "On" : "Off") : "";
  const leaderStartY = kind === "node" ? y + height - 2 : y - 14;
  const leaderEndY = anchor ? anchor.y + (kind === "node" ? -13 : 23) : 0;
  return `<g class="hvac-inspect-vertex ${kind === "node" ? "node" : "equipment"} ${active ? "measured" : ""} ${selected ? "selected" : ""} ${escapeHTML(record.side)} ${anchor ? "anchored" : "unanchored"}" ${attr}="${escapeHTML(id)}" data-hvac-inspect-topology-id="${escapeHTML(kind === "node" ? `node:${normalized(item.name)}` : item.id || item.name)}" data-hvac-inspect-point-name="${escapeHTML(item.name)}" data-hvac-inspect-anchor-kind="${anchor ? record.parent ? "parent" : "exact" : "unavailable"}" role="button" tabindex="0" aria-pressed="${selected}" aria-label="${escapeHTML(title)}">
    <title>${escapeHTML(title)}</title>
    ${anchor ? `<path class="hvac-inspect-leader" d="M${x + 96},${leaderStartY} L${x + 96},${(leaderStartY + leaderEndY) / 2} L${anchor.x},${leaderEndY}"></path>${kind === "node" ? `<circle class="hvac-inspect-node-ring" cx="${anchor.x}" cy="${anchor.y}" r="${active ? 11 : 8}"></circle><circle class="hvac-inspect-node-core" cx="${anchor.x}" cy="${anchor.y}" r="3"></circle>` : `<circle class="hvac-inspect-equipment-ring" cx="${anchor.x}" cy="${anchor.y}" r="29"></circle>`}` : ""}
    <g class="hvac-inspect-annotation" transform="translate(${x} ${y})" data-hvac-inspect-annotation-x="${x}" data-hvac-inspect-annotation-y="${y}" data-hvac-inspect-annotation-width="192" data-hvac-inspect-annotation-height="${height}">
      ${nameLines.map((line, index) => `<text class="hvac-inspect-point-label" x="0" y="${index * 16}">${escapeHTML(line)}</text>`).join("")}
      ${parentLines.map((line, index) => `<text class="hvac-inspect-parent-label" x="0" y="${(nameLines.length + index) * 16}">${escapeHTML(line)}</text>`).join("")}
      ${status ? `<text class="hvac-inspect-state ${status}" x="0" y="${(nameLines.length + parentLines.length) * 16 + 4}">${escapeHTML(statusLabel)}</text>` : ""}
      ${metrics.map((metric, index) => `<text class="hvac-inspect-metric" x="0" y="${(nameLines.length + parentLines.length) * 16 + (status ? 18 : 0) + index * metricRowHeight + 4}" data-hvac-inspect-metric="${escapeHTML(metric.id || metric.label)}"><tspan class="hvac-inspect-metric-label" x="0">${renderMetricSymbol(metric)}</tspan><tspan class="hvac-inspect-metric-value" x="42">${escapeHTML(metricValue(metric))}</tspan></text>`).join("")}
    </g>
  </g>`;
}

export function renderHVACInspectionTopology({ loop, nodes = [], components = [], selectedNode = "", selectedComponent = "", zoom = 1 } = {}) {
  const graph = buildHVACInspectionTopology(loop || {}, { nodes, components });
  if (!loop || !graph.edges.length) return `<div class="hvac-inspect-topology-empty" data-hvac-inspect-topology-empty>${escapeHTML(label("simulation.hvacTopologyUnavailable", "Topology is not available for this result."))}</div>`;
  const initial = buildHVACLoopDiagramLayout(loop, { readOnly: true });
  const bands = reservedBands(observationRecords(loop, graph, initial, nodes, components), initial);
  const layout = buildHVACLoopDiagramLayout(loop, { readOnly: true, annotationBands: bands });
  const records = observationRecords(loop, graph, layout, nodes, components);
  positionAnnotations(records, layout, bands);
  const headings = [["supply", layout.supplyLayout], ["demand", layout.demandLayout]].filter(([side]) => bands[side].extraBottom).map(([side, source]) => `<text class="hvac-inspect-detached-label" x="176" y="${source.top + source.height - bands[side].extraBottom + 8}">${escapeHTML(label("simulation.hvacTopologyOtherPoints", "Other equipment / points"))}</text>`).join("");
  const overlay = headings + records.map((record) => renderObservation(record, selectedNode, selectedComponent)).join("");
  const scale = zoom === "fit" ? "fit" : [1, 1.5, 2].includes(Number(zoom)) ? Number(zoom) : 1;
  const diagram = renderHVACLoopDiagram(loop, { readOnly: true, layout, overlay, hideLegend: true, svgClass: `hvac-inspect-topology-svg${scale === "fit" ? " fit" : ""}` });
  return `<section class="hvac-inspect-topology" data-hvac-inspect-topology="${escapeHTML(loop.id || loop.name)}">
    <div class="hvac-inspect-topology-heading"><h4>${escapeHTML(label("simulation.hvacTopology", "Loop topology"))}</h4><label>${escapeHTML(label("simulation.hvacTopologyZoom", "Zoom"))} <select data-hvac-inspect-zoom aria-label="${escapeHTML(label("simulation.hvacTopologyZoom", "Zoom"))}">${[["fit", label("simulation.hvacTopologyFit", "Fit")], [1, "100%"], [1.5, "150%"], [2, "200%"]].map(([value, text]) => `<option value="${value}"${String(scale) === String(value) ? " selected" : ""}>${escapeHTML(text)}</option>`).join("")}</select></label></div>
    <div class="hvac-inspect-topology-viewport ${scale === "fit" ? "fit" : ""}" style="--hvac-inspect-width:${Math.round(layout.width * (scale === "fit" ? 1 : scale))}px" tabindex="0" aria-label="${escapeHTML(loop.name || label("simulation.hvacTopology", "Loop topology"))}" data-hvac-inspect-topology-viewport>${diagram}</div>
  </section>`;
}
