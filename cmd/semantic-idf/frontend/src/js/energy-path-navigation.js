import { resolveEnergyPathOutputRequest } from "./energy-path-output-requests.js";

const value = (input) => String(input ?? "").trim();
const token = (input) => value(input).toLowerCase();
const frequency = (input) => token(input).replace(/\s+/g, "");

export function energyPathSeriesID(series = {}) {
  series = series || {};
  const legacy = `${series.file || ""}::${series.column || ""}`;
  return value(series.sourceId) ? `${legacy}::${value(series.sourceId)}` : legacy;
}

function sourceLeaves(node, sources) {
  const byID = new Map();
  for (const source of sources) {
    if (!source?.id) continue;
    byID.set(source.id, [...(byID.get(source.id) || []), source]);
  }
  const visited = new Set();
  const leaves = [];
  const visit = (id) => {
    if (!id || visited.has(id)) return;
    visited.add(id);
    for (const source of byID.get(id) || []) {
      if (Array.isArray(source.inputSourceIds) && source.inputSourceIds.length) {
        source.inputSourceIds.forEach(visit);
      } else if (!token(source.sourceType).startsWith("derived") && token(source.sourceType) !== "sql_tabular") {
        leaves.push(source);
      }
    }
  };
  (Array.isArray(node?.sourceIds) ? node.sourceIds : []).forEach(visit);
  return leaves;
}

function fileMatches(source, series) {
  const wanted = value(source.sourceFile || source.file).replace(/\\/g, "/").toLowerCase();
  if (!wanted) return true;
  const actual = value(series.file).replace(/\\/g, "/").toLowerCase();
  if (!actual) return false;
  if (wanted === actual) return true;
  // Series.File is a basename in the Go SQL/CSV parsers. Compare a basename
  // only when one side really is a basename, never two conflicting full paths.
  return (!wanted.includes("/") || !actual.includes("/")) && wanted.split("/").at(-1) === actual.split("/").at(-1);
}

function seriesMetadata(series, source) {
  let column = value(series.column);
  const suffix = column.match(/\((Annual|Run\s*Period|Monthly|Daily|Hourly|Time\s*Step|Detailed)\)\s*$/i);
  if (suffix) column = column.slice(0, suffix.index).trim();
  column = column.replace(/\s*\[[^\]]+\]\s*$/, "").trim();
  const colon = column.indexOf(":");
  const knownMeter = typeof series.isMeter === "boolean" ? series.isMeter : null;
  const sameSource = value(series.sourceId) !== "" && value(series.sourceId) === value(source.id);
  const sourceMeter = source.isMeter === true || token(source.sourceType) === "sql_meter";
  // The same dictionary ID can establish the class when an older SQL schema
  // omitted IsMeter. Otherwise an untyped column is not assumed to be a meter.
  const isMeter = knownMeter ?? (sameSource ? sourceMeter : null);
  const name = value(series.name) || (isMeter === true ? column : colon >= 0 ? column.slice(colon + 1).trim() : column);
  const key = value(series.keyValue) || (isMeter === true ? "" : colon >= 0 ? column.slice(0, colon).trim() : "");
  return { sameSource, isMeter, name, key, frequency: value(series.reportingFrequency) || suffix?.[1] || "" };
}

/**
 * Return every defensible source/series choice. Distinct source observations
 * remain explicit choices; indistinguishable candidates are marked ambiguous,
 * never collapsed to whichever appeared first. Input payloads are not mutated.
 */
