import { backend, elements, escapeHTML, getDocumentText, setDocumentText, setStatus, state } from "../state.js";
import { localizedMessage, t } from "../i18n.js";
import { recordViewHistory } from "../view-history.js";
import { currentSemanticSelection } from "../selection-controller.js";
import { getPanelNavigationAdapter } from "../panel-navigation-registry.js";
import { retainSemanticNavigationProjection } from "../semantic-navigation-cache.js";
import { normalizeInputView } from "../input-view-policy.js";

const FIELD_TABLE_RENDER_LIMIT = 500;

let analyzeCallback = async () => {};
let renderReportCallback = () => renderInputViews();
let jumpIndexCache = { report: null, definitions: new Map(), references: new Map() };
let materializedInputSource = null;
let jsonEditorControlsBound = false;
let formattedTextControlsBound = false;
let fieldTableControlsBound = false;

window.addEventListener("idfAnalyzer:semanticSelectionChanged", () => {
  if (materializedInputSource !== null && materializedInputSource !== selectedInputObjectIndex()) renderInputViews();
});
window.addEventListener("idfAnalyzer:semanticSelectionRemapped", () => {
  restoreInputSelectionAfterAnalysis();
});

export function configureInputViews(callbacks) {
  analyzeCallback = callbacks.analyze || analyzeCallback;
  renderReportCallback = callbacks.renderReport || renderReportCallback;
}

export function renderInputViews() {
  materializedInputSource = null;
  if (state.activeInputView === "text") {
    renderFormattedTextView();
  }
  if (state.activeInputView === "json") {
    renderJSONView();
  }
  if (state.activeInputView === "table") {
    renderFieldTable();
  }
}

export function setInputFilter(query) {
  state.inputFilterQuery = query;
  if (elements.inputFilter && elements.inputFilter.value !== query) {
    elements.inputFilter.value = query;
  }
  renderInputViews();
}

export function clearInputFilter() {
  setInputFilter("");
}

function currentInputFilterTerms() {
  return state.inputFilterQuery.trim().toLowerCase().split(/\s+/).filter(Boolean);
}

function selectedInputObjectIndex() {
  const index = currentSemanticSelection().sourceAnchor?.objectIndex;
  return index === undefined || index === null || index === "" ? null : String(index);
}

function inputObjectIndex(object) {
  return String(object.index ?? object.sourceIndex ?? "");
}

function filterInputObjects(objects) {
  const terms = currentInputFilterTerms();
  const selected = selectedInputObjectIndex();
  return objects.filter((object) => {
    if (matchesInputFilter(object, terms)) return true;
    if (selected !== null && inputObjectIndex(object) === selected) {
      materializedInputSource = selected;
      return true;
    }
    return false;
  });
}

function matchesInputFilter(object, terms) {
  if (!terms.length) {
    return true;
  }
  const fields = object.fields || [];
  const haystack = [
    object.index ?? "",
    object.sourceIndex ?? "",
    object.type || "",
    object.name || "",
    ...fields.flatMap((field) => [field.key || "", field.comment || "", formatJSONValue(field.value)]),
  ]
    .join(" ")
    .toLowerCase();
  return terms.every((term) => haystack.includes(term));
}

function groupObjectsByType(objects) {
  const groups = [];
  const byType = new Map();
  objects.forEach((object) => {
    const objectType = object.type || "Object";
    if (!byType.has(objectType)) {
      const group = { type: objectType, objects: [] };
      groups.push(group);
      byType.set(objectType, group);
    }
    byType.get(objectType).objects.push(object);
  });
  return groups;
}

function groupedReportObjects() {
  const report = state.report;
  if (!report || !Array.isArray(report.objects)) {
    return [];
  }
  return groupObjectsByType(filterInputObjects(report.objects));
}

function hasCurrentAnalysis() {
  return state.reportAnalyzedText !== "" && state.reportAnalyzedText === getDocumentText();
}

function pendingViewMessage(viewName) {
  if (!getDocumentText().trim()) {
    return t("input.noLoaded");
  }
  return t("input.pendingView", { view: viewName });
}

function captureInputEditSelection() {
  const selection = currentSemanticSelection();
  if (!selection.entityId) {
    state.inputEditSelectionRestore = null;
    return null;
  }
  const panelView = state.activeResultTab || "metrics";
  const adapter = getPanelNavigationAdapter(panelView);
  state.inputEditSelectionRestore = {
    selection,
    parentEntityId: selection.relatedEntityIds?.[0] || "",
    objectCount: state.report?.objects?.length || 0,
    panelView,
    panelContext: adapter?.captureContext?.() || null,
  };
  return state.inputEditSelectionRestore;
}

async function restoreInputSelectionAfterAnalysis() {
  const restore = state.inputEditSelectionRestore;
  if (!restore) {
    return;
  }
  state.inputEditSelectionRestore = null;
  const adapter = getPanelNavigationAdapter(restore.panelView);
  await adapter?.restoreContext?.(restore.panelContext || {});
}

