import { defaultSample, loadDefaultSampleIDF } from "./sample.js";
import { bundledAppInfo } from "./app-info.js";
import { loadAndApplyAppSettings } from "./settings-client.js";
import {
  backend,
  elements,
  getDocumentText,
  normalizeThermalTopologyLayout,
  normalizeThermalTopologyMetric,
  resetThermalTopologyDocumentState,
  setStatus,
  setDocumentText,
  state,
} from "./state.js";
import {
  analyze,
  applyCachedAnalysisResult,
  exportMetrics,
  loadBrowserFile,
  openGuide,
  openInputFile,
  openSettings,
  openTools,
  prioritizeAnalysisStageForTab,
  currentDocumentStorageKey,
  registerLoadedDocument,
  revertToLoadedDocument,
  saveInputFile,
  scheduleAnalyzeAfterPaint,
  updateDocumentActions,
} from "./actions.js";
import { markAnalysisDirty, renderEmpty, renderReport } from "./views/analysis-views.js";
import { fitTopologyView, renderTopology, resizeTopology, setTopologyMode, setTopologyStory } from "./topology-loader.js";
import { initializeHVACControls } from "./views/hvac-views.js";
import {
  configureInputViews,
  setInputFilter,
  setTableOrientation,
  switchInputView,
} from "./views/input-views.js";
import { initializeVerticalSplitters, initializeWorkspaceSplitter, restoreWorkspaceLayout } from "./layout.js";
import {
  focusInputObject,
  handleAnalysisActivation,
  handleInputSelectionActivation,
  handleInputJumpActivation,
  initializeLegacyInputNavigationAdapters,
  jumpInputDefinition,
  jumpInputReferences,
  redoViewNavigation,
  refreshInputSelectionStyles,
  switchResultTab,
  restoreViewSnapshot,
  undoViewNavigation,
} from "./navigation.js";
import {
  clearSemanticSelection,
  configureSelectionController,
  openSelectionInView,
  remapSemanticSelection,
  revealSelectionSource,
  resumePendingSemanticNavigation,
  isProfileTopologyLink,
  selectionTargetsForView,
} from "./selection-controller.js";
import { initializeResultPanelNavigationAdapters } from "./panel-navigation-adapters.js";
import { PANEL_NAVIGATION_VIEW_IDS } from "./panel-navigation-registry.js";
import { chooseSemanticOccurrence, chooseViewTarget } from "./navigation-chooser.js";
import { initializePanelNavigationActions } from "./panel-navigation-actions.js";
import {
  closeCommandPalette,
  initializeCommandPalette,
  openAvailableViewsPalette,
  openCommandPalette,
} from "./command-palette.js";
import { captureViewSnapshot, recordViewHistory } from "./view-history.js";
import { initializeProfileControls, renderProfile } from "./views/profile-views.js";
import { initializeSimulationControls, loadSimulationEnvironment } from "./views/simulation-views.js";
import { normalizeAnalyzeTabOrder, t, translatePage } from "./i18n.js";
import { initializeKeyboardShortcuts } from "./shortcuts.js";
import { getSemanticNavigationCache } from "./semantic-navigation-cache.js";

loadAndApplyAppSettings().then((result) => applyRuntimeSettings(result.settings));

function clearAuxiliaryNavigationMarker() {
  try {
    window.sessionStorage.removeItem("idfAnalyzer.auxiliaryNavigation");
  } catch {
    // Main remains usable when browser storage is unavailable.
  }
}

clearAuxiliaryNavigationMarker();
window.addEventListener("pageshow", (event) => {
  if (event.persisted) {
    clearAuxiliaryNavigationMarker();
  }
});

