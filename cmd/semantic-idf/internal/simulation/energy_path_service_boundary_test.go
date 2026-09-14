package simulation

// Hand-authored boundary tests do not depend on simulation/candidate values.

import (
	"encoding/json"
	"reflect"
	"testing"
)

func epathBoundaryTestRestriction() energyPathServiceBoundaryRestriction {
	return energyPathServiceBoundaryRestriction{
		Reason: energyPathBoundaryNonZoneDemand,
		EndUse: "heating", ServiceKind: "heating", Carrier: "natural_gas",
		PlantLoopName:    "Hot Water Loop",
		DemandObjectType: "SwimmingPool:Indoor", DemandObjectName: "Test Pool",
		ConsumerSourceIDs: []string{"sql-rdd-101"},
		MeterSourceIDs:    []string{"sql-rdd-201"},
	}
}

func epathBoundaryTestNode(id, endUse string, value float64, restrictions ...energyPathServiceBoundaryRestriction) EnergyExplanationNode {
	return EnergyExplanationNode{
		ID: id, Level: "end_use", Kind: "end_use." + endUse,
		EndUse: endUse, ServiceKind: endUse, Value: value, RawValue: value,
		EffectiveValue: value, Unit: "kWh", SourceIDs: []string{"sql-rdd-301"},
		serviceBoundaryRestrictions: cloneEnergyPathServiceBoundaryRestrictions(restrictions),
	}
}

func TestEnergyPathServiceBoundaryStableUnionAndDeepCopy(t *testing.T) {
	first := epathBoundaryTestRestriction()
	first.ConsumerSourceIDs = []string{" SQL-RDD-102 ", "sql-rdd-101", "sql-rdd-101"}
	second := epathBoundaryTestRestriction()
	second.PlantLoopName, second.DemandObjectType = " hot water loop ", "swimmingpool:indoor"
	second.ConsumerSourceIDs = []string{"sql-rdd-103"}
	second.MeterSourceIDs = []string{"sql-rdd-202", "sql-rdd-201"}
	other := epathBoundaryTestRestriction()
	other.PlantLoopName, other.DemandObjectName = "Second Plant", "Second Pool"
	a := []energyPathServiceBoundaryRestriction{first, other}
	b := []energyPathServiceBoundaryRestriction{second}
	aBefore, bBefore := cloneEnergyPathServiceBoundaryRestrictions(a), cloneEnergyPathServiceBoundaryRestrictions(b)
	wantFirst := normalizeEnergyPathServiceBoundaryRestriction(first)
	wantFirst.ConsumerSourceIDs = []string{"sql-rdd-101", "sql-rdd-102", "sql-rdd-103"}
	wantFirst.MeterSourceIDs = []string{"sql-rdd-201", "sql-rdd-202"}
	got := unionEnergyPathServiceBoundaryRestrictions(a, b)
	if len(got) != 2 || !reflect.DeepEqual(got, unionEnergyPathServiceBoundaryRestrictions(b, a, a)) {
		t.Fatalf("union must be stable and idempotent: %#v", got)
	}
	found := false
	for _, r := range got {
		if err := validateEnergyPathServiceBoundaryRestriction(r); err != nil {
			t.Fatal(err)
		}
		if r.PlantLoopName == "hot water loop" {
			found = reflect.DeepEqual(r, wantFirst)
		}
	}
	if !found || !reflect.DeepEqual(a, aBefore) || !reflect.DeepEqual(b, bBefore) {
		t.Fatal("union lost exact source roles or mutated input")
	}
	got[0].ConsumerSourceIDs[0], got[0].MeterSourceIDs[0] = "changed", "changed"
	if !reflect.DeepEqual(a, aBefore) || !reflect.DeepEqual(b, bBefore) {
		t.Fatal("union retained source-slice aliases into input")
	}
	cloned := cloneEnergyPathServiceBoundaryRestrictions(a)
	cloned[0].ConsumerSourceIDs[0], cloned[0].MeterSourceIDs[0] = "changed", "changed"
	if !reflect.DeepEqual(a, aBefore) {
		t.Fatal("clone retained source-slice aliases into input")
	}
	if unionEnergyPathServiceBoundaryRestrictions(nil) != nil || cloneEnergyPathServiceBoundaryRestrictions(nil) != nil {
		t.Fatal("absent metadata must remain absent")
	}
}

