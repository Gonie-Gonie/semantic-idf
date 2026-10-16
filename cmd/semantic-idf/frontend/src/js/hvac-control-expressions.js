// Presentation-only compression. Boolean operators come from the native AST;
// signal dependencies remain relationships, never invented Boolean operands.
const MAX_DEPTH = 48;
const MAX_EXPRESSIONS = 4096;
const managerOps = new Set(["air_managers", "plant_managers", "manager_scheduled", "manager_scheduled_on", "manager_scheduled_off", "manager_unknown"]);
const supportedOps = new Set(["all", "any", "always", "schedule_positive", "manager_gate", "fan_gate", "runtime_condition", "passive_flow", "unknown", ...managerOps]);
const sourceKey = (source = {}) => JSON.stringify([source.objectIndex, source.objectType, source.objectName]);

function nodeIndex(nodes) {
  const index = new Map();
  for (const node of nodes) index.set(node.id, index.has(node.id) ? null : node);
  return index;
}

function dependencies(graph, target, index) {
  const needed = new Set([target]), pending = [target];
  while (pending.length && needed.size < MAX_EXPRESSIONS) {
    const id = pending.pop();
    for (const edge of graph.edges || []) {
      if (edge.to !== id || edge.from === "permission" || !index.get(edge.from) || needed.has(edge.from)) continue;
      needed.add(edge.from); pending.push(edge.from);
    }
  }
  return {
    nodes: (graph.nodes || []).filter((node) => node.id !== target && needed.has(node.id)),
    edges: (graph.edges || []).filter((edge) => needed.has(edge.from) && needed.has(edge.to) && edge.from !== "permission"),
    truncated: pending.length > 0,
  };
}

function normalizeExpression(expression, graph, index, domain = "boolean", budget = { remaining: MAX_EXPRESSIONS }, path = new Set(), depth = 0) {
  if (budget.remaining-- <= 0) return { op: "unknown", domain, reason: "expression_truncated" };
  if (!expression || depth > MAX_DEPTH || path.has(expression)) return { op: "unknown", domain, reason: "unresolved_expression" };
  const result = { ...expression, children: undefined, domain };
  if (!supportedOps.has(expression.op)) return { ...result, op: "unknown", originalOp: expression.op, reason: "unsupported_expression" };
  if (expression.referenceId) {
    const node = index.get(expression.referenceId);
    if (!node) return { ...result, op: "unknown", reason: "unresolved_control_reference" };
    result.node = node;
    result.source = node.source;
    result.dependencies = dependencies(graph, node.id, index);
  }
  if (expression.op === "always") {
    if (domain === "manager") result.managerStatus = ["NoAction", "ForceOff", "CycleOn", "CycleOnZoneFansOnly"][Number.isInteger(expression.value) && expression.value >= 0 && expression.value <= 3 ? expression.value : 0];
    else result.booleanValue = expression.value === 0 ? false : true;
  }
  path.add(expression);
  const children = [], authoredChildren = expression.children || [];
  for (let position = 0; position < authoredChildren.length; position++) {
    if (budget.remaining <= 0) { children.push({ op: "unknown", domain, reason: "expression_truncated" }); break; }
    children.push(normalizeExpression(authoredChildren[position], graph, index,
      expression.op === "air_managers" || expression.op === "plant_managers" || position === 0 && (expression.op === "manager_gate" || expression.op === "fan_gate") ? "manager" : "boolean",
      budget, path, depth + 1));
  }
  path.delete(expression);
  if (expression.op === "all" || expression.op === "any") {
    // Associative Boolean groups can share one visual bracket. Preserve named
    // source groups and every nontrivial false/unknown/native rule operand.
    result.children = children.flatMap((child) => child.op === expression.op && !child.source ? child.children : [child]);
    if (expression.op === "all") result.children = result.children.filter((child) => !(child.op === "always" && child.domain === "boolean" && child.booleanValue && !child.reason));
    if (!result.children.length) return { ...result, op: "always", booleanValue: expression.op === "all", children: [] };
    if (result.children.length === 1 && !result.source) return result.children[0];
  } else result.children = children;
  return result;
}

