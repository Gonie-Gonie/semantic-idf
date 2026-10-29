// Temporal plot interaction is independent of values and Y axes. Renderers keep
// complete observations and redraw only the selected horizontal window.
const finite = (value) => typeof value === "number" && Number.isFinite(value);
const pending = new Map(), charts = new WeakMap(), hosts = new WeakMap();
let sequence = 0;

export function zoomTimeSeriesRange(range, extent, ratio, deltaPixels, minSpan = 0) {
  const [low, high] = extent, full = high - low;
  if (![low, high, range.low, range.high, ratio, deltaPixels].every(finite) || full <= 0) return range;
  const span = range.high - range.low, anchor = range.low + span * Math.max(0, Math.min(1, ratio));
  const next = Math.min(full, Math.max(Math.min(full, minSpan || full / 1000), span * Math.exp(Math.max(-1, Math.min(1, deltaPixels / 450)))));
  const start = Math.max(low, Math.min(high - next, anchor - next * Math.max(0, Math.min(1, ratio))));
  return { low: start, high: start + next };
}

// Boundary neighbours preserve line entry without filling missing intervals.
export function timeSeriesWindowPoints(points, range, neighbors = true) {
  if (!range || !points.length) return points;
  const [low, high] = range;
  let first = 0;
  while (first < points.length && points[first].x < low) first++;
  let last = first;
  while (last < points.length && points[last].x <= high) last++;
  return points.slice(neighbors ? Math.max(0, first - 1) : first, neighbors ? Math.min(points.length, last + 1) : last);
}

// Labels occupy their actual viewport positions, including sparse clusters
// beside a large time gap. Keep observed timestamps, never fabricate axis data.
export function observedTimeTicks(points, { x, plotLeft, plotWidth, measure, limit = 5, gap = 10 }) {
  const right = plotLeft + plotWidth;
  if (!finite(plotLeft) || !finite(plotWidth) || plotWidth <= 0 || typeof x !== "function" || typeof measure !== "function") return [];
  const unique = [...new Map(points.filter((point) => finite(point?.x) && x(point.x) >= plotLeft && x(point.x) <= right)
    .map((point) => [point.x, point])).values()].sort((a, b) => a.x - b.x);
  if (!unique.length) return [];
  const count = Math.max(1, Math.floor(limit)), candidates = [unique[0], unique.at(-1)];
  if (unique.length <= count) candidates.push(...unique.slice(1, -1));
  else {
    const fractions = Array.from({ length: Math.max(0, count - 2) }, (_, index) => (index + 1) / (count - 1))
      .sort((a, b) => Math.abs(a - .5) - Math.abs(b - .5));
    for (const fraction of fractions) {
      const target = plotLeft + plotWidth * fraction;
      let closest = unique[0];
      for (const point of unique) if (Math.abs(x(point.x) - target) < Math.abs(x(closest.x) - target)) closest = point;
      candidates.push(closest);
    }
  }
  const selected = [], seen = new Set(), inkGuard = 2;
  for (const point of candidates) {
    if (selected.length >= count || seen.has(point.x)) continue;
    seen.add(point.x);
    const position = x(point.x), width = measure(String(point.label ?? point.x));
    if (!finite(width) || width < 0) continue;
    // Glyph ink can extend beyond its advance width, especially at an end
    // anchor. Inset only the text; its grid line stays at the observed time.
    const anchor = position - width / 2 < plotLeft + inkGuard ? "start" : position + width / 2 > right - inkGuard ? "end" : "middle";
    const labelX = anchor === "start" ? Math.max(position, plotLeft + inkGuard) : anchor === "end" ? Math.min(position, right - inkGuard) : position;
    const textLow = anchor === "start" ? labelX : anchor === "end" ? labelX - width : labelX - width / 2;
    const low = textLow - inkGuard, high = textLow + width + inkGuard;
    if (low < plotLeft || high > right || selected.some((tick) => low < tick.high + gap && high > tick.low - gap)) continue;
    selected.push({ ...point, anchor, labelX, low, high });
  }
  return selected.sort((a, b) => a.x - b.x);
}

