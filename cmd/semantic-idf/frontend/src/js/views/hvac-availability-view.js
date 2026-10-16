import { escapeHTML } from "../state.js";
import { getLanguage, t } from "../i18n.js";
import { buildHVACScheduleTimeline } from "../hvac-availability-timeline.js";

const safe = escapeHTML;
const copy = (key, fallback) => t(`hvac.operation.${key}`, {}, fallback);
const number = (value) => {
  const numeric = Number(value);
  if (numeric === 0) return "0";
  return numeric.toLocaleString(getLanguage(), Math.abs(numeric) < .001
    ? { notation: "scientific", maximumSignificantDigits: 6 }
    : { maximumFractionDigits: 3 });
};
const models = new Map();
let nextID = 0;

function definitionFields(node) {
  return `<article class="hvac-availability-definition" data-hvac-definition-node="${safe(node.id)}"><span class="hvac-availability-definition-kind">${safe(copy(`control.${node.code}`, String(node.code || node.kind || "").replaceAll("_", " ")))}</span><strong>${safe(node.source?.objectName || node.code)}</strong><span>${safe(node.source?.objectType || "")}</span><dl>${(node.parameters || []).map((parameter) => `<div><dt>${safe(parameter.name)}</dt><dd>${safe(String(parameter.value))}</dd></div>`).join("")}</dl></article>`;
}

function scheduleRows(graph) {
  const nodes = graph.nodes || [], byID = new Map(nodes.map((node) => [node.id, node]));
  return nodes.filter((node) => (node.kind === "schedule" || node.code === "unresolved_schedule_reference") && !(graph.edges || []).some((edge) => edge.to === node.id && byID.get(edge.from)?.kind === "schedule"));
}

export function renderHVACAvailability(graph, schedules, calendar) {
  if (!graph.nodes?.length) return `<p class="hvac-operation-unresolved" data-hvac-graph-unavailable>${safe(copy("graphUnavailable", "This graph is not available in this report. Analyze the input again to resolve its source definitions."))}</p>`;
  const view = calendar.timelineView || "day", byID = new Map(graph.nodes.map((node) => [node.id, node]));
  const rows = scheduleRows(graph).map((node) => ({ node, timeline: buildHVACScheduleTimeline(node, schedules, calendar, { view }), owners: [...new Set((graph.edges || []).filter((edge) => edge.to === node.id).map((edge) => byID.get(edge.from)?.source?.objectName).filter(Boolean))] }));
  const id = `hvac-availability-${++nextID}`;
  models.set(id, { rows, calendar, view });
  while (models.size > 8) models.delete(models.keys().next().value);
  return `<div class="hvac-availability-graph" data-hvac-operation-graph="availability" data-hvac-timeline-id="${id}" role="group" aria-label="${safe(copy("availabilityGraph", "Availability definition graph"))}">
    <div class="hvac-availability-toolbar"><label>${safe(copy("timelineView", "View"))}<select data-hvac-operation-calendar="timelineView">${[["day", "representativeDay", "Representative day"], ["week", "representativeWeek", "Representative week"], ["year", "annual", "Annual"]].map(([value, key, label]) => `<option value="${value}"${value === view ? " selected" : ""}>${safe(copy(key, label))}</option>`).join("")}</select></label>${view === "year" ? `<label>${safe(copy("year", "Year"))}<input type="number" min="1900" max="2199" value="${calendar.year}" data-hvac-operation-calendar="year"/></label>` : ""}<button type="button" data-hvac-timeline-reset>${safe(copy("resetTimeline", "Reset view"))}</button><span>${safe(copy("timelineInteractions", "Wheel: zoom time · drag: pan · ← / →: pan · + / −: zoom"))}</span></div>
    <p class="hvac-availability-basis">${safe(copy(view === "year" ? "annualBasis" : view === "week" ? "weekBasis" : "dayBasis", view === "year" ? "Standard Gregorian weekdays; holidays, DST and runtime overrides are not applied." : view === "week" ? "Seven consecutive dates from the selected date and day type; source values, without averaging." : "Exact schedule values for the selected date and day type."))}</p>
    <div class="hvac-availability-legend"><span><i class="curve"></i>${safe(copy("rawScheduleValue", "Raw schedule value"))}</span><span><i class="zero"></i>${safe(copy("zeroScheduleValue", "Zero"))}</span><span><i class="unknown"></i>${safe(copy("unknownScheduleValue", "Unavailable"))}</span></div>
    <div class="hvac-availability-timelines" tabindex="0" role="group" aria-label="${safe(copy("timelineInteractions", "Wheel: zoom time · drag: pan · ← / →: pan · + / −: zoom"))}">${rows.map(({ node, owners, timeline }, index) => {
      const { minimum, maximum } = timeline;
      const presence = timeline.hasPositive ? "present" : timeline.allOff === true ? "absent" : "unknown";
      const presenceLabel = copy(`positiveValues.${presence}`, presence === "present" ? "Positive values present" : presence === "absent" ? "No positive values" : "Positive values unresolved");
      const name = node.parameters?.find((parameter) => parameter.name === "Schedule Name")?.value || node.source?.objectName || node.code;
      return `<article class="hvac-availability-timeline-row" data-hvac-control-node="${safe(node.id)}" data-hvac-node-kind="schedule" data-hvac-timeline-row="${index}"><div class="hvac-availability-timeline-heading"><strong class="hvac-control-source">${safe(name)}</strong><span>${safe(owners.join(" · "))}</span><span class="hvac-availability-presence ${presence}" data-hvac-positive-values="${presence}">${safe(presenceLabel)}</span><b>${minimum === null ? safe(copy("unknownScheduleValue", "Unavailable")) : safe(minimum === maximum ? number(minimum) : `${number(minimum)}–${number(maximum)}`)}</b></div><svg class="hvac-availability-timeline-plot" data-hvac-schedule-plot${timeline.status === "unknown" ? " data-hvac-schedule-unknown" : ""} role="img" aria-label="${safe(name)}" viewBox="0 0 800 112"></svg></article>`;
    }).join("") || `<p class="hvac-operation-unresolved">${safe(copy("noDeclaredSchedules", "No availability schedule is declared for this occurrence. See the source definitions below."))}</p>`}</div>
    <div class="hvac-availability-hover" data-hvac-timeline-hover aria-live="polite"></div>
    <details class="hvac-availability-provenance" data-hvac-definition-details><summary>${safe(copy("definitionReferences", "Source definitions and references"))}</summary><div class="hvac-availability-definition-list">${graph.nodes.map(definitionFields).join("")}</div><ul class="hvac-availability-reference-list">${(graph.edges || []).map((edge) => `<li data-hvac-definition-edge><strong>${safe(byID.get(edge.from)?.source?.objectName || edge.from)}</strong> → <span>${safe(byID.get(edge.to)?.source?.objectName || edge.to)}</span></li>`).join("")}</ul></details>
  </div>`;
}