configureInputViews({ analyze, renderReport });
initializeLegacyInputNavigationAdapters();
initializeResultPanelNavigationAdapters();
initializePanelNavigationActions({
  jumpDefinition: jumpInputDefinition,
  jumpReferences: jumpInputReferences,
});
configureSelectionController({
  state,
  getNavigationIndex: () => state.semanticProjection?.navigation || {},
  getCurrentText: getDocumentText,
  getReportAnalysisKey: () => state.reportAnalysisKey || "",
  isAnalysisCurrent: () => (
    state.reportAnalyzedText !== "" && state.reportAnalyzedText === getDocumentText()
  ),
  getActiveInputView: () => `input-${state.activeInputView || "semantic"}`,
  getActivePanelView: () => state.activeResultTab || "metrics",
  getCurrentSemanticContext: () => ({
    occurrenceId: state.semanticCurrentOccurrenceId || "",
    path: state.semanticCurrentPath || "",
  }),
  chooseSemanticOccurrence,
  chooseViewTarget,
  recordHistory: (payload = {}) => {
    const snapshot = captureViewSnapshot();
    if (payload.previous) {
      snapshot.globalSelection = payload.previous;
    }
    recordViewHistory(snapshot);
  },
  openView: async (view, options = {}) => {
    if (String(view).startsWith("input-")) {
      await switchInputView(String(view).slice("input-".length), { ...options, recordHistory: false, revealSelection: false });
      return;
    }
    switchResultTab(view, { ...options, recordHistory: false });
  },
  queueAnalysisTarget: ({ view }) => {
    if (view && !String(view).startsWith("input-")) {
      prioritizeAnalysisStageForTab(view);
    }
  },
  onAnalysisPending: () => {
    setStatus(t("status.navigationAnalysisPending", {}, "Analysis pending; navigation target will be restored when ready."), "muted");
  },
  onSelectionChange: ({ selection, options, temporaryRevealCleared }) => {
    const objectIndex = selection.sourceAnchor?.objectIndex;
    state.semanticSelectedObjectIndex = objectIndex === undefined || objectIndex === null ? "" : String(objectIndex);
    window.dispatchEvent(new CustomEvent("idfAnalyzer:semanticSelectionChanged", { detail: { selection, options, temporaryRevealCleared } }));
    refreshInputSelectionStyles(selection);
  },
  onHoverChange: ({ hover, options }) => {
    window.dispatchEvent(new CustomEvent("idfAnalyzer:semanticHoverChanged", { detail: { hover, options } }));
  },
  onTemporaryReveal: ({ reveal, options }) => {
    window.dispatchEvent(new CustomEvent("idfAnalyzer:semanticTemporaryReveal", { detail: { reveal, options } }));
  },
  onChooserRequested: (detail) => {
    window.dispatchEvent(new CustomEvent("idfAnalyzer:semanticChooserRequested", { detail }));
  },
  onSelectionRemapped: (detail) => {
    window.dispatchEvent(new CustomEvent("idfAnalyzer:semanticSelectionRemapped", { detail }));
    if (detail.reason === "source" && detail.previous?.entityId !== detail.selection?.entityId) {
      setStatus(t("semantic.selectionMovedAfterRename", {}, "Selection moved to the renamed entity."), "ok");
    } else if (detail.reason === "parent") {
      setStatus(t("semantic.selectionMovedToParent", {}, "The selected item no longer exists; selected its nearest parent."), "warn");
    } else if (detail.reason === "missing") {
      setStatus(t("semantic.selectionClearedAfterEdit", {}, "The selected item no longer exists; selection was cleared."), "warn");
    }
  },
});
const resumePendingNavigationAfterRender = (event) => {
  if (event.type !== "idfAnalyzer:analysisComplete") {
    return;
  }
  prewarmSemanticNavigationCache();
  const eventText = event.detail?.text;
  const eventAnalysisKey = event.detail?.analysisKey || "";
  const currentText = getDocumentText();
  if (eventText && eventText !== currentText) {
    return;
  }
  window.requestAnimationFrame(async () => {
    const latestText = getDocumentText();
    if (
      (eventText && eventText !== latestText) ||
      (eventText && state.reportAnalyzedText !== eventText) ||
      (eventAnalysisKey && state.reportAnalysisKey && eventAnalysisKey !== state.reportAnalysisKey)
    ) {
      return;
    }
    const editRestore = state.semanticEditSelectionRestore;
    await remapSemanticSelection({
      recordHistory: false,
      allowRenamedSourceIndex: Boolean(
        editRestore && editRestore.objectCount === (state.report?.objects?.length || 0),
      ),
    });
    await resumePendingSemanticNavigation({ recordHistory: false });
    await restorePendingWorkspaceContext(eventAnalysisKey);
  });
};

