import { loadAndApplyAppSettings } from "./settings-client.js";
import { renderAppInfo } from "./app-info.js";
import { t } from "./i18n.js";
import { initializeMultiSimulationTool } from "./batch/batch-simulation.js";
import { parseMetricNumber, metricUnit } from "./batch/batch-metrics-utils.js";

loadAndApplyAppSettings();
renderAppInfo();

const BATCH_METRICS_TABLE_RENDER_LIMIT = 500;
const MISSING_VALUE = "\u2014";
const METRICS_GROUP_TOKENS = {
  inventory: ["inventory", "model"],
  geometry: ["geometry", "area", "zone", "surface", "wwr", "fenestration"],
  envelope: ["envelope", "construction", "material", "window"],
  loads: ["load", "lighting", "equipment", "people", "internal"],
  schedules: ["schedule"],
  hvac: ["hvac", "air loop", "plant loop", "node", "coil"],
};

const state = {
  activeTool: "batch-metrics",
  result: null,
  activeRunID: "",
  orientation: "metrics",
  metricGroup: "all",
  statusFilter: "all",
  deltaSort: "table",
  deltaBaselineIndex: null,
  deltaCompareIndex: null,
  includeFullTopology: false,
  running: false,
  progressFiles: new Map(),
  progressListenerRegistered: false,
  diagnose: {
    text: "",
    path: "",
    filename: "",
    diagnostics: [],
    scan: null,
    preview: null,
    selectedRuleIDs: new Set(),
    excludedCandidateKeys: new Set(),
    candidateFilter: "",
    busy: false,
  },
  simulationEnvironment: null,
  multiSimulation: {
    selectedPaths: [],
    rootDirectory: "",
    result: null,
    running: false,
    activeRunID: "",
    selectedRows: new Set(),
    metric: "",
    sort: "filename",
    compareBaselineId: "",
    compareTargetId: "",
  },
};

let multiSimulationTool = null;

const elements = {
  toolNavButtons: document.querySelectorAll("[data-tools-tab]"),
  toolPanels: document.querySelectorAll("[data-tools-panel]"),
  selectButton: document.querySelector("#batchMetricsSelect"),
  exportButton: document.querySelector("#batchMetricsExport"),
  exportJSONButton: document.querySelector("#batchMetricsExportJSON"),
  exportXLSXButton: document.querySelector("#batchMetricsExportXLSX"),
  stats: document.querySelector("#batchMetricsStats"),
  status: document.querySelector("#batchMetricsStatus"),
  percent: document.querySelector("#batchMetricsPercent"),
  progressBar: document.querySelector("#batchMetricsProgressBar"),
  fileList: document.querySelector("#batchMetricsFiles"),
  table: document.querySelector("#batchMetricsTable"),
  orientationButtons: document.querySelectorAll("[data-metrics-orientation]"),
  metricGroup: document.querySelector("#batchMetricsMetricGroup"),
  includeFullTopology: document.querySelector("#batchMetricsIncludeTopology"),
  statusFilter: document.querySelector("#batchMetricsStatusFilter"),
  deltaSort: document.querySelector("#batchMetricsDeltaSort"),
  delta: document.querySelector("#batchMetricsDelta"),
  diagnoseSelectInput: document.querySelector("#diagnoseSelectInput"),
  diagnoseBrowserFile: document.querySelector("#diagnoseBrowserFile"),
  diagnoseRefresh: document.querySelector("#diagnoseRefresh"),
  diagnosePreview: document.querySelector("#diagnosePreview"),
  diagnoseApply: document.querySelector("#diagnoseApply"),
  diagnoseSaveAs: document.querySelector("#diagnoseSaveAs"),
  diagnoseFilename: document.querySelector("#diagnoseFilename"),
  diagnoseStatus: document.querySelector("#diagnoseStatus"),
  diagnoseCandidateFilter: document.querySelector("#diagnoseCandidateFilter"),
  diagnoseCandidateStats: document.querySelector("#diagnoseCandidateStats"),
  diagnoseRules: document.querySelector("#diagnoseRules"),
  diagnoseCandidates: document.querySelector("#diagnoseCandidates"),
  diagnosePreviewPanel: document.querySelector("#diagnosePreviewPanel"),
  diagnoseList: document.querySelector("#diagnoseList"),
  batchPurposeInputs: document.querySelectorAll("[data-batch-purpose]"),
  multiSimulationSelectFiles: document.querySelector("#multiSimulationSelectFiles"),
  multiSimulationSelectFolder: document.querySelector("#multiSimulationSelectFolder"),
  multiSimulationRun: document.querySelector("#multiSimulationRun"),
  multiSimulationExport: document.querySelector("#multiSimulationExport"),
  multiSimulationExportXLSX: document.querySelector("#multiSimulationExportXLSX"),
  multiSimulationExportJSON: document.querySelector("#multiSimulationExportJSON"),
  multiSimulationWeather: document.querySelector("#multiSimulationWeather"),
  multiSimulationWeatherMode: document.querySelector("#multiSimulationWeatherMode"),
  multiSimulationWorkers: document.querySelector("#multiSimulationWorkers"),
  multiSimulationRecursive: document.querySelector("#multiSimulationRecursive"),
  multiSimulationStats: document.querySelector("#multiSimulationStats"),
  multiSimulationSort: document.querySelector("#multiSimulationSort"),
  multiSimulationStatus: document.querySelector("#multiSimulationStatus"),
  multiSimulationPercent: document.querySelector("#multiSimulationPercent"),
  multiSimulationProgressBar: document.querySelector("#multiSimulationProgressBar"),
  multiSimulationFiles: document.querySelector("#multiSimulationFiles"),
  multiSimulationMetric: document.querySelector("#multiSimulationMetric"),
  multiSimulationCompareBaseline: document.querySelector("#multiSimulationCompareBaseline"),
  multiSimulationCompareTarget: document.querySelector("#multiSimulationCompareTarget"),
  multiSimulationChart: document.querySelector("#multiSimulationChart"),
  multiSimulationTable: document.querySelector("#multiSimulationTable"),
};

