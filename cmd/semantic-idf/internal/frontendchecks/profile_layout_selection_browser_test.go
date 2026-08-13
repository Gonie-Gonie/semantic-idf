package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestProfileLayoutAndSelectionBrowserHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser Profile layout/selection harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/profile-layout-selection", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, profileLayoutSelectionHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runHarness := func(label string, width int, expectedSignals []string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, chrome,
			"--headless=new",
			"--disable-gpu",
			"--disable-dev-shm-usage",
			"--no-sandbox",
			"--no-first-run",
			"--no-default-browser-check",
			"--virtual-time-budget=30000",
			fmt.Sprintf("--window-size=%d,1400", width),
			"--user-data-dir="+t.TempDir(),
			"--dump-dom",
			server.URL+"/profile-layout-selection",
		)
		output, err := command.CombinedOutput()
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatalf("%s Profile layout/selection browser harness timed out:\n%s", label, output)
		}
		if err != nil {
			t.Fatalf("%s Profile layout/selection browser harness failed: %v\n%s", label, err, output)
		}
		document := string(output)
		if !strings.Contains(document, `data-profile-layout-selection-status="passed"`) {
			t.Fatalf("%s Profile layout/selection browser harness did not pass:\n%s", label, document)
		}
		for _, signal := range expectedSignals {
			if !strings.Contains(document, signal) {
				t.Fatalf("%s Profile layout/selection result is missing %s:\n%s", label, signal, document)
			}
		}
	}

	commonSignals := []string{
		`"tableAboveGraph":true`,
		`"noHorizontalOverflow":true`,
		`"applyRight":true`,
		`"domSemantics":true`,
		`"inspectByCompact":true`,
		`"profileControlGroupsAdjacent":true`,
		`"profileControlGroupsWrapNarrow":true`,
		`"profileControlsAligned":true`,
		`"secondaryProfileUIAbsent":true`,
		`"semanticRevealGraphAndOverviewFallback":true`,
		`"overviewMetricCells":true`,
		`"profileMetricAvailability":true`,
		`"spaceRatioAggregation":true`,
		`"sourceMetricCompatibility":true`,
		`"assignmentCountWithHoverDetail":true`,
		`"zoneProfileNameOnly":true`,
		`"singleSelection":true`,
		`"singleSelectionLegend":true`,
		`"ctrlToggle":true`,
		`"ctrlToggleRemoval":true`,
		`"ctrlShiftRange":true`,
		`"shiftRange":true`,
		`"zoneCtrlSelection":true`,
		`"metaToggleRemoval":true`,
		`"zoneShiftRange":true`,
		`"profileZoneModes":true`,
		`"profileZoneSelectionIsolation":true`,
		`"zoneProfileOverlaySelection":true`,
		`"graphFidelityStatusVisible":true`,
		`"overlayLegendAlways":true`,
		`"identicalCurveOverlay":true`,
		`"identicalCurveLegend":true`,
		`"semanticLegendOutline":true`,
		`"annualHeatmapsParallel":true`,
		`"annualCanvasPainted":true`,
		`"annualNoHorizontalOverflow":true`,
		`"primaryApplySourceBoundary":true`,
		`"rowGestureHistoryOnce":true`,
		`"rowGestureSemanticPrimary":true`,
		`"regroupSelectionByMembership":true`,
		`"regroupAggregateAverage":true`,
		`"fixedTimeProfileControls":true`,
		`"actualEngineeringValue":true`,
		`"viewToggleImmediate":true`,
		`"viewFocusRestored":true`,
		`"fiveViewAxes":true`,
		`"noVisibleGraphMultiplier":true`,
		`"scaleRemovedFromProfileTab":true`,
		`"legacyViewMigration":true`,
	}
	runHarness("wide", 1440, append(append([]string{}, commonSignals...), `"narrowViewport":false`, `"containerQueryWidths":true`, `"overviewRowsNoOverflowAtContainerWidths":true`, `"responsiveOverlayAxes":true`, `"responsiveAnnualAxes":true`, `"longLegendLabelAt160":true`))
	runHarness("narrow", 520, append(append([]string{}, commonSignals...), `"narrowViewport":true`))
}

const profileLayoutSelectionHarnessHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Profile layout and selection harness</title>
  <link rel="stylesheet" href="/src/styles.css">
  <style>
    html, body { margin: 0; width: 100%; min-height: 100%; }
    body { padding: 12px; box-sizing: border-box; }
    #profilePane { display: block; width: 1100px; max-width: 100%; }
    .profile-pane { height: 1280px; box-sizing: border-box; }
  </style>
