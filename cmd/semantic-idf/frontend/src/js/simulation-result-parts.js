import { decodeSimulationResultTransfer, createSimulationColumnValidation } from "./simulation-result-transport.js";
import { createResultResourceScheduler } from "./result-resource-scheduler.js";
import { t } from "./i18n.js";
import { energyPathSeriesID } from "./energy-path-navigation.js";

export const simulationResultPartsMediaType = "application/vnd.semantic-idf.simulation-parts+json";
const schema = "semantic-idf.simulation-parts/v1";
const stores = new WeakMap();
const forbidden = new Set(["__proto__", "prototype", "constructor"]);
const keyFor = path => JSON.stringify(path);
const prefixOf = (prefix, path) => prefix.length <= path.length && prefix.every((key, index) => key === path[index]);
const yieldTask = () => new Promise(resolve => setTimeout(resolve, 0));

class PendingResultResource extends Error {
  constructor(ready) { super("Result resource pending"); this.ready = ready; }
}
export const isPendingSimulationResource = error => error instanceof PendingResultResource;
export const simulationResultStore = result => result && stores.get(result);

function transferError(error) {
  if (error?.name === "AbortError") return error;
  const message = t("simulation.resultTransfer.invalidJSON", {}, "The simulation result could not be transferred. Open the saved run to inspect its results.");
  const failure = new Error(message, { cause: error });
  failure.code = "simulation_result_transfer_invalidJSON";
  return failure;
}

function createPartParser() {
  let worker = null, sequence = 0, disposed = false;
  const pending = new Map();
  try { worker = new Worker(new URL("./simulation-result-worker.js", import.meta.url), { type: "module" }); }
  catch { /* A bounded main-thread fallback also supports older desktop hosts. */ }
  if (worker) {
    worker.onmessage = ({ data }) => {
      const request = pending.get(data.id);
      if (!request) return;
      pending.delete(data.id);
      if (data.error) request.reject(transferError(new Error(data.error)));
      else request.resolve(data.value);
    };
    worker.onerror = event => {
      const error = transferError(new Error(event.message || "Result parser failed"));
      for (const request of pending.values()) request.reject(error);
      pending.clear(); worker?.terminate(); worker = null;
    };
  }
  return {
    async parse(buffer) {
      if (disposed) throw new DOMException("Result released", "AbortError");
      if (!worker) {
        await yieldTask();
        try { return JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(buffer)); }
        catch (error) { throw transferError(error); }
      }
      const id = ++sequence;
      return new Promise((resolve, reject) => {
        pending.set(id, { resolve, reject });
        worker.postMessage({ id, buffer }, [buffer]);
      });
    },
    dispose() {
      disposed = true; worker?.terminate(); worker = null;
      for (const request of pending.values()) request.reject(new DOMException("Result released", "AbortError"));
      pending.clear();
    },
  };
}

function unpack(value) {
  // Existing consumers deliberately receive ordinary arrays. The bounded worker
  // packet transfers Float64 storage without cloning a giant object graph;
  // this conversion retains binary64 precision, negative zero and array APIs.
  if (value instanceof Float64Array) return Array.from(value);
  if (Array.isArray(value)) { for (let index = 0; index < value.length; index++) value[index] = unpack(value[index]); }
  else if (value && typeof value === "object") for (const key of Object.keys(value)) value[key] = unpack(value[key]);
  return value;
}

