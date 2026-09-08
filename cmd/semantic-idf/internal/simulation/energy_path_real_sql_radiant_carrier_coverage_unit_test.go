package simulation

import "testing"

func TestEnergyPathRealSQLRadiantCarrierLedgerCoverageRequiresOriginalTrace(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		frames, model, bundle := epathSQLRadiantCarrierFixture(t)
		checks := epathSQLRadiantCarrierChecks(t, frames, model)
		if corrupt {
			for i := range bundle.EnergyExplanation.Sources {
				if bundle.EnergyExplanation.Sources[i].ID == "sql-rdd-11" {
					bundle.EnergyExplanation.Sources[i].Name = "Unrelated:Electricity"
				}
			}
		}
		annual := []epathSQLModelCheck{}
		for _, check := range checks.Rows {
			if check.Item.Period == "annual" {
				annual = append(annual, check)
			}
		}
		context := epathSQLCoverageContext{scope: "building", period: "annual", rows: bundle.EnergyExplanation.Reconciliation}
		all := map[string][]epathSQLModelCheck{epathSQLCoverageContextKey("building", "", "annual"): annual}
		var coverage epathSQLModelCoverageReport
		epathSQLCoverageRecords(bundle, context, annual, all, &coverage)
		found := false
		for _, record := range coverage.Records {
			if record.Collection != "reconciliation" || record.ID != "reconcile.zone_hvac_allocation.cooling.electricity.annual" {
				continue
			}
			found = true
			for _, field := range []string{"expectedValue", "directValue", "allocatedValue", "unassignedValue"} {
				count := len(record.Selectors[field])
				if !corrupt && count != 1 || corrupt && count != 0 {
					t.Fatalf("corrupt=%v: coverage bound %d selectors to %s without the exact original trace", corrupt, count, field)
				}
			}
		}
		if !found {
			t.Fatal("carrier allocation record disappeared from required coverage")
		}
	}
}
