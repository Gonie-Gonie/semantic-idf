import { energyPathNodeAppearance } from "./energy-path-appearance.js";

const token = (value) => String(value ?? "").trim().toLowerCase();
const list = (value) => Array.isArray(value) ? value : [];
const unique = (values) => [...new Set(values.filter((value) => typeof value === "string" && value.trim()))].sort();
const number = (value) => (typeof value === "number" || typeof value === "string" && value.trim()) && Number.isFinite(Number(value)) ? Number(value) : null;
const field = (item, key) => item && Object.hasOwn(item, key) ? number(item[key]) : null;
const sum = (values) => values.length && values.every((value) => value !== null) && Number.isFinite(values.reduce((total, value) => total + value, 0)) ? values.reduce((total, value) => total + value, 0) : null;
const close = (a, b) => a !== null && b !== null && Math.abs(a - b) <= 1e-6 * Math.max(Number.MIN_VALUE, Math.abs(a), Math.abs(b));
const row = (key, value, unit = "", extra = {}) => ({ key, value, unit, ...extra });
const service = (item) => ["cooling", "heating"].includes(token(item?.serviceKind)) ? token(item.serviceKind) : "";
const periodOK = (item, period, context = "") => token(item?.period) ? token(item.period) === period : period === "annual" || context === period;
const unitOK = (unit, domain) => ["kwh", `kwh ${domain}`].includes(token(unit).replace(/[()[\]_]/g, " ").replace(/\s+/g, " "));
const displayUnit = (item = {}) => unitOK(item.unit, item.scaleDomain) && ["thermal", "site"].includes(token(item.scaleDomain)) ? `kWh ${token(item.scaleDomain)}` : String(item.unit || "");
const category = (item) => token(item?.driverCategory) === "internal.other" ? "balance.storage_other" : token(item?.driverCategory);
const END_USE_ALIASES = { fans: "fans_pumps", pumps: "fans_pumps", heat_rejection: "hvac_auxiliaries", humidification: "hvac_auxiliaries", heat_recovery: "hvac_auxiliaries" };
const endUse = (item) => END_USE_ALIASES[token(item?.endUse)] || token(item?.endUse);
const CARRIER_ALIASES = {
  electricity: "electricity", gas: "natural_gas", naturalgas: "natural_gas", districtcooling: "district_cooling", districtheating: "district_heating", districtheatingwater: "district_heating", districtheatingsteam: "steam", steam: "steam",
  propane: "propane", fueloil1: "fuel_oil_1", fueloilno1: "fuel_oil_1", fueloil2: "fuel_oil_2", fueloilno2: "fuel_oil_2", coal: "coal", diesel: "diesel", gasoline: "gasoline", otherfuel1: "other_fuel_1", otherfuel2: "other_fuel_2", water: "water",
};
const carrier = (item) => CARRIER_ALIASES[token(item?.carrier).replace(/[^a-z0-9]/g, "")] || "";
const label = (item) => item?.label && item.label !== item.id ? String(item.label) : "";
const BASIS_EXPLANATIONS = {
  reported_meter: "Reported energy-meter value.", reported_variable: "Reported simulation-variable value.", reported_end_use_subtotal: "Subtotal of observed end uses, not a complete facility total.",
  direct_zone_energy: "Directly observed energy for this zone.", integrated_rate: "Reported rates integrated over time.", heat_balance_share: "Allocated using matching heat-balance pressure shares; not a causal decomposition.",
  service_path_allocation: "Allocated using the related HVAC service path.", zone_load_allocation: "Allocated using zone load shares.", allocated: "An explicitly allocated contribution.",
  derived_ratio: "Derived from explicitly linked reported values.", residual: "Unclassified difference between the reported source total and classified uses.", grouped_presentation: "Combined original contributions; their individual accounting bases remain available in Sources.",
};

