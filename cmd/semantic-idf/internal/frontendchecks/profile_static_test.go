package frontendchecks

import (
	"os"
	"strings"
	"testing"
)

func TestFrontendProfileGraphStateContracts(t *testing.T) {
	files := map[string]string{
		"profile views": readTestFile(t, "frontend/src/js/views/profile-views.js"),
		"state":         readTestFile(t, "frontend/src/js/state.js"),
		"styles":        readTestFile(t, "frontend/src/styles/profile.css"),
	}
	required := map[string][]string{
		"profile views": {
			"profileGraphSeries",
			"currentProfileGraphOptions",
			"tabindex=\"0\"",
			"keydown",
			"profile-graph-view-switch",
			"data-profile-time-view",
			"aria-pressed",
			"downsampleValues",
			"profileSelectedDimensions",
			`class="profile-view-switch" role="group"`,
			`aria-pressed="${state.activeProfileView === "profile" ? "true" : "false"}"`,
		},
		"state": {
			"profileSelectedGroupIds: []",
			"profileSelectedZoneNames: []",
			"profileSelectedDimensions: []",
			`profileSelectionAnchorKey: ""`,
		},
		"styles": {
			".profile-live-controls",
			"align-items: start",
			".profile-live-group",
			"grid-template-rows: auto minmax(32px, auto)",
			".profile-view-switch",
			"width: max-content",
			"justify-self: start",
			".profile-graph-view-switch",
			".profile-overlay-graph",
		},
	}
	for label, terms := range required {
		for _, term := range terms {
			if !strings.Contains(files[label], term) {
				t.Fatalf("%s missing Profile Graph state contract %q", label, term)
			}
		}
	}
}

func TestFrontendProfileRenderReusesIndexesAndDelegatesDynamicControls(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	render := sliceBetween(views, "export function renderProfile", "function renderEmptyProfile")
	if strings.Contains(render, "bindProfileControls") {
		t.Fatal("Profile render must not recreate dynamic control listeners")
	}
	for _, required := range []string{
		"profileItemMapCache = new WeakMap()",
		"profileSemanticNavigationCache",
		`cache.occurrenceIDs("view-target"`,
		"const viewSettings = profileNavigationRevealDimension",
		"renderProfileSettings(profile)",
		"renderProfileOverview(visibleGroups, visibleRows)",
		"renderProfileGraph(graphGroup, profile)",
		"const selectedGroupIDs = new Set(",
		"const selectedZoneNames = new Set(",
		"bindProfileControls();",
		`elements.profileOverview?.addEventListener("click", handleProfileOverviewActivation)`,
		`elements.profileGraph?.addEventListener("click", handleProfileGraphActivation)`,
	} {
		if !strings.Contains(views, required) {
			t.Fatalf("Profile render optimization contract is missing %q", required)
		}
	}
}

func TestFrontendProfileRowsDriveSingleToggleAndRangeSelection(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/views/profile-views.js")
	state := readTestFile(t, "frontend/src/js/state.js")
	for _, term := range []string{
		"profileSelectedGroupIds: []",
		"profileSelectedZoneNames: []",
		`profileSelectionAnchorKey: ""`,
	} {
		if !strings.Contains(state, term) {
			t.Fatalf("profile row-selection state is missing %q", term)
		}
	}
	for _, term := range []string{
		"data-profile-row-key",
		`role="option"`,
		`aria-selected="${selected ? "true" : "false"}"`,
		"profileSelectedGroupIds",
		"profileSelectedZoneNames",
		"profileSelectionAnchorKey",
		"event.ctrlKey",
		"event.metaKey",
		"event.shiftKey",
		`const rowKey = button.dataset.profileRowKey || ""`,
		"handleProfileRowSelection(event, rowKey)",
	} {
		if !strings.Contains(content, term) {
			t.Fatalf("profile table row selection is missing %q", term)
		}
	}
	if strings.Contains(content, `aria-selected="${selected ? "true" : "false"}""`) {
		t.Fatal("profile option row contains a malformed aria-selected attribute")
	}
}

