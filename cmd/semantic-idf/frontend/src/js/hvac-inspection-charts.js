import { t } from "./i18n.js";
import { escapeHTML } from "./state.js";
import { renderHVACInspectionHistogram } from "./hvac-inspection-histogram.js";
import { observedTimeTicks, registerTimeSeriesChart, timeSeriesWindowPoints } from "./time-series-chart.js";

const finite = (value) => typeof value === "number" && Number.isFinite(value);
const copy = (key, fallback) => t(`simulation.hvacChart${key}`, {}, fallback);
const colors = ["var(--teal)", "var(--amber)", "var(--hvac-chart-blue)", "var(--hvac-chart-purple)", "var(--green)", "var(--red)", "var(--hvac-chart-gray)", "var(--hvac-chart-cyan)"];
const unitLabel = (unit) => String(unit || "–");
let tickLabelContext = null;
const legendInteractions = new WeakMap();

/** Render observed HVAC time series. x and frameKey share the same numeric time
 * coordinate; interval optionally declares the expected sampling cadence.
 * Missing values are gaps. Scatter joins exact, unambiguous time AND key pairs
 * (or labels when keys are omitted); keys can distinguish output files/runs.
 * Rendering samples geometry only: the caller's complete observations are intact. */
export function renderHVACInspectionChart(options = {}) {
  const { series = [], mode = "line" } = options;
  const traces = series.filter(Boolean).map((trace, index) => ({ ...trace,
    id: String(trace.id ?? index), label: String(trace.label || trace.id || ""),
    unit: unitLabel(trace.unit), color: colors[(Number.isInteger(trace.colorIndex) && trace.colorIndex >= 0 ? trace.colorIndex : index) % colors.length],
    points: mode === "histogram" ? trace.points || [] : normalizedPoints(trace.points || [], trace.interval),
  }));
  return renderPreparedChart(traces, options);
}

