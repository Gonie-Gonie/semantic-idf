package simulation

import "sort"

// Accounting originates in several keyed accumulators. A canonical response
// has stable order regardless of map iteration, for GUI/API/CLI equality.
func orderEnergyPathAccounting(result *EnergyExplanationResult) {
	order := func(rows []EnergyReconciliation) []EnergyReconciliation {
		out := append([]EnergyReconciliation(nil), rows...)
		sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out
	}
	warnings := func(rows []EnergyWarning) []EnergyWarning {
		out := append([]EnergyWarning(nil), rows...)
		sort.SliceStable(out, func(i, j int) bool {
			a, b := out[i], out[j]
			if a.Period != b.Period {
				return a.Period < b.Period
			}
			if a.Code != b.Code {
				return a.Code < b.Code
			}
			if a.Severity != b.Severity {
				return a.Severity < b.Severity
			}
			return a.Message < b.Message
		})
		return out
	}
	periods := func(rows []EnergyPeriod) {
		for i := range rows {
			rows[i].Reconciliation = order(rows[i].Reconciliation)
			rows[i].Warnings = warnings(rows[i].Warnings)
		}
	}
	result.Reconciliation = order(result.Reconciliation)
	result.Warnings = warnings(result.Warnings)
	periods(result.Periods)
	for i := range result.ZoneResults {
		zone := &result.ZoneResults[i]
		zone.Reconciliation = order(zone.Reconciliation)
		zone.Warnings = warnings(zone.Warnings)
		periods(zone.Periods)
	}
}
