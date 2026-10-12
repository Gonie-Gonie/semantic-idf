import { t } from "./i18n.js";
import { escapeHTML } from "./state.js";
import { energyPathDisplayUnit, formatEnergyPathDisplayValue } from "./energy-path-display.js";

const finite = (value) => typeof value === "number" && Number.isFinite(value);
const copy = (key, fallback, values = {}) => t(`simulation.energyPathChart${key}`, values, fallback);
const members = (item) => item.groupedMembers?.length ? item.groupedMembers : [item];
const token = (value) => String(value || "").trim().toLowerCase();
let chartLabelContext = null;

// Keep the selected group's original members fixed while monthly grouping changes.
// Missing observations remain gaps; an annual total never supplies monthly values.
export function energyPathMonthlyChartSeries(item = {}, kind = "node", graphs = [], endpointLabels = {}) {
  const wanted = new Set(members(item).map((member) => member.id));
  const fields = kind === "link" ? ["fromValue", "toValue"] : ["value"];
  return fields.map((field) => ({
    id: field,
    label: kind === "link" ? endpointLabels[field === "fromValue" ? "from" : "to"] || monthlyLinkLabel(item, field) : item.label || item.id || "",
    unit: kind === "link" ? item[field === "fromValue" ? "fromUnit" : "toUnit"] || "kWh" : item.unit || "kWh",
    points: Array.from({ length: 12 }, (_, month) => {
      const graph = graphs[month] || {};
      const records = kind === "link" ? [...(graph.links || []), ...(graph.relations || [])] : graph.nodes || [];
      if (kind === "link" && item.relation === "input_to_driver") {
        const reference = inputContextMonthlyReference(item, field, graph, records, wanted, endpointLabels.fromNode);
        if (reference !== undefined) return { x: month, label: t(`simulation.month.${month + 1}`, {}, `M${month + 1}`), value: reference };
      }
      const found = new Map();
      for (const record of records) {
        for (const member of members(record)) {
          if (!wanted.has(member.id)) continue;
          const value = kind === "node" && member.level === "input" && finite(member.signedValue)
            ? member.signedValue : member[field];
          found.set(member.id, kind === "link" ? member : value);
        }
      }
      if (kind === "link" && ["input_to_driver", "load_to_auxiliary", "source_correspondence"].includes(item.relation)) {
        return { x: month, label: t(`simulation.month.${month + 1}`, {}, `M${month + 1}`),
          value: found.size === wanted.size ? contextMonthlyValue(item.relation, field, [...found.values()], records, wanted) : null };
      }
      const values = [...found.values()].map((value) => kind === "link" ? value[field] : value);
      const ratios = kind === "link" && endpointLabels.ratioForLink
        ? [...found.values()].map((record) => endpointLabels.ratioForLink(record, graph)) : [];
      const ratioKind = ratios.length && ratios.every((ratio) => ratio?.kind && ratio.kind === ratios[0].kind) ? ratios[0].kind : "";
      return {
        x: month,
        label: t(`simulation.month.${month + 1}`, {}, `M${month + 1}`),
        value: found.size === wanted.size && values.every(finite) ? values.reduce((sum, value) => sum + value, 0) : null,
        ...(kind === "link" ? { ratioKind } : {}),
      };
    }),
  }));
}