function renderPreparedChart(traces, options) {
  const { mode = "line", frameKey, title = "", xLabel, yLimits, legendLayout = "inline", markers = true, xRange, interactive = true } = options;
  const heading = title ? `<h4>${escapeHTML(title)}</h4>` : "";
  const empty = (message) => `<section class="hvac-inspection-chart" data-hvac-inspection-chart="${mode === "histogram" ? "histogram" : mode === "scatter" ? "scatter" : "line"}">${heading}<div class="hvac-inspection-chart-empty" data-hvac-chart-empty role="status">${escapeHTML(message)}</div></section>`;
  if (mode === "histogram") return renderHVACInspectionHistogram({ traces, title, heading, empty, legend: (items) => chartLegend(items, legendLayout) });
  if (mode === "scatter" && traces.length !== 2) return empty(copy("ScatterTwo", "Select exactly two properties for a scatter plot."));
  const units = [...new Set(traces.map((trace) => trace.unit))];
  if (mode !== "scatter" && units.length > 2) return empty(copy("TwoUnits", "A line graph supports up to two different units."));
  if (!traces.length || !traces.some((trace) => trace.points.some((point) => finite(point.value)))) return empty(copy("Unavailable", "No observations are available for the selected properties."));
  const scatter = mode === "scatter";
  const pairs = scatter ? alignedPairs(traces[0], traces[1]) : [];
  if (scatter && !pairs.length) return empty(copy("AlignedUnavailable", "No matching time observations are available for these two properties."));
  const width = 1000, height = 340, left = 112, right = scatter || units.length === 1 ? 42 : 112, top = 44, bottom = 81;
  const plotWidth = width - left - right, plotHeight = height - top - bottom;
  const allPoints = scatter ? pairs : traces.flatMap((trace) => trace.points);
  const fullXDomain = domain(allPoints.map((point) => point.x));
  const xDomain = !scatter && xRange ? { low: xRange[0], high: xRange[1] } : fullXDomain;
  const yDomains = scatter ? [domain(pairs.map((point) => point.y), true)] : units.map((unit) => {
    const limit = yLimits?.[unit];
    return finite(limit?.low) && finite(limit?.high) && limit.low < limit.high ? domain([limit.low, limit.high])
      : domain(traces.filter((trace) => trace.unit === unit).flatMap((trace) => trace.points.map((point) => point.value)), true);
  });
  const x = (value) => left + (value - xDomain.low) / (xDomain.high - xDomain.low) * plotWidth;
  const y = (value, axis = 0) => top + (yDomains[axis].high - value) / (yDomains[axis].high - yDomains[axis].low) * plotHeight;
  const axisUnit = (index) => scatter ? traces[1].unit : units[index];
  const axisTitle = (index) => {
    if (scatter) return withUnit(traces[1].label, traces[1].unit);
    const labels = [...new Set(traces.filter((trace) => trace.unit === units[index]).map((trace) => trace.axisLabel || copy("Value", "Value")))];
    return withUnit(labels.length === 1 ? labels[0] : copy("Value", "Value"), units[index]);
  };
  const yAxes = yDomains.map((range, axis) => range.ticks.map((value) => {
    const position = y(value, axis), rightSide = axis === 1;
    return `${axis === 0 ? `<line class="hvac-chart-grid" x1="${left}" x2="${width - right}" y1="${position}" y2="${position}"/>` : ""}<line class="hvac-chart-axis" x1="${rightSide ? width - right : left}" x2="${rightSide ? width - right + 5 : left - 5}" y1="${position}" y2="${position}"/><text class="hvac-chart-tick" data-hvac-chart-tick="${rightSide ? "right" : "left"}" x="${rightSide ? width - right + 10 : left - 10}" y="${position + 5}" text-anchor="${rightSide ? "start" : "end"}">${escapeHTML(number(value, range.step))}</text>`;
  }).join("") + `<text class="hvac-chart-axis-label" data-hvac-chart-axis="${axis === 1 ? "right" : "left"}" data-hvac-chart-unit="${escapeHTML(axisUnit(axis))}" data-hvac-chart-y-low="${range.low}" data-hvac-chart-y-high="${range.high}" x="${axis === 1 ? width - right : left}" y="30" text-anchor="${axis === 1 ? "end" : "start"}">${escapeHTML(axisTitle(axis))}</text>`).join("");
  const xTicks = scatter ? xDomain.ticks.map((value) => ({ x: value, label: number(value, xDomain.step) }))
    : observedTimeTicks(allPoints, { x, plotLeft: left, plotWidth, measure: timeTickLabelWidth });
  const xGrid = xTicks.map((point, index) => {
    const anchor = scatter ? xTicks.length < 2 ? "middle" : index === 0 ? "start" : index === xTicks.length - 1 ? "end" : "middle" : point.anchor;
    return `<line class="hvac-chart-grid" x1="${x(point.x)}" x2="${x(point.x)}" y1="${top}" y2="${height - bottom}"/><text class="hvac-chart-tick" data-hvac-chart-tick="x" x="${point.labelX ?? x(point.x)}" y="${height - bottom + 25}" text-anchor="${anchor}">${escapeHTML(point.label)}</text>`;
  }).join("");
  const marks = scatter ? scatterMarks(pairs, traces, x, y, frameKey) : traces.map((trace) => lineMarks({ ...trace, points: timeSeriesWindowPoints(trace.points, xRange) }, units.indexOf(trace.unit), x, y, frameKey, Math.max(64, Math.floor(2400 / traces.length)), markers)).join("");
  // A nested SVG clips geometry to the plot without changing observations or
  // introducing duplicate clip-path IDs across independently rendered graphs.
  const clippedMarks = !scatter ? `<svg data-hvac-chart-clipped-marks x="${left}" y="${top}" width="${plotWidth}" height="${plotHeight}" viewBox="${left} ${top} ${plotWidth} ${plotHeight}" overflow="hidden">${marks}</svg>` : marks;
  const frameVisible = finite(frameKey) && frameKey >= xDomain.low && frameKey <= xDomain.high, frameX = frameVisible ? x(frameKey) : left;
  const frame = !scatter ? `<g data-hvac-chart-frame-marker${frameVisible ? "" : ' style="display:none"'}><line class="hvac-chart-frame" data-hvac-chart-frame="${frameVisible ? frameKey : ""}" x1="${frameX}" x2="${frameX}" y1="${top}" y2="${height - bottom}"/><text class="hvac-chart-frame-label" x="${frameX}" y="${height - bottom - 8}" text-anchor="${frameX > width / 2 ? "end" : "start"}">${escapeHTML(copy("Frame", "Frame"))}</text></g>` : "";
  const xTitle = scatter ? withUnit(traces[0].label, traces[0].unit) : xLabel || copy("DateTime", "Date & time");
  const label = title || copy(scatter ? "Scatter" : "Line", scatter ? "Scatter plot" : "Time series");
  const legend = scatter ? "" : chartLegend(traces, legendLayout);
  const cadence = Math.min(...traces.map((trace) => {
    if (finite(trace.interval) && trace.interval > 0) return trace.interval;
    let interval = Infinity;
    for (let index = 1; index < trace.points.length; index++) interval = Math.min(interval, trace.points[index].x - trace.points[index - 1].x);
    return interval;
  }));
  const interaction = !scatter && interactive ? registerTimeSeriesChart({ extent: [fullXDomain.low, fullXDomain.high], range: [xDomain.low, xDomain.high],
    minSpan: finite(cadence) ? cadence : 0, plotLeft: left, plotWidth,
    render: (range, currentFrame) => renderPreparedChart(traces, { ...options, xRange: range, frameKey: currentFrame, interactive: false }) }) : "";
  return `<section class="hvac-inspection-chart" data-hvac-inspection-chart="${scatter ? "scatter" : "line"}">${heading}<div class="hvac-chart-scroll"><svg class="hvac-chart-svg" viewBox="0 0 ${width} ${height}"${interaction}${!scatter ? ` data-time-series-frame="${finite(frameKey) ? frameKey : ""}"` : ""} data-hvac-chart-domain-low="${xDomain.low}" data-hvac-chart-domain-high="${xDomain.high}" data-hvac-chart-plot-left="${left}" data-hvac-chart-plot-width="${plotWidth}" role="img" aria-label="${escapeHTML(label)}"><title>${escapeHTML(label)}</title>${yAxes}${xGrid}<line class="hvac-chart-axis" x1="${left}" x2="${width - right}" y1="${height - bottom}" y2="${height - bottom}"/>${clippedMarks}${frame}<text class="hvac-chart-axis-label" data-hvac-chart-axis="x"${scatter ? ` data-hvac-chart-unit="${escapeHTML(traces[0].unit)}"` : ""} x="${left + plotWidth / 2}" y="${height - 16}" text-anchor="middle">${escapeHTML(xTitle)}</text></svg></div>${legend}</section>`;
}

