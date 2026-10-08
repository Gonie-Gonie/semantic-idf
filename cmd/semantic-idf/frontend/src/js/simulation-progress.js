import { getLanguage, t } from "./i18n.js";

const phaseKeys = {
  prepare: "simulation.preparing", discovery: "simulation.phaseDiscovery", plan: "simulation.phasePlan",
  apply_temporary_outputs: "simulation.phaseApplyTemporaryOutputs", execute: "simulation.running",
  parse_sql: "simulation.phaseReadSQL", sql_series: "simulation.phaseSQLSeries", sql_heat_flow: "simulation.phaseSQLHeatFlow",
  parse_fallback: "simulation.phaseReadFallback", build_purpose_results: "simulation.phasePurposeResults",
  energy_geometry: "simulation.phaseEnergyGeometry", energy_dashboard: "simulation.phaseEnergyDashboard",
  energy_drivers: "simulation.phaseEnergyDrivers", energy_service_paths: "simulation.phaseEnergyServicePaths",
  energy_path: "simulation.phaseEnergyPath", zone_heat_flow: "simulation.phaseZoneHeatFlow",
  thermal_topology: "simulation.phaseThermalTopology", receiving_results: "simulation.receivingResults",
  decode_results: "simulation.decodingResults", render_results: "simulation.preparingResults",
  hvac_loops: "simulation.phaseHVACLoops", comfort: "simulation.phaseComfort", integrity: "simulation.phaseIntegrity",
};
const workUnitKeys = { rows: "simulation.progressUnit.rows", series: "simulation.progressUnit.series", files: "simulation.progressUnit.files", frames: "simulation.progressUnit.frames" };
const number = (value) => new Intl.NumberFormat(getLanguage(), { maximumFractionDigits: 0 }).format(value);
const finite = (value) => typeof value === "number" && Number.isFinite(value) && value >= 0;
const escapeHTML = (value) => String(value || "").replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");

function byteLabel(value) {
  const units = ["B", "KiB", "MiB", "GiB"];
  let amount = value, unit = 0;
  while (amount >= 1024 && unit < units.length - 1) { amount /= 1024; unit++; }
  return `${new Intl.NumberFormat(getLanguage(), { maximumFractionDigits: unit ? 1 : 0 }).format(amount)} ${units[unit]}`;
}

function durationLabel(milliseconds, estimate = false) {
  const seconds = Math.max(1, Math.ceil(milliseconds / 1000));
  if (seconds < 60) return t("simulation.progressSeconds", { count: estimate ? Math.ceil(seconds / 5) * 5 : seconds }, "{count}s");
  if (seconds < 3600) return t("simulation.progressMinutes", { count: Math.ceil(seconds / 60) }, "{count}min");
  return t("simulation.progressHours", { count: new Intl.NumberFormat(getLanguage(), { maximumFractionDigits: 1 }).format(seconds / 3600) }, "{count}h");
}

