// Named rules keep manager status overrides separate from Boolean conditions.
// This projection never changes the native expression or evaluates run status.
const MAX_DEPTH = 48;
const MAX_EXPRESSIONS = 4096;
const managerLists = new Set(["air_managers", "plant_managers"]);
const signatureFields = ["op", "domain", "scheduleName", "value", "target", "reason", "referenceId", "booleanValue", "managerStatus", "originalOp"];
const unknown = (domain, reason = "expression_truncated") => ({ op: "unknown", domain, reason });

function sourceSignature(source) {
  return source ? [source.objectIndex, source.objectType, source.objectName] : null;
}

function canonicalExpression(expression, budget, path = new Set(), depth = 0) {
  if (budget.remaining-- <= 0) return unknown(expression?.domain);
  if (!expression || depth > MAX_DEPTH || path.has(expression)) return unknown(expression?.domain, "unresolved_expression");
  const result = { ...expression, children: [] };
  path.add(expression);
  for (const child of expression.children || []) {
    if (budget.remaining <= 0) { result.children.push(unknown(child?.domain)); break; }
    result.children.push(canonicalExpression(child, budget, path, depth + 1));
  }
  path.delete(expression);
  // A source-less one-item aggregate adds no priority or assignment context.
  // Sourced lists and authored plant order remain distinct named definitions.
  if (managerLists.has(result.op) && result.domain === "manager" && result.children.length === 1 &&
      result.children[0].op === result.op && result.children[0].domain === "manager" &&
      !result.source && !result.assignmentOwner && !result.reason && !result.target &&
      !result.scheduleName && !result.referenceId && result.value === undefined) return result.children[0];
  return result;
}

export function buildHVACOnOffPresentation(definitions = []) {
  const rules = [], managers = new Map(), permissions = new Map(), signatures = new WeakMap(), interned = new Map();
  const signature = (expression) => {
    if (!signatures.has(expression)) {
      // Child IDs preserve exact structure without repeatedly escaping nested
      // JSON strings. Deep sourced lists stay linear in the bounded node count.
      const key = JSON.stringify([signatureFields.map((field) => [field, expression[field]]),
        sourceSignature(expression.source), sourceSignature(expression.assignmentOwner),
        (expression.children || []).map(signature)]);
      if (!interned.has(key)) interned.set(key, interned.size + 1);
      signatures.set(expression, interned.get(key));
    }
    return signatures.get(expression);
  };
  const registerManager = (expression) => {
    const key = signature(expression);
    if (!managers.has(key)) {
      const id = `manager-rule-${managers.size + 1}`;
      managers.set(key, id);
      rules.push({ id, kind: "manager", expression });
    }
    return managers.get(key);
  };
  const project = (expression) => {
    if (expression.op === "manager_gate" || expression.op === "fan_gate") {
      const key = signature(expression);
      if (!permissions.has(key)) {
        const id = `permission-rule-${permissions.size + 1}`;
        permissions.set(key, id);
        const manager = expression.children[0] || unknown("manager", "unresolved_expression");
        const fallback = expression.op === "fan_gate" ? expression.children[1] || unknown("boolean", "unresolved_expression")
          : { op: "always", domain: "boolean", booleanValue: true, children: [] };
        const managerId = registerManager(manager);
        rules.push({ id, kind: "permission", op: expression.op, target: expression.target, managerId,
          otherwise: project(fallback), source: expression.source || fallback.source });
      }
      return { op: "rule_reference", referenceId: permissions.get(key), domain: "boolean" };
    }
    let children = expression.children || [];
    if (expression.op === "all") {
      const fans = children.filter((child) => child.op === "fan_gate" && child.children[0]);
      // For the same status and target, an allowed fan override also satisfies
      // manager permission. This implication holds for unknown status sets.
      children = children.filter((child) => child.op !== "manager_gate" || !child.children[0] || !fans.some((fan) =>
        fan.target === child.target && signature(fan.children[0]) === signature(child.children[0])));
    }
    const result = { ...expression, children: children.map(project) };
    if (result.op === "all" && result.children.length === 1 && !result.source) return result.children[0];
    return result;
  };
  const budget = { remaining: MAX_EXPRESSIONS };
  const formulas = definitions.slice(0, MAX_EXPRESSIONS).map((definition) => ({
    ...definition, expression: project(canonicalExpression(definition.expression, budget)),
  }));
  return { formulas, rules };
}
