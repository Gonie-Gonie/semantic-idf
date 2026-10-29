import { getLanguage, t } from "./i18n.js";
import { simulationSeriesPointColumns } from "./simulation-result-transport.js";
import { hasHVACTemperatureSetpoint } from "./hvac-setpoint.js";
import { heatFlowCategoryLabel } from "./heat-flow-data.js";
import { energyPathSeriesID } from "./energy-path-navigation.js";

const finite = value => typeof value === "number" && Number.isFinite(value);
const token = value => String(value ?? "").trim().toLowerCase().replace(/\s+/g, " ");
const text = value => String(value ?? "").trim();
const list = value => Array.isArray(value) ? value : [];
const unique = values => values.length === 1 ? values[0] : null;
const models = new WeakMap(), metadataByResult = new WeakMap(), loaders = new WeakMap(), observations = new WeakMap(), axes = new WeakMap(), selections = new WeakMap(), frameTimes = new WeakMap(), uiScopes = new WeakMap();
let axisNumber = 0;
const axisID = object => { if (!axes.has(object)) axes.set(object, ++axisNumber); return axes.get(object); };
const copy = (key, fallback) => t(`simulation.comparison${key}`, {}, fallback);
const purpose = { energy: "basic_energy", zone_heat_flow: "zone_heat_flow", comfort: "comfort_check" };
const weather = new Map([
  ["site outdoor air drybulb temperature", ["OutdoorTemperature", "Outdoor temperature"]],
  ["site outdoor air wetbulb temperature", ["OutdoorWetbulb", "Outdoor wet-bulb"]],
  ["site outdoor air relative humidity", ["OutdoorHumidity", "Outdoor RH"]],
  ["site outdoor air barometric pressure", ["OutdoorPressure", "Air pressure"]],
  ["site direct solar radiation rate per area", ["DirectSolar", "Direct solar"]],
  ["site diffuse solar radiation rate per area", ["DiffuseSolar", "Diffuse solar"]],
  ["site wind speed", ["WindSpeed", "Wind speed"]],
  ["site wind direction", ["WindDirection", "Wind direction"]],
]);
const frequency = value => ({ "zone timestep": "timestep", "hvac system timestep": "detailed", "system timestep": "detailed" }[token(value)] || token(value));
const file = value => token(value).replace(/\\/g, "/");
const basename = value => file(value).split("/").at(-1);
const sourceName = (source, metadata) => metadata.headers.get(source)?.name ?? (text(source.name || source.variableName) || text(source.column).replace(/^[^:]*:/, "").replace(/\s*\[[^\]]*\].*$/, ""));
const sourceFrequency = (source, metadata) => metadata.headers.get(source)?.frequency ?? text(source.reportingFrequency || /\(([^()]*)\)\s*$/.exec(source.column || "")?.[1]);
const sourceKey = (source, metadata) => metadata.headers.get(source)?.key ?? (text(source.keyValue) || text(source.column).split(":")[0]);
const sourceIsMeter = (source, metadata) => metadata.headers.get(source)?.meter ?? Boolean(source.isMeter);
const sourceFile = source => text(source.file || source.sourceFile || source.source);
const seriesID = source => source?.sourceId || (source ? energyPathSeriesID(source) : "");
const sourceSignature = (source, metadata) => JSON.stringify([file(sourceFile(source)), token(sourceKey(source, metadata)), token(sourceName(source, metadata)), frequency(sourceFrequency(source, metadata)), Boolean(source.isMeter)]);

function displayUnit(raw) {
  const value = text(raw), normalized = token(value);
  if (["c", "degc", "°c"].includes(normalized)) return { unit: "°C", factor: 1 };
  if (normalized === "w") return { unit: "kW", factor: .001 };
  if (normalized === "j") return { unit: "kWh", factor: 1 / 3600000 };
  if (normalized === "j/kg") return { unit: "kJ/kg", factor: .001 };
  if (/kg.*\/kg/i.test(value)) return { unit: "g/kg", factor: 1000 };
  return { unit: value.replace(/w\/m2/i, "W/m²") || "—", factor: 1 };
}

