package frontendchecks

import (
	"regexp"
	"strings"
	"testing"
)

func TestSemanticLineNavigationMetadataAndControllerGestures(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/input-views.js")
	renderLine := sliceBetween(views, "function renderSemanticLine(", "function semanticLineClassNames")
	for _, attribute := range []string{
		"data-entity-id",
		"data-entity-kind",
		"data-occurrence-id",
		"data-semantic-path",
		"data-preferred-view",
		"data-preferred-target-id",
		"data-source-object-id",
	} {
		if !strings.Contains(renderLine, attribute) {
			t.Fatalf("semantic line renderer is missing navigation attribute %q", attribute)
		}
	}
	for _, forbidden := range []string{"data-view-targets", "JSON.stringify(line.viewTargets)"} {
		if strings.Contains(renderLine, forbidden) {
			t.Fatalf("semantic line DOM must resolve view targets from the projection index, found %q", forbidden)
		}
	}

	for _, controllerCall := range []string{
		"selectSemanticEntity(",
		"hoverSemanticEntity(",
		"clearSemanticHover(",
		"openSelectionInView(",
	} {
		if !strings.Contains(views, controllerCall) {
			t.Fatalf("Semantic Text interaction must delegate through %s", controllerCall)
		}
	}
	for _, gesture := range []string{
		`addEventListener("click"`,
		`addEventListener("pointerenter"`,
		`addEventListener("pointerleave"`,
		`addEventListener("dblclick"`,
		`addEventListener("keydown"`,
		`event.key === "Enter"`,
	} {
		if !strings.Contains(views, gesture) {
			t.Fatalf("Semantic Text navigation is missing gesture contract %q", gesture)
		}
	}

	selectionBuilder := sliceBetween(views, "function semanticSelectionForLine", "function selectSemanticLine")
	for _, metadata := range []string{"entityId", "entityKind", "occurrenceId", "semanticPathHint", "sourceAnchor"} {
		if !strings.Contains(selectionBuilder, metadata) {
			t.Fatalf("semantic line selection must be built from navigation identity, missing %q", metadata)
		}
	}
	if !strings.Contains(selectionBuilder, "line.dataset.entityId") {
		t.Fatal("semantic line selection must not require objectIndex when entity identity is present")
	}
}

func TestSemanticLineClickAndPrimaryOpenHaveSingleHistoryBoundary(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/input-views.js")
	interaction := sliceBetween(views, "function selectSemanticLine", "function editSemanticValue")
	if interaction == "" {
		t.Fatal("semantic selection/open interaction helpers are missing")
	}
	for _, required := range []string{
		"selectSemanticEntity(",
		`originView: "input-semantic"`,
		"syncRawTextToFormattedTarget(",
		"openSelectionInView(",
		"preferredView",
		"preferredTargetId",
	} {
		if !strings.Contains(interaction, required) {
			t.Fatalf("semantic selection/open history contract missing %q", required)
		}
	}
	if strings.Contains(interaction, "recordViewHistory(") {
		t.Fatal("semantic line handlers must let the selection controller own the single history item")
	}
	if !strings.Contains(interaction, "recordHistory: false") {
		t.Fatal("primary open must suppress its nested selection/reveal history operation")
	}
	withoutRawHistory := regexp.MustCompile(`syncRawTextToFormattedTarget\([^,\n]+,\s*\{\s*recordHistory:\s*false`)
	rawSync := sliceBetween(views, "function syncRawTextToFormattedTarget", "export function syncRawTextToObjectField")
	if strings.Contains(rawSync, "recordViewHistory(") && !withoutRawHistory.MatchString(interaction) {
		t.Fatal("semantic line selection must suppress the raw-caret helper's otherwise independent history item")
	}

	if strings.Contains(rawSync, "recordViewHistory(") && !strings.Contains(rawSync, "options.recordHistory !== false") {
		t.Fatal("raw caret synchronization needs an explicit recordHistory guard so semantic selection cannot duplicate history")
	}
	if !strings.Contains(rawSync, "state.syncTextRawPosition") {
		t.Fatal("semantic single-click must continue to respect the raw-text sync toggle")
	}
}