function renderFormattedTextView() {
  const report = state.report;
  if (!report || !Array.isArray(report.objects) || !hasCurrentAnalysis()) {
    elements.textObjectView.innerHTML = `<div class="empty">${escapeHTML(pendingViewMessage("formatted text"))}</div>`;
    return;
  }

  const groups = groupedReportObjects();
  elements.textObjectView.innerHTML = `
    ${
      groups.length
        ? `<div class="json-groups">
            ${groups
              .map(
                (group) => `
                  <details class="json-group" data-object-type="${escapeHTML(group.type)}" open>
                    <summary>
                      <span>${escapeHTML(group.type)}</span>
                    </summary>
                    ${group.objects.map(renderFormattedObject).join("")}
                  </details>`,
              )
              .join("")}
          </div>`
        : `<div class="empty">${t("input.noMatchingObjects")}</div>`
    }
  `;
  bindFormattedTextControls();
}

function renderJSONView() {
  const model = state.model;
  if (!model || !Array.isArray(model.objects) || !hasCurrentAnalysis()) {
    elements.jsonStructuredView.innerHTML = `<div class="empty">${escapeHTML(pendingViewMessage("JSON"))}</div>`;
    return;
  }

  const visibleObjects = filterInputObjects(model.objects);

  elements.jsonStructuredView.innerHTML = `
    <div class="json-tree primary-tree json-object-tree">${renderJSONObjectsTree(visibleObjects)}</div>
  `;
  bindJSONEditorControls();
}

function renderJSONObjectsTree(objects) {
  if (!objects.length) {
    return state.inputFilterQuery.trim() ? `<div class="empty">${t("input.noMatchingObjects")}</div>` : `<div class="empty">${t("input.noObjects")}</div>`;
  }
  const groups = groupObjectsByType(objects);
  if (!groups.length) {
    return `<div class="empty">${t("input.noMatchingObjects")}</div>`;
  }

  return `
    <div class="json-root-line">{</div>
    ${groups.map((group, index) => renderJSONTypeGroup(group, index === groups.length - 1)).join("")}
    <div class="json-root-line">}</div>
  `;
}

function renderJSONTypeGroup(group, isLastGroup) {
  return `
    <details class="json-node json-type-group" data-object-type="${escapeHTML(group.type)}" open>
      <summary>
        <span class="json-line"><span class="json-key">${formatJSONKey(group.type)}</span><span class="json-colon">: </span><span class="json-brace">{</span></span>
      </summary>
      <div class="json-children">
        ${group.objects.map((object, index) => renderJSONInstance(object, index === group.objects.length - 1)).join("")}
      </div>
      <div class="json-close-line">}${isLastGroup ? "" : ","}</div>
    </details>
  `;
}

function renderJSONInstance(object, isLastObject) {
  const fields = object.fields || [];
  const objectType = object.type || "Object";
  const sourceIndex = object.sourceIndex ?? object.index ?? "";
  const fallbackOrdinal = Number.isFinite(Number(sourceIndex)) ? Number(sourceIndex) + 1 : 1;
  const objectName = object.name || `${objectType} ${fallbackOrdinal}`;
  const sourceLabel = sourceIndex === "" ? "" : `<span class="row-sub">#${escapeHTML(sourceIndex)}</span>`;
  const selected = String(sourceIndex) === String(state.jsonSelectedObjectIndex);
  return `
    <details class="json-node json-instance ${selected ? "selected" : ""}" data-object-index="${escapeHTML(sourceIndex)}" data-object-type="${escapeHTML(objectType)}" open>
      <summary class="json-object-summary" data-json-object-index="${escapeHTML(sourceIndex)}" data-object-index="${escapeHTML(sourceIndex)}" data-object-type="${escapeHTML(objectType)}">
        <span class="json-line" title="${escapeHTML(objectName)}"><span class="json-key">${formatJSONKey(objectName)}</span><span class="json-colon">: </span><span class="json-brace">{</span></span>
        <span class="json-summary-meta">
          ${sourceLabel}
          ${renderInputJumpControls({ objectIndex: sourceIndex })}
          <span class="badge">${escapeHTML(t("count.fields", { count: fields.length }))}</span>
        </span>
      </summary>
      <div class="json-fields">
        ${fields
          .map((field, index) => renderJSONFieldRow(field, sourceIndex, index, index === fields.length - 1))
          .join("")}
      </div>
      <div class="json-close-line">}${isLastObject ? "" : ","}</div>
    </details>
  `;
}

function renderJSONFieldRow(field, objectIndex, fieldIndex, isLastField) {
  const key = field.key || field.comment || `field_${fieldIndex + 1}`;
  return `
    <div class="json-field-row" data-object-index="${escapeHTML(objectIndex)}" data-field-index="${escapeHTML(fieldIndex)}" data-field-index-kind="model">
      <span class="json-key" title="${escapeHTML(field.comment || key)}">${formatJSONKey(key)}</span>
      <span class="json-colon">: </span>
      <span class="json-field-value">
        ${renderJSONEditorValue(field.value, { objectIndex, fieldIndex, fieldIndexKind: "model", path: [] }, 0, !isLastField)}
        ${renderInputJumpControls({ objectIndex, fieldIndex, fieldIndexKind: "model", value: formatJSONValue(field.value) })}
      </span>
    </div>
  `;
}

function bindJSONEditorControls() {
  if (jsonEditorControlsBound) {
    return;
  }
  elements.jsonStructuredView.addEventListener("click", handleJSONEditorClick);
  jsonEditorControlsBound = true;
}

function handleJSONEditorClick(event) {
  const target = event.target instanceof Element ? event.target : null;
  if (!target) {
    return;
  }
  const valueButton = target.closest(".json-value-token");
  if (valueButton) {
    editJSONValueToken(valueButton);
    return;
  }
  const summary = target.closest(".json-object-summary");
  if (summary) {
    state.jsonSelectedObjectIndex = summary.dataset.jsonObjectIndex || "";
  }
}

