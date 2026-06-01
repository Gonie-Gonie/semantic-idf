import { state } from "./state.js";

export function initializeKeyboardShortcuts(actions) {
  window.addEventListener("keydown", (event) => handleShortcutKeydown(event, actions));
}

function handleShortcutKeydown(event, actions) {
  const shortcutID = shortcutIDForEvent(event);
  if (!shortcutID) {
    return;
  }

  const editable = isEditableTarget(event.target);
  if (editable && !editableSafeShortcut(shortcutID, event)) {
    return;
  }

  const action = shortcutAction(shortcutID, actions);
  if (!action) {
    return;
  }

  event.preventDefault();
  action();
}

function shortcutIDForEvent(event) {
  const pressed = acceleratorForEvent(event);
  if (!pressed) {
    return "";
  }
  const shortcuts = state.keyboardShortcuts || {};
  return Object.entries(shortcuts).find(([, accelerator]) => acceleratorMatches(accelerator, pressed))?.[0] || "";
}

function shortcutAction(id, actions) {
  const map = {
    save: actions.save,
    open: actions.open,
    undoView: actions.undoView,
    redoView: actions.redoView,
    jumpDefinition: actions.jumpDefinition,
    jumpReferences: actions.jumpReferences,
    inputText: () => actions.switchInputView?.("text"),
    inputJson: () => actions.switchInputView?.("json"),
    inputTable: () => actions.switchInputView?.("table"),
    tabSummary: () => actions.switchResultTab?.("summary"),
    tabProfile: () => actions.switchResultTab?.("profile"),
    tabHVAC: () => actions.switchResultTab?.("hvac"),
    tabDiagnose: () => actions.switchResultTab?.("diagnose"),
    tabGeometry: () => actions.switchResultTab?.("geometry"),
  };
  return map[id] || null;
}

function editableSafeShortcut(id, event) {
  if (id === "undoView" || id === "redoView") {
    return false;
  }
  if (id === "save" || id === "open" || id === "jumpDefinition" || id === "jumpReferences") {
    return true;
  }
  return Boolean(event.ctrlKey || event.metaKey || event.altKey || /^f\d{1,2}$/i.test(event.key || ""));
}

function acceleratorForEvent(event) {
  const key = normalizedKey(event.key);
  if (!key) {
    return "";
  }
  const parts = [];
  if (event.ctrlKey || event.metaKey) parts.push("Ctrl");
  if (event.altKey) parts.push("Alt");
  if (event.shiftKey) parts.push("Shift");
  parts.push(key);
  return parts.join("+");
}

function normalizeAccelerator(value) {
  const tokens = String(value || "")
    .split("+")
    .map((token) => token.trim())
    .filter(Boolean);
  if (!tokens.length) {
    return "";
  }
  const key = normalizedKey(tokens[tokens.length - 1]);
  if (!key) {
    return "";
  }
  const modifiers = new Set(tokens.slice(0, -1).map((token) => token.toLowerCase()));
  const parts = [];
  if (modifiers.has("ctrl") || modifiers.has("control") || modifiers.has("cmd") || modifiers.has("meta")) parts.push("Ctrl");
  if (modifiers.has("alt") || modifiers.has("option")) parts.push("Alt");
  if (modifiers.has("shift")) parts.push("Shift");
  parts.push(key);
  return parts.join("+");
}

function acceleratorMatches(value, pressed) {
  return String(value || "")
    .split(",")
    .map((accelerator) => normalizeAccelerator(accelerator))
    .filter(Boolean)
    .includes(pressed);
}

function normalizedKey(value) {
  const key = String(value || "").trim();
  if (!key || key === "Control" || key === "Alt" || key === "Shift" || key === "Meta") {
    return "";
  }
  if (/^f\d{1,2}$/i.test(key)) {
    return key.toUpperCase();
  }
  if (key === " ") {
    return "Space";
  }
  if (key.length === 1) {
    return key.toUpperCase();
  }
  return key[0].toUpperCase() + key.slice(1);
}

function isEditableTarget(target) {
  return Boolean(target?.closest?.("input, textarea, select, [contenteditable='true']"));
}
