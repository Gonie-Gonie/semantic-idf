package frontendchecks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrontendHVACDefaultUICopyAvoidsDebugAndLegacyTerms(t *testing.T) {
	files := []string{
		repoPath("frontend/src/js/views/hvac-views.js"),
		repoPath("frontend/src/js/i18n.js"),
		repoPath("frontend/src/js/state.js"),
	}
	forbidden := []string{
		"Rule edges",
		"Rule trace",
		"Rule path",
		"Terminal / Equipment",
		"Plant / Condenser",
		"terminal:direct",
		"terminalComponents",
		"buildRelationGraph",
		"plant-terminal",
		"source-zone",
		`data-hvac-open-view="relation"`,
		"relation-link:",
		"Zone relations",
		"Other loops",
		"hvac.inferred",
		"Inferred",
		"Cross-loop",
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(content)
		for _, term := range forbidden {
			if strings.Contains(text, term) {
				t.Fatalf("%s contains forbidden HVAC default UI copy %q", file, term)
			}
		}
	}
}

func TestFrontendHVACStartsOnZoneServices(t *testing.T) {
	content, err := os.ReadFile(repoPath("frontend/src/js/state.js"))
	if err != nil {
		t.Fatalf("read state.js: %v", err)
	}
	if !strings.Contains(string(content), `activeHVACView: "services"`) {
		t.Fatalf("state.js should default HVAC to Zone Services view")
	}
	for _, required := range []string{
		"activeHVACEntity",
		"activeHVACContext",
		"hvacNavigationStack",
		"hvacForwardStack",
		`activeHVACGraphScope: "focused"`,
		`hvacServiceKindFilter: "all"`,
		`hvacPathTypeFilter: "all"`,
		`hvacMediumFilter: "all"`,
	} {
		if !strings.Contains(string(content), required) {
			t.Fatalf("state.js should include HVAC navigation state %q", required)
		}
	}
	if !strings.Contains(string(content), `hvacGraphScale: "actual"`) {
		t.Fatalf("state.js should default HVAC graph to actual scale")
	}
}

func TestFrontendHVACServiceDOMContracts(t *testing.T) {
	content, err := os.ReadFile(repoPath("frontend/src/js/views/hvac-views.js"))
	if err != nil {
		t.Fatalf("read hvac views: %v", err)
	}
	text := string(content)
	for _, required := range []string{
		"function buildServiceGraph(paths, couplings)",
		"function serviceGraphNodeIdentity",
		"function layoutServiceGraphNodes",
		"function alignServiceGraphColumnRows",
		"function clearHVACGraphSelection",
		"function isPhysicalServiceCoupling",
		"function serviceLinkPath",
		"function bundleServiceGraphLinks",
		"function navigateHVAC(target = {}, options = {})",
		"function backHVAC()",
		"function forwardHVAC()",
		"function clearHVACFocus()",
		"function pathsForActiveHVACEntity",
		"function renderHVACGraphScopeControls",
		"function renderHVACBreadcrumbBar",
		"function renderHVACServicePicker",
		"function renderHVACLoopPicker",
		"function renderHVACComponentPicker",
		"function renderHVACCouplingPicker",
		"function renderHVACQuickFilters",
		"function servicePathMatchesQuickFilters",
		"function orthogonalPath",
		"function grasshopperWirePath",
		"function serviceLinkCurve",
		"function serviceLinkLaneOffset",
		"function openHVACResultTab",
		"function prepareHVACCrossTabContext",
		"function exportHVACDebugGraph",
		"function buildHVACDebugGraphExportPayload",
		"HVAC_GRAPH_EXPORT_SCHEMA",
		`data-hvac-debug-export="rule"`,
		`data-result-tab="simulation"`,
		`event.key === "Escape"`,
		`state.activeHVACGraphKey = ""`,
		`state.activeHVACNodeName = ""`,
		"hvac-service-svg",
		"hvac-edge-bundle-badge",
		"hvac-trace-drawer",
		"evaporative_cooler",
		`data-hvac-service-subject-key="${escapeHTML(key)}"`,
		`data-hvac-loop-id="${escapeHTML(loop.id)}"`,
		`data-hvac-entity-view="services"`,
		`data-hvac-entity-view="couplings"`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("hvac service renderer is missing DOM contract %q", required)
		}
	}
}

