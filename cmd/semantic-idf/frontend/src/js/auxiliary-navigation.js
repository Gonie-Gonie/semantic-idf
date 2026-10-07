const auxiliaryNavigationStorageKey = "idfAnalyzer.auxiliaryNavigation";
const auxiliaryReturnDepthStorageKey = "idfAnalyzer.auxiliaryReturnDepth";

function validReturnDepth(value) {
  return Number.isInteger(value) && value > 0 && value < window.history.length;
}

export function auxiliaryReturnDepth() {
  const entryDepth = window.history.state?.auxiliaryReturnDepth;
  if (validReturnDepth(entryDepth)) return entryDepth;
  try {
    const value = Number(window.sessionStorage.getItem(auxiliaryReturnDepthStorageKey));
    return validReturnDepth(value) ? value : 1;
  } catch { return 1; }
}

export function setAuxiliaryReturnDepth(depth) {
  if (!validReturnDepth(depth)) return;
  window.history.replaceState({ ...window.history.state, auxiliaryReturnDepth: depth }, "");
  try { window.sessionStorage.setItem(auxiliaryReturnDepthStorageKey, String(depth)); } catch { /* Normal links still work. */ }
}

if (hasMainHistoryEntry()) setAuxiliaryReturnDepth(auxiliaryReturnDepth());

document.addEventListener("click", (event) => {
  const link = event.target.closest?.("a[data-app-return], a[data-app-auxiliary]");
  if (!link || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
    return;
  }

  if (!hasMainHistoryEntry()) {
    return;
  }

  event.preventDefault();
  if (link.matches("[data-app-return]")) {
    const returnDepth = Number(link.dataset.appReturnDepth) || auxiliaryReturnDepth();
    clearMainHistoryEntry();
    if (Number.isInteger(returnDepth) && returnDepth > 1 && returnDepth < window.history.length) {
      window.history.go(-returnDepth);
    } else {
      window.history.back();
    }
    return;
  }

  window.location.replace(link.href);
});

function hasMainHistoryEntry() {
  try {
    return window.sessionStorage.getItem(auxiliaryNavigationStorageKey) === "main" && window.history.length > 1;
  } catch {
    return false;
  }
}

function clearMainHistoryEntry() {
  try {
    window.sessionStorage.removeItem(auxiliaryNavigationStorageKey);
    window.sessionStorage.removeItem(auxiliaryReturnDepthStorageKey);
  } catch {
    // The normal index.html href remains the fallback when storage is unavailable.
  }
}
