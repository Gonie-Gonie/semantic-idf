import { backend, elements, escapeHTML, setStatus, state } from "./state.js";
import { t } from "./i18n.js";
import { openStandardOutputApplyDialog } from "./output-views.js";

let progressListenerRegistered = false;

export function initializeSimulationControls() {
  elements.simulationApplyStandardOutput?.addEventListener("click", () => openStandardOutputApplyDialog());
  elements.simulationRefreshEnv?.addEventListener("click", () => loadSimulationEnvironment());
  elements.simulationRunButton?.addEventListener("click", () => runCurrentSimulation({ silent: false }));
  elements.simulationEnergyPlusSelect?.addEventListener("change", () => renderSimulation());
  elements.simulationWeatherSelect?.addEventListener("change", () => renderSimulation());
  elements.simulationStandardOutput?.addEventListener("change", () => {
    state.simulationStandardOutput = elements.simulationStandardOutput.checked;
    renderSimulation();
  });
  elements.simulationSeriesSelect?.addEventListener("change", () => {
    state.simulationSelectedSeries = elements.simulationSeriesSelect.value || "";
    renderSimulationChart();
  });
  elements.simulationAutoRunOnOpen?.addEventListener("change", () => {
    state.simulationAutoRunOnOpen = elements.simulationAutoRunOnOpen.checked;
  });
  window.addEventListener("idfAnalyzer:simulationProgress", (event) => handleSimulationProgress(event.detail));
  window.addEventListener("idfAnalyzer:documentChanged", () => {
    if (state.simulationResult && state.simulationRunText !== elements.idfInput.value) {
      state.simulationStale = true;
    }
    updateSimulationControls();
    renderSimulation();
  });
  window.addEventListener("idfAnalyzer:analysisComplete", () => maybeAutoRunSimulation());
  window.addEventListener("idfAnalyzer:settingsChanged", (event) => {
    state.simulationAutoRunOnOpen = event.detail?.settings?.simulation?.autoRunOnOpen ?? state.simulationAutoRunOnOpen;
    loadSimulationEnvironment();
  });
  waitForProgressRuntime();
  loadSimulationEnvironment();
  renderSimulationEmpty();
}

export async function loadSimulationEnvironment() {
  try {
    const env = await callSimulationAPI("GetSimulationEnvironment", "/api/simulation-environment");
    state.simulationEnvironment = env;
    state.simulationAutoRunOnOpen = env?.settings?.autoRunOnOpen ?? state.simulationAutoRunOnOpen;
    if (elements.simulationAutoRunOnOpen) {
      elements.simulationAutoRunOnOpen.checked = Boolean(state.simulationAutoRunOnOpen);
    }
    renderSimulationEnvironment();
    renderSimulation();
    return env;
  } catch (error) {
    if (elements.simulationStatus) {
      elements.simulationStatus.textContent = error.message || String(error);
    }
    return null;
  }
}

export function renderSimulation() {
  renderSimulationEnvironment();
  renderSimulationProgress();
  updateSimulationControls();
  if (!state.simulationResult) {
    renderSimulationEmpty();
    return;
  }
  const result = state.simulationResult;
  const stale = state.simulationStale;
  const err = result.err || {};
  const csvCount = result.csvs?.length || 0;
  const issueCount = err.total || 0;
  const statusLabel = statusText(result.status);
  elements.simulationStats.textContent = stale
    ? t("simulation.staleStats", { status: statusLabel }, `${statusLabel}, stale`)
    : t("simulation.stats", { status: statusLabel, warnings: err.warnings || 0, severe: err.severe || 0 }, `${statusLabel}, ${err.warnings || 0} warnings`);
  if (state.simulationRunning) {
    elements.simulationStats.textContent = t("simulation.runningStats", {}, "Simulation running in background");
  }
  elements.simulationResultMeta.textContent = `${result.filename || "current input"} - ${formatDuration(result.durationMs || 0)} - ${csvCount} CSV - ${issueCount} ERR issues`;
  elements.simulationResultSummary.innerHTML = `${state.simulationRunning ? renderRunningNotice() : ""}${renderSimulationSummary(result, stale)}`;
  renderSimulationSeriesSelect(result);
  renderSimulationChart();
  renderSimulationFiles(result);
}

