import { backend, elements, escapeHTML, setStatus, state } from "../state.js";
import { getCurrentAppSettings, saveAppSettings } from "../settings-client.js";
import { profileDimensionLabel as i18nProfileDimensionLabel, profileMetricLabel, t } from "../i18n.js";
import { configureResultPanelNavigationHooks } from "../panel-navigation-adapters.js";

let lastProfileView = null;
let profileNavigationCleanup = null;
let profileNavigationRevealTarget = null;
const PROFILE_MATRIX_RENDER_LIMIT = 500;

export function renderProfile(profile = state.report?.profile) {
  if (!elements.profileStats) {
    return;
  }
  if (!profile) {
    renderEmptyProfile();
    return;
  }

  state.profileSettings = mergeProfileSettings(profile.defaultSettings, state.profileSettings || getCurrentAppSettings().profile);
  state.profileGraphDeck = mergeProfileGraphDeck(profile, state.profileGraphDeck);
  state.activeProfileView = state.activeProfileView === "zone" ? "zone" : "profile";
  lastProfileView = cachedProfileView(profile, state.profileSettings);
  if (!state.activeProfileGroupId || !lastProfileView.groups.some((group) => group.id === state.activeProfileGroupId)) {
    state.activeProfileGroupId = lastProfileView.groups[0]?.id || "";
  }

  const query = profileQuery();
  const visibleGroups = lastProfileView.groups.filter(
    (group) => profileGroupMatchesQuery(group, query) || profileRevealMatchesGroup(group),
  );
  const visibleRows = lastProfileView.matrix.filter(
    (row) => profileMatrixRowMatchesQuery(row, query) || profileRevealMatchesRow(row),
  );
  if (query && !visibleGroups.some((group) => group.id === state.activeProfileGroupId)) {
    state.activeProfileGroupId = visibleGroups[0]?.id || "";
  }
  if (!state.activeProfileZoneName || !lastProfileView.matrix.some((row) => row.zoneName === state.activeProfileZoneName)) {
    state.activeProfileZoneName = visibleRows[0]?.zoneName || lastProfileView.matrix[0]?.zoneName || "";
  }
  if (state.activeProfileView === "zone" && query && !visibleRows.some((row) => row.zoneName === state.activeProfileZoneName)) {
    state.activeProfileZoneName = visibleRows[0]?.zoneName || "";
  }
  const selectedGroup = query
    ? visibleGroups.find((group) => group.id === state.activeProfileGroupId) || null
    : selectedProfileGroup();
  const selectedZone = state.activeProfileView === "zone" ? selectedProfileZoneRow() : null;
  const graphGroup = selectedZone ? groupForZoneName(selectedZone.zoneName) : selectedGroup;

  elements.profileStats.textContent = query
    ? t("count.profilesOf", { shown: visibleGroups.length, total: lastProfileView.groups.length, items: profile.itemCount || 0 })
    : t("count.profiles", { profiles: lastProfileView.groups.length, items: profile.itemCount || 0 });
  elements.profileApplyButton.disabled = !graphGroup;
  renderProfileSettings(profile);
  renderProfileOverview(visibleGroups, visibleRows);
  renderProfileGraph(graphGroup, profile, selectedZone);
  renderProfileMatrix(lastProfileView.matrix, query, profile);
  renderProfileDetail(graphGroup, profile, selectedZone);
  bindProfileControls(profile);
}

function renderEmptyProfile() {
  elements.profileStats.textContent = t("count.profiles", { profiles: 0, items: 0 });
  elements.profileSettings.innerHTML = "";
  elements.profileOverview.innerHTML = `<div class="empty">${t("profile.noAnalysis")}</div>`;
  elements.profileDetail.innerHTML = `<div class="empty">${t("profile.noProfile")}</div>`;
  elements.profileMatrix.innerHTML = `<div class="empty">${t("profile.noMatrix")}</div>`;
  elements.profileMatrixStats.textContent = t("count.zones", { count: 0 });
  elements.profileGraph.innerHTML = `<div class="empty">${t("profile.noGraph")}</div>`;
  elements.profileGraphStats.textContent = t("graph.annualHeatmap");
  elements.profileApplyButton.disabled = true;
}

function renderProfileSettings(profile) {
  const settings = state.profileSettings;
  const dimensions = profile.dimensions || [];
  elements.profileSettings.innerHTML = `
    <div class="profile-live-controls">
      <div class="profile-live-group">
        <span class="profile-live-label">${t("profile.inspectBy")}</span>
        <div class="profile-view-switch" role="tablist" aria-label="${escapeHTML(t("profile.inspectBy"))}">
          <button class="profile-segment-button ${state.activeProfileView === "profile" ? "active" : ""}" type="button" data-profile-view="profile">
            ${escapeHTML(t("profile.viewProfiles"))}
          </button>
          <button class="profile-segment-button ${state.activeProfileView === "zone" ? "active" : ""}" type="button" data-profile-view="zone">
            ${escapeHTML(t("profile.viewZones"))}
          </button>
        </div>
      </div>
      <div class="profile-live-group">
        <span class="profile-live-label">${t("common.dimensions")}</span>
        <div class="profile-toggle-row">
          ${dimensions
            .map(
              (dimension) => `
                <label class="profile-check compact">
                  <input data-profile-dimension="${escapeHTML(dimension.id)}" type="checkbox" ${settings.enabledDimensions.includes(dimension.id) ? "checked" : ""} />
                  <span>${escapeHTML(profileDimensionLabel(dimension.id))}</span>
                </label>`,
            )
            .join("")}
        </div>
      </div>
      <label class="profile-field profile-live-select">
        <span>${t("profile.metricMode", {}, "Metric")}</span>
        <select id="profileMetricMode">
          ${optionHTML("design", t("graph.designValue", {}, "Design"), currentProfileMetricMode())}
          ${optionHTML("multiplier", t("graph.multiplier"), currentProfileMetricMode())}
          ${optionHTML("actual", t("graph.actualValue"), currentProfileMetricMode())}
          ${optionHTML("annual", t("graph.annualContribution", {}, "Annual"), currentProfileMetricMode())}
        </select>
      </label>
    </div>`;
}

function applyModeSelect(id, value) {
  return `
      <select id="${escapeHTML(id)}">
        ${optionHTML("clone", t("profile.applyClone"), value)}
        ${optionHTML("shared", t("profile.applyShared"), value)}
      </select>
    `;
}

function replacePolicySelect(id, value) {
  return `
      <select id="${escapeHTML(id)}">
        ${optionHTML("replace", t("profile.existingReplace"), value)}
        ${optionHTML("keep", t("profile.existingKeep"), value)}
        ${optionHTML("duplicate", t("profile.existingDuplicate"), value)}
      </select>
    `;
}

function optionHTML(value, label, selected) {
  return `<option value="${escapeHTML(value)}" ${String(selected) === String(value) ? "selected" : ""}>${escapeHTML(label)}</option>`;
}

function profileNavigationIndex() {
  return state.semanticProjection?.navigation || {};
}

function profileSemanticAttributes(targetIDs, options = {}) {
  const targets = [...new Set((targetIDs || []).map((value) => String(value || "").trim()).filter(Boolean))];
  for (const targetID of targets) {
    const record = profileSemanticRecordForTarget(targetID);
    if (!record) {
      continue;
    }
    const attributes = [
      semanticDataAttribute("data-entity-id", record.entity.id),
      semanticDataAttribute("data-entity-kind", record.entity.kind),
      semanticDataAttribute("data-panel-target-id", targetID),
      semanticDataAttribute("data-occurrence-id", record.occurrences.length === 1 ? record.occurrences[0].occurrenceId : ""),
      semanticDataAttribute(
        "data-occurrence-context",
        options.occurrenceContext || uniqueSemanticValue(record.occurrences, "contextKind"),
      ),
      semanticDataAttribute("data-source-object-id", record.sourceAnchor?.objectId),
      semanticDataAttribute("data-source-object-index", record.sourceAnchor?.objectIndex),
      semanticDataAttribute("data-source-field-index", record.sourceAnchor?.fieldIndex),
      semanticDataAttribute("data-source-object-type", record.sourceAnchor?.objectType),
      semanticDataAttribute("data-source-object-name", record.sourceAnchor?.objectName),
      semanticDataAttribute("data-source-field-name", record.sourceAnchor?.fieldName),
    ].filter(Boolean);
    return attributes.join(" ");
  }
  return "";
}

function profileSemanticRecordForTarget(targetID) {
  const navigation = profileNavigationIndex();
  const occurrenceByID = new Map((navigation.occurrences || []).map((occurrence) => [occurrence.occurrenceId, occurrence]));
  const occurrences = (navigation.byViewTarget?.[`profile|${targetID}`] || [])
    .map((occurrenceID) => occurrenceByID.get(occurrenceID))
    .filter(Boolean);
  const entityIDs = [...new Set(occurrences.map((occurrence) => occurrence.entityId).filter(Boolean))];
  if (entityIDs.length !== 1) {
    return null;
  }
  const entity = (navigation.entities || []).find((candidate) => candidate.id === entityIDs[0]);
  if (!entity) {
    return null;
  }
  const anchors = [...occurrences.map((occurrence) => occurrence.sourceAnchor), ...(entity.sourceAnchors || [])]
    .filter(Boolean);
  const anchorsByKey = new Map(anchors.map((anchor) => [profileSourceAnchorKey(anchor), anchor]));
  return {
    entity,
    occurrences,
    sourceAnchor: anchorsByKey.size === 1 ? anchorsByKey.values().next().value : null,
  };
}

function semanticDataAttribute(name, value) {
  if (value === undefined || value === null || String(value) === "") {
    return "";
  }
  return `${name}="${escapeHTML(String(value))}"`;
}

function uniqueSemanticValue(items, key) {
  const values = [...new Set((items || []).map((item) => String(item?.[key] || "")).filter(Boolean))];
  return values.length === 1 ? values[0] : "";
}

function profileSourceAnchorKey(anchor = {}) {
  return [anchor.objectId || "", anchor.objectIndex ?? "", anchor.fieldIndex ?? "", anchor.objectType || "", anchor.objectName || ""].join("|");
}

function profileMatrixSemanticTargets(zoneName, dimension, itemIDs = []) {
  const targets = [profileZoneDimensionTargetID(zoneName, dimension)];
  if (itemIDs.length === 1) {
    targets.push(itemIDs[0]);
  }
  return targets;
}

function profileGroupSemanticTargets(group = {}) {
  const reportGroup = (state.report?.profile?.groups || []).find((candidate) => (
    sameStringSet(candidate.zoneNames, group.zoneNames) && sameStringSet(candidate.itemIds, group.itemIds)
  ));
  return reportGroup?.id ? [reportGroup.id] : [];
}

function profileSeriesSemanticTargets(series = {}) {
  const targets = [];
  if ((series.sourceItemIds || []).length === 1) {
    targets.push(series.sourceItemIds[0]);
  }
  if (series.scopeType === "group" && series.groupId) {
    targets.push(series.groupId);
  }
  if (series.zoneName && series.dimension) {
    targets.push(profileZoneDimensionTargetID(series.zoneName, series.dimension));
  }
  targets.push(...profileScheduleTargetNames(series.scheduleName));
  return targets;
}

function profileAggregateSemanticTargets(item = {}) {
  const targets = [];
  if ((item.sourceItemIds || []).length === 1) {
    targets.push(item.sourceItemIds[0]);
  }
  if (item.zoneName && item.dimension) {
    targets.push(profileZoneDimensionTargetID(item.zoneName, item.dimension));
  }
  if (item.groupId) {
    targets.push(item.groupId);
  }
  return targets;
}

function profileScheduleSemanticAttributes(cluster = {}) {
  const scheduleNames = (cluster.scheduleNames || []).map((value) => String(value || "").trim()).filter(Boolean);
  if (scheduleNames.length !== 1) {
    return "";
  }
  return profileSemanticAttributes(scheduleNames, { occurrenceContext: "zone_profile" });
}

function profileScheduleTargetNames(value) {
  const normalized = String(value || "").trim();
  if (!normalized) {
    return [];
  }
  return [normalized, ...normalized.split(/\s+\+\s+/).map((item) => item.trim()).filter(Boolean)];
}

function profileZoneDimensionTargetID(zoneName, dimension) {
  return `profile-zone-dimension:${profileSemanticToken(zoneName)}:${profileSemanticToken(dimension)}`;
}

function profileSemanticToken(value) {
  const bytes = new TextEncoder().encode(String(value || "").trim().toLowerCase());
  return [...bytes]
    .map((byte) => (
      (byte >= 97 && byte <= 122) || (byte >= 48 && byte <= 57) || byte === 45 || byte === 95 || byte === 46
        ? String.fromCharCode(byte)
        : `%${byte.toString(16).padStart(2, "0")}`
    ))
    .join("");
}

function sameStringSet(left = [], right = []) {
  const first = [...new Set(left.map((item) => String(item || "")))].sort();
  const second = [...new Set(right.map((item) => String(item || "")))].sort();
  return first.length === second.length && first.every((item, index) => item === second[index]);
}

function renderProfileOverview(groups, rows) {
  if (state.activeProfileView === "zone") {
    elements.profileOverview.innerHTML = rows.length
      ? rows.map(renderProfileZoneCard).join("")
      : `<div class="empty">${t("profile.noMatchingZones")}</div>`;
    return;
  }
  elements.profileOverview.innerHTML = groups.length
    ? groups.map(renderProfileGroupCard).join("")
    : `<div class="empty">${t("profile.noMatchingGroups")}</div>`;
}

function renderProfileGroupCard(group) {
  const active = group.id === state.activeProfileGroupId ? "active" : "";
  return `
    <button class="profile-group-card navigable-row ${active}" data-profile-group-id="${escapeHTML(group.id)}" type="button"
      ${profileSemanticAttributes(profileGroupSemanticTargets(group), { occurrenceContext: "zone_profile" })}>
      <span>
        <strong>${escapeHTML(group.name)}</strong>
        <small>${escapeHTML(t("count.zones", { count: group.zoneCount }))}</small>
      </span>
      <span class="profile-card-zones">${escapeHTML(group.zoneNames.slice(0, 4).join(", "))}${group.zoneNames.length > 4 ? "..." : ""}</span>
      <span class="profile-card-metrics">${group.dimensions.map((dimension) => `${escapeHTML(dimension.label)} ${escapeHTML(dimension.displayValue)}`).join(" / ")}</span>
    </button>`;
}

function renderProfileZoneCard(row) {
  const active = row.zoneName === state.activeProfileZoneName ? "active" : "";
  return `
    <button class="profile-group-card profile-zone-card navigable-row ${active}" data-profile-zone="${escapeHTML(row.zoneName)}" type="button"
      ${profileSemanticAttributes([row.zoneName], { occurrenceContext: "zone_profile" })}>
      <span>
        <strong>${escapeHTML(row.zoneName)}</strong>
        <small>${escapeHTML(row.groupName || t("profile.noProfileGroup"))}</small>
      </span>
      <span class="profile-card-zones">${escapeHTML(t("profile.receivesProfile", { profile: row.groupName || t("profile.noProfileGroup") }))}</span>
      <span class="profile-card-metrics">${row.dimensions.map((dimension) => `${escapeHTML(dimension.label)} ${escapeHTML(dimension.displayValue)}`).join(" / ")}</span>
    </button>`;
}

