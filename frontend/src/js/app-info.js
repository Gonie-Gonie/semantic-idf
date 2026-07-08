export const bundledAppInfo = {
  name: "SemanticIDF",
  version: "0.4.2",
  title: "SemanticIDF v0.4.2",
  outputFilename: "semantic-idf-v0.4.2",
};

let cachedAppInfo = null;

export async function loadAppInfo() {
  if (cachedAppInfo) {
    return { ...cachedAppInfo };
  }

  const api = window.go && window.go.main && window.go.main.App;
  if (api) {
    try {
      cachedAppInfo = normalizeAppInfo(await api.GetAppInfo());
      return { ...cachedAppInfo };
    } catch {
      // Fall through to the HTTP endpoint/static fallback.
    }
  }

  try {
    const response = await fetch("/api/app-info");
    if (response.ok) {
      cachedAppInfo = normalizeAppInfo(await response.json());
      return { ...cachedAppInfo };
    }
  } catch {
    // Static file previews do not expose the desktop app API.
  }

  cachedAppInfo = normalizeAppInfo(bundledAppInfo);
  return { ...cachedAppInfo };
}

export async function renderAppInfo(appInfoInput) {
  const info = normalizeAppInfo(appInfoInput || (await loadAppInfo()));
  document.querySelectorAll("[data-app-name]").forEach((element) => {
    element.textContent = info.name;
  });
  document.querySelectorAll("[data-app-title]").forEach((element) => {
    element.textContent = info.title;
  });
  document.querySelectorAll("[data-app-brand-version]").forEach((element) => {
    element.textContent = `${info.name} v${info.version}`;
  });
  document.querySelectorAll("[data-app-version]").forEach((element) => {
    element.textContent = `v${info.version}`;
  });
  updateDocumentTitle(info);
  return info;
}

export function formatAppVersion(appInfoInput) {
  const info = normalizeAppInfo(appInfoInput || bundledAppInfo);
  return `v${info.version}`;
}

function normalizeAppInfo(input = {}) {
  const name = String(input.name || bundledAppInfo.name).trim() || bundledAppInfo.name;
  const version = String(input.version || bundledAppInfo.version).trim() || bundledAppInfo.version;
  const defaultOutputFilename = `semantic-idf-v${version}`;
  const outputFilename = String(input.outputFilename || defaultOutputFilename).trim() || defaultOutputFilename;
  const title = String(input.title || `${name} v${version}`).trim() || `${name} v${version}`;
  return { name, version, title, outputFilename };
}

function updateDocumentTitle(info) {
  const suffix = document.title.replace(/^SemanticIDF(?: v\d+\.\d+\.\d+)?\s*/i, "").trim();
  document.title = suffix ? `${info.title} ${suffix}` : info.title;
}
