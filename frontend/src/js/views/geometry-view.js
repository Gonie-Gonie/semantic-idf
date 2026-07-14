import * as THREE from "../../vendor/three.module.js";
import { elements, escapeHTML, state } from "../state.js";
import { t } from "../i18n.js";
import { refreshResultPanelSelectionStyles } from "../panel-navigation-adapters.js";
import { selectSemanticEntity } from "../selection-controller.js";

let rendererState = null;
let temporaryGeometryReveal = null;
let geometrySelectionRequest = 0;

window.addEventListener("idfAnalyzer:semanticSelectionChanged", (event) => {
  if (!temporaryGeometryReveal) {
    return;
  }
  const selection = geometrySelectionForTarget(temporaryGeometryReveal.kind, temporaryGeometryReveal.id);
  if (!selection?.entityId || selection.entityId !== event.detail?.selection?.entityId) {
    temporaryGeometryReveal = null;
    if (state.activeResultTab === "geometry" && state.report?.geometry) {
      renderGeometry();
    }
  }
});
window.addEventListener("idfAnalyzer:documentChanged", () => {
  temporaryGeometryReveal = null;
  geometrySelectionRequest += 1;
});

export function renderGeometry(geometry = state.report?.geometry) {
  if (!elements.geometryStats || !elements.geometryCanvasHost) {
    return;
  }
  if (!geometry) {
    renderEmptyGeometry();
    return;
  }

  ensureSelectedStory(geometry);
  if (elements.geometrySyncLocate) {
    elements.geometrySyncLocate.checked = state.geometrySyncLocate;
  }
  updateSelectionAidControl();
  elements.geometryStats.textContent = t("geometry.stats", {
    zones: geometry.zoneCount || 0,
    surfaces: geometry.surfaceCount || 0,
    windows: geometry.windowCount || 0,
  });
  renderStoryOptions(geometry);
  updateModeVisibility();
  if (state.geometryMode === "plan") {
    renderPlan(geometry);
  } else {
    renderScene(geometry);
  }
  renderGeometryDetails(geometry);
}

export function setGeometryMode(mode) {
  state.geometryMode = mode === "plan" ? "plan" : "3d";
  if (state.geometryMode === "plan" && state.selectedGeometryStory === "all") {
    state.selectedGeometryStory = firstStoryIndex(state.report?.geometry);
  }
  elements.geometryModeButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.geometryMode === state.geometryMode);
  });
  renderGeometry();
}

export function setGeometryStory(storyIndex) {
  state.selectedGeometryStory = storyIndex === "all" ? "all" : Number(storyIndex) || 0;
  renderGeometry();
}

export function setGeometrySelectionAid(enabled) {
  state.geometrySelectionAid = Boolean(enabled);
  updateSelectionAidControl();
  renderGeometry();
}

export async function selectGeometry(kind, id, options = {}) {
  const request = ++geometrySelectionRequest;
  const geometry = state.report?.geometry;
  const normalizedKind = normalizeGeometryKind(kind);
  const targetId = String(id || "");
  const entity = geometryTargetEntity({ targetKind: normalizedKind, targetId }, geometry);
  const selection = geometrySelectionForTarget(normalizedKind, targetId);
  const syncLocate = options.syncLocate !== false && state.geometrySyncLocate && Boolean(entity);

  // The legacy source locator already records a history entry. Run it before
  // changing either selection so a geometry click remains one atomic action.
  if (syncLocate) {
    syncLocatedInputEntity(entity);
  }
  if (options.syncSemantic !== false && selection) {
    await selectSemanticEntity(selection, {
      originView: "geometry",
      action: "select",
      recordHistory: syncLocate ? false : options.recordHistory !== false,
      follow: options.follow,
      rememberForOriginView: "geometry",
    });
  }
  if (request !== geometrySelectionRequest) {
    return;
  }

  temporaryGeometryReveal = null;
  state.selectedGeometryKind = normalizedKind;
  state.selectedGeometryId = targetId;
  renderGeometryDetails();
  highlightSelectedMeshes();
  highlightSelectedPlan();
  refreshResultPanelSelectionStyles("geometry", state.globalSelection, state.globalHover);
}

