import { defaultSample, loadDefaultSampleIDF } from "./sample.js";
import { elements, state, updateTextStats } from "./state.js";
import { analyze, closeToolMenu, convertInput, downloadText, exportSummary, openGuide, removeUnused } from "./actions.js";
import { renderEmpty, renderReport, renderSummary } from "./analysis-views.js";
import { renderGeometry, resizeGeometry, setGeometryMode, setGeometryStory } from "./geometry-view.js";
import {
  configureInputViews,
  renderFieldTable,
  setTableOrientation,
  switchInputView,
  syncTextViewFromRawCaret,
} from "./input-views.js";
import { initializeVerticalSplitters, initializeWorkspaceSplitter } from "./layout.js";
import { handleAnalysisActivation, switchResultTab } from "./navigation.js";

configureInputViews({ analyze, renderReport });

elements.fileInput.addEventListener("change", async (event) => {
  const [file] = event.target.files || [];
  if (!file) {
    return;
  }
  elements.idfInput.value = await file.text();
  updateTextStats();
  await analyze();
});

elements.analyzeButton.addEventListener("click", analyze);
elements.removeUnusedButton.addEventListener("click", async () => {
  closeToolMenu();
  await removeUnused();
});
elements.toIDFButton.addEventListener("click", async () => {
  closeToolMenu();
  await convertInput("idf");
});
elements.toEPJSONButton.addEventListener("click", async () => {
  closeToolMenu();
  await convertInput("epjson");
});
elements.downloadButton.addEventListener("click", downloadText);
elements.exportSummaryJSONButton.addEventListener("click", () => exportSummary("json"));
elements.exportSummaryCSVButton.addEventListener("click", () => exportSummary("csv"));
elements.guideButton.addEventListener("click", openGuide);
elements.idfInput.addEventListener("input", () => {
  updateTextStats();
  state.lastAnalyzedText = "";
});
elements.idfInput.addEventListener("click", syncTextViewFromRawCaret);
elements.idfInput.addEventListener("keyup", syncTextViewFromRawCaret);
elements.syncRawTextToggle.addEventListener("change", () => {
  state.syncTextRawPosition = elements.syncRawTextToggle.checked;
});
elements.fieldFilter.addEventListener("input", renderFieldTable);
elements.summaryFilter.addEventListener("input", () => renderSummary());
elements.resultTabButtons.forEach((button) => {
  button.addEventListener("click", () => switchResultTab(button.dataset.resultTab));
});
elements.geometryModeButtons.forEach((button) => {
  button.addEventListener("click", () => setGeometryMode(button.dataset.geometryMode));
});
elements.geometryStorySelect.addEventListener("change", () => setGeometryStory(elements.geometryStorySelect.value));
elements.geometryShowZones.addEventListener("change", () => renderGeometry());
elements.geometryShowWalls.addEventListener("change", () => renderGeometry());
elements.geometryShowWindows.addEventListener("change", () => renderGeometry());
elements.inputViewButtons.forEach((button) => {
  button.addEventListener("click", () => switchInputView(button.dataset.inputView));
});
window.addEventListener("resize", resizeGeometry);
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

initializeWorkspaceSplitter();
initializeVerticalSplitters();
renderEmpty();
loadDefaultSampleIDF().then(async (sampleText) => {
  elements.idfInput.value = sampleText;
  updateTextStats();
  await analyze();
  state.lastAnalyzedText = sampleText;
  const sourceLabel = sampleText.includes("RefBldgLargeOfficeNew2004_Chicago") ? defaultSample.name : "Fallback sample";
  if (sourceLabel !== "Fallback sample") {
    elements.runtimeStatus.title = defaultSample.source;
  }
});
