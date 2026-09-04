package simulation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestEPATH091AuditFixedTaxonomyCollapsesStoredUnknownsWithoutLoss(t *testing.T) {
	legacy := epath091AuditFixture(false)
	result := UpgradeEnergyExplanationV1(legacy)

	wantValues := map[string]float64{
		"cooling":        4,
		"heating":        88,
		"fans":           12,
		"pumps":          16,
		"heat_rejection": 20,
		"humidification": 24,
		"heat_recovery":  28,
		"lighting":       68,
		"equipment":      84,
		"water_systems":  48,
		"refrigeration":  52,
		"other":          248,
	}
	endUses := epath091AuditEndUses(result.Nodes)
	if len(endUses) != len(wantValues) {
		t.Fatalf("fixed taxonomy emitted %d end-use nodes; want exactly %d: %#v", len(endUses), len(wantValues), endUses)
	}
	for token, wantValue := range wantValues {
		id := "end_use." + token + ".building"
		node := energyPathV2NodeByID(result.Nodes, id)
		if node == nil {
			t.Fatalf("missing fixed taxonomy token %q in %#v", token, endUses)
		}
		if node.EndUse != token || node.Kind != "energy."+token || node.Value != wantValue || node.RawValue != wantValue || node.EffectiveValue != wantValue || node.AllocatedValue != wantValue {
			t.Fatalf("canonical node %q = %#v; want token=%q value=%g", id, node, token, wantValue)
		}
		if node.Carrier != "" || node.Level != "end_use" || node.ScaleDomain != "site" || node.Value == 0 {
			t.Fatalf("canonical node %q is not a non-zero carrier-neutral site end use: %#v", id, node)
		}
	}

	for _, forbidden := range []string{
		"end_use.interior_lighting.building",
		"end_use.exterior_lighting.building",
		"end_use.interior_equipment.building",
		"end_use.exterior_equipment.building",
		"end_use.server_rack_ups.building",
		"end_use.process_steam_skid.building",
		"end_use.data_hall_gas_train.building",
		"end_use.zero_process.building",
	} {
		if energyPathV2NodeByID(result.Nodes, forbidden) != nil {
			t.Fatalf("non-taxonomy or zero end-use node leaked into v2: %s", forbidden)
		}
	}

	epath091AuditAssertSourceSet(t, energyPathV2NodeByID(result.Nodes, "end_use.lighting.building").SourceIDs, []string{
		"meter.interior_lighting.electricity",
		"meter.exterior_lighting.electricity",
	})
	epath091AuditAssertSourceSet(t, energyPathV2NodeByID(result.Nodes, "end_use.equipment.building").SourceIDs, []string{
		"meter.interior_equipment.electricity",
		"meter.exterior_equipment.electricity",
	})
	other := energyPathV2NodeByID(result.Nodes, "end_use.other.building")
	epath091AuditAssertSourceSet(t, other.SourceIDs, []string{
		"meter.other.electricity",
		"meter.server_rack_ups.electricity",
		"meter.process_steam_skid.electricity",
		"meter.data_hall_gas_train.natural_gas",
	})

	electricity := energyPathV2LinkByIDs(result.Links, other.ID, "carrier.electricity.building")
	naturalGas := energyPathV2LinkByIDs(result.Links, other.ID, "carrier.natural_gas.building")
	if electricity == nil || electricity.Relation != "direct_end_use_to_carrier" || electricity.FromValue != 180 || electricity.ToValue != 180 {
		t.Fatalf("merged Other -> electricity split = %#v", electricity)
	}
	if naturalGas == nil || naturalGas.Relation != "direct_end_use_to_carrier" || naturalGas.FromValue != 68 || naturalGas.ToValue != 68 {
		t.Fatalf("Other -> natural gas split = %#v", naturalGas)
	}
	epath091AuditAssertSourceSet(t, electricity.SourceIDs, []string{
		"meter.other.electricity",
		"meter.server_rack_ups.electricity",
		"meter.process_steam_skid.electricity",
	})
	epath091AuditAssertSourceSet(t, naturalGas.SourceIDs, []string{"meter.data_hall_gas_train.natural_gas"})
	if electricity.FromValue+naturalGas.FromValue != other.Value {
		t.Fatalf("Other carrier splits do not close: electricity=%g naturalGas=%g node=%g", electricity.FromValue, naturalGas.FromValue, other.Value)
	}

	for _, sourceID := range other.SourceIDs {
		want := epath091AuditSourceByID(legacy.Sources, sourceID)
		got := epath091AuditSourceByID(result.Sources, sourceID)
		if want == nil || got == nil || got.Name != want.Name || got.KeyValue != want.KeyValue {
			t.Fatalf("source meter identity changed for %q: before=%#v after=%#v", sourceID, want, got)
		}
	}
}

