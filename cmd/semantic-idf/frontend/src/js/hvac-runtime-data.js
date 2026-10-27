// Join executed definitions to reported observations. No representative-day
// schedule, current editor model or forward-filled observation is substituted.
import { getLanguage } from "./i18n.js";
import { hasHVACTemperatureSetpoint } from "./hvac-setpoint.js";
import { simulationSeriesPointColumns } from "./simulation-result-transport.js";

const cache = new WeakMap();
const finite = (value) => typeof value === "number" && Number.isFinite(value);
const token = (value) => String(value ?? "").trim().toLowerCase().replace(/\s+/g, " ");
const points = (series) => Array.isArray(series.displayPoints) && series.displayPoints.length === series.points?.length ? series.displayPoints : series.points || [];
function columns(series, timelineOnly = false) {
  const shared = simulationSeriesPointColumns(series, { timelineOnly });
  if (shared) return shared;
  const observed = points(series);
  return { rows: observed.map((point) => point.x), labels: observed.map((point) => String(point.label || point.x)),
    ...(timelineOnly ? {} : { values: observed.map((point) => point.value) }) };
}
const cadence = (value) => ["timestep", "zone timestep"].includes(token(value)) ? "timestep"
  : ["detailed", "hvac system timestep", "system timestep"].includes(token(value)) ? "detailed" : token(value);
const discrete = (signal) => ["manager_status", "loop_status"].includes(signal.role);
const signature = (item) => [token(item.keyValue), token(item.variableName || item.name), cadence(item.reportingFrequency)].join("\0");
const seriesName = (series) => series.name || String(series.column || "").replace(/^[^:]*:/, "").replace(/\s*\[[^\]]*\].*$/, "").trim();
const seriesFrequency = (series) => series.reportingFrequency || /\(([^()]*)\)\s*$/.exec(series.column || "")?.[1];
const seriesSignature = (series) => signature({ keyValue: series.keyValue || String(series.column || "").split(":")[0],
  variableName: seriesName(series), reportingFrequency: seriesFrequency(series) });
const fileKey = (series) => token(series.file || "result").replace(/\\/g, "/");
const sourceIndex = (source) => source?.objectIndex;
const profileSource = (profile) => profile.parameterControl?.nodes?.find((node) => node.code === "equipment_response")?.source
  || profile.availabilityLayers?.find((layer) => layer.role === "equipment")?.owner || { objectIndex: profile.componentIndex };

function units(series) {
  let unit = series.displayUnit || /\[([^\]]*)\]/.exec(series.displayColumn || series.column || "")?.[1] || "—", factor = 1;
  if (token(unit) === "w") { unit = "kW"; factor = .001; }
  else if (token(unit) === "j") { unit = "kWh"; factor = 1 / 3600000; }
  else if (["c", "degc", "°c"].includes(token(unit))) unit = "°C";
  else if (/kg.*\/kg|kg\/kg/i.test(unit)) { unit = "g/kg"; factor = 1000; }
  else if (token(unit) === "j/kg") { unit = "kJ/kg"; factor = .001; }
  return { unit, factor };
}

function calendar(label, leap) {
  const match = /^(?:(\d{4})[-/])?(\d{1,2})[-/](\d{1,2})\s+(\d{1,2}):(\d{2})(?::(\d{2}))?$/.exec(label.trim());
  if (!match) return null;
  const year = Number(match[1]) || (leap ? 2000 : 2001), month = Number(match[2]), day = Number(match[3]), hour = Number(match[4]), minute = Number(match[5]);
  if (month < 1 || month > 12 || day < 1 || day > new Date(Date.UTC(year, month, 0)).getUTCDate() || hour > 24 || minute > 59) return null;
  const second = Number(match[6] || 0);
  if (second > 59) return null;
  return Date.UTC(year, month - 1, day, hour, minute, second) / 3600000;
}

function nativeMetadata(intervals, offset, length) {
  if (!Array.isArray(intervals) || intervals.length !== length) return null;
  const entry = intervals[offset];
  return Number.isInteger(entry?.timeIndex) && entry.timeIndex > 0
    && Number.isInteger(entry.environmentPeriodIndex) && entry.environmentPeriodIndex > 0
    && Number.isInteger(entry.simulationDays) && entry.simulationDays > 0
    && (entry.year === undefined || Number.isInteger(entry.year) && entry.year >= 0)
    && finite(entry.intervalMinutes) && entry.intervalMinutes > 0 ? entry : null;
}
const nativeEndpoint = (metadata, x) => metadata && finite(x) ? `${metadata.environmentPeriodIndex}:${metadata.year || 0}:${metadata.simulationDays}:${x}` : null;
const sameEnvironment = (left, right) => left?.environmentPeriodIndex === right?.environmentPeriodIndex && (left?.year || 0) === (right?.year || 0);

