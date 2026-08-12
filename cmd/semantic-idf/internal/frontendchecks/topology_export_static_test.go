package frontendchecks

import (
	"strings"
	"testing"
)

func TestThermalTopologyViewDoesNotExposeJSONExport(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	main := readTestFile(t, "frontend/src/js/main.js")
	content := markup + view + main
	for _, removed := range []string{
		`id="thermalTopologyExportJSON"`,
		"idfAnalyzer:thermalTopologyExport",
		"thermalTopologyExportPayload",
		"exportThermalTopologyJSON",
	} {
		if strings.Contains(content, removed) {
			t.Fatalf("removed Network JSON export remains %q", removed)
		}
	}
}
