import { backend, elements, escapeHTML, state } from "../state.js";
import { t } from "../i18n.js";
import {
  configureResultPanelNavigationHooks,
  refreshResultPanelSelectionStyles,
} from "../panel-navigation-adapters.js";

const OUTPUT_PURPOSE_FILTERS = [
  { id: "basic_energy", labelKey: "simulation.purposeBasicEnergy", fallback: "Basic Energy" },
  { id: "zone_heat_flow", labelKey: "simulation.purposeZoneHeatFlow", fallback: "Zone Heat Flow" },
  { id: "hvac_loop_check", labelKey: "simulation.purposeHVACLoop", fallback: "HVAC Loop Check" },
  { id: "integrity_check", labelKey: "simulation.purposeIntegrity", fallback: "Integrity" },
  { id: "comfort_check", labelKey: "simulation.purposeComfort", fallback: "Comfort" },
  { id: "custom_outputs", labelKey: "simulation.purposeCustom", fallback: "Custom Outputs" },
];
const OUTPUT_RENDER_LIMIT = 500;
let outputNavigationInitialized = false;

export function initializeOutputControls() {
  initializeOutputNavigation();
  elements.outputFilter?.addEventListener("input", () => renderOutput());
  elements.outputPurposeFilter?.addEventListener("change", () => {
    state.outputPurposeFilter = elements.outputPurposeFilter.value || "all";
    renderOutput();
  });
  elements.outputExisting?.addEventListener("click", handleOutputAction);
  elements.outputRecommendations?.addEventListener("click", handleOutputAction);
  elements.outputApplyClose?.addEventListener("click", closeOutputApplyDialog);
  elements.outputPreviewApply?.addEventListener("click", previewOutputApply);
  elements.outputApplyForm?.addEventListener("submit", applyOutput);
  elements.outputApplyBody?.addEventListener("input", () => {
    if (elements.outputApplyDialog?.classList.contains("hidden")) {
      return;
    }
    state.outputApplyPreview = null;
    elements.outputConfirmApply.disabled = true;
    elements.outputApplyStatus.textContent = t("status.runPreview");
  });
  elements.outputApplyBody?.addEventListener("change", () => {
    if (elements.outputApplyDialog?.classList.contains("hidden")) {
      return;
    }
    state.outputApplyPreview = null;
    elements.outputConfirmApply.disabled = true;
    elements.outputApplyStatus.textContent = t("status.runPreview");
  });
}

export function renderOutput(output = state.report?.output) {
  if (!elements.outputStats) {
    return;
  }
  if (!output) {
    renderOutputEmpty();
    return;
  }
  if (state.outputPendingFocusQuery && elements.outputFilter) {
    elements.outputFilter.value = state.outputPendingFocusQuery;
    state.outputPendingFocusQuery = "";
  }
  const query = (elements.outputFilter?.value || state.outputPendingFocusQuery || "").trim().toLowerCase();
  const purposeFilter = elements.outputPurposeFilter?.value || state.outputPurposeFilter || "all";
  state.outputPurposeFilter = purposeFilter;
  syncOutputPurposeFilter(output, purposeFilter);
  let existing = (output.existing || []).filter((item) => outputItemMatchesQuery(item, query) && outputItemMatchesPurpose(item, purposeFilter));
  const temporarilyRevealed = (output.existing || []).find((item) => item.signature === state.outputTemporaryRevealSignature);
  if (temporarilyRevealed && !existing.some((item) => item.signature === temporarilyRevealed.signature)) {
    existing = [...existing, temporarilyRevealed];
  }
  const recommendations = (output.recommendations || []).filter(
    (item) => outputRecommendationMatchesQuery(item, query) && outputItemMatchesPurpose(item, purposeFilter),
  );
  const warnings = (output.warnings || []).filter((item) => outputWarningMatchesQuery(item, query));
  elements.outputStats.textContent = t("count.outputs", {
    count: output.objectCount || 0,
    variables: output.variableCount || 0,
    meters: output.meterCount || 0,
  });
  elements.outputExistingStats.textContent = query
    ? t("count.objectsOf", { shown: existing.length, total: output.existing?.length || 0 })
    : t("count.objects", { count: output.existing?.length || 0 });
  elements.outputRecommendationStats.textContent = query
    ? t("count.optionsOf", { shown: recommendations.length, total: output.recommendations?.length || 0 })
    : t("count.options", { count: output.recommendations?.length || 0 });
  elements.outputWarningStats.textContent = t("count.warnings", { count: warnings.length });
  const renderedExisting = existing.slice(0, OUTPUT_RENDER_LIMIT);
  const renderedRecommendations = recommendations.slice(0, OUTPUT_RENDER_LIMIT);
  const renderedWarnings = warnings.slice(0, OUTPUT_RENDER_LIMIT);
  elements.outputExisting.innerHTML = existing.length
    ? `${renderHiddenRowsNotice(existing.length - renderedExisting.length, "outputs")}${renderOutputRequestTable(renderedExisting)}`
    : `<div class="empty">${t("output.noExisting")}</div>`;
  elements.outputRecommendations.innerHTML = recommendations.length
    ? `${renderHiddenRowsNotice(recommendations.length - renderedRecommendations.length, "recommendations")}${renderOutputRecommendationTable(renderedRecommendations)}`
    : `<div class="empty">${t("output.noRecommendations")}</div>`;
  elements.outputWarnings.innerHTML = warnings.length
    ? `${renderHiddenRowsNotice(warnings.length - renderedWarnings.length, "warnings")}${renderedWarnings.map(renderOutputWarning).join("")}`
    : `<div class="empty">${t("output.noWarnings")}</div>`;
  refreshResultPanelSelectionStyles("output");
}