export async function revealGeometrySelection(selection, options = {}, context) {
  geometrySelectionRequest += 1;
  const geometry = state.report?.geometry;
  const target = geometryViewTargetForSelection(selection, context?.navigation);
  const entity = geometryTargetEntity(target, geometry);
  if (!target || !entity) {
    return context?.genericReveal(selection, options) || false;
  }

  if (entity.kind === "story") {
    state.selectedGeometryStory = entity.item.index;
    temporaryGeometryReveal = null;
    renderGeometry(geometry);
    const storySelect = elements.geometryStorySelect;
    if (options.focus !== false) {
      storySelect?.focus?.({ preventScroll: true });
    }
    context?.refreshSelectionStyles(selection, state.globalHover);
    return Boolean(storySelect);
  }

  const ownerZone = owningZoneForGeometryEntity(entity, geometry);
  const storyIndex = geometryStoryIndexForEntity(entity, geometry);
  if (Number.isInteger(storyIndex)) {
    state.selectedGeometryStory = storyIndex;
  }
  if (state.geometryMode === "plan" && !geometryEntityHasPlanShape(entity, geometry)) {
    state.geometryMode = "3d";
  }
  temporaryGeometryReveal = {
    kind: entity.kind,
    id: entity.id,
    ownerZoneId: ownerZone?.id || "",
    baseSurfaceId: entity.kind === "window" ? baseSurfaceForWindow(geometry, entity.item)?.id || "" : "",
  };
  state.selectedGeometryKind = entity.kind;
  state.selectedGeometryId = entity.id;
  renderGeometry(geometry);
  context?.refreshSelectionStyles(selection, state.globalHover);
  await nextGeometryFrame();
  const targetElement = findGeometryNavigationTarget(selection, target, context) || elements.geometryDetails;
  if (options.scroll !== false) {
    targetElement?.scrollIntoView?.({ block: options.block || "nearest", inline: "nearest", behavior: options.behavior || "auto" });
  }
  if (options.focus !== false) {
    targetElement?.focus?.({ preventScroll: true });
  }
  return Boolean(targetElement);
}

export async function restoreGeometryNavigationContext(snapshot = {}, context) {
  geometrySelectionRequest += 1;
  state.geometryMode = snapshot.mode === "plan" ? "plan" : "3d";
  state.selectedGeometryStory = snapshot.story === "all" ? "all" : Number(snapshot.story) || 0;
  state.selectedGeometryKind = normalizeGeometryKind(snapshot.selectedKind);
  state.selectedGeometryId = String(snapshot.selectedId || "");
  state.geometrySelectionAid = Boolean(snapshot.selectionAid);
  state.geometrySyncLocate = snapshot.syncLocate !== false;
  if (elements.geometryShowZones && typeof snapshot.visibility?.zones === "boolean") {
    elements.geometryShowZones.checked = snapshot.visibility.zones;
  }
  if (elements.geometryShowWalls && typeof snapshot.visibility?.walls === "boolean") {
    elements.geometryShowWalls.checked = snapshot.visibility.walls;
  }
  if (elements.geometryShowWindows && typeof snapshot.visibility?.windows === "boolean") {
    elements.geometryShowWindows.checked = snapshot.visibility.windows;
  }
  temporaryGeometryReveal = null;
  renderGeometry();
  return context?.genericRestoreContext(snapshot) ?? true;
}

export function preferredGeometrySemanticOccurrence(selection, context) {
  const target = geometryViewTargetForSelection(selection, context?.navigation);
  const preferred = preferredOccurrenceForGeometryTarget(target?.targetId, selection, context?.navigation);
  return preferred?.occurrenceId || context?.genericPreferredSemanticOccurrence(selection) || "";
}

export function resizeGeometry() {
  if (!rendererState || state.geometryMode !== "3d") {
    return;
  }
  resizeRenderer();
  rendererState.renderer.render(rendererState.scene, rendererState.camera);
}

function renderEmptyGeometry() {
  elements.geometryStats.textContent = t("geometry.stats", { zones: 0, surfaces: 0, windows: 0 });
  elements.geometryStorySelect.innerHTML = "";
  elements.geometryCanvasHost.innerHTML = `<div class="empty">${t("geometry.noGeometry")}</div>`;
  elements.geometryPlan.innerHTML = "";
  elements.geometryDetails.innerHTML = `<div class="empty">${t("geometry.selectObject")}</div>`;
}

function ensureSelectedStory(geometry) {
  const stories = geometry.stories || [];
  if (!stories.length) {
    state.selectedGeometryStory = state.geometryMode === "3d" ? "all" : 0;
    return;
  }
  if (state.geometryMode === "3d" && state.selectedGeometryStory === "all") {
    return;
  }
  const exists = stories.some((story) => story.index === state.selectedGeometryStory);
  if (!exists) {
    state.selectedGeometryStory = stories[0].index;
  }
}

function renderStoryOptions(geometry) {
  const stories = geometry.stories || [];
  const allOption =
    state.geometryMode === "3d"
      ? `<option value="all" ${state.selectedGeometryStory === "all" ? "selected" : ""}>${t("geometry.allLevels")}</option>`
      : "";
  const storyOptions = stories
    .map(
      (story) =>
        `<option value="${escapeHTML(story.index)}" ${geometryNavigationAttributes("story", geometryStoryTargetID(story), { objectName: story.name }, { tabindex: false })} ${story.index === state.selectedGeometryStory ? "selected" : ""}>${escapeHTML(story.name)} (${formatNumber(story.elevation)} m)</option>`,
    )
    .join("");
  elements.geometryStorySelect.innerHTML = `${allOption}${storyOptions}`;
}

function updateModeVisibility() {
  elements.geometryCanvasHost.classList.toggle("active", state.geometryMode === "3d");
  elements.geometryPlan.classList.toggle("active", state.geometryMode === "plan");
  elements.geometryModeButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.geometryMode === state.geometryMode);
  });
}

function updateSelectionAidControl() {
  if (!elements.geometrySelectionAid) {
    return;
  }
  elements.geometrySelectionAid.classList.toggle("active", state.geometrySelectionAid);
  elements.geometrySelectionAid.setAttribute("aria-pressed", String(state.geometrySelectionAid));
  elements.geometrySelectionAid.title = state.geometrySelectionAid ? "Surface selection aid on (H)" : "Surface selection aid off (H)";
}