func TestFrontendProfileOverviewUsesStructuredMetricsAndCountAssignments(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	styles := readTestFile(t, "frontend/src/styles/profile.css")
	overview := sliceBetween(views, "function renderProfileOverview", "function renderProfileAssignment")
	assignment := sliceBetween(views, "function renderProfileAssignment", "function renderProfileMetrics")
	metrics := sliceBetween(views, "function renderProfileMetrics", "function renderProfileGroupCard")
	groupCard := sliceBetween(views, "function renderProfileGroupCard", "function renderProfileZoneCard")
	zoneCard := sliceBetween(views, "function renderProfileZoneCard", "function renderProfileGraph")

	for _, required := range []string{
		"lastProfileView?.dimensions",
		"state.profileSettings.enabledDimensions.includes(dimension.id)",
		"profileDimensionLabel(dimension.id)",
		"renderProfileGroupCard(group, selectedGroupIDs, metricDimensions)",
		"renderProfileZoneCard(row, selectedZoneNames, metricDimensions)",
	} {
		if !strings.Contains(overview, required) {
			t.Fatalf("Profile overview fixed metric-order contract is missing %q", required)
		}
	}
	for _, required := range []string{
		`const summary = t("count.zones", { count: zoneCount })`,
		"const accessibleDetail = detail ? `${summary}: ${detail}` : summary",
		`class="profile-card-assignment"`,
		`title="${escapeHTML(detail || summary)}"`,
		`aria-label="${escapeHTML(accessibleDetail)}"`,
		`>${escapeHTML(summary)}</span>`,
	} {
		if !strings.Contains(assignment, required) {
			t.Fatalf("Profile assignment count/detail contract is missing %q", required)
		}
	}
	for _, required := range []string{
		"const summaryByDimension = new Map(",
		"const visibleDimensions = dimensions.length",
		"const presentation = profileMetricPresentation(summary, dimension)",
		`class="profile-card-metrics"`,
		`class="profile-card-metric ${presentation.className}"`,
		`data-profile-dimension="${escapeHTML(dimension.id)}"`,
		`data-profile-metric-status="${escapeHTML(presentation.status)}"`,
		`aria-label="${escapeHTML(presentation.description)}"`,
		`class="profile-card-metric-label"`,
		`class="profile-card-metric-value"`,
		`.join("")`,
	} {
		if !strings.Contains(metrics, required) {
			t.Fatalf("Profile metric cell contract is missing %q", required)
		}
	}
	if strings.Contains(metrics, `.join(" / ")`) {
		t.Fatal("Profile overview metrics must not flatten dimension cells with slash separators")
	}
	if strings.Contains(metrics, `N/A`) {
		t.Fatal("Profile overview must use an em dash plus an accessible reason instead of N/A")
	}
	for _, required := range []string{
		"function profileMetricPresentation",
		`displayValue: "—"`,
		`status: "not-configured"`,
		`status === "unavailable"`,
		`status === "partial"`,
		`"profile.metricWarnings"`,
	} {
		if !strings.Contains(metrics, required) {
			t.Fatalf("Profile metric availability presentation is missing %q", required)
		}
	}

	for label, renderer := range map[string]string{"group": groupCard, "zone": zoneCard} {
		for _, required := range []string{`role="option"`, `aria-selected="${selected ? "true" : "false"}"`, "renderProfileMetrics("} {
			if !strings.Contains(renderer, required) {
				t.Fatalf("Profile %s row is missing %q", label, required)
			}
		}
		for _, forbiddenRole := range []string{`role="table"`, `role="row"`, `role="cell"`} {
			if strings.Contains(renderer, forbiddenRole) {
				t.Fatalf("Profile %s row must preserve listbox/option semantics, found %q", label, forbiddenRole)
			}
		}
	}
	if !strings.Contains(groupCard, `group.zoneNames.join("\n")`) {
		t.Fatal("Profile assignment hover detail must list zones vertically")
	}
	for _, required := range []string{
		"renderProfileAssignment(group.zoneCount, assignmentDetail)",
		"renderProfileMetrics(group.dimensions, metricDimensions)",
	} {
		if !strings.Contains(groupCard, required) {
			t.Fatalf("Profile group row assignment/metric contract is missing %q", required)
		}
	}
	for _, required := range []string{
		`class="profile-card-assignment profile-card-profile-name"`,
		`title="${escapeHTML(row.groupName || t("profile.noProfileGroup"))}"`,
		`>${escapeHTML(row.groupName || t("profile.noProfileGroup"))}</span>`,
		"renderProfileMetrics(row.dimensions, metricDimensions)",
	} {
		if !strings.Contains(zoneCard, required) {
			t.Fatalf("Profile zone row assignment/metric contract is missing %q", required)
		}
	}
	if strings.Contains(zoneCard, "renderProfileAssignment(") {
		t.Fatal("Profile Zone row must not render a redundant one-zone count assignment")
	}
	if strings.Contains(zoneCard, `t("profile.receivesProfile"`) {
		t.Fatal("Profile Zone row must show only the assigned Profile name, without a receives-Profile sentence")
	}

	for _, required := range []string{
		".profile-card-assignment",
		"cursor: help",
		".profile-card-profile-name",
		".profile-card-metrics",
		"repeat(var(--profile-metric-columns, 1), minmax(92px, 1fr))",
		"display: contents",
		".profile-card-metric",
		".profile-card-metric-label",
		".profile-card-metric-value",
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("Profile structured overview styling is missing %q", required)
		}
	}
}

func TestFrontendProfileAvailabilityAndAirflowMetricsHaveEnglishAndKoreanLabels(t *testing.T) {
	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	for _, key := range []string{
		"profile.metricFallback",
		"profile.metricNotConfigured",
		"profile.metricPartial",
		"profile.metricUnavailable",
		"profile.metricWarnings",
		"profile.metric.infiltration.flow_per_exterior_wall_area",
		"profile.metric.infiltration.effective_leakage_area",
		"profile.metric.infiltration.flow_coefficient",
		"profile.metric.ventilation.opening_area",
	} {
		if count := strings.Count(i18n, `"`+key+`":`); count != 2 {
			t.Fatalf("Profile translation key %q appears %d times, want English and Korean entries", key, count)
		}
	}
}

func TestFrontendProfileMetricAggregationMatchesBackendEngineeringBasis(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	for _, required := range []string{
		`const metricKey = [series.dimension, series.metricId, series.unit].join("\u0000")`,
		`${item.dimension || "unknown"}\u0000${item.metricId || ""}\u0000${metric.unit || ""}`,
		`const itemsShareTarget = () =>`,
		`item.sourceTargetKind || ""`,
		`item.aggregationSignature || ""`,
		`if (items.length <= 1) return true`,
		`current.includes("missing") || current.includes("invalid:")`,
		`["flow_coefficient", "effective_leakage_area", "opening_area"].includes(metricID)`,
		`{ ...direct, value: 0, resolvedCount: 0, completeCount: 0 }`,
		`ach: 3,`,
	} {
		if !strings.Contains(views, required) {
			t.Fatalf("Profile frontend/backend metric parity is missing %q", required)
		}
	}
}