// Legacy percent/completed/total describe stages in Single runs. They are
// deliberately excluded from numeric progress and duration prediction here.
export function simulationProgressPresentation(progress = {}, options = {}) {
  const pendingComplete = options.pending && progress.phase === "complete";
  const phase = pendingComplete ? "receiving_results" : String(progress.phase || "");
  const age = Math.max(0, options.ageMs || 0);
  const elapsedMs = options.elapsedMs;
  const installed = !options.pending && phase === "complete" && options.installed;
  const failed = ["request_failed", "display_failed"].includes(phase) || ["failed", "cancelled", "canceled"].includes(progress.status);
  const label = phaseKeys[phase] ? t(phaseKeys[phase], {}, progress.message || "")
    : installed ? options.completionLabel || progress.message || ""
      : phase === "display_failed" ? t("simulation.resultDisplayFailed", { message: progress.displayError || progress.message || "" }, "Results received, but display failed: {message}")
        : progress.message || options.statusLabel || "";
  const hasWork = !pendingComplete && finite(progress.workCompleted) && Boolean(progress.workUnit);
  const knownWork = hasWork && finite(progress.workTotal) && progress.workTotal > 0 && progress.workCompleted <= progress.workTotal && progress.progressKind === "work";
  const low = finite(progress.remainingLowMs) ? Math.max(0, progress.remainingLowMs - age) : null;
  const high = finite(progress.remainingHighMs) ? progress.remainingHighMs - age : null;
  const predictionExpired = finite(progress.remainingMs) && progress.remainingMs > 0 && age >= progress.remainingMs;
  const usableEstimate = !pendingComplete && !failed && !installed && !predictionExpired && high > 0 && low !== null && high >= low && progress.estimateSamples > 0;
  const estimatedOverall = usableEstimate && finite(progress.overallPercent) && progress.overallPercent <= 100;
  const mode = installed && !failed ? "complete" : failed ? "indeterminate" : knownWork ? "work" : estimatedOverall ? "estimated" : "indeterminate";
  const percent = mode === "complete" ? 100 : mode === "work" ? Math.min(100, progress.workCompleted / progress.workTotal * 100)
    : mode === "estimated" ? Math.min(95, progress.overallPercent) : null;
  const percentText = mode === "work" ? t("simulation.progressWorkPercent", { percent: number(Math.round(percent)) }, "{percent}% of this step")
    : mode === "estimated" ? `≈${number(Math.round(percent))}%` : mode === "complete" ? "100%" : failed ? "—" : "…";
  const count = (value) => progress.workUnit === "bytes" ? byteLabel(value) : number(value);
  const unit = progress.workUnit === "bytes" ? "" : workUnitKeys[progress.workUnit] ? t(workUnitKeys[progress.workUnit], {}, progress.workUnit) : progress.workUnit || "";
  const workText = (hasWork ? knownWork
    ? t("simulation.progressWorkKnown", { completed: count(progress.workCompleted), total: count(progress.workTotal), unit }, "{completed} / {total} {unit}")
    : t("simulation.progressWorkRead", { completed: count(progress.workCompleted), unit }, "{completed} {unit} processed") : "").trim();
  const elapsedSeconds = finite(elapsedMs) ? Math.floor(elapsedMs / 1000) : null;
  const elapsed = elapsedSeconds === null ? "" : t("simulation.progressElapsed", { seconds: number(elapsedSeconds) }, "Elapsed {seconds}s");
  let eta = "";
  if (usableEstimate) {
    const range = low > 0 ? `${durationLabel(low, true)}–${durationLabel(high, true)}`
      : t("simulation.progressUnder", { duration: durationLabel(high, true) }, "under {duration}");
    eta = t("simulation.progressRemainingRange", { range }, "Estimated processing remaining: {range}");
  }
  return {
    phase, mode, percent, percentText, elapsedSeconds, workText, eta,
    statusText: [label, phase === "execute" && /warmup|warming|simulat.*(?:day|date)|\d{2}\/\d{2}/i.test(progress.message || "") ? progress.message : "", workText, elapsed, eta].filter(Boolean).join(" · "),
    title: [progress.message || "", usableEstimate ? t("simulation.progressEstimateSamples", { count: progress.estimateSamples }, "Based on {count} completed runs of the same input in this session. Transfer and display follow processing.") : ""].filter(Boolean).join("\n"),
  };
}

