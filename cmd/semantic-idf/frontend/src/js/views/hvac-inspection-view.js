import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";
import { prepareHVACInspection, hvacInspectionSnapshots, hvacInspectionTrace, hvacInspectionBasicProperties } from "../hvac-inspection-data.js";
import { renderHVACInspectionChart, updateHVACInspectionChartFrame, hvacInspectionChartYRanges } from "../hvac-inspection-charts.js";
import { renderHVACInspectionTopology } from "./hvac-inspection-topology.js";

const copy = (key, fallback) => t(`simulation.hvacInspect${key}`, {}, fallback);
const option = (value, label, selected, disabled = false) => `<option value="${escapeHTML(value)}"${selected ? " selected" : ""}${disabled ? " disabled" : ""}>${escapeHTML(label)}</option>`;
const basicChartCache = new WeakMap();
const pendingBasicPlots = new WeakMap();
const finite = (value) => typeof value === "number" && Number.isFinite(value);
const basicTitle = (kind) => kind === "flow" ? copy("Flow", "Mass flow") : kind === "temperature" ? copy("Temperature", "Temperature") : copy("HumidityTitle", "Humidity");
const rangeNumber = (value) => value.toLocaleString(undefined, { maximumFractionDigits: 4 });

function normalize(model, ui) {
  ui.frameIndex = Math.max(0, Math.min(model.frames.length - 1, Math.round(Number(ui.frameIndex) || 0)));
  ui.visibleNodes ||= {};
  ui.basicYLimits ||= {};
  ui.mode = ui.mode === "scatter" ? "scatter" : "line";
  ui.zoom ||= "fit";
  if (!model.entities.some((entity) => entity.id === ui.selectedComponent)) ui.selectedComponent = "";
  if (!model.entities.some((entity) => entity.id === ui.selectedNode)) ui.selectedNode = ui.selectedComponent ? "" : model.entities.find((entity) => entity.kind === "node")?.id || "";
  if (!Array.isArray(ui.rows)) {
    const properties = model.entities.flatMap((entity) => entity.properties);
    const first = properties.find((property) => property.kind === "power") || properties.find((property) => property.kind === "flow") || properties[0];
    const second = properties.find((property) => property.kind === "temperature" && property !== first) || properties.find((property) => property !== first);
    ui.rows = [first, second].map((property) => ({ entity: property?.entity.id || "", property: property?.id || "" }));
  }
  ui.rows = ui.rows.slice(0, ui.mode === "scatter" ? 2 : 8).map((row) => {
    const entity = model.entities.find((item) => item.id === row.entity);
    return { entity: entity?.id || "", property: entity?.properties.some((property) => property.id === row.property) ? row.property : "" };
  });
  while (ui.rows.length < 2) ui.rows.push({ entity: "", property: "" });
  return ui;
}

function selectedProperties(model, ui) {
  return ui.rows.map((row) => model.entities.find((entity) => entity.id === row.entity)?.properties.find((property) => property.id === row.property)).filter(Boolean);
}

function renderSnapshot(model, ui) {
  const snapshots = hvacInspectionSnapshots(model, ui.frameIndex, ui);
  if (model.loop.topology) return renderHVACInspectionTopology({ loop: model.loop.topology, ...snapshots,
    selectedNode: ui.selectedNode, selectedComponent: ui.selectedComponent, zoom: ui.zoom });
  // Older results can retain observations without the executed model graph.
  // Present the chosen point's values without inventing connections.
  const entity = [...snapshots.nodes, ...snapshots.components].find((item) => item.id === (ui.selectedComponent || ui.selectedNode));
  return `${renderHVACInspectionTopology({ loop: null, ...snapshots })}${entity ? `<article class="hvac-inspection-legacy-frame"><strong>${escapeHTML(entity.name)}</strong><div>${entity.metrics.map((metric) => `<span>${escapeHTML(metric.label)} <b>${escapeHTML(typeof metric.value === "number" && Number.isFinite(metric.value) ? metric.value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + " " + metric.unit : "—")}</b></span>`).join("")}</div></article>` : ""}`;
}

