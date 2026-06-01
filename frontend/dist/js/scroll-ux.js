const horizontalScrollSelector = [
  ".object-type-table-scroll",
  ".profile-item-table",
  ".profile-matrix",
  ".tool-table-wrap",
  ".hvac-graph",
].join(",");

export function initializeScrollUX(root = document) {
  root.addEventListener("wheel", handleHorizontalWheel, { passive: false });
}

function handleHorizontalWheel(event) {
  if (event.defaultPrevented || event.ctrlKey) {
    return;
  }
  const scroller = event.target?.closest?.(horizontalScrollSelector);
  if (!scroller || !hasHorizontalOverflow(scroller)) {
    return;
  }

  const horizontalIntent = event.shiftKey || Math.abs(event.deltaX) > Math.abs(event.deltaY);
  const verticalOverflow = scroller.scrollHeight > scroller.clientHeight + 1;
  if (!horizontalIntent && verticalOverflow) {
    return;
  }

  const delta = Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY;
  if (!delta) {
    return;
  }
  const before = scroller.scrollLeft;
  scroller.scrollLeft += delta;
  if (scroller.scrollLeft !== before) {
    event.preventDefault();
  }
}

function hasHorizontalOverflow(element) {
  return element.scrollWidth > element.clientWidth + 1;
}
