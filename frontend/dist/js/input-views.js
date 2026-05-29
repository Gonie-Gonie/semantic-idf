import { backend, elements, escapeHTML, setStatus, state, updateTextStats } from "./state.js";

let analyzeCallback = async () => {};
let renderReportCallback = () => renderInputViews();

export function configureInputViews(callbacks) {
  analyzeCallback = callbacks.analyze || analyzeCallback;
  renderReportCallback = callbacks.renderReport || renderReportCallback;
}

export function renderInputViews() {
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

function groupedReportObjects() {
  const report = state.report;
  if (!report || !Array.isArray(report.objects)) {
    return [];
  }

  const groups = [];
  const byType = new Map();
  report.objects.forEach((object) => {
    if (!byType.has(object.type)) {
      const group = { type: object.type, objects: [] };
      groups.push(group);
      byType.set(object.type, group);
    }
    byType.get(object.type).objects.push(object);
  });
  return groups;
}

function renderFormattedTextView() {
  const report = state.report;
  if (!report || !Array.isArray(report.objects)) {
    elements.textObjectView.innerHTML = `<div class="empty">Analyze input to build formatted text view</div>`;
    return;
  }

  const versionLabel = state.model?.version?.raw || "unknown";
  const formatLabel = state.model?.format || "unknown";
  const groups = groupedReportObjects();
  elements.textObjectView.innerHTML = `
    <div class="json-meta">
      <span class="badge">${escapeHTML(formatLabel)}</span>
      <span class="badge">Version ${escapeHTML(versionLabel)}</span>
      <span class="badge">${escapeHTML(report.objects.length)} objects</span>
      <span class="badge">Editable fields</span>
    </div>
    <div class="json-groups">
      ${groups
        .map(
          (group) => `
            <details class="json-group" data-object-type="${escapeHTML(group.type)}" open>
              <summary>
                <span>${escapeHTML(group.type)}</span>
                <span class="badge">${escapeHTML(group.objects.length)}</span>
              </summary>
              ${group.objects.map(renderFormattedObject).join("")}
            </details>`,
        )
        .join("")}
    </div>
  `;
  bindFormattedTextControls();
}

function renderJSONView() {
  const model = state.model;
  if (!model || !Array.isArray(model.objects)) {
    elements.jsonStructuredView.innerHTML = `<div class="empty">Analyze input to build JSON view</div>`;
    return;
  }

  const versionLabel = model.version?.raw || "unknown";

  elements.jsonStructuredView.innerHTML = `
    <div class="json-meta">
      <span class="badge">${escapeHTML(model.format || "unknown")}</span>
      <span class="badge">Version ${escapeHTML(versionLabel)}</span>
      <span class="badge">${escapeHTML(model.objects.length)} objects</span>
    </div>
    <div class="json-editor-tools">
      <input id="jsonObjectSearch" type="search" placeholder="Search object, field, value" value="${escapeHTML(state.jsonSearchQuery)}" />
      <select id="jsonCollapseDepth" aria-label="JSON collapse depth">
        ${[
          ["1", "Type only"],
          ["2", "Objects"],
          ["3", "Fields"],
          ["99", "Expand all"],
        ]
          .map(
            ([value, label]) =>
              `<option value="${value}" ${String(state.jsonCollapseDepth) === value ? "selected" : ""}>${label}</option>`,
          )
          .join("")}
      </select>
      <button id="jsonFocusObjectButton" type="button">Focus Object</button>
    </div>
    <div class="json-tree primary-tree json-object-tree">${renderJSONObjectsTree(model.objects)}</div>
  `;
  bindJSONEditorControls();
}

function renderJSONObjectsTree(objects) {
  if (!objects.length) {
    return `<div class="empty">No objects</div>`;
  }
  const query = state.jsonSearchQuery.trim().toLowerCase();
  const groups = [];
  const byType = new Map();
  objects.filter((object) => matchesJSONSearch(object, query)).forEach((object) => {
    const objectType = object.type || "Object";
    if (!byType.has(objectType)) {
      const group = { type: objectType, objects: [] };
      groups.push(group);
      byType.set(objectType, group);
    }
    byType.get(objectType).objects.push(object);
  });
  if (!groups.length) {
    return `<div class="empty">No matching objects</div>`;
  }

  return `
    <div class="json-root-line">{</div>
    ${groups.map((group, index) => renderJSONTypeGroup(group, index === groups.length - 1)).join("")}
    <div class="json-root-line">}</div>
  `;
}

function renderJSONTypeGroup(group, isLastGroup) {
  const openAttr = state.jsonCollapseDepth >= 1 ? "open" : "";
  return `
    <details class="json-node json-type-group" data-object-type="${escapeHTML(group.type)}" ${openAttr}>
      <summary>
        <span class="json-line"><span class="json-key">${formatJSONKey(group.type)}</span><span class="json-colon">: </span><span class="json-brace">{</span></span>
        <span class="badge">${escapeHTML(group.objects.length)} objects</span>
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
  const openAttr = state.jsonCollapseDepth >= 2 || selected ? "open" : "";
  return `
    <details class="json-node json-instance ${selected ? "selected" : ""}" data-object-index="${escapeHTML(sourceIndex)}" data-object-type="${escapeHTML(objectType)}" ${openAttr}>
      <summary class="json-object-summary" data-json-object-index="${escapeHTML(sourceIndex)}" data-object-index="${escapeHTML(sourceIndex)}" data-object-type="${escapeHTML(objectType)}">
        <span class="json-line" title="${escapeHTML(objectName)}"><span class="json-key">${formatJSONKey(objectName)}</span><span class="json-colon">: </span><span class="json-brace">{</span></span>
        <span class="json-summary-meta">
          ${sourceLabel}
          <span class="badge">${escapeHTML(fields.length)} fields</span>
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
    <div class="json-field-row">
      <span class="json-key" title="${escapeHTML(field.comment || key)}">${formatJSONKey(key)}</span>
      <span class="json-colon">: </span>
      <span class="json-field-value">${renderJSONEditorValue(field.value, { objectIndex, fieldIndex, fieldIndexKind: "model", path: [] }, 0, !isLastField)}</span>
    </div>
  `;
}

function matchesJSONSearch(object, query) {
  if (!query) {
    return true;
  }
  const fields = object.fields || [];
  const haystack = [
    object.type || "",
    object.name || "",
    object.sourceIndex ?? "",
    ...fields.flatMap((field) => [field.key || "", field.comment || "", formatJSONValue(field.value)]),
  ]
    .join(" ")
    .toLowerCase();
  return haystack.includes(query);
}

function bindJSONEditorControls() {
  const searchInput = elements.jsonStructuredView.querySelector("#jsonObjectSearch");
  const depthSelect = elements.jsonStructuredView.querySelector("#jsonCollapseDepth");
  const focusButton = elements.jsonStructuredView.querySelector("#jsonFocusObjectButton");

  searchInput?.addEventListener("input", () => {
    const caret = searchInput.selectionStart || 0;
    state.jsonSearchQuery = searchInput.value;
    renderJSONView();
    const nextSearchInput = elements.jsonStructuredView.querySelector("#jsonObjectSearch");
    nextSearchInput?.focus();
    nextSearchInput?.setSelectionRange(caret, caret);
  });
  depthSelect?.addEventListener("change", () => {
    state.jsonCollapseDepth = Number(depthSelect.value);
    renderJSONView();
  });
  focusButton?.addEventListener("click", () => focusSelectedJSONObject());

  elements.jsonStructuredView.querySelectorAll(".json-object-summary").forEach((summary) => {
    summary.addEventListener("click", () => {
      state.jsonSelectedObjectIndex = summary.dataset.jsonObjectIndex || "";
      syncRawTextToFormattedTarget(summary);
    });
  });
  elements.jsonStructuredView.querySelectorAll(".json-value-token").forEach((button) => {
    button.addEventListener("click", () => editJSONValueToken(button));
  });
}

function focusSelectedJSONObject() {
  let target = null;
  if (state.jsonSelectedObjectIndex !== "") {
    target = [...elements.jsonStructuredView.querySelectorAll("[data-object-index]")].find(
      (element) => element.dataset.objectIndex === String(state.jsonSelectedObjectIndex),
    );
  }
  if (!target) {
    target = elements.jsonStructuredView.querySelector(".json-instance");
  }
  if (!target) {
    return;
  }
  target.open = true;
  const container = elements.jsonStructuredView.querySelector(".json-tree");
  if (!container) {
    return;
  }
  const containerRect = container.getBoundingClientRect();
  const targetRect = target.getBoundingClientRect();
  container.scrollTo({
    top: Math.max(0, container.scrollTop + targetRect.top - containerRect.top - container.clientHeight * 0.25),
    left: Math.max(0, container.scrollLeft + targetRect.left - containerRect.left - 24),
    behavior: "smooth",
  });
}

async function editJSONValueToken(button) {
  syncRawTextToFormattedTarget(button);
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
    setStatus("Backend patch API unavailable", "warn");
    restore();
    return;
  }

  try {
    const result = await api.PatchModelValueText(
      elements.idfInput.value,
      Number(editor.dataset.objectIndex),
      Number(editor.dataset.fieldIndex),
      JSON.parse(editor.dataset.jsonPath || "[]"),
      nextRaw,
    );
    elements.idfInput.value = result.text;
    updateTextStats();
    state.report = result.report;
    state.model = result.model || null;
    state.epjsonText = result.epjson || "";
    state.lastAnalyzedText = result.text;
    renderReportCallback();
    setStatus("JSON value updated", "ok");
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
        <span class="row-sub">${secondaryLabel ? `${escapeHTML(secondaryLabel)} ` : ""}#${escapeHTML(objectIndex)}</span>
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
      <input class="text-field-input"
        data-object-index="${escapeHTML(objectIndex)}"
        data-field-index="${escapeHTML(fieldIndex)}"
        data-field-index-kind="idf"
        data-original="${escapeHTML(value)}"
        value="${escapeHTML(value)}" />
    </dd>`;
}

function bindFormattedTextControls() {
  elements.syncRawTextToggle.checked = state.syncTextRawPosition;
  elements.textObjectView.querySelectorAll(".text-object-head").forEach((head) => {
    head.addEventListener("click", () => syncRawTextToFormattedTarget(head));
  });
  elements.textObjectView.querySelectorAll(".text-field-input").forEach((input) => {
    input.addEventListener("focus", () => syncRawTextToFormattedTarget(input));
    input.addEventListener("click", () => syncRawTextToFormattedTarget(input));
    input.addEventListener("blur", () => applyTextValue(input));
    input.addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        input.blur();
      }
      if (event.key === "Escape") {
        event.preventDefault();
        input.value = input.dataset.original || "";
        input.blur();
      }
    });
  });
}

