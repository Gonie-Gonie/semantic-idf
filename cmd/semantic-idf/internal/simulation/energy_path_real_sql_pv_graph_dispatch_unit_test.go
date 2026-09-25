package simulation

import "testing"

// Exercise the actual prepared coverage boundary. These compact hand frames
// test mandatory dispatch only, not native calendar or full-model acceptance.
func TestEnergyPathSQLPVGraphPreparedCoverageCannotOmitNativeGuard(t *testing.T) {
	proof, bundle := epathSQLPVGraphScalarHand(t)
	checks := epathSQLModelChecks{PVHVACRequired: true}
	prepared := epathSQLPVCogenerationValidatedSources{graph: &proof}
	hasGraphFailure := func(report epathSQLModelCoverageReport) bool {
		for _, failure := range report.Failures {
			if failure.Key == "pv_graph/native_roles" {
				return true
			}
		}
		return false
	}
	if hasGraphFailure(epathSQLModelCoveragePrepared(bundle, checks, epathSQLPVValidatedSources{}, prepared, nil)) {
		t.Fatal("valid literal native graph rejected at prepared coverage boundary")
	}
	if !hasGraphFailure(epathSQLModelCoveragePrepared(bundle, checks, epathSQLPVValidatedSources{}, epathSQLPVCogenerationValidatedSources{}, nil)) {
		t.Fatal("full model accepted a deleted boundary-local native graph proof")
	}
	bundle.EnergyExplanation.Links[0].ToValue = 0
	if !hasGraphFailure(epathSQLModelCoveragePrepared(bundle, checks, epathSQLPVValidatedSources{}, prepared, nil)) {
		t.Fatal("coverage omitted the native support endpoint guard")
	}
	if err := epathSQLCheckRequiredPVGraph(PurposeResultBundle{}, epathSQLModelChecks{}, epathSQLPVCogenerationValidatedSources{}); err != nil {
		t.Fatal("source-only diagnostics acquired full graph applicability", err)
	}
}
