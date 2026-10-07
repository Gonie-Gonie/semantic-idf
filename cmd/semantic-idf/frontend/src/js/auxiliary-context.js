// Auxiliary pages share the Main WebView's Wails callback registry and event bus.
// Separate proxies in a frame cannot receive native callbacks sent to Main.
export function getAuxiliaryHost() {
  try {
    if (window.parent === window) return null;
    const host = window.parent.idfAnalyzerAuxiliaryHost;
    return host?.contains(window) ? host : null;
  } catch {
    return null;
  }
}

if (getAuxiliaryHost()) {
  document.documentElement.dataset.embeddedApp = "true";
  for (const name of ["go", "runtime"]) {
    Object.defineProperty(window, name, {
      configurable: true,
      get: () => window.parent[name],
      // Late native script initialization must not create a second callback bus.
      set: () => {},
    });
  }
}
