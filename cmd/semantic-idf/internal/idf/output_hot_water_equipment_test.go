package idf

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOutputHotWaterEquipmentResourceRecognitionIsExact(t *testing.T) {
	for _, tc := range []struct {
		kind string
		want bool
	}{
		{"HotWaterEquipment", true},
		{"hotwaterequipment", true},
		{"HotWaterEquipment:Unsupported", false},
		{"SteamEquipment", false},
		{"GasEquipment", false},
		{"ElectricEquipment", false},
		{"OtherEquipment", false},
		{"WaterUse:Equipment", false},
		{"ZoneHVAC:Baseboard:Convective:Electric", false},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			doc, err := Parse("Version,25.1;\n" + tc.kind + ",Equipment;")
			if err != nil {
				t.Fatal(err)
			}
			before := doc.String()
			features := detectOutputFeatures(doc)
			if features.hasHotWaterEquipment != tc.want {
				t.Fatalf("typed HotWaterEquipment recognition = %t, want %t", features.hasHotWaterEquipment, tc.want)
			}
			if tc.want && (features.hasHeating || features.hasDistrictHeating || features.hasDistrictHeatWater || features.hasDistrictHeatSteam || features.hasSteam) {
				t.Fatalf("internal equipment polluted HVAC heating or steam evidence: %+v", features)
			}
			report := AnalyzeOutput(doc)
			for id, key := range hotWaterOutputRecommendationKeys() {
				item := findOutputRecommendation(report, id)
				if (item != nil) != tc.want {
					t.Fatalf("%s presence=%t, want %t", id, item != nil, tc.want)
				}
				if item != nil && (item.ObjectType != "Output:Meter" || outputFieldValue(item.Fields, "Key Name") != key || outputFieldValue(item.Fields, "Reporting Frequency") != "Monthly" || !outputRecommendationHasTag(*item, standardOutputTag)) {
					t.Fatalf("incorrect native meter identity/frequency: %+v", item)
				}
			}
			if doc.String() != before {
				t.Fatal("feature/recommendation discovery mutated original input")
			}
		})
	}
}

func TestOutputHotWaterEquipmentDoesNotTurnOtherHeatingIntoDistrictHeating(t *testing.T) {
	for _, tc := range []struct {
		name              string
		extra             string
		wantHeating       bool
		wantWaterHeating  bool
		wantLegacyHeating bool
		wantSteamHeating  bool
	}{
		{name: "internal gain only"},
		{name: "electric baseboard", extra: "ZoneHVAC:Baseboard:Convective:Electric,Baseboard;", wantHeating: true},
		{name: "gas boiler", extra: "Boiler:HotWater,Boiler,NaturalGas;", wantHeating: true},
		{name: "real district water plant", extra: "DistrictHeating:Water,Plant;", wantHeating: true, wantWaterHeating: true, wantLegacyHeating: true},
		{name: "real district steam plant", extra: "DistrictHeating:Steam,Plant;", wantHeating: true, wantSteamHeating: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := Parse("Version,25.1;HotWaterEquipment,Internal Gain;" + tc.extra)
			if err != nil {
				t.Fatal(err)
			}
			if features := detectOutputFeatures(doc); features.hasHeating != tc.wantHeating {
				t.Fatalf("independent heating feature changed: %+v", features)
			}
			report := AnalyzeOutput(doc)
			for id, want := range map[string]bool{
				"standard-meter-district-heating-water-heating": tc.wantWaterHeating,
				"standard-meter-district-heating-heating":       tc.wantLegacyHeating,
				"standard-meter-district-heating-steam-heating": tc.wantSteamHeating,
			} {
				if got := findOutputRecommendation(report, id) != nil; got != want {
					t.Fatalf("%s present=%t, want %t: unrelated heating cannot change HWE end use", id, got, want)
				}
			}
			for id := range hotWaterOutputRecommendationKeys() {
				if findOutputRecommendation(report, id) == nil {
					t.Fatalf("missing independent internal-equipment resource %s", id)
				}
			}
		})
	}

	// A district plant or arbitrary matching field is not a typed internal gain.
	for _, text := range []string{"DistrictHeating:Water,Plant;", "OtherEquipment,Equipment,DistrictHeatingWater;", "Schedule:Constant,HotWaterEquipment;"} {
		doc, err := Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		report := AnalyzeOutput(doc)
		for _, id := range []string{"standard-meter-district-heating-water-interior-equipment", "standard-meter-district-heating-interior-equipment"} {
			if findOutputRecommendation(report, id) != nil {
				t.Fatalf("unsupported internal-equipment inference for %s from %s", id, text)
			}
		}
	}
}

