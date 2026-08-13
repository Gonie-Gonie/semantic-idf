package frontendchecks

import (
	"strings"
	"testing"
)

func TestTopologyViewSwitchSupportsThermalAndNormalizesInvalidModes(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	for _, required := range []string{
		`data-topology-mode="3d"`,
		`data-topology-mode="plan"`,
		`data-topology-mode="thermal"`,
		`data-i18n-title="topology.thermalTooltip"`,
		`>Network</button>`,
		`id="thermalTopologyGraph"`,
		`id="thermalTopologyView"`,
		`id="topologyDetails"`,
		`id="thermalTopologyMetric"`,
		`id="thermalTopologyLayout"`,
	} {
		if !strings.Contains(index, required) {
			t.Fatalf("topology view switch is missing %q", required)
		}
	}
	for _, removed := range []string{
		`id="thermalTopologyGraphLevel"`,
		`value="boundary"`,
		`id="thermalTopologyAreaComponent"`,
		`value="opaque"`,
		`value="openings"`,
		`id="thermalTopologyAreaBasis"`,
		`id="thermalTopologyShowOpenings"`,
		`id="thermalTopologyFit"`,
		`id="geometrySelectionAid"`,
		`id="thermalTopologyScope"`,
		`id="thermalTopologyShowAirCoupling"`,
		`id="thermalTopologyExportJSON"`,
		`id="thermalTopologyInspector"`,
		`class="thermal-topology-advanced"`,
	} {
		if strings.Contains(index, removed) {
			t.Fatalf("fixed Zone/Gross topology controls still expose %q", removed)
		}
	}

	state := readTestFile(t, "frontend/src/js/state.js")
	for _, required := range []string{
		`Object.freeze(["3d", "plan", "thermal"])`,
		`return topologyModes.includes(mode) ? mode : "3d"`,
	} {
		if !strings.Contains(state, required) {
			t.Fatalf("topology mode normalization is missing %q", required)
		}
	}
}

func TestTopologyAllModesUseSingleLowerDetailsRenderer(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	if count := strings.Count(index, `id="topologyDetails"`); count != 1 {
		t.Fatalf("Topology must expose exactly one shared lower details host, got %d", count)
	}
	viewport := sliceBetween(index, `id="topologyViewport"`, `id="topologyDetailsSplitter"`)
	if strings.Contains(viewport, `id="topologyDetails"`) {
		t.Fatal("shared Topology details must remain below the viewport splitter")
	}
	for _, removed := range []string{`id="thermalTopologyInspector"`, `class="thermal-topology-inspector"`} {
		if strings.Contains(index, removed) {
			t.Fatalf("Network still exposes its removed side inspector %q", removed)
		}
	}

	state := readTestFile(t, "frontend/src/js/state.js")
	if strings.Contains(state, "thermalTopologyInspector") {
		t.Fatal("removed Network side-inspector element remains in shared state")
	}

	view := readTestFile(t, "frontend/src/js/views/topology-view.js")
	for _, required := range []string{
		"function renderTopologyDetails(",
		"renderGeometryTopologyDetails(geometry, entity)",
		"module.renderThermalTopologyDetails(geometry, thermalTopologyHelpers())",
		`elements.topologyDetails.setAttribute("aria-labelledby", "topologyDetailsHeading")`,
		`id="topologyDetailsHeading"`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("shared lower details renderer is missing %q", required)
		}
	}

	network := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, required := range []string{
		`export { renderThermalTopologyDetails } from "./thermal-topology-details.js"`,
		`currentHelpers?.renderTopologyDetails?.()`,
	} {
		if !strings.Contains(network, required) {
			t.Fatalf("Network does not route selection into the shared lower details renderer: missing %q", required)
		}
	}
	for _, removed := range []string{"renderThermalTopologyInspector", "thermalTopologyInspector"} {
		if strings.Contains(network, removed) {
			t.Fatalf("Network renderer retains private inspector path %q", removed)
		}
	}

	details := readTestFile(t, "frontend/src/js/views/thermal-topology-details.js")
	for _, required := range []string{
		"export function renderThermalTopologyDetails",
		"const details = elements.topologyDetails",
		`details.setAttribute("aria-labelledby", "topologyDetailsHeading")`,
		`id="topologyDetailsHeading"`,
	} {
		if !strings.Contains(details, required) {
			t.Fatalf("Network detail presenter is not hosted by the shared lower panel: missing %q", required)
		}
	}
	for _, removed := range []string{"elements.thermalTopologyInspector", "thermalTopologyInspectorHeading"} {
		if strings.Contains(details, removed) {
			t.Fatalf("Network detail presenter retains private inspector contract %q", removed)
		}
	}

	styles := readTestFile(t, "frontend/src/styles/topology.css")
	for _, removed := range []string{".thermal-topology-inspector", ".topology-viewport.network-active .topology-viewport-actions", "minmax(220px, 27%)"} {
		if strings.Contains(styles, removed) {
			t.Fatalf("single-host Network layout retains side-inspector styling %q", removed)
		}
	}
}

