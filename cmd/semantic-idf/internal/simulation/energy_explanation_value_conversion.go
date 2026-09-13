package simulation

import (
	"math"
	"strings"
)

// Prepared once per selected dictionary, rather than normalizing the same name
// and unit for every hourly observation. Arithmetic and rounding order match
// the uncached conversion, including rate fallback and unknown units.
type energyExplanationValueConversion struct {
	integratesRate bool
	rateUnit       string
	unit           string
	factor         float64
}

func prepareEnergyExplanationValueConversion(dictionary energyExplanationDictionary) energyExplanationValueConversion {
	dictionary.valueConversion = nil
	normalized := normalizeSimulationDisplayUnit(dictionary.row.units)
	conversion := energyExplanationValueConversion{
		integratesRate: energyExplanationIntegratesRate(dictionary),
		rateUnit:       normalizeUnitToken(dictionary.row.units), unit: normalized.Unit, factor: normalized.Factor,
	}
	if conversion.unit == "" {
		conversion.unit, conversion.factor = strings.TrimSpace(dictionary.row.units), 1
	}
	return conversion
}

func (conversion energyExplanationValueConversion) convert(value, intervalHours float64) (float64, string) {
	if conversion.integratesRate {
		hours := intervalHours
		if hours <= 0 || math.IsNaN(hours) || math.IsInf(hours, 0) {
			hours = 1
		}
		switch conversion.rateUnit {
		case "w":
			return roundedEnergyNumber(value * hours / 1000), "kWh"
		case "kw":
			return roundedEnergyNumber(value * hours), "kWh"
		}
	}
	return roundedEnergyNumber(value * conversion.factor), conversion.unit
}