func TestEPATH091AuditManyStoredUnknownsStayBoundedAndMergePerCarrier(t *testing.T) {
	forward := epath091AuditUnknownFixture(false)
	reversed := epath091AuditUnknownFixture(true)
	got := UpgradeEnergyExplanationV1(forward)
	wantOrderInvariant := UpgradeEnergyExplanationV1(reversed)

	if snapshot, want := epath091AuditSnapshot(got), epath091AuditSnapshot(wantOrderInvariant); !reflect.DeepEqual(snapshot, want) {
		t.Fatalf("unknown taxonomy result depends on input node/edge order:\nforward=%#v\nreversed=%#v", snapshot, want)
	}
	endUses := epath091AuditEndUses(got.Nodes)
	if len(endUses) != 1 || endUses[0].ID != "end_use.other.building" || endUses[0].Value != 2080 {
		t.Fatalf("64 arbitrary stored end uses should collapse to one bounded Other node: %#v", endUses)
	}
	if len(endUses) > 12 {
		t.Fatalf("fixed taxonomy node bound exceeded: %d", len(endUses))
	}
	link := energyPathV2LinkByIDs(got.Links, "end_use.other.building", "carrier.electricity.building")
	if link == nil || link.FromValue != 2080 || link.ToValue != 2080 || len(link.SourceIDs) != 64 {
		t.Fatalf("many-unknown merged carrier split lost value or provenance: %#v", link)
	}
	if len(endUses[0].SourceIDs) != 64 {
		t.Fatalf("many-unknown Other node source union = %d; want 64", len(endUses[0].SourceIDs))
	}
	for _, node := range got.Nodes {
		if node.Level == "end_use" && node.Value == 0 {
			t.Fatalf("zero-valued end-use node must be absent: %#v", node)
		}
	}
	for _, link := range got.Links {
		if (link.Relation == "end_use_to_carrier" || link.Relation == "direct_end_use_to_carrier") && link.FromValue == 0 && link.ToValue == 0 {
			t.Fatalf("zero-valued end-use carrier link must be absent: %#v", link)
		}
	}
}

func TestEPATH091AuditZeroOnlyFixedEndUsesAreAbsent(t *testing.T) {
	tokens := []string{
		"cooling", "heating", "fans", "pumps", "heat_rejection", "humidification",
		"heat_recovery", "interior_lighting", "interior_equipment", "water_systems",
		"refrigeration", "other",
	}
	specs := make([]epath091AuditSeriesSpec, 0, len(tokens))
	for index, token := range tokens {
		specs = append(specs, epath091AuditSeriesSpec{
			token:    token,
			carrier:  "electricity",
			sourceID: "meter.zero." + token,
			name:     "Zero " + token + ":Electricity",
			keyValue: fmt.Sprintf("ZERO FIXED KEY %02d", index),
		})
	}
	result := UpgradeEnergyExplanationV1(epath091AuditLegacyFromSpecs(specs, false, false))
	if endUses := epath091AuditEndUses(result.Nodes); len(endUses) != 0 {
		t.Fatalf("zero-only fixed taxonomy nodes must be absent: %#v", endUses)
	}
	for _, link := range result.Links {
		if link.Relation == "end_use_to_carrier" || link.Relation == "direct_end_use_to_carrier" {
			t.Fatalf("zero-only fixed taxonomy link must be absent: %#v", link)
		}
	}
}

