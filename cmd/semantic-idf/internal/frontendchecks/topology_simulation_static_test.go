package frontendchecks

import (
	"strings"
	"testing"
)

func TestThermalTopologySimulationOverlayContract(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	if strings.Contains(markup, `id="simulationPurposeZoneHeatFlowDetail"`) {
		t.Fatal("thermal simulation markup still exposes the removed Zone Heat Flow detail selector")
	}

	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	if !strings.Contains(simulation, `zoneHeatFlowDetail: "surface"`) {
		t.Fatal("simulation purpose request must keep Surface heat-flow detail as its fixed default")
	}

	view := readTestFile(t, "frontend/src/js/views/thermal-topology-view.js")
	for _, removed := range []string{"simulated_heat", "metric-simulated-heat", "thermalTopologyHeatArrow", "thermalTopologySimulationPeriod"} {
		if strings.Contains(markup, removed) || strings.Contains(view, removed) {
			t.Fatalf("removed topology simulation overlay remains: %q", removed)
		}
	}
}

func TestThermalTopologyDetailsPanelIsolationContract(t *testing.T) {
	details := readTestFile(t, "frontend/src/js/views/thermal-topology-details.js")
	for _, required := range []string{
		`renderZoneHVACSummary(node)`,
		`HVAC service`,
	} {
		if !strings.Contains(details, required) {
			t.Fatalf("thermal topology details are missing %q", required)
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
		`detailSection("Actions"`,
		`thermal-detail-actions`,
		`renderZoneProfileSummary`,
		`Profile summary`,
		`state.report?.profile`,
		`No linked occupancy`,
	} {
		if strings.Contains(details, removed) {
			t.Fatalf("removed topology details section, action, or Profile coupling remains %q", removed)
		}
	}

	profile := readTestFile(t, "frontend/src/js/views/profile-views.js")
	for _, removed := range []string{
		`Open Topology`,
		`data-profile-open-topology`,
		`dataset.profileOpenTopology`,
		`state.topologyMode = "thermal"`,
		`thermalTopologySelectedEntityId`,
		`state.report?.geometry?.topology`,
	} {
		if strings.Contains(profile, removed) {
			t.Fatalf("Profile still exposes a direct Topology coupling %q", removed)
		}
	}
}
