import { energyPathSummaryGroups, isEnergyPathSummaryV2 } from "../energy-path-summary.js";
import { ENERGY_PATH_BATCH_STAGES, energyPathBatchSummary, energyPathBatchComparison } from "../energy-path-batch-comparison.js";
import { energyPathBatchExport } from "../energy-path-batch-export.js";
import { batchEnergyPathDetailAvailable, createBatchEnergyPathDetail } from "./batch-energy-path-detail.js";

export function initializeMultiSimulationTool(context) {
  const { state, elements, waitForAppAPI, waitForProgressRuntime, escapeHTML, postJSON, t, downloadCSV } = context;
  const detailHost = elements.multiSimulationEnergyPathDetail;
  const energyDetail = detailHost ? createBatchEnergyPathDetail({ host: detailHost, resolveRun: (id) => {
    const matches = (state.multiSimulation.result?.results || []).filter((item) => rowID(item) === id);
    return matches.length === 1 ? matches[0] : null;
  } }) : null;

  async function loadEnvironment() {
    try {
      const api = await waitForAppAPI("GetSimulationEnvironment");
      state.simulationEnvironment = api
        ? await api.GetSimulationEnvironment()
        : await fetch("/api/simulation-environment").then((response) => (response.ok ? response.json() : null));
      renderEnvironment();
    } catch {
      state.simulationEnvironment = null;
      renderEnvironment();
    }
  }

  function renderEnvironment() {
    const currentWeather = elements.multiSimulationWeather?.value || "";
    const weatherHTML = [`<option value="">${escapeHTML(t("simulation.noWeather", {}, "No weather / design-day only"))}</option>`];
    for (const folder of state.simulationEnvironment?.weatherFolders || []) {
      weatherHTML.push(`<optgroup label="${escapeHTML(`${folder.source || "Weather"} - ${folder.label || folder.path}`)}">`);
      for (const file of folder.files || []) {
        weatherHTML.push(`<option value="${escapeHTML(file.path)}" title="${escapeHTML(file.path)}">${escapeHTML(file.name)}</option>`);
      }
      weatherHTML.push("</optgroup>");
    }
    if (elements.multiSimulationWeather) {
      elements.multiSimulationWeather.innerHTML = weatherHTML.join("");
      if (currentWeather && [...elements.multiSimulationWeather.options].some((option) => option.value === currentWeather)) {
        elements.multiSimulationWeather.value = currentWeather;
      }
    }
    const defaultWorkers = state.simulationEnvironment?.defaultWorkerCount || 0;
    if (elements.multiSimulationWorkers && Number(elements.multiSimulationWorkers.value || 0) === 0 && defaultWorkers > 0) {
      elements.multiSimulationWorkers.placeholder = String(defaultWorkers);
    }
  }

  function setExportButtonsDisabled(disabled) {
    if (elements.multiSimulationExport) {
      elements.multiSimulationExport.disabled = disabled;
    }
    if (elements.multiSimulationExportXLSX) {
      elements.multiSimulationExportXLSX.disabled = disabled;
    }
    if (elements.multiSimulationExportJSON) {
      elements.multiSimulationExportJSON.disabled = disabled;
    }
  }

  async function selectFiles() {
    const api = await waitForAppAPI("SelectSimulationInputFiles");
    if (!api) {
      elements.multiSimulationStatus.textContent = t("tools.desktopOnly");
      return;
    }
    const result = await api.SelectSimulationInputFiles();
    if (!result || result.canceled) {
      elements.multiSimulationStatus.textContent = t("status.fileSelectionCanceled");
      return;
    }
    updateSelection(result.paths || [], result.rootDirectory || "");
  }

  async function selectFolder() {
    const api = await waitForAppAPI("SelectSimulationInputFolder");
    if (!api) {
      elements.multiSimulationStatus.textContent = t("tools.desktopOnly");
      return;
    }
    const recursive = Boolean(elements.multiSimulationRecursive?.checked);
    const result = await api.SelectSimulationInputFolder(recursive);
    if (!result || result.canceled) {
      elements.multiSimulationStatus.textContent = t("status.fileSelectionCanceled");
      return;
    }
    updateSelection(result.paths || [], result.rootDirectory || "");
  }

  function updateSelection(paths, rootDirectory = "") {
    state.multiSimulation.selectedPaths = [...new Set((paths || []).filter(Boolean))].sort();
    state.multiSimulation.rootDirectory = rootDirectory || "";
    state.multiSimulation.result = null;
    state.multiSimulation.selectedRows.clear();
    state.multiSimulation.metric = "";
    state.multiSimulation.compareBaselineId = "";
    state.multiSimulation.compareTargetId = "";
    elements.multiSimulationRun.disabled = !state.multiSimulation.selectedPaths.length || state.multiSimulation.running;
    setExportButtonsDisabled(true);
    elements.multiSimulationStats.textContent = t(
      "tools.simulationFilesSelected",
      { count: state.multiSimulation.selectedPaths.length },
      `${state.multiSimulation.selectedPaths.length} files selected`,
    );
    elements.multiSimulationStatus.textContent = t("tools.readyToRun", {}, "Ready to run");
    updateProgress(0, state.multiSimulation.selectedPaths.length, "", "idle");
    renderSelectedFiles();
    renderResult();
  }

  function renderSelectedFiles() {
    const paths = state.multiSimulation.selectedPaths || [];
    if (!paths.length) {
      elements.multiSimulationFiles.innerHTML = "";
      return;
    }
    elements.multiSimulationFiles.innerHTML = paths
      .slice(0, 80)
      .map(
        (path) => `
          <div class="tool-file-item">
            <strong>${escapeHTML(fileName(path))}</strong>
            <span title="${escapeHTML(path)}">${escapeHTML(path)}</span>
          </div>`,
      )
      .join("");
    if (paths.length > 80) {
      elements.multiSimulationFiles.insertAdjacentHTML(
        "beforeend",
        `<div class="tool-muted">${escapeHTML(t("tools.moreFiles", { count: paths.length - 80 }, `${paths.length - 80} more files`))}</div>`,
      );
    }
  }

  async function run() {
    const paths = state.multiSimulation.selectedPaths || [];
    if (!paths.length || state.multiSimulation.running) {
      return;
    }
    await loadEnvironment();
    state.multiSimulation.activeRunID = `multi-sim-${Date.now()}-${Math.random().toString(36).slice(2)}`;
    state.multiSimulation.running = true;
    elements.multiSimulationRun.disabled = true;
    elements.multiSimulationTable.innerHTML = `<div class="empty status-loading">${escapeHTML(t("tools.simulationRunning", {}, "EnergyPlus batch is running"))}</div>`;
    updateProgress(0, paths.length, t("tools.simulationRunning", {}, "EnergyPlus batch is running"), "running");
    waitForProgressRuntime();
    try {
      const request = {
        runId: state.multiSimulation.activeRunID,
        inputPaths: paths,
        rootDirectory: state.multiSimulation.rootDirectory || "",
        recursive: Boolean(elements.multiSimulationRecursive?.checked),
        energyPlusExecutablePath: "",
        weatherMode: elements.multiSimulationWeatherMode?.value || "same",
        weatherPath: elements.multiSimulationWeather?.value || "",
        workerCount: Number(elements.multiSimulationWorkers?.value || 0),
        purposeRequest: batchPurposeRequest(),
      };
      const result = await callRunAPI(request);
      state.multiSimulation.result = result;
      state.multiSimulation.selectedRows = new Set((result.results || []).filter((item) => item.status === "succeeded").slice(0, 12).map(rowID));
      state.multiSimulation.metric = firstMetric(result);
      normalizeEnergyCompareSelection(result, true);
      updateProgress(result.completed || 0, result.total || paths.length, t("tools.simulationComplete", {}, "Batch simulation complete"), "complete");
      renderResult();
    } catch (error) {
      elements.multiSimulationStatus.textContent = error?.message || String(error);
      elements.multiSimulationTable.innerHTML = `<div class="empty">${escapeHTML(error?.message || String(error))}</div>`;
    } finally {
      state.multiSimulation.running = false;
      elements.multiSimulationRun.disabled = !state.multiSimulation.selectedPaths.length;
    }
  }

  async function callRunAPI(request) {
    const api = await waitForAppAPI("RunMultipleSimulations");
    if (api) {
      return api.RunMultipleSimulations(request);
    }
    try {
      return await postJSON("/api/batch-simulation-run", request);
    } catch {
      return postJSON("/api/multi-simulation-run", request);
    }
  }

  function handleProgress(payload) {
    const progress = Array.isArray(payload) ? payload[0] : payload;
    if (!progress || progress.runId !== state.multiSimulation.activeRunID) {
      return;
    }
    updateProgress(progress.completed || 0, progress.total || 0, progress.message || "", progress.status || "running");
  }

  function updateProgress(completed, total, message = "", status = "running") {
    const percent = total > 0 ? Math.round((completed / total) * 100) : 0;
    if (elements.multiSimulationProgressBar) {
      elements.multiSimulationProgressBar.style.width = `${percent}%`;
    }
    if (elements.multiSimulationPercent) {
      elements.multiSimulationPercent.textContent = `${percent}%`;
    }
    if (elements.multiSimulationStatus) {
      elements.multiSimulationStatus.textContent = message || (total ? `${completed} / ${total}` : t("tools.waitingFiles"));
    }
    elements.multiSimulationStatus?.classList.toggle("status-loading", status === "running" && total > 0 && completed < total);
  }

  function renderResult() {
    energyDetail?.reset();
    const result = state.multiSimulation.result;
    if (!result) {
      setExportButtonsDisabled(true);
      elements.multiSimulationMetric.innerHTML = `<option value="">${escapeHTML(t("simulation.noSeries", {}, "No CSV series"))}</option>`;
      renderEnergyCompareSelects(null);
      elements.multiSimulationChart.innerHTML = `<div class="empty">${escapeHTML(t("tools.noSimulationResult", {}, "Run the selected files to compare simulation output."))}</div>`;
      elements.multiSimulationTable.innerHTML = state.multiSimulation.selectedPaths.length
        ? `<div class="empty">${escapeHTML(t("tools.readyToRun", {}, "Ready to run"))}</div>`
        : `<div class="empty">${escapeHTML(t("tools.selectSimulationFilesHelp", {}, "Select files or a folder to prepare batch simulation."))}</div>`;
      return;
    }
    const total = result.total || 0;
    const succeeded = result.succeeded || 0;
    const failed = result.failed || 0;
    setExportButtonsDisabled(!(result.results || []).length);
    elements.multiSimulationStats.textContent = t(
      "tools.simulationResultStats",
      { total, succeeded, failed, workers: result.workers || 0 },
      `${total} runs, ${succeeded} succeeded, ${failed} failed`,
    );
    renderMetricSelect(result);
    renderEnergyCompareSelects(result);
    renderChart(result);
    renderTable(result);
  }

  function renderMetricSelect(result) {
    const metrics = uniqueMetrics(result);
    if (!metrics.length) {
      state.multiSimulation.metric = "";
      elements.multiSimulationMetric.innerHTML = `<option value="">${escapeHTML(t("simulation.noSeries", {}, "No CSV series"))}</option>`;
      return;
    }
    if (!state.multiSimulation.metric || !metrics.includes(state.multiSimulation.metric)) {
      state.multiSimulation.metric = metrics[0];
    }
    elements.multiSimulationMetric.innerHTML = metrics
      .map((metric) => `<option value="${escapeHTML(metric)}" ${metric === state.multiSimulation.metric ? "selected" : ""}>${escapeHTML(metric)}</option>`)
      .join("");
  }

  function renderTable(result) {
    const rows = sortedResults(result.results || []);
    elements.multiSimulationTable.innerHTML = `
      <table class="tool-table">
        <thead>
          <tr>
            <th>${escapeHTML(t("common.view", {}, "View"))}</th>
            <th class="tool-sticky-col">${escapeHTML(t("common.name"))}</th>
            <th>${escapeHTML(t("common.status", {}, "Status"))}</th>
            <th>${escapeHTML(t("simulation.errWarnings", {}, "ERR warnings"))}</th>
            <th>${escapeHTML(t("simulation.errSevere", {}, "Severe/Fatal"))}</th>
            <th>${escapeHTML(t("batch.purposeMetric", {}, "Purpose metric"))}</th>
            <th>${escapeHTML(t("simulation.csvFiles", {}, "CSV files"))}</th>
            <th>${escapeHTML(t("tools.duration", {}, "Duration"))}</th>
            <th>${escapeHTML(t("simulation.weather", {}, "Weather"))}</th>
          </tr>
        </thead>
        <tbody>
          ${rows
            .map((item) => {
              const id = rowID(item);
              return `
                <tr>
                  <td><input data-multi-sim-row="${escapeHTML(id)}" type="checkbox" aria-label="${escapeHTML(t("batch.energyPathSelectRun", { name: item.filename || fileName(item.inputPath) }, `Select ${item.filename || fileName(item.inputPath)}`))}" ${state.multiSimulation.selectedRows.has(id) ? "checked" : ""} ${item.series?.length || item.purposeMetrics?.length || energyPathBatchSummary(item).status === "ready" || batchEnergyPathDetailAvailable(item) ? "" : "disabled"} />${renderEnergyPathOpenButton(item, "row")}</td>
                  <th class="tool-sticky-col">
                    <strong>${escapeHTML(item.filename || fileName(item.inputPath))}</strong>
                    <span title="${escapeHTML(item.outputDirectory || "")}">${escapeHTML(item.error || item.outputDirectory || "")}</span>
                  </th>
                  <td class="tool-value ${escapeHTML(item.status || "")}">${escapeHTML(item.status || "")}</td>
                  <td>${escapeHTML(item.err?.warnings || 0)}</td>
                  <td>${escapeHTML((item.err?.severe || 0) + (item.err?.fatal || 0))}</td>
                  <td>${escapeHTML(primaryPurposeMetric(item))}</td>
                  <td>${escapeHTML(item.csvs?.length || 0)}</td>
                  <td>${escapeHTML(formatDuration(item.durationMs || 0))}</td>
                  <td title="${escapeHTML(item.weatherPath || "")}">${escapeHTML(fileName(item.weatherPath) || t("common.notAvailable"))}</td>
                </tr>`;
            })
            .join("")}
        </tbody>
      </table>`;
    elements.multiSimulationTable.querySelectorAll("[data-multi-sim-row]").forEach((input) => {
      input.addEventListener("change", () => {
        if (input.checked) {
          state.multiSimulation.selectedRows.add(input.dataset.multiSimRow);
        } else {
          state.multiSimulation.selectedRows.delete(input.dataset.multiSimRow);
        }
        normalizeEnergyCompareSelection(result, true);
        renderEnergyCompareSelects(result);
        renderChart(result);
      });
    });
  }

  function renderChart(result) {
    const hasEnergyResults = (result.results || []).some(hasEnergyPayload);
    if (elements.multiSimulationMetric) elements.multiSimulationMetric.hidden = hasEnergyResults;
    if (hasEnergyResults) {
      elements.multiSimulationChart.innerHTML = renderEnergyExplanationBatchCompare(result);
      return;
    }
    if (uniquePurposeMetrics(result).length) {
      renderPurposeMetricChart(result);
      return;
    }
    const metric = state.multiSimulation.metric;
    const selected = (result.results || [])
      .filter((item) => state.multiSimulation.selectedRows.has(rowID(item)))
      .map((item) => ({ result: item, series: (item.series || []).find((series) => series.column === metric) }))
      .filter((item) => item.series?.points?.length)
      .slice(0, 20);
    if (!metric || !selected.length) {
      elements.multiSimulationChart.innerHTML = `<div class="empty">${escapeHTML(t("tools.selectMetricRows", {}, "Select a metric and result rows to overlay graph lines."))}</div>`;
      return;
    }
    const values = selected.flatMap((item) => item.series.points.map((point) => Number(point.value)).filter(Number.isFinite));
    const min = Math.min(...values);
    const max = Math.max(...values);
    const range = max - min || 1;
    const width = 900;
    const height = 280;
    const pad = { left: 76, right: 18, top: 24, bottom: 46 };
    const colors = ["#007c89", "#b3261e", "#246b44", "#a85f00", "#5b5fc7", "#8b5a2b", "#008a5c", "#c44569"];
    const yFor = (value) => pad.top + (height - pad.top - pad.bottom) * (1 - (value - min) / range);
    const yTicks = [max, min + range / 2, min]
      .map((value) => {
        const y = yFor(value);
        return `<g><line x1="${pad.left}" x2="${width - pad.right}" y1="${y}" y2="${y}" class="simulation-grid" /><text x="8" y="${y + 4}" class="simulation-axis">${escapeHTML(formatNumber(value))}</text></g>`;
      })
      .join("");
    const lines = selected
      .map((item, index) => {
        const points = item.series.points;
        const xStep = points.length > 1 ? (width - pad.left - pad.right) / (points.length - 1) : 1;
        const polyline = points.map((point, pointIndex) => `${pad.left + pointIndex * xStep},${yFor(Number(point.value))}`).join(" ");
        const color = colors[index % colors.length];
        return `<polyline points="${polyline}" fill="none" stroke="${color}" stroke-width="1.8" stroke-linejoin="round" />`;
      })
      .join("");
    const legend = selected
      .map((item, index) => {
        const x = pad.left + (index % 4) * 190;
        const y = height - 28 + Math.floor(index / 4) * 14;
        const color = colors[index % colors.length];
        return `<g><rect x="${x}" y="${y - 8}" width="9" height="9" fill="${color}" /><text x="${x + 14}" y="${y}" class="simulation-axis">${escapeHTML(item.result.filename || fileName(item.result.inputPath))}</text></g>`;
      })
      .join("");
    elements.multiSimulationChart.innerHTML = `
      <svg class="simulation-svg" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHTML(metric)}">
        ${yTicks}
        <line x1="${pad.left}" x2="${pad.left}" y1="${pad.top}" y2="${height - pad.bottom}" class="simulation-axis-line" />
        <line x1="${pad.left}" x2="${width - pad.right}" y1="${height - pad.bottom}" y2="${height - pad.bottom}" class="simulation-axis-line" />
        ${lines}
        <text x="${pad.left}" y="16" class="simulation-title">${escapeHTML(metric)} (${selected.length} selected, max 20)</text>
        ${legend}
      </svg>`;
  }

  function hasEnergyPayload(run = {}) {
    const explanation = run.purposeResults?.energyExplanation || {};
    const summary = run.purposeResults?.energyExplanationSummary || explanation.summary || {};
    if ([explanation.schema, summary.schema].some((schema) => typeof schema === "string" && schema.trim())) return true;
    const groups = ["drivers", "loads", "endUses", "carriers", "ratios", "residuals", "topZones", "energyByCarrier", "energyByEndUse", "deliveredLoadByService", "derivedKpis", "heatDrivers", "topHeatDrivers"];
    if (groups.some((key) => Array.isArray(summary[key]) && summary[key].length)) return true;
    const graphPresent = (graph) => ["nodes", "links", "edges"].some((key) => Array.isArray(graph?.[key]) && graph[key].length);
    return graphPresent(explanation) || (Array.isArray(explanation.periods) ? explanation.periods : []).some(graphPresent) ||
      (Array.isArray(explanation.zoneResults) ? explanation.zoneResults : []).some((zone) => graphPresent(zone) ||
        (Array.isArray(zone?.periods) ? zone.periods : []).some(graphPresent));
  }

  function renderPurposeMetricChart(result) {
    const metricID = state.multiSimulation.metric;
    const rows = (result.results || [])
      .filter((item) => state.multiSimulation.selectedRows.has(rowID(item)))
      .map((item) => ({ result: item, metric: (item.purposeMetrics || []).find((metric) => metric.id === metricID) }))
      .filter((item) => item.metric);
    if (!metricID || !rows.length) {
      elements.multiSimulationChart.innerHTML = `<div class="empty">${escapeHTML(t("tools.selectMetricRows", {}, "Select a purpose metric to compare."))}</div>`;
      return;
    }
    const values = rows.map((item) => Number(item.metric.value)).filter(Number.isFinite);
    const max = Math.max(...values.map((value) => Math.abs(value)), 1);
    elements.multiSimulationChart.innerHTML = `
      <div class="batch-purpose-bars">
        ${rows
          .slice(0, 20)
          .map((item) => {
            const value = Number(item.metric.value);
            const width = Number.isFinite(value) ? Math.max(2, (Math.abs(value) / max) * 100) : 0;
            return `
              <div class="batch-purpose-bar-row">
                <span>${escapeHTML(item.result.filename || fileName(item.result.inputPath))}</span>
                <div><i style="width:${width}%"></i></div>
                <strong>${escapeHTML(item.metric.displayValue || String(item.metric.value ?? ""))}</strong>
              </div>`;
          })
          .join("")}
      </div>`;
  }

  function exportMultiSimulationCSV() {
    const result = state.multiSimulation.result;
    if (!result || !(result.results || []).length || typeof downloadCSV !== "function") {
      return;
    }
    const rows = [[
      "file",
      "status",
      "run_id",
      "metric_type",
      "metric_id",
      "label",
      "value",
      "unit",
      "display_value",
      "level",
      "detail_status",
      "source_type",
      "source_key",
      "source_name",
      "source_frequency",
      "source_aggregation",
      "source_index_group",
      "source_object_index",
      "source_table",
      "source_row",
      "source_column",
      "period",
      "relation",
      "basis",
      "rule_id",
      "formula",
      "from_id",
      "to_id",
      "zone",
      "service_kind",
      "source_ids",
      "related_path_ids",
      "source_unit",
      "normalized_unit",
      "path_type",
      "heat_category",
      "sign",
      "numerator_label",
      "numerator_value",
      "numerator_unit",
      "denominator_label",
      "denominator_value",
      "denominator_unit",
    ]];
    (result.results || []).forEach((item) => {
      const file = item.filename || fileName(item.inputPath);
      const explanation = item.purposeResults?.energyExplanation || {};
      const explanationSummary = item.purposeResults?.energyExplanationSummary || {};
      for (const metric of item.purposeMetrics || []) {
        rows.push([
          file,
          item.status || "",
          item.runId || "",
          "purpose_metric",
          metric.id || "",
          metric.label || "",
          metric.value ?? "",
          metric.unit || "",
          metric.displayValue || "",
          metric.purposeId || "",
          metric.status || "",
          "",
          "",
          "",
          "",
          "",
          "",
          "",
          ...emptyEnergyExplanationSourceTableExportFields(),
          ...emptyEnergyExplanationEdgeExportFields(),
          ...emptyEnergyExplanationSourceUnitExportFields(),
          "",
          ...emptyEnergyExplanationHeatExportFields(),
          ...emptyEnergyExplanationRatioExportFields(),
        ]);
      }
      energyExplanationSummaryExportItems(explanationSummary).forEach((metric) => {
        rows.push([
          file,
          item.status || "",
          item.runId || "",
          metric.type,
          metric.id || "",
          metric.label || "",
          metric.value ?? "",
          metric.unit || "",
          energyExplanationSummaryAllocationDisplay(metric),
          metric.level || "",
          metric.status || "",
          "",
          "",
          "",
          "",
          metric.aggregationBasis || "",
          "",
          energyExplanationSourceObjectIndexes(explanation, metric.sourceIds || []),
          ...energyExplanationSourceTableExportFieldsForIDs(explanation, metric.sourceIds || []),
          ...energyExplanationSummaryEdgeExportFields(metric),
          ...energyExplanationSourceUnitExportFieldsForIDs(explanation, metric.sourceIds || []),
          metric.pathType || "",
          ...energyExplanationHeatExportFields(metric),
          ...energyExplanationRatioExportFields(metric),
        ]);
      });
      energyExplanationSourceExportItems(explanation).forEach((source) => {
        rows.push([
          file,
          item.status || "",
          item.runId || "",
          "energy_explanation.source",
          source.id || "",
          source.label || "",
          "",
          source.units || "",
          "",
          source.level || "",
          source.status || "",
          source.sourceType || "",
          source.keyValue || "",
          source.name || "",
          source.reportingFrequency || "",
          source.aggregationMethod || "",
          source.indexGroup || "",
          source.objectIndex ?? "",
          ...energyExplanationSourceTableExportFields(source),
          ...emptyEnergyExplanationEdgeExportFields(),
          ...energyExplanationSourceUnitExportFields(source),
          "",
          ...emptyEnergyExplanationHeatExportFields(),
          ...emptyEnergyExplanationRatioExportFields(),
        ]);
      });
      energyExplanationSourceAvailabilityExportItems(explanation).forEach((availability) => {
        rows.push([
          file,
          item.status || "",
          item.runId || "",
          "energy_explanation.source_availability",
          availability.id || "",
          availability.name || "",
          "",
          "",
          "",
          availability.level || "",
          availability.status || "",
          "source_availability",
          "",
          availability.name || "",
          "",
          "",
          "",
          energyExplanationSourceObjectIndexes(explanation, availability.sourceIds || []),
          ...energyExplanationSourceTableExportFieldsForIDs(explanation, availability.sourceIds || []),
          ...emptyEnergyExplanationEdgeExportFields(availability.sourceIds || []),
          ...energyExplanationSourceUnitExportFieldsForIDs(explanation, availability.sourceIds || []),
          "",
          ...emptyEnergyExplanationHeatExportFields(),
          ...emptyEnergyExplanationRatioExportFields(),
        ]);
      });
      energyExplanationNodeExportItems(explanation).forEach((node) => {
        const displayValue = Number(node.displayValue) || Number(node.value) || 0;
        rows.push([
          file,
          item.status || "",
          item.runId || "",
          "energy_explanation.node",
          node.id || "",
          node.label || "",
          node.value ?? "",
          node.unit || "",
          formatValue(displayValue, node.unit || ""),
          node.level || "",
          node.basis || "",
          "node",
          node.kind || "",
          node.label || "",
          "",
          "",
          "",
          energyExplanationSourceObjectIndexes(explanation, node.sourceIds || []),
          ...energyExplanationSourceTableExportFieldsForIDs(explanation, node.sourceIds || []),
          node.period || "",
          "node",
          node.basis || "",
          "",
          "",
          "",
          "",
          node.zoneName || "",
          node.serviceKind || "",
          (node.sourceIds || []).join("; "),
          (node.relatedPathIds || []).join("; "),
          ...energyExplanationSourceUnitExportFieldsForIDs(explanation, node.sourceIds || []),
          node.pathType || "",
          ...energyExplanationHeatExportFields(node),
          ...emptyEnergyExplanationRatioExportFields(),
        ]);
      });
      energyExplanationEdgeExportItems(explanation).forEach((edge) => {
        rows.push([
          file,
          item.status || "",
          item.runId || "",
          "energy_explanation.edge",
          edge.id || "",
          edge.label || "",
          edge.value ?? "",
          edge.unit || "",
          edge.displayValue ?? "",
          edge.relation || "",
          edge.basis || "",
          "edge",
          edge.fromId || "",
          edge.toId || "",
          edge.period || "",
          "",
          edge.ruleId || "",
          energyExplanationSourceObjectIndexes(explanation, edge.sourceIds || []),
          ...energyExplanationSourceTableExportFieldsForIDs(explanation, edge.sourceIds || []),
          edge.period || "",
          edge.relation || "",
          edge.basis || "",
          edge.ruleId || "",
          edge.formula || "",
          edge.fromId || "",
          edge.toId || "",
          edge.zoneName || "",
          edge.serviceKind || "",
          (edge.sourceIds || []).join("; "),
          (edge.relatedPathIds || []).join("; "),
          ...energyExplanationSourceUnitExportFieldsForIDs(explanation, edge.sourceIds || []),
          edge.pathType || "",
          ...emptyEnergyExplanationHeatExportFields(),
          ...emptyEnergyExplanationRatioExportFields(),
        ]);
      });
      energyExplanationReconciliationExportItems(explanation).forEach((reconciliation) => {
        rows.push([
          file,
          item.status || "",
          item.runId || "",
          "energy_explanation.reconciliation",
          reconciliation.id || "",
          reconciliation.label || "",
          reconciliation.residualValue ?? "",
          reconciliation.unit || "",
          formatSignedValue(reconciliation.residualValue, reconciliation.unit || ""),
          reconciliation.level || "",
          reconciliation.status || "",
          "",
          "",
          "",
          "",
          "",
          "",
          energyExplanationSourceObjectIndexes(explanation, reconciliation.sourceIds || []),
          ...energyExplanationSourceTableExportFieldsForIDs(explanation, reconciliation.sourceIds || []),
          reconciliation.period || "",
          "reconciliation",
          reconciliation.basis || "",
          "",
          reconciliation.formula || "",
          "",
          "",
          reconciliation.zoneName || "",
          reconciliation.serviceKind || "",
          (reconciliation.sourceIds || []).join("; "),
          "",
          ...energyExplanationSourceUnitExportFieldsForIDs(explanation, reconciliation.sourceIds || []),
          "",
          ...emptyEnergyExplanationHeatExportFields(),
          ...emptyEnergyExplanationRatioExportFields(),
        ]);
      });
      energyExplanationWarningExportItems(explanation).forEach((warning) => {
        rows.push([
          file,
          item.status || "",
          item.runId || "",
          "energy_explanation.warning",
          warning.code || "",
          warning.message || "",
          "",
          "",
          "",
          "",
          warning.severity || "",
          "",
          "",
          "",
          "",
          "",
          "",
          "",
          ...emptyEnergyExplanationSourceTableExportFields(),
          warning.period || "",
          "warning",
          "",
          "",
          "",
          "",
          "",
          "",
          "",
          "",
          "",
          ...emptyEnergyExplanationSourceUnitExportFields(),
          "",
          ...emptyEnergyExplanationHeatExportFields(),
          ...emptyEnergyExplanationRatioExportFields(),
        ]);
      });
    });
    downloadCSV(rows, "batch-simulation-purpose-results.csv");
    if (elements.multiSimulationStatus) {
      elements.multiSimulationStatus.textContent = t("status.exportedCsv", {}, "CSV exported");
    }
  }

  async function exportMultiSimulationXLSX() {
    const result = state.multiSimulation.result;
    if (!result || !(result.results || []).length) {
      return;
    }
    const api = await waitForAppAPI("SaveBatchSimulationXLSX");
    if (!api) {
      if (elements.multiSimulationStatus) {
        elements.multiSimulationStatus.textContent = t("tools.desktopOnly");
      }
      return;
    }
    if (elements.multiSimulationStatus) {
      elements.multiSimulationStatus.textContent = t("common.loadingSettings", {}, "Loading");
    }
    try {
      const exportContext = multiSimulationExportContext(result);
      const saved = await api.SaveBatchSimulationXLSX({
        result,
        context: exportContext,
        comparison: exportContext.comparison,
        includeTraceSheets: Boolean(elements.multiSimulationIncludeTraceSheets?.checked),
        energyPath: energyPathBatchExport(result, exportContext.comparison),
      });
      if (!saved?.canceled && elements.multiSimulationStatus) {
        elements.multiSimulationStatus.textContent = t(
          "status.savedNamed",
          { name: saved?.filename || "batch-simulation-purpose-results.xlsx" },
          `Saved ${saved?.filename || "batch-simulation-purpose-results.xlsx"}`,
        );
      }
    } catch (error) {
      if (elements.multiSimulationStatus) {
        elements.multiSimulationStatus.textContent = error?.message || String(error);
      }
    }
  }

  function exportMultiSimulationJSON() {
    const result = state.multiSimulation.result;
    if (!result || !(result.results || []).length) {
      return;
    }
    const payload = {
      schema: "semantic-idf.batch-simulation/v1",
      exportedAt: new Date().toISOString(),
      context: multiSimulationExportContext(result),
      result,
    };
    const blob = new Blob([`${JSON.stringify(payload, null, 2)}\n`], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "batch-simulation-purpose-results.json";
    link.click();
    URL.revokeObjectURL(url);
    if (elements.multiSimulationStatus) {
      elements.multiSimulationStatus.textContent = t("status.exportedJson", {}, "JSON exported");
    }
  }

  function multiSimulationComparisonContext(result) {
    if (result) {
      normalizeEnergyCompareSelection(result);
    }
    return {
      baselineRowId: state.multiSimulation.compareBaselineId || "",
      targetRowId: state.multiSimulation.compareTargetId || "",
    };
  }

  function multiSimulationExportContext(result) {
    return {
      selectedPaths: [...(state.multiSimulation.selectedPaths || [])],
      rootDirectory: state.multiSimulation.rootDirectory || "",
      selectedRowIds: Array.from(state.multiSimulation.selectedRows || []),
      metric: state.multiSimulation.metric || "",
      sort: state.multiSimulation.sort || "filename",
      viewMode: "purpose",
      weatherMode: elements.multiSimulationWeatherMode?.value || "same",
      weatherPath: elements.multiSimulationWeather?.value || "",
      workerCount: Number(elements.multiSimulationWorkers?.value || 0),
      purposeRequest: batchPurposeRequest(),
      comparison: multiSimulationComparisonContext(result),
    };
  }

  function energyExplanationSummaryExportItems(summary = {}) {
    const v2 = isEnergyPathSummaryV2(summary);
    const groups = energyPathSummaryGroups(summary).map((group) => [group.type, group.items]);
    return groups.flatMap(([type, items]) =>
      (items || []).map((item) => ({
        type,
        summarySchema: v2 ? "v2" : "v1",
        id: item.id || "",
        label: energyExplanationSummaryLabel(item),
        value: item.value,
        rawValue: item.rawValue,
        allocatedValue: item.allocatedValue,
        unit: item.unit || "",
        scaleDomain: item.scaleDomain || "",
        aggregationBasis: item.aggregationBasis || "",
        level: item.level || item.kind || "",
        pathType: item.pathType || "",
        heatCategory: item.heatCategory || "",
        sign: item.sign || "",
        serviceKind: item.serviceKind || "",
        basis: item.basis || "",
        formula: item.formula || "",
        numeratorLabel: item.numeratorLabel || "",
        numeratorValue: item.numeratorValue,
        numeratorUnit: item.numeratorUnit || "",
        denominatorLabel: item.denominatorLabel || "",
        denominatorValue: item.denominatorValue,
        denominatorUnit: item.denominatorUnit || "",
        status: summary.completeness?.status || "",
        sourceIds: item.sourceIds || [],
      })),
    );
  }

  function energyExplanationSummaryAllocationDisplay(metric = {}) {
    if (metric.summarySchema !== "v2") {
      return "";
    }
    const raw = Number(metric.rawValue);
    const allocated = Number(metric.allocatedValue);
    const parts = [];
    if (Number.isFinite(raw)) {
      parts.push(`raw=${formatValue(raw, metric.numeratorUnit || metric.unit || "")}`);
    }
    if (Number.isFinite(allocated)) {
      parts.push(`allocated=${formatValue(allocated, metric.denominatorUnit || metric.unit || "")}`);
    }
    return parts.join("; ");
  }

  function energyExplanationSummaryEdgeExportFields(metric = {}) {
    return [
      "",
      "",
      metric.basis || "",
      "",
      metric.formula || "",
      "",
      "",
      "",
      metric.serviceKind || "",
      (metric.sourceIds || []).join("; "),
      "",
    ];
  }

  function emptyEnergyExplanationEdgeExportFields(sourceIDs = [], relatedPathIDs = []) {
    return ["", "", "", "", "", "", "", "", "", (sourceIDs || []).join("; "), (relatedPathIDs || []).join("; ")];
  }

  function emptyEnergyExplanationSourceUnitExportFields() {
    return ["", ""];
  }

  function energyExplanationSourceTableExportFields(source = {}) {
    return [source.tableName || "", source.rowName || "", source.columnName || ""];
  }

  function energyExplanationSourceTableExportFieldsForIDs(explanation = {}, sourceIDs = []) {
    return [
      energyExplanationSourceValueSummary(explanation, sourceIDs, (source) => source.tableName),
      energyExplanationSourceValueSummary(explanation, sourceIDs, (source) => source.rowName),
      energyExplanationSourceValueSummary(explanation, sourceIDs, (source) => source.columnName),
    ];
  }

  function emptyEnergyExplanationSourceTableExportFields() {
    return ["", "", ""];
  }

  function emptyEnergyExplanationRatioExportFields() {
    return ["", "", "", "", "", ""];
  }

  function emptyEnergyExplanationHeatExportFields() {
    return ["", ""];
  }

  function energyExplanationHeatExportFields(metric = {}) {
    return [metric.heatCategory || "", metric.sign || ""];
  }

  function energyExplanationRatioExportFields(metric = {}) {
    return [
      metric.numeratorLabel || "",
      optionalEnergyExplanationNumber(metric.numeratorValue),
      metric.numeratorUnit || "",
      metric.denominatorLabel || "",
      optionalEnergyExplanationNumber(metric.denominatorValue),
      metric.denominatorUnit || "",
    ];
  }

  function optionalEnergyExplanationNumber(value) {
    const number = Number(value);
    return Number.isFinite(number) && number !== 0 ? number : "";
  }

  function energyExplanationSourceUnitExportFields(source = {}) {
    return [source.sourceUnit || source.units || "", source.normalizedUnit || ""];
  }

  function energyExplanationSourceUnitExportFieldsForIDs(explanation = {}, sourceIDs = []) {
    return [
      energyExplanationSourceValueSummary(explanation, sourceIDs, (source) => source.sourceUnit || source.units),
      energyExplanationSourceValueSummary(explanation, sourceIDs, (source) => source.normalizedUnit),
    ];
  }

  function energyExplanationSourceValueSummary(explanation = {}, sourceIDs = [], valueFn = () => "") {
    const sourceByID = new Map((explanation.sources || []).map((source) => [source.id, source]));
    const values = [];
    for (const sourceID of sourceIDs || []) {
      const source = sourceByID.get(sourceID);
      const value = String(valueFn(source || {}) || "").trim();
      if (value && !values.includes(value)) {
        values.push(value);
      }
    }
    return values.join("; ");
  }

  function energyExplanationSourceObjectIndexes(explanation = {}, sourceIDs = []) {
    const sourceByID = new Map((explanation.sources || []).map((source) => [source.id, source]));
    const indexes = [];
    for (const sourceID of sourceIDs || []) {
      const source = sourceByID.get(sourceID);
      const index = Number(source?.objectIndex);
      if (Number.isFinite(index) && !indexes.includes(index)) {
        indexes.push(index);
      }
    }
    return indexes.join("; ");
  }

  function energyExplanationEdgeExportItems(explanation = {}) {
    const periodGraphs = energyExplanationBatchExportPeriods(explanation).filter((period) => (period.edges || []).length);
    const graphs = periodGraphs.length
      ? periodGraphs
      : [{ id: "annual", label: "Annual", nodes: explanation.nodes || [], edges: explanation.edges || [] }];
    return graphs.flatMap((graph) => {
      const nodeLabels = new Map((graph.nodes || []).map((node) => [node.id, node.label || node.kind || node.id || ""]));
      return (graph.edges || []).map((edge) => ({
        ...edge,
        period: edge.period || graph.id || "",
        periodLabel: graph.label || graph.id || "",
        label: `${nodeLabels.get(edge.fromId) || edge.fromId || ""} -> ${nodeLabels.get(edge.toId) || edge.toId || ""}`,
      }));
    });
  }

  function energyExplanationNodeExportItems(explanation = {}) {
    const periodGraphs = energyExplanationBatchExportPeriods(explanation).filter((period) => (period.nodes || []).length);
    const graphs = periodGraphs.length
      ? periodGraphs
      : [{ id: "annual", label: "Annual", nodes: explanation.nodes || [] }];
    return graphs.flatMap((graph) =>
      (graph.nodes || []).map((node) => ({
        ...node,
        period: node.period || graph.id || "",
        periodLabel: graph.label || graph.id || "",
      })),
    );
  }

  function energyExplanationReconciliationExportItems(explanation = {}) {
    const periodGraphs = energyExplanationBatchExportPeriods(explanation).filter((period) => (period.reconciliation || []).length);
    const graphs = periodGraphs.length
      ? periodGraphs
      : [{ id: "annual", label: "Annual", reconciliation: explanation.reconciliation || [] }];
    return graphs.flatMap((graph) =>
      (graph.reconciliation || []).map((item) => ({
        ...item,
        period: item.period || graph.id || "",
        periodLabel: graph.label || graph.id || "",
      })),
    );
  }

  function energyExplanationWarningExportItems(explanation = {}) {
    const rows = [];
    const seen = new Set();
    const addWarning = (warning = {}, periodID = "") => {
      const period = warning.period || periodID || "";
      const key = [warning.severity || "", warning.code || "", period, warning.message || ""].join("\u0000");
      if ((!warning.code && !warning.message) || seen.has(key)) {
        return;
      }
      seen.add(key);
      rows.push({ ...warning, period });
    };
    (explanation.warnings || []).forEach((warning) => addWarning(warning, warning.period || "annual"));
    energyExplanationBatchExportPeriods(explanation).forEach((period) => {
      (period.warnings || []).forEach((warning) => addWarning(warning, warning.period || period.id || ""));
    });
    return rows;
  }

  function energyExplanationBatchExportPeriods(explanation = {}) {
    const periods = explanation.periods || [];
    if (!periods.length) {
      return [];
    }
    return periods.filter((period) => {
      const kind = String(period.kind || "").toLowerCase();
      const id = String(period.id || "").toLowerCase();
      return kind === "annual" || kind === "monthly" || kind === "selected_range" || id === "annual" || id === "selected_range";
    });
  }

  function energyExplanationSourceExportItems(explanation = {}) {
    const availability = new Map();
    for (const item of explanation.completeness?.sourceAvailability || []) {
      const key = `${String(item.level || "").toLowerCase()}|${String(item.name || "").toLowerCase()}`;
      availability.set(key, item.status || "");
    }
    return (explanation.sources || []).map((source) => {
      const level = source.isMeter ? "energy" : energyExplanationSourceLevel(source.name || "");
      const key = `${String(level || "").toLowerCase()}|${String(source.name || source.keyValue || "").toLowerCase()}`;
      return {
        ...source,
        level,
        label: source.keyValue && source.name ? `${source.keyValue} / ${source.name}` : source.keyValue || source.name || source.id || "",
        status: availability.get(key) || "found",
      };
    });
  }

  function energyExplanationSourceAvailabilityExportItems(explanation = {}) {
    return (explanation.completeness?.sourceAvailability || []).map((item) => ({
      id: [item.level || "", item.name || ""].filter(Boolean).join("|"),
      level: item.level || "",
      name: item.name || "",
      status: item.status || "",
      sourceIds: item.sourceIds || [],
    }));
  }

  function energyExplanationSourceLevel(name = "") {
    const normalized = String(name || "").toLowerCase();
    if (normalized.includes("heat balance") || normalized.includes("infiltration") || normalized.includes("ventilation") || normalized.includes("mixing")) {
      return "heat";
    }
    if (normalized.includes("cooling") || normalized.includes("heating") || normalized.includes("load") || normalized.includes("demand")) {
      return "load";
    }
    return "energy";
  }

  function renderEnergyExplanationBatchCompare(result) {
    const selected = selectedEnergyCompareResults(result);
    if (selected.length < 2) {
      return `<div class="empty" data-batch-energy-comparison>${escapeHTML(t("batch.needTwoEnergyCases", {}, "Need two Basic Energy results"))}</div>`;
    }
    const comparison = energyPathBatchComparison(selected[0], selected[1]);
    const unknown = t("common.notAvailable", {}, "—");
    const numeric = (value, unit = "", signed = false) => typeof value === "number" && Number.isFinite(value)
      ? `${signed && value > 0 ? "+" : ""}${formatNumber(value)}${unit ? ` ${unit}` : ""}` : unknown;
    const sideHTML = (side, row) => {
      const value = escapeHTML(numeric(side?.value, side?.unit || row.unit));
      if (row.kind !== "quality") {
        const status = side?.ambiguous ? "Ambiguous" : side?.missing ? "Missing" : side?.invalid ? "Invalid" : "";
        return status ? `${value}<small class="batch-energy-quality-status" data-batch-energy-value-status="${status.toLowerCase()}">${escapeHTML(t(`batch.energyPathValue${status}`, {}, status))}</small>` : value;
      }
      const statuses = { complete: ["Complete", "complete"], partial: ["Partial", "partial"],
        missing: ["Missing", "missing"], found: ["Found", "found"], overmapped: ["Overmapped", "Overmapped"],
        not_requested: ["NotRequested", "Not requested"], not_applicable: ["NotApplicable", "Not applicable"], unavailable: ["Unavailable", "Unavailable"] };
      const status = Object.hasOwn(statuses, side?.quality?.status) ? side.quality.status : "unavailable";
      const [key, fallback] = statuses[status];
      return `${value}<small class="batch-energy-quality-status" data-batch-energy-quality-status="${status}">${escapeHTML(t(`simulation.energyPathQuality${key}`, {}, fallback))}</small>`;
    };
    const stageLabel = (stage) => t(`batch.energyPathStage.${stage.key}`, {}, stage.label);
    const rows = ENERGY_PATH_BATCH_STAGES.map((stage) => {
      const stageRows = (comparison.rows || []).filter((row) => row.stage === stage.key);
      if (!stageRows.length) return `<tr data-batch-energy-row data-batch-energy-stage="${escapeHTML(stage.key)}"><th scope="row">${escapeHTML(stageLabel(stage))}</th><td>${escapeHTML(t("batch.energyPathNoCategory", {}, "No comparable annual category reported"))}</td><td>${unknown}</td><td>${unknown}</td><td>${unknown}</td><td>${unknown}</td></tr>`;
      return stageRows.map((row) => {
        const baseLabel = row.categoryLabel || row.category;
        const localizedBase = t(row.categoryKey || `batch.energyPathCategory.${baseLabel}`, {}, baseLabel);
        const service = ["drivers", "ratios"].includes(row.stage) && ["cooling", "heating"].includes(row.service)
          ? t(`simulation.${row.service}`, {}, row.service === "cooling" ? "Cooling" : "Heating") : "";
        const category = [localizedBase, service].filter(Boolean).join(" · ");
        const basis = row.basisMismatch ? `<small class="batch-energy-comparison-warning" data-batch-energy-basis-warning>${escapeHTML(t("batch.energyPathNotComparable", {}, "Not directly comparable"))}</small>` : "";
        const coverage = row.coverageMismatch ? `<small class="batch-energy-comparison-warning" data-batch-energy-coverage-warning>${escapeHTML(t("batch.energyPathCoverageDiffers", {}, "Source coverage differs"))}</small>` : "";
        const invalidDeltaKey = row.baseline?.ambiguous || row.target?.ambiguous ? "Ambiguous"
          : row.unitMismatch ? "Units" : row.baseline?.invalid || row.target?.invalid ? "Invalid"
            : row.baseline?.missing || row.target?.missing ? "Missing" : "Unavailable";
        const unavailableDelta = row.delta === null
          ? `<small class="batch-energy-quality-status" data-batch-energy-delta-unavailable="${invalidDeltaKey.toLowerCase()}">${escapeHTML(t(`batch.energyPathDelta${invalidDeltaKey}`, {}, "Comparable units or evidence are unavailable"))}</small>` : "";
        return `<tr data-batch-energy-row="${escapeHTML(row.key || "")}" data-batch-energy-stage="${escapeHTML(stage.key)}" data-batch-energy-comparable="${row.directlyComparable === true}">
          <th scope="row">${escapeHTML(stageLabel(stage))}</th><td>${escapeHTML(category)}${basis}</td>
          <td>${sideHTML(row.baseline, row)}</td><td>${sideHTML(row.target, row)}</td>
          <td>${escapeHTML(numeric(row.delta, row.deltaUnit || row.unit, true))}${coverage}${unavailableDelta}</td><td>${escapeHTML(numeric(row.deltaPercent, "%", true))}</td>
        </tr>`;
      }).join("");
    }).join("");
    return `<section class="batch-energy-explanation-compare" data-batch-energy-comparison>
      <header><h4>${escapeHTML(t("batch.energyPathAnnualComparison", {}, "Energy Path comparison"))}</h4><span class="tool-muted" data-batch-energy-context>${escapeHTML(t("batch.energyPathFixedContext", {}, "Building · Annual"))}</span></header>
      ${renderEnergyComparePair(selected[0], selected[1])}
      ${comparison.status !== "ready" ? `<p class="tool-muted">${escapeHTML(t("batch.energyPathSummaryUnavailable", {}, "A valid Building/Annual Energy Path summary is unavailable for one or both runs."))}</p>` : ""}
      <div class="tool-table-wrap"><table class="tool-table batch-energy-summary-table" data-batch-energy-summary-table>
        <thead><tr>${[t("batch.energyPathStage", {}, "Stage"), t("common.category", {}, "Category"), t("batch.baselineCase", {}, "Baseline"), t("batch.targetCase", {}, "Target"), t("batch.energyPathDelta", {}, "Delta"), t("batch.energyPathDeltaPercent", {}, "Delta %")].map((label) => `<th scope="col">${escapeHTML(label)}</th>`).join("")}</tr></thead><tbody>${rows}</tbody>
      </table></div>
    </section>`;
  }

  function renderEnergyPathOpenButton(run, origin) {
    const unique = Boolean(rowID(run)) && (state.multiSimulation.result?.results || []).filter((item) => rowID(item) === rowID(run)).length === 1;
    const available = unique && Boolean(energyDetail) && batchEnergyPathDetailAvailable(run);
    const reason = !unique ? t("batch.energyPathRunAmbiguous", {}, "This run cannot be identified uniquely.") : t("batch.energyPathDetailUnavailable", {}, "The annual Energy Path graph is not included in this run.");
    return `<button type="button" class="batch-energy-open" data-batch-energy-open="${escapeHTML(rowID(run))}" data-batch-energy-open-origin="${origin}" ${available ? "" : "disabled"} title="${escapeHTML(available ? t("batch.energyPathOpen", {}, "Open Energy Path") : reason)}">${escapeHTML(t("batch.energyPathOpen", {}, "Open Energy Path"))}</button>`;
  }

  function renderEnergyComparePair(leftResult, rightResult) {
    return `
      <div class="batch-energy-compare-pair">
        <div><span>${escapeHTML(t("batch.baselineCase", {}, "Baseline"))}</span><strong>${escapeHTML(leftResult.filename || fileName(leftResult.inputPath))}</strong>${renderEnergyPathOpenButton(leftResult, "baseline")}</div>
        <div><span>${escapeHTML(t("batch.targetCase", {}, "Target"))}</span><strong>${escapeHTML(rightResult.filename || fileName(rightResult.inputPath))}</strong>${renderEnergyPathOpenButton(rightResult, "target")}</div>
      </div>`;
  }

  function renderEnergyCompareSelects(result) {
    if (!elements.multiSimulationCompareBaseline || !elements.multiSimulationCompareTarget) {
      return;
    }
    const candidates = energyCompareCandidates(result);
    if (candidates.length < 2) {
      const empty = `<option value="">${escapeHTML(t("batch.needTwoEnergyCases", {}, "Need two Basic Energy results"))}</option>`;
      elements.multiSimulationCompareBaseline.innerHTML = empty;
      elements.multiSimulationCompareTarget.innerHTML = empty;
      elements.multiSimulationCompareBaseline.disabled = true;
      elements.multiSimulationCompareTarget.disabled = true;
      state.multiSimulation.compareBaselineId = "";
      state.multiSimulation.compareTargetId = "";
      return;
    }
    normalizeEnergyCompareSelection(result);
    const options = candidates
      .map((item) => {
        const id = rowID(item);
        const label = item.filename || fileName(item.inputPath) || id;
        return `<option value="${escapeHTML(id)}">${escapeHTML(label)}</option>`;
      })
      .join("");
    elements.multiSimulationCompareBaseline.innerHTML = options;
    elements.multiSimulationCompareTarget.innerHTML = options;
    elements.multiSimulationCompareBaseline.value = state.multiSimulation.compareBaselineId || "";
    elements.multiSimulationCompareTarget.value = state.multiSimulation.compareTargetId || "";
    elements.multiSimulationCompareBaseline.disabled = false;
    elements.multiSimulationCompareTarget.disabled = false;
  }

  function selectedEnergyCompareResults(result) {
    normalizeEnergyCompareSelection(result);
    const candidates = energyCompareCandidates(result);
    const byID = new Map(candidates.map((item) => [rowID(item), item]));
    const left = byID.get(state.multiSimulation.compareBaselineId || "");
    const right = byID.get(state.multiSimulation.compareTargetId || "");
    if (left && right && rowID(left) !== rowID(right)) {
      return [left, right];
    }
    return candidates.filter((item) => state.multiSimulation.selectedRows.has(rowID(item))).slice(0, 2);
  }

  function energyCompareCandidates(result) {
    const counts = new Map();
    for (const item of result?.results || []) counts.set(rowID(item), (counts.get(rowID(item)) || 0) + 1);
    return (result?.results || []).filter((item) => rowID(item) && counts.get(rowID(item)) === 1 &&
      (energyPathBatchSummary(item).status === "ready" || batchEnergyPathDetailAvailable(item)));
  }

  function normalizeEnergyCompareSelection(result, preferCheckedRows = false) {
    const candidates = energyCompareCandidates(result);
    const candidateIDs = candidates.map(rowID);
    if (candidateIDs.length < 2) {
      state.multiSimulation.compareBaselineId = "";
      state.multiSimulation.compareTargetId = "";
      return;
    }
    const checkedIDs = candidates.filter((item) => state.multiSimulation.selectedRows.has(rowID(item))).map(rowID);
    if (preferCheckedRows && checkedIDs.length >= 2) {
      state.multiSimulation.compareBaselineId = checkedIDs[0];
      state.multiSimulation.compareTargetId = checkedIDs.find((id) => id !== checkedIDs[0]) || checkedIDs[1];
      return;
    }
    if (!candidateIDs.includes(state.multiSimulation.compareBaselineId || "")) {
      state.multiSimulation.compareBaselineId = checkedIDs[0] || candidateIDs[0];
    }
    if (
      !candidateIDs.includes(state.multiSimulation.compareTargetId || "") ||
      state.multiSimulation.compareTargetId === state.multiSimulation.compareBaselineId
    ) {
      state.multiSimulation.compareTargetId =
        checkedIDs.find((id) => id !== state.multiSimulation.compareBaselineId) ||
        candidateIDs.find((id) => id !== state.multiSimulation.compareBaselineId) ||
        "";
    }
  }

  function handleEnergyCompareSelectChange(changed) {
    const result = state.multiSimulation.result;
    if (!result) {
      return;
    }
    state.multiSimulation.compareBaselineId = elements.multiSimulationCompareBaseline?.value || "";
    state.multiSimulation.compareTargetId = elements.multiSimulationCompareTarget?.value || "";
    if (state.multiSimulation.compareBaselineId === state.multiSimulation.compareTargetId) {
      const candidateIDs = energyCompareCandidates(result).map(rowID);
      const replacement = candidateIDs.find((id) => id !== state.multiSimulation.compareBaselineId) || "";
      if (changed === "baseline") {
        state.multiSimulation.compareTargetId = replacement;
      } else {
        state.multiSimulation.compareBaselineId = replacement;
      }
    }
    if (state.multiSimulation.compareBaselineId) {
      state.multiSimulation.selectedRows.add(state.multiSimulation.compareBaselineId);
    }
    if (state.multiSimulation.compareTargetId) {
      state.multiSimulation.selectedRows.add(state.multiSimulation.compareTargetId);
    }
    renderEnergyCompareSelects(result);
    renderChart(result);
    renderTable(result);
  }

  function energyExplanationSummaryComparisonGroups(leftResult = {}, rightResult = {}) {
    const leftSummary = leftResult.purposeResults?.energyExplanationSummary || {};
    const rightSummary = rightResult.purposeResults?.energyExplanationSummary || {};
    const v2Summary = isEnergyPathSummaryV2(leftSummary)
      ? leftSummary
      : isEnergyPathSummaryV2(rightSummary)
        ? rightSummary
        : null;
    return energyPathSummaryGroups(v2Summary || leftSummary, { comparison: true })
      .map((group) => [group.label, group.key]);
  }

  function energyExplanationDeltaRows(group, leftResult, rightResult, key) {
    const left = energyExplanationSummaryMap(leftResult.purposeResults?.energyExplanationSummary?.[key] || []);
    const right = energyExplanationSummaryMap(rightResult.purposeResults?.energyExplanationSummary?.[key] || []);
    const leftExplanation = leftResult.purposeResults?.energyExplanation || {};
    const rightExplanation = rightResult.purposeResults?.energyExplanation || {};
    return [...new Set([...left.keys(), ...right.keys()])].map((id) => {
      const leftItem = left.get(id);
      const rightItem = right.get(id);
      const leftValue = energyExplanationComparisonValue(leftItem);
      const rightValue = energyExplanationComparisonValue(rightItem);
      const unit = rightItem?.unit || leftItem?.unit || "";
      const delta = rightValue - leftValue;
      const percent = leftValue === 0 ? null : (delta / leftValue) * 100;
      return {
        group,
        id,
        label: energyExplanationSummaryLabel(leftItem || rightItem),
        leftValue,
        rightValue,
        leftMissing: !leftItem,
        rightMissing: !rightItem,
        delta,
        percent,
        unit,
        formula: rightItem?.formula || leftItem?.formula || "",
        numeratorLabel: rightItem?.numeratorLabel || leftItem?.numeratorLabel || "",
        denominatorLabel: rightItem?.denominatorLabel || leftItem?.denominatorLabel || "",
        leftNumeratorValue: energyExplanationComparisonValue(leftItem, "numeratorValue"),
        rightNumeratorValue: energyExplanationComparisonValue(rightItem, "numeratorValue"),
        numeratorUnit: rightItem?.numeratorUnit || leftItem?.numeratorUnit || "",
        leftDenominatorValue: energyExplanationComparisonValue(leftItem, "denominatorValue"),
        rightDenominatorValue: energyExplanationComparisonValue(rightItem, "denominatorValue"),
        denominatorUnit: rightItem?.denominatorUnit || leftItem?.denominatorUnit || "",
        heatCategory: rightItem?.heatCategory || leftItem?.heatCategory || "",
        sign: rightItem?.sign || leftItem?.sign || "",
        leftSourceIDs: leftItem?.sourceIds || [],
        rightSourceIDs: rightItem?.sourceIds || [],
        leftSourceSummary: energyExplanationDeltaSourceSummary(leftExplanation, leftItem?.sourceIds || []),
        rightSourceSummary: energyExplanationDeltaSourceSummary(rightExplanation, rightItem?.sourceIds || []),
        status: energyExplanationDeltaStatus(leftItem, rightItem, leftValue, rightValue),
        totalMagnitude: Math.abs(leftValue) + Math.abs(rightValue),
      };
    });
  }

  function energyExplanationEdgeDeltaRows(leftResult, rightResult) {
    const leftExplanation = leftResult.purposeResults?.energyExplanation || {};
    const rightExplanation = rightResult.purposeResults?.energyExplanation || {};
    const left = energyExplanationEdgeMap(leftExplanation);
    const right = energyExplanationEdgeMap(rightExplanation);
    return [...new Set([...left.keys(), ...right.keys()])].map((id) => {
      const leftEdge = left.get(id);
      const rightEdge = right.get(id);
      const leftValue = energyExplanationComparisonValue(leftEdge);
      const rightValue = energyExplanationComparisonValue(rightEdge);
      const unit = rightEdge?.unit || leftEdge?.unit || "";
      const delta = rightValue - leftValue;
      const percent = leftValue === 0 ? null : (delta / leftValue) * 100;
      return {
        id,
        label: rightEdge?.label || leftEdge?.label || id,
        relation: rightEdge?.relation || leftEdge?.relation || "",
        basis: rightEdge?.basis || leftEdge?.basis || "",
        ruleId: rightEdge?.ruleId || leftEdge?.ruleId || "",
        fromId: rightEdge?.fromId || leftEdge?.fromId || "",
        toId: rightEdge?.toId || leftEdge?.toId || "",
        leftValue,
        rightValue,
        leftMissing: !leftEdge,
        rightMissing: !rightEdge,
        delta,
        percent,
        unit,
        leftSourceIDs: leftEdge?.sourceIds || [],
        rightSourceIDs: rightEdge?.sourceIds || [],
        leftSourceSummary: energyExplanationDeltaSourceSummary(leftExplanation, leftEdge?.sourceIds || []),
        rightSourceSummary: energyExplanationDeltaSourceSummary(rightExplanation, rightEdge?.sourceIds || []),
        status: energyExplanationDeltaStatus(leftEdge, rightEdge, leftValue, rightValue),
      };
    });
  }

  function energyExplanationEdgeMap(explanation = {}) {
    const out = new Map();
    energyExplanationAnnualEdgeItems(explanation).forEach((edge) => {
      const id = edge.id || [edge.relation, edge.ruleId, edge.fromId, edge.toId].filter(Boolean).join("|");
      if (id) {
        out.set(id, edge);
      }
    });
    return out;
  }

  function energyExplanationAnnualEdgeItems(explanation = {}) {
    const annual = (explanation.periods || []).find((period) => period.id === "annual" || period.kind === "annual");
    const graph = annual || { id: "annual", label: "Annual", nodes: explanation.nodes || [], edges: explanation.edges || [] };
    const nodeLabels = new Map((graph.nodes || []).map((node) => [node.id, node.label || node.kind || node.id || ""]));
    return (graph.edges || []).map((edge) => ({
      ...edge,
      period: edge.period || graph.id || "annual",
      label: `${nodeLabels.get(edge.fromId) || edge.fromId || ""} -> ${nodeLabels.get(edge.toId) || edge.toId || ""}`,
    }));
  }

  function energyExplanationSummaryMap(items = []) {
    const out = new Map();
    items.forEach((item) => {
      if (item?.id) {
        out.set(item.id, item);
      }
    });
    return out;
  }

  function energyExplanationSummaryLabel(item = {}) {
    return item.label || item.id || "";
  }

  function energyExplanationDeltaSourceSummary(explanation = {}, sourceIDs = []) {
    const sourceByID = new Map((explanation.sources || []).map((source) => [source.id, source]));
    const values = [];
    for (const sourceID of sourceIDs || []) {
      if (!sourceID || values.some((value) => value.sourceID === sourceID)) {
        continue;
      }
      const source = sourceByID.get(sourceID) || {};
      const objectIndex = Number(source.objectIndex);
      const outputObject = Number.isFinite(objectIndex) ? `#${objectIndex + 1}` : "";
      const tableRef = [source.tableName, source.rowName, source.columnName].filter(Boolean).join(" / ");
      const unitRef = [source.sourceUnit || source.units, source.normalizedUnit].filter(Boolean).join(" -> ");
      values.push({
        sourceID,
        label: [source.id || sourceID, outputObject, tableRef, unitRef].filter(Boolean).join(" | "),
      });
    }
    const labels = values.map((value) => value.label).filter(Boolean);
    if (labels.length <= 2) {
      return labels.join("; ");
    }
    return `${labels.slice(0, 2).join("; ")}; +${labels.length - 2}`;
  }

  function energyExplanationComparisonValue(item, field = "value") {
    if (!item) {
      return 0;
    }
    const number = Number(item[field]);
    return Number.isFinite(number) ? number : 0;
  }

  function energyExplanationDeltaStatus(leftItem, rightItem, leftValue = 0, rightValue = 0) {
    if (!leftItem && rightItem) {
      return "missing in baseline";
    }
    if (leftItem && !rightItem) {
      return "missing in comparison";
    }
    if (leftValue === 0 && rightValue !== 0) {
      return "zero baseline";
    }
    if (leftValue !== 0 && rightValue === 0) {
      return "zero comparison";
    }
    return "matched";
  }

  function formatValue(value, unit = "") {
    return `${formatNumber(value)}${unit ? ` ${unit}` : ""}`;
  }

  function formatSignedValue(value, unit = "") {
    const number = Number(value);
    if (!Number.isFinite(number)) {
      return t("common.notAvailable", {}, "—");
    }
    const sign = number > 0 ? "+" : "";
    return `${sign}${formatValue(number, unit)}`;
  }

  function sortedResults(results) {
    const rows = results.slice();
    const key = state.multiSimulation.sort || "filename";
    rows.sort((a, b) => {
      if (key === "warnings") {
        return (b.err?.warnings || 0) - (a.err?.warnings || 0);
      }
      if (key === "severe") {
        return (b.err?.severe || 0) + (b.err?.fatal || 0) - ((a.err?.severe || 0) + (a.err?.fatal || 0));
      }
      if (key === "duration") {
        return (b.durationMs || 0) - (a.durationMs || 0);
      }
      if (key === "status") {
        return String(a.status || "").localeCompare(String(b.status || ""));
      }
      return String(a.filename || a.inputPath || "").localeCompare(String(b.filename || b.inputPath || ""));
    });
    return rows;
  }

  function uniqueMetrics(result) {
    const purpose = uniquePurposeMetrics(result);
    if (purpose.length) {
      return purpose;
    }
    return uniqueSeriesMetrics(result);
  }

  function uniqueSeriesMetrics(result) {
    const seen = new Set();
    for (const item of result?.results || []) {
      for (const series of item.series || []) {
        if (series.column) {
          seen.add(series.column);
        }
      }
    }
    return [...seen].sort((a, b) => a.localeCompare(b));
  }

  function uniquePurposeMetrics(result) {
    const seen = new Set();
    for (const item of result?.results || []) {
      for (const metric of item.purposeMetrics || []) {
        if (metric.id) {
          seen.add(metric.id);
        }
      }
    }
    return [...seen].sort((a, b) => a.localeCompare(b));
  }

  function firstMetric(result) {
    return uniqueMetrics(result)[0] || "";
  }

  function rowID(item) {
    return item.runId || item.inputPath || item.filename || "";
  }

  function fileName(path) {
    const text = String(path || "");
    return text.split(/[\\/]/).filter(Boolean).pop() || "";
  }

  function formatDuration(ms) {
    const value = Number(ms || 0);
    if (value < 1000) {
      return `${value} ms`;
    }
    return `${(value / 1000).toFixed(1)} s`;
  }

  function formatNumber(value) {
    const number = Number(value);
    if (!Number.isFinite(number)) {
      return "—";
    }
    if (Math.abs(number) >= 10000 || (Math.abs(number) > 0 && Math.abs(number) < 0.001)) {
      return number.toExponential(2);
    }
    return number.toLocaleString(undefined, { maximumFractionDigits: 3 });
  }

  function primaryPurposeMetric(item) {
    const metric = (item.purposeMetrics || [])[0];
    if (!metric) {
      return "";
    }
    return `${metric.label || metric.id}: ${metric.displayValue || metric.value || ""}`;
  }

  function batchPurposeRequest() {
    const purposes = [...(elements.batchPurposeInputs || [])]
      .filter((input) => input.checked)
      .map((input) => input.dataset.batchPurpose)
      .filter(Boolean);
    return {
      purposes: purposes.length ? purposes : ["basic_energy"],
      basicEnergyDetail: "energy_path",
      zoneHeatFlowDetail: "surface",
      frequencyPolicy: "purpose_default",
      allocationPolicy: "by_service_path_load_share",
      sqlMode: "sql_first",
      persistOutputs: false,
      discoveryAllowed: false,
      outputApplyMode: "add_missing_only",
      scope: {
        zoneMode: "all",
        zoneNames: [],
        periodMode: "full",
        periodStart: "",
        periodEnd: "",
        loopMode: "all",
        airLoopNames: [],
        plantLoopNames: [],
        condenserLoopNames: [],
        componentIds: [],
        outputSignatures: [],
        customOutputs: [],
      },
    };
  }

  function bindEvents() {
    for (const container of [elements.multiSimulationTable, elements.multiSimulationChart]) container?.addEventListener("click", (event) => {
      const button = event.target instanceof Element ? event.target.closest("[data-batch-energy-open]") : null;
      if (!button || button.disabled) return;
      event.preventDefault();
      if (!energyDetail?.open(button.dataset.batchEnergyOpen, button) && elements.multiSimulationStatus) {
        elements.multiSimulationStatus.textContent = t("batch.energyPathDetailUnavailable", {}, "The annual Energy Path graph is not included in this run.");
      }
    });
    elements.multiSimulationSelectFiles?.addEventListener("click", selectFiles);
    elements.multiSimulationSelectFolder?.addEventListener("click", selectFolder);
    elements.multiSimulationRun?.addEventListener("click", run);
    elements.multiSimulationExport?.addEventListener("click", exportMultiSimulationCSV);
    elements.multiSimulationExportXLSX?.addEventListener("click", exportMultiSimulationXLSX);
    elements.multiSimulationExportJSON?.addEventListener("click", exportMultiSimulationJSON);
    elements.multiSimulationMetric?.addEventListener("change", () => {
      state.multiSimulation.metric = elements.multiSimulationMetric.value || "";
      if (state.multiSimulation.result) {
        renderChart(state.multiSimulation.result);
      }
    });
    elements.multiSimulationCompareBaseline?.addEventListener("change", () => handleEnergyCompareSelectChange("baseline"));
    elements.multiSimulationCompareTarget?.addEventListener("change", () => handleEnergyCompareSelectChange("target"));
    elements.multiSimulationSort?.addEventListener("change", () => {
      state.multiSimulation.sort = elements.multiSimulationSort.value || "filename";
      if (state.multiSimulation.result) {
        renderTable(state.multiSimulation.result);
      }
    });
  }

  bindEvents();

  return {
    handleProgress,
    loadEnvironment,
  };
}