func TestOutputHotWaterEquipmentMergePreservesOriginalOutputsAndLegacyAliases(t *testing.T) {
	for _, version := range []string{"9.6", "25.1"} {
		t.Run(version, func(t *testing.T) {
			doc, err := Parse("Version," + version + `;
HotWaterEquipment,Internal Gain;
Output:Meter,InteriorEquipment:DistrictHeatingWater,Monthly;
Output:Meter,InteriorEquipment:DistrictHeatingWater,Monthly;
Output:Meter,InteriorEquipment:DistrictHeatingWater,Hourly;
Output:Variable,*,Zone Hot Water Equipment District Heating Energy,Hourly;
`)
			if err != nil {
				t.Fatal(err)
			}
			before := doc.String()
			request := OutputApplyRequest{AddRecommendations: []string{
				"standard-meter-district-heating-water-interior-equipment",
				"standard-meter-district-heating-interior-equipment",
			}}
			updated, preview := ApplyOutput(doc, request)
			if !preview.CanApply || len(updated.Objects) != len(doc.Objects)+1 || len(preview.Changes) != 2 || preview.Changes[0].Action != "no_change" || preview.Changes[1].Action != "add_output" {
				t.Fatalf("native/legacy merge lost exact output reuse: %+v", preview)
			}
			if !reflect.DeepEqual(updated.Objects[:len(doc.Objects)], doc.Objects) || doc.String() != before {
				t.Fatal("original output indices/fields or duplicate requests changed")
			}
			last := updated.Objects[len(updated.Objects)-1]
			if last.Type != "Output:Meter" || outputFieldValue(outputFieldValues(last), "Key Name") != "InteriorEquipment:DistrictHeating" || outputFieldValue(outputFieldValues(last), "Reporting Frequency") != "Monthly" {
				t.Fatalf("incorrect legacy alias appended: %+v", last)
			}
			second, again := ApplyOutput(updated, request)
			if !again.CanApply || !reflect.DeepEqual(second.Objects, updated.Objects) {
				t.Fatalf("second merge is not idempotent: %+v", again)
			}
		})
	}
}

func TestOutputHotWaterEquipmentMultiStoryKeepsInternalAndHeatingMetersSeparate(t *testing.T) {
	path := filepath.Join("..", "simulation", "testdata", "energy_path_real_models", "models", "25.1", "MultiStory.idf")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(string(content))
	if err != nil {
		t.Fatal(err)
	}
	before := doc.String()
	features := detectOutputFeatures(doc)
	if !features.hasHotWaterEquipment || !features.hasHeating || !features.hasElectricity || features.hasDistrictHeating || features.hasDistrictHeatWater {
		t.Fatalf("native MultiStory resource evidence changed: %+v", features)
	}
	report := AnalyzeOutput(doc)
	for id := range hotWaterOutputRecommendationKeys() {
		if findOutputRecommendation(report, id) == nil {
			t.Fatalf("native model missing %s", id)
		}
	}
	if findOutputRecommendation(report, "standard-meter-electricity-heating") == nil || findOutputRecommendation(report, "standard-meter-district-heating-water-heating") != nil || findOutputRecommendation(report, "standard-meter-district-heating-heating") != nil {
		t.Fatal("native electric heating and district-water internal gains were conflated")
	}
	if doc.String() != before {
		t.Fatal("native original was mutated")
	}
}

func hotWaterOutputRecommendationKeys() map[string]string {
	return map[string]string{
		"standard-meter-district-heating-water-facility":           "DistrictHeatingWater:Facility",
		"standard-meter-district-heating-facility":                 "DistrictHeating:Facility",
		"standard-meter-district-heating-water-interior-equipment": "InteriorEquipment:DistrictHeatingWater",
		"standard-meter-district-heating-interior-equipment":       "InteriorEquipment:DistrictHeating",
	}
}
