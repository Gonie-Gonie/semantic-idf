import { elements, escapeHTML, state } from "../state.js";
import { renderGeometry } from "../geometry-loader.js";
import { renderHVAC } from "./hvac-views.js";
import { renderInputViews } from "./input-views.js";
import { renderOutput } from "./output-views.js";
import { renderProfile } from "./profile-views.js";
import { renderSimulation } from "./simulation-views.js";
import { t } from "../i18n.js";

const DIAGNOSTIC_RENDER_LIMIT = 500;

export function renderReport(options = {}) {
  const report = state.report;
  if (!report) {
    updateResultTabReadiness();
    renderEmpty();
    return;
  }
  updateResultTabReadiness();

  if (options.scope === "all") {
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
    Object.keys(state.analysisDirty || {}).forEach(markAnalysisRendered);
    return;
  }

  renderSummary(report.summary);
  markAnalysisRendered("summary");
  renderActiveResultTab(report);
  renderInputViews();
  markAnalysisRendered("input");
}

export function renderActiveResultTab(report = state.report) {
  renderResultTab(state.activeResultTab, report);
}

export function renderResultTab(tab, report = state.report) {
  if (!report) {
    return;
  }
  switch (tab) {
    case "profile":
      renderProfile(report.profile);
      markAnalysisRendered("profile");
      break;
    case "hvac":
      renderHVAC(report.hvac);
      markAnalysisRendered("hvac");
      break;
    case "output":
      renderOutput(report.output);
      markAnalysisRendered("output");
      break;
    case "simulation":
      renderSimulation();
      markAnalysisRendered("simulation");
      break;
    case "diagnose":
      renderDiagnostics(report.diagnostics);
      markAnalysisRendered("diagnose");
      break;
    case "geometry":
      if (state.geometryReady) {
        renderGeometry(report.geometry);
      } else {
        renderDeferredGeometry(report.geometry);
      }
      markAnalysisRendered("geometry");
      break;
    case "summary":
    default:
      renderSummary(report.summary);
      markAnalysisRendered("summary");
      break;
  }
}

export function markAnalysisDirty(...tabs) {
  tabs.flat().filter(Boolean).forEach((tab) => {
    if (state.analysisDirty && Object.prototype.hasOwnProperty.call(state.analysisDirty, tab)) {
      state.analysisDirty[tab] = true;
    }
  });
}

export function markAllAnalysisDirty() {
  Object.keys(state.analysisDirty || {}).forEach((tab) => {
    state.analysisDirty[tab] = true;
  });
}

export function updateResultTabReadiness() {
  elements.resultTabButtons?.forEach((button) => {
    const tab = button.dataset.resultTab || "summary";
    const readiness = resultTabReadiness(tab);
    button.dataset.readiness = readiness;
    const baseLabel = button.textContent.replace(/\s+/g, " ").trim();
    const label = `${baseLabel} · ${readiness}`;
    button.title = label;
    button.setAttribute("aria-label", label);
  });
}

function resultTabReadiness(tab) {
  if (!state.report) {
    return "pending";
  }
  if (state.analysisReady?.[tab]) {
    return "ready";
  }
  if (state.analysisStage === "queued" || state.analysisStage === "pending" || state.analysisStage === "overview") {
    return tab === "summary" ? "ready" : "pending";
  }
  if (state.analysisStage === "diagnostics" && ["diagnose", "geometry"].includes(tab)) {
    return tab === "diagnose" ? "ready" : "running";
  }
  return "deferred";
}

function markAnalysisRendered(tab) {
  if (state.analysisDirty && Object.prototype.hasOwnProperty.call(state.analysisDirty, tab)) {
    state.analysisDirty[tab] = false;
  }
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
  const renderedVisible = visible.slice(0, DIAGNOSTIC_RENDER_LIMIT);
  const hiddenVisibleCount = Math.max(0, visible.length - renderedVisible.length);
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
        hiddenVisibleCount ? `<div class="empty compact">${hiddenVisibleCount} additional diagnostics hidden. Narrow the filter to render them.</div>` : ""
      }${
        renderedVisible.length ? renderDiagnosticGroups(renderedVisible) : `<div class="empty">${t("diagnose.noMatching")}</div>`
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
  const unit = metric.unit ? escapeHTML(metric.unit) : "";
  const meta = renderMetricMeta(metric);
  const valueClass = isNumericSummaryMetric(metric) ? " summary-value-numeric" : "";
  return `
    <div class="summary-row" role="row">
      <div class="summary-name" role="cell">
        <strong title="${escapeHTML(metric.name)}">${escapeHTML(metric.name)}</strong>
      </div>
      <div class="summary-value${valueClass}" role="cell">
        ${renderMetricDisplayValue(metric)}
      </div>
      <span class="summary-unit" role="cell">${unit}</span>
      ${meta}
      ${renderMetricStatus(metric)}
    </div>`;
}

