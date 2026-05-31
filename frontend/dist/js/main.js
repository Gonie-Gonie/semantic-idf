import { defaultSample, loadDefaultSampleIDF } from "./sample.js";
import { loadAndApplyAppSettings } from "./settings-client.js";
import { elements, state, updateTextStats } from "./state.js";
import {
  analyze,
  exportSummary,
  loadBrowserFile,
  markDocumentChanged,
  openGuide,
  openInputFile,
  openSettings,
  openTools,
  currentDocumentStorageKey,
  registerLoadedDocument,
  revertToLoadedDocument,
  saveInputFile,
  scheduleAnalyzeAfterPaint,
  scheduleAutoAnalyze,
  updateDocumentActions,
} from "./actions.js";
import { renderDiagnostics, renderEmpty, renderReport, renderSummary } from "./analysis-views.js";
import { renderGeometry, resizeGeometry, setGeometryMode, setGeometryStory } from "./geometry-loader.js";
import {
  configureInputViews,
  setInputFilter,
  setTableOrientation,
  switchInputView,
  syncTextViewFromRawCaret,
} from "./input-views.js";
import { initializeVerticalSplitters, initializeWorkspaceSplitter } from "./layout.js";
import { focusInputObject, handleAnalysisActivation, switchResultTab } from "./navigation.js";

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
elements.toolsButton.addEventListener("click", openTools);
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
  button.addEventListener("click", () => switchResultTab(button.dataset.resultTab));
});
elements.geometryModeButtons.forEach((button) => {
  button.addEventListener("click", () => setGeometryMode(button.dataset.geometryMode));
});
elements.geometryStorySelect.addEventListener("change", () => setGeometryStory(elements.geometryStorySelect.value));
elements.geometrySyncLocate.addEventListener("change", () => {
  state.geometrySyncLocate = elements.geometrySyncLocate.checked;
  renderGeometry();
});
elements.geometryShowZones.addEventListener("change", () => renderGeometry());
elements.geometryShowWalls.addEventListener("change", () => renderGeometry());
elements.geometryShowWindows.addEventListener("change", () => renderGeometry());
elements.inputViewButtons.forEach((button) => {
  button.addEventListener("click", () => switchInputView(button.dataset.inputView));
});
window.addEventListener("resize", () => {
  if (state.activeResultTab === "geometry") {
    resizeGeometry();
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

initializeWorkspaceSplitter();
initializeVerticalSplitters();
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
  scheduleAnalyzeAfterPaint({
    loadingMessage: `Analyzing ${restoredDocument.filename || "current input"}`,
    queuedMessage: `Loaded ${restoredDocument.filename || "current input"}; analysis queued`,
    statusMessage: `Loaded ${restoredDocument.filename || "current input"}`,
    textSnapshot: elements.idfInput.value,
  });
} else {
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
      loadingMessage: `Analyzing ${sourceLabel}`,
      queuedMessage: `Loaded ${sourceLabel}; analysis queued`,
      statusMessage: `Loaded ${sourceLabel}`,
      textSnapshot: loadedText,
    });
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
  if (state.report?.geometry && state.activeResultTab === "geometry") {
    renderGeometry();
  }
}
