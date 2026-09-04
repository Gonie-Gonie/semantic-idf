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
		`const graph = energyPathGraphForState(explanation, viewState)`,
		`renderEnergyPathStage(stage, graph.nodes, selectedID, relatedNodeIDs, index)`,
		`export function energyPathGraphForState`,
		`links = [...(scopedResult.links || [])]`,
		`const connectedLinks = links.filter((link) => nodeIDs.has(link.fromId) && nodeIDs.has(link.toId))`,
		`links: connectedLinks.filter((link) => !isEnergyPathNonFlowRelation(link))`,
		`node.level === stage.level`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("EPATH-090 frontend contract missing %q", required)
		}
	}

	endUseStage := strings.Index(view, `level: "end_use"`)
	carrierStage := strings.Index(view, `level: "carrier"`)
	if endUseStage < 0 || carrierStage <= endUseStage {
		t.Fatal("End-use Energy must remain a distinct stage before Energy Source")
	}

	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	for _, required := range []string{
		`const useEnergyPathV2 = isEnergyPathV2(explanation)`,
		`if (useEnergyPathV2)`,
		`renderEnergyPathView(explanation, state)`,
		`return renderEnergyExplanationSankey(explanation)`,
	} {
		if !strings.Contains(simulation, required) {
			t.Fatalf("EPATH-090 changed the v2 dispatch or v1 compatibility path: missing %q", required)
		}
	}
}
