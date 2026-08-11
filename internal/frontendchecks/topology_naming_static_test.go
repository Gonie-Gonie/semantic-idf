package frontendchecks

import (
	"strings"
	"testing"
)

func TestTopologyNamingKeepsGeometryCompatibilityContracts(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	for _, required := range []string{
		`data-result-tab="geometry"`,
		`id="geometryPane"`,
		`data-i18n="tab.geometry">Topology`,
	} {
		if !strings.Contains(index, required) {
			t.Fatalf("topology panel compatibility or naming contract is missing %q", required)
		}
	}

	translations := readTestFile(t, "frontend/src/js/i18n.js")
	for _, required := range []string{
		`"tab.geometry": "Topology"`,
		`"topology.panelTitle": "Spatial & Thermal Topology"`,
		`"tab.geometry": "공간·열 연결"`,
		`"topology.panelTitle": "공간·열 토폴로지"`,
		`"shortcut.tabGeometry": "Analyze tab: Topology"`,
	} {
		if !strings.Contains(translations, required) {
			t.Fatalf("topology translation contract is missing %q", required)
		}
	}

	shortcuts := readTestFile(t, "frontend/src/js/shortcuts.js")
	if !strings.Contains(shortcuts, `tabGeometry: () => actions.switchResultTab?.("geometry")`) {
		t.Fatal("legacy tabGeometry shortcut must continue to activate the geometry result-tab ID")
	}

	state := readTestFile(t, "frontend/src/js/state.js")
	if !strings.Contains(state, `document.querySelectorAll("[data-result-tab]")`) {
		t.Fatal("result-tab automation must use the stable data-result-tab contract")
	}
}

func TestResultTabsAreTheOnlyTopLevelPanelNames(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	for _, duplicate := range []string{
		`<h2 data-i18n="tab.summary"`,
		`<h2 data-i18n="tab.profile"`,
		`<h2 data-i18n="tab.hvac"`,
		`<h2 data-i18n="tab.diagnose"`,
		`<h2 data-i18n="simulation.runInspect"`,
		`<h2 data-i18n="topology.panelTitle"`,
	} {
		if strings.Contains(index, duplicate) {
			t.Fatalf("result panel repeats its tab name with %q", duplicate)
		}
	}
}

func TestTopologyHeaderWrapsWithinTheAnalysisPanel(t *testing.T) {
	styles := readTestFile(t, "frontend/src/styles/geometry.css")
	for _, required := range []string{
		"grid-template-columns: minmax(0, 1fr)",
		"width: 100%",
		"justify-content: flex-start",
		"flex-wrap: wrap",
		"flex: 1 1 680px",
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("Topology header is missing panel-responsive layout rule %q", required)
		}
	}
}