function renderSimulationEmpty() {
  if (!elements.simulationStats) {
    return;
  }
  updateSimulationControls();
  const installCount = state.simulationEnvironment?.installations?.length || 0;
  const hasText = Boolean((elements.idfInput?.value || "").trim());
  const blockingIssue = simulationBlockingIssue();
  if (state.simulationRunning) {
    elements.simulationStats.textContent = t("simulation.runningStats", {}, "Simulation running in background");
    elements.simulationResultMeta.textContent = t("simulation.backgroundRun", {}, "EnergyPlus is running in the background");
    elements.simulationResultSummary.innerHTML = renderRunningNotice();
    elements.simulationSeriesSelect.innerHTML = `<option value="">${escapeHTML(t("simulation.waitingForCSV", {}, "Waiting for CSV output"))}</option>`;
    elements.simulationChart.innerHTML = `<div class="simulation-running-empty">${renderMiniProgressSVG()}<span>${escapeHTML(t("simulation.graphAfterRun", {}, "The CSV graph will appear when the run finishes."))}</span></div>`;
    elements.simulationFiles.innerHTML = `<div class="empty status-loading">${escapeHTML(t("simulation.writingOutputs", {}, "EnergyPlus is writing output files"))}</div>`;
    return;
  }
  if (blockingIssue) {
    elements.simulationStats.textContent = t("simulation.blockedStats", {}, "Cannot run");
    elements.simulationStatus.textContent = blockingIssue.title;
    elements.simulationStatus.classList.remove("status-loading");
    elements.simulationPercent.textContent = "0%";
    elements.simulationProgressBar.style.width = "0%";
    elements.simulationResultMeta.textContent = t("simulation.blockedMeta", {}, "Run requirements need attention");
    elements.simulationResultSummary.innerHTML = renderSimulationBlocker(blockingIssue);
    elements.simulationSeriesSelect.innerHTML = `<option value="">${escapeHTML(t("simulation.noSeries", {}, "No CSV series"))}</option>`;
    elements.simulationChart.innerHTML = `<div class="simulation-blocked-empty">${escapeHTML(t("simulation.blockedGraph", {}, "Graph output is unavailable until the run can start."))}</div>`;
    elements.simulationFiles.innerHTML = `<div class="simulation-blocked-empty">${escapeHTML(t("simulation.blockedFiles", {}, "No output files will be created while simulation is blocked."))}</div>`;
    return;
  }
  const idleMessage = hasText
    ? t("simulation.idle", {}, "Ready")
    : t("simulation.noInputText", {}, "Open or paste an IDF before running simulation");
  elements.simulationStats.textContent = installCount
    ? idleMessage
    : t("simulation.noEnergyPlus", {}, "No EnergyPlus installation");
  elements.simulationStatus.textContent = installCount
    ? idleMessage
    : t("simulation.registerEnergyPlus", {}, "Register EnergyPlus in Settings");
  elements.simulationStatus.classList.remove("status-loading");
  elements.simulationPercent.textContent = "0%";
  elements.simulationProgressBar.style.width = "0%";
  elements.simulationResultMeta.textContent = "ERR / CSV / output files";
  elements.simulationResultSummary.innerHTML = `<div class="empty">${t("simulation.noResult", {}, "Run a simulation to inspect ERR and CSV outputs.")}</div>`;
  elements.simulationSeriesSelect.innerHTML = `<option value="">${escapeHTML(t("simulation.noSeries", {}, "No CSV series"))}</option>`;
  elements.simulationChart.innerHTML = `<div class="empty">${t("simulation.noGraph", {}, "CSV graph will appear after a run with numeric output.")}</div>`;
  elements.simulationFiles.innerHTML = `<div class="empty">${t("simulation.noFiles", {}, "No output files yet")}</div>`;
}

