// Preserve native disclosures, focus and scroll while translating an existing
// view. Navigation and filters remain owned by each view's application state.
export function captureViewPresentation(root) {
  if (!root) return () => {};
  const address = (element) => {
    if (!element || !root.contains(element)) return null;
    if (element.id) return { id: element.id };
    const path = [];
    for (let current = element; current !== root; current = current.parentElement) {
      path.unshift(Array.prototype.indexOf.call(current.parentElement.children, current));
    }
    return { path };
  };
  const find = (location) => {
    if (!location) return null;
    if (location.id) {
      const element = document.getElementById(location.id);
      return root.contains(element) ? element : null;
    }
    return location.path.reduce((element, index) => element?.children[index], root) || null;
  };
  const disclosures = [...root.querySelectorAll("details")].map((element) => ({
    location: address(element), open: element.open,
  }));
  const scroll = [root, ...root.querySelectorAll(
    '[class*="pane"], [class*="graph"], [class*="chart"], [class*="viewport"], [class*="scroll"], .input-view',
  )].map((element) => ({ location: address(element), top: element.scrollTop, left: element.scrollLeft }));
  const activeElement = document.activeElement;
  const focused = address(activeElement);
  const selection = typeof activeElement?.selectionStart === "number"
    ? [activeElement.selectionStart, activeElement.selectionEnd, activeElement.selectionDirection]
    : null;
  return () => {
    disclosures.forEach(({ location, open }) => {
      const element = find(location);
      if (element?.tagName === "DETAILS") element.open = open;
    });
    const focusTarget = find(focused);
    focusTarget?.focus?.({ preventScroll: true });
    if (selection) focusTarget?.setSelectionRange?.(...selection);
    scroll.forEach(({ location, top, left }) => {
      const element = find(location);
      if (element) { element.scrollTop = top; element.scrollLeft = left; }
    });
  };
}