function appAPI() {
  return window.go && window.go.main && window.go.main.App;
}

async function waitForAppAPI(methodName) {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    const api = appAPI();
    if (api && typeof api[methodName] === "function") {
      return api;
    }
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  return null;
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function registerProgressListener() {
  if (state.progressListenerRegistered || !window.runtime) {
    return;
  }
  if (typeof window.runtime.EventsOn === "function") {
    window.runtime.EventsOn("idfAnalyzer:batchMetricsProgress", handleProgress);
    window.runtime.EventsOn("idfAnalyzer:multiSimulationProgress", handleMultiSimulationProgress);
    state.progressListenerRegistered = true;
  } else if (typeof window.runtime.EventsOnMultiple === "function") {
    window.runtime.EventsOnMultiple("idfAnalyzer:batchMetricsProgress", handleProgress, -1);
    window.runtime.EventsOnMultiple("idfAnalyzer:multiSimulationProgress", handleMultiSimulationProgress, -1);
    state.progressListenerRegistered = true;
  }
}

async function waitForProgressRuntime() {
  for (let attempt = 0; attempt < 40 && !state.progressListenerRegistered; attempt += 1) {
    registerProgressListener();
    if (state.progressListenerRegistered) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
}

function handleProgress(payload) {
  const progress = Array.isArray(payload) ? payload[0] : payload;
  if (!progress || progress.runId !== state.activeRunID) {
    return;
  }
  if (progress.file) {
    state.progressFiles.set(progress.file.index, progress.file);
    renderFileList([...state.progressFiles.values()]);
  }
  updateProgress(progress.completed || 0, progress.total || 0, progress.succeeded || 0, progress.failed || 0);
}

function handleMultiSimulationProgress(payload) {
  multiSimulationTool?.handleProgress(payload);
}

function updateProgress(completed, total, succeeded = 0, failed = 0) {
  const percent = total > 0 ? Math.round((completed / total) * 100) : 0;
  elements.progressBar.style.width = `${percent}%`;
  elements.percent.textContent = `${percent}%`;
  if (total > 0) {
    elements.status.textContent = t("tools.analyzedProgress", { completed, total, ok: succeeded, failed });
  }
}

function setRunning(running) {
  state.running = running;
  elements.selectButton.disabled = running;
  elements.exportButton.disabled = running || !state.result;
  if (elements.exportJSONButton) elements.exportJSONButton.disabled = running || !state.result;
  if (elements.exportXLSXButton) {
    elements.exportXLSXButton.disabled = running || !state.result;
  }
}

async function runBatchMetrics() {
  state.result = null;
  state.progressFiles.clear();
  state.activeRunID = `batch-metrics-${Date.now()}-${Math.random().toString(36).slice(2)}`;
  setRunning(true);
  elements.stats.textContent = t("tools.waitingSelection");
  elements.status.textContent = t("status.openDialog");
  elements.table.innerHTML = `<div class="empty">${t("status.analysisWillStart")}</div>`;
  elements.fileList.innerHTML = "";
  updateProgress(0, 0);
  waitForProgressRuntime();

  try {
    const result = await analyzeBatchMetrics(state.activeRunID);
    if (result?.canceled) {
      elements.stats.textContent = t("tools.noFilesSelected");
      elements.status.textContent = t("status.fileSelectionCanceled");
      elements.table.innerHTML = `<div class="empty">${t("tools.selectFilesHelp")}</div>`;
      return;
    }
    state.result = result;
    updateProgress(result.completed || 0, result.total || 0, result.succeeded || 0, result.failed || 0);
    renderResult();
  } catch (error) {
    elements.status.textContent = error?.message || String(error);
    elements.stats.textContent = t("tools.analysisFailed");
    elements.table.innerHTML = `<div class="empty">${escapeHTML(error?.message || String(error))}</div>`;
  } finally {
    setRunning(false);
  }
}

async function analyzeBatchMetrics(runID) {
  const request = { runId: runID, areaBasis: "effective", includeFullTopology: state.includeFullTopology };
  let responseError = "";
  try {
    for (const url of ["/api/batch-metrics", "/api/multi-idf-summary"]) {
      const response = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(request),
      });
      if (response.ok) {
        return response.json();
      }
      responseError = await response.text();
    }
  } catch (error) {
    responseError = error?.message || String(error);
  }

  const api = await waitForAppAPI("AnalyzeBatchMetrics");
  if (api) {
    return api.AnalyzeBatchMetrics(request);
  }

  throw new Error(responseError || t("tools.desktopOnly"));
}

function renderResult() {
  const result = state.result;
  if (!result) {
    return;
  }
  const total = result.total || 0;
  const succeeded = result.succeeded || 0;
  const failed = result.failed || 0;
  const workers = result.concurrency || 0;
  elements.stats.textContent = t("count.filesMetrics", { total, ok: succeeded, failed, workers }, `${total} files, ${succeeded} ok, ${failed} failed, ${workers} workers`);
  renderFileList(result.files || []);
  renderTable();
  elements.exportButton.disabled = state.running || !result.metrics?.length;
  if (elements.exportJSONButton) elements.exportJSONButton.disabled = state.running || !result.metrics?.length;
  if (elements.exportXLSXButton) {
    elements.exportXLSXButton.disabled = state.running || !result.metrics?.length;
  }
}

function renderFileList(files) {
  if (!files.length) {
    elements.fileList.innerHTML = "";
    return;
  }
  elements.fileList.innerHTML = files
    .slice()
    .sort((a, b) => (a.index || 0) - (b.index || 0))
    .map((file) => {
      const status = file.status === "ok" ? "ok" : "error";
      const detail = status === "ok" ? (file.filename && file.filename !== file.label ? file.filename : t("tools.analyzed")) : file.error || t("tools.failed");
      return `
        <div class="tool-file-item ${status}">
          <strong>${escapeHTML(file.label || file.filename || t("common.inputFile"))}</strong>
          <span>${escapeHTML(detail)}</span>
        </div>`;
    })
    .join("");
}