func TestTopologyUsesCanonicalPhysicalGrossAreaWithSeparateMultiplier(t *testing.T) {
	state := readTestFile(t, "frontend/src/js/state.js")
	for _, removed := range []string{"thermalTopologyAreaBasis", "normalizeThermalTopologyAreaBasis"} {
		if strings.Contains(state, removed) {
			t.Fatalf("removed area-basis state remains %q", removed)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/topology-view.js")
	for _, required := range []string{
		`surface.physicalArea ?? surface.area`,
		`windowItem.physicalArea ?? windowItem.area`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("area basis rendering contract is missing %q", required)
		}
	}
	thermalView := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	if !strings.Contains(thermalView, `Number(connection?.physicalGrossArea)`) || strings.Contains(thermalView, `connection?.effectiveGrossArea`) {
		t.Fatal("thermal graph must render physical gross area as its single canonical basis")
	}
	details := readTestFile(t, "frontend/src/js/views/thermal-topology-details.js")
	for _, required := range []string{`renderVariableTable`, `thermal-detail-table`, `"Multiplier"`, `"Gross area"`, `areaMultiplier`} {
		if !strings.Contains(details, required) {
			t.Fatalf("separate multiplier table contract is missing %q", required)
		}
	}
}

func TestTopologyThermalRendererIsSplitAndLazyLoaded(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/topology-view.js")
	for _, required := range []string{
		`import("./thermal-topology-view.js")`,
		`renderThermalTopologyLazy(geometry)`,
		`export function topologySelectionForTarget`,
		`export function topologyNavigationAttributes`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("geometry renderer split is missing %q", required)
		}
	}
	for _, file := range []string{
		"frontend/src/js/views/thermal-topology-view.js",
		"frontend/src/js/views/thermal-topology-layout.js",
		"frontend/src/js/views/thermal-topology-details.js",
	} {
		if body := readTestFile(t, file); body == "" {
			t.Fatalf("thermal topology module %s is empty", file)
		}
	}
}

