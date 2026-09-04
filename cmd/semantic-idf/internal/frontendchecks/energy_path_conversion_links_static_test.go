package frontendchecks

import (
	"strings"
	"testing"
)

func TestEPATH092FrontendConversionAndAuxiliaryLaneContract(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/energy-path-view.js")
	for _, required := range []string{
		`export function energyPathConversionRatio`,
		`export function energyPathConversionFlows`,
		`export function energyPathAuxiliaryFlows`,
		`export function renderEnergyPathFlowLanes`,
		`coefficient_of_performance`,
		`load_to_fuel`,
		`load_to_site_energy`,
		`load_to_purchased_energy`,
		`data-energy-path-conversion-lane`,
		`data-energy-path-conversion-link=`,
		`data-energy-path-ratio-kind=`,
		`data-energy-path-ratio-label=`,
		`data-energy-path-ratio-value=`,
		`data-energy-path-auxiliary-lane`,
		`data-energy-path-auxiliary-link=`,
		`data-energy-explanation-node="${escapeHTML(flow.fromNode.id || "")}"`,
		`energyPathToken(link?.relation) !== "direct_end_use_to_carrier"`,
		`new Set(["fans_pumps", "hvac_auxiliaries"])`,
		`data-energy-path-humidity-detail=`,
		`"load.humidification"`,
		`"load.dehumidification"`,
		`renderEnergyPathFlowLanes(allGraphNodes, graph.links, selectedID)`,
		`new Set(energyPathAuxiliaryFlows(nodes, links).map((flow) => flow.fromNode.id))`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("EPATH-092 frontend conversion contract missing %q", required)
		}
	}

	ratioGuard := sliceBetween(view, "export function energyPathConversionRatio", "export function energyPathConversionFlows")
	for _, required := range []string{
		`Number.isFinite(fromValue)`,
		`fromValue <= 0`,
		`Number.isFinite(toValue)`,
		`toValue <= 0`,
		`Number.isFinite(ratio)`,
		`ratio <= 0`,
		`0.0005 + Number.EPSILON`,
		`Math.abs(ratio - derivedRatio) > tolerance`,
		`energyPathRatioUnitsCompatible(link.fromUnit, link.toUnit)`,
		`kind === "efficiency" && ratio > 1`,
	} {
		if !strings.Contains(ratioGuard, required) {
			t.Fatalf("EPATH-092 invalid-ratio guard missing %q", required)
		}
	}

	for _, required := range []string{
		`"j", "kj", "mj", "gj", "tj"`,
		`"wh", "kwh", "mwh", "gwh"`,
		`"btu", "kbtu", "mbtu", "mmbtu"`,
		`"therm", "therms", "tonhour", "tonhours"`,
		`supported.has(normalized) ? normalized : ""`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("EPATH-092 exact energy-unit whitelist missing %q", required)
		}
	}
}

func TestEPATH092FrontendConversionTranslationsAndStyles(t *testing.T) {
	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	for _, required := range []string{
		`"simulation.energyPathCoolingConversion": "Cooling load → Cooling equipment energy"`,
		`"simulation.energyPathHeatingConversion": "Heating load → Heating equipment energy"`,
		`"simulation.energyPathRatioCOP": "COP"`,
		`"simulation.energyPathRatioEfficiency": "Efficiency"`,
		`"simulation.energyPathRatioLoadFuel": "Load / fuel"`,
		`"simulation.energyPathRatioLoadSiteEnergy": "Load / site energy"`,
		`"simulation.energyPathRatioLoadPurchasedEnergy": "Load / purchased energy"`,
		`"simulation.energyPathAuxiliaryLane": "Auxiliary energy"`,
		`"simulation.energyPathHumidificationDetail": "Humidification detail"`,
		`"simulation.energyPathDehumidificationDetail": "Dehumidification detail"`,
		`"simulation.energyPathCoolingConversion": "냉방 부하 → 냉방 설비 에너지"`,
		`"simulation.energyPathHeatingConversion": "난방 부하 → 난방 설비 에너지"`,
		`"simulation.energyPathRatioEfficiency": "효율"`,
		`"simulation.energyPathAuxiliaryLane": "보조 설비 에너지"`,
		`"simulation.energyPathHumidificationDetail": "가습 상세"`,
		`"simulation.energyPathDehumidificationDetail": "제습 상세"`,
	} {
		if !strings.Contains(i18n, required) {
			t.Fatalf("EPATH-092 EN/KO translation missing %q", required)
		}
	}

	styles := readTestFile(t, "frontend/src/styles/simulation.css")
	for _, required := range []string{
		`.energy-path-flow-lanes`,
		`.energy-path-conversion-lane`,
		`.energy-path-auxiliary-lane`,
		`.energy-path-flow-lane-items`,
		`.energy-path-conversion-flow`,
		`.energy-path-conversion-ratio`,
		`.energy-path-auxiliary-flow`,
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("EPATH-092 conversion/auxiliary style missing %q", required)
		}
	}
}
