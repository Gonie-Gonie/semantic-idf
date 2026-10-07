import { getLanguage, t } from "./i18n.js";
import { getAuxiliaryHost } from "./auxiliary-context.js";

const escapeHTML = (value) => String(value ?? "")
  .replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;")
  .replaceAll('"', "&quot;").replaceAll("'", "&#039;");

function bytes(value) {
  let amount = Math.max(0, Number(value) || 0);
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) { amount /= 1024; unit += 1; }
  return `${new Intl.NumberFormat(getLanguage(), { maximumFractionDigits: unit ? 1 : 0 }).format(amount)} ${units[unit]}`;
}

const issueMessages = {
  unsafe_or_unreadable_path: ["settings.storageIssueUnsafe", "The location could not be safely read; kept."],
  scan_limit_reached: ["settings.storageIssueScanLimit", "The scan limit was reached; the displayed size is a minimum."],
  unrecognized_files: ["settings.storageIssueUserFiles", "Contains files not identified as app-generated; kept."],
  unknown_ownership: ["settings.storageIssueOwnership", "App ownership could not be verified; kept."],
  in_use: ["settings.storageIssueInUse", "Used by an active or restored workspace; kept."],
  unfinished_or_unknown_run: ["settings.storageIssueUnfinished", "The run is incomplete or its status could not be verified; kept."],
  explicit_output_directory: ["settings.storageIssueExplicit", "User-selected output directory; kept."],
  user_input: ["settings.storageIssueUserInput", "Contains a model opened or saved by the user; kept."],
  cleanup_limit_reached: ["settings.storageIssueCleanupLimit", "The cleanup time limit was reached. Remaining files are kept; refresh and clean again."],
  another_app_instance: ["settings.storageIssueOtherApp", "Used by another app instance; kept."],
  storage_instance_registration_failed: ["settings.storageIssueRegistration", "This app instance could not be registered; managed folders stay protected."],
};

function issueLabel(reason) {
  const entry = issueMessages[reason];
  return entry ? t(entry[0], {}, entry[1]) : String(reason ?? "");
}

function measuredTime(scannedAt) {
  const date = new Date(scannedAt);
  return Number.isFinite(date.getTime()) ? new Intl.DateTimeFormat(getLanguage(), { dateStyle: "medium", timeStyle: "short" }).format(date) : "";
}

function protectedInputDirectories() {
  try {
    // The embedded panel reads Main directly; standalone previews use the saved
    // workspace without importing Main's analysis/view tree.
    const snapshot = getAuxiliaryHost()?.getDocument() || JSON.parse(window.sessionStorage.getItem("idfAnalyzer.currentDocument") || "null");
    const path = typeof snapshot?.path === "string" ? snapshot.path.trim() : "";
    const separator = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
    if (separator < 0) return [];
    const rootLength = separator === 0 ? 1 : separator === 2 && /^[a-z]:[/\\]/i.test(path) ? 3 : separator;
    return [path.slice(0, rootLength)];
  } catch { return []; }
}

