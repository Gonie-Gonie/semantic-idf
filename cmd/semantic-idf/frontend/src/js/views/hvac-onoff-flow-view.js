import { escapeHTML as safe } from "../state.js";
import { t } from "../i18n.js";

const copy = (key, fallback) => t(`hvac.operation.${key}`, {}, fallback);
const human = (value) => String(value || "").replaceAll("_", " ");
const sourceKey = (source = {}) => JSON.stringify([source.objectIndex, source.objectType, source.objectName]);
const nodeLabel = (node = {}) => copy(`control.${node.code || node.kind || "unknown"}`, human(node.code || node.kind));

function managerDefinitionIssues(profile) {
  const issues = new Map(), seen = new Set(), pending = [profile.configuredAvailability, ...(profile.configuredAvailabilityLayers || []).map((layer) => layer.expression)];
  let remaining = 4096;
  while (pending.length && remaining-- > 0) {
    const expression = pending.pop();
    if (!expression || seen.has(expression)) continue;
    seen.add(expression);
    if (expression.op === "manager_unknown" && expression.source && expression.reason && expression.reason !== "runtime_manager") {
      const key = `${sourceKey(expression.source)}:${sourceKey(expression.assignmentOwner)}`;
      const previous = issues.get(key);
      issues.set(key, previous && previous !== expression.reason ? "invalid_runtime_manager_definition" : expression.reason);
    }
    for (const child of expression.children || []) { if (pending.length >= remaining) break; pending.push(child); }
  }
  return issues;
}

function conditionDescription(expression, issues = new Map(), hasConfiguredReport = true) {
  let reason = expression.reason || "";
  if (expression.op === "passive_flow") return { reason, label: copy("passiveTransfer", "Passive transfer"), detail: copy("conditionDetail.passive", "This passive component follows the connected flow and has no independent ON/OFF command.") };
  if (reason === "runtime_manager") reason = expression.source ? issues.get(`${sourceKey(expression.source)}:${sourceKey(expression.assignmentOwner)}`) || reason : "unresolved_manager_source";
  if (reason === "runtime_manager" && !hasConfiguredReport) reason = "manager_definition_requires_reanalysis";
  const category = ["runtime_dispatch", "runtime_manager", "requires_runtime_control_decision", "airflow_path_requires_runtime", "on_off_expression_requires_reanalysis"].includes(reason) ? reason
    : reason === "manager_definition_requires_reanalysis" ? "on_off_expression_requires_reanalysis"
    : ["unsupported_expression", "unsupported_manager_type", "unresolved_control"].includes(reason) ? "unsupported"
    : ["expression_truncated", "unresolved_expression"].includes(reason) ? "incomplete"
    : reason.startsWith("missing_") || /^unresolved_.*(?:source|assignment|membership)$/.test(reason) || ["unresolved_control_reference", "multiple_parent_paths"].includes(reason) ? "reference"
    : reason.startsWith("invalid_") || ["empty_manager_list", "unresolved_availability_schema"].includes(reason) ? "definition" : "unresolved";
  return { reason, label: copy(`condition.${category}`, copy("control.unknown", "Condition unresolved")), detail: copy(`conditionDetail.${reason === "manager_definition_requires_reanalysis" ? "manager_reanalysis" : category}`, copy("unresolved", "Unresolved control")) };
}

function referencedConditions(formulas) {
  const conditions = new Map(), pending = formulas.map((formula) => formula.expression), seen = new Set();
  let remaining = 4096;
  while (pending.length && remaining-- > 0) {
    const expression = pending.pop();
    if (!expression || seen.has(expression)) continue;
    seen.add(expression);
    if (expression.node && (!conditions.has(expression.node.id) || expression.op === "unknown")) conditions.set(expression.node.id, expression);
    for (const child of expression.children || []) { if (pending.length >= remaining) break; pending.push(child); }
  }
  return conditions;
}

