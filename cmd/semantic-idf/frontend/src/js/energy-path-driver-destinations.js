const list = (value) => Array.isArray(value) ? value : [];
const token = (value) => String(value ?? "").trim().toLowerCase();
const unique = (values) => [...new Set(values.filter((value) => typeof value === "string" && value.trim()))].sort();
const text = (value, id = "") => typeof value === "string" && value.trim() && value !== id ? value : "";
const number = (value) => (typeof value === "number" || typeof value === "string" && value.trim()) && Number.isFinite(Number(value)) ? Number(value) : null;
const identity = (parts) => [...new TextEncoder().encode(JSON.stringify(parts))].map((byte) => byte.toString(16).padStart(2, "0")).join("");
const targetKey = (view, kind, id) => JSON.stringify([token(view), token(kind).replaceAll("_", "-"), id]);
const categoryDimensions = {
  "internal.people": ["occupancy"], "internal.lighting": ["lighting"], "internal.equipment": ["equipment"],
  "air.infiltration": ["infiltration"], "air.mechanical_ventilation": ["ventilation", "outdoor_air"],
};
const surfaceCategories = new Set(["surface.exterior_walls", "surface.roofs", "surface.ground_floors", "surface.windows_doors", "surface.interzone"]);
const supportedCategories = new Set([...surfaceCategories, ...Object.keys(categoryDimensions), "air.interzone"]);
const preparedNavigationStates = new WeakMap();

function navigationIdentity(options) {
  const geometry = options.geometry, topology = geometry?.topology, profile = options.profile;
  const hvac = options.hvac, model = hvac?.serviceModel || hvac, navigation = options.semanticNavigation;
  return [geometry, topology, topology?.nodes, topology?.boundaries, topology?.openings,
    topology?.connections, topology?.airCouplings, geometry?.zones, profile, profile?.zoneProfiles,
    profile?.groups, hvac, hvac?.serviceModel, model?.zoneServices, navigation, navigation?.entities, navigation?.occurrences];
}

/**
 * Prepare selection-independent model indexes when a scene/report is activated.
 * The opaque token is valid only for the same report/navigation identities and
 * top-level collections. Nested model records are immutable within those arrays.
 */
export function prepareEnergyPathDriverNavigation(options = {}) {
  const geometry = options.geometry || {}, topology = geometry.topology || {}, profile = options.profile || {};
  const navigation = options.semanticNavigation || {}, entities = indexed(navigation.entities);
  const occurrencesByEntity = new Map();
  for (const occurrence of indexed(navigation.occurrences, "occurrenceId").values()) if (occurrence?.entityId) {
    if (!occurrencesByEntity.has(occurrence.entityId)) occurrencesByEntity.set(occurrence.entityId, []);
    occurrencesByEntity.get(occurrence.entityId).push(occurrence);
  }
  const routesByTarget = new Map();
  for (const entity of entities.values()) if (entity) {
    const targets = new Map();
    const addTarget = (target, occurrence = null) => {
      const key = targetKey(target?.view, target?.targetKind, target?.targetId);
      if (!targets.has(key)) targets.set(key, { target, entityHasTarget: false, occurrences: new Map() });
      const definition = targets.get(key);
      if (occurrence) definition.occurrences.set(occurrence.occurrenceId, occurrence);
      else definition.entityHasTarget = true;
    };
    list(entity.viewTargets).forEach((target) => addTarget(target));
    for (const occurrence of occurrencesByEntity.get(entity.id) || []) list(occurrence.viewTargets).forEach((target) => addTarget(target, occurrence));
    for (const [key, definition] of targets) {
      if (!routesByTarget.has(key)) routesByTarget.set(key, []);
      routesByTarget.get(key).push({ entity, target: definition.target,
        variants: occurrenceVariants([...definition.occurrences.values()], definition.entityHasTarget) });
    }
  }
  const prepared = Object.freeze({});
  preparedNavigationStates.set(prepared, {
    identity: navigationIdentity(options), routesByTarget,
    nodes: indexed(topology.nodes), boundaries: indexed(topology.boundaries), openings: indexed(topology.openings),
    connections: indexed(topology.connections), airCouplings: indexed(topology.airCouplings), zones: indexed(geometry.zones),
    profileZones: list(profile.zoneProfiles).map((zone) => ({ zone, items: indexed(zone.items) })),
    profileGroups: indexed(profile.groups),
  });
  return prepared;
}

