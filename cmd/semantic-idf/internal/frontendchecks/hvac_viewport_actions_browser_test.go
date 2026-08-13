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

func TestHVACViewportActionsBrowserHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser HVAC viewport action harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	index := readTestFile(t, "frontend/src/index.html")
	hvacMain := sliceBetween(index, `<section class="hvac-main"`, `<aside id="hvacSide"`)
	if !strings.Contains(hvacMain, `id="hvacViewportActions"`) || !strings.Contains(hvacMain, `id="hvacGraph"`) {
		t.Fatal("HVAC main viewport fragment is missing its fixed actions or graph")
	}
	harness := strings.Replace(hvacViewportActionsHarnessHTML, "{{HVAC_MAIN}}", hvacMain, 1)

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/hvac-viewport-actions", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, harness)
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
		server.URL+"/hvac-viewport-actions",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("HVAC viewport action harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("HVAC viewport action harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-hvac-viewport-status="passed"`) {
		t.Fatalf("HVAC viewport action harness did not pass:\n%s", document)
	}
	for _, signal := range []string{
		`"toolbarCount":6`,
		`"iconOnly":true`,
		`"fitIcon":true`,
		`"horizontal":true`,
		`"upperRight":true`,
		`"responsiveWidths":true`,
		`"noTextNavigationRow":true`,
		`"initialDisabled":true`,
		`"focusedState":true`,
		`"backState":true`,
		`"forwardState":true`,
		`"clearState":true`,
		`"rootState":true`,
		`"documentResetState":true`,
	} {
		if !strings.Contains(document, signal) {
			t.Fatalf("HVAC viewport action result is missing %s:\n%s", signal, document)
		}
	}
}

const hvacViewportActionsHarnessHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>HVAC viewport actions harness</title>
  <link rel="stylesheet" href="/src/styles/base.css">
  <link rel="stylesheet" href="/src/styles/hvac.css">
  <style>
    body { margin: 0; padding: 24px; }
    .hvac-layout { display: block; width: 760px; height: 360px; }
    .hvac-main { width: 760px; height: 360px; }
  </style>
</head>
<body data-hvac-viewport-status="pending">
  <span id="runtimeStatus"></span>
  <div id="hvacSummary"></div>
  <div class="hvac-layout">
    {{HVAC_MAIN}}
    <aside id="hvacSide">
      <span id="hvacInspectorStats"></span>
      <div id="hvacInspector"></div>
    </aside>
  </div>
  <pre id="result">pending</pre>
  <script type="module">
    const resultElement = document.getElementById("result");
    const assert = (condition, message) => {
      if (!condition) throw new Error(message);
    };
    try {
      const stateModule = await import("/src/js/state.js");
      const view = await import("/src/js/views/hvac-views.js");
      const state = stateModule.state;
      const hvac = {
        loops: [],
        airLoopCount: 0,
        plantLoopCount: 0,
        serviceModel: { zoneServices: [], components: [], couplings: [], networks: [] },
      };
      state.report = { hvac };
      state.activeResultTab = "hvac";
      state.activeHVACView = "services";
      state.activeHVACLoopId = "";
      state.activeHVACGraphKey = "";
      state.activeHVACNodeName = "";
      state.activeHVACEntity = { id: "", kind: "", label: "" };
      state.activeHVACContext = { pathId: "", zoneId: "", loopId: "", componentId: "", couplingId: "", previousView: "" };
      state.hvacNavigationStack = [];
      state.hvacForwardStack = [];

      view.initializeHVACControls();
      view.renderHVAC(hvac);

      const toolbar = document.getElementById("hvacViewportActions");
      const main = document.querySelector(".hvac-main");
      const buttons = [...toolbar.querySelectorAll(":scope > .viewport-icon-button")];
      const toolbarCount = buttons.length;
      const iconOnly = buttons.every((button) =>
        Boolean(button.querySelector("svg.viewport-icon[aria-hidden='true']"))
        && Boolean(button.querySelector(".sr-only"))
        && button.title.length > 0
        && button.getAttribute("aria-label") === button.title
        && [...button.childNodes].filter((node) => node.nodeType === Node.TEXT_NODE).every((node) => !node.textContent.trim())
      );
      const rects = buttons.map((button) => button.getBoundingClientRect());
      const horizontal = getComputedStyle(toolbar).flexWrap === "nowrap"
        && rects.every((rect) => Math.abs(rect.top - rects[0].top) < 1)
        && rects.slice(1).every((rect, index) => rect.left > rects[index].left);
      const toolbarRect = toolbar.getBoundingClientRect();
      const mainRect = main.getBoundingClientRect();
      const upperRight = Math.abs(mainRect.right - toolbarRect.right) <= 22
        && Math.abs(toolbarRect.top - mainRect.top) <= 12;
      const fit = document.getElementById("hvacFitButton");
      const fitIcon = Boolean(fit)
        && fit.getAttribute("aria-label") === "Fit"
        && fit.title === "Fit"
        && Boolean(fit.querySelector("svg.viewport-icon[aria-hidden='true']"))
        && Boolean(fit.querySelector(".sr-only"));

      const layout = document.querySelector(".hvac-layout");
      const responsiveResults = [];
      for (const width of [160, 240, 360]) {
        layout.style.width = width + "px";
        main.style.width = width + "px";
        void main.offsetWidth;
        const narrowMainRect = main.getBoundingClientRect();
        const narrowToolbarRect = toolbar.getBoundingClientRect();
        const narrowRects = buttons.map((button) => button.getBoundingClientRect());
        responsiveResults.push(
          getComputedStyle(toolbar).flexWrap === "nowrap"
          && narrowRects.every((rect) => Math.abs(rect.top - narrowRects[0].top) < 1)
          && narrowRects.slice(1).every((rect, index) => rect.left > narrowRects[index].left)
          && narrowRects.every((rect) => rect.left >= narrowMainRect.left - 1 && rect.right <= narrowMainRect.right + 1)
          && narrowToolbarRect.left >= narrowMainRect.left - 1
          && narrowToolbarRect.right <= narrowMainRect.right + 1
        );
      }
      const responsiveWidths = responsiveResults.every(Boolean);
      layout.style.width = "760px";
      main.style.width = "760px";
      const noTextNavigationRow = !document.querySelector("#hvacSummary .hvac-breadcrumb-bar, #hvacSummary .hvac-history-actions, #hvacSummary .hvac-breadcrumb, #hvacGraph [data-hvac-nav-action], #hvacGraph [data-hvac-open-view='services']");

      const back = document.getElementById("hvacBackButton");
      const forward = document.getElementById("hvacForwardButton");
      const clear = document.getElementById("hvacClearFocusButton");
      const root = document.getElementById("hvacZoneServicesButton");
      const initialDisabled = back.disabled && forward.disabled && clear.disabled && root.disabled;

      view.navigateHVAC({ kind: "zone", id: "zone:a", label: "Zone A", view: "services", graphKey: "subject:zone:a" }, { pushHistory: true });
      const focusedState = !back.disabled && forward.disabled && !clear.disabled && !root.disabled
        && state.activeHVACEntity.id === "zone:a";

      back.click();
      const backState = back.disabled && !forward.disabled && clear.disabled && root.disabled
        && state.activeHVACEntity.id === "";

      forward.click();
      const forwardState = !back.disabled && forward.disabled && !clear.disabled && !root.disabled
        && state.activeHVACEntity.id === "zone:a";

      clear.click();
      const clearState = !back.disabled && forward.disabled && clear.disabled && root.disabled
        && state.activeHVACEntity.id === "" && state.activeHVACGraphKey === "";

      view.navigateHVAC({ kind: "zone", id: "zone:a", label: "Zone A", view: "services", graphKey: "subject:zone:a" }, { pushHistory: true });
      root.click();
      const rootState = !back.disabled && forward.disabled && clear.disabled && root.disabled
        && state.activeHVACView === "services" && state.activeHVACEntity.id === "" && state.activeHVACGraphKey === "";

      window.dispatchEvent(new Event("idfAnalyzer:documentChanged"));
      const documentResetState = back.disabled && forward.disabled && clear.disabled && root.disabled
        && state.hvacNavigationStack.length === 0 && state.hvacForwardStack.length === 0
        && state.activeHVACView === "services" && state.activeHVACEntity.id === "" && state.activeHVACGraphKey === "";

      const result = { toolbarCount, iconOnly, fitIcon, horizontal, upperRight, responsiveWidths, noTextNavigationRow, initialDisabled, focusedState, backState, forwardState, clearState, rootState, documentResetState };
      Object.entries(result).forEach(([key, value]) => assert(value === true || (key === "toolbarCount" && value === 6), key + " contract failed"));
      resultElement.textContent = JSON.stringify(result);
      document.body.dataset.hvacViewportStatus = "passed";
    } catch (error) {
      resultElement.textContent = error.stack || String(error);
      document.body.dataset.hvacViewportStatus = "failed";
    }
  </script>
</body>
</html>`
