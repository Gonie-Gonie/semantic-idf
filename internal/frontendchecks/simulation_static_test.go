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
		"simulationServicePathSupportingAssets",
		"simulation.sourceEnergy",
		"data-simulation-hvac-path-id",
		"simulationRelatedServicePathsForEnergySelection",
		"simulationHVACServicePathsByIDs",
		"relatedPathIds",
		"focusedEnergyExplanationGraph",
		"data-simulation-energy-focus-mode",
		"data-simulation-energy-service-path-focus",
		"data-simulation-energy-service-path-jump",
		"data-simulation-energy-sign-mode",
		"energyExplanationSignModeGraph",
		"cooling_pressure",
		"heating_pressure",
		"groupedEnergyExplanationGraph",
		"data-simulation-energy-node-limit",
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
		"Source IDs",
		"Related Paths",
		"edge.relatedPathIds",
		"completeness.sourceAvailability",
		"explanation.relationshipRules",
		"renderEnergyDerivedKPISection",
		"energyExplanationDerivedKPIItems",
		"renderEnergyUseBreakdownSection",
		"simulation.energyUseBreakdown",
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
	if !strings.Contains(indexHTML, "simulationPurposeAllocationPolicy") || !strings.Contains(indexHTML, "by_zone_load_share") || !strings.Contains(indexHTML, "by_service_path_load_share") {
		t.Fatalf("simulation allocation policy control is missing")
	}
	styles := readTestFile(t, "frontend/src/styles/simulation.css")
	for _, term := range []string{".energy-related-service-paths", ".energy-service-path-chip", ".simulation-energy-focus-controls", ".simulation-energy-period-row", ".simulation-energy-zone-paths", ".simulation-energy-zone-actions", ".simulation-energy-chart-period", ".energy-explanation-output-actions", ".energy-source-availability", ".simulation-source-output-jump", ".energy-reconciliation-sources", ".energy-sankey-edge.selected", ".energy-sankey-node.connected", ".energy-sankey-node.electricity", ".energy-sankey-node.district_cooling", ".energy-sankey-legend i.node"} {
		if !strings.Contains(styles, term) {
			t.Fatalf("simulation energy cross-jump style missing %q", term)
		}
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
		"renderEnergyExplanationCompletenessDelta",
		"renderEnergyCompareSelects",
		"selectedEnergyCompareResults",
		"handleEnergyCompareSelectChange",
		"energyExplanationMissingCategorySummary",
		"elements.multiSimulationAllocationPolicy?.value",
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
		"workerCount: Number(elements.multiSimulationWorkers?.value || 0)",
		"weatherMode: elements.multiSimulationWeatherMode?.value",
		"energyExplanationSummaryExportItems",
		"derivedKpis",
		"energy_explanation.derived_kpi",
		"energyExplanationSourceExportItems",
		"energyExplanationEdgeExportItems",
		"energyExplanationBatchExportPeriods",
		"energyExplanationSourceObjectIndexes",
		"sourceIds: item.sourceIds",
		"emptyEnergyExplanationEdgeExportFields(metric.sourceIds",
		"reconciliation.zoneName",
		"energy_explanation.source",
		"energy_explanation.edge",
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
	if !strings.Contains(html, "multiSimulationAllocationPolicy") || !strings.Contains(html, "by_zone_load_share") || !strings.Contains(html, "by_service_path_load_share") {
		t.Fatalf("batch simulation allocation policy control is missing")
	}
	if !strings.Contains(html, "multiSimulationFrequencyPolicy") || !strings.Contains(html, "highest_resolution") {
		t.Fatalf("batch simulation frequency policy control is missing")
	}
}
