package frontendchecks

import (
	"strings"
	"testing"
)

func TestSummaryMetricsUseSemanticGroupsAndSeparateContributingSources(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	index := readTestFile(t, "frontend/src/index.html")
	if strings.Contains(index, "summaryFilter") || strings.Contains(content, "summaryFilter") || strings.Contains(content, "metricMatchesQuery") {
		t.Fatal("Summary must render all metrics without a metric filter control or filtering path")
	}
	for _, required := range []string{
		"bindSummaryTableLayout()",
		"new ResizeObserver",
		"requestAnimationFrame",
		"entry.contentRect.width",
		"Math.min(4",
		"rowsPerColumn",
		"Math.floor(index / rowsPerColumn)",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("summary table header is missing %q", required)
		}
	}
	if !strings.Contains(content, `class="summary-row-grid"`) {
		t.Fatal("summary metric cells must use a dedicated grid inside the native summary toggle")
	}
	renderer := sliceBetween(content, "function renderMetricRow", "function isNumericSummaryMetric")
	for _, required := range []string{
		`<details class="summary-metric"`,
		`<summary class="summary-row navigable-row"`,
		`class="summary-source-drawer"`,
		"summaryMetricNavigation(metric, category)",
		"summaryMetricContributingSources(metric)",
		"panelTargetId: metric.id",
		"data-summary-metric-id",
		"renderSummarySourceChooser(contributingSources, metric)",
		`title="${escapeHTML(String(metric.displayValue ?? "N/A"))}"`,
	} {
		if !strings.Contains(renderer, required) {
			t.Fatalf("summary metric navigation renderer is missing %q", required)
		}
	}
	if strings.Contains(renderer, "contributingSources[0]") {
		t.Fatal("an aggregate summary metric must not masquerade its first contributing object as the primary entity")
	}

	mapping := sliceBetween(content, "function summaryMetricNavigation", "function summarySourceRecords")
	for _, required := range []string{
		`navigationSelectionForViewTarget("summary"`,
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
			t.Fatalf("summary metric-to-section mapping is missing %q", required)
		}
	}
	resolver := sliceBetween(content, "function navigationSelectionForViewTarget", "function preferredSectionRank")
	for _, required := range []string{"navigation.byViewTarget", `entity.kind === "semantic-section"`, "preferredSectionRank"} {
		if !strings.Contains(resolver, required) {
			t.Fatalf("summary primary entity must be resolved from backend navigation groups, missing %q", required)
		}
	}
	chooser := sliceBetween(content, "function renderSummarySourceChooser", "function sourceAnchorLabel")
	for _, required := range []string{
		"Source objects",
		"summary-source-object-list",
		"panelNavigationAttributes({",
		"...source.navigation",
		"summarySourcePanelTargetID(metric, source, index)",
	} {
		if !strings.Contains(chooser, required) {
			t.Fatalf("summary contributing-source chooser is missing %q", required)
		}
	}
	if strings.Contains(chooser, `class="badge"`) || strings.Contains(chooser, "escapeHTML(sources.length)") {
		t.Fatal("collapsed summary metrics must not expose contributing source-object counts")
	}
}