async function applyTextValue(input) {
  await applyFieldValue(input, "Text field updated");
}

async function applyFieldValue(input, successMessage = "Field updated") {
  const nextValue = input.value;
  if (nextValue === input.dataset.original || input.dataset.committing === "true") {
    return;
  }

  const api = backend();
  if (!api || typeof api.UpdateFieldText !== "function") {
    setStatus("Backend unavailable", "warn");
    input.value = input.dataset.original || "";
    return;
  }

  const objectIndex = Number(input.dataset.objectIndex);
  const fieldIndex = Number(input.dataset.fieldIndex);
  input.dataset.committing = "true";
  input.disabled = true;

  try {
    const result = await api.UpdateFieldText(elements.idfInput.value, objectIndex, fieldIndex, nextValue);
    elements.idfInput.value = result.text;
    updateTextStats();
    await analyzeCallback();
    if (state.syncTextRawPosition) {
      syncRawTextToObjectField(objectIndex, fieldIndex, input.dataset.fieldIndexKind || "idf");
    }
    setStatus(successMessage, "ok");
  } catch (error) {
    input.value = input.dataset.original || "";
    input.disabled = false;
    delete input.dataset.committing;
    setStatus(error.message || String(error), "error");
  }
}

function syncRawTextToFormattedTarget(element) {
  if (!state.syncTextRawPosition) {
    return;
  }
  const objectIndex = Number(element.dataset.objectIndex);
  const fieldIndex = element.dataset.fieldIndex === undefined ? null : Number(element.dataset.fieldIndex);
  syncRawTextToObjectField(objectIndex, fieldIndex, element.dataset.fieldIndexKind || "idf");
}

