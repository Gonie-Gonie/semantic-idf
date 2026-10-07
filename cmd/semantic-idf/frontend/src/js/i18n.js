import { defaultLanguage, supportedLanguages, localeMessages, localeDefinitions } from "./locales/index.js";

export { defaultLanguage, supportedLanguages };
export const defaultAnalyzeTabOrder = ["metrics", "topology", "profile", "hvac", "simulation"];
const dictionaries = localeMessages;

const localeAliases = { kr: "ko", jp: "ja" };
let currentLanguage = defaultLanguage;

export function normalizeLanguage(value) {
  const language = String(value || "").trim().toLowerCase().replaceAll("_", "-").split("-")[0];
  const normalized = localeAliases[language] || language;
  return supportedLanguages.some(([code]) => code === normalized) ? normalized : defaultLanguage;
}

export function getLanguage() {
  return currentLanguage;
}

export function getManualLanguage(value = currentLanguage) {
  return localeDefinitions.find(({ code }) => code === normalizeLanguage(value))?.manual || defaultLanguage;
}

export function setLanguage(language) {
  const previousLanguage = currentLanguage;
  currentLanguage = normalizeLanguage(language);
  document.documentElement.lang = currentLanguage;
  translatePage();
  applyAnalyzeTabOrder();
  if (currentLanguage !== previousLanguage) {
    window.dispatchEvent(new CustomEvent("idfAnalyzer:languageChanged", { detail: { language: currentLanguage, previousLanguage } }));
  }
  return currentLanguage;
}

export function t(key, params = {}, fallback = "") {
  return translateForLanguage(currentLanguage, key, params, fallback);
}

export function translateForLanguage(language, key, params = {}, fallback = "") {
  const normalizedLanguage = normalizeLanguage(language);
  const source = dictionaries[normalizedLanguage]?.[key] ?? dictionaries[defaultLanguage][key] ?? (fallback || key);
  return interpolate(source, params, normalizedLanguage);
}

export function localizedMessage(key, params = {}, fallback = "") {
  return { i18nKey: key, params, fallback };
}

export function resolveLocalizedText(value, language = currentLanguage) {
  if (typeof value === "function") return resolveLocalizedText(value(), language);
  if (value && typeof value === "object" && typeof value.i18nKey === "string") {
    return translateForLanguage(language, value.i18nKey, typeof value.params === "function" ? value.params() : value.params, value.fallback);
  }
  return String(value ?? "");
}

export function translatePage(root = document) {
  root.querySelectorAll("[data-i18n]").forEach((element) => {
    element.textContent = t(element.dataset.i18n, datasetParams(element), element.textContent);
  });
  root.querySelectorAll("[data-i18n-placeholder]").forEach((element) => {
    element.setAttribute("placeholder", t(element.dataset.i18nPlaceholder, datasetParams(element), element.getAttribute("placeholder") || ""));
  });
  root.querySelectorAll("[data-i18n-aria-label]").forEach((element) => {
    element.setAttribute("aria-label", t(element.dataset.i18nAriaLabel, datasetParams(element), element.getAttribute("aria-label") || ""));
  });
  root.querySelectorAll("[data-i18n-title]").forEach((element) => {
    element.setAttribute("title", t(element.dataset.i18nTitle, datasetParams(element), element.getAttribute("title") || ""));
  });
}

export function normalizeAnalyzeTabOrder(value, fallback = defaultAnalyzeTabOrder) {
  const allowed = new Set(defaultAnalyzeTabOrder);
  const source = Array.isArray(value) ? value : String(value || "").split(",");
  const order = [];
  for (const item of source) {
    const legacyID = String(item || "").trim().toLowerCase();
    const id = legacyID === "summary" ? "metrics" : legacyID === "geometry" ? "topology" : legacyID;
    if (allowed.has(id) && !order.includes(id)) {
      order.push(id);
    }
  }
  for (const item of fallback) {
    if (allowed.has(item) && !order.includes(item)) {
      order.push(item);
    }
  }
  for (const item of defaultAnalyzeTabOrder) {
    if (!order.includes(item)) {
      order.push(item);
    }
  }
  return order;
}

export function applyAnalyzeTabOrder(orderInput, root = document) {
  const order = normalizeAnalyzeTabOrder(orderInput || readStoredAnalyzeTabOrder());
  const nav = root.querySelector(".analysis-panel .tabs");
  if (!nav) {
    return order;
  }
  for (const tab of order) {
    const button = nav.querySelector(`[data-result-tab="${cssEscape(tab)}"]`);
    if (button) {
      nav.appendChild(button);
    }
  }
  nav.querySelectorAll("[data-result-tab]").forEach((button) => {
    button.textContent = analysisTabLabel(button.dataset.resultTab);
  });
  return order;
}

export function storeAnalyzeTabOrder(orderInput) {
  const order = normalizeAnalyzeTabOrder(orderInput);
  document.documentElement.dataset.analysisTabOrder = order.join(",");
  return order;
}

export function analysisTabLabel(tab) {
  return t(`tab.${tab}`, {}, tab);
}

export function profileDimensionLabel(dimension, fallback = "") {
  return t(`profile.dimension.${dimension}`, {}, fallback || dimension);
}

export function profileMetricLabel(dimension, metric, fallback = "") {
  return t(`profile.metric.${dimension}.${metric}`, {}, fallback || metric);
}

export function optionHTML(value, label, selected, escapeHTML) {
  return `<option value="${escapeHTML(value)}" ${String(selected) === String(value) ? "selected" : ""}>${escapeHTML(label)}</option>`;
}

function readStoredAnalyzeTabOrder() {
  return document.documentElement.dataset.analysisTabOrder || defaultAnalyzeTabOrder;
}

function interpolate(text, params, language) {
  return String(text).replace(/\{([a-zA-Z0-9_]+)\}/g, (match, name) =>
    Object.prototype.hasOwnProperty.call(params || {}, name) ? resolveLocalizedText(params[name], language) : match,
  );
}

function datasetParams(element) {
  const raw = element.dataset.i18nParams;
  if (!raw) {
    return {};
  }
  try {
    return JSON.parse(raw);
  } catch {
    return {};
  }
}

function cssEscape(value) {
  if (window.CSS && typeof window.CSS.escape === "function") {
    return window.CSS.escape(value);
  }
  return String(value).replace(/"/g, '\\"');
}
