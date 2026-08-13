package frontendchecks

import (
	"strings"
	"testing"
)

func TestTopologyNamingUsesTopologyPanelContracts(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	for _, required := range []string{
		`data-result-tab="topology"`,
		`id="topologyPane"`,
		`data-i18n="tab.topology">Topology`,
	} {
		if !strings.Contains(index, required) {
			t.Fatalf("topology panel naming contract is missing %q", required)
		}
	}

	translations := readTestFile(t, "frontend/src/js/i18n.js")
	for _, required := range []string{
		`"tab.topology": "Topology"`,
		`"tab.topology": "공간·열 연결"`,
		`"shortcut.tabTopology": "Analyze tab: Topology"`,
	} {
		if !strings.Contains(translations, required) {
			t.Fatalf("topology translation contract is missing %q", required)
		}
	}

	shortcuts := readTestFile(t, "frontend/src/js/shortcuts.js")
	if !strings.Contains(shortcuts, `tabTopology: () => actions.switchResultTab?.("topology")`) {
		t.Fatal("tabTopology shortcut must activate the topology result-tab ID")
	}

	state := readTestFile(t, "frontend/src/js/state.js")
	if !strings.Contains(state, `document.querySelectorAll("[data-result-tab]")`) {
		t.Fatal("result-tab automation must use the stable data-result-tab contract")
	}
}

func TestResultTabsAreTheOnlyTopLevelPanelNames(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	for _, duplicate := range []string{
		`<h2 data-i18n="tab.metrics"`,
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

func TestTopologyHeaderUsesSingleDesktopToolbarRow(t *testing.T) {
	styles := readTestFile(t, "frontend/src/styles/topology.css")
	head := sliceBetween(styles, ".topology-head {", ".topology-head > #topologyStats")
	for _, required := range []string{"display: flex", "align-items: center"} {
		if !strings.Contains(head, required) {
			t.Fatalf("Topology header is missing single-row layout rule %q", required)
		}
	}
	tools := sliceBetween(styles, ".topology-tools {", ".topology-control-group")
	for _, required := range []string{"flex: 1 1 auto", "flex-wrap: nowrap"} {
		if !strings.Contains(tools, required) {
			t.Fatalf("Topology tools are missing single-row layout rule %q", required)
		}
	}
}