func TestFrontendHVACUsesHeaderlessCardNavigationWithoutWarnings(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	state := readTestFile(t, "frontend/src/js/state.js")
	view := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	analysis := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	styles := readTestFile(t, "frontend/src/styles/hvac.css")
	i18n := readTestFile(t, "frontend/src/js/i18n.js")

	for sourceName, source := range map[string]string{
		"index.html": index,
		"state.js":   state,
		"i18n.js":    i18n,
	} {
		for _, removed := range []string{"hvacStats", "hvacFilter", "hvac.filter"} {
			if strings.Contains(source, removed) {
				t.Fatalf("%s retains removed HVAC header/search contract %q", sourceName, removed)
			}
		}
	}
	for _, removed := range []string{
		`class="summary-head hvac-head"`,
		`class="summary-tools hvac-tools"`,
	} {
		if strings.Contains(index, removed) {
			t.Fatalf("HVAC pane retains removed top header markup %q", removed)
		}
	}
	for _, removed := range []string{
		"elements.hvacStats",
		"elements.hvacFilter",
		"function hvacQuery()",
		"function renderHVACEntitySearchResults",
		"function groupNavigationEntitiesForSearch",
		"function zoneServiceMatchesQuery",
		"function servicePathMatchesQuery",
		"function couplingMatchesQuery",
		"function loopMatchesQuery",
		"function warningMatchesQuery",
		"function renderHVACViewTab",
		"function renderHVACWarnings",
		"function renderHVACDiagnostics",
		"function renderHVACWarning",
		"function renderHVACCurrentFocusCard",
		`data-hvac-open-view="diagnostics"`,
		"hvac.warningCount",
		"<span>Issues</span>",
		"path.issues",
		"branch.warnings",
		"hvac.currentFocus",
	} {
		if strings.Contains(view, removed) {
			t.Fatalf("HVAC renderer retains removed header, warning, or focus code %q", removed)
		}
	}
	for _, removed := range []string{
		".hvac-tools input",
		".hvac-view-tabs",
		".hvac-view-tab",
		".hvac-warning",
		".hvac-warning-list",
		".hvac-diagnostic-list",
		".hvac-inline-warning",
		".hvac-current-focus",
		".hvac-nav-static",
		".hvac-nav-action",
		".hvac-nav-card summary em",
	} {
		if strings.Contains(styles, removed) {
			t.Fatalf("HVAC stylesheet retains removed top navigation, warning, or focus selector %q", removed)
		}
	}
	for _, removed := range []string{
		`id="hvacWarningStats"`,
		`id="hvacWarnings"`,
		`data-i18n="hvac.warnings"`,
	} {
		if strings.Contains(index, removed) {
			t.Fatalf("HVAC pane retains removed warning DOM %q", removed)
		}
	}
	for sourceName, source := range map[string]string{
		"state.js":          state,
		"analysis-views.js": analysis,
	} {
		for _, removed := range []string{"hvacStats", "hvacWarningStats", "hvacWarnings"} {
			if strings.Contains(source, removed) {
				t.Fatalf("%s retains removed HVAC header/warning element reference %q", sourceName, removed)
			}
		}
	}
	for _, removed := range []string{
		`"hvac.warnings"`,
		`"hvac.noWarnings"`,
		`"hvac.diagnosticsHelp"`,
		`"hvac.zoneServiceHelp"`,
		`"hvac.airLoopHelp"`,
		`"hvac.plantLoopHelp"`,
		`"hvac.otherLoopHelp"`,
	} {
		if strings.Contains(i18n, removed) {
			t.Fatalf("i18n retains removed HVAC warning or card-help key %q", removed)
		}
	}

	renderer := sliceBetween(view, "export function renderHVAC", "function renderEmptyHVAC")
	if !strings.Contains(renderer, `if (!elements.hvacSummary)`) {
		t.Fatal("HVAC renderer must use the surviving summary container as its render gate")
	}
	emptyRenderer := sliceBetween(analysis, "export function renderEmpty()", "export function renderDeferredGeometry")
	if !strings.Contains(emptyRenderer, `if (elements.hvacSummary)`) {
		t.Fatal("empty analysis renderer must use the surviving HVAC summary container as its render gate")
	}

	for _, required := range []string{
		"${renderHVACServicePicker(",
		"${renderHVACLoopPicker({",
		"${renderHVACComponentPicker(",
		"${renderHVACCouplingPicker(",
		"function renderHVACServicePicker",
		"function renderHVACLoopPicker({ kind, label, count, loops, active })",
		"function renderHVACComponentPicker",
		"function renderHVACCouplingPicker",
		"function renderHVACBreadcrumbBar",
		`data-hvac-nav-action="back"`,
		`data-hvac-nav-action="forward"`,
		`data-hvac-nav-action="clear"`,
		"function renderHVACQuickFilters",
		"function servicePathMatchesQuickFilters",
		"data-hvac-filter-kind",
		"state.hvacServiceKindFilter",
		"state.hvacPathTypeFilter",
		"state.hvacMediumFilter",
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("HVAC lower-card navigation contract is missing %q", required)
		}
	}

	pickers := sliceBetween(view, "function renderHVACSummary", "function handleHVACNavigationClick")
	for _, removed := range []string{"<em>", "help: t(\"hvac.", "kind, label, help"} {
		if strings.Contains(pickers, removed) {
			t.Fatalf("HVAC picker card retains removed secondary help contract %q", removed)
		}
	}
}

