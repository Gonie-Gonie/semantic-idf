import { buildHVACLoopDiagramLayout } from "./hvac-views.js";

const finite = Number.isFinite;
const token = (value) => String(value || "").trim().toLowerCase();
const clamp = (value, low, high) => Math.max(low, Math.min(high, value));
const sides = ["supply", "demand"];
const sideLayout = (layout, side) => side === "supply" ? layout.supplyLayout : layout.demandLayout;
const box = (x, y, width, height, meta = {}) => ({ x, y, width, height, ...meta });
const point = (x, y) => ({ x, y });

function overlap(a, b) {
  const width = Math.min(a.x + a.width, b.x + b.width) - Math.max(a.x, b.x);
  const height = Math.min(a.y + a.height, b.y + b.height) - Math.max(a.y, b.y);
  return width > .1 && height > .1 ? width * height : 0;
}

function obstacles(layout, loop, fontSize) {
  const result = layout.pipeSegments.map((pipe) => box(Math.min(pipe.x1, pipe.x2) - 6, Math.min(pipe.y1, pipe.y2) - 6,
    Math.abs(pipe.x2 - pipe.x1) + 12, Math.abs(pipe.y2 - pipe.y1) + 12, { kind: "pipe", pipe, side: pipe.side, rowIndex: pipe.rowIndex }));
  for (const anchor of layout.anchors) {
    if (anchor.kind === "node") result.push(box(anchor.x - 11, anchor.y - 11, 22, 22,
      { kind: "node", name: token(anchor.name), ownerId: anchor.ownerId, side: anchor.side, rowIndex: anchor.rowIndex }));
    else if (["component", "zone"].includes(anchor.kind)) {
      const meta = { ownerId: anchor.ownerId, side: anchor.side, rowIndex: anchor.rowIndex };
      result.push(box(anchor.x - 32, anchor.y - 30, 64, 60, { ...meta, kind: "icon" }));
      result.push(box(anchor.x - 48, anchor.y + 24, 96, 18, { ...meta, kind: "mini-label" }));
      if (anchor.component?.relatedLoopNames?.length) result.push(box(anchor.x + 20, anchor.y - 36, 18, 18, { ...meta, kind: "cross-loop-badge" }));
    }
  }
  const textWidth = (value) => [...String(value || "")].reduce((total, char) => total + (char.charCodeAt(0) > 255 ? fontSize : fontSize * .62), 0);
  result.push(box(layout.leftX - 2, 38, textWidth(loop.type) + 4, 20, { kind: "title" }));
  result.push(box(layout.leftX - 2, 60, textWidth(loop.name) + 4, 22, { kind: "title" }));
  for (const side of sides) {
    const source = sideLayout(layout, side), splitX = source.reverse ? source.branchEndX : source.branchStartX;
    result.push(box(source.branchStartX - 48, source.top + 7, Math.max(104, fontSize * 9), 22, { kind: "heading", side }));
    for (const row of source.rows) result.push(box(splitX + (row.index % 2 === 0 ? -24 : 24) - 10, row.y - 10, 20, 20,
      { kind: "badge", side, rowIndex: row.index }));
  }
  return result;
}

function boundary(record) {
  if (record.kind !== "node" || record.anchor.ownerKind !== "loop-side") return "";
  const inlet = record.anchor.portRole === "inlet";
  return (record.side === "supply" ? inlet : !inlet) ? "left" : "right";
}

function recordsFor(makeRecords, layout) {
  return makeRecords(layout).map((record) => ({ ...record,
    width: finite(record.width) ? Math.max(48, record.width) : 184,
    height: finite(record.height) ? Math.max(24, record.height) : 72 }));
}

