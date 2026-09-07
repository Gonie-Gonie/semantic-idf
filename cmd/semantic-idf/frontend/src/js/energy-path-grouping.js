const DRIVER_CANDIDATES = new Set(["internal.people", "internal.lighting", "internal.equipment"]);
const END_USE_CANDIDATES = new Set(["lighting", "equipment", "water_systems", "refrigeration"]);
const SUM_FIELDS = ["value", "signedValue", "rawValue", "effectiveValue", "allocatedValue", "displayValue"];

const token = (value) => String(value || "").trim().toLowerCase();
const unique = (values) => [...new Set((values || []).filter(Boolean))].sort();

function memberSnapshot(node) {
  const { groupedMembers, originalNodeIds, automaticOther, ...member } = node;
  return {
    ...member,
    sourceIds: unique(node.sourceIds),
    relatedEntityIds: unique(node.relatedEntityIds),
    relatedPathIds: unique(node.relatedPathIds),
  };
}

export function energyPathWithOriginalMembers(node = {}) {
  return {
    ...node,
    originalNodeIds: unique(node.originalNodeIds?.length ? node.originalNodeIds : [node.id]),
    groupedMembers: (node.groupedMembers?.length ? node.groupedMembers : [memberSnapshot(node)])
      .map(memberSnapshot),
  };
}

export function mergeEnergyPathOriginalMembers(current, next) {
  const members = new Map();
  for (const node of [current, next]) {
    for (const member of energyPathWithOriginalMembers(node).groupedMembers) {
      const key = [member.id, member.period, member.zoneName, member.serviceKind].join("|");
      members.set(key, member);
    }
  }
  current.originalNodeIds = unique([
    ...(current.originalNodeIds || [current.id]), ...(next.originalNodeIds || [next.id]),
  ]);
  current.groupedMembers = [...members.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([, member]) => member);
}

function groupScope(node) {
  return token(node.groupingScope || node.zoneName || String(node.id || "").split(".").at(-1) || "building");
}

function groupBucket(node) {
  const level = token(node.level);
  if (level !== "driver" && level !== "end_use") return "";
  const domain = token(node.scaleDomain) || (level === "driver" ? "thermal" : "site");
  const unit = token(node.unit).replace(/[\s_-]+/g, "");
  return [level, groupScope(node), token(node.period) || "annual", token(node.serviceKind) || "all", domain, unit].join("|");
}

function denominatorBucket(node) {
  // HVAC and direct uses share one site-energy stage total even when their
  // service labels differ. Group membership still keeps services separate.
  return groupBucket(token(node.level) === "end_use" ? { ...node, serviceKind: "all" } : node);
}

function category(node) {
  return token(node.level) === "driver" ? token(node.driverCategory) : token(node.endUse);
}

function isOther(node) {
  return token(node.level) === "driver"
    ? ["internal.other", "balance.storage_other"].includes(category(node))
    : token(node.level) === "end_use" && category(node) === "other";
}

function isCandidate(node) {
  return token(node.level) === "driver"
    ? DRIVER_CANDIDATES.has(category(node))
    : token(node.level) === "end_use" && END_USE_CANDIDATES.has(category(node));
}

function mergeNode(current, next) {
  mergeEnergyPathOriginalMembers(current, next);
  for (const field of SUM_FIELDS) {
    if (Object.hasOwn(current, field) || Object.hasOwn(next, field)) {
      current[field] = (Number(current[field]) || 0) + (Number(next[field]) || 0);
    }
  }
  for (const field of ["sourceIds", "relatedEntityIds", "relatedPathIds", "presentationEndUses", "badges"]) {
    current[field] = unique([...(current[field] || []), ...(next[field] || [])]);
  }
  current.offsetEffects = [...(current.offsetEffects || []), ...(next.offsetEffects || [])];
  current.allocationApplied = current.allocationApplied === true && next.allocationApplied === true;
  if (current.basis !== next.basis) current.basis = "grouped_presentation";
  if (Number(current.rawValue)) current.multiplier = Number(current.effectiveValue) / Number(current.rawValue);
}

function linkMemberSnapshots(link) {
  const members = Array.isArray(link.groupedMembers) && link.groupedMembers.length
    ? link.groupedMembers : [link];
  return members.map((member) => ({ ...member, sourceIds: unique(member.sourceIds) }));
}

