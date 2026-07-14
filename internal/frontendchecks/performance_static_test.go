package frontendchecks

import (
	"strings"
	"testing"
)

func TestFrontendPerformanceStageQueueContracts(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/actions.js")
	for _, term := range []string{
		"let activeStageQueue = null",
		"pending: stages.map((stage, index) => ({ stage, index }))",
		"prioritize(stage)",
		"this.pending.unshift(task)",
		"export function prioritizeAnalysisStageForTab",
		"activeStageQueue.prioritize(stage)",
		"maxFrontendStageConcurrency = 2",
		"function stageStatusMessage",
		"Resolving HVAC service paths",
	} {
		if !strings.Contains(content, term) {
			t.Fatalf("stage queue priority contract missing %q", term)
		}
	}
	navigation := readTestFile(t, "frontend/src/js/navigation.js")
	if !strings.Contains(navigation, "prioritizeAnalysisStageForTab(state.activeResultTab)") {
		t.Fatalf("result tab switching should promote pending stage analysis")
	}
}

func TestFrontendPerformanceTimingContracts(t *testing.T) {
	stateContent := readTestFile(t, "frontend/src/js/state.js")
	for _, term := range []string{
		"analysisTiming: null",
		"analysisStageTimings: {}",
		"renderTiming:",
		"export function refreshStatusTitle",
		"formatAnalysisTiming",
		"Last render:",
	} {
		if !strings.Contains(stateContent, term) {
			t.Fatalf("status timing contract missing %q", term)
		}
	}
	views := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	for _, term := range []string{
		"recordRenderTiming(tab",
		"function renderPendingResultTab",
		"Profile pending",
		"HVAC pending",
		"performance.now",
		"refreshStatusTitle()",
	} {
		if !strings.Contains(views, term) {
			t.Fatalf("render timing contract missing %q", term)
		}
	}
}

func TestFrontendGeometryPlanLayoutCacheContract(t *testing.T) {
	stateContent := readTestFile(t, "frontend/src/js/state.js")
	if !strings.Contains(stateContent, "geometryPlanLayoutCache: new Map()") {
		t.Fatalf("state should include geometry plan layout cache")
	}
	geometry := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	for _, term := range []string{
		"function cachedGeometryPlanLayout",
		"function geometryPlanLayoutCacheKey",
		"function buildGeometryPlanLayout",
		"cache.size > 8",
		"hasPlanVertices",
	} {
		if !strings.Contains(geometry, term) {
			t.Fatalf("geometry plan cache contract missing %q", term)
		}
	}
}

func TestFrontendNavigationCacheRestoreContract(t *testing.T) {
	actions := readTestFile(t, "frontend/src/js/actions.js")
	for _, term := range []string{
		"export async function openBatch()",
		"export async function openSettings()",
		"await saveWorkspaceSnapshot()",
		"analysisKey,",
		"window.sessionStorage.setItem(currentDocumentStorageKey, JSON.stringify(snapshot))",
		"export function applyCachedAnalysisResult",
	} {
		if !strings.Contains(actions, term) {
			t.Fatalf("workspace snapshot contract missing %q", term)
		}
	}
	snapshotBody := sliceBetween(actions, "export async function saveWorkspaceSnapshot()", "export function applyCachedAnalysisResult")
	if strings.Contains(snapshotBody, "report") {
		t.Fatalf("workspace snapshot should not store full report payload")
	}

	main := readTestFile(t, "frontend/src/js/main.js")
	restoreBody := sliceBetween(main, "async function restoreCachedDocumentAnalysis", "function restoreCurrentDocument")
	for _, term := range []string{
		"async function restoreCachedDocumentAnalysis",
		"api.GetCachedAnalysis(restoredDocument.analysisKey)",
		"applyCachedAnalysisResult(cached, restoredDocument)",
		"preferCache: Boolean(restoredDocument.analysisKey)",
	} {
		if !strings.Contains(restoreBody, term) {
			t.Fatalf("restore cache contract missing %q", term)
		}
	}
	if strings.Index(restoreBody, "api.GetCachedAnalysis(restoredDocument.analysisKey)") > strings.Index(restoreBody, "scheduleAnalyzeAfterPaint({") {
		t.Fatalf("restore should check backend cache before scheduling analysis")
	}
}

func TestFrontendContextualNavigationShortcutContracts(t *testing.T) {
	main := readTestFile(t, "frontend/src/js/main.js")
	for _, term := range []string{
		"initializeHVACControls",
		"function handleUndoShortcut(event)",
		"function handleRedoShortcut(event)",
		"undoViewNavigation();",
		"redoViewNavigation();",
		"function handleAnalysisTabCycleKey(event)",
		`event.key !== "PageUp" && event.key !== "PageDown"`,
		"switchResultTabByOffset(event.key === \"PageUp\" ? -1 : 1)",
		"function handleHardwareHistoryKey(event)",
		`event.key === "BrowserBack"`,
		`event.key === "BrowserForward"`,
		"function handleHardwareHistoryMouseButton(event)",
		"event.button !== 3 && event.button !== 4",
	} {
		if !strings.Contains(main, term) {
			t.Fatalf("contextual navigation contract missing %q", term)
		}
	}

	navigation := readTestFile(t, "frontend/src/js/navigation.js")
	for _, term := range []string{
		"export async function undoViewNavigation(options = {})",
		"export async function redoViewNavigation(options = {})",
		"export async function restoreViewSnapshot(snapshot, options = {})",
		`const scope = options.scope || "all"`,
		`scope !== "input" && snapshot.resultTab`,
	} {
		if !strings.Contains(navigation, term) {
			t.Fatalf("scoped view history contract missing %q", term)
		}
	}

	shortcuts := readTestFile(t, "frontend/src/js/shortcuts.js")
	if !strings.Contains(shortcuts, "action(event)") {
		t.Fatalf("keyboard shortcut dispatcher should pass the key event to contextual actions")
	}
}

func TestFrontendHVACDebugRuleGraphLoadsExplicitly(t *testing.T) {
	app := readTestFile(t, "app.go")
	if !strings.Contains(app, `"hvac-debug"`) || !strings.Contains(app, "slimReportForMode") {
		t.Fatalf("stage normalization should expose explicit hvac-debug mode")
	}
	hvac := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	for _, term := range []string{
		"function requestHVACDebugRuleGraph",
		`AnalyzeInputStageText(elements.idfInput.value || "", "hvac-debug")`,
		"Loading debug rule graph",
		"hvacDebugRuleGraphEmptyKey",
	} {
		if !strings.Contains(hvac, term) {
			t.Fatalf("HVAC debug lazy-load contract missing %q", term)
		}
	}
}

func sliceBetween(text, start, end string) string {
	startIndex := strings.Index(text, start)
	if startIndex < 0 {
		return ""
	}
	endIndex := strings.Index(text[startIndex:], end)
	if endIndex < 0 {
		return text[startIndex:]
	}
	return text[startIndex : startIndex+endIndex]
}
