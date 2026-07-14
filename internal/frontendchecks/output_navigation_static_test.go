package frontendchecks

import (
	"strings"
	"testing"
)

func TestOutputRowsUseStableSignatureAndStandardNavigationMarkup(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/output-views.js")
	row := sliceBetween(content, "function renderOutputRequestRow", "function renderOutputField")
	for _, required := range []string{
		"item.signature",
		"outputNavigationAttributes",
		"data-output-signature",
		"navigable-row",
		"tabindex=",
	} {
		if !strings.Contains(row, required) {
			t.Fatalf("output request row is missing %q", required)
		}
	}
	attributes := sliceBetween(content, "function outputNavigationAttributes", "function outputTargetForSelection")
	for _, attribute := range []string{
		"data-entity-id",
		"data-entity-kind",
		"data-occurrence-context",
		"data-source-object-id",
		"data-source-object-index",
		"data-source-field-index",
		"data-panel-target-id",
	} {
		if !strings.Contains(attributes, attribute) {
			t.Fatalf("output navigation markup is missing %q", attribute)
		}
	}
	if !strings.Contains(attributes, "navigation.byViewTarget") {
		t.Fatal("output markup must reverse-map the stable signature through navigation.byViewTarget")
	}
}

func TestOutputAdapterRevealsExactRequestWithoutDestroyingFilters(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/output-views.js")
	adapter := sliceBetween(content, "function initializeOutputNavigation", "function outputNavigationAttributes")
	for _, required := range []string{
		`configureResultPanelNavigationHooks("output"`,
		"canReveal(selection",
		"async reveal(selection",
		"captureContext(context)",
		"async restoreContext(snapshot",
		"preferredSemanticOccurrence(selection",
		"state.outputFocusedSignature",
		"state.outputTemporaryRevealSignature",
		"outputRequestIsVisible(request)",
		"renderOutput()",
		"scrollIntoView",
	} {
		if !strings.Contains(adapter, required) {
			t.Fatalf("output adapter is missing %q", required)
		}
	}
	if strings.Contains(adapter, `elements.outputFilter.value = ""`) || strings.Contains(adapter, `state.outputPurposeFilter = "all"`) {
		t.Fatal("output reveal must temporarily materialize a request instead of clearing user filters")
	}
}

func TestOutputPurposeTagsRemainSemanticActions(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/output-views.js")
	tags := sliceBetween(content, "function renderOutputPurposeTags", "function outputPurposeLabel")
	for _, required := range []string{"outputNavigationAttributes", "data-output-purpose", "navigable-row", "tabindex"} {
		if !strings.Contains(tags, required) {
			t.Fatalf("output purpose tag navigation is missing %q", required)
		}
	}
}
