import { elements, escapeHTML, state } from "./state.js";
import { renderGeometry } from "./geometry-loader.js";
import { renderHVAC } from "./hvac-views.js";
import { renderInputViews } from "./input-views.js";
import { renderProfile } from "./profile-views.js";
import { t } from "./i18n.js";

export function renderReport() {
  const report = state.report;
  if (!report) {
    renderEmpty();
    return;
  }

  renderSummary(report.summary);
  renderProfile(report.profile);
  renderHVAC(report.hvac);
  renderDiagnostics(report.diagnostics);
  if (state.activeResultTab === "geometry") {
    renderGeometry(report.geometry);
  } else {
    renderDeferredGeometry(report.geometry);
  }
  renderInputViews();
}

export function renderEmpty() {
  elements.summaryMetricCount.textContent = t("count.metrics", { count: 0 });
  elements.summaryCategories.innerHTML = `<div class="empty">${t("summary.empty")}</div>`;
  if (elements.profileStats) {
    elements.profileStats.textContent = t("count.profiles", { profiles: 0, items: 0 });
    elements.profileSettings.innerHTML = "";
    elements.profileOverview.innerHTML = `<div class="empty">${t("profile.noAnalysis")}</div>`;
    elements.profileDetail.innerHTML = `<div class="empty">${t("profile.noProfile")}</div>`;
    elements.profileMatrixStats.textContent = t("count.zones", { count: 0 });
    elements.profileMatrix.innerHTML = `<div class="empty">${t("profile.noMatrix")}</div>`;
    elements.profileGraphStats.textContent = t("graph.representativeDay");
    elements.profileGraph.innerHTML = `<div class="empty">${t("profile.noGraph")}</div>`;
    elements.profileApplyButton.disabled = true;
  }
  if (elements.hvacStats) {
    elements.hvacStats.textContent = t("count.airPlantZone", { air: 0, plant: 0, zones: 0 });
    elements.hvacSummary.innerHTML = `<div class="empty">${t("hvac.noHVACAnalysis")}</div>`;
    elements.hvacGraph.innerHTML = `<div class="empty">${t("hvac.noLoopGraph")}</div>`;
    elements.hvacInspectorStats.textContent = t("hvac.selectNode");
    elements.hvacInspector.innerHTML = `<div class="empty">${t("hvac.noData")}</div>`;
    elements.hvacWarningStats.textContent = t("count.warnings", { count: 0 });
    elements.hvacWarnings.innerHTML = `<div class="empty">${t("hvac.noWarnings")}</div>`;
  }
  elements.geometryStats.textContent = t("geometry.stats", { zones: 0, surfaces: 0, windows: 0 });
  elements.geometryCanvasHost.innerHTML = `<div class="empty">${t("geometry.noGeometry")}</div>`;
  elements.geometryPlan.innerHTML = "";
  elements.geometryDetails.innerHTML = `<div class="empty">${t("geometry.selectObject")}</div>`;
  elements.diagnosticCount.textContent = t("count.errorsWarnings", { errors: 0, warnings: 0 });
  elements.diagnosticList.innerHTML = `<div class="empty">${t("diagnose.noDiagnosticsYet")}</div>`;
  elements.textObjectView.innerHTML = `<div class="empty">${t("input.formattedEmpty")}</div>`;
  elements.jsonStructuredView.innerHTML = `<div class="empty">${t("input.jsonEmpty")}</div>`;
  elements.fieldTable.innerHTML = `<div class="empty">${t("input.tableEmpty")}</div>`;
  elements.fieldStats.textContent = "0 tables";
}

export function renderDeferredGeometry(geometry) {
  if (!state.geometryReady && state.report) {
    elements.geometryStats.textContent = t("geometry.pending");
    elements.geometryCanvasHost.innerHTML = `<div class="empty status-loading">${t("geometry.running")}</div>`;
    elements.geometryPlan.innerHTML = "";
    elements.geometryDetails.innerHTML = `<div class="empty">${t("geometry.detailsReadySoon")}</div>`;
    return;
  }
  if (!geometry) {
    elements.geometryStats.textContent = t("geometry.stats", { zones: 0, surfaces: 0, windows: 0 });
    return;
  }
  elements.geometryStats.textContent = t("geometry.stats", {
    zones: geometry.zoneCount || 0,
    surfaces: geometry.surfaceCount || 0,
    windows: geometry.windowCount || 0,
  });
  elements.geometryCanvasHost.innerHTML = `<div class="empty">${t("geometry.openToRender")}</div>`;
  elements.geometryPlan.innerHTML = "";
  elements.geometryDetails.innerHTML = `<div class="empty">${t("geometry.openToInspect")}</div>`;
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
    ? t("count.metricsOf", { shown: visibleMetricCount, total: totalMetricCount })
    : t("count.metrics", { count: totalMetricCount });
  elements.summaryCategories.innerHTML = categoryHTML || `<div class="empty">${t("summary.noMatching")}</div>`;
}

export function renderDiagnostics(diagnostics = state.report?.diagnostics) {
  if (!state.diagnosticsReady && state.report) {
    elements.diagnosticCount.textContent = t("diagnose.pending");
    elements.diagnosticList.innerHTML = `<div class="empty status-loading">${t("diagnose.running")}</div>`;
    return;
  }
  const items = diagnostics || [];
  const query = (elements.diagnosticFilter?.value || "").trim().toLowerCase();
  const visible = items.filter((item) => diagnosticMatchesQuery(item, query));
  const errorCount = items.filter((item) => item.severity === "error").length;
  const warningCount = items.filter((item) => item.severity === "warning").length;
  elements.diagnosticCount.textContent = query
    ? t("count.metricsOf", { shown: visible.length, total: items.length })
    : t("count.errorsWarnings", { errors: errorCount, warnings: warningCount });
  elements.diagnosticList.innerHTML = visible.length
    ? visible.map(renderDiagnosticItem).join("")
    : `<div class="empty">${items.length ? t("diagnose.noMatching") : t("diagnose.noDiagnostics")}</div>`;
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
