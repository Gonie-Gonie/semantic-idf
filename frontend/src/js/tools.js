import { loadAndApplyAppSettings } from "./settings-client.js";
import { renderAppInfo } from "./app-info.js";
import { t } from "./i18n.js";

loadAndApplyAppSettings();
renderAppInfo();

const state = {
  activeTool: "multi-idf-summary",
  result: null,
  activeRunID: "",
  orientation: "metrics",
  running: false,
  progressFiles: new Map(),
  progressListenerRegistered: false,
  cleanup: null,
  cleanupSelectedRuleIDs: new Set(),
  cleanupExcludedCandidateKeys: new Set(),
  cleanupCandidateFilter: "",
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

const currentDocumentStorageKey = "idfAnalyzer.currentDocument";
const cleanupRuleCompactFormatting = "compact_formatting";

const elements = {
  toolNavButtons: document.querySelectorAll("[data-tool-tab]"),
  toolPanels: document.querySelectorAll("[data-tool-panel]"),
  selectButton: document.querySelector("#multiSummarySelect"),
  exportButton: document.querySelector("#multiSummaryExport"),
  stats: document.querySelector("#multiSummaryStats"),
  status: document.querySelector("#multiSummaryStatus"),
  percent: document.querySelector("#multiSummaryPercent"),
  progressBar: document.querySelector("#multiSummaryProgressBar"),
  fileList: document.querySelector("#multiSummaryFiles"),
  table: document.querySelector("#multiSummaryTable"),
  orientationButtons: document.querySelectorAll("[data-summary-orientation]"),
  cleanupSave: document.querySelector("#cleanupSave"),
  cleanupSaveAs: document.querySelector("#cleanupSaveAs"),
  cleanupDocumentLabel: document.querySelector("#cleanupDocumentLabel"),
  cleanupRules: document.querySelector("#cleanupRules"),
  cleanupCandidateFilter: document.querySelector("#cleanupCandidateFilter"),
  cleanupCandidateStats: document.querySelector("#cleanupCandidateStats"),
  cleanupCandidates: document.querySelector("#cleanupCandidates"),
  multiSimulationSelectFiles: document.querySelector("#multiSimulationSelectFiles"),
  multiSimulationSelectFolder: document.querySelector("#multiSimulationSelectFolder"),
  multiSimulationRun: document.querySelector("#multiSimulationRun"),
  multiSimulationEnergyPlus: document.querySelector("#multiSimulationEnergyPlus"),
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
  const progress = Array.isArray(payload) ? payload[0] : payload;
  if (!progress || progress.runId !== state.multiSimulation.activeRunID) {
    return;
  }
  updateMultiSimulationProgress(progress.completed || 0, progress.total || 0, progress.message || "", progress.status || "running");
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
    const response = await fetch("/api/multi-idf-summary", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ runId: runID }),
    });
    if (response.ok) {
      return response.json();
    }
    responseError = await response.text();
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
  const metrics = result?.metrics || [];
  const files = result?.files || [];
  if (!metrics.length || !files.length) {
    elements.table.innerHTML = `<div class="empty">${t("tools.noSummaryData")}</div>`;
    return;
  }
  elements.table.innerHTML = state.orientation === "files" ? renderFilesAsRows(metrics, files) : renderMetricsAsRows(metrics, files);
}