function navigationState(options) {
  const prepared = preparedNavigationStates.get(options.preparedNavigation);
  const identity = navigationIdentity(options);
  if (prepared && prepared.identity.every((value, index) => Object.is(value, identity[index]))) return prepared;
  return preparedNavigationStates.get(prepareEnergyPathDriverNavigation(options));
}

function indexed(items, key = "id") {
  const result = new Map();
  for (const item of list(items)) if (item?.[key]) {
    const prior = result.get(item[key]);
    if (!result.has(item[key])) result.set(item[key], item);
    else if (prior && JSON.stringify(prior) !== JSON.stringify(item)) result.set(item[key], null);
  }
  return result;
}

function selectedEvidence(node, allSources) {
  const ids = [], entities = [], paths = [], seen = new Set();
  const visit = (item) => {
    if (!item || typeof item !== "object" || seen.has(item)) return;
    seen.add(item); ids.push(...list(item.sourceIds)); entities.push(...list(item.relatedEntityIds)); paths.push(...list(item.relatedPathIds));
    list(item.groupedMembers).forEach(visit);
  };
  visit(node);
  const byID = indexed(allSources), sources = [], visited = new Set();
  const visitSource = (id) => {
    if (visited.has(id)) return;
    visited.add(id);
    const source = byID.get(id);
    if (!source) return;
    sources.push(source); entities.push(...list(source.relatedEntityIds));
    list(source.inputSourceIds).forEach(visitSource);
  };
  unique(ids).forEach(visitSource);
  return { sources, entityIds: new Set(unique(entities)), nodeEntityIds: new Set(unique([...seen].flatMap((item) => list(item.relatedEntityIds)))), pathIds: new Set(unique(paths)) };
}

function surfaceCategory(boundary) {
  const type = token(boundary.surfaceType), relation = token(boundary.relationKind), condition = token(boundary.boundaryCondition);
  if (relation.includes("adiabatic") || type.includes("internalmass") || relation === "invalid") return "";
  if (/window|door|fenestration|glass/.test(type)) return "surface.windows_doors";
  if (/interzone|interspace/.test(relation)) return "surface.interzone";
  const external = /exterior|outdoor|other_side|otherside|external/.test(relation + " " + condition);
  if (type === "floor" && (/ground|foundation/.test(relation + " " + condition) || external)) return "surface.ground_floors";
  if (type === "wall" && external) return "surface.exterior_walls";
  if (["roof", "ceiling", "roofceiling"].includes(type) && external) return "surface.roofs";
  return "";
}

