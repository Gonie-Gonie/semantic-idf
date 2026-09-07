// A mounted scene, not a history cache. Returning to an older scope after a
// different scene was acquired builds again; multi-key reuse belongs elsewhere.
const CONTEXT_FIELDS = [
  "result", "explanation", "runId", "schema", "scopeKind", "zoneName",
  "period", "service", "presentationKey",
];

// Capture collection replacements without traversing graph data on selection.
// Nested payloads are immutable within a result. An intentional in-place edit
// must replace a containing collection/result or explicitly clear the slot.
const PAYLOAD_FIELDS = [
  "nodes", "links", "periods", "zoneResults", "sources", "relations",
  "relationshipRules", "scope", "reconciliation", "completeness", "quality",
  "summary", "availableZones", "zoneContributions", "warnings",
];

function snapshotContext(context = {}) {
  const input = context && typeof context === "object" ? context : {};
  return [
    ...CONTEXT_FIELDS.map((field) => input[field]),
    input.result?.purposeResults?.energyExplanationSummary,
    ...PAYLOAD_FIELDS.map((field) => input.explanation?.[field]),
  ];
}

function sameContext(left, right) {
  return left !== null && left.length === right.length &&
    left.every((value, index) => Object.is(value, right[index]));
}

/**
 * Keep exactly one prepared scene. The caller supplies normalized scope and
 * period/service values plus actual result/explanation identities. presentationKey
 * covers localized presentation (for example document.documentElement.lang).
 * Selection, drawers, callbacks, report metadata and theme are deliberately not
 * scene keys; callers update those presentation details using current options.
 */
export function createEnergyPathSceneSlot() {
  let currentKey = null;
  let currentScene = null;

  const clear = () => {
    currentKey = null;
    currentScene = null;
  };

  return {
    acquire(context, build) {
      const key = snapshotContext(context);
      if (sameContext(currentKey, key)) return currentScene;

      // A failed new build must not leave an unrelated old scene discoverable.
      clear();
      if (typeof build !== "function") {
        throw new TypeError("Energy Path scene builder must be a function");
      }
      const scene = build();
      if (scene == null) return null;
      if (typeof scene !== "object" || Array.isArray(scene) || typeof scene.then === "function") {
        throw new TypeError("Energy Path scene builder must return a synchronous scene object");
      }
      currentKey = key;
      currentScene = scene;
      return scene;
    },
    peek(context) {
      return arguments.length === 0 || sameContext(currentKey, snapshotContext(context))
        ? currentScene : null;
    },
    clear,
  };
}