async function editJSONValueToken(button) {
  const currentRaw = button.dataset.rawValue || "null";
  const editor = document.createElement("input");
  editor.type = "text";
  editor.className = "json-value-editor";
  editor.value = currentRaw;
  editor.dataset.objectIndex = button.dataset.objectIndex;
  editor.dataset.fieldIndex = button.dataset.fieldIndex;
  editor.dataset.jsonPath = button.dataset.jsonPath || "[]";
  editor.dataset.rawValue = currentRaw;
  editor.setAttribute("aria-label", "JSON value");
  editor.style.width = `${Math.min(Math.max(currentRaw.length + 2, 8), 56)}ch`;

  button.replaceWith(editor);
  editor.focus();
  editor.select();

  let finished = false;
  const restore = () => {
    if (editor.isConnected) {
      editor.replaceWith(button);
    }
  };
  const commit = async () => {
    if (finished) {
      return;
    }
    finished = true;
    const nextRaw = editor.value.trim();
    if (nextRaw === currentRaw) {
      restore();
      return;
    }
    editor.disabled = true;
    await commitJSONValueEdit(editor, nextRaw, restore);
  };

  editor.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      commit();
    }
    if (event.key === "Escape") {
      event.preventDefault();
      finished = true;
      restore();
    }
  });
  editor.addEventListener("blur", () => commit());
}

async function commitJSONValueEdit(editor, nextRaw, restore) {
  const currentRaw = editor.dataset.rawValue || "null";
  if (nextRaw === currentRaw) {
    restore();
    return;
  }

  const api = backend();
  if (!api || typeof api.PatchModelValueText !== "function") {
    setStatus(localizedMessage("status.backendUnavailable"), "warn");
    restore();
    return;
  }

  try {
    captureInputEditSelection();
    const result = await api.PatchModelValueText(
      getDocumentText(),
      Number(editor.dataset.objectIndex),
      Number(editor.dataset.fieldIndex),
      JSON.parse(editor.dataset.jsonPath || "[]"),
      nextRaw,
    );
    setDocumentText(result.text);
    state.report = result.report;
    state.model = result.model || null;
    state.epjsonText = result.epjson || "";
    state.semanticProjection = retainSemanticNavigationProjection(result.semantic);
    state.lastAnalyzedText = result.text;
    state.analysisKey = result.analysisKey || "";
    state.lastAnalyzedKey = state.analysisKey;
    state.reportAnalyzedText = result.text;
    state.reportAnalysisKey = state.analysisKey;
    state.analysisStage = "complete";
    state.geometryReady = true;
    Object.keys(state.analysisReady || {}).forEach((view) => {
      state.analysisReady[view] = true;
    });
    state.reportAnalysisStage = state.analysisStage;
    state.reportAnalysisReady = { ...(state.analysisReady || {}) };
    state.reportGeometryReady = true;
    window.dispatchEvent(new Event("idfAnalyzer:documentChanged"));
    renderReportCallback();
    window.dispatchEvent(new CustomEvent("idfAnalyzer:analysisComplete", {
      detail: { text: result.text, analysisKey: state.reportAnalysisKey, stage: "complete" },
    }));
    setStatus(localizedMessage("input.jsonValueUpdated"), "ok");
  } catch (error) {
    setStatus(error.message || String(error), "error");
    restore();
  }
}

function renderFormattedObject(object) {
  const fields = object.fields || [];
  const objectIndex = object.index ?? object.sourceIndex ?? "";
  const objectName = object.name || "";
  const primaryLabel = objectName || object.type || `#${objectIndex}`;
  const secondaryLabel = objectName ? object.type || "" : "";
  return `
    <section class="json-object text-object" data-object-index="${escapeHTML(objectIndex)}" data-object-type="${escapeHTML(object.type || "")}">
      <div class="json-object-head text-object-head" data-object-index="${escapeHTML(objectIndex)}" data-object-type="${escapeHTML(object.type || "")}">
        <strong title="${escapeHTML(primaryLabel)}">${escapeHTML(primaryLabel)}</strong>
        <span class="row-sub">${secondaryLabel ? `${escapeHTML(secondaryLabel)} ` : ""}#${escapeHTML(objectIndex)} ${renderInputJumpControls({ objectIndex })}</span>
      </div>
      <dl>
        ${fields.map((field, fieldIndex) => renderFormattedTextField(field, objectIndex, fieldIndex)).join("")}
      </dl>
    </section>
  `;
}

function renderFormattedTextField(field, objectIndex, fieldIndex) {
  const label = field.comment || field.key || `Field ${fieldIndex + 1}`;
  const value = formatJSONValue(field.value);
  return `
    <dt title="${escapeHTML(label)}" data-object-index="${escapeHTML(objectIndex)}" data-field-index="${escapeHTML(fieldIndex)}">${escapeHTML(label)}</dt>
    <dd class="text-field-cell" title="${escapeHTML(label)}" data-object-index="${escapeHTML(objectIndex)}" data-field-index="${escapeHTML(fieldIndex)}">
      <span class="field-input-wrap">
        <input class="text-field-input"
          data-object-index="${escapeHTML(objectIndex)}"
          data-field-index="${escapeHTML(fieldIndex)}"
          data-field-index-kind="idf"
          data-original="${escapeHTML(value)}"
          list="${escapeHTML(fieldSuggestionListID(objectIndex, fieldIndex))}"
          value="${escapeHTML(value)}" />
        ${renderInputJumpControls({ objectIndex, fieldIndex, fieldIndexKind: "idf", value })}
      </span>
    </dd>`;
}