func TestTopologyToolbarSeparatesModeSpecificControls(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	for _, required := range []string{
		`id="topologyStoryControl"`,
		`id="topologyStorySelect"`,
		`id="topologySpatialControls"`,
		`id="topologyShowZones"`,
		`id="topologyShowSurfaces"`,
		`id="topologyShowOpenings"`,
		`id="thermalTopologyControls"`,
		`id="thermalTopologyMetric"`,
		`id="thermalTopologyLayout"`,
	} {
		if !strings.Contains(index, required) {
			t.Fatalf("topology toolbar is missing %q", required)
		}
	}
	for _, removed := range []string{
		`id="topology3DControls"`, `id="topologyPlanControls"`,
		`id="topology3DShowZones"`, `id="topologyPlanShowZones"`,
		`id="topologyPlanShowBoundaries"`, `id="thermalTopologyScope"`,
		`id="thermalTopologyShowAirCoupling"`, `id="thermalTopologyExportJSON"`,
		`id="thermalTopologyExpandExternalTargets"`, `class="thermal-topology-advanced"`,
		`id="topologySyncLocate"`, `topology.syncLocate`,
	} {
		if strings.Contains(index, removed) {
			t.Fatalf("simplified topology toolbar still exposes %q", removed)
		}
	}
	spatialControls := sliceBetween(index, `id="topologySpatialControls"`, `id="thermalTopologyControls"`)
	for _, required := range []string{`id="topologyShowZones"`, `id="topologyShowSurfaces"`, `id="topologyShowOpenings"`} {
		if !strings.Contains(spatialControls, required) {
			t.Fatalf("3D/Plan controls are missing %q", required)
		}
	}
	for _, forbidden := range []string{`id="thermalTopologyMetric"`, `id="thermalTopologyLayout"`} {
		if strings.Contains(spatialControls, forbidden) {
			t.Fatalf("3D/Plan controls contain Network option %q", forbidden)
		}
	}
	networkControls := sliceBetween(index, `id="thermalTopologyControls"`, `</div>`)
	for _, required := range []string{`id="thermalTopologyMetric"`, `id="thermalTopologyLayout"`} {
		if !strings.Contains(networkControls, required) {
			t.Fatalf("Network controls are missing %q", required)
		}
	}
	for _, forbidden := range []string{`id="topologyShowZones"`, `id="topologyShowSurfaces"`, `id="topologyShowOpenings"`} {
		if strings.Contains(networkControls, forbidden) {
			t.Fatalf("Network controls contain spatial option %q", forbidden)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/topology-view.js")
	for _, required := range []string{
		`const is3D = state.topologyMode === "3d"`,
		`const isPlan = state.topologyMode === "plan"`,
		`const isNetwork = state.topologyMode === "thermal"`,
		`elements.topologySpatialControls.hidden = isNetwork`,
		`elements.thermalTopologyControls.hidden = !isNetwork`,
		`elements.topologyStoryControl.hidden = false`,
		`state.topologyMode === "3d"`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("topology mode control visibility is missing %q", required)
		}
	}

	styles := readTestFile(t, "frontend/src/styles/topology.css")
	head := sliceBetween(styles, ".topology-head {", ".topology-head > #topologyStats")
	if !strings.Contains(head, "display: flex") {
		t.Fatal("Topology stats and controls must share one toolbar row")
	}
	tools := sliceBetween(styles, ".topology-tools {", ".topology-control-group")
	if !strings.Contains(tools, "flex-wrap: nowrap") {
		t.Fatal("Topology controls must remain on one toolbar row")
	}
	networkStyles := sliceBetween(styles, ".thermal-topology-controls {", ".topology-select-control")
	if !strings.Contains(networkStyles, "flex-wrap: nowrap") {
		t.Fatal("Network Metric/Layout controls must remain on one toolbar row")
	}

}

func TestTopologyLevelAllAndSpecificStoryApplyToEveryMode(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/topology-view.js")
	for _, required := range []string{
		"const allOption = `<option",
		`t("topology.allLevels")`,
		`if (state.selectedTopologyStory === "all")`,
		`const storyIndex = state.selectedTopologyStory`,
		`const matchesStory = (item) => storyIndex === "all" || item.storyIndex === storyIndex`,
		`return state.selectedTopologyStory === "all" || item.storyIndex === state.selectedTopologyStory`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("shared Level filtering contract is missing %q", required)
		}
	}
	for _, removed := range []string{
		`state.topologyMode === "plan" && state.selectedTopologyStory === "all"`,
		`state.selectedTopologyStory === "all" ? firstStoryIndex`,
		`state.thermalTopologyScope`,
	} {
		if strings.Contains(view, removed) {
			t.Fatalf("Level All still has a mode-specific override %q", removed)
		}
	}

	thermalView := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, required := range []string{
		`const level = state.selectedTopologyStory`,
		`storyIndex: level`,
	} {
		if !strings.Contains(thermalView, required) {
			t.Fatalf("Network Level filtering contract is missing %q", required)
		}
	}
	layout := readTestFile(t, "frontend/src/js/views/thermal-topology-layout.js")
	for _, required := range []string{
		`export function applyThermalTopologyLevel`,
		`if (storyIndex === "all")`,
		`Number(node.storyIndex) === Number(storyIndex)`,
	} {
		if !strings.Contains(layout, required) {
			t.Fatalf("Network Level model filtering is missing %q", required)
		}
	}
}

