const token = (value) => String(value ?? "").trim().toLowerCase();
const number = (value) => (typeof value === "number" || typeof value === "string" && value.trim() !== "") && Number.isFinite(Number(value)) ? Number(value) : null;
const ALLOCATION_BASES = new Set(["allocated", "heat_balance_share", "service_path_allocation", "zone_load_allocation"]);
const COMBUSTION_CARRIERS = new Set(["propane", "fuel_oil_1", "fuel_oil_2", "coal", "diesel", "gasoline", "other_fuel_1", "other_fuel_2"]);
const CARRIER_ALIASES = Object.freeze({
  electricity: "electricity", gas: "natural_gas", naturalgas: "natural_gas",
  districtcooling: "district_cooling", districtheating: "district_heating", districtheatingwater: "district_heating",
  steam: "steam", districtheatingsteam: "steam", propane: "propane",
  fueloil1: "fuel_oil_1", fueloilno1: "fuel_oil_1", fueloil2: "fuel_oil_2", fueloilno2: "fuel_oil_2",
  coal: "coal", diesel: "diesel", gasoline: "gasoline", otherfuel1: "other_fuel_1", otherfuel2: "other_fuel_2",
});

function allocationEvidence(item = {}) {
  return item.allocationApplied === true || ALLOCATION_BASES.has(token(item.basis)) ||
    (Array.isArray(item.badges) && item.badges.some((badge) => token(badge) === "allocated"));
}

function allocationStates(item, ancestors = new Set()) {
  if (!item || typeof item !== "object" || ancestors.has(item)) return [null];
  const members = Array.isArray(item.groupedMembers) ? item.groupedMembers : [];
  if (!members.length) return [allocationEvidence(item)];
  const path = new Set(ancestors).add(item);
  return members.flatMap((member) => allocationStates(member, path));
}

function allocationKind(item = {}) {
  // Member snapshots outrank sticky presentation flags. A merged node can
  // contain both reported and allocated contributions without all being allocated.
  const states = allocationStates(item);
  const count = states.filter((state) => state === true).length;
  return count === states.length ? "allocated" : count > 0 ? "includes_allocated" : "";
}

function carrierColor(carrier) {
  const key = CARRIER_ALIASES[token(carrier).replace(/[^a-z0-9]/g, "")] || "";
  if (["electricity", "natural_gas", "district_cooling", "district_heating", "steam"].includes(key)) return `carrier-${key.replaceAll("_", "-")}`;
  return COMBUSTION_CARRIERS.has(key) ? "carrier-combustion" : "neutral";
}

function qualifiedResidual(node = {}) {
  return token(node.level) === "residual" && token(node.presentationLevel) === "end_use" &&
    token(node.presentationKind) === "unclassified_energy" && token(node.basis) === "residual" &&
    token(node.scaleDomain) === "site" && number(node.value) > 0 && !(number(node.signedValue) < 0) &&
    Array.isArray(node.badges) && node.badges.some((badge) => token(badge) === "unclassified_energy");
}

function nodeColor(node) {
  const level = token(node.level);
  if (level === "driver") {
    const category = token(node.driverCategory);
    if (category.startsWith("surface.")) return "driver-envelope";
    if (category.startsWith("air.") || category === "interzone.transfer") return "driver-air";
    if (category.startsWith("internal.") && category !== "internal.other") return "driver-internal";
    return "neutral";
  }
  if (level === "load") return ["cooling", "heating"].includes(token(node.serviceKind)) ? token(node.serviceKind) : "neutral";
  if (level === "end_use") {
    // Cooling-tagged fans are still direct/auxiliary consumption, not cooling equipment.
    const endUse = token(node.endUse);
    if (["cooling", "heating"].includes(endUse)) return endUse;
    if (["fans", "pumps", "fans_pumps", "hvac_auxiliaries", "heat_rejection", "heat_recovery", "humidification"].includes(endUse)) return "direct-auxiliary";
    if (endUse === "lighting") return "direct-lighting";
    if (endUse === "refrigeration") return "direct-refrigeration";
    return "neutral";
  }
  return level === "carrier" ? carrierColor(node.carrier) : "neutral";
}

/** Semantic paint only: no values, geometry, source records, or order are changed. */
export function energyPathNodeAppearance(node = {}) {
  node = node && typeof node === "object" ? node : {};
  const residual = qualifiedResidual(node);
  const allocationLabelKind = residual ? "" : allocationKind(node);
  return { colorKey: residual ? "residual" : nodeColor(node), allocated: Boolean(allocationLabelKind), residual, allocationLabelKind };
}

export function energyPathLinkAppearance(link = {}, nodes = []) {
  link = link && typeof link === "object" ? link : {};
  const nodeByID = new Map((Array.isArray(nodes) ? nodes : []).filter((node) => node && typeof node === "object").map((node) => [node.id, node]));
  const from = nodeByID.get(link.fromId), to = nodeByID.get(link.toId);
  const relation = token(link.relation);
  const residual = relation === "residual" && token(link.basis) === "residual" &&
    qualifiedResidual(from) && token(to?.level) === "carrier" && number(link.fromValue) > 0 && number(link.toValue) > 0;
  const allocationLabelKind = residual ? "" : allocationKind(link);
  let colorKey = "neutral";
  if (residual) colorKey = "residual";
  else if (["driver_to_load", "load_to_end_use"].includes(relation)) {
    colorKey = ["cooling", "heating"].includes(token(link.serviceKind)) ? token(link.serviceKind) : "neutral";
  } else if (["end_use_to_carrier", "direct_end_use_to_carrier"].includes(relation) && token(to?.level) === "carrier") {
    colorKey = carrierColor(to.carrier);
  }
  return { colorKey, allocated: Boolean(allocationLabelKind), residual, allocationLabelKind };
}
