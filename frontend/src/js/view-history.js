import { elements, state } from "./state.js";

const MAX_HISTORY = 80;

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
