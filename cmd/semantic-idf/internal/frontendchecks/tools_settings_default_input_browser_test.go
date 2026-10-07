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

func TestSettingsDefaultInputViewPreservesWorkspaceBrowser(t *testing.T) {
	_ = readTranslationSource(t)
	for _, path := range []string{"frontend/src/js/main.js", "frontend/src/js/actions.js", "frontend/src/js/state.js", "frontend/src/js/views/input-views.js", "frontend/src/js/settings-client.js", "frontend/src/js/ui-features.js"} {
		readTestFile(t, path)
	}
	if testing.Short() {
		t.Skip("headless-browser initial Input View preference")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	page := strings.Replace(readTestFile(t, "frontend/src/index.html"), "<head>", `<head><base href="/src/">`+settingsInputViewSetupHTML, 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/settings-input-main", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	mux.HandleFunc("/settings-input-preference", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, settingsInputViewHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=20000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/settings-input-preference")
	output, err := command.CombinedOutput()
	if err != nil || ctx.Err() != nil || !strings.Contains(string(output), `data-settings-input-status="passed"`) {
		t.Fatalf("default Input View acceptance failed: %v, %v\n%s", err, ctx.Err(), output)
	}
	for _, signal := range []string{`"initialPreferences":3`, `"restoredView":true`, `"userSelectionRace":true`, `"openedDocumentRace":true`, `"settingsPreserveView":true`} {
		if !strings.Contains(string(output), signal) {
			t.Fatalf("default Input View result missing %s:\n%s", signal, output)
		}
	}
}

const settingsInputViewSetupHTML = `<script>
window.__inputViewErrors = [];
window.addEventListener("error", event => window.__inputViewErrors.push(event.error?.stack || event.message));
window.addEventListener("unhandledrejection", event => window.__inputViewErrors.push(event.reason?.stack || String(event.reason)));
sessionStorage.removeItem("idfAnalyzer.currentDocument");
if (new URLSearchParams(location.search).has("restored")) {
  sessionStorage.setItem("idfAnalyzer.currentDocument", JSON.stringify({schemaVersion:4,text:"Version,25.1;\nBuilding,Restored;\n",path:"C:/RestoredRun/restored.idf",filename:"restored.idf",activeInputView:"table",activeResultTab:"metrics"}));
}
window.go = {main:{App:{
  GetSettings: () => new Promise(resolve => { window.__resolveInputViewSettings = resolve; }),
  GetAppInfo: async () => ({name:"SemanticIDF",version:"test"}),
  GetSimulationEnvironment: async () => ({weatherFolders:[],installations:[],defaultWorkerCount:1}),
  SetStorageInputPath: async path => { (window.__registeredStorageInputPaths ||= []).push(path); },
  AnalyzeInputText: async () => ({report:null,model:null,epjson:""}),
}}};
</script><script type="module">
import * as settings from "/src/js/settings-client.js";
import * as input from "/src/js/views/input-views.js";
import * as actions from "/src/js/actions.js";
import * as store from "/src/js/state.js";
window.addEventListener("load", () => { window.__inputViewRuntime = {settings,input,actions,store}; });
</script>`

const settingsInputViewHarnessHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Initial Input View preference</title></head>
<body data-settings-input-status="pending"><pre id="settingsInputResult">pending</pre>
<script type="module">
const resultElement = document.querySelector("#settingsInputResult");
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const waitFor = async (predicate, message) => {
  for (let attempt=0; attempt<400; attempt++) { if (predicate()) return; await new Promise(resolve => setTimeout(resolve,20)); }
  throw new Error(message);
};
async function openWorkspace(restored=false) {
  const frame = document.createElement("iframe"); frame.src = "/settings-input-main"+(restored?"?restored=1":""); document.body.append(frame);
  await waitFor(() => frame.contentWindow?.__inputViewRuntime && frame.contentWindow.__resolveInputViewSettings && frame.contentWindow.__inputViewRuntime.store.state.currentFilename, "actual main workspace did not initialize");
  return {frame,win:frame.contentWindow,...frame.contentWindow.__inputViewRuntime};
}
async function resolvePreference(workspace, view) {
  workspace.win.__resolveInputViewSettings({settings:{appearance:{defaultInputView:view,language:"en",theme:"light"}}});
  await waitFor(() => workspace.settings.getCurrentAppSettings().appearance.defaultInputView === view, "settings response was not applied");
  await new Promise(resolve => setTimeout(resolve,50));
  assert(!workspace.win.__inputViewErrors.length, workspace.win.__inputViewErrors.join("\n"));
}
try {
  const evidence = {initialPreferences:0};
  for (const [preference,expected] of [["json","json"],["table","table"],["semantic","text"]]) {
    const workspace = await openWorkspace(); await resolvePreference(workspace,preference);
    assert(workspace.store.state.activeInputView === expected, "fresh workspace did not use "+preference+" preference");
    assert(workspace.frame.contentDocument.querySelector('[data-input-view="'+expected+'"]').getAttribute("aria-selected") === "true", "initial preference did not update Input tab accessibility");
    if (preference === "json") {
      const changed = workspace.settings.applyAppSettings({...workspace.settings.getCurrentAppSettings(),appearance:{...workspace.settings.getCurrentAppSettings().appearance,defaultInputView:"text"}});
      workspace.win.dispatchEvent(new workspace.win.CustomEvent("idfAnalyzer:settingsChanged",{detail:{settings:changed,external:true}}));
      await new Promise(resolve => setTimeout(resolve,50));
      assert(workspace.store.state.activeInputView === "json", "saving a new preference replaced the current Input View");
      workspace.actions.registerLoadedDocument("Version,25.1;\nBuilding,Next;\n",{filename:"next.idf"});
      assert(workspace.store.state.activeInputView === "json", "opening a new document replaced the chosen Input View");
      evidence.settingsPreserveView = true;
    }
    evidence.initialPreferences++; workspace.frame.remove();
  }
  {
    const workspace = await openWorkspace(true); await resolvePreference(workspace,"json");
    assert(workspace.store.state.activeInputView === "table", "default preference replaced the restored workspace Input View");
    assert(workspace.win.__registeredStorageInputPaths.includes("C:/RestoredRun/restored.idf"), "restored source file was not protected from cleanup");
    evidence.restoredView = true; workspace.frame.remove();
  }
  {
    const workspace = await openWorkspace(); await workspace.input.switchInputView("table"); await resolvePreference(workspace,"json");
    assert(workspace.store.state.activeInputView === "table", "late settings response replaced explicit user selection");
    evidence.userSelectionRace = true; workspace.frame.remove();
  }
  {
    const workspace = await openWorkspace(); workspace.actions.registerLoadedDocument("Version,25.1;\nBuilding,Opened;\n",{filename:"opened.idf",path:"C:/GeneratedRun/opened.idf"}); await resolvePreference(workspace,"json");
    assert(workspace.store.state.activeInputView === "text", "late settings response changed an already opened document");
    assert(workspace.win.__registeredStorageInputPaths.at(-1) === "C:/GeneratedRun/opened.idf", "opened source file was not protected from cleanup");
    evidence.openedDocumentRace = true; workspace.frame.remove();
  }
  resultElement.textContent = JSON.stringify(evidence); document.body.dataset.settingsInputStatus = "passed";
} catch (error) { resultElement.textContent = error.stack || String(error); document.body.dataset.settingsInputStatus = "failed"; }
</script></body></html>`
