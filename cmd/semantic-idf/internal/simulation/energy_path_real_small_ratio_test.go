package simulation

import (
	"encoding/json"
	"math"
	"testing"
)

func TestEnergyPathRealSmallPositiveConversionRatioRemainsKnown(t *testing.T) {
	for _, test := range []struct {
		name      string
		from, to  float64
		wantRatio float64
	}{
		// Actual shared-loader June heating, LargeOffice25.1. Both paired
		// quantities are present; the old ratio rounding erased its availability.
		{"real June heating", .157, 1501.071, .157 / 1501.071},
		{"below three decimal half step", .49, 1000, .49 / 1000},
		{"very small but representable", .001, 10000000, 1e-10},
		{"ordinary rounding unchanged", .5, 1000, .001},
		{"ordinary efficiency unchanged", 85, 100, .85},
	} {
		t.Run(test.name, func(t *testing.T) {
			load := EnergyExplanationNode{ID: "load.heating.building", Level: "load", ServiceKind: "heating", Value: test.from, Unit: "kWh", ScaleDomain: "thermal"}
			use := EnergyExplanationNode{ID: "end_use.heating.building", Level: "end_use", EndUse: "heating", Value: test.to, Unit: "kWh", ScaleDomain: "site", endUseCarriers: []string{"natural_gas"}}
			link := EnergyPathLink{FromID: load.ID, ToID: use.ID, Relation: "load_to_end_use", ServiceKind: "heating", Basis: "service_path_allocation", FromValue: test.from, ToValue: test.to, FromUnit: "kWh", ToUnit: "kWh"}
			setEnergyPathConversionRatioKind(&link, &load, &use)
			finalizeEnergyPathLinkRatio(&link)
			if link.RatioKind != "efficiency" || link.RatioLabel != "Efficiency" || link.Ratio != test.wantRatio || link.Ratio <= 0 {
				t.Fatalf("known paired ratio lost its kind/value: %+v, want %.15g", link, test.wantRatio)
			}
			if link.FromValue != test.from || link.ToValue != test.to {
				t.Fatal("ratio correction changed paired energy quantities")
			}
			encoded, err := json.Marshal(link)
			if err != nil {
				t.Fatal(err)
			}
			var restored EnergyPathLink
			if err := json.Unmarshal(encoded, &restored); err != nil {
				t.Fatal(err)
			}
			finalizeEnergyPathLinkRatio(&restored)
			if restored.Ratio != link.Ratio || restored.RatioKind != link.RatioKind || restored.RatioLabel != link.RatioLabel {
				t.Fatal("JSON reload or repeated finalization erased the positive ratio")
			}
		})
	}
}

func TestEnergyPathRealSmallRatioDoesNotPromoteInvalidEndpoints(t *testing.T) {
	for _, pair := range [][2]float64{{0, 1501.071}, {.157, 0}, {-.157, 1501.071}, {.157, -1}, {math.NaN(), 1}, {1, math.Inf(1)}, {.0001, 1}} {
		link := EnergyPathLink{Relation: "load_to_end_use", FromValue: pair[0], ToValue: pair[1], FromUnit: "kWh", ToUnit: "kWh", RatioKind: "efficiency", RatioLabel: "Efficiency", Ratio: 1}
		finalizeEnergyPathLinkRatio(&link)
		if link.Ratio != 0 || link.RatioKind != "" || link.RatioLabel != "" {
			t.Fatalf("invalid or zero serialized endpoint advertised a ratio: %+v", link)
		}
	}
}
