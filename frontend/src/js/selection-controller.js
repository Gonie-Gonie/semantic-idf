import { getPanelNavigationAdapter } from "./panel-navigation-registry.js";

const emptySelection = () => ({
  entityId: "",
  entityKind: "",
  occurrenceId: "",
  sourceAnchor: null,
  originView: "",
  originTargetId: "",
  semanticPathHint: "",
  relatedEntityIds: [],
  transactionId: "",
});

const emptyHover = () => ({
  entityId: "",
  occurrenceId: "",
  originView: "",
});

const maxRememberedTransactions = 512;
let fallbackTransactionSequence = 0;

/**
 * Adds controller-owned defaults to an application state object. Keeping this
 * explicit makes the controller usable in isolated tests and during staged
 * migration of the legacy state module.
 */
export function initializeSelectionControllerState(targetState = {}) {
  if (!targetState || typeof targetState !== "object") {
    throw new TypeError("Selection controller state must be an object");
  }
  targetState.globalSelection = normalizeSelection(targetState.globalSelection);
  targetState.globalHover = normalizeHover(targetState.globalHover);
  if (typeof targetState.semanticLinkMode !== "boolean") {
    targetState.semanticLinkMode = true;
  }
  if (typeof targetState.semanticFollowSelection !== "boolean") {
    targetState.semanticFollowSelection = true;
  }
  if (!("semanticTemporaryReveal" in targetState)) {
    targetState.semanticTemporaryReveal = null;
  }
  if (!("semanticPendingNavigation" in targetState)) {
    targetState.semanticPendingNavigation = null;
  }
  return targetState;
}

/**
 * Creates a dependency-injected controller. None of its navigation paths
 * imports or invokes an analyzer; stale targets are queued for the ordinary
 * analysis lifecycle to resume explicitly.
 */