function renderHiddenRowsNotice(hiddenCount, label) {
  return hiddenCount > 0
    ? `<div class="empty compact">${escapeHTML(`${hiddenCount} additional ${label} hidden. Narrow the filter to render them.`)}</div>`
    : "";
}

function renderOutputEmpty() {
  syncOutputPurposeFilter(null, "all");
  elements.outputStats.textContent = t("count.outputs", { count: 0, variables: 0, meters: 0 });
  elements.outputExistingStats.textContent = t("count.objects", { count: 0 });
  elements.outputRecommendationStats.textContent = t("count.options", { count: 0 });
  elements.outputWarningStats.textContent = t("count.warnings", { count: 0 });
  elements.outputExisting.innerHTML = `<div class="empty">${t("output.noAnalysis")}</div>`;
  elements.outputRecommendations.innerHTML = `<div class="empty">${t("output.noRecommendations")}</div>`;
  elements.outputWarnings.innerHTML = `<div class="empty">${t("output.noWarnings")}</div>`;
}

function syncOutputPurposeFilter(output, selected) {
  if (!elements.outputPurposeFilter) {
    return;
  }
  const counts = outputPurposeCounts(output);
  const options = [
    `<option value="all">${escapeHTML(t("output.allPurposes", {}, "All purposes"))}</option>`,
    ...OUTPUT_PURPOSE_FILTERS.map((purpose) => {
      const count = counts.get(purpose.id) || 0;
      const label = `${t(purpose.labelKey, {}, purpose.fallback)}${count ? ` (${count})` : ""}`;
      return `<option value="${escapeHTML(purpose.id)}" ${selected === purpose.id ? "selected" : ""}>${escapeHTML(label)}</option>`;
    }),
  ];
  const nextHTML = options.join("");
  if (elements.outputPurposeFilter.innerHTML !== nextHTML) {
    elements.outputPurposeFilter.innerHTML = nextHTML;
  }
  elements.outputPurposeFilter.value = selected || "all";
}

function outputPurposeCounts(output) {
  const counts = new Map();
  for (const item of [...(output?.existing || []), ...(output?.recommendations || [])]) {
    for (const tag of item.purposeTags || []) {
      counts.set(tag, (counts.get(tag) || 0) + 1);
    }
  }
  return counts;
}

function renderOutputRequestTable(items) {
  return `
    <table class="output-table">
      <thead>
        <tr>
          <th>${escapeHTML(t("output.scope"))}</th>
          <th>${escapeHTML(t("output.request"))}</th>
          <th>${escapeHTML(t("output.frequency"))}</th>
          <th>${escapeHTML(t("output.destination"))}</th>
          <th>${escapeHTML(t("output.status"))}</th>
          <th>${escapeHTML(t("output.manage"))}</th>
        </tr>
      </thead>
      <tbody>
        ${items.map(renderOutputRequestRow).join("")}
      </tbody>
    </table>`;
}

function renderOutputRequestRow(item) {
  const navigationAttributes = outputNavigationAttributes(item.signature, {
    objectIndex: item.objectIndex,
    objectType: item.objectType,
    objectName: item.objectName,
  });
  return `
    <tr class="navigable-row ${item.duplicate ? "warning" : ""}" ${navigationAttributes} data-output-signature="${escapeHTML(item.signature || "")}" tabindex="0">
      <td>
        <strong title="${escapeHTML(outputScope(item))}">${escapeHTML(outputScope(item))}</strong>
        <small title="${escapeHTML(outputScopeMeta(item))}">${escapeHTML(outputScopeMeta(item))}</small>
      </td>
      <td>
        <strong title="${escapeHTML(outputRequestName(item))}">${escapeHTML(outputRequestName(item))}</strong>
        <small title="${escapeHTML(item.objectType)}">${escapeHTML(outputRequestMeta(item))}</small>
        ${renderOutputPurposeTags(item.purposeTags, item.signature)}
        ${renderOutputEnergySetTag(item)}
      </td>
      <td>${renderOutputFrequencyCell(item)}</td>
      <td>
        <span title="${escapeHTML(outputDestination(item))}">${escapeHTML(outputDestination(item))}</span>
      </td>
      <td>
        <span class="output-status ${item.duplicate ? "warning" : "ok"}">${escapeHTML(item.duplicate ? t("output.duplicate") : t("output.active"))}</span>
      </td>
      <td>
        <div class="output-row-actions">
          <button class="profile-object-link navigable-row" type="button" data-jump-object-index="${escapeHTML(item.objectIndex)}" data-jump-object-type="${escapeHTML(item.objectType)}" title="${escapeHTML(t("panelNavigation.revealSource", {}, "Reveal source"))}" aria-label="${escapeHTML(t("panelNavigation.revealSource", {}, "Reveal source"))}">#${escapeHTML(Number(item.objectIndex) + 1)}</button>
          <button type="button" data-output-remove="${escapeHTML(item.objectIndex)}">${escapeHTML(t("output.remove"))}</button>
        </div>
        <details class="output-field-details">
          <summary>${escapeHTML(t("output.fields"))}</summary>
          <div class="output-field-list">
            ${outputDetailFields(item).map((field) => renderOutputField(item, field)).join("") || `<div class="empty">${escapeHTML(t("output.noExtraFields", {}, "No extra fields"))}</div>`}
          </div>
        </details>
      </td>
    </tr>`;
}

