import { elements, escapeHTML, state } from "./state.js";
import { renderInputViews } from "./input-views.js";

export function renderReport() {
  const report = state.report;
  if (!report) {
    renderEmpty();
    return;
  }

  elements.objectCount.textContent = report.objectCount ?? 0;
  elements.typeCount.textContent = report.typeCounts?.length ?? 0;
  elements.scheduleCount.textContent = report.schedules?.length ?? 0;
  elements.unusedCount.textContent = report.unusedObjects?.length ?? 0;

  renderTypeList(report.typeCounts || []);
  renderZoneDetails(report.zones || []);
  renderZoneViz(report.zones || []);
  renderScheduleList(report.schedules || []);
  renderUnusedList(report.unusedObjects || []);
  renderSystemViz(report.hvacConnections || []);
  renderConnectionList(report.hvacConnections || []);
  renderInputViews();
}

export function renderEmpty() {
  elements.objectCount.textContent = "0";
  elements.typeCount.textContent = "0";
  elements.scheduleCount.textContent = "0";
  elements.unusedCount.textContent = "0";
  elements.typeList.innerHTML = `<div class="empty">No analysis yet</div>`;
  elements.zoneDetails.innerHTML = `<div class="empty">No zone details yet</div>`;
  elements.scheduleList.innerHTML = `<div class="empty">No schedules yet</div>`;
  elements.unusedList.innerHTML = `<div class="empty">No unused objects yet</div>`;
  elements.connectionList.innerHTML = `<div class="empty">No connections yet</div>`;
  elements.zoneViz.innerHTML = "";
  elements.systemViz.innerHTML = "";
  elements.textObjectView.innerHTML = `<div class="empty">No formatted input yet</div>`;
  elements.jsonStructuredView.innerHTML = `<div class="empty">No structured input yet</div>`;
  elements.jsonTextInput.value = "";
  elements.fieldTable.innerHTML = `<div class="empty">No field table yet</div>`;
  elements.fieldStats.textContent = "0 tables";
}

export function renderTypeList(typeCounts) {
  elements.typeList.innerHTML = typeCounts.length
    ? typeCounts
        .map(
          (item) => `
            <div class="list-row navigable-row" data-jump-object-type="${escapeHTML(item.type)}" role="button" tabindex="0">
              <span class="row-main" title="${escapeHTML(item.type)}">${escapeHTML(item.type)}</span>
              <span class="badge">${escapeHTML(item.count)}</span>
            </div>`,
        )
        .join("")
    : `<div class="empty">No object types</div>`;
}

export function renderScheduleList(schedules) {
  elements.scheduleList.innerHTML = schedules.length
    ? schedules
        .map(
          (schedule) => `
            <div class="list-row navigable-row" data-jump-object-index="${escapeHTML(schedule.index)}" data-jump-object-type="${escapeHTML(schedule.type)}" role="button" tabindex="0">
              <span>
                <span class="row-main" title="${escapeHTML(schedule.name)}">${escapeHTML(schedule.name)}</span>
                <span class="row-sub">${escapeHTML(schedule.type)}</span>
              </span>
              <span class="badge">#${escapeHTML(schedule.index)}</span>
            </div>`,
        )
        .join("")
    : `<div class="empty">No schedules</div>`;
}

export function renderUnusedList(unusedObjects) {
  elements.unusedList.innerHTML = unusedObjects.length
    ? unusedObjects
        .map(
          (object) => `
            <div class="list-row navigable-row" data-jump-object-index="${escapeHTML(object.index)}" data-jump-object-type="${escapeHTML(object.type)}" role="button" tabindex="0">
              <span>
                <span class="row-main" title="${escapeHTML(object.name)}">${escapeHTML(object.name)}</span>
                <span class="row-sub">${escapeHTML(object.type)}</span>
              </span>
              <span class="badge">#${escapeHTML(object.index)}</span>
            </div>`,
        )
        .join("")
    : `<div class="empty">No unused named objects</div>`;
}

export function renderConnectionList(connections) {
  elements.connectionList.innerHTML = connections.length
    ? connections
        .map(
          (connection) => `
            <div class="list-row navigable-row" data-jump-object-index="${escapeHTML(connection.objectIndex)}" data-jump-object-type="${escapeHTML(connection.objectType)}" role="button" tabindex="0">
              <span>
                <span class="row-main">${escapeHTML(connection.fromNode)} -> ${escapeHTML(connection.toNode)}</span>
                <span class="row-sub">${escapeHTML(connection.objectType)} ${escapeHTML(connection.objectName || "")}</span>
              </span>
              <span class="badge">#${escapeHTML(connection.objectIndex)}</span>
            </div>`,
        )
        .join("")
    : `<div class="empty">No node-to-node connections</div>`;
}