function renderSimulationEnvironment() {
  if (!elements.simulationEnergyPlusSelect || !elements.simulationWeatherSelect) {
    return;
  }
  const currentInstall = elements.simulationEnergyPlusSelect.value;
  const currentWeather = elements.simulationWeatherSelect.value;
  const installs = state.simulationEnvironment?.installations || [];
  elements.simulationEnergyPlusSelect.innerHTML = installs.length
    ? installs
        .map((install) => {
          const label = `${install.name || "EnergyPlus"}${install.autoDetected ? " - auto" : ""}`;
          return `<option value="${escapeHTML(install.executablePath)}" title="${escapeHTML(install.executablePath)}">${escapeHTML(label)}</option>`;
        })
        .join("")
    : `<option value="">${escapeHTML(t("simulation.noEnergyPlus", {}, "No EnergyPlus installation"))}</option>`;
  const recommendedInstallPath = recommendedEnergyPlusInstallPath(installs, currentInstall);
  if (recommendedInstallPath && [...elements.simulationEnergyPlusSelect.options].some((option) => option.value === recommendedInstallPath)) {
    elements.simulationEnergyPlusSelect.value = recommendedInstallPath;
  }

  const folders = state.simulationEnvironment?.weatherFolders || [];
  const weatherHTML = [`<option value="">${escapeHTML(t("simulation.noWeather", {}, "No weather / design-day only"))}</option>`];
  for (const folder of folders) {
    const label = `${folder.source || "Weather"} - ${folder.label || folder.path || "Folder"}`;
    weatherHTML.push(`<optgroup label="${escapeHTML(label)}">`);
    for (const file of folder.files || []) {
      weatherHTML.push(`<option value="${escapeHTML(file.path)}" title="${escapeHTML(file.path)}">${escapeHTML(file.name)}</option>`);
    }
    weatherHTML.push("</optgroup>");
  }
  elements.simulationWeatherSelect.innerHTML = weatherHTML.join("");
  if (currentWeather && [...elements.simulationWeatherSelect.options].some((option) => option.value === currentWeather)) {
    elements.simulationWeatherSelect.value = currentWeather;
  }
}

function recommendedEnergyPlusInstallPath(installs, currentPath) {
  if (!installs.length) {
    return "";
  }
  const requiredVersion = currentInputEnergyPlusVersion();
  const currentInstall = installs.find((install) => install.executablePath === currentPath);
  if (requiredVersion) {
    const exact = installs.find((install) => normalizedVersionKey(install.version) === requiredVersion);
    if (exact) {
      return exact.executablePath;
    }
    if (!currentInstall || compareVersions(currentInstall.version, installs[0].version) < 0) {
      return installs[0].executablePath;
    }
  }
  return currentInstall?.executablePath || installs[0].executablePath;
}

function renderSimulationProgress() {
  const progress = state.simulationProgress;
  if (!progress || !elements.simulationProgressBar) {
    updateSimulationProgressClasses(false);
    return;
  }
  const percent = Math.max(0, Math.min(100, Number(progress.percent || 0)));
  elements.simulationProgressBar.style.width = `${percent}%`;
  elements.simulationPercent.textContent = `${Math.round(percent)}%`;
  elements.simulationStatus.textContent = progress.message || statusText(progress.status);
  updateSimulationProgressClasses(state.simulationRunning || progress.status === "running");
}

function updateSimulationControls() {
  const hasText = Boolean((elements.idfInput?.value || "").trim());
  const hasInstall = Boolean(elements.simulationEnergyPlusSelect?.value || state.simulationEnvironment?.installations?.[0]?.executablePath);
  const blockingIssue = simulationBlockingIssue();
  if (elements.simulationRunButton) {
    const disabledReason = blockingIssue
      ? blockingIssue.message
      : !hasInstall
        ? t("simulation.registerEnergyPlus", {}, "Register EnergyPlus in Settings")
        : !hasText
          ? t("simulation.noInputText", {}, "Open or paste an IDF before running simulation")
          : "";
    elements.simulationRunButton.disabled = state.simulationRunning || !hasText || !hasInstall || Boolean(blockingIssue);
    elements.simulationRunButton.textContent = state.simulationRunning ? t("simulation.runningShort", {}, "Running") : t("action.runSimulation", {}, "Run Simulation");
    elements.simulationRunButton.classList.toggle("status-loading", state.simulationRunning);
    elements.simulationRunButton.title = state.simulationRunning
      ? t("simulation.backgroundRun", {}, "EnergyPlus is running in the background")
      : disabledReason;
  }
  if (elements.simulationApplyStandardOutput) {
    elements.simulationApplyStandardOutput.disabled = state.simulationRunning || !hasText;
    elements.simulationApplyStandardOutput.title = hasText
      ? t("output.standardOutputSummary", {}, "Adds the monthly meters and zone energy variables used by standard simulation graphs.")
      : t("simulation.noInputText", {}, "Open or paste an IDF before running simulation");
  }
  if (elements.simulationEnergyPlusSelect) {
    elements.simulationEnergyPlusSelect.disabled = state.simulationRunning;
  }
  if (elements.simulationWeatherSelect) {
    elements.simulationWeatherSelect.disabled = state.simulationRunning;
  }
}

