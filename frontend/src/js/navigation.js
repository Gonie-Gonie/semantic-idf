import { elements, setStatus, state } from "./state.js";
import { t } from "./i18n.js";
import { bundledAppInfo } from "./app-info.js";
import { prioritizeAnalysisStageForTab } from "./actions.js";
import { getPanelNavigationAdapter, registerPanelNavigationAdapter } from "./panel-navigation-registry.js";
import { getSemanticNavigationCache } from "./semantic-navigation-cache.js";
import {
  clearSemanticSelection,
  revealSelectionSource,
  selectSemanticEntity,
} from "./selection-controller.js";
import { renderReport } from "./views/analysis-views.js";
import { renderGeometry, resizeGeometry } from "./geometry-loader.js";
import {
  clearInputFilter,
  currentInputJumpSource,
  jumpSourceForContext,
  renderInputViews,
  resolveInputJumpTargets,
  switchInputView,
  syncRawTextToObjectField,
} from "./views/input-views.js";
import {
  captureViewSnapshot,
  popRedoSnapshot,
  popUndoSnapshot,
  recordViewHistory,
  withHistoryRestore,
} from "./view-history.js";

function currentInputViewElement() {
  return document.querySelector(`#${state.activeInputView}InputView`);
}

function findInputTarget(target) {
  const view = currentInputViewElement();
  if (!view) {
    return null;
  }

  const objectIndex = target.objectIndex === undefined || target.objectIndex === null ? "" : String(target.objectIndex);
  if (objectIndex !== "") {
    if (target.fieldIndex !== undefined && target.fieldIndex !== null && String(target.fieldIndex) !== "") {
      const byField = [...view.querySelectorAll("[data-object-index][data-field-index]")].find(
        (element) => element.dataset.objectIndex === objectIndex && element.dataset.fieldIndex === String(target.fieldIndex),
      );
      if (byField) {
        return byField;
      }
    }
    const byIndex = [...view.querySelectorAll("[data-object-index]")].find(
      (element) => element.dataset.objectIndex === objectIndex,
    );
    if (byIndex) {
      return byIndex;
    }
  }

  if (target.objectType) {
    return [...view.querySelectorAll("[data-object-type]")].find(
      (element) => element.dataset.objectType === target.objectType,
    );
  }
  return null;
}

function expandDetailsFor(element) {
  let current = element;
  while (current) {
    if (current.tagName && current.tagName.toLowerCase() === "details") {
      current.open = true;
    }
    current = current.parentElement;
  }
}

function highlightInputTarget(element) {
  element.classList.remove("input-jump-highlight");
  // Force the class restart so repeated jumps to the same object are visible.
  void element.offsetWidth;
  element.classList.add("input-jump-highlight");
  window.setTimeout(() => element.classList.remove("input-jump-highlight"), 1800);
}

function scrollInputTargetIntoView(element) {
  const container = element.closest(".semantic-editor, .formatted-object-view, .json-view, .field-table");
  if (!container) {
    return;
  }

  const containerRect = container.getBoundingClientRect();
  const elementRect = element.getBoundingClientRect();
  const targetTop = container.scrollTop + elementRect.top - containerRect.top - container.clientHeight * 0.25;
  const targetLeft = container.scrollLeft + elementRect.left - containerRect.left - 24;
  container.scrollTo({
    top: Math.max(0, targetTop),
    left: Math.max(0, targetLeft),
  });
}

function focusNavigatedInputTarget(element) {
  const focusTarget =
    element.closest("td, th, dt, .text-field-cell, .json-field-row, .json-instance, .text-object") || element;
  if (!focusTarget || focusTarget.matches("input, textarea, select, button")) {
    return;
  }
  if (!focusTarget.hasAttribute("tabindex")) {
    focusTarget.setAttribute("tabindex", "-1");
  }
  focusTarget.focus({ preventScroll: true });
}

