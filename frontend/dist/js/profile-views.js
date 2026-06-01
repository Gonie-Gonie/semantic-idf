import { backend, elements, escapeHTML, setStatus, state } from "./state.js";
import { getCurrentAppSettings, saveAppSettings } from "./settings-client.js";

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
  lastProfileView = buildProfileView(profile, state.profileSettings);
  if (!state.activeProfileGroupId || !lastProfileView.groups.some((group) => group.id === state.activeProfileGroupId)) {
    state.activeProfileGroupId = lastProfileView.groups[0]?.id || "";
  }

  const query = profileQuery();
  const visibleGroups = lastProfileView.groups.filter((group) => profileGroupMatchesQuery(group, query));
  if (query && !visibleGroups.some((group) => group.id === state.activeProfileGroupId)) {
    state.activeProfileGroupId = visibleGroups[0]?.id || "";
  }
  const selectedGroup = query
    ? visibleGroups.find((group) => group.id === state.activeProfileGroupId) || null
    : selectedProfileGroup();

  elements.profileStats.textContent = query
    ? `${visibleGroups.length} of ${lastProfileView.groups.length} profiles, ${profile.itemCount || 0} items`
    : `${lastProfileView.groups.length} profiles, ${profile.itemCount || 0} items`;
  elements.profileApplyButton.disabled = !selectedGroup;
  renderProfileSettings(profile);
  renderProfileOverview(visibleGroups);
  renderProfileDetail(selectedGroup, profile);
  renderProfileMatrix(lastProfileView.matrix, query, profile);
  renderProfileGraph(selectedGroup, profile);
  bindProfileControls(profile);
}

function renderEmptyProfile() {
  elements.profileStats.textContent = "0 profiles";
  elements.profileSettings.innerHTML = "";
  elements.profileOverview.innerHTML = `<div class="empty">No profile analysis yet</div>`;
  elements.profileDetail.innerHTML = `<div class="empty">Select a profile</div>`;
  elements.profileMatrix.innerHTML = `<div class="empty">No profile matrix yet</div>`;
  elements.profileMatrixStats.textContent = "0 zones";
  elements.profileGraph.innerHTML = `<div class="empty">No profile graph yet</div>`;
  elements.profileGraphStats.textContent = "Representative day";
  elements.profileApplyButton.disabled = true;
}

function renderProfileSettings(profile) {
  const settings = state.profileSettings;
  const dimensions = profile.dimensions || [];
  elements.profileSettings.innerHTML = `
    <div class="profile-live-controls">
      <div class="profile-live-group">
        <span class="profile-live-label">Dimensions</span>
        <div class="profile-toggle-row">
          ${dimensions
            .map(
              (dimension) => `
                <label class="profile-check compact">
                  <input data-profile-dimension="${escapeHTML(dimension.id)}" type="checkbox" ${settings.enabledDimensions.includes(dimension.id) ? "checked" : ""} />
                  <span>${escapeHTML(dimension.label)}</span>
                </label>`,
            )
            .join("")}
        </div>
      </div>
      <label class="profile-field profile-live-select">
        <span>Graph</span>
        <select id="profileGraphMode">
          ${optionHTML("actual_value", "Actual value", settings.graphMode)}
          ${optionHTML("multiplier", "Multiplier", settings.graphMode)}
        </select>
      </label>
    </div>`;
}

function applyModeSelect(id, value) {
  return `
      <select id="${escapeHTML(id)}">
        ${optionHTML("clone", "Clone objects", value)}
        ${optionHTML("shared", "Edit ZoneList", value)}
      </select>
    `;
}

function replacePolicySelect(id, value) {
  return `
      <select id="${escapeHTML(id)}">
        ${optionHTML("replace", "Replace", value)}
        ${optionHTML("keep", "Keep", value)}
        ${optionHTML("duplicate", "Duplicate", value)}
      </select>
    `;
}

function optionHTML(value, label, selected) {
  return `<option value="${escapeHTML(value)}" ${String(selected) === String(value) ? "selected" : ""}>${escapeHTML(label)}</option>`;
}

function renderProfileOverview(groups) {
  elements.profileOverview.innerHTML = groups.length
    ? groups.map(renderProfileGroupCard).join("")
    : `<div class="empty">No matching profile groups</div>`;
}

