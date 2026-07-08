package frontendchecks

import (
	"strings"
	"testing"
)

func TestFrontendSimulationEnergySystemsCrossJumpContracts(t *testing.T) {
	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	for _, term := range []string{
		`["systems", t("simulation.systems"`,
		"renderEnergySystemsSubview",
		"simulationEnergyServiceAggregates",
		"renderSimulationEnergyConnectedSystems",
		"renderSimulationEnergySupportingAssets",
		"simulationServicePathLoopRefs",
		"simulationServicePathSupportingAssets",
		"simulationServicePathSupportingAssetRefs",
		"simulation.sourceEnergy",
		"data-simulation-hvac-path-id",
		"data-simulation-hvac-loop-name",
		"data-simulation-hvac-coupling-id",
		"openSimulationHVACLoopRef",
		"openSimulationHVACCoupling",
		"simulationHVACLoopRefGraphKey",
		"simulationRelatedServicePathsForEnergySelection",
		"simulationHVACServicePathsByIDs",
		"relatedPathIds",
		"focusedEnergyExplanationGraph",
		"data-simulation-energy-focus-mode",
		"data-simulation-energy-service-path-focus",
		"data-simulation-energy-service-path-jump",
		"data-simulation-energy-sankey-mode",
		"data-simulation-energy-sign-mode",
		"energyExplanationSignModeGraph",
		"cooling_pressure",
		"heating_pressure",
		"groupedEnergyExplanationGraph",
		"data-simulation-energy-node-limit",
		"data-simulation-energy-show-all-nodes",
		"renderEnergyExplanationGroupingNotice",
		"renderEnergySignConventionNote",
		"renderEnergySankeyModeControls",
		"energyExplanationSankeyMode",
		"energyExplanationSankeyColumnConfig",
		"energyExplanationSankeyDisplayGraph",
		"simulation.energySankeyMode",
		"simulation.energySankeyGrouped",
		"simulation.energySignConvention",
		"heat.other_grouped",
		"energyAllocationPolicyLabel",
		"energy-source-availability",
		"simulation-source-output-jump",
		"data-jump-object-index",
		"sourceOutputForEnergySource",
		"findPurposeOutputObjectByIndex",
		"source.objectIndex",
		"source.aggregationMethod",
		"source.sourceUnit",
		"source.normalizedUnit",
		"ruleId",
		"relationshipRules",
		"relationshipRule",
		"energyExplanationRelationshipRuleLabel",
		"selection.relation",
		"selection.pathType",
		"state.simulationEnergySelection === edge.id",
		"energyExplanationEdgeClassTokens",
		"connectedNodeIDs.has(node.id)",
		"energyExplanationNodeClassTokens",
		"data-simulation-energy-period-jump",
		"energyPointPeriodID",
		"simulation-energy-chart-period",
		"renderPurposeHTMLEnergyExplanation",
		"purposeHTMLEnergySummaryRows",
		"purposeHTMLAnnualEnergyGraph",
		"Energy Explanation Annual Edges",
		"Energy Explanation Reconciliation",
		"Energy Explanation Sources",
		"Energy Explanation Source Availability",
		"Energy Explanation Relationship Rules",
		"Energy Explanation Warnings",
		"Energy Explanation Monthly Ledger",
		"purposeHTMLEnergyMonthlyRows",
		"purposeHTMLEnergyWarningRows",
		"period.warnings",
		"Source IDs",
		"Related Paths",
		"edge.relatedPathIds",
		"completeness.sourceAvailability",
		"explanation.relationshipRules",
		"renderEnergyDerivedKPISection",
		"energyExplanationDerivedKPIItems",
		"renderEnergyUseBreakdownSection",
		"renderSimulationEnergyServicePathFocusButton",
		"energyUseTotalBasisNote",
		"energyMeterHierarchyLabel",
		"simulation.energyUseBreakdown",
		"simulation.energyUseTotalBasis",
		"renderEnergyExplanationMonthlyLevelChart",
		"simulation.energyExplanationMonthlyLevels",
		"renderEnergyZoneBreakdownSection",
		"energyZoneBreakdownRows",
		"data-simulation-energy-zone-jump",
		"data-simulation-energy-heatflow-zone-jump",
		"data-simulation-energy-output-plan",
		"data-simulation-energy-apply-outputs",
		"openSimulationPurposeOutputPlan",
		"purposeOutputApplyState",
		"purposeOutputSetLabel",
		"purposeOutputLooksLikeEnergyExplain",
		"purposeOutputLooksLikeHeatDriver",
		"simulation.outputSet",
		"simulationPurposeEnergyDetail",
		"basicEnergyDetail",
		"basicEnergyDetailLabel",
		"currentBasicEnergyDetail",
		"simulation.basicEnergyDetail",
		"simulation.energyDetailTierHint",
		"simulation.energyOutputShortageHint",
		"simulation.energyAccountingCoverageHint",
		"result.purposeResults?.zoneHeatFlow",
		"simulation.energyZoneBreakdown",
		"simulation.openServicePathInSankey",
		"simulation.openZoneInSankey",
		"simulation.openZoneHeatFlow",
		"simulation.relation",
		"selection.meterHierarchyLevel",
		"meterHierarchy",
		"missingCategories",
		"renderEnergyReconciliationSources",
		"energy-reconciliation-sources",
		"energyReconciliationStatus",
		"energyReconciliationStatusLabel",
		"energy-reconciliation-status",
		"renderEnergyZoneResidualRanking",
		"zoneHeatResidualRanking",
		"item.zoneName",
		"item.serviceKind",
		"renderSourceOutputCell(object, { compact: true })",
		"simulationPurposeAllocationPolicy",
		"elements.simulationPurposeAllocationPolicy?.value",
		"navigateHVAC(",
	} {
		if !strings.Contains(simulation, term) {
			t.Fatalf("simulation energy systems contract missing %q", term)
		}
	}
	hvac := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	if !strings.Contains(hvac, "export function navigateHVAC") {
		t.Fatalf("hvac navigation should remain exportable for simulation energy cross-jumps")
	}
	indexHTML := readTestFile(t, "frontend/src/index.html")
	if !strings.Contains(indexHTML, "simulationPurposeAllocationPolicy") || !strings.Contains(indexHTML, "simulationPurposeEnergyDetail") || !strings.Contains(indexHTML, "by_zone_load_share") || !strings.Contains(indexHTML, "by_service_path_load_share") {
		t.Fatalf("simulation allocation policy control is missing")
	}
	styles := readTestFile(t, "frontend/src/styles/simulation.css")
	for _, term := range []string{".energy-related-service-paths", ".energy-service-path-chip", ".energy-service-path-action-row", ".simulation-energy-system-links", ".simulation-energy-system-chip", ".energy-explanation-drilldown-actions", ".energy-use-total-basis", ".simulation-energy-focus-controls", ".simulation-energy-period-row", ".simulation-energy-zone-paths", ".simulation-energy-zone-actions", ".simulation-energy-chart-period", ".energy-explanation-output-actions", ".energy-source-availability", ".energy-source-availability-status.missing", ".energy-source-availability-status.not_applicable", ".simulation-source-output-jump", ".energy-reconciliation-sources", ".energy-reconciliation-status", ".energy-sankey-grouping-notice", ".energy-sankey-sign-note", ".energy-sankey-edge.measured_meter", ".energy-sankey-edge.measured_energy_variable", ".energy-sankey-edge.integrated_rate_variable", ".energy-sankey-edge.selected", ".energy-sankey-node.connected", ".energy-sankey-node.electricity", ".energy-sankey-node.district_cooling", ".energy-sankey-node.fans", ".energy-sankey-node.pumps", ".energy-sankey-node.heat_recovery", ".energy-sankey-node.water_systems", ".energy-sankey-node.refrigeration", ".energy-sankey-node.generators", ".energy-sankey-node.other", ".energy-sankey-legend i.node", ".energy-sankey-legend i.measured_meter", ".energy-sankey-legend i.measured_energy_variable", ".energy-sankey-legend i.integrated_rate_variable"} {
		if !strings.Contains(styles, term) {
			t.Fatalf("simulation energy cross-jump style missing %q", term)
		}
	}
	if !strings.Contains(simulation, "function energyEndUseLabel") || !strings.Contains(simulation, "energyEndUseGenerators") || !strings.Contains(simulation, "energyEndUseStorageCharge") {
		t.Fatalf("simulation energy end-use label mapping is missing")
	}
	if !strings.Contains(simulation, "function energyExplanationBasisLabel") || !strings.Contains(simulation, "basisMeasuredEnergyVariable") || !strings.Contains(simulation, "basisIntegratedRateVariable") {
		t.Fatalf("simulation energy basis label mapping is missing")
	}
	if !strings.Contains(simulation, "function renderSimulationEnergyDrilldownActions") || !strings.Contains(simulation, "data-simulation-energy-heatflow-zone-jump") {
		t.Fatalf("simulation energy drilldown action mapping is missing")
	}
}