function rawColumns(series, timelineOnly = false) {
  const shared = simulationSeriesPointColumns(series, { timelineOnly });
  if (shared) return shared;
  const raw = list(series.points), display = timelineOnly ? null : series.displayPoints;
  const points = Array.isArray(display) && display.length === raw.length ? display : raw;
  return { rows: points.map(point => point.x), labels: points.map(point => String(point.label ?? "")), ...(timelineOnly ? {} : { values: points.map(point => point.value) }) };
}

function requestMetadata(request) {
  const meter = /output:meter/i.test(request.objectType || ""), fields = list(request.fields);
  return { meter, name: request.variableName || request.meterName || (meter ? request.keyValue : "") || fields[meter ? 0 : 1]?.value,
    key: request.keyValue || (meter ? "" : fields[0]?.value), frequency: request.reportingFrequency || fields[meter ? 1 : 2]?.value };
}

function appendIndex(index, key, value) {
  if (!index.has(key)) index.set(key, []);
  index.get(key).push(value);
}

const requestSignature = (cadence, meter, name) => JSON.stringify([frequency(cadence), meter, token(name)]);
const sourceMatchKey = (name, key) => JSON.stringify([token(name), token(key)]);

// The run catalogs are immutable. Resolve metadata once per result rather than
// rescanning a multi-thousand-entry output plan for every source, view and Zone.
// This index never reads points, native intervals, or any deferred data column.
function prepareComparisonMetadata(result) {
  let metadata = metadataByResult.get(result);
  if (metadata) return metadata;
  metadata = { sources: list(result.series), headers: new Map(), requests: new Map(), matches: new Map(), native: new Map(), files: new Map(), counts: new Map(), byPurpose: new Map() };
  list(result.purposeRunPlan?.outputObjects).forEach((request, position) => {
    const wanted = requestMetadata(request), entry = { wanted, position, purposes: new Set(list(request.purposeIds)), scopeZoneName: token(request.scopeZoneName) };
    const signature = requestSignature(wanted.frequency, wanted.meter, wanted.name);
    let bucket = metadata.requests.get(signature);
    if (!bucket) { bucket = { all: [], any: [], keys: new Map() }; metadata.requests.set(signature, bucket); }
    bucket.all.push(entry);
    if (wanted.meter || !wanted.key || wanted.key === "*") bucket.any.push(entry);
    else appendIndex(bucket.keys, token(wanted.key), entry);
    for (const id of entry.purposes) appendIndex(metadata.byPurpose, id, entry);
  });
  for (const source of metadata.sources) {
    resolveHeader(source, metadata);
    const name = basename(sourceFile(source)), entries = metadata.files.get(name) || new Set(); entries.add(file(sourceFile(source))); metadata.files.set(name, entries);
    if (source.sourceId) metadata.counts.set(source.sourceId, (metadata.counts.get(source.sourceId) || 0) + 1);
    appendIndex(metadata.native, sourceMatchKey(sourceName(source, metadata), sourceKey(source, metadata)), source);
  }
  metadataByResult.set(result, metadata);
  return metadata;
}

function requestCandidates(metadata, cadence, meter, name, key) {
  const bucket = metadata.requests.get(requestSignature(cadence, meter, name));
  if (!bucket) return [];
  if (key === undefined || meter) return bucket.all;
  return [...bucket.any, ...(bucket.keys.get(token(key)) || [])].sort((a, b) => a.position - b.position);
}

