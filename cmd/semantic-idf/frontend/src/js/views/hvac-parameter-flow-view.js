import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";

const safe = escapeHTML;
const copy = (key, fallback) => t(`hvac.operation.${key}`, {}, fallback);
const human = (value) => String(value || "").replaceAll("_", " ");
const nodeLabel = (node = {}) => copy(`control.${node.code || node.kind || "unresolved_control"}`, human(node.code || node.kind || "unresolved control"));
const role = (node = {}) => node.boundary === "input" ? "external_input" : node.boundary === "output" ? "external_output" : node.kind || "unknown";
const polarity = (edge) => edge.polarity === "positive" ? "+" : edge.polarity === "negative" ? "−" : edge.polarity === "unknown" ? "?" : "";
let instance = 0;
const flowConnections = new WeakMap();
const flowKinds = [
  ["signal", "signalPath", "Signal"],
  ["setpoint", "setpointPath", "Setpoint / target"],
  ["measurement", "measurementPath", "Measured input"],
  ["feedback", "feedbackPath", "Control feedback"],
];

function edgeKind(relation) {
  if (["feedback", "adds_measurement"].includes(relation)) return "feedback";
  if (relation === "measures") return "measurement";
  if (["references_setpoint", "sets_target", "subtracts_target"].includes(relation)) return "setpoint";
  return "signal";
}

function kindLabel(kind) {
  const [, key, fallback] = flowKinds.find(([value]) => value === kind);
  return copy(`flow.${key}`, fallback);
}

function sourceTitle(source = {}) {
  return [source.objectType, source.objectName].filter(Boolean).join(" · ");
}

function tileLabel(card, group) {
  const member = (...kinds) => kinds.map((kind) => card.members.find((node) => node.kind === kind)).find(Boolean);
  const quantities = [...new Set(card.members.flatMap((node) => (node.parameters || []).filter((parameter) => /^control variable$/i.test(parameter.name)).map((parameter) => parameter.value).filter(Boolean)))];
  const withQuantity = (label) => quantities.length === 1 ? `${copy(`value.${quantities[0]}`, quantities[0])} · ${label}` : label;
  if (group.id === "commands") return nodeLabel(member("control_variable", "actuator", "modulation") || card.members[0]);
  if (group.id === "controls") return withQuantity(nodeLabel(member("comparison", "controller", "control", "condition", "decision") || card.members[0]));
  if (group.id === "response") return nodeLabel(member("state_variable", "external_output", "equipment", "parameter") || card.members[0]);
  const target = member("setpoint");
  if (target) return withQuantity(nodeLabel(target));
  return nodeLabel(card.members.find((node) => node.code === "inlet_state") || member("external_input", "schedule") || card.members[0]);
}

function stageHeading(group) {
  const lines = copy(group.titleCode, human(group.id)).split(/\s*\/\s*/);
  return `<text class="hvac-parameter-flow-heading">${lines.map((line, index) => `<tspan dy="${index ? 14 : 0}">${safe(line)}</tspan>`).join("")}</text>`;
}

function edgeTitle(bundle, byID) {
  const endpoint = (id) => {
    const node = byID.get(id);
    return [nodeLabel(node), node?.source?.objectName].filter(Boolean).join(" · ");
  };
  return `${kindLabel(bundle.kind)} · ${bundle.nativeEdges.map((edge) => `${endpoint(edge.from)} → ${endpoint(edge.to)}: ${copy(`relation.${edge.relation}`, human(edge.relation))}${edge.polarity ? ` (${polarity(edge)})` : ""}${edge.condition ? ` · ${edge.condition}` : ""}`).join("\n")}`;
}

