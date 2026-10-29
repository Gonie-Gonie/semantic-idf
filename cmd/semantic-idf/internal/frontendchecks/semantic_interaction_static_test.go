package frontendchecks

import (
	"regexp"
	"strings"
	"testing"
)

func TestSemanticOccurrenceChooserPriorityAndMemoryContract(t *testing.T) {
	controller := readTestFile(t, "frontend/src/js/selection-controller.js")
	chooser := readTestFile(t, "frontend/src/js/navigation-chooser.js")
	for _, required := range []string{
		"export function chooseSemanticOccurrence",
		"occurrenceId",
	} {
		if !strings.Contains(chooser, required) {
			t.Fatalf("occurrence chooser priority contract missing %q", required)
		}
	}
	for _, required := range []string{
		"const rememberedOccurrences = new Map()",
		"getCurrentSemanticContext",
		"originView",
		"currentOccurrenceId",
		"currentPath",
		"contextKind",
		"semanticPathHint",
		"rememberOccurrenceSelection(",
		"rememberedOccurrences.set(",
		"options.rememberForOriginView || options.originView",
		`=== "definition"`,
	} {
		if !strings.Contains(controller, required) {
			t.Fatalf("controller-owned occurrence priority/memory contract missing %q", required)
		}
	}
	score := sliceBetween(controller, "function occurrenceChoiceScore", "function occurrenceMatchesOrigin")
	origin := strings.Index(score, "score += 1_000_000_000;")
	current := strings.Index(score, "score += 2_000_000;")
	section := strings.Index(score, "score += 1_000_000;")
	remembered := strings.Index(score, "score += 1_000;")
	canonical := strings.Index(score, "score += 1;")
	if origin < 0 || current < 0 || section < 0 || remembered < 0 || canonical < 0 ||
		!(origin < current && current < section && section < remembered && remembered < canonical) {
		t.Fatal("occurrence ordering must prefer origin context, current occurrence/section, memory, then canonical definition")
	}
	choices := sliceBetween(controller, "function semanticOccurrenceChoices", "async function remapSemanticSelection")
	if !strings.Contains(choices, ".sort(") {
		t.Fatal("occurrence choices must be returned in deterministic priority order")
	}
	selectOccurrence := sliceBetween(controller, "async function selectSemanticEntity", "async function hoverSemanticEntity")
	for _, required := range []string{
		"if (chooseOccurrence || !selection.occurrenceId)",
		"const occurrences = semanticOccurrenceChoices(selection, options)",
		"let occurrence = occurrences[0]",
		"if (chooseOccurrence && occurrences.length > 1)",
		"invokeChooser(dependencies.chooseSemanticOccurrence, payload)",
		"occurrences.find((entry) => entry.occurrenceId === occurrenceId)",
		"occurrenceId: occurrence.occurrenceId",
	} {
		if !strings.Contains(selectOccurrence, required) {
			t.Fatalf("source-independent occurrence selection contract missing %q", required)
		}
	}
	fallback := strings.Index(selectOccurrence, "let occurrence = occurrences[0]")
	explicitChooser := strings.Index(selectOccurrence, "invokeChooser(dependencies.chooseSemanticOccurrence, payload)")
	cancelChooser := strings.Index(selectOccurrence, "if (!occurrence)")
	selectionMutation := strings.Index(selectOccurrence, "controllerState.globalSelection = selection")
	if !(fallback < explicitChooser && explicitChooser < cancelChooser && cancelChooser < selectionMutation) ||
		!strings.Contains(sliceBetween(selectOccurrence, "if (!occurrence)", "if (occurrence) selection"), "return null") {
		t.Fatal("deterministic fallback must precede explicit occurrence choosing, and cancelled/invalid choices must stop before selection changes")
	}
}

func TestSemanticEditReanalysisRestoresIdentityOccurrenceAndPanelContext(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/input-views.js")
	controller := readTestFile(t, "frontend/src/js/selection-controller.js")
	main := readTestFile(t, "frontend/src/js/main.js")
	for _, required := range []string{
		"export const remapSemanticSelection",
		"currentSemanticSelection()",
		"entityId",
		"occurrenceId",
		"sourceAnchor",
		"objectType",
		"objectName",
		"fieldName",
		"fieldIndex",
		"sourceFallbackOccurrence",
		"nearestParentOccurrence",
		"selectSemanticEntity(",
	} {
		if !strings.Contains(controller, required) {
			t.Fatalf("semantic edit selection restore contract missing %q", required)
		}
	}
	restore := sliceBetween(controller, "async function remapSemanticSelection", "async function selectSemanticEntity")
	if restore == "" {
		t.Fatal("selection controller must expose a post-analysis semantic remap operation")
	}
	if !strings.Contains(restore, "recordHistory: false") {
		t.Fatal("edit/reanalysis restore must not append a navigation history item")
	}
	if !strings.Contains(restore, "follow: false") || strings.Contains(restore, "openViewWithinTransaction(") {
		t.Fatal("selection remap must preserve the current right-panel context without navigating it")
	}
	if !strings.Contains(restore, "nearestParentOccurrence(") {
		t.Fatal("deleted or renamed targets need a nearest-parent fallback")
	}
	if !strings.Contains(controller, "sourceAnchorMatchesRenamedIdentity") || !strings.Contains(main, "allowRenamedSourceIndex") {
		t.Fatal("a captured same-size semantic edit must safely remap a renamed object by stable index, type, and field identity")
	}
	analysisComplete := sliceBetween(main, "const resumePendingNavigationAfterRender", `window.addEventListener("idfAnalyzer:analysisComplete"`)
	remapIndex := strings.Index(analysisComplete, "await remapSemanticSelection(")
	resumeIndex := strings.Index(analysisComplete, "await resumePendingSemanticNavigation(")
	if remapIndex < 0 || resumeIndex < 0 || remapIndex > resumeIndex {
		t.Fatal("analysis completion must remap the edited selection before resuming any pending target")
	}
	if !strings.Contains(analysisComplete, `event.type !== "idfAnalyzer:analysisComplete"`) {
		t.Fatal("selection remapping must wait for the complete replacement projection")
	}
	if !strings.Contains(views, "idfAnalyzer:semanticSelectionRemapped") {
		t.Fatal("visible source editors must reveal the remapped occurrence after edit/reanalysis")
	}
	for _, status := range []string{
		"semantic.selectionMovedAfterRename",
		"semantic.selectionMovedToParent",
		"semantic.selectionClearedAfterEdit",
	} {
		if !strings.Contains(main, status) {
			t.Fatalf("edit/reanalysis remap must report its user-visible outcome, missing %q", status)
		}
	}
}