// Ratios use paired observations on one calendar, never averages of component
// ratios or pixel widths. A validated conversion kind is supplied by the graph's
// existing carrier/service validator; independent references retain a generic
// quantity-ratio label. Missing pairs and zero denominators remain gaps.
export function energyPathLinkRatioChartSeries(item = {}, series = [], conversionRatio = null) {
  const from = series.find((trace) => trace.id === "fromValue"), to = series.find((trace) => trace.id === "toValue");
  const reference = ["input_to_driver", "load_to_auxiliary", "source_correspondence"].includes(token(item.relation));
  const kind = token(item.relation) === "load_to_end_use" && conversionRatio?.kind ? conversionRatio.kind : "";
  const label = kind ? conversionRatio.label : copy(reference ? "ReferenceRatio" : "Ratio", reference ? "Reference quantity ratio" : "From / to ratio");
  const toPoints = new Map((to?.points || []).map((point) => [point.x, point]));
  const fromFactor = chartEnergyUnitFactor(from?.unit), toFactor = chartEnergyUnitFactor(to?.unit);
  return [{ id: "ratio", label, unit: "", ratio: true, ratioKind: kind || "quantity_ratio", reference,
    points: (from?.points || []).map((point) => {
      const other = toPoints.get(point.x);
      const validated = !kind || point.ratioKind === kind && other?.ratioKind === kind;
      const paired = finite(point.value) && finite(other?.value);
      const value = validated && fromFactor !== null && toFactor !== null && paired && other.value !== 0
        ? point.value * fromFactor / (other.value * toFactor) : null;
      const unavailableReason = !paired ? "missing_pair" : other.value === 0 ? "zero_denominator"
        : !validated || fromFactor === null || toFactor === null ? "unverified_pair" : !finite(value) ? "non_finite" : "";
      return { x: point.x, label: point.label, value: finite(value) ? value : null,
        fromValue: point.value, toValue: other?.value, fromUnit: from?.unit, toUnit: to?.unit,
        fromLabel: from?.label, toLabel: to?.label, unavailableReason };
    }),
  }];
}

function chartEnergyUnitFactor(unit) {
  const match = /^(j|kj|mj|gj|wh|kwh|mwh)(?:\s+(?:thermal|site|boundary))?$/.exec(token(unit));
  return match ? { j: 1 / 3600000, kj: 1 / 3600, mj: 1 / 3.6, gj: 1000 / 3.6, wh: 1 / 1000, kwh: 1, mwh: 1000 }[match[1]] : null;
}

function monthlyLinkLabel(item, field) {
  const from = field === "fromValue";
  if (item.relation === "input_to_driver") return t(`simulation.energyPathContext${from ? "FromInput" : "ToDriver"}`, {}, from ? "Reported observation" : "Allocated load-driver reference");
  if (item.relation === "load_to_auxiliary") return t(`simulation.energyPathContext${from ? "FromThermal" : "ToAuxiliary"}`, {}, from ? "Served thermal reference" : "Allocated auxiliary site energy");
  return from ? copy("From", "From") : copy("To", "To");
}

