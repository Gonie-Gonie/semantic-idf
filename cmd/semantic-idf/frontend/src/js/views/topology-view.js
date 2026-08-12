import * as THREE from "../../vendor/three.module.js";
import {
  elements,
  escapeHTML,
  normalizeTopologyMode,
  restoreThermalTopologyState,
  state,
} from "../state.js";
import { t } from "../i18n.js";
import { refreshResultPanelSelectionStyles } from "../panel-navigation-adapters.js";
import { clearSemanticHover, hoverSemanticEntity, selectSemanticEntity } from "../selection-controller.js";
import { resolveThermalTopologyTarget } from "../thermal-topology-targets.js";

let rendererState = null;
let temporaryTopologyReveal = null;
let temporaryTopologyHover = null;
let topologyHoverTargetKey = "";
let topologySelectionRequest = 0;
let thermalTopologyModule = null;
let thermalTopologyModulePromise = null;
let thermalTopologyRenderRequest = 0;
let topologyPlanInteractionsBound = false;
let topologyDetailInteractionsBound = false;
let geometryLookupCache = null;
let topologyNavigationLookupCache = null;

const EMPTY_GEOMETRY_ITEMS = Object.freeze([]);

window.addEventListener("idfAnalyzer:semanticSelectionChanged", (event) => {
  if (!temporaryTopologyReveal) {
    return;
  }
  const selection = topologySelectionForTarget(temporaryTopologyReveal.kind, temporaryTopologyReveal.id);
  if (!selection?.entityId || selection.entityId !== event.detail?.selection?.entityId) {
    temporaryTopologyReveal = null;
    if (state.activeResultTab === "topology" && state.report?.geometry) {
      renderTopologyView();
    }
  }
});
window.addEventListener("idfAnalyzer:documentChanged", () => {
  temporaryTopologyReveal = null;
  temporaryTopologyHover = null;
  topologyHoverTargetKey = "";
  geometryLookupCache = null;
  topologyNavigationLookupCache = null;
  topologySelectionRequest += 1;
});
window.addEventListener("idfAnalyzer:semanticHoverChanged", (event) => {
  temporaryTopologyHover = topologyProjectionForSemanticSelection(event.detail?.hover, state.report?.geometry);
  highlightSelectedMeshes();
  highlightSelectedPlan();
});

export function renderTopologyView(geometry = state.report?.geometry) {
  if (!elements.topologyStats || !elements.topology3DCanvasHost) {
    return;
  }
  if (!geometry) {
    renderEmptyTopology();
    return;
  }

  ensureSelectedStory(geometry);
  if (elements.topologySyncLocate) {
    elements.topologySyncLocate.checked = state.topologySyncLocate;
  }
  syncTopologyVisibilityControls();
  elements.topologyStats.textContent = t("topology.stats", {
    zones: geometry.zoneCount || 0,
    surfaces: geometry.surfaceCount || 0,
    windows: geometry.windowCount || 0,
  });
  renderStoryOptions(geometry);
  updateModeVisibility();
  if (state.topologyMode === "plan") {
    renderPlan(geometry);
  } else if (state.topologyMode === "3d") {
    renderScene(geometry);
  } else {
    renderThermalTopologyLazy(geometry);
  }
  renderTopologyDetails(geometry);
}

export function setTopologyMode(mode) {
  state.topologyMode = normalizeTopologyMode(mode);
  if (state.topologyMode === "plan" && state.selectedTopologyStory === "all") {
    state.selectedTopologyStory = firstStoryIndex(state.report?.geometry);
  }
  elements.topologyModeButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.topologyMode === state.topologyMode);
  });
  renderTopologyView();
}

export function fitTopologyView() {
  if (state.topologyMode === "thermal") {
    return loadThermalTopologyModule().then((module) => module.fitThermalTopology());
  }
  renderTopologyView();
  return Promise.resolve(true);
}

export function setTopologyStory(storyIndex) {
  state.selectedTopologyStory = storyIndex === "all" ? "all" : Number(storyIndex) || 0;
  renderTopologyView();
}

export async function selectTopologyEntity(kind, id, options = {}) {
  const request = ++topologySelectionRequest;
  const geometry = state.report?.geometry;
  const normalizedKind = normalizeGeometryKind(kind);
  const targetId = String(id || "");
  const entity = topologyTargetEntity({ targetKind: normalizedKind, targetId }, geometry);
  const selection = topologySelectionForTarget(normalizedKind, targetId);
  const syncLocate = options.syncLocate !== false && state.topologySyncLocate && Boolean(entity);

  // The legacy source locator already records a history entry. Run it before
  // changing either selection so a geometry click remains one atomic action.
  if (syncLocate) {
    syncLocatedInputEntity(entity);
  }
  if (options.syncSemantic !== false && selection) {
    await selectSemanticEntity(selection, {
      originView: "topology",
      action: "select",
      recordHistory: syncLocate ? false : options.recordHistory !== false,
      follow: options.follow,
      rememberForOriginView: "topology",
    });
  }
  if (request !== topologySelectionRequest) {
    return;
  }

  temporaryTopologyReveal = null;
  state.selectedTopologyEntityKind = normalizedKind;
  state.selectedTopologyEntityId = targetId;
  projectGeometrySelectionToThermal(normalizedKind, targetId, geometry);
  renderTopologyDetails();
  highlightSelectedMeshes();
  highlightSelectedPlan();
  refreshResultPanelSelectionStyles("topology", state.globalSelection, state.globalHover);
}

export async function revealTopologySelection(selection, options = {}, context) {
  topologySelectionRequest += 1;
  const geometry = state.report?.geometry;
  const target = topologyViewTargetForSelection(selection, context?.navigation);
  const entity = topologyTargetEntity(target, geometry);
  if (!target || !entity) {
    return context?.genericReveal(selection, options) || false;
  }

  if (entity.kind === "story") {
    state.selectedTopologyStory = entity.item.index;
    temporaryTopologyReveal = null;
    renderTopologyView(geometry);
    const storySelect = elements.topologyStorySelect;
    if (options.focus !== false) {
      storySelect?.focus?.({ preventScroll: true });
    }
    context?.refreshSelectionStyles(selection, state.globalHover);
    return Boolean(storySelect);
  }

  const ownerZone = owningZoneForGeometryEntity(entity, geometry);
  const storyIndex = geometryStoryIndexForEntity(entity, geometry);
  if (Number.isInteger(storyIndex)) {
    state.selectedTopologyStory = storyIndex;
  }
  if (entity.thermalTarget) {
    state.topologyMode = "thermal";
    state.thermalTopologySelectedEntityId = entity.id;
    state.thermalTopologySelectedEntityKind = entity.kind;
  } else if (state.topologyMode === "plan" && !geometryEntityHasPlanShape(entity, geometry)) {
    state.topologyMode = "3d";
  }
  temporaryTopologyReveal = {
    kind: entity.kind,
    id: entity.id,
    ownerZoneId: ownerZone?.id || "",
    baseSurfaceId: entity.kind === "window" ? baseSurfaceForWindow(geometry, entity.item)?.id || "" : "",
    surfaceIds: [...(entity.thermalTarget?.surfaceIds || [])],
    primarySurfaceId: entity.thermalTarget?.surfaceIds?.[0] || "",
    windowIds: [...(entity.thermalTarget?.windowIds || [])],
    nodeIds: [...(entity.thermalTarget?.nodeIds || [])],
  };
  state.selectedTopologyEntityKind = entity.kind;
  state.selectedTopologyEntityId = entity.id;
  projectGeometrySelectionToThermal(entity.kind, entity.id, geometry);
  renderTopologyView(geometry);
  context?.refreshSelectionStyles(selection, state.globalHover);
  await nextTopologyFrame();
  const targetElement = findTopologyNavigationTarget(selection, target, context) || elements.topologyDetails;
  if (options.scroll !== false) {
    targetElement?.scrollIntoView?.({ block: options.block || "nearest", inline: "nearest", behavior: options.behavior || "auto" });
  }
  if (options.focus !== false) {
    targetElement?.focus?.({ preventScroll: true });
  }
  return Boolean(targetElement);
}

export async function restoreTopologyNavigationContext(snapshot = {}, context) {
  topologySelectionRequest += 1;
  state.topologyMode = normalizeTopologyMode(snapshot.mode);
  restoreThermalTopologyState(snapshot, state);
  state.selectedTopologyStory = snapshot.story === "all" ? "all" : Number(snapshot.story) || 0;
  state.selectedTopologyEntityKind = normalizeGeometryKind(snapshot.selectedKind);
  state.selectedTopologyEntityId = String(snapshot.selectedId || "");
  state.topologySyncLocate = snapshot.syncLocate !== false;
  const legacyVisibility = snapshot.visibility || {};
  const visibility3D = snapshot.visibility3D || legacyVisibility;
  const visibilityPlan = snapshot.visibilityPlan || legacyVisibility;
  state.topology3DVisibility = {
    zones: visibility3D.zones !== false,
    surfaces: (visibility3D.surfaces ?? visibility3D.walls) !== false,
    openings: (visibility3D.openings ?? visibility3D.windows) !== false,
  };
  state.topologyPlanVisibility = {
    zones: visibilityPlan.zones !== false,
    boundaries: (visibilityPlan.boundaries ?? visibilityPlan.walls) !== false,
    openings: (visibilityPlan.openings ?? visibilityPlan.windows) !== false,
  };
  temporaryTopologyReveal = null;
  renderTopologyView();
  return context?.genericRestoreContext(snapshot) ?? true;
}

export function preferredTopologySemanticOccurrence(selection, context) {
  const target = topologyViewTargetForSelection(selection, context?.navigation);
  const preferred = preferredOccurrenceForTopologyTarget(target?.targetId, selection, context?.navigation);
  return preferred?.occurrenceId || context?.genericPreferredSemanticOccurrence(selection) || "";
}

export function resizeTopologyView() {
  if (!rendererState || state.topologyMode !== "3d") {
    return;
  }
  resizeRenderer();
  rendererState.renderer.render(rendererState.scene, rendererState.camera);
}

export function revealThermalTargetInTopology(kind, id, mode = "3d") {
  const geometry = state.report?.geometry;
  const thermalTarget = resolveThermalTopologyTarget({ targetKind: kind, targetId: id }, geometry);
  if (!thermalTarget) return false;
  const lookup = geometryLookupIndex(geometry);
  const representativeSurface = lookup.surfaceByID.get((thermalTarget.surfaceIds || []).find((surfaceID) => lookup.surfaceByID.has(surfaceID)));
  const representativeWindow = lookup.windowByID.get((thermalTarget.windowIds || []).find((windowID) => lookup.windowByID.has(windowID)));
  const representativeNodeID = (thermalTarget.nodeIds || []).find((nodeID) => lookup.zoneByID.has(nodeID) || lookup.spaceByID.has(nodeID));
  const representativeNode = lookup.zoneByID.get(representativeNodeID) || lookup.spaceByID.get(representativeNodeID);
  const storyIndex = representativeSurface?.storyIndex ?? representativeWindow?.storyIndex ?? representativeNode?.storyIndex;
  if (Number.isInteger(storyIndex)) state.selectedTopologyStory = storyIndex;
  temporaryTopologyReveal = {
    kind,
    id,
    ownerZoneId: thermalTarget.nodeIds?.find((nodeID) => lookup.zoneByID.has(nodeID)) || "",
    baseSurfaceId: representativeWindow ? baseSurfaceForWindow(geometry, representativeWindow)?.id || "" : "",
    surfaceIds: [...(thermalTarget.surfaceIds || [])],
    primarySurfaceId: thermalTarget.surfaceIds?.[0] || "",
    windowIds: [...(thermalTarget.windowIds || [])],
    nodeIds: [...(thermalTarget.nodeIds || [])],
  };
  state.topologyMode = normalizeTopologyMode(mode);
  renderTopologyView(geometry);
  return true;
}

