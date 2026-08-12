package frontendchecks

import (
	"strings"
	"testing"
)

func TestThermalTopologyShortcutsAreConfigurableAndContextGuarded(t *testing.T) {
	settings := readTestFile(t, "frontend/src/js/settings-client.js")
	settingsPage := readTestFile(t, "frontend/src/settings.html")
	shortcuts := readTestFile(t, "frontend/src/js/shortcuts.js")
	main := readTestFile(t, "frontend/src/js/main.js")
	for _, required := range []string{
		`geometry3D: "1"`, `geometryPlan: "2"`, `geometryThermal: "3"`, `geometryFit: "F"`,
		`topologyConnectivity: "T"`, `topologyArea: "A"`,
		`topologyUA: "U"`, `topologyQA: "Q"`, `topologyNeighbors: "N"`,
		"validateShortcutConflicts", "allowsBareKey", `state.activeResultTab !== "geometry"`, `state.geometryMode !== "thermal"`,
	} {
		if !strings.Contains(settings+settingsPage+shortcuts+main, required) {
			t.Fatalf("thermal topology shortcut contract is missing %q", required)
		}
	}
	if !strings.Contains(shortcuts, "isEditableTarget(event.target)") || !strings.Contains(shortcuts, "handled === false") {
		t.Fatal("bare topology shortcuts must not run in editors or consume keys outside Geometry")
	}
}

func TestThermalTopologyGraphAccessibilityContract(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	graph := sliceBetween(index, `<div id="thermalTopologyGraph"`, `></div>`)
	if !strings.Contains(graph, `role="region"`) {
		t.Fatal("interactive thermal topology graph must expose its focusable descendants from a region")
	}
	if strings.Contains(graph, `role="img"`) {
		t.Fatal("interactive thermal topology graph must not flatten focusable nodes and edges into an image role")
	}
	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	inspector := readTestFile(t, "frontend/src/js/views/thermal-topology-inspector.js")
	styles := readTestFile(t, "frontend/src/styles/geometry.css")
	for _, required := range []string{
		`tabindex="0" role="button"`, "connectionAriaLabel", "relation node", "issues ${nodeIssues}",
		"localeCompare", "thermalTopologyInspectorHeading", "aria-labelledby", "thermal-legend-line",
		"stroke-dasharray", ":focus-visible",
	} {
		if !strings.Contains(view+inspector+styles, required) {
			t.Fatalf("thermal topology accessibility contract is missing %q", required)
		}
	}
}

func TestGraphicViewportActionsUseAccessibleIconsInsideTheirFigures(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	hvac := sliceBetween(index, `<section class="hvac-main"`, `<aside id="hvacSide"`)
	for _, required := range []string{
		`class="viewport-action-tools hvac-viewport-actions"`,
		`id="hvacExpandButton"`,
		`data-expand-pane="hvac"`,
		`aria-label="Expand"`,
		`title="Expand"`,
		`aria-pressed="false"`,
		`class="viewport-icon viewport-icon-expand"`,
		`class="sr-only"`,
	} {
		if !strings.Contains(hvac, required) {
			t.Fatalf("HVAC viewport icon contract is missing %q", required)
		}
	}

	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	for _, required := range []string{
		"function renderHeatFlowPlanViewportActions()",
		`class="viewport-action-tools heatflow-viewport-actions"`,
		`data-heatflow-plan-zoom="out"`,
		`data-heatflow-plan-zoom="reset"`,
		`data-heatflow-plan-zoom="in"`,
		`class="viewport-icon"`,
		`aria-hidden="true"`,
		`title="${escapeHTML(fit)}"`,
		`aria-label="${escapeHTML(fit)}"`,
	} {
		if !strings.Contains(simulation, required) {
			t.Fatalf("Heat Flow viewport icon contract is missing %q", required)
		}
	}
	storyCard := sliceBetween(simulation, "function renderHeatFlowStoryCard", "function heatFlowStoryBounds")
	if !strings.Contains(storyCard, "renderHeatFlowPlanViewportActions()") || !strings.Contains(storyCard, `class="heatflow-floor-viewport"`) {
		t.Fatal("Heat Flow zoom/Fit icons must be rendered inside each plan viewport")
	}

	baseStyles := readTestFile(t, "frontend/src/styles/base.css")
	geometryStyles := readTestFile(t, "frontend/src/styles/geometry.css")
	hvacStyles := readTestFile(t, "frontend/src/styles/hvac.css")
	simulationStyles := readTestFile(t, "frontend/src/styles/simulation.css")
	for _, required := range []string{
		".viewport-action-tools",
		".viewport-icon-button",
		"button:focus-visible",
		".geometry-viewport-actions",
		".hvac-viewport-actions",
		".heatflow-viewport-actions",
	} {
		if !strings.Contains(baseStyles+geometryStyles+hvacStyles+simulationStyles, required) {
			t.Fatalf("shared viewport action styling is missing %q", required)
		}
	}
}
