package simulation

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestEPATH093SourceCorrespondenceIsForwardNonFlowAndBounded(t *testing.T) {
	input := epath093BackendBuildingFixture()
	result := UpgradeEnergyExplanationV1(input)

	lighting := epath093BackendLink(result.Links, "driver.internal.lighting.cooling.building", "end_use.lighting.building", energyPathRelationSourceCorrespondence)
	equipment := epath093BackendLink(result.Links, "driver.internal.equipment.cooling.building", "end_use.equipment.building", energyPathRelationSourceCorrespondence)
	if lighting == nil || equipment == nil {
		t.Fatalf("missing direct-use correspondences: lighting=%#v equipment=%#v links=%#v", lighting, equipment, result.Links)
	}
	if lighting.FromValue != 10 || lighting.ToValue != 30 || lighting.FromUnit != "kWh thermal" || lighting.ToUnit != "kWh site" {
		t.Errorf("lighting correspondence endpoints = %#v, want independently measured 10 thermal / 30 site", lighting)
	}
	if equipment.FromValue != 8 || equipment.ToValue != 20 {
		t.Errorf("equipment correspondence endpoints = %#v, want independently measured 8 thermal / 20 site", equipment)
	}
	for name, relation := range map[string]*EnergyPathLink{"lighting": lighting, "equipment": equipment} {
		if relation.Ratio != 0 || relation.RatioKind != "" || relation.RatioLabel != "" {
			t.Errorf("%s correspondence was exposed as a conversion ratio: %#v", name, relation)
		}
		if relation.Basis != "derived_ratio" || relation.RuleID != energyRelationshipRuleInternalGainHeat {
			t.Errorf("%s correspondence traceability = %#v", name, relation)
		}
		if !sort.StringsAreSorted(relation.SourceIDs) {
			t.Errorf("%s correspondence provenance is not deterministic: %#v", name, relation.SourceIDs)
		}
	}
	for _, sourceID := range []string{"edge.lighting.office", "edge.lighting.lab", "meter.lighting", "thermal.lighting.office", "thermal.lighting.lab"} {
		if !stringSliceContains(lighting.SourceIDs, sourceID) {
			t.Errorf("lighting correspondence lost provenance %q: %#v", sourceID, lighting.SourceIDs)
		}
	}

	for _, link := range result.Links {
		from := epath093BackendNode(result.Nodes, link.FromID)
		to := epath093BackendNode(result.Nodes, link.ToID)
		if from == nil || to == nil {
			t.Fatalf("link %q has missing endpoint: %#v", link.ID, link)
		}
		if from.Level != "residual" && epath093BackendStage(from.Level) > epath093BackendStage(to.Level) {
			t.Errorf("backward link in canonical graph: %#v (%s -> %s)", link, from.Level, to.Level)
		}
		if from.Level == "residual" && (link.Relation != "residual" || to.Level != "carrier" || link.ToValue <= 0) {
			t.Errorf("invalid positive carrier residual branch: %#v", link)
		}
		if link.Relation != energyPathRelationSourceCorrespondence &&
			((link.FromID == lighting.FromID && link.ToID == lighting.ToID) || (link.FromID == equipment.FromID && link.ToID == equipment.ToID)) {
			t.Errorf("direct-use counterpart pair leaked into main flow: %#v", link)
		}
	}
	if epath093BackendHasCycle(result.Nodes, result.Links) {
		t.Fatalf("canonical graph contains a cycle: %#v", result.Links)
	}
	for _, node := range result.Nodes {
		if node.Level == "end_use" && (canonicalEnergyPathPart(node.EndUse) == "people" || strings.Contains(node.ID, ".people.")) {
			t.Errorf("People must not become a direct site-energy end use: %#v", node)
		}
	}
	for _, link := range result.Links {
		if link.Relation == energyPathRelationSourceCorrespondence && (strings.Contains(link.FromID, ".people.") || strings.Contains(link.ToID, ".people.")) {
			t.Errorf("People must not gain a source correspondence: %#v", link)
		}
	}

	carrierReconciliation := epath093BackendCarrierReconciliation(result.Reconciliation, "electricity")
	if carrierReconciliation == nil || carrierReconciliation.ExpectedValue != 55 || carrierReconciliation.ExplainedValue != 50 || carrierReconciliation.ResidualValue != 5 {
		t.Errorf("source correspondence contaminated carrier closure: %#v", carrierReconciliation)
	}
	for _, ratio := range buildEnergyExplanationSummary(result).Ratios {
		if strings.Contains(ratio.Kind, "source_correspondence") || strings.Contains(ratio.ID, "source_correspondence") {
			t.Errorf("source correspondence contaminated ratio summary: %#v", ratio)
		}
	}
}