function renderEmptyTopology() {
  elements.topologyStats.textContent = t("topology.stats", { zones: 0, surfaces: 0, windows: 0 });
  elements.topologyStorySelect.innerHTML = "";
  elements.topology3DCanvasHost.innerHTML = `<div class="empty">${t("topology.noGeometry")}</div>`;
  elements.topologyPlan.innerHTML = "";
  elements.thermalTopologyGraph.innerHTML = `<div class="empty">${t("topology.noConnections")}</div>`;
  elements.thermalTopologyInspector.innerHTML = "";
  elements.topologyDetails.innerHTML = `<div class="empty">${t("topology.selectObject")}</div>`;
}

function ensureSelectedStory(geometry) {
  const stories = geometry.stories || [];
  const requiresSpecificStory = state.topologyMode === "plan"
    || (state.topologyMode === "thermal" && state.thermalTopologyScope === "story");
  if (!stories.length) {
    state.selectedTopologyStory = requiresSpecificStory ? 0 : "all";
    return;
  }
  if (!requiresSpecificStory && state.selectedTopologyStory === "all") {
    return;
  }
  const exists = stories.some((story) => story.index === state.selectedTopologyStory);
  if (!exists) {
    state.selectedTopologyStory = stories[0].index;
  }
}

function renderStoryOptions(geometry) {
  const stories = geometry.stories || [];
  const allOption =
    state.topologyMode === "3d"
      ? `<option value="all" ${state.selectedTopologyStory === "all" ? "selected" : ""}>${t("topology.allLevels")}</option>`
      : "";
  const storyOptions = stories
    .map(
      (story) =>
        `<option value="${escapeHTML(story.index)}" ${topologyNavigationAttributes("story", geometryStoryTargetID(story), { objectName: story.name }, { tabindex: false })} ${story.index === state.selectedTopologyStory ? "selected" : ""}>${escapeHTML(story.name)} (${formatNumber(story.elevation)} m)</option>`,
    )
    .join("");
  elements.topologyStorySelect.innerHTML = `${allOption}${storyOptions}`;
}

function updateModeVisibility() {
  const is3D = state.topologyMode === "3d";
  const isPlan = state.topologyMode === "plan";
  const isNetwork = state.topologyMode === "thermal";
  elements.topology3DCanvasHost.classList.toggle("active", is3D);
  elements.topologyPlan.classList.toggle("active", isPlan);
  elements.thermalTopologyView?.classList.toggle("active", isNetwork);
  elements.topology3DControls.hidden = !is3D;
  elements.topologyPlanControls.hidden = !isPlan;
  elements.thermalTopologyControls.hidden = !isNetwork;
  elements.topologyStoryControl.hidden = isNetwork && state.thermalTopologyScope !== "story";
  elements.topologyViewport.classList.toggle("network-active", isNetwork);
  elements.topologyModeButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.topologyMode === state.topologyMode);
  });
}

function renderThermalTopologyLazy(geometry) {
  const request = ++thermalTopologyRenderRequest;
  if (!thermalTopologyModule) {
    elements.thermalTopologyGraph.innerHTML = `<div class="thermal-topology-shell-status status-loading">${escapeHTML(t("topology.loadingGraph"))}</div>`;
  }
  loadThermalTopologyModule()
    .then((module) => {
      if (request !== thermalTopologyRenderRequest || state.topologyMode !== "thermal" || geometry !== state.report?.geometry) {
        return;
      }
      module.renderThermalTopology(geometry, {
        navigationAttributes: topologyNavigationAttributes,
        selectTopologyEntity,
        setTopologyMode,
        revealThermalTargetInTopology,
        selectionForTarget: topologySelectionForTarget,
      });
    })
    .catch((error) => {
      if (request === thermalTopologyRenderRequest && state.topologyMode === "thermal") {
        elements.thermalTopologyGraph.innerHTML = `<div class="thermal-topology-shell-status">${escapeHTML(error?.message || String(error))}</div>`;
      }
    });
}

function loadThermalTopologyModule() {
  if (thermalTopologyModule) {
    return Promise.resolve(thermalTopologyModule);
  }
  if (!thermalTopologyModulePromise) {
    thermalTopologyModulePromise = import("./thermal-topology-view.js").then((module) => {
      thermalTopologyModule = module;
      return module;
    });
  }
  return thermalTopologyModulePromise;
}

function syncTopologyVisibilityControls() {
  const visibility3D = state.topology3DVisibility || {};
  const visibilityPlan = state.topologyPlanVisibility || {};
  elements.topology3DShowZones.checked = visibility3D.zones !== false;
  elements.topology3DShowSurfaces.checked = visibility3D.surfaces !== false;
  elements.topology3DShowOpenings.checked = visibility3D.openings !== false;
  elements.topologyPlanShowZones.checked = visibilityPlan.zones !== false;
  elements.topologyPlanShowBoundaries.checked = visibilityPlan.boundaries !== false;
  elements.topologyPlanShowOpenings.checked = visibilityPlan.openings !== false;
}

function renderScene(geometry) {
  elements.topologyPlan.innerHTML = "";
  ensureRenderer();
  const { scene, group, camera, renderer } = rendererState;
  scene.background = new THREE.Color(geometryColor("background", 0xf7fafc));
  clearGroup(group);
  group.rotation.set(-0.22, 0.72, 0);

  const bounds = geometry.bounds || {};
  const center = bounds.ok
    ? new THREE.Vector3((bounds.minX + bounds.maxX) / 2, (bounds.minZ + bounds.maxZ) / 2, (bounds.minY + bounds.maxY) / 2)
    : new THREE.Vector3();
  const modelSize = bounds.ok
    ? Math.max(bounds.maxX - bounds.minX, bounds.maxY - bounds.minY, bounds.maxZ - bounds.minZ, 1)
    : 18;
  const visibility = state.topology3DVisibility || {};

  (geometry.surfaces || [])
    .filter((surface) => (
      matchesSelectedStory(surface) &&
      surface.surfaceType?.toLowerCase() === "floor" &&
      (visibility.zones !== false || geometryZoneSurfaceIsTemporarilyVisible(surface, geometry))
    ))
    .forEach((surface) => addSurfaceMesh(group, surface, "zone", zoneIdForName(geometry, surface.zoneName), center));
  (geometry.surfaces || [])
    .filter((surface) => (
      matchesSelectedStory(surface) &&
      surface.surfaceType?.toLowerCase() !== "floor" &&
      (visibility.surfaces !== false || geometrySurfaceIsTemporarilyVisible(surface))
    ))
    .forEach((surface) => addSurfaceMesh(group, surface, "surface", surface.id, center));
  (geometry.windows || [])
    .filter((windowItem) => matchesSelectedStory(windowItem) && (visibility.openings !== false || geometryWindowIsTemporarilyVisible(windowItem)))
    .forEach((windowItem) => addWindowMesh(group, windowItem, center));

  addAxes(group, bounds, center);
  resizeRenderer();
  camera.position.set(0, modelSize * 0.72, modelSize * 1.65);
  camera.near = 0.1;
  camera.far = modelSize * 10;
  camera.lookAt(0, 0, 0);
  camera.updateProjectionMatrix();
  highlightSelectedMeshes();
  renderer.render(scene, camera);
  window.requestAnimationFrame(() => {
    if (rendererState?.renderer !== renderer || state.topologyMode !== "3d") {
      return;
    }
    resizeRenderer();
    renderer.render(scene, camera);
  });
}

function ensureRenderer() {
  if (rendererState) {
    if (!elements.topology3DCanvasHost.contains(rendererState.renderer.domElement)) {
      elements.topology3DCanvasHost.innerHTML = "";
      elements.topology3DCanvasHost.appendChild(rendererState.renderer.domElement);
    }
    return;
  }

  const scene = new THREE.Scene();
  scene.background = new THREE.Color(geometryColor("background", 0xf7fafc));
  const camera = new THREE.PerspectiveCamera(42, 1, 0.1, 1000);
  const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: false, preserveDrawingBuffer: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
  renderer.domElement.className = "topology-3d-canvas";
  elements.topology3DCanvasHost.innerHTML = "";
  elements.topology3DCanvasHost.appendChild(renderer.domElement);

  const group = new THREE.Group();
  scene.add(group);
  scene.add(new THREE.HemisphereLight(0xffffff, 0xd7dee4, 2));
  const light = new THREE.DirectionalLight(0xffffff, 1.5);
  light.position.set(20, 40, 24);
  scene.add(light);

  rendererState = {
    scene,
    camera,
    renderer,
    group,
    raycaster: new THREE.Raycaster(),
    pointer: new THREE.Vector2(),
    dragStart: null,
    dragging: false,
    resizeFrame: 0,
  };
  installCanvasInteractions();
  installCanvasResizeObserver();
}

function installCanvasResizeObserver() {
  if (!window.ResizeObserver) {
    return;
  }
  rendererState.resizeObserver = new ResizeObserver(() => {
    if (rendererState.resizeFrame) {
      return;
    }
    rendererState.resizeFrame = window.requestAnimationFrame(() => {
      rendererState.resizeFrame = 0;
      if (!rendererState || state.topologyMode !== "3d" || !elements.topology3DCanvasHost.contains(rendererState.renderer.domElement)) {
        return;
      }
      resizeRenderer();
      rendererState.renderer.render(rendererState.scene, rendererState.camera);
    });
  });
  rendererState.resizeObserver.observe(elements.topology3DCanvasHost);
}

function installCanvasInteractions() {
  const canvas = rendererState.renderer.domElement;
  canvas.addEventListener("pointerdown", (event) => {
    rendererState.dragStart = { x: event.clientX, y: event.clientY, rx: rendererState.group.rotation.x, ry: rendererState.group.rotation.y };
    rendererState.dragging = false;
    canvas.setPointerCapture(event.pointerId);
  });
  canvas.addEventListener("pointermove", (event) => {
    if (!rendererState.dragStart) {
      hoverMesh(event);
      return;
    }
    const dx = event.clientX - rendererState.dragStart.x;
    const dy = event.clientY - rendererState.dragStart.y;
    if (Math.abs(dx) + Math.abs(dy) > 3) {
      rendererState.dragging = true;
    }
    rendererState.group.rotation.y = rendererState.dragStart.ry + dx * 0.01;
    rendererState.group.rotation.x = rendererState.dragStart.rx + dy * 0.006;
    rendererState.renderer.render(rendererState.scene, rendererState.camera);
  });
  canvas.addEventListener("pointerleave", clearGeometryHover);
  canvas.addEventListener("pointerup", (event) => {
    canvas.releasePointerCapture(event.pointerId);
    const wasDragging = rendererState.dragging;
    rendererState.dragStart = null;
    rendererState.dragging = false;
    if (!wasDragging) {
      pickMesh(event);
    }
  });
  canvas.addEventListener("wheel", (event) => {
    event.preventDefault();
    const scale = event.deltaY > 0 ? 1.08 : 0.92;
    rendererState.camera.position.multiplyScalar(scale);
    rendererState.renderer.render(rendererState.scene, rendererState.camera);
  }, { passive: false });
}