func TestFrontendProfileZoneSeriesAndScheduleContributionGroupingContracts(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	zoneSeries := sliceBetween(views, "function profileGraphSeries", "function profileCurrentGroupSeries")
	for _, required := range []string{
		`state.activeProfileView === "zone"`,
		`series.scopeType === "zone" && selectedZones.has(series.zoneName)`,
		"label: `${series.zoneName} / ${series.dimensionLabel || profileDimensionLabel(series.dimension)}`",
	} {
		if !strings.Contains(zoneSeries, required) {
			t.Fatalf("Inspect by Zones must retain each selected Zone series and label, missing %q", required)
		}
	}

	grouping := sliceBetween(views, "function summarizeDimension", "function profileMetricCandidates")
	for _, required := range []string{
		"profileScheduleContributionSignature(items, selectedMetricID, settings)",
		"function profileScheduleContributionSignature",
		`const contributions = new Map()`,
		`contributions.set(schedule, (contributions.get(schedule) || 0) + value)`,
		`const total = [...contributions.values()].reduce((sum, value) => sum + value, 0)`,
		`const fraction = Math.abs(total) > 1e-12 ? value / total : 0`,
		`.sort(([left], [right]) => left.localeCompare(right))`,
		"contributionSignature,",
	} {
		if !strings.Contains(grouping, required) {
			t.Fatalf("Profile grouping must include schedule-specific engineering contributions, missing %q", required)
		}
	}
	groupKey := sliceBetween(views, "function profileGroupKey", "function mergeProfileSettings")
	for _, required := range []string{
		`const schedule = dimension.contributionSignature || ""`,
		`const metricRole = dimension.fallbackMetric ? "fallback" : "preferred"`,
		`${dimension.dimension}:${dimension.metricId}:${metricRole}:`,
	} {
		if !strings.Contains(groupKey, required) {
			t.Fatalf("Profile group identity is missing schedule/selected-metric/fallback identity %q", required)
		}
	}
}

func TestProfileGraphLabelFontSizeIsConfigurable(t *testing.T) {
	settings := readTestFile(t, "frontend/src/js/settings-client.js")
	markup := readTestFile(t, "frontend/src/settings.html")
	styles := readTestFile(t, "frontend/src/styles/profile.css")
	for _, required := range []string{`graphFontSize: 11`, `--graph-label-font-size`, `clampNumber(appearance.graphFontSize, 9, 18`} {
		if !strings.Contains(settings, required) {
			t.Fatalf("graph font setting contract is missing %q", required)
		}
	}
	for _, required := range []string{`id="graphFontSize"`, `min="9" max="18"`, `graphFontSize: document.querySelector("#graphFontSize").value`} {
		if !strings.Contains(markup, required) {
			t.Fatalf("graph font settings UI is missing %q", required)
		}
	}
	for _, required := range []string{`font-size: var(--graph-label-font-size, 11px)`, `calc(var(--graph-label-font-size, 11px) * 5.1)`} {
		if !strings.Contains(styles, required) {
			t.Fatalf("Profile graph font styling is missing %q", required)
		}
	}
}

func TestFrontendProfileRemovesMatrixDetailSourceAndCandidateUI(t *testing.T) {
	files := map[string]string{
		"markup":     readTestFile(t, "frontend/src/index.html"),
		"state":      readTestFile(t, "frontend/src/js/state.js"),
		"analysis":   readTestFile(t, "frontend/src/js/views/analysis-views.js"),
		"views":      readTestFile(t, "frontend/src/js/views/profile-views.js"),
		"styles":     readTestFile(t, "frontend/src/styles/profile.css"),
		"responsive": readTestFile(t, "frontend/src/styles/responsive.css"),
		"i18n":       readTestFile(t, "frontend/src/js/i18n.js"),
	}
	removed := map[string][]string{
		"markup": {
			`id="profileMatrixStats"`,
			`id="profileMatrix"`,
			`id="profileDetail"`,
			`class="profile-matrix"`,
			`class="profile-detail-panel"`,
			`data-i18n="profile.matrix"`,
			`data-i18n="profile.sourceObjects"`,
		},
		"state": {
			"profileSelectedCell",
			"profileMatrixStats:",
			"profileMatrix:",
			"profileDetail:",
		},
		"analysis": {
			"elements.profileMatrixStats",
			"elements.profileMatrix",
			"elements.profileDetail",
		},
		"views": {
			"PROFILE_MATRIX_RENDER_LIMIT",
			"profileMatrixSemanticTargets",
			"function renderProfileDetail",
			"function renderProfileItemRow",
			"function renderProfileCandidatePanel",
			"function renderProfileSourceAccordion",
			"function renderProfileMatrix",
			"function renderProfileMatrixCell",
			"function temporaryProfileDimensionSummary",
			"function renderProfileCandidateRow",
			"function profileMatrixCellClasses",
			"function selectProfileMatrixCell",
			"function selectProfileCellData",
			"function cloneProfileSelectedCell",
			"function handleProfileDetailActivation",
			"function handleProfileMatrixActivation",
			"function profileActiveMatrixZoneNames",
			"function profileCandidatesForDimensions",
			"data-profile-cell",
			"data-profile-candidate-id",
			"profile-source-accordion",
			"parameterCandidates",
		},
		"styles": {
			".profile-matrix",
			".profile-detail-panel",
			".profile-detail-head",
			".profile-detail-actions",
			".profile-item-table",
			".profile-item-row",
			".profile-candidate-panel",
			".profile-qa-row",
			".profile-source-accordion",
			".profile-source-accordion-list",
			".profile-source-metrics",
		},
		"responsive": {
			".profile-matrix",
			".profile-detail-panel",
			".profile-detail-head",
			".profile-detail-actions",
			".profile-item-table",
			".profile-source-accordion",
		},
		"i18n": {
			`"profile.matrix"`,
			`"profile.noMatrix"`,
			`"profile.sourceObjects"`,
		},
	}
	for label, terms := range removed {
		for _, term := range terms {
			if strings.Contains(files[label], term) {
				t.Fatalf("%s still contains removed Profile secondary UI contract %q", label, term)
			}
		}
	}

	for _, required := range []string{
		`id="profileOverview"`,
		`id="profileGraph"`,
		`id="profileApplyButton"`,
	} {
		if !strings.Contains(files["markup"], required) {
			t.Fatalf("Profile core markup was removed with the secondary UI: %q", required)
		}
	}
	for _, required := range []string{
		"renderProfileSettings(profile)",
		"renderProfileOverview(visibleGroups, visibleRows)",
		"renderProfileGraph(graphGroup, profile)",
		"function profileApplyRequest",
		"const itemMap = profileItemMap(profile)",
		"sourceObjectIndexes",
		"profileNavigationRevealDimension",
	} {
		if !strings.Contains(files["views"], required) {
			t.Fatalf("Profile Graph/Apply/navigation contract was removed with the secondary UI: %q", required)
		}
	}
}

