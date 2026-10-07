import { applyCachedAppSettings } from "./js/settings-client.js";
import { renderAppInfo } from "./js/app-info.js";
import { localizedMessage } from "./js/i18n.js";
import { setStatus } from "./js/state.js";

applyCachedAppSettings();
renderAppInfo();

setStatus(localizedMessage("status.loadingInterface"), "loading");

function showStartupError(error) {
  setStatus(error?.message || String(error), "error");
}

window.addEventListener("error", (event) => showStartupError(event.error || event.message));
window.addEventListener("unhandledrejection", (event) => showStartupError(event.reason || event));
