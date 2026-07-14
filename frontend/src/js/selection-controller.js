import { getPanelNavigationAdapter } from "./panel-navigation-registry.js";
import { bundledAppInfo } from "./app-info.js";
import { getSemanticNavigationCache } from "./semantic-navigation-cache.js";

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
  const rememberedOccurrences = new Map();

  function navigationIndex() {
    const supplied = callGetter(dependencies.getNavigationIndex);
    return supplied || controllerState.semanticProjection?.navigation || controllerState.semanticProjection?.Navigation || {};
  }

  function navigationCache(index = navigationIndex()) {
    return getSemanticNavigationCache(index, {
      textHash: callGetter(dependencies.getReportAnalysisKey) ||
        controllerState.reportAnalysisKey || controllerState.lastAnalyzedKey || controllerState.analysisKey || "",
      analyzerVersion: bundledAppInfo.version,
      schemaVersion: controllerState.semanticProjection?.schema || "",
    });
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
    const cache = navigationCache();
    const selection = enrichSelection(providedSelection || controllerState.globalSelection, cache);
    if (!selection.entityId) {
      return [];
    }
    const occurrence = findOccurrence(cache, selection.occurrenceId);
    const entity = findEntity(cache, selection.entityId);
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

  function semanticOccurrenceChoices(providedSelection = null, rawOptions = {}) {
    const index = navigationIndex();
    const cache = navigationCache(index);
    const selection = enrichSelection(providedSelection || controllerState.globalSelection, cache);
    if (!selection.entityId) {
      return [];
    }
    const occurrenceIds = uniqueStrings(cache.occurrenceIdsByEntityId.get(selection.entityId) || []);
    const currentContext = callGetter(dependencies.getCurrentSemanticContext) || {};
    const originView = String(rawOptions.originView || selection.originView || "").toLowerCase();
    const remembered = rememberedOccurrence(selection.entityId, originView);
    const currentOccurrenceId = String(currentContext.occurrenceId || "");
    const currentPath = String(currentContext.path || currentContext.semanticPath || "");
    return occurrenceIds
      .map((occurrenceId) => findOccurrence(cache, occurrenceId))
      .filter(Boolean)
      .map((occurrence) => ({
        occurrence,
        score: occurrenceChoiceScore(occurrence, {
          originView,
          currentOccurrenceId,
          currentPath,
          remembered,
        }),
      }))
      .sort((left, right) => (
        right.score - left.score ||
        String(left.occurrence.path || "").localeCompare(String(right.occurrence.path || "")) ||
        String(left.occurrence.occurrenceId || "").localeCompare(String(right.occurrence.occurrenceId || ""))
      ))
      .map(({ occurrence }) => cloneOccurrenceChoice(occurrence));
  }

  async function remapSemanticSelection(rawOptions = {}) {
    const previous = currentSemanticSelection();
    if (!previous.entityId) {
      return null;
    }
    const index = navigationIndex();
    const cache = navigationCache(index);
    const exactEntity = findEntity(cache, previous.entityId);
    let occurrence = findOccurrence(cache, previous.occurrenceId);
    let reason = "exact";
    if (occurrence && occurrence.entityId !== previous.entityId) {
      occurrence = null;
    }
    let entity = exactEntity;
    if (!entity) {
      occurrence = sourceFallbackOccurrence(cache, previous.sourceAnchor, previous.semanticPathHint, {
        allowRenamedSourceIndex: rawOptions.allowRenamedSourceIndex === true,
      });
      entity = findEntity(cache, occurrence?.entityId);
      reason = occurrence ? "source" : "missing";
    }
    if (!entity) {
      occurrence = nearestParentOccurrence(cache, previous.semanticPathHint);
      entity = findEntity(cache, occurrence?.entityId);
      reason = occurrence ? "parent" : "missing";
    }
    if (!entity) {
      await clearSemanticSelection({ ...rawOptions, recordHistory: false, follow: false });
      await invokeHook(dependencies.onSelectionRemapped, {
        previous,
        selection: currentSemanticSelection(),
        reason,
      });
      return null;
    }
    if (!occurrence) {
      const choices = semanticOccurrenceChoices({ ...previous, entityId: entity.id, occurrenceId: "" }, rawOptions);
      occurrence = findOccurrence(cache, choices[0]?.occurrenceId);
    }
    const mapped = enrichSelection({
      ...previous,
      entityId: entity.id,
      entityKind: entity.kind || previous.entityKind,
      occurrenceId: occurrence?.occurrenceId || "",
      sourceAnchor: cloneAnchor(occurrence?.sourceAnchor || entity.sourceAnchors?.[0]),
      semanticPathHint: occurrence?.path || previous.semanticPathHint,
      relatedEntityIds: entity.relatedEntityIds || [],
      transactionId: "",
    }, cache);
    const selection = await selectSemanticEntity(mapped, {
      ...rawOptions,
      originView: rawOptions.originView || previous.originView,
      recordHistory: false,
      follow: false,
      transactionId: rawOptions.transactionId,
    });
    if (controllerState.semanticPendingNavigation?.selection?.entityId === previous.entityId && selection) {
      controllerState.semanticPendingNavigation = {
        ...controllerState.semanticPendingNavigation,
        selection: cloneSelection(selection),
      };
    }
    await invokeHook(dependencies.onSelectionRemapped, { previous, selection, reason });
    return selection;
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
        navigationCache(),
      );
      if (!selection.entityId) {
        return null;
      }
      const entityChanged = selection.entityId !== previous.entityId;
      const occurrenceChanged = selection.occurrenceId !== previous.occurrenceId;
      const sourceChanged = sourceAnchorKey(selection.sourceAnchor) !== sourceAnchorKey(previous.sourceAnchor);
      const selectionChanged = entityChanged || occurrenceChanged || sourceChanged;
      if (options.recordHistory && selectionChanged) {
        await invokeHook(dependencies.recordHistory, {
          action: "select",
          previous: cloneSelection(previous),
          selection: cloneSelection(selection),
          transactionId: options.transactionId,
        });
      }
      controllerState.globalSelection = selection;
      rememberOccurrenceSelection(selection, options.rememberForOriginView || options.originView);
      const temporaryReveal = controllerState.semanticTemporaryReveal;
      const preserveTemporaryReveal = Boolean(
        temporaryReveal?.entityId === selection.entityId &&
        (!temporaryReveal.occurrenceId || temporaryReveal.occurrenceId === selection.occurrenceId),
      );
      controllerState.semanticTemporaryReveal = preserveTemporaryReveal ? temporaryReveal : null;
      await invokeHook(dependencies.onSelectionChange, {
        selection: cloneSelection(selection),
        previous: cloneSelection(previous),
        options,
        temporaryRevealCleared: Boolean(temporaryReveal && !preserveTemporaryReveal),
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
      const temporaryRevealCleared = Boolean(controllerState.semanticTemporaryReveal);
      controllerState.globalSelection = { ...emptySelection(), transactionId: options.transactionId };
      controllerState.semanticTemporaryReveal = null;
      controllerState.semanticPendingNavigation = null;
      if (rawOptions.resetMemory) {
        rememberedOccurrences.clear();
      }
      await invokeHook(dependencies.onSelectionChange, {
        selection: currentSemanticSelection(),
        previous: cloneSelection(previous),
        options,
        temporaryRevealCleared,
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
      const selection = enrichSelection(controllerState.globalSelection, navigationCache());
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
      const activeInputView = String(callGetter(dependencies.getActiveInputView) || "").toLowerCase();
      if (activeInputView && activeInputView !== "input-semantic") {
        const activeInputAdapter = adapterFor(activeInputView);
        if (activeInputAdapter && await adapterCanReveal(activeInputAdapter, selection)) {
          await invokeAdapterReveal(activeInputAdapter, selection, {
            ...options,
            action: "select",
            follow: false,
            recordHistory: false,
          });
        }
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
    const revealSelection = enrichSelection({ ...selection, occurrenceId }, navigationCache());
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
    const occurrence = findOccurrence(navigationCache(), selection.occurrenceId);
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
    const index = navigationIndex();
    const cache = navigationCache(index);
    const occurrenceIds = uniqueStrings(cache.occurrenceIdsByEntityId.get(selection.entityId) || []);
    if (selection.occurrenceId && occurrenceIds.includes(selection.occurrenceId)) {
      rememberOccurrenceSelection(selection, options.originView);
      return selection.occurrenceId;
    }
    const preferred = String(await Promise.resolve(adapter.preferredSemanticOccurrence(selection)) || "");
    if (preferred && occurrenceIds.includes(preferred)) {
      rememberOccurrenceSelection({ ...selection, occurrenceId: preferred }, options.originView);
      return preferred;
    }
    const occurrences = semanticOccurrenceChoices(selection, options);
    if (occurrences.length <= 1) {
      return occurrences[0]?.occurrenceId || "";
    }
    if (!options.chooseOccurrence) {
      const automatic = occurrences[0]?.occurrenceId || "";
      rememberOccurrenceSelection({ ...selection, occurrenceId: automatic }, options.originView);
      return automatic;
    }
    const payload = {
      kind: "semantic-occurrence",
      view: "input-semantic",
      selection: cloneSelection(selection),
      occurrences,
      options,
    };
    const chosen = await invokeChooser(dependencies.chooseSemanticOccurrence, payload);
    if (chosen) {
      const occurrenceId = typeof chosen === "string" ? chosen : chosen.occurrenceId;
      if (occurrenceIds.includes(occurrenceId)) {
        rememberOccurrenceSelection({ ...selection, occurrenceId }, options.originView);
        return occurrenceId;
      }
    }
    await invokeHook(dependencies.onChooserRequested, payload);
    return null;
  }

  function rememberOccurrenceSelection(selection, originView = "") {
    if (!selection?.entityId || !selection?.occurrenceId) {
      return;
    }
    const normalizedOrigin = String(originView || selection.originView || "").toLowerCase();
    rememberedOccurrences.set(occurrenceMemoryKey(selection.entityId, normalizedOrigin), selection.occurrenceId);
    rememberedOccurrences.set(occurrenceMemoryKey(selection.entityId, ""), selection.occurrenceId);
  }

  function rememberedOccurrence(entityId, originView) {
    return rememberedOccurrences.get(occurrenceMemoryKey(entityId, originView)) ||
      rememberedOccurrences.get(occurrenceMemoryKey(entityId, "")) || "";
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
    semanticOccurrenceChoices,
    remapSemanticSelection,
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
export const semanticOccurrenceChoices = (...args) => configuredController.semanticOccurrenceChoices(...args);
export const remapSemanticSelection = (...args) => configuredController.remapSemanticSelection(...args);
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

function enrichSelection(selection, cache) {
  const normalized = normalizeSelection(selection);
  const occurrence = findOccurrence(cache, normalized.occurrenceId);
  if (!normalized.entityId && occurrence?.entityId) {
    normalized.entityId = occurrence.entityId;
  }
  const entity = findEntity(cache, normalized.entityId);
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

function findEntity(cache, entityId) {
  return entityId ? cache?.entityById?.get(String(entityId)) || null : null;
}

function findOccurrence(cache, occurrenceId) {
  return occurrenceId ? cache?.occurrenceById?.get(String(occurrenceId)) || null : null;
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

function occurrenceChoiceScore(occurrence, context = {}) {
  let score = 0;
  if (occurrenceMatchesOrigin(occurrence, context.originView)) {
    score += 1_000_000_000;
  }
  if (context.currentOccurrenceId && occurrence.occurrenceId === context.currentOccurrenceId) {
    score += 2_000_000;
  } else if (
    context.currentPath &&
    semanticSectionForPath(occurrence.path) === semanticSectionForPath(context.currentPath)
  ) {
    score += 1_000_000;
  }
  if (context.remembered && occurrence.occurrenceId === context.remembered) {
    score += 1_000;
  }
  if (String(occurrence.contextKind || "").toLowerCase() === "definition" || /(^|\/)definitions?(\/|$)/i.test(occurrence.path || "")) {
    score += 1;
  }
  return score;
}

function occurrenceMatchesOrigin(occurrence, originView = "") {
  const view = String(originView || "").toLowerCase().replace(/^input-/, "");
  if (!view || view === "semantic") {
    return false;
  }
  if (String(occurrence?.preferredView || "").toLowerCase() === view) {
    return true;
  }
  const context = `${occurrence?.contextKind || ""} ${occurrence?.path || ""}`.toLowerCase();
  const aliases = {
    summary: ["summary", "project"],
    profile: ["profile", "schedule", "load"],
    hvac: ["hvac", "service", "air_loop", "plant_loop", "condenser_loop", "system", "coupling"],
    output: ["output", "variable", "meter"],
    simulation: ["simulation", "result", "service", "output"],
    diagnose: ["diagnostic", "diagnose", "issue"],
    geometry: ["geometry", "surface", "fenestration", "space", "story"],
    text: ["source", "definition"],
    json: ["source", "definition"],
    table: ["source", "definition"],
  };
  return (aliases[view] || [view]).some((token) => context.includes(token));
}

function semanticSectionForPath(path = "") {
  const parts = String(path || "").split("/").filter(Boolean);
  if (!parts.length) {
    return "";
  }
  if (parts[0] === "zones" && parts.length >= 3) {
    return parts.slice(0, 3).join("/");
  }
  return parts.slice(0, Math.min(3, parts.length)).join("/");
}

function occurrenceMemoryKey(entityId, originView) {
  return `${String(entityId || "")}\u0000${String(originView || "").toLowerCase()}`;
}

function sourceAnchorKey(anchor) {
  if (!anchor) {
    return "";
  }
  return [
    anchor.objectId,
    anchor.objectIndex,
    anchor.objectType,
    anchor.objectName,
    anchor.fieldIndex,
    anchor.fieldName,
  ].map((value) => String(value ?? "")).join("\u0000");
}

function sourceFallbackOccurrence(cache, anchor, semanticPathHint = "", options = {}) {
  if (!anchor || !cache?.occurrenceById) {
    return null;
  }
  const objectIDMatches = uniqueStrings(
    anchor.objectId ? cache.occurrenceIdsByObjectId.get(String(anchor.objectId)) || [] : [],
  )
    .map((id) => findOccurrence(cache, id))
    .filter(Boolean);
  let candidates = objectIDMatches;
  if (!candidates.length && anchor.objectIndex !== undefined && anchor.objectIndex !== null) {
    const indexedCandidates = uniqueStrings(cache.occurrenceIdsByObjectIndex.get(String(anchor.objectIndex)) || [])
      .map((id) => findOccurrence(cache, id))
      .filter(Boolean);
    candidates = indexedCandidates.filter((occurrence) => sourceAnchorMatchesIdentity(occurrence.sourceAnchor, anchor));
    if (!candidates.length && options.allowRenamedSourceIndex) {
      candidates = indexedCandidates.filter((occurrence) => sourceAnchorMatchesRenamedIdentity(occurrence.sourceAnchor, anchor));
    }
  }
  if (!candidates.length) {
    candidates = cache.sourceIdentityCandidates(anchor)
      .filter((occurrence) => sourceAnchorMatchesIdentity(occurrence.sourceAnchor, anchor));
  }
  return candidates
    .map((occurrence) => ({
      occurrence,
      score: sourceFallbackScore(occurrence, anchor, semanticPathHint),
    }))
    .sort((left, right) => right.score - left.score || String(left.occurrence.path || "").localeCompare(String(right.occurrence.path || "")))[0]
    ?.occurrence || null;
}

function sourceAnchorMatchesRenamedIdentity(candidate = {}, anchor = {}) {
  if (!anchor.objectType || !candidate.objectType) {
    return false;
  }
  if (normalizeIdentityText(candidate.objectType) !== normalizeIdentityText(anchor.objectType)) {
    return false;
  }
  if (anchor.fieldName) {
    return normalizeIdentityText(candidate.fieldName) === normalizeIdentityText(anchor.fieldName);
  }
  if (anchor.fieldIndex !== undefined && anchor.fieldIndex !== null) {
    return Number(candidate.fieldIndex) === Number(anchor.fieldIndex);
  }
  return true;
}

function sourceAnchorMatchesIdentity(candidate = {}, anchor = {}) {
  if (anchor.objectType && normalizeIdentityText(candidate.objectType) !== normalizeIdentityText(anchor.objectType)) {
    return false;
  }
  if (anchor.objectName && normalizeIdentityText(candidate.objectName) !== normalizeIdentityText(anchor.objectName)) {
    return false;
  }
  if (anchor.fieldName && normalizeIdentityText(candidate.fieldName) !== normalizeIdentityText(anchor.fieldName)) {
    return false;
  }
  if (
    !anchor.fieldName &&
    anchor.fieldIndex !== undefined && anchor.fieldIndex !== null &&
    Number(candidate.fieldIndex) !== Number(anchor.fieldIndex)
  ) {
    return false;
  }
  return Boolean(anchor.objectType || anchor.objectName || anchor.fieldName || anchor.fieldIndex !== undefined);
}

function sourceFallbackScore(occurrence, anchor, semanticPathHint) {
  const candidate = occurrence?.sourceAnchor || {};
  let score = commonPathPrefixLength(occurrence?.path, semanticPathHint) * 10;
  if (anchor.objectId && candidate.objectId === anchor.objectId) score += 1000;
  if (anchor.objectIndex !== undefined && Number(candidate.objectIndex) === Number(anchor.objectIndex)) score += 800;
  if (anchor.fieldName && normalizeIdentityText(candidate.fieldName) === normalizeIdentityText(anchor.fieldName)) score += 200;
  if (anchor.fieldIndex !== undefined && Number(candidate.fieldIndex) === Number(anchor.fieldIndex)) score += 150;
  if (anchor.objectType && normalizeIdentityText(candidate.objectType) === normalizeIdentityText(anchor.objectType)) score += 100;
  if (anchor.objectName && normalizeIdentityText(candidate.objectName) === normalizeIdentityText(anchor.objectName)) score += 100;
  return score;
}

function nearestParentOccurrence(cache, semanticPathHint = "") {
  return cache?.nearestParentOccurrence?.(semanticPathHint) || null;
}

function commonPathPrefixLength(left = "", right = "") {
  const leftParts = String(left || "").split("/").filter(Boolean);
  const rightParts = String(right || "").split("/").filter(Boolean);
  let count = 0;
  while (count < leftParts.length && count < rightParts.length && leftParts[count] === rightParts[count]) {
    count += 1;
  }
  return count;
}

function normalizeIdentityText(value) {
  return String(value || "").trim().toLowerCase().replace(/\s+/g, " ");
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