function renderTable() {
  const result = state.result;
  const metrics = filteredMetricsItems(result?.metrics || [], result?.files || []);
  const files = result?.files || [];
  if (!metrics.length || !files.length) {
    elements.table.innerHTML = `<div class="empty">${t("tools.noMetricsData", {}, "No metrics data available.")}</div>`;
    renderDeltaDrawer(metrics, files);
    return;
  }
  elements.table.innerHTML = state.orientation === "files" ? renderFilesAsRows(metrics, files) : renderMetricsAsRows(metrics, files);
  bindDeltaColumnButtons();
  renderDeltaDrawer(metrics, files);
}

function renderMetricsAsRows(metrics, files) {
  const renderedMetrics = metrics.slice(0, BATCH_METRICS_TABLE_RENDER_LIMIT);
  return `
    ${renderBatchHiddenRowsNotice(metrics.length - renderedMetrics.length, "metrics")}
    <table class="tool-table">
      <thead>
        <tr>
          <th class="tool-sticky-col">${t("common.name")}</th>
          ${files.map((file) => `<th>${renderFileLabel(file, { selectable: true })}</th>`).join("")}
        </tr>
      </thead>
      <tbody>
        ${renderedMetrics
          .map((metric) => `
            <tr>
              <th class="tool-sticky-col">
                <strong>${escapeHTML(metric.csvName || metric.id)}</strong>
                <span>${escapeHTML(metric.category || "")}</span>
              </th>
              ${files.map((file) => renderValueCell(file, metric.id)).join("")}
            </tr>`)
          .join("")}
      </tbody>
    </table>`;
}

function renderFilesAsRows(metrics, files) {
  const renderedFiles = files.slice(0, BATCH_METRICS_TABLE_RENDER_LIMIT);
  return `
    ${renderBatchHiddenRowsNotice(files.length - renderedFiles.length, "files")}
    <table class="tool-table">
      <thead>
        <tr>
          <th class="tool-sticky-col">${t("common.building")}</th>
          ${metrics.map((metric) => `<th>${escapeHTML(metric.csvName || metric.id)}</th>`).join("")}
        </tr>
      </thead>
      <tbody>
        ${renderedFiles
          .map((file) => `
            <tr>
              <th class="tool-sticky-col">
                ${renderFileLabel(file, { selectable: true })}
                ${file.status === "ok" ? "" : `<span>${escapeHTML(file.error || t("tools.failed"))}</span>`}
              </th>
              ${metrics.map((metric) => renderValueCell(file, metric.id)).join("")}
            </tr>`)
          .join("")}
      </tbody>
    </table>`;
}

function renderFileLabel(file, { selectable = false } = {}) {
  const label = file.label || file.filename || t("common.inputFile");
  const detail = file.filename && file.filename !== label ? file.filename : "";
  const content = `<strong>${escapeHTML(label)}</strong>${detail ? `<span>${escapeHTML(detail)}</span>` : ""}`;
  if (!selectable) {
    return content;
  }
  const role = metricsDeltaRole(file.index);
  return `<button class="batch-column-button ${escapeHTML(role)}" data-batch-metrics-column="${escapeHTML(file.index)}" type="button">${content}</button>`;
}

function renderValueCell(file, metricID) {
  if (file.status !== "ok") {
    return `<td class="tool-value error">${MISSING_VALUE}</td>`;
  }
  const value = file.metricValues?.[metricID];
  const status = value?.status || "missing";
  const coverage = value?.hasCoverage ? ` \u00b7 U coverage ${formatNumber(Number(value.coverage || 0) * 100)}%` : "";
  return `<td class="tool-value ${escapeHTML(status)}" title="${escapeHTML(`${status}${coverage}`)}">${escapeHTML(metricDisplayValue(value))}${coverage ? `<span>${escapeHTML(coverage.slice(3))}</span>` : ""}</td>`;
}

function renderBatchHiddenRowsNotice(hiddenCount, label) {
  return hiddenCount > 0
    ? `<div class="empty compact">${escapeHTML(`${hiddenCount} additional ${label} hidden. Narrow filters to render them.`)}</div>`
    : "";
}

function metricValueForCSV(file, metricID) {
  if (file.status !== "ok") {
    return MISSING_VALUE;
  }
  return metricDisplayValue(file.metricValues?.[metricID]);
}

function metricDisplayValue(value) {
  return String(value?.displayValue ?? "").trim() || MISSING_VALUE;
}

function filteredMetricsItems(metrics, files) {
  return metrics.filter((metric) => {
    if (!metricsItemMatchesGroup(metric, files)) {
      return false;
    }
    if (state.statusFilter === "all") {
      return true;
    }
    return files.some((file) => metricsValueStatus(file, metric.id) === state.statusFilter);
  });
}

function metricsItemMatchesGroup(metric, files) {
  const group = state.metricGroup || "all";
  if (group === "all") {
    return true;
  }
  if (group === "reliable") {
    return files.every((file) => file.status !== "ok" || metricsValueStatus(file, metric.id) === "ok");
  }
  const category = String(metric.category || "").toLowerCase();
  const id = String(metric.id || "").toLowerCase();
  const name = String(metric.name || metric.csvName || "").toLowerCase();
  const haystack = `${category} ${id} ${name}`;
  const tokens = METRICS_GROUP_TOKENS[group];
  return tokens ? tokens.some((token) => haystack.includes(token)) : haystack.includes(group);
}

function metricsValueStatus(file, metricID) {
  if (file.status !== "ok") {
    return "error";
  }
  return file.metricValues?.[metricID]?.status || "missing";
}