func TestEPATH093CorrespondenceSurvivesZoneScopeAndOriginalSources(t *testing.T) {
	input := EnergyExplanationV1{
		Schema:    energyExplanationV1Schema,
		Purpose:   string(SimulationPurposeBasicEnergy),
		Frequency: "annual",
		scope: EnergyExplanationScope{
			Kind:             "zone",
			ZoneName:         "Office",
			AggregationBasis: "model_total",
		},
		Nodes: []EnergyExplanationNode{
			{ID: "energy.lighting.office", Level: "energy", Kind: "energy.interior_lighting", Label: "Office lights", Value: 12, Unit: "kWh site", ZoneName: "Office", Carrier: "electricity", EndUse: "interior_lighting", MeterHierarchyLevel: "zone_direct_use", SourceIDs: []string{"meter.zone.lighting"}},
			{ID: "heat.lighting.office", Level: "heat", Kind: "heat.lighting", Label: "Lighting heat", Value: 9, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "cooling", DriverCategory: energyDriverCategoryLighting, SourceIDs: []string{"variable.zone.lighting.heat"}},
		},
		Edges: []EnergyExplanationEdge{
			{ID: "edge.zone.lighting", FromID: "energy.lighting.office", ToID: "heat.lighting.office", Relation: "internal_gain_heat", Basis: "measured_meter_plus_zone_gain_variable", RuleID: energyRelationshipRuleInternalGainHeat, SourceIDs: []string{"relation.zone.lighting"}, ZoneName: "Office", ServiceKind: "cooling"},
		},
		Sources: []EnergyDataSource{
			{ID: "meter.zone.lighting", SourceType: "sql_variable", Name: "Zone Lights Electricity Energy", KeyValue: "Office", ZoneName: "Office"},
			{ID: "variable.zone.lighting.heat", SourceType: "sql_variable", Name: "Zone Lights Convective Heating Energy", KeyValue: "Office", ZoneName: "Office"},
			{ID: "relation.zone.lighting", SourceType: "relationship", Name: "Lighting measured-source correspondence", KeyValue: "Office", ZoneName: "Office"},
		},
	}

	result := UpgradeEnergyExplanationV1(input)
	relation := epath093BackendLink(result.Links, "driver.internal.lighting.cooling.office", "end_use.lighting.office", energyPathRelationSourceCorrespondence)
	if relation == nil || relation.ZoneName != "Office" || relation.FromValue != 9 || relation.ToValue != 12 {
		t.Fatalf("zone correspondence = %#v; links=%#v", relation, result.Links)
	}
	for _, sourceID := range []string{"meter.zone.lighting", "variable.zone.lighting.heat", "relation.zone.lighting"} {
		if !stringSliceContains(relation.SourceIDs, sourceID) {
			t.Errorf("zone correspondence lost source %q: %#v", sourceID, relation.SourceIDs)
		}
	}
	for sourceID, wantName := range map[string]string{
		"meter.zone.lighting":         "Zone Lights Electricity Energy",
		"variable.zone.lighting.heat": "Zone Lights Convective Heating Energy",
	} {
		source := energyExplanationSourceByID(result.Sources, sourceID)
		if source == nil || source.Name != wantName || source.KeyValue != "Office" {
			t.Errorf("original source metadata %q = %#v, want name %q and Office key", sourceID, source, wantName)
		}
	}
}

