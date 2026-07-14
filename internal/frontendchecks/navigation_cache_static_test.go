package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSemanticNavigationMapCacheContract(t *testing.T) {
	cache := readTestFile(t, "frontend/src/js/semantic-navigation-cache.js")
	for _, required := range []string{
		"export function getSemanticNavigationCache",
		"export function getSemanticNavigationCacheStats",
		"new WeakMap()",
		"const sharedCachesByKey = new Map()",
		"MAX_SHARED_CACHE_ENTRIES",
		"metadata.textHash",
		"metadata.analyzerVersion",
		"entityById",
		"occurrenceById",
		"occurrenceIdsByEntityId",
		"occurrenceIdsByObjectId",
		"occurrenceIdsByObjectIndex",
		"occurrenceIdsByViewTarget",
		"sourceIdentityCandidates",
		"nearestParentOccurrence",
	} {
		if !strings.Contains(cache, required) {
			t.Fatalf("semantic navigation cache contract missing %q", required)
		}
	}
	if !strings.Contains(cache, "while (sharedCachesByKey.size > MAX_SHARED_CACHE_ENTRIES)") {
		t.Fatal("cross-projection navigation caches must use a bounded LRU")
	}

	build := readTestFile(t, "scripts/frontend-build.ps1")
	for _, module := range []string{"command-palette.js", "navigation-link-bar.js", "semantic-navigation-cache.js"} {
		if !strings.Contains(build, `"`+module+`"`) {
			t.Fatalf("frontend readiness manifest is missing %s", module)
		}
	}
}

func TestSemanticNavigationHotPathsUseCachedMaps(t *testing.T) {
	controller := readTestFile(t, "frontend/src/js/selection-controller.js")
	for _, required := range []string{
		`from "./semantic-navigation-cache.js"`,
		"cache.occurrenceIdsByEntityId.get",
		"cache.occurrenceIdsByObjectId.get",
		"cache.occurrenceIdsByObjectIndex.get",
		"cache.sourceIdentityCandidates(anchor)",
		"cache?.nearestParentOccurrence?.(semanticPathHint)",
	} {
		if !strings.Contains(controller, required) {
			t.Fatalf("selection controller cache wiring missing %q", required)
		}
	}
	sourceFallback := sliceBetween(controller, "function sourceFallbackOccurrence", "function sourceAnchorMatchesRenamedIdentity")
	if strings.Contains(sourceFallback, "index.occurrences.filter") || strings.Contains(sourceFallback, ".find(") {
		t.Fatal("source fallback must not scan the complete occurrence array")
	}

	panels := readTestFile(t, "frontend/src/js/panel-navigation-adapters.js")
	for _, required := range []string{
		`from "./semantic-navigation-cache.js"`,
		"cache.occurrenceIdsByViewTarget",
		"cache.occurrenceIdsByObjectId",
		"cache.occurrenceIdsByObjectIndex",
		"cache.occurrenceIdsByEntityId",
		"navigationCache(navigation).entity(entityId)",
		"navigationCache(navigation).occurrence(occurrenceId)",
	} {
		if !strings.Contains(panels, required) {
			t.Fatalf("result-panel cache wiring missing %q", required)
		}
	}

	navigation := readTestFile(t, "frontend/src/js/navigation.js")
	semanticOccurrences := sliceBetween(navigation, "function semanticOccurrences", "function semanticOccurrenceIDs")
	if !strings.Contains(semanticOccurrences, "cache.occurrencesForIDs") || strings.Contains(semanticOccurrences, "new Map") {
		t.Fatal("input occurrence lookup must reuse the semantic navigation cache")
	}

	linkBar := readTestFile(t, "frontend/src/js/navigation-link-bar.js")
	linkRender := sliceBetween(linkBar, "export function renderNavigationLinkBar", "function dispatchModeChange")
	if !strings.Contains(linkRender, "state.semanticProjection?.navigation") ||
		!strings.Contains(linkRender, "semanticNavigationCache()") {
		t.Fatal("analysis lifecycle rendering should prewarm the navigation Map cache before first selection")
	}
	availableTargets := sliceBetween(linkBar, "function availableTargets", "function semanticNavigationCache")
	if !strings.Contains(availableTargets, "cache.occurrenceIdsByEntityId.get") ||
		!strings.Contains(availableTargets, "cache.occurrence(occurrenceID)") ||
		strings.Contains(availableTargets, "new Map") {
		t.Fatal("link-bar selection rendering must use cached entity/occurrence indexes")
	}
}