// Fixed per-unit slider bounds come from the loop's actual observations. They
// have room outside the automatic extent so either handle can narrow or expand.
export function hvacInspectionChartYRanges(series = []) {
  const ranges = new Map();
  for (const trace of series) {
    const unit = unitLabel(trace.unit), range = ranges.get(unit) || { low: Infinity, high: -Infinity };
    for (const point of trace.points || []) if (finite(point?.x) && finite(point.value)) { range.low = Math.min(range.low, point.value); range.high = Math.max(range.high, point.value); }
    if (finite(range.low)) ranges.set(unit, range);
  }
  return [...ranges].map(([unit, range]) => {
    const auto = domain([range.low, range.high], true), span = auto.high - auto.low;
    const step = Number((span / 500).toPrecision(2));
    const rounded = (value) => Number(value.toPrecision(12));
    return { unit, step, min: rounded(Math.floor((auto.low - span * .5) / step) * step), max: rounded(Math.ceil((auto.high + span * .5) / step) * step),
      auto: { low: rounded(Math.floor(auto.low / step) * step), high: rounded(Math.ceil(auto.high / step) * step) } };
  });
}

// Move only the snapshot indicator during playback. A sampled-out observation
// remains absent rather than being interpolated into an invented point.
export function updateHVACInspectionChartFrame(container, frameKey) {
  if (!container?.querySelectorAll) return;
  const charts = container.matches?.("[data-hvac-inspection-chart]") ? [container] : [...container.querySelectorAll("[data-hvac-inspection-chart]")];
  for (const chart of charts) {
    const svg = chart.querySelector("svg");
    if (!svg) continue;
    if (chart.dataset.hvacInspectionChart === "line") svg.dataset.timeSeriesFrame = finite(frameKey) ? frameKey : "";
    for (const point of svg.querySelectorAll("circle.is-frame")) { point.classList.remove("is-frame"); point.setAttribute("r", point.dataset.hvacChartRadius); }
    if (finite(frameKey)) for (const point of svg.querySelectorAll(`circle[data-hvac-chart-time="${frameKey}"]`)) { point.classList.add("is-frame"); point.setAttribute("r", chart.dataset.hvacInspectionChart === "scatter" ? "6" : "5"); }
    const marker = svg.querySelector("[data-hvac-chart-frame-marker]");
    if (!marker) continue;
    const low = Number(svg.dataset.hvacChartDomainLow), high = Number(svg.dataset.hvacChartDomainHigh);
    const visible = finite(frameKey) && frameKey >= low && frameKey <= high;
    marker.style.display = visible ? "" : "none";
    const line = marker.querySelector("line");
    line.dataset.hvacChartFrame = visible ? String(frameKey) : "";
    if (!visible) continue;
    const position = Number(svg.dataset.hvacChartPlotLeft) + (frameKey - low) / (high - low) * Number(svg.dataset.hvacChartPlotWidth);
    line.setAttribute("x1", position); line.setAttribute("x2", position);
    const label = marker.querySelector("text"); label.setAttribute("x", position); label.setAttribute("text-anchor", position > 500 ? "end" : "start");
  }
}