function sourceViewForInputTarget(candidate = "") {
  const normalized = String(candidate || "").trim().toLowerCase().replace(/^input-/, "");
  const viewName = ["text", "json", "table"].includes(normalized)
    ? normalized
    : ["text", "json", "table"].includes(state.activeInputView)
      ? state.activeInputView
      : "text";
  return `input-${viewName}`;
}

function semanticSelectionForSourceTarget(target = {}) {
  const cache = semanticNavigationCache();
  const hasObjectIndex = target.objectIndex !== undefined && target.objectIndex !== null && String(target.objectIndex) !== "";
  const objectIndex = hasObjectIndex ? Number(target.objectIndex) : null;
  let occurrenceIds = hasObjectIndex ? cache.occurrenceIdsByObjectIndex.get(String(objectIndex)) || [] : [];
  if (!occurrenceIds.length && target.objectType) {
    occurrenceIds = cache.sourceIdentityCandidates({ objectType: target.objectType })
      .filter((occurrence) => occurrence.sourceAnchor?.objectType === target.objectType)
      .map((occurrence) => occurrence.occurrenceId);
  }
  const candidates = occurrenceIds
    .map((occurrenceId) => cache.occurrence(occurrenceId))
    .filter(Boolean);
  const requestedField = target.fieldIndex === undefined || target.fieldIndex === null || String(target.fieldIndex) === ""
    ? null
    : Number(target.fieldIndex);
  const occurrence = candidates.find((candidate) => (
    requestedField !== null && Number(candidate.sourceAnchor?.fieldIndex) === requestedField
  )) || candidates[0];
  if (!occurrence) {
    return {};
  }
  const entity = cache.entity(occurrence.entityId) || {};
  const sourceAnchor = {
    ...(occurrence.sourceAnchor || entity.sourceAnchors?.[0] || {}),
  };
  if (hasObjectIndex) {
    sourceAnchor.objectIndex = objectIndex;
  }
  if (target.objectType) {
    sourceAnchor.objectType = target.objectType;
  }
  if (requestedField !== null) {
    sourceAnchor.fieldIndex = requestedField;
  }
  return {
    entityId: occurrence.entityId || entity.id || "",
    entityKind: entity.kind || "",
    occurrenceId: occurrence.occurrenceId || "",
    sourceAnchor,
    originTargetId: target.targetId || "",
    semanticPathHint: occurrence.path || "",
    relatedEntityIds: entity.relatedEntityIds || [],
  };
}

export async function focusInputObject(target, options = {}) {
  const hasObjectIndex = target.objectIndex !== undefined && target.objectIndex !== null && String(target.objectIndex) !== "";
  if (!hasObjectIndex && !target.objectType) {
    return false;
  }
  const selection = semanticSelectionForSourceTarget(target);
  if (!selection.entityId) {
    return revealLegacySourceTarget(target, options);
  }
  if (options.recordHistory !== false) {
    recordViewHistory();
  }
  await selectSemanticEntity(selection, {
    originView: options.originView || `input-${state.activeInputView || "text"}`,
    action: options.action || "reveal_source",
    recordHistory: false,
    follow: false,
    preserveFilters: options.preserveFilters !== false,
    transactionId: options.transactionId,
  });
  return revealSelectionSource({
    originView: options.originView || `input-${state.activeInputView || "text"}`,
    action: "reveal_source",
    recordHistory: false,
    preserveFilters: options.preserveFilters !== false,
    transactionId: options.revealTransactionId,
    view: sourceViewForInputTarget(options.view),
  });
}

async function revealLegacySourceTarget(target, options = {}) {
  if (options.recordHistory !== false) {
    recordViewHistory();
  }
  const requestedView = sourceViewForInputTarget(options.view);
  const viewName = requestedView.slice("input-".length);
  if (state.activeInputView !== viewName) {
    await switchInputView(viewName, { recordHistory: false });
  } else {
    renderInputViews();
  }
  return revealInputSourceTarget(target, options);
}

