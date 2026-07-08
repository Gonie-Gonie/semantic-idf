import { backend, elements, escapeHTML, setStatus, state } from "../state.js";
import { t } from "../i18n.js";

const HVAC_GRAPH_EXPORT_SCHEMA = "semantic-idf.hvac.graph.v1";
let hvacComponentBaseCountCache = { serviceModel: null, counts: new Map() };
let hvacDebugRuleGraphRequestKey = "";
let hvacDebugRuleGraphEmptyKey = "";

export function initializeHVACControls() {
  state.hvacInspectorCollapsed = readHVACInspectorCollapsed();
  syncHVACInspectorDrawer();
  elements.hvacFilter?.addEventListener("input", () => renderHVAC());
  elements.hvacSummary?.addEventListener("click", handleHVACNavigationClick);
  elements.hvacSummary?.addEventListener("toggle", handleHVACNavigationToggle, true);
  elements.hvacInspectorToggle?.addEventListener("click", () => {
    state.hvacInspectorCollapsed = !state.hvacInspectorCollapsed;
    writeHVACInspectorCollapsed(state.hvacInspectorCollapsed);
    syncHVACInspectorDrawer();
  });
  document.addEventListener("click", handleHVACOutsideClick);
  document.addEventListener("keydown", handleHVACEscapeKey);
  elements.hvacGraph?.addEventListener("click", (event) => {
    if (event.target.closest("[data-jump-object-index]")) {
      return;
    }
    const resultTab = event.target.closest("[data-result-tab]");
    if (resultTab) {
      openHVACResultTab(resultTab.dataset.resultTab || state.activeResultTab);
      return;
    }
    const debugExport = event.target.closest("[data-hvac-debug-export]");
    if (debugExport) {
      exportHVACDebugGraph(debugExport.dataset.hvacDebugExport || "rule");
      return;
    }
    const scaleButton = event.target.closest("[data-hvac-graph-scale]");
    if (scaleButton) {
      state.hvacGraphScale = hvacGraphScaleMode(scaleButton.dataset.hvacGraphScale || "fit");
      renderHVAC();
      return;
    }
    const navAction = event.target.closest("[data-hvac-nav-action]");
    if (navAction) {
      handleHVACNavigationAction(navAction.dataset.hvacNavAction || "");
      return;
    }
    const scopeButton = event.target.closest("[data-hvac-graph-scope]");
    if (scopeButton) {
      state.activeHVACGraphScope = hvacGraphScopeMode(scopeButton.dataset.hvacGraphScope || "focused");
      renderHVAC();
      return;
    }
    const quickFilter = event.target.closest("[data-hvac-filter-kind][data-hvac-filter-value]");
    if (quickFilter) {
      setHVACQuickFilter(quickFilter.dataset.hvacFilterKind || "", quickFilter.dataset.hvacFilterValue || "all");
      renderHVAC();
      return;
    }
    const viewButton = event.target.closest("[data-hvac-open-view]");
    if (viewButton) {
      navigateHVAC({ kind: "", id: "", view: viewButton.dataset.hvacOpenView || "services", graphKey: "" }, { pushHistory: true });
      return;
    }
    const loopJump = event.target.closest("[data-hvac-jump-loop-name]");
    if (loopJump) {
      jumpToHVACLoopByName(loopJump.dataset.hvacJumpLoopName || "", loopJump.dataset.hvacJumpGraphKey || "");
      return;
    }
    const editButton = event.target.closest("[data-hvac-edit-key]");
    if (editButton) {
      openHVACApplyDialog(editButton.dataset.hvacEditKey || "");
      return;
    }
    const entityTarget = event.target.closest("[data-hvac-entity-kind][data-hvac-entity-id]");
    if (entityTarget) {
      navigateHVAC(
        {
          kind: entityTarget.dataset.hvacEntityKind || "",
          id: entityTarget.dataset.hvacEntityId || "",
          label: entityTarget.dataset.hvacEntityLabel || "",
          view: entityTarget.dataset.hvacEntityView || "",
          context: {
            pathId: entityTarget.dataset.hvacPathId || "",
            zoneId: entityTarget.dataset.hvacZoneId || "",
            loopId: entityTarget.dataset.hvacLoopEntityId || "",
            componentId: entityTarget.dataset.hvacComponentId || "",
            couplingId: entityTarget.dataset.hvacCouplingId || "",
          },
          graphKey: entityTarget.dataset.hvacGraphKey || "",
        },
        { pushHistory: true },
      );
      return;
    }
    const graphTarget = event.target.closest("[data-hvac-graph-key]");
    if (graphTarget) {
      selectHVACGraphKey(graphTarget.dataset.hvacGraphKey || "");
      return;
    }
    const nodeButton = event.target.closest("[data-hvac-node]");
    if (!nodeButton) {
      return;
    }
    navigateHVAC(hvacTargetForNode(nodeButton.dataset.hvacNode || ""), { pushHistory: true });
  });
  elements.hvacGraph?.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      event.preventDefault();
      clearHVACGraphSelection();
      return;
    }
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    if (event.target.closest("[data-jump-object-index]")) {
      return;
    }
    const graphTarget = event.target.closest("[data-hvac-graph-key]");
    if (!graphTarget) {
      return;
    }
    event.preventDefault();
    selectHVACGraphKey(graphTarget.dataset.hvacGraphKey || "");
  });
  elements.hvacInspector?.addEventListener("click", (event) => {
    const resultTab = event.target.closest("[data-result-tab]");
    if (resultTab) {
      openHVACResultTab(resultTab.dataset.resultTab || state.activeResultTab);
      return;
    }
    const loopJump = event.target.closest("[data-hvac-jump-loop-name]");
    if (loopJump) {
      jumpToHVACLoopByName(loopJump.dataset.hvacJumpLoopName || "", loopJump.dataset.hvacJumpGraphKey || "");
      return;
    }
    const entityTarget = event.target.closest("[data-hvac-entity-kind][data-hvac-entity-id]");
    if (entityTarget) {
      navigateHVAC(
        {
          kind: entityTarget.dataset.hvacEntityKind || "",
          id: entityTarget.dataset.hvacEntityId || "",
          label: entityTarget.dataset.hvacEntityLabel || "",
          view: entityTarget.dataset.hvacEntityView || "",
          context: {
            pathId: entityTarget.dataset.hvacPathId || "",
            zoneId: entityTarget.dataset.hvacZoneId || "",
            loopId: entityTarget.dataset.hvacLoopEntityId || "",
            componentId: entityTarget.dataset.hvacComponentId || "",
            couplingId: entityTarget.dataset.hvacCouplingId || "",
          },
          graphKey: entityTarget.dataset.hvacGraphKey || "",
        },
        { pushHistory: true },
      );
      return;
    }
    const outputButton = event.target.closest("[data-hvac-output-variable]");
    if (outputButton) {
      openHVACOutputDialog({
        keyValue: outputButton.dataset.hvacOutputKey || state.activeHVACNodeName || "",
        variableName: outputButton.dataset.hvacOutputVariable || "",
      });
      return;
    }
    const nodeButton = event.target.closest("[data-hvac-node]");
    if (!nodeButton) {
      return;
    }
    navigateHVAC(hvacTargetForNode(nodeButton.dataset.hvacNode || ""), { pushHistory: true });
  });
  elements.hvacApplyClose?.addEventListener("click", closeHVACApplyDialog);
  elements.hvacPreviewApply?.addEventListener("click", previewHVACApply);
  elements.hvacApplyForm?.addEventListener("submit", applyHVACEdit);
  elements.hvacApplyBody?.addEventListener("input", () => {
    state.hvacApplyPreview = null;
    if (elements.hvacConfirmApply) {
      elements.hvacConfirmApply.disabled = true;
    }
    if (elements.hvacApplyStatus) {
      elements.hvacApplyStatus.textContent = t("status.runPreview");
    }
    const previewList = elements.hvacApplyBody.querySelector("#hvacApplyPreviewList");
    if (previewList) {
      previewList.innerHTML = `<div class="empty">${t("status.runPreview")}</div>`;
    }
  });
}

function selectHVACGraphKey(key) {
  navigateHVAC(hvacTargetForGraphKey(key), { pushHistory: true });
}

function navigateHVAC(target = {}, options = {}) {
  const previous = hvacNavigationSnapshot();
  const entity = resolveHVACNavigationEntity(target);
  const nextView = target.view || viewForHVACEntity(entity) || state.activeHVACView || "services";
  const nextContext = {
    ...emptyHVACContext(),
    ...(state.activeHVACContext || {}),
    ...(target.context || {}),
    previousView: state.activeHVACView || "",
  };
  if (entity.kind === "service_path") {
    nextContext.pathId = target.pathId || nextContext.pathId || pathIDFromNavigationEntityID(entity.id);
  }
  if (entity.kind === "zone" || entity.kind === "space") {
    nextContext.zoneId = entity.id;
  }
  if (entity.kind === "loop") {
    nextContext.loopId = entity.id;
  }
  if (entity.kind === "component") {
    nextContext.componentId = entity.id;
  }
  if (entity.kind === "coupling") {
    nextContext.couplingId = entity.id;
  }
  const graphKey = target.graphKey !== undefined ? target.graphKey : graphKeyForHVACEntity(entity, nextContext);
  const nextLoopID = target.loopID || loopIDForHVACTarget(target, entity) || state.activeHVACLoopId;

  state.activeHVACView = nextView;
  state.activeHVACLoopId = nextLoopID || "";
  state.activeHVACGraphKey = graphKey || "";
  state.activeHVACNodeName = entity.kind === "node" ? nodeNameFromNavigationID(entity.id, target.nodeName || "") : state.activeHVACGraphKey.startsWith("node:") ? state.activeHVACGraphKey.slice(5) : "";
  state.activeHVACEntity = entity;
  state.activeHVACContext = nextContext;
  state.activeHVACGraphScope = hvacGraphScopeMode(target.scope || state.activeHVACGraphScope || "focused");

  const next = hvacNavigationSnapshot();
  if (options.pushHistory !== false && !options.replace && !sameHVACNavigationSnapshot(previous, next)) {
    state.hvacNavigationStack.push(previous);
    state.hvacForwardStack = [];
  }
  notifyHVACSelectionChanged();
  renderHVAC();
}

export function backHVAC() {
  const previous = state.hvacNavigationStack.pop();
  if (!previous) {
    return false;
  }
  state.hvacForwardStack.push(hvacNavigationSnapshot());
  restoreHVACNavigationSnapshot(previous);
  notifyHVACSelectionChanged();
  renderHVAC();
  return true;
}

export function forwardHVAC() {
  const next = state.hvacForwardStack.pop();
  if (!next) {
    return false;
  }
  state.hvacNavigationStack.push(hvacNavigationSnapshot());
  restoreHVACNavigationSnapshot(next);
  notifyHVACSelectionChanged();
  renderHVAC();
  return true;
}

function clearHVACFocus() {
  const previous = hvacNavigationSnapshot();
  state.activeHVACEntity = emptyHVACEntity();
  state.activeHVACContext = emptyHVACContext();
  state.activeHVACGraphKey = "";
  state.activeHVACNodeName = "";
  state.activeHVACGraphScope = "focused";
  const next = hvacNavigationSnapshot();
  if (!sameHVACNavigationSnapshot(previous, next)) {
    state.hvacNavigationStack.push(previous);
    state.hvacForwardStack = [];
  }
  notifyHVACSelectionChanged();
  renderHVAC();
}

function handleHVACNavigationAction(action) {
  if (action === "back") {
    backHVAC();
  } else if (action === "forward") {
    forwardHVAC();
  } else if (action === "clear") {
    clearHVACFocus();
  }
}

function openHVACResultTab(tab) {
  const targetTab = tab || state.activeResultTab || "hvac";
  prepareHVACCrossTabContext(targetTab);
  [...(elements.resultTabButtons || [])].find((button) => button.dataset.resultTab === targetTab)?.click();
}

function prepareHVACCrossTabContext(tab) {
  const zoneName = currentHVACZoneName();
  if (tab === "profile" && zoneName) {
    state.activeProfileZoneName = zoneName;
    return;
  }
  if (tab === "geometry" && zoneName) {
    focusGeometryZoneFromHVAC(zoneName);
    return;
  }
  if (tab === "output") {
    prepareHVACOutputContext(zoneName);
    return;
  }
  if (tab === "simulation") {
    prepareHVACSimulationContext(zoneName);
  }
}

function currentHVACZoneName(hvac = state.report?.hvac) {
  const entity = findHVACNavigationEntity(state.activeHVACEntity?.id || "") || state.activeHVACEntity || {};
  if (entity.zoneName) {
    return entity.zoneName;
  }
  if (entity.kind === "zone") {
    return entity.label || entity.id.replace(/^zone:/, "");
  }
  const path = selectedHVACPath(hvac) || pathsForActiveHVACEntity(hvac)[0];
  return path?.zoneName || "";
}

function focusGeometryZoneFromHVAC(zoneName) {
  const zone = (state.report?.geometry?.zones || []).find((item) => normalizeGraphName(item.name) === normalizeGraphName(zoneName));
  if (!zone) {
    return;
  }
  state.selectedGeometryKind = "zone";
  state.selectedGeometryId = zone.id || "";
  if (zone.storyIndex !== undefined && zone.storyIndex !== null) {
    state.selectedGeometryStory = zone.storyIndex;
  }
}

function prepareHVACOutputContext(zoneName) {
  const query = hvacOutputFocusQuery(zoneName);
  if (query) {
    state.outputPendingFocusQuery = query;
  }
  const purpose = hvacOutputPurposeForActiveEntity();
  state.outputPurposeFilter = purpose;
  if (elements.outputPurposeFilter) {
    elements.outputPurposeFilter.value = purpose;
  }
}

function prepareHVACSimulationContext(zoneName) {
  const entityKind = state.activeHVACEntity?.kind || "";
  if (entityKind === "loop" || entityKind === "component" || state.activeHVACLoopId) {
    ensureHVACSimulationPurpose("hvac_loop_check");
    state.simulationActiveResultView = "hvac_loops";
  }
  if (zoneName && (entityKind === "zone" || entityKind === "space" || entityKind === "service_path")) {
    ensureHVACSimulationPurpose("zone_heat_flow");
    state.activeProfileZoneName = zoneName;
    state.simulationHeatFlowSelectedZone = zoneName;
    state.simulationActiveResultView = "zone_heat_flow";
    if (elements.simulationPurposeZoneMode) {
      elements.simulationPurposeZoneMode.value = "selected";
    }
    if (elements.simulationPurposeZoneNames) {
      elements.simulationPurposeZoneNames.value = zoneName;
    }
  }
}

function ensureHVACSimulationPurpose(purpose) {
  const values = new Set(state.simulationSelectedPurposes || []);
  values.add(purpose);
  state.simulationSelectedPurposes = [...values];
  elements.simulationPurposeInputs?.forEach((input) => {
    const selected = values.has(input.dataset.simulationPurpose);
    input.checked = selected;
    input.closest(".simulation-purpose-card")?.classList.toggle("selected", selected);
  });
}

function hvacOutputPurposeForActiveEntity() {
  if (state.activeHVACEntity?.kind === "loop" || state.activeHVACEntity?.kind === "component" || state.activeHVACNodeName) {
    return "hvac_loop_check";
  }
  if (state.activeHVACEntity?.kind === "zone" || state.activeHVACContext?.zoneId) {
    return "zone_heat_flow";
  }
  return "all";
}

function hvacOutputFocusQuery(zoneName = "") {
  const entity = findHVACNavigationEntity(state.activeHVACEntity?.id || "") || state.activeHVACEntity || {};
  if (state.activeHVACNodeName) {
    return state.activeHVACNodeName;
  }
  if (entity.objectName) {
    return entity.objectName;
  }
  if (entity.loopName) {
    return entity.loopName;
  }
  if (entity.kind === "zone" || entity.kind === "space") {
    return zoneName || entity.zoneName || entity.label || "";
  }
  const path = selectedHVACPath() || pathsForActiveHVACEntity()[0];
  return path?.delivery?.objectName || path?.delivery?.displayName || path?.plantLoop?.name || path?.airLoop?.name || zoneName || "";
}

function hvacNavigationSnapshot() {
  return {
    view: state.activeHVACView || "services",
    loopId: state.activeHVACLoopId || "",
    graphKey: state.activeHVACGraphKey || "",
    nodeName: state.activeHVACNodeName || "",
    entity: { ...(state.activeHVACEntity || emptyHVACEntity()) },
    context: { ...(state.activeHVACContext || emptyHVACContext()) },
    scope: state.activeHVACGraphScope || "focused",
  };
}

function restoreHVACNavigationSnapshot(snapshot = {}) {
  state.activeHVACView = snapshot.view || "services";
  state.activeHVACLoopId = snapshot.loopId || "";
  state.activeHVACGraphKey = snapshot.graphKey || "";
  state.activeHVACNodeName = snapshot.nodeName || "";
  state.activeHVACEntity = { ...emptyHVACEntity(), ...(snapshot.entity || {}) };
  state.activeHVACContext = { ...emptyHVACContext(), ...(snapshot.context || {}) };
  state.activeHVACGraphScope = hvacGraphScopeMode(snapshot.scope || "focused");
}

function sameHVACNavigationSnapshot(left = {}, right = {}) {
  return JSON.stringify(left) === JSON.stringify(right);
}

function emptyHVACEntity() {
  return { id: "", kind: "", label: "" };
}

function emptyHVACContext() {
  return { pathId: "", zoneId: "", loopId: "", componentId: "", couplingId: "", previousView: "" };
}

function resolveHVACNavigationEntity(target = {}) {
  const id = target.id || "";
  const indexed = id ? findHVACNavigationEntity(id) : null;
  return {
    id: id || indexed?.id || "",
    kind: target.kind || indexed?.kind || "",
    label: target.label || indexed?.label || "",
  };
}

function viewForHVACEntity(entity = {}) {
  switch (entity.kind) {
    case "service_path":
    case "zone":
    case "space":
    case "component":
      return "services";
    case "loop":
      return "loop";
    case "coupling":
    case "network":
      return "couplings";
    case "node":
      return state.activeHVACView || "services";
    default:
      return "";
  }
}

function graphKeyForHVACEntity(entity = {}, context = {}) {
  if (!entity.id) {
    return "";
  }
  if (entity.kind === "service_path") {
    return `service-path:${pathIDFromNavigationEntityID(entity.id)}`;
  }
  if (entity.kind === "zone" || entity.kind === "space") {
    return `subject:${entity.id}`;
  }
  if (entity.kind === "loop") {
    const loop = loopForNavigationEntityID(entity.id);
    return loop ? `loop:${loop.id}` : "";
  }
  if (entity.kind === "coupling") {
    return `coupling-node:any:${couplingIDFromNavigationEntityID(entity.id)}`;
  }
  if (entity.kind === "node") {
    return `node:${nodeNameFromNavigationID(entity.id, context.nodeName || "")}`;
  }
  return "";
}

function loopIDForHVACTarget(target = {}, entity = {}) {
  if (target.loopID) {
    return target.loopID;
  }
  if (entity.kind !== "loop") {
    return "";
  }
  return loopForNavigationEntityID(entity.id)?.id || "";
}

function hvacTargetForNode(nodeName = "") {
  return {
    kind: "node",
    id: nodeName ? `node:${normalizeGraphName(nodeName)}` : "",
    label: nodeName,
    view: state.activeHVACView,
    nodeName,
    graphKey: nodeName ? `node:${nodeName}` : "",
  };
}

function hvacTargetForGraphKey(key = "") {
  if (key.startsWith("service-path:")) {
    const pathId = key.slice("service-path:".length);
    return { kind: "service_path", id: navigationPathEntityID(pathId), context: { pathId }, graphKey: key, view: "services" };
  }
  if (key.startsWith("subject:")) {
    const subjectID = key.slice("subject:".length);
    const kind = subjectID.startsWith("space:") ? "space" : "zone";
    return { kind, id: subjectID, context: { zoneId: subjectID }, graphKey: key, view: "services" };
  }
  if (key.startsWith("coupling-node:any:")) {
    const couplingID = key.slice("coupling-node:any:".length);
    return { kind: "coupling", id: navigationCouplingEntityID(couplingID), context: { couplingId: navigationCouplingEntityID(couplingID) }, graphKey: key, view: "couplings" };
  }
  if (key.startsWith("node:")) {
    return hvacTargetForNode(key.slice("node:".length));
  }
  if (key.startsWith("loop:")) {
    const loopID = key.slice("loop:".length);
    const loop = (state.report?.hvac?.loops || []).find((item) => item.id === loopID);
    const entityID = loop ? navigationLoopEntityID(loop.type, loop.name) : "";
    return { kind: "loop", id: entityID, loopID, graphKey: key, view: "loop" };
  }
  if (key.startsWith("service-node:")) {
    const target = hvacTargetForServiceNodeGraphKey(key);
    if (target.id || target.kind) {
      return target;
    }
  }
  if (key.startsWith("service-link:") || key.startsWith("support-link:")) {
    const path = servicePathsForHVAC().find((item) => servicePathLinkKeys(item, new Map((hvacServiceModel().couplings || []).map((coupling) => [coupling.id, coupling]))).has(key));
    if (path) {
      return { kind: "service_path", id: navigationPathEntityID(path.id), context: { pathId: path.id }, graphKey: key, view: "services" };
    }
  }
  if (key.startsWith("component:")) {
    return { kind: "component", id: key, graphKey: key, view: state.activeHVACView || "services" };
  }
  return { kind: "", id: "", graphKey: key, view: state.activeHVACView || "services" };
}

function hvacTargetForServiceNodeGraphKey(key = "") {
  for (const path of servicePathsForHVAC()) {
    const spec = servicePathNodeSpecs(path).find((item) => serviceGraphNodeKey(path, item) === key);
    if (!spec) {
      continue;
    }
    const context = { pathId: path.id };
    if (spec.role === "zone") {
      const subjectID = servedSubjectKey(path.servedSubject || path);
      return { kind: subjectID.startsWith("space:") ? "space" : "zone", id: subjectID, label: spec.label, context: { ...context, zoneId: subjectID }, graphKey: key, view: "services" };
    }
    if (spec.role === "plant_loop" || spec.role === "air_loop") {
      const ref = spec.ref || {};
      const loop = loopForNavigationEntityID(navigationLoopEntityID(ref.type, ref.name));
      return { kind: "loop", id: navigationLoopEntityID(ref.type, ref.name), label: ref.name || spec.label, context: { ...context, loopId: navigationLoopEntityID(ref.type, ref.name) }, loopID: loop?.id || "", graphKey: key, view: "loop" };
    }
    if (spec.role === "delivery" || spec.role === "conditioning") {
      const componentID = navigationComponentEntityID(spec.ref || {});
      return { kind: "component", id: componentID, label: spec.label, context: { ...context, componentId: componentID }, graphKey: key, view: "services" };
    }
    if (spec.role === "source" || spec.role === "refrigerant_system") {
      const ref = spec.ref || {};
      const systemID = `system:${normalizeGraphName(ref.type || ref.objectType || "system")}:${normalizeGraphName(ref.name || ref.objectName || ref.displayName || "")}`;
      return { kind: "system", id: systemID, label: spec.label, context, graphKey: key, view: "services" };
    }
  }
  return {};
}

function findHVACNavigationEntity(id) {
  return (hvacNavigationIndex().entities || []).find((entity) => entity.id === id) || null;
}

function hvacNavigationIndex(hvac = state.report?.hvac) {
  return hvacServiceModel(hvac).navigation || { entities: [], links: [] };
}

function navigationPathEntityID(pathID = "") {
  return pathID.startsWith("path:") ? pathID : `path:${pathID}`;
}

function pathIDFromNavigationEntityID(id = "") {
  return id.startsWith("path:") ? id.slice("path:".length) : id;
}

function navigationLoopEntityID(loopType = "", loopName = "") {
  return `loop:${loopRefGraphKey(loopType, loopName)}`;
}

function loopForNavigationEntityID(id = "") {
  const raw = id.startsWith("loop:") ? id.slice("loop:".length) : id;
  return (state.report?.hvac?.loops || []).find((loop) => loopRefGraphKey(loop.type, loop.name) === raw || loop.id === raw) || null;
}

function navigationCouplingEntityID(couplingID = "") {
  return couplingID.startsWith("coupling:") ? couplingID : `coupling:${normalizeGraphName(couplingID)}`;
}

function couplingIDFromNavigationEntityID(id = "") {
  return id.startsWith("coupling:") ? id : id.replace(/^coupling:/, "");
}

function nodeNameFromNavigationID(id = "", fallback = "") {
  if (!id.startsWith("node:")) {
    return fallback;
  }
  const normalized = id.slice("node:".length);
  const usage = (state.report?.hvac?.nodeUsages || []).find((item) => normalizeGraphName(item.nodeName) === normalized);
  return usage?.nodeName || fallback || normalized;
}

function handleHVACEscapeKey(event) {
  if (event.key !== "Escape" || !hasActiveHVACGraphSelection()) {
    return;
  }
  if (event.target?.closest?.("input, textarea, select, [contenteditable='true']")) {
    return;
  }
  const hvacPaneIsVisible = state.activeResultTab === "hvac" || document.querySelector("#hvacPane")?.classList.contains("active");
  if (!hvacPaneIsVisible) {
    return;
  }
  event.preventDefault();
  clearHVACGraphSelection();
}

function clearHVACGraphSelection() {
  if (!hasActiveHVACGraphSelection()) {
    return;
  }
  state.activeHVACGraphKey = "";
  state.activeHVACNodeName = "";
  renderHVAC();
}

function hasActiveHVACGraphSelection() {
  return Boolean(state.activeHVACGraphKey || state.activeHVACNodeName);
}

function readHVACInspectorCollapsed() {
  try {
    return localStorage.getItem("idfAnalyzer.hvacInspectorCollapsed") === "1";
  } catch {
    return false;
  }
}

function writeHVACInspectorCollapsed(collapsed) {
  try {
    localStorage.setItem("idfAnalyzer.hvacInspectorCollapsed", collapsed ? "1" : "0");
  } catch {
    // localStorage can be unavailable in hardened webview settings.
  }
}

function syncHVACInspectorDrawer() {
  const collapsed = Boolean(state.hvacInspectorCollapsed);
  elements.hvacLayout?.classList.toggle("inspector-collapsed", collapsed);
  elements.hvacSide?.classList.toggle("collapsed", collapsed);
  if (elements.hvacInspectorToggle) {
    elements.hvacInspectorToggle.textContent = collapsed ? t("hvac.showInspector", {}, "Show inspector") : t("hvac.hideInspector", {}, "Hide inspector");
    elements.hvacInspectorToggle.setAttribute("aria-expanded", collapsed ? "false" : "true");
    elements.hvacInspectorToggle.classList.toggle("active", !collapsed);
  }
}

export function renderHVAC(hvac = state.report?.hvac) {
  if (!elements.hvacStats) {
    return;
  }
  syncHVACInspectorDrawer();
  if (!hvac) {
    renderEmptyHVAC();
    return;
  }

  const loops = hvac.loops || [];
  const previousLoopId = state.activeHVACLoopId;
  if (!state.activeHVACLoopId || !loops.some((loop) => loop.id === state.activeHVACLoopId)) {
    state.activeHVACLoopId = loops[0]?.id || "";
  }
  if (previousLoopId !== state.activeHVACLoopId) {
    notifyHVACSelectionChanged();
  }
  const selectedLoop = loops.find((loop) => loop.id === state.activeHVACLoopId) || null;
  const query = hvacQuery();

  elements.hvacStats.textContent = t("count.airPlantZone", {
    air: hvac.airLoopCount || 0,
    plant: hvac.plantLoopCount || 0,
    zones: hvac.zoneRelationCount || 0,
  });
  renderHVACSummary(hvac, selectedLoop);
  renderHVACWarnings(hvac, query);
  renderHVACInspector(hvac, selectedLoop);

  if (state.activeHVACView === "relation") {
    state.activeHVACView = "services";
  }
  if (state.activeHVACView === "services") {
    renderHVACServices(hvac, query);
  } else if (state.activeHVACView === "couplings") {
    renderHVACCouplings(hvac, query);
  } else if (state.activeHVACView === "diagnostics") {
    renderHVACDiagnostics(hvac, query);
  } else if (state.activeHVACView === "debug") {
    renderHVACDebug(hvac, query);
  } else {
    renderHVACLoopView(selectedLoop, query);
  }
}

function renderEmptyHVAC() {
  elements.hvacStats.textContent = t("count.airPlantZone", { air: 0, plant: 0, zones: 0 });
  elements.hvacSummary.innerHTML = `<div class="empty">${t("hvac.noHVACAnalysis")}</div>`;
  elements.hvacGraph.innerHTML = `<div class="empty">${t("hvac.noLoopGraph")}</div>`;
  elements.hvacInspectorStats.textContent = t("hvac.selectNode");
  elements.hvacInspector.innerHTML = `<div class="empty">${t("hvac.noData")}</div>`;
  elements.hvacWarningStats.textContent = t("count.warnings", { count: 0 });
  elements.hvacWarnings.innerHTML = `<div class="empty">${t("hvac.noWarnings")}</div>`;
}

