const auxiliaryNavigationStorageKey = "idfAnalyzer.auxiliaryNavigation";

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
    clearMainHistoryEntry();
    window.history.back();
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
  } catch {
    // The normal index.html href remains the fallback when storage is unavailable.
  }
}