function renderOutputField(item, field) {
  const inputID = `output-field-${item.objectIndex}-${field.index}`;
  const control = (field.choices || []).length
    ? `<select id="${escapeHTML(inputID)}" data-output-field-input="${escapeHTML(item.objectIndex)}:${escapeHTML(field.index)}">
        ${field.choices.map((choice) => `<option value="${escapeHTML(choice)}" ${String(choice) === String(field.value) ? "selected" : ""}>${escapeHTML(choice)}</option>`).join("")}
      </select>`
    : `<input id="${escapeHTML(inputID)}" data-output-field-input="${escapeHTML(item.objectIndex)}:${escapeHTML(field.index)}" value="${escapeHTML(field.value)}" />`;
  return `
    <label class="output-field-row" for="${escapeHTML(inputID)}">
      <span title="${escapeHTML(field.name)}">${escapeHTML(field.name)}</span>
      ${control}
      <button type="button" data-output-update="${escapeHTML(item.objectIndex)}:${escapeHTML(field.index)}">${escapeHTML(t("output.update"))}</button>
    </label>`;
}

function renderOutputFrequencyCell(item) {
  const field = findOutputField(item, "Reporting Frequency");
  if (!field) {
    return `<span>${escapeHTML(item.reportingFrequency || "-")}</span>`;
  }
  const inputID = `output-frequency-${item.objectIndex}`;
  return `
    <label class="output-inline-edit" for="${escapeHTML(inputID)}">
      ${renderOutputFieldControl(item, field, inputID)}
      <button type="button" data-output-update="${escapeHTML(item.objectIndex)}:${escapeHTML(field.index)}">${escapeHTML(t("output.update"))}</button>
    </label>`;
}

function renderOutputFieldControl(item, field, inputID) {
  return (field.choices || []).length
    ? `<select id="${escapeHTML(inputID)}" data-output-field-input="${escapeHTML(item.objectIndex)}:${escapeHTML(field.index)}">
        ${field.choices.map((choice) => `<option value="${escapeHTML(choice)}" ${String(choice) === String(field.value) ? "selected" : ""}>${escapeHTML(choice)}</option>`).join("")}
      </select>`
    : `<input id="${escapeHTML(inputID)}" data-output-field-input="${escapeHTML(item.objectIndex)}:${escapeHTML(field.index)}" value="${escapeHTML(field.value)}" />`;
}

function renderOutputRecommendationTable(items) {
  return `
    <table class="output-table output-library-table">
      <thead>
        <tr>
          <th>${escapeHTML(t("common.category", {}, "Category"))}</th>
          <th>${escapeHTML(t("output.request"))}</th>
          <th>${escapeHTML(t("output.destination"))}</th>
          <th>${escapeHTML(t("common.preview"))}</th>
          <th>${escapeHTML(t("output.manage"))}</th>
        </tr>
      </thead>
      <tbody>
        ${items.map(renderOutputRecommendationRow).join("")}
      </tbody>
    </table>`;
}

function renderOutputRecommendationRow(item) {
  const navigationAttributes = outputNavigationAttributes("outputs");
  return `
    <tr class="navigable-row ${item.exists ? "exists" : ""}" ${navigationAttributes} data-output-recommendation-id="${escapeHTML(item.id || "")}" tabindex="0">
      <td>
        <strong>${escapeHTML(outputCategoryLabel(item.category))}</strong>
        <small>${escapeHTML(item.objectType)}</small>
      </td>
      <td>
        <strong title="${escapeHTML(item.label)}">${escapeHTML(item.label)}</strong>
        <small title="${escapeHTML(item.description || "")}">${escapeHTML(item.description || "")}</small>
        ${renderOutputPurposeTags(item.purposeTags, "outputs")}
        ${renderOutputEnergySetTag(item)}
      </td>
      <td>${escapeHTML(recommendationDestination(item))}</td>
      <td>
        <div class="output-recommendation-fields">
          ${(item.fields || []).map((field) => `<span><b>${escapeHTML(field.name)}</b>${escapeHTML(field.value)}</span>`).join("")}
        </div>
      </td>
      <td>
        <button type="button" data-output-add="${escapeHTML(item.id)}" ${item.exists ? "disabled" : ""}>${escapeHTML(item.exists ? t("output.exists") : t("output.add"))}</button>
      </td>
    </tr>`;
}

