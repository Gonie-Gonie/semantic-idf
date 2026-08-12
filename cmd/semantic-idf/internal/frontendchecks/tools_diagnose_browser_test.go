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

func TestToolsDiagnoseBrowserHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser Tools Diagnose harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	page, err := os.ReadFile(repoPath("frontend/src/tools.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := strings.Replace(string(page), `<script type="module" src="./js/tools.js"></script>`, toolsDiagnoseHarnessSetup+`<script type="module" src="/src/js/tools.js"></script>`+toolsDiagnoseHarnessAssertions, 1)

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/tools-diagnose", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, html)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check",
		"--virtual-time-budget=15000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/tools-diagnose#diagnose",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("Tools Diagnose browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("Tools Diagnose browser harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-tools-diagnose-status="passed"`) {
		t.Fatalf("Tools Diagnose browser harness did not pass:\n%s", document)
	}
	for _, signal := range []string{`"diagnostic":true`, `"candidate":true`, `"preview":true`, `"snapshotApplied":true`} {
		if !strings.Contains(document, signal) {
			t.Fatalf("Tools Diagnose result is missing %s:\n%s", signal, document)
		}
	}
}

const toolsDiagnoseHarnessSetup = `<script>
document.body.dataset.toolsDiagnoseStatus = "pending";
sessionStorage.setItem("idfAnalyzer.currentDocument", JSON.stringify({ text: "Version, 23.1;\n", filename: "current.idf", path: "C:/models/current.idf" }));
const candidate = { key: "unused-1", ruleId: "unused_schedules", objectType: "Schedule:Compact", objectName: "Unused Schedule", reason: "Unused", risk: "safe" };
window.go = { main: { App: {
  GetSettings: async () => ({ settings: { appearance: { language: "en", theme: "system" } } }),
  GetAppInfo: async () => ({ name: "SemanticIDF", version: "test", title: "SemanticIDF test" }),
  GetSimulationEnvironment: async () => ({ weatherFolders: [], defaultWorkerCount: 1 }),
  AnalyzeInputDiagnosticsText: async () => ([{ severity: "error", category: "Reference", message: "Broken reference", code: "E_TEST", objectType: "Zone", objectName: "Zone A" }]),
  ScanCleanupText: async () => ({ scan: { rules: [{ id: "unused_schedules", name: "Unused schedules", description: "Remove unused schedules", group: "Schedules", default: true, available: true }], candidates: [candidate] } }),
  PreviewCleanupText: async () => ({ text: "Version, 24.2;\n", removedCandidates: [candidate], removedCount: 1, objectCount: 1 }),
  SaveCleanupAs: async () => ({ canceled: false, filename: "current-cleaned.idf" })
} } };
</script>`

const toolsDiagnoseHarnessAssertions = `<script>
(() => {
  const result = document.createElement("pre");
  result.id = "toolsDiagnoseHarnessResult";
  document.body.append(result);
  const waitFor = (predicate, timeout = 8000) => new Promise((resolve, reject) => {
    const started = performance.now();
    const timer = setInterval(() => {
      if (predicate()) { clearInterval(timer); resolve(); }
      else if (performance.now() - started > timeout) { clearInterval(timer); reject(new Error("Timed out waiting for Tools Diagnose")); }
    }, 25);
  });
  (async () => {
    await waitFor(() => document.querySelector("#diagnoseList")?.textContent.includes("Broken reference"));
    const diagnostic = document.querySelector("#diagnoseList").textContent.includes("E_TEST");
    const candidateVisible = document.querySelector("#diagnoseCandidates").textContent.includes("Unused Schedule");
    document.querySelector("#diagnosePreview").click();
    await waitFor(() => !document.querySelector("#diagnosePreviewPanel").hidden);
    const preview = document.querySelector("#diagnosePreviewPanel").textContent.includes("1 removals");
    document.querySelector("#diagnoseApply").click();
    await waitFor(() => JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument")).text.includes("24.2"));
    const snapshotApplied = JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument")).text === "Version, 24.2;\n";
    result.textContent = JSON.stringify({ diagnostic, candidate: candidateVisible, preview, snapshotApplied });
    document.body.dataset.toolsDiagnoseStatus = diagnostic && candidateVisible && preview && snapshotApplied ? "passed" : "failed";
  })().catch((error) => {
    result.textContent = error.stack || String(error);
    document.body.dataset.toolsDiagnoseStatus = "failed";
  });
})();
</script>`