function prewarmSemanticNavigationCache() {
  if (!state.semanticProjection?.navigation) {
    return;
  }
  getSemanticNavigationCache(state.semanticProjection, {
    textHash: state.reportAnalysisKey || state.lastAnalyzedKey || state.analysisKey || "",
    analyzerVersion: bundledAppInfo.version,
    schemaVersion: state.semanticProjection.schema || "",
  });
}
window.addEventListener("idfAnalyzer:analysisComplete", resumePendingNavigationAfterRender);

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
elements.exportMetricsJSONButton.addEventListener("click", () => exportMetrics("json"));
elements.exportMetricsCSVButton.addEventListener("click", () => exportMetrics("csv"));
elements.toolsButton.addEventListener("click", openTools);
elements.guideButton.addEventListener("click", openGuide);
elements.settingsButton.addEventListener("click", openSettings);
elements.inputFilter.addEventListener("input", () => setInputFilter(elements.inputFilter.value));
bindHorizontalDragScroll(elements.inputToolbarScroll);
elements.resultTabButtons.forEach((button) => {
  button.addEventListener("click", () => {
    state.resultTabManuallySelected = true;
    switchResultTab(button.dataset.resultTab);
  });
});
elements.topologyModeButtons.forEach((button) => {
  button.addEventListener("click", () => setTopologyMode(button.dataset.topologyMode));
});
elements.topologyStorySelect.addEventListener("change", () => setTopologyStory(elements.topologyStorySelect.value));
elements.thermalTopologyMetric.addEventListener("change", () => {
  updateThermalTopologySetting("thermalTopologyMetric", normalizeThermalTopologyMetric(elements.thermalTopologyMetric.value));
});
elements.thermalTopologyLayout.addEventListener("change", () => {
  updateThermalTopologySetting("thermalTopologyLayout", normalizeThermalTopologyLayout(elements.thermalTopologyLayout.value));
});
elements.topologyFitButton.addEventListener("click", () => void fitTopologyView());
bindTopologyVisibilityControl(elements.topologyShowZones, "zones");
bindTopologyVisibilityControl(elements.topologyShowSurfaces, "surfaces");
bindTopologyVisibilityControl(elements.topologyShowOpenings, "openings");
elements.hvacExpandButton.addEventListener("click", () => toggleExpandedPane("hvac"));
elements.topologyExpandButton.addEventListener("click", () => toggleExpandedPane("topology"));

function bindTopologyVisibilityControl(control, optionKey) {
  control.addEventListener("change", () => {
    state.topologyVisibility = {
      ...(state.topologyVisibility || {}),
      [optionKey]: control.checked,
    };
    renderTopology();
  });
}

function updateThermalTopologySetting(key, value) {
  if (state[key] === value) {
    return;
  }
  recordViewHistory();
  state[key] = value;
  renderTopology();
}

function activateTopologyModeShortcut(mode) {
  if (state.activeResultTab !== "topology") return false;
  setTopologyMode(mode);
  return true;
}

function activateTopologyFitShortcut() {
  if (state.activeResultTab !== "topology") return false;
  void fitTopologyView();
  return true;
}

function activateThermalTopologySettingShortcut(key, value) {
  if (state.activeResultTab !== "topology" || state.topologyMode !== "thermal") return false;
  updateThermalTopologySetting(key, value);
  return true;
}

