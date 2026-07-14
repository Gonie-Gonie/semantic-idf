package frontendchecks

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSemanticSelectionStateContract(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/state.js")
	selection := sliceBetween(content, "globalSelection: {", "globalHover: {")
	for _, field := range []string{
		"entityId",
		"entityKind",
		"occurrenceId",
		"sourceAnchor",
		"originView",
		"originTargetId",
		"semanticPathHint",
		"relatedEntityIds",
		"transactionId",
	} {
		if !regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(field) + `\s*:`).MatchString(selection) {
			t.Fatalf("globalSelection is missing %s", field)
		}
	}

	hover := sliceBetween(content, "globalHover: {", "semanticLinkMode:")
	for _, field := range []string{"entityId", "occurrenceId", "originView"} {
		if !regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(field) + `\s*:`).MatchString(hover) {
			t.Fatalf("globalHover is missing %s", field)
		}
	}
	for _, required := range []string{
		"semanticLinkMode: true",
		"semanticFollowSelection: true",
		"semanticTemporaryReveal: null",
		"semanticSelectedObjectIndex:",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("selection state contract missing %q", required)
		}
	}
}

func TestSelectionControllerPublicAPIAndBuildContract(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/selection-controller.js")
	for _, name := range []string{
		"selectSemanticEntity",
		"hoverSemanticEntity",
		"clearSemanticHover",
		"clearSemanticSelection",
		"revealSelectionInSemantic",
		"openSelectionInView",
		"revealSelectionSource",
		"currentSemanticSelection",
		"selectionTargetsForView",
	} {
		assertJSExport(t, content, name)
	}
	for _, name := range []string{
		"initializeSelectionControllerState",
		"createSelectionController",
		"configureSelectionController",
		"pendingSemanticNavigation",
		"resumePendingSemanticNavigation",
	} {
		assertJSExport(t, content, name)
	}

	build := readTestFile(t, "scripts/frontend-build.ps1")
	for _, module := range []string{"panel-navigation-registry.js", "selection-controller.js"} {
		if !strings.Contains(build, `"`+module+`"`) {
			t.Fatalf("frontend readiness manifest is missing %s", module)
		}
	}
}

func TestPanelNavigationRegistryContract(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/panel-navigation-registry.js")
	for _, name := range []string{"registerPanelNavigationAdapter", "getPanelNavigationAdapter"} {
		assertJSExport(t, content, name)
	}
	for _, viewID := range []string{
		"summary",
		"profile",
		"hvac",
		"output",
		"simulation",
		"diagnose",
		"geometry",
		"input-semantic",
		"input-text",
		"input-json",
		"input-table",
	} {
		if !strings.Contains(content, `"`+viewID+`"`) {
			t.Fatalf("panel navigation registry is missing view %q", viewID)
		}
	}
	for _, method := range []string{
		"canReveal",
		"reveal",
		"selectFromElement",
		"captureContext",
		"restoreContext",
		"preferredSemanticOccurrence",
	} {
		if !strings.Contains(content, `"`+method+`"`) {
			t.Fatalf("panel navigation adapter contract is missing %s()", method)
		}
	}
	for _, forbidden := range []string{"querySelector", "closest(", "classList", ".dataset"} {
		if strings.Contains(readTestFile(t, "frontend/src/js/selection-controller.js"), forbidden) {
			t.Fatalf("selection controller must not depend on panel DOM details: found %q", forbidden)
		}
	}
}

func TestSelectionControllerTransactionsAndFollowContract(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/selection-controller.js")
	transaction := sliceBetween(content, "async function inTransaction", "function rememberTransaction")
	for _, required := range []string{
		"activeTransactions.has(transactionId)",
		"rememberedTransactions.has(transactionId)",
		"activeTransactions.add(transactionId)",
		"rememberTransaction(transactionId)",
		"activeTransactions.delete(transactionId)",
	} {
		if !strings.Contains(transaction, required) {
			t.Fatalf("same-transaction suppression contract missing %q", required)
		}
	}

	options := sliceBetween(content, "function optionsFor", "async function inTransaction")
	for _, name := range []string{
		"originView",
		"action",
		"recordHistory",
		"follow",
		"preserveFilters",
		"transactionId",
	} {
		if !strings.Contains(options, name) {
			t.Fatalf("selection options contract is missing %s", name)
		}
	}

	selection := sliceBetween(content, "async function selectSemanticEntity", "async function hoverSemanticEntity")
	for _, required := range []string{
		"entityChanged",
		"occurrenceChanged",
		"selectionChanged",
		"options.recordHistory && selectionChanged",
		"controllerState.globalSelection = selection",
		"controllerState.semanticLinkMode && options.follow",
	} {
		if !strings.Contains(selection, required) {
			t.Fatalf("selection/follow contract missing %q", required)
		}
	}

	hover := sliceBetween(content, "async function hoverSemanticEntity", "async function clearSemanticHover")
	if !strings.Contains(hover, "recordHistory: false") || !strings.Contains(hover, "follow: false") {
		t.Fatal("hover must explicitly disable history and follow")
	}
	if strings.Contains(hover, "dependencies.recordHistory") {
		t.Fatal("hover must never invoke the history recorder")
	}

	resume := sliceBetween(content, "async function resumePendingSemanticNavigation", "async function followSelection")
	clearIndex := strings.Index(resume, "semanticPendingNavigation = null")
	dispatchIndex := strings.Index(resume, "return inTransaction")
	if clearIndex < 0 || dispatchIndex < 0 || clearIndex > dispatchIndex {
		t.Fatal("pending navigation must be cleared before its one-shot resume")
	}
	if !strings.Contains(resume, "recordHistory: false") {
		t.Fatal("pending navigation restore must not record history")
	}
	if !strings.Contains(resume, "controllerState.semanticPendingNavigation !== pending") {
		t.Fatal("concurrent analysis lifecycle events must not resume the same pending target twice")
	}
	queue := sliceBetween(content, "async function queuePendingNavigation", "return Object.freeze")
	for _, required := range []string{
		"pendingNavigationKey(existing) === pendingNavigationKey(pending)",
		"controllerState.semanticPendingNavigation = pending",
		"queueAnalysisTarget",
		"onAnalysisPending",
	} {
		if !strings.Contains(queue, required) {
			t.Fatalf("stale pending-navigation contract missing %q", required)
		}
	}
	if !strings.Contains(content, "semanticFollowSelection") || !strings.Contains(content, "semanticLinkMode") {
		t.Fatal("controller must keep linked selection and follow selection as separate controls")
	}
}

