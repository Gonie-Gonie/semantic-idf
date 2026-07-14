import { elements, state } from "./state.js";
import { t } from "./i18n.js";
import { bundledAppInfo } from "./app-info.js";
import { getSemanticNavigationCache } from "./semantic-navigation-cache.js";

const targetOrder = ["input-semantic", "hvac", "profile", "geometry", "output", "simulation", "diagnose", "summary", "source"];
let callbacks = {};
let initialized = false;

export function initializeNavigationLinkBar(options = {}) {
  callbacks = { ...callbacks, ...options };
  if (initialized || !elements.workspaceLinkBar) {
    renderNavigationLinkBar();
    return;
  }
  initialized = true;
  elements.semanticLinkedToggle?.addEventListener("click", () => {
    state.semanticLinkMode = !state.semanticLinkMode;
    if (!state.semanticLinkMode) {
      state.globalHover = { entityId: "", occurrenceId: "", originView: "" };
      window.dispatchEvent(new CustomEvent("idfAnalyzer:semanticHoverChanged", {
        detail: { hover: state.globalHover, options: { action: "mode_change" } },
      }));
    }
    dispatchModeChange();
  });
  elements.semanticFollowToggle?.addEventListener("click", () => {
    state.semanticFollowSelection = !state.semanticFollowSelection;
    dispatchModeChange();
  });
  elements.workspaceBackButton?.addEventListener("click", () => callbacks.back?.());
  elements.workspaceForwardButton?.addEventListener("click", () => callbacks.forward?.());
  window.addEventListener("idfAnalyzer:semanticSelectionChanged", renderNavigationLinkBar);
  window.addEventListener("idfAnalyzer:semanticSelectionRemapped", renderNavigationLinkBar);
  window.addEventListener("idfAnalyzer:analysisComplete", renderNavigationLinkBar);
  window.addEventListener("idfAnalyzer:navigationModeChanged", renderNavigationLinkBar);
  window.addEventListener("idfAnalyzer:inputViewChanged", renderNavigationLinkBar);
  renderNavigationLinkBar();
}

export function renderNavigationLinkBar() {
  if (!elements.workspaceLinkBar) {
    return;
  }
  if (state.semanticProjection?.navigation) {
    // Analysis/cache-restore lifecycle events prewarm the one-time Map build
    // before the first user-driven selection needs it.
    semanticNavigationCache();
  }
  setToggle(elements.semanticLinkedToggle, state.semanticLinkMode);
  setToggle(elements.semanticFollowToggle, state.semanticFollowSelection);
  const selection = state.globalSelection || {};
  const label = formatNavigationSelectionLabel(selection);
  updateSelectionLabel(elements.workspaceSelectionLabel, label, selection);
  const targets = availableTargets(selection);
  renderTargets(elements.workspaceLinkTargets, targets);
  renderTargets(elements.workspaceLinkMenuTargets, targets);
  if (elements.workspaceBackButton) {
    elements.workspaceBackButton.disabled = !(state.navigationUndoStack?.length);
  }
  if (elements.workspaceForwardButton) {
    elements.workspaceForwardButton.disabled = !(state.navigationRedoStack?.length);
  }
}

function dispatchModeChange() {
  renderNavigationLinkBar();
  window.dispatchEvent(new CustomEvent("idfAnalyzer:navigationModeChanged", {
    detail: {
      linked: state.semanticLinkMode,
      follow: state.semanticFollowSelection,
    },
  }));
}

function setToggle(button, active) {
  if (!button) {
    return;
  }
  button.setAttribute("aria-pressed", active ? "true" : "false");
  button.classList.toggle("active", active);
}

export function formatNavigationSelectionLabel(selection) {
  if (!selection?.entityId) {
    return t("navigation.noSelection", {}, "No semantic selection");
  }
  const cache = semanticNavigationCache();
  const entity = cache.entity(selection.entityId);
  const occurrence = cache.occurrence(selection.occurrenceId);
  const entityLabel = readableLabel(
    entity?.label || entity?.name || semanticPathTail(selection.semanticPathHint) || selection.entityId,
  );
  const targetLabel = originTargetLabel(selection, entity, occurrence);
  const contextParts = semanticContextParts(selection.semanticPathHint);
  const parts = targetLabel && !labelsEqual(targetLabel, entityLabel)
    ? [entityLabel, targetLabel]
    : [entityLabel, ...contextParts];
  return uniqueLabelParts(parts).join(" / ") || entityLabel;
}