function sourceHTML(source) {
  return source ? `<span class="hvac-expression-source" data-hvac-expression-source-index="${safe(source.objectIndex ?? "")}" data-hvac-expression-source-type="${safe(source.objectType || "")}">${safe(source.objectName || "")}<span class="hvac-control-object-type">${safe(source.objectType || "")}</span></span>` : "";
}
function parametersHTML(parameters = []) {
  return `<dl class="hvac-inspector-fields">${parameters.map((parameter) => `<div data-control-parameter="${safe(parameter.name)}" data-control-value="${safe(parameter.value)}" data-control-source-index="${safe(parameter.objectIndex ?? "")}" data-control-field-index="${safe(parameter.fieldIndex ?? "")}"><dt>${safe(parameter.name)}</dt><dd>${safe(parameter.value)}</dd></div>`).join("")}</dl>`;
}
function expressionHTML(expression = {}, labels = new Map(), depth = 0, describe = conditionDescription) {
  if (depth > 48) return safe(copy("unresolved", "Unresolved"));
  const op = expression.op || "unknown", children = expression.children || [];
  if (op === "all" || op === "any") {
    if (children.length === 1) return expressionHTML(children[0], labels, depth + 1, describe);
    return `<span class="hvac-expression-group" data-hvac-expression-op="${op}"><span class="hvac-expression-terms">${children.map((child) => expressionHTML(child, labels, depth + 1, describe)).join(`<span class="hvac-expression-operator">${op === "all" ? "AND" : "OR"}</span>`) || safe(op === "all" ? "TRUE" : "FALSE")}</span></span>`;
  }
  if (op === "rule_reference") return `<button type="button" class="hvac-expression-reference" data-hvac-rule-reference="${safe(expression.referenceId)}" data-hvac-inspect-rule="${safe(expression.referenceId)}">${safe(labels.get(expression.referenceId) || expression.referenceId)}</button>`;
  if (op === "reference") return `<span class="hvac-expression-reference" data-hvac-expression-reference="${safe(expression.referenceId)}">${safe(labels.get(expression.referenceId) || copy(expression.referenceId === "availability" ? "runPermission" : expression.referenceId, human(expression.referenceId)))}</span>`;
  if (op === "schedule_positive") return `<span class="hvac-expression-leaf" data-hvac-expression-op="${op}" data-hvac-expression-schedule="${safe(expression.scheduleName || "")}" title="${safe(expression.source?.objectName || "")}">${safe(expression.scheduleName || copy("unresolved", "Unresolved"))} &gt; 0</span>`;
  if (op === "always") return `<span class="hvac-expression-leaf" data-hvac-expression-op="always">${safe(expression.domain === "manager" ? expression.managerStatus : expression.booleanValue === false ? "FALSE" : "TRUE")}</span>`;
  const node = expression.node, diagnostic = describe(expression), label = node && ["runtime_condition", "passive_flow"].includes(op) ? nodeLabel(node) : diagnostic.label;
  return `<span class="hvac-expression-leaf" data-hvac-expression-op="${safe(op)}" data-hvac-condition-reason="${safe(diagnostic.reason)}" title="${safe(diagnostic.detail)}">${node ? `<button type="button" data-hvac-inspect-condition="${safe(node.id)}" data-hvac-expression-reference="${safe(node.id)}">${safe(label)}</button>` : safe(label)}</span>`;
}
function managerLeaves(expression, result = [], depth = 0) {
  if (!expression || depth > 48) return result;
  if (["air_managers", "plant_managers"].includes(expression.op)) for (const child of expression.children || []) managerLeaves(child, result, depth + 1);
  else result.push(expression);
  return result;
}
function managerHTML(expression = {}, graphNodes = [], depth = 0, describe = conditionDescription) {
  if (depth > 48) return safe(copy("unresolved", "Unresolved"));
  if (["air_managers", "plant_managers"].includes(expression.op)) {
    const children = expression.children || [], priority = expression.op === "plant_managers" ? copy("flow.plantPriority", "First override in the listed order.") : copy("flow.airPriority", "Priority: ForceOff → Zone fans only → CycleOn → NoAction.");
    return `<section class="hvac-manager-definition">${sourceHTML(expression.source)}${expression.assignmentOwner ? `<p>${safe(copy("flow.assignedTo", "Assigned to"))}: ${safe(expression.assignmentOwner.objectName)}</p>` : ""}${children.length > 1 ? `<p>${safe(priority)}</p>` : ""}${children.map((child) => managerHTML(child, graphNodes, depth + 1, describe)).join("")}</section>`;
  }
  const node = graphNodes.find((candidate) => sourceKey(candidate.source) === sourceKey(expression.source) && candidate.parameters?.length);
  let body;
  if (["manager_scheduled", "manager_scheduled_on", "manager_scheduled_off"].includes(expression.op)) {
    const positive = expression.op !== "manager_scheduled_off", yes = positive ? "CycleOn" : "ForceOff", no = expression.op === "manager_scheduled" ? "ForceOff" : "NoAction";
    body = `<p>${safe(expression.scheduleName)} ${positive ? "&gt; 0" : "= 0"} → ${yes}; ${safe(copy("flow.otherwise", "Otherwise"))} → ${no}</p>`;
  } else if (expression.op === "always") body = `<p>${safe(expression.managerStatus)}</p>`;
  else { const diagnostic = describe(expression); body = `<p class="hvac-inspector-note" data-hvac-condition-reason="${safe(diagnostic.reason)}"><strong>${safe(diagnostic.label)}</strong><br>${safe(diagnostic.detail)}</p>`; }
  return `<section class="hvac-manager-definition">${sourceHTML(expression.source)}${body}${parametersHTML(node?.parameters || [])}</section>`;
}
function permissionHTML(rule, labels) {
  const allowed = copy("allowed", "Allowed"), blocked = copy("blocked", "Blocked");
  const zoneResult = rule.target === "zone_fan" ? allowed : rule.target === "central_fan" ? blocked : copy("conditional", "Conditional / unknown");
  const rows = [["ForceOff", copy("flow.forceOff", "Stop requested"), blocked], ["CycleOn", copy("flow.cycleOn", "Run requested"), allowed], ["CycleOnZoneFansOnly", copy("flow.zoneFansOnly", "Zone fans only"), zoneResult], ["NoAction", copy("flow.noAction", "No override"), expressionHTML(rule.otherwise, labels)]];
  return `<p>${safe(labels.get(rule.managerId))} · ${safe(copy(`flow.${rule.target}`, ""))}</p><table class="hvac-permission-table"><thead><tr><th>${safe(copy("flow.managerAction", "Manager action"))}</th><th>${safe(copy("flow.permission", "Permission"))}</th></tr></thead><tbody>${rows.map(([status, label, result]) => `<tr data-hvac-permission-status="${status}"><th>${safe(label)}<small>${status}</small></th><td>${status === "NoAction" ? result : safe(result)}</td></tr>`).join("")}</tbody></table>${sourceHTML(rule.source)}`;
}

