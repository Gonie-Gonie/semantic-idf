package frontendchecks

import (
	"strings"
	"testing"
)

func TestThermalTopologySimulationOverlayContract(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	for _, required := range []string{`id="simulationPurposeZoneHeatFlowDetail"`, `value="surface"`} {
		if !strings.Contains(markup, required) {
			t.Fatalf("thermal simulation markup is missing %q", required)
		}
	}

	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	for _, required := range []string{
		`zoneHeatFlowDetail: elements.simulationPurposeZoneHeatFlowDetail?.value || "zone"`,
		`idfAnalyzer:openSimulationPurposePlan`,
	} {
		if !strings.Contains(simulation, required) {
			t.Fatalf("simulation purpose bridge is missing %q", required)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, removed := range []string{"simulated_heat", "metric-simulated-heat", "thermalTopologyHeatArrow", "thermalTopologySimulationPeriod"} {
		if strings.Contains(markup, removed) || strings.Contains(view, removed) {
			t.Fatalf("removed topology simulation overlay remains: %q", removed)
		}
	}
}

func TestThermalTopologyInspectorCrossPanelContract(t *testing.T) {
	inspector := readTestFile(t, "frontend/src/js/views/thermal-topology-inspector.js")
	for _, required := range []string{
		`renderZoneProfileSummary(node)`,
		`renderZoneHVACSummary(node)`,
		`Profile summary`,
		`HVAC service`,
	} {
		if !strings.Contains(inspector, required) {
			t.Fatalf("thermal topology inspector is missing %q", required)
		}
	}
	for _, removed := range []string{
		`selectionTargetsForView`,
		`Output requests`,
		`renderZoneOutputSummary`,
		`renderDiagnostics`,
		`renderInspectorActions`,
		`data-inspector-`,
		`Diagnostics`,
		`inspectorSection("Actions"`,
		`thermal-inspector-actions`,
	} {
		if strings.Contains(inspector, removed) {
			t.Fatalf("removed topology inspector section or action remains %q", removed)
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