func TestEPATH093CorrespondenceMonthlyAnnualAndInputOrderInvariant(t *testing.T) {
	forwardInput := epath093BackendMonthlyFixture()
	reverseInput := epath093BackendMonthlyFixture()
	epath093BackendReverse(reverseInput.Nodes)
	epath093BackendReverse(reverseInput.Edges)
	epath093BackendReverse(reverseInput.Periods)
	for index := range reverseInput.Periods {
		epath093BackendReverse(reverseInput.Periods[index].Nodes)
		epath093BackendReverse(reverseInput.Periods[index].Edges)
	}

	forward := UpgradeEnergyExplanationV1(forwardInput)
	reverse := UpgradeEnergyExplanationV1(reverseInput)
	if got, want := epath093BackendCorrespondenceSnapshot(forward), epath093BackendCorrespondenceSnapshot(reverse); !reflect.DeepEqual(got, want) {
		t.Fatalf("correspondence depends on input order:\nforward=%#v\nreverse=%#v", got, want)
	}

	annual := epath093BackendLink(forward.Links, "driver.internal.lighting.cooling.building", "end_use.lighting.building", energyPathRelationSourceCorrespondence)
	if annual == nil || annual.Period != "annual" || annual.FromValue != 25 || annual.ToValue != 70 {
		t.Fatalf("annual correspondence = %#v, want endpoint totals 25 thermal / 70 site", annual)
	}
	for _, test := range []struct {
		period string
		from   float64
		to     float64
	}{{period: "M1", from: 10, to: 30}, {period: "M2", from: 15, to: 40}} {
		period := energyExplanationPeriodByID(forward.Periods, test.period)
		if period == nil {
			t.Fatalf("missing %s period", test.period)
		}
		relation := epath093BackendLink(period.Links, "driver.internal.lighting.cooling.building", "end_use.lighting.building", energyPathRelationSourceCorrespondence)
		if relation == nil || relation.FromValue != test.from || relation.ToValue != test.to || relation.Ratio != 0 || relation.RatioKind != "" {
			t.Errorf("%s correspondence = %#v", test.period, relation)
		}
	}
	// Two zone-specific thermal edges point at one Building end-use endpoint.
	// The non-flow relation must report the 70 kWh endpoint once, not 140 kWh.
	if annual.ToValue != epath093BackendNode(forward.Nodes, annual.ToID).Value {
		t.Errorf("annual correspondence was additively duplicated: relation=%#v endpoint=%#v", annual, epath093BackendNode(forward.Nodes, annual.ToID))
	}
}

