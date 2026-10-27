// Native availability inputs and their combined permission on one reported
// axis. Parameter-control schedules and equipment activity are separate facts.
import { t } from "./i18n.js";
import { hvacRuntimePermission, hvacRuntimeSignalState } from "./hvac-runtime-data.js";

const caches = new WeakMap();
const MAX_CACHE_ENTRIES = 4, MAX_CACHE_POINTS = 160000;
const finite = (value) => typeof value === "number" && Number.isFinite(value);
const token = (value) => String(value ?? "").trim().toLowerCase().replace(/\s+/g, " ");
const sourceKey = (source) => JSON.stringify([source?.objectIndex, source?.objectType, source?.objectName]);
const copy = (key, fallback) => t(`hvac.operation.${key}`, {}, fallback);
const runtimeCopy = (key, fallback) => t(`simulation.hvacRuntime${key}`, {}, fallback);
const sourceName = (source) => source?.objectName || source?.objectType || "";
const booleanState = (state) => ["allowed", "blocked", "mixed"].includes(state) ? state : "error";
const managerLabels = { no_action: "NoAction", force_off: "ForceOff", cycle_on: "CycleOn", zone_fans_only: "Zone fans only", hybrid_idle: "No hybrid control", natural_allowed: "Natural ventilation allowed", natural_blocked: "Natural ventilation blocked", mixed: "Interval average / mixed", error: "Data acquisition error", active: "Activity reported", inactive: "No activity reported" };

function statusLabel(state, kind) {
  if (state === "allowed" || state === "blocked") return copy(`status.${kind === "schedule" ? state === "allowed" ? "on" : "off" : state === "allowed" ? "available" : "unavailable"}`, kind === "schedule" ? state === "allowed" ? "ON" : "OFF" : state === "allowed" ? "Available" : "Unavailable");
  return runtimeCopy(`State.${state}`, managerLabels[state] || state);
}

function observedAt(signals, frame) {
  return signals.map((signal) => ({ ...signal, value: frame && signal.status === "collected" ? signal.values.get(frame.key) ?? null : null }));
}

function conditions(profile) {
  const rows = [], scheduleOwners = new Map(), managerOwners = new Map(), expressions = new WeakMap();
  const addOwner = (map, key, owner) => {
    if (!owner) return;
    if (!map.has(key)) map.set(key, new Map());
    map.get(key).set(sourceKey(owner), owner);
  };
  const configured = new Map((profile.definition.configuredAvailabilityLayers || []).map((layer) => [layer.id, layer]));
  const walk = (expression, owner, path, domain = "boolean", depth = 0) => {
    if (!expression || depth > 64) return;
    if (expression.op === "schedule_positive") addOwner(scheduleOwners, token(expression.scheduleName), expression.source || owner);
    if (expression.op?.startsWith("manager_") && expression.op !== "manager_gate") addOwner(managerOwners, sourceKey(expression.source), expression.assignmentOwner || owner);
    if (["manager_gate", "fan_gate"].includes(expression.op) || domain === "boolean" && expression.op === "always" && expression.value === 0) {
      const row = { id: `condition:${path}`, kind: "condition", operator: expression.op, expression, source: owner || expression.source,
        owners: [owner || expression.source].filter(Boolean), label: expression.op === "fan_gate" ? copy("flow.fanPermission", "Fan permission")
          : expression.op === "manager_gate" ? copy("flow.managerPermission", "Manager permission") : copy("operationSchemeMembership", "Equipment selection") };
      rows.push(row); expressions.set(expression, row.id);
    }
    (expression.children || []).forEach((child, index) => walk(child, owner, `${path}.${index}`, ["air_managers", "plant_managers"].includes(expression.op) || index === 0 && ["manager_gate", "fan_gate"].includes(expression.op) ? "manager" : "boolean", depth + 1));
  };
  const layers = profile.definition.availabilityLayers || [];
  if (layers.length) layers.forEach((layer, index) => walk(profile.availabilityExpression?.children?.[index] || (layer.role === "plant_dispatch" ? configured.get(layer.id)?.expression || layer.expression : layer.expression), layer.owner, String(index)));
  else walk(profile.availabilityExpression, profile.source, "0");
  const graph = profile.definition.availabilityDefinitions || {}, nodes = new Map((graph.nodes || []).map((node) => [node.id, node]));
  for (const node of graph.nodes || []) {
    if (node.kind !== "schedule") continue;
    for (const edge of graph.edges || []) {
      if (edge.to !== node.id) continue;
      const owner = nodes.get(edge.from);
      if (owner && owner.kind !== "schedule") addOwner(scheduleOwners, token(node.source?.objectName), owner.source);
    }
  }
  return { rows, scheduleOwners, managerOwners, expressions };
}

