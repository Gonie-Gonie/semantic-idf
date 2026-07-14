import { backend, elements, escapeHTML, setStatus, state, updateTextStats } from "../state.js";
import { markDocumentChanged, scheduleAnalyzeAfterPaint } from "../actions.js";
import { t } from "../i18n.js";
import {
  captureDiagnoseNavigationContext,
  restoreDiagnoseNavigationContext,
} from "./analysis-views.js";

const compactFormattingRuleID = "compact_formatting";
let refreshTimer = 0;
let pendingFixNavigationContext = null;

export function initializeDiagnoseFixes() {
  if (!elements.diagnoseFixRules || !elements.diagnoseFixCandidates) {
    return;
  }
  elements.diagnoseFixRefresh?.addEventListener("click", () => refreshDiagnoseFixes({ force: true }));
  elements.diagnoseFixPreview?.addEventListener("click", previewDiagnoseFixes);
  elements.diagnoseFixApply?.addEventListener("click", applyDiagnoseFixes);
  elements.diagnoseFixSaveAs?.addEventListener("click", saveDiagnoseFixCopy);
  elements.diagnoseFixCandidateFilter?.addEventListener("input", () => {
    state.diagnoseFixCandidateFilter = elements.diagnoseFixCandidateFilter.value || "";
    renderDiagnoseFixCandidates();
  });
  window.addEventListener("idfAnalyzer:documentChanged", () => {
    state.diagnoseFixPreview = null;
    renderDiagnoseFixStale();
    scheduleFixRefresh();
  });
  window.addEventListener("idfAnalyzer:analysisComplete", () => {
    scheduleFixRefresh(80);
    if (pendingFixNavigationContext) {
      const context = pendingFixNavigationContext;
      pendingFixNavigationContext = null;
      window.requestAnimationFrame(async () => {
        const restored = await restoreDiagnoseNavigationContext(context, { afterApply: true });
        if (restored.resolved) {
          elements.diagnoseFixStatus.textContent = `Resolved · ${context.selectedDiagnosticID || "diagnostic"}`;
        }
      });
    }
  });
  renderDiagnoseFixStale();
}

function scheduleFixRefresh(delay = 700) {
  window.clearTimeout(refreshTimer);
  refreshTimer = window.setTimeout(() => {
    refreshTimer = 0;
    refreshDiagnoseFixes();
  }, delay);
}

async function refreshDiagnoseFixes({ force = false } = {}) {
  const text = currentText();
  if (!text.trim()) {
    renderNoCurrentInput();
    return;
  }
  if (!force && state.diagnoseFixScan?.text === text) {
    renderDiagnoseFixScan();
    return;
  }
  setFixBusy(true);
  elements.diagnoseFixStatus.textContent = t("diagnoseFix.scanning", {}, "Scanning current input for suggested fixes.");
  try {
    const result = await callCleanupScan(text);
    state.diagnoseFixScan = {
      ...result,
      text,
    };
    state.diagnoseFixPreview = null;
    initializeFixSelection(result);
    renderDiagnoseFixScan();
  } catch (error) {
    elements.diagnoseFixStatus.textContent = error?.message || String(error);
    elements.diagnoseFixRules.innerHTML = `<div class="empty">${escapeHTML(error?.message || String(error))}</div>`;
    elements.diagnoseFixCandidates.innerHTML = "";
  } finally {
    setFixBusy(false);
  }
}

async function previewDiagnoseFixes() {
  if (!canRunFixes()) {
    return;
  }
  const navigationContext = captureDiagnoseNavigationContext();
  setFixBusy(true);
  try {
    const preview = await buildFixPreview();
    state.diagnoseFixPreview = preview;
    renderFixPreview(preview);
  } catch (error) {
    elements.diagnoseFixStatus.textContent = error?.message || String(error);
  } finally {
    setFixBusy(false);
    await restoreDiagnoseNavigationContext(navigationContext, { afterPreview: true });
  }
}

