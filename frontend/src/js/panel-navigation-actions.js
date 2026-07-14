import { t } from "./i18n.js";
import { RESULT_PANEL_NAVIGATION_VIEW_IDS } from "./panel-navigation-adapters.js";
import { getPanelNavigationAdapter } from "./panel-navigation-registry.js";
import {
  clearSemanticHover,
  hoverSemanticEntity,
  openSelectionInView,
  revealSelectionSource,
  selectSemanticEntity,
  selectionTargetsForView,
} from "./selection-controller.js";
import { elements, state } from "./state.js";
import { recordViewHistory } from "./view-history.js";

const actionOrder = Object.freeze([
  ["focus", "panelNavigation.openFocus", "Open / Focus"],
  ["semantic", "panelNavigation.revealSemantic", "Reveal in Semantic"],
  ["source", "panelNavigation.revealSource", "Reveal source"],
  ["related", "panelNavigation.relatedViews", "Related views"],
  ["definition", "panelNavigation.definition", "Definition"],
  ["references", "panelNavigation.references", "References"],
  ["pin", "panelNavigation.pin", "Pin"],
]);

let initialized = false;
let menu = null;
let menuContext = null;
let callbacks = {};

export function initializePanelNavigationActions(options = {}) {
  callbacks = { ...callbacks, ...options };
  if (initialized || !elements.analysisPanel) {
    return;
  }
  initialized = true;
  menu = createActionMenu();
  elements.analysisPanel.addEventListener("contextmenu", handleContextMenu);
  elements.analysisPanel.addEventListener("click", handleActionTrigger, { capture: true });
  elements.analysisPanel.addEventListener("keydown", handleMenuKey);
  elements.analysisPanel.addEventListener("pointerover", handlePanelPointerOver);
  elements.analysisPanel.addEventListener("pointerout", handlePanelPointerOut);
  window.addEventListener("blur", () => closePanelNavigationMenu());
  document.addEventListener("pointerdown", (event) => {
    if (menuContext && !menu?.contains(event.target)) {
      closePanelNavigationMenu();
    }
  });
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && menuContext) {
      event.preventDefault();
      closePanelNavigationMenu({ restoreFocus: true });
    }
  });
}

export function openPanelNavigationMenu(element, position = {}) {
  const context = panelContextForElement(element);
  if (!context || !menu) {
    return false;
  }
  menuContext = { ...context, trigger: element };
  renderActionMenu(menuContext);
  menu.hidden = false;
  menu.style.left = "0px";
  menu.style.top = "0px";
  const rect = menu.getBoundingClientRect();
  const x = Math.max(8, Math.min(Number(position.x) || 8, window.innerWidth - rect.width - 8));
  const y = Math.max(8, Math.min(Number(position.y) || 8, window.innerHeight - rect.height - 8));
  menu.style.left = `${x}px`;
  menu.style.top = `${y}px`;
  menu.querySelector("[role='menuitem']:not(:disabled)")?.focus();
  return true;
}

export function closePanelNavigationMenu(options = {}) {
  const trigger = menuContext?.trigger;
  menuContext = null;
  if (menu) {
    menu.hidden = true;
    menu.replaceChildren();
  }
  if (options.restoreFocus) {
    trigger?.focus?.({ preventScroll: true });
  }
}

function handleContextMenu(event) {
  if (openPanelNavigationMenu(event.target, { x: event.clientX, y: event.clientY })) {
    event.preventDefault();
  }
}

function handleActionTrigger(event) {
  const trigger = event.target.closest?.("[data-panel-action-menu]");
  if (!trigger) {
    return;
  }
  const rect = trigger.getBoundingClientRect();
  if (openPanelNavigationMenu(trigger, { x: rect.right, y: rect.bottom })) {
    event.preventDefault();
    event.stopPropagation();
  }
}

function handleMenuKey(event) {
  if (event.key !== "ContextMenu" && !(event.shiftKey && event.key === "F10")) {
    return;
  }
  const context = panelContextForElement(event.target);
  if (!context) {
    return;
  }
  event.preventDefault();
  const rect = event.target.getBoundingClientRect();
  openPanelNavigationMenu(event.target, { x: rect.left + 12, y: rect.top + 12 });
}

function handlePanelPointerOver(event) {
  const context = panelContextForElement(event.target);
  if (!context || context.item.contains(event.relatedTarget)) {
    return;
  }
  hoverSemanticEntity(context.selection, {
    originView: context.view,
    action: "hover",
    recordHistory: false,
    follow: false,
  });
}

function handlePanelPointerOut(event) {
  const context = panelContextForElement(event.target);
  if (!context || context.item.contains(event.relatedTarget)) {
    return;
  }
  clearSemanticHover({ originView: context.view, action: "hover" });
}

function panelContextForElement(element) {
  const view = String(state.activeResultTab || "").toLowerCase();
  if (!RESULT_PANEL_NAVIGATION_VIEW_IDS.includes(view)) {
    return null;
  }
  const adapter = getPanelNavigationAdapter(view);
  const selection = adapter?.selectFromElement?.(element) || null;
  if (!selection?.entityId) {
    return null;
  }
  const item = element.closest?.("[data-panel-target-id], [data-entity-id], [data-source-object-id], [data-source-object-index]") || element;
  return { view, adapter, selection, item };
}

