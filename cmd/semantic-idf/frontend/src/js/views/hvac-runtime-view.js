import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";
import { prepareHVACRuntimeInspection, hvacRuntimeSnapshot, hvacRuntimeTrace, hvacRuntimeSignalState, hvacRuntimePermission } from "../hvac-runtime-data.js";
import { buildHVACControlExpressions } from "../hvac-control-expressions.js";
import { renderHVACParameterFlow, connectHVACParameterFlow } from "./hvac-parameter-flow-view.js";
import { renderHVACInspectionChart, updateHVACInspectionChartFrame } from "../hvac-inspection-charts.js";

const safe = escapeHTML, finite = (value) => typeof value === "number" && Number.isFinite(value);
const copy = (key, fallback) => t(`simulation.hvacRuntime${key}`, {}, fallback);
const connections = new WeakMap(), flowModels = new WeakMap();
const stateLabels = { allowed: "Allowed", blocked: "Blocked", mixed: "Interval average / mixed", no_action: "NoAction", force_off: "ForceOff", cycle_on: "CycleOn", zone_fans_only: "Zone fans only", positive: "Positive", nonpositive: "Zero / nonpositive", active: "Activity reported", inactive: "No activity reported", hybrid_idle: "No hybrid control", natural_allowed: "Natural ventilation allowed", natural_blocked: "Natural ventilation blocked", error: "Data acquisition error" };
const roleLabels = { schedule: "Schedule", manager_status: "AvailabilityManager", loop_status: "Loop availability", sensor: "Sensed value", setpoint: "Setpoint", actuator: "Actual actuator value", actuator_request: "Requested actuator value", demand: "Reported demand", activity: "Equipment response" };
const reasonLabels = { acquisition_error: "Required control outputs are missing. Inspect the acquisition errors below.", permission_blocked: "The reported availability inputs block operation during this interval.", permission_activity_conflict: "Availability and reported activity disagree in this reporting interval. Compare overrides and interval averages below.", reported_activity: "Equipment activity was reported. The availability and control inputs below show the operating context.", zero_reported_demand: "No activity and zero demand were reported. Check the demand source and control values below.", reported_inactivity: "No equipment activity was reported. Inspect availability, demand and controller values below." };
const stateLabel = (state) => copy(`State.${state}`, stateLabels[state] || state);
const otherReasons = { passive_flow: "Passive component: inspect reported flow and boundary conditions. It has no independent ON/OFF command.", reported_controls: "The collected control inputs and actuator values are shown below." };
const roleLabel = (role) => copy(`Role.${role}`, roleLabels[role] || role);
const number = (value, unit = "") => finite(value) ? `${value.toLocaleString(undefined, { minimumFractionDigits: unit === "kW" ? 2 : 0, maximumFractionDigits: unit === "kg/s" ? 4 : 2 })}${unit && unit !== "—" ? ` ${unit}` : ""}` : copy("DataError", "Data acquisition error");
const identity = (signal) => [signal.role, signal.source?.objectIndex, signal.keyValue, signal.variableName, signal.reportingFrequency].join("\0");
const distinct = (signals) => [...new Map(signals.map((signal) => [identity(signal), signal])).values()];

