package frontendchecks

import (
	"strings"
	"testing"
)

func TestProfilePanelSemanticNavigationContract(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/profile-views.js")
	for _, term := range []string{
		`configureResultPanelNavigationHooks("profile"`,
		"profileNavigationIndex",
		"getSemanticNavigationCache",
		"profileSemanticNavigationCache",
		`cache.occurrenceIDs("view-target"`,
		"cache.occurrencesForIDs",
		"cache.entity(",
		"data-entity-id",
		"data-entity-kind",
		"data-panel-target-id",
		"data-occurrence-context",
		"data-source-object-id",
		"data-source-object-index",
		"data-source-field-index",
		"profileZoneDimensionTargetID",
		"profileSeriesSemanticTargets",
		"data-choose-semantic-occurrence",
	} {
		if !strings.Contains(content, term) {
			t.Fatalf("Profile semantic navigation is missing %q", term)
		}
	}
}

func TestProfileSemanticRevealAndHistoryContextContract(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/profile-views.js")
	for _, term := range []string{
		"activeProfileView",
		"activeProfileZoneName",
		"activeProfileGroupId",
		"profileSelectedCell",
		"profileSelectedGroupIds",
		"profileSelectedZoneNames",
		"profileSelectedDimensions",
		"profileSelectionAnchorKey",
		"profileNavigationRevealTarget",
		"captureProfileNavigationContext",
		"restoreProfileNavigationContext",
		"preferredProfileSemanticOccurrence",
		"findProfileNavigationTarget",
		"selectProfileZoneDimensionForNavigation",
		"selectProfileItemForNavigation",
		"temporaryProfileDimensionSummary",
	} {
		if !strings.Contains(content, term) {
			t.Fatalf("Profile reveal/context contract is missing %q", term)
		}
	}

	for _, guard := range []string{
		"entityIDs.length !== 1",
		"anchorsByKey.size === 1",
		"sameStringSet(candidate.zoneNames, group.zoneNames)",
	} {
		if !strings.Contains(content, guard) {
			t.Fatalf("Profile aggregate navigation guard is missing %q", guard)
		}
	}

	if strings.Contains(content, "profileFilter") {
		t.Fatal("Profile navigation history still preserves the removed Profile filter")
	}

	capture := sliceBetween(content, "function captureProfileNavigationContext", "async function restoreProfileNavigationContext")
	restore := sliceBetween(content, "async function restoreProfileNavigationContext", "function preferredProfileSemanticOccurrence")
	for _, field := range []string{"profileSelectedGroupIds", "profileSelectedZoneNames", "profileSelectedDimensions", "profileSelectionAnchorKey"} {
		if !strings.Contains(capture, field) {
			t.Fatalf("Profile navigation capture does not preserve row-selection field %q", field)
		}
		if !strings.Contains(restore, field) || !strings.Contains(restore, "snapshot."+field) {
			t.Fatalf("Profile navigation restore does not restore row-selection field %q", field)
		}
	}
	if !strings.Contains(content, "normalizeProfileRowSelections") {
		t.Fatal("Profile render must normalize restored row selections against the current table rows")
	}
	for _, removed := range []string{"profileGraphDeck", "overviewScrollTop", "graphScrollTop", "detailScrollTop"} {
		if strings.Contains(content, removed) {
			t.Fatalf("Profile history still preserves redundant state %q", removed)
		}
	}
	for _, retained := range []string{"paneScrollTop", "matrixScrollTop"} {
		if !strings.Contains(capture, retained) || !strings.Contains(restore, "snapshot."+retained) {
			t.Fatalf("Profile history does not preserve live scroll state %q", retained)
		}
	}
}
