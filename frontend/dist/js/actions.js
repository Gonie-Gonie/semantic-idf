import { backend, elements, setStatus, state, updateTextStats } from "./state.js";
import { renderEmpty, renderReport } from "./analysis-views.js";

export async function analyze() {
  const api = backend();
  updateTextStats();
  if (!api) {
    setStatus("Run with Go/Wails to enable IDF or epJSON analysis", "warn");
    renderEmpty();
    return;
  }

  try {
    const text = elements.idfInput.value;
    const result =
      typeof api.AnalyzeInputText === "function"
        ? await api.AnalyzeInputText(text)
        : { report: await api.AnalyzeIDFText(text), model: null, epjson: "" };
    state.report = result.report;
    state.model = result.model || null;
    state.epjsonText = result.epjson || "";
    state.lastAnalyzedText = text;
    renderReport();
    setStatus("Analysis complete", "ok");
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
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
    await analyze();
    const label = targetFormat === "epjson" ? "epJSON" : "IDF";
    setStatus(`Converted to ${label}`, "ok");
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
}

export function downloadText() {
  const blob = new Blob([elements.idfInput.value], { type: "text/plain" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = state.model?.format === "epjson" ? "model.epJSON" : "model.idf";
  link.click();
  URL.revokeObjectURL(url);
}

export function openGuide() {
  window.location.assign("./guide.html");
}

export function closeToolMenu() {
  if (elements.toolMenu) {
    elements.toolMenu.open = false;
  }
}