func TestTopologySelectionFocusKeepsOneHopAndDimsEverythingElse(t *testing.T) {
	focus := readTestFile(t, "frontend/src/js/topology-focus.js")
	for _, required := range []string{
		"export function createTopologyFocusContext",
		"seedNodeIDs",
		"relatedConnections",
		"addConnectionToFocus",
		"addParentZones",
		"counterpartSurfaceId",
	} {
		if !strings.Contains(focus, required) {
			t.Fatalf("shared one-hop focus resolver is missing %q", required)
		}
	}

	spatial := readTestFile(t, "frontend/src/js/views/topology-view.js")
	for _, required := range []string{
		`import { createTopologyFocusContext } from "../topology-focus.js"`,
		"currentTopologyFocusContext",
		"geometryRenderableInFocus",
		`shape.classList.toggle("connected"`,
		`shape.classList.toggle("dimmed"`,
		`object.userData.baseOpacity * 0.12`,
	} {
		if !strings.Contains(spatial, required) {
			t.Fatalf("3D/Plan focus rendering is missing %q", required)
		}
	}

	network := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, required := range []string{
		"createThermalRenderFocusContext",
		"thermalTopologyFocusContext",
		`dimmed ? "dimmed"`,
		`connected && !selected ? " connected"`,
	} {
		if !strings.Contains(network, required) {
			t.Fatalf("Network focus rendering is missing %q", required)
		}
	}
	styles := readTestFile(t, "frontend/src/styles/topology.css")
	for _, required := range []string{
		`.thermal-edge-group.dimmed`,
		`.thermal-node.dimmed`,
		`.topology-plan :is(.plan-zone, .plan-surface, .plan-wall, .plan-window).dimmed`,
		`opacity: 0.12`,
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("focus dimming style is missing %q", required)
		}
	}
}

