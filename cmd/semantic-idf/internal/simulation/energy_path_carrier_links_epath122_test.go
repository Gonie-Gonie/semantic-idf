package simulation

import "testing"

func TestEPATH122MissingOtherCarrierContributorsAreNotDroppedOrCountedTwice(t *testing.T) {
	for _, edgeCount := range []int{0, 1, 2} {
		name := "all missing edges"
		if edgeCount == 1 {
			name = "explicit plus missing contributor"
		} else if edgeCount == 2 {
			name = "all explicit contributors counted once"
		}
		t.Run(name, func(t *testing.T) {
			legacy := epath122UnknownEndUsesFixture(edgeCount > 0)
			if edgeCount == 2 {
				legacy.Edges = append(legacy.Edges, EnergyExplanationEdge{ID: "known-b", FromID: "energy.carrier.electricity", ToID: "energy.unknown_b.electricity", Relation: "meter_enduse", Basis: "measured_meter", Value: 3, Unit: "kWh", SourceIDs: []string{"meter.unknown_b"}})
			}
			result := UpgradeEnergyExplanationV1(legacy)
			for _, phase := range []string{"generated", "first reload", "second reload"} {
				if phase != "generated" {
					result = epath120Reload(t, result)
				}
				node := energyPathV2NodeByID(result.Nodes, "end_use.other.building")
				if node == nil || node.Value != 5 || node.ScaleDomain != "site" {
					t.Fatalf("%s distinct unknown contributions 2+3 must form Other5: %#v", phase, node)
				}
				links := epath090ReviewLinksFrom(result.Links, node.ID, "direct_end_use_to_carrier")
				if len(links) != 1 || links[0].FromValue != 5 || links[0].ToValue != 5 || links[0].ToID != "carrier.electricity.building" || links[0].FromUnit != "kWh" || links[0].ToUnit != "kWh" {
					t.Fatalf("%s Other carrier-qualified sum must be exactly5 without duplicate contribution: %#v", phase, links)
				}
				epath090ReviewAssertSourceSet(t, links[0].SourceIDs, []string{"meter.unknown_a", "meter.unknown_b"})
				epath122AssertEndUseClosure(t, result)
			}
		})
	}
}

func TestEPATH122ZeroSurvivingCarrierEnergyDoesNotResurrectSourceContext(t *testing.T) {
	result := epath121GeneratedConversion(t, "cooling", "electricity", 100, 25)
	branch := energyPathV2LinkByIDs(result.Links, "end_use.cooling.building", "carrier.electricity.building")
	branch.FromValue, branch.ToValue = 25, 0
	for pass := 1; pass <= 2; pass++ {
		result = epath120Reload(t, result)
		if node := energyPathV2NodeByID(result.Nodes, "end_use.cooling.building"); node != nil && (node.Value != 0 || node.AllocatedValue != 0 || node.DisplayValue != 0) {
			t.Errorf("reload %d zero reported branch resurrected raw/effective source energy: %#v", pass, node)
		}
		for _, link := range result.Links {
			if link.FromID == "end_use.cooling.building" && link.Relation == "end_use_to_carrier" && (link.FromValue != 0 || link.ToValue != 0) {
				t.Errorf("reload %d zero reported carrier side was replaced by nonzero source context: %#v", pass, link)
			}
		}
		if epath092AuditSourceByID(result.Sources, "meter.cooling.electricity") == nil {
			t.Errorf("reload %d removed raw meter provenance alongside zero graph flow", pass)
		}
	}
}

