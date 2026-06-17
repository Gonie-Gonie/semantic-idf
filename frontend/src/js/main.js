import { defaultSample, loadDefaultSampleIDF } from "./sample.js";
import { loadAndApplyAppSettings } from "./settings-client.js";
import { backend, elements, setStatus, state, updateTextStats } from "./state.js";
import {
  analyze,
  applyCachedAnalysisResult,
  exportSummary,
  loadBrowserFile,
  markDocumentChanged,
  openGuide,
  openInputFile,
  openSettings,
  openBatch,
  currentDocumentStorageKey,
  registerLoadedDocument,
  revertToLoadedDocument,
  saveInputFile,
  scheduleAnalyzeAfterPaint,
  scheduleAutoAnalyze,
  updateDocumentActions,
} from "./actions.js";
import { markAnalysisDirty, renderDiagnostics, renderEmpty, renderReport, renderSummary } from "./views/analysis-views.js";
import { renderGeometry, resizeGeometry, setGeometryMode, setGeometrySelectionAid, setGeometryStory } from "./geometry-loader.js";
import { initializeDiagnoseFixes } from "./views/diagnose-fixes.js";
import { initializeHVACControls } from "./views/hvac-views.js";
import { initializeOutputControls } from "./views/output-views.js";
import {
  configureInputViews,
  setInputFilter,
  setTableOrientation,
  switchInputView,
  syncTextViewFromRawCaret,
} from "./views/input-views.js";
import { initializeVerticalSplitters, initializeWorkspaceSplitter } from "./layout.js";
import {
  focusInputObject,
  handleAnalysisActivation,
  handleInputJumpActivation,
  jumpInputDefinition,
  jumpInputReferences,
  redoViewNavigation,
  switchResultTab,
  undoViewNavigation,
} from "./navigation.js";
import { initializeProfileControls, renderProfile } from "./views/profile-views.js";
import { initializeSimulationControls, loadSimulationEnvironment } from "./views/simulation-views.js";
import { normalizeAnalyzeTabOrder, t, translatePage } from "./i18n.js";
import { initializeKeyboardShortcuts } from "./shortcuts.js";

loadAndApplyAppSettings().then((result) => applyRuntimeSettings(result.settings));

configureInputViews({ analyze, renderReport });

elements.openButton.addEventListener("click", openInputFile);
elements.fileInput.addEventListener("change", async (event) => {
  const [file] = event.target.files || [];
  if (!file) {
    return;
  }
  await loadBrowserFile(file);
  elements.fileInput.value = "";
});

