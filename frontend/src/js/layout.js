import { elements } from "./state.js";
import { resizeGeometry } from "./geometry-loader.js";

const workspacePresetWidths = {
  analysis: "44%",
  balanced: "50%",
  editor: "62%",
};

export function initializeWorkspaceSplitter() {
  const savedWidth = localStorage.getItem("idfAnalyzer.editorWidth");
  if (savedWidth) {
    elements.workspace.style.setProperty("--editor-width", savedWidth);
  }
  updateWorkspacePresetButtons(workspacePresetForWidth(savedWidth || workspacePresetWidths.analysis));
  elements.layoutPresetButtons?.forEach((button) => {
    button.addEventListener("click", () => applyWorkspacePreset(button.dataset.layoutPreset || "balanced"));
  });

  let dragging = false;
  let dragFrame = 0;
  let dragRect = null;
  let splitterWidth = 0;
  let pendingClientX = 0;
  let lastValue = savedWidth || "";

  function applyWorkspaceDrag() {
    dragFrame = 0;
    if (!dragging || !dragRect) {
      return;
    }
    const minLeft = 360;
    const minRight = 360;
    const nextWidth = Math.min(
      Math.max(pendingClientX - dragRect.left, minLeft),
      dragRect.width - splitterWidth - minRight,
    );
    lastValue = `${Math.round(nextWidth)}px`;
    elements.workspace.style.setProperty("--editor-width", lastValue);
  }

  elements.workspaceSplitter.addEventListener("pointerdown", (event) => {
    dragging = true;
    dragRect = elements.workspace.getBoundingClientRect();
    splitterWidth = elements.workspaceSplitter.getBoundingClientRect().width;
    pendingClientX = event.clientX;
    elements.workspaceSplitter.setPointerCapture(event.pointerId);
    document.body.classList.add("resizing-workspace");
  });

  elements.workspaceSplitter.addEventListener("pointermove", (event) => {
    if (!dragging) {
      return;
    }
    pendingClientX = event.clientX;
    if (!dragFrame) {
      dragFrame = window.requestAnimationFrame(applyWorkspaceDrag);
    }
  });

  function stopDrag(event) {
    if (!dragging) {
      return;
    }
    if (event.clientX !== undefined) {
      pendingClientX = event.clientX;
    }
    if (dragFrame) {
      window.cancelAnimationFrame(dragFrame);
      dragFrame = 0;
    }
    applyWorkspaceDrag();
    dragging = false;
    dragRect = null;
    if (lastValue) {
      localStorage.setItem("idfAnalyzer.editorWidth", lastValue);
    }
    resizeGeometry();
    if (event.pointerId !== undefined) {
      try {
        elements.workspaceSplitter.releasePointerCapture(event.pointerId);
      } catch {
        // Pointer capture may already be released by the browser.
      }
    }
    document.body.classList.remove("resizing-workspace");
  }

  elements.workspaceSplitter.addEventListener("pointerup", stopDrag);
  elements.workspaceSplitter.addEventListener("pointercancel", stopDrag);
}

export function applyWorkspacePreset(preset) {
  const nextWidth = workspacePresetWidths[preset] || workspacePresetWidths.balanced;
  elements.workspace?.style.setProperty("--editor-width", nextWidth);
  localStorage.setItem("idfAnalyzer.editorWidth", nextWidth);
  updateWorkspacePresetButtons(workspacePresetForWidth(nextWidth));
  resizeGeometry();
}

function workspacePresetForWidth(width) {
  return Object.entries(workspacePresetWidths).find(([, value]) => value === width)?.[0] || "";
}

function updateWorkspacePresetButtons(activePreset) {
  elements.layoutPresetButtons?.forEach((button) => {
    button.classList.toggle("active", button.dataset.layoutPreset === activePreset);
  });
}

export function initializeVerticalSplitters() {
  initializeHeightSplitter({
    container: elements.editorPanel,
    splitter: elements.inputRawSplitter,
    property: "--raw-height",
    storageKey: "idfAnalyzer.rawHeight",
    minTop: 170,
    minBottom: 160,
    resizingClass: "resizing-input-raw",
    onResize: null,
  });

  initializeHeightSplitter({
    container: elements.geometryBody,
    splitter: elements.geometryDetailsSplitter,
    property: "--geometry-details-height",
    storageKey: "idfAnalyzer.geometryDetailsHeight",
    minTop: 220,
    minBottom: 150,
    resizingClass: "resizing-geometry-details",
    onResize: resizeGeometry,
    onResizeEnd: resizeGeometry,
  });
}

function initializeHeightSplitter({
  container,
  splitter,
  property,
  storageKey,
  minTop,
  minBottom,
  resizingClass,
  onResize,
  onResizeEnd,
}) {
  if (!container || !splitter) {
    return;
  }

  const savedHeight = localStorage.getItem(storageKey);
  if (savedHeight) {
    container.style.setProperty(property, savedHeight);
  }

  let dragging = false;
  let dragFrame = 0;
  let dragRect = null;
  let splitterHeight = 0;
  let pendingClientY = 0;
  let lastValue = savedHeight || "";

  function applyHeightDrag() {
    dragFrame = 0;
    if (!dragging || !dragRect) {
      return;
    }
    const maxBottom = Math.max(minBottom, dragRect.height - splitterHeight - minTop);
    const nextHeight = Math.min(
      Math.max(dragRect.bottom - pendingClientY, minBottom),
      maxBottom,
    );
    lastValue = `${Math.round(nextHeight)}px`;
    container.style.setProperty(property, lastValue);
    if (typeof onResize === "function") {
      onResize();
    }
  }

  splitter.addEventListener("pointerdown", (event) => {
    dragging = true;
    dragRect = container.getBoundingClientRect();
    splitterHeight = splitter.getBoundingClientRect().height;
    pendingClientY = event.clientY;
    splitter.setPointerCapture(event.pointerId);
    document.body.classList.add(resizingClass);
  });

  splitter.addEventListener("pointermove", (event) => {
    if (!dragging) {
      return;
    }
    pendingClientY = event.clientY;
    if (!dragFrame) {
      dragFrame = window.requestAnimationFrame(applyHeightDrag);
    }
  });

  function stopDrag(event) {
    if (!dragging) {
      return;
    }
    if (event.clientY !== undefined) {
      pendingClientY = event.clientY;
    }
    if (dragFrame) {
      window.cancelAnimationFrame(dragFrame);
      dragFrame = 0;
    }
    applyHeightDrag();
    dragging = false;
    dragRect = null;
    if (lastValue) {
      localStorage.setItem(storageKey, lastValue);
    }
    if (typeof onResizeEnd === "function") {
      onResizeEnd();
    }
    if (event.pointerId !== undefined) {
      try {
        splitter.releasePointerCapture(event.pointerId);
      } catch {
        // Pointer capture may already be released by the browser.
      }
    }
    document.body.classList.remove(resizingClass);
  }

  splitter.addEventListener("pointerup", stopDrag);
  splitter.addEventListener("pointercancel", stopDrag);
}