func TestEPATH122StoredRemovedCarrierRebuildsSurvivingEndUseTotal(t *testing.T) {
	result := UpgradeEnergyExplanationV1(epath090ReviewFixture(false))
	gas := energyPathV2NodeByID(result.Nodes, "carrier.natural_gas.building")
	if gas == nil {
		t.Fatal("fixture must have natural gas")
	}
	gas.Carrier = "unrecognized_liquid"
	for pass := 1; pass <= 2; pass++ {
		result = epath120Reload(t, result)
		heating := energyPathV2NodeByID(result.Nodes, "end_use.heating.building")
		if heating == nil || heating.Value != 70 || heating.DisplayValue != 70 || heating.AllocatedValue != 70 || heating.RawValue != 120 || heating.EffectiveValue != 120 {
			t.Errorf("reload %d invalid carrier removal must reduce Heating120 to surviving electricity70: %#v", pass, heating)
		}
		if heating != nil && (!stringSliceContains(heating.Badges, "filtered_carrier_splits") || heating.AllocationExplanation == "") {
			t.Errorf("reload %d filtered end-use total lacks explicit partial-data note/badge: %#v", pass, heating)
		}
		if energyPathV2NodeByID(result.Nodes, "carrier.natural_gas.building") != nil {
			t.Errorf("reload %d retained unrecognized carrier", pass)
		}
		link := energyPathV2LinkByIDs(result.Links, "end_use.heating.building", "carrier.electricity.building")
		if link == nil || link.FromValue != 70 || link.ToValue != 70 {
			t.Errorf("reload %d surviving electricity branch changed: %#v", pass, link)
		} else {
			epath090ReviewAssertSourceSet(t, link.SourceIDs, []string{"meter.heating.electricity"})
		}
		for _, link := range result.Links {
			if link.Relation == "load_to_end_use" && link.ToID == "end_use.heating.building" {
				t.Errorf("reload %d retained a mixed-carrier conversion without evidence to allocate thermal load to surviving electricity: %#v", pass, link)
			}
		}
		if got := len(buildEnergyExplanationSummaryV2(result).Ratios); got != 0 {
			t.Errorf("reload %d advertised %d conversion ratios after required carrier evidence was removed", pass, got)
		}
		if epath092AuditSourceByID(result.Sources, "meter.heating.natural_gas") == nil {
			t.Errorf("reload %d filtered graph discarded original gas-meter source trace", pass)
		}
		epath122AssertEndUseClosure(t, result)
	}
}

func TestEPATH122StoredCarrierSplitUsesReportedCarrierQualifiedEnergyOnBothSides(t *testing.T) {
	result := epath121GeneratedConversion(t, "cooling", "electricity", 100, 25)
	branch := energyPathV2LinkByIDs(result.Links, "end_use.cooling.building", "carrier.electricity.building")
	branch.FromValue = 999
	branch.ToValue = 25
	for index := range result.Sources {
		if result.Sources[index].ID == "meter.cooling.electricity" {
			// Source context can describe a broader reporting period. The
			// period-local carrier side, not this annual value, is authoritative.
			result.Sources[index].RawValue = 250
			result.Sources[index].EffectiveValue = 250
		}
	}
	for pass := 1; pass <= 2; pass++ {
		result = epath120Reload(t, result)
		branch = energyPathV2LinkByIDs(result.Links, "end_use.cooling.building", "carrier.electricity.building")
		if branch == nil || branch.FromValue != 25 || branch.ToValue != 25 || branch.FromUnit != "kWh" || branch.ToUnit != "kWh" {
			t.Errorf("reload %d split must use reported electricity25 at both site endpoints: %#v", pass, branch)
		} else {
			epath090ReviewAssertSourceSet(t, branch.SourceIDs, []string{"meter.cooling.electricity"})
		}
		epath122AssertEndUseClosure(t, result)
	}
}

func TestEPATH122HVACAndDirectCarrierSplitsShareExactClosureAndLocalTrace(t *testing.T) {
	result := UpgradeEnergyExplanationV1(epath090ReviewFixture(false))
	result.Nodes = append(result.Nodes, EnergyExplanationNode{ID: "end_use.lighting.building", Level: "end_use", Kind: "end_use.lighting", EndUse: "lighting", Value: 12, RawValue: 12, EffectiveValue: 12, AllocatedValue: 12, Unit: "kWh", ScaleDomain: "site", SourceIDs: []string{"meter.lighting"}})
	result.Links = append(result.Links, EnergyPathLink{ID: "lighting-electricity", FromID: "end_use.lighting.building", ToID: "carrier.electricity.building", Relation: "direct_end_use_to_carrier", Basis: "reported_meter", FromValue: 12, FromUnit: "kWh", ToValue: 12, ToUnit: "kWh", SourceIDs: []string{"meter.lighting"}})
	result.Sources = append(result.Sources, EnergyDataSource{ID: "meter.lighting", SourceType: "sql_meter", Name: "InteriorLights:Electricity", RawValue: 12, EffectiveValue: 12, NormalizedUnit: "kWh"})
	for pass := 0; pass < 3; pass++ {
		if pass > 0 {
			result = epath120Reload(t, result)
		}
		for _, want := range []struct {
			from, to, source, relation string
			value                      float64
		}{
			{"end_use.heating.building", "carrier.electricity.building", "meter.heating.electricity", "end_use_to_carrier", 70},
			{"end_use.heating.building", "carrier.natural_gas.building", "meter.heating.natural_gas", "end_use_to_carrier", 50},
			{"end_use.lighting.building", "carrier.electricity.building", "meter.lighting", "direct_end_use_to_carrier", 12},
		} {
			link := energyPathV2LinkByIDs(result.Links, want.from, want.to)
			if link == nil || link.Relation != want.relation || link.FromValue != want.value || link.ToValue != want.value || link.FromUnit != "kWh" || link.ToUnit != "kWh" {
				t.Errorf("pass%d carrier split must retain exact measured energy: %#v", pass, link)
			} else {
				epath090ReviewAssertSourceSet(t, link.SourceIDs, []string{want.source})
			}
		}
		epath122AssertEndUseClosure(t, result)
	}
}

