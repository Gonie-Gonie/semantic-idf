package simulation

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestEnergyPathAuxiliaryLinksSeparateServicesAndPreserveLedgers(t *testing.T) {
	input := auxiliaryLinkFixture(60, 40, 10)
	before, _ := json.Marshal(input)
	result := UpgradeEnergyExplanationV1(input)
	assertAuxiliaryLinks(t, result.Links, map[string][2]float64{"cooling": {60, 6}, "heating": {40, 4}})
	for _, link := range result.Links {
		if link.Relation == "load_to_end_use" || link.RatioKind != "" || link.Ratio != 0 {
			t.Fatalf("auxiliary energy acquired an equipment conversion: %+v", link)
		}
	}
	if got := auxiliaryLinkNode(result.Nodes, "end_use", "fans"); got.Value != 10 {
		t.Fatalf("service links changed the original Fans budget: %+v", got)
	}
	carrier := auxiliaryLinkNode(result.Nodes, "carrier", "")
	if carrier.Value != 10 || len(result.Reconciliation) == 0 || result.Reconciliation[0].ResidualValue != 0 {
		t.Fatalf("service links changed carrier closure: carrier=%+v accounting=%+v", carrier, result.Reconciliation)
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("building service attribution mutated source input")
	}
	for round := 0; round < 2; round++ {
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var restored EnergyExplanationResult
		if err := json.Unmarshal(encoded, &restored); err != nil {
			t.Fatal(err)
		}
		assertAuxiliaryLinks(t, restored.Links, map[string][2]float64{"cooling": {60, 6}, "heating": {40, 4}})
		result = restored
	}
}

func TestEnergyPathAuxiliaryLinksAnnualUsesCompletedMonthlySplits(t *testing.T) {
	input := auxiliaryLinkFixture(100, 100, 40)
	input.canonicalMonthlyBasis = true
	first, second := auxiliaryLinkFixture(90, 10, 10), auxiliaryLinkFixture(10, 90, 30)
	input.Periods = []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: first.Nodes, Edges: first.Edges}, {ID: "M2", Kind: "monthly", Nodes: second.Nodes, Edges: second.Edges}}
	result := UpgradeEnergyExplanationV1(input)
	assertAuxiliaryLinks(t, result.Links, map[string][2]float64{"cooling": {100, 12}, "heating": {100, 28}})
	for _, zone := range result.ZoneResults {
		assertAuxiliaryLinks(t, zone.Links, map[string][2]float64{"cooling": {100, 12}, "heating": {100, 28}})
	}
}

func TestEnergyPathAuxiliaryLinksMissingZeroAndOwnership(t *testing.T) {
	for _, scenario := range []string{"known opposite zero", "missing opposite", "missing opposite without owner paths", "no load", "ventilation only", "foreign load path", "direct only", "unresolved topology"} {
		t.Run(scenario, func(t *testing.T) {
			input := auxiliaryLinkFixture(60, 0, 10)
			switch scenario {
			case "missing opposite":
				input.Nodes = input.Nodes[:3]
			case "missing opposite without owner paths":
				input.Nodes = input.Nodes[:3]
				input.Nodes[1].RelatedPathIDs = nil
			case "no load":
				input.Nodes[2].Value = 0
			case "ventilation only":
				for i := range input.servicePathIndex.auxiliaryPaths {
					input.servicePathIndex.auxiliaryPaths[i].ServiceKind = "ventilation"
				}
			case "foreign load path":
				input.Nodes[2].RelatedPathIDs = []string{"foreign"}
			case "direct only":
				input.AllocationPolicy = PurposeAllocationPolicyDirectOnly
			case "unresolved topology":
				input.servicePathIndex = energyServicePathIndex{}
			}
			result := UpgradeEnergyExplanationV1(input)
			if scenario == "known opposite zero" {
				assertAuxiliaryLinks(t, result.Links, map[string][2]float64{"cooling": {60, 10}})
			} else {
				assertAuxiliaryLinks(t, result.Links, nil)
			}
			if auxiliaryLinkNode(result.Nodes, "end_use", "fans").Value != 10 {
				t.Fatal("unattributed auxiliary budget disappeared")
			}
		})
	}
}