export function createSelectionController(dependencies = {}) {
  const controllerState = initializeSelectionControllerState(dependencies.state || {});
  const activeTransactions = new Set();
  const rememberedTransactions = new Set();
  const transactionOrder = [];

  function navigationIndex() {
    const supplied = callGetter(dependencies.getNavigationIndex);
    return supplied || controllerState.semanticProjection?.navigation || controllerState.semanticProjection?.Navigation || {};
  }

  function adapterFor(view) {
    const getter = dependencies.getPanelNavigationAdapter || getPanelNavigationAdapter;
    return getter(String(view || "")) || null;
  }

  function nextTransactionId() {
    const supplied = callGetter(dependencies.createTransactionId);
    if (supplied) {
      return String(supplied);
    }
    if (globalThis.crypto?.randomUUID) {
      return `nav-${globalThis.crypto.randomUUID()}`;
    }
    fallbackTransactionSequence += 1;
    return `nav-${Date.now().toString(36)}-${fallbackTransactionSequence.toString(36)}`;
  }

  function optionsFor(action, options = {}) {
    const historyByDefault = action === "select" || action === "open" || action === "reveal_source";
    return {
      ...options,
      action: options.action || action,
      originView: String(options.originView || ""),
      recordHistory: options.recordHistory === undefined ? historyByDefault : options.recordHistory !== false,
      follow: options.follow === undefined ? controllerState.semanticFollowSelection : options.follow !== false,
      preserveFilters: options.preserveFilters !== false,
      transactionId: String(options.transactionId || nextTransactionId()),
    };
  }

  async function inTransaction(options, callback) {
    const transactionId = options.transactionId;
    if (activeTransactions.has(transactionId) || rememberedTransactions.has(transactionId)) {
      return null;
    }
    activeTransactions.add(transactionId);
    rememberTransaction(transactionId);
    try {
      return await callback(transactionId);
    } finally {
      activeTransactions.delete(transactionId);
    }
  }

  function rememberTransaction(transactionId) {
    rememberedTransactions.add(transactionId);
    transactionOrder.push(transactionId);
    while (transactionOrder.length > maxRememberedTransactions) {
      rememberedTransactions.delete(transactionOrder.shift());
    }
  }

  function currentSemanticSelection() {
    return cloneSelection(controllerState.globalSelection);
  }

  function selectionTargetsForView(view, providedSelection = null) {
    const normalizedView = String(view || "").trim().toLowerCase();
    if (!normalizedView) {
      return [];
    }
    const selection = enrichSelection(providedSelection || controllerState.globalSelection, navigationIndex());
    if (!selection.entityId) {
      return [];
    }
    const index = navigationIndex();
    const occurrence = findOccurrence(index, selection.occurrenceId);
    const entity = findEntity(index, selection.entityId);
    const candidates = [
      ...(Array.isArray(occurrence?.viewTargets) ? occurrence.viewTargets : []),
      ...(Array.isArray(entity?.viewTargets) ? entity.viewTargets : []),
    ];
    const seen = new Set();
    return candidates
      .filter((target) => String(target?.view || "").toLowerCase() === normalizedView && target?.targetId)
      .filter((target) => {
        const key = `${normalizedView}\u0000${target.targetKind || ""}\u0000${target.targetId}`;
        if (seen.has(key)) {
          return false;
        }
        seen.add(key);
        return true;
      })
      .map(cloneViewTarget)
      .sort(compareViewTargets);
  }

  async function selectSemanticEntity(candidate, rawOptions = {}) {
    const options = optionsFor("select", rawOptions);
    return inTransaction(options, async () => {
      const previous = controllerState.globalSelection;
      const selection = enrichSelection(
        {
          ...candidate,
          originView: options.originView || candidate?.originView,
          transactionId: options.transactionId,
        },
        navigationIndex(),
      );
      if (!selection.entityId) {
        return null;
      }
      const entityChanged = selection.entityId !== previous.entityId;
      if (options.recordHistory && entityChanged) {
        await invokeHook(dependencies.recordHistory, {
          action: "select",
          previous: cloneSelection(previous),
          selection: cloneSelection(selection),
          transactionId: options.transactionId,
        });
      }
      controllerState.globalSelection = selection;
      controllerState.semanticTemporaryReveal = null;
      await invokeHook(dependencies.onSelectionChange, {
        selection: cloneSelection(selection),
        previous: cloneSelection(previous),
        options,
      });
      if (controllerState.semanticLinkMode && options.follow) {
        await followSelection(selection, options);
      }
      return cloneSelection(selection);
    });
  }

  async function hoverSemanticEntity(candidate, rawOptions = {}) {
    const options = optionsFor("hover", { ...rawOptions, recordHistory: false, follow: false });
    return inTransaction(options, async () => {
      const hover = normalizeHover({
        ...candidate,
        originView: options.originView || candidate?.originView,
      });
      controllerState.globalHover = hover;
      if (controllerState.semanticLinkMode) {
        await invokeHook(dependencies.onHoverChange, { hover: { ...hover }, options });
      }
      return { ...hover };
    });
  }

  async function clearSemanticHover(rawOptions = {}) {
    const options = optionsFor("hover", { ...rawOptions, recordHistory: false, follow: false });
    return inTransaction(options, async () => {
      controllerState.globalHover = emptyHover();
      await invokeHook(dependencies.onHoverChange, { hover: emptyHover(), options });
      return true;
    });
  }

  async function clearSemanticSelection(rawOptions = {}) {
    const options = optionsFor("clear_selection", { ...rawOptions, recordHistory: false, follow: false });
    return inTransaction(options, async () => {
      const previous = controllerState.globalSelection;
      controllerState.globalSelection = { ...emptySelection(), transactionId: options.transactionId };
      controllerState.semanticTemporaryReveal = null;
      controllerState.semanticPendingNavigation = null;
      await invokeHook(dependencies.onSelectionChange, {
        selection: currentSemanticSelection(),
        previous: cloneSelection(previous),
        options,
      });
      return true;
    });
  }

  async function revealSelectionInSemantic(rawOptions = {}) {
    const options = optionsFor("reveal", { ...rawOptions, recordHistory: false });
    return inTransaction(options, () => revealSemanticWithinTransaction(currentSemanticSelection(), options));
  }

  async function openSelectionInView(view, rawOptions = {}) {
    const normalizedView = String(view || "").trim().toLowerCase();
    if (!normalizedView) {
      return false;
    }
    if (normalizedView === "input-text" && rawOptions.action === "reveal_source") {
      return revealSelectionSource({ ...rawOptions, view: normalizedView });
    }
    const options = optionsFor("open", rawOptions);
    return inTransaction(options, () => openViewWithinTransaction(normalizedView, currentSemanticSelection(), options));
  }

  async function revealSelectionSource(rawOptions = {}) {
    const options = optionsFor("reveal_source", rawOptions);
    return inTransaction(options, async () => {
      const selection = enrichSelection(controllerState.globalSelection, navigationIndex());
      if (!selection.entityId || !selection.sourceAnchor) {
        return false;
      }
      const view = sourceView(rawOptions.view || callGetter(dependencies.getActiveInputView));
      const adapter = adapterFor(view);
      const revealSelection = {
        ...selection,
        viewTarget: {
          view,
          targetKind: "source",
          targetId: selection.sourceAnchor.objectId || String(selection.sourceAnchor.objectIndex ?? ""),
          sourceAnchor: cloneAnchor(selection.sourceAnchor),
        },
      };
      if (!adapter || !(await adapterCanReveal(adapter, revealSelection))) {
        return false;
      }
      if (options.recordHistory) {
        await invokeHook(dependencies.recordHistory, {
          action: "reveal_source",
          selection: cloneSelection(selection),
          transactionId: options.transactionId,
        });
      }
      await invokeHook(dependencies.openView, view, { ...options, recordHistory: false });
      return invokeAdapterReveal(adapter, revealSelection, { ...options, recordHistory: false });
    });
  }

  function pendingSemanticNavigation() {
    return clonePendingNavigation(controllerState.semanticPendingNavigation);
  }

  async function resumePendingSemanticNavigation(rawOptions = {}) {
    if (!(await analysisIsCurrent())) {
      return false;
    }
    const pending = controllerState.semanticPendingNavigation;
    if (!pending) {
      return false;
    }
    if (pending.view && pending.view !== "input-semantic" && !(await viewIsReady(pending.view, pending.selection))) {
      return false;
    }
    if (controllerState.semanticPendingNavigation !== pending) {
      return false;
    }
    // Clear first so an analysis-complete event can never apply the same
    // pending target twice, even when an adapter fails or renders again.
    controllerState.semanticPendingNavigation = null;
    const options = {
      ...pending.options,
      ...rawOptions,
      recordHistory: false,
      transactionId: rawOptions.transactionId || nextTransactionId(),
    };
    const transactionOptions = optionsFor(pending.action === "open_view" ? "open" : "reveal", options);
    if (pending.action === "reveal_semantic") {
      return inTransaction(transactionOptions, () =>
        revealSemanticWithinTransaction(pending.selection, transactionOptions),
      );
    }
    if (pending.action === "reveal_view") {
      return inTransaction(transactionOptions, () =>
        revealViewWithinTransaction(pending.view, pending.selection, transactionOptions),
      );
    }
    return inTransaction(transactionOptions, () =>
      openViewWithinTransaction(pending.view, pending.selection, transactionOptions),
    );
  }

  async function followSelection(selection, options) {
    const originView = String(options.originView || selection.originView || "").toLowerCase();
    if (!originView) {
      return false;
    }
    if (originView === "input-semantic") {
      const activePanel = String(callGetter(dependencies.getActivePanelView) || "").toLowerCase();
      if (!activePanel || activePanel.startsWith("input-")) {
        return false;
      }
      if (!(await analysisIsCurrent())) {
        return queuePendingNavigation("reveal_view", activePanel, selection, options);
      }
      return revealViewWithinTransaction(activePanel, selection, { ...options, recordHistory: false });
    }
    if (!originView.startsWith("input-")) {
      if (!(await analysisIsCurrent())) {
        return queuePendingNavigation("reveal_semantic", "input-semantic", selection, options);
      }
      return revealSemanticWithinTransaction(selection, { ...options, recordHistory: false });
    }
    return false;
  }

  async function revealSemanticWithinTransaction(selection, options) {
    if (!(await analysisIsCurrent())) {
      return queuePendingNavigation("reveal_semantic", "input-semantic", selection, options);
    }
    const adapter = adapterFor("input-semantic");
    if (!adapter) {
      return false;
    }
    const occurrenceId = await resolveSemanticOccurrence(selection, adapter, options);
    if (occurrenceId === null) {
      return false;
    }
    const revealSelection = enrichSelection({ ...selection, occurrenceId }, navigationIndex());
    if (!(await adapterCanReveal(adapter, revealSelection))) {
      return false;
    }
    controllerState.globalSelection = {
      ...controllerState.globalSelection,
      occurrenceId: revealSelection.occurrenceId,
      semanticPathHint: revealSelection.semanticPathHint,
      transactionId: options.transactionId,
    };
    controllerState.semanticTemporaryReveal = {
      entityId: revealSelection.entityId,
      occurrenceId: revealSelection.occurrenceId,
      transactionId: options.transactionId,
    };
    await invokeHook(dependencies.onTemporaryReveal, {
      reveal: { ...controllerState.semanticTemporaryReveal },
      options,
    });
    return invokeAdapterReveal(adapter, revealSelection, { ...options, recordHistory: false });
  }

  async function openViewWithinTransaction(view, selection, options) {
    if (!selection.entityId) {
      return false;
    }
    if (!(await analysisIsCurrent())) {
      return queuePendingOpen(view, selection, options);
    }
    if (!(await viewIsReady(view, selection))) {
      return queuePendingOpen(view, selection, options);
    }
    const adapter = adapterFor(view);
    if (!adapter) {
      return false;
    }
    const target = await resolveViewTarget(view, selection, options);
    if (target === null) {
      return false;
    }
    const revealSelection = target ? { ...selection, viewTarget: target } : selection;
    if (!(await adapterCanReveal(adapter, revealSelection))) {
      return false;
    }
    if (options.recordHistory) {
      await invokeHook(dependencies.recordHistory, {
        action: "open",
        view,
        selection: cloneSelection(selection),
        target: target ? cloneViewTarget(target) : null,
        transactionId: options.transactionId,
      });
    }
    await invokeHook(dependencies.openView, view, { ...options, recordHistory: false });
    return invokeAdapterReveal(adapter, revealSelection, { ...options, recordHistory: false, viewTarget: target });
  }

  async function revealViewWithinTransaction(view, selection, options) {
    if (!(await viewIsReady(view, selection))) {
      return queuePendingNavigation("reveal_view", view, selection, options);
    }
    const adapter = adapterFor(view);
    if (!adapter) {
      return false;
    }
    const target = await resolveViewTarget(view, selection, options);
    if (target === null) {
      return false;
    }
    const revealSelection = target ? { ...selection, viewTarget: target } : selection;
    if (!(await adapterCanReveal(adapter, revealSelection))) {
      return false;
    }
    return invokeAdapterReveal(adapter, revealSelection, { ...options, recordHistory: false, viewTarget: target });
  }

  async function resolveViewTarget(view, selection, options) {
    const targets = selectionTargetsForView(view, selection);
    if (!targets.length) {
      return undefined;
    }
    const requestedTargetId = String(options.targetId || selection.originTargetId || "");
    if (requestedTargetId) {
      const requested = targets.find((target) => target.targetId === requestedTargetId);
      if (requested) {
        return requested;
      }
    }
    const occurrence = findOccurrence(navigationIndex(), selection.occurrenceId);
    if (occurrence?.preferredView === view && occurrence.preferredTargetId) {
      const preferred = targets.find((target) => target.targetId === occurrence.preferredTargetId);
      if (preferred) {
        return preferred;
      }
    }
    if (targets.length === 1) {
      return targets[0];
    }
    const chosen = await invokeChooser(dependencies.chooseViewTarget, {
      kind: "view-target",
      view,
      selection: cloneSelection(selection),
      targets: targets.map(cloneViewTarget),
      options,
    });
    if (chosen) {
      const chosenId = typeof chosen === "string" ? chosen : chosen.targetId;
      return targets.find((target) => target.targetId === chosenId) || null;
    }
    await invokeHook(dependencies.onChooserRequested, {
      kind: "view-target",
      view,
      selection: cloneSelection(selection),
      targets: targets.map(cloneViewTarget),
      options,
    });
    return null;
  }

  async function resolveSemanticOccurrence(selection, adapter, options) {
    if (selection.occurrenceId) {
      return selection.occurrenceId;
    }
    const preferred = await Promise.resolve(adapter.preferredSemanticOccurrence(selection));
    if (preferred) {
      return String(preferred);
    }
    const index = navigationIndex();
    const occurrenceIds = uniqueStrings(index.byEntityId?.[selection.entityId] || []);
    if (occurrenceIds.length <= 1) {
      return occurrenceIds[0] || "";
    }
    const occurrences = occurrenceIds.map((id) => findOccurrence(index, id)).filter(Boolean);
    const chosen = await invokeChooser(dependencies.chooseSemanticOccurrence, {
      kind: "semantic-occurrence",
      view: "input-semantic",
      selection: cloneSelection(selection),
      occurrences: occurrences.map(cloneOccurrenceChoice),
      options,
    });
    if (chosen) {
      const occurrenceId = typeof chosen === "string" ? chosen : chosen.occurrenceId;
      if (occurrenceIds.includes(occurrenceId)) {
        return occurrenceId;
      }
    }
    await invokeHook(dependencies.onChooserRequested, {
      kind: "semantic-occurrence",
      view: "input-semantic",
      selection: cloneSelection(selection),
      occurrences: occurrences.map(cloneOccurrenceChoice),
      options,
    });
    return null;
  }

  async function analysisIsCurrent() {
    if (typeof dependencies.isAnalysisCurrent === "function") {
      return Boolean(await dependencies.isAnalysisCurrent());
    }
    if (!navigationIndex()?.entities && !controllerState.semanticProjection) {
      return false;
    }
    if (typeof dependencies.getCurrentText === "function") {
      return String(dependencies.getCurrentText()) === String(controllerState.lastAnalyzedText || "");
    }
    const currentKey = String(callGetter(dependencies.getCurrentAnalysisKey) || controllerState.analysisKey || "");
    const reportKey = String(callGetter(dependencies.getReportAnalysisKey) || controllerState.lastAnalyzedKey || "");
    return Boolean(currentKey && reportKey && currentKey === reportKey);
  }

  async function viewIsReady(view, selection) {
    if (typeof dependencies.isViewReady === "function") {
      return Boolean(await dependencies.isViewReady(view, cloneSelection(selection)));
    }
    if (controllerState.analysisReady && Object.prototype.hasOwnProperty.call(controllerState.analysisReady, view)) {
      return Boolean(controllerState.analysisReady[view]);
    }
    return true;
  }

  async function queuePendingOpen(view, selection, options) {
    if (options.recordHistory) {
      await invokeHook(dependencies.recordHistory, {
        action: "open",
        view,
        selection: cloneSelection(selection),
        target: null,
        transactionId: options.transactionId,
        pending: true,
      });
    }
    return queuePendingNavigation("open_view", view, selection, options);
  }

  async function queuePendingNavigation(action, view, selection, options) {
    const pending = {
      action,
      view,
      selection: cloneSelection(selection),
      options: {
        originView: options.originView,
        follow: options.follow,
        preserveFilters: options.preserveFilters,
        targetId: options.targetId || "",
      },
    };
    const existing = controllerState.semanticPendingNavigation;
    if (pendingNavigationKey(existing) === pendingNavigationKey(pending)) {
      return false;
    }
    controllerState.semanticPendingNavigation = pending;
    await invokeHook(dependencies.queueAnalysisTarget, {
      view,
      action,
      selection: cloneSelection(selection),
    });
    await invokeHook(dependencies.onAnalysisPending, clonePendingNavigation(pending));
    return false;
  }

  return Object.freeze({
    selectSemanticEntity,
    hoverSemanticEntity,
    clearSemanticHover,
    clearSemanticSelection,
    revealSelectionInSemantic,
    openSelectionInView,
    revealSelectionSource,
    currentSemanticSelection,
    selectionTargetsForView,
    pendingSemanticNavigation,
    resumePendingSemanticNavigation,
  });
}