function resolveHeader(source, metadata) {
  const cadence = text(source.reportingFrequency || /\(([^()]*)\)\s*$/.exec(source.column || "")?.[1]);
  const bare = text(source.column).replace(/\s*\([^()]*\)\s*$/, "").replace(/\s*\[[^\]]*\]\s*$/, "").trim(), candidates = new Map();
  const name = text(source.name || source.variableName), entries = [];
  if (source.isMeter !== false) entries.push(...requestCandidates(metadata, cadence, true, name || bare));
  if (source.isMeter !== true) {
    if (name) entries.push(...requestCandidates(metadata, cadence, false, name, text(source.keyValue) || undefined));
    else {
      // Legacy columns may contain colons within both the key and variable
      // name. Retain every possible suffix and the original ambiguity rules.
      const suffixes = new Set();
      for (let offset = bare.indexOf(":"); offset >= 0; offset = bare.indexOf(":", offset + 1)) suffixes.add(bare.slice(offset + 1));
      for (const suffix of suffixes) entries.push(...requestCandidates(metadata, cadence, false, suffix, text(source.keyValue) || undefined));
    }
  }
  entries.sort((a, b) => a.position - b.position);
  for (const { wanted } of entries) {
    const name = text(wanted.name);
    if (!name || frequency(wanted.frequency) !== frequency(cadence) || typeof source.isMeter === "boolean" && source.isMeter !== wanted.meter) continue;
    let key = text(source.keyValue);
    if (wanted.meter) {
      if (text(source.name || source.variableName) ? token(source.name || source.variableName) !== token(name) : token(bare) !== token(name)) continue;
      key = "";
    } else {
      if (text(source.name || source.variableName)) { if (token(source.name || source.variableName) !== token(name)) continue; if (!key && token(bare).endsWith(":" + token(name))) key = bare.slice(0, bare.length - name.length - 1).trim(); }
      else { const suffix = ":" + token(name); if (!token(bare).endsWith(suffix)) continue; key = bare.slice(0, bare.length - name.length - 1).trim(); }
      if (wanted.key && wanted.key !== "*" && token(wanted.key) !== token(key)) continue;
    }
    const found = { name: text(source.name || source.variableName) || name, key, frequency: cadence, meter: wanted.meter };
    candidates.set(JSON.stringify(found), found);
  }
  if (candidates.size === 1) metadata.headers.set(source, [...candidates.values()][0]);
  else if (candidates.size > 1) metadata.headers.set(source, { name: "", key: "", frequency: cadence, meter: false, ambiguous: true });
}

function matchingRequests(metadata, view, source) {
  let byView = metadata.matches.get(source);
  if (!byView) { byView = new Map(); metadata.matches.set(source, byView); }
  if (!byView.has(view)) {
    const entries = requestCandidates(metadata, sourceFrequency(source, metadata), sourceIsMeter(source, metadata), sourceName(source, metadata), sourceKey(source, metadata));
    byView.set(view, entries.filter(entry => entry.purposes.has(purpose[view])));
  }
  return byView.get(view);
}

function matchingSource(model, name, key, source, cadence = "") {
  return unique((model.metadata.native.get(sourceMatchKey(name, key)) || []).filter(item =>
    (!source || model.fileKey(sourceFile(item)) === model.fileKey(source)) && (!cadence || frequency(sourceFrequency(item, model.metadata)) === frequency(cadence))));
}

function sourceMetadata(model, series, rows) {
  let entries = series?.nativeIntervals;
  if (!Array.isArray(entries) && series?.nativeIntervalsSourceId) {
    const owner = unique(model.sources.filter(item => item.sourceId === series.nativeIntervalsSourceId));
    if (owner && model.fileKey(sourceFile(owner)) === model.fileKey(sourceFile(series))) entries = owner.nativeIntervals;
  }
  return Array.isArray(entries) && entries.length === rows.length ? entries : [];
}

function rawObservations(model, source, factor) {
  const columns = rawColumns(source), metadata = sourceMetadata(model, source, columns.rows);
  const unset = /setpoint.*temperature|temperature.*setpoint/i.test(sourceName(source, model.metadata));
  return columns.rows.map((row, index) => ({ rawX: row, label: String(columns.labels[index] ?? ""),
    value: finite(columns.values[index]) && (!unset || hasHVACTemperatureSetpoint(columns.values[index])) ? columns.values[index] * factor : null,
    metadata: metadata[index]?.timeIndex === row ? metadata[index] : undefined, index }));
}

