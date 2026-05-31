import { applyCachedAppSettings } from "./js/settings-client.js";
import { renderAppInfo } from "./js/app-info.js";

applyCachedAppSettings();
renderAppInfo();

const status = document.querySelector("#runtimeStatus");
if (status) {
  status.textContent = "Loading interface";
}

function boot() {
  import("./js/main.js").catch((error) => {
    if (status) {
      status.textContent = error?.message || String(error);
      status.style.color = "#b3261e";
    }
  });
}

window.setTimeout(boot, 0);