func TestFrontendHVACNavigationCardsAreCompactAndViewsAreCanonicalized(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	styles := readTestFile(t, "frontend/src/styles/hvac.css")

	summaryStyle := sliceBetween(styles, ".hvac-nav-card summary {", ".hvac-nav-card summary::-webkit-details-marker")
	for _, required := range []string{
		"min-height: 38px",
		"padding: 5px 8px",
		"gap: 8px",
	} {
		if !strings.Contains(summaryStyle, required) {
			t.Fatalf("compact HVAC card summary style is missing %q", required)
		}
	}
	countStyle := sliceBetween(styles, ".hvac-nav-card summary b {", ".hvac-nav-card.active summary b")
	for _, required := range []string{
		"min-width: 28px",
		"min-height: 24px",
		"font-size: 12px",
	} {
		if !strings.Contains(countStyle, required) {
			t.Fatalf("compact HVAC card count style is missing %q", required)
		}
	}

	viewMode := sliceBetween(view, "function hvacViewMode", "function graphKeyForHVACEntity")
	for _, required := range []string{
		`["services", "loop", "couplings"].includes(view)`,
		`view === "debug" && hvacDebugEnabled()`,
		`return "services"`,
	} {
		if !strings.Contains(viewMode, required) {
			t.Fatalf("HVAC view canonicalizer is missing %q", required)
		}
	}
	for _, required := range []string{
		`const nextView = hvacViewMode(target.view || viewForHVACEntity(entity) || state.activeHVACView || "services")`,
		`state.activeHVACView = hvacViewMode(snapshot.view || "services")`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("HVAC navigation must canonicalize restored or requested views: missing %q", required)
		}
	}
	renderer := sliceBetween(view, "export function renderHVAC", "function renderEmptyHVAC")
	canonicalizeAt := strings.Index(renderer, "state.activeHVACView = hvacViewMode(state.activeHVACView)")
	summaryAt := strings.Index(renderer, "renderHVACSummary(hvac, selectedLoop)")
	if canonicalizeAt < 0 || summaryAt < 0 || canonicalizeAt > summaryAt {
		t.Fatal("HVAC render must canonicalize deprecated views before rendering lower-card navigation")
	}
}

func TestFrontendHVACZoneServicesIsGraphOnlyAndPhysicalObjectBased(t *testing.T) {
	content, err := os.ReadFile(repoPath("frontend/src/js/views/hvac-views.js"))
	if err != nil {
		t.Fatalf("read hvac views: %v", err)
	}
	text := string(content)
	for _, forbidden := range []string{
		"renderHVACServiceTable",
		"renderHVACServiceRow",
		"hvac-service-table-row",
		"`service-node:${path.id}:",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("HVAC Zone Services graph should not use table/path-scoped physical nodes: found %q", forbidden)
		}
	}
	for _, required := range []string{
		"serviceGraphNodeKey(path, spec)",
		"coupling.placementHint === \"detail_only\"",
		"type === \"operation_scheme\"",
		"physicalSupportingCouplings(path, couplingById)",
		"servicePathLinkKeys(path, couplingById)",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("HVAC Zone Services physical graph contract missing %q", required)
		}
	}
}

