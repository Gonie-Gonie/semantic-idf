import { t } from "./i18n.js";
import { escapeHTML } from "./state.js";
import { energyPathDisplayUnit, formatEnergyPathDisplayValue } from "./energy-path-display.js";

const finite = (value) => typeof value === "number" && Number.isFinite(value);
const copy = (key, fallback, values = {}) => t(`simulation.energyPathChart${key}`, values, fallback);
const members = (item) => item.groupedMembers?.length ? item.groupedMembers : [item];
const token = (value) => String(value || "").trim().toLowerCase();

// Keep the selected group's original members fixed while monthly grouping changes.
// Missing observations remain gaps; an annual total never supplies monthly values.
export function energyPathMonthlyChartSeries(item = {}, kind = "node", graphs = []) {
  const wanted = new Set(members(item).map((member) => member.id));
  const fields = kind === "link" ? ["fromValue", "toValue"] : ["value"];
  return fields.map((field) => ({
    id: field,
    label: kind === "link" ? field === "fromValue" ? copy("From", "From") : copy("To", "To") : item.label || item.id || "",
    unit: kind === "link" ? item[field === "fromValue" ? "fromUnit" : "toUnit"] || "kWh" : item.unit || "kWh",
    points: Array.from({ length: 12 }, (_, month) => {
      const records = graphs[month]?.[kind === "link" ? "links" : "nodes"] || [];
      const found = new Map();
      for (const record of records) {
        for (const member of members(record)) {
          if (wanted.has(member.id)) found.set(member.id, member[field]);
        }
      }
      const values = [...found.values()];
      return {
        x: month,
        label: t(`simulation.month.${month + 1}`, {}, `M${month + 1}`),
        value: found.size === wanted.size && values.every(finite) ? values.reduce((sum, value) => sum + value, 0) : null,
      };
    }),
  }));
}

export function energyPathHourlyChartSeries(item = {}, sources = [], viewState = {}, hourlyLabels = []) {
  if (!Array.isArray(hourlyLabels) || !hourlyLabels.length) return [];
  const leap = hourlyLabels.some((label) => String(label || "").startsWith("02-29")), seenHours = new Set();
  const selectedMonth = /^M([1-9]|1[0-2])$/i.exec(viewState.simulationEnergyPeriod || "");
  const axis = [];
  for (let index = 0; index < hourlyLabels.length; index++) {
    const label = hourlyLabels[index], calendar = hourlyCalendar(label, leap);
    if (!calendar || seenHours.has(calendar.hour)) return [];
    seenHours.add(calendar.hour);
    if (!selectedMonth || calendar.month === Number(selectedMonth[1])) axis.push({ index, x: calendar.hour, label });
  }
  axis.sort((left, right) => left.x - right.x);
  if (!axis.length) return [];
  const byID = new Map(sources.map((source) => [source.id, source])), visited = new Set(), observed = new Map(), matches = new Map(), missingMetrics = new Set();
  for (const source of sources) {
    if (source.hourlyEnergy?.basis !== "reported_source" || token(source.hourlyEnergy.unit) !== "kwh") continue;
    const identity = sourceIdentity(source);
    if (identity) matches.set(identity, [...(matches.get(identity) || []), source]);
  }
  const visit = (id) => {
    if (visited.has(id)) return;
    visited.add(id);
    const source = byID.get(id);
    if (!source) return;
    const candidates = matches.get(sourceIdentity(source)) || [];
    // Monthly and Hourly dictionary IDs are different observations of one exact
    // output identity. Multiple Hourly dictionaries remain ambiguous.
    const actual = candidates.length === 1 ? candidates[0] : null;
    if (!actual) {
      (source.inputSourceIds || []).forEach(visit);
      if (!source.inputSourceIds?.length && source.name) missingMetrics.add(metricIdentity(source));
      return;
    }
    if (observed.has(actual.id)) return;
    const scoped = (source.scopeDetails || []).filter((detail) => token(detail.scope?.kind) === token(viewState.simulationEnergyScopeKind || "building") &&
      (viewState.simulationEnergyScopeKind !== "zone" || token(detail.scope?.zoneName) === token(viewState.simulationEnergyZoneName)));
    const metadata = scoped.length === 1 ? scoped[0] : source;
    const application = token(metadata.multiplierApplication);
    const provenMultiplier = ["requires_zone_multiplier", "requires_group_multiplier"].includes(application) && finite(metadata.effectiveMultiplier) && metadata.effectiveMultiplier > 0;
    const multiplier = provenMultiplier ? metadata.effectiveMultiplier : 1;
    const nativeBasis = application !== "already_model_total" && !provenMultiplier;
    observed.set(actual.id, {
      id: actual.id,
      label: metricLabel(actual), metric: metricIdentity(actual), nativeBasis,
      hourly: actual.hourlyEnergy, multiplier,
    });
  };
  (item.sourceIds || []).forEach(visit);
  const groups = new Map();
  for (const trace of observed.values()) {
    const values = trace.hourly.values;
    const valid = Array.isArray(values) && values.length === hourlyLabels.length &&
      values.every((value) => finite(value) && finite(value * trace.multiplier));
    // Native observations with unproven multipliers remain separate curves.
    const key = trace.metric + (trace.nativeBasis ? `|native:${trace.id}` : "|model");
    const group = groups.get(key) || { label: trace.label, members: [], valid: !missingMetrics.has(trace.metric) };
    group.valid &&= valid;
    group.members.push({ id: trace.id, values, multiplier: trace.multiplier });
    groups.set(key, group);
  }
  const output = [...groups.values()].filter((group) => group.valid).map((group) => {
    return {
      id: group.members.map((member) => member.id).join("|"), label: group.label, unit: "kWh",
      points: axis.map(({ index, x, label }) => {
        const value = group.members.reduce((sum, member) => sum + member.values[index] * member.multiplier, 0);
        return { x, label, value: finite(value) ? value : null };
      }),
    };
  });
  const labelCounts = new Map(), labels = new Map();
  for (const trace of output) labelCounts.set(trace.label, (labelCounts.get(trace.label) || 0) + 1);
  for (const trace of output) {
    if (labelCounts.get(trace.label) > 1) {
      const next = (labels.get(trace.label) || 0) + 1; labels.set(trace.label, next);
      trace.label += ` ${next}`;
    }
  }
  return output;
}