function reportTargets(category, options, prepared) {
  const records = new Map();
  const { nodes, boundaries, openings } = prepared;
  const zoneName = (id) => {
    const node = nodes.get(id);
    if (!node) return "";
    if (node.zoneName) return node.zoneName;
    return token(node.kind) === "zone" ? node.label || node.objectName || "" : "";
  };
  const add = (view, kind, id, record) => {
    if (!id) return;
    const key = targetKey(view, kind, id), prior = records.get(key);
    const value = { ...record, view, targetKind: kind, targetId: id };
    if (!records.has(key)) records.set(key, value);
    else if (prior && JSON.stringify(prior) !== JSON.stringify(value)) records.set(key, null);
  };
  for (const boundary of boundaries.values()) if (boundary && surfaceCategory(boundary) === category) {
    const record = { zoneName: zoneName(boundary.ownerZoneId), label: text(boundary.surfaceName), entityIds: unique([boundary.id, boundary.surfaceId, boundary.surfaceEntityId]), anchors: list(boundary.sourceAnchors), specificity: 2 };
    add("topology", "thermal_boundary", boundary.id, record);
    add("topology", "surface", boundary.surfaceId, record);
  }
  if (category === "surface.windows_doors") for (const opening of openings.values()) if (opening && /window|door|fenestration|glass/.test(token(opening.surfaceType))) {
    add("topology", "fenestration", opening.windowId, { zoneName: zoneName(opening.ownerZoneId), label: text(opening.name), entityIds: unique([opening.id, opening.windowId, opening.entityId]), anchors: list(opening.sourceAnchors), specificity: 2 });
  }
  for (const connection of prepared.connections.values()) if (connection && !connection.qaOnly && surfaceCategories.has(category)) {
    const matches = list(connection.boundaryIds).map((id) => boundaries.get(id)).filter((item) => item && surfaceCategory(item) === category);
    const matchedOpenings = category === "surface.windows_doors" ? list(connection.openingIds).map((id) => openings.get(id)).filter((item) => item && /window|door|fenestration|glass/.test(token(item.surfaceType))) : [];
    if (!matches.length && !matchedOpenings.length) continue;
    const zones = unique([...matches, ...matchedOpenings].map((item) => zoneName(item.ownerZoneId)));
    add("topology", "thermal_connection", connection.id, { zoneName: zones.length === 1 ? zones[0] : "", zoneNames: zones, label: "Related connection context", labelKind: "connection_context", entityIds: [connection.id], anchors: [], contextOnly: true, specificity: 1 });
  }
  for (const coupling of prepared.airCouplings.values()) if (coupling) {
    const kind = token(coupling.couplingKind), fromZone = zoneName(coupling.fromNodeId), toZone = zoneName(coupling.toNodeId);
    const from = nodes.get(coupling.fromNodeId), to = nodes.get(coupling.toNodeId);
    if (!from || !to) continue;
    const interzone = Boolean(fromZone && toZone && token(fromZone) !== token(toZone));
    const outdoor = Boolean((fromZone && !toZone && /outdoor/.test(token(to.kind))) || (toZone && !fromZone && /outdoor/.test(token(from.kind))));
    const allowed = category === "air.interzone" && interzone && ["zone_mixing", "zone_cross_mixing", "refrigeration_door_mixing", "construction_air_boundary", "airflow_network"].includes(kind)
      || category === "air.mechanical_ventilation" && kind === "outdoor_ventilation" && outdoor
      || category === "air.infiltration" && kind === "airflow_network" && outdoor;
    if (allowed) add("topology", "thermal_air_coupling", coupling.id, { zoneName: interzone ? "" : toZone || fromZone, zoneNames: unique([fromZone, toZone]), label: text(coupling.objectName) || "Related air coupling", labelKind: text(coupling.objectName) ? "" : "air_coupling", entityIds: unique([coupling.id, coupling.entityId]), anchors: list(coupling.sourceAnchors), specificity: 2 });
  }
  if (category === "air.infiltration") for (const node of nodes.values()) if (node && token(node.kind) === "zone") {
    add("topology", "zone", node.id, { zoneName: zoneName(node.id), label: text(node.label) || "Zone topology context", labelKind: text(node.label) ? "" : "zone_context", entityIds: unique([node.id, node.entityId]), anchors: [], contextOnly: true, specificity: 0 });
  }
  // Zone view targets use GeometryZone.ID, whereas thermal nodes use the
  // semantic entity ID. Both are real report identities, not interchangeable.
  if (category === "air.infiltration") for (const zone of prepared.zones.values()) if (zone && !records.has(targetKey("topology", "zone", zone.id))) {
    add("topology", "zone", zone.id, { zoneName: zone.name || "", label: text(zone.name) || "Zone topology context", labelKind: text(zone.name) ? "" : "zone_context", entityIds: [zone.id], anchors: [], contextOnly: true, specificity: 0 });
  }
  const dimensions = categoryDimensions[category] || [];
  if (dimensions.length) for (const { zone, items } of prepared.profileZones) {
    for (const item of items.values()) if (item && dimensions.includes(token(item.dimension)) && (!item.zoneName || token(item.zoneName) === token(zone.zoneName))) {
      add("profile", "profile-item", item.id, { zoneName: zone.zoneName, label: text(item.objectName) || token(item.dimension), labelKind: text(item.objectName) ? "" : `profile_${token(item.dimension)}`, entityIds: [item.id], anchors: [{ objectIndex: item.objectIndex, objectType: item.objectType, objectName: item.objectName }], specificity: 2 });
    }
    for (const dimension of list(zone.dimensions)) if (dimensions.includes(token(dimension.dimension))) {
      const encodedZone = [...new TextEncoder().encode(token(zone.zoneName))].map((byte) => byte >= 97 && byte <= 122 || byte >= 48 && byte <= 57 || [45, 95, 46].includes(byte) ? String.fromCharCode(byte) : `%${byte.toString(16).padStart(2, "0")}`).join("");
      add("profile", "zone-dimension", `profile-zone-dimension:${encodedZone}:${token(dimension.dimension)}`, { zoneName: zone.zoneName, label: dimension.label || token(dimension.dimension), labelKind: `profile_${token(dimension.dimension)}`, entityIds: [], anchors: [], contextOnly: true, specificity: 1 });
    }
  }
  if (dimensions.length) for (const group of prepared.profileGroups.values()) if (group && list(group.dimensions).some((item) => dimensions.includes(token(item.dimension)))) {
    const zones = unique(list(group.zoneNames));
    add("profile", "profile-group", group.id, { zoneName: zones.length === 1 ? zones[0] : "", zoneNames: zones, label: text(group.name) || "Profile source group", labelKind: text(group.name) ? "" : "profile_source_group", entityIds: [group.id], anchors: [], contextOnly: true, specificity: 0 });
  }
  if (category === "air.mechanical_ventilation") {
    const model = options.hvac?.serviceModel || options.hvac || {};
    const outdoorAir = (item) => ["outdoor_air", "ventilation"].includes(token(item?.role)) || ["outdoorair:mixer", "airloophvac:outdoorairsystem", "zonehvac:outdoorairunit"].includes(token(item?.objectType));
    for (const zone of list(model.zoneServices)) for (const path of list(zone.paths)) {
      const components = [path.delivery, path.deliveryWrapper, ...list(path.conditioning)].filter(Boolean);
      if (!(["ventilation", "outdoor_air"].includes(token(path.serviceKind)) || components.some(outdoorAir))) continue;
      if (path.zoneName && zone.zoneName && token(path.zoneName) !== token(zone.zoneName)) continue;
      const name = text(path.sourceSystem?.displayName) || text(path.airLoop?.name);
      const record = { zoneName: path.zoneName || zone.zoneName || path.servedSubject?.zoneName || "", label: name || "Outdoor-air service", labelKind: name ? "" : "outdoor_air_service", entityIds: unique([path.id, ...components.filter(outdoorAir).map((item) => item.id)]), pathIds: [path.id], anchors: [], specificity: 2 };
      add("hvac", "service-path", path.id, record);
    }
  }
  return records;
}

