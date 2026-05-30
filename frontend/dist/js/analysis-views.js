import { elements, escapeHTML, state } from "./state.js";
import { renderGeometry } from "./geometry-view.js";
import { renderInputViews } from "./input-views.js";

export function renderReport() {
  const report = state.report;
  if (!report) {
    renderEmpty();
    return;
  }

  renderSummary(report.summary);
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
  elements.textObjectView.innerHTML = `<div class="empty">No formatted input yet</div>`;
  elements.jsonStructuredView.innerHTML = `<div class="empty">No structured input yet</div>`;
  elements.fieldTable.innerHTML = `<div class="empty">No field table yet</div>`;
  elements.fieldStats.textContent = "0 tables";
}

function renderDeferredGeometry(geometry) {
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
