package simulation

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Projection-only evidence boundary: the physical owner/SQL reader is covered
// by the separate native WindowAC tests. A local native cooling observation and
// an allocated central cooling share must not claim a fully measured subtotal.
func TestEnergyPathWindowACMixedConsumptionKeepsNativeAndAllocatedEvidence(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, unresolved := range []bool{false, true} {
			scope := EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"}
			sources := []EnergyDataSource{windowACPathSource("native", "Local Coil", "Cooling Coil Electricity Energy")}
			wantPath := []string{"window-ac"}
			if unresolved {
				wantPath = nil
			}
			paths := map[string][]string{energyPathNativeWindowACPathKey("Office", "Local Coil", "Cooling Coil Electricity Energy"): wantPath}
			native := EnergyExplanationNode{ID: "energy.direct_zone.end_use.cooling.electricity.office", Level: "energy", EndUse: "cooling", Carrier: "electricity", ZoneName: "Office", Value: 10, Unit: "kWh", Basis: "direct_zone_energy", SourceIDs: []string{"native"}, RelatedPathIDs: []string{"central-cooling"}}
			direct := []EnergyExplanationNode{native}
			qualifyEnergyPathNativeWindowACNodes(direct, sources, scope, paths)
			if !direct[0].nativeWindowACPathQualified {
				t.Fatal("native control lacks original ownership qualification")
			}
			// The real v1 allocator starts from a broad native consuming pool and
			// an explicit allocation edge, not a pre-labelled Zone energy node
			// (which the frozen custom-v1 compatibility path treats as direct).
			allocated := EnergyExplanationNode{ID: "allocated.cooling", Level: "energy", EndUse: "cooling", Carrier: "electricity", Value: 20, Unit: "kWh", Basis: "service_path_allocation", SourceIDs: []string{"shared-meter"}, RelatedPathIDs: []string{"central-cooling"}, hvacConsumptionPoolBound: true}
			nodes := []EnergyExplanationNode{direct[0], allocated,
				{ID: "native.electricity", Level: "energy", Carrier: "electricity", EndUse: "total", ZoneName: "Office", Value: 10, Unit: "kWh", Basis: "direct_zone_energy", SourceIDs: []string{"native"}},
				{ID: "central.electricity", Level: "energy", Carrier: "electricity", EndUse: "total", Value: 20, Unit: "kWh", SourceIDs: []string{"shared-meter"}},
				{ID: "load", Level: "load", ServiceKind: "cooling", ZoneName: "Office", Value: 100, Unit: "kWh", SourceIDs: []string{"load-weight"}, RelatedPathIDs: []string{"central-cooling", "window-ac"}},
			}
			edges := []EnergyExplanationEdge{
				{ID: "direct", FromID: "native.electricity", ToID: native.ID, Value: 10, Unit: "kWh", Relation: "energy_variable", Basis: "direct_zone_energy", SourceIDs: []string{"native"}},
				{ID: "allocated", FromID: "central.electricity", ToID: allocated.ID, Value: 20, Unit: "kWh", Relation: "meter_enduse", Basis: "service_path_allocation", SourceIDs: []string{"shared-meter", "load-weight"}},
				{ID: "allocation", FromID: allocated.ID, ToID: "load", Value: 20, Unit: "kWh", Relation: "allocation", RuleID: energyRelationshipRuleAllocatedHVACConsumptionPool, Basis: "service_path_allocation", ServiceKind: "cooling", SourceIDs: []string{"shared-meter", "load-weight"}, RelatedPathIDs: []string{"central-cooling"}},
			}
			if reverse {
				nodes[0], nodes[1] = nodes[1], nodes[0]
				edges[0], edges[1] = edges[1], edges[0]
			}
			built, links := upgradeEnergyExplanationGraph(nodes, edges, sources, scope, PurposeAllocationPolicyByServicePathLoadShare, true, nil)
			result := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: scope, Nodes: built, Links: links, Sources: sources}
			for pass := 0; pass < 3; pass++ {
				if pass > 0 {
					wire, err := json.Marshal(result)
					if err != nil {
						t.Fatal(err)
					}
					var decoded EnergyExplanationResult
					if err := json.Unmarshal(wire, &decoded); err != nil {
						t.Fatal(err)
					}
					result = decoded
				}
				endUse := energyPathV2NodeByID(result.Nodes, "end_use.cooling.office")
				if endUse == nil || endUse.Value != 30 || endUse.Basis != "service_path_allocation" || !endUse.AllocationApplied || !strings.Contains(endUse.AllocationExplanation, "not a fully measured") {
					t.Fatalf("reverse=%t unresolved=%t pass%d mixed cooling claims fully direct observation: %#v", reverse, unresolved, pass, endUse)
				}
				counts := map[string]int{}
				for _, link := range result.Links {
					if link.FromID != endUse.ID || !energyPathLinkIsCarrierSplit(link) {
						continue
					}
					counts[link.Basis]++
					switch link.Basis {
					case "direct_zone_energy":
						pathsMatch := reflect.DeepEqual(link.RelatedPathIDs, wantPath) || len(link.RelatedPathIDs) == 0 && len(wantPath) == 0
						if link.FromValue != 10 || link.ToValue != 10 || !reflect.DeepEqual(link.SourceIDs, []string{"native"}) || !pathsMatch {
							t.Fatalf("reverse=%t unresolved=%t pass%d native direct branch borrowed allocated source/path: %#v", reverse, unresolved, pass, link)
						}
					case "service_path_allocation":
						if link.FromValue != 20 || link.ToValue != 20 || len(link.SourceIDs) != 2 || !stringSliceContains(link.SourceIDs, "shared-meter") || !stringSliceContains(link.SourceIDs, "load-weight") {
							t.Fatalf("allocated cooling branch lost separate source: %#v", link)
						}
					default:
						t.Fatalf("mixed branch lost its basis: %#v", link)
					}
				}
				if counts["direct_zone_energy"] != 1 || counts["service_path_allocation"] != 1 || len(counts) != 2 {
					t.Fatalf("mixed cooling branches=%v", counts)
				}
			}
		}
	}
}