function bindFormattedTextControls() {
  if (formattedTextControlsBound) {
    return;
  }
  bindDelegatedFieldEditor(elements.textObjectView, ".text-field-input", applyTextValue);
  formattedTextControlsBound = true;
}

function bindDelegatedFieldEditor(host, selector, applyValue) {
  host.addEventListener("focusin", (event) => {
    const input = delegatedFieldInput(event, selector);
    if (!input) {
      return;
    }
    loadFieldSuggestions(input);
  });
  host.addEventListener("focusout", (event) => {
    const input = delegatedFieldInput(event, selector);
    if (input) {
      applyValue(input);
    }
  });
  host.addEventListener("keydown", (event) => {
    const input = delegatedFieldInput(event, selector);
    if (!input || (event.key !== "Enter" && event.key !== "Escape")) {
      return;
    }
    event.preventDefault();
    if (event.key === "Escape") {
      input.value = input.dataset.original || "";
    }
    input.blur();
  });
}

function delegatedFieldInput(event, selector) {
  const target = event.target instanceof Element ? event.target : null;
  return target?.matches(selector) ? target : null;
}

function fieldSuggestionListID(objectIndex, fieldIndex) {
  return `fieldSuggestions-${objectIndex}-${fieldIndex}`;
}

function renderInputJumpControls(context) {
  const definitionCount = resolveInputJumpTargets("definition", context).length;
  const referenceCount = resolveInputJumpTargets("references", context).length;
  if (!definitionCount && !referenceCount) {
    return "";
  }
  return `
    <span class="input-jump-tools" aria-label="${escapeHTML(t("input.jumpTools"))}">
      ${
        definitionCount
          ? `<button type="button" data-input-jump-kind="definition" data-object-index="${escapeHTML(context.objectIndex)}" data-field-index="${escapeHTML(context.fieldIndex ?? "")}" data-field-index-kind="${escapeHTML(context.fieldIndexKind || "idf")}" title="${escapeHTML(t("input.jumpDefinition"))}">${escapeHTML(t("input.jumpDefinitionShort"))}</button>`
          : ""
      }
      ${
        referenceCount
          ? `<button type="button" data-input-jump-kind="references" data-object-index="${escapeHTML(context.objectIndex)}" data-field-index="${escapeHTML(context.fieldIndex ?? "")}" data-field-index-kind="${escapeHTML(context.fieldIndexKind || "idf")}" title="${escapeHTML(t("input.jumpReferences"))}">${escapeHTML(t("input.jumpReferencesShort", { count: referenceCount }))}</button>`
          : ""
      }
    </span>`;
}

export function currentInputJumpSource() {
  const element = document.activeElement?.closest?.("[data-object-index]");
  if (!element) {
    return null;
  }
  return jumpSourceForContext({
    objectIndex: element.dataset.objectIndex,
    fieldIndex: element.dataset.fieldIndex === undefined || element.dataset.fieldIndex === "" ? null : Number(element.dataset.fieldIndex),
    fieldIndexKind: element.dataset.fieldIndexKind || "idf",
    value: element.value,
  });
}

export function jumpSourceForContext(context = {}) {
  const objectIndex = Number(context.objectIndex);
  if (!Number.isFinite(objectIndex)) {
    return null;
  }
  const fieldIndex = context.fieldIndex === undefined || context.fieldIndex === null || context.fieldIndex === "" ? null : Number(context.fieldIndex);
  const fieldIndexKind = context.fieldIndexKind || "idf";
  const object = reportObjectByIndex(objectIndex);
  const modelObject = modelObjectByIndex(objectIndex);
  let field = null;
  if (fieldIndex !== null && Number.isFinite(fieldIndex)) {
    if (fieldIndexKind === "model") {
      field = (modelObject?.fields || [])[fieldIndex] || null;
    } else {
      field = (object?.fields || [])[fieldIndex] || null;
    }
  }
  return {
    objectIndex,
    objectType: object?.type || modelObject?.type || "",
    objectName: object?.name || modelObject?.name || "",
    fieldIndex: fieldIndex === null || !Number.isFinite(fieldIndex) ? null : fieldIndex,
    fieldIndexKind,
    fieldLabel: field?.comment || field?.key || "",
    value: context.value !== undefined ? String(context.value || "") : field ? formatJSONValue(field.value) : object?.name || modelObject?.name || "",
  };
}

export function resolveInputJumpTargets(kind, context = currentInputJumpSource()) {
  const source = context?.objectName === undefined ? jumpSourceForContext(context) : context;
  if (!source) {
    return [];
  }
  const targetName = normalizeReferenceName(source.fieldIndex === null ? source.objectName : source.value);
  if (!targetName) {
    return [];
  }
  if (kind === "definition") {
    return definitionTargetsForName(targetName, source);
  }
  if (kind === "references") {
    return referenceTargetsForName(targetName, source);
  }
  return [];
}

