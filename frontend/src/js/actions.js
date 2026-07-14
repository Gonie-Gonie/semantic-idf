import { backend, elements, refreshStatusTitle, setStatus, state, updateTextStats } from "./state.js";
import {
  markAllAnalysisDirty,
  markAnalysisDirty,
  renderDiagnostics,
  renderEmpty,
  renderReport,
  renderResultTab,
  updateResultTabReadiness,
} from "./views/analysis-views.js";
import { preloadGeometryRenderer, renderGeometry } from "./geometry-loader.js";
import { t } from "./i18n.js";

export const currentDocumentStorageKey = "idfAnalyzer.currentDocument";

const workspaceSnapshotVersion = 2;

let autoAnalyzeTimer = 0;
let afterPaintAnalyzeTimer = 0;
let analysisRunID = 0;
let activeAnalysisPromise = null;
let activeAnalysisText = "";
const maxFrontendStageConcurrency = 2;
let idlePreRenderTimer = 0;
let activeStageQueue = null;

export async function analyze(options = {}) {
  const api = backend();
  updateTextStats();
  updateDocumentActions();
  if (!api) {
    setStatus(t("status.backendUnavailable"), "warn");
    renderEmpty();
    return;
  }

  const text = elements.idfInput.value;
  if (activeAnalysisPromise && activeAnalysisText === text) {
    return activeAnalysisPromise;
  }
  const analysisKey = options.analysisKey || (await computeAnalysisKey(text));
  state.analysisKey = analysisKey;

  clearScheduledAnalyze();
  const runID = ++analysisRunID;
  activeAnalysisText = text;
  setStatus(options.loadingMessage || t("status.analyzingInput"), "loading");
  activeAnalysisPromise = runAnalysis(api, text, analysisKey, runID, options);
  return activeAnalysisPromise;
}

async function runAnalysis(api, text, analysisKey, runID, options) {
  try {
    const cached = options.preferCache ? await readBackendCachedAnalysis(api, text, analysisKey, runID, options) : null;
    if (cached) {
      return cached;
    }
    const result = hasQueuedStageAnalysisAPI(api)
      ? await runQueuedStageAnalysis(api, text, analysisKey, runID, options)
      : hasStagedAnalysisAPI(api)
        ? await runStagedAnalysis(api, text, analysisKey, runID, options)
        : await runFullAnalysis(api, text, analysisKey, runID, options);
    return result;
  } catch (error) {
    if (isCurrentAnalysis(runID, text)) {
      setStatus(error.message || String(error), "error");
    }
    return null;
  } finally {
    if (runID === analysisRunID) {
      activeAnalysisPromise = null;
      activeAnalysisText = "";
    }
  }
}

async function runQueuedStageAnalysis(api, text, analysisKey, runID, options) {
  const quick = await api.AnalyzeInputQuickText(text);
  if (!isCurrentAnalysis(runID, text, analysisKey)) {
    return null;
  }
  applyOverviewResult(quick, text, { analysisKey, stage: "quick" });
  setStatus(t("status.summaryReadyStages", {}, "Summary ready; preparing analysis stages"), "loading");
  await nextPaint();

  const stages = orderedAnalysisStages(state.activeResultTab);
  const results = await runStageQueue(stages, async (stage) => {
    setStatus(stageStatusMessage(stage), "loading");
    const result = await api.AnalyzeInputStageText(text, stage);
    if (!isCurrentAnalysis(runID, text, analysisKey)) {
      return null;
    }
    applyStageResult(stage, result);
    return result;
  }, { analysisKey });
  if (!isCurrentAnalysis(runID, text, analysisKey)) {
    return null;
  }
  state.analysisStage = "complete";
  scheduleIdlePreRender();
  window.dispatchEvent(new CustomEvent("idfAnalyzer:analysisComplete", { detail: { text } }));
  setStatus(options.statusMessage || t("status.analysisComplete"), "ok");
  return { ...quick, report: state.report, stages: results.filter(Boolean) };
}