// Presentation only: retain canonical accounting/export records in the payload.
// Named carriers and auxiliary lanes never enter an automatic Other group.
export function energyPathGroupSmallNodes(nodes = [], links = []) {
  const copiedNodes = nodes.map(energyPathWithOriginalMembers);
  const protectedIDs = new Set(links
    .filter((link) => token(link.relation) === "source_correspondence")
    .flatMap((link) => [link.fromId, link.toId]));
  const buckets = new Map();
  const denominators = new Map();
  for (const node of [...copiedNodes].sort((left, right) => String(left.id).localeCompare(String(right.id)))) {
    const key = groupBucket(node);
    const value = Number(node.value);
    if (!key || !Number.isFinite(value) || value <= 0) continue;
    const denominatorKey = denominatorBucket(node);
    denominators.set(denominatorKey, (denominators.get(denominatorKey) || 0) + value);
    const bucket = buckets.get(key) || { key, denominatorKey, nodes: [] };
    bucket.nodes.push(node);
    buckets.set(key, bucket);
  }

  const replacements = new Map();
  const groups = [];
  for (const bucket of buckets.values()) {
    const total = denominators.get(bucket.denominatorKey) || 0;
    const boundary = total * 0.01;
    const tolerance = total * Number.EPSILON * 16;
    const eligible = bucket.nodes.filter((node) => !protectedIDs.has(node.id));
    const candidates = eligible.filter((node) => isCandidate(node) && Number(node.value) < boundary - tolerance);
    const existingOther = eligible.filter(isOther);
    if (!candidates.length || (candidates.length < 2 && !existingOther.length)) continue;
    const members = [...existingOther, ...candidates].sort((left, right) => String(left.id).localeCompare(String(right.id)));
    const first = members[0];
    const driver = token(first.level) === "driver";
    const stableKey = bucket.key.replace(/[^a-z0-9]+/g, "_");
    const id = existingOther.map((node) => node.id).sort()[0] || `group.other.${stableKey}`;
    const group = energyPathWithOriginalMembers(first);
    for (const member of members.slice(1)) mergeNode(group, member);
    group.id = id;
    group.kind = driver ? "driver.balance.storage_other" : "energy.other";
    group.label = driver ? "Other / storage" : "Other";
    group.automaticOther = true;
    group.groupingThreshold = 0.01;
    group.groupingTotal = total;
    group.groupingScope = groupScope(first);
    if (driver) group.driverCategory = "balance.storage_other";
    else group.endUse = "other";
    for (const member of members) replacements.set(member.id, id);
    groups.push(group);
  }

  if (!groups.length) return { nodes: copiedNodes, links: links.map((link) => ({ ...link })) };
  const groupedLinks = new Map();
  for (const original of links) {
    const link = {
      ...original,
      fromId: replacements.get(original.fromId) || original.fromId,
      toId: replacements.get(original.toId) || original.toId,
      sourceIds: unique(original.sourceIds),
      originalLinkIds: unique(original.originalLinkIds || [original.id]),
    };
    const changed = replacements.has(original.fromId) || replacements.has(original.toId);
    const key = [link.fromId, link.toId, link.relation, link.basis, link.serviceKind, link.period, link.zoneName, link.fromUnit, link.toUnit, changed ? "" : original.id].join("|");
    const current = groupedLinks.get(key);
    if (!current) {
      if (changed) link.id = `group.link.${key}`;
      groupedLinks.set(key, link);
      continue;
    }
    // Preserve each link's own allocation evidence before presentation values
    // are summed. Two grouped_presentation links need not have the same basis
    // mix, even though they share this grouping bucket.
    current.groupedMembers = [...linkMemberSnapshots(current), ...linkMemberSnapshots(link)];
    for (const field of ["fromValue", "toValue", "value", "signedValue", "displayValue"]) {
      if (Object.hasOwn(current, field) || Object.hasOwn(link, field)) {
        current[field] = (Number(current[field]) || 0) + (Number(link[field]) || 0);
      }
    }
    for (const field of ["sourceIds", "relatedPathIds", "originalLinkIds"]) {
      current[field] = unique([...(current[field] || []), ...(link[field] || [])]);
    }
  }
  return {
    nodes: [...copiedNodes.filter((node) => !replacements.has(node.id)), ...groups]
      .sort((left, right) => String(left.id).localeCompare(String(right.id))),
    links: [...groupedLinks.values()].sort((left, right) => String(left.id).localeCompare(String(right.id))),
  };
}
