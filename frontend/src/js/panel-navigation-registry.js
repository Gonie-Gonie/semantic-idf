export const PANEL_NAVIGATION_VIEW_IDS = Object.freeze([
  "summary",
  "profile",
  "hvac",
  "output",
  "simulation",
  "diagnose",
  "geometry",
  "input-semantic",
  "input-text",
  "input-json",
  "input-table",
]);

const requiredAdapterMethods = Object.freeze([
  "canReveal",
  "reveal",
  "selectFromElement",
  "captureContext",
  "restoreContext",
  "preferredSemanticOccurrence",
]);

const adapters = new Map();

/**
 * Registers the only component that may translate a semantic selection into
 * panel-specific behavior. Returning an unregister function keeps tests and
 * temporary panel mounts from sharing stale adapters.
 */
export function registerPanelNavigationAdapter(viewId, adapter) {
  const normalizedViewId = normalizeViewId(viewId);
  validateAdapter(normalizedViewId, adapter);
  adapters.set(normalizedViewId, adapter);
  return () => {
    if (adapters.get(normalizedViewId) === adapter) {
      adapters.delete(normalizedViewId);
    }
  };
}

export function getPanelNavigationAdapter(viewId) {
  const normalizedViewId = normalizeViewId(viewId, { allowUnknown: true });
  return normalizedViewId ? adapters.get(normalizedViewId) || null : null;
}

export function hasPanelNavigationAdapter(viewId) {
  return getPanelNavigationAdapter(viewId) !== null;
}

export function listPanelNavigationAdapters() {
  return PANEL_NAVIGATION_VIEW_IDS.filter((viewId) => adapters.has(viewId));
}

// Intended for isolated controller tests. Production teardown should use the
// unregister function returned by registerPanelNavigationAdapter().
export function clearPanelNavigationAdapters() {
  adapters.clear();
}

function normalizeViewId(viewId, options = {}) {
  const normalized = String(viewId || "").trim().toLowerCase();
  if (!normalized) {
    if (options.allowUnknown) {
      return "";
    }
    throw new TypeError("Panel navigation viewId is required");
  }
  if (!options.allowUnknown && !PANEL_NAVIGATION_VIEW_IDS.includes(normalized)) {
    throw new RangeError(`Unknown panel navigation view: ${normalized}`);
  }
  return normalized;
}

function validateAdapter(viewId, adapter) {
  if (!adapter || typeof adapter !== "object") {
    throw new TypeError(`Panel navigation adapter for ${viewId} must be an object`);
  }
  for (const method of requiredAdapterMethods) {
    if (typeof adapter[method] !== "function") {
      throw new TypeError(`Panel navigation adapter for ${viewId} is missing ${method}()`);
    }
  }
}
