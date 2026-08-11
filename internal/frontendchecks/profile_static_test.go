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

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(repoPath(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
