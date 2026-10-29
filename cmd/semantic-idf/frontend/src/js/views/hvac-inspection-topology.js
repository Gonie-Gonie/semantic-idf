import { escapeHTML } from "../state.js";
import { t, getLanguage } from "../i18n.js";
import { renderHVACLoopDiagram } from "./hvac-views.js";
import { planHVACInspectionLayout } from "./hvac-inspection-layout.js";
import { hvacSetpointMode, hvacSetpointComparison } from "../hvac-setpoint.js";

const normalized = (value) => String(value || "").trim().toLowerCase();
const label = (key, fallback) => { const value = t(key); return !value || value === key ? fallback : value; };
const finite = (value) => typeof value === "number" && Number.isFinite(value);
const metricRowHeight = 18;
const layoutCache = new WeakMap();
let layoutVersion = 0;
let measureContext;
const diagramWidth = 1480;

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
      // A reported SQL object is not proof of loop membership. Only an
      // unambiguous typed parent in this circuit can introduce a child.
      if (!parent) continue;
      const id = `observed:${item.id || `${normalized(item.type)}:${normalized(item.name)}`}`;
      component({ objectName: item.name, objectType: item.type }, id, parent.side);
      vertex = vertices.get(id);
      if (!vertex) continue;
      if (!expandedParents.has(parent.id)) expandedParents.set(parent.id, new Set());
      expandedParents.get(parent.id).add(id);
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

function renderMetricSymbol(metric) {
  if (metric.id === "setpoint") return 'T<tspan baseline-shift="sub" font-size="75%">set</tspan>';
  const quantity = { part_load_ratio: "PLR", cycling_ratio: "Cyc", runtime_fraction: "RTF", operating_status: "Op" }[metric.quantity];
  return escapeHTML(quantity || ({ temperature: "T", relativeHumidity: "RH", humidity: "w", flow: "ṁ", power: "P", cop: "COP", cooling: "Q̇c", heating: "Q̇h", heatTransfer: "Q̇", status: "Op" })[metric.id] || metric.label || metric.id);
}

function renderMetricValue(metric) {
  if (!metric.setpoint) return escapeHTML(metricValue(metric));
  const number = (value) => value.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  return `<tspan class="hvac-inspect-setpoint ${metric.setpoint.state}" data-hvac-setpoint-state="${metric.setpoint.state}">${number(metric.value)} ${escapeHTML(metric.setpoint.operator)}<tspan baseline-shift="sub" font-size="75%">set</tspan> ${number(metric.setpoint.value)}</tspan> ${escapeHTML(metric.unit || "")}`;
}

function metricDescription(metric) {
  const title = metric.label || metric.id;
  const source = metric.name && normalized(metric.name) !== normalized(title) ? ` (${metric.name})` : "";
  return `${title}${source}: ${metricValue(metric)}`;
}

function fontKey() {
  return typeof document === "undefined" ? "sans-serif" : `${getComputedStyle(document.body).fontFamily}|${document.fonts?.status || "loaded"}`;
}

function textWidth(value, size = 13, weight = 600) {
  if (typeof document !== "undefined") {
    measureContext ||= document.createElement("canvas").getContext("2d");
    if (measureContext) {
      measureContext.font = `${weight} ${size}px ${getComputedStyle(document.body).fontFamily}`;
      return measureContext.measureText(String(value)).width;
    }
  }
  return String(value).length * size * .65;
}

function wrapName(value) {
  const lines = []; let line = "";
  for (const word of String(value || "").split(/(\s+|_)/).filter(Boolean)) {
    if (line && textWidth(line + word, 12, 400) > 220) { lines.push(line.trim()); line = ""; }
    for (const char of word) {
      if (line && textWidth(line + char, 12, 400) > 220) { lines.push(line.trim()); line = ""; }
      line += char;
    }
  }
  if (line.trim()) lines.push(line.trim());
  return lines;
}