function renderBasicCharts(model, ui) {
  return ["flow", "temperature", "humidity"].map((kind) => `<div data-hvac-inspect-basic-chart="${kind}"><div data-hvac-inspect-basic-plot="${kind}">${renderBasicPlot(model, ui, kind)}</div><div data-hvac-inspect-y-controls="${kind}">${renderBasicYControls(model, ui, kind)}</div></div>`).join("");
}

function basicChartData(model, kind) {
  if (!basicChartCache.has(model)) basicChartCache.set(model, {});
  const cached = basicChartCache.get(model);
  if (!cached[kind]) {
    const nodes = model.entities.filter((entity) => entity.kind === "node");
    const series = hvacInspectionBasicProperties(model, kind).map((property) => ({ ...hvacInspectionTrace(model, property),
      axisLabel: property.label, legendLabel: property.entity.name, entityId: property.entity.id, colorIndex: nodes.indexOf(property.entity) }));
    for (const trace of series) trace.hasObservations = trace.points.some((point) => finite(point.value));
    cached[kind] = { series, ranges: hvacInspectionChartYRanges(series) };
  }
  return cached[kind];
}

function basicYRange(ui, kind, range) {
  const saved = ui.basicYLimits[kind]?.[range.unit];
  if (!finite(saved?.low) || !finite(saved?.high) || saved.low >= saved.high) return { ...range.auto };
  const low = Math.max(range.min, Math.min(range.max - range.step, saved.low));
  const high = Math.min(range.max, Math.max(low + range.step, saved.high));
  return { low, high };
}

function renderBasicPlot(model, ui, kind) {
  const { series, ranges } = basicChartData(model, kind);
  return renderHVACInspectionChart({ title: basicTitle(kind), frameKey: model.frames[ui.frameIndex]?.x, legendLayout: "vertical",
    yLimits: Object.fromEntries(ranges.map((range) => [range.unit, basicYRange(ui, kind, range)])),
    series: series.filter((trace) => ui.visibleNodes[trace.entityId] !== false) });
}

function renderBasicYControls(model, ui, kind) {
  const { series, ranges } = basicChartData(model, kind);
  if (!ranges.length) return "";
  const manual = Boolean(Object.keys(ui.basicYLimits[kind] || {}).length);
  return `<div class="hvac-inspection-y-controls"><div class="hvac-inspection-y-heading"><span>${escapeHTML(copy("YLimits", "Y-axis limits"))}</span><button type="button" data-hvac-inspect-y-reset="${kind}"${manual ? "" : " disabled"}>${escapeHTML(copy("Auto", "Auto"))}</button></div>${ranges.map((range) => {
    const { low, high } = basicYRange(ui, kind, range), span = range.max - range.min;
    const disabled = !series.some((trace) => trace.unit === range.unit && trace.hasObservations && ui.visibleNodes[trace.entityId] !== false);
    return `<div class="hvac-inspection-y-range" data-hvac-inspect-y-range="${kind}" data-hvac-inspect-y-unit="${escapeHTML(range.unit)}" role="group" aria-label="${escapeHTML(`${basicTitle(kind)} · ${copy("YLimits", "Y-axis limits")} (${range.unit})`)}">
      <div class="hvac-inspection-y-values"><output data-hvac-inspect-y-value="low">${escapeHTML(`${copy("YMin", "Min")} ${rangeNumber(low)} ${range.unit}`)}</output><output data-hvac-inspect-y-value="high">${escapeHTML(`${copy("YMax", "Max")} ${rangeNumber(high)} ${range.unit}`)}</output></div>
      <div class="hvac-inspection-y-slider"><div class="hvac-inspection-y-track"><i style="left:${(low - range.min) / span * 100}%;width:${(high - low) / span * 100}%"></i></div>
        ${["low", "high"].map((bound) => `<input type="range" min="${range.min}" max="${range.max}" step="${range.step}" value="${bound === "low" ? low : high}" data-hvac-inspect-y-bound="${bound}" aria-label="${escapeHTML(`${basicTitle(kind)} · ${copy(bound === "low" ? "YMin" : "YMax", bound === "low" ? "Min" : "Max")} (${range.unit})`)}" aria-valuemin="${bound === "low" ? range.min : low + range.step}" aria-valuemax="${bound === "low" ? high - range.step : range.max}" aria-valuetext="${escapeHTML(`${rangeNumber(bound === "low" ? low : high)} ${range.unit}`)}"${disabled ? " disabled" : ""}/>`).join("")}
      </div></div>`;
  }).join("")}</div>`;
}