func TestFrontendProfileLineViewsAlwaysUseLegendAndAnnualViewsUseParallelHeatmaps(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	styles := readTestFile(t, "frontend/src/styles/profile.css")
	body := sliceBetween(views, "function renderProfileGraphBody", "function renderProfileOverlay")
	if strings.Contains(body, "compareMode") {
		t.Fatal("Profile line overlay must not be gated behind the optional Compare mode")
	}
	for _, required := range []string{
		"renderProfileOverlay",
		"renderProfileSeriesLegend",
		"renderProfileAnnualHeatmaps",
		"profile-overlay-legend",
		"profile-annual-heatmap-grid",
		`canvas class="profile-heatmap"`,
		"paintProfileHeatmaps",
		`getContext?.("2d")`,
		"fillRect(",
	} {
		if !strings.Contains(views, required) && !strings.Contains(styles, required) {
			t.Fatalf("Profile graph presentation is missing %q", required)
		}
	}
	if !strings.Contains(styles, ".profile-annual-heatmap-grid") {
		t.Fatal("annual Profile heatmaps need a dedicated wrapping grid for parallel comparison")
	}
	if !strings.Contains(styles, ".profile-overlay-legend") {
		t.Fatal("Profile line overlay legend styling is missing")
	}
}

func TestFrontendProfileGraphsExposeEngineeringFidelityStatusAndReasons(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	styles := readTestFile(t, "frontend/src/styles/profile.css")
	i18n := readTestFile(t, "frontend/src/js/i18n.js")

	for _, required := range []string{
		"profileNominalWarningCodes",
		`"nominal_outdoor_air_profile"`,
		`"weather_modified_design_flow_basis"`,
		`"weather_dependent_opening_profile"`,
		`"schedule_profile_fallback"`,
		"function profileSeriesFidelity",
		"function profilePanelFidelity",
		"function renderProfileFidelityBadge",
		`data-profile-fidelity="${fidelity.kind}"`,
		`role="status"`,
		`title="${escapeHTML(fidelity.description)}"`,
		`aria-label="${escapeHTML(fidelity.description)}"`,
	} {
		if !strings.Contains(views, required) {
			t.Fatalf("Profile graph fidelity presentation is missing %q", required)
		}
	}
	if strings.Count(views, "${renderProfileFidelityBadge(") < 2 {
		t.Fatal("both overlay and annual Profile metric panels must expose fidelity badges")
	}
	for _, required := range []string{
		`.profile-fidelity-badge`,
		`.profile-fidelity-badge.is-partial`,
		`flex-wrap: wrap`,
		`white-space: nowrap`,
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("responsive Profile fidelity badge styling is missing %q", required)
		}
	}
	for _, required := range []string{
		`"profile.fidelityNominal": "Nominal"`,
		`"profile.fidelityPartial": "Partial"`,
		`"profile.fidelityNominal": "명목"`,
		`"profile.fidelityPartial": "부분"`,
	} {
		if !strings.Contains(i18n, required) {
			t.Fatalf("Profile fidelity EN/KO localization is missing %q", required)
		}
	}
}

