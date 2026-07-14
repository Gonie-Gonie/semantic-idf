import { elements, escapeHTML, refreshStatusTitle, state } from "../state.js";
import { renderGeometry } from "../geometry-loader.js";
import { renderHVAC } from "./hvac-views.js";
import { renderInputViews } from "./input-views.js";
import { renderOutput } from "./output-views.js";
import { renderProfile } from "./profile-views.js";
import { renderSimulation } from "./simulation-views.js";
import { t } from "../i18n.js";
import {
  configureResultPanelNavigationHooks,
  extractResultPanelSelection,
  refreshResultPanelSelectionStyles,
} from "../panel-navigation-adapters.js";
import { currentSemanticSelection, revealSelectionSource, selectSemanticEntity } from "../selection-controller.js";

const DIAGNOSTIC_RENDER_LIMIT = 500;
const SUMMARY_SOURCE_RENDER_LIMIT = 48;

let summarySourceIndexCache = { navigation: null, records: [] };
let diagnoseSelectedDiagnosticID = "";
let diagnoseTemporaryRevealID = "";
let preservingDiagnoseContext = false;

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
  if (renderPendingResultTab(tab)) {
    return;
  }
  const startedAt = nowMS();
  try {
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
  } finally {
    recordRenderTiming(tab, nowMS() - startedAt);
  }
}

