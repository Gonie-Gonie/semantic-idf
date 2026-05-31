export const settingsStorageKey = "idfAnalyzer.appSettings";

export const defaultAppSettings = {
  version: 1,
  appearance: {
    theme: "system",
    geometry: {
      background: "#f7fafc",
      zone: "#b8d7b0",
      wall: "#7b9cbc",
      roof: "#b8b0a1",
      window: "#3fb6d4",
      selected: "#f0a202",
    },
  },
  behavior: {
    autoAnalyzeDelayMs: 900,
  },
  interaction: {
    syncRawTextPosition: true,
    geometrySyncLocate: true,
  },
};

let currentSettings = mergeSettings();
let systemThemeQuery = null;
let systemThemeListenerAttached = false;

export function getCurrentAppSettings() {
  return mergeSettings(currentSettings);
}

export function readCachedAppSettings() {
  try {
    const raw = window.localStorage.getItem(settingsStorageKey);
    return raw ? mergeSettings(JSON.parse(raw)) : mergeSettings();
  } catch {
    return mergeSettings();
  }
}

export function applyCachedAppSettings() {
  return applyAppSettings(readCachedAppSettings());
}

export async function loadAndApplyAppSettings() {
  const result = await loadAppSettings();
  applyAppSettings(result.settings);
  return result;
}

export async function loadAppSettings() {
  try {
    const api = await waitForAppAPI("GetSettings");
    const result = api
      ? await api.GetSettings()
      : await fetch("/api/settings").then((response) => {
          if (!response.ok) {
            throw new Error(`Settings request failed: ${response.status}`);
          }
          return response.json();
        });
    const settings = mergeSettings(result?.settings);
    cacheSettings(settings);
    currentSettings = settings;
    return { ...result, settings };
  } catch (error) {
    const settings = readCachedAppSettings();
    currentSettings = settings;
    return {
      path: "",
      settings,
      warning: error?.message || String(error),
    };
  }
}

export async function saveAppSettings(settingsInput) {
  const settings = mergeSettings(settingsInput);
  const api = await waitForAppAPI("SaveSettings");
  const result = api
    ? await api.SaveSettings(settings)
    : await fetch("/api/settings", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(settings),
      }).then((response) => {
        if (!response.ok) {
          throw new Error(`Settings save failed: ${response.status}`);
        }
        return response.json();
      });
  const savedSettings = mergeSettings(result?.settings || settings);
  cacheSettings(savedSettings);
  applyAppSettings(savedSettings);
  window.dispatchEvent(new CustomEvent("idfAnalyzer:settingsChanged", { detail: { settings: savedSettings } }));
  return { ...result, settings: savedSettings };
}

export function applyAppSettings(settingsInput) {
  currentSettings = mergeSettings(settingsInput);
  const resolvedTheme = resolvedThemeName(currentSettings.appearance.theme);
  document.documentElement.dataset.theme = resolvedTheme;
  document.documentElement.dataset.themePreference = currentSettings.appearance.theme;
  const geometry = currentSettings.appearance.geometry;
  Object.entries(geometry).forEach(([name, value]) => {
    document.documentElement.style.setProperty(`--geometry-${name}`, value);
  });
  attachSystemThemeListener();
  return mergeSettings(currentSettings);
}

export function mergeSettings(settingsInput = {}) {
  const settings = settingsInput || {};
  const appearance = settings.appearance || {};
  const geometry = appearance.geometry || {};
  const behavior = settings.behavior || {};
  const interaction = settings.interaction || {};
  return {
    version: Number(settings.version) || defaultAppSettings.version,
    appearance: {
      theme: normalizeTheme(appearance.theme),
      geometry: {
        background: normalizeHexColor(geometry.background, defaultAppSettings.appearance.geometry.background),
        zone: normalizeHexColor(geometry.zone, defaultAppSettings.appearance.geometry.zone),
        wall: normalizeHexColor(geometry.wall, defaultAppSettings.appearance.geometry.wall),
        roof: normalizeHexColor(geometry.roof, defaultAppSettings.appearance.geometry.roof),
        window: normalizeHexColor(geometry.window, defaultAppSettings.appearance.geometry.window),
        selected: normalizeHexColor(geometry.selected, defaultAppSettings.appearance.geometry.selected),
      },
    },
    behavior: {
      autoAnalyzeDelayMs: clampNumber(
        behavior.autoAnalyzeDelayMs,
        150,
        5000,
        defaultAppSettings.behavior.autoAnalyzeDelayMs,
      ),
    },
    interaction: {
      syncRawTextPosition:
        typeof interaction.syncRawTextPosition === "boolean"
          ? interaction.syncRawTextPosition
          : defaultAppSettings.interaction.syncRawTextPosition,
      geometrySyncLocate:
        typeof interaction.geometrySyncLocate === "boolean"
          ? interaction.geometrySyncLocate
          : defaultAppSettings.interaction.geometrySyncLocate,
    },
  };
}

export function normalizeHexColor(value, fallback) {
  let color = String(value || "").trim().toLowerCase();
  if (!color) {
    return fallback;
  }
  if (!color.startsWith("#")) {
    color = `#${color}`;
  }
  if (/^#[0-9a-f]{3}$/.test(color)) {
    return `#${color[1]}${color[1]}${color[2]}${color[2]}${color[3]}${color[3]}`;
  }
  return /^#[0-9a-f]{6}$/.test(color) ? color : fallback;
}

function normalizeTheme(value) {
  const theme = String(value || "").trim().toLowerCase();
  return theme === "light" || theme === "dark" || theme === "system" ? theme : defaultAppSettings.appearance.theme;
}

function clampNumber(value, min, max, fallback) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return fallback;
  }
  return Math.min(Math.max(Math.round(number), min), max);
}

function cacheSettings(settings) {
  try {
    window.localStorage.setItem(settingsStorageKey, JSON.stringify(settings));
  } catch {
    // localStorage can be unavailable in hardened webview settings.
  }
}

function resolvedThemeName(theme) {
  if (theme === "dark" || theme === "light") {
    return theme;
  }
  return systemPrefersDark() ? "dark" : "light";
}

function systemPrefersDark() {
  return Boolean(window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches);
}

function attachSystemThemeListener() {
  if (!window.matchMedia || systemThemeListenerAttached) {
    return;
  }
  systemThemeQuery = window.matchMedia("(prefers-color-scheme: dark)");
  const refreshSystemTheme = () => {
    if (currentSettings.appearance.theme === "system") {
      document.documentElement.dataset.theme = resolvedThemeName("system");
    }
  };
  if (typeof systemThemeQuery.addEventListener === "function") {
    systemThemeQuery.addEventListener("change", refreshSystemTheme);
  } else if (typeof systemThemeQuery.addListener === "function") {
    systemThemeQuery.addListener(refreshSystemTheme);
  }
  systemThemeListenerAttached = true;
}

async function waitForAppAPI(methodName) {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    const api = window.go && window.go.main && window.go.main.App;
    if (api && typeof api[methodName] === "function") {
      return api;
    }
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  return null;
}
