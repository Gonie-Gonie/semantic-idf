package simulation

import "github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"

// This restricts allocation only. The unaltered service-path index continues
// to expose true surviving paths and source lineage. No broad paid remainder
// may be renormalized over an incomplete Furnace recipient cohort.
func applyEnergyPathHeatOnlyCohortGuards(index *energyServicePathIndex, doc idf.Document, report idf.HVACReport) {
	for _, cohort := range idf.ResolveNativeHeatOnlyFurnaceCohorts(doc, report.ServiceModel.ZoneServices) {
		if cohort.Complete {
			continue
		}
		index.incompleteHeatOnly = true
		if index.heatOnlyBlockedFanLoops == nil {
			index.heatOnlyBlockedFanLoops = map[string]bool{}
		}
		key := "" // An unknown owner cannot authorize any reported fan pool.
		if cohort.AirLoop != nil {
			key = normalizePurposeToken(cohort.AirLoop.Name)
		}
		index.heatOnlyBlockedFanLoops[key] = true
		if index.auxiliaryResolvable == nil {
			index.auxiliaryResolvable = map[string]bool{}
		}
		index.auxiliaryResolvable["fans"] = false
	}
}