function bindDeltaColumnButtons() {
  elements.table.querySelectorAll("[data-batch-metrics-column]").forEach((button) => {
    button.addEventListener("click", () => {
      const index = Number(button.dataset.batchMetricsColumn);
      selectDeltaColumn(index);
    });
  });
}

function selectDeltaColumn(index) {
  if (!Number.isFinite(index)) {
    return;
  }
  if (state.deltaBaselineIndex === index && state.deltaCompareIndex === null) {
    state.deltaBaselineIndex = null;
  } else if (state.deltaBaselineIndex === null || state.deltaBaselineIndex === index) {
    state.deltaBaselineIndex = index;
    state.deltaCompareIndex = null;
  } else if (state.deltaCompareIndex === index) {
    state.deltaCompareIndex = null;
  } else {
    state.deltaCompareIndex = index;
  }
  renderTable();
}

function metricsDeltaRole(index) {
  if (state.deltaBaselineIndex === index) {
    return "baseline";
  }
  if (state.deltaCompareIndex === index) {
    return "compare";
  }
  return "";
}

function renderDeltaDrawer(metrics, files) {
  if (!elements.delta) {
    return;
  }
  const baseline = files.find((file) => file.index === state.deltaBaselineIndex);
  const compare = files.find((file) => file.index === state.deltaCompareIndex);
  if (!baseline || !compare) {
    elements.delta.innerHTML = `<div class="empty">${escapeHTML(t("batch.deltaHelp", {}, "Click two file columns to compare baseline and target deltas."))}</div>`;
    return;
  }
  const rows = sortMetricsDeltaRows(metrics.map((metric) => metricsDeltaRow(metric, baseline, compare)));
  elements.delta.innerHTML = `
    <div class="batch-delta-head">
      <div>
        <strong>${escapeHTML(t("batch.selectedCompare", {}, "Selected compare"))}</strong>
        <span>A: ${escapeHTML(baseline.label || baseline.filename)} / B: ${escapeHTML(compare.label || compare.filename)}</span>
      </div>
      <button id="batchMetricsClearDelta" type="button">${escapeHTML(t("action.close"))}</button>
    </div>
    <div class="tool-table-wrap">
      <table class="tool-table">
        <thead>
          <tr>
            <th class="tool-sticky-col">${escapeHTML(t("common.metric"))}</th>
            <th>A</th>
            <th>B</th>
            <th>${escapeHTML(t("batch.delta", {}, "Delta"))}</th>
            <th>%</th>
          </tr>
        </thead>
        <tbody>
          ${rows.map(renderMetricsDeltaRow).join("")}
        </tbody>
      </table>
    </div>`;
  document.querySelector("#batchMetricsClearDelta")?.addEventListener("click", () => {
    state.deltaBaselineIndex = null;
    state.deltaCompareIndex = null;
    renderTable();
  });
}

function metricsDeltaRow(metric, baseline, compare) {
  const a = baseline.metricValues?.[metric.id];
  const b = compare.metricValues?.[metric.id];
  const aNumber = parseMetricNumber(a?.displayValue);
  const bNumber = parseMetricNumber(b?.displayValue);
  const aStatus = metricsValueStatus(baseline, metric.id);
  const bStatus = metricsValueStatus(compare, metric.id);
  const sameUnit = metricUnit(metric, a?.displayValue) === metricUnit(metric, b?.displayValue);
  const status = [aStatus, bStatus].join(" -> ");
  const notComparable = metricsNotComparableReason(a, b);
  if (notComparable) {
    return {
      metric,
      a: metricDisplayValue(a),
      b: metricDisplayValue(b),
      delta: "not comparable",
      percent: "—",
      deltaValue: null,
      percentValue: null,
      missing: false,
      statusChanged: true,
      status: notComparable,
    };
  }
  if (aNumber.ok && bNumber.ok && sameUnit) {
    const delta = bNumber.value - aNumber.value;
    const percentValue = aNumber.value === 0 ? null : (delta / aNumber.value) * 100;
    return {
      metric,
      a: metricDisplayValue(a),
      b: metricDisplayValue(b),
      delta: formatDelta(delta, metric.unit),
      percent: percentValue === null ? "—" : `${formatNumber(percentValue)}%`,
      deltaValue: delta,
      percentValue,
      missing: aStatus === "missing" || bStatus === "missing",
      statusChanged: aStatus !== bStatus,
      status,
    };
  }
  const unchangedLabel = t("batch.unchanged", {}, "unchanged");
  const changed = String(a?.displayValue ?? "") === String(b?.displayValue ?? "") ? unchangedLabel : t("batch.changed", {}, "changed");
  return {
    metric,
    a: metricDisplayValue(a),
    b: metricDisplayValue(b),
    delta: changed,
    percent: "—",
    deltaValue: null,
    percentValue: null,
    missing: aStatus === "missing" || bStatus === "missing",
    statusChanged: aStatus !== bStatus || changed !== unchangedLabel,
    status,
  };
}

function metricsNotComparableReason(a, b) {
  if (a?.hasCoverage || b?.hasCoverage) {
    if (Boolean(a?.hasCoverage) !== Boolean(b?.hasCoverage) || Math.abs(Number(a?.coverage || 0) - Number(b?.coverage || 0)) > 0.0001) {
      return "not comparable: U-value coverage differs";
    }
  }
  return "";
}

function sortMetricsDeltaRows(rows) {
  const mode = state.deltaSort || "table";
  if (mode === "table") {
    return rows;
  }
  return rows.slice().sort((a, b) => {
    if (mode === "absolute") {
      return sortableAbs(b.deltaValue) - sortableAbs(a.deltaValue);
    }
    if (mode === "percent") {
      return sortableAbs(b.percentValue) - sortableAbs(a.percentValue);
    }
    if (mode === "status") {
      return Number(b.statusChanged) - Number(a.statusChanged);
    }
    if (mode === "missing") {
      return Number(b.missing) - Number(a.missing);
    }
    return 0;
  });
}