function renderOutputWarning(warning) {
  return `
    <article class="hvac-warning ${escapeHTML(warning.severity || "warning")}">
      <strong>${escapeHTML(warning.message || "")}</strong>
      <span>${escapeHTML([warning.category, warning.code, warning.objectType, warning.objectName].filter(Boolean).join(" - "))}</span>
      ${
        warning.objectIndex || warning.objectIndex === 0
          ? `<button class="profile-object-link navigable-row" type="button" data-jump-object-index="${escapeHTML(warning.objectIndex)}" data-jump-object-type="${escapeHTML(warning.objectType || "")}">#${escapeHTML(Number(warning.objectIndex) + 1)}</button>`
          : ""
      }
    </article>`;
}

function renderOutputPurposeTags(tags = [], targetId = "outputs") {
  const values = (tags || []).filter(Boolean);
  if (!values.length) {
    return "";
  }
  return `<div class="output-purpose-tags">${values.map((tag) => `<span class="navigable-row" ${outputNavigationAttributes(targetId, {}, tag)} data-output-purpose="${escapeHTML(tag)}" title="${escapeHTML(tag)}" tabindex="0">${escapeHTML(outputPurposeLabel(tag))}</span>`).join("")}</div>`;
}

function outputPurposeLabel(tag) {
  const purpose = OUTPUT_PURPOSE_FILTERS.find((item) => item.id === tag);
  if (!purpose) {
    return String(tag || "").replaceAll("_", " ");
  }
  return t(purpose.labelKey, {}, purpose.fallback);
}

function initializeOutputNavigation() {
  if (outputNavigationInitialized) {
    return;
  }
  outputNavigationInitialized = true;
  configureResultPanelNavigationHooks("output", {
    canReveal(selection, context) {
      const target = outputTargetForSelection(selection);
      return Boolean(target && (target.targetId === "outputs" || outputRequestForTarget(target))) || context.genericCanReveal(selection);
    },
    async reveal(selection, options, context) {
      const target = outputTargetForSelection(selection);
      if (!target) {
        return undefined;
      }
      const request = outputRequestForTarget(target);
      if (request) {
        state.outputFocusedSignature = request.signature || "";
        state.outputTemporaryRevealSignature = outputRequestIsVisible(request) ? "" : request.signature || "";
      } else {
        state.outputFocusedSignature = "";
        state.outputTemporaryRevealSignature = "";
      }
      renderOutput();
      const focused = request
        ? [...(elements.outputExisting?.querySelectorAll("[data-output-signature]") || [])]
            .find((element) => element.dataset.outputSignature === request.signature)
        : null;
      const targetElement = focused || context.genericFindTarget(selection) || elements.outputExisting;
      if (targetElement) {
        if (options.scroll !== false) {
          targetElement.scrollIntoView?.({ block: "nearest", inline: "nearest", behavior: "auto" });
        }
        if (focused && options.focus !== false) {
          focused.focus?.({ preventScroll: true });
        }
      }
      refreshResultPanelSelectionStyles("output", selection, state.globalHover);
      return Boolean(targetElement);
    },
    captureContext(context) {
      return {
        ...context.genericCaptureContext(),
        filter: elements.outputFilter?.value || "",
        purposeFilter: elements.outputPurposeFilter?.value || state.outputPurposeFilter || "all",
        focusedSignature: state.outputFocusedSignature || "",
        temporaryRevealSignature: state.outputTemporaryRevealSignature || "",
      };
    },
    async restoreContext(snapshot, context) {
      if (elements.outputFilter) {
        elements.outputFilter.value = snapshot.filter || "";
      }
      state.outputPurposeFilter = snapshot.purposeFilter || "all";
      if (elements.outputPurposeFilter) {
        elements.outputPurposeFilter.value = state.outputPurposeFilter;
      }
      state.outputFocusedSignature = snapshot.focusedSignature || "";
      state.outputTemporaryRevealSignature = snapshot.temporaryRevealSignature || "";
      renderOutput();
      return context.genericRestoreContext(snapshot);
    },
    preferredSemanticOccurrence(selection, context) {
      return context.genericPreferredSemanticOccurrence(selection);
    },
  });
}

function outputNavigationAttributes(targetId, explicitAnchor = {}, attachmentContext = "") {
  const navigation = state.semanticProjection?.navigation || {};
  const occurrenceIds = navigation.byViewTarget?.[`output|${targetId}`] || [];
  const occurrences = (navigation.occurrences || []).filter((occurrence) => occurrenceIds.includes(occurrence.occurrenceId));
  const currentPath = state.globalSelection?.semanticPathHint || state.semanticCurrentPath || "";
  const occurrence = occurrences.find((candidate) => candidate.occurrenceId === state.globalSelection?.occurrenceId) ||
    occurrences.sort((left, right) => commonOutputPathPrefix(right.path, currentPath) - commonOutputPathPrefix(left.path, currentPath))[0] || null;
  const entity = (navigation.entities || []).find((candidate) => candidate.id === occurrence?.entityId) || null;
  const sourceAnchor = { ...(occurrence?.sourceAnchor || entity?.sourceAnchors?.[0] || {}), ...explicitAnchor };
  const selected = Boolean(
    (entity?.id && state.globalSelection?.entityId === entity.id) ||
    (targetId && state.outputFocusedSignature === targetId),
  );
  return [
    `data-entity-id="${escapeHTML(entity?.id || "")}"`,
    `data-entity-kind="${escapeHTML(entity?.kind || "")}"`,
    `data-occurrence-context="${escapeHTML(occurrence?.occurrenceId || "")}"`,
    `data-source-object-id="${escapeHTML(sourceAnchor.objectId || "")}"`,
    `data-source-object-index="${escapeHTML(sourceAnchor.objectIndex ?? "")}"`,
    `data-source-field-index="${escapeHTML(sourceAnchor.fieldIndex ?? "")}"`,
    `data-panel-target-id="${escapeHTML(targetId || "")}"`,
    `data-output-attachment-context="${escapeHTML(attachmentContext)}"`,
    `aria-selected="${selected ? "true" : "false"}"`,
  ].join(" ");
}

function outputTargetForSelection(selection = {}) {
  const direct = selection.viewTarget;
  if (String(direct?.view || "").toLowerCase() === "output" && direct.targetId) {
    return direct;
  }
  const navigation = state.semanticProjection?.navigation || {};
  const occurrence = (navigation.occurrences || []).find((candidate) => candidate.occurrenceId === selection.occurrenceId);
  const entity = (navigation.entities || []).find((candidate) => candidate.id === selection.entityId);
  const targets = [...(occurrence?.viewTargets || []), ...(entity?.viewTargets || [])]
    .filter((target) => String(target.view || "").toLowerCase() === "output" && target.targetId)
    .sort((left, right) => Number(right.priority || 0) - Number(left.priority || 0));
  if (selection.originView === "output" && selection.originTargetId) {
    return targets.find((target) => target.targetId === selection.originTargetId) || {
      view: "output",
      targetKind: "request",
      targetId: selection.originTargetId,
    };
  }
  return targets[0] || null;
}

function outputRequestForTarget(target, output = state.report?.output) {
  const requests = output?.existing || [];
  const targetId = String(target?.targetId || "");
  const exact = requests.find((item) => item.signature === targetId);
  if (exact) {
    return exact;
  }
  if (["zone", "attachment"].includes(String(target?.targetKind || "").toLowerCase())) {
    const normalizedTarget = normalizeOutputNavigationText(targetId);
    return requests.find((item) => [item.keyValue, item.objectName, outputScope(item)]
      .some((value) => normalizeOutputNavigationText(value) === normalizedTarget)) || null;
  }
  return null;
}

function outputRequestIsVisible(item) {
  const query = (elements.outputFilter?.value || "").trim().toLowerCase();
  const purpose = elements.outputPurposeFilter?.value || state.outputPurposeFilter || "all";
  return outputItemMatchesQuery(item, query) && outputItemMatchesPurpose(item, purpose);
}

function normalizeOutputNavigationText(value) {
  return String(value || "").trim().toLowerCase();
}

function commonOutputPathPrefix(left = "", right = "") {
  const leftParts = String(left || "").split("/").filter(Boolean);
  const rightParts = String(right || "").split("/").filter(Boolean);
  let length = 0;
  while (length < leftParts.length && leftParts[length] === rightParts[length]) {
    length += 1;
  }
  return length;
}

function renderOutputEnergySetTag(item = {}) {
  const label = outputBasicEnergySetLabel(item);
  if (!label) {
    return "";
  }
  return `<div class="output-set-tags"><span title="${escapeHTML(t("simulation.outputSet", {}, "Output set"))}">${escapeHTML(label)}</span></div>`;
}

function outputBasicEnergySetLabel(item = {}) {
  if (!(item.purposeTags || []).includes("basic_energy")) {
    return "";
  }
  const objectType = String(item.objectType || "").toLowerCase();
  if (objectType === "output:sqlite") {
    return t("simulation.outputSetSQL", {}, "SQL");
  }
  const variableName = outputItemVariableName(item);
  if (outputVariableLooksLikeHeatDriver(variableName)) {
    return t("simulation.basicEnergyDetailHeatDrivers", {}, "Heat drivers");
  }
  if (outputVariableLooksLikeEnergyExplain(variableName) || item.category === "zone_energy") {
    return t("simulation.basicEnergyDetailExplain", {}, "Explain");
  }
  if (outputVariableLooksLikeEnergyUse(variableName) || objectType.startsWith("output:meter") || ["facility_energy", "end_use_energy"].includes(item.category)) {
    return t("simulation.basicEnergyDetailLight", {}, "Light");
  }
  return "";
}

function outputItemVariableName(item = {}) {
  return item.variableName || findOutputField(item, "Variable Name")?.value || "";
}

function outputVariableLooksLikeEnergyExplain(variableName = "") {
  const name = String(variableName || "").toLowerCase();
  return Boolean(
    name &&
      (name.includes("sensible cooling") ||
        name.includes("sensible heating") ||
        name.includes("cooling demand") ||
        name.includes("heating demand") ||
        name.includes("unmet demand") ||
        name.includes("not distributed demand") ||
        name.includes("demand not distributed") ||
        name.includes("cooling coil") ||
        name.includes("heating coil") ||
        name.includes("ideal loads") ||
        name.includes("radiant hvac") ||
        name.includes("electricity energy") ||
        name.includes("gas energy")),
  );
}

function outputVariableLooksLikeEnergyUse(variableName = "") {
  const name = String(variableName || "").toLowerCase();
  return Boolean(name && (name.includes("electric storage charge energy") || name.includes("electric storage discharge energy")));
}

function outputVariableLooksLikeHeatDriver(variableName = "") {
  const name = String(variableName || "").toLowerCase();
  return Boolean(
    name &&
      (name.includes("heat balance") ||
        name.includes("total heating") ||
        name.includes("transmitted solar") ||
        name.includes("solar radiation") ||
        name.includes("heat gain") ||
        name.includes("heat loss") ||
        name.includes("sensible heat gain") ||
        name.includes("sensible heat loss") ||
        name.includes("fan air heat gain")),
  );
}

function handleOutputAction(event) {
  const add = event.target.closest("[data-output-add]");
  if (add && !add.disabled) {
    openOutputApplyDialog({
      title: t("output.addRequest"),
      request: { addRecommendations: [add.dataset.outputAdd] },
      summary: recommendationSummary(add.dataset.outputAdd),
    });
    return;
  }
  const update = event.target.closest("[data-output-update]");
  if (update) {
    const [objectIndex, fieldIndex] = update.dataset.outputUpdate.split(":").map((value) => Number(value));
    const input = [...elements.outputExisting.querySelectorAll("[data-output-field-input]")]
      .find((element) => element.dataset.outputFieldInput === update.dataset.outputUpdate);
    const object = (state.report?.output?.existing || []).find((item) => Number(item.objectIndex) === objectIndex);
    const field = object?.fields?.find((item) => Number(item.index) === fieldIndex);
    openOutputApplyDialog({
      title: t("output.updateRequest"),
      request: { updates: [{ objectIndex, fieldIndex, value: input?.value || "" }] },
      summary: `${object?.objectType || "Output"} #${objectIndex + 1} - ${field?.name || `Field ${fieldIndex + 1}`}`,
    });
    return;
  }
  const remove = event.target.closest("[data-output-remove]");
  if (remove) {
    const objectIndex = Number(remove.dataset.outputRemove);
    const object = (state.report?.output?.existing || []).find((item) => Number(item.objectIndex) === objectIndex);
    openOutputApplyDialog({
      title: t("output.removeRequest"),
      request: { removeObjectIndexes: [objectIndex] },
      summary: `${object?.objectType || "Output"} #${objectIndex + 1} - ${object?.summary || ""}`,
    });
  }
}

