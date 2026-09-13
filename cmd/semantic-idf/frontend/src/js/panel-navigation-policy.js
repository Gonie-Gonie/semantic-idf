// HVAC and simulation have their own selections. They do not reveal input
// objects or participate in semantic navigation between workspace panels.
export function isStandaloneResultView(view) {
  return ["hvac", "simulation"].includes(String(view || "").trim().toLowerCase());
}

export function isStandalonePanelLink(originView, targetView) {
  return isStandaloneResultView(originView) || isStandaloneResultView(targetView);
}