export function registerTimeSeriesChart({ extent, range = extent, minSpan = 0, plotLeft, plotWidth, render }) {
  if (!extent?.every(finite) || extent[1] <= extent[0] || typeof render !== "function") return "";
  const id = String(++sequence);
  pending.set(id, { extent, range: { low: range[0], high: range[1] }, minSpan, render, raf: null });
  // Mounted SVGs own their definitions weakly. Abandoned string renders cannot
  // retain an unbounded history of result data while waiting to be installed.
  while (pending.size > 64) pending.delete(pending.keys().next().value);
  return ` data-time-series-chart="${id}" data-time-series-low="${range[0]}" data-time-series-high="${range[1]}" data-time-series-extent-low="${extent[0]}" data-time-series-extent-high="${extent[1]}" data-time-series-plot-left="${plotLeft}" data-time-series-plot-width="${plotWidth}"`;
}

function claim(svg) {
  if (charts.has(svg)) return charts.get(svg);
  const id = svg.dataset.timeSeriesChart, state = pending.get(id);
  if (state) { pending.delete(id); charts.set(svg, state); }
  return state;
}

function mount(host) {
  if (host.matches?.("svg[data-time-series-chart]")) claim(host);
  for (const svg of host.querySelectorAll?.("svg[data-time-series-chart]") || []) claim(svg);
}

function paint(svg, state) {
  state.raf = null;
  if (!svg.isConnected) return;
  const rawFrame = svg.dataset.timeSeriesFrame, frame = rawFrame !== undefined && rawFrame !== "" ? Number(rawFrame) : undefined;
  const template = document.createElement("template");
  template.innerHTML = state.render([state.range.low, state.range.high], finite(frame) ? frame : undefined);
  const next = template.content.querySelector("svg");
  if (!next) return;
  // Preserve controls, focus, the SVG, legends and the horizontal scroller.
  for (const attribute of next.attributes) svg.setAttribute(attribute.name, attribute.value);
  svg.replaceChildren(...next.childNodes);
  svg.dataset.timeSeriesLow = state.range.low;
  svg.dataset.timeSeriesHigh = state.range.high;
  const highlight = svg.closest("[data-hvac-chart-highlight]")?.dataset.hvacChartHighlight;
  if (highlight !== undefined) for (const trace of svg.querySelectorAll("[data-hvac-chart-series]")) {
    trace.classList.toggle("is-highlighted", trace.dataset.hvacChartSeries === highlight);
    trace.classList.toggle("is-muted", trace.dataset.hvacChartSeries !== highlight);
  }
}

export function handleTimeSeriesChartEvent(event, host) {
  if (!["wheel", "dblclick"].includes(event.type)) return false;
  const svg = event.target?.closest?.("svg[data-time-series-chart]");
  if (!svg || !host?.contains(svg)) return false;
  const state = claim(svg);
  if (!state) return false;
  if (event.type === "dblclick") state.range = { low: state.extent[0], high: state.extent[1] };
  else {
    const delta = event.deltaY || event.deltaX || 0;
    if (!delta) return false;
    const rect = svg.getBoundingClientRect(), width = svg.viewBox.baseVal.width;
    const position = (event.clientX - rect.left) / Math.max(1, rect.width) * width;
    const ratio = (position - Number(svg.dataset.timeSeriesPlotLeft)) / Number(svg.dataset.timeSeriesPlotWidth);
    const pixels = delta * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? rect.height : 1);
    state.range = zoomTimeSeriesRange(state.range, state.extent, ratio, pixels, state.minSpan);
  }
  event.preventDefault();
  if (state.raf === null) state.raf = requestAnimationFrame(() => paint(svg, state));
  return true;
}

export function connectTimeSeriesCharts(host) {
  if (!host || hosts.has(host)) return;
  mount(host);
  const observer = new MutationObserver((records) => {
    for (const record of records) for (const node of record.addedNodes) if (node.nodeType === 1) mount(node);
  });
  observer.observe(host, { childList: true, subtree: true });
  host.addEventListener("wheel", (event) => handleTimeSeriesChartEvent(event, host), { passive: false });
  host.addEventListener("dblclick", (event) => handleTimeSeriesChartEvent(event, host));
  hosts.set(host, observer);
}