function definitionTargetsForName(name, source) {
  if (source.fieldIndex === null || source.fieldIndex === undefined) {
    return [];
  }
  const matches = jumpIndex().definitions.get(name) || [];
  if (!matches.length) {
    return [];
  }
  const preferred = preferredDefinitionTarget(matches, source);
  return preferred ? [{ objectIndex: preferred.index, objectType: preferred.type }] : [];
}

function preferredDefinitionTarget(matches, source) {
  const nonCurrent = matches.filter((object) => Number(object.index) !== Number(source.objectIndex));
  const candidates = nonCurrent.length ? nonCurrent : matches;
  const label = String(source.fieldLabel || "").toLowerCase();
  const typeHints = [
    ["schedule", (type) => type.toLowerCase().startsWith("schedule:")],
    ["construction", (type) => type.toLowerCase().startsWith("construction")],
    ["material", (type) => type.toLowerCase().includes("material")],
    ["zone", (type) => ["zone", "zonelist", "space", "spacelist"].includes(type.toLowerCase())],
    ["curve", (type) => type.toLowerCase().startsWith("curve:")],
    ["node", (type) => type.toLowerCase().includes("nodelist")],
  ];
  for (const [hint, predicate] of typeHints) {
    if (label.includes(hint)) {
      const match = candidates.find((object) => predicate(object.type || ""));
      if (match) {
        return match;
      }
    }
  }
  return candidates[0] || null;
}

function referenceTargetsForName(name, source) {
  return (jumpIndex().references.get(name) || []).filter(
    (target) => !(Number(target.objectIndex) === Number(source.objectIndex) && Number(target.fieldIndex) === Number(source.fieldIndex)),
  );
}

function jumpIndex() {
  const report = state.report;
  if (jumpIndexCache.report === report) {
    return jumpIndexCache;
  }
  const definitions = new Map();
  const references = new Map();
  (report?.objects || []).forEach((object) => {
    const objectName = normalizeReferenceName(object.name);
    if (objectName) {
      if (!definitions.has(objectName)) {
        definitions.set(objectName, []);
      }
      definitions.get(objectName).push(object);
    }
    (object.fields || []).forEach((field, fieldIndex) => {
      const fieldName = normalizeReferenceName(formatJSONValue(field.value));
      if (!fieldName) {
        return;
      }
      if (!references.has(fieldName)) {
        references.set(fieldName, []);
      }
      references.get(fieldName).push({ objectIndex: object.index, objectType: object.type, fieldIndex, fieldIndexKind: "idf" });
    });
  });
  jumpIndexCache = { report, definitions, references };
  return jumpIndexCache;
}

function normalizeReferenceName(value) {
  const text = String(value || "").trim();
  if (!text || /^[-+]?\d+(\.\d+)?$/.test(text)) {
    return "";
  }
  return text.toLowerCase();
}

async function loadFieldSuggestions(input) {
  if (input.dataset.suggestionsLoaded === "true" || input.dataset.suggestionsLoading === "true") {
    return;
  }
  const objectIndex = Number(input.dataset.objectIndex);
  const fieldIndex = Number(input.dataset.fieldIndex);
  if (!Number.isFinite(objectIndex) || !Number.isFinite(fieldIndex)) {
    return;
  }

  input.dataset.suggestionsLoading = "true";
  try {
    const suggestions = await requestFieldSuggestions(objectIndex, fieldIndex);
    input.dataset.suggestionsLoaded = "true";
    attachFieldSuggestionList(input, suggestions);
  } catch (error) {
    console.debug(t("input.fieldSuggestionsUnavailable", {}, "Field suggestions unavailable"), error);
  } finally {
    delete input.dataset.suggestionsLoading;
  }
}

async function requestFieldSuggestions(objectIndex, fieldIndex) {
  const api = backend();
  if (api && typeof api.SuggestFieldValuesText === "function") {
    return api.SuggestFieldValuesText(getDocumentText(), objectIndex, fieldIndex);
  }

  const response = await fetch("/api/field-suggestions", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text: getDocumentText(), objectIndex, fieldIndex }),
  });
  if (!response.ok) {
    throw new Error(await response.text());
  }
  return response.json();
}

function attachFieldSuggestionList(input, suggestions) {
  if (!Array.isArray(suggestions) || suggestions.length === 0) {
    return;
  }
  const listID = input.getAttribute("list") || fieldSuggestionListID(input.dataset.objectIndex, input.dataset.fieldIndex);
  let datalist = document.getElementById(listID);
  if (!datalist) {
    datalist = document.createElement("datalist");
    datalist.id = listID;
    document.body.appendChild(datalist);
  }
  datalist.innerHTML = suggestions
    .map((suggestion) => {
      const labelParts = [suggestion.label, suggestion.source].filter(Boolean);
      const label = labelParts.length ? ` label="${escapeHTML(labelParts.join(" / "))}"` : "";
      return `<option value="${escapeHTML(suggestion.value || "")}"${label}></option>`;
    })
    .join("");
}

async function applyTextValue(input) {
  await applyFieldValue(input, localizedMessage("input.textFieldUpdated"));
}

