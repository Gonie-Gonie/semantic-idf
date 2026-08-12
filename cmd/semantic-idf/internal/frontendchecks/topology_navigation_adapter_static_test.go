package frontendchecks

import (
	"strings"
	"testing"
)

func TestTopologyAdapterRegistersBeforeLazyRenderer(t *testing.T) {
	loader := readTestFile(t, "frontend/src/js/topology-loader.js")
	for _, required := range []string{
		`configureResultPanelNavigationHooks("topology"`,
		"topologyViewTargetForSelection",
		"topologyTargetExists",
		"loadTopologyModule",
		"module.revealTopologySelection",
		"module.restoreTopologyNavigationContext",
		"preferredTopologyOccurrenceFromTarget",
		"context.genericPreferredSemanticOccurrence",
		"selectedKind",
		"selectedId",
		"syncLocate",
		"visibility",
	} {
		if !strings.Contains(loader, required) {
			t.Fatalf("topology loader navigation adapter is missing %q", required)
		}
	}
}

func TestTopologySyncLocateIsCommonAndSelectAidIsRemoved(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	state := readTestFile(t, "frontend/src/js/state.js")
	settings := readTestFile(t, "frontend/src/js/settings-client.js")
	main := readTestFile(t, "frontend/src/js/main.js")
	loader := readTestFile(t, "frontend/src/js/topology-loader.js")
	view := readTestFile(t, "frontend/src/js/views/topology-view.js")

	for _, required := range []string{
		`id="topologySyncLocate" type="checkbox" checked`,
		`topologySyncLocate: true`,
		`syncLocate: Boolean(state.topologySyncLocate)`,
		`state.topologySyncLocate = snapshot.syncLocate !== false`,
	} {
		if !strings.Contains(index+state+settings+loader+view, required) {
			t.Fatalf("common Sync locate contract is missing %q", required)
		}
	}
	modeVisibility := sliceBetween(view, "function updateModeVisibility", "function renderThermalTopologyLazy")
	if strings.Contains(modeVisibility, "geometrySyncControl") || strings.Contains(modeVisibility, "geometrySyncLocate") {
		t.Fatal("Sync locate must remain visible in 3D, Plan, and Network modes")
	}

	allGeometryUI := index + state + main + loader + view
	for _, removed := range []string{
		"geometrySelectionAid",
		"setGeometrySelectionAid",
		"selectionAid",
		"geometry.selectAid",
		`event.key.toLowerCase() === "h"`,
	} {
		if strings.Contains(allGeometryUI, removed) {
			t.Fatalf("removed Select aid contract remains %q", removed)
		}
	}
}

func TestGeometryItemsUseReverseMappedNavigationMarkup(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/topology-view.js")
	attributes := sliceBetween(content, "function preferredOccurrenceForTopologyTarget", "function normalizeGeometryKind")
	for _, required := range []string{
		"navigation.byViewTarget",
		"data-entity-id",
		"data-entity-kind",
		"data-occurrence-id",
		"data-occurrence-context",
		"data-semantic-path",
		"data-panel-target-id",
		"data-source-object-id",
		"data-source-object-index",
		"data-source-field-index",
		"aria-selected",
		"tabindex",
	} {
		if !strings.Contains(attributes, required) {
			t.Fatalf("geometry navigation markup is missing %q", required)
		}
	}
	for _, required := range []string{
		`topologyNavigationAttributes("zone"`,
		`topologyNavigationAttributes("surface"`,
		`topologyNavigationAttributes("fenestration"`,
		`topologyNavigationAttributes("story"`,
		"topologyNavigationAttributes(item.kind, item.id",
		`kind: "space"`,
		"relatedItemForSpace",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("geometry object navigation coverage is missing %q", required)
		}
	}
}

func TestTopologyBidirectionalRevealAndAtomicSelection(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/topology-view.js")
	selection := sliceBetween(content, "export async function selectTopologyEntity", "export async function revealTopologySelection")
	for _, required := range []string{
		"topologySelectionForTarget",
		"syncLocatedInputEntity(entity)",
		"selectSemanticEntity(selection",
		`originView: "topology"`,
		"recordHistory: syncLocate ? false",
		"rememberForOriginView",
	} {
		if !strings.Contains(selection, required) {
			t.Fatalf("geometry panel-to-semantic selection is missing %q", required)
		}
	}

	reveal := sliceBetween(content, "export async function revealTopologySelection", "export async function restoreTopologyNavigationContext")
	for _, required := range []string{
		"topologyViewTargetForSelection",
		"topologyTargetEntity",
		"owningZoneForGeometryEntity",
		"geometryStoryIndexForEntity",
		"geometryEntityHasPlanShape",
		"temporaryTopologyReveal",
		"baseSurfaceId",
		"state.selectedTopologyEntityKind",
		"state.selectedTopologyEntityId",
		"findTopologyNavigationTarget",
	} {
		if !strings.Contains(reveal, required) {
			t.Fatalf("semantic-to-geometry reveal is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"topology3DVisibility =",
		"topologyPlanVisibility =",
		"topology3DShowZones.checked =",
		"topologyPlanShowZones.checked =",
	} {
		if strings.Contains(reveal, forbidden) {
			t.Fatalf("semantic geometry reveal must materialize a target without changing visibility filters: found %q", forbidden)
		}
	}

	for _, required := range []string{
		`normalized === "fenestration" ? "window"`,
		"geometryRenderableMatchesSelection",
		"geometrySurfaceIsTemporarilyVisible",
		"geometryWindowIsTemporarilyVisible",
		"event.stopPropagation()",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("geometry exact-focus/selection behavior is missing %q", required)
		}
	}
}