function add(model, category, owner, identity, metadata, load) {
  const entityID = JSON.stringify([category, owner]), id = JSON.stringify([category, owner, identity]);
  if (model.propertyIDs.has(id)) return;
  model.propertyIDs.add(id);
  let entity = model.entities.find(item => item.id === entityID);
  if (!entity) { entity = { id: entityID, kind: category, category, name: owner, properties: [] }; model.entities.push(entity); }
  const property = { id, entityId: entityID, category, seriesId: "", sourceIds: [], ...metadata };
  loaders.set(property, load); entity.properties.push(property); model.properties.push(property);
  model.seriesIDs.add(property.seriesId);
}

function addRaw(model) {
  const { metadata } = model, counts = metadata.counts;
  const zone = model.view === "energy" && model.context.simulationEnergyScopeKind === "zone" ? token(model.context.simulationEnergyZoneName) : "";
  for (const source of model.sources) {
    if (metadata.headers.get(source)?.ambiguous) continue;
    if (source.sourceId && counts.get(source.sourceId) !== 1) continue;
    const name = sourceName(source, metadata), key = sourceKey(source, metadata), requests = matchingRequests(metadata, model.view, source);
    const environment = token(key) === "environment" && weather.has(token(name)) && frequency(sourceFrequency(source, metadata)) === "hourly";
    if (!requests.length) continue;
    if (model.seriesIDs.has(seriesID(source))) continue;
    if (zone && !environment && token(key) !== zone && !requests.some(request => request.scopeZoneName === zone)) continue;
    if (!environment && model.view === "zone_heat_flow" && token(key) !== token(model.scopeZone) && !requests.some(request => request.scopeZoneName === token(model.scopeZone))) continue;
    if (!environment && model.view === "comfort") {
      const comfort = model.result.purposeResults?.comfort || {}, scope = model.scopeZone === "__building__" ? { metrics: comfort.buildingMetrics } : list(comfort.zones).find(item => token(item.zoneName) === token(model.scopeZone));
      const membership = list(scope?.metrics).some(metric => token(metric.keyValue || model.scopeZone) === token(key));
      if (!membership && (model.scopeZone === "__building__" || token(key) !== token(model.scopeZone))) continue;
    }
    const units = displayUnit(source.displayUnit || /\[([^\]]*)\]/.exec(source.displayColumn || source.column || "")?.[1] || source.unit);
    add(model, environment ? "environment" : "reported", key || "Building", source.sourceId || sourceSignature(source, metadata), {
      name, label: environment ? copy(...weather.get(token(name))) : name, kind: "reported", unit: units.unit, frequency: sourceFrequency(source, metadata), seriesId: seriesID(source),
      sourceIds: [seriesID(source)], file: model.fileKey(sourceFile(source)), axis: `source:${seriesID(source)}`,
    }, () => rawObservations(model, source, units.factor));
  }
}

function comfortKind(name) {
  if (/energy|\bpower\b|heating rate|cooling rate|heat transfer/i.test(name)) return "";
  if (/\bpmv\b|predicted mean vote/i.test(name)) return "pmv";
  if (/\bppd\b|predicted percentage.*dissatisfied/i.test(name)) return "ppd";
  if (/setpoint.*temperature|temperature.*setpoint/i.test(name)) return /heating/i.test(name) ? "heatingSetpoint" : /cooling/i.test(name) ? "coolingSetpoint" : "setpoint";
  if (/temperature/i.test(name)) return "temperature";
  if (/relative humidity/i.test(name)) return "relativeHumidity";
  if (/humidity ratio/i.test(name)) return "humidity";
  if (/not met|unmet|discomfort|not comfortable/i.test(name)) return "unmet";
  return "";
}

