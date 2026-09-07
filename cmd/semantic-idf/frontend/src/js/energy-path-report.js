import { energyPathSummaryForState, energyPathZoneDirectCoverage } from "./views/energy-path-view.js";
import { energyPathQualityForState } from "./energy-path-details.js";
import { energyPathCoverageBoundaries } from "./energy-path-kpis.js";
import { energyPathSummaryGroups, isEnergyPathSummaryV2 } from "./energy-path-summary.js";

export const ENERGY_PATH_REPORT_SCHEMA = "semantic-idf.energy-path-report/v2";
const token = (value) => String(value ?? "").trim().toLowerCase();
const numeric = (value) => (typeof value === "number" || typeof value === "string" && value.trim() !== "") && Number.isFinite(Number(value)) ? Number(value) : null;
const list = (value) => Array.isArray(value) ? value : [];
const clone = (value) => JSON.parse(JSON.stringify(value));
const escape = (value) => String(value ?? "").replace(/[&<>"']/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[character]);
const stages = ["drivers", "loads", "endUses", "carriers", "ratios", "residuals"];

function selectedCanonical(explanation, context) {
  let scoped = explanation;
  if (context.scopeKind === "zone") {
    const matches = list(explanation.zoneResults).filter((zone) => token(zone?.scope?.kind) === "zone" && token(zone.scope.zoneName) === token(context.zoneName));
    if (token(explanation.scope?.kind) === "zone" && token(explanation.scope.zoneName) === token(context.zoneName)) matches.push(explanation);
    if (matches.length !== 1) throw new Error("The selected Zone result is unavailable or ambiguous.");
    scoped = matches[0];
  } else if (token(scoped.scope?.kind) !== "building") {
    throw new Error("Building results are unavailable in this scoped snapshot.");
  }
  const periods = list(scoped.periods).filter((period) => token(period?.id) === token(context.period));
  if (periods.length > 1) throw new Error("The selected result period is ambiguous.");
  let selected = scoped;
  if (context.period !== "annual") {
    if (!periods.length) throw new Error("The selected month is unavailable; annual data cannot replace it.");
    selected = periods[0]; // An explicit empty month stays empty.
  } else if (!list(scoped.nodes).length && !list(scoped.links).length && periods.length) {
    selected = periods[0];
  }
  for (const item of [...list(selected.nodes), ...list(selected.links)]) {
    if (item.period && token(item.period) !== token(context.period)) throw new Error("A trace row contradicts the selected period.");
  }
  return { scoped, selected };
}

// A presentation eligibility test, not another balance calculation. It follows
// the canonical v2 facility/subtotal distinction used by the CLI quality export.
function hasReportedCarrierDenominator(nodes) {
  return nodes.some((node) => {
    const basis = token(node.basis), hierarchy = token(node.meterHierarchyLevel);
    const energyUnit = token(node.unit).replace(/[\s_()]/g, "");
    return node.level === "carrier" && node.scaleDomain === "site" && !node.zoneName &&
      numeric(node.value) > 0 && ["j", "kj", "mj", "gj", "wh", "kwh", "kwhsite", "mwh"].includes(energyUnit) &&
      !["reported_end_use_subtotal", "direct_zone_energy"].includes(basis) && !/allocat|load_share/.test(basis) &&
      !["observed_end_use_subtotal", "zone_direct_subtotal"].includes(hierarchy) &&
      (hierarchy === "facility_total" || ["reported_meter", "reported_variable", "integrated_rate"].includes(basis));
  });
}

function reportQualityRows(quality, summary, trace) {
  const rows = [];
  for (const [key, label] of [["drivers", "Drivers"], ["loads", "Loads"], ["endUses", "End uses"], ["carriers", "Carriers"], ["ratios", "Ratios"]]) {
    const level = quality[key] || {};
    const status = token(level.status) || "unavailable";
    const total = numeric(level.total), found = numeric(level.found);
    const known = !["unavailable", "not_requested", "not_applicable"].includes(status) && total > 0 && found !== null && found >= 0;
    rows.push({ key, label: `${label} ${key === "ratios" ? "conversion availability" : "output availability"}`, status,
      value: null, unit: "", found: known ? found : null, total: known ? total : null, message: String(level.message || "") });
  }
  const noFacilityDenominator = !hasReportedCarrierDenominator(trace.nodes);
  for (const boundary of energyPathCoverageBoundaries(quality, energyPathZoneDirectCoverage(summary || {}).limited || noFacilityDenominator)) {
    rows.push({ key: `${boundary.id}_closed_pct`, label: boundary.label, status: boundary.status, value: boundary.value,
      unit: "%", found: null, total: null,
      message: boundary.id === "end_use_to_carrier" && noFacilityDenominator
        ? "Facility denominator unavailable; observed or allocated subtotals are not measured facility totals." : "" });
  }
  for (const [key, field, label] of [["zone_allocated_pct", "zoneAllocatedPct", "Building-wide HVAC assigned to zones (direct + allocated)"], ["unassigned_pct", "unassignedPct", "Building-wide unassigned HVAC"]]) {
    const status = token(quality.zoneAllocationStatus) || "unavailable";
    const value = numeric(quality[field]);
    rows.push({ key, label, status, value: ["complete", "partial", "overmapped"].includes(status) && value !== null && value >= 0 ? value : null,
      unit: "%", found: null, total: null, message: "Coverage across all zones, not the selected zone's energy share." });
  }
  return rows;
}

/** Immutable report snapshot of the existing UI's all-service scope/period
 * summary. No model analysis, SQL read, allocation or graph layout occurs. */
export function buildEnergyPathReport(result = {}, viewState = {}) {
  const explanation = result.purposeResults?.energyExplanation;
  if (explanation?.schema !== "semantic-idf.energy-explanation/v2") return null;
  const scopeKind = token(viewState.simulationEnergyScopeKind) || "building";
  const zoneName = scopeKind === "zone" ? String(viewState.simulationEnergyZoneName || "").trim() : "";
  const rawPeriod = token(viewState.simulationEnergyPeriod) || "annual";
  const period = rawPeriod === "annual" ? "annual" : rawPeriod.toUpperCase();
  const service = token(viewState.simulationEnergyService) || "all";
  if (!["building", "zone"].includes(scopeKind) || scopeKind === "zone" && !zoneName ||
      !/^(annual|M[1-9]|M1[0-2])$/.test(period) || !["all", "cooling", "heating"].includes(service)) throw new Error("Invalid Energy Path export context.");
  const context = { runId: String(result.runId || ""), filename: String(result.filename || ""), inputPath: String(result.inputPath || ""),
    finishedAt: String(result.finishedAt || ""), scopeKind, zoneName, period, service,
    summaryService: "all", modelContext: "simulation_result_snapshot" };
  const { scoped, selected } = selectedCanonical(explanation, context);
  if (scopeKind === "zone") context.zoneName = scoped.scope.zoneName;
  const state = { ...viewState, simulationEnergyScopeKind: scopeKind, simulationEnergyZoneName: context.zoneName,
    simulationEnergyPeriod: period, simulationEnergyService: service };
  const candidate = energyPathSummaryForState(explanation, result.purposeResults.energyExplanationSummary || {}, state);
  const summary = isEnergyPathSummaryV2(candidate) ? clone(candidate) : null;
  const quality = clone(energyPathQualityForState(explanation, state));
  const trace = clone({ nodes: list(selected.nodes), links: list(selected.links), sources: list(explanation.sources),
    reconciliation: list(selected.reconciliation), warnings: list(selected.warnings) });
  return { schema: ENERGY_PATH_REPORT_SCHEMA, context, summary, quality,
    qualityRows: reportQualityRows(quality, summary, trace), trace, rawResult: JSON.stringify(result) };
}

const displayNumber = (value) => numeric(value) === null ? "—" : String(numeric(value));
const join = (value) => list(value).join(", ");
const table = (headers, rows, attribute = "") => `<div class="energy-report-table"><table ${attribute}><thead><tr>${headers.map((header) => `<th>${escape(header)}</th>`).join("")}</tr></thead><tbody>${rows.map((row) => `<tr>${row.map((cell) => `<td>${escape(cell)}</td>`).join("")}</tr>`).join("") || `<tr><td colspan="${headers.length}">No data available for this selection.</td></tr>`}</tbody></table></div>`;

function basisLabel(item) {
  const basis = String(item.basis || ""), normalized = token(basis);
  if (/unassigned/.test(normalized)) return `Unassigned · ${basis}`;
  if (/allocat|load_share/.test(normalized)) return `Allocated · ${basis}`;
  if (["direct_zone_energy", "reported_variable", "reported_meter", "integrated_rate"].includes(normalized)) return `Direct / reported · ${basis}`;
  return basis || "Unavailable";
}

/** A self-contained fragment; the existing purpose report keeps other purposes. */
export function renderEnergyPathReportHTML(report) {
  if (report?.schema !== ENERGY_PATH_REPORT_SCHEMA) return "";
  const context = report.context, trace = report.trace;
  const groups = energyPathSummaryGroups(report.summary || { schema: "semantic-idf.energy-explanation-summary/v2" }).filter((group) => stages.includes(group.key));
  const sourceRows = trace.sources.map((source) => [source.id, source.name || source.keyValue, source.sourceUnit,
    source.normalizedUnit, source.basis, source.sqlPath || source.sourcePath || "", source.tableName || "",
    JSON.stringify(source)]);
  const linkRows = trace.links.map((link) => [link.id, link.relation, link.fromId, displayNumber(link.fromValue), link.fromUnit,
    link.toId, displayNumber(link.toValue), link.toUnit, link.ratioKind ? displayNumber(link.ratio) : "—", link.ratioKind,
    link.basis, join(link.sourceIds), JSON.stringify(link)]);
  return `<section data-energy-path-report>
<style>
[data-energy-path-report]{color:#17202a;line-height:1.5}[data-energy-path-report] h2{font-size:22px;margin:0 0 12px}[data-energy-path-report] h3{font-size:16px;margin:22px 0 8px}.energy-report-context{padding:14px 16px;background:#edf4fb;border:1px solid #d5e1ed;border-radius:8px}.energy-report-context p{margin:5px 0}.energy-report-note{color:#526275;font-size:13px}.energy-report-table{overflow-x:auto;margin:8px 0 14px}.energy-report-table table{width:100%;border-collapse:collapse;background:white}.energy-report-table th,.energy-report-table td{padding:8px 10px;border:1px solid #dce3eb;text-align:left;vertical-align:top}.energy-report-table th{background:#f2f5f9;font-size:12px}.energy-report-trace{margin-top:20px;padding:12px;border:1px solid #d5e1ed;border-radius:8px}.energy-report-trace summary{cursor:pointer;font-weight:600}.energy-report-trace pre{white-space:pre-wrap;overflow-wrap:anywhere;max-height:420px;overflow:auto}.energy-report-trace td{max-width:420px;overflow-wrap:anywhere}@media print{.energy-report-table{overflow:visible}[data-energy-path-report] h3{break-after:avoid}.energy-report-table tr{break-inside:avoid}}
</style>
<h1>Energy Path</h1>
<div class="energy-report-context" data-energy-path-report-context>
<p><strong>${escape(context.scopeKind === "zone" ? `Zone · ${context.zoneName}` : "Building")} · ${escape(context.period)}</strong></p>
<p>Summary: all services · Graph selection: ${escape(context.service)}</p>
<p>Run: ${escape(context.filename || context.runId || "Unnamed result")} ${escape(context.finishedAt || "")}</p>
<p class="energy-report-note">Simulation result snapshot. Current editor changes are not included.</p>
<p class="energy-report-note">Drivers and loads are thermal energy; end uses and carriers are site energy. Do not add the two domains.</p>
${context.scopeKind === "zone" ? '<p class="energy-report-note">Direct observations and allocated HVAC contributions retain their basis. Unassigned building energy is not added to this Zone.</p>' : ""}
</div>
${report.summary ? "" : '<p role="status">Summary unavailable for this selection; no annual or Building values have been substituted.</p>'}
${groups.map((group) => `<section data-energy-path-report-stage="${group.key}"><h3>${escape(group.label)}${["drivers", "loads"].includes(group.key) ? " · thermal" : ["endUses", "carriers"].includes(group.key) ? " · site" : ""}</h3>${table(["Category", "Service", "Value", "Unit", "Basis"], group.items.map((item) => [item.label || "Unlabeled category", item.serviceKind || "", displayNumber(item.value), item.unit || "—", basisLabel(item)]))}</section>`).join("")}
<h3>Quality</h3>
${table(["Metric", "Value", "Unit", "Status", "Found", "Total", "Notes"], report.qualityRows.map((row) => [row.label, displayNumber(row.value), row.unit, row.status, displayNumber(row.found), displayNumber(row.total), row.message]), "data-energy-path-report-quality")}
<details class="energy-report-trace" data-energy-path-report-trace><summary>Trace · sources, links and original result</summary>
<p class="energy-report-note">Trace graph: selected scope/period, all services. Source dictionary: full-run metadata; annual source scalars are not monthly observations. Source correspondence is non-flow, not consumption.</p>
<h3>Source trace</h3>${table(["Source ID", "Output", "Source unit", "Normalized unit", "Basis", "SQL / file", "Table", "Original source JSON"], sourceRows)}
<h3>Link trace</h3>${table(["Link ID", "Relation", "From", "From value", "From unit", "To", "To value", "To unit", "Ratio", "Ratio kind", "Basis", "Sources", "Original link JSON"], linkRows)}
<details><summary>Node trace</summary><pre>${escape(JSON.stringify(trace.nodes, null, 2))}</pre></details>
<details><summary>Reconciliation and warnings</summary><pre>${escape(JSON.stringify({ reconciliation: trace.reconciliation, warnings: trace.warnings }, null, 2))}</pre></details>
<details><summary>Original simulation result JSON · full run</summary><pre>${escape(report.rawResult)}</pre></details>
</details></section>`;
}
