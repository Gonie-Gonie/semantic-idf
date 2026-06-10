import { backend, elements, escapeHTML, setStatus, state } from "./state.js";
import { t } from "./i18n.js";
import { openStandardOutputApplyDialog } from "./output-views.js";

let progressListenerRegistered = false;
let heatFlowPlayTimer = 0;
let simulationRunPlanTimer = 0;
let simulationRunPlanSequence = 0;

const simulationPurposeDefinitions = [
  { id: "basic_energy", label: "Basic Energy" },
  { id: "zone_heat_flow", label: "Zone Heat Flow" },
  { id: "hvac_loop_check", label: "HVAC Loop Check" },
  { id: "integrity_check", label: "Integrity" },
  { id: "comfort_check", label: "Comfort" },
  { id: "custom_outputs", label: "Custom Outputs" },
];

export function initializeSimulationControls() {
  state.simulationHeatFlowInspectorCollapsed = readHeatFlowInspectorCollapsed();
  if (elements.simulationStandardOutput) {
    elements.simulationStandardOutput.checked = state.simulationStandardOutput !== false;
  }
  elements.simulationApplyStandardOutput?.addEventListener("click", () => openStandardOutputApplyDialog());
  elements.simulationPurposeInputs?.forEach((input) => {
    input.checked = state.simulationSelectedPurposes.includes(input.dataset.simulationPurpose);
    input.addEventListener("change", () => {
      syncSimulationPurposeStateFromControls();
      scheduleSimulationRunPlan();
      renderSimulation();
    });
  });
  elements.simulationPurposeZoneMode?.addEventListener("change", () => {
    scheduleSimulationRunPlan();
    renderSimulation();
  });
  elements.simulationPurposeZoneNames?.addEventListener("input", () => scheduleSimulationRunPlan());
  elements.simulationPurposeFrequencyPolicy?.addEventListener("change", () => scheduleSimulationRunPlan());
  elements.simulationPersistOutputs?.addEventListener("change", () => scheduleSimulationRunPlan());
  elements.simulationRefreshPlan?.addEventListener("click", () => refreshSimulationRunPlan({ force: true }));
  elements.simulationApplyPurposeOutputs?.addEventListener("click", () => applyPurposeOutputsToCurrentIDF());
  elements.simulationResultTabs?.querySelectorAll("[data-simulation-result-view-button]").forEach((button) => {
    button.addEventListener("click", () => {
      state.simulationActiveResultView = button.dataset.simulationResultViewButton || "energy";
      renderSimulationResultTabs(state.simulationResult);
      toggleSimulationResultSections();
    });
  });
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
    state.simulationSeriesRangeStart = 0;
    state.simulationSeriesRangeEnd = -1;
    renderSimulationChart();
  });
  elements.simulationHeatFlowSlider?.addEventListener("input", () => {
    state.simulationHeatFlowFrameIndex = Number(elements.simulationHeatFlowSlider.value) || 0;
    normalizeHeatFlowFrameRange();
    renderSimulationHeatFlow();
  });
  elements.simulationHeatFlowStory?.addEventListener("change", () => {
    state.simulationHeatFlowStory = elements.simulationHeatFlowStory.value || "all";
    renderSimulationHeatFlow();
  });
  elements.simulationHeatFlowRangeStart?.addEventListener("input", () => {
    state.simulationHeatFlowRangeStart = Number(elements.simulationHeatFlowRangeStart.value) || 0;
    normalizeHeatFlowFrameRange(undefined, "start");
    renderSimulationHeatFlow();
  });
  elements.simulationHeatFlowRangeEnd?.addEventListener("input", () => {
    state.simulationHeatFlowRangeEnd = Number(elements.simulationHeatFlowRangeEnd.value) || 0;
    normalizeHeatFlowFrameRange(undefined, "end");
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
    scheduleSimulationRunPlan();
    updateSimulationControls();
    renderSimulation();
  });
  window.addEventListener("idfAnalyzer:outputApplied", () => scheduleSimulationRunPlan(0));
  window.addEventListener("idfAnalyzer:analysisComplete", () => maybeAutoRunSimulation());
  window.addEventListener("idfAnalyzer:settingsChanged", (event) => {
    state.simulationAutoRunOnOpen = event.detail?.settings?.simulation?.autoRunOnOpen ?? state.simulationAutoRunOnOpen;
    loadSimulationEnvironment();
  });
  waitForProgressRuntime();
  loadSimulationEnvironment();
  scheduleSimulationRunPlan();
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
  renderSimulationPurposeSetup();
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
  ensureActiveSimulationResultView(result);
  renderSimulationResultTabs(result);
  elements.simulationStats.textContent = stale
    ? t("simulation.staleStats", { status: statusLabel }, `${statusLabel}, stale`)
    : t("simulation.stats", { status: statusLabel, warnings: err.warnings || 0, severe: err.severe || 0 }, `${statusLabel}, ${err.warnings || 0} warnings`);
  if (state.simulationRunning) {
    elements.simulationStats.textContent = t("simulation.runningStats", {}, "Simulation running in background");
  }
  elements.simulationResultMeta.textContent = `${result.filename || "current input"} - ${formatDuration(result.durationMs || 0)} - ${sqlCount} SQL - ${csvCount} CSV - ${issueCount} ERR issues`;
  elements.simulationResultSummary.innerHTML = `${state.simulationRunning ? renderRunningNotice() : ""}${renderSimulationSummary(result, stale)}`;
  renderSimulationEnergyDashboard(result);
  renderSimulationHeatFlow();
  renderSimulationSeriesSelect(result);
  renderSimulationChart();
  renderSimulationFiles(result);
  toggleSimulationResultSections();
}

