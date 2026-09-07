package frontendchecks

import (
	"strings"
	"testing"
)

func TestEPATH093FrontendSeparatesAndNavigatesSourceCorrespondence(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/energy-path-view.js")
	for _, required := range []string{
		`energyPathToken(link.relation) === "source_correspondence"`,
		`links: connectedLinks.filter((link) => !isEnergyPathNonFlowRelation(link))`,
		`relations: connectedLinks.filter(isEnergyPathNonFlowRelation)`,
		`energyPathCorrespondenceCounterparts(scene.allGraphNodes, scene.graph.relations, selectedID)`,
		`driverCategory === "internal.lighting"`,
		`driverCategory === "internal.equipment"`,
		`data-energy-path-related="true"`,
		`data-energy-path-correspondence-action=`,
		`data-energy-explanation-node=`,
		`"related_thermal_effect"`,
		`"related_energy_use"`,
		`"Related thermal effect"`,
		`"Related energy use"`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("EPATH-093 frontend correspondence contract missing %q", required)
		}
	}
	if strings.Contains(view, `data-energy-path-correspondence-link`) {
		t.Fatal("EPATH-093 non-flow source correspondence must not have a ribbon/lane renderer")
	}

	simulationViews := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	for _, required := range []string{
		`const energyNode = event.target.closest("[data-energy-explanation-node]")`,
		`selectSimulationEnergyGraphItem(energyNode.dataset.energyExplanationNode || "")`,
		`function selectSimulationEnergyGraphItem(id)`,
		`state.simulationEnergySelection = id;`,
	} {
		if !strings.Contains(simulationViews, required) {
			t.Fatalf("EPATH-093 related action no longer shares the existing node-selection mechanism: missing %q", required)
		}
	}
}

func TestEPATH093FrontendCorrespondenceAccessibilityStylesAndLocales(t *testing.T) {
	styles := readTestFile(t, "frontend/src/styles/simulation.css")
	for _, required := range []string{
		`.energy-path-node.related:not(.selected)`,
		`outline: 2px solid`,
		`.energy-path-correspondence-action`,
		`.energy-path-correspondence-action:focus-visible`,
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("EPATH-093 counterpart outline/action accessibility style missing %q", required)
		}
	}

	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	for _, required := range []string{
		`"simulation.energyPathRelatedThermalEffect": "Related thermal effect"`,
		`"simulation.energyPathRelatedEnergyUse": "Related energy use"`,
		`"simulation.energyPathRelatedThermalEffect": "관련 열 영향"`,
		`"simulation.energyPathRelatedEnergyUse": "관련 에너지 사용"`,
	} {
		if !strings.Contains(i18n, required) {
			t.Fatalf("EPATH-093 localized correspondence action missing %q", required)
		}
	}
}
