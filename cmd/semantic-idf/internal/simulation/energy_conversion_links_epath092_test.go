package simulation

import (
	"encoding/json"
	"math"
	"testing"
)

func TestEPATH092OnlyMatchingCoolingAndHeatingFormMainConversions(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema: energyExplanationV1Schema,
		Nodes: []EnergyExplanationNode{
			{ID: "carrier.electricity", Level: "energy", Value: 55, Unit: "kWh site", Carrier: "electricity", EndUse: "total"},
			{ID: "carrier.gas", Level: "energy", Value: 50, Unit: "kWh site", Carrier: "natural_gas", EndUse: "total"},
			{ID: "enduse.cooling", Level: "energy", Value: 25, Unit: "kWh site", Carrier: "electricity", EndUse: "cooling", SourceIDs: []string{"cooling-meter"}},
			{ID: "enduse.heating", Level: "energy", Value: 50, Unit: "kWh site", Carrier: "natural_gas", EndUse: "heating", SourceIDs: []string{"heating-meter"}},
			{ID: "enduse.fans", Level: "energy", Value: 10, Unit: "kWh site", Carrier: "electricity", EndUse: "fans"},
			{ID: "enduse.pumps", Level: "energy", Value: 8, Unit: "kWh site", Carrier: "electricity", EndUse: "pumps"},
			{ID: "enduse.rejection", Level: "energy", Value: 7, Unit: "kWh site", Carrier: "electricity", EndUse: "heat_rejection"},
			{ID: "enduse.humidifier", Level: "energy", Value: 5, Unit: "kWh site", Carrier: "natural_gas", EndUse: "humidification"},
			{ID: "load.cooling", Level: "load", Kind: "load.zone_cooling", Value: 100, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"cooling-load"}},
			{ID: "load.heating", Level: "load", Kind: "load.zone_heating", Value: 40, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "heating", SourceIDs: []string{"heating-load"}},
			{ID: "load.dehumidification", Level: "load", Kind: "load.zone_dehumidification", Value: 12, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "dehumidification", SourceIDs: []string{"dehumidification-detail"}},
			{ID: "load.humidification", Level: "load", Kind: "load.zone_humidification", Value: 8, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "humidification", SourceIDs: []string{"humidification-detail"}},
		},
		Edges: []EnergyExplanationEdge{
			{FromID: "carrier.electricity", ToID: "enduse.cooling", Value: 25, Unit: "kWh site", Relation: "meter_enduse"},
			{FromID: "carrier.gas", ToID: "enduse.heating", Value: 50, Unit: "kWh site", Relation: "meter_enduse"},
			{FromID: "carrier.electricity", ToID: "enduse.fans", Value: 10, Unit: "kWh site", Relation: "meter_enduse"},
			{FromID: "carrier.electricity", ToID: "enduse.pumps", Value: 8, Unit: "kWh site", Relation: "meter_enduse"},
			{FromID: "carrier.electricity", ToID: "enduse.rejection", Value: 7, Unit: "kWh site", Relation: "meter_enduse"},
			{FromID: "carrier.gas", ToID: "enduse.humidifier", Value: 5, Unit: "kWh site", Relation: "meter_enduse"},
			{FromID: "enduse.cooling", ToID: "load.cooling", Value: 100, Unit: "kWh thermal", Relation: "delivered_load", ServiceKind: "cooling"},
			{FromID: "enduse.heating", ToID: "load.heating", Value: 40, Unit: "kWh thermal", Relation: "delivered_load", ServiceKind: "heating"},
			// Malformed stored-v1 edges must never pull auxiliaries into either
			// conversion denominator.
			{FromID: "enduse.fans", ToID: "load.cooling", Value: 100, Unit: "kWh thermal", Relation: "delivered_load", ServiceKind: "cooling"},
			{FromID: "enduse.pumps", ToID: "load.heating", Value: 40, Unit: "kWh thermal", Relation: "delivered_load", ServiceKind: "heating"},
			{FromID: "enduse.rejection", ToID: "load.cooling", Value: 100, Unit: "kWh thermal", Relation: "delivered_load", ServiceKind: "cooling"},
			{FromID: "enduse.humidifier", ToID: "load.heating", Value: 40, Unit: "kWh thermal", Relation: "delivered_load", ServiceKind: "heating"},
		},
		Sources: []EnergyDataSource{
			{ID: "cooling-load", Name: "Zone Air System Sensible Cooling Energy"},
			{ID: "heating-load", Name: "Zone Air System Sensible Heating Energy"},
			{ID: "dehumidification-detail", Name: "Zone Ideal Loads Supply Air Latent Cooling Energy"},
			{ID: "humidification-detail", Name: "Zone Ideal Loads Supply Air Latent Heating Energy"},
		},
	}

	result := UpgradeEnergyExplanationV1(legacy)
	conversions := map[string]*EnergyPathLink{}
	for index := range result.Links {
		link := &result.Links[index]
		if link.Relation == "load_to_end_use" {
			conversions[link.ServiceKind] = link
		}
	}
	if len(conversions) != 2 || conversions["cooling"] == nil || conversions["heating"] == nil {
		t.Fatalf("main HVAC conversions = %#v", conversions)
	}
	if link := conversions["cooling"]; link.FromID != "load.cooling.building" || link.ToID != "end_use.cooling.building" || link.FromValue != 100 || link.ToValue != 25 {
		t.Errorf("Cooling load -> Cooling equipment energy = %#v", link)
	}
	if link := conversions["heating"]; link.FromID != "load.heating.building" || link.ToID != "end_use.heating.building" || link.FromValue != 40 || link.ToValue != 50 {
		t.Errorf("Heating load -> Heating equipment energy = %#v", link)
	}
	for _, endUse := range []string{"fans", "pumps", "heat_rejection", "humidification"} {
		endUseID := "end_use." + endUse + ".building"
		for _, link := range result.Links {
			if link.Relation == "load_to_end_use" && link.ToID == endUseID {
				t.Errorf("auxiliary %q leaked into main conversion: %#v", endUse, link)
			}
		}
		carrierID := "carrier.electricity.building"
		if endUse == "humidification" {
			carrierID = "carrier.natural_gas.building"
		}
		link := energyPathV2LinkByIDs(result.Links, endUseID, carrierID)
		if link == nil || link.Relation != "direct_end_use_to_carrier" {
			t.Errorf("auxiliary %q is not a direct carrier branch: %#v", endUse, link)
		}
	}

	coolingLoad := energyPathV2NodeByID(result.Nodes, "load.cooling.building")
	heatingLoad := energyPathV2NodeByID(result.Nodes, "load.heating.building")
	if coolingLoad == nil || !stringSliceContains(coolingLoad.SourceIDs, "dehumidification-detail") ||
		heatingLoad == nil || !stringSliceContains(heatingLoad.SourceIDs, "humidification-detail") {
		t.Fatalf("humidity details were not attached to their respective loads: cooling=%#v heating=%#v", coolingLoad, heatingLoad)
	}
	dehumidification := energyExplanationSourceByID(result.Sources, "dehumidification-detail")
	humidification := energyExplanationSourceByID(result.Sources, "humidification-detail")
	if dehumidification == nil || dehumidification.DriverComponent != "load.dehumidification" || dehumidification.DriverCategory != "load.cooling" || dehumidification.InspectorSection != energyDriverInspectorSectionBreakdown {
		t.Errorf("dehumidification detail metadata = %#v", dehumidification)
	}
	if humidification == nil || humidification.DriverComponent != "load.humidification" || humidification.DriverCategory != "load.heating" || humidification.InspectorSection != energyDriverInspectorSectionBreakdown {
		t.Errorf("humidification detail metadata = %#v", humidification)
	}
}

