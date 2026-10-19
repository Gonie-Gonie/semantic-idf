import { escapeHTML } from "../state.js";
import { t } from "../i18n.js";
import { buildHVACControlExpressions } from "../hvac-control-expressions.js";
import { renderHVACOnOffFlow, connectHVACOnOffFlow } from "./hvac-onoff-flow-view.js";
import { renderHVACParameterFlow, connectHVACParameterFlow } from "./hvac-parameter-flow-view.js";

export function renderHVACControlExpressions(profile, component, calendar) {
  const model = buildHVACControlExpressions(profile || {}, { component });
  const unavailable = `<p class="hvac-operation-unresolved" data-hvac-graph-unavailable>${escapeHTML(t("hvac.operation.graphUnavailable"))}</p>`;
  return {
    onOff: model.onOffPresentation?.formulas?.length ? renderHVACOnOffFlow(model.onOffPresentation, profile || {}) : unavailable,
    parameters: model.parameters?.nodes?.length ? `<div data-hvac-operation-graph="parameters">${renderHVACParameterFlow(model.parameters, { component, calendar })}</div>` : unavailable,
  };
}

export function connectHVACControlExpressions(container) {
  const disconnectOnOff = connectHVACOnOffFlow(container), disconnectParameters = connectHVACParameterFlow(container);
  return () => { disconnectOnOff?.(); disconnectParameters?.(); };
}