func TestEPATH093AnnualOnlyEndUseFallbackRebindsCorrespondenceToMonthlyDriver(t *testing.T) {
	monthlyDriver := func(period string, value float64, sourceID string) EnergyPeriod {
		return EnergyPeriod{
			ID:    period,
			Label: period,
			Kind:  "monthly",
			Nodes: []EnergyExplanationNode{{
				ID:             "heat.lighting",
				Level:          "heat",
				Kind:           "heat.lighting",
				Label:          "Lighting heat",
				Value:          value,
				Unit:           "kWh thermal",
				Period:         period,
				ServiceKind:    "cooling",
				DriverCategory: energyDriverCategoryLighting,
				SourceIDs:      []string{sourceID},
			}},
		}
	}
	input := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "monthly",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.lighting", Level: "energy", Kind: "energy.interior_lighting", Label: "Interior lighting", Value: 70, Unit: "kWh site", Period: "annual", Carrier: "electricity", EndUse: "interior_lighting", SourceIDs: []string{"meter.lighting.annual"}},
			{ID: "heat.lighting", Level: "heat", Kind: "heat.lighting", Label: "Lighting heat", Value: 99, Unit: "kWh thermal", Period: "annual", ServiceKind: "cooling", DriverCategory: energyDriverCategoryLighting, SourceIDs: []string{"heat.lighting.raw_annual"}},
		},
		Edges: []EnergyExplanationEdge{{
			ID:          "correspond.lighting.annual",
			FromID:      "energy.lighting",
			ToID:        "heat.lighting",
			Period:      "annual",
			Relation:    "internal_gain_heat",
			Basis:       "measured_meter_plus_zone_gain_variable",
			Formula:     "independently reported direct-use energy and thermal effect",
			RuleID:      energyRelationshipRuleInternalGainHeat,
			SourceIDs:   []string{"relation.lighting.annual"},
			ServiceKind: "cooling",
		}},
		Periods: []EnergyPeriod{
			monthlyDriver("M1", 10, "heat.lighting.m1"),
			monthlyDriver("M2", 15, "heat.lighting.m2"),
		},
		Sources: []EnergyDataSource{
			{ID: "meter.lighting.annual", SourceType: "sql_meter", Name: "InteriorLights:Electricity"},
			{ID: "heat.lighting.raw_annual", SourceType: "sql_variable", Name: "Annual raw lighting heat", DriverRole: energyDriverSourceRoleMainFlow, DriverCategory: energyDriverCategoryLighting},
			{ID: "heat.lighting.m1", SourceType: "sql_variable", Name: "M1 lighting heat", DriverRole: energyDriverSourceRoleMainFlow, DriverCategory: energyDriverCategoryLighting},
			{ID: "heat.lighting.m2", SourceType: "sql_variable", Name: "M2 lighting heat", DriverRole: energyDriverSourceRoleMainFlow, DriverCategory: energyDriverCategoryLighting},
			{ID: "relation.lighting.annual", SourceType: "relationship", Name: "Lighting source correspondence"},
		},
		canonicalMonthlyBasis: true,
	}

	result := UpgradeEnergyExplanationV1(input)
	driver := epath093BackendNode(result.Nodes, "driver.internal.lighting.cooling.building")
	endUse := epath093BackendNode(result.Nodes, "end_use.lighting.building")
	relation := epath093BackendLink(result.Links, "driver.internal.lighting.cooling.building", "end_use.lighting.building", energyPathRelationSourceCorrespondence)
	if driver == nil || driver.Value != 25 || driver.Unit != "kWh thermal" {
		t.Fatalf("monthly-canonical annual driver = %#v, want 25 kWh thermal rather than raw annual 99", driver)
	}
	if endUse == nil || endUse.Value != 70 || endUse.Unit != "kWh site" {
		t.Fatalf("annual-only direct-use fallback = %#v, want 70 kWh site", endUse)
	}
	if relation == nil || relation.Period != "annual" || relation.FromValue != driver.Value || relation.FromUnit != driver.Unit || relation.ToValue != endUse.Value || relation.ToUnit != endUse.Unit {
		t.Fatalf("fallback correspondence was not rebound to final annual endpoints: relation=%#v driver=%#v endUse=%#v", relation, driver, endUse)
	}
	if relation.Ratio != 0 || relation.RatioKind != "" || relation.RatioLabel != "" {
		t.Errorf("fallback correspondence gained conversion metadata: %#v", relation)
	}
	for _, sourceID := range []string{"heat.lighting.m1", "heat.lighting.m2", "heat.lighting.raw_annual", "meter.lighting.annual", "relation.lighting.annual"} {
		if !stringSliceContains(relation.SourceIDs, sourceID) {
			t.Errorf("fallback correspondence lost provenance %q: %#v", sourceID, relation.SourceIDs)
		}
	}
	if !sort.StringsAreSorted(relation.SourceIDs) {
		t.Errorf("fallback correspondence provenance is not deterministic: %#v", relation.SourceIDs)
	}
}

func TestEPATH093V1InternalGainEdgeRemainsFrozen(t *testing.T) {
	legacy := buildEnergyExplanationResult([]energyExplanationSeries{
		{Level: "energy", Kind: "energy.interior_lighting", Label: "Interior lighting", Unit: "kWh", Carrier: "electricity", EndUse: "interior_lighting", MeterHierarchyLevel: "broad_end_use", SourceIDs: []string{"lighting-energy"}, Total: 15},
		{Level: "heat", Kind: "heat.lighting", Label: "Lighting heat", Unit: "kWh", ZoneName: "Office", HeatCategory: "internal_gains", SourceIDs: []string{"lighting-heat"}, Total: 12, sourceName: "Zone Lights Total Heating Energy", heatSignMultiplier: 1},
	}, nil, &PurposeRunPlan{})
	before, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	_ = UpgradeEnergyExplanationV1(legacy)
	after, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("v2 upgrade mutated the frozen v1 payload:\nbefore=%s\nafter=%s", before, after)
	}
	edge := energyExplanationEdgeByIDs(legacy.Edges, "energy.end_use.interior_lighting.electricity", "heat.lighting.office")
	if edge == nil || edge.Relation != "internal_gain_heat" || edge.RuleID != energyRelationshipRuleInternalGainHeat {
		t.Fatalf("v1 internal-gain edge changed: %#v", edge)
	}
}

