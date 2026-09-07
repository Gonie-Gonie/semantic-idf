package simulation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEnergyPathQualityUnknownDenominatorIsNotObservedOverZero(t *testing.T) {
	result := epath130WeightedGraph([]float64{100}, []float64{100})
	result.Sources = []EnergyDataSource{
		{ID: "cooling", Name: "Cooling:Electricity"},
		{ID: "heating", Name: "Heating:NaturalGas"},
		{ID: "electricity", Name: "Electricity:Facility"},
		{ID: "gas", Name: "NaturalGas:Facility"},
	}
	for _, scope := range []EnergyExplanationScope{{Kind: "building"}, {Kind: "zone", ZoneName: "Office"}} {
		result.Scope = scope
		if scope.Kind == "zone" {
			result.Completeness = energyPathDirectZonePartialCompleteness(result.Completeness)
		}
		quality := BuildEnergyPathQuality(result, "annual")
		for _, level := range []EnergyCompletenessLevel{quality.Drivers, quality.Loads, quality.EndUses, quality.Carriers} {
			if level.Found != 0 || level.Total != 0 || level.Status != "partial" {
				t.Fatalf("%s unknown requested count must not become observed/0 or fabricated complete count: %#v", scope.Kind, level)
			}
			if !strings.Contains(level.Message, "unknown") {
				t.Fatalf("observed presence lost its unknown-denominator explanation: %#v", level)
			}
		}
		if !strings.Contains(quality.EndUses.Message, "2 source group(s)") || !strings.Contains(quality.Carriers.Message, "2 source group(s)") {
			t.Fatalf("observed source inventory lost: %#v", quality)
		}
	}
}

func TestEnergyPathQualityKnownSourceDenominatorsStayExact(t *testing.T) {
	result := EnergyExplanationResult{Completeness: EnergyCompleteness{
		HeatDrivers:   EnergyCompletenessLevel{Status: "partial", Found: 12, Total: 18},
		DeliveredLoad: EnergyCompletenessLevel{Status: "partial", Found: 9, Total: 14},
		SourceAvailability: []EnergySourceAvailabilityEntry{
			{Name: "Cooling:Electricity", Level: "energy", Status: "found"},
			{Name: "WaterSystems:Electricity", Level: "energy", Status: "missing"},
			{Name: "Electricity:Facility", Level: "energy", Status: "found"},
			{Name: "NaturalGas:Facility", Level: "energy", Status: "found"},
		},
	}}
	for repeat := 0; repeat < 2; repeat++ {
		quality := BuildEnergyPathQuality(result, "annual")
		for _, check := range []struct {
			level        EnergyCompletenessLevel
			found, total int
		}{{quality.Drivers, 12, 18}, {quality.Loads, 9, 14}, {quality.EndUses, 1, 2}, {quality.Carriers, 2, 2}} {
			if check.level.Found != check.found || check.level.Total != check.total {
				t.Fatalf("known request evidence changed: %#v", check.level)
			}
		}
		data, err := json.Marshal(result.Completeness)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &result.Completeness); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEnergyPathQualityZoneAllocatedCarrierRetainsUnknownDenominatorExplanation(t *testing.T) {
	result := EnergyExplanationResult{
		Schema: energyExplanationSchema,
		Scope:  EnergyExplanationScope{Kind: "zone", ZoneName: "Office"},
		Nodes: []EnergyExplanationNode{{
			ID: "carrier.electricity.office", Level: "carrier", ScaleDomain: "site",
			ZoneName: "Office", Carrier: "electricity", Basis: "service_path_allocation", Value: 25, Unit: "kWh", SourceIDs: []string{"facility"},
		}},
		Sources: []EnergyDataSource{{ID: "facility", Name: "Electricity:Facility"}},
	}
	result.Completeness = energyPathDirectZonePartialCompleteness(result.Completeness)
	for repeat := 0; repeat < 2; repeat++ {
		level := BuildEnergyPathQuality(result, "annual").Carriers
		if level.Status != "partial" || level.Found != 0 || level.Total != 0 {
			t.Fatalf("allocated Zone subtotal fabricated requested coverage: %#v", level)
		}
		for _, explanation := range []string{"unknown", "1 source group(s)", "subtotals do not establish complete"} {
			if !strings.Contains(level.Message, explanation) {
				t.Fatalf("allocated Zone subtotal lost %q explanation: %#v", explanation, level)
			}
		}
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
	}
}
