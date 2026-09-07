package simulation

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func epathOracleCandidateFixture() (PurposeResultBundle, epathRealOracleMetricRecipe, epathRealOracleMetric) {
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}, Nodes: []EnergyExplanationNode{{ID: "wall", Level: "driver", Kind: "heat.surface", DriverCategory: "surface.exterior_walls", ServiceKind: "cooling", Value: 10, RawValue: 12, EffectiveValue: 24, AllocatedValue: 10, AllocationApplied: true, Unit: "kWh", ScaleDomain: "thermal", Period: "annual", Basis: "heat_balance_share"}}}}
	item := epathRealOracleMetricRecipe{Key: "wall", Group: "drivers", Scope: "building", Period: "annual", Unit: "kWh", Target: epathRealOracleTarget{Collection: "nodes", Field: "value", Level: "driver", Category: "surface.exterior_walls", Service: "cooling", Unit: "kWh", ScaleDomain: "thermal", Basis: "heat_balance_share"}}
	want := epathRealOracleMetric{Key: item.Key, Group: item.Group, Scope: item.Scope, Period: item.Period, Unit: item.Unit, Value: epathOracleNumber(10)}
	return bundle, item, want
}

func TestEnergyPathRealOracleCandidateRejectsSemanticMismatch(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*PurposeResultBundle, *epathRealOracleMetricRecipe)
	}{
		{"mixed numeric unit", func(b *PurposeResultBundle, _ *epathRealOracleMetricRecipe) { b.EnergyExplanation.Nodes[0].Unit = "MJ" }},
		{"wrong scale domain", func(b *PurposeResultBundle, _ *epathRealOracleMetricRecipe) {
			b.EnergyExplanation.Nodes[0].ScaleDomain = "site"
		}},
		{"wrong root scope", func(b *PurposeResultBundle, _ *epathRealOracleMetricRecipe) { b.EnergyExplanation.Scope.Kind = "zone" }},
		{"wrong root schema", func(b *PurposeResultBundle, _ *epathRealOracleMetricRecipe) {
			b.EnergyExplanation.Schema = energyExplanationV1Schema
		}},
		{"unknown provenance Zone", func(b *PurposeResultBundle, _ *epathRealOracleMetricRecipe) {
			b.EnergyExplanation.Nodes[0].ZoneName = "Unrecorded"
		}},
		{"duplicate node identity", func(b *PurposeResultBundle, _ *epathRealOracleMetricRecipe) {
			b.EnergyExplanation.Nodes = append(b.EnergyExplanation.Nodes, b.EnergyExplanation.Nodes[0])
		}},
		{"stale period", func(b *PurposeResultBundle, _ *epathRealOracleMetricRecipe) {
			b.EnergyExplanation.Nodes[0].Period = "M1"
		}},
		{"invalid monthly selector", func(_ *PurposeResultBundle, i *epathRealOracleMetricRecipe) { i.Period = "M01" }},
		{"unsupported ignored selector", func(_ *PurposeResultBundle, i *epathRealOracleMetricRecipe) {
			i.Target.Relation = "source_correspondence"
		}},
		{"implicit multiple sum", func(b *PurposeResultBundle, _ *epathRealOracleMetricRecipe) {
			n := b.EnergyExplanation.Nodes[0]
			n.ID = "second"
			b.EnergyExplanation.Nodes = append(b.EnergyExplanation.Nodes, n)
		}},
		{"duplicate monthly wrapper", func(b *PurposeResultBundle, i *epathRealOracleMetricRecipe) {
			i.Period = "M1"
			b.EnergyExplanation.Periods = []EnergyPeriod{{ID: "M1"}, {ID: "M1"}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, i, w := epathOracleCandidateFixture()
			test.change(&b, &i)
			if _, err := epathReadOracleCandidate(b, i, w); err == nil {
				t.Fatal("semantic mismatch accepted merely because values matched")
			}
		})
	}
	b, i, w := epathOracleCandidateFixture()
	b.EnergyExplanation.AvailableZones = []string{"Office"}
	b.EnergyExplanation.Nodes[0].ZoneName = "Office"
	if _, err := epathReadOracleCandidate(b, i, w); err != nil {
		t.Fatalf("Building contributing-Zone provenance falsely rejected: %v", err)
	}
}

func TestEnergyPathRealOracleCandidateOptionalPresenceAndPrunedZero(t *testing.T) {
	for _, field := range []string{"rawValue", "effectiveValue", "allocatedValue"} {
		for _, value := range []string{"", `,"` + field + `":null`, `,"` + field + `":0`} {
			b, i, w := epathOracleCandidateFixture()
			raw := `{"id":"wall","level":"driver","kind":"heat.surface","driverCategory":"surface.exterior_walls","serviceKind":"cooling","value":10,"unit":"kWh","scaleDomain":"thermal","period":"annual","basis":"heat_balance_share","allocationApplied":true` + value + `}`
			if err := json.Unmarshal([]byte(raw), &b.EnergyExplanation.Nodes[0]); err != nil {
				t.Fatal(err)
			}
			i.Target.Field = field
			actual, err := epathReadOracleCandidate(b, i, w)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasSuffix(value, ":0") {
				if actual == nil || *actual != 0 {
					t.Fatalf("explicit %s zero lost", field)
				}
			} else if actual != nil {
				t.Fatalf("missing/null %s became known zero", field)
			}
		}
	}
	b, i, w := epathOracleCandidateFixture()
	b.EnergyExplanation.Nodes = nil
	w.Value = epathOracleNumber(0)
	if actual, err := epathReadOracleCandidate(b, i, w); err != nil || actual != nil {
		t.Fatalf("absent presentation became reported zero: %v %v", actual, err)
	}
	i.Target.AllowPrunedZero = true
	if actual, err := epathReadOracleCandidate(b, i, w); err != nil || actual == nil || *actual != 0 {
		t.Fatalf("independently known prunable contribution zero lost: %v %v", actual, err)
	}
	i.Target.Field = "rawValue"
	if _, err := epathReadOracleCandidate(b, i, w); err == nil {
		t.Fatal("pruned presentation falsely proves raw source zero")
	}
}

