import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";
import { evaluateHVACOperation } from "../hvac-operation.js";
import { renderHVACAvailability, connectHVACAvailability } from "./hvac-availability-view.js";

const copy = (key, fallback, values = {}) => t(`hvac.operation.${key}`, values, fallback);
const safe = escapeHTML;
const human = (value) => String(value || "").replaceAll("_", " ");
const clock = (minute) => `${String(Math.floor(minute / 60)).padStart(2, "0")}:${String(minute % 60).padStart(2, "0")}`;
const number = (value) => Math.abs(Number(value)) >= 1e6 ? Number(value).toExponential(1).replace(".0e", "e") : String(Number(Number(value).toPrecision(5)));
let graphObserver = null;
let graphFrame = 0;
let disconnectAvailability = null;

// Feedback is a relationship, not a later execution step. Give its endpoints
// stable semantic lanes; never recursively rank a feedback cycle as a DAG.
function graphLevels(graph, mode) {
  const nodes = graph.nodes || [];
  if (mode === "parameters") {
    const lane = (node) => node.boundary === "output" || node.kind === "external_output" ? 4 :
      node.boundary === "input" || ["external_input", "input", "schedule", "sensor", "setpoint", "parameter"].includes(node.kind) ? 0 :
      ["controller", "control", "condition", "decision", "comparison"].includes(node.kind) ? 1 :
      ["control_variable", "actuator", "modulation"].includes(node.kind) ? 2 : 3;
    const levels = new Map();
    for (const node of nodes) { const rank = lane(node); if (!levels.has(rank)) levels.set(rank, []); levels.get(rank).push(node); }
    return [...levels.entries()].sort((a, b) => a[0] - b[0]).map(([, values]) => values);
  }
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const incoming = new Map(nodes.map((node) => [node.id, []]));
  for (const edge of graph.edges || []) if (byID.has(edge.from) && byID.has(edge.to) && edge.from !== edge.to) incoming.get(edge.to).push(edge.from);
  const ranks = new Map(), pending = new Set();
  const rank = (id) => {
    if (ranks.has(id)) return ranks.get(id);
    if (pending.has(id)) return 0;
    pending.add(id);
    const value = Math.min(nodes.length, Math.max(0, ...incoming.get(id).filter((parent) => !pending.has(parent)).map((parent) => rank(parent) + 1)));
    pending.delete(id); ranks.set(id, value); return value;
  };
  const levels = new Map();
  for (const node of nodes) { const depth = rank(node.id); if (!levels.has(depth)) levels.set(depth, []); levels.get(depth).push(node); }
  return [...levels.entries()].sort((a, b) => a[0] - b[0]).map(([, values]) => values);
}

function controlHighlights(node) {
  const choices = (node.parameters || []).filter((parameter) => /control type|control mode|control variable|actuator variable|^action$|^value$|^target$|formula|quantity|node.*name|^zone.*name$|flow mode|flow control|operating mode|deadband|setpoint temperature|lower limit|upper limit|minimum part load|maximum part load|^(cooling|heating) sequence$/i.test(parameter.name));
  return choices.map((parameter) => {
    const raw = String(parameter.value), numeric = /^[-+]?\d+(?:\.\d+)?(?:e[-+]?\d+)?$/i.test(raw.trim());
    const unit = /temperature/i.test(parameter.name) ? " °C" : "";
    const value = numeric ? `${number(raw)}${unit}` : copy(`value.${raw}`, raw);
    const shortName = /cooling sequence/i.test(parameter.name) ? copy("coolingOrder", "Cooling priority") : /heating sequence/i.test(parameter.name) ? copy("heatingOrder", "Heating priority") : /deadband/i.test(parameter.name) ? copy("deadband", "Deadband") : /minimum part load/i.test(parameter.name) ? copy("minLoad", "Min load ratio") : /maximum part load/i.test(parameter.name) ? copy("maxLoad", "Max load ratio") : /lower|minimum/i.test(parameter.name) ? copy("minimum", "Min") : /upper|maximum/i.test(parameter.name) ? copy("maximum", "Max") : "";
    return `<span class="hvac-control-value" title="${safe(`${parameter.name}: ${raw}`)}" data-control-parameter="${safe(parameter.name)}" data-control-value="${safe(raw)}">${safe([shortName, value].filter(Boolean).join(" "))}</span>`;
  }).join("");
}

