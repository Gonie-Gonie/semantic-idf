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
		"visibility",
	} {
		if !strings.Contains(loader, required) {
			t.Fatalf("topology loader navigation adapter is missing %q", required)
		}
	}
}

func TestTopologySyncLocateIsAlwaysOnWithoutUserStateOrUI(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	state := readTestFile(t, "frontend/src/js/state.js")
	settings := readTestFile(t, "frontend/src/js/settings-client.js")
	settingsPage := readTestFile(t, "frontend/src/settings.html")
	main := readTestFile(t, "frontend/src/js/main.js")
	loader := readTestFile(t, "frontend/src/js/topology-loader.js")
	view := readTestFile(t, "frontend/src/js/views/topology-view.js")
	thermalView := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	details := readTestFile(t, "frontend/src/js/views/thermal-topology-details.js")
	i18n := readTestFile(t, "frontend/src/js/i18n.js")

	selection := sliceBetween(view, "export async function selectTopologyEntity", "export async function revealTopologySelection")
	for _, required := range []string{"selectSemanticEntity(selection", `originView: "topology"`, `recordHistory: options.recordHistory !== false`} {
		if !strings.Contains(selection, required) {
			t.Fatalf("always-on topology/input synchronization is missing %q", required)
		}
	}
	for _, forbidden := range []string{"options.syncLocate", "state.topologySyncLocate", "topologySyncLocate", "geometrySyncLocate", "topology.syncLocate", "topology.syncOn", "topology.syncOff", "behavior.topologySync"} {
		if strings.Contains(index+state+settings+settingsPage+main+loader+view+thermalView+details+i18n, forbidden) {
			t.Fatalf("always-on Sync locate still exposes user state or UI %q", forbidden)
		}
	}

	allGeometryUI := index + state + settingsPage + main + loader + view
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
		"selectSemanticEntity(selection",
		`originView: "topology"`,
		"recordHistory: options.recordHistory !== false",
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