func TestEPATH091AuditMonthlyAnnualTaxonomyAndSourceIdentityAreInvariant(t *testing.T) {
	forward := epath091AuditFixture(false)
	reversed := epath091AuditFixture(true)
	pristine := epath091AuditFixture(false)
	result := UpgradeEnergyExplanationV1(forward)
	reordered := UpgradeEnergyExplanationV1(reversed)

	if !reflect.DeepEqual(forward, pristine) {
		t.Fatalf("v1 input mutated while building v2:\nafter=%#v\nwant=%#v", forward, pristine)
	}
	if got, want := epath091AuditSnapshot(result), epath091AuditSnapshot(reordered); !reflect.DeepEqual(got, want) {
		t.Fatalf("monthly/annual fixed taxonomy depends on input node/edge/source order:\nforward=%#v\nreversed=%#v", got, want)
	}

	m1 := energyExplanationPeriodByID(result.Periods, "M1")
	m2 := energyExplanationPeriodByID(result.Periods, "M2")
	if m1 == nil || m2 == nil {
		t.Fatalf("missing monthly graphs: M1=%#v M2=%#v", m1, m2)
	}
	for _, token := range []string{
		"cooling", "heating", "fans", "pumps", "heat_rejection", "humidification",
		"heat_recovery", "lighting", "equipment", "water_systems", "refrigeration", "other",
	} {
		id := "end_use." + token + ".building"
		annual := energyPathV2NodeByID(result.Nodes, id)
		month1 := energyPathV2NodeByID(m1.Nodes, id)
		month2 := energyPathV2NodeByID(m2.Nodes, id)
		if annual == nil || month1 == nil || month2 == nil || annual.Value != month1.Value+month2.Value {
			t.Fatalf("%s annual node is not the completed monthly sum: annual=%#v M1=%#v M2=%#v", token, annual, month1, month2)
		}
		if annual.Value != annual.RawValue || annual.Value != annual.EffectiveValue || annual.Value != annual.AllocatedValue {
			t.Fatalf("%s annual value channels diverged: %#v", token, annual)
		}
	}

	otherID := "end_use.other.building"
	for _, carrierID := range []string{"carrier.electricity.building", "carrier.natural_gas.building"} {
		annual := energyPathV2LinkByIDs(result.Links, otherID, carrierID)
		month1 := energyPathV2LinkByIDs(m1.Links, otherID, carrierID)
		month2 := energyPathV2LinkByIDs(m2.Links, otherID, carrierID)
		if annual == nil || month1 == nil || month2 == nil || annual.FromValue != month1.FromValue+month2.FromValue || annual.ToValue != month1.ToValue+month2.ToValue {
			t.Fatalf("Other split %s is not the monthly contribution sum: annual=%#v M1=%#v M2=%#v", carrierID, annual, month1, month2)
		}
	}

	for _, want := range pristine.Sources {
		got := epath091AuditSourceByID(result.Sources, want.ID)
		if got == nil || got.Name != want.Name || got.KeyValue != want.KeyValue {
			t.Fatalf("source Name/KeyValue changed for %q: got=%#v want=%#v", want.ID, got, want)
		}
	}
}

