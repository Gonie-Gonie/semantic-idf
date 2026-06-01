import { elements, setStatus, state } from "./state.js";
import { t } from "./i18n.js";
import { analyze } from "./actions.js";
import { renderGeometry, resizeGeometry } from "./geometry-loader.js";
import {
  clearInputFilter,
  currentInputJumpSource,
  jumpSourceForContext,
  renderInputViews,
  resolveInputJumpTargets,
  switchInputView,
  syncRawTextToObjectField,
} from "./input-views.js";
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
  const container = element.closest(".formatted-object-view, .json-view, .field-table");
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

export async function focusInputObject(target, options = {}) {
  const hasObjectIndex = target.objectIndex !== undefined && target.objectIndex !== null && String(target.objectIndex) !== "";
  if (!hasObjectIndex && !target.objectType) {
    return;
  }
  if (options.recordHistory !== false) {
    recordViewHistory();
  }
  if (hasObjectIndex) {
    state.jsonSelectedObjectIndex = String(target.objectIndex);
  }
  if (state.lastAnalyzedText !== elements.idfInput.value) {
    await analyze();
  } else {
    renderInputViews();
  }

  let element = findInputTarget(target);
  if (!element && state.inputFilterQuery) {
    clearInputFilter();
    element = findInputTarget(target);
  }
  if (!element && state.activeInputView !== "text") {
    await switchInputView("text", { recordHistory: false });
    element = findInputTarget(target);
  }
  if (!element) {
    setStatus(t("input.objectTargetMissing"), "warn");
    return;
  }

  expandDetailsFor(element);
  if (state.syncTextRawPosition && hasObjectIndex) {
    const fieldIndex = target.fieldIndex === undefined || target.fieldIndex === null || String(target.fieldIndex) === "" ? null : Number(target.fieldIndex);
    syncRawTextToObjectField(Number(target.objectIndex), fieldIndex, target.fieldIndexKind || "idf");
  }
  scrollInputTargetIntoView(element);
  highlightInputTarget(element);
  focusNavigatedInputTarget(element);
  setStatus(t("input.objectLocated"), "ok");
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

export function jumpInputDefinition() {
  jumpFromInputSource("definition", currentInputJumpSource());
}

export function jumpInputReferences() {
  jumpFromInputSource("references", currentInputJumpSource());
}

function jumpFromInputSource(kind, source) {
  const targets = resolveInputJumpTargets(kind, source);
  if (!targets.length) {
    setStatus(kind === "definition" ? t("input.noDefinitionTarget") : t("input.noReferenceTarget"), "warn");
    return;
  }
  const target = kind === "references" ? nextReferenceTarget(source, targets) : targets[0];
  recordViewHistory();
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
  }
}

export function switchResultTab(tabName, options = {}) {
  if (options.recordHistory !== false && state.activeResultTab !== tabName) {
    recordViewHistory();
  }
  state.activeResultTab = ["profile", "hvac", "geometry", "diagnose"].includes(tabName) ? tabName : "summary";
  elements.resultTabButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.resultTab === state.activeResultTab);
  });
  elements.resultPanes.forEach((pane) => {
    pane.classList.toggle("active", pane.id === `${state.activeResultTab}Pane`);
  });
  if (state.activeResultTab === "geometry") {
    window.setTimeout(() => {
      renderGeometry();
      resizeGeometry();
    }, 0);
  }
}

export async function undoViewNavigation() {
  const snapshot = popUndoSnapshot(captureViewSnapshot());
  if (!snapshot) {
    setStatus(t("status.noViewHistory"), "warn");
    return;
  }
  await restoreViewSnapshot(snapshot);
}

export async function redoViewNavigation() {
  const snapshot = popRedoSnapshot(captureViewSnapshot());
  if (!snapshot) {
    setStatus(t("status.noViewHistory"), "warn");
    return;
  }
  await restoreViewSnapshot(snapshot);
}

async function restoreViewSnapshot(snapshot) {
  await withHistoryRestore(async () => {
    state.jsonSelectedObjectIndex = snapshot.jsonSelectedObjectIndex || "";
    if (snapshot.resultTab && snapshot.resultTab !== state.activeResultTab) {
      switchResultTab(snapshot.resultTab, { recordHistory: false });
    }
    if (snapshot.inputView && snapshot.inputView !== state.activeInputView) {
      await switchInputView(snapshot.inputView, { recordHistory: false });
    } else {
      renderInputViews();
    }
    if (snapshot.activeObjectIndex !== undefined && snapshot.activeObjectIndex !== null && String(snapshot.activeObjectIndex) !== "") {
      await focusInputObject({
        objectIndex: snapshot.activeObjectIndex,
        fieldIndex: snapshot.activeFieldIndex,
        fieldIndexKind: snapshot.activeFieldIndexKind || "idf",
      }, { recordHistory: false });
    }
    restoreRawEditorPosition(snapshot);
    restoreViewScrolls(snapshot);
  });
  setStatus(t("status.viewHistoryRestored"), "ok");
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
}