elements.inputViewButtons.forEach((button, index, buttons) => {
  button.addEventListener("click", async () => {
    await switchInputView(button.dataset.inputView);
  });
  button.addEventListener("keydown", async (event) => {
    const key = event.key;
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(key)) {
      return;
    }
    event.preventDefault();
    const targetIndex = key === "Home"
      ? 0
      : key === "End"
        ? buttons.length - 1
        : (index + (key === "ArrowRight" ? 1 : -1) + buttons.length) % buttons.length;
    const target = buttons[targetIndex];
    await switchInputView(target.dataset.inputView);
    target.focus();
  });
});
window.addEventListener("idfAnalyzer:inputViewChanged", () => {
  refreshInputSelectionStyles(state.globalSelection);
});
elements.editorPanel.addEventListener("click", (event) => {
  if (handleInputJumpActivation(event.target)) {
    event.preventDefault();
    return;
  }
  handleInputSelectionActivation(event.target);
});
window.addEventListener("resize", () => {
  if (state.activeResultTab === "topology" || state.expandedPane === "topology") {
    resizeTopology();
  }
});
window.addEventListener("keydown", (event) => {
  if (handleAnalysisTabCycleKey(event) || handleHardwareHistoryKey(event)) {
    return;
  }
  if (event.key === "Escape" && state.expandedPane) {
    event.preventDefault();
    toggleExpandedPane("");
  }
});
window.addEventListener("mousedown", handleHardwareHistoryMouseButton, { capture: true });
window.addEventListener("auxclick", handleHardwareHistoryMouseButton, { capture: true });
window.addEventListener("idfAnalyzer:documentChanged", () => {
  resetThermalTopologyDocumentState(state);
  updateDocumentActions();
});
window.addEventListener("idfAnalyzer:topologyLocate", (event) => {
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
  if (event.defaultPrevented) {
    return;
  }
  const isLocalActivationKey = event.key === "Enter" || event.key === " ";
  if (!isLocalActivationKey || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) {
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
  setDocumentText(result.text);
  state.report = result.report;
  state.model = result.model || null;
  state.epjsonText = result.epjson || "";
  state.semanticProjection = result.semantic || null;
  state.lastAnalyzedText = result.text;
  state.analysisKey = result.analysisKey || "";
  state.lastAnalyzedKey = state.analysisKey;
  state.reportAnalyzedText = result.text;
  state.reportAnalysisKey = state.analysisKey;
  state.analysisStage = "complete";
  state.geometryReady = true;
  markInstalledAnalysisReady();
  renderReport();
  dispatchInstalledAnalysisComplete(result);
  updateDocumentActions();
  const changeCount = result.preview?.changes?.length || 0;
  setStatus(t("status.profileApplied", { count: changeCount }), "ok");
});
window.addEventListener("idfAnalyzer:hvacApplied", (event) => {
  const result = event.detail || {};
  if (!result.text || !result.report) {
    return;
  }
  setDocumentText(result.text);
  state.report = result.report;
  state.model = result.model || null;
  state.epjsonText = result.epjson || "";
  state.semanticProjection = result.semantic || null;
  state.lastAnalyzedText = result.text;
  state.analysisKey = result.analysisKey || "";
  state.lastAnalyzedKey = state.analysisKey;
  state.reportAnalyzedText = result.text;
  state.reportAnalysisKey = state.analysisKey;
  state.analysisStage = "complete";
  state.geometryReady = true;
  markInstalledAnalysisReady();
  renderReport();
  dispatchInstalledAnalysisComplete(result);
  updateDocumentActions();
  const changeCount = result.preview?.changes?.filter((change) => change.requiresSave).length || 0;
  setStatus(t("status.hvacApplied", { count: changeCount }), "ok");
});
function dispatchInstalledAnalysisComplete(result = {}) {
  window.dispatchEvent(new CustomEvent("idfAnalyzer:analysisComplete", {
    detail: {
      text: result.text || state.reportAnalyzedText || "",
      analysisKey: result.analysisKey || state.reportAnalysisKey || "",
      stage: "complete",
    },
  }));
}

function markInstalledAnalysisReady() {
  Object.keys(state.analysisReady || {}).forEach((view) => {
    state.analysisReady[view] = true;
  });
  state.reportAnalysisStage = state.analysisStage || "complete";
  state.reportAnalysisReady = { ...(state.analysisReady || {}) };
  state.reportGeometryReady = Boolean(state.geometryReady);
}

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
  if (state.expandedPane === "topology" || pane === "topology") {
    window.requestAnimationFrame(resizeTopology);
  }
}

