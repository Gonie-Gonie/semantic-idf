import { backend, elements, escapeHTML, state } from "./state.js";
import { t } from "./i18n.js";

export function initializeOutputControls() {
  elements.outputFilter?.addEventListener("input", () => renderOutput());
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
}

export function renderOutput(output = state.report?.output) {
  if (!elements.outputStats) {
    return;
  }
  if (!output) {
    renderOutputEmpty();
    return;
  }
  const query = (elements.outputFilter?.value || "").trim().toLowerCase();
  const existing = (output.existing || []).filter((item) => outputItemMatchesQuery(item, query));
  const recommendations = (output.recommendations || []).filter((item) => outputRecommendationMatchesQuery(item, query));
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
  elements.outputExisting.innerHTML = existing.length
    ? existing.map(renderExistingOutputCard).join("")
    : `<div class="empty">${t("output.noExisting")}</div>`;
  elements.outputRecommendations.innerHTML = recommendations.length
    ? recommendations.map(renderOutputRecommendationCard).join("")
    : `<div class="empty">${t("output.noRecommendations")}</div>`;
  elements.outputWarnings.innerHTML = warnings.length
    ? warnings.map(renderOutputWarning).join("")
    : `<div class="empty">${t("output.noWarnings")}</div>`;
}

function renderOutputEmpty() {
  elements.outputStats.textContent = t("count.outputs", { count: 0, variables: 0, meters: 0 });
  elements.outputExistingStats.textContent = t("count.objects", { count: 0 });
  elements.outputRecommendationStats.textContent = t("count.options", { count: 0 });
  elements.outputWarningStats.textContent = t("count.warnings", { count: 0 });
  elements.outputExisting.innerHTML = `<div class="empty">${t("output.noAnalysis")}</div>`;
  elements.outputRecommendations.innerHTML = `<div class="empty">${t("output.noRecommendations")}</div>`;
  elements.outputWarnings.innerHTML = `<div class="empty">${t("output.noWarnings")}</div>`;
}

function renderExistingOutputCard(item) {
  return `
    <article class="output-card ${item.duplicate ? "warning" : ""}">
      <div class="output-card-head">
        <div>
          <strong title="${escapeHTML(item.objectType)}">${escapeHTML(item.objectType)}</strong>
          <span title="${escapeHTML(item.summary || "")}">${escapeHTML(item.summary || item.category || "")}</span>
        </div>
        <button class="profile-object-link navigable-row" type="button" data-jump-object-index="${escapeHTML(item.objectIndex)}" data-jump-object-type="${escapeHTML(item.objectType)}">#${escapeHTML(Number(item.objectIndex) + 1)}</button>
      </div>
      <div class="output-field-list">
        ${(item.fields || []).map((field) => renderOutputField(item, field)).join("")}
      </div>
      <div class="output-card-actions">
        <button type="button" data-output-remove="${escapeHTML(item.objectIndex)}">${escapeHTML(t("output.remove"))}</button>
      </div>
    </article>`;
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

function renderOutputRecommendationCard(item) {
  return `
    <article class="output-card recommendation ${item.exists ? "exists" : ""}">
      <div class="output-card-head">
        <div>
          <strong title="${escapeHTML(item.label)}">${escapeHTML(item.label)}</strong>
          <span>${escapeHTML(item.objectType)} - ${escapeHTML(item.category || "")}</span>
        </div>
        <button type="button" data-output-add="${escapeHTML(item.id)}" ${item.exists ? "disabled" : ""}>${escapeHTML(item.exists ? t("output.exists") : t("output.add"))}</button>
      </div>
      <p>${escapeHTML(item.description || "")}</p>
      <div class="output-recommendation-fields">
        ${(item.fields || []).map((field) => `<span><b>${escapeHTML(field.name)}</b>${escapeHTML(field.value)}</span>`).join("")}
      </div>
    </article>`;
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

function openOutputApplyDialog({ title, request, summary }) {
  state.outputApplyRequest = request;
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
  state.outputApplyPreview = null;
}

async function previewOutputApply() {
  const request = state.outputApplyRequest;
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
  const request = state.outputApplyRequest;
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
    ...(item.fields || []).flatMap((field) => [field.name, field.value]),
  ].some((value) => String(value ?? "").toLowerCase().includes(query));
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
  ].some((value) => String(value ?? "").toLowerCase().includes(query));
}

function outputWarningMatchesQuery(item, query) {
  if (!query) {
    return true;
  }
  return [item.severity, item.category, item.code, item.message, item.objectType, item.objectName, item.value]
    .some((value) => String(value ?? "").toLowerCase().includes(query));
}
