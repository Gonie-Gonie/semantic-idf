import { loadAndApplyAppSettings } from "./settings-client.js";
import { renderAppInfo } from "./app-info.js";
import { t } from "./i18n.js";

loadAndApplyAppSettings();
renderAppInfo();

const state = {
  activeTool: "multi-idf-summary",
  result: null,
  activeRunID: "",
  orientation: "metrics",
  running: false,
  progressFiles: new Map(),
  progressListenerRegistered: false,
  cleanup: null,
  cleanupSelectedRuleIDs: new Set(),
  cleanupExcludedCandidateKeys: new Set(),
  cleanupCandidateFilter: "",
};

const currentDocumentStorageKey = "idfAnalyzer.currentDocument";
const cleanupRuleCompactFormatting = "compact_formatting";

const elements = {
  toolNavButtons: document.querySelectorAll("[data-tool-tab]"),
  toolPanels: document.querySelectorAll("[data-tool-panel]"),
  selectButton: document.querySelector("#multiSummarySelect"),
  exportButton: document.querySelector("#multiSummaryExport"),
  stats: document.querySelector("#multiSummaryStats"),
  status: document.querySelector("#multiSummaryStatus"),
  percent: document.querySelector("#multiSummaryPercent"),
  progressBar: document.querySelector("#multiSummaryProgressBar"),
  fileList: document.querySelector("#multiSummaryFiles"),
  table: document.querySelector("#multiSummaryTable"),
  orientationButtons: document.querySelectorAll("[data-summary-orientation]"),
  cleanupSave: document.querySelector("#cleanupSave"),
  cleanupSaveAs: document.querySelector("#cleanupSaveAs"),
  cleanupDocumentLabel: document.querySelector("#cleanupDocumentLabel"),
  cleanupRules: document.querySelector("#cleanupRules"),
  cleanupCandidateFilter: document.querySelector("#cleanupCandidateFilter"),
  cleanupCandidateStats: document.querySelector("#cleanupCandidateStats"),
  cleanupCandidates: document.querySelector("#cleanupCandidates"),
};

function appAPI() {
  return window.go && window.go.main && window.go.main.App;
}

async function waitForAppAPI(methodName) {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    const api = appAPI();
    if (api && typeof api[methodName] === "function") {
      return api;
    }
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  return null;
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function registerProgressListener() {
  if (state.progressListenerRegistered || !window.runtime) {
    return;
  }
  if (typeof window.runtime.EventsOn === "function") {
    window.runtime.EventsOn("idfAnalyzer:multiSummaryProgress", handleProgress);
    state.progressListenerRegistered = true;
  } else if (typeof window.runtime.EventsOnMultiple === "function") {
    window.runtime.EventsOnMultiple("idfAnalyzer:multiSummaryProgress", handleProgress, -1);
    state.progressListenerRegistered = true;
  }
}

async function waitForProgressRuntime() {
  for (let attempt = 0; attempt < 40 && !state.progressListenerRegistered; attempt += 1) {
    registerProgressListener();
    if (state.progressListenerRegistered) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
}

function handleProgress(payload) {
  const progress = Array.isArray(payload) ? payload[0] : payload;
  if (!progress || progress.runId !== state.activeRunID) {
    return;
  }
  if (progress.file) {
    state.progressFiles.set(progress.file.index, progress.file);
    renderFileList([...state.progressFiles.values()]);
  }
  updateProgress(progress.completed || 0, progress.total || 0, progress.succeeded || 0, progress.failed || 0);
}

function updateProgress(completed, total, succeeded = 0, failed = 0) {
  const percent = total > 0 ? Math.round((completed / total) * 100) : 0;
  elements.progressBar.style.width = `${percent}%`;
  elements.percent.textContent = `${percent}%`;
  if (total > 0) {
    elements.status.textContent = t("tools.analyzedProgress", { completed, total, ok: succeeded, failed });
  }
}

function setRunning(running) {
  state.running = running;
  elements.selectButton.disabled = running;
  elements.exportButton.disabled = running || !state.result;
}

async function runMultiSummary() {
  state.result = null;
  state.progressFiles.clear();
  state.activeRunID = `multi-summary-${Date.now()}-${Math.random().toString(36).slice(2)}`;
  setRunning(true);
  elements.stats.textContent = t("tools.waitingSelection");
  elements.status.textContent = t("status.openDialog");
  elements.table.innerHTML = `<div class="empty">${t("status.analysisWillStart")}</div>`;
  elements.fileList.innerHTML = "";
  updateProgress(0, 0);
  waitForProgressRuntime();

  try {
    const result = await analyzeMultiSummary(state.activeRunID);
    if (result?.canceled) {
      elements.stats.textContent = t("tools.noFilesSelected");
      elements.status.textContent = t("status.fileSelectionCanceled");
      elements.table.innerHTML = `<div class="empty">${t("tools.selectFilesHelp")}</div>`;
      return;
    }
    state.result = result;
    updateProgress(result.completed || 0, result.total || 0, result.succeeded || 0, result.failed || 0);
    renderResult();
  } catch (error) {
    elements.status.textContent = error?.message || String(error);
    elements.stats.textContent = t("tools.analysisFailed");
    elements.table.innerHTML = `<div class="empty">${escapeHTML(error?.message || String(error))}</div>`;
  } finally {
    setRunning(false);
  }
}

async function analyzeMultiSummary(runID) {
  let responseError = "";
  try {
    const response = await fetch("/api/multi-idf-summary", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ runId: runID }),
    });
    if (response.ok) {
      return response.json();
    }
    responseError = await response.text();
  } catch (error) {
    responseError = error?.message || String(error);
  }

  const api = await waitForAppAPI("AnalyzeMultiIDFSummary");
  if (api) {
    return api.AnalyzeMultiIDFSummary(runID);
  }

  throw new Error(responseError || t("tools.desktopOnly"));
}

