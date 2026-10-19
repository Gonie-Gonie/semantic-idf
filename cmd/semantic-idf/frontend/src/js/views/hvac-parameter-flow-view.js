import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";

const safe = escapeHTML;
const copy = (key, fallback) => t(`hvac.operation.${key}`, {}, fallback);
const human = (value) => String(value || "").replaceAll("_", " ");
const nodeLabel = (node = {}) => copy(`control.${node.code || node.kind || "unresolved_control"}`, human(node.code || node.kind || "unresolved control"));
const role = (node = {}) => node.boundary === "input" ? "external_input" : node.boundary === "output" ? "external_output" : node.kind || "unknown";
const polarity = (edge) => edge.polarity === "positive" ? "+" : edge.polarity === "negative" ? "−" : edge.polarity === "unknown" ? "?" : "";
let instance = 0;

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
  return bundle.nativeEdges.map((edge) => `${nodeLabel(byID.get(edge.from))} → ${nodeLabel(byID.get(edge.to))}: ${copy(`relation.${edge.relation}`, human(edge.relation))}${edge.polarity ? ` (${polarity(edge)})` : ""}${edge.condition ? ` · ${edge.condition}` : ""}`).join("\n");
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

function inspectorTemplate(card, byID, calendar) {
  return `<template data-hvac-parameter-details="${safe(card.id)}"><header class="hvac-parameter-inspector-heading"><div><h5>${safe(card.source?.objectName || card.source?.objectType || copy("unresolved", "Unresolved control"))}</h5><span class="hvac-control-object-type">${safe(card.source?.objectType || "")}</span></div><button type="button" data-hvac-close-parameter aria-label="${safe(copy("flow.closeDetails", "Close details"))}">×</button></header><div class="hvac-parameter-detail-members">${card.members.map((node) => memberDetails(node, card, byID, calendar)).join("")}</div>${card.relationships.length ? `<div class="hvac-parameter-detail-relations">${card.relationships.map((edge) => `<div data-hvac-parameter-term data-from="${safe(edge.from)}" data-to="${safe(edge.to)}"${edge.polarity ? ` data-polarity="${safe(edge.polarity)}"` : ""}><strong class="hvac-parameter-polarity">${safe(polarity(edge))}</strong> ${safe(nodeLabel(byID.get(edge.from)))} → ${safe(nodeLabel(byID.get(edge.to)))} <span>${safe(copy(`relation.${edge.relation}`, human(edge.relation)))}</span>${edge.condition ? ` <span>${safe(edge.condition)}</span>` : ""}</div>`).join("")}</div>` : ""}</template>`;
}

