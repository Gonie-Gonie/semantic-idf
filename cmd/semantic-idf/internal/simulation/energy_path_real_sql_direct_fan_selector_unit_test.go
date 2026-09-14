package simulation

import (
	"encoding/json"
	"testing"
)

func TestEnergyPathRealSQLNativeFanScalarSelectorsKeepObservedZeroProof(t *testing.T) {
	path, model, _ := epathSQLNativeZoneEquipmentUnit(t)
	frames, _ := epathSQLDirectHVACSourceUnitFrames(t, path, model)
	var compiled epathSQLModelChecks
	if err := epathSQLModelDirectFanChecks(frames, model, &compiled); err != nil {
		t.Fatal(err)
	}
	for _, period := range []string{"M1", "M2"} {
		t.Run(period, func(t *testing.T) {
			var checks epathSQLModelChecks
			for _, check := range compiled.Rows {
				if check.DirectFan == nil || check.Item.Period != period {
					continue
				}
				if err := epathValidateOracleTarget(check.Item.Target, check.Item.Unit); err != nil {
					t.Fatalf("native scalar selector cannot pass the diagnostic/pending gate: %v", err)
				}
				field := check.Item.Target.Field
				rawField := field == "rawValue" || field == "effectiveValue"
				if check.Item.Target.AllowPrunedZero == rawField || check.OptionalPresentation != (rawField && period == "M2") || len(check.DirectFan.Sources) != 1 {
					t.Fatalf("native scalar %s lost its strict presentation/source contract", field)
				}
				checks.Rows = append(checks.Rows, check)
			}
			if len(checks.Rows) != 4 {
				t.Fatalf("native fan must retain all four scalar obligations: %d", len(checks.Rows))
			}
			// These are the hand-written SQL fixture observations: M1=10,
			// M2=0. No candidate output supplies either expected quantity.
			value := 0.0
			if period == "M1" {
				value = 10
			}
			makeBundle := func(present bool) PurposeResultBundle {
				graph := EnergyPeriod{ID: period, Kind: "monthly"}
				if present {
					graph.Nodes = []EnergyExplanationNode{
						{ID: "fans", Level: "end_use", EndUse: "fans", Basis: "direct_zone_energy", Value: value, RawValue: value, EffectiveValue: value, AllocatedValue: value, Unit: "kWh", ScaleDomain: "site", AggregationBasis: "model_total", Period: period, ZoneName: "Office", SourceIDs: []string{"sql-rdd-60"}},
						{ID: "electricity", Level: "carrier", Carrier: "electricity", Value: value, Unit: "kWh", ScaleDomain: "site", Period: period, ZoneName: "Office"},
					}
					if value == 0 {
						// A legacy Go zero does not prove scalar presence. This
						// incoming wire explicitly records all three native zeros.
						if err := json.Unmarshal([]byte(`{"id":"fans","level":"end_use","endUse":"fans","basis":"direct_zone_energy","value":0,"rawValue":0,"effectiveValue":0,"allocatedValue":0,"unit":"kWh","scaleDomain":"site","aggregationBasis":"model_total","zoneName":"Office","sourceIds":["sql-rdd-60"]}`), &graph.Nodes[0]); err != nil {
							t.Fatal(err)
						}
						graph.Nodes[0].Period = period
					}
					graph.Links = []EnergyPathLink{{ID: "fan-carrier", FromID: "fans", ToID: "electricity", Relation: "direct_end_use_to_carrier", Basis: "direct_zone_energy", FromValue: value, ToValue: value, FromUnit: "kWh", ToUnit: "kWh", Period: period, ZoneName: "Office", SourceIDs: []string{"sql-rdd-60"}}}
				}
				return PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{
					Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"},
					Sources:     []EnergyDataSource{epathSQLDirectHVACSourceUnitCandidate(frames.DirectHVACSourceIdentities[60])},
					ZoneResults: []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}, Periods: []EnergyPeriod{graph}}},
				}}
			}
			for _, present := range []bool{true, false} {
				failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, makeBundle(present), checks)
				if present || period == "M2" {
					if len(failures) != 0 {
						t.Fatalf("known native fan %s, present=%v: %v", period, present, failures)
					}
				} else if len(failures) != 4 {
					t.Fatalf("positive fan pruning must fail every retained scalar: %v", failures)
				}
			}
			if period == "M2" {
				// The ordinary non-driver writer omits zero-valued scalars.
				// Such a present-but-incomplete node is not whole-node pruning.
				incomplete := makeBundle(true)
				node := &incomplete.EnergyExplanation.ZoneResults[0].Periods[0].Nodes[0]
				encoded, err := json.Marshal(*node)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(encoded, node); err != nil {
					t.Fatal(err)
				}
				failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, incomplete, checks)
				if len(failures) != 3 || !node.inspectorDecodedFromJSON || node.inspectorValuePresence != 0 {
					t.Fatalf("omitted wire fields must fail raw/effective/allocated only: %v", failures)
				}
			}
			missingSource := makeBundle(period == "M1")
			missingSource.EnergyExplanation.Sources = nil
			if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, missingSource, checks); len(failures) != 4 {
				t.Fatalf("absent native source masqueraded as measured %s on a scalar: %v", period, failures)
			}
			for _, check := range checks.Rows {
				if check.Item.Target.Field != "rawValue" && check.Item.Target.Field != "effectiveValue" {
					continue
				}
				if err := epathCheckSQLModelPresentation(makeBundle(true), check, nil); err == nil {
					t.Fatalf("present fan node may not omit %s even at measured zero", check.Item.Target.Field)
				}
			}
			for _, field := range []string{"rawValue", "effectiveValue"} {
				changed := makeBundle(true)
				node := &changed.EnergyExplanation.ZoneResults[0].Periods[0].Nodes[0]
				if field == "rawValue" {
					node.RawValue++
				} else {
					node.EffectiveValue++
				}
				if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, changed, checks); len(failures) != 4 {
					t.Fatalf("wrong %s evaded one of the four native consumer proofs: %v", field, failures)
				}
			}
		})
	}
}
