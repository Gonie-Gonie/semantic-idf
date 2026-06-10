package idf

import "testing"

const outputFixtureIDF = `
Version, 24.1;

Zone,
  Office;

Lights,
  Office Lights,
  Office,
  ,
  LightingLevel,
  100;

ElectricEquipment,
  Office Equipment,
  Office,
  ,
  EquipmentLevel,
  100;

Output:Variable,
  *,
  Zone Air Temperature,
  Hourly;

Output:Meter,
  Electricity:Facility,
  Monthly;
`

func TestAnalyzeOutputListsExistingAndRecommendations(t *testing.T) {
	doc, err := Parse(outputFixtureIDF)
	if err != nil {
		t.Fatalf("parse output fixture: %v", err)
	}
	report := AnalyzeOutput(doc)
	if report.ObjectCount != 2 || report.VariableCount != 1 || report.MeterCount != 1 {
		t.Fatalf("counts = objects %d variables %d meters %d, want 2/1/1", report.ObjectCount, report.VariableCount, report.MeterCount)
	}
	if len(report.Existing) != 2 {
		t.Fatalf("existing count = %d, want 2", len(report.Existing))
	}
	if !outputSummaryHasPurpose(report.Existing, "Output:Meter", "Electricity:Facility", "basic_energy") {
		t.Fatalf("facility meter should be tagged for basic energy: %#v", report.Existing)
	}
	if !recommendationExists(report, "sqlite-simple-tabular") {
		t.Fatalf("expected SQLite recommendation in %#v", report.Recommendations)
	}
	if !recommendationExists(report, "zone-air-temperature") {
		t.Fatalf("expected zone air temperature recommendation")
	}
	for _, item := range report.Recommendations {
		if item.ID == "zone-air-temperature" && !item.Exists {
			t.Fatalf("zone-air-temperature should be marked existing")
		}
		if item.ID == "node-temperature" && !stringSliceContains(item.PurposeTags, "hvac_loop_check") {
			t.Fatalf("node-temperature should be tagged for HVAC loop checks: %#v", item)
		}
	}
}

func TestAnalyzeOutputPurposeTags(t *testing.T) {
	doc, err := Parse(`
Version, 24.1;

Output:SQLite,
  SimpleAndTabular,
  JtoKWH;

Output:Variable,
  *,
  Zone Mean Air Temperature,
  Hourly;

Output:Variable,
  Supply Outlet Node,
  System Node Mass Flow Rate,
  Hourly;

Output:Variable,
  *,
  Zone Thermal Comfort Fanger Model PMV,
  Hourly;

Output:Diagnostics,
  DisplayExtraWarnings;
`)
	if err != nil {
		t.Fatalf("parse output purpose fixture: %v", err)
	}
	report := AnalyzeOutput(doc)
	if !outputSummaryHasPurpose(report.Existing, "Output:SQLite", "", "basic_energy") ||
		!outputSummaryHasPurpose(report.Existing, "Output:SQLite", "", "hvac_loop_check") {
		t.Fatalf("SQLite should be tagged as a purpose-run base output: %#v", report.Existing)
	}
	if !outputSummaryHasPurpose(report.Existing, "Output:Variable", "Zone Mean Air Temperature", "zone_heat_flow") ||
		!outputSummaryHasPurpose(report.Existing, "Output:Variable", "Zone Mean Air Temperature", "comfort_check") {
		t.Fatalf("zone mean air temperature purpose tags missing: %#v", report.Existing)
	}
	if !outputSummaryHasPurpose(report.Existing, "Output:Variable", "System Node Mass Flow Rate", "hvac_loop_check") {
		t.Fatalf("system node purpose tag missing: %#v", report.Existing)
	}
	if !outputSummaryHasPurpose(report.Existing, "Output:Diagnostics", "", "integrity_check") {
		t.Fatalf("diagnostics purpose tag missing: %#v", report.Existing)
	}
}

func TestApplyOutputAddsUpdatesAndRemoves(t *testing.T) {
	doc, err := Parse(outputFixtureIDF)
	if err != nil {
		t.Fatalf("parse output fixture: %v", err)
	}
	updated, preview := ApplyOutput(doc, OutputApplyRequest{
		AddRecommendations: []string{"sqlite-simple-tabular"},
		Updates: []OutputFieldUpdate{{
			ObjectIndex: 4,
			FieldIndex:  2,
			Value:       "Daily",
		}},
		RemoveObjectIndexes: []int{5},
	})
	if !preview.CanApply {
		t.Fatalf("preview blocking warnings: %#v", preview.Warnings)
	}
	report := AnalyzeOutput(updated)
	if report.ObjectCount != 2 {
		t.Fatalf("object count = %d, want 2 after add and remove", report.ObjectCount)
	}
	foundSQLite := false
	foundDailyVariable := false
	for _, obj := range report.Existing {
		if obj.ObjectType == "Output:SQLite" {
			foundSQLite = true
		}
		if obj.ObjectType == "Output:Variable" && obj.ReportingFrequency == "Daily" {
			foundDailyVariable = true
		}
	}
	if !foundSQLite || !foundDailyVariable {
		t.Fatalf("updated output report missing expected changes: %#v", report.Existing)
	}
}

func TestApplyStandardOutputPresetReplacesNonStandardOutput(t *testing.T) {
	doc, err := Parse(outputFixtureIDF + `
Output:Variable,
  *,
  Zone Air Relative Humidity,
  Hourly;
`)
	if err != nil {
		t.Fatalf("parse output fixture: %v", err)
	}
	updated, preview := ApplyOutput(doc, OutputApplyRequest{
		Preset:     "standard",
		PresetMode: "replace",
	})
	if !preview.CanApply {
		t.Fatalf("preview blocking warnings: %#v", preview.Warnings)
	}
	report := AnalyzeOutput(updated)
	if !recommendationExists(report, "standard-meter-electricity-facility") {
		t.Fatalf("expected standard electricity recommendation in %#v", report.Recommendations)
	}
	foundStandardMeter := false
	foundHeatFlow := false
	foundOldHumidity := false
	for _, obj := range report.Existing {
		if obj.ObjectType == "Output:Meter" && obj.KeyValue == "Electricity:Facility" && obj.ReportingFrequency == "Monthly" {
			foundStandardMeter = true
		}
		if obj.ObjectType == "Output:Variable" && obj.VariableName == "Zone Air Heat Balance Surface Convection Rate" && obj.ReportingFrequency == "Hourly" {
			foundHeatFlow = true
		}
		if obj.ObjectType == "Output:Variable" && obj.VariableName == "Zone Air Relative Humidity" {
			foundOldHumidity = true
		}
	}
	if !foundStandardMeter {
		t.Fatalf("standard monthly facility meter was not present: %#v", report.Existing)
	}
	if !foundHeatFlow {
		t.Fatalf("standard hourly heat-flow output was not present: %#v", report.Existing)
	}
	if foundOldHumidity {
		t.Fatalf("replace preset should remove non-standard humidity output: %#v", report.Existing)
	}
}

func recommendationExists(report OutputReport, id string) bool {
	for _, item := range report.Recommendations {
		if item.ID == id {
			return true
		}
	}
	return false
}

func outputSummaryHasPurpose(items []OutputObjectSummary, objectType string, name string, purpose string) bool {
	for _, item := range items {
		if item.ObjectType != objectType {
			continue
		}
		if name != "" && item.VariableName != name && item.KeyValue != name {
			continue
		}
		return stringSliceContains(item.PurposeTags, purpose)
	}
	return false
}

func stringSliceContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
