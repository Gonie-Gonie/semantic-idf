import { backend, elements, setStatus, state, updateTextStats } from "./state.js";
import { renderEmpty, renderReport } from "./analysis-views.js";

let autoAnalyzeTimer = 0;
let analysisRunID = 0;

export async function analyze(options = {}) {
  const api = backend();
  updateTextStats();
  updateDocumentActions();
  if (!api) {
    setStatus("Run with Go/Wails to enable IDF or epJSON analysis", "warn");
    renderEmpty();
    return;
  }

  window.clearTimeout(autoAnalyzeTimer);
  try {
    const text = elements.idfInput.value;
    const runID = ++analysisRunID;
    const result =
      typeof api.AnalyzeInputText === "function"
        ? await api.AnalyzeInputText(text)
        : { report: await api.AnalyzeIDFText(text), model: null, epjson: "" };
    if (runID !== analysisRunID || text !== elements.idfInput.value) {
      return;
    }
    state.report = result.report;
    state.model = result.model || null;
    state.epjsonText = result.epjson || "";
    state.lastAnalyzedText = text;
    renderReport();
    updateDocumentActions();
    setStatus(options.statusMessage || "Analysis complete", "ok");
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
}

export function scheduleAutoAnalyze(delay = 900) {
  window.clearTimeout(autoAnalyzeTimer);
  state.lastAnalyzedText = "";
  updateDocumentActions();
  setStatus("Editing; analysis pending", "muted");
  autoAnalyzeTimer = window.setTimeout(() => {
    analyze({ statusMessage: "Auto analysis complete" });
  }, delay);
}

export function registerLoadedDocument(text, { path = "", filename = "" } = {}) {
  state.currentFilePath = path;
  state.currentFilename = filename;
  state.loadedText = text;
  state.savedText = text;
  state.lastAnalyzedText = "";
  updateDocumentActions();
}

export function markDocumentChanged() {
  state.lastAnalyzedText = "";
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
    await analyze({ statusMessage: `Opened ${result.filename || "input file"}` });
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
}

export async function loadBrowserFile(file) {
  elements.idfInput.value = await file.text();
  updateTextStats();
  registerLoadedDocument(elements.idfInput.value, { filename: file.name || "" });
  await analyze({ statusMessage: `Opened ${file.name || "input file"}` });
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
  await analyze({ statusMessage: "Reverted to opened input" });
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
    await analyze();
    const label = targetFormat === "epjson" ? "epJSON" : "IDF";
    setStatus(`Converted to ${label}`, "ok");
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

function suggestedSaveFilename() {
  if (state.currentFilename) {
    return state.currentFilename;
  }
  return state.model?.format === "epjson" ? "model.epJSON" : "model.idf";
}