func TestEPATH091AuditLegacyCarrierQualifiedAndRawV2SummariesStayDistinct(t *testing.T) {
	legacy := epath091AuditFixture(false)
	rawV1 := buildEnergyExplanationSummaryV1(legacy)
	compat := buildEnergyExplanationSummary(legacy)
	if !reflect.DeepEqual(compat.EnergyByEndUse, rawV1.EnergyByEndUse) {
		t.Fatalf("v1 carrier-qualified compatibility summary changed:\ncompat=%#v\nraw=%#v", compat.EnergyByEndUse, rawV1.EnergyByEndUse)
	}
	for id, wantValue := range map[string]float64{
		"heating.electricity":             8,
		"heating.natural_gas":             80,
		"fans.electricity":                12,
		"pumps.electricity":               16,
		"interior_lighting.electricity":   32,
		"exterior_lighting.electricity":   36,
		"server_rack_ups.electricity":     60,
		"process_steam_skid.electricity":  64,
		"data_hall_gas_train.natural_gas": 68,
	} {
		item := epath091AuditSummaryItemByID(rawV1.EnergyByEndUse, id)
		if item == nil || item.ID != id || item.Value != wantValue {
			t.Fatalf("raw v1 carrier-qualified end use %q = %#v; want %g", id, item, wantValue)
		}
	}

	result := UpgradeEnergyExplanationV1(legacy)
	rawV2 := buildEnergyExplanationSummaryV2(result)
	fans := epath091AuditSummaryItemByID(rawV2.EndUses, "end_use.fans.building")
	pumps := epath091AuditSummaryItemByID(rawV2.EndUses, "end_use.pumps.building")
	if fans == nil || fans.EndUse != "fans" || fans.Value != 12 {
		t.Fatalf("raw v2 Fans summary = %#v", fans)
	}
	if pumps == nil || pumps.EndUse != "pumps" || pumps.Value != 16 {
		t.Fatalf("raw v2 Pumps summary = %#v", pumps)
	}
	if fans.ID == pumps.ID || epath091AuditSummaryItemByID(rawV2.EndUses, "end_use.fans_and_pumps.building") != nil {
		t.Fatalf("raw v2 summary prematurely combined Fans and Pumps: %#v", rawV2.EndUses)
	}
	if len(rawV2.EndUses) != 12 {
		t.Fatalf("raw v2 summary is not the fixed 12-token taxonomy: %#v", rawV2.EndUses)
	}
}

type epath091AuditSeriesSpec struct {
	token    string
	carrier  string
	value    float64
	sourceID string
	name     string
	keyValue string
}

func epath091AuditFixture(reverse bool) EnergyExplanationV1 {
	specs := []epath091AuditSeriesSpec{
		{token: "cooling", carrier: "electricity", value: 4},
		{token: "heating", carrier: "electricity", value: 8},
		{token: "fans", carrier: "electricity", value: 12},
		{token: "pumps", carrier: "electricity", value: 16},
		{token: "heat_rejection", carrier: "electricity", value: 20},
		{token: "humidification", carrier: "electricity", value: 24},
		{token: "heat_recovery", carrier: "electricity", value: 28},
		{token: "interior_lighting", carrier: "electricity", value: 32},
		{token: "exterior_lighting", carrier: "electricity", value: 36},
		{token: "interior_equipment", carrier: "electricity", value: 40},
		{token: "exterior_equipment", carrier: "electricity", value: 44},
		{token: "water_systems", carrier: "electricity", value: 48},
		{token: "refrigeration", carrier: "electricity", value: 52},
		{token: "other", carrier: "electricity", value: 56},
		{token: "server_rack_ups", carrier: "electricity", value: 60},
		{token: "process_steam_skid", carrier: "electricity", value: 64},
		{token: "heating", carrier: "natural_gas", value: 80},
		{token: "data_hall_gas_train", carrier: "natural_gas", value: 68},
		{token: "zero_process", carrier: "electricity", value: 0},
	}
	for index := range specs {
		if specs[index].sourceID == "" {
			specs[index].sourceID = "meter." + specs[index].token + "." + specs[index].carrier
		}
		if specs[index].name == "" {
			specs[index].name = strings.ToUpper(specs[index].token) + ":" + strings.ToUpper(specs[index].carrier)
		}
		if specs[index].keyValue == "" {
			specs[index].keyValue = fmt.Sprintf("ORIGINAL KEY / %02d / %s", index, specs[index].token)
		}
	}
	return epath091AuditLegacyFromSpecs(specs, reverse, true)
}

