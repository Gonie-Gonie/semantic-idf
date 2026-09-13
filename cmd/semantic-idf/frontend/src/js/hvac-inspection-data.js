import { t } from "./i18n.js";

const cache = new WeakMap();
const finite = (value) => typeof value === "number" && Number.isFinite(value);
const token = (value) => String(value || "").trim().toLowerCase().replace(/\s+/g, " ");
const copy = (key, fallback) => t(`simulation.hvacInspect${key}`, {}, fallback);
const pointList = (series) => Array.isArray(series.displayPoints) && series.displayPoints.length === series.points?.length ? series.displayPoints : series.points || [];
const fileKey = (series) => token(series.file || "result").replace(/\\/g, "/");

export function hvacInspectionLoopKey(loop = {}) {
  return `${token(loop.loopType || loop.topology?.type)}|${token(loop.name)}`;
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
    [/electricity rate|electric power/, "power", "Electric power"],
    [/electricity energy/, "energy", "Electric energy"],
    [/cooling rate/, "cooling", "Cooling rate"],
    [/heating rate/, "heating", "Heating rate"],
    [/heat transfer rate/, "heatTransfer", "Heat transfer rate"],
    [/cooling energy/, "coolingEnergy", "Cooling energy"],
    [/heating energy/, "heatingEnergy", "Heating energy"],
    [/heat transfer energy/, "heatTransferEnergy", "Heat transfer energy"],
  ]) {
    if (pattern.test(value)) { kind = id; label = copy(id[0].toUpperCase() + id.slice(1), title); break; }
  }
  if (value.includes("part load ratio")) label = copy("PartLoad", "Part load ratio");
  else if (value.includes("cycling ratio")) label = copy("Cycling", "Cycling ratio");
  else if (value.includes("runtime fraction")) label = copy("Runtime", "Runtime fraction");
  let unit = String(rawUnit).trim(), factor = 1;
  if (token(unit) === "w") { unit = "kW"; factor = .001; }
  else if (token(unit) === "j") { unit = "kWh"; factor = 1 / 3600000; }
  else if (["c", "degc", "°c"].includes(token(unit))) unit = "°C";
  else if (kind === "humidity" && /kg.*\/kg|kg\/kg/i.test(unit)) { unit = "g/kg"; factor = 1000; }
  else if (kind === "enthalpy" && token(unit) === "j/kg") { unit = "kJ/kg"; factor = .001; }
  else if (!unit || ["dimensionless", "w/w"].includes(token(unit))) unit = "—";
  return { kind, label, unit, factor };
}

// Use the shared SQL/CSV row identity first. A secondary file can join the
// calendar only when both files have exactly one observation for that label.
export function prepareHVACInspection(loop = {}) {
  const language = t("simulation.hvacInspectTemperature");
  const saved = cache.get(loop);
  if (saved?.language === language) return saved;
  const entities = new Map(), candidates = [];
  const add = (series, kind, owner = {}) => {
    if (!series || !pointList(series).length || series.reportingFrequency && token(series.reportingFrequency) !== "hourly") return;
    const parts = columnParts(series), name = owner.componentName || parts.key;
    if (!name) return;
    const id = kind === "node" ? `node:${token(name)}` : `component:${token(owner.componentType)}:${token(name)}`;
    if (!entities.has(id)) entities.set(id, { id, name, type: owner.componentType || "", kind, properties: [],
      inletNodes: owner.inletNodes || [], outletNodes: owner.outletNodes || [], nodePorts: owner.nodePorts || [],
      parentComponentName: owner.parentComponentName || "", parentComponentType: owner.parentComponentType || "" });
    candidates.push({ series, entity: entities.get(id), ...parts });
  };
  (loop.series || []).forEach((series) => add(series, "node"));
  (loop.components || []).forEach((component) => (component.series || []).forEach((series) => add(series, "component", component)));
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
    const id = `${item.entity.id}|${token(item.name)}|${token(item.unit)}`;
    if (seen.has(id)) continue;
    seen.add(id);
    item.entity.properties.push({ id, name: item.name, ...measurement(item.name, item.unit), series: item.series, entity: item.entity });
  }
  const model = { language, loop, primary, entities: [...entities.values()].filter((entity) => entity.properties.length), frames, byRow, byLabel, calendarValid };
  cache.set(loop, model);
  return model;
}

function calendarHour(label, leap) {
  const match = /^(?:(\d{4})[-/])?(\d{1,2})[-/](\d{1,2})\s+(\d{1,2}):(\d{2})(?::\d{2})?$/.exec(label.trim());
  if (!match) return null;
  const year = Number(match[1]) || (leap ? 2000 : 2001), month = Number(match[2]), day = Number(match[3]), hour = Number(match[4]), minute = Number(match[5]);
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
    const value = finite(point.value) && !(property.kind === "setpoint" && point.value <= -999) ? point.value * property.factor : null;
    values.set(frame.x, finite(value) ? value : null);
  }
  for (const x of duplicates) values.set(x, null);
  property.values = values;
  return values;
}

export function hvacInspectionTrace(model, property) {
  const values = propertyValues(model, property);
  return { id: property.id, label: `${property.entity.name} · ${property.label}`, unit: property.unit,
    ...(model.calendarValid ? { interval: 1 } : {}),
    points: model.frames.map((frame) => ({ x: frame.x, label: frame.label, value: values.get(frame.x) ?? null })) };
}

export function hvacInspectionSnapshots(model, frameIndex, selection = {}) {
  const frame = model.frames[frameIndex];
  const entities = model.entities.map((entity) => {
    const hasRelativeHumidity = entity.properties.some((property) => property.kind === "relativeHumidity");
    const properties = entity.properties.filter((property) => entity.kind === "node"
      ? ["flow", "temperature", "relativeHumidity", "setpoint"].includes(property.kind) || property.kind === "humidity" && !hasRelativeHumidity
      : ["power", "cop", "status", "cooling", "heating", "heatTransfer"].includes(property.kind));
    const metrics = properties.map((property) => ({ id: property.kind, label: property.label, unit: property.unit,
      value: frame ? propertyValues(model, property).get(frame.x) ?? null : null }));
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
