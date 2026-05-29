import { elements } from "./state.js";

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