function memberDetails(node, card, byID, calendar) {
  let schedule = "";
  if (node.kind === "schedule" && node.scheduleTrace?.length) {
    const values = node.scheduleTrace.map((segment) => segment.value);
    const known = values.filter((value) => Number.isFinite(value));
    const targets = card.relationships.filter((edge) => edge.from === node.id).map((edge) => byID.get(edge.to)).filter(Boolean);
    const quantities = [...card.members, ...targets].flatMap((member) => (member.parameters || []).filter((parameter) => /control variable/i.test(parameter.name)).map((parameter) => parameter.value));
    const temperature = quantities.length > 0 && quantities.every((quantity) => /^temperature$/i.test(quantity));
    const value = known.length === values.length ? `${Math.min(...known)}${Math.min(...known) === Math.max(...known) ? "" : `–${Math.max(...known)}`}${temperature ? " °C" : ""}` : copy("status.unknown", "Unknown");
    const date = `${calendar.year ?? ""}-${String(calendar.month ?? "").padStart(2, "0")}-${String(calendar.day ?? "").padStart(2, "0")}`;
    const basis = t("hvac.operation.compact.scheduleBasis", { date, dayType: copy(`dayType.${calendar.dayType}`, calendar.dayType || "") });
    schedule = `<p class="hvac-parameter-detail-schedule">${safe(copy("flow.representativeSchedule", "Representative-day schedule"))}: <strong class="hvac-control-schedule-value" title="${safe(basis)}">${safe(value)}</strong></p>`;
  }
  return `<section class="hvac-parameter-detail-member" data-hvac-control-node="${safe(node.id)}" data-hvac-node-kind="${safe(node.kind || "")}" data-hvac-source-index="${safe(node.source?.objectIndex ?? "")}" data-hvac-source-type="${safe(node.source?.objectType || "")}" data-hvac-source-name="${safe(node.source?.objectName || "")}"${node.boundary ? ` data-hvac-node-boundary="${safe(node.boundary)}"` : ""}><h6>${safe(nodeLabel(node))}</h6><span class="hvac-parameter-detail-role">${safe(copy(`kind.${role(node)}`, human(role(node))))}</span>${sourceTitle(node.source) ? `<p class="hvac-control-source">${safe(sourceTitle(node.source))}</p>` : ""}${schedule}${node.parameters?.length ? `<dl class="hvac-parameter-detail-values">${node.parameters.map((parameter) => `<div data-control-parameter="${safe(parameter.name)}" data-control-value="${safe(parameter.value)}" data-control-source-index="${safe(parameter.objectIndex ?? "")}" data-control-field-index="${safe(parameter.fieldIndex ?? "")}"><dt>${safe(parameter.name)}</dt><dd>${safe(parameter.value)}</dd></div>`).join("")}</dl>` : ""}</section>`;
}

function relationshipDetails(edges, byID) {
  if (!edges.length) return "";
  return `<div class="hvac-parameter-detail-relations">${edges.map((edge) => `<div data-hvac-parameter-term data-from="${safe(edge.from)}" data-to="${safe(edge.to)}" data-relation="${safe(edge.relation)}"${edge.polarity ? ` data-polarity="${safe(edge.polarity)}"` : ""}${edge.condition ? ` data-condition="${safe(edge.condition)}"` : ""}><strong class="hvac-parameter-polarity">${safe(polarity(edge))}</strong> ${safe(nodeLabel(byID.get(edge.from)))} → ${safe(nodeLabel(byID.get(edge.to)))} <span>${safe(copy(`relation.${edge.relation}`, human(edge.relation)))}</span>${edge.condition ? ` <span>${safe(edge.condition)}</span>` : ""}</div>`).join("")}</div>`;
}

function performanceDetails(card, byID, calendar) {
  if (!card.performanceModels?.length) return "";
  return `<section class="hvac-parameter-performance" data-hvac-performance-model><h5>${safe(copy("flow.performanceModel", "Performance model"))}</h5><p>${safe(copy("flow.performanceModelDetail", "Curves describing equipment response under operating conditions."))}</p>${card.performanceModels.map(({ node, relationships }) => `<section class="hvac-parameter-performance-entry">${memberDetails(node, card, byID, calendar)}${relationshipDetails(relationships, byID)}</section>`).join("")}</section>`;
}

