import * as THREE from "../vendor/three.module.js";
import { elements, escapeHTML, state } from "./state.js";
import { t } from "./i18n.js";

let rendererState = null;

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

export function selectGeometry(kind, id) {
  state.selectedGeometryKind = kind || "";
  state.selectedGeometryId = id || "";
  renderGeometryDetails();
  highlightSelectedMeshes();
  highlightSelectedPlan();
  syncLocatedInputFromSelection();
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
        `<option value="${escapeHTML(story.index)}" ${story.index === state.selectedGeometryStory ? "selected" : ""}>${escapeHTML(story.name)} (${formatNumber(story.elevation)} m)</option>`,
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

  if (elements.geometryShowZones.checked && !state.geometrySelectionAid) {
    (geometry.surfaces || [])
      .filter((surface) => matchesSelectedStory(surface) && surface.surfaceType?.toLowerCase() === "floor")
      .forEach((surface) => addSurfaceMesh(group, surface, "zone", zoneIdForName(geometry, surface.zoneName), center));
  }
  if (elements.geometryShowWalls.checked) {
    (geometry.surfaces || [])
      .filter((surface) => matchesSelectedStory(surface) && (state.geometrySelectionAid || surface.surfaceType?.toLowerCase() !== "floor"))
      .forEach((surface) => addSurfaceMesh(group, surface, "surface", surface.id, center));
  }
  if (elements.geometryShowWindows.checked) {
    (geometry.windows || [])
      .filter((windowItem) => matchesSelectedStory(windowItem))
      .forEach((windowItem) => addWindowMesh(group, windowItem, center));
  }

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
    selectGeometry(hit.object.userData.geometryKind, hit.object.userData.geometryId);
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
  mesh.userData = { geometryKind: kind, geometryId: id, baseColor: material.color.getHex(), baseOpacity: material.opacity };
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
  mesh.userData = { geometryKind: "window", geometryId: windowItem.id, baseColor: material.color.getHex(), baseOpacity: material.opacity };
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
  const surfaces = (geometry.surfaces || []).filter((surface) => surface.storyIndex === storyIndex);
  const windows = (geometry.windows || []).filter((windowItem) => windowItem.storyIndex === storyIndex);
  const bounds = geometry.bounds || {};
  if (!bounds.ok || (!surfaces.length && !windows.length)) {
    elements.geometryPlan.innerHTML = `<text x="24" y="42" fill="#60707c" font-size="14">${t("geometry.noFloorPlan")}</text>`;
    elements.geometryPlan.setAttribute("viewBox", "0 0 640 420");
    return;
  }

  const pad = 18;
  const width = Math.max(bounds.maxX - bounds.minX, 1);
  const height = Math.max(bounds.maxY - bounds.minY, 1);
  const viewWidth = 760;
  const viewHeight = Math.max(360, Math.round((height / width) * 760));
  const scale = Math.min((viewWidth - pad * 2) / width, (viewHeight - pad * 2) / height);
  const project = (point) => `${pad + (point.x - bounds.minX) * scale},${viewHeight - pad - (point.y - bounds.minY) * scale}`;

  const zoneFloorPolygons = elements.geometryShowZones.checked
    ? surfaces
        .filter((surface) => surface.surfaceType?.toLowerCase() === "floor")
        .map((surface) => {
          const zoneID = zoneIdForName(geometry, surface.zoneName);
          return `<polygon class="plan-zone" data-geometry-kind="zone" data-geometry-id="${escapeHTML(zoneID)}" points="${surface.vertices.map(project).join(" ")}"></polygon>`;
        })
        .join("")
    : "";
  const wallLines = elements.geometryShowWalls.checked
    ? surfaces
        .filter((surface) => state.geometrySelectionAid || surface.surfaceType?.toLowerCase() !== "floor")
        .map((surface) => renderPlanSurfaceShape(surface, project))
        .join("")
    : "";
  const windowLines = elements.geometryShowWindows.checked
    ? windows
        .map((windowItem) => `<polyline class="plan-window" data-geometry-kind="window" data-geometry-id="${escapeHTML(windowItem.id)}" points="${windowItem.vertices.map(project).join(" ")} ${project(windowItem.vertices[0])}"></polyline>`)
        .join("")
    : "";

  elements.geometryPlan.setAttribute("viewBox", `0 0 ${viewWidth} ${viewHeight}`);
  elements.geometryPlan.innerHTML = `${zoneFloorPolygons}${wallLines}${windowLines}`;
  elements.geometryPlan.querySelectorAll("[data-geometry-id]").forEach((shape) => {
    shape.addEventListener("click", () => selectGeometry(shape.dataset.geometryKind, shape.dataset.geometryId));
  });
  highlightSelectedPlan();
}