// Keep native edges as relationships between source tiles. A returning arrow is
// a dependency/feedback reference, not an inferred simulation execution order.
export function renderHVACParameterFlow(model = {}, { component = {}, calendar = {} } = {}) {
  const groups = model.groups || [], cards = groups.flatMap((group) => group.cards);
  const byID = new Map((model.nodes || []).map((node) => [node.id, node]));
  const cardForNode = new Map(cards.flatMap((card) => card.members.map((node) => [node.id, card])));
  const groupForCard = new Map(groups.flatMap((group, index) => group.cards.map((card) => [card.id, index])));
  const prefix = `hvac-parameter-${++instance}`;
  const bundles = new Map();
  for (const edge of model.relationships || []) {
    const from = cardForNode.get(edge.from), to = cardForNode.get(edge.to);
    if (!from || !to || from === to && !["positive", "negative"].includes(edge.polarity)) continue;
    const key = JSON.stringify([from.id, to.id, edge.relation, edge.polarity || ""]);
    const existing = bundles.get(key);
    if (existing) { existing.nativeEdges.push(edge); continue; }
    const feedback = edge.relation === "feedback" || groupForCard.get(from.id) > groupForCard.get(to.id);
    bundles.set(key, { ...edge, fromCard: from.id, toCard: to.id, feedback, nativeEdges: [edge] });
  }
  const edges = [...bundles.values()];
  const label = copy("parameterControl", "Parameter control");
  return `<div class="hvac-parameter-flow-shell" data-hvac-parameter-flow-shell><svg class="hvac-parameter-flow" data-hvac-parameter-flow role="group" aria-label="${safe([label, component.objectName].filter(Boolean).join(" · "))}" xmlns="http://www.w3.org/2000/svg"><defs><marker id="${prefix}-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="context-stroke"/></marker></defs><g class="hvac-parameter-flow-links">${edges.map((edge) => `<g data-hvac-flow-edge data-from-card="${safe(edge.fromCard)}" data-to-card="${safe(edge.toCard)}" data-hvac-control-edge data-from="${safe(edge.from)}" data-to="${safe(edge.to)}" data-relation="${safe(edge.relation)}"${edge.polarity ? ` data-polarity="${safe(edge.polarity)}"` : ""} class="${edge.feedback ? "feedback" : "signal"}"><title>${safe(edgeTitle(edge, byID))}</title><path fill="none" marker-end="url(#${prefix}-arrow)"/><text class="hvac-parameter-flow-sign">${safe(polarity(edge))}</text></g>`).join("")}</g>${groups.map((group, groupIndex) => `<g class="hvac-parameter-flow-stage ${safe(group.id)}" data-hvac-flow-group="${groupIndex}" data-hvac-parameter-group="${safe(group.id)}">${stageHeading(group)}${group.cards.map((card, index) => `<foreignObject data-hvac-flow-card="${safe(card.id)}" data-hvac-parameter-card="${safe(card.id)}" data-flow-group="${groupIndex}" data-flow-order="${index}" class="hvac-parameter-flow-tile"><button xmlns="http://www.w3.org/1999/xhtml" type="button" data-hvac-inspect-parameter="${safe(card.id)}" aria-expanded="false" title="${safe(sourceTitle(card.source))}"><strong>${safe(tileLabel(card, group))}</strong><span>${safe(card.source?.objectName || card.source?.objectType || copy("unresolved", "Unresolved control"))}</span></button></foreignObject>`).join("")}</g>`).join("")}</svg><p class="hvac-parameter-flow-hint">${safe(copy("flow.inspect", "Select a block for source fields and connections."))}</p><section class="hvac-parameter-inspector" data-hvac-parameter-inspector hidden aria-live="polite"></section>${cards.map((card) => inspectorTemplate(card, byID, calendar)).join("")}</div>`;
}