function updateExpandButtons() {
  updateExpandButton(elements.hvacExpandButton, "hvac");
  updateExpandButton(elements.topologyExpandButton, "topology");
}

function updateExpandButton(button, pane) {
  if (!button) return;
  const active = state.expandedPane === pane;
  const label = active ? t("action.close") : t("action.expand", {}, "Expand");
  button.classList.toggle("active", active);
  button.setAttribute("aria-pressed", String(active));
  button.setAttribute("aria-label", label);
  button.title = label;
  const assistiveLabel = button.querySelector(".sr-only");
  if (assistiveLabel) assistiveLabel.textContent = label;
}

function isEditableTarget(target) {
  return Boolean(target?.closest?.("input, textarea, select, [contenteditable='true']"));
}

function isEditorPanelTarget(target) {
  return Boolean(target?.closest?.(".editor-panel"));
}

function handleUndoShortcut(event) {
  if (isEditableTarget(event?.target)) {
    return false;
  }
  undoViewNavigation();
  return true;
}

function handleRedoShortcut(event) {
  if (isEditableTarget(event?.target)) {
    return false;
  }
  redoViewNavigation();
  return true;
}

function handleAnalysisTabCycleKey(event) {
  if (isEditableTarget(event.target) || isEditorPanelTarget(event.target)) {
    return false;
  }
  if (!(event.ctrlKey || event.metaKey) || event.altKey || (event.key !== "PageUp" && event.key !== "PageDown")) {
    return false;
  }
  event.preventDefault();
  switchResultTabByOffset(event.key === "PageUp" ? -1 : 1);
  return true;
}

function switchResultTabByOffset(offset) {
  const tabButtons = [...(elements.resultTabButtons || [])].filter((button) => button.dataset.resultTab);
  if (!tabButtons.length) {
    return;
  }
  const currentIndex = Math.max(0, tabButtons.findIndex((button) => button.dataset.resultTab === state.activeResultTab));
  const nextIndex = (currentIndex + offset + tabButtons.length) % tabButtons.length;
  const nextTab = tabButtons[nextIndex].dataset.resultTab;
  state.resultTabManuallySelected = true;
  switchResultTab(nextTab);
  tabButtons[nextIndex].focus?.({ preventScroll: true });
}

function handleHardwareHistoryKey(event) {
  if (isEditableTarget(event.target)) {
    return false;
  }
  const isBack = event.key === "BrowserBack" || (event.altKey && event.key === "ArrowLeft" && !event.ctrlKey && !event.metaKey && !event.shiftKey);
  const isForward =
    event.key === "BrowserForward" || (event.altKey && event.key === "ArrowRight" && !event.ctrlKey && !event.metaKey && !event.shiftKey);
  if (!isBack && !isForward) {
    return false;
  }
  event.preventDefault();
  if (isBack) {
    undoViewNavigation();
  } else {
    redoViewNavigation();
  }
  return true;
}

function handleHardwareHistoryMouseButton(event) {
  if ((event.button !== 3 && event.button !== 4) || isEditableTarget(event.target)) {
    return false;
  }
  event.preventDefault();
  event.stopPropagation();
  if (event.type !== "auxclick") {
    if (event.button === 3) {
      undoViewNavigation();
    } else {
      redoViewNavigation();
    }
  }
  return true;
}

