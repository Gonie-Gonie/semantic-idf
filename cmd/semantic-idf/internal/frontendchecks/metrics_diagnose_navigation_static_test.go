package frontendchecks

import (
	"strings"
	"testing"
)

func TestMetricsUseSemanticGroupsAndSeparateContributingSources(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	index := readTestFile(t, "frontend/src/index.html")
	if strings.Contains(index, "metricsFilter") || strings.Contains(content, "metricsFilter") || strings.Contains(content, "metricMatchesQuery") {
		t.Fatal("Metrics must render all metrics without a metric filter control or filtering path")
	}
	for _, required := range []string{
		"bindMetricsTableLayout()",
		"new ResizeObserver",
		"requestAnimationFrame",
		"entry.contentRect.width",
		"Math.min(4",
		"rowsPerColumn",
		"Math.floor(index / rowsPerColumn)",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("metrics table header is missing %q", required)
		}
	}
	if !strings.Contains(content, `class="metrics-row-grid"`) {
		t.Fatal("metrics metric cells must use a dedicated grid inside the native metrics toggle")
	}
	renderer := sliceBetween(content, "function renderMetricRow", "function isNumericMetric")
	for _, required := range []string{
		`<details class="metrics-metric"`,
		`<summary class="metrics-row navigable-row"`,
		`class="metrics-source-drawer"`,
		"metricNavigation(metric, category)",
		"metricContributingSources(metric)",
		"panelTargetId: metric.id",
		"data-metric-id",
		"renderMetricsSourceChooser(contributingSources, metric)",
		`title="${escapeHTML(String(metric.displayValue ?? "—"))}"`,
	} {
		if !strings.Contains(renderer, required) {
			t.Fatalf("metrics metric navigation renderer is missing %q", required)
		}
	}
	if strings.Contains(renderer, "contributingSources[0]") {
		t.Fatal("an aggregate metrics metric must not masquerade its first contributing object as the primary entity")
	}

	mapping := sliceBetween(content, "function metricNavigation", "function metricsSourceRecords")
	for _, required := range []string{
		`navigationSelectionForViewTarget("metrics"`,
		`"zones"`,
		`"geometry"`,
		`"loads"`,
		`"profiles"`,
		`"hvac"`,
		`"services"`,
		`"outputs"`,
		`"diagnostics"`,
	} {
		if !strings.Contains(mapping, required) {
			t.Fatalf("metrics metric-to-section mapping is missing %q", required)
		}
	}
	resolver := sliceBetween(content, "function navigationSelectionForViewTarget", "function preferredSectionRank")
	for _, required := range []string{"navigation.byViewTarget", `entity.kind === "semantic-section"`, "preferredSectionRank"} {
		if !strings.Contains(resolver, required) {
			t.Fatalf("metrics primary entity must be resolved from backend navigation groups, missing %q", required)
		}
	}
	chooser := sliceBetween(content, "function renderMetricsSourceChooser", "function sourceAnchorLabel")
	for _, required := range []string{
		"Source objects",
		"metrics-source-object-list",
		"panelNavigationAttributes({",
		"...source.navigation",
		"metricSourcePanelTargetID(metric, source, index)",
	} {
		if !strings.Contains(chooser, required) {
			t.Fatalf("metrics contributing-source chooser is missing %q", required)
		}
	}
	if strings.Contains(chooser, `class="badge"`) || strings.Contains(chooser, "escapeHTML(sources.length)") {
		t.Fatal("collapsed Metrics rows must not expose contributing source-object counts")
	}
}
func TestDiagnoseLivesInToolsAndUsesCurrentWorkspaceAndSnapshotFallback(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	toolsHTML := readTestFile(t, "frontend/src/tools.html")
	toolsJS := readTestFile(t, "frontend/src/js/tools.js")
	for _, removed := range []string{`data-result-tab="diagnose"`, `id="diagnosePane"`, `id="diagnosticList"`} {
		if strings.Contains(index, removed) {
			t.Fatalf("main analysis workspace still contains Diagnose contract %q", removed)
		}
	}
	for _, required := range []string{
		`data-tools-panel="diagnose"`, `id="diagnoseSelectInput"`, `id="diagnoseRefresh"`,
		`id="diagnosePreview"`, `id="diagnoseApply"`, `id="diagnoseSaveAs"`,
		`id="diagnoseRules"`, `id="diagnoseCandidates"`, `id="diagnoseList"`,
	} {
		if !strings.Contains(toolsHTML, required) {
			t.Fatalf("Tools Diagnose markup is missing %q", required)
		}
	}
	for _, required := range []string{
		`const CURRENT_DOCUMENT_STORAGE_KEY = "idfAnalyzer.currentDocument"`,
		"restoreDiagnoseDocument()", "OpenInputFile", "AnalyzeInputDiagnosticsText",
		"ScanCleanupText", "PreviewCleanupText", "SaveCleanupAs", "function persistDiagnoseDocument(",
		`"idfAnalyzer:auxiliaryShown"`, "syncDiagnoseFromHost(event.detail?.document)",
	} {
		if !strings.Contains(toolsJS, required) {
			t.Fatalf("Tools Diagnose behavior is missing %q", required)
		}
	}
	restoreBody := sliceBetween(toolsJS, "function restoreDiagnoseDocument()", "function setDiagnoseDocument")
	for _, required := range []string{
		"const hostedDocument = getAuxiliaryHost()?.getDocument()",
		"setDiagnoseDocument(hostedDocument, { persist: false, analyze: false })",
		"JSON.parse(window.sessionStorage.getItem(CURRENT_DOCUMENT_STORAGE_KEY)",
	} {
		if !strings.Contains(restoreBody, required) {
			t.Errorf("Tools Diagnose must prefer the live Main input with standalone snapshot fallback, missing %q", required)
		}
	}
	if strings.Index(restoreBody, "if (hostedDocument)") > strings.Index(restoreBody, "window.sessionStorage.getItem") {
		t.Fatal("embedded Tools must restore Main's live input before reading a stale workspace snapshot")
	}
	if !strings.Contains(restoreBody, "setDiagnoseDocument(saved, { persist: false, analyze: false })") {
		t.Fatal("standalone Tools Diagnose hydration must preserve the workspace snapshot/cache key without starting hidden analysis")
	}
	setDocumentBody := sliceBetween(toolsJS, "function setDiagnoseDocument", "async function selectDiagnoseInput")
	for _, required := range []string{"persist = true", "replaceWorkspace = false", "expected = captureDiagnoseWorkspaceBaseline()", "if (persist)", "if (!persistDiagnoseDocument({ replaceWorkspace, expected })) return false"} {
		if !strings.Contains(setDocumentBody, required) {
			t.Errorf("Tools Diagnose document changes are missing persistence contract %q", required)
		}
	}
	for _, selection := range []struct {
		name, body, asynchronousRead, replacement string
	}{
		{"native input", sliceBetween(toolsJS, "async function selectDiagnoseInput", "async function loadDiagnoseBrowserFile"), "await waitForAppAPI", `setDiagnoseDocument(result, { replaceWorkspace: true, expected })`},
		{"browser input", sliceBetween(toolsJS, "async function loadDiagnoseBrowserFile", "async function refreshDiagnose"), "await file.text()", `setDiagnoseDocument({ text, filename: file.name, path: "" }, { replaceWorkspace: true, expected })`},
		{"cleanup", sliceBetween(toolsJS, "async function applyDiagnoseFixes", "async function saveDiagnoseCopy"), "await buildDiagnosePreview", `setDiagnoseDocument({ ...documentState, text: preview.text || documentState.text }, { analyze: false, expected })`},
	} {
		for _, required := range []string{"const expected = captureDiagnoseWorkspaceBaseline()", "const generation = diagnoseDocumentGeneration", "generation !== diagnoseDocumentGeneration", "rejectDiagnoseWorkspaceChange()", selection.replacement} {
			if !strings.Contains(selection.body, required) {
				t.Errorf("Tools Diagnose %s must guard stale asynchronous input changes, missing %q", selection.name, required)
			}
		}
		if strings.Index(selection.body, "const expected =") > strings.Index(selection.body, selection.asynchronousRead) {
			t.Errorf("Tools Diagnose %s must capture Main's expected input before asynchronous work", selection.name)
		}
		if strings.Index(selection.body, "generation !== diagnoseDocumentGeneration") > strings.Index(selection.body, selection.replacement) {
			t.Errorf("Tools Diagnose %s must reject stale responses before replacing the current input", selection.name)
		}
	}
	selectBody := sliceBetween(toolsJS, "async function selectDiagnoseInput", "async function loadDiagnoseBrowserFile")
	if !strings.Contains(selectBody, `typeof result?.text === "string"`) {
		t.Fatal("Tools Diagnose native input replacement must validate the returned text")
	}
	persistBody := sliceBetween(toolsJS, "function persistDiagnoseDocument", "function initializeDiagnoseSelection")
	for _, required := range []string{"const host = getAuxiliaryHost()", "if (host)", "if (!host.applyDocument(diagnoseDocumentIdentity(state.diagnose), { replaceWorkspace, expected }))", "rejectDiagnoseWorkspaceChange()", "return false"} {
		if !strings.Contains(persistBody, required) {
			t.Errorf("embedded Tools Diagnose persistence must hand off to Main with a guarded baseline, missing %q", required)
		}
	}
	if strings.Index(persistBody, "if (host)") > strings.Index(persistBody, "window.sessionStorage.getItem") {
		t.Fatal("embedded Tools Diagnose must hand off to Main before the standalone snapshot persistence path")
	}
	for _, required := range []string{`analysisKey: ""`, `textHash: ""`, `analysisStage: "idle"`, "geometryReady: false", "simulationResultRef: null"} {
		if !strings.Contains(persistBody, required) {
			t.Errorf("Tools Diagnose edits must invalidate stale analysis state via %q", required)
		}
	}
	for _, required := range []string{"if (replaceWorkspace)", "next.loadedText = state.diagnose.text", "next.savedText = state.diagnose.text", "next.globalSelection = null", "next.viewSnapshot = null", "next.panelContexts = {}"} {
		if !strings.Contains(persistBody, required) {
			t.Errorf("Tools Diagnose replacement must clear stale main document context via %q", required)
		}
	}
}
