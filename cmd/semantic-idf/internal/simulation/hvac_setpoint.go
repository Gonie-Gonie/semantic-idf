package simulation

import "math"

// EnergyPlus uses -999 for an unset node temperature setpoint. Hourly averaging
// can put it a few floating-point steps above -999; retain the same tolerance
// as frontend/src/js/hvac-setpoint.js. Zero/negative real setpoints remain valid.
func hvacHasTemperatureSetpoint(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > -999+1e-6
}

func hvacDefinedSetpointAverage(series SimulationSeries) (float64, int) {
	total, count := 0.0, 0
	for index, point := range series.Points {
		if !hvacHasTemperatureSetpoint(point.Value) {
			continue
		}
		value := point.Value
		if len(series.DisplayPoints) == len(series.Points) {
			value = series.DisplayPoints[index].Value
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		total += value
		count++
	}
	if count == 0 {
		return 0, 0
	}
	return total / float64(count), count
}
