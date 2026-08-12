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
func TestDiagnoseLivesInToolsAndUsesCurrentDocumentSnapshot(t *testing.T) {
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
		"ScanCleanupText", "PreviewCleanupText", "SaveCleanupAs", "persistDiagnoseDocument()",
	} {
		if !strings.Contains(toolsJS, required) {
			t.Fatalf("Tools Diagnose behavior is missing %q", required)
		}
	}
	restoreBody := sliceBetween(toolsJS, "function restoreDiagnoseDocument()", "function setDiagnoseDocument")
	if !strings.Contains(restoreBody, "setDiagnoseDocument(saved, { persist: false })") {
		t.Fatal("Tools Diagnose hydration must preserve the main workspace snapshot and analysis cache key")
	}
	setDocumentBody := sliceBetween(toolsJS, "function setDiagnoseDocument", "async function selectDiagnoseInput")
	for _, required := range []string{"persist = true", "replaceWorkspace = false", "if (persist)", "persistDiagnoseDocument({ replaceWorkspace })"} {
		if !strings.Contains(setDocumentBody, required) {
			t.Errorf("Tools Diagnose document changes are missing persistence contract %q", required)
		}
	}
	selectBody := sliceBetween(toolsJS, "async function selectDiagnoseInput", "async function refreshDiagnose")
	for _, required := range []string{`typeof result?.text === "string"`, `setDiagnoseDocument(result, { replaceWorkspace: true })`, `setDiagnoseDocument({ text: await file.text(), filename: file.name, path: "" }, { replaceWorkspace: true })`} {
		if !strings.Contains(selectBody, required) {
			t.Errorf("Tools Diagnose input replacement is missing %q", required)
		}
	}
	persistBody := sliceBetween(toolsJS, "function persistDiagnoseDocument", "function initializeDiagnoseSelection")
	for _, required := range []string{`analysisKey: ""`, `textHash: ""`, `analysisStage: "idle"`, "geometryReady: false"} {
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