let configuredController = createSelectionController();

export function configureSelectionController(dependencies = {}) {
  configuredController = createSelectionController(dependencies);
  return configuredController;
}

export const selectSemanticEntity = (...args) => configuredController.selectSemanticEntity(...args);
export const hoverSemanticEntity = (...args) => configuredController.hoverSemanticEntity(...args);
export const clearSemanticHover = (...args) => configuredController.clearSemanticHover(...args);
export const clearSemanticSelection = (...args) => configuredController.clearSemanticSelection(...args);
export const revealSelectionInSemantic = (...args) => configuredController.revealSelectionInSemantic(...args);
export const openSelectionInView = (...args) => configuredController.openSelectionInView(...args);
export const revealSelectionSource = (...args) => configuredController.revealSelectionSource(...args);
export const currentSemanticSelection = (...args) => configuredController.currentSemanticSelection(...args);
export const selectionTargetsForView = (...args) => configuredController.selectionTargetsForView(...args);
export const pendingSemanticNavigation = (...args) => configuredController.pendingSemanticNavigation(...args);
export const resumePendingSemanticNavigation = (...args) => configuredController.resumePendingSemanticNavigation(...args);

function normalizeSelection(selection = {}) {
  return {
    entityId: String(selection?.entityId || ""),
    entityKind: String(selection?.entityKind || ""),
    occurrenceId: String(selection?.occurrenceId || ""),
    sourceAnchor: cloneAnchor(selection?.sourceAnchor),
    originView: String(selection?.originView || ""),
    originTargetId: String(selection?.originTargetId || ""),
    semanticPathHint: String(selection?.semanticPathHint || ""),
    relatedEntityIds: uniqueStrings(selection?.relatedEntityIds || []),
    transactionId: String(selection?.transactionId || ""),
  };
}

