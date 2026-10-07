package simulation

import (
	"fmt"
	"reflect"
	"testing"
)

func TestEnergyPathQualityPreparationKeepsPeriodAndScopeEvidenceSeparate(t *testing.T) {
	result := energyPathQualityAllocationContextFixture()
	result.Sources = []EnergyDataSource{
		{ID: "carrier", Name: "Electricity:Facility"},
		{ID: "end-use", Name: "Cooling:Electricity"},
		{ID: "zero", Name: "Cogeneration:Electricity", RawValue: 0},
		{ID: "", Name: "ElectricityProduced:Facility"},
	}
	result.Completeness.SourceAvailability = []EnergySourceAvailabilityEntry{
		{Name: "Electricity:Facility", Level: "energy", Status: "found", SourceIDs: []string{"carrier"}},
		{Name: "Cooling:Electricity", Level: "energy", Status: "missing"},
		{Name: "Cogeneration:Electricity", Level: "energy", Status: "found", SourceIDs: []string{"zero"}},
		{Name: "Water:Facility", Level: "energy", Status: "found"},
		{Name: "ElectricityProduced:Facility", Level: "energy", Status: "found"},
	}
	// Each Zone owns its completeness. Building source availability must not
	// overwrite a Zone's explicit not-requested sentinel or unknown denominator.
	result.ZoneResults[0].Completeness.EnergyUse.Status = "not_requested"
	result.ZoneResults[1].Completeness = result.Completeness
	for _, graph := range append([]EnergyExplanationResult{result}, qualityContextZoneGraphs(result)...) {
		context := prepareEnergyPathQuality(graph)
		periods := []string{"", " annual ", "M1", "m10", "M12", "absent"}
		for _, period := range periods {
			got := buildEnergyPathPeriodQuality(graph, period, context)
			want := BuildEnergyPathQuality(graph, period)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("prepared quality differs for %s/%q: got %+v want %+v", graph.Scope.ZoneName, period, got, want)
			}
		}
	}
	if !refreshEnergyPathQuality(&result) {
		t.Fatal("initial refresh did not produce quality")
	}
	if result.Quality.EndUses.Found != 1 || result.Quality.EndUses.Total != 2 || result.Quality.Carriers.Found != 1 || result.Quality.Carriers.Total != 1 {
		t.Fatalf("requested site groups changed: %+v", result.Quality)
	}
	if result.ZoneResults[0].Quality.EndUses.Status != "not_requested" || result.ZoneResults[2].Quality.EndUses.Total != 0 {
		t.Fatal("scope-local completeness was replaced by Building availability")
	}
	old := result.Quality
	result.Completeness.SourceAvailability[1].Status = "found"
	if !refreshEnergyPathQuality(&result) || result.Quality.EndUses.Found != 2 || old.EndUses.Found != 1 {
		t.Fatal("a later refresh reused stale evidence or mutated previously returned quality")
	}
}

func qualityContextZoneGraphs(result EnergyExplanationResult) []EnergyExplanationResult {
	graphs := make([]EnergyExplanationResult, 0, len(result.ZoneResults))
	for _, zone := range result.ZoneResults {
		graphs = append(graphs, EnergyExplanationResult{
			Scope: zone.Scope, Nodes: zone.Nodes, Links: zone.Links, Periods: zone.Periods,
			Sources: result.Sources, Completeness: zone.Completeness, Reconciliation: zone.Reconciliation,
		})
	}
	return graphs
}

var qualityContextBenchmarkResult *EnergyPathQuality

// Both branches calculate all 13 qualities with the same public semantics.
// Repeated models the previous refresh; prepared removes only repeated
// run-level classification and source identity construction.
func BenchmarkEnergyPathQualityPeriods(b *testing.B) {
	result := EnergyExplanationResult{Scope: EnergyExplanationScope{Kind: "building"}}
	for index := 0; index < 1000; index++ {
		result.Sources = append(result.Sources, EnergyDataSource{
			ID: fmt.Sprintf("surface-%d", index), Name: "Surface Inside Face Conduction Heat Transfer Energy",
		})
	}
	result.Sources = append(result.Sources, EnergyDataSource{ID: "facility", Name: "Electricity:Facility"})
	for month := 1; month <= 12; month++ {
		result.Periods = append(result.Periods, EnergyPeriod{ID: fmt.Sprintf("M%d", month)})
	}
	for _, prepared := range []bool{false, true} {
		name := "repeated"
		if prepared {
			name = "prepared"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for iteration := 0; iteration < b.N; iteration++ {
				if prepared {
					context := prepareEnergyPathQuality(result)
					qualityContextBenchmarkResult = buildEnergyPathPeriodQuality(result, "", context)
					for _, period := range result.Periods {
						qualityContextBenchmarkResult = buildEnergyPathPeriodQuality(result, period.ID, context)
					}
				} else {
					qualityContextBenchmarkResult = BuildEnergyPathQuality(result, "")
					for _, period := range result.Periods {
						qualityContextBenchmarkResult = BuildEnergyPathQuality(result, period.ID)
					}
				}
			}
		})
	}
}
