package frontendchecks

import (
	"strings"
	"testing"
)

func TestTopologyViewDocumentationFixesCanonicalRolesAndTerms(t *testing.T) {
	doc := readTestFile(t, "docs/topology-view.md")
	for _, required := range []string{
		"### 3D",
		"### Plan",
		"### Thermal",
		"### Thermal boundary",
		"### Thermal interface",
		"### Thermal connection",
		"### Geometric adjacency",
		"### Air coupling",
		"### Static UA",
		"### Simulated heat flow",
		"never creates an authoritative thermal",
		"Static UA is not energy flow, a load",
	} {
		if !strings.Contains(doc, required) {
			t.Fatalf("topology view documentation is missing %q", required)
		}
	}
}

func TestThermalTopologySchemaDocumentsStableContracts(t *testing.T) {
	doc := readTestFile(t, "docs/thermal-topology-schema.md")
	for _, required := range []string{
		"semantic-idf.thermal-topology/v1",
		"semantic-idf.thermal-topology-simulation/v1",
		"sourceModelHash",
		"### Boundary",
		"### Opening",
		"### Air coupling",
		"### Connection",
		"### Matrix cell",
		"## Area and UA formulas",
		"Reciprocal interzone surfaces and openings use one canonical pair",
		"## Diagnostic codes",
		"## Simulation overlay (separate schema)",
	} {
		if !strings.Contains(doc, required) {
			t.Fatalf("thermal topology schema documentation is missing %q", required)
		}
	}
}

func TestThermalTopologyReleaseNotesExplainCompatibilityMigration(t *testing.T) {
	doc := readTestFile(t, "docs/release-notes/v0.4.3.md")
	for _, required := range []string{
		"canonical `geometry.topology` thermal-network report",
		"separately versioned signed simulation",
		"Renamed only the visible Geometry result tab to Topology",
		"`[data-result-tab=\"geometry\"]`",
		"customizations require no migration",
	} {
		if !strings.Contains(doc, required) {
			t.Fatalf("thermal topology release notes are missing %q", required)
		}
	}
}

func TestThermalTopologyAcceptanceRecordMapsFinalFlows(t *testing.T) {
	doc := readTestFile(t, "docs/thermal-topology-acceptance.md")
	for _, required := range []string{
		"TOPO-280 Exterior zone",
		"TOPO-281 Interzone pair",
		"TOPO-282 Modeling decision/QA",
		"TOPO-283 Air coupling",
		"TOPO-284 Simulation overlay",
		"TOPO-285 Settings/Batch",
		"backend calls",
		"Wails build",
	} {
		if !strings.Contains(doc, required) {
			t.Fatalf("thermal topology acceptance record is missing %q", required)
		}
	}
}
