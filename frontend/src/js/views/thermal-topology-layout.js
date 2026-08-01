import {
  normalizeThermalTopologyAreaBasis,
  normalizeThermalTopologyGraphLevel,
  normalizeThermalTopologyLayout,
  normalizeThermalTopologyScope,
} from "../state.js";

export function thermalTopologyLayoutCacheKey(geometry, options = {}) {
  const topology = geometry?.topology || {};
  return [
    topology.schema || "thermal-topology",
    normalizeThermalTopologyGraphLevel(options.graphLevel),
    normalizeThermalTopologyLayout(options.layout),
    normalizeThermalTopologyScope(options.scope),
    options.storyIndex ?? "all",
    options.selectedEntityId || "",
    Number(options.neighborDepth) || 1,
    Boolean(options.showOpenings),
    Boolean(options.showAirCoupling),
    Boolean(options.expandExternalTargets),
  ].join("|");
}

export function createThermalTopologyLayoutModel(geometry, options = {}) {
  const topology = geometry?.topology || {};
  const areaBasis = normalizeThermalTopologyAreaBasis(options.areaBasis);
  return {
    cacheKey: thermalTopologyLayoutCacheKey(geometry, options),
    schema: topology.schema || "",
    graphLevel: normalizeThermalTopologyGraphLevel(options.graphLevel),
    layout: normalizeThermalTopologyLayout(options.layout),
    scope: normalizeThermalTopologyScope(options.scope),
    areaBasis,
    areaField: areaBasis === "physical" ? "physicalGrossArea" : "effectiveGrossArea",
    nodes: [...(topology.nodes || [])],
    connections: [...(topology.connections || [])],
    boundaries: [...(topology.boundaries || [])],
    openings: options.showOpenings ? [...(topology.openings || [])] : [],
    airCouplings: options.showAirCoupling ? [...(topology.airCouplings || [])] : [],
  };
}
