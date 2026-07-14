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
  elements.workspaceSelectionLabel.textContent = selectionLabel(selection);
  elements.workspaceSelectionLabel.title = selection.semanticPathHint || selectionLabel(selection);
  const targets = availableTargets(selection);
  renderTargets(elements.workspaceLinkTargets, targets, selection);
  renderTargets(elements.workspaceLinkMenuTargets, targets, selection);
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

function selectionLabel(selection) {
  if (!selection?.entityId) {
    return t("navigation.noSelection", {}, "No semantic selection");
  }
  const entity = semanticNavigationCache().entity(selection.entityId);
  const label = entity?.label || entity?.name || semanticPathTail(selection.semanticPathHint) || selection.entityId;
  const context = semanticContextLabel(selection.semanticPathHint);
  return context && context !== label ? `${label} / ${context}` : label;
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

function renderTargets(container, targets, selection) {
  if (!container) {
    return;
  }
  container.replaceChildren();
  for (const view of targets) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "workspace-link-bar__target";
    button.textContent = viewLabel(view);
    button.dataset.navigationView = view;
    const current = view === `input-${state.activeInputView}` || view === state.activeResultTab;
    if (current) {
      button.setAttribute("aria-current", "page");
    }
    button.setAttribute("aria-label", t("semantic.openInView", { view: viewLabel(view) }, `Open in ${viewLabel(view)}`));
    button.addEventListener("click", async () => {
      if (view === "source") {
        await callbacks.revealSource?.({
          originView: selection.originView || state.activeResultTab,
          action: "reveal_source",
          preserveFilters: true,
        });
      } else {
        await callbacks.openView?.(view, {
          originView: selection.originView || state.activeResultTab,
          action: "open",
          preserveFilters: true,
        });
      }
      renderNavigationLinkBar();
    });
    container.append(button);
  }
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
  const parts = String(path || "").split(/[/.]/).filter(Boolean);
  return parts.at(-1) || "";
}

function semanticContextLabel(path) {
  const parts = String(path || "").split(/[/.]/).filter(Boolean);
  return parts.length > 1 ? parts.slice(-2).join(" / ") : "";
}
