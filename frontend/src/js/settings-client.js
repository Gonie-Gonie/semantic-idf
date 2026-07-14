import {
  applyAnalyzeTabOrder,
  defaultAnalyzeTabOrder,
  normalizeAnalyzeTabOrder,
  normalizeLanguage,
  setLanguage,
  storeAnalyzeTabOrder,
  translatePage,
} from "./i18n.js";

export const settingsStorageKey = "idfAnalyzer.appSettings";

export const defaultAppSettings = {
  version: 1,
  appearance: {
    theme: "system",
    language: "en",
    analysisTabOrder: [...defaultAnalyzeTabOrder],
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
    shortcuts: {
      save: "Ctrl+S",
      open: "Ctrl+O",
      undoView: "Alt+Left",
      redoView: "Alt+Right",
      jumpDefinition: "F12",
      jumpReferences: "Shift+F12",
      commandPalette: "Ctrl+K",
      revealSemantic: "Ctrl+Shift+L",
      revealSource: "Ctrl+Shift+S",
      paneFocus: "F6",
      currentSearch: "/",
      primaryOpen: "Enter",
      availableViews: "Alt+Enter",
      clearSelection: "Escape",
      inputSemantic: "Ctrl+1",
      inputText: "Ctrl+2",
      inputJson: "Ctrl+3",
      inputTable: "Ctrl+4",
      tabSummary: "Ctrl+Alt+1",
      tabProfile: "Ctrl+Alt+2",
      tabHVAC: "Ctrl+Alt+3",
      tabOutput: "Ctrl+Alt+4",
      tabSimulation: "Ctrl+Alt+5",
      tabDiagnose: "Ctrl+Alt+6",
      tabGeometry: "Ctrl+Alt+7",
    },
  },
  profile: {
    enabledDimensions: ["occupancy", "lighting", "equipment", "infiltration", "ventilation", "outdoor_air"],
    displayMetrics: {
      occupancy: "people_per_area",
      lighting: "power_per_area",
      equipment: "power_per_area",
      infiltration: "ach",
      ventilation: "flow_per_person",
      outdoor_air: "flow_per_person",
    },
    groupingMetrics: {
      occupancy: "people_per_area",
      lighting: "power_per_area",
      equipment: "power_per_area",
      infiltration: "ach",
      ventilation: "flow_per_person",
      outdoor_air: "flow_per_person",
    },
    numericTolerance: 0.001,
    scheduleCompareMode: "name",
    graphMode: "actual_value",
    scheduleSummaryMode: "annual_heatmap",
    applyBehavior: {
      defaultMode: "clone",
      allowZoneListEdit: false,
      createMissingZoneList: false,
      nameSuffix: " Profile Copy",
      replaceExistingPolicy: "replace",
    },
  },
  simulation: {
    energyPlusInstallations: [],
    extraWeatherDataPaths: [],
    runDirectory: "",
    workerFraction: 0.5,
    maxWorkers: 0,
    autoRunOnOpen: false,
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
  setLanguage(currentSettings.appearance.language);
  const analysisTabOrder = storeAnalyzeTabOrder(currentSettings.appearance.analysisTabOrder);
  applyAnalyzeTabOrder(analysisTabOrder);
  translatePage();
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
  const defaultShortcuts = defaultAppSettings.interaction.shortcuts;
  const profile = settings.profile || {};
  const applyBehavior = profile.applyBehavior || {};
  const defaultProfile = defaultAppSettings.profile;
  const simulation = settings.simulation || {};
  const defaultSimulation = defaultAppSettings.simulation;
  return {
    version: Number(settings.version) || defaultAppSettings.version,
    appearance: {
      theme: normalizeTheme(appearance.theme),
      language: normalizeLanguage(appearance.language),
      analysisTabOrder: normalizeAnalyzeTabOrder(appearance.analysisTabOrder),
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
      shortcuts: migrateLegacyNavigationShortcuts(normalizeShortcuts(interaction.shortcuts, defaultShortcuts)),
    },
    profile: {
      enabledDimensions: normalizeEnabledDimensions(profile.enabledDimensions, defaultProfile.enabledDimensions),
      displayMetrics: normalizeMetricMap(profile.displayMetrics, defaultProfile.displayMetrics),
      groupingMetrics: normalizeMetricMap(profile.groupingMetrics, defaultProfile.groupingMetrics),
      numericTolerance: clampFloat(profile.numericTolerance, 0.000001, 1000, defaultProfile.numericTolerance),
      scheduleCompareMode: normalizeChoice(profile.scheduleCompareMode, ["none", "name", "resolved"], defaultProfile.scheduleCompareMode),
      graphMode: normalizeChoice(profile.graphMode, ["multiplier", "actual_value"], defaultProfile.graphMode),
      scheduleSummaryMode: normalizeChoice(
        profile.scheduleSummaryMode,
        ["representative_day", "representative_week", "monthly_average", "hourly_average_by_daytype", "load_duration", "annual_heatmap"],
        defaultProfile.scheduleSummaryMode,
      ),
      applyBehavior: {
        defaultMode: normalizeChoice(applyBehavior.defaultMode, ["clone", "shared"], defaultProfile.applyBehavior.defaultMode),
        allowZoneListEdit:
          typeof applyBehavior.allowZoneListEdit === "boolean"
            ? applyBehavior.allowZoneListEdit
            : defaultProfile.applyBehavior.allowZoneListEdit,
        createMissingZoneList:
          typeof applyBehavior.createMissingZoneList === "boolean"
            ? applyBehavior.createMissingZoneList
            : defaultProfile.applyBehavior.createMissingZoneList,
        nameSuffix: String(applyBehavior.nameSuffix || defaultProfile.applyBehavior.nameSuffix).trim() || defaultProfile.applyBehavior.nameSuffix,
        replaceExistingPolicy: normalizeChoice(
          applyBehavior.replaceExistingPolicy,
          ["replace", "keep", "duplicate"],
          defaultProfile.applyBehavior.replaceExistingPolicy,
        ),
      },
    },
    simulation: {
      energyPlusInstallations: Array.isArray(simulation.energyPlusInstallations)
        ? simulation.energyPlusInstallations.map(normalizeEnergyPlusInstallation).filter(Boolean)
        : [...defaultSimulation.energyPlusInstallations],
      extraWeatherDataPaths: normalizeStringList(simulation.extraWeatherDataPaths, defaultSimulation.extraWeatherDataPaths),
      runDirectory: String(simulation.runDirectory || defaultSimulation.runDirectory || "").trim(),
      workerFraction: clampFloat(simulation.workerFraction, 0.1, 1, defaultSimulation.workerFraction),
      maxWorkers: Math.max(0, clampNumber(simulation.maxWorkers, 0, 512, defaultSimulation.maxWorkers)),
      autoRunOnOpen:
        typeof simulation.autoRunOnOpen === "boolean" ? simulation.autoRunOnOpen : defaultSimulation.autoRunOnOpen,
    },
  };
}

function migrateLegacyNavigationShortcuts(shortcuts) {
  const migrated = { ...shortcuts };
  if (normalizedShortcutList(migrated.undoView) === "Ctrl+Z") {
    migrated.undoView = "Alt+Left";
  }
  const redo = normalizedShortcutList(migrated.redoView);
  if (redo === "Ctrl+Y, Ctrl+Shift+Z" || redo === "Ctrl+Shift+Z, Ctrl+Y" || redo === "Ctrl+Y") {
    migrated.redoView = "Alt+Right";
  }
  return migrated;
}

function normalizedShortcutList(value) {
  return String(value || "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean)
    .join(", ");
}

function normalizeEnergyPlusInstallation(value) {
  if (!value || typeof value !== "object") {
    return null;
  }
  const executablePath = String(value.executablePath || "").trim();
  const rootPath = String(value.rootPath || "").trim();
  if (!executablePath && !rootPath) {
    return null;
  }
  return {
    id: String(value.id || "").trim(),
    version: String(value.version || "").trim(),
    name: String(value.name || "").trim(),
    executablePath,
    rootPath,
    weatherDataPath: String(value.weatherDataPath || "").trim(),
    autoDetected: Boolean(value.autoDetected),
  };
}

function normalizeStringList(values, fallback = []) {
  const source = Array.isArray(values) ? values : fallback;
  const seen = new Set();
  const out = [];
  for (const item of source) {
    const value = String(item || "").trim();
    const key = value.toLowerCase();
    if (!value || seen.has(key)) {
      continue;
    }
    seen.add(key);
    out.push(value);
  }
  return out;
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

function normalizeShortcuts(value, fallback) {
  const rawSource = value && typeof value === "object" ? value : {};
  const source = Object.hasOwn(rawSource, "inputSemantic") ? rawSource : migrateInputViewShortcutDefaults(rawSource);
  return Object.fromEntries(
    Object.entries(fallback).map(([id, defaultAccelerator]) => [
      id,
      normalizeShortcutValue(source[id], defaultAccelerator),
    ]),
  );
}

function migrateInputViewShortcutDefaults(source) {
  const migrated = { ...source };
  if (isBlankOrShortcut(migrated.inputText, "Ctrl+1")) {
    migrated.inputText = "Ctrl+2";
  }
  if (isBlankOrShortcut(migrated.inputJson, "Ctrl+2")) {
    migrated.inputJson = "Ctrl+3";
  }
  if (isBlankOrShortcut(migrated.inputTable, "Ctrl+3")) {
    migrated.inputTable = "Ctrl+4";
  }
  return migrated;
}

function isBlankOrShortcut(value, expected) {
  const shortcut = String(value || "").trim();
  return shortcut === "" || shortcut.toLowerCase() === expected.toLowerCase();
}

function normalizeShortcutValue(value, fallback) {
  const shortcut = String(value || "").trim();
  return shortcut || fallback;
}

function clampNumber(value, min, max, fallback) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return fallback;
  }
  return Math.min(Math.max(Math.round(number), min), max);
}

function clampFloat(value, min, max, fallback) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return fallback;
  }
  return Math.min(Math.max(number, min), max);
}

function normalizeChoice(value, allowed, fallback) {
  const normalized = String(value || "").trim().toLowerCase();
  return allowed.includes(normalized) ? normalized : fallback;
}

function normalizeEnabledDimensions(value, fallback) {
  const allowed = new Set(defaultAppSettings.profile.enabledDimensions);
  const values = Array.isArray(value) ? value.map((item) => String(item || "").trim()).filter((item) => allowed.has(item)) : [];
  return values.length ? [...new Set(values)] : [...fallback];
}

function normalizeMetricMap(value, fallback) {
  const source = value && typeof value === "object" ? value : {};
  return Object.fromEntries(Object.entries(fallback).map(([dimension, metric]) => [dimension, String(source[dimension] || metric).trim() || metric]));
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
