import { backend, elements, escapeHTML, setStatus, state } from "../state.js";
import { t } from "../i18n.js";
import { navigateHVAC, renderHVACLoopDiagram } from "./hvac-views.js";
import { openStandardOutputApplyDialog } from "./output-views.js";

let progressListenerRegistered = false;
let heatFlowPlayTimer = 0;
let simulationRunPlanTimer = 0;
let simulationRunPlanSequence = 0;

const simulationCustomOutputsStorageKey = "idfAnalyzer.simulationCustomOutputs";

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
  restoreSimulationCustomOutputsPreset();
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
  elements.simulationPurposePeriodMode?.addEventListener("change", () => {
    scheduleSimulationRunPlan();
    renderSimulation();
  });
  elements.simulationPurposePeriodStart?.addEventListener("input", () => scheduleSimulationRunPlan());
  elements.simulationPurposePeriodEnd?.addEventListener("input", () => scheduleSimulationRunPlan());
  elements.simulationPurposeFrequencyPolicy?.addEventListener("change", () => scheduleSimulationRunPlan());
  elements.simulationPurposeAllocationPolicy?.addEventListener("change", () => scheduleSimulationRunPlan());
  elements.simulationPurposeApplyMode?.addEventListener("change", () => updatePurposeApplyButton());
  elements.simulationCustomOutputs?.addEventListener("input", () => {
    saveSimulationCustomOutputsPreset();
    scheduleSimulationRunPlan();
  });
  elements.simulationOutputDiscoveryFilter?.addEventListener("input", () => {
    state.simulationOutputDiscoveryQuery = elements.simulationOutputDiscoveryFilter.value || "";
    renderSimulationOutputDiscovery();
  });
  elements.simulationOutputDiscoveryRefresh?.addEventListener("click", () => refreshSimulationOutputDiscovery({ force: true }));
  elements.simulationOutputDiscoveryList?.addEventListener("click", (event) => {
    if (!(event.target instanceof Element)) {
      return;
    }
    const button = event.target.closest("[data-simulation-discovery-add]");
    if (!button) {
      return;
    }
    appendDiscoveredCustomOutput(Number(button.dataset.simulationDiscoveryAdd));
  });
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
  elements.simulationIntegrityFilter?.addEventListener("input", () => {
    state.simulationIntegrityQuery = elements.simulationIntegrityFilter.value || "";
    if (state.simulationResult && elements.simulationResultSummary) {
      elements.simulationResultSummary.innerHTML = `${state.simulationRunning ? renderRunningNotice() : ""}${renderSimulationSummary(state.simulationResult, state.simulationStale)}`;
    }
  });
  elements.simulationEnergyDashboard?.addEventListener("click", handleSimulationSeriesInspectClick);
  elements.simulationEnergyDashboard?.addEventListener("change", handleSimulationEnergyDashboardChange);
  elements.simulationHVACLoopResults?.addEventListener("click", handleSimulationSeriesInspectClick);
  elements.simulationHVACLoopResults?.addEventListener("input", handleSimulationHVACResultsInput);
  elements.simulationHVACLoopResults?.addEventListener("change", handleSimulationHVACResultsInput);
  elements.simulationComfortResults?.addEventListener("click", handleSimulationSeriesInspectClick);
  elements.simulationComfortResults?.addEventListener("change", handleSimulationComfortResultsChange);
  elements.simulationExportPurposeJSON?.addEventListener("click", () => exportPurposeResultJSON());
  elements.simulationExportPurposeHTML?.addEventListener("click", () => exportPurposeResultHTML());
  elements.simulationRefreshEnv?.addEventListener("click", () => loadSimulationEnvironment());
  elements.simulationRunButton?.addEventListener("click", () => runCurrentSimulation({ silent: false }));
  elements.simulationEnergyPlusSelect?.addEventListener("change", () => renderSimulation());
  elements.simulationWeatherSelect?.addEventListener("change", () => renderSimulation());
  elements.simulationStandardOutput?.addEventListener("change", () => {
    state.simulationStandardOutput = elements.simulationStandardOutput.checked;
    renderSimulation();
  });
  elements.simulationSeriesGroup?.addEventListener("change", () => {
    state.simulationSeriesGroup = elements.simulationSeriesGroup.value || "all";
    state.simulationSelectedSeries = "";
    state.simulationSeriesRangeStart = 0;
    state.simulationSeriesRangeEnd = -1;
    renderSimulationSeriesSelect(state.simulationResult || {});
    renderSimulationChart();
  });
  elements.simulationSeriesSelect?.addEventListener("change", () => {
    state.simulationSelectedSeries = elements.simulationSeriesSelect.value || "";
    state.simulationSeriesRangeStart = 0;
    state.simulationSeriesRangeEnd = -1;
    renderSimulationChart();
  });
  elements.simulationSeriesRangeAll?.addEventListener("click", () => {
    state.simulationSeriesRangeStart = 0;
    state.simulationSeriesRangeEnd = -1;
    renderSimulationChart();
  });
  elements.simulationSeriesRangeStart?.addEventListener("input", () => updateSimulationSeriesRangeFromControls("start"));
  elements.simulationSeriesRangeEnd?.addEventListener("input", () => updateSimulationSeriesRangeFromControls("end"));
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
  window.addEventListener("idfAnalyzer:hvacSelectionChanged", () => {
    renderSimulationPurposeSetup();
    scheduleSimulationRunPlan(0);
  });
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
  if (elements.simulationIntegrityFilter && elements.simulationIntegrityFilter.value !== state.simulationIntegrityQuery) {
    elements.simulationIntegrityFilter.value = state.simulationIntegrityQuery || "";
  }
  elements.simulationResultSummary.innerHTML = `${state.simulationRunning ? renderRunningNotice() : ""}${renderSimulationSummary(result, stale)}`;
  renderSimulationEnergyDashboard(result);
  renderSimulationHVACLoops(result);
  renderSimulationComfort(result);
  renderSimulationHeatFlow();
  renderSimulationSeriesSelect(result);
  renderSimulationCustomSeriesLinks(result);
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
  renderSimulationSeriesRangeControls(null);
  const installCount = state.simulationEnvironment?.installations?.length || 0;
  const hasText = Boolean((elements.idfInput?.value || "").trim());
  const blockingIssue = simulationBlockingIssue();
  if (state.simulationRunning) {
    setSimulationPreviewMode(false);
    renderSimulationResultTabs(null);
    elements.simulationStats.textContent = t("simulation.runningStats", {}, "Simulation running in background");
    elements.simulationResultMeta.textContent = t("simulation.backgroundRun", {}, "EnergyPlus is running in the background");
    elements.simulationResultSummary.innerHTML = renderRunningNotice();
    renderSimulationHeatFlowEmpty(t("simulation.heatFlowAfterRun", {}, "The heat-flow ledger will appear when Zone Heat Flow SQL/CSV output is available."));
    setSimulationSeriesGroupUnavailable();
    renderSimulationCustomSeriesLinks(null);
    elements.simulationSeriesSelect.innerHTML = `<option value="">${escapeHTML(t("simulation.waitingForCSV", {}, "Waiting for SQL/CSV output"))}</option>`;
    elements.simulationChart.innerHTML = `<div class="simulation-running-empty">${renderMiniProgressSVG()}<span>${escapeHTML(t("simulation.graphAfterRun", {}, "The SQL/CSV graph will appear when the run finishes."))}</span></div>`;
    elements.simulationFiles.innerHTML = `<div class="empty status-loading">${escapeHTML(t("simulation.writingOutputs", {}, "EnergyPlus is writing output files"))}</div>`;
    renderSimulationEnergyEmpty(t("simulation.outputPending", {}, "Outputs are pending while EnergyPlus runs."));
    renderSimulationHVACLoopEmpty(t("simulation.outputPending", {}, "Outputs are pending while EnergyPlus runs."));
    renderSimulationComfortEmpty(t("simulation.outputPending", {}, "Outputs are pending while EnergyPlus runs."));
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
    setSimulationSeriesGroupUnavailable();
    renderSimulationCustomSeriesLinks(null);
    elements.simulationSeriesSelect.innerHTML = `<option value="">${escapeHTML(t("simulation.noSeries", {}, "No SQL/CSV series"))}</option>`;
    elements.simulationChart.innerHTML = `<div class="simulation-blocked-empty">${escapeHTML(t("simulation.blockedGraph", {}, "Graph output is unavailable until the run can start."))}</div>`;
    elements.simulationFiles.innerHTML = `<div class="simulation-blocked-empty">${escapeHTML(t("simulation.blockedFiles", {}, "No output files will be created while simulation is blocked."))}</div>`;
    renderSimulationEnergyEmpty(t("simulation.outputBlocked", {}, "Run requirements must be fixed before outputs are available."));
    renderSimulationHVACLoopEmpty(t("simulation.outputBlocked", {}, "Run requirements must be fixed before outputs are available."));
    renderSimulationComfortEmpty(t("simulation.outputBlocked", {}, "Run requirements must be fixed before outputs are available."));
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
  elements.simulationResultSummary.innerHTML = `<div class="empty">${t("simulation.noResult", {}, "Run & Inspect will show purpose results, integrity diagnostics, and SQL/CSV outputs.")}</div>`;
  renderSimulationResultTabs(null);
  renderSimulationEnergyEmpty(t("simulation.noEnergyResult", {}, "Run Basic Energy to inspect monthly energy results."));
  renderSimulationHVACLoopEmpty(t("simulation.noHVACLoopResult", {}, "Run HVAC Loop Check to inspect node state series."));
  renderSimulationComfortEmpty(t("simulation.noComfortResult", {}, "Run Comfort Check to inspect zone temperature and setpoint series."));
  renderSimulationHeatFlowEmpty(t("simulation.noHeatFlow", {}, "Select Zone Heat Flow to inspect the zone heat-flow ledger."));
  setSimulationSeriesGroupUnavailable();
  renderSimulationCustomSeriesLinks(null);
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
    const hvacScope = simulationHVACScopeSummary(selected);
    elements.simulationPurposeStats.textContent = selected.length
      ? [selected.map(purposeLabel).join(" + "), hvacScope].filter(Boolean).join(" | ")
      : t("simulation.noPurposeSelected", {}, "No purposes selected");
  }
  if (elements.simulationPurposeZoneNames) {
    const zoneMode = elements.simulationPurposeZoneMode?.value || "all";
    const editableMode = zoneMode === "selected" || zoneMode === "filtered";
    elements.simulationPurposeZoneNames.disabled = !editableMode;
    elements.simulationPurposeZoneNames.closest("label")?.classList.toggle("disabled", !editableMode);
  }
  if (elements.simulationCustomOutputs) {
    const customSelected = selected.includes("custom_outputs");
    elements.simulationCustomOutputs.disabled = !customSelected;
    elements.simulationCustomOutputs.closest("label")?.classList.toggle("disabled", !customSelected);
    elements.simulationOutputDiscoveryFilter?.toggleAttribute("disabled", !customSelected);
    elements.simulationOutputDiscoveryRefresh?.toggleAttribute("disabled", !customSelected);
    elements.simulationOutputDiscoveryList?.classList.toggle("disabled", !customSelected);
  }
  renderSimulationOutputDiscovery();
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
  state.simulationActiveResultView = ["energy", "zone_heat_flow", "hvac_loops", "comfort", "integrity", "series", "files"].find((view) => availability[view]) || "energy";
}

