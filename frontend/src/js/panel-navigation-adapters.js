import { getPanelNavigationAdapter, registerPanelNavigationAdapter } from "./panel-navigation-registry.js";
import { bundledAppInfo } from "./app-info.js";
import { getSemanticNavigationCache } from "./semantic-navigation-cache.js";
import { state } from "./state.js";

export const RESULT_PANEL_NAVIGATION_VIEW_IDS = Object.freeze([
  "summary",
  "profile",
  "hvac",
  "output",
  "simulation",
  "diagnose",
  "geometry",
]);

const SELECTABLE_PANEL_ITEM = [
  "[data-entity-id]",
  "[data-occurrence-id]",
  "[data-occurrence-context]",
  "[data-panel-target-id]",
  "[data-source-object-id]",
  "[data-source-object-index]",
  "[data-source-field-index]",
].join(", ");
const hooksByView = new Map();
const adaptersByView = new Map();
let selectionListenersInitialized = false;

/**
 * Adds panel-owned behavior without replacing the common adapter. A hook that
 * returns undefined delegates to the generic implementation; null/false is an
 * intentional handled result. The returned cleanup restores the prior hooks.
 */
export function configureResultPanelNavigationHooks(viewId, hooks = {}) {
  const view = normalizeResultViewId(viewId);
  if (!hooks || typeof hooks !== "object") {
    throw new TypeError(`Navigation hooks for ${view} must be an object`);
  }
  const previous = hooksByView.get(view) || null;
  const configured = Object.freeze({ ...(previous || {}), ...hooks });
  hooksByView.set(view, configured);
  return () => {
    if (hooksByView.get(view) !== configured) {
      return;
    }
    if (previous) {
      hooksByView.set(view, previous);
    } else {
      hooksByView.delete(view);
    }
  };
}

/** Registers the seven result-panel adapters. Safe to call after hot reloads. */
export function initializeResultPanelNavigationAdapters() {
  for (const viewId of RESULT_PANEL_NAVIGATION_VIEW_IDS) {
    let adapter = adaptersByView.get(viewId);
    if (!adapter) {
      adapter = createResultPanelNavigationAdapter(viewId);
      adaptersByView.set(viewId, adapter);
    }
    if (getPanelNavigationAdapter(viewId) !== adapter) {
      registerPanelNavigationAdapter(viewId, adapter);
    }
  }
  initializeSelectionStyleListeners();
  return RESULT_PANEL_NAVIGATION_VIEW_IDS.map((viewId) => getPanelNavigationAdapter(viewId));
}

/**
 * Extracts the shared semantic selection from a standardized panel item. It
 * uses the backend reverse indexes rather than reconstructing entity IDs from
 * labels or panel-local state.
 */
export function extractResultPanelSelection(element, viewId, options = {}) {
  const view = normalizeResultViewId(viewId);
  const root = options.root || resultPanelRoot(view);
  const target = closestPanelItem(element, root);
  if (!target) {
    return null;
  }

  const navigation = options.navigation || navigationIndex();
  const targetId = String(target.dataset.panelTargetId || "");
  const occurrenceContext = String(
    target.dataset.occurrenceId || target.dataset.occurrenceContext || target.dataset.semanticPath || "",
  );
  const requestedEntityId = String(target.dataset.entityId || "");
  const sourceAnchor = sourceAnchorFromElement(target);
  const occurrence = preferredOccurrence(
    occurrencesForPanelItem(navigation, view, {
      targetId,
      entityId: requestedEntityId,
      occurrenceContext,
      sourceAnchor,
    }),
    {
      viewId: view,
      targetId,
      occurrenceContext,
      semanticPathHint: target.dataset.semanticPath || "",
    },
  );
  const entityId = requestedEntityId || String(occurrence?.entityId || "");
  const entity = findEntity(navigation, entityId);
  if (!entityId) {
    return null;
  }

  return {
    entityId,
    entityKind: String(target.dataset.entityKind || entity?.kind || ""),
    occurrenceId: String(occurrence?.occurrenceId || ""),
    sourceAnchor: sourceAnchor || cloneAnchor(occurrence?.sourceAnchor || entity?.sourceAnchors?.[0]),
    originView: view,
    originTargetId: targetId,
    semanticPathHint: String(occurrence?.path || target.dataset.semanticPath || occurrenceContext || ""),
    relatedEntityIds: uniqueStrings(entity?.relatedEntityIds || []),
    chooseOccurrence: target.dataset.chooseSemanticOccurrence === "true",
  };
}

