import { elements, escapeHTML, state } from "./state.js";
import { renderGeometry } from "./geometry-loader.js";
import { renderInputViews } from "./input-views.js";

export function renderReport() {
  const report = state.report;
  if (!report) {
    renderEmpty();
    return;
  }

  renderSummary(report.summary);
  renderDiagnostics(report.diagnostics);
  if (state.activeResultTab === "geometry") {
    renderGeometry(report.geometry);
  } else {
    renderDeferredGeometry(report.geometry);
  }
  renderInputViews();
}

export function renderEmpty() {
  elements.summaryMetricCount.textContent = "0 metrics";
  elements.summaryCategories.innerHTML = `<div class="empty">No summary metrics yet</div>`;
  elements.geometryStats.textContent = "0 zones, 0 surfaces, 0 windows";
  elements.geometryCanvasHost.innerHTML = `<div class="empty">No geometry yet</div>`;
  elements.geometryPlan.innerHTML = "";
  elements.geometryDetails.innerHTML = `<div class="empty">Select a zone, wall, or window</div>`;
  elements.diagnosticCount.textContent = "0 issues";
  elements.diagnosticList.innerHTML = `<div class="empty">No diagnostics yet</div>`;
  elements.textObjectView.innerHTML = `<div class="empty">No formatted input yet</div>`;
  elements.jsonStructuredView.innerHTML = `<div class="empty">No structured input yet</div>`;
  elements.fieldTable.innerHTML = `<div class="empty">No field table yet</div>`;
  elements.fieldStats.textContent = "0 tables";
}

export function renderDeferredGeometry(geometry) {
  if (!state.geometryReady && state.report) {
    elements.geometryStats.textContent = "Geometry pending";
    elements.geometryCanvasHost.innerHTML = `<div class="empty status-loading">Geometry analysis is running</div>`;
    elements.geometryPlan.innerHTML = "";
    elements.geometryDetails.innerHTML = `<div class="empty">Geometry will be ready shortly</div>`;
    return;
  }
  if (!geometry) {
    elements.geometryStats.textContent = "0 zones, 0 surfaces, 0 windows";
    return;
  }
  elements.geometryStats.textContent = `${geometry.zoneCount || 0} zones, ${geometry.surfaceCount || 0} surfaces, ${geometry.windowCount || 0} windows`;
  elements.geometryCanvasHost.innerHTML = `<div class="empty">Open Geometry to render the model view</div>`;
  elements.geometryPlan.innerHTML = "";
  elements.geometryDetails.innerHTML = `<div class="empty">Open Geometry to inspect related objects</div>`;
}

export function renderSummary(summary = state.report?.summary) {
  const categories = summary?.categories || [];
  const totalMetricCount = summary?.metricCount ?? categories.reduce((sum, category) => sum + (category.metrics?.length || 0), 0);
  const query = (elements.summaryFilter?.value || "").trim().toLowerCase();
  let visibleMetricCount = 0;

  const categoryHTML = categories
    .map((category) => {
      const metrics = (category.metrics || []).filter((metric) => metricMatchesQuery(metric, category, query));
      if (!metrics.length) {
        return "";
      }
      visibleMetricCount += metrics.length;
      return `
        <details class="summary-category" open>
          <summary>
            <span>${escapeHTML(category.name)}</span>
            <span class="badge">${metrics.length}</span>
          </summary>
          <div class="summary-table" role="table" aria-label="${escapeHTML(category.name)} summary metrics">
            ${metrics.map(renderMetricRow).join("")}
          </div>
        </details>`;
    })
    .join("");

  elements.summaryMetricCount.textContent = query
    ? `${visibleMetricCount} of ${totalMetricCount} metrics`
    : `${totalMetricCount} metrics`;
  elements.summaryCategories.innerHTML = categoryHTML || `<div class="empty">No matching summary metrics</div>`;
}

export function renderDiagnostics(diagnostics = state.report?.diagnostics) {
  if (!state.diagnosticsReady && state.report) {
    elements.diagnosticCount.textContent = "Diagnostics pending";
    elements.diagnosticList.innerHTML = `<div class="empty status-loading">Diagnostics are running</div>`;
    return;
  }
  const items = diagnostics || [];
  const query = (elements.diagnosticFilter?.value || "").trim().toLowerCase();
  const visible = items.filter((item) => diagnosticMatchesQuery(item, query));
  const errorCount = items.filter((item) => item.severity === "error").length;
  const warningCount = items.filter((item) => item.severity === "warning").length;
  elements.diagnosticCount.textContent = query
    ? `${visible.length} of ${items.length} issues`
    : `${errorCount} errors, ${warningCount} warnings`;
  elements.diagnosticList.innerHTML = visible.length
    ? visible.map(renderDiagnosticItem).join("")
    : `<div class="empty">${items.length ? "No matching diagnostics" : "No diagnostics found"}</div>`;
}

function renderDiagnosticItem(item) {
  const target = item.objectIndex || item.objectIndex === 0
    ? `<button class="diagnostic-link navigable-row" data-jump-object-index="${escapeHTML(item.objectIndex)}" data-jump-object-type="${escapeHTML(item.objectType || "")}" type="button">Object #${escapeHTML(Number(item.objectIndex) + 1)}</button>`
    : "";
  const context = [item.objectType, item.objectName, item.field, item.value]
    .filter((value) => String(value ?? "").trim() !== "")
    .map((value) => `<span>${escapeHTML(value)}</span>`)
    .join("");
  return `
    <article class="diagnostic-item ${escapeHTML(item.severity || "warning")}">
      <div class="diagnostic-main">
        <div>
          <span class="diagnostic-severity">${escapeHTML(item.severity || "warning")}</span>
          <span class="diagnostic-category">${escapeHTML(item.category || "Diagnostic")}</span>
        </div>
        <strong>${escapeHTML(item.message || "")}</strong>
        <div class="diagnostic-context">${context}</div>
      </div>
      ${target}
    </article>`;
}

function diagnosticMatchesQuery(item, query) {
  if (!query) {
    return true;
  }
  return [item.severity, item.category, item.code, item.message, item.objectType, item.objectName, item.field, item.value]
    .some((value) => String(value ?? "").toLowerCase().includes(query));
}

function renderMetricRow(metric) {
  const unit = metric.unit ? `<span class="summary-unit">${escapeHTML(metric.unit)}</span>` : "";
  return `
    <div class="summary-row" role="row">
      <div class="summary-name" role="cell">
        <strong title="${escapeHTML(metric.name)}">${escapeHTML(metric.name)}</strong>
        <span>${escapeHTML(metric.id)}</span>
      </div>
      <div class="summary-value" role="cell">
        <strong>${escapeHTML(metric.displayValue ?? "N/A")}</strong>
        ${unit}
      </div>
      <span class="summary-status ${statusClass(metric.status)}" role="cell">${escapeHTML(metric.status || "missing")}</span>
    </div>`;
}

function metricMatchesQuery(metric, category, query) {
  if (!query) {
    return true;
  }
  return [metric.name, metric.id, metric.unit, metric.status, metric.displayValue, category.name]
    .some((value) => String(value ?? "").toLowerCase().includes(query));
}

function statusClass(status) {
  switch (status) {
    case "ok":
      return "summary-status-ok";
    case "partial":
      return "summary-status-partial";
    default:
      return "summary-status-missing";
  }
}