export function createSimulationProgressPoller({ isCurrent, onProgress }) {
  let timer = 0, pending = null, version = 0;
  function stop() {
    version++;
    if (timer) window.clearTimeout(timer);
    timer = 0; pending?.abort(); pending = null;
  }
  function schedule(runID, expectedVersion) {
    timer = window.setTimeout(async () => {
      timer = 0;
      if (version !== expectedVersion || !isCurrent(runID)) return;
      const abort = new AbortController(); pending = abort;
      let retry = true;
      try {
        const response = await fetch(`/api/simulation/progress?runId=${encodeURIComponent(runID)}`, { signal: abort.signal });
        if (response.status === 404 || response.status === 405) retry = false;
        else if (response.ok) {
          const progress = await response.json();
          if (version === expectedVersion && isCurrent(runID)) onProgress(progress);
        }
      } catch { /* The run response and native events own user-visible errors. */ }
      finally {
        if (pending === abort) pending = null;
        if (retry && version === expectedVersion && isCurrent(runID)) schedule(runID, expectedVersion);
      }
    }, 750);
  }
  return { stop, start(runID) { stop(); schedule(runID, version); } };
}

export function createSimulationProgress({ state, elements, getPendingRunID, getCompletionLabel = () => "", getStatusLabel = () => "", onProgress = () => {} }) {
  let clock = null, transfer = null, sequence = null;
  const current = (runID) => state.simulationRunning && state.simulationActiveRunID === runID && getPendingRunID() === runID;
  const poller = createSimulationProgressPoller({ isCurrent: current, onProgress: (progress) => accept(progress) });
  const effective = () => transfer ? { ...state.simulationProgress, ...transfer } : state.simulationProgress || {};
  const presentation = () => {
    const progress = effective(), ageMs = clock?.sampleAt === undefined ? 0 : performance.now() - clock.sampleAt;
    const elapsedMs = clock && clock.runId === progress.runId
      ? Math.max(performance.now() - clock.startedAt, finite(clock.elapsedMs) ? clock.elapsedMs + ageMs : 0) : undefined;
    return simulationProgressPresentation(progress, {
      pending: getPendingRunID() === progress.runId,
      installed: state.simulationResult?.runId === progress.runId,
      completionLabel: getCompletionLabel(), statusLabel: getStatusLabel(progress.status), elapsedMs, ageMs,
    });
  };

  function render() {
    if (!elements.simulationProgressBar || !elements.simulationStatus) return;
    const model = presentation(), bar = elements.simulationProgressBar, status = elements.simulationStatus;
    bar.style.width = model.percent === null ? "" : `${model.percent}%`;
    bar.classList.toggle("indeterminate", model.mode === "indeterminate" && Boolean(state.simulationRunning));
    bar.setAttribute("role", "progressbar"); bar.setAttribute("aria-valuemin", "0"); bar.setAttribute("aria-valuemax", "100");
    bar.setAttribute("aria-label", model.statusText);
    if (model.percent === null) { bar.removeAttribute("aria-valuenow"); bar.removeAttribute("aria-valuetext"); }
    else { bar.setAttribute("aria-valuenow", String(Math.round(model.percent))); bar.setAttribute("aria-valuetext", [model.percentText, model.eta].filter(Boolean).join(" · ")); }
    elements.simulationPercent.textContent = model.percentText;
    status.textContent = model.statusText; status.title = model.title;
    status.dataset.simulationProgressPhase = model.phase;
    status.dataset.simulationProgressElapsed = model.elapsedSeconds === null ? "" : String(model.elapsedSeconds);
    status.dataset.simulationProgressMode = model.mode;
    const running = Boolean(state.simulationRunning || effective().status === "running");
    status.classList.toggle("status-loading", running);
    bar.closest(".simulation-progress-card")?.classList.toggle("running", running);
    const mini = elements.simulationChart?.querySelector(".simulation-running-empty .simulation-mini-progress");
    if (mini) {
      mini.dataset.progressMode = model.mode;
      mini.querySelector(".simulation-mini-line")?.setAttribute("x2", String(18 + 184 * (model.percent === null ? .3 : model.percent / 100)));
      mini.querySelectorAll(".simulation-mini-node").forEach((node, index) => node.classList.toggle("active", index === 0 || model.percent !== null && model.percent >= index * 50));
    }
  }

  function stop(runID = "") {
    if (runID && clock?.runId !== runID) return;
    if (clock?.timer) window.clearInterval(clock.timer);
    clock = null; transfer = null; sequence = null; poller.stop();
  }

  function refreshClock() {
    if (!clock) return;
    if (clock.timer) window.clearInterval(clock.timer); clock.timer = 0;
    if (!current(clock.runId)) { stop(clock.runId); return; }
    if (document.hidden) return;
    render();
    clock.timer = window.setInterval(() => {
      if (!clock || !current(clock.runId)) { stop(); return; }
      // Only this small progress card changes on a clock tick.
      render();
    }, 1000);
  }

  function start(runID, { http = false } = {}) {
    stop(); clock = { runId: runID, startedAt: performance.now(), timer: 0 };
    refreshClock(); if (http) poller.start(runID);
  }

  function accept(payload) {
    const progress = Array.isArray(payload) ? payload[0] : payload;
    if (!progress || !current(progress.runId)) return false;
    if (finite(progress.sequence)) {
      if (sequence !== null && progress.sequence <= sequence) return false;
      sequence = progress.sequence;
    } else if (sequence !== null) return false;
    if (clock?.runId === progress.runId) {
      clock.sampleAt = performance.now();
      clock.elapsedMs = finite(progress.elapsedMs) ? progress.elapsedMs : undefined;
    }
    state.simulationProgress = progress; render(); onProgress(); return true;
  }

  function receive(runID, progress) {
    if (!current(runID)) return;
    transfer = { ...progress, runId: runID, status: "running", message: "", overallPercent: undefined,
      remainingMs: undefined, remainingLowMs: undefined, remainingHighMs: undefined, estimateSamples: 0, estimateBasis: "" };
    render();
  }

  function miniSVG() {
    const model = presentation(), end = 18 + 184 * (model.percent === null ? .3 : model.percent / 100);
    return `<svg class="simulation-mini-progress" data-progress-mode="${model.mode}" viewBox="0 0 220 72" role="img" aria-label="${escapeHTML(t("simulation.runningShort", {}, "Running"))}"><line x1="18" y1="38" x2="202" y2="38" class="simulation-mini-track"/><line x1="18" y1="38" x2="${end}" y2="38" class="simulation-mini-line"/>${[18,110,202].map((x, index) => `<circle cx="${x}" cy="38" r="5" class="simulation-mini-node ${index === 0 || model.percent !== null && model.percent >= index * 50 ? "active" : ""}"/>`).join("")}</svg>`;
  }
  return { start, stop, accept, receive, render, refreshClock, miniSVG, presentation };
}