function valueBudget(metric) {
  const range = metric.valueRange;
  const numbers = range ? [range.min, range.max].filter(finite) : [-888888.8888, 888888.8888];
  const samples = numbers.map((value) => metricValue({ ...metric, value, formattedValue: undefined }));
  if (metric.formattedValue != null) samples.push(String(metric.formattedValue));
  return Math.max(0, ...samples.map((value) => textWidth(value)));
}

function recordSize(record, setpoint) {
  const { metrics, nameLines, status } = record;
  let width = Math.max(104, ...nameLines.map((line) => textWidth(line, 12, 400)));
  for (const metric of metrics) {
    let measured = valueBudget(metric);
    if (metric.id === "temperature" && setpoint && (!setpoint.valueRange || finite(setpoint.valueRange.min))) {
      measured += valueBudget({ ...setpoint, unit: "" }) + textWidth(" >set ") + 8;
    }
    width = Math.max(width, 32 + measured);
  }
  return { width: Math.ceil(width + 28), height: 24 + nameLines.length * 16 + metrics.length * metricRowHeight + (status ? 18 : 0) };
}

function uniqueNodes(nodes) {
  const unique = new Map();
  for (const item of nodes) {
    const key = normalized(item.name);
    if (!key) continue;
    if (!unique.has(key)) { unique.set(key, { ...item, aliases: [item.id, item.name].filter(Boolean), metrics: [...(item.metrics || [])] }); continue; }
    const prior = unique.get(key);
    prior.aliases.push(...[item.id, item.name].filter(Boolean));
    prior.selected ||= item.selected;
    prior.active = prior.active !== false || item.active !== false;
    for (const metric of item.metrics || []) {
      const index = prior.metrics.findIndex((other) => other.id === metric.id && other.unit === metric.unit);
      if (index < 0) prior.metrics.push(metric);
      else if (!Object.is(prior.metrics[index].value, metric.value)) prior.metrics[index] = { ...prior.metrics[index], value: null, formattedValue: undefined };
    }
  }
  return [...unique.values()];
}