elements.saveButton.addEventListener("click", saveInputFile);
elements.revertButton.addEventListener("click", revertToLoadedDocument);
elements.exportSummaryJSONButton.addEventListener("click", () => exportSummary("json"));
elements.exportSummaryCSVButton.addEventListener("click", () => exportSummary("csv"));
elements.toolsButton.addEventListener("click", openBatch);
elements.guideButton.addEventListener("click", openGuide);
elements.settingsButton.addEventListener("click", openSettings);
elements.idfInput.addEventListener("input", () => {
  updateTextStats();
  markDocumentChanged();
  scheduleAutoAnalyze();
});
elements.idfInput.addEventListener("click", syncTextViewFromRawCaret);
elements.idfInput.addEventListener("keyup", syncTextViewFromRawCaret);
elements.syncRawTextToggle.addEventListener("change", () => {
  state.syncTextRawPosition = elements.syncRawTextToggle.checked;
});
elements.inputFilter.addEventListener("input", () => setInputFilter(elements.inputFilter.value));
elements.summaryFilter.addEventListener("input", () => renderSummary());
elements.diagnosticFilter.addEventListener("input", () => renderDiagnostics());
elements.resultTabButtons.forEach((button) => {
  button.addEventListener("click", () => {
    state.resultTabManuallySelected = true;
    switchResultTab(button.dataset.resultTab);
  });
});
elements.geometryModeButtons.forEach((button) => {
  button.addEventListener("click", () => setGeometryMode(button.dataset.geometryMode));
});
elements.geometryStorySelect.addEventListener("change", () => setGeometryStory(elements.geometryStorySelect.value));
elements.geometrySelectionAid.addEventListener("click", () => setGeometrySelectionAid(!state.geometrySelectionAid));
elements.geometrySyncLocate.addEventListener("change", () => {
  state.geometrySyncLocate = elements.geometrySyncLocate.checked;
  renderGeometry();
});
elements.geometryShowZones.addEventListener("change", () => renderGeometry());
elements.geometryShowWalls.addEventListener("change", () => renderGeometry());
elements.geometryShowWindows.addEventListener("change", () => renderGeometry());
elements.hvacExpandButton.addEventListener("click", () => toggleExpandedPane("hvac"));
elements.geometryExpandButton.addEventListener("click", () => toggleExpandedPane("geometry"));
elements.inputViewButtons.forEach((button) => {
  button.addEventListener("click", () => switchInputView(button.dataset.inputView));
});
elements.editorPanel.addEventListener("click", (event) => {
  if (handleInputJumpActivation(event.target)) {
    event.preventDefault();
  }
});
window.addEventListener("resize", () => {
  if (state.activeResultTab === "geometry" || state.expandedPane === "geometry") {
    resizeGeometry();
  }
});
window.addEventListener("keydown", (event) => {
  if (event.key === "Escape" && state.expandedPane) {
    event.preventDefault();
    toggleExpandedPane("");
    return;
  }
  if (event.key.toLowerCase() === "h" && state.activeResultTab === "geometry" && !isEditableTarget(event.target)) {
    event.preventDefault();
    setGeometrySelectionAid(!state.geometrySelectionAid);
  }
});
window.addEventListener("idfAnalyzer:documentChanged", () => {
  updateDocumentActions();
});
window.addEventListener("idfAnalyzer:geometryLocate", (event) => {
  const { objectIndex, objectType } = event.detail || {};
  if (objectIndex === undefined || objectIndex === null || String(objectIndex) === "") {
    return;
  }
  focusInputObject({ objectIndex, objectType });
});
elements.tableOrientationButtons.forEach((button) => {
  button.addEventListener("click", () => setTableOrientation(button.dataset.tableOrientation));
});
elements.analysisPanel.addEventListener("click", (event) => handleAnalysisActivation(event.target));
elements.analysisPanel.addEventListener("keydown", (event) => {
  if (event.key !== "Enter" && event.key !== " ") {
    return;
  }
  const target = event.target.closest(".navigable-row");
  if (!target) {
    return;
  }
  event.preventDefault();
  handleAnalysisActivation(target);
});
window.addEventListener("idfAnalyzer:settingsChanged", (event) => {
  applyRuntimeSettings(event.detail?.settings);
});
window.addEventListener("idfAnalyzer:profileApplied", (event) => {
  const result = event.detail || {};
  if (!result.text || !result.report) {
    return;
  }
  elements.idfInput.value = result.text;
  updateTextStats();
  state.report = result.report;
  state.model = result.model || null;
  state.epjsonText = result.epjson || "";
  state.semanticProjection = result.semantic || null;
  state.lastAnalyzedText = result.text;
  state.analysisKey = result.analysisKey || "";
  state.lastAnalyzedKey = state.analysisKey;
  state.analysisStage = "complete";
  state.diagnosticsReady = true;
  state.geometryReady = true;
  renderReport();
  updateDocumentActions();
  const changeCount = result.preview?.changes?.length || 0;
  setStatus(t("status.profileApplied", { count: changeCount }), "ok");
});
window.addEventListener("idfAnalyzer:hvacApplied", (event) => {
  const result = event.detail || {};
  if (!result.text || !result.report) {
    return;
  }
  elements.idfInput.value = result.text;
  updateTextStats();
  state.report = result.report;
  state.model = result.model || null;
  state.epjsonText = result.epjson || "";
  state.semanticProjection = result.semantic || null;
  state.lastAnalyzedText = result.text;
  state.analysisKey = result.analysisKey || "";
  state.lastAnalyzedKey = state.analysisKey;
  state.analysisStage = "complete";
  state.diagnosticsReady = true;
  state.geometryReady = true;
  renderReport();
  updateDocumentActions();
  const changeCount = result.preview?.changes?.filter((change) => change.requiresSave).length || 0;
  setStatus(t("status.hvacApplied", { count: changeCount }), "ok");
});
window.addEventListener("idfAnalyzer:outputApplied", (event) => {
  const result = event.detail || {};
  if (!result.text || !result.report) {
    return;
  }
  elements.idfInput.value = result.text;
  updateTextStats();
  state.report = result.report;
  state.model = result.model || null;
  state.epjsonText = result.epjson || "";
  state.semanticProjection = result.semantic || null;
  state.lastAnalyzedText = result.text;
  state.analysisKey = result.analysisKey || "";
  state.lastAnalyzedKey = state.analysisKey;
  state.analysisStage = "complete";
  state.diagnosticsReady = true;
  state.geometryReady = true;
  renderReport();
  updateDocumentActions();
  const changeCount = result.preview?.changes?.filter((change) => change.requiresSave).length || 0;
  setStatus(t("status.outputApplied", { count: changeCount }), "ok");
});