function normalize(model, ui) {
  ui.logic ||= {};
  const logic = ui.logic;
  if (!model.profiles.some((profile) => profile.id === logic.profileID)) logic.profileID = model.profiles[0]?.id || "";
  logic.frameIndex = Math.max(0, Math.min(Math.max(0, model.frames.length - 1), Math.round(Number(logic.frameIndex) || 0)));
  logic.window = ["day", "week", "all"].includes(logic.window) ? logic.window : "week";
  const profile = model.profiles.find((entry) => entry.id === logic.profileID);
  const available = distinct(profile?.signals || []).filter((signal) => signal.status === "collected");
  if (!logic.traces || logic.traceProfile !== logic.profileID) {
    const graph = profile?.definition.parameterControl;
    const comparison = graph?.nodes?.find((node) => node.code === "sensed_minus_setpoint");
    const incoming = (relation, role) => {
      const edge = graph?.edges?.find((entry) => entry.to === comparison?.id && entry.relation === relation);
      return available.find((signal) => signal.graph === "parameterControl" && signal.nodeId === edge?.from && signal.role === role && /temperature/i.test(signal.variableName));
    };
    const target = incoming("subtracts_target", "setpoint") || available.find((signal) => signal.graph === "parameterControl" && signal.role === "setpoint");
    const sensed = incoming("adds_measurement", "sensor") || available.find((signal) => signal.role === "sensor" && signal.keyValue === target?.keyValue && /temperature/i.test(signal.variableName));
    const activity = available.find((signal) => signal.role === "activity");
    const flow = available.find((signal) => signal.graph === "parameterControl" && signal.role === "sensor" && /mass flow rate$/i.test(signal.variableName));
    const primary = !comparison && /^(fan:|pump:|headeredpumps:)/i.test(profile?.source.objectType) ? [activity, flow] : [target, sensed];
    const fallback = ["actuator_request", "actuator", "activity", "demand", "sensor", "setpoint"].flatMap((role) => available.filter((signal) => signal.graph === "parameterControl" && signal.role === role).slice(0, 1));
    logic.traces = [...new Map([...primary, ...fallback, ...available].filter(Boolean).map((signal) => [signal.id, signal])).values()].slice(0, 2).map((signal) => signal.id);
    logic.traceProfile = logic.profileID;
  }
  logic.traces = logic.traces.filter((id) => available.some((signal) => signal.id === id)).slice(0, 4);
  return logic;
}

function windowFrames(model, logic) {
  if (!model.calendarValid || logic.window === "all" || !model.frames.length) return model.frames;
  const hours = logic.window === "day" ? 24 : 168, first = model.frames[0].x, selected = model.frames[logic.frameIndex].x;
  const low = first + Math.floor((selected - first) / hours) * hours;
  return model.frames.filter((frame) => frame.x >= low && frame.x < low + hours);
}

function errorPanel(snapshot, model) {
  if (!snapshot.errors.length && snapshot.profile.signals.length && model.frames.length) return "";
  const definitionError = snapshot.errors.some((error) => error.id === "availability-definition");
  const title = definitionError ? copy("DefinitionError", "Control definition error") : copy("AcquisitionError", "Control output acquisition failed");
  const action = definitionError ? copy("DefinitionAction", "Inspect the source definition and failed rule below. This diagnostic error is not an operating state.") : copy("AcquisitionAction", "Run HVAC Loop Check again with the required outputs. A missing value is not treated as OFF or as an operating reason.");
  return `<section class="hvac-runtime-error" data-hvac-runtime-error role="alert"><h4>${safe(title)}</h4><p>${safe(action)}</p><ul>${snapshot.errors.map((signal) => `<li data-hvac-runtime-missing="${safe(signal.id)}"><strong>${safe(signal.keyValue)} · ${safe(signal.variableName)}</strong>${signal.reportingFrequency ? ` (${safe(signal.reportingFrequency)})` : ""} — ${safe(signal.message || copy("MissingAtFrame", "No valid observation for this reporting interval"))}</li>`).join("") || `<li>${safe(copy("NoSignals", "This result contains no collected control diagnostics for the equipment."))}</li>`}</ul></section>`;
}

function renderSummary(snapshot, model) {
  const error = errorPanel(snapshot, model);
  if (error) return error;
  const cards = [];
  for (const [label, result] of [[copy("Configured", "Configured permission"), snapshot.configuredPermission], [copy("Permission", "Availability with manager actions"), snapshot.permission]]) {
    if (["allowed", "blocked", "mixed"].includes(result.state)) cards.push(`<article data-hvac-runtime-permission="${result.state}"><span>${safe(label)}</span><strong>${safe(stateLabel(result.state))}</strong></article>`);
  }
  if (snapshot.activity !== "not_collected") cards.push(`<article data-hvac-runtime-activity="${snapshot.activity}"><span>${safe(copy("Activity", "Reported equipment activity"))}</span><strong>${safe(stateLabel(snapshot.activity))}</strong></article>`);
  return `<div class="hvac-runtime-facts">${cards.join("")}</div><p class="hvac-runtime-reason" data-hvac-runtime-reason="${safe(snapshot.reason)}">${safe(copy(`Reason.${snapshot.reason}`, reasonLabels[snapshot.reason] || otherReasons[snapshot.reason]))}</p>`;
}