func TestEnergyPathServiceBoundaryInvalidMetadataStillDenies(t *testing.T) {
	cases := []struct {
		name   string
		change func(*energyPathServiceBoundaryRestriction)
	}{
		{"reason", func(r *energyPathServiceBoundaryRestriction) { r.Reason = "future_unknown_reason" }},
		{"service", func(r *energyPathServiceBoundaryRestriction) { r.ServiceKind = "cooling" }},
		{"end_use", func(r *energyPathServiceBoundaryRestriction) { r.EndUse = "lighting" }},
		{"carrier", func(r *energyPathServiceBoundaryRestriction) { r.Carrier = "propane" }},
		{"pump_carrier", func(r *energyPathServiceBoundaryRestriction) { r.EndUse = "pumps" }},
		{"demand_type", func(r *energyPathServiceBoundaryRestriction) { r.DemandObjectType = "Zone" }},
		{"plant", func(r *energyPathServiceBoundaryRestriction) { r.PlantLoopName = "" }},
		{"demand", func(r *energyPathServiceBoundaryRestriction) { r.DemandObjectName = "" }},
		{"no_source", func(r *energyPathServiceBoundaryRestriction) { r.ConsumerSourceIDs, r.MeterSourceIDs = nil, nil }},
		{"cross_role", func(r *energyPathServiceBoundaryRestriction) { r.ConsumerSourceIDs = []string{"sql-rdd-201"} }},
		{"empty_source", func(r *energyPathServiceBoundaryRestriction) { r.ConsumerSourceIDs = []string{""} }},
		{"zero_source", func(r *energyPathServiceBoundaryRestriction) { r.ConsumerSourceIDs = []string{"sql-rdd-0"} }},
		{"negative_source", func(r *energyPathServiceBoundaryRestriction) { r.ConsumerSourceIDs = []string{"sql-rdd--1"} }},
		{"noncanonical_source", func(r *energyPathServiceBoundaryRestriction) { r.ConsumerSourceIDs = []string{"sql-rdd-01"} }},
		{"foreign_source", func(r *energyPathServiceBoundaryRestriction) { r.ConsumerSourceIDs = []string{"tabular-101"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := epathBoundaryTestRestriction()
			tc.change(&r)
			if validateEnergyPathServiceBoundaryRestriction(r) == nil {
				t.Fatal("malformed restriction accepted as qualified evidence")
			}
			node := epathBoundaryTestNode("end_use.heating", "heating", 30, r)
			node.serviceBoundaryRestrictions = unionEnergyPathServiceBoundaryRestrictions(node.serviceBoundaryRestrictions)
			if !energyPathServiceBoundaryDeniesConversion(node) {
				t.Fatal("validation failure silently restored conversion authority")
			}
			links := []EnergyPathLink{{ID: "unproved", ToID: node.ID, Relation: "load_to_end_use", ToValue: 30}}
			if len(filterEnergyPathServiceBoundaryConversions([]EnergyExplanationNode{node}, links)) != 0 || node.Value != 30 {
				t.Fatal("explicit restriction must remove the conversion, not its observed energy")
			}
		})
	}
	partial := epathBoundaryTestRestriction()
	partial.Reason, partial.PlantLoopName = energyPathBoundaryTopologyIncomplete, ""
	partial.ConsumerSourceIDs = nil
	if err := validateEnergyPathServiceBoundaryRestriction(partial); err != nil {
		t.Fatalf("observed broad source plus typed incomplete topology remains an explicit restriction: %v", err)
	}
	partial.MeterSourceIDs = nil
	if err := validateEnergyPathServiceBoundaryRestriction(partial); err != nil {
		t.Fatalf("actual Pool identity with no native source must remain a topology-only denial: %v", err)
	}
	if got := projectEnergyPathServiceBoundaryRestrictions([]energyPathServiceBoundaryRestriction{partial}, []string{"sql-rdd-102"}, true); len(got) != 1 {
		t.Fatal("topology-only restriction became exact source disjointness authority")
	}
	partial.DemandObjectName = ""
	if validateEnergyPathServiceBoundaryRestriction(partial) == nil {
		t.Fatal("incomplete topology cannot invent a Pool identity for an arbitrary foreign demand")
	}
	if energyPathServiceBoundaryDeniesConversion(epathBoundaryTestNode("end_use.heating", "heating", 30)) {
		t.Fatal("legacy nil metadata unexpectedly changed behavior")
	}
}

func TestEnergyPathServiceBoundaryZeroBeforePruningCensus(t *testing.T) {
	for _, tc := range []struct {
		name     string
		value    float64
		presence uint8
	}{
		{"positive", 20, 255},
		{"observed_zero", 0, 255},
		{"unknown", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restricted := epathBoundaryTestNode("legacy.heating.gas", "heating", tc.value, epathBoundaryTestRestriction())
			restricted.Level, restricted.Carrier = "energy", "natural_gas"
			restricted.inspectorDecodedFromJSON, restricted.inspectorValuePresence = true, tc.presence
			restricted.SourceIDs = []string{"sql-rdd-201"}
			electric := epathBoundaryTestNode("legacy.heating.electric", "heating", 10)
			electric.Level, electric.Carrier = "energy", "electricity"
			legacy := []EnergyExplanationNode{restricted, electric}
			census := collectEnergyPathServiceBoundaryCensus(legacy)
			// Simulate the numerical builder pruning a zero/unknown gas node.
			// This deliberately leaves only electric provenance on the target;
			// the independent metadata census must still restrict the aggregate.
			canonical := []EnergyExplanationNode{
				epathBoundaryTestNode("end_use.heating", "heating", 10+tc.value),
				epathBoundaryTestNode("end_use.cooling", "cooling", 40),
			}
			got := applyEnergyPathServiceBoundaryCensus(canonical, census)
			if !energyPathServiceBoundaryDeniesConversion(got[0]) || energyPathServiceBoundaryDeniesConversion(got[1]) {
				t.Fatal("zero/unknown restriction vanished or crossed to unrelated Cooling")
			}
			for index := range got {
				withoutMetadata := got[index]
				withoutMetadata.serviceBoundaryRestrictions = nil
				if !reflect.DeepEqual(withoutMetadata, canonical[index]) {
					t.Fatal("census changed energy, source provenance, or knownness fields")
				}
			}
			got[0].serviceBoundaryRestrictions[0].ConsumerSourceIDs[0] = "changed"
			if canonical[0].serviceBoundaryRestrictions != nil || legacy[0].serviceBoundaryRestrictions[0].ConsumerSourceIDs[0] != "sql-rdd-101" || census["heating"][0].ConsumerSourceIDs[0] != "sql-rdd-101" {
				t.Fatal("canonical application mutated original/census metadata")
			}
		})
	}
}

