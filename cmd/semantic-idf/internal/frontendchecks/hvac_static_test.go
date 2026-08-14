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
		`hvacServiceKindFilter: "all"`,
		`hvacPathTypeFilter: "all"`,
		`hvacMediumFilter: "all"`,
	} {
		if !strings.Contains(string(content), required) {
			t.Fatalf("state.js should include HVAC navigation state %q", required)
		}
	}
	if strings.Contains(string(content), `hvacGraphScale:`) {
		t.Fatal("state.js should not retain the removed HVAC Fit/100%/Compact preset state")
	}
	if strings.Contains(string(content), `activeHVACGraphScope:`) {
		t.Fatal("state.js should not retain selectable HVAC graph scope now that View is fixed to Focused")
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
		"function syncHVACViewportActions(available)",
		"function pathsForActiveHVACEntity",
		"function renderHVACServicePicker",
		"function renderHVACLoopPicker",
		"function renderHVACQuickFilters",
		"function servicePathMatchesQuickFilters",
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
		for _, removed := range []string{"hvacStats", "hvacFilter"} {
			if strings.Contains(source, removed) {
				t.Fatalf("%s retains removed HVAC header/search contract %q", sourceName, removed)
			}
		}
	}
	if strings.Contains(i18n, `"hvac.filter":`) {
		t.Fatal("i18n.js retains the removed free-text HVAC header filter key")
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
		"function renderHVACBreadcrumbBar",
		"function renderHVACBreadcrumbSegment",
		"function hvacBreadcrumbSegments",
		"renderHVACBreadcrumbBar(hvac)",
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
		".hvac-breadcrumb-bar",
		".hvac-history-actions",
		".hvac-breadcrumb",
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
	emptyRenderer := sliceBetween(analysis, "export function renderEmpty()", "export function renderDeferredTopology")
	if !strings.Contains(emptyRenderer, `if (elements.hvacSummary)`) {
		t.Fatal("empty analysis renderer must use the surviving HVAC summary container as its render gate")
	}

	for _, required := range []string{
		"${renderHVACServicePicker(",
		"${renderHVACLoopPicker({",
		"function renderHVACServicePicker",
		"function renderHVACLoopPicker({ kind, label, count, loops, active })",
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
	for _, removed := range []string{
		"${renderHVACComponentPicker(",
		"${renderHVACCouplingPicker(",
		"function renderHVACComponentPicker",
		"function renderHVACComponentChoice",
		"function renderHVACCouplingPicker",
		"function renderHVACCouplingChoice",
		`data-hvac-entity-view="couplings"`,
	} {
		if strings.Contains(view, removed) {
			t.Fatalf("HVAC retains removed Components/Couplings navigator contract %q", removed)
		}
	}

	pickers := sliceBetween(view, "function renderHVACSummary", "function handleHVACNavigationClick")
	for _, removed := range []string{"<em>", "help: t(\"hvac.", "kind, label, help"} {
		if strings.Contains(pickers, removed) {
			t.Fatalf("HVAC picker card retains removed secondary help contract %q", removed)
		}
	}
}

func TestFrontendHVACViewportActionsAreIconOnlyAndSynchronized(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	state := readTestFile(t, "frontend/src/js/state.js")
	view := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	styles := readTestFile(t, "frontend/src/styles/hvac.css")
	hvac := sliceBetween(index, `<section class="hvac-main"`, `<aside id="hvacSide"`)
	toolbar := sliceBetween(hvac, `<div id="hvacViewportActions"`, `<div id="hvacGraph"`)

	for _, required := range []string{
		`id="hvacViewportActions"`,
		`class="viewport-action-tools hvac-viewport-actions"`,
		`role="group"`,
		`aria-label="HVAC view actions"`,
	} {
		if !strings.Contains(toolbar, required) {
			t.Fatalf("HVAC viewport toolbar is missing %q", required)
		}
	}
	if count := strings.Count(toolbar, `class="viewport-icon-button"`); count != 6 {
		t.Fatalf("HVAC viewport toolbar must contain navigation, Fit, and Expand as exactly six shared icon buttons, got %d", count)
	}

	buttons := []struct {
		id     string
		action string
		label  string
	}{
		{id: "hvacBackButton", action: "back", label: "Back"},
		{id: "hvacForwardButton", action: "forward", label: "Forward"},
		{id: "hvacClearFocusButton", action: "clear", label: "Clear focus"},
		{id: "hvacZoneServicesButton", action: "services", label: "Zone Services"},
	}
	for _, button := range buttons {
		markup := htmlButtonByID(toolbar, button.id)
		for _, required := range []string{
			`class="viewport-icon-button"`,
			`data-hvac-nav-action="` + button.action + `"`,
			`aria-label="` + button.label + `"`,
			`title="` + button.label + `"`,
			`<svg class="viewport-icon`,
			`aria-hidden="true"`,
			`class="sr-only"`,
		} {
			if !strings.Contains(markup, required) {
				t.Fatalf("HVAC %s icon button is missing %q", button.id, required)
			}
		}
	}
	fit := htmlButtonByID(toolbar, "hvacFitButton")
	for _, required := range []string{
		`class="viewport-icon-button"`,
		`aria-label="Fit"`,
		`title="Fit"`,
		`<svg class="viewport-icon`,
		`aria-hidden="true"`,
		`class="sr-only"`,
	} {
		if !strings.Contains(fit, required) {
			t.Fatalf("HVAC Fit icon button is missing %q", required)
		}
	}
	expand := htmlButtonByID(toolbar, "hvacExpandButton")
	for _, required := range []string{
		`class="viewport-icon-button"`,
		`data-expand-pane="hvac"`,
		`aria-label="Expand"`,
		`title="Expand"`,
		`class="viewport-icon viewport-icon-expand"`,
		`class="viewport-icon viewport-icon-collapse"`,
		`class="sr-only"`,
	} {
		if !strings.Contains(expand, required) {
			t.Fatalf("HVAC Expand icon button is missing %q", required)
		}
	}

	for _, required := range []string{
		`hvacViewportActions: document.querySelector("#hvacViewportActions")`,
		`hvacBackButton: document.querySelector("#hvacBackButton")`,
		`hvacForwardButton: document.querySelector("#hvacForwardButton")`,
		`hvacClearFocusButton: document.querySelector("#hvacClearFocusButton")`,
		`hvacZoneServicesButton: document.querySelector("#hvacZoneServicesButton")`,
		`hvacFitButton: document.querySelector("#hvacFitButton")`,
	} {
		if !strings.Contains(state, required) {
			t.Fatalf("HVAC viewport element map is missing %q", required)
		}
	}
	for _, required := range []string{
		`elements.hvacViewportActions?.addEventListener("click"`,
		`window.addEventListener("idfAnalyzer:documentChanged", resetHVACNavigationState)`,
		`handleHVACNavigationAction(navAction.dataset.hvacNavAction || "")`,
		`function resetHVACNavigationState()`,
		`state.hvacNavigationStack = []`,
		`state.hvacForwardStack = []`,
		`function syncHVACViewportActions(available)`,
		`elements.hvacBackButton.disabled`,
		`elements.hvacForwardButton.disabled`,
		`elements.hvacClearFocusButton.disabled`,
		`elements.hvacZoneServicesButton.disabled`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("HVAC fixed viewport action behavior is missing %q", required)
		}
	}
	if strings.Contains(view, "elements.hvacViewportActions.innerHTML") {
		t.Fatal("HVAC renderer must preserve the fixed viewport toolbar and its Expand listener/focus")
	}
	graphClick := sliceBetween(view, `elements.hvacGraph?.addEventListener("click"`, `elements.hvacGraph?.addEventListener("keydown"`)
	for _, removed := range []string{`[data-hvac-nav-action]`, `[data-hvac-open-view]`} {
		if strings.Contains(graphClick, removed) {
			t.Fatalf("HVAC graph retains dead delegated viewport-action branch %q", removed)
		}
	}
	summaryClick := sliceBetween(view, "function handleHVACNavigationClick", "function handleHVACNavigationToggle")
	if strings.Contains(summaryClick, `[data-hvac-nav-action]`) {
		t.Fatal("HVAC summary retains dead delegated viewport-action branch")
	}
	loopOverview := sliceBetween(view, "function renderHVACLoopServiceOverview", "function renderHVACLoopRelatedServicePaths")
	for _, removed := range []string{`data-hvac-nav-action="clear"`, `data-hvac-open-view="services"`, `hvac.showServicePaths`} {
		if strings.Contains(loopOverview, removed) {
			t.Fatalf("HVAC loop overview duplicates the fixed icon toolbar action %q", removed)
		}
	}
	renderer := sliceBetween(view, "export function renderHVAC", "function renderEmptyHVAC")
	if !strings.Contains(renderer, "resetHVACNavigationState()") || !strings.Contains(renderer, "syncHVACViewportActions(true)") {
		t.Fatal("HVAC renderer must synchronize fixed viewport actions on every render")
	}

	viewportStyle := sliceBetween(styles, ".hvac-viewport-actions {", "}")
	for _, required := range []string{
		"top:",
		"right:",
		"flex-wrap: nowrap",
	} {
		if !strings.Contains(viewportStyle, required) {
			t.Fatalf("HVAC upper-right single-row viewport toolbar style is missing %q", required)
		}
	}
	for _, required := range []string{
		"padding-inline-end:",
		".hvac-graph-detail:first-child > .hvac-section-head:first-child",
		".hvac-graph > .empty:first-child",
		"@container hvac-main (max-width: 480px)",
		"padding-block-start: 52px",
		"@container hvac-main (max-width: 240px)",
		"@container hvac-main (max-width: 180px)",
		"width: 28px",
		"width: 24px",
		"gap: 3px",
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("HVAC viewport toolbar collision/compact layout contract is missing %q", required)
		}
	}
}