export async function loadSimulationResultParts(manifest, options = {}) {
  if (manifest?.schema !== schema || !manifest.catalogParts?.length) return openSimulationResultParts(manifest, options);
  const invalidCatalog = () => { throw transferError(new Error("Invalid result catalog manifest")); };
  if (!Array.isArray(manifest.catalogParts) || !manifest.runId || !manifest.textHash || !manifest.snapshot) invalidCatalog();
  const catalogIDs = new Set(), catalogLeaves = new Map();
  for (const part of manifest.catalogParts) {
    if (!part || typeof part.id !== "string" || !part.id || catalogIDs.has(part.id) || !Number.isSafeInteger(part.byteLength) || part.byteLength < 1 || part.byteLength > 4 * 1024 * 1024 || !Array.isArray(part.bindings)) invalidCatalog();
    catalogIDs.add(part.id);
    for (const binding of part.bindings) {
      const path = binding.path;
      if (!Array.isArray(path) || !path.length || path.some(key => typeof key === "string" ? forbidden.has(key) || !key : !Number.isSafeInteger(key) || key < 0)) invalidCatalog();
      const key = keyFor(path), previous = catalogLeaves.get(key);
      if (previous && (binding.length === undefined || previous.length !== binding.length)) invalidCatalog();
      if (binding.length !== undefined && (!Number.isSafeInteger(binding.length) || binding.length < 0 || binding.length > 0xffffffff || !Number.isSafeInteger(binding.count) || binding.count < 0 || !Number.isSafeInteger(binding.offset ?? 0) || (binding.offset ?? 0) < 0 || (binding.offset ?? 0) + binding.count > binding.length)) invalidCatalog();
      const leaf = previous || { path, length: binding.length, chunks: [] };
      leaf.chunks.push(binding); catalogLeaves.set(key, leaf);
    }
  }
  for (const leaf of catalogLeaves.values()) {
    for (let length = 1; length < leaf.path.length; length++) if (catalogLeaves.has(keyFor(leaf.path.slice(0, length)))) invalidCatalog();
    if (leaf.length !== undefined) {
      let end = 0;
      for (const binding of leaf.chunks.sort((a, b) => (a.offset || 0) - (b.offset || 0))) {
        if ((binding.offset || 0) !== end) invalidCatalog();
        end += binding.count;
      }
      if (end !== leaf.length) invalidCatalog();
    }
  }
  const parser = createPartParser(), abort = new AbortController();
  const fetcher = options.fetcher || ((...args) => fetch(...args));
  const scheduler = createResultResourceScheduler(async part => {
    const query = new URLSearchParams({ runId: manifest.runId, textHash: manifest.textHash, snapshot: manifest.snapshot, partId: part.id });
    const response = await fetcher(`/api/simulation-result-part?${query}`, { signal: abort.signal });
    if (!response.ok) throw transferError(new Error(`Result catalog HTTP ${response.status}`));
    const buffer = await response.arrayBuffer();
    if (buffer.byteLength !== part.byteLength) throw transferError(new Error("Incomplete result catalog"));
    const values = await parser.parse(buffer);
    if (!Array.isArray(values) || values.length !== part.bindings.length) throw transferError(new Error("Invalid result catalog"));
    await yieldTask();
    part.bindings.forEach((binding, index) => {
      const path = binding.path;
      if (!Array.isArray(path) || !path.length || path.some(key => typeof key === "string" ? forbidden.has(key) || !key : !Number.isSafeInteger(key) || key < 0)) throw transferError(new Error("Invalid result catalog path"));
      let owner = manifest;
      for (const key of path.slice(0, -1)) {
        if (!owner || typeof owner !== "object" || !Object.hasOwn(owner, key)) throw transferError(new Error("Invalid result catalog parent"));
        owner = owner[key];
      }
      const key = path.at(-1), value = unpack(values[index]);
      if (!owner || typeof owner !== "object") throw transferError(new Error("Invalid result catalog target"));
      if (binding.length === undefined) owner[key] = value;
      else {
        if (!Number.isSafeInteger(binding.length) || binding.length < 0 || !Number.isSafeInteger(binding.offset ?? 0) || (binding.offset ?? 0) < 0 || !Array.isArray(value) || value.length !== binding.count || (binding.offset || 0) + binding.count > binding.length) throw transferError(new Error("Invalid result catalog chunk"));
        const target = owner[key];
        if (!Array.isArray(target)) throw transferError(new Error("Invalid result catalog array"));
        target.length = binding.length;
        for (let offset = 0; offset < value.length; offset++) target[(binding.offset || 0) + offset] = value[offset];
      }
    });
  });
  try {
    // The admission window remains bounded even if a future catalog has many
    // pages. Metadata must be complete before installing deferred data getters.
    for (let index = 0; index < manifest.catalogParts.length; index += 2) await Promise.all(manifest.catalogParts.slice(index, index + 2).map(part => scheduler.request(part)));
    if (manifest.parts.some(part => catalogIDs.has(part.id))) invalidCatalog();
    return openSimulationResultParts(manifest, options);
  } catch (error) { throw transferError(error);
  } finally { abort.abort(); scheduler.dispose(); parser.dispose(); }
}

