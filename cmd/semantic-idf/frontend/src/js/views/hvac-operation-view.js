import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";
import { evaluateHVACOperation, hvacOperationDayTypes } from "../hvac-operation.js";

const copy = (key, fallback, values = {}) => t(`hvac.operation.${key}`, values, fallback);
const safe = escapeHTML;
const human = (value) => String(value || "").replaceAll("_", " ");
const clock = (minute) => `${String(Math.floor(minute / 60)).padStart(2, "0")}:${String(minute % 60).padStart(2, "0")}`;
const number = (value) => Math.abs(Number(value)) >= 1e6 ? Number(value).toExponential(1).replace(".0e", "e") : String(Number(Number(value).toPrecision(5)));
const permission = (value) => value === 1 ? copy("allowed", "Allowed") : value === 0 ? copy("blocked", "Blocked") : copy("conditional", "Conditional");
let graphObserver = null;
let graphFrame = 0;

function referencedSchedules(expression, result = new Set()) {
  if (!expression) return result;
  if (expression.scheduleName) result.add(expression.scheduleName);
  for (const child of expression.children || []) referencedSchedules(child, result);
  return result;
}

function renderPlot(trace, label, index) {
  const x = (minute) => 36 + minute / 1440 * 444;
  const pattern = `hvac-availability-unknown-${index}`;
  const ticks = [0, 6, 12, 18, 24];
  return `<svg class="hvac-availability-plot" data-hvac-availability-plot viewBox="0 0 492 78" role="img" aria-label="${safe(label)}">
    <defs><pattern id="${pattern}" width="6" height="6" patternUnits="userSpaceOnUse"><path d="M-1,1 L1,-1 M0,6 L6,0 M5,7 L7,5" class="hvac-availability-hatch"/></pattern></defs>
    ${ticks.map((hour) => `<line x1="${x(hour * 60)}" x2="${x(hour * 60)}" y1="8" y2="52" class="hvac-availability-grid"/><text x="${x(hour * 60)}" y="71" text-anchor="${hour === 24 ? "end" : hour === 0 ? "start" : "middle"}" class="hvac-availability-axis">${String(hour).padStart(2, "0")}:00</text>`).join("")}
    <text x="24" y="20" text-anchor="end" class="hvac-availability-axis">1</text><text x="24" y="50" text-anchor="end" class="hvac-availability-axis">0</text>
    ${trace.segments.map((segment) => {
      const value = segment.value === null ? "unknown" : String(segment.value);
      const title = `${clock(segment.startMinute)}–${clock(segment.endMinute)} · ${permission(segment.value)}`;
      return `<g><rect data-availability-value="${value}" data-start-minute="${segment.startMinute}" data-end-minute="${segment.endMinute}" class="hvac-availability-band ${value === "1" ? "allowed" : value === "0" ? "blocked" : "conditional"}" x="${x(segment.startMinute)}" y="${segment.value === 0 ? 40 : 10}" width="${(segment.endMinute - segment.startMinute) / 1440 * 444}" height="${segment.value === 0 ? 10 : 40}"><title>${safe(title)}</title></rect>${segment.value === null ? `<rect x="${x(segment.startMinute)}" y="10" width="${(segment.endMinute - segment.startMinute) / 1440 * 444}" height="40" fill="url(#${pattern})" pointer-events="none"/>` : ""}</g>`;
    }).join("")}
  </svg>`;
}

function renderAvailabilityRow(layer, index) {
  const owner = layer.owner || {}, schedules = [...referencedSchedules(layer.expression)];
  const title = owner.objectName || owner.objectType || copy("unresolved", "Unresolved control");
  return `<article class="hvac-availability-row" data-hvac-availability-layer="${safe(layer.id)}">
    <div class="hvac-availability-row-heading"><span class="hvac-operation-order">${index + 1}</span><div><strong>${safe(copy(`role.${layer.role}`, human(layer.role)))}</strong><span title="${safe([owner.objectType, title, ...schedules].filter(Boolean).join(" · "))}">${safe(title)}</span></div></div>
    ${renderPlot(layer, title, index)}
  </article>`;
}