function renderMetricsAsRows(metrics, files) {
  return `
    <table class="tool-table">
      <thead>
        <tr>
          <th class="tool-sticky-col">${t("common.name")}</th>
          ${files.map((file) => `<th>${renderFileLabel(file)}</th>`).join("")}
        </tr>
      </thead>
      <tbody>
        ${metrics
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
  return `
    <table class="tool-table">
      <thead>
        <tr>
          <th class="tool-sticky-col">${t("common.building")}</th>
          ${metrics.map((metric) => `<th>${escapeHTML(metric.csvName || metric.id)}</th>`).join("")}
        </tr>
      </thead>
      <tbody>
        ${files
          .map((file) => `
            <tr>
              <th class="tool-sticky-col">
                ${renderFileLabel(file)}
                ${file.status === "ok" ? "" : `<span>${escapeHTML(file.error || t("tools.failed"))}</span>`}
              </th>
              ${metrics.map((metric) => renderValueCell(file, metric.id)).join("")}
            </tr>`)
          .join("")}
      </tbody>
    </table>`;
}

function renderFileLabel(file) {
  const label = file.label || file.filename || t("common.inputFile");
  const detail = file.filename && file.filename !== label ? file.filename : "";
  return `<strong>${escapeHTML(label)}</strong>${detail ? `<span>${escapeHTML(detail)}</span>` : ""}`;
}

function renderValueCell(file, metricID) {
  if (file.status !== "ok") {
    return `<td class="tool-value error"></td>`;
  }
  const value = file.metricValues?.[metricID];
  const status = value?.status || "missing";
  return `<td class="tool-value ${escapeHTML(status)}" title="${escapeHTML(status)}">${escapeHTML(value?.displayValue ?? t("common.notAvailable"))}</td>`;
}

function metricValueForCSV(file, metricID) {
  if (file.status !== "ok") {
    return "";
  }
  return file.metricValues?.[metricID]?.displayValue ?? "N/A";
}

function exportCSV() {
  const result = state.result;
  if (!result) {
    return;
  }
  const metrics = result.metrics || [];
  const files = result.files || [];
  const rows =
    state.orientation === "files"
      ? [["building", ...metrics.map((metric) => metric.csvName || metric.id)], ...files.map((file) => [file.label || file.filename, ...metrics.map((metric) => metricValueForCSV(file, metric.id))])]
      : [["name", ...files.map((file) => file.label || file.filename)], ...metrics.map((metric) => [metric.csvName || metric.id, ...files.map((file) => metricValueForCSV(file, metric.id))])];
  const csvText = `${rows.map((row) => row.map(csvCell).join(",")).join("\r\n")}\r\n`;
  const blob = new Blob([csvText], { type: "text/csv" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `multi-idf-summary-${state.orientation}.csv`;
  link.click();
  URL.revokeObjectURL(url);
}

function csvCell(value) {
  const text = String(value ?? "");
  return /[",\r\n]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text;
}

async function loadToolSimulationEnvironment() {
  try {
    const api = await waitForAppAPI("GetSimulationEnvironment");
    state.simulationEnvironment = api
      ? await api.GetSimulationEnvironment()
      : await fetch("/api/simulation-environment").then((response) => (response.ok ? response.json() : null));
    renderMultiSimulationEnvironment();
  } catch {
    state.simulationEnvironment = null;
    renderMultiSimulationEnvironment();
  }
}

function renderMultiSimulationEnvironment() {
  if (!elements.multiSimulationEnergyPlus) {
    return;
  }
  const installs = state.simulationEnvironment?.installations || [];
  const currentInstall = elements.multiSimulationEnergyPlus.value;
  elements.multiSimulationEnergyPlus.innerHTML = installs.length
    ? installs
        .map((install) => {
          const label = `${install.name || "EnergyPlus"}${install.autoDetected ? " · auto" : ""}`;
          return `<option value="${escapeHTML(install.executablePath)}" title="${escapeHTML(install.executablePath)}">${escapeHTML(label)}</option>`;
        })
        .join("")
    : `<option value="">${escapeHTML(t("simulation.noEnergyPlus", {}, "No EnergyPlus installation"))}</option>`;
  if (currentInstall && [...elements.multiSimulationEnergyPlus.options].some((option) => option.value === currentInstall)) {
    elements.multiSimulationEnergyPlus.value = currentInstall;
  }

  const currentWeather = elements.multiSimulationWeather.value;
  const weatherHTML = [`<option value="">${escapeHTML(t("simulation.noWeather", {}, "No weather / design-day only"))}</option>`];
  for (const folder of state.simulationEnvironment?.weatherFolders || []) {
    weatherHTML.push(`<optgroup label="${escapeHTML(`${folder.source || "Weather"} · ${folder.label || folder.path}`)}">`);
    for (const file of folder.files || []) {
      weatherHTML.push(`<option value="${escapeHTML(file.path)}" title="${escapeHTML(file.path)}">${escapeHTML(file.name)}</option>`);
    }
    weatherHTML.push("</optgroup>");
  }
  elements.multiSimulationWeather.innerHTML = weatherHTML.join("");
  if (currentWeather && [...elements.multiSimulationWeather.options].some((option) => option.value === currentWeather)) {
    elements.multiSimulationWeather.value = currentWeather;
  }
  const defaultWorkers = state.simulationEnvironment?.defaultWorkerCount || 0;
  if (elements.multiSimulationWorkers && Number(elements.multiSimulationWorkers.value || 0) === 0 && defaultWorkers > 0) {
    elements.multiSimulationWorkers.placeholder = String(defaultWorkers);
  }
}

async function selectMultiSimulationFiles() {
  const api = await waitForAppAPI("SelectSimulationInputFiles");
  if (!api) {
    elements.multiSimulationStatus.textContent = t("tools.desktopOnly");
    return;
  }
  const result = await api.SelectSimulationInputFiles();
  if (!result || result.canceled) {
    elements.multiSimulationStatus.textContent = t("status.fileSelectionCanceled");
    return;
  }
  updateMultiSimulationSelection(result.paths || [], result.rootDirectory || "");
}

async function selectMultiSimulationFolder() {
  const api = await waitForAppAPI("SelectSimulationInputFolder");
  if (!api) {
    elements.multiSimulationStatus.textContent = t("tools.desktopOnly");
    return;
  }
  const recursive = Boolean(elements.multiSimulationRecursive?.checked);
  const result = await api.SelectSimulationInputFolder(recursive);
  if (!result || result.canceled) {
    elements.multiSimulationStatus.textContent = t("status.fileSelectionCanceled");
    return;
  }
  updateMultiSimulationSelection(result.paths || [], result.rootDirectory || "");
}

function updateMultiSimulationSelection(paths, rootDirectory = "") {
  state.multiSimulation.selectedPaths = [...new Set((paths || []).filter(Boolean))].sort();
  state.multiSimulation.rootDirectory = rootDirectory || "";
  state.multiSimulation.result = null;
  state.multiSimulation.selectedRows.clear();
  state.multiSimulation.metric = "";
  elements.multiSimulationRun.disabled = !state.multiSimulation.selectedPaths.length || state.multiSimulation.running;
  elements.multiSimulationStats.textContent = t("tools.simulationFilesSelected", { count: state.multiSimulation.selectedPaths.length }, `${state.multiSimulation.selectedPaths.length} files selected`);
  elements.multiSimulationStatus.textContent = t("tools.readyToRun", {}, "Ready to run");
  updateMultiSimulationProgress(0, state.multiSimulation.selectedPaths.length, "", "idle");
  renderMultiSimulationSelectedFiles();
  renderMultiSimulationResult();
}

function renderMultiSimulationSelectedFiles() {
  const paths = state.multiSimulation.selectedPaths || [];
  if (!paths.length) {
    elements.multiSimulationFiles.innerHTML = "";
    return;
  }
  elements.multiSimulationFiles.innerHTML = paths
    .slice(0, 80)
    .map(
      (path) => `
        <div class="tool-file-item">
          <strong>${escapeHTML(fileName(path))}</strong>
          <span title="${escapeHTML(path)}">${escapeHTML(path)}</span>
        </div>`,
    )
    .join("");
  if (paths.length > 80) {
    elements.multiSimulationFiles.insertAdjacentHTML("beforeend", `<div class="tool-muted">${escapeHTML(t("tools.moreFiles", { count: paths.length - 80 }, `${paths.length - 80} more files`))}</div>`);
  }
}

async function runMultiSimulation() {
  const paths = state.multiSimulation.selectedPaths || [];
  if (!paths.length || state.multiSimulation.running) {
    return;
  }
  await loadToolSimulationEnvironment();
  const executablePath = elements.multiSimulationEnergyPlus?.value || "";
  if (!executablePath) {
    elements.multiSimulationStatus.textContent = t("simulation.registerEnergyPlus", {}, "Register EnergyPlus in Settings");
    return;
  }
  state.multiSimulation.activeRunID = `multi-sim-${Date.now()}-${Math.random().toString(36).slice(2)}`;
  state.multiSimulation.running = true;
  elements.multiSimulationRun.disabled = true;
  elements.multiSimulationTable.innerHTML = `<div class="empty status-loading">${escapeHTML(t("tools.simulationRunning", {}, "EnergyPlus batch is running"))}</div>`;
  updateMultiSimulationProgress(0, paths.length, t("tools.simulationRunning", {}, "EnergyPlus batch is running"), "running");
  waitForProgressRuntime();
  try {
    const request = {
      runId: state.multiSimulation.activeRunID,
      inputPaths: paths,
      rootDirectory: state.multiSimulation.rootDirectory || "",
      recursive: Boolean(elements.multiSimulationRecursive?.checked),
      energyPlusExecutablePath: executablePath,
      weatherMode: elements.multiSimulationWeatherMode?.value || "same",
      weatherPath: elements.multiSimulationWeather?.value || "",
      workerCount: Number(elements.multiSimulationWorkers?.value || 0),
    };
    const result = await callMultiSimulationRun(request);
    state.multiSimulation.result = result;
    state.multiSimulation.selectedRows = new Set((result.results || []).filter((item) => item.status === "succeeded").slice(0, 12).map(rowID));
    state.multiSimulation.metric = firstSimulationMetric(result);
    updateMultiSimulationProgress(result.completed || 0, result.total || paths.length, t("tools.simulationComplete", {}, "Batch simulation complete"), "complete");
    renderMultiSimulationResult();
  } catch (error) {
    elements.multiSimulationStatus.textContent = error?.message || String(error);
    elements.multiSimulationTable.innerHTML = `<div class="empty">${escapeHTML(error?.message || String(error))}</div>`;
  } finally {
    state.multiSimulation.running = false;
    elements.multiSimulationRun.disabled = !state.multiSimulation.selectedPaths.length;
  }
}

async function callMultiSimulationRun(request) {
  const api = await waitForAppAPI("RunMultipleSimulations");
  if (api) {
    return api.RunMultipleSimulations(request);
  }
  return postJSON("/api/multi-simulation-run", request);
}

function updateMultiSimulationProgress(completed, total, message = "", status = "running") {
  const percent = total > 0 ? Math.round((completed / total) * 100) : 0;
  if (elements.multiSimulationProgressBar) {
    elements.multiSimulationProgressBar.style.width = `${percent}%`;
  }
  if (elements.multiSimulationPercent) {
    elements.multiSimulationPercent.textContent = `${percent}%`;
  }
  if (elements.multiSimulationStatus) {
    elements.multiSimulationStatus.textContent = message || (total ? `${completed} / ${total}` : t("tools.waitingFiles"));
  }
  elements.multiSimulationStatus?.classList.toggle("status-loading", status === "running" && total > 0 && completed < total);
}

function renderMultiSimulationResult() {
  const result = state.multiSimulation.result;
  if (!result) {
    elements.multiSimulationMetric.innerHTML = `<option value="">${escapeHTML(t("simulation.noSeries", {}, "No CSV series"))}</option>`;
    elements.multiSimulationChart.innerHTML = `<div class="empty">${escapeHTML(t("tools.noSimulationResult", {}, "Run the selected files to compare simulation output."))}</div>`;
    elements.multiSimulationTable.innerHTML = state.multiSimulation.selectedPaths.length
      ? `<div class="empty">${escapeHTML(t("tools.readyToRun", {}, "Ready to run"))}</div>`
      : `<div class="empty">${escapeHTML(t("tools.selectSimulationFilesHelp", {}, "Select files or a folder to prepare batch simulation."))}</div>`;
    return;
  }
  const total = result.total || 0;
  const succeeded = result.succeeded || 0;
  const failed = result.failed || 0;
  elements.multiSimulationStats.textContent = t("tools.simulationResultStats", { total, succeeded, failed, workers: result.workers || 0 }, `${total} runs, ${succeeded} succeeded, ${failed} failed`);
  renderMultiSimulationMetricSelect(result);
  renderMultiSimulationChart(result);
  renderMultiSimulationTable(result);
}

function renderMultiSimulationMetricSelect(result) {
  const metrics = uniqueSimulationMetrics(result);
  if (!metrics.length) {
    state.multiSimulation.metric = "";
    elements.multiSimulationMetric.innerHTML = `<option value="">${escapeHTML(t("simulation.noSeries", {}, "No CSV series"))}</option>`;
    return;
  }
  if (!state.multiSimulation.metric || !metrics.includes(state.multiSimulation.metric)) {
    state.multiSimulation.metric = metrics[0];
  }
  elements.multiSimulationMetric.innerHTML = metrics
    .map((metric) => `<option value="${escapeHTML(metric)}" ${metric === state.multiSimulation.metric ? "selected" : ""}>${escapeHTML(metric)}</option>`)
    .join("");
}

function renderMultiSimulationTable(result) {
  const rows = sortedSimulationResults(result.results || []);
  elements.multiSimulationTable.innerHTML = `
    <table class="tool-table">
      <thead>
        <tr>
          <th>${escapeHTML(t("common.view", {}, "View"))}</th>
          <th class="tool-sticky-col">${escapeHTML(t("common.name"))}</th>
          <th>${escapeHTML(t("common.status", {}, "Status"))}</th>
          <th>${escapeHTML(t("simulation.errWarnings", {}, "ERR warnings"))}</th>
          <th>${escapeHTML(t("simulation.errSevere", {}, "Severe/Fatal"))}</th>
          <th>${escapeHTML(t("simulation.csvFiles", {}, "CSV files"))}</th>
          <th>${escapeHTML(t("tools.duration", {}, "Duration"))}</th>
          <th>${escapeHTML(t("simulation.weather", {}, "Weather"))}</th>
        </tr>
      </thead>
      <tbody>
        ${rows
          .map((item) => {
            const id = rowID(item);
            return `
              <tr>
                <td><input data-multi-sim-row="${escapeHTML(id)}" type="checkbox" ${state.multiSimulation.selectedRows.has(id) ? "checked" : ""} ${item.series?.length ? "" : "disabled"} /></td>
                <th class="tool-sticky-col">
                  <strong>${escapeHTML(item.filename || fileName(item.inputPath))}</strong>
                  <span title="${escapeHTML(item.outputDirectory || "")}">${escapeHTML(item.error || item.outputDirectory || "")}</span>
                </th>
                <td class="tool-value ${escapeHTML(item.status || "")}">${escapeHTML(item.status || "")}</td>
                <td>${escapeHTML(item.err?.warnings || 0)}</td>
                <td>${escapeHTML((item.err?.severe || 0) + (item.err?.fatal || 0))}</td>
                <td>${escapeHTML(item.csvs?.length || 0)}</td>
                <td>${escapeHTML(formatDuration(item.durationMs || 0))}</td>
                <td title="${escapeHTML(item.weatherPath || "")}">${escapeHTML(fileName(item.weatherPath) || t("common.notAvailable"))}</td>
              </tr>`;
          })
          .join("")}
      </tbody>
    </table>`;
  elements.multiSimulationTable.querySelectorAll("[data-multi-sim-row]").forEach((input) => {
    input.addEventListener("change", () => {
      if (input.checked) {
        state.multiSimulation.selectedRows.add(input.dataset.multiSimRow);
      } else {
        state.multiSimulation.selectedRows.delete(input.dataset.multiSimRow);
      }
      renderMultiSimulationChart(result);
    });
  });
}

function renderMultiSimulationChart(result) {
  const metric = state.multiSimulation.metric;
  const selected = (result.results || [])
    .filter((item) => state.multiSimulation.selectedRows.has(rowID(item)))
    .map((item) => ({ result: item, series: (item.series || []).find((series) => series.column === metric) }))
    .filter((item) => item.series?.points?.length)
    .slice(0, 20);
  if (!metric || !selected.length) {
    elements.multiSimulationChart.innerHTML = `<div class="empty">${escapeHTML(t("tools.selectMetricRows", {}, "Select a metric and result rows to overlay graph lines."))}</div>`;
    return;
  }
  const values = selected.flatMap((item) => item.series.points.map((point) => Number(point.value)).filter(Number.isFinite));
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;
  const width = 900;
  const height = 280;
  const pad = { left: 76, right: 18, top: 24, bottom: 46 };
  const colors = ["#007c89", "#b3261e", "#246b44", "#a85f00", "#5b5fc7", "#8b5a2b", "#008a5c", "#c44569"];
  const yFor = (value) => pad.top + (height - pad.top - pad.bottom) * (1 - (value - min) / range);
  const yTicks = [max, min + range / 2, min]
    .map((value) => {
      const y = yFor(value);
      return `<g><line x1="${pad.left}" x2="${width - pad.right}" y1="${y}" y2="${y}" class="simulation-grid" /><text x="8" y="${y + 4}" class="simulation-axis">${escapeHTML(formatNumber(value))}</text></g>`;
    })
    .join("");
  const lines = selected
    .map((item, index) => {
      const points = item.series.points;
      const xStep = points.length > 1 ? (width - pad.left - pad.right) / (points.length - 1) : 1;
      const polyline = points.map((point, pointIndex) => `${pad.left + pointIndex * xStep},${yFor(Number(point.value))}`).join(" ");
      const color = colors[index % colors.length];
      return `<polyline points="${polyline}" fill="none" stroke="${color}" stroke-width="1.8" stroke-linejoin="round" />`;
    })
    .join("");
  const legend = selected
    .map((item, index) => {
      const x = pad.left + (index % 4) * 190;
      const y = height - 28 + Math.floor(index / 4) * 14;
      const color = colors[index % colors.length];
      return `<g><rect x="${x}" y="${y - 8}" width="9" height="9" fill="${color}" /><text x="${x + 14}" y="${y}" class="simulation-axis">${escapeHTML(item.result.filename || fileName(item.result.inputPath))}</text></g>`;
    })
    .join("");
  elements.multiSimulationChart.innerHTML = `
    <svg class="simulation-svg" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(metric)}">
      ${yTicks}
      <line x1="${pad.left}" x2="${pad.left}" y1="${pad.top}" y2="${height - pad.bottom}" class="simulation-axis-line" />
      <line x1="${pad.left}" x2="${width - pad.right}" y1="${height - pad.bottom}" y2="${height - pad.bottom}" class="simulation-axis-line" />
      ${lines}
      <text x="${pad.left}" y="16" class="simulation-title">${escapeHTML(metric)} (${selected.length} selected, max 20)</text>
      ${legend}
    </svg>`;
}

function sortedSimulationResults(results) {
  const rows = results.slice();
  const key = state.multiSimulation.sort || "filename";
  rows.sort((a, b) => {
    if (key === "warnings") {
      return (b.err?.warnings || 0) - (a.err?.warnings || 0);
    }
    if (key === "severe") {
      return (b.err?.severe || 0) + (b.err?.fatal || 0) - ((a.err?.severe || 0) + (a.err?.fatal || 0));
    }
    if (key === "duration") {
      return (b.durationMs || 0) - (a.durationMs || 0);
    }
    if (key === "status") {
      return String(a.status || "").localeCompare(String(b.status || ""));
    }
    return String(a.filename || a.inputPath || "").localeCompare(String(b.filename || b.inputPath || ""));
  });
  return rows;
}

function uniqueSimulationMetrics(result) {
  const seen = new Set();
  for (const item of result?.results || []) {
    for (const series of item.series || []) {
      if (series.column) {
        seen.add(series.column);
      }
    }
  }
  return [...seen].sort((a, b) => a.localeCompare(b));
}

function firstSimulationMetric(result) {
  return uniqueSimulationMetrics(result)[0] || "";
}

function rowID(item) {
  return item.runId || item.inputPath || item.filename || "";
}

function fileName(path) {
  const text = String(path || "");
  return text.split(/[\\/]/).filter(Boolean).pop() || "";
}

function formatDuration(ms) {
  const value = Number(ms || 0);
  if (value < 1000) {
    return `${value} ms`;
  }
  return `${(value / 1000).toFixed(1)} s`;
}

function formatNumber(value) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return "N/A";
  }
  if (Math.abs(number) >= 10000 || (Math.abs(number) > 0 && Math.abs(number) < 0.001)) {
    return number.toExponential(2);
  }
  return number.toLocaleString(undefined, { maximumFractionDigits: 3 });
}

elements.selectButton.addEventListener("click", runMultiSummary);
elements.exportButton.addEventListener("click", exportCSV);
elements.multiSimulationSelectFiles?.addEventListener("click", selectMultiSimulationFiles);
elements.multiSimulationSelectFolder?.addEventListener("click", selectMultiSimulationFolder);
elements.multiSimulationRun?.addEventListener("click", runMultiSimulation);
elements.multiSimulationMetric?.addEventListener("change", () => {
  state.multiSimulation.metric = elements.multiSimulationMetric.value || "";
  if (state.multiSimulation.result) {
    renderMultiSimulationChart(state.multiSimulation.result);
  }
});
elements.multiSimulationSort?.addEventListener("change", () => {
  state.multiSimulation.sort = elements.multiSimulationSort.value || "filename";
  if (state.multiSimulation.result) {
    renderMultiSimulationTable(state.multiSimulation.result);
  }
});
elements.toolNavButtons.forEach((button) => {
  button.addEventListener("click", () => switchToolTab(button.dataset.toolTab));
});
elements.orientationButtons.forEach((button) => {
  button.addEventListener("click", () => {
    state.orientation = button.dataset.summaryOrientation;
    elements.orientationButtons.forEach((item) => item.classList.toggle("active", item === button));
    renderTable();
  });
});

registerProgressListener();
loadToolSimulationEnvironment();
switchToolTab(window.location.hash.replace(/^#/, "") || state.activeTool, { updateHash: false });
loadCleanupFromCurrentDocument();

elements.cleanupSave.addEventListener("click", () => saveCleanup(false));
elements.cleanupSaveAs.addEventListener("click", () => saveCleanup(true));
elements.cleanupCandidateFilter.addEventListener("input", () => {
  state.cleanupCandidateFilter = elements.cleanupCandidateFilter.value;
  renderCleanupCandidates();
});

async function loadCleanupFromCurrentDocument() {
  const currentDocument = readCurrentDocument();
  if (!currentDocument) {
    renderMissingCleanupDocument();
    return;
  }
  setCleanupBusy(true);
  elements.cleanupDocumentLabel.textContent = t("tools.currentInput", { name: currentDocument.filename || t("tools.untitledInput") });
  try {
    const result = await postJSON("/api/cleanup-scan", {
      text: currentDocument.text || "",
      path: currentDocument.path || "",
      filename: currentDocument.filename || "",
    });
    state.cleanup = result;
    initializeCleanupSelection(result);
    renderCleanupScan();
  } catch (error) {
    elements.cleanupDocumentLabel.textContent = error?.message || String(error);
  } finally {
    setCleanupBusy(false);
  }
}

async function buildCleanupPreview() {
  if (!state.cleanup) {
    return null;
  }
  return postJSON("/api/cleanup-preview", cleanupPayload());
}

async function saveCleanup(saveAs) {
  if (!state.cleanup) {
    return;
  }
  setCleanupBusy(true);
  try {
    const preview = await buildCleanupPreview();
    const endpoint = saveAs || !state.cleanup?.path ? "/api/cleanup-save-as" : "/api/cleanup-save";
    const result = await postJSON(endpoint, {
      ...cleanupPayload(),
      text: state.cleanup.text,
      path: state.cleanup.path || "",
      suggestedFilename: saveAs ? saveAsFilename(state.cleanup.filename) : state.cleanup.filename || "cleaned.idf",
    });
    if (result?.canceled) {
      return;
    }
    updateCurrentDocument({
      text: result.text || preview?.text || state.cleanup.text,
      path: result.path || state.cleanup.path || "",
      filename: result.filename || state.cleanup.filename || "",
    });
    await loadCleanupFromCurrentDocument();
    elements.cleanupDocumentLabel.textContent = t("tools.savedCleanup", {
      name: result.filename || state.cleanup?.filename || t("common.inputFile"),
      count: result.removedCount || 0,
    });
  } catch (error) {
    elements.cleanupDocumentLabel.textContent = error?.message || String(error);
  } finally {
    setCleanupBusy(false);
  }
}

function renderCleanupScan() {
  const cleanup = state.cleanup;
  const candidates = cleanup?.scan?.candidates || [];
  const rules = cleanup?.scan?.rules || [];
  elements.cleanupRules.innerHTML = rules.map(renderCleanupRule).join("");
  elements.cleanupRules.querySelectorAll("input[data-cleanup-rule]").forEach((input) => {
    input.addEventListener("change", () => {
      if (input.checked) {
        state.cleanupSelectedRuleIDs.add(input.dataset.cleanupRule);
      } else {
        state.cleanupSelectedRuleIDs.delete(input.dataset.cleanupRule);
      }
      renderCleanupCandidates();
    });
  });
  renderCleanupCandidates();
  updateCleanupButtons();
}

function renderCleanupRule(rule) {
  const disabled = !rule.available ? "disabled" : "";
  const checked = state.cleanupSelectedRuleIDs.has(rule.id) && rule.available ? "checked" : "";
  const status = rule.future ? t("tools.future") : rule.available ? t("tools.available") : t("tools.noCandidates");
  return `
    <label class="cleanup-rule ${rule.available ? "" : "disabled"}">
      <input data-cleanup-rule="${escapeHTML(rule.id)}" type="checkbox" ${checked} ${disabled} />
      <span>
        <strong>${escapeHTML(rule.name)}</strong>
        <small>${escapeHTML(rule.description)}</small>
        <em>${escapeHTML(status)}</em>
      </span>
    </label>`;
}

function renderCleanupCandidates() {
  const candidates = state.cleanup?.scan?.candidates || [];
  const query = state.cleanupCandidateFilter.trim().toLowerCase();
  const visible = candidates.filter((candidate) => cleanupCandidateMatches(candidate, query));
  const selectedCount = selectedCleanupCandidates(candidates).length;
  elements.cleanupCandidateStats.textContent = query
    ? t("tools.selectedShown", { selected: selectedCount, shown: visible.length })
    : t("tools.selectedOf", { selected: selectedCount, total: candidates.length });

  if (!candidates.length) {
    elements.cleanupCandidates.innerHTML = `<div class="empty">${t("tools.noCleanupCandidates")}</div>`;
    updateCleanupButtons();
    return;
  }
  if (!visible.length) {
    elements.cleanupCandidates.innerHTML = `<div class="empty">${t("tools.noMatchingCandidates")}</div>`;
    updateCleanupButtons();
    return;
  }
  elements.cleanupCandidates.innerHTML = `
    <div class="cleanup-candidate-list">
      ${visible.map(renderCleanupCandidate).join("")}
    </div>`;
  elements.cleanupCandidates.querySelectorAll("input[data-cleanup-candidate]").forEach((input) => {
    input.addEventListener("change", () => {
      if (input.checked) {
        state.cleanupExcludedCandidateKeys.delete(input.dataset.cleanupCandidate);
      } else {
        state.cleanupExcludedCandidateKeys.add(input.dataset.cleanupCandidate);
      }
      updateCleanupButtons();
      renderCleanupCandidates();
    });
  });
  updateCleanupButtons();
}

function renderCleanupCandidate(candidate) {
  const ruleSelected = state.cleanupSelectedRuleIDs.has(candidate.ruleId);
  const excluded = state.cleanupExcludedCandidateKeys.has(candidate.key);
  const selected = ruleSelected && !excluded;
  const objectLabel = candidate.objectName || `#${Number(candidate.objectIndex) + 1}`;
  return `
    <label class="cleanup-candidate ${selected ? "selected" : ""} ${ruleSelected ? "" : "inactive"}">
      <input data-cleanup-candidate="${escapeHTML(candidate.key)}" type="checkbox" ${selected ? "checked" : ""} ${ruleSelected ? "" : "disabled"} />
      <span>
        <strong>${escapeHTML(objectLabel)}</strong>
        <small>${escapeHTML(candidate.objectType)} / ${escapeHTML(candidate.ruleId)}</small>
        <em>${escapeHTML(candidate.reason)}</em>
      </span>
    </label>`;
}

