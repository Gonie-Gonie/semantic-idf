import { backend, elements, setStatus, state, updateTextStats } from "./state.js";
import { renderDeferredGeometry, renderDiagnostics, renderEmpty, renderReport } from "./analysis-views.js";
import { preloadGeometryRenderer, renderGeometry } from "./geometry-loader.js";

let autoAnalyzeTimer = 0;
let afterPaintAnalyzeTimer = 0;
let analysisRunID = 0;
let activeAnalysisPromise = null;
let activeAnalysisText = "";

export async function analyze(options = {}) {
  const api = backend();
  updateTextStats();
  updateDocumentActions();
  if (!api) {
    setStatus("Run with Go/Wails to enable IDF or epJSON analysis", "warn");
    renderEmpty();
    return;
  }

  const text = elements.idfInput.value;
  if (activeAnalysisPromise && activeAnalysisText === text) {
    return activeAnalysisPromise;
  }

  clearScheduledAnalyze();
  const runID = ++analysisRunID;
  activeAnalysisText = text;
  setStatus(options.loadingMessage || "Analyzing input", "loading");
  activeAnalysisPromise = runAnalysis(api, text, runID, options);
  return activeAnalysisPromise;
}

async function runAnalysis(api, text, runID, options) {
  try {
    const result = hasStagedAnalysisAPI(api)
      ? await runStagedAnalysis(api, text, runID, options)
      : await runFullAnalysis(api, text, runID, options);
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

async function runFullAnalysis(api, text, runID, options) {
  const result =
    typeof api.AnalyzeInputText === "function"
      ? await api.AnalyzeInputText(text)
      : { report: await api.AnalyzeIDFText(text), model: null, epjson: "" };
  if (!isCurrentAnalysis(runID, text)) {
    return null;
  }
  applyOverviewResult(result, text, { complete: true });
  setStatus(options.statusMessage || "Analysis complete", "ok");
  return result;
}

async function runStagedAnalysis(api, text, runID, options) {
  const overview = await api.AnalyzeInputOverviewText(text);
  if (!isCurrentAnalysis(runID, text)) {
    return null;
  }
  applyOverviewResult(overview, text);
  setStatus("Summary ready; checking diagnostics", "loading");
  await nextPaint();

  const diagnostics = await api.AnalyzeInputDiagnosticsText(text);
  if (!isCurrentAnalysis(runID, text)) {
    return null;
  }
  state.report = state.report || {};
  state.report.diagnostics = diagnostics || [];
  state.diagnosticsReady = true;
  state.analysisStage = "diagnostics";
  renderDiagnostics();
  setStatus("Diagnostics ready; preparing geometry", "loading");
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
  if (state.activeResultTab === "geometry") {
    renderGeometry(state.report.geometry);
  } else {
    renderDeferredGeometry(state.report.geometry);
  }
  setStatus(options.statusMessage || "Analysis complete", "ok");
  return { ...overview, report: state.report };
}

function applyOverviewResult(result, text, { complete = false } = {}) {
  state.report = result.report;
  state.model = result.model || null;
  state.epjsonText = result.epjson || "";
  state.lastAnalyzedText = text;
  state.analysisStage = complete ? "complete" : "overview";
  state.diagnosticsReady = complete;
  state.geometryReady = complete;
  renderReport();
  updateDocumentActions();
}

function hasStagedAnalysisAPI(api) {
  return (
    typeof api.AnalyzeInputOverviewText === "function" &&
    typeof api.AnalyzeInputDiagnosticsText === "function" &&
    typeof api.AnalyzeInputGeometryText === "function"
  );
}

function isCurrentAnalysis(runID, text) {
  return runID === analysisRunID && text === elements.idfInput.value;
}

export function scheduleAnalyzeAfterPaint(options = {}) {
  clearScheduledAnalyze();
  const textSnapshot = normalizeLineEndings(options.textSnapshot ?? elements.idfInput.value);
  state.lastAnalyzedText = "";
  state.analysisStage = "queued";
  state.diagnosticsReady = false;
  state.geometryReady = false;
  updateDocumentActions();
  setStatus(options.queuedMessage || "Analysis queued", "muted");
  const delay = Number.isFinite(Number(options.delay)) ? Math.max(0, Number(options.delay)) : 40;
  afterPaintAnalyzeTimer = window.setTimeout(() => {
    afterPaintAnalyzeTimer = 0;
    if (normalizeLineEndings(elements.idfInput.value) !== textSnapshot) {
      return;
    }
    analyze(options);
  }, delay);
}

export function scheduleAutoAnalyze(delay = 900) {
  clearScheduledAnalyze();
  state.lastAnalyzedText = "";
  updateDocumentActions();
  setStatus("Editing; analysis pending", "muted");
  autoAnalyzeTimer = window.setTimeout(() => {
    autoAnalyzeTimer = 0;
    scheduleAnalyzeAfterPaint({
      queuedMessage: "Editing paused; analysis queued",
      statusMessage: "Auto analysis complete",
    });
  }, delay);
}

export function registerLoadedDocument(text, { path = "", filename = "" } = {}) {
  state.currentFilePath = path;
  state.currentFilename = filename;
  state.loadedText = text;
  state.savedText = text;
  state.lastAnalyzedText = "";
  state.report = null;
  state.model = null;
  state.epjsonText = "";
  state.analysisStage = "idle";
  state.diagnosticsReady = false;
  state.geometryReady = false;
  renderEmpty();
  updateDocumentActions();
}

export function markDocumentChanged() {
  state.lastAnalyzedText = "";
  state.analysisStage = "pending";
  state.diagnosticsReady = false;
  state.geometryReady = false;
  updateDocumentActions();
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
      loadingMessage: `Analyzing ${result.filename || "input file"}`,
      queuedMessage: `Opened ${result.filename || "input file"}; analysis queued`,
      statusMessage: `Opened ${result.filename || "input file"}`,
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
    loadingMessage: `Analyzing ${file.name || "input file"}`,
    queuedMessage: `Opened ${file.name || "input file"}; analysis queued`,
    statusMessage: `Opened ${file.name || "input file"}`,
  });
}

