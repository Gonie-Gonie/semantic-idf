const INPUT_VIEWS = new Set(["text", "json", "table"]);

// Saved preferences and history may name a retired or unknown view. Every
// entry point resolves them to a real source editor rather than a hidden pane.
export function normalizeInputView(viewName) {
  const view = String(viewName || "").trim().toLowerCase();
  return INPUT_VIEWS.has(view) ? view : "text";
}