function renderHVACSummary(hvac, selectedLoop) {
  const loops = hvac.loops || [];
  const groups = groupHVACLoopsByType(loops);
  const activeLoopKind = selectedLoop ? hvacLoopKind(selectedLoop) : "";
  const serviceModel = hvacServiceModel(hvac);
  const zoneServices = serviceModel.zoneServices || [];
  const servicePaths = servicePathsForHVAC(hvac);
  const couplings = serviceModel.couplings || [];
  const components = serviceModel.components || [];
  const debugEnabled = hvacDebugEnabled();
  const query = hvacQuery();
  elements.hvacSummary.innerHTML = `
    <div class="hvac-view-tabs" role="tablist" aria-label="${escapeHTML(t("tab.hvac", {}, "HVAC"))}">
      ${renderHVACViewTab("services", t("hvac.zoneServices", {}, "Zone Services"), servicePaths.length)}
      ${renderHVACViewTab("loop", t("hvac.loops", {}, "Loops"), loops.length)}
      ${renderHVACViewTab("couplings", t("hvac.couplings", {}, "Couplings"), couplings.length)}
      ${renderHVACViewTab("diagnostics", t("hvac.warnings"), hvac.warningCount || 0)}
      ${debugEnabled ? renderHVACViewTab("debug", t("hvac.debug", {}, "Debug"), (hvac.ruleGraph?.edges || []).length) : ""}
    </div>
    ${renderHVACBreadcrumbBar(hvac)}
    <div class="hvac-navigator">
      ${renderHVACCurrentFocusCard()}
      ${renderHVACEntitySearchResults(hvac, query)}
      ${renderHVACServicePicker(zoneServices, state.activeHVACView === "services")}
      ${renderHVACLoopPicker({
        kind: "air",
        label: t("hvac.airLoops"),
        help: t("hvac.airLoopHelp"),
        count: hvac.airLoopCount || groups.air.length,
        loops: groups.air,
        active: state.activeHVACView === "loop" && activeLoopKind === "air",
      })}
      ${renderHVACLoopPicker({
        kind: "plant",
        label: t("hvac.plantLoops"),
        help: t("hvac.plantLoopHelp"),
        count: hvac.plantLoopCount || groups.plant.length,
        loops: groups.plant,
        active: state.activeHVACView === "loop" && activeLoopKind === "plant",
      })}
      ${groups.other.length ? renderHVACLoopPicker({
        kind: "other",
        label: t("hvac.otherLoops"),
        help: t("hvac.otherLoopHelp"),
        count: groups.other.length,
        loops: groups.other,
        active: state.activeHVACView === "loop" && activeLoopKind === "other",
      }) : ""}
      ${renderHVACComponentPicker(components, state.activeHVACEntity?.kind === "component")}
      ${renderHVACCouplingPicker(couplings, state.activeHVACView === "couplings")}
    </div>`;
}

function renderHVACViewTab(view, label, count) {
  const active = state.activeHVACView === view || (view === "services" && state.activeHVACView === "relation");
  return `
    <button class="hvac-view-tab ${active ? "active" : ""}" type="button" data-hvac-open-view="${escapeHTML(view)}" aria-pressed="${active ? "true" : "false"}">
      <span>${escapeHTML(label)}</span>
      <b>${escapeHTML(count)}</b>
    </button>`;
}

function renderHVACCurrentFocusCard() {
  const entity = state.activeHVACEntity || {};
  const active = Boolean(entity.id);
  return `
    <div class="hvac-nav-card hvac-current-focus ${active ? "active" : ""}">
      <div class="hvac-nav-static">
        <span>
          <strong>${escapeHTML(t("hvac.currentFocus", {}, "Current focus"))}</strong>
          <em>${escapeHTML(active ? entity.label || entity.id : t("hvac.allZoneServices", {}, "All zone services"))}</em>
        </span>
        <button type="button" data-hvac-nav-action="clear" ${active ? "" : "disabled"}>${escapeHTML(t("hvac.clearFocus", {}, "Clear focus"))}</button>
      </div>
    </div>`;
}

function renderHVACEntitySearchResults(hvac, query) {
  if (!query) {
    return "";
  }
  const groups = groupNavigationEntitiesForSearch(hvac, query).slice(0, 5);
  if (!groups.length) {
    return "";
  }
  return `
    <details class="hvac-nav-card hvac-search-results active" open>
      <summary>
        <span>
          <strong>${escapeHTML(t("common.search", {}, "Search"))}</strong>
          <em>${escapeHTML(query)}</em>
        </span>
        <b>${escapeHTML(groups.reduce((sum, group) => sum + group.items.length, 0))}</b>
      </summary>
      <div class="hvac-nav-menu">
        ${groups.map((group) => `<div class="hvac-search-group"><strong>${escapeHTML(titleCaseToken(group.kind))}</strong>${group.items.map(renderHVACEntitySearchChoice).join("")}</div>`).join("")}
      </div>
    </details>`;
}

function renderHVACEntitySearchChoice(entity) {
  return `
    <button class="hvac-nav-choice" type="button"
      data-hvac-entity-kind="${escapeHTML(entity.kind || "")}"
      data-hvac-entity-id="${escapeHTML(entity.id || "")}"
      data-hvac-entity-label="${escapeHTML(entity.label || entity.id || "")}"
      data-hvac-entity-view="${escapeHTML(viewForHVACEntity(entity))}">
      <span>${escapeHTML(entity.label || entity.id)}</span>
      <small>${escapeHTML([entity.objectType, entity.loopType, (entity.relatedPathIds || []).length ? `${(entity.relatedPathIds || []).length} paths` : ""].filter(Boolean).join(" / "))}</small>
    </button>`;
}

function groupNavigationEntitiesForSearch(hvac, query) {
  const lowered = String(query || "").toLowerCase();
  const debug = hvacDebugEnabled();
  const entities = (hvacNavigationIndex(hvac).entities || [])
    .filter((entity) => debug || !["node"].includes(entity.kind))
    .filter((entity) =>
      [entity.id, entity.kind, entity.label, entity.objectType, entity.objectName, entity.loopName, entity.zoneName, entity.spaceName, ...(entity.serviceKinds || []), ...(entity.pathTypes || []), ...(entity.mediums || [])]
        .join(" ")
        .toLowerCase()
        .includes(lowered),
    );
  const groups = groupBy(entities, (entity) => entity.kind || "entity");
  return Object.entries(groups).map(([kind, items]) => ({ kind, items: items.slice(0, 8) }));
}

function renderHVACComponentPicker(components, active) {
  const visible = [...(components || [])]
    .sort((a, b) => (b.relatedPathIds || []).length - (a.relatedPathIds || []).length || componentRefLabel(a.component || {}).localeCompare(componentRefLabel(b.component || {})))
    .slice(0, 24);
  return `
    <details class="hvac-nav-card ${active ? "active" : ""}">
      <summary>
        <span>
          <strong>${escapeHTML(t("common.components", {}, "Components"))}</strong>
          <em>${escapeHTML(t("hvac.componentOccurrences", {}, "Occurrences and related paths"))}</em>
        </span>
        <b>${escapeHTML((components || []).length)}</b>
      </summary>
      <div class="hvac-nav-menu">
        ${visible.length ? visible.map(renderHVACComponentChoice).join("") : `<div class="empty compact">${escapeHTML(t("hvac.noComponents"))}</div>`}
      </div>
    </details>`;
}

function renderHVACComponentChoice(item) {
  const component = item.component || {};
  const entityID = navigationComponentEntityID(component);
  return `
    <button class="hvac-nav-choice ${state.activeHVACEntity?.id === entityID ? "active" : ""}" type="button"
      data-hvac-entity-kind="component"
      data-hvac-entity-id="${escapeHTML(entityID)}"
      data-hvac-entity-label="${escapeHTML(componentRefLabel(component))}"
      data-hvac-entity-view="services">
      <span>${escapeHTML(componentRefLabel(component))}</span>
      <small>${escapeHTML([component.objectType, (item.relatedPathIds || []).length ? `${(item.relatedPathIds || []).length} paths` : ""].filter(Boolean).join(" / "))}</small>
    </button>`;
}

function renderHVACCouplingPicker(couplings, active) {
  const physical = (couplings || []).filter((coupling) => isPhysicalServiceCoupling(coupling)).slice(0, 24);
  return `
    <details class="hvac-nav-card ${active ? "active" : ""}">
      <summary>
        <span>
          <strong>${escapeHTML(t("hvac.couplings", {}, "Couplings"))}</strong>
          <em>${escapeHTML(t("hvac.supportingAssets", {}, "Supporting assets"))}</em>
        </span>
        <b>${escapeHTML((couplings || []).length)}</b>
      </summary>
      <div class="hvac-nav-menu">
        ${physical.length ? physical.map(renderHVACCouplingChoice).join("") : `<div class="empty compact">${escapeHTML(t("hvac.noCouplings", {}, "No couplings"))}</div>`}
      </div>
    </details>`;
}

function renderHVACCouplingChoice(coupling) {
  const entityID = navigationCouplingEntityID(coupling.id);
  return `
    <button class="hvac-nav-choice ${state.activeHVACEntity?.id === entityID ? "active" : ""}" type="button"
      data-hvac-entity-kind="coupling"
      data-hvac-entity-id="${escapeHTML(entityID)}"
      data-hvac-entity-label="${escapeHTML(coupling.object?.displayName || coupling.object?.objectName || coupling.role || "")}"
      data-hvac-entity-view="couplings"
      data-hvac-coupling-id="${escapeHTML(entityID)}"
      data-hvac-graph-key="${escapeHTML(`coupling-node:any:${coupling.id}`)}">
      <span>${escapeHTML(coupling.object?.displayName || coupling.object?.objectName || coupling.role || "")}</span>
      <small>${escapeHTML([couplingRoleLabel(coupling), (coupling.connectedLoops || []).map((loop) => loop.name).join(", ")].filter(Boolean).join(" / "))}</small>
    </button>`;
}

function renderHVACBreadcrumbBar(hvac) {
  const segments = hvacBreadcrumbSegments(hvac);
  return `
    <div class="hvac-breadcrumb-bar">
      <div class="hvac-history-actions">
        <button type="button" data-hvac-nav-action="back" ${state.hvacNavigationStack.length ? "" : "disabled"}>${escapeHTML(t("common.back", {}, "Back"))}</button>
        <button type="button" data-hvac-nav-action="forward" ${state.hvacForwardStack.length ? "" : "disabled"}>${escapeHTML(t("common.forward", {}, "Forward"))}</button>
        <button type="button" data-hvac-nav-action="clear" ${hasHVACFocus() ? "" : "disabled"}>${escapeHTML(t("hvac.clearFocus", {}, "Clear focus"))}</button>
      </div>
      <nav class="hvac-breadcrumb" aria-label="${escapeHTML(t("common.navigation", {}, "Navigation"))}">
        ${
          segments.length
            ? segments.map(renderHVACBreadcrumbSegment).join("")
            : `<button type="button" data-hvac-open-view="services">${escapeHTML(t("hvac.allZoneServices", {}, "All zone services"))}</button>`
        }
      </nav>
    </div>`;
}

function renderHVACBreadcrumbSegment(segment, index) {
  const attrs = segment.kind && segment.id
    ? `data-hvac-entity-kind="${escapeHTML(segment.kind)}" data-hvac-entity-id="${escapeHTML(segment.id)}" data-hvac-entity-label="${escapeHTML(segment.label)}" data-hvac-entity-view="${escapeHTML(segment.view || "")}" data-hvac-path-id="${escapeHTML(segment.pathId || "")}" data-hvac-graph-key="${escapeHTML(segment.graphKey || "")}"`
    : `data-hvac-open-view="${escapeHTML(segment.view || "services")}"`;
  return `
    ${index ? `<span aria-hidden="true">/</span>` : ""}
    <button type="button" ${attrs}>${escapeHTML(segment.label)}</button>`;
}

function hvacBreadcrumbSegments(hvac) {
  const entity = state.activeHVACEntity || {};
  const path = selectedHVACPath(hvac);
  const segments = [{ label: t("hvac.zoneServices", {}, "Zone Services"), view: "services" }];
  if (path) {
    const subjectID = servedSubjectKey(path.servedSubject || path);
    segments.push({
      label: servedSubjectLabel(path.servedSubject || path),
      kind: subjectID.startsWith("space:") ? "space" : "zone",
      id: subjectID,
      view: "services",
      graphKey: `subject:${subjectID}`,
    });
    segments.push({
      label: serviceKindLabel(path.serviceKind),
      kind: "service_path",
      id: navigationPathEntityID(path.id),
      pathId: path.id,
      view: "services",
      graphKey: servicePathGraphKey(path),
    });
    for (const ref of [path.plantLoop, path.airLoop, path.refrigerantSystem, path.sourceSystem].filter(Boolean)) {
      const isLoop = Boolean(ref.type && ref.name && !ref.displayName && !ref.objectType);
      segments.push({
        label: ref.name || ref.displayName || ref.objectName || ref.type,
        kind: isLoop ? "loop" : "system",
        id: isLoop ? navigationLoopEntityID(ref.type, ref.name) : `system:${normalizeGraphName(ref.type || ref.objectType || "system")}:${normalizeGraphName(ref.name || ref.objectName || ref.displayName || "")}`,
        pathId: path.id,
        view: isLoop ? "loop" : "services",
      });
    }
    return segments;
  }
  if (entity.id && entity.kind) {
    segments.push({
      label: entity.label || entity.id,
      kind: entity.kind,
      id: entity.id,
      view: viewForHVACEntity(entity),
      graphKey: graphKeyForHVACEntity(entity, state.activeHVACContext || {}),
    });
  }
  return segments;
}

function hasHVACFocus() {
  return Boolean(state.activeHVACEntity?.id || state.activeHVACGraphKey || state.activeHVACNodeName);
}

function renderHVACServicePicker(zoneServices, active) {
  const selectedKey = state.activeHVACGraphKey?.startsWith("subject:") ? state.activeHVACGraphKey.slice("subject:".length) : "";
  return `
    <details class="hvac-nav-card ${active ? "active" : ""}" ${active ? "open" : ""}>
      <summary>
        <span>
          <strong>${escapeHTML(t("hvac.zoneServices", {}, "Zone Services"))}</strong>
          <em>${escapeHTML(t("hvac.zoneServiceHelp", {}, "Zone-centered heating, cooling, and ventilation paths"))}</em>
        </span>
        <b>${escapeHTML(zoneServices.length)}</b>
      </summary>
      <div class="hvac-nav-menu">
        <button class="hvac-nav-choice ${active && !selectedKey ? "active" : ""}" type="button" data-hvac-open-view="services">
          <span>${escapeHTML(t("hvac.allZoneServices", {}, "All zone services"))}</span>
          <small>${escapeHTML(t("hvac.servicePath", {}, "Service paths"))}</small>
        </button>
        ${
          zoneServices.length
            ? zoneServices.map((summary) => renderHVACServiceChoice(summary, active, selectedKey)).join("")
            : `<div class="empty compact">${escapeHTML(t("hvac.noZoneServices", {}, "No zone services"))}</div>`
        }
      </div>
    </details>`;
}

function renderHVACServiceChoice(summary, active, selectedKey) {
  const key = servedSubjectKey(summary.servedSubject || summary);
  const selected = active && selectedKey === key;
  const label = summary.spaceName ? `${summary.spaceName} / ${summary.zoneName || ""}` : summary.zoneName || summary.servedSubject?.name || t("common.blank");
  const services = [...new Set((summary.paths || []).map((path) => serviceKindLabel(path.serviceKind)).filter(Boolean))].join(", ");
  return `
    <button class="hvac-nav-choice ${selected ? "active" : ""}" type="button" data-hvac-service-subject-key="${escapeHTML(key)}" data-hvac-service-label="${escapeHTML(label)}">
      <span>${escapeHTML(label)}</span>
      <small>${escapeHTML(services || t("hvac.noServicePaths", {}, "No service paths"))}</small>
    </button>`;
}

function renderHVACLoopPicker({ kind, label, help, count, loops, active }) {
  return `
    <details class="hvac-nav-card ${active ? "active" : ""}" data-hvac-loop-kind="${escapeHTML(kind)}">
      <summary>
        <span>
          <strong>${escapeHTML(label)}</strong>
          <em>${escapeHTML(help)}</em>
        </span>
        <b>${escapeHTML(count)}</b>
      </summary>
      <div class="hvac-nav-menu">
        ${loops.length ? loops.map(renderHVACLoopChoice).join("") : `<div class="empty compact">${escapeHTML(t(kind === "air" ? "hvac.noAirLoops" : kind === "plant" ? "hvac.noPlantLoops" : "hvac.noLoops"))}</div>`}
      </div>
    </details>`;
}

function renderHVACLoopChoice(loop) {
  const selected = state.activeHVACView === "loop" && loop.id === state.activeHVACLoopId;
  return `
    <button class="hvac-nav-choice ${selected ? "active" : ""}" type="button" data-hvac-loop-id="${escapeHTML(loop.id)}">
      <span>${escapeHTML(loop.name || loop.type || t("hvac.unnamedLoop"))}</span>
      <small>${escapeHTML([loop.type, objectReferenceText(loop.objectIndex)].filter(Boolean).join(" "))}</small>
    </button>`;
}

function renderHVACDiagnosticsCard(count, active) {
  return `
    <button class="hvac-nav-card hvac-nav-action ${active ? "active" : ""}" type="button" data-hvac-open-view="diagnostics">
      <span>
        <strong>${escapeHTML(t("hvac.warnings"))}</strong>
        <em>${escapeHTML(t("hvac.diagnosticsHelp"))}</em>
      </span>
      <b>${escapeHTML(count)}</b>
    </button>`;
}

function handleHVACNavigationClick(event) {
  const navAction = event.target.closest("[data-hvac-nav-action]");
  if (navAction) {
    handleHVACNavigationAction(navAction.dataset.hvacNavAction || "");
    return;
  }
  const loopJump = event.target.closest("[data-hvac-jump-loop-name]");
  if (loopJump) {
    jumpToHVACLoopByName(loopJump.dataset.hvacJumpLoopName || "", loopJump.dataset.hvacJumpGraphKey || "");
    return;
  }
  const loopButton = event.target.closest("[data-hvac-loop-id]");
  if (loopButton) {
    const loopID = loopButton.dataset.hvacLoopId || "";
    const loop = (state.report?.hvac?.loops || []).find((item) => item.id === loopID);
    navigateHVAC(
      {
        kind: "loop",
        id: loop ? navigationLoopEntityID(loop.type, loop.name) : "",
        label: loop?.name || "",
        loopID,
        view: "loop",
        graphKey: loopID ? `loop:${loopID}` : "",
      },
      { pushHistory: true },
    );
    return;
  }
  const serviceButton = event.target.closest("[data-hvac-service-subject-key]");
  if (serviceButton) {
    const subjectID = serviceButton.dataset.hvacServiceSubjectKey || "";
    navigateHVAC(
      {
        kind: subjectID.startsWith("space:") ? "space" : "zone",
        id: subjectID,
        label: serviceButton.dataset.hvacServiceLabel || "",
        view: "services",
        context: { zoneId: subjectID },
        graphKey: subjectID ? `subject:${subjectID}` : "",
      },
      { pushHistory: true },
    );
    return;
  }
  const quickFilter = event.target.closest("[data-hvac-filter-kind][data-hvac-filter-value]");
  if (quickFilter) {
    setHVACQuickFilter(quickFilter.dataset.hvacFilterKind || "", quickFilter.dataset.hvacFilterValue || "all");
    renderHVAC();
    return;
  }
  const entityTarget = event.target.closest("[data-hvac-entity-kind][data-hvac-entity-id]");
  if (entityTarget) {
    navigateHVAC(
      {
        kind: entityTarget.dataset.hvacEntityKind || "",
        id: entityTarget.dataset.hvacEntityId || "",
        label: entityTarget.dataset.hvacEntityLabel || "",
        view: entityTarget.dataset.hvacEntityView || "",
        context: {
          pathId: entityTarget.dataset.hvacPathId || "",
          zoneId: entityTarget.dataset.hvacZoneId || "",
          loopId: entityTarget.dataset.hvacLoopEntityId || "",
          componentId: entityTarget.dataset.hvacComponentId || "",
          couplingId: entityTarget.dataset.hvacCouplingId || "",
        },
        graphKey: entityTarget.dataset.hvacGraphKey || "",
      },
      { pushHistory: true },
    );
    return;
  }
  const viewButton = event.target.closest("[data-hvac-open-view]");
  if (viewButton) {
    const view = viewButton.dataset.hvacOpenView || "services";
    if (view === "loop" && !state.activeHVACLoopId) {
      state.activeHVACLoopId = (state.report?.hvac?.loops || [])[0]?.id || "";
    }
    navigateHVAC({ kind: "", id: "", view, graphKey: "", context: emptyHVACContext() }, { pushHistory: true });
  }
}

function handleHVACNavigationToggle(event) {
  const openedMenu = event.target?.matches?.("details.hvac-nav-card[open]") ? event.target : null;
  if (!openedMenu || !elements.hvacSummary?.contains(openedMenu)) {
    return;
  }
  elements.hvacSummary.querySelectorAll("details.hvac-nav-card[open]").forEach((menu) => {
    if (menu !== openedMenu) {
      menu.open = false;
    }
  });
}

function handleHVACOutsideClick(event) {
  if (!elements.hvacSummary || elements.hvacSummary.contains(event.target)) {
    return;
  }
  elements.hvacSummary.querySelectorAll("details.hvac-nav-card[open]").forEach((menu) => {
    menu.open = false;
  });
}

function jumpToHVACLoopByName(loopName, graphKey = "") {
  const loop = findHVACLoopByName(loopName);
  if (!loop) {
    return;
  }
  navigateHVAC(
    {
      kind: "loop",
      id: navigationLoopEntityID(loop.type, loop.name),
      label: loop.name || "",
      loopID: loop.id,
      view: "loop",
      graphKey: graphKey || `loop:${loop.id}`,
    },
    { pushHistory: true },
  );
}

function notifyHVACSelectionChanged() {
  window.dispatchEvent(new CustomEvent("idfAnalyzer:hvacSelectionChanged"));
}

function findHVACLoopByName(loopName) {
  const wanted = normalizeGraphName(loopName);
  return (state.report?.hvac?.loops || []).find((loop) => normalizeGraphName(loop.name) === wanted) || null;
}

function groupHVACLoopsByType(loops) {
  return loops.reduce(
    (groups, loop) => {
      groups[hvacLoopKind(loop)].push(loop);
      return groups;
    },
    { air: [], plant: [], other: [] },
  );
}

function hvacLoopKind(loop) {
  const type = String(loop?.type || "").toLowerCase();
  if (type.includes("airloop")) {
    return "air";
  }
  if (type.includes("plantloop")) {
    return "plant";
  }
  return "other";
}

function hvacLoopChipClass(loopType = "") {
  const type = String(loopType || "").toLowerCase();
  if (type.includes("airloop")) {
    return "air";
  }
  if (type.includes("condenser")) {
    return "condenser";
  }
  if (type.includes("plant")) {
    return "plant";
  }
  return "other";
}

function renderHVACLoopView(loop, query) {
  if (!loop) {
    elements.hvacGraph.innerHTML = `<div class="empty">${t("hvac.noLoopsFound")}</div>`;
    return;
  }
  if (query && !loopMatchesQuery(loop, query)) {
    elements.hvacGraph.innerHTML = `<div class="empty">${t("hvac.selectedLoopMismatch")}</div>`;
    return;
  }
  const relatedPaths = servicePathsForLoop(state.report?.hvac, loop)
    .filter(servicePathMatchesQuickFilters)
    .filter((path) => servicePathMatchesQuery(path, query));
  const couplings = hvacServiceModel().couplings || [];
  const loopCouplings = supportingCouplingsForLoop(loop, couplings);
  elements.hvacGraph.innerHTML = `
    <div class="hvac-loop-title">
      <div class="hvac-loop-heading">
        <h3>${escapeHTML(loop.name || loop.type)}</h3>
        <span>${escapeHTML(loop.type)} ${renderObjectLink(loop.objectIndex, loop.type)}</span>
        ${renderLoopRelatedSystemChips(loop)}
      </div>
      <div class="hvac-loop-meta">
        <span>${escapeHTML(t("count.zones", { count: relatedZoneNamesForServicePaths(relatedPaths).length || (loop.relatedZones || []).length }))}</span>
        <span>${escapeHTML(t("hvac.servicePath", {}, "Service paths"))}: ${escapeHTML(relatedPaths.length)}</span>
        <span>${escapeHTML(t("hvac.supportingAssets", {}, "Supporting assets"))}: ${escapeHTML(loopCouplings.length)}</span>
        ${renderHVACGraphScaleControls()}
      </div>
    </div>
    ${renderHVACLoopServiceOverview(loop, relatedPaths, loopCouplings)}
    ${renderHVACLoopRelatedServicePaths(relatedPaths)}
    <details class="hvac-graph-detail hvac-physical-loop-detail">
      <summary>
        <span>${escapeHTML(t("hvac.physicalLoopDetail", {}, "Physical loop detail"))}</span>
        <strong>${escapeHTML((loop.supplySide?.branches || []).length + (loop.demandSide?.branches || []).length)}</strong>
      </summary>
      ${renderHVACLoopDiagram(loop)}
      ${renderHVACLoopGraphDetail(loop)}
    </details>
    ${renderHVACLoopSupportingAssets(loop, couplings)}
    ${renderCrossLoopRelations(loop)}`;
}

function renderHVACLoopServiceOverview(loop, relatedPaths, loopCouplings) {
  const media = [...new Set(relatedPaths.flatMap(pathMediumsForFrontend).filter(Boolean))];
  return `
    <section class="hvac-graph-detail hvac-loop-service-overview">
      <div class="hvac-detail-grid">
        <div><span>${escapeHTML(t("common.type"))}</span><strong>${escapeHTML(loop.type || "Loop")}</strong></div>
        <div><span>Medium</span><strong>${escapeHTML(media.map(serviceEdgeLabel).join(", ") || (loop.type || "N/A"))}</strong></div>
        <div><span>${escapeHTML(t("hvac.servicePath", {}, "Service paths"))}</span><strong>${escapeHTML(relatedPaths.length)}</strong></div>
        <div><span>${escapeHTML(t("hvac.relatedZones"))}</span><strong>${escapeHTML(relatedZoneNamesForServicePaths(relatedPaths).length || (loop.relatedZones || []).length)}</strong></div>
        <div><span>${escapeHTML(t("hvac.supportingAssets", {}, "Supporting assets"))}</span><strong>${escapeHTML(loopCouplings.length)}</strong></div>
      </div>
      <div class="hvac-loop-actions">
        <button type="button" data-hvac-open-view="services">${escapeHTML(t("hvac.showServicePaths", {}, "Show service paths"))}</button>
        <button type="button" data-result-tab="output">${escapeHTML(t("tab.output", {}, "Output"))}</button>
        <button type="button" data-result-tab="simulation">${escapeHTML(t("tab.simulation", {}, "Simulation"))}</button>
        <button type="button" data-hvac-nav-action="clear">${escapeHTML(t("hvac.clearFocus", {}, "Clear focus"))}</button>
        ${renderObjectLink(loop.objectIndex, loop.type)}
      </div>
    </section>`;
}

function renderHVACLoopRelatedServicePaths(paths = []) {
  return `
    <section class="hvac-graph-detail hvac-loop-related-paths">
      <div class="hvac-section-head">
        <h3>${escapeHTML(t("hvac.relatedServicePaths", {}, "Related service paths"))}</h3>
        <span>${escapeHTML(paths.length)}</span>
      </div>
      ${
        paths.length
          ? `<div class="hvac-service-path-table">
              <div class="hvac-service-path-row head">
                <span>Zone</span>
                <span>Service</span>
                <span>${escapeHTML(t("hvac.delivery", {}, "Delivery"))}</span>
                <span>Conditioning</span>
                <span>Source</span>
                <span>Path type</span>
                <span>Issues</span>
              </div>
              ${paths.map(renderHVACLoopServicePathRow).join("")}
            </div>`
          : `<div class="empty">${escapeHTML(t("hvac.noServicePaths", {}, "No service paths"))}</div>`
      }
    </section>`;
}

function renderHVACLoopServicePathRow(path) {
  const entityID = navigationPathEntityID(path.id);
  const conditioning = (path.conditioning || []).map((component) => componentRefLabel(component)).join(", ");
  const source = servicePathSourceText(path) || servicePathConnectedSystems(path).join(", ");
  return `
    <button class="hvac-service-path-row ${state.activeHVACContext?.pathId === path.id ? "selected" : ""}" type="button"
      data-hvac-entity-kind="service_path"
      data-hvac-entity-id="${escapeHTML(entityID)}"
      data-hvac-entity-label="${escapeHTML(servedSubjectLabel(path.servedSubject || path))}"
      data-hvac-entity-view="services"
      data-hvac-path-id="${escapeHTML(path.id)}"
      data-hvac-graph-key="${escapeHTML(servicePathGraphKey(path))}">
      <span>${escapeHTML(servedSubjectLabel(path.servedSubject || path))}</span>
      <span>${escapeHTML(serviceKindLabel(path.serviceKind))}</span>
      <span>${escapeHTML(deliveryLabel(path))}</span>
      <span>${escapeHTML(conditioning || "N/A")}</span>
      <span>${escapeHTML(source || "N/A")}</span>
      <span>${escapeHTML(pathTypeLabel(path.pathType))}</span>
      <span>${escapeHTML((path.issues || []).length || "")}</span>
    </button>`;
}

function servicePathsForLoop(hvac, loop = {}) {
  const entityID = navigationLoopEntityID(loop.type, loop.name);
  const navigation = hvacNavigationIndex(hvac);
  const ids = navigation.byLoop?.[entityID] || fallbackPathIDsForLoop(hvac, entityID);
  const wanted = new Set(ids);
  return servicePathsForHVAC(hvac).filter((path) => wanted.has(path.id));
}

function relatedZoneNamesForServicePaths(paths = []) {
  return [...new Set(paths.map((path) => path.zoneName).filter(Boolean))];
}

function pathMediumsForFrontend(path = {}) {
  return [
    path.airLoop ? "air" : "",
    path.plantLoop ? (path.serviceKind === "heating" || path.serviceKind === "radiant_heating" ? "hot_water" : "chilled_water") : "",
    path.condenserLoop ? "condenser_water" : "",
    path.refrigerantSystem ? "refrigerant" : "",
    ...(path.sourceSystem?.mediums || []),
    ...(path.delivery?.mediums || []),
  ].filter(Boolean);
}

