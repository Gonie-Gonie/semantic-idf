import { t } from "./i18n.js";

let commandProvider = () => [];
let palette = null;

export function initializeCommandPalette(provider) {
  commandProvider = typeof provider === "function" ? provider : () => provider || [];
  ensurePalette();
}

export function openCommandPalette() {
  return openPalette({
    title: t("navigation.commandPalette", {}, "Command palette"),
    placeholder: t("navigation.commandSearch", {}, "Type a command"),
    items: commandProvider(),
  });
}

export function openAvailableViewsPalette(items = []) {
  return openPalette({
    title: t("semantic.chooseViewTarget", {}, "Choose a panel target"),
    placeholder: t("navigation.viewSearch", {}, "Filter available views"),
    items,
  });
}

export function closeCommandPalette() {
  if (palette?.dialog?.open) {
    palette.dialog.close();
    return true;
  }
  return false;
}

function openPalette({ title, placeholder, items }) {
  const ui = ensurePalette();
  const available = (items || []).filter((item) => item && item.id && item.label && typeof item.run === "function");
  if (!available.length) {
    return false;
  }
  ui.title.textContent = title;
  ui.search.placeholder = placeholder;
  ui.search.value = "";
  ui.items = available;
  renderItems(ui, available);
  if (!ui.dialog.open) {
    ui.dialog.showModal();
  }
  window.requestAnimationFrame(() => ui.search.focus());
  return true;
}

function ensurePalette() {
  if (palette) {
    return palette;
  }
  const dialog = document.createElement("dialog");
  dialog.className = "navigation-command-palette";
  dialog.setAttribute("aria-labelledby", "navigationCommandPaletteTitle");
  const shell = document.createElement("div");
  shell.className = "navigation-command-palette__shell";
  const title = document.createElement("h2");
  title.id = "navigationCommandPaletteTitle";
  title.className = "navigation-command-palette__title";
  const search = document.createElement("input");
  search.type = "search";
  search.autocomplete = "off";
  search.className = "navigation-command-palette__search";
  search.setAttribute("aria-controls", "navigationCommandPaletteList");
  const list = document.createElement("div");
  list.id = "navigationCommandPaletteList";
  list.className = "navigation-command-palette__list";
  list.setAttribute("role", "listbox");
  shell.append(title, search, list);
  dialog.append(shell);
  document.body.append(dialog);

  palette = { dialog, title, search, list, items: [] };
  search.addEventListener("input", () => {
    const query = search.value.trim().toLocaleLowerCase();
    renderItems(palette, palette.items.filter((item) => searchableItem(item).includes(query)));
  });
  search.addEventListener("keydown", (event) => handlePaletteKeydown(event, palette));
  dialog.addEventListener("click", (event) => {
    if (event.target === dialog) {
      dialog.close();
    }
  });
  return palette;
}

function renderItems(ui, items) {
  ui.list.replaceChildren();
  items.forEach((item, index) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "navigation-command-palette__item";
    button.dataset.commandId = item.id;
    button.setAttribute("role", "option");
    button.setAttribute("aria-selected", index === 0 ? "true" : "false");
    const label = document.createElement("span");
    label.textContent = item.label;
    const meta = document.createElement("kbd");
    meta.textContent = item.shortcut || item.meta || "";
    meta.hidden = !meta.textContent;
    button.append(label, meta);
    button.addEventListener("click", async () => {
      ui.dialog.close();
      await item.run();
    });
    ui.list.append(button);
  });
}

function handlePaletteKeydown(event, ui) {
  const buttons = [...ui.list.querySelectorAll("button")];
  if (!buttons.length || !["ArrowDown", "ArrowUp", "Enter"].includes(event.key)) {
    return;
  }
  event.preventDefault();
  const current = buttons.findIndex((button) => button.getAttribute("aria-selected") === "true");
  if (event.key === "Enter") {
    buttons[Math.max(0, current)]?.click();
    return;
  }
  const direction = event.key === "ArrowDown" ? 1 : -1;
  const next = (Math.max(0, current) + direction + buttons.length) % buttons.length;
  buttons.forEach((button, index) => button.setAttribute("aria-selected", index === next ? "true" : "false"));
  buttons[next].scrollIntoView({ block: "nearest" });
}

function searchableItem(item) {
  return `${item.label || ""} ${item.meta || ""} ${item.shortcut || ""}`.toLocaleLowerCase();
}