function seedConfiguration(layout, records, options) {
  const gutterWidth = Math.max(98, ...records.filter(boundary).map((record) => record.width + 32));
  const bands = {}, minimumMiddle = {};
  for (const side of sides) {
    const source = sideLayout(layout, side), selected = records.filter((record) => record.side === side);
    const first = source.rows[0]?.index, last = source.rows.at(-1)?.index;
    const topHeight = Math.max(0, ...selected.filter((record) => record.kind === "node" && record.anchor.rowIndex === first).map((record) => record.height));
    const bottomHeight = Math.max(0, ...selected.filter((record) => record.kind === "component" && record.anchor.rowIndex === last).map((record) => record.height));
    const nodeWidth = Math.max(0, ...selected.filter((record) => record.kind === "node" && !boundary(record)).map((record) => record.width));
    const equipmentWidth = Math.max(0, ...selected.filter((record) => record.kind === "component").map((record) => record.width));
    const serialCount = Math.max(1, ...source.rows.map((row) => row.items.filter((item) => item.kind === "component").length));
    minimumMiddle[side] = Math.max(source.hasParallel ? 500 : 240, serialCount * Math.max(140, nodeWidth * 1.25 + 100, equipmentWidth + 20));
    const bayWidth = (area, items) => items.length ? Math.max(122, ...selected.filter((record) => record.anchor.area === area).map((record) => record.width + 64)) * items.length : 122;
    const leadIn = bayWidth("lead-in", source.leadInItems), leadOut = bayWidth("lead-out", source.leadOutItems);
    bands[side] = { reserveAnnotations: true, rowTopOffset: Math.max(46, topHeight ? topHeight + 88 : 46),
      rowBottomPadding: Math.max(52, bottomHeight + 68), rowGaps: source.rows.slice(1).map(() => 64), rowItemXs: {},
      leftBay: source.reverse ? leadOut : leadIn, rightBay: source.reverse ? leadIn : leadOut };
  }
  const width = Math.max(options.width || 1480, 1480,
    ...sides.map((side) => gutterWidth * 2 + bands[side].leftBay + bands[side].rightBay + minimumMiddle[side]));
  return { width, gutterWidth, bands };
}

function layoutOptions(config) {
  const annotationBands = {};
  for (const side of sides) {
    const band = config.bands[side];
    annotationBands[side] = { ...band, branchStartX: config.gutterWidth + band.leftBay,
      branchEndX: config.width - config.gutterWidth - band.rightBay };
  }
  return { readOnly: true, width: config.width, gutterWidth: config.gutterWidth, annotationBands };
}

function candidates(record, layout, placed) {
  const { anchor, width, height } = record, edge = boundary(record), source = sideLayout(layout, record.side);
  const result = [], add = (x, y, preference) => {
    x = clamp(x, 12, layout.width - width - 12);
    if (!result.some((candidate) => Math.abs(candidate.x - x) < .1 && Math.abs(candidate.y - y) < .1)) result.push({ x, y, width, height, preference });
  };
  if (edge) {
    const x = edge === "left" ? layout.leftX - width - 20 : layout.rightX + 20;
    [anchor.y - height / 2, anchor.y - height - 20, anchor.y + 20].forEach((y, index) => add(x, y, index * 6));
    return result;
  }
  const inBay = anchor.area === "lead-in" || anchor.area === "lead-out";
  let bayLow, bayHigh;
  if (inBay) {
    const left = source.reverse ? anchor.area === "lead-out" : anchor.area === "lead-in";
    bayLow = left ? layout.leftX + 14 : source.branchEndX + 40;
    bayHigh = (left ? source.branchStartX - 40 : layout.rightX - 14) - width;
  }
  const boundedX = (x) => inBay && bayHigh >= bayLow ? clamp(x, bayLow, bayHigh) : x;
  if (record.kind === "component") {
    const xs = [anchor.x - width / 2, anchor.x - width / 2 - 54, anchor.x - width / 2 + 54,
      anchor.x - width - 58, anchor.x + 58];
    for (const [index, x] of xs.entries()) for (const dy of [52, 68, 84]) add(boundedX(x), anchor.y + dy, index * 12 + (dy - 52) * .2);
  } else {
    const direction = source.reverse ? -1 : 1;
    const portSide = anchor.portRole === "inlet" ? -direction : direction;
    const outward = portSide < 0 ? anchor.x - width - 14 : anchor.x + 14;
    const xs = [outward, anchor.x - width / 2, (anchor.ownerX ?? anchor.x) - width / 2,
      portSide < 0 ? anchor.x + 14 : anchor.x - width - 14, anchor.x - width - 40, anchor.x + 40,
      anchor.x - width - 80, anchor.x + 80];
    const paired = placed.find((item) => item.kind === "node" && item.anchor.ownerId === anchor.ownerId && item.anchor.portRole !== anchor.portRole);
    for (const [index, x] of xs.entries()) {
      if (paired) add(boundedX(x), paired.y, index * 10 + 2);
      for (const clearance of [22, 42, 62]) add(boundedX(x), anchor.y - height - clearance, index * 10 + (clearance - 22) * .2);
      add(boundedX(x), anchor.y - height - 12, index * 10 + 8);
      add(boundedX(x), anchor.y + 20, index * 10 + 24);
    }
  }
  return result;
}

