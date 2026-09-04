package simulation

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestEPATH100AuditEnergyPathOnlyDefaultsToServicePathAllocation(t *testing.T) {
	tests := []struct {
		name    string
		request SimulationPurposeRequest
		want    string
	}{
		{
			name: "energy path blank policy",
			request: SimulationPurposeRequest{
				Purposes:          []SimulationPurposeID{SimulationPurposeBasicEnergy},
				BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath,
			},
			want: PurposeAllocationPolicyByServicePathLoadShare,
		},
		{
			name: "explicit direct-only compatibility",
			request: SimulationPurposeRequest{
				Purposes:          []SimulationPurposeID{SimulationPurposeBasicEnergy},
				BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath,
				AllocationPolicy:  PurposeAllocationPolicyDirectOnly,
			},
			want: PurposeAllocationPolicyDirectOnly,
		},
		{
			name: "non-energy-path blank policy",
			request: SimulationPurposeRequest{
				Purposes:          []SimulationPurposeID{SimulationPurposeBasicEnergy},
				BasicEnergyDetail: PurposeBasicEnergyDetailLight,
			},
			want: PurposeAllocationPolicyDirectOnly,
		},
		{
			name: "different purpose blank policy",
			request: SimulationPurposeRequest{
				Purposes: []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
			},
			want: PurposeAllocationPolicyDirectOnly,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := NormalizeSimulationPurposeRequest(&test.request)
			if got.AllocationPolicy != test.want {
				t.Fatalf("allocation policy = %q, want %q", got.AllocationPolicy, test.want)
			}
		})
	}

	// An empty policy on a stored v1 payload predates EPATH-100. The v1 -> v2
	// adapter must not silently reinterpret that persisted graph as allocated.
	stored := UpgradeEnergyExplanationV1(EnergyExplanationV1{Schema: energyExplanationV1Schema})
	if stored.AllocationPolicy != PurposeAllocationPolicyDirectOnly {
		t.Fatalf("stored v1 empty policy upgraded to %q, want compatibility direct_only", stored.AllocationPolicy)
	}
}

func TestEPATH100AuditMonthlyExactServicePathsAreBoundedAndOrderInvariant(t *testing.T) {
	forwardLegacy := applyEnergyExplanationV1ServicePathLoadShareAllocation(epath100AuditExactPathFixture(false))
	reverseLegacy := applyEnergyExplanationV1ServicePathLoadShareAllocation(epath100AuditExactPathFixture(true))

	for _, item := range []struct {
		period string
		from   string
		to     string
		want   float64
	}{
		{period: "annual", from: "energy.end_use.cooling.electricity", to: "load.cooling.office", want: 55},
		{period: "annual", from: "energy.end_use.cooling.electricity", to: "load.cooling.lab", want: 45},
		{period: "M1", from: "energy.end_use.cooling.electricity", to: "load.cooling.office", want: 10},
		{period: "M1", from: "energy.end_use.cooling.electricity", to: "load.cooling.lab", want: 30},
		{period: "M2", from: "energy.end_use.cooling.electricity", to: "load.cooling.office", want: 45},
		{period: "M2", from: "energy.end_use.cooling.electricity", to: "load.cooling.lab", want: 15},
	} {
		edges := forwardLegacy.Edges
		if item.period != "annual" {
			period := energyExplanationPeriodByID(forwardLegacy.Periods, item.period)
			if period == nil {
				t.Fatalf("missing %s allocation period", item.period)
			}
			edges = period.Edges
		}
		edge := energyExplanationEdgeByIDs(edges, item.from, item.to)
		if edge == nil || edge.Relation != "allocation" || edge.RuleID != energyRelationshipRuleAllocatedServicePathLoad || edge.Value != item.want || edge.ZoneName == "" {
			t.Errorf("%s exact-path allocation %s -> %s = %#v, want %.3f", item.period, item.from, item.to, edge, item.want)
			continue
		}
		wantPath := "path." + strings.ToLower(edge.ZoneName) + ".cooling"
		if !reflect.DeepEqual(edge.RelatedPathIDs, []string{wantPath}) {
			t.Errorf("%s allocation path trace = %#v, want exact target path %q", item.period, edge.RelatedPathIDs, wantPath)
		}
		if !stringSliceContains(edge.SourceIDs, "meter.cooling.electricity") || !stringSliceContains(edge.SourceIDs, "load."+strings.ToLower(edge.ZoneName)+".cooling") ||
			stringSliceContains(edge.SourceIDs, "load.rogue.cooling") || stringSliceContains(edge.SourceIDs, "load.cross.heating") ||
			(strings.EqualFold(edge.ZoneName, "Office") && stringSliceContains(edge.SourceIDs, "load.lab.cooling")) ||
			(strings.EqualFold(edge.ZoneName, "Lab") && stringSliceContains(edge.SourceIDs, "load.office.cooling")) {
			t.Errorf("%s allocation provenance is contaminated: %#v", item.period, edge.SourceIDs)
		}
	}
	for _, target := range []string{"load.cooling.rogue", "load.heating.cross"} {
		if edge := energyExplanationEdgeByIDs(forwardLegacy.Edges, "energy.end_use.cooling.electricity", target); edge != nil && edge.Relation == "allocation" {
			t.Errorf("unrelated/cross-service target entered the exact-path denominator: %#v", edge)
		}
	}

	forward := UpgradeEnergyExplanationV1(forwardLegacy)
	reverse := UpgradeEnergyExplanationV1(reverseLegacy)
	if got, want := epath100AuditAllocationSnapshot(forward), epath100AuditAllocationSnapshot(reverse); !reflect.DeepEqual(got, want) {
		t.Fatalf("service-path projection depends on node/edge/period/provenance order:\nforward=%#v\nreverse=%#v", got, want)
	}
	if got := epath100AuditPeriodIDs(forward.Periods); !reflect.DeepEqual(got, []string{"annual", "M1", "M2"}) {
		t.Errorf("period order = %#v, want annual/M1/M2", got)
	}

	wants := map[string]float64{"Office": 55, "Lab": 45}
	zoneTotal := 0.0
	for zoneName, want := range wants {
		zone := epath100AuditZoneResult(forward.ZoneResults, zoneName)
		if zone == nil {
			t.Fatalf("missing %s zone result: %#v", zoneName, forward.AvailableZones)
		}
		token := metricID(zoneName)
		endUse := epath094AuditNode(zone.Nodes, "end_use.cooling."+token)
		carrier := epath094AuditNode(zone.Nodes, "carrier.electricity."+token)
		link := epath094AuditLink(zone.Links, "load.cooling."+token, "end_use.cooling."+token)
		carrierLink := epath094AuditLink(zone.Links, "end_use.cooling."+token, "carrier.electricity."+token)
		if endUse == nil || endUse.Value != want || endUse.Basis != "service_path_allocation" ||
			carrier == nil || carrier.Value != want || link == nil || link.ToValue != want || link.Basis != "service_path_allocation" ||
			carrierLink == nil || carrierLink.FromValue != want || carrierLink.Basis != "service_path_allocation" {
			t.Errorf("%s service-path v2 graph = endUse %#v carrier %#v load link %#v carrier link %#v", zoneName, endUse, carrier, link, carrierLink)
		}
		wantPath := []string{"path." + strings.ToLower(zoneName) + ".cooling"}
		if link != nil && !reflect.DeepEqual(link.RelatedPathIDs, wantPath) {
			t.Errorf("%s load-to-end-use trace = %#v, want selected path %#v", zoneName, link.RelatedPathIDs, wantPath)
		}
		if carrierLink != nil && !reflect.DeepEqual(carrierLink.RelatedPathIDs, wantPath) {
			t.Errorf("%s end-use-to-carrier trace = %#v, want selected path %#v", zoneName, carrierLink.RelatedPathIDs, wantPath)
		}
		siblingSource := "load.office.cooling"
		if strings.EqualFold(zoneName, "Office") {
			siblingSource = "load.lab.cooling"
		}
		if link != nil && stringSliceContains(link.SourceIDs, siblingSource) {
			t.Errorf("%s service-path link contains sibling-zone source %q: %#v", zoneName, siblingSource, link.SourceIDs)
		}
		if carrierLink != nil && stringSliceContains(carrierLink.SourceIDs, siblingSource) {
			t.Errorf("%s carrier link contains sibling-zone source %q: %#v", zoneName, siblingSource, carrierLink.SourceIDs)
		}
		if got := epath100AuditMonthlyNodeSum(zone.Periods, "end_use.cooling."+token); got != want {
			t.Errorf("%s annual allocation %.3f != monthly sum %.3f", zoneName, want, got)
		}
		if endUse != nil {
			zoneTotal += endUse.Value
		}
		epath100AuditAssertNoBuildingAllocationQuality(t, *zone)
	}
	buildingEndUse := epath094AuditNode(forward.Nodes, "end_use.cooling.building")
	buildingCarrier := epath094AuditNode(forward.Nodes, "carrier.electricity.building")
	if buildingEndUse == nil || buildingEndUse.Value != 100 || buildingCarrier == nil || buildingCarrier.Value != 100 || zoneTotal != buildingEndUse.Value {
		t.Errorf("exact-path allocation changed Building totals or failed zone closure: end-use %#v carrier %#v zone sum %.3f", buildingEndUse, buildingCarrier, zoneTotal)
	}
	for _, zoneName := range []string{"Rogue", "Cross"} {
		zone := epath100AuditZoneResult(forward.ZoneResults, zoneName)
		if zone != nil && epath094AuditNode(zone.Nodes, "end_use.cooling."+metricID(zoneName)) != nil {
			t.Errorf("%s received unrelated/cross-service cooling allocation: %#v", zoneName, zone.Nodes)
		}
	}

	row := epath100AuditAllocationReconciliation(forward.Reconciliation, "cooling", "electricity", "annual")
	if row == nil || row.Level != "allocation" || row.Basis != "service_path_allocation" || row.ExpectedValue != 100 ||
		row.ExplainedValue != 100 || row.ResidualValue != 0 || row.Status != "balanced" || row.Label == "Unassigned building HVAC energy" {
		t.Errorf("balanced exact-path reconciliation = %#v", row)
	}
	if epath100AuditHasWarning(forward.Warnings, "unassigned_building_hvac_energy") {
		t.Errorf("balanced exact-path allocation emitted unassigned warning: %#v", forward.Warnings)
	}
	for periodID, expected := range map[string]float64{"M1": 40, "M2": 60} {
		period := energyExplanationPeriodByID(forward.Periods, periodID)
		if period == nil {
			t.Errorf("missing %s Building period accounting", periodID)
			continue
		}
		periodRow := epath100AuditAllocationReconciliation(period.Reconciliation, "cooling", "electricity", periodID)
		if periodRow == nil || periodRow.Level != "allocation" || periodRow.Basis != "service_path_allocation" || periodRow.ExpectedValue != expected || periodRow.ExplainedValue != expected || periodRow.ResidualValue != 0 || periodRow.Status != "balanced" {
			t.Errorf("%s allocation reconciliation = %#v", periodID, periodRow)
		}
	}
	allocationSource := energyExplanationSourceByID(forward.Sources, "meter.cooling.electricity")
	epath100AuditAssertScopeDetail(t, allocationSource, "Office", true, 0.55, 55)
	epath100AuditAssertScopeDetail(t, allocationSource, "Lab", true, 0.45, 45)
	zoneInput := forwardLegacy
	zoneInput.scope = EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"}
	zoneResult := UpgradeEnergyExplanationV1(zoneInput)
	zoneSource := energyExplanationSourceByID(zoneResult.Sources, "meter.cooling.electricity")
	if zoneSource == nil || !zoneSource.AllocationApplied || !epath100AuditNearlyEqual(zoneSource.AllocationFactor, 0.55) || !epath100AuditNearlyEqual(zoneSource.AllocatedValue, 55) ||
		!strings.Contains(strings.ToLower(zoneSource.AllocationExplanation), "service") || zoneSource.AllocationExplanation == energyDriverAllocationExplanation {
		t.Errorf("monthly-first selected-zone source provenance = %#v", zoneSource)
	}
}