// Count the actual decoded response bytes without cloning a potentially large
// stream. A compressed Content-Length is not a valid total for these bytes.
export async function readSimulationJSONResponse(response, onProgress) {
  if (!onProgress || !response.body?.getReader) return response.json();
  const encoding = response.headers.get("Content-Encoding"), length = Number(response.headers.get("Content-Length"));
  const total = (!encoding || encoding === "identity") && Number.isFinite(length) && length > 0 ? length : 0;
  const reader = response.body.getReader(), decoder = new TextDecoder(), parts = [];
  let completed = 0, notified = -Infinity;
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      completed += value.byteLength; parts.push(decoder.decode(value, { stream: true }));
      if (performance.now() - notified >= 200) {
        const knownTotal = total >= completed ? total : 0;
        onProgress({ phase: "receiving_results", progressKind: knownTotal ? "work" : "indeterminate", workCompleted: completed, workTotal: knownTotal, workUnit: "bytes" });
        notified = performance.now();
      }
    }
    parts.push(decoder.decode());
    onProgress({ phase: "decode_results", progressKind: "indeterminate", workCompleted: completed, workTotal: 0, workUnit: "bytes" });
    await new Promise(resolve => window.setTimeout(resolve, 0));
    const text = parts.join(""); parts.length = 0;
    return JSON.parse(text);
  } finally { reader.releaseLock(); }
}
