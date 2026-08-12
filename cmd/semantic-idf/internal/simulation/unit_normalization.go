package simulation

import (
	"math"
	"strings"
)

type simulationDisplayUnit struct {
	Unit   string
	Factor float64
}

func normalizeSimulationSeriesDisplay(series SimulationSeries) SimulationSeries {
	unit := unitFromSeriesColumn(series.Column)
	normalized := normalizeSimulationDisplayUnit(unit)
	if normalized.Unit == "" {
		return series
	}
	series.DisplayUnit = normalized.Unit
	series.DisplayColumn = replaceSeriesColumnUnit(series.Column, normalized.Unit)
	series.DisplayMin = roundSimulationDisplayNumber(series.Min * normalized.Factor)
	series.DisplayMax = roundSimulationDisplayNumber(series.Max * normalized.Factor)
	series.DisplayAverage = roundSimulationDisplayNumber(series.Average * normalized.Factor)
	if normalized.Factor != 1 {
		series.DisplayPoints = make([]SimulationPoint, 0, len(series.Points))
		for _, point := range series.Points {
			point.Value = roundSimulationDisplayNumber(point.Value * normalized.Factor)
			series.DisplayPoints = append(series.DisplayPoints, point)
		}
	}
	return series
}

func normalizeSimulationDisplayUnit(unit string) simulationDisplayUnit {
	trimmed := strings.TrimSpace(unit)
	if trimmed == "" {
		return simulationDisplayUnit{}
	}
	switch normalizeUnitToken(trimmed) {
	case "j":
		return simulationDisplayUnit{Unit: "kWh", Factor: 1.0 / 3600000}
	case "kj":
		return simulationDisplayUnit{Unit: "kWh", Factor: 1.0 / 3600}
	case "mj":
		return simulationDisplayUnit{Unit: "kWh", Factor: 1.0 / 3.6}
	case "gj":
		return simulationDisplayUnit{Unit: "kWh", Factor: 277.7777777778}
	case "wh":
		return simulationDisplayUnit{Unit: "kWh", Factor: 1.0 / 1000}
	case "kwh":
		return simulationDisplayUnit{Unit: "kWh", Factor: 1}
	case "w":
		return simulationDisplayUnit{Unit: "kW", Factor: 1.0 / 1000}
	case "kw":
		return simulationDisplayUnit{Unit: "kW", Factor: 1}
	case "c", "degc", "degreec", "degreesc":
		return simulationDisplayUnit{Unit: "C", Factor: 1}
	case "kg/s", "kgs", "kgpersec", "kgpers":
		return simulationDisplayUnit{Unit: "kg/s", Factor: 1}
	case "kgwater/kgdryair", "kgwaterperkgdryair", "kg/kg":
		return simulationDisplayUnit{Unit: "kg/kg", Factor: 1}
	case "%", "percent":
		return simulationDisplayUnit{Unit: "%", Factor: 1}
	default:
		return simulationDisplayUnit{Unit: trimmed, Factor: 1}
	}
}

func normalizeUnitToken(unit string) string {
	unit = strings.ToLower(strings.TrimSpace(unit))
	unit = strings.ReplaceAll(unit, " ", "")
	unit = strings.ReplaceAll(unit, "_", "")
	unit = strings.ReplaceAll(unit, "-", "")
	return unit
}

func replaceSeriesColumnUnit(column string, unit string) string {
	start := strings.LastIndex(column, "[")
	end := strings.LastIndex(column, "]")
	if start < 0 || end <= start {
		return column
	}
	return column[:start+1] + unit + column[end:]
}

func roundSimulationDisplayNumber(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return value
	}
	return math.Round(value*1000000) / 1000000
}
