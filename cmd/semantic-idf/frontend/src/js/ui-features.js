// Keep the Semantic YAML implementation available for a low-risk rollback,
// while withholding the unfinished structure view from the user interface.
export const SHOW_SEMANTIC_STRUCTURE = false;

export function exposedInputView(viewName) {
  return !SHOW_SEMANTIC_STRUCTURE && viewName === "semantic" ? "text" : viewName;
}