function hitSegment(a, b, rect) {
  let low = 0, high = 1;
  const dx = b.x - a.x, dy = b.y - a.y;
  for (const [p, q] of [[-dx, a.x - rect.x], [dx, rect.x + rect.width - a.x], [-dy, a.y - rect.y], [dy, rect.y + rect.height - a.y]]) {
    if (Math.abs(p) < .00001) { if (q < -.01) return false; }
    else { const ratio = q / p; if (p < 0) low = Math.max(low, ratio); else high = Math.min(high, ratio); if (low > high + .00001) return false; }
  }
  return true;
}

function cross(a, b, c, d) {
  const turn = (p, q, r) => (q.x - p.x) * (r.y - p.y) - (q.y - p.y) * (r.x - p.x);
  return turn(a, b, c) * turn(a, b, d) < -.01 && turn(c, d, a) * turn(c, d, b) < -.01;
}

function crossesOwnBox(a, b, bounds) {
  const radius = 7, epsilon = .2;
  const centers = [point(bounds.x + radius, bounds.y + radius), point(bounds.x + bounds.width - radius, bounds.y + radius),
    point(bounds.x + radius, bounds.y + bounds.height - radius), point(bounds.x + bounds.width - radius, bounds.y + bounds.height - radius)];
  if (hitSegment(a, b, box(bounds.x + radius, bounds.y + epsilon, bounds.width - radius * 2, bounds.height - epsilon * 2))
    || hitSegment(a, b, box(bounds.x + epsilon, bounds.y + radius, bounds.width - epsilon * 2, bounds.height - radius * 2))) return true;
  const dx = b.x - a.x, dy = b.y - a.y, length = dx * dx + dy * dy;
  return centers.some((center) => {
    const t = length ? clamp(((center.x - a.x) * dx + (center.y - a.y) * dy) / length, 0, 1) : 0;
    return Math.hypot(a.x + t * dx - center.x, a.y + t * dy - center.y) < radius - epsilon;
  });
}

function roundedContact(bounds, target) {
  const radius = 7, inner = point(clamp(target.x, bounds.x + radius, bounds.x + bounds.width - radius),
    clamp(target.y, bounds.y + radius, bounds.y + bounds.height - radius));
  const dx = target.x - inner.x, dy = target.y - inner.y, distance = Math.hypot(dx, dy);
  return distance ? point(inner.x + dx / distance * radius, inner.y + dy / distance * radius) : point(inner.x, bounds.y);
}

function pathCost(path, record, blockers, leaders, bounds) {
  let hits = 0, length = 0, crossings = 0;
  const hitObstacles = new Set();
  for (let index = 1; index < path.length; index++) {
    const a = path[index - 1], b = path[index]; length += Math.hypot(a.x - b.x, a.y - b.y);
    if (crossesOwnBox(a, b, bounds)) hits++;
    for (const obstacle of blockers) {
      if (obstacle.kind === "node" && record.kind === "node" && obstacle.name === token(record.item.name)) continue;
      if (obstacle.kind === "icon" && record.kind === "component" && obstacle.ownerId === record.anchor.ownerId) continue;
      if (hitSegment(a, b, obstacle)) { hits++; hitObstacles.add(obstacle); }
    }
    for (const leader of leaders) for (let segment = 1; segment < leader.length; segment++) if (cross(a, b, leader[segment - 1], leader[segment])) crossings++;
  }
  return { hits, length, crossings, hitObstacles: [...hitObstacles], cost: hits * 10000 + crossings * 500 + length };
}