function setCleanupBusy(busy) {
  elements.cleanupSave.disabled = busy || !canSaveCleanup();
  elements.cleanupSaveAs.disabled = busy || !canSaveCleanup();
  elements.cleanupCandidateFilter.disabled = busy || !state.cleanup;
}

function renderMissingCleanupDocument() {
  state.cleanup = null;
  state.cleanupSelectedRuleIDs.clear();
  state.cleanupExcludedCandidateKeys.clear();
  elements.cleanupDocumentLabel.textContent = t("tools.noCurrentInput");
  elements.cleanupRules.innerHTML = `<div class="empty">${t("tools.noCurrentInputShort")}</div>`;
  elements.cleanupCandidates.innerHTML = `<div class="empty">${t("tools.noCurrentInputShort")}</div>`;
  elements.cleanupCandidateStats.textContent = t("tools.selectedOf", { selected: 0, total: 0 });
  updateCleanupButtons();
}

function initializeCleanupSelection(result) {
  state.cleanupSelectedRuleIDs = new Set(
    (result?.scan?.rules || []).filter((rule) => rule.default && rule.available).map((rule) => rule.id),
  );
  state.cleanupExcludedCandidateKeys = new Set();
  state.cleanupCandidateFilter = "";
  elements.cleanupCandidateFilter.value = "";
}