function simulationVersionIssue() {
  const requiredVersion = currentInputEnergyPlusVersion();
  if (!requiredVersion) {
    return null;
  }
  const selectedInstall = selectedEnergyPlusInstall();
  if (!selectedInstall?.version) {
    return null;
  }
  const selectedVersion = normalizedVersionKey(selectedInstall.version);
  if (selectedVersion === requiredVersion) {
    return null;
  }
  return {
    requiredVersion,
    selectedVersion: selectedInstall.version,
    title: t("simulation.versionBlockedTitle", {}, "EnergyPlus version mismatch"),
    message: t(
      "simulation.versionMismatch",
      { idf: requiredVersion, ep: selectedInstall.version },
      `IDF Version ${requiredVersion} needs matching EnergyPlus. Selected: ${selectedInstall.version}.`,
    ),
  };
}

function simulationBlockingIssue() {
  const text = elements.idfInput?.value || "";
  if (!text.trim()) {
    return null;
  }
  const hasInstall = Boolean(elements.simulationEnergyPlusSelect?.value || state.simulationEnvironment?.installations?.[0]?.executablePath);
  if (!hasInstall) {
    return {
      title: t("simulation.energyPlusBlockedTitle", {}, "EnergyPlus is not configured"),
      message: t("simulation.registerEnergyPlus", {}, "Register EnergyPlus in Settings"),
    };
  }
  const versionIssue = simulationVersionIssue();
  if (versionIssue) {
    return versionIssue;
  }
  if (!elements.simulationWeatherSelect?.value && currentInputRequiresWeatherFile(text)) {
    return {
      title: t("simulation.weatherBlockedTitle", {}, "Weather file required"),
      message: t("simulation.weatherRequired", {}, "This IDF uses weather-file design days or weather run periods. Select an EPW weather file before running."),
    };
  }
  return null;
}

function currentInputRequiresWeatherFile(text) {
  const value = String(text || "");
  if (/(?:^|\n)\s*SizingPeriod:WeatherFile(?:Days|ConditionType|DesignDay)\s*,/i.test(value)) {
    return true;
  }
  if (!/(?:^|\n)\s*RunPeriod\s*,/i.test(value)) {
    return false;
  }
  const simulationControl = value.match(/(?:^|\n)\s*SimulationControl\s*,([\s\S]*?);/i)?.[1] || "";
  const fields = simulationControl
    .split(",")
    .map((item) => item.split("!")[0].trim())
    .filter((item) => item !== "");
  const weatherRunValue = fields[4] || "";
  return !/^no$/i.test(weatherRunValue);
}

function selectedEnergyPlusInstall() {
  const path = elements.simulationEnergyPlusSelect?.value || "";
  return (state.simulationEnvironment?.installations || []).find((install) => install.executablePath === path) || null;
}

function currentInputEnergyPlusVersion() {
  return extractInputEnergyPlusVersion(elements.idfInput?.value || "");
}

function extractInputEnergyPlusVersion(text) {
  const match = String(text || "").match(/(?:^|\n)\s*Version\s*,\s*([^;!\n]+)/i);
  if (!match) {
    return "";
  }
  return normalizedVersionKey(match[1]);
}

function normalizedVersionKey(value) {
  const numbers = versionNumbers(value);
  if (numbers.length < 2) {
    return "";
  }
  return `${numbers[0]}.${numbers[1]}`;
}

function compareVersions(a, b) {
  const left = versionNumbers(a);
  const right = versionNumbers(b);
  if (!left.length && !right.length) {
    return 0;
  }
  if (!left.length) {
    return -1;
  }
  if (!right.length) {
    return 1;
  }
  const length = Math.max(left.length, right.length);
  for (let index = 0; index < length; index += 1) {
    const lValue = left[index] || 0;
    const rValue = right[index] || 0;
    if (lValue !== rValue) {
      return lValue > rValue ? 1 : -1;
    }
  }
  return 0;
}