function leaderPath(record, bounds, blockers, leaders) {
  const { anchor } = record, radius = record.kind === "node" ? 11 : 29;
  const contact = (from) => {
    const dx = from.x - anchor.x, dy = from.y - anchor.y, distance = Math.hypot(dx, dy) || 1;
    return point(anchor.x + dx / distance * radius, anchor.y + dy / distance * radius);
  };
  const start = roundedContact(bounds, anchor);
  const paths = [[start, contact(start)]];
  const top = point(clamp(anchor.x, bounds.x + 8, bounds.x + bounds.width - 8), bounds.y);
  const bottom = point(top.x, bounds.y + bounds.height);
  const left = point(bounds.x, clamp(anchor.y, bounds.y + 8, bounds.y + bounds.height - 8));
  const right = point(bounds.x + bounds.width, left.y);
  for (const from of [top, bottom, left, right]) {
    for (const clearance of record.kind === "node" ? [radius + 9, 42, 66, 96] : [radius + 9]) {
      const above = point(anchor.x, anchor.y - clearance), below = point(anchor.x, anchor.y + clearance);
      paths.push([from, point(from.x, above.y), above, contact(above)], [from, point(from.x, below.y), below, contact(below)]);
    }
    for (const dx of [-54, 54, -86, 86]) {
      const corridor = anchor.x + dx, approach = point(corridor, anchor.y + (record.kind === "component" ? 22 : -18));
      paths.push([from, point(corridor, from.y), approach, contact(approach)]);
    }
  }
  let best;
  for (const candidate of paths) {
    const path = candidate.filter((item, index) => !index || Math.hypot(item.x - candidate[index - 1].x, item.y - candidate[index - 1].y) > .01);
    const score = pathCost(path, record, blockers, leaders, bounds);
    if (!best || score.cost < best.cost) best = { ...score, path };
  }
  return best;
}

function scoredCandidates(record, layout, blockers, placed) {
  return candidates(record, layout, placed).map((candidate) => {
    const collisions = blockers.filter((obstacle) => overlap(candidate, obstacle));
    const area = collisions.reduce((sum, obstacle) => sum + overlap(candidate, obstacle), 0);
    const outside = Math.max(0, 20 - candidate.y) + Math.max(0, candidate.y + candidate.height + 12 - layout.height);
    return { ...candidate, collisions, hard: collisions.length + Number(outside > 0), area: area + outside * candidate.width };
  }).sort((a, b) => a.hard - b.hard || a.area - b.area || a.preference - b.preference);
}

function placedBoxes(records) {
  return records.map((record) => ({ ...record, kind: "box", record, rowIndex: record.anchor.rowIndex }));
}

