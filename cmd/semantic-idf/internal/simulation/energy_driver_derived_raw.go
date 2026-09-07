package simulation

import (
	"math"
	"strings"
)

// Derived balance vectors already contain effective model contributions. Give
// a Zone-owned derived term a consistent native-Zone equivalent for inspection
// without multiplying its effective energy again. It remains a derived formula,
// never a newly observed single-Zone SQL measurement. Building-only or unresolved
// terms have no proven common Zone multiplier and retain their model-total basis.
func retainEnergyDriverDerivedRawZoneEquivalent(series []energyExplanationSeries, sources []EnergyDataSource, context energyDriverBuildContext) ([]energyExplanationSeries, []EnergyDataSource) {
	if !context.Enabled || !context.Multipliers.Enabled {
		return series, sources
	}
	sourcePositions := map[string][]int{}
	for index, source := range sources {
		sourcePositions[source.ID] = append(sourcePositions[source.ID], index)
	}
	outSeries := append([]energyExplanationSeries(nil), series...)
	outSources := append([]EnergyDataSource(nil), sources...)
	for index := range outSeries {
		item := &outSeries[index]
		if item.Stage != "driver" || !item.multiplierApplied || item.driverBuildingOnly || strings.TrimSpace(item.ZoneName) == "" || len(item.SourceIDs) != 1 {
			continue
		}
		positions := sourcePositions[item.SourceIDs[0]]
		if len(positions) != 1 {
			continue
		}
		source := &outSources[positions[0]]
		if source.SourceType != "derived_formula" || source.Formula == "" || len(source.InputSourceIDs) == 0 || !strings.EqualFold(strings.TrimSpace(source.ZoneName), strings.TrimSpace(item.ZoneName)) {
			continue
		}
		record, ok := context.Multipliers.resolve(item.ZoneName)
		factor := record.effectiveMultiplier()
		if !ok || factor <= 0 || factor == 1 || math.IsNaN(factor) || math.IsInf(factor, 0) {
			continue
		}
		// Do not reuse rounded raw metadata as input on a repeated preparation.
		// The effective signed vectors remain the authoritative calculation.
		item.RawTotal = roundedEnergyNumber(item.Total / factor)
		item.RawMonthly = scaledEnergyExplanationPeriodValues(item.Monthly, 1/factor)
		item.RawDaily = scaledEnergyExplanationPeriodValues(item.Daily, 1/factor)
		item.RawHourly = scaledEnergyExplanationPeriodValues(item.Hourly, 1/factor)
		item.RawSelectedRange = roundedEnergyNumber(item.SelectedRange / factor)
		item.EffectiveMultiplier = factor
		item.MultiplierApplication = energyMultiplierRequiresZone
		source.RawValue = roundedEnergyNumber(source.EffectiveValue / factor)
		source.EffectiveMultiplier = factor
		source.MultiplierApplication = energyMultiplierRequiresZone
		const note = "Raw value is the calculated native-Zone equivalent (effective formula result / Zone and ZoneGroup multiplier), not a separate SQL measurement."
		if !strings.Contains(source.Explanation, note) {
			source.Explanation = strings.TrimSpace(source.Explanation + " " + note)
		}
	}
	return outSeries, outSources
}