function versionNumbers(value) {
  return String(value || "")
    .match(/\d+/g)
    ?.map((item) => Number(item))
    .filter((item) => Number.isFinite(item)) || [];
}

function updateSimulationProgressClasses(running) {
  elements.simulationStatus?.classList.toggle("status-loading", running);
  elements.simulationProgressBar?.closest(".simulation-progress-card")?.classList.toggle("running", running);
}

function renderRunningNotice() {
  const progress = state.simulationProgress || {};
  const percent = Math.max(0, Math.min(100, Number(progress.percent || 0)));
  return `
    <div class="simulation-running-notice">
      <div>
        <strong>${escapeHTML(t("simulation.backgroundRun", {}, "EnergyPlus is running in the background"))}</strong>
        <span>${escapeHTML(progress.message || t("simulation.running", {}, "EnergyPlus simulation is running"))}</span>
      </div>
      <span>${Math.round(percent)}%</span>
    </div>`;
}

function renderSimulationBlocker(issue) {
  return `
    <div class="simulation-blocker">
      <strong>${escapeHTML(issue.title || t("simulation.blockedStats", {}, "Cannot run"))}</strong>
      <span>${escapeHTML(issue.message || "")}</span>
    </div>`;
}

function renderMiniProgressSVG() {
  const progress = state.simulationProgress || {};
  const percent = Math.max(0, Math.min(100, Number(progress.percent || 0)));
  const width = 220;
  const x = 18 + (width - 36) * (percent / 100);
  return `
    <svg class="simulation-mini-progress" viewBox="0 0 ${width} 72" role="img" aria-label="${escapeHTML(t("simulation.runningShort", {}, "Running"))}">
      <line x1="18" y1="38" x2="${width - 18}" y2="38" class="simulation-mini-track" />
      <line x1="18" y1="38" x2="${x}" y2="38" class="simulation-mini-line" />
      <circle cx="18" cy="38" r="5" class="simulation-mini-node active" />
      <circle cx="${width / 2}" cy="38" r="5" class="simulation-mini-node ${percent >= 50 ? "active" : ""}" />
      <circle cx="${width - 18}" cy="38" r="5" class="simulation-mini-node ${percent >= 100 ? "active" : ""}" />
    </svg>`;
}

function renderSimulationSummary(result, stale) {
  const err = result.err || {};
  const staleBadge = stale ? `<span class="simulation-badge stale">${escapeHTML(t("simulation.stale", {}, "Stale"))}</span>` : "";
  const statusBadge = `<span class="simulation-badge ${escapeHTML(result.status || "unknown")}">${escapeHTML(statusText(result.status))}</span>`;
  const issueRows = (err.issues || [])
    .slice(0, 16)
    .map(
      (issue) => `
        <tr>
          <td><span class="simulation-severity ${escapeHTML(issue.severity)}">${escapeHTML(issue.severity)}</span></td>
          <td>${escapeHTML(issue.message)}</td>
          <td>${escapeHTML(issue.line)}</td>
        </tr>`,
    )
    .join("");
  const csvRows = (result.csvs || [])
    .map((csv) => {
      const columns = (csv.columnInfo || []).slice(0, 5).map((column) => `${column.name}: ${formatNumber(column.average)} avg`).join("<br />");
      return `
        <tr>
          <td title="${escapeHTML(csv.path)}">${escapeHTML(csv.filename)}</td>
          <td>${escapeHTML(csv.rowCount || 0)}</td>
          <td>${columns || escapeHTML(t("common.notAvailable", {}, "N/A"))}</td>
        </tr>`;
    })
    .join("");
  return `
    <div class="simulation-kpis">
      <div><span>${escapeHTML(t("common.status", {}, "Status"))}</span><strong>${statusBadge}${staleBadge}</strong></div>
      <div><span>${escapeHTML(t("simulation.errWarnings", {}, "ERR warnings"))}</span><strong>${escapeHTML(err.warnings || 0)}</strong></div>
      <div><span>${escapeHTML(t("simulation.errSevere", {}, "Severe/Fatal"))}</span><strong>${escapeHTML((err.severe || 0) + (err.fatal || 0))}</strong></div>
      <div><span>${escapeHTML(t("simulation.csvFiles", {}, "CSV files"))}</span><strong>${escapeHTML(result.csvs?.length || 0)}</strong></div>
    </div>
    ${result.error ? `<div class="simulation-error">${escapeHTML(result.error)}</div>` : ""}
    <div class="simulation-tables">
      <section>
        <h4>${escapeHTML(t("simulation.errIssues", {}, "ERR issues"))}</h4>
        <div class="output-table-wrap">
          <table class="output-table">
            <thead><tr><th>${escapeHTML(t("common.type", {}, "Type"))}</th><th>${escapeHTML(t("common.message", {}, "Message"))}</th><th>${escapeHTML(t("common.line", {}, "Line"))}</th></tr></thead>
            <tbody>${issueRows || `<tr><td colspan="3">${escapeHTML(t("simulation.noErrIssues", {}, "No ERR warnings or errors parsed."))}</td></tr>`}</tbody>
          </table>
        </div>
      </section>
      <section>
        <h4>${escapeHTML(t("simulation.csvSummary", {}, "CSV summary"))}</h4>
        <div class="output-table-wrap">
          <table class="output-table">
            <thead><tr><th>${escapeHTML(t("common.file", {}, "File"))}</th><th>${escapeHTML(t("common.rows", {}, "Rows"))}</th><th>${escapeHTML(t("common.metrics", {}, "Metrics"))}</th></tr></thead>
            <tbody>${csvRows || `<tr><td colspan="3">${escapeHTML(t("simulation.noCSV", {}, "No CSV output was found."))}</td></tr>`}</tbody>
          </table>
        </div>
      </section>
    </div>`;
}