function members(item, ancestors = new Set()) {
  if (!item || typeof item !== "object" || ancestors.has(item)) return [null];
  const children = list(item.groupedMembers);
  if (!children.length) return [item];
  const path = new Set(ancestors).add(item);
  return children.flatMap((child) => members(child, path));
}

function accountingField(item, key, period, context) {
  const originals = members(item);
  const valueOf = (member) => {
    const value = field(member, key);
    if (value === null || member?.level !== "driver" || !["rawValue", "effectiveValue"].includes(key) || value < 0) return value;
    return field(member, "signedValue") < 0 || token(member.sign) === "negative" ? -value : value;
  };
  return list(item.groupedMembers).length ? sum(originals.map((member) => member && periodOK(member, period, context) && token(member.scaleDomain) === token(item.scaleDomain) && unitOK(member.unit, token(item.scaleDomain)) ? valueOf(member) : null)) : valueOf(item);
}

function multiplier(item, period, context) {
  const factors = members(item).map((member) => {
    if (!member || !periodOK(member, period, context) || token(member.scaleDomain) !== token(item.scaleDomain) || !unitOK(member.unit, token(item.scaleDomain))) return null;
    if (Object.hasOwn(member, "multiplier")) {
      const explicit = field(member, "multiplier");
      return explicit !== null && explicit > 0 ? explicit : null;
    }
    const raw = field(member, "rawValue"), effective = field(member, "effectiveValue");
    if (raw === null || effective === null || raw === 0 || effective === 0) return null;
    const factor = effective / raw;
    return Number.isFinite(factor) && factor > 0 ? factor : null;
  });
  return factors.length && factors[0] !== null && factors.every((factor) => close(factor, factors[0])) ? factors[0] : null;
}

function driverDirection(item, key) {
  if (item.level !== "driver") return "";
  const states = members(item).map((member) => {
    const value = field(member, key);
    return value === null ? "unknown" : value === 0 ? "zero" : value < 0 || field(member, "signedValue") !== null && field(member, "signedValue") !== 0 || ["positive", "negative"].includes(token(member?.sign)) ? "signed" : "magnitude";
  }).filter((state) => state !== "zero");
  return states.every((state) => state === "signed") ? "signed" : states.every((state) => state === "magnitude") ? "magnitude" : "unknown";
}

function nodeIdentity(node) {
  return JSON.stringify([node?.level, node?.driverCategory, node?.thermalComponent, node?.serviceKind, node?.endUse, node?.carrier, node?.unit, node?.scaleDomain, node?.period, node?.zoneName,
    node?.value, node?.rawValue, node?.effectiveValue, node?.allocatedValue, node?.loadBreakdown, unique(list(node?.sourceIds))]);
}

function uniqueNodes(nodes) {
  const byID = new Map();
  for (const node of nodes) if (node?.id) {
    if (!byID.has(node.id)) byID.set(node.id, node);
    else if (byID.get(node.id) && nodeIdentity(byID.get(node.id)) !== nodeIdentity(node)) byID.set(node.id, null);
  }
  return byID;
}

function evidence(items) {
  const sourceIds = [], ruleIds = [], relatedEntityIds = [], relatedPathIds = [];
  const seen = new Set();
  const visit = (item) => {
    if (!item || typeof item !== "object" || seen.has(item)) return;
    seen.add(item);
    sourceIds.push(...list(item.sourceIds)); ruleIds.push(item.ruleId);
    relatedEntityIds.push(...list(item.relatedEntityIds)); relatedPathIds.push(...list(item.relatedPathIds));
    for (const child of [...list(item.groupedMembers), ...list(item.loadBreakdown), ...list(item.offsetEffects), item.simultaneousLoad]) visit(child);
  };
  items.forEach(visit);
  return { sourceIds: unique(sourceIds), ruleIds: unique(ruleIds), relatedEntityIds: unique(relatedEntityIds), relatedPathIds: unique(relatedPathIds) };
}