// The original SQL interval owns its action. Never extend a previous action
// into an unreported gap or reconstruct subintervals from an averaged code.
function nativeTimeline(model, signal) {
  if (!signal.series) return { intervals: [], byEnd: new Map() };
  if (model.nativeTimelines.has(signal.series)) return model.nativeTimelines.get(signal.series);
  const axis = columns(signal.series, true), metadataAxis = model.nativeIntervalsFor(signal.series), intervals = [], byEnd = new Map();
  const shared = metadataAxis && model.nativeAxes.get(metadataAxis);
  if (shared?.get(axis.rows)?.labels === axis.labels) { const timeline = shared.get(axis.rows); model.nativeTimelines.set(signal.series, timeline); return timeline; }
  for (let offset = 0; offset < axis.rows.length; offset++) {
    const metadata = nativeMetadata(metadataAxis, offset, axis.rows.length), label = String(axis.labels[offset] || "");
    const x = calendar(label, model.leap), endpoint = nativeEndpoint(metadata, x);
    if (!endpoint) continue;
    const entry = { key: `native:${metadata.environmentPeriodIndex}:${metadata.timeIndex}`, row: axis.rows[offset], label,
      x, startX: x - metadata.intervalMinutes / 60, endX: x, metadata, offset };
    intervals.push(entry);
    byEnd.set(endpoint, byEnd.has(endpoint) ? null : entry);
  }
  intervals.sort((a, b) => a.x - b.x || a.startX - b.startX);
  for (let offset = 1; offset < intervals.length; offset++) {
    const previous = intervals[offset - 1], current = intervals[offset];
    if (sameEnvironment(previous.metadata, current.metadata) && current.startX < previous.endX - 1e-7) {
      previous.invalid = true; current.invalid = true;
    }
  }
  const result = { intervals, byEnd, labels: axis.labels };
  if (metadataAxis) { const axes = shared || new WeakMap(); axes.set(axis.rows, result); model.nativeAxes.set(metadataAxis, axes); }
  model.nativeTimelines.set(signal.series, result); return result;
}

function nativeValue(model, signal, entry) {
  if (!entry || entry.invalid || signal.status !== "collected") return null;
  if (!model.nativeColumns.has(signal.series)) model.nativeColumns.set(signal.series, columns(signal.series));
  const value = model.nativeColumns.get(signal.series).values[entry.offset];
  return finite(value) ? value * signal.factor : null;
}

// Availability describes permission; plant dispatch and transport demand are
// separate operating decisions. Retain actual manager actions, while using
// the native configured dispatch membership/schedules for minimum permission.
function runtimeAvailability(definition) {
  if (!definition.availabilityLayers?.length) return definition.finalAvailability;
  const configured = new Map((definition.configuredAvailabilityLayers || []).map((layer) => [layer.id, layer]));
  const permission = (expression) => expression?.op === "unknown" && expression.reason === "airflow_path_requires_runtime"
    ? { op: "always" } : expression?.children ? { ...expression, children: expression.children.map(permission) } : expression;
  return { op: "all", children: definition.availabilityLayers.map((layer) => layer.role === "plant_dispatch"
    ? configured.get(layer.id)?.expression || layer.expression : permission(layer.expression)) };
}

const sameSource = (left, right) => Number.isInteger(left?.objectIndex) && left.objectIndex === right?.objectIndex
  && token(left.objectType) === token(right.objectType) && token(left.objectName) === token(right.objectName);

