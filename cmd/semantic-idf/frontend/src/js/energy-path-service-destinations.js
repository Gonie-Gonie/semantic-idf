import { energyPathLabelPeriodRange } from "./energy-path-navigation.js";
import { resolveEnergyPathOutputRequest, energyPathOutputRequestKey, energyPathOutputRequestFields } from "./energy-path-output-requests.js";

const list = (value) => Array.isArray(value) ? value : [];
const token = (value) => String(value ?? "").trim().toLowerCase();
const unique = (values) => [...new Set(values.filter((value) => typeof value === "string" && value.trim()))].sort();
const finite = (value) => typeof value === "number" && Number.isFinite(value);
const label = (value, id = "") => typeof value === "string" && value.trim() && value !== id ? value : "";
const idFor = (...parts) => `eps-${[...new TextEncoder().encode(JSON.stringify(parts))].map((byte) => byte.toString(16).padStart(2, "0")).join("")}`;
const service = (item) => ["cooling", "heating"].includes(token(item?.serviceKind)) ? token(item.serviceKind) : "";
const current = (item, period, context) => token(item?.period) ? token(item.period) === period : period === "annual" || context === period;
const unit = (value, domain) => ["kwh", `kwh ${domain}`].includes(token(value).replace(/[()[\]_]/g, " ").replace(/\s+/g, " "));
const domain = (node, expected) => token(node?.scaleDomain) === expected && unit(node.unit, expected);
const close = (a, b) => finite(a) && finite(b) && Math.abs(a - b) <= 1e-6 * Math.max(Number.MIN_VALUE, Math.abs(a), Math.abs(b));
const auxiliary = new Set(["fans", "pumps", "fans_pumps", "heat_rejection", "hvac_auxiliaries", "humidification", "heat_recovery"]);
const hvacEndUse = (node) => ["cooling", "heating"].includes(token(node.endUse)) || auxiliary.has(token(node.endUse));
const targetKey = (target) => JSON.stringify([token(target?.view), token(target?.targetKind).replaceAll("_", "-"), target?.targetId]);
const refKey = (ref) => JSON.stringify([token(ref?.objectType || ref?.type || ref?.loopType), token(ref?.objectName || ref?.name || ref?.loopName)]);

function indexed(items, key = "id") {
  const result = new Map();
  for (const item of list(items)) if (item?.[key]) {
    if (!result.has(item[key])) result.set(item[key], item);
    else if (result.get(item[key]) && JSON.stringify(result.get(item[key])) !== JSON.stringify(item)) result.set(item[key], null);
  }
  return result;
}

function originals(item, period, context, ancestors = new Set()) {
  if (!item || typeof item !== "object" || ancestors.has(item) || !current(item, period, context)) return [];
  const children = list(item.groupedMembers);
  return children.length ? children.flatMap((child) => originals(child, period, context, new Set(ancestors).add(item))) : [item];
}

function physicalLinks(node, options, period, context, scopeZone) {
  const byID = indexed(options.nodes), selectedIDs = new Set(unique([node.id, ...list(node.originalNodeIds)]));
  const output = [];
  for (const link of indexed(options.links).values()) {
    if (!link || !selectedIDs.has(link.fromId) && !selectedIDs.has(link.toId)) continue;
    const from = byID.get(link.fromId), to = byID.get(link.toId);
    if (!from || !to || ![link, from, to].every((item) => current(item, period, context))) continue;
    if (token(from.zoneName) !== token(to.zoneName) || link.zoneName && token(link.zoneName) !== token(from.zoneName)) continue;
    if (scopeZone && token(from.zoneName) !== token(scopeZone)) continue;
    if (!finite(link.fromValue) || !finite(link.toValue) || link.fromValue < 0 || link.toValue < 0) continue;
    const thermalService = service(from) && service(from) === service(to) && (!service(link) || service(link) === service(from));
    const relation = token(link.relation);
    const conversion = relation === "load_to_end_use" && from.level === "load" && to.level === "end_use" && thermalService && token(to.endUse) === service(to)
      && domain(from, "thermal") && domain(to, "site") && unit(link.fromUnit, "thermal") && unit(link.toUnit, "site") && link.fromValue > 0 && link.toValue > 0;
    const split = ["end_use_to_carrier", "direct_end_use_to_carrier"].includes(relation) && from.level === "end_use" && to.level === "carrier"
      && domain(from, "site") && domain(to, "site") && unit(link.fromUnit, "site") && unit(link.toUnit, "site") && close(link.fromValue, link.toValue);
    const driver = relation === "driver_to_load" && from.level === "driver" && to.level === "load" && thermalService
      && domain(from, "thermal") && domain(to, "thermal") && unit(link.fromUnit, "thermal") && unit(link.toUnit, "thermal") && close(link.fromValue, link.toValue);
    if (conversion || split || driver) output.push({ link, from, to, split });
  }
  return output;
}