func TestEnergyPathAuxiliaryLinksKeepIndependentFanPools(t *testing.T) {
	input := auxiliaryLinkFixture(60, 40, 10)
	paths := input.servicePathIndex.auxiliaryPaths
	paths[0].AirLoopName, paths[1].AirLoopName = "Cooling Air", "Heating Air"
	input.servicePathIndex.auxiliaryPaths = paths
	owner := input.Nodes[1]
	pools := []energyPathFanPool{
		auxiliaryLinkFanPool("fan.cool", "Cooling Air"), auxiliaryLinkFanPool("fan.heat", "Heating Air"),
	}
	sources := append(append([]EnergyDataSource(nil), input.Sources...), pools[0].Source, pools[1].Source)
	plan := buildEnergyPathZoneAuxiliaryAllocationPlan(input.Nodes, nil, input.servicePathIndex, "annual", "annual", false)
	if len(plan.Edges) != 1 {
		t.Fatalf("fixture has no original combined Zone edge: %+v", plan)
	}
	plan.Edges[0].SourceIDs = append(plan.Edges[0].SourceIDs, "fan.cool", "fan.heat")
	plan.FanSourceAllocations = []energyPathFanSourceAllocation{{SourceID: "fan.cool", ZoneName: "Office", Value: 3}, {SourceID: "fan.heat", ZoneName: "Office", Value: 7}}
	nodes, _ := upgradeEnergyExplanationGraph(input.Nodes, input.Edges, sources, input.scope, input.AllocationPolicy, false, nil)
	links := buildEnergyPathAuxiliaryServiceLinks(nodes, input.Nodes, sources, plan, input.servicePathIndex, pools, input.scope, "annual")
	assertAuxiliaryLinks(t, links, map[string][2]float64{"cooling": {60, 3}, "heating": {40, 7}})
	for _, link := range links {
		other := "fan.heat"
		if link.ServiceKind == "heating" {
			other = "fan.cool"
		}
		if stringSliceContains(link.SourceIDs, other) || !stringSliceContains(link.SourceIDs, owner.SourceIDs[0]) {
			t.Fatalf("source-local attribution borrowed another fan pool: %+v", link)
		}
	}
	duplicate := append(append([]energyPathFanPool(nil), pools...), pools[0])
	if got := buildEnergyPathAuxiliaryServiceLinks(nodes, input.Nodes, sources, plan, input.servicePathIndex, duplicate, input.scope, "annual"); len(got) != 0 {
		t.Fatalf("duplicate pool identity created links: %+v", got)
	}
}

func TestEnergyPathAuxiliaryLinksRejectForgedStoredMetadata(t *testing.T) {
	input := auxiliaryLinkFixture(60, 40, 10)
	result := UpgradeEnergyExplanationV1(input)
	var original EnergyPathLink
	for _, link := range result.Links {
		if link.Relation == energyPathRelationLoadToAuxiliary {
			original = link
			break
		}
	}
	if original.ID == "" {
		t.Fatal("fixture has no attribution")
	}
	for _, scenario := range []string{"ratio", "foreign source", "foreign path", "foreign scope", "foreign period", "extra thermal", "extra site", "wrong end use"} {
		t.Run(scenario, func(t *testing.T) {
			link := original
			link.SourceIDs, link.RelatedPathIDs = append([]string(nil), original.SourceIDs...), append([]string(nil), original.RelatedPathIDs...)
			switch scenario {
			case "ratio":
				link.RatioKind, link.Ratio = "COP", 10
			case "foreign source":
				link.SourceIDs = append(link.SourceIDs, "foreign")
			case "foreign path":
				link.RelatedPathIDs = []string{"foreign"}
			case "foreign scope":
				link.ZoneName = "Other"
			case "foreign period":
				link.Period = "M8"
			case "extra thermal":
				link.FromValue = 1000
			case "extra site":
				link.ToValue = 1000
			case "wrong end use":
				link.ToID = auxiliaryLinkNode(result.Nodes, "carrier", "").ID
			}
			if got := filterEnergyPathAuxiliaryServiceLinks(result.Nodes, []EnergyPathLink{link}, result.Sources, result.Scope, "annual"); len(got) != 0 {
				t.Fatalf("invalid %s attribution survived: %+v", scenario, got)
			}
		})
	}
}

func TestEnergyPathAuxiliaryLinksUseEffectiveLoadsOnce(t *testing.T) {
	input := auxiliaryLinkFixture(30, 40, 10)
	input.Nodes[2].Multiplier = 2
	result := UpgradeEnergyExplanationV1(input)
	assertAuxiliaryLinks(t, result.Links, map[string][2]float64{"cooling": {60, 6}, "heating": {40, 4}})
}