func epath122UnknownEndUsesFixture(explicit bool) EnergyExplanationV1 {
	result := EnergyExplanationV1{Schema: energyExplanationV1Schema, Purpose: string(SimulationPurposeBasicEnergy), Nodes: []EnergyExplanationNode{
		{ID: "energy.carrier.electricity", Level: "energy", Kind: "energy.electricity.total", Carrier: "electricity", EndUse: "total", Value: 5, Unit: "kWh", MeterHierarchyLevel: "facility_total", Basis: "measured_meter", SourceIDs: []string{"facility"}},
		{ID: "energy.unknown_a.electricity", Level: "energy", Kind: "energy.unknown_a", Carrier: "electricity", EndUse: "unknown_a", Value: 2, Unit: "kWh", MeterHierarchyLevel: "broad_end_use", Basis: "measured_meter", SourceIDs: []string{"meter.unknown_a"}},
		{ID: "energy.unknown_b.electricity", Level: "energy", Kind: "energy.unknown_b", Carrier: "electricity", EndUse: "unknown_b", Value: 3, Unit: "kWh", MeterHierarchyLevel: "broad_end_use", Basis: "measured_meter", SourceIDs: []string{"meter.unknown_b"}},
	}, Sources: []EnergyDataSource{
		{ID: "facility", SourceType: "sql_meter", Name: "Electricity:Facility", RawValue: 5, EffectiveValue: 5, NormalizedUnit: "kWh"},
		{ID: "meter.unknown_a", SourceType: "sql_meter", Name: "UnknownA:Electricity", RawValue: 2, EffectiveValue: 2, NormalizedUnit: "kWh"},
		{ID: "meter.unknown_b", SourceType: "sql_meter", Name: "UnknownB:Electricity", RawValue: 3, EffectiveValue: 3, NormalizedUnit: "kWh"},
	}}
	if explicit {
		result.Edges = []EnergyExplanationEdge{{ID: "known-a", FromID: "energy.carrier.electricity", ToID: "energy.unknown_a.electricity", Relation: "meter_enduse", Basis: "measured_meter", Value: 2, Unit: "kWh", SourceIDs: []string{"meter.unknown_a"}}}
	}
	return result
}

func epath122AssertEndUseClosure(t *testing.T, result EnergyExplanationResult) {
	t.Helper()
	for _, node := range result.Nodes {
		if node.Level != "end_use" {
			continue
		}
		total := 0.0
		for _, link := range result.Links {
			if link.FromID != node.ID || (link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier") {
				continue
			}
			if link.FromValue != link.ToValue {
				t.Errorf("unequal site split endpoints: %#v", link)
			}
			if carrier := energyPathV2NodeByID(result.Nodes, link.ToID); carrier == nil || carrier.Level != "carrier" || carrier.ScaleDomain != "site" || node.ScaleDomain != "site" {
				t.Errorf("split must stay inside site-energy domain: %#v", link)
			}
			total += link.ToValue
		}
		if total != node.Value {
			t.Errorf("%s end-use total=%g, outgoing carrier sum=%g", node.ID, node.Value, total)
		}
	}
}
