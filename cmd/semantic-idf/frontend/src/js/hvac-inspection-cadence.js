// Reuse already-loaded control observations only when the entire existing
// inspection can retain its properties at one proven cadence.
const token = (value) => String(value ?? "").trim().toLowerCase().replace(/\s+/g, " ");
const sourceKey = (source) => source ? [source.objectIndex, token(source.objectType), token(source.objectName)].join("\0") : "";
const frequency = (series) => token(series.reportingFrequency || /\(([^()]*)\)\s*$/.exec(series.column || "")?.[1]);
const timestep = (series) => ["timestep", "zone timestep"].includes(frequency(series));
const timestepIndexes = new WeakMap();
const identity = (series) => {
  const column = String(series.column || ""), split = column.indexOf(":");
  const key = series.keyValue || (split < 0 ? "" : column.slice(0, split));
  const name = series.variableName || series.name || (split < 0 ? column : column.slice(split + 1)).replace(/\s*\[[^\]]*\].*$/, "").replace(/\s*\([^)]*\)$/, "");
  return [token(key), token(name)].join("\0");
};

function profileSource(profile) {
  return profile.parameterControl?.nodes?.find((node) => node.code === "equipment_response")?.source
    || profile.availabilityLayers?.find((layer) => layer.role === "equipment")?.owner;
}

function currentProfiles(loop) {
  const loopID = loop.topology?.id || loop.loopId || "";
  const local = token(loop.loopType) === "zonehvac";
  return (loop.operatingProfiles || []).filter((profile) => loopID ? token(profile.loopId) === token(loopID) : local && !profile.loopId)
    .map((definition) => {
      const source = profileSource(definition);
      const sources = new Set([source, ...(definition.availabilityLayers || []).map((layer) => layer.owner),
        ...[definition.availabilityDefinitions, definition.onOff, definition.parameterControl].flatMap((graph) => (graph?.nodes || []).map((node) => node.source))].filter(Boolean).map(sourceKey));
      return { definition, source, sources };
    });
}

function timestepIndex(loop) {
  if (timestepIndexes.has(loop)) return timestepIndexes.get(loop);
  const profiles = currentProfiles(loop);
  const byIdentity = new Map(), sourceIDs = new Map();
  for (const series of loop.controlSeries || []) {
    if (series.sourceId) sourceIDs.set(series.sourceId, (sourceIDs.get(series.sourceId) || 0) + 1);
    if (!timestep(series)) continue;
    const key = identity(series), entries = byIdentity.get(key) || [];
    entries.push(series); byIdentity.set(key, entries);
  }
  const byOccurrence = new Map(), provenBindings = new Map();
  for (const profile of profiles) {
    const key = `${profile.definition.componentIndex}\0${token(profile.definition.loopId)}`, entries = byOccurrence.get(key) || [];
    entries.push(profile); byOccurrence.set(key, entries);
  }
  for (const binding of loop.controlSignals || []) {
    if (binding.status !== "collected" || !timestep(binding)) continue;
    const occurrences = byOccurrence.get(`${binding.componentIndex}\0${token(binding.loopId)}`) || [];
    if (occurrences.length !== 1 || !occurrences[0].sources.has(sourceKey(binding.source))) continue;
    const key = identity(binding), entries = provenBindings.get(key) || [];
    entries.push({ binding, profile: occurrences[0] }); provenBindings.set(key, entries);
  }
  const index = { byIdentity, sourceIDs, provenBindings };
  timestepIndexes.set(loop, index); return index;
}

export function selectHVACInspectionTimestepSeries(loop, properties) {
  if (!properties.length || !loop.controlSeries?.length) return null;
  const { byIdentity, sourceIDs, provenBindings } = timestepIndex(loop);
  const selected = new Map();
  for (const property of properties) {
    const candidates = byIdentity.get(identity(property.series)) || [];
    if (candidates.length !== 1) return null;
    const series = candidates[0];
    if (series.sourceId && sourceIDs.get(series.sourceId) !== 1) return null;
    const proven = (provenBindings.get(identity(series)) || []).some(({ binding, profile }) => (!binding.seriesId || binding.seriesId === series.sourceId)
      && (property.entity.kind !== "component" || profile.source?.objectIndex === profile.definition.componentIndex
        && token(profile.source.objectName) === token(property.entity.name) && token(profile.source.objectType) === token(property.entity.type)
        && sourceKey(binding.source) === sourceKey(profile.source)));
    if (!proven) return null;
    selected.set(property, series);
  }
  return selected;
}

const matchIndexes = new WeakMap();
function calendarTimestamp(label, leap) {
  const match = /^(?:(\d{4})[-/])?(\d{1,2})[-/](\d{1,2})\s+(\d{1,2}):(\d{2})(?::(\d{2}))?$/.exec(String(label || "").trim());
  if (!match) return null;
  const year = Number(match[1]) || (leap ? 2000 : 2001), month = Number(match[2]), day = Number(match[3]);
  const hour = Number(match[4]), minute = Number(match[5]), second = Number(match[6] || 0);
  if (month < 1 || month > 12 || day < 1 || day > new Date(Date.UTC(year, month, 0)).getUTCDate() || hour > 24 || minute > 59 || second > 59) return null;
  return Date.UTC(year, month - 1, day, hour, minute, second);
}

function timestampIndex(model) {
  if (matchIndexes.has(model)) return matchIndexes.get(model);
  const byTime = new Map(), byLabel = new Map(), leap = (model.frames || []).some((frame) => /02[\/-]29/.test(frame.label));
  for (const [index, frame] of (model.frames || []).entries()) {
    if (model.calendarValid && Number.isFinite(frame.x)) byTime.set(frame.x, byTime.has(frame.x) ? -1 : index);
    // Labels must denote a calendar time; a row number is not a timestamp.
    const label = calendarTimestamp(frame.label, leap);
    if (label === null) continue;
    byLabel.set(label, byLabel.has(label) ? -1 : index);
  }
  const result = { byTime, byLabel, leap };
  matchIndexes.set(model, result); return result;
}

export function matchHVACRuntimeFrame(runtimeModel, inspectionModel, frameIndex) {
  const frame = inspectionModel?.frames?.[frameIndex];
  if (!frame || !runtimeModel?.frames?.length) return -1;
  const inspected = timestampIndex(inspectionModel), runtime = timestampIndex(runtimeModel);
  const label = calendarTimestamp(frame.label, inspected.leap);
  if (label === null || inspected.byLabel.get(label) !== frameIndex) return -1;
  if (inspectionModel.calendarValid && runtimeModel.calendarValid) {
    if (inspected.byTime.get(frame.x) !== frameIndex) return -1;
    const matched = runtime.byTime.get(frame.x) ?? -1;
    return matched >= 0 && runtime.byLabel.get(calendarTimestamp(runtimeModel.frames[matched].label, runtime.leap)) === matched
      && calendarTimestamp(runtimeModel.frames[matched].label, runtime.leap) === label ? matched : -1;
  }
  return runtime.byLabel.get(label) ?? -1;
}
