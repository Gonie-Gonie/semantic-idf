import { applyCachedAppSettings } from "./js/settings-client.js";
import { renderAppInfo } from "./js/app-info.js";
import { t } from "./js/i18n.js";

applyCachedAppSettings();
renderAppInfo();

const status = document.querySelector("#runtimeStatus");
if (status) {
  status.textContent = t("status.loadingInterface");
}

function showStartupError(error) {
  if (!status) {
    return;
  }
  status.textContent = error?.message || String(error);
  status.style.color = "#b3261e";
  status.classList.remove("status-loading");
}

window.addEventListener("error", (event) => showStartupError(event.error || event.message));
window.addEventListener("unhandledrejection", (event) => showStartupError(event.reason || event));
