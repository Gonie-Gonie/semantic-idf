import { sampleIDF } from "./sample.js";
import { elements, state, updateTextStats } from "./state.js";
import { analyze, closeToolMenu, convertInput, downloadText, openGuide, removeUnused } from "./actions.js";
import { renderEmpty } from "./analysis-views.js";
import { applyJSONText, configureInputViews, renderFieldTable, setTableOrientation, switchInputView } from "./input-views.js";
import { initializeWorkspaceSplitter } from "./layout.js";
import { handleAnalysisActivation, switchTab } from "./navigation.js";

configureInputViews({ analyze });

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
elements.guideButton.addEventListener("click", openGuide);
elements.applyJSONButton.addEventListener("click", applyJSONText);
elements.idfInput.addEventListener("input", () => {
  updateTextStats();
  state.lastAnalyzedText = "";
});
elements.jsonTextInput.addEventListener("input", () => {
  state.lastAnalyzedText = "";
});
elements.fieldFilter.addEventListener("input", renderFieldTable);
elements.tabs.forEach((tab) => {
  tab.addEventListener("click", () => switchTab(tab.dataset.tab));
});
elements.inputViewButtons.forEach((button) => {
  button.addEventListener("click", () => switchInputView(button.dataset.inputView));
});
elements.tableOrientationButtons.forEach((button) => {
  button.addEventListener("click", () => setTableOrientation(button.dataset.tableOrientation));
});
elements.analysisPanel.addEventListener("click", (event) => handleAnalysisActivation(event.target));
elements.analysisPanel.addEventListener("keydown", (event) => {
  if (event.key !== "Enter" && event.key !== " ") {
    return;
  }
  const target = event.target.closest(".navigable-row, .zone-card, [data-zone-tab]");
  if (!target) {
    return;
  }
  event.preventDefault();
  handleAnalysisActivation(target);
});

initializeWorkspaceSplitter();
elements.idfInput.value = sampleIDF;
updateTextStats();
renderEmpty();
analyze();
