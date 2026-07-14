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
		"navigation.byViewTarget",
		"data-entity-id",
		"data-entity-kind",
		"data-panel-target-id",
		"data-occurrence-context",
		"data-source-object-id",
		"data-source-object-index",
		"data-source-field-index",
		"profileZoneDimensionTargetID",
		"profileSeriesSemanticTargets",
		"profileScheduleSemanticAttributes",
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
		"profilePinnedSeriesIds",
		"profileGraphDeck",
		"profileFilter",
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
		"scheduleNames.length !== 1",
		"sameStringSet(candidate.zoneNames, group.zoneNames)",
	} {
		if !strings.Contains(content, guard) {
			t.Fatalf("Profile aggregate navigation guard is missing %q", guard)
		}
	}
}
