import { backend, elements, escapeHTML, state } from "./state.js";
import { t } from "./i18n.js";

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
  elements.hvacGraph?.addEventListener("click", (event) => {
    if (event.target.closest("[data-jump-object-index]")) {
      return;
    }
    const scaleButton = event.target.closest("[data-hvac-graph-scale]");
    if (scaleButton) {
      state.hvacGraphScale = hvacGraphScaleMode(scaleButton.dataset.hvacGraphScale || "fit");
      renderHVAC();
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
    const graphTarget = event.target.closest("[data-hvac-graph-key]");
    if (graphTarget) {
      selectHVACGraphKey(graphTarget.dataset.hvacGraphKey || "");
      return;
    }
    const nodeButton = event.target.closest("[data-hvac-node]");
    if (!nodeButton) {
      return;
    }
    state.activeHVACNodeName = nodeButton.dataset.hvacNode || "";
    state.activeHVACGraphKey = `node:${state.activeHVACNodeName}`;
    renderHVAC();
  });
  elements.hvacGraph?.addEventListener("keydown", (event) => {
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
    const loopJump = event.target.closest("[data-hvac-jump-loop-name]");
    if (loopJump) {
      jumpToHVACLoopByName(loopJump.dataset.hvacJumpLoopName || "", loopJump.dataset.hvacJumpGraphKey || "");
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
    state.activeHVACNodeName = nodeButton.dataset.hvacNode || "";
    state.activeHVACGraphKey = `node:${state.activeHVACNodeName}`;
    renderHVAC();
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
  state.activeHVACGraphKey = key;
  state.activeHVACNodeName = key.startsWith("node:") ? key.slice(5) : "";
  renderHVAC();
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
  const debugEnabled = hvacDebugEnabled();
  elements.hvacSummary.innerHTML = `
    <div class="hvac-view-tabs" role="tablist" aria-label="${escapeHTML(t("tab.hvac", {}, "HVAC"))}">
      ${renderHVACViewTab("services", t("hvac.zoneServices", {}, "Zone Services"), servicePaths.length)}
      ${renderHVACViewTab("loop", t("hvac.loops", {}, "Loops"), loops.length)}
      ${renderHVACViewTab("couplings", t("hvac.couplings", {}, "Couplings"), couplings.length)}
      ${renderHVACViewTab("diagnostics", t("hvac.warnings"), hvac.warningCount || 0)}
      ${debugEnabled ? renderHVACViewTab("debug", t("hvac.debug", {}, "Debug"), (hvac.ruleGraph?.edges || []).length) : ""}
    </div>
    <div class="hvac-navigator">
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
    <button class="hvac-nav-choice ${selected ? "active" : ""}" type="button" data-hvac-service-subject-key="${escapeHTML(key)}">
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

function renderHVACRelationPicker(relations, active) {
  const selectedKey = state.activeHVACGraphKey?.startsWith("zone:") || state.activeHVACGraphKey?.startsWith("space:") ? state.activeHVACGraphKey : "";
  return `
    <details class="hvac-nav-card ${active ? "active" : ""}">
      <summary>
        <span>
          <strong>${escapeHTML(t("hvac.zoneRelations"))}</strong>
          <em>${escapeHTML(t("hvac.zoneRelationHelp"))}</em>
        </span>
        <b>${escapeHTML(relations.length)}</b>
      </summary>
      <div class="hvac-nav-menu">
        <button class="hvac-nav-choice ${active && !selectedKey ? "active" : ""}" type="button" data-hvac-open-view="relation">
          <span>${escapeHTML(t("hvac.allZoneRelations"))}</span>
          <small>${escapeHTML(t("hvac.serviceRelation"))}</small>
        </button>
        ${
          relations.length
            ? relations.map((relation) => renderHVACRelationChoice(relation, selectedKey)).join("")
            : `<div class="empty compact">${escapeHTML(t("hvac.noZoneRelations"))}</div>`
        }
      </div>
    </details>`;
}

function renderHVACRelationChoice(relation, selectedKey) {
  const key = relationSelectionKey(relation);
  const selected = state.activeHVACView === "relation" && selectedKey === key;
  const meta = [
    relation.relationScope === "space" ? "SpaceHVAC" : "",
    [...new Set([...(relation.airLoopNames || []), ...(relation.plantLoopNames || [])])].join(", "),
  ]
    .filter(Boolean)
    .join(" / ") || t("hvac.noTerminal");
  return `
    <button class="hvac-nav-choice ${selected ? "active" : ""}" type="button" data-hvac-relation-key="${escapeHTML(key)}">
      <span>${escapeHTML(relationDisplayName(relation))}</span>
      <small>${escapeHTML(meta)}</small>
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

function relationSelectionKey(relation = {}) {
  if (relation.spaceName) {
    return `space:${relation.spaceName}`;
  }
  return relation.zoneName ? `zone:${relation.zoneName}` : "";
}

function relationDisplayName(relation = {}) {
  if (relation.spaceName) {
    return relation.zoneName ? `${relation.spaceName} / ${relation.zoneName}` : relation.spaceName;
  }
  return relation.zoneName || t("common.blank");
}

function renderRelationSubjectLink(relation = {}) {
  if (relation.spaceName) {
    return renderObjectLink(relation.spaceObjectIndex, "Space");
  }
  return renderObjectLink(relation.zoneObjectIndex, "Zone");
}

function handleHVACNavigationClick(event) {
  const loopJump = event.target.closest("[data-hvac-jump-loop-name]");
  if (loopJump) {
    jumpToHVACLoopByName(loopJump.dataset.hvacJumpLoopName || "", loopJump.dataset.hvacJumpGraphKey || "");
    return;
  }
  const loopButton = event.target.closest("[data-hvac-loop-id]");
  if (loopButton) {
    state.activeHVACView = "loop";
    state.activeHVACLoopId = loopButton.dataset.hvacLoopId || "";
    state.activeHVACGraphKey = "";
    state.activeHVACNodeName = "";
    notifyHVACSelectionChanged();
    renderHVAC();
    return;
  }
  const relationButton = event.target.closest("[data-hvac-relation-key]");
  if (relationButton) {
    state.activeHVACView = "services";
    state.activeHVACGraphKey = relationButton.dataset.hvacRelationKey || "";
    state.activeHVACNodeName = "";
    renderHVAC();
    return;
  }
  const serviceButton = event.target.closest("[data-hvac-service-subject-key]");
  if (serviceButton) {
    state.activeHVACView = "services";
    state.activeHVACGraphKey = `subject:${serviceButton.dataset.hvacServiceSubjectKey || ""}`;
    state.activeHVACNodeName = "";
    renderHVAC();
    return;
  }
  const viewButton = event.target.closest("[data-hvac-open-view]");
  if (viewButton) {
    state.activeHVACView = viewButton.dataset.hvacOpenView || "services";
    state.activeHVACGraphKey = "";
    state.activeHVACNodeName = "";
    if (state.activeHVACView === "loop" && !state.activeHVACLoopId) {
      state.activeHVACLoopId = (state.report?.hvac?.loops || [])[0]?.id || "";
    }
    renderHVAC();
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
  state.activeHVACView = "loop";
  state.activeHVACLoopId = loop.id;
  state.activeHVACGraphKey = graphKey || `loop:${loop.id}`;
  state.activeHVACNodeName = state.activeHVACGraphKey.startsWith("node:") ? state.activeHVACGraphKey.slice(5) : "";
  notifyHVACSelectionChanged();
  renderHVAC();
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
  elements.hvacGraph.innerHTML = `
    <div class="hvac-loop-title">
      <div class="hvac-loop-heading">
        <h3>${escapeHTML(loop.name || loop.type)}</h3>
        <span>${escapeHTML(loop.type)} ${renderObjectLink(loop.objectIndex, loop.type)}</span>
        ${renderLoopRelatedSystemChips(loop)}
      </div>
      <div class="hvac-loop-meta">
        <span>${escapeHTML(t("count.zones", { count: (loop.relatedZones || []).length }))}</span>
        <span>${escapeHTML(t("hvac.crossLoopLinks", { count: (loop.relatedLoops || []).length }))}</span>
        ${renderHVACGraphScaleControls()}
      </div>
    </div>
    ${renderHVACLoopDiagram(loop)}
    ${renderHVACLoopGraphDetail(loop)}
    ${renderHVACLoopSupportingAssets(loop, hvacServiceModel().couplings || [])}
    ${renderCrossLoopRelations(loop)}`;
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

function ruleEdgesForRelation(relation = {}) {
  const candidates = [];
  if (relation.spaceName) {
    candidates.push({ objectType: "Space", objectName: relation.spaceName, objectIndex: relation.spaceObjectIndex });
  }
  if (relation.zoneName) {
    candidates.push({ objectType: "Zone", objectName: relation.zoneName, objectIndex: relation.zoneObjectIndex });
  }
  for (const name of relation.airLoopNames || []) {
    candidates.push({ objectType: "AirLoopHVAC", objectName: name });
  }
  for (const name of relation.plantLoopNames || []) {
    candidates.push({ objectType: "PlantLoop", objectName: name });
  }
  for (const name of relation.condenserLoopNames || []) {
    candidates.push({ objectType: "CondenserLoop", objectName: name });
  }
  for (const component of [...(relation.terminalUnits || []), ...(relation.zoneEquipment || []), ...(relation.plantEquipment || [])]) {
    candidates.push({ objectType: component.objectType, objectName: component.objectName, objectIndex: component.objectIndex });
  }
  return uniqueRuleEdges(
    hvacRuleEdges().filter((edge) =>
      candidates.some((candidate) => ruleEdgeTouchesObject(edge, candidate.objectType, candidate.objectName, candidate.objectIndex)),
    ),
  );
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

function ruleEdgeCountLabel(edges = []) {
  if (!edges.length) {
    return "";
  }
  return `${edges.length} trace link${edges.length === 1 ? "" : "s"}`;
}

function ruleEdgeSummary(edges = []) {
  return [...new Set(edges.map((edge) => edge.ruleId).filter(Boolean))].slice(0, 4).join(" / ");
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

function serviceGraphNodeKey(path, role) {
  return `service-node:${path.id}:${role}`;
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
  const selectedSubject = state.activeHVACGraphKey?.startsWith("subject:") ? state.activeHVACGraphKey.slice("subject:".length) : "";
  const paths = summaries
    .filter((summary) => !selectedSubject || servedSubjectKey(summary.servedSubject || summary) === selectedSubject)
    .flatMap((summary) => summary.paths || [])
    .filter((path) => servicePathMatchesQuery(path, query));
  elements.hvacGraph.innerHTML = paths.length
    ? `
      ${renderHVACServiceTable(paths, hvac)}
      ${renderHVACServiceGraph(paths, hvacServiceModel(hvac).couplings || [])}
      ${renderHVACServiceGraphDetail(paths, hvacServiceModel(hvac).couplings || [])}`
    : `<div class="empty">${t("hvac.noMatchingServices", {}, "No matching zone services")}</div>`;
}

function renderHVACServiceTable(paths, hvac) {
  return `
    <section class="hvac-relation-table-shell hvac-service-table-shell">
      <div class="hvac-section-head compact">
        <h3>${escapeHTML(t("hvac.zoneServices", {}, "Zone Services"))}</h3>
        <span>${escapeHTML(t("count.items", { count: paths.length }, `${paths.length} paths`))}</span>
      </div>
      <div class="hvac-service-table" role="table" aria-label="${escapeHTML(t("hvac.zoneServices", {}, "Zone Services"))}">
        <div class="hvac-service-table-row head" role="row">
          <span>Zone</span>
          <span>Service</span>
          <span>${escapeHTML(t("hvac.delivery", {}, "Delivery"))}</span>
          <span>${escapeHTML(t("hvac.source", {}, "Source"))}</span>
          <span>${escapeHTML(t("hvac.airSystem", {}, "Air system"))}</span>
          <span>${escapeHTML(t("hvac.coupledSystems", {}, "Coupled systems"))}</span>
          <span>${escapeHTML(t("hvac.issues", {}, "Issues"))}</span>
        </div>
        ${paths.map((path) => renderHVACServiceRow(path, hvac)).join("")}
      </div>
    </section>`;
}

function renderHVACServiceRow(path, hvac) {
  const pathKey = servicePathGraphKey(path);
  const sourceText = servicePathSourceText(path);
  const coupledText = servicePathCoupledText(path, hvacServiceModel(hvac).couplings || []);
  const issues = (path.issues || []).map((issue) => issue.code || issue.message).filter(Boolean);
  return `
    <div class="hvac-service-table-row ${graphSelectionClass(pathKey, [`subject:${servedSubjectKey(path.servedSubject || path)}`])}" role="row" tabindex="0" data-hvac-graph-key="${escapeHTML(pathKey)}">
      <span>
        <strong>${escapeHTML(servedSubjectLabel(path.servedSubject || path))}</strong>
        <small>${escapeHTML(path.pathType || "")}</small>
      </span>
      <span>${escapeHTML(serviceKindLabel(path.serviceKind))}</span>
      <span>${escapeHTML(deliveryLabel(path))}</span>
      <span>${escapeHTML(sourceText || "-")}</span>
      <span>${escapeHTML(path.airLoop?.name || "-")}</span>
      <span>${escapeHTML(coupledText || "-")}</span>
      <span>${escapeHTML(issues.join(", ") || "-")}</span>
    </div>`;
}

function renderHVACServiceGraph(paths, couplings) {
  const graph = buildServiceGraph(paths, couplings);
  const width = graph.width;
  const height = graph.height;
  return `
    <div class="hvac-graphic-shell hvac-service-shell ${hvacGraphScaleClass()}" style="--hvac-graph-width: ${width}px">
      <div class="hvac-graph-toolbar">
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

function buildServiceGraph(paths, couplings) {
  const nodes = [];
  const links = [];
  const groups = [];
  const couplingById = new Map((couplings || []).map((coupling) => [coupling.id, coupling]));
  const rowGap = 132;
  const baseY = 86;
  const width = 1160;
  paths.forEach((path, index) => {
    const y = baseY + index * rowGap;
    groups.push({ y: y - 54, height: 108, label: `${serviceKindLabel(path.serviceKind)} / ${pathTypeLabel(path.pathType)}` });
    const specs = servicePathNodeSpecs(path);
    const rowNodes = specs.map((spec, specIndex) => {
      const x = serviceNodeX(specIndex, specs.length, width);
      const node = {
        key: serviceGraphNodeKey(path, spec.role),
        pathId: path.id,
        role: spec.role,
        kind: spec.kind,
        x,
        y,
        width: spec.width || 156,
        height: 58,
        label: spec.label,
        meta: spec.meta,
        iconKind: spec.iconKind,
        shortLabel: spec.shortLabel,
        ref: spec.ref,
        path,
        relatedKeys: [servicePathGraphKey(path), `subject:${servedSubjectKey(path.servedSubject || path)}`],
        ports: graphPortsForNode(spec.kind),
      };
      nodes.push(node);
      return node;
    });
    rowNodes.forEach((node, nodeIndex) => {
      const next = rowNodes[nodeIndex + 1];
      if (next) {
        links.push({
          key: `service-link:${path.id}:${node.role}->${next.role}`,
          from: node,
          to: next,
          medium: serviceEdgeMedium(node, next, path),
          label: serviceEdgeLabel(serviceEdgeMedium(node, next, path)),
          path,
          relatedKeys: [servicePathGraphKey(path), node.key, next.key],
        });
      }
    });
    const plantNode = rowNodes.find((node) => ["plant_loop", "source", "refrigerant_system"].includes(node.role));
    const supportCouplings = (path.supportingCouplingIds || []).map((id) => couplingById.get(id)).filter(Boolean);
    const visibleCouplings = supportCouplings.slice(0, 3);
    visibleCouplings.forEach((coupling, supportIndex) => {
      const anchor = plantNode || rowNodes[0];
      const supportNode = {
        key: `coupling-node:${path.id}:${coupling.id}`,
        pathId: path.id,
        role: "coupling",
        kind: coupling.couplingType || "coupling",
        x: anchor.x + supportIndex * 176,
        y: y + 68,
        width: 160,
        height: 48,
        label: coupling.object?.displayName || coupling.object?.objectName || coupling.role,
        meta: couplingRoleLabel(coupling),
        iconKind: iconKindForCoupling(coupling),
        shortLabel: couplingShortLabel(coupling),
        coupling,
        path,
        relatedKeys: [servicePathGraphKey(path), anchor.key],
        ports: graphPortsForNode("support"),
      };
      nodes.push(supportNode);
      links.push({
        key: `support-link:${path.id}:${coupling.id}`,
        from: supportNode,
        to: anchor,
        medium: (coupling.mediums || [])[0] || "control",
        label: serviceEdgeLabel((coupling.mediums || [])[0] || "control"),
        path,
        coupling,
        support: true,
        relatedKeys: [servicePathGraphKey(path), supportNode.key, anchor.key],
      });
    });
    if (supportCouplings.length > visibleCouplings.length) {
      const anchor = plantNode || rowNodes[0];
      nodes.push({
        key: `coupling-node:${path.id}:more`,
        pathId: path.id,
        role: "coupling",
        kind: "support_chip",
        x: anchor.x + visibleCouplings.length * 176,
        y: y + 68,
        width: 150,
        height: 42,
        label: `+${supportCouplings.length - visibleCouplings.length}`,
        meta: t("hvac.supportingAssets", {}, "supporting assets"),
        iconKind: "component",
        shortLabel: `+${supportCouplings.length - visibleCouplings.length}`,
        path,
        relatedKeys: [servicePathGraphKey(path), anchor.key],
        ports: graphPortsForNode("support"),
      });
    }
  });
  return { width, height: Math.max(260, baseY + paths.length * rowGap + 92), nodes, links, groups };
}

function servicePathNodeSpecs(path) {
  const zoneSpec = {
    role: "zone",
    kind: path.servedSubject?.kind === "space" ? "space" : "zone",
    label: servedSubjectLabel(path.servedSubject || path),
    meta: path.servedSubject?.kind === "space" ? "Space" : "Zone",
    iconKind: "zone",
    shortLabel: path.servedSubject?.kind === "space" ? "Space" : "Zone",
    ref: path.servedSubject,
  };
  const deliverySpec = {
    role: "delivery",
    kind: path.deliveryEquipment?.deliveryType || path.delivery?.deliveryType || "direct_zone_unit",
    label: path.delivery?.displayName || path.delivery?.objectName || deliveryLabel(path),
    meta: path.deliveryEquipment?.displayFamily || path.delivery?.displayFamily || path.delivery?.objectType || "",
    iconKind: iconKindForDelivery(path.deliveryEquipment?.deliveryType || path.delivery?.deliveryType),
    shortLabel: path.deliveryEquipment?.displayFamily || deliveryLabel(path),
    ref: path.delivery,
  };
  const plantSpec = path.plantLoop
    ? {
        role: "plant_loop",
        kind: "plant_loop",
        label: path.plantLoop.name,
        meta: "PlantLoop",
        iconKind: "plant",
        shortLabel: "Plant",
        ref: path.plantLoop,
      }
    : path.sourceSystem
      ? {
          role: "source",
          kind: path.sourceSystem.type || "source",
          label: path.sourceSystem.name,
          meta: sourceSystemLabel(path.sourceSystem),
          iconKind: iconKindForSource(path.sourceSystem),
          shortLabel: sourceSystemLabel(path.sourceSystem),
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
        shortLabel: "Air Loop",
        ref: path.airLoop,
      }
    : null;
  const refrigerantSpec = path.refrigerantSystem
    ? {
        role: "refrigerant_system",
        kind: "refrigerant_system",
        label: path.refrigerantSystem.name,
        meta: "Refrigerant System",
        iconKind: "refrigerant",
        shortLabel: "VRF",
        ref: path.refrigerantSystem,
      }
    : null;
  const conditioningSpec = (path.conditioning || [])[0]
    ? {
        role: "conditioning",
        kind: "coil",
        label: (path.conditioning || [])[0].displayName || (path.conditioning || [])[0].objectName,
        meta: (path.conditioning || [])[0].displayFamily || (path.conditioning || [])[0].objectType,
        iconKind: "coil",
        shortLabel: "Coil",
        ref: (path.conditioning || [])[0],
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

function serviceNodeX(index, count, width) {
  if (count <= 1) {
    return width / 2;
  }
  const left = 110;
  const right = width - 110;
  return left + ((right - left) * index) / (count - 1);
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
    <g class="hvac-service-lane">
      <rect x="18" y="${group.y}" width="1124" height="${group.height}" rx="6"></rect>
      <text x="32" y="${group.y + 22}">${escapeHTML(group.label)}</text>
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
  const d = link.support ? orthogonalPath(fromPort, toPort, "vertical") : orthogonalPath(fromPort, toPort);
  const medium = mediumClass(link.medium);
  return `
    <g class="hvac-service-link-group ${selected}">
      <path class="hvac-graph-link service ${medium} ${link.support ? "support" : ""} ${selected}"
        data-hvac-graph-key="${escapeHTML(link.key)}"
        d="${escapeHTML(d)}"
        marker-end="url(#hvacServiceArrow)">
        <title>${escapeHTML(`${link.from.label || ""} -> ${link.to.label || ""} / ${link.label || ""}`)}</title>
      </path>
      <text class="hvac-edge-label" x="${(fromPort.x + toPort.x) / 2}" y="${(fromPort.y + toPort.y) / 2 - 7}">${escapeHTML(link.label || "")}</text>
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

function orthogonalPath(from, to, mode = "horizontal") {
  if (mode === "vertical") {
    const midY = (from.y + to.y) / 2;
    return `M${from.x},${from.y} V${midY} H${to.x} V${to.y}`;
  }
  const midX = (from.x + to.x) / 2;
  return `M${from.x},${from.y} H${midX} V${to.y} H${to.x}`;
}

function renderHVACServiceGraphDetail(paths, couplings) {
  const selected = selectedServiceGraphItem(paths, couplings);
  if (!selected) {
    const zones = new Set(paths.map((path) => servedSubjectKey(path.servedSubject || path))).size;
    const deliveryTypes = new Set(paths.map((path) => path.deliveryEquipment?.deliveryType || path.delivery?.deliveryType).filter(Boolean)).size;
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
          <div><span>${escapeHTML(t("hvac.couplings", {}, "Couplings"))}</span><strong>${escapeHTML(couplings.length)}</strong></div>
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
    const raw = key.slice("service-node:".length);
    const roleIndex = raw.lastIndexOf(":");
    const pathID = roleIndex >= 0 ? raw.slice(0, roleIndex) : raw;
    const role = roleIndex >= 0 ? raw.slice(roleIndex + 1) : "";
    const path = paths.find((item) => item.id === pathID);
    if (!path) {
      return null;
    }
    const node = servicePathNodeSpecs(path).find((item) => item.role === role);
    return node ? { kind: "node", node, path } : null;
  }
  if (key.startsWith("service-link:") || key.startsWith("support-link:")) {
    const raw = key.slice(key.indexOf(":") + 1);
    const path = paths.find((item) => raw.startsWith(`${item.id}:`));
    return path ? { kind: "link", path } : null;
  }
  return null;
}

function renderSelectedServicePathDetail(path, couplings) {
  const pathCouplings = (path.supportingCouplingIds || []).map((id) => (couplings || []).find((coupling) => coupling.id === id)).filter(Boolean);
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
  const connected = servicePathConnectedSystems(path);
  return `
    <section class="hvac-graph-detail">
      <div class="hvac-section-head">
        <h3>${escapeHTML(node.label || ref.displayName || ref.name || "")}</h3>
        <span>${escapeHTML(node.meta || ref.objectType || node.kind || "")}</span>
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
      ${renderHVACTraceDrawer(path.traceIds || [])}
    </section>`;
}

function renderSelectedCouplingDetail(coupling, path) {
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
      ${path ? `<p class="hvac-detail-note">${escapeHTML(t("hvac.affectsPath", {}, "Affects selected service path"))}: ${escapeHTML(servedSubjectLabel(path.servedSubject || path))}</p>` : ""}
      ${renderHVACTraceDrawer(coupling.traceIds || [])}
    </section>`;
}

function renderHVACCouplings(hvac, query) {
  const serviceModel = hvacServiceModel(hvac);
  const couplings = (serviceModel.couplings || []).filter((coupling) => couplingMatchesQuery(coupling, query));
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
  const edges = (hvac.ruleGraph?.edges || []).filter((edge) => ruleEdgeSearchFields(edge).join(" ").toLowerCase().includes(query || ""));
  elements.hvacGraph.innerHTML = `
    <section class="hvac-graph-detail">
      <div class="hvac-section-head">
        <h3>${escapeHTML(t("hvac.debug", {}, "Debug"))}</h3>
        <span>${escapeHTML(edges.length)}</span>
      </div>
      ${renderHVACRuleTraceList(edges)}
    </section>`;
}

function renderHVACRelations(hvac, query) {
  const relations = (hvac.zoneRelations || []).filter((relation) => zoneRelationMatchesQuery(relation, query));
  elements.hvacGraph.innerHTML = relations.length
    ? `
      ${renderHVACRelationTable(relations)}
      ${renderHVACRelationGraph(relations)}
      ${renderHVACRelationGraphDetail(relations)}`
    : `<div class="empty">${t("hvac.noMatchingRelations")}</div>`;
}

function renderHVACRelationTable(relations) {
  return `
    <section class="hvac-relation-table-shell">
      <div class="hvac-section-head compact">
        <h3>${escapeHTML(t("hvac.zoneRelations"))}</h3>
        <span>${escapeHTML(t("count.zones", { count: relations.length }, `${relations.length} zones`))}</span>
      </div>
      <div class="hvac-relation-table" role="table" aria-label="${escapeHTML(t("hvac.zoneRelations"))}">
        <div class="hvac-relation-table-row head" role="row">
          <span>Zone</span>
          <span>${escapeHTML(t("hvac.delivery", {}, "Delivery"))}</span>
          <span>AirLoop</span>
          <span>${escapeHTML(t("hvac.sourceCoupledLoops", {}, "Source / Coupled loops"))}</span>
          <span>${escapeHTML(t("hvac.issues", {}, "Issues"))}</span>
        </div>
        ${relations.map(renderHVACZoneRelation).join("")}
      </div>
    </section>`;
}

function renderHVACRelationGraph(relations) {
  const graph = buildRelationGraph(relations);
  const width = graph.width;
  const height = graph.height;
  return `
    <div class="hvac-graphic-shell hvac-relation-shell ${hvacGraphScaleClass()}" style="--hvac-graph-width: ${width}px">
      <div class="hvac-graph-toolbar">
        ${renderHVACGraphScaleControls()}
      </div>
      <svg class="hvac-relation-svg" viewBox="0 0 ${width} ${height}" role="img" aria-label="HVAC system-zone relation graph">
        <defs>
          <marker id="hvacRelationArrow" markerWidth="10" markerHeight="10" refX="8" refY="3" orient="auto" markerUnits="strokeWidth">
            <path d="M0,0 L8,3 L0,6 Z" class="hvac-arrow-marker"></path>
          </marker>
        </defs>
        ${graph.columns
          .map(
            (column) => `
              <text class="hvac-column-title" x="${column.x}" y="42" text-anchor="middle">${escapeHTML(column.label)}</text>`,
          )
          .join("")}
        ${graph.links.map(renderRelationLink).join("")}
        ${graph.nodes.map(renderRelationNode).join("")}
      </svg>
      <div class="hvac-legend">
        <span><i class="hvac-legend-source"></i>${t("hvac.legendSource")}</span>
        <span><i class="hvac-legend-air"></i>${t("hvac.legendAir")}</span>
        <span><i class="hvac-legend-terminal"></i>${t("hvac.legendTerminal")}</span>
        <span><i class="hvac-legend-zone"></i>Zone</span>
      </div>
    </div>`;
}

function renderRelationLink(link) {
  const related = selectionRelatedToLink(link);
  const selected = state.activeHVACGraphKey === link.key ? "selected" : related ? "related" : state.activeHVACGraphKey ? "dimmed" : "";
  const title = `${link.from.label || ""} -> ${link.to.label || ""} (${t("hvac.relatedZoneCount", { count: (link.relations || []).length })})`;
  return `
    <path class="hvac-graph-link ${escapeHTML(link.kind)} ${selected}"
      data-hvac-graph-key="${escapeHTML(link.key)}"
      d="M${link.from.x},${link.from.y} C${link.from.x + 80},${link.from.y} ${link.to.x - 80},${link.to.y} ${link.to.x},${link.to.y}"
      marker-end="url(#hvacRelationArrow)">
      <title>${escapeHTML(title)}</title>
    </path>`;
}

function renderRelationNode(node) {
  const visual = relationNodeVisual(node);
  return renderGraphNode({
    key: node.key,
    x: node.x,
    y: node.y,
    width: node.width,
    height: node.height,
    label: node.label,
    meta: node.meta,
    iconKind: visual.iconKind,
    shortLabel: visual.shortLabel,
    className: `${node.kind} ${graphSelectionClass(node.key, node.relatedKeys || [])}`,
  });
}

function renderHVACRelationGraphDetail(relations) {
  const selected = selectedRelationGraphItem(relations);
  if (!selected) {
    const zoneCount = relations.length;
    const terminalCount = new Set(relations.flatMap((relation) => (relation.terminalUnits || []).map(componentGraphKey))).size;
    const plantCount = new Set(relations.flatMap((relation) => relation.plantLoopNames || [])).size;
    return `
      <section class="hvac-graph-detail">
        <div class="hvac-section-head">
          <h3>${t("hvac.relationDetail")}</h3>
          <span>${t("hvac.relationHint")}</span>
        </div>
        <div class="hvac-detail-grid">
          <div><span>Zones</span><strong>${escapeHTML(zoneCount)}</strong></div>
          <div><span>${t("hvac.terminals")}</span><strong>${escapeHTML(terminalCount)}</strong></div>
          <div><span>Plant loops</span><strong>${escapeHTML(plantCount)}</strong></div>
          <div><span>${t("common.chains")}</span><strong>${escapeHTML(relations.reduce((sum, relation) => sum + Math.max(1, (relation.serviceChains || []).length), 0))}</strong></div>
        </div>
      </section>`;
  }
  return renderSelectedHVACDetail(selected);
}

function renderHVACZoneRelation(relation) {
  const deliveryText = [...(relation.terminalUnits || []), ...(relation.zoneEquipment || []).filter((component) => !isAirTerminalObjectType(component.objectType))]
    .map(componentDisplayName)
    .filter(Boolean)
    .join(", ") || "N/A";
  const ruleEdges = ruleEdgesForRelation(relation);
  const plantText = [
    ...(relation.plantLoopNames || []),
    ...(relation.condenserLoopNames || []).map((name) => `Condenser: ${name}`),
  ].join(", ") || "N/A";
  const relationKey = relationSelectionKey(relation);
  const rowClass = graphSelectionClass(relationKey, relationGraphKeys(relation));
  return `
    <div class="hvac-relation-table-row ${rowClass}" role="row" tabindex="0" data-hvac-graph-key="${escapeHTML(relationKey)}">
      <span>
        ${renderRelationSubjectLink(relation)}
        <strong>${escapeHTML(relationDisplayName(relation))}</strong>
      </span>
      <span>${escapeHTML(deliveryText)}</span>
      <span>${(relation.airLoopNames || []).map(escapeHTML).join(", ") || "N/A"}</span>
      <span>${escapeHTML(plantText)}</span>
      <span>${escapeHTML(ruleEdgeSummary(ruleEdges) || relationIssueSummary(relation))}</span>
    </div>`;
}

function relationIssueSummary(relation = {}) {
  if (!(relation.terminalUnits || []).length && !(relation.zoneEquipment || []).length) {
    return t("hvac.noTerminal", {}, "No terminal");
  }
  if (!(relation.airLoopNames || []).length && !(relation.plantLoopNames || []).length) {
    return "No loop relation";
  }
  return "N/A";
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
        : state.activeHVACView === "relation"
          ? selectedRelationGraphItem(hvac.zoneRelations || [])
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
    ${
      selected.relations
        ? `<div class="hvac-tag-list">${selected.relations.map((relation) => `<span>${escapeHTML(relationDisplayName(relation))}</span>`).join("") || `<span>N/A</span>`}</div>`
        : ""
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
  for (const relation of selected.relations || []) {
    for (const loopName of relation.airLoopNames || []) {
      add(loopName, "AirLoopHVAC", normalizeGraphName(loopName) === normalizeGraphName(currentLoopName), componentGraphKey(component));
    }
    for (const loopName of relation.plantLoopNames || []) {
      add(loopName, "PlantLoop", normalizeGraphName(loopName) === normalizeGraphName(currentLoopName), componentGraphKey(component));
    }
    for (const loopName of relation.condenserLoopNames || []) {
      add(loopName, "CondenserLoop", normalizeGraphName(loopName) === normalizeGraphName(currentLoopName), componentGraphKey(component));
    }
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

function zoneRelationMatchesQuery(relation, query) {
  if (!query) {
    return true;
  }
  return [
    relation.zoneName,
    ...ruleEdgesForRelation(relation).flatMap(ruleEdgeSearchFields),
    ...(relation.airLoopNames || []),
    ...(relation.airLoopRelations || []).flatMap((item) => [item.loopName]),
    ...(relation.plantLoopNames || []),
    ...(relation.plantLoopRelations || []).flatMap((item) => [item.loopName]),
    ...(relation.condenserLoopNames || []),
    ...(relation.terminalUnits || []).flatMap((item) => componentSearchFields(item)),
    ...(relation.zoneEquipment || []).flatMap((item) => componentSearchFields(item)),
  ]
    .join(" ")
    .toLowerCase()
    .includes(query);
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

function selectedRelationGraphItem(relations) {
  const key = state.activeHVACGraphKey;
  if (!key) {
    return null;
  }
  if (key.startsWith("relation-link:")) {
    return { kind: "link", key, relations: relationsForGraphKey(relations, key) };
  }
  if (key.startsWith("zone:")) {
    const zoneName = key.slice(5);
    return { kind: "zone", zoneName, relation: relations.find((relation) => relation.zoneName === zoneName) };
  }
  if (key.startsWith("space:")) {
    const spaceName = key.slice(6);
    const relation = relations.find((item) => item.spaceName === spaceName);
    return { kind: "zone", zoneName: relationDisplayName(relation), relation };
  }
  if (key.startsWith("plant:")) {
    const loopName = key.slice(6);
    return { kind: "plant", loopName, relations: relations.filter((relation) => (relation.plantLoopNames || []).includes(loopName)) };
  }
  if (key.startsWith("source:")) {
    const matching = relations.filter((relation) =>
      uniqueRelationComponents(relation.plantEquipment || []).some((component) => relationComponentKey(component, "source") === key),
    );
    const component = matching.flatMap((relation) => relation.plantEquipment || []).find((item) => relationComponentKey(item, "source") === key);
    return { kind: "component", component, label: component?.objectName || key.slice(7), relations: matching };
  }
  if (key.startsWith("air:")) {
    const loopName = key.slice(4);
    return { kind: "air", loopName, relations: relations.filter((relation) => (relation.airLoopNames || []).includes(loopName)) };
  }
  if (key.startsWith("terminal:")) {
    const name = key.slice(9);
    const matching = relations.filter((relation) =>
      (relation.terminalUnits || []).some((component) => relationComponentKey(component) === key),
    );
    const component = matching.flatMap((relation) => relation.terminalUnits || []).find((item) => relationComponentKey(item) === key);
    return { kind: "component", component, label: name, relations: matching };
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
        ${
          (selected.relations || []).length
            ? `<div class="hvac-detail-list">
                ${(selected.relations || [])
                  .map(
                    (relation) =>
                      `<div><strong>${renderRelationSubjectLink(relation)} ${escapeHTML(relationDisplayName(relation))}</strong><span>${escapeHTML(
                        [...new Set([...(relation.plantLoopNames || []), ...(relation.airLoopNames || [])])].join(" -> ") || t("hvac.serviceRelation"),
                      )}</span></div>`,
                  )
                  .join("")}
              </div>`
            : ""
        }
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
    const relation = selected.relation;
    const loopNames = relation?.airLoopNames || (selected.loop?.name ? [selected.loop.name] : []);
    const ruleEdges = ruleEdgesForRelation(relation);
    return `
      <section class="hvac-graph-detail">
        <div class="hvac-section-head">
          <h3>${escapeHTML(selected.zoneName)}</h3>
          <span>Zone</span>
        </div>
        <div class="hvac-detail-grid">
          <div><span>${t("common.object")}</span><strong>${renderRelationSubjectLink(relation) || "N/A"}</strong></div>
          <div><span>Air loops</span><strong>${escapeHTML(loopNames.join(", ") || "N/A")}</strong></div>
          <div><span>Plant loops</span><strong>${escapeHTML((relation?.plantLoopNames || []).join(", ") || "N/A")}</strong></div>
          <div><span>Condenser loops</span><strong>${escapeHTML((relation?.condenserLoopNames || []).join(", ") || "N/A")}</strong></div>
          <div><span>${t("hvac.terminals")}</span><strong>${escapeHTML((relation?.terminalUnits || []).map((item) => item.objectName || item.objectType).join(", ") || "N/A")}</strong></div>
          <div><span>${t("common.equipment")}</span><strong>${escapeHTML((relation?.zoneEquipment || []).map((item) => item.objectName || item.objectType).join(", ") || "N/A")}</strong></div>
        </div>
        ${renderHVACTraceDrawer(ruleEdges.map((edge) => [edge.ruleId, edge.sourceObjectName].filter(Boolean).join(" / ")))}
      </section>`;
  }
  if (selected.kind === "plant" || selected.kind === "air") {
    return `
      <section class="hvac-graph-detail">
        <div class="hvac-section-head">
          <h3>${escapeHTML(selected.loopName)}</h3>
          <span>${selected.kind === "plant" ? "PlantLoop" : "AirLoopHVAC"}</span>
        </div>
        <div class="hvac-detail-list">
          ${(selected.relations || []).map((relation) => `<div><strong>${renderRelationSubjectLink(relation)} ${escapeHTML(relationDisplayName(relation))}</strong><span>${escapeHTML((relation.terminalUnits || []).map((item) => item.objectName || item.objectType).join(", ") || t("hvac.noTerminal"))}</span></div>`).join("") || `<div class="empty">${t("profile.noMatchingZones")}</div>`}
        </div>
      </section>`;
  }
  if (selected.kind === "link") {
    return `
      <section class="hvac-graph-detail">
        <div class="hvac-section-head">
          <h3>${t("common.connection")}</h3>
          <span>${escapeHTML(t("hvac.relatedZoneCount", { count: (selected.relations || []).length }))}</span>
        </div>
        <div class="hvac-detail-list">
          ${(selected.relations || []).map((relation) => `<div><strong>${renderRelationSubjectLink(relation)} ${escapeHTML(relationDisplayName(relation))}</strong><span>${escapeHTML([...new Set([...(relation.plantLoopNames || []), ...(relation.airLoopNames || [])])].join(" -> ") || t("hvac.serviceRelation"))}</span></div>`).join("")}
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

function relationNodeVisual(node = {}) {
  if (node.component) {
    return componentVisual(node.component);
  }
  switch (node.kind) {
    case "plant":
      return { iconKind: "plant", shortLabel: "Plant" };
    case "air":
      return { iconKind: "air", shortLabel: "Air Loop" };
    case "terminal":
      return { iconKind: "terminal", shortLabel: "Terminal" };
    case "zone":
      return { iconKind: "zone", shortLabel: "Zone" };
    default:
      return { iconKind: "component", shortLabel: truncateText(node.label || "Item", 10) };
  }
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

function uniqueRelationComponents(components) {
  const byKey = new Map();
  for (const component of components) {
    const key = relationComponentKey(component);
    if (!byKey.has(key)) {
      byKey.set(key, component);
    }
  }
  return [...byKey.values()];
}

function relationGraphKeys(relation) {
  const terminalKeys = uniqueRelationComponents(relation.terminalUnits || []).map((component) => relationComponentKey(component, "terminal"));
  const subjectKey = relationSelectionKey(relation);
  const sourceKeys = uniqueRelationComponents(relation.plantEquipment || []).map((component) => relationComponentKey(component, "source"));
  return [
    subjectKey,
    ...(relation.plantLoopNames || []).map((name) => `plant:${name}`),
    ...sourceKeys,
    ...(relation.airLoopNames || []).map((name) => `air:${name}`),
    ...terminalKeys,
  ].filter(Boolean);
}

function buildRelationGraph(relations) {
  const columns = [
    { id: "plant", label: t("hvac.sourcePlant"), x: 120 },
    { id: "air", label: t("hvac.legendAir"), x: 360 },
    { id: "terminal", label: t("hvac.terminalEquipment"), x: 625 },
    { id: "zone", label: "Zone", x: 900 },
  ];
  const nodesByKey = new Map();
  const linksByKey = new Map();
  for (const relation of relations) {
    const subjectKey = relationSelectionKey(relation);
    const relationRuleEdges = ruleEdgesForRelation(relation);
    const zoneNode = ensureRelationNode(nodesByKey, {
      key: subjectKey,
      kind: "zone",
      column: "zone",
      label: relationDisplayName(relation),
      meta: relation.relationScope === "space" ? "SpaceHVAC" : "Zone",
      objectIndex: relation.spaceName ? relation.spaceObjectIndex : relation.zoneObjectIndex,
    });
    const terminalComponents = uniqueRelationComponents(relation.terminalUnits || []);
    const sourceComponents = uniqueRelationComponents(relation.plantEquipment || []);
    for (const chain of relation.serviceChains || []) {
      const terminalComponent = findRelationComponentByLabel(terminalComponents, chain.terminalName);
      const sourceComponent = findRelationComponentByLabel(sourceComponents, chain.sourceComponent || chain.component);
      const terminalNode =
        terminalComponent || chain.terminalName
          ? ensureRelationNode(nodesByKey, {
              key: terminalComponent ? relationComponentKey(terminalComponent, "terminal") : `terminal:${subjectKey}:${chain.terminalName}`,
              kind: "terminal",
              column: "terminal",
              label: terminalComponent?.objectName || chain.terminalName || t("hvac.delivery", {}, "Delivery"),
              meta: terminalComponent
                ? terminalComponent.displayLabel || terminalComponent.familyLabel || terminalComponent.family || terminalComponent.objectType || "Equipment"
                : "Service path",
              component: terminalComponent,
            })
          : null;
      const airNode = chain.airLoopName
        ? ensureRelationNode(nodesByKey, {
            key: `air:${chain.airLoopName}`,
            kind: "air",
            column: "air",
            label: chain.airLoopName,
            meta: "AirLoopHVAC",
          })
        : null;
      const plantNode =
        sourceComponent || chain.sourceComponent || chain.plantLoop
          ? ensureRelationNode(nodesByKey, {
              key: sourceComponent ? relationComponentKey(sourceComponent, "source") : `plant:${chain.sourceComponent || chain.plantLoop}`,
              kind: sourceComponent || chain.sourceComponent ? "source" : "plant",
              column: "plant",
              label: sourceComponent?.objectName || chain.sourceComponent || chain.plantLoop,
              meta: sourceComponent
                ? [sourceComponent.displayLabel || sourceComponent.familyLabel || sourceComponent.family, sourceComponent.loopName].filter(Boolean).join(" / ") || chain.plantLoop || "PlantLoop"
                : chain.plantLoop || "Service path",
              component: sourceComponent,
            })
          : null;
      if (airNode && terminalNode) {
        addRelationLink(linksByKey, airNode, terminalNode, "air-terminal", relation);
      }
      if (terminalNode) {
        addRelationLink(linksByKey, terminalNode, zoneNode, "terminal-zone", relation);
      }
      if (plantNode && airNode) {
        addRelationLink(linksByKey, plantNode, airNode, "plant-air", relation);
      } else if (plantNode && terminalNode) {
        addRelationLink(linksByKey, plantNode, terminalNode, "plant-terminal", relation);
      } else if (plantNode) {
        addRelationLink(linksByKey, plantNode, zoneNode, "source-zone", relation);
      }
    }
  }
  const nodes = [...nodesByKey.values()];
  const byColumn = new Map(columns.map((column) => [column.id, []]));
  for (const node of nodes) {
    byColumn.get(node.column)?.push(node);
  }
  for (const column of columns) {
    const columnNodes = byColumn.get(column.id) || [];
    columnNodes.sort((a, b) => a.label.localeCompare(b.label));
    const height = Math.max(430, 100 + columnNodes.length * 74);
    columnNodes.forEach((node, index) => {
      node.x = column.x;
      node.y = 86 + index * 74 + Math.max(0, (height - (100 + columnNodes.length * 74)) / 2);
      node.width = column.id === "terminal" ? 166 : 148;
      node.height = 56;
    });
  }
  for (const node of nodes) {
    node.relatedKeys = [...new Set((node.relatedKeys || []).filter((key) => key && key !== node.key))];
  }
  const height = Math.max(430, ...columns.map((column) => 130 + (byColumn.get(column.id)?.length || 0) * 74));
  const links = [...linksByKey.values()].map((link) => ({
    ...link,
    from: link.from,
    to: link.to,
  }));
  return { width: 1020, height, columns, nodes, links };
}

function ensureRelationNode(nodesByKey, node) {
  if (!nodesByKey.has(node.key)) {
    nodesByKey.set(node.key, { ...node, relatedKeys: [] });
  }
  return nodesByKey.get(node.key);
}

function findRelationComponentByLabel(components = [], label = "") {
  const normalized = normalizeGraphName(label);
  if (!normalized) {
    return null;
  }
  return components.find((component) => normalizeGraphName(componentDisplayName(component)) === normalized || normalizeGraphName(component.objectName) === normalized) || null;
}

function addRelationLink(linksByKey, from, to, kind, relation) {
  const key = `relation-link:${from.key}->${to.key}`;
  if (!linksByKey.has(key)) {
    linksByKey.set(key, { key, from, to, kind, relations: [] });
  }
  linksByKey.get(key).relations.push(relation);
  const relationKeys = relationGraphKeys(relation);
  from.relatedKeys.push(to.key, key, ...relationKeys);
  to.relatedKeys.push(from.key, key, ...relationKeys);
}

function relationComponentKey(component, prefix = "terminal") {
  if (Number.isFinite(Number(component?.objectIndex)) && Number(component.objectIndex) >= 0) {
    return `${prefix}:${component.objectIndex}`;
  }
  return `${prefix}:${component?.objectType || ""}:${component?.objectName || ""}`;
}

function selectionRelatedToLink(link) {
  const selectedKey = state.activeHVACGraphKey;
  return (
    selectedKey &&
    (selectedKey === link.from.key ||
      selectedKey === link.to.key ||
      link.from.relatedKeys.includes(selectedKey) ||
      link.to.relatedKeys.includes(selectedKey) ||
      (link.relations || []).some((relation) => relationGraphKeys(relation).includes(selectedKey)))
  );
}

function relationsForGraphKey(relations, key) {
  if (!key.startsWith("relation-link:")) {
    return [];
  }
  const raw = key.slice("relation-link:".length);
  const [fromKey, toKey] = raw.split("->");
  return relations.filter((relation) => {
    const all = relationGraphKeys(relation);
    return all.includes(fromKey) && all.includes(toKey);
  });
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
