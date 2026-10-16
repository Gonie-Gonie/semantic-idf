// Static operating permissions. Unknown schedules and runtime decisions stay
// unknown; these curves never claim that equipment actually ran.
const MINUTES = 1440;
const dayTypes = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday", "Holiday", "SummerDesignDay", "WinterDesignDay", "CustomDay1", "CustomDay2"];
const normalized = (value) => String(value || "").trim().toLowerCase();
const filled = (value) => Array(MINUTES).fill(value);
const finite = (value) => typeof value === "number" && Number.isFinite(value);

export const hvacOperationDayTypes = Object.freeze(dayTypes);

export function normalizeHVACOperationCalendar(options = {}) {
  const year = Math.max(1900, Math.min(2199, Math.trunc(Number(options.year) || new Date().getFullYear())));
  const month = Math.max(1, Math.min(12, Math.trunc(Number(options.month) || 1)));
  const maximum = new Date(Date.UTC(year, month, 0)).getUTCDate();
  const day = Math.max(1, Math.min(maximum, Math.trunc(Number(options.day) || 1)));
  const dayType = dayTypes.find((item) => normalized(item) === normalized(options.dayType)) || "Monday";
  const timelineView = ["day", "week", "year"].includes(options.timelineView) ? options.timelineView : "day";
  return { year, month, day, dayType, timelineView };
}

// Resolve a schedule's own values without transforming them into equipment
// permissions. The caller can share this resolver across source rows for a day.
export function createHVACScheduleDayResolver(schedules = [], options = {}) {
  return scheduleResolver(schedules, normalizeHVACOperationCalendar(options));
}

function scheduleNamespace(schedule) {
  const type = normalized(schedule.objectType);
  return type.startsWith("schedule:day:") || schedule.kind === "day" ? "day" : type.startsWith("schedule:week:") || schedule.kind === "week" ? "week" : "annual";
}

function matchesDay(selector, dayType) {
  const tokens = normalized(selector).split(/[\s,;]+/), day = normalized(dayType);
  return tokens.includes(day) || tokens.includes("alldays") ||
    tokens.includes("weekdays") && ["monday", "tuesday", "wednesday", "thursday", "friday"].includes(day) ||
    tokens.includes("weekends") && ["saturday", "sunday"].includes(day);
}

function scheduleResolver(schedules, calendar) {
  const dayOfYear = Math.round((Date.UTC(2000, calendar.month - 1, calendar.day) - Date.UTC(2000, 0, 1)) / 86400000) + 1;
  const index = new Map(), cache = new Map(), pending = new Set();
  for (const schedule of schedules || []) {
    const key = `${scheduleNamespace(schedule)}:${normalized(schedule.name)}`;
    // Duplicate definitions are ambiguous, including equal-looking duplicates.
    index.set(key, index.has(key) ? null : schedule);
  }
  const intervals = (items) => {
    if (!Array.isArray(items) || !items.length) return filled(null);
    const values = [], valid = items.every((item) => {
      if (!Number.isInteger(item.untilMinute) || item.untilMinute <= values.length || item.untilMinute > MINUTES || !finite(item.value)) return false;
      while (values.length < item.untilMinute) values.push(item.value);
      return true;
    });
    return valid && values.length === MINUTES ? values : filled(null);
  };
  const resolve = (name, namespace = "annual") => {
    const key = `${namespace}:${normalized(name)}`;
    if (cache.has(key)) return cache.get(key);
    if (pending.has(key) || pending.size > 32) return filled(null);
    const schedule = index.get(key);
    if (!schedule || schedule.status !== "supported") return filled(null);
    if (schedule.interpolation && !["no", "none"].includes(normalized(schedule.interpolation))) return filled(null);
    pending.add(key);
    let values = filled(null);
    if (schedule.kind === "constant" && finite(schedule.value)) values = filled(schedule.value);
    else if (schedule.kind === "day") values = intervals(schedule.intervals);
    else if (schedule.kind === "compact" || schedule.kind === "week" && schedule.rules?.length) {
      const candidates = (schedule.rules || []).filter((rule) => (!rule.startDay || rule.startDay <= dayOfYear) && (!rule.endDay || rule.endDay >= dayOfYear));
      const rule = candidates.find((item) => matchesDay(item.selector, calendar.dayType)) || candidates.find((item) => normalized(item.selector).split(/[\s,;]+/).includes("allotherdays"));
      if (rule) values = rule.scheduleName ? resolve(rule.scheduleName, "day") : intervals(rule.intervals);
    } else if (schedule.kind === "week") {
      const entry = Object.entries(schedule.weekDays || {}).find(([day]) => normalized(day) === normalized(calendar.dayType));
      if (entry) values = resolve(entry[1], "day");
    } else if (schedule.kind === "year") {
      const periods = (schedule.periods || []).filter((period) => period.startDay <= dayOfYear && period.endDay >= dayOfYear);
      if (periods.length === 1) values = resolve(periods[0].scheduleName, "week");
    }
    pending.delete(key);
    cache.set(key, values);
    return values;
  };
  return resolve;
}

function statuses(mask) {
  return [0, 1, 2, 3].filter((status) => mask & (1 << status));
}

