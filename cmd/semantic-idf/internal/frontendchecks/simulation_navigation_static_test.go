package frontendchecks

import (
	"strings"
	"testing"
)

func TestSimulationResultsMapToCanonicalModelWithoutSimulationEntities(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	for _, cacheTerm := range []string{"getSemanticNavigationCache", "simulationSemanticNavigationCache", `cache.occurrenceIDs("view-target"`, "cache.occurrence(", "cache.entity("} {
		if !strings.Contains(content, cacheTerm) {
			t.Fatalf("simulation canonical mapping must reuse the semantic navigation cache: missing %q", cacheTerm)
		}
	}
	for _, boundedTerm := range []string{"simulationEnergySourceByIDCache", "simulationEnergySourceByID(explanation)", "simulationSemanticBindings.clear()", "pruneSimulationSemanticBindings", "simulationSemanticBindings.delete(bindingID)"} {
		if !strings.Contains(content, boundedTerm) {
			t.Fatalf("simulation render caches must remain identity-cached and bounded: missing %q", boundedTerm)
		}
	}
	mapping := sliceBetween(content, "function simulationEnergySemanticAttributes", "function simulationSourceSemanticAttributes")
	for _, term := range []string{
		"item.relatedPathIds",
		"simulationHVACPathSemanticCandidate",
		"simulationZoneSemanticCandidate",
		"simulationOutputSourceSemanticCandidate",
		"simulationEnergyGroupSemanticCandidates",
		"[pathCandidates, zoneCandidates, sourceCandidates, modelCandidates]",
	} {
		if !strings.Contains(mapping, term) {
			t.Fatalf("simulation model mapping priority is missing %q", term)
		}
	}
	if strings.Contains(mapping, "relatedPathIds[0]") || strings.Contains(mapping, "relatedPathIds?.[0]") {
		t.Fatal("simulation aggregate mapping must not silently choose the first related service path")
	}
	for _, canonicalKind := range []string{`entityKinds: ["hvac-path"]`, `entityKinds: ["zone"]`, `entityKinds: ["output"]`, `entityKinds: ["hvac-loop"]`} {
		if !strings.Contains(content, canonicalKind) {
			t.Fatalf("simulation must resolve existing canonical entities via %q", canonicalKind)
		}
	}
	if strings.Contains(content, `entityKind: "simulation"`) || strings.Contains(content, `entityKinds: ["simulation"]`) {
		t.Fatal("simulation run values must not become canonical semantic entities")
	}
}

func TestSimulationInteractiveResultsKeepSelectionLocal(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	attributes := sliceBetween(content, "function simulationSemanticNavigationAttributes", "function simulationSemanticBinding")
	if !strings.Contains(attributes, `return ""`) {
		t.Fatal("simulation results must not carry semantic navigation bindings")
	}
	for _, forbidden := range []string{"data-simulation-model-target-chooser", "data-simulation-semantic-select", "requestSimulationModelSelection", "openSimulationHVACTab", "data-simulation-hvac-loop-name", "data-simulation-hvac-path-id", "data-simulation-hvac-coupling-id"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("simulation results still expose model navigation through %q", forbidden)
		}
	}
	for _, local := range []string{"handleHVACInspectionEvent", "selectSimulationEnergyGraphItem", "state.simulationHeatFlowSelectedZone = shape.dataset.heatZone"} {
		if !strings.Contains(content, local) {
			t.Fatalf("simulation local interaction is missing %q", local)
		}
	}
}

func TestSimulationPanelAdapterRevealsWithoutAnalysisAndRestoresFilters(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	adapter := sliceBetween(content, "function configureSimulationPanelNavigation", "function simulationNavigationDestination")
	for _, term := range []string{
		`configureResultPanelNavigationHooks("simulation"`,
		"canReveal(selection",
		"async reveal(selection",
		"selectFromElement(element",
		"findTarget(selection",
		"captureContext(context)",
		"async restoreContext(snapshot",
		"preferredSemanticOccurrence(selection",
	} {
		if !strings.Contains(adapter, term) {
			t.Fatalf("simulation panel adapter is missing %q", term)
		}
	}
	for _, stateTerm := range []string{
		"simulationEnergyScopeKind",
		"simulationEnergyZoneName",
		"simulationEnergyPeriod",
		"simulationEnergyService",
		"simulationEnergySelection",
		"simulationEnergyDetailsOpen",
		"energyDrawer",
		"captureSimulationEnergyWorkspaceContext",
		"restoreSimulationEnergyWorkspaceContext",
		"simulationHVACInspection",
		"hvacInspectionLoopKey",
		"hvacInspection: structuredClone(state.simulationHVACInspection || {})",
		"simulationHVACInspection: snapshot.hvacInspection ? structuredClone(snapshot.hvacInspection) : undefined",
		"simulationComfortZone",
		"simulationHeatFlowSelectedZone",
		"simulationHeatFlowStory",
		"simulationSeriesGroup",
		"simulationSelectedSeries",
		"captureSimulationNavigationContext",
		"restoreSimulationNavigationContext",
	} {
		if !strings.Contains(content, stateTerm) {
			t.Fatalf("simulation navigation context is missing %q", stateTerm)
		}
	}
	reveal := sliceBetween(content, "function applySimulationNavigationDestination", "function findSimulationNavigationTarget")
	for _, forbidden := range []string{"callSimulationAPI", "runCurrentSimulation", "scheduleSimulationRunPlan", "analyze("} {
		if strings.Contains(reveal, forbidden) {
			t.Fatalf("simulation navigation reveal must not trigger analysis or a run: %q", forbidden)
		}
	}
	for _, forbidden := range []string{"state.simulationEnergyFocusMode =", "state.simulationEnergyServicePathFocus =", "state.simulationSeriesGroup =", "state.simulationHeatFlowStory ="} {
		if strings.Contains(reveal, forbidden) {
			t.Fatalf("simulation reveal must preserve the active result filter: %q", forbidden)
		}
	}
	for _, materializationTerm := range []string{
		"materializeSimulationEnergyNavigationTarget",
		"revealNodeIDs",
		"revealStoryIDs",
	} {
		if !strings.Contains(content, materializationTerm) {
			t.Fatalf("simulation reveal must temporarily materialize a filtered target: missing %q", materializationTerm)
		}
	}
}
