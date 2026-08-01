import { elements, escapeHTML, state } from "../state.js";
import { t } from "../i18n.js";
import { resolveThermalTopologyTarget } from "../thermal-topology-targets.js";

export function renderThermalTopologyInspector(geometry, navigationAttributes) {
  if (!elements.thermalTopologyInspector) {
    return;
  }
  const targetId = state.thermalTopologySelectedEntityId || state.selectedGeometryId;
  const targetKind = state.selectedGeometryKind;
  const resolved = targetId ? resolveThermalTopologyTarget({ targetKind, targetId }, geometry) : null;
  if (!resolved?.item) {
    elements.thermalTopologyInspector.innerHTML = `<div class="empty">${escapeHTML(t("topology.inspectorEmpty", {}, "Select a thermal node or connection"))}</div>`;
    return;
  }
  const item = resolved.item;
  const title = item.label || item.surfaceName || item.name || item.objectName || item.id || targetId;
  const subtitle = item.relationKind || item.kind || targetKind;
  const attributes = navigationAttributes?.(targetKind, targetId) || "";
  elements.thermalTopologyInspector.innerHTML = `
    <div class="geometry-detail-head navigable-row" ${attributes}>
      <div>
        <h3>${escapeHTML(title)}</h3>
        <span>${escapeHTML(subtitle)}</span>
      </div>
    </div>
    <div class="empty">${escapeHTML(t("topology.inspectorReady", {}, "Thermal properties and actions will appear here"))}</div>`;
}