func epath100AuditExactPathFixture(reverse bool) EnergyExplanationV1 {
	graph := func(period string, meter float64, office float64, lab float64) ([]EnergyExplanationNode, []EnergyExplanationEdge) {
		nodes := []EnergyExplanationNode{
			epath100AuditCarrierNode("electricity", meter, period, "facility.electricity"),
			epath100AuditEndUseNode("cooling", "electricity", meter, period, "meter.cooling.electricity", []string{"path.office.cooling", "path.lab.cooling"}),
			epath100AuditLoadNode("Office", "cooling", office, period, "load.office.cooling", []string{"path.office.cooling"}),
			epath100AuditLoadNode("Lab", "cooling", lab, period, "load.lab.cooling", []string{"path.lab.cooling"}),
			epath100AuditLoadNode("Rogue", "cooling", 96, period, "load.rogue.cooling", []string{"path.rogue.cooling"}),
			epath100AuditLoadNode("Cross", "heating", 1000, period, "load.cross.heating", []string{"path.office.cooling"}),
		}
		edges := []EnergyExplanationEdge{
			epath100AuditMeterEdge(period, nodes[0], nodes[1]),
			epath100AuditDeliveredEdge(period, nodes[1], nodes[2], "cooling", []string{"path.office.cooling"}),
			epath100AuditDeliveredEdge(period, nodes[1], nodes[3], "cooling", []string{"path.lab.cooling"}),
			epath100AuditDeliveredEdge(period, nodes[1], nodes[4], "cooling", []string{"path.rogue.cooling"}),
			// Edge metadata lies about cooling, but the target is an exact heating
			// load. Endpoint service identity must win over this poisoned hint.
			epath100AuditDeliveredEdge(period, nodes[1], nodes[5], "cooling", []string{"path.office.cooling"}),
		}
		if reverse {
			epath100AuditReverse(nodes)
			epath100AuditReverse(edges)
			for index := range nodes {
				epath100AuditReverse(nodes[index].SourceIDs)
				epath100AuditReverse(nodes[index].RelatedPathIDs)
			}
			for index := range edges {
				epath100AuditReverse(edges[index].SourceIDs)
				epath100AuditReverse(edges[index].RelatedPathIDs)
			}
		}
		return nodes, edges
	}
	annualNodes, annualEdges := graph("annual", 100, 4, 4)
	m1Nodes, m1Edges := graph("M1", 40, 1, 3)
	m2Nodes, m2Edges := graph("M2", 60, 3, 1)
	periods := []EnergyPeriod{
		{ID: "annual", Label: "Annual", Kind: "annual", Nodes: annualNodes, Edges: annualEdges},
		{ID: "M1", Label: "M1", Kind: "monthly", Nodes: m1Nodes, Edges: m1Edges},
		{ID: "M2", Label: "M2", Kind: "monthly", Nodes: m2Nodes, Edges: m2Edges},
	}
	sources := []EnergyDataSource{
		{ID: "facility.electricity", SourceType: "sql_meter", Name: "Electricity:Facility"},
		{ID: "meter.cooling.electricity", SourceType: "sql_meter", Name: "Cooling:Electricity"},
		{ID: "load.office.cooling", SourceType: "sql_variable", ZoneName: "Office", RelatedEntityIDs: []string{"path.office.cooling", "zone.office"}},
		{ID: "load.lab.cooling", SourceType: "sql_variable", ZoneName: "Lab", RelatedEntityIDs: []string{"path.lab.cooling", "zone.lab"}},
		{ID: "load.rogue.cooling", SourceType: "sql_variable", ZoneName: "Rogue"},
		{ID: "load.cross.heating", SourceType: "sql_variable", ZoneName: "Cross"},
	}
	if reverse {
		epath100AuditReverse(periods)
		epath100AuditReverse(sources)
		for index := range sources {
			epath100AuditReverse(sources[index].RelatedEntityIDs)
		}
	}
	return EnergyExplanationV1{
		Schema:                energyExplanationV1Schema,
		Purpose:               string(SimulationPurposeBasicEnergy),
		Frequency:             "monthly",
		AllocationPolicy:      PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:                 annualNodes,
		Edges:                 annualEdges,
		Periods:               periods,
		Sources:               sources,
		canonicalMonthlyBasis: true,
	}
}

func TestEPATH100AuditSameCarrierKeepsCoolingAndHeatingPathProvenanceSeparate(t *testing.T) {
	nodes := []EnergyExplanationNode{
		epath100AuditCarrierNode("electricity", 150, "annual", "facility.electricity"),
		epath100AuditEndUseNode("cooling", "electricity", 100, "annual", "meter.cooling.electricity", []string{"path.office.cooling", "path.lab.cooling"}),
		epath100AuditEndUseNode("heating", "electricity", 50, "annual", "meter.heating.electricity", []string{"path.office.heating", "path.lab.heating"}),
		epath100AuditLoadNode("Office", "cooling", 25, "annual", "load.office.cooling", []string{"path.office.cooling"}),
		epath100AuditLoadNode("Lab", "cooling", 75, "annual", "load.lab.cooling", []string{"path.lab.cooling"}),
		epath100AuditLoadNode("Office", "heating", 40, "annual", "load.office.heating", []string{"path.office.heating"}),
		epath100AuditLoadNode("Lab", "heating", 10, "annual", "load.lab.heating", []string{"path.lab.heating"}),
	}
	input := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:            nodes,
		Edges: []EnergyExplanationEdge{
			epath100AuditMeterEdge("annual", nodes[0], nodes[1]),
			epath100AuditMeterEdge("annual", nodes[0], nodes[2]),
			epath100AuditDeliveredEdge("annual", nodes[1], nodes[3], "cooling", []string{"path.office.cooling"}),
			epath100AuditDeliveredEdge("annual", nodes[1], nodes[4], "cooling", []string{"path.lab.cooling"}),
			epath100AuditDeliveredEdge("annual", nodes[2], nodes[5], "heating", []string{"path.office.heating"}),
			epath100AuditDeliveredEdge("annual", nodes[2], nodes[6], "heating", []string{"path.lab.heating"}),
		},
		Sources: []EnergyDataSource{
			{ID: "facility.electricity", SourceType: "sql_meter"}, {ID: "meter.cooling.electricity", SourceType: "sql_meter"}, {ID: "meter.heating.electricity", SourceType: "sql_meter"},
			{ID: "load.office.cooling", SourceType: "sql_variable", ZoneName: "Office"}, {ID: "load.lab.cooling", SourceType: "sql_variable", ZoneName: "Lab"},
			{ID: "load.office.heating", SourceType: "sql_variable", ZoneName: "Office"}, {ID: "load.lab.heating", SourceType: "sql_variable", ZoneName: "Lab"},
		},
	}
	result := UpgradeEnergyExplanationV1(applyEnergyExplanationV1ServicePathLoadShareAllocation(input))
	for _, zoneName := range []string{"Office", "Lab"} {
		zone := epath100AuditZoneResult(result.ZoneResults, zoneName)
		if zone == nil {
			t.Fatalf("same-carrier fixture lost %s: %#v", zoneName, result.AvailableZones)
		}
		zoneToken := metricID(zoneName)
		for _, service := range []string{"cooling", "heating"} {
			endUseID := "end_use." + service + "." + zoneToken
			carrierID := "carrier.electricity." + zoneToken
			wantPath := []string{"path." + strings.ToLower(zoneName) + "." + service}
			otherService := "heating"
			if service == "heating" {
				otherService = "cooling"
			}
			loadLink := epath094AuditLink(zone.Links, "load."+service+"."+zoneToken, endUseID)
			carrierLink := epath094AuditLink(zone.Links, endUseID, carrierID)
			for relation, link := range map[string]*EnergyPathLink{"load_to_end_use": loadLink, "end_use_to_carrier": carrierLink} {
				if link == nil || link.Basis != "service_path_allocation" || !reflect.DeepEqual(link.RelatedPathIDs, wantPath) ||
					stringSliceContains(link.SourceIDs, "meter."+otherService+".electricity") || stringSliceContains(link.SourceIDs, "load."+strings.ToLower(zoneName)+"."+otherService) {
					t.Errorf("%s %s %s provenance crossed HVAC services: %#v, want path %#v", zoneName, service, relation, link, wantPath)
				}
			}
		}
	}
}