func TestTopologyVisibilityStateAndRenderingAreSharedBySpatialModes(t *testing.T) {
	state := readTestFile(t, "frontend/src/js/state.js")
	for _, required := range []string{
		"topologyVisibility:",
		"zones: true",
		"surfaces: true",
		"openings: true",
	} {
		if !strings.Contains(state, required) {
			t.Fatalf("shared topology visibility state is missing %q", required)
		}
	}
	for _, removed := range []string{"topology3DVisibility", "topologyPlanVisibility", "geometrySelectionAid", "geometryShowZones", "geometryShowWalls", "geometryShowWindows"} {
		if strings.Contains(state, removed) {
			t.Fatalf("legacy shared geometry state remains %q", removed)
		}
	}

	main := readTestFile(t, "frontend/src/js/main.js")
	for _, required := range []string{
		`bindTopologyVisibilityControl(elements.topologyShowZones, "zones")`,
		`bindTopologyVisibilityControl(elements.topologyShowSurfaces, "surfaces")`,
		`bindTopologyVisibilityControl(elements.topologyShowOpenings, "openings")`,
		`state.topologyVisibility = {`,
	} {
		if !strings.Contains(main, required) {
			t.Fatalf("shared topology visibility binding is missing %q", required)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/topology-view.js")
	scene := sliceBetween(view, "function renderScene", "function ensureRenderer")
	for _, required := range []string{
		`const visibility = state.topologyVisibility || {}`,
		`visibility.zones !== false`,
		`visibility.surfaces !== false`,
		`visibility.openings !== false`,
	} {
		if !strings.Contains(scene, required) {
			t.Fatalf("3D visibility renderer is missing %q", required)
		}
	}
	plan := sliceBetween(view, "function renderPlan", "function cachedGeometryPlanLayout")
	for _, required := range []string{
		`const visibility = state.topologyVisibility || {}`,
		`visibility.zones !== false`,
		`visibility.surfaces !== false`,
		`visibility.openings !== false`,
	} {
		if !strings.Contains(plan, required) {
			t.Fatalf("Plan visibility renderer is missing %q", required)
		}
	}
}

func TestTopologyViewportOwnsFitAndExpandIconActions(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	viewport := sliceBetween(index, `id="topologyViewport"`, `id="topologyDetailsSplitter"`)
	for _, required := range []string{
		`class="viewport-action-tools topology-viewport-actions"`,
		`id="topologyFitButton"`,
		`id="topologyExpandButton"`,
		`data-expand-pane="topology"`,
		`class="viewport-icon`,
		`aria-hidden="true"`,
		`class="sr-only"`,
	} {
		if !strings.Contains(viewport, required) {
			t.Fatalf("topology viewport action icons are missing %q", required)
		}
	}

	main := readTestFile(t, "frontend/src/js/main.js")
	for _, required := range []string{
		`elements.topologyFitButton.addEventListener("click", () => void fitTopologyView())`,
		`updateExpandButton(elements.topologyExpandButton, "topology")`,
		`button.setAttribute("aria-pressed", String(active))`,
		`button.setAttribute("aria-label", label)`,
	} {
		if !strings.Contains(main, required) {
			t.Fatalf("topology viewport action behavior is missing %q", required)
		}
	}
	thermal := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, removed := range []string{"idfAnalyzer:thermalTopologyFit", "thermalTopologyFit", "thermalTopologyShowOpenings"} {
		if strings.Contains(index+main+thermal, removed) {
			t.Fatalf("legacy Network-only viewport control remains %q", removed)
		}
	}
}

func TestTopologyNetworkHasNoMatrixOrEdgeLabels(t *testing.T) {
	content := readTestFile(t, "frontend/src/index.html") + readTestFile(t, "frontend/src/js/views/thermal-topology-view.js") + readTestFile(t, "frontend/src/styles/topology.css")
	for _, removed := range []string{"thermalTopologyMatrix", "thermal-matrix", "data-thermal-topology-display", "thermal-edge-label", `} surfaces`} {
		if strings.Contains(content, removed) {
			t.Fatalf("Network view still contains removed matrix or edge-label feature %q", removed)
		}
	}
}

func TestTopologySVGRendererUsesPortsPanZoomAndLayoutCache(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, required := range []string{
		`preserveAspectRatio="xMidYMid meet"`,
		`class="thermal-topology-panzoom"`,
		`new ResizeObserver`,
		`svg.addEventListener("wheel"`,
		`svg.addEventListener("pointerdown"`,
		`state.thermalTopologyLayoutCache.get(cacheKey)`,
		`data-thermal-node-id=`,
		`rerouteThermalTopologyEdges(currentModel, currentLayout)`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("thermal SVG renderer is missing %q", required)
		}
	}
	for _, removed := range []string{"expandConnection", "collapseBoundaryGraph", "data-topology-back"} {
		if strings.Contains(view, removed) {
			t.Fatalf("fixed Zone graph still contains boundary drill-down %q", removed)
		}
	}
	layout := readTestFile(t, "frontend/src/js/views/thermal-topology-layout.js")
	for _, required := range []string{
		`export function computeSpatialLayout`,
		`export function computeNetworkLayout`,
		`export function routeThermalEdge`,
		`export function rerouteThermalTopologyEdges`,
		`resolveNodeCollisions`,
		`barycentricOrder`,
		`adiabaticStub: true`,
	} {
		if !strings.Contains(layout, required) {
			t.Fatalf("thermal layout/routing module is missing %q", required)
		}
	}
	if strings.Contains(layout, "neighborDepth") {
		t.Fatal("one-hop topology scope retains the removed neighbor-depth option")
	}
}

func TestTopologyMetricModesAndInspectorExposeRequiredContracts(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	for _, removed := range []string{`id="thermalTopologyAreaComponent"`, `value="opaque"`, `value="openings"`, `id="thermalTopologyAreaBasis"`} {
		if strings.Contains(index, removed) {
			t.Fatalf("fixed Gross area mode still exposes %q", removed)
		}
	}
	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, required := range []string{
		`edgeMetricPresentation`,
		`connectionAreaValue`,
		`connectionUAValue`,
		`metric-na`,
		`qa-observation`,
		`air-emphasis`,
		`zoneSignatures.find`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("thermal metric renderer is missing %q", required)
		}
	}
	layout := readTestFile(t, "frontend/src/js/views/thermal-topology-layout.js")
	for _, required := range []string{`qaObservationConnections`, `allOpenings`} {
		if !strings.Contains(layout, required) {
			t.Fatalf("fixed Gross topology layout is missing %q", required)
		}
	}
	for _, removed := range []string{"graphLevel", "areaComponent", "areaField", "neighborDepth", "computeBoundaryLayout", "createBoundaryDetailModel", "detailConnection"} {
		if strings.Contains(layout, removed) || strings.Contains(view, removed) {
			t.Fatalf("fixed Zone/Gross topology renderer retains dead option path %q", removed)
		}
	}
	details := readTestFile(t, "frontend/src/js/views/thermal-topology-details.js")
	for _, required := range []string{
		`renderZoneDetails`,
		`renderConnectionDetails`,
		`renderBoundaryDetails`,
		`renderOpeningDetails`,
		`renderAirCouplingDetails`,
		`renderEnvironmentDetails`,
		`renderVariableTable`,
		`thermal-detail-table`,
		`data-thermal-detail-kind`,
		`"Multiplier"`,
	} {
		if !strings.Contains(details, required) {
			t.Fatalf("thermal details are missing %q", required)
		}
	}
	for _, removed := range []string{
		`data-inspector-`,
		`renderInspectorActions`,
		`renderDiagnostics`,
		`renderZoneOutputSummary`,
		`Output requests`,
		`Diagnostics`,
		`detailSection("Actions"`,
		`thermal-detail-actions`,
		`Model total`,
		`Physical / model`,
		`Gross /`,
		`Opaque /`,
	} {
		if strings.Contains(details, removed) {
			t.Fatalf("removed details concept remains %q", removed)
		}
	}
}