func TestEnergyPathAuxiliaryLinksNeverEnterEquipmentCOP(t *testing.T) {
	input := auxiliaryLinkFixture(60, 40, 10)
	input.Nodes[0].Value = 30
	cooling := epath101AuditAuxiliary("cooling", 20, "meter.cooling", []string{"office.cooling"})
	input.Nodes = append(input.Nodes, cooling)
	input.Sources = append(input.Sources, EnergyDataSource{ID: "meter.cooling", SourceType: "sql_meter", IsMeter: true})
	input.Edges = append(input.Edges, EnergyExplanationEdge{ID: "cooling.site", FromID: input.Nodes[0].ID, ToID: cooling.ID, Relation: "meter_enduse", Value: 20, Unit: "kWh site", SourceIDs: []string{"meter.cooling"}})
	input.Edges = append(input.Edges, EnergyExplanationEdge{ID: "cooling.delivered", FromID: cooling.ID, ToID: input.Nodes[2].ID,
		Relation: "delivered_load", RuleID: energyRelationshipRuleMeasuredLoad, Value: 60, Unit: "kWh thermal", RelatedPathIDs: []string{"office.cooling"}, SourceIDs: []string{"meter.cooling", "load.cool"}})
	result := UpgradeEnergyExplanationV1(input)
	assertAuxiliaryLinks(t, result.Links, map[string][2]float64{"cooling": {60, 6}, "heating": {40, 4}})
	conversions := 0
	for _, link := range result.Links {
		if link.Relation == "load_to_end_use" {
			conversions++
			if link.FromValue != 60 || link.ToValue != 20 || link.Ratio != 3 {
				t.Fatalf("fan energy changed equipment COP/denominator: %+v", link)
			}
		}
	}
	if conversions != 1 {
		t.Fatalf("expected one original measured cooling conversion, got %d: %+v", conversions, result.Links)
	}
}

func TestEnergyPathAuxiliaryLinksPumpsRequireProvenSingleServicePlants(t *testing.T) {
	for _, mixedPlant := range []bool{false, true} {
		input := auxiliaryLinkFixture(60, 40, 10)
		input.Nodes[1] = epath101AuditAuxiliary("pumps", 10, "meter.fans", input.Nodes[1].RelatedPathIDs)
		input.Edges[0].ToID = input.Nodes[1].ID
		input.servicePathIndex.auxiliaryPaths[0].PlantLoopName = "Cooling Water"
		input.servicePathIndex.auxiliaryPaths[1].PlantLoopName = "Heating Water"
		if mixedPlant {
			input.servicePathIndex.auxiliaryPaths[1].PlantLoopName = "Cooling Water"
		}
		result := UpgradeEnergyExplanationV1(input)
		if mixedPlant {
			assertAuxiliaryLinks(t, result.Links, nil)
		} else {
			assertAuxiliaryLinks(t, result.Links, map[string][2]float64{"cooling": {60, 6}, "heating": {40, 4}})
		}
		if auxiliaryLinkNode(result.Nodes, "end_use", "pumps").Value != 10 {
			t.Fatal("service attribution changed the original pump budget")
		}
	}
}