function revealInputSourceTarget(target, options = {}) {
  const hasObjectIndex = target.objectIndex !== undefined && target.objectIndex !== null && String(target.objectIndex) !== "";
  if (hasObjectIndex) {
    state.jsonSelectedObjectIndex = String(target.objectIndex);
    state.semanticSelectedObjectIndex = String(target.objectIndex);
  }
  const fieldIndex = target.fieldIndex === undefined || target.fieldIndex === null || String(target.fieldIndex) === ""
    ? null
    : Number(target.fieldIndex);
  const rawLocated = hasObjectIndex
    ? syncRawTextToObjectField(Number(target.objectIndex), fieldIndex, target.fieldIndexKind || "idf")
    : false;
  let element = findInputTarget(target);
  if (!element && state.inputFilterQuery && options.preserveFilters === false) {
    clearInputFilter();
    element = findInputTarget(target);
  }
  if (!element) {
    if (rawLocated) {
      setStatus(t("input.objectLocated"), "ok");
      return true;
    }
    setStatus(t("input.objectTargetMissing"), "warn");
    return false;
  }
  expandDetailsFor(element);
  scrollInputTargetIntoView(element);
  highlightInputTarget(element);
  focusNavigatedInputTarget(element);
  setStatus(t("input.objectLocated"), "ok");
  return true;
}

export function handleInputJumpActivation(element) {
  const jumpButton = element?.closest?.("[data-input-jump-kind]");
  if (!jumpButton) {
    return false;
  }
  const source = jumpSourceForContext({
    objectIndex: jumpButton.dataset.objectIndex,
    fieldIndex: jumpButton.dataset.fieldIndex,
    fieldIndexKind: jumpButton.dataset.fieldIndexKind || "idf",
  });
  jumpFromInputSource(jumpButton.dataset.inputJumpKind, source);
  return true;
}

export function jumpInputDefinition(options = {}) {
  jumpFromInputSource("definition", currentInputJumpSource(), options);
}

export function jumpInputReferences(options = {}) {
  jumpFromInputSource("references", currentInputJumpSource(), options);
}

function jumpFromInputSource(kind, source, options = {}) {
  const targets = resolveInputJumpTargets(kind, source);
  if (!targets.length) {
    setStatus(kind === "definition" ? t("input.noDefinitionTarget") : t("input.noReferenceTarget"), "warn");
    return;
  }
  const target = kind === "references" ? nextReferenceTarget(source, targets) : targets[0];
  if (options.recordHistory !== false) {
    recordViewHistory();
  }
  focusInputObject(target, { recordHistory: false });
  setStatus(
    kind === "definition"
      ? t("input.definitionLocated")
      : t("input.referenceLocated", { count: targets.length }),
    "ok",
  );
}

function nextReferenceTarget(source, targets) {
  const signature = `${source?.objectIndex ?? ""}:${source?.fieldIndex ?? ""}:${source?.value ?? source?.objectName ?? ""}`;
  if (!state.lastReferenceJump || state.lastReferenceJump.signature !== signature) {
    state.lastReferenceJump = { signature, index: 0 };
    return targets[0];
  }
  state.lastReferenceJump.index = (state.lastReferenceJump.index + 1) % targets.length;
  return targets[state.lastReferenceJump.index];
}

