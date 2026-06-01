import { elements, state } from "./state.js";
import { t } from "./i18n.js";

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
    geometryModulePromise = import("./geometry-view.js").then((module) => {
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
