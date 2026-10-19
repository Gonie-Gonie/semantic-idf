import { escapeHTML } from "../state.js";
import { getLanguage, t } from "../i18n.js";
import { buildHVACAvailabilityStack } from "../hvac-availability-timeline.js";

const safe = escapeHTML;
const copy = (key, fallback) => t(`hvac.operation.${key}`, {}, fallback);
const models = new Map();
let nextID = 0;

function rowName(row) {
  if (row.kind === "final") return copy("finalAvailability", "Configured availability");
  if (row.name) return row.name;
  if (row.labelCode) return copy(row.labelCode, "Availability unresolved");
  return row.source?.objectName || copy("unresolvedAvailability", "Availability unresolved");
}

function ownerLabel(row) {
  const owners = row.owners || [];
  return owners.map((owner) => owner.objectName || owner.objectType).filter(Boolean).join(" · ") || copy("ownerUnresolved", "Owner unresolved");
}

function renderOwners(row) {
  const owners = (row.owners || []).filter((owner) => owner.objectName || owner.objectType);
  return `<span class="hvac-availability-owners" title="${safe(ownerLabel(row))}">${owners.length ? owners.map((owner) => `<span data-hvac-availability-owner data-object-index="${safe(owner.objectIndex ?? "")}" data-object-type="${safe(owner.objectType || "")}" title="${safe([owner.objectType, owner.objectName].filter(Boolean).join(" · "))}">${safe(owner.objectName || owner.objectType)}</span>`).join("") : safe(copy("ownerUnresolved", "Owner unresolved"))}</span>`;
}

function statusLabel(row, value) {
  const key = value === null ? "unknown" : row.kind === "final" ? value === 1 ? "available" : "unavailable" : value === 1 ? "on" : "off";
  return copy(`status.${key}`, { unknown: "Unknown", available: "Available", unavailable: "Unavailable", on: "ON", off: "OFF" }[key]);
}

export function renderHVACAvailability(profile, schedules, calendar) {
  const model = buildHVACAvailabilityStack(profile || {}, schedules, calendar, { view: calendar.timelineView || "day" });
  const id = `hvac-availability-${++nextID}`;
  models.set(id, model);
  while (models.size > 2) models.delete(models.keys().next().value);
  return `<div class="hvac-availability-graph" data-hvac-operation-graph="availability" data-hvac-timeline-id="${id}" role="group" aria-label="${safe(copy("availability", "Availability"))}">
    <div class="hvac-availability-toolbar"><select data-hvac-operation-calendar="timelineView" aria-label="${safe(copy("timelineView", "View"))}">${[["day", "representativeDay", "Representative day"], ["week", "representativeWeek", "Representative week"], ["year", "annual", "Annual"]].map(([value, key, label]) => `<option value="${value}"${value === model.view ? " selected" : ""}>${safe(copy(key, label))}</option>`).join("")}</select><span data-hvac-timeline-hover aria-live="polite"></span></div>
    <div class="hvac-availability-timelines" tabindex="0" role="group" aria-label="${safe(copy("availability", "Availability"))}" aria-keyshortcuts="ArrowLeft ArrowRight + - Home">${model.rows.map((row, index) => `<article class="hvac-availability-timeline-row ${row.kind === "final" ? "final" : ""}" data-hvac-timeline-row="${index}" data-hvac-availability-row="${safe(row.id)}" data-hvac-availability-kind="${safe(row.kind)}"${row.kind === "final" && model.requiresReanalysis ? ` title="${safe(copy("reanalyzeAvailability", "Analyze the input again to resolve configured availability."))}"` : ""}><span class="hvac-availability-labels">${renderOwners(row)}<strong class="hvac-availability-name" title="${safe(rowName(row))}">${safe(rowName(row))}</strong></span><svg class="hvac-availability-timeline-plot" data-hvac-schedule-plot role="img" aria-label="${safe(`${ownerLabel(row)} · ${rowName(row)}`)}" viewBox="0 0 800 28"></svg><span class="hvac-availability-row-status" data-hvac-row-status></span></article>`).join("")}
      <div class="hvac-availability-axis-row"><span class="hvac-availability-axis-label"></span><svg class="hvac-availability-time-axis" aria-hidden="true" viewBox="0 0 800 26"></svg><span></span></div>
    </div>
  </div>`;
}

function timeLabel(model, minute, span) {
  const whole = Math.floor(minute);
  if (model.view === "day") return `${String(Math.floor(whole / 60)).padStart(2, "0")}:${String(whole % 60).padStart(2, "0")}`;
  const tick = model.view === "week" && span > 3 * 1440 ? model.ticks.find((item) => item.minute === minute) : null;
  if (tick?.dayType) return `${copy(`dayType.${tick.dayType}`, tick.dayType)} ${tick.month}/${tick.day}`;
  const start = model.startDate, date = new Date(Date.UTC(start.year, start.month - 1, start.day) + whole * 60000);
  return new Intl.DateTimeFormat(getLanguage(), { timeZone: "UTC", month: model.view === "year" && span > 40 * 1440 ? "short" : "2-digit", ...(span > 40 * 1440 ? {} : { day: "2-digit" }), ...(span < 3 * 1440 ? { hour: "2-digit", minute: "2-digit", hour12: false } : {}) }).format(date);
}

