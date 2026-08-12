package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestPhaseHNavigationBrowserHarness runs the controller and history modules as
// native ES modules. Static source checks are useful for architecture rules,
// but this catches regressions in the actual async transaction and Map lookup
// behavior that only appears once the modules execute together.
func TestPhaseHNavigationBrowserHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser acceptance harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	frontendRoot := repoPath("frontend/src")
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(frontendRoot))))
	mux.HandleFunc("/phase-h-harness", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, phaseHBrowserHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	profileDir := t.TempDir()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--no-sandbox",
		"--no-first-run",
		"--no-default-browser-check",
		"--virtual-time-budget=30000",
		"--user-data-dir="+profileDir,
		"--dump-dom",
		server.URL+"/phase-h-harness",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("headless browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("headless browser harness failed: %v\n%s", err, output)
	}
	page := string(output)
	if !strings.Contains(page, `data-phase-h-status="passed"`) {
		t.Fatalf("headless browser harness did not pass:\n%s", page)
	}
	for _, signal := range []string{
		`"largeIndexSize":10000`,
		`"crossPaneNavigations":100`,
		`"backendCalls":0`,
		`"historyDepth":80`,
		`"selectionListenerAdds":2`,
	} {
		if !strings.Contains(page, signal) {
			t.Fatalf("headless browser result is missing %s:\n%s", signal, page)
		}
	}
}

func phaseHChromeExecutable() string {
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome", "msedge"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	if runtime.GOOS != "windows" {
		return ""
	}
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

const phaseHBrowserHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Phase H navigation harness</title></head>
<body data-phase-h-status="pending"><pre id="phaseHResult">pending</pre>
<script type="module">
const resultElement = document.getElementById("phaseHResult");
const assert = (condition, message) => {
  if (!condition) {
    throw new Error(message);
  }
};

