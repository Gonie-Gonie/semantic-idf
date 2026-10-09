import { energyPathWithOriginalMembers, mergeEnergyPathOriginalMembers } from "./energy-path-grouping.js";

const token = (value) => String(value ?? "").trim().toLowerCase();
const finite = (value) => typeof value === "number" && Number.isFinite(value);
const auxiliaryEndUses = new Set(["fans", "pumps", "fans_pumps", "heat_rejection", "heat_recovery", "humidification", "hvac_auxiliaries"]);
const contextRelations = new Set(["input_to_driver", "load_to_auxiliary"]);
const energyUnit = (value) => /^(?:kwh)(?:\s+(?:thermal|site|boundary))?$/.test(token(value).replace(/[()[\]_]/g, " ").replace(/\s+/g, " ").trim());

export function isEnergyPathContextRelation(link = {}) {
  return contextRelations.has(token(link.relation));
}

// A reported net zero stays zero even if a separate gross magnitude is present.
export function energyPathInputSignedValue(node = {}) {
  if (Object.prototype.hasOwnProperty.call(node, "signedValue")) return finite(node.signedValue) ? node.signedValue : null;
  return finite(node.value) ? node.value : null;
}

const unique = (values) => [...new Set((values || []).filter(Boolean))].sort();
const inputKinds = new Set(["input.solar_incident", "input.exterior_convection", "input.exterior_longwave", "input.internal_gains", "input.solar_absorbed", "input.surface_storage"]);

/** Group surface observations by their reported physical quantity, never by
 * magnitude. Member identities remain available for Monthly/Hourly inspection. */
export function energyPathGroupInputs(nodes = [], links = [], labelForKind = (_, fallback) => fallback) {
  const groups = new Map(), replacements = new Map(), others = [];
  for (const node of nodes) {
    const kind = token(node.kind);
    if (token(node.level) !== "input" || token(node.scaleDomain) !== "boundary" || !inputKinds.has(kind)) {
      others.push(node);
      continue;
    }
    const key = [kind, token(node.period) || "annual", token(node.zoneName), token(node.unit)].join("|");
    let group = groups.get(key);
    if (!group) {
      const signed = energyPathInputSignedValue(node);
      const identity = [kind, token(node.zoneName), token(node.unit)].join("|");
      group = { ...energyPathWithOriginalMembers(node), id: `group.input.${encodeURIComponent(identity)}`, label: labelForKind(kind, node.label || kind), signedValue: signed, value: signed === null ? null : Math.abs(signed) };
      groups.set(key, group);
    } else {
      const signed = energyPathInputSignedValue(group), nextSigned = energyPathInputSignedValue(node);
      mergeEnergyPathOriginalMembers(group, node);
      group.signedValue = signed === null || nextSigned === null ? null : signed + nextSigned;
      group.value = group.signedValue === null ? null : Math.abs(group.signedValue);
      for (const field of ["rawValue", "effectiveValue", "allocatedValue", "displayValue"]) {
        if (Object.hasOwn(group, field) || Object.hasOwn(node, field)) group[field] = finite(group[field]) && finite(node[field]) ? group[field] + node[field] : null;
      }
      for (const field of ["sourceIds", "relatedEntityIds", "relatedPathIds"]) group[field] = unique([...(group[field] || []), ...(node[field] || [])]);
      if (group.basis !== node.basis) group.basis = "grouped_presentation";
    }
    replacements.set(node.id, group.id);
  }
  const groupedLinks = new Map(), unchangedLinks = [];
  for (const original of links) {
    const link = { ...original, fromId: replacements.get(original.fromId) || original.fromId, toId: replacements.get(original.toId) || original.toId };
    if (token(link.relation) !== "input_to_driver") {
      unchangedLinks.push(link);
      continue;
    }
    link.groupedMembers = contextLinkMembers(original);
    const key = [link.fromId, link.toId, link.relation, token(link.period), token(link.zoneName)].join("|");
    const current = groupedLinks.get(key);
    if (!current) {
      link.id = `context.${encodeURIComponent([link.fromId, link.toId, link.relation, token(link.zoneName)].join("|"))}`;
      groupedLinks.set(key, link);
    } else {
      current.groupedMembers = [...contextLinkMembers(current), ...contextLinkMembers(link)];
      current.sourceIds = unique([...(current.sourceIds || []), ...(link.sourceIds || [])]);
      current.relatedPathIds = unique([...(current.relatedPathIds || []), ...(link.relatedPathIds || [])]);
    }
  }
  return synchronizeEnergyPathContextValues([...others, ...groups.values()], [...unchangedLinks, ...groupedLinks.values()]);
}

function contextLinkMembers(link) {
  return (link.groupedMembers?.length ? link.groupedMembers : [link]).map((member) => ({ ...member, sourceIds: [...(member.sourceIds || [])] }));
}

/** Re-establish each context endpoint's own reference quantity after visual
 * grouping. A repeated full load is never summed as though it were allocated. */
export function synchronizeEnergyPathContextValues(nodes = [], links = []) {
  const nodeByID = new Map(nodes.map((node) => [node.id, node]));
  return { nodes, links: links.map((original) => {
    if (!isEnergyPathContextRelation(original)) return original;
    const link = { ...original, ratio: 0, ratioKind: "", ratioLabel: "" };
    if (!contextLinkMembers(link).every((member) => finite(member.fromValue) && finite(member.toValue))) return { ...link, fromValue: null, toValue: null };
    const from = nodeByID.get(link.fromId), to = nodeByID.get(link.toId);
    if (!from || !to) return link;
    link.fromUnit = from.unit || link.fromUnit;
    link.toUnit = to.unit || link.toUnit;
    if (token(link.relation) === "input_to_driver") {
      link.fromValue = energyPathInputSignedValue(from);
      link.toValue = finite(to.value) ? to.value : null;
    }
    // An auxiliary relation references only its proven served-load subset.
    // Keep each backend owner relation distinct, even when Fans/Pumps share a
    // display card; neither endpoint can be replaced by that card's total.
    return link;
  }) };
}

