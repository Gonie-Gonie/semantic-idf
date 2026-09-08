package simulation

import (
	"reflect"
	"testing"
)

func TestThermalTopologyReportedEnergyDoesNotDependOnFrameIntervals(t *testing.T) {
	points := []SimulationPoint{
		{X: 1, Label: "01/01 01:00", Value: -3600000},
		{X: 2, Label: "01/01 04:00", Value: 7200000},
		{X: 3, Label: "", Value: 0},
	}
	original := append([]SimulationPoint(nil), points...)
	for _, frequency := range []string{"Hourly", "Daily", "Monthly", "RunPeriod", ""} {
		t.Run(frequency, func(t *testing.T) {
			values, labels := normalizeThermalTopologySeries(thermalTopologyRawSeries{unit: "J", reportingFrequency: frequency, points: points})
			if !reflect.DeepEqual(values, []float64{-1, 2, 0}) || !reflect.DeepEqual(labels, []string{"01/01 01:00", "01/01 04:00", "Frame 3"}) {
				t.Fatalf("reported energy or labels changed: values %v, labels %v", values, labels)
			}
		})
	}
	if !reflect.DeepEqual(points, original) {
		t.Fatal("normalization changed original observations")
	}
	// Unlike J, W still requires the actual three-hour interval, including its
	// first frame; skipping interval calculation for rates would change values.
	values, _ := normalizeThermalTopologySeries(thermalTopologyRawSeries{unit: "W", reportingFrequency: "Hourly", rate: true, points: []SimulationPoint{
		{X: 1, Label: "01/01 01:00", Value: -1000},
		{X: 2, Label: "01/01 04:00", Value: 2000},
	}})
	if !reflect.DeepEqual(values, []float64{-3, 6}) {
		t.Fatalf("rate integration changed: %v", values)
	}
}