func TestEnergyPathAuxiliaryLinksSourcePumpSharesAndServedSubtotal(t *testing.T) {
	input := auxiliaryLinkFixture(60, 40, 10)
	input.Nodes[1] = epath101AuditAuxiliary("pumps", 10, "meter.fans", input.Nodes[1].RelatedPathIDs)
	input.Edges[0].ToID = input.Nodes[1].ID
	input.servicePathIndex.auxiliaryPaths[0].PlantLoopName = "Cooling Water"
	input.servicePathIndex.auxiliaryPaths[1].PlantLoopName = "Heating Water"
	plan := buildEnergyPathZoneAuxiliaryAllocationPlan(input.Nodes, nil, input.servicePathIndex, "annual", "annual", false)
	if len(plan.Edges) != 1 {
		t.Fatalf("expected the existing combined Zone pump budget: %+v", plan.Edges)
	}
	for i := range plan.Edges {
		plan.Edges[i].SourceIDs = append(plan.Edges[i].SourceIDs, "pump.native")
	}
	plan.PoolPumpSourceAllocations = []energyPathPoolPumpSourceAllocation{
		{SourceID: "pump.native", ZoneName: "Office", ServiceKind: "cooling", Value: 6, LoadSourceIDs: []string{"load.cool"}, RelatedPathIDs: []string{"office.cooling"}},
		{SourceID: "pump.native", ZoneName: "Office", ServiceKind: "heating", Value: 4, LoadSourceIDs: []string{"load.heat"}, RelatedPathIDs: []string{"office.heating"}},
	}
	sources := append(append([]EnergyDataSource(nil), input.Sources...), EnergyDataSource{ID: "pump.native", SourceType: "sql_report_data", Name: "Pump Electricity Energy", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly"})
	// A second unserved Zone is part of the Building cooling endpoint, but
	// must never enlarge this pump's actual served thermal reference.
	input.Nodes = append(input.Nodes, epath101AuditLoad("Unserved", "cooling", 900, "load.unserved", []string{"unserved.cooling"}))
	sources = append(sources, EnergyDataSource{ID: "load.unserved", SourceType: "sql_variable", ZoneName: "Unserved"})
	nodes, _ := upgradeEnergyExplanationGraph(input.Nodes, input.Edges, sources, input.scope, input.AllocationPolicy, false, nil)
	links := buildEnergyPathAuxiliaryServiceLinks(nodes, input.Nodes, sources, plan, input.servicePathIndex, nil, input.scope, "annual")
	assertAuxiliaryLinks(t, links, map[string][2]float64{"cooling": {60, 6}, "heating": {40, 4}})
}

func TestEnergyPathAuxiliaryLinksStableIdentityAndServiceProjections(t *testing.T) {
	input := auxiliaryLinkFixture(60, 40, 10)
	month := auxiliaryLinkFixture(15, 10, 2.5)
	input.Periods = []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: month.Nodes, Edges: month.Edges}}
	result := UpgradeEnergyExplanationV1(input)
	annualIDs := map[string]string{}
	for _, link := range result.Links {
		if link.Relation == energyPathRelationLoadToAuxiliary {
			annualIDs[link.ServiceKind] = link.ID
		}
	}
	for _, period := range result.Periods {
		if period.ID != "M1" {
			continue
		}
		assertAuxiliaryLinks(t, period.Links, map[string][2]float64{"cooling": {15, 1.5}, "heating": {10, 1}})
		for _, link := range period.Links {
			if link.Relation == energyPathRelationLoadToAuxiliary && link.ID != annualIDs[link.ServiceKind] {
				t.Fatalf("monthly item lost the Annual identity: %+v", link)
			}
		}
	}
	// A shared Fans card may retain one constituent's service metadata.
	// Attribution service belongs to its load, never to that entire card.
	for i := range result.Nodes {
		if result.Nodes[i].EndUse == "fans" {
			result.Nodes[i].ServiceKind = "heating"
		}
	}
	for _, service := range []string{"all", "cooling", "heating"} {
		selection := EnergyPathSelection{Scope: "building", Period: "annual", Service: service}
		projection, err := ProjectEnergyPath(PurposeResultBundle{EnergyExplanation: result}, selection)
		if err != nil {
			t.Fatalf("%s projection rejected proven shared auxiliary attribution: %v", service, err)
		}
		want := map[string][2]float64{"cooling": {15, 1.5}, "heating": {10, 1}}
		if service != "all" {
			want = map[string][2]float64{service: want[service]}
		}
		assertAuxiliaryLinks(t, projection.View.Links, want)
	}
}

