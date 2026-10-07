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
		`"localePreserved":true`,
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

func TestToolsDiagnoseAuxiliaryHandoffBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser auxiliary Diagnose harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/tools-diagnose-auxiliary", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, toolsDiagnoseAuxiliaryHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check",
		"--virtual-time-budget=20000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/tools-diagnose-auxiliary",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("auxiliary Diagnose browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("auxiliary Diagnose browser harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-auxiliary-diagnose-status="passed"`) {
		t.Fatalf("auxiliary Diagnose browser harness did not pass:\n%s", document)
	}
	for _, signal := range []string{
		`"initialFromMain":true`, `"dormantScan":true`, `"parentBridge":true`,
		`"cachedSelection":true`, `"explicitApply":true`, `"snapshotOwnedByMain":true`,
		`"lateApplyProtected":true`, `"hostConflictProtected":true`, `"latePickerProtected":true`,
		`"replacementApplied":true`, `"localizedConflict":true`,
	} {
		if !strings.Contains(document, signal) {
			t.Fatalf("auxiliary Diagnose browser result is missing %s:\n%s", signal, document)
		}
	}
}

const toolsDiagnoseAuxiliaryHarnessHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Auxiliary Diagnose handoff</title></head>
<body data-auxiliary-diagnose-status="pending"><pre id="result">pending</pre>
<script>
const currentInput = (version, filename = "current.idf") => ({ text: "Version, " + version + ";\n", path: "C:/models/" + filename, filename });
const harness = {
  current: currentInput("23.1"), calls: [], diagnosticCalls: [], scanCalls: [], listeners: [],
  deferPreview: false, resolvePreview: null, deferPicker: false, resolvePicker: null,
};
const staleSnapshot = JSON.stringify({ schemaVersion: 4, ...currentInput("20.1", "stale.idf"), analysisKey: "stale-key" });
sessionStorage.setItem("idfAnalyzer.currentDocument", staleSnapshot);
const candidate = { key: "unused-1", ruleId: "unused_schedules", objectType: "Schedule:Compact", objectName: "Unused Schedule", reason: "Unused", risk: "safe" };
const previewFor = text => ({ text, removedCandidates: [candidate], removedCount: 1, objectCount: 1 });
window.runtime = { EventsOn: (name, callback) => { harness.listeners.push({ name, callback }); return () => {}; } };
window.go = { main: { App: {
  GetSettings: async () => ({ settings: { appearance: { language: "en", theme: "system" } } }),
  GetAppInfo: async () => ({ name: "SemanticIDF", version: "test", title: "SemanticIDF test" }),
  GetSimulationEnvironment: async () => ({ weatherFolders: [], defaultWorkerCount: 1 }),
  SelectSimulationInputFiles: async () => ({ paths: ["C:/models/batch.idf"] }),
  AnalyzeInputDiagnosticsText: async text => {
    harness.diagnosticCalls.push(text);
    return [{ severity: "warning", category: "Input", message: "Diagnostics " + text.trim(), code: "E_TEST" }];
  },
  ScanCleanupText: async (text, path, filename) => {
    harness.scanCalls.push({ text, path, filename });
    return { scan: { rules: [{ id: "unused_schedules", name: "Unused schedules", group: "Schedules", default: true, available: true }], candidates: [candidate] } };
  },
  PreviewCleanupText: async () => harness.deferPreview
    ? new Promise(resolve => { harness.resolvePreview = resolve; })
    : previewFor("Version, 24.2;\n"),
  OpenInputFile: async () => harness.deferPicker
    ? new Promise(resolve => { harness.resolvePicker = resolve; })
    : { canceled: false, ...currentInput("30.1", "replacement.idf") },
} } };
</script>
<script type="module">
import { initializeAuxiliaryPanel, openAuxiliaryPage, closeAuxiliaryPanel } from "/src/js/auxiliary-panel.js";
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const waitFor = (predicate, message, timeout = 8000) => new Promise((resolve, reject) => {
  const started = performance.now();
  const timer = setInterval(() => {
    if (predicate()) { clearInterval(timer); resolve(); }
    else if (performance.now() - started > timeout) { clearInterval(timer); reject(new Error("Timed out: " + message)); }
  }, 25);
});
try {
  initializeAuxiliaryPanel({
    getDocument: () => ({ ...harness.current }),
    applyDocument: (doc, options) => { harness.calls.push({ doc: { ...doc }, options }); harness.current = { ...doc }; return true; },
  });
  openAuxiliaryPage("/src/tools.html#batch-metrics");
  await waitFor(() => document.querySelector("iframe")?.contentDocument?.querySelector("#diagnoseFilename")?.title === harness.current.path, "initial live Main input");
  const frame = document.querySelector("iframe");
  const child = frame.contentWindow;
  const element = selector => child.document.querySelector(selector);
  const dormantScan = harness.scanCalls.length === 0 && harness.diagnosticCalls.length === 0;
  const initialFromMain = element("#diagnoseFilename").textContent === "current.idf";
  const parentBridge = child.go === window.go && child.runtime === window.runtime && harness.listeners.length === 2;
  assert(dormantScan && initialFromMain && parentBridge, "initial source or native parent bridge changed");

  element("#multiSimulationSelectFiles").click();
  await waitFor(() => element("#multiSimulationFiles").textContent.includes("batch.idf"), "batch file selection");
  const selectedFiles = element("#multiSimulationFiles").innerHTML;
  closeAuxiliaryPanel();
  harness.current = currentInput("25.1", "reopened.idf");
  openAuxiliaryPage("/src/tools.html");
  const cachedSelection = document.querySelector("iframe").contentWindow === child
    && element("#multiSimulationFiles").innerHTML === selectedFiles
    && element("#diagnoseFilename").title === harness.current.path
    && harness.scanCalls.length === 0;
  assert(cachedSelection, "reopening lost batch selection or analyzed dormant Diagnose");
  element('[data-tools-tab="diagnose"]').click();
  await waitFor(() => !element("#diagnoseApply").disabled, "current input cleanup scan");
  assert(harness.scanCalls[0].text === harness.current.text && harness.scanCalls[0].path === harness.current.path, "scan mixed text and path from different inputs");
  element("#diagnoseApply").click();
  await waitFor(() => harness.calls.length === 1 && !element("#diagnoseApply").disabled, "explicit cleanup handoff");
  const explicitApply = harness.current.text === "Version, 24.2;\n"
    && harness.calls[0].options.replaceWorkspace === false
    && harness.calls[0].options.expected.text === "Version, 25.1;\n";
  const snapshotOwnedByMain = sessionStorage.getItem("idfAnalyzer.currentDocument") === staleSnapshot;
  assert(explicitApply && snapshotOwnedByMain, "cleanup failed to hand off to Main or rewrote Main's snapshot");

  harness.deferPreview = true;
  element("#diagnoseApply").click();
  await waitFor(() => harness.resolvePreview, "delayed Apply preview");
  const oldResolve = harness.resolvePreview;
  harness.resolvePreview = null;
  closeAuxiliaryPanel();
  harness.current = currentInput("26.1", "changed-while-hidden.idf");
  openAuxiliaryPage("/src/tools.html");
  oldResolve(previewFor("Version, old-result;\n"));
  await waitFor(() => element("#diagnoseStatus").dataset.i18n === "diagnoseFix.workspaceConflict", "late Apply conflict");
  const lateApplyProtected = harness.calls.length === 1 && harness.current.filename === "changed-while-hidden.idf"
    && element("#diagnoseFilename").title === harness.current.path;
  assert(lateApplyProtected, "late Apply overwrote Main after cached frame reopening");

  element("#diagnoseRefresh").click();
  await waitFor(() => !element("#diagnoseApply").disabled, "scan after rejected Apply");
  element("#diagnoseApply").click();
  await waitFor(() => harness.resolvePreview, "Apply before hidden Main edit");
  const conflictResolve = harness.resolvePreview;
  harness.resolvePreview = null;
  closeAuxiliaryPanel();
  harness.current = currentInput("27.1", "host-conflict.idf");
  conflictResolve(previewFor("Version, stale-without-reopen;\n"));
  await waitFor(() => element("#diagnoseFilename").title === harness.current.path, "host rejects stale baseline");
  const hostConflictProtected = harness.calls.length === 1 && harness.current.filename === "host-conflict.idf"
    && element("#diagnoseStatus").dataset.i18n === "diagnoseFix.workspaceConflict";
  const setChildLanguage = language => {
    const script = child.document.createElement("script");
    script.type = "module";
    script.textContent = 'import { setLanguage } from "/src/js/i18n.js"; setLanguage(' + JSON.stringify(language) + ');';
    child.document.body.appendChild(script);
  };
  setChildLanguage("ko");
  await waitFor(() => element("#diagnoseStatus").textContent.includes("Main의 입력이 변경되었습니다"), "localized conflict status");
  const localizedConflict = element("#diagnoseStatus").textContent.includes("Main의 입력이 변경되었습니다");
  setChildLanguage("en");
  assert(hostConflictProtected && localizedConflict, "host guard or conflict localization changed");

  openAuxiliaryPage("/src/tools.html");
  harness.deferPicker = true;
  element("#diagnoseSelectInput").click();
  await waitFor(() => harness.resolvePicker, "native input picker");
  const pickerResolve = harness.resolvePicker;
  harness.resolvePicker = null;
  closeAuxiliaryPanel();
  harness.current = currentInput("28.1", "changed-during-picker.idf");
  openAuxiliaryPage("/src/tools.html");
  pickerResolve({ canceled: false, ...currentInput("29.1", "late-picker.idf") });
  await waitFor(() => element("#diagnoseStatus").dataset.i18n === "diagnoseFix.workspaceConflict", "late picker conflict");
  const latePickerProtected = harness.calls.length === 1 && harness.current.filename === "changed-during-picker.idf"
    && element("#diagnoseFilename").title === harness.current.path;
  assert(latePickerProtected, "late native picker overwrote Main");
  harness.deferPicker = false;
  element("#diagnoseSelectInput").click();
  await waitFor(() => harness.calls.length === 2, "explicit new input handoff");
  const replacementApplied = harness.current.filename === "replacement.idf" && harness.calls[1].options.replaceWorkspace === true;
  assert(replacementApplied && element("#multiSimulationFiles").innerHTML === selectedFiles, "new input handoff reset unrelated batch state");
  document.querySelector("#result").textContent = JSON.stringify({ initialFromMain, dormantScan, parentBridge, cachedSelection,
    explicitApply, snapshotOwnedByMain, lateApplyProtected, hostConflictProtected, latePickerProtected, replacementApplied, localizedConflict });
  document.body.dataset.auxiliaryDiagnoseStatus = "passed";
} catch (error) {
  document.querySelector("#result").textContent = error.stack || String(error);
  document.body.dataset.auxiliaryDiagnoseStatus = "failed";
}
</script></body></html>`

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
  semanticFollowSelection: false,
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
    const { setLanguage } = await import("/src/js/i18n.js");
    const beforeLanguageChange = sessionStorage.getItem("idfAnalyzer.currentDocument");
    setLanguage("ko");
    const koreanPreview = !document.querySelector("#diagnosePreviewPanel").hidden
      && document.querySelector("#diagnosePreviewPanel").textContent.includes("1개 제거");
    const koreanStatus = document.querySelector("#diagnoseStatus").textContent.includes("진단 1개")
      && document.querySelector("#diagnoseCandidateStats").textContent.includes("1개 선택")
      && document.querySelector("[data-diagnose-candidate]")?.checked;
    setLanguage("fr");
    const englishFallback = document.querySelector("#multiSimulationStats").textContent === "No simulation files selected";
    setLanguage("en");
    const localePreserved = koreanPreview && koreanStatus && englishFallback
      && document.querySelector("#diagnosePreviewPanel").textContent.includes("1 removals")
      && sessionStorage.getItem("idfAnalyzer.currentDocument") === beforeLanguageChange;
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
      && appliedSnapshot.layout?.topologyDetailsHeight === "42%";
    const legacyModesDropped = !("semanticLinkMode" in appliedSnapshot)
      && !("semanticFollowSelection" in appliedSnapshot);
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
      localePreserved,
      hydrationPreserved,
      snapshotApplied,
      analysisInvalidated,
      workspaceContextRetained,
      legacyModesDropped,
      replacementReset
    });
    document.body.dataset.toolsDiagnoseStatus = diagnostic && candidateVisible && preview && localePreserved
      && hydrationPreserved && snapshotApplied && analysisInvalidated && workspaceContextRetained
      && legacyModesDropped && replacementReset ? "passed" : "failed";
  })().catch((error) => {
    result.textContent = error.stack || String(error);
    document.body.dataset.toolsDiagnoseStatus = "failed";
  });
})();
</script>`