function addComfort(model) {
  const comfort = model.result.purposeResults?.comfort || {};
  const scopes = model.scopeZone === "__building__" ? [{ zoneName: "Building", metrics: comfort.buildingMetrics }]
    : list(comfort.zones).filter(scope => token(scope.zoneName) === token(model.scopeZone));
  for (const scope of scopes) for (const metric of list(scope.metrics)) {
    const kind = comfortKind(metric.name || ""); if (!kind) continue;
    const units = displayUnit(metric.unit), name = metric.name || "", key = metric.keyValue || scope.zoneName;
    const native = matchingSource(model, name, key, metric.source, metric.reportingFrequency);
    const labels = { pmv: "PMV", ppd: "PPD", temperature: "T", heatingSetpoint: "Tset,h", coolingSetpoint: "Tset,c", relativeHumidity: "RH", humidity: "w" };
    add(model, "comfort", scope.zoneName, [name, key, metric.source, metric.reportingFrequency], {
      name, kind, label: (labels[kind] || name) + (metric.keyValue && token(metric.keyValue) !== token(scope.zoneName) ? ` · ${metric.keyValue}` : ""),
      unit: units.unit, frequency: metric.reportingFrequency || (native && sourceFrequency(native, model.metadata)) || "",
      seriesId: seriesID(native), sourceIds: native ? [seriesID(native)] : [], file: model.fileKey(metric.source), axis: `comfort:${scope.zoneName}`,
    }, () => {
      const points = list(metric.points), metadata = native ? sourceMetadata(model, native, points.map(point => point.x)) : [];
      return points.map((point, index) => ({ rawX: point.x, label: String(point.label ?? ""), value: finite(point.value) && point.value > -999 ? point.value * units.factor : null,
        metadata: metadata[index]?.timeIndex === point.x ? metadata[index] : undefined, index }));
    });
  }
}

function addHeatFlow(model) {
  const dataset = model.dataset; if (!dataset) return;
  const requested = model.metadata.byPurpose.get("zone_heat_flow") || [];
  const names = new Set(list(dataset.categories).map(category => token(category.variableName)));
  const declared = new Set(requested.filter(entry => names.has(token(entry.wanted.name))).map(entry => frequency(entry.wanted.frequency)).filter(Boolean));
  const cadence = declared.size === 1 ? [...declared][0] : "";
  for (const zone of list(dataset.zones)) {
    if (token(zone.name) !== token(model.scopeZone)) continue;
    for (const [index, category] of list(dataset.categories).entries()) {
      const native = matchingSource(model, category.variableName, zone.name, dataset.sourceFile, cadence);
      const units = displayUnit(dataset.unit || category.unit || "W");
      add(model, "heat_flow", zone.name, category.id, { name: category.variableName, kind: category.id, label: heatFlowCategoryLabel(category), unit: units.unit, frequency: cadence || (native && sourceFrequency(native, model.metadata)) || "",
        seriesId: seriesID(native), sourceIds: native ? [seriesID(native)] : [], file: model.fileKey(dataset.sourceFile), axis: `heat-flow:${axisID(dataset)}` }, () => {
        const labels = list(dataset.labels), values = zone.values?.[index] || [], observed = zone.observed?.[index];
        return labels.map((label, frame) => ({ label: String(label), rawX: frame, index: frame,
          value: (!observed || observed[frame] === true) && finite(values[frame]) ? values[frame] * units.factor : null }));
      });
    }
    const units = displayUnit(dataset.temperatureUnit || "C");
    add(model, "heat_flow", zone.name, "temperature", { name: "Zone Mean Air Temperature", kind: "temperature", label: "Temperature", unit: units.unit,
      frequency: cadence, file: model.fileKey(dataset.sourceFile), axis: `heat-flow:${axisID(dataset)}` }, () => {
      const values = zone.temperature || [], observed = zone.temperatureObserved;
      return list(dataset.labels).map((label, frame) => ({ label: String(label), rawX: frame, index: frame, value: (!observed || observed[frame] === true) && finite(values[frame]) ? values[frame] * units.factor : null }));
    });
  }
}