function hoverMesh(event) {
  const hit = meshAtPointer(event);
  const kind = hit?.object.userData?.geometryKind || "";
  const id = hit?.object.userData?.geometryId || "";
  const key = `${kind}|${id}`;
  if (key === topologyHoverTargetKey) return;
  topologyHoverTargetKey = key;
  if (!id) {
    clearSemanticHover({ originView: "topology", action: "hover" });
    return;
  }
  const selection = topologySelectionForTarget(kind, id);
  if (selection) hoverSemanticEntity(selection, { originView: "topology", action: "hover", recordHistory: false, follow: false });
}

function clearGeometryHover() {
  if (!topologyHoverTargetKey) return;
  topologyHoverTargetKey = "";
  clearSemanticHover({ originView: "topology", action: "hover" });
}

function meshAtPointer(event) {
  const rect = rendererState.renderer.domElement.getBoundingClientRect();
  rendererState.pointer.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
  rendererState.pointer.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;
  rendererState.raycaster.setFromCamera(rendererState.pointer, rendererState.camera);
  return rendererState.raycaster.intersectObjects(rendererState.group.children, true).find((item) => item.object.userData?.geometryId) || null;
}

function pickMesh(event) {
  const hit = meshAtPointer(event);
  if (hit) {
    void selectTopologyEntity(hit.object.userData.geometryKind, hit.object.userData.geometryId);
  }
}

function addSurfaceMesh(group, surface, kind, id, center) {
  const geometry = polygonGeometry(surface.worldVertices, center, 0);
  if (!geometry) {
    return;
  }
  const isZone = kind === "zone";
  const isRoof = /roof|ceiling/i.test(surface.surfaceType || "");
  const isFloor = /floor/i.test(surface.surfaceType || "");
  const baseColor = isZone
    ? geometryColor("zone", 0xb8d7b0)
    : isRoof
      ? geometryColor("roof", 0xb8b0a1)
      : isFloor
        ? 0x98b8a7
        : geometryColor("wall", 0x7b9cbc);
  const material = new THREE.MeshStandardMaterial({
    color: baseColor,
    emissive: 0x000000,
    emissiveIntensity: 0,
    roughness: 0.72,
    metalness: 0,
    transparent: true,
    opacity: isZone ? 0.5 : 0.72,
    depthWrite: true,
    depthTest: true,
    side: THREE.DoubleSide,
  });
  const mesh = new THREE.Mesh(geometry, material);
  mesh.userData = {
    geometryKind: kind,
    geometryId: id,
    semanticSelection: topologySelectionForTarget(kind, id),
    baseColor: material.color.getHex(),
    baseOpacity: material.opacity,
  };
  group.add(mesh);
}

function addWindowMesh(group, windowItem, center) {
  const geometry = polygonGeometry(windowItem.worldVertices, center, 0.035);
  if (!geometry) {
    return;
  }
  const windowColor = geometryColor("window", 0x3fb6d4);
  const material = new THREE.MeshStandardMaterial({
    color: windowColor,
    emissive: new THREE.Color(windowColor).multiplyScalar(0.45).getHex(),
    emissiveIntensity: 0.18,
    roughness: 0.35,
    transparent: true,
    opacity: 0.82,
    side: THREE.DoubleSide,
  });
  const mesh = new THREE.Mesh(geometry, material);
  mesh.userData = {
    geometryKind: "window",
    geometryId: windowItem.id,
    semanticSelection: topologySelectionForTarget("window", windowItem.id),
    baseColor: material.color.getHex(),
    baseOpacity: material.opacity,
  };
  group.add(mesh);
}

function polygonGeometry(points, center, offset) {
  if (!points || points.length < 3) {
    return null;
  }
  const vertices = points.map((point) => new THREE.Vector3(point.x - center.x, point.z - center.y, point.y - center.z));
  if (offset) {
    const normal = new THREE.Vector3()
      .crossVectors(vertices[1].clone().sub(vertices[0]), vertices[2].clone().sub(vertices[0]))
      .normalize()
      .multiplyScalar(offset);
    vertices.forEach((vertex) => vertex.add(normal));
  }
  const positions = [];
  const indexes = [];
  vertices.forEach((vertex) => positions.push(vertex.x, vertex.y, vertex.z));
  for (let index = 1; index < vertices.length - 1; index += 1) {
    indexes.push(0, index, index + 1);
  }
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute("position", new THREE.Float32BufferAttribute(positions, 3));
  geometry.setIndex(indexes);
  geometry.computeVertexNormals();
  return geometry;
}

function addAxes(group, bounds, center) {
  if (!bounds.ok) {
    return;
  }
  const length = Math.max(bounds.maxX - bounds.minX, bounds.maxY - bounds.minY, 4) * 0.12;
  const origin = new THREE.Vector3(bounds.minX - center.x, bounds.minZ - center.y, bounds.minY - center.z);
  const materialX = new THREE.LineBasicMaterial({ color: 0xb3261e });
  const materialY = new THREE.LineBasicMaterial({ color: 0x246b44 });
  const xLine = new THREE.Line(new THREE.BufferGeometry().setFromPoints([origin, origin.clone().add(new THREE.Vector3(length, 0, 0))]), materialX);
  const yLine = new THREE.Line(new THREE.BufferGeometry().setFromPoints([origin, origin.clone().add(new THREE.Vector3(0, 0, length))]), materialY);
  group.add(xLine, yLine);
}

function renderPlan(geometry) {
  disposeRendererCanvas();
  bindGeometryPlanInteractions();
  const storyIndex = state.selectedTopologyStory === "all" ? firstStoryIndex(geometry) : state.selectedTopologyStory;
  const layout = cachedGeometryPlanLayout(geometry, storyIndex);
  if (!layout.ok) {
    elements.topologyPlan.innerHTML = `<text x="24" y="42" fill="#60707c" font-size="14">${t("topology.noFloorPlan")}</text>`;
    elements.topologyPlan.setAttribute("viewBox", "0 0 640 420");
    return;
  }

  const visibility = state.topologyPlanVisibility || {};
  const zoneFloorPolygons = visibility.zones !== false
    ? layout.surfaces
        .filter((surface) => surface.isFloor)
        .map((surface) => `<polygon class="plan-zone navigable-row" data-geometry-kind="zone" data-geometry-id="${escapeHTML(surface.zoneID)}" ${topologyNavigationAttributes("zone", surface.zoneID)} points="${surface.openPoints}"></polygon>`)
        .join("")
    : layout.surfaces
        .filter((surface) => surface.isFloor && temporaryTopologyReveal?.ownerZoneId === surface.zoneID)
        .map((surface) => `<polygon class="plan-zone navigable-row" data-geometry-kind="zone" data-geometry-id="${escapeHTML(surface.zoneID)}" ${topologyNavigationAttributes("zone", surface.zoneID)} points="${surface.openPoints}"></polygon>`)
        .join("");
  const wallLines = layout.surfaces
    .filter((surface) => (
      !surface.isFloor &&
      (visibility.boundaries !== false || projectedSurfaceIsTemporarilyVisible(surface))
    ))
    .map(renderPlanSurfaceShape)
    .join("");
  const windowLines = layout.windows
    .filter((windowItem) => visibility.openings !== false || temporaryTopologyReveal?.id === windowItem.id)
    .map((windowItem) => `<polyline class="plan-window navigable-row" data-geometry-kind="window" data-geometry-id="${escapeHTML(windowItem.id)}" ${topologyNavigationAttributes("fenestration", windowItem.id)} points="${windowItem.closedPoints}"></polyline>`)
    .join("");

  elements.topologyPlan.setAttribute("viewBox", `0 0 ${layout.viewWidth} ${layout.viewHeight}`);
  elements.topologyPlan.innerHTML = `${zoneFloorPolygons}${wallLines}${windowLines}`;
  highlightSelectedPlan();
}

function bindGeometryPlanInteractions() {
  const plan = elements.topologyPlan;
  if (topologyPlanInteractionsBound || !plan) return;
  topologyPlanInteractionsBound = true;
  plan.addEventListener("click", (event) => {
    const shape = geometryPlanTarget(event.target, plan);
    if (!shape) return;
    event.stopPropagation();
    void selectTopologyEntity(shape.dataset.geometryKind, shape.dataset.geometryId);
  });
  plan.addEventListener("keydown", (event) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    const shape = geometryPlanTarget(event.target, plan);
    if (!shape) return;
    event.preventDefault();
    event.stopPropagation();
    void selectTopologyEntity(shape.dataset.geometryKind, shape.dataset.geometryId);
  });
  plan.addEventListener("pointerover", (event) => {
    const shape = geometryPlanTarget(event.target, plan);
    if (!shape || shape === geometryPlanTarget(event.relatedTarget, plan)) return;
    const selection = topologySelectionForTarget(shape.dataset.geometryKind, shape.dataset.geometryId);
    if (selection) hoverSemanticEntity(selection, { originView: "topology", action: "hover", recordHistory: false, follow: false });
  });
  plan.addEventListener("pointerout", (event) => {
    const shape = geometryPlanTarget(event.target, plan);
    if (!shape || shape === geometryPlanTarget(event.relatedTarget, plan)) return;
    clearSemanticHover({ originView: "topology", action: "hover" });
  });
}

function geometryPlanTarget(target, plan) {
  const shape = target?.closest?.("[data-geometry-id]") || null;
  return shape && plan.contains(shape) ? shape : null;
}

function cachedGeometryPlanLayout(geometry, storyIndex) {
  const cache = state.geometryPlanLayoutCache || new Map();
  state.geometryPlanLayoutCache = cache;
  const key = geometryPlanLayoutCacheKey(geometry, storyIndex);
  if (cache.has(key)) {
    const cached = cache.get(key);
    cache.delete(key);
    cache.set(key, cached);
    return cached;
  }
  const layout = buildGeometryPlanLayout(geometry, storyIndex);
  cache.set(key, layout);
  while (cache.size > 8) {
    cache.delete(cache.keys().next().value);
  }
  return layout;
}

function geometryPlanLayoutCacheKey(geometry, storyIndex) {
  const bounds = geometry.bounds || {};
  return [
    state.analysisKey || state.lastAnalyzedKey || "",
    storyIndex,
    (geometry.surfaces || []).length,
    (geometry.windows || []).length,
    bounds.ok ? [bounds.minX, bounds.minY, bounds.maxX, bounds.maxY].map((value) => Number(value || 0).toFixed(3)).join(",") : "no-bounds",
  ].join("|");
}