func TestFrontendHVACServiceStylesCoverRoutingAndBundling(t *testing.T) {
	content, err := os.ReadFile(repoPath("frontend/src/styles/hvac.css"))
	if err != nil {
		t.Fatalf("read HVAC styles: %v", err)
	}
	text := string(content)
	for _, required := range []string{
		".hvac-graph-link.service.bundled",
		".hvac-edge-bundle-badge",
		".hvac-service-link-group:hover .hvac-edge-bundle-badge",
		".hvac-service-link-group:hover .hvac-edge-label",
		".hvac-graph-link.medium-chilled-water",
		".hvac-graph-link.medium-hot-water",
		".hvac-graph-link.medium-refrigerant",
		".hvac-graph-link.medium-electricity",
		".hvac-graph-link.medium-control",
		"stroke-linecap: round",
		"vector-effect: non-scaling-stroke",
		".hvac-graphic-shell.scale-actual .hvac-service-svg",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("hvac service styles are missing %q", required)
		}
	}
}

func TestFrontendHVACGraphAreaOwnsScroll(t *testing.T) {
	content, err := os.ReadFile(repoPath("frontend/src/styles/hvac.css"))
	if err != nil {
		t.Fatalf("read HVAC styles: %v", err)
	}
	text := string(content)
	for _, required := range []string{
		".hvac-pane {\n  flex: 1;\n  display: flex;\n  flex-direction: column;",
		".hvac-layout {\n  flex: 1 1 auto;\n  min-height: 0;",
		".hvac-main {\n  position: relative;\n  display: flex;\n  flex-direction: column;",
		".hvac-graph {\n  flex: 1 1 auto;\n  min-height: 0;\n  overflow: auto;",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("HVAC graph scroll contract missing %q", required)
		}
	}
}

func TestFrontendHVACInspectorIsAlwaysVisibleAndExpandUsesViewportIcon(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	state := readTestFile(t, "frontend/src/js/state.js")
	view := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	styles := readTestFile(t, "frontend/src/styles/hvac.css")
	hvac := sliceBetween(index, `<section class="hvac-main"`, `<aside id="hvacSide"`)
	for _, required := range []string{
		`id="hvacExpandButton"`,
		`class="viewport-icon viewport-icon-expand"`,
		`class="viewport-icon viewport-icon-collapse"`,
		`class="sr-only"`,
	} {
		if !strings.Contains(hvac, required) {
			t.Fatalf("HVAC Expand must remain an accessible viewport icon: missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"hvacInspectorToggle",
		"hvacInspectorCollapsed",
		"idfAnalyzer.hvacInspectorCollapsed",
		"inspector-collapsed",
		"hvac-inspector-toggle",
		"Show inspector",
		"Hide inspector",
	} {
		if strings.Contains(index+state+view+styles, forbidden) {
			t.Fatalf("HVAC inspector toggle state must be removed: found %q", forbidden)
		}
	}
}

func TestFrontendHVACRendererAvoidsResolverConfidenceVocabulary(t *testing.T) {
	content, err := os.ReadFile(repoPath("frontend/src/js/views/hvac-views.js"))
	if err != nil {
		t.Fatalf("read hvac views: %v", err)
	}
	text := strings.ToLower(string(content))
	for _, forbidden := range []string{"confidence", "inferred", "weak", "unsupported"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("hvac renderer contains resolver confidence vocabulary %q", forbidden)
		}
	}
}

func TestFrontendHVACRendererAvoidsLegacyRelationGraphImplementation(t *testing.T) {
	content, err := os.ReadFile(repoPath("frontend/src/js/views/hvac-views.js"))
	if err != nil {
		t.Fatalf("read hvac views: %v", err)
	}
	text := string(content)
	for _, forbidden := range []string{
		"selected.relations",
		"ruleEdgeCountLabel",
		"ruleEdgeSummary",
		"ruleEdgesForRelation(",
		`t("hvac.terminals"`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("hvac renderer still contains legacy relation implementation %q", forbidden)
		}
	}
}

func repoPath(path string) string {
	return filepath.Join("..", "..", filepath.FromSlash(path))
}