func TestDiagnosticItemsCarryStableTargetExactSourceAndContextOccurrence(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	renderer := sliceBetween(content, "function renderDiagnosticItem", "function bindDiagnosticControls")
	for _, required := range []string{
		"diagnosticStableID(item)",
		"diagnosticSemanticNavigation(item, stableID)",
		"data-diagnostic-stable-id",
		"panelTargetId: stableID",
		"data-diagnostic-reveal-source",
		"Reveal source",
	} {
		if !strings.Contains(renderer, required) {
			t.Fatalf("diagnostic item navigation markup is missing %q", required)
		}
	}
	metadata := sliceBetween(content, "function diagnosticSemanticNavigation", "function sourceNavigationForAnchor")
	for _, required := range []string{
		`navigationSelectionForViewTarget("diagnose", stableID)`,
		"diagnosticEntity.relatedEntityIds",
		"diagnosticOccurrence?.sourceAnchor",
		"contextPath",
		"diagnosticContextPriority",
		"sourceAnchorsMatch",
	} {
		if !strings.Contains(metadata, required) {
			t.Fatalf("diagnostic semantic occurrence resolver is missing %q", required)
		}
	}
	attributes := sliceBetween(content, "function panelNavigationAttributes", "function hasNavigationIndex")
	for _, attribute := range []string{
		"data-entity-id",
		"data-entity-kind",
		"data-occurrence-id",
		"data-occurrence-context",
		"data-source-object-id",
		"data-source-object-index",
		"data-source-field-index",
		"data-panel-target-id",
	} {
		if !strings.Contains(attributes, attribute) {
			t.Fatalf("standard diagnostic/summary navigation metadata is missing %q", attribute)
		}
	}
}

func TestDiagnoseOmitsFilterSurfacesAndPreservesFixActions(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	state := readTestFile(t, "frontend/src/js/state.js")
	main := readTestFile(t, "frontend/src/js/main.js")
	analysis := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	fixes := readTestFile(t, "frontend/src/js/views/diagnose-fixes.js")
	styles := readTestFile(t, "frontend/src/styles/base.css")

	diagnosePane := sliceBetween(index, `<div class="result-pane" id="diagnosePane">`, `<div class="result-pane" id="geometryPane">`)
	for _, removed := range []string{
		`class="summary-head"`,
		`id="diagnosticCount"`,
		`id="diagnosticFilter"`,
	} {
		if strings.Contains(diagnosePane, removed) {
			t.Fatalf("Diagnose must not retain its top summary/filter row control %q", removed)
		}
	}

	fixHead := sliceBetween(diagnosePane, `<div class="diagnose-fix-head">`, `<div class="cleanup-grid diagnose-fix-grid">`)
	for _, required := range []string{
		`id="diagnoseFixRefresh"`,
		`id="diagnoseFixPreview"`,
		`id="diagnoseFixApply"`,
		`id="diagnoseFixSaveAs"`,
		`id="diagnoseFixCandidateFilter"`,
	} {
		if !strings.Contains(fixHead, required) {
			t.Fatalf("Diagnose Fixes header must retain %q", required)
		}
	}
	if !strings.Contains(fixes, `elements.diagnoseFixCandidateFilter?.addEventListener("input"`) {
		t.Fatal("the separate Diagnose Fixes candidate filter must remain functional")
	}

	for label, content := range map[string]string{
		"index":    index,
		"state":    state,
		"main":     main,
		"analysis": analysis,
	} {
		for _, removed := range []string{"diagnosticCount", "diagnosticFilter"} {
			if strings.Contains(content, removed) {
				t.Fatalf("%s must not retain removed Diagnose top-row control %q", label, removed)
			}
		}
	}
	for _, removed := range []string{
		"diagnosticSeverityFilter",
		"diagnosticSourceFilter",
		"hiddenDiagnosticCodes",
		"renderDiagnosticToolbar",
		"renderDiagnosticFilterButton",
		"diagnosticMatchesQuery",
		"diagnosticMatchesControls",
		"diagnosticSourceOptions",
		"data-diagnostic-filter-kind",
		"data-diagnostic-hide-code",
		"data-diagnostic-clear-hidden",
		"Hide code",
		"Show hidden",
	} {
		if strings.Contains(state, removed) || strings.Contains(analysis, removed) {
			t.Fatalf("Diagnose must not retain filter/hide implementation %q", removed)
		}
	}
	for _, removed := range []string{
		".diagnostic-toolbar",
		".diagnostic-filter-row",
		".diagnostic-filter-button",
		".diagnostic-toolbar-status",
		".diagnostic-clear-hidden",
		".diagnostic-code-action",
	} {
		if strings.Contains(styles, removed) {
			t.Fatalf("Diagnose must not retain filter/hide style %q", removed)
		}
	}

	for _, required := range []string{
		"const DIAGNOSTIC_RENDER_LIMIT = 500",
		"items.slice(0, DIAGNOSTIC_RENDER_LIMIT)",
		"diagnoseTemporaryRevealID",
		"temporarilyMaterialized",
		"data-diagnostic-clear-temporary",
		"index >= DIAGNOSTIC_RENDER_LIMIT ? targetID : \"\"",
	} {
		if !strings.Contains(analysis, required) {
			t.Fatalf("Diagnose must retain bounded rendering and semantic temporary reveal, missing %q", required)
		}
	}
}