function renderLoopRelatedSystemChips(loop) {
  const relations = loop.relatedLoops || [];
  if (!relations.length) {
    return "";
  }
  return `
    <div class="hvac-related-system-chips" aria-label="${escapeHTML(t("hvac.connectedSystems", {}, "Connected systems"))}">
      ${relations
        .map((relation) => {
          const graphKey = componentKeyForCrossLoopRelation(loop, relation);
          const chipClass = hvacLoopChipClass(relation.loopType);
          const label = [relation.loopName, relation.componentName ? `via ${relation.componentName}` : ""].filter(Boolean).join(" ");
          return `
            <button class="${escapeHTML(chipClass)}" type="button"
              data-hvac-jump-loop-name="${escapeHTML(relation.loopName)}"
              data-hvac-jump-graph-key="${escapeHTML(graphKey)}"
              title="${escapeHTML(`${relation.loopType} ${label}`)}">
              ${escapeHTML(label || relation.loopType || t("hvac.viewLoop"))}
            </button>`;
        })
        .join("")}
    </div>`;
}

function renderHVACLoopSupportingAssets(loop, couplings = []) {
  const loopCouplings = supportingCouplingsForLoop(loop, couplings);
  if (!loopCouplings.length) {
    return "";
  }
  return `
    <section class="hvac-graph-detail hvac-supporting-assets">
      <div class="hvac-section-head">
        <h3>${escapeHTML(t("hvac.supportingAssets", {}, "Supporting assets"))}</h3>
        <span>${escapeHTML(loopCouplings.length)}</span>
      </div>
      <div class="hvac-support-grid">
        ${loopCouplings.map((item) => renderHVACLoopSupportingAssetCard(item.coupling, item.viaLoop)).join("")}
      </div>
    </section>`;
}

function supportingCouplingsForLoop(loop = {}, couplings = []) {
  const loopKeys = new Set([loopRefGraphKey(loop.type, loop.name)]);
  for (const relation of loop.relatedLoops || []) {
    loopKeys.add(loopRefGraphKey(relation.loopType, relation.loopName));
  }
  const seen = new Set();
  const out = [];
  for (const coupling of couplings || []) {
    if (!isPhysicalServiceCoupling(coupling)) {
      continue;
    }
    const match = (coupling.connectedLoops || []).find((connectedLoop) => loopKeys.has(loopRefGraphKey(connectedLoop.type, connectedLoop.name)));
    if (!match || seen.has(coupling.id)) {
      continue;
    }
    seen.add(coupling.id);
    out.push({ coupling, viaLoop: normalizeGraphName(match.name) === normalizeGraphName(loop.name) ? "" : match.name });
  }
  return out.sort((left, right) => {
    const leftType = left.coupling.couplingType || "";
    const rightType = right.coupling.couplingType || "";
    if (leftType !== rightType) {
      return leftType.localeCompare(rightType);
    }
    return String(left.coupling.object?.objectName || "").localeCompare(String(right.coupling.object?.objectName || ""));
  });
}

function loopRefGraphKey(loopType = "", loopName = "") {
  return `${normalizeGraphName(loopType)}:${normalizeGraphName(loopName)}`;
}

function renderHVACLoopSupportingAssetCard(coupling = {}, viaLoop = "") {
  const key = `coupling-node:any:${coupling.id}`;
  const loops = (coupling.connectedLoops || []).map((item) => item.name).filter(Boolean);
  const media = (coupling.mediums || []).map(serviceEdgeLabel);
  const meta = [couplingRoleLabel(coupling), viaLoop ? `${t("hvac.connectedSystems", {}, "Connected systems")}: ${viaLoop}` : ""].filter(Boolean).join(" / ");
  return `
    <article class="hvac-support-card ${escapeHTML(coupling.couplingType || "")} ${state.activeHVACGraphKey === key ? "selected" : ""}" data-hvac-graph-key="${escapeHTML(key)}" tabindex="0">
      <div class="hvac-support-card-main">
        <svg class="hvac-support-icon" viewBox="0 0 30 30" aria-hidden="true">${renderHVACNodeIcon(iconKindForCoupling(coupling), 15, 15)}</svg>
        <span>
          <strong>${escapeHTML(coupling.object?.displayName || coupling.object?.objectName || coupling.role || "")}</strong>
          <small>${escapeHTML(meta || coupling.couplingType || "")}</small>
        </span>
      </div>
      <div class="hvac-support-badges">
        ${loops.map((name) => `<span>${escapeHTML(name)}</span>`).join("")}
        ${media.map((name) => `<span>${escapeHTML(name)}</span>`).join("")}
      </div>
    </article>`;
}

function hvacGraphScaleMode(value = state.hvacGraphScale) {
  return ["fit", "actual", "compact"].includes(value) ? value : "fit";
}

function hvacGraphScopeMode(value = state.activeHVACGraphScope) {
  return ["focused", "all", "path_only", "neighbors"].includes(value) ? value : "focused";
}

function hvacGraphScaleClass() {
  return `scale-${hvacGraphScaleMode()}`;
}

function renderHVACGraphScaleControls() {
  const options = [
    ["fit", t("hvac.graphScaleFit", {}, "Fit")],
    ["actual", t("hvac.graphScaleActual", {}, "100%")],
    ["compact", t("hvac.graphScaleCompact", {}, "Compact")],
  ];
  return `
    <div class="hvac-graph-scale" role="group" aria-label="${escapeHTML(t("hvac.graphScale", {}, "HVAC graph scale"))}">
      ${options
        .map(([value, label]) => {
          const active = hvacGraphScaleMode() === value;
          return `
            <button class="${active ? "active" : ""}" type="button" data-hvac-graph-scale="${escapeHTML(value)}" aria-pressed="${active ? "true" : "false"}">
              ${escapeHTML(label)}
            </button>`;
        })
        .join("")}
    </div>`;
}

function setHVACQuickFilter(kind, value) {
  if (kind === "service") {
    state.hvacServiceKindFilter = value || "all";
  } else if (kind === "path") {
    state.hvacPathTypeFilter = value || "all";
  } else if (kind === "medium") {
    state.hvacMediumFilter = value || "all";
  }
}

function renderHVACQuickFilters() {
  const groups = [
    {
      kind: "service",
      value: state.hvacServiceKindFilter || "all",
      options: [
        ["all", "All"],
        ["cooling", "Cooling"],
        ["heating", "Heating"],
        ["ventilation", "Ventilation"],
        ["exhaust", "Exhaust"],
        ["direct", "Direct"],
      ],
    },
    {
      kind: "path",
      value: state.hvacPathTypeFilter || "all",
      options: [
        ["all", "All"],
        ["central_air", "Central air"],
        ["direct_zone", "Direct zone"],
        ["hydronic", "Hydronic"],
        ["refrigerant", "Refrigerant"],
        ["radiant", "Radiant"],
        ["local", "Local"],
      ],
    },
    {
      kind: "medium",
      value: state.hvacMediumFilter || "all",
      options: [
        ["all", "All"],
        ["air", "Air"],
        ["chilled_water", "CHW"],
        ["hot_water", "HW"],
        ["condenser_water", "CW"],
        ["refrigerant", "Refrigerant"],
        ["electricity", "Electric"],
        ["fuel", "Fuel"],
        ["service_water", "Water"],
      ],
    },
  ];
  return `
    <div class="hvac-quick-filters">
      ${groups
        .map(
          (group) => `
            <div class="hvac-filter-group" role="group" aria-label="${escapeHTML(group.kind)}">
              ${group.options
                .map(([value, label]) => `<button class="${group.value === value ? "active" : ""}" type="button" data-hvac-filter-kind="${escapeHTML(group.kind)}" data-hvac-filter-value="${escapeHTML(value)}" aria-pressed="${group.value === value ? "true" : "false"}">${escapeHTML(label)}</button>`)
                .join("")}
            </div>`,
        )
        .join("")}
    </div>`;
}

function servicePathMatchesQuickFilters(path = {}) {
  const serviceFilter = state.hvacServiceKindFilter || "all";
  const pathFilter = state.hvacPathTypeFilter || "all";
  const mediumFilter = state.hvacMediumFilter || "all";
  if (serviceFilter !== "all") {
    if (serviceFilter === "direct") {
      if (!String(path.pathType || "").startsWith("direct_") && !["radiant", "baseboard", "ideal_loads", "local"].includes(path.pathType)) {
        return false;
      }
    } else if (path.serviceKind !== serviceFilter) {
      return false;
    }
  }
  if (pathFilter !== "all" && !pathMatchesPathTypeFilter(path, pathFilter)) {
    return false;
  }
  if (mediumFilter !== "all" && !pathMediumsForFrontend(path).includes(mediumFilter)) {
    return false;
  }
  return true;
}

function pathMatchesPathTypeFilter(path = {}, filter = "all") {
  if (filter === "all") {
    return true;
  }
  if (filter === "central_air") {
    return ["central_air", "central_air_with_plant"].includes(path.pathType);
  }
  if (filter === "direct_zone") {
    return String(path.pathType || "").startsWith("direct_zone");
  }
  if (filter === "hydronic") {
    return path.pathType === "direct_zone_hydronic" || path.plantLoop;
  }
  if (filter === "refrigerant") {
    return path.pathType === "direct_zone_refrigerant" || path.refrigerantSystem;
  }
  if (filter === "radiant") {
    return ["radiant", "baseboard"].includes(path.pathType);
  }
  if (filter === "local") {
    return ["local", "ideal_loads"].includes(path.pathType) || path.sourceSystem?.type?.startsWith("local");
  }
  return path.pathType === filter;
}

function renderHVACGraphScopeControls() {
  const selectedPath = selectedHVACPath();
  const hasEntity = Boolean(state.activeHVACEntity?.id);
  const options = [
    ["focused", "Focused", false],
    ["all", "All", false],
    ["path_only", "Path only", !selectedPath],
    ["neighbors", "Neighbors", !hasEntity],
  ];
  return `
    <div class="hvac-graph-scope" role="group" aria-label="${escapeHTML(t("hvac.graphScope", {}, "Graph scope"))}">
      ${options
        .map(([value, label, disabled]) => {
          const active = hvacGraphScopeMode() === value;
          return `
            <button class="${active ? "active" : ""}" type="button" data-hvac-graph-scope="${escapeHTML(value)}" aria-pressed="${active ? "true" : "false"}" ${disabled ? "disabled" : ""}>
              ${escapeHTML(label)}
            </button>`;
        })
        .join("")}
    </div>`;
}

export function renderHVACLoopDiagram(loop) {
  const width = 1120;
  const leftX = 98;
  const rightX = 1022;
  const branchStartX = 220;
  const branchEndX = 900;
  const supplyFallbackItems = componentsForSide(loop.supplySide).length
    ? componentsForSide(loop.supplySide).map((component) => ({ kind: "component", component }))
    : [{ kind: "placeholder", label: t("hvac.supplySide") }];
  const demandFallbackItems = componentsForSide(loop.demandSide).length
    ? componentsForSide(loop.demandSide).map((component) => ({ kind: "component", component }))
    : (loop.relatedZones || []).map((zone) => ({ kind: "zone", zone }));
  const supplyLayout = buildLoopSideLayout(loop.supplySide, supplyFallbackItems, {
    side: "supply",
    top: 102,
    leftX,
    rightX,
    branchStartX,
    branchEndX,
    reverse: false,
  });
  const demandLayout = buildLoopSideLayout(loop.demandSide, demandFallbackItems.length ? demandFallbackItems : [{ kind: "placeholder", label: t("hvac.demandSide") }], {
    side: "demand",
    top: supplyLayout.top + supplyLayout.height + 110,
    leftX,
    rightX,
    branchStartX,
    branchEndX,
    reverse: true,
  });
  const height = demandLayout.top + demandLayout.height + 92;
  const selectedKey = state.activeHVACGraphKey || `loop:${loop.id}`;
  const loopSelected = selectedKey === `loop:${loop.id}` ? "selected" : "";

  return `
    <div class="hvac-graphic-shell ${hvacGraphScaleClass()}" style="--hvac-graph-width: ${width}px">
      <svg class="hvac-loop-svg" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(loop.name || "HVAC loop")} loop diagram">
        <defs>
          <marker id="hvacLoopArrow" markerWidth="10" markerHeight="10" refX="8" refY="3" orient="auto" markerUnits="strokeWidth">
            <path d="M0,0 L8,3 L0,6 Z" class="hvac-loop-arrow-marker"></path>
          </marker>
        </defs>
        <text class="hvac-loop-label" x="${leftX}" y="54">${escapeHTML(loop.type)}</text>
        <text class="hvac-loop-name" x="${leftX}" y="76">${escapeHTML(loop.name || t("hvac.unnamedLoop"))}</text>
        <path class="hvac-loop-connector ${loopSelected}" data-hvac-graph-key="loop:${escapeHTML(loop.id)}"
          d="M${rightX},${supplyLayout.busY} V${demandLayout.busY} M${leftX},${demandLayout.busY} V${supplyLayout.busY}" marker-end="url(#hvacLoopArrow)"></path>
        ${renderLoopSideNetwork(supplyLayout, loop)}
        ${renderLoopSideNetwork(demandLayout, loop)}
        ${renderLoopEndpoint(leftX, supplyLayout.busY, loop.supplySide?.inletNode, t("hvac.supplyInlet"))}
        ${renderLoopEndpoint(rightX, supplyLayout.busY, loop.supplySide?.outletNode, t("hvac.supplyOutlet"))}
        ${renderLoopEndpoint(rightX, demandLayout.busY, loop.demandSide?.inletNode, t("hvac.demandInlet"))}
        ${renderLoopEndpoint(leftX, demandLayout.busY, loop.demandSide?.outletNode, t("hvac.demandOutlet"))}
      </svg>
      <div class="hvac-legend">
        <span><i class="hvac-legend-supply"></i>${t("hvac.legendSupply")}</span>
        <span><i class="hvac-legend-demand"></i>${t("hvac.demandSide")}</span>
        <span><i class="hvac-legend-zone"></i>${t("hvac.legendZone")}</span>
      </div>
    </div>`;
}

function buildLoopSideLayout(sideData = {}, fallbackItems = [], options = {}) {
  const flow = loopSideFlow(sideData, fallbackItems, options.side);
  const rows = flow.rows;
  const rowGap = 64;
  const rowTopOffset = 46;
  const rowBottomPadding = 42;
  const height = Math.max(116, rowTopOffset + Math.max(1, rows.length - 1) * rowGap + rowBottomPadding);
  const rowYs = rows.map((_, index) => options.top + rowTopOffset + index * rowGap);
  const busY = rowYs.length > 1 ? (rowYs[0] + rowYs[rowYs.length - 1]) / 2 : rowYs[0];
  return {
    ...options,
    sideData,
    rows: rows.map((row, index) => ({ ...row, y: rowYs[index] })),
    leadInItems: flow.leadInItems,
    leadOutItems: flow.leadOutItems,
    hasParallel: flow.hasParallel,
    height,
    busY,
  };
}

function loopSideFlow(sideData = {}, fallbackItems = [], side = "") {
  const branches = sideData.branches || [];
  if (branches.length) {
    const branchByName = new Map(branches.map((branch, index) => [normalizeGraphName(branch.name), { branch, index }]));
    const splitters = (sideData.connectors || []).filter((connector) => String(connector.type || "").toLowerCase().includes("splitter"));
    const mixers = (sideData.connectors || []).filter((connector) => String(connector.type || "").toLowerCase().includes("mixer"));
    const parallelNames = new Set(
      [...splitters.flatMap((connector) => connector.branchNames || []), ...mixers.flatMap((connector) => connector.branchNames || [])].map(normalizeGraphName),
    );
    const leadInNames = new Set(splitters.map((connector) => normalizeGraphName(connector.inletBranchName)).filter(Boolean));
    const leadOutNames = new Set(mixers.map((connector) => normalizeGraphName(connector.outletBranchName)).filter(Boolean));
    if (parallelNames.size > 1) {
      const classifiedNames = new Set([...parallelNames, ...leadInNames, ...leadOutNames]);
      const parallelRows = branches
        .map((branch, index) => ({ branch, index }))
        .filter(({ branch }) => parallelNames.has(normalizeGraphName(branch.name)))
        .map(({ branch, index }) => loopBranchRow(branch, index, sideData, side));
      const extraRows = branches
        .map((branch, index) => ({ branch, index }))
        .filter(({ branch }) => !classifiedNames.has(normalizeGraphName(branch.name)))
        .map(({ branch, index }) => loopBranchRow(branch, index, sideData, side));
      const rows = [...parallelRows, ...extraRows];
      return {
        rows: rows.length ? rows : branches.map((branch, index) => loopBranchRow(branch, index, sideData, side)),
        leadInItems: itemsForBranchNames(branchByName, leadInNames, sideData, side),
        leadOutItems: itemsForBranchNames(branchByName, leadOutNames, sideData, side),
        hasParallel: true,
      };
    }
    return {
      rows: branches.map((branch, index) => loopBranchRow(branch, index, sideData, side)),
      leadInItems: [],
      leadOutItems: [],
      hasParallel: false,
    };
  }
  return {
    rows: [
      {
        branch: null,
        index: 0,
        key: `side:${side}`,
        label: sideData.name || side,
        items: fallbackItems.length ? fallbackItems : [{ kind: "placeholder", label: sideData.name || side }],
      },
    ],
    leadInItems: [],
    leadOutItems: [],
    hasParallel: false,
  };
}

function loopBranchRow(branch, index, sideData = {}, side = "") {
  return {
    branch,
    index,
    key: branchGraphKey(branch),
    label: branch.name || `${sideData.name || side} ${index + 1}`,
    items: loopBranchItems(branch, sideData, side),
  };
}

function loopBranchItems(branch, sideData = {}, side = "") {
  return (branch.components || []).length
    ? (branch.components || []).map((component) => ({ kind: "component", component }))
    : [{ kind: "placeholder", label: branch.name || sideData.name || side }];
}

function itemsForBranchNames(branchByName, names, sideData = {}, side = "") {
  return [...names].flatMap((name) => {
    const entry = branchByName.get(name);
    return entry ? loopBranchItems(entry.branch, sideData, side) : [];
  });
}

function normalizeGraphName(value) {
  return String(value || "").trim().toLowerCase();
}

function renderLoopSideNetwork(layout, loop) {
  const startX = layout.reverse ? layout.rightX : layout.leftX;
  const endX = layout.reverse ? layout.leftX : layout.rightX;
  const splitX = layout.reverse ? layout.branchEndX : layout.branchStartX;
  const mixX = layout.reverse ? layout.branchStartX : layout.branchEndX;
  const minY = layout.rows[0]?.y || layout.busY;
  const maxY = layout.rows[layout.rows.length - 1]?.y || layout.busY;
  const loopKey = `loop:${loop.id}`;
  const loopClass = state.activeHVACGraphKey === loopKey ? "selected" : state.activeHVACGraphKey ? "dimmed" : "";
  const sideLabel = layout.side === "supply" ? t("hvac.supplySide") : t("hvac.demandSide");
  const branchPaths = layout.rows
    .map((row) => {
      const rowComponentKeys = row.items
        .map((item) => (item.kind === "component" ? componentGraphKey(item.component) : ""))
        .filter(Boolean);
      const branchClass = graphSelectionClass(row.key, [loopKey, ...rowComponentKeys]);
      return `
        <path class="hvac-loop-branch-path ${branchClass}" data-hvac-graph-key="${escapeHTML(row.key)}" d="M${splitX},${row.y} H${mixX}"></path>
        ${renderLoopBranchMarker(row, splitX, row.y, loopKey)}
        ${renderLoopBranchItems(row, layout, loopKey)}`;
    })
    .join("");
  return `
    <g class="hvac-loop-side-block ${escapeHTML(layout.side)}">
      <rect class="hvac-loop-side-panel" x="${layout.branchStartX - 62}" y="${layout.top}" width="${layout.branchEndX - layout.branchStartX + 124}" height="${layout.height}" rx="0"></rect>
      <text class="hvac-loop-side-note" x="${layout.branchStartX - 45}" y="${layout.top + 22}">${escapeHTML(sideLabel)}</text>
      <path class="hvac-loop-path ${loopClass}" data-hvac-graph-key="${escapeHTML(loopKey)}"
        d="M${startX},${layout.busY} H${splitX} M${splitX},${minY} V${maxY} M${mixX},${minY} V${maxY} M${mixX},${layout.busY} H${endX}"></path>
      ${renderLoopSerialItems(layout.leadInItems, layout, startX, splitX, layout.busY, loopKey)}
      ${renderLoopSerialItems(layout.leadOutItems, layout, mixX, endX, layout.busY, loopKey)}
      ${branchPaths}
    </g>`;
}

function renderLoopSerialItems(items = [], layout, fromX, toX, y, loopKey) {
  if (!items.length) {
    return "";
  }
  const direction = toX >= fromX ? 1 : -1;
  const start = fromX + direction * 54;
  const end = toX - direction * 54;
  const positions = distributeGraphPositions(items.length, start, end, y);
  return items.map((item, index) => renderLoopDiagramItem(item, positions[index], layout.side, [loopKey])).join("");
}

function renderLoopBranchItems(row, layout, loopKey) {
  const start = layout.reverse ? layout.branchEndX - 72 : layout.branchStartX + 72;
  const end = layout.reverse ? layout.branchStartX + 72 : layout.branchEndX - 72;
  const positions = distributeGraphPositions(row.items.length, start, end, row.y);
  return row.items.map((item, index) => renderLoopDiagramItem(item, positions[index], layout.side, [row.key, loopKey])).join("");
}

function renderLoopBranchMarker(row, splitX, y, loopKey) {
  const markerX = row.index % 2 === 0 ? splitX - 24 : splitX + 24;
  const relatedKeys = row.items
    .map((item) => (item.kind === "component" ? componentGraphKey(item.component) : ""))
    .filter(Boolean);
  return `
    <g class="hvac-branch-badge ${graphSelectionClass(row.key, [loopKey, ...relatedKeys])}" data-hvac-graph-key="${escapeHTML(row.key)}">
      <title>${escapeHTML(row.label || "Branch")}</title>
      <circle cx="${markerX}" cy="${y}" r="8"></circle>
      <text x="${markerX}" y="${y + 4}" text-anchor="middle">${escapeHTML(row.index + 1)}</text>
    </g>`;
}

function renderLoopEndpoint(x, y, nodeName, label) {
  const key = nodeName ? `node:${nodeName}` : `endpoint:${label}`;
  const selected = graphSelectionClass(key, [nodeName ? `node:${nodeName}` : ""]);
  return `
    <g class="hvac-loop-endpoint ${selected}" data-hvac-graph-key="${escapeHTML(key)}">
      <title>${escapeHTML([label, nodeName].filter(Boolean).join(" - "))}</title>
      <circle cx="${x}" cy="${y}" r="9"></circle>
      <circle class="port-ring" cx="${x}" cy="${y}" r="4"></circle>
    </g>`;
}

function renderLoopDiagramItem(item, position, side, relatedKeys = []) {
  if (!position) {
    return "";
  }
  if (item.kind === "zone") {
    const key = `zone:${item.zone}`;
    return renderLoopEquipmentSymbol({
      key,
      x: position.x,
      y: position.y,
      label: item.zone,
      meta: "Zone",
      iconKind: "zone",
      shortLabel: "Zone",
      className: `zone ${graphSelectionClass(key, relatedKeys)}`,
    });
  }
  if (item.kind === "placeholder") {
    const key = `placeholder:${side}`;
    return renderLoopEquipmentSymbol({
      key,
      x: position.x,
      y: position.y,
      label: item.label,
      meta: "No parsed components",
      iconKind: "component",
      shortLabel: "",
      className: `placeholder ${side} ${graphSelectionClass(key, relatedKeys)}`,
    });
  }
  const component = item.component;
  const key = componentGraphKey(component);
  const className = `${side} ${component.exists ? "" : "missing"} ${graphSelectionClass(key, [...componentGraphRelatedKeys(component), ...relatedKeys])}`;
  const visual = componentVisual(component);
  return renderLoopEquipmentSymbol({
    key,
    x: position.x,
    y: position.y,
    label: componentDisplayName(component),
    meta: componentMetaLabel(component),
    iconKind: visual.iconKind,
    shortLabel: visual.shortLabel,
    objectType: component.objectType || "",
    crossLoopNames: component.relatedLoopNames || [],
    className,
  });
}

function renderLoopEquipmentSymbol({ key, x, y, label, meta, iconKind, shortLabel, objectType = "", crossLoopNames = [], className = "" }) {
  const title = [label, meta].filter(Boolean).join(" - ");
  const iconClass = escapeHTML(iconKind || "component");
  return `
    <g class="hvac-loop-equipment ${iconClass} ${className}" data-hvac-graph-key="${escapeHTML(key)}" aria-label="${escapeHTML(title)}">
      <title>${escapeHTML(title)}</title>
      <rect class="hvac-loop-equipment-hit" x="${x - 36}" y="${y - 38}" width="72" height="76" rx="10"></rect>
      <circle class="pipe-port left" cx="${x - 42}" cy="${y}" r="5"></circle>
      <circle class="pipe-port right" cx="${x + 42}" cy="${y}" r="5"></circle>
      ${renderLoopEquipmentBody(iconKind, x, y, objectType)}
      ${shortLabel ? `<text class="mini-label" x="${x}" y="${y + 35}" text-anchor="middle">${escapeHTML(truncateText(shortLabel, 12))}</text>` : ""}
      ${renderLoopCrossIndicator(x, y, crossLoopNames)}
    </g>`;
}

function renderLoopCrossIndicator(x, y, loopNames = []) {
  if (!loopNames.length) {
    return "";
  }
  return `
    <g class="hvac-loop-cross-indicator" aria-hidden="true">
      <title>${escapeHTML(`${t("hvac.crossLoop")}: ${loopNames.join(", ")}`)}</title>
      <circle cx="${x + 29}" cy="${y - 27}" r="9"></circle>
      <text x="${x + 29}" y="${y - 23}" text-anchor="middle">${escapeHTML(Math.min(loopNames.length, 9))}</text>
    </g>`;
}

function renderGraphNode({ key, x, y, width, height, label, meta, className = "", iconKind = "", shortLabel = "", tooltip = "" }) {
  const hasIcon = Boolean(iconKind);
  const labelMax = hasIcon ? Math.max(8, Math.floor((width - 58) / 7)) : Math.max(10, Math.floor((width - 18) / 7));
  const metaMax = hasIcon ? Math.max(8, Math.floor((width - 58) / 6)) : Math.max(10, Math.floor((width - 18) / 6));
  const textX = hasIcon ? x - width / 2 + 48 : x;
  const textAnchor = hasIcon ? "start" : "middle";
  const title = tooltip || [label, meta].filter(Boolean).join(" - ");
  return `
    <g class="hvac-graph-node ${className}" data-hvac-graph-key="${escapeHTML(key)}" aria-label="${escapeHTML(title)}">
      <title>${escapeHTML(title)}</title>
      <rect x="${x - width / 2}" y="${y - height / 2}" width="${width}" height="${height}" rx="8"></rect>
      ${hasIcon ? renderHVACNodeIcon(iconKind, x - width / 2 + 25, y) : ""}
      <text class="label" x="${textX}" y="${y - 4}" text-anchor="${textAnchor}">${escapeHTML(truncateText(shortLabel || label || "", labelMax))}</text>
      <text class="meta" x="${textX}" y="${y + 14}" text-anchor="${textAnchor}">${escapeHTML(truncateText(meta || "", metaMax))}</text>
    </g>`;
}

function renderHVACLoopGraphDetail(loop) {
  const selected = selectedLoopGraphItem(loop);
  if (!selected) {
    return `
      <section class="hvac-graph-detail">
        <div class="hvac-section-head">
          <h3>${t("hvac.loopDetail")}</h3>
          <span>${t("hvac.loopDetailHint")}</span>
        </div>
        <div class="hvac-detail-grid">
          <div><span>${t("hvac.supplyBranches")}</span><strong>${escapeHTML((loop.supplySide?.branches || []).length)}</strong></div>
          <div><span>${t("hvac.demandBranches")}</span><strong>${escapeHTML((loop.demandSide?.branches || []).length)}</strong></div>
          <div><span>${t("hvac.relatedZones")}</span><strong>${escapeHTML((loop.relatedZones || []).length)}</strong></div>
          <div><span>${t("hvac.crossLoopLinksLabel")}</span><strong>${escapeHTML((loop.relatedLoops || []).length)}</strong></div>
        </div>
      </section>`;
  }
  return renderSelectedHVACDetail(selected);
}

function renderHVACLoopSide(side = {}) {
  return `
    <section class="hvac-loop-side">
      <div class="hvac-section-head">
        <h3>${escapeHTML(side.name || "Side")}</h3>
        <span>${escapeHTML(t("hvac.branchCount", { count: (side.branches || []).length }))}</span>
      </div>
      <div class="hvac-node-line">
        ${renderNodePill(side.inletNode, t("common.inlet"))}
        <span class="hvac-arrow">-&gt;</span>
        ${renderNodePill(side.outletNode, t("common.outlet"))}
      </div>
      <div class="hvac-side-meta">
        ${side.branchListName ? `<span>BranchList ${escapeHTML(side.branchListName)}</span>` : ""}
        ${side.connectorListName ? `<span>ConnectorList ${escapeHTML(side.connectorListName)}</span>` : ""}
      </div>
      <div class="hvac-branch-list">
        ${(side.branches || []).map(renderHVACBranch).join("") || `<div class="empty">${t("hvac.noBranches")}</div>`}
      </div>
      ${(side.connectors || []).length ? `<div class="hvac-connector-list">${side.connectors.map(renderHVACConnector).join("")}</div>` : ""}
    </section>`;
}