// These outputs average HVAC-system actions over the reported zone timestep.
// NightCycle cannot emit ForceOff: an average of NoAction=0 and CycleOn=2 can
// nevertheless equal 1 exactly. Only constrain a signal when its executed
// source and control type prove the possible native actions.
function nightCycleStates(definition, source, nodeID) {
  if (token(source?.objectType) !== "availabilitymanager:nightcycle") return null;
  const nodes = (definition.availabilityDefinitions?.nodes || []).filter((node) => node.code === "availability_manager"
    && sameSource(node.source, source) && (!nodeID || node.id === nodeID));
  if (nodes.length !== 1) return null;
  const controls = (nodes[0].parameters || []).filter((parameter) => token(parameter.name) === "control type" && parameter.objectIndex === source.objectIndex);
  if (controls.length !== 1) return null;
  switch (token(controls[0].value)) {
    case "stayoff": return [0];
    case "cycleonany": case "cycleoncontrolzone": case "cycleonanycoolingzone": case "cycleonanyheatingzone": return [0, 2];
    case "cycleonanyzonefansonly": case "cycleonanyheatingzonefansonly": return [0, 3];
    case "cycleonanycoolingorheatingzone": return [0, 2, 3];
    default: return null;
  }
}

function possibleManagerStates(definition, signal) {
  if (signal.role === "manager_status" && signal.graph === "availabilityDefinitions") return nightCycleStates(definition, signal.source, signal.nodeId);
  // A global HybridVentilation manager can override the owning loop outside
  // its assignment list. Its aggregate action domain cannot be inferred here.
  return null;
}

