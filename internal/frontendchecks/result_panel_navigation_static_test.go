package frontendchecks

import (
	"regexp"
	"strings"
	"testing"
)

func TestResultPanelNavigationAdaptersContract(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/panel-navigation-adapters.js")
	for _, name := range []string{
		"RESULT_PANEL_NAVIGATION_VIEW_IDS",
		"configureResultPanelNavigationHooks",
		"initializeResultPanelNavigationAdapters",
		"extractResultPanelSelection",
		"refreshResultPanelSelectionStyles",
	} {
		assertJSExport(t, content, name)
	}

	for _, viewID := range []string{"summary", "profile", "hvac", "output", "simulation", "diagnose", "geometry"} {
		if !strings.Contains(content, `"`+viewID+`"`) {
			t.Fatalf("common result adapter is missing view %q", viewID)
		}
	}
	if !strings.Contains(content, "registerPanelNavigationAdapter(viewId, adapter)") {
		t.Fatal("common result adapters must register through the panel navigation registry")
	}
	for _, method := range []string{
		"canReveal(selection)",
		"async reveal(selection, options = {})",
		"selectFromElement(element)",
		"captureContext()",
		"async restoreContext(snapshot = {})",
		"preferredSemanticOccurrence(selection)",
	} {
		if !strings.Contains(content, method) {
			t.Fatalf("common adapter is missing %s", method)
		}
	}
}

func TestResultPanelSelectionUsesStandardMarkupAndBackendReverseIndex(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/panel-navigation-adapters.js")
	for _, attribute := range []string{
		"data-entity-id",
		"data-occurrence-id",
		"data-panel-target-id",
		"data-source-object-id",
		"data-source-object-index",
		"occurrenceContext",
		"sourceFieldIndex",
		"chooseSemanticOccurrence",
	} {
		if !strings.Contains(content, attribute) {
			t.Fatalf("standard selection extraction is missing %q", attribute)
		}
	}
	for _, required := range []string{
		"navigation.byViewTarget",
		"navigation.byObjectId",
		"navigation.byObjectIndex",
		"navigation.byEntityId",
		"findOccurrence(navigation",
		"findEntity(navigation",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("selection reverse lookup is missing %q", required)
		}
	}
	if regexp.MustCompile(`(?i)\b(?:analyze|analyzeinput|scheduleanalyze)\s*\(`).MatchString(content) ||
		strings.Contains(content, `from "./actions.js"`) {
		t.Fatal("panel navigation adapters must never start or import the analyzer")
	}
}

func TestAmbiguousPanelSelectionRequestsAnOccurrenceChoice(t *testing.T) {
	navigation := readTestFile(t, "frontend/src/js/navigation.js")
	activation := sliceBetween(navigation, "export function handleAnalysisActivation", "let legacyInputAdaptersInitialized")
	if !strings.Contains(activation, "chooseOccurrence:") || !strings.Contains(activation, "data-choose-semantic-occurrence") {
		t.Fatal("ambiguous result-panel items must carry their occurrence-choice request into the controller")
	}
}

func TestExplicitPanelSourceActionsWinOverRowSelection(t *testing.T) {
	navigation := readTestFile(t, "frontend/src/js/navigation.js")
	activation := sliceBetween(navigation, "export function handleAnalysisActivation", "export function handleInputSelectionActivation")
	jump := strings.Index(activation, `closest("[data-jump-object-index], [data-jump-object-type]")`)
	selection := strings.Index(activation, "adapter?.selectFromElement?.(element)")
	if jump < 0 || selection < 0 || jump > selection {
		t.Fatal("an explicit Reveal source control must run before generic row selection")
	}
	if !strings.Contains(activation, "interactive.matches") {
		t.Fatal("editing/removal controls inside a selectable row must not accidentally select the row")
	}
}

func TestGenericResultPanelRevealAndCompactContextContract(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/panel-navigation-adapters.js")
	reveal := sliceBetween(content, "async function genericReveal", "function genericCaptureContext")
	for _, required := range []string{
		"expandAncestorDetails(target)",
		"refreshResultPanelSelectionStyles",
		`querySelector?.("[data-semantic-selected]")`,
		"primary.scrollIntoView",
		"primary.focus({ preventScroll: true })",
	} {
		if !strings.Contains(reveal, required) {
			t.Fatalf("generic result reveal is missing %q", required)
		}
	}

	capture := sliceBetween(content, "function genericCaptureContext", "async function genericRestoreContext")
	for _, field := range []string{"scrollTop", "scrollLeft", "targetId", "entityId"} {
		if !strings.Contains(capture, field) {
			t.Fatalf("compact panel context is missing %s", field)
		}
	}
	for _, forbidden := range []string{"report:", "innerHTML", "outerHTML", "semanticProjection:"} {
		if strings.Contains(capture, forbidden) {
			t.Fatalf("panel history context must remain compact: found %q", forbidden)
		}
	}
	restore := sliceBetween(content, "async function genericRestoreContext", "function genericPreferredSemanticOccurrence")
	if !strings.Contains(restore, "expandAncestorDetails(target)") ||
		!strings.Contains(restore, "root.scrollTop") || !strings.Contains(restore, "root.scrollLeft") {
		t.Fatal("generic context restore must restore target expansion and both scroll axes")
	}
}

func TestResultPanelHooksDelegateDynamicallyAndOccurrenceChoiceIsContextual(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/panel-navigation-adapters.js")
	configure := sliceBetween(content, "export function configureResultPanelNavigationHooks", "/** Registers the seven")
	if !strings.Contains(configure, "hooksByView.set(view, configured)") ||
		!strings.Contains(configure, "hooksByView.get(view) !== configured") {
		t.Fatal("hook configuration must be dynamic and cleanup-safe")
	}
	for _, hook := range []string{
		"hooksByView.get(viewId)?.canReveal",
		"hooksByView.get(viewId)?.reveal",
		"hooksByView.get(viewId)?.selectFromElement",
		"hooksByView.get(viewId)?.captureContext",
		"hooksByView.get(viewId)?.restoreContext",
		"hooksByView.get(viewId)?.preferredSemanticOccurrence",
	} {
		if !strings.Contains(content, hook) {
			t.Fatalf("adapter does not dynamically delegate %q", hook)
		}
	}
	preference := sliceBetween(content, "function preferredOccurrence", "function viewTargetIdsForSelection")
	for _, context := range []string{
		"occurrenceContext",
		"viewTargetMatches",
		"state.semanticCurrentOccurrenceId",
		"commonPathPrefixLength",
		"occurrence.preferredView",
	} {
		if !strings.Contains(preference, context) {
			t.Fatalf("semantic occurrence selection is missing context signal %q", context)
		}
	}
}

func TestFrontendReadinessIncludesResultPanelNavigationAdapters(t *testing.T) {
	build := readTestFile(t, "scripts/frontend-build.ps1")
	if !strings.Contains(build, `"panel-navigation-adapters.js"`) {
		t.Fatal("frontend readiness manifest is missing panel-navigation-adapters.js")
	}
	main := readTestFile(t, "frontend/src/js/main.js")
	if !strings.Contains(main, "initializeResultPanelNavigationAdapters()") {
		t.Fatal("the application must register all seven result-panel adapters during startup")
	}
}