async function applyFieldValue(input, successMessage = localizedMessage("input.fieldUpdated")) {
  const nextValue = input.value;
  if (nextValue === input.dataset.original || input.dataset.committing === "true") {
    return;
  }

  const api = backend();
  if (!api || typeof api.UpdateFieldText !== "function") {
    setStatus(localizedMessage("status.backendUnavailable"), "warn");
    input.value = input.dataset.original || "";
    return;
  }

  const objectIndex = Number(input.dataset.objectIndex);
  const fieldIndex = Number(input.dataset.fieldIndex);
  input.dataset.committing = "true";
  input.disabled = true;

  try {
    captureInputEditSelection();
    const result = await api.UpdateFieldText(getDocumentText(), objectIndex, fieldIndex, nextValue);
    setDocumentText(result.text);
    await analyzeCallback();
    setStatus(successMessage, "ok");
  } catch (error) {
    input.value = input.dataset.original || "";
    input.disabled = false;
    delete input.dataset.committing;
    setStatus(error.message || String(error), "error");
  }
}

function highlightFormattedTextTarget(element) {
  element.classList.remove("input-jump-highlight");
  void element.offsetWidth;
  element.classList.add("input-jump-highlight");
  window.setTimeout(() => element.classList.remove("input-jump-highlight"), 1200);
}

function reportObjectByIndex(objectIndex) {
  return state.report?.objects?.find((object) => Number(object.index) === Number(objectIndex)) || null;
}

function modelObjectByIndex(objectIndex) {
  return (
    state.model?.objects?.find((object, index) => Number(object.sourceIndex) === Number(objectIndex) || index === Number(objectIndex)) || null
  );
}

function objectHasIDFNameField(objectIndex) {
  const reportObject = reportObjectByIndex(objectIndex);
  return Boolean(reportObject?.fields?.[0]?.comment === "Name" && reportObject.fields[0].value);
}

function modelFieldIndexToIDFFieldIndex(objectIndex, modelFieldIndex) {
  if (modelFieldIndex === null || modelFieldIndex === undefined) {
    return null;
  }
  return objectHasIDFNameField(objectIndex) ? modelFieldIndex + 1 : modelFieldIndex;
}

function idfFieldIndexToModelFieldIndex(objectIndex, idfFieldIndex) {
  if (idfFieldIndex === null || idfFieldIndex === undefined) {
    return null;
  }
  if (!objectHasIDFNameField(objectIndex)) {
    return idfFieldIndex;
  }
  return idfFieldIndex === 0 ? null : idfFieldIndex - 1;
}

function formatJSONValue(value) {
  if (value === null || value === undefined) {
    return "";
  }
  if (typeof value === "object") {
    return JSON.stringify(value);
  }
  return String(value);
}

function formatJSONKey(value) {
  return escapeHTML(JSON.stringify(String(value ?? "")));
}

function formatJSONLiteral(value) {
  if (value === undefined) {
    return "null";
  }
  try {
    const encoded = JSON.stringify(value);
    return encoded === undefined ? "null" : encoded;
  } catch (_) {
    return JSON.stringify(formatJSONValue(value));
  }
}

function renderJSONReadonlyValue(value, depth = 0, trailingComma = false) {
  const comma = trailingComma ? "," : "";
  if (Array.isArray(value)) {
    if (!value.length) {
      return `<span class="json-primitive">[]${comma}</span>`;
    }
    return `
      <details class="json-node json-value-node" open>
        <summary><span class="json-brace">[</span> <span class="badge">${escapeHTML(value.length)}</span></summary>
        <div class="json-children">
          ${value
            .map(
              (item, index) =>
                `<div class="json-array-row"><span class="json-index">${escapeHTML(index)}</span>${renderJSONReadonlyValue(item, depth + 1, index !== value.length - 1)}</div>`,
            )
            .join("")}
        </div>
        <div class="json-close-line">]${comma}</div>
      </details>`;
  }

  if (value && typeof value === "object") {
    const entries = Object.entries(value);
    if (!entries.length) {
      return `<span class="json-primitive">{}${comma}</span>`;
    }
    return `
      <details class="json-node json-value-node" open>
        <summary><span class="json-brace">{</span> <span class="badge">${escapeHTML(entries.length)}</span></summary>
        <div class="json-children">
          ${entries
            .map(
              ([key, child], index) => `
                <div class="json-field-row">
                  <span class="json-key">${formatJSONKey(key)}</span>
                  <span class="json-colon">: </span>
                  <span class="json-field-value">${renderJSONReadonlyValue(child, depth + 1, index !== entries.length - 1)}</span>
                </div>`,
            )
            .join("")}
        </div>
        <div class="json-close-line">}${comma}</div>
      </details>`;
  }

  return `<span class="json-primitive">${escapeHTML(formatJSONLiteral(value))}${comma}</span>`;
}