function simulationResultViewAvailability(result) {
  if (!result) {
    return { energy: true, zone_heat_flow: true, hvac_loops: true, comfort: true, integrity: true, series: true, files: true };
  }
  const energy = result.purposeResults?.energy || {};
  const hvacLoops = result.purposeResults?.hvacLoops || [];
  const comfort = result.purposeResults?.comfort || {};
  return {
    energy: Boolean((energy.facilityMonthly || []).length || (energy.endUseMonthly || []).length || (energy.zoneMonthly || []).length),
    zone_heat_flow: Boolean((result.heatFlow?.zones || []).length),
    hvac_loops: Boolean(hvacLoops.some((loop) => (loop.series || []).length || hvacComponentSeriesCount(loop.components || []))),
    comfort: Boolean((comfort.zones || []).length || (comfort.series || []).length),
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
  const explanation = result?.purposeResults?.energyExplanation || {};
  const facility = energy.facilityMonthly || [];
  const endUse = energy.endUseMonthly || [];
  const zones = energy.zoneMonthly || [];
  const explanationNodes = energyExplanationGraphForPeriod(explanation, state.simulationEnergyPeriod || "annual").nodes || [];
  if (!facility.length && !endUse.length && !zones.length && !explanationNodes.length) {
    renderSimulationEnergyEmpty(t("simulation.noEnergyResult", {}, "Run Basic Energy to inspect monthly energy results."));
    return;
  }
  if (elements.simulationEnergyStats) {
    elements.simulationEnergyStats.textContent = t(
      "simulation.energyStats",
      { facility: facility.length, enduse: endUse.length, zones: zones.length, nodes: explanationNodes.length },
      `${facility.length} facility, ${endUse.length} end-use, ${zones.length} zone series, ${explanationNodes.length} explanation nodes`,
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
  const view = energySubview(explanation);
  const viewBody = renderEnergySubview(view, energy, explanation, facility, endUse, zones);
  elements.simulationEnergyDashboard.innerHTML = `
    <div class="simulation-energy-kpis">${kpis || `<div><span>${escapeHTML(t("common.notAvailable", {}, "N/A"))}</span><strong>0</strong></div>`}</div>
    ${renderEnergySubviewControls(view, explanation)}
    ${viewBody}`;
}

function energySubview(explanation = {}) {
  const allowed = ["overview", "sankey", "monthly", "zones", "systems", "sources", "reconciliation"];
  let view = state.simulationEnergyView || "overview";
  if (!allowed.includes(view)) {
    view = "overview";
  }
  if (["sankey", "sources", "reconciliation"].includes(view) && !energyExplanationHasPayload(explanation)) {
    view = "overview";
  }
  state.simulationEnergyView = view;
  return view;
}

function energyExplanationHasPayload(explanation = {}) {
  return Boolean((explanation.nodes || []).length || (explanation.sources || []).length || (explanation.reconciliation || []).length);
}

function renderEnergySubviewControls(view, explanation = {}) {
  const tabs = [
    ["overview", t("simulation.energyOverview", {}, "Overview")],
    ["sankey", t("simulation.energySankey", {}, "Sankey")],
    ["monthly", t("simulation.monthly", {}, "Monthly")],
    ["zones", t("simulation.zones", {}, "Zones")],
    ["systems", t("simulation.systems", {}, "Systems")],
    ["sources", t("simulation.energySources", {}, "Sources")],
    ["reconciliation", t("simulation.energyReconciliation", {}, "Reconciliation")],
  ];
  const periodOptions = (explanation.periods || [])
    .map((period) => `<option value="${escapeHTML(period.id || "")}" ${(state.simulationEnergyPeriod || "annual") === period.id ? "selected" : ""}>${escapeHTML(period.label || period.id || "")}</option>`)
    .join("");
  return `
    <div class="simulation-energy-subnav">
      <div class="view-tabs" role="tablist" aria-label="${escapeHTML(t("simulation.energyViews", {}, "Energy views"))}">
        ${tabs
          .map(([id, label]) => {
            const disabled = ["sankey", "sources", "reconciliation"].includes(id) && !energyExplanationHasPayload(explanation);
            return `<button class="view-tab ${view === id ? "active" : ""}" type="button" data-simulation-energy-view="${id}" ${disabled ? "disabled" : ""}>${escapeHTML(label)}</button>`;
          })
          .join("")}
      </div>
      ${
        ["sankey", "systems", "reconciliation"].includes(view) && periodOptions
          ? `<label><span>${escapeHTML(t("common.period", {}, "Period"))}</span><select data-simulation-energy-period>${periodOptions}</select></label>`
          : ""
      }
      ${["sankey", "systems"].includes(view) && energyExplanationHasPayload(explanation) ? renderEnergyFocusControls(explanation) : ""}
      ${view === "sankey" && energyExplanationHasPayload(explanation) ? renderEnergySignModeControls() : ""}
      ${view === "sankey" && energyExplanationHasPayload(explanation) ? renderEnergyNodeLimitControls() : ""}
    </div>`;
}

function renderEnergySignModeControls() {
  const mode = energyExplanationSignMode(state.simulationEnergySignMode || "display");
  state.simulationEnergySignMode = mode;
  const options = [
    ["display", t("simulation.energyDisplayMagnitude", {}, "Display magnitude")],
    ["signed", t("simulation.energySignedBalance", {}, "Signed balance")],
    ["cooling_pressure", t("simulation.coolingPressure", {}, "Cooling pressure")],
    ["heating_pressure", t("simulation.heatingPressure", {}, "Heating pressure")],
  ]
    .map(([value, label]) => `<option value="${escapeHTML(value)}" ${mode === value ? "selected" : ""}>${escapeHTML(label)}</option>`)
    .join("");
  return `
    <label>
      <span>${escapeHTML(t("simulation.heatDriverView", {}, "Heat driver view"))}</span>
      <select data-simulation-energy-sign-mode>${options}</select>
    </label>`;
}

function renderEnergyNodeLimitControls() {
  const limit = energyExplanationNodeLimit(state.simulationEnergyNodeLimit);
  state.simulationEnergyNodeLimit = limit;
  const options = [
    [40, "Top 40"],
    [80, "Top 80"],
    [120, "Top 120"],
    [0, t("common.all", {}, "All")],
  ]
    .map(([value, label]) => `<option value="${escapeHTML(value)}" ${limit === value ? "selected" : ""}>${escapeHTML(label)}</option>`)
    .join("");
  return `
    <label>
      <span>${escapeHTML(t("simulation.nodeLimit", {}, "Nodes"))}</span>
      <select data-simulation-energy-node-limit>${options}</select>
    </label>`;
}

function renderEnergyFocusControls(explanation = {}) {
  const graph = energyExplanationGraphForPeriod(explanation, state.simulationEnergyPeriod || "annual");
  const zones = energyFocusZones(graph.nodes || []);
  const paths = simulationHVACServicePaths();
  let mode = state.simulationEnergyFocusMode || "all";
  if (mode === "zone" && !zones.length) {
    mode = "all";
  }
  if (mode === "service_path" && !paths.length) {
    mode = "all";
  }
  state.simulationEnergyFocusMode = mode;
  if (mode === "zone" && !zones.some((zone) => simulationZoneKey(zone) === simulationZoneKey(state.simulationEnergyZoneFocus))) {
    state.simulationEnergyZoneFocus = zones[0] || "";
  }
  if (mode === "service_path" && !paths.some((path) => path.id === state.simulationEnergyServicePathFocus)) {
    state.simulationEnergyServicePathFocus = paths[0]?.id || "";
  }
  const modeOptions = [
    ["all", t("common.all", {}, "All")],
    ["zone", t("common.zone", {}, "Zone")],
    ["service_path", t("hvac.servicePath", {}, "Service path")],
  ]
    .map(([value, label]) => {
      const disabled = value === "zone" && !zones.length || value === "service_path" && !paths.length;
      return `<option value="${escapeHTML(value)}" ${mode === value ? "selected" : ""} ${disabled ? "disabled" : ""}>${escapeHTML(label)}</option>`;
    })
    .join("");
  const zoneSelect = mode === "zone"
    ? `<label><span>${escapeHTML(t("common.zone", {}, "Zone"))}</span><select data-simulation-energy-zone-focus>${zones
        .map((zone) => `<option value="${escapeHTML(zone)}" ${simulationZoneKey(zone) === simulationZoneKey(state.simulationEnergyZoneFocus) ? "selected" : ""}>${escapeHTML(zone)}</option>`)
        .join("")}</select></label>`
    : "";
  const pathSelect = mode === "service_path"
    ? `<label><span>${escapeHTML(t("hvac.servicePath", {}, "Service path"))}</span><select data-simulation-energy-service-path-focus>${paths
        .map((path) => `<option value="${escapeHTML(path.id || "")}" ${path.id === state.simulationEnergyServicePathFocus ? "selected" : ""}>${escapeHTML(simulationEnergyServicePathFocusLabel(path))}</option>`)
        .join("")}</select></label>`
    : "";
  return `
    <div class="simulation-energy-focus-controls">
      <label><span>${escapeHTML(t("simulation.focus", {}, "Focus"))}</span><select data-simulation-energy-focus-mode>${modeOptions}</select></label>
      ${zoneSelect}
      ${pathSelect}
    </div>`;
}

function renderEnergySubview(view, energy, explanation, facility, endUse, zones) {
  switch (view) {
    case "sankey":
      return renderEnergyExplanationSankey(explanation);
    case "monthly":
      return renderEnergyMonthlySubview(explanation, facility, endUse);
    case "zones":
      return renderEnergyZonesSubview(zones, explanation);
    case "systems":
      return renderEnergySystemsSubview(explanation);
    case "sources":
      return renderEnergyExplanationSources(explanation);
    case "reconciliation":
      return renderEnergyExplanationReconciliation(explanation);
    default:
      return `
        ${renderPurposeCompletenessRow(energy.completeness || [])}
        ${renderEnergyExplanationCompleteness(explanation)}
        ${renderEnergyDerivedKPISection(explanation)}
        ${renderEnergyUseBreakdownSection(explanation)}
        ${renderEnergyMonthlyChart(t("simulation.facilityMonthlyProfile", {}, "Facility monthly profile"), facility)}
        ${renderEnergyMonthlyChart(t("simulation.endUseMonthlyProfile", {}, "End-use monthly profile"), endUse)}
        ${renderEnergyBarSection(t("simulation.facilityEnergy", {}, "Facility energy"), facility)}
        ${renderEnergyBarSection(t("simulation.endUseEnergy", {}, "End-use energy"), endUse)}
        ${renderZoneEnergyMatrix(zones)}
        ${renderZoneEnergyTable(zones)}`;
  }
}

function renderEnergyUseBreakdownSection(explanation = {}) {
  const graph = energyExplanationGraphForPeriod(explanation, "annual");
  const rows = (graph.nodes || [])
    .filter((node) => node.level === "energy" && String(node.id || "").includes(".end_use.") && node.endUse && node.endUse !== "total")
    .sort((a, b) => {
      const carrierCompare = String(a.carrier || "").localeCompare(String(b.carrier || ""));
      if (carrierCompare) return carrierCompare;
      return Math.abs(Number(b.value) || 0) - Math.abs(Number(a.value) || 0);
    })
    .map(
      (node) => `
        <tr>
          <td>${escapeHTML(titleCaseEnergyToken(node.carrier || ""))}</td>
          <td>${escapeHTML(titleCaseEnergyToken(node.endUse || ""))}</td>
          <td>${escapeHTML(node.label || node.kind || node.id || "")}</td>
          <td>${escapeHTML(formatValueWithUnit(node.value, node.unit))}</td>
          <td>${renderEnergyReconciliationSources(explanation, node.sourceIds || [])}</td>
        </tr>`,
    )
    .join("");
  if (!rows) {
    return "";
  }
  return `
    <section class="simulation-energy-block">
      <div class="simulation-energy-block-head">
        <h4>${escapeHTML(t("simulation.energyUseBreakdown", {}, "Energy use by carrier and end use"))}</h4>
        <span>${escapeHTML(t("simulation.energyUseBreakdownHint", {}, "Carrier and end use stay separate for comparison."))}</span>
      </div>
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>${escapeHTML(t("simulation.carrier", {}, "Carrier"))}</th><th>${escapeHTML(t("simulation.endUse", {}, "End use"))}</th><th>${escapeHTML(t("common.metric", {}, "Metric"))}</th><th>${escapeHTML(t("common.value", {}, "Value"))}</th><th>${escapeHTML(t("common.source", {}, "Source"))}</th></tr></thead>
          <tbody>${rows}</tbody>
        </table>
      </div>
    </section>`;
}

function renderEnergySystemsSubview(explanation = {}) {
  const graph = focusedEnergyExplanationGraph(energyExplanationGraphForPeriod(explanation, state.simulationEnergyPeriod || "annual"));
  const rows = simulationEnergyServiceRows(graph.nodes || []);
  const hasHVAC = simulationHVACServicePaths().length > 0;
  return `
    ${renderEnergyExplanationCompleteness(explanation)}
    <section class="simulation-energy-block">
      <div class="simulation-energy-block-head">
        <h4>${escapeHTML(t("simulation.energySystems", {}, "Systems"))}</h4>
        <span>${escapeHTML(hasHVAC ? t("simulation.energySystemsHint", {}, "Service paths are matched by zone and service kind.") : t("hvac.noServicePaths", {}, "No service paths"))}</span>
      </div>
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>Zone</th><th>Service</th><th>${escapeHTML(t("hvac.delivery", {}, "Delivery"))}</th><th>${escapeHTML(t("hvac.connectedSystems", {}, "Connected systems"))}</th><th>Path type</th><th>Load</th><th>Heat drivers</th></tr></thead>
          <tbody>${rows || `<tr><td colspan="7">${escapeHTML(hasHVAC ? t("simulation.noRelatedSystems", {}, "No related service paths for the current energy graph.") : t("hvac.noServicePaths", {}, "No service paths"))}</td></tr>`}</tbody>
        </table>
      </div>
    </section>`;
}

function renderEnergyDerivedKPISection(explanation = {}) {
  const graph = energyExplanationGraphForPeriod(explanation, "annual");
  const rows = energyExplanationDerivedKPIItems(graph.nodes || [])
    .map(
      (item) => `
        <tr>
          <td>${escapeHTML(simulationServiceKindLabel(item.serviceKind))}</td>
          <td>${escapeHTML(simulationPathTypeLabel(item.pathType))}</td>
          <td>${escapeHTML(formatValueWithUnit(item.loadValue, "kWh"))}</td>
          <td>${escapeHTML(formatValueWithUnit(item.energyValue, "kWh"))}</td>
          <td>${escapeHTML(formatNumber(item.value))}</td>
        </tr>`,
    )
    .join("");
  if (!rows) {
    return "";
  }
  return `
    <section class="simulation-energy-block">
      <div class="simulation-energy-block-head">
        <h4>${escapeHTML(t("simulation.derivedKpis", {}, "Derived KPIs"))}</h4>
        <span>${escapeHTML(t("simulation.annual", {}, "Annual"))}</span>
      </div>
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>Service</th><th>Path type</th><th>Delivered load</th><th>Electric energy</th><th>COP</th></tr></thead>
          <tbody>${rows}</tbody>
        </table>
      </div>
    </section>`;
}

function energyExplanationDerivedKPIItems(nodes = []) {
  return [
    { serviceKind: "cooling", label: "Cooling COP" },
    { serviceKind: "heating", label: "Heating COP" },
  ]
    .map((definition) => {
      const energy = energyExplanationElectricEndUseNode(nodes, definition.serviceKind);
      const load = energyExplanationPreferredLoadNode(nodes, definition.serviceKind);
      const energyValue = Number(energy?.value) || 0;
      const loadValue = Number(load?.displayValue ?? load?.value) || 0;
      if (!energy || !load || energyValue <= 0 || loadValue <= 0) {
        return null;
      }
      return {
        ...definition,
        value: loadValue / energyValue,
        energyValue,
        loadValue,
        pathType: load.pathType || "",
      };
    })
    .filter(Boolean);
}

function energyExplanationElectricEndUseNode(nodes = [], serviceKind = "") {
  const service = simulationCanonicalServiceKind(serviceKind);
  return (nodes || []).find(
    (node) =>
      node.level === "energy" &&
      simulationCanonicalServiceKind(node.endUse) === service &&
      String(node.carrier || "").toLowerCase() === "electricity" &&
      Number(node.value) > 0,
  ) || null;
}

function energyExplanationPreferredLoadNode(nodes = [], serviceKind = "") {
  const service = simulationCanonicalServiceKind(serviceKind);
  let best = null;
  let bestRank = Number.POSITIVE_INFINITY;
  for (const node of nodes || []) {
    if (node.level !== "load" || simulationCanonicalServiceKind(node.serviceKind || energyServiceFromNode(node)) !== service) {
      continue;
    }
    const value = Number(node.displayValue ?? node.value) || 0;
    if (value <= 0) {
      continue;
    }
    const rank = energyExplanationLoadPathPriority(node.pathType);
    if (!best || rank < bestRank || (rank === bestRank && value > (Number(best.displayValue ?? best.value) || 0))) {
      best = node;
      bestRank = rank;
    }
  }
  return best;
}

function energyExplanationLoadPathPriority(pathType = "") {
  switch (String(pathType || "").toLowerCase()) {
    case "zone":
      return 0;
    case "system":
      return 1;
    case "plant":
      return 2;
    default:
      return 3;
  }
}

function simulationEnergyServiceRows(nodes = []) {
  const paths = simulationRelatedServicePathsForEnergyNodes(nodes);
  const loadByService = new Map();
  const loadByZoneService = new Map();
  const heatByZoneService = new Map();
  nodes.forEach((node) => {
    const service = simulationCanonicalServiceKind(node.serviceKind || energyServiceFromNode(node));
    if (!service) {
      return;
    }
    const value = Number(node.displayValue ?? node.value) || 0;
    if (node.level === "load") {
      loadByService.set(service, (loadByService.get(service) || 0) + value);
      if (node.zoneName) {
        const key = `${simulationZoneKey(node.zoneName || "")}|${service}`;
        loadByZoneService.set(key, (loadByZoneService.get(key) || 0) + value);
      }
    } else if (node.level === "heat") {
      const zoneKey = simulationZoneKey(node.zoneName || "");
      const key = `${zoneKey}|${service}`;
      heatByZoneService.set(key, (heatByZoneService.get(key) || 0) + value);
    }
  });
  return paths
    .map((path) => {
      const service = simulationCanonicalServiceKind(path.serviceKind);
      const zoneServiceKey = `${simulationZoneKey(path.zoneName || path.servedSubject?.zoneName || "")}|${service}`;
      const heat = heatByZoneService.get(zoneServiceKey) || 0;
      const load = loadByZoneService.get(zoneServiceKey) || loadByService.get(service) || 0;
      return `
        <tr>
          <td>${renderSimulationEnergyServicePathButton(path, simulationServedSubjectLabel(path.servedSubject || path))}</td>
          <td>${escapeHTML(simulationServiceKindLabel(path.serviceKind))}</td>
          <td>${escapeHTML(simulationDeliveryLabel(path))}</td>
          <td>${escapeHTML(simulationServicePathConnectedSystems(path).join(", ") || "N/A")}</td>
          <td>${escapeHTML(simulationPathTypeLabel(path.pathType))}</td>
          <td>${escapeHTML(formatValueWithUnit(load, "kWh"))}</td>
          <td>${escapeHTML(formatValueWithUnit(heat, "kWh"))}</td>
        </tr>`;
    })
    .join("");
}

function renderEnergyMonthlySubview(explanation = {}, facility = [], endUse = []) {
  const rows = (explanation.periods || [])
    .filter((period) => period.kind === "monthly")
    .map((period) => {
      const totals = energyExplanationLevelTotals(period.nodes || []);
      return `
        <tr class="simulation-energy-period-row" data-simulation-energy-period-jump="${escapeHTML(period.id || "")}">
          <td><button type="button" data-simulation-energy-period-jump="${escapeHTML(period.id || "")}">${escapeHTML(period.label || period.id || "")}</button></td>
          <td>${escapeHTML(formatValueWithUnit(totals.energy, totals.unit))}</td>
          <td>${escapeHTML(formatValueWithUnit(totals.load, totals.unit))}</td>
          <td>${escapeHTML(formatValueWithUnit(totals.heat, totals.unit))}</td>
          <td>${escapeHTML(formatValueWithUnit(totals.residual, totals.unit))}</td>
        </tr>`;
    })
    .join("");
  return `
    ${renderEnergyExplanationCompleteness(explanation)}
    ${renderEnergyMonthlyChart(t("simulation.facilityMonthlyProfile", {}, "Facility monthly profile"), facility)}
    ${renderEnergyMonthlyChart(t("simulation.endUseMonthlyProfile", {}, "End-use monthly profile"), endUse)}
    <section class="simulation-energy-block">
      <div class="simulation-energy-block-head">
        <h4>${escapeHTML(t("simulation.energyExplanationMonthly", {}, "Explanation monthly ledger"))}</h4>
        <span>${escapeHTML(t("simulation.energyBasisNote", {}, "Basis is accounting/source type, not confidence."))}</span>
      </div>
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>${escapeHTML(t("common.period", {}, "Period"))}</th><th>Energy Use</th><th>Delivered Load</th><th>Heat Drivers</th><th>Residual</th></tr></thead>
          <tbody>${rows || `<tr><td colspan="5">${escapeHTML(t("common.notAvailable", {}, "N/A"))}</td></tr>`}</tbody>
        </table>
      </div>
    </section>`;
}

function renderEnergyZonesSubview(zones = [], explanation = {}) {
  const heatRows = (explanation.nodes || [])
    .filter((node) => node.level === "heat" && node.zoneName)
    .sort((a, b) => {
      const zoneCompare = String(a.zoneName || "").localeCompare(String(b.zoneName || ""));
      if (zoneCompare) return zoneCompare;
      return Math.abs(Number(b.value) || 0) - Math.abs(Number(a.value) || 0);
    })
    .map(
      (node) => `
        <tr>
          <td>${escapeHTML(node.zoneName || "")}</td>
          <td>${escapeHTML(node.label || node.kind || "")}</td>
          <td>${escapeHTML(node.heatCategory || "")}</td>
          <td>${escapeHTML(node.serviceKind || "")}</td>
          <td>${escapeHTML(formatValueWithUnit(node.value, node.unit))}</td>
          <td>${escapeHTML(formatValueWithUnit(node.signedValue, node.unit))}</td>
          <td>${escapeHTML((node.sourceIds || []).join(", ") || "-")}</td>
        </tr>`,
    )
    .join("");
  return `
    ${renderEnergyExplanationCompleteness(explanation)}
    ${renderZoneEnergyMatrix(zones)}
    ${renderZoneEnergyTable(zones)}
    <section class="simulation-energy-block">
      <div class="simulation-energy-block-head">
        <h4>${escapeHTML(t("simulation.zoneHeatDrivers", {}, "Zone heat drivers"))}</h4>
        <span>${escapeHTML(t("simulation.energyBasisNote", {}, "Basis is accounting/source type, not confidence."))}</span>
      </div>
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>${escapeHTML(t("common.zone", {}, "Zone"))}</th><th>Driver</th><th>Category</th><th>Service</th><th>Display</th><th>Signed</th><th>${escapeHTML(t("common.source", {}, "Source"))}</th></tr></thead>
          <tbody>${heatRows || `<tr><td colspan="7">${escapeHTML(t("common.notAvailable", {}, "N/A"))}</td></tr>`}</tbody>
        </table>
      </div>
    </section>`;
}

function energyExplanationLevelTotals(nodes = []) {
  const totals = { energy: 0, load: 0, heat: 0, residual: 0, unit: "kWh" };
  nodes.forEach((node) => {
    const value = Number(node.displayValue ?? node.value) || 0;
    if (node.unit) {
      totals.unit = node.unit;
    }
    if (node.level === "energy" && String(node.id || "").includes(".carrier.")) {
      totals.energy += value;
    } else if (node.level === "load") {
      totals.load += value;
    } else if (node.level === "heat") {
      totals.heat += value;
    } else if (node.level === "residual") {
      totals.residual += value;
    }
  });
  totals.energy = Math.round(totals.energy * 1000) / 1000;
  totals.load = Math.round(totals.load * 1000) / 1000;
  totals.heat = Math.round(totals.heat * 1000) / 1000;
  totals.residual = Math.round(totals.residual * 1000) / 1000;
  return totals;
}

function renderEnergyExplanationCompleteness(explanation = {}) {
  const completeness = explanation.completeness || {};
  const items = completeness.items || [completeness.energyUse, completeness.deliveredLoad, completeness.heatDrivers].filter(Boolean);
  if (!items.length) {
    return "";
  }
  const allocationPolicy = energyAllocationPolicyLabel(explanation.allocationPolicy || "direct_only");
  const missingCategories = completeness.missingCategories || [];
  const availabilityRows = (completeness.sourceAvailability || [])
    .filter((item) => item.status && item.status !== "found")
    .slice(0, 12)
    .map(
      (item) => `
        <tr>
          <td>${escapeHTML(energyExplanationLevelLabel(item.level || ""))}</td>
          <td>${escapeHTML(item.name || "")}</td>
          <td>${escapeHTML(item.status || "")}</td>
        </tr>`,
    )
    .join("");
  return `
    <section class="simulation-energy-block simulation-energy-explain-status">
      <div class="simulation-energy-block-head">
        <h4>${escapeHTML(t("simulation.energyExplanation", {}, "Energy explanation"))}</h4>
        <span>${escapeHTML(t("simulation.mappedEnergy", {}, "Mapped energy"))}: ${escapeHTML(formatNumber(completeness.mappedPercent || 0))}% / ${escapeHTML(t("simulation.allocationPolicy", {}, "Allocation"))}: ${escapeHTML(allocationPolicy)}</span>
      </div>
      <div class="simulation-energy-explain-grid">
        ${items
          .map(
            (item) => `
              <article class="${escapeHTML(item.status || "missing")}">
                <span>${escapeHTML(energyExplanationLevelLabel(item.level || ""))}</span>
                <strong>${escapeHTML(item.status || "")}</strong>
                <small>${escapeHTML(item.message || `${item.found || 0}/${item.total || 0}`)}</small>
              </article>`,
          )
          .join("")}
      </div>
      ${
        missingCategories.length
          ? `<div class="energy-explanation-missing"><strong>${escapeHTML(t("simulation.missingOutputs", {}, "Missing outputs"))}</strong><span>${escapeHTML(missingCategories.join(", "))}</span></div>`
          : ""
      }
      ${
        availabilityRows
          ? `<div class="energy-source-availability">
              <table>
                <thead><tr><th>Level</th><th>${escapeHTML(t("common.output", {}, "Output"))}</th><th>Status</th></tr></thead>
                <tbody>${availabilityRows}</tbody>
              </table>
            </div>`
          : ""
      }
    </section>`;
}

function energyAllocationPolicyLabel(policy = "") {
  switch (policy) {
    case "direct_only":
      return t("simulation.allocationDirectOnly", {}, "Direct only");
    case "by_zone_load_share":
      return t("simulation.allocationByZoneLoadShare", {}, "By zone load share");
    case "by_service_path_load_share":
      return t("simulation.allocationByServicePathLoadShare", {}, "By service path load share");
    default:
      return titleCaseEnergyToken(policy || "direct_only");
  }
}

function renderEnergyExplanationSankey(explanation = {}) {
  const graph = groupedEnergyExplanationGraph(energyExplanationSignModeGraph(focusedEnergyExplanationGraph(energyExplanationGraphForPeriod(explanation, state.simulationEnergyPeriod || "annual"))));
  const nodes = graph.nodes || [];
  const edges = graph.edges || [];
  if (!nodes.length) {
    return `<div class="empty">${escapeHTML(t("simulation.noEnergyExplanation", {}, "No energy explanation graph is available."))}</div>`;
  }
  const selected = energyExplanationSelection(explanation, graph);
  const svg = renderEnergyExplanationSVG(nodes, edges);
  return `
    ${renderEnergyExplanationCompleteness(explanation)}
    <section class="simulation-energy-block">
      <div class="simulation-energy-block-head">
        <h4>${escapeHTML(t("simulation.energyPath", {}, "Energy Use -> Delivered Load -> Heat Drivers"))}</h4>
        <span>${escapeHTML(t("simulation.energyBasisNote", {}, "Basis is accounting/source type, not confidence."))} / ${escapeHTML(energyExplanationSignModeLabel(state.simulationEnergySignMode || "display"))}</span>
      </div>
      ${svg}
      ${renderEnergyExplanationLegend()}
      ${renderEnergyExplanationInspector(selected, explanation)}
    </section>`;
}

function energyExplanationSignModeGraph(graph = {}) {
  const mode = energyExplanationSignMode(state.simulationEnergySignMode || "display");
  if (mode === "display") {
    return graph;
  }
  const sourceNodes = graph.nodes || [];
  const nodes = sourceNodes
    .filter((node) => energyExplanationNodeVisibleForSignMode(node, mode))
    .map((node) => energyExplanationNodeForSignMode(node, mode));
  const nodeIDs = new Set(nodes.map((node) => node.id));
  const edges = (graph.edges || [])
    .filter((edge) => nodeIDs.has(edge.fromId) && nodeIDs.has(edge.toId) && energyExplanationEdgeVisibleForSignMode(edge, mode, sourceNodes))
    .map((edge) => energyExplanationEdgeForSignMode(edge, mode));
  return { nodes, edges };
}

function energyExplanationSignMode(mode = "") {
  return ["display", "signed", "cooling_pressure", "heating_pressure"].includes(mode) ? mode : "display";
}

function energyExplanationSignModeLabel(mode = "") {
  const labels = {
    display: t("simulation.energyDisplayMagnitude", {}, "Display magnitude"),
    signed: t("simulation.energySignedBalance", {}, "Signed balance"),
    cooling_pressure: t("simulation.coolingPressure", {}, "Cooling pressure"),
    heating_pressure: t("simulation.heatingPressure", {}, "Heating pressure"),
  };
  return labels[energyExplanationSignMode(mode)] || labels.display;
}

function energyExplanationNodeVisibleForSignMode(node = {}, mode = "display") {
  if (node.level !== "heat") {
    return true;
  }
  const signed = Number(node.signedValue ?? node.value) || 0;
  if (mode === "cooling_pressure") {
    return signed >= 0;
  }
  if (mode === "heating_pressure") {
    return signed < 0;
  }
  return true;
}

function energyExplanationEdgeVisibleForSignMode(edge = {}, mode = "display", nodes = []) {
  if (edge.relation !== "heat_driver") {
    return true;
  }
  const signed = Number(edge.signedValue ?? edge.value) || 0;
  if (mode === "cooling_pressure") {
    return signed >= 0;
  }
  if (mode === "heating_pressure") {
    return signed < 0;
  }
  const target = nodes.find((node) => node.id === edge.toId);
  return energyExplanationNodeVisibleForSignMode(target || {}, mode);
}

function energyExplanationNodeForSignMode(node = {}, mode = "display") {
  if (node.level !== "heat") {
    return { ...node };
  }
  const signed = Number(node.signedValue ?? node.value) || 0;
  const displayValue = Number(node.displayValue ?? node.value) || 0;
  const value = mode === "signed" ? signed : Math.abs(mode === "display" ? displayValue : signed);
  return { ...node, value };
}

function energyExplanationEdgeForSignMode(edge = {}, mode = "display") {
  if (edge.relation !== "heat_driver") {
    return { ...edge };
  }
  const signed = Number(edge.signedValue ?? edge.value) || 0;
  const displayValue = Number(edge.displayValue ?? edge.value) || 0;
  const value = mode === "signed" ? signed : Math.abs(mode === "display" ? displayValue : signed);
  return { ...edge, value };
}

function groupedEnergyExplanationGraph(graph = {}) {
  const limit = energyExplanationNodeLimit(state.simulationEnergyNodeLimit);
  if (!limit) {
    return graph;
  }
  const nodes = graph.nodes || [];
  const heatNodes = nodes.filter((node) => node.level === "heat");
  if (heatNodes.length <= limit) {
    return graph;
  }
  const keepHeatIDs = new Set(
    heatNodes
      .slice()
      .sort((a, b) => Math.abs(Number(b.value) || 0) - Math.abs(Number(a.value) || 0))
      .slice(0, limit)
      .map((node) => node.id),
  );
  const omittedHeatNodes = heatNodes.filter((node) => !keepHeatIDs.has(node.id));
  const omittedHeatIDs = new Set(omittedHeatNodes.map((node) => node.id));
  const otherID = "heat.other_grouped";
  const otherSources = omittedHeatNodes.reduce((out, node) => appendUniqueEnergyStrings(out, ...(node.sourceIds || [])), []);
  const otherValue = roundedDisplayNumber(omittedHeatNodes.reduce((sum, node) => sum + (Number(node.value) || 0), 0));
  const otherUnit = omittedHeatNodes.find((node) => node.unit)?.unit || "kWh";
  const groupedNodes = nodes.filter((node) => node.level !== "heat" || keepHeatIDs.has(node.id));
  groupedNodes.push({
    id: otherID,
    level: "heat",
    kind: "heat.other_grouped",
    label: "Other heat drivers",
    value: otherValue,
    unit: otherUnit,
    heatCategory: "grouped_other",
    basis: "derived_balance",
    sourceIds: otherSources,
  });
  const groupedEdges = [];
  const otherEdges = new Map();
  for (const edge of graph.edges || []) {
    if (!omittedHeatIDs.has(edge.toId)) {
      groupedEdges.push(edge);
      continue;
    }
    const key = edge.fromId || "unknown";
    const existing = otherEdges.get(key) || {
      ...edge,
      id: `edge.grouped_other.${metricTokenForEnergyID(key)}`,
      toId: otherID,
      value: 0,
      signedValue: 0,
      displayValue: 0,
      sourceIds: [],
      formula: "grouped omitted heat-driver edges",
    };
    existing.value = roundedDisplayNumber((Number(existing.value) || 0) + (Number(edge.value) || 0));
    existing.signedValue = roundedDisplayNumber((Number(existing.signedValue) || 0) + (Number(edge.signedValue) || 0));
    existing.displayValue = roundedDisplayNumber((Number(existing.displayValue) || 0) + (Number(edge.displayValue) || Number(edge.value) || 0));
    existing.sourceIds = appendUniqueEnergyStrings(existing.sourceIds || [], ...(edge.sourceIds || []));
    otherEdges.set(key, existing);
  }
  groupedEdges.push(...otherEdges.values());
  return { nodes: groupedNodes, edges: groupedEdges };
}

function energyExplanationNodeLimit(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number < 0) {
    return 80;
  }
  return number === 0 ? 0 : Math.max(10, Math.round(number));
}

function appendUniqueEnergyStrings(values = [], ...items) {
  const out = [...values];
  const seen = new Set(out);
  items.forEach((item) => {
    if (item && !seen.has(item)) {
      out.push(item);
      seen.add(item);
    }
  });
  return out;
}

function metricTokenForEnergyID(value = "") {
  return String(value || "unknown").toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "") || "unknown";
}

function roundedDisplayNumber(value) {
  return Math.round((Number(value) || 0) * 1000) / 1000;
}

function renderEnergyExplanationSVG(nodes = [], edges = []) {
  const columns = [[], [], [], [], []];
  nodes.forEach((node) => {
    columns[energyExplanationColumn(node)].push(node);
  });
  columns.forEach((column) => column.sort((a, b) => Math.abs(Number(b.value) || 0) - Math.abs(Number(a.value) || 0)));
  const width = 960;
  const rowHeight = 48;
  const maxRows = Math.max(1, ...columns.map((column) => column.length));
  const height = Math.max(260, 42 + maxRows * rowHeight);
  const xPositions = [24, 238, 452, 666, 800];
  const nodeWidth = 142;
  const selectedEdge = edges.find((edge) => state.simulationEnergySelection === edge.id);
  const connectedNodeIDs = new Set([selectedEdge?.fromId, selectedEdge?.toId].filter(Boolean));
  const positions = new Map();
  columns.forEach((column, columnIndex) => {
    column.forEach((node, rowIndex) => {
      positions.set(node.id, {
        x: xPositions[columnIndex],
        y: 34 + rowIndex * rowHeight,
        w: nodeWidth,
        h: 28,
      });
    });
  });
  const maxEdge = Math.max(1, ...edges.map((edge) => Math.abs(Number(edge.value) || 0)));
  const edgePaths = edges
    .map((edge) => {
      const from = positions.get(edge.fromId);
      const to = positions.get(edge.toId);
      if (!from || !to) {
        return "";
      }
      const x1 = from.x + from.w;
      const y1 = from.y + from.h / 2;
      const x2 = to.x;
      const y2 = to.y + to.h / 2;
      const mid = Math.max(28, (x2 - x1) / 2);
      const strokeWidth = Math.max(2, Math.min(18, 2 + 16 * Math.abs(Number(edge.value) || 0) / maxEdge));
      const selected = state.simulationEnergySelection === edge.id ? " selected" : "";
      return `<path class="energy-sankey-edge ${escapeHTML(edge.basis || "")}${selected}" d="M ${roundSVG(x1)} ${roundSVG(y1)} C ${roundSVG(x1 + mid)} ${roundSVG(y1)}, ${roundSVG(x2 - mid)} ${roundSVG(y2)}, ${roundSVG(x2)} ${roundSVG(y2)}" stroke-width="${roundSVG(strokeWidth)}" data-energy-explanation-edge="${escapeHTML(edge.id || "")}"><title>${escapeHTML(energyExplanationEdgeTitle(edge))}</title></path>`;
    })
    .join("");
  const nodeRects = nodes
    .map((node) => {
      const pos = positions.get(node.id);
      if (!pos) {
        return "";
      }
      const selected = state.simulationEnergySelection === node.id ? " selected" : "";
      const connected = connectedNodeIDs.has(node.id) ? " connected" : "";
      const classTokens = energyExplanationNodeClassTokens(node);
      return `
        <g class="energy-sankey-node ${escapeHTML(classTokens)}${selected}${connected}" data-energy-explanation-node="${escapeHTML(node.id || "")}" transform="translate(${roundSVG(pos.x)} ${roundSVG(pos.y)})">
          <rect width="${pos.w}" height="${pos.h}" rx="5"></rect>
          <text x="8" y="12">${escapeHTML(shortEnergyExplanationLabel(node.label || node.kind || ""))}</text>
          <text x="8" y="24">${escapeHTML(formatValueWithUnit(node.value, node.unit))}</text>
          <title>${escapeHTML(energyExplanationNodeTitle(node))}</title>
        </g>`;
    })
    .join("");
  const labels = [
    [24, "Energy Use"],
    [238, "End Use"],
    [452, "Delivered Load"],
    [666, "Heat Drivers"],
    [800, "Residual"],
  ]
    .map(([x, label]) => `<text x="${x}" y="18" class="simulation-axis">${escapeHTML(label)}</text>`)
    .join("");
  return `
    <div class="energy-sankey-wrap">
      <svg class="energy-sankey" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(t("simulation.energySankey", {}, "Energy Sankey"))}">
        ${labels}
        <g class="energy-sankey-edges">${edgePaths}</g>
        <g class="energy-sankey-nodes">${nodeRects}</g>
      </svg>
    </div>`;
}

function energyExplanationNodeClassTokens(node = {}) {
  return [node.level, node.carrier, node.serviceKind, node.endUse, node.heatCategory]
    .map(energyExplanationClassToken)
    .filter(Boolean)
    .join(" ");
}

function energyExplanationClassToken(value = "") {
  return String(value || "")
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, "_")
    .replace(/^_+|_+$/g, "");
}

function renderEnergyExplanationLegend() {
  return `
    <div class="energy-sankey-legend">
      <span><i class="node electricity"></i>${escapeHTML(t("simulation.energyElectricity", {}, "Electricity"))}</span>
      <span><i class="node fuel"></i>${escapeHTML(t("simulation.energyFuel", {}, "Fuel/gas"))}</span>
      <span><i class="node district_cooling"></i>${escapeHTML(t("simulation.energyDistrictCooling", {}, "District cooling"))}</span>
      <span><i class="node district_heating"></i>${escapeHTML(t("simulation.energyDistrictHeating", {}, "District heating"))}</span>
      <span><i class="node cooling"></i>${escapeHTML(t("simulation.cooling", {}, "Cooling"))}</span>
      <span><i class="node heating"></i>${escapeHTML(t("simulation.heating", {}, "Heating"))}</span>
      <span><i class="measured_meter"></i>${escapeHTML(t("simulation.basisMeasuredMeter", {}, "measured meter"))}</span>
      <span><i class="measured_variable"></i>${escapeHTML(t("simulation.basisMeasuredVariable", {}, "measured variable"))}</span>
      <span><i class="derived_balance"></i>${escapeHTML(t("simulation.basisDerived", {}, "derived balance"))}</span>
      <span><i class="allocated"></i>${escapeHTML(t("simulation.basisAllocated", {}, "allocated"))}</span>
      <span><i class="residual"></i>${escapeHTML(t("simulation.basisResidual", {}, "residual"))}</span>
    </div>`;
}

function renderEnergyExplanationInspector(selection, explanation = {}) {
  if (!selection) {
    return `<div class="energy-explanation-inspector empty">${escapeHTML(t("simulation.energySelectNode", {}, "Select a node or edge to inspect value, basis, formula, and source metadata."))}</div>`;
  }
  const sources = energyExplanationSourcesForIDs(explanation, selection.sourceIds || []);
  const sourceRows = sources
    .map((source) => {
      const object = sourceOutputForEnergySource(source);
      return `
        <tr>
          <td>${escapeHTML(source.id || "")}</td>
          <td>${escapeHTML(energyExplanationSourceTypeLabel(source))}</td>
          <td>${escapeHTML(source.keyValue || "")}</td>
          <td>${escapeHTML(source.name || "")}</td>
          <td>${escapeHTML(source.reportingFrequency || "")}</td>
          <td>${escapeHTML(source.aggregationMethod || "")}</td>
          <td>${escapeHTML(source.sourceUnit || source.units || "")}</td>
          <td>${escapeHTML(source.normalizedUnit || "")}</td>
          <td>${renderSourceOutputCell(object)}</td>
        </tr>`;
    })
    .join("");
  const type = selection.fromId ? "edge" : "node";
  const signedValue = Number(selection.signedValue);
  const inspectorFields = [
    { label: t("common.value", {}, "Value"), value: formatValueWithUnit(selection.value, selection.unit) },
    { label: t("common.period", {}, "Period"), value: selection.period || state.simulationEnergyPeriod || "annual" },
    { label: t("simulation.basis", {}, "Basis"), value: selection.basis || "measured" },
  ];
  if (selection.relation) {
    inspectorFields.push({ label: t("simulation.relation", {}, "Relation"), value: selection.relation });
  }
  if (selection.meterHierarchyLevel) {
    inspectorFields.push({ label: t("simulation.meterHierarchy", {}, "Meter hierarchy"), value: selection.meterHierarchyLevel });
  }
  if (selection.ruleId) {
    inspectorFields.push({ label: t("simulation.relationshipRule", {}, "Rule"), value: energyExplanationRelationshipRuleLabel(explanation, selection.ruleId) });
  }
  inspectorFields.push({ label: t("common.source", {}, "Source"), value: (selection.sourceIds || []).join(", ") || "-" });
  if (Number.isFinite(signedValue) && signedValue !== 0 && signedValue !== Number(selection.value)) {
    inspectorFields.splice(1, 0, { label: t("simulation.signedValue", {}, "Signed"), value: formatValueWithUnit(signedValue, selection.unit) });
  }
  if (selection.heatCategory) {
    inspectorFields.push({ label: t("simulation.heatCategory", {}, "Heat category"), value: selection.heatCategory });
  }
  if (selection.zoneName) {
    inspectorFields.push({ label: t("common.zone", {}, "Zone"), value: selection.zoneName });
  }
  if (selection.serviceKind) {
    inspectorFields.push({ label: t("simulation.service", {}, "Service"), value: selection.serviceKind });
  }
  if (selection.pathType) {
    inspectorFields.push({ label: t("simulation.pathType", {}, "Path type"), value: simulationPathTypeLabel(selection.pathType) });
  }
  const relatedPaths = renderSimulationEnergyRelatedServicePaths(selection);
  return `
    <section class="energy-explanation-inspector">
      <div class="simulation-energy-block-head">
        <h4>${escapeHTML(selection.label || selection.kind || selection.id || "")}</h4>
        <span>${escapeHTML(type)} / ${escapeHTML(selection.basis || selection.level || "")}</span>
      </div>
      <div class="energy-explanation-inspector-grid">
        ${inspectorFields.map((field) => `<div><span>${escapeHTML(field.label)}</span><strong>${escapeHTML(field.value)}</strong></div>`).join("")}
      </div>
      ${selection.formula ? `<p class="energy-explanation-formula">${escapeHTML(selection.formula)}</p>` : ""}
      ${relatedPaths}
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>ID</th><th>${escapeHTML(t("common.type", {}, "Type"))}</th><th>Key</th><th>Name</th><th>Frequency</th><th>Aggregation</th><th>Source Unit</th><th>Normalized Unit</th><th>${escapeHTML(t("simulation.sourceOutput", {}, "Source output"))}</th></tr></thead>
          <tbody>${sourceRows || `<tr><td colspan="9">${escapeHTML(t("common.notAvailable", {}, "N/A"))}</td></tr>`}</tbody>
        </table>
      </div>
    </section>`;
}