function renderPlanSurfaceShape(surface, project) {
  const points = `${surface.vertices.map(project).join(" ")} ${project(surface.vertices[0])}`;
  const className = `plan-surface ${planSurfaceClass(surface)}`;
  const title = escapeHTML(`${surface.name || surface.type} / ${surface.surfaceType || "Surface"}`);
  if (isHorizontalSurface(surface)) {
    return `<polygon class="${className}" data-geometry-kind="surface" data-geometry-id="${escapeHTML(surface.id)}" points="${points}"><title>${title}</title></polygon>`;
  }
  return `<polyline class="plan-wall ${className}" data-geometry-kind="surface" data-geometry-id="${escapeHTML(surface.id)}" points="${points}"><title>${title}</title></polyline>`;
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
    <div class="geometry-detail-head">
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

function syncLocatedInputFromSelection() {
  if (!state.geometrySyncLocate) {
    return;
  }
  const entity = selectedGeometryEntity(state.report?.geometry);
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
  if (entity.kind === "window") {
    return geometryRelatedGroupsForWindow(geometry, entity.item);
  }
  return geometryRelatedGroupsForSurface(geometry, entity.item);
}

function geometryRelatedGroupsForZone(geometry, zone) {
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
    { title: "Boundary Surfaces", items: surfaces.map((surface) => relatedItemForSurface(surface, surface.surfaceType || "Surface", geometry)) },
    { title: "Openings", items: windows.map((windowItem) => relatedItemForWindow(windowItem, windowItem.surfaceType || "Window", geometry)) },
    { title: "Adjacent", items: adjacent },
  ];
}

function geometryRelatedGroupsForSurface(geometry, surface) {
  const parentZone = zoneByName(geometry, surface.zoneName);
  const windows = windowsForSurface(geometry, surface);
  const adjacentSurface = adjacentSurfaceForSurface(geometry, surface);
  const adjacentZone = adjacentSurface ? zoneByName(geometry, adjacentSurface.zoneName) : null;
  const adjacentItems = [
    adjacentZone && adjacentZone.id !== parentZone?.id ? relatedItemForZone(adjacentZone, "Adjacent zone") : null,
    adjacentSurface ? relatedItemForSurface(adjacentSurface, "Adjacent surface", geometry) : referencedBoundaryItem(surface),
  ].filter(Boolean);
  return [
    { title: "Parent", items: parentZone ? [relatedItemForZone(parentZone, "Zone")] : [] },
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
    return `<button class="geometry-related-row" type="button" data-geometry-kind="${escapeHTML(item.kind)}" data-geometry-id="${escapeHTML(item.id)}">${content}</button>`;
  }
  return `<div class="geometry-related-row geometry-related-static">${content}</div>`;
}

function bindGeometryDetailControls() {
  elements.geometryDetails.querySelectorAll(".geometry-related-row[data-geometry-id]").forEach((button) => {
    button.addEventListener("click", () => selectGeometry(button.dataset.geometryKind, button.dataset.geometryId));
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

function zoneByName(geometry, zoneName) {
  const key = normalizeGeometryName(zoneName);
  return key ? (geometry?.zones || []).find((zone) => normalizeGeometryName(zone.name) === key) || null : null;
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
    const selected = object.userData.geometryId === state.selectedGeometryId && object.userData.geometryKind === state.selectedGeometryKind;
    object.material.color.setHex(selected ? selectedColor : object.userData.baseColor);
    object.material.opacity = selected ? 0.95 : object.userData.baseOpacity;
  });
  rendererState.renderer.render(rendererState.scene, rendererState.camera);
}

function highlightSelectedPlan() {
  elements.geometryPlan.querySelectorAll("[data-geometry-id]").forEach((shape) => {
    shape.classList.toggle(
      "selected",
      shape.dataset.geometryId === state.selectedGeometryId && shape.dataset.geometryKind === state.selectedGeometryKind,
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
