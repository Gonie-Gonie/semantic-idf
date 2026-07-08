import { loadAndApplyAppSettings } from "./settings-client.js";
import { renderAppInfo } from "./app-info.js";
import { t } from "./i18n.js";
import { initializeMultiSimulationTool } from "./batch/batch-simulation.js";

loadAndApplyAppSettings();
renderAppInfo();

const BATCH_TABLE_RENDER_LIMIT = 500;

const state = {
  activeBatchTool: "batch-summary",
  result: null,
  activeRunID: "",
  orientation: "metrics",
  metricGroup: "all",
  statusFilter: "all",
  deltaSort: "table",
  deltaBaselineIndex: null,
  deltaCompareIndex: null,
  running: false,
  progressFiles: new Map(),
  progressListenerRegistered: false,
  batchDiagnose: null,
  batchDiagnoseSource: "all",
  batchDiagnoseCategory: "all",
  batchDiagnoseConfidence: "all",
  batchDiagnoseSeverity: "all",
  batchOutputQA: null,
  batchCleanupReport: null,
  batchConvertExport: null,
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
  },
};

let multiSimulationTool = null;

const elements = {
  batchNavButtons: document.querySelectorAll("[data-batch-tab]"),
  batchPanels: document.querySelectorAll("[data-batch-panel]"),
  selectButton: document.querySelector("#multiSummarySelect"),
  exportButton: document.querySelector("#multiSummaryExport"),
  exportXLSXButton: document.querySelector("#multiSummaryExportXLSX"),
  stats: document.querySelector("#multiSummaryStats"),
  status: document.querySelector("#multiSummaryStatus"),
  percent: document.querySelector("#multiSummaryPercent"),
  progressBar: document.querySelector("#multiSummaryProgressBar"),
  fileList: document.querySelector("#multiSummaryFiles"),
  table: document.querySelector("#multiSummaryTable"),
  orientationButtons: document.querySelectorAll("[data-summary-orientation]"),
  metricGroup: document.querySelector("#batchSummaryMetricGroup"),
  statusFilter: document.querySelector("#batchSummaryStatusFilter"),
  deltaSort: document.querySelector("#batchSummaryDeltaSort"),
  delta: document.querySelector("#batchSummaryDelta"),
  batchDiagnoseSelect: document.querySelector("#batchDiagnoseSelect"),
  batchDiagnoseExport: document.querySelector("#batchDiagnoseExport"),
  batchDiagnoseSourceFilter: document.querySelector("#batchDiagnoseSourceFilter"),
  batchDiagnoseCategoryFilter: document.querySelector("#batchDiagnoseCategoryFilter"),
  batchDiagnoseConfidenceFilter: document.querySelector("#batchDiagnoseConfidenceFilter"),
  batchDiagnoseSeverityFilter: document.querySelector("#batchDiagnoseSeverityFilter"),
  batchDiagnoseStats: document.querySelector("#batchDiagnoseStats"),
  batchDiagnoseSummary: document.querySelector("#batchDiagnoseSummary"),
  batchDiagnoseTable: document.querySelector("#batchDiagnoseTable"),
  batchOutputQASelect: document.querySelector("#batchOutputQASelect"),
  batchOutputQAExport: document.querySelector("#batchOutputQAExport"),
  batchOutputQATable: document.querySelector("#batchOutputQATable"),
  batchConvertSelect: document.querySelector("#batchConvertSelect"),
  batchConvertRun: document.querySelector("#batchConvertRun"),
  batchConvertFormat: document.querySelector("#batchConvertFormat"),
  batchConvertOverwrite: document.querySelector("#batchConvertOverwrite"),
  batchConvertTable: document.querySelector("#batchConvertTable"),
  batchCleanupSelect: document.querySelector("#batchCleanupSelect"),
  batchCleanupCreateCopies: document.querySelector("#batchCleanupCreateCopies"),
  batchCleanupTable: document.querySelector("#batchCleanupTable"),
  batchPurposeInputs: document.querySelectorAll("[data-batch-purpose]"),
  multiSimulationSelectFiles: document.querySelector("#multiSimulationSelectFiles"),
  multiSimulationSelectFolder: document.querySelector("#multiSimulationSelectFolder"),
  multiSimulationRun: document.querySelector("#multiSimulationRun"),
  multiSimulationExport: document.querySelector("#multiSimulationExport"),
  multiSimulationEnergyPlus: document.querySelector("#multiSimulationEnergyPlus"),
  multiSimulationWeather: document.querySelector("#multiSimulationWeather"),
  multiSimulationWeatherMode: document.querySelector("#multiSimulationWeatherMode"),
  multiSimulationWorkers: document.querySelector("#multiSimulationWorkers"),
  multiSimulationViewMode: document.querySelector("#multiSimulationViewMode"),
  multiSimulationRecursive: document.querySelector("#multiSimulationRecursive"),
  multiSimulationStats: document.querySelector("#multiSimulationStats"),
  multiSimulationSort: document.querySelector("#multiSimulationSort"),
  multiSimulationStatus: document.querySelector("#multiSimulationStatus"),
  multiSimulationPercent: document.querySelector("#multiSimulationPercent"),
  multiSimulationProgressBar: document.querySelector("#multiSimulationProgressBar"),
  multiSimulationFiles: document.querySelector("#multiSimulationFiles"),
  multiSimulationMetric: document.querySelector("#multiSimulationMetric"),
  multiSimulationChart: document.querySelector("#multiSimulationChart"),
  multiSimulationTable: document.querySelector("#multiSimulationTable"),
  batchSimulationPlanPreview: document.querySelector("#batchSimulationPlanPreview"),
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
    window.runtime.EventsOn("idfAnalyzer:multiSummaryProgress", handleProgress);
    window.runtime.EventsOn("idfAnalyzer:multiSimulationProgress", handleMultiSimulationProgress);
    state.progressListenerRegistered = true;
  } else if (typeof window.runtime.EventsOnMultiple === "function") {
    window.runtime.EventsOnMultiple("idfAnalyzer:multiSummaryProgress", handleProgress, -1);
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
  if (elements.exportXLSXButton) {
    elements.exportXLSXButton.disabled = running || !state.result;
  }
}