function renderHVACBranch(branch) {
  return `
    <article class="hvac-branch">
      <div class="hvac-branch-head">
        <strong>${escapeHTML(branch.name || "Branch")}</strong>
        ${renderObjectLink(branch.objectIndex, "Branch")}
      </div>
      <div class="hvac-node-line">
        ${renderNodePill(branch.inletNode, "In")}
        <span class="hvac-arrow">-&gt;</span>
        ${renderNodePill(branch.outletNode, "Out")}
      </div>
      <div class="hvac-component-list">
        ${(branch.components || []).map(renderHVACComponent).join("") || `<div class="empty">${t("hvac.noComponents")}</div>`}
      </div>
      ${(branch.warnings || []).length ? `<div class="hvac-inline-warning">${branch.warnings.map((warning) => escapeHTML(warning.message)).join("<br />")}</div>` : ""}
    </article>`;
}

function renderHVACComponent(component) {
  const existsClass = component.exists ? "" : " missing";
  const graphKey = componentGraphKey(component);
  const displayName = componentDisplayName(component);
  const metaLabel = componentMetaLabel(component);
  const title = [displayName, metaLabel].filter(Boolean).join(" - ");
  const ruleEdges = ruleEdgesForComponent(component);
  const relatedLoopNames = verifiedCrossLoopNamesForComponent(component);
  const badges = renderHVACRuleBadges([
    component.exists === false ? "Missing object" : "",
    component.displayLabel || component.familyLabel || component.family,
    component.roleHere,
    component.listedInZoneEquipment ? "Zone equipment" : "",
    component.resolvedFromAirDistributionUnit ? "ADU resolved" : "",
    component.inletOnAirLoopDemandPath ? "Demand path" : "",
    component.outletMatchesZoneInlet ? "Zone inlet" : "",
    ...(hvacDebugEnabled() ? ruleEdges.slice(0, 3).map((edge) => edge.ruleId) : []),
  ]);
  return `
    <div class="hvac-component${existsClass}">
      <div class="hvac-component-main">
        <button class="hvac-component-select" type="button" data-hvac-graph-key="${escapeHTML(graphKey)}" title="${escapeHTML(title)}">
          <strong title="${escapeHTML(displayName)}">${escapeHTML(displayName)}</strong>
          <span title="${escapeHTML(metaLabel)}">${escapeHTML(metaLabel)}</span>
        </button>
        <span class="hvac-component-source">${renderObjectLink(component.objectIndex, component.objectType)}</span>
      </div>
      ${badges}
      ${renderHVACComponentSourceReference(component)}
      <div class="hvac-node-line compact">
        ${renderNodePill(component.inletNode, "In")}
        <span class="hvac-arrow">-&gt;</span>
        ${renderNodePill(component.outletNode, "Out")}
      </div>
      ${
        component.waterInletNode || component.waterOutletNode
          ? `<div class="hvac-node-line compact water">${renderNodePill(component.waterInletNode, "Water In")}<span class="hvac-arrow">-&gt;</span>${renderNodePill(component.waterOutletNode, "Water Out")}</div>`
          : ""
      }
      ${relatedLoopNames.length ? renderComponentCrossLoopButtons(component, relatedLoopNames) : ""}
      ${renderHVACEditableFields(component.editableFields)}
    </div>`;
}

function renderComponentCrossLoopButtons(component, relatedLoopNames = component.relatedLoopNames || []) {
  const graphKey = componentGraphKey(component);
  return `
    <div class="hvac-cross-loop-buttons">
      <small>${t("hvac.crossLoop")}</small>
      ${relatedLoopNames
        .map(
          (loopName) => `
            <button type="button" data-hvac-jump-loop-name="${escapeHTML(loopName)}" data-hvac-jump-graph-key="${escapeHTML(graphKey)}" title="${escapeHTML(loopName)}">
              ${escapeHTML(loopName)}
            </button>`,
        )
        .join("")}
    </div>`;
}

function renderHVACRuleBadges(values = []) {
  const badges = [...new Set(values.map((value) => String(value || "").trim()).filter(Boolean))];
  if (!badges.length) {
    return "";
  }
  return `<div class="hvac-rule-badges">${badges.map((badge) => `<small title="${escapeHTML(badge)}">${escapeHTML(shortRuleBadgeLabel(badge))}</small>`).join("")}</div>`;
}

function shortRuleBadgeLabel(value) {
  const text = String(value || "").trim();
  if (!text.includes(".")) {
    return text;
  }
  const parts = text.split(".");
  return parts[parts.length - 1].replace(/_/g, " ");
}

function hvacRuleEdges() {
  return state.report?.hvac?.ruleGraph?.edges || [];
}

function hvacRuleNodesByID() {
  const nodes = state.report?.hvac?.ruleGraph?.nodes || [];
  return new Map(nodes.map((node) => [node.id, node]));
}

function ruleEdgesForComponent(component = {}) {
  return uniqueRuleEdges(hvacRuleEdges().filter((edge) => ruleEdgeTouchesObject(edge, component.objectType, component.objectName, component.objectIndex)));
}

function ruleEdgesForLoop(loopName, loopType = "") {
  return uniqueRuleEdges(hvacRuleEdges().filter((edge) => ruleEdgeTouchesObject(edge, loopType, loopName)));
}

function ruleEdgeTouchesObject(edge = {}, objectType = "", objectName = "", objectIndex = undefined) {
  const index = Number(objectIndex);
  if (Number.isFinite(index) && index >= 0 && Number(edge.sourceObjectIndex) === index) {
    return true;
  }
  if (sameObjectRef(edge.sourceObjectType, edge.sourceObjectName, objectType, objectName)) {
    return true;
  }
  const nodes = hvacRuleNodesByID();
  for (const nodeID of [edge.fromId, edge.toId]) {
    const node = nodes.get(nodeID);
    if (!node) {
      continue;
    }
    if (Number.isFinite(index) && index >= 0 && Number(node.objectIndex) === index) {
      return true;
    }
    if (sameObjectRef(node.objectType, node.objectName, objectType, objectName)) {
      return true;
    }
  }
  return false;
}

function sameObjectRef(leftType = "", leftName = "", rightType = "", rightName = "") {
  return (
    String(leftType || "").trim().toLowerCase() === String(rightType || "").trim().toLowerCase() &&
    String(leftName || "").trim().toLowerCase() === String(rightName || "").trim().toLowerCase() &&
    String(rightType || "").trim() !== "" &&
    String(rightName || "").trim() !== ""
  );
}

function uniqueRuleEdges(edges = []) {
  const byID = new Map();
  for (const edge of edges) {
    if (!edge?.id || byID.has(edge.id)) {
      continue;
    }
    byID.set(edge.id, edge);
  }
  return [...byID.values()].sort((left, right) => String(left.ruleId || "").localeCompare(String(right.ruleId || "")));
}

function ruleEdgeTraceText(edge = {}) {
  const source = [edge.sourceObjectType, edge.sourceObjectName].filter(Boolean).join(" ");
  const fields = (edge.sourceFieldIndexes || []).map((index) => `F${Number(index) + 1}`).join(", ");
  const nodes = (edge.nodeNames || []).join(" -> ");
  return [source, fields ? `fields ${fields}` : "", nodes ? `nodes ${nodes}` : ""].filter(Boolean).join(" / ") || "Rule source";
}

function ruleEdgeSearchFields(edge = {}) {
  return [
    edge.ruleId,
    edge.edgeKind,
    edge.medium,
    edge.sourceObjectType,
    edge.sourceObjectName,
    ...(edge.sourceFieldIndexes || []).map((index) => `field ${Number(index) + 1}`),
    ...(edge.nodeNames || []),
  ];
}

function verifiedCrossLoopNamesForComponent(component = {}) {
  const hasRuleEdge = ruleEdgesForComponent(component).some((edge) => edge.ruleId === "crossloop.same_water_coil_air_and_plant");
  if (!hasRuleEdge) {
    return [];
  }
  return [...new Set(component.relatedLoopNames || [])].filter(Boolean);
}

function renderHVACEditableFields(fields = []) {
  if (!fields.length) {
    return "";
  }
  return `
    <div class="hvac-edit-field-list">
      ${fields
        .slice(0, 4)
        .map(
          (field) => `
            <button class="hvac-edit-button" data-hvac-edit-key="${escapeHTML(hvacEditKey(field))}" type="button">
              <span>${escapeHTML(hvacEditLabel(field))}</span>
              <small>${escapeHTML(field.currentValue || t("common.blank"))}</small>
            </button>`,
        )
        .join("")}
    </div>`;
}

function renderHVACConnector(connector) {
  return `
    <article class="hvac-connector">
      <strong>${escapeHTML(connector.type)} ${escapeHTML(connector.name)}</strong>
      ${renderObjectLink(connector.objectIndex, connector.type)}
      <div>${(connector.branchNames || []).map((branch) => `<span>${escapeHTML(branch)}</span>`).join("")}</div>
    </article>`;
}

function renderCrossLoopRelations(loop) {
  const relations = loop.relatedLoops || [];
  if (!relations.length) {
    return "";
  }
  return `
    <section class="hvac-cross-loop">
      <div class="hvac-section-head">
        <h3>${t("hvac.crossLoopRelations")}</h3>
        <span>${escapeHTML(t("hvac.linkCount", { count: relations.length }))}</span>
      </div>
      <div class="hvac-relation-list">
        ${relations
          .map(
            (relation) => {
              const componentKey = componentKeyForCrossLoopRelation(loop, relation);
              return `
                <button class="hvac-relation-row hvac-cross-loop-row" type="button"
                  data-hvac-jump-loop-name="${escapeHTML(relation.loopName)}"
                  data-hvac-jump-graph-key="${escapeHTML(componentKey)}"
                  title="${escapeHTML(`${relation.componentType} ${relation.componentName} -> ${relation.loopName}`)}">
                  <strong>${escapeHTML(relation.componentType)} ${escapeHTML(relation.componentName)}</strong>
                  <span>${escapeHTML(relation.loopType)} ${escapeHTML(relation.loopName)}</span>
                </button>`;
            },
          )
          .join("")}
      </div>
    </section>`;
}

function componentKeyForCrossLoopRelation(loop, relation) {
  const component = loopComponents(loop).find(
    (item) =>
      String(item.objectType || "").toLowerCase() === String(relation.componentType || "").toLowerCase() &&
      String(item.objectName || "").toLowerCase() === String(relation.componentName || "").toLowerCase(),
  );
  return component ? componentGraphKey(component) : "";
}

function hvacServiceModel(hvac = state.report?.hvac) {
  return hvac?.serviceModel || { zoneServices: [], systems: [], couplings: [], networks: [] };
}

function servicePathsForHVAC(hvac = state.report?.hvac) {
  return (hvacServiceModel(hvac).zoneServices || []).flatMap((summary) => summary.paths || []);
}

function selectedHVACPath(hvac = state.report?.hvac) {
  const paths = servicePathsForHVAC(hvac);
  const pathId = state.activeHVACContext?.pathId || (state.activeHVACEntity?.kind === "service_path" ? pathIDFromNavigationEntityID(state.activeHVACEntity.id) : "");
  if (pathId) {
    return paths.find((path) => path.id === pathId) || null;
  }
  if (state.activeHVACGraphKey?.startsWith("service-path:")) {
    const graphPathID = state.activeHVACGraphKey.slice("service-path:".length);
    return paths.find((path) => path.id === graphPathID) || null;
  }
  const entity = state.activeHVACEntity || {};
  const indexed = entity.id ? findHVACNavigationEntity(entity.id) : null;
  const relatedPathID = (indexed?.relatedPathIds || [])[0];
  return relatedPathID ? paths.find((path) => path.id === relatedPathID) || null : null;
}

function pathsForActiveHVACEntity(hvac = state.report?.hvac) {
  const paths = servicePathsForHVAC(hvac);
  const scope = hvacGraphScopeMode();
  if (scope === "all" || !state.activeHVACEntity?.id) {
    return paths;
  }
  const selectedPath = selectedHVACPath(hvac);
  if (scope === "path_only") {
    return selectedPath ? [selectedPath] : paths;
  }
  const ids = relatedPathIDsForActiveHVACEntity(hvac);
  if (!ids.length) {
    return selectedPath ? [selectedPath] : paths;
  }
  const wanted = new Set(ids);
  return paths.filter((path) => wanted.has(path.id));
}

function relatedPathIDsForActiveHVACEntity(hvac = state.report?.hvac) {
  const entity = state.activeHVACEntity || {};
  const navigation = hvacNavigationIndex(hvac);
  if (!entity.id || !entity.kind) {
    return [];
  }
  if (entity.kind === "service_path") {
    const pathID = pathIDFromNavigationEntityID(entity.id);
    return pathID ? [pathID] : [];
  }
  if (entity.kind === "zone") {
    return navigation.byZone?.[entity.id] || fallbackPathIDsForSubject(hvac, entity.id);
  }
  if (entity.kind === "space") {
    return navigation.bySpace?.[entity.id] || fallbackPathIDsForSubject(hvac, entity.id);
  }
  if (entity.kind === "loop") {
    return navigation.byLoop?.[entity.id] || fallbackPathIDsForLoop(hvac, entity.id);
  }
  if (entity.kind === "component") {
    return navigation.byComponent?.[entity.id] || fallbackPathIDsForComponent(hvac, entity.id);
  }
  if (entity.kind === "coupling") {
    return navigation.byCoupling?.[entity.id] || [];
  }
  if (entity.kind === "network") {
    const couplingIDs = navigation.byNetwork?.[entity.id] || [];
    return [...new Set(couplingIDs.flatMap((id) => navigation.byCoupling?.[navigationCouplingEntityID(id)] || []))];
  }
  return findHVACNavigationEntity(entity.id)?.relatedPathIds || [];
}

function fallbackPathIDsForSubject(hvac, subjectID) {
  return servicePathsForHVAC(hvac)
    .filter((path) => servedSubjectKey(path.servedSubject || path) === subjectID)
    .map((path) => path.id);
}

function fallbackPathIDsForLoop(hvac, loopEntityID) {
  const loop = loopForNavigationEntityID(loopEntityID);
  if (!loop) {
    return [];
  }
  const loopKey = loopRefGraphKey(loop.type, loop.name);
  return servicePathsForHVAC(hvac)
    .filter((path) => [path.plantLoop, path.condenserLoop, path.airLoop].filter(Boolean).some((ref) => loopRefGraphKey(ref.type, ref.name) === loopKey))
    .map((path) => path.id);
}

function fallbackPathIDsForComponent(hvac, componentID) {
  return servicePathsForHVAC(hvac)
    .filter((path) => pathComponentNavigationIDs(path).includes(componentID))
    .map((path) => path.id);
}

function pathComponentNavigationIDs(path = {}) {
  return [path.delivery, path.deliveryWrapper, ...(path.conditioning || [])]
    .filter(Boolean)
    .map((component) => navigationComponentEntityID(component));
}

function navigationComponentEntityID(component = {}) {
  const baseID = navigationComponentBaseID(component);
  if (baseID) {
    return disambiguatedNavigationComponentEntityID(component, baseID);
  }
  return component.id ? `component:${normalizeGraphName(component.id)}` : "";
}

function navigationComponentBaseID(component = {}) {
  const objectType = normalizeGraphName(component.objectType || "");
  const objectName = normalizeGraphName(component.objectName || component.displayName || "");
  if (objectType && objectName) {
    return `component:${objectType}:${objectName}`;
  }
  return "";
}

function disambiguatedNavigationComponentEntityID(component = {}, baseID = navigationComponentBaseID(component)) {
  if ((navigationComponentBaseCounts().get(baseID) || 0) < 2) {
    return baseID;
  }
  const suffix = navigationComponentInstanceSuffix(component);
  return suffix ? `${baseID}${suffix}` : baseID;
}

function navigationComponentBaseCounts() {
  const serviceModel = hvacServiceModel();
  if (hvacComponentBaseCountCache.serviceModel === serviceModel) {
    return hvacComponentBaseCountCache.counts;
  }
  const groups = new Map();
  const add = (component = {}) => {
    const baseID = navigationComponentBaseID(component);
    if (!baseID) {
      return;
    }
    if (!groups.has(baseID)) {
      groups.set(baseID, new Set());
    }
    groups.get(baseID).add(navigationComponentInstanceKey(component));
  };
  servicePathsForHVAC().forEach((path) => {
    [path.delivery, path.deliveryWrapper, ...(path.conditioning || [])].filter(Boolean).forEach(add);
  });
  (serviceModel.components || []).forEach((item) => {
    add(item.component || {});
    (item.internalRefs || []).forEach(add);
  });
  (serviceModel.couplings || []).forEach((coupling) => {
    add(coupling.object || {});
    (coupling.connectedComponents || []).forEach(add);
  });
  const counts = new Map([...groups.entries()].map(([baseID, instances]) => [baseID, instances.size]));
  hvacComponentBaseCountCache = { serviceModel, counts };
  return counts;
}

function navigationComponentInstanceKey(component = {}) {
  const index = Number(component.objectIndex);
  if (Number.isFinite(index) && index >= 0) {
    return `index:${Math.trunc(index)}`;
  }
  if (component.id) {
    return `id:${component.id}`;
  }
  return `base:${navigationComponentBaseID(component)}`;
}

function navigationComponentInstanceSuffix(component = {}) {
  const index = Number(component.objectIndex);
  if (Number.isFinite(index) && index >= 0) {
    return `:source:${Math.trunc(index)}`;
  }
  const id = String(component.id || "").trim();
  const indexMatch = id.match(/^component:(\d+)$/i);
  if (indexMatch) {
    return `:source:${indexMatch[1]}`;
  }
  return id ? `:source:${normalizeGraphName(id)}` : "";
}

function hvacDebugEnabled() {
  try {
    return new URLSearchParams(window.location.search).has("debug") || localStorage.getItem("idfAnalyzer.debugMode") === "1";
  } catch {
    return false;
  }
}

function servedSubjectKey(subject = {}) {
  if ((subject.kind || "").toLowerCase() === "space" && subject.spaceName) {
    return `space:${normalizeGraphName(subject.spaceName)}`;
  }
  return `zone:${normalizeGraphName(subject.zoneName || subject.name)}`;
}

function servedSubjectLabel(subject = {}) {
  if ((subject.kind || "").toLowerCase() === "space" && subject.spaceName) {
    return subject.zoneName ? `${subject.spaceName} / ${subject.zoneName}` : subject.spaceName;
  }
  return subject.zoneName || subject.name || t("common.blank");
}

function servicePathGraphKey(path = {}) {
  return `service-path:${path.id || servedSubjectKey(path.servedSubject || path)}`;
}

function serviceGraphNodeKey(path, specOrRole) {
  const spec =
    typeof specOrRole === "string"
      ? servicePathNodeSpecs(path).find((item) => item.role === specOrRole) || { role: specOrRole }
      : specOrRole || {};
  return `service-node:${spec.role || "node"}:${serviceGraphNodeIdentity(path, spec)}`;
}

function serviceGraphNodeIdentity(path = {}, spec = {}) {
  if (spec.role === "zone") {
    return servedSubjectKey(path.servedSubject || path);
  }
  const ref = spec.ref || {};
  if (ref.id) {
    return normalizeGraphName(ref.id);
  }
  const typedName = [ref.objectType || ref.type || spec.kind || spec.role, ref.objectName || ref.name || spec.label].filter(Boolean).join(":");
  return normalizeGraphName(typedName || spec.label || spec.role || "node");
}

function serviceKindLabel(kind = "") {
  const labels = {
    cooling: "Cooling",
    heating: "Heating",
    ventilation: "Ventilation",
    exhaust: "Exhaust",
    humidification: "Humidification",
    dehumidification: "Dehumidification",
    radiant_cooling: "Radiant cooling",
    radiant_heating: "Radiant heating",
    mixed: "Mixed",
  };
  return labels[kind] || titleCaseToken(kind || "Service");
}

function pathTypeLabel(pathType = "") {
  const labels = {
    central_air_with_plant: "Central air + plant",
    central_air: "Central air",
    direct_zone_hydronic: "Direct hydronic",
    direct_zone_air: "Direct zone air",
    direct_zone_refrigerant: "Refrigerant",
    radiant: "Radiant",
    baseboard: "Baseboard",
    ideal_loads: "Ideal loads",
    ventilation_only: "Ventilation only",
    exhaust_only: "Exhaust only",
    service_water: "Service water",
    local: "Local",
  };
  return labels[pathType] || titleCaseToken(pathType || "Path");
}

function titleCaseToken(value = "") {
  return String(value || "")
    .replace(/_/g, " ")
    .replace(/\w\S*/g, (part) => part.charAt(0).toUpperCase() + part.slice(1).toLowerCase());
}

function deliveryLabel(path = {}) {
  return path.deliveryEquipment?.displayFamily || path.delivery?.displayFamily || path.delivery?.displayName || path.delivery?.objectName || "Delivery";
}

function sourceSystemLabel(system = {}) {
  const labels = {
    local_dx: "Local DX",
    local: "Local",
    ideal_loads: "Ideal Loads",
    source: "Source",
    refrigerant_system: "Refrigerant System",
  };
  return labels[system.type] || titleCaseToken(system.type || "Source");
}

function servicePathSourceText(path = {}) {
  const parts = [];
  if (path.plantLoop?.name) {
    parts.push(path.plantLoop.name);
  }
  if ((path.conditioning || [])[0]) {
    parts.push((path.conditioning || [])[0].displayName || (path.conditioning || [])[0].objectName);
  }
  if (path.refrigerantSystem?.name) {
    parts.push(path.refrigerantSystem.name);
  }
  if (path.sourceSystem?.name) {
    parts.push(path.sourceSystem.name);
  }
  return parts.filter(Boolean).join(" / ");
}

function servicePathCoupledText(path = {}, couplings = []) {
  const byID = new Map((couplings || []).map((coupling) => [coupling.id, coupling]));
  const names = (path.supportingCouplingIds || [])
    .map((id) => byID.get(id))
    .filter(Boolean)
    .map((coupling) => coupling.object?.displayName || coupling.object?.objectName || coupling.role);
  const loops = [path.plantLoop?.name, path.condenserLoop?.name].filter(Boolean);
  return [...new Set([...loops, ...names])].join(", ");
}

function servicePathConnectedSystems(path = {}) {
  return [
    path.plantLoop?.name ? `PlantLoop ${path.plantLoop.name}` : "",
    path.airLoop?.name ? `AirLoopHVAC ${path.airLoop.name}` : "",
    path.condenserLoop?.name ? `CondenserLoop ${path.condenserLoop.name}` : "",
    path.refrigerantSystem?.name ? `${sourceSystemLabel(path.refrigerantSystem)} ${path.refrigerantSystem.name}` : "",
    path.sourceSystem?.name ? `${sourceSystemLabel(path.sourceSystem)} ${path.sourceSystem.name}` : "",
  ].filter(Boolean);
}

function zoneServiceMatchesQuery(summary, query) {
  if (!query) {
    return true;
  }
  return [
    summary.zoneName,
    summary.spaceName,
    ...(summary.paths || []).flatMap(servicePathSearchFields),
    ...(summary.issues || []).map((issue) => `${issue.code || ""} ${issue.message || ""}`),
  ]
    .join(" ")
    .toLowerCase()
    .includes(query);
}

function servicePathMatchesQuery(path, query) {
  if (!query) {
    return true;
  }
  return servicePathSearchFields(path).join(" ").toLowerCase().includes(query);
}

function servicePathSearchFields(path = {}) {
  return [
    path.zoneName,
    path.spaceName,
    path.serviceKind,
    path.pathType,
    deliveryLabel(path),
    path.delivery?.objectType,
    path.delivery?.objectName,
    path.plantLoop?.name,
    path.airLoop?.name,
    path.condenserLoop?.name,
    path.refrigerantSystem?.name,
    path.sourceSystem?.name,
    ...(path.conditioning || []).map((component) => `${component.objectType || ""} ${component.objectName || ""}`),
    ...(path.supportingCouplingIds || []),
  ];
}

function couplingMatchesQuery(coupling, query) {
  if (!query) {
    return true;
  }
  return [
    coupling.id,
    coupling.couplingType,
    coupling.role,
    coupling.object?.objectType,
    coupling.object?.objectName,
    coupling.object?.displayName,
    ...(coupling.connectedLoops || []).map((loop) => `${loop.type} ${loop.name}`),
    ...(coupling.mediums || []),
  ]
    .join(" ")
    .toLowerCase()
    .includes(query);
}

function serviceEdgeMedium(from, to, path) {
  if (from.kind === "refrigerant_system" || to.kind === "vrf_indoor" || path.pathType === "direct_zone_refrigerant") {
    return "refrigerant";
  }
  if (from.kind === "plant_loop" || to.kind === "plant_loop" || from.role === "conditioning") {
    return path.serviceKind === "heating" || path.serviceKind === "radiant_heating" ? "hot_water" : "chilled_water";
  }
  if (from.kind === "source" && path.sourceSystem?.mediums?.length) {
    return path.sourceSystem.mediums[0];
  }
  return "air";
}

function serviceEdgeLabel(medium = "") {
  const labels = {
    air: "Air",
    chilled_water: "CHW",
    hot_water: "HW",
    condenser_water: "CW",
    refrigerant: "Refrigerant",
    electricity: "Electric",
    fuel: "Fuel",
    service_water: "Water",
    control: "Control",
    fault: "Fault",
  };
  return labels[medium] || titleCaseToken(medium || "");
}

function mediumClass(medium = "") {
  return `medium-${String(medium || "air").replace(/_/g, "-")}`;
}

function iconKindForDelivery(deliveryType = "") {
  switch (deliveryType) {
    case "air_terminal":
    case "vav_reheat_terminal":
    case "constant_volume_terminal":
    case "fan_powered_terminal":
    case "adu":
      return "terminal";
    case "fan_coil":
      return "fan_coil";
    case "ptac":
    case "pthp":
      return "packaged";
    case "vrf_indoor":
      return "refrigerant";
    case "radiant_floor":
    case "radiant_panel":
      return "radiant";
    case "baseboard":
      return "baseboard";
    case "ideal_loads":
      return "ideal_loads";
    case "unit_heater":
      return "boiler";
    case "evaporative_cooler":
    case "unit_ventilator":
    case "erv":
      return "air";
    default:
      return "direct_unit";
  }
}

function iconKindForSource(system = {}) {
  const type = String(system.type || "").toLowerCase();
  if (type.includes("dx") || type.includes("refrigerant")) {
    return "refrigerant";
  }
  if (type.includes("electric")) {
    return "electric";
  }
  if (type.includes("ideal")) {
    return "ideal_loads";
  }
  return "plant";
}

function couplingRoleLabel(coupling = {}) {
  return [titleCaseToken(coupling.role || ""), titleCaseToken(coupling.couplingType || "")].filter(Boolean).join(" / ");
}

function couplingShortLabel(coupling = {}) {
  const role = coupling.role || coupling.couplingType || "";
  if (role.includes("storage")) {
    return "Storage";
  }
  if (role.includes("tower")) {
    return "Tower";
  }
  if (role.includes("pv")) {
    return "PV";
  }
  if (role.includes("fuel_cell")) {
    return "Fuel Cell";
  }
  if (role.includes("water")) {
    return "Water";
  }
  if (role.includes("control") || role.includes("manager")) {
    return "Control";
  }
  return titleCaseToken(role || "Support");
}

function iconKindForCoupling(coupling = {}) {
  const type = coupling.couplingType || "";
  const role = coupling.role || "";
  if (type === "thermal_storage" || type === "electric_storage") {
    return "storage";
  }
  if (type === "generator") {
    return role === "pv" ? "pv" : "generator";
  }
  if (type === "heat_rejection") {
    return "tower";
  }
  if (type === "heat_recovery") {
    return "heat_exchanger";
  }
  if (type === "service_water") {
    return "water";
  }
  if (type === "control_overlay" || type === "operation_scheme") {
    return "control";
  }
  if (type === "fault_overlay") {
    return "fault";
  }
  return "component";
}

function renderHVACTraceDrawer(traceIds = []) {
  const traces = [...new Set(traceIds || [])].filter(Boolean);
  if (!traces.length) {
    return "";
  }
  return `
    <details class="hvac-trace-drawer">
      <summary>${escapeHTML(t("hvac.showTrace", {}, "Show trace"))}</summary>
      <div class="hvac-detail-list hvac-rule-trace-list">
        ${traces.map((trace) => `<div><strong>${escapeHTML(trace)}</strong><span>${escapeHTML(t("hvac.debugTraceItem", {}, "debug trace"))}</span></div>`).join("")}
      </div>
    </details>`;
}

function renderHVACServices(hvac, query) {
  const summaries = (hvacServiceModel(hvac).zoneServices || []).filter((summary) => zoneServiceMatchesQuery(summary, query));
  const scopedIDs = new Set(pathsForActiveHVACEntity(hvac).map((path) => path.id));
  const paths = summaries
    .flatMap((summary) => summary.paths || [])
    .filter((path) => hvacGraphScopeMode() === "all" || !state.activeHVACEntity?.id || scopedIDs.has(path.id))
    .filter(servicePathMatchesQuickFilters)
    .filter((path) => servicePathMatchesQuery(path, query));
  elements.hvacGraph.innerHTML = paths.length
    ? `
      ${renderHVACServiceGraph(paths, hvacServiceModel(hvac).couplings || [])}
      ${renderHVACServiceGraphDetail(paths, hvacServiceModel(hvac).couplings || [])}`
    : `<div class="empty">${t("hvac.noMatchingServices", {}, "No matching zone services")}</div>`;
}