function syncRawTextToObjectField(objectIndex, fieldIndex = null, fieldIndexKind = "idf") {
  const range = findRawTextRangeForTextTarget(objectIndex, fieldIndex, fieldIndexKind);
  if (!range) {
    return;
  }
  moveRawTextToRange(range);
}

function findRawTextRangeForTextTarget(objectIndex, fieldIndex = null, fieldIndexKind = "idf") {
  const text = elements.idfInput.value;
  if (!isLikelyJSONText(text)) {
    const idfFieldIndex = fieldIndexKind === "model" ? modelFieldIndexToIDFFieldIndex(objectIndex, fieldIndex) : fieldIndex;
    return findIDFTokenRange(text, objectIndex, idfFieldIndex);
  }
  return findJSONTextRange(text, objectIndex, fieldIndex, fieldIndexKind);
}

function moveRawTextToRange(range) {
  const start = Math.max(0, Math.min(range.start, elements.idfInput.value.length));
  const end = Math.max(start, Math.min(range.end, elements.idfInput.value.length));
  const lineIndex = elements.idfInput.value.slice(0, start).split(/\n/).length - 1;
  const style = window.getComputedStyle(elements.idfInput);
  const fontSize = Number.parseFloat(style.fontSize) || 13;
  const lineHeight = Number.parseFloat(style.lineHeight) || fontSize * 1.5;
  elements.idfInput.scrollTop = Math.max(0, lineIndex * lineHeight - elements.idfInput.clientHeight * 0.25);
  elements.idfInput.setSelectionRange(start, end);
}