function renderEnergyExplanationSources(explanation = {}) {
  const rows = (explanation.sources || [])
    .map((source) => {
      const object = sourceOutputForEnergySource(source);
      return `
        <tr>
          <td>${escapeHTML(source.id || "")}</td>
          <td>${escapeHTML(source.sourceType || "")}</td>
          <td>${escapeHTML(source.isMeter ? "meter" : "variable")}</td>
          <td>${escapeHTML(source.keyValue || "")}</td>
          <td>${escapeHTML(source.name || "")}</td>
          <td>${escapeHTML(source.reportingFrequency || "")}</td>
          <td>${escapeHTML(source.aggregationMethod || "")}</td>
          <td>${escapeHTML(source.sourceUnit || source.units || "")}</td>
          <td>${escapeHTML(source.normalizedUnit || "")}</td>
          <td>${renderSourceOutputCell(object)}</td>
        </tr>`;
    })
    .join("");
  return `
    <section class="simulation-energy-block">
      <div class="simulation-energy-block-head">
        <h4>${escapeHTML(t("simulation.energySources", {}, "Sources"))}</h4>
        <span>${escapeHTML((explanation.sources || []).length)} SQL/output source(s)</span>
      </div>
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>ID</th><th>Source</th><th>Basis</th><th>Key</th><th>Name</th><th>Frequency</th><th>Aggregation</th><th>Source Unit</th><th>Normalized Unit</th><th>${escapeHTML(t("simulation.sourceOutput", {}, "Source output"))}</th></tr></thead>
          <tbody>${rows || `<tr><td colspan="10">${escapeHTML(t("simulation.noEnergyExplanation", {}, "No energy explanation graph is available."))}</td></tr>`}</tbody>
        </table>
      </div>
    </section>`;
}

function renderEnergyExplanationReconciliation(explanation = {}) {
  const completeness = renderEnergyExplanationCompleteness(explanation);
  const periodID = state.simulationEnergyPeriod || "annual";
  const graph = energyExplanationGraphForPeriod(explanation, periodID);
  const usePeriodRows = periodID !== "annual" || (graph.reconciliation || []).length;
  const reconciliation = usePeriodRows ? graph.reconciliation || [] : explanation.reconciliation || [];
  const warningsForPeriod = periodID !== "annual" || (graph.warnings || []).length ? graph.warnings || [] : explanation.warnings || [];
  const rows = reconciliation
    .map(
      (item) => `
        <tr>
          <td>${escapeHTML(item.label || item.id || "")}</td>
          <td>${escapeHTML(item.period || "")}</td>
          <td>${escapeHTML(item.zoneName || "")}</td>
          <td>${escapeHTML(item.serviceKind || "")}</td>
          <td>${escapeHTML(formatValueWithUnit(item.expectedValue, item.unit))}</td>
          <td>${escapeHTML(formatValueWithUnit(item.explainedValue, item.unit))}</td>
          <td>${escapeHTML(formatValueWithUnit(item.residualValue, item.unit))}</td>
          <td>${escapeHTML(item.basis || "")}</td>
          <td>${escapeHTML(item.formula || "")}</td>
          <td>${renderEnergyReconciliationSources(explanation, item.sourceIds || [])}</td>
        </tr>`,
    )
    .join("");
  const warnings = warningsForPeriod
    .map((warning) => `<article class="simulation-hvac-alert ${escapeHTML(warning.severity || "info")}"><strong>${escapeHTML(warning.code || "")}</strong><span>${escapeHTML(warning.message || "")}</span></article>`)
    .join("");
  const periodLabel = graph.label || periodID;
  const zoneResidualRanking = renderEnergyZoneResidualRanking(reconciliation);
  return `
    ${completeness}
    ${warnings ? `<div class="simulation-hvac-alert-list">${warnings}</div>` : ""}
    <section class="simulation-energy-block">
      <div class="simulation-energy-block-head">
        <h4>${escapeHTML(t("simulation.energyReconciliation", {}, "Reconciliation"))}</h4>
        <span>${escapeHTML(periodLabel)} / ${escapeHTML(t("simulation.accountingGapNote", {}, "Residual is an accounting gap, not automatically a model error."))}</span>
      </div>
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>${escapeHTML(t("common.metric", {}, "Metric"))}</th><th>${escapeHTML(t("common.period", {}, "Period"))}</th><th>${escapeHTML(t("common.zone", {}, "Zone"))}</th><th>${escapeHTML(t("simulation.service", {}, "Service"))}</th><th>Expected</th><th>Mapped</th><th>Residual</th><th>${escapeHTML(t("simulation.basis", {}, "Basis"))}</th><th>Formula</th><th>${escapeHTML(t("common.source", {}, "Source"))}</th></tr></thead>
          <tbody>${rows || `<tr><td colspan="10">${escapeHTML(t("common.notAvailable", {}, "N/A"))}</td></tr>`}</tbody>
        </table>
      </div>
    </section>
    ${zoneResidualRanking}`;
}

function renderEnergyZoneResidualRanking(reconciliation = []) {
  const rows = (reconciliation || [])
    .filter((item) => item.level === "heat" && item.zoneName && Number.isFinite(Number(item.residualValue)) && Math.abs(Number(item.residualValue)) > 0)
    .sort((a, b) => Math.abs(Number(b.residualValue)) - Math.abs(Number(a.residualValue)))
    .slice(0, 8)
    .map(
      (item) => `
        <tr>
          <td>${escapeHTML(item.zoneName || "")}</td>
          <td>${escapeHTML(item.serviceKind || "")}</td>
          <td>${escapeHTML(formatValueWithUnit(item.residualValue, item.unit))}</td>
          <td>${escapeHTML(formatValueWithUnit(item.expectedValue, item.unit))}</td>
          <td>${escapeHTML(formatValueWithUnit(item.explainedValue, item.unit))}</td>
        </tr>`,
    )
    .join("");
  if (!rows) {
    return "";
  }
  return `
    <section class="simulation-energy-block">
      <div class="simulation-energy-block-head">
        <h4>${escapeHTML(t("simulation.zoneHeatResidualRanking", {}, "Largest zone heat residuals"))}</h4>
        <span>${escapeHTML(t("simulation.accountingGapNote", {}, "Residual is an accounting gap, not automatically a model error."))}</span>
      </div>
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>${escapeHTML(t("common.zone", {}, "Zone"))}</th><th>${escapeHTML(t("simulation.service", {}, "Service"))}</th><th>Residual</th><th>Expected</th><th>Mapped</th></tr></thead>
          <tbody>${rows}</tbody>
        </table>
      </div>
    </section>`;
}

function renderEnergyReconciliationSources(explanation = {}, sourceIDs = []) {
  const sourceByID = new Map((explanation.sources || []).map((source) => [source.id, source]));
  const uniqueSourceIDs = appendUniqueEnergyStrings([], ...(sourceIDs || []));
  const rows = uniqueSourceIDs
    .slice(0, 8)
    .map((sourceID) => {
      const source = sourceByID.get(sourceID) || { id: sourceID };
      const object = source.id ? sourceOutputForEnergySource(source) : null;
      const label = [energyExplanationSourceTypeLabel(source), source.keyValue, source.name, source.reportingFrequency].filter(Boolean).join(" / ");
      return `
        <span class="energy-reconciliation-source" title="${escapeHTML(label || sourceID)}">
          <code>${escapeHTML(sourceID)}</code>
          ${renderSourceOutputCell(object, { compact: true })}
        </span>`;
    })
    .join("");
  const hiddenCount = Math.max(0, uniqueSourceIDs.length - 8);
  if (!rows) {
    return `<span class="simulation-source-output missing">${escapeHTML(t("common.notAvailable", {}, "N/A"))}</span>`;
  }
  return `<div class="energy-reconciliation-sources">${rows}${hiddenCount ? `<span class="energy-reconciliation-source more">+${escapeHTML(hiddenCount)}</span>` : ""}</div>`;
}

function energyExplanationGraphForPeriod(explanation = {}, periodID = "annual") {
  const id = periodID || "annual";
  const period = (explanation.periods || []).find((item) => item.id === id);
  if (period) {
    return {
      id: period.id || id,
      label: period.label || period.id || id,
      kind: period.kind || "",
      nodes: period.nodes || [],
      edges: period.edges || [],
      reconciliation: period.reconciliation || [],
      warnings: period.warnings || [],
    };
  }
  return {
    id,
    label: id,
    kind: id === "annual" ? "annual" : "",
    nodes: explanation.nodes || [],
    edges: explanation.edges || [],
    reconciliation: explanation.reconciliation || [],
    warnings: explanation.warnings || [],
  };
}

function focusedEnergyExplanationGraph(graph = {}) {
  const mode = state.simulationEnergyFocusMode || "all";
  if (mode === "all") {
    return graph;
  }
  const nodes = graph.nodes || [];
  const edges = graph.edges || [];
  let includeNode = () => true;
  if (mode === "zone") {
    const zoneKey = simulationZoneKey(state.simulationEnergyZoneFocus || "");
    if (!zoneKey) {
      return graph;
    }
    includeNode = (node) => !node.zoneName || simulationZoneKey(node.zoneName) === zoneKey;
  } else if (mode === "service_path") {
    const path = simulationHVACServicePaths().find((item) => item.id === state.simulationEnergyServicePathFocus);
    if (!path) {
      return graph;
    }
    const service = simulationCanonicalServiceKind(path.serviceKind);
    const zoneKey = simulationZoneKey(path.zoneName || path.servedSubject?.zoneName || "");
    includeNode = (node) => {
      const nodeService = simulationCanonicalServiceKind(node.serviceKind || energyServiceFromNode(node));
      const serviceMatch = !nodeService || !service || nodeService === service;
      const zoneMatch = !zoneKey || !node.zoneName || simulationZoneKey(node.zoneName) === zoneKey;
      if (node.level === "load" || node.level === "heat") {
        return serviceMatch && zoneMatch;
      }
      if (node.level === "energy" && nodeService) {
        return serviceMatch;
      }
      return zoneMatch;
    };
  }
  const focusedNodes = nodes.filter(includeNode);
  const nodeIDs = new Set(focusedNodes.map((node) => node.id));
  const focusedEdges = edges.filter((edge) => nodeIDs.has(edge.fromId) && nodeIDs.has(edge.toId));
  return { nodes: focusedNodes, edges: focusedEdges };
}

function energyFocusZones(nodes = []) {
  return [...new Set(nodes.map((node) => String(node.zoneName || "").trim()).filter(Boolean))].sort((a, b) => a.localeCompare(b));
}

function energyExplanationSelection(explanation = {}, graph = {}) {
  const id = state.simulationEnergySelection || "";
  if (!id) {
    return null;
  }
  const node = (graph.nodes || []).find((item) => item.id === id);
  if (node) {
    return node;
  }
  const edge = (graph.edges || []).find((item) => item.id === id);
  if (edge) {
    return {
      ...edge,
      label: `${energyExplanationNodeLabel(graph.nodes, edge.fromId)} -> ${energyExplanationNodeLabel(graph.nodes, edge.toId)}`,
    };
  }
  const fallback = (explanation.nodes || []).find((item) => item.id === id) || (explanation.edges || []).find((item) => item.id === id);
  return fallback || null;
}

function energyExplanationNodeLabel(nodes = [], id = "") {
  return nodes.find((node) => node.id === id)?.label || id;
}

function energyExplanationColumn(node = {}) {
  if (node.level === "residual") {
    return 4;
  }
  if (node.level === "heat") {
    return 3;
  }
  if (node.level === "load") {
    return 2;
  }
  if (node.level === "energy" && String(node.id || "").includes(".end_use.")) {
    return 1;
  }
  return 0;
}

function energyExplanationLevelLabel(level = "") {
  switch (level) {
    case "energy":
      return t("simulation.energyUse", {}, "Energy Use");
    case "load":
      return t("simulation.deliveredLoad", {}, "Delivered Load");
    case "heat":
      return t("simulation.heatDrivers", {}, "Heat Drivers");
    case "residual":
      return t("simulation.residual", {}, "Residual");
    default:
      return level || t("common.notAvailable", {}, "N/A");
  }
}

function energyExplanationSourcesForIDs(explanation = {}, sourceIDs = []) {
  const wanted = new Set(sourceIDs || []);
  return (explanation.sources || []).filter((source) => wanted.has(source.id));
}

function energyExplanationSourceTypeLabel(source = {}) {
  const basis = source.isMeter ? "meter" : "variable";
  return [source.sourceType || "", basis].filter(Boolean).join(" / ") || basis;
}

function energyExplanationRelationshipRuleLabel(explanation = {}, ruleID = "") {
  const rule = (explanation.relationshipRules || []).find((item) => item.id === ruleID);
  if (!rule) {
    return ruleID;
  }
  const flow = [rule.fromLevel, rule.toLevel].filter(Boolean).join(" -> ");
  return [rule.id, flow, rule.basis].filter(Boolean).join(" / ");
}

function renderSimulationEnergyRelatedServicePaths(selection = {}) {
  const paths = simulationRelatedServicePathsForEnergySelection(selection).slice(0, 8);
  if (!paths.length) {
    return "";
  }
  return `
    <div class="energy-related-service-paths">
      <strong>${escapeHTML(t("hvac.relatedServicePaths", {}, "Related service paths"))}</strong>
      <div>
        ${paths
          .map((path) =>
            renderSimulationEnergyServicePathButton(
              path,
              `${simulationServedSubjectLabel(path.servedSubject || path)} / ${simulationServiceKindLabel(path.serviceKind)} / ${simulationDeliveryLabel(path)}`,
            ),
          )
          .join("")}
      </div>
    </div>`;
}

function renderSimulationEnergyServicePathButton(path = {}, label = "") {
  return `
    <button
      type="button"
      class="energy-service-path-chip"
      data-simulation-hvac-path-id="${escapeHTML(path.id || "")}"
      title="${escapeHTML(simulationServicePathConnectedSystems(path).join(", ") || simulationPathTypeLabel(path.pathType))}"
    >${escapeHTML(label || simulationServedSubjectLabel(path.servedSubject || path))}</button>`;
}

function simulationRelatedServicePathsForEnergySelection(selection = {}) {
  const directPaths = simulationHVACServicePathsByIDs(selection.relatedPathIds || []);
  if (directPaths.length) {
    return directPaths;
  }
  const service = simulationCanonicalServiceKind(selection.serviceKind || energyServiceFromNode(selection));
  const zoneKey = simulationZoneKey(selection.zoneName || "");
  return simulationHVACServicePaths().filter((path) => {
    if (service && simulationCanonicalServiceKind(path.serviceKind) !== service) {
      return false;
    }
    if (zoneKey && simulationZoneKey(path.zoneName || path.servedSubject?.zoneName || "") !== zoneKey) {
      return false;
    }
    return Boolean(service || zoneKey);
  });
}

function simulationRelatedServicePathsForEnergyNodes(nodes = []) {
  const directPathIDs = new Set();
  const wantedServices = new Set();
  const wantedZoneServices = new Set();
  nodes.forEach((node) => {
    (node.relatedPathIds || []).forEach((id) => {
      if (id) {
        directPathIDs.add(id);
      }
    });
    const service = simulationCanonicalServiceKind(node.serviceKind || energyServiceFromNode(node));
    if (!service) {
      return;
    }
    wantedServices.add(service);
    if (node.zoneName) {
      wantedZoneServices.add(`${simulationZoneKey(node.zoneName)}|${service}`);
    }
  });
  if (directPathIDs.size) {
    return simulationSortedEnergyServicePaths(simulationHVACServicePathsByIDs([...directPathIDs]));
  }
  const paths = simulationHVACServicePaths().filter((path) => {
    const service = simulationCanonicalServiceKind(path.serviceKind);
    if (!wantedServices.has(service)) {
      return false;
    }
    if (!wantedZoneServices.size) {
      return true;
    }
    const zoneKey = simulationZoneKey(path.zoneName || path.servedSubject?.zoneName || "");
    return wantedZoneServices.has(`${zoneKey}|${service}`) || wantedServices.has(service);
  });
  return simulationSortedEnergyServicePaths(paths);
}

function simulationHVACServicePathsByIDs(pathIDs = []) {
  const wanted = new Set(pathIDs || []);
  if (!wanted.size) {
    return [];
  }
  return simulationHVACServicePaths().filter((path) => wanted.has(path.id));
}

function simulationSortedEnergyServicePaths(paths = []) {
  return paths.sort((a, b) => {
    const zoneCompare = simulationServedSubjectLabel(a.servedSubject || a).localeCompare(simulationServedSubjectLabel(b.servedSubject || b));
    if (zoneCompare) return zoneCompare;
    return simulationServiceKindLabel(a.serviceKind).localeCompare(simulationServiceKindLabel(b.serviceKind));
  });
}

function simulationHVACServiceModel() {
  return state.report?.hvac?.serviceModel || { zoneServices: [], systems: [], couplings: [], networks: [] };
}

function simulationHVACServicePaths() {
  return (simulationHVACServiceModel().zoneServices || []).flatMap((summary) => summary.paths || []);
}

function simulationCanonicalServiceKind(serviceKind = "") {
  const value = String(serviceKind || "").toLowerCase();
  if (value.includes("cooling") || value === "dehumidification") {
    return "cooling";
  }
  if (value.includes("heating") || value === "humidification") {
    return "heating";
  }
  if (value.includes("ventilation")) {
    return "ventilation";
  }
  return value || "";
}

function energyServiceFromNode(node = {}) {
  const text = `${node.id || ""} ${node.kind || ""} ${node.endUse || ""}`.toLowerCase();
  if (text.includes("cooling")) {
    return "cooling";
  }
  if (text.includes("heating")) {
    return "heating";
  }
  if (text.includes("ventilation")) {
    return "ventilation";
  }
  return "";
}

function simulationZoneKey(value = "") {
  return String(value || "").trim().toLowerCase();
}

function simulationNavigationPathEntityID(pathID = "") {
  return String(pathID || "").startsWith("path:") ? pathID : `path:${pathID}`;
}

function simulationServicePathGraphKey(path = {}) {
  return `service-path:${path.id || simulationServedSubjectKey(path.servedSubject || path)}`;
}

function simulationServedSubjectKey(subject = {}) {
  if (String(subject.kind || "").toLowerCase() === "space" && subject.spaceName) {
    return `space:${simulationZoneKey(subject.spaceName)}`;
  }
  return `zone:${simulationZoneKey(subject.zoneName || subject.name)}`;
}

function simulationServedSubjectLabel(subject = {}) {
  if (String(subject.kind || "").toLowerCase() === "space" && subject.spaceName) {
    return subject.zoneName ? `${subject.spaceName} / ${subject.zoneName}` : subject.spaceName;
  }
  return subject.zoneName || subject.name || t("common.blank", {}, "Blank");
}

function simulationServiceKindLabel(kind = "") {
  const labels = {
    cooling: "Cooling",
    heating: "Heating",
    ventilation: "Ventilation",
    exhaust: "Exhaust",
    humidification: "Humidification",
    dehumidification: "Dehumidification",
    radiant_cooling: "Radiant cooling",
    radiant_heating: "Radiant heating",
  };
  return labels[kind] || titleCaseEnergyToken(kind || "Service");
}

function simulationPathTypeLabel(pathType = "") {
  const labels = {
    central_air_with_plant: "Central air + plant",
    central_air: "Central air",
    direct_zone_hydronic: "Direct hydronic",
    direct_zone_air: "Direct zone air",
    direct_zone_refrigerant: "Refrigerant",
    radiant: "Radiant",
    baseboard: "Baseboard",
    ideal_loads: "Ideal loads",
    ventilation_only: "Ventilation only",
    exhaust_only: "Exhaust only",
    local: "Local",
  };
  return labels[pathType] || titleCaseEnergyToken(pathType || "Path");
}

function simulationDeliveryLabel(path = {}) {
  return path.deliveryEquipment?.displayFamily || path.delivery?.displayFamily || path.delivery?.displayName || path.delivery?.objectName || "Delivery";
}

function simulationServicePathConnectedSystems(path = {}) {
  return [
    path.plantLoop?.name ? `PlantLoop ${path.plantLoop.name}` : "",
    path.airLoop?.name ? `AirLoopHVAC ${path.airLoop.name}` : "",
    path.condenserLoop?.name ? `CondenserLoop ${path.condenserLoop.name}` : "",
    path.refrigerantSystem?.name ? `${path.refrigerantSystem.type || "Refrigerant"} ${path.refrigerantSystem.name}` : "",
    path.sourceSystem?.name ? `${path.sourceSystem.type || "Source"} ${path.sourceSystem.name}` : "",
  ].filter(Boolean);
}

function simulationEnergyServicePathFocusLabel(path = {}) {
  return `${simulationServedSubjectLabel(path.servedSubject || path)} / ${simulationServiceKindLabel(path.serviceKind)} / ${simulationDeliveryLabel(path)}`;
}

function titleCaseEnergyToken(value = "") {
  return String(value || "")
    .replace(/_/g, " ")
    .replace(/\w\S*/g, (part) => part.charAt(0).toUpperCase() + part.slice(1).toLowerCase());
}

function energyExplanationNodeTitle(node = {}) {
  return [
    node.label || node.kind || node.id,
    formatValueWithUnit(node.value, node.unit),
    energyExplanationLevelLabel(node.level || ""),
    node.carrier ? `carrier: ${node.carrier}` : "",
    node.endUse ? `end use: ${node.endUse}` : "",
    node.serviceKind ? `service: ${node.serviceKind}` : "",
    node.heatCategory ? `heat category: ${node.heatCategory}` : "",
    node.sign ? `sign: ${node.sign}` : "",
    Number(node.signedValue) && Number(node.signedValue) !== Number(node.value) ? `signed: ${formatValueWithUnit(node.signedValue, node.unit)}` : "",
  ]
    .filter(Boolean)
    .join(" / ");
}

function energyExplanationEdgeTitle(edge = {}) {
  return [
    edge.relation || "",
    edge.basis || "",
    edge.ruleId ? `rule: ${edge.ruleId}` : "",
    formatValueWithUnit(edge.value, edge.unit),
    Number(edge.signedValue) && Number(edge.signedValue) !== Number(edge.value) ? `signed: ${formatValueWithUnit(edge.signedValue, edge.unit)}` : "",
    edge.formula || "",
  ]
    .filter(Boolean)
    .join(" / ");
}

function shortEnergyExplanationLabel(label = "") {
  label = String(label || "").trim();
  return label.length > 22 ? `${label.slice(0, 21)}...` : label;
}

function renderSimulationHVACLoopEmpty(message) {
  if (elements.simulationHVACLoopStats) {
    elements.simulationHVACLoopStats.textContent = t("simulation.noHVACLoopResult", {}, "No HVAC loop result");
  }
  if (elements.simulationHVACLoopResults) {
    elements.simulationHVACLoopResults.innerHTML = `<div class="empty">${escapeHTML(message)}</div>`;
  }
}

function renderSimulationHVACLoops(result) {
  const loops = result?.purposeResults?.hvacLoops || [];
  const seriesCount = loops.reduce((sum, loop) => sum + (loop.series || []).length + hvacComponentSeriesCount(loop.components || []), 0);
  if (!seriesCount) {
    renderSimulationHVACLoopEmpty(t("simulation.noHVACLoopResult", {}, "Run HVAC Loop Check to inspect node state series."));
    return;
  }
  if (elements.simulationHVACLoopStats) {
    elements.simulationHVACLoopStats.textContent = t(
      "simulation.hvacLoopStats",
      { loops: loops.length, series: seriesCount },
      `${loops.length} loop group, ${seriesCount} node series`,
    );
  }
  elements.simulationHVACLoopResults.innerHTML = loops.map(renderSimulationHVACLoopResult).join("");
}

function renderSimulationHVACLoopResult(loop) {
  const visibleNodeSeries = hvacVisibleSeries(loop.series || []);
  const visibleComponents = hvacVisibleComponents(loop.components || []);
  const staticDiagramLoop = simulationHVACStaticDiagramLoop(loop);
  const rows = visibleNodeSeries
    .slice(0, 80)
    .map(
      (series) => {
        const nodeName = seriesNodeKey(series.column);
        const metricName = seriesVariableName(series.column);
        return `
        <tr>
          <td>${escapeHTML(nodeName)}</td>
          <td>${escapeHTML(metricName)}</td>
          <td>${escapeHTML(series.file || "")}</td>
          <td>${renderSourceInspectorCell(sourceOutputForSeriesColumn(series.column), { series, keyValue: nodeName, variableName: metricName })}</td>
          <td>${escapeHTML(formatSeriesStat(series, "min"))}</td>
          <td>${escapeHTML(formatSeriesStat(series, "max"))}</td>
          <td>${escapeHTML(formatSeriesStat(series, "average"))}</td>
          <td>${escapeHTML(simulationSeriesPointCount(series))}</td>
        </tr>`;
      },
    )
    .join("");
  return `
    <section class="simulation-hvac-loop-result">
      <div class="simulation-hvac-loop-head">
        <div>
          <h4>${escapeHTML(loop.name || t("simulation.hvacLoops", {}, "HVAC Loops"))}</h4>
          ${loop.status ? `<b class="simulation-hvac-status ${escapeHTML(loop.status)}">${escapeHTML(simulationHVACStatusLabel(loop.status))}</b>` : ""}
        </div>
        <span>${escapeHTML([loop.loopType || t("simulation.nodeStateSeries", {}, "Node state series"), loop.statusMessage || ""].filter(Boolean).join(" - "))}</span>
      </div>
      ${renderSimulationHVACLoopControls(loop, staticDiagramLoop)}
      ${staticDiagramLoop && simulationHVACPanelVisible("topology") ? renderSimulationHVACStaticDiagram(staticDiagramLoop) : ""}
      ${simulationHVACPanelVisible("snapshot") ? renderSimulationHVACLoopSnapshot(loop) : ""}
      ${simulationHVACPanelVisible("chart") ? renderSimulationHVACSeriesOverview(loop, visibleNodeSeries, visibleComponents) : ""}
      ${renderSimulationHVACLoopDerivedMetrics(loop.derivedMetrics || [])}
      ${renderSimulationHVACLoopAlerts(loop.alerts || [])}
      ${renderPurposeCompletenessRow(loop.completeness || [])}
      ${renderSimulationHVACNodeSummaries(loop.nodeSummaries || [])}
      ${renderSimulationHVACComponentOperations(visibleComponents)}
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>${escapeHTML(t("common.key", {}, "Key"))}</th><th>${escapeHTML(t("common.metric", {}, "Metric"))}</th><th>${escapeHTML(t("common.source", {}, "Source"))}</th><th>${escapeHTML(t("simulation.sourceOutput", {}, "Source output"))}</th><th>Min</th><th>Max</th><th>Avg</th><th>${escapeHTML(t("common.points", {}, "Points"))}</th></tr></thead>
          <tbody>${rows || `<tr><td colspan="8">${escapeHTML(t("simulation.noSeries", {}, "No SQL/CSV series"))}</td></tr>`}</tbody>
        </table>
      </div>
    </section>`;
}