function renderHVACServiceGraph(paths, couplings) {
  const graph = cachedServiceGraph(paths, couplings);
  const width = graph.width;
  const height = graph.height;
  return `
    <div class="hvac-graphic-shell hvac-service-shell ${hvacGraphScaleClass()}" style="--hvac-graph-width: ${width}px">
      <div class="hvac-graph-toolbar">
        ${renderHVACQuickFilters()}
        ${renderHVACGraphScopeControls()}
        ${renderHVACGraphScaleControls()}
      </div>
      <svg class="hvac-service-svg" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(t("hvac.zoneServices", {}, "Zone Services"))}">
        <defs>
          <marker id="hvacServiceArrow" markerWidth="10" markerHeight="10" refX="8" refY="3" orient="auto" markerUnits="strokeWidth">
            <path d="M0,0 L8,3 L0,6 Z" class="hvac-arrow-marker"></path>
          </marker>
        </defs>
        ${graph.groups.map(renderServiceLaneBand).join("")}
        ${graph.links.map(renderServiceLink).join("")}
        ${graph.nodes.map(renderServiceNode).join("")}
      </svg>
      <div class="hvac-legend">
        <span><i class="hvac-legend-air"></i>Air</span>
        <span><i class="hvac-legend-chilled-water"></i>CHW</span>
        <span><i class="hvac-legend-hot-water"></i>HW</span>
        <span><i class="hvac-legend-refrigerant"></i>Refrigerant</span>
        <span><i class="hvac-legend-electricity"></i>Electric</span>
        <span><i class="hvac-legend-control"></i>Control</span>
      </div>
    </div>`;
}

function cachedServiceGraph(paths, couplings) {
  const key = serviceGraphLayoutCacheKey(paths, couplings);
  const cache = state.hvacServiceGraphLayoutCache || new Map();
  state.hvacServiceGraphLayoutCache = cache;
  if (cache.has(key)) {
    const graph = cache.get(key);
    cache.delete(key);
    cache.set(key, graph);
    return graph;
  }
  const graph = buildServiceGraph(paths, couplings);
  cache.set(key, graph);
  while (cache.size > 8) {
    cache.delete(cache.keys().next().value);
  }
  return graph;
}

function serviceGraphLayoutCacheKey(paths = [], couplings = []) {
  const pathKey = paths.map((path) => path.id || servicePathGraphKey(path)).sort().join(",");
  const couplingKey = (couplings || [])
    .filter(isPhysicalServiceCoupling)
    .map((coupling) => coupling.id || coupling.object?.objectName || coupling.role || "")
    .sort()
    .join(",");
  return [
    state.analysisKey || state.lastAnalyzedKey || "",
    state.activeHVACGraphScope || "",
    state.hvacServiceKindFilter || "",
    state.hvacPathTypeFilter || "",
    state.hvacMediumFilter || "",
    state.activeHVACEntity?.id || "",
    pathKey,
    couplingKey,
  ].join("|");
}

function buildServiceGraph(paths, couplings) {
  const links = [];
  const nodeByKey = new Map();
  const couplingById = new Map((couplings || []).map((coupling) => [coupling.id, coupling]));
  const sortedPaths = sortServiceGraphPaths(paths);
  const width = 1480;
  const addNode = (path, spec) => {
    const key = serviceGraphNodeKey(path, spec);
    let node = nodeByKey.get(key);
    if (!node) {
      node = {
        key,
        role: spec.role,
        kind: spec.kind,
        x: serviceNodeX(spec.role, 0, 1, width),
        y: 0,
        width: spec.width || serviceNodeWidth(spec.role),
        height: 62,
        label: spec.label,
        meta: spec.meta,
        iconKind: spec.iconKind,
        shortLabel: spec.shortLabel,
        ref: spec.ref,
        path,
        paths: [],
        relatedKeys: [],
        ports: graphPortsForNode(spec.kind),
        sortKey: serviceGraphNodeSortKey(path, spec),
      };
      nodeByKey.set(key, node);
    }
    node.paths.push(path);
    node.relatedKeys = appendUniqueString(node.relatedKeys || [], servicePathGraphKey(path));
    node.relatedKeys = appendUniqueString(node.relatedKeys || [], `subject:${servedSubjectKey(path.servedSubject || path)}`);
    return node;
  };
  const addCouplingNode = (path, coupling) => {
    const key = `coupling-node:any:${coupling.id}`;
    let node = nodeByKey.get(key);
    if (!node) {
      node = {
        key,
        role: "coupling",
        kind: coupling.couplingType || "coupling",
        x: serviceNodeX("coupling", 0, 1, width),
        y: 0,
        width: 166,
        height: 50,
        label: coupling.object?.displayName || coupling.object?.objectName || coupling.role,
        meta: couplingRoleLabel(coupling),
        iconKind: iconKindForCoupling(coupling),
        shortLabel: couplingShortLabel(coupling),
        coupling,
        path,
        paths: [],
        relatedKeys: [],
        ports: graphPortsForNode("support"),
        sortKey: serviceGraphCouplingSortKey(coupling),
      };
      nodeByKey.set(key, node);
    }
    node.paths.push(path);
    node.relatedKeys = appendUniqueString(node.relatedKeys || [], servicePathGraphKey(path));
    return node;
  };
  const addLink = (from, to, path, support = false, coupling = null) => {
    const medium = support ? (coupling?.mediums || [])[0] || "control" : serviceEdgeMedium(from, to, path);
    links.push({
      key: `${support ? "support-link" : "service-link"}:${from.key}->${to.key}:${medium}`,
      from,
      to,
      medium,
      label: serviceEdgeLabel(medium),
      path,
      paths: [path],
      coupling,
      support,
      relatedKeys: [servicePathGraphKey(path), `subject:${servedSubjectKey(path.servedSubject || path)}`, from.key, to.key],
    });
  };
  sortedPaths.forEach((path) => {
    const specs = servicePathNodeSpecs(path);
    const rowNodes = specs.map((spec) => addNode(path, spec));
    rowNodes.forEach((node, nodeIndex) => {
      const next = rowNodes[nodeIndex + 1];
      if (next) {
        addLink(node, next, path);
      }
    });
    const plantNode = rowNodes.find((node) => ["plant_loop", "source", "refrigerant_system"].includes(node.role));
    physicalSupportingCouplings(path, couplingById).forEach((coupling) => {
      const anchor = plantNode || rowNodes[0];
      if (anchor) {
        addLink(addCouplingNode(path, coupling), anchor, path, true, coupling);
      }
    });
  });
  const nodes = [...nodeByKey.values()];
  const bundledLinks = bundleServiceGraphLinks(links);
  const height = layoutServiceGraphNodes(nodes, bundledLinks);
  return { width, height, nodes, links: bundledLinks, groups: serviceGraphColumnGroups(nodes, height) };
}

function sortServiceGraphPaths(paths = []) {
  return [...paths].sort((a, b) => serviceGraphPathSortKey(a).localeCompare(serviceGraphPathSortKey(b), undefined, { numeric: true }));
}

function serviceGraphPathSortKey(path = {}) {
  return [
    servedSubjectLabel(path.servedSubject || path),
    String(deliverySortRank(path)).padStart(2, "0"),
    deliveryLabel(path),
    path.delivery?.objectName || path.delivery?.displayName || "",
    path.airLoop?.name || "",
    serviceKindSortRank(path.serviceKind),
    path.plantLoop?.name || path.refrigerantSystem?.name || path.sourceSystem?.name || "",
    path.pathType || "",
  ]
    .join("|")
    .toLowerCase();
}

function deliverySortRank(path = {}) {
  const deliveryType = path.deliveryEquipment?.deliveryType || path.delivery?.deliveryType || "";
  const ranks = {
    vav_reheat_terminal: 10,
    air_terminal: 11,
    constant_volume_terminal: 12,
    fan_powered_terminal: 13,
    adu: 14,
    fan_coil: 20,
    unit_ventilator: 21,
    unit_heater: 22,
    water_to_air_heat_pump: 23,
    radiant_floor: 30,
    radiant_panel: 31,
    baseboard: 32,
    vrf_indoor: 40,
    ptac: 50,
    pthp: 51,
    window_ac: 52,
    evaporative_cooler: 53,
    ideal_loads: 60,
  };
  return ranks[deliveryType] || 90;
}

function serviceKindSortRank(kind = "") {
  const ranks = {
    cooling: "10",
    heating: "20",
    ventilation: "30",
    exhaust: "40",
    radiant_cooling: "50",
    radiant_heating: "51",
  };
  return ranks[kind] || "90";
}

function appendUniqueString(values = [], value = "") {
  if (value && !values.includes(value)) {
    values.push(value);
  }
  return values;
}

const serviceGraphColumns = [
  { id: "source", label: "Source", x: 146, width: 210, roles: ["source", "plant_loop", "refrigerant_system"] },
  { id: "support", label: "Physical support", x: 382, width: 186, roles: ["coupling"] },
  { id: "conditioning", label: "Conditioning", x: 608, width: 194, roles: ["conditioning"] },
  { id: "air_loop", label: "Air loop", x: 832, width: 194, roles: ["air_loop"] },
  { id: "delivery", label: "Delivery", x: 1086, width: 214, roles: ["delivery"] },
  { id: "zone", label: "Zone", x: 1340, width: 196, roles: ["zone"] },
];

function serviceGraphColumnForRole(role = "") {
  return serviceGraphColumns.find((column) => column.roles.includes(role)) || serviceGraphColumns[serviceGraphColumns.length - 1];
}

function layoutServiceGraphNodes(nodes = [], links = []) {
  const top = 116;
  const rowGap = 118;
  let maxRow = 0;
  for (const column of serviceGraphColumns) {
    const columnNodes = serviceGraphColumnNodes(nodes, column);
    columnNodes.forEach((node, index) => {
      node.x = column.x;
      node.y = top + index * rowGap;
      node.width = Math.min(node.width || column.width, column.width);
      maxRow = Math.max(maxRow, index);
    });
  }
  for (const column of serviceGraphColumns.slice(1)) {
    maxRow = Math.max(maxRow, alignServiceGraphColumnRows(nodes, links, column, top, rowGap));
  }
  return Math.max(300, top + maxRow * rowGap + 108);
}

function serviceGraphColumnNodes(nodes = [], column = {}) {
  return nodes.filter((node) => column.roles?.includes(node.role)).sort(compareServiceGraphNodes);
}

function compareServiceGraphNodes(left, right) {
  return String(left.sortKey || left.label || "").localeCompare(String(right.sortKey || right.label || ""), undefined, { numeric: true });
}

function alignServiceGraphColumnRows(nodes = [], links = [], column = {}, top = 0, rowGap = 96) {
  const columnNodes = serviceGraphColumnNodes(nodes, column);
  const usedRows = new Set();
  columnNodes
    .map((node, index) => ({
      node,
      naturalRow: index,
      desiredRow: serviceGraphDesiredRow(node, links, top, rowGap, index),
    }))
    .sort((left, right) => left.desiredRow - right.desiredRow || compareServiceGraphNodes(left.node, right.node))
    .forEach(({ node, desiredRow, naturalRow }) => {
      const row = nearestFreeServiceGraphRow(Number.isFinite(desiredRow) ? desiredRow : naturalRow, usedRows);
      usedRows.add(row);
      node.y = top + row * rowGap;
    });
  return usedRows.size ? Math.max(...usedRows) : 0;
}

function serviceGraphDesiredRow(node, links = [], top = 0, rowGap = 96, fallbackRow = 0) {
  const relatedY = links.flatMap((link) => {
    if (link.from === node && link.to) {
      return [link.to.y];
    }
    if (link.to === node && link.from) {
      return [link.from.y];
    }
    return [];
  });
  if (!relatedY.length) {
    return fallbackRow;
  }
  const averageY = relatedY.reduce((sum, value) => sum + value, 0) / relatedY.length;
  return Math.max(0, Math.round((averageY - top) / rowGap));
}

function nearestFreeServiceGraphRow(preferredRow, usedRows = new Set()) {
  const start = Math.max(0, Math.round(preferredRow));
  for (let distance = 0; distance < usedRows.size + 64; distance += 1) {
    const before = start - distance;
    if (before >= 0 && !usedRows.has(before)) {
      return before;
    }
    const after = start + distance;
    if (!usedRows.has(after)) {
      return after;
    }
  }
  return usedRows.size;
}

function serviceGraphColumnGroups(nodes = [], height = 300) {
  const usedRoles = new Set(nodes.map((node) => node.role));
  return serviceGraphColumns
    .filter((column) => column.roles.some((role) => usedRoles.has(role)))
    .map((column) => ({
      x: column.x - column.width / 2 - 8,
      y: 28,
      width: column.width + 16,
      height: height - 56,
      label: column.label,
      column: true,
    }));
}

function serviceGraphNodeSortKey(path = {}, spec = {}) {
  return [
    String(serviceGraphColumnForRole(spec.role).x).padStart(4, "0"),
    spec.role === "delivery" ? String(deliverySortRank(path)).padStart(2, "0") : "00",
    spec.role === "zone" ? servedSubjectLabel(path.servedSubject || path) : "",
    spec.meta || "",
    spec.label || "",
  ]
    .join("|")
    .toLowerCase();
}

function serviceGraphCouplingSortKey(coupling = {}) {
  return [coupling.couplingType || "", coupling.role || "", coupling.object?.objectName || coupling.object?.displayName || ""].join("|").toLowerCase();
}

function physicalSupportingCouplings(path = {}, couplingById = new Map()) {
  return (path.supportingCouplingIds || [])
    .map((id) => couplingById.get(id))
    .filter((coupling) => isPhysicalServiceCoupling(coupling));
}

function isPhysicalServiceCoupling(coupling = {}) {
  if (!coupling || coupling.placementHint === "detail_only") {
    return false;
  }
  const type = coupling.couplingType || "";
  if (type === "operation_scheme" || type === "control_overlay" || type === "fault_overlay") {
    return false;
  }
  return ["thermal_storage", "electric_storage", "generator", "heat_rejection", "heat_recovery", "service_water", "source_sink"].includes(type);
}

function bundleServiceGraphLinks(links = []) {
  const bundled = new Map();
  for (const link of links) {
    const key = serviceLinkBundleKey(link);
    const current = bundled.get(key);
    if (current) {
      current.bundleCount += 1;
      current.paths = [...(current.paths || []), ...(link.paths || [link.path]).filter(Boolean)];
      current.relatedKeys = [...new Set([...(current.relatedKeys || []), ...(link.relatedKeys || [])])];
      continue;
    }
    bundled.set(key, {
      ...link,
      bundleKey: key,
      bundleCount: 1,
      paths: (link.paths || [link.path]).filter(Boolean),
    });
  }
  return [...bundled.values()];
}

function serviceLinkBundleKey(link = {}) {
  return [
    link.support ? "support" : "main",
    link.from?.role || "",
    link.from?.ref?.id || link.from?.label || "",
    link.to?.role || "",
    link.to?.ref?.id || link.to?.label || "",
    link.medium || "",
  ].join("|");
}

function servicePathNodeSpecs(path) {
  const deliveryRef = path.delivery || {};
  const conditioning = (path.conditioning || [])[0] || null;
  const zoneSpec = {
    role: "zone",
    kind: path.servedSubject?.kind === "space" ? "space" : "zone",
    label: servedSubjectLabel(path.servedSubject || path),
    meta: path.servedSubject?.kind === "space" ? "Space" : "Zone",
    iconKind: "zone",
    shortLabel: servedSubjectLabel(path.servedSubject || path),
    ref: path.servedSubject,
  };
  const deliverySpec = {
    role: "delivery",
    kind: path.deliveryEquipment?.deliveryType || deliveryRef.deliveryType || "direct_zone_unit",
    label: componentRefLabel(deliveryRef, deliveryLabel(path)),
    meta: componentRefMeta(deliveryRef, path.deliveryEquipment?.displayFamily || deliveryRef.displayFamily || ""),
    iconKind: iconKindForDelivery(path.deliveryEquipment?.deliveryType || path.delivery?.deliveryType),
    shortLabel: componentRefLabel(deliveryRef, deliveryLabel(path)),
    ref: path.delivery,
  };
  const plantSpec = path.plantLoop
    ? {
        role: "plant_loop",
        kind: "plant_loop",
        label: path.plantLoop.name,
        meta: "PlantLoop",
        iconKind: "plant",
        shortLabel: path.plantLoop.name,
        ref: path.plantLoop,
      }
    : path.sourceSystem
      ? {
          role: "source",
          kind: path.sourceSystem.type || "source",
          label: systemRefLabel(path.sourceSystem),
          meta: systemRefMeta(path.sourceSystem, sourceSystemLabel(path.sourceSystem)),
          iconKind: iconKindForSource(path.sourceSystem),
          shortLabel: systemRefLabel(path.sourceSystem),
          ref: path.sourceSystem,
        }
      : null;
  const airSpec = path.airLoop
    ? {
        role: "air_loop",
        kind: "air_loop",
        label: path.airLoop.name,
        meta: "AirLoopHVAC",
        iconKind: "air",
        shortLabel: path.airLoop.name,
        ref: path.airLoop,
      }
    : null;
  const refrigerantSpec = path.refrigerantSystem
    ? {
        role: "refrigerant_system",
        kind: "refrigerant_system",
        label: systemRefLabel(path.refrigerantSystem),
        meta: systemRefMeta(path.refrigerantSystem, "Refrigerant System"),
        iconKind: "refrigerant",
        shortLabel: systemRefLabel(path.refrigerantSystem),
        ref: path.refrigerantSystem,
      }
    : null;
  const conditioningSpec = conditioning
    ? {
        role: "conditioning",
        kind: "coil",
        label: componentRefLabel(conditioning, "Conditioning"),
        meta: componentRefMeta(conditioning, conditioning.displayFamily || "Conditioning"),
        iconKind: "coil",
        shortLabel: componentRefLabel(conditioning, "Conditioning"),
        ref: conditioning,
      }
    : null;
  switch (path.pathType) {
    case "central_air_with_plant":
      return [plantSpec, conditioningSpec, airSpec, deliverySpec, zoneSpec].filter(Boolean);
    case "central_air":
      return [airSpec, deliverySpec, zoneSpec].filter(Boolean);
    case "direct_zone_hydronic":
    case "radiant":
    case "baseboard":
      return [plantSpec, deliverySpec, zoneSpec].filter(Boolean);
    case "direct_zone_refrigerant":
      return [refrigerantSpec || plantSpec, deliverySpec, zoneSpec].filter(Boolean);
    case "ideal_loads":
    case "local":
      return [plantSpec, deliverySpec, zoneSpec].filter(Boolean);
    case "ventilation_only":
    case "exhaust_only":
    case "direct_zone_air":
    default:
      return [plantSpec, airSpec, deliverySpec, zoneSpec].filter(Boolean);
  }
}

function componentRefLabel(ref = {}, fallback = "") {
  return ref.displayName || ref.objectName || ref.name || fallback || "";
}

function componentRefMeta(ref = {}, fallback = "") {
  return ref.objectType || ref.displayFamily || ref.family || fallback || "";
}

function systemRefLabel(ref = {}) {
  return ref.displayName || ref.objectName || ref.name || ref.type || "";
}

function systemRefMeta(ref = {}, fallback = "") {
  return ref.objectType || ref.type || fallback || "";
}

function serviceNodeX(role, index, count, width) {
  const column = serviceGraphColumnForRole(role);
  if (column) {
    return column.x;
  }
  if (count <= 1) {
    return width / 2;
  }
  const left = 110;
  const right = width - 110;
  return left + ((right - left) * index) / (count - 1);
}

function serviceNodeWidth(role) {
  switch (role) {
    case "zone":
      return 164;
    case "delivery":
      return 178;
    case "air_loop":
      return 172;
    case "conditioning":
      return 174;
    default:
      return 170;
  }
}

function graphPortsForNode(kind) {
  const base = [
    { id: "in", side: "left", medium: "air" },
    { id: "out", side: "right", medium: "air" },
    { id: "support", side: "bottom", medium: "control" },
    { id: "target", side: "top", medium: "control" },
  ];
  if (kind === "plant_loop") {
    return [
      { id: "in", side: "left", medium: "chilled_water" },
      { id: "out", side: "right", medium: "chilled_water" },
      { id: "support", side: "bottom", medium: "chilled_water" },
      { id: "target", side: "top", medium: "control" },
    ];
  }
  if (kind === "refrigerant_system") {
    return [
      { id: "out", side: "right", medium: "refrigerant" },
      { id: "support", side: "bottom", medium: "refrigerant" },
    ];
  }
  if (kind === "support") {
    return [
      { id: "out", side: "right", medium: "control" },
      { id: "target", side: "top", medium: "control" },
    ];
  }
  return base;
}

function renderServiceLaneBand(group) {
  return `
    <g class="hvac-service-lane ${group.column ? "column" : ""}">
      <rect x="${group.x ?? 18}" y="${group.y}" width="${group.width || 1124}" height="${group.height}" rx="6"></rect>
      <text x="${(group.x ?? 18) + 14}" y="${group.y + 22}">${escapeHTML(group.label)}</text>
    </g>`;
}

function renderServiceNode(node) {
  const visualClass = `${node.kind || "component"} ${graphSelectionClass(node.key, node.relatedKeys || [])}`;
  return renderGraphNode({
    key: node.key,
    x: node.x,
    y: node.y,
    width: node.width,
    height: node.height,
    label: node.label,
    meta: node.meta,
    iconKind: node.iconKind,
    shortLabel: node.shortLabel,
    className: visualClass,
    tooltip: [node.label, node.meta, node.ref?.objectType].filter(Boolean).join(" - "),
  });
}

function renderServiceLink(link) {
  const selected = graphSelectionClass(link.key, link.relatedKeys || []);
  const fromPort = graphPortPoint(link.from, link.support ? "out" : "out");
  const toPort = graphPortPoint(link.to, link.support ? "target" : "in");
  const d = serviceLinkPath(fromPort, toPort, link);
  const medium = mediumClass(link.medium);
  const bundleCount = Number(link.bundleCount || 0);
  const label = bundleCount > 1 ? `${link.label || ""} x${bundleCount}` : link.label || "";
  const labelPoint = serviceLinkLabelPoint(fromPort, toPort, link);
  const labelX = labelPoint.x;
  const labelY = labelPoint.y;
  return `
    <g class="hvac-service-link-group ${bundleCount > 1 ? "bundled" : ""} ${selected}">
      <path class="hvac-graph-link service ${medium} ${link.support ? "support" : ""} ${bundleCount > 1 ? "bundled" : ""} ${selected}"
        data-hvac-graph-key="${escapeHTML(link.key)}"
        d="${escapeHTML(d)}"
        marker-end="url(#hvacServiceArrow)">
        <title>${escapeHTML(`${link.from.label || ""} -> ${link.to.label || ""} / ${label}`)}</title>
      </path>
      <text class="hvac-edge-label" x="${labelX}" y="${labelY}">${escapeHTML(label)}</text>
      ${
        bundleCount > 1
          ? `<g class="hvac-edge-bundle-badge" data-hvac-graph-key="${escapeHTML(link.key)}">
              <rect x="${labelX - 13}" y="${labelY + 10}" width="26" height="18" rx="9"></rect>
              <text x="${labelX}" y="${labelY + 23}" text-anchor="middle">${escapeHTML(bundleCount)}</text>
            </g>`
          : ""
      }
    </g>`;
}

function graphPortPoint(node, portId) {
  const port = (node.ports || []).find((item) => item.id === portId) || (node.ports || [])[0] || { side: "right" };
  switch (port.side) {
    case "left":
      return { x: node.x - node.width / 2, y: node.y };
    case "right":
      return { x: node.x + node.width / 2, y: node.y };
    case "top":
      return { x: node.x, y: node.y - node.height / 2 };
    case "bottom":
      return { x: node.x, y: node.y + node.height / 2 };
    default:
      return { x: node.x, y: node.y };
  }
}

function serviceLinkPath(from, to, link = {}) {
  if (link.support) {
    return grasshopperWirePath(from, to, link, { support: true });
  }
  return grasshopperWirePath(from, to, link);
}

function grasshopperWirePath(from, to, link = {}, options = {}) {
  const curve = serviceLinkCurve(from, to, link, options);
  return `M${graphCoord(curve.from.x)},${graphCoord(curve.from.y)} C${graphCoord(curve.c1.x)},${graphCoord(curve.c1.y)} ${graphCoord(curve.c2.x)},${graphCoord(curve.c2.y)} ${graphCoord(curve.to.x)},${graphCoord(curve.to.y)}`;
}

function serviceLinkCurve(from, to, link = {}, options = {}) {
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  const direction = dx >= 0 ? 1 : -1;
  const absDx = Math.abs(dx);
  const absDy = Math.abs(dy);
  const support = Boolean(options.support || link.support);
  const laneOffset = serviceLinkLaneOffset(link) * (support ? 0.75 : 1);
  if (absDx < 36 && absDy < 36) {
    const loop = 28 + Math.abs(laneOffset);
    return {
      from,
      c1: { x: from.x + direction * loop, y: from.y + laneOffset },
      c2: { x: to.x - direction * loop, y: to.y + laneOffset },
      to,
    };
  }
  const tension = support ? clampGraphValue(absDx * 0.5, 58, 136) : clampGraphValue(absDx * 0.46, 74, 190);
  const verticalEase = support ? clampGraphValue(absDy * 0.18, 10, 42) * (dy >= 0 ? 1 : -1) : clampGraphValue(dy * 0.08, -24, 24);
  return {
    from,
    c1: {
      x: from.x + direction * tension,
      y: from.y + laneOffset + verticalEase,
    },
    c2: {
      x: to.x - direction * tension,
      y: to.y + laneOffset - verticalEase,
    },
    to,
  };
}

function serviceLinkLaneOffset(link = {}) {
  const paths = (link.paths || [link.path]).filter(Boolean);
  const seed = paths.length ? paths.map((path) => path.id || servicePathGraphKey(path)).sort().join("|") : link.key || link.bundleKey || "";
  const bucket = (stableGraphHash(seed) % 7) - 3;
  const mediumBias = {
    air: 0,
    chilled_water: -8,
    hot_water: 8,
    condenser_water: -13,
    refrigerant: 13,
    electricity: 18,
    fuel: 18,
    service_water: -18,
    control: 20,
  };
  const bundleLift = Number(link.bundleCount || 0) > 1 ? Math.min(10, Number(link.bundleCount || 0) * 1.5) : 0;
  return clampGraphValue(bucket * 4 + (mediumBias[link.medium] || 0) + bundleLift, -26, 26);
}

function stableGraphHash(value = "") {
  let hash = 0;
  for (let index = 0; index < String(value).length; index += 1) {
    hash = (hash * 31 + String(value).charCodeAt(index)) >>> 0;
  }
  return hash;
}

function cubicPoint(curve, t) {
  const u = 1 - t;
  const tt = t * t;
  const uu = u * u;
  const uuu = uu * u;
  const ttt = tt * t;
  return {
    x: uuu * curve.from.x + 3 * uu * t * curve.c1.x + 3 * u * tt * curve.c2.x + ttt * curve.to.x,
    y: uuu * curve.from.y + 3 * uu * t * curve.c1.y + 3 * u * tt * curve.c2.y + ttt * curve.to.y,
  };
}

function clampGraphValue(value, min, max) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return min;
  }
  return Math.min(max, Math.max(min, number));
}

function graphCoord(value) {
  const number = Number(value);
  return Number.isFinite(number) ? Number(number.toFixed(1)) : 0;
}

function orthogonalPath(from, to, options = {}) {
  const sameX = Math.abs(to.x - from.x) < 4;
  const sameY = Math.abs(to.y - from.y) < 4;
  if (sameX || sameY) {
    return `M${from.x},${from.y} L${to.x},${to.y}`;
  }
  if (Math.abs(to.x - from.x) < 42 && Math.abs(to.y - from.y) < 42) {
    const loop = options.offset || 28;
    return `M${from.x},${from.y} h${loop} v${to.y - from.y} H${to.x}`;
  }
  if (options.verticalFirst) {
    const direction = to.y >= from.y ? 1 : -1;
    const midY = from.y + direction * Math.max(options.offset || 30, Math.min(92, Math.abs(to.y - from.y) * 0.45));
    return `M${from.x},${from.y} V${midY} H${to.x} V${to.y}`;
  }
  const midX = from.x + (to.x - from.x) / 2;
  return `M${from.x},${from.y} H${midX} V${to.y} H${to.x}`;
}

function serviceLinkLabelPoint(from, to, link = {}) {
  const point = cubicPoint(serviceLinkCurve(from, to, link, { support: link.support }), 0.58);
  return { x: graphCoord(point.x), y: graphCoord(point.y - (link.support ? 15 : 10)) };
}

function renderHVACServiceGraphDetail(paths, couplings) {
  const selected = selectedServiceGraphItem(paths, couplings);
  if (!selected) {
    const zones = new Set(paths.map((path) => servedSubjectKey(path.servedSubject || path))).size;
    const deliveryTypes = new Set(paths.map((path) => path.deliveryEquipment?.deliveryType || path.delivery?.deliveryType).filter(Boolean)).size;
    const physicalCouplings = (couplings || []).filter((coupling) => isPhysicalServiceCoupling(coupling)).length;
    return `
      <section class="hvac-graph-detail">
        <div class="hvac-section-head">
          <h3>${escapeHTML(t("hvac.servicePathDetail", {}, "Service Path Detail"))}</h3>
          <span>${escapeHTML(t("hvac.relationHint"))}</span>
        </div>
        <div class="hvac-detail-grid">
          <div><span>Zones</span><strong>${escapeHTML(zones)}</strong></div>
          <div><span>Service paths</span><strong>${escapeHTML(paths.length)}</strong></div>
          <div><span>${escapeHTML(t("hvac.delivery", {}, "Delivery"))}</span><strong>${escapeHTML(deliveryTypes)}</strong></div>
          <div><span>${escapeHTML(t("hvac.couplings", {}, "Couplings"))}</span><strong>${escapeHTML(physicalCouplings)}</strong></div>
        </div>
      </section>`;
  }
  if (selected.kind === "path") {
    return renderSelectedServicePathDetail(selected.path, couplings);
  }
  if (selected.kind === "coupling") {
    return renderSelectedCouplingDetail(selected.coupling, selected.path);
  }
  if (selected.kind === "node") {
    return renderSelectedServiceNodeDetail(selected.node, selected.path, couplings);
  }
  if (selected.kind === "link") {
    return renderSelectedServicePathDetail(selected.path, couplings);
  }
  return "";
}