async function revealCurrentSelectionSource() {
  if (!state.globalSelection?.entityId) {
    setStatus(t("semantic.noAvailableView", {}, "No selection to reveal"), "warn");
    return false;
  }
  return revealSelectionSource({
    originView: state.activeResultTab || `input-${state.activeInputView}`,
    action: "reveal_source",
    preserveFilters: true,
  });
}

function focusNextWorkspacePane() {
  const panes = [
    elements.editorPanel,
    elements.analysisPanel,
  ].filter(Boolean);
  if (!panes.length) {
    return false;
  }
  const current = panes.findIndex((pane) => pane.contains(document.activeElement));
  const next = panes[(current + 1 + panes.length) % panes.length];
  if (!next.hasAttribute("tabindex")) {
    next.setAttribute("tabindex", "-1");
  }
  next.focus({ preventScroll: true });
  return true;
}

function focusCurrentViewSearch() {
  const inAnalysis = elements.analysisPanel?.contains(document.activeElement);
  const root = inAnalysis
    ? elements.analysisPanel?.querySelector(".result-pane.active")
    : document.querySelector(`#${state.activeInputView}InputView`)?.parentElement || elements.editorPanel;
  const search = root?.querySelector?.('input[type="search"]') || (inAnalysis ? null : elements.inputFilter);
  if (!search) {
    setStatus(t("navigation.noSearch", {}, "This view has no search field"), "warn");
    return false;
  }
  search.focus();
  search.select?.();
  return true;
}

async function primaryOpenFromFocus() {
  const active = document.activeElement;
  const panelTarget = active?.closest?.("[data-entity-id], [data-panel-target-id], [data-source-object-index], .navigable-row");
  if (elements.analysisPanel?.contains(active) && panelTarget) {
    handleAnalysisActivation(panelTarget);
    return true;
  }
  if (active?.matches?.("button, a[href]")) {
    active.click();
    return true;
  }
  return openAvailableViewsForSelection();
}

async function openAvailableViewsForSelection() {
  const selection = state.globalSelection;
  if (!selection?.entityId) {
    setStatus(t("semantic.noAvailableView", {}, "No available view can reveal this selection"), "warn");
    return false;
  }
  const items = [];
  const originView = state.activeResultTab || selection.originView;
  for (const viewID of PANEL_NAVIGATION_VIEW_IDS) {
    if (viewID !== "input-semantic" && viewID.startsWith("input-")) {
      continue;
    }
    if (isProfileTopologyLink(originView, viewID)) {
      continue;
    }
    const targets = selectionTargetsForView(viewID, { ...selection, originView });
    if (viewID !== "input-semantic" && !targets.length) {
      continue;
    }
    items.push({
      id: viewID,
      label: navigationViewLabel(viewID),
      meta: targets.length > 1 ? t("semantic.occurrences", { count: targets.length }, `${targets.length} targets`) : "",
      run: () => openSelectionInView(viewID, {
        originView,
        action: "open",
        preserveFilters: true,
      }),
    });
  }
  if (selection.sourceAnchor) {
    items.push({
      id: "source",
      label: t("semantic.revealSource", {}, "Reveal source"),
      run: revealCurrentSelectionSource,
    });
  }
  if (!items.length) {
    setStatus(t("semantic.noAvailableView", {}, "No available view can reveal this selection"), "warn");
    return false;
  }
  return openAvailableViewsPalette(items);
}

async function clearSelectionOrTransientUI() {
  if (closeCommandPalette()) {
    return true;
  }
  const occurrenceChooser = document.querySelector("[data-semantic-occurrence-chooser]:not([hidden])");
  if (occurrenceChooser) {
    occurrenceChooser.hidden = true;
    return true;
  }
  if (!state.globalSelection?.entityId) {
    return false;
  }
  await clearSemanticSelection({ action: "clear_selection", recordHistory: false, follow: false });
  return true;
}