func TestEnergyPathServiceBoundaryFiltersOnlyAffectedConversions(t *testing.T) {
	nodes := []EnergyExplanationNode{
		epathBoundaryTestNode("end_use.heating", "heating", 30, epathBoundaryTestRestriction()),
		epathBoundaryTestNode("end_use.cooling", "cooling", 40),
		epathBoundaryTestNode("end_use.fans", "fans", 5),
	}
	links := []EnergyPathLink{
		{ID: "heating_conversion_missing_restricted_source", FromID: "load.heating", ToID: "end_use.heating", Relation: "load_to_end_use", FromValue: 15, ToValue: 30, SourceIDs: []string{"sql-rdd-301"}},
		{ID: "heating_conversion_no_sources", FromID: "load.heating", ToID: "end_use.heating", Relation: " LOAD_TO_END_USE ", FromValue: 15, ToValue: 30},
		{ID: "cooling_conversion", FromID: "load.cooling", ToID: "end_use.cooling", Relation: "load_to_end_use", FromValue: 80, ToValue: 40, SourceIDs: []string{"sql-rdd-401"}, RelatedPathIDs: []string{"path.cooling"}},
		{ID: "heating_gas", FromID: "end_use.heating", ToID: "carrier.natural_gas", Relation: "end_use_to_carrier", FromValue: 20, ToValue: 20, SourceIDs: []string{"sql-rdd-201"}},
		{ID: "heating_electric", FromID: "end_use.heating", ToID: "carrier.electricity", Relation: "end_use_to_carrier", FromValue: 10, ToValue: 10, SourceIDs: []string{"sql-rdd-301"}},
		{ID: "fans_electric", FromID: "end_use.fans", ToID: "carrier.electricity", Relation: "end_use_to_carrier", FromValue: 5, ToValue: 5},
		{ID: "driver_heat", FromID: "driver.solar", ToID: "load.heating", Relation: "driver_to_load", FromValue: 4, ToValue: 4},
		{ID: "other_relation", FromID: "context", ToID: "end_use.heating", Relation: "context_only", FromValue: 1, ToValue: 1},
	}
	got := filterEnergyPathServiceBoundaryConversions(nodes, links)
	if !reflect.DeepEqual(got, links[2:]) {
		t.Fatalf("only the two Heating conversion links may be removed: %#v", got)
	}
	got[0].SourceIDs[0], got[0].RelatedPathIDs[0] = "changed", "changed"
	if links[2].SourceIDs[0] != "sql-rdd-401" || links[2].RelatedPathIDs[0] != "path.cooling" || nodes[0].Value != 30 {
		t.Fatal("filter mutated input source/path slices or observed energy")
	}
	if filterEnergyPathServiceBoundaryConversions(nodes, nil) != nil {
		t.Fatal("nil links should remain nil")
	}
}

