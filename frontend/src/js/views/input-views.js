import { backend, elements, escapeHTML, setStatus, state, updateTextStats } from "../state.js";
import { t } from "../i18n.js";
import { recordViewHistory } from "../view-history.js";

const FIELD_TABLE_RENDER_LIMIT = 500;

let analyzeCallback = async () => {};
let renderReportCallback = () => renderInputViews();
let jumpIndexCache = { report: null, definitions: new Map(), references: new Map() };

export function configureInputViews(callbacks) {
  analyzeCallback = callbacks.analyze || analyzeCallback;
  renderReportCallback = callbacks.renderReport || renderReportCallback;
}

export function renderInputViews() {
  if (state.activeInputView === "semantic") {
    renderSemanticView();
  }
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

function setInputFilterStats(matchingObjects, totalObjects) {
  if (!elements.inputFilterStats) {
    return;
  }
  elements.inputFilterStats.textContent = state.inputFilterQuery.trim()
    ? t("count.objectsOf", { shown: matchingObjects, total: totalObjects })
    : t("count.objects", { count: totalObjects });
}

function filterInputObjects(objects) {
  const terms = currentInputFilterTerms();
  return objects.filter((object) => matchesInputFilter(object, terms));
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
  return state.reportAnalyzedText !== "" && state.reportAnalyzedText === elements.idfInput.value;
}

function pendingViewMessage(viewName) {
  if (!elements.idfInput.value.trim()) {
    return t("input.noLoaded");
  }
  return t("input.pendingView", { view: viewName });
}

function renderSemanticView() {
  clearSemanticStickyPathBinding();
  const projection = state.semanticProjection;
  if (!projection || !Array.isArray(projection.lines) || !hasCurrentAnalysis()) {
    elements.semanticEditor.innerHTML = `<div class="empty">${escapeHTML(pendingViewMessage("semantic YAML"))}</div>`;
    setInputFilterStats(0, 0);
    return;
  }

  const terms = currentInputFilterTerms();
  const visibleLines = semanticVisibleLines(projection.lines, terms);
  const visibleObjectIndexes = new Set(
    visibleLines.filter((line) => line.objectIndex !== undefined && line.objectIndex !== null).map((line) => String(line.objectIndex)),
  );
  const keyWidths = semanticKeyWidths(visibleLines);
  setInputFilterStats(visibleObjectIndexes.size || (state.report?.objects?.length || 0), state.report?.objects?.length || 0);

  const sourceNameConflicts = projection.sourceNameConflicts || [];
  const mode = semanticProjectionMode();
  const facet = semanticProjectionFacet();
  elements.semanticEditor.innerHTML = `
    <div class="semantic-toolbar">
      <div class="json-meta">
        <span class="badge">${escapeHTML(projection.schema || "eplus-semantic/0.1")}</span>
        <span class="badge">Version ${escapeHTML(projection.energyplusVersion || "unknown")}</span>
        <span class="badge">${escapeHTML(t("count.objects", { count: projection.objectCount || 0 }))}</span>
        <span class="badge">${escapeHTML(semanticModeLabel(mode))}</span>
        ${facet === "all" ? "" : `<span class="badge">${escapeHTML(semanticFacetLabel(facet))}</span>`}
      </div>
      <div class="semantic-actions">
        <div class="semantic-mode-tabs" role="group" aria-label="Semantic detail level">
          ${["basic", "detailed", "source"].map((item) => `<button class="${item === mode ? "active" : ""}" data-semantic-mode="${item}" type="button">${escapeHTML(semanticModeLabel(item))}</button>`).join("")}
        </div>
        <div class="semantic-filter-tabs" role="group" aria-label="Semantic search facet">
          ${["all", "field", "editable", "derived", "evidence"].map((item) => `<button class="${item === facet ? "active" : ""}" data-semantic-facet="${item}" type="button">${escapeHTML(semanticFacetLabel(item))}</button>`).join("")}
        </div>
        <button id="semanticFocusObjectButton" type="button">${escapeHTML(t("input.focusObject"))}</button>
        <button id="semanticFixDuplicatesButton" type="button" ${sourceNameConflicts.length ? "" : "disabled"}>
          ${escapeHTML(t("semantic.fixSourceNameConflicts", { count: sourceNameConflicts.length }, `Fix source name conflicts (${sourceNameConflicts.length})`))}
        </button>
      </div>
    </div>
    ${renderSemanticWarnings(projection)}
    ${renderSemanticSectionIndex(projection.lines)}
    <div class="semantic-sticky-path" aria-live="polite"></div>
    <div class="semantic-yaml" role="tree" aria-label="Semantic YAML projection">
      ${visibleLines.map((line, index) => renderSemanticLine(line, index, keyWidths)).join("")}
    </div>
  `;
  bindSemanticControls();
}

function semanticKeyWidths(lines) {
  const widths = new Map();
  for (const line of lines) {
    if (!semanticLineHasValue(line)) {
      continue;
    }
    const indent = Number(line.indent || 0);
    const width = Math.min(34, Math.max(8, semanticDisplayKey(line).length));
    widths.set(indent, Math.max(widths.get(indent) || 0, width));
  }
  return widths;
}

function semanticVisibleLines(lines, terms) {
  const mode = semanticProjectionMode();
  const facet = semanticProjectionFacet();
  const compactLines = terms.length || facet !== "all" || mode === "source"
    ? lines
    : mode === "detailed"
      ? compactSemanticLines(lines)
      : basicSemanticLines(lines);
  const facetLines = semanticLinesForFacet(compactLines, facet);
  if (!terms.length) {
    return facetLines;
  }
  const matchingObjects = new Set();
  const objectText = new Map();
  for (const line of facetLines) {
    if (line.objectIndex === undefined || line.objectIndex === null) {
      continue;
    }
    const key = String(line.objectIndex);
    objectText.set(key, `${objectText.get(key) || ""} ${line.text || ""} ${line.objectType || ""} ${line.objectName || ""}`.toLowerCase());
  }
  for (const [objectIndex, text] of objectText) {
    if (terms.every((term) => text.includes(term))) {
      matchingObjects.add(objectIndex);
    }
  }
  return facetLines.filter((line) => {
    if (line.objectIndex === undefined || line.objectIndex === null) {
      return true;
    }
    return matchingObjects.has(String(line.objectIndex));
  });
}

function semanticProjectionMode() {
  return ["basic", "detailed", "source"].includes(state.semanticProjectionMode) ? state.semanticProjectionMode : "basic";
}

function semanticProjectionFacet() {
  return ["all", "field", "editable", "derived", "evidence"].includes(state.semanticProjectionFacet) ? state.semanticProjectionFacet : "all";
}

function semanticModeLabel(mode) {
  switch (mode) {
    case "source":
      return "Source/debug";
    case "detailed":
      return "Detailed";
    default:
      return "Basic";
  }
}

function semanticFacetLabel(facet) {
  switch (facet) {
    case "field":
      return "Source fields";
    case "editable":
      return "Editable";
    case "derived":
      return "Derived";
    case "evidence":
      return "Evidence";
    default:
      return "All";
  }
}

function semanticLinesForFacet(lines, facet) {
  if (facet === "all") {
    return lines;
  }
  const keep = new Set();
  lines.forEach((line, index) => {
    if (!semanticLineMatchesFacet(line, facet)) {
      return;
    }
    keep.add(index);
    for (const ancestorIndex of semanticAncestorLineIndexes(lines, index)) {
      keep.add(ancestorIndex);
    }
    const indent = Number(line.indent || 0);
    for (let childIndex = index + 1; childIndex < lines.length; childIndex += 1) {
      if (Number(lines[childIndex].indent || 0) <= indent) {
        break;
      }
      keep.add(childIndex);
    }
  });
  return lines.filter((_, index) => keep.has(index));
}

function semanticLineMatchesFacet(line, facet) {
  const sourceKind = String(line.sourceKind || "");
  const role = String(line.role || "");
  const key = semanticLineKeyToken(line);
  switch (facet) {
    case "field":
      return sourceKind === "field";
    case "editable":
      return Boolean(line.editable);
    case "derived":
      return sourceKind === "derived" || sourceKind === "summary" || role === "metadata";
    case "evidence":
      return ["source", "confidence", "evidence", "relation", "relation_source", "source_relations", "source_preservation", "duplicated_as", "also_shown_in", "sync_policy"].includes(key);
    default:
      return true;
  }
}

function semanticLineKeyToken(line) {
  const explicit = String(line.key || "").trim();
  if (explicit) {
    return explicit;
  }
  const text = String(line.text || "").trim().replace(/^- /, "");
  return text.split(":")[0].trim();
}

function semanticAncestorLineIndexes(lines, index) {
  const ancestors = [];
  let indent = Number(lines[index]?.indent || 0);
  for (let candidate = index - 1; candidate >= 0; candidate -= 1) {
    const candidateIndent = Number(lines[candidate].indent || 0);
    if (candidateIndent >= indent) {
      continue;
    }
    if (semanticLineIsBranch(lines[candidate]) || candidateIndent <= 1) {
      ancestors.push(candidate);
      indent = candidateIndent;
      if (indent <= 0) {
        break;
      }
    }
  }
  return ancestors.reverse();
}

function basicSemanticLines(lines) {
  const hiddenBlocks = new Set([
    "duplicated_as",
    "also_shown_in",
    "sync_policy",
    "source_relations",
    "source_preservation",
    "raw",
    "computed",
    "vertices",
  ]);
  const keepKeys = new Set([
    "schema",
    "name",
    "class",
    "type",
    "family",
    "family_label",
    "display_label",
    "role_here",
    "source",
    "confidence",
    "status",
    "value",
    "zone",
    "space",
    "schedule",
    "air_loop",
    "plant_loop",
    "air_loops",
    "plant_loops",
    "condenser_loops",
    "terminal_units",
    "zone_equipment",
    "outputs",
    "diagnostics",
  ]);
  const out = [];
  let hideUntilIndent = null;
  for (const line of lines) {
    const indent = Number(line.indent || 0);
    if (hideUntilIndent !== null && indent > hideUntilIndent) {
      continue;
    }
    hideUntilIndent = null;
    const key = String(line.key || "").trim();
    if (hiddenBlocks.has(key)) {
      hideUntilIndent = indent;
      continue;
    }
    const text = String(line.text || "").trimStart();
    if (semanticBasicKeepsSyntax(line)) {
      out.push(line);
      continue;
    }
    if (text.startsWith("- name:") && indent <= 4 && semanticBasicKeepsObjectName(line)) {
      out.push(line);
      continue;
    }
    if (semanticLineHasValue(line) && indent <= 4 && keepKeys.has(key) && semanticBasicKeepsValueLine(line)) {
      out.push(line);
    }
  }
  return materializedBasicSemanticLines(out);
}

function materializedBasicSemanticLines(lines) {
  const expanded = state.semanticExpandedSectionIds instanceof Set
    ? state.semanticExpandedSectionIds
    : new Set(["project"]);
  const out = [];
  let sectionId = "";
  for (const line of lines) {
    const indent = Number(line.indent || 0);
    if (semanticTopLevelSectionLine(line)) {
      sectionId = semanticSectionId(line);
      out.push(line);
      continue;
    }
    if (indent <= 1 || !sectionId || expanded.has(sectionId)) {
      out.push(line);
    }
  }
  return out;
}

function semanticBasicKeepsSyntax(line = {}) {
  if (line.text === "semantic_energyplus_model:") {
    return true;
  }
  const indent = Number(line.indent || 0);
  if (line.role !== "syntax") {
    return false;
  }
  if (indent <= 1) {
    return true;
  }
  if (indent !== 2) {
    return false;
  }
  return ["definitions", "zones", "air_loops", "plant_loops", "condenser_loops", "zone_relations", "files", "variables", "meters", "diagnostics"].includes(semanticLineKeyToken(line));
}

function semanticBasicKeepsObjectName(line = {}) {
  const objectType = String(line.objectType || "").trim().toLowerCase();
  return (
    objectType === "zone" ||
    objectType === "space" ||
    objectType === "airloophvac" ||
    objectType === "plantloop" ||
    objectType === "condenserloop"
  );
}

function semanticBasicKeepsValueLine(line = {}) {
  const indent = Number(line.indent || 0);
  if (line.sourceKind === "summary" && indent <= 2) {
    return true;
  }
  if (semanticBasicKeepsObjectName(line)) {
    return true;
  }
  return ["source", "confidence", "status", "value", "air_loops", "plant_loops", "condenser_loops", "terminal_units", "zone_equipment", "outputs", "diagnostics"].includes(semanticLineKeyToken(line)) && indent <= 4;
}

function compactSemanticLines(lines) {
  const hiddenKeys = new Set(["duplicated_as", "also_shown_in", "sync_policy", "source_relations", "source_preservation"]);
  const out = [];
  let hideUntilIndent = null;
  for (const line of lines) {
    const indent = Number(line.indent || 0);
    if (hideUntilIndent !== null && indent > hideUntilIndent) {
      continue;
    }
    hideUntilIndent = null;
    const key = String(line.key || "").trim();
    if (hiddenKeys.has(key)) {
      hideUntilIndent = indent;
      continue;
    }
    out.push(line);
  }
  return out;
}

function renderSemanticSectionIndex(lines = []) {
  const sections = lines
    .map((line, index) => ({ line, index }))
    .filter(({ line }) => semanticTopLevelSectionLine(line));
  if (!sections.length) {
    return "";
  }
  const expanded = state.semanticExpandedSectionIds instanceof Set
    ? state.semanticExpandedSectionIds
    : new Set();
  return `
    <nav class="semantic-section-index" aria-label="Semantic sections">
      ${sections
        .map(({ line, index }) => {
          const label = semanticSectionLabel(line);
          const sectionId = semanticSectionId(line);
          const count = semanticSectionEntityCount(lines, index);
          const isExpanded = expanded.has(sectionId);
          return `<button type="button" data-semantic-section-id="${escapeHTML(sectionId)}" data-semantic-section-text="${escapeHTML(line.text || "")}" aria-expanded="${isExpanded ? "true" : "false"}">${escapeHTML(label)}${count ? ` <span aria-hidden="true">(${escapeHTML(count)})</span>` : ""}</button>`;
        })
        .join("")}
    </nav>`;
}

function semanticTopLevelSectionLine(line = {}) {
  return line.role === "syntax" &&
    Number(line.indent || 0) === 1 &&
    String(line.text || "").trim() !== "semantic_energyplus_model:" &&
    semanticLineIsBranch(line);
}

function semanticSectionId(line = {}) {
  return semanticLineKeyToken(line).trim().toLowerCase().replace(/[^a-z0-9_-]+/g, "-") || "section";
}

function semanticSectionEntityCount(lines, sectionIndex) {
  const identities = new Set();
  for (let index = sectionIndex + 1; index < lines.length; index += 1) {
    const line = lines[index];
    if (Number(line.indent || 0) <= 1) {
      break;
    }
    const identity = line.entityId || (
      line.objectIndex === undefined || line.objectIndex === null
        ? ""
        : `object:${line.objectIndex}`
    );
    if (identity) {
      identities.add(String(identity));
    }
  }
  return identities.size;
}

function semanticSectionLabel(line) {
  const text = String(line.text || "").trim().replace(/:$/, "");
  return text
    .split("_")
    .map((part) => part.slice(0, 1).toUpperCase() + part.slice(1))
    .join(" ");
}

function renderSemanticWarnings(projection) {
  const groups = projection.sourceNameConflicts || [];
  if (!groups.length) {
    return `<div class="semantic-health ok">${escapeHTML(t("semantic.noSourceNameConflicts", {}, "No source name conflicts in the current registry."))}</div>`;
  }
  return `
    <div class="semantic-health warn">
      <strong>${escapeHTML(t("semantic.sourceNameConflicts", { count: groups.length }, `${groups.length} source name conflict groups`))}</strong>
      ${groups
        .map(
          (group) => `
            <span title="${escapeHTML((group.objectIndexes || []).join(", "))}">
              ${escapeHTML(group.objectType)} / ${escapeHTML(group.name)} / ${escapeHTML((group.objectIndexes || []).join(", "))}
            </span>`,
        )
        .join("")}
    </div>`;
}

function renderSemanticLine(line, lineIndex, keyWidths = new Map()) {
  const objectIndex = line.objectIndex ?? "";
  const fieldIndex = line.fieldIndex ?? "";
  const selected = objectIndex !== "" && String(objectIndex) === String(state.semanticSelectedObjectIndex);
  const indent = Number(line.indent || 0);
  const style = semanticLineHasValue(line) ? `style="--semantic-key-width:${keyWidths.get(indent) || 12}ch"` : "";
  const classes = semanticLineClassNames(line, selected);
  const attrs = [
    `data-semantic-line="${lineIndex}"`,
    `data-object-index="${escapeHTML(objectIndex)}"`,
    `data-object-type="${escapeHTML(line.objectType || "")}"`,
    `data-field-index="${escapeHTML(fieldIndex)}"`,
    `data-field-index-kind="idf"`,
    `data-semantic-indent="${escapeHTML(indent)}"`,
    `data-semantic-key="${escapeHTML(line.key || "")}"`,
    `data-semantic-role="${escapeHTML(line.role || "")}"`,
    `data-semantic-text="${escapeHTML(line.text || "")}"`,
  ].join(" ");
  return `<div class="${classes}" ${style} ${attrs}>${renderSemanticLineContent(line)}</div>`;
}

function semanticLineClassNames(line, selected) {
  const classes = ["semantic-line"];
  if (selected) {
    classes.push("selected");
  }
  if (line.editable) {
    classes.push("editable");
  }
  if (line.text === "semantic_energyplus_model:") {
    classes.push("semantic-root-line");
  }
  if (line.role === "syntax" && Number(line.indent || 0) <= 1 && line.text !== "semantic_energyplus_model:") {
    classes.push("semantic-section-line");
  }
  if (semanticLineIsBranch(line)) {
    classes.push("semantic-branch-line");
  }
  if (String(line.key || "") === "class" || String(line.text || "").trimStart().startsWith("- name:")) {
    classes.push("semantic-object-line");
  }
  return classes.join(" ");
}

function renderSemanticLineContent(line) {
  if (!semanticLineHasValue(line)) {
    return `<code>${escapeHTML(line.text || "")}</code>`;
  }
  const indent = "  ".repeat(Number(line.indent || 0));
  const key = semanticDisplayKey(line);
  const displayValue = line.displayValue ?? line.value ?? "";
  const patchValue = line.patchValue ?? line.sourceValue ?? displayValue;
  const badge = renderSemanticSourceBadge(line, displayValue);
  const value = line.editable
    ? `<button class="semantic-value-token" type="button" data-object-index="${escapeHTML(line.objectIndex ?? "")}" data-field-index="${escapeHTML(line.fieldIndex ?? "")}" data-field-index-kind="idf" data-original="${escapeHTML(patchValue)}" data-display="${escapeHTML(displayValue)}" data-edit-kind="${escapeHTML(line.editKind || "raw_field")}">${escapeHTML(semanticDisplayScalar(displayValue))}</button>`
    : `<span class="semantic-value" data-source-kind="${escapeHTML(line.sourceKind || "")}">${escapeHTML(semanticDisplayScalar(displayValue))}</span>`;
  return `<code class="semantic-code-kv"><span class="semantic-indent">${escapeHTML(indent)}</span><span class="semantic-key">${escapeHTML(key)}</span><span class="semantic-colon">:</span> ${value}${badge}</code>`;
}

function semanticLineHasValue(line) {
  return Boolean(line?.key) && (line.editable || line.displayValue !== undefined || line.value !== undefined || line.role === "metadata" || line.role === "object" || line.role === "field");
}

function renderSemanticSourceBadge(line, displayValue) {
  const badge = semanticSourceBadge(line, displayValue);
  if (!badge) {
    return "";
  }
  const title = [line.sourceKind, line.editKind, line.role].filter(Boolean).join(" / ");
  return `<span class="semantic-source-badge" data-kind="${escapeHTML(badge.kind)}" title="${escapeHTML(title)}">${escapeHTML(badge.label)}</span>`;
}

function semanticSourceBadge(line, displayValue) {
  const key = semanticLineKeyToken(line);
  const scalar = String(displayValue || "").toLowerCase();
  if (key === "confidence" && (scalar.includes("inferred") || scalar.includes("partial") || scalar === "low" || scalar === "medium")) {
    return { label: "Inferred", kind: "inferred" };
  }
  if (key === "source" && (scalar.includes("inference") || scalar.includes("fallback") || scalar.includes("computed"))) {
    return { label: "Inferred", kind: "inferred" };
  }
  if (line.editable || line.sourceKind === "field") {
    return { label: "Raw", kind: "raw" };
  }
  if (line.sourceKind === "derived") {
    return { label: "Computed", kind: "computed" };
  }
  if (line.sourceKind === "summary" || line.role === "metadata") {
    return { label: "Summary", kind: "summary" };
  }
  return null;
}

function semanticDisplayKey(line) {
  const key = String(line?.key || "field");
  if (String(line?.text || "").trimStart().startsWith("- ")) {
    return `- ${key}`;
  }
  return key;
}

function bindSemanticControls() {
  elements.semanticEditor.querySelectorAll("[data-semantic-mode]").forEach((button) => {
    button.addEventListener("click", () => {
      state.semanticProjectionMode = button.dataset.semanticMode || "basic";
      renderSemanticView();
    });
  });
  elements.semanticEditor.querySelectorAll("[data-semantic-facet]").forEach((button) => {
    button.addEventListener("click", () => {
      state.semanticProjectionFacet = button.dataset.semanticFacet || "all";
      renderSemanticView();
    });
  });
  elements.semanticEditor.querySelectorAll("[data-semantic-section-text]").forEach((button) => {
    button.addEventListener("click", () => {
      if (semanticProjectionMode() === "basic") {
        if (!(state.semanticExpandedSectionIds instanceof Set)) {
          state.semanticExpandedSectionIds = new Set(["project"]);
        }
        const sectionId = button.dataset.semanticSectionId || "";
        if (state.semanticExpandedSectionIds.has(sectionId)) {
          state.semanticExpandedSectionIds.delete(sectionId);
        } else if (sectionId) {
          state.semanticExpandedSectionIds.add(sectionId);
        }
        renderSemanticView();
        return;
      }
      scrollSemanticSectionIntoView(button.dataset.semanticSectionText || "");
    });
  });
  elements.semanticEditor.querySelector("#semanticProjectionModeButton")?.addEventListener("click", () => {
    state.semanticProjectionMode = semanticProjectionMode() === "source" ? "basic" : "source";
    renderSemanticView();
  });
  elements.semanticEditor.querySelector("#semanticFocusObjectButton")?.addEventListener("click", () => focusSelectedSemanticObject());
  elements.semanticEditor.querySelector("#semanticFixDuplicatesButton")?.addEventListener("click", () => applySemanticDuplicateFixes());
  elements.semanticEditor.querySelectorAll(".semantic-line[data-object-index]").forEach((line) => {
    if (line.dataset.objectIndex === "") {
      return;
    }
    line.addEventListener("pointerenter", () => highlightSemanticObject(line.dataset.objectIndex, true));
    line.addEventListener("pointerleave", () => highlightSemanticObject(line.dataset.objectIndex, false));
    line.addEventListener("click", () => selectSemanticLine(line));
  });
  elements.semanticEditor.querySelectorAll(".semantic-value-token").forEach((button) => {
    button.addEventListener("click", (event) => {
      event.stopPropagation();
      editSemanticValue(button);
    });
  });
  bindSemanticStickyPath();
}

function scrollSemanticSectionIntoView(sectionText) {
  const target = Array.from(elements.semanticEditor.querySelectorAll(".semantic-line")).find((line) => line.dataset.semanticText === sectionText);
  target?.scrollIntoView({ block: "start", inline: "nearest" });
}

function semanticLineIsBranch(line) {
  const text = String(line?.text || "").trim();
  if (!text || text === "semantic_energyplus_model:") {
    return false;
  }
  if (String(line?.text || "").trimStart().startsWith("- name:")) {
    return true;
  }
  return String(line?.role || "") === "syntax" && text.endsWith(":");
}

function bindSemanticStickyPath() {
  const sticky = elements.semanticEditor.querySelector(".semantic-sticky-path");
  const yaml = elements.semanticEditor.querySelector(".semantic-yaml");
  if (!sticky || !yaml) {
    return;
  }
  const lines = Array.from(yaml.querySelectorAll(".semantic-line"));
  const update = () => {
    const editorRect = elements.semanticEditor.getBoundingClientRect();
    const threshold = editorRect.top + sticky.offsetHeight + 6;
    let activeLine = lines[0] || null;
    for (const line of lines) {
      if (line.getBoundingClientRect().top > threshold) {
        break;
      }
      activeLine = line;
    }
    const path = semanticPathForLine(lines, activeLine);
    sticky.innerHTML = path.length
      ? path.map((label) => `<span>${escapeHTML(label)}</span>`).join(`<span class="semantic-path-separator">/</span>`)
      : `<span>${escapeHTML("semantic_energyplus_model")}</span>`;
  };
  const onScroll = () => requestAnimationFrame(update);
  elements.semanticEditor._semanticStickyScrollHandler = onScroll;
  elements.semanticEditor.addEventListener("scroll", onScroll, { passive: true });
  requestAnimationFrame(update);
}

function clearSemanticStickyPathBinding() {
  const handler = elements.semanticEditor?._semanticStickyScrollHandler;
  if (!handler) {
    return;
  }
  elements.semanticEditor.removeEventListener("scroll", handler);
  delete elements.semanticEditor._semanticStickyScrollHandler;
}

function semanticPathForLine(lines, activeLine) {
  if (!activeLine) {
    return [];
  }
  const activeIndex = Number(activeLine.dataset.semanticLine || 0);
  const stack = [];
  for (const line of lines) {
    const lineIndex = Number(line.dataset.semanticLine || 0);
    if (lineIndex > activeIndex) {
      break;
    }
    const label = semanticPathLabel(line);
    if (!label) {
      continue;
    }
    const indent = Number(line.dataset.semanticIndent || 0);
    while (stack.length && stack[stack.length - 1].indent >= indent) {
      stack.pop();
    }
    stack.push({ indent, label });
  }
  return stack.map((entry) => entry.label).slice(-6);
}

function semanticPathLabel(line) {
  const raw = String(line?.dataset?.semanticText || "").trim();
  if (!raw) {
    return "";
  }
  if (raw === "semantic_energyplus_model:") {
    return "semantic_energyplus_model";
  }
  const text = raw.startsWith("- ") ? raw.slice(2).trim() : raw;
  if (text.startsWith("name:")) {
    return text.slice("name:".length).trim().replace(/^"(.*)"$/, "$1");
  }
  if (text.endsWith(":")) {
    return text.slice(0, -1).trim();
  }
  if (text.endsWith(": {}") || text.endsWith(": []")) {
    return text.split(":")[0].trim();
  }
  return "";
}

function selectSemanticLine(line) {
  state.semanticSelectedObjectIndex = line.dataset.objectIndex || "";
  syncRawTextToFormattedTarget(line);
  renderSemanticSelectionOnly();
}

function renderSemanticSelectionOnly() {
  elements.semanticEditor.querySelectorAll(".semantic-line[data-object-index]").forEach((line) => {
    line.classList.toggle("selected", line.dataset.objectIndex === String(state.semanticSelectedObjectIndex));
  });
}

function highlightSemanticObject(objectIndex, active) {
  elements.semanticEditor.querySelectorAll(`.semantic-line[data-object-index="${cssAttrEscape(objectIndex)}"]`).forEach((line) => {
    line.classList.toggle("hovered", active);
  });
}

function focusSelectedSemanticObject() {
  const selected = state.semanticSelectedObjectIndex || elements.semanticEditor.querySelector(".semantic-line[data-object-index]:not([data-object-index=''])")?.dataset.objectIndex || "";
  if (!selected) {
    return;
  }
  state.semanticSelectedObjectIndex = selected;
  const line = elements.semanticEditor.querySelector(`.semantic-line[data-object-index="${cssAttrEscape(selected)}"]`);
  if (line) {
    line.scrollIntoView({ block: "center", inline: "nearest" });
    syncRawTextToFormattedTarget(line);
    renderSemanticSelectionOnly();
  }
}

function editSemanticValue(button) {
  const current = button.dataset.original || "";
  const editor = document.createElement("input");
  editor.type = "text";
  editor.className = "semantic-value-editor";
  editor.value = current;
  editor.dataset.objectIndex = button.dataset.objectIndex;
  editor.dataset.fieldIndex = button.dataset.fieldIndex;
  editor.dataset.fieldIndexKind = button.dataset.fieldIndexKind || "idf";
  editor.dataset.original = current;
  editor.style.width = `${Math.min(Math.max(current.length + 2, 10), 58)}ch`;
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
    if (editor.value === current) {
      restore();
      return;
    }
    await applyFieldValue(editor, t("semantic.fieldUpdated", {}, "Semantic YAML field updated"));
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

async function applySemanticDuplicateFixes() {
  const api = backend();
  if (!api || typeof api.ApplySemanticDuplicateNameFixText !== "function") {
    setStatus(t("status.backendUnavailable"), "warn");
    return;
  }
  try {
    const result = await api.ApplySemanticDuplicateNameFixText(elements.idfInput.value);
    elements.idfInput.value = result.text;
    updateTextStats();
    state.semanticProjection = result.semantic || null;
    await analyzeCallback();
    const count = result.warnings?.length || 0;
    setStatus(t("semantic.duplicatesFixed", { count }, `Renamed ${count} duplicate objects`), "ok");
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
}

function semanticDisplayScalar(value) {
  const text = String(value ?? "");
  if (text.trim() === "") {
    return "null";
  }
  if (/^(true|false|yes|no|on|off|null)$/i.test(text) || /[,:[\]{}#*!|>&%@`"']/.test(text) || /\s{2,}/.test(text)) {
    return JSON.stringify(text);
  }
  return text;
}

function cssAttrEscape(value) {
  return String(value ?? "").replaceAll("\\", "\\\\").replaceAll('"', '\\"');
}

function renderFormattedTextView() {
  const report = state.report;
  if (!report || !Array.isArray(report.objects) || !hasCurrentAnalysis()) {
    elements.textObjectView.innerHTML = `<div class="empty">${escapeHTML(pendingViewMessage("formatted text"))}</div>`;
    setInputFilterStats(0, 0);
    return;
  }

  const versionLabel = state.model?.version?.raw || "unknown";
  const formatLabel = state.model?.format || "unknown";
  const groups = groupedReportObjects();
  const matchingObjects = groups.reduce((sum, group) => sum + group.objects.length, 0);
  setInputFilterStats(matchingObjects, report.objects.length);
  elements.textObjectView.innerHTML = `
    <div class="json-meta">
      <span class="badge">${escapeHTML(formatLabel)}</span>
      <span class="badge">Version ${escapeHTML(versionLabel)}</span>
      <span class="badge">${escapeHTML(t("count.objects", { count: matchingObjects }))}</span>
      <span class="badge">${escapeHTML(t("input.editableFields"))}</span>
    </div>
    ${
      groups.length
        ? `<div class="json-groups">
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
    setInputFilterStats(0, 0);
    return;
  }

  const versionLabel = model.version?.raw || "unknown";
  const visibleObjects = filterInputObjects(model.objects);
  setInputFilterStats(visibleObjects.length, model.objects.length);

  elements.jsonStructuredView.innerHTML = `
    <div class="json-meta">
      <span class="badge">${escapeHTML(model.format || "unknown")}</span>
      <span class="badge">Version ${escapeHTML(versionLabel)}</span>
      <span class="badge">${escapeHTML(t("count.objects", { count: visibleObjects.length }))}</span>
    </div>
    <div class="json-editor-tools">
      <select id="jsonCollapseDepth" aria-label="JSON collapse depth">
        ${[
          ["1", t("input.typeOnly")],
          ["2", t("common.object")],
          ["3", t("common.field")],
          ["99", t("input.expandAll")],
        ]
          .map(
            ([value, label]) =>
              `<option value="${value}" ${String(state.jsonCollapseDepth) === value ? "selected" : ""}>${label}</option>`,
          )
          .join("")}
      </select>
      <button id="jsonFocusObjectButton" type="button">${t("input.focusObject")}</button>
    </div>
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
  const openAttr = state.jsonCollapseDepth >= 1 ? "open" : "";
  return `
    <details class="json-node json-type-group" data-object-type="${escapeHTML(group.type)}" ${openAttr}>
      <summary>
        <span class="json-line"><span class="json-key">${formatJSONKey(group.type)}</span><span class="json-colon">: </span><span class="json-brace">{</span></span>
        <span class="badge">${escapeHTML(t("count.objects", { count: group.objects.length }))}</span>
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
  const depthSelect = elements.jsonStructuredView.querySelector("#jsonCollapseDepth");
  const focusButton = elements.jsonStructuredView.querySelector("#jsonFocusObjectButton");

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
  });
  highlightFormattedTextTarget(target);
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
    setStatus(t("status.backendUnavailable"), "warn");
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
    state.semanticProjection = result.semantic || null;
    state.lastAnalyzedText = result.text;
    state.analysisKey = result.analysisKey || "";
    state.lastAnalyzedKey = state.analysisKey;
    state.reportAnalyzedText = result.text;
    state.reportAnalysisKey = state.analysisKey;
    state.analysisStage = "complete";
    state.diagnosticsReady = true;
    state.geometryReady = true;
    Object.keys(state.analysisReady || {}).forEach((view) => {
      state.analysisReady[view] = true;
    });
    state.reportAnalysisStage = state.analysisStage;
    state.reportAnalysisReady = { ...(state.analysisReady || {}) };
    state.reportDiagnosticsReady = true;
    state.reportGeometryReady = true;
    window.dispatchEvent(new Event("idfAnalyzer:documentChanged"));
    renderReportCallback();
    window.dispatchEvent(new CustomEvent("idfAnalyzer:analysisComplete", {
      detail: { text: result.text, analysisKey: state.reportAnalysisKey, stage: "complete" },
    }));
    setStatus(t("input.jsonValueUpdated"), "ok");
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
  elements.syncRawTextToggle.checked = state.syncTextRawPosition;
  elements.textObjectView.querySelectorAll(".text-object-head").forEach((head) => {
    head.addEventListener("click", () => syncRawTextToFormattedTarget(head));
  });
  elements.textObjectView.querySelectorAll(".text-field-input").forEach((input) => {
    input.addEventListener("focus", () => {
      syncRawTextToFormattedTarget(input);
      loadFieldSuggestions(input);
    });
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
  if (document.activeElement === elements.idfInput) {
    const text = elements.idfInput.value;
    const token = isLikelyJSONText(text)
      ? findJSONTokenAtOffset(text, elements.idfInput.selectionStart || 0)
      : findIDFTokenAtOffset(text, elements.idfInput.selectionStart || 0);
    if (!token) {
      return null;
    }
    return jumpSourceForContext({
      objectIndex: token.objectIndex,
      fieldIndex: token.type === "field" ? token.fieldIndex : null,
      fieldIndexKind: token.fieldIndexKind || "idf",
    });
  }

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
    console.debug("Field suggestions unavailable", error);
  } finally {
    delete input.dataset.suggestionsLoading;
  }
}

async function requestFieldSuggestions(objectIndex, fieldIndex) {
  const api = backend();
  if (api && typeof api.SuggestFieldValuesText === "function") {
    return api.SuggestFieldValuesText(elements.idfInput.value, objectIndex, fieldIndex);
  }

  const response = await fetch("/api/field-suggestions", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text: elements.idfInput.value, objectIndex, fieldIndex }),
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
  await applyFieldValue(input, t("input.textFieldUpdated"));
}

async function applyFieldValue(input, successMessage = t("input.fieldUpdated")) {
  const nextValue = input.value;
  if (nextValue === input.dataset.original || input.dataset.committing === "true") {
    return;
  }

  const api = backend();
  if (!api || typeof api.UpdateFieldText !== "function") {
    setStatus(t("status.backendUnavailable"), "warn");
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
  recordViewHistory();
  const objectIndex = Number(element.dataset.objectIndex);
  const fieldIndex = element.dataset.fieldIndex === undefined ? null : Number(element.dataset.fieldIndex);
  syncRawTextToObjectField(objectIndex, fieldIndex, element.dataset.fieldIndexKind || "idf");
}

export function syncRawTextToObjectField(objectIndex, fieldIndex = null, fieldIndexKind = "idf") {
  const range = findRawTextRangeForTextTarget(objectIndex, fieldIndex, fieldIndexKind);
  if (!range) {
    return false;
  }
  moveRawTextToRange(range);
  return true;
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
  elements.idfInput.setSelectionRange(start, end);
  elements.idfInput.scrollTop = Math.max(0, lineIndex * lineHeight - elements.idfInput.clientHeight * 0.25);
  highlightRawTextTarget();
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
  if (state.activeInputView === "semantic") {
    if (token.type === "field") {
      const idfFieldIndex =
        token.fieldIndexKind === "model" ? modelFieldIndexToIDFFieldIndex(token.objectIndex, token.fieldIndex) : token.fieldIndex;
      return elements.semanticEditor.querySelector(
        `.semantic-line[data-object-index="${token.objectIndex}"][data-field-index="${idfFieldIndex}"]`,
      );
    }
    return elements.semanticEditor.querySelector(`.semantic-line[data-object-index="${token.objectIndex}"]`);
  }
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
  const container = element.closest(".semantic-editor, .formatted-object-view, .json-view, .field-table");
  if (!container) {
    return;
  }
  const containerRect = container.getBoundingClientRect();
  const elementRect = element.getBoundingClientRect();
  container.scrollTo({
    top: Math.max(0, container.scrollTop + elementRect.top - containerRect.top - container.clientHeight * 0.25),
    left: Math.max(0, container.scrollLeft + elementRect.left - containerRect.left - 24),
  });
}

function highlightFormattedTextTarget(element) {
  element.classList.remove("input-jump-highlight");
  void element.offsetWidth;
  element.classList.add("input-jump-highlight");
  window.setTimeout(() => element.classList.remove("input-jump-highlight"), 1200);
}

function highlightRawTextTarget() {
  const rawBlock = elements.idfInput.closest(".raw-editor-block");
  if (!rawBlock) {
    return;
  }
  rawBlock.classList.remove("raw-text-jump-highlight");
  void rawBlock.offsetWidth;
  rawBlock.classList.add("raw-text-jump-highlight");
  window.setTimeout(() => rawBlock.classList.remove("raw-text-jump-highlight"), 900);
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
  if (!report || !Array.isArray(report.objects) || !hasCurrentAnalysis()) {
    elements.fieldTable.innerHTML = `<div class="empty">${escapeHTML(pendingViewMessage("table"))}</div>`;
    elements.fieldStats.textContent = t("input.tableStats", { tables: 0, objects: 0, orientation: "" });
    setInputFilterStats(0, 0);
    return;
  }

  const groups = groupObjectsByType(filterInputObjects(report.objects));
  const objectCount = groups.reduce((sum, group) => sum + group.objects.length, 0);
  const orientationLabel = state.tableOrientation === "fields" ? t("input.fieldsRows").toLowerCase() : t("input.objectsRows").toLowerCase();
  elements.fieldStats.textContent = t("input.tableStats", { tables: groups.length, objects: objectCount, orientation: orientationLabel });
  setInputFilterStats(objectCount, report.objects.length);
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

  elements.fieldTable.querySelectorAll(".field-value-input").forEach((input) => {
    input.addEventListener("focus", () => {
      syncRawTextToFormattedTarget(input);
      loadFieldSuggestions(input);
    });
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
          <span class="badge">${escapeHTML(t("count.objects", { count: group.objects.length }))}</span>
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
  await applyFieldValue(input, t("input.fieldUpdated"));
}

export async function switchInputView(viewName, options = {}) {
  if (options.recordHistory !== false && state.activeInputView !== viewName) {
    recordViewHistory();
  }
  state.activeInputView = viewName;
  elements.inputViewButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.inputView === viewName);
  });
  elements.inputViews.forEach((view) => {
    view.classList.toggle("active", view.id === `${viewName}InputView`);
  });
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