function navigationViewLabel(viewID) {
  if (viewID === "input-semantic") {
    return t("input.semantic", {}, "Semantic");
  }
  return t(`tab.${viewID}`, {}, viewID[0].toUpperCase() + viewID.slice(1));
}

function commandPaletteItems() {
  const shortcuts = state.keyboardShortcuts || {};
  return [
    ["revealSource", t("shortcut.revealSource", {}, "Reveal source"), revealCurrentSelectionSource],
    ["availableViews", t("shortcut.availableViews", {}, "Available views"), openAvailableViewsForSelection],
    ["undoView", t("shortcut.undoView", {}, "Back"), () => undoViewNavigation()],
    ["redoView", t("shortcut.redoView", {}, "Forward"), () => redoViewNavigation()],
    ["paneFocus", t("shortcut.paneFocus", {}, "Cycle pane focus"), focusNextWorkspacePane],
    ["currentSearch", t("shortcut.currentSearch", {}, "Search current view"), focusCurrentViewSearch],
    ["clearSelection", t("shortcut.clearSelection", {}, "Clear selection"), clearSelectionOrTransientUI],
  ].map(([id, label, run]) => ({ id, label, shortcut: shortcuts[id] || "", run }));
}

initializeWorkspaceSplitter();
initializeVerticalSplitters();
initializeProfileControls();
initializeHVACControls();
initializeSimulationControls();
initializeCommandPalette(commandPaletteItems);
initializeKeyboardShortcuts({
  save: saveInputFile,
  open: openInputFile,
  undoView: handleUndoShortcut,
  redoView: handleRedoShortcut,
  jumpDefinition: jumpInputDefinition,
  jumpReferences: jumpInputReferences,
  commandPalette: openCommandPalette,
  revealSource: revealCurrentSelectionSource,
  paneFocus: focusNextWorkspacePane,
  currentSearch: focusCurrentViewSearch,
  primaryOpen: primaryOpenFromFocus,
  availableViews: openAvailableViewsForSelection,
  clearSelection: clearSelectionOrTransientUI,
  switchInputView,
  switchResultTab,
  setTopologyMode: activateTopologyModeShortcut,
  fitTopology: activateTopologyFitShortcut,
  setTopologyMetric: (metric) => activateThermalTopologySettingShortcut("thermalTopologyMetric", normalizeThermalTopologyMetric(metric)),
});
renderEmpty();
updateDocumentActions();
const restoredDocument = restoreCurrentDocument();
if (restoredDocument) {
  setDocumentText(restoredDocument.text || "");
  registerLoadedDocument(getDocumentText(), {
    path: restoredDocument.path || "",
    filename: restoredDocument.filename || "",
  });
  state.loadedText = typeof restoredDocument.loadedText === "string" ? restoredDocument.loadedText : state.loadedText;
  state.savedText = typeof restoredDocument.savedText === "string" ? restoredDocument.savedText : state.savedText;
  restoreWorkspaceLayout(restoredDocument.layout || {});
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
    setDocumentText(sampleText);
    const loadedText = getDocumentText();
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
        await restoreSavedWorkspaceContext(restoredDocument);
        setStatus(t("status.loadedNamed", { name: label }), "ok");
        return;
      }
    } catch {
      // Fall through to normal analysis if the in-memory backend cache is unavailable.
    }
  }
  state.pendingWorkspaceRestore = restoredDocument;
  scheduleAnalyzeAfterPaint({
    loadingMessage: t("status.analyzingNamed", { name: label }),
    queuedMessage: t("status.loadedQueued", { name: label }),
    statusMessage: t("status.loadedNamed", { name: label }),
    textSnapshot: getDocumentText(),
    analysisKey: restoredDocument.analysisKey || "",
    preferCache: Boolean(restoredDocument.analysisKey),
  });
}