function sortableAbs(value) {
  const number = Number(value);
  return Number.isFinite(number) ? Math.abs(number) : -1;
}

function renderMetricsDeltaRow(row) {
  return `
    <tr title="${escapeHTML(row.status)}">
      <th class="tool-sticky-col">
        <strong>${escapeHTML(row.metric.csvName || row.metric.id)}</strong>
        <span>${escapeHTML(row.metric.category || "")}</span>
      </th>
      <td>${escapeHTML(row.a)}</td>
      <td>${escapeHTML(row.b)}</td>
      <td>${escapeHTML(row.delta)}</td>
      <td>${escapeHTML(row.percent)}</td>
    </tr>`;
}

function formatDelta(value, unit) {
  const suffix = unit && unit !== "-" ? ` ${unit}` : "";
  const sign = value > 0 ? "+" : "";
  const label = unit === "%" ? " pt" : suffix;
  return `${sign}${formatNumber(value)}${label}`;
}

function formatNumber(value) {
  if (!Number.isFinite(value)) {
    return "";
  }
  const abs = Math.abs(value);
  const digits = abs >= 100 ? 1 : abs >= 10 ? 2 : 3;
  return Number(value.toFixed(digits)).toLocaleString();
}

async function exportCSV() {
  const result = state.result;
  if (!result) {
    return;
  }
  if (state.metricGroup === "topology") {
    const api = await waitForAppAPI("ExportBatchTopologyCSV");
    if (api) {
      const csvText = await api.ExportBatchTopologyCSV(result);
      downloadText(csvText, "batch-topology-normalized.csv", "text/csv");
      return;
    }
  }
  const metrics = filteredMetricsItems(result.metrics || [], result.files || []);
  const files = result.files || [];
  const baseline = files.find((file) => file.index === state.deltaBaselineIndex);
  const compare = files.find((file) => file.index === state.deltaCompareIndex);
  const rows =
    state.orientation === "files"
      ? [["building", ...metrics.map((metric) => metric.csvName || metric.id)], ...files.map((file) => [file.label || file.filename, ...metrics.map((metric) => metricValueForCSV(file, metric.id))])]
      : [["name", ...files.map((file) => file.label || file.filename)], ...metrics.map((metric) => [metric.csvName || metric.id, ...files.map((file) => metricValueForCSV(file, metric.id))])];
  if (baseline && compare) {
    rows.push([]);
    rows.push(["delta baseline", baseline.label || baseline.filename || "A"]);
    rows.push(["delta compare", compare.label || compare.filename || "B"]);
    rows.push(["metric", "A", "B", "delta", "percent"]);
    for (const metric of metrics) {
      const row = metricsDeltaRow(metric, baseline, compare);
      rows.push([metric.csvName || metric.id, row.a, row.b, row.delta, row.percent]);
    }
  }
  const csvText = `${rows.map((row) => row.map(csvCell).join(",")).join("\r\n")}\r\n`;
  downloadText(csvText, `batch-metrics-${state.orientation}.csv`, "text/csv");
}

function exportJSON() {
  if (!state.result) return;
  downloadText(`${JSON.stringify(state.result, null, 2)}\n`, "batch-metrics.json", "application/json");
}

function downloadText(text, filename, mime) {
  const blob = new Blob([text], { type: mime });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}

async function exportXLSX() {
  const result = state.result;
  if (!result) {
    return;
  }
  const api = await waitForAppAPI("SaveBatchMetricsXLSX");
  if (!api) {
    elements.status.textContent = t("tools.desktopOnly");
    return;
  }
  elements.status.textContent = t("common.loadingSettings", {}, "Loading");
  try {
    const saved = await api.SaveBatchMetricsXLSX({
      result,
      orientation: state.orientation,
      baselineIndex: state.deltaBaselineIndex ?? -1,
      compareIndex: state.deltaCompareIndex ?? -1,
    });
    if (!saved?.canceled) {
      elements.status.textContent = t("status.savedNamed", { name: saved?.filename || "batch-metrics.xlsx" }, `Saved ${saved?.filename || "batch-metrics.xlsx"}`);
    }
  } catch (error) {
    elements.status.textContent = error?.message || String(error);
  }
}

