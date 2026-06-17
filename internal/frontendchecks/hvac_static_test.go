package frontendchecks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrontendHVACDefaultUICopyAvoidsDebugAndLegacyTerms(t *testing.T) {
	files := []string{
		repoPath("frontend/src/js/views/hvac-views.js"),
		repoPath("frontend/src/js/i18n.js"),
		repoPath("frontend/src/js/state.js"),
	}
	forbidden := []string{
		"Rule edges",
		"Rule trace",
		"Rule path",
		"Terminal / Equipment",
		"Plant / Condenser",
		"terminal:direct",
		"terminalComponents",
		"buildRelationGraph",
		"plant-terminal",
		"source-zone",
		`data-hvac-open-view="relation"`,
		"relation-link:",
		"Zone relations",
		"Other loops",
		"hvac.inferred",
		"Inferred",
		"Cross-loop",
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(content)
		for _, term := range forbidden {
			if strings.Contains(text, term) {
				t.Fatalf("%s contains forbidden HVAC default UI copy %q", file, term)
			}
		}
	}
}

func TestFrontendHVACStartsOnZoneServices(t *testing.T) {
	content, err := os.ReadFile(repoPath("frontend/src/js/state.js"))
	if err != nil {
		t.Fatalf("read state.js: %v", err)
	}
	if !strings.Contains(string(content), `activeHVACView: "services"`) {
		t.Fatalf("state.js should default HVAC to Zone Services view")
	}
	for _, required := range []string{
		"activeHVACEntity",
		"activeHVACContext",
		"hvacNavigationStack",
		"hvacForwardStack",
		`activeHVACGraphScope: "focused"`,
	} {
		if !strings.Contains(string(content), required) {
			t.Fatalf("state.js should include HVAC navigation state %q", required)
		}
	}
	if !strings.Contains(string(content), `hvacGraphScale: "actual"`) {
		t.Fatalf("state.js should default HVAC graph to actual scale")
	}
}

func TestFrontendHVACServiceDOMContracts(t *testing.T) {
	content, err := os.ReadFile(repoPath("frontend/src/js/views/hvac-views.js"))
	if err != nil {
		t.Fatalf("read hvac views: %v", err)
	}
	text := string(content)
	for _, required := range []string{
		"function buildServiceGraph(paths, couplings)",
		"function serviceGraphNodeIdentity",
		"function layoutServiceGraphNodes",
		"function alignServiceGraphColumnRows",
		"function clearHVACGraphSelection",
		"function isPhysicalServiceCoupling",
		"function serviceLinkPath",
		"function bundleServiceGraphLinks",
		"function navigateHVAC(target = {}, options = {})",
		"function backHVAC()",
		"function forwardHVAC()",
		"function clearHVACFocus()",
		"function pathsForActiveHVACEntity",
		"function renderHVACGraphScopeControls",
		"function renderHVACBreadcrumbBar",
		"function orthogonalPath",
		`event.key === "Escape"`,
		`state.activeHVACGraphKey = ""`,
		`state.activeHVACNodeName = ""`,
		"hvac-service-svg",
		"hvac-edge-bundle-badge",
		"hvac-trace-drawer",
		"evaporative_cooler",
		"renderHVACViewTab(\"services\"",
		"renderHVACViewTab(\"couplings\"",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("hvac service renderer is missing DOM contract %q", required)
		}
	}
}

func TestFrontendHVACZoneServicesIsGraphOnlyAndPhysicalObjectBased(t *testing.T) {
	content, err := os.ReadFile(repoPath("frontend/src/js/views/hvac-views.js"))
	if err != nil {
		t.Fatalf("read hvac views: %v", err)
	}
	text := string(content)
	for _, forbidden := range []string{
		"renderHVACServiceTable",
		"renderHVACServiceRow",
		"hvac-service-table-row",
		"`service-node:${path.id}:",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("HVAC Zone Services graph should not use table/path-scoped physical nodes: found %q", forbidden)
		}
	}
	for _, required := range []string{
		"serviceGraphNodeKey(path, spec)",
		"coupling.placementHint === \"detail_only\"",
		"type === \"operation_scheme\"",
		"physicalSupportingCouplings(path, couplingById)",
		"servicePathLinkKeys(path, couplingById)",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("HVAC Zone Services physical graph contract missing %q", required)
		}
	}
}

func TestFrontendHVACServiceStylesCoverRoutingAndBundling(t *testing.T) {
	content, err := os.ReadFile(repoPath("frontend/src/styles/hvac.css"))
	if err != nil {
		t.Fatalf("read HVAC styles: %v", err)
	}
	text := string(content)
	for _, required := range []string{
		".hvac-graph-link.service.bundled",
		".hvac-edge-bundle-badge",
		".hvac-service-link-group:hover .hvac-edge-bundle-badge",
		".hvac-service-link-group:hover .hvac-edge-label",
		".hvac-graph-link.medium-chilled-water",
		".hvac-graph-link.medium-hot-water",
		".hvac-graph-link.medium-refrigerant",
		".hvac-graph-link.medium-electricity",
		".hvac-graph-link.medium-control",
		".hvac-graphic-shell.scale-actual .hvac-service-svg",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("hvac service styles are missing %q", required)
		}
	}
}

func TestFrontendHVACGraphAreaOwnsScroll(t *testing.T) {
	content, err := os.ReadFile(repoPath("frontend/src/styles/hvac.css"))
	if err != nil {
		t.Fatalf("read HVAC styles: %v", err)
	}
	text := string(content)
	for _, required := range []string{
		".hvac-pane {\n  flex: 1;\n  display: flex;\n  flex-direction: column;",
		".hvac-layout {\n  flex: 1 1 auto;\n  min-height: 0;",
		".hvac-main {\n  display: flex;\n  flex-direction: column;",
		".hvac-graph {\n  flex: 1 1 auto;\n  min-height: 0;\n  overflow: auto;",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("HVAC graph scroll contract missing %q", required)
		}
	}
}

func TestFrontendHVACRendererAvoidsResolverConfidenceVocabulary(t *testing.T) {
	content, err := os.ReadFile(repoPath("frontend/src/js/views/hvac-views.js"))
	if err != nil {
		t.Fatalf("read hvac views: %v", err)
	}
	text := strings.ToLower(string(content))
	for _, forbidden := range []string{"confidence", "inferred", "weak", "unsupported"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("hvac renderer contains resolver confidence vocabulary %q", forbidden)
		}
	}
}

func TestFrontendHVACRendererAvoidsLegacyRelationGraphImplementation(t *testing.T) {
	content, err := os.ReadFile(repoPath("frontend/src/js/views/hvac-views.js"))
	if err != nil {
		t.Fatalf("read hvac views: %v", err)
	}
	text := string(content)
	for _, forbidden := range []string{
		"selected.relations",
		"ruleEdgeCountLabel",
		"ruleEdgeSummary",
		"ruleEdgesForRelation(",
		`t("hvac.terminals"`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("hvac renderer still contains legacy relation implementation %q", forbidden)
		}
	}
}

func repoPath(path string) string {
	return filepath.Join("..", "..", filepath.FromSlash(path))
}
