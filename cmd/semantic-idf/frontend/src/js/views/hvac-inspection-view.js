import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";
import { prepareHVACInspection, hvacInspectionSnapshots, hvacInspectionTrace, hvacInspectionBasicProperties } from "../hvac-inspection-data.js";
import { renderHVACInspectionChart, updateHVACInspectionChartFrame } from "../hvac-inspection-charts.js";
import { renderHVACInspectionTopology } from "./hvac-inspection-topology.js";

const copy = (key, fallback) => t(`simulation.hvacInspect${key}`, {}, fallback);
const option = (value, label, selected, disabled = false) => `<option value="${escapeHTML(value)}"${selected ? " selected" : ""}${disabled ? " disabled" : ""}>${escapeHTML(label)}</option>`;

function normalize(model, ui) {
  ui.frameIndex = Math.max(0, Math.min(model.frames.length - 1, Math.round(Number(ui.frameIndex) || 0)));
  ui.visibleNodes ||= {};
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
  const frameKey = model.frames[ui.frameIndex]?.x;
  return [
    ["flow", copy("Flow", "Mass flow")],
    ["temperature", copy("Temperature", "Temperature")],
    ["humidity", copy("HumidityTitle", "Humidity")],
  ].map(([kind, title]) => `<div data-hvac-inspect-basic-chart="${kind}">${renderHVACInspectionChart({ title, frameKey,
    series: hvacInspectionBasicProperties(model, kind, ui.visibleNodes).map((property) => ({ ...hvacInspectionTrace(model, property), axisLabel: property.label })) })}</div>`).join("");
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
  const target = event.target.closest?.("[data-simulation-hvac-frame], [data-hvac-inspect-node], [data-hvac-inspect-component], [data-hvac-inspect-node-visible], [data-hvac-inspect-entity], [data-hvac-inspect-property], [data-hvac-inspect-mode], [data-hvac-inspect-add], [data-hvac-inspect-remove], [data-hvac-inspect-zoom]");
  if (!target || !container.contains(target)) return false;
  const isClick = event.type === "click", clickControl = target.matches("[data-hvac-inspect-node], [data-hvac-inspect-component], [data-hvac-inspect-add], [data-hvac-inspect-remove]");
  if (isClick !== clickControl || !isClick && event.type !== "input" && event.type !== "change") return false;
  event.stopPropagation();
  if (isClick) event.preventDefault();
  const model = prepareHVACInspection(loop);
  normalize(model, ui);
  const data = target.dataset;
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
