import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";
import { prepareHVACRuntimeInspection, hvacRuntimeSnapshot, hvacRuntimeTrace } from "../hvac-runtime-data.js";
import { matchHVACRuntimeFrame } from "../hvac-inspection-data.js";
import { buildHVACRuntimeAvailability } from "../hvac-runtime-availability.js";
import { buildHVACControlExpressions } from "../hvac-control-expressions.js";
import { renderHVACParameterFlow, connectHVACParameterFlow } from "./hvac-parameter-flow-view.js";
import { renderHVACInspectionChart, updateHVACInspectionChartFrame } from "../hvac-inspection-charts.js";

const safe = escapeHTML, finite = (value) => typeof value === "number" && Number.isFinite(value);
const copy = (key, fallback) => t(`simulation.hvacRuntime${key}`, {}, fallback);
const connections = new WeakMap(), flowModels = new WeakMap();
const stateNames = { allowed: "Available", blocked: "Unavailable", mixed: "Mixed", no_action: "NoAction", force_off: "ForceOff", cycle_on: "CycleOn", zone_fans_only: "Zone fans only", active: "Activity reported", inactive: "No activity reported", error: "Data acquisition error" };
const stateName = (state) => copy("State." + state, stateNames[state] || state);
const number = (value, unit = "") => finite(value) ? value.toLocaleString(undefined, { minimumFractionDigits: unit === "kW" ? 2 : 0, maximumFractionDigits: unit === "kg/s" ? 4 : 2 }) + (unit && unit !== "—" ? " " + unit : "") : "—";
const distinct = (signals) => [...new Map(signals.map((signal) => [signal.seriesId + "|" + signal.keyValue + "|" + signal.variableName + "|" + signal.role, signal])).values()];

function metric(signal) {
  const name = signal.variableName.toLowerCase();
  const id = signal.role === "demand" ? /cooling/.test(name) ? "coolingDemand" : /heating/.test(name) ? "heatingDemand" : "demand"
    : signal.role === "actuator_request" ? "requestedFlow" : signal.role === "actuator" ? "actualFlow"
    : /minimum.*mass flow/.test(name) ? "minimumFlow" : /maximum.*mass flow/.test(name) ? "maximumFlow"
      : /heating.*setpoint/.test(name) ? "heatingTarget" : /cooling.*setpoint/.test(name) ? "coolingTarget"
        : signal.role === "setpoint" ? "setpoint" : /mass flow/.test(name) ? "flow"
          : /temperature/.test(name) ? "temperature" : /humidity/.test(name) ? "humidity"
            : /electricity|electric power/.test(name) ? "power" : /cooling/.test(name) ? "cooling"
              : /heating/.test(name) ? "heating" : signal.role === "demand" ? "demand"
                : /control type/.test(name) ? "controlMode" : signal.role === "schedule" ? "schedule" : "response";
  const labels = { requestedFlow: "Requested flow", actualFlow: "Actual flow", minimumFlow: "Minimum flow", maximumFlow: "Maximum flow", heatingTarget: "Heating target", coolingTarget: "Cooling target", setpoint: "Setpoint", flow: "Flow", temperature: "Temperature", humidity: "Humidity", power: "Electric power", cooling: "Cooling output", heating: "Heating output", demand: "Demand", controlMode: "Control mode", schedule: "Schedule", response: "Response" };
  return copy("Metric." + id, id === "coolingDemand" ? "Cooling demand" : id === "heatingDemand" ? "Heating demand" : labels[id]);
}

function controlModel(profile) {
  if (!flowModels.has(profile)) flowModels.set(profile, buildHVACControlExpressions(profile.definition, { component: profile.source }).parameters);
  return flowModels.get(profile);
}

function cards(profile) {
  return controlModel(profile).groups.flatMap((group) => group.cards.map((card) => ({ ...card, group: group.id })));
}

function cardSignals(profile, card) {
  const ids = new Set(card.members.map((node) => node.id));
  const exact = profile.signals.filter((signal) => signal.graph === "parameterControl" && ids.has(signal.nodeId));
  const response = card.group === "response" ? profile.signals.filter((signal) => signal.role === "activity" && signal.source?.objectIndex === card.source?.objectIndex) : [];
  return distinct([...exact, ...response]);
}