function inputContextMonthlyReference(item, field, graph, records, wanted, selectedInput) {
  if (!item.id || !item.fromId || !item.toId) return undefined;
  const sameScope = (record) => token(record.zoneName) === token(item.zoneName);
  const candidates = records.filter((record) => record.relation === item.relation && record.id === item.id &&
    record.fromId === item.fromId && record.toId === item.toId && sameScope(record));
  if (candidates.length > 1) return null;
  const record = candidates[0];
  // Annual signed surface exchange can contain cooling and heating drivers,
  // while a native month contains only one of them. The stable projected pair
  // identifies that month's complete reference; absent annual service branches
  // must not erase it. Reject any unrelated original relation joining the pair.
  if (record && !members(record).every((member) => wanted.has(member.id))) return null;
  if (field === "fromValue" && selectedInput) {
    // A context relation can reference the whole Solar/Equipment input card,
    // including native inputs not associated with this particular driver.
    // Compare that selected card's full roster before accepting a monthly sum.
    const inputs = (graph.nodes || []).filter((node) => node.id === item.fromId && node.level === "input" &&
      token(node.scaleDomain) === "boundary" && sameScope(node));
    if (inputs.length !== 1 || selectedInput.id !== item.fromId || !sameScope(selectedInput)) return null;
    const expected = new Set(members(selectedInput).map((member) => member.id));
    const observed = new Set(members(inputs[0]).map((member) => member.id));
    if (!expected.size || expected.size !== observed.size || ![...expected].every((id) => observed.has(id))) return null;
  }
  if (record && finite(record[field])) return record[field];
  if (!record && field === "toValue") {
    // Known-zero drivers stay ungrouped in the visible graph. Their original
    // endpoint observations still prove zero; the lack of a display-group ID
    // cannot turn that explicit value into a missing month.
    const related = records.filter((entry) => entry.relation === item.relation && sameScope(entry) &&
      members(entry).some((member) => wanted.has(member.id)));
    if (!related.every((entry) => members(entry).every((member) => wanted.has(member.id)))) return null;
    const observations = new Map();
    for (const entry of related) for (const member of members(entry)) {
      if (!member.toId || !finite(member.toValue) || observations.has(member.toId) && observations.get(member.toId) !== member.toValue) return null;
      observations.set(member.toId, member.toValue);
    }
    const total = [...observations.values()].reduce((sum, value) => sum + value, 0);
    return observations.size && finite(total) ? total : null;
  }
  if (field !== "fromValue") return null;
  // An observed input remains useful when no room-driver reference exists in
  // that month. It cannot establish a zero driver or supply a missing ratio.
  const inputs = (graph.nodes || []).filter((node) => node.id === item.fromId && node.level === "input" &&
    token(node.scaleDomain) === "boundary" && sameScope(node));
  if (inputs.length !== 1) return null;
  const input = inputs[0], inputIDs = new Set(members(input).map((member) => member.id));
  const selectedInputIDs = new Set(members(item).map((member) => member.fromId).filter(Boolean));
  if (!selectedInputIDs.size || ![...selectedInputIDs].every((id) => inputIDs.has(id))) return null;
  const value = Object.hasOwn(input, "signedValue") ? input.signedValue : input.value;
  return finite(value) ? value : null;
}