// Delegate from the stable results host so replaced charts need no listeners.
// Hover and keyboard focus retain separate selections; leaving a hovered
// legend restores the focused trace, or all traces when neither remains.
export function handleHVACChartLegendEvent(event, container) {
  if (!["pointerover", "pointerout", "focusin", "focusout"].includes(event.type)) return false;
  const legend = event.target?.closest?.("[data-hvac-chart-legend]");
  const chart = legend?.closest("[data-hvac-inspection-chart]");
  if (!chart || !container?.contains(chart)) return false;
  let interaction = legendInteractions.get(chart);
  if (!interaction) { interaction = { hover: null, focus: null }; legendInteractions.set(chart, interaction); }
  const field = event.type.startsWith("pointer") ? "hover" : "focus";
  if (event.type === "pointerout" || event.type === "focusout") {
    const next = event.relatedTarget?.closest?.("[data-hvac-chart-legend]");
    interaction[field] = next?.closest("[data-hvac-inspection-chart]") === chart ? next : null;
  } else interaction[field] = legend;
  if (!chart.contains(interaction.hover)) interaction.hover = null;
  if (!chart.contains(interaction.focus)) interaction.focus = null;
  const active = interaction.hover || interaction.focus;
  const id = active?.dataset.hvacChartLegend;
  if (active) chart.dataset.hvacChartHighlight = id;
  else delete chart.dataset.hvacChartHighlight;
  for (const series of chart.querySelectorAll("[data-hvac-chart-series]")) {
    const highlighted = Boolean(active) && series.dataset.hvacChartSeries === id;
    series.classList.toggle("is-highlighted", highlighted);
    series.classList.toggle("is-muted", Boolean(active) && !highlighted);
  }
  for (const item of chart.querySelectorAll("[data-hvac-chart-legend]")) {
    item.classList.toggle("is-highlighted", item === active);
    item.classList.toggle("is-muted", Boolean(active) && item !== active);
  }
  return true;
}