func TestSemanticTargetChipsAndSelectionContextContract(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/input-views.js")
	styles := readTestFile(t, "frontend/src/styles/base.css")
	for _, required := range []string{
		"semantic-target-chips",
		"semantic-target-chip",
		"data-semantic-mode=",
		"selectionTargetsForView(",
		"openSelectionInView(",
		"semantic-context-bar",
		"revealSelectionSource(",
		"clearSemanticSelection(",
		"semantic-occurrence-chooser",
	} {
		if !strings.Contains(views, required) {
			t.Fatalf("Semantic Text target/context UI contract missing %q", required)
		}
	}
	context := sliceBetween(views, "function renderSemanticSelectionContext", "function renderSemanticOccurrenceChooser")
	if !regexp.MustCompile(`if\s*\(\s*!selection\??\.entityId\s*\)`).MatchString(context) {
		t.Fatal("semantic selection context bar must be hidden when no entity is selected")
	}
	for _, required := range []string{"sourceAnchor", "Views", "Occurrences", "Reveal source", "Clear"} {
		if !strings.Contains(context, required) {
			t.Fatalf("semantic selection context is missing %q", required)
		}
	}

	for _, selector := range []string{
		".semantic-target-chips",
		".semantic-target-chip",
		".selected",
		".hovered",
		":focus-within",
		`[data-semantic-mode="basic"]`,
		".semantic-context-bar",
	} {
		if !strings.Contains(styles, selector) {
			t.Fatalf("Semantic Text target/context styles missing %q", selector)
		}
	}
	if !strings.Contains(views, "event.stopPropagation()") {
		t.Fatal("semantic target chip actions must not bubble into line selection or editing")
	}
}