export function handleAnalysisActivation(element) {
  if (!element) {
    return;
  }
  const jumpTarget = element.closest("[data-jump-object-index], [data-jump-object-type]");
  if (jumpTarget) {
    focusInputObject({
      objectIndex: jumpTarget.dataset.jumpObjectIndex,
      objectType: jumpTarget.dataset.jumpObjectType,
    });
    return;
  }
  const interactive = element.closest("button, input, select, textarea, a[href], [contenteditable='true']");
  if (interactive && !interactive.matches("[data-entity-id], [data-panel-target-id]")) {
    return;
  }
  const adapter = getPanelNavigationAdapter(state.activeResultTab);
  const selection = adapter?.selectFromElement?.(element) || null;
  if (selection?.entityId) {
    const selectionTarget = element.closest?.("[data-choose-semantic-occurrence]");
    selectSemanticEntity(selection, {
      originView: state.activeResultTab,
      action: "select",
      chooseOccurrence: selection.chooseOccurrence === true || selectionTarget?.dataset.chooseSemanticOccurrence === "true",
    });
    return;
  }
}

export function handleInputSelectionActivation(element) {
  if (!element || state.activeInputView === "semantic") {
    return false;
  }
  if (element.closest?.("button, input, select, textarea, a[href], [contenteditable='true']")) {
    return false;
  }
  const viewId = `input-${state.activeInputView}`;
  const adapter = getPanelNavigationAdapter(viewId);
  const selection = adapter?.selectFromElement?.(element) || null;
  if (!selection?.entityId) {
    return false;
  }
  selectSemanticEntity(selection, {
    originView: viewId,
    action: "select",
    recordHistory: true,
    follow: false,
  });
  return true;
}

export function refreshInputSelectionStyles(selection = state.globalSelection) {
  if (state.activeInputView === "semantic") {
    return 0;
  }
  const root = currentInputViewElement();
  if (!root) {
    return 0;
  }
  const anchor = selection?.sourceAnchor || {};
  const objectIndex = anchor.objectIndex === undefined || anchor.objectIndex === null ? "" : String(anchor.objectIndex);
  const fieldIndex = anchor.fieldIndex === undefined || anchor.fieldIndex === null ? "" : String(anchor.fieldIndex);
  let selectedCount = 0;
  root.querySelectorAll("[data-object-index]").forEach((item) => {
    const sameObject = Boolean(objectIndex && item.dataset.objectIndex === objectIndex);
    const sameField = !fieldIndex || !item.dataset.fieldIndex || item.dataset.fieldIndex === fieldIndex;
    const selected = sameObject && sameField;
    item.classList.toggle("semantic-selected", selected);
    item.toggleAttribute("data-semantic-selected", selected);
    selectedCount += Number(selected);
  });
  return selectedCount;
}

let legacyInputAdaptersInitialized = false;

export function initializeLegacyInputNavigationAdapters() {
  if (legacyInputAdaptersInitialized) {
    return;
  }
  legacyInputAdaptersInitialized = true;
  ["semantic", "text", "json", "table"].forEach((viewName) => {
    const viewId = `input-${viewName}`;
    registerPanelNavigationAdapter(viewId, createLegacyInputNavigationAdapter(viewName));
  });
}

