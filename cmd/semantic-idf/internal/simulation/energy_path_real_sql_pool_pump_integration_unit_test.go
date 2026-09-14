package simulation

// Bounded integration test of mandatory Pool ledger proof dispatch. The hand
// bundle intentionally omits unrelated whole-model quality/source selectors;
// it proves this 52-row ledger integration, not full fixture acceptance.
import (
	"strings"
	"testing"
)

func epathSQLPoolPumpIntegrationChecks(t *testing.T, consumer *epathSQLPoolPumpConsumer) epathSQLModelChecks {
	t.Helper()
	checks := epathSQLModelChecks{}
	if err := epathSQLPoolPumpBuildingLedgerChecks(consumer, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 52 || len(checks.Keys) != 52 {
		t.Fatal("missing native pump ledger selector census")
	}
	for _, check := range checks.Rows {
		if !strings.Contains(check.Item.Key, "|pool_native_pump/") || check.Allocation == nil || check.Allocation.NativePoolPump == nil {
			t.Fatal("native pump ledger lost its required identity")
		}
	}
	return checks
}

func epathSQLPoolPumpIntegrationAnnualIndex(t *testing.T, checks epathSQLModelChecks) int {
	t.Helper()
	found := -1
	for i, check := range checks.Rows {
		if check.Item.Period == "annual" && check.Item.Target.Field == "expectedValue" {
			if found >= 0 {
				t.Fatal("duplicate annual budget gate")
			}
			found = i
		}
	}
	if found < 0 {
		t.Fatal("required annual expected-value joint gate missing")
	}
	return found
}

func epathSQLPoolPumpIntegrationHasFailure(failures []epathSQLModelFailure, key string) bool {
	for _, failure := range failures {
		if failure.Key == key {
			return true
		}
	}
	return false
}

func epathSQLPoolPumpIntegrationBound(report epathSQLModelCoverageReport, key, id, field string) bool {
	for _, record := range report.Records {
		if record.Scope != "building" || record.Collection != "reconciliation" || record.ID != id {
			continue
		}
		for _, selector := range record.Selectors[field] {
			if selector == key {
				return true
			}
		}
	}
	return false
}

func TestEnergyPathRealSQLPoolPumpIntegrationMandatoryFullLedger(t *testing.T) {
	bundle, consumer := epathSQLPoolJointFixture(t)
	checks := epathSQLPoolPumpIntegrationChecks(t, consumer)
	for _, check := range checks.Rows {
		if err := epathSQLCheckPoolPumpConsumer(bundle, check); err != nil {
			t.Fatalf("self-compiled mandatory Pool ledger rejected: %s: %v", check.Item.Key, err)
		}
	}
	out := epathRealOracleEvidence{}
	if failures := epathEvaluateSQLModelChecks(&out, bundle, checks); len(failures) != 0 {
		t.Fatalf("52 native ledger rows failed through top evaluator: %+v", failures)
	}
	coverage := epathSQLModelCoverage(bundle, checks)
	// Unrelated missing quality selectors remain failures, as they should;
	// require every ledger field's own positive proof and exact binding.
	for _, check := range checks.Rows {
		if epathSQLPoolPumpIntegrationHasFailure(coverage.Failures, check.Want.Key) {
			t.Fatalf("valid ledger custom proof failed coverage: %s", check.Want.Key)
		}
		if !epathSQLPoolPumpIntegrationBound(coverage, check.Want.Key, check.Item.Target.ID, check.Item.Target.Field) {
			t.Fatalf("validated Pool ledger lost field binding: %s", check.Want.Key)
		}
	}
}

func TestEnergyPathRealSQLPoolPumpIntegrationRejectsLostOrChangedProof(t *testing.T) {
	for _, mutation := range []string{"nil native", "nil allocation", "nil consumer", "wrong selector", "changed quantity", "changed expectation", "erased prefix", "optional presentation", "forged joint"} {
		t.Run(mutation, func(t *testing.T) {
			bundle, consumer := epathSQLPoolJointFixture(t)
			checks := epathSQLPoolPumpIntegrationChecks(t, consumer)
			index := epathSQLPoolPumpIntegrationAnnualIndex(t, checks)
			check := &checks.Rows[index]
			switch mutation {
			case "nil native":
				copy := *check.Allocation
				copy.NativePoolPump = nil
				check.Allocation = &copy
			case "nil allocation":
				check.Allocation = nil
			case "nil consumer":
				allocation := *check.Allocation
				proof := *allocation.NativePoolPump
				proof.Consumer = nil
				allocation.NativePoolPump = &proof
				check.Allocation = &allocation
			case "wrong selector":
				check.Item.Target.ID += ".other"
			case "changed quantity":
				copy := *check.Quantity
				copy.Value += .01
				check.Quantity = &copy
			case "changed expectation":
				copy := *check.Want.Value
				copy += .01
				check.Want.Value = &copy
			case "erased prefix":
				old := check.Item.Key
				key := strings.Replace(old, "pool_native_pump/", "ordinary_pump/", 1)
				delete(checks.Keys, old)
				checks.Keys[key] = true
				check.Item.Key, check.Want.Key = key, key
			case "optional presentation":
				check.OptionalPresentation = true
			case "forged joint":
				zone := &bundle.EnergyExplanation.ZoneResults[0]
				zone.Periods[1].Nodes[0].Value += .001
				zone.Periods[1].Nodes[0].AllocatedValue += .001
				calculation, err := epathSQLPoolPumpMathFromSources(consumer.Native, consumer.Frames, consumer.Model)
				if err != nil {
					t.Fatal(err)
				}
				q := calculation.Months[0].Shares[strings.ToLower(zone.Scope.ZoneName)]
				if err := epathCheckSQLModelQuantity(&zone.Periods[1].Nodes[0].Value, &q); err != nil {
					t.Fatalf("joint mutation must remain inside the individual allocation envelope: %v", err)
				}
				for _, row := range checks.Rows {
					if err := epathCheckSQLPoolPumpAllocation(bundle, row); err != nil {
						t.Fatalf("joint mutation must retain every scalar Building ledger: %v", err)
					}
				}
			}
			if err := epathSQLCheckPoolPumpConsumer(bundle, *check); err == nil {
				t.Fatal("mandatory supplemental Pool consumer accepted mutation")
			}
			out := epathRealOracleEvidence{}
			failures := epathEvaluateSQLModelChecks(&out, bundle, checks)
			if !epathSQLPoolPumpIntegrationHasFailure(failures, check.Want.Key) {
				t.Fatalf("top evaluate lost required Pool proof failure: %+v", failures)
			}
			coverage := epathSQLModelCoverage(bundle, checks)
			if !epathSQLPoolPumpIntegrationHasFailure(coverage.Failures, check.Want.Key) {
				t.Fatalf("top coverage lost required Pool proof failure: %+v", coverage.Failures)
			}
			if epathSQLPoolPumpIntegrationBound(coverage, check.Want.Key, check.Item.Target.ID, check.Item.Target.Field) {
				t.Fatal("failed Pool custom proof still authorized a primary field binding")
			}
		})
	}
}

func TestEnergyPathRealSQLPoolPumpIntegrationSubquantumLedgerUsesNonnegativePresentation(t *testing.T) {
	frames, model, aux := epathSQLPoolPumpConsumerHandFrames(t)
	energy := [12]float64{}
	for month := range energy {
		energy[month] = .0004
	}
	epathSQLPoolJointNativePumpBudgets(t, &frames, energy, [12]float64{})
	consumer, err := epathSQLPoolPumpConsumerFor(frames, model, aux)
	if err != nil {
		t.Fatal(err)
	}
	bundle := epathSQLPoolJointHandBundle(t, consumer, map[string][12]float64{})
	checks := epathSQLPoolPumpIntegrationChecks(t, consumer)
	// Each month rounds to zero. The native annual CW source remains a
	// positive .005 persisted observation, not .005 of Zone allocation.
	for _, check := range checks.Rows {
		if err := epathCheckSQLPoolPumpAllocation(bundle, check); err != nil {
			t.Fatalf("compiled subquantum ledger was compared to unclamped negative presentation bounds: %s: %v", check.Want.Key, err)
		}
		if err := epathSQLCheckPoolPumpConsumer(bundle, check); err != nil {
			t.Fatalf("canonical supplemental consumer disagrees with bindAllocationProof: %s: %v", check.Want.Key, err)
		}
	}
	out := epathRealOracleEvidence{}
	if failures := epathEvaluateSQLModelChecks(&out, bundle, checks); len(failures) != 0 {
		t.Fatalf("known subquantum monthly zero cannot pass mandatory compiled ledger: %+v", failures)
	}
}
