package frontendchecks

import (
	"strings"
	"testing"
)

func TestEPATH091FrontendDeclaresBoundedEndUseTaxonomy(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/energy-path-view.js")
	for _, required := range []string{
		`"cooling"`,
		`"heating"`,
		`"fans"`,
		`"pumps"`,
		`"heat_rejection"`,
		`"humidification"`,
		`"heat_recovery"`,
		`"lighting"`,
		`"equipment"`,
		`"water_systems"`,
		`"refrigeration"`,
		`"other"`,
		`"Cooling equipment"`,
		`"Heating equipment"`,
		`"Fans & pumps"`,
		`"HVAC auxiliaries"`,
		`"Lighting"`,
		`"Equipment"`,
		`"Water systems"`,
		`"Refrigeration"`,
		`"Other"`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("EPATH-091 fixed end-use UI taxonomy missing %q", required)
		}
	}

	for _, forbidden := range []string{
		`data-simulation-energy-end-use-node-limit`,
		`data-simulation-energy-end-use-taxonomy-edit`,
	} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("EPATH-091 must remain a bounded, non-user-expandable primary taxonomy; found %q", forbidden)
		}
	}
}
