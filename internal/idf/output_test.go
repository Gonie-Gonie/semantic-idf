package idf

import "testing"

const outputFixtureIDF = `
Version, 24.1;

Zone,
  Office;

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
			ObjectIndex: 2,
			FieldIndex:  2,
			Value:       "Daily",
		}},
		RemoveObjectIndexes: []int{3},
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

func recommendationExists(report OutputReport, id string) bool {
	for _, item := range report.Recommendations {
		if item.ID == id {
			return true
		}
	}
	return false
}