/** Updates selection/hover classes without scrolling or changing panel state. */
export function refreshResultPanelSelectionStyles(
  viewId,
  selection = state.globalSelection,
  hover = state.globalHover,
) {
  const view = normalizeResultViewId(viewId);
  const root = resultPanelRoot(view);
  if (!root) {
    return 0;
  }
  const selectedEntityId = String(selection?.entityId || "");
  const hoveredEntityId = String(hover?.entityId || "");
  const related = new Set(uniqueStrings(selection?.relatedEntityIds || []));
  const navigation = navigationIndex();
  const selectedTargetIds = new Set(viewTargetIdsForSelection(view, selection, navigation));
  const hoveredTargetIds = new Set(viewTargetIdsForSelection(view, hover, navigation));
  let selectedCount = 0;

  for (const item of panelItems(root)) {
    const itemEntityId = String(item.dataset.entityId || "");
    const itemTargetId = String(item.dataset.panelTargetId || "");
    const isSelected = Boolean(
      (selectedEntityId && itemEntityId === selectedEntityId) ||
      (itemTargetId && selectedTargetIds.has(itemTargetId)),
    );
    const isHovered = Boolean(
      (hoveredEntityId && itemEntityId === hoveredEntityId) ||
      (itemTargetId && hoveredTargetIds.has(itemTargetId)),
    );
    const isRelated = Boolean(itemEntityId && related.has(itemEntityId) && !isSelected);
    item.classList.toggle("semantic-selected", isSelected);
    item.classList.toggle("semantic-hovered", isHovered);
    item.classList.toggle("semantic-related", isRelated);
    item.toggleAttribute("data-semantic-selected", isSelected);
    if (!item.hasAttribute("tabindex")) {
      item.tabIndex = 0;
    }
    item.setAttribute("aria-selected", isSelected ? "true" : "false");
    if (isSelected) {
      item.setAttribute("aria-current", "location");
    } else {
      item.removeAttribute("aria-current");
    }
    appendARIAReference(item, "aria-describedby", "semanticNavigationHelp");
    selectedCount += Number(isSelected);
  }
  return selectedCount;
}

function createResultPanelNavigationAdapter(viewId) {
  return Object.freeze({
    canReveal(selection) {
      const context = hookContext(viewId);
      const hook = hooksByView.get(viewId)?.canReveal;
      if (typeof hook === "function") {
        const handled = hook(selection, context);
        if (handled !== undefined) {
          return handled;
        }
      }
      return genericCanReveal(viewId, selection, context);
    },
    async reveal(selection, options = {}) {
      const context = hookContext(viewId);
      const hook = hooksByView.get(viewId)?.reveal;
      if (typeof hook === "function") {
        const handled = await hook(selection, options, context);
        if (handled !== undefined) {
          return Boolean(handled);
        }
      }
      return genericReveal(viewId, selection, options, context);
    },
    selectFromElement(element) {
      const context = hookContext(viewId);
      const hook = hooksByView.get(viewId)?.selectFromElement;
      if (typeof hook === "function") {
        const handled = hook(element, context);
        if (handled !== undefined) {
          return handled;
        }
      }
      return extractResultPanelSelection(element, viewId, {
        root: context.root,
        navigation: context.navigation,
      });
    },
    captureContext() {
      const context = hookContext(viewId);
      const hook = hooksByView.get(viewId)?.captureContext;
      if (typeof hook === "function") {
        const handled = hook(context);
        if (handled !== undefined) {
          return handled;
        }
      }
      return genericCaptureContext(viewId, context);
    },
    async restoreContext(snapshot = {}) {
      const context = hookContext(viewId);
      const hook = hooksByView.get(viewId)?.restoreContext;
      if (typeof hook === "function") {
        const handled = await hook(snapshot, context);
        if (handled !== undefined) {
          return handled;
        }
      }
      return genericRestoreContext(viewId, snapshot, context);
    },
    preferredSemanticOccurrence(selection) {
      const context = hookContext(viewId);
      const hook = hooksByView.get(viewId)?.preferredSemanticOccurrence;
      if (typeof hook === "function") {
        const handled = hook(selection, context);
        if (handled !== undefined) {
          return handled;
        }
      }
      return genericPreferredSemanticOccurrence(viewId, selection, context);
    },
  });
}