function buildGeometryPlanLayout(geometry, storyIndex) {
  const surfaces = (geometry.surfaces || []).filter((surface) => surface.storyIndex === storyIndex && hasPlanVertices(surface));
  const windows = (geometry.windows || []).filter((windowItem) => windowItem.storyIndex === storyIndex && hasPlanVertices(windowItem));
  const bounds = geometry.bounds || {};
  if (!bounds.ok || (!surfaces.length && !windows.length)) {
    return { ok: false, viewWidth: 640, viewHeight: 420, surfaces: [], windows: [] };
  }

  const pad = 18;
  const width = Math.max(bounds.maxX - bounds.minX, 1);
  const height = Math.max(bounds.maxY - bounds.minY, 1);
  const viewWidth = 760;
  const viewHeight = Math.max(360, Math.round((height / width) * 760));
  const scale = Math.min((viewWidth - pad * 2) / width, (viewHeight - pad * 2) / height);
  const project = (point) => `${pad + (point.x - bounds.minX) * scale},${viewHeight - pad - (point.y - bounds.minY) * scale}`;
  const projectedSurfaces = surfaces.map((surface) => {
    const openPoints = surface.worldVertices.map(project).join(" ");
    return {
      id: surface.id,
      title: `${surface.name || surface.type} / ${surface.surfaceType || "Surface"}`,
      className: `plan-surface ${planSurfaceClass(surface)}`,
      openPoints,
      closedPoints: `${openPoints} ${project(surface.worldVertices[0])}`,
      isHorizontal: isHorizontalSurface(surface),
      isFloor: surface.surfaceType?.toLowerCase() === "floor",
      zoneID: zoneIdForName(geometry, surface.zoneName),
      spaceName: surface.spaceName || "",
    };
  });
  const projectedWindows = windows.map((windowItem) => {
    const openPoints = windowItem.worldVertices.map(project).join(" ");
    return {
      id: windowItem.id,
      closedPoints: `${openPoints} ${project(windowItem.worldVertices[0])}`,
    };
  });
  return { ok: true, viewWidth, viewHeight, surfaces: projectedSurfaces, windows: projectedWindows };
}

function renderPlanSurfaceShape(surface) {
  const title = escapeHTML(surface.title);
  const attributes = topologyNavigationAttributes("surface", surface.id);
  if (surface.isHorizontal) {
    return `<polygon class="${surface.className} navigable-row" data-geometry-kind="surface" data-geometry-id="${escapeHTML(surface.id)}" ${attributes} points="${surface.closedPoints}"><title>${title}</title></polygon>`;
  }
  return `<polyline class="plan-wall ${surface.className} navigable-row" data-geometry-kind="surface" data-geometry-id="${escapeHTML(surface.id)}" ${attributes} points="${surface.closedPoints}"><title>${title}</title></polyline>`;
}

function hasPlanVertices(item) {
  return Array.isArray(item?.worldVertices) && item.worldVertices.length > 0;
}

function planSurfaceClass(surface) {
  const surfaceType = String(surface.surfaceType || "").toLowerCase();
  const boundary = String(surface.outsideBoundary || "").toLowerCase();
  return [
    surfaceType.includes("floor") ? "floor" : "",
    /roof|ceiling/.test(surfaceType) ? "roof" : "",
    /surface|zone|adiabatic/.test(boundary) ? "interior" : "exterior",
  ]
    .filter(Boolean)
    .join(" ");
}

function isHorizontalSurface(surface) {
  return /floor|roof|ceiling/i.test(surface.surfaceType || "");
}

function renderTopologyDetails(geometry = state.report?.geometry) {
  const entity = selectedGeometryEntity(geometry);
  if (!entity) {
    elements.topologyDetails.innerHTML = `<div class="empty">${t("topology.selectObject")}</div>`;
    return;
  }
  const relatedGroups = geometryRelatedGroups(geometry, entity);
  elements.topologyDetails.innerHTML = `
    <div class="topology-detail-head navigable-row" ${topologyNavigationAttributes(entity.kind, entity.id, {
      objectIndex: entity.objectIndex,
      objectType: entity.objectType,
      objectName: entity.title,
    })}>
      <div>
        <h3>${escapeHTML(entity.title)}</h3>
        <span>${escapeHTML(entity.subtitle)}</span>
      </div>
      <span class="topology-sync-note">${state.topologySyncLocate ? t("topology.syncOn") : t("topology.syncOff")}</span>
    </div>
    <div class="topology-detail-grid">
      <section>
        <h4>Metrics</h4>
        ${renderMetricList(entity.metrics)}
      </section>
      <section>
        <h4>${t("topology.relatedObjects")}</h4>
        ${renderRelatedGroups(relatedGroups)}
      </section>
      ${renderConstructionSection(geometry, entity)}
    </div>`;
  bindTopologyDetailControls();
}

function syncLocatedInputEntity(entity) {
  if (!entity) {
    return;
  }
  window.dispatchEvent(
    new CustomEvent("idfAnalyzer:geometryLocate", {
      detail: {
        objectIndex: entity.objectIndex,
        objectType: entity.objectType,
      },
    }),
  );
}

function matchesSelectedStory(item) {
  return state.selectedTopologyStory === "all" || item.storyIndex === state.selectedTopologyStory;
}

function firstStoryIndex(geometry) {
  return geometry?.stories?.[0]?.index ?? 0;
}

function selectedGeometryEntity(geometry) {
  if (!geometry || !state.selectedTopologyEntityId) {
    return null;
  }
  const lookup = geometryLookupIndex(geometry);
  if (state.selectedTopologyEntityKind === "zone") {
    const zone = lookup.zoneByID.get(state.selectedTopologyEntityId);
    return zone && {
      kind: "zone",
      id: zone.id,
      item: zone,
      title: zone.name,
      subtitle: "Zone",
      objectIndex: zone.objectIndex,
      objectType: "Zone",
      metrics: zone.metrics,
    };
  }
  if (state.selectedTopologyEntityKind === "space") {
    const space = lookup.spaceByID.get(state.selectedTopologyEntityId);
    const zone = space ? zoneByName(geometry, space.zoneName) : null;
    return space && {
      kind: "space",
      id: space.id,
      item: space,
      title: space.name,
      subtitle: `Space${space.zoneName ? ` / ${space.zoneName}` : ""}`,
      objectIndex: space.objectIndex,
      objectType: "Space",
      storyIndex: zone?.storyIndex,
      metrics: [],
    };
  }
  if (state.selectedTopologyEntityKind === "window") {
    const windowItem = lookup.windowByID.get(state.selectedTopologyEntityId);
    return windowItem && {
      kind: "window",
      id: windowItem.id,
      item: windowItem,
      title: windowItem.name || windowItem.type,
      subtitle: `${windowItem.surfaceType || windowItem.type} on ${windowItem.baseSurfaceName || "unknown surface"}`,
      objectIndex: windowItem.objectIndex,
      objectType: windowItem.type,
      metrics: windowItem.metrics,
    };
  }
  const surface = lookup.surfaceByID.get(state.selectedTopologyEntityId);
  return surface && {
    kind: "surface",
    id: surface.id,
    item: surface,
    title: surface.name || surface.type,
    subtitle: `${surface.surfaceType || surface.type} / ${surface.zoneName || "No zone"}`,
    objectIndex: surface.objectIndex,
    objectType: surface.type,
    metrics: surface.metrics,
  };
}

function renderMetricList(metrics = []) {
  return metrics.length
    ? `<div class="topology-property-list">${metrics
        .map((metric) => `<div><span>${escapeHTML(metric.name)}</span><strong>${escapeHTML(metric.displayValue)}${metric.unit ? ` ${escapeHTML(metric.unit)}` : ""}</strong></div>`)
        .join("")}</div>`
    : `<div class="empty">No metrics</div>`;
}

function geometryRelatedGroups(geometry, entity) {
  if (!geometry || !entity?.item) {
    return [];
  }
  if (entity.kind === "zone") {
    return geometryRelatedGroupsForZone(geometry, entity.item);
  }
  if (entity.kind === "space") {
    return geometryRelatedGroupsForSpace(geometry, entity.item);
  }
  if (entity.kind === "window") {
    return geometryRelatedGroupsForWindow(geometry, entity.item);
  }
  return geometryRelatedGroupsForSurface(geometry, entity.item);
}

function geometryRelatedGroupsForZone(geometry, zone) {
  const spaces = (geometry.spaces || []).filter((space) => normalizeGeometryName(space.zoneName) === normalizeGeometryName(zone.name));
  const surfaces = (zone.surfaceIds || []).map((id) => surfaceByID(geometry, id)).filter(Boolean);
  const windows = (zone.windowIds || []).map((id) => windowByID(geometry, id)).filter(Boolean);
  const adjacent = uniqueRelatedItems(
    surfaces
      .flatMap((surface) => {
        const adjacentSurface = adjacentSurfaceForSurface(geometry, surface);
        const adjacentZone = adjacentSurface ? zoneByName(geometry, adjacentSurface.zoneName) : null;
        return [
          adjacentZone && adjacentZone.id !== zone.id ? relatedItemForZone(adjacentZone, "Adjacent zone") : null,
          adjacentSurface ? relatedItemForSurface(adjacentSurface, "Adjacent surface", geometry) : referencedBoundaryItem(surface),
        ];
      })
      .filter(Boolean),
  );
  return [
    { title: "Spaces", items: spaces.map((space) => relatedItemForSpace(space, "Space", geometry)) },
    { title: "Boundary Surfaces", items: surfaces.map((surface) => relatedItemForSurface(surface, surface.surfaceType || "Surface", geometry)) },
    { title: "Openings", items: windows.map((windowItem) => relatedItemForWindow(windowItem, windowItem.surfaceType || "Window", geometry)) },
    { title: "Adjacent", items: adjacent },
  ];
}

function geometryRelatedGroupsForSpace(geometry, space) {
  const parentZone = zoneByName(geometry, space.zoneName);
  const surfaces = (geometry.surfaces || []).filter((surface) => normalizeGeometryName(surface.spaceName) === normalizeGeometryName(space.name));
  const windows = surfaces.flatMap((surface) => windowsForSurface(geometry, surface));
  return [
    { title: "Parent", items: parentZone ? [relatedItemForZone(parentZone, "Zone")] : [] },
    { title: "Boundary Surfaces", items: surfaces.map((surface) => relatedItemForSurface(surface, surface.surfaceType || "Surface", geometry)) },
    { title: "Openings", items: uniqueRelatedItems(windows.map((item) => relatedItemForWindow(item, item.surfaceType || "Window", geometry))) },
  ];
}

function geometryRelatedGroupsForSurface(geometry, surface) {
  const parentZone = zoneByName(geometry, surface.zoneName);
  const parentSpace = spaceByName(geometry, surface.spaceName);
  const windows = windowsForSurface(geometry, surface);
  const adjacentSurface = adjacentSurfaceForSurface(geometry, surface);
  const adjacentZone = adjacentSurface ? zoneByName(geometry, adjacentSurface.zoneName) : null;
  const adjacentItems = [
    adjacentZone && adjacentZone.id !== parentZone?.id ? relatedItemForZone(adjacentZone, "Adjacent zone") : null,
    adjacentSurface ? relatedItemForSurface(adjacentSurface, "Adjacent surface", geometry) : referencedBoundaryItem(surface),
  ].filter(Boolean);
  return [
    { title: "Parent", items: [parentZone && relatedItemForZone(parentZone, "Zone"), parentSpace && relatedItemForSpace(parentSpace, "Space", geometry)].filter(Boolean) },
    { title: "Openings", items: windows.map((windowItem) => relatedItemForWindow(windowItem, windowItem.surfaceType || "Window", geometry)) },
    { title: "Adjacent", items: adjacentItems },
  ];
}