function renderSimulationEmpty() {
  if (!elements.simulationStats) {
    return;
  }
  renderSimulationPurposeSetup();
  updateSimulationControls();
  setSimulationPreviewMode(!state.simulationRunning);
  const installCount = state.simulationEnvironment?.installations?.length || 0;
  const hasText = Boolean((elements.idfInput?.value || "").trim());
  const blockingIssue = simulationBlockingIssue();
  if (state.simulationRunning) {
    setSimulationPreviewMode(false);
    renderSimulationResultTabs(null);
    elements.simulationStats.textContent = t("simulation.runningStats", {}, "Simulation running in background");
    elements.simulationResultMeta.textContent = t("simulation.backgroundRun", {}, "EnergyPlus is running in the background");
    elements.simulationResultSummary.innerHTML = renderRunningNotice();
    renderSimulationHeatFlowEmpty(t("simulation.heatFlowAfterRun", {}, "The heat-flow ledger will appear when standard heat-balance SQL/CSV output is available."));
    elements.simulationSeriesSelect.innerHTML = `<option value="">${escapeHTML(t("simulation.waitingForCSV", {}, "Waiting for SQL/CSV output"))}</option>`;
    elements.simulationChart.innerHTML = `<div class="simulation-running-empty">${renderMiniProgressSVG()}<span>${escapeHTML(t("simulation.graphAfterRun", {}, "The SQL/CSV graph will appear when the run finishes."))}</span></div>`;
    elements.simulationFiles.innerHTML = `<div class="empty status-loading">${escapeHTML(t("simulation.writingOutputs", {}, "EnergyPlus is writing output files"))}</div>`;
    renderSimulationEnergyEmpty(t("simulation.outputPending", {}, "Outputs are pending while EnergyPlus runs."));
    toggleSimulationResultSections();
    updateSimulationOutputAvailability(null, true);
    return;
  }
  if (blockingIssue) {
    renderSimulationResultTabs(null);
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
    renderSimulationEnergyEmpty(t("simulation.outputBlocked", {}, "Run requirements must be fixed before outputs are available."));
    toggleSimulationResultSections();
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
  renderSimulationResultTabs(null);
  renderSimulationEnergyEmpty(t("simulation.noEnergyResult", {}, "Run Basic Energy to inspect monthly energy results."));
  renderSimulationHeatFlowEmpty(t("simulation.noHeatFlow", {}, "Run with standard outputs to inspect zone heat-flow ledger."));
  elements.simulationSeriesSelect.innerHTML = `<option value="">${escapeHTML(t("simulation.noSeries", {}, "No SQL/CSV series"))}</option>`;
  elements.simulationChart.innerHTML = `<div class="empty">${t("simulation.noGraph", {}, "SQL/CSV graph will appear after a run with numeric output.")}</div>`;
  elements.simulationFiles.innerHTML = `<div class="empty">${t("simulation.noFiles", {}, "No output files yet")}</div>`;
  toggleSimulationResultSections();
  updateSimulationOutputAvailability(null, false);
}

function renderSimulationPurposeSetup() {
  syncSimulationPurposeInputsFromState();
  const selected = selectedSimulationPurposes();
  if (elements.simulationPurposeStats) {
    elements.simulationPurposeStats.textContent = selected.length
      ? selected.map(purposeLabel).join(" + ")
      : t("simulation.noPurposeSelected", {}, "No purposes selected");
  }
  if (elements.simulationPurposeZoneNames) {
    const selectedMode = elements.simulationPurposeZoneMode?.value === "selected";
    elements.simulationPurposeZoneNames.disabled = !selectedMode;
    elements.simulationPurposeZoneNames.closest("label")?.classList.toggle("disabled", !selectedMode);
  }
  renderSimulationRunPlanPreview();
}

function renderSimulationRunPlanPreview() {
  if (!elements.simulationRunPlan) {
    return;
  }
  const plan = state.simulationPurposePlan;
  const hasText = Boolean((elements.idfInput?.value || "").trim());
  if (state.simulationPurposePlanLoading) {
    elements.simulationRunPlanStats.textContent = t("simulation.planLoading", {}, "Building plan");
    elements.simulationRunPlan.innerHTML = `<div class="empty status-loading">${escapeHTML(t("simulation.planLoadingDetail", {}, "Checking current IDF outputs and purpose presets."))}</div>`;
    updatePurposeApplyButton();
    return;
  }
  if (state.simulationPurposePlanError) {
    elements.simulationRunPlanStats.textContent = t("simulation.planError", {}, "Plan unavailable");
    elements.simulationRunPlan.innerHTML = `<div class="simulation-error">${escapeHTML(state.simulationPurposePlanError)}</div>`;
    updatePurposeApplyButton();
    return;
  }
  if (!hasText) {
    elements.simulationRunPlanStats.textContent = t("simulation.planPending", {}, "Plan pending");
    elements.simulationRunPlan.innerHTML = `<div class="empty">${escapeHTML(t("simulation.planNeedsInput", {}, "Run plan will appear after an input is available."))}</div>`;
    updatePurposeApplyButton();
    return;
  }
  if (!plan) {
    elements.simulationRunPlanStats.textContent = t("simulation.planPending", {}, "Plan pending");
    elements.simulationRunPlan.innerHTML = `<div class="empty">${escapeHTML(t("simulation.planWillBuild", {}, "Run plan will build automatically."))}</div>`;
    updatePurposeApplyButton();
    return;
  }
  const objects = plan.outputObjects || [];
  const warnings = plan.warnings || [];
  elements.simulationRunPlanStats.textContent = t(
    "simulation.planStats",
    { count: objects.length, weight: plan.estimatedWeight || "Light" },
    `${objects.length} outputs - ${plan.estimatedWeight || "Light"}`,
  );
  const warningHTML = warnings.length
    ? `<div class="simulation-plan-warnings">${warnings
        .map((warning) => `<span class="${escapeHTML(warning.severity || "info")}">${escapeHTML(warning.message || warning.code || "")}</span>`)
        .join("")}</div>`
    : "";
  const rows = objects
    .slice(0, 80)
    .map((object) => {
      const key = object.keyValue || outputFieldValue(object.fields, "Option Type", "Report 1 Name", "Key 1", "Column Separator") || "-";
      return `
        <tr>
          <td>${escapeHTML(object.objectType || "")}</td>
          <td>${escapeHTML(key)}</td>
          <td>${escapeHTML(object.variableName || "")}</td>
          <td>${escapeHTML(object.reportingFrequency || "")}</td>
          <td>${escapeHTML((object.purposeIds || []).map(purposeLabel).join(", "))}</td>
          <td><span class="simulation-output-state ${escapeHTML(object.state || "")}">${escapeHTML(outputStateLabel(object.state))}</span></td>
          <td>${escapeHTML(object.weight || "light")}</td>
        </tr>`;
    })
    .join("");
  const truncated = objects.length > 80 ? `<div class="tool-muted">${escapeHTML(t("simulation.planTruncated", { count: objects.length - 80 }, `${objects.length - 80} more outputs hidden.`))}</div>` : "";
  elements.simulationRunPlan.innerHTML = `
    <div class="simulation-plan-summary">
      <span>${escapeHTML(t("simulation.estimatedSeries", { count: plan.estimatedSeries || 0 }, `${plan.estimatedSeries || 0} series`))}</span>
      <span>${escapeHTML(t("simulation.estimatedFrames", { count: plan.estimatedFrames || 0 }, `${plan.estimatedFrames || 0} frames`))}</span>
      <span>${escapeHTML(plan.requiresSQL ? t("simulation.sqlPrimary", {}, "SQL primary") : t("simulation.sqlOptional", {}, "SQL optional"))}</span>
    </div>
    ${warningHTML}
    <div class="output-table-wrap">
      <table class="output-table simulation-plan-table">
        <thead><tr><th>${escapeHTML(t("common.type", {}, "Type"))}</th><th>${escapeHTML(t("common.key", {}, "Key"))}</th><th>${escapeHTML(t("common.metric", {}, "Metric"))}</th><th>${escapeHTML(t("common.frequency", {}, "Frequency"))}</th><th>${escapeHTML(t("simulation.purpose", {}, "Purpose"))}</th><th>${escapeHTML(t("common.status", {}, "Status"))}</th><th>${escapeHTML(t("simulation.weight", {}, "Weight"))}</th></tr></thead>
        <tbody>${rows || `<tr><td colspan="7">${escapeHTML(t("simulation.noPlanOutputs", {}, "No output objects selected."))}</td></tr>`}</tbody>
      </table>
    </div>
    ${truncated}`;
  updatePurposeApplyButton();
}

function renderSimulationResultTabs(result) {
  if (!elements.simulationResultTabs) {
    return;
  }
  const availability = simulationResultViewAvailability(result);
  elements.simulationResultTabs.querySelectorAll("[data-simulation-result-view-button]").forEach((button) => {
    const view = button.dataset.simulationResultViewButton || "";
    button.classList.toggle("active", view === state.simulationActiveResultView);
    button.disabled = Boolean(result) && !availability[view];
  });
}

function ensureActiveSimulationResultView(result) {
  const availability = simulationResultViewAvailability(result);
  if (availability[state.simulationActiveResultView]) {
    return;
  }
  state.simulationActiveResultView = ["energy", "zone_heat_flow", "integrity", "series", "files"].find((view) => availability[view]) || "energy";
}

function simulationResultViewAvailability(result) {
  if (!result) {
    return { energy: true, zone_heat_flow: true, integrity: true, series: true, files: true };
  }
  const energy = result.purposeResults?.energy || {};
  return {
    energy: Boolean((energy.facilityMonthly || []).length || (energy.endUseMonthly || []).length || (energy.zoneMonthly || []).length),
    zone_heat_flow: Boolean((result.heatFlow?.zones || []).length),
    integrity: true,
    series: Boolean((result.series || []).length),
    files: Boolean((result.files || []).length),
  };
}

function toggleSimulationResultSections() {
  document.querySelectorAll("#simulationPane [data-simulation-result-view]").forEach((section) => {
    section.hidden = section.dataset.simulationResultView !== state.simulationActiveResultView;
  });
}

function renderSimulationEnergyEmpty(message) {
  if (elements.simulationEnergyStats) {
    elements.simulationEnergyStats.textContent = t("simulation.noEnergyResult", {}, "No energy result");
  }
  if (elements.simulationEnergyDashboard) {
    elements.simulationEnergyDashboard.innerHTML = `<div class="empty">${escapeHTML(message)}</div>`;
  }
}

function renderSimulationEnergyDashboard(result) {
  const energy = result?.purposeResults?.energy || {};
  const facility = energy.facilityMonthly || [];
  const endUse = energy.endUseMonthly || [];
  const zones = energy.zoneMonthly || [];
  if (!facility.length && !endUse.length && !zones.length) {
    renderSimulationEnergyEmpty(t("simulation.noEnergyResult", {}, "Run Basic Energy to inspect monthly energy results."));
    return;
  }
  if (elements.simulationEnergyStats) {
    elements.simulationEnergyStats.textContent = t(
      "simulation.energyStats",
      { facility: facility.length, enduse: endUse.length, zones: zones.length },
      `${facility.length} facility, ${endUse.length} end-use, ${zones.length} zone series`,
    );
  }
  const totals = [...facility, ...endUse].map((item) => ({ name: item.name, value: Number(item.total) || 0, unit: item.unit || "", source: item.source || "" }));
  const kpis = totals
    .slice()
    .sort((a, b) => Math.abs(b.value) - Math.abs(a.value))
    .slice(0, 4)
    .map(
      (item) => `
        <div>
          <span>${escapeHTML(item.name)}</span>
          <strong>${escapeHTML(formatEnergyValue(item.value, item.unit))}</strong>
        </div>`,
    )
    .join("");
  elements.simulationEnergyDashboard.innerHTML = `
    <div class="simulation-energy-kpis">${kpis || `<div><span>${escapeHTML(t("common.notAvailable", {}, "N/A"))}</span><strong>0</strong></div>`}</div>
    ${renderEnergyBarSection(t("simulation.facilityEnergy", {}, "Facility energy"), facility)}
    ${renderEnergyBarSection(t("simulation.endUseEnergy", {}, "End-use energy"), endUse)}
    ${renderZoneEnergyTable(zones)}`;
}

function renderEnergyBarSection(title, series) {
  if (!series.length) {
    return `
      <section class="simulation-energy-block">
        <h4>${escapeHTML(title)}</h4>
        <div class="empty">${escapeHTML(t("simulation.noEnergySeries", {}, "No energy series found."))}</div>
      </section>`;
  }
  const maxValue = Math.max(...series.map((item) => Math.abs(Number(item.total) || 0)), 1);
  const rows = series
    .slice()
    .sort((a, b) => Math.abs(Number(b.total) || 0) - Math.abs(Number(a.total) || 0))
    .map((item) => {
      const value = Number(item.total) || 0;
      const width = Math.max(2, Math.min(100, (Math.abs(value) / maxValue) * 100));
      return `
        <div class="simulation-energy-bar-row" title="${escapeHTML(item.source || "")}">
          <span>${escapeHTML(item.name || "")}</span>
          <div><i style="width:${width}%"></i></div>
          <strong>${escapeHTML(formatEnergyValue(value, item.unit))}</strong>
        </div>`;
    })
    .join("");
  return `
    <section class="simulation-energy-block">
      <h4>${escapeHTML(title)}</h4>
      <div class="simulation-energy-bars">${rows}</div>
    </section>`;
}

function renderZoneEnergyTable(zones) {
  const rows = zones
    .slice()
    .sort((a, b) => Math.abs(Number(b.total) || 0) - Math.abs(Number(a.total) || 0))
    .slice(0, 32)
    .map(
      (item) => `
        <tr>
          <td>${escapeHTML(item.zoneName || "")}</td>
          <td>${escapeHTML(item.metric || "")}</td>
          <td>${escapeHTML(formatEnergyValue(Number(item.total) || 0, item.unit || ""))}</td>
          <td>${escapeHTML(item.source || "")}</td>
        </tr>`,
    )
    .join("");
  return `
    <section class="simulation-energy-block">
      <h4>${escapeHTML(t("simulation.zoneEnergy", {}, "Zone reported energy"))}</h4>
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>${escapeHTML(t("common.targetZones", {}, "Target Zones"))}</th><th>${escapeHTML(t("common.metric", {}, "Metric"))}</th><th>${escapeHTML(t("common.value", {}, "Value"))}</th><th>${escapeHTML(t("common.source", {}, "Source"))}</th></tr></thead>
          <tbody>${rows || `<tr><td colspan="4">${escapeHTML(t("simulation.noZoneEnergy", {}, "No zone reported energy series found."))}</td></tr>`}</tbody>
        </table>
      </div>
    </section>`;
}

function formatEnergyValue(value, unit) {
  const safeUnit = unit || "";
  return `${formatNumber(value)}${safeUnit ? ` ${safeUnit}` : ""}`;
}

function scheduleSimulationRunPlan(delay = 260) {
  window.clearTimeout(simulationRunPlanTimer);
  simulationRunPlanTimer = window.setTimeout(() => refreshSimulationRunPlan(), delay);
}

async function refreshSimulationRunPlan({ force = false } = {}) {
  if (!elements.simulationRunPlan) {
    return null;
  }
  const text = elements.idfInput?.value || "";
  if (!text.trim()) {
    state.simulationPurposePlan = null;
    state.simulationPurposePlanKey = "";
    state.simulationPurposePlanError = "";
    state.simulationPurposePlanLoading = false;
    renderSimulationRunPlanPreview();
    return null;
  }
  const purposeRequest = buildSimulationPurposeRequest();
  const key = `${hashString(text)}:${JSON.stringify(purposeRequest)}`;
  if (!force && key === state.simulationPurposePlanKey && (state.simulationPurposePlan || state.simulationPurposePlanLoading)) {
    return state.simulationPurposePlan;
  }
  const sequence = ++simulationRunPlanSequence;
  state.simulationPurposePlanKey = key;
  state.simulationPurposePlanLoading = true;
  state.simulationPurposePlanError = "";
  renderSimulationRunPlanPreview();
  try {
    const plan = await callSimulationAPI("BuildSimulationRunPlan", "/api/simulation-run-plan", {
      text,
      inputPath: state.currentFilePath || "",
      filename: state.currentFilename || "current-input.idf",
      purposeRequest,
    });
    if (sequence !== simulationRunPlanSequence) {
      return plan;
    }
    state.simulationPurposePlan = plan;
    state.simulationPurposePlanError = "";
    return plan;
  } catch (error) {
    if (sequence === simulationRunPlanSequence) {
      state.simulationPurposePlan = null;
      state.simulationPurposePlanError = error?.message || String(error);
    }
    return null;
  } finally {
    if (sequence === simulationRunPlanSequence) {
      state.simulationPurposePlanLoading = false;
      renderSimulationRunPlanPreview();
    }
  }
}

async function applyPurposeOutputsToCurrentIDF() {
  const text = elements.idfInput?.value || "";
  if (!text.trim() || state.simulationRunning) {
    return;
  }
  try {
    elements.simulationApplyPurposeOutputs.disabled = true;
    setStatus(t("simulation.applyingPurposeOutputs", {}, "Applying purpose outputs"), "loading");
    const purposeRequest = buildSimulationPurposeRequest();
    const api = backend();
    let result;
    if (api && typeof api.ApplyPurposeOutputsText === "function") {
      result = await api.ApplyPurposeOutputsText(text, purposeRequest);
    } else {
      const response = await fetch("/api/simulation-purpose-outputs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ text, purpose: purposeRequest }),
      });
      if (!response.ok) {
        throw new Error(await response.text());
      }
      result = await response.json();
    }
    window.dispatchEvent(new CustomEvent("idfAnalyzer:outputApplied", { detail: result }));
    scheduleSimulationRunPlan(0);
  } catch (error) {
    setStatus(error?.message || String(error), "error");
  } finally {
    updatePurposeApplyButton();
  }
}