function toggleExpandedPane(pane) {
  state.expandedPane = state.expandedPane === pane ? "" : pane;
  if (!pane) {
    state.expandedPane = "";
  }
  document.body.classList.toggle("analysis-expanded-active", Boolean(state.expandedPane));
  elements.resultPanes.forEach((item) => {
    const id = item.id.replace(/Pane$/, "").toLowerCase();
    item.classList.toggle("analysis-expanded-pane", id === state.expandedPane);
  });
  updateExpandButtons();
  if (state.expandedPane === "geometry" || pane === "geometry") {
    window.requestAnimationFrame(resizeGeometry);
  }
}

function updateExpandButtons() {
  const expanded = state.expandedPane;
  if (elements.hvacExpandButton) {
    elements.hvacExpandButton.textContent = expanded === "hvac" ? t("action.close") : t("action.expand", {}, "Expand");
    elements.hvacExpandButton.classList.toggle("active", expanded === "hvac");
  }
  if (elements.geometryExpandButton) {
    elements.geometryExpandButton.textContent = expanded === "geometry" ? t("action.close") : t("action.expand", {}, "Expand");
    elements.geometryExpandButton.classList.toggle("active", expanded === "geometry");
  }
}

function isEditableTarget(target) {
  return Boolean(target?.closest?.("input, textarea, select, [contenteditable='true']"));
}

initializeWorkspaceSplitter();
initializeVerticalSplitters();
initializeProfileControls();
initializeHVACControls();
initializeOutputControls();
initializeSimulationControls();
initializeDiagnoseFixes();
initializeKeyboardShortcuts({
  save: saveInputFile,
  open: openInputFile,
  undoView: undoViewNavigation,
  redoView: redoViewNavigation,
  jumpDefinition: jumpInputDefinition,
  jumpReferences: jumpInputReferences,
  switchInputView,
  switchResultTab,
});
renderEmpty();
updateDocumentActions();
const restoredDocument = restoreCurrentDocument();
if (restoredDocument) {
  elements.idfInput.value = restoredDocument.text || "";
  updateTextStats();
  registerLoadedDocument(elements.idfInput.value, {
    path: restoredDocument.path || "",
    filename: restoredDocument.filename || "",
  });
  state.loadedText = typeof restoredDocument.loadedText === "string" ? restoredDocument.loadedText : state.loadedText;
  state.savedText = typeof restoredDocument.savedText === "string" ? restoredDocument.savedText : state.savedText;
  if (restoredDocument.activeInputView) {
    switchInputView(restoredDocument.activeInputView, { recordHistory: false });
  }
  if (restoredDocument.activeResultTab) {
    switchResultTab(restoredDocument.activeResultTab, { recordHistory: false });
  }
  restoreCachedDocumentAnalysis(restoredDocument);
} else {
  setStatus(t("status.analysisWillStart"), "loading");
  loadDefaultSampleIDF().then(async (sampleText) => {
    elements.idfInput.value = sampleText;
    updateTextStats();
    const loadedText = elements.idfInput.value;
    const sourceLabel = sampleText.includes("RefBldgLargeOfficeNew2004_Chicago") ? defaultSample.name : "Fallback sample";
    const sourceFilename = sourceLabel === "Fallback sample" ? "fallback-sample.idf" : "RefBldgLargeOfficeNew2004_Chicago.idf";
    registerLoadedDocument(loadedText, { filename: sourceFilename });
    if (sourceLabel !== "Fallback sample") {
      elements.runtimeStatus.title = defaultSample.source;
    }
    scheduleAnalyzeAfterPaint({
      loadingMessage: t("status.analyzingNamed", { name: sourceLabel }),
      queuedMessage: t("status.loadedQueued", { name: sourceLabel }),
      statusMessage: t("status.loadedNamed", { name: sourceLabel }),
      textSnapshot: loadedText,
    });
  });
}