function renderProfileDetail(group, profile, zoneRow = null) {
  if (!group) {
    elements.profileDetail.innerHTML = `<div class="empty">${t("profile.noProfileGroup")}</div>`;
    return;
  }
  const itemMap = profileItemMap(profile);
  const dimensions = zoneRow?.dimensions || group.dimensions;
  const itemIds = zoneRow?.dimensions?.flatMap((dimension) => dimension.itemIds || []) || group.itemIds;
  const items = uniqueProfileItems(itemIds.map((id) => itemMap.get(id)).filter(Boolean));
  const warnings = [...(zoneRow?.warnings || []), ...group.warnings, ...items.flatMap((item) => item.warnings || [])];
  const candidates = profileCandidatesForDimensions(profile, dimensions.map((dimension) => dimension.dimension));
  elements.profileDetail.innerHTML = `
    <div class="profile-detail-head">
      <div>
        <h3>${escapeHTML(zoneRow ? zoneRow.zoneName : group.name)}</h3>
        <p>${escapeHTML(zoneRow ? t("profile.receivesProfile", { profile: group.name }) : group.zoneNames.join(", "))}</p>
      </div>
      <span class="badge">${escapeHTML(t("count.zones", { count: group.zoneCount }))}</span>
    </div>
    <div class="profile-dimension-grid">
      ${dimensions.map(renderProfileDimensionSummary).join("")}
    </div>
    ${warnings.length ? `<div class="profile-warning-list">${warnings.map(renderProfileWarning).join("")}</div>` : ""}
    ${candidates.length ? renderProfileCandidatePanel(candidates) : ""}
    <div class="profile-item-table" role="table" aria-label="${escapeHTML(t("profile.sourceObjects"))}">
      <div class="profile-item-row head" role="row">
        <span>${t("common.dimension")}</span><span>${t("common.source")}</span><span>${t("common.schedule")}</span><span>${t("common.method")}</span><span>${t("common.normalized")}</span>
      </div>
      ${items.map(renderProfileItemRow).join("")}
    </div>
    <div class="profile-source-accordion-list">
      ${items.map(renderProfileSourceAccordion).join("")}
    </div>`;
}

function renderProfileDimensionSummary(dimension) {
  return `
    <article class="profile-dimension-card">
      <strong>${escapeHTML(dimension.label)}</strong>
      <span>${escapeHTML(dimension.displayValue)}</span>
      <small>${escapeHTML(dimension.scheduleName || dimension.schedulePattern || t("profile.noSchedule"))}</small>
    </article>`;
}

function renderProfileItemRow(item) {
  const metrics = (item.normalized || [])
    .filter((metric) => metric.status !== "missing")
    .map((metric) => `${profileMetricLabel(item.dimension, metric.id, metric.label)}: ${metric.displayValue}`)
    .join("; ");
  return `
    <div class="profile-item-row navigable-row" role="row" tabindex="0"
      ${profileSemanticAttributes([item.id], { occurrenceContext: "zone_profile" })}>
      <span>${escapeHTML(profileDimensionLabel(item.dimension))}</span>
      <span>
        <button class="profile-object-link navigable-row" data-jump-object-index="${escapeHTML(item.objectIndex)}" data-jump-object-type="${escapeHTML(item.objectType)}" type="button">
          #${escapeHTML(Number(item.objectIndex) + 1)}
        </button>
        ${escapeHTML(item.objectName || item.objectType)}
      </span>
      <span class="navigable-row" tabindex="0" data-choose-semantic-occurrence="true"
        ${profileSemanticAttributes(profileScheduleTargetNames(item.scheduleName), { occurrenceContext: "zone_profile" })}>${escapeHTML(item.scheduleName || "N/A")}<small>${escapeHTML(item.schedulePattern || "")}</small></span>
      <span>${escapeHTML(item.rawMethod || "N/A")}<small>${escapeHTML(item.rawValue || "")}</small></span>
      <span>${escapeHTML(metrics || "N/A")}</span>
    </div>`;
}

function renderProfileCandidatePanel(candidates) {
  return `
    <div class="profile-candidate-panel">
      <h4>${escapeHTML(t("profile.parameterCandidates", {}, "Parameter candidates"))}</h4>
      ${candidates.map(renderProfileCandidateRow).join("")}
    </div>`;
}

function renderProfileSourceAccordion(item) {
  const metrics = (item.normalized || [])
    .map((metric) => `<span>${escapeHTML(profileMetricLabel(item.dimension, metric.id, metric.label))}: ${escapeHTML(metric.displayValue || "N/A")}</span>`)
    .join("");
  return `
    <details class="profile-source-accordion" ${profileSemanticAttributes([item.id], { occurrenceContext: "zone_profile" })}>
      <summary class="navigable-row" tabindex="0">
        <span>${escapeHTML(profileDimensionLabel(item.dimension))}</span>
        <strong>${escapeHTML(item.objectName || item.objectType)}</strong>
        <small class="navigable-row" tabindex="0" data-choose-semantic-occurrence="true"
          ${profileSemanticAttributes(profileScheduleTargetNames(item.scheduleName), { occurrenceContext: "zone_profile" })}>${escapeHTML(item.scheduleName || item.schedulePattern || t("profile.noSchedule"))}</small>
      </summary>
      <div>
        <button class="profile-object-link navigable-row" data-jump-object-index="${escapeHTML(item.objectIndex)}" data-jump-object-type="${escapeHTML(item.objectType)}" type="button"
          ${profileSemanticAttributes([item.id], { occurrenceContext: "zone_profile" })}>
          #${escapeHTML(Number(item.objectIndex) + 1)} ${escapeHTML(item.objectType)}
        </button>
        <span>${escapeHTML(item.rawMethod || "N/A")} ${escapeHTML(item.rawValue || "")}</span>
        <div class="profile-source-metrics">${metrics || "N/A"}</div>
      </div>
    </details>`;
}

function renderProfileMatrix(rows, query, profile) {
  const visibleRows = rows.filter((row) => profileMatrixRowMatchesQuery(row, query));
  const renderedRows = visibleRows.slice(0, PROFILE_MATRIX_RENDER_LIMIT);
  const hiddenRows = Math.max(0, visibleRows.length - renderedRows.length);
  const dimensions = (lastProfileView?.dimensions || []).filter((dimension) => (
    state.profileSettings.enabledDimensions.includes(dimension.id) ||
    dimension.id === profileNavigationRevealTarget?.dimension
  ));
  const itemMap = profileItemMap(profile);
  elements.profileMatrixStats.textContent = t("count.zones", { count: visibleRows.length });
  elements.profileMatrix.innerHTML = visibleRows.length
    ? `
      ${hiddenRows ? `<div class="empty compact">${escapeHTML(`${hiddenRows} additional zones hidden. Narrow the filter to render them.`)}</div>` : ""}
      <table>
        <thead>
          <tr><th>Zone</th>${dimensions.map((dimension) => `<th>${escapeHTML(profileDimensionLabel(dimension.id))}</th>`).join("")}</tr>
        </thead>
        <tbody>
          ${renderedRows
            .map(
              (row) => `
                <tr class="${profileMatrixRowActive(row) ? "active" : ""}" data-profile-zone="${escapeHTML(row.zoneName)}"
                  ${profileSemanticAttributes([row.zoneName], { occurrenceContext: "zone_profile" })}>
                  <th>
                    <button class="profile-object-link navigable-row" data-jump-object-index="${escapeHTML(row.zoneObjectIndex)}" data-jump-object-type="Zone" type="button">
                      #${escapeHTML(Number(row.zoneObjectIndex) + 1)}
                    </button>
                    ${escapeHTML(row.zoneName)}
                    <small>${escapeHTML(row.groupName || "")}</small>
                  </th>
                  ${dimensions
                    .map((dimension) => {
                      const summary = row.dimensions.find((item) => item.dimension === dimension.id) ||
                        temporaryProfileDimensionSummary(profile, row.zoneName, dimension.id);
                      return renderProfileMatrixCell(summary, itemMap, row);
                    })
                    .join("")}
                </tr>`,
            )
            .join("")}
        </tbody>
      </table>`
    : `<div class="empty">${t("profile.noMatchingZones")}</div>`;
}

function renderProfileMatrixCell(summary, itemMap, row) {
  if (!summary) {
    return `<td class="profile-matrix-empty">N/A</td>`;
  }
  const cellClasses = profileMatrixCellClasses(summary, row);
  const itemIds = (summary.itemIds || []).join(",");
  const semanticTargets = profileMatrixSemanticTargets(row.zoneName, summary.dimension, summary.itemIds || []);
  const objects = (summary.itemIds || [])
    .map((id) => itemMap.get(id))
    .filter(Boolean)
    .map(
      (item) => `
        <button class="profile-object-link navigable-row" data-jump-object-index="${escapeHTML(item.objectIndex)}" data-jump-object-type="${escapeHTML(item.objectType)}" type="button"
          ${profileSemanticAttributes([item.id], { occurrenceContext: "zone_profile" })}>
          #${escapeHTML(Number(item.objectIndex) + 1)} ${escapeHTML(shortObjectType(item.objectType))}
        </button>`,
    )
    .join("");
  return `
    <td class="${escapeHTML(cellClasses)}" tabindex="0" role="button"
      data-profile-cell="1"
      data-profile-zone="${escapeHTML(row.zoneName)}"
      data-profile-group-id="${escapeHTML(row.groupId || "")}"
      data-profile-dimension="${escapeHTML(summary.dimension)}"
      data-profile-schedule-hash="${escapeHTML(summary.scheduleHash || "")}"
      data-profile-schedule-name="${escapeHTML(summary.scheduleName || "")}"
      data-profile-value="${escapeHTML(String(summary.value ?? ""))}"
      data-profile-item-ids="${escapeHTML(itemIds)}"
      ${profileSemanticAttributes(semanticTargets, { occurrenceContext: "zone_profile" })}
      aria-label="${escapeHTML(`${row.zoneName} ${summary.label} ${summary.displayValue}`)}">
      <strong>${escapeHTML(summary.displayValue)}</strong>
      <small>${escapeHTML(summary.schedulePattern || summary.scheduleName || "")}</small>
      ${objects ? `<div class="profile-matrix-objects">${objects}</div>` : ""}
    </td>`;
}

function temporaryProfileDimensionSummary(profile, zoneName, dimensionID) {
  if (dimensionID !== profileNavigationRevealTarget?.dimension) {
    return null;
  }
  return profileDimensionSummary(profile, zoneName, dimensionID);
}

function renderProfileGraph(group, profile, zoneRow = null) {
  if (!group) {
    elements.profileGraph.innerHTML = `<div class="empty">${t("profile.graphSelect")}</div>`;
    return;
  }
  const sourceDimensions = zoneRow?.dimensions || group.dimensions;
  const deck = state.profileGraphDeck || mergeProfileGraphDeck(profile, null);
  const selectedSeries = profileDeckSeries(profile, group, zoneRow);
  const body = renderProfileDeckBody(profile, selectedSeries, deck);
  elements.profileGraphStats.textContent = profileGraphDeckStats(profile, selectedSeries, deck);
  elements.profileGraph.innerHTML = `
    <div class="profile-graph-toolbar">
      <div class="profile-preset-row" role="group" aria-label="${escapeHTML(t("profile.chartPresets", {}, "Chart presets"))}">
        ${profileGraphPresetButton("time_profile", "Time Profile")}
        ${profileGraphPresetButton("compare_groups", "Compare Groups")}
        ${profileGraphPresetButton("compare_zones", "Compare Zones")}
        ${profileGraphPresetButton("schedule_similarity", "Schedule Similarity")}
        ${profileGraphPresetButton("outliers", "Outliers")}
        ${profileGraphPresetButton("annual_contribution", "Annual Contribution")}
        ${profileGraphPresetButton("source_rules", "Source Rules")}
      </div>
      <label class="profile-field">
        <span>${t("common.scope", {}, "Scope")}</span>
        <select id="profileGraphScopeType">
          ${optionHTML("group", t("profile.viewProfiles"), deck.scopeType)}
          ${optionHTML("zone", t("profile.viewZones"), deck.scopeType)}
          ${optionHTML("schedule", t("profile.scheduleSummary"), deck.scopeType)}
          ${optionHTML("dimension", t("common.dimension"), deck.scopeType)}
          ${optionHTML("selection", t("common.selection", {}, "Selection"), deck.scopeType)}
        </select>
      </label>
      <label class="profile-field">
        <span>${t("common.view")}</span>
        <select id="profileGraphTimeView">
          ${optionHTML("day", t("graph.representativeDay"), deck.timeView)}
          ${optionHTML("week", t("graph.representativeWeek"), deck.timeView)}
          ${optionHTML("month", t("graph.monthlyAverage"), deck.timeView)}
          ${optionHTML("year", t("graph.annualHeatmap"), deck.timeView)}
          ${optionHTML("duration", t("graph.loadDuration"), deck.timeView)}
          ${optionHTML("rules", t("graph.periodRules"), deck.timeView)}
        </select>
      </label>
      <label class="profile-field">
        <span>${t("profile.compareMode", {}, "Compare")}</span>
        <select id="profileGraphCompareMode">
          ${optionHTML("single", t("profile.compareSingle", {}, "Single"), deck.compareMode)}
          ${optionHTML("overlay", t("profile.compareOverlay", {}, "Overlay"), deck.compareMode)}
          ${optionHTML("small_multiples", t("profile.compareSmallMultiples", {}, "Small multiples"), deck.compareMode)}
          ${optionHTML("ranking", t("profile.compareRanking", {}, "Ranking"), deck.compareMode)}
          ${optionHTML("similarity", t("profile.compareSimilarity", {}, "Similarity"), deck.compareMode)}
          ${optionHTML("outliers", t("profile.compareOutliers", {}, "Outliers"), deck.compareMode)}
        </select>
      </label>
      <label class="profile-field">
        <span>${t("common.scale")}</span>
        <select id="profileGraphScaleMode">
          ${optionHTML("auto", t("common.auto"), deck.scaleMode)}
          ${optionHTML("shared", t("common.shared", {}, "Shared"), deck.scaleMode)}
          ${optionHTML("design_peak", t("graph.designPeak"), deck.scaleMode)}
          ${optionHTML("multiplier_0_1", t("graph.multiplier01"), deck.scaleMode)}
          ${optionHTML("percentile", t("common.percentile", {}, "Percentile"), deck.scaleMode)}
        </select>
      </label>
    </div>
    ${renderProfileGraphSummary(group, zoneRow, sourceDimensions)}
    ${body}`;
}

function profileGraphPresetButton(id, label) {
  const active = currentProfilePresetID() === id ? "active" : "";
  return `<button class="profile-preset-button ${active}" type="button" data-profile-graph-preset="${escapeHTML(id)}">${escapeHTML(label)}</button>`;
}

function renderProfileDeckBody(profile, series, deck) {
  if (deck.compareMode === "similarity" || deck.scopeType === "schedule") {
    return renderProfileScheduleSimilarity(profile);
  }
  if (deck.compareMode === "outliers") {
    return renderProfileOutlierDeck(profile);
  }
  if (deck.compareMode === "ranking") {
    return renderProfileSeriesRanking(series, deck);
  }
  if (!series.length) {
    return `<div class="profile-graph-grid"><div class="empty">${t("profile.graphNoValues")}</div></div>`;
  }
  if (deck.compareMode === "overlay") {
    return renderProfileOverlay(series, deck);
  }
  const max = deck.scaleMode === "shared" ? sharedProfileSeriesMax(series, deck) : 0;
  return `
    <div class="profile-graph-grid ${deck.compareMode === "small_multiples" ? "small-multiples" : ""}">
      ${series.slice(0, 80).map((item) => renderProfileSeriesCard(item, deck, max)).join("")}
    </div>`;
}

