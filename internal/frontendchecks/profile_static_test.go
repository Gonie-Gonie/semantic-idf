package frontendchecks

import (
	"os"
	"strings"
	"testing"
)

func TestFrontendProfileGraphStateContracts(t *testing.T) {
	files := map[string]string{
		"profile views": readTestFile(t, "frontend/src/js/views/profile-views.js"),
		"state":         readTestFile(t, "frontend/src/js/state.js"),
		"styles":        readTestFile(t, "frontend/src/styles/profile.css"),
	}
	required := map[string][]string{
		"profile views": {
			"profileGraphSeries",
			"currentProfileGraphOptions",
			"data-profile-cell",
			"tabindex=\"0\"",
			"keydown",
			"profile-graph-view-switch",
			"data-profile-time-view",
			"aria-pressed",
			"downsampleValues",
			"profileSelectedDimensions",
		},
		"state": {
			"profileSelectedCell: null",
			"profileSelectedGroupIds: []",
			"profileSelectedZoneNames: []",
			"profileSelectedDimensions: []",
			`profileSelectionAnchorKey: ""`,
		},
		"styles": {
			".profile-live-controls",
			"align-items: start",
			".profile-live-group",
			"grid-template-rows: auto minmax(32px, auto)",
			".profile-graph-view-switch",
			".profile-overlay-graph",
			".profile-matrix td.same-schedule-different-value",
			".profile-source-accordion",
		},
	}
	for label, terms := range required {
		for _, term := range terms {
			if !strings.Contains(files[label], term) {
				t.Fatalf("%s missing Profile Graph state contract %q", label, term)
			}
		}
	}
}

func TestFrontendProfileRenderReusesIndexesAndDelegatesDynamicControls(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	render := sliceBetween(views, "export function renderProfile", "function renderEmptyProfile")
	if strings.Contains(render, "bindProfileControls") {
		t.Fatal("Profile render must not recreate dynamic control listeners")
	}
	for _, required := range []string{
		"profileItemMapCache = new WeakMap()",
		"profileSemanticNavigationCache",
		`cache.occurrenceIDs("view-target"`,
		"renderProfileMatrix(lastProfileView.matrix, profile, itemMap)",
		"renderProfileDetail(graphGroup, profile, selectedZone, itemMap)",
		"const activeZoneNames = profileActiveMatrixZoneNames()",
		"const dimensionByID = new Map(",
		"const selectedGroupIDs = new Set(",
		"const selectedZoneNames = new Set(",
		"bindProfileControls();",
		`elements.profileOverview?.addEventListener("click", handleProfileOverviewActivation)`,
		`elements.profileMatrix?.addEventListener("click", handleProfileMatrixActivation)`,
	} {
		if !strings.Contains(views, required) {
			t.Fatalf("Profile render optimization contract is missing %q", required)
		}
	}
}

func TestFrontendProfileRowsDriveSingleToggleAndRangeSelection(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/profile-views.js")
	state := readTestFile(t, "frontend/src/js/state.js")
	for _, term := range []string{
		"profileSelectedGroupIds: []",
		"profileSelectedZoneNames: []",
		`profileSelectionAnchorKey: ""`,
	} {
		if !strings.Contains(state, term) {
			t.Fatalf("profile row-selection state is missing %q", term)
		}
	}
	for _, term := range []string{
		"data-profile-row-key",
		`role="option"`,
		`aria-selected="${selected ? "true" : "false"}"`,
		"profileSelectedGroupIds",
		"profileSelectedZoneNames",
		"profileSelectionAnchorKey",
		"event.ctrlKey",
		"event.metaKey",
		"event.shiftKey",
		`const rowKey = button.dataset.profileRowKey || ""`,
		"handleProfileRowSelection(event, rowKey)",
	} {
		if !strings.Contains(content, term) {
			t.Fatalf("profile table row selection is missing %q", term)
		}
	}
	if strings.Contains(content, `aria-selected="${selected ? "true" : "false"}""`) {
		t.Fatal("profile option row contains a malformed aria-selected attribute")
	}
}

