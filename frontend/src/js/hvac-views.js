import { backend, elements, escapeHTML, state } from "./state.js";
import { t } from "./i18n.js";

export function initializeHVACControls() {
  elements.hvacFilter?.addEventListener("input", () => renderHVAC());
  elements.hvacSummary?.addEventListener("click", handleHVACNavigationClick);
  elements.hvacSummary?.addEventListener("toggle", handleHVACNavigationToggle, true);
  document.addEventListener("click", handleHVACOutsideClick);
  elements.hvacGraph?.addEventListener("click", (event) => {
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
      state.activeHVACGraphKey = graphTarget.dataset.hvacGraphKey || "";
      state.activeHVACNodeName = state.activeHVACGraphKey.startsWith("node:") ? state.activeHVACGraphKey.slice(5) : "";
      renderHVAC();
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

export function renderHVAC(hvac = state.report?.hvac) {
  if (!elements.hvacStats) {
    return;
  }
  if (!hvac) {
    renderEmptyHVAC();
    return;
  }

  const loops = hvac.loops || [];
  if (!state.activeHVACLoopId || !loops.some((loop) => loop.id === state.activeHVACLoopId)) {
    state.activeHVACLoopId = loops[0]?.id || "";
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
    renderHVACRelations(hvac, query);
  } else if (state.activeHVACView === "diagnostics") {
    renderHVACDiagnostics(hvac, query);
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
  elements.hvacSummary.innerHTML = `
    <div class="hvac-navigator">
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
      ${renderHVACRelationPicker(hvac.zoneRelations || [], state.activeHVACView === "relation")}
      ${renderHVACDiagnosticsCard(hvac.warningCount || 0, state.activeHVACView === "diagnostics")}
    </div>`;
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
  const selectedZone = state.activeHVACGraphKey?.startsWith("zone:") ? state.activeHVACGraphKey.slice(5) : "";
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
        <button class="hvac-nav-choice ${active && !selectedZone ? "active" : ""}" type="button" data-hvac-open-view="relation">
          <span>${escapeHTML(t("hvac.allZoneRelations"))}</span>
          <small>${escapeHTML(t("hvac.serviceRelation"))}</small>
        </button>
        ${
          relations.length
            ? relations.map((relation) => renderHVACRelationChoice(relation, selectedZone)).join("")
            : `<div class="empty compact">${escapeHTML(t("hvac.noZoneRelations"))}</div>`
        }
      </div>
    </details>`;
}

function renderHVACRelationChoice(relation, selectedZone) {
  const selected = state.activeHVACView === "relation" && selectedZone === relation.zoneName;
  const meta = [...new Set([...(relation.airLoopNames || []), ...(relation.plantLoopNames || [])])].join(", ") || t("hvac.noTerminal");
  return `
    <button class="hvac-nav-choice ${selected ? "active" : ""}" type="button" data-hvac-relation-zone="${escapeHTML(relation.zoneName || "")}">
      <span>${escapeHTML(relation.zoneName || t("common.blank"))}</span>
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
    renderHVAC();
    return;
  }
  const relationButton = event.target.closest("[data-hvac-relation-zone]");
  if (relationButton) {
    state.activeHVACView = "relation";
    state.activeHVACGraphKey = relationButton.dataset.hvacRelationZone ? `zone:${relationButton.dataset.hvacRelationZone}` : "";
    state.activeHVACNodeName = "";
    renderHVAC();
    return;
  }
  const viewButton = event.target.closest("[data-hvac-open-view]");
  if (viewButton) {
    state.activeHVACView = viewButton.dataset.hvacOpenView || "loop";
    state.activeHVACGraphKey = "";
    state.activeHVACNodeName = "";
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
  renderHVAC();
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
      <div>
        <h3>${escapeHTML(loop.name || loop.type)}</h3>
        <span>${escapeHTML(loop.type)} ${renderObjectLink(loop.objectIndex, loop.type)}</span>
      </div>
      <div class="hvac-loop-meta">
        <span>${escapeHTML(t("count.zones", { count: (loop.relatedZones || []).length }))}</span>
        <span>${escapeHTML(t("hvac.crossLoopLinks", { count: (loop.relatedLoops || []).length }))}</span>
      </div>
    </div>
    ${renderHVACLoopDiagram(loop)}
    ${renderHVACLoopGraphDetail(loop)}
    ${renderCrossLoopRelations(loop)}`;
}

function renderHVACLoopDiagram(loop) {
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
    <div class="hvac-graphic-shell">
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
    label: component.objectName || component.objectType || "Component",
    meta: component.objectType || "Component",
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
  return `
    <div class="hvac-component${existsClass}">
      <div class="hvac-component-main">
        <strong>${escapeHTML(component.objectName || component.objectType || t("common.component"))}</strong>
        <span>${escapeHTML(component.objectType || t("hvac.unknownType"))} ${renderObjectLink(component.objectIndex, component.objectType)}</span>
      </div>
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
      ${(component.relatedLoopNames || []).length ? renderComponentCrossLoopButtons(component) : ""}
      ${renderHVACEditableFields(component.editableFields)}
    </div>`;
}

function renderComponentCrossLoopButtons(component) {
  const graphKey = componentGraphKey(component);
  return `
    <div class="hvac-cross-loop-buttons">
      <small>${t("hvac.crossLoop")}</small>
      ${(component.relatedLoopNames || [])
        .map(
          (loopName) => `
            <button type="button" data-hvac-jump-loop-name="${escapeHTML(loopName)}" data-hvac-jump-graph-key="${escapeHTML(graphKey)}" title="${escapeHTML(loopName)}">
              ${escapeHTML(loopName)}
            </button>`,
        )
        .join("")}
    </div>`;
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

function renderHVACRelations(hvac, query) {
  const relations = (hvac.zoneRelations || []).filter((relation) => zoneRelationMatchesQuery(relation, query));
  elements.hvacGraph.innerHTML = relations.length
    ? `
      ${renderHVACRelationGraph(relations)}
      ${renderHVACRelationGraphDetail(relations)}`
    : `<div class="empty">${t("hvac.noMatchingRelations")}</div>`;
}

function renderHVACRelationGraph(relations) {
  const graph = buildRelationGraph(relations);
  const width = graph.width;
  const height = graph.height;
  return `
    <div class="hvac-graphic-shell hvac-relation-shell">
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
  return `
    <div class="hvac-relation-table-row" role="row">
      <span>
        ${renderObjectLink(relation.zoneObjectIndex, "Zone")}
        <strong>${escapeHTML(relation.zoneName)}</strong>
      </span>
      <span>${(relation.terminalUnits || []).map((item) => escapeHTML(item.objectName || item.objectType)).join(", ") || "N/A"}</span>
      <span>${(relation.airLoopNames || []).map(escapeHTML).join(", ") || "N/A"}</span>
      <span>${(relation.plantLoopNames || []).map(escapeHTML).join(", ") || "N/A"}</span>
      <span>${(relation.zoneEquipment || []).map((item) => escapeHTML(item.objectName || item.objectType)).join(", ") || "N/A"}</span>
    </div>`;
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
  return `
    <article class="hvac-warning ${escapeHTML(warning.severity || "warning")}">
      <div>
        <strong>${escapeHTML(warning.message || "")}</strong>
        <span>${escapeHTML([warning.code, warning.objectType, warning.objectName].filter(Boolean).join(" / "))}</span>
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
    const selected =
      state.activeHVACView === "relation"
        ? selectedRelationGraphItem(hvac.zoneRelations || [])
        : selectedLoop
          ? selectedLoopGraphItem(selectedLoop)
          : null;
    if (selected) {
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
  elements.hvacInspector.innerHTML = `
    <div class="hvac-inspector-title">
      <strong>${escapeHTML(title)}</strong>
      <span>${escapeHTML(selected.kind || t("common.selection"))}</span>
    </div>
    ${
      selected.component
        ? `
          <div class="hvac-inspector-kv"><span>${t("common.type")}</span><strong>${escapeHTML(selected.component.objectType || "N/A")}</strong></div>
          <div class="hvac-inspector-kv"><span>${t("common.inlet")}</span><strong>${escapeHTML(selected.component.inletNode || "N/A")}</strong></div>
          <div class="hvac-inspector-kv"><span>${t("common.outlet")}</span><strong>${escapeHTML(selected.component.outletNode || "N/A")}</strong></div>`
        : ""
    }
    ${
      selected.relations
        ? `<div class="hvac-tag-list">${selected.relations.map((relation) => `<span>${escapeHTML(relation.zoneName)}</span>`).join("") || `<span>N/A</span>`}</div>`
        : ""
    }`;
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
      component.objectType,
      component.objectName,
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
    ...(relation.airLoopNames || []),
    ...(relation.plantLoopNames || []),
    ...(relation.terminalUnits || []).flatMap((item) => [item.objectType, item.objectName]),
    ...(relation.zoneEquipment || []).flatMap((item) => [item.objectType, item.objectName]),
  ]
    .join(" ")
    .toLowerCase()
    .includes(query);
}

function warningMatchesQuery(warning, query) {
  if (!query) {
    return true;
  }
  return [warning.severity, warning.category, warning.code, warning.message, warning.objectType, warning.objectName, warning.field, warning.value]
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
    ...(component?.relatedLoopNames || []).map((name) => `loop-name:${name}`),
  ].filter(Boolean);
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
          return { kind: "component", component, branch, side };
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
      [...(relation.terminalUnits || []), ...(relation.zoneEquipment || [])].some((component) => relationComponentKey(component) === key),
    );
    const component = matching.flatMap((relation) => [...(relation.terminalUnits || []), ...(relation.zoneEquipment || [])]).find((item) => relationComponentKey(item) === key);
    return { kind: "component", component, label: name, relations: matching };
  }
  return null;
}

function renderSelectedHVACDetail(selected) {
  if (selected.kind === "component") {
    const component = selected.component || {};
    return `
      <section class="hvac-graph-detail">
        <div class="hvac-section-head">
          <h3>${escapeHTML(component.objectName || selected.label || t("common.component"))}</h3>
          <span>${escapeHTML(component.objectType || t("common.component"))}</span>
        </div>
        <div class="hvac-detail-grid">
          <div><span>${t("common.object")}</span><strong>${renderObjectLink(component.objectIndex, component.objectType) || "N/A"}</strong></div>
          <div><span>${t("common.inlet")}</span><strong>${escapeHTML(component.inletNode || "N/A")}</strong></div>
          <div><span>${t("common.outlet")}</span><strong>${escapeHTML(component.outletNode || "N/A")}</strong></div>
          <div><span>${t("common.water")}</span><strong>${escapeHTML([component.waterInletNode, component.waterOutletNode].filter(Boolean).join(" -> ") || "N/A")}</strong></div>
          ${component.loopName ? `<div><span>${t("hvac.viewLoop")}</span><strong>${escapeHTML(component.loopName)}</strong></div>` : ""}
        </div>
        ${renderComponentCrossLoopMap(component)}
        ${
          (selected.relations || []).length
            ? `<div class="hvac-detail-list">
                ${(selected.relations || [])
                  .map(
                    (relation) =>
                      `<div><strong>${renderObjectLink(relation.zoneObjectIndex, "Zone")} ${escapeHTML(relation.zoneName)}</strong><span>${escapeHTML(
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
    return `
      <section class="hvac-graph-detail">
        <div class="hvac-section-head">
          <h3>${escapeHTML(selected.zoneName)}</h3>
          <span>Zone</span>
        </div>
        <div class="hvac-detail-grid">
          <div><span>${t("common.object")}</span><strong>${renderObjectLink(relation?.zoneObjectIndex, "Zone") || "N/A"}</strong></div>
          <div><span>Air loops</span><strong>${escapeHTML(loopNames.join(", ") || "N/A")}</strong></div>
          <div><span>Plant loops</span><strong>${escapeHTML((relation?.plantLoopNames || []).join(", ") || "N/A")}</strong></div>
          <div><span>${t("hvac.terminals")}</span><strong>${escapeHTML((relation?.terminalUnits || []).map((item) => item.objectName || item.objectType).join(", ") || "N/A")}</strong></div>
          <div><span>${t("common.equipment")}</span><strong>${escapeHTML((relation?.zoneEquipment || []).map((item) => item.objectName || item.objectType).join(", ") || "N/A")}</strong></div>
        </div>
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
          ${(selected.relations || []).map((relation) => `<div><strong>${renderObjectLink(relation.zoneObjectIndex, "Zone")} ${escapeHTML(relation.zoneName)}</strong><span>${escapeHTML((relation.terminalUnits || []).map((item) => item.objectName || item.objectType).join(", ") || t("hvac.noTerminal"))}</span></div>`).join("") || `<div class="empty">${t("profile.noMatchingZones")}</div>`}
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
          ${(selected.relations || []).map((relation) => `<div><strong>${renderObjectLink(relation.zoneObjectIndex, "Zone")} ${escapeHTML(relation.zoneName)}</strong><span>${escapeHTML([...new Set([...(relation.plantLoopNames || []), ...(relation.airLoopNames || [])])].join(" -> ") || t("hvac.serviceRelation"))}</span></div>`).join("")}
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

function renderComponentCrossLoopMap(component = {}) {
  const relatedLoopNames = [...new Set(component.relatedLoopNames || [])].filter(Boolean);
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

function componentVisual(component = {}) {
  const iconKind = hvacVisualKindForType(component.objectType || "");
  return {
    iconKind,
    shortLabel: hvacVisualLabel(iconKind, component.objectType || component.objectName || ""),
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
    return "terminal";
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
    case "chiller":
      return `
        <g class="hvac-loop-icon chiller" aria-hidden="true">
          <rect class="icon-case" x="${cx - 24}" y="${cy - 17}" width="48" height="34" rx="4"></rect>
          <rect class="icon-vent" x="${cx - 18}" y="${cy - 11}" width="36" height="7" rx="2"></rect>
          <path class="snow" d="M${cx},${cy - 1} V${cy + 12} M${cx - 7},${cy + 2} L${cx + 7},${cy + 9} M${cx + 7},${cy + 2} L${cx - 7},${cy + 9}"></path>
        </g>`;
    case "boiler":
      return `
        <g class="hvac-loop-icon boiler" aria-hidden="true">
          <rect class="icon-case" x="${cx - 22}" y="${cy - 17}" width="44" height="34" rx="4"></rect>
          <path class="flame" d="M${cx},${cy - 10} C${cx + 11},${cy - 1} ${cx + 7},${cy + 13} ${cx},${cy + 13} C${cx - 8},${cy + 13} ${cx - 11},${cy + 2} ${cx - 3},${cy - 4} C${cx - 1},${cy - 6} ${cx - 1},${cy - 8} ${cx},${cy - 10} Z"></path>
          <path class="flame-core" d="M${cx + 1},${cy - 2} C${cx + 5},${cy + 4} ${cx + 4},${cy + 10} ${cx},${cy + 10} C${cx - 4},${cy + 10} ${cx - 5},${cy + 4} ${cx + 1},${cy - 2} Z"></path>
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
  const terminalSource = (relation.terminalUnits || []).length ? relation.terminalUnits || [] : relation.zoneEquipment || [];
  const terminalKeys = uniqueRelationComponents(terminalSource).map((component) => relationComponentKey(component, "terminal"));
  if (!terminalKeys.length && relation.zoneName) {
    terminalKeys.push(`terminal:direct:${relation.zoneName}`);
  }
  const sourceKeys = uniqueRelationComponents(relation.plantEquipment || []).map((component) => relationComponentKey(component, "source"));
  return [
    relation.zoneName ? `zone:${relation.zoneName}` : "",
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
    const zoneNode = ensureRelationNode(nodesByKey, {
      key: `zone:${relation.zoneName}`,
      kind: "zone",
      column: "zone",
      label: relation.zoneName,
      meta: "Zone",
      objectIndex: relation.zoneObjectIndex,
    });
    const terminalSource = (relation.terminalUnits || []).length ? relation.terminalUnits || [] : relation.zoneEquipment || [];
    const terminalComponents = uniqueRelationComponents(terminalSource);
    const terminalNodes = terminalComponents.length
      ? terminalComponents.map((component) =>
          ensureRelationNode(nodesByKey, {
            key: relationComponentKey(component, "terminal"),
            kind: "terminal",
            column: "terminal",
            label: component.objectName || component.objectType || "Equipment",
            meta: component.objectType || "Equipment",
            component,
          }),
        )
      : [
          ensureRelationNode(nodesByKey, {
            key: `terminal:direct:${relation.zoneName}`,
            kind: "terminal",
            column: "terminal",
            label: t("hvac.directZoneEquipment"),
            meta: t("hvac.inferred"),
          }),
        ];
    const airNodes = (relation.airLoopNames || []).map((name) =>
      ensureRelationNode(nodesByKey, { key: `air:${name}`, kind: "air", column: "air", label: name, meta: "AirLoopHVAC" }),
    );
    const sourceComponents = uniqueRelationComponents(relation.plantEquipment || []);
    const plantNodes = sourceComponents.length
      ? sourceComponents.map((component) =>
          ensureRelationNode(nodesByKey, {
            key: relationComponentKey(component, "source"),
            kind: "source",
            column: "plant",
            label: component.objectName || component.objectType || "Source",
            meta: [component.objectType, component.loopName].filter(Boolean).join(" / ") || (relation.plantLoopNames || []).join(", ") || "PlantLoop",
            component,
          }),
        )
      : (relation.plantLoopNames || []).map((name) =>
          ensureRelationNode(nodesByKey, { key: `plant:${name}`, kind: "plant", column: "plant", label: name, meta: "PlantLoop" }),
        );
    for (const terminal of terminalNodes) {
      addRelationLink(linksByKey, terminal, zoneNode, "terminal-zone", relation);
    }
    for (const air of airNodes) {
      for (const terminal of terminalNodes) {
        addRelationLink(linksByKey, air, terminal, "air-terminal", relation);
      }
    }
    for (const plant of plantNodes) {
      if (airNodes.length) {
        for (const air of airNodes) {
          addRelationLink(linksByKey, plant, air, "plant-air", relation);
        }
      } else {
        for (const terminal of terminalNodes) {
          addRelationLink(linksByKey, plant, terminal, "plant-terminal", relation);
        }
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
