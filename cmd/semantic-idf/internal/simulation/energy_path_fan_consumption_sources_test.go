package simulation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"
)

func TestEnergyPathFanConsumptionRetainsOnlyAllocatedPoolSources(t *testing.T) {
	_, path := epathFanPoolSQL(t, 2)
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}, energyDriverBuildContext{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	legacy.AllocationPolicy = PurposeAllocationPolicyByServicePathLoadShare
	legacy.servicePathIndex = epathFanPoolTopology(2)
	before, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	for zone := 0; zone < 4; zone++ {
		input := legacy
		input.scope = EnergyExplanationScope{Kind: "zone", ZoneName: fmt.Sprintf("Zone %d", zone)}
		result := UpgradeEnergyExplanationV1(input)
		want := []string{"sql-rdd-1", fmt.Sprintf("sql-rdd-%d", 100+zone/2)}
		sort.Strings(want)
		check := func(period string, nodes []EnergyExplanationNode, links []EnergyPathLink) {
			t.Helper()
			byID := map[string]EnergyExplanationNode{}
			for _, node := range nodes {
				byID[node.ID] = node
			}
			count := 0
			for _, link := range links {
				from, to := byID[link.FromID], byID[link.ToID]
				if from.Level != "end_use" || from.EndUse != "fans" || to.Level != "carrier" {
					continue
				}
				count++
				if to.Carrier != "electricity" || link.Basis != "service_path_allocation" || link.Relation != "direct_end_use_to_carrier" || link.FromValue <= 0 || link.FromValue != link.ToValue || link.FromValue != from.Value {
					t.Fatalf("quantity/domain changed %s/%s: %#v", input.scope.ZoneName, period, link)
				}
				if !reflect.DeepEqual(link.SourceIDs, want) {
					t.Errorf("%s/%s carrier branch lost exact allocated pool or borrowed a sibling/weight/facility: %v want %v", input.scope.ZoneName, period, link.SourceIDs, want)
				}
				if !stringSliceContains(from.SourceIDs, fmt.Sprintf("sql-rdd-%d", 200+zone)) {
					t.Fatalf("node lost its exact thermal weight context %s/%s", input.scope.ZoneName, period)
				}
				if !reflect.DeepEqual(link.RelatedPathIDs, []string{fmt.Sprintf("path.%d.%d", zone/2, zone%2)}) {
					t.Fatalf("fan branch escaped own AirLoop path: %v", link.RelatedPathIDs)
				}
			}
			if count != 1 {
				t.Fatalf("%s/%s expected one physical fan branch, got %d", input.scope.ZoneName, period, count)
			}
		}
		check("annual", result.Nodes, result.Links)
		months := 0
		for _, period := range result.Periods {
			if period.Kind == "monthly" {
				check(period.ID, period.Nodes, period.Links)
				months++
			}
		}
		if months != 12 {
			t.Fatalf("expected actual 12 monthly graphs, got %d", months)
		}
	}
	building := UpgradeEnergyExplanationV1(legacy)
	for _, link := range building.Links {
		if link.Relation == "direct_end_use_to_carrier" && !reflect.DeepEqual(link.SourceIDs, []string{"sql-rdd-1"}) {
			t.Fatalf("Building broad observation gained allocated pool evidence: %v", link.SourceIDs)
		}
	}
	after, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("projection changed original V1 payload")
	}
}

