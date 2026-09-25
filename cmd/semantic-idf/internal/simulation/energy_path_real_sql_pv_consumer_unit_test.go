package simulation

// One transactional literal SQL fixture serves the whole integration regression.
// Neither graph scalars nor a production reader/classifier supplies expectations.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func epathSQLPVConsumerCopyChecks(input epathSQLModelChecks) epathSQLModelChecks {
	out := input
	out.RequiredPVSystems = epathSQLPVCloneDeclarations(input.RequiredPVSystems)
	out.Rows = append([]epathSQLModelCheck(nil), input.Rows...)
	for i := range out.Rows {
		row := &out.Rows[i]
		if row.PVSource != nil {
			copy := *row.PVSource
			row.PVSource = &copy
		}
		if row.Quantity != nil {
			copy := *row.Quantity
			if copy.Bounds != nil {
				bounds := *copy.Bounds
				copy.Bounds = &bounds
			}
			row.Quantity = &copy
		}
		if row.Want.Value != nil {
			value := *row.Want.Value
			row.Want.Value = &value
		}
	}
	out.Keys = map[string]bool{}
	for key, value := range input.Keys {
		out.Keys[key] = value
	}
	if input.PVSourceRegistry != nil {
		out.PVSourceRegistry = &epathSQLPVSourceRegistry{Native: append([]epathSQLPVSourceFrames(nil), input.PVSourceRegistry.Native...)}
		for i := range out.PVSourceRegistry.Native {
			frame := &out.PVSourceRegistry.Native[i]
			frame.Sources = map[int]epathSQLPVSourceIdentity{}
			for id, identity := range input.PVSourceRegistry.Native[i].Sources {
				frame.Sources[id] = identity
			}
		}
	}
	return out
}

func epathSQLPVConsumerWire() map[string]any {
	var sources []any
	for n, literal := range epathSQLPVSourceLiterals() {
		for offset, frequency := range []string{"Monthly", "Hourly"} {
			group := "System"
			if literal.meter {
				group = "Facility:" + strings.TrimSuffix(literal.name, ":Facility")
			}
			// Literal source annual powers, not values copied from compiled checks.
			value := math.Round(literal.power*8760*1000) / 1000
			sources = append(sources, map[string]any{
				"id": fmt.Sprintf("sql-rdd-%d", 2000+2*n+offset), "sourceType": "sql_report_data", "isMeter": literal.meter,
				"name": literal.name, "keyValue": strings.ToUpper(literal.key), "units": "J", "sourceUnit": "J", "normalizedUnit": "kWh",
				"reportingFrequency": frequency, "aggregationMethod": "sum_report_data", "indexGroup": group,
				"rawValue": value, "effectiveValue": value, "effectiveMultiplier": 1, "aggregationBasis": "model_total",
				"multiplierApplication": "already_model_total", "driverRole": "context", "inspectorSection": "Context",
			})
			if frequency == "Hourly" {
				_, values := epathSQLPVLiteralChart(literal.power)
				sources[len(sources)-1].(map[string]any)["hourlyEnergy"] = map[string]any{"unit": "kWh", "basis": "reported_source", "values": values}
			}
		}
	}
	labels, _ := epathSQLPVLiteralChart(0)
	return map[string]any{"energyExplanation": map[string]any{"schema": energyExplanationSchema, "scope": map[string]any{"kind": "building"}, "sources": sources, "hourlyLabels": labels}}
}