function inspectorTemplate(card, byID, calendar) {
  const modelEdges = new Set((card.performanceModels || []).flatMap((model) => model.relationships));
  const controlEdges = card.relationships.filter((edge) => !modelEdges.has(edge));
  return `<template data-hvac-parameter-details="${safe(card.id)}"><header class="hvac-parameter-inspector-heading"><div><h5>${safe(card.source?.objectName || card.source?.objectType || copy("unresolved", "Unresolved control"))}</h5><span class="hvac-control-object-type">${safe(card.source?.objectType || "")}</span></div><button type="button" data-hvac-close-parameter aria-label="${safe(copy("flow.closeDetails", "Close details"))}">×</button></header><div class="hvac-parameter-detail-members">${card.members.map((node) => memberDetails(node, card, byID, calendar)).join("")}</div>${relationshipDetails(controlEdges, byID)}${performanceDetails(card, byID, calendar)}</template>`;
}

// Native relation defines the meaning; direction only chooses the drawn route.
export function renderHVACParameterFlow(model = {}, { component = {}, calendar = {} } = {}) {
  const groups = model.groups || [], cards = groups.flatMap((group) => group.cards);
  const byID = new Map((model.nodes || []).map((node) => [node.id, node]));
  const cardForNode = new Map(cards.flatMap((card) => card.members.map((node) => [node.id, card])));
  const prefix = `hvac-parameter-${++instance}`;
  const bundles = new Map();
  for (const edge of model.relationships || []) {
    const from = cardForNode.get(edge.from), to = cardForNode.get(edge.to);
    if (!from || !to || from === to && !["positive", "negative"].includes(edge.polarity)) continue;
    const key = JSON.stringify([from.id, to.id, edge.relation, edge.polarity || "", edge.condition || ""]);
    const existing = bundles.get(key);
    if (existing) { existing.nativeEdges.push(edge); continue; }
    bundles.set(key, { ...edge, fromCard: from.id, toCard: to.id, kind: edgeKind(edge.relation), nativeEdges: [edge] });
  }
  const edges = [...bundles.values()];
  const label = copy("parameterControl", "Parameter control");
  return `<div class="hvac-parameter-flow-shell" data-hvac-parameter-flow-shell><svg class="hvac-parameter-flow" data-hvac-parameter-flow role="group" aria-label="${safe([label, component.objectName].filter(Boolean).join(" · "))}" xmlns="http://www.w3.org/2000/svg"><defs><marker id="${prefix}-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="5" markerHeight="5" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="context-stroke"/></marker></defs><g class="hvac-parameter-flow-links">${edges.map((edge) => `<g data-hvac-flow-edge data-from-card="${safe(edge.fromCard)}" data-to-card="${safe(edge.toCard)}" data-hvac-control-edge data-from="${safe(edge.from)}" data-to="${safe(edge.to)}" data-relation="${safe(edge.relation)}"${edge.polarity ? ` data-polarity="${safe(edge.polarity)}"` : ""}${edge.condition ? ` data-condition="${safe(edge.condition)}"` : ""} class="${edge.kind}" data-flow-kind="${edge.kind}" tabindex="0" role="button" aria-pressed="false" aria-label="${safe(edgeTitle(edge, byID))}"><title>${safe(edgeTitle(edge, byID))}</title><path class="hvac-parameter-halo"/><path class="hvac-parameter-wire" marker-end="url(#${prefix}-arrow)"/><path class="hvac-parameter-hit"/><circle data-flow-port="source" r="2.5"/><circle data-flow-port="target" r="2.5"/><g class="hvac-parameter-flow-badge"${polarity(edge) ? "" : " hidden"}><rect x="-8" y="-9" width="16" height="18" rx="4"/><text class="hvac-parameter-flow-sign" text-anchor="middle" y="4">${safe(polarity(edge))}</text></g></g>`).join("")}</g>${groups.map((group, groupIndex) => `<g class="hvac-parameter-flow-stage ${safe(group.id)}" data-hvac-flow-group="${groupIndex}" data-hvac-parameter-group="${safe(group.id)}">${stageHeading(group)}${group.cards.map((card, index) => `<foreignObject data-hvac-flow-card="${safe(card.id)}" data-hvac-parameter-card="${safe(card.id)}" data-flow-group="${groupIndex}" data-flow-order="${index}" class="hvac-parameter-flow-tile"><button xmlns="http://www.w3.org/1999/xhtml" type="button" data-hvac-inspect-parameter="${safe(card.id)}" aria-expanded="false" title="${safe(sourceTitle(card.source))}"><strong>${safe(tileLabel(card, group))}</strong><span>${safe(card.source?.objectName || card.source?.objectType || copy("unresolved", "Unresolved control"))}</span></button></foreignObject>`).join("")}</g>`).join("")}</svg><div class="hvac-parameter-flow-legend">${flowKinds.map(([kind, key, text]) => `<span class="${kind}" title="${safe(copy(`flow.${key}Detail`, ""))}"><i aria-hidden="true"></i>${safe(copy(`flow.${key}`, text))}</span>`).join("")}</div><p class="hvac-parameter-flow-hint"><span data-hvac-flow-hint>${safe(copy("flow.inspect", "Hover or focus to trace paths; select a block for details."))}</span><span data-hvac-flow-description hidden aria-live="polite"></span></p><section class="hvac-parameter-inspector" data-hvac-parameter-inspector hidden aria-live="polite"></section>${cards.map((card) => inspectorTemplate(card, byID, calendar)).join("")}</div>`;
}