function metricIdentity(source) {
  return [source.isMeter === true || token(source.sourceType) === "sql_meter" ? "meter" : "variable", token(source.name)].join("|");
}

function metricLabel(source) {
  const name = token(source.name), parts = [];
  if (name.includes("predicted")) parts.push(copy("Predicted", "Predicted"));
  if (name.includes("sensible")) parts.push(copy("Sensible", "Sensible"));
  else if (name.includes("latent")) parts.push(copy("Latent", "Latent"));
  else if (name.includes("total")) parts.push(copy("Total", "Total"));
  if (name.includes("gain")) parts.push(copy("Gain", "Gain"));
  else if (name.includes("loss")) parts.push(copy("Loss", "Loss"));
  else if (name.includes("cooling")) parts.push(copy("Cooling", "Cooling"));
  else if (name.includes("heating")) parts.push(copy("Heating", "Heating"));
  if (parts.length) return parts.join(" ");
  for (const [pattern, key, label] of [
    [/electric/, "Electricity", "Electricity"], [/natural.?gas/, "Gas", "Natural gas"],
    [/infiltration/, "Infiltration", "Infiltration"], [/ventilation|outdoor air/, "OutdoorTransfer", "Outdoor transfer"],
    [/mixing|interzone/, "InterzoneTransfer", "Interzone transfer"], [/convect/, "Convection", "Convection"],
    [/radiat/, "Radiation", "Radiation"], [/storage/, "Storage", "Storage"],
  ]) if (pattern.test(name)) return copy(key, label);
  return copy("Energy", "Energy");
}

function sourceIdentity(source) {
  if (!source?.name) return "";
  const meter = source.isMeter === true || token(source.sourceType) === "sql_meter";
  const file = token(source.sourceFile || source.file).replace(/\\/g, "/").split("/").at(-1) || "eplusout.sql";
  return [file, meter ? "meter" : "variable", token(source.name), meter ? "" : token(source.keyValue || source.zoneName)].join("|");
}

