import { getLanguage, t } from "../i18n.js";
import { escapeHTML } from "../state.js";
import { heatFlowCategoryKind, heatFlowCategoryValue, heatFlowCategoryLabel, heatFlowExactWatts, formatHeatFlowWatts, heatFlowBalance, heatFlowFrameTime } from "../heat-flow-data.js";

const finite = (value) => typeof value === "number" && Number.isFinite(value);
const number = (value) => String(Number(value.toFixed(6)));
const copy = (key, fallback, parameters = {}) => t(`simulation.${key}`, parameters, fallback);
const color = (category) => category.id === "airStorage" ? "var(--heatflow-storage-color)" : category.id === "deviation" ? "var(--heatflow-deviation-color)" : category.color || "var(--muted)";
const layoutCache = new WeakMap();
const renderContexts = new WeakMap();
const cacheLimit = 4;
let renderSequence = 0;

/** Every supplied frame is drawn, without stride sampling or averaging. Local
 * gains/losses, boundary exchanges, and reported diagnostics have separate panels
 * with the same signed scale. Missing observations create gaps, never zeroes. */
export function renderHeatFlowStackChart(dataset, zoneSeries, frameIndex, { start = 0, end } = {}) {
  const layout = frameLayout(dataset, { start, end });
  if (!layout || !zoneSeries) return `<div class="empty">${escapeHTML(copy("noFrame", "No frame"))}</div>`;
  ({ start, end } = layout);
  const categories = (dataset.categories || []).map((category, index) => ({ ...category, index }));
  const groupDefinitions = [
    { kind: "local", title: copy("heatFlowLocalChart", "Local zone gains / losses") },
    { kind: "exchange", title: copy("heatFlowExchangeChart", "Boundary exchange") },
    { kind: "diagnostic", title: copy("heatFlowDiagnostics", "Storage and balance check · reported signs") },
  ];
  let maxAbs = 1;
  const groups = groupDefinitions.map((group) => {
    const items = categories.filter((category) => heatFlowCategoryKind(category) === group.kind);
    const frames = [];
    for (let frame = start; frame <= end; frame++) {
      const values = items.map((category) => heatFlowCategoryValue(zoneSeries, category.index, frame));
      const complete = values.every(finite);
      let positive = 0, negative = 0;
      if (group.kind === "diagnostic") {
        for (const value of values) if (finite(value)) maxAbs = Math.max(maxAbs, Math.abs(value));
      } else if (complete) {
        for (const value of values) value >= 0 ? positive += value : negative += value;
        maxAbs = Math.max(maxAbs, positive, -negative);
      }
      frames.push({ frame, values, complete });
    }
    return { ...group, items, frames };
  }).filter((group) => group.items.length);
  const extent = niceExtent(maxAbs);
  const unit = extent >= 1e6 ? "MW" : extent >= 1000 ? "kW" : "W";
  const divisor = unit === "MW" ? 1e6 : unit === "kW" ? 1000 : 1;
  const selected = finite(frameIndex) && frameIndex >= start && frameIndex <= end ? Math.floor(frameIndex) : null;
  const label = (frame) => String(dataset.labels?.[frame] || `#${frame + 1}`);
  const missingCount = groups.filter((group) => group.kind !== "diagnostic").reduce((sum, group) => sum + group.frames.filter((row) => !row.complete).length, 0);
  const selectedMissing = selectedFrameMissing(dataset, zoneSeries, selected);
  const context = rememberRenderContext(dataset, { zoneSeries, start, end, language: getLanguage() });
  return `<section class="heatflow-history" data-heatflow-history data-heatflow-chart-context="${context}" data-heatflow-chart-missing-count="${missingCount}">
    <div class="heatflow-chart-heading"><h4>${escapeHTML(copy("heatFlowHistory", "Heat-flow history"))}${zoneSeries.name ? ` · <span class="heatflow-chart-zone">${escapeHTML(zoneSeries.name)}</span>` : ""}</h4><span class="heatflow-chart-current" data-heatflow-chart-current>${escapeHTML(selected == null ? "" : label(selected))}</span></div>
    <p class="heatflow-chart-missing" role="status"${selectedMissing ? "" : " hidden"}>${escapeHTML(copy("heatFlowMissingFrame", "Incomplete observations at this frame"))}</p>
    ${layout.elapsed ? "" : `<p class="heatflow-chart-sequence-note">${escapeHTML(copy("heatFlowRecordedFrameNote", "Timestamps are missing, repeated or out of order. Observations are shown in their recorded sequence."))}</p>`}
    <div class="heatflow-chart-panels">${groups.map((group) => renderPanel(group, { zoneSeries, start, end, selected, extent, unit, divisor, label, layout })).join("")}</div>
    <p class="heatflow-chart-help">${escapeHTML(copy("heatFlowChartHelp", "Click the graph to select a time; scroll to zoom the time range."))}</p>
  </section>`;
}