</head>
<body data-profile-layout-selection-status="pending">
  <div class="result-pane active" id="profilePane">
    <div class="profile-pane">
      <div id="profileSettings" class="profile-settings"></div>
      <div class="profile-layout">
        <section class="profile-overview" aria-label="Profile overview">
          <div class="profile-overview-table" aria-label="Profile selection">
            <div class="profile-overview-table-head">
              <span>Selection</span>
              <span>Assignment</span>
              <span class="profile-overview-metrics-head">Metrics</span>
              <span class="profile-overview-apply-head">
                <button id="profileApplyButton" class="profile-apply-badge" type="button" disabled>Apply Profile</button>
              </span>
            </div>
            <div id="profileOverview" class="profile-overview-list" role="listbox" aria-multiselectable="true"></div>
          </div>
        </section>
        <section class="profile-visual" aria-label="Profile graph">
          <div class="profile-section-head"><h3>Profile Graph</h3></div>
          <div id="profileGraph" class="profile-graph"></div>
        </section>
      </div>
    </div>
  </div>
  <div id="profileApplyDialog" class="profile-dialog hidden" role="dialog" aria-modal="true" aria-labelledby="profileApplyTitle">
    <form id="profileApplyForm" class="profile-dialog-panel">
      <div class="profile-dialog-head">
        <h3 id="profileApplyTitle">Apply Profile</h3>
        <button id="profileApplyClose" type="button">Close</button>
      </div>
      <div id="profileApplyBody" class="profile-apply-body"></div>
      <div class="profile-dialog-actions">
        <span id="profileApplyStatus"></span>
        <button id="profilePreviewApply" type="button">Preview</button>
        <button id="profileConfirmApply" type="submit" disabled>Apply</button>
      </div>
    </form>
  </div>
  <pre id="result">pending</pre>
  <script type="module">
    const assert = (condition, message) => { if (!condition) throw new Error(message); };
    const nextPaint = () => Promise.resolve();
    const constantValues = (length, low, high) => Array.from({ length }, (_, index) => index % 24 < 8 ? low : high);

    function profileFixture() {
      const zones = Array.from({ length: 4 }, (_, index) => {
        const number = index + 1;
        const value = number * 0.1;
        const items = [{
          id: "item-" + number,
          zoneName: "Zone " + number,
          dimension: "occupancy",
          objectIndex: 10 + index,
          objectType: "People",
          objectName: "People " + number,
          scheduleName: "Schedule " + number,
          schedulePattern: "Week " + number,
          scheduleHash: "hash-" + number,
          rawMethod: "People/Area",
          rawValue: String(value),
          normalized: [{ id: "people_per_area", label: "People per Area", unit: "person/m2", value, displayValue: String(value), status: "ok" }],
          warnings: [],
        }];
        if (number < 4) {
          const lightingValue = number === 1 ? 1 : 10;
          items.push({
            id: "lighting-item-" + number,
            zoneName: "Zone " + number,
            dimension: "lighting",
            objectIndex: 20 + index,
            objectType: "Lights",
            objectName: "Lights " + number,
            scheduleName: "Lighting Schedule " + number,
            schedulePattern: "Lighting Week " + number,
            scheduleHash: "lighting-hash-" + number,
            rawMethod: "Watts/Area",
            rawValue: String(lightingValue),
            normalized: [{ id: "power_per_area", label: "Lighting power per area", unit: "W/m2", value: lightingValue, displayValue: String(lightingValue), status: "ok" }],
            warnings: [],
          });
        }
        return {
          zoneName: "Zone " + number,
          zoneObjectIndex: index,
          items,
          dimensions: [],
          warnings: [],
        };
      });
      const groupSeries = Array.from({ length: 4 }, (_, index) => {
        const number = index + 1;
        return {
          id: "group-series-" + number,
          label: "Profile " + String.fromCharCode(64 + number),
          scopeType: "group",
          groupId: "report-group-" + number,
          groupName: "Profile " + String.fromCharCode(64 + number),
          dimension: "occupancy",
          dimensionLabel: "Occupancy",
          metricId: "people_per_area",
          metricLabel: "People per Area",
          unit: "person/m2",
          designValue: number,
          scheduleName: "Schedule " + number,
          scheduleHash: "hash-" + number,
          schedulePattern: "Week " + number,
          dayMultiplierProfile: constantValues(72, 0.1 * number, 0.2 * number),
          weekMultiplierProfile: constantValues(168, 0.1 * number, 0.2 * number),
          monthMultiplierProfile: constantValues(12, 0.1 * number, 0.2 * number),
          annualMultiplierProfile: constantValues(8760, 0.1 * number, 0.2 * number),
          durationMultiplierProfile: constantValues(8760, 0.2 * number, 0.1 * number),
          sourceItemIds: ["item-" + number],
          warnings: [],
        };
      });
      const zoneLows = [0.05, 0.2, 0.4, 0.1];
      const zoneHighs = [0.3, 0.8, 0.6, 0.9];
      const zoneSeries = Array.from({ length: 4 }, (_, index) => {
        const number = index + 1;
        return {
          id: "zone-series-" + number,
          label: "Zone " + number,
          scopeType: "zone",
          zoneName: "Zone " + number,
          dimension: "occupancy",
          dimensionLabel: "Occupancy",
          metricId: "people_per_area",
          metricLabel: "People per Area",
          unit: "person/m2",
          designValue: number + 0.5,
          scheduleName: "Schedule " + number,
          scheduleHash: "hash-" + number,
          schedulePattern: "Week " + number,
          dayMultiplierProfile: constantValues(72, zoneLows[index], zoneHighs[index]),
          weekMultiplierProfile: constantValues(168, zoneLows[index], zoneHighs[index]),
          monthMultiplierProfile: constantValues(12, zoneLows[index], zoneHighs[index]),
          annualMultiplierProfile: constantValues(8760, zoneLows[index], zoneHighs[index]),
          durationMultiplierProfile: constantValues(8760, zoneHighs[index], zoneLows[index]),
          sourceItemIds: ["item-" + number],
          status: number === 1 ? "partial" : "ok",
          warnings: number === 1 ? [{
            severity: "warning",
            code: "nominal_outdoor_air_profile",
            message: "Outdoor air curve uses nominal design occupancy, not simulated operation.",
          }] : [],
        };
      });
      const reportGroups = zones.map((zone, index) => ({
        id: "report-group-" + (index + 1),
        zoneNames: [zone.zoneName],
        itemIds: zone.items.map((item) => item.id),
      }));
      return {
        zoneCount: zones.length,
        itemCount: zones.reduce((count, zone) => count + zone.items.length, 0),
        dimensions: [{ id: "occupancy", label: "Occupancy" }, { id: "lighting", label: "Lighting" }],
        zoneProfiles: zones,
        groups: reportGroups,
        matrix: [],
        schedules: [],
        warnings: [],
        defaultSettings: {
          enabledDimensions: ["occupancy", "lighting"],
          displayMetrics: { occupancy: "people_per_area", lighting: "power_per_area" },
          groupingMetrics: { occupancy: "people_per_area", lighting: "power_per_area" },
          numericTolerance: 0.001,
          scheduleCompareMode: "name",
          timeView: "week",
          scaleMode: "shared",
          applyBehavior: { defaultMode: "clone", replaceExistingPolicy: "replace" },
        },
        graphDataset: { series: [...groupSeries, ...zoneSeries] },
        parameterCandidates: [{
          id: "candidate-1",
          label: "Occupancy candidate",
          reason: "Fixture candidate",
          dimension: "occupancy",
          zoneNames: ["Zone 1"],
          currentMin: 0.1,
          currentMax: 0.4,
          severity: "info",
        }],
      };
    }

    function semanticNavigationFixture(profile) {
      const entities = profile.groups.map((group, index) => ({
        id: "entity-" + group.id,
        kind: "profile-group",
        sourceAnchors: [{ objectId: "object-" + (index + 1), objectIndex: 10 + index }],
        viewTargets: [{ view: "profile", targetKind: "profile-group", targetId: group.id }],
      }));
      const occurrences = profile.groups.map((group, index) => ({
        occurrenceId: "occurrence-" + group.id,
        entityId: "entity-" + group.id,
        contextKind: "zone_profile",
        path: "profile/" + group.id,
        sourceAnchor: { objectId: "object-" + (index + 1), objectIndex: 10 + index },
        viewTargets: [{ view: "profile", targetKind: "profile-group", targetId: group.id }],
      }));
      return {
        entities,
        occurrences,
        byViewTarget: Object.fromEntries(profile.groups.map((group) => [
          "profile|" + group.id,
          ["occurrence-" + group.id],
        ])),
      };
    }

    try {
      localStorage.clear();
	  const [{ state }, profileViews, navigationAdapters, panelRegistry, selectionController, i18n, settingsClient] = await Promise.all([
        import("/src/js/state.js"),
        import("/src/js/views/profile-views.js"),
        import("/src/js/panel-navigation-adapters.js"),
		import("/src/js/panel-navigation-registry.js"),
        import("/src/js/selection-controller.js"),
        import("/src/js/i18n.js"),
        import("/src/js/settings-client.js"),
      ]);
      i18n.setLanguage("es");
      const profile = profileFixture();
      state.report = { profile };
      state.analysisKey = "profile-layout-selection-fixture";
      state.reportAnalysisKey = state.analysisKey;
      state.activeResultTab = "profile";
      state.semanticProjection = { navigation: semanticNavigationFixture(profile) };
      state.navigationUndoStack = [];
      state.navigationRedoStack = [];
      state.profileSettings = null;
      state.profileViewCache = new Map();
      state.activeProfileView = "profile";
      state.activeProfileGroupId = "";
      state.activeProfileZoneName = "";
      state.profileSelectedGroupIds = [];
      state.profileSelectedZoneNames = [];
      state.profileSelectedDimensions = ["occupancy"];
      state.profileSelectionAnchorKey = "";
      const semanticEvents = [];
      const semanticFollowAttempts = [];
      navigationAdapters.initializeResultPanelNavigationAdapters();
      selectionController.configureSelectionController({
        state,
        getNavigationIndex: () => state.semanticProjection?.navigation || {},
        getActiveInputView: () => "input-semantic",
        getActivePanelView: () => "profile",
        onSelectionChange: ({ selection, options }) => semanticEvents.push({ selection, options }),
        onTemporaryReveal: (detail) => semanticFollowAttempts.push(detail),
        queueAnalysisTarget: (detail) => semanticFollowAttempts.push(detail),
        openView: (view) => semanticFollowAttempts.push(view),
      });
      profileViews.initializeProfileControls();
      profileViews.renderProfile(profile);
      await nextPaint();
      const narrowViewport = window.innerWidth <= 600;
      if (narrowViewport) {
        document.getElementById("profilePane").style.width = "360px";
        await nextPaint();
      }
	  const secondaryProfileUIAbsent = !document.querySelector([
		"#profilePane .profile-section",
		"#profileMatrixStats",
		"#profileMatrix",
		"#profileDetail",
		".profile-matrix",
		".profile-detail-panel",
		".profile-item-table",
		".profile-source-accordion",
		".profile-candidate-panel",
		"[data-profile-candidate-id]",
	  ].join(", "));
	  assert(secondaryProfileUIAbsent, "removed Profile Matrix/Detail/Source/Candidate UI was rendered");

      const rows = () => [...document.querySelectorAll("#profileOverview [data-profile-row-key]")];
      const selectedRows = () => rows().filter((row) => row.getAttribute("aria-selected") === "true");
      const overviewDimensionIDs = [...profile.defaultSettings.enabledDimensions];
      const overviewDimensionLabels = overviewDimensionIDs.map((dimension) => i18n.profileDimensionLabel(dimension));
      const localizedZoneCounts = new Set(Array.from({ length: profile.zoneCount }, (_, index) => (
        i18n.t("count.zones", { count: index + 1 })
      )));
      let expectedProfileNameByZone = {};
      const inspectOverviewRows = (expectedView) => {
        const metricIssues = [];
        const assignmentIssues = [];
        rows().forEach((row) => {
          const metrics = row.querySelector(":scope > .profile-card-metrics");
          const metricCells = metrics ? [...metrics.querySelectorAll(":scope > .profile-card-metric")] : [];
          const actualDimensionIDs = metricCells.map((cell) => cell.dataset.profileDimension || "");
          const actualLabels = metricCells.map((cell) => (
            cell.querySelector(":scope > .profile-card-metric-label")?.textContent?.trim() || ""
          ));
          const values = metricCells.map((cell) => (
            cell.querySelector(":scope > .profile-card-metric-value")?.textContent?.trim() || ""
          ));
          const directMetricText = metrics
            ? [...metrics.childNodes].filter((node) => node.nodeType === Node.TEXT_NODE && node.textContent.trim())
            : [];
          const metricChecks = {
            metrics: Boolean(metrics),
            cellCount: metricCells.length === overviewDimensionIDs.length,
            dimensionOrder: JSON.stringify(actualDimensionIDs) === JSON.stringify(overviewDimensionIDs),
            labelOrder: JSON.stringify(actualLabels) === JSON.stringify(overviewDimensionLabels),
            values: values.every(Boolean),
            separateCells: metricCells.every((cell) => (
              cell.querySelectorAll(":scope > .profile-card-metric-label").length === 1
              && cell.querySelectorAll(":scope > .profile-card-metric-value").length === 1
            )),
            noJoinedText: directMetricText.length === 0,
          };
          if (!Object.values(metricChecks).every(Boolean)) {
            metricIssues.push({
              rowKey: row.dataset.profileRowKey || "",
              actualDimensionIDs,
              actualLabels,
              values,
              directMetricText: directMetricText.map((node) => node.textContent.trim()),
              checks: metricChecks,
            });
          }

          const assignment = row.querySelector(":scope > .profile-card-assignment");
          if (expectedView === "zone") {
            const visibleProfileName = assignment?.textContent?.trim() || "";
            const expectedProfileName = expectedProfileNameByZone[row.dataset.profileZone] || "";
            const rowCells = [...row.children];
            const zoneAssignmentChecks = {
              assignment: Boolean(assignment?.matches(".profile-card-profile-name")),
              profileNameOnly: Boolean(expectedProfileName) && visibleProfileName === expectedProfileName,
              columnOrder: rowCells.length === 4
                && rowCells[1] === assignment
                && rowCells[2] === metrics
                && rowCells[3].matches(".profile-row-apply-slot"),
              profileNameTooltip: !assignment?.hasAttribute("title")
                || assignment.getAttribute("title")?.trim() === visibleProfileName,
              noReceivesProfileSentence: visibleProfileName
                !== i18n.t("profile.receivesProfile", { profile: expectedProfileName }),
              visibleTextOnly: assignment?.children.length === 0,
            };
            if (!Object.values(zoneAssignmentChecks).every(Boolean)) {
              assignmentIssues.push({
                rowKey: row.dataset.profileRowKey || "",
                visibleProfileName,
                expectedProfileName,
                checks: zoneAssignmentChecks,
              });
            }
            return;
          }
          const visible = assignment?.textContent?.trim() || "";
          const detail = assignment?.getAttribute("title")?.trim() || "";
          const accessibleLabel = assignment?.getAttribute("aria-label")?.trim() || "";
          const assignmentChecks = {
            assignment: Boolean(assignment?.matches("[title]")),
            localizedCountOnly: localizedZoneCounts.has(visible),
            fullHoverDetail: Boolean(detail) && detail !== visible,
            accessibleDetail: Boolean(accessibleLabel)
              && accessibleLabel.startsWith(visible)
              && accessibleLabel.includes(detail),
            visibleTextOnly: assignment?.children.length === 0,
          };
          if (!Object.values(assignmentChecks).every(Boolean)) {
            assignmentIssues.push({
              rowKey: row.dataset.profileRowKey || "",
              visible,
              detail,
              accessibleLabel,
              checks: assignmentChecks,
            });
          }
        });
        return {
          metricCells: rows().length > 0 && metricIssues.length === 0,
          assignments: rows().length > 0 && assignmentIssues.length === 0,
          metricIssues,
          assignmentIssues,
        };
      };
      const profileOverviewPresentation = inspectOverviewRows("profile");
      expectedProfileNameByZone = Object.fromEntries(rows().flatMap((row) => {
        const profileName = row.querySelector(":scope > span:first-child strong")?.textContent?.trim() || "";
        const zoneNames = (row.querySelector(":scope > .profile-card-assignment[title]")?.getAttribute("title") || "")
          .split("\n")
          .map((zoneName) => zoneName.trim())
          .filter(Boolean);
        return zoneNames.map((zoneName) => [zoneName, profileName]);
      }));
      const graphViewButtons = () => [...document.querySelectorAll("#profileGraph [data-profile-time-view]")];
      const graphViewButton = (view) => document.querySelector('#profileGraph [data-profile-time-view="' + view + '"]');
      const clickGraphView = async (view) => {
        const button = graphViewButton(view);
        assert(button, "missing direct Profile Graph View button " + view);
        button.click();
        await nextPaint();
        return document.activeElement === graphViewButton(view);
      };
      const fidelityReason = "Outdoor air curve uses nominal design occupancy, not simulated operation.";
      const inspectFidelityStatus = () => {
        const badge = document.querySelector("#profileGraph .profile-graph-panel-head .profile-fidelity-badge");
        const legend = document.querySelector('#profileGraph .profile-overlay-legend [data-profile-fidelity="nominal"]');
        return Boolean(badge)
          && badge.dataset.profileFidelity === "nominal"
          && badge.textContent.trim() === i18n.t("profile.fidelityNominal", {}, "Nominal")
          && badge.getAttribute("role") === "status"
          && badge.getAttribute("title")?.includes(fidelityReason)
          && badge.getAttribute("aria-label")?.includes(fidelityReason)
          && legend?.getAttribute("title")?.includes(fidelityReason)
          && legend?.getAttribute("aria-label")?.includes(fidelityReason);
      };
      const overlayFidelityStatus = inspectFidelityStatus();
      await clickGraphView("year");
      const annualFidelityStatus = inspectFidelityStatus()
        && Boolean(document.querySelector("#profileGraph .profile-annual-panel .profile-fidelity-badge"));
      await clickGraphView("week");
      const graphFidelityStatusVisible = overlayFidelityStatus && annualFidelityStatus;
      assert(graphFidelityStatusVisible, "nominal/partial Profile graph fidelity was hidden or lacked an accessible reason");
      const visibleElements = (root, selector) => [...(root?.querySelectorAll(selector) || [])]
        .filter((element) => getComputedStyle(element).display !== "none"
          && getComputedStyle(element).visibility !== "hidden"
          && element.getClientRects().length > 0);
      const elementTexts = (elements) => elements.map((element) => element.textContent.trim());
      const expectedOverlayAxes = {
        day: {
          title: i18n.t("profile.axisDayTypeHour", {}, "Day type / hour [h]"),
          ticks: ["WD 00", "WD 12", "Sat 00", "Sat 12", "Sun 00", "Sun 12", "Sun 24"],
        },
        week: {
          title: i18n.t("profile.axisDayOfWeek", {}, "Day of week"),
          ticks: ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"],
        },
        month: {
          title: i18n.t("profile.axisMonth", {}, "Month"),
          ticks: ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"],
        },
        duration: {
          title: i18n.t("profile.axisAnnualHoursExceeded", {}, "Annual hours exceeded [%]"),
          ticks: ["0%", "25%", "50%", "75%", "100%"],
        },
      };
      const inspectGraphAxisView = (view) => {
        const graph = document.getElementById("profileGraph");
        const graphHasMultiplierFallback = /\bMultiplier\b/.test(graph.textContent || "");
        if (view === "year") {
          const frame = graph.querySelector(".profile-heatmap-frame");
          const monthTicks = [...(frame?.querySelectorAll(".profile-heatmap-x-ticks .profile-axis-tick") || [])];
          const hourTicks = [...(frame?.querySelectorAll(".profile-heatmap-y-ticks .profile-axis-tick") || [])];
          const scale = frame?.querySelector(".profile-heatmap-scale");
          const scaleTicks = [...(scale?.querySelectorAll(".profile-heatmap-scale-ticks > span") || [])];
          const expectedMonths = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
          const expectedHours = ["00", "06", "12", "18", "24"];
          const checks = {
            frame: Boolean(frame),
            noOverlay: !graph.querySelector(".profile-line-chart, .profile-overlay-graph"),
            monthTitle: frame?.querySelector(".profile-heatmap-x-title")?.textContent.trim()
              === i18n.t("profile.axisMonth", {}, "Month"),
            hourTitle: frame?.querySelector(".profile-heatmap-y-title")?.textContent.trim()
              === i18n.t("profile.axisHourOfDay", {}, "Hour of day [h]"),
            monthTicks: JSON.stringify(elementTexts(monthTicks)) === JSON.stringify(expectedMonths),
            hourTicks: JSON.stringify(elementTexts(hourTicks)) === JSON.stringify(expectedHours),
            allMonthTicksVisible: visibleElements(frame, ".profile-heatmap-x-ticks .profile-axis-tick").length === expectedMonths.length,
            allHourTicksVisible: visibleElements(frame, ".profile-heatmap-y-ticks .profile-axis-tick").length === expectedHours.length,
            valueScaleSeparated: Boolean(scale)
              && scale !== frame?.querySelector(".profile-heatmap-x-ticks")
              && scale !== frame?.querySelector(".profile-heatmap-y-ticks")
              && scaleTicks.length === 3
              && scale.querySelector(".profile-heatmap-scale-bar")
              && scale.querySelector(".profile-heatmap-scale-title")?.textContent.includes("person/m2")
              && elementTexts(monthTicks).every((label) => !elementTexts(scaleTicks).includes(label))
              && elementTexts(hourTicks).every((label) => !elementTexts(scaleTicks).includes(label)),
            noMultiplierFallback: !graphHasMultiplierFallback,
          };
          return { view, passed: Object.values(checks).every(Boolean), checks };
        }
        const expected = expectedOverlayAxes[view];
        const chart = graph.querySelector(".profile-line-chart");
        const svg = chart?.querySelector("svg.profile-overlay-graph");
        const xTicks = [...(chart?.querySelectorAll(".profile-x-axis-ticks .profile-axis-tick") || [])];
        const yTicks = [...(chart?.querySelectorAll(".profile-y-axis-ticks .profile-axis-tick") || [])];
        const checks = {
          chart: Boolean(chart),
          noHeatmap: !graph.querySelector(".profile-heatmap-frame"),
          xTitle: chart?.querySelector(".profile-x-axis-title")?.textContent.trim() === expected.title,
          xTicks: JSON.stringify(elementTexts(xTicks)) === JSON.stringify(expected.ticks),
          allXTicksVisible: visibleElements(chart, ".profile-x-axis-ticks .profile-axis-tick").length === expected.ticks.length,
          yTitle: chart?.querySelector(".profile-y-axis-title")?.textContent.includes("person/m2"),
          yTicks: yTicks.length >= 3 && visibleElements(chart, ".profile-y-axis-ticks .profile-axis-tick").length === yTicks.length,
          preserveAspectRatioNone: svg?.getAttribute("preserveAspectRatio") === "none",
          htmlAxes: !svg?.querySelector("text")
            && Boolean(chart?.querySelector(".profile-x-axis-ticks"))
            && Boolean(chart?.querySelector(".profile-y-axis-ticks")),
          noMultiplierFallback: !graphHasMultiplierFallback,
        };
        return { view, passed: Object.values(checks).every(Boolean), checks };
      };
      const clickRow = async (index, modifiers = {}) => {
        const row = rows()[index];
        assert(row, "missing Profile row " + index);
        row.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, ...modifiers }));
        await nextPaint();
      };
      const activeRow = () => rows().find((row) => row.classList.contains("active"));
      const applySource = () => {
        const applyButton = document.getElementById("profileApplyButton");
        assert(!applyButton.disabled, "Apply Profile unexpectedly disabled for the primary row");
        applyButton.click();
        const source = document.querySelector("#profileApplyBody > section:first-child h4")?.textContent?.trim() || "";
        document.getElementById("profileApplyClose").click();
        return source;
      };
      const expectedGraphViews = ["day", "week", "month", "year", "duration"];
      const graphViewGroup = document.querySelector("#profileGraph .profile-graph-view-switch");
      const fixedTimeProfileControls = Boolean(graphViewGroup)
        && graphViewGroup.getAttribute("role") === "group"
        && JSON.stringify(graphViewButtons().map((button) => button.dataset.profileTimeView)) === JSON.stringify(expectedGraphViews)
        && graphViewButtons().every((button) => button.tagName === "BUTTON"
          && button.getAttribute("type") === "button"
          && ["true", "false"].includes(button.getAttribute("aria-pressed")))
        && graphViewButtons().filter((button) => button.getAttribute("aria-pressed") === "true").length === 1
        && !document.querySelector("#profileGraphPreset, #profileGraphScopeType, #profileGraphCompareMode, #profileGraphTimeView");
      const scaleRemovedFromProfileTab = !document.querySelector("#profilePane #profileGraphScaleMode, #profilePane #profileScaleMode");
      const legacyViewMigration = [
        ["representative_day", "day"],
        ["representative_week", "week"],
        ["monthly_average", "month"],
        ["hourly_average_by_daytype", "week"],
        ["load_duration", "duration"],
        ["annual_heatmap", "year"],
      ].every(([legacy, expected]) => (
        settingsClient.mergeSettings({ profile: { scheduleSummaryMode: legacy } }).profile.timeView === expected
      ))
        && settingsClient.mergeSettings({ profile: { timeView: "rules" } }).profile.timeView === "year"
        && settingsClient.mergeSettings({ profile: { scheduleSummaryMode: "period_rules" } }).profile.timeView === "year"
        && !("metricMode" in settingsClient.mergeSettings({ profile: { metricMode: "design", graphMode: "multiplier" } }).profile);
      assert(fixedTimeProfileControls, "Profile Graph still exposes removed controls or lacks the five direct View buttons");
      assert(scaleRemovedFromProfileTab, "Profile tab still exposes the Scale selector that belongs in app Settings");
      assert(legacyViewMigration, "legacy Profile schedule-summary settings did not migrate to View without overriding an explicit View");
      let viewFocusRestored = await clickGraphView("week");

      state.navigationUndoStack = [];
      semanticEvents.length = 0;
      semanticFollowAttempts.length = 0;
      const firstSemanticEntity = rows()[1].dataset.entityId || "";
      assert(firstSemanticEntity, "semantic-capable Profile fixture did not annotate its rows");
      await clickRow(1);
      await nextPaint();
      const rowGestureHistoryOnce = state.navigationUndoStack.length === 1;
      assert(rowGestureHistoryOnce, "one Profile row gesture did not record exactly one undo snapshot");
      const firstSemanticSync = state.globalSelection.entityId === firstSemanticEntity
        && semanticEvents.at(-1)?.selection?.entityId === firstSemanticEntity
        && semanticEvents.at(-1)?.options?.follow === true
        && semanticFollowAttempts.length === 1;

      await clickRow(3, { ctrlKey: true });
      const retainedPrimaryID = state.activeProfileGroupId;
      const retainedPrimaryRow = activeRow();
      const retainedPrimaryLabel = retainedPrimaryRow?.querySelector("strong")?.textContent?.trim() || "";
      const retainedPrimaryEntity = retainedPrimaryRow?.dataset.entityId || "";
      await clickRow(1, { ctrlKey: true });
      const nonPrimaryRemovalKeepsSource = state.activeProfileGroupId === retainedPrimaryID
        && activeRow()?.dataset.entityId === retainedPrimaryEntity
        && applySource() === retainedPrimaryLabel;
      const nonPrimarySemanticSync = state.globalSelection.entityId === retainedPrimaryEntity;

      await clickRow(0, { ctrlKey: true });
      const removedPrimaryID = state.activeProfileGroupId;
      await clickRow(0, { ctrlKey: true });
      const replacementPrimaryRow = activeRow();
      const primaryRemovalReplacesSource = removedPrimaryID !== state.activeProfileGroupId
        && state.activeProfileGroupId === retainedPrimaryID
        && applySource() === replacementPrimaryRow?.querySelector("strong")?.textContent?.trim();
      const primaryRemovalSemanticSync = state.globalSelection.entityId === replacementPrimaryRow?.dataset.entityId;
      const primaryApplySourceBoundary = nonPrimaryRemovalKeepsSource && primaryRemovalReplacesSource;
      const rowGestureSemanticPrimary = firstSemanticSync
        && nonPrimarySemanticSync
        && primaryRemovalSemanticSync
        && semanticFollowAttempts.length >= 1;
      assert(primaryApplySourceBoundary, "Ctrl removal changed Apply source before the primary row was removed");
      assert(rowGestureSemanticPrimary, "row gesture did not synchronize and follow the global semantic primary");

      await clickRow(1);
      const singleSelection = state.profileSelectedGroupIds.length === 1 && selectedRows().length === 1;
      assert(singleSelection, "plain click did not replace Profile row selection");
      const singlePath = document.querySelector(".profile-overlay-paths path");
      const singleLegend = document.querySelector(".profile-overlay-legend [data-profile-series-id]");
      const singleSelectionLegend = document.querySelectorAll(".profile-overlay-paths path").length === 1
        && document.querySelectorAll(".profile-overlay-legend [data-profile-series-id]").length === 1
        && singlePath?.dataset.profileSeriesId === singleLegend?.dataset.profileSeriesId
        && getComputedStyle(singlePath).stroke === getComputedStyle(singleLegend.querySelector("i")).backgroundColor;
      assert(singleSelectionLegend, "single Profile selection did not keep one matching line and legend entry");

      const actualYAxisTicks = [...document.querySelectorAll("#profileGraph .profile-y-axis-ticks .profile-axis-tick")]
        .map((tick) => Number((tick.textContent || "").replaceAll(",", "")))
        .filter(Number.isFinite);
      const actualEngineeringValue = document.querySelector("#profileGraph .profile-y-axis-title")?.textContent.trim()
          === i18n.t("profile.axisValue", { unit: "person/m2" }, "Value [{unit}]")
        && Math.abs(Math.max(...actualYAxisTicks) - 2) < 1e-9
        && !document.querySelector("#profileMetricMode, [data-profile-metric-mode]")
        && !(document.getElementById("profileGraph")?.textContent || "").includes("Schedule fraction");
      assert(actualEngineeringValue, "Profile Graph did not stay fixed to design-value-scaled engineering values");

      await clickRow(3, { ctrlKey: true });
      const ctrlToggle = state.profileSelectedGroupIds.length === 2 && selectedRows().length === 2;
      assert(ctrlToggle, "Ctrl click did not toggle a non-contiguous Profile row");
      await clickRow(3, { ctrlKey: true });
      const ctrlToggleRemoval = state.profileSelectedGroupIds.length === 1 && selectedRows().length === 1;
      assert(ctrlToggleRemoval, "Ctrl click did not remove an already-selected Profile row");

      await clickRow(0);
      await clickRow(3, { ctrlKey: true });
      await clickRow(1, { ctrlKey: true, shiftKey: true });
      const ctrlShiftRange = state.profileSelectedGroupIds.length === 4 && selectedRows().length === 4;
      assert(ctrlShiftRange, "Ctrl+Shift did not add the anchor range to the Profile selection");

      await clickRow(0);
      await clickRow(2, { shiftKey: true });
      const shiftRange = state.profileSelectedGroupIds.length === 3 && selectedRows().length === 3;
      assert(shiftRange, "Shift click did not select the contiguous anchor range");
      const profileModeRows = rows().length === 4 && rows().every((row) => row.dataset.profileRowKey.startsWith("group:"));
      const profileSelectedIDs = [...state.profileSelectedGroupIds];
	  const multiplierProfileFields = [
		"dayMultiplierProfile",
		"weekMultiplierProfile",
		"monthMultiplierProfile",
		"annualMultiplierProfile",
		"durationMultiplierProfile",
	  ];
	  const identicalCurveSeries = profile.graphDataset.series
		.filter((series) => series.scopeType === "zone" && ["Zone 1", "Zone 2", "Zone 3"].includes(series.zoneName));
	  const originalCurveSeries = identicalCurveSeries.map((series) => ({
		series,
		designValue: series.designValue,
		profiles: Object.fromEntries(multiplierProfileFields.map((field) => [field, [...series[field]]])),
	  }));
	  const identicalCurveSource = identicalCurveSeries[0];
	  identicalCurveSeries.forEach((series) => {
		series.designValue = identicalCurveSource.designValue;
		multiplierProfileFields.forEach((field) => {
		  series[field] = [...identicalCurveSource[field]];
		});
	  });
	  profileViews.renderProfile(profile);
	  await nextPaint();

      let everyViewApplied = true;
      const graphAxisResults = [];
      for (const view of expectedGraphViews) {
        viewFocusRestored = (await clickGraphView(view)) && viewFocusRestored;
        everyViewApplied = everyViewApplied
          && state.profileSettings.timeView === view
          && graphViewButton(view)?.getAttribute("aria-pressed") === "true";
        graphAxisResults.push(inspectGraphAxisView(view));
      }
      const fiveViewAxes = graphAxisResults.length === expectedGraphViews.length
        && graphAxisResults.every((result) => result.passed);
      const noVisibleGraphMultiplier = graphAxisResults.every((result) => result.checks.noMultiplierFallback);
      assert(fiveViewAxes, "Profile Graph axes are incomplete or semantically mixed across five Views: "
        + JSON.stringify(graphAxisResults));
      assert(noVisibleGraphMultiplier, "Profile Graph exposed a Multiplier unit/header fallback");
      viewFocusRestored = (await clickGraphView("week")) && viewFocusRestored;
      const viewToggleImmediate = everyViewApplied
        && state.profileSettings.timeView === "week"
        && graphViewButton("week")?.getAttribute("aria-pressed") === "true"
        && graphViewButtons().filter((button) => button.getAttribute("aria-pressed") === "true").length === 1;
      assert(viewToggleImmediate, "direct Profile Graph View buttons did not update the graph immediately");
      assert(viewFocusRestored, "Profile Graph View focus was not restored after rerender");
      const overlayPaths = [...document.querySelectorAll(".profile-overlay-paths path")];
      const overlayLabels = [...document.querySelectorAll(".profile-overlay-legend [data-profile-series-id]")];
      const overlayLegendAlways = overlayPaths.length === 3 && overlayLabels.length === overlayPaths.length;
      assert(overlayLegendAlways, "line overlay/legend did not track every selected row");
	  const overlapPathBySeriesID = new Map(overlayPaths.map((path) => [path.dataset.profileSeriesId, path]));
	  const overlapPathIndices = overlayPaths
		.map((path) => Number(path.dataset.profileOverlapIndex))
		.sort((left, right) => left - right);
	  const identicalCurveOverlay = overlayPaths.length === 3
		&& new Set(overlayPaths.map((path) => path.getAttribute("d"))).size === 1
		&& overlayPaths.every((path) => path.dataset.profileOverlapCount === "3")
		&& JSON.stringify(overlapPathIndices) === JSON.stringify([0, 1, 2])
		&& new Set(overlayPaths.map((path) => getComputedStyle(path).stroke)).size === 3
		&& new Set(overlayPaths.map((path) => path.getAttribute("stroke-dasharray"))).size === 1
		&& overlayPaths.every((path) => Boolean(path.getAttribute("stroke-dasharray")))
		&& new Set(overlayPaths.map((path) => path.getAttribute("stroke-dashoffset"))).size === 3
		&& overlayPaths.every((path) => !path.hasAttribute("transform")
		  && getComputedStyle(path).transform === "none"
		  && !path.closest(".profile-overlay-paths")?.hasAttribute("transform"));
	  assert(identicalCurveOverlay, "identical Profile curves were offset, jittered, or painted without interleaved colors");

	  const identicalCurveLegend = overlayLabels.length === 3 && overlayLabels.every((label) => {
		const matchingPath = overlapPathBySeriesID.get(label.dataset.profileSeriesId);
		const swatch = label.querySelector(".profile-line-swatch");
		const swatchRect = swatch?.getBoundingClientRect();
		const swatchStyle = swatch ? getComputedStyle(swatch) : null;
		const title = label.getAttribute("title") || "";
		const ariaLabel = label.getAttribute("aria-label") || "";
		return Boolean(matchingPath && swatch && swatchRect && swatchStyle)
		  && label.dataset.profileOverlapCount === matchingPath.dataset.profileOverlapCount
		  && label.dataset.profileOverlapIndex === matchingPath.dataset.profileOverlapIndex
		  && swatch.style.getPropertyValue("--profile-overlap-period").trim()
			=== (6 * Number(matchingPath.dataset.profileOverlapCount)) + "px"
		  && swatch.style.getPropertyValue("--profile-overlap-offset").trim()
			=== (-6 * Number(matchingPath.dataset.profileOverlapIndex)) + "px"
		  && title.includes("3")
		  && ariaLabel === title
		  && ariaLabel.includes(label.querySelector(".profile-line-label")?.textContent.trim() || "")
		  && swatch.classList.contains("is-overlap")
		  && swatchRect.width >= 14
		  && swatchRect.height > 0
		  && swatchRect.height <= 4
		  && swatchStyle.flexShrink === "0";
	  });
	  assert(identicalCurveLegend, "shared-curve legend metadata, accessibility text, or line swatches do not match the paths");

	  const semanticLegend = overlayLabels[0];
	  const analysisSurface = document.getElementById("profilePane");
	  const retainedSemanticRelated = semanticLegend.classList.contains("semantic-related");
	  analysisSurface.classList.add("analysis-panel");
	  semanticLegend.classList.add("semantic-related");
	  const semanticLegendStyle = getComputedStyle(semanticLegend);
	  const semanticLegendOutline = semanticLegendStyle.boxShadow === "none"
		&& semanticLegendStyle.outlineStyle !== "none"
		&& parseFloat(semanticLegendStyle.outlineWidth) >= 1;
	  if (!retainedSemanticRelated) semanticLegend.classList.remove("semantic-related");
	  analysisSurface.classList.remove("analysis-panel");
	  assert(semanticLegendOutline, "semantic-related Profile legend still uses an inset bar instead of an outline");

      viewFocusRestored = (await clickGraphView("year")) && viewFocusRestored;
      const heatmaps = [...document.querySelectorAll(".profile-annual-heatmap-grid .profile-heatmap-frame")];
      const heatmapLefts = new Set(heatmaps.map((heatmap) => Math.round(heatmap.getBoundingClientRect().left)));
      const annualHeatmapsParallel = heatmaps.length === 3
        && (narrowViewport || heatmapLefts.size > 1)
        && !document.querySelector(".profile-overlay-graph");
      assert(annualHeatmapsParallel, "annual selections were not rendered as parallel heatmaps");
      const annualCanvases = [...document.querySelectorAll(".profile-annual-heatmap-grid canvas.profile-heatmap")];
      const annualCanvasPainted = annualCanvases.length === 3 && annualCanvases.every((canvas) => {
        const pixels = canvas.getContext("2d").getImageData(0, 0, canvas.width, canvas.height).data;
        let opaquePixels = 0;
        const colors = new Set();
        for (let index = 0; index < pixels.length; index += 4) {
          if (pixels[index + 3] > 0) {
            opaquePixels += 1;
            colors.add(pixels[index] + "," + pixels[index + 1] + "," + pixels[index + 2] + "," + pixels[index + 3]);
          }
        }
        return opaquePixels === canvas.width * canvas.height && colors.size >= 2;
      });
      assert(annualCanvasPainted, "annual heatmap canvases did not paint all pixels with meaningful values");
      const annualOverflowSurfaces = [
        document.getElementById("profileGraph"),
        ...document.querySelectorAll(".profile-annual-stack, .profile-annual-panel, .profile-annual-heatmap-grid, .profile-annual-heatmap-card, .profile-heatmap-frame, canvas.profile-heatmap"),
      ];
      const annualNoHorizontalOverflow = annualOverflowSurfaces
        .every((element) => element.scrollWidth <= element.clientWidth + 1);
      assert(annualNoHorizontalOverflow, "annual heatmaps create horizontal overflow");
	  originalCurveSeries.forEach(({ series, designValue, profiles }) => {
		series.designValue = designValue;
		multiplierProfileFields.forEach((field) => {
		  series[field] = profiles[field];
		});
	  });
	  profileViews.renderProfile(profile);
	  await nextPaint();

      document.querySelector('[data-profile-view="zone"]').click();
      await nextPaint();
      const zoneModeRows = state.activeProfileView === "zone"
        && rows().length === 4
        && rows().every((row) => row.dataset.profileRowKey.startsWith("zone:"));
      const zoneOverviewPresentation = inspectOverviewRows("zone");
      const missingLightingCell = document.querySelector(
        '#profileOverview [data-profile-zone="Zone 4"] .profile-card-metric[data-profile-dimension="lighting"]',
      );
      const missingMetricPlaceholder = missingLightingCell?.classList.contains("is-missing")
        && missingLightingCell?.classList.contains("is-not-configured")
        && missingLightingCell?.dataset.profileMetricStatus === "not-configured"
        && missingLightingCell.querySelector(":scope > .profile-card-metric-value")?.textContent?.trim()
          === "—"
        && missingLightingCell.getAttribute("aria-label")?.includes(i18n.profileDimensionLabel("lighting"))
        && missingLightingCell.getAttribute("aria-label")?.includes("objects are configured");
      let profileMetricAvailability = false;
      let spaceRatioAggregation = false;
      let sourceMetricCompatibility = false;
      const overviewMetricCells = profileOverviewPresentation.metricCells
        && zoneOverviewPresentation.metricCells
        && missingMetricPlaceholder;
      const assignmentCountWithHoverDetail = profileOverviewPresentation.assignments;
      const zoneProfileNameOnly = zoneOverviewPresentation.assignments;
      assert(overviewMetricCells, "Profile overview metric cells lost fixed order/labels/placeholders: "
        + JSON.stringify({ profileOverviewPresentation, zoneOverviewPresentation, missingMetricPlaceholder }));
      assert(assignmentCountWithHoverDetail, "Profile assignment is not a localized count with hover/accessibility detail: "
        + JSON.stringify(profileOverviewPresentation.assignmentIssues));
      assert(zoneProfileNameOnly, "Zone assignment must show only its Profile name: "
        + JSON.stringify(zoneOverviewPresentation.assignmentIssues));
      const profileZoneModes = profileModeRows && zoneModeRows;
      assert(profileZoneModes, "Profile and Zone views did not expose their distinct row-key scopes");
      await clickRow(0);
      await clickRow(2, { metaKey: true });
      const zoneCtrlSelection = state.profileSelectedZoneNames.length === 2 && selectedRows().length === 2;
      assert(zoneCtrlSelection, "Meta click did not toggle zone-row selection");
      await clickRow(2, { metaKey: true });
      const metaToggleRemoval = state.profileSelectedZoneNames.length === 1 && selectedRows().length === 1;
      assert(metaToggleRemoval, "Meta click did not remove an already-selected Zone row");
      await clickRow(0);
      await clickRow(2, { shiftKey: true });
      const zoneShiftRange = state.profileSelectedZoneNames.length === 3 && selectedRows().length === 3;
      assert(zoneShiftRange, "Shift click did not select the contiguous Zone range");
      const profileZoneSelectionIsolation = JSON.stringify(state.profileSelectedGroupIds) === JSON.stringify(profileSelectedIDs);
      assert(profileZoneSelectionIsolation, "Zone row interactions changed the preserved Profile row selection");
      viewFocusRestored = (await clickGraphView("week")) && viewFocusRestored;
      const zoneOverlayPaths = [...document.querySelectorAll(".profile-overlay-paths path")];
      const zoneOverlayLabels = [...document.querySelectorAll(".profile-overlay-legend [data-profile-series-id]")];
      const zoneProfileOverlaySelection = state.profileSelectedZoneNames.length === 3
        && zoneOverlayPaths.length === 3
        && zoneOverlayLabels.length === 3
        && zoneOverlayPaths.every((path) => path.dataset.profileSeriesId.startsWith("zone-series-"))
        && zoneOverlayLabels.map((label) => label.textContent.trim()).every((label) => /^Zone [123]\b/.test(label));
      assert(zoneProfileOverlaySelection, "Zone row selection did not render one distinct zone-scope overlay per selected Zone");

      const initialSettings = document.getElementById("profileSettings");
      const initialLiveControls = initialSettings.querySelector(".profile-live-controls");
      const initialLiveGroups = [...initialLiveControls.querySelectorAll(":scope > .profile-live-group")];
      const initialInspectSwitch = initialLiveGroups[0]?.querySelector(".profile-view-switch");
      const initialDimensionsRow = initialLiveGroups[1]?.querySelector(".profile-toggle-row");
      const initialControlsRect = initialLiveControls.getBoundingClientRect();
      const initialInspectGroupRect = initialLiveGroups[0]?.getBoundingClientRect();
      const initialDimensionsGroupRect = initialLiveGroups[1]?.getBoundingClientRect();
      const initialInspectSwitchRect = initialInspectSwitch?.getBoundingClientRect();
      const initialDimensionsRowRect = initialDimensionsRow?.getBoundingClientRect();
      const profileControlGroupsAdjacent = narrowViewport || (
        initialLiveGroups.length === 2
        && Math.abs(initialInspectGroupRect.top - initialDimensionsGroupRect.top) <= 1
        && initialInspectGroupRect.width <= initialInspectSwitchRect.width + 2
        && initialDimensionsGroupRect.left >= initialInspectSwitchRect.right
        && initialDimensionsGroupRect.left - initialInspectSwitchRect.right <= 24
        && initialDimensionsGroupRect.left < initialControlsRect.left + initialControlsRect.width * 0.35
      );
      const profileControlGroupsWrapNarrow = !narrowViewport || (
        initialLiveGroups.length === 2
        && initialDimensionsGroupRect.top >= initialInspectGroupRect.bottom + 6
        && initialInspectGroupRect.left >= initialControlsRect.left
        && initialDimensionsGroupRect.left >= initialControlsRect.left
        && initialInspectGroupRect.right <= initialControlsRect.right + 1
        && initialDimensionsGroupRect.right <= initialControlsRect.right + 1
        && initialDimensionsRowRect.right <= initialControlsRect.right + 1
      );
      assert(profileControlGroupsAdjacent, "Profile Dimensions still starts in an equal-width second column");
      assert(profileControlGroupsWrapNarrow, "Profile live-control groups do not wrap safely at narrow widths");

      document.getElementById("profilePane").style.width = narrowViewport ? "360px" : "720px";
      await nextPaint();
      const pane = document.querySelector(".profile-pane");
      const settings = document.getElementById("profileSettings");
      const inspectBySwitch = settings.querySelector(".profile-view-switch");
      const inspectByGroup = inspectBySwitch.closest(".profile-live-group");
      const inspectByButtons = [...inspectBySwitch.querySelectorAll(".profile-segment-button")];
      const inspectByRect = inspectBySwitch.getBoundingClientRect();
      const inspectByGroupRect = inspectByGroup.getBoundingClientRect();
      const inspectByButtonWidth = inspectByButtons.reduce((total, button) => total + button.getBoundingClientRect().width, 0);
      const liveLabels = [...settings.querySelectorAll(".profile-live-label")];
      const liveControlRows = [
        inspectBySwitch,
        settings.querySelector(".profile-toggle-row"),
      ];
      const topRange = (elements) => {
        const tops = elements.map((element) => element.getBoundingClientRect().top);
        return Math.max(...tops) - Math.min(...tops);
      };
      const inspectByCompact = inspectByButtons.length === 2
        && inspectBySwitch.getAttribute("role") === "group"
        && inspectByButtons.filter((button) => button.getAttribute("aria-pressed") === "true").length === 1
        && inspectByRect.width <= inspectByButtonWidth + 20
        && (narrowViewport || inspectByGroupRect.width <= inspectByRect.width + 2)
        && getComputedStyle(inspectBySwitch).justifySelf === "start";
      const profileControlsAligned = liveLabels.length === 2
        && liveControlRows.every(Boolean)
        && (narrowViewport || (topRange(liveLabels) <= 1 && topRange(liveControlRows) <= 1));
      const table = document.querySelector(".profile-overview-table");
      const overview = document.getElementById("profileOverview");
      const graphSection = document.querySelector(".profile-visual");
      const graph = document.getElementById("profileGraph");
      const tableAboveGraph = table.getBoundingClientRect().top < graphSection.getBoundingClientRect().top;
	  const noHorizontalOverflow = [pane, settings, table, overview, graphSection, graph]
        .every((element) => element.scrollWidth <= element.clientWidth + 1);
      const apply = document.getElementById("profileApplyButton");
      const applyHead = apply.closest(".profile-overview-table-head");
      const applyRect = apply.getBoundingClientRect();
      const headRect = applyHead.getBoundingClientRect();
      const radius = parseFloat(getComputedStyle(apply).borderTopLeftRadius) || 0;
      const applyRight = headRect.right - applyRect.right <= 12 && applyRect.left > headRect.left + headRect.width / 2 && radius >= applyRect.height * 0.4;
      const visualTable = document.querySelector(".profile-overview-table");
      const optionRows = rows();
      const domSemantics = overview.getAttribute("role") === "listbox"
        && overview.getAttribute("aria-multiselectable") === "true"
        && optionRows.length === 4
        && optionRows.every((row) => row.tagName === "BUTTON"
          && row.getAttribute("type") === "button"
          && row.getAttribute("role") === "option"
          && ["true", "false"].includes(row.getAttribute("aria-selected")))
        && optionRows.filter((row) => row.getAttribute("aria-selected") === "true").length === state.profileSelectedZoneNames.length
        && !visualTable.matches('[role="table"]')
        && !visualTable.querySelector('[role="table"], [role="row"], [role="columnheader"], [role="rowgroup"]')
        && document.querySelectorAll("#profilePane #profileApplyButton").length === 1
        && apply.getAttribute("type") === "button";
      assert(tableAboveGraph, "Profile visual table is not above the graph");
      assert(noHorizontalOverflow, "Profile surface still has horizontal overflow");
      assert(applyRight, "Apply Profile is not a right-aligned badge");
      assert(domSemantics, "Profile visual table/listbox semantics are invalid");
      assert(inspectByCompact, "Inspect by toggle still stretches across its grid column");
      assert(profileControlsAligned, "Inspect by, Dimensions, and Metric controls are not top-aligned");

      const regroupProfile = structuredClone(profile);
      regroupProfile.graphDataset.series = regroupProfile.graphDataset.series
        .filter((series) => series.scopeType === "zone");
      state.report = { profile: regroupProfile };
      state.profileViewCache = new Map();
      state.profileSettings = {
        ...state.profileSettings,
        numericTolerance: 0.001,
        scheduleCompareMode: "none",
        timeView: "week",
        scaleMode: "shared",
      };
      state.profileSelectedDimensions = ["occupancy"];
      state.activeProfileView = "profile";
      state.activeProfileGroupId = "";
      state.activeProfileZoneName = "";
      state.profileSelectedGroupIds = [];
      state.profileSelectedZoneNames = [];
      state.profileSelectionAnchorKey = "";
      profileViews.renderProfile(regroupProfile);
      await nextPaint();
      const rowIndexForZone = (zoneName) => rows().findIndex((row) => (
        row.querySelector(":scope > .profile-card-assignment[title]")?.getAttribute("title") || ""
      ).includes(zoneName));
      await clickRow(rowIndexForZone("Zone 2"));
      await clickRow(rowIndexForZone("Zone 3"), { ctrlKey: true });
      const preMergeIDs = [...state.profileSelectedGroupIds];
      assert(preMergeIDs.length === 2, "regroup fixture did not start with two distinct selected groups");
      state.profileSettings.numericTolerance = 0.25;
      profileViews.renderProfile(regroupProfile);
      await nextPaint();
      const mergedSelectedRows = selectedRows();
      const mergedAssignment = mergedSelectedRows[0]?.querySelector(":scope > .profile-card-assignment[title]");
      const mergedZonesDetail = mergedAssignment?.getAttribute("title") || "";
      const mergedVisibleAssignment = mergedAssignment?.textContent?.trim() || "";
      const mergedAccessibleAssignment = mergedAssignment?.getAttribute("aria-label") || "";
      const regroupSelectionByMembership = state.profileSelectedGroupIds.length === 1
        && mergedSelectedRows.length === 1
        && mergedVisibleAssignment === i18n.t("count.zones", { count: 2 })
        && mergedZonesDetail.includes("Zone 2")
        && mergedZonesDetail.includes("Zone 3")
        && !mergedVisibleAssignment.includes("Zone 2")
        && !mergedVisibleAssignment.includes("Zone 3")
        && mergedAccessibleAssignment.includes(mergedZonesDetail)
        && !preMergeIDs.includes(state.profileSelectedGroupIds[0]);
      assert(regroupSelectionByMembership, "group ordinal reassignment/merge did not preserve selection by zone membership");
      const aggregatePath = document.querySelector(".profile-overlay-paths path");
      const aggregatePaths = [...document.querySelectorAll(".profile-overlay-paths path")];
      const aggregateY = [...(aggregatePath?.getAttribute("d") || "").matchAll(/[ML][^,]+,([0-9.]+)/g)]
        .map((match) => Number(match[1]));
      const expectedNiceAxisMax = 2.5;
      const expectedLowY = 200 - (((0.2 * 2.5) + (0.4 * 3.5)) / 2 / expectedNiceAxisMax) * 200;
      const expectedHighY = 200 - (((0.8 * 2.5) + (0.6 * 3.5)) / 2 / expectedNiceAxisMax) * 200;
      const regroupAggregateAverage = aggregatePaths.length === 1
        && aggregatePath?.dataset.profileSeriesId.startsWith("profile-series-current-")
        && Math.abs(aggregateY[0] - expectedLowY) < 0.02
        && Math.abs(aggregateY[8] - expectedHighY) < 0.02;
      assert(regroupAggregateAverage, "merged group line is not the pointwise average of its Zone series");

      const withinHorizontalBounds = (child, parent, tolerance = 1) => {
        const childRect = child.getBoundingClientRect();
        const parentRect = parent.getBoundingClientRect();
        return childRect.width > 0
          && childRect.left >= parentRect.left - tolerance
          && childRect.right <= parentRect.right + tolerance;
      };
      const withinBounds = (child, parent, tolerance = 1) => {
        const childRect = child.getBoundingClientRect();
        const parentRect = parent.getBoundingClientRect();
        return childRect.width > 0
          && childRect.height > 0
          && childRect.left >= parentRect.left - tolerance
          && childRect.right <= parentRect.right + tolerance
          && childRect.top >= parentRect.top - tolerance
          && childRect.bottom <= parentRect.bottom + tolerance;
      };
      const noXScroll = (element) => element.scrollWidth <= element.clientWidth + 1;
      const responsiveAxisExpectations = {
        160: {
          overlayX: ["WD 00", "Sun 24"],
          overlayYCount: 2,
          months: ["Jan", "Dec"],
          hours: ["00", "24"],
          scaleCount: 2,
        },
        240: {
          overlayX: ["WD 00", "Sat 12", "Sun 24"],
          overlayYCount: 3,
          months: ["Jan", "Jul", "Dec"],
          hours: ["00", "12", "24"],
          scaleCount: 3,
        },
        360: {
          overlayX: expectedOverlayAxes.day.ticks,
          overlayYCount: 6,
          months: expectedOverlayAxes.month.ticks,
          hours: ["00", "06", "12", "18", "24"],
          scaleCount: 3,
        },
      };
      const inspectContainerWidth = async (width) => {
        document.getElementById("profilePane").style.width = width + "px";
        Object.assign(state.profileSettings, {
          timeView: "day",
        });
        state.profileSelectedDimensions = ["occupancy"];
        profileViews.renderProfile(regroupProfile);
        await nextPaint();
        const responsiveExpectation = responsiveAxisExpectations[width];
        const overlayGraph = document.getElementById("profileGraph");
        const overlayChart = overlayGraph.querySelector(".profile-line-chart");
        const overlaySVG = overlayChart?.querySelector("svg.profile-overlay-graph");
        const overlayXContainer = overlayChart?.querySelector(".profile-x-axis-ticks");
        const overlayYContainer = overlayChart?.querySelector(".profile-y-axis-ticks");
        const overlayVisibleX = visibleElements(overlayChart, ".profile-x-axis-ticks .profile-axis-tick");
        const overlayVisibleY = visibleElements(overlayChart, ".profile-y-axis-ticks .profile-axis-tick");
        const overlaySurfaces = [
          overlayGraph,
          ...document.querySelectorAll(
            ".profile-overlay-stack, .profile-overlay-panel, .profile-line-chart, .profile-line-plot, .profile-x-axis-ticks, .profile-y-axis-ticks, svg.profile-overlay-graph",
          ),
        ];
        const overlayOverflowingSurfaces = overlaySurfaces
          .filter((element) => !noXScroll(element))
          .map((element) => (element.className?.baseVal || element.className || element.tagName)
            + ":" + element.scrollWidth + "/" + element.clientWidth);
        const overlayAxes = {
          xLabels: elementTexts(overlayVisibleX),
          yLabels: elementTexts(overlayVisibleY),
          expectedXLabels: responsiveExpectation.overlayX,
          expectedYCount: responsiveExpectation.overlayYCount,
          xDensity: JSON.stringify(elementTexts(overlayVisibleX)) === JSON.stringify(responsiveExpectation.overlayX),
          yDensity: overlayVisibleY.length === responsiveExpectation.overlayYCount,
          xBounds: Boolean(overlayXContainer)
            && overlayVisibleX.every((tick) => withinBounds(tick, overlayXContainer)),
          yBounds: Boolean(overlayYContainer)
            && overlayVisibleY.every((tick) => withinBounds(tick, overlayYContainer)),
          noOverflow: overlayOverflowingSurfaces.length === 0,
          preserveAspectRatioNone: overlaySVG?.getAttribute("preserveAspectRatio") === "none",
          htmlTicksOutsideSVG: !overlaySVG?.querySelector("text")
            && !overlaySVG?.contains(overlayXContainer)
            && !overlaySVG?.contains(overlayYContainer),
          overflowingSurfaces: overlayOverflowingSurfaces,
        };
        overlayAxes.responsive = overlayAxes.xDensity
          && overlayAxes.yDensity
          && overlayAxes.xBounds
          && overlayAxes.yBounds
          && overlayAxes.noOverflow
          && overlayAxes.preserveAspectRatioNone
          && overlayAxes.htmlTicksOutsideSVG;
		const overlayLegend = overlayGraph.querySelector(".profile-overlay-legend");
		const longLegendItem = overlayLegend?.querySelector("[data-profile-series-id]");
		const longLegendLabel = longLegendItem?.querySelector(".profile-line-label");
		const longLegendSwatch = longLegendItem?.querySelector(".profile-line-swatch");
		let longLegendLabelAtWidth = width !== 160;
		let longLegendDiagnostics = {};
		if (width === 160 && overlayLegend && longLegendItem && longLegendLabel && longLegendSwatch) {
		  overlayLegend.style.width = "160px";
		  overlayLegend.style.maxWidth = "160px";
		  longLegendLabel.textContent = "A very long engineering profile name that must remain inside the 160 pixel graph";
		  const longLegendLabelStyle = getComputedStyle(longLegendLabel);
		  const longLegendSwatchRect = longLegendSwatch.getBoundingClientRect();
		  const longLegendRect = overlayLegend.getBoundingClientRect();
		  longLegendDiagnostics = {
			overflowHidden: longLegendLabelStyle.overflowX === "hidden",
			ellipsis: longLegendLabelStyle.textOverflow === "ellipsis",
			noWrap: longLegendLabelStyle.whiteSpace === "nowrap",
			clipped: longLegendLabel.scrollWidth > longLegendLabel.clientWidth,
			swatchWide: longLegendSwatchRect.width >= 14,
			swatchThin: longLegendSwatchRect.height <= 4,
			swatchFixed: getComputedStyle(longLegendSwatch).flexShrink === "0",
			itemWithinLegend: withinHorizontalBounds(longLegendItem, overlayLegend),
			legendNoScroll: noXScroll(overlayLegend),
			legendIs160: Math.abs(longLegendRect.width - 160) <= 1,
			labelWidth: longLegendLabel.clientWidth + "/" + longLegendLabel.scrollWidth,
			legendWidth: overlayLegend.clientWidth + "/" + overlayLegend.scrollWidth,
		  };
		  longLegendLabelAtWidth = Object.values(longLegendDiagnostics)
			.slice(0, 10)
			.every(Boolean);
		}

        state.profileSettings.timeView = "year";
        profileViews.renderProfile(regroupProfile);
        await nextPaint();
        const currentPane = document.querySelector(".profile-pane");
        const currentSettings = document.getElementById("profileSettings");
        const currentInspectSwitch = currentSettings.querySelector(".profile-view-switch");
        const currentInspectGroup = currentInspectSwitch.closest(".profile-live-group");
        const currentInspectButtons = [...currentInspectSwitch.querySelectorAll(".profile-segment-button")];
        const currentTable = document.querySelector(".profile-overview-table");
        const currentGraph = document.getElementById("profileGraph");
        const heatmapFrame = currentGraph.querySelector(".profile-heatmap-frame");
        const heatmapMonthContainer = heatmapFrame?.querySelector(".profile-heatmap-x-ticks");
        const heatmapHourContainer = heatmapFrame?.querySelector(".profile-heatmap-y-ticks");
        const heatmapScale = heatmapFrame?.querySelector(".profile-heatmap-scale");
        const heatmapScaleTickContainer = heatmapScale?.querySelector(".profile-heatmap-scale-ticks");
        const visibleMonths = visibleElements(heatmapFrame, ".profile-heatmap-x-ticks .profile-axis-tick");
        const visibleHours = visibleElements(heatmapFrame, ".profile-heatmap-y-ticks .profile-axis-tick");
        const visibleScaleTicks = visibleElements(heatmapScale, ".profile-heatmap-scale-ticks > span");
        const allMonthLabels = elementTexts([...(heatmapMonthContainer?.querySelectorAll(".profile-axis-tick") || [])]);
        const allHourLabels = elementTexts([...(heatmapHourContainer?.querySelectorAll(".profile-axis-tick") || [])]);
        const allScaleLabels = elementTexts([...(heatmapScaleTickContainer?.querySelectorAll(":scope > span") || [])]);
        const annualSurfaces = [...document.querySelectorAll(
          ".profile-annual-stack, .profile-annual-panel, .profile-annual-heatmap-grid, .profile-annual-heatmap-card, .profile-heatmap-frame, .profile-heatmap-scale, canvas.profile-heatmap",
        )];
        const namedSurfaces = [
          ["pane", currentPane],
          ["settings", currentSettings],
          ["table", currentTable],
          ["graph", currentGraph],
          ...annualSurfaces.map((surface, index) => ["annual-" + index + "-" + surface.className, surface]),
        ];
        const overflowingSurfaces = namedSurfaces
          .filter(([, element]) => !noXScroll(element))
          .map(([name, element]) => name + ":" + element.scrollWidth + "/" + element.clientWidth);
        const surfaceNoOverflow = overflowingSurfaces.length === 0;
        const expectedValueTitle = i18n.t("profile.axisValue", { unit: "person/m2" }, "Value [{unit}]");
        const annualAxes = {
          monthLabels: elementTexts(visibleMonths),
          hourLabels: elementTexts(visibleHours),
          scaleLabels: elementTexts(visibleScaleTicks),
          expectedMonthLabels: responsiveExpectation.months,
          expectedHourLabels: responsiveExpectation.hours,
          expectedScaleCount: responsiveExpectation.scaleCount,
          monthDensity: JSON.stringify(elementTexts(visibleMonths)) === JSON.stringify(responsiveExpectation.months),
          hourDensity: JSON.stringify(elementTexts(visibleHours)) === JSON.stringify(responsiveExpectation.hours),
          scaleDensity: visibleScaleTicks.length === responsiveExpectation.scaleCount,
          monthBounds: Boolean(heatmapFrame && heatmapMonthContainer)
            && visibleMonths.every((tick) => withinBounds(tick, heatmapFrame)),
          hourBounds: Boolean(heatmapFrame && heatmapHourContainer)
            && visibleHours.every((tick) => withinBounds(tick, heatmapFrame)),
          scaleBounds: Boolean(heatmapScaleTickContainer)
            && visibleScaleTicks.every((tick) => withinBounds(tick, heatmapScaleTickContainer)),
          axesSeparated: Boolean(heatmapFrame && heatmapMonthContainer && heatmapHourContainer && heatmapScale)
            && heatmapMonthContainer !== heatmapHourContainer
            && heatmapMonthContainer !== heatmapScale
            && heatmapHourContainer !== heatmapScale
            && JSON.stringify(allMonthLabels) === JSON.stringify(expectedOverlayAxes.month.ticks)
            && JSON.stringify(allHourLabels) === JSON.stringify(["00", "06", "12", "18", "24"])
            && allScaleLabels.length === 3
            && heatmapFrame.querySelector(".profile-heatmap-x-title")?.textContent.trim()
              === i18n.t("profile.axisMonth", {}, "Month")
            && heatmapFrame.querySelector(".profile-heatmap-y-title")?.textContent.trim()
              === i18n.t("profile.axisHourOfDay", {}, "Hour of day [h]")
            && heatmapScale.querySelector(".profile-heatmap-scale-title")?.textContent.trim() === expectedValueTitle
            && Boolean(heatmapScale.querySelector(".profile-heatmap-scale-bar")),
          noOverflow: surfaceNoOverflow,
        };
        annualAxes.responsive = annualAxes.monthDensity
          && annualAxes.hourDensity
          && annualAxes.scaleDensity
          && annualAxes.monthBounds
          && annualAxes.hourBounds
          && annualAxes.scaleBounds
          && annualAxes.axesSeparated
          && annualAxes.noOverflow;
        const currentOverview = document.getElementById("profileOverview");
        const currentOverviewRows = rows();
        const currentOverviewPresentation = inspectOverviewRows("profile");
        const overviewBoundaryIssues = [];
        currentOverviewRows.forEach((row) => {
          const assignment = row.querySelector(":scope > .profile-card-assignment");
          const metrics = row.querySelector(":scope > .profile-card-metrics");
          const metricCells = metrics ? [...metrics.querySelectorAll(":scope > .profile-card-metric")] : [];
          const boundaryPairs = [
            ["row>overview", row, currentOverview],
            ["assignment>row", assignment, row],
            ["metrics>row", metrics, row],
            ...metricCells.map((cell, index) => ["metric-" + index + ">metrics", cell, metrics]),
          ];
          boundaryPairs.forEach(([label, child, parent]) => {
            if (!child || !parent || !noXScroll(child) || !withinHorizontalBounds(child, parent)) {
              overviewBoundaryIssues.push({
                rowKey: row.dataset.profileRowKey || "",
                label,
                childScroll: child ? child.scrollWidth + "/" + child.clientWidth : "missing",
              });
            }
          });
        });
        const overviewListboxSemantics = currentOverview.getAttribute("role") === "listbox"
          && currentOverview.getAttribute("aria-multiselectable") === "true"
          && currentOverviewRows.length > 0
          && currentOverviewRows.every((row) => row.getAttribute("role") === "option"
            && ["true", "false"].includes(row.getAttribute("aria-selected")));
        const overviewRowsWithinBounds = currentOverviewPresentation.metricCells
          && currentOverviewPresentation.assignments
          && overviewListboxSemantics
          && overviewBoundaryIssues.length === 0;

        const currentApply = document.getElementById("profileApplyButton");
        const currentHead = currentApply.closest(".profile-overview-table-head");
        const applyStyle = getComputedStyle(currentApply);
        const applyVisibleWithinHeader = applyStyle.display !== "none"
          && applyStyle.visibility !== "hidden"
          && Number(applyStyle.opacity) !== 0
          && currentApply.getClientRects().length === 1
          && noXScroll(currentApply)
          && withinHorizontalBounds(currentApply, currentHead);

        const viewControl = document.querySelector(".profile-time-view-control");
        const viewControlItems = [...viewControl.querySelectorAll(".profile-graph-view-switch, [data-profile-time-view]")];
        const toolbarUnclipped = noXScroll(viewControl)
          && viewControlItems.length === 6
          && viewControlItems.every((item) => noXScroll(item) && withinHorizontalBounds(item, viewControl));
        const inspectRect = currentInspectSwitch.getBoundingClientRect();
        const inspectGroupRect = currentInspectGroup.getBoundingClientRect();
        const inspectToggleUnclipped = noXScroll(currentInspectSwitch)
          && withinHorizontalBounds(currentInspectSwitch, currentInspectGroup)
          && currentInspectButtons.length === 2
          && currentInspectButtons.every((button) => noXScroll(button) && withinHorizontalBounds(button, currentInspectSwitch))
          && (width <= 240
            ? Math.abs(inspectRect.width - inspectGroupRect.width) <= 1
            : inspectRect.width < inspectGroupRect.width - 1);
        return {
          width,
          surfaceNoOverflow,
          overlayAxes,
          annualAxes,
		  longLegendLabelAtWidth,
		  longLegendDiagnostics,
          applyVisibleWithinHeader,
          toolbarUnclipped,
          inspectToggleUnclipped,
          overviewRowsWithinBounds,
          overviewListboxSemantics,
          overviewBoundaryIssues,
          currentOverviewPresentation,
          overflowingSurfaces,
        };
      };
      let containerQueryWidths = false;
      let overviewRowsNoOverflowAtContainerWidths = false;
      let responsiveOverlayAxes = false;
      let responsiveAnnualAxes = false;
	  let longLegendLabelAt160 = false;
      if (!narrowViewport) {
        const widthResults = [];
        for (const width of [160, 240, 360]) {
          widthResults.push(await inspectContainerWidth(width));
        }
        containerQueryWidths = widthResults.every((result) => (
          result.surfaceNoOverflow
          && result.applyVisibleWithinHeader
          && result.toolbarUnclipped
          && result.inspectToggleUnclipped
        ));
        overviewRowsNoOverflowAtContainerWidths = widthResults.every((result) => result.overviewRowsWithinBounds);
        responsiveOverlayAxes = widthResults.every((result) => result.overlayAxes.responsive);
        responsiveAnnualAxes = widthResults.every((result) => result.annualAxes.responsive);
		longLegendLabelAt160 = widthResults.find((result) => result.width === 160)?.longLegendLabelAtWidth === true;
        assert(containerQueryWidths, "Profile container query clipping: " + JSON.stringify(widthResults));
        assert(overviewRowsNoOverflowAtContainerWidths, "Profile overview rows overflow or lose semantics at container widths: "
          + JSON.stringify(widthResults));
        assert(responsiveOverlayAxes, "Profile overlay axis density/bounds failed at container widths: "
          + JSON.stringify(widthResults));
        assert(responsiveAnnualAxes, "Profile annual axis density/separation/bounds failed at container widths: "
          + JSON.stringify(widthResults));
		assert(longLegendLabelAt160, "long Profile legend label escaped or displaced its swatch at 160px: "
		  + JSON.stringify(widthResults));
      }

	  const profileNavigationAdapter = panelRegistry.getPanelNavigationAdapter("profile");
	  assert(profileNavigationAdapter, "Profile navigation adapter was not registered");
	  Object.assign(state.profileSettings, {
		enabledDimensions: ["occupancy"],
		timeView: "week",
	  });
	  state.profileSelectedDimensions = ["occupancy"];
	  state.activeProfileView = "profile";
	  state.report = { profile: regroupProfile };
	  profileViews.renderProfile(regroupProfile);
	  await nextPaint();
	  const graphRevealHandled = await profileNavigationAdapter.reveal({
		viewTarget: { view: "profile", targetKind: "profile-item", targetId: "item-1" },
	  }, { scroll: false });
	  await nextPaint();
	  const graphRevealTarget = document.activeElement;
	  const semanticGraphReveal = graphRevealHandled
		&& document.getElementById("profileGraph").contains(graphRevealTarget)
		&& state.activeProfileView === "zone"
		&& state.activeProfileZoneName === "Zone 1"
		&& JSON.stringify(state.profileSelectedDimensions) === JSON.stringify(["occupancy"]);

	  const overviewFallbackHandled = await profileNavigationAdapter.reveal({
		viewTarget: { view: "profile", targetKind: "profile-item", targetId: "lighting-item-1" },
	  }, { scroll: false });
	  await nextPaint();
	  const revealedZoneRow = document.querySelector('#profileOverview [data-profile-zone="Zone 1"]');
	  const revealedLightingCell = revealedZoneRow?.querySelector(
		'.profile-card-metric[data-profile-dimension="lighting"]:not(.is-missing)',
	  );
	  const revealedLightingToggle = document.querySelector(
		'#profileSettings input[data-profile-dimension="lighting"]',
	  );
	  const semanticOverviewFallback = overviewFallbackHandled
		&& document.activeElement === revealedZoneRow
		&& state.activeProfileView === "zone"
		&& state.activeProfileZoneName === "Zone 1"
		&& JSON.stringify(state.profileSelectedDimensions) === JSON.stringify(["lighting"])
		&& JSON.stringify(state.profileSettings.enabledDimensions) === JSON.stringify(["occupancy"])
		&& Boolean(revealedLightingCell)
		&& revealedLightingToggle?.checked === true;
	  const removedSecondaryUIStillAbsent = !document.querySelector([
		"#profileMatrixStats",
		"#profileMatrix",
		"#profileDetail",
		".profile-matrix",
		".profile-detail-panel",
		".profile-source-accordion",
		".profile-candidate-panel",
		"[data-profile-candidate-id]",
	  ].join(", "));
	  const semanticRevealGraphAndOverviewFallback = semanticGraphReveal
		&& semanticOverviewFallback
		&& removedSecondaryUIStillAbsent;
	  assert(semanticRevealGraphAndOverviewFallback, "Profile semantic reveal did not use Graph then Overview fallback: "
		+ JSON.stringify({
		  graphRevealHandled,
		  graphRevealTarget: graphRevealTarget?.className || graphRevealTarget?.tagName || "",
		  overviewFallbackHandled,
		  activeElement: document.activeElement?.dataset?.profileRowKey || document.activeElement?.tagName || "",
		  activeProfileView: state.activeProfileView,
		  activeProfileZoneName: state.activeProfileZoneName,
		  selectedDimensions: state.profileSelectedDimensions,
		  enabledDimensions: state.profileSettings.enabledDimensions,
		  revealedLightingCell: Boolean(revealedLightingCell),
		  revealedLightingToggle: revealedLightingToggle?.checked,
		  removedSecondaryUIStillAbsent,
		}));

      const ratioProfile = structuredClone(profile);
      const ratioZone = ratioProfile.zoneProfiles.find((zone) => zone.zoneName === "Zone 1");
      ratioZone.floorArea = 100;
      const firstSpaceLights = ratioZone.items.find((item) => item.dimension === "lighting");
      firstSpaceLights.sourceTarget = "Space A";
      firstSpaceLights.sourceTargetKind = "space";
      firstSpaceLights.normalized = [
        { id: "power_per_area", label: "Lighting power per area", unit: "W/m2", value: 4, displayValue: "4 W/m2", status: "ok" },
        { id: "total_power", label: "Total lighting power", unit: "W", value: 100, displayValue: "100 W", status: "ok" },
      ];
      ratioZone.items.push({
        ...structuredClone(firstSpaceLights),
        id: "lighting-item-space-b",
        objectIndex: 31,
        objectName: "Space B Lights",
        sourceTarget: "Space B",
        normalized: [
          { id: "power_per_area", label: "Lighting power per area", unit: "W/m2", value: 4, displayValue: "4 W/m2", status: "ok" },
          { id: "total_power", label: "Total lighting power", unit: "W", value: 300, displayValue: "300 W", status: "ok" },
        ],
      });
      state.report = { profile: ratioProfile };
      state.profileViewCache = new Map();
      state.profileSettings = {
        ...state.profileSettings,
        enabledDimensions: ["lighting"],
        displayMetrics: { ...state.profileSettings.displayMetrics, lighting: "power_per_area" },
        groupingMetrics: { ...state.profileSettings.groupingMetrics, lighting: "power_per_area" },
      };
      state.activeProfileView = "zone";
      state.activeProfileZoneName = "Zone 1";
      state.profileSelectedZoneNames = ["Zone 1"];
      profileViews.renderProfile(ratioProfile);
      await nextPaint();
      const spaceRatioCell = document.querySelector(
        '#profileOverview [data-profile-zone="Zone 1"] .profile-card-metric[data-profile-dimension="lighting"]',
      );
      spaceRatioAggregation = spaceRatioCell?.dataset.profileMetricStatus === "ok"
        && spaceRatioCell.querySelector(":scope > .profile-card-metric-value")?.textContent?.trim() === "4 W/m2"
        && !spaceRatioCell.textContent.includes("8 W/m2");
      assert(spaceRatioAggregation, "Space-level power densities were summed instead of aggregating 400 W / 100 m2");
      ratioZone.floorArea = 0;
      state.profileViewCache = new Map();
      profileViews.renderProfile(ratioProfile);
      await nextPaint();
      const noDenominatorRatioCell = document.querySelector(
        '#profileOverview [data-profile-zone="Zone 1"] .profile-card-metric[data-profile-dimension="lighting"]',
      );
      spaceRatioAggregation = spaceRatioAggregation
        && noDenominatorRatioCell?.dataset.profileMetricStatus === "ok"
        && noDenominatorRatioCell.querySelector(":scope > .profile-card-metric-value")?.textContent?.trim() === "400 W"
        && noDenominatorRatioCell.getAttribute("aria-label")?.includes("Preferred metric unavailable")
        && !noDenominatorRatioCell.textContent.includes("8 W/m2");
      assert(spaceRatioAggregation, "Distinct Space ratios were summed when the Zone denominator was unavailable");

      const weatherProfile = structuredClone(profile);
      weatherProfile.dimensions.push({ id: "infiltration", label: "Infiltration" });
      const weatherZone = weatherProfile.zoneProfiles.find((zone) => zone.zoneName === "Zone 1");
      weatherZone.items.push(
        {
          id: "flow-coefficient-a",
          zoneName: "Zone 1",
          dimension: "infiltration",
          objectIndex: 41,
          objectType: "ZoneInfiltration:FlowCoefficient",
          objectName: "Crack A",
          sourceTarget: "Zone 1",
          sourceTargetKind: "zone",
          aggregationSignature: "flow_coefficient|pressure_exponent=0.67",
          normalized: [{ id: "flow_coefficient", label: "Flow coefficient", unit: "m3/s-Pa^n", value: 0.05, status: "ok" }],
          warnings: [],
        },
        {
          id: "flow-coefficient-b",
          zoneName: "Zone 1",
          dimension: "infiltration",
          objectIndex: 42,
          objectType: "ZoneInfiltration:FlowCoefficient",
          objectName: "Crack B",
          sourceTarget: "Zone 1",
          sourceTargetKind: "zone",
          aggregationSignature: "flow_coefficient|pressure_exponent=0.50",
          normalized: [{ id: "flow_coefficient", label: "Flow coefficient", unit: "m3/s-Pa^n", value: 0.03, status: "ok" }],
          warnings: [],
        },
      );
      state.report = { profile: weatherProfile };
      state.profileViewCache = new Map();
      state.profileSettings = {
        ...state.profileSettings,
        enabledDimensions: ["infiltration"],
        displayMetrics: { ...state.profileSettings.displayMetrics, infiltration: "flow_coefficient" },
        groupingMetrics: { ...state.profileSettings.groupingMetrics, infiltration: "flow_coefficient" },
      };
      profileViews.renderProfile(weatherProfile);
      await nextPaint();
      const incompatibleWeatherCell = document.querySelector(
        '#profileOverview [data-profile-zone="Zone 1"] .profile-card-metric[data-profile-dimension="infiltration"]',
      );
      sourceMetricCompatibility = incompatibleWeatherCell?.dataset.profileMetricStatus === "unavailable"
        && incompatibleWeatherCell.querySelector(":scope > .profile-card-metric-value")?.textContent?.trim() === "—"
        && !incompatibleWeatherCell.textContent.includes("0.08");
      assert(sourceMetricCompatibility, "Incompatible weather-model parameters were summed in the frontend");

      const incompleteWeatherProfile = structuredClone(profile);
      incompleteWeatherProfile.dimensions.push({ id: "infiltration", label: "Infiltration" });
      const incompleteWeatherZone = incompleteWeatherProfile.zoneProfiles.find((zone) => zone.zoneName === "Zone 1");
      incompleteWeatherZone.items.push(...[1, 2].map((itemIndex) => ({
        id: "incomplete-ela-" + itemIndex,
        zoneName: "Zone 1",
        dimension: "infiltration",
        objectIndex: 50 + itemIndex,
        objectType: "ZoneInfiltration:EffectiveLeakageArea",
        objectName: "Incomplete ELA " + itemIndex,
        sourceTarget: "Zone 1",
        sourceTargetKind: "zone",
        aggregationSignature: "zoneinfiltration:effectiveleakagearea|stack=missing|wind=missing",
        normalized: [{
          id: "effective_leakage_area",
          label: "Effective leakage area",
          unit: "cm2",
          value: 500,
          status: "partial",
        }],
        warnings: [{ code: "incomplete_weather_airflow_model", message: "Required coefficients are missing." }],
      })));
      state.report = { profile: incompleteWeatherProfile };
      state.profileViewCache = new Map();
      state.profileSettings = {
        ...state.profileSettings,
        enabledDimensions: ["infiltration"],
        displayMetrics: { ...state.profileSettings.displayMetrics, infiltration: "effective_leakage_area" },
        groupingMetrics: { ...state.profileSettings.groupingMetrics, infiltration: "effective_leakage_area" },
      };
      profileViews.renderProfile(incompleteWeatherProfile);
      await nextPaint();
      const incompleteWeatherCell = document.querySelector(
        '#profileOverview [data-profile-zone="Zone 1"] .profile-card-metric[data-profile-dimension="infiltration"]',
      );
      sourceMetricCompatibility = sourceMetricCompatibility
        && incompleteWeatherCell?.dataset.profileMetricStatus === "unavailable"
        && incompleteWeatherCell.querySelector(":scope > .profile-card-metric-value")?.textContent?.trim() === "—"
        && !incompleteWeatherCell.textContent.includes("1000");
      assert(sourceMetricCompatibility, "Incomplete weather-model source inputs were aggregated in the frontend");

      incompleteWeatherZone.items = incompleteWeatherZone.items.filter((item) => item.id !== "incomplete-ela-2");
      state.profileViewCache = new Map();
      profileViews.renderProfile(incompleteWeatherProfile);
      await nextPaint();
      const singleIncompleteWeatherCell = document.querySelector(
        '#profileOverview [data-profile-zone="Zone 1"] .profile-card-metric[data-profile-dimension="infiltration"]',
      );
      sourceMetricCompatibility = sourceMetricCompatibility
        && singleIncompleteWeatherCell?.dataset.profileMetricStatus === "partial"
        && singleIncompleteWeatherCell.querySelector(":scope > .profile-card-metric-value")?.textContent?.includes("500");
      assert(sourceMetricCompatibility, "A single incomplete weather-model source input did not retain the backend partial-value contract");

      const availabilityProfile = structuredClone(profile);
      const availabilityZone1 = availabilityProfile.zoneProfiles.find((zone) => zone.zoneName === "Zone 1");
      const availabilityZone4 = availabilityProfile.zoneProfiles.find((zone) => zone.zoneName === "Zone 4");
      const zone1Occupancy = availabilityZone1.items.find((item) => item.dimension === "occupancy");
      const zone1Lighting = availabilityZone1.items.find((item) => item.dimension === "lighting");
      const zone4Occupancy = availabilityZone4.items.find((item) => item.dimension === "occupancy");
      zone1Occupancy.normalized = [
        { id: "people_per_area", label: "People per Area", unit: "person/m2", value: 0, displayValue: "N/A", status: "missing" },
        { id: "count", label: "People", unit: "people", value: 10, displayValue: "10 people", status: "ok" },
      ];
      availabilityZone1.items.push({
        ...structuredClone(zone1Occupancy),
        id: "item-1-unresolved",
        objectIndex: 30,
        objectName: "Unresolved People 1",
        normalized: [
          { id: "people_per_area", label: "People per Area", unit: "person/m2", value: 0, displayValue: "N/A", status: "missing" },
          { id: "count", label: "People", unit: "people", value: 0, displayValue: "N/A", status: "missing" },
        ],
        warnings: [{ code: "missing_zone_area", message: "Zone area is required to calculate occupancy." }],
      });
      zone1Lighting.normalized = [
        { id: "power_per_area", label: "Lighting power per area", unit: "W/m2", value: 0, displayValue: "N/A", status: "missing" },
        { id: "total_power", label: "Total lighting power", unit: "W", value: 1, displayValue: "1 W", status: "ok" },
      ];
      zone4Occupancy.normalized = [
        { id: "people_per_area", label: "People per Area", unit: "person/m2", value: 0, displayValue: "N/A", status: "missing" },
      ];
      zone4Occupancy.warnings = [{ code: "missing_zone_area", message: "Zone area is required to calculate occupancy." }];
      state.report = { profile: availabilityProfile };
      state.profileViewCache = new Map();
      state.profileSettings = {
        ...state.profileSettings,
        enabledDimensions: ["occupancy", "lighting"],
        displayMetrics: { occupancy: "people_per_area", lighting: "power_per_area" },
        groupingMetrics: { occupancy: "people_per_area", lighting: "power_per_area" },
      };
      state.activeProfileView = "zone";
      state.activeProfileZoneName = "Zone 1";
      state.profileSelectedZoneNames = ["Zone 1"];
      profileViews.renderProfile(availabilityProfile);
      await nextPaint();
      const fallbackMetricCell = document.querySelector(
        '#profileOverview [data-profile-zone="Zone 1"] .profile-card-metric[data-profile-dimension="lighting"]',
      );
      const partialMetricCell = document.querySelector(
        '#profileOverview [data-profile-zone="Zone 1"] .profile-card-metric[data-profile-dimension="occupancy"]',
      );
      const unavailableMetricCell = document.querySelector(
        '#profileOverview [data-profile-zone="Zone 4"] .profile-card-metric[data-profile-dimension="occupancy"]',
      );
      const notConfiguredMetricCell = document.querySelector(
        '#profileOverview [data-profile-zone="Zone 4"] .profile-card-metric[data-profile-dimension="lighting"]',
      );
      profileMetricAvailability = fallbackMetricCell?.dataset.profileMetricStatus === "ok"
        && fallbackMetricCell.querySelector(":scope > .profile-card-metric-value")?.textContent?.trim() === "1 W"
        && fallbackMetricCell.getAttribute("aria-label")?.includes("Preferred metric unavailable")
        && partialMetricCell?.dataset.profileMetricStatus === "partial"
        && partialMetricCell.querySelector(":scope > .profile-card-metric-value")?.textContent?.trim() === "10 people"
        && partialMetricCell.getAttribute("aria-label")?.includes("1 of 2 configured objects")
        && partialMetricCell.getAttribute("aria-label")?.includes("Zone area is required")
        && unavailableMetricCell?.dataset.profileMetricStatus === "unavailable"
        && unavailableMetricCell.querySelector(":scope > .profile-card-metric-value")?.textContent?.trim() === "—"
        && unavailableMetricCell.getAttribute("aria-label")?.includes("no engineering metric can be calculated")
        && unavailableMetricCell.getAttribute("aria-label")?.includes("Zone area is required")
        && notConfiguredMetricCell?.dataset.profileMetricStatus === "not-configured"
        && notConfiguredMetricCell.querySelector(":scope > .profile-card-metric-value")?.textContent?.trim() === "—"
        && !document.getElementById("profileOverview").textContent.includes("N/A")
        && !document.getElementById("profileOverview").innerHTML.includes("N/A");
      assert(profileMetricAvailability, "Profile overview did not distinguish fallback, partial, unavailable, and not-configured metrics without N/A");

      const result = {
        narrowViewport,
        tableAboveGraph,
        noHorizontalOverflow,
        applyRight,
        domSemantics,
        inspectByCompact,
        profileControlGroupsAdjacent,
        profileControlGroupsWrapNarrow,
        profileControlsAligned,
		secondaryProfileUIAbsent,
		semanticRevealGraphAndOverviewFallback,
        overviewMetricCells,
        profileMetricAvailability,
        spaceRatioAggregation,
        sourceMetricCompatibility,
        assignmentCountWithHoverDetail,
        zoneProfileNameOnly,
        singleSelection,
        singleSelectionLegend,
        ctrlToggle,
        ctrlToggleRemoval,
        ctrlShiftRange,
        shiftRange,
        zoneCtrlSelection,
        metaToggleRemoval,
        zoneShiftRange,
        profileZoneModes,
        profileZoneSelectionIsolation,
        zoneProfileOverlaySelection,
        graphFidelityStatusVisible,
        overlayLegendAlways,
		identicalCurveOverlay,
		identicalCurveLegend,
		semanticLegendOutline,
        annualHeatmapsParallel,
        annualCanvasPainted,
        annualNoHorizontalOverflow,
        primaryApplySourceBoundary,
        rowGestureHistoryOnce,
        rowGestureSemanticPrimary,
        regroupSelectionByMembership,
        regroupAggregateAverage,
        fixedTimeProfileControls,
        actualEngineeringValue,
        viewToggleImmediate,
        viewFocusRestored,
        fiveViewAxes,
        noVisibleGraphMultiplier,
        scaleRemovedFromProfileTab,
        legacyViewMigration,
        containerQueryWidths,
        overviewRowsNoOverflowAtContainerWidths,
        responsiveOverlayAxes,
        responsiveAnnualAxes,
		longLegendLabelAt160,
      };
      document.getElementById("result").textContent = JSON.stringify(result);
      document.body.dataset.profileLayoutSelectionStatus = "passed";
    } catch (error) {
      document.getElementById("result").textContent = error.stack || String(error);
      document.body.dataset.profileLayoutSelectionStatus = "failed";
    }
  </script>
</body>
</html>`
