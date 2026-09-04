package frontendchecks

import (
	"strings"
	"testing"
)

func TestEPATH101FrontendAuxiliaryAllocationQualityContract(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/energy-path-view.js")
	styles := readTestFile(t, "frontend/src/styles/simulation.css")
	for _, required := range []string{
		`export function energyPathAuxiliaryAllocationQuality`,
		`export function renderEnergyPathAuxiliaryAllocationQuality`,
		`reconcile.zone_auxiliary_allocation.`,
		`"directValue", "allocatedValue", "unassignedValue"`,
		`data-energy-path-auxiliary-allocation-quality`,
		`data-energy-path-auxiliary-allocation-ratio="${kind}"`,
		`"Building allocation used by this Zone view"`,
		`Unassigned energy remains quality context and is never added to the selected Zone.`,
		`"direct_end_use_to_carrier", "end_use_to_carrier"`,
		`"Allocated by related AirLoop supply-air volume share"`,
		`const ENERGY_PATH_UNASSIGNED_BUILDING_HVAC_AUXILIARY = "unassigned_building_hvac_auxiliary_energy"`,
		`export function isEnergyPathUnassignedBuildingHVACAuxiliaryItem`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("EPATH-101 frontend contract missing %q", required)
		}
	}
	for _, required := range []string{
		`.energy-path-auxiliary-allocation-quality`,
		`.energy-path-auxiliary-allocation-ratios`,
		`data-energy-path-auxiliary-allocation-ratio="direct"`,
		`data-energy-path-auxiliary-allocation-ratio="allocated"`,
		`data-energy-path-auxiliary-allocation-ratio="unassigned"`,
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("EPATH-101 allocation quality styling missing %q", required)
		}
	}
}

func TestEPATH101FrontendRemainsAutomaticWithoutOutputPlanExpansion(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/energy-path-view.js")
	controls := sliceBetween(view, "export function renderEnergyPathControls", "function renderEnergyPathStage")
	if count := strings.Count(controls, "<label>"); count != 3 {
		t.Fatalf("Energy Path main toolbar must remain Scope, Period, Service only; got %d labels", count)
	}
	for _, forbidden := range []string{
		`data-simulation-energy-allocation-policy`,
		`data-simulation-energy-auxiliary-allocation`,
		`data-simulation-energy-airflow-allocation`,
	} {
		if strings.Contains(controls, forbidden) {
			t.Fatalf("EPATH-101 must not add a main-toolbar selector %q", forbidden)
		}
	}

	purpose := readTestFile(t, "internal/simulation/purpose.go")
	for _, existingMeter := range []string{
		`"standard-meter-electricity-fans"`,
		`"standard-meter-electricity-pumps"`,
		`"standard-meter-electricity-heat-rejection"`,
	} {
		if !strings.Contains(purpose, existingMeter) {
			t.Fatalf("EPATH-101 should reuse the existing output plan; baseline meter %q is missing", existingMeter)
		}
	}
	energyPathPlan := sliceBetween(purpose, "func (builder *purposePlanBuilder) addBasicEnergyPath()", "type purposeOutputKeyTarget")
	for _, forbiddenOutput := range []string{
		`"Fan Electricity Energy"`,
		`"Pump Electricity Energy"`,
		`"Cooling Tower Fan Electricity Energy"`,
		`"System Node Standard Density Volume Flow Rate"`,
		`"System Node Current Density Volume Flow Rate"`,
		`"Air System Supply Air Volume Flow Rate"`,
	} {
		if strings.Contains(energyPathPlan, forbiddenOutput) {
			t.Fatalf("EPATH-101 unexpectedly expands the purpose output plan with %q", forbiddenOutput)
		}
	}
}

func TestEPATH101FrontendLocalesAndDocumentation(t *testing.T) {
	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	for _, required := range []string{
		`"simulation.energyPathAuxiliaryAllocationCoverage": "HVAC auxiliary allocation coverage"`,
		`"simulation.energyPathAllocationDirect": "Direct"`,
		`"simulation.energyPathAllocationAllocated": "Allocated"`,
		`"simulation.energyPathAllocationUnassigned": "Unassigned"`,
		`"simulation.energyPathUnassignedBuildingHVACAuxiliaryEnergy": "Unassigned building HVAC auxiliary energy"`,
		`"simulation.energyPathAuxiliaryAllocationCoverage": "HVAC 보조 에너지 배분 비율"`,
		`"simulation.energyPathAllocationDirect": "직접"`,
		`"simulation.energyPathAllocationAllocated": "배분"`,
		`"simulation.energyPathAllocationUnassigned": "미배정"`,
	} {
		if !strings.Contains(i18n, required) {
			t.Fatalf("EPATH-101 localized quality contract missing %q", required)
		}
	}

	docs := readTestFile(t, "../../docs/simulation-runner.md")
	for _, required := range []string{
		"supply-air volume series only when one is",
		"does not add new EnergyPlus output requests",
		"direct, allocated, and unassigned auxiliary-energy shares",
		"never inserted into the selected Zone graph or value",
	} {
		if !strings.Contains(docs, required) {
			t.Fatalf("EPATH-101 operator documentation missing %q", required)
		}
	}
}