function setGeometry(node, values) {
  Object.entries(values).forEach(([key, value]) => node.setAttribute(key, value));
}

// Long links use broad turns outside the cards. Short neighbouring links use
// smooth curves within their shared gutter instead of a chain of sharp bends.
function perimeterPath(points) {
  let path = `M ${points[0].x} ${points[0].y}`;
  for (let index = 1; index < points.length - 1; index++) {
    const before = points[index - 1], point = points[index], after = points[index + 1];
    const incoming = Math.hypot(point.x - before.x, point.y - before.y);
    const outgoing = Math.hypot(after.x - point.x, after.y - point.y);
    const radius = Math.min(24, incoming / 2, outgoing / 2);
    if (!radius) { path += ` L ${point.x} ${point.y}`; continue; }
    const start = { x: point.x + (before.x - point.x) * radius / incoming, y: point.y + (before.y - point.y) * radius / incoming };
    const end = { x: point.x + (after.x - point.x) * radius / outgoing, y: point.y + (after.y - point.y) * radius / outgoing };
    path += ` L ${start.x} ${start.y} Q ${point.x} ${point.y} ${end.x} ${end.y}`;
  }
  const last = points.at(-1);
  return `${path} L ${last.x} ${last.y}`;
}

function flowRoute(from, to) {
  if (from === to) return "local";
  if (to === from + 1) return "adjacent";
  if (to === from - 1) return "adjacent-return";
  return to > from ? "bypass" : "return";
}

function preferredCardWidth(tiles) {
  const context = document.createElement("canvas").getContext("2d");
  let width = 220;
  for (const tile of tiles) {
    const button = tile.querySelector("button"), box = getComputedStyle(button);
    const inset = [box.paddingLeft, box.paddingRight, box.borderLeftWidth, box.borderRightWidth]
      .reduce((sum, value) => sum + (parseFloat(value) || 0), 0);
    for (const text of button.querySelectorAll("strong, span")) {
      const style = getComputedStyle(text);
      if (context) context.font = `${style.fontWeight} ${style.fontSize} ${style.fontFamily}`;
      for (const line of text.textContent.split(/\r?\n/)) {
        const measured = context ? context.measureText(line).width : line.length * parseFloat(style.fontSize);
        const spacing = (parseFloat(style.letterSpacing) || 0) * Math.max(0, line.length - 1);
        width = Math.max(width, measured + spacing + inset + 4);
      }
    }
  }
  // Unbounded object names wrap rather than forcing every card to grow forever.
  return Math.min(360, Math.ceil(width));
}

