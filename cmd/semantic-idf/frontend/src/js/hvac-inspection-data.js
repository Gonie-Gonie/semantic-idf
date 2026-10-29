import { getLanguage, t } from "./i18n.js";
import { hasHVACTemperatureSetpoint } from "./hvac-setpoint.js";
import { selectHVACInspectionTimestepSeries, matchHVACRuntimeFrame } from "./hvac-inspection-cadence.js";
import { prepareHVACRuntimeInspection } from "./hvac-runtime-data.js";
import { simulationSeriesPointColumns } from "./simulation-result-transport.js";
export { matchHVACRuntimeFrame };

const cache = new WeakMap();
const customSources = new WeakMap(), customTraces = new WeakMap();
const finite = (value) => typeof value === "number" && Number.isFinite(value);
const token = (value) => String(value || "").trim().toLowerCase().replace(/\s+/g, " ");
const copy = (key, fallback) => t(`simulation.hvacInspect${key}`, {}, fallback);
const pointList = (series) => Array.isArray(series.displayPoints) && series.displayPoints.length === series.points?.length ? series.displayPoints : series.points || [];
const fileKey = (series) => token(series.file || "result").replace(/\\/g, "/");

export function hvacInspectionLoopKey(loop = {}) {
  return `${token(loop.loopType || loop.topology?.type)}|${token(loop.name)}`;
}

export function hvacInspectionSharedFrame(loop, context, ui) {
  const frames = prepareHVACInspection(loop).frames;
  let index = finite(context.frameX) ? frames.findIndex((frame) => frame.x >= context.frameX) : Number(context.frameIndex || 0);
  if (index < 0) index = frames.length - 1;
  ui.frameIndex = Math.max(0, Math.min(frames.length - 1, index));
}

export function rememberHVACInspectionFrame(loop, context, ui) {
  const frame = prepareHVACInspection(loop).frames[ui.frameIndex];
  if (frame) { context.frameX = frame.x; context.frameIndex = ui.frameIndex; }
}

function columnParts(series) {
  const column = String(series.column || ""), split = column.indexOf(":");
  return {
    name: series.name || (split < 0 ? column : column.slice(split + 1)).replace(/\s*\[[^\]]*\].*$/, "").replace(/\s*\([^)]*\)$/, "").trim(),
    key: series.keyValue || (split < 0 ? "" : column.slice(0, split)).trim(),
    unit: series.displayUnit || /\[([^\]]*)\]/.exec(series.displayColumn || column)?.[1] || "",
  };
}

