package frontendchecks

import (
	"strings"
	"testing"
)

func TestEPATH090FrontendKeepsEndUseAndCarrierStagesDistinct(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/energy-path-view.js")
	for _, required := range []string{
		`level: "end_use"`,
		`level: "carrier"`,
		`const graph = options.graph || energyPathGraphForState(explanation, viewState)`,
		`data-energy-path-canvas`,
		`energyPathLayout(`,
		`export function energyPathGraphForState`,
		`links = [...(scopedResult.links || [])]`,
		`const connectedLinks = links.filter((link) => nodeIDs.has(link.fromId) && nodeIDs.has(link.toId))`,
		`links: connectedLinks.filter((link) => !isEnergyPathNonFlowRelation(link))`,
		`(node.presentationLevel || node.level) === stage.level`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("EPATH-090 frontend contract missing %q", required)
		}
	}

	// A qualified carrier residual may be presented beside end uses without
	// changing its accounting level or adding another main graph stage.
	stages := sliceBetween(view, "export const ENERGY_PATH_STAGES", "export const ENERGY_PATH_PERIODS")
	if strings.Count(stages, `level: "`) != 4 {
		t.Fatal("Energy Path must retain exactly four main stages")
	}
	previous := -1
	for _, level := range []string{"driver", "load", "end_use", "carrier"} {
		index := strings.Index(stages, `level: "`+level+`"`)
		if index < 0 || index <= previous {
			t.Fatalf("Energy Path stage %q is missing or out of canonical order", level)
		}
		previous = index
	}

	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	for _, required := range []string{
		`const useEnergyPathV2 = isEnergyPathV2(explanation)`,
		`if (useEnergyPathV2)`,
		`renderEnergyPathView(explanation, state, simulationEnergySceneOptions(scene))`,
		`outputObjects: scene.result?.purposeRunPlan?.outputObjects || []`,
		`inspectorActionsForNode:`,
		`"simulation.energyPathUpgradeUnavailable"`,
	} {
		if !strings.Contains(simulation, required) {
			t.Fatalf("EPATH-090 changed the v2 dispatch or legacy-result guidance: missing %q", required)
		}
	}
}
