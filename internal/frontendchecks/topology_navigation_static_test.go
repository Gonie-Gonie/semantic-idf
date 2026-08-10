package frontendchecks

import (
	"strings"
	"testing"
)

func TestThermalTopologyTargetResolverCoversSemanticTargetKinds(t *testing.T) {
	resolver := readTestFile(t, "frontend/src/js/thermal-topology-targets.js")
	for _, required := range []string{
		`"thermal_boundary"`,
		`"thermal_interface"`,
		`"thermal_connection"`,
		`"thermal_environment"`,
		`"thermal_air_coupling"`,
		`"thermal_issue"`,
		`"thermal_observation"`,
		"thermalTopologyObservationID",
		"resolveThermalTopologyTarget",
		"surfaceIds",
		"windowIds",
		"nodeIds",
		"boundaryIds",
		"airCouplingIds",
	} {
		if !strings.Contains(resolver, required) {
			t.Fatalf("thermal topology target resolver is missing %q", required)
		}
	}

	loader := readTestFile(t, "frontend/src/js/geometry-loader.js")
	for _, required := range []string{"isThermalTopologyTargetKind", "thermalTopologyTargetExists"} {
		if !strings.Contains(loader, required) {
			t.Fatalf("lazy geometry adapter is missing topology target support %q", required)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	for _, required := range []string{
		"resolveThermalTopologyTarget",
		`state.geometryMode = "thermal"`,
		"surfaceIds",
		"windowIds",
		"nodeIds",
		"thermalOccurrenceContextPriority",
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("geometry view is missing topology reveal behavior %q", required)
		}
	}
}

func TestThermalSemanticOccurrencePriorityIsContextAware(t *testing.T) {
	for _, path := range []string{
		"frontend/src/js/geometry-loader.js",
		"frontend/src/js/panel-navigation-adapters.js",
		"frontend/src/js/selection-controller.js",
		"frontend/src/js/views/geometry-view.js",
	} {
		content := readTestFile(t, path)
		for _, required := range []string{
			`context === "thermal_connection_context"`,
			`context === "surface_boundary_context"`,
			`context === "zone_geometry"`,
			`context === "definition"`,
		} {
			if !strings.Contains(content, required) {
				t.Fatalf("%s is missing semantic occurrence priority %q", path, required)
			}
		}
	}
}

func TestFrontendReadinessIncludesThermalTopologyResolver(t *testing.T) {
	build := readTestFile(t, "scripts/frontend-build.ps1")
	if !strings.Contains(build, `"thermal-topology-targets.js"`) {
		t.Fatal("frontend readiness manifest is missing thermal-topology-targets.js")
	}
}

func TestThermalTopologyStateNormalizesAndInvalidatesDocumentContext(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/state.js")
	for _, required := range []string{
		`thermalTopologyGraphLevel: "zone"`,
		`thermalTopologyMetric: "topology"`,
		`thermalTopologyScope: "building"`,
		`thermalTopologyLayout: "spatial"`,
		`thermalTopologyAreaBasis: "effective"`,
		"thermalTopologyShowOpenings: true",
		"thermalTopologyShowAirCoupling: false",
		"thermalTopologyExpandExternalTargets: false",
		`thermalTopologySelectedEntityId: ""`,
		`thermalTopologySelectedEntityKind: ""`,
		"thermalTopologyNeighborDepth: 1",
		"thermalTopologyPanX: 0",
		"thermalTopologyPanY: 0",
		"thermalTopologyScale: 1",
		"thermalTopologyLayoutCache: new Map()",
		"normalizeThermalTopologyGraphLevel",
		"normalizeThermalTopologyMetric",
		"normalizeThermalTopologyScope",
		"normalizeThermalTopologyLayout",
		"normalizeThermalTopologyAreaBasis",
		"normalizeThermalTopologyState",
		"resetThermalTopologyDocumentState",
		"target.thermalTopologyLayoutCache?.clear?.()",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("thermal topology state contract is missing %q", required)
		}
	}

	main := readTestFile(t, "frontend/src/js/main.js")
	documentChange := sliceBetween(main, `window.addEventListener("idfAnalyzer:documentChanged"`, `window.addEventListener("idfAnalyzer:geometryLocate"`)
	if !strings.Contains(documentChange, "resetThermalTopologyDocumentState(state)") {
		t.Fatal("document change does not invalidate topology selection/layout state")
	}
}

