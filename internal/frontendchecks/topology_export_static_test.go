package frontendchecks

import (
	"strings"
	"testing"
)

func TestThermalTopologyExportUsesCanonicalReportWithoutViewState(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	main := readTestFile(t, "frontend/src/js/main.js")
	for _, required := range []string{`id="thermalTopologyExportJSON"`, "idfAnalyzer:thermalTopologyExport", "thermalTopologyExportPayload", "...topology", "areaBasis"} {
		if !strings.Contains(markup+view+main, required) {
			t.Fatalf("thermal topology export contract is missing %q", required)
		}
	}
	functionStart := strings.Index(view, "export function thermalTopologyExportPayload")
	functionEnd := strings.Index(view[functionStart:], "export function exportThermalTopologyJSON")
	if functionStart < 0 || functionEnd < 0 {
		t.Fatal("thermalTopologyExportPayload function not found")
	}
	payloadFunction := view[functionStart : functionStart+functionEnd]
	for _, forbidden := range []string{"thermalTopologyPanX", "thermalTopologyPanY", "thermalTopologyScale", "thermalTopologyLayoutCache"} {
		if strings.Contains(payloadFunction, forbidden) {
			t.Fatalf("topology export payload contains UI-only state %q", forbidden)
		}
	}
}