export function openStandardOutputApplyDialog() {
  const recommendations = standardOutputRecommendations();
  if (!recommendations.length) {
    openOutputApplyDialog({
      title: t("output.standardOutputTitle", {}, "Apply legacy output preset"),
      request: { preset: "standard", presetMode: "merge" },
      summary: t("output.standardOutputSummary", {}, "Adds the monthly meters and zone energy variables used by standard simulation graphs."),
    });
    return;
  }
  state.outputApplyRequest = null;
  state.outputApplyRequestBuilder = buildStandardOutputApplyRequest;
  state.outputApplyPreview = null;
  elements.outputConfirmApply.disabled = true;
  elements.outputApplyStatus.textContent = t("status.reviewBeforeApplying");
  elements.outputApplyBody.innerHTML = `
    <section>
      <h4>${escapeHTML(t("output.standardOutputTitle", {}, "Apply legacy output preset"))}</h4>
      <p>${escapeHTML(t("output.standardOutputSummary", {}, "Adds the monthly meters and zone energy variables used by standard simulation graphs."))}</p>
      <label class="output-standard-mode" for="outputStandardMode">
        <span>${escapeHTML(t("output.standardOutputMode", {}, "Apply mode"))}</span>
        <select id="outputStandardMode">
          <option value="merge" selected>${escapeHTML(t("output.standardOutputMerge", {}, "Add missing legacy preset outputs"))}</option>
          <option value="replace">${escapeHTML(t("output.standardOutputReplace", {}, "Replace non-preset outputs"))}</option>
          <option value="selected">${escapeHTML(t("output.standardOutputSelected", {}, "Add selected only"))}</option>
        </select>
      </label>
      <div class="output-standard-list">
        ${recommendations.map(renderStandardOutputOption).join("")}
      </div>
    </section>
    <section>
      <h4>${escapeHTML(t("common.preview"))}</h4>
      <div id="outputApplyPreviewList" class="profile-apply-preview"><div class="empty">${escapeHTML(t("status.runPreview"))}</div></div>
    </section>`;
  const mode = elements.outputApplyBody.querySelector("#outputStandardMode");
  mode?.addEventListener("change", () => setStandardOutputDefaults(mode.value));
  setStandardOutputDefaults("merge");
  elements.outputApplyDialog.classList.remove("hidden");
}