func TestSelectionSyncAvoidsFullRenderAndBackendCalls(t *testing.T) {
	navigation := readTestFile(t, "frontend/src/js/navigation.js")
	for _, bounds := range [][2]string{
		{"export function handleAnalysisActivation", "export function handleInputSelectionActivation"},
		{"export function handleInputSelectionActivation", "export function refreshInputSelectionStyles"},
		{"export function refreshInputSelectionStyles", "let legacyInputAdaptersInitialized"},
	} {
		body := sliceBetween(navigation, bounds[0], bounds[1])
		if body == "" {
			t.Fatalf("selection-sync function %q is missing", bounds[0])
		}
		if strings.Contains(body, "renderReport(") {
			t.Fatalf("selection-sync function %q must not perform a full report render", bounds[0])
		}
		if regexp.MustCompile(`(?i)\b(?:analyze|analyzeinput|analyzeinputstagetext|fetch)\s*\(`).MatchString(body) {
			t.Fatalf("selection-sync function %q must not call a backend/analyzer", bounds[0])
		}
	}
	for _, path := range []string{
		"frontend/src/js/selection-controller.js",
		"frontend/src/js/panel-navigation-adapters.js",
		"frontend/src/js/navigation-link-bar.js",
	} {
		content := readTestFile(t, path)
		if strings.Contains(content, "renderReport(") || strings.Contains(content, "AnalyzeInput") {
			t.Fatalf("selection synchronization module %s must not full-render or call analysis", path)
		}
	}
}

func TestHistoryRestoreRevealsSelectionWithoutRenderOrAnalysis(t *testing.T) {
	navigation := readTestFile(t, "frontend/src/js/navigation.js")
	restore := sliceBetween(navigation, "export async function restoreViewSnapshot", "function restoreSemanticSnapshotState")
	if !strings.Contains(restore, "await revealRestoredSelection(scope)") {
		t.Fatal("history/workspace restore must resynchronize the selected entity after panel contexts")
	}
	reveal := sliceBetween(navigation, "async function revealRestoredSelection", "function restoreRawEditorPosition")
	for _, required := range []string{
		"getPanelNavigationAdapter(viewID)",
		"await adapter.canReveal(selection)",
		"await adapter.reveal(selection",
		"recordHistory: false",
		"follow: false",
		"preserveFilters: true",
		"scroll: false",
		"focus: false",
	} {
		if !strings.Contains(reveal, required) {
			t.Fatalf("restored selection reveal is missing %q", required)
		}
	}
	if strings.Contains(reveal, "renderReport(") ||
		regexp.MustCompile(`(?i)\b(?:analyze|analyzeinput|analyzeinputstagetext|fetch)\s*\(`).MatchString(reveal) {
		t.Fatal("restored selection reveal must not render the full report or call a backend")
	}
}

func TestResultPanelSelectionAccessibilityContract(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/panel-navigation-adapters.js")
	refresh := sliceBetween(content, "export function refreshResultPanelSelectionStyles", "function createResultPanelNavigationAdapter")
	for _, required := range []string{
		`item.tabIndex = 0`,
		`item.setAttribute("aria-selected", isSelected ? "true" : "false")`,
		`item.setAttribute("aria-current", "location")`,
		`item.removeAttribute("aria-current")`,
		`appendARIAReference(item, "aria-describedby", "semanticNavigationHelp")`,
	} {
		if !strings.Contains(refresh, required) {
			t.Fatalf("generic result-panel accessibility wiring missing %q", required)
		}
	}
	if strings.Contains(refresh, `setAttribute("role"`) {
		t.Fatal("generic accessibility refresh must preserve panel-owned roles")
	}
}