async function applyDiagnoseFixes() {
  if (!canRunFixes()) {
    return;
  }
  const navigationContext = captureDiagnoseNavigationContext();
  pendingFixNavigationContext = navigationContext;
  setFixBusy(true);
  try {
    const beforeDiagnostics = state.report?.diagnostics || [];
    const preview = state.diagnoseFixPreview || (await buildFixPreview());
    const afterDiagnostics = await analyzeDiagnostics(preview.text);
    elements.idfInput.value = preview.text || currentText();
    updateTextStats();
    markDocumentChanged();
    scheduleAnalyzeAfterPaint({
      loadingMessage: t("diagnoseFix.reanalyzing", {}, "Re-analyzing after selected fixes."),
      queuedMessage: t("diagnoseFix.applyQueued", {}, "Selected fixes applied. Analysis queued."),
      statusMessage: t("diagnoseFix.applied", { count: preview.removedCount || 0 }, "Selected fixes applied."),
      textSnapshot: elements.idfInput.value,
    });
    const newIssues = newIssueCodeCount(beforeDiagnostics, afterDiagnostics);
    elements.diagnoseFixStatus.textContent = t(
      "diagnoseFix.applyResult",
      { removed: preview.removedCount || 0, unresolved: afterDiagnostics.length || 0, added: newIssues },
      `${preview.removedCount || 0} removed, ${afterDiagnostics.length || 0} unresolved, ${newIssues} new issue codes.`,
    );
    state.diagnoseFixScan = null;
    state.diagnoseFixPreview = null;
    renderDiagnoseFixStale();
    setStatus(t("diagnoseFix.applied", { count: preview.removedCount || 0 }, "Selected fixes applied."), "ok");
  } catch (error) {
    pendingFixNavigationContext = null;
    await restoreDiagnoseNavigationContext(navigationContext, { afterApplyError: true });
    elements.diagnoseFixStatus.textContent = error?.message || String(error);
    setStatus(error?.message || String(error), "error");
  } finally {
    setFixBusy(false);
  }
}

async function saveDiagnoseFixCopy() {
  if (!canRunFixes()) {
    return;
  }
  setFixBusy(true);
  try {
    const result = await callSaveCleanupAs(currentText(), saveAsFilename(state.currentFilename || "model.idf"));
    if (result?.canceled) {
      return;
    }
    elements.diagnoseFixStatus.textContent = t(
      "diagnoseFix.savedCopy",
      { name: result.filename || t("common.inputFile"), count: result.removedCount || 0 },
      `Saved cleaned copy ${result.filename || ""}.`,
    );
  } catch (error) {
    elements.diagnoseFixStatus.textContent = error?.message || String(error);
  } finally {
    setFixBusy(false);
  }
}

async function callCleanupScan(text) {
  const api = backend();
  if (api && typeof api.ScanCleanupText === "function") {
    return api.ScanCleanupText(text, state.currentFilePath || "", state.currentFilename || "");
  }
  return postJSONAny(["/api/diagnose-fix-scan", "/api/cleanup-scan"], {
    text,
    path: state.currentFilePath || "",
    filename: state.currentFilename || "",
  });
}

async function buildFixPreview() {
  const api = backend();
  const payload = cleanupPayload();
  if (api && typeof api.PreviewCleanupText === "function") {
    return api.PreviewCleanupText(payload.text, payload.ruleIds, payload.excludedCandidateKeys);
  }
  return postJSONAny(["/api/diagnose-fix-preview", "/api/cleanup-preview"], payload);
}

async function callSaveCleanupAs(text, suggestedFilename) {
  const payload = {
    ...cleanupPayload(),
    text,
    suggestedFilename,
  };
  const api = backend();
  if (api && typeof api.SaveCleanupAs === "function") {
    return api.SaveCleanupAs(payload.text, payload.suggestedFilename, payload.ruleIds, payload.excludedCandidateKeys);
  }
  return postJSONAny(["/api/cleanup-save-as"], payload);
}

async function analyzeDiagnostics(text) {
  const api = backend();
  if (api && typeof api.AnalyzeInputDiagnosticsText === "function") {
    return api.AnalyzeInputDiagnosticsText(text);
  }
  const result = await postJSONAny(["/api/analyze-input"], { text });
  return result?.report?.diagnostics || [];
}

