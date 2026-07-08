export function initializeMultiSimulationTool(context) {
  const { state, elements, waitForAppAPI, waitForProgressRuntime, escapeHTML, postJSON, t, downloadCSV } = context;
  let planPreviewTimer = 0;

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
    if (!elements.multiSimulationEnergyPlus) {
      return;
    }
    const installs = state.simulationEnvironment?.installations || [];
    const currentInstall = elements.multiSimulationEnergyPlus.value;
    elements.multiSimulationEnergyPlus.innerHTML = installs.length
      ? installs
          .map((install) => {
            const label = `${install.name || "EnergyPlus"}${install.autoDetected ? " - auto" : ""}`;
            return `<option value="${escapeHTML(install.executablePath)}" title="${escapeHTML(install.executablePath)}">${escapeHTML(label)}</option>`;
          })
          .join("")
      : `<option value="">${escapeHTML(t("simulation.noEnergyPlus", {}, "No EnergyPlus installation"))}</option>`;
    if (currentInstall && [...elements.multiSimulationEnergyPlus.options].some((option) => option.value === currentInstall)) {
      elements.multiSimulationEnergyPlus.value = currentInstall;
    }

    const currentWeather = elements.multiSimulationWeather.value;
    const weatherHTML = [`<option value="">${escapeHTML(t("simulation.noWeather", {}, "No weather / design-day only"))}</option>`];
    for (const folder of state.simulationEnvironment?.weatherFolders || []) {
      weatherHTML.push(`<optgroup label="${escapeHTML(`${folder.source || "Weather"} - ${folder.label || folder.path}`)}">`);
      for (const file of folder.files || []) {
        weatherHTML.push(`<option value="${escapeHTML(file.path)}" title="${escapeHTML(file.path)}">${escapeHTML(file.name)}</option>`);
      }
      weatherHTML.push("</optgroup>");
    }
    elements.multiSimulationWeather.innerHTML = weatherHTML.join("");
    if (currentWeather && [...elements.multiSimulationWeather.options].some((option) => option.value === currentWeather)) {
      elements.multiSimulationWeather.value = currentWeather;
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
    schedulePlanPreview();
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
    const executablePath = elements.multiSimulationEnergyPlus?.value || "";
    if (!executablePath) {
      elements.multiSimulationStatus.textContent = t("simulation.registerEnergyPlus", {}, "Register EnergyPlus in Settings");
      return;
    }
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
        energyPlusExecutablePath: executablePath,
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
                  <td><input data-multi-sim-row="${escapeHTML(id)}" type="checkbox" ${state.multiSimulation.selectedRows.has(id) ? "checked" : ""} ${item.series?.length || item.purposeMetrics?.length ? "" : "disabled"} /></td>
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
    const viewMode = elements.multiSimulationViewMode?.value || "purpose";
    if (viewMode !== "advanced" && uniquePurposeMetrics(result).length) {
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
      </div>
      ${renderEnergyExplanationBatchCompare(result)}`;
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
          "",
          metric.level || "",
          metric.status || "",
          "",
          "",
          "",
          "",
          "",
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
      viewMode: elements.multiSimulationViewMode?.value || "purpose",
      weatherMode: elements.multiSimulationWeatherMode?.value || "same",
      weatherPath: elements.multiSimulationWeather?.value || "",
      workerCount: Number(elements.multiSimulationWorkers?.value || 0),
      purposeRequest: batchPurposeRequest(),
      comparison: multiSimulationComparisonContext(result),
    };
  }

  function energyExplanationSummaryExportItems(summary = {}) {
    const groups = [
      ["energy_explanation.energy_by_carrier", summary.energyByCarrier || []],
      ["energy_explanation.energy_by_end_use", summary.energyByEndUse || []],
      ["energy_explanation.delivered_load_by_service", summary.deliveredLoadByService || []],
      ["energy_explanation.derived_kpi", summary.derivedKpis || []],
      ["energy_explanation.heat_drivers", summary.heatDrivers || []],
      ["energy_explanation.residuals", summary.residuals || []],
      ["energy_explanation.top_heat_drivers", summary.topHeatDrivers || []],
      ["energy_explanation.top_zones", summary.topZones || []],
    ];
    return groups.flatMap(([type, items]) =>
      (items || []).map((item) => ({
        type,
        id: item.id || "",
        label: energyExplanationSummaryLabel(item),
        value: item.value,
        unit: item.unit || "",
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
      return "";
    }
    const sections = [
      ["Energy Use", "energyByEndUse"],
      ["Delivered Load", "deliveredLoadByService"],
      ["Derived KPI", "derivedKpis"],
      ["Heat Drivers", "heatDrivers"],
      ["Residual", "residuals"],
    ]
      .map(([label, key]) => renderEnergyExplanationDeltaSection(label, selected[0], selected[1], key))
      .filter(Boolean)
      .join("");
    const completeness = renderEnergyExplanationCompletenessDelta(selected[0], selected[1]);
    const ranking = renderEnergyExplanationDeltaRanking(selected[0], selected[1]);
    const edgeRanking = renderEnergyExplanationEdgeDeltaRanking(selected[0], selected[1]);
    return completeness || sections || ranking || edgeRanking
      ? `<div class="batch-energy-explanation-compare">${renderEnergyComparePair(selected[0], selected[1])}${completeness}${ranking}${edgeRanking}${sections}</div>`
      : "";
  }

  function renderEnergyComparePair(leftResult, rightResult) {
    return `
      <div class="batch-energy-compare-pair">
        <div><span>${escapeHTML(t("batch.baselineCase", {}, "Baseline"))}</span><strong>${escapeHTML(leftResult.filename || fileName(leftResult.inputPath))}</strong></div>
        <div><span>${escapeHTML(t("batch.targetCase", {}, "Target"))}</span><strong>${escapeHTML(rightResult.filename || fileName(rightResult.inputPath))}</strong></div>
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
    return (result?.results || []).filter((item) => item.purposeResults?.energyExplanationSummary?.schema);
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

  function renderEnergyExplanationCompletenessDelta(leftResult, rightResult) {
    const left = leftResult.purposeResults?.energyExplanationSummary?.completeness || {};
    const right = rightResult.purposeResults?.energyExplanationSummary?.completeness || {};
    const rows = [
      {
        label: "Status",
        left: left.status || "",
        right: right.status || "",
      },
      {
        label: "Mapped energy",
        left: Number.isFinite(Number(left.mappedPercent)) ? `${formatNumber(left.mappedPercent)}%` : "",
        right: Number.isFinite(Number(right.mappedPercent)) ? `${formatNumber(right.mappedPercent)}%` : "",
      },
      {
        label: "Missing categories",
        left: energyExplanationMissingCategorySummary(left.missingCategories || []),
        right: energyExplanationMissingCategorySummary(right.missingCategories || []),
      },
      {
        label: "Missing source outputs",
        left: energyExplanationSourceAvailabilitySummary(left.sourceAvailability || [], ["missing"]),
        right: energyExplanationSourceAvailabilitySummary(right.sourceAvailability || [], ["missing"]),
      },
      {
        label: "Not-applicable source outputs",
        left: energyExplanationSourceAvailabilitySummary(left.sourceAvailability || [], ["not_applicable"]),
        right: energyExplanationSourceAvailabilitySummary(right.sourceAvailability || [], ["not_applicable"]),
      },
    ].filter((row) => row.left !== row.right);
    if (!rows.length) {
      return "";
    }
    return `
      <section>
        <h4>${escapeHTML(t("simulation.energyCompletenessDelta", {}, "Completeness Delta"))}</h4>
        <div class="tool-table-wrap">
          <table class="tool-table">
            <thead><tr><th>${escapeHTML(t("common.metric", {}, "Metric"))}</th><th>${escapeHTML(leftResult.filename || fileName(leftResult.inputPath))}</th><th>${escapeHTML(rightResult.filename || fileName(rightResult.inputPath))}</th></tr></thead>
            <tbody>${rows
              .map(
                (row) => `
                  <tr>
                    <td>${escapeHTML(row.label)}</td>
                    <td>${escapeHTML(row.left || t("common.notAvailable", {}, "N/A"))}</td>
                    <td>${escapeHTML(row.right || t("common.notAvailable", {}, "N/A"))}</td>
                  </tr>`,
              )
              .join("")}</tbody>
          </table>
        </div>
      </section>`;
  }

  function energyExplanationMissingCategorySummary(items = []) {
    const values = (items || []).filter(Boolean);
    if (!values.length) {
      return "0";
    }
    const preview = values.slice(0, 3).join("; ");
    return values.length > 3 ? `${values.length}: ${preview}; ...` : `${values.length}: ${preview}`;
  }

  function energyExplanationSourceAvailabilitySummary(items = [], statuses = []) {
    const wanted = new Set((statuses || []).map((status) => String(status || "").toLowerCase()));
    const values = (items || [])
      .filter((item) => wanted.has(String(item.status || "").toLowerCase()))
      .map((item) => [item.level || "", item.name || ""].filter(Boolean).join(": "))
      .filter(Boolean)
      .sort((a, b) => a.localeCompare(b));
    if (!values.length) {
      return "0";
    }
    const preview = values.slice(0, 3).join("; ");
    return values.length > 3 ? `${values.length}: ${preview}; ...` : `${values.length}: ${preview}`;
  }

  function renderEnergyExplanationDeltaRanking(leftResult, rightResult) {
    const groups = [
      ["Energy Use", "energyByEndUse"],
      ["Delivered Load", "deliveredLoadByService"],
      ["Derived KPI", "derivedKpis"],
      ["Heat Drivers", "heatDrivers"],
      ["Residual", "residuals"],
    ];
    const rows = groups
      .flatMap(([group, key]) => energyExplanationDeltaRows(group, leftResult, rightResult, key))
      .sort((a, b) => Math.abs(b.delta) - Math.abs(a.delta) || a.group.localeCompare(b.group) || a.label.localeCompare(b.label))
      .slice(0, 12)
      .map(
        (row) => `
          <tr>
            <td>${escapeHTML(row.group)}</td>
            ${energyExplanationDeltaMetricCell(row)}
            <td>${escapeHTML(energyExplanationDeltaValue(row, "left"))}</td>
            <td>${escapeHTML(energyExplanationDeltaValue(row, "right"))}</td>
            <td>${escapeHTML(formatSignedValue(row.delta, row.unit))}</td>
            <td>${escapeHTML(energyExplanationDeltaPercent(row))}</td>
            ${energyExplanationDeltaSourceCell(row)}
            <td>${escapeHTML(row.status)}</td>
          </tr>`,
      )
      .join("");
    if (!rows) {
      return "";
    }
    return `
      <section>
        <h4>${escapeHTML(t("simulation.energyDeltaRanking", {}, "Largest Energy Explanation Changes"))}</h4>
        <div class="tool-table-wrap">
          <table class="tool-table">
            <thead><tr><th>Level</th><th>${escapeHTML(t("common.metric", {}, "Metric"))}</th><th>${escapeHTML(leftResult.filename || fileName(leftResult.inputPath))}</th><th>${escapeHTML(rightResult.filename || fileName(rightResult.inputPath))}</th><th>Delta</th><th>%</th><th>Sources</th><th>Status</th></tr></thead>
            <tbody>${rows}</tbody>
          </table>
        </div>
      </section>`;
  }

  function renderEnergyExplanationEdgeDeltaRanking(leftResult, rightResult) {
    const sortedRows = energyExplanationEdgeDeltaRows(leftResult, rightResult)
      .sort((a, b) => Math.abs(b.delta) - Math.abs(a.delta) || a.relation.localeCompare(b.relation) || a.label.localeCompare(b.label));
    const rows = sortedRows
      .slice(0, 12)
      .map(
        (row) => `
          <tr>
            <td>${escapeHTML(row.relation)}</td>
            <td>${escapeHTML(row.basis || "")}</td>
            <td>${escapeHTML(row.label)}</td>
            <td>${escapeHTML(row.ruleId || "")}</td>
            <td>${escapeHTML(energyExplanationDeltaValue(row, "left"))}</td>
            <td>${escapeHTML(energyExplanationDeltaValue(row, "right"))}</td>
            <td>${escapeHTML(formatSignedValue(row.delta, row.unit))}</td>
            <td>${escapeHTML(energyExplanationDeltaPercent(row))}</td>
            ${energyExplanationDeltaSourceCell(row)}
            <td>${escapeHTML(row.status)}</td>
          </tr>`,
      )
      .join("");
    if (!rows) {
      return "";
    }
    return `
      <section>
        <h4>${escapeHTML(t("simulation.energySankeyEdgeDelta", {}, "Sankey Edge Delta"))}</h4>
        ${renderEnergyExplanationEdgeDeltaBars(sortedRows.slice(0, 8))}
        <div class="tool-table-wrap">
          <table class="tool-table">
            <thead><tr><th>Relation</th><th>Basis</th><th>Edge</th><th>Rule</th><th>${escapeHTML(leftResult.filename || fileName(leftResult.inputPath))}</th><th>${escapeHTML(rightResult.filename || fileName(rightResult.inputPath))}</th><th>Delta</th><th>%</th><th>Sources</th><th>Status</th></tr></thead>
            <tbody>${rows}</tbody>
          </table>
        </div>
      </section>`;
  }

  function renderEnergyExplanationEdgeDeltaBars(rows = []) {
    if (!rows.length) {
      return "";
    }
    const maxDelta = Math.max(...rows.map((row) => Math.abs(Number(row.delta) || 0)), 0);
    return `
      <div class="batch-energy-edge-delta-view" aria-label="${escapeHTML(t("simulation.energySankeyEdgeDelta", {}, "Sankey Edge Delta"))}">
        ${rows
          .map((row) => {
            const width = maxDelta > 0 ? Math.max(4, Math.min(100, (Math.abs(Number(row.delta) || 0) / maxDelta) * 100)) : 4;
            const className = row.leftMissing || row.rightMissing ? "missing" : row.delta >= 0 ? "positive" : "negative";
            return `
              <article class="${escapeHTML(className)}">
                <div>
                  <strong title="${escapeHTML(row.label)}">${escapeHTML(row.label)}</strong>
                  <span>${escapeHTML([row.relation, row.basis].filter(Boolean).join(" / "))}</span>
                </div>
                <div class="batch-energy-edge-delta-track"><i style="width: ${escapeHTML(formatNumber(width))}%;"></i></div>
                <div class="batch-energy-edge-delta-meta">
                  <span>${escapeHTML(energyExplanationDeltaValue(row, "left"))}</span>
                  <strong>${escapeHTML(formatSignedValue(row.delta, row.unit))}</strong>
                  <span>${escapeHTML(energyExplanationDeltaPercent(row))}</span>
                </div>
              </article>`;
          })
          .join("")}
      </div>`;
  }

  function renderEnergyExplanationDeltaSection(label, leftResult, rightResult, key) {
    const rows = energyExplanationDeltaRows(label, leftResult, rightResult, key)
      .sort((a, b) => b.totalMagnitude - a.totalMagnitude || a.label.localeCompare(b.label))
      .slice(0, 12)
      .map(
        (row) => `
          <tr>
            ${energyExplanationDeltaMetricCell(row)}
            <td>${escapeHTML(energyExplanationDeltaValue(row, "left"))}</td>
            <td>${escapeHTML(energyExplanationDeltaValue(row, "right"))}</td>
            <td>${escapeHTML(formatSignedValue(row.delta, row.unit))}</td>
            <td>${escapeHTML(energyExplanationDeltaPercent(row))}</td>
            ${energyExplanationDeltaSourceCell(row)}
            <td>${escapeHTML(row.status)}</td>
          </tr>`,
      )
      .join("");
    if (!rows) {
      return "";
    }
    return `
      <section>
        <h4>${escapeHTML(label)}</h4>
        <div class="tool-table-wrap">
          <table class="tool-table">
            <thead><tr><th>${escapeHTML(t("common.metric", {}, "Metric"))}</th><th>${escapeHTML(leftResult.filename || fileName(leftResult.inputPath))}</th><th>${escapeHTML(rightResult.filename || fileName(rightResult.inputPath))}</th><th>Delta</th><th>%</th><th>Sources</th><th>Status</th></tr></thead>
            <tbody>${rows}</tbody>
          </table>
        </div>
      </section>`;
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

  function energyExplanationDeltaMetricCell(row = {}) {
    const detail = energyExplanationDeltaMetricDetail(row);
    return `
      <td class="batch-energy-delta-metric">
        <span>${escapeHTML(row.label || "")}</span>
        ${detail ? `<small>${escapeHTML(detail)}</small>` : ""}
      </td>`;
  }

  function energyExplanationDeltaMetricDetail(row = {}) {
    const parts = [];
    if (row.formula) {
      parts.push(row.formula);
    }
    const heatDetail = [row.heatCategory, row.sign].filter(Boolean).join(" / ");
    if (heatDetail) {
      parts.push(heatDetail);
    }
    const left = energyExplanationDeltaRatioSideDetail(t("batch.baselineCase", {}, "Baseline"), row.leftNumeratorValue, row.leftDenominatorValue, row);
    const right = energyExplanationDeltaRatioSideDetail(t("batch.targetCase", {}, "Target"), row.rightNumeratorValue, row.rightDenominatorValue, row);
    if (left) {
      parts.push(left);
    }
    if (right) {
      parts.push(right);
    }
    return parts.join(" | ");
  }

  function energyExplanationDeltaSourceCell(row = {}) {
    const left = row.leftSourceSummary || "";
    const right = row.rightSourceSummary || "";
    if (!left && !right) {
      return `<td class="batch-energy-delta-sources muted">-</td>`;
    }
    const sides = [
      [t("batch.baselineCase", {}, "Baseline"), left],
      [t("batch.targetCase", {}, "Target"), right],
    ]
      .filter(([, value]) => value)
      .map(([label, value]) => `<span><b>${escapeHTML(label)}</b><small title="${escapeHTML(value)}">${escapeHTML(value)}</small></span>`)
      .join("");
    return `<td class="batch-energy-delta-sources">${sides}</td>`;
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

  function energyExplanationDeltaRatioSideDetail(label, numeratorValue, denominatorValue, row = {}) {
    const numerator = energyExplanationDeltaRatioPart(row.numeratorLabel, numeratorValue, row.numeratorUnit);
    const denominator = energyExplanationDeltaRatioPart(row.denominatorLabel, denominatorValue, row.denominatorUnit);
    if (!numerator && !denominator) {
      return "";
    }
    return `${label}: ${[numerator, denominator].filter(Boolean).join(" / ")}`;
  }

  function energyExplanationDeltaRatioPart(label = "", value, unit = "") {
    const number = Number(value);
    if (!Number.isFinite(number) || number === 0) {
      return "";
    }
    return [label, formatValue(number, unit)].filter(Boolean).join(" ");
  }

  function energyExplanationDeltaValue(row = {}, side = "left") {
    if (side === "left" ? row.leftMissing : row.rightMissing) {
      return t("common.missing", {}, "Missing");
    }
    return formatValue(side === "left" ? row.leftValue : row.rightValue, row.unit);
  }

  function energyExplanationDeltaPercent(row = {}) {
    if (row.leftMissing || row.percent === null) {
      return t("common.notAvailable", {}, "N/A");
    }
    return `${formatNumber(row.percent)}%`;
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
      return t("common.notAvailable", {}, "N/A");
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
    if ((elements.multiSimulationViewMode?.value || "purpose") === "advanced") {
      return uniqueSeriesMetrics(result);
    }
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
      return "N/A";
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
      basicEnergyDetail: elements.multiSimulationEnergyDetail?.value || "light",
      frequencyPolicy: elements.multiSimulationFrequencyPolicy?.value || "purpose_default",
      allocationPolicy: elements.multiSimulationAllocationPolicy?.value || "direct_only",
      sqlMode: "sql_first",
      persistOutputs: false,
      discoveryAllowed: false,
      outputApplyMode: "add_missing_only",
      scope: {
        zoneMode: "all",
        periodMode: "run_period",
        loopMode: "all",
      },
    };
  }

  function selectedPurposeLabels() {
    return [...(elements.batchPurposeInputs || [])]
      .filter((input) => input.checked)
      .map((input) => input.closest("label")?.innerText?.trim() || input.dataset.batchPurpose)
      .filter(Boolean);
  }

  function simulationRequestForPreview() {
    return {
      inputPaths: state.multiSimulation.selectedPaths || [],
      rootDirectory: state.multiSimulation.rootDirectory || "",
      recursive: Boolean(elements.multiSimulationRecursive?.checked),
      weatherMode: elements.multiSimulationWeatherMode?.value || "same",
      weatherPath: elements.multiSimulationWeather?.value || "",
      workerCount: Number(elements.multiSimulationWorkers?.value || 0),
      purposeRequest: batchPurposeRequest(),
    };
  }

  function schedulePlanPreview() {
    if (!elements.batchSimulationPlanPreview) {
      return;
    }
    clearTimeout(planPreviewTimer);
    planPreviewTimer = setTimeout(refreshPlanPreview, 250);
  }

  async function refreshPlanPreview() {
    const paths = state.multiSimulation.selectedPaths || [];
    if (!paths.length) {
      elements.batchSimulationPlanPreview.innerHTML = `<div class="empty">${escapeHTML(t("batch.planPreviewEmpty", {}, "Select files to preview purpose output weight and weather mapping."))}</div>`;
      return;
    }
    elements.batchSimulationPlanPreview.innerHTML = `<div class="empty status-loading">${escapeHTML(t("batch.planPreviewRunning", {}, "Building purpose run plan preview."))}</div>`;
    const request = simulationRequestForPreview();
    try {
      const api = await waitForAppAPI("PreviewBatchSimulationPlan");
      const result = api ? await api.PreviewBatchSimulationPlan(request) : await postJSON("/api/batch-simulation-plan", request);
      renderPlanPreview(result);
    } catch {
      renderPlanPreview({
        total: paths.length,
        commonOutputCount: 0,
        heavyFileCount: 0,
        workerCount: request.workerCount,
        weatherMode: request.weatherMode,
        weatherPath: request.weatherPath,
        purposes: request.purposeRequest?.purposes || [],
        files: paths.map((path, index) => ({ index, path, filename: fileName(path), status: "pending" })),
      });
    }
  }

  function renderPlanPreview(result) {
    const files = result?.files || [];
    const purposes = selectedPurposeLabels().join(", ");
    const weather = result?.weatherMode === "subfolder" ? t("tools.weatherSubfolder", {}, "Nearest EPW by folder") : fileName(result?.weatherPath) || t("simulation.noWeather", {}, "No weather / design-day only");
    const heavy = result?.heavyFileCount || 0;
    const summary = t(
      "batch.planPreview",
      { count: result?.total || files.length, outputs: result?.commonOutputCount || 0, heavy, weather },
      `${result?.total || files.length} files, ${result?.commonOutputCount || 0} common outputs, ${heavy} heavy files, weather ${weather}.`,
    );
    elements.batchSimulationPlanPreview.innerHTML = `
      <div class="batch-plan-summary">
        <div><span>${escapeHTML(t("batch.purposes", {}, "Purposes"))}</span><strong>${escapeHTML(purposes || "basic_energy")}</strong></div>
        <div><span>${escapeHTML(t("tools.workers", {}, "Workers"))}</span><strong>${escapeHTML(result?.workerCount || 0)}</strong></div>
        <div><span>${escapeHTML(t("batch.heavyFiles", { count: heavy }, `${heavy} heavy files`))}</span><strong>${escapeHTML(summary)}</strong></div>
      </div>
      <div class="tool-table-wrap">
        <table class="tool-table">
          <thead>
            <tr>
              <th class="tool-sticky-col">${escapeHTML(t("common.file", {}, "File"))}</th>
              <th>${escapeHTML(t("common.status", {}, "Status"))}</th>
              <th>${escapeHTML(t("common.outputs", {}, "Outputs"))}</th>
              <th>${escapeHTML(t("common.existingTarget", {}, "Existing target"))}</th>
              <th>${escapeHTML(t("simulation.outputAdded", {}, "Temporary"))}</th>
              <th>${escapeHTML(t("common.scale", {}, "Scale"))}</th>
            </tr>
          </thead>
          <tbody>
            ${files
              .slice(0, 40)
              .map(
                (file) => `
                  <tr>
                    <th class="tool-sticky-col">
                      <strong>${escapeHTML(file.label || file.filename || fileName(file.path))}</strong>
                      <span>${escapeHTML(file.error || file.path || "")}</span>
                    </th>
                    <td class="tool-value ${escapeHTML(file.status || "")}">${escapeHTML(file.status || "")}</td>
                    <td>${escapeHTML(file.outputCount ?? "")}</td>
                    <td>${escapeHTML(file.existingOutputCount ?? "")}</td>
                    <td>${escapeHTML(file.temporaryOutputCount ?? "")}</td>
                    <td>${escapeHTML(file.estimatedWeight || "")}</td>
                  </tr>`,
              )
              .join("")}
          </tbody>
        </table>
      </div>`;
  }

  function bindEvents() {
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
    elements.multiSimulationViewMode?.addEventListener("change", () => {
      if (state.multiSimulation.result) {
        state.multiSimulation.metric = firstMetric(state.multiSimulation.result);
        renderMetricSelect(state.multiSimulation.result);
        renderEnergyCompareSelects(state.multiSimulation.result);
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
    elements.batchPurposeInputs?.forEach((input) => input.addEventListener("change", schedulePlanPreview));
    elements.multiSimulationWeather?.addEventListener("change", schedulePlanPreview);
    elements.multiSimulationWeatherMode?.addEventListener("change", schedulePlanPreview);
    elements.multiSimulationEnergyDetail?.addEventListener("change", schedulePlanPreview);
    elements.multiSimulationAllocationPolicy?.addEventListener("change", schedulePlanPreview);
    elements.multiSimulationFrequencyPolicy?.addEventListener("change", schedulePlanPreview);
    elements.multiSimulationWorkers?.addEventListener("change", schedulePlanPreview);
    elements.multiSimulationRecursive?.addEventListener("change", schedulePlanPreview);
  }

  bindEvents();

  return {
    handleProgress,
    loadEnvironment,
    schedulePlanPreview,
  };
}