function geometryRelatedGroupsForWindow(geometry, windowItem) {
  const parentSurface = windowItem.baseSurfaceId
    ? surfaceByID(geometry, windowItem.baseSurfaceId)
    : surfaceByName(geometry, windowItem.baseSurfaceName);
  const parentZone = zoneByName(geometry, windowItem.zoneName || parentSurface?.zoneName);
  const siblingWindows = parentSurface
    ? windowsForSurface(geometry, parentSurface).filter((item) => item.id !== windowItem.id)
    : [];
  return [
    { title: "Parent", items: [parentZone && relatedItemForZone(parentZone, "Zone"), parentSurface && relatedItemForSurface(parentSurface, "Base surface", geometry)].filter(Boolean) },
    { title: "Sibling Openings", items: siblingWindows.map((item) => relatedItemForWindow(item, item.surfaceType || "Window", geometry)) },
  ];
}

function renderRelatedGroups(groups = []) {
  const visibleGroups = groups.filter((group) => group.items.length);
  if (!visibleGroups.length) {
    return `<div class="empty">No related objects</div>`;
  }
  return `
    <div class="topology-related-groups">
      ${visibleGroups
        .map(
          (group) => `
            <details class="topology-related-group" open>
              <summary>
                <span>${escapeHTML(group.title)}</span>
                <span class="badge">${escapeHTML(group.items.length)}</span>
              </summary>
              <div class="topology-related-list">
                ${group.items.map(renderRelatedItem).join("")}
              </div>
            </details>`,
        )
        .join("")}
    </div>`;
}

function renderConstructionSection(geometry, entity) {
  const construction = constructionForEntity(geometry, entity);
  const constructionName = entity?.item?.construction || "";
  if (!constructionName && !construction) {
    return "";
  }
  return `
    <section class="topology-construction-section">
      <h4>${t("topology.construction", {}, "Construction")}</h4>
      ${construction ? renderConstructionGraphic(construction, constructionSidesForEntity(geometry, entity)) : `<div class="empty">${t("topology.noConstruction", {}, "No construction layers parsed")}: ${escapeHTML(constructionName)}</div>`}
    </section>`;
}

function constructionForEntity(geometry, entity) {
  const constructionName = entity?.item?.construction;
  const key = normalizeGeometryName(constructionName);
  if (!key) {
    return null;
  }
  return constructionForName(geometry, constructionName);
}

function constructionForName(geometry, constructionName) {
  const key = normalizeGeometryName(constructionName);
  return key ? geometryLookupIndex(geometry).constructionByName.get(key) || null : null;
}

function constructionPerformance(construction) {
  if (!construction) {
    return { uValue: 0, arealHeatCapacity: 0 };
  }
  const layers = construction.layers || [];
  const resistance = layers.reduce((sum, layer) => sum + layerThermalResistance(layer), 0);
  const arealHeatCapacity = layers.reduce((sum, layer) => sum + layerArealHeatCapacity(layer), 0);
  return {
    uValue: Number(construction.uValue) || (resistance > 0 ? 1 / resistance : 0),
    arealHeatCapacity: Number(construction.arealHeatCapacity) || arealHeatCapacity,
  };
}

function layerThermalResistance(layer) {
  if (Number(layer.thermalResistance) > 0) {
    return Number(layer.thermalResistance);
  }
  if (Number(layer.uFactor) > 0) {
    return 1 / Number(layer.uFactor);
  }
  if (layer.hasThickness && Number(layer.thickness) > 0 && Number(layer.conductivity) > 0) {
    return Number(layer.thickness) / Number(layer.conductivity);
  }
  return 0;
}

function layerArealHeatCapacity(layer) {
  if (Number(layer.arealHeatCapacity) > 0) {
    return Number(layer.arealHeatCapacity);
  }
  if (layer.hasThickness && Number(layer.thickness) > 0 && Number(layer.density) > 0 && Number(layer.specificHeat) > 0) {
    return Number(layer.thickness) * Number(layer.density) * Number(layer.specificHeat);
  }
  return 0;
}

function constructionSidesForEntity(geometry, entity) {
  if (entity?.kind === "surface") {
    const surface = entity.item;
    return {
      outside: surfaceOutsideLabel(geometry, surface),
      inside: `${t("topology.thisSurface", {}, "This surface")}${surface.zoneName ? ` / ${surface.zoneName}` : ""}`,
    };
  }
  if (entity?.kind === "window") {
    const windowItem = entity.item;
    const baseSurface = windowItem.baseSurfaceId
      ? surfaceByID(geometry, windowItem.baseSurfaceId)
      : surfaceByName(geometry, windowItem.baseSurfaceName);
    return {
      outside: baseSurface ? surfaceOutsideLabel(geometry, baseSurface) : windowItem.baseSurfaceName || t("common.notAvailable", {}, "—"),
      inside: `${t("topology.thisOpening", {}, "This opening")}${windowItem.zoneName ? ` / ${windowItem.zoneName}` : ""}`,
    };
  }
  return {
    outside: t("common.notAvailable", {}, "—"),
    inside: t("common.notAvailable", {}, "—"),
  };
}

function surfaceOutsideLabel(geometry, surface) {
  const boundary = surface?.outsideBoundary || "";
  const boundaryName = boundaryObjectName(surface);
  const adjacentSurface = boundaryName ? surfaceByName(geometry, boundaryName) : null;
  const adjacentZone = adjacentSurface ? zoneByName(geometry, adjacentSurface.zoneName) : null;
  if (adjacentSurface) {
    return `${boundary || "Surface"}: ${adjacentSurface.name || boundaryName}${adjacentZone?.name ? ` / ${adjacentZone.name}` : ""}`;
  }
  if (boundaryName) {
    return `${boundary || "Boundary"}: ${boundaryName}`;
  }
  return boundary || t("common.notAvailable", {}, "—");
}

function renderConstructionGraphic(construction, sides) {
  const layers = construction.layers || [];
  const totalThickness = layers.reduce((sum, layer) => sum + (layer.hasThickness ? Number(layer.thickness) || 0 : 0), 0);
  const performance = constructionPerformance(construction);
  return `
    <div class="construction-card">
      <div class="construction-card-head">
        <strong>${escapeHTML(construction.name)}</strong>
        <span>${construction.hasThickness ? `${t("topology.totalThickness", {}, "Total thickness")} ${formatThickness(construction.totalThickness || totalThickness)}` : t("topology.thicknessUnknown", {}, "Thickness unknown")} / ${t("topology.outsideToInside", {}, "Outside to inside")}</span>
      </div>
      <div class="construction-performance">
        <span><strong>${t("topology.uValue", {}, "U-value")}</strong><em>${formatUValue(performance.uValue)}</em></span>
        <span><strong>${t("topology.heatCapacity", {}, "Heat capacity")}</strong><em>${formatArealHeatCapacity(performance.arealHeatCapacity)}</em></span>
      </div>
      <div class="construction-stack-frame">
        <span class="construction-side-label">${t("topology.outside", {}, "Outside")} <em>${escapeHTML(sides.outside)}</em></span>
        <div class="construction-stack" role="img" aria-label="${escapeHTML(construction.name)} construction layers">
          ${layers.length ? layers.map((layer, index) => renderConstructionLayer(layer, index, totalThickness)).join("") : `<div class="empty">${t("topology.noConstruction", {}, "No construction layers parsed")}</div>`}
        </div>
        <span class="construction-side-label">${t("topology.inside", {}, "Inside")} <em>${escapeHTML(sides.inside)}</em></span>
      </div>
    </div>`;
}

function renderConstructionLayer(layer, index, totalThickness) {
  const thickness = layer.hasThickness ? Number(layer.thickness) || 0 : 0;
  const flexGrow = layer.hasThickness && totalThickness > 0 ? Math.max(0.16, thickness / totalThickness) : 0.3;
  const height = layer.hasThickness && totalThickness > 0 ? Math.max(34, (thickness / totalThickness) * 220) : 42;
  const color = constructionLayerColor(layer, index);
  const details = [
    layer.objectType,
    layer.thermalResistance ? `R ${formatNumber(layer.thermalResistance)}` : "",
    layer.uFactor ? `U ${formatNumber(layer.uFactor)}` : "",
    layer.conductivity ? `k ${formatNumber(layer.conductivity)}` : "",
  ].filter(Boolean);
  return `
    <button class="construction-layer" type="button" data-object-index="${escapeHTML(layer.objectIndex ?? "")}" style="--layer-color: ${color}; --layer-flex: ${flexGrow}; --layer-height: ${height}px;">
      <span class="construction-layer-bar"></span>
      <span class="construction-layer-text">
        <strong>${escapeHTML(layer.name)}</strong>
        <span>${escapeHTML(details.join(" / ") || t("common.notAvailable", {}, "—"))}</span>
      </span>
      <span class="construction-layer-thickness">${escapeHTML(layer.hasThickness ? formatThickness(layer.thickness) : t("topology.thicknessUnknown", {}, "Thickness unknown"))}</span>
    </button>`;
}

function constructionLayerColor(layer, index) {
  const text = `${layer.objectType || ""} ${layer.name || ""}`.toLowerCase();
  if (text.includes("window") || text.includes("glazing") || text.includes("glass")) {
    return "#68b9d1";
  }
  if (text.includes("air") || text.includes("gas")) {
    return "#dbe8ef";
  }
  if (text.includes("insulation") || text.includes("mass")) {
    return "#d7c878";
  }
  if (text.includes("concrete") || text.includes("gypsum") || text.includes("plaster")) {
    return "#b9bdc3";
  }
  if (text.includes("metal") || text.includes("steel") || text.includes("alum")) {
    return "#8da1ad";
  }
  if (text.includes("wood")) {
    return "#b7895b";
  }
  const palette = ["#9fb7a4", "#a8b5ca", "#c3a995", "#b6b0c8", "#a7c0bf"];
  return palette[index % palette.length];
}

function formatThickness(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) {
    return t("common.notAvailable", {}, "—");
  }
  if (number < 1) {
    return `${(number * 1000).toLocaleString(undefined, { maximumFractionDigits: 1 })} mm`;
  }
  return `${number.toLocaleString(undefined, { maximumFractionDigits: 3 })} m`;
}

function formatUValue(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) {
    return t("common.notAvailable", {}, "—");
  }
  return `${number.toLocaleString(undefined, { maximumFractionDigits: 3 })} W/m2-K`;
}

function formatArealHeatCapacity(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) {
    return t("common.notAvailable", {}, "—");
  }
  return `${(number / 1000).toLocaleString(undefined, { maximumFractionDigits: 1 })} kJ/m2-K`;
}

