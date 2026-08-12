import { elements, escapeHTML, refreshStatusTitle, state } from "../state.js";
import { renderTopology } from "../topology-loader.js";
import { renderHVAC } from "./hvac-views.js";
import { renderInputViews } from "./input-views.js";
import { renderProfile } from "./profile-views.js";
import { renderSimulation } from "./simulation-views.js";
import { t } from "../i18n.js";
import {
  configureResultPanelNavigationHooks,
  refreshResultPanelSelectionStyles,
} from "../panel-navigation-adapters.js";

let metricsSourceIndexCache = { navigation: null, records: [] };
let metricsTableResizeObserver = null;
let metricsTableLayoutFrame = 0;

export function renderReport(options = {}) {
  const report = state.report;
  if (!report) {
    updateResultTabReadiness();
    renderEmpty();
    return;
  }
  updateResultTabReadiness();

  if (options.scope === "all") {
    renderMetrics(report.metrics);
    renderProfile(report.profile);
    renderHVAC(report.hvac);
    renderSimulation();
    if (state.activeResultTab === "topology") {
      renderTopology(report.geometry);
    } else {
      renderDeferredTopology(report.geometry);
    }
    renderInputViews();
    Object.keys(state.analysisDirty || {}).forEach(markAnalysisRendered);
    return;
  }

  renderMetrics(report.metrics);
  markAnalysisRendered("metrics");
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
      case "simulation":
        renderSimulation();
        markAnalysisRendered("simulation");
        break;
      case "topology":
        if (state.geometryReady) {
          renderTopology(report.geometry);
        } else {
          renderDeferredTopology(report.geometry);
        }
        markAnalysisRendered("topology");
        break;
      case "metrics":
      default:
        renderMetrics(report.metrics);
        markAnalysisRendered("metrics");
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
      elements.profileApplyButton.disabled = true;
      elements.profileOverview.innerHTML = `<div class="empty status-loading">${t("profile.running", {}, "Building profile graphs")}</div>`;
      elements.profileGraph.innerHTML = `<div class="empty status-loading">${t("profile.running", {}, "Building profile graphs")}</div>`;
      return true;
    case "hvac":
      [elements.hvacBackButton, elements.hvacForwardButton, elements.hvacClearFocusButton, elements.hvacZoneServicesButton].forEach((button) => {
        if (button) {
          button.disabled = true;
        }
      });
      elements.hvacSummary.innerHTML = `<div class="empty status-loading">${t("hvac.running", {}, "Resolving HVAC service paths")}</div>`;
      elements.hvacGraph.innerHTML = `<div class="empty">${t("hvac.readySoon", {}, "HVAC graph will appear when this stage is ready.")}</div>`;
      elements.hvacInspectorStats.textContent = t("hvac.pending", {}, "HVAC pending");
      elements.hvacInspector.innerHTML = `<div class="empty">${t("hvac.readySoon", {}, "HVAC graph will appear when this stage is ready.")}</div>`;
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
    const tab = button.dataset.resultTab || "metrics";
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
    return tab === "metrics" ? "ready" : "pending";
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
  elements.metricCategories.innerHTML = `<div class="empty">${t("metrics.empty")}</div>`;
  if (elements.profileOverview) {
    elements.profileSettings.innerHTML = "";
    elements.profileOverview.innerHTML = `<div class="empty">${t("profile.noAnalysis")}</div>`;
    elements.profileGraph.innerHTML = `<div class="empty">${t("profile.noGraph")}</div>`;
    elements.profileApplyButton.disabled = true;
  }
  if (elements.hvacSummary) {
    renderHVAC(null);
  }
  if (elements.simulationRunButton) {
    renderSimulation();
  }
  elements.topologyStats.textContent = t("topology.stats", { zones: 0, surfaces: 0, windows: 0 });
  elements.topology3DCanvasHost.innerHTML = `<div class="empty">${t("topology.noGeometry")}</div>`;
  elements.topologyPlan.innerHTML = "";
  elements.topologyDetails.removeAttribute("aria-labelledby");
  elements.topologyDetails.innerHTML = `<div class="empty">${t("topology.selectObject")}</div>`;
  elements.textObjectView.innerHTML = `<div class="empty">${t("input.formattedEmpty")}</div>`;
  elements.jsonStructuredView.innerHTML = `<div class="empty">${t("input.jsonEmpty")}</div>`;
  elements.fieldTable.innerHTML = `<div class="empty">${t("input.tableEmpty")}</div>`;
  elements.fieldStats.textContent = "0 tables";
}