function renderSchedule(node, graph) {
  const segments = node.scheduleTrace;
  if (!segments?.length) return "";
  if (segments.some((segment) => segment.value === null)) return `<p class="hvac-control-schedule-unknown" data-hvac-schedule-unknown>${safe(copy("scheduleUnavailable", "Schedule values unavailable for this date / day type"))}</p>`;
  const targets = new Set((graph.edges || []).filter((edge) => edge.from === node.id).map((edge) => edge.to));
  const temperature = (graph.nodes || []).some((target) => targets.has(target.id) && ((target.parameters || []).some((parameter) => /control variable/i.test(parameter.name) && parameter.value === "Temperature") || /^ThermostatSetpoint:/i.test(target.source?.objectType || "")));
  const unit = temperature ? " °C" : "";
  const values = segments.map((segment) => segment.value), minimum = Math.min(...values), maximum = Math.max(...values);
  const y = (value) => minimum === maximum ? 22 : 39 - (value - minimum) / (maximum - minimum) * 30;
  const path = segments.map((segment, index) => `${index ? "L" : "M"}${18 + segment.startMinute / 1440 * 192},${y(segment.value)} H${18 + segment.endMinute / 1440 * 192}`).join(" ");
  return `<div class="hvac-control-schedule-value">${safe((minimum === maximum ? number(minimum) : `${number(minimum)}–${number(maximum)}`) + unit)}</div>
    <svg class="hvac-control-schedule-plot" data-hvac-schedule-plot viewBox="0 0 228 64" role="img" aria-label="${safe(`${node.source?.objectName || copy("control.schedule_value", "Schedule")} · ${copy("scheduleDay", "Schedule · 00:00–24:00")}`)}">
      <path d="${path}"/>${segments.map((segment) => `<rect data-schedule-value="${segment.value}" data-start-minute="${segment.startMinute}" data-end-minute="${segment.endMinute}" x="${18 + segment.startMinute / 1440 * 192}" y="6" width="${(segment.endMinute - segment.startMinute) / 1440 * 192}" height="36" fill="transparent"><title>${safe(`${clock(segment.startMinute)}–${clock(segment.endMinute)} · ${number(segment.value)}${unit}`)}</title></rect>`).join("")}
      <text x="18" y="58">00:00</text><text x="210" y="58" text-anchor="end">24:00</text>
    </svg>`;
}

function nodeLabel(node) {
  if (node.code === "always" && !(node.parameters || []).some((parameter) => parameter.name === "Value")) return copy("control.unconditional_condition", "Unconditional condition");
  return copy(`control.${node.code || node.kind || "unresolved_control"}`, human(node.code || node.kind || "unresolved control"));
}
function kindLabel(node) { const kind = node.boundary === "input" ? "external_input" : node.boundary === "output" ? "external_output" : node.kind || "unknown"; return copy(`kind.${kind}`, human(kind)); }

function renderNode(node, graph) {
  const source = node.source || {}, label = nodeLabel(node), role = kindLabel(node);
  return `<article class="hvac-control-node ${safe(node.kind || "unknown")}${node.boundary ? ` boundary-${safe(node.boundary)}` : ""}" data-hvac-control-node="${safe(node.id)}" data-hvac-node-kind="${safe(node.kind || "unknown")}"${node.boundary ? ` data-hvac-node-boundary="${safe(node.boundary)}"` : ""} role="group" aria-label="${safe(`${role}: ${label}`)}">
    <span class="hvac-control-kind">${safe(role)}</span><h5>${safe(label)}</h5>
    ${source.objectName ? `<span class="hvac-control-source" title="${safe([source.objectType, source.objectName].filter(Boolean).join(" · "))}">${safe(source.objectName)}</span>` : ""}
    ${source.objectType ? `<span class="hvac-control-object-type">${safe(source.objectType)}</span>` : ""}
    <div class="hvac-control-values">${controlHighlights(node)}</div>
    ${renderSchedule(node, graph)}
  </article>`;
}