function renderLogicExpression(expression, snapshot, depth = 0, domain = "boolean") {
  if (!expression || depth > 48) return "";
  const op = expression.op;
  const children = (expression.children || []).filter((child) => !(op === "all" && child.op === "always" && child.value !== 0));
  if (op === "all" && !children.length) return renderLogicExpression({ op: "always" }, snapshot, depth + 1, domain);
  if (op === "all" && children.length === 1) return renderLogicExpression(children[0], snapshot, depth + 1, domain);
  if (["air_managers", "plant_managers"].includes(op) && children.length === 1 && children[0].op === op) return renderLogicExpression(children[0], snapshot, depth + 1, domain);
  const labels = { all: "AND", any: "OR", air_managers: copy("AirPriority", "Air / zone manager priority"), plant_managers: copy("PlantPriority", "First manager action"), manager_gate: copy("ManagerPermission", "Manager permission"), fan_gate: copy("FanPermission", "Fan schedule / manager override") };
  if (children.length) return `<div class="hvac-runtime-expression-group" data-hvac-runtime-operator="${safe(op)}"><strong>${safe(labels[op] || op)}</strong><div>${children.map((child, index) => renderLogicExpression(child, snapshot, depth + 1, ["air_managers", "plant_managers"].includes(op) || index === 0 && ["manager_gate", "fan_gate"].includes(op) ? "manager" : "boolean")).join("")}</div></div>`;
  let source = expression.source;
  if (!source && expression.referenceId) source = snapshot.profile.definition.onOff?.nodes?.find((node) => node.id === expression.referenceId)?.source;
  const observed = distinct(snapshot.signals.filter((signal) => op === "schedule_positive" ? signal.role === "schedule" && signal.keyValue.toLowerCase() === expression.scheduleName?.toLowerCase()
    : source && signal.source?.objectIndex === source.objectIndex && (op.startsWith("manager_") ? signal.role === "manager_status" : ["demand", "actuator_request"].includes(signal.role))));
  const result = hvacRuntimePermission(snapshot.profile, snapshot.signals, expression, domain);
  const name = expression.scheduleName || source?.objectName || (op === "always" ? domain === "manager" ? stateLabel(["no_action", "force_off", "cycle_on", "zone_fans_only"][expression.value || 0]) : copy("Unrestricted", "No additional restriction") : copy("RuntimeInput", "Runtime decision input"));
  return `<article class="hvac-runtime-expression-leaf" data-hvac-runtime-condition="${safe(op)}" data-hvac-runtime-state="${safe(result.state)}"><strong>${safe(name)}${op === "schedule_positive" ? " > 0" : ""}</strong>${observed.map((signal) => `<span>${safe(signal.role === "manager_status" ? stateLabel(hvacRuntimeSignalState(signal, signal.value)) : number(signal.value, signal.unit))}</span>`).join("")}${["allowed", "blocked", "mixed"].includes(result.state) ? `<b>${safe(stateLabel(result.state))}</b>` : ""}</article>`;
}

function renderDecision(snapshot) {
  if (snapshot.errors.length) return "";
  return `<section data-hvac-runtime-decisions><h4>${safe(copy("DecisionLogic", "Availability conditions and reported manager actions"))}</h4>${renderLogicExpression(snapshot.profile.availabilityExpression, snapshot)}</section>`;
}

