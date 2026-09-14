package simulation

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEPATH051MultiplierIgnoresEditableFieldComments(t *testing.T) {
	for _, mutation := range []string{"none", "Zone factor", "Group list", "Group factor", "Space owner", "all"} {
		t.Run(mutation, func(t *testing.T) {
			doc := parsePurposePlanFixture(t, energyMultiplierFixtureIDF)
			for i := range doc.Objects {
				object := &doc.Objects[i]
				switch strings.ToLower(object.Type) {
				case "zone":
					if mutation == "Zone factor" || mutation == "all" {
						object.Fields[5].Comment = "Multiplier"
					}
				case "zonegroup":
					if mutation == "Group list" || mutation == "all" {
						object.Fields[0].Comment = "Zone List Name"
					}
					if mutation == "Group factor" || mutation == "all" {
						object.Fields[1].Comment = "Zone List Multiplier"
					}
				case "space":
					if mutation == "Space owner" || mutation == "all" {
						object.Fields[0].Comment = "Zone Name"
					}
				}
			}
			before := doc.String()
			context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
			for _, key := range []string{"Office", "Office Space", "Office Ideal Loads"} {
				record, known := context.Multipliers.resolve(key)
				if !known || record.ZoneName != "Office" || record.ZoneMultiplier != 2 || record.GroupMultiplier != 5 || record.effectiveMultiplier() != 10 {
					t.Fatalf("comment changed native ownership/factor for %s: %#v", key, record)
				}
			}
			series := []energyExplanationSeries{{
				Level: "load", Kind: "load.zone_cooling", Label: "Cooling", Unit: "kWh", ServiceKind: "cooling", PathType: "zone", ZoneName: "Office Space",
				SourceIDs: []string{"load"}, sourceName: "Zone Air System Sensible Cooling Energy", SourceName: "Zone Air System Sensible Cooling Energy", sourceFrequency: "Monthly", Total: 10, Monthly: map[int]float64{1: 10},
			}}
			result := UpgradeEnergyExplanationV1(buildEnergyExplanationResultWithDriverContext(series, []EnergyDataSource{{ID: "load", Name: "Zone Air System Sensible Cooling Energy"}}, &PurposeRunPlan{}, context))
			for pass := 0; pass < 3; pass++ {
				if pass > 0 {
					wire, err := json.Marshal(result)
					if err != nil {
						t.Fatal(err)
					}
					var decoded EnergyExplanationResult
					if err := json.Unmarshal(wire, &decoded); err != nil {
						t.Fatal(err)
					}
					result = decoded
				}
				node := energyPathV2NodeByID(result.Nodes, "load.cooling.building")
				if node == nil || node.Value != 100 || node.RawValue != 10 || node.EffectiveValue != 100 || node.Multiplier != 10 {
					t.Fatalf("comment changed Building energy: %#v", node)
				}
				source := energyExplanationSourceByID(result.Sources, "load")
				if source == nil || source.RawValue != 10 || source.EffectiveValue != 100 || source.EffectiveMultiplier != 10 || source.MultiplierApplication != energyMultiplierRequiresZone || len(source.ScopeDetails) != 1 {
					t.Fatalf("comment changed source authority: %#v", source)
				}
				detail := source.ScopeDetails[0]
				if detail.Scope.ZoneName != "Office" || detail.RawValue != 10 || detail.EffectiveValue != 100 || detail.EffectiveMultiplier != 10 {
					t.Fatalf("comment changed precomputed Zone contribution: %#v", detail)
				}
			}
			if doc.String() != before {
				t.Fatal("native multiplier inspection changed source document")
			}
		})
	}
}
