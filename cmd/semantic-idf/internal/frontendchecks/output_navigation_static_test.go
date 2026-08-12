package frontendchecks

import (
	"os"
	"strings"
	"testing"
)

func TestStandaloneOutputTabAndNavigationAreRemoved(t *testing.T) {
	files := map[string][]string{
		"frontend/src/index.html": {
			`data-result-tab="output"`,
			`id="outputPane"`,
			`id="outputApplyDialog"`,
		},
		"frontend/src/js/main.js": {
			`./views/output-views.js`,
			"initializeOutputControls",
			`idfAnalyzer:outputApplied`,
		},
		"frontend/src/js/views/analysis-views.js": {
			`from "./output-views.js"`,
			"renderOutput(",
			`case "output"`,
		},
		"frontend/src/js/views/hvac-views.js": {
			`data-result-tab="output"`,
			"prepareHVACOutputContext",
		},
		"frontend/src/js/actions.js": {
			"outputFocusedSignature",
			"outputTemporaryRevealSignature",
			`case "output":`,
			`["profile", "hvac", "output", "diagnostics", "geometry"]`,
		},
		"frontend/src/js/state.js": {
			"outputPurposeFilter",
			"outputFocusedSignature",
			"outputTemporaryRevealSignature",
			`document.querySelector("#outputStats")`,
		},
		"frontend/src/js/shortcuts.js": {
			"tabOutput",
			`switchResultTab?.("output")`,
		},
		"frontend/src/settings.html": {
			"tabOutput",
			"shortcut.tabOutput",
		},
		"../../scripts/frontend-build.ps1": {
			`"views/output-views.js"`,
		},
	}
	for path, forbidden := range files {
		content := readTestFile(t, path)
		for _, term := range forbidden {
			if strings.Contains(content, term) {
				t.Errorf("%s retains removed Output-tab contract %q", path, term)
			}
		}
	}

	app := readTestFile(t, "app.go")
	if strings.Contains(app, `requiredStages := []string{"profile", "hvac", "output"`) {
		t.Fatal("completed staged analysis still requires the removed Output-tab stage")
	}

	if _, err := os.Stat(repoPath("frontend/src/js/views/output-views.js")); !os.IsNotExist(err) {
		t.Fatalf("standalone Output view module still exists (stat error: %v)", err)
	}
}

func TestCoreOutputAnalysisAndHVACAddMonitorRemain(t *testing.T) {
	app := readTestFile(t, "app.go")
	for _, required := range []string{"func (a *App) AnalyzeInputOutputText", "idf.AnalyzeOutput(doc)"} {
		if !strings.Contains(app, required) {
			t.Fatalf("core Output analysis contract is missing %q", required)
		}
	}

	analysis := readTestFile(t, "internal/idf/output.go")
	for _, required := range []string{"type OutputReport struct", "func AnalyzeOutput(doc Document) OutputReport"} {
		if !strings.Contains(analysis, required) {
			t.Fatalf("core Output analyzer is missing %q", required)
		}
	}

	hvac := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	for _, required := range []string{
		"data-hvac-output-variable",
		"function openHVACOutputDialog",
		"state.hvacOutputRequest",
		`callHVACApplyAPI("PreviewHVACApplyText"`,
		`callHVACApplyAPI("ApplyHVACText"`,
	} {
		if !strings.Contains(hvac, required) {
			t.Fatalf("HVAC inline Add Monitor contract is missing %q", required)
		}
	}
}
