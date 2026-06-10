package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/idf-analyzer/internal/idf"
)

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
	if result.Report.Summary.MetricCount != 57 {
		t.Fatalf("summary metric count = %d, want 57", result.Report.Summary.MetricCount)
	}
	if len(result.Report.Summary.Categories) != 6 {
		t.Fatalf("summary category count = %d, want 6", len(result.Report.Summary.Categories))
	}
	if result.Report.Geometry.ZoneCount != 1 {
		t.Fatalf("geometry zone count = %d, want 1", result.Report.Geometry.ZoneCount)
	}
}

func TestDefaultEnergyPlusSampleAnalyzes(t *testing.T) {
	content, err := os.ReadFile("frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf")
	if err != nil {
		t.Fatalf("ReadFile(default sample) error = %v", err)
	}
	result, err := NewApp().AnalyzeInputText(string(content))
	if err != nil {
		t.Fatalf("AnalyzeInputText(default sample) error = %v", err)
	}
	if result.Report.ObjectCount < 100 {
		t.Fatalf("default sample object count = %d, want a complex example", result.Report.ObjectCount)
	}
	if result.Report.Geometry.ZoneCount < 10 || result.Report.Geometry.SurfaceCount < 50 || result.Report.Geometry.WindowCount < 10 {
		t.Fatalf("default sample geometry too small: zones=%d surfaces=%d windows=%d", result.Report.Geometry.ZoneCount, result.Report.Geometry.SurfaceCount, result.Report.Geometry.WindowCount)
	}
}

func TestReplaceOutputManagementObjectsPreservesNonOutputText(t *testing.T) {
	original := `
Version,
  24.2;

Building,
  Test Building,
  0,
  Suburbs,
  ,
  ,
  MinimalShadowing;

GlobalGeometryRules,
  UpperLeftCorner,
  CounterClockWise,
  World;

Zone,
  Office;

Output:Variable,
  *,
  Zone Mean Air Temperature,
  Hourly;
`
	doc, err := idf.Parse(original)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	updated, preview := idf.ApplyOutput(doc, idf.OutputApplyRequest{
		AddRecommendations:  []string{"sqlite-simple-tabular"},
		RemoveObjectIndexes: []int{4},
	})
	if !preview.CanApply {
		t.Fatalf("preview blocked: %#v", preview.Warnings)
	}
	patched := replaceOutputManagementObjects(original, updated)
	if !strings.Contains(patched, "Building,\n  Test Building") {
		t.Fatalf("patched text lost Building object:\n%s", patched)
	}
	if !strings.Contains(patched, "GlobalGeometryRules,") {
		t.Fatalf("patched text lost GlobalGeometryRules:\n%s", patched)
	}
	if strings.Contains(patched, "Zone Mean Air Temperature") {
		t.Fatalf("patched text kept removed output variable:\n%s", patched)
	}
	if !strings.Contains(patched, "Output:SQLite,") {
		t.Fatalf("patched text did not append Output:SQLite:\n%s", patched)
	}
}

func TestAppAssetHandlerServesSummaryMetricGuides(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/summary-metric-guides", nil)
	response := httptest.NewRecorder()

	appAssetHandler(NewApp()).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("summary metric guide API status = %d, want %d", response.Code, http.StatusOK)
	}
	var guides []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&guides); err != nil {
		t.Fatalf("summary metric guide API did not return JSON: %v", err)
	}
	if len(guides) != 57 {
		t.Fatalf("summary metric guide API returned %d guides, want 57", len(guides))
	}
}

func TestAnalyzeMultiSummaryPaths(t *testing.T) {
	tempDir := t.TempDir()
	first := filepath.Join(tempDir, "first.idf")
	second := filepath.Join(tempDir, "second.idf")
	if err := os.WriteFile(first, []byte(`Version, 24.1;
Building, Alpha;
Zone, Office;
`), 0o644); err != nil {
		t.Fatalf("write first fixture: %v", err)
	}
	if err := os.WriteFile(second, []byte(`Version, 24.1;
Building, Beta;
Zone, Core;
Zone, Perimeter;
`), 0o644); err != nil {
		t.Fatalf("write second fixture: %v", err)
	}

	var progress []MultiSummaryProgress
	result := analyzeMultiSummaryPaths([]string{first, second}, "test-run", func(item MultiSummaryProgress) {
		progress = append(progress, item)
	})

	if result.Total != 2 || result.Completed != 2 || result.Succeeded != 2 || result.Failed != 0 {
		t.Fatalf("multi summary counts = total:%d completed:%d succeeded:%d failed:%d", result.Total, result.Completed, result.Succeeded, result.Failed)
	}
	if len(progress) != 2 {
		t.Fatalf("progress events = %d, want 2", len(progress))
	}
	if len(result.Metrics) != 57 {
		t.Fatalf("multi summary metrics = %d, want 57", len(result.Metrics))
	}
	if result.Files[0].Label != "Alpha" || result.Files[1].Label != "Beta" {
		t.Fatalf("multi summary labels = %q, %q; want Alpha, Beta", result.Files[0].Label, result.Files[1].Label)
	}
	if got := result.Files[1].MetricValues["zone_count"].DisplayValue; got != "2" {
		t.Fatalf("second zone_count = %q, want 2", got)
	}
	if result.Metrics[0].CSVName != "energyplus_version [-]" {
		t.Fatalf("first CSV metric name = %q, want energyplus_version [-]", result.Metrics[0].CSVName)
	}
}