function refreshBasicPlot(container, model, ui, kind, immediate = false) {
  if (!pendingBasicPlots.has(container)) pendingBasicPlots.set(container, new Map());
  const pending = pendingBasicPlots.get(container);
  if (pending.has(kind)) cancelAnimationFrame(pending.get(kind));
  const mount = container.querySelector(`[data-hvac-inspect-basic-plot="${kind}"]`);
  const draw = () => {
    pending.delete(kind);
    // A loop or visibility change may replace the target before the next frame.
    if (mount && container.contains(mount)) mount.innerHTML = renderBasicPlot(model, ui, kind);
  };
  if (immediate) draw();
  else pending.set(kind, requestAnimationFrame(draw));
}

function updateBasicYControl(group, range, selected) {
  for (const bound of ["low", "high"]) {
    const input = group.querySelector(`[data-hvac-inspect-y-bound="${bound}"]`), value = selected[bound];
    input.value = value;
    input.setAttribute("aria-valuemin", bound === "low" ? range.min : selected.low + range.step);
    input.setAttribute("aria-valuemax", bound === "low" ? selected.high - range.step : range.max);
    input.setAttribute("aria-valuetext", `${rangeNumber(value)} ${range.unit}`);
    group.querySelector(`[data-hvac-inspect-y-value="${bound}"]`).textContent = `${copy(bound === "low" ? "YMin" : "YMax", bound === "low" ? "Min" : "Max")} ${rangeNumber(value)} ${range.unit}`;
  }
  const track = group.querySelector(".hvac-inspection-y-track i");
  track.style.left = `${(selected.low - range.min) / (range.max - range.min) * 100}%`;
  track.style.width = `${(selected.high - selected.low) / (range.max - range.min) * 100}%`;
}

function renderCustomChart(model, ui) {
  return renderHVACInspectionChart({ mode: ui.mode, frameKey: model.frames[ui.frameIndex]?.x,
    series: selectedProperties(model, ui).map((property) => ({ ...hvacInspectionTrace(model, property), axisLabel: property.label })) });
}

function renderCustomControls(model, ui) {
  const rows = ui.rows.map((row, index) => {
    const entity = model.entities.find((item) => item.id === row.entity);
    const otherProperties = selectedProperties(model, { ...ui, rows: ui.rows.filter((_, other) => other !== index) });
    const otherUnits = new Set(otherProperties.map((property) => property.unit));
    return `<div class="hvac-inspection-custom-row">
      <span>${ui.mode === "scatter" ? index === 0 ? "X" : "Y" : index + 1}</span>
      <label><span>${escapeHTML(copy("Entity", "Equipment / node"))}</span><select data-hvac-inspect-entity="${index}">
        ${option("", copy("ChooseEntity", "Choose equipment / node"), !entity)}
        ${["component", "node"].map((kind) => `<optgroup label="${escapeHTML(copy(kind === "node" ? "Nodes" : "Equipment", kind === "node" ? "Nodes" : "Equipment"))}">${model.entities.filter((item) => item.kind === kind).map((item) => option(item.id, item.name, item.id === row.entity)).join("")}</optgroup>`).join("")}
      </select></label>
      <label><span>${escapeHTML(copy("Property", "Property"))}</span><select data-hvac-inspect-property="${index}"${entity ? "" : " disabled"}>
        ${option("", copy("ChooseProperty", "Choose a property"), !row.property)}
        ${(entity?.properties || []).map((property) => option(property.id, `${property.label} (${property.unit})`, property.id === row.property,
          otherProperties.some((other) => other.id === property.id) || ui.mode === "line" && otherUnits.size >= 2 && !otherUnits.has(property.unit))).join("")}
      </select></label>
      ${ui.mode === "line" && ui.rows.length > 2 ? `<button type="button" data-hvac-inspect-remove="${index}" aria-label="${escapeHTML(copy("Remove", "Remove series"))}">×</button>` : ""}
    </div>`;
  }).join("");
  return `<div class="hvac-inspection-custom-heading"><h4>${escapeHTML(copy("Custom", "Custom graph"))}</h4><label><span>${escapeHTML(copy("GraphType", "Graph type"))}</span><select data-hvac-inspect-mode>${option("line", "Line", ui.mode === "line")}${option("scatter", "Scatter", ui.mode === "scatter")}</select></label></div>
    <div class="hvac-inspection-custom-rows">${rows}</div>
    ${ui.mode === "line" ? `<button type="button" data-hvac-inspect-add${ui.rows.length >= 8 ? " disabled" : ""}>${escapeHTML(copy("Add", "Add series"))}</button>` : ""}`;
}

