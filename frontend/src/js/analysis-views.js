import { elements, escapeHTML, state } from "./state.js";
import { renderGeometry } from "./geometry-loader.js";
import { renderHVAC } from "./hvac-views.js";
import { renderInputViews } from "./input-views.js";
import { renderOutput } from "./output-views.js";
import { renderProfile } from "./profile-views.js";
import { renderSimulation } from "./simulation-views.js";
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
  renderOutput(report.output);
  renderSimulation();
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
    elements.profileGraphStats.textContent = t("graph.annualHeatmap");
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
  if (elements.outputStats) {
    elements.outputStats.textContent = t("count.outputs", { count: 0, variables: 0, meters: 0 });
    elements.outputExistingStats.textContent = t("count.objects", { count: 0 });
    elements.outputExisting.innerHTML = `<div class="empty">${t("output.noAnalysis")}</div>`;
    elements.outputRecommendationStats.textContent = t("count.options", { count: 0 });
    elements.outputRecommendations.innerHTML = `<div class="empty">${t("output.noRecommendations")}</div>`;
    elements.outputWarningStats.textContent = t("count.warnings", { count: 0 });
    elements.outputWarnings.innerHTML = `<div class="empty">${t("output.noWarnings")}</div>`;
  }
  if (elements.simulationStats) {
    renderSimulation();
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
  pruneDiagnosticReviewState(items);
  const visible = items.filter((item) => diagnosticMatchesQuery(item, query) && diagnosticMatchesControls(item));
  const severityCounts = countBy(items, (item) => item.severity || "warning");
  const errorCount = severityCounts.get("error") || 0;
  const warningCount = severityCounts.get("warning") || 0;
  const noticeCount = severityCounts.get("notice") || 0;
  const activeControls = query || state.diagnosticSeverityFilter !== "all" || state.diagnosticSourceFilter !== "all" || state.hiddenDiagnosticCodes.size > 0;
  elements.diagnosticCount.textContent = activeControls
    ? `${visible.length} of ${items.length} diagnostics`
    : `${errorCount} errors, ${warningCount} warnings, ${noticeCount} notices`;
  elements.diagnosticList.innerHTML = items.length
    ? `${renderDiagnosticToolbar(items, visible.length)}${
        visible.length ? renderDiagnosticGroups(visible) : `<div class="empty">${t("diagnose.noMatching")}</div>`
      }`
    : `<div class="empty">${t("diagnose.noDiagnostics")}</div>`;
  bindDiagnosticControls();
}

function renderDiagnosticGroups(items) {
  const groups = groupBy(items, diagnosticGroupKey);
  return diagnosticGroupDefinitions()
    .filter((group) => groups.has(group.id))
    .map(
      (group) => {
        const diagnostics = groups.get(group.id) || [];
        return `
        <details class="diagnostic-group diagnostic-group-${escapeHTML(group.id)}" open>
          <summary class="diagnostic-group-head">
            <strong>${escapeHTML(group.label)}</strong>
            <span class="badge">${escapeHTML(diagnostics.length)}</span>
          </summary>
          <div class="diagnostic-group-list">${diagnostics.map(renderDiagnosticItem).join("")}</div>
        </details>`;
      },
    )
    .join("");
}

function renderDiagnosticItem(item) {
  const target = item.objectIndex || item.objectIndex === 0
    ? `<button class="diagnostic-link navigable-row" data-jump-object-index="${escapeHTML(item.objectIndex)}" data-jump-object-type="${escapeHTML(item.objectType || "")}" type="button">Object #${escapeHTML(Number(item.objectIndex) + 1)}</button>`
    : "";
  const context = [item.objectType, item.objectName, item.field, item.value]
    .filter((value) => String(value ?? "").trim() !== "")
    .map((value) => `<span>${escapeHTML(value)}</span>`)
    .join("");
  const severity = item.severity || "warning";
  const code = String(item.code || "").trim();
  const source = item.source ? `<span class="diagnostic-source">${escapeHTML(sourceLabel(item.source))}</span>` : "";
  const confidence = item.confidence ? `<span class="diagnostic-confidence">${escapeHTML(item.confidence)}</span>` : "";
  const codeBadge = code ? `<span class="diagnostic-code">${escapeHTML(code)}</span>` : "";
  const hideButton = code
    ? `<button class="diagnostic-code-action" data-diagnostic-hide-code="${escapeHTML(code)}" type="button" title="Hide this diagnostic code">Hide code</button>`
    : "";
  return `
    <details class="diagnostic-item ${escapeHTML(severity)}">
      <summary class="diagnostic-summary">
        <span class="diagnostic-row-main">
          <span class="diagnostic-severity">${escapeHTML(severity)}</span>
          <span class="diagnostic-category">${escapeHTML(item.category || "Diagnostic")}</span>
          <strong>${escapeHTML(item.message || "")}</strong>
        </span>
        <span class="diagnostic-row-actions">${codeBadge}${hideButton}</span>
      </summary>
      <div class="diagnostic-main">
        ${source || confidence ? `<div class="diagnostic-meta">${source}${confidence}</div>` : ""}
        ${context ? `<div class="diagnostic-context">${context}</div>` : ""}
        ${item.evidence ? `<p class="diagnostic-evidence">${escapeHTML(item.evidence)}</p>` : ""}
        ${target ? `<div>${target}</div>` : ""}
      </div>
    </details>`;
}