func TestEnergyPathServiceBoundaryAnnualSameIDMetadataOnlyUnion(t *testing.T) {
	monthly := epathBoundaryTestNode("end_use.heating", "heating", 10)
	monthly.RawValue, monthly.EffectiveValue, monthly.AllocatedValue = 11, 12, 3
	monthly.AllocationApplied, monthly.Period, monthly.Basis = true, "annual", "service_path_allocation"
	monthly.inspectorDecodedFromJSON, monthly.inspectorValuePresence = true, 255
	annual := epathBoundaryTestNode(monthly.ID, "heating", 999, epathBoundaryTestRestriction())
	annual.SourceIDs = []string{"sql-rdd-201"}
	other := epathBoundaryTestNode("not_kept", "cooling", 500, epathBoundaryTestRestriction())
	got := mergeEnergyPathServiceBoundaryNodeMetadata([]EnergyExplanationNode{monthly}, []EnergyExplanationNode{annual, other})
	if len(got) != 1 || !energyPathServiceBoundaryDeniesConversion(got[0]) {
		t.Fatal("same-ID annual metadata was lost before numeric monthly-precedence skip")
	}
	withoutMetadata := got[0]
	withoutMetadata.serviceBoundaryRestrictions = nil
	if !reflect.DeepEqual(withoutMetadata, monthly) {
		t.Fatal("annual fallback altered monthly values, presence mask, or source provenance")
	}
	got[0].serviceBoundaryRestrictions[0].ConsumerSourceIDs[0] = "changed"
	if annual.serviceBoundaryRestrictions[0].ConsumerSourceIDs[0] != "sql-rdd-101" || monthly.serviceBoundaryRestrictions != nil {
		t.Fatal("annual metadata transfer aliased an input")
	}
}

func TestEnergyPathServiceBoundaryExactConsumerProjection(t *testing.T) {
	hw := epathBoundaryTestRestriction()
	hw.EndUse, hw.Carrier = "pumps", "electricity"
	for _, tc := range []struct {
		name  string
		ids   []string
		exact bool
		keep  bool
	}{
		{"exact_hw_consumer", []string{"sql-rdd-101"}, true, true},
		{"exact_disjoint_cw_consumer", []string{"sql-rdd-102"}, true, false},
		{"shared_meter_is_not_a_consumer", []string{"sql-rdd-201"}, true, true},
		{"unqualified_cw", []string{"sql-rdd-102"}, false, true},
		{"empty_cohort", nil, true, true},
		{"invalid_cohort", []string{"not-native"}, true, true},
		{"partially_invalid_cohort", []string{"sql-rdd-102", ""}, true, true},
		{"duplicate_cohort", []string{"sql-rdd-102", "sql-rdd-102"}, true, true},
		{"trace_misused_as_cohort", []string{"sql-rdd-102", "sql-rdd-201"}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := []energyPathServiceBoundaryRestriction{hw}
			before := cloneEnergyPathServiceBoundaryRestrictions(input)
			got := projectEnergyPathServiceBoundaryRestrictions(input, tc.ids, tc.exact)
			if (len(got) > 0) != tc.keep {
				t.Fatalf("restriction retention=%v, want %v", len(got) > 0, tc.keep)
			}
			if len(got) > 0 {
				got[0].ConsumerSourceIDs[0] = "changed"
			}
			if !reflect.DeepEqual(input, before) {
				t.Fatal("projection modified original restrictions")
			}
		})
	}
	meterOnly := hw
	meterOnly.ConsumerSourceIDs = nil
	invalid := hw
	invalid.Reason = "unknown_reason"
	for _, r := range []energyPathServiceBoundaryRestriction{meterOnly, invalid} {
		if len(projectEnergyPathServiceBoundaryRestrictions([]energyPathServiceBoundaryRestriction{r}, []string{"sql-rdd-102"}, true)) != 1 {
			t.Fatal("incomplete or malformed restriction cannot certify disjoint consumption")
		}
	}
}