export function renderDeferredTopology(geometry) {
  if (!state.geometryReady && state.report) {
    elements.topologyStats.textContent = t("topology.pending");
    elements.topology3DCanvasHost.innerHTML = `<div class="empty status-loading">${t("topology.running")}</div>`;
    elements.topologyPlan.innerHTML = "";
    elements.topologyDetails.removeAttribute("aria-labelledby");
    elements.topologyDetails.innerHTML = `<div class="empty">${t("topology.detailsReadySoon")}</div>`;
    return;
  }
  if (!geometry) {
    elements.topologyStats.textContent = t("topology.stats", { zones: 0, surfaces: 0, windows: 0 });
    return;
  }
  elements.topologyStats.textContent = t("topology.stats", {
    zones: geometry.zoneCount || 0,
    surfaces: geometry.surfaceCount || 0,
    windows: geometry.windowCount || 0,
  });
  elements.topology3DCanvasHost.innerHTML = `<div class="empty">${t("topology.openToRender")}</div>`;
  elements.topologyPlan.innerHTML = "";
  elements.topologyDetails.removeAttribute("aria-labelledby");
  elements.topologyDetails.innerHTML = `<div class="empty">${t("topology.openToInspect")}</div>`;
}

export function renderMetrics(metrics = state.report?.metrics) {
  const categories = metrics?.categories || [];

  const categoryHTML = categories
    .map((category) => {
      const metrics = category.metrics || [];
      if (!metrics.length) {
        return "";
      }
      const categoryNavigation = metricsNavigationForCategory(category);
      return `
        <details class="metrics-category" data-metric-category-id="${escapeHTML(category.id)}" ${panelNavigationAttributes({
          ...categoryNavigation,
          panelTargetId: category.id,
          })} open>
          <summary>
            <span>${escapeHTML(category.name)}</span>
          </summary>
          <div class="metrics-table" role="table" aria-label="${escapeHTML(category.name)} metrics">
            <div class="metrics-table-body">
              ${metrics.map((metric) => renderMetricRow(metric, category)).join("")}
            </div>
          </div>
        </details>`;
    })
    .join("");

  elements.metricCategories.innerHTML = categoryHTML || `<div class="empty">${t("metrics.empty")}</div>`;
  bindMetricsTableLayout();
  refreshMetricsNavigationStyles();
}