function controlLevels(control) {
  const nodes = control.nodes || [], byID = new Map(nodes.map((node) => [node.id, node]));
  const incoming = new Map(nodes.map((node) => [node.id, []]));
  for (const edge of control.edges || []) if (byID.has(edge.from) && byID.has(edge.to) && edge.from !== edge.to) incoming.get(edge.to).push(edge.from);
  const ranks = new Map(), pending = new Set();
  const rank = (id) => {
    if (ranks.has(id)) return ranks.get(id);
    if (pending.has(id)) return 0;
    pending.add(id);
    const result = Math.min(nodes.length, Math.max(0, ...incoming.get(id).map((parent) => rank(parent) + 1)));
    pending.delete(id);
    ranks.set(id, result);
    return result;
  };
  for (const node of nodes) rank(node.id);
  const levels = new Map();
  for (const node of nodes) {
    const depth = ranks.get(node.id);
    if (!levels.has(depth)) levels.set(depth, []);
    levels.get(depth).push(node);
  }
  return [...levels.entries()].sort((a, b) => a[0] - b[0]).map(([, items]) => items);
}

function controlHighlights(node) {
  const parameters = node.parameters || [];
  // The graph shows decisions and manipulated quantities, not an object field
  // dump. Keep source field names on the individual value's hover label.
  const choices = parameters.filter((parameter) => /control type|control mode|flow mode|flow control|operating mode|deadband|setpoint temperature|lower limit|upper limit|minimum part load|maximum part load|^(cooling|heating) sequence$/i.test(parameter.name));
  return choices.slice(0, 3).map((parameter) => {
    const raw = String(parameter.value), numeric = /^[-+]?\d+(?:\.\d+)?(?:e[-+]?\d+)?$/i.test(raw.trim());
    const unit = /load range/i.test(parameter.name) ? " W" : /temperature/i.test(parameter.name) ? " °C" : "";
    const value = numeric ? `${number(raw)}${unit}` : copy(`value.${raw}`, raw);
    const shortName = /cooling sequence/i.test(parameter.name) ? copy("coolingOrder", "Cooling priority") : /heating sequence/i.test(parameter.name) ? copy("heatingOrder", "Heating priority") : /deadband/i.test(parameter.name) ? copy("deadband", "Deadband") : /minimum part load/i.test(parameter.name) ? copy("minLoad", "Min load ratio") : /maximum part load/i.test(parameter.name) ? copy("maxLoad", "Max load ratio") : /lower|minimum/i.test(parameter.name) ? copy("minimum", "Min") : /upper|maximum/i.test(parameter.name) ? copy("maximum", "Max") : "";
    return `<span class="hvac-control-value" title="${safe(`${parameter.name}: ${raw}`)}" data-control-parameter="${safe(parameter.name)}" data-control-value="${safe(raw)}">${safe([shortName, value].filter(Boolean).join(" "))}</span>`;
  }).join("");
}

function renderControlSchedule(node, control) {
  const segments = node.scheduleTrace;
  if (!segments?.length || segments.some((segment) => segment.value === null)) return "";
  const targets = new Set((control.edges || []).filter((edge) => edge.from === node.id).map((edge) => edge.to));
  const temperature = (control.nodes || []).some((target) => targets.has(target.id) && ((target.parameters || []).some((parameter) => /control variable/i.test(parameter.name) && parameter.value === "Temperature") || /^ThermostatSetpoint:/i.test(target.source?.objectType || "")));
  const unit = temperature ? " °C" : "";
  const values = segments.map((segment) => segment.value), minimum = Math.min(...values), maximum = Math.max(...values);
  if (minimum === maximum) return `<div class="hvac-control-schedule-value">${safe(number(minimum) + unit)}</div>`;
  const y = (value) => 27 - (value - minimum) / (maximum - minimum) * 21;
  const path = segments.map((segment, index) => `${index ? "L" : "M"}${segment.startMinute / 1440 * 180},${y(segment.value)} H${segment.endMinute / 1440 * 180}`).join(" ");
  return `<div class="hvac-control-schedule-value">${safe(`${number(minimum)}–${number(maximum)}${unit}`)}</div><svg class="hvac-control-schedule-plot" viewBox="0 0 180 42" role="img" aria-label="${safe(copy("scheduleDay", "Schedule · 00:00–24:00"))}"><path d="${path}"/><text x="0" y="40">00</text><text x="180" y="40" text-anchor="end">24</text></svg>`;
}