func TestEnergyPathFanConsumptionRequiresExactObservedPoolIdentity(t *testing.T) {
	valid := energyPathFanPool{
		AirLoopName: "VAV_1",
		Source: EnergyDataSource{
			ID: "pool.1", SourceType: "sql_report_data", Name: "Air System Fan Electricity Energy",
			KeyValue: "VAV_1", ReportingFrequency: "Hourly", SourceUnit: "J", NormalizedUnit: "kWh",
		},
	}
	if got := buildEnergyPathFanConsumptionSources([]energyPathFanPool{valid}); !reflect.DeepEqual(got, map[string]bool{"pool.1": true}) {
		t.Fatalf("exact observed pool missing: %v", got)
	}
	for _, test := range []struct {
		name string
		edit func(*energyPathFanPool)
	}{
		{"invalid observation", func(p *energyPathFanPool) { p.Invalid = true }},
		{"missing ID", func(p *energyPathFanPool) { p.Source.ID = "" }},
		{"wrong loop key", func(p *energyPathFanPool) { p.Source.KeyValue = "VAV_2" }},
		{"missing loop", func(p *energyPathFanPool) { p.AirLoopName = "" }},
		{"wildcard loop", func(p *energyPathFanPool) { p.AirLoopName, p.Source.KeyValue = "*", "*" }},
		{"not SQL observation", func(p *energyPathFanPool) { p.Source.SourceType = "derived" }},
		{"meter", func(p *energyPathFanPool) { p.Source.IsMeter = true }},
		{"thermal weighting source", func(p *energyPathFanPool) { p.Source.Name = "Zone Air System Sensible Cooling Energy" }},
		{"different fan variable", func(p *energyPathFanPool) { p.Source.Name = "Fan Electricity Energy" }},
		{"unobserved monthly alias", func(p *energyPathFanPool) { p.Source.ReportingFrequency = "Monthly" }},
		{"rate unit", func(p *energyPathFanPool) { p.Source.SourceUnit = "W" }},
		{"wrong normalized unit", func(p *energyPathFanPool) { p.Source.NormalizedUnit = "MJ" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			pool := valid
			test.edit(&pool)
			if got := buildEnergyPathFanConsumptionSources([]energyPathFanPool{pool}); len(got) != 0 {
				t.Fatalf("unqualified identity became branch evidence: %v", got)
			}
		})
	}
	for _, duplicateID := range []bool{false, true} {
		other := valid
		if duplicateID {
			other.AirLoopName, other.Source.KeyValue = "VAV_2", "VAV_2"
		} else {
			other.Source.ID = "pool.2"
			other.AirLoopName, other.Source.KeyValue = "vav_1", "vav_1"
		}
		if got := buildEnergyPathFanConsumptionSources([]energyPathFanPool{valid, other}); len(got) != 0 {
			t.Fatalf("ambiguous ID/loop became branch evidence (duplicate ID=%v): %v", duplicateID, got)
		}
	}
	alias := valid
	alias.AirLoopName, alias.Source.KeyValue = " vav_1 ", " VAV_1 "
	alias.Source.Name = " Air System Fan Electric Energy "
	if got := buildEnergyPathFanConsumptionSources([]energyPathFanPool{alias}); !got[valid.Source.ID] {
		t.Fatalf("supported observed legacy alias/case normalization rejected: %v", got)
	}
}

func TestEnergyPathFanConsumptionDoesNotBorrowContextOrOtherCarrier(t *testing.T) {
	base := EnergyExplanationNode{
		EndUse: "fans", Carrier: "electricity", Basis: "service_path_allocation", AllocationApplied: true,
		SourceIDs:           []string{"broad", "pool.own", "pool.sibling", "zone.load", "facility"},
		allocationSourceIDs: []string{"broad", "zone.load", "pool.own", "pool.own"},
	}
	index := map[string]bool{"pool.own": true, "pool.sibling": true}
	for _, test := range []struct {
		name string
		edit func(*EnergyExplanationNode)
		want []string
	}{
		{"selected observed pool only", func(*EnergyExplanationNode) {}, []string{"broad", "pool.own"}},
		{"other carrier", func(n *EnergyExplanationNode) { n.Carrier = "natural_gas" }, []string{"broad"}},
		{"missing carrier", func(n *EnergyExplanationNode) { n.Carrier = "" }, []string{"broad"}},
		{"other end use", func(n *EnergyExplanationNode) { n.EndUse = "pumps" }, []string{"broad"}},
		{"not allocated", func(n *EnergyExplanationNode) { n.AllocationApplied = false }, []string{"broad"}},
		{"direct observation", func(n *EnergyExplanationNode) { n.Basis = "direct_zone_energy" }, []string{"broad"}},
		{"missing allocation trace", func(n *EnergyExplanationNode) { n.allocationSourceIDs = nil }, []string{"broad"}},
		{"unknown allocation source", func(n *EnergyExplanationNode) { n.allocationSourceIDs = []string{"unknown", "zone.load"} }, []string{"broad"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			node := base
			test.edit(&node)
			if got := appendEnergyPathFanConsumptionSources([]string{"broad"}, node, index); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("branch evidence=%v want %v", got, test.want)
			}
		})
	}
}
