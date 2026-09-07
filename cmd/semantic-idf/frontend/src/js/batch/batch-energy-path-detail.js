import { t } from "../i18n.js";
import { escapeHTML } from "../state.js";
import {
  isEnergyPathV2, prepareEnergyPathScene, renderEnergyPathView, renderEnergyPathKPI,
  energyPathSummaryForState, energyPathQualityForState, energyPathRatioQualityForState,
  updateEnergyPathSelection, updateEnergyPathDetails,
} from "../views/energy-path-view.js";
import { energyPathKPIItems } from "../energy-path-kpis.js";
import { resolveEnergyPathOutputRequest } from "../energy-path-output-requests.js";

const copy = (key, fallback) => t(`batch.energyPath${key}`, {}, fallback);
const token = (value) => String(value || "").trim().toLowerCase();

// Eligibility is a cheap payload check, never graph preparation for every row.
export function batchEnergyPathDetailAvailable(run = {}) {
  const explanation = run.purposeResults?.energyExplanation;
  if (!isEnergyPathV2(explanation) || token(explanation.scope?.kind) === "zone") return false;
  const annual = (Array.isArray(explanation.periods) ? explanation.periods : []).find((period) => token(period?.id) === "annual");
  const nodes = Array.isArray(annual?.nodes) && annual.nodes.length ? annual.nodes : Array.isArray(explanation.nodes) ? explanation.nodes : [];
  return nodes.some((node) => ["driver", "load", "end_use", "carrier", "residual"].includes(node?.level) &&
    (!node.period || token(node.period) === "annual"));
}