function ticksFor(model, range, width) {
  const span = range.end - range.start, count = Math.max(2, Math.floor(width / 110));
  if (range.start === 0 && range.end === model.totalMinutes) return model.ticks.filter((_, index) => index % Math.ceil(model.ticks.length / count) === 0).map((tick) => tick.minute);
  const step = [30, 60, 120, 180, 360, 720, 1440, 2880, 10080, 43200].find((value) => value >= span / count) || 43200;
  const ticks = [];
  for (let minute = Math.ceil(range.start / step) * step; minute <= range.end; minute += step) ticks.push(minute);
  return ticks;
}

function segmentIndex(segments, minute) {
  let low = 0, high = segments.length;
  while (low < high) { const middle = (low + high) >>> 1; if (segments[middle].endMinute <= minute) low = middle + 1; else high = middle; }
  return low;
}

// Screen pixels can contain several exact native intervals. Preserve every
// observed status in their patch rather than choosing an average or majority.
function plotPatches(segments, range, width) {
  const first = segmentIndex(segments, range.start), last = segmentIndex(segments, range.end - .0001) + 1;
  const mask = (value) => value === null ? 4 : value === 1 ? 2 : 1;
  if (last - first <= width * 2) return segments.slice(first, last).map((segment) => ({ ...segment, mask: mask(segment.value) }));
  const count = Math.max(1, Math.ceil(width)), span = range.end - range.start;
  const buckets = Array.from({ length: count }, (_, index) => ({ startMinute: range.start + index / count * span, endMinute: range.start + (index + 1) / count * span, mask: 0 }));
  for (let index = first; index < last; index++) {
    const segment = segments[index], begin = Math.max(0, Math.floor((segment.startMinute - range.start) / span * count));
    const end = Math.min(count - 1, Math.floor((Math.min(segment.endMinute, range.end) - range.start - .0001) / span * count));
    for (let pixel = begin; pixel <= end; pixel++) buckets[pixel].mask |= mask(segment.value);
  }
  return buckets;
}