func TestSelectionControllerOwnsGlobalSelectionMutation(t *testing.T) {
	root := repoPath("frontend/src/js")
	assignment := regexp.MustCompile(`\bstate\s*\.\s*globalSelection(?:\s*\.\s*[A-Za-z_$][A-Za-z0-9_$]*)?\s*=(?:[^=>]|$)`)
	objectMutation := regexp.MustCompile(`\b(?:Object\.assign|Object\.definePropert(?:y|ies))\s*\(\s*state\s*\.\s*globalSelection\b`)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".js" || entry.Name() == "selection-controller.js" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if assignment.Match(data) || objectMutation.Match(data) {
			t.Errorf("%s mutates globalSelection outside selection-controller.js", filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan frontend modules: %v", err)
	}
}

func TestSemanticNavigationDoesNotTriggerAnalysis(t *testing.T) {
	controller := readTestFile(t, "frontend/src/js/selection-controller.js")
	if strings.Contains(controller, `from "./actions.js"`) {
		t.Fatal("selection controller must not import the analysis action module")
	}
	analyzerCall := regexp.MustCompile(`(?i)\b(?:analyze|analyzeinput|analyzeinputstagetext)\s*\(`)
	if analyzerCall.MatchString(controller) {
		t.Fatal("selection controller must queue stale navigation without calling an analyzer")
	}
	for _, required := range []string{
		"analysisIsCurrent",
		"queuePendingNavigation",
		"resumePendingSemanticNavigation",
		"queueAnalysisTarget",
	} {
		if !strings.Contains(controller, required) {
			t.Fatalf("stale navigation contract missing %q", required)
		}
	}

	navigation := readTestFile(t, "frontend/src/js/navigation.js")
	focus := sliceBetween(navigation, "export async function focusInputObject", "export function handleInputJumpActivation")
	if focus == "" {
		t.Fatal("legacy focusInputObject export is missing")
	}
	if analyzerCall.MatchString(focus) || strings.Contains(focus, "scheduleAnalyze") {
		t.Fatal("focusInputObject must reveal from existing source/navigation state without starting analysis")
	}
}

func TestPendingNavigationPersistsStagePriorityWithoutStartingAnalysis(t *testing.T) {
	actions := readTestFile(t, "frontend/src/js/actions.js")
	priority := sliceBetween(actions, "export function prioritizeAnalysisStageForTab", "function orderedAnalysisStages")
	for _, required := range []string{
		"if (!activeStageQueue)",
		"state.pendingAnalysisPriorityTab = tab",
	} {
		if !strings.Contains(priority, required) {
			t.Fatalf("pre-queue stage priority contract missing %q", required)
		}
	}
	ordered := sliceBetween(actions, "function orderedAnalysisStages", "function stageMatchesActiveTab")
	if !strings.Contains(ordered, "state.pendingAnalysisPriorityTab") || !strings.Contains(ordered, `state.pendingAnalysisPriorityTab = ""`) {
		t.Fatal("the next analysis queue must consume the persisted navigation priority exactly once")
	}
}

func TestLegacyFocusInputObjectWrapsSelectionController(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/navigation.js")
	focus := sliceBetween(content, "export async function focusInputObject", "export function handleInputJumpActivation")
	for _, controllerCall := range []string{"selectSemanticEntity(", "revealSelectionSource("} {
		if !strings.Contains(focus, controllerCall) {
			t.Fatalf("focusInputObject legacy wrapper must delegate through %s", controllerCall)
		}
	}
	if !strings.Contains(focus, "recordHistory: false") {
		t.Fatal("focusInputObject must suppress nested history when it performs the source reveal")
	}
}

func assertJSExport(t *testing.T, content, name string) {
	t.Helper()
	pattern := regexp.MustCompile(`(?m)^export\s+(?:(?:async\s+)?function|const)\s+` + regexp.QuoteMeta(name) + `\b`)
	if !pattern.MatchString(content) {
		t.Fatalf("JavaScript module is missing public export %s", name)
	}
}