func TestEPATH100AuditDirectPriorityIsCarrierQualifiedAndDoesNotRenormalizeRemainder(t *testing.T) {
	input := epath100AuditDirectPriorityFixture(false)
	allocated := applyEnergyExplanationV1ServicePathLoadShareAllocation(input)

	checks := []struct {
		from string
		to   string
		want float64
	}{
		{"energy.end_use.heating.electricity", "load.heating.lab", 70},
		{"energy.end_use.heating.natural_gas", "load.heating.office", 10},
		{"energy.end_use.heating.natural_gas", "load.heating.lab", 40},
	}
	for _, check := range checks {
		edge := energyExplanationEdgeByIDs(allocated.Edges, check.from, check.to)
		if edge == nil || edge.Relation != "allocation" || edge.RuleID != energyRelationshipRuleAllocatedServicePathLoad || edge.Value != check.want {
			t.Errorf("carrier-qualified allocation %s -> %s = %#v, want %.3f", check.from, check.to, edge, check.want)
		}
	}
	if edge := energyExplanationEdgeByIDs(allocated.Edges, "energy.end_use.heating.electricity", "load.heating.office"); edge != nil && edge.Relation == "allocation" {
		t.Errorf("direct Office electricity received an additional allocation: %#v", edge)
	}

	result := UpgradeEnergyExplanationV1(allocated)
	office := epath100AuditZoneResult(result.ZoneResults, "Office")
	lab := epath100AuditZoneResult(result.ZoneResults, "Lab")
	if office == nil || lab == nil {
		t.Fatalf("direct-priority zone results missing: %#v", result.AvailableZones)
	}
	officeHeating := epath094AuditNode(office.Nodes, "end_use.heating.office")
	officeElectric := epath094AuditLink(office.Links, "end_use.heating.office", "carrier.electricity.office")
	officeGas := epath094AuditLink(office.Links, "end_use.heating.office", "carrier.natural_gas.office")
	if officeHeating == nil || officeHeating.Value != 40 || officeHeating.Basis != "direct_zone_energy" ||
		officeElectric == nil || officeElectric.FromValue != 30 || officeElectric.Basis != "direct_zone_energy" ||
		officeGas == nil || officeGas.FromValue != 10 || officeGas.Basis != "service_path_allocation" {
		t.Errorf("Office carrier-qualified direct priority = node %#v electric %#v gas %#v", officeHeating, officeElectric, officeGas)
	}
	officeConversion := epath094AuditLink(office.Links, "load.heating.office", "end_use.heating.office")
	if officeConversion == nil || officeConversion.FromValue != 20 || officeConversion.ToValue != 40 || officeConversion.Basis != "direct_zone_energy" ||
		!stringSliceContains(officeConversion.SourceIDs, "load.office.heating") || !stringSliceContains(officeConversion.SourceIDs, "direct.office.heating.electricity") || !stringSliceContains(officeConversion.SourceIDs, "meter.heating.gas") || stringSliceContains(officeConversion.SourceIDs, "load.lab.heating") {
		t.Errorf("Office mixed direct+allocated conversion must keep the physical 20 kWh load endpoint and direct precedence: %#v", officeConversion)
	}
	if officeGas != nil && (!reflect.DeepEqual(officeGas.RelatedPathIDs, []string{"path.office.heating"}) || stringSliceContains(officeGas.SourceIDs, "load.lab.heating")) {
		t.Errorf("Office allocated gas carrier link leaked sibling path/source provenance: %#v", officeGas)
	}
	if officeHeating != nil && (!stringSliceContains(officeHeating.SourceIDs, "load.office.heating") || !stringSliceContains(officeHeating.SourceIDs, "direct.office.heating.electricity") || !stringSliceContains(officeHeating.SourceIDs, "meter.heating.gas")) {
		t.Errorf("Office mixed end-use source trace is incomplete: %#v", officeHeating.SourceIDs)
	}
	labHeating := epath094AuditNode(lab.Nodes, "end_use.heating.lab")
	if labHeating == nil || labHeating.Value != 110 || labHeating.Basis != "service_path_allocation" {
		t.Errorf("Lab allocated heating node = %#v, want 70 electricity + 40 gas", labHeating)
	}
	for carrier, want := range map[string]float64{
		"electricity": 70,
		"natural_gas": 40,
	} {
		id := "carrier." + carrier + ".lab"
		if node := epath094AuditNode(lab.Nodes, id); node == nil || node.Value != want || node.Basis != "service_path_allocation" {
			t.Errorf("Lab allocated carrier %q = %#v, want %.3f", id, node, want)
		}
		carrierLink := epath094AuditLink(lab.Links, "end_use.heating.lab", id)
		if carrierLink == nil || carrierLink.Basis != "service_path_allocation" || !reflect.DeepEqual(carrierLink.RelatedPathIDs, []string{"path.lab.heating"}) || stringSliceContains(carrierLink.SourceIDs, "load.office.heating") {
			t.Errorf("Lab %s carrier link path/source provenance = %#v", carrier, carrierLink)
		}
	}
	if conversion := epath094AuditLink(lab.Links, "load.heating.lab", "end_use.heating.lab"); conversion == nil || conversion.FromValue != 80 || conversion.ToValue != 110 || conversion.Basis != "service_path_allocation" || stringSliceContains(conversion.SourceIDs, "load.office.heating") {
		t.Errorf("multi-carrier allocated conversion duplicated its load endpoint: %#v", conversion)
	}

	for carrier, expected := range map[string]float64{"electricity": 100, "natural_gas": 50} {
		row := epath100AuditAllocationReconciliation(result.Reconciliation, "heating", carrier, "annual")
		if row == nil || row.Level != "allocation" || row.Basis != "service_path_allocation" || row.ExpectedValue != expected || row.ExplainedValue != expected || row.ResidualValue != 0 || row.Status != "balanced" || !stringSliceContains(row.SourceIDs, "meter.heating."+map[string]string{"electricity": "electricity", "natural_gas": "gas"}[carrier]) {
			t.Errorf("%s direct+allocated reconciliation = %#v", carrier, row)
		}
	}
	officeElectricNode := epath094AuditNode(office.Nodes, "carrier.electricity.office")
	labElectricNode := epath094AuditNode(lab.Nodes, "carrier.electricity.lab")
	if officeElectricNode == nil || labElectricNode == nil || officeElectricNode.Value+labElectricNode.Value != 100 {
		t.Error("direct + allocated electricity does not close to the Building end use")
	}
	officeGasNode := epath094AuditNode(office.Nodes, "carrier.natural_gas.office")
	labGasNode := epath094AuditNode(lab.Nodes, "carrier.natural_gas.lab")
	if officeGasNode == nil || labGasNode == nil || officeGasNode.Value+labGasNode.Value != 50 {
		t.Error("carrier-qualified natural gas allocations do not close to the Building end use")
	}
	buildingHeating := epath094AuditNode(result.Nodes, "end_use.heating.building")
	buildingElectric := epath094AuditNode(result.Nodes, "carrier.electricity.building")
	buildingGas := epath094AuditNode(result.Nodes, "carrier.natural_gas.building")
	zoneHeatingSum := 0.0
	if officeHeating != nil {
		zoneHeatingSum += officeHeating.Value
	}
	if labHeating != nil {
		zoneHeatingSum += labHeating.Value
	}
	if buildingHeating == nil || buildingHeating.Value != 150 || buildingElectric == nil || buildingElectric.Value != 100 || buildingGas == nil || buildingGas.Value != 50 || zoneHeatingSum != buildingHeating.Value {
		t.Errorf("direct-first projection changed Building totals or failed zone closure: heating %#v electricity %#v gas %#v zone sum %.3f", buildingHeating, buildingElectric, buildingGas, zoneHeatingSum)
	}
	if office != nil {
		epath100AuditAssertNoBuildingAllocationQuality(t, *office)
	}
	epath100AuditAssertNoBuildingAllocationQuality(t, *lab)

	electricSource := energyExplanationSourceByID(result.Sources, "meter.heating.electricity")
	gasSource := energyExplanationSourceByID(result.Sources, "meter.heating.gas")
	directSource := energyExplanationSourceByID(result.Sources, "direct.office.heating.electricity")
	epath100AuditAssertScopeDetail(t, electricSource, "Lab", true, 0.7, 70)
	epath100AuditAssertScopeDetail(t, gasSource, "Office", true, 0.2, 10)
	epath100AuditAssertScopeDetail(t, gasSource, "Lab", true, 0.8, 40)
	epath100AuditAssertScopeDetail(t, directSource, "Office", false, 1, 30)

	zoneInput := allocated
	zoneInput.scope = EnergyExplanationScope{Kind: "zone", ZoneName: "Lab", AggregationBasis: "model_total"}
	zone := UpgradeEnergyExplanationV1(zoneInput)
	zoneGasSource := energyExplanationSourceByID(zone.Sources, "meter.heating.gas")
	if zoneGasSource == nil || !zoneGasSource.AllocationApplied || zoneGasSource.AllocationFactor != 0.8 || zoneGasSource.AllocatedValue != 40 ||
		!strings.Contains(strings.ToLower(zoneGasSource.AllocationExplanation), "service") || zoneGasSource.AllocationExplanation == energyDriverAllocationExplanation {
		t.Errorf("allocated HVAC source explanation/accounting = %#v", zoneGasSource)
	}
	officeInput := allocated
	officeInput.scope = EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"}
	officeOnly := UpgradeEnergyExplanationV1(officeInput)
	officeDirectSource := energyExplanationSourceByID(officeOnly.Sources, "direct.office.heating.electricity")
	if officeDirectSource == nil || officeDirectSource.AllocationApplied || officeDirectSource.AllocationFactor != 1 || officeDirectSource.AllocatedValue != 30 || officeDirectSource.AggregationBasis != "model_total" || officeDirectSource.AllocationExplanation == energyDriverAllocationExplanation {
		t.Errorf("selected direct source lost direct accounting/provenance: %#v", officeDirectSource)
	}

	reversed := UpgradeEnergyExplanationV1(applyEnergyExplanationV1ServicePathLoadShareAllocation(epath100AuditDirectPriorityFixture(true)))
	if got, want := epath100AuditAllocationSnapshot(result), epath100AuditAllocationSnapshot(reversed); !reflect.DeepEqual(got, want) {
		t.Fatalf("direct-first carrier allocation depends on source/node/edge order:\nforward=%#v\nreverse=%#v", got, want)
	}
}

func epath100AuditDirectPriorityFixture(reverse bool) EnergyExplanationV1 {
	nodes := []EnergyExplanationNode{
		epath100AuditCarrierNode("electricity", 100, "annual", "facility.electricity"),
		epath100AuditCarrierNode("natural_gas", 50, "annual", "facility.gas"),
		epath100AuditEndUseNode("heating", "electricity", 100, "annual", "meter.heating.electricity", []string{"path.office.heating", "path.lab.heating"}),
		epath100AuditEndUseNode("heating", "natural_gas", 50, "annual", "meter.heating.gas", []string{"path.office.heating", "path.lab.heating"}),
		epath100AuditLoadNode("Office", "heating", 20, "annual", "load.office.heating", []string{"path.office.heating"}),
		epath100AuditLoadNode("Lab", "heating", 80, "annual", "load.lab.heating", []string{"path.lab.heating"}),
	}
	edges := []EnergyExplanationEdge{
		epath100AuditMeterEdge("annual", nodes[0], nodes[2]),
		epath100AuditMeterEdge("annual", nodes[1], nodes[3]),
		epath100AuditDeliveredEdge("annual", nodes[2], nodes[4], "heating", []string{"path.office.heating"}),
		epath100AuditDeliveredEdge("annual", nodes[2], nodes[5], "heating", []string{"path.lab.heating"}),
		epath100AuditDeliveredEdge("annual", nodes[3], nodes[4], "heating", []string{"path.office.heating"}),
		epath100AuditDeliveredEdge("annual", nodes[3], nodes[5], "heating", []string{"path.lab.heating"}),
	}
	direct := epath094AuditDirectSeries("Office", "heating", "electricity", 30, "direct.office.heating.electricity")
	direct.ServiceKind = "heating"
	direct.RelatedEntityIDs = []string{"component.office.heating", "zone.office"}
	result := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:            nodes,
		Edges:            edges,
		Sources: []EnergyDataSource{
			{ID: "facility.electricity", SourceType: "sql_meter"},
			{ID: "facility.gas", SourceType: "sql_meter"},
			{ID: "meter.heating.electricity", SourceType: "sql_meter"},
			{ID: "meter.heating.gas", SourceType: "sql_meter"},
			{ID: "load.office.heating", SourceType: "sql_variable", ZoneName: "Office", RelatedEntityIDs: []string{"path.office.heating", "zone.office"}},
			{ID: "load.lab.heating", SourceType: "sql_variable", ZoneName: "Lab", RelatedEntityIDs: []string{"path.lab.heating", "zone.lab"}},
			{ID: "direct.office.heating.electricity", SourceType: "sql_variable", ZoneName: "Office", RelatedEntityIDs: []string{"component.office.heating", "zone.office"}},
		},
		zoneDirectUseSeries: []energyExplanationSeries{direct},
	}
	if reverse {
		epath100AuditReverse(result.Nodes)
		epath100AuditReverse(result.Edges)
		epath100AuditReverse(result.Sources)
		epath100AuditReverse(result.zoneDirectUseSeries)
		for index := range result.Nodes {
			epath100AuditReverse(result.Nodes[index].SourceIDs)
			epath100AuditReverse(result.Nodes[index].RelatedPathIDs)
		}
		for index := range result.Edges {
			epath100AuditReverse(result.Edges[index].SourceIDs)
			epath100AuditReverse(result.Edges[index].RelatedPathIDs)
		}
		for index := range result.Sources {
			epath100AuditReverse(result.Sources[index].RelatedEntityIDs)
		}
		for index := range result.zoneDirectUseSeries {
			epath100AuditReverse(result.zoneDirectUseSeries[index].SourceIDs)
			epath100AuditReverse(result.zoneDirectUseSeries[index].RelatedEntityIDs)
		}
	}
	return result
}