function sameExpression(left, right) {
  const pending = [[left, right]], visited = new Map();
  let count = 0;
  while (pending.length) {
    if (++count > MAX_EXPRESSIONS) return false;
    const [a, b] = pending.pop();
    if (a === b) continue;
    if (!a || !b) return false;
    if (visited.get(a) === b) continue;
    visited.set(a, b);
    if (["op", "scheduleName", "value", "target", "reason", "referenceId"].some((key) => a[key] !== b[key]) || sourceKey(a.source) !== sourceKey(b.source) || sourceKey(a.assignmentOwner) !== sourceKey(b.assignmentOwner)) return false;
    const ac = a.children || [], bc = b.children || [];
    if (ac.length !== bc.length || ac.length > MAX_EXPRESSIONS - count - pending.length) return false;
    for (let position = 0; position < ac.length; position++) pending.push([ac[position], bc[position]]);
  }
  return true;
}

function onOffFormulas(profile, component) {
  const graph = profile.onOff || {}, index = nodeIndex(graph.nodes || []);
  const owner = (profile.availabilityLayers || []).find((layer) => layer.role === "equipment")?.owner || {
    objectIndex: component.objectIndex ?? profile.componentIndex, objectType: component.objectType, objectName: component.objectName,
  };
  const formula = (id, code, expression) => ({ id, code, source: owner, expression });
  if (profile.onOffExpression?.op === "passive_flow") return [formula("passive", "passiveTransfer", normalizeExpression(profile.onOffExpression, graph, index))];
  const permission = normalizeExpression(profile.finalAvailability, graph, index);
  let command;
  const authored = profile.onOffExpression;
  if (!authored?.op) command = { op: "unknown", domain: "boolean", reason: "on_off_expression_requires_reanalysis" };
  else if (authored.op === "all" && authored.children?.length === 2 && sameExpression(authored.children[0], profile.finalAvailability)) {
    command = { op: "all", domain: "boolean", children: [{ op: "reference", domain: "boolean", referenceId: "availability" }, normalizeExpression(authored.children[1], graph, index)] };
  } else command = normalizeExpression(authored, graph, index);
  return [formula("availability", "finalAvailability", permission), formula("command", "onCommand", command)];
}

function parameterGroups(graph = {}) {
  const nodes = graph.nodes || [], edges = graph.edges || [], index = nodeIndex(nodes);
  const groups = ["inputs", "controls", "commands", "response"].map((id) => ({ id, titleCode: `compact.${id}`, cards: [] }));
  const groupFor = (node) => ["controller", "control", "comparison", "condition", "decision"].includes(node.kind) ? "controls"
    : ["control_variable", "actuator", "modulation"].includes(node.kind) ? "commands"
    : node.boundary === "output" || ["external_output", "state_variable", "equipment", "parameter"].includes(node.kind) ? "response" : "inputs";
  const destinations = new Map(nodes.map((node) => [node.id, { group: groupFor(node), source: node.source }]));
  for (const node of nodes) {
    // A single-use schedule can share its actual target/provider's input card.
    // Its native member still retains its own schedule source and identity.
    if (node.kind !== "schedule") continue;
    const targets = [...new Set(edges.filter((edge) => edge.from === node.id).map((edge) => edge.to))].map((id) => index.get(id)).filter(Boolean);
    if (targets.length === 1 && targets[0].kind === "setpoint") destinations.set(node.id, { group: "inputs", source: targets[0].source });
  }
  for (const node of nodes) {
    if (node.kind !== "external_input" || node.code !== "node_measurement") continue;
    const targets = edges.filter((edge) => edge.from === node.id).map((edge) => index.get(edge.to)).filter(Boolean);
    if (targets.length && targets.every((target) => groupFor(target) === "controls" && sourceKey(target.source) === sourceKey(node.source))) destinations.set(node.id, { group: "controls", source: node.source });
  }
  const cards = new Map();
  for (const node of nodes) {
    const destination = destinations.get(node.id), key = `${destination.group}:${sourceKey(destination.source)}`;
    let card = cards.get(key);
    if (!card) {
      card = { id: key, source: destination.source, members: [], relationships: [] };
      cards.set(key, card); groups.find((group) => group.id === destination.group).cards.push(card);
    }
    card.members.push(node);
  }
  for (const card of cards.values()) {
    const ids = new Set(card.members.map((node) => node.id));
    card.relationships = edges.filter((edge) => ids.has(edge.from) || ids.has(edge.to));
  }
  return { groups: groups.filter((group) => group.cards.length), relationships: edges, nodes };
}

export function buildHVACControlExpressions(profile = {}, { component = {} } = {}) {
  return { onOff: onOffFormulas(profile, component), parameters: parameterGroups(profile.parameterControl) };
}