func TestFrontendHVACGraphControlsUseRecoverableDropdownsAndFixedFocusedFit(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	styles := readTestFile(t, "frontend/src/styles/hvac.css")
	quickFilters := sliceBetween(view, "function renderHVACQuickFilters", "function servicePathMatchesQuickFilters")
	filterStyles := sliceBetween(styles, ".hvac-filter-field {", ".hvac-loop-meta span")

	for _, required := range []string{
		`<select`,
		`<option`,
		`data-hvac-filter-kind=`,
		`["all", "All"]`,
	} {
		if !strings.Contains(quickFilters, required) {
			t.Fatalf("HVAC quick filters must render accessible dropdowns with a recoverable All option: missing %q", required)
		}
	}
	if count := strings.Count(quickFilters, `kind: "`); count != 3 {
		t.Fatalf("HVAC quick filters must retain exactly the service, path, and medium dropdowns, got %d groups", count)
	}
	for _, removed := range []string{
		`<button`,
		`data-hvac-filter-value`,
		`aria-pressed=`,
	} {
		if strings.Contains(quickFilters, removed) {
			t.Fatalf("HVAC quick filters retain the removed long button-row contract %q", removed)
		}
	}

	for _, required := range []string{
		`elements.hvacGraph?.addEventListener("change"`,
		`event.target.closest("select[data-hvac-filter-kind]")`,
		`setHVACQuickFilter(quickFilter.dataset.hvacFilterKind || "", quickFilter.value || "all")`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("HVAC dropdown change behavior is missing %q", required)
		}
	}

	serviceToolbar := sliceBetween(view, "function renderHVACServiceToolbar", "function renderHVACEmptyServiceGraph")
	emptyServiceGraph := sliceBetween(view, "function renderHVACEmptyServiceGraph", "function renderHVACServiceGraph")
	serviceGraph := sliceBetween(view, "function renderHVACServiceGraph", "function cachedServiceGraph")
	if !strings.Contains(serviceToolbar, "renderHVACQuickFilters()") {
		t.Fatal("HVAC service toolbar must own the three filter dropdowns")
	}
	for name, renderer := range map[string]string{
		"empty result": emptyServiceGraph,
		"graph result": serviceGraph,
	} {
		if !strings.Contains(renderer, "renderHVACServiceToolbar()") {
			t.Fatalf("HVAC %s must retain the filter toolbar so users can reset a no-match selection to All", name)
		}
	}

	for _, removed := range []string{
		"function renderHVACGraphScopeControls",
		`data-hvac-graph-scope`,
		"function renderHVACGraphScaleControls",
		`data-hvac-graph-scale`,
		`graphScaleActual`,
		`graphScaleCompact`,
	} {
		if strings.Contains(view, removed) {
			t.Fatalf("HVAC renderer retains a removed scope or size choice %q", removed)
		}
	}
	for _, removed := range []string{
		".hvac-graph-scope",
		".hvac-graph-scale",
		".hvac-filter-group button",
	} {
		if strings.Contains(styles, removed) {
			t.Fatalf("HVAC stylesheet retains a removed scope, preset, or long filter-row selector %q", removed)
		}
	}
	for _, required := range []string{
		"width: 136px",
		`select[data-hvac-filter-kind="service"]`,
		"width: 128px",
		"@container hvac-main (max-width: 260px)",
	} {
		if !strings.Contains(filterStyles, required) {
			t.Fatalf("HVAC dropdown sizing must stay compact while retaining a narrow-container fallback: missing %q", required)
		}
	}
}

