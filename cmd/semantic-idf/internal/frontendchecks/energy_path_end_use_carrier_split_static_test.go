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

	// Four accounting stages retain their identity and canonical order. The
	// optional first column contains measured boundary observations, so it must
	// not become another thermal/site accounting stage. A qualified residual
	// remains beside end uses without adding another graph stage.
	stages := sliceBetween(view, "export const ENERGY_PATH_STAGES", "export const ENERGY_PATH_PERIODS")
	if strings.Count(stages, `level: "`) != 5 {
		t.Fatal("Energy Path must retain four accounting stages and one optional input stage")
	}
	previous := -1
	for _, level := range []string{"input", "driver", "load", "end_use", "carrier"} {
		index := strings.Index(stages, `level: "`+level+`"`)
		if index < 0 || index <= previous {
			t.Fatalf("Energy Path stage %q is missing or out of canonical order", level)
		}
		previous = index
	}
	inputStage := sliceBetween(stages, `level: "input"`, `level: "driver"`)
	if !strings.Contains(inputStage, `scaleDomain: "boundary"`) {
		t.Fatal("Measured input observations must keep their separate boundary domain")
	}
	layout := readTestFile(t, "frontend/src/js/energy-path-layout.js")
	for _, required := range []string{
		`const hasInputs =`,
		`layoutLevel(node || {}) === "input"`,
		`const levels = hasInputs ? LEVELS : LEVELS.slice(1)`,
	} {
		if !strings.Contains(layout, required) {
			t.Fatalf("Optional inputs must preserve the legacy four-column layout: missing %q", required)
		}
	}
	if !strings.Contains(view, `ENERGY_PATH_STAGES.filter((stage) => layout.columns.some((column) => column.level === stage.level))`) {
		t.Fatal("Only columns present in the selected result may render as graph stages")
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