function renderJSONEditorValue(value, context, depth = 0, trailingComma = false) {
  const comma = trailingComma ? "," : "";
  if (Array.isArray(value)) {
    if (!value.length) {
      return `<span class="json-primitive">[]</span><span class="json-comma">${comma}</span>`;
    }
    return `
      <details class="json-node json-value-node" open>
        <summary><span class="json-brace">[</span> <span class="badge">${escapeHTML(value.length)}</span></summary>
        <div class="json-children">
          ${value
            .map((item, index) => {
              const childContext = { ...context, path: [...context.path, String(index)] };
              return `<div class="json-array-row"><span class="json-index">${escapeHTML(index)}</span>${renderJSONEditorValue(item, childContext, depth + 1, index !== value.length - 1)}</div>`;
            })
            .join("")}
        </div>
        <div class="json-close-line">]</div><span class="json-comma">${comma}</span>
      </details>`;
  }

  if (value && typeof value === "object") {
    const entries = Object.entries(value);
    if (!entries.length) {
      return `<span class="json-primitive">{}</span><span class="json-comma">${comma}</span>`;
    }
    return `
      <details class="json-node json-value-node" open>
        <summary><span class="json-brace">{</span> <span class="badge">${escapeHTML(entries.length)}</span></summary>
        <div class="json-children">
          ${entries
            .map(([key, child], index) => {
              const childContext = { ...context, path: [...context.path, key] };
              return `
                <div class="json-field-row">
                  <span class="json-key">${formatJSONKey(key)}</span>
                  <span class="json-colon">: </span>
                  <span class="json-field-value">${renderJSONEditorValue(child, childContext, depth + 1, index !== entries.length - 1)}</span>
                </div>`;
            })
            .join("")}
        </div>
        <div class="json-close-line">}</div><span class="json-comma">${comma}</span>
      </details>`;
  }

  const rawValue = formatJSONLiteral(value);
  return `
    <button class="json-value-token" type="button"
      data-object-index="${escapeHTML(context.objectIndex)}"
      data-field-index="${escapeHTML(context.fieldIndex)}"
      data-field-index-kind="${escapeHTML(context.fieldIndexKind || "idf")}"
      data-json-path="${escapeHTML(JSON.stringify(context.path))}"
      data-raw-value="${escapeHTML(rawValue)}">${escapeHTML(rawValue)}</button><span class="json-comma">${comma}</span>`;
}

export function renderFieldTable() {
  bindFieldTableControls();
  const report = state.report;
  if (!report || !Array.isArray(report.objects) || !hasCurrentAnalysis()) {
    elements.fieldTable.innerHTML = `<div class="empty">${escapeHTML(pendingViewMessage("table"))}</div>`;
    return;
  }

  const groups = groupObjectsByType(filterInputObjects(report.objects));
  const objectCount = groups.reduce((sum, group) => sum + group.objects.length, 0);
  if (!groups.length) {
    elements.fieldTable.innerHTML = `<div class="empty">${t("input.noMatchingTables")}</div>`;
    return;
  }

  const limitedGroups = limitObjectGroups(groups, FIELD_TABLE_RENDER_LIMIT);
  const hiddenCount = Math.max(0, objectCount - limitedGroups.reduce((sum, group) => sum + group.objects.length, 0));
  elements.fieldTable.innerHTML = `
    ${hiddenCount ? `<div class="empty compact">${escapeHTML(`${hiddenCount} additional objects hidden. Narrow the filter to render them.`)}</div>` : ""}
    ${limitedGroups.map((group, index) => renderObjectTypeTable(group, index)).join("")}
  `;

}

function bindFieldTableControls() {
  if (fieldTableControlsBound) {
    return;
  }
  elements.fieldTable.addEventListener("click", handleFieldTableClick);
  bindDelegatedFieldEditor(elements.fieldTable, ".field-value-input", applyTableValue);
  fieldTableControlsBound = true;
}

function handleFieldTableClick(event) {
  const target = event.target instanceof Element ? event.target : null;
  if (!target) {
    return;
  }
  const orientationButton = target.closest(".object-orientation-button");
  if (orientationButton) {
    event.preventDefault();
    event.stopPropagation();
    state.tableGroupOrientations.set(orientationButton.dataset.objectType, orientationButton.dataset.nextOrientation);
    renderFieldTable();
  }
}

function limitObjectGroups(groups, limit) {
  let remaining = limit;
  const limited = [];
  for (const group of groups) {
    if (remaining <= 0) {
      break;
    }
    const objects = group.objects.slice(0, remaining);
    if (objects.length) {
      limited.push({ ...group, objects });
      remaining -= objects.length;
    }
  }
  const selected = selectedInputObjectIndex();
  if (selected !== null && !limited.some((group) => group.objects.some((object) => inputObjectIndex(object) === selected))) {
    const sourceGroup = groups.find((group) => group.objects.some((object) => inputObjectIndex(object) === selected));
    const object = sourceGroup?.objects.find((item) => inputObjectIndex(item) === selected);
    if (object) {
      const targetGroup = limited.find((group) => group.type === sourceGroup.type);
      if (targetGroup) targetGroup.objects.push(object);
      else limited.push({ ...sourceGroup, objects: [object] });
      materializedInputSource = selected;
    }
  }
  return limited;
}

function renderObjectTypeTable(group, groupIndex) {
  const orientation = state.tableGroupOrientations.get(group.type) || state.tableOrientation;
  const columns = buildObjectTypeColumns(group.objects);
  const nextOrientation = orientation === "objects" ? "fields" : "objects";
  return `
    <details class="object-table-group" data-object-type="${escapeHTML(group.type)}" open>
      <summary>
        <span>${escapeHTML(group.type)}</span>
        <span class="object-table-actions">
          <button class="object-orientation-button" data-object-type="${escapeHTML(group.type)}" data-next-orientation="${escapeHTML(nextOrientation)}" type="button">
            ${orientation === "objects" ? t("input.fieldsRows") : t("input.objectsRows")}
          </button>
        </span>
      </summary>
      <div class="object-type-table-scroll">
        ${orientation === "objects" ? renderObjectsAsRowsTable(group, columns) : renderFieldsAsRowsTable(group, columns)}
      </div>
    </details>
  `;
}