export function createBatchEnergyPathDetail({ host, resolveRun }) {
  let current = null;
  let opener = null;
  let drawerOpener = null;
  const options = () => ({ scene: current.scene, fixedContext: true,
    outputObjects: current.run.purposeRunPlan?.outputObjects || [], drawer: { ...current.drawer } });
  const graphControl = (id) => [...host.querySelectorAll("[data-energy-explanation-node], [data-energy-explanation-edge]")]
    .find((element) => (element.dataset.energyExplanationNode === id || element.dataset.energyExplanationEdge === id) && element.tabIndex >= 0);
  const focusGraph = (id) => {
    const target = graphControl(id) || host.querySelector("[data-energy-path-canvas]");
    if (target && !graphControl(id)) target.tabIndex = -1;
    target?.focus({ preventScroll: true });
  };
  const select = (id) => {
    if (!current) return;
    if (id && !current.scene.allGraphNodes.some((node) => node.id === id) &&
      !current.scene.drawing.ribbons.some((link) => link.id === id)) return;
    current.viewState.simulationEnergySelection = id;
    updateEnergyPathSelection(host, current.scene, current.viewState, options());
    focusGraph(id);
  };
  const close = ({ restoreFocus = true } = {}) => {
    host.hidden = true;
    host.innerHTML = "";
    current = null;
    drawerOpener = null;
    if (restoreFocus && opener?.isConnected) opener.focus();
    opener = null;
  };
  const updateDrawer = ({ closeDrawer = false, output = false, tab = "", accounting = false } = {}) => {
    updateEnergyPathDetails(host, current.scene, current.viewState, options());
    if (closeDrawer) {
      const target = drawerOpener?.isConnected ? drawerOpener : host.querySelector("[data-energy-path-details-toggle]");
      for (let ancestor = target?.parentElement; ancestor && ancestor !== host; ancestor = ancestor.parentElement) {
        if (ancestor.tagName === "DETAILS") ancestor.open = true;
      }
      target?.focus({ preventScroll: true });
      return;
    }
    const target = tab ? host.querySelector(`[data-energy-path-details-tab="${tab}"]`)
      : output ? host.querySelector('[data-energy-path-output-request-selected="true"]')
        : accounting ? host.querySelector("[data-energy-path-accounting-quality]") : null;
    const focus = target || host.querySelector("[data-energy-path-data-details]");
    if (focus) { focus.tabIndex = focus.hasAttribute("data-energy-path-details-tab") ? 0 : -1; focus.focus({ preventScroll: true }); }
  };

  const open = (runID, control) => {
    const run = resolveRun(runID);
    if (!run || !batchEnergyPathDetailAvailable(run)) return false;
    const viewState = { simulationEnergyScopeKind: "building", simulationEnergyZoneName: "",
      simulationEnergyPeriod: "annual", simulationEnergyService: "all", simulationEnergySelection: "", simulationEnergyDetailsOpen: false };
    const explanation = run.purposeResults.energyExplanation;
    const scene = prepareEnergyPathScene(explanation, viewState);
    if (!scene.visibleNodes.length) return false;
    const summary = energyPathSummaryForState(explanation, run.purposeResults.energyExplanationSummary || {}, viewState);
    const quality = energyPathQualityForState(explanation, viewState), ratios = energyPathRatioQualityForState(explanation, viewState);
    if (ratios) quality.ratios = ratios; else delete quality.ratios;
    const kpiOptions = { graph: scene.allServiceGraph, quality, service: "all", period: "annual", detailsOpen: false };
    const kpiTargets = new Set(energyPathKPIItems(summary || {}, scene.allServiceGraph, kpiOptions).flatMap((item) => item.targets || []).map((node) => node.id));
    current = { run, scene, viewState, drawer: { tab: "data", stage: "", outputSource: "" }, kpiTargets };
    opener = control || null;
    drawerOpener = null;
    const label = run.filename || String(run.inputPath || "").split(/[\\/]/).pop() || copy("SelectedModel", "Selected model");
    host.innerHTML = `<header class="batch-energy-detail-header"><div><h4 id="batchEnergyDetailTitle">${escapeHTML(copy("ModelDetail", "Model Energy Path"))}</h4><strong>${escapeHTML(label)}</strong></div><button type="button" data-batch-energy-close>${escapeHTML(t("common.close", {}, "Close"))}</button></header>
      <p class="tool-muted">${escapeHTML(copy("DetailContext", "This detail uses only the selected run. Model-navigation actions require that model to be opened in the main workspace."))}</p>
      <div class="batch-energy-detail-dashboard">${renderEnergyPathKPI(summary, kpiOptions)}${renderEnergyPathView(explanation, viewState, options())}</div>`;
    host.hidden = false;
    host.focus({ preventScroll: true });
    host.scrollIntoView({ block: "nearest" });
    return true;
  };

  host.addEventListener("click", (event) => {
    if (!current || !(event.target instanceof Element)) return;
    const target = event.target;
    if (target.closest("[data-batch-energy-close]")) { event.preventDefault(); close(); return; }
    const details = target.closest("[data-energy-path-details-toggle], [data-energy-path-quality-stage], [data-energy-path-details-tab], [data-energy-path-output-source], [data-energy-path-kpi-details]");
    if (details && !details.disabled) {
      event.preventDefault(); event.stopPropagation();
      const sourceID = details.dataset.energyPathOutputSource;
      if (sourceID !== undefined) {
        const sources = current.scene.explanation.sources || [], matches = sources.filter((source) => source.id === sourceID);
        if (matches.length !== 1 || resolveEnergyPathOutputRequest(matches[0], options().outputObjects, sources).status !== "exact") return;
      }
      if (!current.viewState.simulationEnergyDetailsOpen || !details.closest("[data-energy-path-data-details]")) drawerOpener = details;
      if (details.hasAttribute("data-energy-path-details-toggle")) current.viewState.simulationEnergyDetailsOpen = !current.viewState.simulationEnergyDetailsOpen;
      else current.viewState.simulationEnergyDetailsOpen = true;
      if (details.dataset.energyPathQualityStage !== undefined || details.hasAttribute("data-energy-path-kpi-details")) {
        current.drawer.tab = "data";
        current.drawer.stage = ["drivers", "loads", "endUses", "carriers"].includes(details.dataset.energyPathQualityStage) ? details.dataset.energyPathQualityStage : "";
      }
      if (details.dataset.energyPathDetailsTab !== undefined) current.drawer.tab = details.dataset.energyPathDetailsTab === "output" ? "output" : "data";
      if (sourceID !== undefined) { current.drawer.tab = "output"; current.drawer.outputSource = sourceID; }
      updateDrawer({ closeDrawer: !current.viewState.simulationEnergyDetailsOpen, output: sourceID !== undefined, tab: details.dataset.energyPathDetailsTab, accounting: details.hasAttribute("data-energy-path-kpi-details") });
      return;
    }
    const kpi = target.closest("[data-energy-path-kpi-node]");
    if (kpi) { event.preventDefault(); if (current.kpiTargets.has(kpi.dataset.energyPathKpiNode)) select(kpi.dataset.energyPathKpiNode); return; }
    const control = target.closest("[data-energy-explanation-node], [data-energy-explanation-edge]");
    if (control) { event.preventDefault(); event.stopPropagation(); select(control.dataset.energyExplanationNode || control.dataset.energyExplanationEdge); return; }
    const canvas = target.closest("[data-energy-path-canvas]");
    if (canvas && !target.closest("button, a, input, select, textarea, summary, [role=button]")) {
      const bar = target.closest("[data-energy-path-bar]");
      event.preventDefault();
      if (bar) select(bar.dataset.energyPathBar); else if (!target.closest("[data-energy-path-ribbon]")) select("");
    }
  });
  host.addEventListener("keydown", (event) => {
    if (!current || event.defaultPrevented || event.isComposing || !(event.target instanceof Element)) return;
    const target = event.target;
    if (event.key === "Escape") {
      event.preventDefault(); event.stopPropagation();
      if (current.viewState.simulationEnergyDetailsOpen) {
        current.viewState.simulationEnergyDetailsOpen = false; updateDrawer({ closeDrawer: true });
      } else if (current.viewState.simulationEnergySelection) {
        const previous = current.viewState.simulationEnergySelection; select(""); focusGraph(previous);
      } else close();
      return;
    }
    const tab = target.closest("[data-energy-path-details-tab]");
    if (tab && ["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) {
      event.preventDefault(); event.stopPropagation();
      current.drawer.tab = event.key === "Home" ? "data" : event.key === "End" ? "output" : tab.dataset.energyPathDetailsTab === "data" ? "output" : "data";
      updateDrawer({ tab: current.drawer.tab }); return;
    }
    const edge = target.closest("[data-energy-explanation-edge]");
    if (edge && edge.tagName.toLowerCase() !== "button" && ["Enter", " "].includes(event.key) && !event.repeat) {
      event.preventDefault(); event.stopPropagation(); select(edge.dataset.energyExplanationEdge);
    }
  });
  return { open, close, reset: () => close({ restoreFocus: false }) };
}
