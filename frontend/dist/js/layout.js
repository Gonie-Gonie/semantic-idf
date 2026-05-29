import { elements } from "./state.js";
import { resizeGeometry } from "./geometry-view.js";

export function initializeWorkspaceSplitter() {
  const savedWidth = localStorage.getItem("idfAnalyzer.editorWidth");
  if (savedWidth) {
    elements.workspace.style.setProperty("--editor-width", savedWidth);
  }

  let dragging = false;
  elements.workspaceSplitter.addEventListener("pointerdown", (event) => {
    dragging = true;
    elements.workspaceSplitter.setPointerCapture(event.pointerId);
    document.body.classList.add("resizing-workspace");
  });

  elements.workspaceSplitter.addEventListener("pointermove", (event) => {
    if (!dragging) {
      return;
    }
    const rect = elements.workspace.getBoundingClientRect();
    const splitterWidth = elements.workspaceSplitter.getBoundingClientRect().width;
    const minLeft = 420;
    const minRight = 420;
    const nextWidth = Math.min(
      Math.max(event.clientX - rect.left, minLeft),
      rect.width - splitterWidth - minRight,
    );
    const value = `${Math.round(nextWidth)}px`;
    elements.workspace.style.setProperty("--editor-width", value);
    localStorage.setItem("idfAnalyzer.editorWidth", value);
  });

  function stopDrag(event) {
    if (!dragging) {
      return;
    }
    dragging = false;
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
  });
}

function initializeHeightSplitter({ container, splitter, property, storageKey, minTop, minBottom, resizingClass, onResize }) {
  if (!container || !splitter) {
    return;
  }

  const savedHeight = localStorage.getItem(storageKey);
  if (savedHeight) {
    container.style.setProperty(property, savedHeight);
  }

  let dragging = false;
  splitter.addEventListener("pointerdown", (event) => {
    dragging = true;
    splitter.setPointerCapture(event.pointerId);
    document.body.classList.add(resizingClass);
  });

  splitter.addEventListener("pointermove", (event) => {
    if (!dragging) {
      return;
    }
    const rect = container.getBoundingClientRect();
    const splitterHeight = splitter.getBoundingClientRect().height;
    const maxBottom = Math.max(minBottom, rect.height - splitterHeight - minTop);
    const nextHeight = Math.min(
      Math.max(rect.bottom - event.clientY, minBottom),
      maxBottom,
    );
    const value = `${Math.round(nextHeight)}px`;
    container.style.setProperty(property, value);
    localStorage.setItem(storageKey, value);
    if (typeof onResize === "function") {
      onResize();
    }
  });

  function stopDrag(event) {
    if (!dragging) {
      return;
    }
    dragging = false;
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
