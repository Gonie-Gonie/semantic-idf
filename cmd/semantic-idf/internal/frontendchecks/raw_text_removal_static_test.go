package frontendchecks

import (
	"strings"
	"testing"
)

func TestRawTextSurfaceStateHistoryLayoutAndSettingsAreRemoved(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	for _, removed := range []string{
		`id="idfInput"`,
		`id="inputRawSplitter"`,
		`id="syncRawTextToggle"`,
		"raw-editor-block",
		"input.rawText",
		"input.syncPosition",
	} {
		if strings.Contains(index, removed) {
			t.Fatalf("main workspace still exposes removed Raw Text UI %q", removed)
		}
	}
	for _, retained := range []string{
		`data-input-view="semantic"`,
		`data-input-view="text"`,
		`data-input-view="json"`,
		`data-input-view="table"`,
		`id="semanticEditor"`,
		`id="textObjectView"`,
		`id="jsonStructuredView"`,
		`id="fieldTable"`,
	} {
		if !strings.Contains(index, retained) {
			t.Fatalf("structured input workspace lost retained view contract %q", retained)
		}
	}

	runtimeFiles := map[string]string{
		"actions":      readTestFile(t, "frontend/src/js/actions.js"),
		"layout":       readTestFile(t, "frontend/src/js/layout.js"),
		"main":         readTestFile(t, "frontend/src/js/main.js"),
		"navigation":   readTestFile(t, "frontend/src/js/navigation.js"),
		"settings":     readTestFile(t, "frontend/src/js/settings-client.js"),
		"state":        readTestFile(t, "frontend/src/js/state.js"),
		"view-history": readTestFile(t, "frontend/src/js/view-history.js"),
		"input-views":  readTestFile(t, "frontend/src/js/views/input-views.js"),
	}
	for name, content := range runtimeFiles {
		for _, removed := range []string{
			"elements.idfInput",
			"syncRawTextToggle",
			"syncTextRawPosition",
			"syncRawTextTo",
			"syncTextViewFromRawCaret",
			"restoreRawEditorPosition",
			"rawSelectionStart",
			"rawSelectionEnd",
			"rawScrollTop",
			"rawScrollLeft",
			"inputRawSplitter",
			"idfAnalyzer.rawHeight",
			"--raw-height",
			"autoAnalyzeDelayMs",
			"scheduleAutoAnalyze",
			"autoAnalyzeTimer",
		} {
			if strings.Contains(content, removed) {
				t.Fatalf("%s still contains removed Raw Text runtime contract %q", name, removed)
			}
		}
	}

	settingsPage := readTestFile(t, "frontend/src/settings.html")
	settingsClient := runtimeFiles["settings"]
	app := readTestFile(t, "app.go")
	for name, content := range map[string]string{
		"settings page":    settingsPage,
		"settings client":  settingsClient,
		"backend settings": app,
	} {
		for _, removed := range []string{"syncRawTextPosition", "autoAnalyzeDelayMs"} {
			if strings.Contains(content, removed) {
				t.Fatalf("%s still exposes removed Raw Text setting %q", name, removed)
			}
		}
	}
	for _, removed := range []string{"SyncRawTextPosition", "AutoAnalyzeDelayMS"} {
		if strings.Contains(app, removed) {
			t.Fatalf("backend settings still contain removed Go field %q", removed)
		}
	}

	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	for _, removed := range []string{
		`"behavior.rawSync"`,
		`"behavior.autoDelay"`,
		`"input.rawText"`,
		`"input.syncPosition"`,
		`"status.autoComplete"`,
		`"status.editingPending"`,
		`"status.editingPausedQueued"`,
	} {
		if strings.Contains(i18n, removed) {
			t.Fatalf("translations still expose removed Raw Text behavior %q", removed)
		}
	}

	styles := readTestFile(t, "frontend/src/styles/base.css")
	for _, removed := range []string{
		".raw-editor-block",
		".resizing-input-raw",
		"#inputRawSplitter",
		".sync-toggle",
	} {
		if strings.Contains(styles, removed) {
			t.Fatalf("base styles still contain removed Raw Text selector %q", removed)
		}
	}
}

func TestCanonicalDocumentTextDrivesEditingAnalysisAndPersistence(t *testing.T) {
	stateSource := readTestFile(t, "frontend/src/js/state.js")
	if !strings.Contains(stateSource, `documentText: ""`) {
		t.Fatal("frontend state needs one canonical documentText source")
	}
	for _, helper := range []string{"export function getDocumentText()", "return state.documentText", "export function setDocumentText(text)", "state.documentText ="} {
		if !strings.Contains(stateSource, helper) {
			t.Fatalf("canonical document text accessor contract is missing %q", helper)
		}
	}
	textStats := sliceBetween(stateSource, "export function updateTextStats", "}")
	if !strings.Contains(textStats, "getDocumentText()") {
		t.Fatal("line statistics must read the canonical documentText source")
	}

	actions := readTestFile(t, "frontend/src/js/actions.js")
	for _, contract := range []struct {
		name  string
		start string
		end   string
	}{
		{name: "analysis", start: "export async function analyze", end: "async function runAnalysis"},
		{name: "save", start: "export async function saveInputFile", end: "export async function revertToLoadedDocument"},
		{name: "Metrics export", start: "export async function exportMetrics", end: "export async function openGuide"},
		{name: "workspace snapshot", start: "export async function saveWorkspaceSnapshot", end: "export function applyCachedAnalysisResult"},
	} {
		body := sliceBetween(actions, contract.start, contract.end)
		if !strings.Contains(body, "getDocumentText()") {
			t.Fatalf("%s must consume the canonical documentText accessor", contract.name)
		}
		if strings.Contains(body, "elements.idfInput") {
			t.Fatalf("%s still consumes the removed Raw Text textarea", contract.name)
		}
	}

	inputViews := readTestFile(t, "frontend/src/js/views/input-views.js")
	for _, contract := range []struct {
		name  string
		start string
		end   string
	}{
		{name: "Semantic duplicate repair", start: "async function applySemanticDuplicateFixes", end: "function semanticDisplayScalar"},
		{name: "JSON edit", start: "async function commitJSONValueEdit", end: "function renderFormattedObject"},
		{name: "Semantic/Text/Table field edit", start: "async function applyFieldValue", end: "export async function switchInputView"},
	} {
		body := sliceBetween(inputViews, contract.start, contract.end)
		if !strings.Contains(body, "getDocumentText()") || !strings.Contains(body, "setDocumentText(") {
			t.Fatalf("%s must read and update the canonical documentText source", contract.name)
		}
		if strings.Contains(body, "elements.idfInput") {
			t.Fatalf("%s still uses the removed Raw Text textarea", contract.name)
		}
	}

	for _, path := range []string{
		"frontend/src/js/views/hvac-views.js",
		"frontend/src/js/views/profile-views.js",
		"frontend/src/js/views/simulation-views.js",
	} {
		content := readTestFile(t, path)
		if !strings.Contains(content, "getDocumentText()") {
			t.Fatalf("%s does not consume the canonical document text", path)
		}
		if strings.Contains(content, "elements.idfInput") {
			t.Fatalf("%s still consumes the removed Raw Text textarea", path)
		}
	}
}
