import { backend, elements, escapeHTML, getDocumentText, setDocumentText, setStatus, state } from "../state.js";
import { t } from "../i18n.js";
import { recordViewHistory } from "../view-history.js";
import {
  clearSemanticHover,
  clearSemanticSelection,
  currentSemanticSelection,
  hoverSemanticEntity,
  openSelectionInView,
  revealSelectionSource,
  selectSemanticEntity,
  selectionTargetsForView,
  semanticOccurrenceChoices,
} from "../selection-controller.js";
import { getPanelNavigationAdapter } from "../panel-navigation-registry.js";

const FIELD_TABLE_RENDER_LIMIT = 500;

let analyzeCallback = async () => {};
let renderReportCallback = () => renderInputViews();
let jumpIndexCache = { report: null, definitions: new Map(), references: new Map() };
let semanticClickHistoryKey = "";
let semanticTargetViewCache = new Map();
let semanticControlsBound = false;
let jsonEditorControlsBound = false;
let formattedTextControlsBound = false;
let fieldTableControlsBound = false;

window.addEventListener("idfAnalyzer:semanticSelectionChanged", (event) => {
  if (state.activeInputView === "semantic") {
    if (event.detail?.temporaryRevealCleared) {
      renderSemanticView();
    } else {
      renderSemanticSelectionOnly();
      refreshSemanticSelectionContext();
    }
  }
});
window.addEventListener("idfAnalyzer:semanticHoverChanged", () => {
  if (state.activeInputView === "semantic") {
    renderSemanticSelectionOnly();
  }
});
window.addEventListener("idfAnalyzer:semanticTemporaryReveal", (event) => {
  ensureSemanticOccurrenceVisible(event.detail?.reveal || currentSemanticSelection());
});
window.addEventListener("idfAnalyzer:semanticSelectionRemapped", (event) => {
  const selection = event.detail?.selection || currentSemanticSelection();
  if (selection?.entityId) {
    restoreSemanticSelectionAfterAnalysis(selection);
  }
});

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
  return state.reportAnalyzedText !== "" && state.reportAnalyzedText === getDocumentText();
}

function pendingViewMessage(viewName) {
  if (!getDocumentText().trim()) {
    return t("input.noLoaded");
  }
  return t("input.pendingView", { view: viewName });
}