func TestDiagnoseContextRestoresOnlySameIssueOrSameSource(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	capture := sliceBetween(content, "export function captureDiagnoseNavigationContext", "export async function restoreDiagnoseNavigationContext")
	for _, required := range []string{
		"selectedDiagnosticID",
		"selectedDiagnosticCode",
		"selectedSemanticEntityID",
		"selectedSemanticOccurrenceID",
		"sourceAnchor",
		"temporaryRevealID",
		"expandedGroupIDs",
		"expandedDiagnosticIDs",
		"scrollTop",
		"scrollLeft",
	} {
		if !strings.Contains(capture, required) {
			t.Fatalf("diagnose context snapshot is missing %q", required)
		}
	}
	for _, removed := range []string{"filter:", "severityFilter", "sourceFilter", "hiddenDiagnosticCodes"} {
		if strings.Contains(capture, removed) {
			t.Fatalf("diagnose context snapshot must not retain removed filter field %q", removed)
		}
	}
	restore := sliceBetween(content, "export async function restoreDiagnoseNavigationContext", "function sourceAnchorFromPanelElement")
	for _, required := range []string{
		"diagnosticStableID(item) === requestedID",
		"snapshot.selectedDiagnosticCode",
		"sourceAnchorsMatch",
		"const resolved = Boolean(requestedID && !matched)",
		`diagnoseTemporaryRevealID = snapshot.temporaryRevealID || ""`,
		"renderDiagnostics()",
		"await nextNavigationFrame()",
		"diagnostic-resolved-status",
		"status.textContent = `Resolved",
	} {
		if !strings.Contains(restore, required) {
			t.Fatalf("diagnose post-apply restore is missing %q", required)
		}
	}
	for _, removed := range []string{"snapshot.filter", "snapshot.severityFilter", "snapshot.sourceFilter", "snapshot.hiddenDiagnosticCodes"} {
		if strings.Contains(restore, removed) {
			t.Fatalf("diagnose restore must not retain removed filter field %q", removed)
		}
	}
	if strings.Contains(restore, "diagnostics[0]") {
		t.Fatal("a newly created first diagnostic must not steal post-fix selection")
	}
}

func TestDiagnoseFixPreviewApplyPreserveNavigationContext(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/diagnose-fixes.js")
	for _, required := range []string{
		"captureDiagnoseNavigationContext()",
		"pendingFixNavigationContext = navigationContext",
		"restoreDiagnoseNavigationContext(navigationContext, { afterPreview: true })",
		"restoreDiagnoseNavigationContext(navigationContext, { afterApplyError: true })",
		"restoreDiagnoseNavigationContext(context, { afterApply: true })",
		"idfAnalyzer:analysisComplete",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("diagnose fix context lifecycle is missing %q", required)
		}
	}
	candidate := sliceBetween(content, "function fixCandidateNavigationAttributes", "function fixOccurrencePriority")
	for _, required := range []string{
		"navigation.byObjectIndex",
		"data-entity-id",
		"data-occurrence-context",
		"data-source-object-index",
		"data-panel-target-id",
	} {
		if !strings.Contains(candidate, required) {
			t.Fatalf("fix candidate affected-entity navigation is missing %q", required)
		}
	}
}