export function openSimulationResultParts(manifest, { fetcher = (...args) => fetch(...args), backgroundBytes = 384 * 1024 * 1024 } = {}) {
  if (manifest?.schema !== schema) return decodeSimulationResultTransfer(manifest);
  const invalid = () => { throw transferError(new Error("Invalid simulation resource manifest")); };
  const root = manifest.root;
  if (!root || !root.result || !Array.isArray(manifest.parts) || !manifest.runId || !manifest.textHash || !manifest.snapshot || root.result.runId !== manifest.runId) invalid();
  const parts = new Map(), leaves = new Map();
  let disposed = false, revision = 0, backgroundReserved = 0, readPriority = 0;
  const abort = new AbortController();
  const parentFor = path => {
    let owner = root;
    for (let offset = 0; offset < path.length - 1; offset++) {
      const key = path[offset];
      if (!owner || typeof owner !== "object" || !Object.hasOwn(owner, key)) invalid();
      owner = owner[key];
    }
    if (!owner || typeof owner !== "object") invalid();
    return owner;
  };
  for (const part of manifest.parts) {
    if (!part || typeof part.id !== "string" || !part.id || parts.has(part.id) || !Number.isSafeInteger(part.byteLength) || part.byteLength < 1 || part.byteLength > 4 * 1024 * 1024 || !Array.isArray(part.bindings)) invalid();
    parts.set(part.id, part);
    for (const binding of part.bindings) {
      if (!Array.isArray(binding.path) || !binding.path.length || binding.path.some(key => typeof key === "string" ? !key || forbidden.has(key) : !Number.isSafeInteger(key) || key < 0)) invalid();
      const id = keyFor(binding.path);
      let leaf = leaves.get(id);
      if (!leaf) {
        leaf = { path: binding.path, chunks: [], value: undefined, ready: false, remaining: 0 };
        leaves.set(id, leaf);
      }
      const chunked = binding.length !== undefined;
      if (leaf.chunks.length && (!chunked || leaf.length !== binding.length)) invalid();
      if (chunked && (!Number.isSafeInteger(binding.length) || binding.length < 0 || binding.length > 0xffffffff || !Number.isSafeInteger(binding.count) || binding.count < 0 || !Number.isSafeInteger(binding.offset ?? 0) || (binding.offset ?? 0) < 0 || (binding.offset ?? 0) + binding.count > binding.length)) invalid();
      leaf.length = binding.length;
      leaf.chunks.push({ part, binding }); leaf.remaining++;
    }
  }
  for (const leaf of leaves.values()) {
    for (let length = 1; length < leaf.path.length; length++) if (leaves.has(keyFor(leaf.path.slice(0, length)))) invalid();
    if (leaf.length !== undefined) {
      let end = 0;
      for (const { binding } of [...leaf.chunks].sort((a, b) => (a.binding.offset || 0) - (b.binding.offset || 0))) {
        if ((binding.offset || 0) !== end) invalid();
        end += binding.count;
      }
      if (end !== leaf.length) invalid();
    }
    // All descriptors are leaves; their parent catalogs remain in the root.
    leaf.owner = parentFor(leaf.path); leaf.key = leaf.path.at(-1);
  }
  const parser = createPartParser();
  const scheduler = createResultResourceScheduler(async part => {
    const query = new URLSearchParams({ runId: manifest.runId, textHash: manifest.textHash, snapshot: manifest.snapshot, partId: part.id });
    try {
      const response = await fetcher(`/api/simulation-result-part?${query}`, { signal: abort.signal });
      if (!response.ok) throw new Error(`Result resource HTTP ${response.status}`);
      const buffer = await response.arrayBuffer();
      if (buffer.byteLength !== part.byteLength) throw new Error("Incomplete result resource");
      const values = await parser.parse(buffer);
      if (!Array.isArray(values) || values.length !== part.bindings.length) invalid();
      // Yield between installation jobs, including when fetch resolves from a
      // local native cache. No worker response installs an expanded result.
      await yieldTask();
      if (disposed) throw new DOMException("Result released", "AbortError");
      const prepared = part.bindings.map((binding, index) => {
        const value = unpack(values[index]);
        if (binding.length !== undefined && (!Array.isArray(value) || value.length !== binding.count)) invalid();
        return { binding, value };
      });
      for (const { binding, value } of prepared) {
        const leaf = leaves.get(keyFor(binding.path));
        if (binding.length === undefined) leaf.value = value;
        else {
          leaf.value ||= new Array(binding.length);
          for (let index = 0; index < value.length; index++) leaf.value[(binding.offset || 0) + index] = value[index];
        }
        if (binding.path.length === 3 && binding.path[0] === "columnSets" && binding.path[2] === "values") {
          leaf.validateColumn ||= createSimulationColumnValidation(root.columnSets[binding.path[1]], binding.length ?? value.length);
          leaf.validateColumn(value, binding.offset || 0, leaf.value);
        }
        leaf.remaining--; leaf.ready = leaf.remaining === 0;
      }
      revision++;
    } catch (error) { throw transferError(error); }
  });
  const request = (part, priority = 0) => scheduler.request(part, priority);
  for (const leaf of leaves.values()) Object.defineProperty(leaf.owner, leaf.key, {
    enumerable: true, configurable: true,
    get() {
      if (!leaf.ready) throw new PendingResultResource(readPriority === null ? null : Promise.all(leaf.chunks.map(({ part }) => request(part, readPriority))));
      return leaf.value;
    },
    set(value) { leaf.value = value; leaf.ready = true; },
  });
  let result;
  try { result = decodeSimulationResultTransfer(root, { deferData: true }); }
  catch (error) { abort.abort(); parser.dispose(); scheduler.dispose(); throw error; }

  const wireResources = new Map();
  for (const leaf of leaves.values()) for (let length = 1; length <= leaf.path.length; length++) {
    const key = keyFor(leaf.path.slice(0, length));
    const ids = wireResources.get(key) || new Set();
    for (const { part } of leaf.chunks) ids.add(part.id);
    wireResources.set(key, ids);
  }

  function resourcesFor(match) {
    const ids = new Set();
    const addPath = path => { for (const id of wireResources.get(keyFor(path)) || []) ids.add(id); };
    for (const leaf of leaves.values()) if (leaf.path[0] === "result" && match(leaf.path.slice(1))) for (const { part } of leaf.chunks) ids.add(part.id);
    for (const binding of root.pointBindings || []) if (match(binding.path)) {
      addPath(["pointSets", binding.data]); addPath(["timelines", root.pointSets[binding.data].timeline]);
    }
    for (const binding of root.columnBindings || []) if (match(binding.path)) addPath(["columnSets", binding.data]);
    return [...ids].map(id => parts.get(id));
  }
  function contextMatcher(view, context = {}, nearby = false) {
    const selected = new Set(context.seriesIDs || []);
    const series = result.series || [];
    const indices = new Set(series.flatMap((item, index) => selected.has(item.sourceId) || selected.has(energyPathSeriesID(item)) || selected.has(`${item.file || ""}::${item.column || ""}`) ? [index] : []));
    const comparisonSeries = path => path[0] === "series" && indices.has(path[1]);
    if (view === "energy") {
      const explanation = result.purposeResults?.energyExplanation || {};
      const zone = (explanation.zoneResults || []).findIndex(item => String(item.scope?.zoneName || "").toLowerCase() === String(context.simulationEnergyZoneName || "").toLowerCase());
      const sourceIDs = new Set(context.energySourceIDs || []), sources = explanation.sources || [];
      const sourceIndex = new Map(sources.map((source, index) => [source.id, index]));
      const visit = id => {
        for (const child of sources[sourceIndex.get(id)]?.inputSourceIds || []) if (!sourceIDs.has(child)) { sourceIDs.add(child); visit(child); }
      };
      for (const id of [...sourceIDs]) visit(id);
      return path => {
        if (comparisonSeries(path)) return true;
        if (path[0] !== "purposeResults") return false;
        if (["energy", "energyExplanationSummary"].includes(path[1])) return true;
        if (path[1] !== "energyExplanation") return false;
        if (path.some(key => typeof key === "string" && /hourly/i.test(key))) {
          if (path[2] === "sources" && selected.has(sources[path[3]]?.id)) return true;
          if (!context.simulationEnergySelection || !nearby && context.simulationEnergyChartFrequency !== "hourly") return false;
          return path[2] !== "sources" || sourceIDs.has(sources[path[3]]?.id);
        }
        let scope = explanation, offset = 2;
        if (path[2] === "zoneResults") {
          if (context.simulationEnergyScopeKind !== "zone" || path[3] !== zone) return false;
          scope = explanation.zoneResults[zone]; offset = 4;
        }
        if (path[offset] === "periods") {
          return nearby || Boolean(context.simulationEnergySelection) || scope.periods?.[path[offset + 1]]?.id?.toLowerCase() === String(context.simulationEnergyPeriod || "annual").toLowerCase();
        }
        return true;
      };
    }
    if (view === "zone_heat_flow") return path => {
      if (comparisonSeries(path)) return true;
      if (path[0] === "heatFlow" || prefixOf(["purposeResults", "zoneHeatFlow"], path)) return true;
      if (!prefixOf(["purposeResults", "thermalTopology"], path)) return false;
      if (path[2] === "planGeometry") return true;
      return path[2] === "periods" && result.purposeResults.thermalTopology.periods?.[path[3]]?.id === "hourly" && ["labels", "boundaryFlows"].includes(path[4]);
    };
    if (view === "comfort") return path => comparisonSeries(path) || prefixOf(["purposeResults", "comfort"], path);
    if (view === "hvac_loops") {
      const loops = result.purposeResults?.hvacLoops || [];
      const selected = context.simulationHVACInspection?.selectedLoop;
      const normalize = value => String(value || "").trim().toLowerCase().replace(/\s+/g, " ");
      const index = Math.max(0, loops.findIndex(loop => `${normalize(loop.loopType || loop.topology?.type)}|${normalize(loop.name)}` === selected));
      return path => prefixOf(["purposeResults", "hvacLoops", index], path);
    }
    return comparisonSeries;
  }
  const contextResources = new Map();
  function viewResources(view, context = {}, nearby = false) {
    const key = JSON.stringify([view, nearby, context.simulationEnergyScopeKind, context.simulationEnergyZoneName,
      context.simulationEnergyPeriod, context.simulationEnergySelection, context.simulationEnergyChartFrequency,
      context.energySourceIDs, context.simulationHVACInspection?.selectedLoop, context.simulationComfortZone,
      context.simulationHeatFlowSelectedZone, context.seriesIDs]);
    if (contextResources.has(key)) return contextResources.get(key);
    const resources = resourcesFor(contextMatcher(view, context, nearby));
    contextResources.set(key, resources);
    if (contextResources.size > 32) contextResources.delete(contextResources.keys().next().value);
    return resources;
  }
  const store = {
    result, manifest,
    get revision() { return revision; },
    get disposed() { return disposed; },
    probe(callback) {
      const previousPriority = readPriority;
      try { readPriority = null; return { ready: true, value: callback() }; }
      catch (error) { if (!isPendingSimulationResource(error)) throw error; return { ready: false }; }
      finally { readPriority = previousPriority; }
    },
    ready(view, context) { return viewResources(view, context).every(part => scheduler.status(part.id) === "ready"); },
    ensure(view, context, priority = 0) { return Promise.all(viewResources(view, context).map(part => request(part, priority))); },
    async read(callback, { priority = 0 } = {}) {
      while (!disposed) {
        let pending;
        const previousPriority = readPriority;
        try { readPriority = priority; return callback(); }
        catch (error) { if (!isPendingSimulationResource(error)) throw error; pending = error.ready; }
        finally { readPriority = previousPriority; }
        await pending;
      }
      throw new DOMException("Result released", "AbortError");
    },
    async ensureAll() {
      // Sequential admission keeps an export from queuing thousands of jobs.
      for (const part of parts.values()) await request(part, 0);
    },
    prefetch(view, context) {
      if (disposed) return;
      const admit = (part, priority) => {
        const status = scheduler.status(part.id);
        if (status === "ready" || status === "failed") return;
        if (status === "idle") {
          if (backgroundReserved + part.byteLength > backgroundBytes) return;
          backgroundReserved += part.byteLength;
        }
        // A changed selection promotes existing work without another request
        // or another charge against the speculative admission budget.
        request(part, priority).catch(() => {});
      };
      for (const part of viewResources(view, context, true)) admit(part, 1);
      // All next tabs have the same priority. Round-robin admission in UI order
      // prevents a large tab from occupying the entire next-view queue.
      const nextViews = ["energy", "zone_heat_flow", "hvac_loops", "comfort"]
        .filter(item => item !== view).map(item => viewResources(item, context));
      const longest = Math.max(0, ...nextViews.map(resources => resources.length));
      for (let index = 0; index < longest; index++) {
        for (const resources of nextViews) if (resources[index]) admit(resources[index], 2);
      }
      for (const part of parts.values()) admit(part, 3);
    },
    dispose() { if (disposed) return; disposed = true; abort.abort(); scheduler.dispose(); parser.dispose(); leaves.clear(); parts.clear(); wireResources.clear(); contextResources.clear(); },
  };
  stores.set(result, store);
  return result;
}