func TestEnergyPathAuxiliaryLinksOnlyQualifiedNativeWindowACFan(t *testing.T) {
	scope := EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"}
	path := energyPathAuxiliaryServicePath{ID: "window.cooling", ZoneName: "Office", ServiceKind: "cooling"}
	fan := EnergyExplanationNode{ID: "energy.direct_zone.end_use.fans.electricity.office", Level: "energy", Kind: "energy.fans", EndUse: "fans", Carrier: "electricity", ZoneName: "Office",
		Value: 2, Unit: "kWh", Basis: "direct_zone_energy", SourceIDs: []string{"fan.native"}}
	load := epath101AuditLoad("Office", "cooling", 60, "load.cool", []string{path.ID})
	sources := []EnergyDataSource{
		{ID: "fan.native", SourceType: "sql_report_data", Name: "Fan Electricity Energy", KeyValue: "Window Fan", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", ZoneName: "Office"},
		{ID: "load.cool", SourceType: "sql_report_data", ZoneName: "Office"},
	}
	for _, qualified := range []bool{false, true} {
		original := []EnergyExplanationNode{fan, load}
		if qualified {
			qualifyEnergyPathNativeWindowACNodes(original, sources, scope, map[string][]string{energyPathNativeWindowACPathKey("Office", "Window Fan", "Fan Electricity Energy"): {path.ID}})
		}
		nodes := []EnergyExplanationNode{upgradeEnergyExplanationNode(original[0], scope), upgradeEnergyExplanationNode(original[1], scope)}
		links := buildEnergyPathAuxiliaryServiceLinks(nodes, original, sources, energyPathZoneAuxiliaryAllocationPlan{}, epath101AuditTopology([]energyPathAuxiliaryServicePath{path}), nil, scope, "annual")
		if qualified {
			assertAuxiliaryLinks(t, links, map[string][2]float64{"cooling": {60, 2}})
		} else {
			assertAuxiliaryLinks(t, links, nil)
		}
	}
}

func TestEnergyPathAuxiliaryLinksHeatRejectionRequiresUnambiguousCondenser(t *testing.T) {
	for _, mixedCondenser := range []bool{false, true} {
		input := auxiliaryLinkFixture(60, 40, 10)
		input.Nodes[1] = epath101AuditAuxiliary("heat_rejection", 10, "meter.fans", input.Nodes[1].RelatedPathIDs)
		input.Edges[0].ToID = input.Nodes[1].ID
		input.servicePathIndex.auxiliaryPaths[0].PlantLoopName = "Cooling Water"
		input.servicePathIndex.auxiliaryPaths[0].CondenserLoopName = "Condenser Water"
		input.servicePathIndex.auxiliaryPaths[1].PlantLoopName = "Heating Water"
		if mixedCondenser {
			input.servicePathIndex.auxiliaryPaths[1].CondenserLoopName = "Condenser Water"
		}
		result := UpgradeEnergyExplanationV1(input)
		if mixedCondenser {
			assertAuxiliaryLinks(t, result.Links, nil)
		} else {
			assertAuxiliaryLinks(t, result.Links, map[string][2]float64{"cooling": {60, 10}})
		}
		if auxiliaryLinkNode(result.Nodes, "end_use", "heat_rejection").Value != 10 {
			t.Fatal("service attribution changed the original heat rejection budget")
		}
	}
}

func auxiliaryLinkFixture(cooling, heating, fan float64) EnergyExplanationV1 {
	paths := []energyPathAuxiliaryServicePath{epath101AuditPath("office.cooling", "Office", "cooling", "Main Air", "", ""), epath101AuditPath("office.heating", "Office", "heating", "Main Air", "", "")}
	ids := epath101AuditPathIDs(paths)
	nodes := []EnergyExplanationNode{epath101AuditCarrier(fan), epath101AuditAuxiliary("fans", fan, "meter.fans", ids), epath101AuditLoad("Office", "cooling", cooling, "load.cool", ids[:1]), epath101AuditLoad("Office", "heating", heating, "load.heat", ids[1:])}
	return EnergyExplanationV1{Schema: energyExplanationV1Schema, AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes: nodes, Edges: []EnergyExplanationEdge{{ID: "fan.site", FromID: nodes[0].ID, ToID: nodes[1].ID, Relation: "meter_enduse", Value: fan, Unit: "kWh site", SourceIDs: []string{"meter.fans"}}},
		Sources: []EnergyDataSource{{ID: "meter.facility", SourceType: "sql_meter", IsMeter: true}, {ID: "meter.fans", SourceType: "sql_meter", IsMeter: true}, {ID: "load.cool", SourceType: "sql_variable", ZoneName: "Office"}, {ID: "load.heat", SourceType: "sql_variable", ZoneName: "Office"}},
		scope:   EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}, servicePathIndex: epath101AuditTopology(paths)}
}

func auxiliaryLinkFanPool(id, loop string) energyPathFanPool {
	return energyPathFanPool{AirLoopName: loop, Source: EnergyDataSource{ID: id, SourceType: "sql_report_data", Name: "Air System Fan Electricity Energy", KeyValue: loop, SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Hourly"}}
}

func auxiliaryLinkNode(nodes []EnergyExplanationNode, level, endUse string) EnergyExplanationNode {
	for _, node := range nodes {
		if node.Level == level && (endUse == "" || node.EndUse == endUse) {
			return node
		}
	}
	return EnergyExplanationNode{}
}

func assertAuxiliaryLinks(t *testing.T, links []EnergyPathLink, want map[string][2]float64) {
	t.Helper()
	got := map[string][2]float64{}
	for _, link := range links {
		if link.Relation != energyPathRelationLoadToAuxiliary {
			continue
		}
		if link.Ratio != 0 || link.RatioKind != "" || link.RatioLabel != "" || link.Basis != "service_path_allocation" || len(link.SourceIDs) == 0 || len(link.RelatedPathIDs) == 0 {
			t.Fatalf("attribution loses allocation semantics: %+v", link)
		}
		got[link.ServiceKind] = [2]float64{link.FromValue, link.ToValue}
	}
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("auxiliary service links = %v, want %v; all links=%+v", got, want, links)
	}
}