function updateSelectionLabel(element, label, selection) {
  if (!element) {
    return;
  }
  element.setAttribute("role", "status");
  element.setAttribute("aria-live", "polite");
  element.setAttribute("aria-atomic", "true");
  if (element.textContent !== label) {
    // Avoid announcing the same selection again on mode/tab/history renders.
    element.textContent = label;
  }
  const titleParts = uniqueLabelParts([
    label,
    meaningfulTargetID(selection?.originTargetId),
    selection?.semanticPathHint,
  ]);
  element.title = titleParts.join(" — ");
}

function originTargetLabel(selection, entity, occurrence) {
  const targetID = String(selection?.originTargetId || "").trim();
  if (!targetID) {
    return "";
  }
  const elementLabel = labelForOriginTargetElement(selection.originView, targetID);
  if (elementLabel) {
    return elementLabel;
  }
  const originView = normalizeView(selection.originView);
  const viewTarget = [
    ...(occurrence?.viewTargets || []),
    ...(entity?.viewTargets || []),
  ].find((target) => (
    String(target?.targetId || "") === targetID &&
    (!originView || normalizeView(target?.view) === originView)
  ));
  if (viewTarget?.label) {
    return readableLabel(viewTarget.label);
  }
  // Semantic/source lines store a preferred destination in originTargetId;
  // it is not the row the user clicked and should not replace path context.
  return originView && !originView.startsWith("input-") ? meaningfulTargetID(targetID) : "";
}

function labelForOriginTargetElement(originView, targetID) {
  const root = originViewRoot(originView);
  if (!root) {
    return "";
  }
  const target = [...root.querySelectorAll("[data-panel-target-id]")]
    .find((candidate) => candidate.dataset.panelTargetId === targetID);
  if (!target) {
    return "";
  }
  const explicit = readableLabel(
    target.dataset.navigationLabel || target.getAttribute("aria-label") ||
    target.querySelector(".summary-name strong")?.textContent ||
    target.querySelector("[data-navigation-label]")?.dataset.navigationLabel ||
    target.querySelector("[data-navigation-label]")?.textContent ||
    target.querySelector(".diagnostic-title, .diagnostic-code, .output-request-name")?.textContent ||
    target.getAttribute("title") || target.querySelector("strong")?.textContent,
  );
  if (explicit) {
    return explicit;
  }
  const text = readableLabel(target.textContent);
  return text.length <= 80 ? text : "";
}

function originViewRoot(view) {
  const normalized = normalizeView(view);
  if (!normalized || typeof document === "undefined") {
    return null;
  }
  if (normalized.startsWith("input-")) {
    return document.getElementById(`${normalized.slice(6)}InputView`);
  }
  return document.getElementById(`${normalized}Pane`);
}

function availableTargets(selection) {
  if (!selection?.entityId) {
    return [];
  }
  const cache = semanticNavigationCache();
  const occurrenceIDs = cache.occurrenceIdsByEntityId.get(selection.entityId) || [];
  const views = new Set(["input-semantic"]);
  for (const occurrenceID of occurrenceIDs) {
    const occurrence = cache.occurrence(occurrenceID);
    for (const target of occurrence?.viewTargets || []) {
      if (target?.view) {
        views.add(String(target.view));
      }
    }
  }
  if (selection.sourceAnchor) {
    views.add("source");
  }
  return [...views].sort((left, right) => targetRank(left) - targetRank(right));
}

function semanticNavigationCache() {
  return getSemanticNavigationCache(state.semanticProjection, {
    textHash: state.reportAnalysisKey || state.lastAnalyzedKey || state.analysisKey || "",
    analyzerVersion: bundledAppInfo.version,
    schemaVersion: state.semanticProjection?.schema || "",
  });
}

