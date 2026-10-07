package idf

import (
	"encoding/json"
	"sync"
	"testing"
)

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

func TestDocumentIndexHVACAnalysisPreservesSharedLookups(t *testing.T) {
	doc, err := Parse(sampleIDF)
	if err != nil {
		t.Fatal(err)
	}
	// Last duplicate wins for type/name lookup; source order remains intact for
	// type/name lists even when spelling and surrounding whitespace differ.
	duplicate := doc.Objects[len(doc.Objects)-1]
	duplicate.Index = len(doc.Objects)
	duplicate.Type = " FAN:CONSTANTVOLUME "
	duplicate.Fields = append([]Field(nil), duplicate.Fields...)
	duplicate.Fields[0].Value = " SUPPLY FAN "
	duplicate.Fields[len(duplicate.Fields)-1].Value = "Changed Outlet"
	doc.Objects = append(doc.Objects, duplicate)
	index := NewDocumentIndex(doc)
	before, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(AnalyzeHVAC(doc))
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			got, err := json.Marshal(AnalyzeHVACFromIndex(index))
			if err != nil || string(got) != string(want) {
				t.Errorf("indexed HVAC report differs from direct analysis (error %v)", err)
			}
		}()
	}
	workers.Wait()
	after, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("HVAC analysis mutated the shared document index")
	}
}