function renderPendingResultTab(tab) {
  if (state.analysisReady?.[tab] || state.analysisStage === "complete") {
    return false;
  }
  switch (tab) {
    case "profile":
      elements.profileStats.textContent = t("profile.pending", {}, "Profile pending");
      elements.profileOverview.innerHTML = `<div class="empty status-loading">${t("profile.running", {}, "Building profile graphs")}</div>`;
      elements.profileDetail.innerHTML = `<div class="empty">${t("profile.readySoon", {}, "Profile details will appear when this stage is ready.")}</div>`;
      elements.profileMatrixStats.textContent = t("profile.pending", {}, "Profile pending");
      elements.profileMatrix.innerHTML = `<div class="empty">${t("profile.readySoon", {}, "Profile details will appear when this stage is ready.")}</div>`;
      elements.profileGraphStats.textContent = t("profile.pending", {}, "Profile pending");
      elements.profileGraph.innerHTML = `<div class="empty status-loading">${t("profile.running", {}, "Building profile graphs")}</div>`;
      return true;
    case "hvac":
      elements.hvacStats.textContent = t("hvac.pending", {}, "HVAC pending");
      elements.hvacSummary.innerHTML = `<div class="empty status-loading">${t("hvac.running", {}, "Resolving HVAC service paths")}</div>`;
      elements.hvacGraph.innerHTML = `<div class="empty">${t("hvac.readySoon", {}, "HVAC graph will appear when this stage is ready.")}</div>`;
      elements.hvacInspectorStats.textContent = t("hvac.pending", {}, "HVAC pending");
      elements.hvacInspector.innerHTML = `<div class="empty">${t("hvac.readySoon", {}, "HVAC graph will appear when this stage is ready.")}</div>`;
      elements.hvacWarningStats.textContent = t("hvac.pending", {}, "HVAC pending");
      elements.hvacWarnings.innerHTML = `<div class="empty">${t("hvac.readySoon", {}, "HVAC graph will appear when this stage is ready.")}</div>`;
      return true;
    default:
      return false;
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

function recordRenderTiming(tab, elapsedMS) {
  if (!state.renderTiming) {
    state.renderTiming = { tabs: {}, last: null };
  }
  state.renderTiming.tabs = state.renderTiming.tabs || {};
  state.renderTiming.tabs[tab] = elapsedMS;
  state.renderTiming.last = { tab, ms: elapsedMS, at: Date.now() };
  refreshStatusTitle();
}

function nowMS() {
  return typeof performance !== "undefined" && typeof performance.now === "function" ? performance.now() : Date.now();
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
      const categoryNavigation = summaryNavigationForCategory(category);
      return `
        <details class="summary-category" data-summary-category-id="${escapeHTML(category.id)}" ${panelNavigationAttributes({
          ...categoryNavigation,
          panelTargetId: category.id,
        })} open>
          <summary>
            <span>${escapeHTML(category.name)}</span>
            <span class="badge">${metrics.length}</span>
          </summary>
          <div class="summary-table" role="table" aria-label="${escapeHTML(category.name)} summary metrics">
            ${metrics.map((metric) => renderMetricRow(metric, category)).join("")}
          </div>
        </details>`;
    })
    .join("");

  elements.summaryMetricCount.textContent = query
    ? t("count.metricsOf", { shown: visibleMetricCount, total: totalMetricCount })
    : t("count.metrics", { count: totalMetricCount });
  elements.summaryCategories.innerHTML = categoryHTML || `<div class="empty">${t("summary.noMatching")}</div>`;
  refreshSummaryNavigationStyles();
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
  let visible = items.filter((item) => diagnosticMatchesQuery(item, query) && diagnosticMatchesControls(item));
  const temporaryItem = diagnoseTemporaryRevealID
    ? items.find((item) => diagnosticStableID(item) === diagnoseTemporaryRevealID) || null
    : null;
  const temporarilyHidden = Boolean(temporaryItem && !visible.includes(temporaryItem));
  if (temporarilyHidden) {
    visible = [...visible, temporaryItem];
  }
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
        temporarilyHidden
          ? `<div class="diagnostic-temporary-reveal">${escapeHTML(t("semantic.temporaryReveal", {}, "Temporarily revealing selected item"))}<button type="button" data-diagnostic-clear-temporary>${escapeHTML(t("semantic.clearTemporaryReveal", {}, "Clear temporary reveal"))}</button></div>`
          : ""
      }${
        hiddenVisibleCount ? `<div class="empty compact">${hiddenVisibleCount} additional diagnostics hidden. Narrow the filter to render them.</div>` : ""
      }${
        renderedVisible.length ? renderDiagnosticGroups(renderedVisible) : `<div class="empty">${t("diagnose.noMatching")}</div>`
      }`
    : `<div class="empty">${t("diagnose.noDiagnostics")}</div>`;
  bindDiagnosticControls();
  refreshDiagnoseNavigationStyles();
}

function renderDiagnosticGroups(items) {
  const groups = groupBy(items, diagnosticGroupKey);
  return diagnosticGroupDefinitions()
    .filter((group) => groups.has(group.id))
    .map(
      (group) => {
        const diagnostics = groups.get(group.id) || [];
        const groupNavigation = navigationSelectionForViewTarget("diagnose", group.id);
        return `
        <details class="diagnostic-group diagnostic-group-${escapeHTML(group.id)}" data-diagnostic-group-id="${escapeHTML(group.id)}" ${panelNavigationAttributes({
          ...groupNavigation,
          panelTargetId: group.id,
        })} open>
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
  const stableID = diagnosticStableID(item);
  const navigation = diagnosticSemanticNavigation(item, stableID);
  const sourceAnchor = navigation.sourceAnchor || diagnosticSourceAnchor(item);
  const target = sourceAnchor
    ? `<button class="diagnostic-link" data-diagnostic-reveal-source type="button">Reveal source</button>`
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
  const selected = diagnoseSelectedDiagnosticID === stableID || (
    state.globalSelection?.originView === "diagnose" && state.globalSelection?.originTargetId === stableID
  );
  return `
    <details class="diagnostic-item ${escapeHTML(severity)} ${selected ? "semantic-selected" : ""}" data-diagnostic-id="${escapeHTML(stableID)}" data-diagnostic-stable-id="${escapeHTML(stableID)}" data-diagnostic-code="${escapeHTML(code)}" ${panelNavigationAttributes({
      ...navigation,
      sourceAnchor,
      panelTargetId: stableID,
    })} ${selected ? "data-semantic-selected" : ""}>
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
  elements.diagnosticList.querySelectorAll("[data-diagnostic-id]").forEach((item) => {
    item.addEventListener("click", (event) => {
      if (event.target.closest("[data-diagnostic-hide-code], [data-diagnostic-reveal-source]")) {
        return;
      }
      diagnoseSelectedDiagnosticID = item.dataset.diagnosticId || "";
      if (diagnoseTemporaryRevealID && diagnoseTemporaryRevealID !== diagnoseSelectedDiagnosticID) {
        diagnoseTemporaryRevealID = "";
      }
      refreshDiagnoseNavigationStyles();
    });
  });
  elements.diagnosticList.querySelectorAll("[data-diagnostic-reveal-source]").forEach((button) => {
    button.addEventListener("click", async (event) => {
      event.preventDefault();
      event.stopPropagation();
      const selection = extractResultPanelSelection(button, "diagnose", { root: elements.diagnosticList });
      if (!selection?.entityId || !selection.sourceAnchor) {
        return;
      }
      diagnoseSelectedDiagnosticID = selection.originTargetId || diagnoseSelectedDiagnosticID;
      const previous = currentSemanticSelection();
      const selectionChanged = diagnosticSelectionKey(previous) !== diagnosticSelectionKey(selection);
      await selectSemanticEntity(selection, {
        originView: "diagnose",
        follow: false,
        recordHistory: selectionChanged,
      });
      await revealSelectionSource({
        originView: "diagnose",
        recordHistory: !selectionChanged,
      });
    });
  });
  elements.diagnosticList.querySelector("[data-diagnostic-clear-temporary]")?.addEventListener("click", () => {
    diagnoseTemporaryRevealID = "";
    renderDiagnostics();
  });
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

function diagnosticSelectionKey(selection = {}) {
  const anchor = selection.sourceAnchor || {};
  return [selection.entityId, selection.occurrenceId, anchor.objectId, anchor.objectIndex, anchor.fieldIndex]
    .map((value) => String(value ?? ""))
    .join("\u0000");
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
  if (preservingDiagnoseContext) {
    return;
  }
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

function semanticNavigationIndex() {
  return state.semanticProjection?.navigation || {};
}

function navigationEntity(entityID, navigation = semanticNavigationIndex()) {
  return entityID
    ? (navigation.entities || []).find((entity) => entity.id === entityID) || null
    : null;
}

function navigationOccurrence(occurrenceID, navigation = semanticNavigationIndex()) {
  return occurrenceID
    ? (navigation.occurrences || []).find((occurrence) => occurrence.occurrenceId === occurrenceID) || null
    : null;
}

function navigationSelectionForViewTarget(view, targetID, options = {}) {
  const navigation = semanticNavigationIndex();
  const occurrenceIDs = navigation.byViewTarget?.[`${view}|${targetID}`] || [];
  const sections = options.sections || [];
  const candidates = occurrenceIDs
    .map((occurrenceID, order) => {
      const occurrence = navigationOccurrence(occurrenceID, navigation);
      const entity = navigationEntity(occurrence?.entityId, navigation);
      if (!occurrence || !entity) {
        return null;
      }
      const sectionRank = preferredSectionRank(entity, occurrence, sections);
      return {
        entity,
        occurrence,
        order,
        score:
          Number(sectionRank >= 0) * (10_000_000 - sectionRank * 1_000_000) +
          Number(entity.kind === "semantic-section") * 100_000 +
          Number((occurrence.lineIndexes || []).length > 0) * 10_000 +
          Number(occurrence.occurrenceId === state.semanticCurrentOccurrenceId) * 1_000,
      };
    })
    .filter(Boolean)
    .sort((left, right) => right.score - left.score || left.order - right.order);
  const selected = candidates[0] || null;
  return selected
    ? navigationMetadata(selected.entity, selected.occurrence, selected.occurrence.sourceAnchor)
    : { entity: null, occurrence: null, sourceAnchor: null };
}

function preferredSectionRank(entity, occurrence, sections) {
  if (!sections.length) {
    return -1;
  }
  const entityID = String(entity?.id || "");
  const topLevelPath = String(occurrence?.path || "").split("/").filter(Boolean)[0] || "";
  return sections.findIndex((section) => (
    topLevelPath === section || entityID.endsWith(`:section:${section}`)
  ));
}

function navigationMetadata(entity, occurrence, sourceAnchor = occurrence?.sourceAnchor || null) {
  return {
    entity,
    occurrence,
    sourceAnchor: sourceAnchor ? { ...sourceAnchor } : null,
  };
}

function summaryNavigationForCategory(category = {}) {
  return navigationSelectionForViewTarget("summary", category.id || "", {
    sections: summaryCategorySections(category.id),
  });
}

function summaryMetricNavigation(metric = {}, category = {}) {
  const categoryID = category.id || metric.categoryId || "model_inventory";
  return navigationSelectionForViewTarget("summary", categoryID, {
    sections: summaryMetricSections(metric, categoryID),
  });
}

function summaryCategorySections(categoryID) {
  switch (categoryID) {
    case "geometry_areas":
      return ["geometry", "zones", "spaces", "site_geometry"];
    case "envelope_fenestration":
      return ["envelope", "fenestration", "constructions", "materials", "geometry"];
    case "internal_loads":
      return ["loads", "profiles", "internal_loads"];
    case "schedules_operation":
      return ["schedules", "operation", "profiles"];
    case "hvac_conditioning":
      return ["hvac", "services", "zones"];
    default:
      return ["project", "site", "zones", "outputs", "source_name_conflicts"];
  }
}

function summaryMetricSections(metric = {}, categoryID = metric.categoryId || "") {
  const id = String(metric.id || "").toLowerCase();
  if (id.includes("diagnostic")) {
    return ["source_name_conflicts", "diagnostics", "project"];
  }
  if (id.includes("output")) {
    return ["outputs", "project"];
  }
  if (id.includes("zone") || id.includes("floor_area") || id.includes("volume") || id.includes("wwr") || id.includes("envelope") || id.includes("wall") || id.includes("roof") || id.includes("window") || id.includes("door") || id.includes("footprint") || id.includes("bounding_box")) {
    return ["zones", "geometry", "envelope", "fenestration", "spaces"];
  }
  if (id.includes("load") || id.includes("lighting") || id.includes("equipment") || id.includes("people")) {
    return ["loads", "profiles", "internal_loads"];
  }
  if (id.includes("schedule") || id.includes("operating_hours")) {
    return ["schedules", "operation", "profiles"];
  }
  if (id.includes("hvac") || id.includes("thermostat") || id.includes("conditioned")) {
    return ["hvac", "services", "zones"];
  }
  return summaryCategorySections(categoryID);
}

function summarySourceRecords() {
  const navigation = semanticNavigationIndex();
  if (summarySourceIndexCache.navigation === navigation) {
    return summarySourceIndexCache.records;
  }
  const recordsByKey = new Map();
  for (const occurrence of navigation.occurrences || []) {
    const anchor = occurrence?.sourceAnchor;
    if (!anchor || (!anchor.objectId && !hasNavigationIndex(anchor.objectIndex))) {
      continue;
    }
    const key = anchor.objectId || `index:${anchor.objectIndex}`;
    if (!recordsByKey.has(key)) {
      recordsByKey.set(key, { anchor: { ...anchor }, occurrences: [] });
    }
    recordsByKey.get(key).occurrences.push(occurrence);
  }
  summarySourceIndexCache = {
    navigation,
    records: [...recordsByKey.values()].sort((left, right) => sourceAnchorLabel(left.anchor).localeCompare(sourceAnchorLabel(right.anchor))),
  };
  return summarySourceIndexCache.records;
}

function summaryMetricContributingSources(metric = {}) {
  const sections = summaryMetricSections(metric, metric.categoryId || "");
  return summarySourceRecords()
    .filter((record) => summarySourceContributes(metric, record))
    .map((record) => ({
      ...record,
      navigation: sourceRecordNavigation(record, sections),
    }));
}

function summarySourceContributes(metric, record) {
  const id = String(metric.id || "").toLowerCase();
  const type = String(record.anchor?.objectType || "").toLowerCase();
  const isGeometry = type.includes("surface") || type === "zone" || type === "space" || type.includes("window") || type.includes("door");
  const isLoad = type === "people" || type === "lights" || type.includes("equipment");
  const isSchedule = type.startsWith("schedule:");
  const isHVAC = /(^|:)(airloop|plantloop|condenserloop|zonehvac|spacehvac|coil|fan|pump|boiler|chiller|thermostat|zonecontrol)/.test(type);

  if (id === "object_count" || id === "object_type_count") {
    return true;
  }
  if (id === "zone_count" || id.includes("conditioned_zone")) {
    return type === "zone";
  }
  if (id === "space_count") {
    return type === "space";
  }
  if (id.includes("construction")) {
    return type.startsWith("construction");
  }
  if (id.includes("material")) {
    return type.includes("material");
  }
  if (id.includes("schedule") || id.includes("operating_hours") || id.includes("profile_coverage")) {
    return isSchedule;
  }
  if (id.includes("lighting") || id.includes("equipment") || id.includes("people") || id.includes("internal_load")) {
    return isLoad;
  }
  if (id.includes("hvac") || id.includes("thermostat")) {
    return isHVAC;
  }
  if (id.includes("output")) {
    return type.startsWith("output:");
  }
  if (id.includes("diagnostic")) {
    return record.occurrences.some((occurrence) => String(occurrence.contextKind || "") === "diagnostic_occurrence");
  }
  if (metric.categoryId === "geometry_areas" || metric.categoryId === "envelope_fenestration") {
    return isGeometry;
  }
  return false;
}

function sourceRecordNavigation(record, sections = []) {
  const navigation = semanticNavigationIndex();
  const candidates = (record.occurrences || [])
    .filter((occurrence) => !String(occurrence.path || "").includes("/diagnostics/"))
    .map((occurrence, order) => {
      const entity = navigationEntity(occurrence.entityId, navigation);
      const sectionRank = preferredSectionRank(entity, occurrence, sections);
      return {
        entity,
        occurrence,
        order,
        score:
          Number(sectionRank >= 0) * (10_000_000 - sectionRank * 1_000_000) +
          Number((occurrence.lineIndexes || []).length > 0) * 100_000 +
          Number(occurrence.occurrenceId === state.semanticCurrentOccurrenceId) * 10_000 +
          Number(String(occurrence.contextKind || "") !== "source_only") * 1_000,
      };
    })
    .filter((candidate) => candidate.entity)
    .sort((left, right) => right.score - left.score || left.order - right.order);
  const selected = candidates[0];
  return selected
    ? navigationMetadata(selected.entity, selected.occurrence, record.anchor)
    : { entity: null, occurrence: null, sourceAnchor: { ...record.anchor } };
}

function renderSummarySourceChooser(sources) {
  if (!sources.length) {
    return "";
  }
  const rendered = sources.slice(0, SUMMARY_SOURCE_RENDER_LIMIT);
  return `
    <details class="summary-source-objects">
      <summary title="Contributing source objects">Source objects <span class="badge">${escapeHTML(sources.length)}</span></summary>
      <div class="summary-source-object-list" role="listbox" aria-label="Contributing source objects">
        ${rendered.map((source) => `
          <button class="summary-source-object navigable-row" type="button" role="option" ${panelNavigationAttributes(source.navigation)}>
            <strong>${escapeHTML(sourceAnchorLabel(source.anchor))}</strong>
            <small>${escapeHTML(source.anchor.objectType || "Source object")}</small>
          </button>`).join("")}
        ${sources.length > rendered.length ? `<small>${escapeHTML(sources.length - rendered.length)} more source objects</small>` : ""}
      </div>
    </details>`;
}

function sourceAnchorLabel(anchor = {}) {
  if (String(anchor.objectName || "").trim()) {
    return String(anchor.objectName);
  }
  if (String(anchor.objectType || "").trim()) {
    return String(anchor.objectType);
  }
  return hasNavigationIndex(anchor.objectIndex) ? `Object #${Number(anchor.objectIndex) + 1}` : "Source object";
}