function renderProfileSeriesCard(series, deck, sharedMax = 0) {
  const metric = profileSeriesMetric(series, deck);
  const max = sharedMax || graphScaleMaxForSeries(metric.values, series, deck, metric.unit);
  const pinned = (state.profilePinnedSeriesIds || []).includes(series.id);
  const schedule = series.schedulePattern || series.scheduleName || t("profile.noSchedule");
  const warnings = (series.warnings || []).slice(0, 2).map(renderProfileWarning).join("");
  return `
    <article class="profile-graph-card navigable-row ${pinned ? "pinned" : ""}" data-profile-series-id="${escapeHTML(series.id)}"
      tabindex="0" role="group" ${profileSemanticAttributes(profileSeriesSemanticTargets(series), { occurrenceContext: "zone_profile" })}>
      <div class="profile-graph-card-head">
        <div>
          <strong>${escapeHTML(series.dimensionLabel || profileDimensionLabel(series.dimension))}</strong>
          <span>${escapeHTML(series.label || series.zoneName || series.groupName || "")}</span>
        </div>
        <button class="profile-pin-button ${pinned ? "active" : ""}" type="button" title="${escapeHTML(t("profile.pinSeries", {}, "Pin series"))}" aria-label="${escapeHTML(t("profile.pinSeries", {}, "Pin series"))}" data-profile-pin-series="${escapeHTML(series.id)}">??/button>
      </div>
      <div class="profile-graph-meta">
        <span>${escapeHTML(schedule)}</span>
        <span>${escapeHTML(metric.label)} 쨌 ${escapeHTML(formatGraphNumber(Math.max(...metric.values, 0), metric.unit))}</span>
      </div>
      ${warnings}
      ${renderGraphVisual(metric.graphData, metric.values, max, { label: series.dimensionLabel || series.dimension, value: series.designValue }, metric.unit)}
      <small>${escapeHTML(t("graph.peakScale", { peak: formatGraphNumber(Math.max(...metric.values, 0), metric.unit), scale: formatGraphNumber(max, metric.unit) }))}</small>
    </article>`;
}

function renderProfileOverlay(series, deck) {
  const metrics = series.slice(0, 12).map((item) => ({ series: item, metric: profileSeriesMetric(item, deck) }));
  const max = sharedProfileSeriesMax(series, { ...deck, scaleMode: "shared" });
  const labels = metrics
    .map(({ series: item }, index) => `<span class="navigable-row" role="button" tabindex="0" data-profile-series-id="${escapeHTML(item.id)}"
      ${profileSemanticAttributes(profileSeriesSemanticTargets(item), { occurrenceContext: "zone_profile" })}><i style="background:${profileSeriesColor(index)}"></i>${escapeHTML(item.zoneName || item.groupName || item.label)}</span>`)
    .join("");
  return `
    <div class="profile-overlay-panel">
      ${renderOverlayGraph(metrics, max, metrics[0]?.metric?.unit || "")}
      <div class="profile-overlay-legend">${labels}</div>
    </div>`;
}

function renderOverlayGraph(items, max, unit = "") {
  const plot = { left: 30, right: 592, top: 12, bottom: 180 };
  const width = plot.right - plot.left;
  const height = plot.bottom - plot.top;
  const y = (value) => plot.bottom - (clampGraphValue(value, max) / max) * height;
  const paths = items
    .map(({ series, metric }, index) => {
      const data = metric.values.length > 420 ? downsampleValues(metric.values, 420) : metric.values;
      const path = data
        .map((value, valueIndex) => {
          const x = plot.left + (valueIndex / Math.max(data.length - 1, 1)) * width;
          return `${valueIndex === 0 ? "M" : "L"}${x.toFixed(2)},${y(value).toFixed(2)}`;
        })
        .join(" ");
      return `<path d="${path}" stroke="${profileSeriesColor(index)}" data-profile-series-id="${escapeHTML(series.id)}"
        ${profileSemanticAttributes(profileSeriesSemanticTargets(series), { occurrenceContext: "zone_profile" })}></path>`;
    })
    .join("");
  return `
    <svg class="profile-overlay-graph" viewBox="0 0 620 210" role="img" aria-label="${escapeHTML(t("profile.compareOverlay", {}, "Overlay"))}">
      <line class="profile-grid-line" x1="${plot.left}" y1="${plot.top}" x2="${plot.right}" y2="${plot.top}"></line>
      <line class="profile-grid-line" x1="${plot.left}" y1="${plot.top + height / 2}" x2="${plot.right}" y2="${plot.top + height / 2}"></line>
      <line class="profile-axis-line" x1="${plot.left}" y1="${plot.top}" x2="${plot.left}" y2="${plot.bottom}"></line>
      <line class="profile-axis-line" x1="${plot.left}" y1="${plot.bottom}" x2="${plot.right}" y2="${plot.bottom}"></line>
      <text class="profile-axis-label" x="4" y="${plot.top + 5}">${escapeHTML(formatAxisTick(max, unit))}</text>
      <text class="profile-axis-label" x="4" y="${plot.bottom + 4}">0</text>
      <g class="profile-overlay-paths">${paths}</g>
    </svg>`;
}

function renderProfileSeriesRanking(series, deck) {
  const ranked = series
    .map((item) => ({ series: item, metric: profileSeriesMetric(item, { ...deck, metricMode: deck.metricMode === "multiplier" ? "actual" : deck.metricMode }) }))
    .sort((a, b) => Math.max(...b.metric.values, 0) - Math.max(...a.metric.values, 0))
    .slice(0, 60);
  const max = Math.max(...ranked.map((item) => Math.max(...item.metric.values, 0)), 1e-9);
  return `
    <div class="profile-ranking-table" role="table" aria-label="${escapeHTML(t("profile.compareRanking", {}, "Ranking"))}">
      <div class="profile-ranking-row head" role="row"><span>${t("common.scope", {}, "Scope")}</span><span>${t("common.dimension")}</span><span>${t("common.value", {}, "Value")}</span><span></span></div>
      ${ranked
        .map(({ series: item, metric }) => {
          const value = Math.max(...metric.values, 0);
          return `
            <button class="profile-ranking-row navigable-row" type="button" data-profile-series-focus="${escapeHTML(item.id)}" role="row"
              ${profileSemanticAttributes(profileSeriesSemanticTargets(item), { occurrenceContext: "zone_profile" })}>
              <span>${escapeHTML(item.zoneName || item.groupName || item.label)}</span>
              <span>${escapeHTML(item.dimensionLabel || item.dimension)}</span>
              <span>${escapeHTML(formatGraphNumber(value, metric.unit))}</span>
              <i style="--profile-rank-width:${Math.max(2, (value / max) * 100).toFixed(2)}%"></i>
            </button>`;
        })
        .join("")}
    </div>`;
}

function renderProfileScheduleSimilarity(profile) {
  const clusters = profile.scheduleClusters || profile.graphDataset?.scheduleClusters || [];
  if (!clusters.length) {
    return `<div class="profile-similarity-grid"><div class="empty">${t("profile.noSchedule")}</div></div>`;
  }
  return `
    <div class="profile-similarity-grid">
      ${renderScheduleClusterScatter(clusters)}
      <div class="profile-cluster-table" role="table" aria-label="${escapeHTML(t("profile.compareSimilarity", {}, "Schedule similarity"))}">
        <div class="profile-cluster-row head" role="row"><span>Pattern</span><span>Schedules</span><span>Zones</span><span>Flags</span></div>
        ${clusters
          .map(
            (cluster) => `
              <button class="profile-cluster-row navigable-row" type="button" data-profile-schedule-hash="${escapeHTML(cluster.scheduleHash)}" role="row"
                data-choose-semantic-occurrence="true" ${profileScheduleSemanticAttributes(cluster)}>
                <span>${escapeHTML(cluster.pattern || cluster.label || "")}</span>
                <span>${escapeHTML((cluster.scheduleNames || []).join(", ") || cluster.scheduleHash)}</span>
                <span>${escapeHTML(String((cluster.zoneNames || []).length))}</span>
                <span>${cluster.sameContentDifferentNames ? "same content / different names" : ""}${cluster.sameNameDifferentContent ? " same name / different content" : ""}</span>
              </button>`,
          )
          .join("")}
      </div>
    </div>`;
}

function renderScheduleClusterScatter(clusters) {
  const maxX = Math.max(...clusters.map((cluster) => Number(cluster.centroidX) || 0), 1);
  const maxY = Math.max(...clusters.map((cluster) => Number(cluster.centroidY) || 0), 1);
  const points = clusters
    .map((cluster, index) => {
      const x = 30 + ((Number(cluster.centroidX) || 0) / maxX) * 250;
      const y = 170 - ((Number(cluster.centroidY) || 0) / maxY) * 145;
      const radius = Math.min(18, 5 + Math.sqrt((cluster.zoneNames || []).length || (cluster.scheduleNames || []).length || 1));
      return `<button class="profile-scatter-point navigable-row" style="--x:${x}px;--y:${y}px;--r:${radius}px;--c:${profileSeriesColor(index)}" data-profile-schedule-hash="${escapeHTML(cluster.scheduleHash)}"
        data-choose-semantic-occurrence="true" ${profileScheduleSemanticAttributes(cluster)} title="${escapeHTML((cluster.scheduleNames || []).join(", "))}" aria-label="${escapeHTML(cluster.label || cluster.scheduleHash)}"></button>`;
    })
    .join("");
  return `
    <div class="profile-scatter" role="img" aria-label="${escapeHTML(t("profile.compareSimilarity", {}, "Schedule similarity"))}">
      <span class="profile-scatter-axis x">Average multiplier</span>
      <span class="profile-scatter-axis y">Operating hours</span>
      ${points}
    </div>`;
}

function renderProfileOutlierDeck(profile) {
  const outliers = profile.outliers || profile.graphDataset?.outliers || [];
  const candidates = profile.parameterCandidates || profile.graphDataset?.parameterCandidates || [];
  return `
    <div class="profile-qa-grid">
      <section class="profile-qa-list">
        <h4>${escapeHTML(t("profile.compareOutliers", {}, "Outliers"))}</h4>
        ${outliers.length ? outliers.slice(0, 80).map(renderProfileOutlierRow).join("") : `<div class="empty">${t("output.noWarnings", {}, "No warnings")}</div>`}
      </section>
      <section class="profile-qa-list">
        <h4>${escapeHTML(t("profile.parameterCandidates", {}, "Parameter candidates"))}</h4>
        ${candidates.length ? candidates.slice(0, 40).map(renderProfileCandidateRow).join("") : `<div class="empty">${t("tools.noCandidates", {}, "No candidates")}</div>`}
      </section>
    </div>`;
}

function renderProfileOutlierRow(hint) {
  return `
    <button class="profile-qa-row navigable-row ${escapeHTML(hint.severity || "info")}" type="button" data-profile-outlier-zone="${escapeHTML(hint.zoneName || "")}" data-profile-dimension="${escapeHTML(hint.dimension || "")}" data-profile-schedule-hash="${escapeHTML(hint.scheduleHash || "")}"
      ${profileSemanticAttributes(profileAggregateSemanticTargets(hint), { occurrenceContext: "zone_profile" })}>
      <strong>${escapeHTML(hint.ruleId || hint.severity || "QA")}</strong>
      <span>${escapeHTML(hint.message || "")}</span>
      <small>${escapeHTML([hint.zoneName, profileDimensionLabel(hint.dimension), hint.scheduleName].filter(Boolean).join(" / "))}</small>
    </button>`;
}

function renderProfileCandidateRow(candidate) {
  return `
    <button class="profile-qa-row candidate navigable-row ${escapeHTML(candidate.severity || "info")}" type="button" data-profile-candidate-id="${escapeHTML(candidate.id)}" data-profile-dimension="${escapeHTML(candidate.dimension || "")}"
      ${profileSemanticAttributes(profileAggregateSemanticTargets(candidate), { occurrenceContext: "zone_profile" })}>
      <strong>${escapeHTML(candidate.label || candidate.id)}</strong>
      <span>${escapeHTML(candidate.reason || "")}</span>
      <small>${escapeHTML(`${(candidate.zoneNames || []).length} zones 쨌 ${formatGraphNumber(candidate.currentMin, "")}..${formatGraphNumber(candidate.currentMax, "")}`)}</small>
    </button>`;
}

function renderProfileGraphSummary(group, zoneRow, dimensions) {
  const title = zoneRow ? zoneRow.zoneName : group.name;
  const subtitle = zoneRow ? t("profile.receivesProfile", { profile: group.name }) : t("profile.profileServesZones", { count: group.zoneCount });
  return `
    <div class="profile-graph-summary">
      <div>
        <strong>${escapeHTML(title)}</strong>
        <span>${escapeHTML(subtitle)}</span>
      </div>
      <div class="profile-connection-row">
        <span>${escapeHTML(zoneRow ? t("profile.sameProfileZones") : t("profile.connectedZones"))}</span>
        <div>
          ${group.zoneNames
            .map(
              (zoneName) => `
                <button class="navigable-row ${zoneName === zoneRow?.zoneName ? "active" : ""}" type="button" data-profile-zone-ref="${escapeHTML(zoneName)}" title="${escapeHTML(zoneName)}"
                  ${profileSemanticAttributes([zoneName], { occurrenceContext: "zone_profile" })}>
                  ${escapeHTML(zoneName)}
                </button>`,
            )
            .join("")}
        </div>
      </div>
      <div class="profile-graph-summary-metrics">
        ${dimensions.map(renderProfileDimensionSummary).join("")}
      </div>
    </div>`;
}

function renderProfileGraphCard(dimension, schedule, viewMode, scaleMode) {
  const graphData = graphDataForDimension(dimension, schedule, viewMode);
  const values = state.profileSettings.graphMode === "multiplier"
    ? graphData.values
    : graphData.values.map((value) => value * dimension.value);
  const unit = state.profileSettings.graphMode === "multiplier" ? "" : dimension.unit;
  const max = graphScaleMax(values, dimension, graphData, scaleMode);
  const warnings = graphData.warning ? `<div class="profile-warning info">${escapeHTML(graphData.warning)}</div>` : "";
  return `
    <article class="profile-graph-card">
      <div>
        <strong>${escapeHTML(dimension.label)}</strong>
        <span>${escapeHTML(graphData.label)}</span>
      </div>
      <div class="profile-graph-meta">
        <span>${escapeHTML(t("common.unit"))}: ${escapeHTML(unit || t("graph.multiplier"))}</span>
        <span>${escapeHTML(t("common.max"))}: ${escapeHTML(formatGraphNumber(Math.max(...values, 0), unit))}</span>
      </div>
      ${warnings}
      ${renderGraphVisual(graphData, values, max, dimension, unit)}
      <small>${escapeHTML(t("graph.peakScale", { peak: formatGraphNumber(Math.max(...values, 0), unit), scale: formatGraphNumber(max, unit) }))}</small>
    </article>`;
}

function renderGraphVisual(graphData, values, max, dimension, unit) {
  switch (graphData.kind) {
    case "heatmap":
      return renderHeatmap(values, max, dimension.label, unit);
    case "rules":
      return renderRuleGraph(graphData, values, max, unit);
    case "day_profiles":
      return renderDayProfiles(graphData, values, max, unit);
    default:
      return renderLineGraph(values, max, `${dimension.label} ${graphData.label}`, unit, graphData.xLabel);
  }
}

