import { t } from "./i18n.js";

export function isSimulationFailure(result) {
  return Boolean(result && (["failed", "missing_energyplus", "cancelled", "canceled"].includes(result.status)
    || result.status !== "succeeded" && result.error));
}

export function hasSimulationDiagnostics(result) {
  return isSimulationFailure(result) || Boolean(result?.status === "succeeded" && result.err?.severe > 0);
}

export function simulationResultSummary(result) {
  if (result?.status !== "succeeded") return simulationFailureSummary(result);
  const warnings = result.err?.warnings || 0, severe = result.err?.severe || 0;
  return severe > 0
    ? t("simulation.completeWithSevere", { warnings, severe }, "Simulation complete ({warnings} warnings · {severe} Severe)")
    : t("simulation.complete", { warnings }, `Simulation complete (${warnings} warnings)`);
}

const text = (value) => String(value ?? "").trim();
const firstLine = (value) => text(value).split(/\r?\n/).find((line) => line.trim())?.trim() || "";
const seriousIssues = (result) => (result?.err?.issues || []).filter((issue) => /^(severe|fatal)$/i.test(issue.severity || ""));
const isResultTransferFailure = (result) => String(result?.errorCode || "").startsWith("simulation_result_transfer_");

export function simulationFailureSummary(result) {
  if (["cancelled", "canceled"].includes(result?.status)) return t("simulation.runCancelled", {}, "Simulation cancelled");
  const issues = seriousIssues(result);
  const issue = issues.find((item) => /^severe$/i.test(item.severity)) || issues[0];
  let reason = firstLine(issue?.message || result?.error);
  if (!reason || /^exit status \d+$/i.test(reason)) {
    const logs = text(result?.stderr || result?.stdout).split(/\r?\n/).filter((line) => line.trim());
    reason = logs.find((line) => /\*\*\s*(severe|fatal)\s*\*\*/i.test(line))?.trim() || logs.at(-1)?.trim() || reason;
  }
  if (!reason) return t("simulation.finishedWithIssues", {}, "Simulation finished with issues");
  if (reason.length > 240) reason = `${reason.slice(0, 237)}…`;
  return isResultTransferFailure(result)
    ? t("simulation.resultTransfer.failureReason", { reason }, "Result loading failed: {reason}")
    : t("simulation.failureReason", { reason }, "Simulation failed: {reason}");
}

// Preserve native wording, paths and continuation lines. The disclosure and
// hover use the same evidence, including older runs with only exit status.
export function simulationDiagnosticDetails(result) {
  if (!hasSimulationDiagnostics(result)) return "";
  const details = [simulationResultSummary(result)];
  const field = (label, value) => { if (text(value)) details.push(`${label}: ${text(value)}`); };
  field(t("common.input", {}, "Input"), result.inputPath || result.filename);
  field("EnergyPlus", result.energyPlusVersion);
  field(t("simulation.executablePath", {}, "Executable path"), result.energyPlusExecutablePath);
  field(t("simulation.weather", {}, "Weather"), result.weatherPath);
  if (Number.isInteger(result.exitCode)) field(t("simulation.exitCode", {}, "Exit code"), result.exitCode);
  field(t("simulation.outputDirectory", {}, "Output directory"), result.outputDirectory);
  field("ERR", result.err?.path);
  if (text(result.error)) details.push(`\n${result.error}`);
  const issues = result.err?.issues || [];
  if (issues.length || result.err?.severe || result.err?.fatal) {
    details.push(`\n${t("simulation.failureCounts", { warnings: result.err?.warnings || 0, severe: result.err?.severe || 0, fatal: result.err?.fatal || 0 }, "Warnings {warnings} · Severe {severe} · Fatal {fatal}")}`);
    for (const issue of [...seriousIssues(result), ...issues.filter((issue) => !/^(severe|fatal)$/i.test(issue.severity || ""))]) {
      const line = Number.isInteger(issue.line) && issue.line > 0 ? ` · ${t("simulation.errLine", { line: issue.line }, "line {line}")}` : "";
      details.push(`[${text(issue.severity)}${line}] ${text(issue.message)}`);
    }
  } else if (!isResultTransferFailure(result) && !["cancelled", "canceled"].includes(result.status)) {
    details.push(`\n${t("simulation.failureNoERR", {}, "No Severe/Fatal ERR details were received. Check the process output and run paths below.")}`);
  }
  if (text(result.stderr)) details.push(`\nstderr\n${text(result.stderr)}`);
  if (text(result.stdout)) details.push(`\nstdout\n${text(result.stdout)}`);
  return details.join("\n");
}

export function simulationFailureDetails(result) {
  return isSimulationFailure(result) ? simulationDiagnosticDetails(result) : "";
}

export function simulationDiagnosticHover(result) {
  const details = simulationDiagnosticDetails(result);
  return details.length > 3500 ? `${details.slice(0, 3499)}…` : details;
}

export function simulationFailureHover(result) {
  const details = simulationFailureDetails(result);
  return details.length > 3500 ? `${details.slice(0, 3499)}…` : details;
}

export function renderSimulationDiagnosticDetails(result, escapeHTML) {
  const details = simulationDiagnosticDetails(result);
  const label = isSimulationFailure(result) ? t("simulation.failureDetails", {}, "Failure details")
    : t("simulation.diagnosticDetails", {}, "Run diagnostics");
  return details ? `<details class="simulation-failure-diagnostics"><summary>${escapeHTML(label)}</summary><pre tabindex="0">${escapeHTML(details)}</pre></details>` : "";
}

export function renderSimulationFailureDetails(result, escapeHTML) {
  const details = simulationFailureDetails(result);
  return details ? `<details class="simulation-failure-diagnostics"><summary>${escapeHTML(t("simulation.failureDetails", {}, "Failure details"))}</summary><pre tabindex="0">${escapeHTML(details)}</pre></details>` : "";
}