function renderProfileGroupCard(group) {
  const active = group.id === state.activeProfileGroupId ? "active" : "";
  return `
    <button class="profile-group-card ${active}" data-profile-group-id="${escapeHTML(group.id)}" type="button">
      <span>
        <strong>${escapeHTML(group.name)}</strong>
        <small>${escapeHTML(group.zoneCount)} zones</small>
      </span>
      <span class="profile-card-zones">${escapeHTML(group.zoneNames.slice(0, 4).join(", "))}${group.zoneNames.length > 4 ? "..." : ""}</span>
      <span class="profile-card-metrics">${group.dimensions.map((dimension) => `${escapeHTML(dimension.label)} ${escapeHTML(dimension.displayValue)}`).join(" / ")}</span>
    </button>`;
}

function renderProfileDetail(group, profile) {
  if (!group) {
    elements.profileDetail.innerHTML = `<div class="empty">Select a profile group</div>`;
    return;
  }
  const itemMap = profileItemMap(profile);
  const items = group.itemIds.map((id) => itemMap.get(id)).filter(Boolean);
  const warnings = [...group.warnings, ...items.flatMap((item) => item.warnings || [])];
  elements.profileDetail.innerHTML = `
    <div class="profile-detail-head">
      <div>
        <h3>${escapeHTML(group.name)}</h3>
        <p>${escapeHTML(group.zoneNames.join(", "))}</p>
      </div>
      <span class="badge">${escapeHTML(group.zoneCount)} zones</span>
    </div>
    <div class="profile-dimension-grid">
      ${group.dimensions.map(renderProfileDimensionSummary).join("")}
    </div>
    ${warnings.length ? `<div class="profile-warning-list">${warnings.map(renderProfileWarning).join("")}</div>` : ""}
    <div class="profile-item-table" role="table" aria-label="Profile source objects">
      <div class="profile-item-row head" role="row">
        <span>Dimension</span><span>Source</span><span>Schedule</span><span>Method</span><span>Normalized</span>
      </div>
      ${items.map(renderProfileItemRow).join("")}
    </div>`;
}

function renderProfileDimensionSummary(dimension) {
  return `
    <article class="profile-dimension-card">
      <strong>${escapeHTML(dimension.label)}</strong>
      <span>${escapeHTML(dimension.displayValue)}</span>
      <small>${escapeHTML(dimension.scheduleName || dimension.schedulePattern || "No schedule")}</small>
    </article>`;
}