func epath091AuditUnknownFixture(reverse bool) EnergyExplanationV1 {
	specs := make([]epath091AuditSeriesSpec, 0, 67)
	for index := 0; index < 64; index++ {
		token := fmt.Sprintf("tenant_process_%02d", index)
		specs = append(specs, epath091AuditSeriesSpec{
			token:    token,
			carrier:  "electricity",
			value:    float64(index + 1),
			sourceID: "meter." + token + ".electricity",
			name:     fmt.Sprintf("Tenant Process %02d:Electricity", index),
			keyValue: fmt.Sprintf("ORIGINAL TENANT KEY %02d", index),
		})
	}
	for index := 0; index < 3; index++ {
		token := fmt.Sprintf("zero_unknown_%02d", index)
		specs = append(specs, epath091AuditSeriesSpec{
			token:    token,
			carrier:  "electricity",
			sourceID: "meter." + token + ".electricity",
			name:     fmt.Sprintf("Zero Unknown %02d:Electricity", index),
			keyValue: fmt.Sprintf("ZERO KEY %02d", index),
		})
	}
	return epath091AuditLegacyFromSpecs(specs, reverse, false)
}

func epath091AuditLegacyFromSpecs(specs []epath091AuditSeriesSpec, reverse bool, withMonths bool) EnergyExplanationV1 {
	carrierTotals := map[string]float64{}
	for _, spec := range specs {
		carrierTotals[spec.carrier] += spec.value
	}

	buildGraph := func(period string, factor float64) ([]EnergyExplanationNode, []EnergyExplanationEdge) {
		nodes := make([]EnergyExplanationNode, 0, len(specs)+len(carrierTotals))
		edges := make([]EnergyExplanationEdge, 0, len(specs))
		carriers := make([]string, 0, len(carrierTotals))
		for carrier := range carrierTotals {
			carriers = append(carriers, carrier)
		}
		sort.Strings(carriers)
		for _, carrier := range carriers {
			value := carrierTotals[carrier] * factor
			nodes = append(nodes, epath091AuditLegacyNode(
				"energy.carrier."+carrier, "energy."+carrier+".total", strings.ToUpper(carrier),
				carrier, "total", value, period, []string{"meter.facility." + carrier},
			))
		}
		for index, spec := range specs {
			value := spec.value * factor
			id := fmt.Sprintf("energy.end_use.%s.%s.%02d", spec.token, spec.carrier, index)
			nodes = append(nodes, epath091AuditLegacyNode(
				id, "energy."+spec.token, spec.name, spec.carrier, spec.token, value, period, []string{spec.sourceID},
			))
			edges = append(edges, EnergyExplanationEdge{
				ID:           fmt.Sprintf("edge.%s.%02d.%s", spec.token, index, period),
				FromID:       "energy.carrier." + spec.carrier,
				ToID:         id,
				Value:        value,
				DisplayValue: value,
				Unit:         "kWh",
				Period:       period,
				Relation:     "meter_enduse",
				Basis:        "measured_meter",
				RuleID:       energyRelationshipRuleMeterEndUse,
				SourceIDs:    []string{spec.sourceID},
			})
		}
		if reverse {
			epath090ReviewReverseNodes(nodes)
			epath090ReviewReverseEdges(edges)
		}
		return nodes, edges
	}

	annualNodes, annualEdges := buildGraph("annual", 1)
	periods := []EnergyPeriod{{ID: "annual", Label: "Annual", Kind: "annual", Nodes: annualNodes, Edges: annualEdges}}
	if withMonths {
		m1Nodes, m1Edges := buildGraph("M1", 0.25)
		m2Nodes, m2Edges := buildGraph("M2", 0.75)
		periods = append(periods,
			EnergyPeriod{ID: "M1", Label: "M1", Kind: "monthly", Nodes: m1Nodes, Edges: m1Edges},
			EnergyPeriod{ID: "M2", Label: "M2", Kind: "monthly", Nodes: m2Nodes, Edges: m2Edges},
		)
	}
	sources := make([]EnergyDataSource, 0, len(specs)+len(carrierTotals))
	carriers := make([]string, 0, len(carrierTotals))
	for carrier := range carrierTotals {
		carriers = append(carriers, carrier)
	}
	sort.Strings(carriers)
	for _, carrier := range carriers {
		sources = append(sources, EnergyDataSource{
			ID: "meter.facility." + carrier, SourceType: "sql_meter", IsMeter: true,
			Name: strings.ToUpper(carrier) + ":FACILITY", KeyValue: "ORIGINAL FACILITY KEY / " + carrier, Units: "kWh",
		})
	}
	for _, spec := range specs {
		sources = append(sources, EnergyDataSource{
			ID: spec.sourceID, SourceType: "sql_meter", IsMeter: true,
			Name: spec.name, KeyValue: spec.keyValue, Units: "kWh",
		})
	}
	if reverse {
		epath090ReviewReverseSources(sources)
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
		canonicalMonthlyBasis: withMonths,
	}
}

