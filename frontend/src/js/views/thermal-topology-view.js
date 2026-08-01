import {
  elements,
  escapeHTML,
  normalizeThermalTopologyAreaBasis,
  normalizeThermalTopologyGraphLevel,
  normalizeThermalTopologyLayout,
  normalizeThermalTopologyMetric,
  normalizeThermalTopologyScope,
  state,
} from "../state.js";
import { t } from "../i18n.js";
import { isThermalTopologyTargetKind } from "../thermal-topology-targets.js";
import { createThermalTopologyLayoutModel } from "./thermal-topology-layout.js";
import { renderThermalTopologyInspector } from "./thermal-topology-inspector.js";

let currentGeometry = null;
let currentHelpers = null;

window.addEventListener("idfAnalyzer:thermalTopologyFit", () => {
  state.thermalTopologyPanX = 0;
  state.thermalTopologyPanY = 0;
  state.thermalTopologyScale = 1;
  if (currentGeometry && state.geometryMode === "thermal") {
    renderThermalTopology(currentGeometry, currentHelpers);
  }
});

export function renderThermalTopology(geometry, helpers = {}) {
  if (!elements.thermalTopologyGraph) {
    return;
  }
  currentGeometry = geometry;
  currentHelpers = helpers;
  syncThermalTopologyControls();

  const model = createThermalTopologyLayoutModel(geometry, {
    graphLevel: state.thermalTopologyGraphLevel,
    layout: state.thermalTopologyLayout,
    scope: state.thermalTopologyScope,
    storyIndex: state.selectedGeometryStory,
    selectedEntityId: state.thermalTopologySelectedEntityId,
    neighborDepth: state.thermalTopologyNeighborDepth,
    areaBasis: state.thermalTopologyAreaBasis,
    showOpenings: state.thermalTopologyShowOpenings,
    showAirCoupling: state.thermalTopologyShowAirCoupling,
    expandExternalTargets: state.thermalTopologyExpandExternalTargets,
  });
  const count = model.connections.length || model.boundaries.length;
  const totalArea = model.boundaries.reduce((sum, boundary) => sum + (Number(boundary?.[model.areaField]) || 0), 0);
  const selectedTargetAttributes = isThermalTopologyTargetKind(state.selectedGeometryKind) && state.selectedGeometryId
    ? helpers.navigationAttributes?.(state.selectedGeometryKind, state.selectedGeometryId) || ""
    : "";
  const message = count
    ? t("topology.connectionAreaSummary", { count, area: formatArea(totalArea) })
    : t("topology.noConnections");
  elements.thermalTopologyGraph.innerHTML = `<div class="thermal-topology-shell-status" ${selectedTargetAttributes}>${escapeHTML(message)}</div>`;
  elements.thermalTopologyMatrix.hidden = true;
  renderThermalTopologyInspector(geometry, helpers.navigationAttributes);
}

function syncThermalTopologyControls() {
  elements.thermalTopologyGraphLevel.value = normalizeThermalTopologyGraphLevel(state.thermalTopologyGraphLevel);
  elements.thermalTopologyMetric.value = normalizeThermalTopologyMetric(state.thermalTopologyMetric);
  elements.thermalTopologyScope.value = normalizeThermalTopologyScope(state.thermalTopologyScope);
  elements.thermalTopologyLayout.value = normalizeThermalTopologyLayout(state.thermalTopologyLayout);
  elements.thermalTopologyAreaBasis.value = normalizeThermalTopologyAreaBasis(state.thermalTopologyAreaBasis);
  elements.thermalTopologyShowOpenings.checked = Boolean(state.thermalTopologyShowOpenings);
  elements.thermalTopologyShowAirCoupling.checked = Boolean(state.thermalTopologyShowAirCoupling);
  elements.thermalTopologyExpandExternalTargets.checked = Boolean(state.thermalTopologyExpandExternalTargets);
  elements.thermalTopologyShowLabels.checked = Boolean(state.thermalTopologyShowLabels);
}

function formatArea(value) {
  return `${Number(value || 0).toLocaleString(undefined, { maximumFractionDigits: 2 })} m²`;
}