export function prepareHVACRuntimeInspection(loop = {}) {
  const language = getLanguage(), saved = cache.get(loop);
  if (saved?.language === language) return saved;
  const byID = new Map(), bySignature = new Map();
  for (const item of loop.controlSeries || []) {
    if (item.sourceId) { const list = byID.get(item.sourceId) || []; list.push(item); byID.set(item.sourceId, list); }
    const key = seriesSignature(item);
    const list = bySignature.get(key) || []; list.push(item); bySignature.set(key, list);
  }
  const nativeMetadataCache = new WeakMap();
  const nativeIntervalsFor = (item) => {
    if (nativeMetadataCache.has(item)) return nativeMetadataCache.get(item);
    let intervals = Array.isArray(item.nativeIntervals) ? item.nativeIntervals : null;
    if (!intervals && item.nativeIntervalsSourceId) {
      const sources = byID.get(item.nativeIntervalsSourceId) || [], source = sources.length === 1 ? sources[0] : null;
      if (Array.isArray(source?.nativeIntervals) && fileKey(source) === fileKey(item)) {
        const target = columns(item, true), reference = columns(source, true);
        if (target.rows === reference.rows && target.labels === reference.labels
          || target.rows.length === reference.rows.length && target.rows.every((row, index) => row === reference.rows[index] && target.labels[index] === reference.labels[index])) intervals = source.nativeIntervals;
      }
    }
    nativeMetadataCache.set(item, intervals); return intervals;
  };
  const signals = (loop.controlSignals || []).map((binding) => {
    const matching = binding.seriesId ? byID.get(binding.seriesId) || [] : bySignature.get(signature(binding)) || [];
    const exact = matching.filter((item) => seriesSignature(item) === signature(binding));
    const item = exact.length === 1 ? exact[0] : null;
    let status = binding.status === "collected" && item ? "collected" : binding.status === "collected" ? exact.length > 1 ? "ambiguous" : "missing" : binding.status || "missing";
    const wrongCadence = discrete(binding) && cadence(binding.reportingFrequency) !== "detailed";
    if (status === "collected" && wrongCadence) status = "unsupported";
    return { ...binding, series: item, status, ...(wrongCadence ? { message: "Discrete availability actions require native HVAC substep observations." } : {}), values: new Map(), ...units(item || {}) };
  });
  const available = [...new Set(signals.filter((signal) => signal.status === "collected").map((signal) => signal.series))];
  const files = new Map();
  for (const item of available) files.set(fileKey(item), (files.get(fileKey(item)) || 0) + 1);
  const primary = [...files].sort((a, b) => Number(b[0].endsWith(".sql")) - Number(a[0].endsWith(".sql")) || b[1] - a[1])[0]?.[0];
  const observations = new Map(), seenTimelines = new Set();
  for (const item of available) {
    if (fileKey(item) !== primary || cadence(seriesFrequency(item)) !== "timestep") continue;
    const axis = columns(item, true);
    if (seenTimelines.has(axis.rows)) continue;
    seenTimelines.add(axis.rows);
    for (let offset = 0; offset < axis.rows.length; offset++) {
      const row = axis.rows[offset];
      if (!finite(row)) continue;
      const key = `${cadence(seriesFrequency(item))}:${row}`, label = String(axis.labels[offset] || row);
      const metadata = nativeMetadata(nativeIntervalsFor(item), offset, axis.rows.length);
      if (!observations.has(key)) observations.set(key, { key, row, label, frequency: seriesFrequency(item), ...(metadata ? { metadata } : {}) });
      else if (observations.get(key).label !== label) observations.get(key).ambiguous = true;
    }
  }
  const frames = [...observations.values()].filter((frame) => !frame.ambiguous).sort((a, b) => a.row - b.row);
  const leap = frames.some((frame) => /02[\/-]29/.test(frame.label)), times = frames.map((frame) => calendar(frame.label, leap));
  const calendarValid = times.length > 0 && times.every((value, index) => finite(value) && (!index || value > times[index - 1]));
  const labels = new Map();
  frames.forEach((frame, index) => {
    frame.x = calendarValid ? times[index] : index; frame.index = index;
    if (calendarValid && frame.metadata) { frame.startX = frame.x - frame.metadata.intervalMinutes / 60; frame.endX = frame.x; }
    const key = `${cadence(frame.frequency)}:${frame.label}`; labels.set(key, labels.has(key) ? null : frame);
  });
  const byRow = new Map(frames.map((frame) => [frame.key, frame]));
  const alignedValues = new Map(), indexers = new WeakMap(), empty = new Map();
  const align = (signal) => {
    if (!signal.series) return empty;
    if (alignedValues.has(signal.series)) return alignedValues.get(signal.series);
    if (cadence(signal.reportingFrequency) === "detailed") {
      const timeline = nativeTimeline(model, signal), values = { get(key) {
        const frame = byRow.get(key), endpoint = frame && nativeEndpoint(frame.metadata, frame.x);
        return endpoint ? nativeValue(model, signal, timeline.byEnd.get(endpoint)) : null;
      } };
      alignedValues.set(signal.series, values); return values;
    }
    const axis = columns(signal.series), frequency = cadence(signal.reportingFrequency), own = fileKey(signal.series) === primary;
    let byAxis = indexers.get(axis.rows);
    if (!byAxis) { byAxis = new Map(); indexers.set(axis.rows, byAxis); }
    const mode = `${own}:${frequency}`;
    let offsets = byAxis.get(mode);
    if (!offsets) {
      offsets = new Map();
      const ownLabels = new Map();
      if (!own) for (const label of axis.labels) ownLabels.set(String(label), (ownLabels.get(String(label)) || 0) + 1);
      for (let i = 0; i < axis.rows.length; i++) {
        const label = String(axis.labels[i] || axis.rows[i]);
        const frame = own ? byRow.get(`${frequency}:${axis.rows[i]}`) : ownLabels.get(label) === 1 ? labels.get(`${frequency}:${label}`) : null;
        if (frame) offsets.set(frame.key, offsets.has(frame.key) ? null : i);
      }
      byAxis.set(mode, offsets);
    }
    const values = { get(key) {
      const offset = offsets.get(key), value = offset === null || offset === undefined ? null : axis.values[offset];
      return finite(value) && !(/setpoint/i.test(signal.variableName) && !hasHVACTemperatureSetpoint(value)) ? value * signal.factor : null;
    } };
    alignedValues.set(signal.series, values); return values;
  };
  for (const signal of signals) Object.defineProperty(signal, "values", { enumerable: true, get: () => align(signal) });
  const profiles = (loop.operatingProfiles || []).filter((definition) => !loop.topology?.id || definition.loopId === loop.topology.id).map((definition) => {
    const profileSignals = signals.filter((signal) => signal.componentIndex === definition.componentIndex && (!signal.loopId || signal.loopId === definition.loopId));
    for (const signal of profileSignals) {
      const possibleStates = possibleManagerStates(definition, signal);
      if (possibleStates) signal.possibleStates = possibleStates;
    }
    return { id: String(definition.componentIndex), definition, availabilityExpression: runtimeAvailability(definition), source: profileSource(definition), signals: profileSignals };
  });
  let interval;
  if (calendarValid && frames.length > 1) for (let i = 1; i < frames.length; i++) { const next = frames[i].x - frames[i - 1].x; interval = interval === undefined ? next : Math.min(interval, next); }
  const model = { language, loop, profiles, signals, frames, primary, calendarValid, leap, interval, tracePoints: new Map(), nativeTimelines: new WeakMap(), nativeAxes: new WeakMap(), nativeColumns: new WeakMap(), availabilityFrames: new WeakMap(), nativeIntervalsFor };
  cache.set(loop, model); return model;
}