func TestEnergyPathServiceBoundarySummaryFiltersOnlyRatiosAndAliases(t *testing.T) {
	nodes := []EnergyExplanationNode{
		epathBoundaryTestNode("end_use.heating", "heating", 30, epathBoundaryTestRestriction()),
		epathBoundaryTestNode("end_use.cooling", "cooling", 40),
	}
	energy := []EnergyExplanationSummaryItem{{ID: "observed.heating", Value: 30, RawValue: 30, SourceIDs: []string{"sql-rdd-201", "sql-rdd-301"}}}
	keptRatio := EnergyExplanationSummaryItem{ID: "cooling.ratio", ServiceKind: "cooling", Label: "Heating label must not control semantics", Value: 2, SourceIDs: []string{"sql-rdd-401"}}
	keptLegacy := EnergyExplanationSummaryItem{ID: "kpi.cooling_cop", Kind: "kpi.cooling_cop", Value: 2, SourceIDs: []string{"sql-rdd-401"}}
	summary := EnergyExplanationSummary{
		Schema: "energy-path/v2", Period: "M1",
		Drivers: energy, Loads: energy, EndUses: energy, Carriers: energy,
		Residuals: energy, TopZones: energy,
		EnergyByCarrier: energy, EnergyByEndUse: energy, DeliveredLoadByService: energy,
		HeatDrivers: energy, TopHeatDrivers: energy,
		Quality: &EnergyPathQuality{EndUseToCarrierClosedPct: 100},
		Ratios: []EnergyExplanationSummaryItem{
			{ID: "heating.ratio", ServiceKind: "heating", Value: 0.5},
			{ID: "endpoint.ratio", DenominatorLabel: "end_use.heating", Value: 0.5},
			{ID: "conflicting.kpi", ServiceKind: "cooling", Kind: "kpi.heating_cop", Value: 0.5},
			keptRatio,
		},
		DerivedKPIs: []EnergyExplanationSummaryItem{
			{ID: "kpi.heating_cop", Value: 0.5},
			{ID: "kpi.cooling_cop", Kind: "kpi.heating_cop", Value: 0.5},
			keptLegacy,
		},
	}
	want := summary
	want.Ratios = []EnergyExplanationSummaryItem{keptRatio}
	want.DerivedKPIs = []EnergyExplanationSummaryItem{keptLegacy}
	got := filterEnergyPathServiceBoundarySummary(summary, nodes)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("summary guard altered non-ratio energy or retained unsupported Heating ratios: %#v", got)
	}
	got.Ratios[0].SourceIDs[0], got.DerivedKPIs[0].SourceIDs[0] = "changed", "changed"
	if len(summary.Ratios) != 4 || len(summary.DerivedKPIs) != 3 || summary.Ratios[3].SourceIDs[0] != "sql-rdd-401" || summary.DerivedKPIs[2].SourceIDs[0] != "sql-rdd-401" {
		t.Fatal("summary filter modified input ratio/alias slices")
	}
	if !reflect.DeepEqual(filterEnergyPathServiceBoundarySummary(summary, nil), summary) {
		t.Fatal("nil boundary metadata changed legacy summary behavior")
	}
}

func TestEnergyPathServiceBoundaryMetadataRoundTripPreservesDenial(t *testing.T) {
	for _, reason := range []string{energyPathBoundaryNonZoneDemand, "future_unknown_reason"} {
		r := epathBoundaryTestRestriction()
		r.Reason = reason
		before := unionEnergyPathServiceBoundaryRestrictions([]energyPathServiceBoundaryRestriction{r})
		wire, err := json.Marshal(before)
		if err != nil {
			t.Fatal(err)
		}
		var decoded []energyPathServiceBoundaryRestriction
		if err := json.Unmarshal(wire, &decoded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, decoded) {
			t.Fatal("typed metadata lost reason/original/source identities on its own JSON round trip")
		}
		node := epathBoundaryTestNode("end_use.heating", "heating", 30, decoded...)
		if !energyPathServiceBoundaryDeniesConversion(node) {
			t.Fatal("unknown-version metadata must not become positive conversion authority")
		}
		if (validateEnergyPathServiceBoundaryRestriction(decoded[0]) == nil) != (reason == energyPathBoundaryNonZoneDemand) {
			t.Fatal("validation must distinguish retained malformed metadata from qualified evidence")
		}
	}
}