function addEnergy(model) {
  const explanation = model.result.purposeResults?.energyExplanation || {}, zone = model.context.simulationEnergyScopeKind === "zone" ? token(model.context.simulationEnergyZoneName) : "";
  const counts = new Map(); for (const source of list(explanation.sources)) counts.set(source.id, (counts.get(source.id) || 0) + 1);
  for (const source of list(explanation.sources)) {
    if (!source.id || counts.get(source.id) !== 1 || source.hourlyEnergy?.basis !== "reported_source" || token(source.hourlyEnergy.unit) !== "kwh") continue;
    if (zone && token(source.zoneName || source.keyValue) !== zone) continue;
    add(model, "energy_path", source.keyValue || source.zoneName || "Building", source.id, { name: source.name, label: source.name || source.id, kind: "energy", unit: "kWh", frequency: "Hourly",
      seriesId: source.id, sourceIds: [source.id], file: model.fileKey(sourceFile(source) || "eplusout.sql"), axis: `energy:${source.id}` }, () => {
      const labels = list(explanation.hourlyLabels), values = source.hourlyEnergy.values;
      if (!Array.isArray(values) || values.length !== labels.length) return [];
      return labels.map((label, index) => ({ label: String(label), rawX: index, index, value: finite(values[index]) ? values[index] : null }));
    });
  }
}

/** Metadata only: catalog creation and selection never read observation columns. */
export function prepareSimulationComparison(result = {}, view = "energy", context = {}) {
  const normalized = view === "heatflow" ? "zone_heat_flow" : view;
  let cached = models.get(result); if (!cached) { cached = new Map(); models.set(result, cached); }
  const dataset = result.purposeResults?.zoneHeatFlow?.zones?.length ? result.purposeResults.zoneHeatFlow : result.heatFlow;
  const scopeZone = normalized === "comfort" ? context.simulationComfortZone || result.purposeResults?.comfort?.zones?.[0]?.zoneName || "__building__" : normalized === "zone_heat_flow" ? context.simulationHeatFlowSelectedZone || dataset?.zones?.[0]?.name || "" : context.simulationEnergyScopeKind === "zone" ? context.simulationEnergyZoneName || "" : "";
  const key = JSON.stringify([getLanguage(), normalized, normalized === "energy" ? context.simulationEnergyScopeKind : "", scopeZone]);
  if (cached.has(key)) { const model = cached.get(key); model.context = { ...context }; return model; }
  const metadata = prepareComparisonMetadata(result), { sources, files } = metadata;
  const model = { result, view: normalized, context: { ...context }, scopeZone, scopeKey: key, metadata, sources, entities: [], properties: [], categories: [], dataset, propertyIDs: new Set(), seriesIDs: new Set(),
    fileKey(value) { const candidates = files.get(basename(value)); return candidates?.size === 1 ? [...candidates][0] : file(value); } };
  if (normalized === "comfort") addComfort(model);
  if (normalized === "zone_heat_flow") addHeatFlow(model);
  if (normalized === "energy") addEnergy(model);
  addRaw(model);
  const names = { energy_path: "Energy Path", heat_flow: copy("HeatFlow", "Heat flow"), comfort: "Comfort", environment: copy("Environment", "Environment"), reported: copy("Reported", "Reported outputs") };
  model.categories = [...new Set(model.entities.map(entity => entity.category))].map(id => ({ id, label: names[id] || id }));
  if (cached.size >= 8) cached.delete(cached.keys().next().value);
  cached.set(key, model); return model;
}

export function normalizeSimulationComparison(model, ui = {}) {
  ui.mode = ["scatter", "histogram"].includes(ui.mode) ? ui.mode : "line";
  const scopeChanged = uiScopes.has(ui) && uiScopes.get(ui) !== model.scopeKey;
  const zone = token(model.scopeZone);
  const preferred = model.entities.find(entity => token(entity.name) === zone && entity.category !== "reported") || model.entities.find(entity => entity.category !== "reported" && entity.category !== "environment");
  const first = preferred?.properties.find(property => property.kind === "temperature") || preferred?.properties[0] || model.properties[0];
  const second = model.entities.find(entity => entity.category === "environment")?.properties.find(property => /drybulb/i.test(property.name)) || model.properties.find(property => property !== first);
  if (!Array.isArray(ui.rows)) {
    ui.rows = [first, second].map(property => ({ entity: property?.entityId || "", property: property?.id || "" }));
  }
  const units = new Set(), selected = new Set();
  ui.rows = ui.rows.slice(0, ui.mode === "scatter" ? 2 : 8).map((row, index) => {
    let entity = model.entities.find(item => item.id === row.entity), property = entity?.properties.find(item => item.id === row.property);
    if (!entity && row.property && scopeChanged) { property = index === 0 ? first : second; entity = model.entities.find(item => item.id === property?.entityId); }
    if (!property || selected.has(property.id) || ui.mode === "line" && units.size >= 2 && !units.has(property.unit)) return { entity: entity?.id || "", property: "" };
    units.add(property.unit); selected.add(property.id); return { entity: entity.id, property: property.id };
  });
  while (ui.rows.length < 2) ui.rows.push({ entity: "", property: "" });
  uiScopes.set(ui, model.scopeKey);
  return ui;
}

