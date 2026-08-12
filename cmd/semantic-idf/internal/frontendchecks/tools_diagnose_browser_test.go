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
	for _, signal := range []string{
		`"diagnostic":true`,
		`"candidate":true`,
		`"preview":true`,
		`"hydrationPreserved":true`,
		`"snapshotApplied":true`,
		`"analysisInvalidated":true`,
		`"workspaceContextRetained":true`,
		`"replacementReset":true`,
	} {
		if !strings.Contains(document, signal) {
			t.Fatalf("Tools Diagnose result is missing %s:\n%s", signal, document)
		}
	}
}

const toolsDiagnoseHarnessSetup = `<script>
document.body.dataset.toolsDiagnoseStatus = "pending";
const mainWorkspaceSnapshot = {
  schemaVersion: 3,
  text: "Version, 23.1;\n",
  textHash: "analysis-key-23",
  path: "C:/models/current.idf",
  filename: "current.idf",
  loadedText: "Version, 22.2;\n",
  savedText: "Version, 22.2;\n",
  analysisKey: "analysis-key-23",
  activeResultTab: "profile",
  activeInputView: "json",
  analysisStage: "complete",
  geometryReady: true,
  globalSelection: { entityId: "zone-a", occurrenceId: "zone-a-use", originView: "profile" },
  viewSnapshot: {
    inputView: "json",
    resultTab: "profile",
    semantic: { filter: "Zone A", scrollTop: 35 },
    panelContexts: { profile: { selectedProfileKey: "profile-a" } }
  },
  panelContexts: { profile: { selectedProfileKey: "profile-a" } },
  layout: { editorWidth: "37%", topologyDetailsHeight: "42%" },
  semanticLinkMode: false,
  semanticFollowSelection: true,
  capturedAt: "2026-08-12T00:00:00.000Z"
};
sessionStorage.setItem("idfAnalyzer.currentDocument", JSON.stringify(mainWorkspaceSnapshot));
const candidate = { key: "unused-1", ruleId: "unused_schedules", objectType: "Schedule:Compact", objectName: "Unused Schedule", reason: "Unused", risk: "safe" };
window.go = { main: { App: {
  GetSettings: async () => ({ settings: { appearance: { language: "en", theme: "system" } } }),
  GetAppInfo: async () => ({ name: "SemanticIDF", version: "test", title: "SemanticIDF test" }),
  GetSimulationEnvironment: async () => ({ weatherFolders: [], defaultWorkerCount: 1 }),
  OpenInputFile: async () => ({ canceled: false, text: "Version, 25.1;\n", filename: "replacement.idf", path: "C:/models/replacement.idf" }),
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
    const hydratedSnapshot = JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument"));
    const hydrationPreserved = JSON.stringify(hydratedSnapshot) === JSON.stringify(mainWorkspaceSnapshot);
    const diagnostic = document.querySelector("#diagnoseList").textContent.includes("E_TEST");
    const candidateVisible = document.querySelector("#diagnoseCandidates").textContent.includes("Unused Schedule");
    document.querySelector("#diagnosePreview").click();
    await waitFor(() => !document.querySelector("#diagnosePreviewPanel").hidden);
    const preview = document.querySelector("#diagnosePreviewPanel").textContent.includes("1 removals");
    document.querySelector("#diagnoseApply").click();
    await waitFor(() => JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument")).text.includes("24.2"));
    const appliedSnapshot = JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument"));
    const snapshotApplied = appliedSnapshot.text === "Version, 24.2;\n";
    const analysisInvalidated = appliedSnapshot.analysisKey === ""
      && appliedSnapshot.textHash === ""
      && appliedSnapshot.analysisStage === "idle"
      && appliedSnapshot.geometryReady === false;
    const workspaceContextRetained = appliedSnapshot.activeResultTab === "profile"
      && appliedSnapshot.activeInputView === "json"
      && appliedSnapshot.viewSnapshot?.semantic?.filter === "Zone A"
      && appliedSnapshot.viewSnapshot?.semantic?.scrollTop === 35
      && appliedSnapshot.viewSnapshot?.panelContexts?.profile?.selectedProfileKey === "profile-a"
      && appliedSnapshot.layout?.editorWidth === "37%"
      && appliedSnapshot.layout?.topologyDetailsHeight === "42%"
      && appliedSnapshot.semanticLinkMode === false;
    document.querySelector("#diagnoseSelectInput").click();
    await waitFor(() => JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument")).filename === "replacement.idf");
    const replacementSnapshot = JSON.parse(sessionStorage.getItem("idfAnalyzer.currentDocument"));
    const replacementReset = replacementSnapshot.text === "Version, 25.1;\n"
      && replacementSnapshot.loadedText === replacementSnapshot.text
      && replacementSnapshot.savedText === replacementSnapshot.text
      && replacementSnapshot.globalSelection === null
      && replacementSnapshot.viewSnapshot === null
      && Object.keys(replacementSnapshot.panelContexts || {}).length === 0;
    result.textContent = JSON.stringify({
      diagnostic,
      candidate: candidateVisible,
      preview,
      hydrationPreserved,
      snapshotApplied,
      analysisInvalidated,
      workspaceContextRetained,
      replacementReset
    });
    document.body.dataset.toolsDiagnoseStatus = diagnostic && candidateVisible && preview
      && hydrationPreserved && snapshotApplied && analysisInvalidated && workspaceContextRetained && replacementReset ? "passed" : "failed";
  })().catch((error) => {
    result.textContent = error.stack || String(error);
    document.body.dataset.toolsDiagnoseStatus = "failed";
  });
})();
</script>`