function withUnit(label, unit) { return `${label} (${unit})`; }

function chartLegend(traces, layout) {
  return `<div class="hvac-chart-legend${layout === "vertical" ? " is-vertical" : ""}">${traces.map((trace) => `<span data-hvac-chart-legend="${escapeHTML(trace.id)}" tabindex="0"><i style="--hvac-chart-color:${trace.color}"></i><span title="${escapeHTML(trace.legendLabel ?? withUnit(trace.label, trace.unit))}">${escapeHTML(trace.legendLabel ?? withUnit(trace.label, trace.unit))}</span></span>`).join("")}</div>`;
}

function normalizedPoints(points, interval) {
  const sorted = points.filter((point) => point && finite(point.x)).map((point) => ({ x: point.x, label: String(point.label ?? point.x), key: point.key == null ? null : String(point.key), value: finite(point.value) ? point.value : null })).sort((a, b) => a.x - b.x);
  const unique = [];
  for (const point of sorted) {
    if (unique.at(-1)?.x === point.x) unique[unique.length - 1].value = null;
    else unique.push(point);
  }
  let cadence = finite(interval) && interval > 0 ? interval : Infinity;
  if (cadence === Infinity) for (let index = 1; index < unique.length; index++) cadence = Math.min(cadence, unique[index].x - unique[index - 1].x);
  let segment = 0, previous = null;
  return unique.map((point) => {
    if (!finite(point.value) || previous !== null && point.x - previous > cadence * 1.01) segment++;
    previous = point.x;
    return { ...point, segment };
  });
}

function alignedPairs(left, right) {
  const byTime = new Map(right.points.map((point) => [point.x, point]));
  return left.points.flatMap((point) => {
    const matching = byTime.get(point.x);
    const aligned = matching && (point.key !== null || matching.key !== null ? point.key !== null && point.key === matching.key : point.label === matching.label);
    return finite(point.value) && aligned && finite(matching.value) ? [{ x: point.value, y: matching.value, time: point.x, label: point.label }] : [];
  });
}

function domain(values, padded = false) {
  let low = Infinity, high = -Infinity;
  for (const value of values) if (finite(value)) { low = Math.min(low, value); high = Math.max(high, value); }
  if (!finite(low)) { low = 0; high = 1; }
  if (low === high) { const padding = Math.abs(low) * .05 || 1; low -= padding; high += padding; }
  else if (padded) { const padding = (high - low) * .06; low -= padding; high += padding; }
  const target = (high - low) / 4, magnitude = 10 ** Math.floor(Math.log10(target));
  const step = [1, 2, 2.5, 5, 10].find((value) => value * magnitude >= target) * magnitude;
  const ticks = [];
  for (let value = Math.ceil(low / step) * step; value <= high + step * .0001 && ticks.length < 8; value += step) ticks.push(Math.abs(value) < step * .000001 ? 0 : value);
  return { low, high, step, ticks };
}

function number(value, step) {
  if (!finite(value)) return "–";
  if (Math.abs(value) >= 1e7 || Math.abs(value) > 0 && Math.abs(value) < 1e-5) return value.toExponential(2);
  const decimals = step ? Math.max(0, Math.min(6, Math.ceil(-Math.log10(step)) + (step / 10 ** Math.floor(Math.log10(step)) === 2.5 ? 1 : 0))) : Math.abs(value) < .01 && value !== 0 ? 5 : 2;
  return value.toLocaleString(undefined, { minimumFractionDigits: step ? 0 : Math.min(2, decimals), maximumFractionDigits: decimals });
}