export function selectedSimulationComparisonSeriesIDs(result, view, ui = {}, context = {}) {
  const model = prepareSimulationComparison(result, view, context); normalizeSimulationComparison(model, ui);
  return [...new Set(ui.rows.flatMap(row => model.properties.find(property => property.id === row.property)?.sourceIds || []))];
}

const stampPattern = /^(?:(\d{4})[-/])?(\d{1,2})[-/](\d{1,2})\s+(\d{1,2}):(\d{2})(?::(\d{2}))?$/;
const monthNames = ["january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december"];
function dateParts(label, cadence = "") {
  const match = stampPattern.exec(String(label).trim());
  if (match) return { year: Number(match[1]) || 0, month: Number(match[2]), day: Number(match[3]), hour: Number(match[4]), minute: Number(match[5]), second: Number(match[6] || 0) };
  if (frequency(cadence) !== "monthly") return null;
  const name = token(label), numeric = /^(?:(\d{4})[-/])?m?(\d{1,2})$/.exec(name), month = numeric ? Number(numeric[2]) : monthNames.findIndex(value => value === name || value.slice(0, 3) === name) + 1;
  return month >= 1 && month <= 12 ? { year: numeric ? Number(numeric[1]) || 0 : 0, month, day: 1, hour: 0, minute: 0, second: 0, monthOnly: true } : null;
}
function clock(parts, year) {
  if (!parts || parts.month < 1 || parts.month > 12 || parts.day < 1 || parts.hour > 24 || parts.minute > 59 || parts.second > 59 || parts.hour === 24 && (parts.minute || parts.second)) return null;
  const date = new Date(0); date.setUTCFullYear(parts.year || year, parts.month - 1, parts.day);
  if (date.getUTCMonth() !== parts.month - 1 || date.getUTCDate() !== parts.day) return null;
  if (parts.monthOnly) date.setUTCMonth(parts.month, 1); else date.setUTCHours(parts.hour, parts.minute, parts.second, 0);
  return date.getTime() / 3600000;
}

function monthlyMinutes(parts, year) {
  if (!parts) return undefined;
  const days = new Date(Date.UTC(parts.year || year, parts.month, 0)).getUTCDate();
  return parts.monthOnly || parts.day === days && parts.hour === 24 && parts.minute === 0 && parts.second === 0 ? days * 1440 : undefined;
}