export function resolveEnergyPathSeriesCandidates(node = {}, sources = [], series = []) {
  const observations = Array.isArray(series) ? series.filter((item) => item && typeof item === "object") : [];
  const identities = new Map();
  observations.forEach((item) => {
    const id = energyPathSeriesID(item);
    identities.set(id, (identities.get(id) || 0) + 1);
  });
  const output = new Map();
  for (const source of sourceLeaves(node, Array.isArray(sources) ? sources : [])) {
    const matches = [];
    for (const item of observations) {
      if (!fileMatches(source, item)) continue;
      const metadata = seriesMetadata(item, source);
      if (value(item.sourceId) && !metadata.sameSource) continue;
      if (metadata.isMeter === null) continue;
      if (frequency(source.reportingFrequency) && frequency(source.reportingFrequency) !== frequency(metadata.frequency)) continue;
      if (!metadata.frequency) continue;
      const request = {
        objectType: metadata.isMeter ? value(source.objectType) || "Output:Meter" : "Output:Variable",
        keyValue: metadata.isMeter ? metadata.key || metadata.name : metadata.key,
        variableName: metadata.isMeter ? "" : metadata.name,
        reportingFrequency: metadata.frequency,
      };
      // A shared dictionary ID is exact source identity even when the source
      // record omitted frequency, but never overrides a known contradiction.
      const observed = metadata.sameSource && !value(source.reportingFrequency)
        ? { ...source, reportingFrequency: metadata.frequency }
        : source;
      if (resolveEnergyPathOutputRequest(observed, [request]).status !== "exact") continue;
      matches.push({ source, series: item, seriesId: energyPathSeriesID(item), status: "exact" });
    }
    for (const match of matches) {
      if (matches.length > 1 || identities.get(match.seriesId) > 1) match.status = "ambiguous";
      const previous = output.get(match.seriesId);
      if (previous && previous.source.id !== match.source.id) match.status = "ambiguous";
      if (!previous || match.status === "ambiguous") output.set(match.seriesId, match);
    }
  }
  return [...output.values()];
}

function pointCalendar(label) {
  const input = value(label);
  const iso = input.match(/^(\d{4})-(\d{2})-(\d{2})(?:[T ](\d{2}):(\d{2})(?::(\d{2})(?:\.\d+)?)?(?:Z|[+-]\d{2}:?\d{2})?)?$/);
  const energyPlus = input.match(/^(\d{2})-(\d{2})\s+(\d{2}):(\d{2})(?::(\d{2}))?$/);
  if (!iso && !energyPlus) return null;
  const year = iso ? Number(iso[1]) : null;
  const month = Number(iso ? iso[2] : energyPlus[1]);
  const day = Number(iso ? iso[3] : energyPlus[2]);
  const hour = Number((iso ? iso[4] : energyPlus[3]) || 0);
  const minute = Number((iso ? iso[5] : energyPlus[4]) || 0);
  const second = Number((iso ? iso[6] : energyPlus[5]) || 0);
  const monthDays = [31, year === null || year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (month < 1 || month > 12 || day < 1 || day > monthDays[month - 1] || hour > 24 || minute > 59 || second > 59 || hour === 24 && (minute !== 0 || second !== 0)) return null;
  return { year, month, key: [year ?? "unknown", month, day, hour, minute, second].join("|") };
}

/** Inclusive point-array indices; no assumption that array index is a month. */
export function energyPathSeriesPeriodRange(series = {}, period = "annual") {
  const points = Array.isArray(series?.points) ? series.points : [];
  const reported = (point) => typeof point?.value === "number" && Number.isFinite(point.value);
  if (token(period) === "annual") return points.length && points.every(reported) ? { start: 0, end: -1 } : null;
  const match = value(period).match(/^M([1-9]|1[0-2])$/i);
  if (!match) return null;
  const calendars = points.map((point) => pointCalendar(point?.label));
  if (!calendars.length || calendars.some((point) => !point)) return null;
  const wanted = Number(match[1]);
  const indices = calendars.flatMap((point, index) => point.month === wanted ? [index] : []);
  if (!indices.length || indices.at(-1) - indices[0] + 1 !== indices.length) return null;
  const selected = indices.map((index) => calendars[index]);
  if (new Set(selected.map((point) => point.year)).size !== 1 || new Set(selected.map((point) => point.key)).size !== selected.length) return null;
  if (!indices.every((index) => reported(points[index]))) return null;
  return { start: indices[0], end: indices.at(-1) };
}