function renderSimulationHVACLoopControls(loop, staticDiagramLoop = null) {
  const groupOptions = hvacLoopGroupOptions(loop);
  const panelOptions = [
    staticDiagramLoop ? { id: "topology", label: t("simulation.hvacTopology", {}, "Topology") } : null,
    { id: "snapshot", label: t("simulation.hvacSnapshot", {}, "Frame snapshot") },
    { id: "chart", label: t("simulation.hvacMultiSeries", {}, "Multi-series") },
  ].filter(Boolean);
  return `
    <div class="simulation-hvac-loop-controls">
      <div class="simulation-hvac-toggle-row" role="group" aria-label="${escapeHTML(t("simulation.hvacPanels", {}, "HVAC panels"))}">
        ${panelOptions
          .map(
            (option) => `
              <label>
                <input type="checkbox" data-simulation-hvac-panel-toggle="${escapeHTML(option.id)}" ${simulationHVACPanelVisible(option.id) ? "checked" : ""} />
                <span>${escapeHTML(option.label)}</span>
              </label>`,
          )
          .join("")}
      </div>
      <div class="simulation-hvac-toggle-row" role="group" aria-label="${escapeHTML(t("simulation.variableGroups", {}, "Variable groups"))}">
        ${groupOptions
          .map(
            (option) => `
              <label>
                <input type="checkbox" data-simulation-hvac-group-toggle="${escapeHTML(option.value)}" ${simulationHVACGroupVisible(option.value) ? "checked" : ""} />
                <span>${escapeHTML(option.label)}</span>
              </label>`,
          )
          .join("")}
      </div>
    </div>`;
}

function hvacLoopGroupOptions(loop = {}) {
  const series = [
    ...(loop.series || []),
    ...(loop.components || []).flatMap((component) => component.series || []),
  ];
  return simulationSeriesGroupOptions(series).filter((option) => option.value !== "all");
}

function simulationHVACPanelVisible(panelID) {
  if (!state.simulationHVACPanels) {
    state.simulationHVACPanels = {};
  }
  if (state.simulationHVACPanels[panelID] === undefined) {
    state.simulationHVACPanels[panelID] = true;
  }
  return Boolean(state.simulationHVACPanels[panelID]);
}

function simulationHVACGroupVisible(groupID) {
  if (!state.simulationHVACVisibleGroups) {
    state.simulationHVACVisibleGroups = {};
  }
  if (state.simulationHVACVisibleGroups[groupID] === undefined) {
    state.simulationHVACVisibleGroups[groupID] = true;
  }
  return Boolean(state.simulationHVACVisibleGroups[groupID]);
}

function hvacVisibleSeries(series = []) {
  return (series || []).filter((item) => simulationHVACGroupVisible(simulationSeriesGroupID(item)));
}

function hvacVisibleComponents(components = []) {
  return (components || [])
    .map((component) => {
      const metrics = (component.metrics || []).filter((metric) => simulationHVACGroupVisible(simulationVariableGroupID(metric.name)));
      const series = (component.series || []).filter((item) => simulationHVACGroupVisible(simulationSeriesGroupID(item)));
      return { ...component, metrics, series };
    })
    .filter((component) => (component.metrics || []).length || (component.series || []).length);
}

function simulationHVACStaticDiagramLoop(resultLoop = {}) {
  const loop = activeSimulationHVACLoop();
  if (!loop) {
    return null;
  }
  const resultName = normalizeOutputMatchToken(resultLoop.name || "");
  const loopName = normalizeOutputMatchToken(loop.name || "");
  if (!resultName || !loopName || resultName === loopName || resultName.includes(loopName) || loopName.includes(resultName)) {
    return loop;
  }
  return null;
}

function renderSimulationHVACStaticDiagram(loop) {
  return `
    <section class="simulation-hvac-static-diagram">
      <div class="simulation-hvac-series-head">
        <h5>${escapeHTML(t("simulation.hvacTopology", {}, "Topology"))}</h5>
        <span>${escapeHTML(t("simulation.hvacTopologySource", {}, "From the current HVAC graph selection"))}</span>
      </div>
      ${renderHVACLoopDiagram(loop)}
    </section>`;
}

function renderSimulationHVACLoopSnapshot(loop) {
  const frameCount = hvacLoopFrameCount(loop);
  if (frameCount <= 0) {
    return "";
  }
  const frameIndex = Math.round(clampNumber(state.simulationHVACFrameIndex, 0, frameCount - 1));
  state.simulationHVACFrameIndex = frameIndex;
  const nodes = hvacNodeFrameSnapshots(loop, frameIndex);
  if (!nodes.length) {
    return "";
  }
  const maxFlow = Math.max(...nodes.map((node) => Math.abs(Number(node.flow?.value) || 0)), 1);
  const components = hvacComponentFrameSnapshots(loop, frameIndex);
  return `
    <section class="simulation-hvac-snapshot">
      <div class="simulation-hvac-snapshot-head">
        <h5>${escapeHTML(t("simulation.hvacFrameSnapshot", {}, "Frame snapshot"))}</h5>
        <label>
          <span>${escapeHTML(hvacLoopFrameLabel(loop, frameIndex))}</span>
          <input type="range" min="0" max="${frameCount - 1}" value="${frameIndex}" data-simulation-hvac-frame />
        </label>
      </div>
      ${renderSimulationHVACSchematic(nodes)}
      <div class="simulation-hvac-node-strip">
        ${nodes.map((node) => renderSimulationHVACNodeSnapshot(node, maxFlow)).join("")}
      </div>
      ${components.length ? `<div class="simulation-hvac-component-strip">${components.map(renderSimulationHVACComponentSnapshot).join("")}</div>` : ""}
    </section>`;
}

function hvacLoopFrameCount(loop) {
  return Math.max(0, ...(loop.series || []).map((series) => simulationSeriesPointCount(series)));
}

function hvacLoopFrameLabel(loop, frameIndex) {
  const series = (loop.series || []).find((item) => simulationSeriesPointAt(item, frameIndex));
  const point = simulationSeriesPointAt(series, frameIndex);
  if (!point) {
    return `Frame ${frameIndex + 1}`;
  }
  return String(point.label || point.x || `Frame ${frameIndex + 1}`);
}

function hvacNodeFrameSnapshots(loop, frameIndex) {
  const nodes = new Map();
  for (const series of loop.series || []) {
    const nodeName = seriesNodeKey(series.column) || t("common.unknown", {}, "Unknown");
    const metricName = seriesVariableName(series.column);
    const point = simulationSeriesPointAt(series, frameIndex);
    if (!point) {
      continue;
    }
    const key = normalizeOutputMatchToken(nodeName);
    const node = nodes.get(key) || { nodeName, source: series.file || "" };
    const metric = {
      name: metricName,
      value: Number(point.value),
      unit: simulationSeriesDisplayUnit(series),
      source: series.file || "",
    };
    switch (normalizeOutputMatchToken(metricName)) {
      case "system node temperature":
        node.temperature = metric;
        break;
      case "system node setpoint temperature":
        node.setpoint = metric;
        break;
      case "system node mass flow rate":
        node.flow = metric;
        break;
      case "system node humidity ratio":
        node.humidity = metric;
        break;
      case "system node enthalpy":
        node.enthalpy = metric;
        break;
      default:
        break;
    }
    nodes.set(key, node);
  }
  return [...nodes.values()].sort((a, b) => String(a.nodeName).localeCompare(String(b.nodeName))).slice(0, 48);
}

function renderSimulationHVACNodeSnapshot(node, maxFlow) {
  const flowValue = Number(node.flow?.value) || 0;
  const activeFlow = Math.abs(flowValue) > 0.001;
  const flowPercent = clampNumber(Math.abs(flowValue) / Math.max(maxFlow, 0.001), 0, 1) * 100;
  return `
    <article class="simulation-hvac-node-card ${activeFlow ? "active" : ""}">
      <header>
        <strong title="${escapeHTML(node.nodeName || "")}">${escapeHTML(node.nodeName || "")}</strong>
        <span>${escapeHTML(activeFlow ? t("simulation.activeFlow", {}, "Active flow") : t("simulation.lowFlow", {}, "Low flow"))}</span>
      </header>
      <div class="simulation-hvac-node-values">
        ${renderHVACSnapshotMetric(t("common.temperature", {}, "Temp"), node.temperature)}
        ${renderHVACSnapshotMetric(t("simulation.setpoint", {}, "Setpoint"), node.setpoint)}
        ${renderHVACSnapshotMetric(t("simulation.massFlow", {}, "Flow"), node.flow)}
        ${renderHVACSnapshotMetric(t("simulation.humidityRatio", {}, "Humidity"), node.humidity)}
      </div>
      <div class="simulation-hvac-flow-track" title="${escapeHTML(renderHVACSnapshotMetricText(node.flow))}">
        <i style="width:${flowPercent.toFixed(1)}%"></i>
      </div>
    </article>`;
}

function renderSimulationHVACSchematic(nodes) {
  const shown = nodes.slice(0, 8);
  if (shown.length < 2) {
    return "";
  }
  const width = 920;
  const height = 150;
  const padX = 58;
  const y = 58;
  const step = shown.length > 1 ? (width - padX * 2) / (shown.length - 1) : 0;
  const nodeGroups = shown
    .map((node, index) => {
      const x = padX + step * index;
      const active = Math.abs(Number(node.flow?.value) || 0) > 0.001;
      const value = node.temperature
        ? formatValueWithUnit(node.temperature.value, node.temperature.unit)
        : node.flow
          ? formatValueWithUnit(node.flow.value, node.flow.unit)
          : "-";
      return `
        <g class="simulation-hvac-schematic-node ${active ? "active" : ""}">
          <circle cx="${roundSVG(x)}" cy="${y}" r="17"></circle>
          <text x="${roundSVG(x)}" y="${y + 4}" text-anchor="middle">${escapeHTML(String(index + 1))}</text>
          <text x="${roundSVG(x)}" y="${y + 36}" text-anchor="middle">${escapeHTML(shortSimulationNodeLabel(node.nodeName || ""))}</text>
          <text x="${roundSVG(x)}" y="${y + 52}" text-anchor="middle">${escapeHTML(value)}</text>
        </g>`;
    })
    .join("");
  const lineStart = padX;
  const lineEnd = padX + step * (shown.length - 1);
  return `
    <div class="simulation-hvac-schematic-wrap">
      <svg class="simulation-hvac-schematic" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(t("simulation.hvacSchematic", {}, "HVAC loop schematic"))}">
        <line x1="${roundSVG(lineStart)}" y1="${y}" x2="${roundSVG(lineEnd)}" y2="${y}" class="simulation-hvac-schematic-line"></line>
        ${nodeGroups}
      </svg>
    </div>`;
}

function shortSimulationNodeLabel(value) {
  const text = String(value || "").trim();
  if (text.length <= 18) {
    return text;
  }
  return `${text.slice(0, 15)}...`;
}

function renderHVACSnapshotMetric(label, metric) {
  return `<span><b>${escapeHTML(label)}</b>${escapeHTML(renderHVACSnapshotMetricText(metric))}</span>`;
}

function renderHVACSnapshotMetricText(metric) {
  if (!metric || !Number.isFinite(Number(metric.value))) {
    return "-";
  }
  return formatValueWithUnit(metric.value, metric.unit || "");
}

function hvacComponentFrameSnapshots(loop, frameIndex) {
  const out = [];
  for (const component of loop.components || []) {
    for (const series of component.series || []) {
      const point = simulationSeriesPointAt(series, frameIndex);
      if (!point) {
        continue;
      }
      out.push({
        componentName: component.componentName || seriesNodeKey(series.column),
        componentType: component.componentType || "",
        metricName: seriesVariableName(series.column),
        value: Number(point.value),
        unit: simulationSeriesDisplayUnit(series),
      });
    }
  }
  return out.slice(0, 16);
}

function renderSimulationHVACComponentSnapshot(item) {
  return `
    <article class="simulation-hvac-component-chip" title="${escapeHTML([item.componentName, item.metricName].filter(Boolean).join(" / "))}">
      <span>${escapeHTML(item.componentName || "")}</span>
      <b>${escapeHTML(formatValueWithUnit(item.value, item.unit))}</b>
      <small>${escapeHTML(item.metricName || item.componentType || "")}</small>
    </article>`;
}

function hvacComponentSeriesCount(components) {
  return components.reduce((sum, component) => sum + (component.series || []).length, 0);
}

function renderSimulationHVACSeriesOverview(loop, nodeSeries = [], components = []) {
  const series = [
    ...nodeSeries.map((item) => ({ series: item, label: hvacSeriesLabel(item) })),
    ...components.flatMap((component) =>
      (component.series || []).map((item) => ({
        series: item,
        label: [component.componentName, seriesVariableName(item.column)].filter(Boolean).join(" / ") || hvacSeriesLabel(item),
      })),
    ),
  ].filter((item) => simulationSeriesPointCount(item.series) > 1);
  if (!series.length) {
    return "";
  }
  const shown = series.slice(0, 8);
  const frameCount = Math.max(...shown.map((item) => simulationSeriesPointCount(item.series)));
  const frameIndex = Math.round(clampNumber(state.simulationHVACFrameIndex, 0, Math.max(0, frameCount - 1)));
  const width = 920;
  const height = 260;
  const pad = { left: 48, right: 24, top: 26, bottom: 42 };
  const plotWidth = width - pad.left - pad.right;
  const plotHeight = height - pad.top - pad.bottom;
  const palette = ["#0f766e", "#2563eb", "#f59e0b", "#dc2626", "#16a34a", "#7c3aed", "#475569", "#0891b2"];
  const chartLines = shown
    .map((item, seriesIndex) => {
      const points = simulationSeriesPoints(item.series);
      const values = points.map((point) => Number(point.value)).filter(Number.isFinite);
      if (values.length < 2) {
        return "";
      }
      const min = Math.min(...values);
      const max = Math.max(...values);
      const range = max - min || 1;
      const sampleIndexes = sampledSeriesIndexes(points.length, 180);
      const polyline = sampleIndexes
        .map((pointIndex) => {
          const point = points[pointIndex] || {};
          const value = Number(point.value);
          if (!Number.isFinite(value)) {
            return "";
          }
          const x = pad.left + plotWidth * (pointIndex / Math.max(points.length - 1, 1));
          const y = pad.top + plotHeight * (1 - (value - min) / range);
          return `${roundSVG(x)},${roundSVG(y)}`;
        })
        .filter(Boolean)
        .join(" ");
      if (!polyline) {
        return "";
      }
      return `<polyline points="${polyline}" class="simulation-hvac-series-line" style="stroke:${palette[seriesIndex % palette.length]}"></polyline>`;
    })
    .join("");
  const firstPoints = simulationSeriesPoints(shown[0]?.series || {});
  const firstLabel = firstPoints[0]?.label || "start";
  const lastLabel = firstPoints[firstPoints.length - 1]?.label || "end";
  const markerX = pad.left + plotWidth * (frameIndex / Math.max(frameCount - 1, 1));
  const legend = shown
    .map((item, index) => {
      const group = simulationSeriesGroupID(item.series);
      const unit = simulationSeriesDisplayUnit(item.series);
      return `
        <span title="${escapeHTML(item.label)}">
          <i style="background:${palette[index % palette.length]}"></i>
          ${escapeHTML(shortSimulationNodeLabel(item.label))}
          ${unit ? `<small>${escapeHTML(unit)}</small>` : ""}
          <em>${escapeHTML(simulationSeriesGroupLabel(group))}</em>
        </span>`;
    })
    .join("");
  const hiddenCount = series.length - shown.length;
  return `
    <section class="simulation-hvac-series-overview">
      <div class="simulation-hvac-series-head">
        <h5>${escapeHTML(t("simulation.hvacMultiSeries", {}, "Multi-series"))}</h5>
        <span>${escapeHTML(t("simulation.hvacNormalizedSeries", {}, "Normalized by each series range"))}${hiddenCount > 0 ? ` - +${hiddenCount}` : ""}</span>
      </div>
      <div class="simulation-hvac-series-chart-wrap">
        <svg class="simulation-hvac-series-chart" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(t("simulation.hvacMultiSeries", {}, "Multi-series"))}">
          <line x1="${pad.left}" y1="${pad.top}" x2="${pad.left}" y2="${height - pad.bottom}" class="simulation-axis-line"></line>
          <line x1="${pad.left}" y1="${height - pad.bottom}" x2="${width - pad.right}" y2="${height - pad.bottom}" class="simulation-axis-line"></line>
          <line x1="${pad.left}" y1="${pad.top}" x2="${width - pad.right}" y2="${pad.top}" class="simulation-grid"></line>
          <line x1="${pad.left}" y1="${pad.top + plotHeight / 2}" x2="${width - pad.right}" y2="${pad.top + plotHeight / 2}" class="simulation-grid"></line>
          ${chartLines}
          <line x1="${roundSVG(markerX)}" y1="${pad.top}" x2="${roundSVG(markerX)}" y2="${height - pad.bottom}" class="simulation-hvac-frame-marker"></line>
          <text x="${pad.left}" y="${height - 14}" class="simulation-axis">${escapeHTML(firstLabel)}</text>
          <text x="${width - pad.right}" y="${height - 14}" text-anchor="end" class="simulation-axis">${escapeHTML(lastLabel)}</text>
          <text x="${pad.left}" y="17" class="simulation-title">${escapeHTML(t("simulation.hvacMultiSeries", {}, "Multi-series"))}</text>
        </svg>
      </div>
      <div class="simulation-hvac-series-legend">${legend}</div>
    </section>`;
}

function sampledSeriesIndexes(pointCount, maxPoints) {
  const count = Math.max(0, Number(pointCount) || 0);
  if (count <= maxPoints) {
    return Array.from({ length: count }, (_value, index) => index);
  }
  const last = count - 1;
  const step = last / Math.max(maxPoints - 1, 1);
  return Array.from({ length: maxPoints }, (_value, index) => Math.min(last, Math.round(index * step)));
}

function hvacSeriesLabel(series = {}) {
  series = safeSimulationSeries(series);
  return [seriesNodeKey(series.column), seriesVariableName(series.column)].filter(Boolean).join(" / ") || simulationSeriesDisplayColumn(series);
}

function renderSimulationHVACLoopDerivedMetrics(metrics) {
  if (!metrics.length) {
    return "";
  }
  return `
    <section class="simulation-hvac-derived-block">
      <div class="simulation-hvac-source-legend">
        <span>${escapeHTML(t("simulation.reportedByEnergyPlus", {}, "reported by EnergyPlus"))}</span>
        <span class="derived">${escapeHTML(t("simulation.derivedFromNodeState", {}, "derived from node state"))}</span>
      </div>
      <div class="simulation-hvac-derived-grid">
        ${metrics
          .map(
            (metric) => `
              <article class="simulation-hvac-derived ${escapeHTML(metric.status || "info")} ${metric.source === "derived_from_node_state" ? "derived" : "reported"}">
                <span>${escapeHTML(metric.name || "")}</span>
                <strong>${escapeHTML(formatValueWithUnit(metric.value, metric.unit))}</strong>
                <small>${escapeHTML(metric.message || simulationHVACMetricSourceLabel(metric.source || ""))}</small>
              </article>`,
          )
          .join("")}
      </div>
    </section>`;
}

function simulationHVACStatusLabel(status) {
  switch (String(status || "").toLowerCase()) {
    case "off":
      return t("simulation.hvacStatusOff", {}, "Off");
    case "flow_no_load":
      return t("simulation.hvacStatusFlowNoLoad", {}, "Flow, no load");
    case "active_heating":
      return t("simulation.hvacStatusActiveHeating", {}, "Active heating");
    case "active_cooling":
      return t("simulation.hvacStatusActiveCooling", {}, "Active cooling");
    case "setpoint_tracking":
      return t("simulation.hvacStatusSetpointTracking", {}, "Setpoint tracking");
    case "setpoint_not_met":
      return t("simulation.hvacStatusSetpointNotMet", {}, "Setpoint not met");
    case "suspicious":
      return t("simulation.hvacStatusSuspicious", {}, "Suspicious");
    default:
      return simulationDiagnosticSourceLabel(status || "unknown");
  }
}

function simulationHVACMetricSourceLabel(source) {
  return source === "derived_from_node_state"
    ? t("simulation.derivedFromNodeState", {}, "derived from node state")
    : t("simulation.reportedByEnergyPlus", {}, "reported by EnergyPlus");
}

function renderSimulationHVACLoopAlerts(alerts) {
  if (!alerts.length) {
    return "";
  }
  return `
    <div class="simulation-hvac-alert-list">
      ${alerts
        .map(
          (alert) => `
            <article class="simulation-hvac-alert ${escapeHTML(alert.severity || "info")}">
              <strong>${escapeHTML(alert.message || alert.code || "")}</strong>
              <span>${escapeHTML([alert.nodeName, alert.code, alert.source, formatOptionalValueWithUnit(alert.value, alert.unit)].filter(Boolean).join(" - "))}</span>
            </article>`,
        )
        .join("")}
    </div>`;
}

function renderSimulationHVACNodeSummaries(nodes) {
  if (!nodes.length) {
    return "";
  }
  const rows = nodes
    .slice(0, 80)
    .map(
      (node) => `
        <tr>
          <td>${escapeHTML(node.nodeName || "")}</td>
          <td>${escapeHTML(node.hasTemperature ? formatValueWithUnit(node.temperatureAverage, node.temperatureUnit) : "-")}</td>
          <td>${escapeHTML(node.hasSetpoint ? formatValueWithUnit(node.setpointAverage, node.setpointUnit) : "-")}</td>
          <td>${escapeHTML(node.temperatureSetpointSamples ? formatValueWithUnit(node.temperatureSetpointDelta, node.temperatureUnit || "C") : "-")}</td>
          <td>${escapeHTML(node.hasMassFlow ? formatValueWithUnit(node.massFlowMax, node.massFlowUnit) : "-")}</td>
          <td>${escapeHTML(node.hasMassFlow ? formatValueWithUnit((Number(node.activeMassFlowFraction) || 0) * 100, "%") : "-")}</td>
          <td>${escapeHTML(node.source || "")}</td>
        </tr>`,
    )
    .join("");
  return `
    <div class="output-table-wrap simulation-hvac-node-summary">
      <table class="output-table">
        <thead>
          <tr><th>${escapeHTML(t("common.node", {}, "Node"))}</th><th>Avg temp</th><th>Avg setpoint</th><th>Avg delta</th><th>Peak flow</th><th>Active flow</th><th>${escapeHTML(t("common.source", {}, "Source"))}</th></tr>
        </thead>
        <tbody>${rows}</tbody>
      </table>
    </div>`;
}

function renderSimulationHVACComponentOperations(components) {
  if (!components.length) {
    return "";
  }
  const rows = components
    .flatMap((component) =>
      (component.metrics || []).map((metric) => ({
        component,
        metric,
      })),
    )
    .slice(0, 120)
    .map(
      ({ component, metric }) => {
        const series = findComponentMetricSeries(component, metric);
        return `
        <tr>
          <td>${escapeHTML(component.componentName || "")}</td>
          <td>${escapeHTML(component.componentType || "")}</td>
          <td>${escapeHTML(metric.name || "")}</td>
          <td>${renderSourceInspectorCell(sourceOutputForVariable(component.componentName, metric.name), {
            series,
            keyValue: component.componentName,
            variableName: metric.name,
          })}</td>
          <td>${escapeHTML(formatValueWithUnit(metric.max, metric.unit))}</td>
          <td>${escapeHTML(formatValueWithUnit(metric.average, metric.unit))}</td>
          <td>${escapeHTML(formatValueWithUnit(metric.total, metric.unit))}</td>
          <td>${escapeHTML(metric.pointCount || 0)}</td>
        </tr>`;
      },
    )
    .join("");
  return `
    <div class="output-table-wrap simulation-hvac-component-summary">
      <table class="output-table">
        <thead>
          <tr><th>${escapeHTML(t("common.component", {}, "Component"))}</th><th>${escapeHTML(t("common.type", {}, "Type"))}</th><th>${escapeHTML(t("common.metric", {}, "Metric"))}</th><th>${escapeHTML(t("simulation.sourceOutput", {}, "Source output"))}</th><th>Peak</th><th>Avg</th><th>Total</th><th>${escapeHTML(t("common.points", {}, "Points"))}</th></tr>
        </thead>
        <tbody>${rows}</tbody>
      </table>
    </div>`;
}

function renderSimulationComfortEmpty(message) {
  if (elements.simulationComfortStats) {
    elements.simulationComfortStats.textContent = t("simulation.noComfortResult", {}, "No comfort result");
  }
  if (elements.simulationComfortResults) {
    elements.simulationComfortResults.innerHTML = `<div class="empty">${escapeHTML(message)}</div>`;
  }
}

function renderSimulationComfort(result) {
  const comfort = result?.purposeResults?.comfort || {};
  const zones = comfort.zones || [];
  const seriesCount = (comfort.series || []).length;
  if (!zones.length && !seriesCount) {
    renderSimulationComfortEmpty(t("simulation.noComfortResult", {}, "Run Comfort Check to inspect zone temperature and setpoint series."));
    return;
  }
  if (elements.simulationComfortStats) {
    elements.simulationComfortStats.textContent = t(
      "simulation.comfortStats",
      { zones: zones.length, series: seriesCount },
      `${zones.length} zones, ${seriesCount} comfort series`,
    );
  }
  const completenessHTML = (comfort.completeness || []).length
    ? renderPurposeCompletenessRow(comfort.completeness || [])
    : "";
  const periodHTML = comfort.periodScope
    ? `<div class="simulation-result-sources"><span>${escapeHTML(t("simulation.periodScope", {}, "Period scope"))}: ${escapeHTML(comfort.periodScope)}</span></div>`
    : "";
  const unmetHTML = renderComfortUnmetSummary(comfort.unmetHours || []);
  const issuesHTML = renderComfortIssueRanking(comfort.issues || []);
  const rows = zones
    .flatMap((zone) => (zone.metrics || []).map((metric) => ({ zoneName: zone.zoneName, metric })))
    .slice(0, 120)
    .map(
      ({ zoneName, metric }) => `
        <tr>
          <td>${escapeHTML(zoneName || "")}</td>
          <td>${escapeHTML(metric.name || "")}</td>
          <td>${escapeHTML(metric.unit || "")}</td>
          <td>${escapeHTML(formatNumber(metric.min))}</td>
          <td>${escapeHTML(formatNumber(metric.max))}</td>
          <td>${escapeHTML(formatNumber(metric.average))}</td>
          <td>${escapeHTML(metric.source || "")}</td>
          <td>${renderSourceInspectorCell(sourceOutputForVariable(zoneName, metric.name), { keyValue: zoneName, variableName: metric.name })}</td>
          <td>${escapeHTML(metric.points?.length || 0)}</td>
        </tr>`,
    )
    .join("");
  elements.simulationComfortResults.innerHTML = `
    ${completenessHTML}
    ${periodHTML}
    ${renderComfortTimeline(zones)}
    ${unmetHTML}
    ${issuesHTML}
    <div class="output-table-wrap">
      <table class="output-table">
        <thead><tr><th>${escapeHTML(t("common.targetZones", {}, "Target Zones"))}</th><th>${escapeHTML(t("common.metric", {}, "Metric"))}</th><th>${escapeHTML(t("common.unit", {}, "Unit"))}</th><th>Min</th><th>Max</th><th>Avg</th><th>${escapeHTML(t("common.source", {}, "Source"))}</th><th>${escapeHTML(t("simulation.sourceOutput", {}, "Source output"))}</th><th>${escapeHTML(t("common.points", {}, "Points"))}</th></tr></thead>
        <tbody>${rows || `<tr><td colspan="9">${escapeHTML(t("simulation.noComfortResult", {}, "No comfort result"))}</td></tr>`}</tbody>
      </table>
    </div>`;
}