// Context endpoints retain independent reference quantities. Repeated input
// endpoints count once; each auxiliary owner keeps its own served-load subset.
function contextMonthlyValue(relation, field, links, records, wanted) {
  if (relation === "load_to_auxiliary") {
    // Fan and pump served-load sets can overlap without being identical.
    // Keep each source attribution separate rather than invent their union.
    return links.length === 1 && finite(links[0][field]) ? links[0][field] : null;
  }
  if (relation === "source_correspondence") {
    // Cooling and heating drivers can reference one measured direct-use total.
    // Sum distinct original driver endpoints; count that shared site endpoint
    // once. A changed group may not import an unrelated original member.
    for (const record of records) {
      const original = members(record);
      if (original.some((member) => wanted.has(member.id)) && !original.every((member) => wanted.has(member.id))) return null;
    }
    const observations = new Map(), endpointField = field === "fromValue" ? "fromId" : "toId";
    for (const link of links) {
      const endpoint = link[endpointField], value = link[field];
      if (!endpoint || !finite(value) || observations.has(endpoint) && observations.get(endpoint) !== value) return null;
      observations.set(endpoint, value);
    }
    const total = [...observations.values()].reduce((sum, value) => sum + value, 0);
    return observations.size && finite(total) ? total : null;
  }
  const observations = new Map();
  for (const record of records) {
    const original = members(record);
    if (!original.some((member) => wanted.has(member.id))) continue;
    if (!original.every((member) => wanted.has(member.id))) return null;
    const endpoint = record[field === "fromValue" ? "fromId" : "toId"], value = record[field];
    if (observations.has(endpoint) && observations.get(endpoint) !== value) return null;
    observations.set(endpoint, value);
  }
  const values = [...observations.values()];
  return values.length && values.every(finite) ? values.reduce((sum, value) => sum + value, 0) : null;
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

export function renderEnergyPathComponentChart({ item = {}, kind = "node", label = "", frequency = "monthly", monthly = [], hourly = [], display, endpointLabels = {}, conversionRatio = null } = {}) {
  frequency = frequency === "hourly" ? "hourly" : "monthly";
  const series = frequency === "hourly" ? hourly : monthly;
  const available = series.some((trace) => (trace.points || []).some((point) => finite(point.value))) &&
    (!display?.perArea || finite(display.areaM2) && display.areaM2 > 0);
  const heading = label || item.label || item.id || "";
  const validatedConversion = token(item.relation) === "load_to_end_use" ? conversionRatio : null;
  const quantities = available ? renderPlot(series, frequency, display, item.level === "input") : `<div class="energy-path-chart-empty" data-energy-path-chart-empty>${escapeHTML(display?.perArea && !(display.areaM2 > 0)
    ? copy("AreaUnavailable", "Floor area is unavailable for this result.")
    : frequency === "hourly" ? copy("HourlyUnavailable", "Hourly data is unavailable for this component.") : copy("MonthlyUnavailable", "Monthly data is unavailable for this component."))}</div>`;
  const ratio = kind === "link" ? energyPathLinkRatioChartSeries(item, frequency === "monthly" ? monthly : [], validatedConversion) : [];
  const ratioAvailable = ratio.some((trace) => trace.points.some((point) => finite(point.value)));
  const ratioPanel = kind === "link" ? `<section class="energy-path-chart-panel" data-energy-path-chart-panel="ratio"><h4>${escapeHTML(ratio[0]?.label || copy("Ratio", "From / to ratio"))}</h4>
    ${ratioAvailable ? renderPlot(ratio, frequency, display, false, true) : `<div class="energy-path-chart-empty" data-energy-path-ratio-empty>${escapeHTML(chartRatioEmptyMessage(ratio))}</div>`}
    ${ratio[0]?.reference ? `<small class="energy-path-chart-note" data-energy-path-reference-note>${escapeHTML(copy("ReferenceRatioNote", "Reference quantities have different physical boundaries; this ratio is not a conversion efficiency."))}</small>` : ""}</section>` : "";
  return `<aside class="energy-path-component-chart" data-energy-path-component-chart="${escapeHTML(item.id || "")}" data-energy-path-inspector="${escapeHTML(item.id || "")}"${kind === "link" ? ` data-energy-path-link-inspector="${escapeHTML(item.id || "")}"` : ""} data-energy-path-chart-kind="${kind}" data-energy-path-chart-mode="${frequency}">
    <header><strong>${escapeHTML(heading)}</strong><select data-energy-path-chart-frequency aria-label="${escapeHTML(copy("Frequency", "Chart frequency"))}">
      <option value="monthly"${frequency === "monthly" ? " selected" : ""}>${escapeHTML(copy("Monthly", "Monthly"))}</option>
      <option value="hourly"${frequency === "hourly" ? " selected" : ""}>${escapeHTML(copy("Hourly", "Hourly"))}</option>
    </select></header>
    ${kind === "link" ? renderLinkQuantities(item, endpointLabels, display, validatedConversion, ratio[0]) : ""}
    <div class="energy-path-chart-panels${kind === "link" ? " has-ratio" : ""}"><section class="energy-path-chart-panel" data-energy-path-chart-panel="quantities">${kind === "link" ? `<h4>${escapeHTML(copy("Quantities", "Endpoint quantities"))}</h4>` : ""}${quantities}</section>${ratioPanel}</div>
  </aside>`;
}

function renderLinkQuantities(item, labels, display, conversionRatio, ratio) {
  const from = labels.from || monthlyLinkLabel(item, "fromValue"), to = labels.to || monthlyLinkLabel(item, "toValue");
  const fromFactor = chartEnergyUnitFactor(item.fromUnit), toFactor = chartEnergyUnitFactor(item.toUnit);
  const value = fromFactor !== null && toFactor !== null && finite(item.fromValue) && finite(item.toValue) && item.toValue !== 0
    ? item.fromValue * fromFactor / (item.toValue * toFactor) : null;
  return `<dl class="energy-path-chart-quantities"><div><dt>${escapeHTML(from)}</dt><dd>${escapeHTML(formatEnergyPathDisplayValue(item.fromValue, item.fromUnit || "kWh", display))}</dd></div><div><dt>${escapeHTML(to)}</dt><dd>${escapeHTML(formatEnergyPathDisplayValue(item.toValue, item.toUnit || "kWh", display))}</dd></div><div><dt>${escapeHTML(conversionRatio?.label || ratio?.label || copy("Ratio", "From / to ratio"))}</dt><dd data-energy-path-current-ratio="${finite(value) ? value : ""}">${escapeHTML(chartRatioValue(value))}</dd></div></dl>`;
}

function chartRatioValue(value) {
  if (finite(value) && Math.abs(value) < .005) value = 0;
  return finite(value) ? value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }) : "—";
}

