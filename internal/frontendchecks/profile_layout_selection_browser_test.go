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
		`"overlayLegendAlways":true`,
		`"annualHeatmapsParallel":true`,
		`"annualCanvasPainted":true`,
		`"annualNoHorizontalOverflow":true`,
		`"primaryApplySourceBoundary":true`,
		`"rowGestureHistoryOnce":true`,
		`"rowGestureSemanticPrimary":true`,
		`"regroupSelectionByMembership":true`,
		`"regroupAggregateAverage":true`,
		`"fixedTimeProfileControls":true`,
		`"viewToggleImmediate":true`,
		`"viewFocusRestored":true`,
		`"scaleRemovedFromProfileTab":true`,
		`"legacyViewMigration":true`,
	}
	runHarness("wide", 1440, append(append([]string{}, commonSignals...), `"narrowViewport":false`, `"containerQueryWidths":true`, `"stackedMatrixDimensionMapping":true`))
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
              <span>Metrics</span>
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
      <section class="profile-section">
        <div class="profile-section-head"><h3>Profile Matrix</h3><span id="profileMatrixStats"></span></div>
        <div id="profileMatrix" class="profile-matrix"></div>
      </section>
      <section class="profile-section">
        <div class="profile-section-head"><h3>Profile source objects</h3></div>
        <div id="profileDetail" class="profile-detail-panel"></div>
      </section>
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
          weekMultiplierProfile: constantValues(168, 0.1 * number, 0.2 * number),
          annualMultiplierProfile: constantValues(8760, 0.1 * number, 0.2 * number),
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
          weekMultiplierProfile: constantValues(168, zoneLows[index], zoneHighs[index]),
          annualMultiplierProfile: constantValues(8760, zoneLows[index], zoneHighs[index]),
          sourceItemIds: ["item-" + number],
          warnings: [],
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
          metricMode: "actual",
          timeView: "week",
          scaleMode: "shared",
          applyBehavior: { defaultMode: "clone", replaceExistingPolicy: "replace" },
        },
        graphDataset: { defaultDeck: {}, series: [...groupSeries, ...zoneSeries] },
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
      const [{ state }, profileViews, navigationAdapters, selectionController, i18n, settingsClient] = await Promise.all([
        import("/src/js/state.js"),
        import("/src/js/views/profile-views.js"),
        import("/src/js/panel-navigation-adapters.js"),
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
      state.profileSelectedCell = null;
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

      const rows = () => [...document.querySelectorAll("#profileOverview [data-profile-row-key]")];
      const selectedRows = () => rows().filter((row) => row.getAttribute("aria-selected") === "true");
      const graphViewButtons = () => [...document.querySelectorAll("#profileGraph [data-profile-time-view]")];
      const graphViewButton = (view) => document.querySelector('#profileGraph [data-profile-time-view="' + view + '"]');
      const clickGraphView = async (view) => {
        const button = graphViewButton(view);
        assert(button, "missing direct Profile Graph View button " + view);
        button.click();
        await nextPaint();
        return document.activeElement === graphViewButton(view);
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
      const expectedGraphViews = ["day", "week", "month", "year", "duration", "rules"];
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
      )) && settingsClient.mergeSettings({
        profile: { timeView: "rules", scheduleSummaryMode: "representative_day" },
      }).profile.timeView === "rules"
        && settingsClient.mergeSettings({ profile: { graphMode: "multiplier" } }).profile.metricMode === "multiplier"
        && settingsClient.mergeSettings({ profile: { graphMode: "annual" } }).profile.metricMode === "annual"
        && settingsClient.mergeSettings({ profile: { metricMode: "design", graphMode: "multiplier" } }).profile.metricMode === "design";
      assert(fixedTimeProfileControls, "Profile Graph still exposes Graph Type/Scope/Compare/select View controls or lacks six direct View buttons");
      assert(scaleRemovedFromProfileTab, "Profile tab still exposes the Scale selector that belongs in app Settings");
      assert(legacyViewMigration, "legacy Profile schedule-summary settings did not migrate to View without overriding an explicit View");
      let viewFocusRestored = await clickGraphView("week");

      state.navigationUndoStack = [];
      semanticEvents.length = 0;
      semanticFollowAttempts.length = 0;
      const firstSemanticEntity = rows()[1].dataset.entityId || "";
      assert(firstSemanticEntity, "semantic-capable Profile fixture did not annotate its rows");
      await clickRow(1);
      const rowGestureHistoryOnce = state.navigationUndoStack.length === 1;
      assert(rowGestureHistoryOnce, "one Profile row gesture did not record exactly one undo snapshot");
      const firstSemanticSync = state.globalSelection.entityId === firstSemanticEntity
        && semanticEvents.at(-1)?.selection?.entityId === firstSemanticEntity
        && semanticEvents.at(-1)?.options?.follow === false;

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
        && semanticFollowAttempts.length === 0;
      assert(primaryApplySourceBoundary, "Ctrl removal changed Apply source before the primary row was removed");
      assert(rowGestureSemanticPrimary, "row gesture did not synchronize the global primary without semantic follow");

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

      let everyViewApplied = true;
      for (const view of ["month", "duration", "rules"]) {
        viewFocusRestored = (await clickGraphView(view)) && viewFocusRestored;
        everyViewApplied = everyViewApplied
          && state.profileSettings.timeView === view
          && graphViewButton(view)?.getAttribute("aria-pressed") === "true";
      }
      viewFocusRestored = (await clickGraphView("day")) && viewFocusRestored;
      const dayViewApplied = state.profileSettings.timeView === "day"
        && graphViewButton("day")?.getAttribute("aria-pressed") === "true"
        && Boolean(document.querySelector(".profile-overlay-graph"));
      viewFocusRestored = (await clickGraphView("week")) && viewFocusRestored;
      const viewToggleImmediate = everyViewApplied
        && dayViewApplied
        && state.profileSettings.timeView === "week"
        && graphViewButton("week")?.getAttribute("aria-pressed") === "true"
        && graphViewButtons().filter((button) => button.getAttribute("aria-pressed") === "true").length === 1;
      assert(viewToggleImmediate, "direct Profile Graph View buttons did not update the graph immediately");
      assert(viewFocusRestored, "Profile Graph View focus was not restored after rerender");
      const overlayPaths = [...document.querySelectorAll(".profile-overlay-paths path")];
      const overlayLabels = [...document.querySelectorAll(".profile-overlay-legend [data-profile-series-id]")];
      const overlayLegendAlways = overlayPaths.length === 3 && overlayLabels.length === overlayPaths.length;
      assert(overlayLegendAlways, "line overlay/legend did not track every selected row");

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

      document.querySelector('[data-profile-view="zone"]').click();
      await nextPaint();
      const zoneModeRows = state.activeProfileView === "zone"
        && rows().length === 4
        && rows().every((row) => row.dataset.profileRowKey.startsWith("zone:"));
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
        && zoneOverlayPaths.every((path) => path.dataset.profileSeriesId.startsWith("profile-series-current-"));
      assert(zoneProfileOverlaySelection, "Zone row selection did not drive its assigned Profile overlays under fixed Profile scope");

      document.getElementById("profilePane").style.width = narrowViewport ? "360px" : "720px";
      await nextPaint();
      const pane = document.querySelector(".profile-pane");
      const settings = document.getElementById("profileSettings");
      const table = document.querySelector(".profile-overview-table");
      const overview = document.getElementById("profileOverview");
      const graphSection = document.querySelector(".profile-visual");
      const graph = document.getElementById("profileGraph");
      const matrix = document.getElementById("profileMatrix");
      const detail = document.getElementById("profileDetail");
      const tableAboveGraph = table.getBoundingClientRect().top < graphSection.getBoundingClientRect().top;
      const noHorizontalOverflow = [pane, settings, table, overview, graphSection, graph, matrix, detail]
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

      const regroupProfile = structuredClone(profile);
      regroupProfile.graphDataset.series = regroupProfile.graphDataset.series
        .filter((series) => series.scopeType === "zone");
      state.report = { profile: regroupProfile };
      state.profileViewCache = new Map();
      state.profileSettings = {
        ...state.profileSettings,
        numericTolerance: 0.001,
        scheduleCompareMode: "none",
        metricMode: "multiplier",
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
        row.querySelector(".profile-card-zones")?.textContent || ""
      ).includes(zoneName));
      await clickRow(rowIndexForZone("Zone 2"));
      await clickRow(rowIndexForZone("Zone 3"), { ctrlKey: true });
      const preMergeIDs = [...state.profileSelectedGroupIds];
      assert(preMergeIDs.length === 2, "regroup fixture did not start with two distinct selected groups");
      state.profileSettings.numericTolerance = 0.25;
      profileViews.renderProfile(regroupProfile);
      await nextPaint();
      const mergedSelectedRows = selectedRows();
      const mergedZonesText = mergedSelectedRows[0]?.querySelector(".profile-card-zones")?.textContent || "";
      const regroupSelectionByMembership = state.profileSelectedGroupIds.length === 1
        && mergedSelectedRows.length === 1
        && mergedZonesText.includes("Zone 2")
        && mergedZonesText.includes("Zone 3")
        && !preMergeIDs.includes(state.profileSelectedGroupIds[0]);
      assert(regroupSelectionByMembership, "group ordinal reassignment/merge did not preserve selection by zone membership");
      const aggregatePath = document.querySelector(".profile-overlay-paths path");
      const aggregatePaths = [...document.querySelectorAll(".profile-overlay-paths path")];
      const aggregateY = [...(aggregatePath?.getAttribute("d") || "").matchAll(/[ML][^,]+,([0-9.]+)/g)]
        .map((match) => Number(match[1]));
      const expectedLowY = 180 - (0.3 / 0.7) * 168;
      const regroupAggregateAverage = aggregatePaths.length === 1
        && aggregatePath?.dataset.profileSeriesId.startsWith("profile-series-current-")
        && Math.abs(aggregateY[0] - expectedLowY) < 0.02
        && Math.abs(aggregateY[8] - 12) < 0.02;
      assert(regroupAggregateAverage, "merged group line is not the pointwise average of its Zone series");

      const matrixDimensions = [
        { id: "occupancy", label: i18n.profileDimensionLabel("occupancy") },
        { id: "lighting", label: i18n.profileDimensionLabel("lighting") },
      ];
      assert(matrixDimensions[0].label !== "Occupancy" && matrixDimensions[1].label !== "Lighting", "matrix fixture language is not localized");
      const localizedNumber = (value) => Number(value).toLocaleString(undefined, {
        maximumFractionDigits: Math.abs(Number(value)) < 1 ? 4 : 2,
      });
      const matrixExpectedValues = Object.fromEntries(Array.from({ length: 4 }, (_, index) => {
        const number = index + 1;
        return ["Zone " + number, {
          occupancy: localizedNumber(number * 0.1) + " person/m2",
          lighting: number < 4 ? localizedNumber(number === 1 ? 1 : 10) + " W/m2" : "N/A",
        }];
      }));
      const hasVisibleExactText = (root, expected) => {
        const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
        for (let node = walker.nextNode(); node; node = walker.nextNode()) {
          if (node.nodeValue.trim() !== expected) {
            continue;
          }
          const parent = node.parentElement;
          const style = getComputedStyle(parent);
          if (style.display !== "none" && style.visibility !== "hidden" && parent.getClientRects().length > 0) {
            return true;
          }
        }
        return false;
      };
      const withinHorizontalBounds = (child, parent, tolerance = 1) => {
        const childRect = child.getBoundingClientRect();
        const parentRect = parent.getBoundingClientRect();
        return childRect.width > 0
          && childRect.left >= parentRect.left - tolerance
          && childRect.right <= parentRect.right + tolerance;
      };
      const noXScroll = (element) => element.scrollWidth <= element.clientWidth + 1;
      const inspectContainerWidth = async (width) => {
        document.getElementById("profilePane").style.width = width + "px";
        Object.assign(state.profileSettings, {
          metricMode: "multiplier",
          timeView: "year",
        });
        state.profileSelectedDimensions = ["occupancy"];
        profileViews.renderProfile(regroupProfile);
        await nextPaint();
        const currentPane = document.querySelector(".profile-pane");
        const currentSettings = document.getElementById("profileSettings");
        const currentTable = document.querySelector(".profile-overview-table");
        const currentGraph = document.getElementById("profileGraph");
        const currentMatrix = document.getElementById("profileMatrix");
        const currentDetail = document.getElementById("profileDetail");
        const annualSurfaces = [...document.querySelectorAll(
          ".profile-annual-stack, .profile-annual-panel, .profile-annual-heatmap-grid, .profile-annual-heatmap-card, .profile-heatmap-frame, canvas.profile-heatmap",
        )];
        const namedSurfaces = [
          ["pane", currentPane],
          ["settings", currentSettings],
          ["table", currentTable],
          ["graph", currentGraph],
          ["matrix", currentMatrix],
          ["detail", currentDetail],
          ...annualSurfaces.map((surface, index) => ["annual-" + index + "-" + surface.className, surface]),
        ];
        const overflowingSurfaces = namedSurfaces
          .filter(([, element]) => !noXScroll(element))
          .map(([name, element]) => name + ":" + element.scrollWidth + "/" + element.clientWidth);
        const surfaceNoOverflow = overflowingSurfaces.length === 0;

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
          && viewControlItems.length === 7
          && viewControlItems.every((item) => noXScroll(item) && withinHorizontalBounds(item, viewControl));

        const accordions = [...document.querySelectorAll("#profileDetail .profile-source-accordion")];
        accordions.forEach((accordion) => { accordion.open = true; });
        await nextPaint();
        const clippedSourceItems = [];
        const sourceAccordionUnclipped = accordions.length > 0 && accordions.every((accordion, accordionIndex) => {
          const summary = accordion.querySelector("summary");
          const body = summary.nextElementSibling;
          const structuralItems = [accordion, summary, body];
          const boundaryPairs = [
            ["accordion>detail", accordion, currentDetail],
            ["summary>accordion", summary, accordion],
            ["body>accordion", body, accordion],
            ...[...summary.children].map((item, index) => ["summary-child-" + index, item, summary]),
            ...[...body.children].map((item, index) => ["body-child-" + index, item, body]),
          ];
          const invalidScrollItems = structuralItems.filter((item) => !noXScroll(item));
          const invalidBoundaryPairs = boundaryPairs.filter(([, child, parent]) => !withinHorizontalBounds(child, parent));
          const valid = invalidScrollItems.length === 0 && invalidBoundaryPairs.length === 0;
          if (!valid) {
            clippedSourceItems.push({
              accordionIndex,
              scroll: invalidScrollItems.map((item) => (
                item.tagName + "." + item.className + "=" + item.scrollWidth + "/" + item.clientWidth
              )),
              bounds: invalidBoundaryPairs.map(([label, child, parent]) => {
                const childRect = child.getBoundingClientRect();
                const parentRect = parent.getBoundingClientRect();
                return {
                  label,
                  child: child.tagName + "." + child.className,
                  parent: parent.tagName + "." + parent.className,
                  childLeft: Math.round(childRect.left * 10) / 10,
                  childRight: Math.round(childRect.right * 10) / 10,
                  childWidth: Math.round(childRect.width * 10) / 10,
                  parentLeft: Math.round(parentRect.left * 10) / 10,
                  parentRight: Math.round(parentRect.right * 10) / 10,
                  parentWidth: Math.round(parentRect.width * 10) / 10,
                };
              }),
            });
          }
          return valid;
        });
        const matrixTable = currentMatrix.querySelector("table");
        const matrixRows = [...matrixTable.querySelectorAll("tbody > tr")];
        const matrixMappingIssues = [];
        matrixRows.forEach((row) => {
          const zoneName = row.dataset.profileZone || "";
          const cells = [...row.querySelectorAll(":scope > td")];
          if (getComputedStyle(row).display !== "block" || cells.length !== matrixDimensions.length) {
            matrixMappingIssues.push({ zoneName, rowDisplay: getComputedStyle(row).display, cellCount: cells.length });
            return;
          }
          cells.forEach((cell, index) => {
            const dimension = matrixDimensions[index];
            const expectedValue = matrixExpectedValues[zoneName]?.[dimension.id] || "";
            const checks = {
              cellDisplay: getComputedStyle(cell).display === "block",
              dimensionID: cell.dataset.profileDimension === dimension.id,
              visibleLabel: hasVisibleExactText(cell, dimension.label),
              visibleValue: hasVisibleExactText(cell, expectedValue),
            };
            if (!Object.values(checks).every(Boolean)) {
              matrixMappingIssues.push({
                zoneName,
                expectedDimension: dimension.id,
                expectedLabel: dimension.label,
                expectedValue,
                actualDimension: cell.dataset.profileDimension || "",
                actualText: cell.innerText.trim().replace(/\s+/g, " "),
                checks,
              });
            }
          });
        });
        const missingCells = matrixRows.flatMap((row) => [...row.querySelectorAll(":scope > td.profile-matrix-empty")]);
        const stackedMatrixMapping = getComputedStyle(matrixTable.querySelector("thead")).display === "none"
          && matrixRows.length === 4
          && missingCells.length === 1
          && matrixMappingIssues.length === 0;
        return {
          width,
          surfaceNoOverflow,
          applyVisibleWithinHeader,
          toolbarUnclipped,
          sourceAccordionUnclipped,
          stackedMatrixMapping,
          matrixMappingIssues,
          overflowingSurfaces,
          clippedSourceItems,
        };
      };
      let containerQueryWidths = false;
      let stackedMatrixDimensionMapping = false;
      if (!narrowViewport) {
        const widthResults = [];
        for (const width of [160, 240, 360]) {
          widthResults.push(await inspectContainerWidth(width));
        }
        containerQueryWidths = widthResults.every((result) => (
          result.surfaceNoOverflow
          && result.applyVisibleWithinHeader
          && result.toolbarUnclipped
          && result.sourceAccordionUnclipped
        ));
        stackedMatrixDimensionMapping = widthResults.every((result) => result.stackedMatrixMapping);
        assert(containerQueryWidths, "Profile container query clipping: " + JSON.stringify(widthResults));
        assert(stackedMatrixDimensionMapping, "stacked Profile matrix lost dimension/value mapping: " + JSON.stringify(widthResults));
      }

      const result = {
        narrowViewport,
        tableAboveGraph,
        noHorizontalOverflow,
        applyRight,
        domSemantics,
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
        overlayLegendAlways,
        annualHeatmapsParallel,
        annualCanvasPainted,
        annualNoHorizontalOverflow,
        primaryApplySourceBoundary,
        rowGestureHistoryOnce,
        rowGestureSemanticPrimary,
        regroupSelectionByMembership,
        regroupAggregateAverage,
        fixedTimeProfileControls,
        viewToggleImmediate,
        viewFocusRestored,
        scaleRemovedFromProfileTab,
        legacyViewMigration,
        containerQueryWidths,
        stackedMatrixDimensionMapping,
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
