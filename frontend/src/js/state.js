import { t } from "./i18n.js";

export const state = {
  report: null,
  model: null,
  epjsonText: "",
  analysisStage: "idle",
  diagnosticsReady: false,
  geometryReady: false,
  activeResultTab: "summary",
  defaultResultTab: "summary",
  resultTabManuallySelected: false,
  activeInputView: "text",
  activeProfileView: "profile",
  activeProfileGroupId: "",
  activeProfileZoneName: "",
  activeHVACLoopId: "",
  activeHVACView: "loop",
  activeHVACNodeName: "",
  activeHVACGraphKey: "",
  hvacApplyField: null,
  hvacOutputRequest: null,
  hvacApplyPreview: null,
  profileGraphViewMode: "",
  profileGraphScaleMode: "auto",
  profileSettings: null,
  profileApplyPreview: null,
  geometryMode: "3d",
  selectedGeometryId: "",
  selectedGeometryKind: "",
  selectedGeometryStory: "all",
  geometrySyncLocate: true,
  geometrySelectionAid: false,
  expandedPane: "",
  geometryRenderer: null,
  lastAnalyzedText: "",
  currentFilePath: "",
  currentFilename: "",
  keyboardShortcuts: {},
  navigationUndoStack: [],
  navigationRedoStack: [],
  navigationRestoring: false,
  lastReferenceJump: null,
  loadedText: "",
  savedText: "",
  tableOrientation: "objects",
  tableGroupOrientations: new Map(),
  inputFilterQuery: "",
  jsonCollapseDepth: 2,
  jsonSelectedObjectIndex: "",
  syncTextRawPosition: true,
  autoAnalyzeDelayMs: 900,
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
  profileStats: document.querySelector("#profileStats"),
  profileFilter: document.querySelector("#profileFilter"),
  profileApplyButton: document.querySelector("#profileApplyButton"),
  profileSettings: document.querySelector("#profileSettings"),
  profileOverview: document.querySelector("#profileOverview"),
  profileDetail: document.querySelector("#profileDetail"),
  profileMatrixStats: document.querySelector("#profileMatrixStats"),
  profileMatrix: document.querySelector("#profileMatrix"),
  profileGraphStats: document.querySelector("#profileGraphStats"),
  profileGraph: document.querySelector("#profileGraph"),
  profileApplyDialog: document.querySelector("#profileApplyDialog"),
  profileApplyForm: document.querySelector("#profileApplyForm"),
  profileApplyClose: document.querySelector("#profileApplyClose"),
  profileApplyBody: document.querySelector("#profileApplyBody"),
  profilePreviewApply: document.querySelector("#profilePreviewApply"),
  profileConfirmApply: document.querySelector("#profileConfirmApply"),
  profileApplyStatus: document.querySelector("#profileApplyStatus"),
  hvacStats: document.querySelector("#hvacStats"),
  hvacFilter: document.querySelector("#hvacFilter"),
  hvacExpandButton: document.querySelector("#hvacExpandButton"),
  hvacSummary: document.querySelector("#hvacSummary"),
  hvacGraph: document.querySelector("#hvacGraph"),
  hvacInspectorStats: document.querySelector("#hvacInspectorStats"),
  hvacInspector: document.querySelector("#hvacInspector"),
  hvacWarningStats: document.querySelector("#hvacWarningStats"),
  hvacWarnings: document.querySelector("#hvacWarnings"),
  hvacApplyDialog: document.querySelector("#hvacApplyDialog"),
  hvacApplyForm: document.querySelector("#hvacApplyForm"),
  hvacApplyClose: document.querySelector("#hvacApplyClose"),
  hvacApplyBody: document.querySelector("#hvacApplyBody"),
  hvacPreviewApply: document.querySelector("#hvacPreviewApply"),
  hvacConfirmApply: document.querySelector("#hvacConfirmApply"),
  hvacApplyStatus: document.querySelector("#hvacApplyStatus"),
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
  geometrySelectionAid: document.querySelector("#geometrySelectionAid"),
  geometryExpandButton: document.querySelector("#geometryExpandButton"),
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
    muted: "--muted",
    ok: "--green",
    warn: "--amber",
    error: "--red",
    loading: "--muted",
  };
  elements.runtimeStatus.style.color = cssVar(colors[tone] || colors.muted);
  elements.runtimeStatus.classList.toggle("status-loading", tone === "loading");
}

function cssVar(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
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
  elements.textStats.textContent = t("count.lines", { count: lines });
}
