import { elements, setStatus, state } from "./state.js";
import { analyze } from "./actions.js";
import { renderGeometry, resizeGeometry } from "./geometry-loader.js";
import { clearInputFilter, renderInputViews, switchInputView, syncRawTextToObjectField } from "./input-views.js";

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
    behavior: "smooth",
  });
}

export async function focusInputObject(target) {
  const hasObjectIndex = target.objectIndex !== undefined && target.objectIndex !== null && String(target.objectIndex) !== "";
  if (!hasObjectIndex && !target.objectType) {
    return;
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
    await switchInputView("text");
    element = findInputTarget(target);
  }
  if (!element) {
    setStatus("Object target not found in input view", "warn");
    return;
  }

  expandDetailsFor(element);
  scrollInputTargetIntoView(element);
  highlightInputTarget(element);
  if (state.syncTextRawPosition && hasObjectIndex) {
    syncRawTextToObjectField(Number(target.objectIndex));
  }
  setStatus("Input object located", "ok");
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

export function switchResultTab(tabName) {
  state.activeResultTab = ["geometry", "diagnose"].includes(tabName) ? tabName : "summary";
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
