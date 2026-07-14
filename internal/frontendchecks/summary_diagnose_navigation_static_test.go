package frontendchecks

import (
	"strings"
	"testing"
)

func TestSummaryMetricsUseSemanticGroupsAndSeparateContributingSources(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	renderer := sliceBetween(content, "function renderMetricRow", "function isNumericSummaryMetric")
	for _, required := range []string{
		"summaryMetricNavigation(metric, category)",
		"summaryMetricContributingSources(metric)",
		"panelTargetId: metric.id",
		"data-summary-metric-id",
		"renderSummarySourceChooser(contributingSources, metric)",
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
}

func TestDiagnosticItemsCarryStableTargetExactSourceAndContextOccurrence(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	renderer := sliceBetween(content, "function renderDiagnosticItem", "function diagnosticMatchesQuery")
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

func TestDiagnoseContextRestoresOnlySameIssueOrSameSource(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	capture := sliceBetween(content, "export function captureDiagnoseNavigationContext", "export async function restoreDiagnoseNavigationContext")
	for _, required := range []string{
		"selectedDiagnosticID",
		"selectedDiagnosticCode",
		"selectedSemanticEntityID",
		"selectedSemanticOccurrenceID",
		"sourceAnchor",
		"severityFilter",
		"sourceFilter",
		"hiddenDiagnosticCodes",
		"expandedGroupIDs",
		"scrollTop",
	} {
		if !strings.Contains(capture, required) {
			t.Fatalf("diagnose context snapshot is missing %q", required)
		}
	}
	restore := sliceBetween(content, "export async function restoreDiagnoseNavigationContext", "function sourceAnchorFromPanelElement")
	for _, required := range []string{
		"diagnosticStableID(item) === requestedID",
		"snapshot.selectedDiagnosticCode",
		"sourceAnchorsMatch",
		"const resolved = Boolean(requestedID && !matched)",
		"preservingDiagnoseContext = true",
		"diagnostic-resolved-status",
		"Resolved ·",
	} {
		if !strings.Contains(restore, required) {
			t.Fatalf("diagnose post-apply restore is missing %q", required)
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
