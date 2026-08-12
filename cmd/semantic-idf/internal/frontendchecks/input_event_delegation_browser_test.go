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

func TestInputViewsKeepConstantEventListenerCountsAcrossRenders(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser input event delegation harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/input-event-delegation", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, inputEventDelegationHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--no-sandbox",
		"--no-first-run",
		"--no-default-browser-check",
		"--virtual-time-budget=15000",
		"--window-size=1280,900",
		"--user-data-dir="+t.TempDir(),
		"--dump-dom",
		server.URL+"/input-event-delegation",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("input event delegation browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("input event delegation browser harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-input-event-delegation-status="passed"`) {
		t.Fatalf("input event delegation browser harness did not pass:\n%s", document)
	}
	for _, signal := range []string{
		`"semanticListeners":5`,
		`"jsonListeners":2`,
		`"textListeners":3`,
		`"tableListeners":4`,
		`"rerenderStable":true`,
		`"dynamicControlsWork":true`,
	} {
		if !strings.Contains(document, signal) {
			t.Fatalf("input event delegation result is missing %s:\n%s", signal, document)
		}
	}
}

const inputEventDelegationHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Input event delegation harness</title></head>
<body data-input-event-delegation-status="pending">
  <span id="inputFilterStats"></span>
  <div id="semanticEditor"></div>
  <div id="textObjectView"></div>
  <div id="jsonStructuredView"></div>
  <span id="fieldStats"></span>
  <div id="fieldTable"></div>
  <pre id="result">pending</pre>
  <script type="module">
    const assert = (condition, message) => { if (!condition) throw new Error(message); };
    const listenerCounts = new WeakMap();
    const originalAddEventListener = EventTarget.prototype.addEventListener;
    EventTarget.prototype.addEventListener = function(type, listener, options) {
      let counts = listenerCounts.get(this);
      if (!counts) {
        counts = new Map();
        listenerCounts.set(this, counts);
      }
      counts.set(type, (counts.get(type) || 0) + 1);
      return originalAddEventListener.call(this, type, listener, options);
    };
    const countListeners = (element, types) => types.reduce((total, type) => total + (listenerCounts.get(element)?.get(type) || 0), 0);

    try {
      const [{ state, elements, setDocumentText }, inputViews] = await Promise.all([
        import("/src/js/state.js"),
        import("/src/js/views/input-views.js"),
      ]);
      const sourceObject = {
        index: 0,
        sourceIndex: 0,
        type: "Version",
        name: "24.2",
        fields: [{ key: "version_identifier", comment: "Version Identifier", value: "24.2" }],
      };
      setDocumentText("Version,24.2;");
      state.report = { objects: [sourceObject] };
      state.model = { format: "idf", version: { raw: "24.2" }, objects: [sourceObject] };
      state.reportAnalyzedText = state.documentText;
      state.semanticProjection = {
        schema: "eplus-semantic/0.1",
        energyplusVersion: "24.2",
        objectCount: 1,
        lines: [],
        navigation: { entities: [], occurrences: [], byEntityId: {}, byObjectId: {}, byObjectIndex: {}, byViewTarget: {} },
      };

      const countsAfterTwoRenders = (view, root, types) => {
        state.activeInputView = view;
        inputViews.renderInputViews();
        const first = countListeners(root, types);
        inputViews.renderInputViews();
        const second = countListeners(root, types);
        assert(first === second, view + " listeners changed after rerender: " + first + " -> " + second);
        return second;
      };

      const semanticListeners = countsAfterTwoRenders("semantic", elements.semanticEditor, ["click", "dblclick", "keydown", "pointerover", "pointerout"]);
      const basicButton = elements.semanticEditor.querySelector('[data-semantic-mode="detailed"]');
      basicButton.click();
      assert(state.semanticProjectionMode === "detailed", "delegated Semantic mode button did not update state");

      const jsonListeners = countsAfterTwoRenders("json", elements.jsonStructuredView, ["click", "change"]);
      const depth = elements.jsonStructuredView.querySelector("#jsonCollapseDepth");
      depth.value = "3";
      depth.dispatchEvent(new Event("change", { bubbles: true }));
      assert(state.jsonCollapseDepth === 3, "delegated JSON depth control did not update state");
      elements.jsonStructuredView.querySelector(".json-object-summary").click();
      assert(String(state.jsonSelectedObjectIndex) === "0", "delegated JSON object selection did not update state");

      const textListeners = countsAfterTwoRenders("text", elements.textObjectView, ["click", "focusin", "focusout", "keydown"]);
      elements.textObjectView.querySelector(".text-object-head").click();

      const tableListeners = countsAfterTwoRenders("table", elements.fieldTable, ["click", "focusin", "focusout", "keydown"]);
      const orientation = elements.fieldTable.querySelector(".object-orientation-button");
      const nextOrientation = orientation.dataset.nextOrientation;
      orientation.click();
      assert(state.tableGroupOrientations.get("Version") === nextOrientation, "delegated table orientation control did not update state");

      const result = {
        semanticListeners,
        jsonListeners,
        textListeners,
        tableListeners,
        rerenderStable: true,
        dynamicControlsWork: true,
      };
      assert(semanticListeners === 5, "unexpected Semantic listener count");
      assert(jsonListeners === 2, "unexpected JSON listener count");
      assert(textListeners === 3, "unexpected Text listener count");
      assert(tableListeners === 4, "unexpected Table listener count");
      document.querySelector("#result").textContent = JSON.stringify(result);
      document.body.dataset.inputEventDelegationStatus = "passed";
    } catch (error) {
      document.querySelector("#result").textContent = String(error?.stack || error);
      document.body.dataset.inputEventDelegationStatus = "failed";
    }
  </script>
</body>
</html>`
