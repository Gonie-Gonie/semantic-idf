package frontendchecks

import (
	"strings"
	"testing"
)

func TestThermalTopologySimulationOverlayContract(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	for _, required := range []string{
		`id="simulationPurposeZoneHeatFlowDetail"`,
		`value="surface"`,
		`id="thermalTopologySimulationControls"`,
		`id="thermalTopologySimulationPeriod"`,
		`value="selected_range"`,
		`id="thermalTopologySimulationFrame"`,
	} {
		if !strings.Contains(markup, required) {
			t.Fatalf("thermal simulation markup is missing %q", required)
		}
	}

	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	for _, required := range []string{
		`zoneHeatFlowDetail: elements.simulationPurposeZoneHeatFlowDetail?.value || "zone"`,
		`idfAnalyzer:openSimulationPurposePlan`,
		`idfAnalyzer:simulationResultChanged`,
	} {
		if !strings.Contains(simulation, required) {
			t.Fatalf("simulation purpose bridge is missing %q", required)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, required := range []string{
		`simulatedOption.disabled = !overlay.available`,
		`metric-simulated-heat`,
		`metric-gain`,
		`metric-loss`,
		`thermalTopologyHeatArrow`,
		`selected_range`,
		`positive enters owner`,
		`Simulation heat flow is not compared directly with static UA`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("thermal simulation renderer is missing %q", required)
		}
	}
}

func TestThermalTopologyInspectorCrossPanelContract(t *testing.T) {
	inspector := readTestFile(t, "frontend/src/js/views/thermal-topology-inspector.js")
	for _, required := range []string{
		"selectionTargetsForView",
		`No linked ${label} target for this zone`,
		`Profile summary`,
		`HVAC service`,
		`Output requests`,
		`Heat-flow ledger`,
		`data-inspector-output-source`,
		`data-inspector-purpose-plan`,
		`source.normalizedUnit`,
		`source.aggregationMethod`,
	} {
		if !strings.Contains(inspector, required) {
			t.Fatalf("thermal topology inspector is missing %q", required)
		}
	}

	profile := readTestFile(t, "frontend/src/js/views/profile-views.js")
	for _, required := range []string{
		`data-profile-open-topology`,
		`state.geometryMode = "thermal"`,
		`thermalTopologySelectedEntityId`,
	} {
		if !strings.Contains(profile, required) {
			t.Fatalf("profile topology action is missing %q", required)
		}
	}
}
