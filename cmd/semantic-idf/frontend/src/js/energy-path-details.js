import { t } from "./i18n.js";
import { escapeHTML } from "./state.js";
import { resolveEnergyPathOutputRequest, energyPathOutputRequestKey, energyPathOutputRequestFields } from "./energy-path-output-requests.js";

const STAGES = [
  { key: "drivers", label: "Drivers", domain: "thermal" },
  { key: "loads", label: "Loads", domain: "thermal" },
  { key: "endUses", label: "End uses", domain: "site" },
  { key: "carriers", label: "Carriers", domain: "site" },
];
const token = (value) => String(value || "").trim().toLowerCase();
const copy = (key, fallback, values = {}) => t(`simulation.energyPath${key}`, values, fallback);
const unavailable = () => ({ status: "unavailable", found: 0, total: 0 });
const percent = (value) => `${Number(value).toLocaleString(undefined, { maximumFractionDigits: 1 })}%`;
const numericValue = (value) => (typeof value === "number" || typeof value === "string" && value.trim() !== "") && Number.isFinite(Number(value)) ? Number(value) : null;

function scopedResult(explanation, viewState) {
  if (viewState.simulationEnergyScopeKind !== "zone") return explanation;
  const zone = token(viewState.simulationEnergyZoneName);
  return (explanation.zoneResults || []).find((result) => token(result.scope?.zoneName) === zone) ||
    (token(explanation.scope?.kind) === "zone" && token(explanation.scope?.zoneName) === zone ? explanation : null);
}

function selectedContext(explanation = {}, viewState = {}) {
  const result = scopedResult(explanation, viewState) || {};
  const periodID = viewState.simulationEnergyPeriod || "annual";
  const period = (result.periods || []).find((item) => token(item.id) === token(periodID));
  const annual = token(periodID) === "annual";
  return {
    result, period, periodID, annual,
    reconciliation: annual ? result.reconciliation || period?.reconciliation || [] : period?.reconciliation || [],
    sourceAvailability: result.completeness?.sourceAvailability || [],
  };
}

export function energyPathQualityForState(explanation = {}, viewState = {}) {
  const { result, period, annual } = selectedContext(explanation, viewState);
  const runQuality = result.quality || result.summary?.quality || {};
  const localQuality = annual ? result.quality || period?.quality || result.summary?.quality || {} : period?.quality || period?.summary?.quality || {};
  const quality = {};
  for (const stage of STAGES) quality[stage.key] = localQuality[stage.key] || runQuality[stage.key] || unavailable();
  quality.ratios = localQuality.ratios || unavailable();
  for (const field of ["driverToLoadClosedPct", "endUseToCarrierClosedPct", "zoneAllocatedPct", "unassignedPct"]) {
    const value = numericValue(localQuality[field]);
    quality[field] = value !== null && value >= 0 ? value : null;
  }
  for (const field of ["driverToLoadStatus", "endUseToCarrierStatus", "zoneAllocationStatus"]) {
    quality[field] = localQuality[field] || "unavailable";
  }
  for (const [status, fields] of [
    ["driverToLoadStatus", ["driverToLoadClosedPct"]],
    ["endUseToCarrierStatus", ["endUseToCarrierClosedPct"]],
    ["zoneAllocationStatus", ["zoneAllocatedPct", "unassignedPct"]],
  ]) {
    if (["complete", "partial", "overmapped"].includes(token(quality[status])) && fields.some((field) => quality[field] === null)) quality[status] = "unavailable";
  }
  return quality;
}

function statusLabel(status) {
  const labels = {
    complete: ["QualityComplete", "complete"],
    partial: ["QualityPartial", "partial"],
    missing: ["QualityMissing", "missing"],
    found: ["QualityFound", "found"],
    not_requested: ["QualityNotRequested", "Not requested"],
    not_applicable: ["QualityNotApplicable", "Not applicable"],
    unavailable: ["QualityUnavailable", "Unavailable"],
    overmapped: ["QualityOvermapped", "Overmapped"],
  };
  const [key, label] = labels[token(status)] || labels.unavailable;
  return copy(key, label);
}

