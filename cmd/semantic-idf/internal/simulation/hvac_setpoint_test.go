package simulation

import (
	"math"
	"testing"
)

func TestHVACSetpointSummaryRequiresDefinedObservations(t *testing.T) {
	for _, test := range []struct {
		name    string
		values  []float64
		average float64
		count   int
	}{
		{"unset SQL roundoff", []float64{-999, -999.0000000000005, -998.9999999999999}, 0, 0},
		{"missing observations", []float64{math.NaN(), math.Inf(1), math.Inf(-1)}, 0, 0},
		{"no observations", nil, 0, 0},
		{"intermittent setpoint", []float64{21, -998.9999999999999, 25}, 23, 2},
		{"real zero and negative", []float64{0, -10}, -5, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			setpoint := SimulationSeries{Column: "Node:System Node Setpoint Temperature [C]", Average: -600}
			temperature := SimulationSeries{Column: "Node:System Node Temperature [C]"}
			for index, value := range test.values {
				setpoint.Points = append(setpoint.Points, SimulationPoint{X: index, Value: value})
				temperature.Points = append(temperature.Points, SimulationPoint{X: index, Value: 20 + float64(index)*2})
			}
			original := append([]SimulationPoint(nil), setpoint.Points...)
			summary := hvacNodeSummaryFromBucket(&hvacNodeSeriesBucket{nodeName: "Node", setpoint: &setpoint, temperature: &temperature})
			if summary.HasSetpoint != (test.count > 0) || summary.SetpointAverage != test.average || summary.TemperatureSetpointSamples != test.count {
				t.Fatalf("invalid setpoint summary: %+v", summary)
			}
			if test.name == "intermittent setpoint" && summary.TemperatureSetpointDelta != 1 {
				t.Fatalf("unset frame contaminated temperature deviation: %+v", summary)
			}
			if test.count == 0 && len(buildHVACLoopAlerts([]HVACNodeRunSummary{summary})) != 0 {
				t.Fatal("uncontrolled node generated a setpoint alert")
			}
			for index := range original {
				if math.Float64bits(original[index].Value) != math.Float64bits(setpoint.Points[index].Value) {
					t.Fatal("summary filtering changed raw observations")
				}
			}
		})
	}
}
