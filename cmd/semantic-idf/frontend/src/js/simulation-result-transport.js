export const simulationResultTransferMediaType = "application/vnd.semantic-idf.simulation-transfer+json";

const transferSchema = "semantic-idf.simulation-transfer/v1";
const unsafePathKeys = new Set(["__proto__", "prototype", "constructor"]);
const object = (value) => value !== null && typeof value === "object";
const index = (value, length) => Number.isInteger(value) && value >= 0 && value < length;
const validatedColumns = new WeakMap();
const pointSources = new WeakMap(), validatedTimelines = new WeakSet(), validatedPointSets = new WeakSet();
const invalidColumn = () => { throw new Error("Invalid simulation result transfer data"); };

// Read shared columns without materializing an object for every observation.
// Canonical results and already materialized sets retain their public arrays.
export function simulationSeriesPointColumns(series, { timelineOnly = false } = {}) {
  const sources = pointSources.get(series);
  if (!sources?.points) return null;
  const raw = sources.points(timelineOnly);
  if (!raw) return null;
  const display = sources.displayPoints?.(timelineOnly);
  return display && display.rows.length === raw.rows.length ? display : raw;
}

function validateColumnValues(kind, values) {
  if (!["number", "boolean", "string"].includes(kind) || !Array.isArray(values)) invalidColumn();
  for (const value of values) if (typeof value !== kind || kind === "number" && !Number.isFinite(value)) invalidColumn();
}

function columnValues(set) {
  const values = set.values, previous = validatedColumns.get(set);
  if (!previous || previous.values !== values || previous.kind !== set.kind) {
    validateColumnValues(set.kind, values);
    validatedColumns.set(set, { kind: set.kind, values });
  }
  return values;
}

// Resource readers validate each bounded chunk once, before a UI needs it.
// The cache is shared by every canonical binding of this immutable column.
// Completion requires nonoverlapping coverage; no pending getter is read here.
export function createSimulationColumnValidation(set, length) {
  const kind = set.kind, ranges = [];
  let count = 0;
  return (values, offset, assembled) => {
    validateColumnValues(kind, values);
    const end = offset + values.length;
    if (set.kind !== kind || !Number.isSafeInteger(offset) || offset < 0 || end > length ||
      !Array.isArray(assembled) || assembled.length !== length || ranges.some(range => offset < range.end && end > range.start)) invalidColumn();
    ranges.push({ start: offset, end }); count += values.length;
    if (count === length) validatedColumns.set(set, { kind, values: assembled });
  };
}

