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
		"data-simulation-hvac-path-id",
		"simulationRelatedServicePathsForEnergySelection",
		"simulationHVACServicePathsByIDs",
		"relatedPathIds",
		"focusedEnergyExplanationGraph",
		"data-simulation-energy-focus-mode",
		"data-simulation-energy-service-path-focus",
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
		"ruleId",
		"relationshipRules",
		"relationshipRule",
		"energyExplanationRelationshipRuleLabel",
		"missingCategories",
		"renderEnergyReconciliationSources",
		"energy-reconciliation-sources",
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
	if !strings.Contains(indexHTML, "simulationPurposeAllocationPolicy") || !strings.Contains(indexHTML, "by_zone_load_share") {
		t.Fatalf("simulation allocation policy control is missing")
	}
	styles := readTestFile(t, "frontend/src/styles/simulation.css")
	for _, term := range []string{".energy-related-service-paths", ".energy-service-path-chip", ".simulation-energy-focus-controls", ".energy-source-availability", ".simulation-source-output-jump", ".energy-reconciliation-sources"} {
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
		"elements.multiSimulationAllocationPolicy?.value",
		"exportMultiSimulationCSV",
		"energyExplanationSummaryExportItems",
		"energyExplanationSourceExportItems",
		"energyExplanationEdgeExportItems",
		"energyExplanationSourceObjectIndexes",
		"energy_explanation.source",
		"energy_explanation.edge",
		"source_frequency",
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
	if !strings.Contains(html, "multiSimulationExport") {
		t.Fatalf("batch simulation export button is missing")
	}
	if !strings.Contains(html, "multiSimulationAllocationPolicy") || !strings.Contains(html, "by_zone_load_share") {
		t.Fatalf("batch simulation allocation policy control is missing")
	}
}