func TestEnergyPathRealOracleCandidateExactReconciliation(t *testing.T) {
	b, i, w := epathOracleCandidateFixture()
	b.EnergyExplanation.Reconciliation = []EnergyReconciliation{{ID: "electricity-row", Level: "carrier", Period: "annual", Unit: "kWh", ExpectedValue: 100}, {ID: "gas-row", Level: "carrier", Period: "annual", Unit: "kWh", ExpectedValue: 200}}
	i.Target = epathRealOracleTarget{Collection: "reconciliation", Field: "expectedValue", Level: "carrier", ID: "electricity-row", Unit: "kWh"}
	actual, err := epathReadOracleCandidate(b, i, w)
	if err != nil || actual == nil || *actual != 100 {
		t.Fatalf("exact carrier row was pooled: %v %v", actual, err)
	}
	i.Target.Carrier = "electricity"
	if _, err := epathReadOracleCandidate(b, i, w); err == nil {
		t.Fatal("nonexistent reconciliation Carrier field was silently ignored")
	}
}

func TestEnergyPathRealOracleCandidatePairedRatioNotMean(t *testing.T) {
	b, i, w := epathOracleCandidateFixture()
	b.EnergyExplanation.Nodes = []EnergyExplanationNode{{ID: "load-a", Level: "load", ServiceKind: "cooling", ScaleDomain: "thermal", Unit: "kWh", Period: "annual"}, {ID: "use-a", Level: "end_use", ServiceKind: "cooling", ScaleDomain: "site", Unit: "kWh", Period: "annual"}, {ID: "load-b", Level: "load", ServiceKind: "cooling", ScaleDomain: "thermal", Unit: "kWh", Period: "annual"}, {ID: "use-b", Level: "end_use", ServiceKind: "cooling", ScaleDomain: "site", Unit: "kWh", Period: "annual"}}
	b.EnergyExplanation.Sources = []EnergyDataSource{{ID: "observed-sql"}}
	b.EnergyExplanation.Links = []EnergyPathLink{{ID: "a", FromID: "load-a", ToID: "use-a", Relation: "load_to_end_use", ServiceKind: "cooling", Period: "annual", FromValue: 100, ToValue: 25, FromUnit: "kWh", ToUnit: "kWh", Ratio: 4, RatioKind: "load_to_site_energy", SourceIDs: []string{"observed-sql"}}, {ID: "b", FromID: "load-b", ToID: "use-b", Relation: "load_to_end_use", ServiceKind: "cooling", Period: "annual", FromValue: 100, ToValue: 100, FromUnit: "kWh", ToUnit: "kWh", Ratio: 1, RatioKind: "load_to_site_energy", SourceIDs: []string{"observed-sql"}}}
	i.Unit = "ratio"
	i.Target = epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Service: "cooling", FromUnit: "kWh", ToUnit: "kWh", RatioKind: "load_to_site_energy", Aggregate: "sum"}
	actual, err := epathReadOracleCandidate(b, i, w)
	if err != nil || actual == nil || *actual != 1.6 {
		t.Fatalf("paired ratio should be200/125, not mean2.5: %v %v", actual, err)
	}
	b.EnergyExplanation.Links[1].Ratio = 2
	if _, err := epathReadOracleCandidate(b, i, w); err == nil {
		t.Fatal("stored ratio mismatch ignored")
	}
	b.EnergyExplanation.Links[1].Ratio = 1
	b.EnergyExplanation.Links[1].ToUnit = "MJ"
	if _, err := epathReadOracleCandidate(b, i, w); err == nil {
		t.Fatal("mixed ratio denominator units summed")
	}
}

func TestEnergyPathRealOracleCandidateCountsUnknownAndTolerance(t *testing.T) {
	b, i, w := epathOracleCandidateFixture()
	i.Unit = "count"
	i.Target = epathRealOracleTarget{Collection: "quality", Field: "loads"}
	w.Unit = "count"
	b.EnergyExplanation.Quality = &EnergyPathQuality{Loads: EnergyCompletenessLevel{Status: "missing", Found: 0, Total: 14}}
	actual, err := epathReadOracleCandidate(b, i, w)
	if err != nil || actual == nil || *actual != 0 {
		t.Fatalf("requested missing0/14 must be known zero count: %v %v", actual, err)
	}
	b.EnergyExplanation.Quality.Loads = EnergyCompletenessLevel{Status: "not_requested"}
	if actual, err := epathReadOracleCandidate(b, i, w); err != nil || actual != nil {
		t.Fatalf("not requested became0: %v %v", actual, err)
	}
	w.Found = new(int)
	if _, err := epathReadOracleCandidate(b, i, w); err == nil {
		t.Fatal("one-sided count presence accepted")
	}
	w.Found = nil
	b.EnergyExplanation.Quality.Loads = EnergyCompletenessLevel{Status: "partial", Found: 15, Total: 14}
	if _, err := epathReadOracleCandidate(b, i, w); err == nil {
		t.Fatal("found exceeds total")
	}
	if err := epathCompareOracleNumber(epathOracleNumber(999), epathOracleNumber(0), math.NaN(), 0); err == nil {
		t.Fatal("NaN tolerance bypassed numeric oracle")
	}
}