func TestSemanticNavigationCacheRuntimeAndLookupBudget(t *testing.T) {
	chrome := findHeadlessChrome()
	if chrome == "" {
		t.Skip("Chrome/Chromium is unavailable for the frontend cache runtime test")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/navigation-cache-test", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, navigationCacheRuntimePage)
	})
	mux.Handle("/", http.FileServer(http.Dir(repoPath("."))))
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	profileDir := t.TempDir()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new",
		"--disable-background-networking",
		"--disable-default-apps",
		"--disable-extensions",
		"--disable-gpu",
		"--disable-sync",
		"--no-first-run",
		"--no-default-browser-check",
		"--no-sandbox",
		"--user-data-dir="+profileDir,
		"--dump-dom",
		server.URL+"/navigation-cache-test",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("headless cache test timed out: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("headless cache test failed: %v\n%s", err, output)
	}
	dom := string(output)
	if !strings.Contains(dom, `data-test-status="pass"`) {
		t.Fatalf("semantic navigation cache runtime/performance contract failed:\n%s", dom)
	}
	if body := regexp.MustCompile(`(?s)<body[^>]*>(.*?)</body>`).FindStringSubmatch(dom); len(body) > 1 {
		t.Logf("frontend navigation cache performance: %s", body[1])
	}
}

func findHeadlessChrome() string {
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	if runtime.GOOS != "windows" {
		return ""
	}
	for _, path := range []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
	} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

