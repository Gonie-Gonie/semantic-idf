package simulation

import (
	"reflect"
	"testing"
)

// This IO fixture adds the independently authored native source checks, not
// candidate fields. The pending writer must retain their exact known/unknown
// values while requiring the same typed selector contract as field coverage.
func epathOraclePendingVRFUnitFixture(t *testing.T) epathOraclePendingUnitInput {
	t.Helper()
	input := epathOraclePendingUnitFixture(t)
	_, checks, _ := epathVRFSourceUnitFixture(t)
	for _, check := range checks.Rows {
		if input.checks.Keys[check.Want.Key] {
			t.Fatal("native source fixture duplicates pending registry")
		}
		input.checks.Keys[check.Want.Key] = true
		input.checks.Rows = append(input.checks.Rows, check)
		input.observed.Metrics = append(input.observed.Metrics, check.Want)
	}
	return input
}

func TestEnergyPathRealOraclePendingVRFRetainsTypedSourceObligations(t *testing.T) {
	input := epathOraclePendingVRFUnitFixture(t)
	if err := input.write(); err != nil {
		t.Fatal(err)
	}
	var pending epathOraclePendingMetrics
	if err := epathDecodeOracleFile(input.destination, &pending); err != nil {
		t.Fatal(err)
	}
	wants := map[string]epathRealOracleMetric{}
	allocated, unknown := 0, 0
	for _, check := range input.checks.Rows {
		wants[check.Want.Key] = check.Want
		if check.NativeVRFSource == nil {
			continue
		}
		if check.Item.Target.Field == "allocatedValue" {
			allocated++
		}
		if check.Quantity == nil {
			unknown++
		}
	}
	if allocated != 44 || unknown != 40 {
		t.Fatalf("native source obligations changed: allocated=%d unknown=%d", allocated, unknown)
	}
	for _, metric := range pending.Metrics {
		// Existing quality fixtures derive their status after compilation;
		// every source scalar must instead remain exactly its compiled value.
		for _, check := range input.checks.Rows {
			if check.Want.Key == metric.Key && check.NativeVRFSource != nil && !reflect.DeepEqual(metric, wants[metric.Key]) {
				t.Fatalf("pending changed independently compiled native metric %s", metric.Key)
			}
		}
	}
	if len(pending.Metrics) != len(input.checks.Rows) || pending.Approved || pending.Acceptance {
		t.Fatal("pending changed selector roster or claimed approval")
	}
}

func TestEnergyPathRealOraclePendingVRFRejectsUnboundSourceSelectors(t *testing.T) {
	for _, kind := range []string{"missing typed proof", "wrong source name", "foreign owner", "monthly selector", "changed allocation", "invented measured zero"} {
		t.Run(kind, func(t *testing.T) {
			input := epathOraclePendingVRFUnitFixture(t)
			found := false
			for index := range input.checks.Rows {
				check := &input.checks.Rows[index]
				if check.NativeVRFSource == nil || !check.NativeVRFSource.Observation.Shared || check.Item.Scope != "zone" {
					continue
				}
				if kind == "invented measured zero" && check.Item.Target.Field != "rawValue" || kind != "invented measured zero" && check.Item.Target.Field != "allocatedValue" {
					continue
				}
				found = true
				switch kind {
				case "missing typed proof":
					check.NativeVRFSource = nil
				case "wrong source name":
					check.Item.Target.SourceName = "Zone Lights Electricity Energy"
				case "foreign owner":
					check.Item.Zone, check.Want.Zone = "PLENUM-1", "PLENUM-1"
				case "monthly selector":
					check.Item.Period, check.Want.Period = "M1", "M1"
				case "changed allocation":
					quantity := *check.Quantity
					quantity.Value += .001
					check.Quantity = &quantity
					check.Want.Value = epathOracleNumber(quantity.Value)
				case "invented measured zero":
					check.Quantity = &epathSQLQuantity{}
					check.Want.Value = epathOracleNumber(0)
				}
				// Keep the metric copy consistent, so target binding rather than
				// a later accidental copy mismatch must reject the altered claim.
				input.observed.Metrics[index] = check.Want
				break
			}
			if !found {
				t.Fatal("missing shared source fixture")
			}
			if err := input.write(); err == nil {
				t.Fatal("pending accepted an unbound native source selector")
			}
		})
	}
}