function normalizeHover(hover = {}) {
  return {
    entityId: String(hover?.entityId || ""),
    occurrenceId: String(hover?.occurrenceId || ""),
    originView: String(hover?.originView || ""),
  };
}

function enrichSelection(selection, index) {
  const normalized = normalizeSelection(selection);
  const occurrence = findOccurrence(index, normalized.occurrenceId);
  if (!normalized.entityId && occurrence?.entityId) {
    normalized.entityId = occurrence.entityId;
  }
  const entity = findEntity(index, normalized.entityId);
  if (!normalized.entityKind && entity?.kind) {
    normalized.entityKind = entity.kind;
  }
  if (!normalized.sourceAnchor) {
    normalized.sourceAnchor = cloneAnchor(occurrence?.sourceAnchor || entity?.sourceAnchors?.[0]);
  }
  if (!normalized.semanticPathHint && occurrence?.path) {
    normalized.semanticPathHint = occurrence.path;
  }
  if (!normalized.relatedEntityIds.length && Array.isArray(entity?.relatedEntityIds)) {
    normalized.relatedEntityIds = uniqueStrings(entity.relatedEntityIds);
  }
  return normalized;
}

function findEntity(index, entityId) {
  if (!entityId || !Array.isArray(index?.entities)) {
    return null;
  }
  return index.entities.find((entity) => entity.id === entityId) || null;
}