function openOutputApplyDialog({ title, request, summary, buildRequest = null }) {
  state.outputApplyRequest = request;
  state.outputApplyRequestBuilder = buildRequest;
  state.outputApplyPreview = null;
  elements.outputConfirmApply.disabled = true;
  elements.outputApplyStatus.textContent = t("status.reviewBeforeApplying");
  elements.outputApplyBody.innerHTML = `
    <section>
      <h4>${escapeHTML(title)}</h4>
      <p>${escapeHTML(summary || "")}</p>
    </section>
    <section>
      <h4>${escapeHTML(t("common.preview"))}</h4>
      <div id="outputApplyPreviewList" class="profile-apply-preview"><div class="empty">${escapeHTML(t("status.runPreview"))}</div></div>
    </section>`;
  elements.outputApplyDialog.classList.remove("hidden");
}

function closeOutputApplyDialog() {
  elements.outputApplyDialog.classList.add("hidden");
  state.outputApplyRequest = null;
  state.outputApplyRequestBuilder = null;
  state.outputApplyPreview = null;
}

async function previewOutputApply() {
  const request = currentOutputApplyRequest();
  if (!request) {
    elements.outputApplyStatus.textContent = t("status.noChangesCanApply");
    return;
  }
  try {
    elements.outputApplyStatus.textContent = t("status.buildingPreview");
    const preview = await callOutputApplyAPI("PreviewOutputApplyText", "/api/output-apply-preview", request);
    state.outputApplyPreview = preview;
    const canApply = preview.canApply ?? preview.CanApply;
    elements.outputConfirmApply.disabled = !canApply;
    renderOutputApplyPreview(preview);
    elements.outputApplyStatus.textContent = canApply ? t("status.previewReady") : t("status.previewBlocking");
  } catch (error) {
    elements.outputApplyStatus.textContent = error?.message || String(error);
  }
}

