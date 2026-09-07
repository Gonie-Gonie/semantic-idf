package simulation

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestEPATH130EmptyGraphKeepsNotRequestedSummary(t *testing.T) {
	result := EnergyExplanationResult{
		Schema: energyExplanationSchema,
		Scope:  EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
		Completeness: EnergyCompleteness{
			HeatDrivers:   EnergyCompletenessLevel{Status: "not_requested"},
			DeliveredLoad: EnergyCompletenessLevel{Status: "not_requested"},
			EnergyUse:     EnergyCompletenessLevel{Status: "not_requested"},
		},
	}
	refreshEnergyPathQuality(&result)
	summary := buildEnergyExplanationSummary(result)
	if summary.Schema != energyExplanationSummarySchema || summary.Quality == nil {
		t.Fatalf("empty graph discarded its quality: %+v", summary)
	}
	for _, level := range []EnergyCompletenessLevel{summary.Quality.Drivers, summary.Quality.Loads, summary.Quality.EndUses, summary.Quality.Carriers, summary.Quality.Ratios} {
		if level.Status != "not_requested" {
			t.Fatalf("unrequested %s stage presented as %s", level.Level, level.Status)
		}
	}
	data, err := json.Marshal(PurposeResultBundle{EnergyExplanation: result})
	if err != nil {
		t.Fatal(err)
	}
	var restored PurposeResultBundle
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.EnergyExplanationSummary.Quality, restored.EnergyExplanation.Quality) {
		t.Fatal("empty stored graph lost quality in its sibling summary")
	}
}

func TestEPATH130RefreshReplacesQualityWithoutMutatingOldPointers(t *testing.T) {
	stale := &EnergyPathQuality{DriverToLoadClosedPct: 999, Drivers: EnergyCompletenessLevel{Status: "complete"}}
	result := EnergyExplanationResult{
		Schema:  energyExplanationSchema,
		Quality: stale,
		Periods: []EnergyPeriod{{ID: "M1", Quality: stale, Summary: &EnergyExplanationSummary{Quality: stale}}},
		ZoneResults: []EnergyExplanationZoneResult{{
			Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Core"}, Quality: stale,
			Summary: EnergyExplanationSummary{Quality: stale},
			Periods: []EnergyPeriod{{ID: "M1", Quality: stale, Summary: &EnergyExplanationSummary{Quality: stale}}},
		}},
	}
	if !refreshEnergyPathQuality(&result) {
		t.Fatal("stale quality was not detected")
	}
	if stale.DriverToLoadClosedPct != 999 || stale.Drivers.Status != "complete" {
		t.Fatal("refresh mutated old shared quality pointer")
	}
	qualities := []*EnergyPathQuality{result.Quality, result.Periods[0].Quality, result.Periods[0].Summary.Quality,
		result.ZoneResults[0].Quality, result.ZoneResults[0].Summary.Quality,
		result.ZoneResults[0].Periods[0].Quality, result.ZoneResults[0].Periods[0].Summary.Quality}
	for _, quality := range qualities {
		if quality == nil || quality == stale || quality.DriverToLoadClosedPct == 999 {
			t.Fatalf("stale nested quality survived refresh: %+v", quality)
		}
	}
	if refreshEnergyPathQuality(&result) {
		t.Fatal("unchanged quality projection was not idempotent")
	}
	data, err := json.Marshal(PurposeResultBundle{EnergyExplanation: result,
		EnergyExplanationSummary: EnergyExplanationSummary{Schema: energyExplanationSummarySchema, Quality: stale,
			Carriers: []EnergyExplanationSummaryItem{{ID: "stale", Value: 999}}}})
	if err != nil {
		t.Fatal(err)
	}
	var restored PurposeResultBundle
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.EnergyExplanation.Quality, restored.EnergyExplanationSummary.Quality) ||
		len(restored.EnergyExplanationSummary.Carriers) != 0 {
		t.Fatal("stored sibling summary kept stale quality or graph items")
	}
}
