export const simulationResultTransferMediaType = "application/vnd.semantic-idf.simulation-transfer+json";

const transferSchema = "semantic-idf.simulation-transfer/v1";
const unsafePathKeys = new Set(["__proto__", "prototype", "constructor"]);
const object = (value) => value !== null && typeof value === "object";
const index = (value, length) => Number.isInteger(value) && value >= 0 && value < length;

// Keep the canonical result shape. Repeated series share their original point
// arrays, and untouched loops keep their compact numeric columns until opened.
export function decodeSimulationResultTransfer(payload) {
  if (!object(payload) || !String(payload.schema || "").startsWith("semantic-idf.simulation-transfer/")) return payload;
  if (payload.schema !== transferSchema) throw new Error("Unsupported simulation result transfer format");
  const { result, timelines, pointSets, pointBindings } = payload;
  const invalid = () => { throw new Error("Invalid simulation result transfer data"); };
  if (!object(result) || Array.isArray(result) || !Array.isArray(timelines) || !Array.isArray(pointSets) || !Array.isArray(pointBindings)) invalid();
  for (const timeline of timelines) {
    if (!object(timeline) || !Array.isArray(timeline.x) || !Array.isArray(timeline.labels) || timeline.x.length !== timeline.labels.length) invalid();
  }
  for (const set of pointSets) {
    if (!object(set) || !index(set.timeline, timelines.length) || !Array.isArray(set.values) || set.values.length !== timelines[set.timeline].x.length) invalid();
  }
  const targets = [], bound = new WeakMap();
  for (const binding of pointBindings) {
    if (!object(binding) || !Array.isArray(binding.path) || !binding.path.length || !index(binding.data, pointSets.length)) invalid();
    let owner = result;
    for (let offset = 0; offset < binding.path.length; offset++) {
      const key = binding.path[offset], last = offset === binding.path.length - 1;
      if (!object(owner) || (Array.isArray(owner) ? !index(key, owner.length) : typeof key !== "string" || !key || unsafePathKeys.has(key))) invalid();
      if (last) {
        // Go's omitempty can omit a compacted points/displayPoints member.
        if (Object.hasOwn(owner, key) && owner[key] !== null) invalid();
        const keys = bound.get(owner) || new Set();
        if (keys.has(key)) invalid();
        keys.add(key); bound.set(owner, keys);
        targets.push({ owner, key, data: binding.data });
      } else {
        if (!Object.hasOwn(owner, key)) invalid();
        owner = owner[key];
      }
    }
  }
  const pointsBySet = new Array(pointSets.length);
  const materialize = (data) => {
    if (pointsBySet[data]) return pointsBySet[data];
    const set = pointSets[data], timeline = timelines[set.timeline];
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
  for (const { owner, key, data } of targets) {
    const assign = (value) => Object.defineProperty(owner, key, { value, enumerable: true, configurable: true, writable: true });
    Object.defineProperty(owner, key, {
      enumerable: true, configurable: true,
      get() { const points = materialize(data); assign(points); return points; },
      set(value) { assign(value); },
    });
  }
  return result;
}
