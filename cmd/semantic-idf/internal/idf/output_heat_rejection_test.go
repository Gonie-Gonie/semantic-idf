package idf

import "testing"

func TestOutputCoolingTowerRecommendsHeatRejectionMeter(t *testing.T) {
	for _, kind := range []string{"CoolingTower:SingleSpeed", "CoolingTower:TwoSpeed", "CoolingTower:VariableSpeed"} {
		t.Run(kind, func(t *testing.T) {
			doc, err := Parse("Version,25.1;\n" + kind + ",Actual Tower;")
			if err != nil {
				t.Fatal(err)
			}
			features := detectOutputFeatures(doc)
			if !features.hasHeatRejection || !features.hasElectricity || !features.hasCooling {
				t.Fatalf("cooling-name match hid heat-rejection electricity: %#v", features)
			}
			for _, recommendation := range AnalyzeOutput(doc).Recommendations {
				if recommendation.ID == "standard-meter-electricity-heat-rejection" {
					return
				}
			}
			t.Fatal("actual tower lacks a heat-rejection meter recommendation")
		})
	}
}