function chartRatioGapMessage(reason, point = {}) {
  if (reason === "missing_pair" && finite(point.fromValue) !== finite(point.toValue)) {
    const endpoint = finite(point.fromValue) ? point.toLabel || copy("To", "To") : point.fromLabel || copy("From", "From");
    return copy("GapMissingEndpoint", "{endpoint} has no monthly observation.", { endpoint });
  }
  return {
    missing_pair: copy("GapMissing", "Paired monthly data is missing."),
    zero_denominator: copy("GapZeroDenominator", "The ratio is undefined because its denominator is zero."),
    unverified_pair: copy("GapUnverified", "The conversion boundary is not verified for this month."),
    non_finite: copy("GapNonFinite", "The ratio is outside the numeric range."),
  }[reason] || copy("RatioUnavailable", "A ratio requires paired data and a nonzero denominator.");
}

function chartRatioEmptyMessage(series) {
  const reasons = new Set(series.flatMap((trace) => trace.points || []).filter((point) => point.unavailableReason).map((point) => chartRatioGapMessage(point.unavailableReason, point)));
  return reasons.size ? [...reasons].join(" ")
    : copy("RatioUnavailable", "A ratio requires paired data and a nonzero denominator.");
}

function chartTooltipEnergyValue(value, unit, display) {
  const factor = chartEnergyUnitFactor(unit);
  if (display?.perArea && factor !== null && (!finite(display.areaM2) || display.areaM2 <= 0)) {
    return formatEnergyPathDisplayValue(value, unit || "kWh", display);
  }
  const displayed = finite(value) && display?.perArea && factor !== null && display.areaM2 > 0 ? value * factor / display.areaM2 : value;
  // A rounded 0.00 must not imply that a tiny but positive denominator was zero.
  return finite(displayed) && displayed !== 0 && Math.abs(displayed) < .005
    ? `${displayed.toExponential()} ${energyPathDisplayUnit(unit || "kWh", display)}`
    : formatEnergyPathDisplayValue(value, unit || "kWh", display);
}

// A few ratio spikes must not flatten the ordinary observations. This only
// chooses the linear display range; every original value remains inspectable.
function chartRatioOutlierLimit(points) {
  const magnitudes = points.map((point) => Math.abs(point.value)).filter((value) => finite(value) && value > 0).sort((left, right) => left - right);
  if (magnitudes.length < 3) return Infinity;
  const middle = Math.floor(magnitudes.length / 2);
  const median = magnitudes.length % 2 ? magnitudes[middle] : magnitudes[middle - 1] / 2 + magnitudes[middle] / 2;
  const limit = median * 10;
  return magnitudes.filter((value) => value <= limit).length >= 2 ? limit : Infinity;
}

