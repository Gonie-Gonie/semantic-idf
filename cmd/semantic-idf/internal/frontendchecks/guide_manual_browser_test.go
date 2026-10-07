package frontendchecks

import (
	"context"
	"encoding/json"
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

func TestGuideManualNavigationSearchAndSafeMarkdownBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping technical reference browser harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	// Read the runtime inputs so Go's cache invalidates when the renderer or
	// application shell changes, even though Chromium opens them over HTTP.
	for _, path := range []string{"frontend/src/guide.html", "frontend/src/js/guide-manual.js", "frontend/src/js/auxiliary-navigation.js", "frontend/src/js/app-info.js", "frontend/src/js/settings-client.js", "frontend/src/js/i18n.js", "frontend/src/styles/guide-manual.css"} {
		readTestFile(t, path)
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/manual-harness", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, guideManualBrowserHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox",
		"--no-first-run", "--no-default-browser-check", "--virtual-time-budget=30000",
		"--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/manual-harness",
	)
	output, err := command.CombinedOutput()
	if err != nil || ctx.Err() != nil {
		t.Fatalf("technical reference harness failed: %v, %v\n%s", err, ctx.Err(), output)
	}
	if !strings.Contains(string(output), `data-guide-test-status="passed"`) {
		t.Fatalf("technical reference harness did not pass:\n%s", output)
	}
	for _, signal := range []string{`"cachedChapters":true`, `"safeMarkdown":true`, `"searchAndCatalog":true`, `"language":true`, `"mainReturn":true`} {
		if !strings.Contains(string(output), signal) {
			t.Fatalf("technical reference result missing %s:\n%s", signal, output)
		}
	}
}

func TestGuideManualBundledReferenceBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bundled technical reference browser in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	manualRoot := repoPath("frontend/src/manual")
	manifestBytes, err := os.ReadFile(filepath.Join(manualRoot, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Chapters []struct {
			File map[string]string `json:"file"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, chapter := range manifest.Chapters {
		for _, language := range []string{"en", "ko"} {
			if _, err := os.ReadFile(filepath.Join(manualRoot, chapter.File[language])); err != nil {
				t.Fatal(err)
			}
		}
	}
	readTestFile(t, "frontend/src/manual/metric-guides.json")
	for _, path := range []string{"frontend/src/guide.html", "frontend/src/js/guide-manual.js", "frontend/src/js/auxiliary-navigation.js", "frontend/src/js/app-info.js", "frontend/src/js/settings-client.js", "frontend/src/js/i18n.js", "frontend/src/styles/guide-manual.css"} {
		readTestFile(t, path)
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/api/settings", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"settings":{"appearance":{"language":"ko","theme":"dark"}}}`)
	})
	mux.HandleFunc("/api/metric-guides", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/bundled-manual-harness", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, guideManualBundledBrowserHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox",
		"--no-first-run", "--no-default-browser-check", "--virtual-time-budget=30000",
		"--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/bundled-manual-harness",
	)
	output, err := command.CombinedOutput()
	if err != nil || ctx.Err() != nil {
		t.Fatalf("bundled technical reference browser failed: %v, %v\n%s", err, ctx.Err(), output)
	}
	if !strings.Contains(string(output), `data-bundled-guide-status="passed"`) {
		t.Fatalf("bundled technical reference browser did not pass:\n%s", output)
	}
}

const guideManualBundledBrowserHarnessHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Bundled manual</title></head>
<body data-bundled-guide-status="pending"><pre id="bundledResult">pending</pre>
<iframe id="manualFrame" src="/src/guide.html?lang=ko#metrics/metric-catalog" width="1400" height="900"></iframe>
<script type="module">
const output = document.querySelector("#bundledResult");
const frame = document.querySelector("#manualFrame");
const assert = (value, message) => { if (!value) throw new Error(message); };
const sleep = (ms = 20) => new Promise((resolve) => setTimeout(resolve, ms));
async function until(condition, message) { for (let n = 0; n < 250; n += 1) { if (condition()) return; await sleep(); } throw new Error(message); }
const doc = () => frame.contentDocument;
try {
  await until(() => doc()?.querySelectorAll("#metricGuide .guide-metric-card").length === 59, "offline bundled catalog must load all 59 definitions");
  assert(doc().querySelector("#manualContent").lang === "ko", "Korean deep-linked chapter is rendered");
  assert(doc().querySelector("#manualChapters a[aria-current]").getAttribute("href") === "#metrics", "deep link chapter is active");
  assert(doc().querySelectorAll("#metricGuide dd").length === 59 * 5, "all catalog units and interpretation fields are present");
  doc().querySelector("#metricGuide .guide-metric-card:last-child").scrollIntoView();
  await sleep(80);
  assert(doc().querySelector("#manualSections [aria-current=location]")?.dataset.section === "metric-catalog", "catalog cards keep the catalog section active while scrolling");
  await until(() => doc().documentElement.dataset.theme === "dark", "saved app theme should apply");
  frame.width = "480";
  frame.getBoundingClientRect(); doc().body.offsetWidth; frame.contentWindow.dispatchEvent(new Event("resize"));
  await until(() => !doc().querySelector("#manualChapterDisclosure").open && !doc().querySelector("#manualSectionDisclosure").open, "narrow screen starts with compact chapter/section disclosures");
  assert(doc().querySelector("#manualChapters").getBoundingClientRect().height === 0, "closed chapter navigation does not occupy mobile content space");
  const summary = doc().querySelector("#manualChapterDisclosure summary"); summary.focus(); summary.click();
  await until(() => doc().querySelector("#manualChapterDisclosure").open, "native disclosure expands chapter navigation");
  assert(doc().activeElement === summary, "native summary supports keyboard focus");
  frame.width = "1400";
  frame.getBoundingClientRect(); doc().body.offsetWidth; frame.contentWindow.dispatchEvent(new Event("resize"));
  await until(() => doc().querySelector("#manualChapterDisclosure").open && doc().querySelector("#manualSectionDisclosure").open, "desktop always expands both tables of contents");
  frame.width = "480";
  frame.getBoundingClientRect(); doc().body.offsetWidth; frame.contentWindow.dispatchEvent(new Event("resize"));
  await until(() => doc().querySelector("#manualChapterDisclosure").open && !doc().querySelector("#manualSectionDisclosure").open, "mobile disclosure preferences survive resizing");
  assert(doc().documentElement.scrollWidth <= frame.contentWindow.innerWidth, "narrow manual has no page horizontal overflow");
  frame.width = "1400";
  frame.getBoundingClientRect(); doc().body.offsetWidth; frame.contentWindow.dispatchEvent(new Event("resize"));
  await until(() => frame.contentWindow.innerWidth > 780, "restore desktop width");
  const language = doc().querySelector("#manualLanguage"); language.value = "en"; language.dispatchEvent(new Event("change", { bubbles: true }));
  await until(() => doc().querySelector("#manualContent").lang === "en" && doc().querySelectorAll("#metricGuide .guide-metric-card").length === 59, "English source should replace Korean source");
  const chapters = [...doc().querySelectorAll("#manualChapters a")].map((link) => link.getAttribute("href"));
  assert(chapters.length === 10, "all ten chapters are navigable");
  for (const href of chapters) {
    doc().querySelector('#manualChapters a[href="' + href + '"]').click();
    await until(() => doc().querySelector("#manualChapters a[aria-current]")?.getAttribute("href") === href && doc().querySelector("#manualContent h1"), "chapter did not render: " + href);
    assert(doc().querySelector("#manualContent").textContent.length > 400, "chapter has substantial technical content: " + href);
    assert(doc().querySelectorAll("#manualSections a").length > 1, "chapter has section navigation: " + href);
    assert(!doc().querySelector("#manualStatus").dataset.error, "chapter loaded without errors: " + href);
  }
  const search = doc().querySelector("#manualSearch"); search.value = "missing"; search.dispatchEvent(new Event("input", { bubbles: true }));
  await until(() => doc().querySelectorAll("#manualResultsList .manual-search-result").length > 1, "whole manual search must find missing-data interpretation");
  assert(doc().querySelector("#manualResultsList mark"), "bundled manual search highlights a match");
  const first = doc().querySelector("#manualResultsList .manual-search-result"); const destination = first.getAttribute("href"); first.click();
  await until(() => frame.contentWindow.location.hash === destination && doc().querySelector("#manualSearchResults").hidden, "search result opens its section");
  const beforeReload = frame.contentWindow.location.hash;
  await new Promise((resolve) => { frame.addEventListener("load", resolve, { once: true }); frame.contentWindow.location.reload(); });
  await until(() => doc().querySelector("#manualContent h1"), "reload restores deep-linked manual");
  assert(frame.contentWindow.location.hash === beforeReload && doc().querySelector("#manualContent").lang === "en", "reload preserves chapter, section and explicit language");
  const resources = frame.contentWindow.performance.getEntriesByType("resource");
  assert(resources.every((entry) => new URL(entry.name).origin === location.origin), "manual loads without remote assets");
  document.body.dataset.bundledGuideStatus = "passed"; output.textContent = JSON.stringify({ chapters: chapters.length, catalog: 59, offline: true, search: true, reload: true });
} catch (error) { document.body.dataset.bundledGuideStatus = "failed"; output.textContent = error.stack || String(error); }
</script></body></html>`

const guideManualBrowserHarnessHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Manual harness</title></head>
<body data-guide-test-status="pending"><pre id="testResult">pending</pre><div id="manualTestRoot"></div>
<script type="module">
const result = document.querySelector("#testResult");
const assert = (value, message) => { if (!value) throw new Error(message); };
const sleep = (ms = 10) => new Promise((resolve) => setTimeout(resolve, ms));
async function until(condition, message) { for (let n = 0; n < 100; n += 1) { if (condition()) return; await sleep(); } throw new Error(message); }
try {
  const { GuideManual, renderManualMarkdown } = await import("/src/js/guide-manual.js");
  const shell = new DOMParser().parseFromString(await (await fetch("/src/guide.html")).text(), "text/html");
  const root = document.querySelector("#manualTestRoot");
  for (const script of shell.querySelectorAll("script")) script.remove();
  root.innerHTML = shell.body.innerHTML;
  root.querySelector("[data-app-return]").setAttribute("href", "/main-sentinel");
  const ids = ["getting-started", "input", "metrics"];
  const manifest = { version: 1, defaultLanguage: "en", chapters: ids.map((id) => ({ id, title: { en: "Chapter " + id, ko: "한국어 " + id }, file: { en: id + ".en.md", ko: id + ".ko.md" } })) };
  const scriptText = "<scr" + "ipt>literal</scr" + "ipt>";
  const fixture = [
    "# Input reference", "", "## Editing {#editing}", "", "A **field value** has *meaning*, and <img src=x onerror=alert(1)> is literal text.",
    "", "1. Open the model", "   - Find the field", "   - Read its unit", "2. Save the model", "", "## Formula {#formula}", "",
    "| Quantity | Calculation |", "| --- | ---: |", "| Area | " + String.fromCharCode(96) + "width | length" + String.fromCharCode(96) + " |", "",
    String.fromCharCode(96).repeat(3) + "python", "# not a heading", "print('" + scriptText + "')", String.fromCharCode(96).repeat(3), "",
    "[Catalog](./metrics.en.md#metric-catalog) [Unsafe](javascript:alert(1)) [Unsafe encoded](%6aavascript:alert(1))", "",
    "## Duplicate", "## Duplicate",
  ].join("\n");
  const sources = {
    "getting-started.en.md": "# Getting started\n\n## Open {#open}\n\nRead the [input](./input.en.md#editing).",
    "getting-started.ko.md": "# 시작\n\n## 열기 {#open}\n\n입력 열기.",
    "input.en.md": fixture, "input.ko.md": fixture.replace("Input reference", "입력 참고서").replace("A **field value**", "한국어 **필드 값**"),
    "metrics.en.md": "# Metrics\n\n## Metric catalog {#metric-catalog}\n\nLive sources and formulas.",
    "metrics.ko.md": "# 지표\n\n## 지표 카탈로그 {#metric-catalog}\n\n원본과 계산 방법.",
  };
  const calls = new Map();
  const guides = [{ id: "floor_area", name: "Floor <scr" + "ipt>area</scr" + "ipt>", category: "Envelope", unit: "m²", source: "variable source", method: "polygon area", assumptions: "planar surfaces", missingData: "unavailable" }];
  const fetcher = async (url) => {
    calls.set(url, (calls.get(url) || 0) + 1);
    const file = url.split("/").at(-1);
    if (file === "manifest.json") return { ok: true, json: async () => manifest };
    if (url === "/api/metric-guides") return { ok: true, json: async () => guides };
    if (Object.hasOwn(sources, file)) return { ok: true, text: async () => sources[file] };
    return { ok: false };
  };
  history.replaceState({}, "", "/main-sentinel");
  history.pushState({ manualDepth: 0 }, "", "/manual-harness#input/formula");
  sessionStorage.setItem("idfAnalyzer.auxiliaryNavigation", "main");
  const manual = new GuideManual({ root, fetcher, sourceURL: "/manual-fixture/" });
  await manual.initialize();
  assert(root.querySelector("#manualChapters [aria-current=page]").getAttribute("href") === "#input", "deep link selects its chapter");
  assert(root.querySelector("#manualSections [aria-current=location]").dataset.section === "formula", "deep link selects its section");
  assert(root.querySelectorAll("#manualContent table").length === 1, "GFM table rendered");
  assert(root.querySelector("#manualContent td code").textContent === "width | length", "code pipe does not split a table cell");
  assert(root.querySelector("#manualContent ol > li > ul").children.length === 2, "nested ordered steps rendered");
  assert(root.querySelector("#manualContent pre code").textContent.includes(scriptText), "code stays literal");
  assert(root.querySelector("#manualContent img,#manualContent script") === null, "raw HTML is escaped");
  assert(![...root.querySelectorAll("#manualContent a")].some((a) => /javascript:/i.test(a.href)), "unsafe links are blocked");
  assert(root.querySelector("#duplicate-1"), "duplicate headings have unique slugs");
  assert(!root.querySelector("#manualSections").textContent.includes("not a heading"), "fenced code is absent from the TOC");
  const stable = renderManualMarkdown("## 안정된 제목 {#stable}\n\n**safe**", { chapter: "input" });
  assert(stable.headings[0].id === "stable", "explicit translated heading anchor is stable");
  root.dispatchEvent(new KeyboardEvent("keydown", { key: "/", bubbles: true }));
  assert(document.activeElement === root.querySelector("#manualSearch"), "keyboard search focus");
  root.querySelector("#manualSearch").value = "variable";
  const found = await manual.search("variable");
  assert(found.some((item) => item.section === "metric-floor_area"), "catalog included in global search");
  assert(root.querySelector("#manualResultsList mark").textContent.toLowerCase() === "variable", "safe search highlighting");
  root.querySelector("#manualSearch").value = "field value";
  await manual.search("field value");
  assert(root.querySelector("#manualResultsList").textContent.includes("field value"), "chapter prose is searchable");
  root.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  assert(root.querySelector("#manualSearchResults").hidden, "Escape clears results");
  manual.navigate("#metrics/metric-floor_area");
  await until(() => root.querySelector("#metric-floor_area"), "metric catalog did not mount");
  assert(root.querySelector("#metric-floor_area dd").textContent === "m²", "live unit metadata preserved");
  assert(root.querySelectorAll("#metric-floor_area dd")[1].textContent === "variable source", "live source metadata preserved");
  assert(!root.querySelector("#metricGuide script"), "catalog values escaped");
  manual.navigate("#input/editing");
  await until(() => root.querySelector("#editing"), "input chapter did not return");
  assert(calls.get("/manual-fixture/input.en.md") === 1, "chapter parsed and fetched once");
  assert(calls.get("/api/metric-guides") === 1, "catalog fetched once across search/navigation");
  history.back();
  await until(() => location.hash === "#metrics/metric-floor_area" && root.querySelector("#metric-floor_area"), "browser back restores chapter");
  history.forward();
  await until(() => location.hash === "#input/editing" && root.querySelector("#editing"), "browser forward restores chapter");
  const depthBeforeLanguage = history.state.manualDepth;
  await manual.setLanguage("ko");
  assert(root.querySelector("#manualContent").textContent.includes("입력 참고서"), "localized chapter source loaded");
  assert(root.querySelector("#manualChapters").textContent.includes("한국어"), "localized chapter titles");
  assert(root.querySelector("#manualContent").lang === "ko", "content language declared");
  assert(history.state.manualDepth === depthBeforeLanguage && location.hash === "#input/editing", "language change keeps history depth and section");
  await manual.setLanguage("en");
  assert(calls.get("/manual-fixture/input.en.md") === 1, "language switch reuses cache");
  const staleSearch = manual.search("field"); manual.clearSearch();
  assert((await staleSearch).length === 0 && root.querySelector("#manualSearchResults").hidden, "cleared search cannot restore stale results");
  const oldBridge = window.go;
  window.go = { main: { App: { GetMetricGuides: async () => guides } } };
  const bridgeManual = new GuideManual({ root, fetcher: async () => { throw new Error("bridge should serve catalog"); } });
  assert(!(await bridgeManual.loadMetricCatalog()).bundled, "Wails catalog bridge remains supported");
  window.go = { main: { App: { GetMetricGuides: async () => { throw new Error("offline bridge"); } } } };
  const offlineManual = new GuideManual({ root, sourceURL: "/offline/", fetcher: async (url) => ({ ok: url === "/offline/metric-guides.json", json: async () => guides }) });
  assert((await offlineManual.loadMetricCatalog()).bundled, "bundled catalog fallback remains available offline");
  window.go = oldBridge;
  let failedAttempts = 0;
  const retryManual = new GuideManual({ root, fetcher: async () => ({ ok: ++failedAttempts > 1, text: async () => "# Retry" }) });
  retryManual.chapterIds = new Set(ids);
  let rejected = false; try { await retryManual.loadChapter(manifest.chapters[1]); } catch { rejected = true; }
  assert(rejected && (await retryManual.loadChapter(manifest.chapters[1])).headings[0].title === "Retry", "failed chapter fetch can be retried");
  const expectedDepth = depthBeforeLanguage + 1;
  assert(Number(root.querySelector("[data-app-return]").dataset.appReturnDepth) === expectedDepth, "return depth accounts for Guide history");
  const { auxiliaryReturnDepth } = await import("/src/js/auxiliary-navigation.js");
  const toolsState = { ...history.state, manualDepth: undefined };
  history.replaceState(toolsState, "", "/tools-sentinel");
  sessionStorage.setItem("idfAnalyzer.auxiliaryReturnDepth", "1");
  assert(auxiliaryReturnDepth() === expectedDepth, "auxiliary entry depth outranks mutable session fallback");
  history.back();
  await until(() => location.hash === "#metrics/metric-floor_area", "back from replaced Tools restores earlier Guide route");
  await manual.openRoute();
  await until(() => root.querySelector("#metric-floor_area"), "back from replaced Tools restores earlier Guide chapter");
  // The real Tools page has its own document, and therefore no Guide handler.
  window.removeEventListener("hashchange", manual.onHashChange);
  history.forward();
  await until(() => location.pathname === "/tools-sentinel", "forward returns to replaced Tools entry");
  await sleep(20);
  assert(auxiliaryReturnDepth() === expectedDepth, "Tools entry retains its Main return depth after Guide back/forward");
  root.querySelector("[data-app-return]").removeAttribute("data-app-return-depth");
  root.querySelector("[data-app-return]").click();
  await until(() => location.pathname === "/main-sentinel", "Back to App did not return to live Main");
  assert(sessionStorage.getItem("idfAnalyzer.auxiliaryNavigation") === null, "return clears auxiliary session marker");
  const signals = { cachedChapters: true, safeMarkdown: true, searchAndCatalog: true, language: true, mainReturn: true };
  document.body.dataset.guideTestStatus = "passed";
  result.textContent = JSON.stringify(signals);
} catch (error) { document.body.dataset.guideTestStatus = "failed"; result.textContent = error.stack || String(error); }
</script></body></html>`