function selectedServiceGraphItem(paths, couplings) {
  const key = state.activeHVACGraphKey || "";
  if (!key) {
    return null;
  }
  const couplingById = new Map((couplings || []).map((coupling) => [coupling.id, coupling]));
  if (key.startsWith("service-path:")) {
    const path = paths.find((item) => servicePathGraphKey(item) === key);
    return path ? { kind: "path", path } : null;
  }
  if (key.startsWith("subject:")) {
    const subject = key.slice("subject:".length);
    const subjectPaths = paths.filter((path) => servedSubjectKey(path.servedSubject || path) === subject);
    return subjectPaths[0] ? { kind: "path", path: subjectPaths[0], paths: subjectPaths } : null;
  }
  if (key.startsWith("coupling-node:")) {
    const raw = key.slice("coupling-node:".length);
    let path = null;
    let couplingID = raw;
    if (raw.startsWith("any:")) {
      couplingID = raw.slice("any:".length);
    } else {
      path = paths.find((item) => raw.startsWith(`${item.id}:`));
      if (path) {
        couplingID = raw.slice(path.id.length + 1);
      }
    }
    const coupling = couplingById.get(couplingID);
    return coupling ? { kind: "coupling", coupling, path } : null;
  }
  if (key.startsWith("service-node:")) {
    const matches = paths
      .map((path) => {
        const node = servicePathNodeSpecs(path).find((spec) => serviceGraphNodeKey(path, spec) === key);
        return node ? { path, node } : null;
      })
      .filter(Boolean);
    if (!matches.length) {
      return null;
    }
    return {
      kind: "node",
      node: { ...matches[0].node, key, paths: matches.map((match) => match.path) },
      path: matches[0].path,
      paths: matches.map((match) => match.path),
    };
  }
  if (key.startsWith("service-link:") || key.startsWith("support-link:")) {
    const matchingPaths = paths.filter((path) => servicePathLinkKeys(path, couplingById).has(key));
    return matchingPaths[0] ? { kind: "link", path: matchingPaths[0], paths: matchingPaths } : null;
  }
  return null;
}

function servicePathLinkKeys(path = {}, couplingById = new Map()) {
  const keys = new Set();
  const nodes = servicePathNodeSpecs(path).map((spec) => ({
    key: serviceGraphNodeKey(path, spec),
    role: spec.role,
    kind: spec.kind,
    ref: spec.ref,
    label: spec.label,
  }));
  nodes.forEach((node, index) => {
    const next = nodes[index + 1];
    if (!next) {
      return;
    }
    const medium = serviceEdgeMedium(node, next, path);
    keys.add(`service-link:${node.key}->${next.key}:${medium}`);
  });
  const anchor = nodes.find((node) => ["plant_loop", "source", "refrigerant_system"].includes(node.role)) || nodes[0];
  if (anchor) {
    for (const coupling of physicalSupportingCouplings(path, couplingById)) {
      const supportKey = `coupling-node:any:${coupling.id}`;
      const medium = (coupling.mediums || [])[0] || "control";
      keys.add(`support-link:${supportKey}->${anchor.key}:${medium}`);
    }
  }
  return keys;
}

function renderSelectedServicePathDetail(path, couplings) {
  const pathCouplings = (path.supportingCouplingIds || [])
    .map((id) => (couplings || []).find((coupling) => coupling.id === id))
    .filter((coupling) => isPhysicalServiceCoupling(coupling));
  return `
    <section class="hvac-graph-detail">
      <div class="hvac-section-head">
        <h3>${escapeHTML(servedSubjectLabel(path.servedSubject || path))}</h3>
        <span>${escapeHTML(`${serviceKindLabel(path.serviceKind)} / ${pathTypeLabel(path.pathType)}`)}</span>
      </div>
      <div class="hvac-detail-grid">
        <div><span>${escapeHTML(t("hvac.delivery", {}, "Delivery"))}</span><strong>${escapeHTML(deliveryLabel(path))}</strong></div>
        <div><span>${escapeHTML(t("hvac.connectedSystems", {}, "Connected systems"))}</span><strong>${escapeHTML(servicePathConnectedSystems(path).join(", ") || "N/A")}</strong></div>
        <div><span>${escapeHTML(t("common.inlet"))}</span><strong>${escapeHTML(path.delivery?.inletNode || "N/A")}</strong></div>
        <div><span>${escapeHTML(t("common.outlet"))}</span><strong>${escapeHTML(path.delivery?.outletNode || "N/A")}</strong></div>
      </div>
      ${pathCouplings.length ? `<div class="hvac-detail-list">${pathCouplings.map((coupling) => `<div><strong>${escapeHTML(coupling.object?.displayName || coupling.role || "")}</strong><span>${escapeHTML(couplingRoleLabel(coupling))}</span></div>`).join("")}</div>` : ""}
      ${renderHVACTraceDrawer(path.traceIds || [])}
    </section>`;
}

function renderSelectedServiceNodeDetail(node, path, couplings) {
  const ref = node.ref || {};
  const nodePaths = (node.paths || [path]).filter(Boolean);
  const connected = [...new Set(nodePaths.flatMap(servicePathConnectedSystems))];
  const traceIds = [...new Set(nodePaths.flatMap((item) => item.traceIds || []))];
  return `
    <section class="hvac-graph-detail">
      <div class="hvac-section-head">
        <h3>${escapeHTML(node.label || ref.displayName || ref.name || "")}</h3>
        <span>${escapeHTML([node.meta || ref.objectType || node.kind || "", nodePaths.length > 1 ? `${nodePaths.length} paths` : ""].filter(Boolean).join(" / "))}</span>
      </div>
      ${connected.length ? `<section class="hvac-connected-systems"><strong>${escapeHTML(t("hvac.connectedSystems", {}, "Connected systems"))}</strong><div class="hvac-connected-system-list">${connected.map((item) => `<span>${escapeHTML(item)}</span>`).join("")}</div></section>` : ""}
      <div class="hvac-detail-grid">
        <div><span>${escapeHTML(t("common.type"))}</span><strong>${escapeHTML(ref.objectType || ref.type || node.kind || "N/A")}</strong></div>
        <div><span>Role</span><strong>${escapeHTML(ref.role || node.role || "N/A")}</strong></div>
        <div><span>${escapeHTML(t("hvac.deliveryType", {}, "Delivery type"))}</span><strong>${escapeHTML(ref.deliveryType || path.deliveryEquipment?.deliveryType || "N/A")}</strong></div>
        <div><span>${escapeHTML(t("common.inlet"))}</span><strong>${escapeHTML(ref.inletNode || "N/A")}</strong></div>
        <div><span>${escapeHTML(t("common.outlet"))}</span><strong>${escapeHTML(ref.outletNode || "N/A")}</strong></div>
        <div><span>${escapeHTML(t("common.water"))}</span><strong>${escapeHTML([ref.waterInletNode, ref.waterOutletNode].filter(Boolean).join(" -> ") || "N/A")}</strong></div>
      </div>
      ${renderHVACTraceDrawer(traceIds)}
    </section>`;
}

function renderSelectedCouplingDetail(coupling, path) {
  const affectedPaths = path ? [path] : pathsForActiveHVACEntity().filter((item) => (item.supportingCouplingIds || []).includes(coupling.id));
  return `
    <section class="hvac-graph-detail">
      <div class="hvac-section-head">
        <h3>${escapeHTML(coupling.object?.displayName || coupling.object?.objectName || coupling.role || "")}</h3>
        <span>${escapeHTML(couplingRoleLabel(coupling))}</span>
      </div>
      <div class="hvac-detail-grid">
        <div><span>Role</span><strong>${escapeHTML(coupling.role || "N/A")}</strong></div>
        <div><span>${escapeHTML(t("hvac.couplingType", {}, "Coupling type"))}</span><strong>${escapeHTML(coupling.couplingType || "N/A")}</strong></div>
        <div><span>${escapeHTML(t("hvac.connectedSystems", {}, "Connected systems"))}</span><strong>${escapeHTML((coupling.connectedLoops || []).map((loop) => loop.name).join(", ") || "N/A")}</strong></div>
        <div><span>Medium</span><strong>${escapeHTML((coupling.mediums || []).join(", ") || "N/A")}</strong></div>
      </div>
      ${(coupling.connectedLoops || []).length ? `<section class="hvac-connected-systems"><strong>${escapeHTML(t("hvac.connectedSystems", {}, "Connected systems"))}</strong><div class="hvac-connected-system-list">${(coupling.connectedLoops || []).map((loop) => `<button class="${escapeHTML(hvacLoopChipClass(loop.type))}" type="button" data-hvac-entity-kind="loop" data-hvac-entity-id="${escapeHTML(navigationLoopEntityID(loop.type, loop.name))}" data-hvac-entity-label="${escapeHTML(loop.name)}">${escapeHTML(loop.name)}</button>`).join("")}</div></section>` : ""}
      ${renderRelatedServicePathList(affectedPaths)}
      ${path ? `<p class="hvac-detail-note">${escapeHTML(t("hvac.affectsPath", {}, "Affects selected service path"))}: ${escapeHTML(servedSubjectLabel(path.servedSubject || path))}</p>` : ""}
      ${renderHVACTraceDrawer(coupling.traceIds || [])}
    </section>`;
}

function renderHVACCouplings(hvac, query) {
  const serviceModel = hvacServiceModel(hvac);
  const couplings = (serviceModel.couplings || []).filter((coupling) => isPhysicalServiceCoupling(coupling)).filter((coupling) => couplingMatchesQuery(coupling, query));
  const networks = serviceModel.networks || [];
  elements.hvacGraph.innerHTML = couplings.length || networks.length
    ? `
      <section class="hvac-coupling-overview">
        <div class="hvac-section-head compact">
          <h3>${escapeHTML(t("hvac.couplings", {}, "Couplings"))}</h3>
          <span>${escapeHTML(couplings.length)}</span>
        </div>
        <div class="hvac-coupling-grid">
          ${couplings.map(renderHVACCouplingCard).join("") || `<div class="empty">${escapeHTML(t("hvac.noCouplings", {}, "No couplings"))}</div>`}
        </div>
      </section>
      ${networks.length ? `<section class="hvac-coupling-overview"><div class="hvac-section-head compact"><h3>${escapeHTML(t("hvac.networks", {}, "Networks"))}</h3><span>${escapeHTML(networks.length)}</span></div><div class="hvac-network-list">${networks.map(renderHVACNetworkCard).join("")}</div></section>` : ""}
      ${renderHVACServiceGraphDetail(servicePathsForHVAC(hvac), couplings)}`
    : `<div class="empty">${escapeHTML(t("hvac.noCouplings", {}, "No couplings"))}</div>`;
}

function renderHVACCouplingCard(coupling) {
  const key = `coupling-node:any:${coupling.id}`;
  return `
    <article class="hvac-coupling-card ${state.activeHVACGraphKey === key ? "selected" : ""}" data-hvac-graph-key="${escapeHTML(key)}" tabindex="0">
      <strong>${escapeHTML(coupling.object?.displayName || coupling.object?.objectName || coupling.role || "")}</strong>
      <span>${escapeHTML(couplingRoleLabel(coupling))}</span>
      <small>${escapeHTML((coupling.connectedLoops || []).map((loop) => loop.name).join(", ") || (coupling.mediums || []).join(", ") || "N/A")}</small>
    </article>`;
}

function renderHVACNetworkCard(network) {
  return `
    <article class="hvac-network-card">
      <strong>${escapeHTML(network.name || network.networkType || "")}</strong>
      <span>${escapeHTML((network.mediums || []).join(", ") || network.networkType || "")}</span>
      <small>${escapeHTML(t("count.items", { count: (network.components || []).length }, `${(network.components || []).length} items`))}</small>
    </article>`;
}

function renderHVACDebug(hvac, query) {
  if (!hvacDebugEnabled()) {
    elements.hvacGraph.innerHTML = `<div class="empty">${escapeHTML(t("hvac.debugHidden", {}, "Debug details are hidden in the default HVAC view."))}</div>`;
    return;
  }
  if (!hasHVACRuleGraph(hvac) && requestHVACDebugRuleGraph()) {
    elements.hvacGraph.innerHTML = `<div class="empty status-loading">${escapeHTML(t("hvac.debugLoading", {}, "Loading debug rule graph"))}</div>`;
    return;
  }
  const edges = (hvac.ruleGraph?.edges || []).filter((edge) => ruleEdgeSearchFields(edge).join(" ").toLowerCase().includes(query || ""));
  elements.hvacGraph.innerHTML = `
    <section class="hvac-graph-detail">
      <div class="hvac-section-head">
        <h3>${escapeHTML(t("hvac.debug", {}, "Debug"))}</h3>
        <span>${escapeHTML(edges.length)}</span>
      </div>
      <div class="hvac-debug-actions">
        <button type="button" data-hvac-debug-export="rule">${escapeHTML(t("hvac.exportRuleGraph", {}, "Export rule JSON"))}</button>
        <button type="button" data-hvac-debug-export="service">${escapeHTML(t("hvac.exportServiceGraph", {}, "Export service JSON"))}</button>
        <button type="button" data-hvac-debug-export="coupling">${escapeHTML(t("hvac.exportCouplingGraph", {}, "Export coupling JSON"))}</button>
      </div>
      ${renderHVACRuleTraceList(edges)}
    </section>`;
}

function hasHVACRuleGraph(hvac) {
  return Boolean((hvac?.ruleGraph?.nodes || []).length || (hvac?.ruleGraph?.edges || []).length);
}

function requestHVACDebugRuleGraph() {
  const api = backend();
  const key = state.analysisKey || state.lastAnalyzedKey || "";
  if (
    !api ||
    typeof api.AnalyzeInputStageText !== "function" ||
    !key ||
    hvacDebugRuleGraphRequestKey === key ||
    hvacDebugRuleGraphEmptyKey === key
  ) {
    return false;
  }
  hvacDebugRuleGraphRequestKey = key;
  api
    .AnalyzeInputStageText(elements.idfInput.value || "", "hvac-debug")
    .then((result) => {
      if ((state.analysisKey || state.lastAnalyzedKey || "") !== key) {
        return;
      }
      const ruleGraph = result?.report?.hvac?.ruleGraph;
      if (!hasHVACRuleGraph({ ruleGraph })) {
        hvacDebugRuleGraphEmptyKey = key;
        return;
      }
      state.report = state.report || {};
      state.report.hvac = state.report.hvac || {};
      state.report.hvac.ruleGraph = ruleGraph;
      renderHVAC(state.report.hvac);
    })
    .catch((error) => setStatus(error.message || String(error), "error"))
    .finally(() => {
      if (hvacDebugRuleGraphRequestKey === key) {
        hvacDebugRuleGraphRequestKey = "";
      }
    });
  return true;
}