function renderDispatch(snapshot) {
  const nodes = [...new Map([snapshot.profile.definition.onOff, snapshot.profile.definition.parameterControl]
    .flatMap((graph) => graph?.nodes || []).filter((node) => node.code === "plant_dispatch").map((node) => [node.source.objectIndex, node])).values()];
  if (!nodes.length) return "";
  const inputs = distinct(snapshot.signals.filter((signal) => signal.graph === "loop" && signal.role === "demand" || nodes.some((node) => node.source.objectIndex === signal.source.objectIndex)));
  return `<section class="hvac-runtime-source-detail" data-hvac-runtime-dispatch><h4>${safe(copy("Dispatch", "Plant dispatch context"))}</h4><p>${safe(copy("DispatchDetail", "Dispatch uses reported demand and the operation scheme. These inputs are separate from availability permission."))}</p>${nodes.map((node) => `<h5>${safe(node.source.objectName)} · ${safe(node.source.objectType)}</h5><dl>${(node.parameters || []).filter((field) => field.fieldIndex >= 0).map((field) => `<div><dt>${safe(field.name)}</dt><dd>${safe(field.value)}</dd></div>`).join("")}</dl>`).join("")}<dl>${inputs.map((signal) => `<div><dt>${safe(signal.keyValue)} · ${safe(signal.variableName)}</dt><dd>${safe(number(signal.value, signal.unit))}</dd></div>`).join("")}</dl></section>`;
}

function renderSourceReadings(signal, snapshot) {
  const inputs = distinct(snapshot.signals.filter((item) => item.source?.objectIndex === signal.source?.objectIndex));
  return `<h5>${safe(copy("SourceInputs", "Reported inputs"))}</h5><dl>${inputs.map((item) => `<div><dt>${safe(item.keyValue)} · ${safe(item.variableName)}</dt><dd>${safe(item.role === "manager_status" ? stateLabel(hvacRuntimeSignalState(item, item.value)) : number(item.value, item.unit))}</dd></div>`).join("")}</dl>`;
}

function renderSourceDetail(snapshot, logic) {
  const signal = snapshot.signals.find((item) => item.id === logic.sourceID);
  if (!signal) return "";
  const definition = snapshot.profile.definition;
  const nodes = [definition.availabilityDefinitions, definition.onOff, definition.parameterControl].flatMap((graph) => graph?.nodes || []);
  const sourceNodes = [...new Map(nodes.filter((node) => node.source?.objectIndex === signal.source?.objectIndex).map((node) => [node.id, node])).values()];
  const fields = [...new Map(sourceNodes.flatMap((node) => node.parameters || [])
    .filter((parameter) => parameter.fieldIndex >= 0 && parameter.objectIndex === signal.source.objectIndex)
    .map((parameter) => [`${parameter.objectIndex}:${parameter.fieldIndex}:${parameter.name}`, parameter])).values()];
  return `<section class="hvac-runtime-source-detail" data-hvac-runtime-source-detail><h4>${safe(signal.source.objectName)}</h4><span>${safe(signal.source.objectType)}</span><dl>${fields.map((field) => `<div><dt>${safe(field.name)}</dt><dd>${safe(field.value)}</dd></div>`).join("")}</dl>${renderSourceReadings(signal, snapshot)}</section>`;
}

function renderAvailability(snapshot, frames, interval = 1) {
  const rows = distinct([...snapshot.availability, ...snapshot.signals.filter((signal) => signal.role === "activity")]);
  if (!rows.length) return "";
  const low = (frames[0]?.x ?? 0) - interval, high = frames.at(-1)?.x ?? low + 1, span = high - low || 1;
  return `<section data-hvac-runtime-availability><h4>${safe(copy("Availability", "Availability and reported activity"))}</h4>${rows.map((signal) => {
    const bins = new Map();
    for (const frame of frames) {
      const start = Math.max(0, Math.floor((frame.x - interval - low) / span * 1000)), end = Math.min(1000, Math.max(start + 1, Math.ceil((frame.x - low) / span * 1000)));
      const state = hvacRuntimeSignalState(signal, signal.values.get(frame.key));
      for (let bin = start; bin < end; bin++) bins.set(bin, bins.has(bin) && bins.get(bin) !== state ? "mixed" : state);
    }
    const ranges = [];
    for (const [bin, state] of bins) {
      const last = ranges.at(-1);
      if (last && last.state === state && bin === last.end) last.end++;
      else ranges.push({ start: bin, end: bin + 1, state });
    }
    const state = hvacRuntimeSignalState(signal, signal.value);
    return `<div class="hvac-runtime-status-row"><div><button type="button" class="hvac-runtime-source-button" data-hvac-runtime-source="${safe(signal.id)}">${safe(signal.source?.objectName || signal.keyValue)}</button><small>${safe(signal.keyValue)} · ${safe(signal.variableName)}</small><span data-hvac-runtime-current="${safe(signal.id)}">${safe(stateLabel(state))} · ${safe(number(signal.value, signal.unit))}</span></div><svg viewBox="0 0 1000 28" preserveAspectRatio="none" role="img" aria-label="${safe(signal.variableName)}" data-hvac-runtime-seek><title>${safe(copy("Seek", "Choose an observed interval"))}</title>${ranges.map((range) => `<rect class="hvac-runtime-patch is-${range.state}" x="${range.start}" y="4" width="${Math.max(1, range.end - range.start)}" height="20"><title>${safe(stateLabel(range.state))}</title></rect>`).join("")}<line data-hvac-runtime-cursor x1="0" x2="0" y1="0" y2="28"/></svg></div>`;
  }).join("")}<div class="hvac-runtime-axis"><span>${safe(frames[0]?.label || "")}</span><span>${safe(frames.at(-1)?.label || "")}</span></div></section>`;
}