async function restorePendingWorkspaceContext(analysisKey = "") {
  const pending = state.pendingWorkspaceRestore;
  if (!pending) {
    return false;
  }
  if (pending.text && pending.text !== getDocumentText()) {
    return false;
  }
  if (analysisKey && pending.analysisKey && analysisKey !== pending.analysisKey) {
    return false;
  }
  state.pendingWorkspaceRestore = null;
  await restoreSavedWorkspaceContext(pending);
  return true;
}

async function restoreSavedWorkspaceContext(restoredDocument = {}) {
  const snapshot = restoredDocument.viewSnapshot || {
    inputView: restoredDocument.activeInputView,
    resultTab: restoredDocument.activeResultTab,
    globalSelection: restoredDocument.globalSelection || null,
    semanticCurrentOccurrenceId: restoredDocument.semanticOccurrenceId || "",
    panelContexts: restoredDocument.panelContexts || {},
  };
  restoreWorkspaceLayout(restoredDocument.layout || {});
  await restoreViewSnapshot(snapshot, { recordHistory: false, quiet: true });
}

function restoreCurrentDocument() {
  try {
    const raw = window.sessionStorage.getItem(currentDocumentStorageKey);
    if (!raw) {
      return null;
    }
    const documentState = JSON.parse(raw);
    const hasCurrentSnapshot = Number(documentState?.schemaVersion) >= 3;
    const hasLegacyDocument = typeof documentState?.text === "string" && Boolean(documentState.text.trim());
    return typeof documentState?.text === "string" && (hasCurrentSnapshot || hasLegacyDocument) ? documentState : null;
  } catch {
    return null;
  }
}

function applyRuntimeSettings(settings) {
  if (!settings) {
    return;
  }
  state.simulationAutoRunOnOpen = settings.simulation?.autoRunOnOpen ?? state.simulationAutoRunOnOpen;
  loadSimulationEnvironment();
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
    if (state.report) {
      renderReport();
    } else {
      renderEmpty();
    }
  }
  if (state.report?.geometry && state.activeResultTab === "topology") {
    renderTopology();
  }
}

function applyDefaultResultTab(orderInput) {
  const [firstTab] = normalizeAnalyzeTabOrder(orderInput);
  if (!firstTab) {
    return;
  }
  if (!state.resultTabManuallySelected && state.activeResultTab !== firstTab) {
    switchResultTab(firstTab, { recordHistory: false });
  }
}

function bindHorizontalDragScroll(element) {
  if (!element) return;
  let pointerID = null;
  let startX = 0;
  let startScrollLeft = 0;
  let dragged = false;
  let suppressClick = false;

  element.addEventListener("pointerdown", (event) => {
    if (event.button !== 0 || event.target instanceof HTMLInputElement) return;
    pointerID = event.pointerId;
    startX = event.clientX;
    startScrollLeft = element.scrollLeft;
    dragged = false;
  });
  element.addEventListener("pointermove", (event) => {
    if (event.pointerId !== pointerID) return;
    const delta = event.clientX - startX;
    if (Math.abs(delta) > 4) {
      dragged = true;
      element.classList.add("is-dragging");
      if (!element.hasPointerCapture(pointerID)) {
        element.setPointerCapture(pointerID);
      }
    }
    if (dragged) {
      element.scrollLeft = startScrollLeft - delta;
      event.preventDefault();
    }
  });
  const finishDrag = (event) => {
    if (event.pointerId !== pointerID) return;
    suppressClick = dragged;
    pointerID = null;
    dragged = false;
    element.classList.remove("is-dragging");
  };
  element.addEventListener("pointerup", finishDrag);
  element.addEventListener("pointercancel", finishDrag);
  element.addEventListener("click", (event) => {
    if (!suppressClick) return;
    event.preventDefault();
    event.stopPropagation();
    suppressClick = false;
  }, true);
  element.addEventListener("wheel", (event) => {
    if (element.scrollWidth <= element.clientWidth || Math.abs(event.deltaY) <= Math.abs(event.deltaX)) return;
    element.scrollLeft += event.deltaY;
    event.preventDefault();
  }, { passive: false });
}