export async function saveInputFile() {
  const api = backend();
  if (!api || typeof api.SaveInputFile !== "function") {
    setStatus("Run with Go/Wails to save to disk", "warn");
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
    setStatus(`Saved ${state.currentFilename || "input file"}`, "ok");
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
    queuedMessage: "Reverted to opened input; analysis queued",
    statusMessage: "Reverted to opened input",
  });
}

export async function removeUnused() {
  const api = backend();
  if (!api) {
    setStatus("Backend unavailable", "warn");
    return;
  }

  try {
    const result = await api.RemoveUnusedObjectsText(elements.idfInput.value);
    elements.idfInput.value = result.text;
    updateTextStats();
    markDocumentChanged();
    await analyze();
    setStatus("Unused objects removed", "ok");
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
}

export async function convertInput(targetFormat) {
  const api = backend();
  if (!api || typeof api.ConvertInputText !== "function") {
    setStatus("Backend unavailable", "warn");
    return;
  }

  try {
    const result = await api.ConvertInputText(elements.idfInput.value, targetFormat);
    elements.idfInput.value = result.text;
    updateTextStats();
    markDocumentChanged();
    const label = targetFormat === "epjson" ? "epJSON" : "IDF";
    scheduleAnalyzeAfterPaint({
      queuedMessage: `Converted to ${label}; analysis queued`,
      statusMessage: `Converted to ${label}`,
    });
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
}

export async function exportSummary(format) {
  const api = backend();
  if (!api || typeof api.ExportSummaryText !== "function") {
    setStatus("Backend unavailable", "warn");
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
    setStatus(`Summary ${String(format).toUpperCase()} exported`, "ok");
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
}

export function openGuide() {
  window.location.assign("./guide.html");
}

export function openTools() {
  window.location.assign("./tools.html");
}

export function openSettings() {
  window.location.assign("./settings.html");
}

export function closeToolbarMenus() {
  elements.toolbarMenus.forEach((menu) => {
    menu.open = false;
  });
}

export function closeToolMenu() {
  closeToolbarMenus();
}

function clearScheduledAnalyze() {
  window.clearTimeout(autoAnalyzeTimer);
  autoAnalyzeTimer = 0;
  window.clearTimeout(afterPaintAnalyzeTimer);
  afterPaintAnalyzeTimer = 0;
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

function normalizeLineEndings(text) {
  return String(text ?? "").replace(/\r\n?/g, "\n");
}

function suggestedSaveFilename() {
  if (state.currentFilename) {
    return state.currentFilename;
  }
  return state.model?.format === "epjson" ? "model.epJSON" : "model.idf";
}
