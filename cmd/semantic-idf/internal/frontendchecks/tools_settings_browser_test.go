package frontendchecks

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSettingsStorageAndPreferencesBrowser(t *testing.T) {
	source := readTestFile(t, "frontend/src/settings.html")
	_ = readSettingsSource(t)
	_ = readTranslationSource(t)
	for _, path := range []string{"frontend/src/js/settings-storage.js", "frontend/src/js/settings-client.js", "frontend/src/js/auxiliary-context.js", "frontend/src/styles/settings.css"} {
		readTestFile(t, path)
	}
	if testing.Short() {
		t.Skip("skipping Settings storage acceptance browser in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	html := strings.Replace(source, "<head>", `<head><base href="/src/">`+settingsStorageSetup, 1)
	html = strings.Replace(html, "</body>", settingsStorageAssertions+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/settings-storage", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, html)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox",
		"--no-first-run", "--no-default-browser-check", "--window-size=1200,900",
		"--virtual-time-budget=20000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/settings-storage",
	)
	output, err := command.CombinedOutput()
	if err != nil || ctx.Err() != nil {
		t.Fatalf("Settings storage browser failed: %v, %v\n%s", err, ctx.Err(), output)
	}
	if !strings.Contains(string(output), `data-settings-storage-status="passed"`) {
		t.Fatalf("Settings storage browser did not pass:\n%s", output)
	}
	if directory := strings.TrimSpace(os.Getenv("IDF_SETTINGS_SCREENSHOT_DIR")); directory != "" {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		browser := epath201FreshBrowser(t, ctx, chrome)
		for _, capture := range []struct {
			name, section string
			width         int
		}{
			{"desktop-appearance", "appearance", 1200}, {"desktop-storage", "storage", 1200},
			{"mobile-appearance", "appearance", 420}, {"mobile-storage", "storage", 420},
		} {
			browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": capture.width, "height": 900, "deviceScaleFactor": 1, "mobile": capture.width < 560}, nil)
			browser.call("Page.navigate", map[string]any{"url": server.URL + "/settings-storage?capture=" + capture.section}, nil)
			ready := false
			for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
				status := string(browser.evaluate(`document.body?.dataset.settingsStorageStatus || "pending"`))
				if status == `"passed"` {
					ready = true
					break
				}
				if status == `"failed"` {
					t.Fatalf("Settings screenshot fixture failed: %s", browser.evaluate(`document.getElementById("settingsStorageResult")?.textContent`))
				}
				time.Sleep(25 * time.Millisecond)
			}
			if !ready {
				t.Fatal("Settings screenshot did not become ready")
			}
			proof := browser.evaluate(fmt.Sprintf(`new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>{const target=document.getElementById(%q);target.scrollIntoView({block:"start"});requestAnimationFrame(()=>resolve({width:innerWidth,height:innerHeight,documentWidth:document.documentElement.scrollWidth,targetTop:target.getBoundingClientRect().top,save:document.getElementById("settingsSave").getBoundingClientRect().right}));})))`, capture.section))
			t.Logf("Settings %s screenshot layout: %s", capture.name, proof)
			var screenshot struct {
				Data string `json:"data"`
			}
			browser.call("Page.captureScreenshot", map[string]any{"format": "png", "fromSurface": true, "captureBeyondViewport": false}, &screenshot)
			data, err := base64.StdEncoding.DecodeString(screenshot.Data)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, capture.name+".png"), data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

const settingsStorageSetup = `<script>
window.settingsStorageCalls = [];
window.settingsSaves = [];
window.settingsHoldSave = false;
window.settingsPendingSaves = [];
window.settingsStorageReads = 0;
window.settingsEnvironmentReads = 0;
window.settingsStorageErrors = [];
sessionStorage.setItem("idfAnalyzer.currentDocument", JSON.stringify({ path: "C:\\fixture\\app-runs\\model.idf" }));
window.addEventListener("error", (event) => window.settingsStorageErrors.push(event.message));
window.storageFixture = {
  scannedAt: "2026-10-08T00:00:00Z", totalBytes: 12288, reclaimableBytes: 8192, protectedBytes: 4096,
  runCount: 3, reclaimableRunCount: 2, protectedRunCount: 1,
  roots: [{ path: "C:/fixture/app-runs", isDefault: true, totalBytes: 12288, reclaimableBytes: 8192, protectedBytes: 4096, runCount: 3, reclaimableRunCount: 2, protectedRunCount: 1 }],
  warnings: [{ path: "C:/fixture/<unreadable>", reason: "in_use" }],
  browserDataPath: "C:/fixture/webview", browserDataBytes: 2048,
};
window.go = { main: { App: {
  GetSettings: async () => { if (new URLSearchParams(location.search).has("loadFailure")) throw new Error("fixture-settings-load-failure"); return { path: "C:/fixture/settings.json", settings: { appearance: { language: "en", theme: "light" }, simulation: { energyPlusInstallations: [{ id: "fixture-engine", name: "EnergyPlus fixture", version: "23.1", executablePath: "C:/fixture/EnergyPlus/energyplus.exe", rootPath: "C:/fixture/EnergyPlus", weatherDataPath: "C:/fixture/EnergyPlus/WeatherData", autoDetected: false }] } } }; },
  GetAppInfo: async () => ({ name: "SemanticIDF", version: "test" }),
  GetSimulationEnvironment: async () => { window.settingsEnvironmentReads += 1; return { weatherFolders: [], installations: [], defaultWorkerCount: 1 }; },
  GetStorageUsage: async () => { window.settingsStorageReads += 1; return window.storageFixture; },
  CleanStorage: (request) => new Promise((resolve, reject) => window.settingsStorageCalls.push({ request, resolve, reject })),
  SaveSettings: async (settings) => { window.settingsSaves.push(settings); if (window.settingsHoldSave) await new Promise(resolve => window.settingsPendingSaves.push(resolve)); return { settings: { ...settings, simulation: { ...settings.simulation, runDirectory: settings.simulation.runDirectory || "C:/fixture/default-runs" } } }; },
  SelectSimulationRunDirectory: async () => "C:/fixture/new-runs",
} } };
</script>`

const settingsStorageAssertions = `<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const pause = () => new Promise((resolve) => setTimeout(resolve, 10));
async function until(condition, message) { for (let n = 0; n < 250; n += 1) { if (condition()) return; await pause(); } throw new Error(message); }
const selectValue = (id, value) => { const input = document.getElementById(id); input.value = value; input.dispatchEvent(new Event("input", { bubbles: true })); };
const setAge = (value) => { const input = document.getElementById("storageCleanupAge"); input.value = String(value); input.dispatchEvent(new Event("change", { bubbles: true })); };
try {
  if (new URLSearchParams(location.search).has("embeddedStorage")) {
    await until(() => document.getElementById("settingsForm") && !document.getElementById("storageRefresh")?.disabled, "embedded Settings usage should initialize");
    assert(window.go === parent.go && document.documentElement.dataset.embeddedApp === "true", "embedded Settings uses the Main native bridge");
    for (const [path, expected] of [
      ["C:\\live-main\\runs\\model.idf", ["C:\\live-main\\runs"]],
      ["/tmp/live-main/runs/model.idf", ["/tmp/live-main/runs"]],
      ["C:\\model.idf", ["C:\\"]],
      ["", []],
    ]) {
      parent.settingsStorageLiveDocument = { text: "Version, 23.1;", path, filename: "model.idf" };
      sessionStorage.setItem("idfAnalyzer.currentDocument", JSON.stringify({ path: "C:\\stale-snapshot\\model.idf" }));
      const previousCalls = parent.settingsStorageCalls.length;
      setAge(0); document.getElementById("storagePrepareClean").click(); document.getElementById("storageConfirmClean").click();
      await until(() => parent.settingsStorageCalls.length === previousCalls + 1, "embedded cleanup should reach the native Main API");
      const operation = parent.settingsStorageCalls[previousCalls];
      assert(JSON.stringify(operation.request.protectedOutputDirectories) === JSON.stringify(expected), "embedded cleanup must protect the live Main path, including an empty path, without falling back to the stale snapshot: " + path);
      operation.resolve({ freedBytes: 0, removedRunCount: 0, skippedRunCount: 0, failures: [], usage: parent.storageFixture });
      await until(() => !document.getElementById("storageRefresh").disabled, "embedded cleanup should settle before the next source change");
    }
  } else if (new URLSearchParams(location.search).has("loadFailure")) {
    await until(() => document.getElementById("settingsForm") && document.getElementById("storageTotalBytes")?.textContent === "12 KiB", "cached settings and storage should initialize after a settings read failure");
    assert(document.querySelector(".settings-message.warning")?.textContent.includes("fixture-settings-load-failure"), "settings load failure remains visible beside the cached values");
    assert(document.getElementById("settingsSaveStatus").dataset.i18n === "status.settingsSaveUnverified" && document.getElementById("settingsForm").dataset.dirty === "true" && !document.getElementById("settingsSave").disabled, "unverified cached or default settings must not claim to be saved and can be confirmed by saving");
    selectValue("languageSelect", "en");
    assert(document.getElementById("settingsSaveStatus").dataset.i18n === "status.settingsSaveUnverified" && document.getElementById("settingsSaveStatus").textContent.includes("could not be verified"), "changing language preserves and translates the unverified saved state");
    selectValue("languageSelect", "ko");
    const saveSettings = window.go.main.App.SaveSettings;
    window.go.main.App.SaveSettings = async () => { throw new Error("fixture-settings-save-failure"); };
    document.getElementById("settingsForm").dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
    await until(() => document.getElementById("settingsSaveStatus").textContent.includes("fixture-settings-save-failure") && !document.getElementById("settingsSave").disabled, "failed confirmation save remains retryable");
    assert(document.getElementById("settingsForm").dataset.dirty === "true", "a failed save does not establish a saved baseline");
    window.go.main.App.SaveSettings = saveSettings;
    document.getElementById("settingsForm").dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
    await until(() => document.getElementById("settingsForm").dataset.dirty === "false" && document.getElementById("settingsSaveStatus").dataset.i18n === "status.savedSettings", "a successful save confirms the previously unverified values");
  } else {
  await until(() => document.getElementById("storageTotalBytes")?.textContent === "12 KiB", "Settings and usage should initialize");
  assert(window.settingsStorageErrors.length === 0, "Settings initial render has no runtime errors: " + window.settingsStorageErrors);
  assert(document.getElementById("settingsForm").dataset.dirty === "false" && document.getElementById("settingsSave").disabled, "initial settings must report saved rather than unsaved preview");
  assert(document.getElementById("storageReclaimableBytes").textContent === "8 KiB" && document.getElementById("storageProtectedBytes").textContent === "4 KiB", "storage separates reclaimable and protected bytes");
  assert(document.querySelector(".settings-browser-storage").textContent.includes("2 KiB") && !document.querySelector(".settings-browser-storage button"), "browser restoration data is visible without destructive browser deletion");
  assert(document.querySelector(".settings-storage-warnings").textContent.includes("<unreadable>") && !document.querySelector("unreadable"), "storage paths are escaped");
  setAge(30); document.getElementById("storagePrepareClean").click();
  assert(window.settingsStorageCalls.length === 0 && document.getElementById("storageConfirmation"), "cleanup must require a visible review before calling the backend");
  assert(document.activeElement.id === "storageConfirmCancel" && document.getElementById("storageConfirmation").textContent.includes("30"), "cleanup review focuses Cancel and retains its age condition");
  document.getElementById("storageConfirmCancel").click();
  assert(!document.getElementById("storageConfirmation") && document.activeElement.id === "storagePrepareClean", "Cancel restores the review action and never removes files");
  assert(document.getElementById("settingsForm").dataset.dirty === "false", "storage scope and review never mark app preferences as dirty");
  document.getElementById("storagePrepareClean").click(); document.getElementById("storageConfirmClean").click();
  await until(() => window.settingsStorageCalls.length === 1, "confirmed cleanup should start");
  assert(window.settingsStorageCalls[0].request.olderThanDays === 30 && Array.isArray(window.settingsStorageCalls[0].request.protectedOutputDirectories), "cleanup sends only a scoped age request with protected work support");
  assert(window.settingsStorageCalls[0].request.protectedOutputDirectories[0] === "C:\\fixture\\app-runs", "cleanup protects the current Windows input folder from the workspace snapshot");
  assert(document.getElementById("storageRefresh").disabled && document.getElementById("storageConfirmClean").disabled, "busy cleanup prevents duplicate operations");
  selectValue("languageSelect", "ko");
  assert(document.documentElement.lang === "ko" && document.getElementById("languageSelect").value === "ko", "language preview applies before pending cleanup resolves");
  assert(document.getElementById("storageCleanupAge").value === "30" && document.getElementById("storageConfirmClean").disabled, "language rebuild preserves cleanup scope and active operation");
  window.storageFixture = { ...window.storageFixture, totalBytes: 8192, reclaimableBytes: 4096, runCount: 2, reclaimableRunCount: 1 };
  window.settingsStorageCalls[0].resolve({ freedBytes: 4096, removedRunCount: 1, skippedRunCount: 1, failures: [{ path: "C:/fixture/locked", reason: "fixture-locked-file" }], usage: window.storageFixture });
  await until(() => document.getElementById("storageOperationStatus").textContent.includes("4 KiB") && !document.getElementById("storageRefresh").disabled, "partial cleanup results should report freed bytes and remain usable");
  assert(document.getElementById("storageOperationStatus").classList.contains("error") && document.querySelector(".settings-storage-warnings").textContent.includes("fixture-locked-file"), "partial errors retain raw evidence and show remaining work");
  assert(document.getElementById("storageTotalBytes").textContent === "8 KiB" && document.activeElement.id === "storageRefresh", "cleanup updates usage and restores a useful focus target");
  selectValue("defaultInputView", "json"); selectValue("profileTimeView", "duration");
  assert(document.getElementById("languageSelect").value === "ko", "changing input and Profile defaults retains the language preview");
  const autoClean = document.getElementById("storageAutoClean"); autoClean.checked = true; autoClean.dispatchEvent(new Event("input", { bubbles: true }));
  assert(!document.querySelector('#defaultInputView option[value="semantic"]'), "dormant Semantic input view remains hidden");
  document.getElementById("simulationBrowseRunDirectory").click();
  await until(() => document.getElementById("simulationRunDirectory").value === "C:/fixture/new-runs", "run directory picker should update the editable preference");
  assert(document.getElementById("languageSelect").value === "ko", "asynchronous directory selection retains the language preview");
  assert(document.getElementById("settingsForm").dataset.dirty === "true", "input/time view and run folder edits mark settings as dirty");
  selectValue("simulationRunDirectory", "");
  document.getElementById("settingsForm").dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  await until(() => window.settingsSaves.length === 1 && document.getElementById("settingsForm").dataset.dirty === "false", "edited preferences should save cleanly");
  assert(window.settingsSaves[0].appearance.defaultInputView === "json" && window.settingsSaves[0].profile.timeView === "duration" && window.settingsSaves[0].storage.autoClean === true && window.settingsSaves[0].appearance.language === "ko" && document.documentElement.lang === "ko", "new controls persist through the normalized settings API and preserve the selected language");
  assert(window.settingsSaves[0].simulation.runDirectory === "" && document.getElementById("simulationRunDirectory").value === "C:/fixture/default-runs" && document.getElementById("settingsSave").disabled, "normalized backend defaults become the saved form baseline");
  assert(window.settingsSaves[0].simulation.energyPlusInstallations[0].id === "fixture-engine", "editing unrelated preferences preserves registered EnergyPlus identity");
  assert(window.settingsEnvironmentReads === 2, "changed simulation settings refresh environment metadata after the initial discovery");
  selectValue("graphFontSize", "12"); document.getElementById("settingsForm").dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  await until(() => window.settingsSaves.length === 2 && document.getElementById("settingsForm").dataset.dirty === "false", "appearance-only preferences should save cleanly");
  assert(window.settingsEnvironmentReads === 2, "appearance-only saves skip expensive EnergyPlus and weather discovery");
  const savedAutoClean = document.getElementById("storageAutoClean"); savedAutoClean.checked = false; savedAutoClean.dispatchEvent(new Event("input", { bubbles: true }));
  document.getElementById("settingsForm").dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  await until(() => window.settingsSaves.length === 3 && document.getElementById("settingsForm").dataset.dirty === "false", "storage policy preferences should save cleanly");
  assert(window.settingsEnvironmentReads === 2, "storage-only saves also skip environment discovery");
  await until(() => !document.getElementById("storageRefresh").disabled, "saved storage policy refresh should settle before the next save");
  const readsBeforeEarlierSave = window.settingsStorageReads;
  const earlierAutoClean = document.getElementById("storageAutoClean"); earlierAutoClean.checked = true; earlierAutoClean.dispatchEvent(new Event("input", { bubbles: true }));
  window.settingsHoldSave = true;
  document.getElementById("settingsForm").dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  await until(() => window.settingsPendingSaves.length === 1, "an earlier settings save should remain pending");
  selectValue("graphFontSize", "14");
  const usageBeforeEarlierSave = window.storageFixture;
  window.storageFixture = { ...window.storageFixture, totalBytes: 16384, reclaimableBytes: 12288 };
  window.settingsHoldSave = false; window.settingsPendingSaves[0]();
  await until(() => document.getElementById("settingsSaveStatus").dataset.i18n === "status.settingsSavedEarlier" && document.getElementById("storageTotalBytes").textContent === "16 KiB", "a successful earlier save still refreshes storage after later form edits");
  assert(window.settingsStorageReads === readsBeforeEarlierSave + 1 && document.getElementById("settingsForm").dataset.dirty === "true" && document.getElementById("graphFontSize").value === "14", "earlier save refresh preserves later unsaved edits and their dirty state");
  document.getElementById("settingsForm").dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  await until(() => window.settingsSaves.length === 5 && document.getElementById("settingsForm").dataset.dirty === "false" && !document.getElementById("storageRefresh").disabled, "later form edits should save normally after an earlier request completes");
  assert(window.settingsEnvironmentReads === 2, "earlier storage and later appearance saves do not rediscover simulation engines");
  window.storageFixture = usageBeforeEarlierSave;
  document.getElementById("storageRefresh").click();
  await until(() => document.getElementById("storageTotalBytes").textContent === "8 KiB" && !document.getElementById("storageRefresh").disabled, "storage should return to the cleanup fixture");
  sessionStorage.setItem("idfAnalyzer.currentDocument", JSON.stringify({ path: "/tmp/fixture/app-runs/model.idf" }));
  setAge(0); document.getElementById("storagePrepareClean").click(); document.getElementById("storageConfirmClean").click();
  await until(() => window.settingsStorageCalls.length === 2, "second explicit cleanup should start");
  assert(window.settingsStorageCalls[1].request.protectedOutputDirectories[0] === "/tmp/fixture/app-runs", "cleanup also protects POSIX input folders");
  window.settingsStorageCalls[1].reject(new Error("fixture-clean-failure"));
  await until(() => document.getElementById("storageOperationStatus").textContent.includes("fixture-clean-failure"), "cleanup failure must be visible");
  assert(!document.getElementById("storageConfirmClean").disabled && document.getElementById("settingsForm").dataset.dirty === "false", "failed storage cleanup is retryable and does not change saved settings");
  document.getElementById("storageConfirmCancel").click();
  const settingsAPI = window.go.main.App;
  settingsAPI.GetStorageUsage = async () => { throw new Error("fixture-scan-failure"); };
  document.getElementById("storageRefresh").click();
  await until(() => document.getElementById("storageOperationStatus").textContent.includes("fixture-scan-failure"), "usage refresh failure should be visible");
  assert(document.getElementById("storageTotalBytes").textContent === "8 KiB", "refresh failure retains the last known usage");
  settingsAPI.GetStorageUsage = async () => ({ ...window.storageFixture, roots: [{ path: "C:/fixture/app-runs", totalBytes: 0, reclaimableBytes: 0 }, { path: "C:/fixture/app-runs-other", totalBytes: 8192, reclaimableBytes: 4096 }], browserDataBytes: 0, browserDataUnavailable: true, warnings: [{ path: "c:\\FIXTURE\\APP-RUNS", reason: "unsafe_or_unreadable_path" }], lastCleanup: { completedAt: "2026-10-08T00:00:00Z", automatic: true, freedBytes: 1024, removedRunCount: 1, skippedRunCount: 2, failureCount: 0, failures: [] } });
  document.getElementById("storageRefresh").click();
  await until(() => document.querySelector(".settings-browser-storage strong")?.textContent !== "2 KiB", "incomplete browser measurement should update");
  assert(document.querySelector(".settings-browser-storage strong").textContent !== "0 B" && document.getElementById("storageTotalBytes").textContent !== "12 KiB", "partial or unavailable measurements must never be reported as exact zero or complete totals");
  assert(document.querySelector(".settings-storage-last-cleanup")?.textContent.includes("1 KiB"), "latest automatic cleanup evidence should remain visible after usage refresh");
  const rootSizes = [...document.querySelectorAll("#storageFolders li span")].map(element => element.textContent);
  assert(rootSizes[0].includes("최소 0 B") && !rootSizes[1].includes("최소") && rootSizes[1].includes("8 KiB"), "unreadable roots show a minimum instead of exact zero, without marking adjacent complete roots as partial");
  document.getElementById("storagePrepareClean").click();
  assert(document.getElementById("storageConfirmation").textContent.includes("현재 측정된 정리 가능 용량") && document.getElementById("storageConfirmation").textContent.includes("최소 4 KiB") && !document.getElementById("storageConfirmation").textContent.includes("최대"), "partial cleanup review describes measured availability without claiming it is an upper bound");
  document.getElementById("storageConfirmCancel").click();
  const failedLoadFrame = document.createElement("iframe"); failedLoadFrame.style.display = "none"; failedLoadFrame.src = "/settings-storage?loadFailure=1"; document.body.append(failedLoadFrame);
  await until(() => ["passed", "failed"].includes(failedLoadFrame.contentDocument?.body?.dataset.settingsStorageStatus), "actual Settings page should validate failed initial settings loading");
  assert(failedLoadFrame.contentDocument.body.dataset.settingsStorageStatus === "passed", "failed-load Settings assertions: " + failedLoadFrame.contentDocument.getElementById("settingsStorageResult")?.textContent);
  failedLoadFrame.remove();
  const embeddedFrame = document.createElement("iframe"); embeddedFrame.style.display = "none";
  window.settingsStorageLiveDocument = { text: "Version, 23.1;", path: "C:/live-main/model.idf", filename: "model.idf" };
  window.idfAnalyzerAuxiliaryHost = {
    contains: child => child === embeddedFrame.contentWindow,
    getDocument: () => ({ ...window.settingsStorageLiveDocument }),
  };
  embeddedFrame.src = "/settings-storage?embeddedStorage=1"; document.body.append(embeddedFrame);
  await until(() => ["passed", "failed"].includes(embeddedFrame.contentDocument?.body?.dataset.settingsStorageStatus), "actual embedded Settings should validate live input cleanup protection");
  assert(embeddedFrame.contentDocument.body.dataset.settingsStorageStatus === "passed", "embedded Settings assertions: " + embeddedFrame.contentDocument.getElementById("settingsStorageResult")?.textContent);
  embeddedFrame.remove(); delete window.idfAnalyzerAuxiliaryHost;
  }
  assert(window.settingsStorageErrors.length === 0, "Settings interactions have no runtime errors: " + window.settingsStorageErrors);
  document.body.dataset.settingsStorageStatus = "passed";
  const capture = new URLSearchParams(location.search).get("capture");
  if (capture === "storage") document.getElementById("storage").scrollIntoView({ block: "start" });
  else if (capture) window.scrollTo(0, 0);
  if (capture) { await new Promise(requestAnimationFrame); await new Promise(requestAnimationFrame); }
} catch (error) {
  document.body.dataset.settingsStorageStatus = "failed";
  const result = document.createElement("pre"); result.id = "settingsStorageResult"; result.textContent = error.stack || String(error); document.body.prepend(result);
}
</script>`
