package simulation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestEPATH090CarrierNeutralHeatingPreservesExactCarrierSplitsAndLocalSources(t *testing.T) {
	result := UpgradeEnergyExplanationV1(epath090ReviewFixture(false))

	heating := energyPathV2NodeByID(result.Nodes, "end_use.heating.building")
	if heating == nil {
		t.Fatalf("missing carrier-neutral Heating node in %#v", result.Nodes)
	}
	if heating.Level != "end_use" || heating.ScaleDomain != "site" || heating.Carrier != "" || heating.EndUse != "heating" || heating.Label != "Heating" || heating.Value != 120 || heating.RawValue != 120 || heating.EffectiveValue != 120 || heating.AllocatedValue != 120 {
		t.Fatalf("carrier-neutral Heating node = %#v", heating)
	}
	epath090ReviewAssertSourceSet(t, heating.SourceIDs, []string{"meter.heating.electricity", "meter.heating.natural_gas"})
	if energyPathV2NodeByID(result.Nodes, "end_use.heating.electricity.building") != nil || energyPathV2NodeByID(result.Nodes, "end_use.heating.natural_gas.building") != nil {
		t.Fatalf("carrier-qualified Heating nodes leaked into v2: %#v", result.Nodes)
	}
	endUseCount := 0
	for _, node := range result.Nodes {
		if node.Level == "end_use" {
			endUseCount++
		}
	}
	if endUseCount != 1 {
		t.Fatalf("end-use stage should contain one neutral Heating node, got %d in %#v", endUseCount, result.Nodes)
	}

	wantSplits := map[string]struct {
		value     float64
		sourceIDs []string
	}{
		"carrier.electricity.building": {
			value:     70,
			sourceIDs: []string{"meter.heating.electricity"},
		},
		"carrier.natural_gas.building": {
			value:     50,
			sourceIDs: []string{"meter.heating.natural_gas"},
		},
	}
	splits := epath090ReviewLinksFrom(result.Links, heating.ID, "end_use_to_carrier")
	if len(splits) != len(wantSplits) {
		t.Fatalf("Heating carrier splits = %#v", splits)
	}
	outgoing := 0.0
	for _, link := range splits {
		want, ok := wantSplits[link.ToID]
		if !ok {
			t.Fatalf("unexpected Heating carrier branch = %#v", link)
		}
		if link.FromValue != want.value || link.ToValue != want.value || link.FromUnit != "kWh" || link.ToUnit != "kWh" || link.Basis != "reported_meter" {
			t.Fatalf("Heating split %q = %#v", link.ToID, link)
		}
		epath090ReviewAssertSourceSet(t, link.SourceIDs, want.sourceIDs)
		outgoing += link.FromValue
	}
	if outgoing != heating.Value {
		t.Fatalf("Heating node does not close: node=%g outgoing=%g links=%#v", heating.Value, outgoing, splits)
	}
	if splits[0].ID >= splits[1].ID {
		t.Fatalf("carrier split order is not deterministic by ID: %#v", splits)
	}

	for _, carrierID := range []string{"carrier.electricity.building", "carrier.natural_gas.building"} {
		carrier := energyPathV2NodeByID(result.Nodes, carrierID)
		if carrier == nil || carrier.Level != "carrier" || carrier.ScaleDomain != "site" {
			t.Fatalf("carrier stage %q = %#v", carrierID, carrier)
		}
		if energyPathV2LinkByIDs(result.Links, carrierID, heating.ID) != nil {
			t.Fatalf("reverse carrier-to-end-use edge leaked for %q", carrierID)
		}
	}
	loadLink := energyPathV2LinkByIDs(result.Links, "load.heating.building", heating.ID)
	if loadLink == nil || loadLink.Relation != "load_to_end_use" {
		t.Fatalf("thermal/site stage boundary does not terminate at neutral Heating: %#v", loadLink)
	}
}

