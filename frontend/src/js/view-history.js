import { elements, state } from "./state.js";
import { getPanelNavigationAdapter, PANEL_NAVIGATION_VIEW_IDS } from "./panel-navigation-registry.js";

const MAX_HISTORY = 80;
const MAX_CONTEXT_ARRAY_LENGTH = 96;
const MAX_CONTEXT_DEPTH = 7;

export function captureViewSnapshot() {
  return {
    inputView: state.activeInputView,
    resultTab: state.activeResultTab,
    jsonSelectedObjectIndex: state.jsonSelectedObjectIndex,
    rawSelectionStart: elements.idfInput?.selectionStart ?? 0,
    rawSelectionEnd: elements.idfInput?.selectionEnd ?? 0,
    rawScrollTop: elements.idfInput?.scrollTop ?? 0,
    rawScrollLeft: elements.idfInput?.scrollLeft ?? 0,
    textScrollTop: elements.textObjectView?.scrollTop ?? 0,
    jsonScrollTop: elements.jsonStructuredView?.scrollTop ?? 0,
    tableScrollTop: elements.fieldTable?.scrollTop ?? 0,
    activeObjectIndex: activeElementDataset("objectIndex"),
    activeFieldIndex: activeElementDataset("fieldIndex"),
    activeFieldIndexKind: activeElementDataset("fieldIndexKind"),
    globalSelection: cloneHistorySelection(state.globalSelection),
    semanticTemporaryReveal: state.semanticTemporaryReveal ? { ...state.semanticTemporaryReveal } : null,
    semanticCurrentOccurrenceId: state.semanticCurrentOccurrenceId || "",
    semanticCurrentPath: state.semanticCurrentPath || "",
    semantic: {
      mode: state.semanticProjectionMode || "basic",
      facet: state.semanticProjectionFacet || "all",
      filter: state.inputFilterQuery || "",
      temporaryReveal: compactClone(state.semanticTemporaryReveal),
      scrollTop: Number(elements.semanticEditor?.scrollTop) || 0,
      expandedSectionIds: [...(state.semanticExpandedSectionIds || [])].map(String),
    },
    panelContexts: capturePanelContexts(),
  };
}

export function recordViewHistory(snapshot = captureViewSnapshot()) {
  if (state.navigationRestoring) {
    return;
  }
  const previous = state.navigationUndoStack[state.navigationUndoStack.length - 1];
  if (previous && snapshotsEqual(previous, snapshot)) {
    return;
  }
  state.navigationUndoStack.push(snapshot);
  if (state.navigationUndoStack.length > MAX_HISTORY) {
    state.navigationUndoStack.shift();
  }
  state.navigationRedoStack = [];
}

export function popUndoSnapshot(current = captureViewSnapshot()) {
  const previous = state.navigationUndoStack.pop();
  if (!previous) {
    return null;
  }
  state.navigationRedoStack.push(current);
  return previous;
}

export function popRedoSnapshot(current = captureViewSnapshot()) {
  const next = state.navigationRedoStack.pop();
  if (!next) {
    return null;
  }
  state.navigationUndoStack.push(current);
  return next;
}

export function withHistoryRestore(callback) {
  state.navigationRestoring = true;
  return Promise.resolve()
    .then(callback)
    .finally(() => {
      state.navigationRestoring = false;
    });
}

function activeElementDataset(key) {
  const target = document.activeElement?.closest?.("[data-object-index], [data-field-index]");
  return target?.dataset?.[key] ?? "";
}

function snapshotsEqual(a, b) {
  return JSON.stringify(a) === JSON.stringify(b);
}

function cloneHistorySelection(selection = {}) {
  return {
    entityId: String(selection.entityId || ""),
    entityKind: String(selection.entityKind || ""),
    occurrenceId: String(selection.occurrenceId || ""),
    sourceAnchor: selection.sourceAnchor ? { ...selection.sourceAnchor } : null,
    originView: String(selection.originView || ""),
    originTargetId: String(selection.originTargetId || ""),
    semanticPathHint: String(selection.semanticPathHint || ""),
    relatedEntityIds: [...(selection.relatedEntityIds || [])],
    transactionId: "",
  };
}

/**
 * Panel adapters own panel-local state. Capturing through the registry keeps
 * history lightweight and prevents this module from importing seven views.
 */
export function capturePanelContexts(viewIDs = PANEL_NAVIGATION_VIEW_IDS) {
  const contexts = {};
  for (const viewID of viewIDs) {
    const adapter = getPanelNavigationAdapter(viewID);
    if (!adapter) {
      continue;
    }
    try {
      const context = compactClone(adapter.captureContext());
      if (context !== undefined) {
        contexts[viewID] = context;
      }
    } catch {
      // A panel that has not mounted yet must not break global history.
    }
  }
  return contexts;
}

function compactClone(value, depth = 0, seen = new WeakSet()) {
  if (value === null || value === undefined || typeof value === "string" || typeof value === "boolean") {
    return value;
  }
  if (typeof value === "number") {
    return Number.isFinite(value) ? value : 0;
  }
  if (typeof value !== "object" || depth >= MAX_CONTEXT_DEPTH) {
    return undefined;
  }
  if (seen.has(value)) {
    return undefined;
  }
  seen.add(value);
  if (Array.isArray(value)) {
    return value
      .slice(0, MAX_CONTEXT_ARRAY_LENGTH)
      .map((item) => compactClone(item, depth + 1, seen))
      .filter((item) => item !== undefined);
  }
  if (value instanceof Set) {
    return compactClone([...value], depth + 1, seen);
  }
  if (value instanceof Map || (globalThis.HTMLElement && value instanceof globalThis.HTMLElement)) {
    return undefined;
  }
  const result = {};
  for (const [key, item] of Object.entries(value)) {
    const cloned = compactClone(item, depth + 1, seen);
    if (cloned !== undefined) {
      result[key] = cloned;
    }
  }
  return result;
}