func TestEPATH092RatioLabelsFollowCarrierAndServiceEvidence(t *testing.T) {
	tests := []struct {
		name      string
		service   string
		carrier   string
		load      float64
		energy    float64
		wantKind  string
		wantLabel string
		wantRatio float64
	}{
		{name: "electric cooling", service: "cooling", carrier: "electricity", load: 100, energy: 25, wantKind: "coefficient_of_performance", wantLabel: "COP", wantRatio: 4},
		{name: "electric heating is not inferred COP", service: "heating", carrier: "electricity", load: 90, energy: 100, wantKind: "load_to_site_energy", wantLabel: "Load / site energy", wantRatio: 0.9},
		{name: "plausible combustion efficiency", service: "heating", carrier: "natural_gas", load: 85, energy: 100, wantKind: "efficiency", wantLabel: "Efficiency", wantRatio: 0.85},
		{name: "fuel ratio without efficiency claim", service: "heating", carrier: "natural_gas", load: 120, energy: 100, wantKind: "load_to_fuel", wantLabel: "Load / fuel", wantRatio: 1.2},
		{name: "district cooling", service: "cooling", carrier: "district_cooling", load: 100, energy: 40, wantKind: "load_to_purchased_energy", wantLabel: "Load / purchased energy", wantRatio: 2.5},
		{name: "district heating water", service: "heating", carrier: "district_heating", load: 80, energy: 100, wantKind: "load_to_purchased_energy", wantLabel: "Load / purchased energy", wantRatio: 0.8},
		{name: "district heating steam alias carrier", service: "heating", carrier: "steam", load: 75, energy: 100, wantKind: "load_to_purchased_energy", wantLabel: "Load / purchased energy", wantRatio: 0.75},
		{name: "non-electric cooling has no inferred label", service: "cooling", carrier: "natural_gas", load: 80, energy: 20},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := epath092SingleCarrierConversion(test.service, test.carrier, test.load, test.energy, "kWh thermal", "kWh site")
			link := energyPathV2LinkByIDs(result.Links, "load."+test.service+".building", "end_use."+test.service+".building")
			if link == nil {
				t.Fatalf("conversion link missing: %#v", result.Links)
			}
			if link.FromValue != test.load || link.ToValue != test.energy || link.FromUnit != "kWh thermal" || link.ToUnit != "kWh site" {
				t.Errorf("both-side values = %#v", link)
			}
			if link.RatioKind != test.wantKind || link.RatioLabel != test.wantLabel || link.Ratio != test.wantRatio {
				t.Errorf("ratio metadata = %#v, want kind=%q label=%q ratio=%g", link, test.wantKind, test.wantLabel, test.wantRatio)
			}
		})
	}
}