function layoutFlow(shell) {
  const svg = shell.querySelector("[data-hvac-parameter-flow]");
  if (!svg) return;
  const width = Math.max(240, Math.round(shell.getBoundingClientRect().width || 1100));
  const groups = [...svg.querySelectorAll("[data-hvac-flow-group]")];
  const tiles = [...svg.querySelectorAll("[data-hvac-flow-card]")];
  const byID = new Map(tiles.map((tile) => [tile.dataset.hvacFlowCard, tile]));
  const plans = [...svg.querySelectorAll("[data-hvac-flow-edge]")].map((edge, index) => {
    const from = byID.get(edge.dataset.fromCard), to = byID.get(edge.dataset.toCard);
    return { edge, index, from, to, fromGroup: Number(from.dataset.flowGroup), toGroup: Number(to.dataset.flowGroup) };
  });
  const gutters = new Map();
  const register = (group, key) => {
    const entries = gutters.get(group) || [];
    entries.push(key); gutters.set(group, entries);
  };
  plans.forEach((plan) => {
    plan.route = flowRoute(plan.fromGroup, plan.toGroup);
    if (plan.route === "adjacent") register(plan.fromGroup, `${plan.index}:middle`);
    else if (plan.route === "adjacent-return") register(plan.fromGroup - 1, `${plan.index}:middle`);
    else if (plan.route === "local") register(plan.fromGroup - 1, `${plan.index}:local`);
    else {
      register(plan.route === "bypass" ? plan.fromGroup : plan.fromGroup - 1, `${plan.index}:from`);
      register(plan.route === "return" ? plan.toGroup : plan.toGroup - 1, `${plan.index}:to`);
    }
  });
  let gap = Math.max(72, ...[...gutters.values()].map((entries) => entries.length * 8 + 24));
  let leftMargin = Math.max(28, (gutters.get(-1)?.length || 0) * 8 + 24), rightMargin = 24;
  const vertical = width < 960 || (width - leftMargin - rightMargin - gap * (groups.length - 1)) / groups.length < 150;
  const leftPlans = plans.filter((plan) => plan.toGroup <= plan.fromGroup);
  const rightPlans = plans.filter((plan) => plan.toGroup > plan.fromGroup);
  if (vertical) {
    leftMargin = Math.min(width * .25, 20 + leftPlans.length * 8);
    rightMargin = Math.min(width * .25, 20 + rightPlans.length * 8);
  }
  const columns = vertical ? 1 : Math.max(1, groups.length);
  const available = width - leftMargin - rightMargin - (vertical ? 0 : gap * (columns - 1));
  const tileWidth = Math.min(preferredCardWidth(tiles), available / columns);
  const surplus = Math.max(0, available - columns * tileWidth);
  // Half a spacing unit at each outer edge balances the full intercolumn units.
  leftMargin += surplus / (columns * 2);
  rightMargin += surplus / (columns * 2);
  if (!vertical) gap += surplus / columns;
  // Set all widths before measuring wrapped text, then stack actual heights.
  tiles.forEach((tile) => tile.setAttribute("width", tileWidth));
  const heights = new Map(tiles.map((tile) => {
    // CSS box dimensions stay in SVG coordinates while an old viewBox is being
    // resized; screen-space rectangles would temporarily scale the card height.
    const height = parseFloat(getComputedStyle(tile.querySelector("button")).height) || 76;
    return [tile, Math.max(76, Math.ceil(height))];
  }));
  const bypasses = plans.filter((plan) => plan.route === "bypass");
  const returns = plans.filter((plan) => plan.route === "return");
  const positions = new Map();
  const start = 56 + (vertical ? 0 : bypasses.length * 18);
  let cursor = 8, bottom = 120;
  groups.forEach((group, groupIndex) => {
    const members = [...group.querySelectorAll("[data-hvac-flow-card]")];
    const left = leftMargin + (vertical ? 0 : groupIndex * (tileWidth + gap));
    const heading = group.querySelector("text");
    setGeometry(heading, { x: left, y: vertical ? cursor + 15 : 23 });
    heading.querySelectorAll("tspan").forEach((line) => line.setAttribute("x", left));
    let rowTop = vertical ? cursor + 45 : start;
    members.forEach((tile) => {
      const position = { x: left, y: rowTop, width: tileWidth, height: heights.get(tile) };
      setGeometry(tile, position);
      positions.set(tile, position);
      bottom = Math.max(bottom, position.y + position.height + 24);
      rowTop += position.height + 20;
    });
    cursor = rowTop + 24;
  });
  if (!vertical) {
    // Align the controller/command/response spine with its actual incoming
    // sources. Returning dependencies do not influence this geometric anchor.
    groups.slice(1).forEach((group, offset) => {
      let next = start;
      for (const tile of group.querySelectorAll("[data-hvac-flow-card]")) {
        const incoming = plans.filter((plan) => plan.to === tile && plan.fromGroup < offset + 1);
        const nearest = Math.max(-1, ...incoming.map((plan) => plan.fromGroup));
        const centers = incoming.filter((plan) => plan.fromGroup === nearest).map((plan) => {
          const source = positions.get(plan.from);
          return source.y + source.height / 2;
        }).sort((a, b) => a - b);
        const position = positions.get(tile);
        if (centers.length) position.y = centers[Math.floor(centers.length / 2)] - position.height / 2;
        position.y = Math.max(next, position.y);
        setGeometry(tile, position);
        next = position.y + position.height + 20;
      }
    });
    bottom = Math.max(120, ...[...positions.values()].map((position) => position.y + position.height + 24));
  }
  const ports = new Map();
  plans.forEach((plan) => {
    plan.fromSide = plan.toGroup > plan.fromGroup ? "right" : "left";
    plan.toSide = vertical
      ? plan.fromSide
      : ["return", "adjacent-return"].includes(plan.route) ? "right" : "left";
    for (const end of ["from", "to"]) {
      const key = `${plan[end].dataset.hvacFlowCard}:${plan[`${end}Side`]}`;
      const entries = ports.get(key) || [];
      entries.push({ plan, end }); ports.set(key, entries);
    }
  });
  ports.forEach((entries) => {
    entries.sort((a, b) => positions.get(a.plan[a.end === "from" ? "to" : "from"]).y - positions.get(b.plan[b.end === "from" ? "to" : "from"]).y || a.plan.index - b.plan.index);
    entries.forEach(({ plan, end }, index) => {
      const position = positions.get(plan[end]);
      plan[`${end}Point`] = { x: position.x + (plan[`${end}Side`] === "right" ? position.width : 0), y: position.y + (index + 1) * position.height / (entries.length + 1) };
    });
  });
  const gutterX = (group, key) => {
    const entries = gutters.get(group), index = entries.indexOf(key);
    const left = group < 0 ? 0 : leftMargin + group * (tileWidth + gap) + tileWidth;
    const available = group < 0 ? leftMargin : gap;
    return left + (index + 1) * available / (entries.length + 1);
  };
  plans.forEach((plan) => {
    const { edge, fromPoint: from, toPoint: to } = plan;
    let points, path, badgeX;
    if (vertical) {
      const sidePlans = plan.toGroup > plan.fromGroup ? rightPlans : leftPlans;
      const ordinal = sidePlans.indexOf(plan);
      const lane = plan.toGroup > plan.fromGroup
        ? width - 12 - (ordinal + 1) * Math.max(0, rightMargin - 20) / (sidePlans.length + 1)
        : 12 + (ordinal + 1) * Math.max(0, leftMargin - 20) / (sidePlans.length + 1);
      points = [from, { x: lane, y: from.y }, { x: lane, y: to.y }, to];
      badgeX = lane;
      edge.dataset.flowRoute = plan.toGroup > plan.fromGroup ? "vertical" : "vertical-return";
    } else if (["adjacent", "adjacent-return", "local"].includes(plan.route)) {
      const lane = gutterX(plan.route === "adjacent" ? plan.fromGroup : plan.fromGroup - 1, `${plan.index}:${plan.route === "local" ? "local" : "middle"}`);
      points = [from, { x: lane, y: from.y }, { x: lane, y: to.y }, to];
      path = `M ${from.x} ${from.y} C ${lane} ${from.y} ${lane} ${to.y} ${to.x} ${to.y}`;
      badgeX = lane;
    } else {
      const top = plan.route === "bypass";
      const laneY = top ? 52 + bypasses.indexOf(plan) * 18 : bottom + returns.indexOf(plan) * 20;
      const exit = gutterX(top ? plan.fromGroup : plan.fromGroup - 1, `${plan.index}:from`);
      const enter = gutterX(top ? plan.toGroup - 1 : plan.toGroup, `${plan.index}:to`);
      points = [from, { x: exit, y: from.y }, { x: exit, y: laneY }, { x: enter, y: laneY }, { x: enter, y: to.y }, to];
      badgeX = enter;
    }
    if (!vertical) edge.dataset.flowRoute = plan.route;
    edge.dataset.flowLane = `${edge.dataset.flowRoute}-${plan.index}`;
    edge.querySelectorAll("path").forEach((item) => item.setAttribute("d", path || perimeterPath(points)));
    for (const [name, point] of [["source", from], ["target", to]]) setGeometry(edge.querySelector(`[data-flow-port="${name}"]`), { cx: point.x, cy: point.y });
    edge.querySelector(".hvac-parameter-flow-badge").setAttribute("transform", `translate(${badgeX} ${to.y - 13})`);
  });
  const height = bottom + (vertical ? 0 : returns.length * 20) + 16;
  setGeometry(svg, { viewBox: `0 0 ${width} ${height}`, height });
  shell.dataset.flowLayout = vertical ? "vertical" : "horizontal";
}