function plottedSignals(profile) {
  return distinct(profile.signals.filter((signal) => signal.graph === "parameterControl" || signal.graph === "loop" && signal.role === "demand"))
    .filter((signal) => !["manager_status", "loop_status"].includes(signal.role));
}

function context(loop, ui, inspection) {
  const model = prepareHVACRuntimeInspection(loop), logic = ui.logic ||= {};
  const profile = model.profiles.find((entry) => entry.id === logic.profileID && (!entry.definition.loopId || entry.definition.loopId === loop.topology?.id));
  if (!profile) return null;
  if (!["day", "week", "all"].includes(logic.window)) logic.window = "week";
  const index = matchHVACRuntimeFrame(model, inspection, ui.frameIndex);
  const snapshot = hvacRuntimeSnapshot(model, profile.id, index);
  const mainFrame = inspection.frames[ui.frameIndex];
  const frameX = model.frames[index]?.x ?? (model.calendarValid && inspection.calendarValid ? mainFrame?.x : undefined);
  let frames = model.frames;
  if (model.calendarValid && logic.window !== "all" && finite(frameX)) {
    const hours = logic.window === "day" ? 24 : 168, first = model.frames[0]?.x || frameX;
    const low = first + Math.floor((frameX - first) / hours) * hours;
    frames = model.frames.filter((frame) => frame.x >= low && frame.x < low + hours);
  }
  if (logic.traceProfile !== profile.id || !Array.isArray(logic.traces)) {
    logic.traceProfile = profile.id; logic.cardID = "";
    const counts = new Map();
    logic.traces = plottedSignals(profile).filter((signal) => {
      const count = counts.get(signal.unit) || 0; counts.set(signal.unit, count + 1);
      return count < 8 && signal.status === "collected";
    }).map((signal) => signal.id);
  }
  return { model, logic, profile, snapshot, frames, frameX, index, mainFrame,
    stack: buildHVACRuntimeAvailability(model, profile.id, frames, index) };
}

function diagnostic(snapshot) {
  if (!snapshot.errors.length) return "";
  const definition = snapshot.errors.some((error) => error.id === "availability-definition");
  const label = copy(definition ? "DefinitionError" : "AcquisitionError", definition ? "Control definition error" : "Control output acquisition failed");
  const detail = snapshot.errors.map((signal) => [signal.keyValue, signal.variableName, signal.reportingFrequency, signal.message].filter(Boolean).join(" · ")).join("\n");
  return `<div class="hvac-runtime-error" data-hvac-runtime-error role="alert" title="${safe(detail)}"><strong>${safe(label)}</strong><span>${snapshot.errors.length}</span></div>`;
}

function statusHeader(data) {
  const { snapshot, stack } = data, final = stack.rows.at(-1);
  const state = snapshot.errors.length ? "error" : final?.current.state || "error";
  return `<span class="hvac-runtime-state is-${safe(state)}" data-hvac-runtime-permission="${safe(state)}">${safe(stateName(state))}</span>${stack.activity ? `<span class="hvac-runtime-state" data-hvac-runtime-activity="${safe(stack.activity.current.state)}">${safe(stack.activity.current.label)}</span>` : ""}`;
}

function patches(points, frames, interval) {
  const low = (frames[0]?.x ?? 0) - interval, high = frames.at(-1)?.x ?? low + 1, span = high - low || 1;
  const bins = new Map();
  for (const point of points) {
    const start = Math.max(0, Math.floor((point.x - interval - low) / span * 1000));
    const end = Math.min(1000, Math.max(start + 1, Math.ceil((point.x - low) / span * 1000)));
    for (let bin = start; bin < end; bin++) bins.set(bin, bins.has(bin) && bins.get(bin) !== point.state ? "mixed" : point.state);
  }
  const ranges = [];
  for (const [bin, state] of bins) {
    const last = ranges.at(-1);
    if (last && last.state === state && last.end === bin) last.end++;
    else ranges.push({ start: bin, end: bin + 1, state });
  }
  return ranges.map((range) => `<rect class="hvac-runtime-patch is-${safe(range.state)}" x="${range.start}" y="6" width="${Math.max(.5, range.end - range.start)}" height="16"/>`).join("");
}