export function renderHVACInspection(loop, ui = {}) {
  const model = prepareHVACInspection(loop);
  normalize(model, ui);
  return `<div class="hvac-inspection" data-hvac-inspection>
    <div class="hvac-inspection-frame-control"><label for="hvac-inspection-frame">${escapeHTML(copy("Frame", "Frame"))}</label>
      <input id="hvac-inspection-frame" data-simulation-hvac-frame type="range" min="0" max="${Math.max(0, model.frames.length - 1)}" value="${ui.frameIndex}"${model.frames.length ? "" : " disabled"}/>
      <output data-hvac-inspect-frame-label for="hvac-inspection-frame">${escapeHTML(model.frames[ui.frameIndex]?.label || copy("NoFrame", "No frame"))}</output>
    </div>
    <div data-hvac-inspect-snapshot>${renderSnapshot(model, ui)}</div>
    <section class="hvac-inspection-basic"><h4>${escapeHTML(copy("NodeGraphs", "Node graphs"))}</h4>
      <div class="hvac-inspection-node-toggles" role="group" aria-label="${escapeHTML(copy("Nodes", "Nodes"))}">
        ${model.entities.filter((entity) => entity.kind === "node").map((entity) => `<label><input type="checkbox" data-hvac-inspect-node-visible="${escapeHTML(entity.id)}"${ui.visibleNodes[entity.id] !== false ? " checked" : ""}/><span>${escapeHTML(entity.name)}</span></label>`).join("")}
      </div><div data-hvac-inspect-basic-charts>${renderBasicCharts(model, ui)}</div>
    </section>
    <section class="hvac-inspection-custom"><div data-hvac-inspect-custom-controls>${renderCustomControls(model, ui)}</div><div data-hvac-inspect-custom-chart>${renderCustomChart(model, ui)}</div></section>
  </div>`;
}

