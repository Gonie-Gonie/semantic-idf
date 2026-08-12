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

func TestProfileZoneSelectionAndScheduleContributionGroupingBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser Profile selection/grouping harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/profile-selection-grouping", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, profileSelectionGroupingHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

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
		"--window-size=1200,1000",
		"--user-data-dir="+t.TempDir(),
		"--dump-dom",
		server.URL+"/profile-selection-grouping",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("Profile selection/grouping browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("Profile selection/grouping browser harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-profile-selection-grouping-status="passed"`) {
		t.Fatalf("Profile selection/grouping browser harness did not pass:\n%s", document)
	}
	for _, signal := range []string{
		`"zoneSeriesRemainDistinct":true`,
		`"scaledDensitySharesProfile":true`,
		`"scheduleContributionsSplitGroups":true`,
		`"fallbackMetricIdentitySplitsGroups":true`,
	} {
		if !strings.Contains(document, signal) {
			t.Fatalf("Profile selection/grouping browser result is missing %s:\n%s", signal, document)
		}
	}
}

const profileSelectionGroupingHarnessHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Profile selection and grouping harness</title>
  <link rel="stylesheet" href="/src/styles.css">
</head>
<body data-profile-selection-grouping-status="pending">
  <div class="result-pane active" id="profilePane">
    <div class="profile-pane">
      <div id="profileSettings" class="profile-settings"></div>
      <div class="profile-layout">
        <section class="profile-overview">
          <div class="profile-overview-table">
            <div class="profile-overview-table-head">
              <span>Selection</span><span>Assignment</span><span>Metrics</span>
              <span><button id="profileApplyButton" type="button">Apply Profile</button></span>
            </div>
            <div id="profileOverview" class="profile-overview-list" role="listbox" aria-multiselectable="true"></div>
          </div>
        </section>
        <section class="profile-visual"><div id="profileGraph" class="profile-graph"></div></section>
      </div>
    </div>
  </div>
  <div id="profileApplyDialog" class="hidden">
    <form id="profileApplyForm">
      <button id="profileApplyClose" type="button">Close</button>
      <div id="profileApplyBody"></div>
      <span id="profileApplyStatus"></span>
      <button id="profilePreviewApply" type="button">Preview</button>
      <button id="profileConfirmApply" type="submit">Apply</button>
    </form>
  </div>
  <pre id="result">pending</pre>
  <script type="module">
    const assert = (condition, message, detail = {}) => {
      if (!condition) throw new Error(message + ": " + JSON.stringify(detail));
    };
    const scheduleNames = ["Schedule A", "Schedule B"];
    const contributionsByZone = [[10, 20], [20, 10]];
    const weekProfiles = [
      Array.from({ length: 168 }, (_, hour) => hour % 24 < 8 ? 0.2 : 0.8),
      Array.from({ length: 168 }, (_, hour) => hour % 24 < 8 ? 0.6 : 0.4),
    ];

    const zones = contributionsByZone.map((contributions, zoneIndex) => {
      const zoneName = "Zone " + (zoneIndex + 1);
      return {
        zoneName,
        zoneObjectIndex: zoneIndex,
        floorArea: 1,
        dimensions: [],
        warnings: [],
        items: scheduleNames.map((scheduleName, scheduleIndex) => ({
          id: "people-" + zoneIndex + "-" + scheduleIndex,
          zoneName,
          dimension: "occupancy",
          objectIndex: zoneIndex * 10 + scheduleIndex,
          objectType: "People",
          objectName: zoneName + " People " + (scheduleIndex + 1),
          sourceTarget: zoneName,
          sourceTargetKind: "zone",
          scheduleName,
          scheduleHash: "hash-" + scheduleName,
          schedulePattern: "pattern-" + scheduleName,
          rawMethod: "People",
          rawValue: String(contributions[scheduleIndex]),
          normalized: [
            { id: "people_per_area", label: "People per Area", unit: "person/m2", value: contributions[scheduleIndex], status: "ok" },
            { id: "count", label: "People", unit: "people", value: contributions[scheduleIndex], status: "ok" },
          ],
          warnings: [],
        })),
      };
    });
    const series = zones.map((zone, zoneIndex) => ({
      id: "zone-series-" + (zoneIndex + 1),
      label: zone.zoneName,
      scopeType: "zone",
      zoneName: zone.zoneName,
      dimension: "occupancy",
      dimensionLabel: "Occupancy",
      metricId: "people_per_area",
      metricLabel: "People per Area",
      unit: "person/m2",
      designValue: 30,
      scheduleName: scheduleNames.join(" + "),
      scheduleHash: "hash-Schedule A+hash-Schedule B",
      schedulePattern: "pattern-Schedule A + pattern-Schedule B",
      dayMultiplierProfile: weekProfiles[zoneIndex].slice(0, 72),
      weekMultiplierProfile: weekProfiles[zoneIndex],
      monthMultiplierProfile: Array.from({ length: 12 }, () => 0.5),
      annualMultiplierProfile: Array.from({ length: 8760 }, (_, hour) => weekProfiles[zoneIndex][hour % 168]),
      durationMultiplierProfile: [...weekProfiles[zoneIndex]].sort((left, right) => right - left),
      sourceItemIds: zone.items.map((item) => item.id),
      warnings: [],
    }));
    const profile = {
      zoneCount: zones.length,
      itemCount: zones.reduce((count, zone) => count + zone.items.length, 0),
      dimensions: [{ id: "occupancy", label: "Occupancy" }],
      zoneProfiles: zones,
      groups: [],
      matrix: [],
      schedules: [],
      warnings: [],
      defaultSettings: {
        enabledDimensions: ["occupancy"],
        displayMetrics: { occupancy: "people_per_area" },
        groupingMetrics: { occupancy: "people_per_area" },
        numericTolerance: 0.001,
        scheduleCompareMode: "none",
        timeView: "week",
        scaleMode: "shared",
        applyBehavior: { defaultMode: "clone", replaceExistingPolicy: "replace" },
      },
      graphDataset: { series },
    };

    try {
      localStorage.clear();
      const [{ state }, profileViews] = await Promise.all([
        import("/src/js/state.js"),
        import("/src/js/views/profile-views.js"),
      ]);
      state.report = { profile };
      state.analysisKey = "profile-selection-grouping-fixture";
      state.reportAnalysisKey = state.analysisKey;
      state.activeResultTab = "profile";
      state.profileSettings = structuredClone(profile.defaultSettings);
      state.profileViewCache = new Map();
      state.profileSelectedDimensions = ["occupancy"];
      state.activeProfileView = "zone";
      state.activeProfileGroupId = "";
      state.activeProfileZoneName = "";
      state.profileSelectedGroupIds = [];
      state.profileSelectedZoneNames = [];
      state.profileSelectionAnchorKey = "";
      profileViews.initializeProfileControls();
      profileViews.renderProfile(profile);

      state.activeProfileView = "zone";
      state.activeProfileZoneName = "Zone 1";
      state.profileSelectedZoneNames = ["Zone 1", "Zone 2"];
      state.profileSelectionAnchorKey = "zone:Zone 1";
      profileViews.renderProfile(profile);
      const zoneRows = [...document.querySelectorAll("#profileOverview [data-profile-zone]")];
      const selectedZoneRows = zoneRows.filter((row) => row.getAttribute("aria-selected") === "true");
      const assignments = selectedZoneRows.map((row) => (
        row.querySelector(":scope > .profile-card-assignment")?.textContent?.trim() || ""
      ));
      const paths = [...document.querySelectorAll(".profile-overlay-paths path")];
      const legends = [...document.querySelectorAll(".profile-overlay-legend [data-profile-series-id]")];
      const pathIDs = paths.map((path) => path.dataset.profileSeriesId).sort();
      const legendIDs = legends.map((legend) => legend.dataset.profileSeriesId).sort();
      const legendLabels = legends.map((legend) => (
        legend.querySelector(".profile-line-label")?.textContent?.trim() || ""
      ));
      const zoneSeriesRemainDistinct = selectedZoneRows.length === 2
        && new Set(assignments).size === 1
        && paths.length === 2
        && legends.length === 2
        && JSON.stringify(pathIDs) === JSON.stringify(["zone-series-1", "zone-series-2"])
        && JSON.stringify(legendIDs) === JSON.stringify(pathIDs)
        && legendLabels.some((label) => label.startsWith("Zone 1"))
        && legendLabels.some((label) => label.startsWith("Zone 2"))
        && new Set(paths.map((path) => path.getAttribute("d"))).size === 2
        && paths.every((path) => !path.dataset.profileSeriesId.startsWith("profile-series-current-"));
      assert(zoneSeriesRemainDistinct,
        "same-Profile Zone selection was averaged or deduplicated",
        { assignments, pathIDs, legendIDs, legendLabels });

      state.profileSettings.scheduleCompareMode = "name";
      state.profileViewCache = new Map();
      state.activeProfileView = "profile";
      state.activeProfileGroupId = "";
      state.profileSelectedGroupIds = [];
      state.profileSelectedZoneNames = [];
      state.profileSelectionAnchorKey = "";
      profileViews.renderProfile(profile);
      const profileRows = [...document.querySelectorAll("#profileOverview [data-profile-group-id]")];
      const profileAssignments = profileRows.map((row) => (
        row.querySelector(":scope > .profile-card-assignment[title]")?.getAttribute("title") || ""
      ));
      const profileNames = profileRows.map((row) => (
        row.querySelector(":scope > span:first-child strong")?.textContent?.trim() || ""
      ));
      const totals = zones.map((zone) => zone.items.reduce((sum, item) => (
        sum + Number(item.normalized.find((metric) => metric.id === "count")?.value || 0)
      ), 0));
      const scheduleSets = zones.map((zone) => zone.items.map((item) => item.scheduleName).sort().join("+"));
      const scheduleContributionsSplitGroups = totals[0] === totals[1]
        && scheduleSets[0] === scheduleSets[1]
        && profileRows.length === 2
        && profileAssignments.every((assignment) => !assignment.includes("\n"))
        && profileAssignments.includes("Zone 1")
        && profileAssignments.includes("Zone 2")
        && new Set(profileNames).size === 2;
      assert(scheduleContributionsSplitGroups,
        "swapped schedule-specific contributions collapsed into one Profile",
        { totals, scheduleSets, profileAssignments, profileNames });

      const scaledDensityProfile = structuredClone(profile);
      scaledDensityProfile.dimensions = [{ id: "lighting", label: "Lighting" }];
      scaledDensityProfile.zoneProfiles = [100, 200].map((floorArea, zoneIndex) => {
        const zoneName = "Scaled Zone " + (zoneIndex + 1);
        return {
          zoneName,
          zoneObjectIndex: 20 + zoneIndex,
          floorArea,
          dimensions: [],
          warnings: [],
          items: [{
            id: "scaled-lights-" + zoneIndex,
            zoneName,
            dimension: "lighting",
            objectIndex: 200 + zoneIndex,
            objectType: "Lights",
            objectName: zoneName + " Lights",
            sourceTarget: zoneName,
            sourceTargetKind: "zone",
            scheduleName: "Schedule A",
            scheduleHash: "hash-Schedule A",
            schedulePattern: "pattern-Schedule A",
            rawMethod: "Watts/Area",
            rawValue: "10",
            normalized: [
              { id: "power_per_area", label: "Power per Area", unit: "W/m2", value: 10, status: "ok" },
              { id: "total_power", label: "Total Power", unit: "W", value: floorArea * 10, status: "ok" },
            ],
            warnings: [],
          }],
        };
      });
      scaledDensityProfile.zoneCount = 2;
      scaledDensityProfile.itemCount = 2;
      scaledDensityProfile.graphDataset.series = [];
      scaledDensityProfile.defaultSettings = {
        ...scaledDensityProfile.defaultSettings,
        enabledDimensions: ["lighting"],
        displayMetrics: { lighting: "power_per_area" },
        groupingMetrics: { lighting: "power_per_area" },
        scheduleCompareMode: "name",
      };
      state.report = { profile: scaledDensityProfile };
      state.profileSettings = structuredClone(scaledDensityProfile.defaultSettings);
      state.profileViewCache = new Map();
      state.profileSelectedDimensions = ["lighting"];
      state.activeProfileView = "profile";
      state.activeProfileGroupId = "";
      state.profileSelectedGroupIds = [];
      state.profileSelectedZoneNames = [];
      state.profileSelectionAnchorKey = "";
      profileViews.renderProfile(scaledDensityProfile);
      const scaledRows = [...document.querySelectorAll("#profileOverview [data-profile-group-id]")];
      const scaledAssignment = scaledRows[0]?.querySelector(":scope > .profile-card-assignment[title]")
        ?.getAttribute("title") || "";
      const scaledDensitySharesProfile = scaledRows.length === 1
        && scaledAssignment.includes("Scaled Zone 1")
        && scaledAssignment.includes("Scaled Zone 2")
        && scaledRows[0]?.querySelector('[data-profile-dimension="lighting"] .profile-card-metric-value')
          ?.textContent.trim() === "10 W/m2";
      assert(scaledDensitySharesProfile,
        "equal W/m2 and schedule shape split by absolute Zone numerator",
        { rowCount: scaledRows.length, scaledAssignment });

      const fallbackProfile = structuredClone(profile);
      fallbackProfile.dimensions = [{ id: "occupancy", label: "Occupancy" }];
      fallbackProfile.zoneProfiles = [
        {
          zoneName: "Density Zone",
          zoneObjectIndex: 30,
          floorArea: 100,
          dimensions: [],
          warnings: [],
          items: [{
            id: "density-people",
            zoneName: "Density Zone",
            dimension: "occupancy",
            objectIndex: 300,
            objectType: "People",
            objectName: "Density People",
            sourceTarget: "Density Zone",
            sourceTargetKind: "zone",
            scheduleName: "Schedule A",
            scheduleHash: "hash-Schedule A",
            schedulePattern: "pattern-Schedule A",
            normalized: [
              { id: "people_per_area", label: "People per Area", unit: "person/m2", value: 10, status: "ok" },
              { id: "count", label: "People", unit: "people", value: 1000, status: "ok" },
            ],
            warnings: [],
          }],
        },
        {
          zoneName: "Count Zone",
          zoneObjectIndex: 31,
          floorArea: 0,
          dimensions: [],
          warnings: [],
          items: [{
            id: "count-people",
            zoneName: "Count Zone",
            dimension: "occupancy",
            objectIndex: 301,
            objectType: "People",
            objectName: "Count People",
            sourceTarget: "Count Zone",
            sourceTargetKind: "zone",
            scheduleName: "Schedule A",
            scheduleHash: "hash-Schedule A",
            schedulePattern: "pattern-Schedule A",
            normalized: [
              { id: "people_per_area", label: "People per Area", unit: "person/m2", value: 0, status: "missing" },
              { id: "count", label: "People", unit: "people", value: 10, status: "ok" },
            ],
            warnings: [{ code: "missing_zone_area", message: "Zone area is required." }],
          }],
        },
      ];
      fallbackProfile.zoneCount = 2;
      fallbackProfile.itemCount = 2;
      fallbackProfile.graphDataset.series = [];
      fallbackProfile.defaultSettings = {
        ...fallbackProfile.defaultSettings,
        enabledDimensions: ["occupancy"],
        displayMetrics: { occupancy: "people_per_area" },
        groupingMetrics: { occupancy: "people_per_area" },
        scheduleCompareMode: "name",
      };
      state.report = { profile: fallbackProfile };
      state.profileSettings = structuredClone(fallbackProfile.defaultSettings);
      state.profileViewCache = new Map();
      state.profileSelectedDimensions = ["occupancy"];
      state.activeProfileView = "profile";
      state.activeProfileGroupId = "";
      state.profileSelectedGroupIds = [];
      state.profileSelectedZoneNames = [];
      state.profileSelectionAnchorKey = "";
      profileViews.renderProfile(fallbackProfile);
      const fallbackRows = [...document.querySelectorAll("#profileOverview [data-profile-group-id]")];
      const fallbackAssignments = fallbackRows.map((row) => (
        row.querySelector(":scope > .profile-card-assignment[title]")?.getAttribute("title") || ""
      ));
      const fallbackValues = fallbackRows.map((row) => (
        row.querySelector('[data-profile-dimension="occupancy"] .profile-card-metric-value')?.textContent.trim() || ""
      ));
      const fallbackDescriptions = fallbackRows.map((row) => (
        row.querySelector('[data-profile-dimension="occupancy"]')?.getAttribute("aria-label") || ""
      ));
      const fallbackMetricIdentitySplitsGroups = fallbackRows.length === 2
        && fallbackAssignments.includes("Density Zone")
        && fallbackAssignments.includes("Count Zone")
        && fallbackValues.includes("10 person/m2")
        && fallbackValues.includes("10 people")
        && fallbackDescriptions.some((description) => description.includes("Preferred metric unavailable"));
      assert(fallbackMetricIdentitySplitsGroups,
        "equal numeric preferred density and count fallback merged into one Profile",
        { fallbackAssignments, fallbackValues, fallbackDescriptions });

      document.getElementById("result").textContent = JSON.stringify({
        zoneSeriesRemainDistinct,
        scaledDensitySharesProfile,
        scheduleContributionsSplitGroups,
        fallbackMetricIdentitySplitsGroups,
      });
      document.body.dataset.profileSelectionGroupingStatus = "passed";
    } catch (error) {
      document.getElementById("result").textContent = error.stack || String(error);
      document.body.dataset.profileSelectionGroupingStatus = "failed";
    }
  </script>
</body>
</html>`