function levelLabel(level = {}) {
  const status = token(level.status) || "unavailable";
  const found = numericValue(level.found), total = numericValue(level.total);
  if (status === "partial" && total > 0 && found !== null && found >= 0 && found <= total) return percent(100 * found / total);
  return statusLabel(status);
}

function stageLabel(stage) {
  return copy(`Quality${stage.key[0].toUpperCase()}${stage.key.slice(1)}`, stage.label);
}

function contextLabel(explanation, viewState) {
  const { result, period, periodID, annual } = selectedContext(explanation, viewState);
  const zone = viewState.simulationEnergyScopeKind === "zone";
  const scope = zone ? `${copy("QualityZone", "Zone")} ${result.scope?.zoneName || viewState.simulationEnergyZoneName || ""}` : copy("QualityBuilding", "Building");
  const month = /^M([1-9]|1[0-2])$/i.exec(periodID);
  const periodLabel = annual ? t("simulation.periodAnnual", {}, "Annual") : month
    ? t(`simulation.month.${month[1]}`, {}, period?.label || periodID)
    : period?.label || periodID;
  return `${scope} · ${periodLabel} · ${zone ? copy("QualityModelContribution", "Model total contribution") : copy("QualityModelTotal", "Model total")}`;
}

function allocationLabel(quality) {
  const status = token(quality.zoneAllocationStatus);
  if (!["complete", "partial", "overmapped"].includes(status)) return statusLabel(status);
  const value = copy("QualityAssignedShares", "{assigned} direct/allocated · {unassigned} unassigned", {
    assigned: percent(quality.zoneAllocatedPct), unassigned: percent(quality.unassignedPct),
  });
  return status === "overmapped" ? `${value} · ${statusLabel(status)}` : value;
}