function csvCell(value) {
  const text = String(value ?? "");
  return /[",\r\n]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text;
}

const CURRENT_DOCUMENT_STORAGE_KEY = "idfAnalyzer.currentDocument";
const COMPACT_FORMATTING_RULE_ID = "compact_formatting";

function restoreDiagnoseDocument() {
  try {
    const saved = JSON.parse(window.sessionStorage.getItem(CURRENT_DOCUMENT_STORAGE_KEY) || "null");
    if (typeof saved?.text === "string" && saved.text.trim()) {
      setDiagnoseDocument(saved);
      return true;
    }
  } catch {
    // An invalid or unavailable snapshot should not block selecting another input.
  }
  renderDiagnoseEmpty();
  return false;
}

function setDiagnoseDocument(documentState = {}) {
  state.diagnose.text = String(documentState.text || "");
  state.diagnose.path = String(documentState.path || "");
  state.diagnose.filename = String(documentState.filename || "model.idf");
  state.diagnose.scan = null;
  state.diagnose.preview = null;
  state.diagnose.diagnostics = [];
  state.diagnose.selectedRuleIDs = new Set();
  state.diagnose.excludedCandidateKeys = new Set();
  state.diagnose.candidateFilter = "";
  elements.diagnoseCandidateFilter.value = "";
  elements.diagnoseFilename.textContent = state.diagnose.filename || t("common.inputFile", {}, "Input file");
  elements.diagnoseFilename.title = state.diagnose.path;
  persistDiagnoseDocument();
  refreshDiagnose();
}

async function selectDiagnoseInput() {
  const api = await waitForAppAPI("OpenInputFile");
  if (api) {
    const result = await api.OpenInputFile();
    if (!result?.canceled && result?.text) {
      setDiagnoseDocument(result);
    }
    return;
  }
  elements.diagnoseBrowserFile.click();
}

async function loadDiagnoseBrowserFile(event) {
  const file = event.target.files?.[0];
  if (!file) {
    return;
  }
  setDiagnoseDocument({ text: await file.text(), filename: file.name, path: "" });
  event.target.value = "";
}

async function refreshDiagnose() {
  if (!state.diagnose.text.trim()) {
    renderDiagnoseEmpty();
    return;
  }
  setDiagnoseBusy(true);
  elements.diagnoseStatus.textContent = t("diagnose.running", {}, "Diagnostics are running");
  try {
    const [diagnostics, cleanup] = await Promise.all([
      analyzeDiagnoseText(state.diagnose.text),
      scanDiagnoseText(state.diagnose.text),
    ]);
    state.diagnose.diagnostics = diagnostics || [];
    state.diagnose.scan = cleanup;
    state.diagnose.preview = null;
    initializeDiagnoseSelection(cleanup);
    renderDiagnose();
  } catch (error) {
    renderDiagnoseError(error);
  } finally {
    setDiagnoseBusy(false);
  }
}

async function analyzeDiagnoseText(text) {
  const api = await waitForAppAPI("AnalyzeInputDiagnosticsText");
  if (api) {
    return api.AnalyzeInputDiagnosticsText(text);
  }
  const result = await postJSON("/api/analyze-input", { text });
  return result?.report?.diagnostics || [];
}

async function scanDiagnoseText(text) {
  const api = await waitForAppAPI("ScanCleanupText");
  if (api) {
    return api.ScanCleanupText(text, state.diagnose.path, state.diagnose.filename);
  }
  return postJSON("/api/cleanup-scan", {
    text,
    path: state.diagnose.path,
    filename: state.diagnose.filename,
  });
}

async function previewDiagnoseFixes() {
  if (!canRunDiagnoseFixes()) {
    return;
  }
  setDiagnoseBusy(true);
  try {
    state.diagnose.preview = await buildDiagnosePreview();
    renderDiagnosePreview(state.diagnose.preview);
  } catch (error) {
    renderDiagnoseError(error);
  } finally {
    setDiagnoseBusy(false);
  }
}

async function buildDiagnosePreview() {
  const payload = diagnoseCleanupPayload();
  const api = await waitForAppAPI("PreviewCleanupText");
  if (api) {
    return api.PreviewCleanupText(payload.text, payload.ruleIds, payload.excludedCandidateKeys);
  }
  return postJSON("/api/cleanup-preview", payload);
}

async function applyDiagnoseFixes() {
  if (!canRunDiagnoseFixes()) {
    return;
  }
  setDiagnoseBusy(true);
  try {
    const preview = state.diagnose.preview || await buildDiagnosePreview();
    state.diagnose.text = preview.text || state.diagnose.text;
    persistDiagnoseDocument();
    elements.diagnoseStatus.textContent = t(
      "diagnoseFix.applied",
      { count: preview.removedCount || 0 },
      `${preview.removedCount || 0} selected fixes applied.`,
    );
    await refreshDiagnose();
  } catch (error) {
    renderDiagnoseError(error);
  } finally {
    setDiagnoseBusy(false);
  }
}

async function saveDiagnoseCopy() {
  if (!canRunDiagnoseFixes()) {
    return;
  }
  setDiagnoseBusy(true);
  try {
    const payload = diagnoseCleanupPayload();
    const api = await waitForAppAPI("SaveCleanupAs");
    if (!api) {
      const preview = state.diagnose.preview || await buildDiagnosePreview();
      downloadText(preview.text || state.diagnose.text, diagnoseSaveAsFilename(), "text/plain");
      return;
    }
    const result = await api.SaveCleanupAs(payload.text, diagnoseSaveAsFilename(), payload.ruleIds, payload.excludedCandidateKeys);
    if (!result?.canceled) {
      elements.diagnoseStatus.textContent = t("status.savedNamed", { name: result.filename || diagnoseSaveAsFilename() });
    }
  } catch (error) {
    renderDiagnoseError(error);
  } finally {
    setDiagnoseBusy(false);
  }
}

function persistDiagnoseDocument() {
  try {
    const previous = JSON.parse(window.sessionStorage.getItem(CURRENT_DOCUMENT_STORAGE_KEY) || "{}") || {};
    window.sessionStorage.setItem(CURRENT_DOCUMENT_STORAGE_KEY, JSON.stringify({
      ...previous,
      text: state.diagnose.text,
      path: state.diagnose.path,
      filename: state.diagnose.filename,
      analysisKey: "",
      textHash: "",
      analysisStage: "idle",
      geometryReady: false,
      capturedAt: new Date().toISOString(),
    }));
  } catch {
    // Continue to support analysis even when browser storage is unavailable.
  }
}

function initializeDiagnoseSelection(cleanup) {
  state.diagnose.selectedRuleIDs = new Set(
    (cleanup?.scan?.rules || []).filter((rule) => rule.default && rule.available).map((rule) => rule.id),
  );
  state.diagnose.excludedCandidateKeys = new Set();
  state.diagnose.candidateFilter = "";
  elements.diagnoseCandidateFilter.value = "";
}

function renderDiagnose() {
  const diagnostics = state.diagnose.diagnostics || [];
  const candidates = state.diagnose.scan?.scan?.candidates || [];
  const errors = diagnostics.filter((item) => item.severity === "error").length;
  elements.diagnoseStatus.textContent = `${diagnostics.length} diagnostics \u00b7 ${errors} errors \u00b7 ${candidates.length} fix candidates`;
  renderDiagnoseList(diagnostics);
  renderDiagnoseRules();
  renderDiagnoseCandidates();
  renderDiagnosePreview(null);
  updateDiagnoseButtons();
}

function renderDiagnoseList(diagnostics) {
  if (!diagnostics.length) {
    elements.diagnoseList.innerHTML = `<div class="empty">${escapeHTML(t("diagnose.noDiagnostics", {}, "No diagnostics found"))}</div>`;
    return;
  }
  elements.diagnoseList.innerHTML = diagnostics.slice(0, BATCH_METRICS_TABLE_RENDER_LIMIT).map((item) => `
    <details class="diagnostic-item ${escapeHTML(item.severity || "warning")}">
      <summary class="diagnostic-metrics">
        <span class="diagnostic-row-main">
          <span class="diagnostic-severity">${escapeHTML(item.severity || "warning")}</span>
          <span class="diagnostic-category">${escapeHTML(item.category || "Diagnostic")}</span>
          <strong>${escapeHTML(item.message || "")}</strong>
        </span>
        ${item.code ? `<span class="diagnostic-code">${escapeHTML(item.code)}</span>` : ""}
      </summary>
      <div class="diagnostic-main">
        <div class="diagnostic-meta">${[item.source, item.confidence].filter(Boolean).map((value) => `<span>${escapeHTML(value)}</span>`).join("")}</div>
        <div class="diagnostic-context">${[item.objectType, item.objectName, item.field, item.value].filter((value) => String(value ?? "").trim()).map((value) => `<span>${escapeHTML(value)}</span>`).join("")}</div>
        ${item.evidence ? `<p class="diagnostic-evidence">${escapeHTML(item.evidence)}</p>` : ""}
      </div>
    </details>`).join("");
}

function renderDiagnoseRules() {
  const rules = state.diagnose.scan?.scan?.rules || [];
  elements.diagnoseRules.innerHTML = rules.length ? rules.map((rule) => {
    const checked = rule.available && state.diagnose.selectedRuleIDs.has(rule.id) ? "checked" : "";
    return `<label class="cleanup-rule ${rule.available ? "" : "disabled"}">
      <input data-diagnose-rule="${escapeHTML(rule.id)}" type="checkbox" ${checked} ${rule.available ? "" : "disabled"} />
      <span><strong>${escapeHTML(rule.name || rule.id)}</strong><small>${escapeHTML(rule.description || "")}</small><em>${escapeHTML(rule.group || "")}</em></span>
    </label>`;
  }).join("") : `<div class="empty">${escapeHTML(t("tools.noCleanupCandidates", {}, "No cleanup candidates found."))}</div>`;
  elements.diagnoseRules.querySelectorAll("[data-diagnose-rule]").forEach((input) => input.addEventListener("change", () => {
    if (input.checked) state.diagnose.selectedRuleIDs.add(input.dataset.diagnoseRule);
    else state.diagnose.selectedRuleIDs.delete(input.dataset.diagnoseRule);
    state.diagnose.preview = null;
    renderDiagnoseCandidates();
  }));
}

function renderDiagnoseCandidates() {
  const candidates = state.diagnose.scan?.scan?.candidates || [];
  const query = state.diagnose.candidateFilter.trim().toLowerCase();
  const visible = candidates.filter((item) => !query || [item.ruleId, item.objectType, item.objectName, item.reason, item.risk].some((value) => String(value || "").toLowerCase().includes(query)));
  const selectedCount = selectedDiagnoseCandidates(candidates).length;
  elements.diagnoseCandidateStats.textContent = `${selectedCount} selected of ${candidates.length}`;
  elements.diagnoseCandidates.innerHTML = visible.length ? `<div class="cleanup-candidate-list">${visible.map((item) => {
    const active = state.diagnose.selectedRuleIDs.has(item.ruleId);
    const selected = active && !state.diagnose.excludedCandidateKeys.has(item.key);
    return `<label class="cleanup-candidate ${selected ? "selected" : ""} ${active ? "" : "inactive"}">
      <input data-diagnose-candidate="${escapeHTML(item.key)}" type="checkbox" ${selected ? "checked" : ""} ${active ? "" : "disabled"} />
      <span><strong>${escapeHTML(item.objectName || `#${Number(item.objectIndex) + 1}`)}</strong><small>${escapeHTML(item.objectType || "")} / ${escapeHTML(item.ruleId || "")}</small><em>${escapeHTML(item.reason || "")}</em></span>
    </label>`;
  }).join("")}</div>` : `<div class="empty">${escapeHTML(t("tools.noMatchingCandidates", {}, "No matching candidates."))}</div>`;
  elements.diagnoseCandidates.querySelectorAll("[data-diagnose-candidate]").forEach((input) => input.addEventListener("change", () => {
    if (input.checked) state.diagnose.excludedCandidateKeys.delete(input.dataset.diagnoseCandidate);
    else state.diagnose.excludedCandidateKeys.add(input.dataset.diagnoseCandidate);
    state.diagnose.preview = null;
    renderDiagnoseCandidates();
  }));
  updateDiagnoseButtons();
}

function renderDiagnosePreview(preview) {
  elements.diagnosePreviewPanel.hidden = !preview;
  elements.diagnosePreviewPanel.innerHTML = preview ? `
    <div class="diagnose-fix-preview-head"><strong>${escapeHTML(t("common.preview", {}, "Preview"))}</strong><span>${preview.removedCount || 0} removals \u00b7 ${preview.objectCount || 0} objects</span></div>
    ${(preview.removedCandidates || []).length ? `<ul>${preview.removedCandidates.slice(0, 80).map((item) => `<li><strong>${escapeHTML(item.objectType || "")}</strong> ${escapeHTML(item.objectName || "")} <span>${escapeHTML(item.reason || "")}</span></li>`).join("")}</ul>` : `<div class="empty">${escapeHTML(t("diagnoseFix.formattingOnly", {}, "Formatting-only preview."))}</div>`}` : "";
}

function renderDiagnoseEmpty() {
  elements.diagnoseFilename.textContent = t("tools.noCurrentInputShort", {}, "No current input.");
  elements.diagnoseStatus.textContent = t("tools.noCurrentInput", {}, "Open an input first.");
  elements.diagnoseList.innerHTML = `<div class="empty">${escapeHTML(t("tools.noCurrentInput", {}, "Open an input first."))}</div>`;
  elements.diagnoseRules.innerHTML = elements.diagnoseList.innerHTML;
  elements.diagnoseCandidates.innerHTML = elements.diagnoseList.innerHTML;
  updateDiagnoseButtons();
}

function renderDiagnoseError(error) {
  const message = error?.message || String(error);
  elements.diagnoseStatus.textContent = message;
  elements.diagnoseList.innerHTML = `<div class="empty">${escapeHTML(message)}</div>`;
}

function selectedDiagnoseCandidates(candidates = state.diagnose.scan?.scan?.candidates || []) {
  return candidates.filter((item) => state.diagnose.selectedRuleIDs.has(item.ruleId) && !state.diagnose.excludedCandidateKeys.has(item.key));
}

function canRunDiagnoseFixes() {
  return Boolean(state.diagnose.scan) && (selectedDiagnoseCandidates().length > 0 || state.diagnose.selectedRuleIDs.has(COMPACT_FORMATTING_RULE_ID));
}

function diagnoseCleanupPayload() {
  return {
    text: state.diagnose.text,
    ruleIds: [...state.diagnose.selectedRuleIDs],
    excludedCandidateKeys: [...state.diagnose.excludedCandidateKeys],
  };
}

function setDiagnoseBusy(busy) {
  state.diagnose.busy = busy;
  updateDiagnoseButtons();
}

function updateDiagnoseButtons() {
  const disabled = state.diagnose.busy || !canRunDiagnoseFixes();
  elements.diagnoseRefresh.disabled = state.diagnose.busy || !state.diagnose.text.trim();
  elements.diagnosePreview.disabled = disabled;
  elements.diagnoseApply.disabled = disabled;
  elements.diagnoseSaveAs.disabled = disabled;
  elements.diagnoseCandidateFilter.disabled = state.diagnose.busy || !state.diagnose.scan;
}

function diagnoseSaveAsFilename() {
  const filename = state.diagnose.filename || "model.idf";
  const dot = filename.lastIndexOf(".");
  return dot > 0 ? `${filename.slice(0, dot)}-cleaned${filename.slice(dot)}` : `${filename}-cleaned.idf`;
}

function downloadCSV(rows, filename) {
  const csvText = `${rows.map((row) => row.map(csvCell).join(",")).join("\r\n")}\r\n`;
  const blob = new Blob([csvText], { type: "text/csv" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}

multiSimulationTool = initializeMultiSimulationTool({
  state,
  elements,
  waitForAppAPI,
  waitForProgressRuntime,
  escapeHTML,
  postJSON,
  t,
  downloadCSV,
});
elements.selectButton.addEventListener("click", runBatchMetrics);
elements.exportButton.addEventListener("click", exportCSV);
elements.exportJSONButton?.addEventListener("click", exportJSON);
elements.exportXLSXButton?.addEventListener("click", exportXLSX);

elements.toolNavButtons.forEach((button) => {
  button.addEventListener("click", () => switchToolsTab(button.dataset.toolsTab));
});
elements.orientationButtons.forEach((button) => {
  button.addEventListener("click", () => {
    state.orientation = button.dataset.metricsOrientation;
    elements.orientationButtons.forEach((item) => item.classList.toggle("active", item === button));
    renderTable();
  });
});
elements.metricGroup?.addEventListener("change", () => {
  state.metricGroup = elements.metricGroup.value || "all";
  renderTable();
});
elements.includeFullTopology?.addEventListener("change", () => {
  state.includeFullTopology = Boolean(elements.includeFullTopology.checked);
});
elements.statusFilter?.addEventListener("change", () => {
  state.statusFilter = elements.statusFilter.value || "all";
  renderTable();
});
elements.deltaSort?.addEventListener("change", () => {
  state.deltaSort = elements.deltaSort.value || "table";
  renderTable();
});
elements.diagnoseSelectInput?.addEventListener("click", selectDiagnoseInput);
elements.diagnoseBrowserFile?.addEventListener("change", loadDiagnoseBrowserFile);
elements.diagnoseRefresh?.addEventListener("click", refreshDiagnose);
elements.diagnosePreview?.addEventListener("click", previewDiagnoseFixes);
elements.diagnoseApply?.addEventListener("click", applyDiagnoseFixes);
elements.diagnoseSaveAs?.addEventListener("click", saveDiagnoseCopy);
elements.diagnoseCandidateFilter?.addEventListener("input", () => {
  state.diagnose.candidateFilter = elements.diagnoseCandidateFilter.value || "";
  renderDiagnoseCandidates();
});

registerProgressListener();
multiSimulationTool?.loadEnvironment();
restoreDiagnoseDocument();
switchToolsTab(window.location.hash.replace(/^#/, "") || state.activeTool, { updateHash: false });

async function postJSON(url, payload) {
  const response = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload || {}),
  });
  if (!response.ok) {
    throw new Error(await response.text() || `${url} failed`);
  }
  return response.json();
}

function switchToolsTab(toolID, { updateHash = true } = {}) {
  const panel = [...elements.toolPanels].find((item) => item.dataset.toolsPanel === toolID);
  if (!panel) {
    toolID = "batch-metrics";
  }
  state.activeTool = toolID;
  elements.toolNavButtons.forEach((button) => {
    const active = button.dataset.toolsTab === toolID;
    button.classList.toggle("active", active);
    button.setAttribute("aria-current", active ? "page" : "false");
  });
  elements.toolPanels.forEach((item) => {
    item.classList.toggle("active", item.dataset.toolsPanel === toolID);
  });
  if (updateHash) {
    window.history.replaceState(null, "", `#${toolID}`);
  }
}