function availability(data) {
  const { stack, frames, model } = data;
  return `<section data-hvac-runtime-availability><h4>${safe(copy("AvailabilityStack", "Availability"))}</h4><div class="hvac-runtime-stack">${stack.rows.map((row) => `<article class="hvac-runtime-stack-row ${row.kind === "permission" ? "final" : ""}${row.decisive ? " decisive" : ""}" data-hvac-runtime-availability-row="${safe(row.id)}" data-kind="${safe(row.kind)}" data-state="${safe(row.current.state)}" data-decisive="${row.decisive}"><div class="hvac-runtime-stack-label"><span title="${safe(row.owner)}">${safe(row.owner)}</span><strong title="${safe(row.label)}">${safe(row.label)}</strong></div><svg viewBox="0 0 1000 28" preserveAspectRatio="none" role="img" aria-label="${safe(row.label)}">${patches(row.points, frames, model.interval || 1)}<line data-hvac-runtime-cursor x1="0" x2="0" y1="1" y2="27"/></svg><span data-hvac-runtime-current="${safe(row.id)}">${safe(row.current.label)}</span></article>`).join("")}<div class="hvac-runtime-stack-axis"><span></span><div><span>${safe(frames[0]?.label || "")}</span><span>${safe(frames.at(-1)?.label || "")}</span></div><span></span></div></div></section>`;
}

function control(data) {
  return `<section class="hvac-runtime-logic parameters" data-hvac-runtime-control><h4>${safe(copy("ControlSequence", "Control sequence"))}</h4>${renderHVACParameterFlow(controlModel(data.profile), { component: data.profile.source, details: false, hint: false })}</section>`;
}

function signalGroups(profile) {
  const groups = new Map();
  for (const signal of plottedSignals(profile)) {
    const key = signal.unit;
    if (!groups.has(key)) groups.set(key, { id: key, unit: key, signals: [] });
    groups.get(key).signals.push(signal);
  }
  return [...groups.values()];
}

function groupTitle(unit) {
  return copy("Plot." + ({ "°C": "temperature", "kg/s": "flow", "kW": "power", "g/kg": "humidity", "%": "humidity" }[unit] || "signals"),
    { "°C": "Temperature / setpoints", "kg/s": "Flow / commands", "kW": "Demand / equipment response", "g/kg": "Humidity", "%": "Humidity" }[unit] || "Control signals");
}

function history(data) {
  const { model, profile, snapshot, logic, frames, frameX } = data;
  const current = new Map(snapshot.signals.map((signal) => [signal.id, signal]));
  const selectedCard = cards(profile).find((card) => card.id === logic.cardID);
  const highlighted = new Set(selectedCard ? cardSignals(profile, selectedCard).map((signal) => signal.id) : []);
  return `<section data-hvac-runtime-history>${signalGroups(profile).map((group) => {
    const selected = group.signals.filter((signal) => logic.traces.includes(signal.id));
    const traces = selected.map((signal) => ({ ...hvacRuntimeTrace(model, signal, frames), label: metric(signal) + " · " + signal.keyValue, axisLabel: groupTitle(group.unit) }));
    return `<section class="hvac-runtime-history-group" data-hvac-runtime-history-group="${safe(group.id)}"><h4>${safe(groupTitle(group.unit))}</h4><div class="hvac-inspection-node-toggles hvac-runtime-trace-toggles">${group.signals.map((signal) => `<label class="${highlighted.has(signal.id) ? "related" : ""}" title="${safe(signal.keyValue + " · " + signal.variableName)}"><input type="checkbox" data-hvac-runtime-trace="${safe(signal.id)}"${logic.traces.includes(signal.id) ? " checked" : ""}${signal.status !== "collected" ? " disabled" : ""}/><span><strong>${safe(metric(signal))}</strong><small>${safe(signal.keyValue)}</small></span><b data-hvac-runtime-value="${safe(signal.id)}">${safe(number(current.get(signal.id)?.value, signal.unit))}</b></label>`).join("")}</div>${renderHVACInspectionChart({ series: traces, frameKey: frameX })}</section>`;
  }).join("")}</section>`;
}