function renderComfortTimeline(zones = []) {
  const zoneOptions = zones.filter((zone) => (zone.metrics || []).some((metric) => metric.points?.length));
  if (!zoneOptions.length) {
    return "";
  }
  if (!zoneOptions.some((zone) => zone.zoneName === state.simulationComfortZone)) {
    state.simulationComfortZone = zoneOptions[0].zoneName || "";
  }
  const zone = zoneOptions.find((item) => item.zoneName === state.simulationComfortZone) || zoneOptions[0];
  const options = zoneOptions
    .map((item) => `<option value="${escapeHTML(item.zoneName || "")}" ${item.zoneName === zone.zoneName ? "selected" : ""}>${escapeHTML(item.zoneName || "")}</option>`)
    .join("");
  return `
    <section class="simulation-comfort-timeline">
      <div class="simulation-energy-block-head">
        <h4>${escapeHTML(t("simulation.comfortTimeline", {}, "Comfort timeline"))}</h4>
        <label>
          <span>${escapeHTML(t("common.targetZones", {}, "Target Zones"))}</span>
          <select data-simulation-comfort-zone>${options}</select>
        </label>
      </div>
      ${renderComfortTimelineSVG(zone)}
    </section>`;
}

function renderComfortTimelineSVG(zone) {
  const metrics = comfortTimelineMetrics(zone);
  const base = metrics.temperature || metrics.heatingSetpoint || metrics.coolingSetpoint || metrics.humidity || metrics.heatingRate || metrics.coolingRate;
  const points = base?.points || [];
  if (points.length < 2) {
    return `<div class="empty">${escapeHTML(t("simulation.noComfortResult", {}, "No comfort result"))}</div>`;
  }
  const width = 820;
  const height = 280;
  const pad = { left: 54, right: 22, top: 18, bottom: 36 };
  const plotWidth = width - pad.left - pad.right;
  const plotHeight = height - pad.top - pad.bottom;
  const tempValues = [
    ...(metrics.temperature?.points || []).map((point) => Number(point.value)),
    ...(metrics.heatingSetpoint?.points || []).map((point) => Number(point.value)),
    ...(metrics.coolingSetpoint?.points || []).map((point) => Number(point.value)),
  ].filter(Number.isFinite);
  const minTemp = Math.min(...tempValues, 18) - 1;
  const maxTemp = Math.max(...tempValues, 26) + 1;
  const tempY = (value) => pad.top + plotHeight * (1 - (Number(value) - minTemp) / Math.max(maxTemp - minTemp, 1));
  const xFor = (index) => pad.left + (index / Math.max(points.length - 1, 1)) * plotWidth;
  const lineFor = (metric, yFor) => (metric?.points || [])
    .slice(0, points.length)
    .map((point, index) => `${roundSVG(xFor(index))},${roundSVG(yFor(point.value))}`)
    .join(" ");
  const heatLine = lineFor(metrics.heatingSetpoint, tempY);
  const coolLine = lineFor(metrics.coolingSetpoint, tempY);
  const tempLine = lineFor(metrics.temperature, tempY);
  const humidityLine = lineFor(metrics.humidity, (value) => pad.top + plotHeight * (1 - clampNumber(Number(value), 0, 100) / 100));
  const rateMax = Math.max(1, ...[...(metrics.heatingRate?.points || []), ...(metrics.coolingRate?.points || [])].map((point) => Math.abs(Number(point.value) || 0)));
  const barWidth = Math.max(1, plotWidth / Math.max(points.length, 1));
  const rateBars = [
    ...comfortRateBars(metrics.heatingRate, points.length, xFor, barWidth, height - pad.bottom, rateMax, "#dc2626"),
    ...comfortRateBars(metrics.coolingRate, points.length, xFor, barWidth, height - pad.bottom, rateMax, "#2563eb"),
  ].join("");
  const deviations = comfortDeviationBands(metrics, points.length, xFor, barWidth, pad.top, plotHeight);
  const labels = [points[0]?.label || "start", points[points.length - 1]?.label || "end"];
  return `
    <div class="simulation-comfort-chart-wrap">
      <svg class="simulation-comfort-chart" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(zone.zoneName || "")} comfort timeline">
        <line x1="${pad.left}" x2="${width - pad.right}" y1="${height - pad.bottom}" y2="${height - pad.bottom}" class="simulation-axis-line" />
        <line x1="${pad.left}" x2="${pad.left}" y1="${pad.top}" y2="${height - pad.bottom}" class="simulation-axis-line" />
        ${deviations}
        ${heatLine && coolLine ? `<polygon points="${comfortBandPolygon(heatLine, coolLine)}" class="comfort-setpoint-band"></polygon>` : ""}
        ${heatLine ? `<polyline points="${heatLine}" class="comfort-setpoint-line heat" />` : ""}
        ${coolLine ? `<polyline points="${coolLine}" class="comfort-setpoint-line cool" />` : ""}
        ${humidityLine ? `<polyline points="${humidityLine}" class="comfort-humidity-line" />` : ""}
        ${rateBars}
        ${tempLine ? `<polyline points="${tempLine}" class="comfort-temperature-line" />` : ""}
        <text x="8" y="${pad.top + 8}" class="simulation-axis">${escapeHTML(formatTemperature(maxTemp))}</text>
        <text x="8" y="${height - pad.bottom}" class="simulation-axis">${escapeHTML(formatTemperature(minTemp))}</text>
        <text x="${pad.left}" y="${height - 12}" class="simulation-axis">${escapeHTML(labels[0])}</text>
        <text x="${width - pad.right}" y="${height - 12}" text-anchor="end" class="simulation-axis">${escapeHTML(labels[1])}</text>
      </svg>
      <div class="simulation-energy-legend">
        <span><i style="background:#f59e0b"></i>${escapeHTML(t("simulation.temperature", {}, "Temperature"))}</span>
        <span><i style="background:#dc2626"></i>${escapeHTML(t("simulation.heating", {}, "Heating"))}</span>
        <span><i style="background:#2563eb"></i>${escapeHTML(t("simulation.cooling", {}, "Cooling"))}</span>
        <span><i style="background:#0f766e"></i>${escapeHTML(t("simulation.humidity", {}, "Humidity"))}</span>
      </div>
    </div>`;
}

function comfortTimelineMetrics(zone) {
  const byName = new Map((zone.metrics || []).map((metric) => [normalizeOutputMatchToken(metric.name), metric]));
  return {
    temperature: byName.get("zone mean air temperature"),
    humidity: byName.get("zone air relative humidity"),
    heatingSetpoint: byName.get("zone thermostat heating setpoint temperature"),
    coolingSetpoint: byName.get("zone thermostat cooling setpoint temperature"),
    heatingRate: byName.get("zone air system sensible heating rate"),
    coolingRate: byName.get("zone air system sensible cooling rate"),
  };
}

function comfortBandPolygon(heatLine, coolLine) {
  const heatPoints = heatLine.split(" ");
  const coolPoints = coolLine.split(" ").reverse();
  return [...heatPoints, ...coolPoints].join(" ");
}

function comfortRateBars(metric, pointCount, xFor, barWidth, baseline, maxAbs, color) {
  return (metric?.points || []).slice(0, pointCount).map((point, index) => {
    const value = Math.abs(Number(point.value) || 0);
    if (value <= 1e-9) {
      return "";
    }
    const height = Math.max(1, Math.min(42, value / maxAbs * 42));
    return `<rect x="${roundSVG(xFor(index) - barWidth / 2)}" y="${roundSVG(baseline - height)}" width="${roundSVG(barWidth)}" height="${roundSVG(height)}" fill="${color}" opacity="0.38"><title>${escapeHTML(formatValueWithUnit(point.value, metric.unit || ""))}</title></rect>`;
  });
}

function comfortDeviationBands(metrics, pointCount, xFor, barWidth, top, height) {
  const temp = metrics.temperature?.points || [];
  const heat = metrics.heatingSetpoint?.points || [];
  const cool = metrics.coolingSetpoint?.points || [];
  if (!temp.length || (!heat.length && !cool.length)) {
    return "";
  }
  return temp.slice(0, pointCount).map((point, index) => {
    const value = Number(point.value);
    const heatValue = Number(heat[index]?.value);
    const coolValue = Number(cool[index]?.value);
    const outside = (Number.isFinite(heatValue) && value < heatValue) || (Number.isFinite(coolValue) && value > coolValue);
    return outside ? `<rect x="${roundSVG(xFor(index) - barWidth / 2)}" y="${top}" width="${roundSVG(barWidth)}" height="${height}" class="comfort-deviation-band"><title>${escapeHTML(point.label || "")}</title></rect>` : "";
  }).join("");
}

function renderComfortUnmetSummary(items = []) {
  if (!items.length) {
    return "";
  }
  const rows = items
    .slice(0, 24)
    .map(
      (item) => `
        <tr>
          <td>${escapeHTML(item.zoneName || "")}</td>
          <td>${escapeHTML(item.metric || "")}</td>
          <td>${escapeHTML(formatValueWithUnit(item.value || 0, item.unit || ""))}</td>
          <td>${escapeHTML(item.report || "")}</td>
          <td>${escapeHTML(item.table || "")}</td>
          <td>${escapeHTML(item.source || "")}</td>
        </tr>`,
    )
    .join("");
  return `
    <div class="output-table-wrap simulation-comfort-unmet">
      <table class="output-table">
        <thead><tr><th>${escapeHTML(t("common.targetZones", {}, "Target Zones"))}</th><th>${escapeHTML(t("common.metric", {}, "Metric"))}</th><th>${escapeHTML(t("common.value", {}, "Value"))}</th><th>${escapeHTML(t("simulation.report", {}, "Report"))}</th><th>${escapeHTML(t("simulation.table", {}, "Table"))}</th><th>${escapeHTML(t("common.source", {}, "Source"))}</th></tr></thead>
        <tbody>${rows}</tbody>
      </table>
    </div>`;
}

function renderComfortIssueRanking(issues = []) {
  if (!issues.length) {
    return "";
  }
  const rows = issues
    .slice(0, 24)
    .map(
      (issue) => `
        <tr>
          <td>${escapeHTML(issue.zoneName || "")}</td>
          <td>${escapeHTML(issue.unmetSamples || 0)}</td>
          <td>${escapeHTML(issue.heatingSamples || 0)}</td>
          <td>${escapeHTML(issue.coolingSamples || 0)}</td>
          <td>${escapeHTML(formatValueWithUnit(issue.maxDeviation || 0, issue.unit || ""))}</td>
          <td>${escapeHTML(formatValueWithUnit(issue.averageDeviation || 0, issue.unit || ""))}</td>
          <td>${escapeHTML(issue.peakLabel || "")}</td>
          <td>${escapeHTML(issue.source || "")}</td>
        </tr>`,
    )
    .join("");
  return `
    <div class="output-table-wrap simulation-comfort-issues">
      <table class="output-table">
        <thead><tr><th>${escapeHTML(t("common.targetZones", {}, "Target Zones"))}</th><th>${escapeHTML(t("simulation.unmetSamples", {}, "Unmet samples"))}</th><th>${escapeHTML(t("simulation.heating", {}, "Heating"))}</th><th>${escapeHTML(t("simulation.cooling", {}, "Cooling"))}</th><th>${escapeHTML(t("simulation.maxDeviation", {}, "Max deviation"))}</th><th>${escapeHTML(t("simulation.avgDeviation", {}, "Avg deviation"))}</th><th>${escapeHTML(t("common.time", {}, "Time"))}</th><th>${escapeHTML(t("common.source", {}, "Source"))}</th></tr></thead>
        <tbody>${rows}</tbody>
      </table>
    </div>`;
}