func epath093BackendBuildingFixture() EnergyExplanationV1 {
	nodes := []EnergyExplanationNode{
		{ID: "energy.carrier.electricity", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Value: 55, Unit: "kWh site", Carrier: "electricity", EndUse: "total", SourceIDs: []string{"meter.facility"}},
		{ID: "energy.lighting", Level: "energy", Kind: "energy.interior_lighting", Label: "Interior lighting", Value: 30, Unit: "kWh site", Carrier: "electricity", EndUse: "interior_lighting", SourceIDs: []string{"meter.lighting"}},
		{ID: "energy.equipment", Level: "energy", Kind: "energy.interior_equipment", Label: "Interior equipment", Value: 20, Unit: "kWh site", Carrier: "electricity", EndUse: "interior_equipment", SourceIDs: []string{"meter.equipment"}},
		{ID: "energy.people", Level: "energy", Kind: "energy.people", Label: "People electricity", Value: 5, Unit: "kWh site", Carrier: "electricity", EndUse: "people", SourceIDs: []string{"meter.people"}},
		{ID: "load.office", Level: "load", Kind: "load.zone_cooling", Label: "Office cooling", Value: 20, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"load.office"}},
		{ID: "load.lab", Level: "load", Kind: "load.zone_cooling", Label: "Lab cooling", Value: 20, Unit: "kWh thermal", ZoneName: "Lab", ServiceKind: "cooling", SourceIDs: []string{"load.lab"}},
		{ID: "heat.lighting.office", Level: "heat", Kind: "heat.lighting", Label: "Lighting heat", Value: 6, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "cooling", DriverCategory: energyDriverCategoryLighting, SourceIDs: []string{"thermal.lighting.office"}},
		{ID: "heat.lighting.lab", Level: "heat", Kind: "heat.lighting", Label: "Lighting heat", Value: 4, Unit: "kWh thermal", ZoneName: "Lab", ServiceKind: "cooling", DriverCategory: energyDriverCategoryLighting, SourceIDs: []string{"thermal.lighting.lab"}},
		{ID: "heat.equipment.office", Level: "heat", Kind: "heat.equipment", Label: "Equipment heat", Value: 8, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "cooling", DriverCategory: energyDriverCategoryEquipment, SourceIDs: []string{"thermal.equipment.office"}},
		{ID: "heat.people.office", Level: "heat", Kind: "heat.people", Label: "People heat", Value: 7, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "cooling", DriverCategory: energyDriverCategoryPeople, SourceIDs: []string{"thermal.people.office"}},
	}
	edges := []EnergyExplanationEdge{
		{ID: "meter.lighting.edge", FromID: nodes[0].ID, ToID: nodes[1].ID, Value: 30, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.lighting"}},
		{ID: "meter.equipment.edge", FromID: nodes[0].ID, ToID: nodes[2].ID, Value: 20, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.equipment"}},
		{ID: "meter.people.edge", FromID: nodes[0].ID, ToID: nodes[3].ID, Value: 5, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.people"}},
		{ID: "flow.lighting.office", FromID: nodes[4].ID, ToID: nodes[6].ID, Value: 6, Unit: "kWh thermal", Relation: "heat_driver", Basis: "heat_balance_share", SourceIDs: []string{"thermal.lighting.office"}},
		{ID: "flow.lighting.lab", FromID: nodes[5].ID, ToID: nodes[7].ID, Value: 4, Unit: "kWh thermal", Relation: "heat_driver", Basis: "heat_balance_share", SourceIDs: []string{"thermal.lighting.lab"}},
		{ID: "flow.equipment.office", FromID: nodes[4].ID, ToID: nodes[8].ID, Value: 8, Unit: "kWh thermal", Relation: "heat_driver", Basis: "heat_balance_share", SourceIDs: []string{"thermal.equipment.office"}},
		{ID: "flow.people.office", FromID: nodes[4].ID, ToID: nodes[9].ID, Value: 7, Unit: "kWh thermal", Relation: "heat_driver", Basis: "heat_balance_share", SourceIDs: []string{"thermal.people.office"}},
		{ID: "correspond.lighting.office", FromID: nodes[1].ID, ToID: nodes[6].ID, Relation: "internal_gain_heat", Basis: "measured_meter_plus_zone_gain_variable", RuleID: energyRelationshipRuleInternalGainHeat, SourceIDs: []string{"edge.lighting.office"}},
		{ID: "correspond.lighting.lab", FromID: nodes[1].ID, ToID: nodes[7].ID, Relation: "internal_gain_heat", Basis: "measured_meter_plus_zone_gain_variable", RuleID: energyRelationshipRuleInternalGainHeat, SourceIDs: []string{"edge.lighting.lab"}},
		{ID: "correspond.equipment.office", FromID: nodes[2].ID, ToID: nodes[8].ID, Relation: "source_correspondence", Basis: "derived_ratio", RuleID: energyRelationshipRuleInternalGainHeat, SourceIDs: []string{"edge.equipment.office"}},
		{ID: "correspond.people.office", FromID: nodes[3].ID, ToID: nodes[9].ID, Relation: "internal_gain_heat", Basis: "measured_meter_plus_zone_gain_variable", RuleID: energyRelationshipRuleInternalGainHeat, SourceIDs: []string{"edge.people.office"}},
	}
	return EnergyExplanationV1{
		Schema:            energyExplanationV1Schema,
		Purpose:           string(SimulationPurposeBasicEnergy),
		Frequency:         "annual",
		AllocationPolicy:  PurposeAllocationPolicyDirectOnly,
		Nodes:             nodes,
		Edges:             edges,
		RelationshipRules: energyRelationshipRuleCatalog(),
	}
}

func epath093BackendMonthlyFixture() EnergyExplanationV1 {
	m1 := epath093BackendPeriod("M1", 30, 6, 4, "m1")
	m2 := epath093BackendPeriod("M2", 40, 9, 6, "m2")
	return EnergyExplanationV1{
		Schema:                energyExplanationV1Schema,
		Purpose:               string(SimulationPurposeBasicEnergy),
		Frequency:             "monthly",
		AllocationPolicy:      PurposeAllocationPolicyDirectOnly,
		Periods:               []EnergyPeriod{m1, m2},
		canonicalMonthlyBasis: true,
	}
}

func epath093BackendPeriod(id string, endUseValue float64, officeHeat float64, labHeat float64, sourcePrefix string) EnergyPeriod {
	nodes := []EnergyExplanationNode{
		{ID: "energy.lighting", Level: "energy", Kind: "energy.interior_lighting", Label: "Interior lighting", Value: endUseValue, Unit: "kWh site", Carrier: "electricity", EndUse: "interior_lighting", SourceIDs: []string{"meter.lighting." + sourcePrefix}},
		{ID: "heat.lighting.office", Level: "heat", Kind: "heat.lighting", Label: "Lighting heat", Value: officeHeat, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "cooling", DriverCategory: energyDriverCategoryLighting, SourceIDs: []string{"thermal.lighting.office." + sourcePrefix}},
		{ID: "heat.lighting.lab", Level: "heat", Kind: "heat.lighting", Label: "Lighting heat", Value: labHeat, Unit: "kWh thermal", ZoneName: "Lab", ServiceKind: "cooling", DriverCategory: energyDriverCategoryLighting, SourceIDs: []string{"thermal.lighting.lab." + sourcePrefix}},
	}
	edges := []EnergyExplanationEdge{
		{ID: "correspond.office." + sourcePrefix, FromID: nodes[0].ID, ToID: nodes[1].ID, Relation: "internal_gain_heat", Basis: "measured_meter_plus_zone_gain_variable", RuleID: energyRelationshipRuleInternalGainHeat, SourceIDs: []string{"relation.office." + sourcePrefix}},
		{ID: "correspond.lab." + sourcePrefix, FromID: nodes[0].ID, ToID: nodes[2].ID, Relation: "internal_gain_heat", Basis: "measured_meter_plus_zone_gain_variable", RuleID: energyRelationshipRuleInternalGainHeat, SourceIDs: []string{"relation.lab." + sourcePrefix}},
	}
	return EnergyPeriod{ID: id, Label: id, Kind: "monthly", Nodes: nodes, Edges: edges}
}

type epath093BackendRelationSnapshot struct {
	Period      string
	ID          string
	FromID      string
	ToID        string
	FromValue   float64
	ToValue     float64
	FromUnit    string
	ToUnit      string
	SourceIDs   []string
	RelatedPath []string
}

func epath093BackendCorrespondenceSnapshot(result EnergyExplanationResult) []epath093BackendRelationSnapshot {
	out := []epath093BackendRelationSnapshot{}
	appendLinks := func(period string, links []EnergyPathLink) {
		for _, link := range links {
			if link.Relation != energyPathRelationSourceCorrespondence {
				continue
			}
			sources := append([]string(nil), link.SourceIDs...)
			paths := append([]string(nil), link.RelatedPathIDs...)
			sort.Strings(sources)
			sort.Strings(paths)
			out = append(out, epath093BackendRelationSnapshot{Period: period, ID: link.ID, FromID: link.FromID, ToID: link.ToID, FromValue: link.FromValue, ToValue: link.ToValue, FromUnit: link.FromUnit, ToUnit: link.ToUnit, SourceIDs: sources, RelatedPath: paths})
		}
	}
	appendLinks("annual", result.Links)
	for _, period := range result.Periods {
		appendLinks(period.ID, period.Links)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Period+"|"+out[i].ID < out[j].Period+"|"+out[j].ID
	})
	return out
}

func epath093BackendLink(links []EnergyPathLink, fromID string, toID string, relation string) *EnergyPathLink {
	for index := range links {
		if links[index].FromID == fromID && links[index].ToID == toID && links[index].Relation == relation {
			return &links[index]
		}
	}
	return nil
}

func epath093BackendNode(nodes []EnergyExplanationNode, id string) *EnergyExplanationNode {
	for index := range nodes {
		if nodes[index].ID == id {
			return &nodes[index]
		}
	}
	return nil
}

func epath093BackendStage(level string) int {
	switch level {
	case "driver":
		return 0
	case "load":
		return 1
	case "end_use":
		return 2
	case "carrier":
		return 3
	case "support", "residual":
		return 4
	default:
		return 99
	}
}

func epath093BackendHasCycle(nodes []EnergyExplanationNode, links []EnergyPathLink) bool {
	known := map[string]bool{}
	indegree := map[string]int{}
	outgoing := map[string][]string{}
	for _, node := range nodes {
		known[node.ID] = true
		indegree[node.ID] = 0
	}
	for _, link := range links {
		if !known[link.FromID] || !known[link.ToID] {
			continue
		}
		outgoing[link.FromID] = append(outgoing[link.FromID], link.ToID)
		indegree[link.ToID]++
	}
	queue := []string{}
	for id, degree := range indegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, target := range outgoing[id] {
			indegree[target]--
			if indegree[target] == 0 {
				queue = append(queue, target)
			}
		}
	}
	return visited != len(known)
}

func epath093BackendCarrierReconciliation(items []EnergyReconciliation, carrier string) *EnergyReconciliation {
	for index := range items {
		if strings.Contains(items[index].ID, "."+carrier+".") || strings.HasSuffix(items[index].ID, "."+carrier) {
			return &items[index]
		}
	}
	return nil
}

func epath093BackendReverse[T any](items []T) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}