func TestEPATH100AuditFallsBackOnlyToMatchingZoneServiceLoad(t *testing.T) {
	nodes := []EnergyExplanationNode{
		epath100AuditCarrierNode("natural_gas", 50, "annual", "facility.gas"),
		epath100AuditEndUseNode("heating", "natural_gas", 50, "annual", "meter.heating.gas", nil),
		epath100AuditLoadNode("Office", "heating", 10, "annual", "load.office.heating", nil),
		epath100AuditLoadNode("Lab", "heating", 40, "annual", "load.lab.heating", nil),
		epath100AuditLoadNode("ColdRoom", "cooling", 950, "annual", "load.cold.cooling", nil),
	}
	input := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:            nodes,
		Edges: []EnergyExplanationEdge{
			epath100AuditMeterEdge("annual", nodes[0], nodes[1]),
			epath100AuditDeliveredEdge("annual", nodes[1], nodes[2], "heating", nil),
			epath100AuditDeliveredEdge("annual", nodes[1], nodes[3], "heating", nil),
			// A malformed heating edge aimed at a cooling load must not dilute
			// the matching-service fallback denominator.
			epath100AuditDeliveredEdge("annual", nodes[1], nodes[4], "heating", nil),
		},
		Sources: []EnergyDataSource{{ID: "facility.gas"}, {ID: "meter.heating.gas"}, {ID: "load.office.heating", ZoneName: "Office"}, {ID: "load.lab.heating", ZoneName: "Lab"}, {ID: "load.cold.cooling", ZoneName: "ColdRoom"}},
	}
	allocated := applyEnergyExplanationV1ServicePathLoadShareAllocation(input)
	for _, check := range []struct {
		zone string
		want float64
	}{{"Office", 10}, {"Lab", 40}} {
		edge := energyExplanationEdgeByIDs(allocated.Edges, "energy.end_use.heating.natural_gas", "load.heating."+strings.ToLower(check.zone))
		if edge == nil || edge.Relation != "allocation" || edge.RuleID != energyRelationshipRuleAllocatedZoneLoad || edge.Value != check.want {
			t.Errorf("%s fallback allocation = %#v, want %.3f", check.zone, edge, check.want)
		}
	}
	if edge := energyExplanationEdgeByIDs(allocated.Edges, "energy.end_use.heating.natural_gas", "load.cooling.coldroom"); edge != nil && edge.Relation == "allocation" {
		t.Errorf("cross-service cooling load entered heating fallback: %#v", edge)
	}
	result := UpgradeEnergyExplanationV1(allocated)
	zoneTotal := 0.0
	for _, check := range []struct {
		zone string
		want float64
	}{{"Office", 10}, {"Lab", 40}} {
		zone := epath100AuditZoneResult(result.ZoneResults, check.zone)
		if zone == nil {
			t.Fatalf("missing fallback zone %s", check.zone)
		}
		node := epath094AuditNode(zone.Nodes, "end_use.heating."+metricID(check.zone))
		link := epath094AuditLink(zone.Links, "load.heating."+metricID(check.zone), "end_use.heating."+metricID(check.zone))
		if node == nil || node.Value != check.want || node.Basis != "zone_load_allocation" || link == nil || link.Basis != "zone_load_allocation" {
			t.Errorf("%s fallback v2 basis/value = node %#v link %#v", check.zone, node, link)
		}
		if node != nil {
			zoneTotal += node.Value
		}
		epath100AuditAssertNoBuildingAllocationQuality(t, *zone)
	}
	row := epath100AuditAllocationReconciliation(result.Reconciliation, "heating", "natural_gas", "annual")
	if row == nil || row.Level != "allocation" || row.Basis != "service_path_allocation" || row.ExpectedValue != 50 || row.ExplainedValue != 50 || row.ResidualValue != 0 || row.Status != "balanced" || !stringSliceContains(row.SourceIDs, "meter.heating.gas") {
		t.Errorf("fallback allocation reconciliation = %#v", row)
	}
	buildingEndUse := epath094AuditNode(result.Nodes, "end_use.heating.building")
	buildingCarrier := epath094AuditNode(result.Nodes, "carrier.natural_gas.building")
	if buildingEndUse == nil || buildingEndUse.Value != 50 || buildingCarrier == nil || buildingCarrier.Value != 50 || zoneTotal != 50 {
		t.Errorf("fallback allocation changed Building totals or failed zone closure: end-use %#v carrier %#v zone sum %.3f", buildingEndUse, buildingCarrier, zoneTotal)
	}
	zoneInput := allocated
	zoneInput.scope = EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"}
	zoneResult := UpgradeEnergyExplanationV1(zoneInput)
	zoneSource := energyExplanationSourceByID(zoneResult.Sources, "meter.heating.gas")
	if zoneSource == nil || !zoneSource.AllocationApplied || zoneSource.AllocationFactor != 0.2 || zoneSource.AllocatedValue != 10 ||
		!strings.Contains(strings.ToLower(zoneSource.AllocationExplanation), "zone") || !strings.Contains(strings.ToLower(zoneSource.AllocationExplanation), "load") || zoneSource.AllocationExplanation == energyDriverAllocationExplanation {
		t.Errorf("fallback selected-zone source provenance = %#v", zoneSource)
	}
}

func TestEPATH100AuditRuntimeGraphAllocatesEveryHeatingCarrier(t *testing.T) {
	monthly := func(value float64) map[int]float64 { return map[int]float64{1: value} }
	series := []energyExplanationSeries{
		{Stage: "carrier", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Unit: "kWh", Carrier: "electricity", MeterHierarchyLevel: "facility_total", Basis: "measured_meter", SourceIDs: []string{"facility.electricity"}, Total: 100, Monthly: monthly(100), sourceName: "Electricity:Facility", sourceFrequency: "Monthly"},
		{Stage: "carrier", Level: "energy", Kind: "energy.natural_gas.total", Label: "Natural gas", Unit: "kWh", Carrier: "natural_gas", MeterHierarchyLevel: "facility_total", Basis: "measured_meter", SourceIDs: []string{"facility.gas"}, Total: 50, Monthly: monthly(50), sourceName: "NaturalGas:Facility", sourceFrequency: "Monthly"},
		{Stage: "end_use", Level: "energy", Kind: "energy.heating", Label: "Heating electricity", Unit: "kWh", Carrier: "electricity", EndUse: "heating", ServiceKind: "heating", MeterHierarchyLevel: "broad_end_use", Basis: "measured_meter", SourceIDs: []string{"meter.heating.electricity"}, Total: 100, Monthly: monthly(100), sourceName: "Heating:Electricity", sourceFrequency: "Monthly"},
		{Stage: "end_use", Level: "energy", Kind: "energy.heating", Label: "Heating gas", Unit: "kWh", Carrier: "natural_gas", EndUse: "heating", ServiceKind: "heating", MeterHierarchyLevel: "broad_end_use", Basis: "measured_meter", SourceIDs: []string{"meter.heating.gas"}, Total: 50, Monthly: monthly(50), sourceName: "Heating:NaturalGas", sourceFrequency: "Monthly"},
		{Stage: "load", Level: "load", Kind: "load.zone_heating", Label: "Office heating load", Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "heating", PathType: "zone", Basis: "measured_energy_variable", SourceIDs: []string{"load.office.heating"}, Total: 20, Monthly: monthly(20), sourceName: "Zone Air System Sensible Heating Energy", sourceFrequency: "Monthly"},
		{Stage: "load", Level: "load", Kind: "load.zone_heating", Label: "Lab heating load", Unit: "kWh thermal", ZoneName: "Lab", ServiceKind: "heating", PathType: "zone", Basis: "measured_energy_variable", SourceIDs: []string{"load.lab.heating"}, Total: 80, Monthly: monthly(80), sourceName: "Zone Air System Sensible Heating Energy", sourceFrequency: "Monthly"},
	}
	sources := []EnergyDataSource{
		{ID: "facility.electricity", SourceType: "sql_meter"},
		{ID: "facility.gas", SourceType: "sql_meter"},
		{ID: "meter.heating.electricity", SourceType: "sql_meter"},
		{ID: "meter.heating.gas", SourceType: "sql_meter"},
		{ID: "load.office.heating", SourceType: "sql_variable", ZoneName: "Office"},
		{ID: "load.lab.heating", SourceType: "sql_variable", ZoneName: "Lab"},
	}
	plan := &PurposeRunPlan{
		Purposes:          []SimulationPurposeID{SimulationPurposeBasicEnergy},
		BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath,
		AllocationPolicy:  PurposeAllocationPolicyByServicePathLoadShare,
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, sources, plan, energyDriverBuildContext{Enabled: true, Multipliers: unitEnergyMultiplierIndex("Office", "Lab")})
	result := UpgradeEnergyExplanationV1(applyEnergyExplanationV1ServicePathLoadShareAllocation(legacy))

	wants := map[string]map[string]float64{
		"Office": {"electricity": 20, "natural_gas": 10},
		"Lab":    {"electricity": 80, "natural_gas": 40},
	}
	for zoneName, carriers := range wants {
		zone := epath100AuditZoneResult(result.ZoneResults, zoneName)
		if zone == nil {
			t.Fatalf("runtime graph lost %s: %#v", zoneName, result.AvailableZones)
		}
		for carrier, want := range carriers {
			id := "carrier." + carrier + "." + metricID(zoneName)
			node := epath094AuditNode(zone.Nodes, id)
			if node == nil || node.Value != want || node.Basis != "zone_load_allocation" {
				t.Errorf("runtime %s %s carrier allocation = %#v, want %.3f", zoneName, carrier, node, want)
			}
			link := epath094AuditLink(zone.Links, "end_use.heating."+metricID(zoneName), id)
			siblingMeter := "meter.heating.gas"
			if carrier == "natural_gas" {
				siblingMeter = "meter.heating.electricity"
			}
			if link == nil || link.Basis != "zone_load_allocation" || stringSliceContains(link.SourceIDs, siblingMeter) {
				t.Errorf("runtime %s %s ribbon leaked sibling-carrier provenance %q: %#v", zoneName, carrier, siblingMeter, link)
			}
			month := energyExplanationPeriodByID(zone.Periods, "M1")
			if month == nil || epath094AuditNode(month.Nodes, id) == nil || epath094AuditNode(month.Nodes, id).Value != want {
				t.Errorf("runtime %s %s monthly allocation missing: %#v", zoneName, carrier, month)
			}
		}
	}
	for carrier, expected := range map[string]float64{"electricity": 100, "natural_gas": 50} {
		row := epath100AuditAllocationReconciliation(result.Reconciliation, "heating", carrier, "annual")
		if row == nil || row.ExpectedValue != expected || row.ExplainedValue != expected || row.ResidualValue != 0 || row.Status != "balanced" {
			t.Errorf("runtime %s reconciliation = %#v", carrier, row)
		}
	}
}