function isLikelyJSONText(text) {
  return /^\s*[\[{]/.test(text);
}

function findIDFTokenRange(text, targetObjectIndex, targetFieldIndex = null) {
  return scanIDFTokens(text, (token) => {
    if (token.objectIndex !== targetObjectIndex) {
      return false;
    }
    if (targetFieldIndex === null) {
      return token.type === "object";
    }
    return token.type === "field" && token.fieldIndex === targetFieldIndex;
  });
}

function findIDFTokenAtOffset(text, offset) {
  let nearest = null;
  const exact = scanIDFTokens(text, (token) => {
    if (offset >= token.rawStart && offset <= token.rawEnd) {
      return true;
    }
    if (token.rawStart <= offset) {
      nearest = token;
    }
    return token.rawStart > offset;
  });
  if (exact && offset >= exact.rawStart && offset <= exact.rawEnd) {
    return exact;
  }
  return nearest;
}

function scanIDFTokens(text, visitor) {
  let objectIndex = -1;
  let fieldIndex = -1;
  let inObject = false;
  let inComment = false;
  let tokenStart = 0;

  for (let index = 0; index < text.length; index += 1) {
    const char = text[index];
    if (inComment) {
      if (char === "\n") {
        inComment = false;
      }
      continue;
    }
    if (char === "!") {
      inComment = true;
      continue;
    }
    if (char !== "," && char !== ";") {
      continue;
    }

    const range = trimmedRange(text, tokenStart, index);
    const hasContent = range.end > range.start;
    if (!inObject) {
      if (hasContent) {
        objectIndex += 1;
        fieldIndex = -1;
        inObject = true;
        const token = {
          ...range,
          rawStart: tokenStart,
          rawEnd: index,
          type: "object",
          objectIndex,
          fieldIndex: null,
          fieldIndexKind: "idf",
        };
        if (visitor(token)) {
          return token;
        }
      }
    } else {
      fieldIndex += 1;
      const token = { ...range, rawStart: tokenStart, rawEnd: index, type: "field", objectIndex, fieldIndex, fieldIndexKind: "idf" };
      if (visitor(token)) {
        return token;
      }
    }

    if (char === ";") {
      inObject = false;
    }
    tokenStart = index + 1;
  }
  return null;
}

function trimmedRange(text, start, end) {
  let rangeStart = start;
  let rangeEnd = end;
  while (rangeStart < rangeEnd && /\s/.test(text[rangeStart])) {
    rangeStart += 1;
  }
  while (rangeEnd > rangeStart && /\s/.test(text[rangeEnd - 1])) {
    rangeEnd -= 1;
  }
  return { start: rangeStart, end: rangeEnd };
}

function findJSONTextRange(text, objectIndex, fieldIndex = null, fieldIndexKind = "idf") {
  const reportObject = reportObjectByIndex(objectIndex);
  const modelObject = modelObjectByIndex(objectIndex);
  if (!reportObject && !modelObject) {
    return null;
  }

  const typeNeedle = JSON.stringify(modelObject?.type || reportObject?.type || "");
  const typeOffset = typeNeedle === "\"\"" ? -1 : text.indexOf(typeNeedle);
  const searchStart = typeOffset >= 0 ? typeOffset : 0;
  if (fieldIndex === null) {
    return typeOffset >= 0 ? { start: typeOffset, end: typeOffset + typeNeedle.length } : null;
  }

  const candidates = jsonFieldNeedles(reportObject, modelObject, fieldIndex, fieldIndexKind);
  for (const candidate of candidates) {
    const offset = text.indexOf(candidate, searchStart);
    if (offset >= 0) {
      return { start: offset, end: offset + candidate.length };
    }
  }
  return typeOffset >= 0 ? { start: typeOffset, end: typeOffset + typeNeedle.length } : null;
}

function jsonFieldNeedles(reportObject, modelObject, fieldIndex, fieldIndexKind = "idf") {
  const candidates = [];
  const idfFieldIndex = fieldIndexKind === "model" ? modelFieldIndexToIDFFieldIndex(reportObject?.index, fieldIndex) : fieldIndex;
  const modelFieldIndex = fieldIndexKind === "model" ? fieldIndex : idfFieldIndexToModelFieldIndex(reportObject?.index, fieldIndex);
  const reportField = idfFieldIndex === null ? null : reportObject?.fields?.[idfFieldIndex];
  if (idfFieldIndex === 0 && reportField?.comment === "Name" && reportField.value) {
    candidates.push(JSON.stringify(String(reportField.value)));
  }

  const modelField = modelFieldIndex >= 0 ? modelObject?.fields?.[modelFieldIndex] : null;
  if (modelField?.key) {
    candidates.push(JSON.stringify(modelField.key));
  }
  if (modelField?.value !== undefined && modelField?.value !== null && typeof modelField.value !== "object") {
    candidates.push(JSON.stringify(String(modelField.value)));
  }
  if (reportField?.value) {
    candidates.push(JSON.stringify(String(reportField.value)));
  }
  return [...new Set(candidates)];
}

export function syncTextViewFromRawCaret(event) {
  const rawNavigationKeys = new Set(["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight", "Home", "End", "PageUp", "PageDown"]);
  if (event?.type === "keyup" && !rawNavigationKeys.has(event.key)) {
    return;
  }
  if (!state.syncTextRawPosition) {
    return;
  }
  const text = elements.idfInput.value;
  const token = isLikelyJSONText(text)
    ? findJSONTokenAtOffset(text, elements.idfInput.selectionStart || 0)
    : findIDFTokenAtOffset(text, elements.idfInput.selectionStart || 0);
  if (!token) {
    return;
  }

  const target = findActiveViewTargetForRawToken(token);
  if (!target) {
    return;
  }
  expandDetailsForViewTarget(target);
  const highlightTarget = target.closest("td, th, .text-field-cell, .json-instance, .text-object") || target;
  scrollActiveInputTargetIntoView(highlightTarget);
  highlightFormattedTextTarget(highlightTarget);
}

function findJSONTokenAtOffset(text, offset) {
  const key = nearestJSONStringBeforeOffset(text, offset);
  if (!key) {
    return null;
  }
  for (const object of state.model?.objects || []) {
    const objectIndex = object.sourceIndex ?? object.index;
    if (key === object.name || key === object.type) {
      return { type: "object", objectIndex, fieldIndex: null, fieldIndexKind: "model" };
    }
    const fieldIndex = (object.fields || []).findIndex((field) => field.key === key);
    if (fieldIndex >= 0) {
      return { type: "field", objectIndex, fieldIndex, fieldIndexKind: "model" };
    }
  }
  return null;
}

function nearestJSONStringBeforeOffset(text, offset) {
  const pattern = /"((?:\\.|[^"\\])*)"/g;
  let match = null;
  let current = null;
  while ((match = pattern.exec(text)) && match.index <= offset) {
    current = match[0];
  }
  if (!current) {
    return "";
  }
  try {
    return JSON.parse(current);
  } catch (_) {
    return "";
  }
}