function observationRecords(loop, layout, nodes, components, sizing = true) {
  const componentAnchors = layout.anchors.filter((anchor) => anchor.kind === "component");
  const exactEquipment = (name, type) => componentAnchors.filter((anchor) => normalized(anchor.name) === normalized(name) && normalized(anchor.type) === normalized(type));
  return [...uniqueNodes(nodes).map((item) => ({ item, kind: "node" })), ...components.map((item) => ({ item, kind: "component" }))].map(({ item, kind }) => {
    let anchor = null, parent = "";
    if (kind === "node") {
      const anchors = layout.anchors.filter((point) => point.kind === "node" && normalized(point.name) === normalized(item.name));
      // A boundary anchor remains the canonical point when the same physical
      // node is also a component port. Never deduplicate by numeric values.
      anchor = anchors.find((point) => point.ownerKind === "loop-side") || anchors[0];
      if (!anchor) {
        const mapped = (loop.demandGraph?.nodes || []).filter((point) => normalized(point.nodeName) === normalized(item.name) && point.zoneName && ["zone_inlet", "zone_return"].includes(point.role));
        const owners = new Set(mapped.map((point) => normalized(point.zoneName)));
        if (owners.size === 1 && new Set(mapped.map((point) => point.role)).size === 1) {
          const zone = layout.anchors.find((point) => point.kind === "zone" && normalized(point.name) === [...owners][0]);
          if (zone) anchor = { ...zone, kind: "node", name: item.name, x: zone.x + (mapped[0].role === "zone_inlet" ? 42 : -42), role: mapped[0].role, portRole: mapped[0].role, ownerId: zone.ownerId, ownerX: zone.x, ownerY: zone.y };
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
    if (!anchor) return null;
    const setpoint = (item.metrics || []).find((metric) => metric.id === "setpoint");
    const metrics = (item.metrics || []).filter((metric) => kind !== "node" || metric.id !== "setpoint").slice(0, 4).map((metric) => metric.id === "temperature" && setpoint
      ? { ...metric, setpoint: hvacSetpointComparison(metric.value, setpoint.value, hvacSetpointMode(loop, item.name, components)) } : metric);
    if (kind === "node") metrics.sort((a, b) => ["temperature", "flow", "relativeHumidity", "humidity"].indexOf(a.id) - ["temperature", "flow", "relativeHumidity", "humidity"].indexOf(b.id));
    const nameLines = kind === "node" || !sizing ? [] : wrapName(item.name);
    const status = kind === "component" ? ["on", "off"].includes(item.status) ? item.status : "unknown" : "";
    const key = kind === "node" ? `node:${normalized(item.name)}` : `component:${item.id || `${normalized(item.type)}:${normalized(item.name)}`}`;
    const record = { key, item, kind, anchor, parent, side: anchor.side, nameLines, metrics, status };
    return sizing ? { ...record, ...recordSize(record, setpoint) } : record;
  }).filter(Boolean);
}

function shapeKey(nodes, components) {
  return JSON.stringify([getLanguage(), fontKey(), [...uniqueNodes(nodes), ...components].map((item) => [item.id, item.name, item.type, item.parentComponentName, item.parentComponentType,
    (item.metrics || []).map((metric) => [metric.id, metric.quantity, metric.name, metric.label, metric.unit, metric.valueRange])])]);
}

export function buildHVACInspectionLayout({ loop, nodes = [], components = [] } = {}) {
  const graph = buildHVACInspectionTopology(loop || {}, { components });
  if (!loop || !graph.edges.length) return { layout: null, records: [], graph };
  const members = new Set(graph.vertices.filter((vertex) => vertex.kind === "component").map((item) => `${normalized(item.type)}\u0000${normalized(item.name)}`));
  const circuitComponents = components.filter((item) => members.has(`${normalized(item.type)}\u0000${normalized(item.name)}`));
  const key = shapeKey(nodes, circuitComponents);
  if (!layoutCache.has(loop)) layoutCache.set(loop, new Map());
  const entries = layoutCache.get(loop);
  let cached = entries.get(key);
  if (!cached) {
    cached = { ...planHVACInspectionLayout(loop, (layout) => observationRecords(loop, layout, nodes, circuitComponents), { width: diagramWidth }), version: ++layoutVersion };
    entries.set(key, cached);
    if (entries.size > 4) entries.delete(entries.keys().next().value);
  }
  const fresh = observationRecords(loop, cached.layout, nodes, circuitComponents, false);
  const geometry = new Map(cached.records.map((record) => [record.key, record]));
  return { ...cached, graph, records: fresh.map((record) => {
    const placed = geometry.get(record.key);
    return { ...record, x: placed.x, y: placed.y, width: placed.width, height: placed.height, nameLines: placed.nameLines, leader: placed.leader };
  }) };
}

function renderObservation(record, selectedNode, selectedComponent) {
  const { item, kind, anchor, nameLines, metrics, status, x, y, width, height } = record;
  const target = kind === "node" ? selectedNode : selectedComponent;
  const selected = Boolean(item.selected || target && [item.id, item.name, ...(item.aliases || [])].some((value) => normalized(value) === normalized(target)));
  const active = item.active !== false && metrics.some((metric) => finite(metric.value) || metric.formattedValue != null);
  const id = item.id || item.name;
  const attr = kind === "node" ? "data-hvac-inspect-node" : "data-hvac-inspect-component";
  const title = [item.name, record.parent ? `↳ ${record.parent}` : "", ...metrics.map(metricDescription)].filter(Boolean).join("\n");
  const statusLabel = status === "unknown" ? "—" : status ? label(status === "on" ? "simulation.hvacStatusOn" : "simulation.hvacStatusOff", status === "on" ? "On" : "Off") : "";
  const leaderPath = (record.leader || []).map((point, index) => `${index ? "L" : "M"}${point.x},${point.y}`).join(" ");
  return `<g class="hvac-inspect-vertex ${kind === "node" ? "node" : "equipment"} ${active ? "measured" : ""} ${selected ? "selected" : ""} ${escapeHTML(record.side)} anchored" ${attr}="${escapeHTML(id)}" data-hvac-inspect-topology-id="${escapeHTML(record.key)}" data-hvac-inspect-owner="${escapeHTML(anchor.ownerId || "")}" data-hvac-inspect-port-role="${escapeHTML(anchor.portRole || anchor.role || "")}" data-hvac-inspect-point-name="${escapeHTML(item.name)}" data-hvac-inspect-anchor-kind="${record.parent ? "parent" : "exact"}" role="button" tabindex="0" aria-pressed="${selected}" aria-label="${escapeHTML(title)}">
    <title>${escapeHTML(title)}</title>
    <path class="hvac-inspect-leader" d="${leaderPath}"></path>${kind === "node" ? `<circle class="hvac-inspect-node-ring" cx="${anchor.x}" cy="${anchor.y}" r="11"></circle><circle class="hvac-inspect-node-core" cx="${anchor.x}" cy="${anchor.y}" r="3"></circle>` : `<circle class="hvac-inspect-equipment-ring" cx="${anchor.x}" cy="${anchor.y}" r="29"></circle>`}
    <g class="hvac-inspect-annotation" transform="translate(${x} ${y})" data-hvac-inspect-annotation-x="${x}" data-hvac-inspect-annotation-y="${y}" data-hvac-inspect-annotation-width="${width}" data-hvac-inspect-annotation-height="${height}">
      <rect class="hvac-inspect-info-box" x="0" y="0" width="${width}" height="${height}" rx="7"></rect>
      <g class="hvac-inspect-info-content" transform="translate(12 20)">
      ${nameLines.map((line, index) => `<text class="hvac-inspect-point-label" x="0" y="${index * 16}">${escapeHTML(line)}</text>`).join("")}
      ${status ? `<text class="hvac-inspect-state ${status}" x="0" y="${nameLines.length * 16 + 4}">${escapeHTML(statusLabel)}</text>` : ""}
      ${metrics.map((metric, index) => `<text class="hvac-inspect-metric" x="0" y="${nameLines.length * 16 + (status ? 18 : 0) + index * metricRowHeight + 4}" data-hvac-inspect-metric="${escapeHTML(metric.id || metric.label)}"${metric.quantity ? ` data-hvac-inspect-metric-quantity="${escapeHTML(metric.quantity)}"` : ""}${metric.name ? ` data-hvac-inspect-metric-name="${escapeHTML(metric.name)}"` : ""}><tspan class="hvac-inspect-metric-label" x="0">${renderMetricSymbol(metric)}</tspan><tspan class="hvac-inspect-metric-value" x="32">${renderMetricValue(metric)}</tspan></text>`).join("")}
      </g>
    </g>
  </g>`;
}

export function renderHVACInspectionTopology({ loop, nodes = [], components = [], selectedNode = "", selectedComponent = "", operatingProfiles = [], selectedProfile = "" } = {}) {
  const { graph, layout, records, version } = buildHVACInspectionLayout({ loop, nodes, components });
  if (!layout || !graph.edges.length) return `<div class="hvac-inspect-topology-empty" data-hvac-inspect-topology-empty>${escapeHTML(label("simulation.hvacTopologyUnavailable", "Topology is not available for this result."))}</div>`;
  const equipmentTargets = layout.anchors.filter((anchor) => anchor.kind === "component").map((anchor) => {
    const matches = operatingProfiles.filter((profile) => profile.definition.loopId === loop.id && profile.source.objectIndex === anchor.component?.objectIndex
      && normalized(profile.source.objectType) === normalized(anchor.type) && normalized(profile.source.objectName) === normalized(anchor.name));
    if (matches.length !== 1) return "";
    const profile = matches[0], selected = profile.id === selectedProfile;
    return `<circle class="hvac-inspect-equipment-target${selected ? " selected" : ""}" data-hvac-inspect-native-component="${escapeHTML(profile.source.objectIndex)}" cx="${anchor.x}" cy="${anchor.y}" r="27" role="button" tabindex="0" aria-pressed="${selected}" aria-label="${escapeHTML(anchor.name)}"><title>${escapeHTML(anchor.name)}</title></circle>`;
  }).join("");
  const overlay = equipmentTargets + records.map((record) => renderObservation(record, selectedNode, selectedComponent)).join("");
  const diagram = renderHVACLoopDiagram(loop, { readOnly: true, layout, overlay, hideLegend: true, svgClass: "hvac-inspect-topology-svg" });
  return `<section class="hvac-inspect-topology" data-hvac-inspect-layout-version="${version}" data-hvac-inspect-topology="${escapeHTML(loop.id || loop.name)}">
    <div class="hvac-inspect-topology-viewport" style="--hvac-inspect-width:${layout.width}px" tabindex="0" aria-label="${escapeHTML(loop.name || label("simulation.hvacTopology", "Loop topology"))}" data-hvac-inspect-topology-viewport>${diagram}</div>
  </section>`;
}

// Keep resolved geometry and focus/scroll intact when only the frame or
// selected point changes. A changed structure/font/metric shape asks the
// owning view for a full render through its normal presentation path.
export function updateHVACInspectionTopology(container, options = {}) {
  const section = container.querySelector("[data-hvac-inspect-layout-version]");
  if (!section || !options.loop) return false;
  const plan = buildHVACInspectionLayout(options);
  if (!plan.layout || Number(section.dataset.hvacInspectLayoutVersion) !== plan.version) return false;
  const expectedTargets = plan.layout.anchors.filter((anchor) => anchor.kind === "component").flatMap((anchor) => {
    const matches = (options.operatingProfiles || []).filter((profile) => profile.definition.loopId === options.loop.id && profile.source.objectIndex === anchor.component?.objectIndex
      && normalized(profile.source.objectType) === normalized(anchor.type) && normalized(profile.source.objectName) === normalized(anchor.name));
    return matches.length === 1 ? [String(matches[0].source.objectIndex)] : [];
  }).sort();
  const targets = [...section.querySelectorAll("[data-hvac-inspect-native-component]")];
  if (JSON.stringify(expectedTargets) !== JSON.stringify(targets.map((target) => target.dataset.hvacInspectNativeComponent).sort())) return false;
  const groups = new Map([...section.querySelectorAll("[data-hvac-inspect-topology-id]")].map((group) => [group.dataset.hvacInspectTopologyId, group]));
  if (groups.size !== plan.records.length) return false;
  for (const record of plan.records) {
    const group = groups.get(record.key);
    if (!group) return false;
    const item = record.item, target = record.kind === "node" ? options.selectedNode : options.selectedComponent;
    const selected = Boolean(item.selected || target && [item.id, item.name, ...(item.aliases || [])].some((value) => normalized(value) === normalized(target)));
    const active = item.active !== false && record.metrics.some((metric) => finite(metric.value) || metric.formattedValue != null);
    group.classList.toggle("selected", selected); group.classList.toggle("measured", active);
    group.setAttribute("aria-pressed", String(selected));
    const title = [item.name, record.parent ? `\u21b3 ${record.parent}` : "", ...record.metrics.map(metricDescription)].filter(Boolean).join("\n");
    group.setAttribute("aria-label", title); group.querySelector("title").textContent = title;
    const rows = [...group.querySelectorAll("[data-hvac-inspect-metric]")];
    record.metrics.forEach((metric, index) => { rows[index].querySelector(".hvac-inspect-metric-value").innerHTML = renderMetricValue(metric); });
    const status = group.querySelector(".hvac-inspect-state");
    if (status) {
      status.setAttribute("class", `hvac-inspect-state ${record.status}`);
      status.textContent = record.status === "unknown" ? "\u2014" : label(record.status === "on" ? "simulation.hvacStatusOn" : "simulation.hvacStatusOff", record.status === "on" ? "On" : "Off");
    }
  }
  for (const target of section.querySelectorAll("[data-hvac-inspect-native-component]")) {
    const selected = (options.operatingProfiles || []).some((profile) => profile.id === options.selectedProfile && profile.source.objectIndex === Number(target.dataset.hvacInspectNativeComponent));
    target.classList.toggle("selected", selected); target.setAttribute("aria-pressed", String(selected));
  }
  return true;
}