func TestEPATH100AuditMonthlyAggregationKeepsAnnualOnlyHVACGroup(t *testing.T) {
	annualNodes := []EnergyExplanationNode{
		epath100AuditCarrierNode("electricity", 100, "annual", "facility.electricity"),
		epath100AuditCarrierNode("natural_gas", 50, "annual", "facility.gas"),
		epath100AuditEndUseNode("cooling", "electricity", 100, "annual", "meter.cooling.electricity", []string{"path.office.cooling", "path.lab.cooling"}),
		epath100AuditEndUseNode("heating", "natural_gas", 50, "annual", "meter.heating.gas", nil),
		epath100AuditLoadNode("Office", "cooling", 4, "annual", "load.office.cooling", []string{"path.office.cooling"}),
		epath100AuditLoadNode("Lab", "cooling", 4, "annual", "load.lab.cooling", []string{"path.lab.cooling"}),
		epath100AuditLoadNode("Office", "heating", 10, "annual", "load.office.heating", nil),
		epath100AuditLoadNode("Lab", "heating", 40, "annual", "load.lab.heating", nil),
	}
	annualEdges := []EnergyExplanationEdge{
		epath100AuditMeterEdge("annual", annualNodes[0], annualNodes[2]),
		epath100AuditMeterEdge("annual", annualNodes[1], annualNodes[3]),
		epath100AuditDeliveredEdge("annual", annualNodes[2], annualNodes[4], "cooling", []string{"path.office.cooling"}),
		epath100AuditDeliveredEdge("annual", annualNodes[2], annualNodes[5], "cooling", []string{"path.lab.cooling"}),
		epath100AuditDeliveredEdge("annual", annualNodes[3], annualNodes[6], "heating", nil),
		epath100AuditDeliveredEdge("annual", annualNodes[3], annualNodes[7], "heating", nil),
	}
	monthlyGraph := func(period string, meter float64, office float64, lab float64) ([]EnergyExplanationNode, []EnergyExplanationEdge) {
		nodes := []EnergyExplanationNode{
			epath100AuditCarrierNode("electricity", meter, period, "facility.electricity"),
			epath100AuditEndUseNode("cooling", "electricity", meter, period, "meter.cooling.electricity", []string{"path.office.cooling", "path.lab.cooling"}),
			epath100AuditLoadNode("Office", "cooling", office, period, "load.office.cooling", []string{"path.office.cooling"}),
			epath100AuditLoadNode("Lab", "cooling", lab, period, "load.lab.cooling", []string{"path.lab.cooling"}),
		}
		edges := []EnergyExplanationEdge{
			epath100AuditMeterEdge(period, nodes[0], nodes[1]),
			epath100AuditDeliveredEdge(period, nodes[1], nodes[2], "cooling", []string{"path.office.cooling"}),
			epath100AuditDeliveredEdge(period, nodes[1], nodes[3], "cooling", []string{"path.lab.cooling"}),
		}
		return nodes, edges
	}
	m1Nodes, m1Edges := monthlyGraph("M1", 40, 1, 3)
	m2Nodes, m2Edges := monthlyGraph("M2", 60, 3, 1)
	input := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "monthly",
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:            annualNodes,
		Edges:            annualEdges,
		Periods: []EnergyPeriod{
			{ID: "annual", Label: "Annual", Kind: "annual", Nodes: annualNodes, Edges: annualEdges},
			{ID: "M1", Label: "M1", Kind: "monthly", Nodes: m1Nodes, Edges: m1Edges},
			{ID: "M2", Label: "M2", Kind: "monthly", Nodes: m2Nodes, Edges: m2Edges},
		},
		Sources: []EnergyDataSource{
			{ID: "facility.electricity", SourceType: "sql_meter"}, {ID: "facility.gas", SourceType: "sql_meter"},
			{ID: "meter.cooling.electricity", SourceType: "sql_meter"}, {ID: "meter.heating.gas", SourceType: "sql_meter"},
			{ID: "load.office.cooling", SourceType: "sql_variable", ZoneName: "Office"}, {ID: "load.lab.cooling", SourceType: "sql_variable", ZoneName: "Lab"},
			{ID: "load.office.heating", SourceType: "sql_variable", ZoneName: "Office"}, {ID: "load.lab.heating", SourceType: "sql_variable", ZoneName: "Lab"},
		},
		canonicalMonthlyBasis: true,
	}
	allocated := applyEnergyExplanationV1ServicePathLoadShareAllocation(input)
	result := UpgradeEnergyExplanationV1(allocated)

	for _, check := range []struct {
		zone        string
		cooling     float64
		heating     float64
		heatingLoad float64
	}{{zone: "Office", cooling: 55, heating: 10, heatingLoad: 10}, {zone: "Lab", cooling: 45, heating: 40, heatingLoad: 40}} {
		zone := epath100AuditZoneResult(result.ZoneResults, check.zone)
		if zone == nil {
			t.Fatalf("mixed temporal allocation lost %s: %#v", check.zone, result.AvailableZones)
		}
		zoneToken := metricID(check.zone)
		cooling := epath094AuditNode(zone.Nodes, "end_use.cooling."+zoneToken)
		heating := epath094AuditNode(zone.Nodes, "end_use.heating."+zoneToken)
		heatingLink := epath094AuditLink(zone.Links, "load.heating."+zoneToken, "end_use.heating."+zoneToken)
		if cooling == nil || cooling.Value != check.cooling || cooling.Basis != "service_path_allocation" {
			t.Errorf("%s monthly-derived cooling = %#v, want %.3f", check.zone, cooling, check.cooling)
		}
		if heating == nil || heating.Value != check.heating || heating.Basis != "zone_load_allocation" || heatingLink == nil || heatingLink.FromValue != check.heatingLoad || heatingLink.ToValue != check.heating || heatingLink.Basis != "zone_load_allocation" {
			t.Errorf("%s annual-only heating fallback = node %#v link %#v", check.zone, heating, heatingLink)
		}
		annual := energyExplanationPeriodByID(zone.Periods, "annual")
		if annual == nil || epath094AuditNode(annual.Nodes, "end_use.heating."+zoneToken) == nil || epath094AuditNode(annual.Nodes, "end_use.heating."+zoneToken).Value != check.heating {
			t.Errorf("%s annual period dropped annual-only heating: %#v", check.zone, annual)
		}
	}
	for _, check := range []struct {
		service string
		carrier string
		value   float64
	}{{service: "cooling", carrier: "electricity", value: 100}, {service: "heating", carrier: "natural_gas", value: 50}} {
		row := epath100AuditAllocationReconciliation(result.Reconciliation, check.service, check.carrier, "annual")
		if row == nil || row.ExpectedValue != check.value || row.ExplainedValue != check.value || row.ResidualValue != 0 || row.Status != "balanced" {
			t.Errorf("mixed temporal %s/%s annual reconciliation = %#v", check.service, check.carrier, row)
		}
	}
	officeInput := allocated
	officeInput.scope = EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"}
	office := UpgradeEnergyExplanationV1(officeInput)
	heatingSource := energyExplanationSourceByID(office.Sources, "meter.heating.gas")
	if heatingSource == nil || !heatingSource.AllocationApplied || heatingSource.AllocationFactor != 0.2 || heatingSource.AllocatedValue != 10 ||
		!strings.Contains(strings.ToLower(heatingSource.AllocationExplanation), "zone") || !strings.Contains(strings.ToLower(heatingSource.AllocationExplanation), "load") {
		t.Errorf("annual-only fallback source provenance = %#v", heatingSource)
	}
}

func TestEPATH100AuditAnnualOnlyDirectWinsAnnualWithoutFabricatingMonthlyDirect(t *testing.T) {
	input := epath100AuditExactPathFixture(false)
	direct := epath094AuditDirectSeries("Office", "cooling", "electricity", 30, "direct.office.cooling.annual")
	direct.ServiceKind = "cooling"
	direct.RelatedEntityIDs = []string{"component.office.cooling", "path.office.cooling", "zone.office"}
	input.zoneDirectUseSeries = []energyExplanationSeries{direct}
	input.Sources = append(input.Sources, EnergyDataSource{ID: "direct.office.cooling.annual", SourceType: "sql_variable", ZoneName: "Office"})
	allocated := applyEnergyExplanationV1ServicePathLoadShareAllocation(input)

	if edge := energyExplanationEdgeByIDs(allocated.Edges, "energy.end_use.cooling.electricity", "load.cooling.office"); edge != nil && edge.Relation == "allocation" {
		t.Errorf("annual-only direct Office also received annual allocation: %#v", edge)
	}
	if edge := energyExplanationEdgeByIDs(allocated.Edges, "energy.end_use.cooling.electricity", "load.cooling.lab"); edge == nil || edge.Value != 70 || edge.RuleID != energyRelationshipRuleAllocatedServicePathLoad {
		t.Errorf("annual direct-first remainder was not assigned only to Lab: %#v", edge)
	}
	for periodID, wants := range map[string]map[string]float64{
		"M1": {"Office": 10, "Lab": 30},
		"M2": {"Office": 45, "Lab": 15},
	} {
		period := energyExplanationPeriodByID(allocated.Periods, periodID)
		if period == nil {
			t.Fatalf("missing %s legacy allocation period", periodID)
		}
		for zoneName, want := range wants {
			edge := energyExplanationEdgeByIDs(period.Edges, "energy.end_use.cooling.electricity", "load.cooling."+metricID(zoneName))
			if edge == nil || edge.Value != want || stringSliceContains(edge.SourceIDs, "direct.office.cooling.annual") {
				t.Errorf("%s %s monthly allocation fabricated annual direct evidence: %#v", periodID, zoneName, edge)
			}
		}
	}

	result := UpgradeEnergyExplanationV1(allocated)
	office := epath100AuditZoneResult(result.ZoneResults, "Office")
	lab := epath100AuditZoneResult(result.ZoneResults, "Lab")
	if office == nil || lab == nil {
		t.Fatalf("annual-only direct allocation zones missing: %#v", result.AvailableZones)
	}
	officeAnnual := epath094AuditNode(office.Nodes, "end_use.cooling.office")
	labAnnual := epath094AuditNode(lab.Nodes, "end_use.cooling.lab")
	if officeAnnual == nil || officeAnnual.Value != 30 || officeAnnual.Basis != "direct_zone_energy" || labAnnual == nil || labAnnual.Value != 70 || labAnnual.Basis != "service_path_allocation" {
		t.Errorf("annual direct-first truth = Office %#v / Lab %#v", officeAnnual, labAnnual)
	}
	officeAnnualLink := epath094AuditLink(office.Links, "load.cooling.office", "end_use.cooling.office")
	if officeAnnualLink == nil || officeAnnualLink.Basis != "direct_zone_energy" || officeAnnualLink.ToValue != 30 || !strings.Contains(strings.ToLower(officeAnnualLink.Explanation), "partial temporal coverage") {
		t.Errorf("annual-only direct mismatch lacks honest partial-temporal provenance: %#v", officeAnnualLink)
	}
	monthlyWants := map[string]map[string]float64{
		"Office": {"M1": 10, "M2": 45},
		"Lab":    {"M1": 30, "M2": 15},
	}
	for _, zone := range []*EnergyExplanationZoneResult{office, lab} {
		for periodID, want := range monthlyWants[zone.Scope.ZoneName] {
			period := energyExplanationPeriodByID(zone.Periods, periodID)
			node := (*EnergyExplanationNode)(nil)
			if period != nil {
				node = epath094AuditNode(period.Nodes, "end_use.cooling."+metricID(zone.Scope.ZoneName))
			}
			if node == nil || node.Value != want || node.Basis != "service_path_allocation" || stringSliceContains(node.SourceIDs, "direct.office.cooling.annual") {
				t.Errorf("%s %s projected annual-only direct into monthly result: %#v", zone.Scope.ZoneName, periodID, node)
			}
		}
	}
	for periodID, expected := range map[string]float64{"M1": 40, "M2": 60} {
		period := energyExplanationPeriodByID(result.Periods, periodID)
		if period == nil {
			t.Errorf("missing %s Building allocation period", periodID)
			continue
		}
		periodRow := epath100AuditAllocationReconciliation(period.Reconciliation, "cooling", "electricity", periodID)
		if periodRow == nil || periodRow.ExpectedValue != expected || periodRow.ExplainedValue != expected || periodRow.ResidualValue != 0 || periodRow.Status != "balanced" || stringSliceContains(periodRow.SourceIDs, "direct.office.cooling.annual") {
			t.Errorf("%s allocation incorrectly used annual-only direct source: %#v", periodID, periodRow)
		}
	}
	row := epath100AuditAllocationReconciliation(result.Reconciliation, "cooling", "electricity", "annual")
	if row == nil || row.ExpectedValue != 100 || row.ExplainedValue != 100 || row.ResidualValue != 0 || row.Status != "balanced" ||
		!stringSliceContains(row.SourceIDs, "direct.office.cooling.annual") || !stringSliceContains(row.SourceIDs, "meter.cooling.electricity") {
		t.Errorf("annual-only direct Building reconciliation = %#v", row)
	}
	if officeAnnual != nil && labAnnual != nil && officeAnnual.Value+labAnnual.Value != 100 {
		t.Errorf("annual direct + remainder does not close to Building: %.3f + %.3f", officeAnnual.Value, labAnnual.Value)
	}
}