async function runStageQueue(stages, worker, context = {}) {
  const results = [];
  const queue = {
    analysisKey: context.analysisKey || state.analysisKey || "",
    pending: stages.map((stage, index) => ({ stage, index })),
    running: new Set(),
    completed: new Set(),
    prioritize(stage) {
      const index = this.pending.findIndex((task) => task.stage === stage);
      if (index <= 0) {
        return false;
      }
      const [task] = this.pending.splice(index, 1);
      this.pending.unshift(task);
      return true;
    },
  };
  activeStageQueue = queue;
  async function runNext() {
    for (;;) {
      const task = queue.pending.shift();
      if (!task) {
        return;
      }
      queue.running.add(task.stage);
      try {
        results[task.index] = await worker(task.stage);
      } finally {
        queue.running.delete(task.stage);
        queue.completed.add(task.stage);
      }
    }
  }
  try {
    const workers = Array.from({ length: Math.min(maxFrontendStageConcurrency, stages.length) }, runNext);
    await Promise.all(workers);
    return results;
  } finally {
    if (activeStageQueue === queue) {
      activeStageQueue = null;
    }
  }
}

async function runFullAnalysis(api, text, analysisKey, runID, options) {
  const result =
    typeof api.AnalyzeInputText === "function"
      ? await api.AnalyzeInputText(text)
      : { report: await api.AnalyzeIDFText(text), model: null, epjson: "" };
  if (!isCurrentAnalysis(runID, text)) {
    return null;
  }
  applyOverviewResult(result, text, { complete: true, analysisKey });
  scheduleIdlePreRender();
  window.dispatchEvent(new CustomEvent("idfAnalyzer:analysisComplete", { detail: { text } }));
  setStatus(options.statusMessage || t("status.analysisComplete"), "ok");
  return result;
}

async function runStagedAnalysis(api, text, analysisKey, runID, options) {
  const overview = await api.AnalyzeInputOverviewText(text);
  if (!isCurrentAnalysis(runID, text)) {
    return null;
  }
  applyOverviewResult(overview, text, { analysisKey });
  setStatus(t("status.summaryReadyDiagnostics"), "loading");
  await nextPaint();

  const diagnostics = await api.AnalyzeInputDiagnosticsText(text);
  if (!isCurrentAnalysis(runID, text)) {
    return null;
  }
  state.report = state.report || {};
  state.report.diagnostics = diagnostics || [];
  state.diagnosticsReady = true;
  state.analysisStage = "diagnostics";
  markAnalysisDirty("diagnose");
  if (state.activeResultTab === "diagnose") {
    renderDiagnostics();
  }
  setStatus(t("status.diagnosticsReadyGeometry"), "loading");
  await nextPaint();

  const geometryPromise = api.AnalyzeInputGeometryText(text);
  const rendererPromise = preloadGeometryRenderer();
  const geometry = await geometryPromise;
  await rendererPromise;
  if (!isCurrentAnalysis(runID, text)) {
    return null;
  }
  state.report = state.report || {};
  state.report.geometry = geometry || {};
  state.geometryReady = true;
  state.analysisStage = "complete";
  markAnalysisDirty("geometry");
  if (state.activeResultTab === "geometry") {
    renderGeometry(state.report.geometry);
  }
  scheduleIdlePreRender();
  window.dispatchEvent(new CustomEvent("idfAnalyzer:analysisComplete", { detail: { text } }));
  setStatus(options.statusMessage || t("status.analysisComplete"), "ok");
  return { ...overview, report: state.report };
}

function applyStageResult(stage, result) {
  const report = result?.report || {};
  state.report = state.report || {};
  recordAnalysisTiming(result?.timing, stage);
  switch (stage) {
    case "profile":
      state.report.profile = report.profile || {};
      state.analysisReady.profile = true;
      markAnalysisDirty("profile");
      break;
    case "hvac":
      state.report.hvac = report.hvac || {};
      state.analysisReady.hvac = true;
      markAnalysisDirty("hvac");
      break;
    case "output":
      state.report.output = report.output || {};
      state.analysisReady.output = true;
      markAnalysisDirty("output");
      break;
    case "diagnostics":
      state.report.diagnostics = report.diagnostics || [];
      state.diagnosticsReady = true;
      state.analysisReady.diagnose = true;
      markAnalysisDirty("diagnose");
      break;
    case "geometry":
      state.report.geometry = report.geometry || {};
      state.geometryReady = true;
      state.analysisReady.geometry = true;
      markAnalysisDirty("geometry");
      break;
    default:
      return;
  }
  if (stageMatchesActiveTab(stage)) {
    renderReport({ scope: "active" });
  } else {
    updateResultTabReadiness();
  }
}

