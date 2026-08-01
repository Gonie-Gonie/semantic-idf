package frontendchecks

import (
	"strings"
	"testing"
)

func TestBatchSummaryExposesTopologyMetricsBasisAndExports(t *testing.T) {
	markup := readTestFile(t, "frontend/src/batch.html")
	script := readTestFile(t, "frontend/src/js/batch.js")
	for _, required := range []string{
		`value="topology"`, `id="batchSummaryAreaBasis"`, `id="batchSummaryIncludeTopology"`,
		`id="multiSummaryExportJSON"`, "AnalyzeMultiIDFSummaryWithOptions", "includeFullTopology",
		"ExportBatchTopologyCSV", "batch-topology-normalized.csv", "not comparable: U-value coverage differs",
	} {
		if !strings.Contains(markup+script, required) {
			t.Fatalf("batch topology UI contract is missing %q", required)
		}
	}
	if strings.Contains(script, "full graph visual compare") {
		t.Fatal("batch topology unexpectedly exposes full graph visual comparison")
	}
}
