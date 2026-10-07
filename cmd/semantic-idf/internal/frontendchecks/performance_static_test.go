package frontendchecks

import (
	"strings"
	"testing"
)

func TestFrontendPerformanceStageQueueContracts(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/actions.js") + readTestFile(t, "frontend/src/js/analysis-stage-queue.js")
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
		`t("status.lastRender"`,
	} {
		if !strings.Contains(stateContent, term) {
			t.Fatalf("status timing contract missing %q", term)
		}
	}
	if !strings.Contains(readTranslationSource(t), `"status.lastRender": "Last render: {tab} {duration}"`) {
		t.Fatal("English render timing must preserve its tab and duration placeholders")
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
		{name: "Settings", start: "export async function openSettings()", end: "export async function saveWorkspaceSnapshot()", destination: `openAuxiliaryPage("./settings.html")`},
	}
	for _, navigation := range auxiliaryNavigations {
		body := sliceBetween(actions, navigation.start, navigation.end)
		if !strings.Contains(body, navigation.destination) {
			t.Errorf("%s should open its auxiliary panel inside live Main", navigation.name)
		}
		if strings.Contains(body, "saveWorkspaceSnapshot()") {
			t.Errorf("%s opening must not serialize or hash the workspace before showing its panel", navigation.name)
		}
	}
	if strings.Contains(actions, "window.location.assign(") {
		t.Fatal("auxiliary actions must retain Main's live workspace instead of unloading it")
	}
	if !strings.Contains(actions, `import { openAuxiliaryPage } from "./auxiliary-panel.js"`) {
		t.Fatal("auxiliary actions must use the shared live panel host")
	}
	snapshotBody := sliceBetween(actions, "export async function saveWorkspaceSnapshot()", "export function applyCachedAnalysisResult")
	if strings.Contains(snapshotBody, "report") {
		t.Fatalf("workspace snapshot should not store full report payload")
	}
	if strings.Contains(snapshotBody, "if (!text.trim())") {
		t.Fatal("workspace snapshot must preserve an intentionally empty main document")
	}
	main := readTestFile(t, "frontend/src/js/main.js")
	for _, term := range []string{"initializeAuxiliaryPanel({", "getDocument:", "applyDocument:"} {
		if !strings.Contains(main, term) {
			t.Errorf("Main's live auxiliary workspace hand-off is missing %q", term)
		}
	}
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

	panel := readTestFile(t, "frontend/src/js/auxiliary-panel.js")
	for _, term := range []string{
		`const pageNames = ["settings", "guide", "tools"]`,
		"const frames = new Map()",
		"export function initializeAuxiliaryPanel",
		`document.createElement("dialog")`,
		`dialog.id = "auxiliaryPanel"`,
		`"aria-labelledby", "auxiliaryPanelTitle"`,
		`id="auxiliaryPanelClose"`,
		"data-auxiliary-open",
		"dialog.showModal()",
		"frames.get(page)",
		"if (!frame)",
		`document.createElement("iframe")`,
		"frame.dataset.auxiliaryPage = page",
		"frames.set(page, frame)",
		"item.hidden = name !== page",
		"returnFocus.focus({ preventScroll: true })",
		`"idfAnalyzer:auxiliaryShown", { detail: { document: getDocument() } }`,
		"const expected = options.expected",
		`["text", "path", "filename"].some`,
	} {
		if !strings.Contains(panel, term) {
			t.Errorf("live auxiliary panel contract missing %q", term)
		}
	}
	closeBody := sliceBetween(panel, "export function closeAuxiliaryPanel()", "function notifyShown")
	if !strings.Contains(closeBody, "dialog.close()") {
		t.Error("closing an auxiliary panel must dismiss the native dialog")
	}
	for _, discarded := range []string{"frames.clear(", "frames.delete(", ".remove(", "window.location"} {
		if strings.Contains(closeBody, discarded) {
			t.Errorf("closing an auxiliary panel must retain its cached pages, found %q", discarded)
		}
	}

	context := readTestFile(t, "frontend/src/js/auxiliary-context.js")
	for _, term := range []string{
		"window.parent === window",
		"host?.contains(window)",
		`document.documentElement.dataset.embeddedApp = "true"`,
		`["go", "runtime"]`,
		"Object.defineProperty(window, name",
		"get: () => window.parent[name]",
	} {
		if !strings.Contains(context, term) {
			t.Errorf("embedded auxiliary bridge sharing contract missing %q", term)
		}
	}

	auxiliary := readTestFile(t, "frontend/src/js/auxiliary-navigation.js")
	for _, required := range []string{
		`import { getAuxiliaryHost } from "./auxiliary-context.js"`,
		"const host = getAuxiliaryHost()",
		"host.close()",
		"host.open(link.href)",
	} {
		if !strings.Contains(auxiliary, required) {
			t.Errorf("embedded auxiliary navigation is missing %q", required)
		}
	}
	escapeBody := sliceBetween(auxiliary, `document.addEventListener("keydown"`, "function hasMainHistoryEntry")
	for _, required := range []string{`event.key !== "Escape"`, "event.defaultPrevented", "queueMicrotask", "getAuxiliaryHost()?.close()"} {
		if !strings.Contains(escapeBody, required) {
			t.Errorf("embedded Escape must let feature handlers consume the key before closing, missing %q", required)
		}
	}
	for _, required := range []string{"a[data-app-return]", "a[data-app-auxiliary]", "window.history.back()", "window.location.replace(link.href)"} {
		if !strings.Contains(auxiliary, required) {
			t.Errorf("standalone auxiliary history fallback is missing %q", required)
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
	app := readTestFile(t, "analysis_app.go")
	if !strings.Contains(app, `"hvac-debug"`) || !strings.Contains(app, "slimReportForMode") {
		t.Fatalf("stage normalization should expose explicit hvac-debug mode")
	}
	hvac := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	debugLoad := sliceBetween(hvac, "function requestHVACDebugRuleGraph", "function exportHVACDebugGraph")
	for _, term := range []string{
		"function requestHVACDebugRuleGraph",
		"Loading debug rule graph",
		"hvacDebugRuleGraphEmptyKey",
	} {
		if !strings.Contains(hvac, term) {
			t.Fatalf("HVAC debug lazy-load contract missing %q", term)
		}
	}
	for _, term := range []string{`AnalyzeInputStageText(`, `getDocumentText()`, `"hvac-debug"`} {
		if !strings.Contains(debugLoad, term) {
			t.Fatalf("HVAC debug lazy-load must use the canonical document text, missing %q", term)
		}
	}
	if strings.Contains(debugLoad, "elements.idfInput") {
		t.Fatal("HVAC debug lazy-load still reads the removed Raw Text textarea")
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