function exportHVACDebugGraph(graph) {
  if (!hvacDebugEnabled()) {
    return;
  }
  const payload = buildHVACDebugGraphExportPayload(graph);
  const blob = new Blob([`${JSON.stringify(payload, null, 2)}\n`], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `semantic-idf-hvac-${payload.graph}-graph.json`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
  setStatus(t("status.hvacGraphExported", {}, "HVAC graph JSON exported"), "ok");
}

function buildHVACDebugGraphExportPayload(graph, hvac = state.report?.hvac) {
  const serviceModel = hvacServiceModel(hvac);
  const graphKind = hvacDebugGraphKind(graph);
  const payload = {
    schema: HVAC_GRAPH_EXPORT_SCHEMA,
    graph: graphKind,
    counts: {
      ruleNodes: (hvac?.ruleGraph?.nodes || []).length,
      ruleEdges: (hvac?.ruleGraph?.edges || []).length,
      zoneServices: (serviceModel.zoneServices || []).length,
      servicePaths: servicePathsForHVAC(hvac).length,
      systems: (serviceModel.systems || []).length,
      components: (serviceModel.components || []).length,
      couplings: (serviceModel.couplings || []).length,
      networks: (serviceModel.networks || []).length,
      navigationEntities: (serviceModel.navigation?.entities || []).length,
      navigationLinks: (serviceModel.navigation?.links || []).length,
    },
    data: {},
  };
  if (graphKind === "rule") {
    payload.data.ruleGraph = hvac?.ruleGraph || { nodes: [], edges: [] };
  } else if (graphKind === "service") {
    payload.data.serviceModel = serviceModel;
  } else {
    payload.data.couplings = serviceModel.couplings || [];
    payload.data.networks = serviceModel.networks || [];
    payload.data.navigation = serviceModel.navigation || { entities: [], links: [] };
  }
  return payload;
}

function hvacDebugGraphKind(value) {
  const graph = String(value || "").toLowerCase();
  return ["rule", "service", "coupling"].includes(graph) ? graph : "rule";
}

function renderHVACDiagnostics(hvac, query) {
  const warnings = (hvac.warnings || []).filter((warning) => warningMatchesQuery(warning, query));
  elements.hvacGraph.innerHTML = warnings.length
    ? `<div class="hvac-diagnostic-list">${warnings.map(renderHVACWarning).join("")}</div>`
    : `<div class="empty">${(hvac.warnings || []).length ? t("hvac.noMatchingWarnings") : t("hvac.noWarnings")}</div>`;
}

function renderHVACWarnings(hvac, query) {
  const warnings = (hvac.warnings || []).filter((warning) => warningMatchesQuery(warning, query)).slice(0, 8);
  elements.hvacWarningStats.textContent = query
    ? t("count.matching", { count: warnings.length })
    : t("count.warnings", { count: (hvac.warnings || []).length });
  elements.hvacWarnings.innerHTML = warnings.length
    ? warnings.map(renderHVACWarning).join("")
    : `<div class="empty">${(hvac.warnings || []).length ? t("hvac.noMatchingWarnings") : t("hvac.noWarnings")}</div>`;
}

function renderHVACWarning(warning) {
  const details = [
    warning.edgeId ? `edge ${warning.edgeId}` : "",
    warning.sourceFieldIndex !== undefined && warning.sourceFieldIndex !== null ? `source field ${warning.sourceFieldIndex}` : "",
    (warning.expectedNodes || []).length ? `expected ${warning.expectedNodes.join(", ")}` : "",
    warning.actualNode ? `actual ${warning.actualNode}` : "",
    warning.suggestedFixTarget ? `fix ${warning.suggestedFixTarget}` : "",
  ].filter(Boolean);
  return `
    <article class="hvac-warning ${escapeHTML(warning.severity || "warning")}">
      <div>
        <strong>${escapeHTML(warning.message || "")}</strong>
        <span>${escapeHTML([warning.code, warning.source, warning.objectType, warning.objectName].filter(Boolean).join(" / "))}</span>
        ${details.length ? `<small>${escapeHTML(details.join(" / "))}</small>` : ""}
      </div>
      ${renderObjectLink(warning.objectIndex, warning.objectType)}
    </article>`;
}

function renderHVACInspector(hvac, selectedLoop) {
  if (state.activeHVACNodeName) {
    const usages = (hvac.nodeUsages || []).filter((usage) => usage.nodeName === state.activeHVACNodeName);
    const monitors = hvacNodeOutputMonitorsForNode(hvac, state.activeHVACNodeName);
    elements.hvacInspectorStats.textContent = t("count.uses", { count: usages.length });
    elements.hvacInspector.innerHTML = `
      <div class="hvac-inspector-title">
        <strong title="${escapeHTML(state.activeHVACNodeName)}">${escapeHTML(state.activeHVACNodeName)}</strong>
        <span>Node</span>
      </div>
      ${renderNodeOutputMonitorPanel(hvac, state.activeHVACNodeName, monitors)}
      ${usages.length ? usages.map(renderNodeUsage).join("") : `<div class="empty">${t("hvac.noNodeUsages")}</div>`}`;
    return;
  }
  if (state.activeHVACGraphKey) {
    const isServiceSelection =
      state.activeHVACView === "services" || state.activeHVACView === "couplings" || state.activeHVACGraphKey.startsWith("coupling-node:");
    const selected =
      isServiceSelection
        ? selectedServiceGraphItem(servicePathsForHVAC(hvac), hvacServiceModel(hvac).couplings || [])
        : selectedLoop
          ? selectedLoopGraphItem(selectedLoop)
          : null;
    if (selected) {
      if (isServiceSelection) {
        renderHVACInspectorServiceSelection(selected, hvacServiceModel(hvac).couplings || []);
        return;
      }
      renderHVACInspectorSelection(selected);
      return;
    }
  }
  const navigationSelection = selectedHVACNavigationEntity(hvac);
  if (navigationSelection) {
    renderHVACInspectorNavigationSelection(navigationSelection, hvac);
    return;
  }
  if (!selectedLoop) {
    elements.hvacInspectorStats.textContent = t("hvac.noLoopSelected");
    elements.hvacInspector.innerHTML = `<div class="empty">${t("hvac.selectLoop")}</div>`;
    return;
  }
  elements.hvacInspectorStats.textContent = t("count.zones", { count: (selectedLoop.relatedZones || []).length });
  elements.hvacInspector.innerHTML = `
    <div class="hvac-inspector-title">
      <strong>${escapeHTML(selectedLoop.name || selectedLoop.type)}</strong>
      <span>${escapeHTML(selectedLoop.type)}</span>
    </div>
    <div class="hvac-inspector-kv"><span>${t("hvac.supplyBranches")}</span><strong>${escapeHTML((selectedLoop.supplySide?.branches || []).length)}</strong></div>
    <div class="hvac-inspector-kv"><span>${t("hvac.demandBranches")}</span><strong>${escapeHTML((selectedLoop.demandSide?.branches || []).length)}</strong></div>
    <div class="hvac-inspector-kv"><span>${t("hvac.relatedZones")}</span><strong>${escapeHTML((selectedLoop.relatedZones || []).length)}</strong></div>
    <div class="hvac-tag-list">${(selectedLoop.relatedZones || []).map((zone) => `<span>${escapeHTML(zone)}</span>`).join("") || `<span>N/A</span>`}</div>`;
}

function renderHVACInspectorServiceSelection(selected, couplings) {
  if (selected.kind === "coupling") {
    elements.hvacInspectorStats.textContent = selected.coupling?.couplingType || t("hvac.couplings", {}, "Couplings");
    elements.hvacInspector.innerHTML = renderSelectedCouplingDetail(selected.coupling, selected.path);
    return;
  }
  const path = selected.path || selected.paths?.[0];
  if (!path) {
    return;
  }
  const nodeRef = selected.node?.ref || path.delivery || {};
  const title = selected.node?.label || nodeRef.displayName || nodeRef.objectName || servedSubjectLabel(path.servedSubject || path);
  elements.hvacInspectorStats.textContent = selected.kind === "node" ? selected.node?.kind || "node" : path.serviceKind || "service";
  elements.hvacInspector.innerHTML = `
    <div class="hvac-inspector-title">
      <strong>${escapeHTML(title)}</strong>
      <span>${escapeHTML(selected.kind === "node" ? selected.node?.meta || "" : `${serviceKindLabel(path.serviceKind)} / ${pathTypeLabel(path.pathType)}`)}</span>
    </div>
    <section class="hvac-connected-systems">
      <strong>${escapeHTML(t("hvac.connectedSystems", {}, "Connected systems"))}</strong>
      <div class="hvac-connected-system-list">
        ${servicePathConnectedSystems(path).map((item) => `<span>${escapeHTML(item)}</span>`).join("") || `<span>N/A</span>`}
      </div>
    </section>
    <div class="hvac-inspector-kv"><span>Name</span><strong>${escapeHTML(nodeRef.displayName || nodeRef.objectName || title || "N/A")}</strong></div>
    <div class="hvac-inspector-kv"><span>${escapeHTML(t("common.type"))}</span><strong>${escapeHTML(nodeRef.objectType || nodeRef.type || selected.node?.kind || "N/A")}</strong></div>
    <div class="hvac-inspector-kv"><span>Role</span><strong>${escapeHTML(nodeRef.role || selected.node?.role || path.pathType || "N/A")}</strong></div>
    <div class="hvac-inspector-kv"><span>${escapeHTML(t("hvac.deliveryType", {}, "Delivery type"))}</span><strong>${escapeHTML(path.deliveryEquipment?.deliveryType || nodeRef.deliveryType || "N/A")}</strong></div>
    <div class="hvac-inspector-kv"><span>${escapeHTML(t("common.inlet"))}</span><strong>${escapeHTML(nodeRef.inletNode || "N/A")}</strong></div>
    <div class="hvac-inspector-kv"><span>${escapeHTML(t("common.outlet"))}</span><strong>${escapeHTML(nodeRef.outletNode || "N/A")}</strong></div>
    <div class="hvac-inspector-kv"><span>${escapeHTML(t("common.water"))}</span><strong>${escapeHTML([nodeRef.waterInletNode, nodeRef.waterOutletNode].filter(Boolean).join(" -> ") || "N/A")}</strong></div>
    ${renderHVACTraceDrawer(path.traceIds || [])}
    ${(path.supportingCouplingIds || []).length ? `<div class="hvac-detail-list">${(path.supportingCouplingIds || []).map((id) => (couplings || []).find((coupling) => coupling.id === id)).filter(Boolean).map((coupling) => `<div><strong>${escapeHTML(coupling.object?.displayName || coupling.role || "")}</strong><span>${escapeHTML(couplingRoleLabel(coupling))}</span></div>`).join("")}</div>` : ""}
  `;
}

function selectedHVACNavigationEntity(hvac) {
  const entity = state.activeHVACEntity || {};
  if (!entity.id || !entity.kind) {
    return null;
  }
  const indexed = findHVACNavigationEntity(entity.id) || entity;
  const paths = pathsForActiveHVACEntity(hvac);
  if (entity.kind === "component") {
    return { kind: "component", entity: indexed, component: findServiceModelComponentByEntityID(entity.id), paths };
  }
  if (entity.kind === "zone" || entity.kind === "space") {
    return { kind: "zone", entity: indexed, paths };
  }
  if (entity.kind === "coupling") {
    const coupling = (hvacServiceModel(hvac).couplings || []).find((item) => navigationCouplingEntityID(item.id) === entity.id);
    return { kind: "coupling", entity: indexed, coupling, paths };
  }
  if (entity.kind === "service_path") {
    return { kind: "service_path", entity: indexed, path: selectedHVACPath(hvac), paths };
  }
  return { kind: entity.kind, entity: indexed, paths };
}

function renderHVACInspectorNavigationSelection(selection, hvac) {
  if (selection.kind === "component" && selection.component) {
    renderHVACInspectorComponentFocus(selection.component, selection.paths);
    return;
  }
  if (selection.kind === "coupling" && selection.coupling) {
    elements.hvacInspectorStats.textContent = selection.coupling.couplingType || t("hvac.couplings", {}, "Couplings");
    elements.hvacInspector.innerHTML = renderSelectedCouplingDetail(selection.coupling, selection.paths[0]);
    return;
  }
  if (selection.kind === "service_path" && selection.path) {
    elements.hvacInspectorStats.textContent = selection.path.serviceKind || "service";
    elements.hvacInspector.innerHTML = renderSelectedServicePathDetail(selection.path, hvacServiceModel(hvac).couplings || []);
    return;
  }
  if (selection.kind === "zone") {
    renderHVACInspectorZoneFocus(selection.entity, selection.paths);
    return;
  }
  elements.hvacInspectorStats.textContent = selection.entity.kind || t("common.selection");
  elements.hvacInspector.innerHTML = `
    <div class="hvac-inspector-title">
      <strong>${escapeHTML(selection.entity.label || selection.entity.id)}</strong>
      <span>${escapeHTML(selection.entity.kind || "")}</span>
    </div>
    ${renderRelatedServicePathList(selection.paths)}`;
}

function renderHVACInspectorComponentFocus(componentItem, paths = []) {
  const component = componentItem.component || {};
  const systems = (componentItem.relatedLoopRefs || []).map((loop) => ({ name: loop.name, type: loop.type, id: navigationLoopEntityID(loop.type, loop.name) }));
  elements.hvacInspectorStats.textContent = component.objectType || componentItem.displayFamily || t("common.component");
  elements.hvacInspector.innerHTML = `
    <div class="hvac-inspector-title">
      <strong>${escapeHTML(componentRefLabel(component))}</strong>
      <span>${escapeHTML([component.objectType, componentItem.displayFamily || componentItem.family].filter(Boolean).join(" / "))}</span>
    </div>
    ${systems.length ? `<section class="hvac-connected-systems"><strong>${escapeHTML(t("hvac.connectedSystems", {}, "Connected systems"))}</strong><div class="hvac-connected-system-list">${systems.map((system) => `<button class="${escapeHTML(hvacLoopChipClass(system.type))}" type="button" data-hvac-entity-kind="loop" data-hvac-entity-id="${escapeHTML(system.id)}" data-hvac-entity-label="${escapeHTML(system.name)}">${escapeHTML(system.name)}</button>`).join("")}</div></section>` : ""}
    <div class="hvac-inspector-kv"><span>${escapeHTML(t("common.type"))}</span><strong>${escapeHTML(component.objectType || "N/A")}</strong></div>
    <div class="hvac-inspector-kv"><span>Role</span><strong>${escapeHTML(component.role || componentItem.deliveryType || componentItem.couplingType || "N/A")}</strong></div>
    <div class="hvac-inspector-kv"><span>Medium</span><strong>${escapeHTML((componentItem.mediums || component.mediums || []).map(serviceEdgeLabel).join(", ") || "N/A")}</strong></div>
    ${renderComponentOccurrenceList(componentItem)}
    ${renderRelatedServicePathList(paths)}
    ${renderComponentOutputActions(component)}`;
}

function renderHVACInspectorZoneFocus(entity, paths = []) {
  elements.hvacInspectorStats.textContent = t("count.items", { count: paths.length }, `${paths.length} items`);
  elements.hvacInspector.innerHTML = `
    <div class="hvac-inspector-title">
      <strong>${escapeHTML(entity.label || entity.zoneName || entity.id)}</strong>
      <span>${escapeHTML(entity.kind || "zone")}</span>
    </div>
    ${renderZoneServiceDashboard(paths)}
    <div class="hvac-loop-actions">
      <button type="button" data-result-tab="profile">${escapeHTML(t("tab.profile", {}, "Profile"))}</button>
      <button type="button" data-result-tab="geometry">${escapeHTML(t("tab.geometry", {}, "Geometry"))}</button>
      <button type="button" data-result-tab="output">${escapeHTML(t("tab.output", {}, "Output"))}</button>
      <button type="button" data-result-tab="simulation">${escapeHTML(t("tab.simulation", {}, "Simulation"))}</button>
      ${renderObjectLink(entity.objectIndex, entity.kind === "space" ? "Space" : "Zone")}
    </div>`;
}

function renderComponentOccurrenceList(componentItem = {}) {
  const occurrences = componentItem.occurrences || [];
  if (!occurrences.length) {
    return "";
  }
  return `
    <section class="hvac-detail-list">
      ${occurrences
        .map((occurrence) => {
          const label = [occurrence.contextType, occurrence.loopType, occurrence.loopName, occurrence.zoneName].filter(Boolean).join(" / ");
          const detail = [occurrence.loopSide, occurrence.branchName, occurrence.spaceName, occurrence.roleHere].filter(Boolean).join(" / ");
          return `<div><strong>${escapeHTML(label || "Occurrence")}</strong><span>${escapeHTML(detail || "N/A")}</span></div>`;
        })
        .join("")}
    </section>`;
}

function renderRelatedServicePathList(paths = []) {
  if (!paths.length) {
    return "";
  }
  return `
    <section class="hvac-detail-list">
      ${paths
        .slice(0, 8)
        .map((path) => `<button class="hvac-related-path-chip" type="button" data-hvac-entity-kind="service_path" data-hvac-entity-id="${escapeHTML(navigationPathEntityID(path.id))}" data-hvac-entity-view="services" data-hvac-path-id="${escapeHTML(path.id)}" data-hvac-graph-key="${escapeHTML(servicePathGraphKey(path))}"><strong>${escapeHTML(servedSubjectLabel(path.servedSubject || path))}</strong><span>${escapeHTML([serviceKindLabel(path.serviceKind), deliveryLabel(path)].filter(Boolean).join(" / "))}</span></button>`)
        .join("")}
    </section>`;
}

function renderZoneServiceDashboard(paths = []) {
  const groups = groupBy(paths, (path) => serviceKindLabel(path.serviceKind));
  return `
    <section class="hvac-zone-service-dashboard">
      ${Object.entries(groups)
        .map(([label, rows]) => `<div><strong>${escapeHTML(label)}</strong>${rows.map((path) => `<button type="button" data-hvac-entity-kind="service_path" data-hvac-entity-id="${escapeHTML(navigationPathEntityID(path.id))}" data-hvac-entity-view="services" data-hvac-path-id="${escapeHTML(path.id)}" data-hvac-graph-key="${escapeHTML(servicePathGraphKey(path))}">${escapeHTML(servicePathSummaryText(path))}</button>`).join("")}</div>`)
        .join("") || `<div class="empty">${escapeHTML(t("hvac.noServicePaths", {}, "No HVAC service path resolved"))}</div>`}
    </section>`;
}

function renderComponentOutputActions(component = {}) {
  const outputVariables = outputVariablesForComponent(component).slice(0, 4);
  const keyValue = component.outletNode || component.objectName || component.displayName || "";
  if (!outputVariables.length && !keyValue) {
    return "";
  }
  return `
    <section class="hvac-node-output-list">
      <div class="hvac-loop-actions">
        <button type="button" data-result-tab="output">${escapeHTML(t("tab.output", {}, "Output"))}</button>
        <button type="button" data-result-tab="simulation">${escapeHTML(t("tab.simulation", {}, "Simulation"))}</button>
      </div>
      ${outputVariables
        .map((variable) => `<article class="hvac-node-output-row"><div><strong>${escapeHTML(variable)}</strong><span>${escapeHTML(keyValue || component.objectType || "")}</span></div><button class="hvac-edit-button" type="button" data-hvac-output-key="${escapeHTML(keyValue)}" data-hvac-output-variable="${escapeHTML(variable)}"><span>${escapeHTML(t("hvac.addMonitor", {}, "Add monitor"))}</span></button></article>`)
        .join("")}
    </section>`;
}

function outputVariablesForComponent(component = {}) {
  const type = normalizeGraphName(component.objectType || "");
  if (type.includes("coil")) {
    return ["Cooling Coil Total Cooling Rate", "Heating Coil Heating Rate", "System Node Temperature", "System Node Mass Flow Rate"];
  }
  if (type.includes("fan")) {
    return ["Fan Electricity Rate", "Fan Pressure Rise", "System Node Mass Flow Rate"];
  }
  if (type.includes("pump")) {
    return ["Pump Electricity Rate", "Pump Mass Flow Rate"];
  }
  if (type.includes("chiller")) {
    return ["Chiller Evaporator Cooling Rate", "Chiller Electricity Rate", "Chiller COP"];
  }
  if (type.includes("boiler")) {
    return ["Boiler Heating Rate", "Boiler Fuel Rate"];
  }
  return [];
}

function servicePathSummaryText(path = {}) {
  return [path.plantLoop?.name || path.refrigerantSystem?.name || path.sourceSystem?.name, (path.conditioning || [])[0]?.displayName || (path.conditioning || [])[0]?.objectName, path.airLoop?.name, deliveryLabel(path), servedSubjectLabel(path.servedSubject || path)]
    .filter(Boolean)
    .join(" -> ");
}

function findServiceModelComponentByEntityID(entityID = "") {
  return (hvacServiceModel().components || []).find((item) => navigationComponentEntityID(item.component || {}) === entityID) || null;
}

function groupBy(values = [], keyFn) {
  return values.reduce((groups, value) => {
    const key = keyFn(value) || "Other";
    groups[key] = groups[key] || [];
    groups[key].push(value);
    return groups;
  }, {});
}

function renderHVACInspectorSelection(selected) {
  const title =
    selected.component?.objectName ||
    selected.branch?.name ||
    selected.zoneName ||
    selected.loopName ||
    selected.nodeName ||
    selected.loop?.name ||
    t("common.selection");
  elements.hvacInspectorStats.textContent = selected.kind || t("common.selection");
  const componentRuleEdges = selected.component ? ruleEdgesForComponent(selected.component) : [];
  elements.hvacInspector.innerHTML = `
    <div class="hvac-inspector-title">
      <strong>${escapeHTML(title)}</strong>
      <span>${escapeHTML(selected.kind || t("common.selection"))}</span>
    </div>
    ${selected.component ? renderConnectedSystemsPanel(selected) : ""}
    ${
      selected.component
        ? `
          <div class="hvac-inspector-kv"><span>${t("common.type")}</span><strong>${escapeHTML(selected.component.objectType || "N/A")}</strong></div>
          <div class="hvac-inspector-kv"><span>Family</span><strong>${escapeHTML(componentMetaLabel(selected.component))}</strong></div>
          <div class="hvac-inspector-kv"><span>${t("common.inlet")}</span><strong>${escapeHTML(selected.component.inletNode || "N/A")}</strong></div>
          <div class="hvac-inspector-kv"><span>${t("common.outlet")}</span><strong>${escapeHTML(selected.component.outletNode || "N/A")}</strong></div>`
        : ""
    }
    }`;
}

function renderConnectedSystemsPanel(selected) {
  const systems = connectedSystemsForSelection(selected);
  if (!systems.length) {
    return "";
  }
  return `
    <section class="hvac-connected-systems">
      <strong>${escapeHTML(t("hvac.connectedSystems", {}, "Connected systems"))}</strong>
      <div class="hvac-connected-system-list">
        ${systems
          .map((system) =>
            system.current
              ? `<span class="${escapeHTML(hvacLoopChipClass(system.type))}" title="${escapeHTML(system.name)}">${escapeHTML(system.name)} / ${escapeHTML(t("common.current"))}</span>`
              : `<button class="${escapeHTML(hvacLoopChipClass(system.type))}" type="button" data-hvac-jump-loop-name="${escapeHTML(system.name)}" data-hvac-jump-graph-key="${escapeHTML(system.graphKey || componentGraphKey(selected.component))}" title="${escapeHTML([system.type, system.name].filter(Boolean).join(" "))}">${escapeHTML(system.name)}</button>`,
          )
          .join("")}
      </div>
    </section>`;
}

function connectedSystemsForSelection(selected) {
  const component = selected.component || {};
  const systems = [];
  const add = (name, type = "", current = false, graphKey = "") => {
    const cleanName = String(name || "").trim();
    if (!cleanName) {
      return;
    }
    const key = `${normalizeGraphName(type)}:${normalizeGraphName(cleanName)}:${current ? "current" : "related"}`;
    if (systems.some((system) => system.key === key)) {
      return;
    }
    systems.push({ key, name: cleanName, type, current, graphKey });
  };
  const currentLoopName = component.loopName || selected.loop?.name || "";
  const currentLoop = findHVACLoopByName(currentLoopName);
  if (currentLoopName) {
    add(currentLoopName, selected.loop?.type || currentLoop?.type || "", true);
  }
  for (const loopName of verifiedCrossLoopNamesForComponent(component)) {
    const loop = findHVACLoopByName(loopName);
    add(loopName, loop?.type || "", false, componentGraphKey(component));
  }
  return systems.slice(0, 10);
}

function renderNodeUsage(usage) {
  return `
    <div class="hvac-node-usage">
      <div>
        <strong>${escapeHTML(usage.role || "node")}</strong>
        <span>${escapeHTML(usage.fieldName || `Field ${Number(usage.fieldIndex) + 1}`)}</span>
      </div>
      <div>
        <span>${escapeHTML(usage.objectType)} ${escapeHTML(usage.objectName || "")}</span>
        ${renderObjectLink(usage.objectIndex, usage.objectType)}
      </div>
    </div>`;
}

function renderNodeOutputMonitorPanel(hvac, nodeName, monitors) {
  const variables = hvacNodeOutputVariables(hvac);
  return `
    <section class="hvac-node-monitor">
      <div class="hvac-section-head compact">
        <h3>${t("hvac.outputMonitor")}</h3>
        <span>${t("hvac.monitorNodeHint")}</span>
      </div>
      <p>${t("hvac.nodeOutputAvailabilityNote")}</p>
      <div class="hvac-node-monitor-existing">
        <strong>${t("hvac.existingOutputVariables")}</strong>
        <div class="hvac-tag-list">
          ${
            monitors.length
              ? monitors
                  .map(
                    (monitor) =>
                      `<span title="${escapeHTML(monitor.variableName)}">${escapeHTML(shortOutputMonitorLabel(monitor))} ${renderObjectLink(monitor.objectIndex, "Output:Variable")}</span>`,
                  )
                  .join("")
              : `<span>${t("hvac.noOutputMonitors")}</span>`
          }
        </div>
      </div>
      <div class="hvac-node-output-list">
        ${variables.map((variable) => renderNodeOutputVariable(nodeName, variable, monitors)).join("")}
      </div>
    </section>`;
}

function renderNodeOutputVariable(nodeName, variable, monitors) {
  const alreadyRequested = monitors.some((monitor) => sameOutputVariableName(monitor.variableName, variable.variableName));
  const badges = [
    variable.units ? `[${variable.units}]` : "",
    variable.reportType || "",
    variable.appliesTo || "",
    variable.advanced ? t("hvac.advancedOutput") : "",
  ].filter(Boolean);
  return `
    <article class="hvac-node-output-row ${alreadyRequested ? "requested" : ""}">
      <div>
        <strong title="${escapeHTML(variable.variableName)}">${escapeHTML(variable.variableName)}</strong>
        <span title="${escapeHTML(variable.description || "")}">${escapeHTML(variable.description || "")}</span>
        <div class="hvac-output-badges">${badges.map((badge) => `<small>${escapeHTML(badge)}</small>`).join("")}</div>
      </div>
      <button class="hvac-edit-button" type="button"
        data-hvac-output-key="${escapeHTML(nodeName)}"
        data-hvac-output-variable="${escapeHTML(variable.variableName)}">
        <span>${escapeHTML(alreadyRequested ? t("hvac.addAnotherMonitor") : t("hvac.addMonitor"))}</span>
      </button>
    </article>`;
}

function hvacNodeOutputVariables(hvac) {
  return hvac?.nodeOutputVariables?.length ? hvac.nodeOutputVariables : fallbackHVACNodeOutputVariables();
}

function fallbackHVACNodeOutputVariables() {
  return [
    { variableName: "System Node Temperature", units: "C", reportType: "Average", category: "core" },
    { variableName: "System Node Mass Flow Rate", units: "kg/s", reportType: "Average", category: "core" },
    { variableName: "System Node Standard Density Volume Flow Rate", units: "m3/s", reportType: "Average", category: "core" },
    { variableName: "System Node Enthalpy", units: "J/kg", reportType: "Average", category: "core" },
    { variableName: "System Node Setpoint Temperature", units: "C", reportType: "Average", category: "setpoint" },
  ];
}

function hvacNodeOutputMonitorsForNode(hvac, nodeName) {
  return (hvac?.nodeOutputMonitors || []).filter((monitor) => monitor.wildcard || String(monitor.keyValue || "").toLowerCase() === String(nodeName || "").toLowerCase());
}

function shortOutputMonitorLabel(monitor) {
  const frequency = monitor.reportingFrequency || "Hourly";
  const key = monitor.wildcard ? "*" : monitor.keyValue;
  return `${monitor.variableName} / ${frequency}${monitor.wildcard ? ` / ${key}` : ""}`;
}

function sameOutputVariableName(left, right) {
  return String(left || "").trim().toLowerCase() === String(right || "").trim().toLowerCase();
}

function renderNodePill(nodeName, label) {
  if (!nodeName) {
    return `<span class="hvac-node empty-node">${escapeHTML(label)} N/A</span>`;
  }
  const active = nodeName === state.activeHVACNodeName ? " active" : "";
  return `<button class="hvac-node${active}" data-hvac-node="${escapeHTML(nodeName)}" title="${escapeHTML(nodeName)}" type="button"><small>${escapeHTML(label)}</small><span>${escapeHTML(nodeName)}</span></button>`;
}

function renderObjectLink(objectIndex, objectType) {
  const index = Number(objectIndex);
  if (!Number.isFinite(index) || index < 0) {
    return "";
  }
  return `<button class="profile-object-link navigable-row" data-jump-object-index="${escapeHTML(index)}" data-jump-object-type="${escapeHTML(objectType || "")}" type="button">#${escapeHTML(index + 1)}</button>`;
}

function objectReferenceText(objectIndex) {
  const index = Number(objectIndex);
  return Number.isFinite(index) && index >= 0 ? `#${index + 1}` : "";
}

function hvacQuery() {
  return (elements.hvacFilter?.value || "").trim().toLowerCase();
}

function loopMatchesQuery(loop, query) {
  if (!query) {
    return true;
  }
  const haystack = [
    loop.type,
    loop.name,
    ...(loop.relatedZones || []),
    ...loopComponents(loop).flatMap((component) => [
      ...componentSearchFields(component),
      component.inletNode,
      component.outletNode,
      component.waterInletNode,
      component.waterOutletNode,
    ]),
  ]
    .join(" ")
    .toLowerCase();
  return haystack.includes(query);
}

function warningMatchesQuery(warning, query) {
  if (!query) {
    return true;
  }
  return [warning.severity, warning.category, warning.code, warning.source, warning.message, warning.objectType, warning.objectName, warning.field, warning.value, warning.edgeId, warning.sourceFieldIndex, warning.actualNode, warning.suggestedFixTarget, ...(warning.expectedNodes || [])]
    .join(" ")
    .toLowerCase()
    .includes(query);
}

function loopComponents(loop) {
  const sides = [loop.supplySide, loop.demandSide].filter(Boolean);
  return sides.flatMap((side) => (side.branches || []).flatMap((branch) => branch.components || []));
}

function componentsForSide(side = {}) {
  return (side.branches || []).flatMap((branch) => branch.components || []);
}

function distributeGraphPositions(count, startX, endX, y) {
  if (count <= 0) {
    return [];
  }
  if (count === 1) {
    return [{ x: (startX + endX) / 2, y }];
  }
  const step = (endX - startX) / (count - 1);
  return Array.from({ length: count }, (_, index) => ({ x: startX + step * index, y }));
}

function componentGraphKey(component) {
  if (!component) {
    return "component:";
  }
  if (Number.isFinite(Number(component.objectIndex)) && Number(component.objectIndex) >= 0) {
    return `component:${component.objectIndex}`;
  }
  return `component:${component.objectType || ""}:${component.objectName || ""}`;
}

function branchGraphKey(branch) {
  return `branch:${branch.objectIndex}:${branch.name || ""}`;
}

function componentGraphRelatedKeys(component) {
  return [
    component?.inletNode ? `node:${component.inletNode}` : "",
    component?.outletNode ? `node:${component.outletNode}` : "",
    component?.waterInletNode ? `node:${component.waterInletNode}` : "",
    component?.waterOutletNode ? `node:${component.waterOutletNode}` : "",
    ...verifiedCrossLoopNamesForComponent(component).map((name) => `loop-name:${name}`),
  ].filter(Boolean);
}

function componentSearchFields(component = {}) {
  return [
    component.objectType,
    component.objectName,
    component.family,
    component.familyLabel,
    component.displayLabel,
    component.roleHere,
    component.sourceOwner,
    component.sourceOwnerType,
    component.sourceOwnerName,
    component.typeFieldIndex,
    component.nameFieldIndex,
    component.expectedObjectType,
    ...ruleEdgesForComponent(component).flatMap(ruleEdgeSearchFields),
  ];
}

function graphSelectionClass(key, relatedKeys = []) {
  if (!state.activeHVACGraphKey) {
    return "";
  }
  if (state.activeHVACGraphKey === key) {
    return "selected";
  }
  if (relatedKeys.includes(state.activeHVACGraphKey)) {
    return "related";
  }
  return "dimmed";
}

function selectedLoopGraphItem(loop) {
  const key = state.activeHVACGraphKey;
  if (!key || key === `loop:${loop.id}`) {
    return key ? { kind: "loop", loop } : null;
  }
  if (key.startsWith("node:")) {
    return { kind: "node", nodeName: key.slice(5) };
  }
  for (const side of [loop.supplySide, loop.demandSide]) {
    for (const branch of side?.branches || []) {
      if (branchGraphKey(branch) === key) {
        return { kind: "branch", branch, side };
      }
      for (const component of branch.components || []) {
        if (componentGraphKey(component) === key) {
          return { kind: "component", component, branch, side, loop };
        }
      }
    }
  }
  if (key.startsWith("zone:")) {
    return { kind: "zone", zoneName: key.slice(5), loop };
  }
  return null;
}

function renderSelectedHVACDetail(selected) {
  if (selected.kind === "component") {
    const component = selected.component || {};
    const ruleEdges = ruleEdgesForComponent(component);
    const relatedLoopNames = verifiedCrossLoopNamesForComponent(component);
    return `
      <section class="hvac-graph-detail">
        <div class="hvac-section-head">
          <h3>${escapeHTML(component.objectName || selected.label || t("common.component"))}</h3>
          <span>${escapeHTML(component.objectType || t("common.component"))}</span>
        </div>
        <div class="hvac-detail-grid">
          <div><span>${t("common.object")}</span><strong>${renderObjectLink(component.objectIndex, component.objectType) || "N/A"}</strong></div>
          <div><span>Family</span><strong>${escapeHTML(componentMetaLabel(component))}</strong></div>
          <div><span>${t("common.inlet")}</span><strong>${escapeHTML(component.inletNode || "N/A")}</strong></div>
          <div><span>${t("common.outlet")}</span><strong>${escapeHTML(component.outletNode || "N/A")}</strong></div>
          <div><span>${t("common.water")}</span><strong>${escapeHTML([component.waterInletNode, component.waterOutletNode].filter(Boolean).join(" -> ") || "N/A")}</strong></div>
          ${component.sourceOwner ? `<div><span>Source owner</span><strong>${escapeHTML(component.sourceOwner)}</strong></div>` : ""}
          ${component.expectedObjectType ? `<div><span>Expected type</span><strong>${escapeHTML(component.expectedObjectType)}</strong></div>` : ""}
          ${component.loopName ? `<div><span>${t("hvac.viewLoop")}</span><strong>${escapeHTML(component.loopName)}</strong></div>` : ""}
        </div>
        ${renderHVACTraceDrawer(ruleEdges.map((edge) => [edge.ruleId, edge.sourceObjectName, hvacSourceFieldLabel(component)].filter(Boolean).join(" / ")))}
        ${renderComponentCrossLoopMap(component, relatedLoopNames)}
        ${renderHVACEditableFields(component.editableFields)}
      </section>`;
  }
  if (selected.kind === "branch") {
    return `
      <section class="hvac-graph-detail">
        <div class="hvac-section-head">
          <h3>${escapeHTML(selected.branch.name || "Branch")}</h3>
          <span>${renderObjectLink(selected.branch.objectIndex, "Branch")}</span>
        </div>
        <div class="hvac-detail-list">
          ${(selected.branch.components || []).map((component) => `<div><strong>${escapeHTML(component.objectName || component.objectType)}</strong><span>${escapeHTML(component.objectType || "")}</span></div>`).join("") || `<div class="empty">${t("hvac.noBranchComponents")}</div>`}
        </div>
      </section>`;
  }
  if (selected.kind === "node") {
    return `
      <section class="hvac-graph-detail">
        <div class="hvac-section-head">
          <h3>${escapeHTML(selected.nodeName)}</h3>
          <span>Node</span>
        </div>
        <p class="hvac-detail-note">${t("hvac.nodeUsageInspector")}</p>
      </section>`;
  }
  if (selected.kind === "zone") {
    const loop = selected.loop || {};
    return `
      <section class="hvac-graph-detail">
        <div class="hvac-section-head">
          <h3>${escapeHTML(selected.zoneName)}</h3>
          <span>Zone</span>
        </div>
        <div class="hvac-detail-grid">
          <div><span>${t("hvac.connectedSystems", {}, "Connected systems")}</span><strong>${escapeHTML(loop.name ? `${loop.type || "Loop"} ${loop.name}` : "N/A")}</strong></div>
          <div><span>${t("hvac.relatedZones")}</span><strong>${escapeHTML((loop.relatedZones || []).length)}</strong></div>
        </div>
      </section>`;
  }
  return `
    <section class="hvac-graph-detail">
      <div class="hvac-section-head">
        <h3>${escapeHTML(selected.loop?.name || "Loop")}</h3>
        <span>${escapeHTML(selected.loop?.type || "Loop")}</span>
      </div>
      <div class="hvac-detail-grid">
        <div><span>${t("hvac.relatedZones")}</span><strong>${escapeHTML((selected.loop?.relatedZones || []).length)}</strong></div>
        <div><span>${t("hvac.crossLoopLinksLabel")}</span><strong>${escapeHTML((selected.loop?.relatedLoops || []).length)}</strong></div>
      </div>
    </section>`;
}

function renderHVACRuleTraceList(rows = []) {
  if (!rows.length) {
    return "";
  }
  return `
    <div class="hvac-detail-list hvac-rule-trace-list">
      ${rows
        .slice(0, 12)
        .map(
          (edge) => `
            <div>
              <strong title="${escapeHTML(edge.ruleId || "")}">${escapeHTML(edge.ruleId || "trace item")}</strong>
              <span>${renderObjectLink(edge.sourceObjectIndex, edge.sourceObjectType)} ${escapeHTML(ruleEdgeTraceText(edge))}</span>
            </div>`,
        )
        .join("")}
    </div>`;
}

function renderComponentCrossLoopMap(component = {}, names = verifiedCrossLoopNamesForComponent(component)) {
  const relatedLoopNames = [...new Set(names || [])].filter(Boolean);
  if (!relatedLoopNames.length) {
    return "";
  }
  const componentKey = componentGraphKey(component);
  const rows = relatedLoopNames.map((loopName, index) => {
    const loop = findHVACLoopByName(loopName);
    const y = 44 + index * 58;
    return { loopName, loop, y };
  });
  const height = Math.max(126, 42 + rows.length * 58);
  const componentY = height / 2;
  return `
    <section class="hvac-cross-loop-map">
      <div class="hvac-section-head compact">
        <h3>${t("hvac.crossLoopRelations")}</h3>
        <span>${t("hvac.crossLoop")}</span>
      </div>
      <svg class="hvac-cross-loop-svg" viewBox="0 0 720 ${height}" role="img" aria-label="${escapeHTML(t("hvac.crossLoopRelations"))}">
        <g class="hvac-cross-loop-component">
          <rect x="24" y="${componentY - 24}" width="238" height="48" rx="8"></rect>
          <text class="label" x="42" y="${componentY - 4}">${escapeHTML(truncateText(component.objectName || component.objectType || t("common.component"), 28))}</text>
          <text class="meta" x="42" y="${componentY + 14}">${escapeHTML(truncateText(component.objectType || t("common.component"), 32))}</text>
        </g>
        ${rows
          .map(
            ({ loopName, loop, y }) => `
              <path class="hvac-cross-loop-edge" d="M262,${componentY} C338,${componentY} 378,${y} 454,${y}"></path>
              <g class="hvac-cross-loop-target ${loop ? "" : "missing"}" data-hvac-jump-loop-name="${escapeHTML(loopName)}" data-hvac-jump-graph-key="${escapeHTML(componentKey)}">
                <title>${escapeHTML(`${t("action.open")} ${loopName}`)}</title>
                <rect x="454" y="${y - 24}" width="240" height="48" rx="8"></rect>
                <text class="label" x="474" y="${y - 4}">${escapeHTML(truncateText(loopName, 30))}</text>
                <text class="meta" x="474" y="${y + 14}">${escapeHTML(truncateText(loop?.type || t("hvac.viewLoop"), 32))}</text>
              </g>`,
          )
          .join("")}
      </svg>
      <div class="hvac-cross-loop-actions">
        ${rows
          .map(
            ({ loopName, loop }) => `
              <button class="hvac-edit-button" type="button" ${loop ? "" : "disabled"}
                data-hvac-jump-loop-name="${escapeHTML(loopName)}"
                data-hvac-jump-graph-key="${escapeHTML(componentKey)}"
                title="${escapeHTML(loopName)}">
                <span>${escapeHTML(loopName)}</span>
                <small>${escapeHTML(loop?.type || t("hvac.viewLoop"))}</small>
              </button>`,
          )
          .join("")}
      </div>
    </section>`;
}

function truncateText(value, maxLength) {
  const text = String(value || "");
  if (text.length <= maxLength) {
    return text;
  }
  return `${text.slice(0, Math.max(0, maxLength - 3))}...`;
}

function renderHVACComponentSourceReference(component = {}) {
  if (!component.sourceOwner && component.typeFieldIndex === undefined && component.nameFieldIndex === undefined) {
    return "";
  }
  const parts = [
    component.sourceOwner,
    component.typeFieldIndex !== undefined ? `type field ${component.typeFieldIndex}` : "",
    component.nameFieldIndex !== undefined ? `name field ${component.nameFieldIndex}` : "",
    component.expectedObjectType ? `expected ${component.expectedObjectType}` : "",
  ].filter(Boolean);
  return `<small class="hvac-component-source-ref">${escapeHTML(parts.join(" / "))}</small>`;
}

function hvacSourceFieldLabel(component = {}) {
  return [
    component.typeFieldIndex !== undefined ? `type ${component.typeFieldIndex}` : "",
    component.nameFieldIndex !== undefined ? `name ${component.nameFieldIndex}` : "",
  ]
    .filter(Boolean)
    .join(" / ") || "N/A";
}

function componentDisplayName(component = {}) {
  const name = component.objectName || "";
  const label = component.displayLabel || componentFamilyLabel(component.family, component.objectType);
  if (name) {
    return name;
  }
  return label || component.objectType || t("hvac.unknownType", {}, "Unknown HVAC object");
}

function componentMetaLabel(component = {}) {
  return [component.displayLabel || componentFamilyLabel(component.family, component.objectType), component.objectType]
    .filter(Boolean)
    .join(" / ") || t("hvac.unknownType", {}, "Unknown HVAC object");
}

function componentFamilyLabel(family, objectType = "") {
  switch (family) {
    case "fan":
      return "Fan";
    case "cooling_coil":
      return "Cooling Coil";
    case "heating_coil":
      return "Heating Coil";
    case "coil":
      return "Coil";
    case "pump":
      return "Pump";
    case "pipe":
      return "Pipe";
    case "chiller":
      return "Chiller";
    case "boiler":
      return "Boiler";
    case "cooling_tower":
      return "Cooling Tower";
    case "heat_pump":
      return "Heat Pump";
    case "water_heater":
      return "Water Heater";
    case "thermal_storage":
      return "Thermal Storage";
    case "heat_exchanger":
      return "Heat Exchanger";
    case "district_cooling":
      return "District Cooling";
    case "district_heating":
      return "District Heating";
    case "terminal":
      return "Air Terminal";
    case "zone_hvac":
      return "Zone HVAC";
    case "unitary_system":
      return "Unitary System";
    case "outdoor_air":
      return "Outdoor Air";
    case "controller":
      return "Controller";
    case "setpoint_manager":
      return "Setpoint Manager";
    case "availability_manager":
      return "Availability Manager";
    case "plant_component":
      return "Plant Component";
    case "air_distribution":
      return "Air Distribution";
    default:
      return objectType ? "" : "Unknown HVAC object";
  }
}

function isAirTerminalObjectType(objectType = "") {
  return String(objectType || "").trim().toLowerCase().startsWith("airterminal:");
}

function componentVisual(component = {}) {
  const iconKind = hvacVisualKindForFamily(component.family, component.objectType || "");
  return {
    iconKind,
    shortLabel: component.displayLabel || componentFamilyLabel(component.family, component.objectType) || hvacVisualLabel(iconKind, component.objectType || component.objectName || ""),
  };
}

function hvacVisualKindForFamily(family, objectType) {
  switch (family) {
    case "fan":
      return "fan";
    case "cooling_coil":
    case "heating_coil":
    case "coil":
      return "coil";
    case "pump":
      return "pump";
    case "pipe":
      return "pipe";
    case "chiller":
      return "chiller";
    case "boiler":
    case "water_heater":
      return "boiler";
    case "cooling_tower":
      return "tower";
    case "heat_pump":
      return "heat_pump";
    case "heat_exchanger":
      return "heat_exchanger";
    case "district_cooling":
    case "district_heating":
      return "district";
    case "terminal":
      return "terminal";
    case "zone_hvac":
    case "unitary_system":
      return "direct_unit";
    case "outdoor_air":
      return "air";
    case "controller":
    case "setpoint_manager":
    case "availability_manager":
      return "control";
    case "plant_component":
      return "plant";
    default:
      return hvacVisualKindForType(objectType);
  }
}

function hvacVisualKindForType(objectType) {
  const lower = String(objectType || "").toLowerCase();
  if (lower.includes("fan")) {
    return "fan";
  }
  if (lower.includes("coil")) {
    return "coil";
  }
  if (lower.includes("pump")) {
    return "pump";
  }
  if (lower.includes("chiller")) {
    return "chiller";
  }
  if (lower.includes("boiler")) {
    return "boiler";
  }
  if (lower.includes("airterminal") || lower.includes("airdistributionunit")) {
    return "terminal";
  }
  if (lower.startsWith("zonehvac:")) {
    return "direct_unit";
  }
  return "component";
}

function hvacVisualLabel(iconKind, objectType) {
  const lower = String(objectType || "").toLowerCase();
  if (iconKind === "coil" && lower.includes("cooling")) {
    return "Cool Coil";
  }
  if (iconKind === "coil" && lower.includes("heating")) {
    return "Heat Coil";
  }
  switch (iconKind) {
    case "fan":
      return "Fan";
    case "coil":
      return "Coil";
    case "pump":
      return "Pump";
    case "chiller":
      return "Chiller";
    case "boiler":
      return "Boiler";
    case "terminal":
      return "Terminal";
    case "direct_unit":
      return "Direct Unit";
    default:
      return "Component";
  }
}

function renderLoopEquipmentBody(kind, cx, cy, objectType = "") {
  const lower = String(objectType || "").toLowerCase();
  const coilTone = lower.includes("cooling") ? "cooling" : lower.includes("heating") ? "heating" : "mixed";
  switch (kind) {
    case "fan":
      return `
        <g class="hvac-loop-icon fan" aria-hidden="true">
          <circle class="icon-case" cx="${cx}" cy="${cy}" r="18"></circle>
          <path class="fan-blade" d="M${cx},${cy - 3} C${cx + 12},${cy - 19} ${cx + 21},${cy - 4} ${cx + 5},${cy + 1} Z"></path>
          <path class="fan-blade" d="M${cx - 3},${cy + 2} C${cx - 21},${cy + 6} ${cx - 13},${cy + 21} ${cx + 1},${cy + 6} Z"></path>
          <path class="fan-blade" d="M${cx + 2},${cy + 2} C${cx + 8},${cy + 20} ${cx + 21},${cy + 9} ${cx + 5},${cy - 3} Z"></path>
          <circle class="icon-core" cx="${cx}" cy="${cy}" r="4"></circle>
        </g>`;
    case "coil":
      return `
        <g class="hvac-loop-icon coil ${coilTone}" aria-hidden="true">
          <rect class="icon-case" x="${cx - 23}" y="${cy - 15}" width="46" height="30" rx="3"></rect>
          <path class="coil-fin" d="M${cx - 17},${cy - 11} V${cy + 11} M${cx - 9},${cy - 11} V${cy + 11} M${cx - 1},${cy - 11} V${cy + 11} M${cx + 7},${cy - 11} V${cy + 11} M${cx + 15},${cy - 11} V${cy + 11}"></path>
          <path class="coil-line" d="M${cx - 19},${cy + 8} C${cx - 14},${cy - 9} ${cx - 9},${cy - 9} ${cx - 4},${cy + 8} S${cx + 6},${cy + 25} ${cx + 11},${cy + 8} S${cx + 18},${cy - 9} ${cx + 21},${cy + 2}"></path>
        </g>`;
    case "pump":
      return `
        <g class="hvac-loop-icon pump" aria-hidden="true">
          <rect class="icon-case" x="${cx - 23}" y="${cy - 12}" width="42" height="24" rx="10"></rect>
          <circle class="pump-volute" cx="${cx - 12}" cy="${cy}" r="12"></circle>
          <path class="pump-arrow" d="M${cx - 16},${cy} H${cx + 13} M${cx + 7},${cy - 6} L${cx + 15},${cy} L${cx + 7},${cy + 6}"></path>
        </g>`;
    case "pipe":
      return `
        <g class="hvac-loop-icon pipe" aria-hidden="true">
          <path class="pipe-body" d="M${cx - 30},${cy} H${cx + 30}"></path>
          <path class="pipe-body" d="M${cx - 16},${cy - 9} V${cy + 9} M${cx + 16},${cy - 9} V${cy + 9}"></path>
        </g>`;
    case "chiller":
      return `
        <g class="hvac-loop-icon chiller" aria-hidden="true">
          <rect class="icon-case" x="${cx - 24}" y="${cy - 17}" width="48" height="34" rx="4"></rect>
          <rect class="icon-vent" x="${cx - 18}" y="${cy - 11}" width="36" height="7" rx="2"></rect>
          <path class="snow" d="M${cx},${cy - 1} V${cy + 12} M${cx - 7},${cy + 2} L${cx + 7},${cy + 9} M${cx + 7},${cy + 2} L${cx - 7},${cy + 9}"></path>
        </g>`;
    case "tower":
      return `
        <g class="hvac-loop-icon tower" aria-hidden="true">
          <path class="icon-case tower-case" d="M${cx - 20},${cy + 17} L${cx - 13},${cy - 16} H${cx + 13} L${cx + 20},${cy + 17} Z"></path>
          <path class="tower-fill" d="M${cx - 13},${cy - 5} H${cx + 13} M${cx - 16},${cy + 6} H${cx + 16}"></path>
          <circle class="tower-fan" cx="${cx}" cy="${cy - 11}" r="5"></circle>
        </g>`;
    case "heat_pump":
      return `
        <g class="hvac-loop-icon heat-pump" aria-hidden="true">
          <rect class="icon-case" x="${cx - 23}" y="${cy - 16}" width="46" height="32" rx="5"></rect>
          <path class="heat-pump-arrows" d="M${cx - 13},${cy - 3} H${cx + 12} M${cx + 6},${cy - 9} L${cx + 14},${cy - 3} L${cx + 6},${cy + 3} M${cx + 13},${cy + 8} H${cx - 12} M${cx - 6},${cy + 2} L${cx - 14},${cy + 8} L${cx - 6},${cy + 14}"></path>
        </g>`;
    case "boiler":
      return `
        <g class="hvac-loop-icon boiler" aria-hidden="true">
          <rect class="icon-case" x="${cx - 22}" y="${cy - 17}" width="44" height="34" rx="4"></rect>
          <path class="flame" d="M${cx},${cy - 10} C${cx + 11},${cy - 1} ${cx + 7},${cy + 13} ${cx},${cy + 13} C${cx - 8},${cy + 13} ${cx - 11},${cy + 2} ${cx - 3},${cy - 4} C${cx - 1},${cy - 6} ${cx - 1},${cy - 8} ${cx},${cy - 10} Z"></path>
          <path class="flame-core" d="M${cx + 1},${cy - 2} C${cx + 5},${cy + 4} ${cx + 4},${cy + 10} ${cx},${cy + 10} C${cx - 4},${cy + 10} ${cx - 5},${cy + 4} ${cx + 1},${cy - 2} Z"></path>
        </g>`;
    case "heat_exchanger":
      return `
        <g class="hvac-loop-icon heat-exchanger" aria-hidden="true">
          <rect class="icon-case" x="${cx - 23}" y="${cy - 16}" width="46" height="32" rx="5"></rect>
          <path class="heat-exchanger-lines" d="M${cx - 16},${cy - 9} L${cx + 16},${cy + 9} M${cx - 16},${cy + 9} L${cx + 16},${cy - 9} M${cx - 19},${cy} H${cx + 19}"></path>
        </g>`;
    case "district":
      return `
        <g class="hvac-loop-icon district" aria-hidden="true">
          <rect class="icon-case" x="${cx - 22}" y="${cy - 17}" width="44" height="34" rx="4"></rect>
          <path class="district-grid" d="M${cx - 12},${cy + 10} V${cy - 10} H${cx + 12} V${cy + 10} M${cx - 12},${cy - 1} H${cx + 12} M${cx},${cy - 10} V${cy + 10}"></path>
        </g>`;
    case "control":
      return `
        <g class="hvac-loop-icon control" aria-hidden="true">
          <rect class="icon-case" x="${cx - 21}" y="${cy - 15}" width="42" height="30" rx="6"></rect>
          <path class="control-line" d="M${cx - 12},${cy - 5} H${cx + 12} M${cx - 7},${cy} H${cx + 7} M${cx - 12},${cy + 5} H${cx + 12}"></path>
          <circle class="control-dot" cx="${cx - 4}" cy="${cy}" r="2"></circle>
        </g>`;
    case "terminal":
      return `
        <g class="hvac-loop-icon terminal" aria-hidden="true">
          <rect class="duct left" x="${cx - 31}" y="${cy - 6}" width="12" height="12" rx="2"></rect>
          <rect class="duct right" x="${cx + 19}" y="${cy - 6}" width="12" height="12" rx="2"></rect>
          <rect class="icon-case" x="${cx - 21}" y="${cy - 15}" width="42" height="30" rx="4"></rect>
          <path class="terminal-damper" d="M${cx - 12},${cy + 8} L${cx + 12},${cy - 8} M${cx - 12},${cy - 8} H${cx + 12} M${cx - 12},${cy + 8} H${cx + 12}"></path>
        </g>`;
    case "zone":
      return `
        <g class="hvac-loop-icon zone" aria-hidden="true">
          <rect class="icon-case" x="${cx - 23}" y="${cy - 17}" width="46" height="34" rx="3"></rect>
          <path class="room-line" d="M${cx - 11},${cy - 17} V${cy + 17} M${cx + 6},${cy - 17} V${cy + 17} M${cx - 23},${cy - 2} H${cx + 23}"></path>
        </g>`;
    default:
      return `
        <g class="hvac-loop-icon component" aria-hidden="true">
          <rect class="icon-case" x="${cx - 22}" y="${cy - 15}" width="44" height="30" rx="5"></rect>
          <path class="component-mark" d="M${cx - 11},${cy} H${cx + 11} M${cx},${cy - 10} V${cy + 10}"></path>
        </g>`;
  }
}

function renderHVACNodeIcon(kind, cx, cy) {
  const safeKind = escapeHTML(kind || "component");
  switch (kind) {
    case "fan":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <circle class="icon-bg" cx="${cx}" cy="${cy}" r="15"></circle>
          <path class="icon-fill" d="M${cx},${cy - 2} C${cx + 10},${cy - 15} ${cx + 18},${cy - 2} ${cx + 4},${cy + 1} Z"></path>
          <path class="icon-fill" d="M${cx - 2},${cy + 1} C${cx - 17},${cy + 5} ${cx - 10},${cy + 18} ${cx + 1},${cy + 5} Z"></path>
          <path class="icon-fill" d="M${cx + 1},${cy + 2} C${cx + 6},${cy + 17} ${cx + 18},${cy + 8} ${cx + 5},${cy - 2} Z"></path>
          <circle class="icon-dot" cx="${cx}" cy="${cy}" r="3"></circle>
        </g>`;
    case "coil":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <rect class="icon-bg" x="${cx - 15}" y="${cy - 14}" width="30" height="28" rx="6"></rect>
          <path class="icon-line" d="M${cx - 10},${cy + 7} L${cx - 5},${cy - 7} L${cx},${cy + 7} L${cx + 5},${cy - 7} L${cx + 10},${cy + 7}"></path>
        </g>`;
    case "pump":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <circle class="icon-bg" cx="${cx}" cy="${cy}" r="15"></circle>
          <path class="icon-line" d="M${cx - 10},${cy} H${cx + 8}"></path>
          <path class="icon-line" d="M${cx + 3},${cy - 5} L${cx + 9},${cy} L${cx + 3},${cy + 5}"></path>
        </g>`;
    case "chiller":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <rect class="icon-bg" x="${cx - 15}" y="${cy - 14}" width="30" height="28" rx="5"></rect>
          <path class="icon-line" d="M${cx},${cy - 9} V${cy + 9} M${cx - 8},${cy - 5} L${cx + 8},${cy + 5} M${cx + 8},${cy - 5} L${cx - 8},${cy + 5}"></path>
        </g>`;
    case "boiler":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <rect class="icon-bg" x="${cx - 15}" y="${cy - 14}" width="30" height="28" rx="5"></rect>
          <path class="icon-fill" d="M${cx},${cy - 10} C${cx + 9},${cy - 2} ${cx + 6},${cy + 10} ${cx},${cy + 10} C${cx - 7},${cy + 10} ${cx - 9},${cy + 1} ${cx - 3},${cy - 4} C${cx - 1},${cy - 6} ${cx - 1},${cy - 8} ${cx},${cy - 10} Z"></path>
        </g>`;
    case "terminal":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <rect class="icon-bg" x="${cx - 15}" y="${cy - 14}" width="30" height="28" rx="6"></rect>
          <path class="icon-line" d="M${cx - 9},${cy - 5} H${cx + 9} M${cx},${cy - 5} V${cy + 8} M${cx - 6},${cy + 8} H${cx + 6}"></path>
        </g>`;
    case "fan_coil":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <rect class="icon-bg" x="${cx - 15}" y="${cy - 14}" width="30" height="28" rx="6"></rect>
          <circle class="icon-line" cx="${cx - 5}" cy="${cy}" r="6"></circle>
          <path class="icon-line" d="M${cx + 4},${cy - 8} V${cy + 8} M${cx + 9},${cy - 8} V${cy + 8}"></path>
        </g>`;
    case "packaged":
    case "direct_unit":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <rect class="icon-bg" x="${cx - 16}" y="${cy - 13}" width="32" height="26" rx="4"></rect>
          <path class="icon-line" d="M${cx - 10},${cy - 5} H${cx + 10} M${cx - 10},${cy} H${cx + 10} M${cx - 10},${cy + 5} H${cx + 10}"></path>
        </g>`;
    case "refrigerant":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <rect class="icon-bg" x="${cx - 15}" y="${cy - 14}" width="30" height="28" rx="6"></rect>
          <path class="icon-line" d="M${cx - 8},${cy + 7} C${cx - 2},${cy - 8} ${cx + 2},${cy + 8} ${cx + 8},${cy - 7}"></path>
        </g>`;
    case "radiant":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <rect class="icon-bg" x="${cx - 15}" y="${cy - 14}" width="30" height="28" rx="4"></rect>
          <path class="icon-line" d="M${cx - 9},${cy - 6} H${cx + 9} M${cx - 9},${cy} H${cx + 9} M${cx - 9},${cy + 6} H${cx + 9}"></path>
        </g>`;
    case "baseboard":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <rect class="icon-bg" x="${cx - 16}" y="${cy - 8}" width="32" height="16" rx="3"></rect>
          <path class="icon-line" d="M${cx - 11},${cy} H${cx + 11} M${cx - 8},${cy - 4} V${cy + 4} M${cx},${cy - 4} V${cy + 4} M${cx + 8},${cy - 4} V${cy + 4}"></path>
        </g>`;
    case "ideal_loads":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <circle class="icon-bg" cx="${cx}" cy="${cy}" r="15"></circle>
          <path class="icon-line" d="M${cx - 7},${cy + 4} L${cx - 1},${cy - 6} L${cx + 4},${cy + 2} L${cx + 8},${cy - 5}"></path>
        </g>`;
    case "storage":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <rect class="icon-bg" x="${cx - 13}" y="${cy - 15}" width="26" height="30" rx="5"></rect>
          <path class="icon-line" d="M${cx - 7},${cy - 5} H${cx + 7} M${cx - 7},${cy + 4} H${cx + 7}"></path>
        </g>`;
    case "generator":
    case "electric":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <rect class="icon-bg" x="${cx - 15}" y="${cy - 12}" width="30" height="24" rx="5"></rect>
          <path class="icon-line" d="M${cx - 3},${cy - 8} L${cx - 7},${cy + 1} H${cx} L${cx - 3},${cy + 8} L${cx + 8},${cy - 3} H${cx + 1} Z"></path>
        </g>`;
    case "pv":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <rect class="icon-bg" x="${cx - 15}" y="${cy - 12}" width="30" height="24" rx="3"></rect>
          <path class="icon-line" d="M${cx - 15},${cy - 4} H${cx + 15} M${cx - 5},${cy - 12} V${cy + 12} M${cx + 5},${cy - 12} V${cy + 12}"></path>
        </g>`;
    case "water":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <path class="icon-bg" d="M${cx},${cy - 15} C${cx + 10},${cy - 3} ${cx + 12},${cy + 4} ${cx + 6},${cy + 10} C${cx},${cy + 16} ${cx - 10},${cy + 12} ${cx - 11},${cy + 4} C${cx - 12},${cy - 3} ${cx - 7},${cy - 8} ${cx},${cy - 15} Z"></path>
          <path class="icon-line" d="M${cx - 5},${cy + 4} C${cx - 1},${cy + 7} ${cx + 4},${cy + 6} ${cx + 7},${cy + 2}"></path>
        </g>`;
    case "fault":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <path class="icon-bg" d="M${cx},${cy - 15} L${cx + 15},${cy + 13} H${cx - 15} Z"></path>
          <path class="icon-line" d="M${cx},${cy - 5} V${cy + 4} M${cx},${cy + 9} V${cy + 9}"></path>
        </g>`;
    case "plant":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <path class="icon-bg" d="M${cx},${cy - 16} L${cx + 15},${cy - 7} V${cy + 7} L${cx},${cy + 16} L${cx - 15},${cy + 7} V${cy - 7} Z"></path>
          <path class="icon-line" d="M${cx - 8},${cy + 5} C${cx - 3},${cy - 6} ${cx + 3},${cy + 6} ${cx + 8},${cy - 5}"></path>
        </g>`;
    case "air":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <circle class="icon-bg" cx="${cx}" cy="${cy}" r="15"></circle>
          <path class="icon-line" d="M${cx - 10},${cy - 5} C${cx - 3},${cy - 10} ${cx + 7},${cy - 9} ${cx + 10},${cy - 4} M${cx - 11},${cy + 1} H${cx + 8} M${cx - 8},${cy + 7} C${cx - 1},${cy + 11} ${cx + 7},${cy + 10} ${cx + 10},${cy + 5}"></path>
        </g>`;
    case "zone":
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <rect class="icon-bg" x="${cx - 15}" y="${cy - 14}" width="30" height="28" rx="4"></rect>
          <path class="icon-line" d="M${cx - 8},${cy + 8} V${cy - 7} H${cx + 8} V${cy + 8} M${cx - 8},${cy - 1} H${cx + 8} M${cx},${cy - 7} V${cy + 8}"></path>
        </g>`;
    default:
      return `
        <g class="hvac-node-icon ${safeKind}" aria-hidden="true">
          <circle class="icon-bg" cx="${cx}" cy="${cy}" r="15"></circle>
          <path class="icon-line" d="M${cx - 7},${cy} H${cx + 7} M${cx},${cy - 7} V${cy + 7}"></path>
        </g>`;
  }
}

function hvacEditKey(field) {
  return `${field.objectIndex}:${field.fieldIndex}`;
}

function hvacEditLabel(field) {
  if (field.editKind === "availability_schedule") {
    return t("hvac.availability");
  }
  if (field.editKind === "flow") {
    return t("common.flow");
  }
  if (field.editKind === "capacity") {
    return t("common.capacity");
  }
  if (field.editKind === "sequence") {
    return t("common.sequence");
  }
  return field.fieldName || t("common.field");
}

function allHVACEditableFields(hvac = state.report?.hvac) {
  const loops = hvac?.loops || [];
  const loopFields = loops.flatMap((loop) => loopComponents(loop).flatMap((component) => component.editableFields || []));
  const relationFields = (hvac?.zoneRelations || []).flatMap((relation) =>
    [...(relation.terminalUnits || []), ...(relation.zoneEquipment || [])].flatMap((component) => component.editableFields || []),
  );
  const byKey = new Map();
  [...loopFields, ...relationFields].forEach((field) => byKey.set(hvacEditKey(field), field));
  return [...byKey.values()];
}

function findHVACEditableField(key) {
  return allHVACEditableFields().find((field) => hvacEditKey(field) === key) || null;
}

function openHVACApplyDialog(key) {
  const field = findHVACEditableField(key);
  if (!field) {
    return;
  }
  state.hvacApplyField = field;
  state.hvacOutputRequest = null;
  state.hvacApplyPreview = null;
  const listID = "hvacApplyValueSuggestions";
  elements.hvacApplyBody.innerHTML = `
    <section>
      <h4>${escapeHTML(field.objectType)} ${escapeHTML(field.objectName || "")}</h4>
      <p>${escapeHTML(field.impact || t("hvac.editImpactFallback"))}</p>
      <div class="settings-profile-grid">
        <label class="settings-profile-field">
          <span>${t("common.field")}</span>
          <input type="text" value="${escapeHTML(field.fieldName || `${t("common.field")} ${Number(field.fieldIndex) + 1}`)}" readonly />
        </label>
        <label class="settings-profile-field">
          <span>${t("common.current")}</span>
          <input type="text" value="${escapeHTML(field.currentValue || "")}" readonly />
        </label>
        <label class="settings-profile-field">
          <span>${t("common.newValue")}</span>
          <input id="hvacApplyValue" type="text" value="${escapeHTML(field.currentValue || "")}" list="${listID}" />
          <datalist id="${listID}">
            ${(field.suggestedValues || []).map((item) => `<option value="${escapeHTML(item.value || "")}" label="${escapeHTML(item.label || item.source || "")}"></option>`).join("")}
          </datalist>
        </label>
      </div>
    </section>
    <section>
      <h4>${t("common.preview")}</h4>
      <div id="hvacApplyPreviewList" class="profile-apply-preview"><div class="empty">${t("status.runPreview")}</div></div>
    </section>`;
  elements.hvacApplyStatus.textContent = t("status.reviewBeforeApplying");
  elements.hvacConfirmApply.disabled = true;
  elements.hvacApplyDialog.classList.remove("hidden");
  elements.hvacApplyBody.querySelector("#hvacApplyValue")?.focus();
}

function openHVACOutputDialog({ keyValue, variableName }) {
  const variables = hvacNodeOutputVariables(state.report?.hvac);
  const selectedVariable = variables.find((item) => item.variableName === variableName) || variables[0] || { variableName };
  state.hvacApplyField = null;
  state.hvacOutputRequest = {
    keyValue: keyValue || state.activeHVACNodeName || "",
    variableName: selectedVariable.variableName || variableName || "",
    reportingFrequency: "Hourly",
    scheduleName: "",
  };
  state.hvacApplyPreview = null;
  elements.hvacApplyBody.innerHTML = `
    <section>
      <h4>${t("hvac.addOutputVariable")}</h4>
      <p>${t("hvac.outputRequestImpact")}</p>
      <div class="settings-profile-grid">
        <label class="settings-profile-field">
          <span>${t("hvac.keyValue")}</span>
          <input id="hvacOutputKeyValue" type="text" value="${escapeHTML(state.hvacOutputRequest.keyValue)}" readonly title="${escapeHTML(state.hvacOutputRequest.keyValue)}" />
        </label>
        <label class="settings-profile-field">
          <span>${t("hvac.outputVariable")}</span>
          <select id="hvacOutputVariable">
            ${variables
              .map(
                (item) =>
                  `<option value="${escapeHTML(item.variableName)}" ${item.variableName === state.hvacOutputRequest.variableName ? "selected" : ""}>${escapeHTML(item.variableName)}${item.units ? ` [${escapeHTML(item.units)}]` : ""}</option>`,
              )
              .join("")}
          </select>
        </label>
        <label class="settings-profile-field">
          <span>${t("hvac.reportingFrequency")}</span>
          <select id="hvacOutputFrequency">
            ${["Hourly", "Timestep", "Detailed", "Daily", "Monthly", "RunPeriod", "Annual"]
              .map((item) => `<option value="${item}" ${item === state.hvacOutputRequest.reportingFrequency ? "selected" : ""}>${item}</option>`)
              .join("")}
          </select>
        </label>
        <label class="settings-profile-field">
          <span>${t("hvac.scheduleOptional")}</span>
          <input id="hvacOutputSchedule" type="text" value="" />
        </label>
      </div>
    </section>
    <section>
      <h4>${t("common.preview")}</h4>
      <div id="hvacApplyPreviewList" class="profile-apply-preview"><div class="empty">${t("status.runPreview")}</div></div>
    </section>`;
  elements.hvacApplyStatus.textContent = t("status.reviewBeforeApplying");
  elements.hvacConfirmApply.disabled = true;
  elements.hvacApplyDialog.classList.remove("hidden");
  elements.hvacApplyBody.querySelector("#hvacOutputVariable")?.focus();
}

function closeHVACApplyDialog() {
  elements.hvacApplyDialog.classList.add("hidden");
  state.hvacOutputRequest = null;
}

async function previewHVACApply() {
  const request = hvacApplyRequest();
  if (!request) {
    return;
  }
  try {
    elements.hvacApplyStatus.textContent = t("status.buildingPreview");
    const preview = await callHVACApplyAPI("PreviewHVACApplyText", "/api/hvac-apply-preview", request);
    state.hvacApplyPreview = preview;
    renderHVACApplyPreview(preview);
    elements.hvacConfirmApply.disabled = !preview.canApply;
    elements.hvacApplyStatus.textContent = preview.canApply ? t("status.previewReady") : t("status.previewBlocking");
  } catch (error) {
    elements.hvacApplyStatus.textContent = error?.message || String(error);
    elements.hvacConfirmApply.disabled = true;
  }
}

async function applyHVACEdit(event) {
  event.preventDefault();
  const request = hvacApplyRequest();
  if (!request) {
    return;
  }
  try {
    elements.hvacApplyStatus.textContent = t("status.applyHVAC");
    const result = await callHVACApplyAPI("ApplyHVACText", "/api/hvac-apply", request);
    window.dispatchEvent(new CustomEvent("idfAnalyzer:hvacApplied", { detail: result }));
    closeHVACApplyDialog();
  } catch (error) {
    elements.hvacApplyStatus.textContent = error?.message || String(error);
  }
}

function hvacApplyRequest() {
  if (state.hvacOutputRequest) {
    const keyValue = elements.hvacApplyBody.querySelector("#hvacOutputKeyValue")?.value ?? state.hvacOutputRequest.keyValue;
    const variableName = elements.hvacApplyBody.querySelector("#hvacOutputVariable")?.value ?? state.hvacOutputRequest.variableName;
    const reportingFrequency = elements.hvacApplyBody.querySelector("#hvacOutputFrequency")?.value ?? state.hvacOutputRequest.reportingFrequency;
    const scheduleName = elements.hvacApplyBody.querySelector("#hvacOutputSchedule")?.value ?? "";
    return {
      changes: [],
      outputVariables: [
        {
          keyValue,
          variableName,
          reportingFrequency,
          scheduleName,
        },
      ],
    };
  }
  const field = state.hvacApplyField;
  if (!field) {
    return null;
  }
  const value = elements.hvacApplyBody.querySelector("#hvacApplyValue")?.value ?? "";
  return {
    changes: [
      {
        objectIndex: Number(field.objectIndex),
        fieldIndex: Number(field.fieldIndex),
        value,
      },
    ],
  };
}

async function callHVACApplyAPI(methodName, endpoint, request) {
  const api = backend();
  if (api && typeof api[methodName] === "function") {
    return api[methodName](elements.idfInput.value, request);
  }
  const response = await fetch(endpoint, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text: elements.idfInput.value, apply: request }),
  });
  if (!response.ok) {
    throw new Error(await response.text());
  }
  return response.json();
}

function renderHVACApplyPreview(preview) {
  const list = elements.hvacApplyBody.querySelector("#hvacApplyPreviewList");
  if (!list) {
    return;
  }
  const changes = preview?.changes || [];
  const warnings = preview?.warnings || [];
  list.innerHTML = `
    ${warnings.map(renderHVACApplyWarning).join("")}
    ${
      changes.length
        ? changes.map(renderHVACApplyChange).join("")
        : `<div class="empty">${warnings.length ? t("status.noChangesCanApply") : t("hvac.noFieldChanges")}</div>`
    }`;
}

function renderHVACApplyChange(change) {
  return `
    <div class="profile-apply-change">
      <strong>${escapeHTML(change.message || "")}</strong>
      <span>${escapeHTML(change.objectType || "")} ${escapeHTML(change.objectName || "")} / ${escapeHTML(change.fieldName || "")}</span>
    </div>`;
}

function renderHVACApplyWarning(warning) {
  return `
    <div class="profile-warning ${escapeHTML(warning.severity || "warning")}">
      <strong>${escapeHTML(warning.code || "warning")}</strong>
      <span>${escapeHTML(warning.message || "")}</span>
    </div>`;
}