function edgeLabel(edge) {
  const relation = copy(`relation.${edge.relation}`, human(edge.relation));
  const condition = edge.condition ? copy(`condition.${edge.condition}`, human(edge.condition)) : ["blocked", "requested", "no_request"].includes(edge.relation) ? copy(`branch.${edge.relation}`, human(edge.relation)) : "";
  const polarity = edge.polarity === "positive" ? "+" : edge.polarity === "negative" ? "−" : edge.polarity === "unknown" ? "?" : "";
  return { text: [condition, polarity].filter(Boolean).join(" "), description: [relation, condition, polarity ? copy(`polarity.${edge.polarity}`, human(edge.polarity)) : ""].filter(Boolean).join(" · ") };
}

function renderGraph(graph, mode) {
  if (!graph.nodes?.length) return `<p class="hvac-operation-unresolved" data-hvac-graph-unavailable>${safe(copy("graphUnavailable", "This graph is not available in this report. Analyze the input again to resolve its source definitions."))}</p>`;
  const graphID = `hvac-operation-${mode}`, byID = new Map(graph.nodes.map((node) => [node.id, node]));
  const edges = (graph.edges || []).filter((edge) => byID.has(edge.from) && byID.has(edge.to));
  return `<div class="hvac-control-graph ${mode}" data-hvac-operation-graph="${mode}" role="group" aria-label="${safe(copy(`${mode}Graph`, human(mode)))}">
    <svg class="hvac-control-connections" aria-hidden="true"><defs><marker id="${graphID}-arrow" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path d="M0,0 L7,3.5 L0,7 Z"/></marker></defs>${edges.map((edge) => { const label = edgeLabel(edge); return `<g data-hvac-control-edge data-from="${safe(edge.from)}" data-to="${safe(edge.to)}" data-relation="${safe(edge.relation)}"${edge.polarity ? ` data-polarity="${safe(edge.polarity)}"` : ""}${edge.condition ? ` data-condition="${safe(edge.condition)}"` : ""}><title>${safe(label.description)}</title><path marker-end="url(#${graphID}-arrow)"/>${label.text ? `<text>${safe(label.text)}</text>` : ""}</g>`; }).join("")}</svg>
    ${graphLevels(graph, mode).map((nodes) => `<div class="hvac-control-level">${nodes.map((node) => renderNode(node, graph)).join("")}</div>`).join("")}
    <ul class="sr-only">${edges.map((edge) => `<li>${safe(`${nodeLabel(byID.get(edge.from))} → ${nodeLabel(byID.get(edge.to))}: ${edgeLabel(edge).description}`)}</li>`).join("")}</ul>
  </div>`;
}

export function renderHVACOperationDetail(profile, schedules = [], { component = {}, calendar = {} } = {}) {
  const model = evaluateHVACOperation(profile || {}, schedules, calendar), date = model.calendar;
  const label = component.objectName || copy("equipment", "Equipment");
  const panel = (mode, title, graph) => `<section class="hvac-operation-panel ${mode}" data-hvac-operation-${mode}><div class="hvac-operation-panel-heading"><h4>${safe(copy(title, human(title)))}</h4></div>${renderGraph(graph, mode)}</section>`;
  return `<section class="hvac-operation-detail" data-hvac-operation-detail data-component-index="${safe(component.objectIndex ?? profile?.componentIndex ?? "")}">
    <header class="hvac-operation-heading"><h3>${safe(label)}</h3><span class="hvac-operation-object-type">${safe(component.objectType || "")}</span></header>
    <div class="hvac-operation-columns">
      <section class="hvac-operation-panel availability" data-hvac-operation-availability><div class="hvac-operation-panel-heading"><h4>${safe(copy("availability", "Availability"))}</h4></div>${renderHVACAvailability(profile, schedules, date)}</section>
      ${panel("onoff", "onOff", model.onOff)}
      ${panel("parameters", "parameterControl", model.parameterControl)}
    </div>
  </section>`;
}