function matchingAnchor(left, right) {
  if (left?.objectId && right?.objectId) return left.objectId === right.objectId;
  return Number.isInteger(left?.objectIndex) && left.objectIndex >= 0 && left.objectIndex === right?.objectIndex
    && token(left.objectType) && token(left.objectType) === token(right.objectType)
    && token(left.objectName) && token(left.objectName) === token(right.objectName);
}

function occurrenceVariants(occurrences, entityHasTarget) {
  // A verified entity target identifies the physical destination independently
  // of its many semantic field contexts. Do not select one field arbitrarily.
  if (entityHasTarget) return [{ destinationAnchors: occurrences.map((item) => item.sourceAnchor).filter(Boolean) }];
  if (occurrences.length < 2) return occurrences.length ? occurrences : [null];
  const contextLabels = { zone_profile: "Zone profile", zone_geometry: "Zone geometry", definition: "Definition", source_only: "Source", zone_service: "Zone service" };
  const contextLabel = (item) => [item.sourceAnchor?.objectName, item.sourceAnchor?.fieldName, contextLabels[item.contextKind],
    Number.isInteger(item.sourceAnchor?.objectIndex) ? `Source ${item.sourceAnchor.objectIndex + 1}` : ""].filter(Boolean).join(" · ");
  const labels = occurrences.map(contextLabel);
  return labels.every(Boolean) && new Set(labels).size === labels.length ? occurrences.map((item, index) => ({ ...item, destinationContextLabel: labels[index] })) : [];
}

