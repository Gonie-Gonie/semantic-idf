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