export function connectHVACOperationDetail(container) {
  disconnectAvailability?.(); disconnectAvailability = connectHVACAvailability(container);
  graphObserver?.disconnect(); graphObserver = null; cancelAnimationFrame(graphFrame);
  const graphs = [...(container?.querySelectorAll(".hvac-control-graph") || [])]; if (!graphs.length) return;
  const drawGraph = (graph) => {
    if (!graph.isConnected) return;
    const bounds = graph.getBoundingClientRect(), svg = graph.querySelector(".hvac-control-connections");
    const horizontal = graph.dataset.hvacOperationGraph === "parameters" && getComputedStyle(graph).gridAutoFlow === "column";
    const nodes = new Map([...graph.querySelectorAll("[data-hvac-control-node]")].map((node) => { const rect = node.getBoundingClientRect(); return [node.dataset.hvacControlNode, { left: rect.left - bounds.left, right: rect.right - bounds.left, top: rect.top - bounds.top, bottom: rect.bottom - bounds.top, width: rect.width, height: rect.height }]; }));
    svg.setAttribute("viewBox", `0 0 ${Math.max(1, bounds.width)} ${Math.max(1, bounds.height)}`);
    const edges = [...svg.querySelectorAll("[data-hvac-control-edge]")];
    const port = (edge, side, rect) => { const group = edges.filter((candidate) => candidate.dataset[side] === edge.dataset[side]); return (horizontal ? rect.top : rect.left) + (horizontal ? rect.height : rect.width) * (group.indexOf(edge) + 1) / (group.length + 1); };
    let lane = 0;
    for (const edge of edges) {
      const from = nodes.get(edge.dataset.from), to = nodes.get(edge.dataset.to); if (!from || !to) continue;
      const x1 = horizontal ? from.right : port(edge, "from", from), y1 = horizontal ? port(edge, "from", from) : from.bottom;
      const x2 = horizontal ? to.left - 3 : port(edge, "to", to), y2 = horizontal ? port(edge, "to", to) : to.top - 3;
      const mid = horizontal ? (x1 + x2) / 2 : (y1 + y2) / 2;
      let points = horizontal ? [[x1, y1], [mid, y1], [mid, y2], [x2, y2]] : [[x1, y1], [x1, mid], [x2, mid], [x2, y2]];
      const intersects = [...nodes.entries()].some(([id, rect]) => id !== edge.dataset.from && id !== edge.dataset.to && points.slice(1).some(([x, y], index) => { const [px, py] = points[index]; return Math.max(x, px) >= rect.left - 3 && Math.min(x, px) <= rect.right + 3 && Math.max(y, py) >= rect.top - 3 && Math.min(y, py) <= rect.bottom + 3; }));
      if (intersects || (horizontal ? x2 <= x1 : y2 <= y1)) {
        const gutter = horizontal ? bounds.height - 4 - lane % 4 * 6 : ((x1 + x2) / 2 <= bounds.width / 2 ? 4 + lane % 4 * 6 : bounds.width - 4 - lane % 4 * 6);
        points = horizontal ? [[x1, y1], [x1 + 10, y1], [x1 + 10, gutter], [x2 - 10, gutter], [x2 - 10, y2], [x2, y2]] : [[x1, y1], [x1, y1 + 12], [gutter, y1 + 12], [gutter, y2 - 12], [x2, y2 - 12], [x2, y2]];
        lane++;
      }
      edge.querySelector("path").setAttribute("d", points.map(([x, y], index) => `${index ? "L" : "M"}${x},${y}`).join(" "));
      const text = edge.querySelector("text");
      if (text) {
        const comparisonInput = Boolean(edge.dataset.polarity);
        text.setAttribute("x", comparisonInput ? horizontal ? x2 - 8 : x2 : horizontal ? x1 + 12 : x1);
        text.setAttribute("y", comparisonInput ? y2 - 8 : horizontal ? y1 - 6 : y1 + 17);
        text.setAttribute("text-anchor", comparisonInput ? horizontal ? "end" : "middle" : horizontal ? "start" : "middle");
      }
    }
  };
  const draw = () => graphs.forEach(drawGraph); draw();
  if (typeof ResizeObserver !== "undefined") { graphObserver = new ResizeObserver(() => { cancelAnimationFrame(graphFrame); graphFrame = requestAnimationFrame(draw); }); graphs.forEach((graph) => graphObserver.observe(graph)); }
}