function renderSimulationSeriesSelect(result) {
  const series = result.series || [];
  if (!series.length) {
    state.simulationSelectedSeries = "";
    elements.simulationSeriesSelect.innerHTML = `<option value="">${escapeHTML(t("simulation.noSeries", {}, "No CSV series"))}</option>`;
    return;
  }
  if (!state.simulationSelectedSeries || !series.some((item) => seriesID(item) === state.simulationSelectedSeries)) {
    state.simulationSelectedSeries = seriesID(preferredSimulationSeries(series) || series[0]);
  }
  elements.simulationSeriesSelect.innerHTML = series
    .map((item) => {
      const id = seriesID(item);
      return `<option value="${escapeHTML(id)}" ${id === state.simulationSelectedSeries ? "selected" : ""}>${escapeHTML(item.file)} - ${escapeHTML(item.column)}</option>`;
    })
    .join("");
}

function renderSimulationChart() {
  const result = state.simulationResult;
  const series = (result?.series || []).find((item) => seriesID(item) === state.simulationSelectedSeries);
  if (!series || !series.points?.length) {
    elements.simulationChart.innerHTML = `<div class="empty">${t("simulation.noGraph", {}, "CSV graph will appear after a run with numeric output.")}</div>`;
    return;
  }
  const width = 900;
  const height = 260;
  const pad = { left: 76, right: 18, top: 24, bottom: 42 };
  const values = series.points.map((point) => Number(point.value)).filter(Number.isFinite);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;
  const xStep = series.points.length > 1 ? (width - pad.left - pad.right) / (series.points.length - 1) : 1;
  const yFor = (value) => pad.top + (height - pad.top - pad.bottom) * (1 - (value - min) / range);
  const points = series.points
    .map((point, index) => `${pad.left + index * xStep},${yFor(Number(point.value))}`)
    .join(" ");
  const yTicks = [max, min + range / 2, min];
  const tickHTML = yTicks
    .map((value) => {
      const y = yFor(value);
      return `<g><line x1="${pad.left}" x2="${width - pad.right}" y1="${y}" y2="${y}" class="simulation-grid" /><text x="8" y="${y + 4}" class="simulation-axis">${escapeHTML(formatNumber(value))}</text></g>`;
    })
    .join("");
  const firstLabel = series.points[0]?.label || "start";
  const lastLabel = series.points[series.points.length - 1]?.label || "end";
  elements.simulationChart.innerHTML = `
    <svg class="simulation-svg" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(series.column)}">
      ${tickHTML}
      <line x1="${pad.left}" x2="${pad.left}" y1="${pad.top}" y2="${height - pad.bottom}" class="simulation-axis-line" />
      <line x1="${pad.left}" x2="${width - pad.right}" y1="${height - pad.bottom}" y2="${height - pad.bottom}" class="simulation-axis-line" />
      <polyline points="${points}" class="simulation-line" />
      <text x="${pad.left}" y="${height - 12}" class="simulation-axis">${escapeHTML(firstLabel)}</text>
      <text x="${width - pad.right}" y="${height - 12}" text-anchor="end" class="simulation-axis">${escapeHTML(lastLabel)}</text>
      <text x="${pad.left}" y="16" class="simulation-title">${escapeHTML(series.column)}</text>
    </svg>`;
}