func TestEPATH092MixedCarrierUsesWholeSiteEnergyDenominator(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema: energyExplanationV1Schema,
		Nodes: []EnergyExplanationNode{
			{ID: "electricity", Level: "energy", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "total"},
			{ID: "gas", Level: "energy", Value: 20, Unit: "kWh", Carrier: "natural_gas", EndUse: "total"},
			{ID: "heating-electricity", Level: "energy", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "heating"},
			{ID: "heating-gas", Level: "energy", Value: 20, Unit: "kWh", Carrier: "natural_gas", EndUse: "heating"},
			{ID: "heating-load", Level: "load", Kind: "load.zone_heating", Value: 90, Unit: "kWh", ServiceKind: "heating"},
		},
		Edges: []EnergyExplanationEdge{
			{FromID: "electricity", ToID: "heating-electricity", Value: 10, Unit: "kWh", Relation: "meter_enduse"},
			{FromID: "gas", ToID: "heating-gas", Value: 20, Unit: "kWh", Relation: "meter_enduse"},
			{FromID: "heating-electricity", ToID: "heating-load", Value: 90, Unit: "kWh", Relation: "delivered_load"},
		},
	}
	result := UpgradeEnergyExplanationV1(legacy)
	link := energyPathV2LinkByIDs(result.Links, "load.heating.building", "end_use.heating.building")
	if link == nil || link.FromValue != 90 || link.ToValue != 30 || link.Ratio != 3 || link.RatioKind != "load_to_site_energy" || link.RatioLabel != "Load / site energy" {
		t.Fatalf("mixed-carrier conversion = %#v", link)
	}
}

func TestEPATH092MergedLoadLinksRecomputeRatioFromBothSides(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema: energyExplanationV1Schema,
		Nodes: []EnergyExplanationNode{
			{ID: "electricity", Level: "energy", Value: 25, Unit: "kWh site", Carrier: "electricity", EndUse: "total"},
			{ID: "cooling", Level: "energy", Value: 25, Unit: "kWh site", Carrier: "electricity", EndUse: "cooling"},
			{ID: "office", Level: "load", Kind: "load.zone_cooling", Value: 60, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "cooling"},
			{ID: "lab", Level: "load", Kind: "load.zone_cooling", Value: 40, Unit: "kWh thermal", ZoneName: "Lab", ServiceKind: "cooling"},
		},
		Edges: []EnergyExplanationEdge{
			{FromID: "electricity", ToID: "cooling", Value: 25, Unit: "kWh site", Relation: "meter_enduse"},
			{FromID: "cooling", ToID: "office", Value: 60, Unit: "kWh thermal", Relation: "delivered_load", SourceIDs: []string{"office-link"}},
			{FromID: "cooling", ToID: "lab", Value: 40, Unit: "kWh thermal", Relation: "delivered_load", SourceIDs: []string{"lab-link"}},
		},
	}
	result := UpgradeEnergyExplanationV1(legacy)
	link := energyPathV2LinkByIDs(result.Links, "load.cooling.building", "end_use.cooling.building")
	if link == nil || link.FromValue != 100 || link.ToValue != 25 || link.Ratio != 4 || link.RatioKind != "coefficient_of_performance" ||
		!stringSliceContains(link.SourceIDs, "office-link") || !stringSliceContains(link.SourceIDs, "lab-link") {
		t.Fatalf("merged conversion ratio = %#v", link)
	}
}