function diagnosticMatchesQuery(item, query) {
  if (!query) {
    return true;
  }
  return [item.severity, item.category, item.code, item.source, item.confidence, item.evidence, item.message, item.objectType, item.objectName, item.field, item.value]
    .some((value) => String(value ?? "").toLowerCase().includes(query));
}

function diagnosticMatchesControls(item) {
  const severity = item.severity || "warning";
  if (state.diagnosticSeverityFilter !== "all" && severity !== state.diagnosticSeverityFilter) {
    return false;
  }
  const source = item.source || "unspecified";
  if (state.diagnosticSourceFilter !== "all" && source !== state.diagnosticSourceFilter) {
    return false;
  }
  return !state.hiddenDiagnosticCodes.has(String(item.code || "").trim());
}

function renderDiagnosticToolbar(items, visibleCount) {
  const severities = ["all", "error", "warning", "notice"];
  const severityCounts = countBy(items, (item) => item.severity || "warning");
  const sources = diagnosticSourceOptions(items);
  const hiddenCodes = [...state.hiddenDiagnosticCodes].sort();
  return `
    <div class="diagnostic-toolbar">
      <div class="diagnostic-filter-row" aria-label="Diagnostic severity filters">
        ${severities
          .map((severity) =>
            renderDiagnosticFilterButton({
              kind: "severity",
              value: severity,
              active: state.diagnosticSeverityFilter === severity,
              label: severity === "all" ? "All severities" : sourceLabel(severity),
              count: severity === "all" ? items.length : severityCounts.get(severity) || 0,
            }),
          )
          .join("")}
      </div>
      <div class="diagnostic-filter-row" aria-label="Diagnostic source filters">
        ${sources
          .map((source) =>
            renderDiagnosticFilterButton({
              kind: "source",
              value: source,
              active: state.diagnosticSourceFilter === source,
              label: source === "all" ? "All sources" : sourceLabel(source),
              count: source === "all" ? items.length : items.filter((item) => (item.source || "unspecified") === source).length,
            }),
          )
          .join("")}
      </div>
      <div class="diagnostic-toolbar-status">
        <span>${escapeHTML(visibleCount)} visible</span>
        ${
          hiddenCodes.length
            ? `<span>${escapeHTML(hiddenCodes.length)} hidden code${hiddenCodes.length === 1 ? "" : "s"}</span><button class="diagnostic-clear-hidden" data-diagnostic-clear-hidden type="button">Show hidden</button>`
            : ""
        }
      </div>
    </div>`;
}

function renderDiagnosticFilterButton({ kind, value, active, label, count }) {
  return `
    <button class="diagnostic-filter-button ${active ? "active" : ""}" data-diagnostic-filter-kind="${escapeHTML(kind)}" data-diagnostic-filter-value="${escapeHTML(value)}" type="button">
      <span>${escapeHTML(label)}</span>
      <small>${escapeHTML(count)}</small>
    </button>`;
}

function bindDiagnosticControls() {
  elements.diagnosticList.querySelectorAll("[data-diagnostic-filter-kind]").forEach((button) => {
    button.addEventListener("click", () => {
      const kind = button.dataset.diagnosticFilterKind;
      if (kind === "severity") {
        state.diagnosticSeverityFilter = button.dataset.diagnosticFilterValue || "all";
      }
      if (kind === "source") {
        state.diagnosticSourceFilter = button.dataset.diagnosticFilterValue || "all";
      }
      renderDiagnostics();
    });
  });
  elements.diagnosticList.querySelectorAll("[data-diagnostic-hide-code]").forEach((button) => {
    button.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
      const code = String(button.dataset.diagnosticHideCode || "").trim();
      if (code) {
        state.hiddenDiagnosticCodes.add(code);
        renderDiagnostics();
      }
    });
  });
  elements.diagnosticList.querySelectorAll("[data-diagnostic-clear-hidden]").forEach((button) => {
    button.addEventListener("click", () => {
      state.hiddenDiagnosticCodes.clear();
      renderDiagnostics();
    });
  });
}

