import { energyPathBatchComparison } from "./energy-path-batch-comparison.js";
import { isEnergyPathSummaryV2 } from "./energy-path-summary.js";

export const ENERGY_PATH_BATCH_EXPORT_SCHEMA = "semantic-idf.energy-path-batch-export/v2";

const rowID = (run) => run?.runId || run?.inputPath || run?.filename || "";
const pick = (object, keys) => Object.fromEntries(keys.filter((key) => Object.hasOwn(object, key)).map((key) => [key, object[key]]));
const hasV2 = (run) => isEnergyPathSummaryV2(run?.purposeResults?.energyExplanationSummary) ||
  isEnergyPathSummaryV2(run?.purposeResults?.energyExplanation?.summary) ||
  String(run?.purposeResults?.energyExplanation?.schema || "").toLowerCase() === "semantic-idf.energy-explanation/v2";

function sideSnapshot(side) {
  return {
    ...pick(side, ["value", "unit", "basis", "aggregationBasis", "missing", "ambiguous", "invalid", "unitValid", "sign", "scaleDomain", "unitIdentity"]),
    quality: pick(side.quality, ["status", "found", "total", "percent"]),
  };
}

function comparisonSnapshot(comparison) {
  return {
    status: comparison.status,
    reason: comparison.reason,
    rows: comparison.rows.map((row) => ({
      ...pick(row, ["key", "kind", "stage", "stageLabel", "category", "categoryId", "service", "unit", "deltaUnit",
        "delta", "deltaPercent", "basisMismatch", "coverageMismatch", "unitMismatch", "directlyComparable"]),
      baseline: sideSnapshot(row.baseline), target: sideSnapshot(row.target), warningCodes: [...row.warningCodes],
    })),
  };
}

/**
 * Snapshot the same nullable, semantic comparison used on screen for workbook
 * formatting. There is no graph aggregation or second export delta algorithm.
 * Result indexes bind summaries even when two runs have the same row ID; a
 * comparison still requires two distinct, uniquely identified submitted runs.
 */
export function energyPathBatchExport(result = {}, comparison = {}) {
  const runs = Array.isArray(result?.results) ? result.results : [];
  if (!runs.some(hasV2)) return null; // Existing non-Energy and genuine v1 export.
  const baselineRowId = typeof comparison?.baselineRowId === "string" ? comparison.baselineRowId : "";
  const targetRowId = typeof comparison?.targetRowId === "string" ? comparison.targetRowId : "";
  const baseline = runs.filter((run) => rowID(run) === baselineRowId);
  const target = runs.filter((run) => rowID(run) === targetRowId);
  let delta = { status: "unavailable", reason: "missing_comparison", rows: [] };
  if (baselineRowId && targetRowId) {
    if (baselineRowId === targetRowId) delta.reason = "same_comparison_run";
    else if (baseline.length > 1 || target.length > 1) delta.reason = "ambiguous_comparison";
    else if (!baseline.length || !target.length) delta.reason = "missing_comparison_run";
    else delta = comparisonSnapshot(energyPathBatchComparison(baseline[0], target[0]));
  }
  return {
    schema: ENERGY_PATH_BATCH_EXPORT_SCHEMA, scope: "building", period: "annual",
    runs: runs.map((run, resultIndex) => ({ resultIndex, rowId: rowID(run),
      ...comparisonSnapshot(energyPathBatchComparison(run, run)) })),
    comparison: { baselineRowId, targetRowId, ...delta },
  };
}