function diagnosticStableID(item = {}) {
  if (String(item.id || "").trim()) {
    return String(item.id);
  }
  const identity = [item.code, item.objectIndex, item.fieldIndex, item.message].map((value) => String(value ?? "")).join("\u0000");
  let hash = 2166136261;
  for (let index = 0; index < identity.length; index += 1) {
    hash ^= identity.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return `diagnostic:${String(item.code || "issue").toLowerCase().replace(/[^a-z0-9]+/g, "-")}:${(hash >>> 0).toString(16)}`;
}

function diagnosticSemanticNavigation(item, stableID) {
  const navigation = semanticNavigationIndex();
  const diagnostic = navigationSelectionForViewTarget("diagnose", stableID);
  const diagnosticOccurrence = diagnostic.occurrence;
  const diagnosticEntity = diagnostic.entity;
  const exactAnchor = diagnosticOccurrence?.sourceAnchor || diagnosticSourceAnchor(item);
  if (!diagnosticOccurrence || !diagnosticEntity) {
    return sourceNavigationForAnchor(exactAnchor);
  }

  const contextPath = String(diagnosticOccurrence.path || "").replace(/\/diagnostics\/[^/]+$/, "");
  const relatedEntityIDs = diagnosticEntity.relatedEntityIds || [];
  const candidates = relatedEntityIDs
    .flatMap((entityID) => (navigation.byEntityId?.[entityID] || []).map((occurrenceID) => navigationOccurrence(occurrenceID, navigation)))
    .filter((occurrence) => occurrence && String(occurrence.contextKind || "") !== "diagnostic_occurrence")
    .map((occurrence, order) => ({
      occurrence,
      entity: navigationEntity(occurrence.entityId, navigation),
      order,
      score:
        Number(occurrence.path === contextPath) * 10_000_000 +
        Number(sourceAnchorsMatch(occurrence.sourceAnchor, exactAnchor)) * 1_000_000 +
        Number(sharedLineIndex(occurrence.lineIndexes, diagnosticOccurrence.lineIndexes)) * 100_000 +
        diagnosticContextPriority(occurrence.contextKind) * 10_000 +
        Number((occurrence.lineIndexes || []).length > 0) * 1_000,
    }))
    .filter((candidate) => candidate.entity)
    .sort((left, right) => right.score - left.score || left.order - right.order);
  const selected = candidates[0];
  if (selected) {
    return navigationMetadata(selected.entity, selected.occurrence, exactAnchor);
  }
  return sourceNavigationForAnchor(exactAnchor);
}

function sourceNavigationForAnchor(anchor) {
  if (!anchor) {
    return { entity: null, occurrence: null, sourceAnchor: null };
  }
  const navigation = semanticNavigationIndex();
  const occurrenceIDs = anchor.objectId
    ? navigation.byObjectId?.[anchor.objectId] || []
    : navigation.byObjectIndex?.[String(anchor.objectIndex)] || [];
  const record = {
    anchor: { ...anchor },
    occurrences: occurrenceIDs.map((occurrenceID) => navigationOccurrence(occurrenceID, navigation)).filter(Boolean),
  };
  return sourceRecordNavigation(record);
}

function diagnosticSourceAnchor(item = {}) {
  const hasSourceIdentity = String(item.objectType || item.objectName || item.field || "").trim() !== "";
  if (!hasSourceIdentity) {
    return null;
  }
  return {
    objectIndex: Number(item.objectIndex),
    objectType: String(item.objectType || ""),
    objectName: String(item.objectName || ""),
    fieldIndex: String(item.field || "").trim() || Number(item.fieldIndex) > 0 ? Number(item.fieldIndex) : undefined,
    fieldName: String(item.field || ""),
  };
}

function sourceAnchorsMatch(left, right) {
  if (!left || !right) {
    return false;
  }
  if (left.objectId && right.objectId) {
    return left.objectId === right.objectId && (
      !hasNavigationIndex(right.fieldIndex) || Number(left.fieldIndex) === Number(right.fieldIndex)
    );
  }
  return Number(left.objectIndex) === Number(right.objectIndex) && (
    !hasNavigationIndex(right.fieldIndex) || Number(left.fieldIndex) === Number(right.fieldIndex)
  );
}

function sharedLineIndex(left = [], right = []) {
  const rightIndexes = new Set(right || []);
  return (left || []).some((index) => rightIndexes.has(index));
}

function diagnosticContextPriority(contextKind) {
  return {
    zone_service: 7,
    zone_profile: 6,
    zone_geometry: 5,
    zone_output: 4,
    definition: 3,
    source_only: 1,
  }[String(contextKind || "")] || 2;
}

function panelNavigationAttributes({ entity, occurrence, sourceAnchor, panelTargetId = "" } = {}) {
  const attributes = [];
  const add = (name, value) => {
    if (value === undefined || value === null || String(value) === "") {
      return;
    }
    attributes.push(`${name}="${escapeHTML(value)}"`);
  };
  add("data-entity-id", entity?.id || occurrence?.entityId);
  add("data-entity-kind", entity?.kind);
  add("data-occurrence-id", occurrence?.occurrenceId);
  add("data-occurrence-context", occurrence?.contextKind || occurrence?.path);
  add("data-semantic-path", occurrence?.path);
  add("data-source-object-id", sourceAnchor?.objectId);
  add("data-source-object-index", sourceAnchor?.objectIndex);
  add("data-source-object-type", sourceAnchor?.objectType);
  add("data-source-object-name", sourceAnchor?.objectName);
  add("data-source-field-index", sourceAnchor?.fieldIndex);
  add("data-source-field-name", sourceAnchor?.fieldName);
  add("data-panel-target-id", panelTargetId);
  return attributes.join(" ");
}

function hasNavigationIndex(value) {
  return value !== undefined && value !== null && String(value) !== "" && Number.isInteger(Number(value)) && Number(value) >= 0;
}

function renderMetricRow(metric, category = {}) {
  const unit = metric.unit ? escapeHTML(metric.unit) : "";
  const meta = renderMetricMeta(metric);
  const valueClass = isNumericSummaryMetric(metric) ? " summary-value-numeric" : "";
  const navigation = summaryMetricNavigation(metric, category);
  const contributingSources = summaryMetricContributingSources(metric);
  return `
    <div class="summary-row navigable-row" role="row" tabindex="0" data-summary-metric-id="${escapeHTML(metric.id)}" ${panelNavigationAttributes({
      ...navigation,
      panelTargetId: metric.id,
    })}>
      <div class="summary-name" role="cell">
        <strong title="${escapeHTML(metric.name)}">${escapeHTML(metric.name)}</strong>
        ${renderSummarySourceChooser(contributingSources)}
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

function refreshSummaryNavigationStyles() {
  if (elements.summaryCategories) {
    refreshResultPanelSelectionStyles("summary", state.globalSelection, state.globalHover);
  }
  const selectedMetricID = state.globalSelection?.originView === "summary"
    ? String(state.globalSelection.originTargetId || "")
    : "";
  if (selectedMetricID) {
    for (const row of elements.summaryCategories?.querySelectorAll?.("[data-summary-metric-id]") || []) {
      const selected = row.dataset.summaryMetricId === selectedMetricID;
      row.classList.toggle("semantic-selected", selected);
      row.toggleAttribute("data-semantic-selected", selected);
    }
  }
}

function refreshDiagnoseNavigationStyles() {
  if (elements.diagnosticList) {
    refreshResultPanelSelectionStyles("diagnose", state.globalSelection, state.globalHover);
  }
  if (diagnoseSelectedDiagnosticID) {
    for (const item of elements.diagnosticList?.querySelectorAll?.("[data-diagnostic-id]") || []) {
      const selected = item.dataset.diagnosticId === diagnoseSelectedDiagnosticID;
      item.classList.toggle("semantic-selected", selected);
      item.toggleAttribute("data-semantic-selected", selected);
    }
  }
}

function summaryNavigationContext(context = {}) {
  const scrollHost = elements.summaryCategories?.closest?.(".summary-pane");
  const selectedRow = elements.summaryCategories?.querySelector?.("[data-summary-metric-id][data-semantic-selected]");
  return {
    ...context.genericCaptureContext?.(),
    scrollTop: Number(scrollHost?.scrollTop) || 0,
    scrollLeft: Number(scrollHost?.scrollLeft) || 0,
    filter: elements.summaryFilter?.value || "",
    expandedCategoryIDs: [...(elements.summaryCategories?.querySelectorAll?.("[data-summary-category-id][open]") || [])]
      .map((category) => category.dataset.summaryCategoryId)
      .filter(Boolean),
    selectedMetricID: selectedRow?.dataset.summaryMetricId || (
      state.globalSelection?.originView === "summary" ? state.globalSelection.originTargetId || "" : ""
    ),
  };
}

async function restoreSummaryNavigationContext(snapshot = {}, context = {}) {
  if (elements.summaryFilter) {
    elements.summaryFilter.value = String(snapshot.filter || "");
  }
  renderSummary();
  const expanded = new Set(snapshot.expandedCategoryIDs || []);
  if (Array.isArray(snapshot.expandedCategoryIDs)) {
    for (const category of elements.summaryCategories?.querySelectorAll?.("[data-summary-category-id]") || []) {
      category.open = expanded.has(category.dataset.summaryCategoryId);
    }
  }
  const selectedMetricID = String(snapshot.selectedMetricID || "");
  const selected = [...(elements.summaryCategories?.querySelectorAll?.("[data-summary-metric-id]") || [])]
    .find((row) => row.dataset.summaryMetricId === selectedMetricID);
  selected?.classList.add("semantic-selected");
  selected?.toggleAttribute("data-semantic-selected", true);
  const scrollHost = elements.summaryCategories?.closest?.(".summary-pane");
  if (scrollHost) {
    scrollHost.scrollTop = Number(snapshot.scrollTop) || 0;
    scrollHost.scrollLeft = Number(snapshot.scrollLeft) || 0;
  }
  refreshSummaryNavigationStyles();
  return true;
}

export function captureDiagnoseNavigationContext(context = {}) {
  const scrollHost = elements.diagnosticList?.closest?.(".diagnostic-pane");
  const selectedItem = diagnoseSelectedDiagnosticID
    ? [...(elements.diagnosticList?.querySelectorAll?.("[data-diagnostic-id]") || [])]
      .find((item) => item.dataset.diagnosticId === diagnoseSelectedDiagnosticID)
    : elements.diagnosticList?.querySelector?.("[data-diagnostic-id][data-semantic-selected]");
  const selectedDiagnosticID = selectedItem?.dataset.diagnosticId || diagnoseSelectedDiagnosticID || (
    state.globalSelection?.originView === "diagnose" ? state.globalSelection.originTargetId || "" : ""
  );
  const selectedDiagnostic = (state.report?.diagnostics || [])
    .find((item) => diagnosticStableID(item) === selectedDiagnosticID);
  const sourceAnchor = selectedItem ? sourceAnchorFromPanelElement(selectedItem) : state.globalSelection?.sourceAnchor || null;
  return {
    ...context.genericCaptureContext?.(),
    selectedDiagnosticID,
    selectedDiagnosticCode: selectedItem?.dataset.diagnosticCode || selectedDiagnostic?.code || "",
    selectedSemanticEntityID: state.globalSelection?.entityId || "",
    selectedSemanticOccurrenceID: state.globalSelection?.occurrenceId || "",
    sourceAnchor: sourceAnchor ? { ...sourceAnchor } : null,
    filter: elements.diagnosticFilter?.value || "",
    severityFilter: state.diagnosticSeverityFilter,
    sourceFilter: state.diagnosticSourceFilter,
    hiddenDiagnosticCodes: [...state.hiddenDiagnosticCodes],
    temporaryRevealID: diagnoseTemporaryRevealID,
    expandedGroupIDs: [...(elements.diagnosticList?.querySelectorAll?.("[data-diagnostic-group-id][open]") || [])]
      .map((group) => group.dataset.diagnosticGroupId)
      .filter(Boolean),
    expandedDiagnosticIDs: [...(elements.diagnosticList?.querySelectorAll?.("[data-diagnostic-id][open]") || [])]
      .map((item) => item.dataset.diagnosticId)
      .filter(Boolean),
    scrollTop: Number(scrollHost?.scrollTop) || 0,
    scrollLeft: Number(scrollHost?.scrollLeft) || 0,
  };
}

export async function restoreDiagnoseNavigationContext(snapshot = {}, context = {}) {
  const diagnostics = state.report?.diagnostics || [];
  const requestedID = String(snapshot.selectedDiagnosticID || "");
  let matched = requestedID
    ? diagnostics.find((item) => diagnosticStableID(item) === requestedID) || null
    : null;
  if (!matched && snapshot.sourceAnchor) {
    matched = diagnostics.find((item) => {
      if (snapshot.selectedDiagnosticCode && String(item.code || "") !== String(snapshot.selectedDiagnosticCode)) {
        return false;
      }
      const stableID = diagnosticStableID(item);
      const navigation = diagnosticSemanticNavigation(item, stableID);
      return sourceAnchorsMatch(navigation.sourceAnchor || diagnosticSourceAnchor(item), snapshot.sourceAnchor);
    }) || null;
  }
  const resolved = Boolean(requestedID && !matched);
  diagnoseSelectedDiagnosticID = matched ? diagnosticStableID(matched) : "";
  state.diagnosticSeverityFilter = snapshot.severityFilter || "all";
  state.diagnosticSourceFilter = snapshot.sourceFilter || "all";
  state.hiddenDiagnosticCodes = new Set(snapshot.hiddenDiagnosticCodes || []);
  diagnoseTemporaryRevealID = snapshot.temporaryRevealID || "";
  if (elements.diagnosticFilter) {
    elements.diagnosticFilter.value = String(snapshot.filter || "");
  }
  preservingDiagnoseContext = true;
  try {
    renderDiagnostics();
  } finally {
    preservingDiagnoseContext = false;
  }

  const expandedGroups = new Set(snapshot.expandedGroupIDs || []);
  if (Array.isArray(snapshot.expandedGroupIDs)) {
    for (const group of elements.diagnosticList?.querySelectorAll?.("[data-diagnostic-group-id]") || []) {
      group.open = expandedGroups.has(group.dataset.diagnosticGroupId);
    }
  }
  const expandedDiagnostics = new Set(snapshot.expandedDiagnosticIDs || []);
  for (const item of elements.diagnosticList?.querySelectorAll?.("[data-diagnostic-id]") || []) {
    item.open = expandedDiagnostics.has(item.dataset.diagnosticId) || item.dataset.diagnosticId === diagnoseSelectedDiagnosticID;
  }
  const target = diagnoseSelectedDiagnosticID
    ? [...(elements.diagnosticList?.querySelectorAll?.("[data-diagnostic-id]") || [])]
      .find((item) => item.dataset.diagnosticId === diagnoseSelectedDiagnosticID)
    : null;
  target?.classList.add("semantic-selected");
  target?.toggleAttribute("data-semantic-selected", true);
  if (resolved && elements.diagnosticList) {
    const status = document.createElement("div");
    status.className = "diagnostic-resolved-status";
    status.dataset.resolvedDiagnosticId = requestedID;
    status.textContent = `Resolved · ${requestedID}`;
    elements.diagnosticList.prepend(status);
  }
  await nextNavigationFrame();
  const scrollHost = elements.diagnosticList?.closest?.(".diagnostic-pane");
  if (scrollHost) {
    scrollHost.scrollTop = Number(snapshot.scrollTop) || 0;
    scrollHost.scrollLeft = Number(snapshot.scrollLeft) || 0;
  }
  refreshDiagnoseNavigationStyles();
  return {
    restored: Boolean(target),
    resolved,
    selectedDiagnosticID: diagnoseSelectedDiagnosticID,
    selectedSemanticEntityID: snapshot.selectedSemanticEntityID || "",
  };
}

function sourceAnchorFromPanelElement(element) {
  if (!element) {
    return null;
  }
  const objectIndex = optionalPanelIndex(element.dataset.sourceObjectIndex);
  const fieldIndex = optionalPanelIndex(element.dataset.sourceFieldIndex);
  const anchor = {
    objectId: element.dataset.sourceObjectId || "",
    objectIndex,
    objectType: element.dataset.sourceObjectType || "",
    objectName: element.dataset.sourceObjectName || "",
    fieldIndex,
    fieldName: element.dataset.sourceFieldName || "",
  };
  return anchor.objectId || hasNavigationIndex(objectIndex) || anchor.objectType || anchor.objectName || hasNavigationIndex(fieldIndex)
    ? anchor
    : null;
}

function optionalPanelIndex(value) {
  return hasNavigationIndex(value) ? Number(value) : undefined;
}

function panelSelectionTargetID(selection = {}) {
  if (selection.viewTarget?.targetId) {
    return String(selection.viewTarget.targetId);
  }
  if (selection.originView === state.activeResultTab && selection.originTargetId) {
    return String(selection.originTargetId);
  }
  return String(selection.originTargetId || "");
}

function findSummaryNavigationTarget(selection = {}) {
  const targetID = panelSelectionTargetID(selection);
  if (targetID) {
    const category = [...(elements.summaryCategories?.querySelectorAll?.("[data-summary-category-id]") || [])]
      .find((item) => item.dataset.summaryCategoryId === targetID);
    if (category) {
      return category;
    }
  }
  return undefined;
}

function findDiagnoseNavigationTarget(selection = {}) {
  const targetID = panelSelectionTargetID(selection);
  if (targetID) {
    const exact = [...(elements.diagnosticList?.querySelectorAll?.("[data-panel-target-id]") || [])]
      .find((item) => item.dataset.panelTargetId === targetID);
    if (exact) {
      return exact;
    }
    if (targetID === "source-name-conflicts") {
      const duplicate = [...(elements.diagnosticList?.querySelectorAll?.("[data-diagnostic-code]") || [])]
        .find((item) => String(item.dataset.diagnosticCode || "").includes("duplicate"));
      return duplicate || elements.diagnosticList?.querySelector?.("[data-diagnostic-group-id='warnings']") || null;
    }
  }
  return undefined;
}

function revealNavigationTarget(target, options = {}) {
  if (!target) {
    return false;
  }
  let details = target.closest?.("details") || null;
  while (details) {
    details.open = true;
    details = details.parentElement?.closest?.("details") || null;
  }
  target.classList.add("semantic-selected");
  target.toggleAttribute("data-semantic-selected", true);
  if (options.scroll !== false) {
    target.scrollIntoView?.({ block: options.block || "nearest", inline: "nearest", behavior: options.behavior || "auto" });
  }
  if (options.focus !== false) {
    if (!target.matches?.("a[href], button, input, select, textarea, [tabindex]")) {
      target.tabIndex = -1;
    }
    target.focus?.({ preventScroll: true });
  }
  return true;
}

function prepareDiagnosticForReveal(targetID) {
  const item = (state.report?.diagnostics || []).find((diagnostic) => diagnosticStableID(diagnostic) === targetID);
  if (!item) {
    return false;
  }
  const query = (elements.diagnosticFilter?.value || "").trim().toLowerCase();
  const hidden = !diagnosticMatchesQuery(item, query) || !diagnosticMatchesControls(item);
  diagnoseTemporaryRevealID = hidden ? targetID : "";
  diagnoseSelectedDiagnosticID = targetID;
  renderDiagnostics();
  return true;
}

function nextNavigationFrame() {
  if (typeof window === "undefined" || typeof window.requestAnimationFrame !== "function") {
    return Promise.resolve();
  }
  return new Promise((resolve) => window.requestAnimationFrame(resolve));
}

configureResultPanelNavigationHooks("summary", {
  getRoot: () => elements.summaryCategories,
  findTarget: (selection) => findSummaryNavigationTarget(selection),
  selectFromElement(element) {
    if (element?.closest?.(".summary-source-objects > summary")) {
      return null;
    }
    return undefined;
  },
  reveal(selection, options, context) {
    const target = findSummaryNavigationTarget(selection) || context.genericFindTarget(selection);
    const revealed = revealNavigationTarget(target, options);
    refreshSummaryNavigationStyles();
    return revealed;
  },
  captureContext: (context) => summaryNavigationContext(context),
  restoreContext: (snapshot, context) => restoreSummaryNavigationContext(snapshot, context),
  preferredSemanticOccurrence(selection, context) {
    const targetID = panelSelectionTargetID(selection);
    const target = targetID
      ? [...(elements.summaryCategories?.querySelectorAll?.("[data-panel-target-id]") || [])]
        .find((item) => item.dataset.panelTargetId === targetID && item.dataset.occurrenceId)
      : null;
    return target?.dataset.occurrenceId || context.genericPreferredSemanticOccurrence(selection);
  },
});

configureResultPanelNavigationHooks("diagnose", {
  getRoot: () => elements.diagnosticList?.closest?.("#diagnosePane") || elements.diagnosticList,
  findTarget: (selection) => findDiagnoseNavigationTarget(selection),
  selectFromElement(element) {
    if (element?.closest?.("[data-diagnostic-hide-code], [data-diagnostic-filter-kind], [data-diagnostic-clear-hidden], [data-diagnostic-reveal-source]")) {
      return null;
    }
    return undefined;
  },
  reveal(selection, options, context) {
    const targetID = panelSelectionTargetID(selection);
    if (targetID) {
      prepareDiagnosticForReveal(targetID);
    }
    const target = findDiagnoseNavigationTarget(selection) || context.genericFindTarget(selection);
    if (target?.dataset?.diagnosticId) {
      diagnoseSelectedDiagnosticID = target.dataset.diagnosticId;
    }
    const revealed = revealNavigationTarget(target, options);
    refreshDiagnoseNavigationStyles();
    return revealed;
  },
  captureContext: (context) => captureDiagnoseNavigationContext(context),
  async restoreContext(snapshot, context) {
    await restoreDiagnoseNavigationContext(snapshot, context);
    return true;
  },
  preferredSemanticOccurrence(selection, context) {
    const targetID = panelSelectionTargetID(selection);
    const target = targetID
      ? [...(elements.diagnosticList?.querySelectorAll?.("[data-diagnostic-id]") || [])]
        .find((item) => item.dataset.diagnosticId === targetID)
      : null;
    return target?.dataset.occurrenceId || context.genericPreferredSemanticOccurrence(selection);
  },
});

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