function sourceEvidence(sourceIDs, sources) {
  const roots = new Set(unique(sourceIDs)), byID = indexed(sources), visited = new Set(), output = [];
  const visit = (id) => {
    if (visited.has(id)) return;
    visited.add(id);
    const source = byID.get(id);
    if (!source) return;
    output.push({ source, derived: !roots.has(id) });
    list(source.inputSourceIds).forEach(visit);
  };
  unique(sourceIDs).forEach((id) => visit(id));
  return output;
}

function hvacCandidates(node, evidence, options, scopeZone) {
  const model = options.hvac?.serviceModel || {}, navigation = options.semanticNavigation || {};
  const semanticEntities = indexed(navigation.entities), semanticOccurrences = indexed(navigation.occurrences, "occurrenceId");
  const paths = indexed(list(model.zoneServices).flatMap((zone) => list(zone.paths).filter((path) => !path.zoneName || !zone.zoneName || token(path.zoneName) === token(zone.zoneName)).map((path) => ({ ...path, zoneName: path.zoneName || zone.zoneName || path.servedSubject?.zoneName || "" }))));
  const wanted = new Set(evidence.pathIds), related = new Set(evidence.entityIds), wantedService = service(node) || service({ serviceKind: node.endUse }) || service({ serviceKind: options.service });
  const reportNavigation = indexed(model.navigation?.entities);
  const occurrencesByEntity = new Map();
  for (const occurrence of semanticOccurrences.values()) if (occurrence?.entityId) {
    if (!occurrencesByEntity.has(occurrence.entityId)) occurrencesByEntity.set(occurrence.entityId, []);
    occurrencesByEntity.get(occurrence.entityId).push(occurrence);
  }
  for (const entity of semanticEntities.values()) if (entity && related.has(entity.id)) {
    const entityKind = token(entity.kind).replaceAll("_", "-");
    if (!entityKind.startsWith("hvac-")) continue;
    for (const target of [...list(entity.viewTargets), ...(occurrencesByEntity.get(entity.id) || []).flatMap((item) => list(item.viewTargets))]) {
      if (token(target.view) !== "hvac") continue;
      const kind = token(target.targetKind).replaceAll("_", "-"), record = reportNavigation.get(target.targetId);
      if (entityKind === "hvac-path" && ["service-path", "hvac-path"].includes(kind) && paths.get(target.targetId)) wanted.add(target.targetId);
      if (record && ["component", "loop", "system"].includes(token(record.kind)) && kind === `hvac-${token(record.kind)}`) related.add(record.id);
    }
  }
  for (const record of reportNavigation.values()) if (record && related.has(record.id)) list(record.relatedPathIds).forEach((id) => wanted.add(id));
  const pathRefs = (path) => [path.airLoop, path.plantLoop, path.condenserLoop, path.sourceSystem, path.refrigerantSystem, path.delivery, path.deliveryWrapper, ...list(path.conditioning)].filter(Boolean);
  for (const path of paths.values()) if (path && pathRefs(path).some((ref) => ref.id && related.has(ref.id))) wanted.add(path.id);
  const allowedKinds = auxiliary.has(token(node.endUse)) ? ["cooling", "heating", "ventilation", "exhaust"] : ["cooling", "heating"];
  const selectedPaths = [...paths.values()].filter((path) => path && (wanted.has(path.id) || related.has(path.id)) && allowedKinds.includes(token(path.serviceKind))
    && (!scopeZone || token(path.zoneName) === token(scopeZone)) && (!wantedService || token(path.serviceKind) === wantedService)
    && (!evidence.services.length || evidence.services.includes(token(path.serviceKind)))
    && (!evidence.pathServices.has(path.id) || evidence.pathServices.get(path.id).has(token(path.serviceKind))));
  const selectedIDs = new Set(selectedPaths.map((path) => path.id)), records = new Map(), references = new Map();
  const add = (kind, id, record) => records.set(targetKey({ view: "hvac", targetKind: kind, targetId: id }), record);
  for (const path of selectedPaths) {
    const name = unique([label(path.delivery?.displayName) || label(path.delivery?.objectName), label(path.sourceSystem?.displayName), label(path.airLoop?.name), label(path.plantLoop?.name)]).join(" · ");
    const record = { label: name || "HVAC service path", labelKind: name ? "" : "service_path", zoneName: path.zoneName, pathIds: [path.id], serviceKind: path.serviceKind, physicalKey: `path:${path.id}`, evidenceKind: "explicit_path" };
    add("service-path", path.id, record); add("hvac-path", path.id, record);
    for (const ref of pathRefs(path)) {
      const key = refKey(ref);
      if (!references.has(key)) references.set(key, []);
      references.get(key).push({ ref, path });
    }
  }
  const actualLoops = list(options.hvac?.loops);
  for (const record of reportNavigation.values()) {
    if (!record || !["loop", "component", "system"].includes(token(record.kind))) continue;
    const matches = (references.get(refKey(record)) || []).filter(({ ref, path }) => (!Number.isInteger(ref.objectIndex) || !Number.isInteger(record.objectIndex) || ref.objectIndex === record.objectIndex)
      && (!list(record.relatedPathIds).length || record.relatedPathIds.includes(path.id)));
    if (!matches.length) continue;
    if (record.kind === "loop" && !actualLoops.some((loop) => refKey(loop) === refKey(record) && (!Number.isInteger(loop.objectIndex) || !Number.isInteger(record.objectIndex) || loop.objectIndex === record.objectIndex))) continue;
    const pathIds = unique(matches.map(({ path }) => path.id).filter((id) => selectedIDs.has(id)));
    if (!pathIds.length) continue;
    const zones = unique(matches.map(({ path }) => path.zoneName));
    const kind = token(record.kind), routeKind = kind === "loop" ? token(record.loopType || record.objectType) === "airloophvac" ? "air_loop" : "plant_loop" : kind;
    const name = label(record.label, record.id) || label(record.objectName, record.id);
    add(`hvac-${kind}`, record.id, { label: name || "Connected HVAC equipment", labelKind: name ? "" : routeKind, routeKind, zoneName: scopeZone || (zones.length === 1 ? zones[0] : ""), pathIds, physicalKey: record.id, evidenceKind: "explicit_component" });
  }
  const output = new Map();
  for (const entity of semanticEntities.values()) if (entity) {
    const own = new Map(list(entity.viewTargets).map((target) => [targetKey(target), target]));
    const choices = [...own.values()].map((target) => ({ target, occurrenceId: "", contextLabel: "" })), occurrenceTargets = new Map();
    for (const occurrence of occurrencesByEntity.get(entity.id) || []) for (const target of list(occurrence.viewTargets)) if (!own.has(targetKey(target))) {
      const key = targetKey(target);
      if (!occurrenceTargets.has(key)) occurrenceTargets.set(key, []);
      const contextLabel = [occurrence.sourceAnchor?.objectName, occurrence.sourceAnchor?.fieldName, Number.isInteger(occurrence.sourceAnchor?.objectIndex) ? `#${occurrence.sourceAnchor.objectIndex + 1}` : ""].filter(Boolean).join(" · ");
      occurrenceTargets.get(key).push({ target, occurrenceId: occurrence.occurrenceId, contextLabel });
    }
    for (const variants of occurrenceTargets.values()) if (variants.length === 1 || variants.every((item) => item.contextLabel) && new Set(variants.map((item) => item.contextLabel)).size === variants.length) choices.push(...variants);
    for (const { target, occurrenceId, contextLabel } of choices) {
      const record = records.get(targetKey(target));
      if (!record) continue;
      const id = idFor("hvac", entity.id, occurrenceId, target.view, target.targetKind, target.targetId);
      output.set(id, { id, kind: "hvac", view: "hvac", entityId: entity.id, entityKind: entity.kind, occurrenceId, target: { ...target }, contextLabel, ...record });
    }
  }
  const physicalTargets = new Map();
  for (const candidate of output.values()) {
    if (!physicalTargets.has(candidate.physicalKey)) physicalTargets.set(candidate.physicalKey, []);
    physicalTargets.get(candidate.physicalKey).push(candidate);
  }
  const candidates = [];
  for (const variants of physicalTargets.values()) {
    const specific = (candidate) => token(candidate.entityKind).replaceAll("_", "-") === (candidate.physicalKey.startsWith("path:") ? "hvac-path" : token(candidate.target.targetKind).replaceAll("_", "-"));
    // A Zone may expose the same real route as its dedicated HVAC entity. Prefer
    // the physical owner, not a first record or a value-dependent choice.
    let retained = variants.some(specific) ? variants.filter(specific) : variants;
    if (new Set(retained.map((item) => item.entityId)).size !== 1) continue;
    if (retained.some((item) => token(item.target.targetKind).replaceAll("_", "-") === "service-path")) retained = retained.filter((item) => token(item.target.targetKind).replaceAll("_", "-") !== "hvac-path");
    candidates.push(...retained);
  }
  return { candidates, paths: selectedPaths };
}