// Keep the canonical result shape. Repeated heat-flow/topology columns share
// their exact arrays; untouched series keep their numeric columns until opened.
export function decodeSimulationResultTransfer(payload, { deferData = false } = {}) {
  if (!object(payload) || !String(payload.schema || "").startsWith("semantic-idf.simulation-transfer/")) return payload;
  if (payload.schema !== transferSchema) throw new Error("Unsupported simulation result transfer format");
  const { result, timelines, pointSets, pointBindings } = payload;
  const { columnSets = [], columnBindings = [] } = payload;
  const invalid = () => { throw new Error("Invalid simulation result transfer data"); };
  if (!object(result) || Array.isArray(result) || !Array.isArray(timelines) || !Array.isArray(pointSets) || !Array.isArray(pointBindings) || !Array.isArray(columnSets) || !Array.isArray(columnBindings)) invalid();
  for (const timeline of timelines) {
    if (!object(timeline) || (!deferData && (!Array.isArray(timeline.x) || !Array.isArray(timeline.labels) || timeline.x.length !== timeline.labels.length))) invalid();
  }
  for (const set of pointSets) {
    if (!object(set) || !index(set.timeline, timelines.length) || (!deferData && (!Array.isArray(set.values) || set.values.length !== timelines[set.timeline].x.length))) invalid();
  }
  for (const set of columnSets) {
    if (!object(set) || !["number", "boolean", "string"].includes(set.kind)) invalid();
    if (deferData) continue;
    columnValues(set);
  }
  const bound = new WeakMap();
  const resolve = (binding, sets) => {
    if (!object(binding) || !Array.isArray(binding.path) || !binding.path.length || !index(binding.data, sets.length)) invalid();
    let owner = result;
    for (let offset = 0; offset < binding.path.length; offset++) {
      const key = binding.path[offset], last = offset === binding.path.length - 1;
      if (!object(owner) || (Array.isArray(owner) ? !index(key, owner.length) : typeof key !== "string" || !key || unsafePathKeys.has(key))) invalid();
      if (last) {
        // Go's omitempty can omit a compacted array member.
        if (Object.hasOwn(owner, key) && owner[key] !== null) invalid();
        const keys = bound.get(owner) || new Set();
        if (keys.has(key)) invalid();
        keys.add(key); bound.set(owner, keys);
        return { owner, key, data: binding.data };
      } else {
        if (!Object.hasOwn(owner, key)) invalid();
        owner = owner[key];
      }
    }
  };
  // Resolve every path before changing any target, including bindings across
  // both tables. A malformed response cannot overwrite existing result data.
  const pointTargets = pointBindings.map(binding => resolve(binding, pointSets));
  const columnTargets = columnBindings.map(binding => resolve(binding, columnSets));
  const pointsBySet = new Array(pointSets.length);
  const materialize = (data) => {
    if (pointsBySet[data]) return pointsBySet[data];
    const set = pointSets[data], timeline = timelines[set.timeline];
    if (!Array.isArray(set.values) || !Array.isArray(timeline.x) || !Array.isArray(timeline.labels) || set.values.length !== timeline.x.length || timeline.x.length !== timeline.labels.length) invalid();
    const points = set.values.map((value, offset) => {
      const x = timeline.x[offset], label = timeline.labels[offset];
      if (!Number.isSafeInteger(x) || typeof label !== "string" || typeof value !== "number" || !Number.isFinite(value)) invalid();
      return label ? { x, label, value } : { x, value };
    });
    pointsBySet[data] = points;
    // The materialized array now owns these observations. Release the numeric
    // column while keeping other bindings to this same array lossless.
    set.values = null;
    return points;
  };
  for (const { owner, key, data } of columnTargets) {
    const assign = (value) => Object.defineProperty(owner, key, { value, enumerable: true, configurable: true, writable: true });
    if (!deferData) assign(columnSets[data].values);
    else Object.defineProperty(owner, key, {
      enumerable: true, configurable: true,
      get() {
        const values = columnValues(columnSets[data]);
        assign(values);
        return values;
      },
      set(value) { assign(value); },
    });
  }
  for (const { owner, key, data } of pointTargets) {
    const sources = pointSources.get(owner) || {};
    sources[key] = (timelineOnly) => {
      if (pointsBySet[data] || !Object.getOwnPropertyDescriptor(owner, key)?.get) return null;
      const set = pointSets[data], timeline = timelines[set.timeline];
      if (!validatedTimelines.has(timeline)) {
        if (!Array.isArray(timeline.x) || !Array.isArray(timeline.labels) || timeline.x.length !== timeline.labels.length) invalid();
        for (let i = 0; i < timeline.x.length; i++) if (!Number.isSafeInteger(timeline.x[i]) || typeof timeline.labels[i] !== "string") invalid();
        validatedTimelines.add(timeline);
      }
      if (!timelineOnly && !validatedPointSets.has(set)) {
        if (!Array.isArray(set.values) || set.values.length !== timeline.x.length || set.values.some(value => typeof value !== "number" || !Number.isFinite(value))) invalid();
        validatedPointSets.add(set);
      }
      return { rows: timeline.x, labels: timeline.labels, ...(timelineOnly ? {} : { values: set.values }) };
    };
    pointSources.set(owner, sources);
    const assign = (value) => Object.defineProperty(owner, key, { value, enumerable: true, configurable: true, writable: true });
    Object.defineProperty(owner, key, {
      enumerable: true, configurable: true,
      get() { const points = materialize(data); assign(points); return points; },
      set(value) { assign(value); },
    });
  }
  return result;
}