function renderResult() {
  const result = state.result;
  if (!result) {
    return;
  }
  const total = result.total || 0;
  const succeeded = result.succeeded || 0;
  const failed = result.failed || 0;
  const workers = result.concurrency || 0;
  elements.stats.textContent = t("count.filesSummary", { total, ok: succeeded, failed, workers });
  renderFileList(result.files || []);
  renderTable();
  elements.exportButton.disabled = state.running || !result.metrics?.length;
}

function renderFileList(files) {
  if (!files.length) {
    elements.fileList.innerHTML = "";
    return;
  }
  elements.fileList.innerHTML = files
    .slice()
    .sort((a, b) => (a.index || 0) - (b.index || 0))
    .map((file) => {
      const status = file.status === "ok" ? "ok" : "error";
      const detail = status === "ok" ? (file.filename && file.filename !== file.label ? file.filename : t("tools.analyzed")) : file.error || t("tools.failed");
      return `
        <div class="tool-file-item ${status}">
          <strong>${escapeHTML(file.label || file.filename || t("common.inputFile"))}</strong>
          <span>${escapeHTML(detail)}</span>
        </div>`;
    })
    .join("");
}

function renderTable() {
  const result = state.result;
  const metrics = result?.metrics || [];
  const files = result?.files || [];
  if (!metrics.length || !files.length) {
    elements.table.innerHTML = `<div class="empty">${t("tools.noSummaryData")}</div>`;
    return;
  }
  elements.table.innerHTML = state.orientation === "files" ? renderFilesAsRows(metrics, files) : renderMetricsAsRows(metrics, files);
}

function renderMetricsAsRows(metrics, files) {
  return `
    <table class="tool-table">
      <thead>
        <tr>
          <th class="tool-sticky-col">${t("common.name")}</th>
          ${files.map((file) => `<th>${renderFileLabel(file)}</th>`).join("")}
        </tr>
      </thead>
      <tbody>
        ${metrics
          .map((metric) => `
            <tr>
              <th class="tool-sticky-col">
                <strong>${escapeHTML(metric.csvName || metric.id)}</strong>
                <span>${escapeHTML(metric.category || "")}</span>
              </th>
              ${files.map((file) => renderValueCell(file, metric.id)).join("")}
            </tr>`)
          .join("")}
      </tbody>
    </table>`;
}

function renderFilesAsRows(metrics, files) {
  return `
    <table class="tool-table">
      <thead>
        <tr>
          <th class="tool-sticky-col">${t("common.building")}</th>
          ${metrics.map((metric) => `<th>${escapeHTML(metric.csvName || metric.id)}</th>`).join("")}
        </tr>
      </thead>
      <tbody>
        ${files
          .map((file) => `
            <tr>
              <th class="tool-sticky-col">
                ${renderFileLabel(file)}
                ${file.status === "ok" ? "" : `<span>${escapeHTML(file.error || t("tools.failed"))}</span>`}
              </th>
              ${metrics.map((metric) => renderValueCell(file, metric.id)).join("")}
            </tr>`)
          .join("")}
      </tbody>
    </table>`;
}

