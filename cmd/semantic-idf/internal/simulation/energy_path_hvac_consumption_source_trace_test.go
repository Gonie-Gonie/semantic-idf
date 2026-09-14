package simulation

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestEnergyPathHVACConsumptionSourceTracePreservesNativeAndScopedBoundaries(t *testing.T) {
	nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
	plan := reserveEnergyPathHVACConsumptionPools(buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology), nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
	source := epathHVACConsumptionTraceSource("sql.boiler.ancillary", "Boiler Ancillary Electricity Energy", "Monthly", 10)
	before := source
	buildingScope := EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}
	building := applyEnergyPathHVACConsumptionSourceAllocations([]EnergyDataSource{source}, plan, []energyPathHVACConsumptionPool{pool}, buildingScope)
	if len(building) != 1 || building[0].RawValue != 10 || building[0].EffectiveValue != 10 || building[0].AllocationApplied || len(building[0].ScopeDetails) != 5 {
		t.Fatalf("native total changed or exact Zone details missing: %#v", building)
	}
	for _, detail := range building[0].ScopeDetails {
		if detail.RawValue != 0 || detail.EffectiveValue != 0 || detail.inspectorValuePresence != 0 || !detail.inspectorScopedValuePresence || detail.AllocatedValue != 2 || !detail.AllocationApplied || detail.AllocationFactor != 0.2 {
			t.Errorf("shared pool became individually measured Zone energy: %#v", detail)
		}
	}
	if !reflect.DeepEqual(source, before) {
		t.Fatal("source detail replacement mutated the input source")
	}
	selected := applyEnergyPathHVACConsumptionSourceAllocations([]EnergyDataSource{source}, plan, []energyPathHVACConsumptionPool{pool}, EnergyExplanationScope{Kind: "zone", ZoneName: "SPACE2-1", AggregationBasis: "model_total"})
	if selected[0].RawValue != 10 || selected[0].EffectiveValue != 10 || selected[0].AllocatedValue != 2 || !selected[0].AllocationApplied || len(selected[0].ScopeDetails) != 1 || !stringSliceContains(selected[0].InputSourceIDs, "sql.load.SPACE2-1") {
		t.Fatalf("selected Zone source lacks native/allocated distinction: %#v", selected[0])
	}
	wire, err := json.Marshal(energyPathSourceWire{selected[0]})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	var details []map[string]json.RawMessage
	if err := json.Unmarshal(decoded["scopeDetails"], &details); err != nil {
		t.Fatal(err)
	}
	if string(decoded["rawValue"]) != "10" || string(decoded["allocatedValue"]) != "2" || len(details) != 1 || details[0]["rawValue"] != nil || details[0]["effectiveValue"] != nil || string(details[0]["allocatedValue"]) != "2" {
		t.Fatalf("JSON fabricated a raw Zone zero/total or lost actual native source: %s", wire)
	}
}

func TestEnergyPathHVACConsumptionSourceTraceDistinguishesTwoMergedBoilers(t *testing.T) {
	nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
	pool.Members = append(pool.Members, energyPathHVACConsumptionMember{ID: "second", ObjectType: "Boiler:HotWater", ObjectName: "Second Boiler", OutputName: "Boiler Ancillary Electricity Energy", RelatedPathIDs: []string{"heating.SPACE2-1"}, Series: epathHVACConsumptionPoolSeries("", "sql.second.ancillary", 5)})
	plan := reserveEnergyPathHVACConsumptionPools(buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology), nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
	if len(plan.ConsumptionSourceAllocations) != 6 {
		t.Fatalf("merged broad/Zone edges lost an independent source: %#v", plan.ConsumptionSourceAllocations)
	}
	first := epathHVACConsumptionTraceSource("sql.boiler.ancillary", "Boiler Ancillary Electricity Energy", "Monthly", 10)
	second := epathHVACConsumptionTraceSource("sql.second.ancillary", "Boiler Ancillary Electricity Energy", "Monthly", 5)
	second.KeyValue = "Second Boiler"
	got := applyEnergyPathHVACConsumptionSourceAllocations([]EnergyDataSource{first, second}, plan, []energyPathHVACConsumptionPool{pool}, EnergyExplanationScope{Kind: "zone", ZoneName: "SPACE2-1", AggregationBasis: "model_total"})
	if got[0].AllocatedValue != 2 || got[0].AllocationFactor != 0.2 || got[1].AllocatedValue != 5 || got[1].AllocationFactor != 1 {
		t.Fatalf("one merged edge factor was reused across distinct source budgets: %#v", got)
	}
}