export function renderZoneViz(zones) {
  const svg = elements.zoneViz;
  const width = 560;
  const height = 260;
  svg.setAttribute("viewBox", `0 0 ${width} ${height}`);

  if (!zones.length) {
    svg.innerHTML = `<text x="24" y="48" fill="#60707c" font-size="14">No zones</text>`;
    return;
  }

  const columns = Math.min(3, zones.length);
  const cellWidth = (width - 48) / columns;
  const cellHeight = 78;
  const content = zones
    .map((zone, index) => {
      const col = index % columns;
      const row = Math.floor(index / columns);
      const x = 24 + col * cellWidth;
      const y = 28 + row * (cellHeight + 18);
      const surfaceText = `${zone.surfaceCount || 0} surfaces`;
      const selected = state.selectedZoneName === zone.name;
      return `
        <g class="zone-card ${selected ? "selected" : ""}" data-zone-name="${escapeHTML(zone.name)}"
          data-jump-object-index="${escapeHTML(zone.index)}" data-jump-object-type="Zone" role="button" tabindex="0">
          <rect x="${x}" y="${y}" width="${cellWidth - 14}" height="${cellHeight}" rx="6" fill="${selected ? "#dff1f2" : "#e9f5f6"}" stroke="${selected ? "#005d66" : "#007c89"}" stroke-width="${selected ? "2" : "1"}" />
          <text x="${x + 12}" y="${y + 30}" fill="#18222b" font-size="14" font-weight="700">${escapeHTML(zone.name)}</text>
          <text x="${x + 12}" y="${y + 54}" fill="#60707c" font-size="12">${escapeHTML(surfaceText)}</text>
        </g>`;
    })
    .join("");
  svg.innerHTML = content;
}

export function renderZoneDetails(zones) {
  if (!zones.length) {
    state.selectedZoneName = "";
    elements.zoneDetails.innerHTML = `<div class="empty">No zones</div>`;
    return;
  }

  let zone = zones.find((item) => item.name === state.selectedZoneName);
  if (!zone) {
    zone = zones[0];
    state.selectedZoneName = zone.name;
  }

  const activeTab = state.selectedZoneTab === "related" ? "related" : "surfaces";
  const surfaces = zone.surfaces || [];
  const relatedObjects = zone.relatedObjects || [];
  const items = activeTab === "surfaces" ? surfaces : relatedObjects;
  const emptyLabel = activeTab === "surfaces" ? "No surfaces linked to this zone" : "No related objects linked to this zone";

  elements.zoneDetails.innerHTML = `
    <div class="zone-detail-head">
      <button class="zone-title navigable-row" data-jump-object-index="${escapeHTML(zone.index)}" data-jump-object-type="Zone" type="button">
        <span>${escapeHTML(zone.name)}</span>
        <span class="badge">#${escapeHTML(zone.index)}</span>
      </button>
      <div class="zone-detail-tabs" role="tablist" aria-label="Zone details">
        <button class="zone-detail-tab ${activeTab === "surfaces" ? "active" : ""}" data-zone-tab="surfaces" type="button">Surfaces</button>
        <button class="zone-detail-tab ${activeTab === "related" ? "active" : ""}" data-zone-tab="related" type="button">Related</button>
      </div>
    </div>
    <div class="zone-detail-list">
      ${
        items.length
          ? items
              .map(
                (item) => `
                  <div class="list-row navigable-row" data-jump-object-index="${escapeHTML(item.index)}" data-jump-object-type="${escapeHTML(item.type)}" role="button" tabindex="0">
                    <span>
                      <span class="row-main" title="${escapeHTML(item.name || item.type)}">${escapeHTML(item.name || item.type)}</span>
                      <span class="row-sub">${escapeHTML(item.type)}${item.role ? ` - ${escapeHTML(item.role)}` : ""}</span>
                    </span>
                    <span class="badge">#${escapeHTML(item.index)}</span>
                  </div>`,
              )
              .join("")
          : `<div class="empty">${emptyLabel}</div>`
      }
    </div>
  `;
}

export function renderSystemViz(connections) {
  const svg = elements.systemViz;
  const width = 800;
  const height = 260;
  svg.setAttribute("viewBox", `0 0 ${width} ${height}`);

  if (!connections.length) {
    svg.innerHTML = `<text x="24" y="48" fill="#60707c" font-size="14">No HVAC connections</text>`;
    return;
  }

  const nodes = [...new Set(connections.flatMap((item) => [item.fromNode, item.toNode]))].slice(0, 9);
  const spacing = (width - 100) / Math.max(nodes.length - 1, 1);
  const y = 112;
  const nodeX = new Map(nodes.map((node, index) => [node, 50 + index * spacing]));

  const paths = connections
    .filter((connection) => nodeX.has(connection.fromNode) && nodeX.has(connection.toNode))
    .map((connection) => {
      const x1 = nodeX.get(connection.fromNode);
      const x2 = nodeX.get(connection.toNode);
      const mid = (x1 + x2) / 2;
      return `
        <path class="system-link" data-jump-object-index="${escapeHTML(connection.objectIndex)}" data-jump-object-type="${escapeHTML(connection.objectType)}"
          d="M ${x1} ${y} C ${mid} ${y - 52}, ${mid} ${y - 52}, ${x2} ${y}"
          fill="none" stroke="#a85f00" stroke-width="2" marker-end="url(#arrow)">
          <title>${escapeHTML(connection.objectType)} ${escapeHTML(connection.objectName || "")}</title>
        </path>`;
    })
    .join("");

  const nodeMarks = nodes
    .map((node) => {
      const x = nodeX.get(node);
      const label = node.length > 18 ? `${node.slice(0, 16)}...` : node;
      return `
        <g>
          <circle cx="${x}" cy="${y}" r="15" fill="#ffffff" stroke="#007c89" stroke-width="2" />
          <text x="${x}" y="${y + 36}" text-anchor="middle" fill="#18222b" font-size="12">${escapeHTML(label)}</text>
        </g>`;
    })
    .join("");

  svg.innerHTML = `
    <defs>
      <marker id="arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto">
        <path d="M 0 0 L 8 4 L 0 8 z" fill="#a85f00"></path>
      </marker>
    </defs>
    ${paths}
    ${nodeMarks}
  `;
}