/** Read-only destinations: report membership proves model context, never an energy attribution. */
export function energyPathDriverDestinations(node = {}, options = {}) {
  const category = token(node.driverCategory), period = token(options.period || "annual");
  const empty = (reason) => ({ category, status: "unavailable", groups: [], unavailableReason: reason });
  if (node.level !== "driver" || !supportedCategories.has(category)) return empty("unsupported_category");
  if (node.period && token(node.period) !== period) return empty("period_mismatch");
  const scopeZone = token(options.scope?.kind) === "zone" ? String(options.scope.zoneName || "") : String(node.zoneName || "");
  if (token(options.scope?.kind) === "zone" && !scopeZone || node.zoneName && scopeZone && token(node.zoneName) !== token(scopeZone)) return empty("scope_mismatch");
  const prepared = navigationState(options), evidence = selectedEvidence(node, options.sources);
  const records = reportTargets(category, options, prepared), candidates = new Map();
  for (const [recordKey, record] of records) if (record) {
    for (const { entity, target, variants } of prepared.routesByTarget.get(recordKey) || []) {
      const zones = unique([record.zoneName, ...list(record.zoneNames)]);
      if (scopeZone && (!zones.length || !zones.some((zone) => token(zone) === token(scopeZone)))) continue;
      if (scopeZone && zones.length > 1 && record.targetKind === "profile-group") continue;
      for (const occurrence of variants) {
        const anchors = [...list(entity.sourceAnchors), ...list(occurrence?.destinationAnchors), occurrence?.sourceAnchor].filter(Boolean);
        // Anchors cross-check Profile's generated item ID against its semantic source object.
        if (record.view === "profile" && record.targetKind === "profile-item" && !anchors.some((anchor) => record.anchors.some((item) => matchingAnchor(anchor, item)))) continue;
        const actualZone = scopeZone || record.zoneName || "";
        const ownsRecord = record.entityIds.includes(entity.id) || anchors.some((anchor) => record.anchors.some((item) => matchingAnchor(anchor, item)));
        const matchesEntity = (id) => record.entityIds.includes(id) || ownsRecord && (id === entity.id || anchors.some((anchor) => anchor.objectId === id));
        const ownSources = evidence.sources.filter((source) => (!source.zoneName || !actualZone || token(source.zoneName) === token(actualZone)) && list(source.relatedEntityIds).some(matchesEntity));
        const exact = !record.contextOnly && (ownSources.length > 0 || [...evidence.nodeEntityIds].some(matchesEntity) || list(record.pathIds).some((id) => evidence.pathIds.has(id)));
        const sourceIds = unique(ownSources.map((source) => source.id));
        const key = identity([entity.id, occurrence?.occurrenceId || "", token(target.view), target.targetKind, target.targetId]);
        candidates.set(key, { id: `epd-${key}`, entityId: entity.id, entityKind: entity.kind, occurrenceId: occurrence?.occurrenceId || "", view: token(target.view),
          target: { view: token(target.view), targetKind: target.targetKind, targetId: target.targetId, label: text(target.label, target.targetId) || record.label || "Related model context" },
          label: record.label || text(target.label, target.targetId) || text(entity.label, entity.id) || "Related model context", labelKind: record.labelKind || "", contextLabel: occurrence?.destinationContextLabel || "", evidenceKind: exact ? "exact_source" : "category_context",
          zoneName: actualZone, zoneNames: zones, sourceIds, specificity: record.specificity, physicalKey: JSON.stringify([record.entityIds, actualZone]), contextOnly: record.contextOnly === true });
      }
    }
  }
  const allCandidates = [...candidates.values()], specificGroups = new Set(), thermalBoundaries = new Set();
  for (const candidate of allCandidates) {
    if (candidate.specificity > 0) specificGroups.add(JSON.stringify([candidate.view, token(candidate.zoneName)]));
    if (token(candidate.target.targetKind).replaceAll("-", "_") === "thermal_boundary") thermalBoundaries.add(candidate.physicalKey);
  }
  const values = allCandidates.filter((candidate) => (candidate.specificity > 0 || !specificGroups.has(JSON.stringify([candidate.view, token(candidate.zoneName)])))
    && !(candidate.target.targetKind === "surface" && thermalBoundaries.has(candidate.physicalKey)));
  const groups = new Map();
  for (const candidate of values) {
    const sharedZones = candidate.zoneName ? [] : candidate.zoneNames;
    const key = identity([category, candidate.view, token(candidate.zoneName), sharedZones.map(token)]);
    if (!groups.has(key)) groups.set(key, { id: `epdg-${key}`, label: candidate.zoneName || sharedZones.join(" ↔ ") || "Building source group", labelKind: candidate.zoneName || sharedZones.length ? "" : "building_source_group", zoneName: candidate.zoneName, value: null, unit: "", evidenceKind: "category_context", sourceIds: [], entityIds: [], candidates: [] });
    const group = groups.get(key);
    group.candidates.push(candidate); group.sourceIds.push(...candidate.sourceIds); group.entityIds.push(candidate.entityId);
  }
  for (const group of groups.values()) {
    group.candidates.sort((a, b) => a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
    group.sourceIds = unique(group.sourceIds); group.entityIds = unique(group.entityIds);
    group.evidenceKind = group.candidates.every((candidate) => candidate.evidenceKind === "exact_source") ? "exact_source" : "category_context";
    const rows = list(options.zoneRows).filter((item) => token(item.zoneName || item.key || item.label) === token(group.zoneName) && (!item.period || token(item.period) === period));
    if (group.zoneName && rows.length === 1 && number(rows[0].value) !== null) { group.value = number(rows[0].value); group.unit = String(rows[0].unit || ""); }
  }
  const output = [...groups.values()].sort((a, b) => (b.value ?? -Infinity) - (a.value ?? -Infinity) || a.id.localeCompare(b.id));
  return output.length ? { category, status: "available", groups: output, unavailableReason: "" } : empty("matching_model_target_unavailable");
}