function layoutFlow(shell) {
  const svg = shell.querySelector("[data-hvac-parameter-flow]");
  if (!svg) return;
  const width = Math.max(280, Math.round(shell.getBoundingClientRect().width || 1100));
  const vertical = width < 760, margin = 24, gap = vertical ? 0 : 54;
  const groups = [...svg.querySelectorAll("[data-hvac-flow-group]")];
  const tileWidth = vertical ? width - margin * 2 : (width - margin * 2 - gap * (groups.length - 1)) / Math.max(1, groups.length);
  const positions = new Map();
  let cursor = 8, bottom = 120;
  groups.forEach((group, groupIndex) => {
    const tiles = [...group.querySelectorAll("[data-hvac-flow-card]")];
    const left = vertical ? margin : margin + groupIndex * (tileWidth + gap);
    const top = vertical ? cursor : 8;
    const heading = group.querySelector("text");
    heading.setAttribute("x", left); heading.setAttribute("y", top + 15);
    heading.querySelectorAll("tspan").forEach((line) => line.setAttribute("x", left));
    tiles.forEach((tile, index) => {
      const position = { x: left, y: top + 45 + index * 82, width: tileWidth, height: 66, group: groupIndex };
      for (const key of ["x", "y", "width", "height"]) tile.setAttribute(key, position[key]);
      positions.set(tile.getAttribute("data-hvac-flow-card"), position);
      bottom = Math.max(bottom, position.y + position.height + 34);
    });
    cursor = top + 45 + tiles.length * 82 + 24;
  });
  const repeated = new Map();
  const edges = [...svg.querySelectorAll("[data-hvac-flow-edge]")];
  edges.forEach((edge) => {
    const from = positions.get(edge.getAttribute("data-from-card")), to = positions.get(edge.getAttribute("data-to-card"));
    if (!from || !to) return;
    const pair = `${edge.getAttribute("data-from-card")}\n${edge.getAttribute("data-to-card")}`;
    const ordinal = repeated.get(pair) || 0; repeated.set(pair, ordinal + 1);
    const offset = (ordinal % 5 - 2) * 4;
    let path, labelX, labelY;
    if (from === to) {
      const x = from.x, y = from.y + from.height / 2;
      path = `M ${x} ${y + 12} C ${x - 22} ${y + 12}, ${x - 22} ${y - 12}, ${x} ${y - 12}`;
      labelX = x - 15; labelY = y - 4;
    } else if (vertical) {
      const forward = to.y > from.y;
      if (forward && to.group !== from.group) {
        const x1 = from.x + from.width * .68 + offset, y1 = from.y + from.height, x2 = to.x + to.width * .68 + offset, y2 = to.y;
        const lane = width - 11 - ordinal % 3 * 5;
        path = `M ${x1} ${y1} C ${lane} ${y1 + 12}, ${lane} ${y2 - 12}, ${x2} ${y2}`;
        labelX = lane - 6; labelY = (y1 + y2) / 2;
      } else {
        const lane = 8 + ordinal % 3 * 5, y1 = from.y + from.height / 2 + offset, y2 = to.y + to.height / 2 + offset;
        path = `M ${from.x} ${y1} C ${lane} ${y1}, ${lane} ${y2}, ${to.x} ${y2}`;
        labelX = lane + 2; labelY = (y1 + y2) / 2 - 3;
      }
    } else if (to.group === from.group + 1) {
      const x1 = from.x + from.width, y1 = from.y + from.height / 2 + offset, x2 = to.x, y2 = to.y + to.height / 2 + offset;
      const middle = (x1 + x2) / 2;
      path = `M ${x1} ${y1} C ${middle} ${y1}, ${middle} ${y2}, ${x2} ${y2}`;
      labelX = x2 - 11; labelY = y2 - 5;
    } else {
      const y1 = from.y + from.height, y2 = to.y + to.height, x1 = from.x + from.width * .7 + offset, x2 = to.x + to.width * .3 + offset;
      const lane = bottom - 8 + ordinal % 3 * 5;
      path = `M ${x1} ${y1} C ${x1} ${lane}, ${x2} ${lane}, ${x2} ${y2}`;
      labelX = x2 - 10; labelY = y2 + 14;
    }
    edge.querySelector("path").setAttribute("d", path);
    const text = edge.querySelector("text"); text.setAttribute("x", labelX); text.setAttribute("y", labelY);
  });
  svg.setAttribute("viewBox", `0 0 ${width} ${bottom}`);
  svg.setAttribute("height", bottom);
  shell.dataset.flowLayout = vertical ? "vertical" : "horizontal";
}

export function connectHVACParameterFlow(container) {
  const shells = [...container.querySelectorAll("[data-hvac-parameter-flow-shell]")];
  if (container.matches?.("[data-hvac-parameter-flow-shell]")) shells.unshift(container);
  const listeners = shells.map((shell) => {
    const inspector = shell.querySelector("[data-hvac-parameter-inspector]");
    const buttons = [...shell.querySelectorAll("[data-hvac-inspect-parameter]")];
    const select = (button) => {
      const id = button?.getAttribute("data-hvac-inspect-parameter");
      const template = [...shell.querySelectorAll("template[data-hvac-parameter-details]")].find((item) => item.getAttribute("data-hvac-parameter-details") === id);
      buttons.forEach((item) => { item.setAttribute("aria-expanded", String(item === button)); item.classList.toggle("selected", item === button); });
      shell.querySelectorAll("[data-hvac-flow-edge]").forEach((edge) => { edge.classList.toggle("selected", Boolean(button) && [edge.getAttribute("data-from-card"), edge.getAttribute("data-to-card")].includes(id)); });
      inspector.replaceChildren(...(template ? [template.content.cloneNode(true)] : []));
      inspector.hidden = !template;
    };
    const click = (event) => {
      const button = event.target.closest("[data-hvac-inspect-parameter]");
      if (button && shell.contains(button)) select(button);
      else if (event.target.closest("[data-hvac-close-parameter]")) {
        const selected = buttons.find((item) => item.getAttribute("aria-expanded") === "true");
        select(null); selected?.focus();
      }
    };
    shell.addEventListener("click", click);
    const observer = typeof ResizeObserver === "function" ? new ResizeObserver(() => layoutFlow(shell)) : null;
    observer?.observe(shell); layoutFlow(shell);
    return () => { observer?.disconnect(); shell.removeEventListener("click", click); };
  });
  return () => listeners.forEach((disconnect) => disconnect());
}