func TestEPATH100AuditAnnualOnlyNonDirectGroupDoesNotMarkCompleteMonthlyDirectPartial(t *testing.T) {
	input := epath100AuditExactPathFixture(false)
	districtCarrier := epath100AuditCarrierNode("district_cooling", 50, "annual", "facility.district_cooling")
	districtEndUse := epath100AuditEndUseNode("cooling", "district_cooling", 50, "annual", "meter.cooling.district", []string{"path.office.cooling", "path.lab.cooling"})
	input.Nodes = append(input.Nodes, districtCarrier, districtEndUse)
	input.Edges = append(input.Edges,
		epath100AuditMeterEdge("annual", districtCarrier, districtEndUse),
		epath100AuditDeliveredEdge("annual", districtEndUse, *epath094AuditNode(input.Nodes, "load.cooling.office"), "cooling", []string{"path.office.cooling"}),
		epath100AuditDeliveredEdge("annual", districtEndUse, *epath094AuditNode(input.Nodes, "load.cooling.lab"), "cooling", []string{"path.lab.cooling"}),
	)
	annual := energyExplanationPeriodByID(input.Periods, "annual")
	if annual == nil {
		t.Fatal("fixture lost annual period")
	}
	annual.Nodes = append(annual.Nodes, districtCarrier, districtEndUse)
	annual.Edges = append(annual.Edges,
		epath100AuditMeterEdge("annual", districtCarrier, districtEndUse),
		epath100AuditDeliveredEdge("annual", districtEndUse, *epath094AuditNode(annual.Nodes, "load.cooling.office"), "cooling", []string{"path.office.cooling"}),
		epath100AuditDeliveredEdge("annual", districtEndUse, *epath094AuditNode(annual.Nodes, "load.cooling.lab"), "cooling", []string{"path.lab.cooling"}),
	)
	direct := epath094AuditDirectMonthlySeries("Office", "cooling", "electricity", 30, map[int]float64{1: 10, 2: 20}, "direct.office.cooling.monthly")
	direct.ServiceKind = "cooling"
	direct.RelatedEntityIDs = []string{"component.office.cooling", "path.office.cooling", "zone.office"}
	input.zoneDirectUseSeries = []energyExplanationSeries{direct}
	input.Sources = append(input.Sources,
		EnergyDataSource{ID: "facility.district_cooling", SourceType: "sql_meter"},
		EnergyDataSource{ID: "meter.cooling.district", SourceType: "sql_meter"},
		EnergyDataSource{ID: "direct.office.cooling.monthly", SourceType: "sql_variable", ZoneName: "Office"},
	)
	result := UpgradeEnergyExplanationV1(applyEnergyExplanationV1ServicePathLoadShareAllocation(input))
	office := epath100AuditZoneResult(result.ZoneResults, "Office")
	lab := epath100AuditZoneResult(result.ZoneResults, "Lab")
	if office == nil || lab == nil {
		t.Fatalf("mixed temporal same-service zones missing: %#v", result.AvailableZones)
	}
	officeCooling := epath094AuditNode(office.Nodes, "end_use.cooling.office")
	officeElectric := epath094AuditNode(office.Nodes, "carrier.electricity.office")
	officeDistrict := epath094AuditNode(office.Nodes, "carrier.district_cooling.office")
	officeLink := epath094AuditLink(office.Links, "load.cooling.office", "end_use.cooling.office")
	if officeCooling == nil || officeCooling.Value != 55 || officeCooling.Basis != "direct_zone_energy" ||
		officeElectric == nil || officeElectric.Value != 30 || officeElectric.Basis != "direct_zone_energy" ||
		officeDistrict == nil || officeDistrict.Value != 25 || officeDistrict.Basis != "service_path_allocation" ||
		officeLink == nil || officeLink.FromValue != 4 || officeLink.ToValue != 55 || officeLink.Basis != "direct_zone_energy" {
		t.Errorf("complete-monthly direct plus annual-only non-direct branch = end-use %#v electric %#v district %#v link %#v", officeCooling, officeElectric, officeDistrict, officeLink)
	}
	if officeLink != nil && strings.Contains(strings.ToLower(officeLink.Explanation), "partial temporal coverage") {
		t.Errorf("annual-only non-direct sibling falsely marked complete monthly direct as partial: %q", officeLink.Explanation)
	}
	labCooling := epath094AuditNode(lab.Nodes, "end_use.cooling.lab")
	labElectric := epath094AuditNode(lab.Nodes, "carrier.electricity.lab")
	labDistrict := epath094AuditNode(lab.Nodes, "carrier.district_cooling.lab")
	if labCooling == nil || labCooling.Value != 95 || labElectric == nil || labElectric.Value != 70 || labDistrict == nil || labDistrict.Value != 25 {
		t.Errorf("annual-only non-direct group did not coexist with monthly remainder: end-use %#v electric %#v district %#v", labCooling, labElectric, labDistrict)
	}
	for _, check := range []struct {
		carrier string
		value   float64
	}{{carrier: "electricity", value: 100}, {carrier: "district_cooling", value: 50}} {
		row := epath100AuditAllocationReconciliation(result.Reconciliation, "cooling", check.carrier, "annual")
		if row == nil || row.ExpectedValue != check.value || row.ExplainedValue != check.value || row.ResidualValue != 0 || row.Status != "balanced" {
			t.Errorf("same-service mixed temporal %s reconciliation = %#v", check.carrier, row)
		}
	}
}

func TestEPATH100AuditTinyUnassignedRemainderStaysBuildingOnly(t *testing.T) {
	nodes := []EnergyExplanationNode{
		epath100AuditCarrierNode("district_cooling", 25, "annual", "facility.district_cooling"),
		epath100AuditEndUseNode("cooling", "district_cooling", 25, "annual", "meter.cooling.district", []string{"path.office.cooling"}),
		epath100AuditLoadNode("Office", "cooling", 5, "annual", "load.office.cooling", []string{"path.office.cooling"}),
	}
	direct := epath094AuditDirectSeries("Office", "cooling", "district_cooling", 24.999, "direct.office.cooling.district")
	direct.ServiceKind = "cooling"
	input := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:            nodes,
		Edges: []EnergyExplanationEdge{
			epath100AuditMeterEdge("annual", nodes[0], nodes[1]),
			epath100AuditDeliveredEdge("annual", nodes[1], nodes[2], "cooling", []string{"path.office.cooling"}),
		},
		Sources: []EnergyDataSource{
			{ID: "facility.district_cooling", SourceType: "sql_meter"},
			{ID: "meter.cooling.district", SourceType: "sql_meter"},
			{ID: "load.office.cooling", SourceType: "sql_variable", ZoneName: "Office"},
			{ID: "direct.office.cooling.district", SourceType: "sql_variable", ZoneName: "Office"},
		},
		zoneDirectUseSeries: []energyExplanationSeries{direct},
	}
	allocated := applyEnergyExplanationV1ServicePathLoadShareAllocation(input)
	if edge := energyExplanationEdgeByIDs(allocated.Edges, "energy.end_use.cooling.district_cooling", "load.cooling.office"); edge != nil && edge.Relation == "allocation" {
		t.Errorf("direct target received the 0.001 kWh allocation remainder: %#v", edge)
	}
	result := UpgradeEnergyExplanationV1(allocated)
	row := epath100AuditAllocationReconciliation(result.Reconciliation, "cooling", "district_cooling", "annual")
	if row == nil || row.Level != "allocation" || row.Basis != "service_path_allocation" || row.Label != "Unassigned building HVAC energy" ||
		row.ExpectedValue != 25 || row.ExplainedValue != 24.999 || !epath100AuditNearlyEqual(row.ResidualValue, 0.001) || row.Status != "partial" ||
		!stringSliceContains(row.SourceIDs, "meter.cooling.district") || !stringSliceContains(row.SourceIDs, "direct.office.cooling.district") {
		t.Errorf("tiny unassigned Building reconciliation = %#v", row)
	}
	warning := epath100AuditWarning(result.Warnings, "unassigned_building_hvac_energy")
	if warning == nil {
		t.Fatalf("0.001 kWh unassigned remainder was hidden by a visibility cutoff: %#v", result.Warnings)
	}
	message := strings.ToLower(warning.Message)
	for _, token := range []string{"cooling", "district", "0.001", "kwh"} {
		if !strings.Contains(message, token) {
			t.Errorf("unassigned warning lacks %q context: %q", token, warning.Message)
		}
	}
	office := epath100AuditZoneResult(result.ZoneResults, "Office")
	if office == nil {
		t.Fatalf("missing direct Office zone result: %#v", result.AvailableZones)
	}
	if node := epath094AuditNode(office.Nodes, "end_use.cooling.office"); node == nil || node.Value != 24.999 || node.Basis != "direct_zone_energy" {
		t.Errorf("direct truth changed while preserving Building remainder: %#v", node)
	}
	buildingEndUse := epath094AuditNode(result.Nodes, "end_use.cooling.building")
	buildingCarrier := epath094AuditNode(result.Nodes, "carrier.district_cooling.building")
	closed := row != nil && buildingEndUse != nil && epath100AuditNearlyEqual(24.999+row.ResidualValue, buildingEndUse.Value)
	if buildingEndUse == nil || buildingEndUse.Value != 25 || buildingCarrier == nil || buildingCarrier.Value != 25 || !closed {
		t.Errorf("unassigned remainder does not close to unchanged Building totals: end-use %#v carrier %#v row %#v", buildingEndUse, buildingCarrier, row)
	}
	if office != nil {
		epath100AuditAssertNoBuildingAllocationQuality(t, *office)
	}
}