function isNumericSummaryMetric(metric) {
  return (typeof metric.value === "number" && Number.isFinite(metric.value)) || Boolean(metric.unit);
}

function renderMetricDisplayValue(metric) {
  const displayValue = String(metric.displayValue ?? "N/A");
  if (!isNumericSummaryMetric(metric)) {
    return `<strong>${escapeHTML(displayValue)}</strong>`;
  }
  const match = displayValue.match(/^(-?)(\d+)(\.\d+)?$/);
  if (!match && isNumericSummaryMetric(metric)) {
    return `
      <strong class="summary-number">
        <span class="summary-number-int">${escapeHTML(displayValue)}</span>
        <span class="summary-number-frac"></span>
      </strong>`;
  }
  if (!match) {
    return `<strong>${escapeHTML(displayValue)}</strong>`;
  }
  return `
    <strong class="summary-number">
      <span class="summary-number-int">${escapeHTML(`${match[1]}${match[2]}`)}</span>
      <span class="summary-number-frac">${escapeHTML(match[3] || "")}</span>
    </strong>`;
}

function renderMetricMeta(metric) {
  const badges = summaryNoteBadges(metric);
  if (!badges.length) {
    return `<div class="summary-meta" role="cell"></div>`;
  }
  const seen = new Set();
  const tags = badges
    .filter((badge) => {
      const key = `${badge.label}:${badge.abbr}`;
      if (seen.has(key)) {
        return false;
      }
      seen.add(key);
      return true;
    })
    .map((badge) => `<small title="${escapeHTML(badge.title)}" aria-label="${escapeHTML(badge.title)}">${escapeHTML(badge.abbr)}</small>`)
    .join("");
  return `<div class="summary-meta" role="cell">${tags}</div>`;
}

function summaryNoteBadges(metric) {
  const evidence = String(metric.evidence || "").trim();
  const rawBadges = new Set((metric.badges || []).map((badge) => String(badge || "").toLowerCase()));
  const source = String(metric.source || "").toLowerCase();
  const confidence = String(metric.confidence || "").toLowerCase();
  const badges = [];

  if (rawBadges.has("inferred") || confidence === "inferred" || source.includes("inference") || source.includes("semantic_evidence")) {
    badges.push(summaryNoteBadge("Inferred", "I", evidence ? `Inferred value. ${evidence}` : "Inferred value."));
  }
  if (rawBadges.has("orientation")) {
    badges.push(summaryNoteBadge("Orientation", "O", evidence ? `Orientation-dependent value. ${evidence}` : "Orientation-dependent value."));
  }
  if (rawBadges.has("base-surface")) {
    badges.push(summaryNoteBadge("Base surface", "B", evidence ? `Depends on base-surface resolution. ${evidence}` : "Depends on base-surface resolution."));
  }
  if (rawBadges.has("readiness")) {
    badges.push(summaryNoteBadge("Readiness", "R", evidence ? `Readiness check. ${evidence}` : "Readiness check."));
  }
  if (rawBadges.has("diagnostic")) {
    badges.push(summaryNoteBadge("Diagnostic", "D", evidence ? `Diagnostic summary. ${evidence}` : "Diagnostic summary."));
  }
  return badges;
}

function summaryNoteBadge(label, abbr, title) {
  return { label, abbr, title };
}

function renderMetricStatus(metric) {
  const status = metric.status;
  const title = metricStatusTitle(metric);
  switch (status) {
    case "ok":
      return `<span class="summary-status summary-status-ok" role="cell" aria-label="OK"></span>`;
    case "partial":
      return `
        <span class="summary-status summary-status-partial" role="cell" title="${escapeHTML(title)}" aria-label="${escapeHTML(title)}">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="M12 3 22 20H2z"></path>
            <path d="M12 9v5"></path>
            <path d="M12 17h.01"></path>
          </svg>
        </span>`;
    default:
      return `
        <span class="summary-status summary-status-missing" role="cell" title="${escapeHTML(title)}" aria-label="${escapeHTML(title)}">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <circle cx="12" cy="12" r="9"></circle>
            <path d="m15 9-6 6"></path>
            <path d="m9 9 6 6"></path>
          </svg>
        </span>`;
  }
}

function metricStatusTitle(metric) {
  const status = metric.status === "partial" ? "Partial result" : "Missing result";
  const evidence = String(metric.evidence || "").trim();
  if (evidence) {
    return `${status}. ${evidence}`;
  }
  return status;
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
    .replace(/\b\w/g, (match) => match.toUpperCase())
    .replace(/\bIdd\b/g, "IDD")
    .replace(/\bHvac\b/g, "HVAC")
    .replace(/\bIdf\b/g, "IDF");
}