function renderScene(geometry) {
  elements.geometryPlan.innerHTML = "";
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

  if (!state.geometrySelectionAid) {
    (geometry.surfaces || [])
      .filter((surface) => (
        matchesSelectedStory(surface) &&
        surface.surfaceType?.toLowerCase() === "floor" &&
        (elements.geometryShowZones.checked || geometryZoneSurfaceIsTemporarilyVisible(surface, geometry))
      ))
      .forEach((surface) => addSurfaceMesh(group, surface, "zone", zoneIdForName(geometry, surface.zoneName), center));
  }
  (geometry.surfaces || [])
    .filter((surface) => (
      matchesSelectedStory(surface) &&
      (state.geometrySelectionAid || surface.surfaceType?.toLowerCase() !== "floor") &&
      (elements.geometryShowWalls.checked || geometrySurfaceIsTemporarilyVisible(surface))
    ))
    .forEach((surface) => addSurfaceMesh(group, surface, "surface", surface.id, center));
  (geometry.windows || [])
    .filter((windowItem) => matchesSelectedStory(windowItem) && (elements.geometryShowWindows.checked || geometryWindowIsTemporarilyVisible(windowItem)))
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
    if (rendererState?.renderer !== renderer || state.geometryMode !== "3d") {
      return;
    }
    resizeRenderer();
    renderer.render(scene, camera);
  });
}

function ensureRenderer() {
  if (rendererState) {
    if (!elements.geometryCanvasHost.contains(rendererState.renderer.domElement)) {
      elements.geometryCanvasHost.innerHTML = "";
      elements.geometryCanvasHost.appendChild(rendererState.renderer.domElement);
    }
    return;
  }

  const scene = new THREE.Scene();
  scene.background = new THREE.Color(geometryColor("background", 0xf7fafc));
  const camera = new THREE.PerspectiveCamera(42, 1, 0.1, 1000);
  const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: false, preserveDrawingBuffer: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
  renderer.domElement.className = "geometry-canvas";
  elements.geometryCanvasHost.innerHTML = "";
  elements.geometryCanvasHost.appendChild(renderer.domElement);

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
      if (!rendererState || state.geometryMode !== "3d" || !elements.geometryCanvasHost.contains(rendererState.renderer.domElement)) {
        return;
      }
      resizeRenderer();
      rendererState.renderer.render(rendererState.scene, rendererState.camera);
    });
  });
  rendererState.resizeObserver.observe(elements.geometryCanvasHost);
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

function pickMesh(event) {
  const rect = rendererState.renderer.domElement.getBoundingClientRect();
  rendererState.pointer.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
  rendererState.pointer.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;
  rendererState.raycaster.setFromCamera(rendererState.pointer, rendererState.camera);
  const hits = rendererState.raycaster.intersectObjects(rendererState.group.children, true);
  const hit = hits.find((item) => item.object.userData?.geometryId);
  if (hit) {
    void selectGeometry(hit.object.userData.geometryKind, hit.object.userData.geometryId);
  }
}

function addSurfaceMesh(group, surface, kind, id, center) {
  const geometry = polygonGeometry(surface.vertices, center, state.geometrySelectionAid && kind === "surface" ? 0.055 : 0);
  if (!geometry) {
    return;
  }
  const isZone = kind === "zone";
  const isRoof = /roof|ceiling/i.test(surface.surfaceType || "");
  const isFloor = /floor/i.test(surface.surfaceType || "");
  const isInterior = /surface|zone|adiabatic/i.test(surface.outsideBoundary || "");
  const baseColor = isZone
    ? geometryColor("zone", 0xb8d7b0)
    : isRoof
      ? geometryColor("roof", 0xb8b0a1)
      : isFloor
        ? 0x98b8a7
        : geometryColor("wall", 0x7b9cbc);
  const material = new THREE.MeshStandardMaterial({
    color: baseColor,
    emissive: state.geometrySelectionAid && kind === "surface" ? new THREE.Color(baseColor).multiplyScalar(isInterior ? 0.35 : 0.22).getHex() : 0x000000,
    emissiveIntensity: state.geometrySelectionAid && kind === "surface" ? 0.16 : 0,
    roughness: 0.72,
    metalness: 0,
    transparent: true,
    opacity: state.geometrySelectionAid && kind === "surface" ? (isFloor ? 0.64 : 0.82) : isZone ? 0.5 : 0.72,
    depthWrite: !(state.geometrySelectionAid && kind === "surface"),
    depthTest: !(state.geometrySelectionAid && kind === "surface"),
    side: THREE.DoubleSide,
  });
  const mesh = new THREE.Mesh(geometry, material);
  mesh.userData = {
    geometryKind: kind,
    geometryId: id,
    semanticSelection: geometrySelectionForTarget(kind, id),
    baseColor: material.color.getHex(),
    baseOpacity: material.opacity,
  };
  group.add(mesh);
  if (state.geometrySelectionAid && kind === "surface") {
    addSurfaceOutline(group, surface, id, center, baseColor);
  }
}