function renderFileLabel(file) {
  const label = file.label || file.filename || t("common.inputFile");
  const detail = file.filename && file.filename !== label ? file.filename : "";
  return `<strong>${escapeHTML(label)}</strong>${detail ? `<span>${escapeHTML(detail)}</span>` : ""}`;
}

function renderValueCell(file, metricID) {
  if (file.status !== "ok") {
    return `<td class="tool-value error"></td>`;
  }
  const value = file.metricValues?.[metricID];
  const status = value?.status || "missing";
  return `<td class="tool-value ${escapeHTML(status)}" title="${escapeHTML(status)}">${escapeHTML(value?.displayValue ?? t("common.notAvailable"))}</td>`;
}

function metricValueForCSV(file, metricID) {
  if (file.status !== "ok") {
    return "";
  }
  return file.metricValues?.[metricID]?.displayValue ?? "N/A";
}

function exportCSV() {
  const result = state.result;
  if (!result) {
    return;
  }
  const metrics = result.metrics || [];
  const files = result.files || [];
  const rows =
    state.orientation === "files"
      ? [["building", ...metrics.map((metric) => metric.csvName || metric.id)], ...files.map((file) => [file.label || file.filename, ...metrics.map((metric) => metricValueForCSV(file, metric.id))])]
      : [["name", ...files.map((file) => file.label || file.filename)], ...metrics.map((metric) => [metric.csvName || metric.id, ...files.map((file) => metricValueForCSV(file, metric.id))])];
  const csvText = `${rows.map((row) => row.map(csvCell).join(",")).join("\r\n")}\r\n`;
  const blob = new Blob([csvText], { type: "text/csv" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `multi-idf-summary-${state.orientation}.csv`;
  link.click();
  URL.revokeObjectURL(url);
}

function csvCell(value) {
  const text = String(value ?? "");
  return /[",\r\n]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text;
}

elements.selectButton.addEventListener("click", runMultiSummary);
elements.exportButton.addEventListener("click", exportCSV);
elements.toolNavButtons.forEach((button) => {
  button.addEventListener("click", () => switchToolTab(button.dataset.toolTab));
});
elements.orientationButtons.forEach((button) => {
  button.addEventListener("click", () => {
    state.orientation = button.dataset.summaryOrientation;
    elements.orientationButtons.forEach((item) => item.classList.toggle("active", item === button));
    renderTable();
  });
});

registerProgressListener();
switchToolTab(window.location.hash.replace(/^#/, "") || state.activeTool, { updateHash: false });
loadCleanupFromCurrentDocument();

elements.cleanupSave.addEventListener("click", () => saveCleanup(false));
elements.cleanupSaveAs.addEventListener("click", () => saveCleanup(true));
elements.cleanupCandidateFilter.addEventListener("input", () => {
  state.cleanupCandidateFilter = elements.cleanupCandidateFilter.value;
  renderCleanupCandidates();
});

async function loadCleanupFromCurrentDocument() {
  const currentDocument = readCurrentDocument();
  if (!currentDocument) {
    renderMissingCleanupDocument();
    return;
  }
  setCleanupBusy(true);
  elements.cleanupDocumentLabel.textContent = t("tools.currentInput", { name: currentDocument.filename || t("tools.untitledInput") });
  try {
    const result = await postJSON("/api/cleanup-scan", {
      text: currentDocument.text || "",
      path: currentDocument.path || "",
      filename: currentDocument.filename || "",
    });
    state.cleanup = result;
    initializeCleanupSelection(result);
    renderCleanupScan();
  } catch (error) {
    elements.cleanupDocumentLabel.textContent = error?.message || String(error);
  } finally {
    setCleanupBusy(false);
  }
}

async function buildCleanupPreview() {
  if (!state.cleanup) {
    return null;
  }
  return postJSON("/api/cleanup-preview", cleanupPayload());
}

async function saveCleanup(saveAs) {
  if (!state.cleanup) {
    return;
  }
  setCleanupBusy(true);
  try {
    const preview = await buildCleanupPreview();
    const endpoint = saveAs || !state.cleanup?.path ? "/api/cleanup-save-as" : "/api/cleanup-save";
    const result = await postJSON(endpoint, {
      ...cleanupPayload(),
      text: state.cleanup.text,
      path: state.cleanup.path || "",
      suggestedFilename: saveAs ? saveAsFilename(state.cleanup.filename) : state.cleanup.filename || "cleaned.idf",
    });
    if (result?.canceled) {
      return;
    }
    updateCurrentDocument({
      text: result.text || preview?.text || state.cleanup.text,
      path: result.path || state.cleanup.path || "",
      filename: result.filename || state.cleanup.filename || "",
    });
    await loadCleanupFromCurrentDocument();
    elements.cleanupDocumentLabel.textContent = t("tools.savedCleanup", {
      name: result.filename || state.cleanup?.filename || t("common.inputFile"),
      count: result.removedCount || 0,
    });
  } catch (error) {
    elements.cleanupDocumentLabel.textContent = error?.message || String(error);
  } finally {
    setCleanupBusy(false);
  }
}

function renderCleanupScan() {
  const cleanup = state.cleanup;
  const candidates = cleanup?.scan?.candidates || [];
  const rules = cleanup?.scan?.rules || [];
  elements.cleanupRules.innerHTML = rules.map(renderCleanupRule).join("");
  elements.cleanupRules.querySelectorAll("input[data-cleanup-rule]").forEach((input) => {
    input.addEventListener("change", () => {
      if (input.checked) {
        state.cleanupSelectedRuleIDs.add(input.dataset.cleanupRule);
      } else {
        state.cleanupSelectedRuleIDs.delete(input.dataset.cleanupRule);
      }
      renderCleanupCandidates();
    });
  });
  renderCleanupCandidates();
  updateCleanupButtons();
}

function renderCleanupRule(rule) {
  const disabled = !rule.available ? "disabled" : "";
  const checked = state.cleanupSelectedRuleIDs.has(rule.id) && rule.available ? "checked" : "";
  const status = rule.future ? t("tools.future") : rule.available ? t("tools.available") : t("tools.noCandidates");
  return `
    <label class="cleanup-rule ${rule.available ? "" : "disabled"}">
      <input data-cleanup-rule="${escapeHTML(rule.id)}" type="checkbox" ${checked} ${disabled} />
      <span>
        <strong>${escapeHTML(rule.name)}</strong>
        <small>${escapeHTML(rule.description)}</small>
        <em>${escapeHTML(status)}</em>
      </span>
    </label>`;
}

function renderCleanupCandidates() {
  const candidates = state.cleanup?.scan?.candidates || [];
  const query = state.cleanupCandidateFilter.trim().toLowerCase();
  const visible = candidates.filter((candidate) => cleanupCandidateMatches(candidate, query));
  const selectedCount = selectedCleanupCandidates(candidates).length;
  elements.cleanupCandidateStats.textContent = query
    ? t("tools.selectedShown", { selected: selectedCount, shown: visible.length })
    : t("tools.selectedOf", { selected: selectedCount, total: candidates.length });

  if (!candidates.length) {
    elements.cleanupCandidates.innerHTML = `<div class="empty">${t("tools.noCleanupCandidates")}</div>`;
    updateCleanupButtons();
    return;
  }
  if (!visible.length) {
    elements.cleanupCandidates.innerHTML = `<div class="empty">${t("tools.noMatchingCandidates")}</div>`;
    updateCleanupButtons();
    return;
  }
  elements.cleanupCandidates.innerHTML = `
    <div class="cleanup-candidate-list">
      ${visible.map(renderCleanupCandidate).join("")}
    </div>`;
  elements.cleanupCandidates.querySelectorAll("input[data-cleanup-candidate]").forEach((input) => {
    input.addEventListener("change", () => {
      if (input.checked) {
        state.cleanupExcludedCandidateKeys.delete(input.dataset.cleanupCandidate);
      } else {
        state.cleanupExcludedCandidateKeys.add(input.dataset.cleanupCandidate);
      }
      updateCleanupButtons();
      renderCleanupCandidates();
    });
  });
  updateCleanupButtons();
}

function renderCleanupCandidate(candidate) {
  const ruleSelected = state.cleanupSelectedRuleIDs.has(candidate.ruleId);
  const excluded = state.cleanupExcludedCandidateKeys.has(candidate.key);
  const selected = ruleSelected && !excluded;
  const objectLabel = candidate.objectName || `#${Number(candidate.objectIndex) + 1}`;
  return `
    <label class="cleanup-candidate ${selected ? "selected" : ""} ${ruleSelected ? "" : "inactive"}">
      <input data-cleanup-candidate="${escapeHTML(candidate.key)}" type="checkbox" ${selected ? "checked" : ""} ${ruleSelected ? "" : "disabled"} />
      <span>
        <strong>${escapeHTML(objectLabel)}</strong>
        <small>${escapeHTML(candidate.objectType)} / ${escapeHTML(candidate.ruleId)}</small>
        <em>${escapeHTML(candidate.reason)}</em>
      </span>
    </label>`;
}

function setCleanupBusy(busy) {
  elements.cleanupSave.disabled = busy || !canSaveCleanup();
  elements.cleanupSaveAs.disabled = busy || !canSaveCleanup();
  elements.cleanupCandidateFilter.disabled = busy || !state.cleanup;
}

function renderMissingCleanupDocument() {
  state.cleanup = null;
  state.cleanupSelectedRuleIDs.clear();
  state.cleanupExcludedCandidateKeys.clear();
  elements.cleanupDocumentLabel.textContent = t("tools.noCurrentInput");
  elements.cleanupRules.innerHTML = `<div class="empty">${t("tools.noCurrentInputShort")}</div>`;
  elements.cleanupCandidates.innerHTML = `<div class="empty">${t("tools.noCurrentInputShort")}</div>`;
  elements.cleanupCandidateStats.textContent = t("tools.selectedOf", { selected: 0, total: 0 });
  updateCleanupButtons();
}

function initializeCleanupSelection(result) {
  state.cleanupSelectedRuleIDs = new Set(
    (result?.scan?.rules || []).filter((rule) => rule.default && rule.available).map((rule) => rule.id),
  );
  state.cleanupExcludedCandidateKeys = new Set();
  state.cleanupCandidateFilter = "";
  elements.cleanupCandidateFilter.value = "";
}

function cleanupPayload() {
  return {
    text: state.cleanup?.text || "",
    ruleIds: selectedCleanupRuleIDs(),
    excludedCandidateKeys: [...state.cleanupExcludedCandidateKeys],
  };
}

function selectedCleanupRuleIDs() {
  return [...state.cleanupSelectedRuleIDs];
}

function selectedCleanupCandidates(candidates = state.cleanup?.scan?.candidates || []) {
  return candidates.filter((candidate) => state.cleanupSelectedRuleIDs.has(candidate.ruleId) && !state.cleanupExcludedCandidateKeys.has(candidate.key));
}

function canSaveCleanup() {
  return (
    Boolean(state.cleanup) &&
    (selectedCleanupCandidates().length > 0 || state.cleanupSelectedRuleIDs.has(cleanupRuleCompactFormatting))
  );
}

function updateCleanupButtons() {
  elements.cleanupSave.disabled = !canSaveCleanup();
  elements.cleanupSaveAs.disabled = !canSaveCleanup();
}

function cleanupCandidateMatches(candidate, query) {
  if (!query) {
    return true;
  }
  return [candidate.ruleId, candidate.objectType, candidate.objectName, candidate.reason]
    .some((value) => String(value ?? "").toLowerCase().includes(query));
}

function readCurrentDocument() {
  try {
    const raw = window.sessionStorage.getItem(currentDocumentStorageKey);
    if (!raw) {
      return null;
    }
    const currentDocument = JSON.parse(raw);
    return typeof currentDocument?.text === "string" && currentDocument.text.trim() ? currentDocument : null;
  } catch {
    return null;
  }
}

function updateCurrentDocument(nextDocument) {
  const currentDocument = {
    text: nextDocument.text || "",
    path: nextDocument.path || "",
    filename: nextDocument.filename || "cleaned.idf",
  };
  try {
    window.sessionStorage.setItem(currentDocumentStorageKey, JSON.stringify(currentDocument));
  } catch {
    // Session storage may be unavailable in hardened webview settings.
  }
  state.cleanup = {
    ...state.cleanup,
    ...currentDocument,
  };
}

async function postJSON(url, payload) {
  const response = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload || {}),
  });
  if (!response.ok) {
    throw new Error(await response.text() || `${url} failed`);
  }
  return response.json();
}

function saveAsFilename(filename = "cleaned.idf") {
  const dot = filename.lastIndexOf(".");
  if (dot <= 0) {
    return `${filename}-cleaned.idf`;
  }
  return `${filename.slice(0, dot)}-cleaned${filename.slice(dot)}`;
}

function switchToolTab(toolID, { updateHash = true } = {}) {
  const panel = [...elements.toolPanels].find((item) => item.dataset.toolPanel === toolID);
  if (!panel) {
    toolID = "multi-idf-summary";
  }
  state.activeTool = toolID;
  elements.toolNavButtons.forEach((button) => {
    const active = button.dataset.toolTab === toolID;
    button.classList.toggle("active", active);
    button.setAttribute("aria-current", active ? "page" : "false");
  });
  elements.toolPanels.forEach((item) => {
    item.classList.toggle("active", item.dataset.toolPanel === toolID);
  });
  if (updateHash) {
    window.history.replaceState(null, "", `#${toolID}`);
  }
}