func TestEPATH100AuditDirectAboveBuildingStaysTruthfulAndStopsAllocation(t *testing.T) {
	nodes := []EnergyExplanationNode{
		epath100AuditCarrierNode("electricity", 20, "annual", "facility.electricity"),
		epath100AuditEndUseNode("heating", "electricity", 20, "annual", "meter.heating.electricity", []string{"path.office.heating", "path.lab.heating"}),
		epath100AuditLoadNode("Office", "heating", 20, "annual", "load.office.heating", []string{"path.office.heating"}),
		epath100AuditLoadNode("Lab", "heating", 80, "annual", "load.lab.heating", []string{"path.lab.heating"}),
	}
	direct := epath094AuditDirectSeries("Office", "heating", "electricity", 30, "direct.office.heating.electricity")
	direct.ServiceKind = "heating"
	input := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:            nodes,
		Edges: []EnergyExplanationEdge{
			epath100AuditMeterEdge("annual", nodes[0], nodes[1]),
			epath100AuditDeliveredEdge("annual", nodes[1], nodes[2], "heating", []string{"path.office.heating"}),
			epath100AuditDeliveredEdge("annual", nodes[1], nodes[3], "heating", []string{"path.lab.heating"}),
		},
		Sources:             []EnergyDataSource{{ID: "facility.electricity"}, {ID: "meter.heating.electricity"}, {ID: "load.office.heating", ZoneName: "Office"}, {ID: "load.lab.heating", ZoneName: "Lab"}, {ID: "direct.office.heating.electricity", ZoneName: "Office"}},
		zoneDirectUseSeries: []energyExplanationSeries{direct},
	}
	allocated := applyEnergyExplanationV1ServicePathLoadShareAllocation(input)
	for _, edge := range allocated.Edges {
		if edge.Relation == "allocation" && edge.RuleID == energyRelationshipRuleAllocatedServicePathLoad {
			t.Errorf("negative remainder produced an allocation edge: %#v", edge)
		}
	}
	result := UpgradeEnergyExplanationV1(allocated)
	office := epath100AuditZoneResult(result.ZoneResults, "Office")
	lab := epath100AuditZoneResult(result.ZoneResults, "Lab")
	if office == nil || epath094AuditNode(office.Nodes, "end_use.heating.office") == nil || epath094AuditNode(office.Nodes, "end_use.heating.office").Value != 30 {
		t.Errorf("direct-above-building truth was clamped: %#v", office)
	}
	if lab != nil && epath094AuditNode(lab.Nodes, "end_use.heating.lab") != nil {
		t.Errorf("negative allocatable pool leaked into Lab: %#v", lab.Nodes)
	}
	if office != nil {
		epath100AuditAssertNoBuildingAllocationQuality(t, *office)
	}
	if lab != nil {
		epath100AuditAssertNoBuildingAllocationQuality(t, *lab)
	}
	row := epath100AuditAllocationReconciliation(result.Reconciliation, "heating", "electricity", "annual")
	if row == nil || row.Level != "allocation" || row.Basis != "service_path_allocation" || row.ExpectedValue != 20 || row.ExplainedValue != 30 || row.ResidualValue != -10 || row.Status != "overmapped" || row.Label != "Direct zone HVAC energy exceeds building HVAC energy" ||
		!stringSliceContains(row.SourceIDs, "meter.heating.electricity") || !stringSliceContains(row.SourceIDs, "direct.office.heating.electricity") {
		t.Errorf("direct-above-building reconciliation = %#v", row)
	}
	if warning := epath100AuditWarning(result.Warnings, "direct_zone_hvac_energy_exceeds_building"); warning == nil {
		t.Errorf("direct-above-building condition lacks an exceeds warning: %#v", result.Warnings)
	} else {
		message := strings.ToLower(warning.Message)
		for _, token := range []string{"heating", "electric", "10", "kwh"} {
			if !strings.Contains(message, token) {
				t.Errorf("direct-above-building warning lacks %q context: %q", token, warning.Message)
			}
		}
	}
	if epath100AuditHasWarning(result.Warnings, "unassigned_building_hvac_energy") {
		t.Errorf("negative residual was mislabeled as unassigned energy: %#v", result.Warnings)
	}
	buildingEndUse := epath094AuditNode(result.Nodes, "end_use.heating.building")
	if buildingEndUse == nil || buildingEndUse.Value != 20 || row == nil || row.ExplainedValue+row.ResidualValue != buildingEndUse.Value {
		t.Errorf("overmapped ledger did not preserve the Building truth: node %#v row %#v", buildingEndUse, row)
	}
}

func TestEPATH100AuditPositiveDirectAgainstZeroBuildingMeterIsOvermapped(t *testing.T) {
	nodes := []EnergyExplanationNode{
		epath100AuditCarrierNode("electricity", 0, "annual", "facility.electricity"),
		epath100AuditEndUseNode("heating", "electricity", 0, "annual", "meter.heating.electricity", []string{"path.office.heating", "path.lab.heating"}),
		epath100AuditLoadNode("Office", "heating", 20, "annual", "load.office.heating", []string{"path.office.heating"}),
		epath100AuditLoadNode("Lab", "heating", 80, "annual", "load.lab.heating", []string{"path.lab.heating"}),
	}
	direct := epath094AuditDirectSeries("Office", "heating", "electricity", 30, "direct.office.heating.electricity")
	direct.ServiceKind = "heating"
	input := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:            nodes,
		Edges: []EnergyExplanationEdge{
			epath100AuditMeterEdge("annual", nodes[0], nodes[1]),
			epath100AuditDeliveredEdge("annual", nodes[1], nodes[2], "heating", []string{"path.office.heating"}),
			epath100AuditDeliveredEdge("annual", nodes[1], nodes[3], "heating", []string{"path.lab.heating"}),
		},
		Sources: []EnergyDataSource{
			{ID: "facility.electricity", SourceType: "sql_meter"}, {ID: "meter.heating.electricity", SourceType: "sql_meter"},
			{ID: "load.office.heating", SourceType: "sql_variable", ZoneName: "Office"}, {ID: "load.lab.heating", SourceType: "sql_variable", ZoneName: "Lab"},
			{ID: "direct.office.heating.electricity", SourceType: "sql_variable", ZoneName: "Office"},
		},
		zoneDirectUseSeries: []energyExplanationSeries{direct},
	}
	allocated := applyEnergyExplanationV1ServicePathLoadShareAllocation(input)
	for _, edge := range allocated.Edges {
		if edge.Relation == "allocation" {
			t.Errorf("zero Building meter generated allocation edge: %#v", edge)
		}
	}
	result := UpgradeEnergyExplanationV1(allocated)
	office := epath100AuditZoneResult(result.ZoneResults, "Office")
	lab := epath100AuditZoneResult(result.ZoneResults, "Lab")
	if office == nil {
		t.Fatalf("positive direct observation disappeared against zero Building meter: %#v", result.AvailableZones)
	}
	if node := epath094AuditNode(office.Nodes, "end_use.heating.office"); node == nil || node.Value != 30 || node.Basis != "direct_zone_energy" {
		t.Errorf("zero-meter direct truth = %#v", node)
	}
	if lab != nil && epath094AuditNode(lab.Nodes, "end_use.heating.lab") != nil {
		t.Errorf("zero Building meter allocated energy to Lab: %#v", lab.Nodes)
	}
	row := epath100AuditAllocationReconciliation(result.Reconciliation, "heating", "electricity", "annual")
	if row == nil || row.ExpectedValue != 0 || row.ExplainedValue != 30 || row.ResidualValue != -30 || row.Status != "overmapped" || row.Label != "Direct zone HVAC energy exceeds building HVAC energy" ||
		!stringSliceContains(row.SourceIDs, "meter.heating.electricity") || !stringSliceContains(row.SourceIDs, "direct.office.heating.electricity") {
		t.Errorf("zero-meter overmapped reconciliation = %#v", row)
	}
	if warning := epath100AuditWarning(result.Warnings, "direct_zone_hvac_energy_exceeds_building"); warning == nil {
		t.Errorf("zero-meter direct overlap lacks exact exceeds warning: %#v", result.Warnings)
	}
	if epath100AuditHasWarning(result.Warnings, "unassigned_building_hvac_energy") {
		t.Errorf("zero-meter negative residual mislabeled as unassigned: %#v", result.Warnings)
	}
	epath100AuditAssertNoBuildingAllocationQuality(t, *office)
}

func TestEPATH100AuditEPATH094DirectBranchesRemainUnallocated(t *testing.T) {
	input := epath094AuditSelectedZoneFixture()
	input.AllocationPolicy = PurposeAllocationPolicyByServicePathLoadShare
	result := UpgradeEnergyExplanationV1(applyEnergyExplanationV1ServicePathLoadShareAllocation(input))
	for id, want := range map[string]float64{
		"end_use.lighting.office":    11,
		"end_use.equipment.office":   12,
		"end_use.cooling.office":     10,
		"carrier.electricity.office": 29,
	} {
		node := epath094AuditNode(result.Nodes, id)
		if node == nil || node.Value != want || node.Basis != "direct_zone_energy" {
			t.Errorf("EPATH-094 direct node changed under automatic allocation %q: %#v", id, node)
		}
	}
	for _, link := range result.Links {
		if (strings.Contains(link.FromID, "lighting") || strings.Contains(link.FromID, "equipment") || strings.Contains(link.ToID, "lighting") || strings.Contains(link.ToID, "equipment")) &&
			(link.Basis == "service_path_allocation" || link.Basis == "zone_load_allocation") {
			t.Errorf("EPATH-100 allocated a direct EPATH-094 branch: %#v", link)
		}
	}
}

func epath100AuditCarrierNode(carrier string, value float64, period string, sourceID string) EnergyExplanationNode {
	part := canonicalEnergyPathPart(carrier)
	return EnergyExplanationNode{
		ID:                  "energy.carrier." + part,
		Level:               "energy",
		Kind:                "energy." + part + ".total",
		Label:               strings.ReplaceAll(part, "_", " "),
		Value:               value,
		RawValue:            value,
		EffectiveValue:      value,
		Unit:                "kWh",
		Period:              period,
		Carrier:             carrier,
		EndUse:              "total",
		MeterHierarchyLevel: "facility_total",
		Basis:               "measured_meter",
		SourceIDs:           []string{sourceID},
	}
}

