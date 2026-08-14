package frontendchecks

import (
	"regexp"
	"strings"
	"testing"
)

func TestInputWorkspaceUsesTheResultTabVisualAndAccessibilityContract(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	editor := sliceBetween(index, `<aside class="editor-panel"`, `<div id="workspaceSplitter"`)

	inputTabs := regexp.MustCompile(`(?s)<nav[^>]*class="[^"]*\btabs\b[^"]*"[^>]*role="tablist"[^>]*aria-label="Input views"[^>]*>(.*?)</nav>`).FindStringSubmatch(editor)
	if len(inputTabs) != 2 {
		t.Fatal("input views must use the same labeled <nav class=\"tabs\"> pattern as result tabs")
	}
	views := []struct {
		name  string
		tabID string
	}{
		{name: "semantic", tabID: "inputSemanticTab"},
		{name: "text", tabID: "inputTextTab"},
		{name: "json", tabID: "inputJSONTab"},
		{name: "table", tabID: "inputTableTab"},
	}
	for index, contract := range views {
		view := contract.name
		button := regexp.MustCompile(`(?s)<button[^>]*class="([^"]*)"[^>]*data-input-view="` + view + `"[^>]*type="button"[^>]*>`).FindStringSubmatch(inputTabs[1])
		if len(button) != 2 || !hasHTMLClass(button[1], "tab") {
			t.Fatalf("%s input view must use the shared tab button contract", view)
		}
		buttonTag := button[0]
		expectedSelected := "false"
		if index == 0 {
			expectedSelected = "true"
		}
		for _, required := range []string{`role="tab"`, `aria-selected="` + expectedSelected + `"`, `aria-controls="` + view + `InputView"`} {
			if !strings.Contains(buttonTag, required) {
				t.Fatalf("%s input tab is missing accessibility state %q", view, required)
			}
		}
		if index == 0 && !hasHTMLClass(button[1], "active") {
			t.Fatal("Semantic must remain the initially active input tab")
		}
		panel := regexp.MustCompile(`(?s)<div[^>]*id="` + view + `InputView"[^>]*>`).FindString(editor)
		for _, required := range []string{`role="tabpanel"`, `aria-labelledby="` + contract.tabID + `"`} {
			if !strings.Contains(panel, required) {
				t.Fatalf("%s input panel is missing accessibility relationship %q", view, required)
			}
		}
	}

	resultTabs := regexp.MustCompile(`(?s)<nav[^>]*class="[^"]*\btabs\b[^"]*"[^>]*aria-label="Result tabs"[^>]*>`).FindString(index)
	if resultTabs == "" {
		t.Fatal("result tabs lost the shared labeled navigation contract")
	}

	stateSource := readTestFile(t, "frontend/src/js/state.js")
	if !strings.Contains(stateSource, `document.querySelectorAll(".tab[data-input-view]")`) {
		t.Fatal("input tab discovery must be scoped to shared .tab input-view buttons")
	}
	if strings.Contains(stateSource, `document.querySelectorAll(".view-tab")`) {
		t.Fatal("input tab state still depends on the superseded view-tab styling hook")
	}
	inputViews := readTestFile(t, "frontend/src/js/views/input-views.js")
	switchView := sliceBetween(inputViews, "export async function switchInputView", "export function setTableOrientation")
	for _, required := range []string{`setAttribute("aria-selected"`, "button.tabIndex", "view.hidden"} {
		if !strings.Contains(switchView, required) {
			t.Fatalf("input tab switching must update accessibility state, missing %q", required)
		}
	}
	mainSource := readTestFile(t, "frontend/src/js/main.js")
	for _, required := range []string{
		`button.addEventListener("keydown"`,
		`"ArrowLeft"`,
		`"ArrowRight"`,
		`"Home"`,
		`"End"`,
		"target.focus()",
	} {
		if !strings.Contains(mainSource, required) {
			t.Fatalf("roving input tabs must remain keyboard reachable, missing %q", required)
		}
	}
}

func TestInputWorkspaceOmitsVersionAndCountChrome(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	views := readTestFile(t, "frontend/src/js/views/input-views.js")
	state := readTestFile(t, "frontend/src/js/state.js")
	for _, removed := range []string{"inputFilterStats", "fieldStats"} {
		if strings.Contains(index, removed) || strings.Contains(state, removed) {
			t.Fatalf("left input panel count element remains: %q", removed)
		}
	}
	for _, removed := range []string{`Version ${escapeHTML`, `t("count.objects"`, `t("input.tableStats"`} {
		if strings.Contains(views, removed) {
			t.Fatalf("left input view count/version chrome remains: %q", removed)
		}
	}
}

