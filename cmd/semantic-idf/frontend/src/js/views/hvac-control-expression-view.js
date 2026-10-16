import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";
import { buildHVACControlExpressions } from "../hvac-control-expressions.js";

const safe = escapeHTML;
const copy = (key, fallback) => t(`hvac.operation.${key}`, {}, fallback);
const human = (value) => String(value || "").replaceAll("_", " ");
const number = (value) => Math.abs(Number(value)) >= 1e6 ? Number(value).toExponential(1).replace(".0e", "e") : String(Number(Number(value).toPrecision(5)));
const nodeLabel = (node = {}) => copy(`control.${node.code || node.kind || "unresolved_control"}`, human(node.code || node.kind || "unresolved control"));
const nodeKind = (node) => node.boundary === "input" ? "external_input" : node.boundary === "output" ? "external_output" : node.kind || "unknown";

function sourceHTML(source, className = "hvac-expression-source") {
  if (!source?.objectName && !source?.objectType) return "";
  return `<span class="${className}" data-hvac-expression-source-index="${safe(source.objectIndex ?? "")}" data-hvac-expression-source-type="${safe(source.objectType || "")}" title="${safe([source.objectType, source.objectName].filter(Boolean).join(" · "))}">${safe(source.objectName || source.objectType)}</span>`;
}

function highlights(node) {
  const choices = (node.parameters || []).filter(({ name }) =>
    /control type|control mode|control variable|actuator variable|^action$|^value$|^target$|formula|quantity|node.*name|^zone.*name$|flow mode|flow control|operating mode|deadband|setpoint temperature|lower limit|upper limit|minimum part load|maximum part load|^(cooling|heating) sequence$/i.test(name)
    || !/curve/i.test(name) && /capacity|\bcop\b|efficiency|design.*flow|reference.*flow|head|pressure rise|power/i.test(name));
  return choices.map((parameter) => {
    const raw = String(parameter.value), numeric = /^[-+]?\d+(?:\.\d+)?(?:e[-+]?\d+)?$/i.test(raw.trim());
    const unit = /temperature/i.test(parameter.name) ? " °C" : "";
    const value = numeric ? `${number(raw)}${unit}` : /^action$/i.test(parameter.name) ? raw : copy(`value.${raw}`, raw);
    return `<span class="hvac-control-value" data-control-parameter="${safe(parameter.name)}" data-control-value="${safe(raw)}" data-control-source-index="${safe(parameter.objectIndex ?? "")}" data-control-field-index="${safe(parameter.fieldIndex ?? "")}" title="${safe(`${parameter.name}: ${raw}`)}"><span class="hvac-control-value-label">${safe(parameter.name)}:</span> ${safe(value)}</span>`;
  }).join("");
}

function expressionSource(expression) {
  return sourceHTML(expression.source || expression.node?.source);
}

