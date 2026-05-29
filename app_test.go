package main

import "testing"

const appSummaryIDF = `
Version,
  24.1;                    !- Version Identifier

Zone,
  Office;                  !- Name
`

func TestAnalyzeInputTextIncludesSummary(t *testing.T) {
	app := NewApp()
	result, err := app.AnalyzeInputText(appSummaryIDF)
	if err != nil {
		t.Fatalf("AnalyzeInputText() error = %v", err)
	}
	if result.Report == nil {
		t.Fatalf("AnalyzeInputText() report = nil")
	}
	if result.Report.Summary.MetricCount != 50 {
		t.Fatalf("summary metric count = %d, want 50", result.Report.Summary.MetricCount)
	}
	if len(result.Report.Summary.Categories) != 6 {
		t.Fatalf("summary category count = %d, want 6", len(result.Report.Summary.Categories))
	}
	if result.Report.Geometry.ZoneCount != 1 {
		t.Fatalf("geometry zone count = %d, want 1", result.Report.Geometry.ZoneCount)
	}
}
