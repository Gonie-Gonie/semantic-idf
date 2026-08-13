package frontendchecks

import (
	"os"
	"strings"
	"testing"
)

func TestWorkspaceLinkBarAndModeControlsAreRemoved(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	for _, removed := range []string{
		`id="workspaceLinkBar"`,
		`id="semanticLinkedToggle"`,
		`id="semanticFollowToggle"`,
		`id="workspaceSelectionLabel"`,
		`id="workspaceLinkTargets"`,
		`id="workspaceLinkMenuTargets"`,
		`class="workspace-link-bar`,
		`data-i18n="navigation.linked"`,
		`data-i18n="navigation.follow"`,
		`data-i18n="navigation.noSelection"`,
	} {
		if strings.Contains(markup, removed) {
			t.Fatalf("main workspace retains removed link-bar UI %q", removed)
		}
	}

	if _, err := os.Stat(repoPath("frontend/src/js/navigation-link-bar.js")); !os.IsNotExist(err) {
		if err == nil {
			t.Fatal("removed workspace navigation-link-bar module still exists")
		}
		t.Fatalf("stat removed workspace navigation-link-bar module: %v", err)
	}

	build := readTestFile(t, "../../scripts/frontend-build.ps1")
	if strings.Contains(build, `"navigation-link-bar.js"`) {
		t.Fatal("frontend readiness manifest retains the removed navigation-link-bar module")
	}

	stateSource := readTestFile(t, "frontend/src/js/state.js")
	for _, removed := range []string{
		"semanticLinkMode",
		"semanticFollowSelection",
		"workspaceLinkBar",
		"semanticLinkedToggle",
		"semanticFollowToggle",
		"workspaceSelectionLabel",
		"workspaceLinkTargets",
		"workspaceLinkMenuTargets",
	} {
		if strings.Contains(stateSource, removed) {
			t.Fatalf("frontend state retains removed link/follow mode state %q", removed)
		}
	}

	for _, path := range []string{
		"frontend/src/js/actions.js",
		"frontend/src/js/main.js",
		"frontend/src/js/selection-controller.js",
	} {
		content := readTestFile(t, path)
		for _, removed := range []string{"semanticLinkMode", "semanticFollowSelection"} {
			if strings.Contains(content, removed) {
				t.Fatalf("%s still reads, writes, or persists removed mode %q", path, removed)
			}
		}
	}

	translations := readTestFile(t, "frontend/src/js/i18n.js")
	for _, removed := range []string{
		`"navigation.linkBar"`,
		`"navigation.linked"`,
		`"navigation.follow"`,
		`"navigation.noSelection"`,
		`"navigation.availableTargets"`,
	} {
		if strings.Contains(translations, removed) {
			t.Fatalf("translations retain removed link-bar string %q", removed)
		}
	}

	for _, path := range []string{"frontend/src/styles/base.css", "frontend/src/styles/responsive.css"} {
		styles := readTestFile(t, path)
		if strings.Contains(styles, ".workspace-link-bar") {
			t.Fatalf("%s retains removed workspace link-bar styles", path)
		}
	}
}

func TestSemanticLinkAndFollowAreAlwaysEnabled(t *testing.T) {
	controller := readTestFile(t, "frontend/src/js/selection-controller.js")
	options := sliceBetween(controller, "function optionsFor", "async function inTransaction")
	if !strings.Contains(options, "follow: options.follow !== false") {
		t.Fatal("selection options must default follow to true without consulting persistent state")
	}

	selection := sliceBetween(controller, "async function selectSemanticEntity", "async function hoverSemanticEntity")
	if !strings.Contains(selection, "if (options.follow)") || !strings.Contains(selection, "await followSelection(selection, options)") {
		t.Fatal("committed selections must follow whenever the transaction does not explicitly suppress it")
	}
	for _, removed := range []string{"semanticLinkMode", "semanticFollowSelection"} {
		if strings.Contains(selection, removed) {
			t.Fatalf("committed selection propagation must not be gated by persistent mode %q", removed)
		}
	}

	hover := sliceBetween(controller, "async function hoverSemanticEntity", "async function clearSemanticHover")
	if !strings.Contains(hover, "await invokeHook(dependencies.onHoverChange") {
		t.Fatal("semantic hover changes must always propagate")
	}
	if strings.Contains(hover, "semanticLinkMode") || strings.Contains(hover, "semanticFollowSelection") || strings.Contains(hover, "followSelection(") {
		t.Fatal("hover propagation must be unconditional without navigating another pane")
	}
	if !strings.Contains(hover, "recordHistory: false") || !strings.Contains(hover, "follow: false") {
		t.Fatal("hover must remain history-free and non-navigating")
	}
}
