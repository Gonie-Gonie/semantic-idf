package simulation

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// This audit fixture deliberately mixes two zones, two periods, two valid
// direct-use families, a People heat driver, and malformed correspondence
// edges.  Only exact Lighting and Equipment pairs may survive the v1 -> v2
// compatibility boundary.
func TestEPATH093AuditCorrespondenceIsBoundedNonFlowAndTraceable(t *testing.T) {
	legacy := epath093AuditLegacy(false)
	before, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}

	result := UpgradeEnergyExplanationV1(legacy)
	after, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("EPATH-093 upgrade mutated frozen v1 input\nbefore: %s\nafter:  %s", before, after)
	}

	epath093AuditAssertPair(t, result.Nodes, result.Links, "building", "lighting", "cooling", 20, 30,
		[]string{"heat.lighting.lab", "heat.lighting.office", "meter.lighting.lab", "meter.lighting.office"})
	epath093AuditAssertPair(t, result.Nodes, result.Links, "building", "equipment", "heating", 10, 14,
		[]string{"heat.equipment.lab", "heat.equipment.office", "meter.equipment.lab", "meter.equipment.office"})
	epath093AuditAssertOnlyExactPairs(t, result.Nodes, result.Links)
	epath093AuditAssertAcyclicForwardLinks(t, result.Nodes, result.Links)
	epath093AuditAssertNoPeopleSiteNode(t, result.Nodes, result.Links)

	for _, source := range []struct {
		id   string
		name string
	}{
		{id: "meter.lighting.office", name: "Office InteriorLights:Electricity"},
		{id: "heat.lighting.office", name: "Office Zone Lights Total Heating Energy"},
		{id: "meter.equipment.lab", name: "Lab InteriorEquipment:Electricity"},
		{id: "heat.equipment.lab", name: "Lab Zone Electric Equipment Total Heating Energy"},
	} {
		got := energyExplanationSourceByID(result.Sources, source.id)
		if got == nil || got.Name != source.name {
			t.Errorf("source provenance %q = %#v, want original name %q", source.id, got, source.name)
		}
	}
}