function bindMetricsTableLayout() {
  metricsTableResizeObserver?.disconnect();
  if (metricsTableLayoutFrame && typeof cancelAnimationFrame === "function") {
    cancelAnimationFrame(metricsTableLayoutFrame);
  }
  metricsTableLayoutFrame = 0;
  const tables = [...(elements.metricCategories?.querySelectorAll?.(".metrics-table") || [])];
  const layout = (table) => {
    const body = table.querySelector(".metrics-table-body");
    if (!body) {
      return;
    }
    const metrics = [...body.querySelectorAll(".metrics-metric")];
    const width = table.getBoundingClientRect().width || table.clientWidth || 0;
    table.dataset.metricWidth = String(Math.round(width));
    const columnCount = Math.max(1, Math.min(4, metrics.length, Math.floor(width / 420) || 1));
    if (Number(table.dataset.metricColumns) === columnCount) {
      return;
    }
    table.dataset.metricColumns = String(columnCount);
    table.style.setProperty("--metrics-column-count", String(columnCount));
    const rowsPerColumn = Math.ceil(metrics.length / columnCount);
    const columns = Array.from({ length: columnCount }, () => {
      const column = document.createElement("div");
      column.className = "metrics-table-column";
      return column;
    });
    metrics.forEach((metric, index) => {
      columns[Math.min(columnCount - 1, Math.floor(index / rowsPerColumn))].append(metric);
    });
    body.replaceChildren(...columns);
  };
  tables.forEach(layout);
  if (typeof ResizeObserver === "function") {
    metricsTableResizeObserver = new ResizeObserver((entries) => {
      const changed = entries.some((entry) => Math.round(entry.contentRect.width) !== Number(entry.target.dataset.metricWidth));
      if (!changed || metricsTableLayoutFrame) {
        return;
      }
      metricsTableLayoutFrame = requestAnimationFrame(() => {
        metricsTableLayoutFrame = 0;
        entries.forEach((entry) => layout(entry.target));
      });
    });
    tables.forEach((table) => metricsTableResizeObserver.observe(table));
  } else {
    metricsTableResizeObserver = null;
  }
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

function metricsNavigationForCategory(category = {}) {
  return navigationSelectionForViewTarget("metrics", category.id || "", {
    sections: metricsCategorySections(category.id),
  });
}

function metricNavigation(metric = {}, category = {}) {
  const categoryID = category.id || metric.categoryId || "model_inventory";
  return navigationSelectionForViewTarget("metrics", categoryID, {
    sections: metricSections(metric, categoryID),
  });
}

function metricsCategorySections(categoryID) {
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

function metricSections(metric = {}, categoryID = metric.categoryId || "") {
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
  return metricsCategorySections(categoryID);
}

function metricsSourceRecords() {
  const navigation = semanticNavigationIndex();
  if (metricsSourceIndexCache.navigation === navigation) {
    return metricsSourceIndexCache.records;
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
  metricsSourceIndexCache = {
    navigation,
    records: [...recordsByKey.values()].sort((left, right) => sourceAnchorLabel(left.anchor).localeCompare(sourceAnchorLabel(right.anchor))),
  };
  return metricsSourceIndexCache.records;
}

function metricContributingSources(metric = {}) {
  const sections = metricSections(metric, metric.categoryId || "");
  return metricsSourceRecords()
    .filter((record) => metricSourceContributes(metric, record))
    .map((record) => ({
      ...record,
      navigation: sourceRecordNavigation(record, sections),
    }));
}

function metricSourceContributes(metric, record) {
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

function renderMetricsSourceChooser(sources, metric = {}) {
  if (!sources.length) {
    return `<div class="metrics-source-empty">No source object information</div>`;
  }
  return `
    <div class="metrics-source-objects">
      <strong class="metrics-source-title">Source objects</strong>
      <div class="metrics-source-object-list" role="listbox" aria-label="Contributing source objects">
        ${sources.map((source, index) => `
          <button class="metrics-source-object navigable-row" type="button" role="option" ${panelNavigationAttributes({
            ...source.navigation,
            panelTargetId: metricSourcePanelTargetID(metric, source, index),
          })}>
            <strong title="${escapeHTML(sourceAnchorLabel(source.anchor))}">${escapeHTML(sourceAnchorLabel(source.anchor))}</strong>
            <small>${escapeHTML(source.anchor.objectType || "Source object")}</small>
          </button>`).join("")}
      </div>
    </div>`;
}

function metricSourcePanelTargetID(metric = {}, source = {}, index = 0) {
  const metricID = String(metric.id || metric.categoryId || "metric");
  const anchor = source.anchor || {};
  const sourceID = String(anchor.objectId || (hasNavigationIndex(anchor.objectIndex) ? `index:${anchor.objectIndex}` : index));
  return `metrics-source:${metricID}:${sourceID}`;
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
  const valueClass = isNumericMetric(metric) ? " metrics-value-numeric" : "";
  const navigation = metricNavigation(metric, category);
  const contributingSources = metricContributingSources(metric);
  return `
    <details class="metrics-metric" role="row">
      <summary class="metrics-row navigable-row" data-metric-id="${escapeHTML(metric.id)}" ${panelNavigationAttributes({
        ...navigation,
        panelTargetId: metric.id,
      })}>
        <div class="metrics-row-grid">
          <div class="metrics-name" role="cell">
            <strong title="${escapeHTML(metric.name)}">${escapeHTML(metric.name)}</strong>
          </div>
          <div class="metrics-value${valueClass}" role="cell" title="${escapeHTML(String(metric.displayValue ?? "—"))}">
            ${renderMetricDisplayValue(metric)}
          </div>
          <span class="metrics-unit" role="cell" title="${unit}">${unit}</span>
          ${meta}
          ${renderMetricStatus(metric)}
        </div>
      </summary>
      <div class="metrics-source-drawer">
        ${renderMetricsSourceChooser(contributingSources, metric)}
      </div>
    </details>`;
}

function isNumericMetric(metric) {
  return (typeof metric.value === "number" && Number.isFinite(metric.value)) || Boolean(metric.unit);
}

function renderMetricDisplayValue(metric) {
  const displayValue = String(metric.displayValue ?? "—");
  if (!isNumericMetric(metric)) {
    return `<strong>${escapeHTML(displayValue)}</strong>`;
  }
  return `<strong class="metrics-number">${escapeHTML(displayValue)}</strong>`;
}

function renderMetricMeta(metric) {
  const badges = metricNoteBadges(metric);
  if (!badges.length) {
    return `<div class="metrics-meta" role="cell"></div>`;
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
  return `<div class="metrics-meta" role="cell">${tags}</div>`;
}

function metricNoteBadges(metric) {
  const evidence = String(metric.evidence || "").trim();
  const rawBadges = new Set((metric.badges || []).map((badge) => String(badge || "").toLowerCase()));
  const source = String(metric.source || "").toLowerCase();
  const confidence = String(metric.confidence || "").toLowerCase();
  const badges = [];

  if (rawBadges.has("inferred") || confidence === "inferred" || source.includes("inference") || source.includes("semantic_evidence")) {
    badges.push(metricNoteBadge("Inferred", "I", evidence ? `Inferred value. ${evidence}` : "Inferred value."));
  }
  if (rawBadges.has("orientation")) {
    badges.push(metricNoteBadge("Orientation", "O", evidence ? `Orientation-dependent value. ${evidence}` : "Orientation-dependent value."));
  }
  if (rawBadges.has("base-surface")) {
    badges.push(metricNoteBadge("Base surface", "B", evidence ? `Depends on base-surface resolution. ${evidence}` : "Depends on base-surface resolution."));
  }
  if (rawBadges.has("readiness")) {
    badges.push(metricNoteBadge("Readiness", "R", evidence ? `Readiness check. ${evidence}` : "Readiness check."));
  }
  if (rawBadges.has("diagnostic")) {
    badges.push(metricNoteBadge("Diagnostic", "D", evidence ? `Diagnostic summary. ${evidence}` : "Diagnostic summary."));
  }
  return badges;
}

function metricNoteBadge(label, abbr, title) {
  return { label, abbr, title };
}

function renderMetricStatus(metric) {
  const status = metric.status;
  const title = metricStatusTitle(metric);
  switch (status) {
    case "ok":
      return `<span class="metrics-status metrics-status-ok" role="cell" aria-label="OK"></span>`;
    case "partial":
      return `
        <span class="metrics-status metrics-status-partial" role="cell" title="${escapeHTML(title)}" aria-label="${escapeHTML(title)}">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="M12 3 22 20H2z"></path>
            <path d="M12 9v5"></path>
            <path d="M12 17h.01"></path>
          </svg>
        </span>`;
    default:
      return `
        <span class="metrics-status metrics-status-missing" role="cell" title="${escapeHTML(title)}" aria-label="${escapeHTML(title)}">
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

function refreshMetricsNavigationStyles() {
  if (elements.metricCategories) {
    refreshResultPanelSelectionStyles("metrics", state.globalSelection, state.globalHover);
  }
}

function metricsNavigationContext(context = {}) {
  const scrollHost = elements.metricCategories?.closest?.(".metrics-pane");
  const selectedRow = elements.metricCategories?.querySelector?.("[data-metric-id][data-semantic-selected]");
  return {
    ...context.genericCaptureContext?.(),
    scrollTop: Number(scrollHost?.scrollTop) || 0,
    scrollLeft: Number(scrollHost?.scrollLeft) || 0,
    expandedCategoryIDs: [...(elements.metricCategories?.querySelectorAll?.("[data-metric-category-id][open]") || [])]
      .map((category) => category.dataset.metricCategoryId)
      .filter(Boolean),
    selectedMetricID: selectedRow?.dataset.metricId || (
      state.globalSelection?.originView === "metrics" ? state.globalSelection.originTargetId || "" : ""
    ),
  };
}

async function restoreMetricsNavigationContext(snapshot = {}, context = {}) {
  renderMetrics();
  const expanded = new Set(snapshot.expandedCategoryIDs || []);
  if (Array.isArray(snapshot.expandedCategoryIDs)) {
    for (const category of elements.metricCategories?.querySelectorAll?.("[data-metric-category-id]") || []) {
      category.open = expanded.has(category.dataset.metricCategoryId);
    }
  }
  const scrollHost = elements.metricCategories?.closest?.(".metrics-pane");
  if (scrollHost) {
    scrollHost.scrollTop = Number(snapshot.scrollTop) || 0;
    scrollHost.scrollLeft = Number(snapshot.scrollLeft) || 0;
  }
  refreshMetricsNavigationStyles();
  return true;
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

function findMetricsNavigationTarget(selection = {}) {
  const targetID = panelSelectionTargetID(selection);
  if (targetID) {
    const category = [...(elements.metricCategories?.querySelectorAll?.("[data-metric-category-id]") || [])]
      .find((item) => item.dataset.metricCategoryId === targetID);
    if (category) {
      return category;
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

function nextNavigationFrame() {
  if (typeof window === "undefined" || typeof window.requestAnimationFrame !== "function") {
    return Promise.resolve();
  }
  return new Promise((resolve) => window.requestAnimationFrame(resolve));
}

configureResultPanelNavigationHooks("metrics", {
  getRoot: () => elements.metricCategories,
  findTarget: (selection) => findMetricsNavigationTarget(selection),
  selectFromElement(element) {
    if (element?.closest?.(".metrics-source-objects > .metrics-source-object")) {
      return null;
    }
    return undefined;
  },
  reveal(selection, options, context) {
    const target = findMetricsNavigationTarget(selection) || context.genericFindTarget(selection);
    const revealed = revealNavigationTarget(target, options);
    refreshMetricsNavigationStyles();
    return revealed;
  },
  captureContext: (context) => metricsNavigationContext(context),
  restoreContext: (snapshot, context) => restoreMetricsNavigationContext(snapshot, context),
  preferredSemanticOccurrence(selection, context) {
    const targetID = panelSelectionTargetID(selection);
    const target = targetID
      ? [...(elements.metricCategories?.querySelectorAll?.("[data-panel-target-id]") || [])]
        .find((item) => item.dataset.panelTargetId === targetID && item.dataset.occurrenceId)
      : null;
    return target?.dataset.occurrenceId || context.genericPreferredSemanticOccurrence(selection);
  },
});