function hookContext(viewId) {
  const navigation = navigationIndex();
  const cache = navigationCache(navigation);
  const root = resultPanelRoot(viewId);
  return Object.freeze({
    viewId,
    root,
    navigation,
    navigationCache: cache,
    state,
    extractSelection: (element) => extractResultPanelSelection(element, viewId, { root, navigation }),
    refreshSelectionStyles: (selection = state.globalSelection, hover = state.globalHover) => (
      refreshResultPanelSelectionStyles(viewId, selection, hover)
    ),
    genericCanReveal: (selection) => genericCanReveal(viewId, selection, { root, navigation, skipFindHook: true }),
    genericReveal: (selection, options = {}) => (
      genericReveal(viewId, selection, options, { root, navigation, skipFindHook: true })
    ),
    genericFindTarget: (selection) => genericFindTarget(viewId, selection, { root, navigation, skipFindHook: true }),
    genericCaptureContext: () => genericCaptureContext(viewId, { root, navigation }),
    genericRestoreContext: (snapshot) => genericRestoreContext(viewId, snapshot, { root, navigation }),
    genericPreferredSemanticOccurrence: (selection) => (
      genericPreferredSemanticOccurrence(viewId, selection, { root, navigation })
    ),
  });
}

function genericCanReveal(viewId, selection, context = {}) {
  return Boolean(genericFindTarget(viewId, selection, context));
}

function genericFindTarget(viewId, selection = {}, context = {}) {
  const root = context.root || resultPanelRoot(viewId);
  if (!root) {
    return null;
  }
  const hooks = hooksByView.get(viewId);
  if (!context.skipFindHook && typeof hooks?.findTarget === "function") {
    const handled = hooks.findTarget(selection, hookContext(viewId));
    if (handled !== undefined) {
      return handled;
    }
  }
  const targetIds = viewTargetIdsForSelection(viewId, selection, context.navigation || navigationIndex());
  const entityId = String(selection?.entityId || "");
  const occurrenceId = String(selection?.occurrenceId || "");
  const anchor = selection?.sourceAnchor || {};
  const items = panelItems(root);
  for (const targetId of targetIds) {
    const target = items.find((item) => item.dataset.panelTargetId === targetId);
    if (target) {
      return target;
    }
  }
  return items.find((item) => (
    (occurrenceId && item.dataset.occurrenceId === occurrenceId) ||
    (entityId && item.dataset.entityId === entityId) ||
    (anchor.objectId && item.dataset.sourceObjectId === String(anchor.objectId)) ||
    (hasIndex(anchor.objectIndex) && item.dataset.sourceObjectIndex === String(anchor.objectIndex))
  )) || null;
}

async function genericReveal(viewId, selection, options = {}, context = {}) {
  const target = genericFindTarget(viewId, selection, context);
  if (!target) {
    return false;
  }
  expandAncestorDetails(target);
  refreshResultPanelSelectionStyles(viewId, selection, state.globalHover);
  target.classList.add("semantic-selected");
  target.toggleAttribute("data-semantic-selected", true);
  if (options.scroll !== false && typeof target.scrollIntoView === "function") {
    target.scrollIntoView({
      block: options.block || "nearest",
      inline: options.inline || "nearest",
      behavior: options.behavior || "auto",
    });
  }
  if (options.focus !== false && typeof target.focus === "function") {
    if (!target.matches("a[href], button, input, select, textarea, [tabindex]")) {
      target.tabIndex = -1;
    }
    target.focus({ preventScroll: true });
  }
  return true;
}