function graphDataForDimension(dimension, schedule, viewMode) {
  const unresolvedWarning = schedule && schedule.resolved === false
    ? t("profile.scheduleUnresolvedWarning")
    : "";
  const missingWarning = !schedule && dimension.scheduleName
    ? t("profile.scheduleMissingGraph", { schedule: dimension.scheduleName })
    : "";
  const warning = unresolvedWarning || missingWarning;
  const pattern = schedule?.detectedPattern || (dimension.scheduleName ? t("profile.scheduleFallback") : t("profile.alwaysOn"));
  switch (viewMode) {
    case "representative_week":
      return {
        kind: "line",
        label: `${pattern} / ${t("graph.representativeWeek")}`,
        values: scheduleWeeklyProfile(schedule),
        warning,
        xLabel: "7d",
      };
    case "hourly_average_by_daytype": {
      const profiles = daytypeAverageProfiles(schedule, dimension);
      return {
        kind: "day_profiles",
        label: `${pattern} / ${t("graph.hourlyByDaytype")}`,
        values: profiles.flatMap((profile) => profile.values),
        profiles,
        warning,
      };
    }
    case "monthly_average":
      return {
        kind: "line",
        label: `${pattern} / ${t("graph.monthlyAverage")}`,
        values: monthlyAverageValues(schedule, dimension),
        warning,
        xLabel: "12m",
      };
    case "load_duration":
      return {
        kind: "line",
        label: `${pattern} / ${t("graph.loadDuration")}`,
        values: annualScheduleValues(schedule, dimension).sort((a, b) => b - a),
        warning,
        xLabel: "8760h",
      };
    case "period_rules":
      return {
        kind: "rules",
        label: `${pattern} / ${t("graph.periodRules")}`,
        values: scheduleRuleValues(schedule, dimension),
        rules: scheduleRules(schedule, dimension),
        warning,
      };
    case "representative_day":
      return {
        kind: "day_profiles",
        label: `${pattern} / ${t("graph.representativeDay")}`,
        values: [
          ...scheduleDayProfile(schedule, "weekdayProfile"),
          ...scheduleDayProfile(schedule, "saturdayProfile"),
          ...scheduleDayProfile(schedule, "sundayProfile"),
        ],
        profiles: [
          { label: t("day.weekday"), values: scheduleDayProfile(schedule, "weekdayProfile") },
          { label: t("day.saturday"), values: scheduleDayProfile(schedule, "saturdayProfile") },
          { label: t("day.sunday"), values: scheduleDayProfile(schedule, "sundayProfile") },
        ],
        warning,
      };
    default:
      return {
        kind: "heatmap",
        label: `${pattern} / ${t("graph.annualHeatmap")}`,
        values: annualScheduleValues(schedule, dimension),
        warning,
      };
  }
}

function renderLineGraph(values, max, label, unit = "", xLabel = "") {
  const data = values.length ? values : [0];
  const plot = { left: 28, right: 118, top: 10, bottom: 76 };
  const width = plot.right - plot.left;
  const y = (value) => plot.bottom - (clampGraphValue(value, max) / max) * (plot.bottom - plot.top);
  const stepPath = data.reduce((path, value, index) => {
    const currentY = y(value);
    const nextX = plot.left + ((index + 1) / data.length) * width;
    if (index === 0) {
      return `M${plot.left},${currentY} H${nextX}`;
    }
    return `${path} V${currentY} H${nextX}`;
  }, "");
  const mid = max / 2;
  return `
    <svg class="profile-line-graph" viewBox="0 0 124 92" role="img" aria-label="${escapeHTML(label)}">
      <line class="profile-grid-line" x1="${plot.left}" y1="${plot.top}" x2="${plot.right}" y2="${plot.top}"></line>
      <line class="profile-grid-line" x1="${plot.left}" y1="${y(mid)}" x2="${plot.right}" y2="${y(mid)}"></line>
      <line class="profile-axis-line" x1="${plot.left}" y1="${plot.top}" x2="${plot.left}" y2="${plot.bottom}"></line>
      <line class="profile-axis-line" x1="${plot.left}" y1="${plot.bottom}" x2="${plot.right}" y2="${plot.bottom}"></line>
      <text class="profile-axis-label" x="2" y="${plot.top + 4}">${escapeHTML(formatAxisTick(max, unit))}</text>
      <text class="profile-axis-label" x="2" y="${y(mid) + 4}">${escapeHTML(formatAxisTick(mid, unit))}</text>
      <text class="profile-axis-label" x="2" y="${plot.bottom + 4}">0</text>
      <text class="profile-axis-label x" x="${plot.left}" y="88">0</text>
      <text class="profile-axis-label x" x="${plot.right}" y="88" text-anchor="end">${escapeHTML(xLabel || graphXLabel(data.length))}</text>
      <path class="profile-step-path" d="${stepPath}"></path>
    </svg>`;
}

function renderDayProfiles(graphData, values, max, unit = "") {
  let offset = 0;
  return `
    <div class="profile-day-graphs">
      ${graphData.profiles
        .map((profile) => {
          const profileValues = values.slice(offset, offset + profile.values.length);
          offset += profile.values.length;
          return `
            <div>
              <span>${escapeHTML(profile.label)}</span>
              ${renderLineGraph(profileValues, max, profile.label, unit, "24h")}
            </div>`;
        })
        .join("")}
    </div>`;
}

function renderRuleGraph(graphData, values, max, unit = "") {
  const rules = graphData.rules || [];
  let offset = 0;
  return `
    <div class="profile-rule-list">
      ${rules
        .map((rule) => {
          const scaledIntervals = values.slice(offset, offset + (rule.intervals || []).length);
          offset += (rule.intervals || []).length;
          const ruleValues = (rule.intervals || []).flatMap((interval, index) => {
            const hours = Math.max(1, Math.round((Number(interval.endHour) || 0) - (Number(interval.startHour) || 0)));
            return Array.from({ length: hours }, () => Number(scaledIntervals[index]) || 0);
          });
          return `
            <div class="profile-rule-row">
              <span>${escapeHTML(rule.label || `${rule.through || ""} ${rule.selector || ""}`)}</span>
              ${renderLineGraph(ruleValues.length ? ruleValues : [0], max, rule.label || "Schedule rule", unit, "24h")}
            </div>`;
        })
        .join("") || `<div class="empty">${t("profile.noRules")}</div>`}
    </div>`;
}

function renderHeatmap(values, max, label, unit = "") {
  const rects = values
    .map((value, index) => {
      const day = Math.floor(index / 24);
      const hour = index % 24;
      return `<rect x="${day}" y="${hour}" width="1" height="1" fill="${heatColor(value, max)}"></rect>`;
    })
    .join("");
  return `
    <div class="profile-heatmap-frame" role="img" aria-label="${escapeHTML(`${label} ${t("graph.annualHeatmap")}`)}">
      <div class="profile-heatmap-y">
        <span>00</span>
        <span>12</span>
        <span>24</span>
      </div>
      <svg class="profile-heatmap" viewBox="0 0 365 24" preserveAspectRatio="none" aria-hidden="true">
        ${rects}
      </svg>
      <div class="profile-heatmap-x">
        <span>Jan</span>
        <span>${escapeHTML(formatAxisTick(max / 2, unit))}</span>
        <span>${escapeHTML(formatAxisTick(max, unit))}</span>
        <span>Dec</span>
      </div>
    </div>`;
}

function mergeProfileGraphDeck(profile, saved) {
  const defaults = profile?.graphDataset?.defaultDeck || profile?.defaultSettings?.graphDeck || {};
  const source = saved || defaults || {};
  const deck = {
    scopeType: source.scopeType || defaults.scopeType || "group",
    selectedGroupIds: Array.isArray(source.selectedGroupIds) ? source.selectedGroupIds : defaults.selectedGroupIds || [],
    selectedZoneNames: Array.isArray(source.selectedZoneNames) ? source.selectedZoneNames : defaults.selectedZoneNames || [],
    selectedScheduleHashes: Array.isArray(source.selectedScheduleHashes) ? source.selectedScheduleHashes : defaults.selectedScheduleHashes || [],
    selectedDimensions: Array.isArray(source.selectedDimensions) ? source.selectedDimensions : defaults.selectedDimensions || state.profileSettings?.enabledDimensions || [],
    metricMode: source.metricMode || state.profileSettings?.metricMode || profileMetricModeFromLegacy(state.profileSettings?.graphMode) || "actual",
    timeView: source.timeView || state.profileSettings?.timeView || profileTimeViewFromLegacy(state.profileSettings?.scheduleSummaryMode) || "year",
    compareMode: source.compareMode || state.profileSettings?.compareMode || "single",
    scaleMode: source.scaleMode || state.profileSettings?.scaleMode || state.profileGraphScaleMode || "auto",
    timeRange: Array.isArray(source.timeRange) ? source.timeRange : defaults.timeRange || [],
    pinnedSeriesIds: Array.isArray(source.pinnedSeriesIds) ? source.pinnedSeriesIds : state.profilePinnedSeriesIds || [],
  };
  state.profilePinnedSeriesIds = deck.pinnedSeriesIds;
  state.profileGraphScaleMode = deck.scaleMode;
  state.profileGraphViewMode = deck.timeView;
  return deck;
}

function currentProfileMetricMode() {
  return state.profileGraphDeck?.metricMode || state.profileSettings?.metricMode || profileMetricModeFromLegacy(state.profileSettings?.graphMode) || "actual";
}

function profileMetricModeFromLegacy(value) {
  switch (String(value || "").trim().toLowerCase()) {
    case "multiplier":
      return "multiplier";
    case "design":
      return "design";
    case "annual":
      return "annual";
    default:
      return "actual";
  }
}

function profileTimeViewFromLegacy(value) {
  switch (String(value || "").trim().toLowerCase()) {
    case "representative_day":
      return "day";
    case "representative_week":
    case "hourly_average_by_daytype":
      return "week";
    case "monthly_average":
      return "month";
    case "load_duration":
      return "duration";
    case "period_rules":
      return "rules";
    default:
      return "year";
  }
}

function currentProfilePresetID() {
  const deck = state.profileGraphDeck || {};
  if (deck.compareMode === "similarity") return "schedule_similarity";
  if (deck.compareMode === "outliers") return "outliers";
  if (deck.compareMode === "ranking" && deck.metricMode === "annual") return "annual_contribution";
  if (deck.timeView === "rules") return "source_rules";
  if (deck.scopeType === "group" && deck.compareMode === "overlay") return "compare_groups";
  if (deck.scopeType === "zone" && (deck.compareMode === "small_multiples" || deck.compareMode === "overlay")) return "compare_zones";
  return "time_profile";
}

function profileDeckSeries(profile, group, zoneRow = null) {
  const deck = state.profileGraphDeck || {};
  const enabledDimensions = new Set(state.profileSettings?.enabledDimensions || []);
  const selectedDimensions = new Set(deck.selectedDimensions?.length ? deck.selectedDimensions : state.profileSettings?.enabledDimensions || []);
  const pinned = new Set(state.profilePinnedSeriesIds || []);
  const cell = state.profileSelectedCell || null;
  const allSeries = Array.isArray(profile?.graphDataset?.series) ? profile.graphDataset.series : [];
  const base = allSeries.filter((series) => {
    if (enabledDimensions.size && !enabledDimensions.has(series.dimension)) return false;
    if (selectedDimensions.size && !selectedDimensions.has(series.dimension)) return false;
    return true;
  });
  const selected = base.filter((series) => profileSeriesInDeckScope(series, deck, group, zoneRow, cell));
  base.forEach((series) => {
    if (pinned.has(series.id) && !selected.some((item) => item.id === series.id)) {
      selected.push(series);
    }
  });
  return selected.slice(0, 120);
}

function profileSeriesInDeckScope(series, deck, group, zoneRow, cell) {
  if (deck.scopeType === "selection" && cell) {
    if (cell.itemIds?.some((id) => (series.sourceItemIds || []).includes(id))) return true;
    if (cell.scheduleHash && series.scheduleHash === cell.scheduleHash && series.dimension === cell.dimension) return true;
    return series.zoneName === cell.zoneName && series.dimension === cell.dimension;
  }
  if (deck.scopeType === "schedule") {
    const hashes = new Set([...(deck.selectedScheduleHashes || []), cell?.scheduleHash].filter(Boolean));
    return hashes.size ? hashes.has(series.scheduleHash) : Boolean(series.scheduleHash);
  }
  if (deck.scopeType === "dimension") {
    return (deck.selectedDimensions || []).includes(series.dimension);
  }
  if (deck.scopeType === "zone") {
    if (deck.compareMode === "small_multiples" || deck.compareMode === "overlay" || deck.compareMode === "ranking") {
      const groupZones = new Set(group?.zoneNames || []);
      return series.scopeType === "zone" && groupZones.has(series.zoneName);
    }
    const zoneName = zoneRow?.zoneName || cell?.zoneName || state.activeProfileZoneName;
    return series.scopeType === "zone" && series.zoneName === zoneName;
  }
  const groupID = group?.id || state.activeProfileGroupId;
  if (deck.compareMode === "overlay" || deck.compareMode === "ranking") {
    return series.scopeType === "group" || ((deck.selectedGroupIds || []).includes(series.groupId));
  }
  return series.scopeType === "group" && series.groupId === groupID;
}

function profileSeriesMetric(series, deck) {
  const timeView = deck.timeView || "year";
  const metricMode = deck.metricMode || currentProfileMetricMode();
  const multiplier = profileSeriesMultiplier(series, timeView);
  let values = multiplier;
  let unit = "";
  let label = t("graph.multiplier");
  if (metricMode === "design") {
    values = multiplier.map(() => Number(series.designValue) || 0);
    unit = series.unit || "";
    label = t("graph.designValue", {}, "Design");
  } else if (metricMode === "actual") {
    values = multiplier.map((value) => value * (Number(series.designValue) || 0));
    unit = series.unit || "";
    label = t("graph.actualValue");
  } else if (metricMode === "annual") {
    values = annualizedProfileValues(series, multiplier, timeView);
    unit = series.unit ? `${series.unit}h` : "h";
    label = t("graph.annualContribution", {}, "Annual");
  }
  values = values.map((value) => (Number.isFinite(Number(value)) ? Number(value) : 0));
  return {
    label,
    unit,
    values,
    graphData: profileSeriesGraphData(series, deck, multiplier),
  };
}

function profileSeriesMultiplier(series, timeView) {
  switch (timeView) {
    case "day":
      return numberArray(series.dayMultiplierProfile, 72, 1);
    case "week":
      return numberArray(series.weekMultiplierProfile, 168, 1);
    case "month":
      return numberArray(series.monthMultiplierProfile, 12, 1);
    case "duration":
      return numberArray(series.durationMultiplierProfile, 8760, 1);
    case "rules":
      return numberArray(series.ruleMultiplierProfile, Math.max(1, series.ruleMultiplierProfile?.length || 1), 1);
    default:
      return numberArray(series.annualMultiplierProfile || series.values, 8760, 1);
  }
}

function profileSeriesGraphData(series, deck, multiplier) {
  const pattern = series.schedulePattern || series.scheduleName || t("profile.fallbackAllDays");
  switch (deck.timeView) {
    case "day":
      return {
        kind: "day_profiles",
        label: `${pattern} / ${t("graph.representativeDay")}`,
        values: multiplier,
        profiles: [
          { label: t("day.weekday"), values: multiplier.slice(0, 24) },
          { label: t("day.saturday"), values: multiplier.slice(24, 48) },
          { label: t("day.sunday"), values: multiplier.slice(48, 72) },
        ],
      };
    case "week":
      return { kind: "line", label: `${pattern} / ${t("graph.representativeWeek")}`, values: multiplier, xLabel: "7d" };
    case "month":
      return { kind: "line", label: `${pattern} / ${t("graph.monthlyAverage")}`, values: multiplier, xLabel: "12m" };
    case "duration":
      return { kind: "line", label: `${pattern} / ${t("graph.loadDuration")}`, values: multiplier, xLabel: "8760h" };
    case "rules":
      return { kind: "rules", label: `${pattern} / ${t("graph.periodRules")}`, values: multiplier, rules: fallbackRulesForSeries(series, multiplier) };
    default:
      return { kind: "heatmap", label: `${pattern} / ${t("graph.annualHeatmap")}`, values: multiplier };
  }
}

function annualizedProfileValues(series, multiplier, timeView) {
  const design = Number(series.designValue) || 0;
  if (timeView === "month") {
    const hours = [744, 672, 744, 720, 744, 720, 744, 744, 720, 744, 720, 744];
    return multiplier.map((value, index) => value * design * (hours[index] || 730));
  }
  if (timeView === "rules") {
    return multiplier.map((value) => value * design * 24);
  }
  return multiplier.map((value) => value * design);
}

function fallbackRulesForSeries(series, values) {
  if (!values.length) {
    return [];
  }
  return [{
    label: series.scheduleName || t("profile.fallbackAllDays"),
    intervals: values.map((value, index) => ({ startHour: index, endHour: index + 1, value })),
  }];
}

