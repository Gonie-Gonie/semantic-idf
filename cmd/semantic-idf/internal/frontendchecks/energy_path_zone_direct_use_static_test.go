package frontendchecks

import (
	"strings"
	"testing"
)

func TestEPATH094FrontendZoneDirectUseAndPartialCoverageContract(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/energy-path-view.js")
	for _, required := range []string{
		`export function energyPathZoneDirectCoverage`,
		`"zone_direct_energy_partial_coverage"`,
		`energyPathToken(item?.status) === "partial"`,
		`energyPathToken(item?.basis) === "direct_zone_energy"`,
		`export function energyPathZoneDirectUseNodeIsTrusted`,
		`!["lighting", "equipment"].includes(energyPathCanonicalEndUse(node))`,
		`basis === "direct_zone_energy"`,
		`hierarchy === "zone_direct_use"`,
		`sourceType.includes("variable")`,
		`hasOnlyUnscopedMeters`,
		`data-energy-path-zone-coverage-notice="partial"`,
		`data-energy-path-value-scope="observed_direct_use_subtotal"`,
		`const knownZoneOnly = options.knownZoneOnly ?? energyPathZoneDirectCoverage(summary).limited`,
		`renderEnergyPathKPIBoundaries`,
		`data-energy-path-kpi-boundary-status=`,
		`"Known zone site energy"`,
		`"Known energy sources"`,
		`"Observed direct-use subtotal"`,
		`"Direct zone energy"`,
		`data-energy-path-inspector-value="${key}"`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("EPATH-094 frontend contract missing %q", required)
		}
	}
	if strings.Contains(view, `data-simulation-energy-allocation-policy`) {
		t.Fatal("EPATH-094 must not add the later EPATH-100 allocation-policy selector")
	}
}

func TestEPATH094FrontendZoneCoverageLocalesAndStyles(t *testing.T) {
	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	for _, required := range []string{
		`"simulation.energyPathKnownZoneSiteEnergy": "Known zone site energy"`,
		`"simulation.energyPathPartialDirectUseCoverage": "Partial direct-use coverage"`,
		`"simulation.energyPathObservedDirectUseSubtotal": "Observed direct-use subtotal"`,
		`"simulation.energyPathDirectZoneEnergy": "Direct zone energy"`,
		`"simulation.energyPathKnownZoneSiteEnergy": "확인된 Zone site energy"`,
		`"simulation.energyPathPartialDirectUseCoverage": "직접 용도 에너지 일부만 확인됨"`,
		`"simulation.energyPathDirectZoneEnergy": "Zone 직접 에너지"`,
	} {
		if !strings.Contains(i18n, required) {
			t.Fatalf("EPATH-094 localized partial/direct label missing %q", required)
		}
	}

	styles := readTestFile(t, "frontend/src/styles/simulation.css")
	for _, required := range []string{
		`.energy-path-zone-coverage-notice`,
		`.energy-path-partial-coverage-badge`,
		`.energy-path-summary-coverage-note`,
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("EPATH-094 partial-coverage style missing %q", required)
		}
	}
}
