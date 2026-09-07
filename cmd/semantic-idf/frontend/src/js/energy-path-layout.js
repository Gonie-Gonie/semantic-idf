const LEVELS = Object.freeze(["driver", "load", "end_use", "carrier"]);
const token = (value) => String(value ?? "").trim().toLowerCase();
const dimension = (value, fallback) => typeof value === "number" && Number.isFinite(value) && value > 0 ? value : fallback;
const clamp = (value, low, high) => Math.max(low, Math.min(high, value));

function layoutLevel(node) {
  return token(node.presentationLevel || node.level);
}

function nodeLane(node, level) {
  if (level === "driver") return "thermal";
  if (level === "load") return "main";
  if (level === "carrier") return "shared";
  // The end-use category, not the allocation service, determines the lane.
  // Fans allocated to cooling remain direct end uses, never thermal loads.
  return ["cooling", "heating"].includes(token(node.endUse)) ? "main" : "direct";
}

function laneGeometry(height, mainCount, directCount) {
  if (!directCount) return { mainHeight: height, directY: height, directHeight: 0 };
  if (!mainCount) return { mainHeight: 0, directY: 0, directHeight: height };
  const gap = Math.min(16, height * 0.04);
  const mainNeed = mainCount * 52 + Math.max(0, mainCount - 1) * 8;
  const directNeed = directCount * 30 + Math.max(0, directCount - 1) * 4 + 20;
  const mainHeight = (height - gap) * clamp(mainNeed / (mainNeed + directNeed), 0.26, 0.55);
  const directY = mainHeight + gap;
  return { mainHeight, directY, directHeight: height - directY };
}

function placeRows(entries, column, top, available, preferredHeight, preferredGap) {
  if (!entries.length) return [];
  const count = entries.length;
  // Spend spare space on gaps only after reserving readable label rows. A
  // dense direct lane shrinks whitespace before shrinking its 30px cards.
  const gap = count > 1 ? Math.max(0, Math.min(preferredGap, (available - preferredHeight * count) / (count - 1))) : 0;
  const height = Math.max(0, Math.min(preferredHeight, (available - gap * (count - 1)) / count));
  const used = count * height + (count - 1) * gap;
  const start = top + Math.max(0, (available - used) / 2);
  // Reserve inter-column space for the future same-domain and conversion
  // ribbons. Labels remain HTML hit boxes, not quantitative energy widths.
  const width = Math.min(192, column.width * 0.74);
  const x = column.x + (column.width - width) / 2;
  return entries.map((entry, index) => {
    const y = start + index * (height + gap);
    const insetX = Math.min(6, width / 4), insetY = Math.min(4, height / 4);
    return {
      ...entry, x, y, width, height, anchorY: y + height / 2,
      labelBox: { x: x + insetX, y: y + insetY, width: width - 2 * insetX, height: height - 2 * insetY, maxLines: 2 },
    };
  });
}

function flowIsValid(link, from, to) {
  const relation = token(link.relation);
  if (relation === "driver_to_load") return from.level === "driver" && to.level === "load";
  if (relation === "load_to_end_use") return from.level === "load" && to.level === "end_use" && to.lane === "main";
  if (["end_use_to_carrier", "direct_end_use_to_carrier"].includes(relation)) return from.level === "end_use" && to.level === "carrier";
  return relation === "residual" && from.level === "end_use" && token(from.node.level) === "residual" && to.level === "carrier";
}

/**
 * Immutable presentation geometry only. Inputs have already passed canonical
 * scope/period/taxonomy filtering. There are no energy scales or ribbon widths
 * here; changing reported values cannot change this four-column layout.
 */
export function energyPathLayout(nodes = [], links = [], options = {}) {
  const width = dimension(options?.width, 1000), height = dimension(options?.height, 420);
  const columns = LEVELS.map((level, index) => ({ level, x: index * width / LEVELS.length, width: width / LEVELS.length }));
  const byLevel = new Map(columns.map((column) => [column.level, column]));
  const seen = new Set();
  const entries = (Array.isArray(nodes) ? nodes : []).flatMap((node) => {
    if (!node || typeof node !== "object" || !node.id || seen.has(node.id)) return [];
    const level = layoutLevel(node);
    if (!LEVELS.includes(level)) return [];
    seen.add(node.id);
    return [{ id: node.id, level, lane: nodeLane(node, level), node }];
  });
  const group = (level, lane) => entries.filter((entry) => entry.level === level && (!lane || entry.lane === lane));
  const mainCount = Math.max(group("load").length, group("end_use", "main").length);
  const direct = group("end_use", "direct");
  const { mainHeight, directY, directHeight } = laneGeometry(height, mainCount, direct.length);
  const directLabelHeight = direct.length ? Math.min(20, directHeight * 0.2) : 0;
  const positioned = [
    ...placeRows(group("driver"), byLevel.get("driver"), 0, height, 32, 3),
    ...placeRows(group("load"), byLevel.get("load"), 0, mainHeight, 52, 8),
    ...placeRows(group("end_use", "main"), byLevel.get("end_use"), 0, mainHeight, 52, 8),
    ...placeRows(direct, byLevel.get("end_use"), directY + directLabelHeight, directHeight - directLabelHeight, 30, 4),
    ...placeRows(group("carrier"), byLevel.get("carrier"), 0, height, 52, 12),
  ];
  const nodeByID = new Map(positioned.map((node) => [node.id, node]));
  const layoutLinks = (Array.isArray(links) ? links : []).flatMap((link) => {
    if (!link || typeof link !== "object") return [];
    const from = nodeByID.get(link.fromId), to = nodeByID.get(link.toId);
    if (!from || !to || !flowIsValid(link, from, to)) return [];
    return [{
      id: link.id || "", fromId: link.fromId, toId: link.toId, link,
      fromAnchor: { x: from.x + from.width, y: from.anchorY },
      toAnchor: { x: to.x, y: to.anchorY },
    }];
  });
  return {
    width, height, columns,
    lanes: [
      { id: "main", y: 0, height: mainHeight, startLevel: "load" },
      { id: "direct", y: directY, height: directHeight, startLevel: "end_use", labelHeight: directLabelHeight },
    ],
    nodes: positioned, links: layoutLinks, dividerX: byLevel.get("end_use").x,
  };
}