function genericCaptureContext(viewId, context = {}) {
  const root = context.root || resultPanelRoot(viewId);
  if (!root) {
    return { scrollTop: 0, scrollLeft: 0, targetId: "", entityId: "" };
  }
  const focused = root.contains(document.activeElement)
    ? document.activeElement?.closest?.(SELECTABLE_PANEL_ITEM)
    : null;
  const target = focused || root.querySelector("[data-semantic-selected]") || root.querySelector(".semantic-selected");
  return {
    scrollTop: Number(root.scrollTop) || 0,
    scrollLeft: Number(root.scrollLeft) || 0,
    targetId: String(target?.dataset?.panelTargetId || ""),
    entityId: String(target?.dataset?.entityId || ""),
  };
}

async function genericRestoreContext(viewId, snapshot = {}, context = {}) {
  const root = context.root || resultPanelRoot(viewId);
  if (!root) {
    return false;
  }
  const targetId = String(snapshot.targetId || "");
  const entityId = String(snapshot.entityId || "");
  const target = panelItems(root).find((item) => (
    (targetId && item.dataset.panelTargetId === targetId) ||
    (!targetId && entityId && item.dataset.entityId === entityId)
  ));
  if (target) {
    expandAncestorDetails(target);
    if (typeof target.focus === "function") {
      if (!target.matches("a[href], button, input, select, textarea, [tabindex]")) {
        target.tabIndex = -1;
      }
      target.focus({ preventScroll: true });
    }
  }
  root.scrollTop = finiteNumber(snapshot.scrollTop);
  root.scrollLeft = finiteNumber(snapshot.scrollLeft);
  return true;
}

function genericPreferredSemanticOccurrence(viewId, selection = {}, context = {}) {
  const navigation = context.navigation || navigationIndex();
  const requestedOccurrence = findOccurrence(navigation, selection.occurrenceId);
  if (requestedOccurrence && (!selection.entityId || requestedOccurrence.entityId === selection.entityId)) {
    return selection.occurrenceId;
  }
  const occurrences = occurrencesForSelection(navigation, viewId, selection);
  return preferredOccurrence(occurrences, {
    viewId,
    targetId: selectionTargetIdForView(selection, viewId),
    occurrenceContext: selection.semanticPathHint || "",
    semanticPathHint: selection.semanticPathHint || state.semanticCurrentPath || "",
  })?.occurrenceId || "";
}

function occurrencesForPanelItem(navigation, viewId, item) {
  const cache = navigationCache(navigation);
  const ids = [];
  if (item.occurrenceContext && findOccurrence(navigation, item.occurrenceContext)) {
    ids.push(item.occurrenceContext);
  }
  if (item.targetId) {
    const key = `${viewId}|${item.targetId}`;
    ids.push(...reverseOccurrenceIDs(cache.occurrenceIdsByViewTarget, navigation.byViewTarget, key));
  }
  if (item.sourceAnchor?.objectId) {
    ids.push(...reverseOccurrenceIDs(
      cache.occurrenceIdsByObjectId,
      navigation.byObjectId,
      item.sourceAnchor.objectId,
    ));
  }
  if (hasIndex(item.sourceAnchor?.objectIndex)) {
    ids.push(...reverseOccurrenceIDs(
      cache.occurrenceIdsByObjectIndex,
      navigation.byObjectIndex,
      String(item.sourceAnchor.objectIndex),
    ));
  }
  if (item.entityId) {
    ids.push(...reverseOccurrenceIDs(cache.occurrenceIdsByEntityId, navigation.byEntityId, item.entityId));
  }
  return uniqueStrings(ids)
    .map((id) => findOccurrence(navigation, id))
    .filter((occurrence) => occurrence && (!item.entityId || occurrence.entityId === item.entityId));
}

