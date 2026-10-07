import en from "./en.js";
import ko from "./ko.js";
import ja from "./ja.js";
import hi from "./hi.js";
import es from "./es.js";
import fr from "./fr.js";

export const defaultLanguage = "en";
export const localeDefinitions = Object.freeze([
  { code: "en", name: "English", manual: "en", messages: en },
  { code: "ko", name: "한국어", manual: "ko", messages: ko },
  { code: "ja", name: "日本語", manual: "en", messages: ja },
  { code: "hi", name: "हिन्दी", manual: "en", messages: hi },
  { code: "es", name: "Español", manual: "en", messages: es },
  { code: "fr", name: "Français", manual: "en", messages: fr },
]);
export const supportedLanguages = localeDefinitions.map(({ code, name }) => [code, name]);
export const localeMessages = Object.freeze(Object.fromEntries(localeDefinitions.map(({ code, messages }) => [code, messages])));