function controlModel(profile) {
  if (!flowModels.has(profile)) flowModels.set(profile, buildHVACControlExpressions(profile.definition, { component: profile.source }).parameters);
  return flowModels.get(profile);
}

function renderControl(snapshot) {
  return `<section class="hvac-runtime-logic parameters" data-hvac-runtime-control><h4>${safe(copy("Control", "Parameter control"))}</h4>${renderHVACParameterFlow(controlModel(snapshot.profile), { component: snapshot.profile.source })}<div class="hvac-runtime-residuals">${snapshot.residuals.map((entry) => `<article data-hvac-runtime-residual="${safe(entry.id)}"><span>${safe(entry.source.objectName)}</span><strong>${safe(number(entry.sensor.value, entry.unit))} − ${safe(number(entry.setpoint.value, entry.unit))} = ${safe(number(entry.value, entry.unit))}</strong></article>`).join("")}</div></section>`;
}

function renderSignals(snapshot, logic) {
  const signals = distinct(snapshot.signals), units = new Set(signals.filter((signal) => logic.traces.includes(signal.id)).map((signal) => signal.unit));
  return `<section class="hvac-runtime-signals"><h4>${safe(copy("Signals", "Reported control values"))}</h4><p>${safe(copy("ChooseTraces", "Select up to four values and two units for the time-series graph."))}</p><div class="hvac-runtime-table"><table><thead><tr><th>${safe(copy("Plot", "Plot"))}</th><th>${safe(copy("Role", "Role"))}</th><th>${safe(copy("Source", "Source / output"))}</th><th>${safe(copy("Value", "Value"))}</th></tr></thead><tbody>${signals.map((signal) => {
    const checked = logic.traces.includes(signal.id), disabled = signal.status !== "collected" || !checked && (logic.traces.length >= 4 || units.size >= 2 && !units.has(signal.unit));
    return `<tr data-hvac-runtime-signal="${safe(signal.id)}"><td><input type="checkbox" data-hvac-runtime-trace="${safe(signal.id)}" aria-label="${safe(`${signal.source?.objectName || signal.keyValue} · ${signal.variableName}`)}"${checked ? " checked" : ""}${disabled ? " disabled" : ""}/></td><td>${safe(roleLabel(signal.role))}</td><td><strong>${safe(signal.source?.objectName || signal.keyValue)}</strong><small>${safe(signal.keyValue)} · ${safe(signal.variableName)}${signal.parameterName ? ` · ${safe(signal.parameterName)}` : ""}</small></td><td data-hvac-runtime-value="${safe(signal.id)}" title="${safe(String(signal.value))}">${safe(number(signal.value, signal.unit))}</td></tr>`;
  }).join("")}</tbody></table></div></section>`;
}

function renderHistory(model, snapshot, logic, frames) {
  return renderHVACInspectionChart({ title: copy("History", "Control time series"), frameKey: snapshot.frame?.x,
    series: distinct(snapshot.profile.signals).filter((signal) => logic.traces.includes(signal.id)).map((signal) => ({ ...hvacRuntimeTrace(model, signal, frames), axisLabel: roleLabel(signal.role) })) });
}