function addWindowMesh(group, windowItem, center) {
  const geometry = polygonGeometry(windowItem.vertices, center, 0.035);
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
    semanticSelection: geometrySelectionForTarget("window", windowItem.id),
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

function addSurfaceOutline(group, surface, id, center, color) {
  if (!surface.vertices || surface.vertices.length < 2) {
    return;
  }
  const points = surface.vertices.map((point) => new THREE.Vector3(point.x - center.x, point.z - center.y, point.y - center.z));
  points.push(points[0].clone());
  const geometry = new THREE.BufferGeometry().setFromPoints(points);
  const material = new THREE.LineBasicMaterial({
    color,
    transparent: true,
    opacity: 0.68,
    depthTest: false,
  });
  const line = new THREE.Line(geometry, material);
  line.userData = { geometryKind: "surface", geometryId: id, baseColor: material.color.getHex(), baseOpacity: material.opacity };
  group.add(line);
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
  elements.geometryPlan.classList.toggle("selection-aid", state.geometrySelectionAid);
  const storyIndex = state.selectedGeometryStory === "all" ? firstStoryIndex(geometry) : state.selectedGeometryStory;
  const layout = cachedGeometryPlanLayout(geometry, storyIndex);
  if (!layout.ok) {
    elements.geometryPlan.innerHTML = `<text x="24" y="42" fill="#60707c" font-size="14">${t("geometry.noFloorPlan")}</text>`;
    elements.geometryPlan.setAttribute("viewBox", "0 0 640 420");
    return;
  }

  const zoneFloorPolygons = elements.geometryShowZones.checked
    ? layout.surfaces
        .filter((surface) => surface.isFloor)
        .map((surface) => `<polygon class="plan-zone navigable-row" data-geometry-kind="zone" data-geometry-id="${escapeHTML(surface.zoneID)}" ${geometryNavigationAttributes("zone", surface.zoneID)} points="${surface.openPoints}"></polygon>`)
        .join("")
    : layout.surfaces
        .filter((surface) => surface.isFloor && temporaryGeometryReveal?.ownerZoneId === surface.zoneID)
        .map((surface) => `<polygon class="plan-zone navigable-row" data-geometry-kind="zone" data-geometry-id="${escapeHTML(surface.zoneID)}" ${geometryNavigationAttributes("zone", surface.zoneID)} points="${surface.openPoints}"></polygon>`)
        .join("");
  const wallLines = layout.surfaces
    .filter((surface) => (
      (state.geometrySelectionAid || !surface.isFloor) &&
      (elements.geometryShowWalls.checked || projectedSurfaceIsTemporarilyVisible(surface))
    ))
    .map(renderPlanSurfaceShape)
    .join("");
  const windowLines = layout.windows
    .filter((windowItem) => elements.geometryShowWindows.checked || temporaryGeometryReveal?.id === windowItem.id)
    .map((windowItem) => `<polyline class="plan-window navigable-row" data-geometry-kind="window" data-geometry-id="${escapeHTML(windowItem.id)}" ${geometryNavigationAttributes("fenestration", windowItem.id)} points="${windowItem.closedPoints}"></polyline>`)
    .join("");

  elements.geometryPlan.setAttribute("viewBox", `0 0 ${layout.viewWidth} ${layout.viewHeight}`);
  elements.geometryPlan.innerHTML = `${zoneFloorPolygons}${wallLines}${windowLines}`;
  elements.geometryPlan.querySelectorAll("[data-geometry-id]").forEach((shape) => {
    shape.addEventListener("click", (event) => {
      event.stopPropagation();
      void selectGeometry(shape.dataset.geometryKind, shape.dataset.geometryId);
    });
    shape.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" && event.key !== " ") {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      void selectGeometry(shape.dataset.geometryKind, shape.dataset.geometryId);
    });
  });
  highlightSelectedPlan();
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
    const openPoints = surface.vertices.map(project).join(" ");
    return {
      id: surface.id,
      title: `${surface.name || surface.type} / ${surface.surfaceType || "Surface"}`,
      className: `plan-surface ${planSurfaceClass(surface)}`,
      openPoints,
      closedPoints: `${openPoints} ${project(surface.vertices[0])}`,
      isHorizontal: isHorizontalSurface(surface),
      isFloor: surface.surfaceType?.toLowerCase() === "floor",
      zoneID: zoneIdForName(geometry, surface.zoneName),
      spaceName: surface.spaceName || "",
    };
  });
  const projectedWindows = windows.map((windowItem) => {
    const openPoints = windowItem.vertices.map(project).join(" ");
    return {
      id: windowItem.id,
      closedPoints: `${openPoints} ${project(windowItem.vertices[0])}`,
    };
  });
  return { ok: true, viewWidth, viewHeight, surfaces: projectedSurfaces, windows: projectedWindows };
}

function renderPlanSurfaceShape(surface) {
  const title = escapeHTML(surface.title);
  const attributes = geometryNavigationAttributes("surface", surface.id);
  if (surface.isHorizontal) {
    return `<polygon class="${surface.className} navigable-row" data-geometry-kind="surface" data-geometry-id="${escapeHTML(surface.id)}" ${attributes} points="${surface.closedPoints}"><title>${title}</title></polygon>`;
  }
  return `<polyline class="plan-wall ${surface.className} navigable-row" data-geometry-kind="surface" data-geometry-id="${escapeHTML(surface.id)}" ${attributes} points="${surface.closedPoints}"><title>${title}</title></polyline>`;
}