function definitions(profile) {
  const context = conditions(profile), inputs = [], grouped = new Map();
  for (const signal of profile.signals) {
    if (!["manager_status", "loop_status"].includes(signal.role) && !(signal.role === "schedule" && signal.graph !== "parameterControl")) continue;
    const kind = signal.role === "schedule" ? "schedule" : "manager";
    const identity = kind === "schedule" ? `schedule:${token(signal.keyValue)}` : `${signal.role}:${sourceKey(signal.source)}:${signal.domain}`;
    let row = grouped.get(identity);
    if (!row) {
      const owners = [...(kind === "schedule" ? context.scheduleOwners.get(token(signal.keyValue)) : context.managerOwners.get(sourceKey(signal.source)))?.values() || []];
      row = { id: identity, kind, source: signal.source, owners: owners.length ? owners : [signal.role === "loop_status" ? signal.source : profile.source],
        label: signal.role === "loop_status" ? runtimeCopy("Role.loop_status", "Loop availability") : signal.source?.objectName || signal.keyValue,
        signals: [], signalIds: [] };
      grouped.set(identity, row); inputs.push(row);
    }
    row.signals.push(signal); row.signalIds.push(signal.id);
  }
  const final = { id: "final-availability", kind: "permission", source: profile.source, owners: [profile.source],
    label: copy("control.availability_permission", "Final availability"), operator: profile.availabilityExpression?.op, expression: profile.availabilityExpression };
  const activitySignals = profile.signals.filter((signal) => signal.role === "activity"), fractions = activitySignals.filter((signal) => /runtime fraction|cycling ratio/i.test(signal.variableName));
  const activity = activitySignals.length ? { id: "equipment-activity", kind: "activity", label: runtimeCopy("Activity", "Reported equipment activity"), source: profile.source, owners: [profile.source], signals: fractions.length ? fractions : activitySignals, signalIds: activitySignals.map((signal) => signal.id) } : null;
  const rows = [...inputs.filter((row) => row.kind === "schedule"), ...inputs.filter((row) => row.kind === "manager"), ...context.rows, final];
  return { rows, activity, expressions: context.expressions,
    signals: profile.signals.filter((signal) => signal.role === "schedule" && signal.graph !== "parameterControl" || ["manager_status", "loop_status", "activity"].includes(signal.role)) };
}

function rowState(profile, row, observed) {
  if (row.expression || row.kind === "condition" || row.kind === "permission") {
    const result = hvacRuntimePermission(profile, observed, row.expression);
    return { state: booleanState(result.state), value: result.state === "allowed" ? 1 : result.state === "blocked" ? 0 : null, signalIds: result.signals };
  }
  const selected = row.signals.map((signal) => observed.find((entry) => entry.id === signal.id));
  if (!selected.length || selected.some((signal) => !finite(signal?.value))) return { state: "error", value: null, signalIds: row.signalIds };
  if (row.kind === "activity") return { state: selected.some((signal) => Math.abs(signal.value) > 1e-8) ? "active" : "inactive", value: null, signalIds: row.signalIds };
  const values = new Set(selected.map((signal) => signal.value));
  if (values.size !== 1) return { state: "error", value: null, signalIds: row.signalIds };
  const signal = selected[0], value = signal.value;
  return { state: row.kind === "schedule" ? value > 0 ? "allowed" : "blocked" : hvacRuntimeSignalState(signal, value), value, signalIds: row.signalIds };
}

// Trace only operators whose native semantics determine permission. A manager
// override suppresses the fan's own schedule; NoAction leaves it in charge.
function deciding(profile, observed, expression, expressions, domain = "boolean", depth = 0) {
  const empty = () => ({ signals: new Set(), rows: new Set() });
  if (!expression || depth > 64) return empty();
  const result = hvacRuntimePermission(profile, observed, expression, domain), children = expression.children || [];
  const merge = (entries) => ({ signals: new Set(entries.flatMap((entry) => [...entry.signals])), rows: new Set(entries.flatMap((entry) => [...entry.rows])) });
  const evaluate = (child, childDomain = "boolean") => hvacRuntimePermission(profile, observed, child, childDomain);
  const trace = (child, childDomain = "boolean") => deciding(profile, observed, child, expressions, childDomain, depth + 1);
  let selected;
  if (["all", "any"].includes(expression.op)) {
    const determining = expression.op === "all" && result.state === "blocked" ? "blocked" : expression.op === "any" && result.state === "allowed" ? "allowed" : result.state === "mixed" ? "mixed" : null;
    selected = merge(children.filter((child) => !determining || evaluate(child).state === determining).map((child) => trace(child)));
  } else if (["manager_gate", "fan_gate"].includes(expression.op)) {
    const action = evaluate(children[0], "manager");
    selected = expression.op === "fan_gate" && action.state === "manager" && action.value === 0 ? trace(children[1]) : trace(children[0], "manager");
  } else if (["air_managers", "plant_managers"].includes(expression.op)) {
    const actions = children.map((child) => evaluate(child, "manager"));
    if (actions.every((action) => action.state === "manager")) {
      const rank = [0, 3, 1, 2], winning = expression.op === "plant_managers" ? actions.findIndex((action) => action.value !== 0) : -1;
      const value = expression.op === "plant_managers" ? winning < 0 ? 0 : actions[winning].value : actions.reduce((current, action) => rank[action.value] > rank[current] ? action.value : current, 0);
      selected = merge(children.filter((_, index) => expression.op === "plant_managers" && winning >= 0 ? index === winning : actions[index].value === value).map((child) => trace(child, "manager")));
    } else selected = merge(children.filter((_, index) => actions[index].state === "mixed").map((child) => trace(child, "manager")));
  } else selected = { signals: new Set(result.signals), rows: new Set() };
  if (expressions.has(expression)) selected.rows.add(expressions.get(expression));
  return selected;
}