function cleanupPayload() {
  return {
    text: state.cleanup?.text || "",
    ruleIds: selectedCleanupRuleIDs(),
    excludedCandidateKeys: [...state.cleanupExcludedCandidateKeys],
  };
}

function selectedCleanupRuleIDs() {
  return [...state.cleanupSelectedRuleIDs];
}

function selectedCleanupCandidates(candidates = state.cleanup?.scan?.candidates || []) {
  return candidates.filter((candidate) => state.cleanupSelectedRuleIDs.has(candidate.ruleId) && !state.cleanupExcludedCandidateKeys.has(candidate.key));
}

function canSaveCleanup() {
  return (
    Boolean(state.cleanup) &&
    (selectedCleanupCandidates().length > 0 || state.cleanupSelectedRuleIDs.has(cleanupRuleCompactFormatting))
  );
}

function updateCleanupButtons() {
  elements.cleanupSave.disabled = !canSaveCleanup();
  elements.cleanupSaveAs.disabled = !canSaveCleanup();
}

function cleanupCandidateMatches(candidate, query) {
  if (!query) {
    return true;
  }
  return [candidate.ruleId, candidate.objectType, candidate.objectName, candidate.reason]
    .some((value) => String(value ?? "").toLowerCase().includes(query));
}

function readCurrentDocument() {
  try {
    const raw = window.sessionStorage.getItem(currentDocumentStorageKey);
    if (!raw) {
      return null;
    }
    const currentDocument = JSON.parse(raw);
    return typeof currentDocument?.text === "string" && currentDocument.text.trim() ? currentDocument : null;
  } catch {
    return null;
  }
}

