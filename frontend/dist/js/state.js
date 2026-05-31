export const state = {
  report: null,
  model: null,
  epjsonText: "",
  analysisStage: "idle",
  diagnosticsReady: false,
  geometryReady: false,
  activeResultTab: "summary",
  activeInputView: "text",
  geometryMode: "3d",
  selectedGeometryId: "",
  selectedGeometryKind: "",
  selectedGeometryStory: "all",
  geometrySyncLocate: true,
  geometryRenderer: null,
  lastAnalyzedText: "",
  currentFilePath: "",
  currentFilename: "",
  loadedText: "",
  savedText: "",
  tableOrientation: "objects",
  tableGroupOrientations: new Map(),
  inputFilterQuery: "",
  jsonCollapseDepth: 2,
  jsonSelectedObjectIndex: "",
  syncTextRawPosition: true,
};

export const elements = {
  runtimeStatus: document.querySelector("#runtimeStatus"),
  fileInput: document.querySelector("#fileInput"),
  openButton: document.querySelector("#openButton"),
  saveButton: document.querySelector("#saveButton"),
  revertButton: document.querySelector("#revertButton"),
  toolsButton: document.querySelector("#toolsButton"),
  guideButton: document.querySelector("#guideButton"),
  settingsButton: document.querySelector("#settingsButton"),
  idfInput: document.querySelector("#idfInput"),
  syncRawTextToggle: document.querySelector("#syncRawTextToggle"),
  textStats: document.querySelector("#textStats"),
  inputFilter: document.querySelector("#inputFilter"),
  inputFilterStats: document.querySelector("#inputFilterStats"),
  fieldStats: document.querySelector("#fieldStats"),
  textObjectView: document.querySelector("#textObjectView"),
  jsonStructuredView: document.querySelector("#jsonStructuredView"),
  fieldTable: document.querySelector("#fieldTable"),
  tableOrientationButtons: document.querySelectorAll(".orientation-button"),
  workspace: document.querySelector(".workspace"),
  workspaceSplitter: document.querySelector("#workspaceSplitter"),
  editorPanel: document.querySelector(".editor-panel"),
  inputRawSplitter: document.querySelector("#inputRawSplitter"),
  inputViewButtons: document.querySelectorAll(".view-tab"),
  inputViews: document.querySelectorAll(".input-view"),
  analysisPanel: document.querySelector(".analysis-panel"),
  resultTabButtons: document.querySelectorAll("[data-result-tab]"),
  resultPanes: document.querySelectorAll(".result-pane"),
  summaryMetricCount: document.querySelector("#summaryMetricCount"),
  summaryFilter: document.querySelector("#summaryFilter"),
  summaryCategories: document.querySelector("#summaryCategories"),
  diagnosticCount: document.querySelector("#diagnosticCount"),
  diagnosticFilter: document.querySelector("#diagnosticFilter"),
  diagnosticList: document.querySelector("#diagnosticList"),
  exportSummaryJSONButton: document.querySelector("#exportSummaryJSONButton"),
  exportSummaryCSVButton: document.querySelector("#exportSummaryCSVButton"),
  geometryStats: document.querySelector("#geometryStats"),
  geometryViewport: document.querySelector("#geometryViewport"),
  geometryBody: document.querySelector(".geometry-body"),
  geometryDetailsSplitter: document.querySelector("#geometryDetailsSplitter"),
  geometryCanvasHost: document.querySelector("#geometryCanvasHost"),
  geometryPlan: document.querySelector("#geometryPlan"),
  geometryDetails: document.querySelector("#geometryDetails"),
  geometryModeButtons: document.querySelectorAll("[data-geometry-mode]"),
  geometryStorySelect: document.querySelector("#geometryStorySelect"),
  geometrySyncLocate: document.querySelector("#geometrySyncLocate"),
  geometryShowZones: document.querySelector("#geometryShowZones"),
  geometryShowWalls: document.querySelector("#geometryShowWalls"),
  geometryShowWindows: document.querySelector("#geometryShowWindows"),
};

export function backend() {
  return window.go && window.go.main && window.go.main.App;
}

export function setStatus(message, tone = "muted") {
  elements.runtimeStatus.textContent = message;
  const colors = {
    muted: "#60707c",
    ok: "#246b44",
    warn: "#a85f00",
    error: "#b3261e",
    loading: "#60707c",
  };
  elements.runtimeStatus.style.color = colors[tone] || colors.muted;
  elements.runtimeStatus.classList.toggle("status-loading", tone === "loading");
}

export function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

export function updateTextStats() {
  const text = elements.idfInput.value;
  const lines = text.length === 0 ? 0 : text.split(/\r\n|\r|\n/).length;
  elements.textStats.textContent = `${lines} lines`;
}