async function runMultiSummary() {
  state.result = null;
  state.progressFiles.clear();
  state.activeRunID = `multi-summary-${Date.now()}-${Math.random().toString(36).slice(2)}`;
  setRunning(true);
  elements.stats.textContent = t("tools.waitingSelection");
  elements.status.textContent = t("status.openDialog");
  elements.table.innerHTML = `<div class="empty">${t("status.analysisWillStart")}</div>`;
  elements.fileList.innerHTML = "";
  updateProgress(0, 0);
  waitForProgressRuntime();

  try {
    const result = await analyzeMultiSummary(state.activeRunID);
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

async function analyzeMultiSummary(runID) {
  let responseError = "";
  try {
    for (const url of ["/api/batch-summary", "/api/multi-idf-summary"]) {
      const response = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ runId: runID }),
      });
      if (response.ok) {
        return response.json();
      }
      responseError = await response.text();
    }
  } catch (error) {
    responseError = error?.message || String(error);
  }

  const api = await waitForAppAPI("AnalyzeMultiIDFSummary");
  if (api) {
    return api.AnalyzeMultiIDFSummary(runID);
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
  elements.stats.textContent = t("count.filesSummary", { total, ok: succeeded, failed, workers });
  renderFileList(result.files || []);
  renderTable();
  elements.exportButton.disabled = state.running || !result.metrics?.length;
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
  const metrics = filteredSummaryMetrics(result?.metrics || [], result?.files || []);
  const files = result?.files || [];
  if (!metrics.length || !files.length) {
    elements.table.innerHTML = `<div class="empty">${t("tools.noSummaryData")}</div>`;
    renderDeltaDrawer(metrics, files);
    return;
  }
  elements.table.innerHTML = state.orientation === "files" ? renderFilesAsRows(metrics, files) : renderMetricsAsRows(metrics, files);
  bindDeltaColumnButtons();
  renderDeltaDrawer(metrics, files);
}

function renderMetricsAsRows(metrics, files) {
  const renderedMetrics = metrics.slice(0, BATCH_TABLE_RENDER_LIMIT);
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
  const renderedFiles = files.slice(0, BATCH_TABLE_RENDER_LIMIT);
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
  const role = summaryDeltaRole(file.index);
  return `<button class="batch-column-button ${escapeHTML(role)}" data-batch-summary-column="${escapeHTML(file.index)}" type="button">${content}</button>`;
}

function renderValueCell(file, metricID) {
  if (file.status !== "ok") {
    return `<td class="tool-value error"></td>`;
  }
  const value = file.metricValues?.[metricID];
  const status = value?.status || "missing";
  return `<td class="tool-value ${escapeHTML(status)}" title="${escapeHTML(status)}">${escapeHTML(value?.displayValue ?? t("common.notAvailable"))}</td>`;
}

function renderBatchHiddenRowsNotice(hiddenCount, label) {
  return hiddenCount > 0
    ? `<div class="empty compact">${escapeHTML(`${hiddenCount} additional ${label} hidden. Narrow filters to render them.`)}</div>`
    : "";
}

function metricValueForCSV(file, metricID) {
  if (file.status !== "ok") {
    return "";
  }
  return file.metricValues?.[metricID]?.displayValue ?? "N/A";
}

function filteredSummaryMetrics(metrics, files) {
  return metrics.filter((metric) => {
    if (!summaryMetricMatchesGroup(metric, files)) {
      return false;
    }
    if (state.statusFilter === "all") {
      return true;
    }
    return files.some((file) => summaryValueStatus(file, metric.id) === state.statusFilter);
  });
}

