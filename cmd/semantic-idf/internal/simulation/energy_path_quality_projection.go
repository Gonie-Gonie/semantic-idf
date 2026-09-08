package simulation

import (
	"reflect"
	"strings"
)

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
	// Allocation coverage describes the whole Building's HVAC/auxiliary
	// accounting, not whichever allocation records happen to be repeated in a
	// Zone. Keep the other quality fields local to the selected Zone graph.
	buildingAllocation := map[string]*EnergyPathQuality{}
	if result.Scope.Kind == "building" {
		period := strings.ToLower(energyPathQualityGraphPeriod(result.Nodes, result.Reconciliation))
		if period == "" {
			period = "annual"
		}
		buildingAllocation[period] = result.Quality
		for _, period := range result.Periods {
			buildingAllocation[strings.ToLower(strings.TrimSpace(period.ID))] = period.Quality
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
		if result.Scope.Kind == "building" {
			period := strings.ToLower(energyPathQualityGraphPeriod(zone.Nodes, zone.Reconciliation))
			if period == "" {
				period = "annual"
			}
			copyEnergyPathBuildingAllocationQuality(quality, buildingAllocation[period])
		}
		if replaceEnergyPathQuality(&zone.Quality, quality) {
			changed = true
		}
		if replaceEnergyPathQuality(&zone.Summary.Quality, quality) {
			changed = true
		}
		for periodIndex := range zone.Periods {
			period := &zone.Periods[periodIndex]
			quality := BuildEnergyPathQuality(selected, period.ID)
			if result.Scope.Kind == "building" {
				copyEnergyPathBuildingAllocationQuality(quality, buildingAllocation[strings.ToLower(strings.TrimSpace(period.ID))])
			}
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

func copyEnergyPathBuildingAllocationQuality(target, building *EnergyPathQuality) {
	if building == nil {
		// A Zone period cannot borrow an unrelated Annual denominator.
		target.ZoneAllocatedPct, target.UnassignedPct, target.ZoneAllocationStatus = 0, 0, "unavailable"
		return
	}
	target.ZoneAllocatedPct = building.ZoneAllocatedPct
	target.UnassignedPct = building.UnassignedPct
	target.ZoneAllocationStatus = building.ZoneAllocationStatus
}

func replaceEnergyPathQuality(target **EnergyPathQuality, quality *EnergyPathQuality) bool {
	changed := !reflect.DeepEqual(*target, quality)
	*target = quality
	return changed
}