function place(records, layout, nativeObstacles) {
  const placed = [], leaders = [];
  const ordered = [...records].sort((a, b) => Number(Boolean(boundary(b))) - Number(Boolean(boundary(a)))
    || a.anchor.y - b.anchor.y || Number(b.kind === "component") - Number(a.kind === "component") || a.anchor.x - b.anchor.x);
  for (const record of ordered) {
    let blockers = [...nativeObstacles, ...placedBoxes(placed)];
    let options = scoredCandidates(record, layout, blockers, placed);
    // Input/output labels are one assembly. Repair a greedy earlier port
    // choice locally before asking another branch to move or grow its gap.
    const paired = record.kind === "node" && placed.find((item) => item.kind === "node"
      && item.anchor.ownerId === record.anchor.ownerId && item.anchor.portRole !== record.anchor.portRole);
    if (options[0].hard && paired && !boundary(record)) {
      const others = placed.filter((item) => item !== paired), pairBlockers = [...nativeObstacles, ...placedBoxes(others)];
      for (const candidate of scoredCandidates(paired, layout, pairBlockers, others).filter((item) => !item.hard).slice(0, 20)) {
        const moved = { ...paired, x: candidate.x, y: candidate.y };
        const trialPlaced = [...others, moved], trialBlockers = [...nativeObstacles, ...placedBoxes(trialPlaced)];
        const trial = scoredCandidates(record, layout, trialBlockers, trialPlaced);
        if (trial[0].hard) continue;
        Object.assign(paired, { x: candidate.x, y: candidate.y, collisions: [], outside: false });
        blockers = trialBlockers; options = trial; break;
      }
    }
    let best;
    for (const candidate of options.slice(0, 6)) {
      const route = leaderPath(record, candidate, blockers, leaders);
      const pair = placed.find((item) => item.kind === "node" && record.kind === "node" && item.anchor.ownerId === record.anchor.ownerId);
      const score = candidate.preference + route.cost * .03 + Math.hypot(candidate.x + candidate.width / 2 - record.anchor.x, candidate.y + candidate.height / 2 - record.anchor.y) * .02
        + (pair ? Math.abs(pair.y - candidate.y) * .05 : 0);
      if (!best || candidate.hard < best.hard || candidate.hard === best.hard && (candidate.area < best.area - .1 || Math.abs(candidate.area - best.area) <= .1 && score < best.score)) best = { ...candidate, score, route };
    }
    Object.assign(record, { x: best.x, y: best.y, leader: best.route.path, collisions: best.collisions, outside: best.hard > best.collisions.length });
    placed.push(record); leaders.push(record.leader);
  }
  // Final routing sees later cards too, not only already-visited neighbors.
  leaders.length = 0;
  for (const record of placed) {
    const blockers = [...nativeObstacles, ...placed.filter((item) => item !== record).map((item) => ({ ...item, kind: "box", record: item }))];
    const route = leaderPath(record, record, blockers, leaders);
    record.leader = route.path; record.leaderCollisions = route.hits;
    record.leaderObstacles = route.hitObstacles.map((obstacle) => ({ kind: obstacle.kind,
      rowIndex: obstacle.rowIndex ?? obstacle.record?.anchor.rowIndex, side: obstacle.side ?? obstacle.record?.side,
      ownerId: obstacle.ownerId ?? obstacle.record?.anchor.ownerId }));
    leaders.push(route.path);
  }
  alignPortPairs(placed, layout, nativeObstacles);
  return records;
}

// Align a proved inlet/outlet pair when the existing safe routes leave room.
// This pass moves only its boxes; it never changes ports, pipes or row order.
function alignPortPairs(records, layout, nativeObstacles) {
  if (records.some((record) => record.collisions.length || record.outside || record.leaderCollisions)) return;
  const owners = new Map();
  for (const record of records) if (record.kind === "node" && record.anchor.ownerKind === "component" && ["inlet", "outlet"].includes(record.anchor.portRole)) {
    const key = record.anchor.ownerId;
    if (!owners.has(key)) owners.set(key, []);
    owners.get(key).push(record);
  }
  for (const pair of owners.values()) {
    if (pair.length !== 2 || pair[0].anchor.portRole === pair[1].anchor.portRole || Math.abs(pair[0].y - pair[1].y) < .1) continue;
    const others = records.filter((record) => !pair.includes(record));
    for (const y of [Math.max(...pair.map((record) => record.y)), Math.min(...pair.map((record) => record.y))]) {
      const trial = pair.map((record) => ({ ...record, y }));
      if (trial.some((rect) => rect.y < 20 || rect.y + rect.height > layout.height - 12
        || [...nativeObstacles, ...others].some((obstacle) => overlap(rect, obstacle))) || overlap(trial[0], trial[1])) continue;
      if (others.some((record) => record.leader.some((point, index) => index && trial.some((rect) => hitSegment(record.leader[index - 1], point, rect))))) continue;
      const routes = [], leaders = others.map((record) => record.leader);
      for (let index = 0; index < trial.length; index++) {
        const record = trial[index], blockers = [...nativeObstacles, ...placedBoxes([...others, ...trial.filter((item) => item !== record)])];
        const route = leaderPath(record, record, blockers, leaders);
        routes.push(route); leaders.push(route.path);
      }
      if (routes.some((route) => route.hits)) continue;
      pair.forEach((record, index) => Object.assign(record, { y, leader: routes[index].path, leaderCollisions: 0, leaderObstacles: [] }));
      break;
    }
  }
}