function updatePurposeApplyButton() {
  if (!elements.simulationApplyPurposeOutputs) {
    return;
  }
  const hasText = Boolean((elements.idfInput?.value || "").trim());
  const plan = state.simulationPurposePlan;
  const hasTemporaryOutputs = (plan?.outputObjects || []).some((object) => object.state !== "existing");
  elements.simulationApplyPurposeOutputs.disabled = state.simulationRunning || state.simulationPurposePlanLoading || !hasText || !plan || !hasTemporaryOutputs;
  elements.simulationApplyPurposeOutputs.title = hasTemporaryOutputs
    ? t("simulation.makePurposeOutputsPermanent", {}, "Make outputs permanent")
    : t("simulation.noTemporaryOutputs", {}, "No temporary purpose outputs need to be applied.");
}

function buildSimulationPurposeRequest() {
  return {
    purposes: selectedSimulationPurposes(),
    scope: {
      zoneMode: elements.simulationPurposeZoneMode?.value || "all",
      zoneNames: parseCommaList(elements.simulationPurposeZoneNames?.value || ""),
    },
    frequencyPolicy: elements.simulationPurposeFrequencyPolicy?.value || "purpose_default",
    sqlMode: "sql_first",
    persistOutputs: Boolean(elements.simulationPersistOutputs?.checked),
    discoveryAllowed: false,
  };
}