function mergeManagerMasks(left, right, first) {
  // Air/zone priority: ForceOff > CycleOnZoneFansOnly > CycleOn > NoAction.
  const rank = [0, 3, 1, 2];
  let result = 0;
  for (const a of statuses(left)) for (const b of statuses(right)) {
    const status = first ? a === 0 ? b : a : rank[a] >= rank[b] ? a : b;
    result |= 1 << status;
  }
  return result;
}

function consensus(values) {
  return values.every((value) => value === 0) ? 0 : values.every((value) => value === 1) ? 1 : null;
}

function managerPermission(status, target, own = 1) {
  if (status === 1) return 0;
  if (status === 2) return 1;
  if (status === 3) return target === "zone_fan" ? 1 : 0;
  return own;
}

function expressionEvaluator(resolve) {
  const gateCache = new WeakMap(), managerCache = new WeakMap();
  const managers = (expression, depth = 0) => {
    if (!expression || depth > 64) return filled(15);
    if (managerCache.has(expression)) return managerCache.get(expression);
    let result = filled(15);
    const op = String(expression.op || "unknown"), children = expression.children || [];
    if (op === "always") result = filled(1 << (Number.isInteger(expression.value) && expression.value >= 0 && expression.value <= 3 ? expression.value : 0));
    else if (["manager_scheduled", "manager_scheduled_on", "manager_scheduled_off"].includes(op)) {
      result = resolve(expression.scheduleName).map((value) => {
        if (value === null) return op === "manager_scheduled" ? 6 : op === "manager_scheduled_on" ? 5 : 3;
        return 1 << (op === "manager_scheduled" ? value > 0 ? 2 : 1 : op === "manager_scheduled_on" ? value > 0 ? 2 : 0 : value === 0 ? 1 : 0);
      });
    } else if (op === "air_managers" || op === "plant_managers") {
      result = filled(1);
      for (const child of children) {
        const next = managers(child, depth + 1);
        result = result.map((mask, minute) => mergeManagerMasks(mask, next[minute], op === "plant_managers"));
      }
    }
    managerCache.set(expression, result);
    return result;
  };
  const gates = (expression, depth = 0) => {
    if (!expression || depth > 64) return filled(null);
    if (gateCache.has(expression)) return gateCache.get(expression);
    const op = String(expression.op || "unknown"), children = expression.children || [];
    let result = filled(null);
    if (op === "always") result = filled(expression.value === 0 ? 0 : 1);
    else if (op === "schedule_positive") result = resolve(expression.scheduleName).map((value) => value === null ? null : value > 0 ? 1 : 0);
    else if (op === "all" || op === "any") {
      const inputs = children.map((child) => gates(child, depth + 1));
      result = Array.from({ length: MINUTES }, (_, minute) => {
        const values = inputs.map((input) => input[minute]);
        if (op === "all") return values.includes(0) ? 0 : values.includes(null) ? null : 1;
        return values.includes(1) ? 1 : values.includes(null) ? null : 0;
      });
    } else if (op === "manager_gate" || op === "fan_gate") {
      const masks = managers(children[0], depth + 1), own = op === "fan_gate" ? gates(children[1], depth + 1) : filled(1);
      result = masks.map((mask, minute) => consensus(statuses(mask).map((status) => managerPermission(status, expression.target, own[minute]))));
    } else if (op.startsWith("manager_") || op === "air_managers" || op === "plant_managers") {
      result = managers(expression, depth + 1).map((mask) => consensus(statuses(mask).map((status) => managerPermission(status, expression.target))));
    }
    gateCache.set(expression, result);
    return result;
  };
  return gates;
}

// The stacked availability view uses the same native manager precedence and
// overrides as the day inspector, with one shared schedule resolver per day.
export function createHVACAvailabilityDayEvaluator(schedules = [], options = {}) {
  return expressionEvaluator(createHVACScheduleDayResolver(schedules, options));
}

export function hvacAvailabilitySegments(values) {
  const segments = [];
  for (let minute = 0; minute < values.length; minute++) {
    const previous = segments[segments.length - 1], value = values[minute];
    if (previous && previous.value === value) previous.endMinute = minute + 1;
    else segments.push({ startMinute: minute, endMinute: minute + 1, value });
  }
  return segments;
}

export function evaluateHVACOperation(profile = {}, schedules = [], options = {}) {
  const calendar = normalizeHVACOperationCalendar(options);
  const resolve = scheduleResolver(schedules, calendar);
  const evaluate = expressionEvaluator(resolve);
  const trace = (expression) => {
    const values = evaluate(expression);
    return { values, segments: hvacAvailabilitySegments(values) };
  };
  const graph = (sequence = {}) => ({
    ...sequence,
    nodes: (sequence.nodes || []).map((node) => node.kind === "schedule" ? {
      ...node,
      // A definition trace retains the schedule's raw values. It is not a
      // permission, manager status, or inferred equipment operating state.
      scheduleTrace: hvacAvailabilitySegments(resolve(
        node.parameters?.find((item) => item.name === "Schedule Name")?.value || node.source?.objectName,
        scheduleNamespace({ objectType: node.source?.objectType }),
      )),
    } : node),
    edges: sequence.edges || [],
  });
  return {
    calendar,
    availabilityDefinitions: graph(profile.availabilityDefinitions),
    onOff: graph(profile.onOff),
    parameterControl: graph(profile.parameterControl),
    layers: (profile.availabilityLayers || []).map((layer) => ({ ...layer, ...trace(layer.expression) })),
    final: trace(profile.finalAvailability),
    control: graph(profile.control),
  };
}
