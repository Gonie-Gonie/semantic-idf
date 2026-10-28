import { t } from "./i18n.js";
import { escapeHTML as safe } from "./state.js";

const finite = value => typeof value === "number" && Number.isFinite(value);
const copy = (key, fallback) => t(`simulation.hvacChart${key}`, {}, fallback);
const format = value => value.toLocaleString(undefined, { maximumFractionDigits: Number.isInteger(value) ? 0 : 6 });
let labelContext;

function measurement(value, step) {
  const significant = finite(step) && step > 0 && value !== 0
    ? Math.max(3, Math.min(17, Math.floor(Math.log10(Math.abs(value))) - Math.floor(Math.log10(step)) + 3)) : 3;
  return Math.abs(value) >= 1e7 || Math.abs(value) > 0 && Math.abs(value) < 1e-5
    ? value.toExponential(significant - 1) : value.toLocaleString(undefined, { maximumSignificantDigits: significant });
}

function labelWidth(label) {
  labelContext ||= document.createElement("canvas").getContext("2d");
  if (!labelContext) return label.length * 12;
  // The shared minimum chart width bounds physical SVG text to 20 units.
  labelContext.font = `20px ${getComputedStyle(document.documentElement).fontFamily}`;
  return labelContext.measureText(label).width;
}

// Count complete observations, before time-series geometry sampling. A
// duplicate timestamp is ambiguous, just as it is in line/scatter charts.
function observations(points) {
  const values = [];
  let previous = -Infinity, pending;
  for (const point of points) if (point && finite(point.x)) {
    if (point.x < previous) return unorderedObservations(points);
    if (point.x === previous) pending = null;
    else { if (finite(pending)) values.push(pending); previous = point.x; pending = point.value; }
  }
  if (finite(pending)) values.push(pending);
  return values;
}

// Native observations are ordered. Only unordered callers need a full index.
function unorderedObservations(points) {
  const byTime = new Map();
  for (const point of points) if (point && finite(point.x)) byTime.set(point.x, byTime.has(point.x) ? null : point.value);
  const values = [];
  for (const value of byTime.values()) if (finite(value)) values.push(value);
  return values;
}

function niceStep(target) {
  const magnitude = 10 ** Math.floor(Math.log10(target));
  if (!finite(magnitude) || magnitude <= 0) return target;
  return [1, 2, 2.5, 5, 10].find(value => value * magnitude >= target) * magnitude;
}

function binRange(group) {
  const { low, high } = group;
  if (low === high) {
    const padding = Math.abs(low) * .05 || .5;
    const lower = finite(low - padding) ? low - padding : low;
    const upper = finite(high + padding) ? high + padding : high;
    return { low: lower, high: upper, count: 1 };
  }
  const wanted = Math.max(1, Math.min(40, Math.ceil(Math.sqrt(group.largest))));
  const span = high - low;
  let step = niceStep(span / wanted);
  for (let attempt = 0; attempt < 8 && finite(span) && finite(step) && step > 0; attempt++) {
    const lower = Math.min(low, Math.floor(low / step) * step), upper = Math.max(high, Math.ceil(high / step) * step), count = Math.round((upper - lower) / step);
    if (!finite(lower) || !finite(upper) || upper <= lower || !finite(count)) break;
    if (count <= 40) return { low: lower, high: upper, count: Math.max(1, count) };
    step = niceStep(step * 1.01);
  }
  // Keep extreme finite inputs measurable without overflowing a span.
  return { low, high, count: wanted };
}

const edge = (range, index) => {
  if (index === 0) return range.low;
  if (index === range.count) return range.high;
  const ratio = index / range.count;
  return range.low * (1 - ratio) + range.high * ratio;
};

function binIndex(value, edges) {
  let low = 0, high = edges.length - 1;
  while (high - low > 1) {
    const middle = Math.floor((low + high) / 2);
    if (value < edges[middle]) high = middle; else low = middle;
  }
  return low;
}

