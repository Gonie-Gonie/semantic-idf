import * as THREE from "../vendor/three.module.js";
import { elements, escapeHTML, state } from "./state.js";

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
  elements.geometryStats.textContent = `${geometry.zoneCount || 0} zones, ${geometry.surfaceCount || 0} surfaces, ${geometry.windowCount || 0} windows`;
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
  elements.geometryStats.textContent = "0 zones, 0 surfaces, 0 windows";
  elements.geometryStorySelect.innerHTML = "";
  elements.geometryCanvasHost.innerHTML = `<div class="empty">No geometry yet</div>`;
  elements.geometryPlan.innerHTML = "";
  elements.geometryDetails.innerHTML = `<div class="empty">Select a zone, wall, or window</div>`;
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
      ? `<option value="all" ${state.selectedGeometryStory === "all" ? "selected" : ""}>All levels</option>`
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

  if (elements.geometryShowZones.checked) {
    (geometry.surfaces || [])
      .filter((surface) => matchesSelectedStory(surface) && surface.surfaceType?.toLowerCase() === "floor")
      .forEach((surface) => addSurfaceMesh(group, surface, "zone", zoneIdForName(geometry, surface.zoneName), center));
  }
  if (elements.geometryShowWalls.checked) {
    (geometry.surfaces || [])
      .filter((surface) => matchesSelectedStory(surface) && surface.surfaceType?.toLowerCase() !== "floor")
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
  };
  installCanvasInteractions();
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
  const geometry = polygonGeometry(surface.vertices, center, 0);
  if (!geometry) {
    return;
  }
  const isZone = kind === "zone";
  const isRoof = /roof|ceiling/i.test(surface.surfaceType || "");
  const material = new THREE.MeshStandardMaterial({
    color: isZone
      ? geometryColor("zone", 0xb8d7b0)
      : isRoof
        ? geometryColor("roof", 0xb8b0a1)
        : geometryColor("wall", 0x7b9cbc),
    roughness: 0.72,
    metalness: 0,
    transparent: true,
    opacity: isZone ? 0.5 : 0.72,
    side: THREE.DoubleSide,
  });
  const mesh = new THREE.Mesh(geometry, material);
  mesh.userData = { geometryKind: kind, geometryId: id, baseColor: material.color.getHex(), baseOpacity: material.opacity };
  group.add(mesh);
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
  const storyIndex = state.selectedGeometryStory === "all" ? firstStoryIndex(geometry) : state.selectedGeometryStory;
  const surfaces = (geometry.surfaces || []).filter((surface) => surface.storyIndex === storyIndex);
  const windows = (geometry.windows || []).filter((windowItem) => windowItem.storyIndex === storyIndex);
  const bounds = geometry.bounds || {};
  if (!bounds.ok || (!surfaces.length && !windows.length)) {
    elements.geometryPlan.innerHTML = `<text x="24" y="42" fill="#60707c" font-size="14">No floor plan geometry</text>`;
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
        .filter((surface) => surface.surfaceType?.toLowerCase() !== "floor")
        .map((surface) => `<polyline class="plan-wall" data-geometry-kind="surface" data-geometry-id="${escapeHTML(surface.id)}" points="${surface.vertices.map(project).join(" ")} ${project(surface.vertices[0])}"></polyline>`)
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

function renderGeometryDetails(geometry = state.report?.geometry) {
  const entity = selectedGeometryEntity(geometry);
  if (!entity) {
    elements.geometryDetails.innerHTML = `<div class="empty">Select a zone, wall, or window</div>`;
    return;
  }
  const relatedGroups = geometryRelatedGroups(geometry, entity);
  elements.geometryDetails.innerHTML = `
    <div class="geometry-detail-head">
      <div>
        <h3>${escapeHTML(entity.title)}</h3>
        <span>${escapeHTML(entity.subtitle)}</span>
      </div>
      <span class="geometry-sync-note">${state.geometrySyncLocate ? "Sync locate on" : "Sync locate off"}</span>
    </div>
    <div class="geometry-detail-grid">
      <section>
        <h4>Metrics</h4>
        ${renderMetricList(entity.metrics)}
      </section>
      <section>
        <h4>Related Objects</h4>
        ${renderRelatedGroups(relatedGroups)}
      </section>
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
          adjacentSurface ? relatedItemForSurface(adjacentSurface, "Adjacent surface") : referencedBoundaryItem(surface),
        ];
      })
      .filter(Boolean),
  );
  return [
    { title: "Boundary Surfaces", items: surfaces.map((surface) => relatedItemForSurface(surface, surface.surfaceType || "Surface")) },
    { title: "Openings", items: windows.map((windowItem) => relatedItemForWindow(windowItem, windowItem.surfaceType || "Window")) },
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
    adjacentSurface ? relatedItemForSurface(adjacentSurface, "Adjacent surface") : referencedBoundaryItem(surface),
  ].filter(Boolean);
  return [
    { title: "Parent", items: parentZone ? [relatedItemForZone(parentZone, "Zone")] : [] },
    { title: "Openings", items: windows.map((windowItem) => relatedItemForWindow(windowItem, windowItem.surfaceType || "Window")) },
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
    { title: "Parent", items: [parentZone && relatedItemForZone(parentZone, "Zone"), parentSurface && relatedItemForSurface(parentSurface, "Base surface")].filter(Boolean) },
    { title: "Sibling Openings", items: siblingWindows.map((item) => relatedItemForWindow(item, item.surfaceType || "Window")) },
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

function renderRelatedItem(item) {
  const content = `
    <span class="geometry-related-main">
      <strong>${escapeHTML(item.title)}</strong>
      <span>${escapeHTML(item.subtitle)}</span>
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

function relatedItemForSurface(surface, role) {
  return {
    kind: "surface",
    id: surface.id,
    role,
    title: surface.name || surface.type,
    subtitle: `${surface.surfaceType || surface.type}${surface.zoneName ? ` / ${surface.zoneName}` : ""}`,
  };
}

function relatedItemForWindow(windowItem, role) {
  return {
    kind: "window",
    id: windowItem.id,
    role,
    title: windowItem.name || windowItem.type,
    subtitle: windowItem.baseSurfaceName ? `On ${windowItem.baseSurfaceName}` : windowItem.type,
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
  return key ? (geometry.zones || []).find((zone) => normalizeGeometryName(zone.name) === key) || null : null;
}

function surfaceByID(geometry, id) {
  return (geometry.surfaces || []).find((surface) => surface.id === id) || null;
}

function surfaceByName(geometry, name) {
  const key = normalizeGeometryName(name);
  return key ? (geometry.surfaces || []).find((surface) => normalizeGeometryName(surface.name) === key) || null : null;
}

function windowByID(geometry, id) {
  return (geometry.windows || []).find((windowItem) => windowItem.id === id) || null;
}

function windowsForSurface(geometry, surface) {
  const surfaceName = normalizeGeometryName(surface?.name);
  return (geometry.windows || []).filter(
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