export function renderEnergyPathQualityLine(explanation = {}, viewState = {}) {
  const quality = energyPathQualityForState(explanation, viewState);
  const open = viewState.simulationEnergyDetailsOpen === true;
  return `<div class="energy-path-quality-line" data-energy-path-quality-line>
    <span class="energy-path-quality-context">${escapeHTML(contextLabel(explanation, viewState))}</span>
    <span class="energy-path-quality-stages">${STAGES.map((stage) => `<button type="button"
      data-energy-path-quality-stage="${stage.key}" data-energy-path-quality-status="${escapeHTML(token(quality[stage.key].status) || "unavailable")}" data-energy-path-quality-domain="${stage.domain}"
      aria-controls="energyPathDataDetails" aria-expanded="${open}" title="${escapeHTML(quality[stage.key].message || "")}">
      ${escapeHTML(stageLabel(stage))} ${escapeHTML(levelLabel(quality[stage.key]))}
    </button>`).join('<span aria-hidden="true"> · </span>')}</span>
    ${viewState.simulationEnergyScopeKind === "zone" ? `<span data-energy-path-zone-allocation-status="${escapeHTML(quality.zoneAllocationStatus)}" title="${escapeHTML(copy("QualityZoneCoverageNote", "Coverage of building-wide HVAC and auxiliary energy across all zones, not the selected zone's energy share."))}">${escapeHTML(copy("QualityZoneCoverage", "Building-wide zone coverage"))}: ${escapeHTML(allocationLabel(quality))}</span>` : ""}
    <button type="button" data-energy-path-details-toggle aria-controls="energyPathDataDetails" aria-expanded="${open}">${escapeHTML(copy("DataDetails", "Data details"))}</button>
  </div>`;
}

function sourceStageKeys(source = {}) {
  const level = token(source.level);
  if (["driver", "drivers", "heat", "heat_driver", "heat_drivers"].includes(level)) return ["drivers"];
  if (["load", "loads", "delivered_load"].includes(level) || token(source.driverCategory).startsWith("load.")) return ["loads"];
  if (source.driverCategory || source.driverRole) return ["drivers"];
  if (["carrier", "carriers"].includes(level)) return ["carriers"];
  if (["end_use", "end_uses"].includes(level)) return ["endUses"];
  if (["context", "support"].includes(level)) return [];
  const name = token(source.name || source.keyValue);
  if (name === "not requested by current output plan") return ["endUses", "carriers"];
  // Charge is consumption (canonical Other); only discharge is supply context.
  if (token(source.endUse) === "storage_charge" || name.replace(/[^a-z0-9]/g, "").includes("storagecharge")) return ["endUses"];
  if (/produced|purchased|surplussold|storage|generators/.test(name) || /(^|:)water($|:)/.test(name)) return [];
  if (name.includes(":facility") || name.startsWith("facility:")) return ["carriers"];
  return level === "energy" || source.isMeter ? ["endUses"] : [];
}

function sourceRoles(result = {}) {
  const roles = new Map();
  for (const node of [...(result.nodes || []), ...(result.periods || []).flatMap((period) => period.nodes || [])]) {
    const stage = { driver: "drivers", load: "loads", end_use: "endUses", carrier: "carriers" }[node.level];
    if (!stage) continue;
    for (const id of node.sourceIds || []) {
      const stages = roles.get(id) || new Set();
      stages.add(stage);
      roles.set(id, stages);
    }
  }
  return roles;
}

function rowMatchesStage(row, stage, roles, availability = false) {
  if (!stage) return true;
  const stages = sourceStageKeys(row);
  // Availability describes the requested output's stage, not every graph stage
  // that happens to cite the same observed source as a downstream input.
  if (availability && (token(row.level) || stages.length)) return stages.includes(stage);
  return stages.includes(stage) || (row.sourceIds || [row.id]).some((id) => roles.get(id)?.has(stage));
}

function scopedSources(sources, context, viewState) {
  if (viewState.simulationEnergyScopeKind !== "zone") return sources;
  const zone = token(viewState.simulationEnergyZoneName);
  const ids = new Set();
  const byID = new Map(sources.map((source) => [source.id, source]));
  const matchesScope = (source = {}) => token(source.zoneName || source.scope?.zoneName) === zone || (source.scopeDetails || []).some((detail) =>
    token(detail.scope?.kind) === "zone" && token(detail.scope?.zoneName) === zone);
  const graphs = [context.result, ...(context.result.periods || [])];
  for (const graph of graphs) {
    for (const record of [...(graph.nodes || []), ...(graph.links || []), ...(graph.reconciliation || [])]) {
      for (const id of record.sourceIds || []) ids.add(id);
    }
  }
  // Availability is run-level: its source IDs alone do not establish that an
  // observation from another explicit zone belongs to this selected zone.
  for (const row of context.sourceAvailability) {
    for (const id of row.sourceIds || []) {
      const source = byID.get(id) || {};
      if (!token(source.zoneName || source.scope?.zoneName) || matchesScope(source)) ids.add(id);
    }
  }
  for (const source of sources) {
    if (matchesScope(source)) ids.add(source.id);
  }
  const pending = [...ids];
  while (pending.length) {
    for (const id of byID.get(pending.pop())?.inputSourceIds || []) {
      if (!ids.has(id)) { ids.add(id); pending.push(id); }
    }
  }
  return sources.filter((source) => ids.has(source.id));
}

function resolutionLabel(status) {
  const labels = {
    ambiguous: ["OutputAmbiguous", "More than one matching request"],
    unavailable: ["OutputUnavailable", "No exact request in this run plan"],
    derived: ["OutputDerived", "Calculated from input sources"],
    tabular: ["OutputTabular", "Reported in an annual summary table"],
  };
  const [key, fallback] = labels[status] || labels.unavailable;
  return copy(key, fallback);
}

function renderOutputAction(source, outputObjects, sources) {
  const resolution = resolveEnergyPathOutputRequest(source, outputObjects, sources);
  if (resolution.status === "exact") return `<button type="button" data-energy-path-output-source="${escapeHTML(source.id || "")}">${escapeHTML(copy("OutputRequest", "Output request"))}</button>`;
  const inputs = (source.inputSourceIds || []).map((id) => sources.find((item) => item.id === id)).filter(Boolean);
  return `<span class="energy-path-output-resolution" data-energy-path-output-resolution="${escapeHTML(resolution.status)}">${escapeHTML(resolutionLabel(resolution.status))}</span>${inputs.map((input) => {
    const match = resolveEnergyPathOutputRequest(input, outputObjects, sources);
    return match.status === "exact" ? `<button type="button" data-energy-path-output-source="${escapeHTML(input.id)}">${escapeHTML(input.name || input.id)}</button>` : `<small>${escapeHTML(input.name || input.id)}</small>`;
  }).join("")}`;
}

function valueLabel(value, unit = "") {
  return numericValue(value) !== null ? `${Number(value).toLocaleString(undefined, { maximumFractionDigits: 3 })} ${unit}`.trim() : "—";
}

function renderAccountingQuality(quality) {
  const rows = [
    ["driverToLoad", copy("DriverClosure", "Driver → load closure"), quality.driverToLoadStatus, quality.driverToLoadClosedPct],
    ["endUseToCarrier", copy("CarrierClosure", "End use → source closure"), quality.endUseToCarrierStatus, quality.endUseToCarrierClosedPct],
  ];
  return `<section class="energy-path-details-accounting" data-energy-path-accounting-quality>
    <h5>${escapeHTML(copy("PeriodAccounting", "Selected-period accounting"))}</h5>
    <dl>${rows.map(([key, label, status, value]) => `<div data-energy-path-closure="${key}" data-energy-path-closure-status="${escapeHTML(status)}"><dt>${escapeHTML(label)}</dt><dd>${escapeHTML(["complete", "partial", "overmapped"].includes(token(status)) ? `${percent(value)} · ${statusLabel(status)}` : statusLabel(status))}</dd></div>`).join("")}
      <div data-energy-path-ratio-availability="${escapeHTML(token(quality.ratios.status))}"><dt>${escapeHTML(copy("RatioAvailability", "Conversion ratio availability"))}</dt><dd>${escapeHTML(levelLabel(quality.ratios))}${Number(quality.ratios.total) > 0 ? ` · ${escapeHTML(`${quality.ratios.found || 0}/${quality.ratios.total}`)}` : ""}</dd></div>
    </dl>
    <p>${escapeHTML(copy("RatioNotClosure", "Thermal loads and equipment energy use different scales. Conversion ratios are not a conservation check."))}</p>
  </section>`;
}

function renderExportSection(exportContext) {
  if (!exportContext?.sceneToken || typeof exportContext.runId !== "string") return "";
  return `<section class="energy-path-export" data-energy-path-export-section>
    <h5>${escapeHTML(copy("ExportTitle", "Export Energy Path"))}</h5>
    <p>${escapeHTML(copy("ExportSnapshot", "Exports the displayed run snapshot for {name}. Current editor changes are not included.", { name: exportContext.filename || exportContext.runId || copy("ExportNamedRun", "this run") }))}</p>
    <div class="energy-path-export-actions">${[
      ["html", "ExportHTML", "HTML report"], ["xlsx", "ExportXLSX", "XLSX report"], ["json", "ExportJSON", "Full-run JSON"],
    ].map(([format, key, label]) => `<button type="button" data-energy-path-export="${format}" data-energy-path-export-scene="${escapeHTML(exportContext.sceneToken)}" data-energy-path-export-run="${escapeHTML(exportContext.runId)}">${escapeHTML(copy(key, label))}</button>`).join("")}</div>
    <label class="energy-path-export-trace"><input type="checkbox" data-energy-path-export-trace> <span>${escapeHTML(copy("ExportTrace", "Include trace sheets (XLSX only)"))}</span></label>
  </section>`;
}

function renderDataPanel(explanation, viewState, outputObjects, diagnosticsHTML, drawer, exportContext) {
  const context = selectedContext(explanation, viewState);
  const stage = STAGES.some((item) => item.key === drawer.stage) ? drawer.stage : "";
  const roles = sourceRoles(context.result);
  const availability = context.sourceAvailability.filter((row) => rowMatchesStage(row, stage, roles, true));
  const sources = explanation.sources || [];
  const visibleSources = scopedSources(sources, context, viewState).filter((source) => rowMatchesStage(source, stage, roles));
  return `<section id="energyPathDataPanel" class="energy-path-details-panel" role="tabpanel" aria-labelledby="energyPathDataTab" data-energy-path-details-panel="data" ${drawer.tab === "output" ? "hidden" : ""}>
    <p>${escapeHTML(contextLabel(explanation, viewState))}</p>
    ${renderExportSection(exportContext)}
    ${renderAccountingQuality(energyPathQualityForState(explanation, viewState))}
    ${diagnosticsHTML || ""}
    <section data-energy-path-source-availability-section="${escapeHTML(stage || "all")}">
      <h5>${escapeHTML(copy("SourceAvailability", "Source availability"))}${stage ? ` · ${escapeHTML(stageLabel(STAGES.find((item) => item.key === stage)))}` : ""}</h5>
      ${stage ? `<button type="button" data-energy-path-quality-stage="">${escapeHTML(copy("AllStages", "All stages"))}</button>` : ""}
      <p>${escapeHTML(copy("RunAvailability", "Output availability applies to the whole run; accounting and ratios use the selected period."))}</p>
      <div class="energy-path-details-table-wrap"><table><thead><tr><th>${escapeHTML(copy("Source", "Source"))}</th><th>${escapeHTML(copy("QualityStatus", "Status"))}</th></tr></thead><tbody>
        ${availability.map((row) => `<tr data-energy-path-source-availability="${escapeHTML(row.name || "")}" data-energy-path-availability-status="${escapeHTML(row.status || "unavailable")}"><td>${escapeHTML(row.name || "—")}</td><td>${escapeHTML(statusLabel(row.status))}</td></tr>`).join("") || `<tr><td colspan="2">${escapeHTML(copy("NoAvailabilityRecords", "No source-availability records for this selection."))}</td></tr>`}
      </tbody></table></div>
    </section>
    <section data-energy-path-data-sources><h5>${escapeHTML(copy("InspectorSources", "Source details"))}</h5>
      <div class="energy-path-details-table-wrap"><table><thead><tr><th>${escapeHTML(copy("Source", "Source"))}</th><th>${escapeHTML(copy("RequestKey", "Key"))}</th><th>${escapeHTML(copy("RequestFrequency", "Frequency"))}</th><th>${escapeHTML(copy("OutputTab", "Output"))}</th></tr></thead><tbody>
        ${visibleSources.map((source) => `<tr data-energy-path-data-source="${escapeHTML(source.id || "")}"><td>${escapeHTML(source.name || source.id || "—")}<small>${escapeHTML(source.id || "")}</small></td><td>${escapeHTML(source.keyValue || "—")}</td><td>${escapeHTML(source.reportingFrequency || "—")}</td><td>${renderOutputAction(source, outputObjects, sources)}</td></tr>`).join("") || `<tr><td colspan="4">${escapeHTML(copy("NoSourceRecords", "No source records for this selection."))}</td></tr>`}
      </tbody></table></div>
    </section>
    <section data-energy-path-data-reconciliation><h5>${escapeHTML(copy("Reconciliation", "Reconciliation"))}</h5>
      <div class="energy-path-details-table-wrap"><table><thead><tr><th>${escapeHTML(copy("Reconciliation", "Reconciliation"))}</th><th>${escapeHTML(copy("CarrierTotal", "Total"))}</th><th>${escapeHTML(copy("ClassifiedEndUses", "Mapped"))}</th><th>${escapeHTML(copy("UnclassifiedResidual", "Residual"))}</th><th>${escapeHTML(copy("QualityStatus", "Status"))}</th></tr></thead><tbody>
        ${context.reconciliation.map((row) => `<tr data-energy-path-data-reconciliation-row="${escapeHTML(row.id || "")}"><td>${escapeHTML(row.label || row.id || "—")}<small>${escapeHTML(row.formula || row.basis || "")}</small></td><td>${escapeHTML(valueLabel(row.expectedValue, row.unit))}</td><td>${escapeHTML(valueLabel(row.explainedValue, row.unit))}</td><td>${escapeHTML(valueLabel(row.residualValue, row.unit))}</td><td>${escapeHTML(row.status || "—")}</td></tr>`).join("") || `<tr><td colspan="5">${escapeHTML(copy("NoReconciliation", "No reconciliation records for this period."))}</td></tr>`}
      </tbody></table></div>
    </section>
  </section>`;
}

function renderOutputPanel(explanation, drawer, outputObjects) {
  const sources = explanation.sources || [];
  const source = sources.find((item) => item.id === drawer.outputSource);
  const resolution = source ? resolveEnergyPathOutputRequest(source, outputObjects, sources) : null;
  return `<section id="energyPathOutputPanel" class="energy-path-details-panel" role="tabpanel" aria-labelledby="energyPathOutputTab" data-energy-path-details-panel="output" ${drawer.tab !== "output" ? "hidden" : ""}>
    <h5>${escapeHTML(copy("OutputRequests", "Run output requests"))}</h5>
    ${source ? `<p data-energy-path-output-match-status="${escapeHTML(resolution.status)}">${escapeHTML(source.name || source.id)} · ${escapeHTML(resolution.status === "exact" ? copy("ExactOutputRequest", "Exact request") : resolutionLabel(resolution.status))}</p>` : ""}
    <div class="energy-path-details-table-wrap"><table><thead><tr><th>${escapeHTML(copy("RequestType", "Type"))}</th><th>${escapeHTML(copy("RequestKey", "Key"))}</th><th>${escapeHTML(copy("RequestName", "Name"))}</th><th>${escapeHTML(copy("RequestFrequency", "Frequency"))}</th><th>${escapeHTML(copy("QualityStatus", "Status"))}</th></tr></thead><tbody>
      ${outputObjects.map((object, index) => {
        const selected = resolution?.status === "exact" && resolution.requestIndex === index;
        const fields = energyPathOutputRequestFields(object);
        return `<tr data-energy-path-output-request="${escapeHTML(energyPathOutputRequestKey(object, index))}" data-energy-path-output-request-selected="${selected}" aria-selected="${selected}" tabindex="-1">
          <td>${escapeHTML(fields.objectType || "—")}</td><td>${escapeHTML(fields.keyValue || "—")}</td><td>${escapeHTML(fields.variableName || object.description || object.signature || "—")}</td><td>${escapeHTML(fields.reportingFrequency || "—")}</td><td>${escapeHTML(object.state || "—")}</td>
        </tr>`;
      }).join("") || `<tr><td colspan="5">${escapeHTML(copy("NoOutputRequests", "No output requests are recorded for this run."))}</td></tr>`}
    </tbody></table></div>
  </section>`;
}

export function renderEnergyPathDataDetails(explanation = {}, viewState = {}, options = {}) {
  const open = viewState.simulationEnergyDetailsOpen === true;
  const drawer = options.drawer && typeof options.drawer === "object" ? options.drawer : {};
  const tab = drawer.tab === "output" ? "output" : "data";
  const outputObjects = options.outputObjects || [];
  return `<aside id="energyPathDataDetails" class="energy-path-data-details" data-energy-path-data-details ${open ? "" : "hidden"} tabindex="-1" role="dialog" aria-modal="false" aria-labelledby="energyPathDataDetailsTitle">
    <header><strong id="energyPathDataDetailsTitle">${escapeHTML(copy("DataDetails", "Data details"))}</strong><button type="button" data-energy-path-details-toggle>${escapeHTML(copy("CloseDetails", "Close"))}</button></header>
    <div class="energy-path-details-tabs" role="tablist" aria-label="${escapeHTML(copy("DataDetails", "Data details"))}">${["data", "output"].map((name) => `<button id="energyPath${name === "data" ? "Data" : "Output"}Tab" type="button" role="tab" tabindex="${tab === name ? 0 : -1}" aria-selected="${tab === name}" aria-controls="energyPath${name === "data" ? "Data" : "Output"}Panel" data-energy-path-details-tab="${name}">${escapeHTML(name === "data" ? copy("DataTab", "Data") : copy("OutputTab", "Output"))}</button>`).join("")}</div>
    ${renderDataPanel(explanation, viewState, outputObjects, options.diagnosticsHTML, drawer, options.exportContext)}
    ${renderOutputPanel(explanation, drawer, outputObjects)}
  </aside>`;
}