function graphScaleMaxForSeries(values, series, deck, unit) {
  const metricMode = deck.metricMode || currentProfileMetricMode();
  if (deck.scaleMode === "multiplier_0_1") {
    return metricMode === "multiplier" ? 1 : Math.max(Number(series.designValue) || 0, 1e-9);
  }
  if (deck.scaleMode === "design_peak") {
    return Math.max(Number(series.designValue) || 0, Math.max(...values, 0), 1e-9);
  }
  if (deck.scaleMode === "percentile") {
    return percentileValue(values, 0.95) || Math.max(...values, 1e-9);
  }
  return Math.max(...values, 1e-9);
}

function sharedProfileSeriesMax(series, deck) {
  return Math.max(...series.map((item) => {
    const metric = profileSeriesMetric(item, deck);
    return graphScaleMaxForSeries(metric.values, item, { ...deck, scaleMode: deck.scaleMode === "shared" ? "auto" : deck.scaleMode }, metric.unit);
  }), 1e-9);
}

function profileGraphDeckStats(profile, series, deck) {
  if (deck.compareMode === "similarity" || deck.scopeType === "schedule") {
    return `${(profile.scheduleClusters || []).length} schedule clusters`;
  }
  if (deck.compareMode === "outliers") {
    return `${(profile.outliers || []).length} QA hints / ${(profile.parameterCandidates || []).length} candidates`;
  }
  return `${series.length} series 쨌 ${deck.metricMode} 쨌 ${deck.timeView} 쨌 ${deck.compareMode}`;
}

function profileMatrixCellClasses(summary, row) {
  const cell = state.profileSelectedCell;
  const classes = ["profile-matrix-cell"];
  if (!cell) {
    return classes.join(" ");
  }
  const sameZone = cell.zoneName === row.zoneName;
  const sameDimension = cell.dimension === summary.dimension;
  const sameSchedule = cell.scheduleHash && cell.scheduleHash === summary.scheduleHash;
  const sameGroup = cell.groupId && cell.groupId === row.groupId;
  const sameValue = Math.abs((Number(cell.value) || 0) - (Number(summary.value) || 0)) <= 0.0001;
  if (sameZone && sameDimension) classes.push("active");
  if (sameGroup) classes.push("same-group");
  if (sameSchedule) classes.push("same-schedule");
  if (sameSchedule && !sameValue && sameDimension) classes.push("same-schedule-different-value");
  if (!sameSchedule && sameValue && sameDimension) classes.push("same-value-different-schedule");
  return classes.join(" ");
}

function numberArray(values, fallbackLength, fallbackValue = 0) {
  if (Array.isArray(values) && values.length) {
    return values.map((value) => Number(value) || 0);
  }
  return Array.from({ length: fallbackLength }, () => fallbackValue);
}

function percentileValue(values, percentile) {
  const sorted = values.filter((value) => Number.isFinite(value)).slice().sort((a, b) => a - b);
  if (!sorted.length) return 0;
  const index = Math.min(sorted.length - 1, Math.max(0, Math.ceil(sorted.length * percentile) - 1));
  return sorted[index];
}

function downsampleValues(values, targetLength) {
  if (values.length <= targetLength) return values;
  const step = values.length / targetLength;
  return Array.from({ length: targetLength }, (_, index) => values[Math.floor(index * step)] || 0);
}

function profileSeriesColor(index) {
  const colors = ["#007c89", "#a85f00", "#4d6f9f", "#7a5a9e", "#2f7d4f", "#b04444", "#667085", "#0b5f6a", "#8a6f2a", "#805ad5", "#2b6cb0", "#b83280"];
  return colors[index % colors.length];
}

function currentGraphViewMode() {
  const allowed = new Set(["annual_heatmap", "representative_week", "hourly_average_by_daytype", "monthly_average", "load_duration", "period_rules", "representative_day"]);
  if (allowed.has(state.profileGraphViewMode)) {
    return state.profileGraphViewMode;
  }
  return "annual_heatmap";
}

function graphStatsLabel(viewMode, graphMode) {
  switch (viewMode) {
    case "representative_week":
      return graphMode === "multiplier" ? t("graph.multiplierWeek") : t("graph.actualWeek");
    case "hourly_average_by_daytype":
      return `${graphMode === "multiplier" ? t("graph.multiplier") : t("graph.actualValue")}, ${t("graph.hourlyByDaytype")}`;
    case "monthly_average":
      return `${graphMode === "multiplier" ? t("graph.multiplier") : t("graph.actualValue")}, ${t("graph.monthlyAverage")}`;
    case "load_duration":
      return `${graphMode === "multiplier" ? t("graph.multiplier") : t("graph.actualValue")}, ${t("graph.loadDuration")}`;
    case "period_rules":
      return graphMode === "multiplier" ? t("graph.multiplierRules") : t("graph.actualRules");
    case "representative_day":
      return graphMode === "multiplier" ? t("graph.multiplierDay") : t("graph.actualDay");
    default:
      return graphMode === "multiplier" ? t("graph.multiplierAnnual") : t("graph.actualAnnual");
  }
}

function graphScaleMax(values, dimension, graphData, scaleMode) {
  if (scaleMode === "multiplier_0_1") {
    return state.profileSettings.graphMode === "multiplier" ? 1 : Math.max(Number(dimension.value) || 0, 1e-9);
  }
  if (scaleMode === "design_peak") {
    return state.profileSettings.graphMode === "multiplier" ? Math.max(...graphData.values, 1) : Math.max(Number(dimension.value) || 0, 1e-9);
  }
  return Math.max(...values, 1e-9);
}

function scheduleLookupMap(schedules) {
  const map = new Map();
  schedules.forEach((schedule) => {
    const name = String(schedule.scheduleName || "").trim();
    if (!name) {
      return;
    }
    map.set(name, schedule);
    map.set(normalizeProfileScheduleName(name), schedule);
  });
  return map;
}

function scheduleForProfileDimension(dimension, item, schedules) {
  const names = [
    item?.scheduleName,
    dimension.scheduleName,
    ...(String(dimension.scheduleName || "")
      .split("+")
      .map((name) => name.trim())
      .filter(Boolean)),
  ];
  for (const name of names) {
    const schedule = schedules.get(name) || schedules.get(normalizeProfileScheduleName(name));
    if (schedule) {
      return schedule;
    }
  }
  return null;
}

function normalizeProfileScheduleName(name) {
  return String(name || "").trim().toLowerCase();
}

function scheduleDayProfile(schedule, key) {
  const values = schedule?.[key];
  if (Array.isArray(values) && values.length) {
    return values.map((value) => Number(value) || 0);
  }
  return Array.from({ length: 24 }, () => 1);
}

function scheduleWeeklyProfile(schedule) {
  if (Array.isArray(schedule?.weeklyProfile) && schedule.weeklyProfile.length) {
    return schedule.weeklyProfile.map((value) => Number(value) || 0);
  }
  return [
    ...scheduleDayProfile(schedule, "weekdayProfile"),
    ...scheduleDayProfile(schedule, "weekdayProfile"),
    ...scheduleDayProfile(schedule, "weekdayProfile"),
    ...scheduleDayProfile(schedule, "weekdayProfile"),
    ...scheduleDayProfile(schedule, "weekdayProfile"),
    ...scheduleDayProfile(schedule, "saturdayProfile"),
    ...scheduleDayProfile(schedule, "sundayProfile"),
  ];
}

function scheduleRules(schedule, dimension = {}) {
  return Array.isArray(schedule?.rules) && schedule.rules.length
    ? schedule.rules
    : [{
        startDay: 1,
        endDay: 365,
        selector: "AllDays",
        label: dimension.scheduleName ? t("profile.scheduleFallback") : t("profile.alwaysOn"),
        intervals: [{ startHour: 0, endHour: 24, value: 1 }],
      }];
}

function scheduleRuleValues(schedule, dimension = {}) {
  return scheduleRules(schedule, dimension).flatMap((rule) => (rule.intervals || []).map((interval) => Number(interval.value) || 0));
}

function annualScheduleValues(schedule, dimension = {}) {
  const rules = scheduleRules(schedule, dimension);
  const values = [];
  for (let day = 1; day <= 365; day += 1) {
    const rule = rules.find((candidate) => day >= Number(candidate.startDay || 1) && day <= Number(candidate.endDay || 365) && dayMatchesScheduleSelector(day, candidate.selector));
    values.push(...profileFromRule(rule));
  }
  return values;
}

function monthlyAverageValues(schedule, dimension = {}) {
  const values = annualScheduleValues(schedule, dimension);
  const monthDays = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  let offset = 0;
  return monthDays.map((days) => {
    const hours = days * 24;
    const monthValues = values.slice(offset, offset + hours);
    offset += hours;
    if (!monthValues.length) {
      return 0;
    }
    return monthValues.reduce((sum, value) => sum + value, 0) / monthValues.length;
  });
}

function daytypeAverageProfiles(schedule, dimension = {}) {
  const values = annualScheduleValues(schedule, dimension);
  const buckets = {
    weekday: Array.from({ length: 24 }, () => []),
    saturday: Array.from({ length: 24 }, () => []),
    sunday: Array.from({ length: 24 }, () => []),
  };
  for (let day = 1; day <= 365; day += 1) {
    const dayOfWeek = (day - 1) % 7;
    const key = dayOfWeek === 5 ? "saturday" : dayOfWeek === 6 ? "sunday" : "weekday";
    for (let hour = 0; hour < 24; hour += 1) {
      buckets[key][hour].push(Number(values[(day - 1) * 24 + hour]) || 0);
    }
  }
  return [
    { label: t("day.weekday"), values: averageHourlyBucket(buckets.weekday) },
    { label: t("day.saturday"), values: averageHourlyBucket(buckets.saturday) },
    { label: t("day.sunday"), values: averageHourlyBucket(buckets.sunday) },
  ];
}

function averageHourlyBucket(bucket) {
  return bucket.map((values) => (values.length ? values.reduce((sum, value) => sum + value, 0) / values.length : 0));
}

function profileFromRule(rule) {
  const profile = Array.from({ length: 24 }, () => 0);
  (rule?.intervals || []).forEach((interval) => {
    const start = Math.max(0, Math.floor(Number(interval.startHour) || 0));
    const end = Math.min(24, Math.ceil(Number(interval.endHour) || 0));
    for (let hour = start; hour < end; hour += 1) {
      profile[hour] = Number(interval.value) || 0;
    }
  });
  return profile;
}

function dayMatchesScheduleSelector(day, selectorInput) {
  const dayOfWeek = (day - 1) % 7;
  for (const selector of scheduleSelectorTokens(selectorInput || "AllDays")) {
    switch (selector) {
      case "alldays":
      case "everyday":
      case "allotherdays":
        return true;
      case "weekdays":
        if (dayOfWeek >= 0 && dayOfWeek <= 4) return true;
        break;
      case "weekends":
        if (dayOfWeek === 5 || dayOfWeek === 6) return true;
        break;
      case "monday":
        if (dayOfWeek === 0) return true;
        break;
      case "tuesday":
        if (dayOfWeek === 1) return true;
        break;
      case "wednesday":
        if (dayOfWeek === 2) return true;
        break;
      case "thursday":
        if (dayOfWeek === 3) return true;
        break;
      case "friday":
        if (dayOfWeek === 4) return true;
        break;
      case "saturday":
        if (dayOfWeek === 5) return true;
        break;
      case "sunday":
        if (dayOfWeek === 6) return true;
        break;
      default:
        break;
    }
  }
  return false;
}

function scheduleSelectorTokens(selectorInput) {
  return String(selectorInput || "")
    .replaceAll(",", " ")
    .trim()
    .split(/\s+/)
    .map((selector) => selector.trim().toLowerCase().replaceAll(" ", ""))
    .filter(Boolean);
}

function heatColor(value, max) {
  const t = Math.max(0, Math.min(1, max <= 0 ? 0 : value / max));
  const stops = [
    [238, 243, 245],
    [162, 208, 207],
    [0, 124, 137],
    [168, 95, 0],
  ];
  const scaled = t * (stops.length - 1);
  const index = Math.min(stops.length - 2, Math.floor(scaled));
  const local = scaled - index;
  const color = stops[index].map((start, channel) => Math.round(start + (stops[index + 1][channel] - start) * local));
  return `rgb(${color[0]}, ${color[1]}, ${color[2]})`;
}

function clampGraphValue(value, max) {
  if (!Number.isFinite(value) || value < 0) {
    return 0;
  }
  return Math.min(value, max);
}

function renderProfileWarning(warning) {
  return `<div class="profile-warning ${escapeHTML(warning.severity || "warning")}">${escapeHTML(warning.message || warning.code || t("profile.warning"))}</div>`;
}

function bindProfileDeckSelect(selector, key, profile) {
  const input = elements.profileGraph.querySelector(selector);
  input?.addEventListener("change", () => {
    state.profileGraphDeck = state.profileGraphDeck || mergeProfileGraphDeck(profile, null);
    state.profileGraphDeck[key] = input.value;
    if (key === "scaleMode") {
      state.profileGraphScaleMode = input.value;
    }
    if (key === "timeView") {
      state.profileGraphViewMode = input.value;
    }
    persistProfileSettings();
    renderProfile(profile);
  });
}

function applyProfileGraphPreset(preset) {
  const deck = state.profileGraphDeck || {};
  switch (preset) {
    case "compare_groups":
      Object.assign(deck, { scopeType: "group", compareMode: "overlay", timeView: deck.timeView || "year" });
      break;
    case "compare_zones":
      Object.assign(deck, { scopeType: "zone", compareMode: "small_multiples", timeView: deck.timeView || "week" });
      break;
    case "schedule_similarity":
      Object.assign(deck, { scopeType: "schedule", compareMode: "similarity", timeView: "week", metricMode: "multiplier" });
      break;
    case "outliers":
      Object.assign(deck, { scopeType: "selection", compareMode: "outliers" });
      break;
    case "annual_contribution":
      Object.assign(deck, { scopeType: "dimension", compareMode: "ranking", timeView: "month", metricMode: "annual", scaleMode: "shared" });
      break;
    case "source_rules":
      Object.assign(deck, { compareMode: "single", timeView: "rules" });
      break;
    default:
      Object.assign(deck, { compareMode: "single", timeView: deck.timeView === "rules" ? "year" : deck.timeView || "year" });
      break;
  }
  state.profileGraphDeck = deck;
  state.profileSettings.metricMode = deck.metricMode || state.profileSettings.metricMode;
  state.profileSettings.timeView = deck.timeView || state.profileSettings.timeView;
  state.profileSettings.compareMode = deck.compareMode || state.profileSettings.compareMode;
  state.profileSettings.scaleMode = deck.scaleMode || state.profileSettings.scaleMode;
}

function toggleProfilePinnedSeries(seriesID) {
  if (!seriesID) {
    return;
  }
  const pinned = new Set(state.profilePinnedSeriesIds || []);
  if (pinned.has(seriesID)) {
    pinned.delete(seriesID);
  } else {
    pinned.add(seriesID);
  }
  state.profilePinnedSeriesIds = [...pinned];
  state.profileGraphDeck = { ...(state.profileGraphDeck || {}), pinnedSeriesIds: state.profilePinnedSeriesIds };
}

function focusProfileSeries(seriesID) {
  const series = (state.report?.profile?.graphDataset?.series || []).find((item) => item.id === seriesID);
  if (!series) {
    return;
  }
  if (series.zoneName) {
    selectProfileZone(series.zoneName);
  } else if (series.groupId) {
    state.activeProfileView = "profile";
    state.activeProfileGroupId = series.groupId;
  }
  selectProfileDimension(series.dimension);
  selectProfileScheduleHash(series.scheduleHash || "", false);
}