export function prioritizeAnalysisStageForTab(tab = state.activeResultTab) {
  const stage = resultTabStage(tab);
  if (!stage || !activeStageQueue) {
    return false;
  }
  if (activeStageQueue.analysisKey && state.analysisKey && activeStageQueue.analysisKey !== state.analysisKey) {
    return false;
  }
  const promoted = activeStageQueue.prioritize(stage);
  if (promoted) {
    updateResultTabReadiness();
  }
  return promoted;
}

function orderedAnalysisStages(activeTab) {
  const stages = ["profile", "hvac", "output", "diagnostics", "geometry"];
  const priority = resultTabStage(activeTab);
  if (!priority) {
    return stages;
  }
  return [priority, ...stages.filter((stage) => stage !== priority)];
}

function stageMatchesActiveTab(stage) {
  return resultTabStage(state.activeResultTab) === stage;
}

function resultTabStage(tab) {
  const stagesByTab = {
    profile: "profile",
    hvac: "hvac",
    output: "output",
    diagnose: "diagnostics",
    geometry: "geometry",
  };
  return stagesByTab[tab] || "";
}

function stageStatusMessage(stage) {
  switch (stage) {
    case "profile":
      return t("status.buildingProfile", {}, "Building profile graphs");
    case "hvac":
      return t("status.resolvingHVAC", {}, "Resolving HVAC service paths");
    case "output":
      return t("status.checkingOutput", {}, "Checking output requests");
    case "diagnostics":
      return t("status.checkingDiagnostics", {}, "Checking diagnostics");
    case "geometry":
      return t("status.preparingGeometry", {}, "Preparing geometry");
    default:
      return t("status.analyzingInput");
  }
}

async function readBackendCachedAnalysis(api, text, analysisKey, runID, options) {
  if (!analysisKey || typeof api.GetCachedAnalysis !== "function") {
    return null;
  }
  const cached = await api.GetCachedAnalysis(analysisKey);
  if (!cached || !isCurrentAnalysis(runID, text) || !isCompleteAnalysisResult(cached)) {
    return null;
  }
  applyOverviewResult(cached, text, { complete: true, analysisKey });
  scheduleIdlePreRender();
  window.dispatchEvent(new CustomEvent("idfAnalyzer:analysisComplete", { detail: { text } }));
  setStatus(options.statusMessage || t("status.analysisComplete"), "ok");
  return cached;
}

function isCompleteAnalysisResult(result) {
  if (result?.timing?.mode === "full") {
    return true;
  }
  const report = result?.report;
  return Array.isArray(report?.diagnostics) && Boolean(report?.geometry);
}

function applyOverviewResult(result, text, { complete = false, analysisKey = "" } = {}) {
  state.report = result.report;
  state.model = result.model || null;
  state.epjsonText = result.epjson || "";
  state.semanticProjection = result.semantic || null;
  state.lastAnalyzedText = text;
  state.analysisKey = result.analysisKey || analysisKey || state.analysisKey;
  state.lastAnalyzedKey = state.analysisKey;
  state.hvacServiceGraphLayoutCache?.clear?.();
  state.profileViewCache?.clear?.();
  state.geometryPlanLayoutCache?.clear?.();
  state.analysisStageTimings = {};
  recordAnalysisTiming(result.timing);
  state.analysisStage = complete ? "complete" : "overview";
  state.diagnosticsReady = complete;
  state.geometryReady = complete;
  setAnalysisReadiness(complete);
  markAllAnalysisDirty();
  renderReport({ scope: "active" });
  updateDocumentActions();
}

function hasQueuedStageAnalysisAPI(api) {
  return typeof api.AnalyzeInputQuickText === "function" && typeof api.AnalyzeInputStageText === "function";
}

function setAnalysisReadiness(complete) {
  state.analysisReady.summary = Boolean(state.report?.summary);
  state.analysisReady.output = complete || Boolean(state.report?.output);
  state.analysisReady.profile = complete;
  state.analysisReady.hvac = complete;
  state.analysisReady.diagnose = complete;
  state.analysisReady.geometry = complete;
  state.analysisReady.simulation = true;
  updateResultTabReadiness();
}

function resetAnalysisReadiness() {
  state.analysisReady.summary = false;
  state.analysisReady.profile = false;
  state.analysisReady.hvac = false;
  state.analysisReady.output = false;
  state.analysisReady.diagnose = false;
  state.analysisReady.geometry = false;
  state.analysisReady.simulation = true;
  updateResultTabReadiness();
}