// The same controller survives Settings form rebuilds, including language
// changes and an in-flight cleanup. Only its Storage panel is replaced.
export function createSettingsStorage({ getAppAPI, getAutoClean = () => false, getSavedAutoClean = getAutoClean }) {
  let usage = null;
  let busy = false;
  let olderThanDays = 7;
  let confirming = false;
  let message = null;
  let failures = [];
  let focusAfterRender = "";

  const label = (key, fallback, params = {}) => escapeHTML(t(key, params, fallback));
  const ageLabel = () => olderThanDays
    ? t("settings.storageOlderThan", { days: olderThanDays }, "Completed more than {days} days ago")
    : t("settings.storageAllCompleted", {}, "All completed runs");

  function markup() {
    const value = usage || {};
    const warnings = [...new Map([...(value.warnings || []), ...failures, ...(value.lastCleanup?.failures || [])]
      .map((issue) => [`${issue.path}\n${issue.reason}`, issue])).values()];
    const incompleteWarnings = (value.warnings || []).filter((warning) => warning.path !== value.browserDataPath
      && (!Object.hasOwn(issueMessages, warning.reason) || ["scan_limit_reached", "unsafe_or_unreadable_path"].includes(warning.reason)));
    const partial = incompleteWarnings.length > 0;
    const size = (amount, incomplete = partial) => !usage ? "—" : incomplete ? t("settings.storageAtLeast", { size: bytes(amount) }, "At least {size}") : bytes(amount);
    const rootIncomplete = (root) => {
      const pathKey = (path) => {
        const normalized = String(path || "").replaceAll("\\", "/").replace(/\/+$/, "");
        return /^(?:[a-z]:\/|\/\/)/i.test(normalized) ? normalized.toLowerCase() : normalized;
      };
      const rootPath = pathKey(root.path);
      return incompleteWarnings.some((warning) => {
        const path = pathKey(warning.path);
        return path === rootPath || path.startsWith(`${rootPath}/`);
      });
    };
    const lastMeasured = measuredTime(value.scannedAt);
    return `
      <div class="settings-storage-heading">
        <h3>${label("settings.storageGenerated", "Simulation files generated by the app")}</h3>
        <button id="storageRefresh" type="button" ${busy ? "disabled" : ""}>${label("settings.storageRefresh", "Refresh usage")}</button>
      </div>
      <div class="settings-storage-summary" aria-busy="${busy}">
        <div><span>${label("settings.storageTotal", "Total generated files")}</span><strong id="storageTotalBytes">${escapeHTML(size(value.totalBytes))}</strong><small>${label("settings.storageRunCount", "Runs: {count}", { count: value.runCount || 0 })}</small></div>
        <div><span>${label("settings.storageReclaimable", "Available to clean")}</span><strong id="storageReclaimableBytes">${escapeHTML(size(value.reclaimableBytes))}</strong><small>${label("settings.storageRunCount", "Runs: {count}", { count: value.reclaimableRunCount || 0 })}</small></div>
        <div><span>${label("settings.storageProtected", "Retained / protected")}</span><strong id="storageProtectedBytes">${escapeHTML(size(value.protectedBytes))}</strong><small>${label("settings.storageRunCount", "Runs: {count}", { count: value.protectedRunCount || 0 })}</small></div>
      </div>
      ${lastMeasured ? `<p class="settings-storage-hint">${label("settings.storageMeasuredAt", "Last checked: {time}", { time: lastMeasured })}</p>` : ""}
      ${partial ? `<p class="settings-storage-hint">${label("settings.storagePartialScan", "Some locations could not be fully measured. Sizes and run counts show the known portion; review the retained locations below.")}</p>` : ""}
      <p class="settings-storage-hint">${label("settings.storageProtectionHelp", "Active runs, restored work, and folders not identified as disposable app runs are retained. Simulation output files are removed together with their completed run.")}</p>
      ${(value.roots || []).length ? `<details id="storageFolders" class="settings-storage-folders"><summary>${label("settings.storageLocations", "Generated file locations")}</summary><ul>${value.roots.map((root) => `<li><code>${escapeHTML(root.path)}</code><span>${escapeHTML(size(root.totalBytes, rootIncomplete(root)))} · ${label("settings.storageAvailable", "{size} available", { size: size(root.reclaimableBytes, rootIncomplete(root)) })}</span></li>`).join("")}</ul></details>` : ""}
      <div class="settings-storage-clean-controls">
        <label for="storageCleanupAge"><span>${label("settings.storageCleanupScope", "Completed runs to remove")}</span><select id="storageCleanupAge" ${busy ? "disabled" : ""}>
          ${[7, 30, 90, 0].map((days) => `<option value="${days}" ${olderThanDays === days ? "selected" : ""}>${escapeHTML(days ? t("settings.storageOlderThan", { days }, "Completed more than {days} days ago") : t("settings.storageAllCompleted", {}, "All completed runs"))}</option>`).join("")}
        </select></label>
        <button id="storagePrepareClean" type="button" ${busy || !usage || !value.reclaimableRunCount ? "disabled" : ""}>${label("settings.storageReviewCleanup", "Review cleanup")}</button>
      </div>
      ${confirming ? `<div id="storageConfirmation" class="settings-storage-confirmation" role="group" aria-labelledby="storageConfirmHeading">
        <h4 id="storageConfirmHeading">${label("settings.storageConfirmTitle", "Remove generated simulation files?")}</h4>
        <p>${label("settings.storageConfirmHelp", "Remove eligible completed runs matching “{scope}”. Currently measured space available to clean: {size}. Actual removals depend on the age filter, protected work, and any locations not fully measured. Removed simulation results must be run again to restore them.", { scope: ageLabel(), size: size(value.reclaimableBytes) })}</p>
        <div><button id="storageConfirmCancel" type="button" ${busy ? "disabled" : ""}>${label("common.cancel", "Cancel")}</button><button id="storageConfirmClean" type="button" class="danger" ${busy ? "disabled" : ""}>${label("settings.storageCleanNow", "Remove generated files")}</button></div>
      </div>` : ""}
      <div id="storageOperationStatus" class="settings-storage-status ${message?.error ? "error" : ""}" role="status" aria-live="polite" aria-atomic="true">${message ? label(message.key, message.fallback, message.params) : !usage ? label("settings.storageLoading", "Reading generated file usage…") : getAutoClean() !== getSavedAutoClean() ? label("status.unsavedPreview", "Unsaved changes (preview)") : getAutoClean() ? label("settings.storageAutomaticCleanup", "Automatic cleanup is on. Completed app runs are removed when they are no longer in use.") : label("settings.storageNoAutomaticCleanup", "Files are removed only when you choose to clean them.")}</div>
      ${value.lastCleanup ? `<div class="settings-storage-last-cleanup"><h4>${label("settings.storageLastCleanup", "Most recent cleanup")}</h4><span>${value.lastCleanup.automatic ? label("settings.storageAutomatic", "Automatic") : label("settings.storageManual", "Manual")} · ${escapeHTML(measuredTime(value.lastCleanup.completedAt))}</span><p>${label(value.lastCleanup.failureCount ? "settings.storageCleanedPartial" : "settings.storageCleaned", value.lastCleanup.failureCount ? "Removed {runs} runs and freed {size}. {skipped} runs retained; {failures} locations could not be removed." : "Removed {runs} runs and freed {size}. {skipped} runs retained.", { runs: value.lastCleanup.removedRunCount || 0, size: bytes(value.lastCleanup.freedBytes), skipped: value.lastCleanup.skippedRunCount || 0, failures: value.lastCleanup.failureCount || 0 })}</p></div>` : ""}
      ${warnings.length ? `<details id="storageWarnings" class="settings-storage-warnings"><summary>${label("settings.storageIssues", "{count} retained or unreadable locations", { count: warnings.length })}</summary><ul>${warnings.map((warning) => `<li><code>${escapeHTML(warning.path)}</code><span>${escapeHTML(issueLabel(warning.reason))}</span></li>`).join("")}</ul></details>` : ""}
      ${usage?.browserDataPath || value.browserDataUnavailable ? `<div class="settings-browser-storage"><h3>${label("settings.storageBrowserData", "Workspace and browser data")}</h3><strong>${value.browserDataUnavailable ? value.browserDataBytes > 0 ? label("settings.storageAtLeast", "At least {size}", { size: bytes(value.browserDataBytes) }) : label("settings.storageNotMeasured", "Not measured") : bytes(value.browserDataBytes)}</strong>${value.browserDataPath ? `<code>${escapeHTML(value.browserDataPath)}</code>` : ""}<p class="settings-storage-hint">${label("settings.storageBrowserDataHelp", "Includes workspace restoration, local settings, and WebView cache. Shown for visibility and kept by this cleanup.")}</p>${value.browserDataUnavailable ? `<p class="settings-storage-hint">${label("settings.storageBrowserUnavailable", "The browser data location could not be fully measured, for example because files are in use.")}</p>` : ""}</div>` : ""}`;
  }

  function render() {
    const panel = document.querySelector("#settingsStorage");
    if (!panel) return;
    const focusedID = panel.contains(document.activeElement) ? document.activeElement.id : "";
    const expanded = panel.querySelector("#storageFolders")?.open;
    panel.innerHTML = markup();
    if (expanded) panel.querySelector("#storageFolders").open = true;
    bind();
    const target = focusAfterRender || focusedID;
    if (target) document.getElementById(target)?.focus({ preventScroll: true });
    focusAfterRender = "";
  }

  async function request(method, endpoint, input) {
    for (let attempt = 0; attempt < 40; attempt += 1) {
      const api = getAppAPI();
      if (typeof api?.[method] === "function") return input === undefined ? api[method]() : api[method](input);
      if (!api) break;
      await new Promise((resolve) => setTimeout(resolve, 50));
    }
    const response = await fetch(endpoint, input === undefined ? undefined : {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input),
    });
    if (!response.ok) throw new Error(`Storage request failed: ${response.status}`);
    return response.json();
  }

  async function refresh() {
    if (busy) return;
    busy = true;
    confirming = false;
    message = { key: "settings.storageLoading", fallback: "Reading generated file usage…" };
    render();
    try {
      usage = await request("GetStorageUsage", "/api/storage");
      failures = [];
      message = null;
    } catch (error) {
      message = { key: "settings.storageFailed", fallback: "Unable to update storage: {message}", params: { message: error?.message || String(error) }, error: true };
    } finally { busy = false; render(); }
  }

  async function clean() {
    if (busy || !confirming) return;
    busy = true;
    message = { key: "settings.storageCleaning", fallback: "Removing eligible generated files…" };
    render();
    try {
      const result = await request("CleanStorage", "/api/storage/clean", { olderThanDays, protectedOutputDirectories: protectedInputDirectories() });
      usage = result.usage || usage;
      failures = result.failures || [];
      confirming = false;
      message = {
        key: failures.length ? "settings.storageCleanedPartial" : "settings.storageCleaned",
        fallback: failures.length ? "Removed {runs} runs and freed {size}. {skipped} runs retained; {failures} locations could not be removed." : "Removed {runs} runs and freed {size}. {skipped} runs retained.",
        params: { runs: result.removedRunCount || 0, size: bytes(result.freedBytes), skipped: result.skippedRunCount || 0, failures: failures.length },
        error: failures.length > 0,
      };
      focusAfterRender = "storageRefresh";
    } catch (error) {
      message = { key: "settings.storageCleanFailed", fallback: "Unable to clean generated files: {message}", params: { message: error?.message || String(error) }, error: true };
    } finally { busy = false; render(); }
  }

  function bind() {
    document.querySelector("#storageRefresh")?.addEventListener("click", refresh);
    document.querySelector("#storageCleanupAge")?.addEventListener("change", (event) => {
      olderThanDays = Number(event.target.value);
      confirming = false;
      render();
    });
    document.querySelector("#storagePrepareClean")?.addEventListener("click", () => {
      if (busy || !usage?.reclaimableRunCount) return;
      confirming = true;
      focusAfterRender = "storageConfirmCancel";
      render();
    });
    document.querySelector("#storageConfirmCancel")?.addEventListener("click", () => {
      if (busy) return;
      confirming = false;
      focusAfterRender = "storagePrepareClean";
      render();
    });
    document.querySelector("#storageConfirmClean")?.addEventListener("click", clean);
  }

  return { markup, bind, refresh, render };
}
