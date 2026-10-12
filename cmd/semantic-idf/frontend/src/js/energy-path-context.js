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
const inputKinds = new Set(["input.solar_incident", "input.exterior_convection", "input.exterior_longwave", "input.people", "input.lighting", "input.equipment", "input.internal_gains", "input.solar_absorbed", "input.surface_storage"]);
const inputKind = (value) => token(value).startsWith("input.equipment.") ? "input.equipment" : token(value);
const heatDirection = (node) => ["gain", "loss"].includes(node.heatDirection) ? node.heatDirection : "";

/** Group surface observations by their reported physical quantity, never by
 * magnitude. Member identities remain available for Monthly/Hourly inspection. */
export function energyPathGroupInputs(nodes = [], links = [], labelForKind = (_, fallback) => fallback) {
  const groups = new Map(), replacements = new Map(), others = [];
  for (const node of nodes) {
    const kind = inputKind(node.kind);
    if (token(node.level) !== "input" || token(node.scaleDomain) !== "boundary" || !inputKinds.has(kind)) {
      others.push(node);
      continue;
    }
    const direction = heatDirection(node);
    const key = [kind, direction, token(node.period) || "annual", token(node.zoneName), token(node.unit)].join("|");
    let group = groups.get(key);
    if (!group) {
      const signed = energyPathInputSignedValue(node);
      const identity = [kind, ...(direction ? [direction] : []), token(node.zoneName), token(node.unit)].join("|");
      group = { ...energyPathWithOriginalMembers(node), kind, id: `group.input.${encodeURIComponent(identity)}`, label: labelForKind(kind, node.label || kind), signedValue: signed, value: signed === null ? null : Math.abs(signed) };
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
      if (group.heatDirectionBasis !== node.heatDirectionBasis) group.heatDirectionBasis = "mixed";
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

function contextLinkMembers(link, ancestors = new Set()) {
  if (!link || ancestors.has(link)) return [];
  const members = link.groupedMembers;
  if (Array.isArray(members) && members.length) {
    const path = new Set(ancestors).add(link);
    return members.flatMap((member) => contextLinkMembers(member, path));
  }
  return [{ ...link, sourceIds: [...(link.sourceIds || [])] }];
}

function inputDriverReference(node, members) {
  const targets = new Set(energyPathWithOriginalMembers(node).groupedMembers
    .filter((member) => token(member.level) === "driver").map((member) => member.id));
  const references = new Map();
  for (const member of members) {
    if (!targets.has(member.toId) || !finite(member.toValue) || member.toValue < 0) return null;
    if (references.has(member.toId) && references.get(member.toId) !== member.toValue) return null;
    references.set(member.toId, member.toValue);
  }
  const total = [...references.values()].reduce((sum, value) => sum + value, 0);
  return references.size && finite(total) ? total : null;
}

/** Re-establish each context endpoint's own reference quantity after visual
 * grouping. A repeated full load is never summed as though it were allocated. */
export function synchronizeEnergyPathContextValues(nodes = [], links = []) {
  const nodeByID = new Map(nodes.map((node) => [node.id, node]));
  return { nodes, links: links.map((original) => {
    if (!isEnergyPathContextRelation(original)) return original;
    const link = { ...original, ratio: 0, ratioKind: "", ratioLabel: "" };
    const members = contextLinkMembers(link);
    if (!members.length || !members.every((member) => finite(member.fromValue) && finite(member.toValue))) return { ...link, fromValue: null, toValue: null };
    const from = nodeByID.get(link.fromId), to = nodeByID.get(link.toId);
    if (!from || !to) return link;
    link.fromUnit = from.unit || link.fromUnit;
    link.toUnit = to.unit || link.toUnit;
    if (token(link.relation) === "input_to_driver") {
      link.fromValue = energyPathInputSignedValue(from);
      // Several surfaces can reference the same original room driver, while
      // People/Lighting/Equipment can share one Internal gains display card.
      // Count each matching original target once, never the card's full total.
      link.toValue = inputDriverReference(to, members);
    }
    // An auxiliary relation references only its proven served-load subset.
    // Keep each backend owner relation distinct, even when Fans/Pumps share a
    // display card; neither endpoint can be replaced by that card's total.
    return link;
  }) };
}

/** Measured associations only. Each endpoint is a separate reference quantity,
 * not an additive share of its node. These ribbons use independent context
 * scales and visual ports; accounting scales, load closure and COP stay owned
 * by the canonical flow overlay. Negative inputs retain their signed values
 * while their geometric widths show magnitude. Zero never acquires volume. */
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
      if (energyPathInputSignedValue(from.node) === null || link.toValue < 0) continue;
    } else {
      if (from.level !== "load" || to.level !== "end_use" || !auxiliaryEndUses.has(token(to.node.endUse)) ||
        token(from.node.scaleDomain) !== "thermal" || token(to.node.scaleDomain) !== "site" || link.fromValue < 0 || link.toValue < 0) continue;
      if (!Array.isArray(link.relatedPathIds) || !link.relatedPathIds.some((id) => typeof id === "string" && id.trim())) continue;
      const service = token(link.serviceKind);
      if (!["cooling", "heating"].includes(service) || token(from.node.serviceKind) !== service) continue;
    }
    if (![from, to].every((node) => [node.x, node.y, node.width, node.height, node.anchorY].every(finite) && node.width > 0 && node.height > 0)) continue;
    const x0 = from.x + from.width + 3, x1 = to.x - 3;
    if (!(x1 > x0)) continue;
    connectors.push({ id: link.id, fromId: link.fromId, toId: link.toId, relation, domain: "context", fromValue: link.fromValue, toValue: link.toValue,
      fromUnit: link.fromUnit, toUnit: link.toUnit, link,
      fromDomain: relation === "input_to_driver" ? "boundary" : "thermal", toDomain: relation === "input_to_driver" ? "thermal" : "site",
      fromPort: { x: x0 }, toPort: { x: x1 }, gap: { x0, x1, width: x1 - x0 } });
    drawn.add(link.id);
  }
  return placeContextRibbons(connectors, nodes);
}

