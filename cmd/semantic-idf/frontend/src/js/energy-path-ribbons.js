const token = (value) => String(value ?? "").trim().toLowerCase();
const number = (value) => (typeof value === "number" || typeof value === "string" && value.trim() !== "") && Number.isFinite(Number(value)) ? Number(value) : null;
const service = (node = {}) => token(node.serviceKind) || token(node.endUse) || token(node.kind).match(/(?:^|\.)(cooling|heating)(?:$|\.)/)?.[1] || "";
const thermalServices = new Set(["cooling", "heating"]);
const domainForLevel = (level) => ["driver", "load"].includes(level) ? "thermal" : ["end_use", "carrier"].includes(level) ? "site" : "";
const close = (left, right) => Math.abs(left - right) <= 1e-6 * Math.max(Number.MIN_VALUE, Math.abs(left), Math.abs(right));

function unitMatches(unit, domain) {
  const normalized = token(unit).replace(/[()[\]_]/g, " ").replace(/\s+/g, " ");
  return normalized === "kwh" || normalized === `kwh ${domain}`;
}

function periodMatches(period, items, requireEvidence = false) {
  const known = items.map((item) => token(item?.period)).filter(Boolean);
  return known.every((value) => value === period) && (!requireEvidence || period === "annual" || known.length > 0);
}

function sourceTraced(link) {
  return Array.isArray(link.sourceIds) && link.sourceIds.some((id) => typeof id === "string" && id.trim() !== "");
}

function driverServes(node, wanted, period, zone) {
  if (service(node) === wanted) return true;
  // All-service presentation merges proven original driver members. The word
  // "all" alone is not evidence that an arbitrary driver serves both loads.
  return service(node) === "all" && (node.groupedMembers || []).some((member) =>
    member?.level === "driver" && service(member) === wanted &&
    token(member.scaleDomain) === "thermal" && unitMatches(member.unit, "thermal") &&
    token(member.zoneName) === zone && periodMatches(period, [member]));
}

function validateLink(item, from, to, period) {
  const link = item.link;
  if (!link || !link.id || item.id !== link.id || item.fromId !== link.fromId || item.toId !== link.toId) return "identity";
  if (!from || !to) return "endpoint";
  if (!periodMatches(period, [link, from.node, to.node], true)) return "period";
  const zones = [link.zoneName, from.node.zoneName, to.node.zoneName].map(token).filter(Boolean);
  if (new Set(zones).size > 1) return "zone";
  const relation = token(link.relation), fromDomain = from.domain, toDomain = to.domain;
  const wanted = token(link.serviceKind);
  if (relation === "driver_to_load") {
    if (from.level !== "driver" || to.level !== "load" || fromDomain !== "thermal" || toDomain !== "thermal") return "relation_domain";
    if (!thermalServices.has(wanted) || service(to.node) !== wanted || !driverServes(from.node, wanted, period, token(to.node.zoneName))) return "service";
  } else if (relation === "load_to_end_use") {
    if (from.level !== "load" || to.node.level !== "end_use" || fromDomain !== "thermal" || toDomain !== "site") return "relation_domain";
    if (!thermalServices.has(wanted) || service(from.node) !== wanted || token(to.node.endUse) !== wanted || service(to.node) !== wanted) return "service";
    if (token(from.node.zoneName) !== token(to.node.zoneName)) return "zone";
    if (!sourceTraced(link)) return "source_trace";
  } else if (["end_use_to_carrier", "direct_end_use_to_carrier"].includes(relation)) {
    if (from.node.level !== "end_use" || to.level !== "carrier" || fromDomain !== "site" || toDomain !== "site") return "relation_domain";
    if (thermalServices.has(wanted) && thermalServices.has(service(from.node)) && service(from.node) !== wanted) return "service";
  } else if (relation === "residual") {
    if (from.node.level !== "residual" || from.level !== "end_use" || to.level !== "carrier" || fromDomain !== "site" || toDomain !== "site" ||
      token(from.node.basis) !== "residual" || token(link.basis) !== "residual" || number(from.node.signedValue) < 0) return "residual";
  } else return "relation";
  if (!unitMatches(link.fromUnit, fromDomain) || !unitMatches(link.toUnit, toDomain)) return "unit";
  const fromValue = number(link.fromValue), toValue = number(link.toValue);
  if (!(fromValue > 0) || !(toValue > 0)) return "value";
  if (fromDomain === toDomain && !close(fromValue, toValue)) return "same_domain_values";
  if (fromDomain !== toDomain && (!(fromValue / toValue > 0) || !Number.isFinite(fromValue / toValue))) return "ratio";
  return "";
}