function ledgerCandidates(evidence, paths, options, period, scopeZone) {
  const dataset = options.heatFlow || {}, geometry = options.geometry || {}, count = dataset.frameCount;
  if (!Number.isInteger(count) || count <= 0 || list(dataset.labels).length !== count || !list(dataset.categories).length) return [];
  const labels = dataset.labels.map((value) => typeof value === "string" ? value.trim().replace(/^(\d{2})\/(\d{2})(\s+\d{2}:\d{2}(?::\d{2})?)$/, "$1-$2$3") : value);
  const range = energyPathLabelPeriodRange(labels, period);
  if (!range) return [];
  const zones = new Set(unique([scopeZone, ...evidence.zoneNames, ...paths.map((path) => path.zoneName)]).map(token));
  const actualZones = indexed(list(dataset.zones).map((zone) => ({ ...zone, id: token(zone.name) })));
  const geometryZones = indexed(list(geometry.zones).map((zone) => ({ ...zone, id: token(zone.name) })));
  const indices = Array.from({ length: (range.end === -1 ? count - 1 : range.end) - range.start + 1 }, (_, index) => range.start + index);
  const output = [];
  for (const [key, zone] of actualZones) {
    if (!zone || !zones.has(key) || scopeZone && key !== token(scopeZone) || !geometryZones.get(key)) continue;
    const floors = list(geometry.surfaces).filter((surface) => token(surface.zoneName) === key && token(surface.surfaceType) === "floor" && list(surface.vertices).length >= 3 && surface.vertices.every((point) => finite(point.x) && finite(point.y)));
    if (!floors.length || list(zone.values).length !== dataset.categories.length || !zone.values.every((values) => list(values).length === count && indices.every((index) => finite(values[index])))) continue;
    const target = { view: "simulation", targetKind: "heat-flow-zone", targetId: zone.name };
    output.push({ id: idFor("heat_flow", zone.name, period, dataset.sourceFile || ""), kind: "heat_flow", view: "simulation", target, zoneName: zone.name, label: zone.name, labelKind: "", evidenceKind: "zone_context", pathIds: paths.filter((path) => token(path.zoneName) === key).map((path) => path.id), range: { ...range } });
  }
  return output;
}