export function renderHVACOnOffFlow(presentation, profile) {
  const rules = presentation.rules || [], managers = rules.filter((rule) => rule.kind === "manager"), permissions = rules.filter((rule) => rule.kind === "permission"), labels = new Map();
  for (const formula of presentation.formulas) labels.set(formula.id, copy(formula.code, human(formula.code)));
  const issues = managerDefinitionIssues(profile), hasConfiguredReport = Boolean(profile.configuredAvailability || profile.configuredAvailabilityLayers), describe = (expression) => conditionDescription(expression, issues, hasConfiguredReport);
  for (const [kind, items] of [["manager", managers], ["permission", permissions]]) items.forEach((rule, index) => labels.set(rule.id, `${copy(kind === "manager" ? "flow.managerAction" : rule.op === "fan_gate" ? "flow.fanPermission" : "flow.managerPermission", kind === "manager" ? "Manager action" : rule.op === "fan_gate" ? "Fan permission" : "Manager permission")}${items.length > 1 ? ` ${index + 1}` : ""}`));
  const tile = (rule) => {
    const leaves = rule.kind === "manager" ? managerLeaves(rule.expression) : [], leaf = leaves.length === 1 ? leaves[0] : null;
    const main = rule.kind === "manager" ? leaf?.source?.objectType?.replace(/^AvailabilityManager:/i, "") || labels.get(rule.id) : labels.get(rule.id);
    const secondary = rule.kind === "manager" ? leaf?.source?.objectName || rule.expression.source?.objectName || "" : rule.otherwise.op === "schedule_positive" ? `${rule.otherwise.scheduleName} > 0` : copy("flow.managerOverride", "Manager override");
    return `<button type="button" class="hvac-flow-tile ${rule.kind}" data-hvac-inspect-rule="${safe(rule.id)}" ${rule.kind === "manager" ? "data-hvac-manager-rule" : "data-hvac-permission-rule"}="${safe(rule.id)}"${rule.managerId ? ` data-hvac-manager-reference="${safe(rule.managerId)}"` : ""} title="${safe(secondary)}"><span>${safe(main)}</span>${secondary ? `<small>${safe(secondary)}</small>` : ""}</button>`;
  };
  const diagrams = rules.length ? `<div class="hvac-onoff-rule-flow"><svg aria-hidden="true" class="hvac-flow-connectors"></svg><div class="hvac-onoff-rule-columns"><div>${managers.map(tile).join("")}</div><div>${permissions.map(tile).join("")}</div></div></div>` : "";
  const formulas = presentation.formulas.map((definition) => `<article class="hvac-expression-card" data-hvac-boolean-definition="${safe(definition.id)}"><div class="hvac-expression-formula"><span class="hvac-expression-lhs">${safe(copy(definition.code, human(definition.code)))}</span><span class="hvac-expression-equals">=</span>${expressionHTML(definition.expression, labels, 0, describe)}</div></article>`).join("");
  const templates = rules.map((rule) => `<template data-hvac-inspector-template="${safe(rule.id)}"><h5>${safe(labels.get(rule.id))}</h5>${rule.kind === "manager" ? managerHTML(rule.expression, profile.onOff?.nodes, 0, describe) : permissionHTML(rule, labels)}</template>`);
  const conditions = referencedConditions(presentation.formulas);
  for (const node of profile.onOff?.nodes || []) {
    const condition = conditions.get(node.id), diagnostic = condition ? describe(condition) : null, title = condition?.op === "unknown" ? diagnostic.label : nodeLabel(node);
    const inputs = (profile.onOff?.edges || []).filter((edge) => edge.to === node.id && edge.from !== "permission").map((edge) => (profile.onOff.nodes || []).find((input) => input.id === edge.from)).filter(Boolean);
    templates.push(`<template data-hvac-condition-template="${safe(node.id)}"><h5>${safe(title)}</h5>${diagnostic ? `<p class="hvac-inspector-note" data-hvac-condition-reason="${safe(diagnostic.reason)}">${safe(diagnostic.detail)}</p>` : ""}${sourceHTML(node.source)}${parametersHTML(node.parameters)}${inputs.length ? `<h6>${safe(copy("flow.inputs", "Inputs"))}</h6>${inputs.map((input) => `<section class="hvac-manager-definition" data-hvac-expression-input="${safe(input.id)}"><strong>${safe(nodeLabel(input))}</strong>${sourceHTML(input.source)}${parametersHTML(input.parameters)}</section>`).join("")}` : ""}</template>`);
  }
  return `<div class="hvac-expression-stack" data-hvac-operation-graph="onoff" role="group" aria-label="${safe(copy("onOff", "On / Off decisions"))}">${diagrams}${formulas}<div class="hvac-control-inspector" data-hvac-control-inspector hidden><button type="button" class="hvac-inspector-close" data-hvac-close-inspector aria-label="${safe(copy("flow.closeDetails", "Close details"))}">×</button><div data-hvac-inspector-content></div></div>${templates.join("")}</div>`;
}

