import { elements, state } from "./state.js";

let geometryModule = null;
let geometryModulePromise = null;

function geometryStatsLabel(geometry) {
  return `${geometry?.zoneCount || 0} zones, ${geometry?.surfaceCount || 0} surfaces, ${geometry?.windowCount || 0} windows`;
}

function renderGeometryPlaceholder(geometry) {
  if (!elements.geometryStats || !elements.geometryCanvasHost) {
    return;
  }
  if (!state.geometryReady && state.report) {
    elements.geometryStats.textContent = "Geometry pending";
    elements.geometryCanvasHost.innerHTML = `<div class="empty status-loading">Geometry analysis is running</div>`;
    elements.geometryPlan.innerHTML = "";
    elements.geometryDetails.innerHTML = `<div class="empty">Geometry will be ready shortly</div>`;
    return;
  }
  elements.geometryStats.textContent = geometryStatsLabel(geometry);
  if (!geometry) {
    elements.geometryCanvasHost.innerHTML = `<div class="empty">No geometry yet</div>`;
    elements.geometryPlan.innerHTML = "";
    elements.geometryDetails.innerHTML = `<div class="empty">Select a zone, wall, or window</div>`;
    return;
  }
  elements.geometryCanvasHost.innerHTML = `<div class="empty status-loading">Loading geometry renderer</div>`;
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
