import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";
import { evaluateHVACOperation } from "../hvac-operation.js";
import { renderHVACAvailability, connectHVACAvailability } from "./hvac-availability-view.js";
import { renderHVACControlExpressions } from "./hvac-control-expression-view.js";

const copy = (key, fallback) => t(`hvac.operation.${key}`, {}, fallback);
let disconnectAvailability = null;

export function renderHVACOperationDetail(profile, schedules = [], { component = {}, calendar = {} } = {}) {
  const model = evaluateHVACOperation(profile || {}, schedules, calendar);
  const control = renderHVACControlExpressions({ ...profile, parameterControl: model.parameterControl }, component, model.calendar);
  const panel = (mode, title, body) => `<section class="hvac-operation-panel ${mode}" data-hvac-operation-${mode}><div class="hvac-operation-panel-heading"><h4>${escapeHTML(title)}</h4></div>${body}</section>`;
  return `<section class="hvac-operation-detail" data-hvac-operation-detail data-component-index="${escapeHTML(component.objectIndex ?? profile?.componentIndex ?? "")}">
    <header class="hvac-operation-heading"><h3>${escapeHTML(component.objectName || copy("equipment", "Equipment"))}</h3><span class="hvac-operation-object-type">${escapeHTML(component.objectType || "")}</span></header>
    <div class="hvac-operation-columns">
      ${panel("availability", copy("availability", "Availability"), renderHVACAvailability(profile, schedules, model.calendar))}
      ${panel("onoff", copy("onOff", "On / Off decisions"), control.onOff)}
      ${panel("parameters", copy("parameterControl", "Parameter control"), control.parameters)}
    </div>
  </section>`;
}

export function connectHVACOperationDetail(container) {
  disconnectAvailability?.();
  disconnectAvailability = connectHVACAvailability(container);
}