func TestFrontendProfileLineViewsAlwaysUseLegendAndAnnualViewsUseParallelHeatmaps(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	styles := readTestFile(t, "frontend/src/styles/profile.css")
	body := sliceBetween(views, "function renderProfileGraphBody", "function renderProfileOverlay")
	if strings.Contains(body, "compareMode") {
		t.Fatal("Profile line overlay must not be gated behind the optional Compare mode")
	}
	for _, required := range []string{
		"renderProfileOverlay",
		"renderProfileSeriesLegend",
		"renderProfileAnnualHeatmaps",
		"profile-overlay-legend",
		"profile-annual-heatmap-grid",
		`canvas class="profile-heatmap"`,
		"paintProfileHeatmaps",
		`getContext?.("2d")`,
		"fillRect(",
	} {
		if !strings.Contains(views, required) && !strings.Contains(styles, required) {
			t.Fatalf("Profile graph presentation is missing %q", required)
		}
	}
	if !strings.Contains(styles, ".profile-annual-heatmap-grid") {
		t.Fatal("annual Profile heatmaps need a dedicated wrapping grid for parallel comparison")
	}
	if !strings.Contains(styles, ".profile-overlay-legend") {
		t.Fatal("Profile line overlay legend styling is missing")
	}
}

func TestFrontendProfileGraphControlsUseFixedTimeProfileAndDirectViewButtons(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	state := readTestFile(t, "frontend/src/js/state.js")
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	analysis := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	settingsPage := readTestFile(t, "frontend/src/settings.html")
	settingsClient := readTestFile(t, "frontend/src/js/settings-client.js")
	styles := readTestFile(t, "frontend/src/styles/profile.css")
	responsive := readTestFile(t, "frontend/src/styles/responsive.css")
	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	graphBody := sliceBetween(views, "function renderProfileGraph", "function renderProfileGraphBody")
	bindings := sliceBetween(views, "function bindProfileControls", "export function initializeProfileControls")
	for label, content := range map[string]string{
		"markup":   markup,
		"state":    state,
		"views":    views,
		"analysis": analysis,
	} {
		for _, removed := range []string{"profileGraphStats", "profileGraphDeckStats"} {
			if strings.Contains(content, removed) {
				t.Fatalf("%s still exposes redundant Profile Graph metadata %q", label, removed)
			}
		}
	}
	for _, removed := range []string{
		`id="profileScheduleSummaryMode"`,
		`for="profileScheduleSummaryMode"`,
		`document.querySelector("#profileScheduleSummaryMode")`,
	} {
		if strings.Contains(settingsPage, removed) {
			t.Fatalf("Settings still exposes the legacy Profile View selector %q", removed)
		}
	}
	for _, required := range []string{
		`class="profile-graph-view-switch"`,
		`role="group"`,
		`data-profile-time-view`,
		`aria-pressed`,
		`"day"`,
		`"week"`,
		`"month"`,
		`"year"`,
		`"duration"`,
		`"rules"`,
	} {
		if !strings.Contains(graphBody, required) {
			t.Fatalf("Profile Graph direct View buttons are missing %q", required)
		}
	}
	for _, removed := range []string{
		`id="profileGraphPreset"`,
		`id="profileGraphScopeType"`,
		`id="profileGraphCompareMode"`,
		`id="profileGraphTimeView"`,
		`"#profileGraphPreset"`,
		`"#profileGraphScopeType"`,
		`"#profileGraphCompareMode"`,
		`"#profileGraphTimeView"`,
		"applyProfileGraphPreset",
		"currentProfilePresetID",
		`selectionDriven: false`,
		`selectionDriven = false`,
		`scopeType: "schedule"`,
		`scopeType = "schedule"`,
		`compareMode: "similarity"`,
		`compareMode = "similarity"`,
		`compareMode: "outliers"`,
		`compareMode = "outliers"`,
	} {
		if strings.Contains(views, removed) {
			t.Fatalf("Profile Graph still exposes a removed Graph Type/Scope/Compare/View selector contract %q", removed)
		}
	}
	if strings.Contains(graphBody, `id="profileGraphScaleMode"`) {
		t.Fatal("Profile Graph toolbar still owns the Scale selector")
	}
	for _, removed := range []string{`id="profileGraphScaleMode"`, `"#profileGraphScaleMode"`} {
		if strings.Contains(views, removed) {
			t.Fatalf("Profile tab still owns the removed Scale selector %q", removed)
		}
	}
	deadContracts := map[string][]string{
		"views": {
			"profileGraphDeck",
			"mergeProfileGraphDeck",
			"profileDeckSeries",
			"renderProfileDeckBody",
			"renderProfileSeriesCard",
			"renderProfileSeriesRanking",
			"renderProfileScheduleSimilarity",
			"renderScheduleClusterScatter",
			"renderProfileOutlierDeck",
			"renderProfileOutlierRow",
			"renderProfileGraphCard",
			"renderGraphVisual",
			"graphDataForDimension",
			"profileSeriesGraphData",
			"fallbackRulesForSeries",
			"scheduleLookupMap",
			"scheduleForProfileDimension",
			"profileScheduleSemanticAttributes",
			`querySelectorAll("[data-profile-series-focus]")`,
			`elements.profileGraph.querySelectorAll("[data-profile-candidate-id]")`,
			"profilePinnedSeriesIds",
			"pinnedSeriesIds",
		},
		"state": {"profileGraphDeck", "profilePinnedSeriesIds"},
		"styles": {
			".profile-graph-card",
			".profile-pin-button",
			".profile-ranking-row",
			".profile-cluster-row",
			".profile-similarity-grid",
			".profile-scatter",
		},
		"responsive": {
			".profile-ranking-row",
			".profile-cluster-row",
			".profile-similarity-grid",
			".profile-scatter",
		},
		"i18n": {`"profile.graphType"`, `"profile.scheduleSummary"`},
	}
	deadSources := map[string]string{
		"views": views, "state": state, "styles": styles, "responsive": responsive, "i18n": i18n,
	}
	for label, terms := range deadContracts {
		for _, term := range terms {
			if strings.Contains(deadSources[label], term) {
				t.Fatalf("%s still contains unreachable Profile Graph contract %q", label, term)
			}
		}
	}
	for _, required := range []string{
		"function renderProfileCandidateRow",
		`event.target.closest("[data-profile-candidate-id]")`,
		"zones · ",
	} {
		if !strings.Contains(views, required) {
			t.Fatalf("live Profile candidate detail contract is missing %q", required)
		}
	}
	for _, required := range []string{".profile-qa-row", ".profile-qa-row.warning", ".profile-qa-row.error"} {
		if !strings.Contains(styles, required) {
			t.Fatalf("live Profile QA styling is missing %q", required)
		}
	}
	for _, required := range []string{
		`elements.profileGraph?.addEventListener("click", handleProfileGraphActivation)`,
		`event.target.closest("[data-profile-time-view]")`,
		`timeViewButton.dataset.profileTimeView`,
		`focus({ preventScroll: true })`,
	} {
		if !strings.Contains(bindings, required) {
			t.Fatalf("Profile direct View binding is missing %q", required)
		}
	}
	for _, required := range []string{
		`id="profileScaleMode"`,
		`optionHTML("auto"`,
		`optionHTML("shared"`,
		`optionHTML("design_peak"`,
		`optionHTML("multiplier_0_1"`,
		`optionHTML("percentile"`,
		`scaleMode: document.querySelector("#profileScaleMode").value`,
	} {
		if !strings.Contains(settingsPage, required) {
			t.Fatalf("Settings Profile Analysis Scale field is missing %q", required)
		}
	}
	for _, required := range []string{
		`scaleMode: "auto"`,
		`scaleMode: normalizeChoice(`,
		`profile.scaleMode`,
		`["auto", "shared", "design_peak", "multiplier_0_1", "percentile"]`,
	} {
		if !strings.Contains(settingsClient, required) {
			t.Fatalf("Profile Scale default/normalization contract is missing %q", required)
		}
	}
	for _, required := range []string{
		`metricMode: "actual"`,
		`metricMode: normalizeChoice(`,
		`profile.metricMode`,
		`profileMetricModeFromLegacy(profile.graphMode`,
	} {
		if !strings.Contains(settingsClient, required) {
			t.Fatalf("Profile Metric default/normalization contract is missing %q", required)
		}
	}
	for _, legacyOutput := range []string{`graphMode: normalizeChoice(`, `scheduleSummaryMode: normalizeChoice(`} {
		if strings.Contains(settingsClient, legacyOutput) {
			t.Fatalf("Profile settings still emits legacy graph state %q", legacyOutput)
		}
	}
}

