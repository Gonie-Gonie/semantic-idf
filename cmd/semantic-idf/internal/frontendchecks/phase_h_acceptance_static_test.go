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
			{path: "frontend/src/js/topology-loader.js", terms: []string{`configureResultPanelNavigationHooks("topology"`, "restoreTopologyNavigationContext"}},
			{path: "frontend/src/js/views/topology-view.js", terms: []string{"topologySelectionForTarget", "preferredTopologySemanticOccurrence", "export async function restoreTopologyNavigationContext"}},
			{path: "frontend/src/js/navigation.js", terms: []string{"snapshot.globalSelection", "restoreRegisteredPanelContext"}},
		},
		"B profile schedule and source": {
			{path: "frontend/src/js/views/profile-views.js", terms: []string{"function handleProfileOverviewActivation", "data-profile-row-key", "profileSelectionAnchorKey", "function profileSeriesSemanticTargets", "function applyProfileNavigationTarget", "function selectProfileItemForNavigation", "function selectProfileZoneDimensionForNavigation", "captureProfileNavigationContext", "restoreProfileNavigationContext"}},
			{path: "frontend/src/js/selection-controller.js", terms: []string{"chooseSemanticOccurrence", "semanticOccurrenceChoices"}},
			{path: "frontend/src/js/views/input-views.js", terms: []string{"revealSelectionSource", "sourceAnchor"}},
		},
		"C HVAC service and loop": {
			{path: "frontend/src/js/views/hvac-views.js", terms: []string{"async function revealHVACSelection", "navigateHVAC(navigationTarget, { pushHistory: false", "captureHVACNavigationContext", "restoreHVACNavigationContext", `"loop_occurrence"`}},
			{path: "frontend/src/js/navigation.js", terms: []string{"popUndoSnapshot", "restoreRegisteredPanelContext"}},
		},
		"D simulation output source to input": {
			{path: "frontend/src/js/views/simulation-views.js", terms: []string{"simulationEnergySemanticAttributes", "simulationHVACPathSemanticCandidate", "simulationOutputSourceSemanticCandidate", `view: "input-text"`, `targetKind: "source"`, "requestSimulationModelSelection"}},
			{path: "frontend/src/js/views/input-views.js", terms: []string{"revealSelectionInSemantic", "revealSelectionSource"}},
		},
		"E diagnose edit remap": {
			{path: "frontend/src/js/tools.js", terms: []string{"restoreDiagnoseDocument", "AnalyzeInputDiagnosticsText", "PreviewCleanupText", "persistDiagnoseDocument"}},
			{path: "frontend/src/tools.html", terms: []string{`data-tools-panel="diagnose"`, `id="diagnoseApply"`, `id="diagnoseSaveAs"`}},
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
			"profileNavigationRevealDimension",
		},
		"frontend/src/js/views/hvac-views.js": {
			"hvacNavigationRevealTarget",
			"serviceKindFilter",
			"pathTypeFilter",
		},
		"frontend/src/js/views/simulation-views.js": {
			"simulationNavigationRevealTarget",
			"captureSimulationNavigationContext",
		},
		"frontend/src/js/views/topology-view.js": {
			"temporaryTopologyReveal",
			"restoreTopologyNavigationContext",
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
