export const state = {
  report: null,
  model: null,
  epjsonText: "",
  activeTab: "summary",
  activeInputView: "text",
  lastAnalyzedText: "",
  tableOrientation: "objects",
  tableGroupOrientations: new Map(),
  selectedZoneName: "",
  selectedZoneTab: "surfaces",
  jsonSearchQuery: "",
  jsonCollapseDepth: 2,
  jsonSelectedObjectIndex: "",
  syncTextRawPosition: true,
};

export const elements = {
  runtimeStatus: document.querySelector("#runtimeStatus"),
  fileInput: document.querySelector("#fileInput"),
  analyzeButton: document.querySelector("#analyzeButton"),
  removeUnusedButton: document.querySelector("#removeUnusedButton"),
  toIDFButton: document.querySelector("#toIDFButton"),
  toEPJSONButton: document.querySelector("#toEPJSONButton"),
  downloadButton: document.querySelector("#downloadButton"),
  guideButton: document.querySelector("#guideButton"),
  toolMenu: document.querySelector(".tool-menu"),
  idfInput: document.querySelector("#idfInput"),
  jsonTextInput: document.querySelector("#jsonTextInput"),
  applyJSONButton: document.querySelector("#applyJSONButton"),
  syncRawTextToggle: document.querySelector("#syncRawTextToggle"),
  textStats: document.querySelector("#textStats"),
  fieldStats: document.querySelector("#fieldStats"),
  textObjectView: document.querySelector("#textObjectView"),
  jsonStructuredView: document.querySelector("#jsonStructuredView"),
  fieldFilter: document.querySelector("#fieldFilter"),
  fieldTable: document.querySelector("#fieldTable"),
  tableOrientationButtons: document.querySelectorAll(".orientation-button"),
  workspace: document.querySelector(".workspace"),
  workspaceSplitter: document.querySelector("#workspaceSplitter"),
  inputViewButtons: document.querySelectorAll(".view-tab"),
  inputViews: document.querySelectorAll(".input-view"),
  analysisPanel: document.querySelector(".analysis-panel"),
  objectCount: document.querySelector("#objectCount"),
  typeCount: document.querySelector("#typeCount"),
  scheduleCount: document.querySelector("#scheduleCount"),
  unusedCount: document.querySelector("#unusedCount"),
  typeList: document.querySelector("#typeList"),
  zoneViz: document.querySelector("#zoneViz"),
  zoneDetails: document.querySelector("#zoneDetails"),
  systemViz: document.querySelector("#systemViz"),
  scheduleList: document.querySelector("#scheduleList"),
  unusedList: document.querySelector("#unusedList"),
  connectionList: document.querySelector("#connectionList"),
  tabs: document.querySelectorAll(".tab"),
  panes: document.querySelectorAll(".tab-pane"),
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
  };
  elements.runtimeStatus.style.color = colors[tone] || colors.muted;
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