function hasStagedAnalysisAPI(api) {
  return (
    typeof api.AnalyzeInputOverviewText === "function" &&
    typeof api.AnalyzeInputDiagnosticsText === "function" &&
    typeof api.AnalyzeInputGeometryText === "function"
  );
}

function isCurrentAnalysis(runID, text, analysisKey = "") {
  return runID === analysisRunID && text === elements.idfInput.value && (!analysisKey || !state.analysisKey || analysisKey === state.analysisKey);
}

export function scheduleAnalyzeAfterPaint(options = {}) {
  clearScheduledAnalyze();
  const textSnapshot = normalizeLineEndings(options.textSnapshot ?? elements.idfInput.value);
  state.lastAnalyzedText = "";
  state.lastAnalyzedKey = "";
  state.analysisStage = "queued";
  state.analysisTiming = null;
  state.analysisStageTimings = {};
  resetAnalysisReadiness();
  state.diagnosticsReady = false;
  state.geometryReady = false;
  updateDocumentActions();
  setStatus(options.queuedMessage || t("status.analysisQueued"), "muted");
  const delay = Number.isFinite(Number(options.delay)) ? Math.max(0, Number(options.delay)) : 40;
  afterPaintAnalyzeTimer = window.setTimeout(() => {
    afterPaintAnalyzeTimer = 0;
    if (normalizeLineEndings(elements.idfInput.value) !== textSnapshot) {
      return;
    }
    analyze(options);
  }, delay);
}

export function scheduleAutoAnalyze(delay = state.autoAnalyzeDelayMs) {
  clearScheduledAnalyze();
  state.lastAnalyzedText = "";
  state.lastAnalyzedKey = "";
  updateDocumentActions();
  setStatus(t("status.editingPending"), "muted");
  autoAnalyzeTimer = window.setTimeout(() => {
    autoAnalyzeTimer = 0;
    scheduleAnalyzeAfterPaint({
      queuedMessage: t("status.editingPausedQueued"),
      statusMessage: t("status.autoComplete"),
    });
  }, delay);
}

export function registerLoadedDocument(text, { path = "", filename = "" } = {}) {
  state.currentFilePath = path;
  state.currentFilename = filename;
  state.loadedText = text;
  state.savedText = text;
  state.lastAnalyzedText = "";
  state.lastAnalyzedKey = "";
  state.analysisKey = "";
  resetAnalysisReadiness();
  state.report = null;
  state.model = null;
  state.epjsonText = "";
  state.semanticProjection = null;
  state.analysisTiming = null;
  state.analysisStageTimings = {};
  state.renderTiming = { tabs: {}, last: null };
  state.semanticSelectedObjectIndex = "";
  state.semanticExpandedSectionIds = new Set(["project"]);
  state.analysisStage = "idle";
  state.diagnosticsReady = false;
  state.geometryReady = false;
  renderEmpty();
  updateDocumentActions();
}

export function markDocumentChanged() {
  state.lastAnalyzedText = "";
  state.lastAnalyzedKey = "";
  state.analysisKey = "";
  state.analysisStage = "pending";
  state.analysisTiming = null;
  state.analysisStageTimings = {};
  state.renderTiming = { tabs: {}, last: null };
  resetAnalysisReadiness();
  state.diagnosticsReady = false;
  state.geometryReady = false;
  updateDocumentActions();
  window.dispatchEvent(new Event("idfAnalyzer:documentChanged"));
}

export function updateDocumentActions() {
  const text = elements.idfInput?.value || "";
  const hasLoadedText = state.loadedText !== "";
  const changedFromLoad = hasLoadedText && text !== state.loadedText;
  if (elements.revertButton) {
    elements.revertButton.disabled = !changedFromLoad;
  }
  if (elements.saveButton) {
    elements.saveButton.disabled = text.length === 0;
  }
}

