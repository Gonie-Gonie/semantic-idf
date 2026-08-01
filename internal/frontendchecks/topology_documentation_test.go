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