export function connectHVACAvailability(container) {
  const host = container?.querySelector("[data-hvac-timeline-id]"), model = host && models.get(host.dataset.hvacTimelineId);
  if (!host || !model) { models.clear(); return () => {}; }
  models.clear(); models.set(host.dataset.hvacTimelineId, model);
  const plotHost = host.querySelector(".hvac-availability-timelines"), total = model.totalMinutes;
  let range = { start: 0, end: total }, drag = null, hoverMinute = null;
  const controller = new AbortController(), { signal } = controller;
  const paintHover = (minute) => {
    hoverMinute = minute;
    const visible = minute !== null && minute >= range.start && minute < range.end;
    host.querySelector("[data-hvac-timeline-hover]").textContent = visible ? timeLabel(model, minute, 0) : "";
    for (const [index, row] of model.rows.entries()) {
      const element = host.querySelector(`[data-hvac-timeline-row="${index}"]`), svg = element.querySelector("svg"), line = svg.querySelector("[data-hvac-crosshair]"), status = element.querySelector("[data-hvac-row-status]");
      line.setAttribute("visibility", visible ? "visible" : "hidden");
      if (!visible) { status.textContent = ""; element.removeAttribute("data-hvac-hover-value"); continue; }
      const x = 2 + (minute - range.start) / (range.end - range.start) * (svg.clientWidth - 4);
      line.setAttribute("x1", x); line.setAttribute("x2", x);
      const segment = row.timeline.segments[segmentIndex(row.timeline.segments, minute)], value = segment?.value ?? null;
      element.dataset.hvacHoverValue = value === null ? "unknown" : String(value);
      status.textContent = statusLabel(row, value);
    }
  };
  const draw = () => {
    host.dataset.timelineStart = range.start; host.dataset.timelineEnd = range.end; host.dataset.timelineTotal = total;
    for (const [index, row] of model.rows.entries()) {
      const svg = host.querySelector(`[data-hvac-timeline-row="${index}"] svg`), width = Math.max(50, svg.clientWidth), span = range.end - range.start;
      const x = (minute) => 2 + (minute - range.start) / span * (width - 4), patches = plotPatches(row.timeline.segments, range, width - 4);
      const pattern = `${host.dataset.hvacTimelineId}-unknown-${index}`;
      svg.setAttribute("viewBox", `0 0 ${width} 28`); svg.dataset.renderedSegments = patches.length; svg.dataset.sourceSegments = row.timeline.segments.length;
      svg.innerHTML = `<defs><pattern id="${pattern}" width="6" height="6" patternUnits="userSpaceOnUse"><path d="M0,6 L6,0"/></pattern></defs>${patches.map((patch) => {
        const start = x(Math.max(patch.startMinute, range.start)), patchWidth = Math.max(.4, x(Math.min(patch.endMinute, range.end)) - start);
        const status = patch.mask === 2 ? "on" : patch.mask === 1 ? "off" : patch.mask === 4 ? "unknown" : "mixed";
        return `<g data-hvac-patch-status="${status}" data-start-minute="${patch.startMinute}" data-end-minute="${patch.endMinute}" data-has-on="${Boolean(patch.mask & 2)}"><rect class="hvac-availability-patch ${patch.mask & 2 ? "on" : "off"}" x="${start}" y="6" width="${patchWidth}" height="16"/>${patch.mask & 1 && patch.mask & 2 ? `<rect class="hvac-availability-patch off" x="${start}" y="14" width="${patchWidth}" height="8"/>` : ""}${patch.mask & 4 ? `<rect class="hvac-availability-patch unknown" x="${start}" y="6" width="${patchWidth}" height="16" fill="url(#${pattern})"/>` : ""}</g>`;
      }).join("")}<line class="hvac-availability-crosshair" data-hvac-crosshair x1="0" x2="0" y1="1" y2="27" visibility="hidden"/>`;
    }
    const axis = host.querySelector(".hvac-availability-time-axis"), width = Math.max(50, axis.clientWidth), span = range.end - range.start;
    axis.setAttribute("viewBox", `0 0 ${width} 26`);
    axis.innerHTML = ticksFor(model, range, width).map((minute) => { const x = 2 + (minute - range.start) / span * (width - 4); return `<text x="${x}" y="17" text-anchor="${minute === range.start ? "start" : minute === range.end ? "end" : "middle"}">${safe(timeLabel(model, minute, span))}</text>`; }).join("");
    paintHover(hoverMinute);
  };
  const setRange = (start, span) => { span = Math.max(Math.min(60, total), Math.min(total, span)); start = Math.max(0, Math.min(total - span, start)); range = { start, end: start + span }; draw(); };
  const ratioAt = (event, svg) => Math.max(0, Math.min(1, (event.clientX - svg.getBoundingClientRect().left - 2) / Math.max(1, svg.clientWidth - 4)));
  plotHost.addEventListener("wheel", (event) => { const svg = event.target.closest(".hvac-availability-timeline-plot"); if (!svg) return; event.preventDefault(); const span = range.end - range.start, ratio = ratioAt(event, svg), minute = range.start + span * ratio, next = span * Math.exp(Math.max(-1, Math.min(1, event.deltaY / 450))); setRange(minute - next * ratio, next); paintHover(Math.min(total - .001, minute)); }, { passive: false, signal });
  plotHost.addEventListener("pointerdown", (event) => { const svg = event.target.closest(".hvac-availability-timeline-plot"); if (!svg || event.button !== 0) return; drag = { svg, x: event.clientX, start: range.start, span: range.end - range.start, width: svg.clientWidth - 4 }; if (event.isTrusted) plotHost.setPointerCapture(event.pointerId); plotHost.classList.add("dragging"); }, { signal });
  plotHost.addEventListener("pointermove", (event) => {
    const svg = drag?.svg || event.target.closest(".hvac-availability-timeline-plot"); if (!svg) return;
    if (drag) setRange(drag.start - (event.clientX - drag.x) / Math.max(1, drag.width) * drag.span, drag.span);
    paintHover(Math.min(total - .001, range.start + ratioAt(event, svg) * (range.end - range.start)));
  }, { signal });
  const stop = () => { drag = null; plotHost.classList.remove("dragging"); };
  plotHost.addEventListener("pointerup", stop, { signal }); plotHost.addEventListener("pointercancel", stop, { signal });
  plotHost.addEventListener("pointerleave", () => { if (!drag) paintHover(null); }, { signal });
  plotHost.addEventListener("keydown", (event) => { const span = range.end - range.start; if (["ArrowLeft", "ArrowRight", "+", "=", "-", "Home", "0"].includes(event.key)) event.preventDefault(); else return; if (event.key === "Home" || event.key === "0") setRange(0, total); else if (event.key === "ArrowLeft" || event.key === "ArrowRight") setRange(range.start + span * (event.key === "ArrowLeft" ? -.2 : .2), span); else { const next = span * (event.key === "-" ? 1.5 : 1 / 1.5); setRange(range.start + (span - next) / 2, next); } }, { signal });
  const observer = typeof ResizeObserver !== "undefined" ? new ResizeObserver(draw) : null;
  observer?.observe(plotHost); draw();
  return () => { controller.abort(); observer?.disconnect(); };
}