func TestEPATH093AuditScopePeriodAndInputOrderInvariance(t *testing.T) {
	forward := UpgradeEnergyExplanationV1(epath093AuditLegacy(false))
	reverse := UpgradeEnergyExplanationV1(epath093AuditLegacy(true))

	epath093AuditAssertRelationJSONEqual(t, "building annual", forward.Links, reverse.Links)
	for _, periodID := range []string{"annual", "M1", "M2"} {
		left := energyExplanationPeriodByID(forward.Periods, periodID)
		right := energyExplanationPeriodByID(reverse.Periods, periodID)
		if left == nil || right == nil {
			t.Fatalf("period %q missing: forward=%#v reverse=%#v", periodID, left, right)
		}
		epath093AuditAssertRelationJSONEqual(t, "building "+periodID, left.Links, right.Links)
	}

	annualLighting := epath093AuditRelation(forward.Links, "lighting")
	m1Lighting := epath093AuditRelation(energyExplanationPeriodByID(forward.Periods, "M1").Links, "lighting")
	m2Lighting := epath093AuditRelation(energyExplanationPeriodByID(forward.Periods, "M2").Links, "lighting")
	if annualLighting == nil || m1Lighting == nil || m2Lighting == nil ||
		annualLighting.FromValue != m1Lighting.FromValue+m2Lighting.FromValue ||
		annualLighting.ToValue != m1Lighting.ToValue+m2Lighting.ToValue {
		t.Fatalf("annual correspondence is not the exact monthly sum: annual=%#v M1=%#v M2=%#v", annualLighting, m1Lighting, m2Lighting)
	}

	if len(forward.ZoneResults) != 2 || len(reverse.ZoneResults) != 2 {
		t.Fatalf("zone result inventory changed: forward=%#v reverse=%#v", forward.AvailableZones, reverse.AvailableZones)
	}
	for _, zoneName := range []string{"Lab", "Office"} {
		left := epath093AuditZoneResult(forward.ZoneResults, zoneName)
		right := epath093AuditZoneResult(reverse.ZoneResults, zoneName)
		if left == nil || right == nil {
			t.Fatalf("zone %q missing: forward=%#v reverse=%#v", zoneName, left, right)
		}
		epath093AuditAssertRelationJSONEqual(t, zoneName+" annual", left.Links, right.Links)
		epath093AuditAssertOnlyExactPairs(t, left.Nodes, left.Links)
		epath093AuditAssertAcyclicForwardLinks(t, left.Nodes, left.Links)
		epath093AuditAssertNoPeopleSiteNode(t, left.Nodes, left.Links)

		lightingThermal, lightingEnergy := 12.0, 20.0
		equipmentThermal, equipmentEnergy := 6.0, 8.0
		if zoneName == "Office" {
			lightingThermal, lightingEnergy = 8, 10
			equipmentThermal, equipmentEnergy = 4, 6
		}
		scopeToken := strings.ToLower(zoneName)
		epath093AuditAssertPair(t, left.Nodes, left.Links, scopeToken, "lighting", "cooling", lightingThermal, lightingEnergy,
			[]string{"heat.lighting." + scopeToken, "meter.lighting." + scopeToken})
		epath093AuditAssertPair(t, left.Nodes, left.Links, scopeToken, "equipment", "heating", equipmentThermal, equipmentEnergy,
			[]string{"heat.equipment." + scopeToken, "meter.equipment." + scopeToken})

		for _, periodID := range []string{"M1", "M2"} {
			leftPeriod := energyExplanationPeriodByID(left.Periods, periodID)
			rightPeriod := energyExplanationPeriodByID(right.Periods, periodID)
			if leftPeriod == nil || rightPeriod == nil {
				t.Fatalf("zone %q period %q missing", zoneName, periodID)
			}
			epath093AuditAssertRelationJSONEqual(t, zoneName+" "+periodID, leftPeriod.Links, rightPeriod.Links)
		}
	}
}

