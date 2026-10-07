import { t } from "./i18n.js";

// Retain a dynamic label's translation key so language changes never restore
// the page's original placeholder instead of the current status or count.
export function setLocalizedText(element, key, params = {}, fallback = "") {
  if (!element) return;
  element.dataset.i18n = key;
  element.dataset.i18nParams = JSON.stringify(params);
  element.textContent = t(key, params, fallback);
}

export function setLiteralText(element, value) {
  if (!element) return;
  delete element.dataset.i18n;
  delete element.dataset.i18nParams;
  element.textContent = String(value ?? "");
}