function renderTargets(container, targets) {
  if (!container) {
    return;
  }
  const focusedView = container.contains(document.activeElement)
    ? document.activeElement?.dataset?.navigationView || ""
    : "";
  const existing = new Map(
    [...container.querySelectorAll(":scope > [data-navigation-view]")]
      .map((button) => [button.dataset.navigationView, button]),
  );
  const retained = new Set();
  for (const view of targets) {
    const button = existing.get(view) || createTargetButton(view);
    retained.add(button);
    button.type = "button";
    button.className = "workspace-link-bar__target";
    button.textContent = viewLabel(view);
    button.dataset.navigationView = view;
    const current = view === `input-${state.activeInputView}` || view === state.activeResultTab;
    if (current) {
      button.setAttribute("aria-current", "page");
    } else {
      button.removeAttribute("aria-current");
    }
    button.setAttribute("aria-label", t("semantic.openInView", { view: viewLabel(view) }, `Open in ${viewLabel(view)}`));
    container.append(button);
  }
  for (const button of existing.values()) {
    if (!retained.has(button)) {
      button.remove();
    }
  }
  if (focusedView) {
    const focusedButton = existing.get(focusedView);
    if (focusedButton?.isConnected && document.activeElement !== focusedButton) {
      focusedButton.focus({ preventScroll: true });
    }
  }
}

function createTargetButton(view) {
  const button = document.createElement("button");
  button.dataset.navigationView = view;
  button.addEventListener("click", async () => {
    const currentView = button.dataset.navigationView;
    const selection = state.globalSelection || {};
    if (currentView === "source") {
      await callbacks.revealSource?.({
        originView: selection.originView || state.activeResultTab,
        action: "reveal_source",
        preserveFilters: true,
      });
    } else {
      await callbacks.openView?.(currentView, {
        originView: selection.originView || state.activeResultTab,
        action: "open",
        preserveFilters: true,
      });
    }
    renderNavigationLinkBar();
  });
  return button;
}

function viewLabel(view) {
  if (view === "input-semantic") {
    return t("input.semantic", {}, "Semantic");
  }
  if (view === "source") {
    return t("semantic.source", {}, "Source");
  }
  return t(`tab.${view}`, {}, view[0].toUpperCase() + view.slice(1));
}

function targetRank(view) {
  const index = targetOrder.indexOf(view);
  return index < 0 ? targetOrder.length : index;
}

function semanticPathTail(path) {
  const parts = semanticPathParts(path);
  return parts.at(-1) || "";
}

function semanticContextParts(path) {
  const parts = semanticPathParts(path);
  return parts.length > 1 ? parts.slice(-2) : [];
}

function semanticPathParts(path) {
  return String(path || "").split(/[/.]/).map(readableLabel).filter(Boolean);
}

function uniqueLabelParts(parts) {
  const seen = new Set();
  const result = [];
  for (const rawPart of parts) {
    const part = readableLabel(rawPart);
    const key = normalizeLabelKey(part);
    if (!part || !key || seen.has(key)) {
      continue;
    }
    seen.add(key);
    result.push(part);
  }
  return result;
}

function labelsEqual(left, right) {
  return Boolean(normalizeLabelKey(left) && normalizeLabelKey(left) === normalizeLabelKey(right));
}

function normalizeLabelKey(value) {
  return readableLabel(value).normalize("NFKC").toLowerCase()
    .replace(/[_\-\s/.:|]+/g, " ")
    .replace(/[^\p{L}\p{N} ]/gu, "")
    .trim();
}

function readableLabel(value) {
  const text = String(value || "").trim().replace(/\s+/g, " ");
  if (!text) {
    return "";
  }
  try {
    return decodeURIComponent(text);
  } catch {
    return text;
  }
}

function meaningfulTargetID(value) {
  const targetID = readableLabel(value);
  if (!targetID || targetID.length > 96) {
    return "";
  }
  const tail = targetID.split(/[|:/]/).map(readableLabel).filter(Boolean).at(-1) || "";
  if (!tail || /^\d+$/.test(tail) || /^[a-f\d]{12,}$/i.test(tail) || /^[a-f\d-]{32,}$/i.test(tail)) {
    return "";
  }
  return tail;
}

function normalizeView(value) {
  return String(value || "").trim().toLowerCase();
}
