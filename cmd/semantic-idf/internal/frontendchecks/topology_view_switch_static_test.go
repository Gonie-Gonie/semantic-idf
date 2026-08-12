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
		`id="thermalTopologyInspector"`,
		`id="thermalTopologyMetric"`,
		`id="thermalTopologyScope"`,
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
	inspector := readTestFile(t, "frontend/src/js/views/thermal-topology-inspector.js")
	for _, required := range []string{`renderVariableTable`, `thermal-inspector-table`, `"Multiplier"`, `"Gross area"`, `areaMultiplier`} {
		if !strings.Contains(inspector, required) {
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
		"frontend/src/js/views/thermal-topology-inspector.js",
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
		`id="topologySyncLocate" type="checkbox" checked`,
		`id="topology3DControls"`,
		`id="topology3DShowZones"`,
		`id="topology3DShowSurfaces"`,
		`id="topology3DShowOpenings"`,
		`id="topologyPlanControls"`,
		`id="topologyPlanShowZones"`,
		`id="topologyPlanShowBoundaries"`,
		`id="topologyPlanShowOpenings"`,
		`id="thermalTopologyControls"`,
		`id="thermalTopologyLayout"`,
		`id="thermalTopologyShowAirCoupling"`,
	} {
		if !strings.Contains(index, required) {
			t.Fatalf("topology toolbar is missing %q", required)
		}
	}
	if strings.Contains(index, `id="thermalTopologyExpandExternalTargets"`) {
		t.Fatal("automatic directional Outdoor projection still exposes a manual external-target toggle")
	}

	view := readTestFile(t, "frontend/src/js/views/topology-view.js")
	for _, required := range []string{
		`const is3D = state.topologyMode === "3d"`,
		`const isPlan = state.topologyMode === "plan"`,
		`const isNetwork = state.topologyMode === "thermal"`,
		`elements.topology3DControls.hidden = !is3D`,
		`elements.topologyPlanControls.hidden = !isPlan`,
		`elements.thermalTopologyControls.hidden = !isNetwork`,
		`elements.topologyStoryControl.hidden = isNetwork && state.thermalTopologyScope !== "story"`,
		`state.topologyMode === "3d"`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("topology mode control visibility is missing %q", required)
		}
	}

	styles := readTestFile(t, "frontend/src/styles/topology.css")
	if !strings.Contains(styles, `@media (max-width: 1280px)`) || !strings.Contains(styles, `.thermal-topology-advanced-menu`) {
		t.Fatal("thermal toolbar must collapse its advanced controls at narrow widths")
	}
}

func TestTopologyVisibilityStateAndRenderingAreIndependentByMode(t *testing.T) {
	state := readTestFile(t, "frontend/src/js/state.js")
	for _, required := range []string{
		"topology3DVisibility:",
		"zones: true",
		"surfaces: true",
		"openings: true",
		"topologyPlanVisibility:",
		"boundaries: true",
	} {
		if !strings.Contains(state, required) {
			t.Fatalf("mode-specific topology visibility state is missing %q", required)
		}
	}
	for _, removed := range []string{"geometrySelectionAid", "geometryShowZones", "geometryShowWalls", "geometryShowWindows"} {
		if strings.Contains(state, removed) {
			t.Fatalf("legacy shared geometry state remains %q", removed)
		}
	}

	main := readTestFile(t, "frontend/src/js/main.js")
	for _, required := range []string{
		`bindTopologyVisibilityControl(elements.topology3DShowZones, "topology3DVisibility", "zones")`,
		`bindTopologyVisibilityControl(elements.topology3DShowSurfaces, "topology3DVisibility", "surfaces")`,
		`bindTopologyVisibilityControl(elements.topology3DShowOpenings, "topology3DVisibility", "openings")`,
		`bindTopologyVisibilityControl(elements.topologyPlanShowZones, "topologyPlanVisibility", "zones")`,
		`bindTopologyVisibilityControl(elements.topologyPlanShowBoundaries, "topologyPlanVisibility", "boundaries")`,
		`bindTopologyVisibilityControl(elements.topologyPlanShowOpenings, "topologyPlanVisibility", "openings")`,
	} {
		if !strings.Contains(main, required) {
			t.Fatalf("mode-specific topology visibility binding is missing %q", required)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/topology-view.js")
	scene := sliceBetween(view, "function renderScene", "function ensureRenderer")
	for _, required := range []string{
		`const visibility = state.topology3DVisibility || {}`,
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
		`const visibility = state.topologyPlanVisibility || {}`,
		`visibility.zones !== false`,
		`visibility.boundaries !== false`,
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
	inspector := readTestFile(t, "frontend/src/js/views/thermal-topology-inspector.js")
	for _, required := range []string{
		`renderZoneDetails`,
		`renderConnectionDetails`,
		`renderBoundaryDetails`,
		`renderOpeningDetails`,
		`renderAirCouplingDetails`,
		`renderEnvironmentDetails`,
		`renderVariableTable`,
		`thermal-inspector-table`,
		`data-thermal-inspector-kind`,
		`"Multiplier"`,
	} {
		if !strings.Contains(inspector, required) {
			t.Fatalf("thermal inspector is missing %q", required)
		}
	}
	for _, removed := range []string{
		`data-inspector-`,
		`renderInspectorActions`,
		`renderDiagnostics`,
		`renderZoneOutputSummary`,
		`Output requests`,
		`Diagnostics`,
		`inspectorSection("Actions"`,
		`thermal-inspector-actions`,
		`Model total`,
		`Physical / model`,
		`Gross /`,
		`Opaque /`,
	} {
		if strings.Contains(inspector, removed) {
			t.Fatalf("removed inspector concept remains %q", removed)
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
	for _, forbidden := range []string{"selectedGeometryId =", "selectedGeometryKind =", "globalSelection =", "analyze(", "backend."} {
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
	} {
		if !strings.Contains(restore, required) {
			t.Fatalf("topology history restore is missing %q", required)
		}
	}
}
