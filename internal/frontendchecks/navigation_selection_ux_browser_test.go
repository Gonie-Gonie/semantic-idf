package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestNavigationSelectionUXBrowserHarness exercises the real result-panel
// styling module and the application's delegated keyboard listeners in a
// browser. The same semantic entity can legitimately occur more than once in
// a pane; only the exact occurrence/target is the primary location.
func TestNavigationSelectionUXBrowserHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser navigation UX harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	frontendRoot := repoPath("frontend/src")
	index, err := os.ReadFile(repoPath("frontend/src/index.html"))
	if err != nil {
		t.Fatalf("read frontend index: %v", err)
	}
	page := strings.Replace(string(index), "</body>", navigationSelectionUXHarnessHTML+"\n</body>", 1)
	if page == string(index) {
		t.Fatal("frontend index has no closing body for navigation UX harness injection")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(frontendRoot))))
	mux.HandleFunc("/navigation-selection-ux", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, strings.Replace(page, "<head>", `<head><base href="/src/">`, 1))
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
		"--user-data-dir="+t.TempDir(),
		"--dump-dom",
		server.URL+"/navigation-selection-ux",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("headless browser navigation UX harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("headless browser navigation UX harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-navigation-selection-ux-status="passed"`) {
		t.Fatalf("headless browser navigation UX harness did not pass:\n%s", document)
	}
	for _, signal := range []string{
		`"profilePrimary":1`,
		`"summaryPrimary":1`,
		`"sourcePrimary":1`,
		`"maxCurrentLocations":1`,
		`"altEnterPalette":true`,
	} {
		if !strings.Contains(document, signal) {
			t.Fatalf("headless browser navigation UX result is missing %s:\n%s", signal, document)
		}
	}
}

const navigationSelectionUXHarnessHTML = `<script type="module">
const assert = (condition, message) => {
  if (!condition) {
    throw new Error(message);
  }
};

function item(id, attributes = {}, tagName = "button") {
  const element = document.createElement(tagName);
  element.id = id;
  if (tagName === "button") {
    element.type = "button";
  }
  element.className = "navigable-row";
  for (const [name, value] of Object.entries(attributes)) {
    element.dataset[name] = value;
  }
  element.textContent = id;
  return element;
}

function primaryItems(root) {
  return [...root.querySelectorAll(".semantic-selected")];
}

function currentLocations(root) {
  return [...root.querySelectorAll('[aria-current="location"]')];
}

function assertExclusivePrimary(root, exact, related, label) {
  const primary = primaryItems(root);
  assert(primary.length === 1, label + " must expose exactly one primary, got " + primary.length);
  assert(primary[0] === exact, label + " selected the wrong primary item");
  assert(exact.getAttribute("aria-current") === "location", label + " exact target is not the current location");
  assert(currentLocations(root).length <= 1, label + " exposes multiple current locations");
  for (const sibling of related) {
    assert(!sibling.classList.contains("semantic-selected"), label + " same-entity sibling became primary");
    assert(sibling.classList.contains("semantic-related"), label + " same-entity sibling is not related");
    assert(sibling.getAttribute("aria-current") !== "location", label + " related sibling became a current location");
  }
  return primary.length;
}

async function runNavigationSelectionUXHarness() {
  const [{ state }, adapters] = await Promise.all([
    import("/src/js/state.js"),
    import("/src/js/panel-navigation-adapters.js"),
  ]);

  const scheduleDefinitionTarget = { view: "profile", targetKind: "profile-item", targetId: "profile-definition", priority: 100 };
  const scheduleUseTarget = { view: "profile", targetKind: "profile-item", targetId: "profile-use", priority: 90 };
  const summaryCategoryTarget = { view: "summary", targetKind: "summary-category", targetId: "schedules_operation", priority: 100 };
  const navigation = {
    entities: [
      {
        id: "schedule-entity",
        kind: "schedule",
        label: "Office Schedule",
        occurrenceIds: ["schedule-definition", "schedule-use"],
        viewTargets: [scheduleDefinitionTarget, scheduleUseTarget],
      },
      {
        id: "schedule-section",
        kind: "semantic-section",
        label: "Schedules and operation",
        occurrenceIds: ["schedule-section-occurrence"],
        viewTargets: [summaryCategoryTarget],
      },
    ],
    occurrences: [
      {
        occurrenceId: "schedule-definition",
        entityId: "schedule-entity",
        path: "schedules/definitions/Office Schedule",
        contextKind: "definition",
        preferredView: "profile",
        preferredTargetId: "profile-definition",
        viewTargets: [scheduleDefinitionTarget],
      },
      {
        occurrenceId: "schedule-use",
        entityId: "schedule-entity",
        path: "zones/Office/schedules/Office Schedule",
        contextKind: "reference",
        preferredView: "profile",
        preferredTargetId: "profile-use",
        viewTargets: [scheduleUseTarget],
      },
      {
        occurrenceId: "schedule-section-occurrence",
        entityId: "schedule-section",
        path: "schedules",
        contextKind: "section",
        preferredView: "summary",
        preferredTargetId: "schedules_operation",
        viewTargets: [summaryCategoryTarget],
      },
    ],
    byEntityId: {
      "schedule-entity": ["schedule-definition", "schedule-use"],
      "schedule-section": ["schedule-section-occurrence"],
    },
    byObjectId: {},
    byObjectIndex: {},
    byViewTarget: {
      "profile|profile-definition": ["schedule-definition"],
      "profile|profile-use": ["schedule-use"],
      "summary|schedules_operation": ["schedule-section-occurrence"],
    },
  };
  state.semanticProjection = { schema: "eplus-semantic/0.2", navigation };
  state.reportAnalysisKey = "navigation-selection-ux";

  const profilePane = document.getElementById("profilePane");
  const profileFixture = document.createElement("div");
  profileFixture.dataset.navigationSelectionFixture = "profile";
  const profileDefinition = item("profileDefinition", {
    entityId: "schedule-entity",
    occurrenceId: "schedule-definition",
    panelTargetId: "profile-definition",
  });
  const profileUse = item("profileUse", {
    entityId: "schedule-entity",
    occurrenceId: "schedule-use",
    panelTargetId: "profile-use",
  });
  profileFixture.append(profileDefinition, profileUse);
  profilePane.append(profileFixture);

  const profileSelection = {
    entityId: "schedule-entity",
    occurrenceId: "schedule-definition",
    originView: "profile",
    originTargetId: "profile-definition",
    semanticPathHint: "schedules/definitions/Office Schedule",
    relatedEntityIds: [],
  };
  const profilePrimary = adapters.refreshResultPanelSelectionStyles("profile", profileSelection, null);
  assert(profilePrimary === 1, "profile refresh returned more than one primary");
  assertExclusivePrimary(profilePane, profileDefinition, [profileUse], "profile duplicate occurrence");

  const summaryCategories = document.getElementById("summaryCategories");
  const summaryCategory = item("summaryScheduleCategory", {
    entityId: "schedule-section",
    occurrenceId: "schedule-section-occurrence",
    panelTargetId: "schedules_operation",
    summaryCategoryId: "schedules_operation",
  }, "details");
  const summaryLabel = document.createElement("summary");
  summaryLabel.textContent = "Schedules and operation";
  const scheduleCount = item("summaryScheduleCount", {
    entityId: "schedule-section",
    occurrenceId: "schedule-section-occurrence",
    panelTargetId: "schedule_count",
    summaryMetricId: "schedule_count",
  }, "div");
  const scheduleRules = item("summaryScheduleRules", {
    entityId: "schedule-section",
    occurrenceId: "schedule-section-occurrence",
    panelTargetId: "schedule_rules",
    summaryMetricId: "schedule_rules",
  }, "div");
  const sourceDefinition = item("summarySourceDefinition", {
    entityId: "schedule-entity",
    occurrenceId: "schedule-definition",
  });
  const sourceUse = item("summarySourceUse", {
    entityId: "schedule-entity",
    occurrenceId: "schedule-use",
  });
  summaryCategory.append(summaryLabel, scheduleCount, scheduleRules, sourceDefinition, sourceUse);
  summaryCategories.append(summaryCategory);

  const summarySelection = {
    entityId: "schedule-section",
    occurrenceId: "schedule-section-occurrence",
    viewTarget: summaryCategoryTarget,
    originView: "profile",
    semanticPathHint: "schedules",
    relatedEntityIds: [],
  };
  const summaryPrimary = adapters.refreshResultPanelSelectionStyles("summary", summarySelection, null);
  assert(summaryPrimary === 1, "Summary category refresh returned more than one primary");
  assertExclusivePrimary(summaryCategories, summaryCategory, [scheduleCount, scheduleRules], "Summary category rows");

  const sourceSelection = {
    entityId: "schedule-entity",
    occurrenceId: "schedule-definition",
    originView: "input-semantic",
    semanticPathHint: "schedules/definitions/Office Schedule",
    relatedEntityIds: [],
  };
  const sourcePrimary = adapters.refreshResultPanelSelectionStyles("summary", sourceSelection, null);
  assert(sourcePrimary === 1, "Summary source refresh returned more than one primary");
  assertExclusivePrimary(summaryCategories, sourceDefinition, [sourceUse], "Summary source duplicate occurrences");
  const maxCurrentLocations = Math.max(
    currentLocations(profilePane).length,
    currentLocations(summaryCategories).length,
  );

  state.globalSelection = sourceSelection;
  state.keyboardShortcuts = { ...state.keyboardShortcuts, availableViews: "Alt+Enter" };
  sourceDefinition.focus();
  const altEnter = new KeyboardEvent("keydown", {
    key: "Enter",
    altKey: true,
    bubbles: true,
    cancelable: true,
  });
  sourceDefinition.dispatchEvent(altEnter);
  await new Promise((resolve) => window.requestAnimationFrame(resolve));
  const palette = document.querySelector(".navigation-command-palette");
  const altEnterPalette = Boolean(altEnter.defaultPrevented && palette?.open && palette.querySelector('[data-command-id="input-semantic"]'));
  assert(altEnterPalette, "Alt+Enter on an analysis row did not reach the available-views shortcut");

  return {
    profilePrimary,
    summaryPrimary,
    sourcePrimary,
    maxCurrentLocations,
    altEnterPalette,
  };
}

const resultElement = document.createElement("pre");
resultElement.id = "navigationSelectionUXResult";
document.body.append(resultElement);
try {
  const result = await runNavigationSelectionUXHarness();
  document.body.dataset.navigationSelectionUxStatus = "passed";
  resultElement.textContent = JSON.stringify(result);
} catch (error) {
  document.body.dataset.navigationSelectionUxStatus = "failed";
  resultElement.textContent = String(error?.stack || error);
}
</script>`
