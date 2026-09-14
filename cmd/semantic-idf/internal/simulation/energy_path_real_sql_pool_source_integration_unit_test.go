package simulation

// Independent acceptance support. Tests the root-owned dispatcher/decoder integration using the
// original topology and hand native SQL rows. No candidate/artifact values.
import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func epathSQLPoolSourceIntegrationHand(t *testing.T) (epathSQLModelChecks, PurposeResultBundle) {
	t.Helper()
	frame := epathSQLPoolConsumerHandFrame(t)
	checks := epathSQLModelChecks{}
	if err := epathSQLModelPoolSourceChecks(epathSQLFrames{PoolSystems: []epathSQLPoolSourceFrames{frame}}, &checks); err != nil {
		t.Fatal(err)
	}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}}
	seen := map[int]bool{}
	for _, check := range checks.Rows {
		if check.PoolSource == nil {
			t.Fatal("native source compiler emitted a row without required proof")
		}
		proof := check.PoolSource
		id := proof.Identity.Source.DictionaryIndex
		if seen[id] {
			continue
		}
		seen[id] = true
		one := epathSQLPoolChartHandBundle(t, *proof)
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, epathSQLPoolConsumerDecodeSource(t, one.EnergyExplanation.Sources[0], true))
		if len(one.EnergyExplanation.HourlyLabels) != 0 {
			if len(bundle.EnergyExplanation.HourlyLabels) != 0 && !reflect.DeepEqual(bundle.EnergyExplanation.HourlyLabels, one.EnergyExplanation.HourlyLabels) {
				t.Fatal("independent native source proofs disagree on shared Hourly axis")
			}
			bundle.EnergyExplanation.HourlyLabels = append([]string(nil), one.EnergyExplanation.HourlyLabels...)
		}
	}
	return checks, bundle
}

func TestEnergyPathRealSQLPoolSourceIntegrationCompilerCensusAndKnownZero(t *testing.T) {
	checks, bundle := epathSQLPoolSourceIntegrationHand(t)
	if len(checks.Rows) != 56 || len(checks.Keys) != 56 || len(bundle.EnergyExplanation.Sources) != 28 {
		t.Fatalf("required native source census changed: checks=%d keys=%d sources=%d", len(checks.Rows), len(checks.Keys), len(bundle.EnergyExplanation.Sources))
	}
	groupCounts, fieldCounts, zeroCount := map[string]int{}, map[string]int{}, 0
	for _, check := range checks.Rows {
		groupCounts[check.Want.Group]++
		fieldCounts[check.Item.Target.Field]++
		if !checks.Keys[check.Want.Key] || check.Want.Key != check.Item.Key || check.Quantity == nil || check.Want.Value == nil || check.OptionalPresentation || check.Item.Target.AllowPrunedZero {
			t.Fatal("native required row became optional or lost a scalar/registry binding")
		}
		if check.PoolSource.Identity.Spec.ID == "boiler.ancillary_electricity" {
			zeroCount++
			lo, hi := check.Quantity.bounds()
			if check.Quantity.Value != 0 || lo != 0 || hi != 0 || *check.Want.Value != 0 {
				t.Fatal("independently observed zero gained unknown/rounding slack")
			}
		}
		if err := epathSQLCheckPoolSourceConsumer(bundle, check); err != nil {
			t.Fatalf("%s: %v", check.Want.Key, err)
		}
	}
	if groupCounts["loads"] != 16 || groupCounts["endUses"] != 40 || fieldCounts["rawValue"] != 28 || fieldCounts["effectiveValue"] != 28 || zeroCount != 8 {
		t.Fatalf("native E/R source-only role/zero coverage changed: groups=%v fields=%v zero=%d", groupCounts, fieldCounts, zeroCount)
	}
	var out epathRealOracleEvidence
	if failures := epathEvaluateSQLModelChecks(&out, bundle, checks); len(failures) != 0 {
		t.Fatalf("integrated dispatch rejected independently valid native source/check/chart: %v", failures)
	}
	if len(out.Metrics) != 56 {
		t.Fatal("integrated source dispatcher silently skipped required rows")
	}
	// No Pool declaration leaves legacy source compiler behavior unchanged.
	legacy := epathSQLModelChecks{}
	if err := epathSQLModelPoolSourceChecks(epathSQLFrames{}, &legacy); err != nil || len(legacy.Rows) != 0 || len(legacy.Keys) != 0 {
		t.Fatalf("Pool source integration changed an undeclared legacy model: %v", err)
	}
}