function createLegacyInputNavigationAdapter(viewName) {
  const viewId = `input-${viewName}`;
  return {
    canReveal(selection) {
      if (viewName === "semantic") {
        return Boolean(selection?.entityId && semanticOccurrenceIDs(selection.entityId).length);
      }
      const objectIndex = selection?.sourceAnchor?.objectIndex;
      return Boolean(
        selection?.sourceAnchor?.objectId ||
        (objectIndex !== undefined && objectIndex !== null && String(objectIndex) !== ""),
      );
    },
    async reveal(selection, options = {}) {
      if (viewName === "semantic" && state.activeInputView !== "semantic" && options.action === "select") {
        window.dispatchEvent(new CustomEvent("idfAnalyzer:semanticRevealAvailable", { detail: { selection, options } }));
        return false;
      }
      if (state.activeInputView !== viewName) {
        await switchInputView(viewName, { recordHistory: false, revealSelection: false });
      } else {
        renderInputViews();
      }
      const element = findInputSelectionElement(selection);
      if (element) {
        if (viewName === "semantic") {
          state.semanticCurrentOccurrenceId = selection.occurrenceId || element.dataset.occurrenceId || "";
          state.semanticCurrentPath = selection.semanticPathHint || element.dataset.semanticPath || "";
        }
        expandDetailsFor(element);
        scrollInputTargetIntoView(element);
        highlightInputTarget(element);
        focusNavigatedInputTarget(element);
        return true;
      }
      if (viewName === "semantic") {
        return false;
      }
      const anchor = selection?.sourceAnchor || {};
      return revealInputSourceTarget({
        objectIndex: anchor.objectIndex,
        objectType: anchor.objectType,
        fieldIndex: anchor.fieldIndex,
        fieldIndexKind: "idf",
      }, options);
    },
    selectFromElement(element) {
      const target = element?.closest?.("[data-entity-id], [data-occurrence-id], [data-object-index], [data-object-type]");
      if (!target) {
        return null;
      }
      if (target.dataset.entityId) {
        return {
          entityId: target.dataset.entityId,
          entityKind: target.dataset.entityKind || "",
          occurrenceId: target.dataset.occurrenceId || "",
          originView: viewId,
          originTargetId: target.dataset.panelTargetId || "",
          semanticPathHint: target.dataset.semanticPath || "",
        };
      }
      return {
        ...semanticSelectionForSourceTarget({
          objectIndex: target.dataset.objectIndex,
          objectType: target.dataset.objectType,
          fieldIndex: target.dataset.fieldIndex,
          fieldIndexKind: target.dataset.fieldIndexKind || "idf",
        }),
        originView: viewId,
      };
    },
    captureContext() {
      const container = inputViewContainer(viewName);
      return {
        view: viewName,
        scrollTop: Number(container?.scrollTop) || 0,
        scrollLeft: Number(container?.scrollLeft) || 0,
      };
    },
    async restoreContext(context = {}) {
      if (state.activeInputView !== viewName) {
        await switchInputView(viewName, { recordHistory: false, revealSelection: false });
      }
      const container = inputViewContainer(viewName);
      if (container) {
        container.scrollTop = Number(context.scrollTop) || 0;
        container.scrollLeft = Number(context.scrollLeft) || 0;
      }
    },
    preferredSemanticOccurrence(selection) {
      if (selection?.occurrenceId) {
        return selection.occurrenceId;
      }
      const occurrences = semanticOccurrences(selection?.entityId);
      if (selection?.semanticPathHint) {
        const contextual = occurrences.find((occurrence) => occurrence.path === selection.semanticPathHint);
        if (contextual) {
          return contextual.occurrenceId;
        }
      }
      return "";
    },
  };
}

function semanticOccurrences(entityId) {
  const cache = semanticNavigationCache();
  return cache.occurrencesForIDs(cache.occurrenceIdsByEntityId.get(String(entityId || "")) || []);
}

function semanticNavigationCache() {
  return getSemanticNavigationCache(state.semanticProjection, {
    textHash: state.reportAnalysisKey || state.lastAnalyzedKey || state.analysisKey || "",
    analyzerVersion: bundledAppInfo.version,
    schemaVersion: state.semanticProjection?.schema || "",
  });
}

function semanticOccurrenceIDs(entityId) {
  return semanticOccurrences(entityId).map((occurrence) => occurrence.occurrenceId);
}

function findInputSelectionElement(selection = {}) {
  const view = currentInputViewElement();
  if (!view) {
    return null;
  }
  const candidates = [...view.querySelectorAll("[data-occurrence-id], [data-entity-id], [data-object-index], [data-object-type]")];
  if (selection.occurrenceId) {
    const occurrence = candidates.find((element) => element.dataset.occurrenceId === selection.occurrenceId);
    if (occurrence) {
      return occurrence;
    }
  }
  if (selection.entityId) {
    const entity = candidates.find((element) => element.dataset.entityId === selection.entityId);
    if (entity) {
      return entity;
    }
  }
  const anchor = selection.sourceAnchor || {};
  return findInputTarget({
    objectIndex: anchor.objectIndex,
    objectType: anchor.objectType,
    fieldIndex: anchor.fieldIndex,
  });
}

