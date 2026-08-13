package frontendchecks

import (
	"strings"
	"testing"
)

func TestHVACPanelSemanticMarkupUsesProjectionReverseIndex(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	for _, required := range []string{
		`configureResultPanelNavigationHooks("hvac"`,
		"state.semanticProjection?.navigation",
		"navigation.byViewTarget?.[`hvac|${target}`]",
		"hvacSemanticRecordForTarget",
		"hvacServicePathSemanticAttributes",
		"hvacLoopSemanticAttributes",
		"hvacComponentSemanticAttributes",
		"hvacCouplingSemanticAttributes",
		"hvacZoneSemanticAttributes",
		`semanticHVACDataAttribute("data-entity-id"`,
		`semanticHVACDataAttribute("data-entity-kind"`,
		`semanticHVACDataAttribute("data-occurrence-context"`,
		`semanticHVACDataAttribute("data-source-object-id"`,
		`semanticHVACDataAttribute("data-source-object-index"`,
		`semanticHVACDataAttribute("data-source-field-index"`,
		`semanticHVACDataAttribute("data-panel-target-id"`,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("HVAC semantic panel markup is missing %q", required)
		}
	}

	resolver := sliceBetween(content, "function hvacSemanticRecordForTarget", "function semanticHVACDataAttribute")
	for _, guard := range []string{
		"expectedEntityKinds",
		"expectedContextKinds",
		"viewTarget.targetId === target",
		"viewTarget.targetKind === options.targetKind",
		"occurrence.sourceAnchor",
	} {
		if !strings.Contains(resolver, guard) {
			t.Fatalf("HVAC reverse-target resolution is missing %q", guard)
		}
	}

	for _, rendererContract := range []string{
		"serviceNodeSemanticAttributes(node)",
		"hvacServicePathSemanticAttributes(path)",
		"hvacLoopSemanticAttributes(loop)",
		"hvacComponentSemanticAttributes(component",
		"hvacCouplingSemanticAttributes(coupling",
		"hvacZoneSemanticAttributesForName(item.zone)",
	} {
		if !strings.Contains(content, rendererContract) {
			t.Fatalf("HVAC selectable renderer is missing %q", rendererContract)
		}
	}
}

func TestHVACPanelAdapterPreservesContextAndCompatibleOccurrence(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	for _, hook := range []string{
		"hvacCanRevealSelection",
		"revealHVACSelection",
		"findHVACNavigationTarget",
		"captureHVACNavigationContext",
		"restoreHVACNavigationContext",
		"preferredHVACSemanticOccurrence",
	} {
		if !strings.Contains(content, hook) {
			t.Fatalf("HVAC panel adapter is missing %q", hook)
		}
	}

	reveal := sliceBetween(content, "async function revealHVACSelection", "function findHVACNavigationTarget")
	if !strings.Contains(reveal, `navigateHVAC(navigationTarget, { pushHistory: false, replace: true })`) {
		t.Fatal("semantic HVAC reveal must reuse navigateHVAC without adding local history")
	}

	capture := sliceBetween(content, "function captureHVACNavigationContext", "async function restoreHVACNavigationContext")
	for _, field := range []string{
		"hvacNavigationSnapshot()",
		"serviceKindFilter",
		"pathTypeFilter",
		"mediumFilter",
		"graphScrollTop",
		"graphScrollLeft",
		"navigationRevealTarget",
	} {
		if !strings.Contains(capture, field) {
			t.Fatalf("HVAC history context is missing %q", field)
		}
	}
	if strings.Contains(capture, "graphScale") {
		t.Fatal("HVAC history context retains the removed Fit/100%/Compact preset")
	}
	if !strings.Contains(content, "hvacNavigationRevealMatchesPath") {
		t.Fatal("HVAC service-path reveal must remain compatible with graph-specific quick filters")
	}

	preference := sliceBetween(content, "function preferredHVACSemanticOccurrence", "function hvacViewTargetForSelection")
	for _, signal := range []string{
		`occurrence.contextKind === "zone_service"`,
		`["loop_occurrence", "component_occurrence"]`,
		`/^hvac\/service-paths\//i`,
		"state.activeHVACContext?.pathId",
		"state.activeHVACLoopId",
	} {
		if !strings.Contains(preference, signal) {
			t.Fatalf("HVAC occurrence preference is missing %q", signal)
		}
	}
}

func TestHVACRemovedCouplingViewRedirectsSemanticTargets(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	viewMode := sliceBetween(content, "function hvacViewMode", "function graphKeyForHVACEntity")
	if strings.Contains(viewMode, "couplings") {
		t.Fatal("legacy Couplings view must not remain a canonical HVAC route")
	}
	for _, required := range []string{`["services", "loop"].includes(view)`, `return "services"`} {
		if !strings.Contains(viewMode, required) {
			t.Fatalf("legacy HVAC view canonicalization is missing %q", required)
		}
	}

	restore := sliceBetween(content, "function restoreHVACNavigationSnapshot", "function notifyHVACSelectionChanged")
	if !strings.Contains(restore, `state.activeHVACView = hvacViewMode(snapshot.view || "services")`) {
		t.Fatal("restoring saved HVAC history must canonicalize the removed Couplings view to Zone Services")
	}

	couplingTarget := sliceBetween(content, "function hvacNavigationTargetForSemanticCoupling", "function compatibleHVACPathID")
	for _, required := range []string{
		"compatibleHVACPathID",
		"firstConnectedHVACLoop(coupling)",
		`view: "services"`,
		`view: "loop"`,
		"loopID:",
		"coupling-node:any:",
	} {
		if !strings.Contains(couplingTarget, required) {
			t.Fatalf("semantic coupling redirect is missing %q", required)
		}
	}
	if strings.Contains(couplingTarget, `view: "couplings"`) {
		t.Fatal("semantic coupling reveal still targets the removed Couplings view")
	}

	semanticTarget := sliceBetween(content, "function hvacNavigationTargetForSemanticSelection", "function hvacNavigationTargetForSemanticComponent")
	if strings.Contains(semanticTarget, `view: "couplings"`) {
		t.Fatal("semantic HVAC network/coupling fallback still targets the removed Couplings view")
	}
}

func TestHVACCommittedNavigationDelegatesGlobalHistoryOnce(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	delegation := sliceBetween(content, "function navigateHVACFromPanelElement", "export function navigateHVAC")
	for _, required := range []string{
		`closest?.("[data-entity-id][data-panel-target-id]")`,
		`navigateHVAC(target, { pushHistory: false, replace: true })`,
		"queueMicrotask(navigate)",
	} {
		if !strings.Contains(delegation, required) {
			t.Fatalf("HVAC committed navigation delegation is missing %q", required)
		}
	}
	if strings.Contains(delegation, "recordViewHistory(") {
		t.Fatal("HVAC committed selection must let the global controller own history")
	}
	if strings.Contains(content, `[data-hvac-graph-scope]`) {
		t.Fatal("HVAC graph scope is fixed to Focused and must not expose a history-producing scope selector")
	}
}