func TestFrontendProfileUsesTableAboveGraphWithoutTopFilter(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	state := readTestFile(t, "frontend/src/js/state.js")
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	analysis := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	styles := readTestFile(t, "frontend/src/styles/profile.css")

	for label, content := range map[string]string{
		"markup":     markup,
		"state":      state,
		"views":      views,
		"analysis":   analysis,
		"simulation": simulation,
		"i18n":       i18n,
	} {
		for _, removed := range []string{"profileStats", "profileFilter"} {
			if strings.Contains(content, removed) {
				t.Fatalf("%s still contains removed Profile header/filter state %q", label, removed)
			}
		}
	}
	for _, removed := range []string{
		`class="summary-head profile-head"`,
		`data-i18n-placeholder="profile.filter"`,
	} {
		if strings.Contains(markup, removed) {
			t.Fatalf("Profile markup still contains removed top-header UI %q", removed)
		}
	}
	for _, removed := range []string{
		"function profileQuery",
		"function profileGroupMatchesQuery",
		"function profileMatrixRowMatchesQuery",
		"function profileRevealMatchesGroup",
		"function profileRevealMatchesRow",
	} {
		if strings.Contains(views, removed) {
			t.Fatalf("Profile views still contain removed filter behavior %q", removed)
		}
	}

	profilePane := sliceBetween(markup, `<div class="result-pane" id="profilePane">`, `<div class="result-pane" id="hvacPane">`)
	tableStart := strings.Index(profilePane, `<div class="profile-overview-table"`)
	profileVisualStart := strings.Index(profilePane, `<section class="profile-visual"`)
	if tableStart < 0 || profileVisualStart < 0 || tableStart >= profileVisualStart {
		t.Fatal("Profile visual table must appear above the Profile graph")
	}
	tableHead := sliceBetween(profilePane, `<div class="profile-overview-table-head"`, `<div id="profileOverview"`)
	tableMarkup := sliceBetween(profilePane, `<div class="profile-overview-table"`, `<section class="profile-visual"`)
	for _, required := range []string{
		`id="profileOverview" class="profile-overview-list" role="listbox" aria-multiselectable="true"`,
		`class="profile-overview-apply-head"`,
		`id="profileApplyButton" class="profile-apply-badge" type="button"`,
	} {
		if !strings.Contains(profilePane, required) && !strings.Contains(tableHead, required) {
			t.Fatalf("Profile visual table/header is missing %q", required)
		}
	}
	if strings.Count(markup, `id="profileApplyButton"`) != 1 {
		t.Fatal("Profile markup must contain exactly one Apply Profile button")
	}
	for _, forbiddenRole := range []string{`role="table"`, `role="row"`, `role="columnheader"`, `role="rowgroup"`} {
		if strings.Contains(tableMarkup, forbiddenRole) {
			t.Fatalf("Profile visual table must preserve native button/listbox semantics, found %q", forbiddenRole)
		}
	}
	graphSection := profilePane[profileVisualStart:]
	graphHeader := sliceBetween(graphSection, `<div class="profile-section-head">`, `</div>`)
	if strings.Contains(graphHeader, `id="profileApplyButton"`) {
		t.Fatal("Apply Profile must live in the top visual-table header, not the graph header")
	}
	for _, required := range []string{
		".profile-overview-table",
		".profile-overview-table-head",
		".profile-table-row",
		".profile-overview-apply-head",
		".profile-apply-badge",
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("Profile visual table styling is missing %q", required)
		}
	}
	for _, removed := range []string{
		"overflow-x: auto",
		"min-width: 760px",
		"width: max-content",
		"grid-template-columns: minmax(250px, min(32%, 360px)) minmax(0, 1fr)",
	} {
		if strings.Contains(styles, removed) {
			t.Fatalf("Profile still contains a horizontal/two-column overflow source %q", removed)
		}
	}

	for _, retained := range []string{
		"if (!elements.profileGraph)",
		"renderProfileGraph(graphGroup, profile)",
		"elements.profileApplyButton.disabled = !graphGroup",
		`elements.profileApplyButton?.addEventListener("click", openProfileApplyDialog)`,
		"profileNavigationRevealTarget",
		"navigationRevealTarget: profileNavigationRevealTarget",
		"captureProfileNavigationContext",
		"restoreProfileNavigationContext",
	} {
		if !strings.Contains(views, retained) {
			t.Fatalf("Profile graph/navigation behavior was removed with the header: %q", retained)
		}
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(repoPath(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