export function connectHVACOnOffFlow(container) {
  const onClick = (event) => {
    const target = event.target.closest("[data-hvac-inspect-rule],[data-hvac-inspect-condition],[data-hvac-close-inspector]");
    if (!target || !container.contains(target)) return;
    const panel = target.closest('[data-hvac-operation-graph="onoff"]'), inspector = panel?.querySelector("[data-hvac-control-inspector]");
    if (!inspector) return;
    if (target.hasAttribute("data-hvac-close-inspector")) { const selected = panel.querySelector('[aria-pressed="true"]'); inspector.hidden = true; panel.querySelectorAll('[aria-pressed="true"]').forEach((button) => button.setAttribute("aria-pressed", "false")); selected?.focus(); return; }
    const value = target.dataset.hvacInspectRule || target.dataset.hvacInspectCondition, attribute = target.dataset.hvacInspectRule ? "data-hvac-inspector-template" : "data-hvac-condition-template";
    const template = [...panel.querySelectorAll(`template[${attribute}]`)].find((item) => item.getAttribute(attribute) === value);
    if (!template) return;
    inspector.querySelector("[data-hvac-inspector-content]").replaceChildren(template.content.cloneNode(true)); inspector.hidden = false;
    panel.querySelectorAll('[aria-pressed="true"]').forEach((button) => button.setAttribute("aria-pressed", "false")); target.setAttribute("aria-pressed", "true");
  };
  const draw = () => {
    for (const flow of container.querySelectorAll(".hvac-onoff-rule-flow")) {
      const svg = flow.querySelector("svg"), bounds = flow.getBoundingClientRect();
      svg.setAttribute("viewBox", `0 0 ${bounds.width} ${bounds.height}`);
      svg.innerHTML = [...flow.querySelectorAll("[data-hvac-manager-reference]")].map((button) => {
        const manager = [...flow.querySelectorAll("[data-hvac-manager-rule]")].find((item) => item.dataset.hvacManagerRule === button.dataset.hvacManagerReference);
        if (!manager) return "";
        const from = manager.getBoundingClientRect(), to = button.getBoundingClientRect(), x1 = from.right - bounds.left, x2 = to.left - bounds.left, y1 = from.top + from.height / 2 - bounds.top, y2 = to.top + to.height / 2 - bounds.top;
        return `<path d="M ${x1} ${y1} C ${(x1 + x2) / 2} ${y1}, ${(x1 + x2) / 2} ${y2}, ${x2 - 7} ${y2}"/><path class="hvac-flow-arrow" d="M ${x2 - 7} ${y2 - 4} L ${x2} ${y2} L ${x2 - 7} ${y2 + 4} Z"/>`;
      }).join("");
    }
  };
  container.addEventListener("click", onClick);
  const observer = new ResizeObserver(draw); observer.observe(container); draw();
  return () => { container.removeEventListener("click", onClick); observer.disconnect(); };
}