func TestSemanticInteractionDoesNotMutateSelectionOrRunAnalyzerDirectly(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/input-views.js")
	directSelectionMutation := regexp.MustCompile(`\bstate\s*\.\s*globalSelection(?:\s*\.\s*[A-Za-z_$][A-Za-z0-9_$]*)?\s*=(?:[^=>]|$)`)
	if directSelectionMutation.MatchString(views) {
		t.Fatal("source editors must mutate globalSelection only through the selection controller")
	}

	navigation := readTestFile(t, "frontend/src/js/navigation.js")
	interaction := sliceBetween(navigation, "export function handleInputSelectionActivation", "export function refreshInputSelectionStyles")
	if interaction == "" {
		t.Fatal("visible source selection delegation is missing")
	}
	analyzerCall := regexp.MustCompile(`(?i)\b(?:analyze|analyzeinput|analyzeinputstagetext|scheduleanalyze)\s*\(`)
	if analyzerCall.MatchString(interaction) || strings.Contains(interaction, "analyzeCallback(") {
		t.Fatal("click, hover, open, chips, chooser, and reveal navigation must not start analysis")
	}
	if strings.Contains(interaction, "recordViewHistory(") {
		t.Fatal("semantic interactions must not duplicate controller-owned navigation history")
	}
}

func TestResultSelectionPreservesTheCurrentInputViewWithoutASeparateSemanticRevealAction(t *testing.T) {
	controller := readTestFile(t, "frontend/src/js/selection-controller.js")
	main := readTestFile(t, "frontend/src/js/main.js")
	index := readTestFile(t, "frontend/src/index.html")
	styles := readTestFile(t, "frontend/src/styles/base.css")
	for _, required := range []string{
		"getActiveInputView",
		"sourceView(callGetter(dependencies.getActiveInputView))",
		"revealViewWithinTransaction(activeInputView",
		`action: "select"`,
	} {
		if !strings.Contains(controller, required) {
			t.Fatalf("linked result selection must highlight the current source view without forcing Semantic, missing %q", required)
		}
	}
	for _, removed := range []string{"input-semantic", "revealSemanticWithinTransaction", "reveal_semantic"} {
		if strings.Contains(controller, removed) {
			t.Fatalf("selection still routes through a removed Semantic view: %q", removed)
		}
	}
	for name, content := range map[string]string{
		"main":   main,
		"markup": index,
		"styles": styles,
	} {
		for _, removed := range []string{
			"semanticRevealIndicator",
			"semantic-reveal-indicator",
			"idfAnalyzer:semanticRevealAvailable",
			"updateSemanticRevealIndicator",
			"semantic.revealInSemantic",
			"Reveal in Semantic",
		} {
			if strings.Contains(content, removed) {
				t.Fatalf("%s still exposes removed standalone Semantic reveal action %q", name, removed)
			}
		}
	}
	inputViews := readTestFile(t, "frontend/src/js/views/input-views.js")
	switchView := sliceBetween(inputViews, "export async function switchInputView", "export function setTableOrientation")
	for _, required := range []string{
		"currentSemanticSelection()",
		"getPanelNavigationAdapter(`input-${viewName}`)",
		`action: "view_switch"`,
		"preserveFilters: true",
	} {
		if !strings.Contains(switchView, required) {
			t.Fatalf("input-view switching must preserve and reveal the same semantic/source selection, missing %q", required)
		}
	}
}

func TestTextJSONAndTableClicksCommitTheSameSemanticSelection(t *testing.T) {
	navigation := readTestFile(t, "frontend/src/js/navigation.js")
	activation := sliceBetween(navigation, "export function handleInputSelectionActivation", "export function refreshInputSelectionStyles")
	for _, required := range []string{
		"getPanelNavigationAdapter(viewId)",
		"adapter?.selectFromElement?.(element)",
		"selectSemanticEntity(selection",
		"recordHistory: true",
	} {
		if !strings.Contains(activation, required) {
			t.Fatalf("source input views must commit through the global semantic controller, missing %q", required)
		}
	}
	if strings.Contains(activation, "follow: false") {
		t.Fatal("source input view selections must use the always-on follow default")
	}
	styles := sliceBetween(navigation, "export function refreshInputSelectionStyles", "let inputAdaptersInitialized")
	for _, required := range []string{"selection?.sourceAnchor", "data-object-index", "semantic-selected", "data-semantic-selected"} {
		if !strings.Contains(styles, required) {
			t.Fatalf("source input views must retain the global selection styling, missing %q", required)
		}
	}
	main := readTestFile(t, "frontend/src/js/main.js")
	if !strings.Contains(main, "handleInputSelectionActivation(event.target)") || !strings.Contains(main, "refreshInputSelectionStyles(selection)") {
		t.Fatal("application delegation must wire source-view selection and background highlighting")
	}
}