function renderControlNode(node, componentIndex, control) {
  const source = node.source || {}, code = node.code || node.kind || "unresolved";
  return `<article class="hvac-control-node ${safe(node.kind || "condition")}" data-hvac-control-node="${safe(node.id)}">
    <h5>${safe(copy(`control.${code}`, human(code)))}</h5>
    ${source.objectName && source.objectIndex !== componentIndex ? `<span class="hvac-control-source" title="${safe([source.objectType, source.objectName].filter(Boolean).join(" · "))}">${safe(source.objectName)}</span>` : ""}
    <div class="hvac-control-values">${controlHighlights(node)}</div>
    ${renderControlSchedule(node, control)}
  </article>`;
}

function renderControlSequence(control, componentIndex) {
  if (!control.nodes?.length) return `<div class="hvac-operation-unresolved">${safe(copy("unresolvedSequence", "The input does not expose a resolved control sequence for this equipment."))}</div>`;
  return `<div class="hvac-control-graph"><svg class="hvac-control-connections" aria-label="${safe(copy("relationships", "Control relationships"))}"><defs><marker id="hvac-control-arrow" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path d="M0,0 L7,3.5 L0,7 Z"/></marker></defs>${(control.edges || []).map((edge) => `<g data-hvac-control-edge data-from="${safe(edge.from)}" data-to="${safe(edge.to)}"><title>${safe(copy(`relation.${edge.relation}`, human(edge.relation)))}</title><path marker-end="url(#hvac-control-arrow)"/>${["blocked", "requested", "no_request"].includes(edge.relation) ? `<text>${safe(copy(`branch.${edge.relation}`, edge.relation))}</text>` : ""}</g>`).join("")}</svg>
    ${controlLevels(control).map((nodes) => `<div class="hvac-control-level">${nodes.map((node) => renderControlNode(node, componentIndex, control)).join("")}</div>`).join("")}
  </div>`;
}

export function renderHVACOperationDetail(profile, schedules = [], { component = {}, calendar = {} } = {}) {
  const model = evaluateHVACOperation(profile || {}, schedules, calendar), date = model.calendar;
  const label = component.objectName || copy("equipment", "Equipment");
  return `<section class="hvac-operation-detail" data-hvac-operation-detail data-component-index="${safe(component.objectIndex ?? profile?.componentIndex ?? "")}">
    <header class="hvac-operation-heading"><div><span class="hvac-operation-eyebrow">${safe(copy("operatingConditions", "OPERATING CONDITIONS"))}</span><h3>${safe(label)}</h3><span class="hvac-operation-object-type">${safe(component.objectType || "")}</span></div><span class="hvac-operation-basis">${safe(copy("configured", "From model controls"))}</span></header>
    <div class="hvac-operation-columns">
      <section class="hvac-operation-panel availability" data-hvac-operation-availability>
        <div class="hvac-operation-panel-heading"><span class="hvac-operation-panel-icon" aria-hidden="true">◷</span><div><h4>${safe(copy("availability", "Availability"))}</h4><p>${safe(copy("availabilityHint", "Upstream permissions → equipment → final permission"))}</p></div></div>
        <div class="hvac-operation-calendar"><label>${safe(copy("month", "Month"))}<input type="number" min="1" max="12" value="${date.month}" data-hvac-operation-calendar="month"/></label><label>${safe(copy("day", "Day"))}<input type="number" min="1" max="${new Date(Date.UTC(2000, date.month, 0)).getUTCDate()}" value="${date.day}" data-hvac-operation-calendar="day"/></label><label class="day-type">${safe(copy("dayType", "Day type"))}<select data-hvac-operation-calendar="dayType">${hvacOperationDayTypes.map((dayType) => `<option value="${dayType}"${dayType === date.dayType ? " selected" : ""}>${safe(copy(`dayType.${dayType}`, human(dayType)))}</option>`).join("")}</select></label></div>
        <div class="hvac-availability-legend">${[1, 0, null].map((value) => `<span><i class="${value === 1 ? "allowed" : value === 0 ? "blocked" : "conditional"}"></i>${safe(permission(value))}</span>`).join("")}</div>
        <div class="hvac-availability-stack">${model.layers.map(renderAvailabilityRow).join("") || `<p class="hvac-operation-unresolved">${safe(copy("unresolvedAvailability", "No resolved availability chain is available for this occurrence."))}</p>`}
          <article class="hvac-availability-row final" data-hvac-availability-final><div class="hvac-availability-final-heading"><span aria-hidden="true">↓</span><strong>${safe(copy("finalAvailability", "Final availability"))}</strong></div>${renderPlot(model.final, copy("finalAvailability", "Final availability"), "final")}</article>
        </div>
      </section>
      <section class="hvac-operation-panel sequence" data-hvac-operation-sequence><div class="hvac-operation-panel-heading"><span class="hvac-operation-panel-icon" aria-hidden="true">⑂</span><div><h4>${safe(copy("sequence", "Control sequence"))}</h4><p>${safe(copy("sequenceHint", "Input → decision → adjustment"))}</p></div></div>${renderControlSequence(model.control, component.objectIndex)}</section>
    </div>
  </section>`;
}