func TestInputToolbarKeepsTabsAndSearchHorizontallyReachable(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	styles := readTestFile(t, "frontend/src/styles/base.css")
	main := readTestFile(t, "frontend/src/js/main.js")
	toolbar := sliceBetween(index, `<div id="inputToolbarScroll"`, `<div class="input-view active"`)
	for _, required := range []string{`class="tabs input-tabs"`, `id="inputFilter"`} {
		if !strings.Contains(toolbar, required) {
			t.Fatalf("horizontal Input toolbar markup is missing %q", required)
		}
	}
	if strings.Index(toolbar, `class="tabs input-tabs"`) > strings.Index(toolbar, `id="inputFilter"`) {
		t.Fatal("Input search must follow the view tabs in the horizontal toolbar")
	}
	for _, required := range []string{".input-toolbar-scroll", "overflow-x: auto", "cursor: grab", "touch-action: pan-x", "flex: 0 0 280px"} {
		if !strings.Contains(styles, required) {
			t.Fatalf("horizontal Input toolbar styling is missing %q", required)
		}
	}
	for _, required := range []string{"bindHorizontalDragScroll(elements.inputToolbarScroll)", `element.scrollLeft = startScrollLeft - delta`, `element.addEventListener("wheel"`, `if (!element.hasPointerCapture(pointerID))`} {
		if !strings.Contains(main, required) {
			t.Fatalf("horizontal Input toolbar interaction is missing %q", required)
		}
	}
	if strings.Contains(sliceBetween(main, `element.addEventListener("pointerdown"`, `element.addEventListener("pointermove"`), "setPointerCapture") {
		t.Fatal("Input toolbar must not capture a simple pointer press before drag starts because that retargets tab clicks")
	}
}

func TestInputWorkspaceHeadingLineCountAndExplicitSemanticRevealAreRemoved(t *testing.T) {
	files := map[string]string{
		"app settings":      readTestFile(t, "app.go"),
		"input markup":      readTestFile(t, "frontend/src/index.html"),
		"translations":      readTestFile(t, "frontend/src/js/i18n.js"),
		"main runtime":      readTestFile(t, "frontend/src/js/main.js"),
		"state runtime":     readTestFile(t, "frontend/src/js/state.js"),
		"input views":       readTestFile(t, "frontend/src/js/views/input-views.js"),
		"selection control": readTestFile(t, "frontend/src/js/selection-controller.js"),
		"navigation":        readTestFile(t, "frontend/src/js/navigation.js"),
		"panel actions":     readTestFile(t, "frontend/src/js/panel-navigation-actions.js"),
		"shortcuts":         readTestFile(t, "frontend/src/js/shortcuts.js"),
		"settings client":   readTestFile(t, "frontend/src/js/settings-client.js"),
		"settings markup":   readTestFile(t, "frontend/src/settings.html"),
		"workspace styles":  readTestFile(t, "frontend/src/styles/base.css"),
	}
	removedEverywhere := []string{
		"semanticRevealIndicator",
		"semantic-reveal-indicator",
		"idfAnalyzer:semanticRevealAvailable",
		"updateSemanticRevealIndicator",
		"semantic.revealInSemantic",
		"revealSelectionInSemantic",
	}
	for name, content := range files {
		for _, removed := range removedEverywhere {
			if strings.Contains(content, removed) {
				t.Fatalf("%s retains removed Semantic reveal UI contract %q", name, removed)
			}
		}
	}

	index := files["input markup"]
	for _, removed := range []string{`data-i18n="input.view"`, `id="textStats"`, ">0 lines<"} {
		if strings.Contains(index, removed) {
			t.Fatalf("input workspace still renders removed heading/line statistic %q", removed)
		}
	}
	stateSource := files["state runtime"]
	for _, removed := range []string{"textStats:", "updateTextStats", `t("count.lines"`} {
		if strings.Contains(stateSource, removed) {
			t.Fatalf("state runtime still computes removed line count via %q", removed)
		}
	}

	for name, content := range map[string]string{
		"app settings":    files["app settings"],
		"main runtime":    files["main runtime"],
		"panel actions":   files["panel actions"],
		"shortcuts":       files["shortcuts"],
		"settings client": files["settings client"],
		"settings markup": files["settings markup"],
	} {
		for _, removed := range []string{"revealSemantic", "revealCurrentSelectionInSemantic", "panelNavigation.revealSemantic", "shortcut.revealSemantic"} {
			if strings.Contains(content, removed) {
				t.Fatalf("%s still exposes removed explicit Reveal in Semantic command %q", name, removed)
			}
		}
	}

	i18n := files["translations"]
	for _, removed := range []string{`"input.view"`, `"count.lines"`, `"shortcut.revealSemantic"`, `"panelNavigation.revealSemantic"`} {
		if strings.Contains(i18n, removed) {
			t.Fatalf("translations retain removed input chrome string %q", removed)
		}
	}
}

func hasHTMLClass(classList, target string) bool {
	for _, className := range strings.Fields(classList) {
		if className == target {
			return true
		}
	}
	return false
}