func epath100AuditEndUseNode(endUse string, carrier string, value float64, period string, sourceID string, pathIDs []string) EnergyExplanationNode {
	endUsePart := canonicalEnergyPathPart(endUse)
	carrierPart := canonicalEnergyPathPart(carrier)
	return EnergyExplanationNode{
		ID:                  "energy.end_use." + endUsePart + "." + carrierPart,
		Level:               "energy",
		Kind:                "energy." + endUsePart,
		Label:               strings.ReplaceAll(endUsePart, "_", " "),
		Value:               value,
		RawValue:            value,
		EffectiveValue:      value,
		Unit:                "kWh",
		Period:              period,
		ServiceKind:         endUse,
		Carrier:             carrier,
		EndUse:              endUse,
		MeterHierarchyLevel: "broad_end_use",
		Basis:               "measured_meter",
		RelatedPathIDs:      append([]string(nil), pathIDs...),
		SourceIDs:           []string{sourceID},
	}
}

func epath100AuditLoadNode(zone string, service string, value float64, period string, sourceID string, pathIDs []string) EnergyExplanationNode {
	servicePart := canonicalEnergyPathPart(service)
	return EnergyExplanationNode{
		ID:               "load." + servicePart + "." + metricID(zone),
		Level:            "load",
		Kind:             "load.zone_" + servicePart,
		Label:            zone + " " + strings.ReplaceAll(servicePart, "_", " ") + " load",
		Value:            value,
		RawValue:         value,
		EffectiveValue:   value,
		Unit:             "kWh thermal",
		Period:           period,
		ZoneName:         zone,
		ServiceKind:      service,
		PathType:         "zone",
		Basis:            "measured_energy_variable",
		RelatedPathIDs:   append([]string(nil), pathIDs...),
		RelatedEntityIDs: append([]string(nil), pathIDs...),
		SourceIDs:        []string{sourceID},
	}
}

func epath100AuditMeterEdge(period string, carrier EnergyExplanationNode, endUse EnergyExplanationNode) EnergyExplanationEdge {
	return EnergyExplanationEdge{
		ID:             "meter." + canonicalEnergyPathPart(endUse.EndUse) + "." + canonicalEnergyPathPart(endUse.Carrier) + "." + metricID(period),
		FromID:         carrier.ID,
		ToID:           endUse.ID,
		Value:          endUse.Value,
		DisplayValue:   endUse.Value,
		Unit:           endUse.Unit,
		Period:         period,
		Relation:       "meter_enduse",
		Basis:          "measured_meter",
		RuleID:         energyRelationshipRuleMeterEndUse,
		SourceIDs:      append([]string(nil), endUse.SourceIDs...),
		ServiceKind:    endUse.ServiceKind,
		RelatedPathIDs: append([]string(nil), endUse.RelatedPathIDs...),
	}
}

func epath100AuditDeliveredEdge(period string, endUse EnergyExplanationNode, load EnergyExplanationNode, service string, pathIDs []string) EnergyExplanationEdge {
	return EnergyExplanationEdge{
		ID:             "delivered." + canonicalEnergyPathPart(endUse.Carrier) + "." + canonicalEnergyPathPart(service) + "." + metricID(load.ZoneName) + "." + metricID(period),
		FromID:         endUse.ID,
		ToID:           load.ID,
		Value:          load.Value,
		DisplayValue:   load.Value,
		Unit:           load.Unit,
		Period:         period,
		Relation:       "delivered_load",
		Basis:          "measured_variable",
		RuleID:         energyRelationshipRuleMeasuredLoad,
		SourceIDs:      append([]string(nil), load.SourceIDs...),
		ZoneName:       load.ZoneName,
		ServiceKind:    service,
		RelatedPathIDs: append([]string(nil), pathIDs...),
	}
}

func epath100AuditReverse[T any](values []T) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

type epath100AuditSnapshot struct {
	Periods        []string
	ZoneNodes      []string
	ZoneLinks      []string
	Reconciliation []string
	Warnings       []string
	Sources        []string
}

func epath100AuditAllocationSnapshot(result EnergyExplanationResult) epath100AuditSnapshot {
	snapshot := epath100AuditSnapshot{Periods: epath100AuditPeriodIDs(result.Periods)}
	appendZone := func(zone string, period string, nodes []EnergyExplanationNode, links []EnergyPathLink) {
		for _, node := range nodes {
			if node.Basis != "service_path_allocation" && node.Basis != "zone_load_allocation" && node.Basis != "direct_zone_energy" {
				continue
			}
			snapshot.ZoneNodes = append(snapshot.ZoneNodes, fmt.Sprintf("%s|%s|%s|%.12g|%s|%s", zone, period, node.ID, node.Value, node.Basis, strings.Join(node.SourceIDs, ",")))
		}
		for _, link := range links {
			if link.Basis != "service_path_allocation" && link.Basis != "zone_load_allocation" && link.Basis != "direct_zone_energy" {
				continue
			}
			snapshot.ZoneLinks = append(snapshot.ZoneLinks, fmt.Sprintf("%s|%s|%s|%s|%.12g|%.12g|%s|%s|%s", zone, period, link.FromID, link.ToID, link.FromValue, link.ToValue, link.Basis, strings.Join(link.SourceIDs, ","), strings.Join(link.RelatedPathIDs, ",")))
		}
	}
	for _, zone := range result.ZoneResults {
		appendZone(zone.Scope.ZoneName, "annual", zone.Nodes, zone.Links)
		for _, period := range zone.Periods {
			appendZone(zone.Scope.ZoneName, period.ID, period.Nodes, period.Links)
		}
	}
	for _, row := range result.Reconciliation {
		if !strings.HasPrefix(row.ID, "reconcile.zone_hvac_allocation.") {
			continue
		}
		snapshot.Reconciliation = append(snapshot.Reconciliation, fmt.Sprintf("%s|%s|%s|%.12g|%.12g|%.12g|%s", row.ID, row.Status, row.Label, row.ExpectedValue, row.ExplainedValue, row.ResidualValue, strings.Join(row.SourceIDs, ",")))
	}
	for _, warning := range result.Warnings {
		if warning.Code == "unassigned_building_hvac_energy" || warning.Code == "direct_zone_hvac_energy_exceeds_building" {
			snapshot.Warnings = append(snapshot.Warnings, warning.Code+"|"+warning.Period+"|"+warning.Message)
		}
	}
	for _, source := range result.Sources {
		if !strings.Contains(source.ID, "cooling") && !strings.Contains(source.ID, "heating") {
			continue
		}
		details := make([]string, 0, len(source.ScopeDetails))
		for _, detail := range source.ScopeDetails {
			details = append(details, fmt.Sprintf("%s:%t:%.12g:%.12g:%s", detail.Scope.ZoneName, detail.AllocationApplied, detail.AllocationFactor, detail.AllocatedValue, detail.AggregationBasis))
		}
		snapshot.Sources = append(snapshot.Sources, source.ID+"|"+strings.Join(source.RelatedEntityIDs, ",")+"|"+strings.Join(details, ","))
	}
	sort.Strings(snapshot.ZoneNodes)
	sort.Strings(snapshot.ZoneLinks)
	sort.Strings(snapshot.Reconciliation)
	sort.Strings(snapshot.Warnings)
	sort.Strings(snapshot.Sources)
	return snapshot
}

func epath100AuditPeriodIDs(periods []EnergyPeriod) []string {
	ids := make([]string, 0, len(periods))
	for _, period := range periods {
		ids = append(ids, period.ID)
	}
	return ids
}

func epath100AuditZoneResult(results []EnergyExplanationZoneResult, zoneName string) *EnergyExplanationZoneResult {
	for index := range results {
		if strings.EqualFold(strings.TrimSpace(results[index].Scope.ZoneName), strings.TrimSpace(zoneName)) {
			return &results[index]
		}
	}
	return nil
}

func epath100AuditMonthlyNodeSum(periods []EnergyPeriod, nodeID string) float64 {
	total := 0.0
	for _, period := range periods {
		if !strings.EqualFold(period.Kind, "monthly") {
			continue
		}
		if node := epath094AuditNode(period.Nodes, nodeID); node != nil {
			total += node.Value
		}
	}
	return total
}

func epath100AuditAssertNoBuildingAllocationQuality(t *testing.T, zone EnergyExplanationZoneResult) {
	t.Helper()
	for _, row := range zone.Reconciliation {
		if strings.HasPrefix(row.ID, "reconcile.zone_hvac_allocation.") || row.Label == "Unassigned building HVAC energy" || row.Label == "Direct zone HVAC energy exceeds building HVAC energy" {
			t.Errorf("building-only HVAC allocation reconciliation leaked into %s: %#v", zone.Scope.ZoneName, row)
		}
	}
	for _, warning := range zone.Warnings {
		if warning.Code == "unassigned_building_hvac_energy" || warning.Code == "direct_zone_hvac_energy_exceeds_building" {
			t.Errorf("building-only HVAC allocation warning leaked into %s: %#v", zone.Scope.ZoneName, warning)
		}
	}
	for _, node := range zone.Nodes {
		identity := strings.ToLower(node.ID + " " + node.Label)
		if strings.Contains(identity, "unassigned") && strings.Contains(identity, "hvac") {
			t.Errorf("building-only unassigned HVAC node leaked into %s: %#v", zone.Scope.ZoneName, node)
		}
	}
	for _, link := range zone.Links {
		identity := strings.ToLower(link.ID + " " + link.FromID + " " + link.ToID + " " + link.Explanation)
		if strings.Contains(identity, "unassigned") && strings.Contains(identity, "hvac") {
			t.Errorf("building-only unassigned HVAC link leaked into %s: %#v", zone.Scope.ZoneName, link)
		}
	}
}

func epath100AuditAllocationReconciliation(rows []EnergyReconciliation, service string, carrier string, period string) *EnergyReconciliation {
	wantID := "reconcile.zone_hvac_allocation." + canonicalEnergyPathPart(service) + "." + canonicalEnergyPathPart(carrier) + "." + metricID(period)
	for index := range rows {
		if rows[index].ID == wantID {
			return &rows[index]
		}
	}
	return nil
}

func epath100AuditHasWarning(warnings []EnergyWarning, code string) bool {
	return epath100AuditWarning(warnings, code) != nil
}

func epath100AuditWarning(warnings []EnergyWarning, code string) *EnergyWarning {
	for index := range warnings {
		if warnings[index].Code == code {
			return &warnings[index]
		}
	}
	return nil
}

func epath100AuditAssertScopeDetail(t *testing.T, source *EnergyDataSource, zoneName string, allocationApplied bool, factor float64, allocatedValue float64) {
	t.Helper()
	if source == nil {
		t.Errorf("missing source for %s scope detail", zoneName)
		return
	}
	for _, detail := range source.ScopeDetails {
		if !strings.EqualFold(detail.Scope.ZoneName, zoneName) {
			continue
		}
		if detail.AllocationApplied != allocationApplied || !epath100AuditNearlyEqual(detail.AllocationFactor, factor) || !epath100AuditNearlyEqual(detail.AllocatedValue, allocatedValue) || detail.AggregationBasis != "model_total" {
			t.Errorf("%s source %s scope detail = %#v, want applied=%t factor=%.6g value=%.6g aggregation=model_total", zoneName, source.ID, detail, allocationApplied, factor, allocatedValue)
		}
		return
	}
	t.Errorf("source %s has no %s scope detail: %#v", source.ID, zoneName, source.ScopeDetails)
}

func epath100AuditNearlyEqual(left float64, right float64) bool {
	return math.Abs(left-right) <= 1e-9*math.Max(1, math.Max(math.Abs(left), math.Abs(right)))
}
