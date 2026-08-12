package frontendchecks

import (
	"strings"
	"testing"
)

func TestTOPO260ViewSwitchingAcceptance(t *testing.T) {
	loader := readTestFile(t, "frontend/src/js/topology-loader.js")
	modeSetter := sliceBetween(loader, "export function setTopologyMode", "export function setTopologyStory")
	for _, required := range []string{"normalizeTopologyMode(mode)", "recordViewHistory()", "state.topologyMode = nextMode"} {
		if !strings.Contains(modeSetter, required) {
			t.Fatalf("view switching contract missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"selectedGeometryId =", "selectedGeometryKind =", "AnalyzeInput", "api.",
		"selectedTopologyEntityId =", "selectedTopologyEntityKind =",
		"thermalTopologySelectedEntityId =", "thermalTopologySelectedEntityKind =",
	} {
		if strings.Contains(modeSetter, forbidden) {
			t.Fatalf("3D/Plan/Thermal switch must preserve selection and avoid analysis; found %q", forbidden)
		}
	}
	state := readTestFile(t, "frontend/src/js/state.js")
	if !strings.Contains(state, `return topologyModes.includes(mode) ? mode : "3d"`) {
		t.Fatal("invalid geometry modes must normalize to 3D")
	}
	view := readTestFile(t, "frontend/src/js/views/topology-view.js")
	for _, required := range []string{
		`elements.topologySpatialControls.hidden = isNetwork`,
		`elements.thermalTopologyControls.hidden = !isNetwork`,
		`elements.topologyStoryControl.hidden = false`,
		`state.topologyMode === "3d"`,
		`return state.selectedTopologyStory === "all" || item.storyIndex === state.selectedTopologyStory`,
		`storyIndex === "all" || item.storyIndex === storyIndex`,
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
		"adiabaticStub: true",
		"rerouteThermalTopologyEdges",
	} {
		if !strings.Contains(layout, required) {
			t.Fatalf("layout/routing acceptance missing %q", required)
		}
	}
	for _, removed := range []string{"expandExternalTargets", "expandExternalTargets:"} {
		if strings.Contains(layout, removed) {
			t.Fatalf("automatic Outdoor projection retains manual expansion path %q", removed)
		}
	}
	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, required := range []string{
		"isThermalPointNode",
		`environment-point`,
		`data-thermal-orientation=`,
		`thermal-node-endpoint`,
		`thermal-edge-group navigable-row${stub ? " adiabatic-stub" : ""}`,
		`thermal-edge-cap`,
		`const markerEnd = stub ? ""`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("directional Outdoor/Adiabatic renderer contract missing %q", required)
		}
	}
	styles := readTestFile(t, "frontend/src/styles/topology.css")
	for _, required := range []string{
		`.thermal-node.environment-point .thermal-node-endpoint`,
		`.thermal-node.environment-point[data-thermal-orientation="east"]`,
		`.thermal-node.environment-point[data-thermal-orientation="south"]`,
		`.thermal-node.environment-point[data-thermal-orientation="west"]`,
		`.thermal-node.environment-point[data-thermal-orientation="roof"]`,
		`.thermal-node.environment-point[data-thermal-orientation="floor"]`,
		`.thermal-edge-cap`,
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("directional Outdoor/Adiabatic styling contract missing %q", required)
		}
	}
	for _, removed := range []string{"expandConnection", "collapseBoundaryGraph", "data-topology-back"} {
		if strings.Contains(view, removed) {
			t.Fatalf("Zone-only Network still exposes boundary drill-down %q", removed)
		}
	}
	for _, removed := range []string{"graphLevel", "areaComponent", "areaField", "neighborDepth", "computeBoundaryLayout", "createBoundaryDetailModel", "detailConnection"} {
		if strings.Contains(layout, removed) || strings.Contains(view, removed) {
			t.Fatalf("Zone/Gross-only Network retains dead option path %q", removed)
		}
	}
}

func TestTOPO262NavigationAcceptance(t *testing.T) {
	geometry := readTestFile(t, "frontend/src/js/views/topology-view.js")
	for _, required := range []string{
		"projectGeometrySelectionToThermal",
		"revealThermalTargetInTopology",
		"topologySelectionForTarget",
		"preferredTopologySemanticOccurrence",
	} {
		if !strings.Contains(geometry, required) {
			t.Fatalf("cross-view navigation acceptance missing %q", required)
		}
	}
	details := readTestFile(t, "frontend/src/js/views/thermal-topology-details.js")
	for _, required := range []string{
		`renderVariableTable`,
		`thermal-detail-table`,
		`data-thermal-detail-kind`,
		`activeHelpers.selectTopologyEntity?.`,
		`"Multiplier"`,
	} {
		if !strings.Contains(details, required) {
			t.Fatalf("details navigation acceptance missing %q", required)
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
		`detailSection("Actions"`,
		`thermal-detail-actions`,
	} {
		if strings.Contains(details, removed) {
			t.Fatalf("removed detail action remains %q", removed)
		}
	}
	selection := readTestFile(t, "frontend/src/js/selection-controller.js")
	if !strings.Contains(selection, "options.follow === undefined ? controllerState.semanticFollowSelection") || !strings.Contains(selection, "if (controllerState.semanticLinkMode && options.follow)") {
		t.Fatal("Follow ON/OFF selection behavior is not centralized")
	}
	thermalView := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	activation := sliceBetween(thermalView, "function activateGraphTarget", "function applyGraphTransform")
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
	batch := readTestFile(t, "frontend/src/js/tools.js")
	for _, required := range []string{`state.metricGroup === "topology"`, "metricsDeltaRow", "deltaValue", "percentValue", "ExportBatchTopologyCSV"} {
		if !strings.Contains(batch, required) {
			t.Fatalf("topology batch delta-export acceptance missing %q", required)
		}
	}
}

func TestTOPO264SimulationUIRemoved(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	details := readTestFile(t, "frontend/src/js/views/thermal-topology-details.js")
	for _, removed := range []string{"simulated_heat", "thermalTopologySimulationPeriod", "metric-gain", "metric-loss", "Heat-flow ledger", "data-inspector-output-source"} {
		if strings.Contains(view, removed) || strings.Contains(details, removed) {
			t.Fatalf("removed simulation topology UI remains: %q", removed)
		}
	}
}