func epathSQLPVConsumerDecode(t *testing.T, wire map[string]any) PurposeResultBundle {
	t.Helper()
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := epathDecodeOriginalOracleCandidate(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func epathSQLPVConsumerHasFailure(failures []epathSQLModelFailure) bool {
	for _, failure := range failures {
		if strings.Contains(failure.Key, "pv_native_source/") || strings.Contains(failure.Message, "PV ") {
			return true
		}
	}
	return false
}

func TestEnergyPathSQLPVSourceRegistryAndConsumerIntegration(t *testing.T) {
	original, _ := epathSQLPVSourceOriginal(t)
	literals := epathSQLPVSourceLiterals()
	path := epathSQLPVSourceSQL(t, literals)
	before, err := epathSQLPVFileSHA(path)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	requests, plan := epathSQLPVSourceHandRequests(literals)
	observed.originalText, observed.executedText, observed.outputPlan = original, requests+original, &plan
	model := epathRealSQLModel{PVSystems: []epathRealSQLPVSystem{epathSQLPVOriginalDeclaration()}, Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1}}
	frames := epathSQLFrames{}
	if err := epathSQLBindPVSources(observed, model, &frames); err != nil {
		t.Fatal(err)
	}
	checks := epathSQLModelChecks{}
	if err := epathSQLModelPVSourceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	prepared, err := epathSQLPreparePVSourceChecks(checks)
	if err != nil {
		t.Fatal(err)
	}
	bundle := epathSQLPVConsumerDecode(t, epathSQLPVConsumerWire())
	if len(checks.Rows) != 80 || len(checks.Keys) != 80 || len(prepared.identities) != 40 || len(bundle.EnergyExplanation.Sources) != 40 {
		t.Fatal("finite20/40/80 census changed")
	}
	groups, fields, zeros := map[string]int{}, map[string]int{}, 0
	for _, check := range checks.Rows {
		groups[check.Want.Group]++
		fields[check.Item.Target.Field]++
		if check.Quantity == nil || check.Want.Value == nil || check.PVSource == nil || check.Want.Scope != "building" || check.Want.Zone != "" || check.Want.Period != "annual" {
			t.Fatal("source requirement lost exact mandatory scalar context")
		}
		if *check.Want.Value == 0 {
			zeros++
			lo, hi := check.Quantity.bounds()
			if lo != 0 || hi != 0 {
				t.Fatal("known native zero gained interval slack")
			}
		}
		if err := epathSQLCheckPVSourceConsumer(bundle, check, prepared); err != nil {
			t.Fatalf("%s: %v", check.Want.Key, err)
		}
	}
	if groups["carriers"] != 68 || groups["endUses"] != 4 || groups["loads"] != 8 || fields["rawValue"] != 40 || fields["effectiveValue"] != 40 || zeros != 8 {
		t.Fatalf("literal group/zero census changed: %v %v zero=%d", groups, fields, zeros)
	}
	var out epathRealOracleEvidence
	if failures := epathEvaluateSQLModelChecks(&out, bundle, checks); len(failures) != 0 {
		t.Fatalf("required source dispatcher: %v", failures)
	}
	if len(out.Metrics) != 80 {
		t.Fatal("dispatcher omitted native source rows")
	}
	coverage := epathSQLModelCoverage(bundle, checks)
	if epathSQLPVConsumerHasFailure(coverage.Failures) {
		t.Fatalf("source registry coverage failed: %v", coverage.Failures)
	}
	// This source-only hand graph intentionally has no quality proofs. It is
	// NOT an all-eight-group coverage or acceptance fixture.
	if len(coverage.Failures) == 0 {
		t.Fatal("source-only proof improperly granted full graph quality coverage")
	}
	if err := epathSQLValidatePVSourceChecksForModel(checks, &model); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLValidatePVCompiledSourceChecks(checks, model); err != nil {
		t.Fatal(err)
	}
	t.Run("source proof cannot authorize graph fields", func(t *testing.T) {
		withNode := bundle
		withNode.EnergyExplanation.Nodes = []EnergyExplanationNode{{ID: "unproved-source-promotion", Level: "end_use", EndUse: "interior_lights", Carrier: "electricity", Unit: "kWh", ScaleDomain: "site", Value: 1}}
		report := epathSQLModelCoverage(withNode, checks)
		found := false
		for _, record := range report.Records {
			if record.Collection == "nodes" && record.ID == "unproved-source-promotion" {
				found = true
				if len(record.Selectors["value"]) != 0 {
					t.Fatal("source scalar proof authorized an unrelated graph node")
				}
			}
		}
		if !found {
			t.Fatal("unproved graph node disappeared from coverage")
		}
	})

	t.Run("required registry and dispatcher removal guards", func(t *testing.T) {
		cases := []struct {
			name   string
			change func(*epathSQLModelChecks)
		}{
			{"nil proof", func(c *epathSQLModelChecks) { c.Rows[0].PVSource = nil }},
			{"wrong dictionary", func(c *epathSQLModelChecks) { c.Rows[0].PVSource.DictionaryIndex++ }},
			{"wrong spec", func(c *epathSQLModelChecks) { c.Rows[0].PVSource.SpecID = "storage.charge" }},
			{"wrong system", func(c *epathSQLModelChecks) { c.Rows[0].PVSource.SystemID = "foreign" }},
			{"wrong selector", func(c *epathSQLModelChecks) { c.Rows[0].Item.Target.Frequency = "Timestep" }},
			{"wrong quantity", func(c *epathSQLModelChecks) { c.Rows[0].Quantity.Value++ }},
			{"unknown status", func(c *epathSQLModelChecks) { c.Rows[0].Want.Status = "unavailable" }},
			{"optional", func(c *epathSQLModelChecks) { c.Rows[0].OptionalPresentation = true }},
			{"remove row", func(c *epathSQLModelChecks) { c.Rows = c.Rows[1:] }},
			{"remove key", func(c *epathSQLModelChecks) { delete(c.Keys, c.Rows[0].Want.Key) }},
			{"remove both", func(c *epathSQLModelChecks) { delete(c.Keys, c.Rows[0].Want.Key); c.Rows = c.Rows[1:] }},
			{"remove all rows and keys", func(c *epathSQLModelChecks) { c.Rows = nil; c.Keys = nil }},
			{"nil registry", func(c *epathSQLModelChecks) { c.PVSourceRegistry = nil }},
			{"nil frame", func(c *epathSQLModelChecks) { c.PVSourceRegistry.Native = nil }},
			{"nil declaration", func(c *epathSQLModelChecks) { c.RequiredPVSystems = nil }},
			{"duplicate row", func(c *epathSQLModelChecks) { c.Rows = append(c.Rows, c.Rows[0]) }},
			{"false key", func(c *epathSQLModelChecks) { c.Keys[c.Rows[0].Want.Key] = false }},
			{"foreign reserved key", func(c *epathSQLModelChecks) { c.Keys["loads|building||annual|pv_native_source/foreign"] = true }},
			{"native factor", func(c *epathSQLModelChecks) {
				f := &c.PVSourceRegistry.Native[0]
				s := f.Sources[2000]
				s.EffectiveMultiplier = 3
				f.Sources[2000] = s
			}},
			{"native calendar", func(c *epathSQLModelChecks) {
				f := &c.PVSourceRegistry.Native[0]
				s := f.Sources[2000]
				s.Rows = append([]epathSQLPVNativeRow(nil), s.Rows...)
				s.Rows[0].SimulationDays--
				f.Sources[2000] = s
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				changed := epathSQLPVConsumerCopyChecks(checks)
				tc.change(&changed)
				if _, err := epathSQLPreparePVSourceChecks(changed); err == nil {
					t.Fatal("altered/removed native registry accepted")
				}
				var output epathRealOracleEvidence
				if !epathSQLPVConsumerHasFailure(epathEvaluateSQLModelChecks(&output, bundle, changed)) {
					t.Fatal("top evaluator bypassed native registry")
				}
				if !epathSQLPVConsumerHasFailure(epathSQLModelCoverage(bundle, changed).Failures) {
					t.Fatal("top coverage bypassed native registry")
				}
			})
		}
	})

	t.Run("original wire identity presence and native values", func(t *testing.T) {
		cases := []struct {
			name   string
			change func([]any) []any
		}{
			{"removed source", func(s []any) []any { return s[1:] }},
			{"duplicate id", func(s []any) []any { return append(s, s[0]) }},
			{"borrowed actual id", func(s []any) []any { s[0].(map[string]any)["id"] = "sql-rdd-9999"; return s }},
			{"duplicate semantic identity", func(s []any) []any {
				copy := map[string]any{}
				for k, v := range s[0].(map[string]any) {
					copy[k] = v
				}
				copy["id"] = "sql-rdd-9999"
				return append(s, copy)
			}},
			{"wrong meter flag", func(s []any) []any { s[0].(map[string]any)["isMeter"] = true; return s }},
			{"wrong native key", func(s []any) []any { s[0].(map[string]any)["keyValue"] = "foreign"; return s }},
			{"wrong frequency", func(s []any) []any { s[0].(map[string]any)["reportingFrequency"] = "Timestep"; return s }},
			{"wrong units", func(s []any) []any { s[0].(map[string]any)["units"] = "W"; return s }},
			{"wrong index group", func(s []any) []any { s[0].(map[string]any)["indexGroup"] = "Plant"; return s }},
			{"wrong factor", func(s []any) []any { s[0].(map[string]any)["effectiveMultiplier"] = 3; return s }},
			{"physical owner as output", func(s []any) []any {
				s[0].(map[string]any)["objectIndex"] = frames.PVSystems[0].Sources[2000].Spec.Owner.ObjectIndex
				return s
			}},
			{"zero raw omitted", func(s []any) []any { delete(s[18].(map[string]any), "rawValue"); return s }},
			{"zero raw null", func(s []any) []any { s[18].(map[string]any)["rawValue"] = nil; return s }},
			{"zero effective null", func(s []any) []any { s[18].(map[string]any)["effectiveValue"] = nil; return s }},
			{"negative clamped", func(s []any) []any {
				s[16].(map[string]any)["rawValue"] = 0
				s[16].(map[string]any)["effectiveValue"] = 0
				return s
			}},
			{"monthly scalar borrowed by hourly", func(s []any) []any { s[1].(map[string]any)["rawValue"] = 8761; return s }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				wire := epathSQLPVConsumerWire()
				result := wire["energyExplanation"].(map[string]any)
				result["sources"] = tc.change(result["sources"].([]any))
				changed := epathSQLPVConsumerDecode(t, wire)
				fail := false
				for _, check := range checks.Rows {
					if epathSQLCheckPVSourceConsumer(changed, check, prepared) != nil {
						fail = true
						break
					}
				}
				if !fail {
					t.Fatal("bad source wire accepted by typed native consumer")
				}
				var output epathRealOracleEvidence
				if !epathSQLPVConsumerHasFailure(epathEvaluateSQLModelChecks(&output, changed, checks)) {
					t.Fatal("top evaluator bypassed typed source consumer")
				}
				if !epathSQLPVConsumerHasFailure(epathSQLModelCoverage(changed, checks).Failures) {
					t.Fatal("top coverage bypassed typed source consumer")
				}
			})
		}
	})

	t.Run("external model anchor and no opt in", func(t *testing.T) {
		if err := epathSQLValidatePVSourceChecksForModel(epathSQLModelChecks{}, &model); err == nil {
			t.Fatal("pending/model boundary accepted deletion of all PV evidence")
		}
		if err := epathSQLValidatePVSourceChecksForModel(checks, nil); err == nil {
			t.Fatal("undeclared native evidence accepted")
		}
		legacy := epathSQLModelChecks{}
		if err := epathSQLModelPVSourceChecks(epathSQLFrames{}, epathRealSQLModel{}, &legacy); err != nil {
			t.Fatal(err)
		}
		if err := epathSQLValidatePVSourceChecksForModel(legacy, nil); err != nil || len(legacy.Rows) != 0 || len(legacy.Keys) != 0 {
			t.Fatal("legacy opt-out changed")
		}
		if err := epathSQLBindPVSources(observed, model, nil); err == nil {
			t.Fatal("nil frames accepted")
		}
		copyFrames := frames
		if err := epathSQLBindPVSources(observed, model, &copyFrames); err == nil {
			t.Fatal("native frame rebound twice")
		}
		if err := epathSQLBindPVSources(observed, epathRealSQLModel{}, &copyFrames); err == nil {
			t.Fatal("unrequired frame accepted")
		}
		badObserved := observed
		badObserved.originalText = ""
		if err := epathSQLBindPVSources(badObserved, model, &epathSQLFrames{}); err == nil {
			t.Fatal("unbound original accepted")
		}
		if err := epathSQLModelPVSourceChecks(frames, model, &checks); err == nil {
			t.Fatal("source keys registered twice")
		}
	})
	if !reflect.DeepEqual(model.PVSystems, []epathRealSQLPVSystem{epathSQLPVOriginalDeclaration()}) {
		t.Fatal("source registry mutated model declaration")
	}
	after, err := epathSQLPVFileSHA(path)
	if err != nil || before != after {
		t.Fatal("oracle consumer/binder modified native SQL")
	}
}