async function applyOutput(event) {
  event.preventDefault();
  const request = currentOutputApplyRequest();
  if (!request) {
    return;
  }
  try {
    elements.outputApplyStatus.textContent = t("status.applyOutput");
    const result = await callOutputApplyAPI("ApplyOutputText", "/api/output-apply", request);
    window.dispatchEvent(new CustomEvent("idfAnalyzer:outputApplied", { detail: result }));
    closeOutputApplyDialog();
  } catch (error) {
    elements.outputApplyStatus.textContent = error?.message || String(error);
  }
}

async function callOutputApplyAPI(methodName, endpoint, request) {
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
    throw new Error(`Output apply request failed: ${response.status}`);
  }
  return response.json();
}

function renderOutputApplyPreview(preview) {
  const list = elements.outputApplyBody.querySelector("#outputApplyPreviewList");
  const warnings = preview.warnings || [];
  const changes = preview.changes || [];
  list.innerHTML = `
    ${warnings.map(renderOutputWarning).join("")}
    ${
      changes.length
        ? changes.map((change) => `<div class="profile-apply-change"><strong>${escapeHTML(change.action)}</strong><span>${escapeHTML(change.message)}</span></div>`).join("")
        : `<div class="empty">${escapeHTML(t("status.noChanges"))}</div>`
    }`;
}

function recommendationSummary(id) {
  const item = (state.report?.output?.recommendations || []).find((candidate) => candidate.id === id);
  if (!item) {
    return id;
  }
  return `${item.objectType} - ${(item.fields || []).map((field) => `${field.name}: ${field.value}`).join(", ")}`;
}

function currentOutputApplyRequest() {
  return state.outputApplyRequestBuilder ? state.outputApplyRequestBuilder() : state.outputApplyRequest;
}

function standardOutputRecommendations() {
  return (state.report?.output?.recommendations || []).filter((item) => (item.tags || []).some((tag) => String(tag).toLowerCase() === "standard"));
}

function renderStandardOutputOption(item) {
  const checkboxID = `standard-output-${item.id}`;
  return `
    <label class="output-standard-item ${item.exists ? "exists" : ""}" for="${escapeHTML(checkboxID)}">
      <input id="${escapeHTML(checkboxID)}" type="checkbox" data-standard-output-id="${escapeHTML(item.id)}" />
      <span>
        <strong>${escapeHTML(item.label)}</strong>
        <small>${escapeHTML(outputCategoryLabel(item.category))} - ${escapeHTML(standardOutputFieldSummary(item))}</small>
      </span>
      <em>${escapeHTML(item.exists ? t("output.exists") : t("output.add"))}</em>
    </label>`;
}

