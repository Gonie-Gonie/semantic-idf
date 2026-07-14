package frontendchecks

import (
	"regexp"
	"strings"
	"testing"
)

func TestPanelNavigationActionMenuOrderAndControllerRouting(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/panel-navigation-actions.js")
	for _, name := range []string{"initializePanelNavigationActions", "openPanelNavigationMenu", "closePanelNavigationMenu"} {
		assertJSExport(t, content, name)
	}
	order := []string{`["focus"`, `["semantic"`, `["source"`, `["related"`, `["definition"`, `["references"`, `["pin"`}
	last := -1
	for _, token := range order {
		index := strings.Index(content, token)
		if index < 0 || index <= last {
			t.Fatalf("panel action order is missing or unstable at %q", token)
		}
		last = index
	}
	for _, required := range []string{
		`addEventListener("contextmenu"`,
		`event.key !== "ContextMenu"`,
		`event.shiftKey && event.key === "F10"`,
		"selectSemanticEntity(",
		"openSelectionInView(",
		"revealSelectionSource(",
		"selectionTargetsForView(",
		"recordViewHistory()",
		"recordHistory: false",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("common panel actions are missing %q", required)
		}
	}
	if regexp.MustCompile(`(?i)\b(?:analyze|analyzeinput|scheduleanalyze)\s*\(`).MatchString(content) {
		t.Fatal("panel navigation actions must not start analysis")
	}
}

func TestPanelNavigationHoverPinAndStartupContract(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/panel-navigation-actions.js")
	for _, required := range []string{
		"hoverSemanticEntity(",
		"clearSemanticHover(",
		"semanticPinnedEntityIds",
		"data-semantic-pinned",
		"idfAnalyzer:semanticPinChanged",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("panel hover/pin contract is missing %q", required)
		}
	}
	main := readTestFile(t, "frontend/src/js/main.js")
	if !strings.Contains(main, "initializePanelNavigationActions({") {
		t.Fatal("application startup must initialize common panel actions")
	}
	build := readTestFile(t, "scripts/frontend-build.ps1")
	if !strings.Contains(build, `"panel-navigation-actions.js"`) {
		t.Fatal("frontend readiness manifest is missing panel-navigation-actions.js")
	}
}