export function handleHVACInspectionEvent(event, container, loop, ui) {
  const target = event.target.closest?.("[data-simulation-hvac-frame], [data-hvac-inspect-node], [data-hvac-inspect-component], [data-hvac-inspect-node-visible], [data-hvac-inspect-entity], [data-hvac-inspect-property], [data-hvac-inspect-mode], [data-hvac-inspect-add], [data-hvac-inspect-remove], [data-hvac-inspect-zoom], [data-hvac-inspect-y-bound], [data-hvac-inspect-y-reset]");
  if (!target || !container.contains(target)) return false;
  const isClick = event.type === "click", clickControl = target.matches("[data-hvac-inspect-node], [data-hvac-inspect-component], [data-hvac-inspect-add], [data-hvac-inspect-remove], [data-hvac-inspect-y-reset]");
  if (isClick !== clickControl || !isClick && event.type !== "input" && event.type !== "change") return false;
  event.stopPropagation();
  if (isClick) event.preventDefault();
  const model = prepareHVACInspection(loop);
  normalize(model, ui);
  const data = target.dataset;
  if (data.hvacInspectYBound !== undefined) {
    const group = target.closest("[data-hvac-inspect-y-range]"), kind = group?.dataset.hvacInspectYRange;
    const range = basicChartData(model, kind).ranges.find((item) => item.unit === group.dataset.hvacInspectYUnit);
    if (!range || target.disabled) return true;
    const selected = basicYRange(ui, kind, range), value = Number(target.value);
    if (!finite(value)) return true;
    if (data.hvacInspectYBound === "low") selected.low = Math.max(range.min, Math.min(selected.high - range.step, value));
    else selected.high = Math.min(range.max, Math.max(selected.low + range.step, value));
    for (const bound of ["low", "high"]) selected[bound] = Number(selected[bound].toPrecision(12));
    (ui.basicYLimits[kind] ||= {})[range.unit] = selected;
    updateBasicYControl(group, range, selected);
    container.querySelector(`[data-hvac-inspect-y-reset="${kind}"]`).disabled = false;
    refreshBasicPlot(container, model, ui, kind, event.type === "change");
    return true;
  }
  if (data.hvacInspectYReset !== undefined) {
    const kind = data.hvacInspectYReset;
    delete ui.basicYLimits[kind];
    for (const range of basicChartData(model, kind).ranges) {
      const group = [...container.querySelectorAll(`[data-hvac-inspect-y-range="${kind}"]`)].find((item) => item.dataset.hvacInspectYUnit === range.unit);
      if (group) updateBasicYControl(group, range, range.auto);
    }
    target.disabled = true;
    refreshBasicPlot(container, model, ui, kind, true);
    return true;
  }
  if (target.matches("[data-simulation-hvac-frame]")) {
    if (ui.frameIndex === Number(target.value)) return true;
    ui.frameIndex = Number(target.value) || 0;
    normalize(model, ui);
    container.querySelector("[data-hvac-inspect-frame-label]").textContent = model.frames[ui.frameIndex]?.label || copy("NoFrame", "No frame");
    refreshSnapshot(container, model, ui);
    updateHVACInspectionChartFrame(container, model.frames[ui.frameIndex]?.x);
    return true;
  }
  if (data.hvacInspectNode !== undefined || data.hvacInspectComponent !== undefined || data.hvacInspectZoom !== undefined) {
    if (data.hvacInspectNode !== undefined) { ui.selectedNode = data.hvacInspectNode; ui.selectedComponent = ""; }
    if (data.hvacInspectComponent !== undefined) { ui.selectedComponent = data.hvacInspectComponent; ui.selectedNode = ""; }
    if (data.hvacInspectZoom !== undefined) ui.zoom = target.value;
    refreshSnapshot(container, model, ui);
    const attribute = [...target.attributes].find((item) => item.name.startsWith("data-hvac-inspect-"));
    if (attribute) container.querySelector(`[${attribute.name}="${CSS.escape(attribute.value)}"]`)?.focus({ preventScroll: true });
    return true;
  }
  if (data.hvacInspectNodeVisible !== undefined) {
    if ((ui.visibleNodes[data.hvacInspectNodeVisible] !== false) === target.checked) return true;
    ui.visibleNodes[data.hvacInspectNodeVisible] = target.checked;
    container.querySelector("[data-hvac-inspect-basic-charts]").innerHTML = renderBasicCharts(model, ui);
    return true;
  }
  if (data.hvacInspectMode !== undefined) ui.mode = target.value;
  if (data.hvacInspectEntity !== undefined) ui.rows[Number(data.hvacInspectEntity)] = { entity: target.value, property: "" };
  if (data.hvacInspectProperty !== undefined) ui.rows[Number(data.hvacInspectProperty)].property = target.selectedOptions?.[0]?.disabled ? "" : target.value;
  if (data.hvacInspectAdd !== undefined && ui.mode === "line" && ui.rows.length < 8) ui.rows.push({ entity: "", property: "" });
  if (data.hvacInspectRemove !== undefined && ui.rows.length > 2) ui.rows.splice(Number(data.hvacInspectRemove), 1);
  normalize(model, ui);
  const attribute = [...target.attributes].find((item) => item.name.startsWith("data-hvac-inspect-"));
  container.querySelector("[data-hvac-inspect-custom-controls]").innerHTML = renderCustomControls(model, ui);
  container.querySelector("[data-hvac-inspect-custom-chart]").innerHTML = renderCustomChart(model, ui);
  if (attribute) container.querySelector(`[${attribute.name}="${CSS.escape(attribute.value)}"]`)?.focus({ preventScroll: true });
  return true;
}

function refreshSnapshot(container, model, ui) {
  const mount = container.querySelector("[data-hvac-inspect-snapshot]");
  const viewport = mount.querySelector("[data-hvac-inspect-topology-viewport]");
  const top = viewport?.scrollTop || 0, left = viewport?.scrollLeft || 0;
  mount.innerHTML = renderSnapshot(model, ui);
  const next = mount.querySelector("[data-hvac-inspect-topology-viewport]");
  if (next) { next.scrollTop = top; next.scrollLeft = left; }
}
