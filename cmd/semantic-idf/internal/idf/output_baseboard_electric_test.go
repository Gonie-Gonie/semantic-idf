package idf

import "testing"

func TestOutputElectricBaseboardHeatingRecognitionIsExact(t *testing.T) {
	for _, tc := range []struct {
		kind string
		want bool
	}{
		{"ZoneHVAC:Baseboard:Convective:Electric", true},
		{"ZoneHVAC:Baseboard:RadiantConvective:Electric", true},
		{"zonehvac:baseboard:convective:electric", true},
		{"ZoneHVAC:Baseboard:Convective:Water", false},
		{"ZoneHVAC:Baseboard:RadiantConvective:Water", false},
		{"ZoneHVAC:Baseboard:RadiantConvective:Steam", false},
		{"ZoneHVAC:Baseboard:Unsupported:Electric", false},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			doc, err := Parse("Version,25.1;\n" + tc.kind + ",Baseboard;")
			if err != nil {
				t.Fatal(err)
			}
			before := doc.String()
			features := detectOutputFeatures(doc)
			if features.hasHeating != tc.want || features.hasElectricity != tc.want {
				t.Fatalf("exact native feature detection changed: %+v want electric heating=%t", features, tc.want)
			}
			count := 0
			for _, recommendation := range AnalyzeOutput(doc).Recommendations {
				if recommendation.ID == "standard-meter-electricity-heating" {
					count++
				}
			}
			wantCount := 0
			if tc.want {
				wantCount = 1
			}
			if count != wantCount || doc.String() != before {
				t.Fatalf("heating recommendation count=%d want=%d, or original changed", count, wantCount)
			}
		})
	}
}
