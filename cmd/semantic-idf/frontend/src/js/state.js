import { t } from "./i18n.js";

export const topologyModes = Object.freeze(["3d", "plan", "thermal"]);
export const thermalTopologyMetrics = Object.freeze(["topology", "area", "ua", "exposure", "qa", "air"]);
export const thermalTopologyLayouts = Object.freeze(["spatial", "network"]);

export function normalizeTopologyMode(value) {
  const mode = String(value || "").toLowerCase();
  return topologyModes.includes(mode) ? mode : "3d";
}

export function normalizeThermalTopologyMetric(value) {
  const metric = String(value || "").toLowerCase();
  return thermalTopologyMetrics.includes(metric) ? metric : "topology";
}

export function normalizeThermalTopologyLayout(value) {
  const layout = String(value || "").toLowerCase();
  return thermalTopologyLayouts.includes(layout) ? layout : "spatial";
}

export function normalizeThermalTopologyState(target = state) {
  target.topologyMode = normalizeTopologyMode(target.topologyMode);
  target.thermalTopologyMetric = normalizeThermalTopologyMetric(target.thermalTopologyMetric);
  target.thermalTopologyLayout = normalizeThermalTopologyLayout(target.thermalTopologyLayout);
  target.thermalTopologySelectedEntityId = String(target.thermalTopologySelectedEntityId || "");
  target.thermalTopologySelectedEntityKind = String(target.thermalTopologySelectedEntityKind || "");
  target.thermalTopologyPanX = finiteNumber(target.thermalTopologyPanX, 0);
  target.thermalTopologyPanY = finiteNumber(target.thermalTopologyPanY, 0);
  target.thermalTopologyScale = clampNumber(target.thermalTopologyScale, 0.1, 8, 1);
  target.thermalTopologyLayoutCache = target.thermalTopologyLayoutCache instanceof Map ? target.thermalTopologyLayoutCache : new Map();
  return target;
}

export function captureThermalTopologyState(source = state) {
  normalizeThermalTopologyState(source);
  return {
    thermalTopologyMetric: source.thermalTopologyMetric,
    thermalTopologyLayout: source.thermalTopologyLayout,
    thermalTopologySelectedEntityId: source.thermalTopologySelectedEntityId,
    thermalTopologySelectedEntityKind: source.thermalTopologySelectedEntityKind,
    thermalTopologyPanX: source.thermalTopologyPanX,
    thermalTopologyPanY: source.thermalTopologyPanY,
    thermalTopologyScale: source.thermalTopologyScale,
  };
}

export function restoreThermalTopologyState(snapshot = {}, target = state) {
  for (const key of Object.keys(captureThermalTopologyState(target))) {
    if (Object.prototype.hasOwnProperty.call(snapshot, key)) {
      target[key] = snapshot[key];
    }
  }
  return normalizeThermalTopologyState(target);
}

export function resetThermalTopologyDocumentState(target = state) {
  target.thermalTopologySelectedEntityId = "";
  target.thermalTopologySelectedEntityKind = "";
  target.thermalTopologyPanX = 0;
  target.thermalTopologyPanY = 0;
  target.thermalTopologyScale = 1;
  target.thermalTopologyLayoutCache?.clear?.();
  if (!(target.thermalTopologyLayoutCache instanceof Map)) {
    target.thermalTopologyLayoutCache = new Map();
  }
}

function finiteNumber(value, fallback) {
  const number = Number(value);
  return Number.isFinite(number) ? number : fallback;
}

function clampNumber(value, minimum, maximum, fallback) {
  return Math.min(maximum, Math.max(minimum, finiteNumber(value, fallback)));
}

