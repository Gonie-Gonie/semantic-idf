import { backend, elements, escapeHTML, setStatus, state, updateTextStats } from "./state.js";

let analyzeCallback = async () => {};

export function configureInputViews(callbacks) {
  analyzeCallback = callbacks.analyze || analyzeCallback;
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

function groupedObjectsFromModel() {
  const model = state.model;
  if (!model || !Array.isArray(model.objects)) {
    return [];
  }

  const groups = [];
  const byType = new Map();
  model.objects.forEach((object) => {
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
  const model = state.model;
  if (!model || !Array.isArray(model.objects)) {
    elements.textObjectView.innerHTML = `<div class="empty">Analyze input to build formatted text view</div>`;
    return;
  }

  const versionLabel = model.version?.raw || "unknown";
  const groups = groupedObjectsFromModel();
  elements.textObjectView.innerHTML = `
    <div class="json-meta">
      <span class="badge">${escapeHTML(model.format || "unknown")}</span>
      <span class="badge">Version ${escapeHTML(versionLabel)}</span>
      <span class="badge">${escapeHTML(model.objects.length)} objects</span>
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
}

function renderJSONView() {
  const model = state.model;
  if (!model || !Array.isArray(model.objects)) {
    elements.jsonStructuredView.innerHTML = `<div class="empty">Analyze input to build JSON view</div>`;
    elements.jsonTextInput.value = "";
    return;
  }

  const versionLabel = model.version?.raw || "unknown";
  if (document.activeElement !== elements.jsonTextInput) {
    elements.jsonTextInput.value = state.epjsonText || "";
  }

  elements.jsonStructuredView.innerHTML = `
    <div class="json-meta">
      <span class="badge">${escapeHTML(model.format || "unknown")}</span>
      <span class="badge">Version ${escapeHTML(versionLabel)}</span>
      <span class="badge">${escapeHTML(model.objects.length)} objects</span>
    </div>
    <div class="json-tree primary-tree json-object-tree">${renderJSONObjectsTree(model.objects)}</div>
  `;
}

function renderJSONObjectsTree(objects) {
  if (!objects.length) {
    return `<div class="empty">No objects</div>`;
  }
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
  return `
    <details class="json-node json-instance" data-object-index="${escapeHTML(sourceIndex)}" data-object-type="${escapeHTML(objectType)}" open>
      <summary>
        <span class="json-line" title="${escapeHTML(objectName)}"><span class="json-key">${formatJSONKey(objectName)}</span><span class="json-colon">: </span><span class="json-brace">{</span></span>
        <span class="json-summary-meta">
          ${sourceLabel}
          <span class="badge">${escapeHTML(fields.length)} fields</span>
        </span>
      </summary>
      <div class="json-fields">
        ${fields
          .map((field, index) => renderJSONFieldRow(field, index, index === fields.length - 1))
          .join("")}
      </div>
      <div class="json-close-line">}${isLastObject ? "" : ","}</div>
    </details>
  `;
}

function renderJSONFieldRow(field, index, isLastField) {
  const key = field.key || field.comment || `field_${index + 1}`;
  return `
    <div class="json-field-row">
      <span class="json-key" title="${escapeHTML(field.comment || key)}">${formatJSONKey(key)}</span>
      <span class="json-colon">: </span>
      <span class="json-field-value">${renderJSONValue(field.value, 0, !isLastField)}</span>
    </div>
  `;
}

function renderFormattedObject(object) {
  const fields = object.fields || [];
  return `
    <section class="json-object" data-object-index="${escapeHTML(object.sourceIndex ?? "")}" data-object-type="${escapeHTML(object.type || "")}">
      <div class="json-object-head">
        <strong title="${escapeHTML(object.name || "")}">${escapeHTML(object.name || "(unnamed)")}</strong>
        <span class="row-sub">#${escapeHTML(object.sourceIndex ?? "")}</span>
      </div>
      <dl>
        ${fields
          .map(
            (field) => `
              <dt title="${escapeHTML(field.key || field.comment || "")}">${escapeHTML(field.key || field.comment || "field")}</dt>
              <dd>${renderJSONFieldValue(field.value)}</dd>`,
          )
          .join("")}
      </dl>
    </section>
  `;
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
    return `<div class="json-inline-tree">${renderJSONValue(value, 0, false)}</div>`;
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

function renderJSONValue(value, depth = 0, trailingComma = false) {
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
                `<div class="json-array-row"><span class="json-index">${escapeHTML(index)}</span>${renderJSONValue(item, depth + 1, index !== value.length - 1)}</div>`,
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
                  <span class="json-field-value">${renderJSONValue(child, depth + 1, index !== entries.length - 1)}</span>
                </div>`,
            )
            .join("")}
        </div>
        <div class="json-close-line">}${comma}</div>
      </details>`;
  }

  return `<span class="json-primitive">${escapeHTML(formatJSONLiteral(value))}${comma}</span>`;
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
          <th class="sticky-col">Obj</th>
          <th>Name</th>
          ${columns.map((column) => `<th title="${escapeHTML(column.label)}">${escapeHTML(column.label)}</th>`).join("")}
        </tr>
      </thead>
      <tbody>
        ${group.objects
          .map(
            (object) => `
              <tr data-object-index="${escapeHTML(object.index)}" data-object-type="${escapeHTML(object.type)}">
                <td class="sticky-col">#${escapeHTML(object.index)}</td>
                <td title="${escapeHTML(object.name || "")}">${escapeHTML(object.name || "-")}</td>
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
                <th title="${escapeHTML(object.name || "")}" data-object-index="${escapeHTML(object.index)}" data-object-type="${escapeHTML(object.type)}">
                  #${escapeHTML(object.index)} ${escapeHTML(object.name || "-")}
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
        data-field-index="${escapeHTML(fieldIndex)}" data-original="${escapeHTML(value)}"
        value="${escapeHTML(value)}" />
    </td>`;
}

async function applyTableValue(input) {
  const nextValue = input.value;
  if (nextValue === input.dataset.original) {
    return;
  }

  const api = backend();
  if (!api || typeof api.UpdateFieldText !== "function") {
    setStatus("Backend unavailable", "warn");
    input.value = input.dataset.original || "";
    return;
  }

  try {
    const result = await api.UpdateFieldText(
      elements.idfInput.value,
      Number(input.dataset.objectIndex),
      Number(input.dataset.fieldIndex),
      nextValue,
    );
    elements.idfInput.value = result.text;
    updateTextStats();
    await analyzeCallback();
    setStatus("Field updated", "ok");
  } catch (error) {
    input.value = input.dataset.original || "";
    setStatus(error.message || String(error), "error");
  }
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

export async function applyJSONText() {
  elements.idfInput.value = elements.jsonTextInput.value;
  updateTextStats();
  state.lastAnalyzedText = "";
  await analyzeCallback();
  setStatus("JSON applied", "ok");
}

export function setTableOrientation(orientation) {
  state.tableOrientation = orientation;
  state.tableGroupOrientations.clear();
  elements.tableOrientationButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.tableOrientation === orientation);
  });
  renderFieldTable();
}