func htmlButtonByID(markup, id string) string {
	marker := `id="` + id + `"`
	idIndex := strings.Index(markup, marker)
	if idIndex < 0 {
		return ""
	}
	start := strings.LastIndex(markup[:idIndex], "<button")
	if start < 0 {
		return ""
	}
	end := strings.Index(markup[idIndex:], "</button>")
	if end < 0 {
		return markup[start:]
	}
	return markup[start : idIndex+end+len("</button>")]
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
		`["services", "loop"].includes(view)`,
		`view === "debug" && hvacDebugEnabled()`,
		`return "services"`,
	} {
		if !strings.Contains(viewMode, required) {
			t.Fatalf("HVAC view canonicalizer is missing %q", required)
		}
	}
	if strings.Contains(viewMode, `"couplings"`) {
		t.Fatal("removed Couplings view must canonicalize to Zone Services")
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

func TestFrontendHVACNavigatorContainsOnlyZoneAndLoopGroups(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	summary := sliceBetween(view, "function renderHVACSummary", "function hasHVACFocus")

	if count := strings.Count(summary, "${renderHVACServicePicker("); count != 1 {
		t.Fatalf("HVAC navigator must render one Zone Services card, got %d", count)
	}
	if count := strings.Count(summary, "${renderHVACLoopPicker({"); count != 3 {
		t.Fatalf("HVAC navigator must render AirLoopHVAC, PlantLoop, and Other cards, got %d loop cards", count)
	}
	for _, kind := range []string{`kind: "air"`, `kind: "plant"`, `kind: "other"`} {
		if !strings.Contains(summary, kind) {
			t.Fatalf("HVAC navigator is missing canonical loop group %q", kind)
		}
	}
	for _, removed := range []string{
		"renderHVACComponentPicker",
		"renderHVACCouplingPicker",
		"serviceModel.components",
		"serviceModel.couplings",
		"groups.other.length ? renderHVACLoopPicker",
	} {
		if strings.Contains(summary, removed) {
			t.Fatalf("HVAC navigator retains removed or conditional group contract %q", removed)
		}
	}
}

func TestFrontendHVACLoopDiagramIsPrimaryVisibleContent(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	styles := readTestFile(t, "frontend/src/styles/hvac.css")
	loopView := sliceBetween(view, "function renderHVACLoopView", "function renderHVACLoopServiceOverview")

	if !strings.Contains(loopView, "${renderHVACLoopDiagram(loop, { interactive: true })}") {
		t.Fatal("HVAC loop view must render the interactive loop diagram")
	}
	for _, hiddenContract := range []string{"<details", "hvac-physical-loop-detail", "physicalLoopDetail"} {
		if strings.Contains(loopView, hiddenContract) {
			t.Fatalf("HVAC loop diagram remains hidden behind %q", hiddenContract)
		}
	}
	if strings.Contains(styles, ".hvac-physical-loop-detail") {
		t.Fatal("HVAC stylesheet retains the removed collapsed loop-detail surface")
	}
}

func TestFrontendHVACHasNoStandaloneComponentsOrCouplingsRoute(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	renderer := sliceBetween(view, "export function renderHVAC", "function renderEmptyHVAC")
	viewMode := sliceBetween(view, "function hvacViewMode", "function graphKeyForHVACEntity")

	for _, removed := range []string{
		`state.activeHVACView === "couplings"`,
		"renderHVACCouplings(hvac)",
	} {
		if strings.Contains(renderer, removed) {
			t.Fatalf("HVAC render dispatcher retains standalone route %q", removed)
		}
	}
	if strings.Contains(viewMode, "couplings") {
		t.Fatal("HVAC view canonicalizer retains standalone Couplings route")
	}
	for _, removed := range []string{
		"function renderHVACCouplings",
		"function renderHVACCouplingCard",
		"function renderHVACNetworkCard",
	} {
		if strings.Contains(view, removed) {
			t.Fatalf("HVAC renderer retains dead standalone Components/Couplings code %q", removed)
		}
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

func TestFrontendHVACDiagramCtrlWheelZoomIsLocalBoundedAndResettable(t *testing.T) {
	stateSource := readTestFile(t, "frontend/src/js/state.js")
	view := readTestFile(t, "frontend/src/js/views/hvac-views.js")

	for _, required := range []string{
		`hvacDiagramViewportKey: ""`,
		`hvacDiagramScale: 1`,
		`hvacDiagramPanX: 0`,
		`hvacDiagramPanY: 0`,
	} {
		if !strings.Contains(stateSource, required) {
			t.Fatalf("HVAC diagram viewport state is missing %q", required)
		}
	}

	for _, required := range []string{
		"handleHVACDiagramWheel",
		`elements.hvacGraph?.addEventListener(`,
		`"wheel"`,
		`{ passive: false }`,
		`event.ctrlKey`,
		`event.metaKey`,
		`[data-hvac-diagram-key]`,
		`event.preventDefault()`,
		`event.stopPropagation()`,
		"function resetHVACDiagramViewport",
		"function updateHVACDiagramTransform",
		`class="hvac-diagram-panzoom"`,
		`data-hvac-diagram-content`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("HVAC local Ctrl+wheel viewport contract is missing %q", required)
		}
	}
	if !strings.Contains(view, "0.1") || !strings.Contains(view, "8") {
		t.Fatal("HVAC diagram zoom must retain the 0.1..8 clamp")
	}
	if count := strings.Count(view, `data-hvac-diagram-key=`); count != 2 {
		t.Fatalf("HVAC loop and Zone Services SVGs must both expose the delegated diagram key, got %d occurrences", count)
	}
	if count := strings.Count(view, `class="hvac-diagram-panzoom"`); count != 2 {
		t.Fatalf("HVAC loop and Zone Services SVGs must both render a pan/zoom content group, got %d occurrences", count)
	}
	if count := strings.Count(view, "resetHVACDiagramViewport("); count < 3 {
		t.Fatalf("HVAC viewport must reset from the Fit icon and document reset paths, got %d reset references", count)
	}
	if !strings.Contains(view, "renderHVACLoopDiagram(loop, { interactive: true })") {
		t.Fatal("the primary HVAC loop view must opt into interaction without changing Simulation's reused loop diagram")
	}
}

func TestFrontendHVACInspectorIsRemovedAndExpandUsesViewportIcon(t *testing.T) {
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
		`id="hvacInspector"`,
		`id="hvacInspectorStats"`,
		`aria-label="HVAC inspector"`,
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

func TestFrontendHVACLoopRootDoesNotDimDiagramItems(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	for _, required := range []string{
		`const activeKey = state.activeHVACGraphKey || ""`,
		`if (!activeKey || activeKey.startsWith("loop:"))`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("opening an HVAC loop must preserve normal diagram contrast until an inner item is selected: missing %q", required)
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