function renderPurposeCompletenessRow(items) {
  if (!items.length) {
    return "";
  }
  return `<div class="simulation-completeness-row">${items
    .map((item) => {
      const source = item.source ? ` - ${item.source}` : "";
      const status = item.status || (item.found ? "found" : "missing");
      return `<span class="${escapeHTML(status)}" title="${escapeHTML(`${item.requiredOutput || ""}${source}`)}">${escapeHTML(item.requiredOutput || "")}</span>`;
    })
    .join("")}</div>`;
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

function renderEnergyMonthlyChart(title, series) {
  const frames = energyChartFrames(series);
  if (!series.length || !frames.length) {
    return "";
  }
  const width = 760;
  const height = 240;
  const margin = { top: 18, right: 18, bottom: 44, left: 58 };
  const plotWidth = width - margin.left - margin.right;
  const plotHeight = height - margin.top - margin.bottom;
  const colors = ["#0f766e", "#2563eb", "#d97706", "#16a34a", "#9333ea", "#dc2626", "#0891b2", "#4f46e5"];
  const stacks = frames.map((frame) => {
    let total = 0;
    const segments = series.map((item, index) => {
      const value = Math.max(0, energyPointValue(item, frame.key));
      const segment = { name: item.name || "", value, color: colors[index % colors.length], y0: total, y1: total + value };
      total += value;
      return segment;
    });
    return { ...frame, total, segments };
  });
  const maxTotal = Math.max(...stacks.map((frame) => frame.total), 1);
  const barGap = 8;
  const barWidth = Math.max(8, (plotWidth - barGap * Math.max(0, stacks.length - 1)) / stacks.length);
  const bars = stacks
    .map((frame, index) => {
      const x = margin.left + index * (barWidth + barGap);
      const segments = frame.segments
        .filter((segment) => segment.value > 0)
        .map((segment) => {
          const y = margin.top + plotHeight - (segment.y1 / maxTotal) * plotHeight;
          const segmentHeight = Math.max(1, ((segment.y1 - segment.y0) / maxTotal) * plotHeight);
          return `<rect x="${roundSVG(x)}" y="${roundSVG(y)}" width="${roundSVG(barWidth)}" height="${roundSVG(segmentHeight)}" fill="${segment.color}"><title>${escapeHTML(`${segment.name}: ${formatEnergyValue(segment.value, series[0]?.unit || "")}`)}</title></rect>`;
        })
        .join("");
      const labelY = margin.top + plotHeight + 16;
      return `${segments}<text x="${roundSVG(x + barWidth / 2)}" y="${labelY}" class="simulation-axis" text-anchor="middle">${escapeHTML(frame.label)}</text>`;
    })
    .join("");
  const legend = series
    .slice(0, 8)
    .map((item, index) => `<span><i style="background:${colors[index % colors.length]}"></i>${escapeHTML(item.name || "")}</span>`)
    .join("");
  return `
    <section class="simulation-energy-block">
      <h4>${escapeHTML(title)}</h4>
      <div class="simulation-energy-chart-wrap">
        <svg class="simulation-energy-chart" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(title)}">
          <line x1="${margin.left}" y1="${margin.top + plotHeight}" x2="${width - margin.right}" y2="${margin.top + plotHeight}" class="simulation-axis-line"></line>
          <line x1="${margin.left}" y1="${margin.top}" x2="${margin.left}" y2="${margin.top + plotHeight}" class="simulation-axis-line"></line>
          <text x="${margin.left}" y="12" class="simulation-title">${escapeHTML(formatEnergyValue(maxTotal, series[0]?.unit || ""))}</text>
          ${bars}
        </svg>
        <div class="simulation-energy-legend">${legend}</div>
      </div>
    </section>`;
}

function energyChartFrames(series) {
  const frameMap = new Map();
  for (const item of series) {
    for (const point of item.points || []) {
      const key = String(point.x ?? point.label ?? frameMap.size + 1);
      if (!frameMap.has(key)) {
        frameMap.set(key, { key, label: energyPointLabel(point, frameMap.size) });
      }
    }
  }
  return [...frameMap.values()].slice(0, 12);
}

function energyPointLabel(point, index) {
  const label = String(point.label || "").trim();
  if (label) {
    const monthMatch = label.match(/^(\d{1,2})\//);
    return monthMatch ? `M${monthMatch[1]}` : label.slice(0, 8);
  }
  return String(point.x || index + 1);
}

function energyPointValue(series, key) {
  const point = (series.points || []).find((item) => String(item.x ?? item.label ?? "") === key);
  return Number(point?.value) || 0;
}

function renderZoneEnergyMatrix(zones) {
  const frames = energyChartFrames(zones);
  if (!zones.length || !frames.length) {
    return "";
  }
  const metricOptions = zoneEnergyMetricOptions(zones);
  const selectedMetric = metricOptions.some((option) => option.value === state.simulationZoneEnergyMetric)
    ? state.simulationZoneEnergyMetric
    : metricOptions[0]?.value || "__total";
  state.simulationZoneEnergyMetric = selectedMetric;
  const rows = zoneEnergyMatrixRows(zones, frames, selectedMetric)
    .slice()
    .sort((a, b) => Math.abs(Number(b.total) || 0) - Math.abs(Number(a.total) || 0))
    .slice(0, 24);
  if (!rows.length) {
    return "";
  }
  const maxValue = Math.max(...rows.flatMap((row) => frames.map((frame) => Math.abs(row.values.get(frame.key) || 0))), 1);
  const header = frames.map((frame) => `<th>${escapeHTML(frame.label)}</th>`).join("");
  const options = metricOptions
    .map((option) => `<option value="${escapeHTML(option.value)}" ${option.value === selectedMetric ? "selected" : ""}>${escapeHTML(option.label)}</option>`)
    .join("");
  const body = rows
    .map((row) => {
      const cells = frames
        .map((frame) => {
          const value = row.values.get(frame.key) || 0;
          const alpha = Math.min(0.82, Math.max(0.08, Math.abs(value) / maxValue));
          return `<td><span style="background:rgba(15,118,110,${alpha})" title="${escapeHTML(zoneEnergyCellTitle(row, frame, value))}"></span></td>`;
        })
        .join("");
      return `<tr><th title="${escapeHTML(zoneEnergyRowTitle(row))}">${escapeHTML(row.zoneName || "")}</th>${cells}</tr>`;
    })
    .join("");
  return `
    <section class="simulation-energy-block">
      <div class="simulation-energy-block-head">
        <h4>${escapeHTML(t("simulation.zoneReportedEnergyMatrix", {}, "Zone reported energy matrix"))}</h4>
        <label>
          <span>${escapeHTML(t("common.metric", {}, "Metric"))}</span>
          <select data-simulation-zone-energy-metric>${options}</select>
        </label>
      </div>
      <div class="simulation-zone-energy-matrix">
        <table>
          <thead><tr><th>${escapeHTML(t("common.targetZones", {}, "Target Zones"))}</th>${header}</tr></thead>
          <tbody>${body}</tbody>
        </table>
      </div>
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
          <td title="${escapeHTML(zoneEnergySourceTitle(item))}">${escapeHTML(item.metric || "")}</td>
          <td>${escapeHTML(formatEnergyValue(Number(item.total) || 0, item.unit || ""))}</td>
          <td>${escapeHTML(item.source || "")}</td>
          <td>${renderSourceInspectorCell(sourceOutputForVariable(item.zoneName, item.metric), { keyValue: item.zoneName, variableName: item.metric })}</td>
        </tr>`,
    )
    .join("");
  return `
    <section class="simulation-energy-block">
      <h4>${escapeHTML(t("simulation.zoneEnergy", {}, "Zone reported energy"))}</h4>
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>${escapeHTML(t("common.targetZones", {}, "Target Zones"))}</th><th>${escapeHTML(t("common.metric", {}, "Metric"))}</th><th>${escapeHTML(t("common.value", {}, "Value"))}</th><th>${escapeHTML(t("common.source", {}, "Source"))}</th><th>${escapeHTML(t("simulation.sourceOutput", {}, "Source output"))}</th></tr></thead>
          <tbody>${rows || `<tr><td colspan="5">${escapeHTML(t("simulation.noZoneEnergy", {}, "No zone reported energy series found."))}</td></tr>`}</tbody>
        </table>
      </div>
    </section>`;
}

function zoneEnergyMetricOptions(zones) {
  const metrics = [...new Set((zones || []).map((zone) => String(zone.metric || "").trim()).filter(Boolean))].sort((a, b) => a.localeCompare(b));
  return [
    { value: "__total", label: t("simulation.totalReportedEnergy", {}, "Total reported") },
    ...metrics.map((metric) => ({ value: metric, label: metric })),
  ];
}

function zoneEnergyMatrixRows(zones, frames, selectedMetric) {
  const byZone = new Map();
  for (const item of zones || []) {
    const metric = String(item.metric || "").trim();
    if (!metric || (selectedMetric !== "__total" && metric !== selectedMetric)) {
      continue;
    }
    const zoneName = String(item.zoneName || t("common.unknown", {}, "Unknown")).trim();
    const row = byZone.get(zoneName) || {
      zoneName,
      metricLabel: selectedMetric === "__total" ? t("simulation.totalReportedEnergy", {}, "Total reported") : selectedMetric,
      variables: new Set(),
      sources: new Set(),
      units: new Set(),
      values: new Map(frames.map((frame) => [frame.key, 0])),
      total: 0,
    };
    row.variables.add(metric);
    if (item.source) {
      row.sources.add(item.source);
    }
    if (item.unit) {
      row.units.add(item.unit);
    }
    row.total += Number(item.total) || 0;
    for (const frame of frames) {
      row.values.set(frame.key, (row.values.get(frame.key) || 0) + energyPointValue(item, frame.key));
    }
    byZone.set(zoneName, row);
  }
  return [...byZone.values()].map((row) => ({
    ...row,
    variables: [...row.variables].sort((a, b) => a.localeCompare(b)),
    sources: [...row.sources].sort((a, b) => a.localeCompare(b)),
    unit: row.units.size === 1 ? [...row.units][0] : "",
  }));
}

function zoneEnergyCellTitle(row, frame, value) {
  return [
    `${row.zoneName || ""} - ${row.metricLabel || ""}`,
    `${frame.label}: ${formatEnergyValue(value, row.unit || "")}`,
    zoneEnergySourceLine(row.variables),
    row.sources.length ? `${t("common.source", {}, "Source")}: ${row.sources.join(", ")}` : "",
  ]
    .filter(Boolean)
    .join("\n");
}

function zoneEnergyRowTitle(row) {
  return [row.metricLabel || "", zoneEnergySourceLine(row.variables), row.sources.length ? `${t("common.source", {}, "Source")}: ${row.sources.join(", ")}` : ""]
    .filter(Boolean)
    .join("\n");
}

function zoneEnergySourceTitle(item) {
  return [zoneEnergySourceLine([item.metric]), item.source ? `${t("common.source", {}, "Source")}: ${item.source}` : ""].filter(Boolean).join("\n");
}

function zoneEnergySourceLine(variables) {
  const names = (variables || []).map((value) => String(value || "").trim()).filter(Boolean);
  return names.length ? `${t("simulation.sourceVariables", {}, "Source variables")}: ${names.join(", ")}` : "";
}

function formatEnergyValue(value, unit) {
  const safeUnit = unit || "";
  return `${formatNumber(value)}${safeUnit ? ` ${safeUnit}` : ""}`;
}

function formatValueWithUnit(value, unit) {
  return formatEnergyValue(value, unit);
}

function formatOptionalValueWithUnit(value, unit) {
  return Number.isFinite(Number(value)) ? formatValueWithUnit(value, unit) : "";
}

function safeSimulationSeries(series) {
  return series && typeof series === "object" ? series : {};
}

function safeSimulationSeriesList(series) {
  return Array.isArray(series) ? series.filter((item) => item && typeof item === "object") : [];
}

function simulationSeriesDisplayColumn(series = {}) {
  series = safeSimulationSeries(series);
  return series.displayColumn || series.column || "";
}

function simulationSeriesDisplayUnit(series = {}) {
  series = safeSimulationSeries(series);
  return series.displayUnit || seriesColumnUnit(simulationSeriesDisplayColumn(series));
}

function simulationSeriesPoints(series = {}) {
  series = safeSimulationSeries(series);
  return Array.isArray(series.displayPoints) && series.displayPoints.length === (series.points || []).length
    ? series.displayPoints
    : (series.points || []);
}

function simulationSeriesPointCount(series = {}) {
  return simulationSeriesPoints(series).length;
}

function simulationSeriesPointAt(series = {}, frameIndex = 0) {
  const points = simulationSeriesPoints(series);
  if (!points.length) {
    return null;
  }
  return points[Math.min(frameIndex, Math.max(0, points.length - 1))] || null;
}

function formatSeriesStat(series = {}, key = "average") {
  series = safeSimulationSeries(series);
  const displayKey = `display${key.slice(0, 1).toUpperCase()}${key.slice(1)}`;
  const displayValue = Number(series[displayKey]);
  const rawValue = Number(series[key]);
  const value = Number.isFinite(displayValue) ? displayValue : rawValue;
  return Number.isFinite(value) ? formatValueWithUnit(value, simulationSeriesDisplayUnit(series)) : "";
}

function sourceOutputForSeriesColumn(column) {
  return sourceOutputForVariable(seriesNodeKey(column), seriesVariableName(column));
}

function sourceOutputForEnergySource(source = {}) {
  const indexed = findPurposeOutputObjectByIndex(source.objectIndex);
  if (indexed) {
    return indexed;
  }
  return source.isMeter
    ? findPurposeOutputObject("Output:Meter", source.keyValue || source.name || "", "")
    : sourceOutputForVariable(source.keyValue || "", source.name || "");
}

function sourceOutputForVariable(keyValue, variableName) {
  return findPurposeOutputObject("Output:Variable", keyValue, variableName);
}

function findPurposeOutputObjectByIndex(objectIndex) {
  const index = Number(objectIndex);
  if (!Number.isFinite(index)) {
    return null;
  }
  return activePurposeOutputObjects().find((object) => Number(object.objectIndex) === index) || null;
}

function findPurposeOutputObject(objectType, keyValue, variableName) {
  const objectTypeKey = normalizeOutputMatchToken(objectType);
  const key = normalizeOutputMatchToken(keyValue);
  const variable = normalizeOutputMatchToken(variableName);
  const candidates = activePurposeOutputObjects().filter((object) => {
    if (normalizeOutputMatchToken(object.objectType) !== objectTypeKey) {
      return false;
    }
    if (objectTypeKey === "output:variable") {
      return normalizeOutputMatchToken(object.variableName) === variable && purposeOutputKeyMatches(object.keyValue, key);
    }
    return purposeOutputKeyMatches(object.keyValue, key);
  });
  return candidates.sort((a, b) => purposeSourceOutputRank(b, key) - purposeSourceOutputRank(a, key))[0] || null;
}

function activePurposeOutputObjects() {
  return state.simulationResult?.purposeRunPlan?.outputObjects || state.simulationPurposePlan?.outputObjects || [];
}

function purposeOutputKeyMatches(objectKeyValue, resultKey) {
  const objectKey = normalizeOutputMatchToken(objectKeyValue);
  return objectKey === resultKey || objectKey === "*" || objectKey === "";
}

function purposeSourceOutputRank(object, resultKey) {
  const objectKey = normalizeOutputMatchToken(object.keyValue);
  if (objectKey === resultKey) {
    return 3;
  }
  if (objectKey === "*") {
    return 2;
  }
  return 1;
}

function renderSourceOutputCell(object, options = {}) {
  if (!object) {
    return `<span class="simulation-source-output missing">${escapeHTML(t("common.notAvailable", {}, "N/A"))}</span>`;
  }
  const signature = object.signature || [object.objectType, object.keyValue, object.variableName, object.reportingFrequency].filter(Boolean).join(" / ");
  const stateLabel = outputStateLabel(object.state || "");
  const objectIndex = Number(object.objectIndex);
  const jump = Number.isFinite(objectIndex)
    ? `<button class="profile-object-link navigable-row simulation-source-output-jump" type="button" data-jump-object-index="${escapeHTML(objectIndex)}" data-jump-object-type="${escapeHTML(object.objectType || "")}">#${escapeHTML(objectIndex + 1)}</button>`
    : "";
  const signatureHTML = options.compact ? "" : `<small class="simulation-source-signature" title="${escapeHTML(signature)}">${escapeHTML(signature)}</small>`;
  return `${jump}<span class="simulation-source-output ${escapeHTML(object.state || "")}" title="${escapeHTML(signature)}">${escapeHTML(stateLabel)}</span>${signatureHTML}`;
}

function renderSourceInspectorCell(object, seriesRef = {}) {
  return `<div class="simulation-source-cell">${renderSourceOutputCell(object)}${renderSeriesInspectButton(seriesRef)}</div>`;
}

function renderSeriesInspectButton(seriesRef = {}) {
  const series = seriesRef.series || findSimulationSeriesForMetric(seriesRef.keyValue, seriesRef.variableName);
  const id = series ? seriesID(series) : "";
  const label = t("simulation.inspectSeriesAction", {}, "Chart");
  const title = series
    ? t("simulation.inspectSeries", {}, "Inspect this output in the common Series chart")
    : t("simulation.inspectSeriesUnavailable", {}, "No matching SQL/CSV series is available for this row");
  return `
    <button
      type="button"
      class="simulation-series-inspect"
      data-simulation-inspect-series="1"
      data-simulation-series-id="${escapeHTML(id)}"
      data-simulation-series-key="${escapeHTML(seriesRef.keyValue || "")}"
      data-simulation-series-metric="${escapeHTML(seriesRef.variableName || "")}"
      title="${escapeHTML(title)}"
      ${series ? "" : "disabled"}
    >${escapeHTML(label)}</button>`;
}

function handleSimulationSeriesInspectClick(event) {
  if (!(event.target instanceof Element)) {
    return;
  }
  const energyViewButton = event.target.closest("[data-simulation-energy-view]");
  if (energyViewButton && !energyViewButton.disabled) {
    state.simulationEnergyView = energyViewButton.dataset.simulationEnergyView || "overview";
    renderSimulationEnergyDashboard(state.simulationResult);
    return;
  }
  const energyNode = event.target.closest("[data-energy-explanation-node]");
  if (energyNode) {
    state.simulationEnergySelection = energyNode.dataset.energyExplanationNode || "";
    renderSimulationEnergyDashboard(state.simulationResult);
    return;
  }
  const energyEdge = event.target.closest("[data-energy-explanation-edge]");
  if (energyEdge) {
    state.simulationEnergySelection = energyEdge.dataset.energyExplanationEdge || "";
    renderSimulationEnergyDashboard(state.simulationResult);
    return;
  }
  const energyPeriodJump = event.target.closest("[data-simulation-energy-period-jump]");
  if (energyPeriodJump) {
    state.simulationEnergyPeriod = energyPeriodJump.dataset.simulationEnergyPeriodJump || "annual";
    state.simulationEnergySelection = "";
    state.simulationEnergyView = "sankey";
    renderSimulationEnergyDashboard(state.simulationResult);
    return;
  }
  const hvacPath = event.target.closest("[data-simulation-hvac-path-id]");
  if (hvacPath) {
    openSimulationHVACServicePath(hvacPath.dataset.simulationHvacPathId || "");
    return;
  }
  const button = event.target.closest("[data-simulation-inspect-series]");
  if (!button || button.disabled) {
    return;
  }
  const series = findSimulationSeriesByID(button.dataset.simulationSeriesId || "")
    || findSimulationSeriesForMetric(button.dataset.simulationSeriesKey || "", button.dataset.simulationSeriesMetric || "");
  if (!series) {
    setStatus(t("simulation.inspectSeriesUnavailable", {}, "No matching SQL/CSV series is available for this row"), "warn");
    return;
  }
  selectSimulationSeries(series);
}

function openSimulationHVACServicePath(pathID) {
  const path = simulationHVACServicePaths().find((item) => item.id === pathID);
  if (!path) {
    setStatus(t("hvac.noServicePaths", {}, "No service paths"), "warn");
    return;
  }
  navigateHVAC(
    {
      kind: "service_path",
      id: simulationNavigationPathEntityID(path.id),
      label: simulationServedSubjectLabel(path.servedSubject || path),
      view: "services",
      context: { pathId: path.id },
      graphKey: simulationServicePathGraphKey(path),
    },
    { pushHistory: true },
  );
  const hvacTab = [...(elements.resultTabButtons || [])].find((button) => button.dataset.resultTab === "hvac");
  if (hvacTab) {
    hvacTab.click();
  } else {
    state.activeResultTab = "hvac";
  }
}

function handleSimulationEnergyDashboardChange(event) {
  if (!(event.target instanceof Element)) {
    return;
  }
  const period = event.target.closest("[data-simulation-energy-period]");
  if (period) {
    state.simulationEnergyPeriod = period.value || "annual";
    state.simulationEnergySelection = "";
    renderSimulationEnergyDashboard(state.simulationResult);
    return;
  }
  const focusMode = event.target.closest("[data-simulation-energy-focus-mode]");
  if (focusMode) {
    state.simulationEnergyFocusMode = focusMode.value || "all";
    state.simulationEnergySelection = "";
    renderSimulationEnergyDashboard(state.simulationResult);
    return;
  }
  const zoneFocus = event.target.closest("[data-simulation-energy-zone-focus]");
  if (zoneFocus) {
    state.simulationEnergyZoneFocus = zoneFocus.value || "";
    state.simulationEnergySelection = "";
    renderSimulationEnergyDashboard(state.simulationResult);
    return;
  }
  const pathFocus = event.target.closest("[data-simulation-energy-service-path-focus]");
  if (pathFocus) {
    state.simulationEnergyServicePathFocus = pathFocus.value || "";
    state.simulationEnergySelection = "";
    renderSimulationEnergyDashboard(state.simulationResult);
    return;
  }
  const signMode = event.target.closest("[data-simulation-energy-sign-mode]");
  if (signMode) {
    state.simulationEnergySignMode = energyExplanationSignMode(signMode.value || "display");
    state.simulationEnergySelection = "";
    renderSimulationEnergyDashboard(state.simulationResult);
    return;
  }
  const nodeLimit = event.target.closest("[data-simulation-energy-node-limit]");
  if (nodeLimit) {
    state.simulationEnergyNodeLimit = energyExplanationNodeLimit(nodeLimit.value);
    state.simulationEnergySelection = "";
    renderSimulationEnergyDashboard(state.simulationResult);
    return;
  }
  const select = event.target.closest("[data-simulation-zone-energy-metric]");
  if (!select) {
    return;
  }
  state.simulationZoneEnergyMetric = select.value || "__total";
  renderSimulationEnergyDashboard(state.simulationResult);
}

function handleSimulationHVACResultsInput(event) {
  if (!(event.target instanceof Element)) {
    return;
  }
  const groupToggle = event.target.closest("[data-simulation-hvac-group-toggle]");
  if (groupToggle) {
    state.simulationHVACVisibleGroups = state.simulationHVACVisibleGroups || {};
    state.simulationHVACVisibleGroups[groupToggle.dataset.simulationHvacGroupToggle || "other"] = Boolean(groupToggle.checked);
    renderSimulationHVACLoops(state.simulationResult);
    return;
  }
  const panelToggle = event.target.closest("[data-simulation-hvac-panel-toggle]");
  if (panelToggle) {
    state.simulationHVACPanels = state.simulationHVACPanels || {};
    state.simulationHVACPanels[panelToggle.dataset.simulationHvacPanelToggle || "snapshot"] = Boolean(panelToggle.checked);
    renderSimulationHVACLoops(state.simulationResult);
    return;
  }
  const input = event.target.closest("[data-simulation-hvac-frame]");
  if (!input) {
    return;
  }
  state.simulationHVACFrameIndex = Number(input.value) || 0;
  renderSimulationHVACLoops(state.simulationResult);
}

function handleSimulationComfortResultsChange(event) {
  if (!(event.target instanceof Element)) {
    return;
  }
  const select = event.target.closest("[data-simulation-comfort-zone]");
  if (!select) {
    return;
  }
  state.simulationComfortZone = select.value || "";
  renderSimulationComfort(state.simulationResult);
}

function selectSimulationSeries(series) {
  state.simulationSelectedSeries = seriesID(series);
  state.simulationSeriesRangeStart = 0;
  state.simulationSeriesRangeEnd = -1;
  state.simulationActiveResultView = "series";
  renderSimulationResultTabs(state.simulationResult);
  toggleSimulationResultSections();
  renderSimulationSeriesSelect(state.simulationResult || {});
  renderSimulationChart();
  setStatus(t("simulation.inspectSeriesOpened", {}, "Series chart opened"), "ok");
  elements.simulationChart?.scrollIntoView({ block: "nearest" });
}

function findSimulationSeriesByID(id) {
  if (!id) {
    return null;
  }
  return safeSimulationSeriesList(state.simulationResult?.series).find((series) => seriesID(series) === id) || null;
}

function findSimulationSeriesForMetric(keyValue, variableName) {
  const key = normalizeOutputMatchToken(keyValue);
  const variable = normalizeOutputMatchToken(variableName);
  if (!key || !variable) {
    return null;
  }
  return safeSimulationSeriesList(state.simulationResult?.series).find((series) => {
    return (key === "*" || normalizeOutputMatchToken(seriesNodeKey(series.column)) === key)
      && normalizeOutputMatchToken(seriesVariableName(series.column)) === variable;
  }) || null;
}

function findSimulationSeriesForMeter(meterName) {
  const meter = normalizeOutputMatchToken(meterName);
  if (!meter) {
    return null;
  }
  return safeSimulationSeriesList(state.simulationResult?.series).find((series) => normalizeOutputMatchToken(seriesColumnMainName(series.column)) === meter) || null;
}

function findComponentMetricSeries(component = {}, metric = {}) {
  const componentName = normalizeOutputMatchToken(component.componentName);
  const metricName = normalizeOutputMatchToken(metric.name);
  return (component.series || []).find((series) => {
    return normalizeOutputMatchToken(seriesNodeKey(series.column)) === componentName
      && normalizeOutputMatchToken(seriesVariableName(series.column)) === metricName;
  }) || null;
}

function normalizeOutputMatchToken(value) {
  return String(value || "").trim().toLowerCase().replace(/\s+/g, " ");
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

async function refreshSimulationOutputDiscovery({ force = false } = {}) {
  if (!elements.simulationOutputDiscoveryList) {
    return null;
  }
  const text = elements.idfInput?.value || "";
  const outputDirectory = state.simulationResult?.outputDirectory || "";
  if (!text.trim() && !outputDirectory) {
    state.simulationOutputDiscovery = null;
    state.simulationOutputDiscoveryKey = "";
    state.simulationOutputDiscoveryError = "";
    state.simulationOutputDiscoveryLoading = false;
    renderSimulationOutputDiscovery();
    return null;
  }
  const purposeRequest = { ...buildSimulationPurposeRequest(), discoveryAllowed: true };
  const key = `${hashString(text)}:${outputDirectory}:${JSON.stringify(purposeRequest)}`;
  if (!force && key === state.simulationOutputDiscoveryKey && (state.simulationOutputDiscovery || state.simulationOutputDiscoveryLoading)) {
    return state.simulationOutputDiscovery;
  }
  state.simulationOutputDiscoveryKey = key;
  state.simulationOutputDiscoveryLoading = true;
  state.simulationOutputDiscoveryError = "";
  renderSimulationOutputDiscovery();
  try {
    const result = await callSimulationAPI("DiscoverAvailableOutputs", "/api/simulation-output-discovery", {
      text,
      outputDirectory,
      purposeRequest,
    });
    state.simulationOutputDiscovery = result;
    state.simulationOutputDiscoveryError = "";
    return result;
  } catch (error) {
    state.simulationOutputDiscovery = null;
    state.simulationOutputDiscoveryError = error?.message || String(error);
    return null;
  } finally {
    state.simulationOutputDiscoveryLoading = false;
    renderSimulationOutputDiscovery();
  }
}

function renderSimulationOutputDiscovery() {
  if (!elements.simulationOutputDiscoveryList) {
    return;
  }
  const customSelected = selectedSimulationPurposes().includes("custom_outputs");
  const result = state.simulationOutputDiscovery;
  const items = Array.isArray(result?.items) ? result.items : [];
  const query = (elements.simulationOutputDiscoveryFilter?.value || state.simulationOutputDiscoveryQuery || "").trim().toLowerCase();
  const filtered = items
    .map((item, index) => ({ item, index }))
    .filter(({ item }) => discoveryItemMatchesQuery(item, query));
  const shown = filtered.slice(0, 40);
  if (elements.simulationOutputDiscoveryStats) {
    if (state.simulationOutputDiscoveryLoading) {
      elements.simulationOutputDiscoveryStats.textContent = t("simulation.outputDiscoveryLoading", {}, "Loading catalog");
    } else if (state.simulationOutputDiscoveryError) {
      elements.simulationOutputDiscoveryStats.textContent = t("simulation.outputDiscoveryError", {}, "Catalog unavailable");
    } else if (result) {
      elements.simulationOutputDiscoveryStats.textContent = t(
        "simulation.outputDiscoveryStats",
        { shown: filtered.length, total: items.length },
        `${filtered.length} of ${items.length} outputs`,
      );
    } else {
      elements.simulationOutputDiscoveryStats.textContent = t("simulation.outputDiscoveryNone", {}, "No catalog");
    }
  }
  if (!customSelected) {
    elements.simulationOutputDiscoveryList.innerHTML = "";
    return;
  }
  if (state.simulationOutputDiscoveryLoading) {
    elements.simulationOutputDiscoveryList.innerHTML = `<div class="empty status-loading">${escapeHTML(t("simulation.outputDiscoveryLoading", {}, "Loading catalog"))}</div>`;
    return;
  }
  if (state.simulationOutputDiscoveryError) {
    elements.simulationOutputDiscoveryList.innerHTML = `<div class="simulation-error">${escapeHTML(state.simulationOutputDiscoveryError)}</div>`;
    return;
  }
  if (!result) {
    elements.simulationOutputDiscoveryList.innerHTML = "";
    return;
  }
  elements.simulationOutputDiscoveryList.innerHTML =
    shown.map(({ item, index }) => renderSimulationOutputDiscoveryItem(item, index)).join("") ||
    `<div class="empty">${escapeHTML(t("simulation.outputDiscoveryEmpty", {}, "No matching outputs"))}</div>`;
}

function discoveryItemMatchesQuery(item, query) {
  if (!query) {
    return true;
  }
  const haystack = [
    item.objectType,
    item.keyValue,
    item.name,
    item.units,
    item.resourceType,
    item.endUseCategory,
    item.meterGroup,
    item.reportingFrequency,
    item.source,
    item.status,
    item.aliasOf,
    item.aliasReason,
    ...(item.purposeIds || []),
  ]
    .join(" ")
    .toLowerCase();
  return haystack.includes(query);
}

function renderSimulationOutputDiscoveryItem(item, index) {
  const isMeter = String(item.objectType || "").toLowerCase() === "output:meter";
  const title = isMeter ? item.name || item.keyValue || "" : item.name || "";
  const alias = item.aliasOf ? `alias: ${item.aliasOf}` : "";
  const meterParts = [item.resourceType, item.endUseCategory, item.meterGroup].filter(Boolean).join(" / ");
  const detail = isMeter ? [item.objectType || "Output:Meter", meterParts, alias].filter(Boolean).join(" / ") : [item.keyValue || "*", item.objectType || "Output:Variable", alias].filter(Boolean).join(" / ");
  const meta = [item.reportingFrequency, item.units, item.source, item.aliasReason].filter(Boolean).join(" - ");
  return `
    <div class="simulation-output-discovery-item">
      <span>
        <strong title="${escapeHTML(title)}">${escapeHTML(title)}</strong>
        <small title="${escapeHTML(detail)}">${escapeHTML(detail)}</small>
        <em>${escapeHTML(meta || item.status || "")}</em>
      </span>
      <b class="simulation-discovery-badge ${escapeHTML(item.status || "")}">${escapeHTML(item.status || "")}</b>
      <button type="button" data-simulation-discovery-add="${index}">${escapeHTML(t("action.add", {}, "Add"))}</button>
    </div>`;
}

function appendDiscoveredCustomOutput(index) {
  const item = state.simulationOutputDiscovery?.items?.[index];
  const line = customOutputLineFromDiscoveryItem(item);
  if (!line || !elements.simulationCustomOutputs) {
    return;
  }
  const lines = String(elements.simulationCustomOutputs.value || "")
    .split(/\r?\n/)
    .map((value) => value.trim())
    .filter(Boolean);
  const lineKey = line.toLowerCase();
  if (!lines.some((value) => value.toLowerCase() === lineKey)) {
    lines.push(line);
  }
  elements.simulationCustomOutputs.value = lines.join("\n");
  saveSimulationCustomOutputsPreset();
  scheduleSimulationRunPlan(0);
}

function restoreSimulationCustomOutputsPreset() {
  if (!elements.simulationCustomOutputs || String(elements.simulationCustomOutputs.value || "").trim()) {
    return;
  }
  try {
    const saved = window.localStorage.getItem(simulationCustomOutputsStorageKey);
    if (saved) {
      elements.simulationCustomOutputs.value = saved;
    }
  } catch {
    // localStorage can be unavailable in hardened webview settings.
  }
}

function saveSimulationCustomOutputsPreset() {
  if (!elements.simulationCustomOutputs) {
    return;
  }
  try {
    const value = normalizeCustomOutputPresetText(elements.simulationCustomOutputs.value || "");
    if (value) {
      window.localStorage.setItem(simulationCustomOutputsStorageKey, value);
    } else {
      window.localStorage.removeItem(simulationCustomOutputsStorageKey);
    }
  } catch {
    // localStorage can be unavailable in hardened webview settings.
  }
}

function normalizeCustomOutputPresetText(value) {
  return String(value || "")
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .join("\n");
}

function customOutputLineFromDiscoveryItem(item) {
  if (!item) {
    return "";
  }
  const objectType = String(item.objectType || "").trim();
  const frequency = item.reportingFrequency || "Hourly";
  if (objectType.toLowerCase() === "output:meter") {
    const meterName = item.aliasOf || item.name || item.keyValue || "";
    return meterName ? `Output:Meter | ${meterName} | ${frequency}` : "";
  }
  const variableName = item.aliasOf || item.name || "";
  return variableName ? `Output:Variable | ${item.keyValue || "*"} | ${variableName} | ${frequency}` : "";
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
  const mode = purposeOutputApplyMode();
  const hasApplicableOutputs = (plan?.outputObjects || []).some((object) => purposeOutputAppliesInMode(object, mode));
  elements.simulationApplyPurposeOutputs.disabled = state.simulationRunning || state.simulationPurposePlanLoading || !hasText || !plan || !hasApplicableOutputs;
  elements.simulationApplyPurposeOutputs.title = hasApplicableOutputs
    ? t("simulation.makePurposeOutputsPermanent", {}, "Make outputs permanent")
    : t("simulation.noTemporaryOutputs", {}, "No temporary purpose outputs need to be applied.");
}

function purposeOutputApplyMode() {
  return elements.simulationPurposeApplyMode?.value || "add_missing_only";
}

function purposeOutputAppliesInMode(object = {}, mode = purposeOutputApplyMode()) {
  const stateValue = object.state || "";
  switch (mode) {
    case "replace_conflicting":
      return stateValue === "temporary" || stateValue === "will_be_persisted" || stateValue === "conflict";
    case "keep_existing_and_add":
      return stateValue !== "existing";
    case "remove_purpose_outputs":
      return Number.isFinite(Number(object.objectIndex));
    default:
      return stateValue === "temporary" || stateValue === "will_be_persisted";
  }
}

function buildSimulationPurposeRequest() {
  const purposes = selectedSimulationPurposes();
  const zoneMode = elements.simulationPurposeZoneMode?.value || "all";
  return {
    purposes,
    scope: {
      zoneMode,
      zoneNames: simulationPurposeZoneNamesForMode(zoneMode),
      periodMode: elements.simulationPurposePeriodMode?.value || "full",
      periodStart: elements.simulationPurposePeriodStart?.value || "",
      periodEnd: elements.simulationPurposePeriodEnd?.value || "",
      ...simulationHVACPurposeScope(purposes),
      customOutputs: parseCustomOutputs(elements.simulationCustomOutputs?.value || ""),
    },
    frequencyPolicy: elements.simulationPurposeFrequencyPolicy?.value || "purpose_default",
    allocationPolicy: elements.simulationPurposeAllocationPolicy?.value || "direct_only",
    outputApplyMode: purposeOutputApplyMode(),
    sqlMode: "sql_first",
    persistOutputs: Boolean(elements.simulationPersistOutputs?.checked),
    discoveryAllowed: false,
  };
}

function simulationPurposeZoneNamesForMode(zoneMode) {
  const typed = parseCommaList(elements.simulationPurposeZoneNames?.value || "");
  switch (zoneMode) {
    case "selected":
      return typed;
    case "visible":
      return simulationVisibleZoneNames();
    case "filtered":
      return simulationFilteredZoneNames(typed);
    default:
      return [];
  }
}

function simulationVisibleZoneNames() {
  const geometry = state.report?.geometry;
  const zones = geometry?.zones || [];
  if (!zones.length) {
    return [];
  }
  if (state.selectedGeometryStory === "all" || state.selectedGeometryStory === "" || state.selectedGeometryStory === undefined) {
    return zones.map((zone) => zone.name).filter(Boolean);
  }
  return zones
    .filter((zone) => String(zone.storyIndex) === String(state.selectedGeometryStory))
    .map((zone) => zone.name)
    .filter(Boolean);
}

function simulationFilteredZoneNames(terms) {
  const zoneNames = simulationAllZoneNames();
  const filters = (terms || []).map(normalizeOutputMatchToken).filter(Boolean);
  if (filters.length) {
    return zoneNames.filter((zoneName) => filters.some((term) => normalizeOutputMatchToken(zoneName).includes(term)));
  }
  const activeNames = [state.activeProfileZoneName, selectedGeometryZoneName()].map((value) => String(value || "").trim()).filter(Boolean);
  return activeNames.length ? activeNames : [];
}

function simulationAllZoneNames() {
  const geometryZones = (state.report?.geometry?.zones || []).map((zone) => zone.name).filter(Boolean);
  if (geometryZones.length) {
    return [...new Set(geometryZones)].sort((a, b) => a.localeCompare(b));
  }
  return (state.model?.objects || [])
    .filter((object) => String(object.type || "").toLowerCase() === "zone")
    .map((object) => object.name || object.fields?.[0]?.value || "")
    .filter(Boolean)
    .sort((a, b) => a.localeCompare(b));
}

function selectedGeometryZoneName() {
  if (state.selectedGeometryKind !== "zone" || !state.selectedGeometryId) {
    return "";
  }
  const zone = (state.report?.geometry?.zones || []).find((item) => item.id === state.selectedGeometryId);
  return zone?.name || "";
}

function simulationHVACPurposeScope(purposes = selectedSimulationPurposes()) {
  if (!purposes.includes("hvac_loop_check")) {
    return {};
  }
  const loop = activeSimulationHVACLoop();
  if (!loop) {
    return { loopMode: "all" };
  }
  const scope = { loopMode: "selected" };
  const name = loop.name || "";
  switch (loop.type) {
    case "AirLoopHVAC":
      scope.airLoopNames = name ? [name] : [];
      break;
    case "PlantLoop":
      scope.plantLoopNames = name ? [name] : [];
      break;
    case "CondenserLoop":
      scope.condenserLoopNames = name ? [name] : [];
      break;
    default:
      scope.loopMode = "all";
      break;
  }
  const component = activeSimulationHVACComponent(loop);
  if (component) {
    scope.componentIds = simulationHVACComponentScopeIDs(component);
  }
  return scope;
}

function simulationHVACScopeSummary(purposes = selectedSimulationPurposes()) {
  if (!purposes.includes("hvac_loop_check")) {
    return "";
  }
  const loop = activeSimulationHVACLoop();
  const component = activeSimulationHVACComponent(loop);
  if (loop?.name && component) {
    return `HVAC: ${loop.name} / ${component.objectName || component.objectType || t("common.component", {}, "Component")}`;
  }
  return loop?.name ? `HVAC: ${loop.name}` : t("simulation.allHVACLoops", {}, "HVAC: all loops");
}

function activeSimulationHVACLoop() {
  const loops = state.report?.hvac?.loops || [];
  return loops.find((loop) => loop.id === state.activeHVACLoopId) || null;
}

function activeSimulationHVACComponent(loop = activeSimulationHVACLoop()) {
  const selectedKey = normalizeOutputMatchToken(state.activeHVACGraphKey || "");
  if (!loop || !selectedKey || selectedKey.startsWith("loop:") || selectedKey.startsWith("node:")) {
    return null;
  }
  return simulationHVACLoopComponents(loop).find((component) =>
    simulationHVACComponentScopeIDs(component).some((id) => normalizeOutputMatchToken(id) === selectedKey),
  ) || null;
}

function simulationHVACLoopComponents(loop = {}) {
  const sides = [loop.supplySide, loop.demandSide].filter(Boolean);
  return sides.flatMap((side) => (side.branches || []).flatMap((branch) => branch.components || []));
}

function simulationHVACComponentScopeIDs(component = {}) {
  const ids = [];
  const add = (value) => {
    const text = String(value || "").trim();
    if (text && !ids.includes(text)) {
      ids.push(text);
    }
  };
  const index = Number(component.objectIndex);
  if (Number.isFinite(index) && index >= 0) {
    for (const prefix of ["component", "source", "terminal"]) {
      add(`${prefix}:${index}`);
    }
  }
  const objectType = component.objectType || "";
  const objectName = component.objectName || "";
  if (objectType && objectName) {
    for (const prefix of ["component", "source", "terminal"]) {
      add(`${prefix}:${objectType}:${objectName}`);
    }
    add(`${objectType}:${objectName}`);
  }
  add(objectName);
  return ids;
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

function parseCustomOutputs(value) {
  const outputs = [];
  const seen = new Set();
  String(value || "")
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .forEach((line) => {
      const tokens = line
        .split(line.includes("|") ? "|" : ",")
        .map((token) => token.trim())
        .filter(Boolean);
      const output = customOutputFromTokens(tokens);
      if (!output) {
        return;
      }
      const key = [
        output.objectType,
        output.keyValue || "",
        output.variableName || "",
        output.meterName || "",
        output.reportingFrequency || "",
      ]
        .map((token) => token.toLowerCase())
        .join("|");
      if (!seen.has(key)) {
        seen.add(key);
        outputs.push(output);
      }
    });
  return outputs;
}

function customOutputFromTokens(tokens) {
  if (!tokens.length) {
    return null;
  }
  const first = tokens[0].toLowerCase();
  if (first === "output:meter" || first === "meter") {
    const meterName = tokens[1] || "";
    if (!meterName) {
      return null;
    }
    return {
      objectType: "Output:Meter",
      meterName,
      reportingFrequency: tokens[2] || "Hourly",
    };
  }
  if (first === "output:variable" || first === "variable") {
    const hasExplicitKey = tokens.length >= 4;
    const keyValue = hasExplicitKey ? tokens[1] : "*";
    const variableName = hasExplicitKey ? tokens[2] : tokens[1] || "";
    if (!variableName) {
      return null;
    }
    return {
      objectType: "Output:Variable",
      keyValue,
      variableName,
      reportingFrequency: hasExplicitKey ? tokens[3] || "Hourly" : tokens[2] || "Hourly",
    };
  }
  if (tokens.length >= 3) {
    return {
      objectType: "Output:Variable",
      keyValue: tokens[0] || "*",
      variableName: tokens[1],
      reportingFrequency: tokens[2] || "Hourly",
    };
  }
  if (tokens.length === 2) {
    return {
      objectType: "Output:Meter",
      meterName: tokens[0],
      reportingFrequency: tokens[1] || "Hourly",
    };
  }
  return null;
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
  if (elements.simulationPurposePeriodMode) {
    elements.simulationPurposePeriodMode.disabled = state.simulationRunning;
  }
  if (elements.simulationPurposeAllocationPolicy) {
    elements.simulationPurposeAllocationPolicy.disabled = state.simulationRunning;
  }
  const customPeriod = elements.simulationPurposePeriodMode?.value === "custom";
  for (const input of [elements.simulationPurposePeriodStart, elements.simulationPurposePeriodEnd]) {
    if (input) {
      input.disabled = state.simulationRunning || !customPeriod;
    }
  }
  updatePurposeExportButton();
  if (elements.simulationPurposeInputs?.length) {
    elements.simulationPurposeInputs.forEach((input) => {
      input.disabled = state.simulationRunning;
    });
  }
  updatePurposeApplyButton();
}

function updatePurposeExportButton() {
  const canExport = Boolean(state.simulationResult?.purposeResults);
  for (const button of [elements.simulationExportPurposeJSON, elements.simulationExportPurposeHTML]) {
    if (!button) {
      continue;
    }
    button.disabled = state.simulationRunning || !canExport;
    button.title = canExport
      ? button.textContent || t("action.exportPurposeJson", {}, "Export Purpose JSON")
      : t("simulation.noPurposeResultExport", {}, "No purpose results to export yet.");
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
  const integrity = result.purposeResults?.integrity || {};
  const staticDiagnostics = integrity.staticDiagnostics || (!stale ? state.report?.diagnostics || [] : []);
  const crossChecks = integrity.crossChecks || [];
  const sqlIssues = integrity.sqlIssues || [];
  const tabularReports = integrity.tabularReports || [];
  const staleBadge = stale ? `<span class="simulation-badge stale">${escapeHTML(t("simulation.stale", {}, "Stale"))}</span>` : "";
  const statusBadge = `<span class="simulation-badge ${escapeHTML(result.status || "unknown")}">${escapeHTML(statusText(result.status))}</span>`;
  const issueSummary = simulationIssueSummary(err.issues || []);
  const issueRows = errIssueGroups(err.issues || [])
    .slice(0, 16)
    .map(
      (group) => `
        <tr>
          <td><span class="simulation-severity ${escapeHTML(group.severity)}">${escapeHTML(group.severity)}</span></td>
          <td>
            <details ${group.count === 1 ? "open" : ""}>
              <summary>${escapeHTML(group.message)}</summary>
              <small>${escapeHTML(t("common.line", {}, "Line"))}: ${escapeHTML(group.lines.join(", "))}</small>
            </details>
          </td>
          <td>${escapeHTML(group.count)}</td>
          <td>${escapeHTML(group.lines[0] || "")}</td>
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
      <div><span>${escapeHTML(t("simulation.staticDiagnostics", {}, "Static diagnostics"))}</span><strong>${escapeHTML(staticDiagnostics.length)}</strong></div>
      <div><span>${escapeHTML(t("simulation.integrityCrossChecks", {}, "Cross checks"))}</span><strong>${escapeHTML(crossChecks.length)}</strong></div>
      <div><span>${escapeHTML(t("simulation.sqlDiagnostics", {}, "SQL diagnostics"))}</span><strong>${escapeHTML(sqlIssues.length)}</strong></div>
      <div><span>${escapeHTML(t("simulation.tabularReports", {}, "Tabular reports"))}</span><strong>${escapeHTML(tabularReports.length)}</strong></div>
    </div>
    <div class="simulation-issue-summary">
      ${issueSummary.map((item) => `<span class="${escapeHTML(item.key)}">${escapeHTML(item.label)} ${escapeHTML(item.count)}</span>`).join("")}
    </div>
    ${renderSimulationResultSourceSummary(result)}
    ${result.error ? `<div class="simulation-error">${escapeHTML(result.error)}</div>` : ""}
    <div class="simulation-tables">
      <section>
        <h4>${escapeHTML(t("simulation.errIssues", {}, "ERR issues"))}</h4>
        <div class="output-table-wrap">
          <table class="output-table">
            <thead><tr><th>${escapeHTML(t("common.type", {}, "Type"))}</th><th>${escapeHTML(t("common.message", {}, "Message"))}</th><th>${escapeHTML(t("common.count", {}, "Count"))}</th><th>${escapeHTML(t("common.firstLine", {}, "First line"))}</th></tr></thead>
            <tbody>${issueRows || `<tr><td colspan="4">${escapeHTML(t("simulation.noErrIssues", {}, "No ERR warnings or errors parsed."))}</td></tr>`}</tbody>
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
    </div>
    ${renderIntegrityStaticDiagnostics(staticDiagnostics)}
    ${renderIntegrityCrossChecks(crossChecks)}
    ${renderIntegritySQLDetails(sqlIssues, tabularReports)}`;
}

function errIssueGroups(issues = []) {
  const groups = new Map();
  for (const issue of issues) {
    const severity = issue.severity || "info";
    const message = issue.message || "";
    const key = `${severity}|${normalizeOutputMatchToken(message)}`;
    const group = groups.get(key) || { severity, message, count: 0, lines: [] };
    group.count += 1;
    if (issue.line) {
      group.lines.push(issue.line);
    }
    groups.set(key, group);
  }
  return [...groups.values()].sort((left, right) => {
    const severityDelta = errSeverityRank(right.severity) - errSeverityRank(left.severity);
    if (severityDelta !== 0) {
      return severityDelta;
    }
    if (left.count !== right.count) {
      return right.count - left.count;
    }
    return (left.lines[0] || 0) - (right.lines[0] || 0);
  });
}

function errSeverityRank(severity) {
  switch (String(severity || "").toLowerCase()) {
    case "fatal":
      return 3;
    case "severe":
      return 2;
    case "warning":
      return 1;
    default:
      return 0;
  }
}

function renderIntegritySQLDetails(sqlIssues = [], tabularReports = []) {
  if (!sqlIssues.length && !tabularReports.length) {
    return "";
  }
  const query = normalizeOutputMatchToken(state.simulationIntegrityQuery);
  const filteredSQLIssues = query ? sqlIssues.filter((issue) => integritySQLIssueMatches(issue, query)) : sqlIssues;
  const filteredReports = query
    ? tabularReports.map((report) => filterIntegrityTabularReport(report, query)).filter(Boolean)
    : tabularReports;
  const sqlRows = filteredSQLIssues
    .slice(0, 24)
    .map(
      (issue) => `
        <tr>
          <td><span class="simulation-severity ${escapeHTML(issue.severity || "info")}">${escapeHTML(issue.severity || "info")}</span></td>
          <td>${escapeHTML(issue.message || "")}</td>
          <td>${escapeHTML(issue.count || "")}</td>
          <td>${escapeHTML(issue.source || "")}</td>
        </tr>`,
    )
    .join("");
  const issueSection = `
    <section>
      <h4>${escapeHTML(integrityFilteredTitle(t("simulation.sqlDiagnostics", {}, "SQL diagnostics"), filteredSQLIssues.length, sqlIssues.length, query))}</h4>
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>${escapeHTML(t("common.type", {}, "Type"))}</th><th>${escapeHTML(t("common.message", {}, "Message"))}</th><th>${escapeHTML(t("common.count", {}, "Count"))}</th><th>${escapeHTML(t("common.source", {}, "Source"))}</th></tr></thead>
          <tbody>${sqlRows || `<tr><td colspan="4">${escapeHTML(t("simulation.noSQLDiagnostics", {}, "No SQL diagnostics rows were found."))}</td></tr>`}</tbody>
        </table>
      </div>
    </section>`;
  const reportSections = filteredReports.length
    ? `<div class="simulation-tabular-reports">${filteredReports.slice(0, 6).map(renderIntegrityTabularReport).join("")}</div>`
    : `<div class="empty">${escapeHTML(t("simulation.noTabularReports", {}, "No SQL tabular reports were found."))}</div>`;
  return `
    <div class="simulation-integrity-sql">
      ${issueSection}
      <section>
        <h4>${escapeHTML(integrityFilteredTitle(t("simulation.tabularReports", {}, "Tabular reports"), filteredReports.length, tabularReports.length, query))}</h4>
        ${reportSections}
      </section>
    </div>`;
}

function renderIntegrityStaticDiagnostics(diagnostics = []) {
  if (!diagnostics.length) {
    return "";
  }
  const query = normalizeOutputMatchToken(state.simulationIntegrityQuery);
  const filtered = query ? diagnostics.filter((diagnostic) => integrityStaticDiagnosticMatches(diagnostic, query)) : diagnostics;
  const rows = filtered
    .slice(0, 32)
    .map(
      (diagnostic) => `
        <tr>
          <td><span class="simulation-severity ${escapeHTML(diagnostic.severity || "notice")}">${escapeHTML(diagnostic.severity || "notice")}</span></td>
          <td>${escapeHTML(diagnostic.category || "Static Diagnose")}</td>
          <td>${escapeHTML(diagnostic.message || "")}</td>
          <td>${escapeHTML(staticDiagnosticLocation(diagnostic))}</td>
          <td>${escapeHTML(diagnostic.code || "")}</td>
          <td>${escapeHTML(simulationDiagnosticSourceLabel(diagnostic.source || ""))}</td>
        </tr>`,
    )
    .join("");
  return `
    <div class="simulation-integrity-static">
      <section>
        <h4>${escapeHTML(integrityFilteredTitle(t("simulation.staticDiagnostics", {}, "Static diagnostics"), filtered.length, diagnostics.length, query))}</h4>
        <div class="output-table-wrap">
          <table class="output-table">
            <thead><tr><th>${escapeHTML(t("common.type", {}, "Type"))}</th><th>${escapeHTML(t("common.category", {}, "Category"))}</th><th>${escapeHTML(t("common.message", {}, "Message"))}</th><th>${escapeHTML(t("common.location", {}, "Location"))}</th><th>${escapeHTML(t("common.code", {}, "Code"))}</th><th>${escapeHTML(t("common.source", {}, "Source"))}</th></tr></thead>
            <tbody>${rows || `<tr><td colspan="6">${escapeHTML(t("diagnose.noMatching", {}, "No matching diagnostics"))}</td></tr>`}</tbody>
          </table>
        </div>
      </section>
    </div>`;
}

function renderIntegrityCrossChecks(crossChecks = []) {
  if (!crossChecks.length) {
    return "";
  }
  const query = normalizeOutputMatchToken(state.simulationIntegrityQuery);
  const filtered = query ? crossChecks.filter((item) => integrityCrossCheckMatches(item, query)) : crossChecks;
  const rows = filtered
    .slice(0, 48)
    .map((item) => {
      const sqlLocation = [item.sqlReport, item.sqlTable].filter(Boolean).join(" / ");
      const values = Object.entries(item.values || {})
        .slice(0, 4)
        .map(([key, value]) => `${key}: ${value}`)
        .join("; ");
      return `
        <tr>
          <td>${escapeHTML(simulationDiagnosticSourceLabel(item.category || ""))}</td>
          <td>${escapeHTML(item.name || "")}</td>
          <td><span class="simulation-crosscheck-status ${escapeHTML(item.status || "info")}">${escapeHTML(simulationIntegrityCrossCheckStatusLabel(item.status || "info"))}</span></td>
          <td>${escapeHTML(item.staticSource || "")}</td>
          <td>${escapeHTML(sqlLocation || item.sqlSource || "")}</td>
          <td>${escapeHTML(item.message || "")}</td>
          <td>${escapeHTML(values)}</td>
        </tr>`;
    })
    .join("");
  return `
    <div class="simulation-integrity-crosscheck">
      <section>
        <h4>${escapeHTML(integrityFilteredTitle(t("simulation.integrityCrossChecks", {}, "Cross checks"), filtered.length, crossChecks.length, query))}</h4>
        <div class="output-table-wrap">
          <table class="output-table">
            <thead><tr><th>${escapeHTML(t("common.category", {}, "Category"))}</th><th>${escapeHTML(t("common.name", {}, "Name"))}</th><th>${escapeHTML(t("common.status", {}, "Status"))}</th><th>${escapeHTML(t("simulation.staticSource", {}, "Static source"))}</th><th>${escapeHTML(t("simulation.sqlSource", {}, "SQL source"))}</th><th>${escapeHTML(t("common.message", {}, "Message"))}</th><th>${escapeHTML(t("common.values", {}, "Values"))}</th></tr></thead>
            <tbody>${rows || `<tr><td colspan="7">${escapeHTML(t("simulation.noIntegrityCrossChecks", {}, "No matching cross checks"))}</td></tr>`}</tbody>
          </table>
        </div>
      </section>
    </div>`;
}

function integrityFilteredTitle(title, shown, total, query) {
  return query ? `${title} (${shown}/${total})` : title;
}

function integrityStaticDiagnosticMatches(diagnostic, query) {
  return normalizeOutputMatchToken([
    diagnostic.severity,
    diagnostic.category,
    diagnostic.message,
    diagnostic.objectType,
    diagnostic.objectName,
    diagnostic.field,
    diagnostic.value,
    diagnostic.code,
    diagnostic.source,
    diagnostic.evidence,
  ].filter(Boolean).join(" ")).includes(query);
}

function staticDiagnosticLocation(diagnostic) {
  return [diagnostic.objectType, diagnostic.objectName, diagnostic.field, diagnostic.value].filter((value) => String(value || "").trim() !== "").join(" / ");
}

function simulationDiagnosticSourceLabel(source) {
  return String(source || "")
    .replaceAll("_", " ")
    .replace(/\b\w/g, (match) => match.toUpperCase());
}

function integritySQLIssueMatches(issue, query) {
  return normalizeOutputMatchToken([issue.severity, issue.message, issue.count, issue.source].filter(Boolean).join(" ")).includes(query);
}

function integrityCrossCheckMatches(item, query) {
  return normalizeOutputMatchToken([
    item.category,
    item.name,
    item.status,
    item.staticSource,
    item.sqlSource,
    item.sqlReport,
    item.sqlTable,
    item.message,
    ...Object.keys(item.values || {}),
    ...Object.values(item.values || {}),
  ].filter(Boolean).join(" ")).includes(query);
}

function simulationIntegrityCrossCheckStatusLabel(status) {
  switch (String(status || "").toLowerCase()) {
    case "exact":
      return t("simulation.crossCheckExact", {}, "Exact");
    case "normalized":
      return t("simulation.crossCheckNormalized", {}, "Normalized");
    case "alias":
      return t("simulation.crossCheckAlias", {}, "Alias");
    case "static_only":
      return t("simulation.crossCheckStaticOnly", {}, "Static only");
    case "sql_only":
      return t("simulation.crossCheckSQLOnly", {}, "SQL only");
    default:
      return simulationDiagnosticSourceLabel(status || "info");
  }
}

function filterIntegrityTabularReport(report, query) {
  const reportText = normalizeOutputMatchToken([report.reportName, report.for, report.tableName, report.source, ...(report.columns || [])].filter(Boolean).join(" "));
  if (reportText.includes(query)) {
    return report;
  }
  const rows = (report.rows || []).filter((row) => integrityTabularRowMatches(row, query));
  if (!rows.length) {
    return null;
  }
  return { ...report, rows };
}

function integrityTabularRowMatches(row, query) {
  return normalizeOutputMatchToken([row.name, ...Object.values(row.values || {})].filter(Boolean).join(" ")).includes(query);
}

function renderIntegrityTabularReport(report) {
  const columns = (report.columns || []).slice(0, 6);
  const rows = (report.rows || []).slice(0, 10);
  const header = columns.map((column) => `<th>${escapeHTML(column)}</th>`).join("");
  const body = rows
    .map((row) => {
      const cells = columns.map((column) => `<td>${escapeHTML(row.values?.[column] || "")}</td>`).join("");
      return `<tr><td>${escapeHTML(row.name || "")}</td>${cells}</tr>`;
    })
    .join("");
  const subtitle = [report.reportName, report.for, report.source].filter(Boolean).join(" - ");
  return `
    <article class="simulation-tabular-report">
      <h5>${escapeHTML(report.tableName || t("simulation.tabularReport", {}, "Tabular report"))}</h5>
      <span>${escapeHTML(subtitle)}</span>
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>${escapeHTML(t("common.row", {}, "Row"))}</th>${header}</tr></thead>
          <tbody>${body || `<tr><td colspan="${columns.length + 1}">${escapeHTML(t("common.notAvailable", {}, "N/A"))}</td></tr>`}</tbody>
        </table>
      </div>
    </article>`;
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

function renderSimulationResultSourceSummary(result) {
  const priority = result.resultSourcePriority || [];
  const sources = result.resultSources || [];
  if (!priority.length && !sources.length) {
    return "";
  }
  return `
    <div class="simulation-result-sources">
      ${priority.length ? `<span>${escapeHTML(t("simulation.resultSourcePriority", {}, "Source priority"))}: ${escapeHTML(priority.join(" -> "))}</span>` : ""}
      ${sources.length ? `<span>${escapeHTML(t("simulation.resultSourcesUsed", {}, "Used sources"))}: ${escapeHTML(sources.join(", "))}</span>` : ""}
    </div>`;
}

function renderSimulationCustomSeriesLinks(result) {
  if (!elements.simulationCustomSeries) {
    return;
  }
  const customOutputs = activePurposeOutputObjects().filter((object) => {
    const purposeIDs = object.purposeIds || [];
    return purposeIDs.includes("custom_outputs") && purposeObjectIsSeries(object.objectType);
  });
  if (!result || !customOutputs.length) {
    elements.simulationCustomSeries.innerHTML = "";
    return;
  }
  const rows = customOutputs
    .slice(0, 32)
    .map((object) => {
      const ref = customOutputSeriesRef(object);
      return `
        <tr>
          <td>${escapeHTML(object.objectType || "")}</td>
          <td>${escapeHTML(customOutputSeriesLabel(object))}</td>
          <td>${escapeHTML(object.reportingFrequency || "")}</td>
          <td>${renderSeriesInspectButton(ref)}</td>
        </tr>`;
    })
    .join("");
  elements.simulationCustomSeries.innerHTML = `
    <section class="simulation-custom-series-links">
      <h4>${escapeHTML(t("simulation.customOutputs", {}, "Custom outputs"))}</h4>
      <div class="output-table-wrap">
        <table class="output-table">
          <thead><tr><th>${escapeHTML(t("common.type", {}, "Type"))}</th><th>${escapeHTML(t("simulation.sourceOutput", {}, "Source output"))}</th><th>${escapeHTML(t("hvac.reportingFrequency", {}, "Reporting frequency"))}</th><th>${escapeHTML(t("simulation.inspectSeriesAction", {}, "Chart"))}</th></tr></thead>
          <tbody>${rows}</tbody>
        </table>
      </div>
    </section>`;
}

function customOutputSeriesRef(object) {
  if (outputObjectIsMeter(object.objectType)) {
    const meterName = object.keyValue || object.meterName || customOutputFieldValue(object, "Key Name");
    return { series: findSimulationSeriesForMeter(meterName), keyValue: meterName, variableName: meterName };
  }
  return {
    series: findSimulationSeriesForMetric(object.keyValue || "*", object.variableName),
    keyValue: object.keyValue || "*",
    variableName: object.variableName || "",
  };
}

function customOutputSeriesLabel(object) {
  if (outputObjectIsMeter(object.objectType)) {
    return object.keyValue || object.meterName || customOutputFieldValue(object, "Key Name") || "";
  }
  return [object.keyValue || "*", object.variableName || customOutputFieldValue(object, "Variable Name")].filter(Boolean).join(" / ");
}

function customOutputFieldValue(object, fieldName) {
  const field = (object.fields || []).find((item) => normalizeOutputMatchToken(item.name) === normalizeOutputMatchToken(fieldName));
  return field?.value || "";
}

function purposeObjectIsSeries(objectType) {
  const normalized = normalizeOutputMatchToken(objectType);
  return normalized === "output:variable" || outputObjectIsMeter(objectType);
}

function outputObjectIsMeter(objectType) {
  return normalizeOutputMatchToken(objectType).startsWith("output:meter");
}

function renderSimulationSeriesSelect(result) {
  const series = safeSimulationSeriesList(result?.series);
  const groupOptions = simulationSeriesGroupOptions(series);
  const selectedGroup = groupOptions.some((option) => option.value === state.simulationSeriesGroup) ? state.simulationSeriesGroup : "all";
  state.simulationSeriesGroup = selectedGroup;
  if (elements.simulationSeriesGroup) {
    elements.simulationSeriesGroup.disabled = !series.length;
    elements.simulationSeriesGroup.innerHTML = groupOptions
      .map((option) => `<option value="${escapeHTML(option.value)}" ${option.value === selectedGroup ? "selected" : ""}>${escapeHTML(option.label)}</option>`)
      .join("");
  }
  if (!series.length) {
    state.simulationSelectedSeries = "";
    elements.simulationSeriesSelect.innerHTML = `<option value="">${escapeHTML(t("simulation.noSeries", {}, "No SQL/CSV series"))}</option>`;
    renderSimulationSeriesRangeControls(null);
    if (elements.simulationSeriesStats) {
      elements.simulationSeriesStats.textContent = t("simulation.noSeries", {}, "No SQL/CSV series");
    }
    return;
  }
  const visibleSeries = selectedGroup === "all" ? series : series.filter((item) => simulationSeriesGroupID(item) === selectedGroup);
  if (!visibleSeries.length) {
    state.simulationSelectedSeries = "";
    elements.simulationSeriesSelect.innerHTML = `<option value="">${escapeHTML(t("simulation.noSeriesInGroup", {}, "No series in this group"))}</option>`;
    renderSimulationSeriesRangeControls(null);
    if (elements.simulationSeriesStats) {
      elements.simulationSeriesStats.textContent = t("simulation.seriesGroupStats", { shown: 0, total: series.length }, `0 of ${series.length} SQL/CSV series`);
    }
    return;
  }
  if (!state.simulationSelectedSeries || !visibleSeries.some((item) => seriesID(item) === state.simulationSelectedSeries)) {
    state.simulationSelectedSeries = seriesID(preferredSimulationSeries(visibleSeries) || visibleSeries[0]);
    state.simulationSeriesRangeStart = 0;
    state.simulationSeriesRangeEnd = -1;
  }
  elements.simulationSeriesSelect.innerHTML = visibleSeries
    .map((item) => {
      const id = seriesID(item);
      return `<option value="${escapeHTML(id)}" ${id === state.simulationSelectedSeries ? "selected" : ""}>${escapeHTML(item.file)} - ${escapeHTML(item.column)}</option>`;
    })
    .join("");
  if (elements.simulationSeriesStats) {
    elements.simulationSeriesStats.textContent = selectedGroup === "all"
      ? t("simulation.seriesStats", { count: series.length }, `${series.length} SQL/CSV series`)
      : t("simulation.seriesGroupStats", { shown: visibleSeries.length, total: series.length }, `${visibleSeries.length} of ${series.length} SQL/CSV series`);
  }
}

function setSimulationSeriesGroupUnavailable() {
  if (!elements.simulationSeriesGroup) {
    return;
  }
  state.simulationSeriesGroup = "all";
  elements.simulationSeriesGroup.disabled = true;
  elements.simulationSeriesGroup.innerHTML = `<option value="all">${escapeHTML(t("simulation.seriesAllGroups", {}, "All groups"))}</option>`;
}

function simulationSeriesGroupOptions(series) {
  const options = simulationSeriesGroupDefinitions();
  const groups = new Set((series || []).map(simulationSeriesGroupID));
  return options.filter((option) => option.value === "all" || groups.has(option.value));
}

function simulationSeriesGroupDefinitions() {
  return [
    { value: "all", label: t("simulation.seriesAllGroups", {}, "All groups") },
    { value: "temperature", label: t("simulation.seriesGroupTemperature", {}, "Temperature") },
    { value: "mass_flow", label: t("simulation.seriesGroupMassFlow", {}, "Mass flow") },
    { value: "setpoint", label: t("simulation.seriesGroupSetpoint", {}, "Setpoint") },
    { value: "psychrometric", label: t("simulation.seriesGroupPsychrometric", {}, "Humidity / enthalpy") },
    { value: "rate_load", label: t("simulation.seriesGroupRateLoad", {}, "Rate / load") },
    { value: "power_energy", label: t("simulation.seriesGroupPowerEnergy", {}, "Power / energy") },
    { value: "other", label: t("simulation.seriesGroupOther", {}, "Other") },
  ];
}

function simulationSeriesGroupID(series) {
  return simulationVariableGroupID(seriesVariableName(series?.column || ""));
}

function simulationVariableGroupID(variableName) {
  const name = normalizeOutputMatchToken(variableName);
  if (name.includes("setpoint")) {
    return "setpoint";
  }
  if (name.includes("temperature") || name.includes("drybulb")) {
    return "temperature";
  }
  if (name.includes("mass flow") || name.includes("flow rate") || name.includes("flowrate")) {
    return "mass_flow";
  }
  if (name.includes("humidity") || name.includes("enthalpy")) {
    return "psychrometric";
  }
  if (name.includes("rate") || name.includes("load") || name.includes("runtime fraction")) {
    return "rate_load";
  }
  if (name.includes("power") || name.includes("energy") || name.includes("electricity") || name.includes(":")) {
    return "power_energy";
  }
  return "other";
}

function simulationSeriesGroupLabel(groupID) {
  return simulationSeriesGroupDefinitions().find((option) => option.value === groupID)?.label || groupID;
}

function renderSimulationChart() {
  const result = state.simulationResult;
  const series = currentSimulationSeries();
  const seriesPoints = simulationSeriesPoints(series);
  if (!series || !seriesPoints.length) {
    renderSimulationSeriesRangeControls(null);
    elements.simulationChart.innerHTML = `<div class="empty">${t("simulation.noGraph", {}, "SQL/CSV graph will appear after a run with numeric output.")}</div>`;
    return;
  }
  const visibleRange = normalizeSimulationSeriesRange(seriesPoints.length);
  renderSimulationSeriesRangeControls(series, visibleRange);
  const visiblePoints = seriesPoints.slice(visibleRange.start, visibleRange.end + 1);
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
      return `<g><line x1="${pad.left}" x2="${width - pad.right}" y1="${y}" y2="${y}" class="simulation-grid" /><text x="8" y="${y + 4}" class="simulation-axis">${escapeHTML(formatValueWithUnit(value, simulationSeriesDisplayUnit(series)))}</text></g>`;
    })
    .join("");
  const firstLabel = visiblePoints[0]?.label || "start";
  const lastLabel = visiblePoints[visiblePoints.length - 1]?.label || "end";
  const displayColumn = simulationSeriesDisplayColumn(series);
  const title = visiblePoints.length === seriesPoints.length
    ? displayColumn
    : `${displayColumn} (${visibleRange.start + 1}-${visibleRange.end + 1} / ${seriesPoints.length})`;
  elements.simulationChart.innerHTML = `
    <svg class="simulation-svg" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(displayColumn)}">
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

function currentSimulationSeries() {
  const result = state.simulationResult;
  return safeSimulationSeriesList(result?.series).find((item) => seriesID(item) === state.simulationSelectedSeries) || null;
}

function renderSimulationSeriesRangeControls(series, visibleRange = null) {
  if (!elements.simulationSeriesRangeStart || !elements.simulationSeriesRangeEnd || !elements.simulationSeriesRangeLabel) {
    return;
  }
  const pointCount = simulationSeriesPointCount(series);
  const disabled = pointCount <= 1;
  const maxIndex = Math.max(0, pointCount - 1);
  const range = pointCount > 0 ? (visibleRange || normalizeSimulationSeriesRange(pointCount)) : { start: 0, end: 0 };
  for (const input of [elements.simulationSeriesRangeStart, elements.simulationSeriesRangeEnd]) {
    input.min = "0";
    input.max = String(maxIndex);
    input.disabled = disabled;
  }
  elements.simulationSeriesRangeStart.value = String(range.start);
  elements.simulationSeriesRangeEnd.value = String(range.end);
  if (elements.simulationSeriesRangeAll) {
    elements.simulationSeriesRangeAll.disabled = pointCount <= 0 || (range.start === 0 && range.end === maxIndex);
  }
  elements.simulationSeriesRangeLabel.value = pointCount > 0
    ? seriesRangeLabel(series, range)
    : t("simulation.seriesRangeEmpty", {}, "No range");
}

function seriesRangeLabel(series, range) {
  const points = simulationSeriesPoints(series);
  const pointCount = points.length;
  if (!pointCount) {
    return t("simulation.seriesRangeEmpty", {}, "No range");
  }
  const startPoint = points[range.start] || {};
  const endPoint = points[range.end] || {};
  const labels = [startPoint.label, endPoint.label].filter(Boolean);
  const indexLabel = `${range.start + 1}-${range.end + 1} / ${pointCount}`;
  return labels.length === 2 ? `${indexLabel} (${labels[0]} - ${labels[1]})` : indexLabel;
}

function updateSimulationSeriesRangeFromControls(changed) {
  const series = currentSimulationSeries();
  if (!simulationSeriesPointCount(series)) {
    renderSimulationSeriesRangeControls(null);
    return;
  }
  state.simulationSeriesRangeStart = Number(elements.simulationSeriesRangeStart?.value) || 0;
  state.simulationSeriesRangeEnd = Number(elements.simulationSeriesRangeEnd?.value) || 0;
  normalizeSimulationSeriesRange(simulationSeriesPointCount(series), changed);
  renderSimulationChart();
}

function normalizeSimulationSeriesRange(pointCount = 0, changed = "") {
  const maxIndex = Math.max(0, Number(pointCount) - 1);
  let start = Math.round(clampNumber(state.simulationSeriesRangeStart, 0, maxIndex));
  let end = Number(state.simulationSeriesRangeEnd);
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
  const pointCount = simulationSeriesPointCount(series);
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
  const dataset = activeHeatFlowDataset();
  if (!dataset?.zones?.length || !dataset?.categories?.length || !(dataset.frameCount > 0)) {
    renderSimulationHeatFlowEmpty(t("simulation.noHeatFlow", {}, "Select Zone Heat Flow to inspect the zone heat-flow ledger."));
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
    ${renderPurposeCompletenessRow(dataset.completeness || [])}
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

function activeHeatFlowDataset() {
  const purposeDataset = state.simulationResult?.purposeResults?.zoneHeatFlow;
  return purposeDataset?.zones?.length ? purposeDataset : state.simulationResult?.heatFlow;
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
  state.simulationHeatFlowZoomStart = 0;
  state.simulationHeatFlowZoomEnd = -1;
  state.simulationHeatFlowVisibleFrameIndexes = [];
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
  state.simulationHeatFlowZoomStart = start;
  state.simulationHeatFlowZoomEnd = end;
  state.simulationHeatFlowVisibleFrameIndexes = heatFlowFrameIndexes(start, end);
  state.simulationHeatFlowFrameIndex = clampNumber(state.simulationHeatFlowFrameIndex, start, end);
  return { start, end };
}

function heatFlowFrameIndexes(start, end) {
  const indexes = [];
  for (let index = start; index <= end; index += 1) {
    indexes.push(index);
  }
  return indexes;
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
  const dataset = activeHeatFlowDataset();
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

function exportPurposeResultJSON() {
  const result = state.simulationResult;
  if (!result?.purposeResults) {
    return;
  }
  const payload = purposeResultExportPayload(result);
  const blob = new Blob([`${JSON.stringify(payload, null, 2)}\n`], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = purposeResultExportFilename(result, "json");
  link.click();
  URL.revokeObjectURL(url);
  setStatus(t("status.purposeResultsExported", {}, "Purpose result JSON exported"), "ok");
}

function exportPurposeResultHTML() {
  const result = state.simulationResult;
  if (!result?.purposeResults) {
    return;
  }
  const payload = purposeResultExportPayload(result);
  const blob = new Blob([purposeResultHTML(payload)], { type: "text/html" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = purposeResultExportFilename(result, "html");
  link.click();
  URL.revokeObjectURL(url);
  setStatus(t("status.purposeHTMLExported", {}, "Purpose result HTML exported"), "ok");
}

function purposeResultExportPayload(result) {
  return {
    runId: result.runId || "",
    status: result.status || "",
    filename: result.filename || "",
    inputPath: result.inputPath || "",
    weatherPath: result.weatherPath || "",
    energyPlusExecutablePath: result.energyPlusExecutablePath || "",
    outputDirectory: result.outputDirectory || "",
    startedAt: result.startedAt || "",
    finishedAt: result.finishedAt || "",
    durationMs: result.durationMs || 0,
    resultSourcePriority: result.resultSourcePriority || [],
    resultSources: result.resultSources || [],
    purposeRunPlan: result.purposeRunPlan || null,
    purposeResults: result.purposeResults,
    files: result.files || [],
  };
}

function purposeResultExportFilename(result, extension) {
  const base = String(result.filename || result.runId || "purpose-results").replace(/\.[^.]+$/, "");
  return `${sanitizeExportFilename(base)}-purpose-results.${extension}`;
}

function sanitizeExportFilename(value) {
  const safe = String(value || "")
    .replace(/[<>:"/\\|?*\x00-\x1f]+/g, "-")
    .replace(/\s+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 80);
  return safe || "purpose-results";
}

function purposeResultHTML(payload) {
  const planObjects = payload.purposeRunPlan?.outputObjects || [];
  const completeness = payload.purposeResults?.completeness || [];
  const files = payload.files || [];
  const summaryRows = [
    ["Run ID", payload.runId],
    ["Status", payload.status],
    ["Input", payload.filename || payload.inputPath],
    ["Weather", payload.weatherPath],
    ["Output directory", payload.outputDirectory],
    ["Result source priority", (payload.resultSourcePriority || []).join(" -> ")],
    ["Result sources used", (payload.resultSources || []).join(", ")],
    ["Started", payload.startedAt],
    ["Finished", payload.finishedAt],
  ];
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>${escapeHTML(payload.filename || "Purpose Simulation Results")}</title>
<style>
body{margin:0;background:#f6f8fb;color:#17202a;font:14px/1.45 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
main{max-width:1180px;margin:0 auto;padding:24px}
h1{margin:0 0 6px;font-size:26px} h2{margin:24px 0 10px;font-size:17px}
.muted{color:#667085} table{width:100%;border-collapse:collapse;background:white;border:1px solid #d8dee8}
th,td{padding:8px 10px;border-bottom:1px solid #e5e9f0;text-align:left;vertical-align:top}
th{background:#eef3f8;font-size:12px;text-transform:uppercase;letter-spacing:0}
pre{max-height:520px;overflow:auto;background:#0f172a;color:#e2e8f0;padding:14px;border-radius:6px}
</style>
</head>
<body>
<main>
<h1>Purpose Simulation Results</h1>
<p class="muted">${escapeHTML(payload.filename || payload.runId || "")}</p>
<h2>Run</h2>
${renderPurposeHTMLTable(["Field", "Value"], summaryRows)}
<h2>Output Plan</h2>
${renderPurposeHTMLTable(
  ["Type", "Key", "Metric", "Frequency", "Purpose", "State"],
  planObjects.map((object) => [
    object.objectType || "",
    object.keyValue || "",
    object.variableName || "",
    object.reportingFrequency || "",
    (object.purposeIds || []).join(", "),
    object.state || "",
  ]),
)}
<h2>Completeness</h2>
${renderPurposeHTMLTable(
  ["Purpose", "Required Output", "Found", "Source"],
  completeness.map((item) => [item.purposeId || "", item.requiredOutput || "", item.found ? "Yes" : "No", item.source || ""]),
)}
${renderPurposeHTMLResultSections(payload.purposeResults || {})}
<h2>Files</h2>
${renderPurposeHTMLTable(
  ["Name", "Type", "Path"],
  files.map((file) => [file.name || "", file.kind || "", file.path || ""]),
)}
<h2>Raw Bundle</h2>
<pre>${escapeHTML(JSON.stringify(payload, null, 2))}</pre>
</main>
</body>
</html>`;
}

function renderPurposeHTMLResultSections(results) {
  return [
    renderPurposeHTMLEnergy(results.energy || {}),
    renderPurposeHTMLEnergyExplanation(results.energyExplanationSummary || {}, results.energyExplanation || {}),
    renderPurposeHTMLHeatFlow(results.zoneHeatFlow || {}),
    renderPurposeHTMLHVAC(results.hvacLoops || []),
    renderPurposeHTMLIntegrity(results.integrity || {}),
    renderPurposeHTMLComfort(results.comfort || {}),
  ]
    .filter(Boolean)
    .join("\n");
}

function renderPurposeHTMLEnergy(energy) {
  const totals = energy.totals || [];
  const rows = totals.length
    ? totals.map((item) => [item.name || "", item.unit || "", formatNumber(item.value), item.source || ""])
    : [...(energy.facilityMonthly || []), ...(energy.endUseMonthly || []), ...(energy.zoneMonthly || [])].map((item) => [
        item.name || [item.zoneName, item.metric].filter(Boolean).join(" / "),
        item.unit || "",
        formatNumber(item.total),
        item.source || "",
      ]);
  if (!rows.length) {
    return "";
  }
  return `<h2>Energy Results</h2>${renderPurposeHTMLTable(["Metric", "Unit", "Total", "Source"], rows.slice(0, 120))}`;
}

function renderPurposeHTMLEnergyExplanation(summary = {}, explanation = {}) {
  const sections = [];
  const completeness = summary.completeness || explanation.completeness || {};
  if (summary.schema || explanation.schema || completeness.status) {
    sections.push(
      `<h2>Energy Explanation Completeness</h2>${renderPurposeHTMLTable(
        ["Field", "Value"],
        [
          ["Schema", summary.schema || explanation.schema || ""],
          ["Period", summary.period || "annual"],
          ["Allocation policy", summary.allocationPolicy || explanation.allocationPolicy || ""],
          ["Status", completeness.status || ""],
          ["Mapped energy", Number.isFinite(Number(completeness.mappedPercent)) ? `${formatNumber(completeness.mappedPercent)}%` : ""],
          ["Missing categories", (completeness.missingCategories || []).join(", ")],
        ],
      )}`,
    );
  }
  const availabilityRows = (completeness.sourceAvailability || [])
    .slice(0, 240)
    .map((item) => [item.level || "", item.name || "", item.status || ""]);
  if (availabilityRows.length) {
    sections.push(`<h2>Energy Explanation Source Availability</h2>${renderPurposeHTMLTable(["Level", "Output", "Status"], availabilityRows)}`);
  }
  const ruleRows = (explanation.relationshipRules || [])
    .slice(0, 120)
    .map((rule) => [
      rule.id || "",
      [rule.fromLevel, rule.toLevel].filter(Boolean).join(" -> "),
      [rule.fromKind, rule.toKind].filter(Boolean).join(" -> "),
      rule.basis || "",
      (rule.requiredSource || []).join(", "),
      rule.formula || "",
    ]);
  if (ruleRows.length) {
    sections.push(
      `<h2>Energy Explanation Relationship Rules</h2>${renderPurposeHTMLTable(["Rule", "Level flow", "Kind flow", "Basis", "Required source", "Formula"], ruleRows)}`,
    );
  }
  const summaryRows = purposeHTMLEnergySummaryRows(summary);
  if (summaryRows.length) {
    sections.push(
      `<h2>Energy Explanation Summary</h2>${renderPurposeHTMLTable(
        ["Type", "Metric", "Label", "Value", "Unit", "Level", "Service", "Path", "Basis", "Source IDs"],
        summaryRows.slice(0, 180),
      )}`,
    );
  }
  const monthlyRows = purposeHTMLEnergyMonthlyRows(explanation);
  if (monthlyRows.length) {
    sections.push(`<h2>Energy Explanation Monthly Ledger</h2>${renderPurposeHTMLTable(["Period", "Energy Use", "Delivered Load", "Heat Drivers", "Residual"], monthlyRows)}`);
  }
  const graph = purposeHTMLAnnualEnergyGraph(explanation);
  const nodeLabels = new Map((graph.nodes || []).map((node) => [node.id, node.label || node.kind || node.id || ""]));
  const edgeRows = (graph.edges || [])
    .slice(0, 180)
    .map((edge) => [
      edge.relation || "",
      edge.basis || "",
      edge.ruleId || "",
      nodeLabels.get(edge.fromId) || edge.fromId || "",
      nodeLabels.get(edge.toId) || edge.toId || "",
      formatValueWithUnit(edge.value, edge.unit),
      (edge.sourceIds || []).join(", "),
      (edge.relatedPathIds || []).join(", "),
      edge.formula || "",
    ]);
  if (edgeRows.length) {
    sections.push(
      `<h2>Energy Explanation Annual Edges</h2>${renderPurposeHTMLTable(
        ["Relation", "Basis", "Rule", "From", "To", "Value", "Source IDs", "Related Paths", "Formula"],
        edgeRows,
      )}`,
    );
  }
  const warningRows = [...(explanation.warnings || []), ...(graph.warnings || [])]
    .slice(0, 120)
    .map((warning) => [warning.severity || "", warning.code || "", warning.period || graph.id || "", warning.message || ""]);
  if (warningRows.length) {
    sections.push(`<h2>Energy Explanation Warnings</h2>${renderPurposeHTMLTable(["Severity", "Code", "Period", "Message"], warningRows)}`);
  }
  const reconciliationRows = (graph.reconciliation || [])
    .slice(0, 180)
    .map((row) => [
      row.label || row.id || "",
      row.period || graph.id || "annual",
      row.level || "",
      row.zoneName || "",
      row.serviceKind || "",
      formatValueWithUnit(row.expectedValue, row.unit),
      formatValueWithUnit(row.explainedValue, row.unit),
      formatValueWithUnit(row.residualValue, row.unit),
      row.basis || "",
      (row.sourceIds || []).join(", "),
      row.formula || "",
    ]);
  if (reconciliationRows.length) {
    sections.push(
      `<h2>Energy Explanation Reconciliation</h2>${renderPurposeHTMLTable(
        ["Metric", "Period", "Level", "Zone", "Service", "Expected", "Mapped", "Residual", "Basis", "Source IDs", "Formula"],
        reconciliationRows,
      )}`,
    );
  }
  const sourceRows = (explanation.sources || [])
    .slice(0, 180)
    .map((source) => [
      source.id || "",
      source.sourceType || "",
      source.isMeter ? "meter" : "variable",
      source.keyValue || "",
      source.name || "",
      source.reportingFrequency || "",
      source.aggregationMethod || "",
      source.sourceUnit || source.units || "",
      source.normalizedUnit || "",
      source.objectIndex ?? "",
    ]);
  if (sourceRows.length) {
    sections.push(
      `<h2>Energy Explanation Sources</h2>${renderPurposeHTMLTable(
        ["ID", "Source", "Basis", "Key", "Name", "Frequency", "Aggregation", "Source Unit", "Normalized Unit", "Output Object"],
        sourceRows,
      )}`,
    );
  }
  return sections.join("\n");
}

function purposeHTMLEnergySummaryRows(summary = {}) {
  const groups = [
    ["Energy by carrier", summary.energyByCarrier || []],
    ["Energy by end use", summary.energyByEndUse || []],
    ["Delivered load", summary.deliveredLoadByService || []],
    ["Derived KPI", summary.derivedKpis || []],
    ["Heat drivers", summary.heatDrivers || []],
    ["Residuals", summary.residuals || []],
    ["Top heat drivers", summary.topHeatDrivers || []],
    ["Top zones", summary.topZones || []],
  ];
  return groups.flatMap(([group, items]) =>
    (items || []).map((item) => [
      group,
      item.id || "",
      item.label || "",
      formatValueWithUnit(item.value, item.unit),
      item.unit || "",
      item.level || "",
      item.serviceKind || "",
      item.pathType || "",
      item.basis || "",
      (item.sourceIds || []).join(", "),
    ]),
  );
}

function purposeHTMLEnergyMonthlyRows(explanation = {}) {
  return (explanation.periods || [])
    .filter((period) => period.kind === "monthly")
    .map((period) => {
      const totals = energyExplanationLevelTotals(period.nodes || []);
      return [
        period.label || period.id || "",
        formatValueWithUnit(totals.energy, totals.unit),
        formatValueWithUnit(totals.load, totals.unit),
        formatValueWithUnit(totals.heat, totals.unit),
        formatValueWithUnit(totals.residual, totals.unit),
      ];
    });
}

function purposeHTMLAnnualEnergyGraph(explanation = {}) {
  return (
    (explanation.periods || []).find((period) => period.id === "annual" || period.kind === "annual") || {
      id: "annual",
      kind: "annual",
      nodes: explanation.nodes || [],
      edges: explanation.edges || [],
      reconciliation: explanation.reconciliation || [],
    }
  );
}

function renderPurposeHTMLHeatFlow(heatFlow) {
  const zones = heatFlow.zones || [];
  if (!zones.length) {
    return "";
  }
  const rows = zones.map((zone) => [
    zone.name || "",
    heatFlow.frameCount || "",
    formatValueWithUnit(maxAbsNestedValues(zone.values || []), heatFlow.unit || "W"),
    formatValueWithUnit(maxNumber(zone.temperature || []), heatFlow.temperatureUnit || "C"),
  ]);
  return `<h2>Zone Heat Flow Results</h2>${renderPurposeHTMLTable(["Zone", "Frames", "Peak abs heat flow", "Max temperature"], rows.slice(0, 120))}`;
}

function renderPurposeHTMLHVAC(loops) {
  if (!loops.length) {
    return "";
  }
  const nodeRows = loops
    .flatMap((loop) =>
      (loop.nodeSummaries || []).map((node) => [
        loop.name || "",
        node.nodeName || "",
        formatValueWithUnit(node.temperatureAverage, node.temperatureUnit || "C"),
        formatValueWithUnit(node.massFlowMax, node.massFlowUnit || "kg/s"),
        formatValueWithUnit(node.temperatureSetpointDelta, node.temperatureUnit || "C"),
        node.source || "",
      ]),
    )
    .slice(0, 160);
  const componentRows = loops
    .flatMap((loop) =>
      (loop.components || []).flatMap((component) =>
        (component.metrics || []).map((metric) => [
          loop.name || "",
          component.componentName || "",
          component.componentType || "",
          metric.name || "",
          formatValueWithUnit(metric.max, metric.unit),
          formatValueWithUnit(metric.total, metric.unit),
          metric.source || component.source || "",
        ]),
      ),
    )
    .slice(0, 160);
  const statusRows = loops.map((loop) => [
    loop.name || "",
    simulationHVACStatusLabel(loop.status || "unknown"),
    loop.statusMessage || "",
    loop.loopType || "",
  ]);
  const derivedRows = loops
    .flatMap((loop) =>
      (loop.derivedMetrics || []).map((metric) => [
        loop.name || "",
        metric.name || "",
        formatValueWithUnit(metric.value, metric.unit),
        simulationHVACMetricSourceLabel(metric.source || ""),
        metric.status || "",
        metric.message || "",
      ]),
    )
    .slice(0, 120);
  return [
    statusRows.length ? `<h2>HVAC Loop Status</h2>${renderPurposeHTMLTable(["Loop", "Status", "Message", "Type"], statusRows)}` : "",
    derivedRows.length
      ? `<h2>HVAC Derived Metrics</h2>${renderPurposeHTMLTable(["Loop", "Metric", "Value", "Source", "Status", "Message"], derivedRows)}`
      : "",
    nodeRows.length ? `<h2>HVAC Node Results</h2>${renderPurposeHTMLTable(["Loop", "Node", "Avg temp", "Peak flow", "Avg delta", "Source"], nodeRows)}` : "",
    componentRows.length
      ? `<h2>HVAC Component Results</h2>${renderPurposeHTMLTable(["Loop", "Component", "Type", "Metric", "Peak", "Total", "Source"], componentRows)}`
      : "",
  ]
    .filter(Boolean)
    .join("\n");
}

function renderPurposeHTMLIntegrity(integrity) {
  const err = integrity.err || {};
  const errRows = errIssueGroups(err.issues || [])
    .slice(0, 120)
    .map((group) => [group.severity || "", group.message || "", group.count || 0, (group.lines || []).join(", ")]);
  const sqlRows = (integrity.sqlIssues || [])
    .slice(0, 120)
    .map((issue) => [issue.severity || "", issue.message || "", issue.count || "", issue.source || ""]);
  const tabularRows = (integrity.tabularReports || [])
    .flatMap((report) =>
      (report.rows || []).slice(0, 40).map((row) => [
        report.reportName || "",
        report.tableName || "",
        row.name || row.rowName || "",
        Object.entries(row.values || {})
          .slice(0, 4)
          .map(([key, value]) => `${key}: ${value}`)
          .join("; "),
        report.source || "",
      ]),
    )
    .slice(0, 160);
  const staticRows = (integrity.staticDiagnostics || [])
    .slice(0, 120)
    .map((diagnostic) => [
      diagnostic.severity || "",
      diagnostic.category || "",
      diagnostic.message || "",
      staticDiagnosticLocation(diagnostic),
      diagnostic.code || "",
      simulationDiagnosticSourceLabel(diagnostic.source || ""),
    ]);
  const crossCheckRows = (integrity.crossChecks || [])
    .slice(0, 160)
    .map((item) => [
      simulationDiagnosticSourceLabel(item.category || ""),
      item.name || "",
      simulationIntegrityCrossCheckStatusLabel(item.status || "info"),
      item.staticSource || "",
      [item.sqlReport, item.sqlTable].filter(Boolean).join(" / ") || item.sqlSource || "",
      item.message || "",
      Object.entries(item.values || {})
        .slice(0, 4)
        .map(([key, value]) => `${key}: ${value}`)
        .join("; "),
    ]);
  if (!staticRows.length && !crossCheckRows.length && !errRows.length && !sqlRows.length && !tabularRows.length && !err.total && !(integrity.tabularReports || []).length) {
    return "";
  }
  const summaryRows = [
    ["Status", integrity.status || ""],
    ["ERR completed", err.completed ? "Yes" : "No"],
    ["ERR warnings", err.warnings || 0],
    ["ERR severe/fatal", (err.severe || 0) + (err.fatal || 0)],
    ["Static diagnostics", (integrity.staticDiagnostics || []).length],
    ["Cross checks", (integrity.crossChecks || []).length],
    ["SQL diagnostics", (integrity.sqlIssues || []).length],
    ["Tabular reports", (integrity.tabularReports || []).length],
  ];
  return [
    `<h2>Integrity Summary</h2>${renderPurposeHTMLTable(["Field", "Value"], summaryRows)}`,
    staticRows.length
      ? `<h2>Integrity Static Diagnostics</h2>${renderPurposeHTMLTable(["Severity", "Category", "Message", "Location", "Code", "Source"], staticRows)}`
      : "",
    crossCheckRows.length
      ? `<h2>Integrity Cross Checks</h2>${renderPurposeHTMLTable(["Category", "Name", "Status", "Static Source", "SQL Source", "Message", "Values"], crossCheckRows)}`
      : "",
    errRows.length ? `<h2>Integrity ERR Issues</h2>${renderPurposeHTMLTable(["Severity", "Message", "Count", "Lines"], errRows)}` : "",
    sqlRows.length
      ? `<h2>Integrity SQL Diagnostics</h2>${renderPurposeHTMLTable(["Severity", "Message", "Count", "Source"], sqlRows)}`
      : "",
    tabularRows.length
      ? `<h2>Integrity Tabular Reports</h2>${renderPurposeHTMLTable(["Report", "Table", "Row", "Values", "Source"], tabularRows)}`
      : "",
  ]
    .filter(Boolean)
    .join("\n");
}

function renderPurposeHTMLComfort(comfort) {
  const unmetRows = (comfort.unmetHours || [])
    .map((item) => [
      item.zoneName || "",
      item.metric || "",
      formatValueWithUnit(item.value || 0, item.unit || ""),
      item.report || "",
      item.table || "",
      item.source || "",
    ])
    .slice(0, 80);
  const issueRows = (comfort.issues || [])
    .map((issue) => [
      issue.zoneName || "",
      issue.unmetSamples || 0,
      issue.heatingSamples || 0,
      issue.coolingSamples || 0,
      formatValueWithUnit(issue.maxDeviation || 0, issue.unit || ""),
      formatValueWithUnit(issue.averageDeviation || 0, issue.unit || ""),
      issue.peakLabel || "",
      issue.source || "",
    ])
    .slice(0, 80);
  const rows = (comfort.zones || [])
    .flatMap((zone) =>
      (zone.metrics || []).map((metric) => [
        zone.zoneName || "",
        metric.name || "",
        formatValueWithUnit(metric.min, metric.unit),
        formatValueWithUnit(metric.max, metric.unit),
        formatValueWithUnit(metric.average, metric.unit),
        metric.source || "",
      ]),
    )
    .slice(0, 160);
  if (!rows.length && !issueRows.length && !unmetRows.length) {
    return "";
  }
  return [
    comfort.periodScope ? `<h2>Comfort Period Scope</h2>${renderPurposeHTMLTable(["Field", "Value"], [["Period", comfort.periodScope]])}` : "",
    unmetRows.length ? `<h2>Comfort Unmet Hours</h2>${renderPurposeHTMLTable(["Zone", "Metric", "Value", "Report", "Table", "Source"], unmetRows)}` : "",
    issueRows.length
      ? `<h2>Comfort Issue Ranking</h2>${renderPurposeHTMLTable(["Zone", "Unmet samples", "Heating", "Cooling", "Max deviation", "Avg deviation", "Time", "Source"], issueRows)}`
      : "",
    rows.length ? `<h2>Comfort Results</h2>${renderPurposeHTMLTable(["Zone", "Metric", "Min", "Max", "Avg", "Source"], rows)}` : "",
  ]
    .filter(Boolean)
    .join("\n");
}

function maxAbsNestedValues(rows) {
  let peak = 0;
  for (const row of rows || []) {
    for (const value of row || []) {
      const number = Math.abs(Number(value));
      if (Number.isFinite(number)) {
        peak = Math.max(peak, number);
      }
    }
  }
  return peak;
}

function maxNumber(values) {
  let maximum = Number.NEGATIVE_INFINITY;
  for (const value of values || []) {
    const number = Number(value);
    if (Number.isFinite(number)) {
      maximum = Math.max(maximum, number);
    }
  }
  return Number.isFinite(maximum) ? maximum : NaN;
}

function renderPurposeHTMLTable(headers, rows) {
  const body = rows.length
    ? rows
        .map((row) => `<tr>${row.map((cell) => `<td>${escapeHTML(String(cell ?? ""))}</td>`).join("")}</tr>`)
        .join("")
    : `<tr><td colspan="${headers.length}"><span class="muted">No data</span></td></tr>`;
  return `<table><thead><tr>${headers.map((header) => `<th>${escapeHTML(header)}</th>`).join("")}</tr></thead><tbody>${body}</tbody></table>`;
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
    state.simulationHeatFlowZoomStart = 0;
    state.simulationHeatFlowZoomEnd = -1;
    state.simulationHeatFlowVisibleFrameIndexes = [];
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
  series = safeSimulationSeries(series);
  return `${series.file || ""}::${series.column || ""}`;
}

function preferredSimulationSeries(series = []) {
  const items = Array.isArray(series) ? series.filter(Boolean) : [];
  const preferred = [
    "Electricity:Facility",
    "NaturalGas:Facility",
    "DistrictCooling:Facility",
    "DistrictHeating:Facility",
    "Water:Facility",
  ];
  return preferred
    .map((name) => items.find((item) => String(item.column || "").includes(name)))
    .find(Boolean) || null;
}

function seriesNodeKey(column) {
  const [key] = String(column || "").split(":");
  return key?.trim() || "";
}

function seriesColumnMainName(column) {
  return String(column || "").replace(/\s*\[[^\]]+\]\s*$/, "").trim();
}

function seriesVariableName(column) {
  const value = String(column || "");
  const variable = value.includes(":") ? value.slice(value.indexOf(":") + 1) : value;
  return variable.replace(/\s*\[[^\]]+\]\s*$/, "").trim();
}

function seriesColumnUnit(column) {
  const match = String(column || "").match(/\[([^\]]+)\]\s*$/);
  return match ? match[1].trim() : "";
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