function physicalLinks(nodes, links, period, context) {
  const byID = uniqueNodes(nodes);
  const groups = new Map();
  for (const link of links) if (link?.id) groups.set(link.id, [...(groups.get(link.id) || []), link]);
  const result = [];
  for (const candidates of groups.values()) {
    const link = candidates[0];
    const identity = (item) => JSON.stringify([item.fromId, item.toId, item.relation, item.fromValue, item.toValue, item.fromUnit, item.toUnit, item.period, item.zoneName, item.serviceKind, unique(list(item.sourceIds))]);
    if (candidates.some((item) => identity(item) !== identity(link))) continue;
    const from = byID.get(link.fromId), to = byID.get(link.toId);
    if (!from || !to || ![link, from, to].every((item) => periodOK(item, period, context || ([link, from, to].some((item) => token(item.period) === period) ? period : "")))) continue;
    if (new Set([link, from, to].map((item) => token(item.zoneName)).filter(Boolean)).size > 1) continue;
    const fromValue = field(link, "fromValue"), toValue = field(link, "toValue");
    if (fromValue === null || toValue === null || fromValue < 0 || toValue < 0) continue;
    const conversion = token(link.relation) === "load_to_end_use";
    if (token(link.relation) === "driver_to_load") {
      const wanted = service(link), fromMatches = service(from) === wanted || token(from.serviceKind) === "all" && members(from).some((member) => member?.level === "driver" && service(member) === wanted && periodOK(member, period, context) && token(member.scaleDomain) === "thermal" && unitOK(member.unit, "thermal"));
      if (from.level !== "driver" || to.level !== "load" || !wanted || !fromMatches || service(to) !== wanted || !close(fromValue, toValue) || [from, to].some((item) => token(item.scaleDomain) !== "thermal" || !unitOK(item.unit, "thermal")) || !unitOK(link.fromUnit, "thermal") || !unitOK(link.toUnit, "thermal")) continue;
    } else if (conversion) {
      const wanted = service(link);
      if (from.level !== "load" || to.level !== "end_use" || !wanted || service(from) !== wanted || service(to) !== wanted || endUse(to) !== wanted || token(from.zoneName) !== token(to.zoneName) || !unique(list(link.sourceIds)).length) continue;
      if (token(from.scaleDomain) !== "thermal" || token(to.scaleDomain) !== "site" || !unitOK(from.unit, "thermal") || !unitOK(to.unit, "site") || !unitOK(link.fromUnit, "thermal") || !unitOK(link.toUnit, "site")) continue;
    } else {
      if (!["end_use_to_carrier", "direct_end_use_to_carrier"].includes(token(link.relation)) || from.level !== "end_use" || to.level !== "carrier" || !carrier(to)) continue;
      if ([from, to].some((item) => token(item.scaleDomain) !== "site" || !unitOK(item.unit, "site")) || !unitOK(link.fromUnit, "site") || !unitOK(link.toUnit, "site") || !close(fromValue, toValue)) continue;
      if (service(link) && service(from) && service(link) !== service(from)) continue;
    }
    result.push({ link, from, to, fromValue, toValue, conversion });
  }
  return result;
}

function componentRows(item, current, period, context) {
  if (!["driver", "load"].includes(item.level)) return [];
  const values = item.level === "load" ? list(item.loadBreakdown) : members(item).filter(Boolean).map((member) => ({ component: member.thermalComponent, value: periodOK(member, period, context) && token(member.scaleDomain) === "thermal" ? member.value : null, unit: member.unit, sourceIds: member.sourceIds, nodeId: member.id }));
  return ["sensible", "latent"].map((key) => {
    const matches = values.filter((value) => token(value.component) === key);
    const valid = current && matches.every((value) => unitOK(value.unit || item.unit, "thermal"));
    return row(key, valid ? sum(matches.map((value) => field(value, "value"))) : null, "kWh thermal", { sourceIds: unique(matches.flatMap((value) => list(value.sourceIds))), nodeIds: unique(matches.map((value) => value.nodeId)) });
  });
}

