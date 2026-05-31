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

if (typeof window.requestAnimationFrame === "function") {
  window.requestAnimationFrame(() => window.setTimeout(boot, 0));
} else {
  window.setTimeout(boot, 0);
}