func TestEnergyPathRealSQLPoolSourceIntegrationRejectsCheckRebinding(t *testing.T) {
	checks, bundle := epathSQLPoolSourceIntegrationHand(t)
	var baseline epathSQLModelCheck
	for _, check := range checks.Rows {
		if check.PoolSource.Identity.Spec.ID == "boiler.ancillary_electricity" && check.PoolSource.Identity.Canonical && check.Item.Target.Field == "rawValue" {
			baseline = check
			break
		}
	}
	if baseline.PoolSource == nil {
		t.Fatal("missing independently observed zero baseline")
	}
	for _, mutation := range []string{"lost proof", "lost proof and Item key", "Item key", "Want key", "both keys", "target owner", "target field", "target frequency", "target unit", "target pruned zero", "quantity nil", "quantity value", "quantity bounds", "Want nil", "Want value", "Want status", "Item group", "Want group", "scope", "period", "optional presentation", "proof field", "proof identity"} {
		t.Run(mutation, func(t *testing.T) {
			bad := baseline
			proof := *baseline.PoolSource
			bad.PoolSource = &proof
			q := *baseline.Quantity
			bad.Quantity = &q
			if q.Bounds != nil {
				bounds := *q.Bounds
				q.Bounds = &bounds
			}
			v := *baseline.Want.Value
			bad.Want.Value = &v
			switch mutation {
			case "lost proof":
				bad.PoolSource = nil
			case "lost proof and Item key":
				bad.PoolSource = nil
				bad.Item.Key = "ordinary-source-row"
			case "Item key":
				bad.Item.Key += "/changed"
			case "Want key":
				bad.Want.Key += "/changed"
			case "both keys":
				bad.Item.Key += "/changed"
				bad.Want.Key = bad.Item.Key
			case "target owner":
				bad.Item.Target.SourceKey = "another boiler"
			case "target field":
				bad.Item.Target.Field = "effectiveValue"
			case "target frequency":
				bad.Item.Target.Frequency = "Hourly"
			case "target unit":
				bad.Item.Target.Unit = "J"
			case "target pruned zero":
				bad.Item.Target.AllowPrunedZero = true
			case "quantity nil":
				bad.Quantity = nil
			case "quantity value":
				q = epathSQLQuantity{Value: 1}
			case "quantity bounds":
				q = epathSQLBounded(0, 0, 1)
			case "Want nil":
				bad.Want.Value = nil
			case "Want value":
				v = 1
			case "Want status":
				bad.Want.Status = "unavailable"
			case "Item group":
				bad.Item.Group = "drivers"
			case "Want group":
				bad.Want.Group = "drivers"
			case "scope":
				bad.Item.Scope = "zone"
				bad.Want.Scope = "zone"
				bad.Item.Zone = "SPACE1-1"
				bad.Want.Zone = "SPACE1-1"
			case "period":
				bad.Item.Period = "M1"
				bad.Want.Period = "M1"
			case "optional presentation":
				bad.OptionalPresentation = true
			case "proof field":
				proof.Field = "effectiveValue"
			case "proof identity":
				proof.Identity.ExecutedOwnerIndex++
			}
			if err := epathSQLCheckPoolSourceConsumer(bundle, bad); err == nil {
				t.Fatalf("consumer accepted %s", mutation)
			}
			var out epathRealOracleEvidence
			if failures := epathEvaluateSQLModelChecks(&out, bundle, epathSQLModelChecks{Rows: []epathSQLModelCheck{bad}, Keys: map[string]bool{bad.Want.Key: true}}); len(failures) == 0 {
				t.Fatalf("main evaluator bypassed required native proof for %s", mutation)
			}
		})
	}
}

