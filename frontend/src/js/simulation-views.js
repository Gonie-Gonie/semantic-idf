import { backend, elements, escapeHTML, setStatus, state } from "./state.js";
import { t } from "./i18n.js";
import { openStandardOutputApplyDialog } from "./output-views.js";

let progressListenerRegistered = false;
let heatFlowPlayTimer = 0;

export function initializeSimulationControls() {
  if (elements.simulationStandardOutput) {
    elements.simulationStandardOutput.checked = state.simulationStandardOutput !== false;
  }
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
  elements.simulationHeatFlowSlider?.addEventListener("input", () => {
    state.simulationHeatFlowFrameIndex = Number(elements.simulationHeatFlowSlider.value) || 0;
    renderSimulationHeatFlow();
  });
  elements.simulationHeatFlowPlay?.addEventListener("click", toggleHeatFlowPlayback);
  elements.simulationHeatFlowSpeed?.addEventListener("change", () => {
    if (state.simulationHeatFlowPlaying) {
      startHeatFlowPlayback();
    }
  });
  elements.simulationHeatFlowOverlay?.addEventListener("change", () => {
    state.simulationHeatFlowOverlay = elements.simulationHeatFlowOverlay.value || "net";
    renderSimulationHeatFlow();
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
  setSimulationPreviewMode(false);
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
  const sqlCount = (result.files || []).filter((file) => file.kind === "sqlite").length;
  const issueCount = err.total || 0;
  const statusLabel = statusText(result.status);
  elements.simulationStats.textContent = stale
    ? t("simulation.staleStats", { status: statusLabel }, `${statusLabel}, stale`)
    : t("simulation.stats", { status: statusLabel, warnings: err.warnings || 0, severe: err.severe || 0 }, `${statusLabel}, ${err.warnings || 0} warnings`);
  if (state.simulationRunning) {
    elements.simulationStats.textContent = t("simulation.runningStats", {}, "Simulation running in background");
  }
  elements.simulationResultMeta.textContent = `${result.filename || "current input"} - ${formatDuration(result.durationMs || 0)} - ${sqlCount} SQL - ${csvCount} CSV - ${issueCount} ERR issues`;
  elements.simulationResultSummary.innerHTML = `${state.simulationRunning ? renderRunningNotice() : ""}${renderSimulationSummary(result, stale)}`;
  renderSimulationHeatFlow();
  renderSimulationSeriesSelect(result);
  renderSimulationChart();
  renderSimulationFiles(result);
}

function renderSimulationEmpty() {
  if (!elements.simulationStats) {
    return;
  }
  updateSimulationControls();
  setSimulationPreviewMode(!state.simulationRunning);
  const installCount = state.simulationEnvironment?.installations?.length || 0;
  const hasText = Boolean((elements.idfInput?.value || "").trim());
  const blockingIssue = simulationBlockingIssue();
  if (state.simulationRunning) {
    setSimulationPreviewMode(false);
    elements.simulationStats.textContent = t("simulation.runningStats", {}, "Simulation running in background");
    elements.simulationResultMeta.textContent = t("simulation.backgroundRun", {}, "EnergyPlus is running in the background");
    elements.simulationResultSummary.innerHTML = renderRunningNotice();
    renderSimulationHeatFlowEmpty(t("simulation.heatFlowAfterRun", {}, "The heat-flow ledger will appear when standard heat-balance SQL/CSV output is available."));
    elements.simulationSeriesSelect.innerHTML = `<option value="">${escapeHTML(t("simulation.waitingForCSV", {}, "Waiting for SQL/CSV output"))}</option>`;
    elements.simulationChart.innerHTML = `<div class="simulation-running-empty">${renderMiniProgressSVG()}<span>${escapeHTML(t("simulation.graphAfterRun", {}, "The SQL/CSV graph will appear when the run finishes."))}</span></div>`;
    elements.simulationFiles.innerHTML = `<div class="empty status-loading">${escapeHTML(t("simulation.writingOutputs", {}, "EnergyPlus is writing output files"))}</div>`;
    updateSimulationOutputAvailability(null, true);
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
    renderSimulationHeatFlowEmpty(t("simulation.blockedGraph", {}, "Graph output is unavailable until the run can start."));
    elements.simulationSeriesSelect.innerHTML = `<option value="">${escapeHTML(t("simulation.noSeries", {}, "No SQL/CSV series"))}</option>`;
    elements.simulationChart.innerHTML = `<div class="simulation-blocked-empty">${escapeHTML(t("simulation.blockedGraph", {}, "Graph output is unavailable until the run can start."))}</div>`;
    elements.simulationFiles.innerHTML = `<div class="simulation-blocked-empty">${escapeHTML(t("simulation.blockedFiles", {}, "No output files will be created while simulation is blocked."))}</div>`;
    updateSimulationOutputAvailability(blockingIssue, false);
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
  elements.simulationResultSummary.innerHTML = `<div class="empty">${t("simulation.noResult", {}, "Run a simulation to inspect ERR and CSV outputs.")}</div>`;
  renderSimulationHeatFlowEmpty(t("simulation.noHeatFlow", {}, "Run with standard outputs to inspect zone heat-flow ledger."));
  elements.simulationSeriesSelect.innerHTML = `<option value="">${escapeHTML(t("simulation.noSeries", {}, "No SQL/CSV series"))}</option>`;
  elements.simulationChart.innerHTML = `<div class="empty">${t("simulation.noGraph", {}, "SQL/CSV graph will appear after a run with numeric output.")}</div>`;
  elements.simulationFiles.innerHTML = `<div class="empty">${t("simulation.noFiles", {}, "No output files yet")}</div>`;
  updateSimulationOutputAvailability(null, false);
}

function setSimulationPreviewMode(preview) {
  elements.simulationResultSummary?.closest(".simulation-pane")?.classList.toggle("preview", Boolean(preview));
}

function updateSimulationOutputAvailability(blockingIssue, running) {
  const standardOn = state.simulationStandardOutput !== false && Boolean(elements.simulationStandardOutput?.checked);
  const blocked = Boolean(blockingIssue);
  if (elements.simulationResultMeta) {
    elements.simulationResultMeta.textContent = running
      ? t("simulation.outputPending", {}, "Outputs are pending while EnergyPlus runs.")
      : blocked
        ? t("simulation.outputBlocked", {}, "Run requirements must be fixed before outputs are available.")
        : t("simulation.outputAvailable", {}, "After a run, ERR status and SQL/CSV output summaries will be available.");
  }
  if (elements.simulationHeatFlowStats) {
    elements.simulationHeatFlowStats.textContent = blocked
      ? t("simulation.outputBlockedShort", {}, "Unavailable until run is possible")
      : standardOn
        ? t("simulation.heatFlowAvailable", {}, "Standard outputs ON: SQL heat-flow ledger will be available after this run.")
        : t("simulation.heatFlowUnavailableStandardOff", {}, "Standard outputs OFF: running now will not show the heat-flow ledger.");
  }
  if (elements.simulationSeriesStats) {
    elements.simulationSeriesStats.textContent = blocked
      ? t("simulation.outputBlockedShort", {}, "Unavailable until run is possible")
      : standardOn
        ? t("simulation.seriesAvailable", {}, "Standard outputs ON: SQL/CSV time-series graph will be available after this run.")
        : t("simulation.seriesMaybeUnavailableStandardOff", {}, "Standard outputs OFF: graph depends on existing output requests.");
  }
  if (elements.simulationFilesStats) {
    elements.simulationFilesStats.textContent = blocked
      ? t("simulation.outputBlockedShort", {}, "Unavailable until run is possible")
      : t("simulation.filesAvailable", {}, "Output files will be listed after the run.");
  }
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
      ? t("output.standardOutputSummary", {}, "Adds standard meters, zone energy, and hourly heat-balance outputs used by simulation graphs.")
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
    elements.simulationSeriesSelect.innerHTML = `<option value="">${escapeHTML(t("simulation.noSeries", {}, "No SQL/CSV series"))}</option>`;
    if (elements.simulationSeriesStats) {
      elements.simulationSeriesStats.textContent = t("simulation.noSeries", {}, "No SQL/CSV series");
    }
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
  if (elements.simulationSeriesStats) {
    elements.simulationSeriesStats.textContent = t("simulation.seriesStats", { count: series.length }, `${series.length} SQL/CSV series`);
  }
}

function renderSimulationChart() {
  const result = state.simulationResult;
  const series = (result?.series || []).find((item) => seriesID(item) === state.simulationSelectedSeries);
  if (!series || !series.points?.length) {
    elements.simulationChart.innerHTML = `<div class="empty">${t("simulation.noGraph", {}, "SQL/CSV graph will appear after a run with numeric output.")}</div>`;
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

function renderSimulationHeatFlow() {
  if (!elements.simulationHeatFlow) {
    return;
  }
  const dataset = state.simulationResult?.heatFlow;
  if (!dataset?.zones?.length || !dataset?.categories?.length || !(dataset.frameCount > 0)) {
    renderSimulationHeatFlowEmpty(t("simulation.noHeatFlow", {}, "Run with standard outputs to inspect zone heat-flow ledger."));
    return;
  }
  const geometry = state.report?.geometry;
  if (!geometry?.zones?.length || !geometry?.stories?.length) {
    renderSimulationHeatFlowEmpty(t("simulation.noHeatFlowGeometry", {}, "Heat-flow data was parsed, but floor-plan geometry is not available yet."));
    return;
  }

  const frameCount = Math.max(0, Number(dataset.frameCount) || dataset.labels?.length || 0);
  state.simulationHeatFlowFrameIndex = clampNumber(state.simulationHeatFlowFrameIndex, 0, Math.max(frameCount - 1, 0));
  state.simulationHeatFlowOverlay = elements.simulationHeatFlowOverlay?.value || state.simulationHeatFlowOverlay || "net";
  const frameIndex = state.simulationHeatFlowFrameIndex;
  const zoneMap = heatFlowZoneMap(dataset);
  const selectedZone = ensureHeatFlowSelectedZone(dataset, geometry, zoneMap);
  const stats = [
    t("count.zones", { count: dataset.zones.length }, `${dataset.zones.length} zones`),
    `${geometry.stories.length} floors`,
    dataset.originalFrameCount > dataset.frameCount
      ? `${dataset.frameCount}/${dataset.originalFrameCount} frames`
      : `${dataset.frameCount} frames`,
    dataset.sourceFile || "",
  ].filter(Boolean);

  elements.simulationHeatFlowStats.textContent = stats.join(" - ");
  if (elements.simulationHeatFlowSlider) {
    elements.simulationHeatFlowSlider.disabled = false;
    elements.simulationHeatFlowSlider.max = String(Math.max(frameCount - 1, 0));
    elements.simulationHeatFlowSlider.value = String(frameIndex);
  }
  if (elements.simulationHeatFlowFrame) {
    elements.simulationHeatFlowFrame.textContent = heatFlowFrameLabel(dataset, frameIndex);
  }
  if (elements.simulationHeatFlowPlay) {
    elements.simulationHeatFlowPlay.disabled = frameCount <= 1;
    elements.simulationHeatFlowPlay.textContent = state.simulationHeatFlowPlaying ? t("common.pause", {}, "Pause") : t("common.play", {}, "Play");
  }
  if (elements.simulationHeatFlowOverlay) {
    elements.simulationHeatFlowOverlay.disabled = false;
    elements.simulationHeatFlowOverlay.value = state.simulationHeatFlowOverlay;
  }

  elements.simulationHeatFlow.innerHTML = `
    ${renderHeatFlowGuide()}
    <div class="heatflow-layout">
      <div class="heatflow-floor-grid">
        ${(geometry.stories || []).map((story) => renderHeatFlowStoryCard(geometry, story, dataset, zoneMap, frameIndex)).join("")}
      </div>
      <aside class="heatflow-inspector">
        ${renderHeatFlowInspector(dataset, zoneMap.get(normalizeHeatFlowName(selectedZone)), selectedZone, frameIndex)}
      </aside>
    </div>
    <div class="heatflow-tooltip hidden" role="tooltip"></div>`;
  bindHeatFlowInteractions(dataset, geometry, zoneMap);
}

function renderHeatFlowGuide() {
  return `
    <div class="heatflow-reading-guide">
      <span><i class="heatflow-guide-fill"></i>${escapeHTML(t("simulation.heatFlowGuideFill", {}, "Zone fill shows the selected overlay: net heat flow or temperature."))}</span>
      <span><i class="heatflow-guide-stack"></i>${escapeHTML(t("simulation.heatFlowGuideStack", {}, "Stack bars show each heat-flow category; up is heat entering, down is heat leaving."))}</span>
      <span><i class="heatflow-guide-ring"></i>${escapeHTML(t("simulation.heatFlowGuideRing", {}, "The +/- ring marks the zone's net direction and relative magnitude. Click a zone for the ledger."))}</span>
    </div>`;
}

function renderSimulationHeatFlowEmpty(message) {
  stopHeatFlowPlayback();
  if (elements.simulationHeatFlowStats) {
    elements.simulationHeatFlowStats.textContent = t("simulation.noHeatFlowShort", {}, "No heat-flow output");
  }
  if (elements.simulationHeatFlowSlider) {
    elements.simulationHeatFlowSlider.disabled = true;
    elements.simulationHeatFlowSlider.max = "0";
    elements.simulationHeatFlowSlider.value = "0";
  }
  if (elements.simulationHeatFlowFrame) {
    elements.simulationHeatFlowFrame.textContent = t("simulation.noFrame", {}, "No frame");
  }
  if (elements.simulationHeatFlowPlay) {
    elements.simulationHeatFlowPlay.disabled = true;
    elements.simulationHeatFlowPlay.textContent = t("common.play", {}, "Play");
  }
  if (elements.simulationHeatFlowOverlay) {
    elements.simulationHeatFlowOverlay.disabled = true;
  }
  if (elements.simulationHeatFlow) {
    elements.simulationHeatFlow.innerHTML = `<div class="empty">${escapeHTML(message)}</div>`;
  }
}

function renderHeatFlowStoryCard(geometry, story, dataset, zoneMap, frameIndex) {
  const surfaces = (geometry.surfaces || []).filter((surface) => surface.storyIndex === story.index && surface.surfaceType?.toLowerCase() === "floor");
  const bounds = heatFlowStoryBounds(surfaces);
  if (!bounds.ok || !surfaces.length) {
    return `
      <article class="heatflow-floor-card">
        <h4>${escapeHTML(story.name || `Level ${story.index + 1}`)}</h4>
        <div class="heatflow-floor-empty">${escapeHTML(t("geometry.noFloorPlan", {}, "No floor plan geometry"))}</div>
      </article>`;
  }

  const pad = 14;
  const width = 460;
  const modelWidth = Math.max(bounds.maxX - bounds.minX, 1);
  const modelHeight = Math.max(bounds.maxY - bounds.minY, 1);
  const height = Math.max(210, Math.round((modelHeight / modelWidth) * width));
  const scale = Math.min((width - pad * 2) / modelWidth, (height - pad * 2) / modelHeight);
  const projectPoint = (point) => ({
    x: pad + (point.x - bounds.minX) * scale,
    y: height - pad - (point.y - bounds.minY) * scale,
  });
  const shapes = surfaces.map((surface) => {
    const zoneName = surface.zoneName || "";
    const zoneSeries = zoneMap.get(normalizeHeatFlowName(zoneName));
    const points = (surface.vertices || []).map(projectPoint);
    const pointText = points.map((point) => `${roundSVG(point.x)},${roundSVG(point.y)}`).join(" ");
    const center = polygonCentroid(points);
    const value = heatFlowZoneNet(zoneSeries, dataset, frameIndex);
    const fill = heatFlowZoneFill(zoneSeries, dataset, frameIndex);
    const selected = normalizeHeatFlowName(zoneName) === normalizeHeatFlowName(state.simulationHeatFlowSelectedZone);
    return `
      <g class="heatflow-zone ${selected ? "selected" : ""} ${zoneSeries ? "" : "missing"}" data-heat-zone="${escapeHTML(zoneName)}">
        <polygon points="${pointText}" style="--heatflow-fill: ${fill};"></polygon>
        ${zoneSeries ? renderHeatFlowZoneStack(zoneSeries, dataset, frameIndex, center, value) : ""}
        <title>${escapeHTML(heatFlowZoneTitle(zoneName, zoneSeries, dataset, frameIndex))}</title>
      </g>`;
  });

  return `
    <article class="heatflow-floor-card">
      <h4>${escapeHTML(story.name || `Level ${story.index + 1}`)}</h4>
      <svg class="heatflow-floor-plan" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(story.name || "Floor")} heat-flow plan">
        ${shapes.join("")}
      </svg>
    </article>`;
}

function renderHeatFlowZoneStack(zoneSeries, dataset, frameIndex, center, netValue) {
  const maxAbs = Math.max(Number(dataset.maxAbs) || 1, 1);
  const axisY = 0;
  const barWidth = 8;
  const maxHeight = 28;
  let positiveOffset = 0;
  let negativeOffset = 0;
  const rects = (dataset.categories || []).map((category, index) => {
    const value = heatFlowCategoryValue(zoneSeries, index, frameIndex);
    if (!Number.isFinite(value) || Math.abs(value) <= 1e-9) {
      return "";
    }
    const height = Math.max(1.2, Math.min(maxHeight, Math.abs(value) / maxAbs * maxHeight));
    const y = value >= 0 ? axisY - positiveOffset - height : axisY + negativeOffset;
    if (value >= 0) {
      positiveOffset += height;
    } else {
      negativeOffset += height;
    }
    return `<rect x="${-barWidth / 2}" y="${roundSVG(y)}" width="${barWidth}" height="${roundSVG(height)}" fill="${escapeHTML(category.color || "#94a3b8")}"></rect>`;
  });
  const radius = Math.max(6, Math.min(18, Math.abs(netValue) / maxAbs * 22));
  const absNet = Math.abs(Number(netValue) || 0);
  const netClass = absNet <= 1e-9 ? "neutral" : netValue >= 0 ? "gain" : "loss";
  const netLabel = absNet <= 1e-9 ? "0" : netValue >= 0 ? "+" : "-";
  return `
    <g class="heatflow-mini-stack" transform="translate(${roundSVG(center.x)} ${roundSVG(center.y)})">
      <line x1="-9" x2="9" y1="0" y2="0"></line>
      ${rects.join("")}
      <circle class="heatflow-net-ring ${netClass}" r="${roundSVG(radius)}"></circle>
      <text y="3">${netLabel}</text>
    </g>`;
}

function renderHeatFlowInspector(dataset, zoneSeries, zoneName, frameIndex) {
  if (!zoneSeries) {
    return `<div class="empty">${escapeHTML(t("simulation.selectHeatFlowZone", {}, "Select a zone in the floor plan."))}</div>`;
  }
  const frameLabel = heatFlowFrameLabel(dataset, frameIndex);
  const net = heatFlowZoneNet(zoneSeries, dataset, frameIndex);
  const temp = heatFlowZoneTemperature(zoneSeries, frameIndex);
  const rows = (dataset.categories || [])
    .map((category, index) => {
      const value = heatFlowCategoryValue(zoneSeries, index, frameIndex);
      return `
        <div class="heatflow-ledger-row">
          <span><i style="--legend-color: ${escapeHTML(category.color || "#94a3b8")}"></i>${escapeHTML(category.label)}</span>
          <strong>${escapeHTML(formatWatts(value))}</strong>
        </div>`;
    })
    .join("");
  return `
    <div class="heatflow-inspector-head">
      <strong title="${escapeHTML(zoneName)}">${escapeHTML(zoneName)}</strong>
      <span>${escapeHTML(frameLabel)}</span>
    </div>
    <div class="heatflow-current-kpis">
      <span><em>${escapeHTML(t("common.temperature", {}, "Temperature"))}</em><strong>${escapeHTML(formatTemperature(temp))}</strong></span>
      <span><em>${escapeHTML(t("common.net", {}, "Net"))}</em><strong>${escapeHTML(formatWatts(net))}</strong></span>
    </div>
    <div class="heatflow-flow-note">${escapeHTML(t("simulation.heatFlowSignNote", {}, "Positive values mean heat entering the zone; negative values mean heat leaving it."))}</div>
    <div class="heatflow-ledger-list">
      ${rows}
      <div class="heatflow-ledger-row net"><span>${escapeHTML(t("common.net", {}, "Net"))}</span><strong>${escapeHTML(formatWatts(net))}</strong></div>
    </div>
    ${renderHeatFlowStackChart(dataset, zoneSeries, frameIndex)}`;
}

function renderHeatFlowStackChart(dataset, zoneSeries, frameIndex) {
  const frameCount = Math.max(dataset.frameCount || 0, dataset.labels?.length || 0);
  if (!frameCount) {
    return `<div class="empty">${escapeHTML(t("simulation.noFrame", {}, "No frame"))}</div>`;
  }
  const width = 760;
  const height = 260;
  const pad = { left: 70, right: 16, top: 18, bottom: 34 };
  const plotWidth = width - pad.left - pad.right;
  const plotHeight = height - pad.top - pad.bottom;
  const stacks = heatFlowFrameStackExtents(dataset, zoneSeries);
  const maxAbs = Math.max(Math.abs(stacks.positive), Math.abs(stacks.negative), 1);
  const yZero = pad.top + plotHeight / 2;
  const yScale = (plotHeight / 2 - 6) / maxAbs;
  const barWidth = Math.max(1, plotWidth / frameCount);
  const bars = [];
  for (let frame = 0; frame < frameCount; frame += 1) {
    const x = pad.left + frame * barWidth;
    let posY = yZero;
    let negY = yZero;
    for (let catIndex = 0; catIndex < (dataset.categories || []).length; catIndex += 1) {
      const category = dataset.categories[catIndex];
      const value = heatFlowCategoryValue(zoneSeries, catIndex, frame);
      if (!Number.isFinite(value) || Math.abs(value) <= 1e-9) {
        continue;
      }
      const h = Math.max(0.5, Math.abs(value) * yScale);
      if (value >= 0) {
        posY -= h;
        bars.push(`<rect x="${roundSVG(x)}" y="${roundSVG(posY)}" width="${roundSVG(barWidth + 0.2)}" height="${roundSVG(h)}" fill="${escapeHTML(category.color || "#94a3b8")}"></rect>`);
      } else {
        bars.push(`<rect x="${roundSVG(x)}" y="${roundSVG(negY)}" width="${roundSVG(barWidth + 0.2)}" height="${roundSVG(h)}" fill="${escapeHTML(category.color || "#94a3b8")}"></rect>`);
        negY += h;
      }
    }
  }
  const cursorX = pad.left + frameIndex * barWidth + barWidth / 2;
  const firstLabel = dataset.labels?.[0] || "start";
  const lastLabel = dataset.labels?.[frameCount - 1] || "end";
  return `
    <svg class="heatflow-stack-chart" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(zoneSeries.name)} heat-flow stack">
      <line x1="${pad.left}" x2="${width - pad.right}" y1="${yZero}" y2="${yZero}" class="simulation-axis-line" />
      <line x1="${pad.left}" x2="${pad.left}" y1="${pad.top}" y2="${height - pad.bottom}" class="simulation-axis-line" />
      <text x="8" y="${pad.top + 10}" class="simulation-axis">${escapeHTML(formatWatts(maxAbs))}</text>
      <text x="8" y="${yZero + 4}" class="simulation-axis">0</text>
      <text x="8" y="${height - pad.bottom}" class="simulation-axis">${escapeHTML(formatWatts(-maxAbs))}</text>
      ${bars.join("")}
      <line x1="${roundSVG(cursorX)}" x2="${roundSVG(cursorX)}" y1="${pad.top}" y2="${height - pad.bottom}" class="heatflow-cursor" />
      <rect class="heatflow-chart-hit" x="${pad.left}" y="${pad.top}" width="${plotWidth}" height="${plotHeight}" data-heatflow-chart="1"></rect>
      <text x="${pad.left}" y="${height - 10}" class="simulation-axis">${escapeHTML(firstLabel)}</text>
      <text x="${width - pad.right}" y="${height - 10}" text-anchor="end" class="simulation-axis">${escapeHTML(lastLabel)}</text>
    </svg>`;
}

function bindHeatFlowInteractions(dataset, geometry, zoneMap) {
  const host = elements.simulationHeatFlow;
  const tooltip = host.querySelector(".heatflow-tooltip");
  host.querySelectorAll("[data-heat-zone]").forEach((shape) => {
    shape.addEventListener("pointerenter", (event) => showHeatFlowTooltip(event, shape.dataset.heatZone, dataset, zoneMap, tooltip));
    shape.addEventListener("pointermove", (event) => showHeatFlowTooltip(event, shape.dataset.heatZone, dataset, zoneMap, tooltip));
    shape.addEventListener("pointerleave", () => hideHeatFlowTooltip(tooltip));
    shape.addEventListener("click", () => {
      state.simulationHeatFlowSelectedZone = shape.dataset.heatZone || "";
      renderSimulationHeatFlow();
    });
  });
  host.querySelector("[data-heatflow-chart]")?.addEventListener("pointermove", (event) => {
    const rect = event.currentTarget.getBoundingClientRect();
    const frameCount = Number(dataset.frameCount) || 1;
    const ratio = clampNumber((event.clientX - rect.left) / rect.width, 0, 1);
    const nextFrame = clampNumber(Math.round(ratio * (frameCount - 1)), 0, frameCount - 1);
    if (nextFrame === state.simulationHeatFlowFrameIndex) {
      return;
    }
    state.simulationHeatFlowFrameIndex = nextFrame;
    renderSimulationHeatFlow();
  });
  host.querySelector("[data-heatflow-chart]")?.addEventListener("pointerleave", () => hideHeatFlowTooltip(tooltip));
  void geometry;
}

function showHeatFlowTooltip(event, zoneName, dataset, zoneMap, tooltip) {
  const zoneSeries = zoneMap.get(normalizeHeatFlowName(zoneName));
  if (!tooltip || !zoneSeries) {
    return;
  }
  const frameIndex = state.simulationHeatFlowFrameIndex;
  const rows = (dataset.categories || [])
    .map((category, index) => {
      const value = heatFlowCategoryValue(zoneSeries, index, frameIndex);
      return `<span><i style="--legend-color: ${escapeHTML(category.color || "#94a3b8")}"></i>${escapeHTML(category.label)}: ${escapeHTML(formatWatts(value))}</span>`;
    })
    .join("");
  tooltip.innerHTML = `
    <strong>${escapeHTML(zoneName)}</strong>
    <span>${escapeHTML(heatFlowFrameLabel(dataset, frameIndex))}</span>
    <span>${escapeHTML(formatTemperature(heatFlowZoneTemperature(zoneSeries, frameIndex)))}</span>
    ${rows}
    <b>${escapeHTML(t("common.net", {}, "Net"))}: ${escapeHTML(formatWatts(heatFlowZoneNet(zoneSeries, dataset, frameIndex)))}</b>`;
  const rect = elements.simulationHeatFlow.getBoundingClientRect();
  tooltip.style.left = `${Math.min(rect.width - 220, Math.max(8, event.clientX - rect.left + 14))}px`;
  tooltip.style.top = `${Math.max(8, event.clientY - rect.top + 14)}px`;
  tooltip.classList.remove("hidden");
}

function hideHeatFlowTooltip(tooltip) {
  tooltip?.classList.add("hidden");
}

function toggleHeatFlowPlayback() {
  if (state.simulationHeatFlowPlaying) {
    stopHeatFlowPlayback();
    renderSimulationHeatFlow();
    return;
  }
  startHeatFlowPlayback();
}

function startHeatFlowPlayback() {
  stopHeatFlowPlayback(false);
  const dataset = state.simulationResult?.heatFlow;
  const frameCount = Number(dataset?.frameCount) || 0;
  if (frameCount <= 1) {
    return;
  }
  state.simulationHeatFlowPlaying = true;
  const delay = Math.max(80, Number(elements.simulationHeatFlowSpeed?.value) || 420);
  heatFlowPlayTimer = window.setInterval(() => {
    const next = (Number(state.simulationHeatFlowFrameIndex) + 1) % frameCount;
    state.simulationHeatFlowFrameIndex = next;
    renderSimulationHeatFlow();
  }, delay);
  renderSimulationHeatFlow();
}

function stopHeatFlowPlayback(render = true) {
  if (heatFlowPlayTimer) {
    window.clearInterval(heatFlowPlayTimer);
    heatFlowPlayTimer = 0;
  }
  state.simulationHeatFlowPlaying = false;
  if (render && elements.simulationHeatFlowPlay) {
    elements.simulationHeatFlowPlay.textContent = t("common.play", {}, "Play");
  }
}

function heatFlowZoneMap(dataset) {
  const map = new Map();
  (dataset?.zones || []).forEach((zone) => {
    map.set(normalizeHeatFlowName(zone.name), zone);
  });
  return map;
}

function ensureHeatFlowSelectedZone(dataset, geometry, zoneMap) {
  if (state.simulationHeatFlowSelectedZone && zoneMap.has(normalizeHeatFlowName(state.simulationHeatFlowSelectedZone))) {
    return state.simulationHeatFlowSelectedZone;
  }
  const geometryMatch = (geometry.zones || []).find((zone) => zoneMap.has(normalizeHeatFlowName(zone.name)));
  state.simulationHeatFlowSelectedZone = geometryMatch?.name || dataset.zones?.[0]?.name || "";
  return state.simulationHeatFlowSelectedZone;
}

function heatFlowZoneFill(zoneSeries, dataset, frameIndex) {
  if (!zoneSeries) {
    return "#1f2933";
  }
  if (state.simulationHeatFlowOverlay === "temperature") {
    return heatFlowTemperatureColor(heatFlowZoneTemperature(zoneSeries, frameIndex), dataset.minTemperature, dataset.maxTemperature);
  }
  return heatFlowNetColor(heatFlowZoneNet(zoneSeries, dataset, frameIndex), dataset.maxAbs);
}

function heatFlowZoneTitle(zoneName, zoneSeries, dataset, frameIndex) {
  if (!zoneSeries) {
    return `${zoneName} / no heat-flow CSV data`;
  }
  return `${zoneName} / ${formatTemperature(heatFlowZoneTemperature(zoneSeries, frameIndex))} / net ${formatWatts(heatFlowZoneNet(zoneSeries, dataset, frameIndex))}`;
}

function heatFlowCategoryValue(zoneSeries, categoryIndex, frameIndex) {
  const value = zoneSeries?.values?.[categoryIndex]?.[frameIndex];
  const number = Number(value);
  return Number.isFinite(number) ? number : 0;
}

function heatFlowZoneNet(zoneSeries, dataset, frameIndex) {
  if (!zoneSeries) {
    return 0;
  }
  return (dataset.categories || []).reduce((sum, _category, index) => sum + heatFlowCategoryValue(zoneSeries, index, frameIndex), 0);
}

function heatFlowZoneTemperature(zoneSeries, frameIndex) {
  const value = zoneSeries?.temperature?.[frameIndex];
  const number = Number(value);
  return Number.isFinite(number) ? number : NaN;
}

function heatFlowFrameStackExtents(dataset, zoneSeries) {
  let positive = 0;
  let negative = 0;
  const frameCount = Number(dataset.frameCount) || 0;
  for (let frame = 0; frame < frameCount; frame += 1) {
    let pos = 0;
    let neg = 0;
    for (let index = 0; index < (dataset.categories || []).length; index += 1) {
      const value = heatFlowCategoryValue(zoneSeries, index, frame);
      if (value >= 0) {
        pos += value;
      } else {
        neg += value;
      }
    }
    positive = Math.max(positive, pos);
    negative = Math.min(negative, neg);
  }
  return { positive, negative };
}

function heatFlowStoryBounds(surfaces) {
  const bounds = { minX: Infinity, maxX: -Infinity, minY: Infinity, maxY: -Infinity, ok: false };
  surfaces.forEach((surface) => {
    (surface.vertices || []).forEach((point) => {
      bounds.ok = true;
      bounds.minX = Math.min(bounds.minX, Number(point.x) || 0);
      bounds.maxX = Math.max(bounds.maxX, Number(point.x) || 0);
      bounds.minY = Math.min(bounds.minY, Number(point.y) || 0);
      bounds.maxY = Math.max(bounds.maxY, Number(point.y) || 0);
    });
  });
  return bounds;
}

function polygonCentroid(points) {
  if (!points?.length) {
    return { x: 0, y: 0 };
  }
  let x = 0;
  let y = 0;
  points.forEach((point) => {
    x += point.x;
    y += point.y;
  });
  return { x: x / points.length, y: y / points.length };
}

function heatFlowFrameLabel(dataset, frameIndex) {
  const label = dataset?.labels?.[frameIndex] || "";
  return label ? `${label}` : `Frame ${frameIndex + 1}`;
}

function heatFlowNetColor(value, maxAbs) {
  const scale = Math.min(1, Math.abs(Number(value) || 0) / Math.max(Number(maxAbs) || 1, 1));
  if (scale < 0.035) {
    return mixHexColor("#24303a", "#50d878", 0.72);
  }
  return value >= 0
    ? mixHexColor("#23313a", "#ef4444", 0.2 + scale * 0.72)
    : mixHexColor("#23313a", "#3b82f6", 0.2 + scale * 0.72);
}

function heatFlowTemperatureColor(value, min, max) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return "#24303a";
  }
  const ratio = clampNumber((number - Number(min || 0)) / Math.max(Number(max || 0) - Number(min || 0), 0.001), 0, 1);
  if (ratio < 0.33) {
    return mixHexColor("#2563eb", "#22c55e", ratio / 0.33);
  }
  if (ratio < 0.68) {
    return mixHexColor("#22c55e", "#facc15", (ratio - 0.33) / 0.35);
  }
  return mixHexColor("#facc15", "#ef4444", (ratio - 0.68) / 0.32);
}

function mixHexColor(left, right, amount) {
  const a = hexToRGB(left);
  const b = hexToRGB(right);
  const tValue = clampNumber(amount, 0, 1);
  const mixed = a.map((channel, index) => Math.round(channel + (b[index] - channel) * tValue));
  return `rgb(${mixed[0]}, ${mixed[1]}, ${mixed[2]})`;
}

function hexToRGB(value) {
  const hex = String(value || "#000000").replace("#", "");
  const full = hex.length === 3 ? hex.split("").map((char) => `${char}${char}`).join("") : hex.padEnd(6, "0").slice(0, 6);
  return [0, 2, 4].map((offset) => Number.parseInt(full.slice(offset, offset + 2), 16) || 0);
}

function clampNumber(value, min, max) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return min;
  }
  return Math.max(min, Math.min(max, number));
}

function roundSVG(value) {
  return Number(value || 0).toFixed(2);
}

function normalizeHeatFlowName(value) {
  return String(value || "").trim().toLowerCase().replace(/\s+/g, " ");
}

function formatWatts(value) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return "N/A";
  }
  const sign = number > 0 ? "+" : "";
  if (Math.abs(number) >= 1000000) {
    return `${sign}${(number / 1000000).toLocaleString(undefined, { maximumFractionDigits: 2 })} MW`;
  }
  if (Math.abs(number) >= 1000) {
    return `${sign}${(number / 1000).toLocaleString(undefined, { maximumFractionDigits: 1 })} kW`;
  }
  return `${sign}${number.toLocaleString(undefined, { maximumFractionDigits: 0 })} W`;
}

function formatTemperature(value) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return "N/A";
  }
  return `${number.toLocaleString(undefined, { maximumFractionDigits: 1 })} degC`;
}

function renderSimulationFiles(result) {
  if (elements.simulationFilesStats) {
    elements.simulationFilesStats.textContent = t("simulation.fileStats", { count: result.files?.length || 0 }, `${result.files?.length || 0} files`);
  }
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
