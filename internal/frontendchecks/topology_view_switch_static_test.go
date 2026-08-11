package frontendchecks

import (
	"strings"
	"testing"
)

func TestTopologyViewSwitchSupportsThermalAndNormalizesInvalidModes(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	for _, required := range []string{
		`data-geometry-mode="3d"`,
		`data-geometry-mode="plan"`,
		`data-geometry-mode="thermal"`,
		`data-i18n-title="topology.thermalTooltip"`,
		`>Network</button>`,
		`id="thermalTopologyGraph"`,
		`id="thermalTopologyView"`,
		`id="thermalTopologyInspector"`,
		`id="thermalTopologyMetric"`,
		`id="thermalTopologyScope"`,
		`id="thermalTopologyAdvanced"`,
	} {
		if !strings.Contains(index, required) {
			t.Fatalf("topology view switch is missing %q", required)
		}
	}
	for _, removed := range []string{`id="thermalTopologyGraphLevel"`, `value="boundary"`, `id="thermalTopologyAreaComponent"`, `value="opaque"`, `value="openings"`, `id="thermalTopologyAreaBasis"`} {
		if strings.Contains(index, removed) {
			t.Fatalf("fixed Zone/Gross topology controls still expose %q", removed)
		}
	}

	state := readTestFile(t, "frontend/src/js/state.js")
	for _, required := range []string{
		`Object.freeze(["3d", "plan", "thermal"])`,
		`return geometryModes.includes(mode) ? mode : "3d"`,
	} {
		if !strings.Contains(state, required) {
			t.Fatalf("geometry mode normalization is missing %q", required)
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

	view := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	for _, required := range []string{
		`surface.physicalArea ?? surface.area`,
		`windowItem.physicalArea ?? windowItem.area`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("area basis rendering contract is missing %q", required)
		}
	}
	layout := readTestFile(t, "frontend/src/js/views/thermal-topology-layout.js")
	if !strings.Contains(layout, `areaField: "physicalGrossArea"`) || strings.Contains(layout, `effectiveGrossArea`) {
		t.Fatal("thermal layout must use physical gross area as its single canonical basis")
	}
	inspector := readTestFile(t, "frontend/src/js/views/thermal-topology-inspector.js")
	for _, required := range []string{`renderVariableTable`, `thermal-inspector-table`, `"Multiplier"`, `"Gross area"`, `areaMultiplier`} {
		if !strings.Contains(inspector, required) {
			t.Fatalf("separate multiplier table contract is missing %q", required)
		}
	}
}

func TestTopologyThermalRendererIsSplitAndLazyLoaded(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	for _, required := range []string{
		`import("./thermal-topology-view.js")`,
		`renderThermalTopologyLazy(geometry)`,
		`export function geometrySelectionForTarget`,
		`export function geometryNavigationAttributes`,
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

func TestTopologyToolbarSeparatesSpatialAndThermalControls(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	for _, required := range []string{
		`id="geometrySpatialControls"`,
		`id="thermalTopologyControls"`,
		`id="thermalTopologyLayout"`,
		`id="thermalTopologyShowOpenings"`,
		`id="thermalTopologyShowAirCoupling"`,
		`id="thermalTopologyExpandExternalTargets"`,
	} {
		if !strings.Contains(index, required) {
			t.Fatalf("topology toolbar is missing %q", required)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	for _, required := range []string{
		`elements.geometrySpatialControls.hidden = state.geometryMode === "thermal"`,
		`elements.thermalTopologyControls.hidden = state.geometryMode !== "thermal"`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("topology mode control visibility is missing %q", required)
		}
	}

	styles := readTestFile(t, "frontend/src/styles/geometry.css")
	if !strings.Contains(styles, `@media (max-width: 1280px)`) || !strings.Contains(styles, `.thermal-topology-advanced-menu`) {
		t.Fatal("thermal toolbar must collapse its advanced controls at narrow widths")
	}
}

func TestTopologyNetworkHasNoMatrixOrEdgeLabels(t *testing.T) {
	content := readTestFile(t, "frontend/src/index.html") + readTestFile(t, "frontend/src/js/views/thermal-topology-view.js") + readTestFile(t, "frontend/src/styles/geometry.css")
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
		`resolveNodeCollisions`,
		`barycentricOrder`,
		`Math.min(3, Math.max(1`,
	} {
		if !strings.Contains(layout, required) {
			t.Fatalf("thermal layout/routing module is missing %q", required)
		}
	}
}

func TestTopologyMetricModesAndInspectorExposeRequiredContracts(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	for _, removed := range []string{`id="thermalTopologyAreaComponent"`, `value="opaque"`, `value="openings"`, `id="thermalTopologyAreaBasis"`} {
		if strings.Contains(index, removed) {
			t.Fatalf("fixed Gross area mode still exposes %q", removed)
		}
	}
	state := readTestFile(t, "frontend/src/js/state.js")
	for _, required := range []string{`export function normalizeThermalTopologyGraphLevel()`, `return "zone";`, `export function normalizeThermalTopologyAreaComponent()`, `return "gross";`} {
		if !strings.Contains(state, required) {
			t.Fatalf("fixed Zone/Gross state contract is missing %q", required)
		}
	}
	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, required := range []string{
		`graphLevel: "zone"`,
		`areaComponent: "gross"`,
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
	for _, required := range []string{`areaField: "physicalGrossArea"`, `qaObservationConnections`} {
		if !strings.Contains(layout, required) {
			t.Fatalf("fixed Gross topology layout is missing %q", required)
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
	loader := readTestFile(t, "frontend/src/js/geometry-loader.js")
	setter := sliceBetween(loader, "export function setGeometryMode", "export function setGeometryStory")
	for _, required := range []string{"normalizeGeometryMode(mode)", "recordViewHistory()", "state.geometryMode = nextMode"} {
		if !strings.Contains(setter, required) {
			t.Fatalf("geometry mode setter is missing %q", required)
		}
	}
	for _, forbidden := range []string{"selectedGeometryId =", "selectedGeometryKind =", "globalSelection =", "analyze(", "backend."} {
		if strings.Contains(setter, forbidden) {
			t.Fatalf("mode switching must preserve the shared selection; found %q", forbidden)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	restore := sliceBetween(view, "export async function restoreGeometryNavigationContext", "export function preferredGeometrySemanticOccurrence")
	for _, required := range []string{
		`state.geometryMode = normalizeGeometryMode(snapshot.mode)`,
		`state.selectedGeometryKind = normalizeGeometryKind(snapshot.selectedKind)`,
		`state.selectedGeometryId = String(snapshot.selectedId || "")`,
	} {
		if !strings.Contains(restore, required) {
			t.Fatalf("topology history restore is missing %q", required)
		}
	}
}
