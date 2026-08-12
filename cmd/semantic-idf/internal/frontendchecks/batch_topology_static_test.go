package frontendchecks

import (
	"strings"
	"testing"
)

func TestBatchMetricsUsesFixedTopologyBasisAndExports(t *testing.T) {
	markup := readTestFile(t, "frontend/src/tools.html")
	script := readTestFile(t, "frontend/src/js/tools.js")
	for _, required := range []string{
		`value="topology"`, `id="batchMetricsIncludeTopology"`, `areaBasis: "effective"`,
		`id="batchMetricsExportJSON"`, "AnalyzeBatchMetrics", "includeFullTopology",
		"ExportBatchTopologyCSV", "batch-topology-normalized.csv", "not comparable: U-value coverage differs",
	} {
		if !strings.Contains(markup+script, required) {
			t.Fatalf("batch topology UI contract is missing %q", required)
		}
	}
	if strings.Contains(script, "full graph visual compare") {
		t.Fatal("batch topology unexpectedly exposes full graph visual comparison")
	}
	for _, removed := range []string{
		`id="batchMetricsAreaBasis"`, `value="physical"`, "state.areaBasis", "elements.areaBasis",
		"basisSensitive", "not comparable: area basis differs", "topology basis:",
	} {
		if strings.Contains(markup+script, removed) {
			t.Fatalf("batch topology still exposes removed area-basis UI contract %q", removed)
		}
	}
}