/** Measured associations only. These lines never enter ribbon scales, port
 * budgets, load closure, or conversion ratios. Every endpoint keeps its own
 * quantity and physical boundary; zero and negative observations are valid. */
export function energyPathContextConnectors(layout = {}, links = [], { period = "annual" } = {}) {
  const wantedPeriod = token(period), nodes = new Map((layout.nodes || []).map((node) => [node.id, node]));
  if (wantedPeriod !== "annual" && !/^m([1-9]|1[0-2])$/.test(wantedPeriod)) return [];
  const connectors = [], seen = new Map(), conflicting = new Set();
  for (const link of links || []) {
    if (!link?.id || !isEnergyPathContextRelation(link)) continue;
    const identity = JSON.stringify([link.fromId, link.toId, link.relation, link.fromValue, link.toValue, link.fromUnit, link.toUnit,
      link.period, link.zoneName, link.serviceKind, link.basis, [...(link.sourceIds || [])].sort(), [...(link.relatedPathIds || [])].sort()]);
    if (seen.has(link.id) && seen.get(link.id) !== identity) conflicting.add(link.id);
    else seen.set(link.id, identity);
  }
  const drawn = new Set();
  for (const link of links || []) {
    if (!link?.id || !isEnergyPathContextRelation(link) || conflicting.has(link.id) || drawn.has(link.id)) continue;
    const from = nodes.get(link.fromId), to = nodes.get(link.toId);
    if (!from || !to || ![link, from.node, to.node].every((item) => !item.period ? wantedPeriod === "annual" : token(item.period) === wantedPeriod)) continue;
    if (!finite(link.fromValue) || !finite(link.toValue) || !energyUnit(link.fromUnit) || !energyUnit(link.toUnit)) continue;
    if (!Array.isArray(link.sourceIds) || !link.sourceIds.some((id) => typeof id === "string" && id.trim())) continue;
    const zones = [link.zoneName, from.node.zoneName, to.node.zoneName].map(token).filter(Boolean);
    if (new Set(zones).size > 1) continue;
    const relation = token(link.relation);
    if (relation === "input_to_driver") {
      if (from.level !== "input" || token(from.node.scaleDomain) !== "boundary" || to.level !== "driver" || token(to.node.scaleDomain) !== "thermal") continue;
      if (energyPathInputSignedValue(from.node) === null) continue;
    } else {
      if (from.level !== "load" || to.level !== "end_use" || !auxiliaryEndUses.has(token(to.node.endUse)) ||
        token(from.node.scaleDomain) !== "thermal" || token(to.node.scaleDomain) !== "site" || link.fromValue < 0 || link.toValue < 0) continue;
      if (!Array.isArray(link.relatedPathIds) || !link.relatedPathIds.some((id) => typeof id === "string" && id.trim())) continue;
      const service = token(link.serviceKind);
      if (!["cooling", "heating"].includes(service) || token(from.node.serviceKind) !== service) continue;
    }
    const fromPoint = { x: from.x + from.width + 4, y: from.anchorY }, toPoint = { x: to.x - 4, y: to.anchorY };
    if (![fromPoint.x, fromPoint.y, toPoint.x, toPoint.y].every(finite) || toPoint.x <= fromPoint.x) continue;
    const middle = (fromPoint.x + toPoint.x) / 2;
    connectors.push({ id: link.id, fromId: link.fromId, toId: link.toId, relation, domain: "context", fromValue: link.fromValue, toValue: link.toValue,
      fromUnit: link.fromUnit, toUnit: link.toUnit, link,
      path: `M${fromPoint.x},${fromPoint.y} C${middle},${fromPoint.y} ${middle},${toPoint.y} ${toPoint.x},${toPoint.y}`,
      arrowPath: `M${toPoint.x - 5},${toPoint.y - 3} L${toPoint.x},${toPoint.y} L${toPoint.x - 5},${toPoint.y + 3}` });
    drawn.add(link.id);
  }
  const byPair = new Map();
  for (const connector of connectors) {
    const key = JSON.stringify([connector.fromId, connector.toId]);
    byPair.set(key, [...(byPair.get(key) || []), connector]);
  }
  for (const samePair of byPair.values()) {
    if (samePair.length < 2) continue;
    samePair.sort((left, right) => left.id.localeCompare(right.id));
    samePair.forEach((connector, index) => {
      const from = nodes.get(connector.fromId), to = nodes.get(connector.toId);
      const extent = Math.max(0, Math.min(6, from.height / 2 - 4, to.height / 2 - 4));
      const direction = 2 * index / (samePair.length - 1) - 1;
      const offset = extent * direction;
      const x0 = from.x + from.width + 4, x1 = to.x - 4;
      // Vertical offsets alone overlap on a steep curve once the canvas is
      // narrow. Spread control points sideways as well so each attribution
      // retains its own native pointer target without changing any quantity.
      const middle = Math.max(x0 + 1, Math.min(x1 - 1, (x0 + x1) / 2 + direction * 14));
      const y0 = from.anchorY + offset, y1 = to.anchorY + offset;
      connector.path = `M${x0},${y0} C${middle},${y0} ${middle},${y1} ${x1},${y1}`;
      connector.arrowPath = `M${x1 - 5},${y1 - 3} L${x1},${y1} L${x1 - 5},${y1 + 3}`;
    });
  }
  return connectors;
}
