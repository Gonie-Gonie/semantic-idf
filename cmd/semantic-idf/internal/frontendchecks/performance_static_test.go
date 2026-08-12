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
		"Building profile graphs",
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
	geometry := readTestFile(t, "frontend/src/js/views/topology-view.js")
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

func TestFrontendThermalTopologyPerformanceContracts(t *testing.T) {
	topologyView := readTestFile(t, "frontend/src/js/views/topology-view.js")
	if !strings.Contains(topologyView, `import("./thermal-topology-view.js")`) {
		t.Fatal("thermal topology module should load lazily on first use")
	}

	layout := readTestFile(t, "frontend/src/js/views/thermal-topology-layout.js")
	for _, term := range []string{
		"topology.sourceModelHash",
		`options.storyIndex ?? "all"`,
		"applyThermalTopologyLevel",
	} {
		if !strings.Contains(layout, term) {
			t.Fatalf("thermal layout cache key contract missing %q", term)
		}
	}
	for _, removed := range []string{"graphLevel", "areaComponent", "areaField", "neighborDepth", "computeBoundaryLayout", "createBoundaryDetailModel"} {
		if strings.Contains(layout, removed) {
			t.Fatalf("fixed zone/gross topology cache retains dead dimension %q", removed)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, term := range []string{
		"THERMAL_LAYOUT_CACHE_LIMIT = 24",
		"rememberThermalTopologyLayout",
		"renderThermalTopologySVG(currentModel, currentLayout)",
		"createThermalRenderFocusContext",
	} {
		if !strings.Contains(view, term) {
			t.Fatalf("thermal rendering performance contract missing %q", term)
		}
	}
	selectionBody := sliceBetween(view, "function activateGraphTarget", "function applyGraphTransform")
	if strings.Contains(selectionBody, "computeThermalTopologyLayout") || strings.Contains(selectionBody, "renderThermalTopology(") {
		t.Fatal("selection-only updates should not recompute thermal layout")
	}
}

func TestFrontendTopologyLookupAndDelegationPerformanceContracts(t *testing.T) {
	targets := readTestFile(t, "frontend/src/js/thermal-topology-targets.js")
	for _, term := range []string{
		"thermalTopologyLookupCache = new WeakMap()",
		"function createThermalTopologyLookup",
		"boundaryByID: indexFirst",
		"observationByID: indexFirst",
	} {
		if !strings.Contains(targets, term) {
			t.Fatalf("thermal target lookup cache contract missing %q", term)
		}
	}
	resolveBody := sliceBetween(targets, "export function resolveThermalTopologyTarget", "function createThermalTopologyLookup")
	for _, repeatedScan := range []string{"boundaries.find(", "openings.find(", "airCouplings.find(", "nodes.some("} {
		if strings.Contains(resolveBody, repeatedScan) {
			t.Fatalf("thermal target resolution retains repeated collection scan %q", repeatedScan)
		}
	}

	layout := readTestFile(t, "frontend/src/js/views/thermal-topology-layout.js")
	for _, term := range []string{
		"const nodeByID = new Map(",
		"function routeThermalEdgeWithNodeIndex",
		"function connectionNeighbors",
		"function indexConnectionsByNode",
	} {
		if !strings.Contains(layout, term) {
			t.Fatalf("thermal layout indexing contract missing %q", term)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, term := range []string{
		"graphTargetInteractionsBound",
		`graph.addEventListener("pointerover"`,
		"function indexAirCouplingsByConnection",
		"airCouplingsByConnection.get(connection.id)",
		"airCouplingsByConnection?.has(connectionID)",
		"function createThermalSelectionContext",
	} {
		if !strings.Contains(view, term) {
			t.Fatalf("thermal renderer delegation/index contract missing %q", term)
		}
	}
	metricIndex := sliceBetween(view, "function createMetricContext", "function edgeMetricPresentation")
	lookup := sliceBetween(view, "function airCouplingsForConnection", "function connectionTooltip")
	for _, body := range []string{metricIndex, lookup} {
		if strings.Contains(body, "airCouplingsByConnection.get(connection)") {
			t.Fatal("air-coupling indexes must use the stable connection ID because layout edges are cloned")
		}
	}
	details := readTestFile(t, "frontend/src/js/views/thermal-topology-details.js")
	if !strings.Contains(details, "thermalDetailsInteractionsBound") || !strings.Contains(details, `details.addEventListener("click"`) {
		t.Fatal("thermal details should use one delegated interaction listener")
	}

	geometry := readTestFile(t, "frontend/src/js/views/topology-view.js")
	for _, term := range []string{
		"function geometryLookupIndex",
		"function topologySemanticNavigationLookup",
		"topologyPlanInteractionsBound",
		`plan.addEventListener("pointerover"`,
		"topologyDetailInteractionsBound",
	} {
		if !strings.Contains(geometry, term) {
			t.Fatalf("geometry lookup/delegation contract missing %q", term)
		}
	}

	hvac := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	for _, term := range []string{
		"hvacControlsInitialized",
		"function hvacServiceLookup",
		"function semanticHVACNavigationLookup",
		"function indexServiceGraphNeighbors",
	} {
		if !strings.Contains(hvac, term) {
			t.Fatalf("HVAC lookup/listener contract missing %q", term)
		}
	}
}

func TestFrontendNavigationCacheRestoreContract(t *testing.T) {
	actions := readTestFile(t, "frontend/src/js/actions.js")
	for _, term := range []string{
		"export async function openGuide()",
		"export async function openTools()",
		"export async function openSettings()",
		"analysisKey,",
		"window.sessionStorage.setItem(currentDocumentStorageKey, JSON.stringify(snapshot))",
		"export function applyCachedAnalysisResult",
	} {
		if !strings.Contains(actions, term) {
			t.Fatalf("workspace snapshot contract missing %q", term)
		}
	}
	auxiliaryNavigations := []struct {
		name        string
		start       string
		end         string
		destination string
	}{
		{name: "Guide", start: "export async function openGuide()", end: "export async function openTools()", destination: `openAuxiliaryPage("./guide.html")`},
		{name: "Tools", start: "export async function openTools()", end: "export async function openSettings()", destination: `openAuxiliaryPage("./tools.html")`},
		{name: "Settings", start: "export async function openSettings()", end: "function openAuxiliaryPage", destination: `openAuxiliaryPage("./settings.html")`},
	}
	for _, navigation := range auxiliaryNavigations {
		body := sliceBetween(actions, navigation.start, navigation.end)
		saveIndex := strings.Index(body, "await saveWorkspaceSnapshot()")
		assignIndex := strings.Index(body, navigation.destination)
		if saveIndex < 0 || assignIndex < 0 || saveIndex > assignIndex {
			t.Errorf("%s navigation must await the workspace snapshot before leaving main", navigation.name)
		}
	}
	snapshotBody := sliceBetween(actions, "export async function saveWorkspaceSnapshot()", "export function applyCachedAnalysisResult")
	if strings.Contains(snapshotBody, "report") {
		t.Fatalf("workspace snapshot should not store full report payload")
	}
	if strings.Contains(snapshotBody, "if (!text.trim())") {
		t.Fatal("workspace snapshot must preserve an intentionally empty main document")
	}
	auxiliaryBody := sliceBetween(actions, "function openAuxiliaryPage", "export async function saveWorkspaceSnapshot")
	for _, required := range []string{`window.sessionStorage.setItem(auxiliaryNavigationStorageKey, "main")`, "window.location.assign(path)"} {
		if !strings.Contains(auxiliaryBody, required) {
			t.Errorf("auxiliary navigation history fallback is missing %q", required)
		}
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
	restoreDocumentBody := sliceBetween(main, "function restoreCurrentDocument()", "function applyRuntimeSettings")
	if !strings.Contains(restoreDocumentBody, "hasCurrentSnapshot") || strings.Contains(restoreDocumentBody, "documentState.text.trim() ?") {
		t.Fatal("current-schema snapshots must restore even when the main editor is intentionally empty")
	}

	auxiliary := readTestFile(t, "frontend/src/js/auxiliary-navigation.js")
	for _, required := range []string{"a[data-app-return]", "a[data-app-auxiliary]", "window.history.back()", "window.location.replace(link.href)"} {
		if !strings.Contains(auxiliary, required) {
			t.Errorf("live Main history preservation is missing %q", required)
		}
	}
	for _, page := range []string{"frontend/src/tools.html", "frontend/src/guide.html", "frontend/src/settings.html"} {
		markup := readTestFile(t, page)
		for _, required := range []string{"data-app-return", "data-app-auxiliary", `src="./js/auxiliary-navigation.js"`} {
			if !strings.Contains(markup, required) {
				t.Errorf("%s is missing auxiliary navigation contract %q", page, required)
			}
		}
	}
}

func TestFrontendContextualNavigationShortcutContracts(t *testing.T) {
	main := readTestFile(t, "frontend/src/js/main.js")
	linkBarInitialization := sliceBetween(main, "initializeNavigationLinkBar({", "});")
	for _, removed := range []string{"back:", "forward:", "workspaceBackButton", "workspaceForwardButton"} {
		if strings.Contains(linkBarInitialization, removed) {
			t.Fatalf("main link bar retains removed global history button callback %q", removed)
		}
	}
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