func TestTopologyModeHistoryPreservesSharedSelection(t *testing.T) {
	loader := readTestFile(t, "frontend/src/js/topology-loader.js")
	setter := sliceBetween(loader, "export function setTopologyMode", "export function setTopologyStory")
	for _, required := range []string{"normalizeTopologyMode(mode)", "recordViewHistory()", "state.topologyMode = nextMode"} {
		if !strings.Contains(setter, required) {
			t.Fatalf("geometry mode setter is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"selectedGeometryId =", "selectedGeometryKind =", "globalSelection =", "analyze(", "backend.",
		"selectedTopologyEntityId =", "selectedTopologyEntityKind =",
		"thermalTopologySelectedEntityId =", "thermalTopologySelectedEntityKind =",
	} {
		if strings.Contains(setter, forbidden) {
			t.Fatalf("mode switching must preserve the shared selection; found %q", forbidden)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/topology-view.js")
	restore := sliceBetween(view, "export async function restoreTopologyNavigationContext", "export function preferredTopologySemanticOccurrence")
	for _, required := range []string{
		`state.topologyMode = normalizeTopologyMode(snapshot.mode)`,
		`state.selectedTopologyEntityKind = normalizeGeometryKind(snapshot.selectedKind)`,
		`state.selectedTopologyEntityId = String(snapshot.selectedId || "")`,
		`restoreThermalTopologyState(snapshot, state)`,
		`renderTopologyView()`,
	} {
		if !strings.Contains(restore, required) {
			t.Fatalf("topology history restore is missing %q", required)
		}
	}
}