function zoneRows(item, options, period) {
  if (!["driver", "load"].includes(item.level)) return [];
  const wanted = members(item).filter((member) => member?.level === item.level);
  const matches = (node) => node?.level === item.level && wanted.some((member) =>
    (item.level !== "driver" || category(member) && category(node) === category(member)) &&
    (service(member) ? service(node) === service(member) : token(member.serviceKind) === "all" && Boolean(service(node))));
  const results = token(options.scope?.kind) === "zone"
    ? [{ scope: options.scope, nodes: [item], periods: [{ id: period, nodes: [item] }] }]
    : list(options.zoneResults);
  const groupedZones = new Map(), rows = [];
  for (const result of results) {
    const zoneName = String(result?.scope?.zoneName || "").trim();
    if (token(result?.scope?.kind) === "zone" && zoneName) groupedZones.set(token(zoneName), [...(groupedZones.get(token(zoneName)) || []), result]);
  }
  for (const variants of groupedZones.values()) {
    const result = variants[0];
    const zoneName = String(result?.scope?.zoneName || "").trim();
    const periodNodes = (candidate) => {
      const periods = list(candidate.periods).filter((value) => token(value?.id) === period);
      if (periods.length > 1 && periods.some((value) => JSON.stringify(value.nodes) !== JSON.stringify(periods[0].nodes))) return null;
      return periods.length ? list(periods[0].nodes) : period === "annual" ? list(candidate.nodes) : [];
    };
    const zoneNodes = periodNodes(result);
    if (zoneNodes === null || variants.some((variant) => JSON.stringify(periodNodes(variant)) !== JSON.stringify(zoneNodes))) {
      rows.push(row(zoneName, null, "kWh thermal", { label: zoneName, nodeIds: [], sourceIds: [], status: "ambiguous" })); continue;
    }
    const selected = list(result.periods).some((candidate) => token(candidate?.id) === period);
    const selectedNodes = zoneNodes.filter((node) => matches(node) && periodOK(node, period, selected ? period : "") && (!node.zoneName || token(node.zoneName) === token(zoneName)) && token(node.scaleDomain) === "thermal" && unitOK(node.unit, "thermal"));
    if (!selectedNodes.length) continue;
    const distinct = [...uniqueNodes(selectedNodes).values()];
    rows.push(row(zoneName, sum(distinct.map((node) => field(node, "value"))), "kWh thermal", { label: zoneName, nodeIds: unique(selectedNodes.map((node) => node.id)), sourceIds: evidence(selectedNodes).sourceIds, ...(distinct.includes(null) ? { status: "ambiguous" } : {}) }));
  }
  return rows.sort((a, b) => (b.value ?? -Infinity) - (a.value ?? -Infinity) || a.key.localeCompare(b.key)).slice(0, 5);
}

function splitRows(items, endpoint, keyForNode) {
  const groups = new Map();
  for (const item of items) {
    const node = item[endpoint], key = keyForNode(node);
    if (!key) continue;
    const group = groups.get(key) || { nodes: [], links: [], values: [] };
    group.nodes.push(node); group.links.push(item.link); group.values.push(item.toValue);
    groups.set(key, group);
  }
  return [...groups].map(([key, group]) => row(key, sum(group.values), "kWh site", {
    label: label(group.nodes[0]), nodeIds: unique(group.nodes.map((node) => node.id)), linkIds: unique(group.links.map((link) => link.id)), sourceIds: evidence(group.links).sourceIds,
  }));
}

