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
		"focusedEnergyExplanationGraph",
		"data-simulation-energy-focus-mode",
		"data-simulation-energy-service-path-focus",
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
	styles := readTestFile(t, "frontend/src/styles/simulation.css")
	for _, term := range []string{".energy-related-service-paths", ".energy-service-path-chip", ".simulation-energy-focus-controls"} {
		if !strings.Contains(styles, term) {
			t.Fatalf("simulation energy cross-jump style missing %q", term)
		}
	}
}

func TestFrontendBatchEnergyExplanationDeltaContracts(t *testing.T) {
	batch := readTestFile(t, "frontend/src/js/batch/batch-simulation.js")
	for _, term := range []string{
		"renderEnergyExplanationDeltaRanking",
		"energyExplanationDeltaRows",
		"energyExplanationDeltaStatus",
		"Largest Energy Explanation Changes",
		"missing in baseline",
	} {
		if !strings.Contains(batch, term) {
			t.Fatalf("batch energy explanation delta contract missing %q", term)
		}
	}
}