function diagnosticSourceOptions(items) {
  const priority = ["energyplus_rule", "analyzer_rule", "analyzer_limitation", "heuristic_inference", "user_quality_check", "schema_role_reference"];
  const sources = new Set(items.map((item) => item.source || "unspecified"));
  return [
    "all",
    ...priority.filter((source) => sources.has(source)),
    ...[...sources].filter((source) => !priority.includes(source)).sort(),
  ];
}

function diagnosticGroupDefinitions() {
  return [
    { id: "errors", label: "Errors" },
    { id: "warnings", label: "Warnings" },
    { id: "analyzer-limitations", label: "Analyzer limitations" },
    { id: "cleanup-candidates", label: "Cleanup candidates" },
    { id: "notices", label: "Notices" },
  ];
}

function diagnosticGroupKey(item) {
  if ((item.severity || "warning") === "error") {
    return "errors";
  }
  if (item.source === "analyzer_limitation") {
    return "analyzer-limitations";
  }
  if (item.source === "user_quality_check" || String(item.category || "").toLowerCase().includes("cleanup")) {
    return "cleanup-candidates";
  }
  if ((item.severity || "warning") === "warning") {
    return "warnings";
  }
  return "notices";
}

function pruneDiagnosticReviewState(items) {
  const knownCodes = new Set(items.map((item) => String(item.code || "").trim()).filter(Boolean));
  const knownSources = new Set(items.map((item) => item.source || "unspecified"));
  if (state.diagnosticSourceFilter !== "all" && !knownSources.has(state.diagnosticSourceFilter)) {
    state.diagnosticSourceFilter = "all";
  }
  for (const code of [...state.hiddenDiagnosticCodes]) {
    if (!knownCodes.has(code)) {
      state.hiddenDiagnosticCodes.delete(code);
    }
  }
}

function countBy(values, keyFn) {
  const counts = new Map();
  for (const value of values) {
    const key = keyFn(value);
    counts.set(key, (counts.get(key) || 0) + 1);
  }
  return counts;
}

function renderMetricRow(metric) {
  const unit = metric.unit ? `<span class="summary-unit">${escapeHTML(metric.unit)}</span>` : "";
  const meta = renderMetricMeta(metric);
  const compact = shouldRenderCompactMetricRow(metric) ? " compact" : "";
  return `
    <div class="summary-row${compact}" role="row">
      <div class="summary-name" role="cell">
        <strong title="${escapeHTML(metric.name)}">${escapeHTML(metric.name)}</strong>
        <span>${escapeHTML(metric.id)}</span>
        ${meta}
      </div>
      <div class="summary-value" role="cell">
        <strong>${escapeHTML(metric.displayValue ?? "N/A")}</strong>
        ${unit}
      </div>
      <span class="summary-status ${statusClass(metric.status)}" role="cell">${escapeHTML(metric.status || "missing")}</span>
    </div>`;
}

function shouldRenderCompactMetricRow(metric) {
  const badges = metric.badges || [];
  return String(metric.name || "").length > 34 ||
    metric.visibility === "advanced" ||
    badges.includes("inferred") ||
    badges.includes("readiness");
}

function renderMetricMeta(metric) {
  const badges = [
    metric.source ? sourceLabel(metric.source) : "",
    metric.confidence || "",
    metric.visibility === "advanced" ? "advanced" : "",
    ...(metric.badges || []),
  ].filter(Boolean);
  if (!badges.length && !metric.evidence) {
    return "";
  }
  return `
    <div class="summary-meta" title="${escapeHTML(metric.evidence || "")}">
      ${[...new Set(badges)].map((badge) => `<small>${escapeHTML(badge)}</small>`).join("")}
    </div>`;
}

function metricMatchesQuery(metric, category, query) {
  if (!query) {
    return true;
  }
  return [metric.name, metric.id, metric.unit, metric.status, metric.displayValue, metric.source, metric.confidence, metric.visibility, metric.evidence, ...(metric.badges || []), category.name]
    .some((value) => String(value ?? "").toLowerCase().includes(query));
}

function groupBy(values, keyFn) {
  const groups = new Map();
  for (const value of values) {
    const key = keyFn(value);
    if (!groups.has(key)) {
      groups.set(key, []);
    }
    groups.get(key).push(value);
  }
  return groups;
}

function sourceLabel(source) {
  return String(source || "")
    .replaceAll("_", " ")
    .replace(/\b\w/g, (match) => match.toUpperCase());
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
