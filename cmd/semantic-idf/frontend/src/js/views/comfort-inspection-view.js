import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";
import { COMFORT_BUILDING_SCOPE, prepareComfortInspection } from "../comfort-inspection-data.js";
import { renderHVACInspectionChart } from "../hvac-inspection-charts.js";

const copy = (key, fallback) => t(`simulation.comfortInspect${key}`, {}, fallback);
const finite = (value) => typeof value === "number" && Number.isFinite(value);
const number = (value) => finite(value) ? value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }) : "–";
const unit = (value) => value === "–" ? "" : value;
const titles = () => ({ temperature: copy("Temperature", "Temperature & setpoints"), pmv: "PMV", ppd: "PPD",
  humidity: copy("Humidity", "Humidity"), unmet: copy("Unmet", "Setpoint & comfort unmet time") });

function scopeName(scope) { return scope.id === COMFORT_BUILDING_SCOPE ? copy("Building", "Building") : scope.name; }

function metricLabel(metric, metrics, scope) {
  const matching = metrics.filter((item) => item.kind === metric.kind);
  const group = metric.keyValue && metric.keyValue.toLowerCase() !== scope.name.toLowerCase() ? metric.keyValue : "";
  if (group && matching.length > 1) {
    const sameGroup = matching.filter((item) => item.keyValue === metric.keyValue);
    const method = sameGroup.length > 1 ? /\b(Fanger|Pierce|KSU)\b/i.exec(metric.name)?.[1] : "";
    return `${metric.label} · ${group}${method ? ` · ${method}` : ""}`;
  }
  if (matching.length > 1) {
    const method = /\b(Fanger|Pierce|KSU)\b/i.exec(metric.name)?.[1];
    if (method) return `${metric.label} · ${method}`;
  }
  return metric.label;
}

function hoursLabel(metric) {
  const occupied = metric.kind.endsWith("Occupied");
  const base = metric.kind.startsWith("unmetHeating") ? copy("BelowSetpoint", "Below Tset,h")
    : metric.kind.startsWith("unmetCooling") ? copy("AboveSetpoint", "Above Tset,c")
      : metric.label === "ASHRAE 55" ? copy("ASHRAEDiscomfort", "Discomfort · ASHRAE 55") : copy("Discomfort", "Discomfort");
  return occupied ? `${base} · ${copy("Occupied", "occupied")}` : base;
}

function indicator(metric, scope) {
  const hours = metric.category === "unmet";
  const value = hours ? metric.summary.total : metric.summary.average;
  if (!finite(value)) return "";
  const label = hours ? hoursLabel(metric) : metricLabel(metric, scope.metrics, scope);
  const range = !hours && finite(metric.summary.min) && finite(metric.summary.max)
    ? `${copy("Range", "Range")} ${number(metric.summary.min)}–${number(metric.summary.max)} ${unit(metric.unit)}` : "";
  return `<article class="comfort-inspection-indicator" data-comfort-inspect-metric="${metric.kind}"${hours ? ` data-comfort-inspect-hours="${value}"` : ""}>
    <div class="comfort-inspection-indicator-label" title="${escapeHTML(label)}">${escapeHTML(label)}</div>
    <div class="comfort-inspection-indicator-value"><strong data-comfort-inspect-value="${value}">${escapeHTML(number(value))}</strong><span>${escapeHTML(unit(metric.unit))}</span></div>
    <div class="comfort-inspection-indicator-note">${escapeHTML(hours ? copy("Total", "Total") : copy("Average", "Average"))}${range ? `<span>${escapeHTML(range)}</span>` : ""}</div>
  </article>`;
}