function selectProfileScheduleHash(hash, switchMode = true) {
  if (!hash) {
    return;
  }
  state.profileGraphDeck = state.profileGraphDeck || {};
  state.profileGraphDeck.selectedScheduleHashes = [hash];
  if (switchMode) {
    state.profileGraphDeck.scopeType = "schedule";
    state.profileGraphDeck.compareMode = "similarity";
  }
}

function selectProfileDimension(dimension) {
  if (!dimension) {
    return;
  }
  state.profileGraphDeck = state.profileGraphDeck || {};
  state.profileGraphDeck.selectedDimensions = [dimension];
}

function selectProfileMatrixCell(cell) {
  const itemIds = String(cell.dataset.profileItemIds || "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
  selectProfileCellData({
    zoneName: cell.dataset.profileZone || "",
    groupId: cell.dataset.profileGroupId || "",
    dimension: cell.dataset.profileDimension || "",
    scheduleHash: cell.dataset.profileScheduleHash || "",
    scheduleName: cell.dataset.profileScheduleName || "",
    value: Number(cell.dataset.profileValue) || 0,
    itemIds,
  });
}

function selectProfileCellData(selected) {
  state.profileSelectedCell = selected;
  if (selected.zoneName) {
    selectProfileZone(selected.zoneName);
  }
  state.profileGraphDeck = {
    ...(state.profileGraphDeck || {}),
    scopeType: "selection",
    compareMode: "single",
    selectedGroupIds: selected.groupId ? [selected.groupId] : state.profileGraphDeck?.selectedGroupIds || [],
    selectedZoneNames: selected.zoneName ? [selected.zoneName] : [],
    selectedScheduleHashes: selected.scheduleHash ? [selected.scheduleHash] : [],
    selectedDimensions: selected.dimension ? [selected.dimension] : state.profileGraphDeck?.selectedDimensions || [],
  };
}

function configureProfilePanelNavigation() {
  if (profileNavigationCleanup) {
    return;
  }
  profileNavigationCleanup = configureResultPanelNavigationHooks("profile", {
    canReveal: profileCanRevealSelection,
    reveal: revealProfileSelection,
    selectFromElement: selectProfileSemanticFromElement,
    findTarget: findProfileNavigationTarget,
    captureContext: captureProfileNavigationContext,
    restoreContext: restoreProfileNavigationContext,
    preferredSemanticOccurrence: preferredProfileSemanticOccurrence,
  });
}

function profileCanRevealSelection(selection, context) {
  if (!state.report?.profile) {
    return false;
  }
  const target = profileViewTargetForSelection(selection, context.navigation);
  if (!target) {
    return context.genericCanReveal(selection);
  }
  return Boolean(profileNavigationTargetData(target));
}

async function revealProfileSelection(selection, options, context) {
  const profile = state.report?.profile;
  if (!profile) {
    return false;
  }
  if (!lastProfileView) {
    renderProfile(profile);
  }
  const target = profileViewTargetForSelection(selection, context.navigation);
  const targetData = target ? profileNavigationTargetData(target) : null;
  if (!target || !targetData || !applyProfileNavigationTarget(targetData, selection)) {
    return context.genericReveal(selection, options);
  }
  profileNavigationRevealTarget = {
    targetId: target.targetId,
    targetKind: target.targetKind,
    zoneName: targetData.zoneName || "",
    groupId: targetData.currentGroup?.id || "",
    dimension: targetData.dimension || "",
  };
  renderProfile(profile);
  context.refreshSelectionStyles(selection, state.globalHover);
  const targetElement = findProfileNavigationTarget({ ...selection, viewTarget: target }, context);
  if (!targetElement) {
    return false;
  }
  focusProfileNavigationElement(targetElement, options);
  return true;
}

function selectProfileSemanticFromElement(element, context) {
  const selection = context.extractSelection(element);
  if (!selection) {
    return null;
  }
  const target = element?.closest?.("[data-choose-semantic-occurrence]");
  if (target?.dataset.chooseSemanticOccurrence === "true") {
    return {
      ...selection,
      occurrenceId: "",
      semanticPathHint: "",
      chooseOccurrence: true,
    };
  }
  return selection;
}

function findProfileNavigationTarget(selection, context) {
  const target = profileViewTargetForSelection(selection, context.navigation);
  if (!target) {
    return context.genericFindTarget(selection);
  }
  const root = context.root;
  if (!root) {
    return null;
  }
  if (target.targetKind === "profile-item") {
    const matrixCell = [...(elements.profileMatrix?.querySelectorAll("[data-profile-item-ids]") || [])]
      .find((cell) => String(cell.dataset.profileItemIds || "").split(",").includes(target.targetId));
    if (matrixCell) {
      return matrixCell;
    }
  }
  const containers = target.targetKind === "schedule"
    ? [elements.profileGraph, elements.profileDetail, elements.profileMatrix, elements.profileOverview]
    : target.targetKind === "profile-group" || target.targetKind === "zone"
      ? [elements.profileOverview, elements.profileMatrix, elements.profileGraph, elements.profileDetail]
      : [elements.profileMatrix, elements.profileGraph, elements.profileDetail, elements.profileOverview];
  for (const container of containers) {
    const match = panelTargetElement(container, target.targetId);
    if (match) {
      return match;
    }
  }
  return context.genericFindTarget({ ...selection, viewTarget: target });
}

function captureProfileNavigationContext(context) {
  return {
    ...context.genericCaptureContext(),
    activeProfileView: state.activeProfileView,
    activeProfileZoneName: state.activeProfileZoneName,
    activeProfileGroupId: state.activeProfileGroupId,
    profileSelectedCell: cloneProfileSelectedCell(state.profileSelectedCell),
    profilePinnedSeriesIds: [...(state.profilePinnedSeriesIds || [])],
    profileGraphDeck: cloneProfileGraphDeck(state.profileGraphDeck),
    profileFilter: String(elements.profileFilter?.value || ""),
    navigationRevealTarget: profileNavigationRevealTarget ? { ...profileNavigationRevealTarget } : null,
    overviewScrollTop: Number(elements.profileOverview?.scrollTop) || 0,
    graphScrollTop: Number(elements.profileGraph?.scrollTop) || 0,
    matrixScrollTop: Number(elements.profileMatrix?.scrollTop) || 0,
    detailScrollTop: Number(elements.profileDetail?.scrollTop) || 0,
  };
}

async function restoreProfileNavigationContext(snapshot = {}, context) {
  if (!state.report?.profile) {
    return false;
  }
  if (Object.prototype.hasOwnProperty.call(snapshot, "activeProfileView")) {
    state.activeProfileView = snapshot.activeProfileView === "zone" ? "zone" : "profile";
  }
  if (Object.prototype.hasOwnProperty.call(snapshot, "activeProfileZoneName")) {
    state.activeProfileZoneName = String(snapshot.activeProfileZoneName || "");
  }
  if (Object.prototype.hasOwnProperty.call(snapshot, "activeProfileGroupId")) {
    state.activeProfileGroupId = String(snapshot.activeProfileGroupId || "");
  }
  if (Object.prototype.hasOwnProperty.call(snapshot, "profileSelectedCell")) {
    state.profileSelectedCell = cloneProfileSelectedCell(snapshot.profileSelectedCell);
  }
  if (Object.prototype.hasOwnProperty.call(snapshot, "profilePinnedSeriesIds")) {
    state.profilePinnedSeriesIds = [...(snapshot.profilePinnedSeriesIds || [])];
  }
  if (snapshot.profileGraphDeck) {
    state.profileGraphDeck = cloneProfileGraphDeck(snapshot.profileGraphDeck);
    state.profileGraphDeck.pinnedSeriesIds = [...state.profilePinnedSeriesIds];
  }
  if (elements.profileFilter && Object.prototype.hasOwnProperty.call(snapshot, "profileFilter")) {
    elements.profileFilter.value = String(snapshot.profileFilter || "");
  }
  profileNavigationRevealTarget = snapshot.navigationRevealTarget ? { ...snapshot.navigationRevealTarget } : null;
  renderProfile(state.report.profile);
  context.refreshSelectionStyles(state.globalSelection, state.globalHover);
  await context.genericRestoreContext(snapshot);
  restoreElementScroll(elements.profileOverview, snapshot.overviewScrollTop);
  restoreElementScroll(elements.profileGraph, snapshot.graphScrollTop);
  restoreElementScroll(elements.profileMatrix, snapshot.matrixScrollTop);
  restoreElementScroll(elements.profileDetail, snapshot.detailScrollTop);
  return true;
}

function preferredProfileSemanticOccurrence(selection, context) {
  const navigation = context.navigation;
  const requested = (navigation.occurrences || []).find((occurrence) => occurrence.occurrenceId === selection?.occurrenceId);
  if (requested && (!selection.entityId || requested.entityId === selection.entityId)) {
    return requested.occurrenceId;
  }
  const target = profileViewTargetForSelection(selection, navigation);
  if (!target) {
    return context.genericPreferredSemanticOccurrence(selection);
  }
  const occurrences = profileOccurrencesForTarget(target.targetId, navigation)
    .filter((occurrence) => !selection.entityId || occurrence.entityId === selection.entityId);
  if (!occurrences.length) {
    return "";
  }
  if (target.targetKind === "schedule" && occurrences.length > 1) {
    return "";
  }
  if (occurrences.length === 1) {
    return occurrences[0].occurrenceId;
  }
  const dimension = state.profileSelectedCell?.dimension || profileNavigationRevealTarget?.dimension || "";
  const contextual = occurrences.filter((occurrence) => (
    occurrence.contextKind === "zone_profile" &&
    semanticProfilePathMatches(occurrence.path, state.activeProfileZoneName, dimension)
  ));
  return contextual.length === 1 ? contextual[0].occurrenceId : "";
}

function profileViewTargetForSelection(selection = {}, navigation = profileNavigationIndex()) {
  if (String(selection.viewTarget?.view || "").toLowerCase() === "profile" && selection.viewTarget?.targetId) {
    return normalizeProfileViewTarget(selection.viewTarget);
  }
  const requestedTargetID = String(selection.originTargetId || "");
  const occurrence = (navigation.occurrences || []).find((candidate) => candidate.occurrenceId === selection.occurrenceId);
  const entity = (navigation.entities || []).find((candidate) => candidate.id === selection.entityId);
  const targets = [...(occurrence?.viewTargets || []), ...(entity?.viewTargets || [])]
    .filter((target) => String(target?.view || "").toLowerCase() === "profile" && target?.targetId);
  if (requestedTargetID) {
    const requested = targets.find((target) => target.targetId === requestedTargetID);
    if (requested) {
      return normalizeProfileViewTarget(requested);
    }
  }
  return targets.length ? normalizeProfileViewTarget(targets[0]) : null;
}

function normalizeProfileViewTarget(target = {}) {
  return {
    view: "profile",
    targetKind: String(target.targetKind || ""),
    targetId: String(target.targetId || ""),
    label: String(target.label || ""),
  };
}

function profileNavigationTargetData(target) {
  const profile = state.report?.profile;
  if (!profile || !target?.targetId) {
    return null;
  }
  switch (target.targetKind) {
    case "profile-item": {
      const item = profileItemByID(target.targetId, profile);
      return item ? { item, zoneName: item.zoneName, dimension: item.dimension } : null;
    }
    case "zone-dimension": {
      const match = profileZoneDimensionForTarget(target.targetId);
      return match ? { ...match } : null;
    }
    case "profile-group": {
      const reportGroup = (profile.groups || []).find((group) => group.id === target.targetId);
      const currentGroup = reportGroup ? currentProfileGroupForReportGroup(reportGroup) : null;
      return reportGroup ? { reportGroup, currentGroup, zoneName: reportGroup.zoneNames?.[0] || "" } : null;
    }
    case "schedule": {
      const schedules = profileSchedulesForTarget(target.targetId);
      const scheduleHashes = [...new Set(schedules.map((schedule) => schedule.contentHash).filter(Boolean))];
      const series = (profile.graphDataset?.series || []).find((candidate) => (
        profileScheduleTargetNames(candidate.scheduleName).some((name) => sameProfileName(name, target.targetId)) ||
        scheduleHashes.some((hash) => String(candidate.scheduleHash || "").split("+").includes(hash))
      ));
      const reportGroup = (profile.groups || []).find((group) => group.id === series?.groupId);
      const currentGroup = series?.zoneName
        ? groupForZoneName(series.zoneName)
        : reportGroup ? currentProfileGroupForReportGroup(reportGroup) : null;
      return schedules.length ? {
        schedules,
        scheduleHashes,
        currentGroup: currentGroup || selectedProfileGroup(),
        zoneName: series?.zoneName || currentGroup?.zoneNames?.[0] || "",
      } : null;
    }
    case "zone": {
      const zone = (profile.zoneProfiles || []).find((candidate) => sameProfileName(candidate.zoneName, target.targetId));
      return zone ? { zoneName: zone.zoneName } : null;
    }
    default:
      return null;
  }
}

function applyProfileNavigationTarget(targetData, selection = {}) {
  if (targetData.item) {
    selectProfileItemForNavigation(targetData.item);
    return true;
  }
  if (targetData.dimension && targetData.zoneName) {
    selectProfileZoneDimensionForNavigation(targetData.zoneName, targetData.dimension);
    return true;
  }
  if (targetData.reportGroup) {
    const currentGroup = targetData.currentGroup || currentProfileGroupForReportGroup(targetData.reportGroup);
    if (!currentGroup) {
      return false;
    }
    state.activeProfileView = "profile";
    state.activeProfileGroupId = currentGroup.id;
    if (!currentGroup.zoneNames.includes(state.activeProfileZoneName)) {
      state.activeProfileZoneName = currentGroup.zoneNames[0] || "";
    }
    state.profileSelectedCell = null;
    state.profileGraphDeck = {
      ...(state.profileGraphDeck || {}),
      scopeType: "group",
      compareMode: "single",
      selectedGroupIds: [targetData.reportGroup.id],
    };
    return true;
  }
  if (targetData.schedules) {
    const exact = profileScheduleForSelection(targetData.schedules, selection);
    const hashes = exact?.contentHash ? [exact.contentHash] : targetData.scheduleHashes;
    state.profileSelectedCell = null;
    state.profileGraphDeck = {
      ...(state.profileGraphDeck || {}),
      scopeType: "schedule",
      compareMode: "similarity",
      metricMode: "multiplier",
      selectedScheduleHashes: hashes,
    };
    if (targetData.currentGroup) {
      state.activeProfileGroupId = targetData.currentGroup.id;
    }
    if (targetData.zoneName) {
      state.activeProfileZoneName = targetData.zoneName;
    }
    return true;
  }
  if (targetData.zoneName) {
    selectProfileZone(targetData.zoneName);
    state.profileSelectedCell = null;
    state.profileGraphDeck = {
      ...(state.profileGraphDeck || {}),
      scopeType: "zone",
      selectedZoneNames: [targetData.zoneName],
    };
    return true;
  }
  return false;
}

function selectProfileItemForNavigation(item) {
  const row = lastProfileView?.matrix.find((candidate) => sameProfileName(candidate.zoneName, item.zoneName));
  const summary = row?.dimensions.find((candidate) => candidate.dimension === item.dimension);
  selectProfileCellData({
    zoneName: item.zoneName,
    groupId: row?.groupId || groupForZoneName(item.zoneName)?.id || "",
    dimension: item.dimension,
    scheduleHash: item.scheduleHash || summary?.scheduleHash || "",
    scheduleName: item.scheduleName || summary?.scheduleName || "",
    value: Number(summary?.value) || 0,
    itemIds: summary?.itemIds || [item.id],
  });
  pinProfileSeries({ itemID: item.id, zoneName: item.zoneName, dimension: item.dimension });
}

function selectProfileZoneDimensionForNavigation(zoneName, dimension) {
  const row = lastProfileView?.matrix.find((candidate) => sameProfileName(candidate.zoneName, zoneName));
  const profile = state.report?.profile;
  const summary = row?.dimensions.find((candidate) => candidate.dimension === dimension) ||
    profileDimensionSummary(profile, zoneName, dimension);
  selectProfileCellData({
    zoneName: row?.zoneName || zoneName,
    groupId: row?.groupId || groupForZoneName(zoneName)?.id || "",
    dimension,
    scheduleHash: summary?.scheduleHash || "",
    scheduleName: summary?.scheduleName || "",
    value: Number(summary?.value) || 0,
    itemIds: summary?.itemIds || [],
  });
  pinProfileSeries({ zoneName, dimension });
}

function pinProfileSeries(criteria = {}) {
  const series = (state.report?.profile?.graphDataset?.series || []).find((candidate) => (
    (!criteria.itemID || (candidate.sourceItemIds || []).includes(criteria.itemID)) &&
    (!criteria.zoneName || sameProfileName(candidate.zoneName, criteria.zoneName)) &&
    (!criteria.dimension || candidate.dimension === criteria.dimension)
  ));
  if (!series) {
    return;
  }
  state.profilePinnedSeriesIds = [...new Set([...(state.profilePinnedSeriesIds || []), series.id])];
  state.profileGraphDeck = {
    ...(state.profileGraphDeck || {}),
    pinnedSeriesIds: [...state.profilePinnedSeriesIds],
  };
}

function profileZoneDimensionForTarget(targetID) {
  const prefix = "profile-zone-dimension:";
  if (!String(targetID).startsWith(prefix)) {
    return null;
  }
  const [zoneToken = "", dimensionToken = ""] = String(targetID).slice(prefix.length).split(":");
  const rows = [...(lastProfileView?.matrix || []), ...(state.report?.profile?.zoneProfiles || [])];
  const row = rows.find((candidate) => (
    profileSemanticToken(candidate.zoneName) === zoneToken &&
    candidate.dimensions.some((dimension) => profileSemanticToken(dimension.dimension) === dimensionToken)
  ));
  const dimension = row?.dimensions.find((candidate) => profileSemanticToken(candidate.dimension) === dimensionToken)?.dimension;
  return row && dimension ? { zoneName: row.zoneName, dimension } : null;
}

function profileDimensionSummary(profile, zoneName, dimensionID) {
  const zone = (profile?.zoneProfiles || []).find((candidate) => sameProfileName(candidate.zoneName, zoneName));
  const dimension = (profile?.dimensions || []).find((candidate) => candidate.id === dimensionID);
  const metricID = state.profileSettings?.displayMetrics?.[dimensionID];
  return zone && dimension ? summarizeDimension(zone, dimension, metricID) : null;
}

function profileItemByID(itemID, profile = state.report?.profile) {
  for (const zone of profile?.zoneProfiles || []) {
    const item = (zone.items || []).find((candidate) => candidate.id === itemID);
    if (item) {
      return item;
    }
  }
  return null;
}

function currentProfileGroupForReportGroup(reportGroup) {
  return lastProfileView?.groups.find((group) => (
    sameStringSet(group.zoneNames, reportGroup.zoneNames) && sameStringSet(group.itemIds, reportGroup.itemIds)
  )) || null;
}

function profileSchedulesForTarget(targetID) {
  const target = String(targetID || "");
  return (state.report?.profile?.schedules || []).filter((schedule) => sameProfileName(schedule.scheduleName, target));
}

function profileScheduleForSelection(schedules, selection = {}) {
  const objectIndex = selection.sourceAnchor?.objectIndex;
  return schedules.find((schedule) => objectIndex !== undefined && objectIndex !== null && schedule.objectIndex === Number(objectIndex)) || schedules[0] || null;
}

function profileOccurrencesForTarget(targetID, navigation = profileNavigationIndex()) {
  const occurrenceByID = new Map((navigation.occurrences || []).map((occurrence) => [occurrence.occurrenceId, occurrence]));
  return (navigation.byViewTarget?.[`profile|${targetID}`] || [])
    .map((occurrenceID) => occurrenceByID.get(occurrenceID))
    .filter(Boolean);
}

function semanticProfilePathMatches(path, zoneName, dimension = "") {
  const normalizedPath = String(path || "").toLowerCase();
  const zone = String(zoneName || "").toLowerCase();
  const profilePath = zone && normalizedPath.includes(`zones/${zone}/profiles/`);
  return Boolean(profilePath && (!dimension || normalizedPath.includes(`/${String(dimension).toLowerCase()}`)));
}

function panelTargetElement(container, targetID) {
  return [...(container?.querySelectorAll("[data-panel-target-id]") || [])]
    .find((element) => element.dataset.panelTargetId === targetID) || null;
}

function focusProfileNavigationElement(element, options = {}) {
  let details = element.closest?.("details") || null;
  while (details) {
    details.open = true;
    details = details.parentElement?.closest?.("details") || null;
  }
  element.classList.add("semantic-selected");
  element.toggleAttribute("data-semantic-selected", true);
  if (options.scroll !== false) {
    element.scrollIntoView?.({ block: "nearest", inline: "nearest", behavior: options.behavior || "auto" });
  }
  if (options.focus !== false) {
    if (!element.matches("a[href], button, input, select, textarea, [tabindex]")) {
      element.tabIndex = -1;
    }
    element.focus?.({ preventScroll: true });
  }
}

function cloneProfileSelectedCell(cell) {
  return cell ? { ...cell, itemIds: [...(cell.itemIds || [])] } : null;
}

function cloneProfileGraphDeck(deck) {
  if (!deck) {
    return null;
  }
  return {
    ...deck,
    selectedGroupIds: [...(deck.selectedGroupIds || [])],
    selectedZoneNames: [...(deck.selectedZoneNames || [])],
    selectedScheduleHashes: [...(deck.selectedScheduleHashes || [])],
    selectedDimensions: [...(deck.selectedDimensions || [])],
    timeRange: [...(deck.timeRange || [])],
    pinnedSeriesIds: [...(deck.pinnedSeriesIds || [])],
  };
}

function restoreElementScroll(element, value) {
  if (element) {
    element.scrollTop = Number(value) || 0;
  }
}

function sameProfileName(left, right) {
  return String(left || "").trim().toLowerCase() === String(right || "").trim().toLowerCase();
}

function bindProfileControls(profile) {
  elements.profileSettings.querySelectorAll("[data-profile-view]").forEach((button) => {
    button.addEventListener("click", () => {
      state.activeProfileView = button.dataset.profileView === "zone" ? "zone" : "profile";
      renderProfile(profile);
    });
  });
  elements.profileOverview.querySelectorAll("[data-profile-group-id]").forEach((button) => {
    button.addEventListener("click", () => {
      state.activeProfileView = "profile";
      state.activeProfileGroupId = button.dataset.profileGroupId || "";
      const group = selectedProfileGroup();
      state.activeProfileZoneName = group?.zoneNames?.[0] || state.activeProfileZoneName;
      renderProfile(profile);
    });
  });
  elements.profileOverview.querySelectorAll("[data-profile-zone]").forEach((button) => {
    button.addEventListener("click", () => {
      selectProfileZone(button.dataset.profileZone || "");
      renderProfile(profile);
    });
  });
  elements.profileSettings.querySelectorAll("[data-profile-dimension]").forEach((input) => {
    input.addEventListener("change", () => {
      const dimension = input.dataset.profileDimension;
      const enabled = new Set(state.profileSettings.enabledDimensions);
      if (input.checked) {
        enabled.add(dimension);
      } else {
        enabled.delete(dimension);
      }
      state.profileSettings.enabledDimensions = [...enabled];
      persistProfileSettings();
      renderProfile(profile);
    });
  });
  bindSettingControl("#profileMetricMode", (input) => {
    state.profileSettings.metricMode = input.value;
    state.profileSettings.graphMode = input.value === "multiplier" ? "multiplier" : "actual_value";
    state.profileGraphDeck.metricMode = input.value;
  });
  bindProfileDeckSelect("#profileGraphScopeType", "scopeType", profile);
  bindProfileDeckSelect("#profileGraphTimeView", "timeView", profile);
  bindProfileDeckSelect("#profileGraphCompareMode", "compareMode", profile);
  bindProfileDeckSelect("#profileGraphScaleMode", "scaleMode", profile);
  elements.profileGraph.querySelectorAll("[data-profile-graph-preset]").forEach((button) => {
    button.addEventListener("click", () => {
      applyProfileGraphPreset(button.dataset.profileGraphPreset || "time_profile");
      persistProfileSettings();
      renderProfile(profile);
    });
  });
  elements.profileGraph.querySelectorAll("[data-profile-series-id]").forEach((element) => {
    const activate = (event) => {
      if (event.target.closest?.("[data-profile-pin-series]")) {
        return;
      }
      focusProfileSeries(element.dataset.profileSeriesId || "");
      renderProfile(profile);
    };
    element.addEventListener("click", activate);
    element.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" && event.key !== " ") {
        return;
      }
      event.preventDefault();
      activate(event);
    });
  });
  elements.profileGraph.querySelectorAll("[data-profile-pin-series]").forEach((button) => {
    button.addEventListener("click", (event) => {
      event.stopPropagation();
      toggleProfilePinnedSeries(button.dataset.profilePinSeries || "");
      renderProfile(profile);
    });
  });
  elements.profileGraph.querySelectorAll("[data-profile-series-focus]").forEach((button) => {
    button.addEventListener("click", () => {
      focusProfileSeries(button.dataset.profileSeriesFocus || "");
      renderProfile(profile);
    });
  });
  elements.profileGraph.querySelectorAll("[data-profile-schedule-hash]").forEach((button) => {
    button.addEventListener("click", () => {
      selectProfileScheduleHash(button.dataset.profileScheduleHash || "");
      renderProfile(profile);
    });
  });
  elements.profileGraph.querySelectorAll("[data-profile-outlier-zone]").forEach((button) => {
    button.addEventListener("click", () => {
      if (button.dataset.profileOutlierZone) {
        selectProfileZone(button.dataset.profileOutlierZone);
      }
      selectProfileDimension(button.dataset.profileDimension || "");
      selectProfileScheduleHash(button.dataset.profileScheduleHash || "", false);
      renderProfile(profile);
    });
  });
  elements.profileGraph.querySelectorAll("[data-profile-candidate-id]").forEach((button) => {
    button.addEventListener("click", () => {
      selectProfileDimension(button.dataset.profileDimension || "");
      state.profileGraphDeck.compareMode = "outliers";
      renderProfile(profile);
    });
  });
  elements.profileDetail.querySelectorAll("[data-profile-candidate-id]").forEach((button) => {
    button.addEventListener("click", () => {
      selectProfileDimension(button.dataset.profileDimension || "");
      state.profileGraphDeck.compareMode = "outliers";
      renderProfile(profile);
    });
  });
  elements.profileMatrix.querySelectorAll("[data-profile-cell]").forEach((cell) => {
    cell.addEventListener("click", (event) => {
      if (event.target.closest(".profile-object-link")) {
        return;
      }
      selectProfileMatrixCell(cell);
      renderProfile(profile);
    });
    cell.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" && event.key !== " ") {
        return;
      }
      event.preventDefault();
      selectProfileMatrixCell(cell);
      renderProfile(profile);
    });
  });
  elements.profileGraph.querySelectorAll("[data-profile-zone-ref]").forEach((button) => {
    button.addEventListener("click", () => {
      selectProfileZone(button.dataset.profileZoneRef || "");
      renderProfile(profile);
    });
  });
  elements.profileMatrix.querySelectorAll("tr[data-profile-zone]").forEach((row) => {
    row.addEventListener("click", (event) => {
      if (event.target.closest(".profile-object-link")) {
        return;
      }
      selectProfileZone(row.dataset.profileZone || "");
      renderProfile(profile);
    });
  });
}