function hourlyCalendar(label, leap) {
  const match = /^(\d{2})-(\d{2})\s+(\d{2}):00$/.exec(String(label || ""));
  if (!match) return null;
  const month = Number(match[1]), day = Number(match[2]), hour = Number(match[3]);
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (month < 1 || month > 12 || day < 1 || day > days[month - 1] || hour < 0 || hour > 24) return null;
  return { month, hour: (days.slice(0, month - 1).reduce((sum, count) => sum + count, 0) + day - 1) * 24 + hour };
}

export function renderEnergyPathComponentChart({ item = {}, kind = "node", label = "", frequency = "monthly", monthly = [], hourly = [], display } = {}) {
  frequency = frequency === "hourly" ? "hourly" : "monthly";
  const series = frequency === "hourly" ? hourly : monthly;
  const available = series.some((trace) => (trace.points || []).some((point) => finite(point.value))) &&
    (!display?.perArea || finite(display.areaM2) && display.areaM2 > 0);
  const heading = label || item.label || item.id || "";
  return `<aside class="energy-path-component-chart" data-energy-path-component-chart="${escapeHTML(item.id || "")}" data-energy-path-inspector="${escapeHTML(item.id || "")}"${kind === "link" ? ` data-energy-path-link-inspector="${escapeHTML(item.id || "")}"` : ""} data-energy-path-chart-kind="${kind}" data-energy-path-chart-mode="${frequency}">
    <header><strong>${escapeHTML(heading)}</strong><select data-energy-path-chart-frequency aria-label="${escapeHTML(copy("Frequency", "Chart frequency"))}">
      <option value="monthly"${frequency === "monthly" ? " selected" : ""}>${escapeHTML(copy("Monthly", "Monthly"))}</option>
      <option value="hourly"${frequency === "hourly" ? " selected" : ""}>${escapeHTML(copy("Hourly", "Hourly"))}</option>
    </select></header>
    ${available ? renderPlot(series, frequency, display) : `<div class="energy-path-chart-empty" data-energy-path-chart-empty>${escapeHTML(display?.perArea && !(display.areaM2 > 0)
      ? copy("AreaUnavailable", "Floor area is unavailable for this result.")
      : frequency === "hourly" ? copy("HourlyUnavailable", "Hourly data is unavailable for this component.") : copy("MonthlyUnavailable", "Monthly data is unavailable for this component."))}</div>`}
  </aside>`;
}