function lowerBound(entries, value, coordinate = (entry) => entry) {
  let low = 0, high = entries.length;
  while (low < high) { const middle = (low + high) >>> 1; if (coordinate(entries[middle]) < value) low = middle + 1; else high = middle; }
  return low;
}

// A shared frame remains a zone timestep. Availability history additionally
// retains every reported HVAC interval within that frame, including gaps.
export function hvacRuntimeAvailabilityFrames(model, profile, frames = model.frames) {
  let boundaries = model.availabilityFrames.get(profile);
  if (!boundaries) {
    const reported = profile.signals.filter((signal) => discrete(signal) && cadence(signal.reportingFrequency) === "detailed")
      .flatMap((signal) => nativeTimeline(model, signal).intervals.flatMap((entry) => [entry.startX, entry.endX])).sort((a, b) => a - b);
    // Subtracting duration from an absolute calendar hour can differ from the
    // neighboring endpoint by floating-point noise. It is one SQL boundary,
    // not an additional physical interval with almost-zero duration.
    boundaries = reported.filter((value, index) => !index || value - reported[index - 1] > 1e-7);
    model.availabilityFrames.set(profile, boundaries);
  }
  const result = [];
  for (const frame of frames) {
    const startX = frame.startX ?? frame.x - (model.interval || 1), endX = frame.x, stops = [startX];
    if (model.calendarValid) for (let offset = lowerBound(boundaries, startX + 1e-7); offset < boundaries.length && boundaries[offset] < endX - 1e-7; offset++) stops.push(boundaries[offset]);
    stops.push(endX);
    for (let offset = 1; offset < stops.length; offset++) result.push({ key: `availability:${frame.key}:${offset}`, x: stops[offset],
      startX: stops[offset - 1], endX: stops[offset], label: frame.label, sourceFrame: frame });
  }
  return result;
}

export function hvacRuntimeSignalsAt(model, signals, frame) {
  return signals.map((signal) => {
    let value = null;
    if (frame && signal.status === "collected") {
      if (!frame.sourceFrame || cadence(signal.reportingFrequency) !== "detailed") value = signal.values.get((frame.sourceFrame || frame).key) ?? null;
      else if (model.calendarValid && frame.sourceFrame.metadata) {
        const timeline = nativeTimeline(model, signal), offset = lowerBound(timeline.intervals, frame.endX - 1e-7, (entry) => entry.endX), entry = timeline.intervals[offset];
        if (entry && sameEnvironment(entry.metadata, frame.sourceFrame.metadata)
          && entry.metadata.simulationDays === frame.sourceFrame.metadata.simulationDays
          && entry.startX <= frame.startX + 1e-7 && entry.endX >= frame.endX - 1e-7
          && entry.startX >= frame.sourceFrame.startX - 1e-7 && entry.endX <= frame.sourceFrame.endX + 1e-7) value = nativeValue(model, signal, entry);
      }
    }
    return { ...signal, value };
  });
}

export function hvacRuntimeTrace(model, signal, frames) {
  const source = signal.source?.objectName;
  const label = [source, token(source) === token(signal.keyValue) ? "" : signal.keyValue, signal.variableName].filter(Boolean).join(" · ");
  if (frames) return { id: signal.id, label, unit: signal.unit,
    ...(model.interval ? { interval: model.interval } : {}), points: frames.map((frame) => ({ key: frame.key, x: frame.x, label: frame.label, value: signal.values.get(frame.key) ?? null })) };
  if (!model.tracePoints.has(signal.values)) model.tracePoints.set(signal.values,
    model.frames.map((frame) => ({ key: frame.key, x: frame.x, label: frame.label, value: signal.values.get(frame.key) ?? null })));
  return { id: signal.id, label, unit: signal.unit,
    ...(model.interval ? { interval: model.interval } : {}),
    points: model.tracePoints.get(signal.values) };
}

export function hvacRuntimeSignalState(signal, value) {
  if (signal.role === "manager_status" || signal.role === "loop_status") return managerStatus(signal, value).state;
  if (!finite(value)) return "error";
  return signal.role === "activity" ? Math.abs(value) > 1e-8 ? "active" : "inactive" : value > 0 ? "positive" : "nonpositive";
}

