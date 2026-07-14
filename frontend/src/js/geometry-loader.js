import { elements, state } from "./state.js";
import { t } from "./i18n.js";
import { configureResultPanelNavigationHooks } from "./panel-navigation-adapters.js";

let geometryModule = null;
let geometryModulePromise = null;

function geometryStatsLabel(geometry) {
  return t("geometry.stats", {
    zones: geometry?.zoneCount || 0,
    surfaces: geometry?.surfaceCount || 0,
    windows: geometry?.windowCount || 0,
  });
}

function renderGeometryPlaceholder(geometry) {
  if (!elements.geometryStats || !elements.geometryCanvasHost) {
    return;
  }
  if (!state.geometryReady && state.report) {
    elements.geometryStats.textContent = t("geometry.pending");
    elements.geometryCanvasHost.innerHTML = `<div class="empty status-loading">${t("geometry.running")}</div>`;
    elements.geometryPlan.innerHTML = "";
    elements.geometryDetails.innerHTML = `<div class="empty">${t("geometry.detailsReadySoon")}</div>`;
    return;
  }
  elements.geometryStats.textContent = geometryStatsLabel(geometry);
  if (!geometry) {
    elements.geometryCanvasHost.innerHTML = `<div class="empty">${t("geometry.noGeometry")}</div>`;
    elements.geometryPlan.innerHTML = "";
    elements.geometryDetails.innerHTML = `<div class="empty">${t("geometry.selectObject")}</div>`;
    return;
  }
  elements.geometryCanvasHost.innerHTML = `<div class="empty status-loading">${t("geometry.loadingRenderer")}</div>`;
  elements.geometryPlan.innerHTML = "";
}

async function loadGeometryModule() {
  if (geometryModule) {
    return geometryModule;
  }
  if (!geometryModulePromise) {
    geometryModulePromise = import("./views/geometry-view.js").then((module) => {
      geometryModule = module;
      return module;
    });
  }
  return geometryModulePromise;
}

export function isGeometryLoaded() {
  return Boolean(geometryModule);
}

export function preloadGeometryRenderer() {
  return loadGeometryModule();
}

export function renderGeometry(geometry = state.report?.geometry) {
  if (!state.geometryReady && state.report) {
    renderGeometryPlaceholder(geometry);
    preloadGeometryRenderer();
    return;
  }
  if (!geometry) {
    renderGeometryPlaceholder(null);
    return;
  }
  renderGeometryPlaceholder(geometry);
  loadGeometryModule()
    .then((module) => {
      if (geometry !== state.report?.geometry && state.report?.geometry) {
        return;
      }
      module.renderGeometry(geometry);
    })
    .catch((error) => {
      elements.geometryCanvasHost.innerHTML = `<div class="empty">${error?.message || String(error)}</div>`;
    });
}

export function resizeGeometry() {
  if (!geometryModule) {
    return;
  }
  geometryModule.resizeGeometry();
}

export function setGeometryMode(mode) {
  state.geometryMode = mode === "plan" ? "plan" : "3d";
  loadGeometryModule().then((module) => module.setGeometryMode(state.geometryMode));
}

export function setGeometryStory(storyIndex) {
  state.selectedGeometryStory = storyIndex === "all" ? "all" : Number(storyIndex) || 0;
  loadGeometryModule().then((module) => module.setGeometryStory(state.selectedGeometryStory));
}

export function setGeometrySelectionAid(enabled) {
  state.geometrySelectionAid = Boolean(enabled);
  if (elements.geometrySelectionAid) {
    elements.geometrySelectionAid.classList.toggle("active", state.geometrySelectionAid);
    elements.geometrySelectionAid.setAttribute("aria-pressed", String(state.geometrySelectionAid));
  }
  loadGeometryModule().then((module) => module.setGeometrySelectionAid(state.geometrySelectionAid));
}

configureResultPanelNavigationHooks("geometry", {
  getRoot: () => document.getElementById("geometryPane"),
  canReveal(selection, context) {
    const target = geometryViewTargetForSelection(selection, context.navigation);
    return Boolean(target && geometryTargetExists(target, state.report?.geometry)) || context.genericCanReveal(selection);
  },
  async reveal(selection, options, context) {
    const module = await loadGeometryModule();
    return module.revealGeometrySelection(selection, options, context);
  },
  captureContext(context) {
    return {
      ...context.genericCaptureContext(),
      mode: state.geometryMode === "plan" ? "plan" : "3d",
      story: state.selectedGeometryStory,
      selectedKind: state.selectedGeometryKind || "",
      selectedId: state.selectedGeometryId || "",
      selectionAid: Boolean(state.geometrySelectionAid),
      syncLocate: Boolean(state.geometrySyncLocate),
      visibility: {
        zones: Boolean(elements.geometryShowZones?.checked),
        walls: Boolean(elements.geometryShowWalls?.checked),
        windows: Boolean(elements.geometryShowWindows?.checked),
      },
    };
  },
  async restoreContext(snapshot, context) {
    const module = await loadGeometryModule();
    return module.restoreGeometryNavigationContext(snapshot, context);
  },
  preferredSemanticOccurrence(selection, context) {
    if (geometryModule) {
      return geometryModule.preferredGeometrySemanticOccurrence(selection, context);
    }
    return preferredGeometryOccurrenceFromTarget(selection, context) || context.genericPreferredSemanticOccurrence(selection);
  },
});

