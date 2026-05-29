import { elements, setStatus, state } from "./state.js";
import { analyze } from "./actions.js";
import { renderZoneDetails, renderZoneViz } from "./analysis-views.js";
import { renderFieldTable, renderInputViews, switchInputView } from "./input-views.js";

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

export async function focusInputObject(target) {
  const hasObjectIndex = target.objectIndex !== undefined && target.objectIndex !== null && String(target.objectIndex) !== "";
  if (!hasObjectIndex && !target.objectType) {
    return;
  }
  if (state.lastAnalyzedText !== elements.idfInput.value) {
    await analyze();
  } else {
    renderInputViews();
  }

  let element = findInputTarget(target);
  if (!element && state.activeInputView === "table" && elements.fieldFilter.value) {
    elements.fieldFilter.value = "";
    renderFieldTable();
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
  element.scrollIntoView({ behavior: "smooth", block: "center", inline: "center" });
  highlightInputTarget(element);
  setStatus("Input object located", "ok");
}

export function selectZone(zoneName) {
  state.selectedZoneName = zoneName;
  renderZoneViz(state.report?.zones || []);
  renderZoneDetails(state.report?.zones || []);
}

export function handleAnalysisActivation(element) {
  if (!element) {
    return;
  }
  const zoneTab = element.closest("[data-zone-tab]");
  if (zoneTab) {
    state.selectedZoneTab = zoneTab.dataset.zoneTab;
    renderZoneDetails(state.report?.zones || []);
    return;
  }

  const zoneTarget = element.closest("[data-zone-name]");
  if (zoneTarget) {
    selectZone(zoneTarget.dataset.zoneName);
  }

  const jumpTarget = element.closest("[data-jump-object-index], [data-jump-object-type]");
  if (jumpTarget) {
    focusInputObject({
      objectIndex: jumpTarget.dataset.jumpObjectIndex,
      objectType: jumpTarget.dataset.jumpObjectType,
    });
  }
}

export function switchTab(tabName) {
  state.activeTab = tabName;
  elements.tabs.forEach((tab) => {
    tab.classList.toggle("active", tab.dataset.tab === tabName);
  });
  elements.panes.forEach((pane) => {
    pane.classList.toggle("active", pane.id === `${tabName}Pane`);
  });
}