func TestEnergyPathRealSQLPoolSourceIntegrationChartAndPresenceDispatch(t *testing.T) {
	checks, baseline := epathSQLPoolSourceIntegrationHand(t)
	var selected epathSQLModelCheck
	for _, check := range checks.Rows {
		if check.PoolSource.Identity.Spec.ID == "boiler.ancillary_electricity" && check.Item.Target.Frequency == "Hourly" && check.PoolSource.Identity.Source.SourceUnit == "J" && check.Item.Target.Field == "rawValue" {
			selected = check
			break
		}
	}
	if selected.PoolSource == nil {
		t.Fatal("missing observed-zero Hourly source/check")
	}
	for _, mutation := range []string{"whole source", "raw presence", "effective presence", "whole chart", "chart scalar", "shared axis"} {
		bundle := baseline
		bundle.EnergyExplanation.Sources = append([]EnergyDataSource(nil), baseline.EnergyExplanation.Sources...)
		id := selected.PoolSource.Identity.Source.DictionaryIndex
		for n, source := range bundle.EnergyExplanation.Sources {
			if source.Name != selected.Item.Target.SourceName || source.ReportingFrequency != "Hourly" {
				continue
			}
			if !strings.EqualFold(source.KeyValue, selected.Item.Target.SourceKey) {
				continue
			}
			switch mutation {
			case "whole source":
				bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources[:n], bundle.EnergyExplanation.Sources[n+1:]...)
			case "raw presence":
				bundle.EnergyExplanation.Sources[n].inspectorValuePresence = 2
			case "effective presence":
				bundle.EnergyExplanation.Sources[n].inspectorValuePresence = 1
			case "whole chart":
				bundle.EnergyExplanation.Sources[n].HourlyEnergy = nil
			case "chart scalar":
				chart := *source.HourlyEnergy
				chart.Values = append([]float64(nil), chart.Values...)
				chart.Values[0] = .001
				bundle.EnergyExplanation.Sources[n].HourlyEnergy = &chart
			case "shared axis":
				bundle.EnergyExplanation.HourlyLabels = nil
			}
			break
		}
		var out epathRealOracleEvidence
		if failures := epathEvaluateSQLModelChecks(&out, bundle, epathSQLModelChecks{Rows: []epathSQLModelCheck{selected}, Keys: map[string]bool{selected.Want.Key: true}}); len(failures) == 0 {
			t.Fatalf("main evaluator accepted native source %d %s", id, mutation)
		}
	}
}

func TestEnergyPathRealSQLPoolSourceIntegrationOriginalDecoderRawGuards(t *testing.T) {
	for _, context := range []string{"building", "building-period", "zone", "zone-period"} {
		for _, mutation := range []string{"explicit zero", "missing value", "null value", "string value"} {
			wire := epathOracleWireFixture()
			root := wire["energyExplanation"].(map[string]any)
			root["scope"] = map[string]any{"kind": "building"}
			node := map[string]any{"id": "present-node", "value": 0, "allocatedValue": 0}
			switch mutation {
			case "missing value":
				delete(node, "value")
			case "null value":
				node["value"] = nil
			case "string value":
				node["value"] = "0"
			}
			epathOracleWireFixtureGraph(wire, context)["nodes"] = []any{node}
			raw, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			before := append([]byte(nil), raw...)
			_, err = epathDecodeOriginalOracleCandidate(bytes.NewReader(raw))
			if (err == nil) != (mutation == "explicit zero") {
				t.Fatalf("actual original decoder %s/%s: %v", context, mutation, err)
			}
			if !bytes.Equal(before, raw) {
				t.Fatal("original decoder repaired node bytes")
			}
		}
	}
	for _, mutation := range []string{"explicit zero", "null sample", "string sample", "absent chart", "null chart"} {
		wire := epathOracleWireFixture()
		root := wire["energyExplanation"].(map[string]any)
		root["scope"] = map[string]any{"kind": "building"}
		source := map[string]any{"id": "native-source", "rawValue": 0, "effectiveValue": 0}
		values := []any{0, 0.001, 0}
		chart := any(map[string]any{"unit": "kWh", "basis": "reported_source", "values": values})
		switch mutation {
		case "null sample":
			values[0] = nil
		case "string sample":
			values[0] = "0"
		case "null chart":
			chart = nil
		}
		if mutation != "absent chart" {
			source["hourlyEnergy"] = chart
		}
		root["sources"] = []any{source}
		raw, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		_, err = epathDecodeOriginalOracleCandidate(bytes.NewReader(raw))
		valid := mutation != "null sample" && mutation != "string sample"
		if (err == nil) != valid {
			t.Fatalf("actual original decoder did not retain raw chart numeric presence for %s: %v", mutation, err)
		}
	}
}