function measurement(name, rawUnit) {
  const value = token(name);
  let kind = "other", label = name;
  for (const [pattern, id, title] of [
    [/setpoint.*temperature|temperature.*setpoint/, "setpoint", "Setpoint temperature"],
    [/relative humidity/, "relativeHumidity", "Relative humidity"],
    [/humidity ratio/, "humidity", "Humidity ratio"],
    [/mass flow rate/, "flow", "Mass flow"],
    [/temperature/, "temperature", "Temperature"],
    [/enthalpy/, "enthalpy", "Enthalpy"],
    [/\bcop\b|coefficient of performance/, "cop", "COP"],
    [/cycling ratio|part load ratio|runtime fraction|operating status|on off|on\/off/, "status", "Operating fraction"],
    [/shaft power/, "shaftPower", "Shaft power"],
    [/efficiency/, "efficiency", "Efficiency"],
    [/^boiler electricity rate$/, "fuelPower", "Electric input"],
    [/^boiler electricity energy$/, "fuelEnergy", "Electric input energy"],
    [/electricity rate|electric power/, "power", "Electric power"],
    [/electricity energy/, "energy", "Electric energy"],
    [/cooling rate/, "cooling", "Cooling rate"],
    [/heating rate/, "heating", "Heating rate"],
    [/heat transfer rate/, "heatTransfer", "Heat transfer rate"],
    [/cooling energy/, "coolingEnergy", "Cooling energy"],
    [/heating energy/, "heatingEnergy", "Heating energy"],
    [/heat transfer energy/, "heatTransferEnergy", "Heat transfer energy"],
    [/^boiler (?:natural ?gas|propane(?:gas)?|fuel ?oil(?:no[12]|#[12])|diesel|gasoline|coal|other ?fuel[12]) rate$/, "fuelPower", "Fuel input"],
    [/^boiler (?:natural ?gas|propane(?:gas)?|fuel ?oil(?:no[12]|#[12])|diesel|gasoline|coal|other ?fuel[12]) energy$/, "fuelEnergy", "Fuel energy"],
  ]) {
    if (pattern.test(value)) { kind = id; label = copy(id[0].toUpperCase() + id.slice(1), title); break; }
  }
  if (value.includes("part load ratio")) label = copy("PartLoad", "PLR");
  else if (value.includes("cycling ratio")) label = copy("Cycling", "Cycling ratio");
  else if (value.includes("runtime fraction")) label = copy("Runtime", "Runtime fraction");
  else if (value === "boiler electricity rate") label = copy("ElectricInput", "Electric input");
  else if (value === "boiler electricity energy") label = copy("ElectricInputEnergy", "Electric input energy");
  else if (value.includes("ancillary electricity rate")) label = copy("AuxiliaryPower", "Auxiliary electric power");
  else if (value.includes("ancillary electricity energy")) label = copy("AuxiliaryEnergy", "Auxiliary electric energy");
  let unit = String(rawUnit).trim(), factor = 1;
  if (token(unit) === "w") { unit = "kW"; factor = .001; }
  else if (token(unit) === "j") { unit = "kWh"; factor = 1 / 3600000; }
  else if (["c", "degc", "°c"].includes(token(unit))) unit = "°C";
  else if (kind === "humidity" && /kg.*\/kg|kg\/kg/i.test(unit)) { unit = "g/kg"; factor = 1000; }
  else if (kind === "enthalpy" && token(unit) === "j/kg") { unit = "kJ/kg"; factor = .001; }
  else if (!unit || ["dimensionless", "w/w"].includes(token(unit))) unit = "—";
  const quantity = kind === "status" ? value.includes("part load ratio") ? "part_load_ratio"
    : value.includes("cycling ratio") ? "cycling_ratio" : value.includes("runtime fraction") ? "runtime_fraction" : "operating_status" : undefined;
  return { kind, quantity, label, unit, factor };
}

// Use the shared SQL/CSV row identity first. A secondary file can join the
// calendar only when both files have exactly one observation for that label.
export function prepareHVACInspection(loop = {}) {
  const language = getLanguage();
  const saved = cache.get(loop);
  if (saved?.language === language) return saved;
  const waterLoop = ["plantloop", "condenserloop"].includes(token(loop.loopType || loop.topology?.type));
  const entities = new Map(), candidates = [];
  const add = (series, kind, owner = {}) => {
    if (!series || series.reportingFrequency && token(series.reportingFrequency) !== "hourly") return;
    const parts = columnParts(series), name = owner.componentName || parts.key;
    if (!name || kind === "node" && waterLoop && /humidity/i.test(parts.name) || !pointList(series).length) return;
    const id = kind === "node" ? `node:${token(name)}` : `component:${token(owner.componentType)}:${token(name)}`;
    if (!entities.has(id)) entities.set(id, { id, name, type: owner.componentType || "", kind, properties: [],
      inletNodes: owner.inletNodes || [], outletNodes: owner.outletNodes || [], nodePorts: owner.nodePorts || [],
      parentComponentName: owner.parentComponentName || "", parentComponentType: owner.parentComponentType || "" });
    candidates.push({ series, entity: entities.get(id), ...parts });
  };
  (loop.series || []).forEach((series) => add(series, "node"));
  (loop.components || []).forEach((component) => (component.series || []).forEach((series) => add(series, "component", component)));
  let model = buildInspectionModel(loop, language, waterLoop, candidates, "Hourly");
  const properties = model.entities.flatMap((entity) => entity.properties);
  const timestep = selectHVACInspectionTimestepSeries(loop, properties);
  if (timestep) {
    const promotedEntities = new Map();
    const promotedCandidates = properties.map((property) => {
      const series = timestep.get(property), parts = columnParts(series), converted = measurement(parts.name, parts.unit);
      if (!pointList(series).length || converted.kind !== property.kind || converted.unit !== property.unit) return null;
      if (!promotedEntities.has(property.entity.id)) promotedEntities.set(property.entity.id, { ...property.entity, properties: [] });
      return { series, ...parts, entity: promotedEntities.get(property.entity.id), propertyID: property.id };
    });
    if (promotedCandidates.every(Boolean)) {
      const promoted = buildInspectionModel(loop, language, waterLoop, promotedCandidates, "Timestep");
      if (promoted.frames.length && promoted.entities.flatMap((entity) => entity.properties).length === properties.length
        && preservesInspectionObservations(model, promoted)) model = promoted;
    }
  }
  if (!model.entities.length && !model.frames.length && !loop.topology && token(loop.loopType) === "zonehvac") {
    const profiles = loop.operatingProfiles || [];
    const current = profiles.length && profiles.every((profile) => !profile.loopId && Number.isInteger(profile.componentIndex));
    const owners = new Set(profiles.map((profile) => profile.componentIndex));
    if (current && (loop.controlSignals || []).every((signal) => !signal.loopId && owners.has(signal.componentIndex))) {
      const runtime = prepareHVACRuntimeInspection(loop);
      if (runtime.profiles.every((profile) => profile.source.objectIndex === profile.definition.componentIndex
        && profile.source.objectType && profile.source.objectName) && runtime.frames.length
        && runtime.frames.every((frame) => ["timestep", "zone timestep"].includes(token(frame.frequency)))) {
        const frames = runtime.frames.map((frame) => ({ ...frame })), byLabel = new Map();
        for (const frame of frames) byLabel.set(frame.label, byLabel.has(frame.label) ? null : frame);
        model = { ...model, primary: runtime.primary, frames, byRow: new Map(frames.map((frame) => [frame.row, frame])), byLabel,
          calendarValid: runtime.calendarValid, reportingFrequency: "Timestep", interval: runtime.interval };
      }
    }
  }
  cache.set(loop, model);
  return model;
}

// A finer series must retain each already-visible observation time. Hourly
// means and Timestep values need not agree, but neither gaps nor ambiguous
// timestamps may remove a valid node/equipment observation during promotion.
function preservesInspectionObservations(original, promoted) {
  const targets = new Map(promoted.entities.flatMap((entity) => entity.properties).map((property) => [property.id, property]));
  const frames = original.frames.map((frame) => promoted.frames[matchHVACRuntimeFrame(promoted, original, frame.index)]);
  for (const property of original.entities.flatMap((entity) => entity.properties)) {
    const target = targets.get(property.id);
    if (!target) return false;
    const before = propertyValues(original, property), after = propertyValues(promoted, target);
    for (const frame of original.frames) {
      if (finite(before.get(frame.x)) && (!frames[frame.index] || !finite(after.get(frames[frame.index].x)))) return false;
    }
  }
  return true;
}

function buildInspectionModel(loop, language, waterLoop, candidates, reportingFrequency) {
  const entities = new Map(candidates.map((item) => [item.entity.id, item.entity]));
  const files = new Map();
  for (const item of candidates) files.set(fileKey(item.series), (files.get(fileKey(item.series)) || 0) + 1);
  const primary = [...files].sort((a, b) => Number(b[0].endsWith(".sql")) - Number(a[0].endsWith(".sql")) || b[1] - a[1])[0]?.[0];
  const observations = new Map(), byLabel = new Map();
  for (const { series } of candidates) {
    if (fileKey(series) !== primary) continue;
    for (const point of pointList(series)) {
      if (!finite(point.x)) continue;
      const label = String(point.label || point.x);
      if (!observations.has(point.x)) observations.set(point.x, { row: point.x, label });
      else if (observations.get(point.x).label !== label) observations.get(point.x).ambiguous = true;
    }
  }
  const frames = [...observations.values()].filter((frame) => !frame.ambiguous).sort((a, b) => a.row - b.row);
  const leap = frames.some((frame) => /02[\/-]29/.test(frame.label));
  const calendar = frames.map((frame) => calendarHour(frame.label, leap));
  const calendarValid = calendar.length && calendar.every((x, index) => finite(x) && (!index || x > calendar[index - 1]));
  frames.forEach((frame, index) => {
    frame.x = calendarValid ? calendar[index] : frame.row;
    frame.index = index;
    byLabel.set(frame.label, byLabel.has(frame.label) ? null : frame);
  });
  const byRow = new Map(frames.map((frame) => [frame.row, frame]));
  const seen = new Set();
  candidates.sort((a, b) => Number(fileKey(b.series) === primary) - Number(fileKey(a.series) === primary));
  for (const item of candidates) {
    const id = item.propertyID || `${item.entity.id}|${token(item.name)}|${token(item.unit)}`;
    if (seen.has(id)) continue;
    seen.add(id);
    item.entity.properties.push({ id, name: item.name, ...measurement(item.name, item.unit), series: item.series, entity: item.entity });
  }
  let interval = reportingFrequency === "Hourly" ? 1 : undefined;
  if (reportingFrequency === "Timestep" && calendarValid && frames.length > 1) {
    for (let index = 1; index < frames.length; index++) {
      const difference = frames[index].x - frames[index - 1].x;
      interval = interval === undefined ? difference : Math.min(interval, difference);
    }
  }
  const model = { language, loop, waterLoop, primary, entities: [...entities.values()].filter((entity) => entity.properties.length), frames, byRow, byLabel, calendarValid, reportingFrequency, interval };
  // A requested output exists for every node, including nodes with no setpoint.
  // Offer this property only when at least one aligned observation is defined.
  for (const entity of model.entities) entity.properties = entity.properties.filter((property) => property.kind !== "setpoint" || [...propertyValues(model, property).values()].some(finite));
  model.entities = model.entities.filter((entity) => entity.properties.length);
  return model;
}

function calendarHour(label, leap, defaultYear = leap ? 2000 : 2001) {
  const match = /^(?:(\d{4})[-/])?(\d{1,2})[-/](\d{1,2})\s+(\d{1,2}):(\d{2})(?::\d{2})?$/.exec(label.trim());
  if (!match) return null;
  const year = Number(match[1]) || defaultYear, month = Number(match[2]), day = Number(match[3]), hour = Number(match[4]), minute = Number(match[5]);
  if (month < 1 || month > 12 || day < 1 || day > new Date(Date.UTC(year, month, 0)).getUTCDate() || hour > 24 || minute > 59) return null;
  return Date.UTC(year, month - 1, day, hour, minute) / 3600000;
}

function propertyValues(model, property) {
  if (property.values) return property.values;
  const values = new Map(), duplicates = new Set(), points = pointList(property.series);
  const ownLabels = new Map();
  for (const point of points) ownLabels.set(String(point.label || point.x), (ownLabels.get(String(point.label || point.x)) || 0) + 1);
  for (const point of points) {
    const label = String(point.label || point.x);
    const frame = fileKey(property.series) === model.primary ? model.byRow.get(point.x) : ownLabels.get(label) === 1 ? model.byLabel.get(label) : null;
    if (!frame) continue;
    if (values.has(frame.x)) duplicates.add(frame.x);
    const value = finite(point.value) && (property.kind !== "setpoint" || hasHVACTemperatureSetpoint(point.value)) ? point.value * property.factor : null;
    values.set(frame.x, finite(value) ? value : null);
  }
  for (const x of duplicates) values.set(x, null);
  let min = Infinity, max = -Infinity;
  for (const value of values.values()) if (finite(value)) { min = Math.min(min, value); max = Math.max(max, value); }
  property.valueRange = { min: min === Infinity ? null : min, max: max === -Infinity ? null : max };
  property.values = values;
  return values;
}

export function hvacInspectionTrace(model, property) {
  const values = propertyValues(model, property);
  return { id: property.id, label: `${property.entity.name} · ${property.label}`, unit: property.unit,
    ...(model.calendarValid && finite(model.interval) ? { interval: model.interval } : {}),
    points: model.frames.map((frame) => ({ x: frame.x, label: frame.label, value: values.get(frame.x) ?? null })) };
}

function customSource(model, property) {
  if (customSources.has(property)) return customSources.get(property);
  const counterpart = selectHVACInspectionTimestepSeries(model.loop, [property])?.get(property);
  const converted = counterpart && measurement(columnParts(counterpart).name, columnParts(counterpart).unit);
  const series = converted?.kind === property.kind && converted.unit === property.unit ? counterpart : property.series;
  const raw = token(series.reportingFrequency || /\(([^()]*)\)\s*$/.exec(series.column || "")?.[1] || model.reportingFrequency);
  const frequency = ["timestep", "zone timestep"].includes(raw) ? "Timestep" : raw === "hourly" ? "Hourly" : series.reportingFrequency || model.reportingFrequency;
  const result = { series, frequency, native: series === counterpart, converted: series === counterpart ? converted : property };
  customSources.set(property, result); return result;
}

export function hvacInspectionCustomFrequency(model, property) { return customSource(model, property).frequency; }

// Custom graphs may use a proven native Timestep channel independently of the
// loop's all-or-nothing snapshot promotion. The default node charts stay on
// their existing axis; no native sample is interpolated into that axis.
export function hvacInspectionCustomTrace(model, property) {
  let traces = customTraces.get(model);
  if (!traces) { traces = new Map(); customTraces.set(model, traces); }
  if (traces.has(property)) {
    const trace = traces.get(property); traces.delete(property); traces.set(property, trace); return trace;
  }
  const { series, frequency, native, converted } = customSource(model, property);
  const runtime = native ? prepareHVACRuntimeInspection(model.loop) : null;
  let rows, labels, values;
  if (native) {
    const columns = simulationSeriesPointColumns(series);
    const points = columns ? null : pointList(series);
    rows = columns?.rows || points.map(point => point.x); labels = columns?.labels || points.map(point => String(point.label ?? point.x));
    values = columns?.values || points.map(point => point.value);
  } else {
    const legacy = hvacInspectionTrace(model, property);
    rows = model.frames.map(frame => frame.row); labels = legacy.points.map(point => point.label); values = legacy.points.map(point => point.value);
  }
  const year = model.calendarValid && model.frames.length ? new Date(model.frames[0].x * 3600000).getUTCFullYear() : undefined;
  const leap = runtime?.leap || model.frames.some(frame => /02[\/-]29/.test(frame.label));
  const coordinates = native ? labels.map(label => calendarHour(String(label), leap, year)) : model.frames.map(frame => frame.x);
  const calendar = !native || model.calendarValid && coordinates.every((value, index) => finite(value) && (!index || value >= coordinates[index - 1]));
  const intervals = native ? runtime.nativeIntervalsFor(series) : series.nativeIntervals;
  let interval = frequency === "Hourly" ? 1 : undefined;
  const points = rows.map((row, index) => {
    const entry = intervals?.length === rows.length ? intervals[index] : null;
    // Point rows are presentation ordinals. Native SQL TimeIndex remains a
    // separate identity, aligned by offset in the verified source timeline.
    const metadata = Number.isInteger(entry?.timeIndex) && entry.timeIndex > 0 && Number.isInteger(entry.environmentPeriodIndex) && entry.environmentPeriodIndex > 0
      && Number.isInteger(entry.simulationDays) && entry.simulationDays > 0 && finite(entry.intervalMinutes) && entry.intervalMinutes > 0
      && (entry.year === undefined || Number.isInteger(entry.year) && entry.year >= 0) ? entry : null;
    if (metadata) interval = interval === undefined ? metadata.intervalMinutes / 60 : Math.min(interval, metadata.intervalMinutes / 60);
    const identity = metadata ? [metadata.environmentPeriodIndex, metadata.year || 0, metadata.simulationDays, metadata.timeIndex, metadata.intervalMinutes]
      : frequency === "Hourly" ? [row, String(labels[index]), 60] : ["unverified", property.id, row];
    const value = finite(values[index]) && (property.kind !== "setpoint" || hasHVACTemperatureSetpoint(values[index])) ? values[index] * (native ? converted.factor : 1) : null;
    return { x: calendar ? coordinates[index] : row, label: String(labels[index]), value: finite(value) ? value : null,
      key: JSON.stringify([fileKey(series), frequency, identity]) };
  });
  const trace = { id: property.id, label: `${property.entity.name} · ${property.label}`, unit: property.unit, reportingFrequency: frequency,
    ...(calendar && finite(interval) ? { interval } : {}), points };
  traces.set(property, trace);
  while (traces.size > 8) traces.delete(traces.keys().next().value);
  return trace;
}

export function hvacInspectionSnapshots(model, frameIndex, selection = {}) {
  const frame = model.frames[frameIndex];
  const entities = model.entities.map((entity) => {
    const hasRelativeHumidity = entity.properties.some((property) => property.kind === "relativeHumidity");
    const properties = entity.properties.filter((property) => entity.kind === "node"
      ? ["flow", "temperature", "relativeHumidity", "setpoint"].includes(property.kind) || property.kind === "humidity" && !hasRelativeHumidity
      : ["power", "cop", "status", "cooling", "heating", "heatTransfer"].includes(property.kind));
    const metrics = properties.map((property) => {
      const values = propertyValues(model, property);
      return { id: property.kind, quantity: property.quantity, name: property.name, label: property.label, unit: property.unit, valueRange: property.valueRange,
        value: frame ? values.get(frame.x) ?? null : null };
    });
    if (entity.kind === "component") {
      const priority = ["power", "cop", "status", "cooling", "heating", "heatTransfer"];
      metrics.sort((a, b) => priority.indexOf(a.id) - priority.indexOf(b.id));
    }
    const known = (id) => metrics.filter((metric) => metric.id === id && finite(metric.value));
    const indicators = known("status").length ? known("status") : metrics.filter((metric) => ["power", "cooling", "heating", "heatTransfer"].includes(metric.id) && finite(metric.value));
    const status = indicators.length ? indicators.some((metric) => Math.abs(metric.value) > .000001) ? "on" : "off" : "unknown";
    const selected = entity.id === selection.selectedNode || entity.id === selection.selectedComponent;
    return { ...entity, metrics, status, selected, active: metrics.some((metric) => finite(metric.value)) };
  });
  return { frame, nodes: entities.filter((entity) => entity.kind === "node"), components: entities.filter((entity) => entity.kind === "component") };
}

export function hvacInspectionBasicProperties(model, kind, visibleNodes = {}) {
  return model.entities.filter((entity) => entity.kind === "node" && visibleNodes[entity.id] !== false).flatMap((entity) => {
    const wanted = kind === "humidity" && entity.properties.some((property) => property.kind === "relativeHumidity") ? "relativeHumidity" : kind;
    return entity.properties.filter((property) => property.kind === wanted);
  });
}