export const state = {
  report: null,
  model: null,
  epjsonText: "",
  semanticProjection: null,
  analysisStage: "idle",
  analysisDirty: {
    metrics: true,
    profile: true,
    hvac: true,
    simulation: true,
    topology: true,
    input: true,
  },
  analysisReady: {
    metrics: false,
    profile: false,
    hvac: false,
    simulation: true,
    topology: false,
  },
  analysisTiming: null,
  analysisStageTimings: {},
  renderTiming: {
    tabs: {},
    last: null,
  },
  geometryReady: false,
  activeResultTab: "metrics",
  resultTabManuallySelected: false,
  activeInputView: "semantic",
  activeProfileView: "profile",
  activeProfileGroupId: "",
  activeProfileZoneName: "",
  activeHVACLoopId: "",
  activeHVACView: "services",
  activeHVACNodeName: "",
  activeHVACGraphKey: "",
  activeHVACEntity: {
    id: "",
    kind: "",
    label: "",
  },
  activeHVACContext: {
    pathId: "",
    zoneId: "",
    loopId: "",
    componentId: "",
    couplingId: "",
    previousView: "",
  },
  hvacNavigationStack: [],
  hvacForwardStack: [],
  activeHVACGraphScope: "focused",
  hvacServiceKindFilter: "all",
  hvacPathTypeFilter: "all",
  hvacMediumFilter: "all",
  hvacGraphScale: "actual",
  hvacDiagramViewportKey: "",
  hvacDiagramScale: 1,
  hvacDiagramPanX: 0,
  hvacDiagramPanY: 0,
  hvacServiceGraphLayoutCache: new Map(),
  hvacApplyField: null,
  hvacOutputRequest: null,
  hvacApplyPreview: null,
  simulationEnvironment: null,
  simulationResult: null,
  simulationProgress: null,
  simulationRunning: false,
  simulationActiveRunID: "",
  simulationSelectedSeries: "",
  simulationSeriesGroup: "all",
  simulationSeriesRangeStart: 0,
  simulationSeriesRangeEnd: -1,
  simulationHeatFlowFrameIndex: 0,
  simulationHVACFrameIndex: 0,
  simulationHeatFlowSelectedZone: "",
  simulationHeatFlowStory: "all",
  simulationHeatFlowRangeStart: 0,
  simulationHeatFlowRangeEnd: -1,
  simulationHeatFlowOverlay: "net",
  simulationHeatFlowPlanScale: 1,
  simulationHeatFlowPlanPanX: 0,
  simulationHeatFlowPlanPanY: 0,
  simulationHeatFlowInspectorCollapsed: false,
  simulationHeatFlowPlaying: false,
  simulationHVACPanels: {
    topology: false,
    snapshot: true,
    chart: true,
  },
  simulationHVACVisibleGroups: {
    temperature: true,
    setpoint: true,
    mass_flow: true,
    psychrometric: true,
    rate_load: true,
    power_energy: true,
    other: true,
  },
  simulationAutoRunOnOpen: false,
  simulationSelectedPurposes: ["basic_energy", "zone_heat_flow"],
  simulationActiveResultView: "energy",
  simulationEnergyView: "overview",
  simulationEnergyPeriod: "annual",
  simulationEnergyPeriodKind: "",
  simulationEnergySelection: "",
  simulationEnergyFocusMode: "all",
  simulationEnergyZoneFocus: "",
  simulationEnergyServicePathFocus: "",
  simulationEnergyLoopFocus: "",
  simulationEnergySankeyMode: "detailed",
  simulationEnergySignMode: "display",
  simulationEnergyNodeLimit: 80,
  simulationZoneEnergyMetric: "__total",
  simulationComfortZone: "",
  simulationAutoStartedKey: "",
  profileViewCache: new Map(),
  profileSelectedGroupIds: [],
  profileSelectedZoneNames: [],
  profileSelectedDimensions: [],
  profileSelectionAnchorKey: "",
  profileSettings: null,
  profileApplyPreview: null,
  topologyMode: "3d",
  thermalTopologyMetric: "topology",
  thermalTopologyLayout: "spatial",
  thermalTopologySelectedEntityId: "",
  thermalTopologySelectedEntityKind: "",
  thermalTopologyPanX: 0,
  thermalTopologyPanY: 0,
  thermalTopologyScale: 1,
  thermalTopologyLayoutCache: new Map(),
  geometryPlanLayoutCache: new Map(),
  selectedTopologyEntityId: "",
  selectedTopologyEntityKind: "",
  selectedTopologyStory: "all",
  topologySyncLocate: true,
  topologyVisibility: {
    zones: true,
    surfaces: true,
    openings: true,
  },
  expandedPane: "",
  reportAnalyzedText: "",
  reportAnalysisKey: "",
  reportAnalysisStage: "idle",
  reportAnalysisReady: null,
  reportGeometryReady: false,
  lastAnalyzedText: "",
  lastAnalyzedKey: "",
  analysisKey: "",
  currentFilePath: "",
  currentFilename: "",
  keyboardShortcuts: {},
  navigationUndoStack: [],
  navigationRedoStack: [],
  navigationRestoring: false,
  globalSelection: {
    entityId: "",
    entityKind: "",
    occurrenceId: "",
    sourceAnchor: null,
    originView: "",
    originTargetId: "",
    semanticPathHint: "",
    relatedEntityIds: [],
    transactionId: "",
  },
  globalHover: {
    entityId: "",
    occurrenceId: "",
    originView: "",
  },
  semanticLinkMode: true,
  semanticFollowSelection: true,
  semanticTemporaryReveal: null,
  semanticPendingNavigation: null,
  pendingWorkspaceRestore: null,
  pendingAnalysisPriorityTab: "",
  lastReferenceJump: null,
  loadedText: "",
  savedText: "",
  tableOrientation: "objects",
  tableGroupOrientations: new Map(),
  inputFilterQuery: "",
  // Source-location fallback for legacy views; semantic identity lives in globalSelection.
  semanticSelectedObjectIndex: "",
  semanticProjectionMode: "basic",
  semanticProjectionFacet: "all",
  semanticExpandedSectionIds: new Set(["project"]),
  semanticCurrentOccurrenceId: "",
  semanticCurrentPath: "",
  semanticPinnedEntityIds: new Set(),
  semanticEditSelectionRestore: null,
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
  workspaceLinkBar: document.querySelector("#workspaceLinkBar"),
  semanticLinkedToggle: document.querySelector("#semanticLinkedToggle"),
  semanticFollowToggle: document.querySelector("#semanticFollowToggle"),
  workspaceSelectionLabel: document.querySelector("#workspaceSelectionLabel"),
  workspaceLinkTargets: document.querySelector("#workspaceLinkTargets"),
  workspaceLinkMenuTargets: document.querySelector("#workspaceLinkMenuTargets"),
  idfInput: document.querySelector("#idfInput"),
  syncRawTextToggle: document.querySelector("#syncRawTextToggle"),
  textStats: document.querySelector("#textStats"),
  inputFilter: document.querySelector("#inputFilter"),
  inputFilterStats: document.querySelector("#inputFilterStats"),
  fieldStats: document.querySelector("#fieldStats"),
  semanticEditor: document.querySelector("#semanticEditor"),
  textObjectView: document.querySelector("#textObjectView"),
  jsonStructuredView: document.querySelector("#jsonStructuredView"),
  fieldTable: document.querySelector("#fieldTable"),
  tableOrientationButtons: document.querySelectorAll(".orientation-button"),
  workspace: document.querySelector(".workspace"),
  workspaceSplitter: document.querySelector("#workspaceSplitter"),
  layoutPresetButtons: document.querySelectorAll("[data-layout-preset]"),
  editorPanel: document.querySelector(".editor-panel"),
  inputRawSplitter: document.querySelector("#inputRawSplitter"),
  inputViewButtons: document.querySelectorAll(".view-tab"),
  semanticRevealIndicator: document.querySelector("#semanticRevealIndicator"),
  inputViews: document.querySelectorAll(".input-view"),
  analysisPanel: document.querySelector(".analysis-panel"),
  resultTabButtons: document.querySelectorAll("[data-result-tab]"),
  resultPanes: document.querySelectorAll(".result-pane"),
  profileApplyButton: document.querySelector("#profileApplyButton"),
  profileSettings: document.querySelector("#profileSettings"),
  profileOverview: document.querySelector("#profileOverview"),
  profileGraph: document.querySelector("#profileGraph"),
  profileApplyDialog: document.querySelector("#profileApplyDialog"),
  profileApplyForm: document.querySelector("#profileApplyForm"),
  profileApplyClose: document.querySelector("#profileApplyClose"),
  profileApplyBody: document.querySelector("#profileApplyBody"),
  profilePreviewApply: document.querySelector("#profilePreviewApply"),
  profileConfirmApply: document.querySelector("#profileConfirmApply"),
  profileApplyStatus: document.querySelector("#profileApplyStatus"),
  hvacViewportActions: document.querySelector("#hvacViewportActions"),
  hvacBackButton: document.querySelector("#hvacBackButton"),
  hvacForwardButton: document.querySelector("#hvacForwardButton"),
  hvacClearFocusButton: document.querySelector("#hvacClearFocusButton"),
  hvacZoneServicesButton: document.querySelector("#hvacZoneServicesButton"),
  hvacExpandButton: document.querySelector("#hvacExpandButton"),
  hvacSummary: document.querySelector("#hvacSummary"),
  hvacGraph: document.querySelector("#hvacGraph"),
  hvacInspectorStats: document.querySelector("#hvacInspectorStats"),
  hvacInspector: document.querySelector("#hvacInspector"),
  hvacApplyDialog: document.querySelector("#hvacApplyDialog"),
  hvacApplyForm: document.querySelector("#hvacApplyForm"),
  hvacApplyClose: document.querySelector("#hvacApplyClose"),
  hvacApplyBody: document.querySelector("#hvacApplyBody"),
  hvacPreviewApply: document.querySelector("#hvacPreviewApply"),
  hvacConfirmApply: document.querySelector("#hvacConfirmApply"),
  hvacApplyStatus: document.querySelector("#hvacApplyStatus"),
  simulationRunButton: document.querySelector("#simulationRunButton"),
  simulationWeatherSelect: document.querySelector("#simulationWeatherSelect"),
  simulationPurposeInputs: document.querySelectorAll("[data-simulation-purpose]"),
  simulationResultTabs: document.querySelector("#simulationResultTabs"),
  simulationEnergyStats: document.querySelector("#simulationEnergyStats"),
  simulationEnergyDashboard: document.querySelector("#simulationEnergyDashboard"),
  simulationHVACLoopStats: document.querySelector("#simulationHVACLoopStats"),
  simulationHVACLoopResults: document.querySelector("#simulationHVACLoopResults"),
  simulationComfortStats: document.querySelector("#simulationComfortStats"),
  simulationComfortResults: document.querySelector("#simulationComfortResults"),
  simulationStatus: document.querySelector("#simulationStatus"),
  simulationPercent: document.querySelector("#simulationPercent"),
  simulationProgressBar: document.querySelector("#simulationProgressBar"),
  simulationHeatFlowStats: document.querySelector("#simulationHeatFlowStats"),
  simulationHeatFlowPlay: document.querySelector("#simulationHeatFlowPlay"),
  simulationHeatFlowSlider: document.querySelector("#simulationHeatFlowSlider"),
  simulationHeatFlowFrame: document.querySelector("#simulationHeatFlowFrame"),
  simulationHeatFlowStory: document.querySelector("#simulationHeatFlowStory"),
  simulationHeatFlowRangeStart: document.querySelector("#simulationHeatFlowRangeStart"),
  simulationHeatFlowRangeEnd: document.querySelector("#simulationHeatFlowRangeEnd"),
  simulationHeatFlowSpeed: document.querySelector("#simulationHeatFlowSpeed"),
  simulationHeatFlowOverlay: document.querySelector("#simulationHeatFlowOverlay"),
  simulationHeatFlow: document.querySelector("#simulationHeatFlow"),
  simulationSeriesStats: document.querySelector("#simulationSeriesStats"),
  simulationSeriesSelect: document.querySelector("#simulationSeriesSelect"),
  simulationSeriesGroup: document.querySelector("#simulationSeriesGroup"),
  simulationSeriesRangeAll: document.querySelector("#simulationSeriesRangeAll"),
  simulationSeriesRangeStart: document.querySelector("#simulationSeriesRangeStart"),
  simulationSeriesRangeEnd: document.querySelector("#simulationSeriesRangeEnd"),
  simulationSeriesRangeLabel: document.querySelector("#simulationSeriesRangeLabel"),
  simulationChart: document.querySelector("#simulationChart"),
  simulationFilesStats: document.querySelector("#simulationFilesStats"),
  simulationExportPurposeJSON: document.querySelector("#simulationExportPurposeJSON"),
  simulationExportPurposeHTML: document.querySelector("#simulationExportPurposeHTML"),
  simulationFiles: document.querySelector("#simulationFiles"),
  metricCategories: document.querySelector("#metricCategories"),
  exportMetricsJSONButton: document.querySelector("#exportMetricsJSONButton"),
  exportMetricsCSVButton: document.querySelector("#exportMetricsCSVButton"),
  topologyStats: document.querySelector("#topologyStats"),
  topologyViewport: document.querySelector("#topologyViewport"),
  topologyBody: document.querySelector(".topology-body"),
  topologyDetailsSplitter: document.querySelector("#topologyDetailsSplitter"),
  topology3DCanvasHost: document.querySelector("#topology3DCanvasHost"),
  topologyPlan: document.querySelector("#topologyPlan"),
  thermalTopologyView: document.querySelector("#thermalTopologyView"),
  thermalTopologyGraph: document.querySelector("#thermalTopologyGraph"),
  thermalTopologyInspector: document.querySelector("#thermalTopologyInspector"),
  topologyDetails: document.querySelector("#topologyDetails"),
  topologyModeButtons: document.querySelectorAll("[data-topology-mode]"),
  topologyStoryControl: document.querySelector("#topologyStoryControl"),
  topologyStorySelect: document.querySelector("#topologyStorySelect"),
  topologySpatialControls: document.querySelector("#topologySpatialControls"),
  thermalTopologyControls: document.querySelector("#thermalTopologyControls"),
  thermalTopologyMetric: document.querySelector("#thermalTopologyMetric"),
  thermalTopologyLayout: document.querySelector("#thermalTopologyLayout"),
  topologyFitButton: document.querySelector("#topologyFitButton"),
  topologyExpandButton: document.querySelector("#topologyExpandButton"),
  topologySyncLocate: document.querySelector("#topologySyncLocate"),
  topologyShowZones: document.querySelector("#topologyShowZones"),
  topologyShowSurfaces: document.querySelector("#topologyShowSurfaces"),
  topologyShowOpenings: document.querySelector("#topologyShowOpenings"),
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
  refreshStatusTitle();
}