function createActionMenu() {
  const element = document.createElement("div");
  element.className = "panel-navigation-menu";
  element.hidden = true;
  element.setAttribute("role", "menu");
  element.setAttribute("aria-label", t("panelNavigation.actions", {}, "Navigation actions"));
  element.addEventListener("click", handleMenuAction);
  document.body.append(element);
  return element;
}

function renderActionMenu(context) {
  menu.replaceChildren();
  const related = relatedPanelTargets(context.selection, context.view);
  for (const [action, key, fallback] of actionOrder) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "panel-navigation-menu__item";
    button.dataset.panelNavigationAction = action;
    button.setAttribute("role", "menuitem");
    button.textContent = t(key, {}, fallback);
    button.disabled = !actionAvailable(action, context, related);
    menu.append(button);
    if (action === "related") {
      const submenu = document.createElement("div");
      submenu.className = "panel-navigation-menu__related";
      submenu.hidden = true;
      for (const entry of related) {
        const relatedButton = document.createElement("button");
        relatedButton.type = "button";
        relatedButton.dataset.relatedView = entry.view;
        relatedButton.dataset.relatedTargetId = entry.target.targetId;
        relatedButton.textContent = `${panelViewLabel(entry.view)} · ${entry.target.label || entry.target.targetId}`;
        submenu.append(relatedButton);
      }
      menu.append(submenu);
    }
  }
}

function actionAvailable(action, context, related) {
  if (action === "focus" || action === "semantic" || action === "pin") {
    return true;
  }
  if (action === "source" || action === "definition" || action === "references") {
    return Boolean(context.selection.sourceAnchor);
  }
  return action === "related" && related.length > 0;
}

async function handleMenuAction(event) {
  const relatedButton = event.target.closest?.("[data-related-view]");
  if (relatedButton && menuContext) {
    const context = menuContext;
    const view = relatedButton.dataset.relatedView;
    const targetId = relatedButton.dataset.relatedTargetId;
    closePanelNavigationMenu();
    await runAtomicPanelAction(context, async () => {
      await openSelectionInView(view, {
        originView: context.view,
        action: "open",
        targetId,
        recordHistory: false,
        follow: false,
      });
    });
    return;
  }
  const button = event.target.closest?.("[data-panel-navigation-action]");
  if (!button || !menuContext || button.disabled) {
    return;
  }
  const action = button.dataset.panelNavigationAction;
  if (action === "related") {
    const submenu = button.nextElementSibling;
    if (submenu) {
      submenu.hidden = !submenu.hidden;
      if (!submenu.hidden) submenu.querySelector("button")?.focus();
    }
    return;
  }
  const context = menuContext;
  closePanelNavigationMenu();
  await executePanelAction(action, context);
}

async function executePanelAction(action, context) {
  if (action === "pin") {
    togglePinnedSelection(context.selection);
    return;
  }
  await runAtomicPanelAction(context, async () => {
    if (action === "focus") {
      await openSelectionInView(context.view, panelOpenOptions(context));
    } else if (action === "semantic") {
      await openSelectionInView("input-semantic", panelOpenOptions(context));
    } else if (action === "source") {
      await revealSelectionSource(panelRevealOptions(context));
    } else if (action === "definition" || action === "references") {
      await revealSelectionSource(panelRevealOptions(context));
      const callback = action === "definition" ? callbacks.jumpDefinition : callbacks.jumpReferences;
      await callback?.({ recordHistory: false });
    }
  });
}

async function runAtomicPanelAction(context, action) {
  recordViewHistory();
  await selectSemanticEntity(context.selection, {
    originView: context.view,
    action: "select",
    recordHistory: false,
    follow: false,
    chooseOccurrence: context.selection.chooseOccurrence === true,
  });
  await action();
}

function panelOpenOptions(context) {
  return {
    originView: context.view,
    action: "open",
    recordHistory: false,
    follow: false,
    preserveFilters: true,
    targetId: context.selection.originTargetId || "",
  };
}

function panelRevealOptions(context) {
  return {
    originView: context.view,
    action: "reveal_source",
    recordHistory: false,
    follow: false,
    preserveFilters: true,
  };
}

function relatedPanelTargets(selection, currentView) {
  const out = [];
  for (const view of RESULT_PANEL_NAVIGATION_VIEW_IDS) {
    if (view === currentView) continue;
    for (const target of selectionTargetsForView(view, selection)) {
      out.push({ view, target });
    }
  }
  return out;
}

function panelViewLabel(view) {
  return String(view || "").slice(0, 1).toUpperCase() + String(view || "").slice(1);
}

function togglePinnedSelection(selection) {
  if (!(state.semanticPinnedEntityIds instanceof Set)) {
    state.semanticPinnedEntityIds = new Set();
  }
  if (state.semanticPinnedEntityIds.has(selection.entityId)) {
    state.semanticPinnedEntityIds.delete(selection.entityId);
  } else {
    state.semanticPinnedEntityIds.add(selection.entityId);
  }
  document.querySelectorAll("[data-entity-id]").forEach((element) => {
    element.toggleAttribute("data-semantic-pinned", state.semanticPinnedEntityIds.has(element.dataset.entityId));
  });
  window.dispatchEvent(new CustomEvent("idfAnalyzer:semanticPinChanged", {
    detail: { entityId: selection.entityId, pinned: state.semanticPinnedEntityIds.has(selection.entityId) },
  }));
}
