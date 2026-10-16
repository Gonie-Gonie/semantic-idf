import { createHVACScheduleDayResolver, normalizeHVACOperationCalendar } from "./hvac-operation.js";

const MINUTES_PER_DAY = 1440;
const regularDays = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
const caches = new WeakMap();
const MAX_CACHE_ENTRIES = 24;
const MAX_CACHED_SEGMENTS = 30000;
const normalized = (value) => String(value || "").trim().toLowerCase();

function scheduleIdentity(node) {
  const type = normalized(node.source?.objectType);
  return {
    name: node.parameters?.find((parameter) => parameter.name === "Schedule Name")?.value || node.source?.objectName || "",
    namespace: type.startsWith("schedule:day:") ? "day" : type.startsWith("schedule:week:") ? "week" : "annual",
  };
}

function appendSegment(segments, startMinute, endMinute, value) {
  const previous = segments[segments.length - 1];
  if (previous && previous.endMinute === startMinute && previous.value === value) previous.endMinute = endMinute;
  else segments.push({ startMinute, endMinute, value });
}

function timelineTicks(start, days, view, calendar) {
  if (view === "day") return [0, 6, 12, 18, 24].map((hour) => ({ minute: hour * 60, label: `${String(hour).padStart(2, "0")}:00`, kind: "hour", hour }));
  if (view === "week") return Array.from({ length: days + 1 }, (_, index) => {
    const date = new Date(start.getTime() + index * 86400000);
    const selected = regularDays.indexOf(calendar.dayType);
    const dayType = index === 0 ? calendar.dayType : regularDays[((selected < 0 ? start.getUTCDay() : selected) + index) % 7];
    return { minute: index * MINUTES_PER_DAY, label: `${date.getUTCMonth() + 1}/${date.getUTCDate()}`, kind: "day", dayType, month: date.getUTCMonth() + 1, day: date.getUTCDate() };
  });
  return Array.from({ length: 13 }, (_, month) => ({
    minute: (Date.UTC(start.getUTCFullYear(), month, 1) - start.getTime()) / 60000,
    label: `${month % 12 + 1}/1`, kind: "month", month: month % 12 + 1, day: 1,
  }));
}

/** Build source schedule timelines, never a combined equipment permission.
 * Day uses the selected date/day type. Week advances seven dates from that
 * anchor; special day types apply only to its first day. Annual uses the chosen
 * Gregorian year and standard weekdays, without inventing holidays or DST.
 * Native minute changes and missing intervals survive run-length compression.
 * Callers must treat the schedule array and returned timeline as immutable. */
export function buildHVACScheduleTimeline(node = {}, schedules = [], options = {}, { view: requestedView } = {}) {
  const calendar = normalizeHVACOperationCalendar(options);
  const view = ["day", "week", "year"].includes(requestedView) ? requestedView : calendar.timelineView;
  const identity = scheduleIdentity(node);
  const key = JSON.stringify([identity.namespace, normalized(identity.name), view, calendar.year,
    ...(view === "year" ? [] : [calendar.month, calendar.day, calendar.dayType])]);
  let cache = caches.get(schedules);
  if (!cache) { cache = { entries: new Map(), segments: 0 }; caches.set(schedules, cache); }
  if (cache.entries.has(key)) {
    const result = cache.entries.get(key);
    cache.entries.delete(key); cache.entries.set(key, result);
    return result;
  }

  const start = new Date(Date.UTC(calendar.year, view === "year" ? 0 : calendar.month - 1, view === "year" ? 1 : calendar.day));
  const days = view === "year" ? (Date.UTC(calendar.year + 1, 0, 1) - start.getTime()) / 86400000 : view === "week" ? 7 : 1;
  const anchorWeekday = regularDays.indexOf(calendar.dayType);
  const firstWeekday = anchorWeekday < 0 ? start.getUTCDay() : anchorWeekday;
  const segments = [];
  let positiveMinutes = 0, unknownMinutes = 0, minimum = Infinity, maximum = -Infinity;
  for (let index = 0; index < days; index++) {
    const date = new Date(start.getTime() + index * 86400000);
    const dayType = view === "year" ? regularDays[date.getUTCDay()]
      : index === 0 ? calendar.dayType : regularDays[(firstWeekday + index) % 7];
    const resolve = createHVACScheduleDayResolver(schedules, { year: date.getUTCFullYear(), month: date.getUTCMonth() + 1, day: date.getUTCDate(), dayType });
    const values = resolve(identity.name, identity.namespace);
    let beginning = 0, previous = values[0];
    for (let minute = 0; minute < MINUTES_PER_DAY; minute++) {
      const value = typeof values[minute] === "number" && Number.isFinite(values[minute]) ? values[minute] : null;
      if (minute === 0) previous = value;
      if (value !== previous) {
        appendSegment(segments, index * MINUTES_PER_DAY + beginning, index * MINUTES_PER_DAY + minute, previous);
        beginning = minute; previous = value;
      }
      if (value === null) unknownMinutes++;
      else {
        if (value > 0) positiveMinutes++;
        minimum = Math.min(minimum, value); maximum = Math.max(maximum, value);
      }
    }
    appendSegment(segments, index * MINUTES_PER_DAY + beginning, (index + 1) * MINUTES_PER_DAY, previous);
  }
  const totalMinutes = days * MINUTES_PER_DAY;
  const result = {
    view, calendar, startDate: { year: start.getUTCFullYear(), month: start.getUTCMonth() + 1, day: start.getUTCDate() },
    scheduleName: identity.name, namespace: identity.namespace, segments, totalMinutes,
    ticks: timelineTicks(start, days, view, calendar), resolutionMinutes: 1,
    status: unknownMinutes === totalMinutes ? "unknown" : unknownMinutes ? "partial" : "known",
    positiveMinutes, unknownMinutes, hasPositive: positiveMinutes > 0,
    allOff: positiveMinutes > 0 ? false : unknownMinutes > 0 ? null : true,
    minimum: minimum === Infinity ? null : minimum, maximum: maximum === -Infinity ? null : maximum,
    basis: view === "year" ? "standard_gregorian_weekdays" : "selected_date_day_type",
  };
  // Keep cache retention bounded even for schedules that change every minute.
  // Dense source evidence stays available, but does not become a permanent cache.
  if (segments.length <= MAX_CACHED_SEGMENTS) {
    while (cache.entries.size >= MAX_CACHE_ENTRIES || cache.segments + segments.length > MAX_CACHED_SEGMENTS) {
      const first = cache.entries.keys().next().value;
      cache.segments -= cache.entries.get(first).segments.length; cache.entries.delete(first);
    }
    cache.entries.set(key, result); cache.segments += segments.length;
  }
  return result;
}