function refine(config, layout, records) {
  let changed = false;
  for (const side of sides) {
    const source = sideLayout(layout, side), band = config.bands[side], rows = source.rows;
    const rowPosition = new Map(rows.map((row, index) => [row.index, index]));
    const affected = records.filter((record) => record.side === side && (record.collisions.length || record.outside || record.leaderCollisions));
    for (const record of affected) {
      const index = rowPosition.get(record.anchor.rowIndex);
      if (index === undefined) continue;
      for (const obstacle of [...record.collisions, ...record.leaderObstacles]) {
        const otherIndex = rowPosition.get(obstacle.rowIndex ?? obstacle.record?.anchor.rowIndex);
        if (otherIndex === undefined || otherIndex === index || obstacle.side && obstacle.side !== side && obstacle.kind !== "box") continue;
        // Stagger only adjacent, single-equipment parallel assemblies under
        // real collision pressure; source order and physical edges stay intact.
        if (source.hasParallel && Math.abs(otherIndex - index) === 1) for (const rowIndex of [index, otherIndex]) {
          const row = rows[rowIndex];
          if (row.items.length === 1 && row.items[0].kind === "component" && !band.rowItemXs[row.index]) {
            band.rowItemXs[row.index] = [(source.branchStartX + source.branchEndX) / 2 + (rowIndex % 2 ? 160 : -160)]; changed = true;
          }
        }
        const gapIndex = otherIndex > index ? index : index - 1;
        if (gapIndex < 0 || gapIndex >= band.rowGaps.length) continue;
        const target = otherIndex > index ? record.y + record.height - record.anchor.y + 48 : record.anchor.y - record.y + 48;
        const next = Math.min(260, Math.max(band.rowGaps[gapIndex] + 32, target));
        if (next > band.rowGaps[gapIndex]) { band.rowGaps[gapIndex] = next; changed = true; }
      }
      if (record.y < source.top + 34 || record.outside && record.y < 20) {
        band.rowTopOffset += Math.max(24, source.top + 40 - record.y); changed = true;
      }
      if (record.y + record.height > source.top + source.height - 12) {
        band.rowBottomPadding += record.y + record.height - source.top - source.height + 24; changed = true;
      }
    }
  }
  return changed;
}

const hasCollision = (record) => record.collisions.length || record.outside || record.leaderCollisions;

function rowShapes(layout, records, side, fontSize) {
  const rows = sideLayout(layout, side).rows;
  const result = new Map(rows.map((row) => [row.index, []]));
  for (const obstacle of obstacles(layout, {}, fontSize)) {
    if (obstacle.side === side && result.has(obstacle.rowIndex)) result.get(obstacle.rowIndex).push(obstacle);
  }
  for (const record of records) {
    if (record.side !== side || !result.has(record.anchor.rowIndex)) continue;
    const shapes = result.get(record.anchor.rowIndex);
    shapes.push(record);
    for (let index = 1; index < record.leader.length; index++) {
      const a = record.leader[index - 1], b = record.leader[index];
      shapes.push(box(Math.min(a.x, b.x) - 1, Math.min(a.y, b.y) - 1, Math.abs(a.x - b.x) + 2, Math.abs(a.y - b.y) + 2));
    }
  }
  return result;
}

