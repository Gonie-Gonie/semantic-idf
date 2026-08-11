package frontendchecks

import (
	"strings"
	"testing"
)

func TestTOPO260ViewSwitchingAcceptance(t *testing.T) {
	loader := readTestFile(t, "frontend/src/js/geometry-loader.js")
	modeSetter := sliceBetween(loader, "export function setGeometryMode", "export function setGeometryStory")
	for _, required := range []string{"normalizeGeometryMode(mode)", "recordViewHistory()", "state.geometryMode = nextMode"} {
		if !strings.Contains(modeSetter, required) {
			t.Fatalf("view switching contract missing %q", required)
		}
	}
	for _, forbidden := range []string{"selectedGeometryId =", "selectedGeometryKind =", "AnalyzeInput", "api."} {
		if strings.Contains(modeSetter, forbidden) {
			t.Fatalf("3D/Plan/Thermal switch must preserve selection and avoid analysis; found %q", forbidden)
		}
	}
	state := readTestFile(t, "frontend/src/js/state.js")
	if !strings.Contains(state, `return geometryModes.includes(mode) ? mode : "3d"`) {
		t.Fatal("invalid geometry modes must normalize to 3D")
	}
	view := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	for _, required := range []string{
		`elements.geometry3DControls.hidden = !is3D`,
		`elements.geometryPlanControls.hidden = !isPlan`,
		`elements.thermalTopologyControls.hidden = !isNetwork`,
		`elements.geometryStoryControl.hidden = isNetwork && state.thermalTopologyScope !== "story"`,
		`state.geometryMode === "3d"`,
		`return state.selectedGeometryStory === "all" || item.storyIndex === state.selectedGeometryStory`,
		"restoreThermalTopologyState(snapshot, state)",
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("view/story/history acceptance missing %q", required)
		}
	}
}

func TestTOPO261LayoutRoutingAcceptance(t *testing.T) {
	layout := readTestFile(t, "frontend/src/js/views/thermal-topology-layout.js")
	for _, required := range []string{
		"resolveNodeCollisions",
		"placeExternalNodes",
		"chooseThermalPorts",
		"parallelCounts",
		"laneOffset",
		"selfLoop: true",
	} {
		if !strings.Contains(layout, required) {
			t.Fatalf("layout/routing acceptance missing %q", required)
		}
	}
	state := readTestFile(t, "frontend/src/js/state.js")
	if !strings.Contains(state, `export function normalizeThermalTopologyGraphLevel()`) || !strings.Contains(state, `return "zone";`) {
		t.Fatal("Network graph type must be fixed to Zone")
	}
	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, removed := range []string{"expandConnection", "collapseBoundaryGraph", "data-topology-back"} {
		if strings.Contains(view, removed) {
			t.Fatalf("Zone-only Network still exposes boundary drill-down %q", removed)
		}
	}
}

func TestTOPO262NavigationAcceptance(t *testing.T) {
	geometry := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	for _, required := range []string{
		"projectGeometrySelectionToThermal",
		"revealThermalTargetInGeometry",
		"geometrySelectionForTarget",
		"preferredGeometrySemanticOccurrence",
	} {
		if !strings.Contains(geometry, required) {
			t.Fatalf("cross-view navigation acceptance missing %q", required)
		}
	}
	inspector := readTestFile(t, "frontend/src/js/views/thermal-topology-inspector.js")
	for _, required := range []string{
		`renderVariableTable`,
		`thermal-inspector-table`,
		`data-thermal-inspector-kind`,
		`activeHelpers.selectGeometry?.`,
		`"Multiplier"`,
	} {
		if !strings.Contains(inspector, required) {
			t.Fatalf("inspector navigation acceptance missing %q", required)
		}
	}
	for _, removed := range []string{
		`data-inspector-`,
		`data-panel-action-menu`,
		`navigationAttributes`,
		`navigable-row`,
		`openSelectionInView(`,
		`renderInspectorActions`,
		`renderDiagnostics`,
		`renderZoneOutputSummary`,
		`Output requests`,
		`Diagnostics`,
		`inspectorSection("Actions"`,
		`thermal-inspector-actions`,
	} {
		if strings.Contains(inspector, removed) {
			t.Fatalf("removed inspector action remains %q", removed)
		}
	}
	selection := readTestFile(t, "frontend/src/js/selection-controller.js")
	if !strings.Contains(selection, "options.follow === undefined ? controllerState.semanticFollowSelection") || !strings.Contains(selection, "if (controllerState.semanticLinkMode && options.follow)") {
		t.Fatal("Follow ON/OFF selection behavior is not centralized")
	}
	thermalView := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	activation := sliceBetween(thermalView, "function activateGraphTarget", "function markGraphTargetSelected")
	for _, forbidden := range []string{"AnalyzeInput", "waitForAppAPI", "api.", "fetch("} {
		if strings.Contains(activation, forbidden) {
			t.Fatalf("topology navigation must not call backend analysis; found %q", forbidden)
		}
	}
}

func TestTOPO263NetworkAcceptance(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, required := range []string{
		"connectionAreaValue",
		"connectionUAValue",
		"thermal-edge-group navigable-row",
		"connectionAriaLabel",
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("Network acceptance missing %q", required)
		}
	}
	batch := readTestFile(t, "frontend/src/js/batch.js")
	for _, required := range []string{`state.metricGroup === "topology"`, "summaryDeltaRow", "deltaValue", "percentValue", "ExportBatchTopologyCSV"} {
		if !strings.Contains(batch, required) {
			t.Fatalf("topology batch delta-export acceptance missing %q", required)
		}
	}
}

func TestTOPO264SimulationUIRemoved(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	inspector := readTestFile(t, "frontend/src/js/views/thermal-topology-inspector.js")
	for _, removed := range []string{"simulated_heat", "thermalTopologySimulationPeriod", "metric-gain", "metric-loss", "Heat-flow ledger", "data-inspector-output-source"} {
		if strings.Contains(view, removed) || strings.Contains(inspector, removed) {
			t.Fatalf("removed simulation topology UI remains: %q", removed)
		}
	}
}