async function runPhaseHHarness() {
  let backendCalls = 0;
  window.go = {
    main: {
      App: new Proxy({}, {
        get() {
          backendCalls += 1;
          return async () => {
            backendCalls += 1;
            throw new Error("navigation must not call the backend");
          };
        },
      }),
    },
  };

  const cacheModule = await import("/src/js/semantic-navigation-cache.js");
  const controllerModule = await import("/src/js/selection-controller.js");
  const historyModule = await import("/src/js/view-history.js");
  const stateModule = await import("/src/js/state.js");

  cacheModule.resetSemanticNavigationCacheForTests();
  const largeIndexSize = 10_000;
  const entities = new Array(largeIndexSize);
  const occurrences = new Array(largeIndexSize);
  const byEntityId = Object.create(null);
  const byObjectId = Object.create(null);
  const byObjectIndex = Object.create(null);
  const byViewTarget = Object.create(null);
  for (let index = 0; index < largeIndexSize; index += 1) {
    const entityId = "entity-" + index;
    const occurrenceId = "occurrence-" + index;
    const objectId = "object-" + index;
    const targetId = "profile-target-" + index;
    const target = { view: "profile", targetKind: "profile-item", targetId, priority: 100 };
    const sourceAnchor = {
      objectId,
      objectIndex: index,
      objectType: "Synthetic:Object",
      objectName: "Synthetic Object " + index,
      fieldIndex: 0,
      fieldName: "Name",
    };
    entities[index] = {
      id: entityId,
      kind: "profile-item",
      label: "Entity " + index,
      occurrenceIds: [occurrenceId],
      sourceAnchors: [sourceAnchor],
      viewTargets: [target],
    };
    occurrences[index] = {
      occurrenceId,
      entityId,
      path: "zones/Zone " + index + "/profiles/item",
      contextKind: "zone_profile",
      preferredView: "profile",
      preferredTargetId: targetId,
      sourceAnchor,
      viewTargets: [target],
      lineIndexes: [index + 300],
    };
    byEntityId[entityId] = [occurrenceId];
    byObjectId[objectId] = [occurrenceId];
    byObjectIndex[index] = [occurrenceId];
    byViewTarget["profile|" + targetId] = [occurrenceId];
  }
  const navigation = { entities, occurrences, byEntityId, byObjectId, byObjectIndex, byViewTarget };
  const cache = cacheModule.getSemanticNavigationCache(navigation, {
    textHash: "phase-h-large-index",
    analyzerVersion: "test",
    schemaVersion: "eplus-semantic/0.2",
  });
  assert(cache.entityById.size === largeIndexSize, "large entity Map size changed");
  assert(cache.occurrenceById.size === largeIndexSize, "large occurrence Map size changed");
  assert(cache.entity("entity-9999") === entities[9999], "last entity direct lookup failed");
  assert(cache.occurrence("occurrence-9999") === occurrences[9999], "last occurrence direct lookup failed");
  assert(cache.occurrenceIDs("object-index", 9999)[0] === "occurrence-9999", "object reverse lookup failed");
  assert(cache.occurrenceIDs("view-target", "profile|profile-target-9999")[0] === "occurrence-9999", "view reverse lookup failed");
  assert(cache.sourceIdentityCandidates(occurrences[9999].sourceAnchor)[0] === occurrences[9999], "source fallback index failed");
  const cachedAgain = cacheModule.getSemanticNavigationCache(navigation, {
    textHash: "phase-h-large-index",
    analyzerVersion: "test",
    schemaVersion: "eplus-semantic/0.2",
  });
  assert(cachedAgain === cache, "navigation cache identity reuse failed");
  const cacheStats = cacheModule.getSemanticNavigationCacheStats();
  assert(cacheStats.builds === 1 && cacheStats.identityHits >= 1, "navigation cache rebuilt unexpectedly");

  const appState = stateModule.state;
  appState.navigationUndoStack = [];
  appState.navigationRedoStack = [];
  appState.navigationRestoring = false;
  for (let index = 0; index < 100; index += 1) {
    historyModule.recordViewHistory({ sequence: index, selection: "entity-" + index });
  }
  assert(appState.navigationUndoStack.length === 80, "history must retain exactly 80 snapshots");
  assert(appState.navigationUndoStack[0].sequence === 20, "history did not evict its oldest snapshot");
  assert(appState.navigationUndoStack[79].sequence === 99, "history lost its newest snapshot");

  let queueAnalysisCalls = 0;
  let recordHistoryCalls = 0;
  let selectionChanges = 0;
  let semanticReveals = 0;
  let profileReveals = 0;
  let openedViews = 0;
  const semanticAdapter = {
    canReveal: () => true,
    reveal: () => { semanticReveals += 1; return true; },
    selectFromElement: () => null,
    captureContext: () => ({}),
    restoreContext: () => true,
    preferredSemanticOccurrence: (selection) => selection.occurrenceId || "",
  };
  const profileAdapter = {
    canReveal: () => true,
    reveal: () => { profileReveals += 1; return true; },
    selectFromElement: () => null,
    captureContext: () => ({}),
    restoreContext: () => true,
    preferredSemanticOccurrence: (selection) => selection.occurrenceId || "",
  };
  const controllerState = {
    semanticLinkMode: true,
    semanticFollowSelection: true,
    reportAnalysisKey: "phase-h-large-index",
  };
  const controller = controllerModule.createSelectionController({
    state: controllerState,
    getNavigationIndex: () => navigation,
    getReportAnalysisKey: () => "phase-h-large-index",
    isAnalysisCurrent: () => true,
    getActivePanelView: () => "profile",
    getActiveInputView: () => "input-semantic",
    getPanelNavigationAdapter: (view) => view === "input-semantic" ? semanticAdapter : view === "profile" ? profileAdapter : null,
    recordHistory: () => { recordHistoryCalls += 1; },
    onSelectionChange: () => { selectionChanges += 1; },
    openView: () => { openedViews += 1; },
    queueAnalysisTarget: () => { queueAnalysisCalls += 1; },
  });
  for (let index = 0; index < 100; index += 1) {
    const selected = await controller.selectSemanticEntity({
      entityId: "entity-" + index,
      occurrenceId: "occurrence-" + index,
      originView: "profile",
    }, {
      originView: "profile",
      transactionId: "cross-pane-" + index,
    });
    assert(selected?.entityId === "entity-" + index, "cross-pane selection failed at " + index);
  }
  assert(selectionChanges === 100, "100 selections must produce 100 state changes");
  assert(semanticReveals === 100, "100 linked result selections must reveal Semantic exactly 100 times");
  assert(recordHistoryCalls === 100, "100 committed selections must record one history item each");
  assert(queueAnalysisCalls === 0, "current-report navigation queued analysis");
  assert(backendCalls === 0, "current-report navigation called a backend binding");

  controllerState.semanticFollowSelection = false;
  await controller.selectSemanticEntity({ entityId: "entity-101", occurrenceId: "occurrence-101" }, {
    originView: "profile",
    transactionId: "follow-off",
  });
  assert(semanticReveals === 100, "Follow OFF still revealed another pane");

  const historyBeforeHover = recordHistoryCalls;
  await controller.hoverSemanticEntity({ entityId: "entity-102", occurrenceId: "occurrence-102" }, {
    originView: "profile",
    transactionId: "hover-no-history",
  });
  assert(recordHistoryCalls === historyBeforeHover, "hover recorded history");

  const changesBeforeRepeatedTransaction = selectionChanges;
  await controller.selectSemanticEntity({ entityId: "entity-103", occurrenceId: "occurrence-103" }, {
    originView: "profile",
    follow: false,
    transactionId: "repeated-transaction",
  });
  const repeated = await controller.selectSemanticEntity({ entityId: "entity-104", occurrenceId: "occurrence-104" }, {
    originView: "profile",
    follow: false,
    transactionId: "repeated-transaction",
  });
  assert(repeated === null, "replayed transaction was not suppressed");
  assert(selectionChanges === changesBeforeRepeatedTransaction + 1, "replayed transaction changed global state twice");

  controllerState.semanticFollowSelection = true;
  await controller.selectSemanticEntity({ entityId: "entity-105", occurrenceId: "occurrence-105" }, {
    originView: "input-semantic",
    transactionId: "compatible-panel-reveal",
  });
  assert(profileReveals === 1, "compatible current panel was not revealed");
  const opened = await controller.openSelectionInView("profile", {
    targetId: "profile-target-105",
    transactionId: "preferred-view-open",
  });
  assert(opened === true && openedViews === 1 && profileReveals === 2, "preferred view target did not open exactly once");

  let reportCurrent = false;
  let pendingQueueCalls = 0;
  let pendingHistoryCalls = 0;
  let pendingReveals = 0;
  const pendingAdapter = { ...semanticAdapter, reveal: () => { pendingReveals += 1; return true; } };
  const pendingState = { semanticLinkMode: true, semanticFollowSelection: true, reportAnalysisKey: "phase-h-large-index" };
  const pendingController = controllerModule.createSelectionController({
    state: pendingState,
    getNavigationIndex: () => navigation,
    getReportAnalysisKey: () => "phase-h-large-index",
    isAnalysisCurrent: () => reportCurrent,
    getPanelNavigationAdapter: (view) => view === "input-semantic" ? pendingAdapter : null,
    recordHistory: () => { pendingHistoryCalls += 1; },
    queueAnalysisTarget: () => { pendingQueueCalls += 1; },
  });
  await pendingController.selectSemanticEntity({ entityId: "entity-200", occurrenceId: "occurrence-200" }, {
    originView: "profile",
    transactionId: "stale-select",
  });
  assert(pendingQueueCalls === 1 && pendingController.pendingSemanticNavigation(), "stale navigation target was not queued once");
  reportCurrent = true;
  const resumed = await pendingController.resumePendingSemanticNavigation({ transactionId: "stale-resume" });
  assert(resumed === true && pendingReveals === 1, "pending target was not restored after analysis became current");
  assert(pendingController.pendingSemanticNavigation() === null, "pending target was not consumed exactly once");
  assert(pendingHistoryCalls === 1, "pending restore duplicated committed-selection history");

  const originalAddEventListener = EventTarget.prototype.addEventListener;
  let selectionListenerAdds = 0;
  EventTarget.prototype.addEventListener = function(type, listener, options) {
    if (this === window && (type === "idfAnalyzer:semanticSelectionChanged" || type === "idfAnalyzer:semanticHoverChanged")) {
      selectionListenerAdds += 1;
    }
    return originalAddEventListener.call(this, type, listener, options);
  };
  try {
    const adaptersModule = await import("/src/js/panel-navigation-adapters.js?phase-h-listeners");
    for (let index = 0; index < 100; index += 1) {
      adaptersModule.initializeResultPanelNavigationAdapters();
    }
  } finally {
    EventTarget.prototype.addEventListener = originalAddEventListener;
  }
  assert(selectionListenerAdds === 2, "repeated adapter initialization grew detached/global listeners");

  return {
    largeIndexSize,
    crossPaneNavigations: 100,
    backendCalls,
    historyDepth: appState.navigationUndoStack.length,
    selectionListenerAdds,
    cacheBuilds: cacheStats.builds,
  };
}

try {
  const result = await runPhaseHHarness();
  document.body.dataset.phaseHStatus = "passed";
  resultElement.textContent = JSON.stringify(result);
} catch (error) {
  document.body.dataset.phaseHStatus = "failed";
  resultElement.textContent = String(error?.stack || error);
}
</script>
</body>
</html>`