function bindSettingControl(selector, update) {
  const input = elements.profileSettings.querySelector(selector);
  input?.addEventListener("change", () => {
    update(input);
    persistProfileSettings();
    renderProfile();
  });
}

export function initializeProfileControls() {
  configureProfilePanelNavigation();
  window.addEventListener("idfAnalyzer:semanticSelectionChanged", (event) => {
    if (!event.detail?.selection?.entityId) {
      profileNavigationRevealTarget = null;
    }
  });
  elements.profileOverview?.closest("#profilePane")?.addEventListener("click", (event) => {
    const targetID = event.target.closest?.("[data-panel-target-id]")?.dataset.panelTargetId || "";
    if (!profileNavigationRevealTarget || targetID !== profileNavigationRevealTarget.targetId) {
      profileNavigationRevealTarget = null;
    }
  }, { capture: true });
  elements.profileFilter?.addEventListener("input", () => {
    profileNavigationRevealTarget = null;
    renderProfile();
  });
  elements.profileApplyButton?.addEventListener("click", openProfileApplyDialog);
  elements.profileApplyClose?.addEventListener("click", closeProfileApplyDialog);
  elements.profilePreviewApply?.addEventListener("click", previewProfileApply);
  elements.profileApplyForm?.addEventListener("submit", applyProfile);
  elements.profileApplyBody?.addEventListener("change", () => {
    if (elements.profileApplyDialog?.classList.contains("hidden")) {
      return;
    }
    state.profileApplyPreview = null;
    elements.profileConfirmApply.disabled = true;
    elements.profileApplyStatus.textContent = t("status.runPreview");
  });
}

function openProfileApplyDialog() {
  const group = selectedProfileGroup();
  const profile = state.report?.profile;
  if (!group || !profile) {
    return;
  }
  state.profileApplyPreview = null;
  elements.profileConfirmApply.disabled = true;
  elements.profileApplyStatus.textContent = t("status.reviewBeforeApplying");
  const sourceZones = new Set(group.zoneNames);
  const targets = (profile.zoneProfiles || []).filter((zone) => !sourceZones.has(zone.zoneName));
  const dimensions = (profile.dimensions || []).filter((dimension) => state.profileSettings.enabledDimensions.includes(dimension.id));
  elements.profileApplyBody.innerHTML = `
    <section>
      <h4>${escapeHTML(group.name)}</h4>
      <p>${escapeHTML(group.zoneNames.join(", "))}</p>
    </section>
    <section>
      <h4>${t("common.targetZones")}</h4>
      <div class="profile-target-list">
        ${targets
          .map(
            (zone) => `
              <label class="profile-check">
                <input data-profile-target-zone="${escapeHTML(zone.zoneName)}" type="checkbox" />
                <span>${escapeHTML(zone.zoneName)}</span>
              </label>`,
          )
          .join("") || `<div class="empty">${t("profile.noOtherZones")}</div>`}
      </div>
    </section>
    <section>
      <h4>${t("common.dimensions")}</h4>
      <div class="profile-chip-grid">
        ${dimensions
          .map(
            (dimension) => `
              <label class="profile-check">
                <input data-profile-apply-dimension="${escapeHTML(dimension.id)}" type="checkbox" checked />
                <span>${escapeHTML(profileDimensionLabel(dimension.id))}</span>
              </label>`,
          )
          .join("")}
      </div>
    </section>
    <section>
      <h4>${t("common.options")}</h4>
      <div class="profile-dialog-options">
        <label class="profile-field">
          <span>${t("common.applyMode")}</span>
          ${applyModeSelect("profileApplyModeDialog", state.profileSettings.applyBehavior.defaultMode)}
        </label>
        <label class="profile-field">
          <span>${t("common.existingTarget")}</span>
          ${replacePolicySelect("profileReplacePolicyDialog", state.profileSettings.applyBehavior.replaceExistingPolicy)}
        </label>
        <label class="profile-check"><input id="profileAllowZoneListEdit" type="checkbox" ${state.profileSettings.applyBehavior.allowZoneListEdit ? "checked" : ""} /> <span>${t("profile.allowSharedZoneList")}</span></label>
      </div>
    </section>
    <section>
      <h4>${t("common.preview")}</h4>
      <div id="profileApplyPreviewList" class="profile-apply-preview"><div class="empty">${t("status.runPreview")}</div></div>
    </section>`;
  elements.profileApplyDialog.classList.remove("hidden");
}