func TestEPATH093AuditStoredV1PeopleEndUseIsSuppressedButUnknownIsPreserved(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Nodes: []EnergyExplanationNode{
			{ID: "legacy.carrier", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Value: 9, Unit: "kWh site", Carrier: "electricity", EndUse: "total", MeterHierarchyLevel: "facility_total", SourceIDs: []string{"meter.facility"}},
			{ID: "legacy.people.enduse", Level: "energy", Kind: "energy.people", Label: "People electricity", Value: 4, Unit: "kWh site", Carrier: "electricity", EndUse: "people", MeterHierarchyLevel: "broad_end_use", SourceIDs: []string{"meter.people"}},
			{ID: "legacy.occupants.enduse", Level: "energy", Kind: "energy.occupants", Label: "Occupants electricity", Value: 3, Unit: "kWh site", Carrier: "electricity", EndUse: "occupants", MeterHierarchyLevel: "broad_end_use", SourceIDs: []string{"meter.occupants"}},
			{ID: "legacy.unknown.enduse", Level: "energy", Kind: "energy.owner_misc_alpha", Label: "Owner misc alpha", Value: 5, Unit: "kWh site", Carrier: "electricity", EndUse: "owner_misc_alpha", MeterHierarchyLevel: "broad_end_use", SourceIDs: []string{"meter.unknown"}},
			{ID: "legacy.people.driver", Level: "heat", Kind: "heat.people", Label: "People", Value: 2, SignedValue: 2, DisplayValue: 2, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "cooling", DriverCategory: "internal.people", SourceIDs: []string{"heat.people"}},
		},
		Edges: []EnergyExplanationEdge{
			{ID: "carrier.people", FromID: "legacy.carrier", ToID: "legacy.people.enduse", Value: 4, Unit: "kWh site", Relation: "meter_enduse", SourceIDs: []string{"meter.people"}},
			{ID: "carrier.occupants", FromID: "legacy.carrier", ToID: "legacy.occupants.enduse", Value: 3, Unit: "kWh site", Relation: "meter_enduse", SourceIDs: []string{"meter.occupants"}},
			{ID: "carrier.unknown", FromID: "legacy.carrier", ToID: "legacy.unknown.enduse", Value: 5, Unit: "kWh site", Relation: "meter_enduse", SourceIDs: []string{"meter.unknown"}},
			{ID: "people.correspondence", FromID: "legacy.people.enduse", ToID: "legacy.people.driver", Value: 2, Unit: "kWh thermal", Relation: "internal_gain_heat", RuleID: energyRelationshipRuleInternalGainHeat, SourceIDs: []string{"meter.people", "heat.people"}},
		},
		Sources: []EnergyDataSource{
			{ID: "meter.facility", SourceType: "sql_meter", Name: "Electricity:Facility"},
			{ID: "meter.people", SourceType: "stored_v1", Name: "People:Electricity"},
			{ID: "meter.occupants", SourceType: "stored_v1", Name: "Occupants:Electricity"},
			{ID: "meter.unknown", SourceType: "stored_v1", Name: "OwnerMiscAlpha:Electricity"},
			{ID: "heat.people", SourceType: "sql_variable", Name: "Zone People Convective Heating Energy", ZoneName: "Office", DriverRole: energyDriverSourceRoleMainFlow, DriverCategory: "internal.people"},
		},
		Completeness: EnergyCompleteness{Status: "complete"},
	}
	result := UpgradeEnergyExplanationV1(legacy)
	epath093AuditAssertNoPeopleSiteNode(t, result.Nodes, result.Links)

	other := energyPathV2NodeByID(result.Nodes, "end_use.other.building")
	if other == nil || other.Value != 5 || !slices.Equal(other.SourceIDs, []string{"meter.unknown"}) {
		t.Fatalf("unrelated unknown end use was lost or contaminated while suppressing People: %#v", other)
	}
	branch := energyPathV2LinkByIDs(result.Links, "end_use.other.building", "carrier.electricity.building")
	if branch == nil || branch.Relation != "direct_end_use_to_carrier" || branch.FromValue != 5 || branch.ToValue != 5 || !slices.Equal(branch.SourceIDs, []string{"meter.unknown"}) {
		t.Fatalf("unrelated unknown carrier branch = %#v", branch)
	}
	if energyExplanationSourceByID(result.Sources, "meter.unknown") == nil {
		t.Error("unrelated unknown source provenance was dropped")
	}
}

func epath093AuditAssertPair(t *testing.T, nodes []EnergyExplanationNode, links []EnergyPathLink, scopeToken string, endUse string, service string, thermal float64, site float64, wantSources []string) {
	t.Helper()
	driverID := "driver.internal." + endUse + "." + service + "." + scopeToken
	endUseID := "end_use." + endUse + "." + scopeToken
	driver := energyPathV2NodeByID(nodes, driverID)
	energy := energyPathV2NodeByID(nodes, endUseID)
	if driver == nil || driver.Level != "driver" || driver.ScaleDomain != "thermal" || driver.DriverCategory != "internal."+endUse {
		t.Fatalf("%s thermal endpoint = %#v; nodes=%#v", endUse, driver, nodes)
	}
	if energy == nil || energy.Level != "end_use" || energy.ScaleDomain != "site" || energy.EndUse != endUse || energy.Carrier != "" {
		t.Fatalf("%s site-energy endpoint = %#v", endUse, energy)
	}
	relation := epath093AuditRelationForIDs(links, driverID, endUseID)
	if relation == nil {
		t.Fatalf("%s correspondence missing from %#v", endUse, links)
	}
	if relation.Relation != "source_correspondence" || relation.FromValue != thermal || relation.ToValue != site ||
		relation.FromUnit != "kWh thermal" || relation.ToUnit != "kWh site" || relation.ServiceKind != service ||
		relation.RuleID != energyRelationshipRuleInternalGainHeat || relation.Basis == "" || relation.Explanation == "" {
		t.Errorf("%s correspondence contract = %#v", endUse, relation)
	}
	if relation.Ratio != 0 || relation.RatioKind != "" || relation.RatioLabel != "" {
		t.Errorf("%s non-flow correspondence advertised conversion metadata: %#v", endUse, relation)
	}
	gotSources := append([]string(nil), relation.SourceIDs...)
	slices.Sort(gotSources)
	wantSources = append([]string(nil), wantSources...)
	slices.Sort(wantSources)
	if !slices.Equal(gotSources, wantSources) {
		t.Errorf("%s correspondence sources = %#v, want %#v", endUse, gotSources, wantSources)
	}
}