export function renderHVACInspectionHistogram({ traces, title, heading, empty, legend }) {
  const groups = new Map();
  const series = traces.map(trace => {
    const values = observations(trace.points), item = { ...trace, values };
    if (values.length) {
      const group = groups.get(trace.unit) || { unit: trace.unit, low: Infinity, high: -Infinity, largest: 0, traces: [] };
      for (const value of values) { group.low = Math.min(group.low, value); group.high = Math.max(group.high, value); }
      group.largest = Math.max(group.largest, values.length); group.traces.push(item); groups.set(trace.unit, group);
    }
    return item;
  });
  if (!groups.size) return empty(copy("Unavailable", "No observations are available for the selected properties."));
  let maximum = 0;
  for (const group of groups.values()) {
    group.range = binRange(group);
    group.edges = Array.from({ length: group.range.count + 1 }, (_, index) => edge(group.range, index));
    if (group.edges.some((value, index) => index > 0 && value <= group.edges[index - 1])) {
      // Adjacent binary64 values cannot always support the requested bins.
      group.range.count = 1; group.edges = [group.range.low, group.range.high];
    }
    for (const trace of group.traces) {
      trace.bins = Array(group.range.count).fill(0);
      for (const value of trace.values) trace.bins[binIndex(value, group.edges)]++;
      for (const count of trace.bins) maximum = Math.max(maximum, count);
    }
  }
  const width = 1000, left = 112, right = 42, top = 44, plotBottom = 259, axisGap = 70;
  const height = 340 + (groups.size - 1) * axisGap, plotWidth = width - left - right;
  const countStep = Math.max(1, Math.ceil(niceStep(maximum / 4))), countHigh = Math.ceil(maximum / countStep) * countStep;
  const y = count => plotBottom - count / countHigh * (plotBottom - top);
  const yTicks = [];
  for (let count = 0; count <= countHigh + countStep * .001; count += countStep) yTicks.push(count);
  const yAxis = yTicks.map(count => `<line class="hvac-chart-grid" x1="${left}" x2="${width - right}" y1="${y(count)}" y2="${y(count)}"/><line class="hvac-chart-axis" x1="${left - 5}" x2="${left}" y1="${y(count)}" y2="${y(count)}"/><text class="hvac-chart-tick" data-hvac-chart-tick="left" x="${left - 10}" y="${y(count) + 5}" text-anchor="end">${safe(format(count))}</text>`).join("")
    + `<text class="hvac-chart-axis-label" data-hvac-chart-axis="left" data-hvac-chart-y-low="0" data-hvac-chart-y-high="${countHigh}" x="${left}" y="22">${safe(copy("HistogramCount", "Observations"))}</text>`;
  const axes = [...groups.values()].map((group, index) => {
    const baseline = plotBottom + index * axisGap, range = group.range;
    const step = edge({ ...range, count: 4 }, 1) - range.low;
    const candidates = Array.from({ length: 5 }, (_, tick) => {
      const value = edge({ ...range, count: 4 }, tick);
      return { value, label: measurement(value, step), position: left + plotWidth * tick / 4 };
    });
    const ticks = [candidates[0]], last = candidates.at(-1), lastLeft = last.position - labelWidth(last.label);
    const seen = new Set([candidates[0].value, last.value]);
    let previousRight = left + labelWidth(candidates[0].label);
    for (const tick of candidates.slice(1, -1)) {
      if (seen.has(tick.value)) continue;
      seen.add(tick.value);
      const half = labelWidth(tick.label) / 2;
      if (tick.position - half >= previousRight + 12 && tick.position + half <= lastLeft - 12) {
        ticks.push(tick); previousRight = tick.position + half;
      }
    }
    ticks.push(last);
    const labels = ticks.map((tick, tickIndex) => `${index === 0 ? `<line class="hvac-chart-grid" x1="${tick.position}" x2="${tick.position}" y1="${top}" y2="${plotBottom}"/>` : ""}<line class="hvac-chart-axis" x1="${tick.position}" x2="${tick.position}" y1="${baseline}" y2="${baseline + 5}"/><text class="hvac-chart-tick" data-hvac-chart-tick="x" x="${tick.position}" y="${baseline + 25}" text-anchor="${tickIndex === 0 ? "start" : tickIndex === ticks.length - 1 ? "end" : "middle"}">${safe(tick.label)}</text>`).join("");
    const label = group.unit;
    return `<g data-hvac-chart-histogram-axis="${safe(group.unit)}" data-hvac-chart-x-low="${range.low}" data-hvac-chart-x-high="${range.high}"><line class="hvac-chart-axis" x1="${left}" x2="${width - right}" y1="${baseline}" y2="${baseline}"/>${labels}<text class="hvac-chart-axis-label" data-hvac-chart-axis="x" data-hvac-chart-unit="${safe(group.unit)}" x="${left + plotWidth / 2}" y="${baseline + 51}" text-anchor="middle">${safe(label)}</text></g>`;
  }).join("");
  const marks = series.map(trace => {
    const group = groups.get(trace.unit), range = group?.range, bins = trace.bins || [];
    let outline = "";
    const bars = bins.map((count, index) => {
      const binLow = group.edges[index], binHigh = group.edges[index + 1], x = left + plotWidth * index / range.count, binWidth = plotWidth / range.count;
      outline += index === 0 ? `M${x},${plotBottom} L${x},${y(count)}` : ` L${x},${y(count)}`;
      outline += ` L${x + binWidth},${y(count)}`;
      const interval = `${measurement(binLow, binHigh - binLow)} ≤ x ${index === bins.length - 1 ? "≤" : "<"} ${measurement(binHigh, binHigh - binLow)} ${trace.unit}`;
      const tooltip = `${trace.label} · ${interval} · ${copy("HistogramCount", "Observations")}: ${format(count)} / ${format(trace.values.length)} · ${copy("HistogramShare", "Share")}: ${format(count / trace.values.length * 100)}%`;
      return `<rect class="hvac-chart-histogram-bin" data-hvac-chart-bin-low="${binLow}" data-hvac-chart-bin-high="${binHigh}" data-hvac-chart-bin-count="${count}" x="${x}" y="${y(count)}" width="${binWidth}" height="${plotBottom - y(count)}"><title>${safe(tooltip)}</title></rect>`;
    }).join("");
    if (bins.length) outline += ` L${width - right},${plotBottom}`;
    return `<g data-hvac-chart-series="${safe(trace.id)}" data-hvac-chart-unit="${safe(trace.unit)}" data-hvac-chart-observations="${trace.values.length}" style="--hvac-chart-color:${trace.color}">${bars}${outline ? `<path class="hvac-chart-line hvac-chart-histogram-outline" d="${outline}"/>` : ""}</g>`;
  }).join("");
  const label = title || copy("Histogram", "Histogram");
  const legends = series.map(trace => ({ ...trace, legendLabel: `${trace.legendLabel || `${trace.label} (${trace.unit})`} · n=${format(trace.values.length)}` }));
  return `<section class="hvac-inspection-chart" data-hvac-inspection-chart="histogram">${heading}<div class="hvac-chart-scroll"><svg class="hvac-chart-svg" viewBox="0 0 ${width} ${height}" role="img" aria-label="${safe(label)}"><title>${safe(label)}</title>${yAxis}${axes}${marks}</svg></div>${legend(legends)}</section>`;
}