function summaryMetricMatchesGroup(metric, files) {
  const group = state.metricGroup || "all";
  if (group === "all") {
    return true;
  }
  if (group === "reliable") {
    return files.every((file) => file.status !== "ok" || summaryValueStatus(file, metric.id) === "ok");
  }
  const category = String(metric.category || "").toLowerCase();
  const id = String(metric.id || "").toLowerCase();
  const name = String(metric.name || metric.csvName || "").toLowerCase();
  const haystack = `${category} ${id} ${name}`;
  const groups = {
    inventory: ["inventory", "model"],
    geometry: ["geometry", "area", "zone", "surface", "wwr", "fenestration"],
    envelope: ["envelope", "construction", "material", "window"],
    loads: ["load", "lighting", "equipment", "people", "internal"],
    schedules: ["schedule"],
    hvac: ["hvac", "air loop", "plant loop", "node", "coil"],
  };
  return (groups[group] || [group]).some((token) => haystack.includes(token));
}

function summaryValueStatus(file, metricID) {
  if (file.status !== "ok") {
    return "error";
  }
  return file.metricValues?.[metricID]?.status || "missing";
}

function bindDeltaColumnButtons() {
  elements.table.querySelectorAll("[data-batch-summary-column]").forEach((button) => {
    button.addEventListener("click", () => {
      const index = Number(button.dataset.batchSummaryColumn);
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

function summaryDeltaRole(index) {
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
  const rows = sortSummaryDeltaRows(metrics.map((metric) => summaryDeltaRow(metric, baseline, compare)));
  elements.delta.innerHTML = `
    <div class="batch-delta-head">
      <div>
        <strong>${escapeHTML(t("batch.selectedCompare", {}, "Selected compare"))}</strong>
        <span>A: ${escapeHTML(baseline.label || baseline.filename)} / B: ${escapeHTML(compare.label || compare.filename)}</span>
      </div>
      <button id="batchSummaryClearDelta" type="button">${escapeHTML(t("action.close"))}</button>
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
          ${rows.map(renderSummaryDeltaRow).join("")}
        </tbody>
      </table>
    </div>`;
  document.querySelector("#batchSummaryClearDelta")?.addEventListener("click", () => {
    state.deltaBaselineIndex = null;
    state.deltaCompareIndex = null;
    renderTable();
  });
}

function summaryDeltaRow(metric, baseline, compare) {
  const a = baseline.metricValues?.[metric.id];
  const b = compare.metricValues?.[metric.id];
  const aNumber = parseSummaryNumber(a?.displayValue);
  const bNumber = parseSummaryNumber(b?.displayValue);
  const aStatus = summaryValueStatus(baseline, metric.id);
  const bStatus = summaryValueStatus(compare, metric.id);
  const sameUnit = summaryUnit(metric, a?.displayValue) === summaryUnit(metric, b?.displayValue);
  const status = [aStatus, bStatus].join(" -> ");
  if (aNumber.ok && bNumber.ok && sameUnit) {
    const delta = bNumber.value - aNumber.value;
    const percentValue = aNumber.value === 0 ? null : (delta / aNumber.value) * 100;
    return {
      metric,
      a: a?.displayValue ?? t("common.notAvailable"),
      b: b?.displayValue ?? t("common.notAvailable"),
      delta: formatDelta(delta, metric.unit),
      percent: percentValue === null ? t("common.notAvailable") : `${formatNumber(percentValue)}%`,
      deltaValue: delta,
      percentValue,
      missing: aStatus === "missing" || bStatus === "missing",
      statusChanged: aStatus !== bStatus,
      status,
    };
  }
  const changed = String(a?.displayValue ?? "") === String(b?.displayValue ?? "") ? t("batch.unchanged", {}, "unchanged") : t("batch.changed", {}, "changed");
  return {
    metric,
    a: a?.displayValue ?? t("common.notAvailable"),
    b: b?.displayValue ?? t("common.notAvailable"),
    delta: changed,
    percent: t("common.notAvailable"),
    deltaValue: null,
    percentValue: null,
    missing: aStatus === "missing" || bStatus === "missing",
    statusChanged: aStatus !== bStatus || changed !== t("batch.unchanged", {}, "unchanged"),
    status,
  };
}

function sortSummaryDeltaRows(rows) {
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

function renderSummaryDeltaRow(row) {
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

function parseSummaryNumber(value) {
  const match = String(value ?? "").trim().match(/^[-+]?(\d+(\.\d*)?|\.\d+)([eE][-+]?\d+)?/);
  if (!match) {
    return { ok: false, value: 0 };
  }
  const number = Number(match[0]);
  return Number.isFinite(number) ? { ok: true, value: number } : { ok: false, value: 0 };
}

function summaryUnit(metric, displayValue) {
  const unit = String(metric.unit || "").trim();
  if (unit) {
    return unit;
  }
  const text = String(displayValue || "").trim();
  const number = parseSummaryNumber(text);
  return number.ok ? text.slice(String(number.value).length).trim() : "";
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

function exportCSV() {
  const result = state.result;
  if (!result) {
    return;
  }
  const metrics = filteredSummaryMetrics(result.metrics || [], result.files || []);
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
      const row = summaryDeltaRow(metric, baseline, compare);
      rows.push([metric.csvName || metric.id, row.a, row.b, row.delta, row.percent]);
    }
  }
  const csvText = `${rows.map((row) => row.map(csvCell).join(",")).join("\r\n")}\r\n`;
  const blob = new Blob([csvText], { type: "text/csv" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `multi-idf-summary-${state.orientation}.csv`;
  link.click();
  URL.revokeObjectURL(url);
}

async function exportXLSX() {
  const result = state.result;
  if (!result) {
    return;
  }
  const api = await waitForAppAPI("SaveBatchSummaryXLSX");
  if (!api) {
    elements.status.textContent = t("tools.desktopOnly");
    return;
  }
  elements.status.textContent = t("common.loadingSettings", {}, "Loading");
  try {
    const saved = await api.SaveBatchSummaryXLSX({
      result,
      orientation: state.orientation,
      baselineIndex: state.deltaBaselineIndex ?? -1,
      compareIndex: state.deltaCompareIndex ?? -1,
    });
    if (!saved?.canceled) {
      elements.status.textContent = t("status.savedNamed", { name: saved?.filename || "batch-summary.xlsx" }, `Saved ${saved?.filename || "batch-summary.xlsx"}`);
    }
  } catch (error) {
    elements.status.textContent = error?.message || String(error);
  }
}

function csvCell(value) {
  const text = String(value ?? "");
  return /[",\r\n]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text;
}

async function runBatchDiagnose() {
  const runID = `batch-diagnose-${Date.now()}`;
  elements.batchDiagnoseStats.textContent = t("diagnose.running", {}, "Diagnostics are running");
  elements.batchDiagnoseTable.innerHTML = `<div class="empty status-loading">${escapeHTML(t("diagnose.running", {}, "Diagnostics are running"))}</div>`;
  try {
    const api = await waitForAppAPI("RunBatchDiagnose");
    const result = api ? await api.RunBatchDiagnose(runID) : await postJSON("/api/batch-diagnose", { runId: runID, inputPaths: [] });
    if (result?.canceled) {
      elements.batchDiagnoseStats.textContent = t("status.fileSelectionCanceled");
      return;
    }
    state.batchDiagnose = result;
    renderBatchDiagnoseFilterOptions();
    renderBatchDiagnose();
  } catch (error) {
    elements.batchDiagnoseStats.textContent = error?.message || String(error);
    elements.batchDiagnoseTable.innerHTML = `<div class="empty">${escapeHTML(error?.message || String(error))}</div>`;
  }
}

function renderBatchDiagnose() {
  const result = state.batchDiagnose;
  if (!result) {
    return;
  }
  const files = result.files || [];
  const codes = filteredBatchIssueCodes(result.issueCodes || []);
  elements.batchDiagnoseStats.textContent = t("count.filesSummary", {
    total: result.total || files.length,
    ok: result.succeeded || 0,
    failed: result.failed || 0,
    workers: 0,
  });
  elements.batchDiagnoseExport.disabled = !codes.length;
  renderBatchDiagnoseSummary(files, codes);
  if (!codes.length) {
    elements.batchDiagnoseTable.innerHTML = `<div class="empty">${escapeHTML(t("diagnose.noDiagnostics", {}, "No diagnostics found"))}</div>`;
    return;
  }
  const renderedCodes = codes.slice(0, BATCH_TABLE_RENDER_LIMIT);
  elements.batchDiagnoseTable.innerHTML = `
    ${renderBatchHiddenRowsNotice(codes.length - renderedCodes.length, "diagnostic codes")}
    <table class="tool-table">
      <thead>
        <tr>
          <th class="tool-sticky-col">${escapeHTML(t("common.key"))}</th>
          <th>${escapeHTML(t("common.type"))}</th>
          <th>${escapeHTML(t("common.source"))}</th>
          <th>${escapeHTML(t("common.confidence", {}, "Confidence"))}</th>
          ${files.map((file) => `<th>${escapeHTML(file.label || file.filename)}</th>`).join("")}
          <th>${escapeHTML(t("common.total", {}, "Total"))}</th>
        </tr>
      </thead>
      <tbody>
        ${renderedCodes
          .map(
            (item) => `
              <tr>
                <th class="tool-sticky-col">
                  <strong>${escapeHTML(item.code || "")}</strong>
                  <span>${escapeHTML(item.category || "")}</span>
                </th>
                <td>${escapeHTML(item.severity || "")}</td>
                <td>${escapeHTML(item.source || "")}</td>
                <td>${escapeHTML(item.confidence || "")}</td>
                ${files.map((file) => `<td>${escapeHTML(item.fileCounts?.[file.label || file.filename] || 0)}</td>`).join("")}
                <td>${escapeHTML(item.count || 0)}</td>
              </tr>`,
          )
          .join("")}
      </tbody>
    </table>`;
}

function renderBatchDiagnoseFilterOptions() {
  const codes = state.batchDiagnose?.issueCodes || [];
  renderSelectOptions(elements.batchDiagnoseCategoryFilter, uniqueValues(codes.map((item) => item.category)));
  renderSelectOptions(elements.batchDiagnoseConfidenceFilter, uniqueValues(codes.map((item) => item.confidence)));
  state.batchDiagnoseCategory = elements.batchDiagnoseCategoryFilter?.value || "all";
  state.batchDiagnoseConfidence = elements.batchDiagnoseConfidenceFilter?.value || "all";
}

function renderSelectOptions(select, values) {
  if (!select) {
    return;
  }
  const current = select.value || "all";
  select.innerHTML = [`<option value="all">${escapeHTML(t("common.all"))}</option>`, ...values.map((value) => `<option value="${escapeHTML(value)}">${escapeHTML(value)}</option>`)].join("");
  select.value = values.includes(current) ? current : "all";
}

function uniqueValues(values) {
  return [...new Set(values.map((value) => String(value || "").trim()).filter(Boolean))].sort((a, b) => a.localeCompare(b));
}

function renderBatchDiagnoseSummary(files, codes) {
  if (!elements.batchDiagnoseSummary) {
    return;
  }
  if (!files.length || !codes.length) {
    elements.batchDiagnoseSummary.innerHTML = "";
    return;
  }
  const fileLabels = files.map((file) => file.label || file.filename);
  const common = codes.filter((item) => fileLabels.every((label) => (item.fileCounts?.[label] || 0) > 0));
  const specific = codes.filter((item) => fileLabels.some((label) => (item.fileCounts?.[label] || 0) > 0) && !fileLabels.every((label) => (item.fileCounts?.[label] || 0) > 0));
  const top = codes.slice(0, 5).map((item) => `${item.code} (${item.count || 0})`).join(", ") || t("common.notAvailable");
  elements.batchDiagnoseSummary.innerHTML = `
    <div><span>${escapeHTML(t("batch.commonIssues", {}, "Common issues"))}</span><strong>${escapeHTML(common.length)}</strong></div>
    <div><span>${escapeHTML(t("batch.fileSpecificIssues", {}, "File-specific issues"))}</span><strong>${escapeHTML(specific.length)}</strong></div>
    <div class="batch-summary-wide"><span>Top</span><strong>${escapeHTML(top)}</strong></div>`;
}

function filteredBatchIssueCodes(codes) {
  return codes.filter((item) => {
    if (state.batchDiagnoseSource !== "all" && item.source !== state.batchDiagnoseSource) {
      return false;
    }
    if (state.batchDiagnoseCategory !== "all" && item.category !== state.batchDiagnoseCategory) {
      return false;
    }
    if (state.batchDiagnoseConfidence !== "all" && item.confidence !== state.batchDiagnoseConfidence) {
      return false;
    }
    if (state.batchDiagnoseSeverity !== "all" && item.severity !== state.batchDiagnoseSeverity) {
      return false;
    }
    return true;
  });
}

function exportBatchDiagnoseCSV() {
  const result = state.batchDiagnose;
  if (!result) {
    return;
  }
  const files = result.files || [];
  const rows = [
    ["code", "severity", "category", "source", "confidence", ...files.map((file) => file.label || file.filename), "total"],
    ...filteredBatchIssueCodes(result.issueCodes || []).map((item) => [
      item.code,
      item.severity,
      item.category,
      item.source,
      item.confidence,
      ...files.map((file) => item.fileCounts?.[file.label || file.filename] || 0),
      item.count || 0,
    ]),
  ];
  downloadCSV(rows, "batch-diagnose.csv");
}

async function runBatchOutputQA() {
  const runID = `batch-output-qa-${Date.now()}`;
  elements.batchOutputQATable.innerHTML = `<div class="empty status-loading">${escapeHTML(t("common.loadingSettings", {}, "Loading"))}</div>`;
  try {
    const api = await waitForAppAPI("RunBatchOutputQA");
    const result = api ? await api.RunBatchOutputQA(runID) : await postJSON("/api/batch-output-qa", { runId: runID, inputPaths: [] });
    if (result?.canceled) {
      return;
    }
    state.batchOutputQA = result;
    renderBatchOutputQA();
  } catch (error) {
    elements.batchOutputQATable.innerHTML = `<div class="empty">${escapeHTML(error?.message || String(error))}</div>`;
  }
}

function renderBatchOutputQA() {
  const files = state.batchOutputQA?.files || [];
  elements.batchOutputQAExport.disabled = !files.length;
  if (!files.length) {
    elements.batchOutputQATable.innerHTML = `<div class="empty">${escapeHTML(t("batch.selectOutputQAFilesHelp", {}, "Select files to inspect output readiness and heavy output risks."))}</div>`;
    return;
  }
  const renderedFiles = files.slice(0, BATCH_TABLE_RENDER_LIMIT);
  elements.batchOutputQATable.innerHTML = `
    ${renderBatchHiddenRowsNotice(files.length - renderedFiles.length, "files")}
    <table class="tool-table">
      <thead>
        <tr>
          <th class="tool-sticky-col">${escapeHTML(t("common.file"))}</th>
          <th>SQLite</th>
          <th>Variable dictionary</th>
          <th>Output:Variable</th>
          <th>Output:Meter</th>
          <th>Output:Table</th>
          <th>Detailed/Timestep</th>
          <th>Duplicate</th>
          <th>Heavy</th>
          <th>Basic</th>
          <th>Heat Flow</th>
          <th>HVAC</th>
          <th>Integrity</th>
        </tr>
      </thead>
      <tbody>
        ${renderedFiles
          .map(
            (file) => `
              <tr>
                <th class="tool-sticky-col">
                  <strong>${escapeHTML(file.label || file.filename)}</strong>
                  <span>${escapeHTML(file.error || file.status || "")}</span>
                </th>
                <td>${yesNo(file.sqlitePresent)}</td>
                <td>${yesNo(file.variableDictionary)}</td>
                <td>${escapeHTML(file.outputVariableCount || 0)}</td>
                <td>${escapeHTML(file.outputMeterCount || 0)}</td>
                <td>${escapeHTML(file.outputTableCount || 0)}</td>
                <td>${escapeHTML(file.detailedOrTimestepCount || 0)}</td>
                <td>${escapeHTML(file.duplicateOutputCount || 0)}</td>
                <td>${escapeHTML(file.heavyWarningCount || 0)}</td>
                <td>${yesNo(file.purposeReadiness?.basic_energy)}</td>
                <td>${yesNo(file.purposeReadiness?.zone_heat_flow)}</td>
                <td>${yesNo(file.purposeReadiness?.hvac_loop_check)}</td>
                <td>${yesNo(file.purposeReadiness?.integrity_check)}</td>
              </tr>`,
          )
          .join("")}
      </tbody>
    </table>`;
}

function exportBatchOutputQACSV() {
  const rows = [
    ["file", "status", "sqlite", "variable_dictionary", "output_variable", "output_meter", "output_table", "detailed_timestep", "duplicate", "heavy", "basic_energy_ready", "zone_heat_flow_ready", "hvac_ready", "integrity_ready"],
    ...(state.batchOutputQA?.files || []).map((file) => [
      file.label || file.filename,
      file.status,
      file.sqlitePresent,
      file.variableDictionary,
      file.outputVariableCount || 0,
      file.outputMeterCount || 0,
      file.outputTableCount || 0,
      file.detailedOrTimestepCount || 0,
      file.duplicateOutputCount || 0,
      file.heavyWarningCount || 0,
      file.purposeReadiness?.basic_energy,
      file.purposeReadiness?.zone_heat_flow,
      file.purposeReadiness?.hvac_loop_check,
      file.purposeReadiness?.integrity_check,
    ]),
  ];
  downloadCSV(rows, "batch-output-qa.csv");
}

async function runBatchCleanupReport() {
  const runID = `batch-cleanup-${Date.now()}`;
  elements.batchCleanupTable.innerHTML = `<div class="empty status-loading">${escapeHTML(t("diagnoseFix.scanning", {}, "Scanning current input for suggested fixes."))}</div>`;
  try {
    const api = await waitForAppAPI("RunBatchCleanupReport");
    const result = api ? await api.RunBatchCleanupReport(runID) : await postJSON("/api/batch-cleanup-report", { runId: runID, inputPaths: [] });
    if (result?.canceled) {
      return;
    }
    state.batchCleanupReport = result;
    renderBatchCleanupReport();
  } catch (error) {
    elements.batchCleanupTable.innerHTML = `<div class="empty">${escapeHTML(error?.message || String(error))}</div>`;
  }
}

function renderBatchCleanupReport() {
  const result = state.batchCleanupReport;
  const files = result?.files || [];
  const rules = result?.rules || [];
  if (!files.length) {
    elements.batchCleanupTable.innerHTML = `<div class="empty">${escapeHTML(t("batch.selectCleanupFilesHelp", {}, "Select files to build a dry-run cleanup matrix."))}</div>`;
    return;
  }
  elements.batchCleanupCreateCopies.disabled = !files.length;
  const renderedRules = rules.slice(0, BATCH_TABLE_RENDER_LIMIT);
  elements.batchCleanupTable.innerHTML = `
    ${renderBatchHiddenRowsNotice(rules.length - renderedRules.length, "rules")}
    <table class="tool-table">
      <thead>
        <tr>
          <th class="tool-sticky-col">${escapeHTML(t("tools.rules", {}, "Rules"))}</th>
          ${files.map((file) => `<th>${escapeHTML(file.label || file.filename)}</th>`).join("")}
        </tr>
      </thead>
      <tbody>
        ${renderedRules
          .map(
            (rule) => `
              <tr>
                <th class="tool-sticky-col">
                  <strong>${escapeHTML(rule.name || rule.id)}</strong>
                  <span>${escapeHTML(rule.group || "")}</span>
                </th>
                ${files.map((file) => `<td>${escapeHTML(file.ruleCounts?.[rule.id] || 0)}</td>`).join("")}
              </tr>`,
          )
          .join("")}
      </tbody>
    </table>`;
}

async function createBatchCleanupCopies() {
  const paths = (state.batchCleanupReport?.files || []).map((file) => file.path).filter(Boolean);
  elements.batchCleanupTable.insertAdjacentHTML(
    "beforeend",
    `<div class="empty status-loading">${escapeHTML(t("batch.creatingCleanedCopies", {}, "Creating cleaned copies"))}</div>`,
  );
  try {
    const api = await waitForAppAPI("CreateBatchSafeCleanedCopies");
    const result = api
      ? await api.CreateBatchSafeCleanedCopies(paths)
      : await postJSON("/api/batch-cleanup-copy", { inputPaths: paths, overwritePolicy: "rename" });
    if (result?.canceled) {
      return;
    }
    renderBatchCleanupCopyResult(result);
  } catch (error) {
    elements.batchCleanupTable.insertAdjacentHTML("beforeend", `<div class="empty">${escapeHTML(error?.message || String(error))}</div>`);
  }
}

function renderBatchCleanupCopyResult(result) {
  const files = result?.files || [];
  const renderedFiles = files.slice(0, BATCH_TABLE_RENDER_LIMIT);
  elements.batchCleanupTable.insertAdjacentHTML(
    "beforeend",
    `
    <div class="tool-table-wrap batch-cleanup-copy-result">
      ${renderBatchHiddenRowsNotice(files.length - renderedFiles.length, "files")}
      <table class="tool-table">
        <thead>
          <tr>
            <th class="tool-sticky-col">${escapeHTML(t("common.file"))}</th>
            <th>${escapeHTML(t("common.status"))}</th>
            <th>${escapeHTML(t("common.output", {}, "Output"))}</th>
          </tr>
        </thead>
        <tbody>
          ${renderedFiles
            .map(
              (file) => `
                <tr>
                  <th class="tool-sticky-col">
                    <strong>${escapeHTML(file.label || file.filename)}</strong>
                    <span>${escapeHTML(file.error || "")}</span>
                  </th>
                  <td class="tool-value ${escapeHTML(file.status || "")}">${escapeHTML(file.status || "")}</td>
                  <td title="${escapeHTML(file.outputPath || "")}">${escapeHTML(file.outputPath || "")}</td>
                </tr>`,
            )
            .join("")}
        </tbody>
      </table>
    </div>`,
  );
}

async function runBatchConvertExport() {
  const targetFormat = elements.batchConvertFormat?.value || "idf";
  const overwritePolicy = elements.batchConvertOverwrite?.value || "rename";
  elements.batchConvertTable.innerHTML = `<div class="empty status-loading">${escapeHTML(t("common.loadingSettings", {}, "Loading"))}</div>`;
  try {
    const api = await waitForAppAPI("RunBatchConvertExport");
    const result = api
      ? await api.RunBatchConvertExport(targetFormat, overwritePolicy)
      : await postJSON("/api/batch-convert-export", { targetFormat, overwritePolicy, inputPaths: [] });
    if (result?.canceled) {
      elements.batchConvertTable.innerHTML = `<div class="empty">${escapeHTML(t("status.fileSelectionCanceled"))}</div>`;
      return;
    }
    state.batchConvertExport = result;
    renderBatchConvertExport();
  } catch (error) {
    elements.batchConvertTable.innerHTML = `<div class="empty">${escapeHTML(error?.message || String(error))}</div>`;
  }
}

function renderBatchConvertExport() {
  const files = state.batchConvertExport?.files || [];
  if (!files.length) {
    elements.batchConvertTable.innerHTML = `<div class="empty">${escapeHTML(t("batch.selectConvertFilesHelp", {}, "Select files and an output format to prepare batch conversion."))}</div>`;
    return;
  }
  const renderedFiles = files.slice(0, BATCH_TABLE_RENDER_LIMIT);
  elements.batchConvertTable.innerHTML = `
    ${renderBatchHiddenRowsNotice(files.length - renderedFiles.length, "files")}
    <table class="tool-table">
      <thead>
        <tr>
          <th class="tool-sticky-col">${escapeHTML(t("common.file"))}</th>
          <th>${escapeHTML(t("common.status"))}</th>
          <th>${escapeHTML(t("common.format", {}, "Format"))}</th>
          <th>${escapeHTML(t("common.output", {}, "Output"))}</th>
        </tr>
      </thead>
      <tbody>
        ${renderedFiles
          .map(
            (file) => `
              <tr>
                <th class="tool-sticky-col">
                  <strong>${escapeHTML(file.label || file.filename)}</strong>
                  <span>${escapeHTML(file.error || "")}</span>
                </th>
                <td class="tool-value ${escapeHTML(file.status || "")}">${escapeHTML(file.status || "")}</td>
                <td>${escapeHTML(file.format || "")}</td>
                <td title="${escapeHTML(file.outputPath || "")}">${escapeHTML(file.outputPath || "")}</td>
              </tr>`,
          )
          .join("")}
      </tbody>
    </table>`;
}

function yesNo(value) {
  return escapeHTML(value ? t("common.yes") : t("common.no"));
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
elements.selectButton.addEventListener("click", runMultiSummary);
elements.exportButton.addEventListener("click", exportCSV);
elements.exportXLSXButton?.addEventListener("click", exportXLSX);

elements.batchNavButtons.forEach((button) => {
  button.addEventListener("click", () => switchBatchTab(button.dataset.batchTab));
});
elements.orientationButtons.forEach((button) => {
  button.addEventListener("click", () => {
    state.orientation = button.dataset.summaryOrientation;
    elements.orientationButtons.forEach((item) => item.classList.toggle("active", item === button));
    renderTable();
  });
});
elements.metricGroup?.addEventListener("change", () => {
  state.metricGroup = elements.metricGroup.value || "all";
  renderTable();
});
elements.statusFilter?.addEventListener("change", () => {
  state.statusFilter = elements.statusFilter.value || "all";
  renderTable();
});
elements.deltaSort?.addEventListener("change", () => {
  state.deltaSort = elements.deltaSort.value || "table";
  renderTable();
});
elements.batchDiagnoseSelect?.addEventListener("click", runBatchDiagnose);
elements.batchDiagnoseExport?.addEventListener("click", exportBatchDiagnoseCSV);
elements.batchDiagnoseSourceFilter?.addEventListener("change", () => {
  state.batchDiagnoseSource = elements.batchDiagnoseSourceFilter.value || "all";
  renderBatchDiagnose();
});
elements.batchDiagnoseCategoryFilter?.addEventListener("change", () => {
  state.batchDiagnoseCategory = elements.batchDiagnoseCategoryFilter.value || "all";
  renderBatchDiagnose();
});
elements.batchDiagnoseConfidenceFilter?.addEventListener("change", () => {
  state.batchDiagnoseConfidence = elements.batchDiagnoseConfidenceFilter.value || "all";
  renderBatchDiagnose();
});
elements.batchDiagnoseSeverityFilter?.addEventListener("change", () => {
  state.batchDiagnoseSeverity = elements.batchDiagnoseSeverityFilter.value || "all";
  renderBatchDiagnose();
});
elements.batchOutputQASelect?.addEventListener("click", runBatchOutputQA);
elements.batchOutputQAExport?.addEventListener("click", exportBatchOutputQACSV);
elements.batchCleanupSelect?.addEventListener("click", runBatchCleanupReport);
elements.batchCleanupCreateCopies?.addEventListener("click", createBatchCleanupCopies);
elements.batchConvertSelect?.addEventListener("click", runBatchConvertExport);
elements.batchConvertRun?.addEventListener("click", runBatchConvertExport);

registerProgressListener();
multiSimulationTool?.loadEnvironment();
switchBatchTab(window.location.hash.replace(/^#/, "") || state.activeBatchTool, { updateHash: false });

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

function switchBatchTab(toolID, { updateHash = true } = {}) {
  const panel = [...elements.batchPanels].find((item) => item.dataset.batchPanel === toolID);
  if (!panel) {
    toolID = "batch-summary";
  }
  state.activeBatchTool = toolID;
  elements.batchNavButtons.forEach((button) => {
    const active = button.dataset.batchTab === toolID;
    button.classList.toggle("active", active);
    button.setAttribute("aria-current", active ? "page" : "false");
  });
  elements.batchPanels.forEach((item) => {
    item.classList.toggle("active", item.dataset.batchPanel === toolID);
  });
  if (updateHash) {
    window.history.replaceState(null, "", `#${toolID}`);
  }
}