func epath091AuditLegacyNode(id, kind, label, carrier, endUse string, value float64, period string, sourceIDs []string) EnergyExplanationNode {
	return EnergyExplanationNode{
		ID: id, Level: "energy", Kind: kind, Label: label,
		Value: value, RawValue: value, EffectiveValue: value, AllocatedValue: value,
		Unit: "kWh", Period: period, Carrier: carrier, EndUse: endUse,
		MeterHierarchyLevel: "broad_end_use", Basis: "measured_meter", Multiplier: 1,
		SourceIDs: sourceIDs,
	}
}

func epath091AuditEndUses(nodes []EnergyExplanationNode) []EnergyExplanationNode {
	out := make([]EnergyExplanationNode, 0, 12)
	for _, node := range nodes {
		if node.Level == "end_use" {
			out = append(out, node)
		}
	}
	return out
}

func epath091AuditSourceByID(sources []EnergyDataSource, id string) *EnergyDataSource {
	for index := range sources {
		if sources[index].ID == id {
			return &sources[index]
		}
	}
	return nil
}

func epath091AuditSummaryItemByID(items []EnergyExplanationSummaryItem, id string) *EnergyExplanationSummaryItem {
	for index := range items {
		if items[index].ID == id {
			return &items[index]
		}
	}
	return nil
}

func epath091AuditAssertSourceSet(t *testing.T, got, want []string) {
	t.Helper()
	got = append([]string(nil), got...)
	want = append([]string(nil), want...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("source IDs = %#v; want %#v", got, want)
	}
}

type epath091AuditSemanticSnapshot struct {
	Nodes   []string
	Links   []string
	Periods []string
}

func epath091AuditSnapshot(result EnergyExplanationResult) epath091AuditSemanticSnapshot {
	out := epath091AuditSemanticSnapshot{}
	appendGraph := func(prefix string, nodes []EnergyExplanationNode, links []EnergyPathLink) {
		for _, node := range nodes {
			if node.Level != "end_use" && node.Level != "carrier" {
				continue
			}
			sources := append([]string(nil), node.SourceIDs...)
			sort.Strings(sources)
			out.Nodes = append(out.Nodes, fmt.Sprintf("%s|%s|%s|%s|%g|%g|%g|%s", prefix, node.ID, node.EndUse, node.Carrier, node.Value, node.RawValue, node.AllocatedValue, strings.Join(sources, ",")))
		}
		for _, link := range links {
			if link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier" {
				continue
			}
			sources := append([]string(nil), link.SourceIDs...)
			sort.Strings(sources)
			out.Links = append(out.Links, fmt.Sprintf("%s|%s|%s|%s|%g|%g|%s", prefix, link.Relation, link.FromID, link.ToID, link.FromValue, link.ToValue, strings.Join(sources, ",")))
		}
	}
	appendGraph("top", result.Nodes, result.Links)
	for _, period := range result.Periods {
		appendGraph(period.ID, period.Nodes, period.Links)
		out.Periods = append(out.Periods, period.ID)
	}
	sort.Strings(out.Nodes)
	sort.Strings(out.Links)
	return out
}