function occurrencesForSelection(navigation, viewId, selection) {
  const cache = navigationCache(navigation);
  const ids = [];
  for (const targetId of viewTargetIdsForSelection(viewId, selection, navigation)) {
    const key = `${viewId}|${targetId}`;
    ids.push(...reverseOccurrenceIDs(cache.occurrenceIdsByViewTarget, navigation.byViewTarget, key));
  }
  if (selection.entityId) {
    ids.push(...reverseOccurrenceIDs(cache.occurrenceIdsByEntityId, navigation.byEntityId, selection.entityId));
  }
  return uniqueStrings(ids)
    .map((id) => findOccurrence(navigation, id))
    .filter((occurrence) => occurrence && (!selection.entityId || occurrence.entityId === selection.entityId));
}

function preferredOccurrence(occurrences, context = {}) {
  const currentPath = String(context.semanticPathHint || state.semanticCurrentPath || "");
  const occurrenceContext = String(context.occurrenceContext || "");
  return [...occurrences]
    .map((occurrence, order) => ({
      occurrence,
      order,
      score:
        Number(occurrence.occurrenceId === occurrenceContext) * 1_000_000_000 +
        Number(Boolean(occurrenceContext) && occurrence.path === occurrenceContext) * 900_000_000 +
        Number(String(occurrence.contextKind || "") === occurrenceContext) * 800_000_000 +
        Number(viewTargetMatches(occurrence, context.viewId, context.targetId)) * 100_000_000 +
        Number(occurrence.occurrenceId === state.semanticCurrentOccurrenceId) * 10_000_000 +
        commonPathPrefixLength(occurrence.path, currentPath) * 100_000 +
        Number(String(occurrence.preferredView || "") === context.viewId) * 10_000 +
        Number(/(^|\/)definitions?(\/|$)/i.test(occurrence.path || "")),
    }))
    .sort((left, right) => right.score - left.score || left.order - right.order)[0]?.occurrence || null;
}

function viewTargetIdsForSelection(viewId, selection, navigation) {
  const ids = [];
  const direct = selectionTargetIdForView(selection, viewId);
  if (direct) {
    ids.push(direct);
  }
  const cache = navigationCache(navigation);
  const occurrence = cache.occurrence(selection?.occurrenceId);
  const entity = cache.entity(selection?.entityId);
  for (const target of [...(occurrence?.viewTargets || []), ...(entity?.viewTargets || [])]) {
    if (String(target?.view || "").toLowerCase() === viewId && target?.targetId) {
      ids.push(String(target.targetId));
    }
  }
  return uniqueStrings(ids);
}

function selectionTargetIdForView(selection, viewId) {
  if (String(selection?.viewTarget?.view || viewId).toLowerCase() === viewId && selection?.viewTarget?.targetId) {
    return String(selection.viewTarget.targetId);
  }
  if (String(selection?.originView || "").toLowerCase() === viewId && selection?.originTargetId) {
    return String(selection.originTargetId);
  }
  return "";
}

function viewTargetMatches(occurrence, viewId, targetId) {
  if (!viewId) {
    return false;
  }
  return (occurrence?.viewTargets || []).some((target) => (
    String(target?.view || "").toLowerCase() === viewId && (!targetId || target.targetId === targetId)
  ));
}

function sourceAnchorFromElement(element) {
  const objectId = String(element.dataset.sourceObjectId || "");
  const objectIndex = optionalIndex(element.dataset.sourceObjectIndex);
  const fieldIndex = optionalIndex(element.dataset.sourceFieldIndex);
  const anchor = {
    objectId,
    objectIndex,
    objectType: String(element.dataset.sourceObjectType || ""),
    objectName: String(element.dataset.sourceObjectName || ""),
    fieldIndex,
    fieldName: String(element.dataset.sourceFieldName || ""),
  };
  return objectId || hasIndex(objectIndex) || anchor.objectType || anchor.objectName || hasIndex(fieldIndex)
    ? anchor
    : null;
}

function resultPanelRoot(viewId) {
  const hooks = hooksByView.get(viewId);
  const navigation = navigationIndex();
  const provisional = { viewId, navigation, navigationCache: navigationCache(navigation), state };
  const configured = typeof hooks?.getRoot === "function" ? hooks.getRoot(provisional) : hooks?.root;
  return configured || document.getElementById(`${viewId}Pane`);
}

