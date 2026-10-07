import { t, translatePage } from "./i18n.js";

const pageNames = ["settings", "guide", "tools"];
const pageLabels = {
  settings: () => t("action.settings"),
  guide: () => t("action.guide"),
  tools: () => t("action.tools"),
};
const frames = new Map();
let dialog = null;
let activePage = "";
let returnFocus = null;
let getDocument = () => null;
let applyDocument = () => false;

export function initializeAuxiliaryPanel(handlers = {}) {
  getDocument = handlers.getDocument || getDocument;
  applyDocument = handlers.applyDocument || applyDocument;
  window.idfAnalyzerAuxiliaryHost = {
    open: openAuxiliaryPage,
    close: closeAuxiliaryPanel,
    contains: (child) => [...frames.values()].some((frame) => frame.contentWindow === child),
    getDocument: () => getDocument(),
    applyDocument: (documentState, options = {}) => {
      const current = getDocument();
      const expected = options.expected;
      if (!expected || !current || ["text", "path", "filename"].some((key) => (expected[key] || "") !== (current[key] || ""))) return false;
      return applyDocument(documentState, options);
    },
  };
}

export function openAuxiliaryPage(path) {
  const url = new URL(path, window.location.href);
  const page = pageNames.find((name) => url.pathname.endsWith(`/${name}.html`));
  if (!page || url.origin !== window.location.origin) return false;
  ensureDialog();
  if (!dialog.open) {
    returnFocus = document.activeElement;
    dialog.showModal();
  }
  activePage = page;
  let frame = frames.get(page);
  if (!frame) {
    frame = document.createElement("iframe");
    frame.dataset.auxiliaryPage = page;
    frame.className = "auxiliary-panel-frame";
    frame.addEventListener("load", () => {
      frame.dataset.ready = "true";
      if (dialog.open && activePage === page) notifyShown(frame);
    });
    frames.set(page, frame);
    frame.src = url.href;
    dialog.querySelector(".auxiliary-panel-body").appendChild(frame);
  } else if (url.hash && frame.contentWindow.location.hash !== url.hash) {
    frame.contentWindow.location.hash = url.hash;
  }
  for (const [name, item] of frames) item.hidden = name !== page;
  updateLabels();
  if (frame.dataset.ready) notifyShown(frame);
  return true;
}

export function closeAuxiliaryPanel() {
  if (dialog?.open) dialog.close();
}

function notifyShown(frame) {
  const child = frame.contentWindow;
  child.dispatchEvent(new child.CustomEvent("idfAnalyzer:auxiliaryShown", { detail: { document: getDocument() } }));
}

function ensureDialog() {
  if (dialog) return;
  if (!window.idfAnalyzerAuxiliaryHost) initializeAuxiliaryPanel();
  dialog = document.createElement("dialog");
  dialog.id = "auxiliaryPanel";
  dialog.className = "auxiliary-panel";
  dialog.setAttribute("aria-labelledby", "auxiliaryPanelTitle");
  dialog.innerHTML = `
    <header class="auxiliary-panel-header">
      <h2 id="auxiliaryPanelTitle"></h2>
      <nav class="auxiliary-panel-nav">${pageNames.map((name) => `<button type="button" data-auxiliary-open="${name}"></button>`).join("")}</nav>
      <button id="auxiliaryPanelClose" type="button" class="auxiliary-panel-close" data-i18n-aria-label="action.backToApp" data-i18n-title="action.backToApp">×</button>
    </header>
    <div class="auxiliary-panel-body"></div>`;
  dialog.querySelector("#auxiliaryPanelClose").addEventListener("click", closeAuxiliaryPanel);
  // Main's global shortcuts must not change the workspace behind this panel.
  dialog.addEventListener("keydown", (event) => event.stopPropagation());
  dialog.addEventListener("auxclick", (event) => event.stopPropagation());
  dialog.querySelectorAll("[data-auxiliary-open]").forEach((button) => {
    button.addEventListener("click", () => openAuxiliaryPage(`./${button.dataset.auxiliaryOpen}.html`));
  });
  dialog.addEventListener("click", (event) => {
    if (event.target !== dialog) return;
    const bounds = dialog.getBoundingClientRect();
    if (event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom) closeAuxiliaryPanel();
  });
  dialog.addEventListener("close", () => {
    if (returnFocus?.isConnected) returnFocus.focus({ preventScroll: true });
  });
  window.addEventListener("idfAnalyzer:languageChanged", updateLabels);
  document.body.appendChild(dialog);
}

function updateLabels() {
  if (!dialog) return;
  translatePage(dialog);
  dialog.querySelector("#auxiliaryPanelTitle").textContent = pageLabels[activePage]();
  dialog.querySelectorAll("[data-auxiliary-open]").forEach((button) => {
    button.textContent = pageLabels[button.dataset.auxiliaryOpen]();
    button.setAttribute("aria-current", button.dataset.auxiliaryOpen === activePage ? "page" : "false");
  });
  for (const [name, frame] of frames) frame.title = pageLabels[name]();
}
