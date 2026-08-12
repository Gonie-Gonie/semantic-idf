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

func TestSimulationInteractiveResultsUseStandardNavigationMarkup(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	attributes := sliceBetween(content, "function simulationSemanticNavigationAttributes", "function simulationSemanticBinding")
	for _, attribute := range []string{
		"data-entity-id",
		"data-entity-kind",
		"data-occurrence-id",
		"data-occurrence-context",
		"data-semantic-path",
		"data-source-object-id",
		"data-source-object-index",
		"data-source-field-index",
		"data-panel-target-id",
		"aria-selected",
	} {
		if !strings.Contains(attributes, attribute) {
			t.Fatalf("simulation standard navigation markup is missing %q", attribute)
		}
	}
	for _, renderer := range []string{
		`simulationEnergySemanticAttributes(edge, "edge")`,
		`simulationEnergySemanticAttributes(node, "node")`,
		"simulationSourceSemanticAttributes(source, object)",
		"simulationHeatFlowZoneSemanticAttributes(zoneName)",
		"simulationHVACLoopSemanticAttributes(loop)",
		"simulationComfortZoneSemanticAttributes(zoneName, object, metric)",
		"simulationSeriesSemanticAttributes(series, sourceObject, seriesRef)",
	} {
		if !strings.Contains(content, renderer) {
			t.Fatalf("simulation interactive result mapping is missing %q", renderer)
		}
	}
}

func TestSimulationAggregateModelChooserKeepsAllCandidateGroups(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	binding := sliceBetween(content, "function simulationSemanticBinding", "function simulationResolvedSemanticGroup")
	for _, term := range []string{
		"resolvedGroups",
		"preferred.selections.length > 1",
		"resolvedGroups.flatMap",
		"simulationUniqueSelections",
	} {
		if !strings.Contains(binding, term) {
			t.Fatalf("simulation aggregate chooser binding is missing %q", term)
		}
	}
	for _, group := range []string{"Model entities", "HVAC service paths", "Output sources", "Zones"} {
		if !strings.Contains(content, group) {
			t.Fatalf("simulation chooser group is missing %q", group)
		}
	}
	for _, term := range []string{
		"data-simulation-model-target-chooser",
		"data-choose-semantic-occurrence",
		"chooseViewTarget",
		"requestSimulationModelSelection",
		"chooseOccurrence: selection.chooseOccurrence === true",
	} {
		if !strings.Contains(content, term) {
			t.Fatalf("simulation multiple-target chooser is missing %q", term)
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
		"simulationEnergyFocusMode",
		"simulationEnergyServicePathFocus",
		"simulationHVACVisibleGroups",
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
		"seriesID(item) === revealSeriesID",
		"revealStoryIDs",
	} {
		if !strings.Contains(content, materializationTerm) {
			t.Fatalf("simulation reveal must temporarily materialize a filtered target: missing %q", materializationTerm)
		}
	}
}
