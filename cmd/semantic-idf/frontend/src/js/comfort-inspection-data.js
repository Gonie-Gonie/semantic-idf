import { simulationSeriesPointColumns } from "./simulation-result-transport.js";

const finite = (value) => typeof value === "number" && Number.isFinite(value);
const token = (value) => String(value || "").trim().toLowerCase().replace(/\s+/g, " ");
const cache = new WeakMap();

export const COMFORT_BUILDING_SCOPE = "__building__";

// Only comfort observations belong here. In particular, older bundles also
// contain zone heating/cooling loads, which must not become comfort charts.
function measurement(name, rawUnit) {
  const value = token(name);
  if (/energy|\bpower\b|heating rate|cooling rate|heat transfer/.test(value)) return null;
  let kind, label, category;
  if (/not met|unmet|not comfortable|discomfort/.test(value)) {
    category = "unmet";
    const occupied = /occupied/.test(value);
    if (/heating/.test(value)) { kind = occupied ? "unmetHeatingOccupied" : "unmetHeating"; label = occupied ? "Tset,h · occupied" : "Tset,h"; }
    else if (/cooling/.test(value)) { kind = occupied ? "unmetCoolingOccupied" : "unmetCooling"; label = occupied ? "Tset,c · occupied" : "Tset,c"; }
    else { kind = "discomfort"; label = /ashrae/.test(value) ? "ASHRAE 55" : "Comfort"; }
  } else if (/\bpmv\b|predicted mean vote/.test(value)) { kind = "pmv"; label = "PMV"; category = "pmv"; }
  else if (/\bppd\b|predicted percentage.*dissatisfied/.test(value)) { kind = "ppd"; label = "PPD"; category = "ppd"; }
  else if (/setpoint.*temperature|temperature.*setpoint/.test(value)) {
    kind = /heating/.test(value) ? "heatingSetpoint" : /cooling/.test(value) ? "coolingSetpoint" : "setpoint";
    label = kind === "heatingSetpoint" ? "Tset,h" : kind === "coolingSetpoint" ? "Tset,c" : "Tset";
    category = "temperature";
  } else if (/operative temperature/.test(value)) { kind = "operativeTemperature"; label = "Top"; category = "temperature"; }
  else if (/mean radiant temperature/.test(value)) { kind = "radiantTemperature"; label = "Tmrt"; category = "temperature"; }
  else if (/mean air temperature|zone air temperature/.test(value)) { kind = "temperature"; label = "T"; category = "temperature"; }
  else if (/relative humidity/.test(value)) { kind = "relativeHumidity"; label = "RH"; category = "humidity"; }
  else if (/humidity ratio/.test(value)) { kind = "humidity"; label = "w"; category = "humidity"; }
  else return null;
  let unit = String(rawUnit || "").trim(), factor = 1;
  if (category === "temperature" && (!unit || ["c", "degc", "°c"].includes(token(unit)))) unit = "°C";
  else if (kind === "humidity" && /kg.*\/kg/i.test(unit)) { unit = "g/kg"; factor = 1000; }
  else if (["pmv"].includes(kind)) unit = "–";
  else if (["ppd", "relativeHumidity"].includes(kind) && (!unit || unit === "%")) unit = "%";
  else if (category === "unmet" && (!unit || /^(h|hr|hrs|hour|hours)$/i.test(unit))) unit = "h";
  else if (!unit) unit = "–";
  return { kind, label, category, unit, factor };
}

function observations(metric, factor) {
  const byTime = new Map();
  const add = (x, label, rawValue) => {
    if (!finite(x)) return;
    const value = finite(rawValue) && rawValue > -999 ? rawValue * factor : null;
    // Ambiguous duplicate timestamps are gaps, not a silently chosen reading.
    byTime.set(x, byTime.has(x) ? { ...byTime.get(x), value: null }
      : { x, label: String(label ?? x), value });
  };
  // Preserve raw units and the deferred source arrays. Preparing Comfort should
  // not first expand another object graph for every transported observation.
  const columns = simulationSeriesPointColumns(metric, { display: false });
  let rawCount;
  if (columns) {
    rawCount = columns.rows.length;
    // Compact point materialization omits empty labels; preserve its numeric
    // fallback while leaving the shared timeline's empty string untouched.
    for (let i = 0; i < rawCount; i++) add(columns.rows[i], columns.labels[i] || undefined, columns.values[i]);
  } else {
    const raw = metric.points || [];
    rawCount = raw.length;
    for (const point of raw) if (point) add(point.x, point.label, point.value);
  }
  return { points: [...byTime.values()].sort((a, b) => a.x - b.x), rawCount };
}

function summarize(points, metric, factor, rawCount) {
  let count = 0, total = 0, min = Infinity, max = -Infinity;
  for (const point of points) if (finite(point.value)) {
    count++; total += point.value; min = Math.min(min, point.value); max = Math.max(max, point.value);
  }
  if (count) return { count, average: total / count, min, max, total };
  // Old runs can include valid reported summaries without a time series.
  if (!rawCount && finite(metric.average)) return {
    count: 0, average: metric.average * factor,
    min: finite(metric.min) ? metric.min * factor : null,
    max: finite(metric.max) ? metric.max * factor : null, total: null,
  };
  return { count: 0, average: null, min: null, max: null, total: null };
}