function timeTickLabelWidth(label) {
  // The common minimum CSS width bounds scaled tick fonts to 21 SVG units.
  // Measure the inherited UI family at that bound so every supported panel
  // width fits; this detached canvas never draws or changes the plot DOM.
  tickLabelContext ||= document.createElement("canvas").getContext("2d");
  if (!tickLabelContext) return String(label).length * 12;
  tickLabelContext.font = `21px ${getComputedStyle(document.documentElement).fontFamily}`;
  return tickLabelContext.measureText(String(label)).width;
}

// Preserve each bucket's actual extrema and endpoints. Segment IDs prevent a
// sampled path from crossing omitted missing observations or absent intervals.
function sample(points, limit, fields, frameKey, timeField = "x") {
  if (points.length <= limit) return points;
  const selected = new Set([0, points.length - 1]);
  const bucketSize = Math.ceil(points.length / Math.max(1, Math.floor(limit / (2 + fields.length * 2))));
  for (let start = 0; start < points.length; start += bucketSize) {
    const end = Math.min(points.length, start + bucketSize);
    selected.add(start); selected.add(end - 1);
    for (const field of fields) {
      let min = start, max = start;
      for (let index = start + 1; index < end; index++) {
        if (points[index][field] < points[min][field]) min = index;
        if (points[index][field] > points[max][field]) max = index;
      }
      selected.add(min); selected.add(max);
    }
  }
  if (finite(frameKey)) points.forEach((point, index) => { if (point[timeField] === frameKey) selected.add(index); });
  return [...selected].sort((a, b) => a - b).map((index) => points[index]);
}

function lineMarks(trace, axis, x, y, frameKey, limit, markers) {
  const drawn = sample(trace.points.filter((point) => finite(point.value)), limit, ["value"], frameKey);
  let previous = null;
  const path = drawn.map((point) => { const command = previous === point.segment ? "L" : "M"; previous = point.segment; return `${command}${x(point.x)},${y(point.value, axis)}`; }).join(" ");
  return `<g data-hvac-chart-series="${escapeHTML(trace.id)}" data-hvac-chart-series-axis="${axis === 1 ? "right" : "left"}" style="--hvac-chart-color:${trace.color}"><path class="hvac-chart-line" d="${path}"/>${markers ? drawn.map((point) => `<circle class="hvac-chart-point${point.x === frameKey ? " is-frame" : ""}" data-hvac-chart-radius="${drawn.length > 250 ? 1.6 : 2.7}" data-hvac-chart-value="${point.value}" data-hvac-chart-time="${point.x}" cx="${x(point.x)}" cy="${y(point.value, axis)}" r="${point.x === frameKey ? 5 : drawn.length > 250 ? 1.6 : 2.7}"><title>${escapeHTML(`${point.label} · ${trace.label}: ${number(point.value)} ${trace.unit}`)}</title></circle>`).join("") : ""}</g>`;
}

function scatterMarks(pairs, traces, x, y, frameKey) {
  return `<g data-hvac-chart-scatter style="--hvac-chart-color:${traces[0].color}">${sample(pairs, 1600, ["x", "y"], frameKey, "time").map((point) => `<circle class="hvac-chart-point hvac-chart-scatter-point${point.time === frameKey ? " is-frame" : ""}" data-hvac-chart-radius="3.3" data-hvac-chart-time="${point.time}" data-hvac-chart-x-value="${point.x}" data-hvac-chart-y-value="${point.y}" cx="${x(point.x)}" cy="${y(point.y)}" r="${point.time === frameKey ? 6 : 3.3}"><title>${escapeHTML(`${point.label} · ${traces[0].label}: ${number(point.x)} ${traces[0].unit} · ${traces[1].label}: ${number(point.y)} ${traces[1].unit}`)}</title></circle>`).join("")}</g>`;
}