function renderDiagnoseFixScan() {
  const cleanup = state.diagnoseFixScan;
  const candidates = cleanup?.scan?.candidates || [];
  const rules = cleanup?.scan?.rules || [];
  elements.diagnoseFixStatus.textContent = candidates.length
    ? t("diagnoseFix.ready", { count: candidates.length }, `${candidates.length} fix candidates found.`)
    : t("tools.noCleanupCandidates", {}, "No cleanup candidates found.");
  elements.diagnoseFixRules.innerHTML = rules.map(renderFixRule).join("");
  elements.diagnoseFixRules.querySelectorAll("input[data-cleanup-rule]").forEach((input) => {
    input.addEventListener("change", () => {
      if (input.checked) {
        state.diagnoseFixSelectedRuleIDs.add(input.dataset.cleanupRule);
      } else {
        state.diagnoseFixSelectedRuleIDs.delete(input.dataset.cleanupRule);
      }
      state.diagnoseFixPreview = null;
      renderDiagnoseFixCandidates();
      updateFixButtons();
    });
  });
  renderDiagnoseFixCandidates();
  renderFixPreview(null);
  updateFixButtons();
}

function renderFixRule(rule) {
  const disabled = !rule.available ? "disabled" : "";
  const checked = state.diagnoseFixSelectedRuleIDs.has(rule.id) && rule.available ? "checked" : "";
  const status = rule.future ? t("tools.future") : rule.available ? t("tools.available") : t("tools.noCandidates");
  return `
    <label class="cleanup-rule ${rule.available ? "" : "disabled"}">
      <input data-cleanup-rule="${escapeHTML(rule.id)}" type="checkbox" ${checked} ${disabled} />
      <span>
        <strong>${escapeHTML(rule.name)}</strong>
        <small>${escapeHTML(rule.description)}</small>
        <em>${escapeHTML(status)}${rule.group ? ` / ${escapeHTML(rule.group)}` : ""}</em>
      </span>
    </label>`;
}

function renderDiagnoseFixCandidates() {
  const candidates = state.diagnoseFixScan?.scan?.candidates || [];
  const query = state.diagnoseFixCandidateFilter.trim().toLowerCase();
  const visible = candidates.filter((candidate) => candidateMatches(candidate, query));
  const selectedCount = selectedCandidates(candidates).length;
  elements.diagnoseFixCandidateStats.textContent = query
    ? t("tools.selectedShown", { selected: selectedCount, shown: visible.length })
    : t("tools.selectedOf", { selected: selectedCount, total: candidates.length });

  if (!candidates.length) {
    elements.diagnoseFixCandidates.innerHTML = `<div class="empty">${escapeHTML(t("tools.noCleanupCandidates", {}, "No cleanup candidates found."))}</div>`;
    updateFixButtons();
    return;
  }
  if (!visible.length) {
    elements.diagnoseFixCandidates.innerHTML = `<div class="empty">${escapeHTML(t("tools.noMatchingCandidates", {}, "No matching candidates."))}</div>`;
    updateFixButtons();
    return;
  }
  elements.diagnoseFixCandidates.innerHTML = `
    <div class="cleanup-candidate-list">
      ${visible.map(renderFixCandidate).join("")}
    </div>`;
  elements.diagnoseFixCandidates.querySelectorAll("input[data-cleanup-candidate]").forEach((input) => {
    input.addEventListener("change", () => {
      if (input.checked) {
        state.diagnoseFixExcludedCandidateKeys.delete(input.dataset.cleanupCandidate);
      } else {
        state.diagnoseFixExcludedCandidateKeys.add(input.dataset.cleanupCandidate);
      }
      state.diagnoseFixPreview = null;
      renderDiagnoseFixCandidates();
      updateFixButtons();
    });
  });
  updateFixButtons();
}

function renderFixCandidate(candidate) {
  const ruleSelected = state.diagnoseFixSelectedRuleIDs.has(candidate.ruleId);
  const excluded = state.diagnoseFixExcludedCandidateKeys.has(candidate.key);
  const selected = ruleSelected && !excluded;
  const objectLabel = candidate.objectName || `#${Number(candidate.objectIndex) + 1}`;
  return `
    <label class="cleanup-candidate ${selected ? "selected" : ""} ${ruleSelected ? "" : "inactive"}" ${fixCandidateNavigationAttributes(candidate)}>
      <input data-cleanup-candidate="${escapeHTML(candidate.key)}" type="checkbox" ${selected ? "checked" : ""} ${ruleSelected ? "" : "disabled"} />
      <span>
        <strong>${escapeHTML(objectLabel)}</strong>
        <small>${escapeHTML(candidate.objectType)} / ${escapeHTML(candidate.ruleId)}${candidate.source ? ` / ${escapeHTML(candidate.source)}` : ""}</small>
        <em>${escapeHTML(candidate.reason)}</em>
        <em class="cleanup-risk ${escapeHTML(candidate.risk || "review")}">${escapeHTML(candidate.risk || "review")}${candidate.relatedCodes?.length ? ` / ${escapeHTML(candidate.relatedCodes.join(", "))}` : ""}</em>
      </span>
    </label>`;
}