func TestFrontendBatchEnergyExplanationDeltaContracts(t *testing.T) {
	batch := readTestFile(t, "frontend/src/js/batch/batch-simulation.js")
	for _, term := range []string{
		"renderEnergyExplanationDeltaRanking",
		"renderEnergyExplanationEdgeDeltaRanking",
		"energyExplanationDeltaRows",
		"energyExplanationEdgeDeltaRows",
		"energyExplanationAnnualEdgeItems",
		"energyExplanationDeltaStatus",
		"renderEnergyExplanationEdgeDeltaBars",
		"batch-energy-edge-delta-view",
		"energyExplanationDeltaValue",
		"energyExplanationDeltaPercent",
		"leftMissing",
		"rightMissing",
		"common.missing",
		"renderEnergyExplanationCompletenessDelta",
		"renderEnergyCompareSelects",
		"selectedEnergyCompareResults",
		"handleEnergyCompareSelectChange",
		"energyExplanationMissingCategorySummary",
		"elements.multiSimulationAllocationPolicy?.value",
		"elements.multiSimulationEnergyDetail?.value",
		"elements.multiSimulationFrequencyPolicy?.value",
		"exportMultiSimulationCSV",
		"exportMultiSimulationXLSX",
		"exportMultiSimulationJSON",
		"multiSimulationComparisonContext",
		"multiSimulationExportContext",
		"context: multiSimulationExportContext(result)",
		"SaveBatchSimulationXLSX({",
		"context: exportContext",
		"semantic-idf.batch-simulation/v1",
		"baselineRowId",
		"targetRowId",
		"purposeRequest: batchPurposeRequest()",
		"basicEnergyDetail: elements.multiSimulationEnergyDetail?.value",
		"workerCount: Number(elements.multiSimulationWorkers?.value || 0)",
		"weatherMode: elements.multiSimulationWeatherMode?.value",
		"energyExplanationSummaryExportItems",
		"derivedKpis",
		"energy_explanation.derived_kpi",
		"energyExplanationSourceExportItems",
		"energyExplanationEdgeExportItems",
		"energyExplanationWarningExportItems",
		"energyExplanationBatchExportPeriods",
		"energyExplanationSourceObjectIndexes",
		"sourceIds: item.sourceIds",
		"emptyEnergyExplanationEdgeExportFields(metric.sourceIds",
		"reconciliation.zoneName",
		"reconciliation.status",
		"energy_explanation.source",
		"energy_explanation.edge",
		"energy_explanation.warning",
		"source_frequency",
		"source_aggregation",
		"source_unit",
		"normalized_unit",
		"path_type",
		"source_object_index",
		"rule_id",
		"source_ids",
		"related_path_ids",
		"Largest Energy Explanation Changes",
		"Sankey Edge Delta",
		"Basis</th><th>Edge",
		"missing in baseline",
	} {
		if !strings.Contains(batch, term) {
			t.Fatalf("batch energy explanation delta contract missing %q", term)
		}
	}
	html := readTestFile(t, "frontend/src/batch.html")
	if !strings.Contains(html, "multiSimulationExport") || !strings.Contains(html, "multiSimulationExportXLSX") || !strings.Contains(html, "multiSimulationExportJSON") {
		t.Fatalf("batch simulation export button is missing")
	}
	if !strings.Contains(html, "multiSimulationCompareBaseline") || !strings.Contains(html, "multiSimulationCompareTarget") {
		t.Fatalf("batch simulation energy comparison selectors are missing")
	}
	if !strings.Contains(html, "multiSimulationAllocationPolicy") || !strings.Contains(html, "multiSimulationEnergyDetail") || !strings.Contains(html, "by_zone_load_share") || !strings.Contains(html, "by_service_path_load_share") {
		t.Fatalf("batch simulation allocation policy control is missing")
	}
	if !strings.Contains(html, "multiSimulationFrequencyPolicy") || !strings.Contains(html, "highest_resolution") {
		t.Fatalf("batch simulation frequency policy control is missing")
	}
	batchApp := readTestFile(t, "batch_app.go")
	for _, term := range []string{"batchSimulationEnergyWarningSection", "batchSimulationEnergyWarningRows", "Energy Warnings", "energy_warnings"} {
		if !strings.Contains(batchApp, term) {
			t.Fatalf("batch simulation warning workbook export missing %q", term)
		}
	}
	if !strings.Contains(batchApp, "reconciliation_status") || !strings.Contains(batchApp, "row.Status") {
		t.Fatalf("batch simulation reconciliation workbook status export is missing")
	}
	if !strings.Contains(batchApp, "basic_energy_detail") || !strings.Contains(batchApp, "purposeRequest.BasicEnergyDetail") {
		t.Fatalf("batch simulation run context should preserve Basic Energy detail")
	}
	styles := readTestFile(t, "frontend/src/styles/workspace.css")
	if !strings.Contains(styles, ".batch-energy-edge-delta-view") || !strings.Contains(styles, ".batch-energy-edge-delta-track") {
		t.Fatalf("batch energy edge delta styles are missing")
	}
}