func TestEnergyPathWindowACMixedAnnualPreservesUnmixedMonths(t *testing.T) {
	for _, nativeFirst := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			scope := EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"}
			source := windowACPathSource("native", "Local Coil", "Cooling Coil Electricity Energy")
			period := func(id string, native bool) EnergyPeriod {
				basis, sourceID, path, value := "service_path_allocation", "shared", "central-cooling", 20.0
				if native {
					basis, sourceID, path, value = "direct_zone_energy", "native", "window-ac", 10
				}
				loadID, endUseID, carrierID := "load.cooling.office", "end_use.cooling.office", "carrier.electricity.office"
				nodes := []EnergyExplanationNode{
					{ID: loadID, Level: "load", ServiceKind: "cooling", ZoneName: "Office", Period: id, Value: value * 10, RawValue: value * 10, EffectiveValue: value * 10, AllocatedValue: value * 10, Unit: "kWh", ScaleDomain: "thermal", SourceIDs: []string{"load"}},
					{ID: endUseID, Level: "end_use", EndUse: "cooling", ZoneName: "Office", Period: id, Value: value, RawValue: value, EffectiveValue: value, AllocatedValue: value, Unit: "kWh", ScaleDomain: "site", Basis: basis, SourceIDs: []string{sourceID}, RelatedPathIDs: []string{path}, nativeWindowACPathQualified: native, AllocationApplied: !native},
					{ID: carrierID, Level: "carrier", Carrier: "electricity", ZoneName: "Office", Period: id, Value: value, RawValue: value, EffectiveValue: value, AllocatedValue: value, Unit: "kWh", ScaleDomain: "site", Basis: basis, SourceIDs: []string{sourceID}},
				}
				links := []EnergyPathLink{
					{ID: id + "-carrier", FromID: endUseID, ToID: carrierID, Relation: "end_use_to_carrier", Basis: basis, FromValue: value, ToValue: value, FromUnit: "kWh", ToUnit: "kWh", Period: id, ZoneName: "Office", SourceIDs: []string{sourceID}, RelatedPathIDs: []string{path}},
					{ID: id + "-conversion", FromID: loadID, ToID: endUseID, Relation: "load_to_end_use", ServiceKind: "cooling", Basis: basis, FromValue: value * 10, ToValue: value, FromUnit: "kWh", ToUnit: "kWh", Period: id, ZoneName: "Office", SourceIDs: []string{"load", sourceID}, RelatedPathIDs: []string{path}},
				}
				return EnergyPeriod{ID: id, Kind: "monthly", Nodes: nodes, Links: links}
			}
			periods := []EnergyPeriod{period("M1", nativeFirst), period("M2", !nativeFirst)}
			if reverse {
				periods[0], periods[1] = periods[1], periods[0]
			}
			before, err := json.Marshal(periods)
			if err != nil {
				t.Fatal(err)
			}
			result := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: scope, Periods: periods, Sources: []EnergyDataSource{source}}
			applyCanonicalMonthlyBasisToEnergyPathResult(&result)
			after, err := json.Marshal(result.Periods)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("annual mixture rewrote unmixed monthly graphs")
			}
			for pass := 0; pass < 3; pass++ {
				if pass > 0 {
					wire, err := json.Marshal(result)
					if err != nil {
						t.Fatal(err)
					}
					var decoded EnergyExplanationResult
					if err := json.Unmarshal(wire, &decoded); err != nil {
						t.Fatal(err)
					}
					result = decoded
				}
				node := energyPathV2NodeByID(result.Nodes, "end_use.cooling.office")
				if node == nil || node.Value != 30 || node.Basis != "service_path_allocation" || !node.AllocationApplied || !strings.Contains(node.AllocationExplanation, "not a fully measured") {
					t.Fatalf("nativeFirst=%t reverse=%t pass%d annual mixture lost evidence: %#v", nativeFirst, reverse, pass, node)
				}
				conversions, branches := 0, 0
				for _, link := range result.Links {
					if link.Relation == "load_to_end_use" {
						conversions++
						if link.FromValue != 300 || link.ToValue != 30 || link.Basis != "service_path_allocation" || link.RuleID != energyRelationshipRuleMixedHVACConsumptionBasis || !strings.Contains(link.Explanation, "not a fully measured") {
							t.Fatalf("annual mixed conversion lost paired domains/evidence: %#v", link)
						}
					}
					if link.FromID != node.ID || !energyPathLinkIsCarrierSplit(link) {
						continue
					}
					branches++
					want, id, path := 20.0, "shared", "central-cooling"
					if link.Basis == "direct_zone_energy" {
						want, id, path = 10, "native", "window-ac"
					} else if link.Basis != "service_path_allocation" {
						t.Fatal("annual branch lost basis")
					}
					if link.FromValue != want || link.ToValue != want || !reflect.DeepEqual(link.SourceIDs, []string{id}) || !reflect.DeepEqual(link.RelatedPathIDs, []string{path}) {
						t.Fatalf("annual branch mixed separate observed/allocated sources: %#v", link)
					}
				}
				if conversions != 1 || branches != 2 {
					t.Fatalf("annual conversion/branch census=%d/%d", conversions, branches)
				}
			}
		}
	}
}