function updateCurrentDocument(nextDocument) {
  const currentDocument = {
    text: nextDocument.text || "",
    path: nextDocument.path || "",
    filename: nextDocument.filename || "cleaned.idf",
  };
  try {
    window.sessionStorage.setItem(currentDocumentStorageKey, JSON.stringify(currentDocument));
  } catch {
    // Session storage may be unavailable in hardened webview settings.
  }
  state.cleanup = {
    ...state.cleanup,
    ...currentDocument,
  };
}

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

function saveAsFilename(filename = "cleaned.idf") {
  const dot = filename.lastIndexOf(".");
  if (dot <= 0) {
    return `${filename}-cleaned.idf`;
  }
  return `${filename.slice(0, dot)}-cleaned${filename.slice(dot)}`;
}

function switchToolTab(toolID, { updateHash = true } = {}) {
  const panel = [...elements.toolPanels].find((item) => item.dataset.toolPanel === toolID);
  if (!panel) {
    toolID = "multi-idf-summary";
  }
  state.activeTool = toolID;
  elements.toolNavButtons.forEach((button) => {
    const active = button.dataset.toolTab === toolID;
    button.classList.toggle("active", active);
    button.setAttribute("aria-current", active ? "page" : "false");
  });
  elements.toolPanels.forEach((item) => {
    item.classList.toggle("active", item.dataset.toolPanel === toolID);
  });
  if (updateHash) {
    window.history.replaceState(null, "", `#${toolID}`);
  }
}