function timelineDate(model, minute) {
  const calendar = model.rows[0]?.timeline.startDate || model.calendar, origin = Date.UTC(calendar.year, calendar.month - 1, calendar.day);
  return new Date(origin + minute * 60000);
}

function timeLabel(model, minute, span) {
  if (model.view === "day") return `${String(Math.floor(minute / 60)).padStart(2, "0")}:${String(Math.floor(minute % 60)).padStart(2, "0")}`;
  const weekTick = model.view === "week" && span > 3 * 1440 ? model.rows[0].timeline.ticks.find((tick) => tick.minute === minute) : null;
  if (weekTick?.dayType) return `${copy(`dayType.${weekTick.dayType}`, weekTick.dayType)} ${weekTick.month}/${weekTick.day}`;
  return new Intl.DateTimeFormat(getLanguage(), { timeZone: "UTC", month: model.view === "year" && span > 40 * 1440 ? "short" : "2-digit", ...(span > 40 * 1440 ? {} : { day: "2-digit" }), ...(span < 3 * 1440 ? { hour: "2-digit", minute: "2-digit", hour12: false } : {}) }).format(timelineDate(model, minute));
}

function ticksFor(model, range, width) {
  const span = range.end - range.start, count = Math.max(2, Math.floor(width / 110));
  if (model.view === "week" && range.start === 0 && range.end === model.rows[0].timeline.totalMinutes) return model.rows[0].timeline.ticks.filter((_, index) => index % Math.ceil(model.rows[0].timeline.ticks.length / count) === 0).map((tick) => tick.minute);
  if (model.view === "year" && span > 40 * 1440) {
    const ticks = [];
    for (let month = 0; month < 12; month++) { const minute = (Date.UTC(model.calendar.year, month, 1) - Date.UTC(model.calendar.year, 0, 1)) / 60000; if (minute >= range.start && minute <= range.end) ticks.push(minute); }
    return ticks.filter((_, index) => index % Math.ceil(ticks.length / count) === 0);
  }
  const step = [30, 60, 120, 180, 360, 720, 1440, 2880, 10080].find((value) => value >= span / count) || 10080;
  const ticks = [];
  for (let minute = Math.ceil(range.start / step) * step; minute <= range.end; minute += step) ticks.push(minute);
  return ticks;
}