func epath093AuditAssertOnlyExactPairs(t *testing.T, nodes []EnergyExplanationNode, links []EnergyPathLink) {
	t.Helper()
	nodeByID := make(map[string]EnergyExplanationNode, len(nodes))
	for _, node := range nodes {
		nodeByID[node.ID] = node
	}
	count := 0
	for _, link := range links {
		from := nodeByID[link.FromID]
		to := nodeByID[link.ToID]
		if link.Relation != "source_correspondence" {
			if (from.DriverCategory == "internal.lighting" || from.DriverCategory == "internal.equipment") && to.Level == "end_use" {
				t.Errorf("direct-use pair leaked into main-flow relation: %#v", link)
			}
			continue
		}
		count++
		if from.Level != "driver" || to.Level != "end_use" {
			t.Errorf("correspondence endpoints are not Stage 1 -> Stage 3: from=%#v to=%#v link=%#v", from, to, link)
			continue
		}
		wantEndUse := strings.TrimPrefix(from.DriverCategory, "internal.")
		if (wantEndUse != "lighting" && wantEndUse != "equipment") || to.EndUse != wantEndUse {
			t.Errorf("non-exact direct-use correspondence survived: from=%#v to=%#v link=%#v", from, to, link)
		}
		if epath093AuditRelationForIDs(links, link.ToID, link.FromID) != nil {
			t.Errorf("reverse correspondence survived: %#v", link)
		}
	}
	if count != 2 {
		t.Errorf("source_correspondence count = %d, want exactly Lighting and Equipment; links=%#v", count, links)
	}
}

func epath093AuditAssertAcyclicForwardLinks(t *testing.T, nodes []EnergyExplanationNode, links []EnergyPathLink) {
	t.Helper()
	rank := map[string]int{"driver": 0, "load": 1, "end_use": 2, "carrier": 3}
	nodeByID := make(map[string]EnergyExplanationNode, len(nodes))
	for _, node := range nodes {
		nodeByID[node.ID] = node
	}
	adjacency := map[string][]string{}
	for _, link := range links {
		from, fromOK := nodeByID[link.FromID]
		to, toOK := nodeByID[link.ToID]
		if !fromOK || !toOK {
			t.Errorf("link has missing endpoint: %#v", link)
			continue
		}
		fromRank, rankedFrom := rank[from.Level]
		toRank, rankedTo := rank[to.Level]
		if rankedFrom && rankedTo && fromRank >= toRank {
			t.Errorf("reverse/same-stage link violates canonical direction: from=%#v to=%#v link=%#v", from, to, link)
		}
		adjacency[link.FromID] = append(adjacency[link.FromID], link.ToID)
	}
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(id string) bool {
		if state[id] == 1 {
			return true
		}
		if state[id] == 2 {
			return false
		}
		state[id] = 1
		for _, next := range adjacency[id] {
			if visit(next) {
				return true
			}
		}
		state[id] = 2
		return false
	}
	for id := range nodeByID {
		if visit(id) {
			t.Fatalf("energy path contains a cycle through %q: %#v", id, links)
		}
	}
}