func TestEPATH090MonthlyAnnualCarrierSplitAndReconciliationAreOrderInvariant(t *testing.T) {
	forward := UpgradeEnergyExplanationV1(epath090ReviewFixture(false))
	reversed := UpgradeEnergyExplanationV1(epath090ReviewFixture(true))
	withoutLegacyRows := epath090ReviewFixture(false)
	withoutLegacyRows.Reconciliation = nil
	for index := range withoutLegacyRows.Periods {
		withoutLegacyRows.Periods[index].Reconciliation = nil
	}
	rebuilt := UpgradeEnergyExplanationV1(withoutLegacyRows)

	if got, want := epath090ReviewSnapshot(reversed), epath090ReviewSnapshot(forward); !reflect.DeepEqual(got, want) {
		t.Fatalf("carrier/end-use semantics depend on v1 node or edge order:\nreversed=%#v\nforward=%#v", got, want)
	}
	if got, want := epath090ReviewSnapshot(rebuilt), epath090ReviewSnapshot(forward); !reflect.DeepEqual(got, want) {
		t.Fatalf("v2 carrier reconciliation depends on legacy rows instead of canonical split links:\nrebuilt=%#v\nforward=%#v", got, want)
	}

	periodCases := []struct {
		period               string
		heating              float64
		electricitySplit     float64
		naturalGasSplit      float64
		electricityExpected  float64
		electricityExplained float64
		electricityResidual  float64
		electricityStatus    string
		naturalGasExpected   float64
		naturalGasExplained  float64
		naturalGasResidual   float64
		naturalGasStatus     string
	}{
		{period: "M1", heating: 70, electricitySplit: 40, naturalGasSplit: 30, electricityExpected: 60, electricityExplained: 40, electricityResidual: 20, electricityStatus: "residual", naturalGasExpected: 20, naturalGasExplained: 30, naturalGasResidual: -10, naturalGasStatus: "overmapped"},
		{period: "M2", heating: 50, electricitySplit: 30, naturalGasSplit: 20, electricityExpected: 40, electricityExplained: 30, electricityResidual: 10, electricityStatus: "residual", naturalGasExpected: 20, naturalGasExplained: 20, naturalGasResidual: 0, naturalGasStatus: "balanced"},
		{period: "annual", heating: 120, electricitySplit: 70, naturalGasSplit: 50, electricityExpected: 100, electricityExplained: 70, electricityResidual: 30, electricityStatus: "residual", naturalGasExpected: 40, naturalGasExplained: 50, naturalGasResidual: -10, naturalGasStatus: "overmapped"},
	}
	for _, test := range periodCases {
		t.Run(test.period, func(t *testing.T) {
			nodes, links, reconciliation := epath090ReviewPeriodGraph(forward, test.period)
			heating := energyPathV2NodeByID(nodes, "end_use.heating.building")
			if heating == nil || heating.Value != test.heating {
				t.Fatalf("%s Heating = %#v", test.period, heating)
			}
			electricity := energyPathV2LinkByIDs(links, heating.ID, "carrier.electricity.building")
			naturalGas := energyPathV2LinkByIDs(links, heating.ID, "carrier.natural_gas.building")
			if electricity == nil || electricity.Relation != "end_use_to_carrier" || electricity.FromValue != test.electricitySplit || electricity.ToValue != test.electricitySplit {
				t.Fatalf("%s electricity split = %#v", test.period, electricity)
			}
			if naturalGas == nil || naturalGas.Relation != "end_use_to_carrier" || naturalGas.FromValue != test.naturalGasSplit || naturalGas.ToValue != test.naturalGasSplit {
				t.Fatalf("%s natural-gas split = %#v", test.period, naturalGas)
			}
			epath090ReviewAssertSourceSet(t, electricity.SourceIDs, []string{"meter.heating.electricity"})
			epath090ReviewAssertSourceSet(t, naturalGas.SourceIDs, []string{"meter.heating.natural_gas"})
			if electricity.FromValue+naturalGas.FromValue != heating.Value {
				t.Fatalf("%s Heating split closure = %g + %g, node=%g", test.period, electricity.FromValue, naturalGas.FromValue, heating.Value)
			}

			electricityRec := epath090ReviewReconciliation(reconciliation, "reconcile.energy.electricity."+test.period)
			epath090ReviewAssertReconciliation(t, electricityRec, test.electricityExpected, test.electricityExplained, test.electricityResidual, test.electricityStatus)
			epath090ReviewAssertSourceSet(t, electricityRec.SourceIDs, []string{"meter.facility.electricity", "meter.heating.electricity"})
			naturalGasRec := epath090ReviewReconciliation(reconciliation, "reconcile.energy.natural_gas."+test.period)
			epath090ReviewAssertReconciliation(t, naturalGasRec, test.naturalGasExpected, test.naturalGasExplained, test.naturalGasResidual, test.naturalGasStatus)
			epath090ReviewAssertSourceSet(t, naturalGasRec.SourceIDs, []string{"meter.facility.natural_gas", "meter.heating.natural_gas"})
		})
	}

	annualHeating := energyPathV2NodeByID(forward.Nodes, "end_use.heating.building")
	m1 := energyExplanationPeriodByID(forward.Periods, "M1")
	m2 := energyExplanationPeriodByID(forward.Periods, "M2")
	if annualHeating == nil || m1 == nil || m2 == nil {
		t.Fatalf("missing annual/month graphs: annual=%#v M1=%#v M2=%#v", annualHeating, m1, m2)
	}
	if annualHeating.Value != epath090ReviewNodeValue(m1.Nodes, annualHeating.ID)+epath090ReviewNodeValue(m2.Nodes, annualHeating.ID) {
		t.Fatalf("annual Heating is not the monthly contribution sum: annual=%g M1=%g M2=%g", annualHeating.Value, epath090ReviewNodeValue(m1.Nodes, annualHeating.ID), epath090ReviewNodeValue(m2.Nodes, annualHeating.ID))
	}
}