function allocationRows(items, period, context) {
  if (!items.length) return [];
  const values = { direct: [], allocated: [], unknown: [] }, links = { direct: [], allocated: [], unknown: [] };
  for (const item of items) {
    const originals = members(item.link);
    const leafTotal = sum(originals.map((member) => member && periodOK(member, period, context || token(item.link.period)) && unitOK(member.toUnit, "site") && unitOK(member.fromUnit, "site") && close(field(member, "fromValue"), field(member, "toValue")) ? field(member, "toValue") : null));
    if (!close(leafTotal, item.toValue)) { values.unknown.push(item.toValue); links.unknown.push(item.link); continue; }
    for (const member of originals) {
      const appearance = energyPathNodeAppearance(member || {});
      const key = appearance.allocated ? "allocated" : ["reported_meter", "reported_variable", "direct_zone_energy", "integrated_rate"].includes(token(member?.basis)) ? "direct" : "unknown";
      values[key].push(field(member, "toValue")); links[key].push(member || item.link);
    }
  }
  return Object.keys(values).map((key) => row(key, values[key].length ? sum(values[key]) : 0, "kWh site", { linkIds: unique(links[key].map((link) => link.id)), sourceIds: evidence(links[key]).sourceIds }));
}

function ratioRows(items, options, period, context) {
  const groups = new Map();
  for (const item of items.filter((item) => item.conversion)) groups.set(service(item.link), [...(groups.get(service(item.link)) || []), item]);
  return [...groups].map(([key, group]) => {
    const fromValue = sum(group.map((item) => item.fromValue)), toValue = sum(group.map((item) => item.toValue));
    const status = token(options.ratioQuality?.status);
    const available = !["missing", "unavailable", "not_requested", "not_applicable"].includes(status) && fromValue > 0 && toValue > 0 && Number.isFinite(fromValue / toValue);
    const total = (endpoint) => sum([...new Map(group.map((item) => [item[endpoint].id, item[endpoint]])).values()].map((node) => periodOK(node, period, context) ? field(node, "value") : null));
    return row(key, available ? fromValue / toValue : null, "", {
      fromValue, toValue, fromUnit: "kWh thermal", toUnit: "kWh site", serviceKind: key, ratioKind: "load_to_site_energy", status,
      partial: status === "partial" || !close(fromValue, total("from")) || !close(toValue, total("to")), linkIds: unique(group.map((item) => item.link.id)), sourceIds: evidence(group.map((item) => item.link)).sourceIds,
    });
  });
}

function contextRows(item, sources, period) {
  if (item.level !== "load") return [];
  const wanted = service(item);
  return sources.filter((source) => wanted && token(source.driverCategory) === `load.${wanted}` &&
    ["load.predicted.sensible", "load.predicted_vs_delivered.sensible"].includes(token(source.driverComponent)) &&
    (!item.zoneName || token(source.zoneName) === token(item.zoneName)))
    .map((source) => row(token(source.driverComponent), period === "annual" ? field(source, "effectiveValue") : null, String(source.normalizedUnit || ""), {
      scope: "run", sourceIds: [source.id], status: period === "annual" ? "reported" : "unavailable",
    }));
}

