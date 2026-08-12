package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestNavigationLinkBarConciseLabelAndFocusContracts(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	for _, required := range []string{`id="workspaceBackButton"`, `>←</button>`, `id="workspaceForwardButton"`, `>→</button>`} {
		if !strings.Contains(markup, required) {
			t.Fatalf("navigation history button markup missing %q", required)
		}
	}
	if strings.Contains(markup, "??/button>") {
		t.Fatal("navigation history buttons contain malformed closing tags")
	}

	content := readTestFile(t, "frontend/src/js/navigation-link-bar.js")
	for _, required := range []string{
		"export function formatNavigationSelectionLabel",
		"originTargetLabel(selection, entity, occurrence)",
		`!originView.startsWith("input-")`,
		"labelForOriginTargetElement",
		"semanticContextParts",
		"uniqueLabelParts(parts)",
		"normalizeLabelKey",
		`element.setAttribute("aria-live", "polite")`,
		`element.setAttribute("aria-atomic", "true")`,
		"if (element.textContent !== label)",
		"const existing = new Map(",
		"existing.get(view) || createTargetButton(view)",
		"focusedButton.focus({ preventScroll: true })",
		`import { isProfileTopologyLink } from "./selection-controller.js"`,
		`!isProfileTopologyLink(state.activeResultTab, target.view)`,
		`!isProfileTopologyLink(selection.originView, target.view)`,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("navigation link-bar UX contract missing %q", required)
		}
	}
	if strings.Contains(content, "container.replaceChildren()") {
		t.Fatal("link-bar rerenders must reconcile target buttons instead of discarding keyboard focus")
	}
	legacy := regexp.MustCompile(`context\s*&&\s*context\s*!==\s*label`)
	if legacy.MatchString(content) {
		t.Fatal("link-bar labels must use normalized segment de-duplication")
	}
}

func TestNavigationLinkBarRuntimeLabelsAndFocus(t *testing.T) {
	chrome := findHeadlessChrome()
	if chrome == "" {
		t.Skip("Chrome/Chromium is unavailable for the link-bar runtime test")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/navigation-link-bar-test", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, navigationLinkBarRuntimePage)
	})
	mux.Handle("/", http.FileServer(http.Dir(repoPath("."))))
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new",
		"--disable-background-networking",
		"--disable-default-apps",
		"--disable-extensions",
		"--disable-gpu",
		"--disable-sync",
		"--no-first-run",
		"--no-default-browser-check",
		"--no-sandbox",
		"--user-data-dir="+t.TempDir(),
		"--dump-dom",
		server.URL+"/navigation-link-bar-test",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("headless link-bar test timed out: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("headless link-bar test failed: %v\n%s", err, output)
	}
	dom := string(output)
	if !strings.Contains(dom, `data-test-status="pass"`) {
		t.Fatalf("navigation link-bar runtime contract failed:\n%s", dom)
	}
	if result := regexp.MustCompile(`(?s)<pre id="result">(.*?)</pre>`).FindStringSubmatch(dom); len(result) > 1 {
		t.Logf("navigation link-bar result: %s", result[1])
	}
}