function renderIndicators(scope) {
  const cards = scope.metrics.filter((metric) => metric.category !== "unmet" || !scope.hours.some((hour) => hour.kind === metric.kind)).map((metric) => indicator(metric, scope)).join("");
  const hours = scope.hours.map((metric) => `<article class="comfort-inspection-indicator" data-comfort-inspect-metric="${metric.kind}" data-comfort-inspect-hours="${metric.value}">
    <div class="comfort-inspection-indicator-label">${escapeHTML(hoursLabel(metric))}</div>
    <div class="comfort-inspection-indicator-value"><strong data-comfort-inspect-value="${metric.value}">${escapeHTML(number(metric.value))}</strong><span>${escapeHTML(unit(metric.unit))}</span></div>
    <div class="comfort-inspection-indicator-note">${escapeHTML(copy("Total", "Total"))}</div>
  </article>`).join("");
  return cards || hours ? `<div class="comfort-inspection-indicators">${cards}${hours}</div>` : "";
}

function renderScope(scope) {
  const headings = titles();
  const charts = ["temperature", "pmv", "ppd", "humidity", "unmet"].map((category) => {
    const metrics = scope.metrics.filter((metric) => metric.category === category && metric.points.some((point) => finite(point.value)));
    if (!metrics.length) return "";
    const series = metrics.map((metric) => ({ id: metric.id, label: metricLabel(metric, metrics, scope),
      legendLabel: category === "unmet" ? hoursLabel(metric) : metricLabel(metric, metrics, scope), unit: metric.unit,
      axisLabel: category === "temperature" ? "T" : category === "humidity" ? metric.label : category === "unmet" ? copy("UnmetTime", "Unmet time") : metric.label,
      points: metric.points }));
    return `<div data-comfort-inspect-chart="${category}">${renderHVACInspectionChart({ title: headings[category], series, legendLayout: "vertical" })}</div>`;
  }).join("");
  const indicators = renderIndicators(scope);
  if (!charts && !indicators) return `<div class="comfort-inspection-empty" role="status">${escapeHTML(scope.id === COMFORT_BUILDING_SCOPE
    ? copy("NoBuilding", "No building-wide comfort observations were reported.") : copy("NoData", "No comfort observations are available for this zone."))}</div>`;
  return `${indicators}${charts ? `<div class="comfort-inspection-charts">${charts}</div>` : ""}`;
}

export function renderComfortInspection(comfort = {}, ui = {}) {
  const model = prepareComfortInspection(comfort);
  const scopes = [model.building, ...model.zones];
  if (!scopes.some((scope) => scope.id === ui.selectedZone)) ui.selectedZone = model.zones[0]?.id || COMFORT_BUILDING_SCOPE;
  const selected = scopes.find((scope) => scope.id === ui.selectedZone);
  return `<section class="comfort-inspection" data-comfort-inspection>
    <div class="comfort-inspection-toolbar"><label><span>${escapeHTML(copy("Scope", "Zone / building"))}</span><select data-comfort-inspect-zone>
      ${scopes.map((scope) => `<option value="${escapeHTML(scope.id)}"${scope.id === ui.selectedZone ? " selected" : ""}>${escapeHTML(scopeName(scope))}</option>`).join("")}
    </select></label></div>
    <div data-comfort-inspect-content>${renderScope(selected)}</div>
  </section>`;
}

export function handleComfortInspectionEvent(event, container, comfort, ui) {
  const picker = event.target?.closest?.("[data-comfort-inspect-zone]");
  if (!picker || event.type !== "change") return false;
  ui.selectedZone = picker.value;
  const model = prepareComfortInspection(comfort);
  const scope = [model.building, ...model.zones].find((item) => item.id === ui.selectedZone);
  if (!scope) return false;
  const content = container.querySelector("[data-comfort-inspect-content]");
  if (content) content.innerHTML = renderScope(scope);
  return true;
}

export function renderComfortInspectionReport(comfort = {}) {
  const model = prepareComfortInspection(comfort);
  return [model.building, ...model.zones].filter((scope) => scope.metrics.length || scope.hours.length)
    .map((scope) => `<section class="comfort-inspection comfort-inspection-report"><h3>${escapeHTML(scopeName(scope))}</h3>${renderScope(scope)}</section>`).join("");
}
