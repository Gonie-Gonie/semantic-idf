import { t } from "./i18n.js";

const finite = (value) => typeof value === "number" && Number.isFinite(value);
const components = Object.freeze({
  air_storage: ["AirStorage", "Zone-air storage"],
  surface_balance: ["SurfaceBalance", "Surface heat-balance difference"],
  outdoor_air_balance: ["OutdoorAirBalance", "Outdoor-air balance difference"],
  interzone_balance: ["InterzoneBalance", "Interzone balance difference"],
  internal_gain_balance: ["InternalBalance", "Internal-gain balance difference"],
  other_internal_gains: ["OtherInternalGains", "Other internal gains"],
  surface_exchange: ["SurfaceExchange", "Other surface heat exchange"],
  unresolved_balance: ["UnresolvedBalance", "Unresolved heat balance"],
});

export function energyPathDriverPresentationLabel(category, fallback = category) {
  if (category === "internal.gains") return t("simulation.energyPathDriverInternalGains", {}, "Internal gains");
  if (category === "balance.storage_other") return t("simulation.energyPathDriverUnresolvedBalance", {}, "Unresolved heat balance");
  const definition = components[String(category || "").replace(/^balance\./, "")];
  return definition ? t(`simulation.energyPathDriver${definition[0]}`, {}, definition[1]) : fallback;
}

// Direction is a backend observation contract, never inferred from a service,
// an annual net sign, a label, or an opaque source ID.
export function energyPathHeatDirection(node = {}) {
  return ["gain", "loss"].includes(node.heatDirection) ? node.heatDirection : "";
}

export function energyPathDirectionalLabel(node = {}, fallback = node.label || node.kind || "") {
  const direction = energyPathHeatDirection(node);
  if (!direction) return fallback;
  if (node.driverCategory === "balance.air_storage") {
    return direction === "gain"
      ? t("simulation.energyPathAirHeatRelease", {}, "Zone-air heat release")
      : t("simulation.energyPathAirHeatStorage", {}, "Zone-air heat storage");
  }
  const namedCategories = ["surface.exterior_walls", "surface.roofs", "surface.ground_floors", "surface.windows_doors", "surface.interzone",
    "air.infiltration", "air.mechanical_ventilation", "air.interzone", "interzone.transfer"];
  if (node.level === "driver" && namedCategories.includes(node.driverCategory)) {
    fallback = t(`simulation.energyPathDriverCategory.${node.driverCategory}`, {}, fallback);
  }
  return t(`simulation.energyPathHeat${direction === "gain" ? "Gain" : "Loss"}Label`, { label: fallback },
    `${fallback} (${direction})`);
}

export function energyPathGrossHeatValue(node = {}) {
  const direction = energyPathHeatDirection(node);
  return direction && finite(node.effectiveValue) && node.effectiveValue >= 0
    ? node.effectiveValue : null;
}

/** Project the backend's additive, within-parent allocation breakdown. Native
 * parent accounting and opaque source identities remain in the saved result.
 * Older results without evidence retain one explicitly unresolved component. */
export function energyPathProjectDriverBreakdown(nodes = [], links = []) {
  const nodeByID = new Map(nodes.map((node) => [node.id, node]));
  const children = new Map();
  const projectedNodes = [];
  for (const node of nodes) {
    if (node.level === "input" && ["input.internal_gains", "input.solar_absorbed", "input.surface_storage"].includes(node.kind)) continue;
    const parts = node.driverBreakdown;
    const valid = node.level === "driver" && node.driverCategory === "balance.storage_other" &&
      finite(node.value) && node.value >= 0 && Array.isArray(parts) && parts.length > 0 &&
      parts.every((part) => components[part.component] && finite(part.value) && part.value >= 0 &&
        finite(part.rawValue) && part.rawValue >= 0 && finite(part.effectiveValue) && part.effectiveValue >= 0 &&
        Array.isArray(part.sourceIds) && part.sourceIds.length && part.sourceIds.every((id) => node.sourceIds?.includes(id))) &&
      new Set(parts.map((part) => part.component)).size === parts.length &&
      Math.abs(parts.reduce((sum, part) => sum + part.value, 0) - node.value) <= Math.max(1e-8, node.value * 1e-10) &&
      ["rawValue", "effectiveValue"].every((field) => !finite(node[field]) ||
        Math.abs(parts.reduce((sum, part) => sum + part[field], 0) - node[field]) <= Math.max(1e-8, node[field] * 1e-10)) &&
      !links.some((link) => (link.fromId === node.id || link.toId === node.id) &&
        (!["driver_to_load", "input_to_driver"].includes(link.relation) || !finite(link.fromValue) || !finite(link.toValue))) &&
      links.filter((link) => link.fromId === node.id && link.relation === "driver_to_load")
        .every((link) => link.fromValue >= 0 && link.toValue >= 0 && (node.value > 0 || link.fromValue === 0 && link.toValue === 0));
    if (!valid) {
      projectedNodes.push(node);
      continue;
    }
    const members = parts.map((part) => {
      const { groupedMembers, originalNodeIds, driverBreakdown, ...parent } = node;
      return { ...parent, id: `${node.id}.component.${part.component}`, originalDriverId: node.id,
        kind: `driver.balance.${part.component}`, driverCategory: `balance.${part.component}`,
        label: energyPathDriverPresentationLabel(`balance.${part.component}`, part.label),
        value: part.value, allocatedValue: part.value, displayValue: part.value,
        signedValue: (energyPathHeatDirection(node) === "loss" || !energyPathHeatDirection(node) && (node.signedValue < 0 || node.serviceKind === "heating") ? -1 : 1) * part.effectiveValue,
        rawValue: part.rawValue, effectiveValue: part.effectiveValue,
        sourceIds: [...(part.sourceIds || [])], basis: node.basis,
      };
    });
    children.set(node.id, members);
    projectedNodes.push(...members);
  }
  const visibleIDs = new Set(projectedNodes.map((node) => node.id));
  const projectedLinks = [];
  for (const link of links) {
    const members = children.get(link.fromId);
    if (members && link.relation === "driver_to_load") {
      const total = members.reduce((sum, member) => sum + member.value, 0);
      let fromUsed = 0, toUsed = 0;
      members.forEach((member, index) => {
        const last = index === members.length - 1;
        const fromValue = last ? link.fromValue - fromUsed : total ? link.fromValue * member.value / total : 0;
        const toValue = last ? link.toValue - toUsed : total ? link.toValue * member.value / total : 0;
        fromUsed += fromValue; toUsed += toValue;
        const { groupedMembers, originalLinkIds, ...original } = link;
        projectedLinks.push({ ...original, id: `${link.id}.component.${member.driverCategory}`,
          fromId: member.id, fromValue, toValue,
          sourceIds: [...new Set([...member.sourceIds, ...(nodeByID.get(link.toId)?.sourceIds || [])])] });
      });
    } else if (children.has(link.toId) && link.relation === "input_to_driver") {
      for (const member of children.get(link.toId)) {
        // Buried-wall inside-face convection is classified as surface_exchange.
        // Sharing its residual parent does not associate the Ground boundary
        // with air storage or heat-balance reconciliation components.
        if (nodeByID.get(link.fromId)?.kind === "input.ground_conduction" && member.driverCategory !== "balance.surface_exchange") continue;
        projectedLinks.push({ ...link,
          id: `${link.id}.component.${member.driverCategory}`, toId: member.id, toValue: member.value });
      }
    } else if (visibleIDs.has(link.fromId) && visibleIDs.has(link.toId)) {
      projectedLinks.push(link);
    }
  }
  return { nodes: projectedNodes, links: projectedLinks };
}
