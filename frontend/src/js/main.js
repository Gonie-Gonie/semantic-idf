import { defaultSample, loadDefaultSampleIDF } from "./sample.js";
import { loadAndApplyAppSettings } from "./settings-client.js";
import {
  backend,
  elements,
  normalizeThermalTopologyLayout,
  normalizeThermalTopologyMetric,
  normalizeThermalTopologyScope,
  resetThermalTopologyDocumentState,
  setStatus,
  state,
  updateTextStats,
} from "./state.js";
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
  prioritizeAnalysisStageForTab,
  currentDocumentStorageKey,
  registerLoadedDocument,
  revertToLoadedDocument,
  saveInputFile,
  scheduleAnalyzeAfterPaint,
  scheduleAutoAnalyze,
  updateDocumentActions,
} from "./actions.js";
import { markAnalysisDirty, renderEmpty, renderReport } from "./views/analysis-views.js";
import { fitGeometryView, renderGeometry, resizeGeometry, setGeometryMode, setGeometryStory } from "./geometry-loader.js";
import { initializeDiagnoseFixes } from "./views/diagnose-fixes.js";
import { initializeHVACControls } from "./views/hvac-views.js";
import {
  configureInputViews,
  setInputFilter,
  setTableOrientation,
  switchInputView,
  syncTextViewFromRawCaret,
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
  revealSelectionInSemantic,
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
import { initializeNavigationLinkBar, renderNavigationLinkBar } from "./navigation-link-bar.js";
import { captureViewSnapshot, recordViewHistory } from "./view-history.js";
import { initializeProfileControls, renderProfile } from "./views/profile-views.js";
import { initializeSimulationControls, loadSimulationEnvironment } from "./views/simulation-views.js";
import { normalizeAnalyzeTabOrder, t, translatePage } from "./i18n.js";
import { initializeKeyboardShortcuts } from "./shortcuts.js";

loadAndApplyAppSettings().then((result) => applyRuntimeSettings(result.settings));

function updateSemanticRevealIndicator(selection = state.globalSelection) {
  if (!elements.semanticRevealIndicator) {
    return;
  }
  const available = Boolean(selection?.entityId && state.activeInputView !== "semantic");
  elements.semanticRevealIndicator.hidden = !available;
  elements.semanticRevealIndicator.setAttribute("aria-hidden", available ? "false" : "true");
}

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
  getCurrentText: () => elements.idfInput?.value || "",
  getReportAnalysisKey: () => state.reportAnalysisKey || "",
  isAnalysisCurrent: () => (
    state.reportAnalyzedText !== "" && state.reportAnalyzedText === (elements.idfInput?.value || "")
  ),
  getActiveInputView: () => `input-${state.activeInputView || "semantic"}`,
  getActivePanelView: () => state.activeResultTab || "summary",
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
    renderNavigationLinkBar();
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
    updateSemanticRevealIndicator(selection);
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
initializeNavigationLinkBar({
  openView: openSelectionInView,
  revealSource: revealSelectionSource,
  back: undoViewNavigation,
  forward: redoViewNavigation,
});

const resumePendingNavigationAfterRender = (event) => {
  if (event.type !== "idfAnalyzer:analysisComplete") {
    return;
  }
  const eventText = event.detail?.text;
  const eventAnalysisKey = event.detail?.analysisKey || "";
  const currentText = elements.idfInput?.value || "";
  if (eventText && eventText !== currentText) {
    return;
  }
  window.requestAnimationFrame(async () => {
    const latestText = elements.idfInput?.value || "";
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
elements.resultTabButtons.forEach((button) => {
  button.addEventListener("click", () => {
    state.resultTabManuallySelected = true;
    switchResultTab(button.dataset.resultTab);
    renderNavigationLinkBar();
  });
});
elements.geometryModeButtons.forEach((button) => {
  button.addEventListener("click", () => setGeometryMode(button.dataset.geometryMode));
});
elements.geometryStorySelect.addEventListener("change", () => setGeometryStory(elements.geometryStorySelect.value));
elements.thermalTopologyMetric.addEventListener("change", () => {
  updateThermalTopologySetting("thermalTopologyMetric", normalizeThermalTopologyMetric(elements.thermalTopologyMetric.value));
});
elements.thermalTopologyScope.addEventListener("change", () => {
  updateThermalTopologySetting("thermalTopologyScope", normalizeThermalTopologyScope(elements.thermalTopologyScope.value));
});
elements.thermalTopologyLayout.addEventListener("change", () => {
  updateThermalTopologySetting("thermalTopologyLayout", normalizeThermalTopologyLayout(elements.thermalTopologyLayout.value));
});
elements.thermalTopologyShowAirCoupling.addEventListener("change", () => {
  updateThermalTopologySetting("thermalTopologyShowAirCoupling", elements.thermalTopologyShowAirCoupling.checked);
});
elements.thermalTopologyExpandExternalTargets.addEventListener("change", () => {
  updateThermalTopologySetting("thermalTopologyExpandExternalTargets", elements.thermalTopologyExpandExternalTargets.checked);
});
elements.thermalTopologyExportJSON.addEventListener("click", () => {
  window.dispatchEvent(new CustomEvent("idfAnalyzer:thermalTopologyExport"));
});
elements.geometryFitButton.addEventListener("click", () => void fitGeometryView());
elements.geometrySyncLocate.addEventListener("change", () => {
  state.geometrySyncLocate = elements.geometrySyncLocate.checked;
  renderGeometry();
});
bindGeometryVisibilityControl(elements.geometry3DShowZones, "geometry3DVisibility", "zones");
bindGeometryVisibilityControl(elements.geometry3DShowSurfaces, "geometry3DVisibility", "surfaces");
bindGeometryVisibilityControl(elements.geometry3DShowOpenings, "geometry3DVisibility", "openings");
bindGeometryVisibilityControl(elements.geometryPlanShowZones, "geometryPlanVisibility", "zones");
bindGeometryVisibilityControl(elements.geometryPlanShowBoundaries, "geometryPlanVisibility", "boundaries");
bindGeometryVisibilityControl(elements.geometryPlanShowOpenings, "geometryPlanVisibility", "openings");
elements.hvacExpandButton.addEventListener("click", () => toggleExpandedPane("hvac"));
elements.geometryExpandButton.addEventListener("click", () => toggleExpandedPane("geometry"));

function bindGeometryVisibilityControl(control, stateKey, optionKey) {
  control.addEventListener("change", () => {
    state[stateKey] = {
      ...(state[stateKey] || {}),
      [optionKey]: control.checked,
    };
    renderGeometry();
  });
}

function updateThermalTopologySetting(key, value) {
  if (state[key] === value) {
    return;
  }
  recordViewHistory();
  state[key] = value;
  renderGeometry();
}

function activateGeometryModeShortcut(mode) {
  if (state.activeResultTab !== "geometry") return false;
  setGeometryMode(mode);
  return true;
}

function activateGeometryFitShortcut() {
  if (state.activeResultTab !== "geometry") return false;
  void fitGeometryView();
  return true;
}

function activateThermalTopologySettingShortcut(key, value) {
  if (state.activeResultTab !== "geometry" || state.geometryMode !== "thermal") return false;
  updateThermalTopologySetting(key, value);
  return true;
}

elements.inputViewButtons.forEach((button) => {
  button.addEventListener("click", async () => {
    await switchInputView(button.dataset.inputView);
    updateSemanticRevealIndicator(state.globalSelection);
  });
});
elements.semanticRevealIndicator?.addEventListener("click", async () => {
  await revealSelectionInSemantic({
    originView: state.activeResultTab,
    action: "reveal",
    recordHistory: true,
    preserveFilters: true,
  });
  updateSemanticRevealIndicator(state.globalSelection);
});
window.addEventListener("idfAnalyzer:semanticRevealAvailable", (event) => {
  updateSemanticRevealIndicator(event.detail?.selection || state.globalSelection);
});
window.addEventListener("idfAnalyzer:inputViewChanged", () => {
  updateSemanticRevealIndicator(state.globalSelection);
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
  if (state.activeResultTab === "geometry" || state.expandedPane === "geometry") {
    resizeGeometry();
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
  elements.idfInput.value = result.text;
  updateTextStats();
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
  state.diagnosticsReady = true;
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
  elements.idfInput.value = result.text;
  updateTextStats();
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
  state.diagnosticsReady = true;
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
  state.reportDiagnosticsReady = Boolean(state.diagnosticsReady);
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
  if (state.expandedPane === "geometry" || pane === "geometry") {
    window.requestAnimationFrame(resizeGeometry);
  }
}

function updateExpandButtons() {
  updateExpandButton(elements.hvacExpandButton, "hvac");
  updateExpandButton(elements.geometryExpandButton, "geometry");
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
  renderNavigationLinkBar();
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

async function revealCurrentSelectionInSemantic() {
  if (!state.globalSelection?.entityId) {
    setStatus(t("semantic.noAvailableView", {}, "No selection to reveal"), "warn");
    return false;
  }
  return openSelectionInView("input-semantic", {
    originView: state.activeResultTab || `input-${state.activeInputView}`,
    action: "reveal_semantic",
    preserveFilters: true,
  });
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
    document.querySelector(".workspace-link-bar"),
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
  const linkMenu = document.querySelector(".workspace-link-bar__menu[open]");
  if (linkMenu) {
    linkMenu.removeAttribute("open");
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
    ["revealSemantic", t("shortcut.revealSemantic", {}, "Reveal in Semantic"), revealCurrentSelectionInSemantic],
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
initializeDiagnoseFixes();
initializeCommandPalette(commandPaletteItems);
initializeKeyboardShortcuts({
  save: saveInputFile,
  open: openInputFile,
  undoView: handleUndoShortcut,
  redoView: handleRedoShortcut,
  jumpDefinition: jumpInputDefinition,
  jumpReferences: jumpInputReferences,
  commandPalette: openCommandPalette,
  revealSemantic: revealCurrentSelectionInSemantic,
  revealSource: revealCurrentSelectionSource,
  paneFocus: focusNextWorkspacePane,
  currentSearch: focusCurrentViewSearch,
  primaryOpen: primaryOpenFromFocus,
  availableViews: openAvailableViewsForSelection,
  clearSelection: clearSelectionOrTransientUI,
  switchInputView,
  switchResultTab,
  setGeometryMode: activateGeometryModeShortcut,
  fitGeometry: activateGeometryFitShortcut,
  setTopologyMetric: (metric) => activateThermalTopologySettingShortcut("thermalTopologyMetric", normalizeThermalTopologyMetric(metric)),
  setTopologyNeighbors: () => activateThermalTopologySettingShortcut("thermalTopologyScope", "neighbors"),
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
  state.semanticLinkMode = restoredDocument.semanticLinkMode !== false;
  state.semanticFollowSelection = restoredDocument.semanticFollowSelection !== false;
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
    textSnapshot: elements.idfInput.value,
    analysisKey: restoredDocument.analysisKey || "",
    preferCache: Boolean(restoredDocument.analysisKey),
  });
}

async function restorePendingWorkspaceContext(analysisKey = "") {
  const pending = state.pendingWorkspaceRestore;
  if (!pending) {
    return false;
  }
  if (pending.text && pending.text !== (elements.idfInput?.value || "")) {
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
  state.semanticLinkMode = restoredDocument.semanticLinkMode !== false;
  state.semanticFollowSelection = restoredDocument.semanticFollowSelection !== false;
  restoreWorkspaceLayout(restoredDocument.layout || {});
  await restoreViewSnapshot(snapshot, { recordHistory: false, quiet: true });
  window.dispatchEvent(new CustomEvent("idfAnalyzer:navigationModeChanged", {
    detail: {
      linked: state.semanticLinkMode,
      follow: state.semanticFollowSelection,
    },
  }));
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
  if (!state.resultTabManuallySelected && state.activeResultTab !== firstTab) {
    switchResultTab(firstTab, { recordHistory: false });
  }
}