function renderPlot(series, frequency, display, boundaryInput = false, ratio = false) {
  const width = 1000, height = 340, right = 42, top = 52, bottom = 81;
  const traces = series.map((trace) => ({ ...trace, points: trace.points || [] }));
  // Both thermal and site traces are canonical kWh, divided by one scope area.
  const points = traces.flatMap((trace) => trace.points).filter((point) => finite(point.value));
  const outlierLimit = ratio ? chartRatioOutlierLimit(points) : Infinity;
  const isOutlier = (point) => finite(point.value) && Math.abs(point.value) > outlierLimit;
  const axisPoints = points.filter((point) => !isOutlier(point));
  const outliers = points.filter(isOutlier);
  const minimum = axisPoints.reduce((value, point) => Math.min(value, point.value), 0), maximum = axisPoints.reduce((value, point) => Math.max(value, point.value), 0);
  const low = minimum, high = maximum > minimum ? maximum : minimum + 1;
  const number = (value, unit = "kWh") => ratio ? chartRatioValue(value) : formatEnergyPathDisplayValue(value, unit, display, { includeUnit: false });
  const yTicks = Array.from({ length: 5 }, (_, index) => {
    const value = low + (high - low) * index / 4;
    const displayed = !ratio && display?.perArea ? value / display.areaM2 : value;
    const fullLabel = number(value);
    const label = Math.abs(displayed) >= 10000 ? displayed.toLocaleString(undefined, { notation: "compact", maximumFractionDigits: 2 }) : fullLabel;
    return { value, label, fullLabel };
  });
  const left = Math.max(112, ...yTicks.map((tick) => Math.ceil(chartLabelWidth(tick.label) + 18)));
  const plotWidth = width - left - right, plotHeight = height - top - bottom;
  const allX = traces.flatMap((trace) => trace.points).map((point) => point.x).filter(finite);
  const xMin = frequency === "monthly" ? -.5 : allX.reduce((value, next) => Math.min(value, next), Infinity), xMax = frequency === "monthly" ? 11.5 : allX.reduce((value, next) => Math.max(value, next), -Infinity);
  const x = (value) => xMax > xMin ? left + (value - xMin) / (xMax - xMin) * plotWidth : left + plotWidth / 2;
  const y = (value) => top + (high - value) / (high - low) * plotHeight;
  const baseline = y(0);
  const grid = yTicks.map((tick) => {
    const position = y(tick.value);
    return `<line class="energy-path-chart-grid" x1="${left}" y1="${position}" x2="${width - right}" y2="${position}"/><text class="energy-path-chart-tick" data-energy-path-chart-tick="y" x="${left - 10}" y="${position + 5}" text-anchor="end">${escapeHTML(tick.label)}<title>${escapeHTML(tick.fullLabel)}</title></text>`;
  }).join("");
  const visiblePoints = traces[0]?.points || [];
  const ticks = frequency === "monthly" ? visiblePoints : hourlyChartTicks(visiblePoints, x);
  const labels = ticks.map((point, index) => {
    const anchor = frequency === "monthly" || ticks.length < 2 ? "middle" : index === 0 ? "start" : index === ticks.length - 1 ? "end" : "middle";
    return `<line class="energy-path-chart-grid" x1="${x(point.x)}" y1="${top}" x2="${x(point.x)}" y2="${height - bottom}"/><text class="energy-path-chart-tick" data-energy-path-chart-tick="x" x="${x(point.x)}" y="${height - bottom + 27}" text-anchor="${anchor}">${escapeHTML(point.label)}</text>`;
  }).join("");
  const marks = traces.map((trace, traceIndex) => {
    const color = traceIndex % 6;
    const title = (point) => `${point.label} · ${trace.label}: ${ratio ? chartRatioValue(point.value) : formatEnergyPathDisplayValue(point.value, trace.unit || "kWh", display)}` +
      (ratio ? ` · ${point.fromLabel || copy("From", "From")}: ${chartTooltipEnergyValue(point.fromValue, point.fromUnit, display)} · ${point.toLabel || copy("To", "To")}: ${chartTooltipEnergyValue(point.toValue, point.toUnit, display)}` : "");
    if (frequency === "monthly" && !ratio) {
      const barWidth = plotWidth / 12 * .68 / traces.length;
      return trace.points.filter((point) => finite(point.value)).map((point) => `<rect class="energy-path-chart-mark energy-path-chart-color-${color}" data-energy-path-chart-${ratio ? "ratio" : "value"}="${point.value}" x="${x(point.x) - barWidth * traces.length / 2 + traceIndex * barWidth}" y="${Math.min(y(point.value), baseline)}" width="${barWidth}" height="${Math.max(1, Math.abs(y(point.value) - baseline))}" tabindex="0"><title>${escapeHTML(title(point))}</title></rect>`).join("");
    }
    let previous = null;
    const drawn = hourlyPlotPoints(trace.points, Math.max(48, Math.floor(2400 / traces.length)));
    const path = drawn.map((point) => {
      if (!finite(point.value) || isOutlier(point)) { previous = null; return ""; }
      const command = previous !== null && point.segment === previous ? "L" : "M";
      previous = point.segment;
      return `${command}${x(point.x)},${y(point.value)}`;
    }).join(" ");
    const gaps = ratio && frequency === "monthly" ? trace.points.filter((point) => !finite(point.value)).map((point) => {
      const gapTitle = `${point.label} · ${chartRatioGapMessage(point.unavailableReason, point)}`;
      return `<g class="energy-path-chart-gap" data-energy-path-chart-gap="${escapeHTML(point.unavailableReason || "missing_pair")}" transform="translate(${x(point.x)},${baseline - 8})" tabindex="0" role="img" aria-label="${escapeHTML(gapTitle)}"><title>${escapeHTML(gapTitle)}</title><circle class="energy-path-chart-hit" r="12"/><path d="M-6,-6L6,6M-6,6L6,-6"/></g>`;
    }).join("") : "";
    return `<path class="energy-path-chart-line energy-path-chart-color-${color}" d="${path}"/>` + drawn.filter((point) => finite(point.value)).map((point) => {
      if (isOutlier(point)) {
        const side = point.value > high ? "upper" : "lower", position = side === "upper" ? top + 14 : height - bottom - 16;
        const outlierTitle = `${title(point)} · ${copy("OutlierPoint", "Outside the automatic axis range; actual value is preserved.")}`;
        return `<g class="energy-path-chart-outlier energy-path-chart-color-${color}" data-energy-path-chart-ratio="${point.value}" data-energy-path-chart-outlier="${side}" transform="translate(${x(point.x)},${position})" tabindex="0" role="img" aria-label="${escapeHTML(outlierTitle)}"><title>${escapeHTML(outlierTitle)}</title><circle class="energy-path-chart-hit" r="18"/><text class="energy-path-chart-tick energy-path-chart-outlier-symbol" text-anchor="middle" dominant-baseline="central">≈</text></g>`;
      }
      return `${ratio ? `<circle class="energy-path-chart-hit" cx="${x(point.x)}" cy="${y(point.value)}" r="12"><title>${escapeHTML(title(point))}</title></circle>` : ""}<circle class="energy-path-chart-mark energy-path-chart-color-${color}" data-energy-path-chart-${ratio ? "ratio" : "value"}="${point.value}" cx="${x(point.x)}" cy="${y(point.value)}" r="${ratio ? 4 : 2.5}"${ratio ? ` tabindex="0"` : ""}><title>${escapeHTML(title(point))}</title></circle>`;
    }).join("") + gaps;
  }).join("");
  const scaleNote = outliers.length ? `<small class="energy-path-chart-note" data-energy-path-chart-scale-note>${escapeHTML(copy("OutlierNote", "Linear axis · ≈ marks values >10× the median magnitude. Original values are preserved."))}</small><div class="energy-path-chart-outlier-summary" data-energy-path-chart-outlier-summary>${escapeHTML(copy("OffScale", "Off scale"))}: ${outliers.map((point) => escapeHTML(`${point.label}: ${chartRatioValue(point.value)}`)).join(" · ")}</div>` : "";
  const gapNote = ratio && traces.some((trace) => trace.points.some((point) => !finite(point.value))) ? `<small class="energy-path-chart-note" data-energy-path-chart-gap-note>${escapeHTML(copy("GapNote", "× marks an unavailable ratio, not zero. Hover or focus each mark for its reason."))}</small>` : "";
  const denominatorNote = outliers.length && traces[0]?.ratioKind === "coefficient_of_performance" ? `<small class="energy-path-chart-note" data-energy-path-chart-denominator-note>${escapeHTML(copy("SmallDenominatorNote", "A very small equipment-energy denominator can produce a large ratio. This delivered-load/equipment-energy comparison is not the equipment's rated COP; separately shown fan/pump energy is excluded."))}</small>` : "";
  return `<div class="energy-path-chart-plot"><svg viewBox="0 0 ${width} ${height}" data-energy-path-chart-scale="linear" data-energy-path-chart-y-min="${low}" data-energy-path-chart-y-max="${high}" role="img" aria-label="${escapeHTML(copy(frequency === "hourly" ? "Hourly" : "Monthly", frequency === "hourly" ? "Hourly" : "Monthly"))}">
    <text class="energy-path-chart-axis-label" data-energy-path-chart-axis="y" x="${left}" y="40">${escapeHTML(ratio ? traces[0]?.label || copy("Ratio", "From / to ratio") : `${boundaryInput ? copy("Boundary", "Reported boundary energy") : frequency === "hourly" ? copy("Measured", "Measured energy") : copy("Component", "Component energy")} (${energyPathDisplayUnit("kWh", display)})`)}</text>
    <text class="energy-path-chart-axis-label" data-energy-path-chart-axis="x" x="${left + plotWidth / 2}" y="${height - 16}" text-anchor="middle">${escapeHTML(frequency === "hourly" ? copy("DateHour", "Date & hour") : copy("Month", "Month"))}</text>${grid}${labels}<line class="energy-path-chart-axis" x1="${left}" x2="${width - right}" y1="${baseline}" y2="${baseline}"/>${marks}
  </svg></div>${scaleNote}${gapNote}${denominatorNote}${traces.length > 1 || frequency === "hourly" ? `<div class="energy-path-chart-legend">${traces.map((trace, index) => `<span><i class="energy-path-chart-color-${index % 6}"></i>${escapeHTML(trace.label)}</span>`).join("")}</div>` : ""}`;
}

// Minimum chart widths bound corrected tick fonts to 34 SVG units. Reserve a
// little extra for the inherited UI family without measuring the mounted SVG.
function chartLabelWidth(label) {
  chartLabelContext ||= document.createElement("canvas").getContext("2d");
  if (!chartLabelContext) return String(label).length * 13;
  chartLabelContext.font = `34px ${getComputedStyle(document.documentElement).fontFamily}`;
  return chartLabelContext.measureText(String(label)).width;
}

function hourlyChartTicks(points, x) {
  if (points.length < 3) return points;
  const first = points[0], last = points.at(-1), result = [first];
  const candidates = points.filter((_, index) => index > 0 && index < points.length - 1 && index % Math.max(1, Math.floor(points.length / 4)) === 0);
  let previousRight = x(first.x) + chartLabelWidth(first.label);
  const lastLeft = x(last.x) - chartLabelWidth(last.label);
  for (const point of candidates) {
    const position = x(point.x), halfWidth = chartLabelWidth(point.label) / 2;
    if (position - halfWidth < previousRight + 10 || position + halfWidth > lastLeft - 10) continue;
    result.push(point);
    previousRight = position + halfWidth;
  }
  result.push(last);
  return result;
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