function managerStatus(signal, value) {
  if (!finite(value)) return { state: "error", value: null };
  if (cadence(signal.reportingFrequency) !== "detailed") return { state: "error", value };
  const possible = signal.possibleStates, minimum = possible ? possible[0] : 0, maximum = possible ? possible.at(-1) : signal.domain === "hybrid_ventilation_status" ? 2 : 3;
  if (value < minimum - 1e-7 || value > maximum + 1e-7) return { state: "error", value };
  if (Math.abs(value - Math.round(value)) > 1e-7) return { state: "error", value };
  const code = Math.round(value);
  if (possible && !possible.includes(code)) return { state: "error", value };
  if (signal.domain === "hybrid_ventilation_status") return { state: ["hybrid_idle", "natural_allowed", "natural_blocked"][code] || "error", value: code };
  return { state: ["no_action", "force_off", "cycle_on", "zone_fans_only"][code] || "error", value: code };
}

function evaluatePermission(profile, observed, expression, depth = 0, domain = "boolean") {
  const result = (state, signals = [], value) => ({ state, signals, value });
  if (!expression || depth > 64) return result("not_evaluated");
  const children = expression.children || [], op = expression.op;
  const managerResult = (value, signals = []) => domain === "manager" ? result("manager", signals, value)
    : result(value === 1 || value === 3 && expression.target !== "zone_fan" ? "blocked" : "allowed", signals, value);
  if (op === "always") return domain === "manager" ? managerResult(expression.value ?? 0) : result(expression.value === 0 ? "blocked" : "allowed", [], expression.value ?? 1);
  if (op === "schedule_positive") {
    const matches = observed.filter((signal) => signal.role === "schedule" && token(signal.keyValue) === token(expression.scheduleName));
    if (!matches.length || matches.some((signal) => !finite(signal.value)) || new Set(matches.map((signal) => signal.value)).size > 1) return result("not_evaluated", matches.map((signal) => signal.id));
    return result(matches[0].value > 0 ? "allowed" : "blocked", matches.map((signal) => signal.id), matches[0].value);
  }
  if (["all", "any"].includes(op)) {
    const inputs = children.map((entry) => evaluatePermission(profile, observed, entry, depth + 1)), states = inputs.map((entry) => entry.state), ids = inputs.flatMap((entry) => entry.signals);
    if (op === "all") return result(states.includes("blocked") ? "blocked" : states.every((state) => state === "allowed") ? "allowed" : "not_evaluated", ids);
    return result(states.includes("allowed") ? "allowed" : states.every((state) => state === "blocked") ? "blocked" : "not_evaluated", ids);
  }
  if (["air_managers", "plant_managers"].includes(op)) {
    // The native air-loop result also includes overrides outside its authored
    // assignment list. Keep those reported actions distinct from recomputing
    // the priority of individual manager definitions.
    if (op === "air_managers" && token(expression.assignmentOwner?.objectType) === "airloophvac") {
      const aggregate = observed.filter((signal) => signal.role === "loop_status" && sameSource(signal.source, expression.assignmentOwner));
      if (aggregate.length) {
        const ids = aggregate.map((signal) => signal.id), status = aggregate.length === 1 ? managerStatus(aggregate[0], aggregate[0].value) : { state: "error" };
        return status.state === "error" ? result("not_evaluated", ids) : managerResult(status.value, ids);
      }
    }
    const values = children.map((entry) => evaluatePermission(profile, observed, entry, depth + 1, "manager"));
    if (values.some((entry) => entry.state !== "manager")) return result("not_evaluated", values.flatMap((entry) => entry.signals));
    const rank = [0, 3, 1, 2], value = op === "plant_managers" ? values.find((entry) => entry.value !== 0)?.value || 0
      : values.reduce((current, entry) => rank[entry.value] > rank[current] ? entry.value : current, 0);
    return managerResult(value, values.flatMap((entry) => entry.signals));
  }
  if (["manager_gate", "fan_gate"].includes(op)) {
    const action = evaluatePermission(profile, observed, children[0], depth + 1, "manager"), own = op === "fan_gate" ? evaluatePermission(profile, observed, children[1], depth + 1) : result("allowed");
    const ids = [...action.signals, ...own.signals];
    if (action.state !== "manager") return result(action.state, ids);
    return result(action.value === 1 || action.value === 3 && expression.target !== "zone_fan" ? "blocked" : action.value === 2 || action.value === 3 ? "allowed" : own.state, ids);
  }
  if (op?.startsWith("manager_")) {
    const matches = observed.filter((signal) => signal.role === "manager_status" && sourceIndex(signal.source) === sourceIndex(expression.source));
    if (!matches.length || new Set(matches.map((signal) => signal.value)).size > 1) return result("not_evaluated");
    const signal = matches[0], status = managerStatus(signal, signal.value);
    if (status.state === "error" || signal.domain === "hybrid_ventilation_status") return result("not_evaluated", matches.map((entry) => entry.id), status.value);
    return managerResult(status.value, matches.map((entry) => entry.id));
  }
  return result("not_evaluated");
}

