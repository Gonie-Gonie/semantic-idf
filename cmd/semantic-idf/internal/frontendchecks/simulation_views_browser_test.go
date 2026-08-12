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

func TestSimulationViewsBrowserModuleLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser Simulation module harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/simulation-module", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, simulationViewsModuleHarnessHTML)
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
		"--virtual-time-budget=10000",
		"--user-data-dir="+t.TempDir(),
		"--dump-dom",
		server.URL+"/simulation-module",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("Simulation module browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("Simulation module browser harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-simulation-module-status="passed"`) {
		t.Fatalf("Simulation module did not load as a native ES module:\n%s", document)
	}
}

const simulationViewsModuleHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Simulation module harness</title></head>
<body data-simulation-module-status="pending">
<pre id="result">pending</pre>
<script type="module">
try {
  const module = await import("/src/js/views/simulation-views.js");
  if (typeof module.renderSimulation !== "function" || typeof module.initializeSimulationControls !== "function") {
    throw new Error("Simulation exports are unavailable");
  }
  document.body.dataset.simulationModuleStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.simulationModuleStatus = "failed";
  document.getElementById("result").textContent = error?.stack || error?.message || String(error);
}
</script>
</body>
</html>`