func TestFrontendProfileIdenticalRenderedCurvesUseInterleavedColorsAndLineLegend(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	styles := readTestFile(t, "frontend/src/styles/profile.css")
	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	legend := sliceBetween(views, "function renderProfileSeriesLegend", "function profileYAxisScale")
	overlay := sliceBetween(views, "function renderOverlayGraph", "function renderProfileGraphSummary")
	pathRendering := sliceBetween(overlay, "const paths = renderedPaths", "const horizontalGrid")

	for _, required := range []string{
		"const renderedPaths = items",
		"const renderedByPath = new Map()",
		"renderedByPath.get(entry.path)",
		"renderedByPath.set(entry.path, matching)",
		"entry.item.visualOverlapCount = matching.length",
		"entry.item.visualOverlapIndex = overlapIndex",
		`class="profile-overlap-path"`,
		`stroke-dasharray="8 ${8 * (overlapCount - 1)}"`,
		`stroke-dashoffset="${-8 * overlapIndex}"`,
		`data-profile-overlap-count="${overlapCount}"`,
		`data-profile-overlap-index="${overlapIndex}"`,
		`d="${path}"`,
	} {
		if !strings.Contains(overlay, required) {
			t.Fatalf("Profile displayed-curve overlap rendering is missing %q", required)
		}
	}
	pathBuilt := strings.Index(overlay, "const renderedPaths = items")
	pathGrouped := strings.Index(overlay, "const renderedByPath = new Map()")
	if pathBuilt < 0 || pathGrouped < 0 || pathBuilt >= pathGrouped {
		t.Fatal("Profile overlap detection must group final rendered SVG paths after downsampling and coordinate rounding")
	}
	for _, forbidden := range []string{`transform=`, `translate(`, `translateY(`} {
		if strings.Contains(pathRendering, forbidden) {
			t.Fatalf("Profile overlap rendering must not distort values with path jitter or translation: %q", forbidden)
		}
	}

	for _, required := range []string{
		"let accessibleLabel = overlapDescription",
		"const fidelity = profileSeriesFidelity(item)",
		"if (fidelity?.description)",
		"accessibleLabel = `${accessibleLabel}; ${fidelity.description}`",
		`data-profile-overlap-count="${visualOverlapCount}"`,
		`data-profile-overlap-index="${visualOverlapIndex}"`,
		`aria-label="${escapeHTML(accessibleLabel)}"`,
		`title="${escapeHTML(accessibleLabel)}"`,
		`class="profile-line-swatch ${visualOverlapCount > 1 ? "is-overlap" : ""}"`,
		`aria-hidden="true"`,
		`--profile-overlap-period:${6 * visualOverlapCount}px`,
		`--profile-overlap-offset:${-6 * visualOverlapIndex}px`,
		`class="profile-line-label"`,
	} {
		if !strings.Contains(legend, required) {
			t.Fatalf("Profile shared-curve legend metadata/accessibility contract is missing %q", required)
		}
	}
	if !strings.Contains(i18n, `"profile.sharedCurve": "{count} profiles overlap at the current scale"`) {
		t.Fatal("Profile shared-curve description must explain that overlap is based on the displayed scale")
	}

	for _, required := range []string{
		".profile-overlay-paths path.profile-overlap-path",
		"stroke-linecap: butt",
		".profile-overlay-legend .profile-line-swatch",
		"width: 18px",
		"height: 3px",
		"flex: 0 0 auto",
		".profile-overlay-legend .profile-line-swatch.is-overlap",
		"repeating-linear-gradient",
		"var(--profile-overlap-period, 6px)",
		"background-position-x: var(--profile-overlap-offset, 0)",
		".profile-overlay-legend .profile-line-label",
		"text-overflow: ellipsis",
		"white-space: nowrap",
		".analysis-panel .profile-overlay-legend .semantic-related",
		"box-shadow: none",
		"outline: 1px dashed",
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("Profile shared-curve line/legend styling is missing %q", required)
		}
	}
}