export function renderHVACRuntimeInspection(loop, ui, inspection) {
  const data = context(loop, ui, inspection);
  if (!data) return ui.runtimeSelected ? `<section class="hvac-runtime" data-hvac-runtime><div class="empty" data-hvac-runtime-error>${safe(copy("Rerun", "Run HVAC Loop Check again to collect control diagnostics."))}</div></section>` : "";
  const { profile, logic, frames } = data;
  return `<section class="hvac-runtime" data-hvac-runtime data-hvac-runtime-profile-id="${safe(profile.id)}" data-window="${safe((frames[0]?.key || "") + "|" + (frames.at(-1)?.key || ""))}"><header class="hvac-runtime-heading"><div><h3>${safe(profile.source.objectName)}</h3><span>${safe(profile.source.objectType)}</span></div><div data-hvac-runtime-states>${statusHeader(data)}</div><select data-hvac-runtime-window aria-label="${safe(copy("Window", "Time window"))}">${["day", "week", "all"].map((value) => `<option value="${value}"${logic.window === value ? " selected" : ""}>${safe(copy("Window." + value, { day: "Day", week: "Week", all: "Full run" }[value]))}</option>`).join("")}</select></header><div data-hvac-runtime-diagnostic>${diagnostic(data.snapshot)}</div>${availability(data)}${control(data)}${history(data)}</section>`;
}

function flowValues(mount, data) {
  const allCards = cards(data.profile), values = new Map(data.snapshot.signals.map((signal) => [signal.id, signal]));
  for (const tile of mount.querySelectorAll("[data-hvac-flow-card]")) {
    const card = allCards.find((entry) => entry.id === tile.dataset.hvacFlowCard);
    if (!card) continue;
    const related = cardSignals(data.profile, card).map((signal) => values.get(signal.id));
    const roles = { inputs: ["setpoint", "schedule", "sensor", "demand"], controls: ["sensor", "setpoint"], commands: ["actuator_request", "actuator"], response: ["activity", "actuator"] }[card.group];
    const pick = roles.map((role) => related.find((signal) => signal.role === role)).find(Boolean);
    const ids = new Set(card.members.map((node) => node.id));
    const residual = data.snapshot.residuals.find((entry) => ids.has(entry.id));
    if (!pick && !residual) continue;
    let label = tile.querySelector("[data-hvac-runtime-card-value]");
    if (!label) { label = document.createElement("span"); label.dataset.hvacRuntimeCardValue = ""; tile.querySelector("button").append(label); }
    label.textContent = residual ? "Δ " + number(residual.value, residual.unit) : (card.group === "commands" ? metric(pick) + ": " : "") + number(pick.value, pick.unit);
    label.title = residual ? number(residual.sensor.value, residual.unit) + " − " + number(residual.setpoint.value, residual.unit) : metric(pick) + " · " + pick.keyValue;
    if (card.group === "commands" && pick?.role === "actuator_request") {
      const actual = related.find((signal) => signal.role === "actuator");
      if (actual) {
        let actualLabel = tile.querySelector("[data-hvac-runtime-card-actual]");
        if (!actualLabel) { actualLabel = document.createElement("span"); actualLabel.dataset.hvacRuntimeCardActual = ""; tile.querySelector("button").append(actualLabel); }
        actualLabel.textContent = copy("Actual", "Actual") + ": " + number(actual.value, actual.unit);
      }
    }
  }
}

