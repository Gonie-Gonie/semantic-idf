// Only raw bounded resource bytes cross into this worker. Canonical result
// getters must never be structured-cloned: doing so expands every point set.
function pack(value, buffers) {
  if (Array.isArray(value)) {
    if (value.length >= 256 && value.every(item => typeof item === "number" && Number.isFinite(item))) {
      const values = Float64Array.from(value);
      buffers.push(values.buffer);
      return values;
    }
    for (let index = 0; index < value.length; index++) value[index] = pack(value[index], buffers);
  } else if (value && typeof value === "object") {
    for (const key of Object.keys(value)) value[key] = pack(value[key], buffers);
  }
  return value;
}
self.onmessage = ({ data }) => {
  const { id, buffer } = data;
  try {
    const text = new TextDecoder("utf-8", { fatal: true }).decode(buffer);
    if (!text.length) throw new Error("Empty result resource");
    const buffers = [];
    const value = pack(JSON.parse(text), buffers);
    self.postMessage({ id, value }, buffers);
  } catch (error) {
    self.postMessage({ id, error: String(error.message || error) });
  }
};