func TestEPATH090V1AdapterRemainsCarrierQualifiedAndReadOnly(t *testing.T) {
	legacy := epath090ReviewFixture(false)
	before, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Schema != energyExplanationV1Schema || energyExplanationNodeByID(legacy.Nodes, "energy.end_use.heating.electricity") == nil || energyExplanationNodeByID(legacy.Nodes, "energy.end_use.heating.natural_gas") == nil {
		t.Fatalf("v1 carrier-qualified nodes changed: schema=%q nodes=%#v", legacy.Schema, legacy.Nodes)
	}
	for _, pair := range [][2]string{
		{"energy.carrier.electricity", "energy.end_use.heating.electricity"},
		{"energy.carrier.natural_gas", "energy.end_use.heating.natural_gas"},
	} {
		edge := energyExplanationEdgeByIDs(legacy.Edges, pair[0], pair[1])
		if edge == nil || edge.Relation != "meter_enduse" || edge.RuleID != energyRelationshipRuleMeterEndUse {
			t.Fatalf("v1 meter edge %s -> %s changed: %#v", pair[0], pair[1], edge)
		}
	}

	_ = UpgradeEnergyExplanationV1(legacy)
	after, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("v1 compatibility input was mutated by upgrade:\nbefore=%s\nafter=%s", before, after)
	}
}