export function renderHVACRuntimeInspection(loop, ui) {
  const model = prepareHVACRuntimeInspection(loop), logic = normalize(model, ui);
  if (!model.profiles.length) return `<section class="hvac-runtime" data-hvac-runtime><div class="empty" data-hvac-runtime-error>${safe(copy("Rerun", "This saved result has no executed control diagnostics. Run HVAC Loop Check again to collect them."))}</div></section>`;
  const snapshot = hvacRuntimeSnapshot(model, logic.profileID, logic.frameIndex), frames = windowFrames(model, logic);
  return `<section class="hvac-runtime" data-hvac-runtime data-window="${safe(`${frames[0]?.key}|${frames.at(-1)?.key}`)}">
    <div class="hvac-runtime-toolbar"><label><span>${safe(copy("Equipment", "Equipment"))}</span><select data-hvac-runtime-profile>${model.profiles.map((profile) => `<option value="${safe(profile.id)}"${profile.id === logic.profileID ? " selected" : ""}>${safe(profile.source.objectName || profile.id)} · ${safe(profile.source.objectType || "")}</option>`).join("")}</select></label><label><span>${safe(copy("Window", "Time window"))}</span><select data-hvac-runtime-window>${["day", "week", "all"].map((value) => `<option value="${value}"${logic.window === value ? " selected" : ""}>${safe(copy(`Window.${value}`, { day: "Day", week: "Week", all: "Full run" }[value]))}</option>`).join("")}</select></label></div>
    <p class="hvac-runtime-cadence" title="${safe(copy("IntervalDetail", "Values summarize the reported interval, not solver iterations or the order of events within it."))}">${safe(copy("Cadence", "Timestep · reported interval values"))}</p>
    <div class="hvac-inspection-frame-control"><button type="button" data-hvac-runtime-step="-1" aria-label="${safe(copy("Previous", "Previous observation"))}"${logic.frameIndex ? "" : " disabled"}>‹</button><input data-hvac-runtime-frame type="range" min="0" max="${Math.max(0, model.frames.length - 1)}" step="1" value="${logic.frameIndex}" aria-label="${safe(copy("Frame", "Control observation"))}"${model.frames.length ? "" : " disabled"}/><button type="button" data-hvac-runtime-step="1" aria-label="${safe(copy("Next", "Next observation"))}"${logic.frameIndex < model.frames.length - 1 ? "" : " disabled"}>›</button><output data-hvac-runtime-frame-label>${safe(snapshot.frame?.label || copy("NoFrame", "No collected observations"))}</output></div>
    <div data-hvac-runtime-summary>${renderSummary(snapshot, model)}</div>
    ${renderAvailability(snapshot, frames, model.interval)}<div data-hvac-runtime-source-wrap>${renderSourceDetail(snapshot, logic)}</div><div data-hvac-runtime-decision-wrap>${renderDecision(snapshot)}</div><div data-hvac-runtime-dispatch-wrap>${renderDispatch(snapshot)}</div>${renderControl(snapshot)}
    <div data-hvac-runtime-signal-list>${renderSignals(snapshot, logic)}</div><div data-hvac-runtime-history>${renderHistory(model, snapshot, logic, frames)}</div>
  </section>`;
}