function renderPlot(series, frequency, display) {
  const width = 1000, height = 285, left = 76, right = 60, top = 32, bottom = 62;
  const plotWidth = width - left - right, plotHeight = height - top - bottom;
  const traces = series.map((trace) => ({ ...trace, points: trace.points || [] }));
  // Both thermal and site traces are canonical kWh, divided by one scope area.
  const points = traces.flatMap((trace) => trace.points).filter((point) => finite(point.value));
  const minimum = points.reduce((value, point) => Math.min(value, point.value), 0), maximum = points.reduce((value, point) => Math.max(value, point.value), 0);
  const low = minimum, high = maximum > minimum ? maximum : minimum + 1;
  const allX = traces.flatMap((trace) => trace.points).map((point) => point.x).filter(finite);
  const xMin = frequency === "monthly" ? -.5 : allX.reduce((value, next) => Math.min(value, next), Infinity), xMax = frequency === "monthly" ? 11.5 : allX.reduce((value, next) => Math.max(value, next), -Infinity);
  const x = (value) => xMax > xMin ? left + (value - xMin) / (xMax - xMin) * plotWidth : left + plotWidth / 2;
  const y = (value) => top + (high - value) / (high - low) * plotHeight;
  const baseline = y(0);
  const number = (value, unit = "kWh") => formatEnergyPathDisplayValue(value, unit, display, { includeUnit: false });
  const grid = Array.from({ length: 5 }, (_, index) => {
    const value = low + (high - low) * index / 4, position = y(value);
    return `<line class="energy-path-chart-grid" x1="${left}" y1="${position}" x2="${width - right}" y2="${position}"/><text class="energy-path-chart-tick" x="${left - 9}" y="${position + 4}" text-anchor="end">${escapeHTML(number(value))}</text>`;
  }).join("");
  const visiblePoints = traces[0]?.points || [];
  const ticks = frequency === "monthly" ? visiblePoints : visiblePoints.filter((_, index) => index === 0 || index === visiblePoints.length - 1 || index % Math.max(1, Math.floor(visiblePoints.length / 4)) === 0);
  const labels = ticks.map((point) => `<line class="energy-path-chart-grid" x1="${x(point.x)}" y1="${top}" x2="${x(point.x)}" y2="${height - bottom}"/><text class="energy-path-chart-tick" x="${x(point.x)}" y="${height - 36}" text-anchor="middle">${escapeHTML(point.label)}</text>`).join("");
  const marks = traces.map((trace, traceIndex) => {
    const color = traceIndex % 6;
    const title = (point) => `${point.label} · ${trace.label}: ${formatEnergyPathDisplayValue(point.value, trace.unit || "kWh", display)}`;
    if (frequency === "monthly") {
      const barWidth = plotWidth / 12 * .68 / traces.length;
      return trace.points.filter((point) => finite(point.value)).map((point) => `<rect class="energy-path-chart-mark energy-path-chart-color-${color}" data-energy-path-chart-value="${point.value}" x="${x(point.x) - barWidth * traces.length / 2 + traceIndex * barWidth}" y="${Math.min(y(point.value), baseline)}" width="${barWidth}" height="${Math.max(1, Math.abs(y(point.value) - baseline))}" tabindex="0"><title>${escapeHTML(title(point))}</title></rect>`).join("");
    }
    let previous = null;
    const drawn = hourlyPlotPoints(trace.points, Math.max(48, Math.floor(2400 / traces.length)));
    const path = drawn.map((point) => {
      if (!finite(point.value)) { previous = null; return ""; }
      const command = previous !== null && point.segment === previous ? "L" : "M";
      previous = point.segment;
      return `${command}${x(point.x)},${y(point.value)}`;
    }).join(" ");
    return `<path class="energy-path-chart-line energy-path-chart-color-${color}" d="${path}"/>` + drawn.filter((point) => finite(point.value)).map((point) => `<circle class="energy-path-chart-mark energy-path-chart-color-${color}" data-energy-path-chart-value="${point.value}" cx="${x(point.x)}" cy="${y(point.value)}" r="2.5"><title>${escapeHTML(title(point))}</title></circle>`).join("");
  }).join("");
  return `<div class="energy-path-chart-plot"><svg viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(copy(frequency === "hourly" ? "Hourly" : "Monthly", frequency === "hourly" ? "Hourly" : "Monthly"))}">
    <text class="energy-path-chart-axis-label" data-energy-path-chart-axis="y" x="${left}" y="17">${escapeHTML(`${frequency === "hourly" ? copy("Measured", "Measured energy") : copy("Component", "Component energy")} (${energyPathDisplayUnit("kWh", display)})`)}</text>
    <text class="energy-path-chart-axis-label" data-energy-path-chart-axis="x" x="${left + plotWidth / 2}" y="${height - 7}" text-anchor="middle">${escapeHTML(frequency === "hourly" ? copy("DateHour", "Date & hour") : copy("Month", "Month"))}</text>${grid}${labels}${marks}
  </svg></div>${traces.length > 1 || frequency === "hourly" ? `<div class="energy-path-chart-legend">${traces.map((trace, index) => `<span><i class="energy-path-chart-color-${index % 6}"></i>${escapeHTML(trace.label)}</span>`).join("")}</div>` : ""}`;
}

// Bound SVG geometry while preserving observed extrema and real missing-hour gaps.
// Every displayed mark remains an actual observation; chart data stays complete.
function hourlyPlotPoints(points, limit) {
  let segment = 0, previous = null;
  const records = points.map((point) => {
    if (!finite(point.value) || previous !== null && point.x - previous > 1) segment++;
    previous = point.x;
    return { ...point, segment };
  });
  if (records.length <= limit) return records;
  const size = Math.ceil(records.length / Math.max(1, Math.floor(limit / 2))), selected = new Set([0, records.length - 1]);
  for (let start = 0; start < records.length; start += size) {
    let min = start, max = start;
    for (let index = start; index < Math.min(start + size, records.length); index++) {
      if (records[index].value < records[min].value) min = index;
      if (records[index].value > records[max].value) max = index;
      if (index && records[index].segment !== records[index - 1].segment) { selected.add(index - 1); selected.add(index); }
    }
    selected.add(min); selected.add(max);
  }
  return [...selected].sort((left, right) => left - right).map((index) => records[index]);
}
