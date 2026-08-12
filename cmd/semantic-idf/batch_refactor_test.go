package main

import (
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

func TestBatchMetricNumberAndUnitParsing(t *testing.T) {
	for _, test := range []struct {
		input string
		want  float64
	}{
		{input: "1.00e+2 W", want: 100},
		{input: "-.5 kW", want: -0.5},
		{input: "+12. W", want: 12},
	} {
		value, ok := parseBatchMetricNumber(test.input)
		if !ok || value != test.want {
			t.Fatalf("parseBatchMetricNumber(%q) = %v, %v; want %v, true", test.input, value, ok, test.want)
		}
	}
	if _, ok := parseBatchMetricNumber("not available"); ok {
		t.Fatal("non-numeric summary value parsed as a number")
	}
	if unit := batchMetricUnit(BatchMetricsMetric{}, "1.00e+2 kWh"); unit != "kWh" {
		t.Fatalf("summary unit = %q, want kWh", unit)
	}
	if unit := batchMetricUnit(BatchMetricsMetric{Unit: "W/m2"}, "1.00 ignored"); unit != "W/m2" {
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