export function connectHVACOperationDetail(container) {
  graphObserver?.disconnect();
  graphObserver = null;
  cancelAnimationFrame(graphFrame);
  const graph = container?.querySelector(".hvac-control-graph");
  if (!graph) return;
  const draw = () => {
    if (!graph.isConnected) return;
    const bounds = graph.getBoundingClientRect(), svg = graph.querySelector(".hvac-control-connections");
    const nodes = new Map([...graph.querySelectorAll("[data-hvac-control-node]")].map((node) => {
      const rect = node.getBoundingClientRect();
      return [node.dataset.hvacControlNode, { left: rect.left - bounds.left, right: rect.right - bounds.left, top: rect.top - bounds.top, bottom: rect.bottom - bounds.top, width: rect.width }];
    }));
    svg.setAttribute("viewBox", `0 0 ${Math.max(1, bounds.width)} ${Math.max(1, bounds.height)}`);
    const edges = [...svg.querySelectorAll("[data-hvac-control-edge]")];
    const port = (edge, side, rect) => {
      const group = edges.filter((candidate) => candidate.dataset[side] === edge.dataset[side]);
      return rect.left + rect.width * (group.indexOf(edge) + 1) / (group.length + 1);
    };
    let lane = 0;
    for (const edge of edges) {
      const from = nodes.get(edge.dataset.from), to = nodes.get(edge.dataset.to);
      if (!from || !to) continue;
      const x1 = port(edge, "from", from), y1 = from.bottom;
      const x2 = port(edge, "to", to), y2 = to.top - 3, mid = (y1 + y2) / 2;
      const points = [[x1, y1], [x1, mid], [x2, mid], [x2, y2]];
      const crossesNode = [...nodes.entries()].some(([id, rect]) => id !== edge.dataset.from && id !== edge.dataset.to && points.slice(1).some(([x, y], index) => {
        const [px, py] = points[index];
        return Math.max(x, px) >= rect.left - 3 && Math.min(x, px) <= rect.right + 3 && Math.max(y, py) >= rect.top - 3 && Math.min(y, py) <= rect.bottom + 3;
      }));
      if (crossesNode) {
        // Skip-level links travel outside every card, including wrapped rows.
        const gutter = (x1 + x2) / 2 <= bounds.width / 2 ? 4 + lane % 4 * 4 : bounds.width - 4 - lane % 4 * 4;
        const startY = y1 + 12, endY = y2 - 10;
        points.splice(1, 2, [x1, startY], [gutter, startY], [gutter, endY], [x2, endY]);
        lane++;
      }
      edge.querySelector("path").setAttribute("d", points.map(([x, y], index) => `${index ? "L" : "M"}${x},${y}`).join(" "));
      const label = edge.querySelector("text");
      if (label) { label.setAttribute("x", x1); label.setAttribute("y", y1 + 12); label.setAttribute("text-anchor", "middle"); }
    }
  };
  draw();
  if (typeof ResizeObserver !== "undefined") {
    graphObserver = new ResizeObserver(() => { cancelAnimationFrame(graphFrame); graphFrame = requestAnimationFrame(draw); });
    graphObserver.observe(graph);
  }
}