async function restoreCachedDocumentAnalysis(restoredDocument) {
  const label = restoredDocument.filename || "current input";
  const api = backend();
  if (restoredDocument.analysisKey && api && typeof api.GetCachedAnalysis === "function") {
    try {
      const cached = await api.GetCachedAnalysis(restoredDocument.analysisKey);
      if (cached && applyCachedAnalysisResult(cached, restoredDocument)) {
        if (restoredDocument.activeResultTab) {
          switchResultTab(restoredDocument.activeResultTab, { recordHistory: false });
        }
        setStatus(t("status.loadedNamed", { name: label }), "ok");
        return;
      }
    } catch {
      // Fall through to normal analysis if the in-memory backend cache is unavailable.
    }
  }
  scheduleAnalyzeAfterPaint({
    loadingMessage: t("status.analyzingNamed", { name: label }),
    queuedMessage: t("status.loadedQueued", { name: label }),
    statusMessage: t("status.loadedNamed", { name: label }),
    textSnapshot: elements.idfInput.value,
    analysisKey: restoredDocument.analysisKey || "",
    preferCache: Boolean(restoredDocument.analysisKey),
  });
}

function restoreCurrentDocument() {
  try {
    const raw = window.sessionStorage.getItem(currentDocumentStorageKey);
    if (!raw) {
      return null;
    }
    const documentState = JSON.parse(raw);
    return typeof documentState?.text === "string" && documentState.text.trim() ? documentState : null;
  } catch {
    return null;
  }
}

function applyRuntimeSettings(settings) {
  if (!settings) {
    return;
  }
  state.autoAnalyzeDelayMs = settings.behavior?.autoAnalyzeDelayMs || state.autoAnalyzeDelayMs;
  state.simulationAutoRunOnOpen = settings.simulation?.autoRunOnOpen ?? state.simulationAutoRunOnOpen;
  if (elements.simulationAutoRunOnOpen) {
    elements.simulationAutoRunOnOpen.checked = Boolean(state.simulationAutoRunOnOpen);
  }
  loadSimulationEnvironment();
  if (typeof settings.interaction?.syncRawTextPosition === "boolean") {
    state.syncTextRawPosition = settings.interaction.syncRawTextPosition;
    if (elements.syncRawTextToggle) {
      elements.syncRawTextToggle.checked = state.syncTextRawPosition;
    }
  }
  if (typeof settings.interaction?.geometrySyncLocate === "boolean") {
    state.geometrySyncLocate = settings.interaction.geometrySyncLocate;
    if (elements.geometrySyncLocate) {
      elements.geometrySyncLocate.checked = state.geometrySyncLocate;
    }
  }
  if (settings.interaction?.shortcuts) {
    state.keyboardShortcuts = settings.interaction.shortcuts;
  }
  if (settings.profile) {
    state.profileSettings = settings.profile;
    state.profileViewCache?.clear?.();
    if (state.report?.profile) {
      markAnalysisDirty("profile");
      if (state.activeResultTab === "profile") {
        renderProfile(state.report.profile);
      }
    }
  }
  if (settings.appearance) {
    applyDefaultResultTab(settings.appearance.analysisTabOrder);
    translatePage();
    updateTextStats();
    if (state.report) {
      renderReport();
    } else {
      renderEmpty();
    }
  }
  if (state.report?.geometry && state.activeResultTab === "geometry") {
    renderGeometry();
  }
}

function applyDefaultResultTab(orderInput) {
  const [firstTab] = normalizeAnalyzeTabOrder(orderInput);
  if (!firstTab) {
    return;
  }
  state.defaultResultTab = firstTab;
  if (!state.resultTabManuallySelected && state.activeResultTab !== firstTab) {
    switchResultTab(firstTab, { recordHistory: false });
  }
}
