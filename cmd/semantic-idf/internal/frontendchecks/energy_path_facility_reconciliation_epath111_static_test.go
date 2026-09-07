package frontendchecks

import (
	"strings"
	"testing"
)

func TestEPATH111FrontendDeclaresFacilityReconciliationAndSupplyContracts(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/energy-path-view.js")
	for _, required := range []string{
		"energyPathSupplyActivities",
		"energyPathCarrierReconciliation",
		"renderEnergyPathSupportStrip",
		`data-energy-path-support-strip`,
		`data-energy-path-support-kind`,
		`data-energy-path-supply-breakdown`,
		`data-energy-path-carrier-reconciliation`,
		`data-energy-path-carrier-reconciliation-term`,
		`data-energy-path-carrier-residual-badge`,
		`data-energy-path-quality-basis="carrier_residual"`,
		`data-energy-path-unclassified-energy`,
		`"electricity_purchased"`,
		`"electricity_sold"`,
		`"generators"`,
		`"storage_discharge"`,
		`"support_supply"`,
	} {
		if !strings.Contains(view, required) {
			t.Errorf("EPATH-111 facility reconciliation UI contract is missing %q", required)
		}
	}

	// Supply activity is a support strip/inspector concern, not a fifth main
	// Sankey stage and not another carrier identity.
	if strings.Contains(view, `level: "support",`+"\n"+`    labelKey:`) {
		t.Error("support activity was added to ENERGY_PATH_STAGES instead of a conditional support strip")
	}
	if strings.Contains(view, `carrier: "electricity_purchased"`) || strings.Contains(view, `carrier: "electricity_sold"`) {
		t.Error("purchased/sold electricity must not become extra carrier nodes")
	}
}

func TestEPATH111FrontendHasUserFacingSupplyAndResidualCopy(t *testing.T) {
	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	for _, required := range []string{
		"Supply breakdown",
		"Purchased electricity",
		"Onsite production",
		"Sold electricity",
		"Storage discharge",
		"Unclassified energy",
		"Facility carrier total",
		"Mapped end uses",
		"Residual",
	} {
		if !strings.Contains(i18n, required) {
			t.Errorf("EPATH-111 user-facing copy is missing %q", required)
		}
	}

	styles := readTestFile(t, "frontend/src/styles/simulation.css")
	for _, required := range []string{
		".energy-path-support-strip",
		".energy-path-supply-breakdown",
		".energy-path-carrier-residual-badge",
		".energy-path-carrier-reconciliation",
		".energy-path-unclassified-energy",
	} {
		if !strings.Contains(styles, required) {
			t.Errorf("EPATH-111 UI has no style contract for %q", required)
		}
	}
}