/** Update the selected observation without replacing static SVG paths or axes.
 * False means the existing render belongs to another dataset/zone/range/language
 * or its expected dynamic elements are absent; the caller can render it fully. */
export function updateHeatFlowStackChart(host, dataset, zoneSeries, frameIndex, range = {}) {
  const history = host?.matches?.("[data-heatflow-history]") ? host : host?.querySelector?.("[data-heatflow-history]");
  const context = renderContexts.get(dataset)?.get(history?.dataset.heatflowChartContext);
  const normalized = normalizedRange(dataset, range);
  if (!context || !normalized || context.zoneSeries !== zoneSeries || context.language !== getLanguage()
    || context.start !== normalized.start || context.end !== normalized.end) return false;
  const current = history.querySelector("[data-heatflow-chart-current]");
  const missing = history.querySelector(".heatflow-chart-missing");
  if (!current || !missing) return false;
  const layout = frameLayout(dataset, normalized);
  const selected = finite(frameIndex) && frameIndex >= layout.start && frameIndex <= layout.end ? Math.floor(frameIndex) : null;
  const categories = dataset.categories || [];
  const legendNodes = [...history.querySelectorAll("[data-heatflow-chart-legend]")];
  const cursors = [...history.querySelectorAll(".heatflow-cursor")];
  if (legendNodes.length !== categories.length || cursors.length !== history.querySelectorAll("[data-heatflow-chart-panel]").length) return false;
  const legendUpdates = legendNodes.map(node => {
    const index = categories.findIndex(category => category.id === node.dataset.heatflowChartLegend);
    return { node, index, valueNode: node.querySelector("[data-heatflow-legend-value]") };
  });
  const cursorUpdates = cursors.map(cursor => {
    const svg = cursor.closest("svg"), hit = svg?.querySelector("[data-heatflow-chart]");
    return { cursor, svg, left: Number(hit?.getAttribute("x")), width: Number(hit?.getAttribute("width")) };
  });
  if (legendUpdates.some(item => item.index < 0 || !item.valueNode)
    || cursorUpdates.some(item => !finite(item.left) || !finite(item.width) || item.width <= 0
      || Number(item.svg?.dataset.heatflowChartStart) !== layout.start || Number(item.svg?.dataset.heatflowChartEnd) !== layout.end)) return false;
  current.textContent = selected == null ? "" : String(dataset.labels?.[selected] || `#${selected + 1}`);
  missing.hidden = !selectedFrameMissing(dataset, zoneSeries, selected);
  for (const { cursor, left, width } of cursorUpdates) {
    const x = selected == null ? left : left + layout.ratio(selected) * width;
    cursor.setAttribute("x1", number(x));
    cursor.setAttribute("x2", number(x));
    cursor.dataset.heatflowChartFrame = selected ?? "";
    cursor.style.display = selected == null ? "none" : "";
  }
  for (const { node, index, valueNode } of legendUpdates) {
    const value = selected == null ? NaN : heatFlowCategoryValue(zoneSeries, index, selected);
    node.title = `${heatFlowCategoryLabel(categories[index])}: ${heatFlowExactWatts(value)}`;
    valueNode.dataset.heatflowLegendValue = finite(value) ? value : "";
    valueNode.textContent = formatHeatFlowWatts(value);
  }
  return true;
}

function selectedFrameMissing(dataset, zoneSeries, selected) {
  return selected != null && (!heatFlowBalance(dataset, zoneSeries, selected).complete
    || (dataset.categories || []).some((_category, index) => !finite(heatFlowCategoryValue(zoneSeries, index, selected))));
}

function rememberRenderContext(dataset, context) {
  let contexts = renderContexts.get(dataset);
  if (!contexts) { contexts = new Map(); renderContexts.set(dataset, contexts); }
  const token = String(++renderSequence);
  contexts.set(token, context);
  while (contexts.size > cacheLimit) contexts.delete(contexts.keys().next().value);
  return token;
}