func TestEnergyPathSQLPVSourceSignedSerializationHasNoGenericSlack(t *testing.T) {
	for _, value := range []float64{0, -.0004, -.0006, -29.784, 1000000, -1000000} {
		identity := epathSQLPVSourceIdentity{Spec: epathSQLPVNativeSpec{Sign: "signed"}, Source: epathRealSQLSource{SourceUnit: "J", RawSum: epathOracleNumber(value * 3600000), EnergyKWh: epathOracleNumber(value)}, NativeAnnualJ: value * 3600000, NativeAnnualKWh: value, AggregationBasis: "model_total", EffectiveMultiplier: 1}
		q, err := epathSQLPVSerializedSourceQuantity(identity)
		if err != nil {
			t.Fatal(err)
		}
		want := math.Round(value*1000) / 1000
		if q.Value != want {
			t.Fatal("signed native source quantization changed")
		}
		if err := epathSQLPVPersistedSourceValue(want, q); err != nil {
			t.Fatal(err)
		}
		lo, hi := q.bounds()
		if err := epathSQLPVPersistedSourceValue(hi+.001, q); err == nil {
			t.Fatal("extra positive serialization quantum accepted")
		}
		if err := epathSQLPVPersistedSourceValue(lo-.001, q); err == nil {
			t.Fatal("extra negative serialization quantum accepted")
		}
		if value == 0 && (lo != 0 || hi != 0) {
			t.Fatal("known zero became a numeric uncertainty interval")
		}
		if value == 1000000 && epathCheckSQLModelQuantity(epathOracleNumber(value+.001), &q) != nil {
			t.Fatal("test no longer exposes the generic relative-slack difference")
		}
	}
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), .0001} {
		if epathSQLPVPersistedSourceValue(v, epathSQLQuantity{}) == nil {
			t.Fatal("invalid/non-grid source accepted")
		}
	}
}