function compactGaps(layout, records, fontSize) {
  const result = {};
  for (const side of sides) {
    const source = sideLayout(layout, side), shapes = rowShapes(layout, records, side, fontSize);
    result[side] = source.rows.slice(1).map((next, index) => {
      const row = source.rows[index], original = next.y - row.y;
      let needed = 64;
      for (const upper of shapes.get(row.index)) for (const lower of shapes.get(next.index)) {
        if (upper.x + upper.width + 4 <= lower.x || lower.x + lower.width + 4 <= upper.x) continue;
        needed = Math.max(needed, upper.y + upper.height - row.y - (lower.y - next.y) + 4);
      }
      return Math.max(64, Math.min(original, Math.ceil(needed)));
    });
  }
  return result;
}

function restoreImplicatedGaps(config, safeLayout, trialRecords) {
  let restored = false;
  for (const record of trialRecords.filter(hasCollision)) {
    const source = sideLayout(safeLayout, record.side), positions = new Map(source.rows.map((row, index) => [row.index, index]));
    const own = positions.get(record.anchor.rowIndex), implicated = new Set();
    for (const obstacle of [...record.collisions, ...record.leaderObstacles]) {
      const other = positions.get(obstacle.rowIndex ?? obstacle.record?.anchor.rowIndex);
      if (own !== undefined && other !== undefined && own !== other) {
        for (let index = Math.min(own, other); index < Math.max(own, other); index++) implicated.add(index);
      } else if (other !== undefined || own !== undefined) {
        const index = other ?? own; implicated.add(index - 1); implicated.add(index);
      }
    }
    // A side bus depends on the first/last branch midpoint. Without a branch
    // attribution its changed intervals all participate in that dependency.
    if (!implicated.size) for (let index = 0; index < source.rows.length - 1; index++) implicated.add(index);
    for (const index of implicated) {
      if (index < 0 || index >= source.rows.length - 1) continue;
      const original = source.rows[index + 1].y - source.rows[index].y;
      if (config.bands[record.side].rowGaps[index] < original) {
        config.bands[record.side].rowGaps[index] = original; restored = true;
      }
    }
  }
  return restored;
}

function compact(loop, makeRecords, options, config, layout, records) {
  if (records.some(hasCollision)) return { layout, records };
  const proposed = compactGaps(layout, records, options.fontSize || 13);
  if (!sides.some((side) => proposed[side].some((gap, index) => gap < sideLayout(layout, side).rows[index + 1].y - sideLayout(layout, side).rows[index].y - 4))) return { layout, records };
  const trialConfig = { ...config, bands: Object.fromEntries(sides.map((side) => [side, { ...config.bands[side], rowGaps: proposed[side] }])) };
  for (let attempt = 0; attempt < 2; attempt++) {
    const trialLayout = buildHVACLoopDiagramLayout(loop, layoutOptions(trialConfig));
    const trialRecords = place(recordsFor(makeRecords, trialLayout), trialLayout, obstacles(trialLayout, loop, options.fontSize || 13));
    if (!trialRecords.some(hasCollision)) return { layout: trialLayout, records: trialRecords };
    if (!restoreImplicatedGaps(trialConfig, layout, trialRecords)) break;
  }
  return { layout, records };
}

export function planHVACInspectionLayout(loop, makeRecords, options = {}) {
  let layout = buildHVACLoopDiagramLayout(loop, { readOnly: true, width: options.width || 1480 });
  const config = seedConfiguration(layout, recordsFor(makeRecords, layout), options);
  let records = [];
  for (let pass = 0; pass < 6; pass++) {
    layout = buildHVACLoopDiagramLayout(loop, layoutOptions(config));
    records = place(recordsFor(makeRecords, layout), layout, obstacles(layout, loop, options.fontSize || 13));
    if (!records.some((record) => record.collisions.length || record.outside || record.leaderCollisions) || !refine(config, layout, records)) break;
  }
  ({ layout, records } = compact(loop, makeRecords, options, config, layout, records));
  const unresolved = records.filter((record) => record.collisions.length || record.outside || record.leaderCollisions)
    .map((record) => ({ key: record.key || record.item.id || record.item.name,
      boxes: record.collisions.length, outside: record.outside, leaders: record.leaderCollisions }));
  return { layout, records, unresolved };
}