function setStandardOutputDefaults(mode) {
  elements.outputApplyBody.querySelectorAll("[data-standard-output-id]").forEach((input) => {
    const item = standardOutputRecommendations().find((candidate) => candidate.id === input.dataset.standardOutputId);
    input.checked = mode === "replace" ? true : mode === "selected" ? false : !item?.exists;
  });
}

function buildStandardOutputApplyRequest() {
  const mode = elements.outputApplyBody.querySelector("#outputStandardMode")?.value || "merge";
  const selected = [...elements.outputApplyBody.querySelectorAll("[data-standard-output-id]:checked")]
    .map((input) => input.dataset.standardOutputId)
    .filter(Boolean);
  return {
    preset: "standard",
    presetMode: mode === "replace" ? "replace" : "merge",
    presetRecommendationIds: selected,
  };
}

function standardOutputFieldSummary(item) {
  return (item.fields || []).map((field) => field.value).filter(Boolean).join(" / ") || item.objectType || "";
}

function outputScope(item) {
  if (item.keyValue) {
    return item.keyValue === "*" ? t("output.allKeys") : item.keyValue;
  }
  if (item.objectType === "Output:SQLite") {
    return t("output.wholeSimulation");
  }
  if (item.objectType === "Output:Table:SummaryReports" || item.objectType === "OutputControl:Table:Style") {
    return t("output.tabularReports");
  }
  if (item.objectType === "Output:VariableDictionary") {
    return t("output.variableDictionary");
  }
  if (item.objectType === "Output:Diagnostics") {
    return t("output.diagnostics");
  }
  return item.category || item.objectType || "-";
}

function outputScopeMeta(item) {
  if (item.objectType === "Output:Variable") {
    return t("output.timeSeriesVariable");
  }
  if ((item.objectType || "").startsWith("Output:Meter")) {
    return t("output.energyMeter");
  }
  return item.objectType || "";
}

function outputRequestName(item) {
  if (item.variableName) {
    return item.variableName;
  }
  return item.summary || item.objectType || "-";
}

function outputRequestMeta(item) {
  const parts = [outputCategoryLabel(item.category)];
  if (item.scheduleName) {
    parts.push(`${t("common.schedule")}: ${item.scheduleName}`);
  }
  return parts.filter(Boolean).join(" - ");
}

function outputDestination(item) {
  switch (item.category) {
    case "variables":
      return "Time-series output";
    case "meters":
      return (item.objectType || "").includes("MeterFileOnly") ? "Meter file only" : "Meter output";
    case "dictionary":
      return "Variable dictionary";
    case "files":
      return "SQLite / result files";
    case "tabular":
      return "Tabular reports";
    case "diagnostics":
      return "Diagnostics report";
    case "standard_controls":
      return "Standard result files";
    case "facility_energy":
      return "Facility monthly meter";
    case "end_use_energy":
      return "End-use monthly meter";
    case "zone_energy":
      return "Zone monthly variable";
    case "zone_heat_flow":
      return "Zone hourly heat-flow variable";
    default:
      return item.objectType || "-";
  }
}

function recommendationDestination(item) {
  return outputDestination({ category: item.category, objectType: item.objectType });
}

function outputCategoryLabel(category) {
  return t(`output.category.${category}`, {}, category || "Output");
}

function findOutputField(item, name) {
  const wanted = normalizeOutputFieldName(name);
  return (item.fields || []).find((field) => normalizeOutputFieldName(field.name) === wanted);
}

function outputDetailFields(item) {
  return (item.fields || []).filter((field) => normalizeOutputFieldName(field.name) !== "reporting frequency");
}

function normalizeOutputFieldName(value) {
  return String(value || "").trim().toLowerCase().replaceAll("_", " ").replaceAll("-", " ").replace(/\s+/g, " ");
}

function outputItemMatchesQuery(item, query) {
  if (!query) {
    return true;
  }
  return [
    item.objectType,
    item.objectName,
    item.category,
    item.summary,
    item.keyValue,
    item.variableName,
    item.reportingFrequency,
    ...(item.purposeTags || []).map(outputPurposeLabel),
    outputBasicEnergySetLabel(item),
    ...(item.fields || []).flatMap((field) => [field.name, field.value]),
  ].some((value) => String(value ?? "").toLowerCase().includes(query));
}

function outputItemMatchesPurpose(item, purposeFilter) {
  if (!purposeFilter || purposeFilter === "all") {
    return true;
  }
  return (item.purposeTags || []).includes(purposeFilter);
}

function outputRecommendationMatchesQuery(item, query) {
  if (!query) {
    return true;
  }
  return [
    item.id,
    item.label,
    item.category,
    item.description,
    item.objectType,
    ...(item.fields || []).flatMap((field) => [field.name, field.value]),
    ...(item.tags || []),
    ...(item.purposeTags || []).map(outputPurposeLabel),
    outputBasicEnergySetLabel(item),
  ].some((value) => String(value ?? "").toLowerCase().includes(query));
}

function outputWarningMatchesQuery(item, query) {
  if (!query) {
    return true;
  }
  return [item.severity, item.category, item.code, item.message, item.objectType, item.objectName, item.value]
    .some((value) => String(value ?? "").toLowerCase().includes(query));
}