function renderSemanticView() {
  clearSemanticStickyPathBinding();
  semanticTargetViewCache = new Map();
  const projection = state.semanticProjection;
  if (!projection || !Array.isArray(projection.lines) || !hasCurrentAnalysis()) {
    elements.semanticEditor.innerHTML = `<div class="empty">${escapeHTML(pendingViewMessage("semantic YAML"))}</div>`;
    return;
  }

  const terms = currentInputFilterTerms();
  const visibleLines = semanticVisibleLines(projection.lines, terms);
  const keyWidths = semanticKeyWidths(visibleLines);

  const mode = semanticProjectionMode();
  const facet = semanticProjectionFacet();
  elements.semanticEditor.dataset.semanticMode = mode;
  elements.semanticEditor.innerHTML = `
    <div class="semantic-toolbar">
      <div class="json-meta">
        <span class="badge">${escapeHTML(projection.schema || "eplus-semantic/0.1")}</span>
        <span class="badge">${escapeHTML(semanticModeLabel(mode))}</span>
        ${facet === "all" ? "" : `<span class="badge">${escapeHTML(semanticFacetLabel(facet))}</span>`}
      </div>
    </div>
    <div id="semanticSelectionContext">${renderSemanticSelectionContext(currentSemanticSelection())}</div>
    ${renderSemanticTemporaryReveal()}
    ${renderSemanticWarnings(projection)}
    ${renderSemanticSectionIndex(projection.lines)}
    <div class="semantic-sticky-path" aria-live="polite"></div>
    <div class="semantic-yaml" data-semantic-mode="${escapeHTML(mode)}" role="tree" aria-label="Semantic YAML projection">
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
    return semanticLinesWithTemporaryReveal(lines, facetLines);
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
  const filteredLines = facetLines.filter((line) => {
    if (line.objectIndex === undefined || line.objectIndex === null) {
      return true;
    }
    return matchingObjects.has(String(line.objectIndex));
  });
  return semanticLinesWithTemporaryReveal(lines, filteredLines);
}

function semanticLinesWithTemporaryReveal(allLines, visibleLines) {
  const reveal = state.semanticTemporaryReveal;
  if (!reveal?.entityId && !reveal?.occurrenceId) {
    return visibleLines;
  }
  const keep = new Set(visibleLines);
  allLines.forEach((line, index) => {
    const matches = reveal.occurrenceId
      ? line.occurrenceId === reveal.occurrenceId
      : line.entityId === reveal.entityId;
    if (!matches) {
      return;
    }
    keep.add(line);
    semanticAncestorLineIndexes(allLines, index).forEach((ancestorIndex) => keep.add(allLines[ancestorIndex]));
  });
  return allLines.filter((line) => keep.has(line));
}

function semanticProjectionMode() {
  return "detailed";
}

function semanticProjectionFacet() {
  return "all";
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
  const selection = currentSemanticSelection();
  const selected = Boolean(
    line.entityId && selection.entityId === line.entityId &&
    (!selection.occurrenceId || !line.occurrenceId || selection.occurrenceId === line.occurrenceId),
  );
  const related = Boolean(line.entityId && selection.entityId === line.entityId && !selected);
  const indent = Number(line.indent || 0);
  const style = semanticLineHasValue(line) ? `style="--semantic-key-width:${keyWidths.get(indent) || 12}ch"` : "";
  const classes = semanticLineClassNames(line, selected, related);
  const sourceObjectID = line.sourceAnchor?.objectId || "";
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
    `data-entity-id="${escapeHTML(line.entityId || "")}"`,
    `data-entity-kind="${escapeHTML(line.entityKind || "")}"`,
    `data-occurrence-id="${escapeHTML(line.occurrenceId || "")}"`,
    `data-semantic-path="${escapeHTML(line.semanticPath || "")}"`,
    `data-preferred-view="${escapeHTML(line.preferredView || "")}"`,
    `data-preferred-target-id="${escapeHTML(line.preferredTargetId || "")}"`,
    `data-source-object-id="${escapeHTML(sourceObjectID)}"`,
    `tabindex="${line.entityId ? "0" : "-1"}"`,
    `role="treeitem"`,
    `aria-selected="${selected ? "true" : "false"}"`,
    `aria-current="${selected ? "location" : "false"}"`,
    `aria-describedby="semanticNavigationHelp"`,
  ].join(" ");
  return `<div class="${classes}" ${style} ${attrs}>${renderSemanticLineContent(line)}${renderSemanticTargetChips(line)}</div>`;
}

function semanticLineClassNames(line, selected, related = false) {
  const classes = ["semantic-line"];
  if (selected) {
    classes.push("selected");
  }
  if (related) {
    classes.push("related");
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

function semanticSelectionForLine(line) {
  const navigation = state.semanticProjection?.navigation || {};
  const occurrenceId = line.dataset.occurrenceId || "";
  const entityId = line.dataset.entityId || "";
  const occurrence = (navigation.occurrences || []).find((item) => item.occurrenceId === occurrenceId) || null;
  const entity = (navigation.entities || []).find((item) => item.id === (entityId || occurrence?.entityId)) || null;
  const objectIndex = line.dataset.objectIndex === "" ? null : Number(line.dataset.objectIndex);
  const fieldIndex = line.dataset.fieldIndex === "" ? null : Number(line.dataset.fieldIndex);
  const sourceAnchor = occurrence?.sourceAnchor || entity?.sourceAnchors?.[0] || (
    objectIndex === null && !line.dataset.sourceObjectId
      ? null
      : {
          objectId: line.dataset.sourceObjectId || "",
          objectIndex,
          objectType: line.dataset.objectType || "",
          fieldIndex,
        }
  );
  return {
    entityId: entityId || occurrence?.entityId || "",
    entityKind: line.dataset.entityKind || entity?.kind || "",
    occurrenceId,
    sourceAnchor,
    originView: "input-semantic",
    originTargetId: line.dataset.preferredTargetId || "",
    semanticPathHint: line.dataset.semanticPath || occurrence?.path || "",
    relatedEntityIds: entity?.relatedEntityIds || [],
  };
}

function semanticAvailableViews(selection) {
  if (!selection?.entityId) {
    return [];
  }
  const cacheKey = `${selection.entityId}\u0000${selection.occurrenceId || ""}`;
  if (semanticTargetViewCache.has(cacheKey)) {
    return semanticTargetViewCache.get(cacheKey);
  }
  const views = ["metrics", "topology", "profile", "hvac", "simulation"]
    .map((view) => ({ view, targets: selectionTargetsForView(view, selection) }))
    .filter((item) => item.targets.length);
  semanticTargetViewCache.set(cacheKey, views);
  return views;
}

function renderSemanticTargetChips(line) {
  if (!line.entityId) {
    return "";
  }
  const selection = semanticSelectionForProjectionLine(line);
  const views = semanticAvailableViews(selection);
  if (!views.length) {
    return "";
  }
  const ordered = [...views].sort((left, right) => (
    Number(right.view === line.preferredView) - Number(left.view === line.preferredView) || left.view.localeCompare(right.view)
  ));
  return `<span class="semantic-target-chips" aria-label="${escapeHTML(t("semantic.views", {}, "Views"))}">
    ${ordered.map(({ view, targets }) => `<button class="semantic-target-chip" type="button" data-semantic-target-view="${escapeHTML(view)}" data-semantic-target-id="${escapeHTML(targets[0]?.targetId || "")}" aria-label="${escapeHTML(t("semantic.openInView", { view: semanticViewLabel(view) }, `Open in ${semanticViewLabel(view)}`))}">${escapeHTML(semanticViewLabel(view))}</button>`).join("")}
  </span>`;
}

function semanticSelectionForProjectionLine(line = {}) {
  return {
    entityId: line.entityId || "",
    entityKind: line.entityKind || "",
    occurrenceId: line.occurrenceId || "",
    sourceAnchor: line.sourceAnchor || null,
    originView: "input-semantic",
    originTargetId: line.preferredTargetId || "",
    semanticPathHint: line.semanticPath || "",
  };
}

function semanticViewLabel(view) {
  return String(view || "").slice(0, 1).toUpperCase() + String(view || "").slice(1);
}

function renderSemanticSelectionContext(selection = currentSemanticSelection()) {
  if (!selection?.entityId) {
    return "";
  }
  const navigation = state.semanticProjection?.navigation || {};
  const entity = (navigation.entities || []).find((item) => item.id === selection.entityId) || {};
  const choices = semanticOccurrenceChoices(selection, { originView: selection.originView });
  const views = semanticAvailableViews(selection);
  const sourceAnchor = selection.sourceAnchor || {};
  const source = [sourceAnchor.objectType, sourceAnchor.objectName, sourceAnchor.fieldName].filter(Boolean).join(" / ") ||
    sourceAnchor.objectId || "—";
  return `<section class="semantic-context-bar" aria-label="Semantic selection">
    <div class="semantic-context-bar__identity"><span class="semantic-context-bar__label">${escapeHTML(t("semantic.selected", {}, "Selected"))}</span><span class="semantic-context-bar__value">${escapeHTML(entity.label || selection.semanticPathHint || selection.entityId)}</span></div>
    <div class="semantic-context-bar__item"><span class="semantic-context-bar__label">${escapeHTML(t("semantic.source", {}, "Source"))}</span><span class="semantic-context-bar__value semantic-context-bar__source">${escapeHTML(source)}</span></div>
    <div class="semantic-context-bar__views"><span class="semantic-context-bar__label">${escapeHTML(t("semantic.views", {}, "Views"))}</span>${views.map(({ view }) => `<button type="button" data-semantic-context-view="${escapeHTML(view)}">${escapeHTML(semanticViewLabel(view))}</button>`).join("")}</div>
    <div class="semantic-context-bar__actions">
      <button type="button" data-semantic-context-action="source" ${selection.sourceAnchor ? "" : "disabled"}>${escapeHTML(t("semantic.revealSource", {}, "Reveal source"))}</button>
      <button type="button" data-semantic-context-action="occurrences" ${choices.length > 1 ? "" : "disabled"}>${escapeHTML(t("semantic.occurrences", { count: choices.length }, `Occurrences ${choices.length}`))}</button>
      <button type="button" data-semantic-context-action="clear">${escapeHTML(t("semantic.clearSelection", {}, "Clear"))}</button>
    </div>
    ${renderSemanticOccurrenceChooser(selection, choices)}
  </section>`;
}

function renderSemanticOccurrenceChooser(selection, choices = semanticOccurrenceChoices(selection)) {
  if (!selection?.entityId || choices.length < 2) {
    return "";
  }
  const first = choices[0];
  return `<div class="semantic-occurrence-chooser" data-semantic-occurrence-chooser role="listbox" aria-label="${escapeHTML(t("semantic.chooseOccurrence", {}, "Choose where to reveal this item"))}" hidden>
    <p class="semantic-occurrence-chooser__heading">${escapeHTML(t("semantic.appearsIn", { name: selection.entityId }, `${selection.entityId} appears in:`))}</p>
    <ul class="semantic-occurrence-chooser__list">
      ${choices.map((choice) => {
        const canonical = choice.contextKind === "definition" || /(^|\/)definitions?(\/|$)/i.test(choice.path || "");
        return `<li><button class="semantic-occurrence-chooser__option" type="button" role="option" data-semantic-occurrence-id="${escapeHTML(choice.occurrenceId)}" aria-selected="${choice.occurrenceId === selection.occurrenceId ? "true" : "false"}"><span class="semantic-occurrence-chooser__path">${escapeHTML(choice.path || choice.occurrenceId)}</span><span class="semantic-occurrence-chooser__meta">${escapeHTML(canonical ? `Definition / canonical` : choice.contextKind || (choice.occurrenceId === first?.occurrenceId ? "First" : "Context"))}</span></button></li>`;
      }).join("")}
    </ul>
  </div>`;
}

function renderSemanticTemporaryReveal() {
  if (!state.semanticTemporaryReveal) {
    return "";
  }
  return `<div class="semantic-temporary-reveal" role="status">${escapeHTML(t("semantic.temporaryReveal", {}, "Temporarily revealing selected item"))}<button class="semantic-temporary-reveal__clear" type="button" data-semantic-clear-temporary>${escapeHTML(t("semantic.clearTemporaryReveal", {}, "Clear temporary reveal"))}</button></div>`;
}

function refreshSemanticSelectionContext() {
  const host = elements.semanticEditor?.querySelector("#semanticSelectionContext");
  if (!host) {
    return;
  }
  host.innerHTML = renderSemanticSelectionContext(currentSemanticSelection());
}

function bindSemanticControls() {
  if (!semanticControlsBound) {
    elements.semanticEditor.addEventListener("click", handleSemanticEditorClick);
    elements.semanticEditor.addEventListener("dblclick", handleSemanticEditorDoubleClick);
    elements.semanticEditor.addEventListener("keydown", handleSemanticEditorKeydown);
    elements.semanticEditor.addEventListener("pointerover", handleSemanticEditorPointerOver);
    elements.semanticEditor.addEventListener("pointerout", handleSemanticEditorPointerOut);
    semanticControlsBound = true;
  }
  bindSemanticStickyPath();
}

function handleSemanticEditorClick(event) {
  const target = event.target instanceof Element ? event.target : null;
  if (!target) {
    return;
  }
  const sectionButton = target.closest("[data-semantic-section-text]");
  if (sectionButton) {
    scrollSemanticSectionIntoView(sectionButton.dataset.semanticSectionText || "");
    return;
  }
  const targetButton = target.closest("[data-semantic-target-view]");
  if (targetButton) {
    event.stopPropagation();
    const line = targetButton.closest(".semantic-line");
    if (line) {
      openSemanticLine(line, targetButton.dataset.semanticTargetView, targetButton.dataset.semanticTargetId);
    }
    return;
  }
  const valueButton = target.closest(".semantic-value-token");
  if (valueButton) {
    event.stopPropagation();
    editSemanticValue(valueButton);
    return;
  }
  const contextView = target.closest("[data-semantic-context-view]");
  if (contextView) {
    event.stopPropagation();
    openSelectionInView(contextView.dataset.semanticContextView, {
      originView: "input-semantic",
      action: "open",
      recordHistory: true,
    });
    return;
  }
  const contextAction = target.closest("[data-semantic-context-action]")?.dataset.semanticContextAction || "";
  if (contextAction === "source") {
    revealSelectionSource({ originView: "input-semantic", action: "reveal_source", recordHistory: true });
    return;
  }
  if (contextAction === "clear") {
    clearSemanticSelection();
    return;
  }
  if (contextAction === "occurrences") {
    const chooser = elements.semanticEditor.querySelector("[data-semantic-occurrence-chooser]");
    if (chooser) {
      chooser.hidden = !chooser.hidden;
      if (!chooser.hidden) {
        chooser.querySelector("[role='option']")?.focus();
      }
    }
    return;
  }
  const occurrenceButton = target.closest("[data-semantic-occurrence-id]");
  if (occurrenceButton) {
    chooseSemanticOccurrence(occurrenceButton.dataset.semanticOccurrenceId);
    return;
  }
  if (target.closest("[data-semantic-clear-temporary]")) {
    clearSemanticTemporaryReveal();
    return;
  }
  const line = target.closest(".semantic-line[data-entity-id]:not([data-entity-id=''])");
  if (!line || target.closest("button, input, [data-semantic-target-view]") || event.detail > 1) {
    return;
  }
  const selection = semanticSelectionForLine(line);
  semanticClickHistoryKey = semanticSelectionIdentityKey(currentSemanticSelection()) === semanticSelectionIdentityKey(selection)
    ? ""
    : semanticSelectionIdentityKey(selection);
  selectSemanticLine(line);
}

function handleSemanticEditorDoubleClick(event) {
  const target = event.target instanceof Element ? event.target : null;
  const line = target?.closest(".semantic-line[data-entity-id]:not([data-entity-id=''])");
  if (!line || target.closest("button, input")) {
    return;
  }
  event.preventDefault();
  const historyAlreadyRecorded = semanticClickHistoryKey === semanticSelectionIdentityKey(semanticSelectionForLine(line));
  semanticClickHistoryKey = "";
  openSemanticLine(line, "", "", { historyAlreadyRecorded });
}

function handleSemanticEditorKeydown(event) {
  if (event.key !== "Enter" || event.altKey) {
    return;
  }
  const target = event.target instanceof Element ? event.target : null;
  const line = target?.closest(".semantic-line[data-entity-id]:not([data-entity-id=''])");
  if (!line || (target !== line && target?.closest("button, input"))) {
    return;
  }
  event.preventDefault();
  openSemanticLine(line);
}

function handleSemanticEditorPointerOver(event) {
  const target = event.target instanceof Element ? event.target : null;
  const line = target?.closest(".semantic-line[data-entity-id]:not([data-entity-id=''])");
  const related = event.relatedTarget instanceof Node ? event.relatedTarget : null;
  if (!line || (related && line.contains(related))) {
    return;
  }
  line.classList.add("hovered");
  hoverSemanticEntity(semanticSelectionForLine(line), { originView: "input-semantic", action: "hover" });
}

function handleSemanticEditorPointerOut(event) {
  const target = event.target instanceof Element ? event.target : null;
  const line = target?.closest(".semantic-line[data-entity-id]:not([data-entity-id=''])");
  const related = event.relatedTarget instanceof Node ? event.relatedTarget : null;
  if (!line || (related && line.contains(related))) {
    return;
  }
  line.classList.remove("hovered");
  clearSemanticHover();
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

async function selectSemanticLine(line) {
  const selection = semanticSelectionForLine(line);
  if (!selection.entityId) {
    return false;
  }
  state.semanticCurrentOccurrenceId = selection.occurrenceId || "";
  state.semanticCurrentPath = selection.semanticPathHint || "";
  await selectSemanticEntity(selection, {
    originView: "input-semantic",
    action: "select",
    recordHistory: true,
    follow: true,
    preserveFilters: true,
  });
  renderSemanticSelectionOnly();
  refreshSemanticSelectionContext();
  return true;
}

async function openSemanticLine(line, requestedView = "", requestedTargetId = "", options = {}) {
  const selection = semanticSelectionForLine(line);
  if (!selection.entityId) {
    return false;
  }
  const current = currentSemanticSelection();
  const selectionChanged = semanticSelectionIdentityKey(current) !== semanticSelectionIdentityKey(selection);
  await selectSemanticEntity(selection, {
    originView: "input-semantic",
    action: "select",
    recordHistory: selectionChanged,
    follow: false,
    preserveFilters: true,
  });
  state.semanticCurrentOccurrenceId = selection.occurrenceId || "";
  state.semanticCurrentPath = selection.semanticPathHint || "";
  const view = requestedView || line.dataset.preferredView || semanticAvailableViews(selection)[0]?.view || "";
  if (!view) {
    setStatus(t("semantic.noAvailableView", {}, "No available view can reveal this selection"), "warn");
    return false;
  }
  const opened = await openSelectionInView(view, {
    originView: "input-semantic",
    action: "open",
    recordHistory: options.historyAlreadyRecorded ? false : !selectionChanged,
    follow: false,
    preserveFilters: true,
    targetId: requestedTargetId || line.dataset.preferredTargetId || "",
  });
  renderSemanticSelectionOnly();
  refreshSemanticSelectionContext();
  return opened;
}

function semanticSelectionIdentityKey(selection = {}) {
  const anchor = selection.sourceAnchor || {};
  return [selection.entityId, selection.occurrenceId, anchor.objectId, anchor.objectIndex, anchor.fieldIndex]
    .map((value) => String(value ?? ""))
    .join("\u0000");
}

function renderSemanticSelectionOnly() {
  const selection = currentSemanticSelection();
  const hover = state.globalHover || {};
  elements.semanticEditor?.querySelectorAll(".semantic-line[data-entity-id]").forEach((line) => {
    const sameEntity = Boolean(selection.entityId && line.dataset.entityId === selection.entityId);
    const selected = sameEntity && (
      !selection.occurrenceId || !line.dataset.occurrenceId || line.dataset.occurrenceId === selection.occurrenceId
    );
    line.classList.toggle("selected", selected);
    line.classList.toggle("related", sameEntity && !selected);
    line.classList.toggle("hovered", Boolean(
      hover.entityId && line.dataset.entityId === hover.entityId &&
      (!hover.occurrenceId || line.dataset.occurrenceId === hover.occurrenceId),
    ));
    line.setAttribute("aria-selected", selected ? "true" : "false");
    line.setAttribute("aria-current", selected ? "location" : "false");
  });
}

async function chooseSemanticOccurrence(occurrenceId) {
  const current = currentSemanticSelection();
  const choice = semanticOccurrenceChoices(current).find((item) => item.occurrenceId === occurrenceId);
  if (!choice) {
    return false;
  }
  const selection = {
    ...current,
    occurrenceId,
    sourceAnchor: choice.sourceAnchor || current.sourceAnchor,
    semanticPathHint: choice.path || current.semanticPathHint,
  };
  await selectSemanticEntity(selection, {
    originView: "input-semantic",
    action: "select",
    recordHistory: true,
    follow: false,
    preserveFilters: true,
    rememberForOriginView: current.originView || "input-semantic",
  });
  ensureSemanticOccurrenceVisible(selection);
  return true;
}

function ensureSemanticOccurrenceVisible(selection = currentSemanticSelection(), options = {}) {
  const projection = state.semanticProjection;
  if (!selection?.entityId || !Array.isArray(projection?.lines)) {
    return false;
  }
  const occurrenceId = selection.occurrenceId || "";
  const lineIndex = projection.lines.findIndex((line) => occurrenceId
    ? line.occurrenceId === occurrenceId
    : line.entityId === selection.entityId);
  if (lineIndex < 0) {
    return false;
  }
  if (!(state.semanticExpandedSectionIds instanceof Set)) {
    state.semanticExpandedSectionIds = new Set(["project"]);
  }
  for (let index = lineIndex; index >= 0; index -= 1) {
    if (semanticTopLevelSectionLine(projection.lines[index])) {
      state.semanticExpandedSectionIds.add(semanticSectionId(projection.lines[index]));
      break;
    }
  }
  state.semanticTemporaryReveal = null;
  const currentlyVisible = semanticVisibleLines(projection.lines, currentInputFilterTerms()).some((line) => (
    occurrenceId ? line.occurrenceId === occurrenceId : line.entityId === selection.entityId
  ));
  if (options.temporary !== false && !currentlyVisible) {
    state.semanticTemporaryReveal = {
      entityId: selection.entityId,
      occurrenceId,
      semanticPathHint: selection.semanticPathHint || projection.lines[lineIndex].semanticPath || "",
    };
  }
  state.semanticCurrentOccurrenceId = occurrenceId;
  state.semanticCurrentPath = selection.semanticPathHint || projection.lines[lineIndex].semanticPath || "";
  if (state.activeInputView === "semantic") {
    renderSemanticView();
    window.requestAnimationFrame(() => {
      const target = [...elements.semanticEditor.querySelectorAll(".semantic-line")].find((line) => (
        occurrenceId ? line.dataset.occurrenceId === occurrenceId : line.dataset.entityId === selection.entityId
      ));
      target?.scrollIntoView({ block: "center", inline: "nearest" });
      target?.focus?.({ preventScroll: true });
    });
  }
  return true;
}

function clearSemanticTemporaryReveal() {
  state.semanticTemporaryReveal = null;
  if (state.activeInputView === "semantic") {
    renderSemanticView();
  }
}

function captureSemanticEditSelection() {
  const selection = currentSemanticSelection();
  if (!selection.entityId) {
    state.semanticEditSelectionRestore = null;
    return null;
  }
  const panelView = state.activeResultTab || "metrics";
  const adapter = getPanelNavigationAdapter(panelView);
  state.semanticEditSelectionRestore = {
    selection,
    parentEntityId: selection.relatedEntityIds?.[0] || "",
    objectCount: state.report?.objects?.length || 0,
    panelView,
    panelContext: adapter?.captureContext?.() || null,
  };
  return state.semanticEditSelectionRestore;
}

async function restoreSemanticSelectionAfterAnalysis(selection = currentSemanticSelection()) {
  ensureSemanticOccurrenceVisible(selection, { temporary: true });
  const restore = state.semanticEditSelectionRestore;
  if (!restore) {
    return;
  }
  state.semanticEditSelectionRestore = null;
  const adapter = getPanelNavigationAdapter(restore.panelView);
  await adapter?.restoreContext?.(restore.panelContext || {});
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
    captureSemanticEditSelection();
    const result = await api.ApplySemanticDuplicateNameFixText(getDocumentText());
    setDocumentText(result.text);
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
    return;
  }

  const formatLabel = state.model?.format || "unknown";
  const groups = groupedReportObjects();
  elements.textObjectView.innerHTML = `
    <div class="json-meta">
      <span class="badge">${escapeHTML(formatLabel)}</span>
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
    <div class="json-meta">
      <span class="badge">${escapeHTML(model.format || "unknown")}</span>
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
    setStatus(t("status.backendUnavailable"), "warn");
    restore();
    return;
  }

  try {
    captureSemanticEditSelection();
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
    state.semanticProjection = result.semantic || null;
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
    console.debug("Field suggestions unavailable", error);
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
    captureSemanticEditSelection();
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
  if (viewName === "semantic") {
    ensureSemanticOccurrenceVisible(selection, { temporary: true });
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