function renderObjectsAsRowsTable(group, columns) {
  return `
    <table>
      <thead>
        <tr>
          <th class="sticky-col">${t("common.object")}</th>
          ${columns.map((column) => `<th title="${escapeHTML(column.label)}">${escapeHTML(column.label)}</th>`).join("")}
        </tr>
      </thead>
      <tbody>
        ${group.objects
          .map(
            (object) => `
              <tr data-object-index="${escapeHTML(object.index)}" data-object-type="${escapeHTML(object.type)}" data-table-object-index="${escapeHTML(object.index)}">
                <td class="sticky-col" title="${escapeHTML(tableObjectLabel(object))}">${escapeHTML(tableObjectLabel(object))}</td>
                ${columns.map((column) => renderObjectTypeCell(object, column.index)).join("")}
              </tr>`,
          )
          .join("")}
      </tbody>
    </table>
  `;
}

function renderFieldsAsRowsTable(group, columns) {
  return `
    <table>
      <thead>
        <tr>
          <th class="sticky-col">${escapeHTML(t("common.field", {}, "Field"))}</th>
          ${group.objects
            .map(
              (object) => `
                <th title="${escapeHTML(tableObjectLabel(object))}" data-object-index="${escapeHTML(object.index)}" data-object-type="${escapeHTML(object.type)}" data-table-object-index="${escapeHTML(object.index)}">
                  ${escapeHTML(tableObjectLabel(object))}
                </th>`,
            )
            .join("")}
        </tr>
      </thead>
      <tbody>
        ${columns
          .map(
            (column) => `
              <tr>
                <td class="sticky-col" title="${escapeHTML(column.label)}">${escapeHTML(column.label)}</td>
                ${group.objects.map((object) => renderObjectTypeCell(object, column.index)).join("")}
              </tr>`,
          )
          .join("")}
      </tbody>
    </table>
  `;
}

function buildObjectTypeColumns(objects) {
  const maxFields = Math.max(...objects.map((object) => (object.fields || []).length), 0);
  return Array.from({ length: maxFields }, (_, index) => {
    const fieldWithComment = objects
      .map((object) => (object.fields || [])[index])
      .find((field) => field && field.comment);
    return {
      index,
      label: fieldWithComment?.comment || `Field ${index + 1}`,
    };
  });
}

function renderObjectTypeCell(object, fieldIndex) {
  const field = (object.fields || [])[fieldIndex];
  if (!field) {
    return `<td class="empty-cell"></td>`;
  }

  const value = field.value || "";
  const label = field.comment || `Field ${fieldIndex + 1}`;
  return `
    <td title="${escapeHTML(label)}" data-object-index="${escapeHTML(object.index)}" data-object-type="${escapeHTML(object.type)}" data-field-index="${escapeHTML(fieldIndex)}" data-field-index-kind="idf">
      <span class="field-input-wrap table">
        <input class="field-value-input" data-object-index="${escapeHTML(object.index)}"
          data-field-index="${escapeHTML(fieldIndex)}" data-field-index-kind="idf" data-original="${escapeHTML(value)}"
          list="${escapeHTML(fieldSuggestionListID(object.index, fieldIndex))}"
          value="${escapeHTML(value)}" />
        ${renderInputJumpControls({ objectIndex: object.index, fieldIndex, fieldIndexKind: "idf", value })}
      </span>
    </td>`;
}

function tableObjectLabel(object) {
  if (object.name) {
    return `#${object.index} ${object.name}`;
  }
  return `#${object.index} ${object.type || ""}`.trim();
}

async function applyTableValue(input) {
  await applyFieldValue(input, localizedMessage("input.fieldUpdated"));
}

// A default is only an initial preference. Explicit navigation or opening another
// document wins even when the backend settings response arrives afterwards.
export function applyInitialInputView(viewName) {
  if (state.initialInputViewSettled) return;
  state.initialInputViewSettled = true;
  const preferredView = normalizeInputView(viewName);
  return switchInputView(preferredView, { initial: true, recordHistory: false, revealSelection: false });
}

export async function switchInputView(viewName, options = {}) {
  viewName = normalizeInputView(viewName);
  if (!options.initial) state.initialInputViewSettled = true;
  if (options.recordHistory !== false && state.activeInputView !== viewName) {
    recordViewHistory();
  }
  state.activeInputView = viewName;
  elements.inputViewButtons.forEach((button) => {
    const active = button.dataset.inputView === viewName;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", active ? "true" : "false");
    button.tabIndex = active ? 0 : -1;
  });
  elements.inputViews.forEach((view) => {
    const active = view.id === `${viewName}InputView`;
    view.classList.toggle("active", active);
    view.hidden = !active;
  });
  renderInputViews();
  window.requestAnimationFrame(() => {
    window.dispatchEvent(new CustomEvent("idfAnalyzer:inputViewChanged", { detail: { viewName } }));
  });
  if (options.revealSelection === false) {
    return;
  }
  const selection = currentSemanticSelection();
  if (!selection.entityId) {
    return;
  }
  const adapter = getPanelNavigationAdapter(`input-${viewName}`);
  if (adapter && await adapter.canReveal(selection)) {
    await adapter.reveal(selection, {
      action: "view_switch",
      follow: false,
      preserveFilters: true,
      recordHistory: false,
    });
  }
}

export function setTableOrientation(orientation) {
  state.tableOrientation = orientation;
  state.tableGroupOrientations.clear();
  elements.tableOrientationButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.tableOrientation === orientation);
  });
  renderFieldTable();
}