function updateValues(container, model, snapshot, logic, frames) {
  const mount = container.querySelector("[data-hvac-runtime]");
  if (!mount || !snapshot.profile) return;
  mount.querySelector("[data-hvac-runtime-summary]").innerHTML = renderSummary(snapshot, model);
  mount.querySelector("[data-hvac-runtime-decision-wrap]").innerHTML = renderDecision(snapshot);
  mount.querySelector("[data-hvac-runtime-dispatch-wrap]").innerHTML = renderDispatch(snapshot);
  mount.querySelector("[data-hvac-runtime-source-wrap]").innerHTML = renderSourceDetail(snapshot, logic);
  for (const button of mount.querySelectorAll("[data-hvac-runtime-source]")) button.setAttribute("aria-pressed", String(button.dataset.hvacRuntimeSource === logic.sourceID));
  mount.querySelector("[data-hvac-runtime-frame-label]").textContent = snapshot.frame?.label || copy("NoFrame", "No collected observations");
  mount.querySelector("[data-hvac-runtime-frame]").value = logic.frameIndex;
  mount.querySelector('[data-hvac-runtime-step="-1"]').disabled = !logic.frameIndex;
  mount.querySelector('[data-hvac-runtime-step="1"]').disabled = logic.frameIndex >= model.frames.length - 1;
  for (const signal of distinct(snapshot.signals)) {
    const cell = mount.querySelector(`[data-hvac-runtime-value="${CSS.escape(signal.id)}"]`);
    if (cell) { cell.textContent = number(signal.value, signal.unit); cell.title = String(signal.value); }
    const current = mount.querySelector(`[data-hvac-runtime-current="${CSS.escape(signal.id)}"]`);
    if (current) current.textContent = finite(signal.value) ? `${stateLabel(hvacRuntimeSignalState(signal, signal.value))} · ${number(signal.value, signal.unit)}` : copy("DataError", "Data acquisition error");
  }
  const low = (frames[0]?.x ?? 0) - (model.interval || 1), span = (frames.at(-1)?.x ?? low + 1) - low || 1;
  const x = Math.max(0, Math.min(1000, ((snapshot.frame?.x || low) - low) / span * 1000));
  for (const cursor of mount.querySelectorAll("[data-hvac-runtime-cursor]")) { cursor.setAttribute("x1", x); cursor.setAttribute("x2", x); }
  updateFlowValues(mount, snapshot);
  updateHVACInspectionChartFrame(mount, snapshot.frame?.x);
}

function updateFlowValues(mount, snapshot) {
  const cards = controlModel(snapshot.profile).groups.flatMap((group) => group.cards.map((card) => ({ ...card, group: group.id })));
  for (const tile of mount.querySelectorAll("[data-hvac-flow-card]")) {
    const card = cards.find((entry) => entry.id === tile.dataset.hvacFlowCard);
    if (!card) continue;
    const roles = { inputs: ["setpoint", "schedule", "sensor", "demand"], controls: ["sensor", "setpoint"], commands: ["actuator_request", "actuator"], response: ["activity", "actuator"] }[card.group];
    const ids = new Set(card.members.map((node) => node.id));
    const candidates = snapshot.signals.filter((signal) => signal.graph === "parameterControl" && ids.has(signal.nodeId));
    const pick = roles.map((role) => candidates.find((signal) => signal.role === role)).find(Boolean)
      || (card.group === "response" ? roles.map((role) => snapshot.signals.find((signal) => signal.role === role && signal.source?.objectIndex === card.source?.objectIndex)).find(Boolean) : null);
    const residual = snapshot.residuals.find((entry) => ids.has(entry.id));
    if (!pick && !residual) continue;
    let value = tile.querySelector("[data-hvac-runtime-card-value]");
    if (!value) { value = document.createElement("span"); value.dataset.hvacRuntimeCardValue = ""; tile.querySelector("button").append(value); }
    value.textContent = residual ? `Δ ${number(residual.value, residual.unit)}` : `${card.group === "commands" ? copy(pick.role === "actuator_request" ? "Requested" : "Actual", pick.role === "actuator_request" ? "Requested" : "Actual") + ": " : ""}${number(pick.value, pick.unit)}`;
    value.title = residual ? copy("Residual", "Sensed value − setpoint") : `${roleLabel(pick.role)} · ${pick.keyValue} · ${pick.variableName}`;
    if (card.group === "commands" && pick?.role === "actuator_request") {
      const actual = candidates.find((signal) => signal.role === "actuator") || snapshot.signals.find((signal) => signal.role === "actuator" && signal.source?.objectIndex === card.source?.objectIndex);
      if (actual) {
        let actualValue = tile.querySelector("[data-hvac-runtime-card-actual]");
        if (!actualValue) { actualValue = document.createElement("span"); actualValue.dataset.hvacRuntimeCardActual = ""; tile.querySelector("button").append(actualValue); }
        actualValue.textContent = `${copy("Actual", "Actual")}: ${number(actual.value, actual.unit)}`;
        actualValue.title = `${actual.keyValue} · ${actual.variableName}`;
      }
    }
  }
  const residuals = mount.querySelector(".hvac-runtime-residuals");
  if (residuals) residuals.innerHTML = snapshot.residuals.map((entry) => `<article data-hvac-runtime-residual="${safe(entry.id)}"><span>${safe(entry.source.objectName)}</span><strong>${safe(number(entry.sensor.value, entry.unit))} − ${safe(number(entry.setpoint.value, entry.unit))} = ${safe(number(entry.value, entry.unit))}</strong></article>`).join("");
}

