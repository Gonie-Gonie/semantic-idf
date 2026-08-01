package frontendchecks

import (
	"strings"
	"testing"
)

func TestThermalTopologyTargetResolverCoversSemanticTargetKinds(t *testing.T) {
	resolver := readTestFile(t, "frontend/src/js/thermal-topology-targets.js")
	for _, required := range []string{
		`"thermal_boundary"`,
		`"thermal_interface"`,
		`"thermal_connection"`,
		`"thermal_environment"`,
		`"thermal_air_coupling"`,
		`"thermal_issue"`,
		"resolveThermalTopologyTarget",
		"surfaceIds",
		"windowIds",
		"nodeIds",
		"boundaryIds",
		"airCouplingIds",
	} {
		if !strings.Contains(resolver, required) {
			t.Fatalf("thermal topology target resolver is missing %q", required)
		}
	}

	loader := readTestFile(t, "frontend/src/js/geometry-loader.js")
	for _, required := range []string{"isThermalTopologyTargetKind", "thermalTopologyTargetExists"} {
		if !strings.Contains(loader, required) {
			t.Fatalf("lazy geometry adapter is missing topology target support %q", required)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	for _, required := range []string{
		"resolveThermalTopologyTarget",
		`state.geometryMode = "thermal"`,
		"surfaceIds",
		"windowIds",
		"nodeIds",
		"thermalOccurrenceContextPriority",
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("geometry view is missing topology reveal behavior %q", required)
		}
	}
}

func TestThermalSemanticOccurrencePriorityIsContextAware(t *testing.T) {
	for _, path := range []string{
		"frontend/src/js/geometry-loader.js",
		"frontend/src/js/panel-navigation-adapters.js",
		"frontend/src/js/selection-controller.js",
		"frontend/src/js/views/geometry-view.js",
	} {
		content := readTestFile(t, path)
		for _, required := range []string{
			`context === "thermal_connection_context"`,
			`context === "surface_boundary_context"`,
			`context === "zone_geometry"`,
			`context === "definition"`,
		} {
			if !strings.Contains(content, required) {
				t.Fatalf("%s is missing semantic occurrence priority %q", path, required)
			}
		}
	}
}

func TestFrontendReadinessIncludesThermalTopologyResolver(t *testing.T) {
	build := readTestFile(t, "scripts/frontend-build.ps1")
	if !strings.Contains(build, `"thermal-topology-targets.js"`) {
		t.Fatal("frontend readiness manifest is missing thermal-topology-targets.js")
	}
}