const navigationLinkBarRuntimePage = `<!doctype html>
<html><head><meta charset="utf-8"><title>navigation link-bar test</title></head>
<body data-test-status="running">
  <section id="workspaceLinkBar">
    <button id="semanticLinkedToggle" type="button"></button>
    <button id="semanticFollowToggle" type="button"></button>
    <span id="workspaceSelectionLabel"></span>
    <nav id="workspaceLinkTargets"></nav>
    <nav id="workspaceLinkMenuTargets"></nav>
    <button id="workspaceBackButton" type="button"></button>
    <button id="workspaceForwardButton" type="button"></button>
  </section>
  <section id="metricsPane">
    <div data-panel-target-id="zone-count"><span class="metrics-name"><strong>Zone count</strong></span></div>
  </section>
  <pre id="result"></pre>
  <script type="module">
    import { state } from "/frontend/src/js/state.js";
    import {
      formatNavigationSelectionLabel,
      renderNavigationLinkBar,
    } from "/frontend/src/js/navigation-link-bar.js";
    import {
      createSelectionController,
      isProfileTopologyLink,
    } from "/frontend/src/js/selection-controller.js";

    const navigation = {
      entities: [
        { id: "section:schedules", kind: "semantic-section", label: "Schedules" },
        { id: "schedule:activity", kind: "schedule", label: "ACTIVITY_SCH" },
        { id: "section:zones", kind: "semantic-section", label: "Zones" },
      ],
      occurrences: [
        { occurrenceId: "occ:schedules", entityId: "section:schedules", path: "schedules/definitions" },
        { occurrenceId: "occ:activity", entityId: "schedule:activity", path: "schedules/definitions/ACTIVITY_SCH" },
        {
          occurrenceId: "occ:zones",
          entityId: "section:zones",
          path: "zones/definitions",
          viewTargets: [
            { view: "metrics", targetId: "zones", label: "Zones" },
            { view: "profile", targetId: "profile-zone", label: "Zones" },
            { view: "topology", targetId: "topology-zone", label: "Zones" },
          ],
        },
      ],
      byEntityId: {
        "section:schedules": ["occ:schedules"],
        "schedule:activity": ["occ:activity"],
        "section:zones": ["occ:zones"],
      },
      byObjectId: {},
      byObjectIndex: {},
      byViewTarget: {
        "metrics|zones": ["occ:zones"],
        "profile|profile-zone": ["occ:zones"],
        "topology|topology-zone": ["occ:zones"],
      },
    };
    state.semanticProjection = { schema: "eplus-semantic/0.2", navigation };
    state.reportAnalysisKey = "link-bar-labels";
    state.activeInputView = "semantic";
    state.activeResultTab = "metrics";

    const isolatedController = createSelectionController({
      state: {},
      getNavigationIndex: () => navigation,
    });
    const linkedZoneSelection = {
      entityId: "section:zones",
      occurrenceId: "occ:zones",
      semanticPathHint: "zones/definitions",
    };
    const controllerBlocksProfileToTopology = isolatedController.selectionTargetsForView("topology", {
      ...linkedZoneSelection,
      originView: "profile",
    }).length === 0;
    const controllerBlocksTopologyToProfile = isolatedController.selectionTargetsForView("profile", {
      ...linkedZoneSelection,
      originView: "topology",
    }).length === 0;
    const controllerKeepsSamePanelTargets =
      isolatedController.selectionTargetsForView("profile", { ...linkedZoneSelection, originView: "profile" }).length === 1 &&
      isolatedController.selectionTargetsForView("topology", { ...linkedZoneSelection, originView: "topology" }).length === 1;
    const bilateralGuard =
      isProfileTopologyLink("profile", "topology") &&
      isProfileTopologyLink("topology", "profile") &&
      !isProfileTopologyLink("profile", "metrics");

    const schedules = formatNavigationSelectionLabel({
      entityId: "section:schedules",
      occurrenceId: "occ:schedules",
      originView: "input-semantic",
      originTargetId: "schedules_operation",
      semanticPathHint: "schedules/definitions",
    });
    const activity = formatNavigationSelectionLabel({
      entityId: "schedule:activity",
      occurrenceId: "occ:activity",
      originView: "input-semantic",
      originTargetId: "profile:schedule:ACTIVITY_SCH",
      semanticPathHint: "schedules/definitions/ACTIVITY_SCH",
    });
    state.globalSelection = {
      entityId: "section:zones",
      entityKind: "semantic-section",
      occurrenceId: "occ:zones",
      originView: "metrics",
      originTargetId: "zone-count",
      semanticPathHint: "zones/definitions",
      sourceAnchor: null,
      relatedEntityIds: [],
    };
    const metric = formatNavigationSelectionLabel(state.globalSelection);
    renderNavigationLinkBar();
    const metricsButton = document.querySelector('#workspaceLinkTargets [data-navigation-view="metrics"]');
    metricsButton.focus();
    renderNavigationLinkBar();
    const metricsButtonAfter = document.querySelector('#workspaceLinkTargets [data-navigation-view="metrics"]');
    const labelElement = document.getElementById("workspaceSelectionLabel");

    state.globalSelection = { ...state.globalSelection, originView: "profile" };
    renderNavigationLinkBar();
    const profileKeepsProfile = Boolean(document.querySelector('#workspaceLinkTargets [data-navigation-view="profile"]'));
    const profileHidesTopology = !document.querySelector('#workspaceLinkTargets [data-navigation-view="topology"]');

    state.globalSelection = { ...state.globalSelection, originView: "topology" };
    renderNavigationLinkBar();
    const topologyKeepsTopology = Boolean(document.querySelector('#workspaceLinkTargets [data-navigation-view="topology"]'));
    const topologyHidesProfile = !document.querySelector('#workspaceLinkTargets [data-navigation-view="profile"]');

    state.globalSelection = { ...state.globalSelection, originView: "input-semantic" };
    state.activeResultTab = "profile";
    renderNavigationLinkBar();
    const activeProfileHidesTopology = !document.querySelector('#workspaceLinkTargets [data-navigation-view="topology"]');
    state.activeResultTab = "topology";
    renderNavigationLinkBar();
    const activeTopologyHidesProfile = !document.querySelector('#workspaceLinkTargets [data-navigation-view="profile"]');

    state.activeResultTab = "metrics";
    state.globalSelection = { ...state.globalSelection, originView: "metrics" };
    renderNavigationLinkBar();
    const metricsButtonFinal = document.querySelector('#workspaceLinkTargets [data-navigation-view="metrics"]');

    const passed = Boolean(
      schedules === "Schedules / definitions" &&
      activity === "ACTIVITY_SCH / definitions" &&
      metric === "Zones / Zone count" &&
      metricsButton === metricsButtonAfter &&
      metricsButtonAfter === metricsButtonFinal &&
      document.activeElement === metricsButtonFinal &&
      labelElement.textContent === metric &&
      labelElement.getAttribute("role") === "status" &&
      labelElement.getAttribute("aria-live") === "polite" &&
      labelElement.getAttribute("aria-atomic") === "true" &&
      profileKeepsProfile &&
      profileHidesTopology &&
      topologyKeepsTopology &&
      topologyHidesProfile &&
      activeProfileHidesTopology &&
      activeTopologyHidesProfile &&
      controllerBlocksProfileToTopology &&
      controllerBlocksTopologyToProfile &&
      controllerKeepsSamePanelTargets &&
      bilateralGuard
    );
    document.body.dataset.testStatus = passed ? "pass" : "fail";
    document.getElementById("result").textContent = JSON.stringify({
      passed,
      schedules,
      activity,
      metric,
      sameButton: metricsButton === metricsButtonAfter && metricsButtonAfter === metricsButtonFinal,
      focusPreserved: document.activeElement === metricsButtonFinal,
      profileKeepsProfile,
      profileHidesTopology,
      topologyKeepsTopology,
      topologyHidesProfile,
      activeProfileHidesTopology,
      activeTopologyHidesProfile,
      controllerBlocksProfileToTopology,
      controllerBlocksTopologyToProfile,
      controllerKeepsSamePanelTargets,
      bilateralGuard,
    });
  </script>
</body></html>`