function inputViewContainer(viewName) {
  switch (viewName) {
    case "semantic": return elements.semanticEditor;
    case "json": return elements.jsonStructuredView;
    case "table": return elements.fieldTable;
    default: return elements.textObjectView;
  }
}

export function switchResultTab(tabName, options = {}) {
  if (options.recordHistory !== false && state.activeResultTab !== tabName) {
    recordViewHistory();
  }
  state.activeResultTab = knownResultTabIDs().includes(tabName) ? tabName : "summary";
  elements.resultTabButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.resultTab === state.activeResultTab);
  });
  elements.resultPanes.forEach((pane) => {
    pane.classList.toggle("active", pane.id === `${state.activeResultTab}Pane`);
  });
  prioritizeAnalysisStageForTab(state.activeResultTab);
  if (state.report && (state.analysisDirty?.[state.activeResultTab] ?? true)) {
    renderReport({ scope: "active" });
  }
  if (state.activeResultTab === "geometry") {
    window.setTimeout(() => {
      if (state.geometryReady) {
        renderGeometry();
      } else {
        renderReport({ scope: "active" });
      }
      resizeGeometry();
    }, 0);
  }
}

function knownResultTabIDs() {
  const ids = [...elements.resultTabButtons].map((button) => button.dataset.resultTab).filter(Boolean);
  return ids.length ? ids : ["summary", "profile", "hvac", "output", "simulation", "diagnose", "geometry"];
}

export async function undoViewNavigation(options = {}) {
  const snapshot = popUndoSnapshot(captureViewSnapshot());
  if (!snapshot) {
    setStatus(t("status.noViewHistory"), "warn");
    return;
  }
  await restoreViewSnapshot(snapshot, options);
}

export async function redoViewNavigation(options = {}) {
  const snapshot = popRedoSnapshot(captureViewSnapshot());
  if (!snapshot) {
    setStatus(t("status.noViewHistory"), "warn");
    return;
  }
  await restoreViewSnapshot(snapshot, options);
}

export async function restoreViewSnapshot(snapshot, options = {}) {
  const scope = options.scope || "all";
  await withHistoryRestore(async () => {
    restoreSemanticSnapshotState(snapshot);
    if (snapshot.globalSelection?.entityId) {
      await selectSemanticEntity(snapshot.globalSelection, {
        originView: snapshot.globalSelection.originView || "history",
        action: "restore",
        recordHistory: false,
        follow: false,
      });
    } else if (snapshot.globalSelection) {
      await clearSemanticSelection({ recordHistory: false, follow: false });
    }
    state.semanticTemporaryReveal = snapshot.semanticTemporaryReveal || null;
    if (snapshot.semantic && Object.prototype.hasOwnProperty.call(snapshot.semantic, "temporaryReveal")) {
      state.semanticTemporaryReveal = snapshot.semantic.temporaryReveal || null;
    }
    state.semanticCurrentOccurrenceId = snapshot.semanticCurrentOccurrenceId || snapshot.globalSelection?.occurrenceId || "";
    state.semanticCurrentPath = snapshot.semanticCurrentPath || snapshot.globalSelection?.semanticPathHint || "";
    state.jsonSelectedObjectIndex = snapshot.jsonSelectedObjectIndex || "";
    if (scope !== "input" && snapshot.resultTab && snapshot.resultTab !== state.activeResultTab) {
      switchResultTab(snapshot.resultTab, { recordHistory: false });
    }
    if (snapshot.inputView && snapshot.inputView !== state.activeInputView) {
      await switchInputView(snapshot.inputView, { recordHistory: false });
    } else {
      renderInputViews();
    }
    await restoreRegisteredPanelContext(`input-${state.activeInputView}`, snapshot.panelContexts);
    if (scope !== "input") {
      await restoreRegisteredPanelContext(state.activeResultTab, snapshot.panelContexts);
    }
    if (snapshot.activeObjectIndex !== undefined && snapshot.activeObjectIndex !== null && String(snapshot.activeObjectIndex) !== "") {
      await focusInputObject({
        objectIndex: snapshot.activeObjectIndex,
        fieldIndex: snapshot.activeFieldIndex,
        fieldIndexKind: snapshot.activeFieldIndexKind || "idf",
      }, { recordHistory: false });
    }
    await revealRestoredSelection(scope);
    restoreRawEditorPosition(snapshot);
    restoreViewScrolls(snapshot);
  });
  if (!options.quiet) {
    setStatus(t("status.viewHistoryRestored"), "ok");
  }
}