func TestEnergyPathHVACConsumptionSourceTraceUnknownZeroAndCompanions(t *testing.T) {
	for _, scenario := range []string{"missing", "known zero", "no recipients"} {
		t.Run(scenario, func(t *testing.T) {
			nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
			value := 10.0
			switch scenario {
			case "missing":
				delete(pool.Members[2].Series.Monthly, 1)
			case "known zero":
				value, pool.Members[2].Series.Monthly[1] = 0, 0
			case "no recipients":
				pool.Members[2].RelatedPathIDs = nil
			}
			plan := reserveEnergyPathHVACConsumptionPools(buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology), nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
			source := epathHVACConsumptionTraceSource("sql.boiler.ancillary", "Boiler Ancillary Electricity Energy", "Monthly", value)
			got := applyEnergyPathHVACConsumptionSourceAllocations([]EnergyDataSource{source}, plan, []energyPathHVACConsumptionPool{pool}, EnergyExplanationScope{Kind: "zone", ZoneName: "SPACE2-1", AggregationBasis: "model_total"})[0]
			if scenario == "known zero" {
				if len(plan.ConsumptionSourceAllocations) != 5 || len(got.ScopeDetails) != 1 || !got.AllocationApplied || got.AllocatedValue != 0 {
					t.Fatalf("actual zero allocation became unknown: %#v", got)
				}
			} else if len(plan.ConsumptionSourceAllocations) != 0 || len(got.ScopeDetails) != 0 || got.AllocationApplied || got.AllocatedValue != 0 {
				t.Fatalf("unknown allocation became an observed zero/full source: %#v", got)
			}
		})
	}
	nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
	plan := reserveEnergyPathHVACConsumptionPools(buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology), nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
	hourly := epathHVACConsumptionTraceSource("sql.boiler.hourly", "Boiler Ancillary Electricity Energy", "Hourly", 10)
	rate := epathHVACConsumptionTraceSource("sql.boiler.rate", "Boiler Ancillary Electricity Rate", "Hourly", 42)
	foreign := epathHVACConsumptionTraceSource("sql.foreign.rate", "Boiler Ancillary Electricity Rate", "Hourly", 84)
	foreign.KeyValue = "Unrelated Boiler"
	got := applyEnergyPathHVACConsumptionSourceAllocations([]EnergyDataSource{hourly, rate, foreign}, plan, []energyPathHVACConsumptionPool{pool}, EnergyExplanationScope{Kind: "zone", ZoneName: "SPACE2-1", AggregationBasis: "model_total"})
	for i, want := range []float64{10, 42} {
		if got[i].RawValue != want || got[i].EffectiveValue != want || got[i].AllocationApplied || got[i].AllocatedValue != 0 || len(got[i].ScopeDetails) != 0 {
			t.Errorf("Hourly/Rate companion became an extra monthly allocation: %#v", got[i])
		}
	}
	if !reflect.DeepEqual(got[2], foreign) {
		t.Fatal("component context qualification escaped exact original identity")
	}
	// Direct-only graphs have no allocation plan, but a global native source
	// still must not acquire an individually measured Zone value from context.
	unallocated := applyEnergyPathHVACConsumptionSourceAllocations([]EnergyDataSource{hourly}, energyPathZoneHVACAllocationPlan{}, []energyPathHVACConsumptionPool{pool}, EnergyExplanationScope{Kind: "zone", ZoneName: "SPACE2-1"})
	if unallocated[0].RawValue != 10 || unallocated[0].AllocationApplied || unallocated[0].AllocatedValue != 0 || len(unallocated[0].ScopeDetails) != 0 {
		t.Fatalf("direct-only context fabricated a Zone allocation: %#v", unallocated[0])
	}
}

func TestEnergyPathHVACConsumptionSourceTraceDirectOnlyBuildingAndZone(t *testing.T) {
	_, _, _, pool := epathHVACConsumptionPoolFixture()
	native := epathHVACConsumptionTraceSource("sql.boiler.ancillary", "Boiler Ancillary Electricity Energy", "Monthly", 10)
	hourly := epathHVACConsumptionTraceSource("sql.boiler.hourly", "Boiler Ancillary Electricity Energy", "Hourly", 10)
	rate := epathHVACConsumptionTraceSource("sql.boiler.rate", "Boiler Ancillary Electricity Rate", "Hourly", 42)
	native.AllocationApplied, native.AllocationFactor = true, 1
	for _, scope := range []EnergyExplanationScope{{Kind: "building", AggregationBasis: "model_total"}, {Kind: "zone", ZoneName: "SPACE2-1", AggregationBasis: "model_total"}} {
		got := applyEnergyPathHVACConsumptionSourceAllocations([]EnergyDataSource{native, hourly, rate}, energyPathZoneHVACAllocationPlan{}, []energyPathHVACConsumptionPool{pool}, scope)
		wires := []energyPathSourceWire{}
		for i, value := range []float64{10, 10, 42} {
			if got[i].RawValue != value || got[i].EffectiveValue != value || !energyDataSourceValueKnown(got[i], energySourceObservedRaw) || got[i].AllocationApplied || got[i].AllocationFactor != 0 || got[i].AllocatedValue != 0 || len(got[i].ScopeDetails) != 0 {
				t.Errorf("%s direct-only source retained inferred allocation or lost native quantity: %#v", scope.Kind, got[i])
			}
			wires = append(wires, energyPathSourceWire{got[i]})
		}
		data, err := json.Marshal(wires)
		if err != nil {
			t.Fatal(err)
		}
		var reopened []EnergyDataSource
		if err := json.Unmarshal(data, &reopened); err != nil {
			t.Fatal(err)
		}
		for i, value := range []float64{10, 10, 42} {
			if reopened[i].RawValue != value || reopened[i].EffectiveValue != value || !energyDataSourceValueKnown(reopened[i], energySourceObservedRaw) || reopened[i].AllocationApplied || reopened[i].AllocatedValue != 0 || len(reopened[i].ScopeDetails) != 0 {
				t.Errorf("%s direct-only JSON resurrected inferred Zone consumption: %s", scope.Kind, data)
			}
		}
	}
}

func epathHVACConsumptionTraceSource(id, name, frequency string, value float64) EnergyDataSource {
	return EnergyDataSource{ID: id, SourceType: "sql_report_data", KeyValue: "Central Boiler", Name: name, ReportingFrequency: frequency, RawValue: value, EffectiveValue: value, EffectiveMultiplier: 1, MultiplierApplication: energyMultiplierAlreadyModelTotal, AllocatedValue: value, observedValuePresence: energySourceObservedRaw | energySourceObservedEffective,
		ScopeDetails: []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "PLENUM-1"}, RawValue: value, EffectiveValue: value, AllocatedValue: value, AllocationApplied: true}}}
}