function segmentIndex(segments, minute) {
  let low = 0, high = segments.length;
  while (low < high) { const middle = (low + high) >>> 1; if (segments[middle].endMinute <= minute) low = middle + 1; else high = middle; }
  return low;
}

// Preserve exact minute evidence in the model and hover. At year scale there
// can be more changes than screen pixels: draw each pixel's observed min/max,
// never an hourly average that could erase a short positive interval.
function plotSegments(segments, range, width) {
  const first = segmentIndex(segments, range.start), last = segmentIndex(segments, range.end - .0001) + 1;
  if (last - first <= width * 3) return segments.slice(first, last);
  const count = Math.max(1, Math.ceil(width)), span = range.end - range.start;
  const buckets = Array.from({ length: count }, (_, index) => ({ startMinute: range.start + index / count * span, endMinute: range.start + (index + 1) / count * span, minimum: Infinity, maximum: -Infinity, unknown: false }));
  for (let index = first; index < last; index++) {
    const segment = segments[index];
    const start = Math.max(0, Math.floor((segment.startMinute - range.start) / span * count));
    const end = Math.min(count - 1, Math.floor((Math.min(segment.endMinute, range.end) - range.start - .0001) / span * count));
    for (let pixel = start; pixel <= end; pixel++) {
      const bucket = buckets[pixel];
      if (segment.value === null) bucket.unknown = true;
      else { bucket.minimum = Math.min(bucket.minimum, segment.value); bucket.maximum = Math.max(bucket.maximum, segment.value); }
    }
  }
  return buckets.flatMap((bucket) => {
    const result = [];
    if (bucket.unknown) result.push({ ...bucket, value: null });
    if (bucket.minimum !== Infinity) result.push({ ...bucket, value: bucket.minimum === 0 && bucket.maximum === 0 ? 0 : bucket.maximum });
    return result;
  });
}