func TestFrontendProfileAxesUseSemanticHTMLResponsiveDensityAndSeparateHeatmapScale(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	styles := readTestFile(t, "frontend/src/styles/profile.css")
	responsive := readTestFile(t, "frontend/src/styles/responsive.css")

	graphBody := sliceBetween(views, "function renderProfileGraphBody", "function renderProfileOverlay")
	overlayPanels := sliceBetween(views, "function renderProfileOverlay", "function profileMetricSeriesGroups")
	xAxisSpec := sliceBetween(views, "function profileXAxisSpec", "function profileMonthLabels")
	heatmapAxisSpecs := sliceBetween(views, "function profileMonthLabels", "function renderProfileYAxisTicks")
	overlay := sliceBetween(views, "function renderOverlayGraph", "function renderProfileGraphSummary")
	heatmap := sliceBetween(views, "function renderHeatmap", "function paintProfileHeatmaps")
	profileSettingsAndGraph := sliceBetween(views, "function renderProfileSettings", "function paintProfileHeatmaps")

	for _, required := range []string{
		"function profileYAxisScale",
		"function profileXAxisSpec",
		"function renderProfileYAxisTicks",
		"function renderProfileXAxisTicks",
		"function renderProfileAxisTick",
		"function profileAxisDensityClass",
		`class="profile-line-chart"`,
		`class="profile-y-axis-title"`,
		`class="profile-y-axis-ticks"`,
		`class="profile-line-plot"`,
		`class="profile-x-axis-ticks"`,
		`class="profile-x-axis-title"`,
	} {
		if !strings.Contains(views, required) {
			t.Fatalf("Profile graph semantic HTML axis contract is missing %q", required)
		}
	}
	for _, required := range []string{
		".profile-line-chart",
		".profile-y-axis-title",
		".profile-y-axis-ticks",
		".profile-line-plot",
		".profile-x-axis-ticks",
		".profile-x-axis-title",
		".profile-axis-tick",
		"font-variant-numeric: tabular-nums",
		"vector-effect: non-scaling-stroke",
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("Profile graph HTML/CSS axis styling is missing %q", required)
		}
	}
	if !strings.Contains(overlay, `preserveAspectRatio="none"`) {
		t.Fatal("Profile overlay SVG must stretch its plot coordinates with preserveAspectRatio=none")
	}
	if strings.Contains(overlay, "<text") {
		t.Fatal("Profile overlay axis text must remain HTML/CSS text outside the stretchable SVG")
	}

	for view, required := range map[string][]string{
		"day": {
			`case "day"`,
			`t("profile.axisDayTypeHour"`,
			`atIndex(0, "WD 00")`,
			`atIndex(24, "Sat 00")`,
			`atIndex(48, "Sun 00")`,
			`atIndex(count - 1, "Sun 24")`,
		},
		"week": {
			`case "week"`,
			`t("profile.axisDayOfWeek"`,
			`["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]`,
			`index * 24`,
		},
		"month": {
			`case "month"`,
			`t("profile.axisMonth"`,
			`profileMonthLabels().map((label, index) => atIndex(index, label))`,
		},
		"duration": {
			`case "duration"`,
			`t("profile.axisAnnualHoursExceeded"`,
			`[0, 25, 50, 75, 100]`,
			"`${percent}%`",
		},
		"rules": {
			`case "rules"`,
			`t("profile.axisRuleInterval"`,
			`const tickCount = Math.min(7, count)`,
			`String(valueIndex + 1)`,
		},
	} {
		for _, term := range required {
			if !strings.Contains(xAxisSpec, term) {
				t.Fatalf("Profile %s View X-axis specification is missing %q", view, term)
			}
		}
	}
	for _, required := range []string{
		`options.timeView === "year"`,
		"renderProfileAnnualHeatmaps(series, options)",
	} {
		if !strings.Contains(graphBody, required) {
			t.Fatalf("Profile year View X-axis routing is missing %q", required)
		}
	}
	for _, required := range []string{
		"function profileMonthLabels",
		`["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]`,
		"function profileHeatmapMonthTicks",
		"const monthStartDays = [0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334]",
		"function profileHeatmapHourTicks",
		"[0, 6, 12, 18, 24]",
	} {
		if !strings.Contains(heatmapAxisSpecs, required) {
			t.Fatalf("Profile year View heatmap axis specification is missing %q", required)
		}
	}

	for _, required := range []string{
		`density: index === 0 || index === ticks.length - 1 ? "base"`,
		`index === midpointIndex ? "medium" : "full"`,
		`profile-axis-density-${density === "base" || density === "medium" ? density : "full"}`,
	} {
		if !strings.Contains(views, required) {
			t.Fatalf("Profile graph responsive tick-density assignment is missing %q", required)
		}
	}
	for _, required := range []string{
		"@container profile-pane",
		".profile-axis-density-full",
		".profile-axis-density-medium",
		"display: none",
	} {
		if !strings.Contains(responsive, required) {
			t.Fatalf("Profile graph responsive tick-density CSS is missing %q", required)
		}
	}
	fullDensity := sliceBetween(responsive, ".profile-axis-density-full", "}")
	mediumDensity := sliceBetween(responsive, ".profile-axis-density-medium", "}")
	if !strings.Contains(fullDensity, "display: none") || !strings.Contains(mediumDensity, "display: none") {
		t.Fatal("Profile graph container queries must progressively hide full and medium density ticks")
	}
	if strings.Index(responsive, ".profile-axis-density-full") >= strings.Index(responsive, ".profile-axis-density-medium") {
		t.Fatal("Profile graph must hide full-density ticks before medium-density ticks as the pane narrows")
	}

	for _, required := range []string{
		`class="profile-heatmap-y-title"`,
		`class="profile-heatmap-y-ticks"`,
		"profileHeatmapHourTicks().map(renderProfileAxisTick)",
		`class="profile-heatmap-x-ticks"`,
		"profileHeatmapMonthTicks().map(renderProfileAxisTick)",
		`class="profile-heatmap-x-title"`,
		`class="profile-heatmap-scale"`,
		`class="profile-heatmap-scale-title"`,
		`class="profile-heatmap-scale-bar"`,
		`class="profile-heatmap-scale-ticks"`,
		"formatAxisTick(max / 2)",
		"formatAxisTick(max)",
	} {
		if !strings.Contains(heatmap, required) {
			t.Fatalf("Profile year heatmap month/hour/value-scale separation is missing %q", required)
		}
	}
	xTicks := sliceBetween(heatmap, `class="profile-heatmap-x-ticks"`, `class="profile-heatmap-x-title"`)
	if strings.Contains(xTicks, "formatAxisTick(max") || strings.Contains(xTicks, "profileValueAxisTitle") {
		t.Fatal("Profile year heatmap X axis must contain only month ticks, not value-scale labels")
	}
	for _, required := range []string{
		".profile-heatmap-y-title",
		".profile-heatmap-y-ticks",
		".profile-heatmap-x-ticks",
		".profile-heatmap-x-title",
		".profile-heatmap-scale",
		".profile-heatmap-scale-title",
		".profile-heatmap-scale-bar",
		".profile-heatmap-scale-ticks",
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("Profile year heatmap separated-axis styling is missing %q", required)
		}
	}

	for _, forbidden := range []string{`t("graph.multiplier"`, `"Multiplier"`} {
		if strings.Contains(profileSettingsAndGraph, forbidden) {
			t.Fatalf("Profile graph still exposes the legacy visible Multiplier fallback %q", forbidden)
		}
	}
	for _, forbidden := range []string{`id="profileMetricMode"`, `currentProfileMetricMode`, `profile.scheduleFraction`} {
		if strings.Contains(profileSettingsAndGraph, forbidden) {
			t.Fatalf("Profile fixed Time Profile UI still exposes legacy Metric mode %q", forbidden)
		}
	}
	if strings.Contains(overlayPanels, `group.unit ||`) || strings.Contains(overlayPanels, `t("graph.multiplier"`) {
		t.Fatal("Profile graph panel headings must not use Multiplier as a fallback unit")
	}
}

