package simulation

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestEnergyPathHVACConsumptionMixedBasisPreservesBranches(t *testing.T) {
	scope := EnergyExplanationScope{Kind: "zone", ZoneName: "A"}
	sources := []EnergyDataSource{{ID: "native", SourceType: "sql_report_data", Name: "Baseboard Electricity Energy", ReportingFrequency: "Monthly", SourceUnit: "J", NormalizedUnit: "kWh", ZoneName: "A"}}
	nodes := []EnergyExplanationNode{{ID: "load", Level: "load", ServiceKind: "heating", ScaleDomain: "thermal", Unit: "kWh", ZoneName: "A", Value: 100}, {ID: "heat", Level: "end_use", EndUse: "heating", ScaleDomain: "site", Unit: "kWh", ZoneName: "A", Value: 30, Basis: "direct_zone_energy"}}
	links := []EnergyPathLink{
		{ID: "direct", FromID: "heat", ToID: "electric", Relation: "end_use_to_carrier", Basis: "direct_zone_energy", FromValue: 10, ToValue: 10, SourceIDs: []string{"native"}},
		{ID: "allocated", FromID: "heat", ToID: "gas", Relation: "end_use_to_carrier", Basis: "service_path_allocation", FromValue: 20, ToValue: 20, SourceIDs: []string{"meter", "weight"}},
		{ID: "conversion", FromID: "load", ToID: "heat", Relation: "load_to_end_use", Basis: "direct_zone_energy", FromValue: 100, ToValue: 30, ServiceKind: "heating", ZoneName: "A", FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"native", "meter", "weight"}},
	}
	out := qualifyEnergyPathMixedConsumptionBasis(nodes, links, sources, scope)
	if nodes[1].Basis != "service_path_allocation" || nodes[1].Value != 30 || !strings.Contains(nodes[1].AllocationExplanation, "not a fully measured") {
		t.Fatal("mixed energy still claims an entirely direct measured Zone subtotal")
	}
	for _, link := range out {
		switch link.ID {
		case "direct":
			if !reflect.DeepEqual(link, links[0]) {
				t.Fatal("direct branch changed")
			}
		case "allocated":
			if !reflect.DeepEqual(link, links[1]) {
				t.Fatal("allocated branch changed")
			}
		}
		if link.Relation == "load_to_end_use" && (link.Basis != "service_path_allocation" || link.RuleID != energyRelationshipRuleMixedHVACConsumptionBasis || link.FromValue != 100 || link.ToValue != 30) {
			t.Fatal("mixed conversion lost its observed endpoints or aggregate basis")
		}
	}
	again := qualifyEnergyPathMixedConsumptionBasis(nodes, out, sources, scope)
	if !reflect.DeepEqual(out, again) {
		t.Fatal("mixed basis qualification is not idempotent")
	}
	for name, mutate := range map[string]func(*EnergyDataSource){"hourly": func(s *EnergyDataSource) { s.ReportingFrequency = "Hourly" }, "other Zone": func(s *EnergyDataSource) { s.ZoneName = "B" }, "meter": func(s *EnergyDataSource) { s.IsMeter = true }, "packaged coil": func(s *EnergyDataSource) { s.Name = "Heating Coil Electricity Energy" }} {
		t.Run(name, func(t *testing.T) {
			copySource := sources[0]
			mutate(&copySource)
			copyNodes := append([]EnergyExplanationNode(nil), nodes...)
			before := append([]EnergyExplanationNode(nil), copyNodes...)
			got := qualifyEnergyPathMixedConsumptionBasis(copyNodes, links, []EnergyDataSource{copySource}, scope)
			if !reflect.DeepEqual(links, got) || !reflect.DeepEqual(before, copyNodes) {
				t.Fatal("unrelated/unknown source changed the legacy projection")
			}
		})
	}
}

func TestEnergyPathHVACConsumptionSQLMixedBasisAndReload(t *testing.T) {
	path, input, _, plan, context := mixedHeatingConsumptionSQLFixture(t)
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(enrichEnergyExplanationWithServicePaths(legacy, input))
	check := func(result EnergyExplanationResult) {
		t.Helper()
		for _, zone := range result.ZoneResults {
			if zone.Scope.ZoneName != "SPACE2-1" && zone.Scope.ZoneName != "SPACE4-1" {
				continue
			}
			periods := append([]EnergyPeriod{{ID: "annual", Nodes: zone.Nodes, Links: zone.Links}}, zone.Periods...)
			for _, period := range periods {
				var endUse, load float64
				var endUseID string
				for _, node := range period.Nodes {
					if node.Level == "load" && node.ServiceKind == "heating" {
						load = node.Value
					}
					if node.Level == "end_use" && node.EndUse == "heating" {
						endUse = node.Value
						endUseID = node.ID
						if node.Basis != "service_path_allocation" || !strings.Contains(node.AllocationExplanation, "Mixed Zone subtotal") {
							t.Errorf("%s/%s mixed subtotal claims direct evidence: %s", zone.Scope.ZoneName, period.ID, node.Basis)
						}
					}
				}
				var from, to, direct, allocated float64
				for _, link := range period.Links {
					if link.Relation == "load_to_end_use" && link.ToID == endUseID {
						from += link.FromValue
						to += link.ToValue
						if link.Basis != "service_path_allocation" || link.RuleID != energyRelationshipRuleMixedHVACConsumptionBasis {
							t.Errorf("%s/%s mixed conversion qualification absent", zone.Scope.ZoneName, period.ID)
						}
					}
					if link.Relation == "end_use_to_carrier" && link.FromID == endUseID {
						if link.Basis == "direct_zone_energy" {
							direct += link.ToValue
						} else if link.Basis == "service_path_allocation" {
							allocated += link.ToValue
						}
					}
				}
				if math.Abs(from-load) > 1e-9 || math.Abs(to-endUse) > 1e-9 || math.Abs(direct+allocated-endUse) > 1e-9 || direct <= 0 || allocated <= 0 {
					t.Errorf("%s/%s native and allocated branches or single thermal endpoint changed: load=%g from=%g endUse=%g to=%g direct=%g allocated=%g", zone.Scope.ZoneName, period.ID, load, from, endUse, to, direct, allocated)
				}
				factor := 1.0
				if period.ID == "annual" {
					factor = 3
				} else if period.ID == "M2" {
					factor = 2
				}
				wantDirect := 10.0 * factor
				if zone.Scope.ZoneName == "SPACE4-1" {
					wantDirect = 30 * factor
				}
				if math.Abs(direct-wantDirect) > 1e-9 || math.Abs(allocated-22*factor) > 1e-9 {
					t.Errorf("%s/%s shared electricity became a direct observation: direct=%g want=%g allocated=%g want=%g", zone.Scope.ZoneName, period.ID, direct, wantDirect, allocated, 22*factor)
				}
			}
		}
	}
	check(result)
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var reopened EnergyExplanationResult
	if err := json.Unmarshal(wire, &reopened); err != nil {
		t.Fatal(err)
	}
	check(reopened)
}