const navigationCacheRuntimePage = `<!doctype html>
<html><head><meta charset="utf-8"><title>navigation cache test</title></head>
<body data-test-status="running">running
<script type="module">
import {
  getSemanticNavigationCache,
  getSemanticNavigationCacheStats,
  resetSemanticNavigationCacheForTests,
} from "/frontend/src/js/semantic-navigation-cache.js";
import { createSelectionController } from "/frontend/src/js/selection-controller.js";

resetSemanticNavigationCacheForTests();
const navigation = {
  entities: [{ id: "entity:zone", kind: "zone", label: "Zone" }],
  occurrences: [],
  byEntityId: { "entity:zone": [] },
  byObjectId: {},
  byObjectIndex: {},
  byViewTarget: {},
};
for (let index = 0; index < 12000; index += 1) {
  const occurrenceId = ` + "`occ:${index}`" + `;
  const occurrence = {
    occurrenceId,
    entityId: "entity:zone",
    path: ` + "`zones/${index % 500}/profile/${index}`" + `,
    sourceAnchor: {
      objectId: ` + "`object:${index}`" + `,
      objectIndex: index,
      objectType: "Zone",
      objectName: ` + "`Zone ${index % 500}`" + `,
      fieldIndex: index % 20,
    },
    viewTargets: [{ view: "profile", targetId: ` + "`profile:${index % 500}`" + ` }],
  };
  navigation.occurrences.push(occurrence);
  navigation.byEntityId["entity:zone"].push(occurrenceId);
  navigation.byObjectId[occurrence.sourceAnchor.objectId] = [occurrenceId];
  navigation.byObjectIndex[String(index)] = [occurrenceId];
  navigation.byViewTarget[` + "`profile|profile:${index % 500}`" + `] ||= [];
  navigation.byViewTarget[` + "`profile|profile:${index % 500}`" + `].push(occurrenceId);
}
navigation.occurrences.push({
  occurrenceId: "parent:12",
  entityId: "entity:zone",
  path: "zones/12/profile",
  sourceAnchor: { objectType: "Zone", objectName: "Zone 12" },
});
navigation.byEntityId["entity:zone"].push("parent:12");

const projection = { schema: "eplus-semantic/0.2", navigation };
const metadata = { textHash: "large-model-hash", analyzerVersion: "0.4.2" };
const buildStart = performance.now();
const cache = getSemanticNavigationCache(projection, metadata);
const buildElapsedMs = performance.now() - buildStart;
for (let repeat = 0; repeat < 1000; repeat += 1) {
  getSemanticNavigationCache(projection, metadata);
}
const equivalentProjection = JSON.parse(JSON.stringify(projection));
const equivalentCache = getSemanticNavigationCache(equivalentProjection, metadata);
let remapReason = "";
const controllerState = {
  semanticProjection: equivalentProjection,
  reportAnalysisKey: metadata.textHash,
  globalSelection: {
    entityId: "missing:renamed",
    entityKind: "zone",
    occurrenceId: "missing:occurrence",
    sourceAnchor: { objectType: "zone", objectName: "Zone 123", fieldIndex: 3 },
    semanticPathHint: "zones/123/profile/123",
  },
};
const controller = createSelectionController({
  state: controllerState,
  getNavigationIndex: () => equivalentProjection.navigation,
  getReportAnalysisKey: () => metadata.textHash,
  isAnalysisCurrent: () => true,
  onSelectionRemapped: ({ reason }) => { remapReason = reason; },
});
const remappedSelection = await controller.remapSemanticSelection({ recordHistory: false, follow: false });
const stableStats = getSemanticNavigationCacheStats();

let checksum = 0;
const lookupStart = performance.now();
for (let repeat = 0; repeat < 50000; repeat += 1) {
  checksum += cache.occurrence(` + "`occ:${repeat % 12000}`" + `)?.sourceAnchor?.objectIndex || 0;
}
const lookupElapsedMs = performance.now() - lookupStart;
const sourceCandidates = cache.sourceIdentityCandidates({
  objectType: "zone",
  objectName: "Zone 123",
  fieldIndex: 3,
});
const parent = cache.nearestParentOccurrence("zones/12/profile/use/definition");

for (let index = 0; index < 10; index += 1) {
  getSemanticNavigationCache({
    navigation: {
      entities: [{ id: ` + "`tiny:${index}`" + ` }],
      occurrences: [],
      byEntityId: {},
    },
  }, { textHash: ` + "`tiny-hash:${index}`" + `, analyzerVersion: "0.4.2" });
}
const boundedStats = getSemanticNavigationCacheStats();
const passed = Boolean(
  cache === getSemanticNavigationCache(projection, metadata) &&
  cache === equivalentCache &&
  stableStats.builds === 1 &&
  stableStats.sharedHits >= 1 &&
  cache.entity("entity:zone")?.label === "Zone" &&
  cache.occurrence("occ:11999")?.entityId === "entity:zone" &&
  sourceCandidates.some((candidate) => candidate.sourceAnchor?.objectName === "Zone 123") &&
  parent?.occurrenceId === "parent:12" &&
  remapReason === "source" &&
  remappedSelection?.entityId === "entity:zone" &&
  remappedSelection?.occurrenceId === "occ:123" &&
  lookupElapsedMs <= 50 &&
  checksum > 0 &&
  boundedStats.sharedEntries <= boundedStats.sharedEntryLimit
);
document.body.dataset.testStatus = passed ? "pass" : "fail";
document.body.textContent = JSON.stringify({
  passed,
  buildElapsedMs: Number(buildElapsedMs.toFixed(2)),
  lookupElapsedMs: Number(lookupElapsedMs.toFixed(2)),
  stableStats,
  boundedStats,
  sourceCandidateCount: sourceCandidates.length,
  parent: parent?.occurrenceId || "",
  remapReason,
  remappedOccurrence: remappedSelection?.occurrenceId || "",
});
</script></body></html>`