func TestFrontendProfileGraphControlsUseFixedTimeProfileAndDirectViewButtons(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	state := readTestFile(t, "frontend/src/js/state.js")
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	analysis := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	settingsPage := readTestFile(t, "frontend/src/settings.html")
	settingsClient := readTestFile(t, "frontend/src/js/settings-client.js")
	styles := readTestFile(t, "frontend/src/styles/profile.css")
	responsive := readTestFile(t, "frontend/src/styles/responsive.css")
	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	graphBody := sliceBetween(views, "function renderProfileGraph", "function renderProfileGraphBody")
	bindings := sliceBetween(views, "function bindProfileControls", "export function initializeProfileControls")
	for label, content := range map[string]string{
		"markup":   markup,
		"state":    state,
		"views":    views,
		"analysis": analysis,
	} {
		for _, removed := range []string{"profileGraphStats", "profileGraphDeckStats"} {
			if strings.Contains(content, removed) {
				t.Fatalf("%s still exposes redundant Profile Graph metadata %q", label, removed)
			}
		}
	}
	for _, removed := range []string{
		`id="profileScheduleSummaryMode"`,
		`for="profileScheduleSummaryMode"`,
		`document.querySelector("#profileScheduleSummaryMode")`,
	} {
		if strings.Contains(settingsPage, removed) {
			t.Fatalf("Settings still exposes the legacy Profile View selector %q", removed)
		}
	}
	for _, required := range []string{
		`class="profile-graph-view-switch"`,
		`role="group"`,
		`data-profile-time-view`,
		`aria-pressed`,
		`"day"`,
		`"week"`,
		`"month"`,
		`"year"`,
		`"duration"`,
		`"rules"`,
	} {
		if !strings.Contains(graphBody, required) {
			t.Fatalf("Profile Graph direct View buttons are missing %q", required)
		}
	}
	for _, removed := range []string{
		`id="profileGraphPreset"`,
		`id="profileGraphScopeType"`,
		`id="profileGraphCompareMode"`,
		`id="profileGraphTimeView"`,
		`"#profileGraphPreset"`,
		`"#profileGraphScopeType"`,
		`"#profileGraphCompareMode"`,
		`"#profileGraphTimeView"`,
		"applyProfileGraphPreset",
		"currentProfilePresetID",
		`selectionDriven: false`,
		`selectionDriven = false`,
		`scopeType: "schedule"`,
		`scopeType = "schedule"`,
		`compareMode: "similarity"`,
		`compareMode = "similarity"`,
		`compareMode: "outliers"`,
		`compareMode = "outliers"`,
	} {
		if strings.Contains(views, removed) {
			t.Fatalf("Profile Graph still exposes a removed Graph Type/Scope/Compare/View selector contract %q", removed)
		}
	}
	if strings.Contains(graphBody, `id="profileGraphScaleMode"`) {
		t.Fatal("Profile Graph toolbar still owns the Scale selector")
	}
	for _, removed := range []string{`id="profileGraphScaleMode"`, `"#profileGraphScaleMode"`} {
		if strings.Contains(views, removed) {
			t.Fatalf("Profile tab still owns the removed Scale selector %q", removed)
		}
	}
	deadContracts := map[string][]string{
		"views": {
			"profileGraphDeck",
			"mergeProfileGraphDeck",
			"profileDeckSeries",
			"renderProfileDeckBody",
			"renderProfileSeriesCard",
			"renderProfileSeriesRanking",
			"renderProfileScheduleSimilarity",
			"renderScheduleClusterScatter",
			"renderProfileOutlierDeck",
			"renderProfileOutlierRow",
			"renderProfileGraphCard",
			"renderGraphVisual",
			"graphDataForDimension",
			"profileSeriesGraphData",
			"fallbackRulesForSeries",
			"scheduleLookupMap",
			"scheduleForProfileDimension",
			"profileScheduleSemanticAttributes",
			`querySelectorAll("[data-profile-series-focus]")`,
			`elements.profileGraph.querySelectorAll("[data-profile-candidate-id]")`,
			"profilePinnedSeriesIds",
			"pinnedSeriesIds",
		},
		"state": {"profileGraphDeck", "profilePinnedSeriesIds"},
		"styles": {
			".profile-graph-card",
			".profile-pin-button",
			".profile-ranking-row",
			".profile-cluster-row",
			".profile-similarity-grid",
			".profile-scatter",
		},
		"responsive": {
			".profile-ranking-row",
			".profile-cluster-row",
			".profile-similarity-grid",
			".profile-scatter",
		},
		"i18n": {`"profile.graphType"`, `"profile.scheduleSummary"`},
	}
	deadSources := map[string]string{
		"views": views, "state": state, "styles": styles, "responsive": responsive, "i18n": i18n,
	}
	for label, terms := range deadContracts {
		for _, term := range terms {
			if strings.Contains(deadSources[label], term) {
				t.Fatalf("%s still contains unreachable Profile Graph contract %q", label, term)
			}
		}
	}
	for _, required := range []string{
		`elements.profileGraph?.addEventListener("click", handleProfileGraphActivation)`,
		`event.target.closest("[data-profile-time-view]")`,
		`timeViewButton.dataset.profileTimeView`,
		`focus({ preventScroll: true })`,
	} {
		if !strings.Contains(bindings, required) {
			t.Fatalf("Profile direct View binding is missing %q", required)
		}
	}
	for _, required := range []string{
		`id="profileScaleMode"`,
		`optionHTML("auto"`,
		`optionHTML("shared"`,
		`optionHTML("design_peak"`,
		`optionHTML("multiplier_0_1"`,
		`optionHTML("percentile"`,
		`scaleMode: document.querySelector("#profileScaleMode").value`,
	} {
		if !strings.Contains(settingsPage, required) {
			t.Fatalf("Settings Profile Analysis Scale field is missing %q", required)
		}
	}
	for _, required := range []string{
		`scaleMode: "auto"`,
		`scaleMode: normalizeChoice(`,
		`profile.scaleMode`,
		`["auto", "shared", "design_peak", "multiplier_0_1", "percentile"]`,
	} {
		if !strings.Contains(settingsClient, required) {
			t.Fatalf("Profile Scale default/normalization contract is missing %q", required)
		}
	}
	for _, removed := range []string{`metricMode: "actual"`, `metricMode: normalizeChoice(`, `profile.metricMode`, `profileMetricModeFromLegacy`} {
		if strings.Contains(settingsClient, removed) {
			t.Fatalf("Profile Settings still persists removed Metric mode %q", removed)
		}
	}
	for _, legacyOutput := range []string{`graphMode: normalizeChoice(`, `scheduleSummaryMode: normalizeChoice(`} {
		if strings.Contains(settingsClient, legacyOutput) {
			t.Fatalf("Profile settings still emits legacy graph state %q", legacyOutput)
		}
	}
}