function nodeGeometry(entry, layout, period) {
  const node = entry?.node, domain = domainForLevel(entry?.level);
  if (!node || !domain || token(node.scaleDomain) !== domain || !unitMatches(node.unit, domain) || !periodMatches(period, [node])) return null;
  if (![entry.x, entry.y, entry.width, entry.height, entry.anchorY].every((value) => typeof value === "number" && Number.isFinite(value)) || entry.width <= 0 || entry.height <= 0) return null;
  const value = number(node.value);
  if (value !== null && value < 0) return null;
  return {
    ...entry, domain, value,
    capacity: Math.max(0, entry.height - 4),
    barWidth: Math.max(0, Math.min(3, entry.width / 8, entry.x, layout.width - entry.x - entry.width)),
    incoming: 0, outgoing: 0,
  };
}

function ribbonPath(from, to) {
  const middle = (from.x + to.x) / 2;
  return `M${from.x},${from.y0} C${middle},${from.y0} ${middle},${to.y0} ${to.x},${to.y0} L${to.x},${to.y1} C${middle},${to.y1} ${middle},${from.y1} ${from.x},${from.y1} Z`;
}

function observationIdentity(link = {}) {
  return JSON.stringify([
    link.fromId, link.toId, token(link.relation), number(link.fromValue), number(link.toValue),
    token(link.fromUnit), token(link.toUnit), token(link.serviceKind), token(link.period), token(link.zoneName),
    Array.isArray(link.sourceIds) ? [...link.sourceIds].sort() : [],
  ]);
}

/**
 * Quantitative overlay for EPATH142's immutable label slots. Every domain has
 * one linear coefficient; only its output range is calibrated/shrunk so valid
 * conversion taper direction agrees with From/To. Pixel ratios are not COPs.
 */
