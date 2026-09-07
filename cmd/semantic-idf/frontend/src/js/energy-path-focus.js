const token = (value) => String(value ?? "").trim().toLowerCase();
const LEVELS = ["driver", "load", "end_use", "carrier"];
const DRIVER_ORDER = [
  "surface.exterior_walls", "surface.roofs", "surface.ground_floors", "surface.windows_doors", "surface.interzone",
  "air.infiltration", "air.mechanical_ventilation", "air.interzone",
  "internal.people", "internal.lighting", "internal.equipment", "interzone.transfer", "balance.storage_other",
];
const END_USE_ORDER = ["cooling", "heating", "fans_pumps", "hvac_auxiliaries", "lighting", "equipment", "water_systems", "refrigeration", "other"];
const CARRIER_ORDER = [
  "electricity", "natural_gas", "district_cooling", "district_heating", "steam", "propane", "fuel_oil_1", "fuel_oil_2",
  "coal", "diesel", "gasoline", "other_fuel_1", "other_fuel_2", "water",
];
const CARRIER_ALIASES = {
  gas: "natural_gas", naturalgas: "natural_gas", districtcooling: "district_cooling", districtheating: "district_heating",
  districtheatingwater: "district_heating", districtheatingsteam: "steam", fueloil1: "fuel_oil_1", fueloilno1: "fuel_oil_1",
  fueloil2: "fuel_oil_2", fueloilno2: "fuel_oil_2", otherfuel1: "other_fuel_1", otherfuel2: "other_fuel_2",
};
const END_USE_ALIASES = { fans: "fans_pumps", pumps: "fans_pumps", heat_rejection: "hvac_auxiliaries", heat_recovery: "hvac_auxiliaries", humidification: "hvac_auxiliaries" };
const rank = (order, value) => { const found = order.indexOf(value); return found < 0 ? order.length : found; };
const nodeLevel = (node) => token(node?.presentationLevel || node?.level);
const compareID = (left, right) => { const a = String(left?.id ?? ""), b = String(right?.id ?? ""); return a < b ? -1 : a > b ? 1 : 0; };

function taxonomyRank(level, node) {
  if (level === "driver") return rank(DRIVER_ORDER, token(node.driverCategory) === "internal.other" ? "balance.storage_other" : token(node.driverCategory));
  if (level === "load") return rank(["cooling", "heating"], token(node.serviceKind));
  if (level === "end_use") {
    if (token(node.level) === "residual" && token(node.presentationKind) === "unclassified_energy") return END_USE_ORDER.length + 1;
    const category = token(node.endUse);
    return rank(END_USE_ORDER, END_USE_ALIASES[category] || category);
  }
  if (level === "carrier") {
    const carrier = token(node.carrier);
    return rank(CARRIER_ORDER, CARRIER_ALIASES[carrier.replace(/[^a-z0-9]/g, "")] || carrier);
  }
  return 0;
}

/** Fixed presentation order. Neither reported values nor localized labels are ordering evidence. */
export function energyPathCompareNodes(stage, left = {}, right = {}) {
  const a = left?.node || left || {}, b = right?.node || right || {};
  const requested = token(typeof stage === "object" ? stage?.level : stage);
  const leftLevel = requested || nodeLevel(left), rightLevel = requested || nodeLevel(right);
  const levelDifference = rank(LEVELS, leftLevel) - rank(LEVELS, rightLevel);
  if (levelDifference) return levelDifference;
  return taxonomyRank(leftLevel, a) - taxonomyRank(rightLevel, b) || compareID(a, b);
}

/** Stack both ribbon ends in the same fixed visible-node order, not link-ID or value order. */
export function energyPathOrderLinks(nodes = [], links = []) {
  const nodeOrder = new Map();
  for (const node of Array.isArray(nodes) ? nodes : []) {
    if (node?.id && !nodeOrder.has(node.id)) nodeOrder.set(node.id, nodeOrder.size);
  }
  const index = (id) => nodeOrder.get(id) ?? nodeOrder.size;
  return [...(Array.isArray(links) ? links : [])].sort((left, right) =>
    index(left?.fromId) - index(right?.fromId) || index(left?.toId) - index(right?.toId) || compareID(left, right));
}

function inactiveFocus() {
  return { active: false, selectedNodeID: "", selectedLinkID: "", nodeIDs: new Set(), linkIDs: new Set(), counterpartIDs: new Set() };
}

/**
 * Focus only the visible, quantitatively validated graph. Ancestors and
 * descendants are walked independently: arriving at a shared source must not
 * turn around and highlight unrelated energy demand. Correspondence is not flow.
 */
export function energyPathFocus(layout = {}, drawing = {}, selection = "", counterpartIDs = []) {
  const nodes = new Map((Array.isArray(layout?.nodes) ? layout.nodes : [])
    .filter((node) => node?.id && LEVELS.includes(nodeLevel(node)))
    .map((node) => [node.id, node]));
  const links = new Map((Array.isArray(drawing?.ribbons) ? drawing.ribbons : [])
    .filter((link) => link?.id && nodes.has(link.fromId) && nodes.has(link.toId))
    .map((link) => [link.id, link]));
  const selectedNode = nodes.get(selection), selectedLink = links.get(selection);
  // A malformed shared node/link identity is ambiguous, not a first-match choice.
  if ((!selectedNode && !selectedLink) || (selectedNode && selectedLink)) return inactiveFocus();

  const incoming = new Map(), outgoing = new Map();
  for (const link of links.values()) {
    incoming.set(link.toId, [...(incoming.get(link.toId) || []), link]);
    outgoing.set(link.fromId, [...(outgoing.get(link.fromId) || []), link]);
  }
  const nodeIDs = new Set(), linkIDs = new Set();
  const walk = (start, adjacency, endpoint) => {
    const visited = new Set(), pending = [start];
    while (pending.length) {
      const id = pending.pop();
      if (visited.has(id)) continue;
      visited.add(id); nodeIDs.add(id);
      for (const link of adjacency.get(id) || []) {
        linkIDs.add(link.id);
        pending.push(link[endpoint]);
      }
    }
  };
  if (selectedNode) {
    walk(selectedNode.id, incoming, "fromId");
    walk(selectedNode.id, outgoing, "toId");
  } else {
    linkIDs.add(selectedLink.id);
    walk(selectedLink.fromId, incoming, "fromId");
    walk(selectedLink.toId, outgoing, "toId");
  }
  const counterpartValues = counterpartIDs instanceof Set || Array.isArray(counterpartIDs) ? [...counterpartIDs] : [];
  return {
    active: true,
    selectedNodeID: selectedNode?.id || "",
    selectedLinkID: selectedLink?.id || "",
    nodeIDs, linkIDs,
    counterpartIDs: new Set(selectedNode ? counterpartValues.filter((id) => id !== selectedNode.id && nodes.has(id)) : []),
  };
}