func TestFrontendProfileUsesTableAboveGraphWithoutTopFilter(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	state := readTestFile(t, "frontend/src/js/state.js")
	views := readTestFile(t, "frontend/src/js/views/profile-views.js")
	analysis := readTestFile(t, "frontend/src/js/views/analysis-views.js")
	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	styles := readTestFile(t, "frontend/src/styles/profile.css")

	for label, content := range map[string]string{
		"markup":     markup,
		"state":      state,
		"views":      views,
		"analysis":   analysis,
		"simulation": simulation,
		"i18n":       i18n,
	} {
		for _, removed := range []string{"profileStats", "profileFilter"} {
			if strings.Contains(content, removed) {
				t.Fatalf("%s still contains removed Profile header/filter state %q", label, removed)
			}
		}
	}
	for _, removed := range []string{
		`class="summary-head profile-head"`,
		`data-i18n-placeholder="profile.filter"`,
	} {
		if strings.Contains(markup, removed) {
			t.Fatalf("Profile markup still contains removed top-header UI %q", removed)
		}
	}
	for _, removed := range []string{
		"function profileQuery",
		"function profileGroupMatchesQuery",
		"function profileMatrixRowMatchesQuery",
		"function profileRevealMatchesGroup",
		"function profileRevealMatchesRow",
	} {
		if strings.Contains(views, removed) {
			t.Fatalf("Profile views still contain removed filter behavior %q", removed)
		}
	}

	profilePane := sliceBetween(markup, `<div class="result-pane" id="profilePane">`, `<div class="result-pane" id="hvacPane">`)
	tableStart := strings.Index(profilePane, `<div class="profile-overview-table"`)
	profileVisualStart := strings.Index(profilePane, `<section class="profile-visual"`)
	if tableStart < 0 || profileVisualStart < 0 || tableStart >= profileVisualStart {
		t.Fatal("Profile visual table must appear above the Profile graph")
	}
	tableHead := sliceBetween(profilePane, `<div class="profile-overview-table-head"`, `<div id="profileOverview"`)
	tableMarkup := sliceBetween(profilePane, `<div class="profile-overview-table"`, `<section class="profile-visual"`)
	for _, required := range []string{
		`id="profileOverview" class="profile-overview-list" role="listbox" aria-multiselectable="true"`,
		`class="profile-overview-apply-head"`,
		`id="profileApplyButton" class="profile-apply-badge" type="button"`,
	} {
		if !strings.Contains(profilePane, required) && !strings.Contains(tableHead, required) {
			t.Fatalf("Profile visual table/header is missing %q", required)
		}
	}
	if strings.Count(markup, `id="profileApplyButton"`) != 1 {
		t.Fatal("Profile markup must contain exactly one Apply Profile button")
	}
	for _, forbiddenRole := range []string{`role="table"`, `role="row"`, `role="columnheader"`, `role="rowgroup"`} {
		if strings.Contains(tableMarkup, forbiddenRole) {
			t.Fatalf("Profile visual table must preserve native button/listbox semantics, found %q", forbiddenRole)
		}
	}
	graphSection := profilePane[profileVisualStart:]
	graphHeader := sliceBetween(graphSection, `<div class="profile-section-head">`, `</div>`)
	if strings.Contains(graphHeader, `id="profileApplyButton"`) {
		t.Fatal("Apply Profile must live in the top visual-table header, not the graph header")
	}
	for _, required := range []string{
		".profile-overview-table",
		".profile-overview-table-head",
		".profile-table-row",
		".profile-overview-apply-head",
		".profile-apply-badge",
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("Profile visual table styling is missing %q", required)
		}
	}
	for _, removed := range []string{
		"overflow-x: auto",
		"min-width: 760px",
		"grid-template-columns: minmax(250px, min(32%, 360px)) minmax(0, 1fr)",
	} {
		if strings.Contains(styles, removed) {
			t.Fatalf("Profile still contains a horizontal/two-column overflow source %q", removed)
		}
	}

	for _, retained := range []string{
		"if (!elements.profileGraph)",
		"renderProfileGraph(graphGroup, profile)",
		"elements.profileApplyButton.disabled = !graphGroup",
		`elements.profileApplyButton?.addEventListener("click", openProfileApplyDialog)`,
		"profileNavigationRevealDimension",
		"navigationRevealDimension: profileNavigationRevealDimension",
		"captureProfileNavigationContext",
		"restoreProfileNavigationContext",
	} {
		if !strings.Contains(views, retained) {
			t.Fatalf("Profile graph/navigation behavior was removed with the header: %q", retained)
		}
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(repoPath(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