function renderExpression(expression = {}, depth = 0) {
  if (depth > 48) return `<span class="hvac-expression-leaf">${safe(copy("unresolved", "Unresolved control"))}</span>`;
  const op = expression.op || "unknown", children = expression.children || [];
  if (op === "all" || op === "any") {
    const operator = op === "all" ? "AND" : "OR";
    return `<span class="hvac-expression-group" data-hvac-expression-op="${op}"><span class="hvac-expression-parenthesis">(</span><span class="hvac-expression-terms">${children.map((child) => renderExpression(child, depth + 1)).join(`<span class="hvac-expression-operator">${operator}</span>`) || safe(op === "all" ? "TRUE" : "FALSE")}</span><span class="hvac-expression-parenthesis">)</span></span>`;
  }
  if (op === "reference") return `<span class="hvac-expression-reference" data-hvac-expression-op="reference" data-hvac-expression-reference="${safe(expression.referenceId || "")}">${safe(copy(expression.referenceId === "availability" ? "finalAvailability" : expression.referenceId, human(expression.referenceId)))}</span>`;
  if (op === "schedule_positive") return `<span class="hvac-expression-leaf" data-hvac-expression-op="${op}" data-hvac-expression-schedule="${safe(expression.scheduleName || "")}"><strong>${safe(expression.scheduleName || copy("unresolved", "Unresolved control"))}</strong><span>&gt; 0</span>${expressionSource(expression)}</span>`;
  if (["manager_scheduled", "manager_scheduled_on", "manager_scheduled_off"].includes(op)) {
    const condition = op === "manager_scheduled_off" ? "= 0" : "> 0";
    const yes = op === "manager_scheduled_off" ? "ForceOff" : "CycleOn", no = op === "manager_scheduled" ? "ForceOff" : "NoAction";
    return `<span class="hvac-expression-leaf rule" data-hvac-expression-op="${op}" data-hvac-expression-schedule="${safe(expression.scheduleName || "")}"><strong>${safe(expression.scheduleName || copy("unresolved", "Unresolved control"))}</strong><span>${safe(condition)} ? ${yes} : ${no}</span>${expressionSource(expression)}</span>`;
  }
  if (op === "always") {
    const literal = expression.domain === "manager" ? expression.managerStatus || copy("status.unknown", "Unknown") : expression.booleanValue === false ? "FALSE" : expression.booleanValue === true ? "TRUE" : copy("status.unknown", "Unknown");
    return `<span class="hvac-expression-leaf" data-hvac-expression-op="always">${safe(literal)}${expressionSource(expression)}</span>`;
  }
  if (children.length) {
    return `<span class="hvac-expression-group rule" data-hvac-expression-op="${safe(op)}"><span class="hvac-expression-operator">${safe(copy(`control.${op}`, human(op)))}</span><span class="hvac-expression-parenthesis">[</span><span class="hvac-expression-terms">${children.map((child) => renderExpression(child, depth + 1)).join('<span class="hvac-expression-parenthesis">;</span>')}</span><span class="hvac-expression-parenthesis">]</span>${expression.target ? `<span class="hvac-expression-rule-target">${safe(expression.target)}</span>` : ""}${expressionSource(expression)}</span>`;
  }
  const node = expression.node || {}, dependencies = expression.dependencies?.nodes || [];
  return `<span class="hvac-expression-leaf" data-hvac-expression-op="${safe(op)}"${expression.referenceId ? ` data-hvac-expression-reference="${safe(expression.referenceId)}"` : ""}${expression.reason === "on_off_expression_requires_reanalysis" ? ` title="${safe(copy("compact.reanalyze", "Analyze the input again to resolve command logic."))}"` : ""}><strong>${safe(node.code ? nodeLabel(node) : copy(`control.${op}`, human(op)))}</strong>${expressionSource(expression)}${node.parameters?.length ? `<span class="hvac-control-values">${highlights(node)}</span>` : ""}${dependencies.length ? `<span class="hvac-expression-inputs">${dependencies.map((input) => `<span class="hvac-expression-input" data-hvac-expression-input="${safe(input.id)}" title="${safe([input.source?.objectType, input.source?.objectName, nodeLabel(input)].filter(Boolean).join(" · "))}">${safe(input.source?.objectName || nodeLabel(input))}<span class="hvac-control-values">${highlights(input)}</span></span>`).join("")}</span>` : ""}</span>`;
}

function renderOnOff(definitions) {
  return `<div class="hvac-expression-stack" data-hvac-operation-graph="onoff" role="group" aria-label="${safe(copy("onOff", "On / Off decisions"))}">${definitions.map((definition) => `<article class="hvac-expression-card" data-hvac-boolean-definition="${safe(definition.id)}"><div class="hvac-expression-formula"><span class="hvac-expression-lhs">${safe(copy(definition.code, human(definition.code)))}</span><span class="hvac-expression-equals">=</span>${renderExpression(definition.expression)}</div>${sourceHTML(definition.source)}</article>`).join("")}</div>`;
}

function endpointLabel(node = {}) {
  const name = node.parameters?.find((parameter) => /node.*name/i.test(parameter.name))?.value;
  return [nodeLabel(node), name || node.source?.objectName].filter(Boolean).join(" · ");
}

function renderRelationships(card, byID) {
  const members = new Set(card.members.map((node) => node.id));
  const incoming = card.relationships.filter((edge) => members.has(edge.to) && (!members.has(edge.from) || edge.polarity));
  return incoming.length ? `<div class="hvac-parameter-relations">${incoming.map((edge) => {
    const sign = edge.polarity === "positive" ? "+" : edge.polarity === "negative" ? "−" : edge.polarity === "unknown" ? "?" : "";
    return `<div data-hvac-parameter-term data-from="${safe(edge.from)}" data-to="${safe(edge.to)}"${edge.polarity ? ` data-polarity="${safe(edge.polarity)}"` : ""}><span class="hvac-parameter-polarity">${safe(sign)}</span> ${safe(endpointLabel(byID.get(edge.from)))} <span class="hvac-parameter-relation-label">${safe(copy(`relation.${edge.relation}`, human(edge.relation)))}</span></div>`;
  }).join("")}</div>` : "";
}