func epath090ReviewFixture(reverse bool) EnergyExplanationV1 {
	series := []energyExplanationSeries{
		{
			Stage: "carrier", CanonicalKind: "energy.electricity.total", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Unit: "kWh",
			Carrier: "electricity", EndUse: "total", MeterHierarchyLevel: "facility_total", Basis: "measured_meter", SourceIDs: []string{"meter.facility.electricity"},
			Total: 100, Monthly: map[int]float64{1: 60, 2: 40},
		},
		{
			Stage: "carrier", CanonicalKind: "energy.natural_gas.total", Level: "energy", Kind: "energy.natural_gas.total", Label: "Natural Gas", Unit: "kWh",
			Carrier: "natural_gas", EndUse: "total", MeterHierarchyLevel: "facility_total", Basis: "measured_meter", SourceIDs: []string{"meter.facility.natural_gas"},
			Total: 40, Monthly: map[int]float64{1: 20, 2: 20},
		},
		{
			Stage: "end_use", CanonicalKind: "energy.heating", Level: "energy", Kind: "energy.heating", Label: "Heating electricity", Unit: "kWh",
			Carrier: "electricity", EndUse: "heating", MeterHierarchyLevel: "broad_end_use", Basis: "measured_meter", SourceIDs: []string{"meter.heating.electricity"},
			Total: 70, Monthly: map[int]float64{1: 40, 2: 30},
		},
		{
			Stage: "end_use", CanonicalKind: "energy.heating", Level: "energy", Kind: "energy.heating", Label: "Heating gas", Unit: "kWh",
			Carrier: "natural_gas", EndUse: "heating", MeterHierarchyLevel: "broad_end_use", Basis: "measured_meter", SourceIDs: []string{"meter.heating.natural_gas"},
			Total: 50, Monthly: map[int]float64{1: 30, 2: 20},
		},
		{
			Stage: "load", CanonicalKind: "load.zone_heating", Level: "load", Kind: "load.zone_heating", Label: "Heating load", Unit: "kWh",
			ServiceKind: "heating", ZoneName: "Office", Basis: "measured_variable", SourceIDs: []string{"variable.load.heating"},
			Total: 240, Monthly: map[int]float64{1: 140, 2: 100},
		},
	}
	if reverse {
		epath090ReviewReverseSeries(series)
	}
	graph := func(period string, valueFor func(energyExplanationSeries) float64) energyExplanationGraph {
		return buildEnergyExplanationGraphForPeriod(period, series, PurposeAllocationPolicyDirectOnly, valueFor)
	}
	annual := graph("annual", func(item energyExplanationSeries) float64 { return item.Total })
	m1 := graph("M1", func(item energyExplanationSeries) float64 { return item.Monthly[1] })
	m2 := graph("M2", func(item energyExplanationSeries) float64 { return item.Monthly[2] })
	legacy := EnergyExplanationV1{
		Schema:            energyExplanationV1Schema,
		Purpose:           string(SimulationPurposeBasicEnergy),
		Frequency:         "monthly",
		AllocationPolicy:  PurposeAllocationPolicyDirectOnly,
		RelationshipRules: energyRelationshipRuleCatalog(),
		Periods: []EnergyPeriod{
			{ID: "annual", Label: "Annual", Kind: "annual", Nodes: annual.Nodes, Edges: annual.Edges, Reconciliation: annual.Reconciliation, Warnings: annual.Warnings},
			{ID: "M1", Label: "M1", Kind: "monthly", Nodes: m1.Nodes, Edges: m1.Edges, Reconciliation: m1.Reconciliation, Warnings: m1.Warnings},
			{ID: "M2", Label: "M2", Kind: "monthly", Nodes: m2.Nodes, Edges: m2.Edges, Reconciliation: m2.Reconciliation, Warnings: m2.Warnings},
		},
		Nodes:          annual.Nodes,
		Edges:          annual.Edges,
		Reconciliation: annual.Reconciliation,
		Sources: []EnergyDataSource{
			{ID: "meter.facility.electricity", SourceType: "sql_meter", IsMeter: true, Name: "Electricity:Facility", Units: "kWh"},
			{ID: "meter.facility.natural_gas", SourceType: "sql_meter", IsMeter: true, Name: "NaturalGas:Facility", Units: "kWh"},
			{ID: "meter.heating.electricity", SourceType: "sql_meter", IsMeter: true, Name: "Heating:Electricity", Units: "kWh"},
			{ID: "meter.heating.natural_gas", SourceType: "sql_meter", IsMeter: true, Name: "Heating:NaturalGas", Units: "kWh"},
			{ID: "variable.load.heating", SourceType: "sql_variable", Name: "Zone Air System Sensible Heating Energy", Units: "kWh", ZoneName: "Office"},
		},
		Completeness:          EnergyCompleteness{Status: "complete"},
		scope:                 EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
		canonicalMonthlyBasis: true,
	}
	if reverse {
		epath090ReviewReverseNodes(legacy.Nodes)
		epath090ReviewReverseEdges(legacy.Edges)
		epath090ReviewReverseReconciliation(legacy.Reconciliation)
		epath090ReviewReverseSources(legacy.Sources)
		for index := range legacy.Periods {
			epath090ReviewReverseNodes(legacy.Periods[index].Nodes)
			epath090ReviewReverseEdges(legacy.Periods[index].Edges)
			epath090ReviewReverseReconciliation(legacy.Periods[index].Reconciliation)
		}
	}
	return legacy
}

type epath090ReviewSemanticSnapshot struct {
	Nodes          []string
	Links          []string
	Reconciliation []string
	Periods        []string
}