function hasPlanVertices(item) {
  return Array.isArray(item?.vertices) && item.vertices.length > 0;
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

function renderGeometryDetails(geometry = state.report?.geometry) {
  const entity = selectedGeometryEntity(geometry);
  if (!entity) {
    elements.geometryDetails.innerHTML = `<div class="empty">${t("geometry.selectObject")}</div>`;
    return;
  }
  const relatedGroups = geometryRelatedGroups(geometry, entity);
  elements.geometryDetails.innerHTML = `
    <div class="geometry-detail-head navigable-row" ${geometryNavigationAttributes(entity.kind, entity.id, {
      objectIndex: entity.objectIndex,
      objectType: entity.objectType,
      objectName: entity.title,
    })}>
      <div>
        <h3>${escapeHTML(entity.title)}</h3>
        <span>${escapeHTML(entity.subtitle)}</span>
      </div>
      <span class="geometry-sync-note">${state.geometrySyncLocate ? t("geometry.syncOn") : t("geometry.syncOff")}</span>
    </div>
    <div class="geometry-detail-grid">
      <section>
        <h4>Metrics</h4>
        ${renderMetricList(entity.metrics)}
      </section>
      <section>
        <h4>${t("geometry.relatedObjects")}</h4>
        ${renderRelatedGroups(relatedGroups)}
      </section>
      ${renderConstructionSection(geometry, entity)}
    </div>`;
  bindGeometryDetailControls();
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
  return state.selectedGeometryStory === "all" || item.storyIndex === state.selectedGeometryStory;
}

function firstStoryIndex(geometry) {
  return geometry?.stories?.[0]?.index ?? 0;
}

function selectedGeometryEntity(geometry) {
  if (!geometry || !state.selectedGeometryId) {
    return null;
  }
  if (state.selectedGeometryKind === "zone") {
    const zone = (geometry.zones || []).find((item) => item.id === state.selectedGeometryId);
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
  if (state.selectedGeometryKind === "space") {
    const space = (geometry.spaces || []).find((item) => item.id === state.selectedGeometryId);
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
  if (state.selectedGeometryKind === "window") {
    const windowItem = (geometry.windows || []).find((item) => item.id === state.selectedGeometryId);
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
  const surface = (geometry.surfaces || []).find((item) => item.id === state.selectedGeometryId);
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
    ? `<div class="geometry-property-list">${metrics
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
    <div class="geometry-related-groups">
      ${visibleGroups
        .map(
          (group) => `
            <details class="geometry-related-group" open>
              <summary>
                <span>${escapeHTML(group.title)}</span>
                <span class="badge">${escapeHTML(group.items.length)}</span>
              </summary>
              <div class="geometry-related-list">
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
    <section class="geometry-construction-section">
      <h4>${t("geometry.construction", {}, "Construction")}</h4>
      ${construction ? renderConstructionGraphic(construction, constructionSidesForEntity(geometry, entity)) : `<div class="empty">${t("geometry.noConstruction", {}, "No construction layers parsed")}: ${escapeHTML(constructionName)}</div>`}
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
  return key ? (geometry?.constructions || []).find((construction) => normalizeGeometryName(construction.name) === key) || null : null;
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
      inside: `${t("geometry.thisSurface", {}, "This surface")}${surface.zoneName ? ` / ${surface.zoneName}` : ""}`,
    };
  }
  if (entity?.kind === "window") {
    const windowItem = entity.item;
    const baseSurface = windowItem.baseSurfaceId
      ? surfaceByID(geometry, windowItem.baseSurfaceId)
      : surfaceByName(geometry, windowItem.baseSurfaceName);
    return {
      outside: baseSurface ? surfaceOutsideLabel(geometry, baseSurface) : windowItem.baseSurfaceName || t("common.notAvailable", {}, "N/A"),
      inside: `${t("geometry.thisOpening", {}, "This opening")}${windowItem.zoneName ? ` / ${windowItem.zoneName}` : ""}`,
    };
  }
  return {
    outside: t("common.notAvailable", {}, "N/A"),
    inside: t("common.notAvailable", {}, "N/A"),
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
  return boundary || t("common.notAvailable", {}, "N/A");
}

function renderConstructionGraphic(construction, sides) {
  const layers = construction.layers || [];
  const totalThickness = layers.reduce((sum, layer) => sum + (layer.hasThickness ? Number(layer.thickness) || 0 : 0), 0);
  const performance = constructionPerformance(construction);
  return `
    <div class="construction-card">
      <div class="construction-card-head">
        <strong>${escapeHTML(construction.name)}</strong>
        <span>${construction.hasThickness ? `${t("geometry.totalThickness", {}, "Total thickness")} ${formatThickness(construction.totalThickness || totalThickness)}` : t("geometry.thicknessUnknown", {}, "Thickness unknown")} / ${t("geometry.outsideToInside", {}, "Outside to inside")}</span>
      </div>
      <div class="construction-performance">
        <span><strong>${t("geometry.uValue", {}, "U-value")}</strong><em>${formatUValue(performance.uValue)}</em></span>
        <span><strong>${t("geometry.heatCapacity", {}, "Heat capacity")}</strong><em>${formatArealHeatCapacity(performance.arealHeatCapacity)}</em></span>
      </div>
      <div class="construction-stack-frame">
        <span class="construction-side-label">${t("geometry.outside", {}, "Outside")} <em>${escapeHTML(sides.outside)}</em></span>
        <div class="construction-stack" role="img" aria-label="${escapeHTML(construction.name)} construction layers">
          ${layers.length ? layers.map((layer, index) => renderConstructionLayer(layer, index, totalThickness)).join("") : `<div class="empty">${t("geometry.noConstruction", {}, "No construction layers parsed")}</div>`}
        </div>
        <span class="construction-side-label">${t("geometry.inside", {}, "Inside")} <em>${escapeHTML(sides.inside)}</em></span>
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
        <span>${escapeHTML(details.join(" / ") || t("common.notAvailable", {}, "N/A"))}</span>
      </span>
      <span class="construction-layer-thickness">${escapeHTML(layer.hasThickness ? formatThickness(layer.thickness) : t("geometry.thicknessUnknown", {}, "Thickness unknown"))}</span>
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
    return t("common.notAvailable", {}, "N/A");
  }
  if (number < 1) {
    return `${(number * 1000).toLocaleString(undefined, { maximumFractionDigits: 1 })} mm`;
  }
  return `${number.toLocaleString(undefined, { maximumFractionDigits: 3 })} m`;
}

function formatUValue(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) {
    return t("common.notAvailable", {}, "N/A");
  }
  return `${number.toLocaleString(undefined, { maximumFractionDigits: 3 })} W/m2-K`;
}

function formatArealHeatCapacity(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) {
    return t("common.notAvailable", {}, "N/A");
  }
  return `${(number / 1000).toLocaleString(undefined, { maximumFractionDigits: 1 })} kJ/m2-K`;
}

function formatArea(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) {
    return t("common.notAvailable", {}, "N/A");
  }
  return `${number.toLocaleString(undefined, { maximumFractionDigits: 2 })} m2`;
}

function renderRelatedItem(item) {
  const content = `
    <span class="geometry-related-main">
      <strong>${escapeHTML(item.title)}</strong>
      <span>${escapeHTML(item.subtitle)}</span>
      ${item.detail ? `<em>${escapeHTML(item.detail)}</em>` : ""}
    </span>
    <span class="geometry-related-role">${escapeHTML(item.role)}</span>`;
  if (item.kind && item.id) {
    return `<button class="geometry-related-row navigable-row" type="button" data-geometry-kind="${escapeHTML(item.kind)}" data-geometry-id="${escapeHTML(item.id)}" ${geometryNavigationAttributes(item.kind, item.id, item.sourceAnchor)}>${content}</button>`;
  }
  return `<div class="geometry-related-row geometry-related-static">${content}</div>`;
}

function bindGeometryDetailControls() {
  elements.geometryDetails.querySelectorAll(".geometry-related-row[data-geometry-id]").forEach((button) => {
    button.addEventListener("click", (event) => {
      event.stopPropagation();
      void selectGeometry(button.dataset.geometryKind, button.dataset.geometryId);
    });
  });
  elements.geometryDetails.querySelectorAll(".construction-layer[data-object-index]").forEach((button) => {
    button.addEventListener("click", () => {
      const objectIndex = Number(button.dataset.objectIndex);
      if (!Number.isFinite(objectIndex) || objectIndex < 0) {
        return;
      }
      window.dispatchEvent(
        new CustomEvent("idfAnalyzer:geometryLocate", {
          detail: {
            objectIndex,
          },
        }),
      );
    });
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
    `${t("geometry.area", {}, "Area")} ${formatArea(surface.area)}`,
    `${t("geometry.uValue", {}, "U-value")} ${formatUValue(performance.uValue)}`,
    `${t("geometry.boundary", {}, "Boundary")} ${surfaceOutsideLabel(geometry, surface)}`,
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
    `${t("geometry.area", {}, "Area")} ${formatArea(windowItem.area)}`,
    `${t("geometry.uValue", {}, "U-value")} ${formatUValue(performance.uValue)}`,
    windowItem.baseSurfaceName ? `${t("geometry.baseSurface", {}, "Base surface")} ${windowItem.baseSurfaceName}` : "",
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

function geometryTargetEntity(target, geometry = state.report?.geometry) {
  if (!target || !geometry) {
    return null;
  }
  const targetId = String(target.targetId || "");
  const requestedKind = normalizeGeometryKind(target.targetKind);
  const candidates = requestedKind ? [requestedKind] : ["zone", "space", "surface", "window", "story"];
  for (const kind of candidates) {
    if (kind === "zone") {
      const item = (geometry.zones || []).find((candidate) => candidate.id === targetId);
      if (item) return { kind, id: item.id, item, objectIndex: item.objectIndex, objectType: "Zone", storyIndex: item.storyIndex };
    } else if (kind === "space") {
      const item = (geometry.spaces || []).find((candidate) => candidate.id === targetId);
      const zone = item ? zoneByName(geometry, item.zoneName) : null;
      if (item) return { kind, id: item.id, item, objectIndex: item.objectIndex, objectType: "Space", storyIndex: zone?.storyIndex };
    } else if (kind === "surface") {
      const item = (geometry.surfaces || []).find((candidate) => candidate.id === targetId);
      if (item) return { kind, id: item.id, item, objectIndex: item.objectIndex, objectType: item.type, storyIndex: item.storyIndex };
    } else if (kind === "window") {
      const item = (geometry.windows || []).find((candidate) => candidate.id === targetId);
      if (item) return { kind, id: item.id, item, objectIndex: item.objectIndex, objectType: item.type, storyIndex: item.storyIndex };
    } else if (kind === "story") {
      const item = (geometry.stories || []).find((story) => geometryStoryMatchesTarget(story, targetId));
      if (item) return { kind, id: geometryStoryTargetID(item), item, storyIndex: item.index };
    }
  }
  if (requestedKind) {
    return geometryTargetEntity({ ...target, targetKind: "" }, geometry);
  }
  return null;
}

function geometrySelectionForTarget(kind, targetId, navigation = state.semanticProjection?.navigation || {}) {
  if (!targetId) {
    return null;
  }
  const occurrence = preferredOccurrenceForGeometryTarget(targetId, state.globalSelection, navigation);
  const entity = (navigation.entities || []).find((candidate) => candidate.id === occurrence?.entityId) || null;
  if (!occurrence || !entity) {
    return null;
  }
  return {
    entityId: entity.id,
    entityKind: entity.kind || normalizeGeometryEntityKind(kind),
    occurrenceId: occurrence.occurrenceId,
    sourceAnchor: { ...(occurrence.sourceAnchor || entity.sourceAnchors?.[0] || {}) },
    originView: "geometry",
    originTargetId: String(targetId),
    semanticPathHint: occurrence.path || "",
    relatedEntityIds: [...(entity.relatedEntityIds || [])],
  };
}

function preferredOccurrenceForGeometryTarget(targetId, selection = {}, navigation = state.semanticProjection?.navigation || {}) {
  const occurrenceIds = navigation.byViewTarget?.[`geometry|${targetId}`] || [];
  const currentPath = String(selection.semanticPathHint || state.semanticCurrentPath || "");
  return (navigation.occurrences || [])
    .filter((occurrence) => occurrenceIds.includes(occurrence.occurrenceId))
    .map((occurrence, order) => ({
      occurrence,
      order,
      exact: Number(occurrence.occurrenceId === selection.occurrenceId),
      current: Number(occurrence.occurrenceId === state.semanticCurrentOccurrenceId),
      geometryContext: Number(occurrence.contextKind === "zone_geometry" || /(^|\/)geometry(\/|$)/.test(occurrence.path || "")),
      path: commonPathPrefixLength(occurrence.path, currentPath),
      preferred: Number(occurrence.preferredView === "geometry"),
    }))
    .sort((left, right) => (
      right.geometryContext - left.geometryContext ||
      right.exact - left.exact ||
      right.current - left.current ||
      right.path - left.path ||
      right.preferred - left.preferred ||
      left.order - right.order
    ))[0]?.occurrence || null;
}

function geometryNavigationAttributes(kind, targetId, explicitAnchor = {}, options = {}) {
  const navigation = state.semanticProjection?.navigation || {};
  const occurrence = preferredOccurrenceForGeometryTarget(targetId, state.globalSelection, navigation);
  const entity = (navigation.entities || []).find((candidate) => candidate.id === occurrence?.entityId) || null;
  const sourceAnchor = { ...(occurrence?.sourceAnchor || entity?.sourceAnchors?.[0] || {}), ...explicitAnchor };
  const selected = Boolean(
    (entity?.id && entity.id === state.globalSelection?.entityId) ||
    (String(targetId || "") === state.selectedGeometryId && normalizeGeometryKind(kind) === state.selectedGeometryKind),
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
      String(candidate?.view || "").toLowerCase() === "geometry" &&
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

function findGeometryNavigationTarget(selection, target, context) {
  const root = context?.root || document.getElementById("geometryPane");
  const items = [...(root?.querySelectorAll?.("[data-panel-target-id], [data-entity-id]") || [])];
  return items.find((item) => item.dataset.panelTargetId === String(target?.targetId || "")) ||
    items.find((item) => item.dataset.entityId === String(selection?.entityId || "")) ||
    context?.genericFindTarget(selection) || null;
}

function nextGeometryFrame() {
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
  return Boolean(
    temporaryGeometryReveal?.ownerZoneId &&
    temporaryGeometryReveal.ownerZoneId === zoneIdForName(geometry, surface.zoneName),
  );
}

function geometrySurfaceIsTemporarilyVisible(surface) {
  if (temporaryGeometryReveal?.kind === "surface" && temporaryGeometryReveal.id === surface.id) {
    return true;
  }
  if (temporaryGeometryReveal?.kind === "space") {
    const space = spaceByID(state.report?.geometry, temporaryGeometryReveal.id);
    return Boolean(space && normalizeGeometryName(surface.spaceName) === normalizeGeometryName(space.name));
  }
  if (temporaryGeometryReveal?.kind === "zone") {
    const zone = zoneByName(state.report?.geometry, surface.zoneName);
    return zone?.id === temporaryGeometryReveal.id;
  }
  return temporaryGeometryReveal?.baseSurfaceId === surface.id;
}

function projectedSurfaceIsTemporarilyVisible(surface) {
  if (temporaryGeometryReveal?.kind === "surface" && temporaryGeometryReveal.id === surface.id) {
    return true;
  }
  if (temporaryGeometryReveal?.kind === "space") {
    const space = spaceByID(state.report?.geometry, temporaryGeometryReveal.id);
    return Boolean(space && normalizeGeometryName(surface.spaceName) === normalizeGeometryName(space.name));
  }
  if (temporaryGeometryReveal?.kind === "zone") {
    return surface.zoneID === temporaryGeometryReveal.id;
  }
  return temporaryGeometryReveal?.baseSurfaceId === surface.id;
}

function geometryWindowIsTemporarilyVisible(windowItem) {
  return temporaryGeometryReveal?.kind === "window" && temporaryGeometryReveal.id === windowItem.id;
}

function geometryRenderableMatchesSelection(kind, id) {
  const normalizedKind = normalizeGeometryKind(kind);
  if (normalizedKind === state.selectedGeometryKind && String(id || "") === state.selectedGeometryId) {
    return true;
  }
  if (state.selectedGeometryKind === "zone" && normalizedKind === "surface") {
    const zone = (state.report?.geometry?.zones || []).find((item) => item.id === state.selectedGeometryId);
    const surface = surfaceByID(state.report?.geometry, id);
    return Boolean(zone && surface && normalizeGeometryName(surface.zoneName) === normalizeGeometryName(zone.name));
  }
  if (state.selectedGeometryKind !== "space") {
    return false;
  }
  const space = spaceByID(state.report?.geometry, state.selectedGeometryId);
  if (normalizedKind === "zone") {
    return Boolean(space && zoneByName(state.report?.geometry, space.zoneName)?.id === id);
  }
  if (normalizedKind !== "surface") {
    return false;
  }
  const surface = surfaceByID(state.report?.geometry, id);
  return Boolean(space && surface && normalizeGeometryName(surface.spaceName) === normalizeGeometryName(space.name));
}

function zoneByName(geometry, zoneName) {
  const key = normalizeGeometryName(zoneName);
  return key ? (geometry?.zones || []).find((zone) => normalizeGeometryName(zone.name) === key) || null : null;
}

function spaceByID(geometry, id) {
  return (geometry?.spaces || []).find((space) => space.id === id) || null;
}

function spaceByName(geometry, name) {
  const key = normalizeGeometryName(name);
  return key ? (geometry?.spaces || []).find((space) => normalizeGeometryName(space.name) === key) || null : null;
}

function surfaceByID(geometry, id) {
  return (geometry?.surfaces || []).find((surface) => surface.id === id) || null;
}

function surfaceByName(geometry, name) {
  const key = normalizeGeometryName(name);
  return key ? (geometry?.surfaces || []).find((surface) => normalizeGeometryName(surface.name) === key) || null : null;
}

function windowByID(geometry, id) {
  return (geometry?.windows || []).find((windowItem) => windowItem.id === id) || null;
}

function windowsForSurface(geometry, surface) {
  const surfaceName = normalizeGeometryName(surface?.name);
  return (geometry?.windows || []).filter(
    (windowItem) =>
      (surface?.id && windowItem.baseSurfaceId === surface.id) ||
      (surfaceName && normalizeGeometryName(windowItem.baseSurfaceName) === surfaceName),
  );
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
  const story = (geometry?.stories || []).find((item) => item.index === storyIndex);
  return story ? `${story.name} (${formatNumber(story.elevation)} m)` : "Story unknown";
}

function normalizeGeometryName(value) {
  return String(value || "").trim().toLowerCase();
}

function zoneIdForName(geometry, zoneName) {
  const zone = (geometry.zones || []).find((item) => item.name === zoneName);
  return zone?.id || "";
}

function highlightSelectedMeshes() {
  if (!rendererState) {
    return;
  }
  const selectedColor = geometryColor("selected", 0xf0a202);
  rendererState.group.traverse((object) => {
    if (!object.material || !object.userData?.geometryId) {
      return;
    }
    const selected = geometryRenderableMatchesSelection(object.userData.geometryKind, object.userData.geometryId);
    object.material.color.setHex(selected ? selectedColor : object.userData.baseColor);
    object.material.opacity = selected ? 0.95 : object.userData.baseOpacity;
  });
  rendererState.renderer.render(rendererState.scene, rendererState.camera);
}

function highlightSelectedPlan() {
  elements.geometryPlan.querySelectorAll("[data-geometry-id]").forEach((shape) => {
    shape.classList.toggle(
      "selected",
      geometryRenderableMatchesSelection(shape.dataset.geometryKind, shape.dataset.geometryId),
    );
  });
}

function resizeRenderer() {
  const rect = elements.geometryCanvasHost.getBoundingClientRect();
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
