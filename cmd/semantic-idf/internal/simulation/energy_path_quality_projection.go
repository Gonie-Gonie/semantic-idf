package simulation

import "reflect"

// refreshEnergyPathQuality runs after all monthly/annual allocation accounting
// and stored-graph repairs. A persisted score is never authoritative over the
// graph and source-availability evidence that the user is actually viewing.
func refreshEnergyPathQuality(result *EnergyExplanationResult) bool {
	if result == nil {
		return false
	}
	changed := replaceEnergyPathQuality(&result.Quality, BuildEnergyPathQuality(*result, ""))
	for index := range result.Periods {
		period := &result.Periods[index]
		quality := BuildEnergyPathQuality(*result, period.ID)
		if replaceEnergyPathQuality(&period.Quality, quality) {
			changed = true
		}
		if period.Summary != nil && replaceEnergyPathQuality(&period.Summary.Quality, quality) {
			changed = true
		}
	}
	for index := range result.ZoneResults {
		zone := &result.ZoneResults[index]
		selected := EnergyExplanationResult{
			Scope: zone.Scope, Nodes: zone.Nodes, Links: zone.Links,
			Periods: zone.Periods, Sources: result.Sources,
			Completeness: zone.Completeness, Reconciliation: zone.Reconciliation,
		}
		quality := BuildEnergyPathQuality(selected, "")
		if replaceEnergyPathQuality(&zone.Quality, quality) {
			changed = true
		}
		if replaceEnergyPathQuality(&zone.Summary.Quality, quality) {
			changed = true
		}
		for periodIndex := range zone.Periods {
			period := &zone.Periods[periodIndex]
			quality := BuildEnergyPathQuality(selected, period.ID)
			if replaceEnergyPathQuality(&period.Quality, quality) {
				changed = true
			}
			if period.Summary != nil && replaceEnergyPathQuality(&period.Summary.Quality, quality) {
				changed = true
			}
		}
	}
	return changed
}

func replaceEnergyPathQuality(target **EnergyPathQuality, quality *EnergyPathQuality) bool {
	changed := !reflect.DeepEqual(*target, quality)
	*target = quality
	return changed
}
