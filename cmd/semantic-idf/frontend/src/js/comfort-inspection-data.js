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

function observations(points, factor) {
  const byTime = new Map();
  for (const point of points || []) {
    if (!point || !finite(point.x)) continue;
    const value = finite(point.value) && point.value > -999 ? point.value * factor : null;
    // Ambiguous duplicate timestamps are gaps, not a silently chosen reading.
    byTime.set(point.x, byTime.has(point.x) ? { ...byTime.get(point.x), value: null }
      : { x: point.x, label: String(point.label ?? point.x), value });
  }
  return [...byTime.values()].sort((a, b) => a.x - b.x);
}

function summarize(points, metric, factor) {
  let count = 0, total = 0, min = Infinity, max = -Infinity;
  for (const point of points) if (finite(point.value)) {
    count++; total += point.value; min = Math.min(min, point.value); max = Math.max(max, point.value);
  }
  if (count) return { count, average: total / count, min, max, total };
  // Old runs can include valid reported summaries without a time series.
  if (!(metric.points || []).length && finite(metric.average)) return {
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
    const points = observations(metric.points, kind.factor);
    return [{ ...kind, id, name: metric.name, keyValue, points, summary: summarize(points, metric, kind.factor) }];
  });
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
  const result = { zones, building };
  cache.set(comfort, result);
  return result;
}