function findActiveViewTargetForRawToken(token) {
  if (state.activeInputView === "json") {
    const modelFieldIndex =
      token.fieldIndexKind === "model" ? token.fieldIndex : idfFieldIndexToModelFieldIndex(token.objectIndex, token.fieldIndex);
    if (token.type === "field" && modelFieldIndex !== null && modelFieldIndex >= 0) {
      return elements.jsonStructuredView.querySelector(
        `.json-value-token[data-object-index="${token.objectIndex}"][data-field-index="${modelFieldIndex}"]`,
      );
    }
    return elements.jsonStructuredView.querySelector(`.json-instance[data-object-index="${token.objectIndex}"]`);
  }
  if (state.activeInputView === "table") {
    const idfFieldIndex =
      token.fieldIndexKind === "model" ? modelFieldIndexToIDFFieldIndex(token.objectIndex, token.fieldIndex) : token.fieldIndex;
    if (token.type === "field" && idfFieldIndex !== null) {
      return elements.fieldTable.querySelector(
        `.field-value-input[data-object-index="${token.objectIndex}"][data-field-index="${idfFieldIndex}"]`,
      );
    }
    return elements.fieldTable.querySelector(`[data-object-index="${token.objectIndex}"]`);
  }
  if (token.type === "field") {
    const idfFieldIndex =
      token.fieldIndexKind === "model" ? modelFieldIndexToIDFFieldIndex(token.objectIndex, token.fieldIndex) : token.fieldIndex;
    return elements.textObjectView.querySelector(
      `.text-field-input[data-object-index="${token.objectIndex}"][data-field-index="${idfFieldIndex}"]`,
    );
  }
  return elements.textObjectView.querySelector(`.text-object[data-object-index="${token.objectIndex}"]`);
}