function restoreSemanticSnapshotState(snapshot = {}) {
  const semantic = snapshot.semantic || {};
  if (semantic.mode) {
    state.semanticProjectionMode = semantic.mode;
  }
  if (semantic.facet) {
    state.semanticProjectionFacet = semantic.facet;
  }
  if (Object.prototype.hasOwnProperty.call(semantic, "filter")) {
    state.inputFilterQuery = String(semantic.filter || "");
    if (elements.inputFilter) {
      elements.inputFilter.value = state.inputFilterQuery;
    }
  }
  if (Array.isArray(semantic.expandedSectionIds)) {
    state.semanticExpandedSectionIds = new Set(semantic.expandedSectionIds.map(String));
  }
}

async function restoreRegisteredPanelContext(viewID, contexts = {}) {
  const context = contexts?.[viewID];
  const adapter = context ? getPanelNavigationAdapter(viewID) : null;
  if (!adapter) {
    return false;
  }
  try {
    await adapter.restoreContext(context);
    return true;
  } catch {
    return false;
  }
}

async function revealRestoredSelection(scope = "all") {
  const selection = state.globalSelection;
  if (!selection?.entityId) {
    return false;
  }
  const viewIDs = new Set([`input-${state.activeInputView}`]);
  if (scope !== "input" && state.activeResultTab) {
    viewIDs.add(state.activeResultTab);
  }
  let revealed = false;
  for (const viewID of viewIDs) {
    const adapter = getPanelNavigationAdapter(viewID);
    if (!adapter) {
      continue;
    }
    try {
      if (await adapter.canReveal(selection)) {
        revealed = Boolean(await adapter.reveal(selection, {
          action: "restore",
          recordHistory: false,
          follow: false,
          preserveFilters: true,
          scroll: false,
          focus: false,
        })) || revealed;
      }
    } catch {
      // A panel can become unavailable while a workspace snapshot is loading.
    }
  }
  return revealed;
}

function restoreRawEditorPosition(snapshot) {
  if (!elements.idfInput) {
    return;
  }
  const start = Math.max(0, Math.min(Number(snapshot.rawSelectionStart) || 0, elements.idfInput.value.length));
  const end = Math.max(start, Math.min(Number(snapshot.rawSelectionEnd) || start, elements.idfInput.value.length));
  elements.idfInput.setSelectionRange(start, end);
  elements.idfInput.scrollTop = Number(snapshot.rawScrollTop) || 0;
  elements.idfInput.scrollLeft = Number(snapshot.rawScrollLeft) || 0;
}

function restoreViewScrolls(snapshot) {
  if (elements.textObjectView) elements.textObjectView.scrollTop = Number(snapshot.textScrollTop) || 0;
  if (elements.jsonStructuredView) elements.jsonStructuredView.scrollTop = Number(snapshot.jsonScrollTop) || 0;
  if (elements.fieldTable) elements.fieldTable.scrollTop = Number(snapshot.tableScrollTop) || 0;
  if (elements.semanticEditor) elements.semanticEditor.scrollTop = Number(snapshot.semantic?.scrollTop) || 0;
}