export function connectHVACParameterFlow(container) {
  const shells = [...container.querySelectorAll("[data-hvac-parameter-flow-shell]")];
  if (container.matches?.("[data-hvac-parameter-flow-shell]")) shells.unshift(container);
  const listeners = shells.map((shell) => {
    flowConnections.get(shell)?.();
    const inspector = shell.querySelector("[data-hvac-parameter-inspector]");
    const buttons = [...shell.querySelectorAll("[data-hvac-inspect-parameter]")];
    const edges = [...shell.querySelectorAll("[data-hvac-flow-edge]")];
    const cards = [...shell.querySelectorAll("[data-hvac-flow-card]")];
    let selectedCard = buttons.find((button) => button.getAttribute("aria-expanded") === "true") || null;
    let selectedEdge = edges.find((edge) => edge.getAttribute("aria-pressed") === "true") || null;
    let hovered = null, focused = null;
    const trace = () => {
      const target = hovered || focused || selectedEdge || selectedCard;
      const edge = target?.matches("[data-hvac-flow-edge]") ? target : null;
      const id = target?.getAttribute("data-hvac-inspect-parameter");
      const active = edges.filter((item) => edge ? item === edge : id && [item.dataset.fromCard, item.dataset.toCard].includes(id));
      const related = new Set(active.flatMap((item) => [item.dataset.fromCard, item.dataset.toCard]));
      if (id) related.add(id);
      if (target) shell.dataset.flowFocus = edge ? "edge" : "card";
      else delete shell.dataset.flowFocus;
      edges.forEach((item) => {
        item.classList.toggle("is-highlighted", active.includes(item));
        item.setAttribute("aria-pressed", String(item === selectedEdge));
      });
      cards.forEach((card) => {
        card.classList.toggle("is-related", related.has(card.dataset.hvacFlowCard));
        card.classList.toggle("is-source", Boolean(edge) && card.dataset.hvacFlowCard === edge.dataset.fromCard);
        card.classList.toggle("is-target", Boolean(edge) && card.dataset.hvacFlowCard === edge.dataset.toCard);
      });
      const description = shell.querySelector("[data-hvac-flow-description]");
      description.textContent = edge?.getAttribute("aria-label") || "";
      description.hidden = !edge;
      shell.querySelector("[data-hvac-flow-hint]").hidden = Boolean(edge);
    };
    const select = (button) => {
      selectedCard = button; selectedEdge = null;
      const id = button?.getAttribute("data-hvac-inspect-parameter");
      const template = [...shell.querySelectorAll("template[data-hvac-parameter-details]")].find((item) => item.getAttribute("data-hvac-parameter-details") === id);
      buttons.forEach((item) => { item.setAttribute("aria-expanded", String(item === button)); item.classList.toggle("selected", item === button); });
      inspector.replaceChildren(...(template ? [template.content.cloneNode(true)] : []));
      inspector.hidden = !template;
      trace();
    };
    const targetFor = (node) => node instanceof Element && shell.contains(node) ? node.closest("[data-hvac-flow-edge],[data-hvac-inspect-parameter]") : null;
    const click = (event) => {
      const button = event.target.closest("[data-hvac-inspect-parameter]");
      if (button && shell.contains(button)) select(button);
      else if (event.target.closest("[data-hvac-flow-edge]")) {
        const edge = event.target.closest("[data-hvac-flow-edge]");
        selectedEdge = selectedEdge === edge ? null : edge; trace();
      }
      else if (event.target.closest("[data-hvac-close-parameter]")) {
        const selected = buttons.find((item) => item.getAttribute("aria-expanded") === "true");
        select(null); selected?.focus();
      }
    };
    const pointerover = (event) => { hovered = targetFor(event.target); trace(); };
    const pointerout = (event) => { hovered = targetFor(event.relatedTarget); trace(); };
    const focusin = (event) => { focused = targetFor(event.target); trace(); };
    const focusout = (event) => { focused = targetFor(event.relatedTarget); trace(); };
    const keydown = (event) => {
      if (event.key === "Escape") {
        const edgeActive = selectedEdge || hovered?.matches("[data-hvac-flow-edge]") || focused?.matches("[data-hvac-flow-edge]");
        event.preventDefault(); hovered = null; focused = null;
        selectedEdge = null;
        targetFor(document.activeElement)?.blur();
        if (!edgeActive) select(null);
        trace();
      } else if (["Enter", " "].includes(event.key) && event.target.matches("[data-hvac-flow-edge]")) {
        event.preventDefault(); selectedEdge = selectedEdge === event.target ? null : event.target; trace();
      }
    };
    const handlers = { click, pointerover, pointerout, focusin, focusout, keydown };
    Object.entries(handlers).forEach(([type, handler]) => shell.addEventListener(type, handler));
    let lastWidth = -1, disconnected = false;
    const resize = () => {
      const width = Math.round(shell.getBoundingClientRect().width);
      if (width === lastWidth) return;
      lastWidth = width;
      layoutFlow(shell);
    };
    const observer = typeof ResizeObserver === "function" ? new ResizeObserver(resize) : null;
    observer?.observe(shell); resize(); trace();
    document.fonts?.ready.then(() => { if (!disconnected) layoutFlow(shell); });
    const disconnect = () => {
      disconnected = true;
      observer?.disconnect();
      Object.entries(handlers).forEach(([type, handler]) => shell.removeEventListener(type, handler));
      if (flowConnections.get(shell) === disconnect) flowConnections.delete(shell);
    };
    flowConnections.set(shell, disconnect);
    return disconnect;
  });
  return () => listeners.forEach((disconnect) => disconnect());
}