function renderPanel(group, { zoneSeries, start, end, selected, extent, unit, divisor, label, layout }) {
  const width = 760, height = 300, left = 116, right = 20, top = 36, bottom = 72;
  const plotWidth = width - left - right, plotHeight = height - top - bottom;
  const yZero = top + plotHeight / 2;
  const y = (value) => yZero - value / extent * plotHeight / 2;
  // These are observation glyphs, not energy-integral columns. Their widths do
  // not fill missing/sampled time intervals or imply a constant rate in a gap.
  const barWidth = Math.min(8, plotWidth * layout.cadence / layout.span);
  const x = (frame) => left + layout.ratio(frame) * plotWidth;
  const paths = group.items.map(() => []);
  if (group.kind !== "diagnostic") {
    for (const row of group.frames) {
      if (!row.complete) continue;
      let positive = 0, negative = 0;
      row.values.forEach((value, index) => {
        const baseline = value >= 0 ? positive : negative;
        if (value >= 0) positive += value;
        else negative += value;
        if (value === 0) return;
        const x0 = x(row.frame) - barWidth / 2, x1 = x0 + barWidth;
        paths[index].push(`M${number(x0)},${number(y(baseline))}H${number(x1)}V${number(y(baseline + value))}H${number(x0)}Z`);
      });
    }
  } else group.items.forEach((_category, index) => {
    let connected = false, previous = null;
    for (const row of group.frames) {
      const value = row.values[index];
      if (!finite(value)) { connected = false; previous = null; continue; }
      if (previous != null && layout.coordinates[row.frame - start] - layout.coordinates[previous - start] > layout.cadence * 1.01) connected = false;
      // A real, short horizontal glyph paints isolated observations even when
      // the dashed path would not paint a zero-length moveto (including zero).
      paths[index].push(connected ? `L${number(x(row.frame))},${number(y(value))}` : `M${number(x(row.frame) - .5)},${number(y(value))}h1L${number(x(row.frame))},${number(y(value))}`);
      connected = true;
      previous = row.frame;
    }
  });
  const marks = group.items.map((category, index) => {
    let low = Infinity, high = -Infinity, observed = 0;
    for (const row of group.frames) if (finite(row.values[index])) { low = Math.min(low, row.values[index]); high = Math.max(high, row.values[index]); observed++; }
    const range = observed ? ` data-heatflow-observed-min="${low}" data-heatflow-observed-max="${high}"` : "";
    const className = group.kind === "diagnostic" ? `heatflow-chart-diagnostic is-${category.id}` : "heatflow-chart-stack";
    return `<path class="${escapeHTML(className)}" data-heatflow-${group.kind === "diagnostic" ? "diagnostic" : "category"}="${escapeHTML(category.id)}" data-heatflow-observed-count="${observed}"${range} d="${paths[index].join("")}" style="--heatflow-series-color:${escapeHTML(color(category))}"><title>${escapeHTML(heatFlowCategoryLabel(category))}</title></path>`;
  }).join("");
  const ticks = [-extent, -extent / 2, 0, extent / 2, extent].map((value) => `<line class="heatflow-chart-grid${value === 0 ? " is-zero" : ""}" x1="${left}" x2="${width - right}" y1="${y(value)}" y2="${y(value)}"/><text class="heatflow-chart-tick" data-heatflow-chart-tick="y" x="${left - 12}" y="${y(value)}" dy=".35em" text-anchor="end">${escapeHTML(tickNumber(value / divisor))}</text>`).join("");
  const cursorX = selected == null ? left : x(selected);
  const aria = `${copy("heatFlowStackAria", "{name} heat-flow stack", { name: zoneSeries.name || "" })} · ${group.title}`;
  const legend = `<div class="heatflow-chart-legend is-${group.kind}">${group.items.map((category) => {
    const value = selected == null ? NaN : heatFlowCategoryValue(zoneSeries, category.index, selected);
    return `<span data-heatflow-chart-legend="${escapeHTML(category.id)}" title="${escapeHTML(`${heatFlowCategoryLabel(category)}: ${heatFlowExactWatts(value)}`)}"><i style="--heatflow-series-color:${escapeHTML(color(category))}"></i><span>${escapeHTML(heatFlowCategoryLabel(category))}</span><strong class="heatflow-chart-legend-value" data-heatflow-legend-value="${finite(value) ? value : ""}">${escapeHTML(formatHeatFlowWatts(value))}</strong></span>`;
  }).join("")}</div>`;
  return `<div class="heatflow-chart-panel" data-heatflow-chart-panel="${group.kind}"><h5>${escapeHTML(group.title)}</h5>
    <div class="heatflow-chart-scroll"><svg class="heatflow-stack-chart" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(aria)}" data-heatflow-chart-start="${start}" data-heatflow-chart-end="${end}" data-heatflow-chart-extent="${extent}" data-heatflow-chart-time-mode="${layout.elapsed ? "elapsed" : "ordinal"}"><title>${escapeHTML(aria)}</title>
      ${ticks}<text class="heatflow-chart-axis-label" x="${left}" y="18">${escapeHTML(copy("heatFlowChartAxis", "Heat flow ({unit})", { unit }))}</text>
      ${marks}<line class="heatflow-chart-axis" x1="${left}" x2="${left}" y1="${top}" y2="${height - bottom}"/>
      <line class="heatflow-cursor" data-heatflow-chart-frame="${selected ?? ""}" x1="${number(cursorX)}" x2="${number(cursorX)}" y1="${top}" y2="${height - bottom}"${selected == null ? ' style="display:none"' : ""}/>
      <rect class="heatflow-chart-hit" x="${left}" y="${top}" width="${plotWidth}" height="${plotHeight}" data-heatflow-chart="1"></rect>
      <text class="heatflow-chart-tick" data-heatflow-chart-tick="x" x="${number(x(start))}" y="${height - bottom + 28}" text-anchor="${start === end ? "middle" : "start"}">${escapeHTML(layout.elapsed ? label(start) : `#${start + 1}`)}</text>
      ${end !== start ? `<text class="heatflow-chart-tick" data-heatflow-chart-tick="x" x="${number(x(end))}" y="${height - bottom + 28}" text-anchor="end">${escapeHTML(layout.elapsed ? label(end) : `#${end + 1}`)}</text>` : ""}
      <text class="heatflow-chart-axis-label" x="${left + plotWidth / 2}" y="${height - 8}" text-anchor="middle">${escapeHTML(layout.elapsed ? copy("hvacChartDateTime", "Date & time") : copy("heatFlowRecordedFrameSequence", "Recorded frame sequence"))}</text>
    </svg></div>${legend}</div>`;
}

/** Map a pointer position in the plot to the nearest actual recorded frame,
 * using exactly the same elapsed-time or ordinal spacing as its glyphs. */
export function heatFlowChartFrameFromRatio(dataset, range, ratio) {
  const layout = frameLayout(dataset, range);
  if (!layout) return 0;
  ratio = Math.max(0, Math.min(1, Number(ratio) || 0));
  let selected = layout.start, distance = Infinity;
  for (let frame = layout.start; frame <= layout.end; frame++) {
    const candidate = Math.abs(layout.ratio(frame) - ratio);
    if (candidate < distance) { selected = frame; distance = candidate; }
  }
  return selected;
}

function normalizedRange(dataset, { start = 0, end } = {}) {
  const count = Math.max(Number(dataset?.frameCount) || 0, dataset?.labels?.length || 0);
  if (!count) return null;
  start = Math.max(0, Math.min(count - 1, Math.floor(Number(start) || 0)));
  end = Math.max(start, Math.min(count - 1, end == null ? count - 1 : Math.floor(Number(end) || 0)));
  return { start, end };
}

function frameLayout(dataset, range = {}) {
  const normalized = normalizedRange(dataset, range);
  if (!normalized) return null;
  const { start, end } = normalized;
  let layouts = layoutCache.get(dataset);
  if (!layouts) { layouts = new Map(); layoutCache.set(dataset, layouts); }
  const key = `${start}:${end}`;
  if (layouts.has(key)) {
    const cached = layouts.get(key);
    layouts.delete(key);
    layouts.set(key, cached);
    return cached;
  }
  const times = [];
  let elapsed = true, minGap = Infinity;
  for (let frame = start; frame <= end; frame++) {
    const value = heatFlowFrameTime(dataset.labels?.[frame]);
    if (!finite(value) || times.length && value <= times.at(-1)) elapsed = false;
    if (times.length && value > times.at(-1)) minGap = Math.min(minGap, value - times.at(-1));
    times.push(value);
  }
  const coordinates = elapsed ? times : times.map((_time, index) => index);
  const cadence = elapsed ? Math.min(minGap, 3600000) : 1;
  const low = coordinates[0] - cadence / 2;
  const span = coordinates.at(-1) - coordinates[0] + cadence;
  const layout = { start, end, elapsed, coordinates, cadence, span, ratio: frame => (coordinates[frame - start] - low) / span };
  layouts.set(key, layout);
  while (layouts.size > cacheLimit) layouts.delete(layouts.keys().next().value);
  return layout;
}

function niceExtent(value) {
  const power = 10 ** Math.floor(Math.log10(value));
  const normalized = value / power;
  return (normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10) * power;
}

function tickNumber(value) {
  return `${value > 0 ? "+" : ""}${value.toLocaleString(getLanguage(), { maximumFractionDigits: 4 })}`;
}