function formatArea(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) {
    return t("common.notAvailable", {}, "—");
  }
  return `${number.toLocaleString(undefined, { maximumFractionDigits: 2 })} m2`;
}

function renderRelatedItem(item) {
  const content = `
    <span class="topology-related-main">
      <strong>${escapeHTML(item.title)}</strong>
      <span>${escapeHTML(item.subtitle)}</span>
      ${item.detail ? `<em>${escapeHTML(item.detail)}</em>` : ""}
    </span>
    <span class="topology-related-role">${escapeHTML(item.role)}</span>`;
  if (item.kind && item.id) {
    return `<button class="topology-related-row navigable-row" type="button" data-geometry-kind="${escapeHTML(item.kind)}" data-geometry-id="${escapeHTML(item.id)}" ${topologyNavigationAttributes(item.kind, item.id, item.sourceAnchor)}>${content}</button>`;
  }
  return `<div class="topology-related-row topology-related-static">${content}</div>`;
}

function bindTopologyDetailControls() {
  const details = elements.topologyDetails;
  if (topologyDetailInteractionsBound || !details) return;
  topologyDetailInteractionsBound = true;
  details.addEventListener("click", (event) => {
    const related = event.target.closest?.(".topology-related-row[data-geometry-id]");
    if (related && details.contains(related)) {
      event.stopPropagation();
      void selectTopologyEntity(related.dataset.geometryKind, related.dataset.geometryId);
      return;
    }
    const layer = event.target.closest?.(".construction-layer[data-object-index]");
    if (!layer || !details.contains(layer)) return;
    const objectIndex = Number(layer.dataset.objectIndex);
    if (!Number.isFinite(objectIndex) || objectIndex < 0) return;
    window.dispatchEvent(
      new CustomEvent("idfAnalyzer:geometryLocate", {
        detail: { objectIndex },
      }),
    );
  });
}

function relatedItemForZone(zone, role) {
  return {
    kind: "zone",
    id: zone.id,
    role,
    title: zone.name,
    subtitle: storyLabelForIndex(state.report?.geometry, zone.storyIndex),
    sourceAnchor: { objectIndex: zone.objectIndex, objectType: "Zone", objectName: zone.name },
  };
}

function relatedItemForSpace(space, role, geometry = state.report?.geometry) {
  const zone = zoneByName(geometry, space.zoneName);
  return {
    kind: "space",
    id: space.id,
    role,
    title: space.name,
    subtitle: [space.zoneName, storyLabelForIndex(geometry, zone?.storyIndex)].filter(Boolean).join(" / "),
    sourceAnchor: { objectIndex: space.objectIndex, objectType: "Space", objectName: space.name },
  };
}

function relatedItemForSurface(surface, role, geometry = state.report?.geometry) {
  const construction = constructionForName(geometry, surface.construction);
  const performance = constructionPerformance(construction);
  const details = [
    `${t("topology.area", {}, "Area")} ${formatArea(surface.physicalArea ?? surface.area)}`,
    `${t("topology.uValue", {}, "U-value")} ${formatUValue(performance.uValue)}`,
    `${t("topology.boundary", {}, "Boundary")} ${surfaceOutsideLabel(geometry, surface)}`,
  ];
  return {
    kind: "surface",
    id: surface.id,
    role,
    title: surface.name || surface.type,
    subtitle: `${surface.surfaceType || surface.type}${surface.construction ? ` / ${surface.construction}` : ""}`,
    detail: details.join(" / "),
    sourceAnchor: { objectIndex: surface.objectIndex, objectType: surface.type, objectName: surface.name },
  };
}

function relatedItemForWindow(windowItem, role, geometry = state.report?.geometry) {
  const construction = constructionForName(geometry, windowItem.construction);
  const performance = constructionPerformance(construction);
  const details = [
    `${t("topology.area", {}, "Area")} ${formatArea(windowItem.physicalArea ?? windowItem.area)}`,
    `${t("topology.uValue", {}, "U-value")} ${formatUValue(performance.uValue)}`,
    windowItem.baseSurfaceName ? `${t("topology.baseSurface", {}, "Base surface")} ${windowItem.baseSurfaceName}` : "",
  ].filter(Boolean);
  return {
    kind: "window",
    id: windowItem.id,
    role,
    title: windowItem.name || windowItem.type,
    subtitle: `${windowItem.surfaceType || windowItem.type}${windowItem.construction ? ` / ${windowItem.construction}` : ""}`,
    detail: details.join(" / "),
    sourceAnchor: { objectIndex: windowItem.objectIndex, objectType: windowItem.type, objectName: windowItem.name },
  };
}

function referencedBoundaryItem(surface) {
  const boundaryName = boundaryObjectName(surface);
  if (!boundaryName) {
    return null;
  }
  return {
    role: "Referenced surface",
    title: boundaryName,
    subtitle: "Not parsed in geometry",
  };
}

function uniqueRelatedItems(items) {
  const seen = new Set();
  return items.filter((item) => {
    const key = `${item.kind || "static"}:${item.id || item.title}:${item.role}`;
    if (seen.has(key)) {
      return false;
    }
    seen.add(key);
    return true;
  });
}

function topologySemanticNavigationLookup(navigation = {}) {
  const occurrences = navigation.occurrences || EMPTY_GEOMETRY_ITEMS;
  const entities = navigation.entities || EMPTY_GEOMETRY_ITEMS;
  if (
    topologyNavigationLookupCache?.navigation === navigation &&
    topologyNavigationLookupCache.occurrences === occurrences &&
    topologyNavigationLookupCache.entities === entities &&
    topologyNavigationLookupCache.occurrenceCount === occurrences.length &&
    topologyNavigationLookupCache.entityCount === entities.length
  ) {
    return topologyNavigationLookupCache;
  }
  topologyNavigationLookupCache = {
    navigation,
    occurrences,
    entities,
    occurrenceCount: occurrences.length,
    entityCount: entities.length,
    occurrenceByID: indexFirstBy(occurrences, (occurrence) => occurrence.occurrenceId),
    occurrenceOrderByID: new Map(occurrences.map((occurrence, index) => [occurrence.occurrenceId, index])),
    entityByID: indexFirstBy(entities, (entity) => entity.id),
  };
  return topologyNavigationLookupCache;
}

