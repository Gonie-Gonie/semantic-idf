package idf

import "testing"

func TestDocumentIndexProvidesTypeAndNameLookups(t *testing.T) {
	doc, err := Parse(`
Version,
  24.1;

Zone,
  Office;

Schedule:Constant,
  Always On,
  ,
  1;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	index := NewDocumentIndex(doc)
	if got := len(index.ObjectsOfType("Zone")); got != 1 {
		t.Fatalf("zone lookup count = %d, want 1", got)
	}
	if got := len(index.ObjectsNamed("office")); got != 1 {
		t.Fatalf("name lookup count = %d, want 1", got)
	}
	if _, ok := index.ObjectByTypeName("Schedule:Constant", "Always On"); !ok {
		t.Fatalf("type/name lookup did not find Schedule:Constant Always On")
	}
	if got := len(index.Schedules); got != 1 {
		t.Fatalf("schedule index count = %d, want 1", got)
	}
}

func TestDocumentIndexAnalyzerAdaptersPreserveBasicResults(t *testing.T) {
	doc, err := Parse(`
Version,
  24.1;

Zone,
  Office;

Output:Variable,
  *,
  Zone Mean Air Temperature,
  Hourly;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	index := NewDocumentIndex(doc)
	if got := AnalyzeProfileFromIndex(index).ZoneCount; got != AnalyzeProfile(doc).ZoneCount {
		t.Fatalf("profile adapter zone count = %d, want direct result", got)
	}
	if got := AnalyzeHVACFromIndex(index).LoopCount; got != AnalyzeHVAC(doc).LoopCount {
		t.Fatalf("hvac adapter loop count = %d, want direct result", got)
	}
	if got := len(AnalyzeOutputFromIndex(index).Existing); got != len(AnalyzeOutput(doc).Existing) {
		t.Fatalf("output adapter existing count = %d, want direct result", got)
	}
	if got := AnalyzeGeometryFromIndex(index).ZoneCount; got != AnalyzeGeometry(doc).ZoneCount {
		t.Fatalf("geometry adapter zone count = %d, want direct result", got)
	}
	if got := len(AnalyzeDiagnosticsFromIndex(index)); got != len(AnalyzeDiagnostics(doc)) {
		t.Fatalf("diagnostics adapter count = %d, want direct result", got)
	}
}
