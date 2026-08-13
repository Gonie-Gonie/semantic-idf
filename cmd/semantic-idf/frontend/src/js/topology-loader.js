import {
  captureThermalTopologyState,
  elements,
  normalizeTopologyMode,
  state,
} from "./state.js";
import { t } from "./i18n.js";
import { configureResultPanelNavigationHooks } from "./panel-navigation-adapters.js";
import { recordViewHistory } from "./view-history.js";
import { isThermalTopologyTargetKind, thermalTopologyTargetExists } from "./thermal-topology-targets.js";

let topologyModule = null;
let topologyModulePromise = null;

function topologyStatsLabel(geometry) {
  return t("topology.stats", {
    zones: geometry?.zoneCount || 0,
    surfaces: geometry?.surfaceCount || 0,
    windows: geometry?.windowCount || 0,
  });
}

function renderTopologyPlaceholder(geometry) {
  if (!elements.topologyStats || !elements.topology3DCanvasHost) {
    return;
  }
  if (!state.geometryReady && state.report) {
    elements.topologyStats.textContent = t("topology.pending");
    elements.topology3DCanvasHost.innerHTML = `<div class="empty status-loading">${t("topology.running")}</div>`;
    elements.topologyPlan.innerHTML = "";
    elements.thermalTopologyGraph.innerHTML = "";
    elements.topologyDetails.removeAttribute("aria-labelledby");
    elements.topologyDetails.innerHTML = `<div class="empty">${t("topology.detailsReadySoon")}</div>`;
    return;
  }
  elements.topologyStats.textContent = topologyStatsLabel(geometry);
  if (!geometry) {
    elements.topology3DCanvasHost.innerHTML = `<div class="empty">${t("topology.noGeometry")}</div>`;
    elements.topologyPlan.innerHTML = "";
    elements.thermalTopologyGraph.innerHTML = "";
    elements.topologyDetails.removeAttribute("aria-labelledby");
    elements.topologyDetails.innerHTML = `<div class="empty">${t("topology.selectObject")}</div>`;
    return;
  }
  elements.topology3DCanvasHost.innerHTML = `<div class="empty status-loading">${t("topology.loadingRenderer")}</div>`;
  elements.topologyPlan.innerHTML = "";
}

async function loadTopologyModule() {
  if (topologyModule) {
    return topologyModule;
  }
  if (!topologyModulePromise) {
    topologyModulePromise = import("./views/topology-view.js").then((module) => {
      topologyModule = module;
      return module;
    });
  }
  return topologyModulePromise;
}

export function preloadTopologyRenderer() {
  return loadTopologyModule();
}

export function renderTopology(geometry = state.report?.geometry) {
  if (!state.geometryReady && state.report) {
    renderTopologyPlaceholder(geometry);
    preloadTopologyRenderer();
    return;
  }
  if (!geometry) {
    renderTopologyPlaceholder(null);
    return;
  }
  renderTopologyPlaceholder(geometry);
  loadTopologyModule()
    .then((module) => {
      if (geometry !== state.report?.geometry && state.report?.geometry) {
        return;
      }
      module.renderTopologyView(geometry);
    })
    .catch((error) => {
      elements.topology3DCanvasHost.innerHTML = `<div class="empty">${error?.message || String(error)}</div>`;
    });
}

export function resizeTopology() {
  if (!topologyModule) {
    return;
  }
  topologyModule.resizeTopologyView();
}

export function fitTopologyView() {
  return loadTopologyModule().then((module) => module.fitTopologyView());
}

export function setTopologyMode(mode) {
  const nextMode = normalizeTopologyMode(mode);
  if (nextMode === state.topologyMode) {
    return;
  }
  recordViewHistory();
  state.topologyMode = nextMode;
  loadTopologyModule().then((module) => module.setTopologyMode(state.topologyMode));
}

export function setTopologyStory(storyIndex) {
  const nextStory = storyIndex === "all" ? "all" : Number(storyIndex) || 0;
  if (nextStory === state.selectedTopologyStory) {
    return;
  }
  recordViewHistory();
  state.selectedTopologyStory = nextStory;
  loadTopologyModule().then((module) => module.setTopologyStory(state.selectedTopologyStory));
}