function renderSimulationFiles(result) {
  const rows = (result.files || [])
    .map(
      (file) => `
        <tr>
          <td title="${escapeHTML(file.path)}">${escapeHTML(file.name)}</td>
          <td>${escapeHTML(file.kind)}</td>
          <td>${escapeHTML(formatBytes(file.size || 0))}</td>
        </tr>`,
    )
    .join("");
  elements.simulationFiles.innerHTML = `
    <div class="output-table-wrap">
      <table class="output-table">
        <thead><tr><th>${escapeHTML(t("common.file", {}, "File"))}</th><th>${escapeHTML(t("common.type", {}, "Type"))}</th><th>${escapeHTML(t("common.size", {}, "Size"))}</th></tr></thead>
        <tbody>${rows || `<tr><td colspan="3">${escapeHTML(t("simulation.noFiles", {}, "No output files yet"))}</td></tr>`}</tbody>
      </table>
    </div>`;
}

async function runCurrentSimulation({ silent = false, auto = false } = {}) {
  const text = elements.idfInput?.value || "";
  if (!text.trim() || state.simulationRunning) {
    return null;
  }
  const env = state.simulationEnvironment || (await loadSimulationEnvironment());
  renderSimulationEnvironment();
  const installPath = elements.simulationEnergyPlusSelect?.value || env?.installations?.[0]?.executablePath || "";
  if (!installPath) {
    if (!silent) {
      setStatus(t("simulation.registerEnergyPlus", {}, "Register EnergyPlus in Settings"), "warn");
    }
    renderSimulation();
    return null;
  }
  const versionIssue = simulationVersionIssue();
  if (versionIssue) {
    if (!silent) {
      setStatus(versionIssue.message, "warn");
    }
    renderSimulation();
    return null;
  }
  const blockingIssue = simulationBlockingIssue();
  if (blockingIssue) {
    if (!silent) {
      setStatus(blockingIssue.message, "warn");
    }
    renderSimulation();
    return null;
  }
  const runID = `sim-${Date.now()}`;
  state.simulationRunning = true;
  state.simulationActiveRunID = runID;
  state.simulationProgress = { runId: runID, percent: 0, message: t("simulation.preparing", {}, "Preparing simulation") };
  state.simulationRunText = text;
  state.simulationStale = false;
  renderSimulation();
  if (!silent) {
    setStatus(t("simulation.running", {}, "EnergyPlus simulation is running"), "loading");
  }
  const request = {
    runId: runID,
    text,
    inputPath: state.currentFilePath || "",
    filename: state.currentFilename || "current-input.idf",
    energyPlusExecutablePath: installPath,
    weatherPath: elements.simulationWeatherSelect?.value || "",
    standardOutput: Boolean(elements.simulationStandardOutput?.checked),
    standardOutputMode: "replace",
    silent,
    auto,
  };
  try {
    const result = await callSimulationAPI("RunSimulationText", "/api/simulation-run", request);
    if (state.simulationActiveRunID !== runID) {
      return result;
    }
    state.simulationResult = result;
    state.simulationRunning = false;
    state.simulationStale = state.simulationRunText !== (elements.idfInput?.value || "");
    state.simulationProgress = { runId: runID, percent: 100, message: simulationDoneMessage(result), status: result.status };
    renderSimulation();
    if (!silent) {
      setStatus(simulationDoneMessage(result), result.status === "succeeded" ? "ok" : "warn");
    }
    return result;
  } catch (error) {
    state.simulationRunning = false;
    state.simulationProgress = { runId: runID, percent: 100, message: error.message || String(error), status: "failed" };
    renderSimulation();
    if (!silent) {
      setStatus(error.message || String(error), "error");
    }
    return null;
  }
}