function topologyViewTargetForSelection(selection = {}, navigation = state.semanticProjection?.navigation || {}) {
  const direct = selection.viewTarget;
  if (String(direct?.view || "").toLowerCase() === "topology" && direct.targetId) {
    return direct;
  }
  const lookup = topologySemanticNavigationLookup(navigation);
  const occurrence = lookup.occurrenceByID.get(selection.occurrenceId);
  const entity = lookup.entityByID.get(selection.entityId);
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

function topologyTargetEntity(target, geometry = state.report?.geometry) {
  if (!target || !geometry) {
    return null;
  }
  const targetId = String(target.targetId || "");
  const requestedKind = normalizeGeometryKind(target.targetKind);
  const thermalTarget = resolveThermalTopologyTarget(target, geometry);
  if (thermalTarget) {
    return {
      kind: thermalTarget.kind,
      id: thermalTarget.id,
      item: thermalTarget.item,
      thermalTarget,
    };
  }
  const lookup = geometryLookupIndex(geometry);
  const candidates = requestedKind ? [requestedKind] : ["zone", "space", "surface", "window", "story"];
  for (const kind of candidates) {
    if (kind === "zone") {
      const item = lookup.zoneByID.get(targetId);
      if (item) return { kind, id: item.id, item, objectIndex: item.objectIndex, objectType: "Zone", storyIndex: item.storyIndex };
    } else if (kind === "space") {
      const item = lookup.spaceByID.get(targetId);
      const zone = item ? zoneByName(geometry, item.zoneName) : null;
      if (item) return { kind, id: item.id, item, objectIndex: item.objectIndex, objectType: "Space", storyIndex: zone?.storyIndex };
    } else if (kind === "surface") {
      const item = lookup.surfaceByID.get(targetId);
      if (item) return { kind, id: item.id, item, objectIndex: item.objectIndex, objectType: item.type, storyIndex: item.storyIndex };
    } else if (kind === "window") {
      const item = lookup.windowByID.get(targetId);
      if (item) return { kind, id: item.id, item, objectIndex: item.objectIndex, objectType: item.type, storyIndex: item.storyIndex };
    } else if (kind === "story") {
      const item = (geometry.stories || []).find((story) => geometryStoryMatchesTarget(story, targetId));
      if (item) return { kind, id: geometryStoryTargetID(item), item, storyIndex: item.index };
    }
  }
  if (requestedKind) {
    return topologyTargetEntity({ ...target, targetKind: "" }, geometry);
  }
  return null;
}

export function topologySelectionForTarget(kind, targetId, navigation = state.semanticProjection?.navigation || {}) {
  if (!targetId) {
    return null;
  }
  const occurrence = preferredOccurrenceForTopologyTarget(targetId, state.globalSelection, navigation);
  const entity = topologySemanticNavigationLookup(navigation).entityByID.get(occurrence?.entityId) || null;
  if (!occurrence || !entity) {
    return null;
  }
  return {
    entityId: entity.id,
    entityKind: entity.kind || normalizeGeometryEntityKind(kind),
    occurrenceId: occurrence.occurrenceId,
    sourceAnchor: { ...(occurrence.sourceAnchor || entity.sourceAnchors?.[0] || {}) },
    originView: "topology",
    originTargetId: String(targetId),
    semanticPathHint: occurrence.path || "",
    relatedEntityIds: [...(entity.relatedEntityIds || [])],
  };
}

function preferredOccurrenceForTopologyTarget(targetId, selection = {}, navigation = state.semanticProjection?.navigation || {}) {
  const occurrenceIds = navigation.byViewTarget?.[`topology|${targetId}`] || [];
  const currentPath = String(selection.semanticPathHint || state.semanticCurrentPath || "");
  const lookup = topologySemanticNavigationLookup(navigation);
  return [...new Set(occurrenceIds)]
    .map((occurrenceID) => lookup.occurrenceByID.get(occurrenceID))
    .filter(Boolean)
    .sort((left, right) => (lookup.occurrenceOrderByID.get(left.occurrenceId) || 0) - (lookup.occurrenceOrderByID.get(right.occurrenceId) || 0))
    .map((occurrence, order) => ({
      occurrence,
      order,
      contextPriority: thermalOccurrenceContextPriority(occurrence),
      exact: Number(occurrence.occurrenceId === selection.occurrenceId),
      current: Number(occurrence.occurrenceId === state.semanticCurrentOccurrenceId),
      geometryContext: Number(occurrence.contextKind === "zone_geometry" || /(^|\/)geometry(\/|$)/.test(occurrence.path || "")),
      path: commonPathPrefixLength(occurrence.path, currentPath),
      preferred: Number(occurrence.preferredView === "topology"),
    }))
    .sort((left, right) => (
      right.contextPriority - left.contextPriority ||
      right.geometryContext - left.geometryContext ||
      right.exact - left.exact ||
      right.current - left.current ||
      right.path - left.path ||
      right.preferred - left.preferred ||
      left.order - right.order
    ))[0]?.occurrence || null;
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

export function topologyNavigationAttributes(kind, targetId, explicitAnchor = {}, options = {}) {
  const navigation = state.semanticProjection?.navigation || {};
  const occurrence = preferredOccurrenceForTopologyTarget(targetId, state.globalSelection, navigation);
  const entity = topologySemanticNavigationLookup(navigation).entityByID.get(occurrence?.entityId) || null;
  const sourceAnchor = { ...(occurrence?.sourceAnchor || entity?.sourceAnchors?.[0] || {}), ...explicitAnchor };
  const selected = Boolean(
    (entity?.id && entity.id === state.globalSelection?.entityId) ||
    (String(targetId || "") === state.selectedTopologyEntityId && normalizeGeometryKind(kind) === state.selectedTopologyEntityKind),
  );
  const attributes = [
    `data-entity-id="${escapeHTML(entity?.id || "")}"`,
    `data-entity-kind="${escapeHTML(entity?.kind || normalizeGeometryEntityKind(kind))}"`,
    `data-occurrence-id="${escapeHTML(occurrence?.occurrenceId || "")}"`,
    `data-occurrence-context="${escapeHTML(occurrence?.occurrenceId || "")}"`,
    `data-semantic-path="${escapeHTML(occurrence?.path || "")}"`,
    `data-panel-target-id="${escapeHTML(targetId || "")}"`,
    `data-source-object-id="${escapeHTML(sourceAnchor.objectId || "")}"`,
    `data-source-object-index="${escapeHTML(sourceAnchor.objectIndex ?? "")}"`,
    `data-source-object-type="${escapeHTML(sourceAnchor.objectType || "")}"`,
    `data-source-object-name="${escapeHTML(sourceAnchor.objectName || "")}"`,
    `data-source-field-index="${escapeHTML(sourceAnchor.fieldIndex ?? "")}"`,
    `aria-selected="${selected ? "true" : "false"}"`,
  ];
  if (options.tabindex !== false) {
    attributes.push('tabindex="0"', 'role="button"');
  }
  return attributes.join(" ");
}

function normalizeGeometryKind(kind) {
  const normalized = String(kind || "").trim().toLowerCase();
  return normalized === "fenestration" ? "window" : normalized;
}

function normalizeGeometryEntityKind(kind) {
  return normalizeGeometryKind(kind) === "window" ? "fenestration" : normalizeGeometryKind(kind);
}

function geometryStoryTargetID(story, navigation = state.semanticProjection?.navigation || {}) {
  for (const occurrence of navigation.occurrences || []) {
    const target = (occurrence.viewTargets || []).find((candidate) => (
      String(candidate?.view || "").toLowerCase() === "topology" &&
      normalizeGeometryKind(candidate.targetKind) === "story" &&
      geometryStoryMatchesTarget(story, candidate.targetId)
    ));
    if (target?.targetId) {
      return target.targetId;
    }
  }
  return `story-${story.index}`;
}

function geometryStoryMatchesTarget(story, targetId) {
  const normalized = String(targetId || "").trim().toLowerCase();
  return normalized === String(story.index) ||
    normalized === `story-${story.index}` ||
    normalized === String(story.name || "").trim().toLowerCase();
}

function owningZoneForGeometryEntity(entity, geometry) {
  if (entity?.thermalTarget) {
    for (const surfaceId of entity.thermalTarget.surfaceIds || []) {
      const surface = surfaceByID(geometry, surfaceId);
      const zone = surface ? zoneByName(geometry, surface.zoneName) : null;
      if (zone) return zone;
    }
    for (const nodeId of entity.thermalTarget.nodeIds || []) {
      const zone = geometryLookupIndex(geometry).zoneByID.get(nodeId);
      if (zone) return zone;
      const space = spaceByID(geometry, nodeId);
      const owner = space ? zoneByName(geometry, space.zoneName) : null;
      if (owner) return owner;
    }
  }
  if (entity?.kind === "zone") {
    return entity.item;
  }
  if (entity?.kind === "space") {
    return zoneByName(geometry, entity.item.zoneName);
  }
  if (entity?.kind === "surface") {
    return zoneByName(geometry, entity.item.zoneName);
  }
  if (entity?.kind === "window") {
    const baseSurface = baseSurfaceForWindow(geometry, entity.item);
    return zoneByName(geometry, entity.item.zoneName || baseSurface?.zoneName);
  }
  return null;
}

function geometryStoryIndexForEntity(entity, geometry) {
  if (Number.isInteger(entity?.storyIndex)) {
    return entity.storyIndex;
  }
  return owningZoneForGeometryEntity(entity, geometry)?.storyIndex;
}

function geometryEntityHasPlanShape(entity, geometry) {
  if (entity?.thermalTarget) {
    return (entity.thermalTarget.surfaceIds || []).some((id) => hasPlanVertices(surfaceByID(geometry, id))) ||
      (entity.thermalTarget.windowIds || []).some((id) => hasPlanVertices(windowByID(geometry, id)));
  }
  if (entity?.kind === "zone") {
    return (entity.item.surfaceIds || []).some((id) => {
      const surface = surfaceByID(geometry, id);
      return surface?.surfaceType?.toLowerCase() === "floor" && hasPlanVertices(surface);
    });
  }
  if (entity?.kind === "space") {
    return (geometry.surfaces || []).some((surface) => (
      normalizeGeometryName(surface.spaceName) === normalizeGeometryName(entity.item.name) && hasPlanVertices(surface)
    ));
  }
  return entity?.kind === "surface" || entity?.kind === "window" ? hasPlanVertices(entity.item) : true;
}

function baseSurfaceForWindow(geometry, windowItem) {
  return windowItem?.baseSurfaceId
    ? surfaceByID(geometry, windowItem.baseSurfaceId)
    : surfaceByName(geometry, windowItem?.baseSurfaceName);
}

function findTopologyNavigationTarget(selection, target, context) {
  const root = context?.root || document.getElementById("topologyPane");
  const items = [...(root?.querySelectorAll?.("[data-panel-target-id], [data-entity-id]") || [])];
  return items.find((item) => item.dataset.panelTargetId === String(target?.targetId || "")) ||
    items.find((item) => item.dataset.entityId === String(selection?.entityId || "")) ||
    context?.genericFindTarget(selection) || null;
}

function nextTopologyFrame() {
  if (typeof window === "undefined" || typeof window.requestAnimationFrame !== "function") {
    return Promise.resolve();
  }
  return new Promise((resolve) => window.requestAnimationFrame(resolve));
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

function geometryZoneSurfaceIsTemporarilyVisible(surface, geometry) {
  const zoneID = zoneIdForName(geometry, surface.zoneName);
  return Boolean(
    (temporaryTopologyReveal?.ownerZoneId && temporaryTopologyReveal.ownerZoneId === zoneID) ||
    temporaryTopologyReveal?.nodeIds?.includes(zoneID),
  );
}

function geometrySurfaceIsTemporarilyVisible(surface) {
  if (temporaryTopologyReveal?.surfaceIds?.includes(surface.id)) {
    return true;
  }
  if (temporaryTopologyReveal?.kind === "surface" && temporaryTopologyReveal.id === surface.id) {
    return true;
  }
  if (temporaryTopologyReveal?.kind === "space") {
    const space = spaceByID(state.report?.geometry, temporaryTopologyReveal.id);
    return Boolean(space && normalizeGeometryName(surface.spaceName) === normalizeGeometryName(space.name));
  }
  if (temporaryTopologyReveal?.kind === "zone") {
    const zone = zoneByName(state.report?.geometry, surface.zoneName);
    return zone?.id === temporaryTopologyReveal.id;
  }
  return temporaryTopologyReveal?.baseSurfaceId === surface.id;
}

function projectedSurfaceIsTemporarilyVisible(surface) {
  if (temporaryTopologyReveal?.surfaceIds?.includes(surface.id)) {
    return true;
  }
  if (temporaryTopologyReveal?.kind === "surface" && temporaryTopologyReveal.id === surface.id) {
    return true;
  }
  if (temporaryTopologyReveal?.kind === "space") {
    const space = spaceByID(state.report?.geometry, temporaryTopologyReveal.id);
    return Boolean(space && normalizeGeometryName(surface.spaceName) === normalizeGeometryName(space.name));
  }
  if (temporaryTopologyReveal?.kind === "zone") {
    return surface.zoneID === temporaryTopologyReveal.id;
  }
  return temporaryTopologyReveal?.baseSurfaceId === surface.id;
}

function geometryWindowIsTemporarilyVisible(windowItem) {
  return temporaryTopologyReveal?.windowIds?.includes(windowItem.id) ||
    temporaryTopologyReveal?.kind === "window" && temporaryTopologyReveal.id === windowItem.id;
}

function geometryRenderableMatchesSelection(kind, id) {
  const normalizedKind = normalizeGeometryKind(kind);
  if (normalizedKind === "surface" && temporaryTopologyReveal?.surfaceIds?.includes(String(id || ""))) {
    return true;
  }
  if (normalizedKind === "window" && temporaryTopologyReveal?.windowIds?.includes(String(id || ""))) {
    return true;
  }
  if ((normalizedKind === "zone" || normalizedKind === "space") && temporaryTopologyReveal?.nodeIds?.includes(String(id || ""))) {
    return true;
  }
  if (normalizedKind === state.selectedTopologyEntityKind && String(id || "") === state.selectedTopologyEntityId) {
    return true;
  }
  if (state.selectedTopologyEntityKind === "zone" && normalizedKind === "surface") {
    const zone = (state.report?.geometry?.zones || []).find((item) => item.id === state.selectedTopologyEntityId);
    const surface = surfaceByID(state.report?.geometry, id);
    return Boolean(zone && surface && normalizeGeometryName(surface.zoneName) === normalizeGeometryName(zone.name));
  }
  if (state.selectedTopologyEntityKind !== "space") {
    return false;
  }
  const space = spaceByID(state.report?.geometry, state.selectedTopologyEntityId);
  if (normalizedKind === "zone") {
    return Boolean(space && zoneByName(state.report?.geometry, space.zoneName)?.id === id);
  }
  if (normalizedKind !== "surface") {
    return false;
  }
  const surface = surfaceByID(state.report?.geometry, id);
  return Boolean(space && surface && normalizeGeometryName(surface.spaceName) === normalizeGeometryName(space.name));
}

function geometryRenderableSelectionRole(kind, id) {
  if (!geometryRenderableMatchesSelection(kind, id)) return "";
  const normalizedKind = normalizeGeometryKind(kind);
  if (normalizedKind === "surface" && temporaryTopologyReveal?.surfaceIds?.length > 1) {
    return String(id || "") === temporaryTopologyReveal.primarySurfaceId ? "primary" : "counterpart";
  }
  return "primary";
}

function geometryRenderableMatchesHover(kind, id) {
  const normalizedKind = normalizeGeometryKind(kind);
  const targetID = String(id || "");
  if (normalizedKind === temporaryTopologyHover?.kind && targetID === temporaryTopologyHover?.id) return true;
  if (normalizedKind === "surface" && temporaryTopologyHover?.surfaceIds?.includes(targetID)) return true;
  if (normalizedKind === "window" && temporaryTopologyHover?.windowIds?.includes(targetID)) return true;
  if ((normalizedKind === "zone" || normalizedKind === "space") && temporaryTopologyHover?.nodeIds?.includes(targetID)) return true;
  return false;
}

function topologyProjectionForSemanticSelection(selection, geometry) {
  if (!selection?.entityId || !geometry) return null;
  const target = topologyViewTargetForSelection(selection, state.semanticProjection?.navigation || {});
  const entity = topologyTargetEntity(target, geometry);
  if (!entity) return null;
  const ownerZone = owningZoneForGeometryEntity(entity, geometry);
  return {
    kind: entity.kind,
    id: entity.id,
    ownerZoneId: ownerZone?.id || "",
    baseSurfaceId: entity.kind === "window" ? baseSurfaceForWindow(geometry, entity.item)?.id || "" : "",
    surfaceIds: [...(entity.thermalTarget?.surfaceIds || [])],
    windowIds: [...(entity.thermalTarget?.windowIds || [])],
    nodeIds: [...(entity.thermalTarget?.nodeIds || [])],
  };
}

function projectGeometrySelectionToThermal(kind, id, geometry) {
  const normalizedKind = normalizeGeometryKind(kind);
  if (isThermalSelectionKind(normalizedKind)) {
    state.thermalTopologySelectedEntityKind = normalizedKind;
    state.thermalTopologySelectedEntityId = String(id || "");
    return;
  }
  if (normalizedKind === "surface") {
    const boundary = (geometry?.topology?.boundaries || []).find((item) => item.surfaceId === id || item.surfaceEntityId === id);
    if (boundary) {
      state.thermalTopologySelectedEntityKind = "thermal_boundary";
      state.thermalTopologySelectedEntityId = boundary.id;
      return;
    }
  }
  if (normalizedKind === "window") {
    const opening = (geometry?.topology?.openings || []).find((item) => item.windowId === id || item.entityId === id);
    if (opening) {
      state.thermalTopologySelectedEntityKind = "window";
      state.thermalTopologySelectedEntityId = opening.entityId || opening.windowId;
      return;
    }
  }
  if (normalizedKind === "zone" || normalizedKind === "space") {
    state.thermalTopologySelectedEntityKind = normalizedKind;
    state.thermalTopologySelectedEntityId = String(id || "");
  }
}

function isThermalSelectionKind(kind) {
  return ["thermal_boundary", "thermal_interface", "thermal_connection", "thermal_environment", "thermal_air_coupling", "thermal_issue", "thermal_observation"].includes(kind);
}

function geometryLookupIndex(geometry) {
  const collections = {
    zones: geometry?.zones || EMPTY_GEOMETRY_ITEMS,
    spaces: geometry?.spaces || EMPTY_GEOMETRY_ITEMS,
    surfaces: geometry?.surfaces || EMPTY_GEOMETRY_ITEMS,
    windows: geometry?.windows || EMPTY_GEOMETRY_ITEMS,
    stories: geometry?.stories || EMPTY_GEOMETRY_ITEMS,
    constructions: geometry?.constructions || EMPTY_GEOMETRY_ITEMS,
  };
  if (
    geometryLookupCache?.geometry === geometry &&
    Object.entries(collections).every(([key, values]) => geometryLookupCache.collections[key] === values && geometryLookupCache.lengths[key] === values.length)
  ) {
    return geometryLookupCache;
  }
  const windowsBySurfaceID = new Map();
  const windowsBySurfaceName = new Map();
  const windowOrder = new Map();
  collections.windows.forEach((windowItem, index) => {
    windowOrder.set(windowItem, index);
    appendLookupGroup(windowsBySurfaceID, windowItem.baseSurfaceId, windowItem);
    appendLookupGroup(windowsBySurfaceName, normalizeGeometryName(windowItem.baseSurfaceName), windowItem);
  });
  geometryLookupCache = {
    geometry,
    collections,
    lengths: Object.fromEntries(Object.entries(collections).map(([key, values]) => [key, values.length])),
    zoneByID: indexFirstBy(collections.zones, (zone) => zone.id),
    zoneByName: indexFirstBy(collections.zones, (zone) => normalizeGeometryName(zone.name)),
    zoneByExactName: indexFirstBy(collections.zones, (zone) => zone.name),
    spaceByID: indexFirstBy(collections.spaces, (space) => space.id),
    spaceByName: indexFirstBy(collections.spaces, (space) => normalizeGeometryName(space.name)),
    surfaceByID: indexFirstBy(collections.surfaces, (surface) => surface.id),
    surfaceByName: indexFirstBy(collections.surfaces, (surface) => normalizeGeometryName(surface.name)),
    windowByID: indexFirstBy(collections.windows, (windowItem) => windowItem.id),
    constructionByName: indexFirstBy(collections.constructions, (construction) => normalizeGeometryName(construction.name)),
    storyByIndex: indexFirstBy(collections.stories, (story) => story.index),
    windowsBySurfaceID,
    windowsBySurfaceName,
    windowOrder,
  };
  return geometryLookupCache;
}

function indexFirstBy(values, keyForValue) {
  const index = new Map();
  for (const value of values) {
    const key = keyForValue(value);
    if (key !== undefined && key !== null && key !== "" && !index.has(key)) index.set(key, value);
  }
  return index;
}

function appendLookupGroup(index, key, value) {
  if (key === undefined || key === null || key === "") return;
  const values = index.get(key) || [];
  values.push(value);
  index.set(key, values);
}

function zoneByName(geometry, zoneName) {
  const key = normalizeGeometryName(zoneName);
  return key ? geometryLookupIndex(geometry).zoneByName.get(key) || null : null;
}

function spaceByID(geometry, id) {
  return geometryLookupIndex(geometry).spaceByID.get(id) || null;
}

function spaceByName(geometry, name) {
  const key = normalizeGeometryName(name);
  return key ? geometryLookupIndex(geometry).spaceByName.get(key) || null : null;
}

function surfaceByID(geometry, id) {
  return geometryLookupIndex(geometry).surfaceByID.get(id) || null;
}

function surfaceByName(geometry, name) {
  const key = normalizeGeometryName(name);
  return key ? geometryLookupIndex(geometry).surfaceByName.get(key) || null : null;
}

function windowByID(geometry, id) {
  return geometryLookupIndex(geometry).windowByID.get(id) || null;
}

function windowsForSurface(geometry, surface) {
  const lookup = geometryLookupIndex(geometry);
  const surfaceName = normalizeGeometryName(surface?.name);
  const byID = surface?.id ? lookup.windowsBySurfaceID.get(surface.id) || [] : [];
  const byName = surfaceName ? lookup.windowsBySurfaceName.get(surfaceName) || [] : [];
  if (!byID.length) return [...byName];
  if (!byName.length) return [...byID];
  return [...new Set([...byID, ...byName])].sort((left, right) => lookup.windowOrder.get(left) - lookup.windowOrder.get(right));
}

function adjacentSurfaceForSurface(geometry, surface) {
  const boundaryName = boundaryObjectName(surface);
  return boundaryName ? surfaceByName(geometry, boundaryName) : null;
}

function boundaryObjectName(surface) {
  return fieldValueByCommentWords(surface?.fields, ["outside", "boundary", "condition", "object"]);
}

function fieldValueByCommentWords(fields = [], words = []) {
  const lowerWords = words.map((word) => word.toLowerCase());
  const field = fields.find((item) => {
    const comment = String(item.comment || "").toLowerCase();
    return lowerWords.every((word) => comment.includes(word));
  });
  return field?.value || "";
}

function storyLabelForIndex(geometry, storyIndex) {
  const story = geometryLookupIndex(geometry).storyByIndex.get(storyIndex);
  return story ? `${story.name} (${formatNumber(story.elevation)} m)` : "Story unknown";
}

function normalizeGeometryName(value) {
  return String(value || "").trim().toLowerCase();
}

function zoneIdForName(geometry, zoneName) {
  const zone = geometryLookupIndex(geometry).zoneByExactName.get(zoneName);
  return zone?.id || "";
}

function highlightSelectedMeshes() {
  if (!rendererState) {
    return;
  }
  const selectedColor = geometryColor("selected", 0xf0a202);
  const hoverColor = geometryColor("hover", 0x31a7a0);
  const counterpartColor = geometryColor("counterpart", 0x6c7fd8);
  rendererState.group.traverse((object) => {
    if (!object.material || !object.userData?.geometryId) {
      return;
    }
    const selectionRole = geometryRenderableSelectionRole(object.userData.geometryKind, object.userData.geometryId);
    const selected = Boolean(selectionRole);
    const hovered = !selected && geometryRenderableMatchesHover(object.userData.geometryKind, object.userData.geometryId);
    object.material.color.setHex(selectionRole === "counterpart" ? counterpartColor : selected ? selectedColor : hovered ? hoverColor : object.userData.baseColor);
    object.material.opacity = selected ? 0.95 : hovered ? Math.max(0.72, object.userData.baseOpacity) : object.userData.baseOpacity;
  });
  rendererState.renderer.render(rendererState.scene, rendererState.camera);
}

function highlightSelectedPlan() {
  elements.topologyPlan.querySelectorAll("[data-geometry-id]").forEach((shape) => {
    shape.classList.toggle(
      "selected",
      geometryRenderableMatchesSelection(shape.dataset.geometryKind, shape.dataset.geometryId),
    );
    shape.classList.toggle("hovered", geometryRenderableMatchesHover(shape.dataset.geometryKind, shape.dataset.geometryId));
    shape.classList.toggle("counterpart", geometryRenderableSelectionRole(shape.dataset.geometryKind, shape.dataset.geometryId) === "counterpart");
  });
}

function resizeRenderer() {
  const rect = elements.topology3DCanvasHost.getBoundingClientRect();
  const width = Math.max(1, Math.floor(rect.width));
  const height = Math.max(1, Math.floor(rect.height));
  rendererState.renderer.setSize(width, height, false);
  rendererState.camera.aspect = width / height;
  rendererState.camera.updateProjectionMatrix();
}

function geometryColor(name, fallback) {
  const value = getComputedStyle(document.documentElement).getPropertyValue(`--geometry-${name}`).trim();
  return parseHexColor(value, fallback);
}

function parseHexColor(value, fallback) {
  const color = String(value || "").trim();
  if (/^#[0-9a-f]{6}$/i.test(color)) {
    return Number.parseInt(color.slice(1), 16);
  }
  if (/^#[0-9a-f]{3}$/i.test(color)) {
    return Number.parseInt(color.slice(1).split("").map((char) => `${char}${char}`).join(""), 16);
  }
  return fallback;
}

function clearGroup(group) {
  while (group.children.length) {
    const child = group.children.pop();
    child.geometry?.dispose?.();
    if (Array.isArray(child.material)) {
      child.material.forEach((material) => material.dispose?.());
    } else {
      child.material?.dispose?.();
    }
  }
}

function disposeRendererCanvas() {
  if (rendererState?.renderer?.domElement?.parentElement) {
    rendererState.renderer.domElement.remove();
  }
}

function formatNumber(value) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return "0";
  }
  return number.toLocaleString(undefined, { maximumFractionDigits: 2 });
}