func TestThermalTopologyHistoryCapturesContextWithoutGraphOrLayoutCache(t *testing.T) {
	stateContent := readTestFile(t, "frontend/src/js/state.js")
	capture := sliceBetween(stateContent, "export function captureThermalTopologyState", "export function restoreThermalTopologyState")
	for _, required := range []string{
		"thermalTopologyGraphLevel",
		"thermalTopologyMetric",
		"thermalTopologyScope",
		"thermalTopologyLayout",
		"thermalTopologyAreaBasis",
		"thermalTopologyShowOpenings",
		"thermalTopologyShowAirCoupling",
		"thermalTopologyExpandExternalTargets",
		"thermalTopologySelectedEntityId",
		"thermalTopologySelectedEntityKind",
		"thermalTopologyNeighborDepth",
		"thermalTopologyPanX",
		"thermalTopologyPanY",
		"thermalTopologyScale",
	} {
		if !strings.Contains(capture, required) {
			t.Fatalf("thermal history snapshot is missing %q", required)
		}
	}
	for _, forbidden := range []string{"thermalTopologyLayoutCache", "connections", "boundaries", "nodes", "matrix"} {
		if strings.Contains(capture, forbidden) {
			t.Fatalf("thermal history snapshot must not retain graph/cache data %q", forbidden)
		}
	}

	loader := readTestFile(t, "frontend/src/js/geometry-loader.js")
	if !strings.Contains(loader, "...captureThermalTopologyState(state)") {
		t.Fatal("geometry panel history does not capture thermal topology context")
	}
	view := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	restore := sliceBetween(view, "export async function restoreGeometryNavigationContext", "export function preferredGeometrySemanticOccurrence")
	if !strings.Contains(restore, "restoreThermalTopologyState(snapshot, state)") {
		t.Fatal("geometry history restore does not restore thermal topology context")
	}

	history := readTestFile(t, "frontend/src/js/view-history.js")
	if !strings.Contains(history, `analysisKey: state.reportAnalysisKey || state.analysisKey || ""`) {
		t.Fatal("view history does not retain the analysis cache key")
	}
	navigation := readTestFile(t, "frontend/src/js/navigation.js")
	restoreSnapshot := sliceBetween(navigation, "export async function restoreViewSnapshot", "function restoreSemanticSnapshotState")
	for _, required := range []string{"analysisCacheHit", "remapSemanticSelection", "restoreRegisteredPanelContext"} {
		if !strings.Contains(restoreSnapshot, required) {
			t.Fatalf("history restore is missing cache-aware selection remap %q", required)
		}
	}
}

func TestThermalTopologySharesSelectionAndHoverNavigation(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, required := range []string{
		`currentHelpers?.selectGeometry?.(kind, id`,
		`originView: "geometry"`,
		`recordHistory: false`,
		`follow: false`,
		`data-thermal-target-kind="${targetKind}"`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("thermal selection/hover contract is missing %q", required)
		}
	}
	geometry := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	for _, required := range []string{
		`projectGeometrySelectionToThermal`,
		`revealThermalTargetInGeometry`,
		`idfAnalyzer:semanticHoverChanged`,
		`geometryRenderableMatchesHover`,
		`hoverSemanticEntity(selection`,
		`clearSemanticHover`,
	} {
		if !strings.Contains(geometry, required) {
			t.Fatalf("3D/Plan thermal projection contract is missing %q", required)
		}
	}
	styles := readTestFile(t, "frontend/src/styles/geometry.css")
	for _, required := range []string{`.semantic-hovered`, `.thermal-edge-group`} {
		if !strings.Contains(styles, required) {
			t.Fatalf("thermal graph/hover styling is missing %q", required)
		}
	}
}