async function maybeAutoRunSimulation() {
  if (!state.simulationAutoRunOnOpen || state.simulationRunning) {
    return;
  }
  const text = elements.idfInput?.value || "";
  if (!text.trim()) {
    return;
  }
  const env = state.simulationEnvironment || (await loadSimulationEnvironment());
  if (!env?.installations?.length) {
    return;
  }
  renderSimulationEnvironment();
  if (!elements.simulationWeatherSelect?.value) {
    renderSimulation();
    return;
  }
  if (simulationVersionIssue()) {
    renderSimulation();
    return;
  }
  const key = `${state.currentFilePath || state.currentFilename || "current"}:${hashString(text)}`;
  if (state.simulationAutoStartedKey === key) {
    return;
  }
  state.simulationAutoStartedKey = key;
  runCurrentSimulation({ silent: true, auto: true });
}

function handleSimulationProgress(payload) {
  const progress = Array.isArray(payload) ? payload[0] : payload;
  if (!progress || progress.runId !== state.simulationActiveRunID) {
    return;
  }
  state.simulationProgress = progress;
  renderSimulation();
}

async function callSimulationAPI(methodName, endpoint, payload) {
  const api = backend();
  if (api && typeof api[methodName] === "function") {
    return payload === undefined ? api[methodName]() : api[methodName](payload);
  }
  const response = await fetch(endpoint, {
    method: payload === undefined ? "GET" : "POST",
    headers: payload === undefined ? undefined : { "Content-Type": "application/json" },
    body: payload === undefined ? undefined : JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await response.text());
  }
  return response.json();
}

function waitForProgressRuntime() {
  if (progressListenerRegistered) {
    return;
  }
  const register = () => {
    if (progressListenerRegistered || !window.runtime) {
      return false;
    }
    if (typeof window.runtime.EventsOn === "function") {
      window.runtime.EventsOn("idfAnalyzer:simulationProgress", handleSimulationProgress);
      progressListenerRegistered = true;
      return true;
    }
    if (typeof window.runtime.EventsOnMultiple === "function") {
      window.runtime.EventsOnMultiple("idfAnalyzer:simulationProgress", handleSimulationProgress, -1);
      progressListenerRegistered = true;
      return true;
    }
    return false;
  };
  if (register()) {
    return;
  }
  let attempts = 0;
  const timer = window.setInterval(() => {
    attempts += 1;
    if (register() || attempts > 40) {
      window.clearInterval(timer);
    }
  }, 50);
}

function seriesID(series) {
  return `${series.file}::${series.column}`;
}

function preferredSimulationSeries(series) {
  const preferred = [
    "Electricity:Facility",
    "NaturalGas:Facility",
    "DistrictCooling:Facility",
    "DistrictHeating:Facility",
    "Water:Facility",
  ];
  return preferred
    .map((name) => series.find((item) => String(item.column || "").includes(name)))
    .find(Boolean) || null;
}

function statusText(status) {
  switch (status) {
    case "succeeded":
      return t("simulation.succeeded", {}, "Succeeded");
    case "failed":
      return t("simulation.failed", {}, "Failed");
    case "missing_energyplus":
      return t("simulation.missingEnergyPlus", {}, "EnergyPlus missing");
    case "blocked":
      return t("simulation.blockedStats", {}, "Cannot run");
    case "running":
      return t("simulation.runningShort", {}, "Running");
    default:
      return t("common.notAvailable", {}, "N/A");
  }
}

function simulationDoneMessage(result) {
  if (result?.status === "succeeded") {
    return t("simulation.complete", { warnings: result.err?.warnings || 0 }, `Simulation complete (${result.err?.warnings || 0} warnings)`);
  }
  return result?.error || t("simulation.finishedWithIssues", {}, "Simulation finished with issues");
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

function formatBytes(value) {
  const number = Number(value || 0);
  if (number < 1024) {
    return `${number} B`;
  }
  if (number < 1024 * 1024) {
    return `${(number / 1024).toFixed(1)} KB`;
  }
  return `${(number / 1024 / 1024).toFixed(1)} MB`;
}

function hashString(value) {
  let hash = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return (hash >>> 0).toString(16);
}
