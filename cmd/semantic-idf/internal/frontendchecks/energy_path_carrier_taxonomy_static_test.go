package frontendchecks

import (
	"strings"
	"testing"
)

func TestEPATH110FrontendDeclaresCompleteCanonicalCarrierPresentation(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/energy-path-view.js")
	want := []struct {
		token string
		label string
	}{
		{token: "electricity", label: "Electricity"},
		{token: "natural_gas", label: "Natural gas"},
		{token: "district_cooling", label: "District cooling"},
		{token: "district_heating", label: "District heating"},
		{token: "steam", label: "Steam"},
		{token: "propane", label: "Propane"},
		{token: "fuel_oil_1", label: "Fuel oil #1"},
		{token: "fuel_oil_2", label: "Fuel oil #2"},
		{token: "coal", label: "Coal"},
		{token: "diesel", label: "Diesel"},
		{token: "gasoline", label: "Gasoline"},
		{token: "other_fuel_1", label: "Other fuel 1"},
		{token: "other_fuel_2", label: "Other fuel 2"},
		{token: "water", label: "Water"},
	}
	for _, item := range want {
		if !strings.Contains(view, item.token) {
			t.Errorf("EPATH-110 frontend carrier presentation is missing canonical token %q", item.token)
		}
		if !strings.Contains(view, item.label) {
			t.Errorf("EPATH-110 frontend carrier presentation is missing canonical label %q", item.label)
		}
	}

	for _, required := range []string{
		`level: "carrier"`,
		`label: "Energy Source"`,
		`scaleDomain: "site"`,
		`unitLabel: "kWh site"`,
		`inspectorSection`,
		`derived_ratio`,
	} {
		if !strings.Contains(view, required) {
			t.Errorf("EPATH-110 frontend carrier/water boundary is missing %q", required)
		}
	}

	for _, forbidden := range []string{
		`level: "fuel"`,
		`node.level === "fuel"`,
		`carrier: "fuel"`,
		`data-energy-path-stage="fuel"`,
	} {
		if strings.Contains(view, forbidden) {
			t.Errorf("electricity and district energy must not share a generic fuel stage/classification; found %q", forbidden)
		}
	}
}

func TestEPATH110FrontendSourceAliasMatcherCoversEveryCarrierAndBothOrders(t *testing.T) {
	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	for _, required := range []string{
		`electricity: "electricity"`,
		`naturalgas: "natural_gas"`,
		`gas: "natural_gas"`,
		`districtcooling: "district_cooling"`,
		`districtheatingwater: "district_heating"`,
		`districtheating: "district_heating"`,
		`districtheatingsteam: "steam"`,
		`steam: "steam"`,
		`propane: "propane"`,
		`fueloilno1: "fuel_oil_1"`,
		`fueloilno2: "fuel_oil_2"`,
		`coal: "coal"`,
		`diesel: "diesel"`,
		`gasoline: "gasoline"`,
		`otherfuel1: "other_fuel_1"`,
		`otherfuel2: "other_fuel_2"`,
		`water: "water"`,
		`if (leftCarrier && rightEndUse)`,
		`if (leftEndUse && rightCarrier)`,
	} {
		if !strings.Contains(simulation, required) {
			t.Errorf("EPATH-110 source/output alias matcher is missing %q", required)
		}
	}

	// These two spellings are the minimum explicit regression named by the
	// checklist. The private matcher may normalize them through the generic
	// two-sided branches above; neither spelling may be hard-coded as the sole
	// accepted direction.
	if strings.Count(simulation, `leftCarrier && rightEndUse`) != 1 || strings.Count(simulation, `leftEndUse && rightCarrier`) != 1 {
		t.Error("carrier/end-use alias matching must have one symmetric branch for Cooling:Electricity and Electricity:Cooling")
	}
}

func TestEPATH110FrontendTotalSiteEnergyHasExplicitWaterExclusion(t *testing.T) {
	summary := readTestFile(t, "frontend/src/js/energy-path-summary.js")
	if !strings.Contains(summary, "water") {
		t.Fatal("Total site energy has no explicit water-carrier exclusion")
	}
	for _, required := range []string{
		`id: "total_site_energy"`,
		`const carriers =`,
		`energyPathSummaryTotal`,
	} {
		if !strings.Contains(summary, required) {
			t.Fatalf("EPATH-110 Total site energy contract missing %q", required)
		}
	}
}