function placeContextRibbons(connectors, nodes) {
  const nodeOrder = new Map([...nodes.keys()].map((id, index) => [id, index]));
  const compare = (left, right) => nodeOrder.get(left.fromId) - nodeOrder.get(right.fromId) ||
    nodeOrder.get(left.toId) - nodeOrder.get(right.toId) || left.id.localeCompare(right.id);
  connectors.sort(compare);
  const sides = new Map();
  for (const connector of connectors) {
    for (const side of ["from", "to"]) {
      const key = JSON.stringify([connector[`${side}Id`], side]);
      if (!sides.has(key)) sides.set(key, { node: nodes.get(connector[`${side}Id`]), side, domain: connector[`${side}Domain`], entries: [] });
      sides.get(key).entries.push(connector);
    }
  }
  const scales = { boundary: Infinity, thermal: Infinity, site: Infinity };
  for (const group of sides.values()) {
    const capacity = Math.max(0, group.node.height - 4), count = group.entries.length;
    group.gap = count > 1 ? Math.min(2, capacity / (2 * (count - 1))) : 0;
    // This sum fits independent references into separate visual lanes only;
    // it is never exported or reported as a node/flow energy total.
    group.magnitude = group.entries.reduce((sum, connector) => sum + Math.abs(connector[`${group.side}Value`]), 0);
    if (!finite(group.magnitude)) {
      group.entries.forEach((connector) => { connector.invalidGeometry = true; });
      continue;
    }
    const available = capacity - group.gap * Math.max(0, count - 1);
    if (group.magnitude > 0) scales[group.domain] = Math.min(scales[group.domain], Number.MAX_VALUE, available / group.magnitude);
  }
  for (const domain of Object.keys(scales)) if (!finite(scales[domain])) scales[domain] = 0;
  for (const group of sides.values()) {
    const coefficient = scales[group.domain];
    const totalHeight = group.magnitude * coefficient + group.gap * Math.max(0, group.entries.length - 1);
    let y = group.node.anchorY - totalHeight / 2;
    for (const connector of group.entries) {
      const width = Math.abs(connector[`${group.side}Value`]) * coefficient;
      connector[`${group.side}Width`] = width;
      connector[`${group.side}PixelsPerKWh`] = coefficient;
      Object.assign(connector[`${group.side}Port`], { y0: y, y1: y + width });
      y += width + group.gap;
    }
  }
  return connectors.filter((connector) => !connector.invalidGeometry).map((connector) => {
    const from = connector.fromPort, to = connector.toPort, middle = (from.x + to.x) / 2;
    const fromCenter = (from.y0 + from.y1) / 2, toCenter = (to.y0 + to.y1) / 2;
    return { ...connector,
      path: `M${from.x},${from.y0} C${middle},${from.y0} ${middle},${to.y0} ${to.x},${to.y0} L${to.x},${to.y1} C${middle},${to.y1} ${middle},${from.y1} ${from.x},${from.y1} Z`,
      centerPath: `M${from.x},${fromCenter} C${middle},${fromCenter} ${middle},${toCenter} ${to.x},${toCenter}`,
      ratioAnchor: { x: middle, y: (fromCenter + toCenter) / 2 },
    };
  });
}