export function connectHVACAvailability(container) {
  const host = container?.querySelector("[data-hvac-timeline-id]"), model = host && models.get(host.dataset.hvacTimelineId);
  if (!host || !model) { models.clear(); return () => {}; }
  // String-rendered models only bridge render/connect. Keep the active model;
  // replaced equipment or calendar views must not retain dense annual traces.
  models.clear(); models.set(host.dataset.hvacTimelineId, model);
  if (!model.rows.length) return () => {};
  const plotHost = host.querySelector(".hvac-availability-timelines"), total = model.rows[0].timeline.totalMinutes;
  let range = { start: 0, end: total }, drag = null;
  const controller = new AbortController(), { signal } = controller;
  const hover = host.querySelector("[data-hvac-timeline-hover]");
  const draw = () => {
    host.dataset.timelineStart = range.start; host.dataset.timelineEnd = range.end; host.dataset.timelineTotal = total;
    for (const [index, { timeline }] of model.rows.entries()) {
      const svg = host.querySelector(`[data-hvac-timeline-row="${index}"] svg`), width = Math.max(160, svg.clientWidth), span = range.end - range.start;
      const x = (minute) => 44 + (minute - range.start) / span * (width - 58);
      const visible = plotSegments(timeline.segments, range, width - 58);
      const low = Math.min(0, timeline.minimum ?? 0), maximum = Math.max(0, timeline.maximum ?? 0);
      const high = maximum > 0 ? maximum : low < 0 ? 0 : 1;
      const y = (value) => 72 - (value - low) / (high - low) * 58, pattern = `${host.dataset.hvacTimelineId}-unknown-${index}`;
      const ticks = ticksFor(model, range, width - 58);
      let path = "", previous = null;
      for (const segment of visible) {
        if (segment.value === null) { previous = null; continue; }
        const start = x(Math.max(segment.startMinute, range.start)), end = x(Math.min(segment.endMinute, range.end));
        if (segment.minimum !== undefined) {
          path += `M${(start + end) / 2},${y(segment.minimum)} V${y(segment.maximum)} `; previous = null; continue;
        }
        path += `${previous ? "L" : "M"}${start},${y(segment.value)} H${end} `; previous = segment;
      }
      svg.setAttribute("viewBox", `0 0 ${width} 112`);
      svg.dataset.renderedSegments = visible.length; svg.dataset.sourceSegments = timeline.segments.length;
      svg.innerHTML = `<defs><pattern id="${pattern}" width="7" height="7" patternUnits="userSpaceOnUse"><path d="M0,7 L7,0" stroke="#d2a867" stroke-opacity=".5"/></pattern></defs>${visible.filter((segment) => segment.value === 0 || segment.value === null).map((segment) => `<rect class="${segment.value === null ? "unknown" : "zero"}" x="${x(Math.max(segment.startMinute, range.start))}" y="10" width="${Math.max(.3, x(Math.min(segment.endMinute, range.end)) - x(Math.max(segment.startMinute, range.start)))}" height="66"${segment.value === null ? ` fill="url(#${pattern})"` : ""}/>`).join("")}${ticks.map((minute) => `<line class="hvac-timeline-grid" x1="${x(minute)}" x2="${x(minute)}" y1="10" y2="76"/><text x="${x(minute)}" y="100" text-anchor="middle">${safe(timeLabel(model, minute, span))}</text>`).join("")}<line class="hvac-timeline-baseline" x1="44" x2="${width - 14}" y1="${y(0)}" y2="${y(0)}"/><path class="hvac-timeline-curve" d="${path}"/><text x="35" y="17" text-anchor="end">${safe(number(high))}</text><text x="35" y="76" text-anchor="end">${safe(number(low))}</text>${model.view === "day" ? visible.map((segment) => `<rect data-schedule-value="${segment.value}" data-start-minute="${segment.startMinute}" data-end-minute="${segment.endMinute}" x="${x(Math.max(segment.startMinute, range.start))}" y="10" width="${Math.max(.3, x(Math.min(segment.endMinute, range.end)) - x(Math.max(segment.startMinute, range.start)))}" height="66" fill="transparent"/>`).join("") : ""}`;
    }
  };
  const setRange = (start, span) => { span = Math.max(Math.min(60, total), Math.min(total, span)); start = Math.max(0, Math.min(total - span, start)); range = { start, end: start + span }; draw(); };
  const ratioAt = (event, svg) => Math.max(0, Math.min(1, (event.clientX - svg.getBoundingClientRect().left - 44) / Math.max(1, svg.clientWidth - 58)));
  plotHost.addEventListener("wheel", (event) => { const svg = event.target.closest("svg"); if (!svg) return; event.preventDefault(); const span = range.end - range.start, ratio = ratioAt(event, svg), nextSpan = span * Math.exp(Math.max(-1, Math.min(1, event.deltaY / 450))); setRange(range.start + span * ratio - nextSpan * ratio, nextSpan); }, { passive: false, signal });
  plotHost.addEventListener("pointerdown", (event) => { const svg = event.target.closest("svg"); if (!svg || event.button !== 0) return; drag = { x: event.clientX, start: range.start, span: range.end - range.start, width: svg.clientWidth - 58 }; if (event.isTrusted) plotHost.setPointerCapture(event.pointerId); plotHost.classList.add("dragging"); }, { signal });
  plotHost.addEventListener("pointermove", (event) => {
    if (drag) { setRange(drag.start - (event.clientX - drag.x) / Math.max(1, drag.width) * drag.span, drag.span); return; }
    const row = event.target.closest("[data-hvac-timeline-row]"), svg = row?.querySelector("svg"); if (!svg) return;
    const index = Number(row.dataset.hvacTimelineRow), minute = Math.min(total - 1, range.start + ratioAt(event, svg) * (range.end - range.start));
    const source = model.rows[index].timeline.segments, segment = source[segmentIndex(source, minute)];
    hover.textContent = `${row.querySelector("strong").textContent} · ${timeLabel(model, minute, 0)} · ${segment?.value == null ? copy("unknownScheduleValue", "Unavailable") : number(segment.value)}`;
  }, { signal });
  const stop = () => { drag = null; plotHost.classList.remove("dragging"); };
  plotHost.addEventListener("pointerup", stop, { signal }); plotHost.addEventListener("pointercancel", stop, { signal });
  plotHost.addEventListener("keydown", (event) => { const span = range.end - range.start; if (["ArrowLeft", "ArrowRight", "+", "=", "-", "Home", "0"].includes(event.key)) event.preventDefault(); else return; if (event.key === "Home" || event.key === "0") setRange(0, total); else if (event.key === "ArrowLeft" || event.key === "ArrowRight") setRange(range.start + span * (event.key === "ArrowLeft" ? -.2 : .2), span); else { const next = span * (event.key === "-" ? 1.5 : 1 / 1.5); setRange(range.start + (span - next) / 2, next); } }, { signal });
  host.querySelector("[data-hvac-timeline-reset]").addEventListener("click", () => setRange(0, total), { signal });
  const observer = typeof ResizeObserver !== "undefined" ? new ResizeObserver(draw) : null;
  observer?.observe(plotHost); draw();
  return () => { controller.abort(); observer?.disconnect(); };
}
