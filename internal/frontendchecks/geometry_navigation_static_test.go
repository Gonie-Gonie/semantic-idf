package frontendchecks

import (
	"strings"
	"testing"
)

func TestGeometryAdapterRegistersBeforeLazyRenderer(t *testing.T) {
	loader := readTestFile(t, "frontend/src/js/geometry-loader.js")
	for _, required := range []string{
		`configureResultPanelNavigationHooks("geometry"`,
		"geometryViewTargetForSelection",
		"geometryTargetExists",
		"loadGeometryModule",
		"module.revealGeometrySelection",
		"module.restoreGeometryNavigationContext",
		"preferredGeometryOccurrenceFromTarget",
		"context.genericPreferredSemanticOccurrence",
		"selectedKind",
		"selectedId",
		"selectionAid",
		"syncLocate",
		"visibility",
	} {
		if !strings.Contains(loader, required) {
			t.Fatalf("geometry loader navigation adapter is missing %q", required)
		}
	}
}

func TestGeometryItemsUseReverseMappedNavigationMarkup(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	attributes := sliceBetween(content, "function preferredOccurrenceForGeometryTarget", "function normalizeGeometryKind")
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
		`geometryNavigationAttributes("zone"`,
		`geometryNavigationAttributes("surface"`,
		`geometryNavigationAttributes("fenestration"`,
		`geometryNavigationAttributes("story"`,
		"geometryNavigationAttributes(item.kind, item.id",
		`kind: "space"`,
		"relatedItemForSpace",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("geometry object navigation coverage is missing %q", required)
		}
	}
}

func TestGeometryBidirectionalRevealAndAtomicSelection(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	selection := sliceBetween(content, "export async function selectGeometry", "export async function revealGeometrySelection")
	for _, required := range []string{
		"geometrySelectionForTarget",
		"syncLocatedInputEntity(entity)",
		"selectSemanticEntity(selection",
		`originView: "geometry"`,
		"recordHistory: syncLocate ? false",
		"rememberForOriginView",
	} {
		if !strings.Contains(selection, required) {
			t.Fatalf("geometry panel-to-semantic selection is missing %q", required)
		}
	}

	reveal := sliceBetween(content, "export async function revealGeometrySelection", "export async function restoreGeometryNavigationContext")
	for _, required := range []string{
		"geometryViewTargetForSelection",
		"geometryTargetEntity",
		"owningZoneForGeometryEntity",
		"geometryStoryIndexForEntity",
		"geometryEntityHasPlanShape",
		"temporaryGeometryReveal",
		"baseSurfaceId",
		"state.selectedGeometryKind",
		"state.selectedGeometryId",
		"findGeometryNavigationTarget",
	} {
		if !strings.Contains(reveal, required) {
			t.Fatalf("semantic-to-geometry reveal is missing %q", required)
		}
	}
	if strings.Contains(reveal, "geometryShowZones.checked =") ||
		strings.Contains(reveal, "geometryShowWalls.checked =") ||
		strings.Contains(reveal, "geometryShowWindows.checked =") {
		t.Fatal("semantic geometry reveal must materialize a target without changing visibility filters")
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