function renderProfileItemRow(item) {
  const metrics = (item.normalized || [])
    .filter((metric) => metric.status !== "missing")
    .map((metric) => `${metric.label}: ${metric.displayValue}`)
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
  elements.profileMatrixStats.textContent = `${visibleRows.length} zones`;
  elements.profileMatrix.innerHTML = visibleRows.length
    ? `
      <table>
        <thead>
          <tr><th>Zone</th>${dimensions.map((dimension) => `<th>${escapeHTML(dimension.label)}</th>`).join("")}</tr>
        </thead>
        <tbody>
          ${visibleRows
            .map(
              (row) => `
                <tr class="${selectedProfileGroup()?.zoneNames.includes(row.zoneName) ? "active" : ""}" data-profile-zone="${escapeHTML(row.zoneName)}">
                  <th>
                    <button class="profile-object-link navigable-row" data-jump-object-index="${escapeHTML(row.zoneObjectIndex)}" data-jump-object-type="Zone" type="button">
                      #${escapeHTML(Number(row.zoneObjectIndex) + 1)}
                    </button>
                    ${escapeHTML(row.zoneName)}
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
    : `<div class="empty">No matching zones</div>`;
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

function renderProfileGraph(group, profile) {
  if (!group) {
    elements.profileGraph.innerHTML = `<div class="empty">Select a profile group to view graph summaries.</div>`;
    return;
  }
  const viewMode = currentGraphViewMode();
  const scaleMode = state.profileGraphScaleMode || "auto";
  const schedules = new Map((profile.schedules || []).map((schedule) => [schedule.scheduleName, schedule]));
  const itemMap = profileItemMap(profile);
  const dimensions = group.dimensions
    .filter((dimension) => state.profileSettings.enabledDimensions.includes(dimension.dimension))
    .map((dimension) => {
      const item = dimension.itemIds.map((id) => itemMap.get(id)).find((candidate) => candidate?.scheduleName) || null;
      return { dimension, item, schedule: item ? schedules.get(item.scheduleName) : null };
    });
  const cards = dimensions.map((entry) => renderProfileGraphCard(entry.dimension, entry.schedule, viewMode, scaleMode)).join("");
  elements.profileGraphStats.textContent = graphStatsLabel(viewMode, state.profileSettings.graphMode);
  elements.profileGraph.innerHTML = `
    <div class="profile-graph-toolbar">
      <label class="profile-field">
        <span>View</span>
        <select id="profileGraphViewMode">
          ${optionHTML("annual_heatmap", "Annual heatmap", viewMode)}
          ${optionHTML("representative_week", "Representative week", viewMode)}
          ${optionHTML("period_rules", "Through / For rules", viewMode)}
          ${optionHTML("representative_day", "Representative days", viewMode)}
        </select>
      </label>
      <label class="profile-field">
        <span>Scale</span>
        <select id="profileGraphScaleMode">
          ${optionHTML("auto", "Auto", scaleMode)}
          ${optionHTML("design_peak", "Design peak", scaleMode)}
          ${optionHTML("multiplier_0_1", "Multiplier 0-1", scaleMode)}
        </select>
      </label>
    </div>
    ${renderProfileGraphSummary(group)}
    <div class="profile-graph-grid">
      ${cards || `<div class="empty">No scheduled profile values for this group.</div>`}
    </div>`;
}

function renderProfileGraphSummary(group) {
  return `
    <div class="profile-graph-summary">
      <div>
        <strong>${escapeHTML(group.name)}</strong>
        <span>${escapeHTML(group.zoneNames.join(", "))}</span>
      </div>
      <div class="profile-graph-summary-metrics">
        ${group.dimensions.map(renderProfileDimensionSummary).join("")}
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
      ${warnings}
      ${renderGraphVisual(graphData, values, max, dimension)}
      <small>Peak ${escapeHTML(formatGraphNumber(Math.max(...values, 0), unit))} / scale ${escapeHTML(formatGraphNumber(max, unit))}</small>
    </article>`;
}

function renderGraphVisual(graphData, values, max, dimension) {
  switch (graphData.kind) {
    case "heatmap":
      return renderHeatmap(values, max, dimension.label);
    case "rules":
      return renderRuleGraph(graphData, values, max);
    case "day_profiles":
      return renderDayProfiles(graphData, values, max);
    default:
      return renderLineGraph(values, max, `${dimension.label} ${graphData.label}`);
  }
}

function graphDataForDimension(dimension, schedule, viewMode) {
  const unresolvedWarning = schedule && schedule.resolved === false
    ? "Schedule could not be fully parsed; showing design-level fallback instead of treating it as zero."
    : "";
  switch (viewMode) {
    case "representative_week":
      return {
        kind: "line",
        label: `${schedule?.detectedPattern || "No schedule"} / representative week`,
        values: scheduleWeeklyProfile(schedule),
        warning: unresolvedWarning,
      };
    case "period_rules":
      return {
        kind: "rules",
        label: `${schedule?.detectedPattern || "No schedule"} / Through-For rules`,
        values: scheduleRuleValues(schedule),
        rules: scheduleRules(schedule),
        warning: unresolvedWarning,
      };
    case "representative_day":
      return {
        kind: "day_profiles",
        label: `${schedule?.detectedPattern || "No schedule"} / representative days`,
        values: [
          ...scheduleDayProfile(schedule, "weekdayProfile"),
          ...scheduleDayProfile(schedule, "saturdayProfile"),
          ...scheduleDayProfile(schedule, "sundayProfile"),
        ],
        profiles: [
          { label: "Weekday", values: scheduleDayProfile(schedule, "weekdayProfile") },
          { label: "Saturday", values: scheduleDayProfile(schedule, "saturdayProfile") },
          { label: "Sunday", values: scheduleDayProfile(schedule, "sundayProfile") },
        ],
        warning: unresolvedWarning,
      };
    default:
      return {
        kind: "heatmap",
        label: `${schedule?.detectedPattern || "No schedule"} / annual heatmap`,
        values: annualScheduleValues(schedule),
        warning: unresolvedWarning,
      };
  }
}

function renderLineGraph(values, max, label) {
  const points = values
    .map((value, index) => `${values.length <= 1 ? 0 : (index / (values.length - 1)) * 100},${80 - (clampGraphValue(value, max) / max) * 70}`)
    .join(" ");
  return `
    <svg class="profile-line-graph" viewBox="0 0 100 84" role="img" aria-label="${escapeHTML(label)}">
      <line x1="0" y1="80" x2="100" y2="80"></line>
      <polyline points="${points}"></polyline>
    </svg>`;
}

function renderDayProfiles(graphData, values, max) {
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
              ${renderLineGraph(profileValues, max, profile.label)}
            </div>`;
        })
        .join("")}
    </div>`;
}

function renderRuleGraph(graphData, values, max) {
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
              ${renderLineGraph(ruleValues.length ? ruleValues : [0], max, rule.label || "Schedule rule")}
            </div>`;
        })
        .join("") || `<div class="empty">No Through / For rules available.</div>`}
    </div>`;
}

function renderHeatmap(values, max, label) {
  const rects = values
    .map((value, index) => {
      const day = Math.floor(index / 24);
      const hour = index % 24;
      return `<rect x="${day}" y="${hour}" width="1" height="1" fill="${heatColor(value, max)}"></rect>`;
    })
    .join("");
  return `
    <svg class="profile-heatmap" viewBox="0 0 365 24" preserveAspectRatio="none" role="img" aria-label="${escapeHTML(label)} annual heatmap">
      ${rects}
    </svg>`;
}

function currentGraphViewMode() {
  const allowed = new Set(["annual_heatmap", "representative_week", "period_rules", "representative_day"]);
  if (allowed.has(state.profileGraphViewMode)) {
    return state.profileGraphViewMode;
  }
  return "annual_heatmap";
}

function graphStatsLabel(viewMode, graphMode) {
  const valueLabel = graphMode === "multiplier" ? "Schedule multiplier" : "Actual value";
  switch (viewMode) {
    case "representative_week":
      return `${valueLabel}, representative week`;
    case "period_rules":
      return `${valueLabel}, Through / For rules`;
    case "representative_day":
      return `${valueLabel}, representative days`;
    default:
      return `${valueLabel}, annual heatmap`;
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

function scheduleRules(schedule) {
  return Array.isArray(schedule?.rules) && schedule.rules.length
    ? schedule.rules
    : [{ startDay: 1, endDay: 365, selector: "AllDays", label: "Fallback all days", intervals: [{ startHour: 0, endHour: 24, value: 1 }] }];
}

function scheduleRuleValues(schedule) {
  return scheduleRules(schedule).flatMap((rule) => (rule.intervals || []).map((interval) => Number(interval.value) || 0));
}

function annualScheduleValues(schedule) {
  const rules = scheduleRules(schedule);
  const values = [];
  for (let day = 1; day <= 365; day += 1) {
    const rule = rules.find((candidate) => day >= Number(candidate.startDay || 1) && day <= Number(candidate.endDay || 365) && dayMatchesScheduleSelector(day, candidate.selector));
    values.push(...profileFromRule(rule));
  }
  return values;
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
  const selector = String(selectorInput || "AllDays").trim().toLowerCase().replaceAll(" ", "");
  const dayOfWeek = (day - 1) % 7;
  switch (selector) {
    case "weekdays":
      return dayOfWeek >= 0 && dayOfWeek <= 4;
    case "weekends":
      return dayOfWeek === 5 || dayOfWeek === 6;
    case "monday":
      return dayOfWeek === 0;
    case "tuesday":
      return dayOfWeek === 1;
    case "wednesday":
      return dayOfWeek === 2;
    case "thursday":
      return dayOfWeek === 3;
    case "friday":
      return dayOfWeek === 4;
    case "saturday":
      return dayOfWeek === 5;
    case "sunday":
      return dayOfWeek === 6;
    default:
      return true;
  }
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
  return `<div class="profile-warning ${escapeHTML(warning.severity || "warning")}">${escapeHTML(warning.message || warning.code || "Profile warning")}</div>`;
}

function bindProfileControls(profile) {
  elements.profileOverview.querySelectorAll("[data-profile-group-id]").forEach((button) => {
    button.addEventListener("click", () => {
      state.activeProfileGroupId = button.dataset.profileGroupId || "";
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
  elements.profileMatrix.querySelectorAll("[data-profile-zone]").forEach((row) => {
    row.addEventListener("click", () => {
      const group = lastProfileView?.groups.find((candidate) => candidate.zoneNames.includes(row.dataset.profileZone));
      if (!group) {
        return;
      }
      state.activeProfileGroupId = group.id;
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
    elements.profileApplyStatus.textContent = "Run preview before applying.";
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
  elements.profileApplyStatus.textContent = "Review changes before applying.";
  const sourceZones = new Set(group.zoneNames);
  const targets = (profile.zoneProfiles || []).filter((zone) => !sourceZones.has(zone.zoneName));
  const dimensions = (profile.dimensions || []).filter((dimension) => state.profileSettings.enabledDimensions.includes(dimension.id));
  elements.profileApplyBody.innerHTML = `
    <section>
      <h4>${escapeHTML(group.name)}</h4>
      <p>${escapeHTML(group.zoneNames.join(", "))}</p>
    </section>
    <section>
      <h4>Target Zones</h4>
      <div class="profile-target-list">
        ${targets
          .map(
            (zone) => `
              <label class="profile-check">
                <input data-profile-target-zone="${escapeHTML(zone.zoneName)}" type="checkbox" />
                <span>${escapeHTML(zone.zoneName)}</span>
              </label>`,
          )
          .join("") || `<div class="empty">No other zones available.</div>`}
      </div>
    </section>
    <section>
      <h4>Dimensions</h4>
      <div class="profile-chip-grid">
        ${dimensions
          .map(
            (dimension) => `
              <label class="profile-check">
                <input data-profile-apply-dimension="${escapeHTML(dimension.id)}" type="checkbox" checked />
                <span>${escapeHTML(dimension.label)}</span>
              </label>`,
          )
          .join("")}
      </div>
    </section>
    <section>
      <h4>Options</h4>
      <div class="profile-dialog-options">
        <label class="profile-field">
          <span>Apply mode</span>
          ${applyModeSelect("profileApplyModeDialog", state.profileSettings.applyBehavior.defaultMode)}
        </label>
        <label class="profile-field">
          <span>Existing target</span>
          ${replacePolicySelect("profileReplacePolicyDialog", state.profileSettings.applyBehavior.replaceExistingPolicy)}
        </label>
        <label class="profile-check"><input id="profileAllowZoneListEdit" type="checkbox" ${state.profileSettings.applyBehavior.allowZoneListEdit ? "checked" : ""} /> <span>Allow shared ZoneList edits</span></label>
      </div>
    </section>
    <section>
      <h4>Preview</h4>
      <div id="profileApplyPreviewList" class="profile-apply-preview"><div class="empty">Run preview before applying.</div></div>
    </section>`;
  elements.profileApplyDialog.classList.remove("hidden");
}

function closeProfileApplyDialog() {
  elements.profileApplyDialog.classList.add("hidden");
}

async function previewProfileApply() {
  const request = profileApplyRequest();
  if (!request.targetZoneNames.length) {
    elements.profileApplyStatus.textContent = "Select at least one target zone.";
    return;
  }
  try {
    elements.profileApplyStatus.textContent = "Building preview";
    const preview = await callProfileApplyAPI("PreviewProfileApplyText", "/api/profile-apply-preview", request);
    state.profileApplyPreview = preview;
    const canApply = preview.canApply ?? preview.CanApply;
    elements.profileConfirmApply.disabled = !canApply;
    renderApplyPreview(preview);
    elements.profileApplyStatus.textContent = canApply ? "Preview ready." : "Preview has blocking warnings.";
  } catch (error) {
    elements.profileApplyStatus.textContent = error?.message || String(error);
  }
}

async function applyProfile(event) {
  event.preventDefault();
  const request = profileApplyRequest();
  try {
    elements.profileApplyStatus.textContent = "Applying profile";
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
    ${changes.length ? changes.map((change) => `<div class="profile-apply-change"><strong>${escapeHTML(change.action)}</strong><span>${escapeHTML(change.message)}</span></div>`).join("") : `<div class="empty">No changes.</div>`}`;
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
  return { dimensions, matrix, groups };
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
      label = metric.label || label;
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
    label: dimension.label,
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
  return state.report?.profile?.dimensions?.find((item) => item.id === dimension)?.label || dimension;
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