function closeProfileApplyDialog() {
  elements.profileApplyDialog.classList.add("hidden");
}

async function previewProfileApply() {
  const request = profileApplyRequest();
  if (!request.targetZoneNames.length) {
    elements.profileApplyStatus.textContent = t("status.selectTargetZone");
    return;
  }
  try {
    elements.profileApplyStatus.textContent = t("status.buildingPreview");
    const preview = await callProfileApplyAPI("PreviewProfileApplyText", "/api/profile-apply-preview", request);
    state.profileApplyPreview = preview;
    const canApply = preview.canApply ?? preview.CanApply;
    elements.profileConfirmApply.disabled = !canApply;
    renderApplyPreview(preview);
    elements.profileApplyStatus.textContent = canApply ? t("status.previewReady") : t("status.previewBlocking");
  } catch (error) {
    elements.profileApplyStatus.textContent = error?.message || String(error);
  }
}

async function applyProfile(event) {
  event.preventDefault();
  const request = profileApplyRequest();
  try {
    elements.profileApplyStatus.textContent = t("status.applyProfile");
    const result = await callProfileApplyAPI("ApplyProfileText", "/api/profile-apply", request);
    window.dispatchEvent(new CustomEvent("idfAnalyzer:profileApplied", { detail: result }));
    closeProfileApplyDialog();
  } catch (error) {
    elements.profileApplyStatus.textContent = error?.message || String(error);
  }
}

function profileApplyRequest() {
  const group = selectedProfileGroup();
  const profile = state.report?.profile;
  const itemMap = profileItemMap(profile);
  const selectedDimensions = [...elements.profileApplyBody.querySelectorAll("[data-profile-apply-dimension]:checked")].map((input) => input.dataset.profileApplyDimension);
  const dimensionSet = new Set(selectedDimensions.length ? selectedDimensions : state.profileSettings.enabledDimensions);
  const sourceObjectIndexes = [
    ...new Set(
      (group?.itemIds || [])
        .map((id) => itemMap.get(id))
        .filter((item) => item && item.cloneEligible !== false && dimensionSet.has(item.dimension))
        .map((item) => item.objectIndex)
        .filter((index) => index !== undefined),
    ),
  ];
  const targetZoneNames = [...elements.profileApplyBody.querySelectorAll("[data-profile-target-zone]:checked")].map((input) => input.dataset.profileTargetZone);
  return {
    sourceObjectIndexes,
    sourceZoneNames: group?.zoneNames || [],
    targetZoneNames,
    dimensions: [...dimensionSet],
    mode: elements.profileApplyBody.querySelector("#profileApplyModeDialog")?.value || state.profileSettings.applyBehavior.defaultMode,
    replaceExistingPolicy: elements.profileApplyBody.querySelector("#profileReplacePolicyDialog")?.value || state.profileSettings.applyBehavior.replaceExistingPolicy,
    nameSuffix: state.profileSettings.applyBehavior.nameSuffix,
    allowZoneListEdit: Boolean(elements.profileApplyBody.querySelector("#profileAllowZoneListEdit")?.checked),
  };
}

async function callProfileApplyAPI(methodName, endpoint, request) {
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
    throw new Error(`Profile apply request failed: ${response.status}`);
  }
  return response.json();
}

function renderApplyPreview(preview) {
  const list = elements.profileApplyBody.querySelector("#profileApplyPreviewList");
  const warnings = preview.warnings || [];
  const changes = preview.changes || [];
  list.innerHTML = `
    ${warnings.map(renderProfileWarning).join("")}
    ${changes.length ? changes.map((change) => `<div class="profile-apply-change"><strong>${escapeHTML(change.action)}</strong><span>${escapeHTML(change.message)}</span></div>`).join("") : `<div class="empty">${t("status.noChanges")}</div>`}`;
}

function cachedProfileView(profile, settings) {
  const cache = state.profileViewCache || new Map();
  state.profileViewCache = cache;
  const key = profileViewCacheKey(profile, settings);
  if (cache.has(key)) {
    const view = cache.get(key);
    cache.delete(key);
    cache.set(key, view);
    return view;
  }
  const view = buildProfileView(profile, settings);
  cache.set(key, view);
  while (cache.size > 6) {
    cache.delete(cache.keys().next().value);
  }
  return view;
}

function profileViewCacheKey(profile, settings) {
  return [
    state.analysisKey || state.lastAnalyzedKey || "",
    profile.itemCount || 0,
    (profile.zoneProfiles || []).length,
    (settings?.enabledDimensions || []).join(","),
    settings?.groupBy || "",
    settings?.metricMode || "",
    settings?.scheduleCompareMode || "",
    settings?.numericTolerance || "",
  ].join("|");
}

function buildProfileView(profile, settings) {
  const dimensions = profile.dimensions || [];
  const matrix = (profile.zoneProfiles || []).map((zone) => ({
    zoneName: zone.zoneName,
    zoneObjectIndex: zone.zoneObjectIndex,
    dimensions: summarizeZoneDimensions(zone, settings, settings.displayMetrics),
    warnings: zone.warnings || [],
  }));
  const groups = buildProfileGroups(profile.zoneProfiles || [], settings);
  const groupByZone = new Map();
  groups.forEach((group) => {
    group.zoneNames.forEach((zoneName) => groupByZone.set(zoneName, group));
  });
  matrix.forEach((row) => {
    const group = groupByZone.get(row.zoneName);
    row.groupId = group?.id || "";
    row.groupName = group?.name || "";
  });
  return { dimensions, matrix, groups, groupByZone };
}

function buildProfileGroups(zones, settings) {
  const map = new Map();
  zones.forEach((zone) => {
    const groupingDimensions = summarizeZoneDimensions(zone, settings, settings.groupingMetrics);
    const displayDimensions = summarizeZoneDimensions(zone, settings, settings.displayMetrics);
    const key = profileGroupKey(groupingDimensions, settings);
    if (!map.has(key)) {
      map.set(key, { id: "", key, name: "", zoneNames: [], zoneCount: 0, dimensions: displayDimensions, itemIds: [], warnings: [] });
    }
    const group = map.get(key);
    group.zoneNames.push(zone.zoneName);
    group.itemIds.push(...(zone.items || []).map((item) => item.id));
    group.warnings.push(...(zone.warnings || []));
  });
  return [...map.values()]
    .sort((a, b) => b.zoneNames.length - a.zoneNames.length || a.key.localeCompare(b.key))
    .map((group, index) => ({
      ...group,
      id: `profile-group-${index + 1}`,
      name: `Profile ${String.fromCharCode(65 + (index % 26))}`,
      zoneCount: group.zoneNames.length,
    }));
}

function summarizeZoneDimensions(zone, settings, metricMap) {
  return (state.report?.profile?.dimensions || [])
    .filter((dimension) => settings.enabledDimensions.includes(dimension.id))
    .map((dimension) => summarizeDimension(zone, dimension, metricMap[dimension.id]))
    .filter(Boolean);
}

function summarizeDimension(zone, dimension, metricId) {
  const items = (zone.items || []).filter((item) => item.dimension === dimension.id);
  if (!items.length) {
    return null;
  }
  let value = 0;
  let okCount = 0;
  const itemIds = [];
  const scheduleNames = new Set();
  const schedulePatterns = new Set();
  const scheduleHashes = new Set();
  const warnings = [];
  let label = metricId;
  let unit = "";
  items.forEach((item) => {
    const metric = (item.normalized || []).find((candidate) => candidate.id === metricId);
    if (metric && metric.status !== "missing") {
      value += Number(metric.value) || 0;
      okCount += 1;
      label = profileMetricLabel(dimension.id, metric.id, metric.label || label);
      unit = metric.unit || unit;
    }
    itemIds.push(item.id);
    if (item.scheduleName) scheduleNames.add(item.scheduleName);
    if (item.schedulePattern) schedulePatterns.add(item.schedulePattern);
    if (item.scheduleHash) scheduleHashes.add(item.scheduleHash);
    warnings.push(...(item.warnings || []));
  });
  const status = okCount === 0 ? "missing" : okCount < items.length ? "partial" : "ok";
  return {
    dimension: dimension.id,
    label: profileDimensionLabel(dimension.id),
    metricId,
    metricLabel: label,
    unit,
    value,
    displayValue: status === "missing" ? "N/A" : `${formatNumber(value)}${unit ? ` ${unit}` : ""}`,
    status,
    scheduleName: [...scheduleNames].join(" + "),
    schedulePattern: [...schedulePatterns].join(" + "),
    scheduleHash: [...scheduleHashes].join("+"),
    itemIds,
    itemCount: items.length,
    warnings,
  };
}

function profileGroupKey(dimensions, settings) {
  const tolerance = Number(settings.numericTolerance) || 0.001;
  return dimensions
    .map((dimension) => {
      const bucket = Math.round((Number(dimension.value) || 0) / tolerance) * tolerance;
      const schedule =
        settings.scheduleCompareMode === "none"
          ? ""
          : settings.scheduleCompareMode === "resolved"
            ? dimension.scheduleHash
            : dimension.scheduleName;
      return `${dimension.dimension}:${dimension.metricId}:${bucket.toFixed(6)}:${schedule}`;
    })
    .sort()
    .join("|");
}

function mergeProfileSettings(defaults = {}, saved = {}) {
  const source = saved || {};
  return {
    ...defaults,
    ...source,
    enabledDimensions: Array.isArray(source.enabledDimensions) ? source.enabledDimensions : defaults.enabledDimensions || [],
    displayMetrics: { ...(defaults.displayMetrics || {}), ...(source.displayMetrics || {}) },
    groupingMetrics: { ...(defaults.groupingMetrics || {}), ...(source.groupingMetrics || {}) },
    graphDeck: { ...(defaults.graphDeck || {}), ...(source.graphDeck || {}) },
    applyBehavior: { ...(defaults.applyBehavior || {}), ...(source.applyBehavior || {}) },
  };
}

function persistProfileSettings() {
  if (state.profileSettings && state.profileGraphDeck) {
    state.profileSettings.graphDeck = { ...state.profileGraphDeck };
    state.profileSettings.metricMode = state.profileGraphDeck.metricMode || state.profileSettings.metricMode;
    state.profileSettings.timeView = state.profileGraphDeck.timeView || state.profileSettings.timeView;
    state.profileSettings.compareMode = state.profileGraphDeck.compareMode || state.profileSettings.compareMode;
    state.profileSettings.scaleMode = state.profileGraphDeck.scaleMode || state.profileSettings.scaleMode;
  }
  const settings = getCurrentAppSettings();
  saveAppSettings({ ...settings, profile: state.profileSettings }).catch((error) => {
    setStatus(error?.message || String(error), "warn");
  });
}

function selectedProfileGroup() {
  return lastProfileView?.groups.find((group) => group.id === state.activeProfileGroupId) || lastProfileView?.groups[0] || null;
}

function selectedProfileZoneRow() {
  return lastProfileView?.matrix.find((row) => row.zoneName === state.activeProfileZoneName) || lastProfileView?.matrix[0] || null;
}

function groupForZoneName(zoneName) {
  return lastProfileView?.groupByZone?.get(zoneName) || lastProfileView?.groups.find((group) => group.zoneNames.includes(zoneName)) || null;
}

function selectProfileZone(zoneName) {
  const row = lastProfileView?.matrix.find((candidate) => candidate.zoneName === zoneName);
  if (!row) {
    return;
  }
  state.activeProfileView = "zone";
  state.activeProfileZoneName = row.zoneName;
  state.activeProfileGroupId = row.groupId || groupForZoneName(row.zoneName)?.id || state.activeProfileGroupId;
}

function profileMatrixRowActive(row) {
  if (state.activeProfileView === "zone") {
    return row.zoneName === state.activeProfileZoneName;
  }
  return selectedProfileGroup()?.zoneNames.includes(row.zoneName);
}

function uniqueProfileItems(items) {
  const byID = new Map();
  items.forEach((item) => {
    if (item?.id && !byID.has(item.id)) {
      byID.set(item.id, item);
    }
  });
  return [...byID.values()];
}

function profileCandidatesForDimensions(profile, dimensions) {
  const wanted = new Set(dimensions || []);
  return (profile?.parameterCandidates || profile?.graphDataset?.parameterCandidates || [])
    .filter((candidate) => wanted.has(candidate.dimension))
    .slice(0, 6);
}

function profileItemMap(profile) {
  const map = new Map();
  (profile?.zoneProfiles || []).forEach((zone) => {
    (zone.items || []).forEach((item) => map.set(item.id, item));
  });
  return map;
}

function profileQuery() {
  return String(elements.profileFilter?.value || "").trim().toLowerCase();
}

function profileRevealMatchesGroup(group) {
  if (!profileNavigationRevealTarget) {
    return false;
  }
  return Boolean(
    (profileNavigationRevealTarget.groupId && group.id === profileNavigationRevealTarget.groupId) ||
    (profileNavigationRevealTarget.zoneName && group.zoneNames.includes(profileNavigationRevealTarget.zoneName)),
  );
}

function profileRevealMatchesRow(row) {
  return Boolean(
    profileNavigationRevealTarget?.zoneName &&
    sameProfileName(row.zoneName, profileNavigationRevealTarget.zoneName),
  );
}

function profileGroupMatchesQuery(group, query) {
  if (!query) {
    return true;
  }
  return [group.name, group.zoneNames.join(" "), ...group.dimensions.flatMap((dimension) => [dimension.label, dimension.displayValue, dimension.scheduleName, dimension.schedulePattern])]
    .join(" ")
    .toLowerCase()
    .includes(query);
}

function profileMatrixRowMatchesQuery(row, query) {
  if (!query) {
    return true;
  }
  return [row.zoneName, ...row.dimensions.flatMap((dimension) => [dimension.label, dimension.displayValue, dimension.scheduleName, dimension.schedulePattern])]
    .join(" ")
    .toLowerCase()
    .includes(query);
}

function profileDimensionLabel(dimension) {
  const fallback = state.report?.profile?.dimensions?.find((item) => item.id === dimension)?.label || dimension;
  return i18nProfileDimensionLabel(dimension, fallback);
}

function shortObjectType(value) {
  return String(value || "")
    .replace(/^Zone/i, "Z")
    .replace(/^DesignSpecification:/i, "DS:")
    .replace(/^ElectricEquipment$/i, "Elec")
    .replace(/^GasEquipment$/i, "Gas")
    .replace(/^OtherEquipment$/i, "Other")
    .replace(/^ZoneInfiltration:/i, "Inf:")
    .replace(/^ZoneVentilation:/i, "Vent:");
}

function formatNumber(value) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return "N/A";
  }
  return number.toLocaleString(undefined, { maximumFractionDigits: Math.abs(number) < 1 ? 4 : 2 });
}

function formatGraphNumber(value, unit) {
  return `${formatNumber(value)}${unit ? ` ${unit}` : ""}`;
}

function formatAxisTick(value, unit = "") {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return "0";
  }
  const abs = Math.abs(number);
  const compact = abs >= 1000
    ? number.toLocaleString(undefined, { maximumFractionDigits: 0 })
    : abs >= 10
      ? number.toLocaleString(undefined, { maximumFractionDigits: 1 })
      : number.toLocaleString(undefined, { maximumFractionDigits: 3 });
  const shortUnit = String(unit || "").replace("people/", "p/").replace("person", "p");
  return shortUnit ? `${compact} ${shortUnit}` : compact;
}

function graphXLabel(length) {
  if (length >= 8760) {
    return "8760h";
  }
  if (length >= 168) {
    return "7d";
  }
  if (length === 12) {
    return "12m";
  }
  return "24h";
}