export function disconnectHVACRuntimeInspection(container) {
  connections.get(container)?.(); connections.delete(container);
}

export function connectHVACRuntimeInspection(container, loop, ui) {
  disconnectHVACRuntimeInspection(container);
  if (!container.querySelector("[data-hvac-runtime-control]")) return;
  const model = prepareHVACRuntimeInspection(loop), logic = normalize(model, ui), snapshot = hvacRuntimeSnapshot(model, logic.profileID, logic.frameIndex);
  updateValues(container, model, snapshot, logic, windowFrames(model, logic));
  connections.set(container, connectHVACParameterFlow(container.querySelector("[data-hvac-runtime-control]")));
}

export function handleHVACRuntimeEvent(event, container, loop, ui) {
  const target = event.target.closest?.("[data-hvac-runtime-profile], [data-hvac-runtime-window], [data-hvac-runtime-frame], [data-hvac-runtime-step], [data-hvac-runtime-trace], [data-hvac-runtime-seek], [data-hvac-runtime-source]");
  if (!target || !container.contains(target)) return false;
  const clickControl = target.matches("[data-hvac-runtime-step], [data-hvac-runtime-seek], [data-hvac-runtime-source]");
  if (clickControl !== (event.type === "click") || !clickControl && !["input", "change"].includes(event.type)) return false;
  event.stopPropagation(); if (clickControl) event.preventDefault();
  const model = prepareHVACRuntimeInspection(loop), logic = normalize(model, ui), oldWindow = windowFrames(model, logic);
  let rerender = false;
  if (target.hasAttribute("data-hvac-runtime-source")) logic.sourceID = logic.sourceID === target.dataset.hvacRuntimeSource ? "" : target.dataset.hvacRuntimeSource;
  else if (target.hasAttribute("data-hvac-runtime-profile")) { logic.profileID = target.value; rerender = true; }
  else if (target.hasAttribute("data-hvac-runtime-window")) { logic.window = target.value; rerender = true; }
  else if (target.hasAttribute("data-hvac-runtime-trace")) {
    const selected = new Set(logic.traces); target.checked ? selected.add(target.dataset.hvacRuntimeTrace) : selected.delete(target.dataset.hvacRuntimeTrace);
    logic.traces = [...selected]; rerender = true;
  } else if (target.hasAttribute("data-hvac-runtime-frame")) logic.frameIndex = Number(target.value);
  else if (target.hasAttribute("data-hvac-runtime-step")) logic.frameIndex += Number(target.dataset.hvacRuntimeStep);
  else if (oldWindow.length) {
    const rect = target.getBoundingClientRect(), ratio = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width));
    const x = oldWindow[0].x + ratio * (oldWindow.at(-1).x - oldWindow[0].x);
    logic.frameIndex = oldWindow.reduce((best, frame) => Math.abs(frame.x - x) < Math.abs(best.x - x) ? frame : best).index;
  }
  normalize(model, ui);
  const frames = windowFrames(model, logic);
  rerender ||= oldWindow[0]?.key !== frames[0]?.key || oldWindow.at(-1)?.key !== frames.at(-1)?.key;
  if (rerender) {
    disconnectHVACRuntimeInspection(container);
    container.querySelector("[data-hvac-runtime]").outerHTML = renderHVACRuntimeInspection(loop, ui);
    connectHVACRuntimeInspection(container, loop, ui);
  } else updateValues(container, model, hvacRuntimeSnapshot(model, logic.profileID, logic.frameIndex), logic, frames);
  return true;
}