export function simulationComparisonTraces(model, properties) {
  const selectionKey = JSON.stringify([properties.filter(Boolean).map(property => property.id), model.view === "energy" ? model.context.simulationEnergyPeriod : "", model.view === "zone_heat_flow" ? [model.context.simulationHeatFlowRangeStart, model.context.simulationHeatFlowRangeEnd] : []]), previousSelection = selections.get(model);
  if (previousSelection?.key === selectionKey) return previousSelection.traces;
  const selected = properties.filter(Boolean).map(property => {
    if (!observations.has(property)) observations.set(property, loaders.get(property)?.() || []);
    return { property, points: observations.get(property) };
  });
  const years = new Set(), environments = new Map(); let leap = false;
  for (const { property, points } of selected) for (const point of points) {
    const parts = dateParts(point.label, property.frequency); if (parts?.year) years.add(parts.year); if (parts?.month === 2 && parts.day === 29) leap = true;
    if (point.metadata?.year > 0) years.add(point.metadata.year);
    if (point.metadata?.environmentPeriodIndex > 0) { const values = environments.get(property.file) || new Set(); values.add(point.metadata.environmentPeriodIndex); environments.set(property.file, values); }
  }
  const year = years.size === 1 ? [...years][0] : leap ? 2000 : 2001;
  let low = -Infinity, high = Infinity;
  if (model.view === "zone_heat_flow") {
    const labels = list(model.dataset?.labels), start = Number(model.context.simulationHeatFlowRangeStart), end = Number(model.context.simulationHeatFlowRangeEnd);
    if (Number.isInteger(start) && Number.isInteger(end) && start >= 0 && end >= start && end < labels.length) { low = clock(dateParts(labels[start]), year) ?? low; high = clock(dateParts(labels[end]), year) ?? high; }
  }
  const month = model.view === "energy" && /^m([1-9]|1[0-2])$/i.exec(model.context.simulationEnergyPeriod || "");
  const traces = selected.map(({ property, points }) => {
    const byTime = new Map(), cadence = frequency(property.frequency), fixed = { hourly: 60, daily: 1440 }[cadence]; let previous = -Infinity, valid = true;
    for (const point of points) {
      const parts = dateParts(point.label, cadence), x = parts && (parts.year || years.size <= 1) ? clock(parts, year) : null;
      if (!finite(x) || x < previous) { valid = false; break; } previous = x;
      const metadata = point.metadata, duration = finite(metadata?.intervalMinutes) && metadata.intervalMinutes > 0 ? metadata.intervalMinutes : fixed || (cadence === "monthly" ? monthlyMinutes(parts, year) : undefined);
      const ownEnvironments = environments.get(property.file), environment = metadata?.environmentPeriodIndex || (ownEnvironments?.size === 1 ? [...ownEnvironments][0] : "");
      const temporal = cadence && duration ? `${cadence}:${duration}` : `axis:${property.axis}`;
      const entry = { x, label: point.label, value: point.value, key: JSON.stringify([property.file, temporal, environment, x]) };
      if (byTime.has(x)) byTime.set(x, { ...entry, value: null, key: `ambiguous:${property.id}:${x}` }); else byTime.set(x, entry);
      if (x < low || x > high || month && parts.month !== Number(month[1])) byTime.delete(x);
    }
    return { id: property.id, label: `${model.entities.find(entity => entity.id === property.entityId)?.name || ""} · ${property.label}`, unit: property.unit,
      axisLabel: property.label, ...(fixed || cadence === "monthly" ? { interval: cadence === "monthly" ? 31 * 24 : fixed / 60 } : {}), points: valid ? [...byTime.values()] : [] };
  });
  selections.set(model, { key: selectionKey, traces }); return traces;
}
export function simulationComparisonTrace(model, property) { return simulationComparisonTraces(model, [property])[0]; }
export function simulationComparisonFrame(model, traces, context = model.context) {
  if (model.view !== "zone_heat_flow" || !Number.isInteger(Number(context.simulationHeatFlowFrameIndex))) return undefined;
  const label = list(model.dataset?.labels)[Number(context.simulationHeatFlowFrameIndex)];
  if (!label) return undefined;
  const signature = parts => `${parts.month}:${parts.day}:${parts.hour}:${parts.minute}:${parts.second}`;
  let times = frameTimes.get(traces);
  if (!times) {
    times = new Map();
    for (const trace of traces) for (const point of trace.points) {
      const parts = dateParts(point.label); if (!parts) continue;
      for (const key of [signature(parts), `${new Date(point.x * 3600000).getUTCFullYear()}:${signature(parts)}`]) {
        const entries = times.get(key) || new Set(); entries.add(point.x); times.set(key, entries);
      }
    }
    frameTimes.set(traces, times);
  }
  const match = dateParts(label), entries = match && times.get(`${match.year ? match.year + ":" : ""}${signature(match)}`);
  return entries?.size === 1 ? [...entries][0] : undefined;
}
