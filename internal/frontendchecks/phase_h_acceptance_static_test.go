package frontendchecks

import (
	"strings"
	"testing"
)

type phaseHScenarioProbe struct {
	path  string
	terms []string
}

// TestPhaseHEndToEndScenarioContracts locks the cross-module hand-offs for the
// six LINK-183 journeys. The headless harness covers controller execution; this
// table ensures each concrete panel still supplies the required endpoints.
func TestPhaseHEndToEndScenarioContracts(t *testing.T) {
	scenarios := map[string][]phaseHScenarioProbe{
		"A zone geometry and Back": {
			{path: "frontend/src/js/views/input-views.js", terms: []string{"async function openSemanticLine", "openSelectionInView("}},
			{path: "frontend/src/js/geometry-loader.js", terms: []string{`configureResultPanelNavigationHooks("geometry"`, "restoreGeometryNavigationContext"}},
			{path: "frontend/src/js/views/geometry-view.js", terms: []string{"geometrySelectionForTarget", "preferredGeometrySemanticOccurrence", "export async function restoreGeometryNavigationContext"}},
			{path: "frontend/src/js/navigation.js", terms: []string{"snapshot.globalSelection", "restoreRegisteredPanelContext"}},
		},
		"B profile schedule and source": {
			{path: "frontend/src/js/views/profile-views.js", terms: []string{"function selectProfileMatrixCell", "data-choose-semantic-occurrence", "captureProfileNavigationContext", "restoreProfileNavigationContext"}},
			{path: "frontend/src/js/selection-controller.js", terms: []string{"chooseSemanticOccurrence", "semanticOccurrenceChoices"}},
			{path: "frontend/src/js/views/input-views.js", terms: []string{"revealSelectionSource", "sourceAnchor"}},
		},
		"C HVAC service and loop": {
			{path: "frontend/src/js/views/hvac-views.js", terms: []string{"async function revealHVACSelection", "navigateHVAC(navigationTarget, { pushHistory: false", "captureHVACNavigationContext", "restoreHVACNavigationContext", `"loop_occurrence"`}},
			{path: "frontend/src/js/navigation.js", terms: []string{"popUndoSnapshot", "restoreRegisteredPanelContext"}},
		},
		"D simulation path to output": {
			{path: "frontend/src/js/views/simulation-views.js", terms: []string{"simulationEnergySemanticAttributes", "simulationHVACPathSemanticCandidate", "simulationOutputSourceSemanticCandidate", "requestSimulationModelSelection"}},
			{path: "frontend/src/js/views/output-views.js", terms: []string{`configureResultPanelNavigationHooks("output"`, "outputNavigationAttributes", "outputFocusedSignature"}},
			{path: "frontend/src/js/views/input-views.js", terms: []string{"revealSelectionInSemantic", "revealSelectionSource"}},
		},
		"E diagnose edit remap": {
			{path: "frontend/src/js/views/analysis-views.js", terms: []string{"diagnosticSemanticNavigation", "data-diagnostic-reveal-source", "captureDiagnoseNavigationContext", "restoreDiagnoseNavigationContext"}},
			{path: "frontend/src/js/views/diagnose-fixes.js", terms: []string{"pendingFixNavigationContext", `idfAnalyzer:analysisComplete`, "restoreDiagnoseNavigationContext"}},
			{path: "frontend/src/js/main.js", terms: []string{"semanticEditSelectionRestore", "remapSemanticSelection"}},
		},
		"F Settings cache round trip": {
			{path: "frontend/src/js/actions.js", terms: []string{"await saveWorkspaceSnapshot()", "panelContexts: viewSnapshot.panelContexts", "applyCachedAnalysisResult"}},
			{path: "frontend/src/js/main.js", terms: []string{"api.GetCachedAnalysis(restoredDocument.analysisKey)", "pendingWorkspaceRestore", "restoreSavedWorkspaceContext"}},
			{path: "frontend/src/js/view-history.js", terms: []string{"panelContexts: capturePanelContexts()"}},
			{path: "frontend/src/js/navigation.js", terms: []string{"restoreRegisteredPanelContext"}},
		},
	}

	for scenario, probes := range scenarios {
		t.Run(scenario, func(t *testing.T) {
			for _, probe := range probes {
				content := readTestFile(t, probe.path)
				for _, term := range probe.terms {
					if !strings.Contains(content, term) {
						t.Errorf("%s is missing end-to-end hand-off %q", probe.path, term)
					}
				}
			}
		})
	}
}

func TestPhaseHLargeModelFilterHiddenRevealContracts(t *testing.T) {
	files := map[string][]string{
		"frontend/src/js/views/input-views.js": {
			"semanticLinesWithTemporaryReveal",
			"semanticTemporaryReveal",
		},
		"frontend/src/js/views/profile-views.js": {
			"profileNavigationRevealTarget",
			"profileFilter",
		},
		"frontend/src/js/views/hvac-views.js": {
			"hvacNavigationRevealTarget",
			"serviceKindFilter",
			"pathTypeFilter",
		},
		"frontend/src/js/views/output-views.js": {
			"outputTemporaryRevealSignature",
			"temporaryRevealSignature",
		},
		"frontend/src/js/views/simulation-views.js": {
			"simulationNavigationRevealTarget",
			"captureSimulationNavigationContext",
		},
		"frontend/src/js/views/analysis-views.js": {
			"diagnoseTemporaryRevealID",
			"temporaryRevealID",
		},
		"frontend/src/js/views/geometry-view.js": {
			"temporaryGeometryReveal",
			"restoreGeometryNavigationContext",
		},
	}
	for path, terms := range files {
		content := readTestFile(t, path)
		for _, term := range terms {
			if !strings.Contains(content, term) {
				t.Errorf("%s is missing filter-preserving reveal state %q", path, term)
			}
		}
	}

	input := readTestFile(t, "frontend/src/js/views/input-views.js")
	if strings.Contains(input, "slice(0, 250)") || strings.Contains(input, "SEMANTIC_BASIC_LINE_BUDGET") {
		t.Fatal("large-model Semantic reveal must not reintroduce the legacy 250-line limit")
	}
}