function findOccurrence(index, occurrenceId) {
  if (!occurrenceId || !Array.isArray(index?.occurrences)) {
    return null;
  }
  return index.occurrences.find((occurrence) => occurrence.occurrenceId === occurrenceId) || null;
}

function cloneSelection(selection) {
  return normalizeSelection(selection);
}

function cloneAnchor(anchor) {
  return anchor && typeof anchor === "object" ? { ...anchor } : null;
}

function cloneViewTarget(target) {
  return target && typeof target === "object" ? { ...target } : null;
}

function cloneOccurrenceChoice(occurrence) {
  return {
    occurrenceId: String(occurrence?.occurrenceId || ""),
    entityId: String(occurrence?.entityId || ""),
    path: String(occurrence?.path || ""),
    contextKind: String(occurrence?.contextKind || ""),
    preferredView: String(occurrence?.preferredView || ""),
    preferredTargetId: String(occurrence?.preferredTargetId || ""),
    sourceAnchor: cloneAnchor(occurrence?.sourceAnchor),
  };
}

function clonePendingNavigation(pending) {
  if (!pending) {
    return null;
  }
  return {
    action: pending.action,
    view: pending.view,
    selection: cloneSelection(pending.selection),
    options: { ...(pending.options || {}) },
  };
}

function pendingNavigationKey(pending) {
  if (!pending) {
    return "";
  }
  return [
    pending.action,
    pending.view,
    pending.selection?.entityId,
    pending.selection?.occurrenceId,
    pending.options?.targetId,
  ].join("\u0000");
}

function compareViewTargets(left, right) {
  const priorityDifference = Number(right?.priority || 0) - Number(left?.priority || 0);
  if (priorityDifference) {
    return priorityDifference;
  }
  return `${left?.targetKind || ""}\u0000${left?.targetId || ""}`.localeCompare(
    `${right?.targetKind || ""}\u0000${right?.targetId || ""}`,
  );
}

function uniqueStrings(values) {
  return [...new Set((Array.isArray(values) ? values : []).map((value) => String(value || "")).filter(Boolean))];
}

function sourceView(candidate) {
  const normalized = String(candidate || "").toLowerCase();
  return ["input-text", "input-json", "input-table"].includes(normalized) ? normalized : "input-text";
}

async function adapterCanReveal(adapter, selection) {
  return Boolean(await adapter.canReveal(selection));
}

async function invokeAdapterReveal(adapter, selection, options) {
  return Boolean(await adapter.reveal(selection, options));
}

async function invokeChooser(chooser, payload) {
  return typeof chooser === "function" ? chooser(payload) : null;
}

async function invokeHook(hook, ...args) {
  return typeof hook === "function" ? hook(...args) : undefined;
}

function callGetter(getter) {
  return typeof getter === "function" ? getter() : undefined;
}