function navigationIndex() {
  return state.semanticProjection?.navigation || {};
}

function navigationCache(navigation = navigationIndex()) {
  return getSemanticNavigationCache(navigation, {
    textHash: state.reportAnalysisKey || state.lastAnalyzedKey || state.analysisKey || "",
    analyzerVersion: bundledAppInfo.version,
    schemaVersion: state.semanticProjection?.schema || "",
  });
}

function closestPanelItem(element, root) {
  const target = element?.closest?.(SELECTABLE_PANEL_ITEM) || null;
  if (!target || (root && target !== root && !root.contains(target))) {
    return null;
  }
  return target;
}

function panelItems(root) {
  const descendants = [...root.querySelectorAll(SELECTABLE_PANEL_ITEM)];
  return root.matches?.(SELECTABLE_PANEL_ITEM) ? [root, ...descendants] : descendants;
}

function expandAncestorDetails(target) {
  let details = target.closest?.("details") || null;
  while (details) {
    details.open = true;
    details = details.parentElement?.closest?.("details") || null;
  }
}

function findEntity(navigation, entityId) {
  return entityId ? navigationCache(navigation).entity(entityId) : null;
}

function findOccurrence(navigation, occurrenceId) {
  return occurrenceId ? navigationCache(navigation).occurrence(occurrenceId) : null;
}

function reverseOccurrenceIDs(cacheIndex, rawIndex, key) {
  return cacheIndex.get(String(key ?? "")) || rawIndex?.[key] || [];
}

function appendARIAReference(element, attribute, referenceID) {
  const references = new Set(String(element.getAttribute(attribute) || "").split(/\s+/).filter(Boolean));
  references.add(referenceID);
  element.setAttribute(attribute, [...references].join(" "));
}

function commonPathPrefixLength(left, right) {
  const leftParts = String(left || "").split("/").filter(Boolean);
  const rightParts = String(right || "").split("/").filter(Boolean);
  let length = 0;
  while (length < leftParts.length && leftParts[length] === rightParts[length]) {
    length += 1;
  }
  return length;
}

function optionalIndex(value) {
  if (value === undefined || value === null || String(value) === "") {
    return undefined;
  }
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed >= 0 ? parsed : undefined;
}

function hasIndex(value) {
  return value !== undefined && value !== null && String(value) !== "";
}

function finiteNumber(value) {
  const number = Number(value);
  return Number.isFinite(number) ? number : 0;
}

function cloneAnchor(anchor) {
  return anchor && typeof anchor === "object" ? { ...anchor } : null;
}

function uniqueStrings(values) {
  return [...new Set((Array.isArray(values) ? values : []).map((value) => String(value || "")).filter(Boolean))];
}

function normalizeResultViewId(viewId) {
  const normalized = String(viewId || "").trim().toLowerCase();
  if (!RESULT_PANEL_NAVIGATION_VIEW_IDS.includes(normalized)) {
    throw new RangeError(`Unknown result panel navigation view: ${normalized || "(empty)"}`);
  }
  return normalized;
}

function initializeSelectionStyleListeners() {
  if (selectionListenersInitialized || typeof window === "undefined") {
    return;
  }
  selectionListenersInitialized = true;
  window.addEventListener("idfAnalyzer:semanticSelectionChanged", (event) => {
    for (const viewId of RESULT_PANEL_NAVIGATION_VIEW_IDS) {
      refreshResultPanelSelectionStyles(viewId, event.detail?.selection || state.globalSelection, state.globalHover);
    }
  });
  window.addEventListener("idfAnalyzer:semanticHoverChanged", (event) => {
    const activeView = String(state.activeResultTab || "").toLowerCase();
    if (RESULT_PANEL_NAVIGATION_VIEW_IDS.includes(activeView)) {
      refreshResultPanelSelectionStyles(activeView, state.globalSelection, event.detail?.hover || state.globalHover);
    }
  });
}