func epath093AuditAssertNoPeopleSiteNode(t *testing.T, nodes []EnergyExplanationNode, links []EnergyPathLink) {
	t.Helper()
	for _, node := range nodes {
		if node.Level == "end_use" && (node.EndUse == "people" || strings.Contains(node.ID, ".people.") || strings.EqualFold(node.Label, "People")) {
			t.Errorf("People was promoted to a direct site-energy end use: %#v", node)
		}
		if node.Level == "end_use" && (stringSliceContains(node.SourceIDs, "meter.people") || stringSliceContains(node.SourceIDs, "meter.occupants") || stringSliceContains(node.SourceIDs, "meter.people.office") || stringSliceContains(node.SourceIDs, "meter.people.lab")) {
			t.Errorf("People/occupants was merely renamed and retained in a Stage 3 node: %#v", node)
		}
	}
	for _, link := range links {
		if link.Relation == "source_correspondence" && strings.Contains(link.FromID, ".internal.people.") {
			t.Errorf("People received a direct-use correspondence: %#v", link)
		}
		for _, sourceID := range link.SourceIDs {
			if sourceID == "meter.people" || sourceID == "meter.occupants" || strings.HasPrefix(sourceID, "meter.people.") {
				t.Errorf("People/occupants direct site-energy branch survived through provenance: %#v", link)
			}
		}
	}
}

func epath093AuditAssertRelationJSONEqual(t *testing.T, label string, left []EnergyPathLink, right []EnergyPathLink) {
	t.Helper()
	leftJSON, err := json.Marshal(epath093AuditRelations(left))
	if err != nil {
		t.Fatal(err)
	}
	rightJSON, err := json.Marshal(epath093AuditRelations(right))
	if err != nil {
		t.Fatal(err)
	}
	if string(leftJSON) != string(rightJSON) {
		t.Errorf("%s correspondence depends on input ordering\nforward: %s\nreverse: %s", label, leftJSON, rightJSON)
	}
}

func epath093AuditRelations(links []EnergyPathLink) []EnergyPathLink {
	out := make([]EnergyPathLink, 0, 2)
	for _, link := range links {
		if link.Relation == "source_correspondence" {
			out = append(out, link)
		}
	}
	return out
}

func epath093AuditRelation(links []EnergyPathLink, endUse string) *EnergyPathLink {
	needle := ".internal." + endUse + "."
	for index := range links {
		if links[index].Relation == "source_correspondence" && strings.Contains(links[index].FromID, needle) {
			return &links[index]
		}
	}
	return nil
}

func epath093AuditRelationForIDs(links []EnergyPathLink, fromID string, toID string) *EnergyPathLink {
	for index := range links {
		if links[index].Relation == "source_correspondence" && links[index].FromID == fromID && links[index].ToID == toID {
			return &links[index]
		}
	}
	return nil
}

func epath093AuditZoneResult(zones []EnergyExplanationZoneResult, zoneName string) *EnergyExplanationZoneResult {
	for index := range zones {
		if strings.EqualFold(zones[index].Scope.ZoneName, zoneName) {
			return &zones[index]
		}
	}
	return nil
}