func TestSemanticOccurrenceChooserPriorityAndMemoryContract(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/input-views.js")
	controller := readTestFile(t, "frontend/src/js/selection-controller.js")
	chooser := sliceBetween(views, "function renderSemanticOccurrenceChooser", "function ensureSemanticOccurrenceVisible")
	if chooser == "" {
		t.Fatal("multiple semantic occurrences need an explicit chooser")
	}
	for _, required := range []string{
		"semantic-occurrence-chooser",
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
	resolve := sliceBetween(controller, "async function resolveSemanticOccurrence", "function rememberOccurrenceSelection")
	if !strings.Contains(resolve, "occurrences[0]") {
		t.Fatal("occurrence priority must have a deterministic first-occurrence fallback")
	}
}

func TestSemanticTemporaryRevealPreservesUserFiltersAndMode(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/input-views.js")
	stateContent := readTestFile(t, "frontend/src/js/state.js")
	styles := readTestFile(t, "frontend/src/styles/base.css")
	for _, required := range []string{
		"semanticTemporaryReveal",
		"ensureSemanticOccurrenceVisible",
		"clearSemanticTemporaryReveal",
		"semanticExpandedSectionIds.add(",
		"semantic-temporary-reveal",
		"Temporarily revealing selected item",
	} {
		if !strings.Contains(views, required) {
			t.Fatalf("temporary semantic reveal contract missing %q", required)
		}
	}
	if !strings.Contains(stateContent, "semanticTemporaryReveal: null") {
		t.Fatal("temporary reveal state must remain separate from persistent mode/facet/filter state")
	}
	for _, selector := range []string{".semantic-temporary-reveal", ".semantic-temporary-reveal__clear"} {
		if !strings.Contains(styles, selector) {
			t.Fatalf("temporary reveal UI styles missing %q", selector)
		}
	}

	reveal := sliceBetween(views, "function ensureSemanticOccurrenceVisible", "function clearSemanticTemporaryReveal")
	clearReveal := sliceBetween(views, "function clearSemanticTemporaryReveal", "function captureSemanticEditSelection")
	protectedAssignment := regexp.MustCompile(`state\.(?:inputFilterQuery|semanticProjectionFacet|semanticProjectionMode)\s*=`)
	if protectedAssignment.MatchString(reveal) || protectedAssignment.MatchString(clearReveal) {
		t.Fatal("temporary reveal must preserve the user's filter query, facet, and semantic mode")
	}
	if !strings.Contains(clearReveal, "semanticTemporaryReveal = null") {
		t.Fatal("clearing a temporary reveal must restore the already-preserved view state")
	}
	selection := sliceBetween(readTestFile(t, "frontend/src/js/selection-controller.js"), "async function selectSemanticEntity", "async function hoverSemanticEntity")
	if !strings.Contains(selection, "preserveTemporaryReveal") || !strings.Contains(selection, "temporaryRevealCleared") {
		t.Fatal("selecting the temporarily revealed occurrence must keep it visible while other selections trigger a full restore render")
	}
}

func TestSemanticBasicDirectRevealMaterializesOnlyTargetSection(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/input-views.js")
	for _, forbidden := range []string{"SEMANTIC_BASIC_LINE_BUDGET", "slice(0, 250)", "slice(0,250)"} {
		if strings.Contains(views, forbidden) {
			t.Fatalf("Basic Semantic Text must not discard late entities with %q", forbidden)
		}
	}
	for _, required := range []string{
		"materializedBasicSemanticLines",
		"semanticExpandedSectionIds",
		"semanticSectionId",
		"semanticTemporaryReveal",
		"data-occurrence-id",
		"scrollIntoView",
	} {
		if !strings.Contains(views, required) {
			t.Fatalf("Basic direct-reveal contract missing %q", required)
		}
	}
	reveal := sliceBetween(views, "function ensureSemanticOccurrenceVisible", "function clearSemanticTemporaryReveal")
	if !strings.Contains(reveal, "semanticExpandedSectionIds.add(") {
		t.Fatal("direct reveal must materialize the selected entity's section, including the final entity in a large model")
	}
	if strings.Contains(reveal, "semanticExpandedSectionIds.clear(") {
		t.Fatal("direct reveal must not materialize every Basic section")
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
		t.Fatal("Semantic Text must reveal the remapped occurrence after edit/reanalysis")
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
		t.Fatal("Semantic Text must mutate globalSelection only through the selection controller")
	}

	interaction := sliceBetween(views, "function selectSemanticLine", "function editSemanticValue")
	analyzerCall := regexp.MustCompile(`(?i)\b(?:analyze|analyzeinput|analyzeinputstagetext|scheduleanalyze)\s*\(`)
	if analyzerCall.MatchString(interaction) || strings.Contains(interaction, "analyzeCallback(") {
		t.Fatal("click, hover, open, chips, chooser, and reveal navigation must not start analysis")
	}
	if strings.Contains(interaction, "recordViewHistory(") {
		t.Fatal("semantic interactions must not duplicate controller-owned navigation history")
	}
}

func TestResultSelectionPreservesTheCurrentInputViewAndOffersSemanticReveal(t *testing.T) {
	controller := readTestFile(t, "frontend/src/js/selection-controller.js")
	main := readTestFile(t, "frontend/src/js/main.js")
	index := readTestFile(t, "frontend/src/index.html")
	styles := readTestFile(t, "frontend/src/styles/base.css")
	for _, required := range []string{
		"getActiveInputView",
		"activeInputView !== \"input-semantic\"",
		"invokeAdapterReveal(activeInputAdapter",
		`action: "select"`,
		"revealSemanticWithinTransaction",
	} {
		if !strings.Contains(controller, required) {
			t.Fatalf("linked result selection must highlight the current source view without forcing Semantic, missing %q", required)
		}
	}
	for _, required := range []string{
		"semanticRevealIndicator",
		"idfAnalyzer:semanticRevealAvailable",
		"revealSelectionInSemantic(",
		"updateSemanticRevealIndicator",
	} {
		if !strings.Contains(main+index, required) {
			t.Fatalf("non-Semantic input views need an explicit reveal indicator, missing %q", required)
		}
	}
	if !strings.Contains(styles, ".semantic-reveal-indicator") {
		t.Fatal("Semantic reveal indicator needs visible keyboard-accessible styling")
	}
	inputViews := readTestFile(t, "frontend/src/js/views/input-views.js")
	switchView := sliceBetween(inputViews, "export async function switchInputView", "export function setTableOrientation")
	for _, required := range []string{
		"currentSemanticSelection()",
		"ensureSemanticOccurrenceVisible(selection",
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
		`state.activeInputView === "semantic"`,
		"getPanelNavigationAdapter(viewId)",
		"adapter?.selectFromElement?.(element)",
		"selectSemanticEntity(selection",
		"recordHistory: true",
		"follow: false",
	} {
		if !strings.Contains(activation, required) {
			t.Fatalf("source input views must commit through the global semantic controller, missing %q", required)
		}
	}
	styles := sliceBetween(navigation, "export function refreshInputSelectionStyles", "let legacyInputAdaptersInitialized")
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