function selectedSimulationPurposes() {
  if (elements.simulationPurposeInputs?.length) {
    syncSimulationPurposeStateFromControls();
  }
  return state.simulationSelectedPurposes.length ? [...state.simulationSelectedPurposes] : ["basic_energy"];
}

function syncSimulationPurposeStateFromControls() {
  const selected = [];
  elements.simulationPurposeInputs?.forEach((input) => {
    if (input.checked) {
      selected.push(input.dataset.simulationPurpose);
    }
  });
  state.simulationSelectedPurposes = selected.length ? selected : ["basic_energy"];
}

function syncSimulationPurposeInputsFromState() {
  elements.simulationPurposeInputs?.forEach((input) => {
    input.checked = state.simulationSelectedPurposes.includes(input.dataset.simulationPurpose);
    input.closest(".simulation-purpose-card")?.classList.toggle("selected", input.checked);
  });
}

function parseCommaList(value) {
  const seen = new Set();
  const out = [];
  String(value || "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean)
    .forEach((item) => {
      const key = item.toLowerCase();
      if (!seen.has(key)) {
        seen.add(key);
        out.push(item);
      }
    });
  return out;
}

function purposeLabel(value) {
  const id = String(value || "");
  const found = simulationPurposeDefinitions.find((item) => item.id === id);
  return found ? found.label : id;
}

function outputStateLabel(value) {
  switch (value) {
    case "existing":
      return t("simulation.outputExisting", {}, "Already in IDF");
    case "temporary":
      return t("simulation.outputTemporary", {}, "Temporary");
    case "will_be_persisted":
      return t("simulation.outputWillPersist", {}, "Will persist");
    case "conflict":
      return t("simulation.outputConflict", {}, "Conflict");
    default:
      return value || "";
  }
}

function outputFieldValue(fields = [], ...names) {
  const wanted = new Set(names.map((name) => String(name || "").toLowerCase()));
  const found = fields.find((field) => wanted.has(String(field.name || "").toLowerCase()));
  return found?.value || "";
}

function setSimulationPreviewMode(preview) {
  elements.simulationResultSummary?.closest(".simulation-pane")?.classList.toggle("preview", Boolean(preview));
}

function updateSimulationOutputAvailability(blockingIssue, running) {
  const purposes = selectedSimulationPurposes();
  const heatFlowOn = purposes.includes("zone_heat_flow");
  const seriesOn = purposes.length > 0;
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
      : heatFlowOn
        ? t("simulation.heatFlowAvailable", {}, "Zone Heat Flow selected: SQL heat-flow ledger will be available after this run.")
        : t("simulation.heatFlowUnavailableStandardOff", {}, "Zone Heat Flow is not selected for this run.");
  }
  if (elements.simulationSeriesStats) {
    elements.simulationSeriesStats.textContent = blocked
      ? t("simulation.outputBlockedShort", {}, "Unavailable until run is possible")
      : seriesOn
        ? t("simulation.seriesAvailable", {}, "Purpose outputs selected: SQL/CSV time-series graph will be available after this run.")
        : t("simulation.seriesMaybeUnavailableStandardOff", {}, "No purpose outputs are selected; graph depends on existing output requests.");
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
    elements.simulationRunButton.textContent = state.simulationRunning ? t("simulation.runningShort", {}, "Running") : t("action.runSimulation", {}, "Run & Inspect");
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
  if (elements.simulationRefreshPlan) {
    elements.simulationRefreshPlan.disabled = state.simulationRunning || !hasText;
  }
  if (elements.simulationPurposeInputs?.length) {
    elements.simulationPurposeInputs.forEach((input) => {
      input.disabled = state.simulationRunning;
    });
  }
  updatePurposeApplyButton();
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
  const issueSummary = simulationIssueSummary(err.issues || []);
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
    <div class="simulation-issue-summary">
      ${issueSummary.map((item) => `<span class="${escapeHTML(item.key)}">${escapeHTML(item.label)} ${escapeHTML(item.count)}</span>`).join("")}
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

function simulationIssueSummary(issues = []) {
  const counts = { warning: 0, severe: 0, fatal: 0 };
  for (const issue of issues) {
    const key = String(issue.severity || "").toLowerCase();
    if (Object.prototype.hasOwnProperty.call(counts, key)) {
      counts[key]++;
    }
  }
  return [
    { key: "warning", label: t("simulation.errWarnings", {}, "Warnings"), count: counts.warning },
    { key: "severe", label: t("simulation.errSevere", {}, "Severe"), count: counts.severe },
    { key: "fatal", label: "Fatal", count: counts.fatal },
  ];
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
    state.simulationSeriesRangeStart = 0;
    state.simulationSeriesRangeEnd = -1;
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
  const visibleRange = normalizeSimulationSeriesRange(series.points.length);
  const visiblePoints = series.points.slice(visibleRange.start, visibleRange.end + 1);
  const width = 900;
  const height = 260;
  const pad = { left: 76, right: 18, top: 24, bottom: 42 };
  const values = visiblePoints.map((point) => Number(point.value)).filter(Number.isFinite);
  if (!values.length) {
    elements.simulationChart.innerHTML = `<div class="empty">${t("simulation.noGraph", {}, "SQL/CSV graph will appear after a run with numeric output.")}</div>`;
    return;
  }
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;
  const plotWidth = width - pad.left - pad.right;
  const plotHeight = height - pad.top - pad.bottom;
  const xStep = visiblePoints.length > 1 ? plotWidth / (visiblePoints.length - 1) : plotWidth;
  const yFor = (value) => pad.top + (height - pad.top - pad.bottom) * (1 - (value - min) / range);
  const points = visiblePoints
    .map((point, index) => `${pad.left + index * xStep},${yFor(Number(point.value))}`)
    .join(" ");
  const yTicks = [max, min + range / 2, min];
  const tickHTML = yTicks
    .map((value) => {
      const y = yFor(value);
      return `<g><line x1="${pad.left}" x2="${width - pad.right}" y1="${y}" y2="${y}" class="simulation-grid" /><text x="8" y="${y + 4}" class="simulation-axis">${escapeHTML(formatNumber(value))}</text></g>`;
    })
    .join("");
  const firstLabel = visiblePoints[0]?.label || "start";
  const lastLabel = visiblePoints[visiblePoints.length - 1]?.label || "end";
  const title = visiblePoints.length === series.points.length
    ? series.column
    : `${series.column} (${visibleRange.start + 1}-${visibleRange.end + 1} / ${series.points.length})`;
  elements.simulationChart.innerHTML = `
    <svg class="simulation-svg" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(series.column)}">
      ${tickHTML}
      <line x1="${pad.left}" x2="${pad.left}" y1="${pad.top}" y2="${height - pad.bottom}" class="simulation-axis-line" />
      <line x1="${pad.left}" x2="${width - pad.right}" y1="${height - pad.bottom}" y2="${height - pad.bottom}" class="simulation-axis-line" />
      <polyline points="${points}" class="simulation-line" />
      <rect class="simulation-chart-hit" x="${pad.left}" y="${pad.top}" width="${plotWidth}" height="${plotHeight}" data-simulation-chart-hit="1"></rect>
      <text x="${pad.left}" y="${height - 12}" class="simulation-axis">${escapeHTML(firstLabel)}</text>
      <text x="${width - pad.right}" y="${height - 12}" text-anchor="end" class="simulation-axis">${escapeHTML(lastLabel)}</text>
      <text x="${pad.left}" y="16" class="simulation-title">${escapeHTML(title)}</text>
    </svg>`;
  bindSimulationChartInteractions(series);
}

function normalizeSimulationSeriesRange(pointCount = 0) {
  const maxIndex = Math.max(0, Number(pointCount) - 1);
  let start = Math.round(clampNumber(state.simulationSeriesRangeStart, 0, maxIndex));
  let end = Number(state.simulationSeriesRangeEnd);
  if (!Number.isFinite(end) || end < 0 || end > maxIndex) {
    end = maxIndex;
  }
  end = Math.round(clampNumber(end, 0, maxIndex));
  if (start > end) {
    start = end;
  }
  state.simulationSeriesRangeStart = start;
  state.simulationSeriesRangeEnd = end;
  return { start, end };
}

function bindSimulationChartInteractions(series) {
  const hitTarget = elements.simulationChart?.querySelector("[data-simulation-chart-hit]");
  if (!hitTarget) {
    return;
  }
  hitTarget.addEventListener(
    "wheel",
    (event) => {
      event.preventDefault();
      zoomSimulationSeriesRange(event, series);
    },
    { passive: false },
  );
  hitTarget.addEventListener("dblclick", () => {
    state.simulationSeriesRangeStart = 0;
    state.simulationSeriesRangeEnd = -1;
    renderSimulationChart();
  });
}

function zoomSimulationSeriesRange(event, series) {
  const pointCount = series?.points?.length || 0;
  if (pointCount <= 2) {
    return;
  }
  const { start, end } = normalizeSimulationSeriesRange(pointCount);
  const currentSize = end - start + 1;
  const nextSize = event.deltaY < 0
    ? Math.max(6, Math.ceil(currentSize * 0.72))
    : Math.min(pointCount, Math.ceil(currentSize / 0.72));
  if (nextSize >= pointCount) {
    state.simulationSeriesRangeStart = 0;
    state.simulationSeriesRangeEnd = -1;
    renderSimulationChart();
    return;
  }
  const rect = event.currentTarget.getBoundingClientRect();
  const ratio = clampNumber((event.clientX - rect.left) / Math.max(rect.width, 1), 0, 1);
  const center = Math.round(start + ratio * Math.max(currentSize - 1, 0));
  let nextStart = Math.round(center - nextSize * ratio);
  let nextEnd = nextStart + nextSize - 1;
  if (nextStart < 0) {
    nextEnd -= nextStart;
    nextStart = 0;
  }
  if (nextEnd > pointCount - 1) {
    const overflow = nextEnd - (pointCount - 1);
    nextStart = Math.max(0, nextStart - overflow);
    nextEnd = pointCount - 1;
  }
  state.simulationSeriesRangeStart = nextStart;
  state.simulationSeriesRangeEnd = nextEnd;
  renderSimulationChart();
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
  const visibleRange = normalizeHeatFlowFrameRange(frameCount);
  state.simulationHeatFlowFrameIndex = clampNumber(state.simulationHeatFlowFrameIndex, visibleRange.start, visibleRange.end);
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
    elements.simulationHeatFlowSlider.min = String(visibleRange.start);
    elements.simulationHeatFlowSlider.max = String(visibleRange.end);
    elements.simulationHeatFlowSlider.value = String(frameIndex);
  }
  updateHeatFlowStorySelect(geometry);
  updateHeatFlowRangeControls(frameCount);
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
    ${renderHeatFlowTimelineBrush(dataset, zoneMap.get(normalizeHeatFlowName(selectedZone)), visibleRange, frameIndex)}
    ${renderHeatFlowSpatialToolbar()}
    <div class="heatflow-layout ${state.simulationHeatFlowInspectorCollapsed ? "inspector-collapsed" : ""}">
      <div class="heatflow-floor-grid">
        ${visibleHeatFlowStories(geometry).map((story) => renderHeatFlowStoryCard(geometry, story, dataset, zoneMap, frameIndex)).join("")}
      </div>
      <aside class="heatflow-inspector">
        ${renderHeatFlowInspector(dataset, zoneMap.get(normalizeHeatFlowName(selectedZone)), selectedZone, frameIndex)}
      </aside>
    </div>
    <div class="heatflow-tooltip hidden" role="tooltip"></div>`;
  bindHeatFlowInteractions(dataset, geometry, zoneMap);
}

function readHeatFlowInspectorCollapsed() {
  try {
    return localStorage.getItem("idfAnalyzer.heatFlowInspectorCollapsed") === "1";
  } catch {
    return false;
  }
}

function writeHeatFlowInspectorCollapsed(collapsed) {
  try {
    localStorage.setItem("idfAnalyzer.heatFlowInspectorCollapsed", collapsed ? "1" : "0");
  } catch {
    // localStorage can be unavailable in hardened webview settings.
  }
}

function renderHeatFlowGuide() {
  return `
    <div class="heatflow-reading-guide">
      <span><i class="heatflow-guide-fill"></i>${escapeHTML(t("simulation.heatFlowGuideFill", {}, "Zone fill shows the selected overlay: net heat flow or temperature."))}</span>
      <span><i class="heatflow-guide-stack"></i>${escapeHTML(t("simulation.heatFlowGuideStack", {}, "Stack bars show each heat-flow category; up is heat entering, down is heat leaving."))}</span>
      <span><i class="heatflow-guide-ring"></i>${escapeHTML(t("simulation.heatFlowGuideRing", {}, "The +/- ring marks the zone's net direction and relative magnitude. Click a zone for the ledger."))}</span>
    </div>`;
}

function renderHeatFlowSpatialToolbar() {
  const collapsed = Boolean(state.simulationHeatFlowInspectorCollapsed);
  return `
    <div class="heatflow-spatial-toolbar">
      <div class="heatflow-plan-actions" role="group" aria-label="${escapeHTML(t("simulation.heatFlowPlanView", {}, "Heat-flow plan view"))}">
        <button type="button" data-heatflow-plan-zoom="out" title="${escapeHTML(t("action.zoomOut", {}, "Zoom out"))}">-</button>
        <button type="button" data-heatflow-plan-zoom="reset">${escapeHTML(t("action.fit", {}, "Fit"))}</button>
        <button type="button" data-heatflow-plan-zoom="in" title="${escapeHTML(t("action.zoomIn", {}, "Zoom in"))}">+</button>
      </div>
      <button class="heatflow-inspector-toggle ${collapsed ? "" : "active"}" type="button" data-heatflow-inspector-toggle aria-expanded="${collapsed ? "false" : "true"}">
        ${escapeHTML(collapsed ? t("simulation.showHeatFlowLedger", {}, "Show ledger") : t("simulation.hideHeatFlowLedger", {}, "Hide ledger"))}
      </button>
    </div>`;
}

function renderHeatFlowTimelineBrush(dataset, zoneSeries, visibleRange, frameIndex) {
  const frameCount = Math.max(0, Number(dataset?.frameCount) || dataset?.labels?.length || 0);
  if (frameCount <= 1) {
    return "";
  }
  const width = 760;
  const height = 92;
  const pad = { left: 38, right: 18, top: 14, bottom: 24 };
  const plotWidth = width - pad.left - pad.right;
  const plotHeight = height - pad.top - pad.bottom;
  const sampleCount = Math.min(frameCount, 240);
  const values = [];
  for (let index = 0; index < sampleCount; index += 1) {
    const frame = sampleCount === 1 ? 0 : Math.round(index * (frameCount - 1) / (sampleCount - 1));
    values.push({ frame, value: heatFlowTimelineValue(dataset, zoneSeries, frame) });
  }
  const maxAbs = Math.max(1, ...values.map((point) => Math.abs(point.value || 0)));
  const yMid = pad.top + plotHeight / 2;
  const path = values
    .map((point, index) => {
      const x = pad.left + (point.frame / Math.max(frameCount - 1, 1)) * plotWidth;
      const y = yMid - (point.value / maxAbs) * (plotHeight / 2 - 3);
      return `${index === 0 ? "M" : "L"} ${roundSVG(x)} ${roundSVG(y)}`;
    })
    .join(" ");
  const rangeStartX = pad.left + (visibleRange.start / Math.max(frameCount - 1, 1)) * plotWidth;
  const rangeEndX = pad.left + (visibleRange.end / Math.max(frameCount - 1, 1)) * plotWidth;
  const cursorX = pad.left + (clampNumber(frameIndex, 0, frameCount - 1) / Math.max(frameCount - 1, 1)) * plotWidth;
  const presets = [
    ["reset", "Reset"],
    ["fit", "Fit"],
    ["day", "Day"],
    ["week", "Week"],
    ["month", "Month"],
    ["runperiod", "RunPeriod"],
  ];
  return `
    <div class="heatflow-timeline">
      <div class="heatflow-range-actions">
        ${presets.map(([preset, label]) => `<button type="button" data-heatflow-range-preset="${escapeHTML(preset)}">${escapeHTML(label)}</button>`).join("")}
      </div>
      <svg class="heatflow-timeline-brush" viewBox="0 0 ${width} ${height}" role="img" aria-label="Heat-flow visible frame range">
        <line x1="${pad.left}" x2="${width - pad.right}" y1="${roundSVG(yMid)}" y2="${roundSVG(yMid)}" class="simulation-axis-line" />
        ${path ? `<path d="${path}" class="heatflow-timeline-line" />` : ""}
        <rect x="${roundSVG(rangeStartX)}" y="${pad.top}" width="${roundSVG(Math.max(2, rangeEndX - rangeStartX))}" height="${plotHeight}" class="heatflow-range-window"></rect>
        <line x1="${roundSVG(cursorX)}" x2="${roundSVG(cursorX)}" y1="${pad.top}" y2="${pad.top + plotHeight}" class="heatflow-cursor" />
        <rect class="heatflow-brush-hit" x="${pad.left}" y="${pad.top}" width="${plotWidth}" height="${plotHeight}" data-heatflow-brush="1"></rect>
        <text x="${pad.left}" y="${height - 7}" class="simulation-axis">${escapeHTML(dataset.labels?.[0] || "start")}</text>
        <text x="${width - pad.right}" y="${height - 7}" text-anchor="end" class="simulation-axis">${escapeHTML(dataset.labels?.[frameCount - 1] || "end")}</text>
      </svg>
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
  if (elements.simulationHeatFlowStory) {
    elements.simulationHeatFlowStory.disabled = true;
    elements.simulationHeatFlowStory.innerHTML = `<option value="all">${escapeHTML(t("common.all", {}, "All"))}</option>`;
    elements.simulationHeatFlowStory.value = "all";
  }
  for (const rangeInput of [elements.simulationHeatFlowRangeStart, elements.simulationHeatFlowRangeEnd]) {
    if (rangeInput) {
      rangeInput.disabled = true;
      rangeInput.min = "0";
      rangeInput.max = "0";
      rangeInput.value = "0";
    }
  }
  if (elements.simulationHeatFlow) {
    elements.simulationHeatFlow.innerHTML = `<div class="empty">${escapeHTML(message)}</div>`;
  }
}

function normalizeHeatFlowFrameRange(frameCount = Number(state.simulationResult?.heatFlow?.frameCount) || 0, changed = "") {
  const maxIndex = Math.max(0, Number(frameCount) - 1);
  let start = Math.round(clampNumber(state.simulationHeatFlowRangeStart, 0, maxIndex));
  let end = Number(state.simulationHeatFlowRangeEnd);
  if (!Number.isFinite(end) || end < 0 || end > maxIndex) {
    end = maxIndex;
  }
  end = Math.round(clampNumber(end, 0, maxIndex));
  if (start > end) {
    if (changed === "start") {
      end = start;
    } else {
      start = end;
    }
  }
  state.simulationHeatFlowRangeStart = start;
  state.simulationHeatFlowRangeEnd = end;
  state.simulationHeatFlowFrameIndex = clampNumber(state.simulationHeatFlowFrameIndex, start, end);
  return { start, end };
}

function updateHeatFlowStorySelect(geometry) {
  if (!elements.simulationHeatFlowStory) {
    return;
  }
  const stories = geometry?.stories || [];
  const hasSelectedStory = state.simulationHeatFlowStory === "all" || stories.some((story) => String(story.index) === String(state.simulationHeatFlowStory));
  if (!hasSelectedStory) {
    state.simulationHeatFlowStory = "all";
  }
  elements.simulationHeatFlowStory.disabled = stories.length <= 1;
  elements.simulationHeatFlowStory.innerHTML = [
    `<option value="all">${escapeHTML(t("common.all", {}, "All"))}</option>`,
    ...stories.map((story) => `<option value="${escapeHTML(story.index)}">${escapeHTML(story.name || `Level ${story.index + 1}`)}</option>`),
  ].join("");
  elements.simulationHeatFlowStory.value = state.simulationHeatFlowStory;
}

function updateHeatFlowRangeControls(frameCount) {
  const { start, end } = normalizeHeatFlowFrameRange(frameCount);
  const maxIndex = Math.max(0, Number(frameCount) - 1);
  const controls = [
    { element: elements.simulationHeatFlowRangeStart, value: start },
    { element: elements.simulationHeatFlowRangeEnd, value: end },
  ];
  controls.forEach(({ element, value }) => {
    if (!element) {
      return;
    }
    element.disabled = frameCount <= 1;
    element.min = "0";
    element.max = String(maxIndex);
    element.value = String(value);
  });
}

function visibleHeatFlowStories(geometry) {
  const stories = geometry?.stories || [];
  if (state.simulationHeatFlowStory === "all") {
    return stories;
  }
  const selected = stories.find((story) => String(story.index) === String(state.simulationHeatFlowStory));
  return selected ? [selected] : stories;
}

function heatFlowVisibleRange(dataset) {
  const frameCount = Math.max(0, Number(dataset?.frameCount) || dataset?.labels?.length || 0);
  return normalizeHeatFlowFrameRange(frameCount);
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
      <svg class="heatflow-floor-plan" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(story.name || "Floor")} heat-flow plan" data-heatflow-plan="1">
        <g class="heatflow-plan-content" data-heatflow-plan-content transform="${escapeHTML(heatFlowPlanTransform())}">
          ${shapes.join("")}
        </g>
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
  const fullFrameCount = Math.max(dataset.frameCount || 0, dataset.labels?.length || 0);
  if (!fullFrameCount) {
    return `<div class="empty">${escapeHTML(t("simulation.noFrame", {}, "No frame"))}</div>`;
  }
  const { start, end } = heatFlowVisibleRange(dataset);
  const frameCount = Math.max(1, end - start + 1);
  const width = 760;
  const height = 260;
  const pad = { left: 70, right: 16, top: 18, bottom: 34 };
  const plotWidth = width - pad.left - pad.right;
  const plotHeight = height - pad.top - pad.bottom;
  const stacks = heatFlowFrameStackExtents(dataset, zoneSeries, start, end);
  const maxAbs = Math.max(Math.abs(stacks.positive), Math.abs(stacks.negative), 1);
  const yZero = pad.top + plotHeight / 2;
  const yScale = (plotHeight / 2 - 6) / maxAbs;
  const barWidth = Math.max(1, plotWidth / frameCount);
  const bars = [];
  for (let frame = start; frame <= end; frame += 1) {
    const visibleIndex = frame - start;
    const x = pad.left + visibleIndex * barWidth;
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
  const cursorFrame = clampNumber(frameIndex, start, end);
  const cursorX = pad.left + (cursorFrame - start) * barWidth + barWidth / 2;
  const firstLabel = dataset.labels?.[start] || `Frame ${start + 1}`;
  const lastLabel = dataset.labels?.[end] || `Frame ${end + 1}`;
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
  host.querySelectorAll("[data-heatflow-range-preset]").forEach((button) => {
    button.addEventListener("click", () => applyHeatFlowRangePreset(dataset, button.dataset.heatflowRangePreset || "fit"));
  });
  host.querySelectorAll("[data-heatflow-plan-zoom]").forEach((button) => {
    button.addEventListener("click", () => applyHeatFlowPlanZoomButton(host, button.dataset.heatflowPlanZoom || "reset"));
  });
  host.querySelector("[data-heatflow-inspector-toggle]")?.addEventListener("click", () => {
    state.simulationHeatFlowInspectorCollapsed = !state.simulationHeatFlowInspectorCollapsed;
    writeHeatFlowInspectorCollapsed(state.simulationHeatFlowInspectorCollapsed);
    renderSimulationHeatFlow();
  });
  bindHeatFlowTimelineBrush(host, dataset);
  bindHeatFlowPlanInteractions(host);
  host.querySelectorAll("[data-heat-zone]").forEach((shape) => {
    shape.addEventListener("pointerenter", (event) => showHeatFlowTooltip(event, shape.dataset.heatZone, dataset, zoneMap, tooltip));
    shape.addEventListener("pointermove", (event) => showHeatFlowTooltip(event, shape.dataset.heatZone, dataset, zoneMap, tooltip));
    shape.addEventListener("pointerleave", () => hideHeatFlowTooltip(tooltip));
    shape.addEventListener("click", () => {
      state.simulationHeatFlowSelectedZone = shape.dataset.heatZone || "";
      renderSimulationHeatFlow();
    });
  });
  const chartTarget = host.querySelector("[data-heatflow-chart]");
  chartTarget?.addEventListener("pointermove", (event) => {
    const rect = event.currentTarget.getBoundingClientRect();
    const { start, end } = heatFlowVisibleRange(dataset);
    const frameCount = Math.max(1, end - start + 1);
    const ratio = clampNumber((event.clientX - rect.left) / rect.width, 0, 1);
    const nextFrame = clampNumber(start + Math.round(ratio * (frameCount - 1)), start, end);
    if (nextFrame === state.simulationHeatFlowFrameIndex) {
      return;
    }
    state.simulationHeatFlowFrameIndex = nextFrame;
    renderSimulationHeatFlow();
  });
  chartTarget?.addEventListener("pointerleave", () => hideHeatFlowTooltip(tooltip));
  chartTarget?.addEventListener(
    "wheel",
    (event) => {
      event.preventDefault();
      zoomHeatFlowRange(event, dataset);
    },
    { passive: false },
  );
  chartTarget?.addEventListener("dblclick", () => {
    state.simulationHeatFlowRangeStart = 0;
    state.simulationHeatFlowRangeEnd = -1;
    normalizeHeatFlowFrameRange(Number(dataset.frameCount) || 0);
    renderSimulationHeatFlow();
  });
  void geometry;
}

function heatFlowPlanTransform() {
  const scale = clampNumber(Number(state.simulationHeatFlowPlanScale) || 1, 0.5, 4);
  state.simulationHeatFlowPlanScale = scale;
  const panX = Number(state.simulationHeatFlowPlanPanX) || 0;
  const panY = Number(state.simulationHeatFlowPlanPanY) || 0;
  return `translate(${roundSVG(panX)} ${roundSVG(panY)}) scale(${roundSVG(scale)})`;
}

function bindHeatFlowPlanInteractions(host) {
  host.querySelectorAll("[data-heatflow-plan]").forEach((svg) => {
    svg.addEventListener(
      "wheel",
      (event) => {
        event.preventDefault();
        applyHeatFlowPlanZoom(host, event.deltaY < 0 ? 1.18 : 1 / 1.18, heatFlowSVGPoint(svg, event));
      },
      { passive: false },
    );
    svg.addEventListener("dblclick", (event) => {
      event.preventDefault();
      resetHeatFlowPlanTransform(host);
    });
    svg.addEventListener("pointerdown", (event) => {
      if (event.button !== 0 || event.target.closest("[data-heat-zone]")) {
        return;
      }
      event.preventDefault();
      const rect = svg.getBoundingClientRect();
      const viewBox = svg.viewBox.baseVal;
      const unitsPerPixelX = viewBox.width / Math.max(rect.width, 1);
      const unitsPerPixelY = viewBox.height / Math.max(rect.height, 1);
      const start = {
        x: event.clientX,
        y: event.clientY,
        panX: Number(state.simulationHeatFlowPlanPanX) || 0,
        panY: Number(state.simulationHeatFlowPlanPanY) || 0,
      };
      const move = (moveEvent) => {
        state.simulationHeatFlowPlanPanX = start.panX + (moveEvent.clientX - start.x) * unitsPerPixelX;
        state.simulationHeatFlowPlanPanY = start.panY + (moveEvent.clientY - start.y) * unitsPerPixelY;
        updateHeatFlowPlanTransformNodes(host);
      };
      const end = (endEvent) => {
        svg.classList.remove("panning");
        svg.releasePointerCapture?.(endEvent.pointerId);
        svg.removeEventListener("pointermove", move);
      };
      svg.classList.add("panning");
      svg.setPointerCapture?.(event.pointerId);
      svg.addEventListener("pointermove", move);
      svg.addEventListener("pointerup", end, { once: true });
      svg.addEventListener("pointercancel", end, { once: true });
    });
  });
}

function applyHeatFlowPlanZoomButton(host, action) {
  if (action === "reset") {
    resetHeatFlowPlanTransform(host);
    return;
  }
  const svg = host.querySelector("[data-heatflow-plan]");
  const viewBox = svg?.viewBox?.baseVal;
  const anchor = viewBox
    ? { x: viewBox.x + viewBox.width / 2, y: viewBox.y + viewBox.height / 2 }
    : { x: 230, y: 120 };
  applyHeatFlowPlanZoom(host, action === "in" ? 1.25 : 0.8, anchor);
}

function applyHeatFlowPlanZoom(host, factor, anchor) {
  const previousScale = clampNumber(Number(state.simulationHeatFlowPlanScale) || 1, 0.5, 4);
  const nextScale = clampNumber(previousScale * factor, 0.5, 4);
  const ratio = nextScale / previousScale;
  const panX = Number(state.simulationHeatFlowPlanPanX) || 0;
  const panY = Number(state.simulationHeatFlowPlanPanY) || 0;
  state.simulationHeatFlowPlanScale = nextScale;
  state.simulationHeatFlowPlanPanX = anchor.x - (anchor.x - panX) * ratio;
  state.simulationHeatFlowPlanPanY = anchor.y - (anchor.y - panY) * ratio;
  updateHeatFlowPlanTransformNodes(host);
}

function resetHeatFlowPlanTransform(host) {
  state.simulationHeatFlowPlanScale = 1;
  state.simulationHeatFlowPlanPanX = 0;
  state.simulationHeatFlowPlanPanY = 0;
  updateHeatFlowPlanTransformNodes(host);
}

function updateHeatFlowPlanTransformNodes(host) {
  const transform = heatFlowPlanTransform();
  host.querySelectorAll("[data-heatflow-plan-content]").forEach((node) => node.setAttribute("transform", transform));
}

function heatFlowSVGPoint(svg, event) {
  const rect = svg.getBoundingClientRect();
  const viewBox = svg.viewBox.baseVal;
  return {
    x: viewBox.x + clampNumber((event.clientX - rect.left) / Math.max(rect.width, 1), 0, 1) * viewBox.width,
    y: viewBox.y + clampNumber((event.clientY - rect.top) / Math.max(rect.height, 1), 0, 1) * viewBox.height,
  };
}

function zoomHeatFlowRange(event, dataset) {
  const frameCount = Math.max(0, Number(dataset?.frameCount) || dataset?.labels?.length || 0);
  if (frameCount <= 2) {
    return;
  }
  const { start, end } = heatFlowVisibleRange(dataset);
  const currentSize = end - start + 1;
  if (event.shiftKey) {
    panHeatFlowRange(event, dataset, currentSize);
    return;
  }
  const nextSize = event.deltaY < 0
    ? Math.max(4, Math.ceil(currentSize * 0.7))
    : Math.min(frameCount, Math.ceil(currentSize / 0.7));
  if (nextSize >= frameCount) {
    state.simulationHeatFlowRangeStart = 0;
    state.simulationHeatFlowRangeEnd = -1;
    normalizeHeatFlowFrameRange(frameCount);
    renderSimulationHeatFlow();
    return;
  }
  const rect = event.currentTarget.getBoundingClientRect();
  const ratio = clampNumber((event.clientX - rect.left) / Math.max(rect.width, 1), 0, 1);
  const center = Math.round(start + ratio * Math.max(currentSize - 1, 0));
  let nextStart = Math.round(center - nextSize * ratio);
  let nextEnd = nextStart + nextSize - 1;
  if (nextStart < 0) {
    nextEnd -= nextStart;
    nextStart = 0;
  }
  if (nextEnd > frameCount - 1) {
    const overflow = nextEnd - (frameCount - 1);
    nextStart = Math.max(0, nextStart - overflow);
    nextEnd = frameCount - 1;
  }
  state.simulationHeatFlowRangeStart = nextStart;
  state.simulationHeatFlowRangeEnd = nextEnd;
  state.simulationHeatFlowFrameIndex = clampNumber(state.simulationHeatFlowFrameIndex, nextStart, nextEnd);
  renderSimulationHeatFlow();
}

function panHeatFlowRange(event, dataset, currentSize) {
  const frameCount = Math.max(0, Number(dataset?.frameCount) || dataset?.labels?.length || 0);
  const { start, end } = heatFlowVisibleRange(dataset);
  if (currentSize >= frameCount) {
    return;
  }
  const delta = Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY;
  const direction = delta < 0 ? -1 : 1;
  const step = Math.max(1, Math.round(currentSize * 0.15));
  let nextStart = clampNumber(start + direction * step, 0, Math.max(0, frameCount - currentSize));
  let nextEnd = nextStart + currentSize - 1;
  if (nextEnd > frameCount - 1) {
    nextEnd = frameCount - 1;
    nextStart = Math.max(0, nextEnd - currentSize + 1);
  }
  state.simulationHeatFlowRangeStart = nextStart;
  state.simulationHeatFlowRangeEnd = nextEnd;
  state.simulationHeatFlowFrameIndex = clampNumber(state.simulationHeatFlowFrameIndex, nextStart, nextEnd);
  renderSimulationHeatFlow();
}

function applyHeatFlowRangePreset(dataset, preset) {
  const frameCount = Math.max(0, Number(dataset?.frameCount) || dataset?.labels?.length || 0);
  if (frameCount <= 1) {
    return;
  }
  const sizes = {
    day: 24,
    week: 24 * 7,
    month: 24 * 30,
  };
  if (preset === "reset" || preset === "fit" || preset === "runperiod" || !sizes[preset]) {
    state.simulationHeatFlowRangeStart = 0;
    state.simulationHeatFlowRangeEnd = -1;
    normalizeHeatFlowFrameRange(frameCount);
    renderSimulationHeatFlow();
    return;
  }
  const size = Math.min(frameCount, sizes[preset]);
  setHeatFlowRangeAroundFrame(frameCount, size, state.simulationHeatFlowFrameIndex);
  renderSimulationHeatFlow();
}

function setHeatFlowRangeAroundFrame(frameCount, size, frame) {
  const center = clampNumber(Math.round(frame), 0, frameCount - 1);
  let start = Math.round(center - (size - 1) / 2);
  let end = start + size - 1;
  if (start < 0) {
    end -= start;
    start = 0;
  }
  if (end > frameCount - 1) {
    const overflow = end - (frameCount - 1);
    start = Math.max(0, start - overflow);
    end = frameCount - 1;
  }
  state.simulationHeatFlowRangeStart = start;
  state.simulationHeatFlowRangeEnd = end;
  state.simulationHeatFlowFrameIndex = clampNumber(state.simulationHeatFlowFrameIndex, start, end);
}

function bindHeatFlowTimelineBrush(host, dataset) {
  const brush = host.querySelector("[data-heatflow-brush]");
  if (!brush) {
    return;
  }
  let startFrame = null;
  brush.addEventListener("pointerdown", (event) => {
    startFrame = heatFlowFrameFromBrushEvent(event, dataset);
    brush.setPointerCapture?.(event.pointerId);
  });
  brush.addEventListener("pointerup", (event) => {
    if (startFrame === null) {
      return;
    }
    const endFrame = heatFlowFrameFromBrushEvent(event, dataset);
    if (Math.abs(endFrame - startFrame) <= 1) {
      state.simulationHeatFlowFrameIndex = endFrame;
    } else {
      state.simulationHeatFlowRangeStart = Math.min(startFrame, endFrame);
      state.simulationHeatFlowRangeEnd = Math.max(startFrame, endFrame);
      state.simulationHeatFlowFrameIndex = clampNumber(state.simulationHeatFlowFrameIndex, state.simulationHeatFlowRangeStart, state.simulationHeatFlowRangeEnd);
    }
    startFrame = null;
    normalizeHeatFlowFrameRange(Math.max(0, Number(dataset?.frameCount) || dataset?.labels?.length || 0));
    renderSimulationHeatFlow();
  });
  brush.addEventListener("pointercancel", () => {
    startFrame = null;
  });
}

function heatFlowFrameFromBrushEvent(event, dataset) {
  const frameCount = Math.max(1, Number(dataset?.frameCount) || dataset?.labels?.length || 1);
  const rect = event.currentTarget.getBoundingClientRect();
  const ratio = clampNumber((event.clientX - rect.left) / Math.max(rect.width, 1), 0, 1);
  return clampNumber(Math.round(ratio * (frameCount - 1)), 0, frameCount - 1);
}

function heatFlowTimelineValue(dataset, zoneSeries, frameIndex) {
  if (zoneSeries) {
    return heatFlowZoneNet(zoneSeries, dataset, frameIndex);
  }
  return (dataset.zones || []).reduce((sum, zone) => sum + heatFlowZoneNet(zone, dataset, frameIndex), 0);
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
  const { start, end } = heatFlowVisibleRange(dataset);
  if (end <= start) {
    return;
  }
  state.simulationHeatFlowFrameIndex = clampNumber(state.simulationHeatFlowFrameIndex, start, end);
  state.simulationHeatFlowPlaying = true;
  const delay = Math.max(80, Number(elements.simulationHeatFlowSpeed?.value) || 420);
  heatFlowPlayTimer = window.setInterval(() => {
    const nextRange = heatFlowVisibleRange(dataset);
    const current = clampNumber(state.simulationHeatFlowFrameIndex, nextRange.start, nextRange.end);
    state.simulationHeatFlowFrameIndex = current >= nextRange.end ? nextRange.start : current + 1;
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

function heatFlowFrameStackExtents(dataset, zoneSeries, start = 0, end = Number(dataset.frameCount) - 1) {
  let positive = 0;
  let negative = 0;
  const frameCount = Number(dataset.frameCount) || 0;
  const first = clampNumber(start, 0, Math.max(frameCount - 1, 0));
  const last = clampNumber(end, first, Math.max(frameCount - 1, 0));
  for (let frame = first; frame <= last; frame += 1) {
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
  const purposeRequest = buildSimulationPurposeRequest();
  const request = {
    runId: runID,
    text,
    inputPath: state.currentFilePath || "",
    filename: state.currentFilename || "current-input.idf",
    energyPlusExecutablePath: installPath,
    weatherPath: elements.simulationWeatherSelect?.value || "",
    standardOutput: false,
    purposeRequest,
    resultMode: "sql_first",
    useReadVarsESO: false,
    silent,
    auto,
  };
  try {
    const result = await callSimulationAPI("RunPurposeSimulationText", "/api/simulation-run", request);
    if (state.simulationActiveRunID !== runID) {
      return result;
    }
    state.simulationResult = result;
    state.simulationPurposePlan = result?.purposeRunPlan || state.simulationPurposePlan;
    state.simulationRunning = false;
    state.simulationStale = state.simulationRunText !== (elements.idfInput?.value || "");
    state.simulationSeriesRangeStart = 0;
    state.simulationSeriesRangeEnd = -1;
    state.simulationHeatFlowRangeStart = 0;
    state.simulationHeatFlowRangeEnd = -1;
    state.simulationHeatFlowFrameIndex = 0;
    state.simulationHeatFlowStory = "all";
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
