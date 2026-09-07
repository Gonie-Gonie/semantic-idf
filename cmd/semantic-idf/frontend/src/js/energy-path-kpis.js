import { ENERGY_PATH_SUMMARY_SCHEMA_V2, energyPathSummaryKPIValues, isEnergyPathSummaryV2 } from "./energy-path-summary.js";

const token = (input) => String(input ?? "").trim().toLowerCase();
const numeric = (input) => (typeof input === "number" || typeof input === "string" && input.trim() !== "") && Number.isFinite(Number(input)) ? Number(input) : null;
const serviceFor = (item = {}) => {
  const explicit = token(item.serviceKind || item.endUse);
  return explicit || token(item.kind || item.id).match(/(?:^|[._:-])(cooling|heating)(?:$|[._:-])/)?.[1] || "";
};

function kWhUnit(unit, domain) {
  const normalized = token(unit).replace(/[()[\]_]/g, " ").replace(/\s+/g, " ");
  return normalized === "kwh" || normalized === `kwh ${domain}`;
}

function periodMatches(period, ...items) {
  const known = items.map((item) => token(item?.period)).filter(Boolean);
  return known.every((item) => item === period) && (period === "annual" || known.length > 0);
}

function targetsFor(item, nodes) {
  const level = item.id === "total_site_energy" ? "carrier" : "load";
  const service = item.id === "cooling_load" ? "cooling" : item.id === "heating_load" ? "heating" : "";
  const targets = new Map();
  let complete = true;
  for (const [index, id] of (item.nodeIds || []).entries()) {
    const matching = nodes.filter((node) => node?.level === level &&
      (node.id === id || (node.originalNodeIds || []).includes(id)) &&
      (!service || serviceFor(node) === service));
    // A summary identity must identify an actual graph node, not whichever
    // same-service node happened to be first in the current presentation.
    if (!id || matching.length !== 1) {
      // Canonical graphs may omit zero-width nodes; an explicitly reported
      // zero stays known without acquiring an invented clickable target.
      if (matching.length > 1 || numeric(item.nodeValues?.[index]) !== 0) complete = false;
      continue;
    }
    const node = matching[0];
    targets.set(node.id, { id: node.id, label: node.label || node.id, serviceKind: service });
  }
  return { targets: [...targets.values()], complete };
}

function conversionRatio(graph, service, period) {
  if (!["cooling", "heating"].includes(service)) return null;
  const nodes = Array.isArray(graph.nodes) ? graph.nodes : [];
  const links = Array.isArray(graph.links) ? graph.links : [];
  const byID = new Map(nodes.filter((node) => node?.id).map((node) => [node.id, node]));
  const observed = new Map();
  for (const link of links) {
    if (token(link?.relation) !== "load_to_end_use") continue;
    if (!(Array.isArray(link.sourceIds) && link.sourceIds.some((id) => typeof id === "string" && id.trim() !== ""))) continue;
    const from = byID.get(link.fromId), to = byID.get(link.toId);
    if (from?.level !== "load" || to?.level !== "end_use" ||
      token(from.scaleDomain) !== "thermal" || token(to.scaleDomain) !== "site" ||
      serviceFor(from) !== service || serviceFor(to) !== service ||
      (link.serviceKind && token(link.serviceKind) !== service) ||
      token(from.zoneName) !== token(to.zoneName) ||
      !periodMatches(period, link, from, to) ||
      !kWhUnit(from.unit, "thermal") || !kWhUnit(to.unit, "site") ||
      !kWhUnit(link.fromUnit, "thermal") || !kWhUnit(link.toUnit, "site")) continue;
    const fromValue = numeric(link.fromValue), toValue = numeric(link.toValue);
    if (!(fromValue > 0) || !(toValue > 0)) continue;
    const key = link.id || `${link.fromId}|${link.toId}`;
    if (observed.has(key)) {
      const previous = observed.get(key);
      if (previous.fromId !== link.fromId || previous.toId !== link.toId || previous.fromValue !== fromValue || previous.toValue !== toValue) return null;
      continue;
    }
    observed.set(key, { id: link.id || "", fromId: link.fromId, toId: link.toId, fromValue, toValue });
  }
  if (!observed.size) return null;
  const values = [...observed.values()];
  const fromValue = values.reduce((sum, item) => sum + item.fromValue, 0);
  const toValue = values.reduce((sum, item) => sum + item.toValue, 0);
  if (!Number.isFinite(fromValue) || !Number.isFinite(toValue) || !Number.isFinite(fromValue / toValue)) return null;
  const endpointTotal = (level, domain) => {
    const selected = nodes.filter((node) => node.level === level && token(node.scaleDomain) === domain &&
      serviceFor(node) === service && periodMatches(period, node) && kWhUnit(node.unit, domain));
    if (!selected.length || selected.some((node) => numeric(node.value) === null || numeric(node.value) < 0)) return null;
    return selected.reduce((sum, node) => sum + numeric(node.value), 0);
  };
  const loadTotal = endpointTotal("load", "thermal"), energyTotal = endpointTotal("end_use", "site");
  const close = (left, right) => right !== null && Math.abs(left - right) <= 1e-6 * Math.max(1, Math.abs(right));
  return {
    value: fromValue / toValue, fromValue, toValue,
    linkIds: values.map((item) => item.id).filter(Boolean),
    // The displayed ratio describes only paired observations. Node totals
    // establish a partial-coverage label; they never replace either side.
    partial: !close(fromValue, loadTotal) || !close(toValue, energyTotal),
  };
}

