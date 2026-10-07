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

func TestLocalizationLanguageSettingsAndDynamicMessagesBrowser(t *testing.T) {
	_ = readTranslationSource(t)
	for _, path := range []string{"frontend/src/js/settings-client.js", "frontend/src/js/auxiliary-context.js", "frontend/src/js/settings.js", "frontend/src/js/settings-storage.js", "frontend/src/styles/settings.css", "frontend/src/js/state.js", "frontend/src/js/localized-text.js"} {
		readTestFile(t, path)
	}
	if testing.Short() {
		t.Skip("skipping localization runtime browser in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	for _, fixture := range []struct {
		page, setup string
	}{
		{"settings", localizationSettingsSetupHTML},
		{"index", localizationMainSetupHTML},
	} {
		markup, err := os.ReadFile(repoPath("frontend/src/" + fixture.page + ".html"))
		if err != nil {
			t.Fatal(err)
		}
		html := strings.Replace(string(markup), "<head>", `<head><base href="/src/">`+fixture.setup, 1)
		mux.HandleFunc("/localization-"+fixture.page, func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(writer, html)
		})
	}
	mux.HandleFunc("/localization-runtime", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, localizationRuntimeBrowserHTML)
	})
	mux.HandleFunc("/localization-other-window", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, `<!doctype html><html><head><meta charset="utf-8"><title>Other settings window</title></head><body>Settings peer</body></html>`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox",
		"--no-first-run", "--no-default-browser-check", "--virtual-time-budget=20000",
		"--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/localization-runtime",
	)
	output, err := command.CombinedOutput()
	if err != nil || ctx.Err() != nil {
		t.Fatalf("localization runtime browser failed: %v, %v\n%s", err, ctx.Err(), output)
	}
	if !strings.Contains(string(output), `data-localization-status="passed"`) {
		t.Fatalf("localization runtime browser did not pass:\n%s", output)
	}
	for _, signal := range []string{`"languages":6`, `"dynamicMessages":true`, `"settingsSynchronization":true`, `"settingsSaveRaces":true`, `"mainTabPreserved":true`} {
		if !strings.Contains(string(output), signal) {
			t.Fatalf("localization runtime result missing %s:\n%s", signal, output)
		}
	}
}

const localizationRuntimeBrowserHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Localization runtime</title></head>
<body data-localization-status="pending"><pre id="localizationResult">pending</pre>
<button id="saveLabel" data-i18n="action.save">Save</button>
<input id="filter" data-i18n-placeholder="input.filter" data-i18n-aria-label="shell.inputFilter" placeholder="Filter objects, fields, values">
<div id="runtimeStatus" data-i18n="status.analysisComplete">Analysis complete</div>
<span id="countLabel"></span><span id="literalLabel"></span>
<div id="sourceLegend"><code data-source-kind="rule">energyplus_rule</code><span data-i18n="simulation.energyPathSourceDerived">Derived</span></div>
<iframe id="settingsPeer" src="/localization-other-window" hidden></iframe>
<script type="module">
const output = document.querySelector("#localizationResult");
const assert = (value, message) => { if (!value) throw new Error(message); };
const sleep = (ms = 10) => new Promise((resolve) => setTimeout(resolve, ms));
async function until(condition, message) { for (let n = 0; n < 150; n += 1) { if (condition()) return; await sleep(); } throw new Error(message); }
try {
  const { supportedLanguages, normalizeLanguage, getLanguage, getManualLanguage, setLanguage, t, translateForLanguage, localizedMessage } = await import("/src/js/i18n.js");
  const { localeDefinitions } = await import("/src/js/locales/index.js");
  const { applyAppSettings, getCurrentAppSettings, loadAndApplyAppSettings, saveAppSettings, settingsStorageKey } = await import("/src/js/settings-client.js");
  const { state, setStatus } = await import("/src/js/state.js");
  const { setLocalizedText, setLiteralText } = await import("/src/js/localized-text.js");
  applyAppSettings({ appearance: { language: "en", theme: "light" } });
  const events = []; window.addEventListener("idfAnalyzer:languageChanged", (event) => events.push(event.detail));
  assert(supportedLanguages.length === 6, "all six application languages are supported");
  for (const [language] of supportedLanguages) {
    const before = getLanguage(); const count = events.length;
    setLanguage(language);
    assert(getLanguage() === language && document.documentElement.lang === language, "current application language is applied: " + language);
    assert(document.querySelector("#saveLabel").textContent === t("action.save"), "static action label uses the selected language: " + language);
    assert(document.querySelector("#filter").placeholder === t("input.filter") && document.querySelector("#filter").getAttribute("aria-label") === t("shell.inputFilter"), "placeholder and accessible text follow language: " + language);
    assert(events.length === count + Number(before !== language), "one language event per actual change: " + language);
    if (before !== language) assert(events.at(-1).language === language && events.at(-1).previousLanguage === before, "event retains the previous and next language");
    setLanguage(language.toUpperCase() + "_regional");
    assert(events.length === count + Number(before !== language), "equivalent regional setting does not emit a duplicate event");
    assert(getManualLanguage() === (language === "ko" ? "ko" : "en"), "manual falls back to English for unsupported manual locales: " + language);
    assert(t("tab.hvac") === "HVAC", "HVAC remains a stable industry term: " + language);
    assert(document.querySelector("#sourceLegend code").textContent === "energyplus_rule" && document.querySelector("#sourceLegend code").dataset.sourceKind === "rule", "localizing source descriptions preserves technical IDs");
  }
  for (const [input, expected] of [["KR", "ko"], ["ko-KR", "ko"], ["kr_KR", "ko"], ["JP", "ja"], ["ja-JP", "ja"], ["EN_us", "en"], ["es-MX", "es"], ["fr_CA", "fr"], ["hi-IN", "hi"], ["unknown", "en"], ["", "en"]]) {
    assert(normalizeLanguage(input) === expected, "locale alias/region normalization: " + input);
    assert(getManualLanguage(input) === (expected === "ko" ? "ko" : "en"), "normalized manual language: " + input);
  }
  const english = localeDefinitions.find((locale) => locale.code === "en").messages;
  const japanese = localeDefinitions.find((locale) => locale.code === "ja").messages;
  const fallbackKey = Object.keys(english).find((key) => !Object.hasOwn(japanese, key));
  assert(fallbackKey && translateForLanguage("ja", fallbackKey) === english[fallbackKey], "an untranslated interface message falls back to the authoritative English catalog");
  setLanguage("en");
  assert(t("tools.selectedOf", { selected: 3, total: 8 }) === "3 selected of 8", "English placeholders preserve supplied values");
  setLocalizedText(document.querySelector("#countLabel"), "tools.selectedOf", { selected: 3, total: 8 });
  setStatus(localizedMessage("status.metricsExported", { format: "CSV" }), "ok");
  state.analysisTiming = { mode: "standard", cacheHit: true, totalMs: 123, parseMs: 12.5 };
  state.analysisStageTimings = { hvac: 4.5 }; state.renderTiming.last = { tab: "metrics", ms: 56.7 };
  setStatus(localizedMessage("status.metricsExported", { format: "CSV" }), "ok");
  const englishTiming = document.querySelector("#runtimeStatus").title;
  assert(document.querySelector("#runtimeStatus").textContent === "Metrics CSV exported", "status descriptor resolves in English");
  setLanguage("ko");
  assert(translateForLanguage("en", "status.openedNamed", { name: localizedMessage("common.inputFile") }) === "Opened Input file", "explicit-language interpolation resolves nested descriptors in the requested language");
  assert(translateForLanguage("en", "status.openedNamed", { name: () => localizedMessage("common.inputFile") }) === "Opened Input file", "explicit-language interpolation resolves callback descriptors in the requested language");
  assert(translateForLanguage("en", "status.openedNamed", { name: localizedMessage("tools.selectedOf", () => ({ selected: 3, total: 8 })) }) === "Opened 3 selected of 8", "explicit-language interpolation resolves nested descriptor parameter callbacks in the requested language");
  assert(document.querySelector("#saveLabel").textContent === "저장" && document.querySelector("#filter").placeholder === "객체, 필드, 값 필터", "ordinary Korean interface text is translated");
  assert(document.querySelector("#runtimeStatus").textContent === "Metrics CSV 내보내기 완료", "stored status descriptor is reinterpreted after a language change");
  assert(document.querySelector("#countLabel").textContent === "8개 중 3개 선택", "dynamic label retains its count and Korean placeholder order");
  const koreanTiming = document.querySelector("#runtimeStatus").title;
  assert(koreanTiming !== englishTiming && koreanTiming.includes("분석") && !koreanTiming.includes("??"), "timing tooltip switches to readable Korean");
  assert(koreanTiming.includes("123 ms") && koreanTiming.includes("12.5 ms") && koreanTiming.includes("metrics") && koreanTiming.includes("hvac"), "timing localization preserves values and technical stage IDs");
  const literalError = "EnergyPlus: failed on HVAC:Coil#3 [J]";
  setStatus(literalError, "error"); setLiteralText(document.querySelector("#literalLabel"), literalError);
  setLanguage("en");
  assert(document.querySelector("#runtimeStatus").textContent === literalError && document.querySelector("#literalLabel").textContent === literalError, "literal backend errors are preserved across language changes");
  assert(!document.querySelector("#runtimeStatus").dataset.i18n && !document.querySelector("#literalLabel").dataset.i18n, "literal values are detached from earlier translation descriptors");
  const peer = document.querySelector("#settingsPeer");
  await until(() => peer.contentWindow?.location.pathname === "/localization-other-window" && peer.contentDocument?.readyState === "complete", "other settings window must load");
  peer.contentWindow.localStorage.setItem(settingsStorageKey, JSON.stringify({ appearance: { language: "KR", theme: "dark" } }));
  await until(() => getLanguage() === "ko" && getCurrentAppSettings().appearance.language === "ko" && document.documentElement.dataset.theme === "dark", "storage event from another window must apply normalized app settings");
  const beforeSameLanguage = events.length;
  peer.contentWindow.localStorage.setItem(settingsStorageKey, JSON.stringify({ appearance: { language: "ko-KR", theme: "light" } }));
  await until(() => document.documentElement.dataset.theme === "light", "same-language storage change must still apply appearance");
  assert(events.length === beforeSameLanguage, "same normalized storage language does not duplicate the language event");
  peer.contentWindow.localStorage.setItem("localization.unrelated", "changed"); await sleep(20);
  assert(getLanguage() === "ko", "unrelated storage keys do not change the application language");
  localStorage.setItem(settingsStorageKey, JSON.stringify({ appearance: { language: "fr-CA", theme: "dark" } }));
  window.dispatchEvent(new PageTransitionEvent("pageshow", { persisted: true }));
  await until(() => getLanguage() === "fr" && getCurrentAppSettings().appearance.language === "fr" && document.documentElement.dataset.theme === "dark", "restored page must reread current application language and appearance");
  const beforeRestoredLanguage = events.length;
  window.dispatchEvent(new PageTransitionEvent("pageshow", { persisted: true }));
  assert(events.length === beforeRestoredLanguage, "unchanged restored settings do not duplicate the language event");
  const previousBridge = window.go; let releaseBackendSettings;
  window.go = { main: { App: { GetSettings: () => new Promise((resolve) => { releaseBackendSettings = resolve; }) } } };
  const pendingSettings = loadAndApplyAppSettings();
  await until(() => releaseBackendSettings, "delayed backend settings request must start");
  peer.contentWindow.localStorage.setItem(settingsStorageKey, JSON.stringify({ appearance: { language: "ko", theme: "dark" } }));
  await until(() => getLanguage() === "ko", "new external settings apply while backend request is pending");
  releaseBackendSettings({ settings: { appearance: { language: "en", theme: "light" } } });
  assert((await pendingSettings).stale && getLanguage() === "ko" && getCurrentAppSettings().appearance.language === "ko" && document.documentElement.dataset.theme === "dark", "old backend settings cannot overwrite a newer external language or appearance");
  assert(JSON.parse(localStorage.getItem(settingsStorageKey)).appearance.language === "ko", "stale backend response cannot overwrite the newer cached settings");
  let releaseBackendSave;
  window.go.main.App.SaveSettings = () => new Promise((resolve) => { releaseBackendSave = resolve; });
  const pendingSave = saveAppSettings({ appearance: { language: "en", theme: "light" } });
  await until(() => releaseBackendSave, "delayed save must start");
  applyAppSettings({ appearance: { language: "ko", theme: "dark" } });
  releaseBackendSave({ settings: { appearance: { language: "en", theme: "light" } } });
  const staleSave = await pendingSave;
  assert(staleSave.stale && staleSave.settings.appearance.language === "ko" && staleSave.savedSettings.appearance.language === "en" && getLanguage() === "ko", "stale save distinguishes the backend snapshot from the current preview without applying the old language");
  window.go = previousBridge;
  peer.contentWindow.localStorage.removeItem(settingsStorageKey);
  await until(() => getLanguage() === "en" && getCurrentAppSettings().appearance.language === "en", "removing cached settings restores the default language in another window");
  assert(document.querySelector("#runtimeStatus").textContent === literalError, "settings synchronization retains literal errors");
  const settingsFrame = document.createElement("iframe"); settingsFrame.src = "/localization-settings"; document.body.appendChild(settingsFrame);
  await until(() => settingsFrame.contentWindow?.localizationSettingsReads?.length === 1, "actual Settings initial read must start");
  peer.contentWindow.localStorage.setItem(settingsStorageKey, JSON.stringify({ appearance: { language: "ko", theme: "dark" } }));
  await until(() => settingsFrame.contentDocument?.documentElement.lang === "ko", "external settings apply during actual Settings initialization");
  settingsFrame.contentWindow.localizationSettingsReads[0]({ settings: { appearance: { language: "en", theme: "light" } } });
  await until(() => settingsFrame.contentDocument?.querySelector("#settingsForm"), "actual Settings form must initialize");
  const settingsWindow = settingsFrame.contentWindow;
  const settingsDocument = settingsFrame.contentDocument;
  const saves = settingsWindow.localizationSettingsSaves;
  const saveStatus = () => settingsDocument.querySelector("#settingsSaveStatus");
  const submitSettings = () => settingsDocument.querySelector("#settingsForm").dispatchEvent(new settingsWindow.Event("submit", { bubbles: true, cancelable: true }));
  const switchSettingsLanguage = (language) => {
    const select = settingsDocument.querySelector("#languageSelect"); select.value = language;
    select.dispatchEvent(new settingsWindow.Event("input", { bubbles: true }));
  };
  assert(settingsDocument.querySelector("#languageSelect").value === "ko" && settingsDocument.querySelector("#themeSelect").value === "dark", "a stale initial Settings response renders the latest external language and theme");
  switchSettingsLanguage("en");
  submitSettings(); await until(() => saves.length === 1, "first actual Settings save must start");
  switchSettingsLanguage("ko");
  assert(saveStatus().classList.contains("status-loading") && settingsDocument.documentElement.lang === "ko", "language change keeps the active saving indicator on the replacement Settings form: " + saveStatus().outerHTML + "; lang=" + settingsDocument.documentElement.lang);
  saves[0].resolve({ settings: saves[0].settings });
  await until(() => saveStatus().dataset.i18n === "status.settingsSavedEarlier", "stale save must show the saved earlier request guidance");
  assert(settingsDocument.querySelector("#languageSelect").value === "ko" && !saveStatus().classList.contains("status-loading"), "stale save retains the newer preview and never reports that it was saved");
  submitSettings(); await until(() => saves.length === 2, "second actual Settings save must start");
  switchSettingsLanguage("en"); saves[1].reject(new Error("visible-save-failure"));
  await until(() => saveStatus().textContent.includes("visible-save-failure"), "save rejection after a language change must reach the connected status element");
  assert(saveStatus().isConnected && settingsDocument.querySelector("#languageSelect").value === "en" && !saveStatus().classList.contains("status-loading"), "failed save preserves the newer form and clears its active indicator");
  submitSettings(); await until(() => saves.length === 3, "save queue must recover after rejection");
  switchSettingsLanguage("ko"); submitSettings(); await sleep(20);
  assert(saves.length === 3, "a second save waits for the earlier backend write to complete");
  saves[2].resolve({ settings: saves[2].settings });
  await until(() => saves.length === 4, "queued current save must start after the previous write");
  assert(settingsDocument.querySelector("#languageSelect").value === "ko" && saveStatus().dataset.i18n === "status.savingSettings", "earlier queued save cannot replace the current form or overwrite its saving status");
  saves[3].resolve({ settings: saves[3].settings });
  await until(() => saveStatus().dataset.i18n === "status.savedSettings", "latest queued save must report success");
  assert(saves[3].settings.appearance.language === "ko" && settingsDocument.querySelector("#languageSelect").value === "ko" && !saveStatus().classList.contains("status-loading"), "the last backend write and visible saved form agree");
  const externalSettings = { appearance: { language: "en", theme: "dark", graphFontSize: 17 }, profile: { numericTolerance: 0.02 } };
  peer.contentWindow.localStorage.setItem(settingsStorageKey, JSON.stringify(externalSettings));
  await until(() => settingsDocument.querySelector("#languageSelect").value === "en" && settingsDocument.querySelector("#themeSelect").value === "dark", "external settings synchronize all Settings controls after the locale rebuild");
  assert(settingsDocument.querySelector("#graphFontSize").value === "17" && settingsDocument.querySelector("#profileNumericTolerance").value === "0.02", "external settings form retains the full new appearance and profile rather than old form values");
  settingsFrame.remove();
  const mainFrame = document.createElement("iframe"); mainFrame.src = "/localization-index"; document.body.appendChild(mainFrame);
  await until(() => mainFrame.contentWindow?.localizationMainSettings, "actual Main module must initialize");
  const mainWindow = mainFrame.contentWindow;
  const main = mainWindow.localizationMainSettings;
  main.navigation.switchResultTab("profile", { recordHistory: false }); main.state.state.resultTabManuallySelected = false;
  assert(main.state.state.activeResultTab === "profile", "restored Profile tab is active before external settings change");
  const mainSettings = main.settings.getCurrentAppSettings(); mainSettings.appearance.language = "ko";
  mainWindow.localStorage.setItem(settingsStorageKey, JSON.stringify(mainSettings));
  mainWindow.dispatchEvent(new mainWindow.PageTransitionEvent("pageshow", { persisted: true }));
  await until(() => mainWindow.document.documentElement.lang === "ko", "restored Main applies external language");
  assert(main.state.state.activeResultTab === "profile" && mainWindow.document.querySelector('[data-result-tab="profile"]').classList.contains("active"), "external language synchronization preserves a restored or programmatically selected analysis tab");
  mainSettings.appearance.language = "fr";
  peer.contentWindow.localStorage.setItem(settingsStorageKey, JSON.stringify(mainSettings));
  await until(() => mainWindow.document.documentElement.lang === "fr", "active Main receives the settings storage event");
  assert(main.state.state.activeResultTab === "profile" && mainWindow.document.querySelector('[data-result-tab="profile"]').classList.contains("active"), "storage synchronization also preserves the current analysis tab");
  document.body.dataset.localizationStatus = "passed";
  output.textContent = JSON.stringify({ languages: supportedLanguages.length, dynamicMessages: true, settingsSynchronization: true, settingsSaveRaces: true, mainTabPreserved: true });
} catch (error) { document.body.dataset.localizationStatus = "failed"; output.textContent = error.stack || String(error); }
</script></body></html>`

const localizationSettingsSetupHTML = `<script>
window.localizationSettingsSaves = [];
window.localizationSettingsReads = [];
window.go = { main: { App: {
  GetSettings: () => new Promise((resolve) => window.localizationSettingsReads.push(resolve)),
  GetAppInfo: async () => ({ name: "SemanticIDF", version: "test" }),
  GetSimulationEnvironment: async () => ({ weatherFolders: [], installations: [], defaultWorkerCount: 1 }),
  SaveSettings: (settings) => new Promise((resolve, reject) => window.localizationSettingsSaves.push({ settings, resolve, reject })),
} } };
</script>`

const localizationMainSetupHTML = `<script>
window.go = { main: { App: {
  GetSettings: async () => ({ settings: { appearance: { language: "en", theme: "light" } } }),
  GetAppInfo: async () => ({ name: "SemanticIDF", version: "test" }),
  GetSimulationEnvironment: async () => ({ weatherFolders: [], installations: [], defaultWorkerCount: 1 }),
  AnalyzeInputText: async () => ({ report: null, model: null, epjson: "" }),
} } };
</script><script type="module">
import * as settings from "/src/js/settings-client.js";
import * as navigation from "/src/js/navigation.js";
import * as state from "/src/js/state.js";
window.addEventListener("load", () => { window.localizationMainSettings = { settings, navigation, state }; });
</script>`
