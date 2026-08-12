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

func TestBatchSummaryUtilitiesBrowserHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser batch summary harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/batch-summary", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, batchSummaryHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check",
		"--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/batch-summary",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("batch summary browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("batch summary browser harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-batch-summary-status="passed"`) {
		t.Fatalf("batch summary browser harness did not pass:\n%s", document)
	}
	for _, signal := range []string{`"scientific":100`, `"decimalUnit":"W"`, `"scientificUnit":"kWh"`, `"explicitUnit":"W/m2"`, `"invalid":false`} {
		if !strings.Contains(document, signal) {
			t.Fatalf("batch summary browser result is missing %s:\n%s", signal, document)
		}
	}
}

const batchSummaryHarnessHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Batch summary harness</title></head>
<body data-batch-summary-status="pending"><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
try {
  const summary = await import("/src/js/batch/batch-summary-utils.js");
  const scientific = summary.parseSummaryNumber("1.00e+2 kWh");
  const decimalUnit = summary.summaryUnit({}, "1.00 W");
  const scientificUnit = summary.summaryUnit({}, "1.00e+2 kWh");
  const explicitUnit = summary.summaryUnit({ unit: "W/m2" }, "1.00 ignored");
  const invalid = summary.parseSummaryNumber("not available").ok;
  assert(scientific.ok && scientific.value === 100 && scientific.token === "1.00e+2", "scientific value parsing changed");
  assert(decimalUnit === "W", "decimal formatting leaked into the inferred unit");
  assert(scientificUnit === "kWh", "scientific notation leaked into the inferred unit");
  assert(explicitUnit === "W/m2", "explicit metric unit lost precedence");
  assert(!invalid, "invalid summary value parsed as numeric");
  document.getElementById("result").textContent = JSON.stringify({ scientific: scientific.value, decimalUnit, scientificUnit, explicitUnit, invalid });
  document.body.dataset.batchSummaryStatus = "passed";
} catch (error) {
  document.getElementById("result").textContent = error.stack || String(error);
  document.body.dataset.batchSummaryStatus = "failed";
}
</script></body></html>`