function frameKey(model, frames) {
  if (!frames.length) return "empty";
  const first = frames[0], last = frames.at(-1);
  return frames === model.frames || last.index - first.index + 1 === frames.length ? `${first.key}|${last.key}|${frames.length}` : frames.map((frame) => frame.key).join("\0");
}

/** Return immutable plot points plus cheap selected-frame state. The final
 * permission is always the last stack row; activity remains a separate fact.
 * Points preserve all native frames and mixed/error states, never fill gaps. */
export function buildHVACRuntimeAvailability(model, profileID, frames = model.frames, frameIndex = 0) {
  const profile = profileID === undefined ? model.profiles[0] : model.profiles.find((entry) => entry.id === String(profileID));
  const index = Number(frameIndex);
  const frame = Number.isInteger(index) && index >= 0 && index < model.frames.length ? model.frames[index] : undefined;
  if (!profile) return { profile: null, frame, rows: [], activity: null, decidingIds: [], errors: [] };
  let cache = caches.get(model);
  if (!cache || cache.profile !== profile) { cache = { profile, ...definitions(profile), entries: new Map(), points: 0 }; caches.set(model, cache); }
  const key = frameKey(model, frames);
  let cached = cache.entries.get(key);
  if (!cached) {
    const allRows = [...cache.rows, ...(cache.activity ? [cache.activity] : [])];
    const points = allRows.map(() => []);
    for (const entry of frames) {
      const observed = observedAt(cache.signals, entry);
      allRows.forEach((row, index) => { const current = rowState(profile, row, observed); points[index].push({ key: entry.key, x: entry.x, label: entry.label, state: current.state, value: current.value }); });
    }
    cached = { rows: cache.rows.map((row, index) => ({ ...row, points: points[index] })), activity: cache.activity ? { ...cache.activity, points: points.at(-1) } : null, count: allRows.length * frames.length };
    // Keep the selected window, even when a full run exceeds the usual cache
    // budget. A large window owns the cache alone; moving its cursor is cheap.
    while (cache.entries.size && (cache.entries.size >= MAX_CACHE_ENTRIES || cache.points + cached.count > MAX_CACHE_POINTS)) {
      const oldest = cache.entries.keys().next().value;
      cache.points -= cache.entries.get(oldest).count; cache.entries.delete(oldest);
    }
    cache.entries.set(key, cached); cache.points += cached.count;
  }
  const observed = observedAt(cache.signals, frame), decision = frame ? deciding(profile, observed, profile.availabilityExpression, cache.expressions) : { rows: new Set(), signals: new Set() };
  const present = (row) => {
    const current = frame ? rowState(profile, row, observed) : { state: "error", value: null, signalIds: row.signalIds || [] };
    return { ...row, signalIds: row.signalIds || current.signalIds, owner: row.owners.map(sourceName).filter(Boolean).join(" · "), current: { ...current, label: statusLabel(current.state, row.kind) },
      decisive: row.kind !== "permission" && (decision.rows.has(row.id) || (row.signalIds || []).some((id) => decision.signals.has(id))) };
  };
  const rows = cached.rows.map(present), activity = cached.activity ? present(cached.activity) : null;
  const errors = [...rows, ...(activity ? [activity] : [])].filter((row) => row.current.state === "error").map((row) => ({ id: row.id, source: row.source, signalIds: row.signalIds || [], status: "error" }));
  return { profile, frame, rows, activity, decidingIds: rows.filter((row) => row.decisive).map((row) => row.id), errors };
}
