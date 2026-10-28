import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";
import { prepareSimulationComparison, normalizeSimulationComparison, simulationComparisonTraces, simulationComparisonFrame } from "../simulation-comparison-data.js";
import { renderHVACInspectionChart, updateHVACInspectionChartFrame, handleHVACChartLegendEvent } from "../hvac-inspection-charts.js";

const safe = escapeHTML;
const copy = (key, fallback) => t(`simulation.comparison${key}`, {}, fallback);
const hvacCopy = (key, fallback) => t(`simulation.hvacInspect${key}`, {}, fallback);
const option = (value, label, selected, disabled = false, title = "") => `<option value="${safe(value)}"${selected ? " selected" : ""}${disabled ? " disabled" : ""}${title ? ` title="${safe(title)}"` : ""}>${safe(label)}</option>`;
const selected = (model, ui) => ui.rows.map(row => model.properties.find(property => property.id === row.property)).filter(Boolean);
const plots = new WeakMap();

function controls(model, ui) {
  return `<div class="hvac-inspection-custom-heading"><h4>${safe(copy("Title", "Comparison graph"))}</h4><label><span>${safe(hvacCopy("GraphType", "Graph type"))}</span><select data-simulation-comparison-mode>${option("line", "Line", ui.mode === "line")}${option("scatter", "Scatter", ui.mode === "scatter")}${option("histogram", t("simulation.hvacChartHistogram", {}, "Histogram"), ui.mode === "histogram")}</select></label></div>
    <div class="hvac-inspection-custom-rows">${ui.rows.map((row, index) => {
      const entity = model.entities.find(item => item.id === row.entity), current = entity?.properties.find(property => property.id === row.property), others = selected(model, { rows: ui.rows.filter((_, other) => other !== index) }), units = new Set(others.map(property => property.unit));
      return `<div class="hvac-inspection-custom-row simulation-comparison-row"><span>${ui.mode === "scatter" ? index === 0 ? "X" : "Y" : index + 1}</span>
        <label><span>${safe(copy("Category", "Category"))}</span><select data-simulation-comparison-category="${index}">${option("", "—", !entity)}${model.categories.map(category => option(category.id, category.label, category.id === entity?.category)).join("")}</select></label>
        <label><span>${safe(copy("Target", "Target"))}</span><select data-simulation-comparison-entity="${index}"${entity ? "" : " disabled"}>${option("", "—", !entity)}${model.entities.filter(item => item.category === entity?.category).map(item => option(item.id, item.name, item.id === row.entity)).join("")}</select></label>
        <label><span>${safe(copy("Variable", "Variable"))}</span><select data-simulation-comparison-property="${index}"${current?.name ? ` title="${safe(current.name)}"` : ""}${entity ? "" : " disabled"}>${option("", "—", !row.property)}${(entity?.properties || []).map(property => option(property.id, `${property.label} (${property.unit}${property.frequency ? ` · ${property.frequency}` : ""})`, property.id === row.property, others.some(other => other.id === property.id) || ui.mode === "line" && units.size >= 2 && !units.has(property.unit), property.name)).join("")}</select></label>
        ${ui.mode !== "scatter" && ui.rows.length > 2 ? `<button type="button" data-simulation-comparison-remove="${index}" aria-label="${safe(hvacCopy("Remove", "Remove series"))}">×</button>` : ""}</div>`;
    }).join("")}</div>${ui.mode !== "scatter" ? `<button type="button" data-simulation-comparison-add${ui.rows.length >= 8 ? " disabled" : ""}>${safe(hvacCopy("Add", "Add series"))}</button>` : ""}`;
}

function chart(model, ui, mount) {
  const properties = selected(model, ui), traces = simulationComparisonTraces(model, properties), frameKey = simulationComparisonFrame(model, traces);
  const series = ui.mode === "histogram" ? traces.map((trace, index) => ({ ...trace, legendLabel: `${trace.label} (${trace.unit}${properties[index].frequency ? ` · ${properties[index].frequency}` : ""})` })) : traces;
  if (mount) plots.set(mount, { model, series });
  return renderHVACInspectionChart({ markers: false, mode: ui.mode, frameKey, series });
}

export function renderSimulationComparison(result, view, ui = {}, context = {}) {
  const model = prepareSimulationComparison(result, view, context); normalizeSimulationComparison(model, ui);
  return `<section class="hvac-inspection-custom simulation-comparison" data-simulation-comparison="${safe(view)}"><div data-simulation-comparison-controls>${controls(model, ui)}</div><div data-simulation-comparison-chart>${chart(model, ui)}</div></section>`;
}

export function handleSimulationComparisonEvent(event, container, result, view, ui = {}, context = {}) {
  const wrapper = event.target.closest?.("[data-simulation-comparison]");
  if (!wrapper || !container.contains(wrapper)) return false;
  if (handleHVACChartLegendEvent(event, wrapper)) return true;
  const target = event.target.closest("[data-simulation-comparison-mode],[data-simulation-comparison-category],[data-simulation-comparison-entity],[data-simulation-comparison-property],[data-simulation-comparison-add],[data-simulation-comparison-remove]");
  if (!target) return false;
  const click = target.matches("button");
  if (click ? event.type !== "click" : !["change", "input"].includes(event.type)) return false;
  const model = prepareSimulationComparison(result, view, context); normalizeSimulationComparison(model, ui);
  if (target.matches("select") && target.selectedOptions?.[0]?.disabled) return true;
  if (target.hasAttribute("data-simulation-comparison-mode")) ui.mode = target.value;
  if (target.hasAttribute("data-simulation-comparison-category")) {
    const entity = model.entities.find(item => item.category === target.value); ui.rows[Number(target.dataset.simulationComparisonCategory)] = { entity: entity?.id || "", property: "" };
  }
  if (target.hasAttribute("data-simulation-comparison-entity")) ui.rows[Number(target.dataset.simulationComparisonEntity)] = { entity: target.value, property: "" };
  if (target.hasAttribute("data-simulation-comparison-property")) ui.rows[Number(target.dataset.simulationComparisonProperty)].property = target.value;
  if (target.hasAttribute("data-simulation-comparison-add") && ui.mode !== "scatter" && ui.rows.length < 8) ui.rows.push({ entity: "", property: "" });
  if (target.hasAttribute("data-simulation-comparison-remove") && ui.mode !== "scatter" && ui.rows.length > 2) ui.rows.splice(Number(target.dataset.simulationComparisonRemove), 1);
  normalizeSimulationComparison(model, ui);
  const attribute = [...target.attributes].find(item => item.name.startsWith("data-simulation-comparison-"));
  wrapper.querySelector("[data-simulation-comparison-controls]").innerHTML = controls(model, ui);
  wrapper.querySelector("[data-simulation-comparison-chart]").innerHTML = chart(model, ui, wrapper);
  if (attribute) wrapper.querySelector(`[${attribute.name}="${CSS.escape(attribute.value)}"]`)?.focus({ preventScroll: true });
  return true;
}

export function updateSimulationComparisonFrame(container, result, view, ui = {}, context = {}) {
  const wrapper = container.matches?.("[data-simulation-comparison]") ? container : container.querySelector("[data-simulation-comparison]");
  if (!wrapper) return;
  const model = prepareSimulationComparison(result, view, context); normalizeSimulationComparison(model, ui);
  const saved = plots.get(wrapper), series = saved?.model === model ? saved.series : simulationComparisonTraces(model, selected(model, ui));
  plots.set(wrapper, { model, series }); updateHVACInspectionChartFrame(wrapper, simulationComparisonFrame(model, series, context));
}
