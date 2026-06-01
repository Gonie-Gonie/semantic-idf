import { backend, elements, escapeHTML, setStatus, state } from "./state.js";
import { getCurrentAppSettings, saveAppSettings } from "./settings-client.js";
import { profileDimensionLabel as i18nProfileDimensionLabel, profileMetricLabel, t } from "./i18n.js";

let lastProfileView = null;

export function renderProfile(profile = state.report?.profile) {
  if (!elements.profileStats) {
    return;
  }
  if (!profile) {
    renderEmptyProfile();
    return;
  }

  state.profileSettings = mergeProfileSettings(profile.defaultSettings, state.profileSettings || getCurrentAppSettings().profile);
  state.activeProfileView = state.activeProfileView === "zone" ? "zone" : "profile";
  lastProfileView = buildProfileView(profile, state.profileSettings);
  if (!state.activeProfileGroupId || !lastProfileView.groups.some((group) => group.id === state.activeProfileGroupId)) {
    state.activeProfileGroupId = lastProfileView.groups[0]?.id || "";
  }

  const query = profileQuery();
  const visibleGroups = lastProfileView.groups.filter((group) => profileGroupMatchesQuery(group, query));
  const visibleRows = lastProfileView.matrix.filter((row) => profileMatrixRowMatchesQuery(row, query));
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
        <span>${t("profile.graph")}</span>
        <select id="profileGraphMode">
          ${optionHTML("actual_value", t("graph.actualValue"), settings.graphMode)}
          ${optionHTML("multiplier", t("graph.multiplier"), settings.graphMode)}
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
    <button class="profile-group-card ${active}" data-profile-group-id="${escapeHTML(group.id)}" type="button">
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
    <button class="profile-group-card profile-zone-card ${active}" data-profile-zone="${escapeHTML(row.zoneName)}" type="button">
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
    <div class="profile-item-table" role="table" aria-label="${escapeHTML(t("profile.sourceObjects"))}">
      <div class="profile-item-row head" role="row">
        <span>${t("common.dimension")}</span><span>${t("common.source")}</span><span>${t("common.schedule")}</span><span>${t("common.method")}</span><span>${t("common.normalized")}</span>
      </div>
      ${items.map(renderProfileItemRow).join("")}
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
    <div class="profile-item-row" role="row">
      <span>${escapeHTML(profileDimensionLabel(item.dimension))}</span>
      <span>
        <button class="profile-object-link navigable-row" data-jump-object-index="${escapeHTML(item.objectIndex)}" data-jump-object-type="${escapeHTML(item.objectType)}" type="button">
          #${escapeHTML(Number(item.objectIndex) + 1)}
        </button>
        ${escapeHTML(item.objectName || item.objectType)}
      </span>
      <span>${escapeHTML(item.scheduleName || "N/A")}<small>${escapeHTML(item.schedulePattern || "")}</small></span>
      <span>${escapeHTML(item.rawMethod || "N/A")}<small>${escapeHTML(item.rawValue || "")}</small></span>
      <span>${escapeHTML(metrics || "N/A")}</span>
    </div>`;
}

function renderProfileMatrix(rows, query, profile) {
  const visibleRows = rows.filter((row) => profileMatrixRowMatchesQuery(row, query));
  const dimensions = (lastProfileView?.dimensions || []).filter((dimension) => state.profileSettings.enabledDimensions.includes(dimension.id));
  const itemMap = profileItemMap(profile);
  elements.profileMatrixStats.textContent = t("count.zones", { count: visibleRows.length });
  elements.profileMatrix.innerHTML = visibleRows.length
    ? `
      <table>
        <thead>
          <tr><th>Zone</th>${dimensions.map((dimension) => `<th>${escapeHTML(profileDimensionLabel(dimension.id))}</th>`).join("")}</tr>
        </thead>
        <tbody>
          ${visibleRows
            .map(
              (row) => `
                <tr class="${profileMatrixRowActive(row) ? "active" : ""}" data-profile-zone="${escapeHTML(row.zoneName)}">
                  <th>
                    <button class="profile-object-link navigable-row" data-jump-object-index="${escapeHTML(row.zoneObjectIndex)}" data-jump-object-type="Zone" type="button">
                      #${escapeHTML(Number(row.zoneObjectIndex) + 1)}
                    </button>
                    ${escapeHTML(row.zoneName)}
                    <small>${escapeHTML(row.groupName || "")}</small>
                  </th>
                  ${dimensions
                    .map((dimension) => {
                      const summary = row.dimensions.find((item) => item.dimension === dimension.id);
                      return renderProfileMatrixCell(summary, itemMap);
                    })
                    .join("")}
                </tr>`,
            )
            .join("")}
        </tbody>
      </table>`
    : `<div class="empty">${t("profile.noMatchingZones")}</div>`;
}

function renderProfileMatrixCell(summary, itemMap) {
  if (!summary) {
    return `<td>N/A</td>`;
  }
  const objects = (summary.itemIds || [])
    .map((id) => itemMap.get(id))
    .filter(Boolean)
    .map(
      (item) => `
        <button class="profile-object-link navigable-row" data-jump-object-index="${escapeHTML(item.objectIndex)}" data-jump-object-type="${escapeHTML(item.objectType)}" type="button">
          #${escapeHTML(Number(item.objectIndex) + 1)} ${escapeHTML(shortObjectType(item.objectType))}
        </button>`,
    )
    .join("");
  return `
    <td>
      <strong>${escapeHTML(summary.displayValue)}</strong>
      <small>${escapeHTML(summary.schedulePattern || summary.scheduleName || "")}</small>
      ${objects ? `<div class="profile-matrix-objects">${objects}</div>` : ""}
    </td>`;
}

function renderProfileGraph(group, profile, zoneRow = null) {
  if (!group) {
    elements.profileGraph.innerHTML = `<div class="empty">${t("profile.graphSelect")}</div>`;
    return;
  }
  const viewMode = currentGraphViewMode();
  const scaleMode = state.profileGraphScaleMode || "auto";
  const schedules = scheduleLookupMap(profile.schedules || []);
  const itemMap = profileItemMap(profile);
  const sourceDimensions = zoneRow?.dimensions || group.dimensions;
  const dimensions = sourceDimensions
    .filter((dimension) => state.profileSettings.enabledDimensions.includes(dimension.dimension))
    .map((dimension) => {
      const item = dimension.itemIds.map((id) => itemMap.get(id)).find((candidate) => candidate?.scheduleName) || null;
      return { dimension, item, schedule: scheduleForProfileDimension(dimension, item, schedules) };
    });
  const cards = dimensions.map((entry) => renderProfileGraphCard(entry.dimension, entry.schedule, viewMode, scaleMode)).join("");
  elements.profileGraphStats.textContent = graphStatsLabel(viewMode, state.profileSettings.graphMode);
  elements.profileGraph.innerHTML = `
    <div class="profile-graph-toolbar">
      <label class="profile-field">
        <span>${t("common.view")}</span>
        <select id="profileGraphViewMode">
          ${optionHTML("annual_heatmap", t("graph.annualHeatmap"), viewMode)}
          ${optionHTML("representative_week", t("graph.representativeWeek"), viewMode)}
          ${optionHTML("hourly_average_by_daytype", t("graph.hourlyByDaytype"), viewMode)}
          ${optionHTML("monthly_average", t("graph.monthlyAverage"), viewMode)}
          ${optionHTML("load_duration", t("graph.loadDuration"), viewMode)}
          ${optionHTML("period_rules", t("graph.periodRules"), viewMode)}
          ${optionHTML("representative_day", t("graph.representativeDay"), viewMode)}
        </select>
      </label>
      <label class="profile-field">
        <span>${t("common.scale")}</span>
        <select id="profileGraphScaleMode">
          ${optionHTML("auto", t("common.auto"), scaleMode)}
          ${optionHTML("design_peak", t("graph.designPeak"), scaleMode)}
          ${optionHTML("multiplier_0_1", t("graph.multiplier01"), scaleMode)}
        </select>
      </label>
    </div>
    ${renderProfileGraphSummary(group, zoneRow, sourceDimensions)}
    <div class="profile-graph-grid">
      ${cards || `<div class="empty">${t("profile.graphNoValues")}</div>`}
    </div>`;
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
                <button class="${zoneName === zoneRow?.zoneName ? "active" : ""}" type="button" data-profile-zone-ref="${escapeHTML(zoneName)}" title="${escapeHTML(zoneName)}">
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
  bindSettingControl("#profileGraphMode", (input) => {
    state.profileSettings.graphMode = input.value;
  });
  const graphView = elements.profileGraph.querySelector("#profileGraphViewMode");
  graphView?.addEventListener("change", () => {
    state.profileGraphViewMode = graphView.value;
    renderProfile(profile);
  });
  const graphScale = elements.profileGraph.querySelector("#profileGraphScaleMode");
  graphScale?.addEventListener("change", () => {
    state.profileGraphScaleMode = graphScale.value;
    renderProfile(profile);
  });
  elements.profileGraph.querySelectorAll("[data-profile-zone-ref]").forEach((button) => {
    button.addEventListener("click", () => {
      selectProfileZone(button.dataset.profileZoneRef || "");
      renderProfile(profile);
    });
  });
  elements.profileMatrix.querySelectorAll("[data-profile-zone]").forEach((row) => {
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
  elements.profileFilter?.addEventListener("input", () => renderProfile());
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
