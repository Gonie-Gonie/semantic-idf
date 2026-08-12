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
		"applyProfileNavigationTarget",
		"selectProfileItemForNavigation",
		"selectProfileZoneDimensionForNavigation",
		"profileNavigationRevealDimension",
	} {
		if !strings.Contains(content, term) {
			t.Fatalf("Profile semantic navigation is missing %q", term)
		}
	}
}

func TestProfileSemanticNavigationUsesOverviewAndGraphContract(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/profile-views.js")

	targetData := sliceBetween(content, "function profileNavigationTargetData", "function applyProfileNavigationTarget")
	for _, term := range []string{
		`case "profile-item"`,
		"profileItemByID(target.targetId",
		`case "zone-dimension"`,
		"profileZoneDimensionForTarget(target.targetId)",
	} {
		if !strings.Contains(targetData, term) {
			t.Fatalf("Profile navigation target resolution is missing %q", term)
		}
	}

	applyTarget := sliceBetween(content, "function applyProfileNavigationTarget", "function selectProfileItemForNavigation")
	for _, route := range []string{
		"selectProfileItemForNavigation(targetData.item)",
		"selectProfileZoneDimensionForNavigation(targetData.zoneName, targetData.dimension)",
	} {
		if !strings.Contains(applyTarget, route) {
			t.Fatalf("Profile navigation target application is missing %q", route)
		}
	}

	selectItem := sliceBetween(content, "function selectProfileItemForNavigation", "function selectProfileZoneDimensionForNavigation")
	if !strings.Contains(selectItem, "selectProfileZoneDimensionForNavigation(item.zoneName, item.dimension)") {
		t.Fatal("Profile item navigation must delegate to the shared Zone/dimension selection path")
	}

	selectZoneDimension := sliceBetween(content, "function selectProfileZoneDimensionForNavigation", "function profileZoneDimensionForTarget")
	for _, term := range []string{
		"profileNavigationRevealDimension = dimension",
		"state.profileSettings.enabledDimensions.includes(dimension)",
		"selectProfileZone(zoneName)",
		"selectProfileDimension(dimension)",
	} {
		if !strings.Contains(selectZoneDimension, term) {
			t.Fatalf("Profile Zone/dimension navigation is missing %q", term)
		}
	}

	findTarget := sliceBetween(content, "function findProfileNavigationTarget", "function captureProfileNavigationContext")
	fallbacks := []string{
		`elements.profileGraph?.querySelectorAll("[data-profile-dimension]")`,
		`elements.profileOverview?.querySelectorAll("[data-profile-zone]")`,
		`elements.profileOverview?.querySelectorAll("[data-profile-group-id]")`,
	}
	previousIndex := -1
	for _, fallback := range fallbacks {
		index := strings.Index(findTarget, fallback)
		if index < 0 {
			t.Fatalf("Profile navigation target lookup is missing fallback %q", fallback)
		}
		if index <= previousIndex {
			t.Fatalf("Profile navigation fallbacks must search Graph dimension, Overview Zone, then Overview group; %q is out of order", fallback)
		}
		previousIndex = index
	}
}

func TestProfileSemanticNavigationOmitsRemovedSecondaryUIContract(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/profile-views.js")
	for _, removed := range []string{
		"function renderProfileMatrix",
		"function renderProfileDetail",
		"function renderProfileCandidateRow",
		"function selectProfileMatrixCell",
		"temporaryProfileDimensionSummary",
		"profileSelectedCell",
		"matrixScrollTop",
		"data-profile-cell",
		"data-profile-candidate-id",
		"data-choose-semantic-occurrence",
		"profile-source-accordion",
		"profile-detail-panel",
		"profile-matrix",
		"elements.profileMatrix",
		"elements.profileDetail",
	} {
		if strings.Contains(content, removed) {
			t.Fatalf("Profile navigation still depends on removed Matrix/Detail/Source/Candidate UI %q", removed)
		}
	}
}

func TestProfileSemanticRevealAndHistoryContextContract(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/profile-views.js")
	for _, term := range []string{
		"activeProfileView",
		"activeProfileZoneName",
		"activeProfileGroupId",
		"profileSelectedGroupIds",
		"profileSelectedZoneNames",
		"profileSelectedDimensions",
		"profileSelectionAnchorKey",
		"profileNavigationRevealDimension",
		"captureProfileNavigationContext",
		"restoreProfileNavigationContext",
		"preferredProfileSemanticOccurrence",
		"findProfileNavigationTarget",
		"selectProfileZoneDimensionForNavigation",
		"selectProfileItemForNavigation",
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
	for _, removed := range []string{"profileGraphDeck", "overviewScrollTop", "graphScrollTop", "detailScrollTop", "matrixScrollTop", "profileSelectedCell"} {
		if strings.Contains(content, removed) {
			t.Fatalf("Profile history still preserves redundant state %q", removed)
		}
	}
	if !strings.Contains(capture, "paneScrollTop") || !strings.Contains(restore, "snapshot.paneScrollTop") {
		t.Fatal("Profile history does not preserve the live Profile pane scroll position")
	}
	if !strings.Contains(capture, "navigationRevealDimension: profileNavigationRevealDimension") {
		t.Fatal("Profile navigation capture does not preserve its transient revealed dimension")
	}
	if !strings.Contains(restore, "profileNavigationRevealDimension = String(snapshot.navigationRevealDimension") {
		t.Fatal("Profile navigation restore does not restore its transient revealed dimension")
	}
}
