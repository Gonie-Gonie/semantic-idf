package main

import (
	"sync/atomic"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/internal/simulation"
)

func TestRunBatchAnalysisHonorsWorkerLimitAndPreservesOrder(t *testing.T) {
	paths := []string{"zero", "one", "two", "three", "four", "five"}
	started := make(chan struct{}, len(paths))
	release := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32
	done := make(chan []string, 1)

	go func() {
		done <- runBatchAnalysis(paths, 3, func(_ int, path string) string {
			current := active.Add(1)
			for observed := maximum.Load(); current > observed && !maximum.CompareAndSwap(observed, current); observed = maximum.Load() {
			}
			started <- struct{}{}
			<-release
			active.Add(-1)
			return path
		})
	}()

	for range 3 {
		<-started
	}
	select {
	case <-started:
		t.Fatal("more analyses started than the requested worker count")
	default:
	}
	close(release)
	results := <-done
	if maximum.Load() != 3 {
		t.Fatalf("maximum concurrent analyses = %d, want 3", maximum.Load())
	}
	for index, result := range results {
		if result != paths[index] {
			t.Fatalf("result %d = %q, want %q", index, result, paths[index])
		}
	}
}

func TestBatchAnalysisWorkerCountBounds(t *testing.T) {
	for _, test := range []struct {
		name      string
		requested int
		total     int
		want      int
	}{
		{name: "empty", requested: 4, total: 0, want: 0},
		{name: "single", requested: 8, total: 1, want: 1},
		{name: "requested", requested: 3, total: 10, want: 3},
		{name: "bounded", requested: 20, total: 20, want: 8},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := batchAnalysisWorkerCount(test.requested, test.total); got != test.want {
				t.Fatalf("batchAnalysisWorkerCount(%d, %d) = %d, want %d", test.requested, test.total, got, test.want)
			}
		})
	}
}

func TestBatchSummaryNumberAndUnitParsing(t *testing.T) {
	for _, test := range []struct {
		input string
		want  float64
	}{
		{input: "1.00e+2 W", want: 100},
		{input: "-.5 kW", want: -0.5},
		{input: "+12. W", want: 12},
	} {
		value, ok := parseBatchSummaryNumber(test.input)
		if !ok || value != test.want {
			t.Fatalf("parseBatchSummaryNumber(%q) = %v, %v; want %v, true", test.input, value, ok, test.want)
		}
	}
	if _, ok := parseBatchSummaryNumber("not available"); ok {
		t.Fatal("non-numeric summary value parsed as a number")
	}
	if unit := batchSummaryUnit(MultiSummaryMetric{}, "1.00e+2 kWh"); unit != "kWh" {
		t.Fatalf("summary unit = %q, want kWh", unit)
	}
	if unit := batchSummaryUnit(MultiSummaryMetric{Unit: "W/m2"}, "1.00 ignored"); unit != "W/m2" {
		t.Fatalf("explicit summary unit = %q, want W/m2", unit)
	}
}

func TestBatchSimulationSourceIndexBuildsAllMetadataInSourceOrder(t *testing.T) {
	firstIndex, secondIndex := 4, 5
	explanation := simulation.EnergyExplanationResult{Sources: []simulation.EnergyDataSource{
		{ID: "first", ObjectIndex: &firstIndex, TableName: "Table A", RowName: "Row A", ColumnName: "Column A", SourceUnit: "J", NormalizedUnit: "kWh"},
		{ID: "second", ObjectIndex: &secondIndex, TableName: "Table A", RowName: "Row B", ColumnName: "Column B", Units: "J", NormalizedUnit: "kWh"},
	}}
	fields := newBatchSimulationSourceIndex(explanation).fields([]string{"first", "missing", "second", "first"})
	want := [...]string{"4; 5", "Table A", "Row A; Row B", "Column A; Column B", "J", "kWh"}
	if fields != want {
		t.Fatalf("source metadata fields = %#v, want %#v", fields, want)
	}
}