const carriers = { electricity: "electricity", naturalgas: "natural_gas", gas: "natural_gas", districtcooling: "district_cooling", districtheating: "district_heating", districtheatingwater: "district_heating", districtheatingsteam: "steam", steam: "steam", propane: "propane", coal: "coal", diesel: "diesel", gasoline: "gasoline", fueloilno1: "fuel_oil_1", fueloil1: "fuel_oil_1", fueloilno2: "fuel_oil_2", fueloil2: "fuel_oil_2", otherfuel1: "other_fuel_1", otherfuel2: "other_fuel_2" };
const carrier = (value) => carriers[token(value).replace(/[^a-z0-9]/g, "")] || "";
function facilitySource(source, wanted) {
  if (!(source.isMeter === true || token(source.sourceType) === "sql_meter")) return false;
  const parts = token(source.keyValue || source.name).split(":").map((item) => item.replace(/[^a-z0-9]/g, ""));
  return parts.length === 2 && (parts[0] === "facility" && carrier(parts[1]) === wanted || parts[1] === "facility" && carrier(parts[0]) === wanted);
}

function outputCandidates(node, sources, options, scopeZone) {
  const output = [], statuses = [], wantedCarrier = carrier(node.carrier);
  for (const item of sources) {
    const { source } = item;
    if (scopeZone && source.zoneName && token(source.zoneName) !== token(scopeZone)) continue;
    const resolution = resolveEnergyPathOutputRequest(source, list(options.outputObjects));
    statuses.push(resolution.status);
    if (resolution.status !== "exact" || node.level === "carrier" && (!wantedCarrier || !facilitySource(source, wantedCarrier))) continue;
    const requestKey = energyPathOutputRequestKey(resolution.request, resolution.requestIndex);
    const name = label(source.name, source.id) || label(source.keyValue, source.id);
    output.push({ id: idFor("output", source.id, requestKey), kind: "output", view: "simulation", target: { view: "simulation", targetKind: "output-request", targetId: requestKey }, sourceId: source.id, requestKey, requestIndex: resolution.requestIndex, requestFields: energyPathOutputRequestFields(resolution.request),
      zoneName: scopeZone || source.zoneName || "", label: name || "Output request", labelKind: name ? "" : "output_request", evidenceKind: item.derived ? "derived_input" : "exact_source", pathIds: [] });
  }
  const reason = node.level === "carrier" ? "facility_meter_unavailable" : statuses.includes("ambiguous") ? "output_ambiguous" : statuses.includes("derived") ? "output_derived" : statuses.includes("tabular") ? "output_tabular" : "output_unavailable";
  return { candidates: output, reason };
}

