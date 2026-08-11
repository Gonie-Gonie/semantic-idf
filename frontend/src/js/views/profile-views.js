import { backend, elements, escapeHTML, setStatus, state } from "../state.js";
import { getCurrentAppSettings, saveAppSettings } from "../settings-client.js";
import { profileDimensionLabel as i18nProfileDimensionLabel, profileMetricLabel, t } from "../i18n.js";
import { configureResultPanelNavigationHooks } from "../panel-navigation-adapters.js";
import { getPanelNavigationAdapter } from "../panel-navigation-registry.js";
import { getSemanticNavigationCache } from "../semantic-navigation-cache.js";
import { clearSemanticSelection, selectSemanticEntity } from "../selection-controller.js";
import { captureViewSnapshot, recordViewHistory } from "../view-history.js";

let lastProfileView = null;
let lastProfileReport = null;
let profileNavigationCleanup = null;
let profileNavigationRevealTarget = null;
let lastProfileSeriesByID = new Map();
let profileSelectionAnalysisKey = "";
let previousProfileGroupMembership = new Map();
let profileHeatmapSequence = 0;
const profileHeatmapPaintQueue = new Map();
const profileItemMapCache = new WeakMap();
const PROFILE_MATRIX_RENDER_LIMIT = 500;

export function renderProfile(profile = state.report?.profile) {
  if (!elements.profileGraph) {
    return;
  }
  if (!profile) {
    renderEmptyProfile();
    return;
  }

  resetProfileRowSelectionForDocument(profile);
  state.profileSettings = mergeProfileSettings(profile.defaultSettings, state.profileSettings || getCurrentAppSettings().profile);
  state.profileSelectedDimensions = normalizeProfileSelection(
    state.profileSelectedDimensions,
    (profile.dimensions || [])
      .map((dimension) => dimension.id)
      .filter((dimension) => state.profileSettings.enabledDimensions.includes(dimension)),
  );
  state.activeProfileView = state.activeProfileView === "zone" ? "zone" : "profile";
  lastProfileView = cachedProfileView(profile, state.profileSettings);

  const visibleGroups = lastProfileView.groups;
  const visibleRows = lastProfileView.matrix;
  if (!state.activeProfileZoneName || !lastProfileView.matrix.some((row) => row.zoneName === state.activeProfileZoneName)) {
    state.activeProfileZoneName = visibleRows[0]?.zoneName || lastProfileView.matrix[0]?.zoneName || "";
  }
  normalizeProfileRowSelections(visibleGroups, visibleRows);
  const selectedGroup = selectedProfileGroup();
  const selectedZone = state.activeProfileView === "zone" ? selectedProfileZoneRow() : null;
  const graphGroup = selectedZone ? groupForZoneName(selectedZone.zoneName) : selectedGroup;

  elements.profileApplyButton.disabled = !graphGroup;
  const itemMap = profileItemMap(profile);
  renderProfileSettings(profile);
  renderProfileOverview(visibleGroups, visibleRows);
  renderProfileGraph(graphGroup, profile);
  renderProfileMatrix(lastProfileView.matrix, profile, itemMap);
  renderProfileDetail(graphGroup, profile, selectedZone, itemMap);
}

function renderEmptyProfile() {
  elements.profileSettings.innerHTML = "";
  elements.profileOverview.innerHTML = `<div class="empty">${t("profile.noAnalysis")}</div>`;
  elements.profileDetail.innerHTML = `<div class="empty">${t("profile.noProfile")}</div>`;
  elements.profileMatrix.innerHTML = `<div class="empty">${t("profile.noMatrix")}</div>`;
  elements.profileMatrixStats.textContent = t("count.zones", { count: 0 });
  elements.profileGraph.innerHTML = `<div class="empty">${t("profile.noGraph")}</div>`;
  elements.profileApplyButton.disabled = true;
  state.profileSelectedGroupIds = [];
  state.profileSelectedZoneNames = [];
  state.profileSelectionAnchorKey = "";
  lastProfileReport = null;
  profileSelectionAnalysisKey = "";
  previousProfileGroupMembership = new Map();
}