function geometryViewTargetForSelection(selection = {}, navigation = state.semanticProjection?.navigation || {}) {
  const direct = selection.viewTarget;
  if (String(direct?.view || "").toLowerCase() === "geometry" && direct.targetId) {
    return direct;
  }
  const occurrence = (navigation.occurrences || []).find((candidate) => candidate.occurrenceId === selection.occurrenceId);
  const entity = (navigation.entities || []).find((candidate) => candidate.id === selection.entityId);
  const targets = [...(occurrence?.viewTargets || []), ...(entity?.viewTargets || [])]
    .filter((target) => String(target?.view || "").toLowerCase() === "geometry" && target.targetId)
    .sort((left, right) => Number(right.priority || 0) - Number(left.priority || 0));
  if (selection.originView === "geometry" && selection.originTargetId) {
    return targets.find((target) => target.targetId === selection.originTargetId) || {
      view: "geometry",
      targetKind: selection.entityKind || "",
      targetId: selection.originTargetId,
    };
  }
  return targets[0] || null;
}

function geometryTargetExists(target, geometry) {
  if (!target || !geometry) {
    return false;
  }
  const kind = normalizeGeometryTargetKind(target.targetKind);
  const targetId = String(target.targetId || "");
  if (kind === "zone") {
    return (geometry.zones || []).some((item) => item.id === targetId);
  }
  if (kind === "space") {
    return (geometry.spaces || []).some((item) => item.id === targetId);
  }
  if (kind === "surface") {
    return (geometry.surfaces || []).some((item) => item.id === targetId);
  }
  if (kind === "window") {
    return (geometry.windows || []).some((item) => item.id === targetId);
  }
  if (kind === "story") {
    return (geometry.stories || []).some((story) => geometryStoryMatchesTarget(story, targetId));
  }
  return [geometry.zones, geometry.spaces, geometry.surfaces, geometry.windows]
    .some((items) => (items || []).some((item) => item.id === targetId));
}

function preferredGeometryOccurrenceFromTarget(selection, context) {
  const target = geometryViewTargetForSelection(selection, context.navigation);
  if (!target?.targetId) {
    return "";
  }
  const occurrenceIds = context.navigation.byViewTarget?.[`geometry|${target.targetId}`] || [];
  const occurrences = (context.navigation.occurrences || []).filter((occurrence) => occurrenceIds.includes(occurrence.occurrenceId));
  const currentPath = String(selection.semanticPathHint || state.semanticCurrentPath || "");
  return occurrences
    .map((occurrence, order) => ({
      occurrence,
      order,
      exact: Number(occurrence.occurrenceId === selection.occurrenceId),
      geometryContext: Number(occurrence.contextKind === "zone_geometry" || /(^|\/)geometry(\/|$)/.test(occurrence.path || "")),
      path: commonPathPrefixLength(occurrence.path, currentPath),
      preferred: Number(occurrence.preferredView === "geometry"),
    }))
    .sort((left, right) => (
      right.geometryContext - left.geometryContext ||
      right.exact - left.exact ||
      right.path - left.path ||
      right.preferred - left.preferred ||
      left.order - right.order
    ))[0]?.occurrence?.occurrenceId || "";
}

function normalizeGeometryTargetKind(kind) {
  const normalized = String(kind || "").toLowerCase();
  return normalized === "fenestration" ? "window" : normalized;
}

function geometryStoryMatchesTarget(story, targetId) {
  const normalized = String(targetId || "").trim().toLowerCase();
  return normalized === String(story.index) ||
    normalized === `story-${story.index}` ||
    normalized === String(story.name || "").trim().toLowerCase();
}

function commonPathPrefixLength(left, right) {
  const leftParts = String(left || "").split("/").filter(Boolean);
  const rightParts = String(right || "").split("/").filter(Boolean);
  let length = 0;
  while (length < leftParts.length && leftParts[length] === rightParts[length]) {
    length += 1;
  }
  return length;
}