export function hvacRuntimePermission(profile, signals, expression, domain = "boolean") { return evaluatePermission(profile, signals, expression, 0, domain); }

export function hvacRuntimeSnapshot(model, profileID, frameIndex = 0) {
  const profile = profileID === undefined ? model.profiles[0] : model.profiles.find((entry) => entry.id === String(profileID));
  const frame = Number.isInteger(frameIndex) && frameIndex >= 0 ? model.frames[frameIndex] : undefined;
  if (!profile) return { profile: null, frame, errors: [], signals: [], availability: [], residuals: [] };
  const observed = profile.signals.map((signal) => ({ ...signal, value: frame ? signal.values.get(frame.key) ?? null : null }));
  const errors = observed.filter((signal) => signal.required && (signal.status !== "collected" || !finite(signal.value)
    || ["manager_status", "loop_status"].includes(signal.role) && managerStatus(signal, signal.value).state === "error"));
  const permission = evaluatePermission(profile, observed, profile.availabilityExpression), configuredPermission = evaluatePermission(profile, observed, profile.definition.configuredAvailability);
  if (!errors.length && permission.state === "not_evaluated") {
    const pending = [profile.availabilityExpression], reasons = new Set();
    for (let count = 0; pending.length && count < 4096; count++) {
      const expression = pending.pop();
      if (expression?.reason) reasons.add(expression.reason);
      pending.push(...expression?.children || []);
    }
    errors.push({ id: "availability-definition", keyValue: profile.source.objectName, variableName: "Availability definition", status: "unsupported",
      message: `The availability rule is not bound to a valid executed definition or required output${reasons.size ? `: ${[...reasons].join(", ")}` : ""}.` });
  }
  const activities = observed.filter((signal) => signal.role === "activity" && finite(signal.value)), fractions = activities.filter((signal) => /runtime fraction|cycling ratio/i.test(signal.variableName));
  const activity = activities.length ? (fractions.length ? fractions : activities).some((signal) => Math.abs(signal.value) > 1e-8) ? "active" : "inactive" : "not_collected";
  const demand = observed.filter((signal) => signal.role === "demand" && finite(signal.value)), residuals = [];
  for (const node of profile.definition.parameterControl?.nodes || []) {
    if (node.code !== "sensed_minus_setpoint") continue;
    const input = (relation, role) => {
      const edge = profile.definition.parameterControl.edges.find((entry) => entry.to === node.id && entry.relation === relation);
      const candidates = observed.filter((signal) => signal.role === role && signal.graph === "parameterControl" && finite(signal.value) && /temperature/i.test(signal.variableName));
      return candidates.find((signal) => signal.nodeId === edge?.from) || candidates.find((signal) => sourceIndex(signal.source) === sourceIndex(node.source));
    };
    const sensor = input("adds_measurement", "sensor"), setpoint = input("subtracts_target", "setpoint");
    if (sensor && setpoint && sensor.unit === setpoint.unit) residuals.push({ id: node.id, source: node.source, sensor, setpoint, value: sensor.value - setpoint.value, unit: sensor.unit });
  }
  const reason = errors.length ? "acquisition_error" : profile.definition.onOffExpression?.op === "passive_flow" ? "passive_flow" : activity === "not_collected" ? "reported_controls" : permission.state === "blocked" ? activity === "active" ? "permission_activity_conflict" : "permission_blocked"
    : activity === "active" ? "reported_activity" : demand.length && demand.every((signal) => Math.abs(signal.value) < 1e-8) ? "zero_reported_demand" : "reported_inactivity";
  return { profile, frame, signals: observed, errors, permission, configuredPermission, activity, reason, residuals,
    availability: observed.filter((signal) => signal.role === "manager_status" || signal.role === "loop_status" || signal.role === "schedule" && signal.graph !== "parameterControl")
      .map((signal) => ({ ...signal, state: hvacRuntimeSignalState(signal, signal.value) })) };
}