export function energyPathRibbons(layout = {}, options = {}) {
  const period = token(options?.period) || "annual";
  const validPeriod = period === "annual" || /^m([1-9]|1[0-2])$/.test(period);
  const excluded = [];
  const nodes = validPeriod ? (layout.nodes || []).map((entry) => nodeGeometry(entry, layout, period)).filter(Boolean) : [];
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const identities = new Map(), conflicting = new Set();
  for (const item of layout.links || []) {
    const id = item?.link?.id;
    if (!id) continue;
    const identity = observationIdentity(item.link);
    if (identities.has(id) && identities.get(id) !== identity) conflicting.add(id);
    else identities.set(id, identity);
  }
  for (const id of conflicting) excluded.push({ id, reason: "conflicting_duplicate" });
  const candidates = [];
  for (const item of layout.links || []) {
    if (conflicting.has(item?.link?.id)) continue;
    const from = byID.get(item.fromId), to = byID.get(item.toId);
    const reason = validPeriod ? validateLink(item, from, to, period) : "period";
    if (reason) { excluded.push({ id: item.id || "", reason }); continue; }
    candidates.push({ item, link: item.link, from, to, fromValue: number(item.link.fromValue), toValue: number(item.link.toValue) });
  }
  const groups = new Map();
  for (const candidate of candidates) groups.set(candidate.link.id, [...(groups.get(candidate.link.id) || []), candidate]);
  let valid = [];
  for (const [id, group] of groups) {
    const first = group[0];
    if (group.some((item) => item.link.fromId !== first.link.fromId || item.link.toId !== first.link.toId ||
      item.link.relation !== first.link.relation || item.fromValue !== first.fromValue || item.toValue !== first.toValue ||
      token(item.link.serviceKind) !== token(first.link.serviceKind))) {
      excluded.push({ id, reason: "conflicting_duplicate" }); continue;
    }
    if (group.length > 1) excluded.push({ id, reason: "duplicate" });
    valid.push(first);
  }
  for (const item of valid) {
    item.from.outgoing += item.fromValue;
    item.to.incoming += item.toValue;
  }
  const overflow = new Set(nodes.filter((node) => !Number.isFinite(node.incoming) || !Number.isFinite(node.outgoing)).map((node) => node.id));
  if (overflow.size) {
    valid = valid.filter((item) => {
      if (!overflow.has(item.from.id) && !overflow.has(item.to.id)) return true;
      excluded.push({ id: item.link.id, reason: "aggregate_overflow" }); return false;
    });
    for (const node of nodes) { node.incoming = 0; node.outgoing = 0; }
    for (const item of valid) { item.from.outgoing += item.fromValue; item.to.incoming += item.toValue; }
  }
  const scales = {};
  for (const domain of ["thermal", "site"]) {
    const selected = nodes.filter((node) => node.domain === domain);
    const maxValue = Math.max(0, ...selected.map((node) => node.value ?? 0));
    const capacityMaxValue = Math.max(0, ...selected.map((node) => Math.max(node.value ?? 0, node.incoming, node.outgoing)));
    const constrained = selected.filter((node) => Math.max(node.value ?? 0, node.incoming, node.outgoing) > 0);
    const fit = constrained.length ? Math.min(...constrained.map((node) => Math.min(Number.MAX_VALUE, node.capacity / Math.max(node.value ?? 0, node.incoming, node.outgoing)))) : 0;
    scales[domain] = { maxValue, capacityMaxValue, fitPixelsPerKWh: Number.isFinite(fit) ? fit : 0, pixelsPerKWh: Number.isFinite(fit) ? fit : 0, kWhPerPixel: null };
  }
  const a0 = scales.thermal.pixelsPerKWh, b0 = scales.site.pixelsPerKWh;
  if (a0 > 0 && b0 > 0) {
    const quotients = valid.filter((item) => token(item.link.relation) === "load_to_end_use").map((item) => item.fromValue / item.toValue);
    if (quotients.length) {
      const lower = Math.max(0, ...quotients.filter((ratio) => ratio > 1).map((ratio) => 1 / ratio));
      const upper = Math.min(Infinity, ...quotients.filter((ratio) => ratio < 1).map((ratio) => 1 / ratio));
      const natural = Math.max(Number.MIN_VALUE, Math.min(Number.MAX_VALUE, a0 / b0));
      const ratio = quotients.some((ratio) => ratio === 1) ? 1 : Math.max(Math.sqrt(lower), Math.min(Math.sqrt(upper), natural));
      const a = Math.min(a0, ratio * b0), b = Math.min(b0, a / ratio);
      scales.thermal.pixelsPerKWh = a;
      scales.site.pixelsPerKWh = b;
    }
  }
  for (const scale of Object.values(scales)) {
    const inverse = scale.pixelsPerKWh > 0 ? 1 / scale.pixelsPerKWh : NaN;
    scale.kWhPerPixel = Number.isFinite(inverse) ? inverse : null;
  }
  const bars = [], portExtents = [], starts = new Map(), offsets = new Map();
  for (const entry of nodes) {
    const coefficient = scales[entry.domain].pixelsPerKWh;
    const sides = entry.level === "driver" || entry.level === "end_use" && entry.lane === "direct" ? ["outgoing"] : entry.level === "carrier" ? ["incoming"] : ["incoming", "outgoing"];
    for (const side of sides) {
      if (entry.value !== null) bars.push({
        nodeId: entry.id, side, x: side === "incoming" ? entry.x - entry.barWidth : entry.x + entry.width,
        y: entry.anchorY - entry.value * coefficient / 2, width: entry.barWidth, height: entry.value * coefficient,
        value: entry.value, domain: entry.domain, node: entry.node,
      });
    }
    for (const side of ["incoming", "outgoing"]) {
      const totalValue = entry[side], height = totalValue * coefficient, y = entry.anchorY - height / 2;
      starts.set(`${entry.id}|${side}`, y); offsets.set(`${entry.id}|${side}`, 0);
      if (totalValue > 0) portExtents.push({ nodeId: entry.id, side, y, height, totalValue, overmapped: entry.value !== null && totalValue > entry.value && !close(totalValue, entry.value) });
    }
  }
  const ribbons = [];
  for (const item of valid) {
    const { from, to, link, fromValue, toValue } = item;
    const fromWidth = fromValue * scales[from.domain].pixelsPerKWh, toWidth = toValue * scales[to.domain].pixelsPerKWh;
    if (!(fromWidth > 0) || !(toWidth > 0) || !Number.isFinite(fromWidth) || !Number.isFinite(toWidth)) { excluded.push({ id: link.id, reason: "capacity" }); continue; }
    const fromKey = `${from.id}|outgoing`, toKey = `${to.id}|incoming`;
    const fromY = starts.get(fromKey) + offsets.get(fromKey), toY = starts.get(toKey) + offsets.get(toKey);
    const fromPort = { x: from.x + from.width + from.barWidth, y0: fromY, y1: fromY + fromWidth };
    const toPort = { x: to.x - to.barWidth, y0: toY, y1: toY + toWidth };
    offsets.set(fromKey, offsets.get(fromKey) + fromWidth); offsets.set(toKey, offsets.get(toKey) + toWidth);
    ribbons.push({
      id: link.id, fromId: from.id, toId: to.id, relation: link.relation,
      domain: from.domain === to.domain ? from.domain : "conversion",
      fromValue, toValue, fromWidth, toWidth, fromPort, toPort, path: ribbonPath(fromPort, toPort), link,
      ratioAnchor: { x: (fromPort.x + toPort.x) / 2, y: (fromPort.y0 + fromPort.y1 + toPort.y0 + toPort.y1) / 4 },
      gap: { x0: fromPort.x, x1: toPort.x, width: toPort.x - fromPort.x },
    });
  }
  return { scales, bars, ribbons, portExtents, excluded };
}