func TestEPATH092InvalidRatioValuesNeverExposeLabels(t *testing.T) {
	invalid := []struct {
		name string
		link EnergyPathLink
	}{
		{name: "stale label without kind", link: EnergyPathLink{Relation: "load_to_end_use", FromValue: 100, FromUnit: "kWh", ToValue: 25, ToUnit: "kWh", Ratio: 4, RatioLabel: "COP"}},
		{name: "zero denominator", link: epath092RatioLink(100, 0, "kWh", "kWh")},
		{name: "negative denominator", link: epath092RatioLink(100, -25, "kWh", "kWh")},
		{name: "negative numerator", link: epath092RatioLink(-100, 25, "kWh", "kWh")},
		{name: "nan numerator", link: epath092RatioLink(math.NaN(), 25, "kWh", "kWh")},
		{name: "infinite denominator", link: epath092RatioLink(100, math.Inf(1), "kWh", "kWh")},
		{name: "incompatible units", link: epath092RatioLink(100, 25, "kWh", "MJ")},
		{name: "matching non-energy units", link: epath092RatioLink(100, 25, "widgets", "widgets")},
		{name: "missing numerator unit", link: epath092RatioLink(100, 25, "", "kWh")},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			finalizeEnergyPathLinkRatio(&test.link)
			if test.link.Ratio != 0 || test.link.RatioKind != "" || test.link.RatioLabel != "" {
				t.Errorf("invalid ratio still advertised: %#v", test.link)
			}
		})
	}

	compatible := epath092RatioLink(100, 25, "kWh thermal", "kWh site")
	finalizeEnergyPathLinkRatio(&compatible)
	if compatible.Ratio != 4 || compatible.RatioKind != "coefficient_of_performance" || compatible.RatioLabel != "COP" {
		t.Errorf("cross-domain unit qualifiers rejected: %#v", compatible)
	}

	for _, test := range []struct {
		name   string
		load   float64
		energy float64
	}{
		{name: "negative canonical load", load: -100, energy: 25},
		{name: "negative canonical end use", load: 100, energy: -25},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := epath092SingleCarrierConversion("cooling", "electricity", test.load, test.energy, "kWh", "kWh")
			link := energyPathV2LinkByIDs(result.Links, "load.cooling.building", "end_use.cooling.building")
			if link == nil {
				t.Fatalf("traceable magnitude link missing: %#v", result.Links)
			}
			if link.Ratio != 0 || link.RatioKind != "" || link.RatioLabel != "" {
				t.Errorf("signed invalid endpoint advertised a ratio: %#v", link)
			}
		})
	}
}

func TestEPATH092UpgradeDoesNotMutateFrozenV1Input(t *testing.T) {
	legacy := epath092SingleCarrierLegacy("cooling", "electricity", 100, 25, "kWh", "kWh")
	before, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	_ = UpgradeEnergyExplanationV1(legacy)
	after, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("v1 input mutated\nbefore: %s\nafter:  %s", before, after)
	}
}

func epath092SingleCarrierConversion(service string, carrier string, load float64, energy float64, loadUnit string, energyUnit string) EnergyExplanationResult {
	return UpgradeEnergyExplanationV1(epath092SingleCarrierLegacy(service, carrier, load, energy, loadUnit, energyUnit))
}

func epath092SingleCarrierLegacy(service string, carrier string, load float64, energy float64, loadUnit string, energyUnit string) EnergyExplanationV1 {
	return EnergyExplanationV1{
		Schema: energyExplanationV1Schema,
		Nodes: []EnergyExplanationNode{
			{ID: "carrier", Level: "energy", Value: energy, Unit: energyUnit, Carrier: carrier, EndUse: "total"},
			{ID: "enduse", Level: "energy", Kind: "energy." + service, Value: energy, Unit: energyUnit, Carrier: carrier, EndUse: service},
			{ID: "load", Level: "load", Kind: "load.zone_" + service, Value: load, Unit: loadUnit, ServiceKind: service},
		},
		Edges: []EnergyExplanationEdge{
			{FromID: "carrier", ToID: "enduse", Value: energy, Unit: energyUnit, Relation: "meter_enduse"},
			{FromID: "enduse", ToID: "load", Value: load, Unit: loadUnit, Relation: "delivered_load", ServiceKind: service},
		},
	}
}

func epath092RatioLink(from float64, to float64, fromUnit string, toUnit string) EnergyPathLink {
	return EnergyPathLink{
		FromID:     "load.cooling.building",
		ToID:       "end_use.cooling.building",
		Relation:   "load_to_end_use",
		FromValue:  from,
		FromUnit:   fromUnit,
		ToValue:    to,
		ToUnit:     toUnit,
		RatioKind:  "coefficient_of_performance",
		RatioLabel: "COP",
	}
}