function coverageBoundaries(quality, knownZoneOnly) {
  const definitions = [
    { id: "driver_to_load", label: "Drivers → loads", value: "driverToLoadClosedPct", status: "driverToLoadStatus" },
    { id: "end_use_to_carrier", label: "End uses → sources", value: "endUseToCarrierClosedPct", status: "endUseToCarrierStatus" },
  ];
  return definitions.map((definition) => {
    let status = token(quality?.[definition.status]) || "unavailable";
    let value = numeric(quality?.[definition.value]);
    if (knownZoneOnly && definition.id === "end_use_to_carrier") status = "unavailable";
    if (!["complete", "partial", "overmapped"].includes(status)) value = null;
    else if (value === null || value < 0 || value > 100) { value = null; status = "unavailable"; }
    return { id: definition.id, label: definition.label, value, status };
  });
}

/** Four compact, scope/period-invariant cards; service changes emphasis only. */
export function energyPathKPIItems(summary = {}, graph = {}, options = {}) {
  const canonical = isEnergyPathSummaryV2(summary);
  const base = energyPathSummaryKPIValues(canonical ? summary : { schema: ENERGY_PATH_SUMMARY_SCHEMA_V2 });
  const nodes = Array.isArray(graph?.nodes) ? graph.nodes : [];
  const service = token(options.service) || "all";
  const period = token(options.period) || "annual";
  const quality = options.quality || summary?.quality || {};
  const ratioStatus = token(quality.ratios?.status);
  const ratio = canonical && !["unavailable", "not_requested", "not_applicable", "missing"].includes(ratioStatus)
    ? conversionRatio(graph || {}, service, period) : null;
  return base.map((item) => {
    const { nodeIds, nodeValues, ...value } = item;
    const emphasized = item.id === `${service}_load`;
    const targeting = targetsFor(item, nodes);
    return {
      ...value,
      // Summary-only callers retain summary values. Once a graph is supplied,
      // do not present a complete total containing an unvalidated contributor.
      value: Array.isArray(graph?.nodes) && !targeting.complete ? null : value.value,
      targets: item.id === "coverage" ? [] : targeting.targets,
      emphasized,
      ...(emphasized && ratio ? { ratio } : {}),
      ...(item.id === "coverage" ? { coverage: { boundaries: coverageBoundaries(quality, options.knownZoneOnly === true) } } : {}),
    };
  });
}