export async function openInputFile() {
  const api = backend();
  if (!api || typeof api.OpenInputFile !== "function") {
    elements.fileInput?.click();
    return;
  }

  try {
    const result = await api.OpenInputFile();
    if (!result || result.canceled) {
      return;
    }
    elements.idfInput.value = result.text || "";
    updateTextStats();
    registerLoadedDocument(elements.idfInput.value, {
      path: result.path || "",
      filename: result.filename || "",
    });
    scheduleAnalyzeAfterPaint({
      loadingMessage: t("status.analyzingNamed", { name: result.filename || t("common.inputFile") }),
      queuedMessage: t("status.openedQueued", { name: result.filename || t("common.inputFile") }),
      statusMessage: t("status.openedNamed", { name: result.filename || t("common.inputFile") }),
    });
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
}

export async function loadBrowserFile(file) {
  elements.idfInput.value = await file.text();
  updateTextStats();
  registerLoadedDocument(elements.idfInput.value, { filename: file.name || "" });
  scheduleAnalyzeAfterPaint({
    loadingMessage: t("status.analyzingNamed", { name: file.name || t("common.inputFile") }),
    queuedMessage: t("status.openedQueued", { name: file.name || t("common.inputFile") }),
    statusMessage: t("status.openedNamed", { name: file.name || t("common.inputFile") }),
  });
}

export async function saveInputFile() {
  const api = backend();
  if (!api || typeof api.SaveInputFile !== "function") {
    setStatus(t("status.backendUnavailable"), "warn");
    return;
  }

  try {
    const text = elements.idfInput.value;
    const suggestedFilename = suggestedSaveFilename();
    const result = state.currentFilePath
      ? await api.SaveInputFile(state.currentFilePath, text)
      : await api.SaveInputFileAs(text, suggestedFilename);
    if (!result || result.canceled) {
      return;
    }
    state.currentFilePath = result.path || state.currentFilePath;
    state.currentFilename = result.filename || state.currentFilename || suggestedFilename;
    state.savedText = text;
    updateDocumentActions();
    setStatus(t("status.savedNamed", { name: state.currentFilename || t("common.inputFile") }), "ok");
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
}

export async function revertToLoadedDocument() {
  if (state.loadedText === "" || elements.idfInput.value === state.loadedText) {
    return;
  }
  elements.idfInput.value = state.loadedText;
  updateTextStats();
  markDocumentChanged();
  scheduleAnalyzeAfterPaint({
    queuedMessage: t("status.revertedQueued"),
    statusMessage: t("status.reverted"),
  });
}

export async function removeUnused() {
  const api = backend();
  if (!api) {
    setStatus(t("status.backendUnavailable"), "warn");
    return;
  }

  try {
    const result = await api.RemoveUnusedObjectsText(elements.idfInput.value);
    elements.idfInput.value = result.text;
    updateTextStats();
    markDocumentChanged();
    await analyze();
    setStatus(t("status.unusedRemoved"), "ok");
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
}

export async function exportSummary(format) {
  const api = backend();
  if (!api || typeof api.ExportSummaryText !== "function") {
    setStatus(t("status.backendUnavailable"), "warn");
    return;
  }

  try {
    const result = await api.ExportSummaryText(elements.idfInput.value, format);
    const blob = new Blob([result.text], { type: result.mime || "text/plain" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = result.filename || `summary.${format}`;
    link.click();
    URL.revokeObjectURL(url);
    setStatus(t("status.summaryExported", { format: String(format).toUpperCase() }), "ok");
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
}

export function openGuide() {
  window.location.assign("./guide.html");
}

export async function openBatch() {
  await saveWorkspaceSnapshot();
  window.location.assign("./batch.html");
}

export function openTools() {
  return openBatch();
}

export async function openSettings() {
  await saveWorkspaceSnapshot();
  window.location.assign("./settings.html");
}

export async function saveWorkspaceSnapshot() {
  const text = elements.idfInput.value || "";
  if (!text.trim()) {
    return;
  }
  const analysisKey = state.analysisKey || state.lastAnalyzedKey || (await computeAnalysisKey(text));
  const snapshot = {
    schemaVersion: workspaceSnapshotVersion,
    text,
    path: state.currentFilePath || "",
    filename: state.currentFilename || "",
    loadedText: state.loadedText || "",
    savedText: state.savedText || "",
    analysisKey,
    activeResultTab: state.activeResultTab || "summary",
    activeInputView: state.activeInputView || "semantic",
    analysisStage: state.analysisStage || "idle",
    diagnosticsReady: Boolean(state.diagnosticsReady),
    geometryReady: Boolean(state.geometryReady),
    capturedAt: new Date().toISOString(),
  };
  try {
    window.sessionStorage.setItem(currentDocumentStorageKey, JSON.stringify(snapshot));
  } catch {
    // Navigation should still proceed if the browser refuses session storage.
  }
}

export function applyCachedAnalysisResult(result, snapshot = {}) {
  if (!result?.report) {
    return false;
  }
  const text = elements.idfInput.value || "";
  if (result.text && normalizeLineEndings(result.text) !== normalizeLineEndings(text)) {
    return false;
  }
  const analysisKey = snapshot.analysisKey || result.analysisKey || "";
  state.report = result.report;
  state.model = result.model || null;
  state.epjsonText = result.epjson || "";
  state.semanticProjection = result.semantic || null;
  state.lastAnalyzedText = text;
  state.analysisKey = analysisKey;
  state.lastAnalyzedKey = analysisKey;
  state.hvacServiceGraphLayoutCache?.clear?.();
  state.profileViewCache?.clear?.();
  state.geometryPlanLayoutCache?.clear?.();
  state.analysisStageTimings = {};
  recordAnalysisTiming(result.timing);
  const complete = isCompleteAnalysisResult(result);
  state.analysisStage = complete ? "complete" : "overview";
  state.diagnosticsReady = complete;
  state.geometryReady = complete;
  setAnalysisReadiness(complete);
  markAllAnalysisDirty();
  renderReport({ scope: "active" });
  scheduleIdlePreRender();
  updateDocumentActions();
  return complete;
}

function recordAnalysisTiming(timing, stage = "") {
  if (!timing) {
    return;
  }
  state.analysisTiming = timing;
  if (!state.analysisStageTimings) {
    state.analysisStageTimings = {};
  }
  if (timing.stages) {
    Object.assign(state.analysisStageTimings, timing.stages);
  } else if (stage && Number.isFinite(Number(timing.analyzeMs))) {
    state.analysisStageTimings[stage] = Number(timing.analyzeMs);
  }
  refreshStatusTitle();
}

function clearScheduledAnalyze() {
  window.clearTimeout(autoAnalyzeTimer);
  autoAnalyzeTimer = 0;
  window.clearTimeout(afterPaintAnalyzeTimer);
  afterPaintAnalyzeTimer = 0;
  cancelIdlePreRender();
}

function nextPaint() {
  return new Promise((resolve) => {
    let done = false;
    let fallbackTimer = 0;
    const finish = () => {
      if (done) {
        return;
      }
      done = true;
      window.clearTimeout(fallbackTimer);
      resolve();
    };
    fallbackTimer = window.setTimeout(finish, 80);
    if (typeof window.requestAnimationFrame !== "function") {
      window.setTimeout(finish, 0);
      return;
    }
    window.requestAnimationFrame(() => window.setTimeout(finish, 0));
  });
}

function scheduleIdlePreRender() {
  cancelIdlePreRender();
  const schedule = window.requestIdleCallback || ((callback) => window.setTimeout(() => callback({ timeRemaining: () => 10 }), 200));
  idlePreRenderTimer = schedule(() => {
    idlePreRenderTimer = 0;
    const nextTab = nextDirtyInactiveTab();
    if (!nextTab || !state.report) {
      return;
    }
    renderResultTab(nextTab, state.report);
    if (nextDirtyInactiveTab()) {
      scheduleIdlePreRender();
    }
  }, { timeout: 1500 });
}

function cancelIdlePreRender() {
  if (!idlePreRenderTimer) {
    return;
  }
  if (window.cancelIdleCallback) {
    window.cancelIdleCallback(idlePreRenderTimer);
  } else {
    window.clearTimeout(idlePreRenderTimer);
  }
  idlePreRenderTimer = 0;
}

function nextDirtyInactiveTab() {
  const tabs = ["summary", "profile", "hvac", "output", "diagnose"];
  return tabs.find((tab) => tab !== state.activeResultTab && state.analysisDirty?.[tab]);
}

async function computeAnalysisKey(text) {
  try {
    const normalized = normalizeLineEndings(text);
    if (!window.crypto?.subtle || typeof TextEncoder !== "function") {
      return "";
    }
    const bytes = new TextEncoder().encode(normalized);
    const digest = await window.crypto.subtle.digest("SHA-256", bytes);
    return [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, "0")).join("");
  } catch {
    return "";
  }
}

function normalizeLineEndings(text) {
  return String(text ?? "").replace(/\r\n?/g, "\n");
}

function suggestedSaveFilename() {
  if (state.currentFilename) {
    return state.currentFilename;
  }
  return state.model?.format === "epjson" ? "model.epJSON" : "model.idf";
}
