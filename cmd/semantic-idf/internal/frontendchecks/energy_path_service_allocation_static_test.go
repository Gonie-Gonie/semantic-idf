package frontendchecks

import (
	"strings"
	"testing"
)

func TestEPATH100FrontendServiceAllocationContract(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/energy-path-view.js")
	simulationView := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	for _, required := range []string{
		`const ENERGY_PATH_UNASSIGNED_BUILDING_HVAC = "unassigned_building_hvac_energy"`,
		`export function isEnergyPathUnassignedBuildingHVACItem`,
		`basisToken === "service_path_allocation"`,
		`"Allocated by HVAC service-path load share"`,
		`data-energy-path-quality-detail="${ENERGY_PATH_UNASSIGNED_BUILDING_HVAC}"`,
		`"Unassigned building HVAC energy"`,
		`!isEnergyPathUnassignedBuildingHVACItem(node)`,
		`!isEnergyPathUnassignedBuildingHVACItem(link)`,
		`zoneSafeItems(candidate.residuals)`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("EPATH-100 frontend contract missing %q", required)
		}
	}
	if strings.Contains(view, `data-simulation-energy-allocation-policy`) {
		t.Fatal("EPATH-100 must remain automatic and must not add an allocation-policy selector")
	}
	if !strings.Contains(simulationView, `allocationPolicy: "by_service_path_load_share"`) ||
		!strings.Contains(simulationView, `allocationPolicy: simulationPurposeDefaults.allocationPolicy`) {
		t.Fatal("EPATH-100 Basic Energy request must automatically use service-path load-share allocation")
	}
	if strings.Contains(simulationView, `data-simulation-energy-allocation-policy`) {
		t.Fatal("EPATH-100 main simulation UI must not expose an allocation-policy selector")
	}
}

func TestEPATH100FrontendServiceAllocationLocales(t *testing.T) {
	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	for _, required := range []string{
		`"simulation.energyPathServicePathAllocation": "Allocated by HVAC service-path load share"`,
		`"simulation.energyPathUnassignedBuildingHVACEnergy": "Unassigned building HVAC energy"`,
		`"simulation.energyPathServicePathAllocation": "HVAC 서비스 경로 부하 비율로 배분"`,
		`"simulation.energyPathUnassignedBuildingHVACEnergy": "미배정 건물 HVAC 에너지"`,
	} {
		if !strings.Contains(i18n, required) {
			t.Fatalf("EPATH-100 localized presentation missing %q", required)
		}
	}
}