/** Resolve explicit service evidence without mutating the report or choosing a first target. */
export function energyPathServiceDestinations(node = {}, options = {}) {
  const period = token(options.period || "annual"), context = token(options.periodContext), scopeZone = token(options.scope?.kind) === "zone" ? String(options.scope.zoneName || "") : String(node.zoneName || "");
  const isLoad = node.level === "load" && service(node), isEndUse = node.level === "end_use", isCarrier = node.level === "carrier";
  const unavailableReasons = {};
  if (isLoad || isEndUse && hvacEndUse(node)) unavailableReasons.hvac = "no_explicit_hvac_path";
  if (isLoad) unavailableReasons.heat_flow = "heat_flow_unavailable";
  if (isEndUse || isCarrier) unavailableReasons.output = "output_unavailable";
  const empty = (reason) => ({ groups: [], unavailableReasons: Object.fromEntries(Object.keys(unavailableReasons).map((key) => [key, reason || unavailableReasons[key]])) });
  if (!Object.keys(unavailableReasons).length) return empty();
  if (!current(node, period, context) || !/^(annual|m([1-9]|1[0-2]))$/.test(period)) return empty("period_mismatch");
  if (node.zoneName && scopeZone && token(node.zoneName) !== token(scopeZone) || token(options.scope?.kind) === "zone" && !scopeZone) return empty("scope_mismatch");
  if (isEndUse && service({ serviceKind: node.endUse }) && service(node) && service({ serviceKind: node.endUse }) !== service(node)) return empty("service_mismatch");
  if (service(node) && service({ serviceKind: options.service }) && service(node) !== token(options.service)) return empty("service_mismatch");
  if (isEndUse && service({ serviceKind: node.endUse }) && service({ serviceKind: options.service }) && token(node.endUse) !== token(options.service)) return empty("service_mismatch");
  if (!domain(node, isLoad ? "thermal" : "site")) return empty("unsupported_domain");
  const members = originals(node, period, context), links = physicalLinks(node, options, period, context, scopeZone);
  const ownSourceIDs = unique(members.flatMap((item) => list(item.sourceIds)));
  const metadata = [...members, ...links.flatMap(({ link }) => originals(link, period, context))];
  const sources = sourceEvidence(unique([...ownSourceIDs, ...metadata.flatMap((item) => list(item.sourceIds))]), options.sources);
  const services = unique(metadata.map((item) => token(item.serviceKind)).filter((kind) => ["cooling", "heating", "ventilation", "exhaust"].includes(kind))), pathServices = new Map();
  for (const item of metadata) if (services.includes(token(item.serviceKind))) for (const path of list(item.relatedPathIds)) {
    if (!pathServices.has(path)) pathServices.set(path, new Set());
    pathServices.get(path).add(token(item.serviceKind));
  }
  const evidence = { pathIds: unique(metadata.flatMap((item) => list(item.relatedPathIds))), entityIds: unique([...metadata.flatMap((item) => list(item.relatedEntityIds)), ...sources.flatMap(({ source }) => list(source.relatedEntityIds))]), zoneNames: unique([...members.map((item) => item.zoneName), ...sources.map(({ source }) => source.zoneName)]), services, pathServices };
  const candidates = [], hvac = unavailableReasons.hvac ? hvacCandidates(node, evidence, options, scopeZone) : { candidates: [], paths: [] };
  if (Object.hasOwn(unavailableReasons, "hvac")) { candidates.push(...hvac.candidates); if (hvac.candidates.length) unavailableReasons.hvac = ""; }
  if (isLoad) {
    const ledger = ledgerCandidates(evidence, hvac.paths, options, period, scopeZone); candidates.push(...ledger);
    unavailableReasons.heat_flow = ledger.length ? "" : period !== "annual" ? "heat_flow_period_unavailable" : "heat_flow_unavailable";
  }
  if (isEndUse || isCarrier) {
    const outputSources = sourceEvidence(unique([...ownSourceIDs, ...links.filter(({ split, from }) => split && from.id === node.id).flatMap(({ link }) => list(link.sourceIds))]), options.sources);
    const output = outputCandidates(node, outputSources, options, scopeZone); candidates.push(...output.candidates); unavailableReasons.output = output.candidates.length ? "" : output.reason;
  }
  const groups = new Map();
  for (const candidate of candidates) {
    const id = idFor("group", candidate.kind, token(candidate.zoneName));
    if (!groups.has(id)) groups.set(id, { id, kind: candidate.kind, zoneName: candidate.zoneName || "", label: candidate.zoneName || "Building sources", labelKind: candidate.zoneName ? "" : "building_source_group", value: null, unit: "", candidates: [] });
    groups.get(id).candidates.push(candidate);
  }
  for (const group of groups.values()) {
    group.candidates.sort((a, b) => a.id.localeCompare(b.id));
    const rows = list(options.zoneRows).filter((row) => group.zoneName && token(row.zoneName || row.key || row.label) === token(group.zoneName) && (!row.period || token(row.period) === period));
    if (rows.length === 1 && finite(rows[0].value)) { group.value = rows[0].value; group.unit = rows[0].unit || ""; }
  }
  return { groups: [...groups.values()].sort((a, b) => a.kind.localeCompare(b.kind) || (b.value ?? -Infinity) - (a.value ?? -Infinity) || a.id.localeCompare(b.id)), unavailableReasons };
}