export function refreshStatusTitle() {
  if (!elements.runtimeStatus) {
    return;
  }
  const details = [];
  const analysis = formatAnalysisTiming();
  if (analysis) {
    details.push(analysis);
  }
  const render = state.renderTiming?.last;
  if (render?.tab) {
    details.push(`Last render: ${render.tab} ${formatMS(render.ms)}`);
  }
  elements.runtimeStatus.title = details.join("\n");
}

function formatAnalysisTiming() {
  const timing = state.analysisTiming;
  const stageTimings = state.analysisStageTimings || timing?.stages || {};
  if (!timing && !Object.keys(stageTimings).length) {
    return "";
  }
  const mode = timing?.mode ? ` ${timing.mode}` : "";
  const cache = timing?.cacheHit ? " (cache hit)" : "";
  const parts = [`Analysis${mode}${cache}`];
  [
    ["total", timing?.totalMs],
    ["queue", timing?.queueWaitMs],
    ["parse", timing?.parseMs],
    ["analyze", timing?.analyzeMs],
    ["semantic", timing?.semanticMs],
    ["epjson", timing?.epjsonMs],
  ].forEach(([label, value]) => {
    if (Number.isFinite(Number(value)) && Number(value) > 0) {
      parts.push(`${label} ${formatMS(value)}`);
    }
  });
  const stages = Object.entries(stageTimings)
    .filter(([, value]) => Number.isFinite(Number(value)) && Number(value) >= 0)
    .sort((a, b) => String(a[0]).localeCompare(String(b[0])))
    .slice(0, 8)
    .map(([label, value]) => `${label} ${formatMS(value)}`);
  if (stages.length) {
    parts.push(`stages ${stages.join(", ")}`);
  }
  return parts.join(" | ");
}

function formatMS(value) {
  const numeric = Number(value) || 0;
  return numeric >= 100 ? `${Math.round(numeric)} ms` : `${numeric.toFixed(1)} ms`;
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