function metricsFor(metrics, scope) {
  const seen = new Set();
  return (metrics || []).flatMap((metric) => {
    const kind = measurement(metric.name, metric.unit);
    if (!kind) return [];
    const keyValue = String(metric.keyValue || "").trim();
    const id = `${scope}|${token(metric.name)}|${token(keyValue)}|${kind.unit}`;
    if (seen.has(id)) return [];
    seen.add(id);
    const { points, rawCount } = observations(metric, kind.factor);
    return [{ ...kind, id, name: metric.name, keyValue, reportingFrequency: metric.reportingFrequency,
      points, summary: summarize(points, metric, kind.factor, rawCount) }];
  });
}

const calendarPattern = /^(?:(\d{4})[-/])?(\d{1,2})[-/](\d{1,2})\s+(\d{1,2}):(\d{2})(?::(\d{2}))?$/;

function calendarCoordinates(metrics) {
  const labels = new Map(), years = new Set();
  let leap = false;
  for (const metric of metrics) for (const point of metric.points) {
    if (labels.has(point.label)) continue;
    const match = calendarPattern.exec(point.label.trim());
    labels.set(point.label, match);
    if (match?.[1] && Number(match[1]) > 0) years.add(Number(match[1]));
    if (match && Number(match[2]) === 2 && Number(match[3]) === 29) leap = true;
  }
  // Yearless labels use one common reference calendar, never a separate leap
  // year for each People group, metric or zone. Do not guess a year rollover.
  const referenceYear = years.size === 1 ? [...years][0] : leap ? 2000 : 2001;
  const coordinates = new Map();
  for (const [label, match] of labels) {
    let value = null;
    if (match && (match[1] || years.size <= 1)) {
      const year = match[1] ? Number(match[1]) : referenceYear;
      const month = Number(match[2]), day = Number(match[3]), hour = Number(match[4]), minute = Number(match[5]), second = Number(match[6] || 0);
      if (year > 0 && month >= 1 && month <= 12 && day >= 1 && hour <= 24 && minute < 60 && second < 60 && (hour !== 24 || minute === 0 && second === 0)) {
        const date = new Date(0);
        date.setUTCFullYear(year, month - 1, day);
        if (date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day) {
          date.setUTCHours(hour, minute, second, 0);
          value = date.getTime() / 3600000;
        }
      }
    }
    coordinates.set(label, value);
  }
  return coordinates;
}

function projectCalendar(metrics, coordinates) {
  const categories = new Map();
  for (const metric of metrics) {
    if (!categories.has(metric.category)) categories.set(metric.category, []);
    categories.get(metric.category).push(metric);
  }
  for (const group of categories.values()) {
    // A chart cannot mix row-number and calendar coordinates. Repeated,
    // backward or invalid dates retain the original axis for that category.
    if (!group.every((metric) => metric.points.every((point, index) => finite(coordinates.get(point.label))
      && (!index || coordinates.get(point.label) > coordinates.get(metric.points[index - 1].label))))) continue;
    for (const metric of group) {
      metric.points = metric.points.map((point) => ({ ...point, key: String(point.x), x: coordinates.get(point.label) }));
      if (token(metric.reportingFrequency) === "hourly") metric.interval = 1;
    }
  }
}

function buildingRow(row) {
  if (row.scope) return row.scope === "building";
  // Compatibility for saved runs predating the explicit scope field. Accept
  // only actual facility labels, never manufacture a total from zone hours.
  return /^(entire (facility|building)|facility|building|whole building)$/i.test(String(row.zoneName || "").trim());
}

function hoursFor(rows) {
  const seen = new Set();
  return rows.flatMap((row) => {
    const kind = measurement(row.metric, row.unit);
    if (!kind || kind.category !== "unmet" || !finite(row.value)) return [];
    // Reports can repeat the same quantity in multiple tables. Do not add
    // these repetitions together or display duplicate identical indicators.
    const id = `${kind.kind}|${kind.unit}|${row.value}`;
    if (seen.has(id)) return [];
    seen.add(id);
    return [{ ...kind, id, value: row.value * kind.factor }];
  });
}

export function prepareComfortInspection(comfort = {}) {
  if (cache.has(comfort)) return cache.get(comfort);
  const unmet = comfort.unmetHours || [];
  const zones = (comfort.zones || []).map((zone) => ({
    id: zone.zoneName, name: zone.zoneName,
    metrics: metricsFor(zone.metrics, zone.zoneName),
    hours: hoursFor(unmet.filter((row) => !buildingRow(row) && token(row.zoneName) === token(zone.zoneName))),
  })).filter((zone) => zone.name && (zone.metrics.length || zone.hours.length));
  // A zone with only a tabular comfort-hour observation is still selectable.
  for (const row of unmet) if (!buildingRow(row) && row.zoneName && !zones.some((zone) => token(zone.name) === token(row.zoneName))) {
    const hours = hoursFor(unmet.filter((item) => !buildingRow(item) && token(item.zoneName) === token(row.zoneName)));
    if (hours.length) zones.push({ id: row.zoneName, name: row.zoneName, metrics: [], hours });
  }
  const building = { id: COMFORT_BUILDING_SCOPE, name: "Building",
    metrics: metricsFor(comfort.buildingMetrics, COMFORT_BUILDING_SCOPE), hours: hoursFor(unmet.filter(buildingRow)) };
  const coordinates = calendarCoordinates([building, ...zones].flatMap((scope) => scope.metrics));
  for (const scope of [building, ...zones]) projectCalendar(scope.metrics, coordinates);
  const result = { zones, building };
  cache.set(comfort, result);
  return result;
}