/** Immutable, period-scoped inspector facts. Source scalars are never fallback graph values. */
export function energyPathInspectorModel(item = {}, options = {}) {
  item = item && typeof item === "object" ? item : {};
  const kind = options.kind === "link" ? "link" : "node", period = token(options.period) || "annual";
  const context = token(options.periodContext) === period ? period : "";
  const current = (period === "annual" || /^m([1-9]|1[0-2])$/.test(period)) && periodOK(item, period, context);
  const nodes = list(options.nodes), links = physicalLinks(nodes, list(options.links), period, context);
  const relevant = links.filter(({ link }) => kind === "link" ? link.id === item.id : link.fromId === item.id || link.toId === item.id);
  const trace = evidence([item, ...relevant.map(({ link }) => link)]);
  const sourceMap = new Map(list(options.sources).filter((source) => source?.id).map((source) => [source.id, source]));
  const selectedSources = [], visited = new Set(), pending = [...trace.sourceIds];
  while (pending.length) {
    const id = pending.shift();
    if (visited.has(id)) continue;
    visited.add(id);
    const source = sourceMap.get(id);
    if (source) { selectedSources.push(source); pending.push(...list(source.inputSourceIds)); }
  }
  const matchingContext = list(options.sources).filter((source) => item.level === "load" && token(source?.driverCategory) === `load.${service(item)}` && (!item.zoneName || token(source.zoneName) === token(item.zoneName)));
  const sources = [...new Map([...selectedSources, ...matchingContext].map((source) => [source.id, source])).values()];
  trace.sourceIds = unique([...trace.sourceIds, ...visited, ...sources.map((source) => source.id)]);
  trace.relatedEntityIds = unique([...trace.relatedEntityIds, ...sources.flatMap((source) => list(source.relatedEntityIds))]);
  const appearance = energyPathNodeAppearance(item), basis = token(item.basis);
  const valueRows = kind === "link"
    ? [row("from", current ? field(item, "fromValue") : null, String(item.fromUnit || "")), row("to", current ? field(item, "toValue") : null, String(item.toUnit || ""))]
    : [row("total", current ? accountingField(item, "value", period, context || token(item.period)) : null, displayUnit(item)), ...["raw", "effective", "allocated"].map((key) => {
      const direction = key === "allocated" ? "" : driverDirection(item, key + "Value");
      return row(key, current && direction !== "unknown" ? accountingField(item, key + "Value", period, context || token(item.period)) : null, displayUnit(item), direction ? { direction } : {});
    }), row("multiplier", current ? multiplier(item, period, context) : null)];
  const splits = current ? relevant.filter((link) => !link.conversion) : [];
  const breakdown = {
    componentRows: kind === "node" ? componentRows(item, current, period, context || token(item.period)) : [], zoneRows: kind === "node" && current ? zoneRows(item, options, period) : [],
    carrierRows: kind === "node" && item.level === "end_use" ? splitRows(splits, "to", carrier) : [],
    endUseRows: kind === "node" && item.level === "carrier" ? splitRows(splits, "from", endUse) : [],
    allocationRows: item.level === "end_use" ? allocationRows(splits, period, context) : [], contextRows: current ? contextRows(item, sources, period) : [],
    ratioRows: current ? ratioRows(relevant, options, period, context) : [],
  };
  if (kind === "node" && item.level === "carrier") {
    const quality = options.carrierQuality;
    if (current && quality && periodOK(quality, period, period) && carrier(quality) === carrier(item)) {
      const subtotal = token(options.scope?.kind) === "zone" || token(item.basis) === "reported_end_use_subtotal" || token(item.presentationCoverage) === "partial";
      for (const [key, name] of [[subtotal ? "observed_subtotal" : "facility_total", "expectedValue"], ["classified", "explainedValue"], ["residual", "residualValue"]]) breakdown.contextRows.push(row(key, field(quality, name), String(quality.unit || item.unit || ""), { scope: "period", sourceIds: unique(list(quality.sourceIds)) }));
    }
    for (const activity of current ? list(options.supplyActivities) : []) if (carrier(activity) === carrier(item) && periodOK(activity, period, period)) {
      breakdown.contextRows.push(row(token(activity.kind), field(activity, "value"), String(activity.unit || ""), { label: label(activity), scope: "period", nodeIds: unique(list(activity.nodeIds)), sourceIds: unique(list(activity.sourceIds)) }));
    }
  }
  for (const rows of Object.values(breakdown)) trace.sourceIds = unique([...trace.sourceIds, ...rows.flatMap((item) => list(item.sourceIds))]);
  return {
    representation: { stage: kind === "link" ? "link" : token(item.level), serviceKind: service(item), label: label(item) }, valueRows, breakdown,
    basis: { kind: basis, allocationLabelKind: appearance.allocationLabelKind, explanation: BASIS_EXPLANATIONS[basis] || "Calculation basis is unavailable.", application: unique(sources.map((source) => token(source.multiplierApplication))).filter((value) => ["already_model_total", "requires_zone_multiplier", "requires_group_multiplier", "unknown"].includes(value)).join(", ") },
    ...trace,
  };
}