function resetProfileRowSelectionForDocument(profile) {
  const analysisKey = String(state.reportAnalysisKey || state.analysisKey || state.lastAnalyzedKey || "");
  const changed = analysisKey
    ? profileSelectionAnalysisKey !== analysisKey
    : Boolean(lastProfileReport && lastProfileReport !== profile);
  if (changed) {
    state.activeProfileGroupId = "";
    state.activeProfileZoneName = "";
    state.profileSelectedGroupIds = [];
    state.profileSelectedZoneNames = [];
    state.profileSelectionAnchorKey = "";
    state.profileSelectedCell = null;
    previousProfileGroupMembership = new Map();
  }
  profileSelectionAnalysisKey = analysisKey;
  lastProfileReport = profile;
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

function profileSemanticNavigationCache(source = state.semanticProjection) {
  return getSemanticNavigationCache(source, {
    textHash: state.reportAnalysisKey || state.analysisKey || state.lastAnalyzedKey || "",
  });
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
  const cache = profileSemanticNavigationCache();
  const occurrences = cache.occurrencesForIDs(cache.occurrenceIDs("view-target", `profile|${targetID}`));
  const entityIDs = [...new Set(occurrences.map((occurrence) => occurrence.entityId).filter(Boolean))];
  if (entityIDs.length !== 1) {
    return null;
  }
  const entity = cache.entity(entityIDs[0]);
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
  if (series.scopeType === "group" && series.currentGroupId) {
    const currentGroup = lastProfileView?.groups.find((group) => group.id === series.currentGroupId);
    targets.push(...profileGroupSemanticTargets(currentGroup));
  } else if (series.scopeType === "group" && series.groupId) {
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

function profileGroupRowKey(groupID) {
  return `group:${String(groupID || "")}`;
}

function profileZoneRowKey(zoneName) {
  return `zone:${String(zoneName || "")}`;
}

function normalizeProfileSelection(values, validValues) {
  const valid = new Set(validValues);
  return [...new Set((values || []).map((value) => String(value || "")).filter((value) => valid.has(value)))];
}

function currentProfileGroupIDForSeries(series = {}) {
  if (series.currentGroupId) {
    return series.currentGroupId;
  }
  const reportGroup = (state.report?.profile?.groups || []).find((group) => group.id === series.groupId);
  if (!reportGroup) {
    return series.groupId || "";
  }
  return currentProfileGroupForReportGroup(reportGroup)?.id || "";
}

function profileGroupMembership(group = {}) {
  return {
    zoneNames: [...new Set(group.zoneNames || [])],
    itemIds: [...new Set(group.itemIds || [])],
  };
}

function profileGroupsForMembership(membership, groups) {
  if (!membership?.zoneNames?.length) {
    return [];
  }
  const exact = groups.find((group) => (
    sameStringSet(group.zoneNames, membership.zoneNames) &&
    (!membership.itemIds?.length || sameStringSet(group.itemIds, membership.itemIds))
  ));
  if (exact) {
    return [exact];
  }
  const zones = new Set(membership.zoneNames);
  return groups.filter((group) => (group.zoneNames || []).some((zoneName) => zones.has(zoneName)));
}

function mapPreviousProfileGroupIDs(groupIDs, groups) {
  const mapped = [];
  (groupIDs || []).forEach((groupID) => {
    const membership = previousProfileGroupMembership.get(groupID);
    const matches = membership ? profileGroupsForMembership(membership, groups) : [];
    if (matches.length) {
      mapped.push(...matches.map((group) => group.id));
    } else if (!membership && groups.some((group) => group.id === groupID)) {
      mapped.push(groupID);
    }
  });
  return [...new Set(mapped)];
}

function normalizeProfileRowSelections(groups, rows) {
  const groupIDs = groups.map((group) => group.id);
  const zoneNames = rows.map((row) => row.zoneName);
  const previousPrimaryMembership = previousProfileGroupMembership.get(state.activeProfileGroupId);
  const previousAnchorID = String(state.profileSelectionAnchorKey || "").startsWith("group:")
    ? String(state.profileSelectionAnchorKey).slice("group:".length)
    : "";
  const previousAnchorMembership = previousProfileGroupMembership.get(previousAnchorID);
  let selectedGroupIDs = normalizeProfileSelection(mapPreviousProfileGroupIDs(state.profileSelectedGroupIds, groups), groupIDs);
  let selectedZoneNames = normalizeProfileSelection(state.profileSelectedZoneNames, zoneNames);
  if (!selectedGroupIDs.length && groupIDs.length) {
    selectedGroupIDs = [groupIDs.includes(state.activeProfileGroupId) ? state.activeProfileGroupId : groupIDs[0]];
  }
  if (!selectedZoneNames.length && zoneNames.length) {
    selectedZoneNames = [zoneNames.includes(state.activeProfileZoneName) ? state.activeProfileZoneName : zoneNames[0]];
  }
  state.profileSelectedGroupIds = selectedGroupIDs;
  state.profileSelectedZoneNames = selectedZoneNames;

  if (state.activeProfileView === "zone") {
    if (!selectedZoneNames.includes(state.activeProfileZoneName)) {
      state.activeProfileZoneName = selectedZoneNames[selectedZoneNames.length - 1] || zoneNames[0] || "";
    }
    state.activeProfileGroupId = groupForZoneName(state.activeProfileZoneName)?.id || state.activeProfileGroupId;
  } else {
    const mappedPrimary = profileGroupsForMembership(previousPrimaryMembership, groups)
      .find((group) => selectedGroupIDs.includes(group.id) && group.zoneNames.includes(state.activeProfileZoneName)) ||
      profileGroupsForMembership(previousPrimaryMembership, groups).find((group) => selectedGroupIDs.includes(group.id));
    if (mappedPrimary) {
      state.activeProfileGroupId = mappedPrimary.id;
    } else if (!selectedGroupIDs.includes(state.activeProfileGroupId)) {
      state.activeProfileGroupId = selectedGroupIDs[selectedGroupIDs.length - 1] || groupIDs[0] || "";
    }
  }

  const visibleKeys = state.activeProfileView === "zone"
    ? zoneNames.map(profileZoneRowKey)
    : groupIDs.map(profileGroupRowKey);
  if (state.activeProfileView === "profile" && previousAnchorMembership) {
    const anchorGroups = profileGroupsForMembership(previousAnchorMembership, groups)
      .filter((group) => selectedGroupIDs.includes(group.id));
    const mappedAnchor = anchorGroups.find((group) => group.zoneNames.includes(state.activeProfileZoneName)) || anchorGroups[0];
    state.profileSelectionAnchorKey = mappedAnchor ? profileGroupRowKey(mappedAnchor.id) : state.profileSelectionAnchorKey;
  }
  if (!visibleKeys.includes(state.profileSelectionAnchorKey)) {
    state.profileSelectionAnchorKey = state.activeProfileView === "zone"
      ? profileZoneRowKey(state.activeProfileZoneName)
      : profileGroupRowKey(state.activeProfileGroupId);
  }
  previousProfileGroupMembership = new Map(groups.map((group) => [group.id, profileGroupMembership(group)]));
}

function currentProfileRowKeys() {
  return state.activeProfileView === "zone"
    ? (lastProfileView?.matrix || []).map((row) => profileZoneRowKey(row.zoneName))
    : (lastProfileView?.groups || []).map((group) => profileGroupRowKey(group.id));
}

function selectedProfileRowKeys() {
  return state.activeProfileView === "zone"
    ? (state.profileSelectedZoneNames || []).map(profileZoneRowKey)
    : (state.profileSelectedGroupIds || []).map(profileGroupRowKey);
}

function profileGroupIDsForGraphSelection() {
  if (state.activeProfileView !== "zone") {
    return [...new Set(state.profileSelectedGroupIds || [])];
  }
  return [...new Set((state.profileSelectedZoneNames || [])
    .map((zoneName) => groupForZoneName(zoneName)?.id || "")
    .filter(Boolean))];
}

function applyProfileRowSelection(rowKeys, primaryKey, anchorKey) {
  const visibleKeys = currentProfileRowKeys();
  const requestedKeys = new Set(rowKeys);
  const selectedKeys = visibleKeys.filter((key) => requestedKeys.has(key));
  if (!selectedKeys.length || !selectedKeys.includes(primaryKey)) {
    return false;
  }
  if (primaryKey.startsWith("zone:")) {
    const primaryZone = primaryKey.slice("zone:".length);
    state.activeProfileView = "zone";
    state.profileSelectedZoneNames = selectedKeys.map((key) => key.slice("zone:".length));
    state.activeProfileZoneName = primaryZone;
    state.activeProfileGroupId = groupForZoneName(primaryZone)?.id || state.activeProfileGroupId;
  } else {
    const primaryGroupID = primaryKey.slice("group:".length);
    state.activeProfileView = "profile";
    state.profileSelectedGroupIds = selectedKeys.map((key) => key.slice("group:".length));
    state.activeProfileGroupId = primaryGroupID;
    const primaryGroup = selectedProfileGroup();
    if (primaryGroup && !primaryGroup.zoneNames.includes(state.activeProfileZoneName)) {
      state.activeProfileZoneName = primaryGroup.zoneNames[0] || state.activeProfileZoneName;
    }
  }
  state.profileSelectionAnchorKey = anchorKey;
  state.profileSelectedCell = null;
  return true;
}

function handleProfileRowSelection(event, rowKey) {
  const visibleKeys = currentProfileRowKeys();
  if (!visibleKeys.includes(rowKey)) {
    return false;
  }
  const additive = event.ctrlKey || event.metaKey;
  const range = event.shiftKey;
  const current = new Set(selectedProfileRowKeys());
  const currentPrimary = state.activeProfileView === "zone"
    ? profileZoneRowKey(state.activeProfileZoneName)
    : profileGroupRowKey(state.activeProfileGroupId);
  let next = [];
  let primary = rowKey;
  let anchor = state.profileSelectionAnchorKey;

  if (range && visibleKeys.includes(anchor)) {
    const start = visibleKeys.indexOf(anchor);
    const end = visibleKeys.indexOf(rowKey);
    const rangeKeys = visibleKeys.slice(Math.min(start, end), Math.max(start, end) + 1);
    if (additive) {
      const rangeKeySet = new Set(rangeKeys);
      next = visibleKeys.filter((key) => current.has(key) || rangeKeySet.has(key));
    } else {
      next = rangeKeys;
    }
  } else if (additive) {
    if (current.has(rowKey)) {
      if (current.size === 1) {
        return false;
      }
      current.delete(rowKey);
      next = visibleKeys.filter((key) => current.has(key));
      primary = next.includes(currentPrimary)
        ? currentPrimary
        : next[Math.max(0, Math.min(next.length - 1, visibleKeys.indexOf(rowKey) - 1))] || next[next.length - 1];
    } else {
      current.add(rowKey);
      next = visibleKeys.filter((key) => current.has(key));
    }
    anchor = rowKey;
  } else {
    next = [rowKey];
    anchor = rowKey;
  }
  return applyProfileRowSelection(next, primary, anchor);
}

function renderProfileOverview(groups, rows) {
  if (state.activeProfileView === "zone") {
    const selectedZoneNames = new Set(state.profileSelectedZoneNames || []);
    elements.profileOverview.innerHTML = rows.length
      ? rows.map((row) => renderProfileZoneCard(row, selectedZoneNames)).join("")
      : `<div class="empty">${t("profile.noMatchingZones")}</div>`;
    return;
  }
  const selectedGroupIDs = new Set(state.profileSelectedGroupIds || []);
  elements.profileOverview.innerHTML = groups.length
    ? groups.map((group) => renderProfileGroupCard(group, selectedGroupIDs)).join("")
    : `<div class="empty">${t("profile.noMatchingGroups")}</div>`;
}

function renderProfileGroupCard(group, selectedGroupIDs = new Set(state.profileSelectedGroupIds || [])) {
  const active = group.id === state.activeProfileGroupId ? "active" : "";
  const selected = selectedGroupIDs.has(group.id);
  return `
    <button class="profile-group-card profile-table-row navigable-row ${selected ? "selected" : ""} ${active}" data-profile-group-id="${escapeHTML(group.id)}" data-profile-row-key="${escapeHTML(profileGroupRowKey(group.id))}" type="button" role="option" aria-selected="${selected ? "true" : "false"}"
      ${profileSemanticAttributes(profileGroupSemanticTargets(group), { occurrenceContext: "zone_profile" })}>
      <span>
        <strong>${escapeHTML(group.name)}</strong>
        <small>${escapeHTML(t("count.zones", { count: group.zoneCount }))}</small>
      </span>
      <span class="profile-card-zones">${escapeHTML(group.zoneNames.slice(0, 4).join(", "))}${group.zoneNames.length > 4 ? "..." : ""}</span>
      <span class="profile-card-metrics">${group.dimensions.map((dimension) => `${escapeHTML(dimension.label)} ${escapeHTML(dimension.displayValue)}`).join(" / ")}</span>
      <span class="profile-row-apply-slot" aria-hidden="true"></span>
    </button>`;
}

function renderProfileZoneCard(row, selectedZoneNames = new Set(state.profileSelectedZoneNames || [])) {
  const active = row.zoneName === state.activeProfileZoneName ? "active" : "";
  const selected = selectedZoneNames.has(row.zoneName);
  return `
    <button class="profile-group-card profile-zone-card profile-table-row navigable-row ${selected ? "selected" : ""} ${active}" data-profile-zone="${escapeHTML(row.zoneName)}" data-profile-row-key="${escapeHTML(profileZoneRowKey(row.zoneName))}" type="button" role="option" aria-selected="${selected ? "true" : "false"}"
      ${profileSemanticAttributes([row.zoneName], { occurrenceContext: "zone_profile" })}>
      <span>
        <strong>${escapeHTML(row.zoneName)}</strong>
        <small>${escapeHTML(row.groupName || t("profile.noProfileGroup"))}</small>
      </span>
      <span class="profile-card-zones">${escapeHTML(t("profile.receivesProfile", { profile: row.groupName || t("profile.noProfileGroup") }))}</span>
      <span class="profile-card-metrics">${row.dimensions.map((dimension) => `${escapeHTML(dimension.label)} ${escapeHTML(dimension.displayValue)}`).join(" / ")}</span>
      <span class="profile-row-apply-slot" aria-hidden="true"></span>
    </button>`;
}

function renderProfileDetail(group, profile, zoneRow = null, itemMap = profileItemMap(profile)) {
  if (!group) {
    elements.profileDetail.innerHTML = `<div class="empty">${t("profile.noProfileGroup")}</div>`;
    return;
  }
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
      <div class="profile-detail-actions">
        <span class="badge">${escapeHTML(t("count.zones", { count: group.zoneCount }))}</span>
      </div>
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

function renderProfileMatrix(rows, profile, itemMap = profileItemMap(profile)) {
  const visibleRows = rows;
  const renderedRows = visibleRows.slice(0, PROFILE_MATRIX_RENDER_LIMIT);
  const hiddenRows = Math.max(0, visibleRows.length - renderedRows.length);
  const dimensions = (lastProfileView?.dimensions || []).filter((dimension) => (
    state.profileSettings.enabledDimensions.includes(dimension.id) ||
    dimension.id === profileNavigationRevealTarget?.dimension
  ));
  const activeZoneNames = profileActiveMatrixZoneNames();
  elements.profileMatrixStats.textContent = t("count.zones", { count: visibleRows.length });
  elements.profileMatrix.innerHTML = visibleRows.length
    ? `
      ${hiddenRows ? `<div class="empty compact">${escapeHTML(`${hiddenRows} additional zones hidden by the render limit.`)}</div>` : ""}
      <table>
        <thead>
          <tr><th>Zone</th>${dimensions.map((dimension) => `<th>${escapeHTML(profileDimensionLabel(dimension.id))}</th>`).join("")}</tr>
        </thead>
        <tbody>
          ${renderedRows
            .map((row) => {
              const dimensionByID = new Map((row.dimensions || []).map((item) => [item.dimension, item]));
              return `
                <tr class="${activeZoneNames.has(row.zoneName) ? "active" : ""}" data-profile-zone="${escapeHTML(row.zoneName)}"
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
                      const summary = dimensionByID.get(dimension.id) ||
                        temporaryProfileDimensionSummary(profile, row.zoneName, dimension.id);
                      return renderProfileMatrixCell(summary, itemMap, row, dimension);
                    })
                    .join("")}
                </tr>`;
            })
            .join("")}
        </tbody>
      </table>`
    : `<div class="empty">${t("profile.noMatchingZones")}</div>`;
}

function renderProfileMatrixCell(summary, itemMap, row, dimension = {}) {
  const dimensionLabel = profileDimensionLabel(dimension.id || summary?.dimension || "");
  if (!summary) {
    return `<td class="profile-matrix-empty" data-profile-dimension="${escapeHTML(dimension.id || "")}" data-profile-dimension-label="${escapeHTML(dimensionLabel)}" aria-label="${escapeHTML(`${dimensionLabel} N/A`)}"><span class="profile-matrix-dimension-label">${escapeHTML(dimensionLabel)}</span>N/A</td>`;
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
      data-profile-dimension-label="${escapeHTML(dimensionLabel)}"
      data-profile-schedule-hash="${escapeHTML(summary.scheduleHash || "")}"
      data-profile-schedule-name="${escapeHTML(summary.scheduleName || "")}"
      data-profile-value="${escapeHTML(String(summary.value ?? ""))}"
      data-profile-item-ids="${escapeHTML(itemIds)}"
      ${profileSemanticAttributes(semanticTargets, { occurrenceContext: "zone_profile" })}
      aria-label="${escapeHTML(`${row.zoneName} ${summary.label} ${summary.displayValue}`)}">
      <span class="profile-matrix-dimension-label">${escapeHTML(dimensionLabel)}</span>
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

function renderProfileGraph(group, profile) {
  if (!group) {
    lastProfileSeriesByID = new Map();
    elements.profileGraph.innerHTML = `<div class="empty">${t("profile.graphSelect")}</div>`;
    return;
  }
  const sourceDimensions = group.dimensions;
  const options = currentProfileGraphOptions();
  profileHeatmapPaintQueue.clear();
  const selectedSeries = profileGraphSeries(profile, group);
  lastProfileSeriesByID = new Map(selectedSeries.map((series) => [series.id, series]));
  const body = renderProfileGraphBody(selectedSeries, options);
  elements.profileGraph.innerHTML = `
    <fieldset class="profile-time-view-control">
      <legend>${escapeHTML(t("common.view"))}</legend>
      <div class="profile-graph-view-switch" role="group" aria-label="${escapeHTML(t("common.view"))}">
        ${[
          ["day", t("graph.representativeDay")],
          ["week", t("graph.representativeWeek")],
          ["month", t("graph.monthlyAverage")],
          ["year", t("graph.annualHeatmap")],
          ["duration", t("graph.loadDuration")],
          ["rules", t("graph.periodRules")],
        ].map(([value, label]) => `
          <button class="profile-time-view-button ${options.timeView === value ? "active" : ""}" type="button"
            data-profile-time-view="${escapeHTML(value)}" aria-pressed="${options.timeView === value ? "true" : "false"}">${escapeHTML(label)}</button>`).join("")}
      </div>
    </fieldset>
    ${renderProfileGraphSummary(group, sourceDimensions)}
    ${body}`;
  paintProfileHeatmaps();
}

function renderProfileGraphBody(series, options) {
  if (!series.length) {
    return `<div class="empty">${t("profile.graphNoValues")}</div>`;
  }
  return options.timeView === "year"
    ? renderProfileAnnualHeatmaps(series, options)
    : renderProfileOverlay(series, options);
}

function renderProfileOverlay(series, options) {
  return `<div class="profile-overlay-stack">${profileMetricSeriesGroups(series, options)
    .map((group) => {
      const metrics = group.items;
      const max = sharedProfileMetricMax(metrics, options);
      return `
        <section class="profile-overlay-panel" data-profile-dimension="${escapeHTML(group.dimension)}">
          <div class="profile-graph-panel-head">
            <strong>${escapeHTML(group.label)}</strong>
            <span>${escapeHTML(group.unit || t("graph.multiplier"))}</span>
          </div>
          ${renderOverlayGraph(metrics, max, group.unit)}
          ${renderProfileSeriesLegend(metrics)}
        </section>`;
    })
    .join("")}</div>`;
}

function renderProfileAnnualHeatmaps(series, options) {
  return `<div class="profile-annual-stack">${profileMetricSeriesGroups(series, options)
    .map((group) => {
      const max = sharedProfileMetricMax(group.items, options);
      return `
        <section class="profile-annual-panel" data-profile-dimension="${escapeHTML(group.dimension)}">
          <div class="profile-graph-panel-head">
            <strong>${escapeHTML(group.label)}</strong>
            <span>${escapeHTML(group.unit || t("graph.multiplier"))}</span>
          </div>
          <div class="profile-annual-heatmap-grid">
            ${group.items.map(({ series: item, metric, color }) => `
              <article class="profile-annual-heatmap-card navigable-row" data-profile-series-id="${escapeHTML(item.id)}"
                ${profileSemanticAttributes(profileSeriesSemanticTargets(item), { occurrenceContext: "zone_profile" })}>
                <strong><i style="background:${color}"></i>${escapeHTML(profileSeriesScopeLabel(item))}</strong>
                ${renderHeatmap(metric.values, max, group.label, group.unit)}
              </article>`).join("")}
          </div>
          ${renderProfileSeriesLegend(group.items)}
        </section>`;
    })
    .join("")}</div>`;
}

function profileMetricSeriesGroups(series, options) {
  const groups = new Map();
  const selectedGroupIDs = profileGroupIDsForGraphSelection();
  series.forEach((item, index) => {
    const metric = profileSeriesMetric(item, options);
    const key = `${item.dimension || "unknown"}\u0000${metric.unit || ""}`;
    if (!groups.has(key)) {
      groups.set(key, {
        dimension: item.dimension || "unknown",
        label: item.dimensionLabel || profileDimensionLabel(item.dimension),
        unit: metric.unit || "",
        items: [],
      });
    }
    groups.get(key).items.push({
      series: item,
      metric,
      color: profileSeriesSelectionColor(item, index, selectedGroupIDs),
    });
  });
  return [...groups.values()];
}

function sharedProfileMetricMax(items, options) {
  return Math.max(...items.map(({ series, metric }) => (
    graphScaleMaxForSeries(metric.values, series, {
      ...options,
      scaleMode: options.scaleMode === "shared" ? "auto" : options.scaleMode,
    })
  )), 1e-9);
}

function profileSeriesScopeLabel(series) {
  return series.groupName || series.label || t("common.selection", {}, "Selection");
}

function profileSeriesSelectionColor(series, fallbackIndex = 0, selection = profileGroupIDsForGraphSelection()) {
  const selectionKey = currentProfileGroupIDForSeries(series);
  const selectionIndex = selection.indexOf(selectionKey);
  return profileSeriesColor(selectionIndex >= 0 ? selectionIndex : fallbackIndex);
}

function renderProfileSeriesLegend(items) {
  return `<div class="profile-overlay-legend" aria-label="${escapeHTML(t("common.legend", {}, "Legend"))}">${items
    .map(({ series: item, color }) => `<span class="navigable-row" role="button" tabindex="0" data-profile-series-id="${escapeHTML(item.id)}"
      ${profileSemanticAttributes(profileSeriesSemanticTargets(item), { occurrenceContext: "zone_profile" })}><i style="background:${color}"></i>${escapeHTML(profileSeriesScopeLabel(item))}</span>`)
    .join("")}</div>`;
}

function renderOverlayGraph(items, max, unit = "") {
  const plot = { left: 30, right: 592, top: 12, bottom: 180 };
  const width = plot.right - plot.left;
  const height = plot.bottom - plot.top;
  const y = (value) => plot.bottom - (clampGraphValue(value, max) / max) * height;
  const paths = items
    .map(({ series, metric, color }, index) => {
      const data = metric.values.length > 420 ? downsampleValues(metric.values, 420) : metric.values;
      const path = data
        .map((value, valueIndex) => {
          const x = plot.left + (valueIndex / Math.max(data.length - 1, 1)) * width;
          return `${valueIndex === 0 ? "M" : "L"}${x.toFixed(2)},${y(value).toFixed(2)}`;
        })
        .join(" ");
      return `<path d="${path}" stroke="${color || profileSeriesColor(index)}" data-profile-series-id="${escapeHTML(series.id)}"
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

function renderProfileCandidateRow(candidate) {
  return `
    <button class="profile-qa-row candidate navigable-row ${escapeHTML(candidate.severity || "info")}" type="button" data-profile-candidate-id="${escapeHTML(candidate.id)}" data-profile-dimension="${escapeHTML(candidate.dimension || "")}"
      ${profileSemanticAttributes(profileAggregateSemanticTargets(candidate), { occurrenceContext: "zone_profile" })}>
      <strong>${escapeHTML(candidate.label || candidate.id)}</strong>
      <span>${escapeHTML(candidate.reason || "")}</span>
      <small>${escapeHTML(`${(candidate.zoneNames || []).length} zones · ${formatGraphNumber(candidate.currentMin, "")}..${formatGraphNumber(candidate.currentMax, "")}`)}</small>
    </button>`;
}

function renderProfileGraphSummary(group, dimensions) {
  return `
    <div class="profile-graph-summary">
      <div>
        <strong>${escapeHTML(group.name)}</strong>
        <span>${escapeHTML(t("profile.profileServesZones", { count: group.zoneCount }))}</span>
      </div>
      <div class="profile-connection-row">
        <span>${escapeHTML(t("profile.connectedZones"))}</span>
        <div>
          ${group.zoneNames
            .map(
              (zoneName) => `
                <button class="navigable-row" type="button" data-profile-zone-ref="${escapeHTML(zoneName)}" title="${escapeHTML(zoneName)}"
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

function renderHeatmap(values, max, label, unit = "") {
  profileHeatmapSequence += 1;
  const key = `profile-heatmap-${profileHeatmapSequence}`;
  profileHeatmapPaintQueue.set(key, {
    values: [...values],
    max: Math.max(Number(max) || 0, 1e-9),
  });
  return `
    <div class="profile-heatmap-frame" role="img" aria-label="${escapeHTML(`${label} ${t("graph.annualHeatmap")}`)}">
      <div class="profile-heatmap-y">
        <span>00</span>
        <span>12</span>
        <span>24</span>
      </div>
      <canvas class="profile-heatmap" width="365" height="24" data-profile-heatmap-key="${escapeHTML(key)}" aria-hidden="true"></canvas>
      <div class="profile-heatmap-x">
        <span>Jan</span>
        <span>${escapeHTML(formatAxisTick(max / 2, unit))}</span>
        <span>${escapeHTML(formatAxisTick(max, unit))}</span>
        <span>Dec</span>
      </div>
    </div>`;
}

function paintProfileHeatmaps() {
  elements.profileGraph.querySelectorAll("canvas[data-profile-heatmap-key]").forEach((canvas) => {
    const queued = profileHeatmapPaintQueue.get(canvas.dataset.profileHeatmapKey || "");
    const context = canvas.getContext?.("2d");
    if (!queued || !context) {
      return;
    }
    context.clearRect(0, 0, 365, 24);
    queued.values.slice(0, 8760).forEach((value, index) => {
      context.fillStyle = heatColor(value, queued.max);
      context.fillRect(Math.floor(index / 24), index % 24, 1, 1);
    });
  });
  profileHeatmapPaintQueue.clear();
}

function currentProfileGraphOptions() {
  return {
    metricMode: state.profileSettings?.metricMode || "actual",
    timeView: state.profileSettings?.timeView || "year",
    scaleMode: state.profileSettings?.scaleMode || "auto",
  };
}

function currentProfileMetricMode() {
  return state.profileSettings?.metricMode || "actual";
}

function profileGraphSeries(profile, group) {
  const enabledDimensions = new Set(state.profileSettings?.enabledDimensions || []);
  const selectedDimensions = new Set(state.profileSelectedDimensions?.length
    ? state.profileSelectedDimensions
    : state.profileSettings?.enabledDimensions || []);
  const allSeries = Array.isArray(profile?.graphDataset?.series) ? profile.graphDataset.series : [];
  const base = allSeries.filter((series) => {
    if (enabledDimensions.size && !enabledDimensions.has(series.dimension)) return false;
    if (selectedDimensions.size && !selectedDimensions.has(series.dimension)) return false;
    return true;
  });
  const selectedGroupIDs = profileGroupIDsForGraphSelection();
  return profileCurrentGroupSeries(base, selectedGroupIDs.length
    ? selectedGroupIDs
    : [group?.id || state.activeProfileGroupId].filter(Boolean));
}

function profileCurrentGroupSeries(allSeries, selectedGroupIDs) {
  const aggregates = [];
  selectedGroupIDs.forEach((groupID) => {
    const currentGroup = lastProfileView?.groups.find((group) => group.id === groupID);
    if (!currentGroup) {
      return;
    }
    const zoneNames = new Set(currentGroup.zoneNames || []);
    const byDimension = new Map();
    allSeries.forEach((series) => {
      if (series.scopeType !== "zone" || !zoneNames.has(series.zoneName)) {
        return;
      }
      const members = byDimension.get(series.dimension) || [];
      members.push(series);
      byDimension.set(series.dimension, members);
    });
    byDimension.forEach((members, dimensionKey) => {
      const series = members[0];
      const dimension = series.dimension || dimensionKey;
      aggregates.push({
        ...series,
        id: `profile-series-current-${profileSemanticToken(groupID)}-${profileSemanticToken(dimension)}-${profileSemanticToken(series.unit)}`,
        label: `${currentGroup.name} / ${series.dimensionLabel || profileDimensionLabel(dimension)}`,
        scopeType: "group",
        groupId: groupID,
        currentGroupId: groupID,
        groupName: currentGroup.name,
        zoneName: "",
        designValue: members.reduce((sum, member) => sum + (Number(member.designValue) || 0), 0) / members.length,
        scheduleName: [...new Set(members.map((member) => member.scheduleName).filter(Boolean))].join(" + "),
        scheduleHash: [...new Set(members.map((member) => member.scheduleHash).filter(Boolean))].join("+"),
        schedulePattern: [...new Set(members.map((member) => member.schedulePattern).filter(Boolean))].join(" + "),
        sourceItemIds: [...new Set(members.flatMap((member) => member.sourceItemIds || []))],
        warnings: members.flatMap((member) => member.warnings || []),
        aggregateSeries: members,
      });
    });
  });
  return aggregates;
}

function profileSeriesMetric(series, options) {
  if (series.aggregateSeries?.length) {
    const memberMetrics = series.aggregateSeries.map((member) => profileSeriesMetric(member, options));
    const first = memberMetrics[0];
    const values = averageProfileMetricValues(memberMetrics.map((metric) => metric.values));
    return {
      ...first,
      values,
    };
  }
  const timeView = options.timeView || "year";
  const metricMode = options.metricMode || currentProfileMetricMode();
  const multiplier = profileSeriesMultiplier(series, timeView);
  let values = multiplier;
  let unit = "";
  if (metricMode === "design") {
    values = multiplier.map(() => Number(series.designValue) || 0);
    unit = series.unit || "";
  } else if (metricMode === "actual") {
    values = multiplier.map((value) => value * (Number(series.designValue) || 0));
    unit = series.unit || "";
  } else if (metricMode === "annual") {
    values = annualizedProfileValues(series, multiplier, timeView);
    unit = series.unit ? `${series.unit}h` : "h";
  }
  values = values.map((value) => (Number.isFinite(Number(value)) ? Number(value) : 0));
  return {
    unit,
    values,
  };
}

function averageProfileMetricValues(valueSets) {
  const length = Math.max(...valueSets.map((values) => values.length), 0);
  return Array.from({ length }, (_, index) => {
    let total = 0;
    let count = 0;
    valueSets.forEach((values) => {
      const value = Number(values[index]);
      if (Number.isFinite(value)) {
        total += value;
        count += 1;
      }
    });
    return count ? total / count : 0;
  });
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

function graphScaleMaxForSeries(values, series, options) {
  const metricMode = options.metricMode || currentProfileMetricMode();
  if (options.scaleMode === "multiplier_0_1") {
    return metricMode === "multiplier" ? 1 : Math.max(Number(series.designValue) || 0, 1e-9);
  }
  if (options.scaleMode === "design_peak") {
    return Math.max(Number(series.designValue) || 0, Math.max(...values, 0), 1e-9);
  }
  if (options.scaleMode === "percentile") {
    return percentileValue(values, 0.95) || Math.max(...values, 1e-9);
  }
  return Math.max(...values, 1e-9);
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

function focusProfileSeries(seriesID) {
  const series = lastProfileSeriesByID.get(seriesID);
  if (!series) {
    return;
  }
  if (series.groupId) {
    state.activeProfileView = "profile";
    state.activeProfileGroupId = currentProfileGroupIDForSeries(series);
    state.profileSelectedGroupIds = [state.activeProfileGroupId].filter(Boolean);
    state.profileSelectionAnchorKey = profileGroupRowKey(state.activeProfileGroupId);
  }
  selectProfileDimension(series.dimension);
}

function selectProfileDimension(dimension) {
  if (!dimension) {
    return;
  }
  state.profileSelectedDimensions = [dimension];
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
  if (selected.dimension) {
    state.profileSelectedDimensions = [selected.dimension];
  }
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
  if (!target || !targetData || !applyProfileNavigationTarget(targetData)) {
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
    profileSelectedGroupIds: [...(state.profileSelectedGroupIds || [])],
    profileSelectedZoneNames: [...(state.profileSelectedZoneNames || [])],
    profileSelectedDimensions: [...(state.profileSelectedDimensions || [])],
    profileSelectionAnchorKey: state.profileSelectionAnchorKey || "",
    navigationRevealTarget: profileNavigationRevealTarget ? { ...profileNavigationRevealTarget } : null,
    paneScrollTop: Number(elements.profileOverview?.closest(".profile-pane")?.scrollTop) || 0,
    matrixScrollTop: Number(elements.profileMatrix?.scrollTop) || 0,
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
  if (Object.prototype.hasOwnProperty.call(snapshot, "profileSelectedGroupIds")) {
    state.profileSelectedGroupIds = [...(snapshot.profileSelectedGroupIds || [])];
  }
  if (Object.prototype.hasOwnProperty.call(snapshot, "profileSelectedZoneNames")) {
    state.profileSelectedZoneNames = [...(snapshot.profileSelectedZoneNames || [])];
  }
  if (Object.prototype.hasOwnProperty.call(snapshot, "profileSelectedDimensions")) {
    state.profileSelectedDimensions = [...(snapshot.profileSelectedDimensions || [])];
  }
  if (Object.prototype.hasOwnProperty.call(snapshot, "profileSelectionAnchorKey")) {
    state.profileSelectionAnchorKey = String(snapshot.profileSelectionAnchorKey || "");
  }
  profileNavigationRevealTarget = snapshot.navigationRevealTarget ? { ...snapshot.navigationRevealTarget } : null;
  renderProfile(state.report.profile);
  context.refreshSelectionStyles(state.globalSelection, state.globalHover);
  await context.genericRestoreContext(snapshot);
  restoreElementScroll(elements.profileOverview?.closest(".profile-pane"), snapshot.paneScrollTop);
  restoreElementScroll(elements.profileMatrix, snapshot.matrixScrollTop);
  return true;
}

function preferredProfileSemanticOccurrence(selection, context) {
  const navigation = context.navigation;
  const requested = profileSemanticNavigationCache(navigation).occurrence(selection?.occurrenceId);
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
  const cache = profileSemanticNavigationCache(navigation);
  const occurrence = cache.occurrence(selection.occurrenceId);
  const entity = cache.entity(selection.entityId);
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

function applyProfileNavigationTarget(targetData) {
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
    state.profileSelectedGroupIds = [currentGroup.id];
    state.profileSelectionAnchorKey = profileGroupRowKey(currentGroup.id);
    return true;
  }
  if (targetData.schedules) {
    state.profileSelectedCell = null;
    if (targetData.currentGroup) {
      state.activeProfileView = "profile";
      state.activeProfileGroupId = targetData.currentGroup.id;
      state.profileSelectedGroupIds = [targetData.currentGroup.id];
      state.profileSelectionAnchorKey = profileGroupRowKey(targetData.currentGroup.id);
    }
    if (targetData.zoneName) {
      state.activeProfileZoneName = targetData.zoneName;
    }
    return true;
  }
  if (targetData.zoneName) {
    selectProfileZone(targetData.zoneName);
    state.profileSelectedCell = null;
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
  return profileItemMap(profile).get(itemID) || null;
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

function profileOccurrencesForTarget(targetID, navigation = profileNavigationIndex()) {
  const cache = profileSemanticNavigationCache(navigation);
  return cache.occurrencesForIDs(cache.occurrenceIDs("view-target", `profile|${targetID}`));
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

function restoreElementScroll(element, value) {
  if (element) {
    element.scrollTop = Number(value) || 0;
  }
}

function sameProfileName(left, right) {
  return String(left || "").trim().toLowerCase() === String(right || "").trim().toLowerCase();
}

function bindProfileControls() {
  elements.profileSettings?.addEventListener("click", handleProfileSettingsClick);
  elements.profileSettings?.addEventListener("change", handleProfileSettingsChange);
  elements.profileOverview?.addEventListener("click", handleProfileOverviewActivation);
  elements.profileOverview?.addEventListener("keydown", handleProfileOverviewActivation);
  elements.profileGraph?.addEventListener("click", handleProfileGraphActivation);
  elements.profileGraph?.addEventListener("keydown", handleProfileGraphActivation);
  elements.profileDetail?.addEventListener("click", handleProfileDetailActivation);
  elements.profileMatrix?.addEventListener("click", handleProfileMatrixActivation);
  elements.profileMatrix?.addEventListener("keydown", handleProfileMatrixActivation);
}

function handleProfileSettingsClick(event) {
  const button = event.target instanceof Element ? event.target.closest("[data-profile-view]") : null;
  if (!button) {
    return;
  }
  state.activeProfileView = button.dataset.profileView === "zone" ? "zone" : "profile";
  renderCurrentProfile();
}

function handleProfileSettingsChange(event) {
  const input = event.target instanceof Element ? event.target : null;
  if (input?.matches("[data-profile-dimension]")) {
    const dimension = input.dataset.profileDimension;
    const enabled = new Set(state.profileSettings.enabledDimensions);
    if (input.checked) {
      enabled.add(dimension);
    } else {
      enabled.delete(dimension);
    }
    state.profileSettings.enabledDimensions = [...enabled];
  } else if (input?.matches("#profileMetricMode")) {
    state.profileSettings.metricMode = input.value;
  } else {
    return;
  }
  persistProfileSettings();
  renderCurrentProfile();
}

function handleProfileOverviewActivation(event) {
  if (event.type === "keydown" && event.key !== "Enter" && event.key !== " ") {
    return;
  }
  const button = event.target instanceof Element ? event.target.closest("[data-profile-row-key]") : null;
  if (!button) {
    return;
  }
  if (event.type === "keydown") {
    event.preventDefault();
  }
  event.stopPropagation();
  const rowKey = button.dataset.profileRowKey || "";
  const historySnapshot = captureViewSnapshot();
  if (!handleProfileRowSelection(event, rowKey)) {
    return;
  }
  recordViewHistory(historySnapshot);
  renderCurrentProfile();
  const renderedRows = new Map(
    [...elements.profileOverview.querySelectorAll("[data-profile-row-key]")]
      .map((row) => [row.dataset.profileRowKey || "", row]),
  );
  renderedRows.get(rowKey)?.focus({ preventScroll: true });
  const primaryKey = state.activeProfileView === "zone"
    ? profileZoneRowKey(state.activeProfileZoneName)
    : profileGroupRowKey(state.activeProfileGroupId);
  const primaryRow = renderedRows.get(primaryKey);
  const semanticSelection = primaryRow
    ? getPanelNavigationAdapter("profile")?.selectFromElement?.(primaryRow) || null
    : null;
  if (semanticSelection?.entityId) {
    void selectSemanticEntity(semanticSelection, {
      originView: "profile",
      action: "select",
      recordHistory: false,
      follow: false,
    });
  } else {
    void clearSemanticSelection({ originView: "profile", recordHistory: false });
  }
}

function handleProfileGraphActivation(event) {
  if (!(event.target instanceof Element)) {
    return;
  }
  const timeViewButton = event.type === "click" ? event.target.closest("[data-profile-time-view]") : null;
  if (timeViewButton) {
    const timeView = timeViewButton.dataset.profileTimeView || "";
    if (!["day", "week", "month", "year", "duration", "rules"].includes(timeView)) {
      return;
    }
    state.profileSettings.timeView = timeView;
    persistProfileSettings();
    renderCurrentProfile();
    elements.profileGraph.querySelector(`[data-profile-time-view="${timeView}"]`)?.focus({ preventScroll: true });
    return;
  }
  const series = event.target.closest("[data-profile-series-id]");
  if (series && (event.type === "click" || event.key === "Enter" || event.key === " ")) {
    if (event.type === "keydown") {
      event.preventDefault();
    }
    focusProfileSeries(series.dataset.profileSeriesId || "");
    renderCurrentProfile();
    return;
  }
  const zoneButton = event.type === "click" ? event.target.closest("[data-profile-zone-ref]") : null;
  if (zoneButton) {
    selectProfileZone(zoneButton.dataset.profileZoneRef || "");
    renderCurrentProfile();
  }
}

function handleProfileDetailActivation(event) {
  const button = event.target instanceof Element ? event.target.closest("[data-profile-candidate-id]") : null;
  if (!button) {
    return;
  }
  selectProfileDimension(button.dataset.profileDimension || "");
  renderCurrentProfile();
}

function handleProfileMatrixActivation(event) {
  if (!(event.target instanceof Element)) {
    return;
  }
  const cell = event.target.closest("[data-profile-cell]");
  if (event.type === "keydown") {
    if (!cell || (event.key !== "Enter" && event.key !== " ")) {
      return;
    }
    event.preventDefault();
    selectProfileMatrixCell(cell);
    renderCurrentProfile();
    return;
  }
  if (event.target.closest(".profile-object-link")) {
    return;
  }
  if (cell) {
    selectProfileMatrixCell(cell);
    renderCurrentProfile();
    return;
  }
  const row = event.target.closest("tr[data-profile-zone]");
  if (row) {
    selectProfileZone(row.dataset.profileZone || "");
    renderCurrentProfile();
  }
}

function renderCurrentProfile() {
  renderProfile(lastProfileReport || state.report?.profile);
}

export function initializeProfileControls() {
  configureProfilePanelNavigation();
  bindProfileControls();
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
    enabledDimensions: Array.isArray(source.enabledDimensions) ? source.enabledDimensions : defaults.enabledDimensions || [],
    displayMetrics: { ...(defaults.displayMetrics || {}), ...(source.displayMetrics || {}) },
    groupingMetrics: { ...(defaults.groupingMetrics || {}), ...(source.groupingMetrics || {}) },
    numericTolerance: Number(source.numericTolerance) > 0 ? Number(source.numericTolerance) : defaults.numericTolerance || 0.001,
    scheduleCompareMode: source.scheduleCompareMode || defaults.scheduleCompareMode || "name",
    metricMode: source.metricMode || defaults.metricMode || "actual",
    timeView: source.timeView || defaults.timeView || "year",
    scaleMode: source.scaleMode || defaults.scaleMode || "auto",
    applyBehavior: { ...(defaults.applyBehavior || {}), ...(source.applyBehavior || {}) },
  };
}

function persistProfileSettings() {
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
  state.profileSelectedZoneNames = [row.zoneName];
  state.profileSelectionAnchorKey = profileZoneRowKey(row.zoneName);
}

function profileActiveMatrixZoneNames() {
  if (state.activeProfileView === "zone") {
    return new Set(state.profileSelectedZoneNames || []);
  }
  const selectedGroups = new Set(state.profileSelectedGroupIds || []);
  const zoneNames = new Set();
  for (const group of lastProfileView?.groups || []) {
    if (!selectedGroups.has(group.id)) {
      continue;
    }
    for (const zoneName of group.zoneNames || []) {
      zoneNames.add(zoneName);
    }
  }
  return zoneNames;
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
  if (!profile || typeof profile !== "object") {
    return new Map();
  }
  const zoneProfiles = profile.zoneProfiles || [];
  const cached = profileItemMapCache.get(profile);
  if (cached?.zoneProfiles === zoneProfiles) {
    return cached.map;
  }
  const map = new Map();
  zoneProfiles.forEach((zone) => {
    (zone.items || []).forEach((item) => map.set(item.id, item));
  });
  profileItemMapCache.set(profile, { zoneProfiles, map });
  return map;
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