func epath090ReviewSnapshot(result EnergyExplanationResult) epath090ReviewSemanticSnapshot {
	snapshot := epath090ReviewSemanticSnapshot{}
	appendGraph := func(prefix string, nodes []EnergyExplanationNode, links []EnergyPathLink, reconciliation []EnergyReconciliation) {
		for _, node := range nodes {
			if node.Level != "end_use" && node.Level != "carrier" {
				continue
			}
			sources := append([]string(nil), node.SourceIDs...)
			sort.Strings(sources)
			snapshot.Nodes = append(snapshot.Nodes, fmt.Sprintf("%s|%s|%s|%s|%s|%g|%s", prefix, node.ID, node.Level, node.Label, node.Carrier, node.Value, strings.Join(sources, ",")))
		}
		for _, link := range links {
			if link.Relation != "end_use_to_carrier" {
				continue
			}
			sources := append([]string(nil), link.SourceIDs...)
			sort.Strings(sources)
			snapshot.Links = append(snapshot.Links, fmt.Sprintf("%s|%s|%s|%s|%g|%g|%s", prefix, link.ID, link.FromID, link.ToID, link.FromValue, link.ToValue, strings.Join(sources, ",")))
		}
		for _, item := range reconciliation {
			if item.Level != "energy" {
				continue
			}
			snapshot.Reconciliation = append(snapshot.Reconciliation, fmt.Sprintf("%s|%s|%g|%g|%g|%s", prefix, item.ID, item.ExpectedValue, item.ExplainedValue, item.ResidualValue, item.Status))
		}
	}
	appendGraph("top", result.Nodes, result.Links, result.Reconciliation)
	for _, period := range result.Periods {
		appendGraph(period.ID, period.Nodes, period.Links, period.Reconciliation)
		snapshot.Periods = append(snapshot.Periods, period.ID)
	}
	sort.Strings(snapshot.Nodes)
	sort.Strings(snapshot.Links)
	sort.Strings(snapshot.Reconciliation)
	return snapshot
}

func epath090ReviewPeriodGraph(result EnergyExplanationResult, period string) ([]EnergyExplanationNode, []EnergyPathLink, []EnergyReconciliation) {
	if period == "annual" {
		return result.Nodes, result.Links, result.Reconciliation
	}
	item := energyExplanationPeriodByID(result.Periods, period)
	if item == nil {
		return nil, nil, nil
	}
	return item.Nodes, item.Links, item.Reconciliation
}

func epath090ReviewLinksFrom(links []EnergyPathLink, fromID string, relation string) []EnergyPathLink {
	out := []EnergyPathLink{}
	for _, link := range links {
		if link.FromID == fromID && link.Relation == relation {
			out = append(out, link)
		}
	}
	return out
}

func epath090ReviewReconciliation(items []EnergyReconciliation, id string) *EnergyReconciliation {
	for index := range items {
		if items[index].ID == id {
			return &items[index]
		}
	}
	return nil
}

func epath090ReviewAssertReconciliation(t *testing.T, item *EnergyReconciliation, expected float64, explained float64, residual float64, status string) {
	t.Helper()
	if item == nil || item.ExpectedValue != expected || item.ExplainedValue != explained || item.ResidualValue != residual || item.Status != status || item.Basis != "residual" || item.Unit != "kWh" {
		t.Fatalf("reconciliation = %#v; want expected=%g explained=%g residual=%g status=%q", item, expected, explained, residual, status)
	}
}

func epath090ReviewAssertSourceSet(t *testing.T, got []string, want []string) {
	t.Helper()
	got = append([]string(nil), got...)
	want = append([]string(nil), want...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("source IDs = %#v; want branch-local %#v", got, want)
	}
}

func epath090ReviewNodeValue(nodes []EnergyExplanationNode, id string) float64 {
	if node := energyPathV2NodeByID(nodes, id); node != nil {
		return node.Value
	}
	return 0
}

func epath090ReviewReverseSeries(values []energyExplanationSeries) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func epath090ReviewReverseNodes(values []EnergyExplanationNode) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func epath090ReviewReverseEdges(values []EnergyExplanationEdge) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func epath090ReviewReverseReconciliation(values []EnergyReconciliation) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func epath090ReviewReverseSources(values []EnergyDataSource) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