function synchronize(mount, data) {
  mount.querySelector("[data-hvac-runtime-states]").innerHTML = statusHeader(data);
  mount.querySelector("[data-hvac-runtime-diagnostic]").innerHTML = diagnostic(data.snapshot);
  const current = new Map(data.snapshot.signals.map((signal) => [signal.id, signal]));
  for (const label of mount.querySelectorAll("[data-hvac-runtime-value]")) {
    const signal = current.get(label.dataset.hvacRuntimeValue);
    label.textContent = number(signal?.value, signal?.unit);
  }
  for (const row of data.stack.rows) {
    const element = mount.querySelector('[data-hvac-runtime-availability-row="' + CSS.escape(row.id) + '"]');
    if (!element) continue;
    element.dataset.state = row.current.state; element.dataset.decisive = String(row.decisive);
    element.classList.toggle("decisive", row.decisive);
    element.querySelector("[data-hvac-runtime-current]").textContent = row.current.label;
  }
  const low = (data.frames[0]?.x ?? 0) - (data.model.interval || 1), high = data.frames.at(-1)?.x ?? low + 1;
  const visible = finite(data.frameX) && data.frameX >= low && data.frameX <= high;
  for (const cursor of mount.querySelectorAll("[data-hvac-runtime-cursor]")) {
    cursor.style.display = visible ? "" : "none";
    const x = visible ? (data.frameX - low) / (high - low || 1) * 1000 : 0;
    cursor.setAttribute("x1", x); cursor.setAttribute("x2", x);
  }
  flowValues(mount, data); updateHVACInspectionChartFrame(mount, data.frameX);
}

export function disconnectHVACRuntimeInspection(container) {
  connections.get(container)?.(); connections.delete(container);
}

export function connectHVACRuntimeInspection(container, loop, ui, inspection) {
  disconnectHVACRuntimeInspection(container);
  const mount = container.querySelector("[data-hvac-runtime-control]");
  if (!mount) return;
  const data = context(loop, ui, inspection);
  if (!data) return;
  synchronize(container.querySelector("[data-hvac-runtime]"), data);
  connections.set(container, connectHVACParameterFlow(mount));
}

export function updateHVACRuntimeInspection(container, loop, ui, inspection) {
  const host = container.querySelector("[data-hvac-inspect-runtime]");
  if (!host) return;
  const data = ui.runtimeSelected && context(loop, ui, inspection), mount = host.querySelector("[data-hvac-runtime]");
  const key = data && (data.frames[0]?.key || "") + "|" + (data?.frames.at(-1)?.key || "");
  if (!data || mount?.dataset.hvacRuntimeProfileId !== data.profile.id || mount?.dataset.window !== key) {
    disconnectHVACRuntimeInspection(container);
    host.innerHTML = ui.runtimeSelected ? renderHVACRuntimeInspection(loop, ui, inspection) : "";
    connectHVACRuntimeInspection(container, loop, ui, inspection); return;
  }
  synchronize(mount, data);
}

export function handleHVACRuntimeEvent(event, container, loop, ui, inspection) {
  const host = event.target.closest?.("[data-hvac-runtime]");
  if (!host || !container.contains(host)) return false;
  const target = event.target.closest("[data-hvac-runtime-window], [data-hvac-runtime-trace], [data-hvac-inspect-parameter]");
  if (!target) return false;
  const click = target.matches("[data-hvac-inspect-parameter]");
  if (click ? event.type !== "click" : !["input", "change"].includes(event.type)) return false;
  const data = context(loop, ui, inspection);
  if (!data) return true;
  event.stopPropagation(); if (click) event.preventDefault();
  if (target.hasAttribute("data-hvac-runtime-window")) {
    data.logic.window = target.value;
    disconnectHVACRuntimeInspection(container); host.outerHTML = renderHVACRuntimeInspection(loop, ui, inspection);
    connectHVACRuntimeInspection(container, loop, ui, inspection); return true;
  }
  if (click) {
    data.logic.cardID = target.dataset.hvacInspectParameter;
    const card = cards(data.profile).find((entry) => entry.id === data.logic.cardID);
    const related = card && cardSignals(data.profile, card);
    if (related?.length) data.logic.traces = related.filter((signal) => signal.status === "collected").map((signal) => signal.id);
  } else {
    const selected = new Set(data.logic.traces);
    target.checked ? selected.add(target.dataset.hvacRuntimeTrace) : selected.delete(target.dataset.hvacRuntimeTrace);
    data.logic.traces = [...selected];
  }
  host.querySelector("[data-hvac-runtime-history]").outerHTML = history(data);
  synchronize(host, data); return true;
}
