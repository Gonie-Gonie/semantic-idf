package simulation

import (
	"encoding/json"
	"reflect"
	"testing"
)

// These literal cases exercise the public quality path and stored-result
// refresh. A complete Hourly parent never replaces a missing Monthly request.
func TestCogenerationQualityRefreshRetainsNativeFrequencyAvailability(t *testing.T) {
	for _, tc := range []struct {
		name        string
		monthly     bool
		wantFound   int
		wantStatus  string
		wantSources []string
	}{
		{"native known zero", true, 1, "complete", []string{"sql-rdd-61", "sql-rdd-62"}},
		{"native Hourly only", false, 0, "missing", []string{"sql-rdd-62"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const parent = "Cogeneration:Electricity"
			monthly := cogenCoverageReviewSource("sql-rdd-61", parent, "Monthly", 0, true)
			hourly := cogenCoverageReviewSource("sql-rdd-62", parent, "Hourly", 0, true)
			sources := []EnergyDataSource{monthly, hourly}
			if !tc.monthly {
				sources = sources[1:]
			}
			plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, OutputObjects: []PurposeOutputObject{
				cogenCoverageReviewRequest(parent, "Monthly"), cogenCoverageReviewRequest(parent, "Hourly"),
			}}
			completeness := buildEnergyExplanationCompleteness(nil, sources, plan, 0)
			applyEnergyPathCogenerationAvailability(&completeness, plan, energyPathCogenerationReadResult{Enabled: true, SourceSnapshots: sources})
			// The generic expected-output identity remains name-based. The
			// quality identity is physical and carrier-qualified; neither is
			// allowed to alias ancillary electricity or produced electricity.
			if got := expectedEnergyExplanationOutputGroupKey(parent, "energy"); got != "energy|cogeneration:electricity" {
				t.Fatalf("unexpected requested-parent identity: %q", got)
			}
			parentEntries := 0
			for _, entry := range completeness.SourceAvailability {
				if entry.Name == parent {
					parentEntries++
					if entry.Status != map[bool]string{true: "found", false: "missing"}[tc.monthly] || !reflect.DeepEqual(entry.SourceIDs, tc.wantSources) {
						t.Fatalf("exact parent availability/source trace lost: %+v", entry)
					}
				}
			}
			if parentEntries != 1 {
				t.Fatalf("parent availability duplicated: %d", parentEntries)
			}
			stale := func() *EnergyPathQuality {
				return &EnergyPathQuality{EndUses: EnergyCompletenessLevel{Level: "end_use", Found: 99, Total: 99, Status: "complete"}}
			}
			period := func() EnergyPeriod {
				return EnergyPeriod{ID: "M01", Quality: stale(), Summary: &EnergyExplanationSummary{Quality: stale()}}
			}
			result := EnergyExplanationResult{
				Scope: EnergyExplanationScope{Kind: "building"}, Sources: sources, Completeness: completeness,
				Quality: stale(), Periods: []EnergyPeriod{period()},
				ZoneResults: []EnergyExplanationZoneResult{{
					Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Z"}, Completeness: completeness,
					Quality: stale(), Summary: EnergyExplanationSummary{Quality: stale()}, Periods: []EnergyPeriod{period()},
				}},
			}
			before, err := json.Marshal(struct {
				Sources      []EnergyDataSource
				Completeness EnergyCompleteness
			}{result.Sources, result.Completeness})
			if err != nil {
				t.Fatal(err)
			}
			if !refreshEnergyPathQuality(&result) {
				t.Fatal("stale saved quality was not refreshed")
			}
			qualities := []*EnergyPathQuality{result.Quality, result.Periods[0].Quality, result.Periods[0].Summary.Quality,
				result.ZoneResults[0].Quality, result.ZoneResults[0].Summary.Quality,
				result.ZoneResults[0].Periods[0].Quality, result.ZoneResults[0].Periods[0].Summary.Quality}
			for i, quality := range qualities {
				if quality == nil || quality.EndUses.Found != tc.wantFound || quality.EndUses.Total != 1 || quality.EndUses.Status != tc.wantStatus {
					t.Fatalf("refresh view %d reused stale/frequency-substituted parent quality: %+v", i, quality)
				}
			}
			if refreshEnergyPathQuality(&result) {
				t.Fatal("same native evidence did not refresh idempotently")
			}
			after, err := json.Marshal(struct {
				Sources      []EnergyDataSource
				Completeness EnergyCompleteness
			}{result.Sources, result.Completeness})
			if err != nil || string(before) != string(after) {
				t.Fatal("quality refresh rewrote source/availability provenance")
			}
		})
	}
}
