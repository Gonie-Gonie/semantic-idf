const state = {
  result: null,
  activeRunID: "",
  orientation: "metrics",
  running: false,
  progressFiles: new Map(),
  progressListenerRegistered: false,
};

const elements = {
  selectButton: document.querySelector("#multiSummarySelect"),
  exportButton: document.querySelector("#multiSummaryExport"),
  stats: document.querySelector("#multiSummaryStats"),
  status: document.querySelector("#multiSummaryStatus"),
  percent: document.querySelector("#multiSummaryPercent"),
  progressBar: document.querySelector("#multiSummaryProgressBar"),
  fileList: document.querySelector("#multiSummaryFiles"),
  table: document.querySelector("#multiSummaryTable"),
  orientationButtons: document.querySelectorAll("[data-summary-orientation]"),
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
    elements.status.textContent = `Analyzed ${completed} of ${total} files (${succeeded} ok, ${failed} failed)`;
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
  elements.stats.textContent = "Waiting for file selection";
  elements.status.textContent = "Opening file dialog";
  elements.table.innerHTML = `<div class="empty">Analysis will start after files are selected.</div>`;
  elements.fileList.innerHTML = "";
  updateProgress(0, 0);
  waitForProgressRuntime();

  try {
    const result = await analyzeMultiSummary(state.activeRunID);
    if (result?.canceled) {
      elements.stats.textContent = "No files selected";
      elements.status.textContent = "File selection canceled";
      elements.table.innerHTML = `<div class="empty">Select one or more IDF or epJSON files to build a comparison table.</div>`;
      return;
    }
    state.result = result;
    updateProgress(result.completed || 0, result.total || 0, result.succeeded || 0, result.failed || 0);
    renderResult();
  } catch (error) {
    elements.status.textContent = error?.message || String(error);
    elements.stats.textContent = "Analysis failed";
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

  throw new Error(responseError || "Multi-IDF Summary is available in the desktop app.");
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
  elements.stats.textContent = `${total} files, ${succeeded} ok, ${failed} failed, ${workers} workers`;
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
      const detail = status === "ok" ? (file.filename && file.filename !== file.label ? file.filename : "Analyzed") : file.error || "Failed";
      return `
        <div class="tool-file-item ${status}">
          <strong>${escapeHTML(file.label || file.filename || "Input file")}</strong>
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
    elements.table.innerHTML = `<div class="empty">No summary data available.</div>`;
    return;
  }
  elements.table.innerHTML = state.orientation === "files" ? renderFilesAsRows(metrics, files) : renderMetricsAsRows(metrics, files);
}

function renderMetricsAsRows(metrics, files) {
  return `
    <table class="tool-table">
      <thead>
        <tr>
          <th class="tool-sticky-col">name</th>
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
          <th class="tool-sticky-col">building</th>
          ${metrics.map((metric) => `<th>${escapeHTML(metric.csvName || metric.id)}</th>`).join("")}
        </tr>
      </thead>
      <tbody>
        ${files
          .map((file) => `
            <tr>
              <th class="tool-sticky-col">
                ${renderFileLabel(file)}
                ${file.status === "ok" ? "" : `<span>${escapeHTML(file.error || "Failed")}</span>`}
              </th>
              ${metrics.map((metric) => renderValueCell(file, metric.id)).join("")}
            </tr>`)
          .join("")}
      </tbody>
    </table>`;
}

function renderFileLabel(file) {
  const label = file.label || file.filename || "Input file";
  const detail = file.filename && file.filename !== label ? file.filename : "";
  return `<strong>${escapeHTML(label)}</strong>${detail ? `<span>${escapeHTML(detail)}</span>` : ""}`;
}

function renderValueCell(file, metricID) {
  if (file.status !== "ok") {
    return `<td class="tool-value error"></td>`;
  }
  const value = file.metricValues?.[metricID];
  const status = value?.status || "missing";
  return `<td class="tool-value ${escapeHTML(status)}" title="${escapeHTML(status)}">${escapeHTML(value?.displayValue ?? "N/A")}</td>`;
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
elements.orientationButtons.forEach((button) => {
  button.addEventListener("click", () => {
    state.orientation = button.dataset.summaryOrientation;
    elements.orientationButtons.forEach((item) => item.classList.toggle("active", item === button));
    renderTable();
  });
});

registerProgressListener();