func epath093AuditLegacy(reverse bool) EnergyExplanationV1 {
	type pair struct {
		zone       string
		family     string
		rawEndUse  string
		service    string
		thermal    float64
		site       float64
		heatName   string
		energyName string
	}
	pairs := []pair{
		{zone: "Office", family: "lighting", rawEndUse: "interior_lighting", service: "cooling", thermal: 8, site: 10, heatName: "Office Zone Lights Total Heating Energy", energyName: "Office InteriorLights:Electricity"},
		{zone: "Lab", family: "lighting", rawEndUse: "interior_lights", service: "cooling", thermal: 12, site: 20, heatName: "Lab Zone Lights Total Heating Energy", energyName: "Lab InteriorLights:Electricity"},
		{zone: "Office", family: "equipment", rawEndUse: "interior_equipment", service: "heating", thermal: 4, site: 6, heatName: "Office Zone Electric Equipment Total Heating Energy", energyName: "Office InteriorEquipment:Electricity"},
		{zone: "Lab", family: "equipment", rawEndUse: "interiorequipment", service: "heating", thermal: 6, site: 8, heatName: "Lab Zone Electric Equipment Total Heating Energy", energyName: "Lab InteriorEquipment:Electricity"},
	}

	buildGraph := func(period string, factor float64) ([]EnergyExplanationNode, []EnergyExplanationEdge) {
		nodes := make([]EnergyExplanationNode, 0, len(pairs)*2+4)
		edges := make([]EnergyExplanationEdge, 0, len(pairs)+4)
		ids := map[string]string{}
		for _, item := range pairs {
			zone := strings.ToLower(item.zone)
			endUseID := fmt.Sprintf("legacy.enduse.%s.%s", item.family, zone)
			driverID := fmt.Sprintf("legacy.driver.%s.%s", item.family, zone)
			energySource := fmt.Sprintf("meter.%s.%s", item.family, zone)
			heatSource := fmt.Sprintf("heat.%s.%s", item.family, zone)
			nodes = append(nodes,
				EnergyExplanationNode{
					ID: endUseID, Level: "energy", Kind: "energy." + item.rawEndUse, Label: item.family + " energy",
					Value: item.site * factor, Unit: "kWh site", Period: period, ZoneName: item.zone,
					Carrier: "electricity", EndUse: item.rawEndUse, MeterHierarchyLevel: "zone_direct_use",
					Basis: "measured_energy_variable", SourceIDs: []string{energySource},
				},
				EnergyExplanationNode{
					ID: driverID, Level: "heat", Kind: "heat." + item.family, Label: item.family + " heat",
					Value: item.thermal * factor, SignedValue: item.thermal * factor, DisplayValue: item.thermal * factor,
					Unit: "kWh thermal", Period: period, ZoneName: item.zone, ServiceKind: item.service,
					DriverCategory: "internal." + item.family, HeatCategory: "internal_gains", Basis: "derived_balance",
					SourceIDs: []string{heatSource},
				},
			)
			edges = append(edges, EnergyExplanationEdge{
				ID:     "correspondence." + item.family + "." + zone + "." + period,
				FromID: endUseID, ToID: driverID, Value: item.thermal * factor, SignedValue: item.thermal * factor,
				DisplayValue: item.thermal * factor, Unit: "kWh thermal", Period: period, ZoneName: item.zone,
				ServiceKind: item.service, Relation: "internal_gain_heat", Basis: "measured_meter_plus_zone_gain_variable",
				Formula: "independently reported direct-use energy and thermal effect", RuleID: energyRelationshipRuleInternalGainHeat,
				SourceIDs: []string{energySource, heatSource},
			})
			ids[item.family+"."+zone+".enduse"] = endUseID
			ids[item.family+"."+zone+".driver"] = driverID
		}

		// People is a thermal driver only. The deliberately malformed stored-v1
		// end use must be suppressed rather than merely renamed Other. A crossed
		// Equipment -> Lighting edge must also be rejected.
		for _, zoneName := range []string{"Office", "Lab"} {
			zone := strings.ToLower(zoneName)
			peopleDriverID := "legacy.driver.people." + zone
			peopleEndUseID := "legacy.enduse.people." + zone
			nodes = append(nodes,
				EnergyExplanationNode{ID: peopleDriverID, Level: "heat", Kind: "heat.people", Label: "People", Value: 2 * factor, SignedValue: 2 * factor, DisplayValue: 2 * factor, Unit: "kWh thermal", Period: period, ZoneName: zoneName, ServiceKind: "cooling", DriverCategory: "internal.people", HeatCategory: "internal_gains", SourceIDs: []string{"heat.people." + zone}},
				EnergyExplanationNode{ID: peopleEndUseID, Level: "energy", Kind: "energy.people", Label: "People energy (malformed legacy)", Value: factor, Unit: "kWh site", Period: period, ZoneName: zoneName, Carrier: "electricity", EndUse: "people", MeterHierarchyLevel: "zone_direct_use", SourceIDs: []string{"meter.people." + zone}},
			)
			edges = append(edges,
				EnergyExplanationEdge{ID: "malformed.people." + zone + "." + period, FromID: peopleEndUseID, ToID: peopleDriverID, Value: 2 * factor, DisplayValue: 2 * factor, Unit: "kWh thermal", Period: period, ZoneName: zoneName, ServiceKind: "cooling", Relation: "internal_gain_heat", Basis: "measured_meter_plus_zone_gain_variable", RuleID: energyRelationshipRuleInternalGainHeat, SourceIDs: []string{"meter.people." + zone, "heat.people." + zone}},
				EnergyExplanationEdge{ID: "malformed.crossed." + zone + "." + period, FromID: ids["equipment."+zone+".enduse"], ToID: ids["lighting."+zone+".driver"], Value: factor, DisplayValue: factor, Unit: "kWh thermal", Period: period, ZoneName: zoneName, ServiceKind: "cooling", Relation: "internal_gain_heat", Basis: "measured_meter_plus_zone_gain_variable", RuleID: energyRelationshipRuleInternalGainHeat},
			)
		}
		if reverse {
			slices.Reverse(nodes)
			slices.Reverse(edges)
			for index := range edges {
				slices.Reverse(edges[index].SourceIDs)
			}
		}
		return nodes, edges
	}

	annualNodes, annualEdges := buildGraph("annual", 1)
	m1Nodes, m1Edges := buildGraph("M1", 0.25)
	m2Nodes, m2Edges := buildGraph("M2", 0.75)
	periods := []EnergyPeriod{
		{ID: "annual", Label: "Annual", Kind: "annual", Nodes: annualNodes, Edges: annualEdges},
		{ID: "M1", Label: "January", Kind: "monthly", Nodes: m1Nodes, Edges: m1Edges},
		{ID: "M2", Label: "February", Kind: "monthly", Nodes: m2Nodes, Edges: m2Edges},
	}
	if reverse {
		slices.Reverse(periods)
	}

	sources := make([]EnergyDataSource, 0, len(pairs)*2+4)
	for _, item := range pairs {
		zone := strings.ToLower(item.zone)
		sources = append(sources,
			EnergyDataSource{ID: "meter." + item.family + "." + zone, SourceType: "sql_variable", Name: item.energyName, ZoneName: item.zone, Units: "J", NormalizedUnit: "kWh"},
			EnergyDataSource{ID: "heat." + item.family + "." + zone, SourceType: "sql_variable", Name: item.heatName, ZoneName: item.zone, Units: "J", NormalizedUnit: "kWh", DriverRole: energyDriverSourceRoleMainFlow, DriverCategory: "internal." + item.family, DriverComponent: item.family + ".total", InspectorSection: energyDriverInspectorSectionBreakdown},
		)
	}
	for _, zoneName := range []string{"Office", "Lab"} {
		zone := strings.ToLower(zoneName)
		sources = append(sources,
			EnergyDataSource{ID: "heat.people." + zone, SourceType: "sql_variable", Name: zoneName + " Zone People Convective Heating Energy", ZoneName: zoneName, DriverRole: energyDriverSourceRoleMainFlow, DriverCategory: "internal.people"},
			EnergyDataSource{ID: "meter.people." + zone, SourceType: "stored_v1", Name: zoneName + " malformed People energy", ZoneName: zoneName},
		)
	}
	if reverse {
		slices.Reverse(sources)
	}

	return EnergyExplanationV1{
		Schema:                energyExplanationV1Schema,
		Purpose:               string(SimulationPurposeBasicEnergy),
		Frequency:             "monthly",
		AllocationPolicy:      PurposeAllocationPolicyDirectOnly,
		RelationshipRules:     energyRelationshipRuleCatalog(),
		Periods:               periods,
		Nodes:                 annualNodes,
		Edges:                 annualEdges,
		Sources:               sources,
		Completeness:          EnergyCompleteness{Status: "complete"},
		scope:                 EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
		canonicalMonthlyBasis: true,
	}
}