function renderMember(node, card, calendar, model, byID) {
  const kind = nodeKind(node);
  const sameSource = node.source?.objectIndex === card.source?.objectIndex && node.source?.objectType === card.source?.objectType && node.source?.objectName === card.source?.objectName;
  let scheduleValue = "";
  if (node.kind === "schedule" && node.scheduleTrace?.length) {
    let minimum = Infinity, maximum = -Infinity, unknown = false;
    for (const segment of node.scheduleTrace) { if (segment.value === null) unknown = true; else { minimum = Math.min(minimum, segment.value); maximum = Math.max(maximum, segment.value); } }
    const targets = model.relationships.filter((edge) => edge.from === node.id).map((edge) => byID.get(edge.to)).filter(Boolean);
    const quantities = [...card.members, ...targets].flatMap((member) => (member.parameters || []).filter((parameter) => /control variable/i.test(parameter.name)).map((parameter) => parameter.value));
    const temperature = quantities.length > 0 && quantities.every((quantity) => /^temperature$/i.test(quantity));
    const basis = t("hvac.operation.compact.scheduleBasis", { date: `${calendar.year}-${String(calendar.month).padStart(2, "0")}-${String(calendar.day).padStart(2, "0")}`, dayType: copy(`dayType.${calendar.dayType}`, calendar.dayType) });
    scheduleValue = `<span class="hvac-control-schedule-value" title="${safe(basis)}">${safe(unknown ? copy("status.unknown", "Unknown") : (minimum === maximum ? number(minimum) : `${number(minimum)}–${number(maximum)}`) + (temperature ? " °C" : ""))}</span>`;
  }
  return `<div class="hvac-parameter-member" data-hvac-control-node="${safe(node.id)}" data-hvac-node-kind="${safe(node.kind)}" data-hvac-source-index="${safe(node.source?.objectIndex ?? "")}" data-hvac-source-type="${safe(node.source?.objectType || "")}" data-hvac-source-name="${safe(node.source?.objectName || "")}"${node.boundary ? ` data-hvac-node-boundary="${safe(node.boundary)}"` : ""}><span class="hvac-parameter-member-kind">${safe(copy(`kind.${kind}`, human(kind)))}</span><div>${safe(nodeLabel(node))}</div><div class="hvac-control-values">${highlights(node)}</div>${sameSource ? "" : sourceHTML(node.source, "hvac-control-source")}${scheduleValue}</div>`;
}

function renderParameters(model, calendar) {
  const byID = new Map(model.nodes.map((node) => [node.id, node]));
  return `<div data-hvac-operation-graph="parameters" role="group" aria-label="${safe(copy("parameterControl", "Parameter control"))}"><div class="hvac-parameter-groups">${model.groups.map((group) => `<section class="hvac-parameter-group ${safe(group.id)}" data-hvac-parameter-group="${safe(group.id)}"><h5>${safe(copy(group.titleCode, human(group.id)))}</h5>${group.cards.map((card) => `<article class="hvac-parameter-source-card" data-hvac-parameter-card="${safe(card.id)}"><h6>${safe(card.source?.objectName || card.source?.objectType || copy("unresolved", "Unresolved control"))}</h6>${card.source?.objectType ? `<span class="hvac-control-object-type">${safe(card.source.objectType)}</span>` : ""}<div class="hvac-parameter-members">${card.members.map((node) => renderMember(node, card, calendar, model, byID)).join("")}</div>${renderRelationships(card, byID)}</article>`).join("")}</section>`).join("")}</div><ul class="sr-only">${model.relationships.map((edge) => `<li data-hvac-control-edge data-from="${safe(edge.from)}" data-to="${safe(edge.to)}" data-relation="${safe(edge.relation)}"${edge.polarity ? ` data-polarity="${safe(edge.polarity)}"` : ""}>${safe(`${endpointLabel(byID.get(edge.from))} → ${endpointLabel(byID.get(edge.to))}: ${copy(`relation.${edge.relation}`, human(edge.relation))}`)}</li>`).join("")}</ul></div>`;
}

export function renderHVACControlExpressions(profile, component, calendar) {
  const model = buildHVACControlExpressions(profile || {}, { component });
  const unavailable = `<p class="hvac-operation-unresolved" data-hvac-graph-unavailable>${safe(copy("graphUnavailable", "This graph is not available in this report. Analyze the input again to resolve its source definitions."))}</p>`;
  return { onOff: model.onOff?.length ? renderOnOff(model.onOff) : unavailable, parameters: model.parameters?.nodes?.length ? renderParameters(model.parameters, calendar) : unavailable };
}