function renderFixPreview(preview) {
  if (!elements.diagnoseFixPreviewPanel) {
    return;
  }
  if (!preview) {
    elements.diagnoseFixPreviewPanel.hidden = true;
    elements.diagnoseFixPreviewPanel.innerHTML = "";
    return;
  }
  const removed = preview.removedCandidates || [];
  elements.diagnoseFixPreviewPanel.hidden = false;
  elements.diagnoseFixPreviewPanel.innerHTML = `
    <div class="diagnose-fix-preview-head">
      <strong>${escapeHTML(t("common.preview"))}</strong>
      <span>${escapeHTML(t("diagnoseFix.previewStats", { removed: preview.removedCount || 0, objects: preview.objectCount || 0 }, `${preview.removedCount || 0} removals, ${preview.objectCount || 0} objects after preview.`))}</span>
    </div>
    ${
      removed.length
        ? `<ul>${removed
            .slice(0, 80)
            .map((candidate) => `<li><strong>${escapeHTML(candidate.objectType)}</strong> ${escapeHTML(candidate.objectName || `#${Number(candidate.objectIndex) + 1}`)} <span>${escapeHTML(candidate.reason)}</span></li>`)
            .join("")}</ul>`
        : `<div class="empty">${escapeHTML(t("diagnoseFix.formattingOnly", {}, "Formatting-only preview."))}</div>`
    }`;
}

function renderDiagnoseFixStale() {
  elements.diagnoseFixStatus.textContent = t("diagnoseFix.pending", {}, "Scan current input for suggested fixes.");
  elements.diagnoseFixRules.innerHTML = `<div class="empty">${escapeHTML(t("diagnoseFix.pending", {}, "Scan current input for suggested fixes."))}</div>`;
  elements.diagnoseFixCandidates.innerHTML = `<div class="empty">${escapeHTML(t("diagnoseFix.pending", {}, "Scan current input for suggested fixes."))}</div>`;
  elements.diagnoseFixCandidateStats.textContent = t("tools.selectedOf", { selected: 0, total: 0 });
  renderFixPreview(null);
  updateFixButtons();
}

function renderNoCurrentInput() {
  state.diagnoseFixScan = null;
  elements.diagnoseFixStatus.textContent = t("tools.noCurrentInput", {}, "Open an input first.");
  elements.diagnoseFixRules.innerHTML = `<div class="empty">${escapeHTML(t("tools.noCurrentInputShort", {}, "No current input."))}</div>`;
  elements.diagnoseFixCandidates.innerHTML = `<div class="empty">${escapeHTML(t("tools.noCurrentInputShort", {}, "No current input."))}</div>`;
  elements.diagnoseFixCandidateStats.textContent = t("tools.selectedOf", { selected: 0, total: 0 });
  renderFixPreview(null);
  updateFixButtons();
}

function initializeFixSelection(result) {
  state.diagnoseFixSelectedRuleIDs = new Set(
    (result?.scan?.rules || []).filter((rule) => rule.default && rule.available).map((rule) => rule.id),
  );
  state.diagnoseFixExcludedCandidateKeys = new Set();
  state.diagnoseFixCandidateFilter = "";
  if (elements.diagnoseFixCandidateFilter) {
    elements.diagnoseFixCandidateFilter.value = "";
  }
}

function cleanupPayload() {
  return {
    text: currentText(),
    ruleIds: [...state.diagnoseFixSelectedRuleIDs],
    excludedCandidateKeys: [...state.diagnoseFixExcludedCandidateKeys],
  };
}

function selectedCandidates(candidates = state.diagnoseFixScan?.scan?.candidates || []) {
  return candidates.filter((candidate) => state.diagnoseFixSelectedRuleIDs.has(candidate.ruleId) && !state.diagnoseFixExcludedCandidateKeys.has(candidate.key));
}

function canRunFixes() {
  return Boolean(state.diagnoseFixScan) && (selectedCandidates().length > 0 || state.diagnoseFixSelectedRuleIDs.has(compactFormattingRuleID));
}

function setFixBusy(busy) {
  state.diagnoseFixBusy = busy;
  updateFixButtons();
  if (elements.diagnoseFixCandidateFilter) {
    elements.diagnoseFixCandidateFilter.disabled = busy || !state.diagnoseFixScan;
  }
}

function updateFixButtons() {
  const disabled = state.diagnoseFixBusy || !canRunFixes();
  if (elements.diagnoseFixPreview) {
    elements.diagnoseFixPreview.disabled = disabled;
  }
  if (elements.diagnoseFixApply) {
    elements.diagnoseFixApply.disabled = disabled;
  }
  if (elements.diagnoseFixSaveAs) {
    elements.diagnoseFixSaveAs.disabled = disabled;
  }
}

function fixCandidateNavigationAttributes(candidate = {}) {
  const navigation = state.semanticProjection?.navigation || {};
  const occurrenceIDs = navigation.byObjectIndex?.[String(candidate.objectIndex)] || [];
  const occurrences = occurrenceIDs
    .map((occurrenceID) => (navigation.occurrences || []).find((occurrence) => occurrence.occurrenceId === occurrenceID))
    .filter((occurrence) => occurrence && String(occurrence.contextKind || "") !== "diagnostic_occurrence")
    .sort((left, right) => fixOccurrencePriority(right) - fixOccurrencePriority(left));
  const occurrence = occurrences[0] || null;
  const entity = (navigation.entities || []).find((item) => item.id === occurrence?.entityId) || null;
  const sourceAnchor = occurrence?.sourceAnchor || {
    objectIndex: candidate.objectIndex,
    objectType: candidate.objectType || "",
    objectName: candidate.objectName || "",
  };
  const attributes = [];
  const add = (name, value) => {
    if (value === undefined || value === null || String(value) === "") {
      return;
    }
    attributes.push(`${name}="${escapeHTML(value)}"`);
  };
  add("data-entity-id", entity?.id || occurrence?.entityId);
  add("data-entity-kind", entity?.kind);
  add("data-occurrence-id", occurrence?.occurrenceId);
  add("data-occurrence-context", occurrence?.contextKind || occurrence?.path);
  add("data-semantic-path", occurrence?.path);
  add("data-source-object-id", sourceAnchor.objectId);
  add("data-source-object-index", sourceAnchor.objectIndex);
  add("data-source-object-type", sourceAnchor.objectType);
  add("data-source-object-name", sourceAnchor.objectName);
  add("data-source-field-index", sourceAnchor.fieldIndex);
  add("data-source-field-name", sourceAnchor.fieldName);
  add("data-panel-target-id", candidate.key);
  return attributes.join(" ");
}

function fixOccurrencePriority(occurrence = {}) {
  const contextPriority = {
    zone_service: 7,
    zone_profile: 6,
    zone_geometry: 5,
    zone_output: 4,
    definition: 3,
    source_only: 1,
  }[String(occurrence.contextKind || "")] || 2;
  return contextPriority * 100_000 + Number((occurrence.lineIndexes || []).length > 0) * 10_000;
}

function candidateMatches(candidate, query) {
  if (!query) {
    return true;
  }
  return [candidate.ruleId, candidate.objectType, candidate.objectName, candidate.reason, candidate.risk, candidate.source, ...(candidate.relatedCodes || [])].some((value) =>
    String(value ?? "").toLowerCase().includes(query),
  );
}

function currentText() {
  return elements.idfInput?.value || "";
}

function newIssueCodeCount(beforeDiagnostics, afterDiagnostics) {
  const before = new Set((beforeDiagnostics || []).map((item) => item.code).filter(Boolean));
  const after = new Set((afterDiagnostics || []).map((item) => item.code).filter(Boolean));
  let count = 0;
  for (const code of after) {
    if (!before.has(code)) {
      count += 1;
    }
  }
  return count;
}

async function postJSONAny(urls, payload) {
  let lastError = "";
  for (const url of urls) {
    try {
      const response = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload || {}),
      });
      if (response.ok) {
        return response.json();
      }
      lastError = await response.text();
    } catch (error) {
      lastError = error?.message || String(error);
    }
  }
  throw new Error(lastError || "Request failed");
}

function saveAsFilename(filename = "cleaned.idf") {
  const dot = filename.lastIndexOf(".");
  if (dot <= 0) {
    return `${filename}-cleaned.idf`;
  }
  return `${filename.slice(0, dot)}-cleaned${filename.slice(dot)}`;
}
