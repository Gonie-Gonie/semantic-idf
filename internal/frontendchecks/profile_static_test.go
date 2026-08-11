package frontendchecks

import (
	"os"
	"strings"
	"testing"
)

func TestFrontendProfileGraphDeckContracts(t *testing.T) {
	files := map[string]string{
		"profile views": readTestFile(t, "frontend/src/js/views/profile-views.js"),
		"state":         readTestFile(t, "frontend/src/js/state.js"),
		"styles":        readTestFile(t, "frontend/src/styles/profile.css"),
	}
	required := map[string][]string{
		"profile views": {
			"mergeProfileGraphDeck",
			"profileDeckSeries",
			"data-profile-cell",
			"tabindex=\"0\"",
			"keydown",
			"id=\"profileGraphPreset\"",
			"graphPreset.addEventListener(\"change\"",
			"renderProfileScheduleSimilarity",
			"renderProfileOutlierDeck",
			"renderProfileSeriesRanking",
			"downsampleValues",
			"slice(0, 80)",
			"selectedScheduleHashes",
			"selectedDimensions",
			">◎</button>",
		},
		"state": {
			"profileGraphDeck: null",
			"profileSelectedCell: null",
			"profilePinnedSeriesIds: []",
		},
		"styles": {
			".profile-live-controls",
			"align-items: start",
			".profile-live-group",
			"grid-template-rows: auto minmax(32px, auto)",
			".profile-similarity-grid",
			".profile-qa-grid",
			".profile-overlay-graph",
			".profile-matrix td.same-schedule-different-value",
			".profile-source-accordion",
		},
	}
	for label, terms := range required {
		for _, term := range terms {
			if !strings.Contains(files[label], term) {
				t.Fatalf("%s missing Profile Graph Deck contract %q", label, term)
			}
		}
	}
	if strings.Contains(files["profile views"], "??/button>") {
		t.Fatal("profile pin button contains a malformed closing tag")
	}
}

func TestFrontendProfileMatrixCellsDriveDeckSelection(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/profile-views.js")
	for _, term := range []string{
		"function selectProfileMatrixCell",
		"data-profile-schedule-hash",
		"data-profile-item-ids",
		"scopeType: \"selection\"",
		"compareMode: \"single\"",
	} {
		if !strings.Contains(content, term) {
			t.Fatalf("profile matrix selection is missing %q", term)
		}
	}
}

func TestFrontendProfileGraphOmitsRedundantDeckStats(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	state := readTestFile(t, "frontend/src/js/state.js")
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	analysis := readTestFile(t, "frontend/src/js/views/analysis-views.js")
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
	for _, retained := range []string{
		`id="profileGraphPreset"`,
		`id="profileGraphTimeView"`,
		`id="profileGraphCompareMode"`,
		`id="profileGraphScaleMode"`,
	} {
		if !strings.Contains(views, retained) {
			t.Fatalf("Profile Graph behavior control was removed with its redundant stats: %q", retained)
		}
	}
}

func TestFrontendProfileUsesGraphHeaderWithoutTopFilter(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	state := readTestFile(t, "frontend/src/js/state.js")
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	analysis := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	i18n := readTestFile(t, "frontend/src/js/i18n.js")

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

	profileVisualStart := strings.Index(markup, `<section class="profile-visual"`)
	if profileVisualStart < 0 {
		t.Fatal("Profile visual section is missing")
	}
	profileVisualTail := markup[profileVisualStart:]
	profileVisualEnd := strings.Index(profileVisualTail, "</section>")
	if profileVisualEnd < 0 {
		t.Fatal("Profile visual section is not closed")
	}
	profileVisual := profileVisualTail[:profileVisualEnd]
	headerStart := strings.Index(profileVisual, `<div class="profile-section-head">`)
	if headerStart < 0 {
		t.Fatal("Profile Graph header is missing")
	}
	headerTail := profileVisual[headerStart:]
	headerEnd := strings.Index(headerTail, "</div>")
	if headerEnd < 0 {
		t.Fatal("Profile Graph header is not closed")
	}
	header := headerTail[:headerEnd]
	headingIndex := strings.Index(header, `data-i18n="profile.graph">Profile Graph</h3>`)
	applyIndex := strings.Index(header, `id="profileApplyButton"`)
	if headingIndex < 0 || applyIndex < 0 || applyIndex <= headingIndex {
		t.Fatal("Apply Profile must follow the Profile Graph heading inside its header")
	}
	if strings.Count(markup, `id="profileApplyButton"`) != 1 {
		t.Fatal("Profile markup must contain exactly one Apply Profile button")
	}

	for _, retained := range []string{
		"if (!elements.profileGraph)",
		"renderProfileGraph(graphGroup, profile, selectedZone)",
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