function expandDetailsForViewTarget(element) {
  let current = element;
  while (current) {
    if (current.tagName && current.tagName.toLowerCase() === "details") {
      current.open = true;
    }
    current = current.parentElement;
  }
}

function scrollActiveInputTargetIntoView(element) {
  const container = element.closest(".formatted-object-view, .json-view, .field-table");
  if (!container) {
    return;
  }
  const containerRect = container.getBoundingClientRect();
  const elementRect = element.getBoundingClientRect();
  container.scrollTo({
    top: Math.max(0, container.scrollTop + elementRect.top - containerRect.top - container.clientHeight * 0.25),
    left: Math.max(0, container.scrollLeft + elementRect.left - containerRect.left - 24),
    behavior: "smooth",
  });
}

function highlightFormattedTextTarget(element) {
  element.classList.remove("input-jump-highlight");
  void element.offsetWidth;
  element.classList.add("input-jump-highlight");
  window.setTimeout(() => element.classList.remove("input-jump-highlight"), 1200);
}

function reportObjectByIndex(objectIndex) {
  return state.report?.objects?.find((object) => object.index === objectIndex) || null;
}

function modelObjectByIndex(objectIndex) {
  return (
    state.model?.objects?.find((object, index) => object.sourceIndex === objectIndex || index === objectIndex) || null
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

function renderJSONFieldValue(value) {
  if (value && typeof value === "object") {
    return `<div class="json-inline-tree">${renderJSONReadonlyValue(value, 0, false)}</div>`;
  }
  return `<span title="${escapeHTML(formatJSONValue(value))}">${escapeHTML(formatJSONValue(value))}</span>`;
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
  const openAttr = depth < 2 ? "open" : "";
  if (Array.isArray(value)) {
    if (!value.length) {
      return `<span class="json-primitive">[]${comma}</span>`;
    }
    return `
      <details class="json-node json-value-node" ${openAttr}>
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
      <details class="json-node json-value-node" ${openAttr}>
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
  const openAttr = state.jsonCollapseDepth >= depth + 3 ? "open" : "";
  if (Array.isArray(value)) {
    if (!value.length) {
      return `<span class="json-primitive">[]</span><span class="json-comma">${comma}</span>`;
    }
    return `
      <details class="json-node json-value-node" ${openAttr}>
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
      <details class="json-node json-value-node" ${openAttr}>
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
  const report = state.report;
  if (!report || !Array.isArray(report.objects)) {
    elements.fieldTable.innerHTML = `<div class="empty">Analyze input to build table view</div>`;
    elements.fieldStats.textContent = "0 tables";
    return;
  }

  const filter = elements.fieldFilter.value.trim().toLowerCase();
  const groups = [];
  const byType = new Map();
  report.objects.forEach((object) => {
    const haystack = [
      object.index,
      object.type,
      object.name || "",
      ...(object.fields || []).flatMap((field) => [field.comment || "", field.value || ""]),
    ]
      .join(" ")
      .toLowerCase();
    if (filter && !haystack.includes(filter)) {
      return;
    }

    if (!byType.has(object.type)) {
      const group = { type: object.type, objects: [] };
      groups.push(group);
      byType.set(object.type, group);
    }
    byType.get(object.type).objects.push(object);
  });

  const objectCount = groups.reduce((sum, group) => sum + group.objects.length, 0);
  const orientationLabel = state.tableOrientation === "fields" ? "fields as rows" : "objects as rows";
  elements.fieldStats.textContent = `${groups.length} tables, ${objectCount} objects, ${orientationLabel}`;
  if (!groups.length) {
    elements.fieldTable.innerHTML = `<div class="empty">No matching object tables</div>`;
    return;
  }

  elements.fieldTable.innerHTML = `
    ${groups.map((group, index) => renderObjectTypeTable(group, index)).join("")}
  `;

  elements.fieldTable.querySelectorAll(".field-value-input").forEach((input) => {
    input.addEventListener("focus", () => syncRawTextToFormattedTarget(input));
    input.addEventListener("click", () => syncRawTextToFormattedTarget(input));
    input.addEventListener("blur", () => applyTableValue(input));
    input.addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        input.blur();
      }
      if (event.key === "Escape") {
        input.value = input.dataset.original || "";
        input.blur();
      }
    });
  });
  elements.fieldTable.querySelectorAll("[data-table-object-index]").forEach((element) => {
    element.addEventListener("click", () => syncRawTextToFormattedTarget(element));
  });
  elements.fieldTable.querySelectorAll(".object-orientation-button").forEach((button) => {
    button.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
      state.tableGroupOrientations.set(button.dataset.objectType, button.dataset.nextOrientation);
      renderFieldTable();
    });
  });
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
            ${orientation === "objects" ? "Fields as rows" : "Objects as rows"}
          </button>
          <span class="badge">${escapeHTML(group.objects.length)} objects</span>
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
          <th class="sticky-col">Object</th>
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
          <th class="sticky-col">Field</th>
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
    <td title="${escapeHTML(label)}" data-object-index="${escapeHTML(object.index)}" data-object-type="${escapeHTML(object.type)}">
      <input class="field-value-input" data-object-index="${escapeHTML(object.index)}"
        data-field-index="${escapeHTML(fieldIndex)}" data-field-index-kind="idf" data-original="${escapeHTML(value)}"
        value="${escapeHTML(value)}" />
    </td>`;
}

function tableObjectLabel(object) {
  if (object.name) {
    return `#${object.index} ${object.name}`;
  }
  return `#${object.index} ${object.type || ""}`.trim();
}

async function applyTableValue(input) {
  await applyFieldValue(input, "Field updated");
}

export async function switchInputView(viewName) {
  state.activeInputView = viewName;
  elements.inputViewButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.inputView === viewName);
  });
  elements.inputViews.forEach((view) => {
    view.classList.toggle("active", view.id === `${viewName}InputView`);
  });

  if (viewName !== "text" && state.lastAnalyzedText !== elements.idfInput.value) {
    await analyzeCallback();
    return;
  }
  renderInputViews();
}

export function setTableOrientation(orientation) {
  state.tableOrientation = orientation;
  state.tableGroupOrientations.clear();
  elements.tableOrientationButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.tableOrientation === orientation);
  });
  renderFieldTable();
}