configureResultPanelNavigationHooks("topology", {
  getRoot: () => document.getElementById("topologyPane"),
  canReveal(selection, context) {
    const target = topologyViewTargetForSelection(selection, context.navigation);
    return Boolean(target && topologyTargetExists(target, state.report?.geometry)) || context.genericCanReveal(selection);
  },
  async reveal(selection, options, context) {
    const module = await loadTopologyModule();
    return module.revealTopologySelection(selection, options, context);
  },
  captureContext(context) {
    return {
      ...context.genericCaptureContext(),
      mode: normalizeTopologyMode(state.topologyMode),
      story: state.selectedTopologyStory,
      selectedKind: state.selectedTopologyEntityKind || "",
      selectedId: state.selectedTopologyEntityId || "",
      ...captureThermalTopologyState(state),
      visibility: { ...(state.topologyVisibility || {}) },
    };
  },
  async restoreContext(snapshot, context) {
    const module = await loadTopologyModule();
    return module.restoreTopologyNavigationContext(snapshot, context);
  },
  preferredSemanticOccurrence(selection, context) {
    if (topologyModule) {
      return topologyModule.preferredTopologySemanticOccurrence(selection, context);
    }
    return preferredTopologyOccurrenceFromTarget(selection, context) || context.genericPreferredSemanticOccurrence(selection);
  },
});

function topologyViewTargetForSelection(selection = {}, navigation = state.semanticProjection?.navigation || {}) {
  const direct = selection.viewTarget;
  if (String(direct?.view || "").toLowerCase() === "topology" && direct.targetId) {
    return direct;
  }
  const occurrence = (navigation.occurrences || []).find((candidate) => candidate.occurrenceId === selection.occurrenceId);
  const entity = (navigation.entities || []).find((candidate) => candidate.id === selection.entityId);
  const targets = [...(occurrence?.viewTargets || []), ...(entity?.viewTargets || [])]
    .filter((target) => String(target?.view || "").toLowerCase() === "topology" && target.targetId)
    .sort((left, right) => Number(right.priority || 0) - Number(left.priority || 0));
  if (selection.originView === "topology" && selection.originTargetId) {
    return targets.find((target) => target.targetId === selection.originTargetId) || {
      view: "topology",
      targetKind: selection.entityKind || "",
      targetId: selection.originTargetId,
    };
  }
  return targets[0] || null;
}

function topologyTargetExists(target, geometry) {
  if (!target || !geometry) {
    return false;
  }
  const kind = normalizeTopologyTargetKind(target.targetKind);
  const targetId = String(target.targetId || "");
  if (isThermalTopologyTargetKind(kind)) {
    return thermalTopologyTargetExists(target, geometry);
  }
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
    return (geometry.stories || []).some((story) => topologyStoryMatchesTarget(story, targetId));
  }
  return [geometry.zones, geometry.spaces, geometry.surfaces, geometry.windows]
    .some((items) => (items || []).some((item) => item.id === targetId));
}

function preferredTopologyOccurrenceFromTarget(selection, context) {
  const target = topologyViewTargetForSelection(selection, context.navigation);
  if (!target?.targetId) {
    return "";
  }
  const occurrenceIds = context.navigation.byViewTarget?.[`topology|${target.targetId}`] || [];
  const occurrences = (context.navigation.occurrences || []).filter((occurrence) => occurrenceIds.includes(occurrence.occurrenceId));
  const currentPath = String(selection.semanticPathHint || state.semanticCurrentPath || "");
  return occurrences
    .map((occurrence, order) => ({
      occurrence,
      order,
      contextPriority: thermalOccurrenceContextPriority(occurrence),
      exact: Number(occurrence.occurrenceId === selection.occurrenceId),
      geometryContext: Number(occurrence.contextKind === "zone_geometry" || /(^|\/)geometry(\/|$)/.test(occurrence.path || "")),
      path: commonPathPrefixLength(occurrence.path, currentPath),
      preferred: Number(occurrence.preferredView === "topology"),
    }))
    .sort((left, right) => (
      right.contextPriority - left.contextPriority ||
      right.geometryContext - left.geometryContext ||
      right.exact - left.exact ||
      right.path - left.path ||
      right.preferred - left.preferred ||
      left.order - right.order
    ))[0]?.occurrence?.occurrenceId || "";
}

function thermalOccurrenceContextPriority(occurrence) {
  const context = String(occurrence?.contextKind || "");
  if (context === "thermal_connection_context") return 4;
  if (context === "surface_boundary_context") return 3;
  if (context === "zone_geometry") {
    return /(^|\/)surfaces(\/|$)/.test(String(occurrence?.path || "")) ? 1 : 2;
  }
  return context === "definition" ? 1 : 0;
}

function normalizeTopologyTargetKind(kind) {
  const normalized = String(kind || "").toLowerCase();
  return normalized === "fenestration" ? "window" : normalized;
}

function topologyStoryMatchesTarget(story, targetId) {
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
