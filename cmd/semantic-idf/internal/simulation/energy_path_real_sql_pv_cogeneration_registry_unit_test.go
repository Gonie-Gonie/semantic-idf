package simulation

// Transactional literal42 fixtures. The required4 extension does not alter
// the core20/40/80 registry or derive expectations from a candidate graph.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func epathSQLPVCogenerationRegistryCopy(t *testing.T, input epathSQLModelChecks) epathSQLModelChecks {
	t.Helper()
	out := epathSQLPVConsumerCopyChecks(input)
	if input.RequiredPVCogeneration != nil {
		value := *input.RequiredPVCogeneration
		out.RequiredPVCogeneration = &value
	}
	if input.PVCogenerationRegistry != nil {
		out.PVCogenerationRegistry = &epathSQLPVCogenerationRegistry{SQLPath: input.PVCogenerationRegistry.SQLPath, Native: epathSQLPVCogenerationClone(t, input.PVCogenerationRegistry.Native)}
	}
	for i := range out.Rows {
		if out.Rows[i].PVCogenerationSource != nil {
			value := *out.Rows[i].PVCogenerationSource
			out.Rows[i].PVCogenerationSource = &value
		}
	}
	return out
}

func epathSQLPVCogenerationRegistryWithoutCG(input epathSQLModelChecks) epathSQLModelChecks {
	out := input
	out.RequiredPVCogeneration = nil
	out.PVCogenerationRegistry = nil
	out.Rows = nil
	out.Keys = map[string]bool{}
	for _, row := range input.Rows {
		if !epathSQLPVCogenerationHasSourceCheck(row) {
			out.Rows = append(out.Rows, row)
			out.Keys[row.Want.Key] = true
		}
	}
	return out
}

// Source scalars/charts are independently built from the literal power and
// fixed2017 calendar that seeded SQL, never from the oracle's expected rows.
func epathSQLPVCogenerationConsumerWire(literals []epathSQLPVSourceLiteral, onlyCG bool) map[string]any {
	labels, _ := epathSQLPVLiteralChart(0)
	var sources []any
	for n, literal := range literals {
		if onlyCG && literal.id != "cogeneration.electricity" {
			continue
		}
		for offset, frequency := range []string{"Monthly", "Hourly"} {
			group := "System"
			if literal.meter {
				group = "Facility:" + strings.TrimSuffix(literal.name, ":Facility")
			}
			value := math.Round(literal.power*8760*1000) / 1000
			source := map[string]any{"id": fmt.Sprintf("sql-rdd-%d", 2000+2*n+offset), "sourceType": "sql_report_data", "isMeter": literal.meter, "name": literal.name, "keyValue": strings.ToUpper(literal.key), "units": "J", "sourceUnit": "J", "normalizedUnit": "kWh", "reportingFrequency": frequency, "aggregationMethod": "sum_report_data", "indexGroup": group, "rawValue": value, "effectiveValue": value, "effectiveMultiplier": 1, "aggregationBasis": "model_total", "multiplierApplication": "already_model_total", "driverRole": "context", "inspectorSection": "Context"}
			if frequency == "Hourly" {
				_, literalValues := epathSQLPVLiteralChart(literal.power)
				values := make([]any, len(literalValues))
				for i, value := range literalValues {
					values[i] = value
				}
				source["hourlyEnergy"] = map[string]any{"unit": "kWh", "basis": "reported_source", "values": values}
			}
			sources = append(sources, source)
		}
	}
	return map[string]any{"energyExplanation": map[string]any{"schema": energyExplanationSchema, "scope": map[string]any{"kind": "building"}, "sources": sources, "hourlyLabels": labels}}
}

func TestEnergyPathSQLPVCogenerationRegistryFourAndCompleteConsumer(t *testing.T) {
	original, proof := epathSQLPVSourceOriginal(t)
	literals := epathSQLPVBalanceHandLiterals()
	// Positive native annual .0000876kWh rounds to0 once; zero presentation
	// still requires actual raw/effective numeric presence and all8760 samples.
	for i := range literals {
		if literals[i].id == "inverter.ancillary" || literals[i].id == "cogeneration.electricity" {
			literals[i].power = 1e-8
		}
	}
	path := epathSQLPVSourceSQL(t, literals, `CREATE INDEX cg_registry_dictionary_time ON ReportData(ReportDataDictionaryIndex,TimeIndex)`)
	mtdPath := filepath.Join(filepath.Dir(path), "eplusout.mtd")
	if err := os.WriteFile(mtdPath, []byte(epathSQLPVCogenerationHandMTD(proof)), 0600); err != nil {
		t.Fatal(err)
	}
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
	model := epathRealSQLModel{PVSystems: []epathRealSQLPVSystem{proof.Declaration}, PVCogeneration: &epathRealSQLPVCogeneration{SystemID: proof.Declaration.ID, Boundary: "shop-25.1-native-cogeneration", MTDFile: "eplusout.mtd"}, Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1}}
	frames := epathSQLFrames{}
	if err := epathSQLBindPVSources(observed, model, &frames); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLBindPVCogenerationSources(observed, model, &frames); err != nil {
		t.Fatal(err)
	}
	checks := epathSQLModelChecks{}
	if err := epathSQLModelPVSourceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	coreRows := append([]epathSQLModelCheck(nil), checks.Rows...)
	if err := epathSQLModelPVCogenerationSourceChecks(observed, frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLValidatePVAndCogenerationCompiledSourceChecks(checks, model); err != nil {
		t.Fatal(err)
	}
	core, cg, err := epathSQLPreparePVAndCogenerationSourceChecksForModel(checks, &model, &observed)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 84 || len(checks.Keys) != 84 || len(core.canonical) != 80 || len(core.identities) != 40 || len(cg.canonical) != 4 || len(cg.adapter.identities) != 2 || !reflect.DeepEqual(coreRows, checks.Rows[:80]) {
		t.Fatal("separate required4 altered or replaced core80")
	}
	for _, key := range []string{
		"endUses|building||annual|pv_cogeneration_source/cogeneration.electricity/Cogeneration:Electricity/Monthly/rawValue",
		"endUses|building||annual|pv_cogeneration_source/cogeneration.electricity/Cogeneration:Electricity/Monthly/effectiveValue",
		"endUses|building||annual|pv_cogeneration_source/cogeneration.electricity/Cogeneration:Electricity/Hourly/rawValue",
		"endUses|building||annual|pv_cogeneration_source/cogeneration.electricity/Cogeneration:Electricity/Hourly/effectiveValue",
	} {
		if _, found := cg.canonical[key]; !found || !checks.Keys[key] {
			t.Fatalf("missing literal required parent key %s", key)
		}
	}
	bundle := epathSQLPVConsumerDecode(t, epathSQLPVCogenerationConsumerWire(literals, false))
	t.Run("actual evaluator coverage and pending dispatch", func(t *testing.T) {
		output := observed
		if failures := epathEvaluateSQLModelChecks(&output, bundle, checks, &model); len(failures) != 0 || len(output.Metrics) != 84 {
			t.Fatalf("required84 evaluator: %v metrics=%d", failures, len(output.Metrics))
		}
		coverage := epathSQLModelCoverage(bundle, checks)
		for _, failure := range coverage.Failures {
			if strings.Contains(failure.Key, "pv_native_source/") || strings.Contains(failure.Key, "pv_cogeneration_source/") {
				t.Fatalf("native source coverage dispatch: %v", failure)
			}
		}
		if len(coverage.Failures) == 0 {
			t.Fatal("source-only hand fixture must not authorize full graph quality coverage")
		}
		output = observed
		if len(epathEvaluateSQLModelChecks(&output, bundle, checks)) == 0 {
			t.Fatal("CG evaluator accepted no external recipe")
		}
		without := epathSQLPVCogenerationRegistryWithoutCG(checks)
		output = observed
		output.Metrics = []epathRealOracleMetric{{Key: "stale"}}
		if failures := epathEvaluateSQLModelChecks(&output, bundle, without, &model); len(failures) == 0 || len(output.Metrics) != 0 || len(output.CheckedGroups) != 0 {
			t.Fatal("external CG deletion did not block evaluator and clear stale approval")
		}
		changed := epathSQLPVCogenerationRegistryCopy(t, checks)
		changed.Rows[80].PVCogenerationSource = nil
		output = observed
		if len(epathEvaluateSQLModelChecks(&output, bundle, changed, &model)) == 0 {
			t.Fatal("CG proof deletion escaped evaluator")
		}
		report := epathSQLModelCoverage(bundle, changed)
		found := false
		for _, failure := range report.Failures {
			found = found || strings.Contains(failure.Message, "CG required source")
		}
		if !found {
			t.Fatalf("CG proof deletion escaped coverage: %v", report.Failures)
		}
		wire := epathSQLPVCogenerationConsumerWire(literals, false)
		sources := wire["energyExplanation"].(map[string]any)["sources"].([]any)
		delete(sources[len(sources)-2].(map[string]any), "rawValue")
		output = observed
		if failures := epathEvaluateSQLModelChecks(&output, epathSQLPVConsumerDecode(t, wire), checks, &model); len(failures) == 0 {
			t.Fatal("CG missing zero scalar escaped actual consumer dispatch")
		}
		pending := epathOraclePendingUnitFixture(t)
		pending.recipe.SQLModel = &model
		pending.checks = without
		pending.checks.RequireCoverage = true
		if err := pending.write(); err == nil || !strings.Contains(err.Error(), "external required recipe") {
			t.Fatalf("pending all-state deletion did not hit external CG gate: %v", err)
		}
		if _, err := os.Stat(pending.destination); !os.IsNotExist(err) {
			t.Fatal("rejected CG pending created an output")
		}
	})
	groups := map[string]int{}
	cgRows := 0
	for _, check := range checks.Rows {
		groups[check.Want.Group]++
		if epathSQLPVCogenerationHasSourceCheck(check) {
			cgRows++
			if check.PVCogenerationSource == nil || check.PVSource != nil || check.Quantity == nil || check.Want.Value == nil || *check.Want.Value != 0 || check.OptionalPresentation || check.Item.Target.Collection != "sources" {
				t.Fatal("rounded0 parent lost mandatory source-only scalar")
			}
			lo, hi := check.Quantity.bounds()
			if lo != 0 || hi != 0 {
				t.Fatal("rounded native scalar gained an extra presentation quantum")
			}
			if err := epathSQLCheckPVCogenerationSourceConsumer(bundle, check, cg); err != nil {
				t.Fatalf("CG %s: %v", check.Want.Key, err)
			}
		} else if err := epathSQLCheckPVSourceConsumerComplete(bundle, check, core); err != nil {
			t.Fatalf("core %s: %v", check.Want.Key, err)
		}
	}
	if cgRows != 4 || groups["carriers"] != 68 || groups["loads"] != 8 || groups["endUses"] != 8 {
		t.Fatalf("source registry group census changed: %v", groups)
	}
	for _, frequency := range []string{"Monthly", "Hourly"} {
		identity := frames.PVCogeneration.Parents[frequency]
		if identity.NativeAnnualKWh <= 0 || identity.Source.EnergyKWh == nil || *identity.Source.EnergyKWh <= 0 {
			t.Fatal("positive native parent was rewritten as measured0")
		}
	}
	for _, identity := range cg.adapter.identities {
		if identity.Frequency == "Monthly" && identity.Chart != nil || identity.Frequency == "Hourly" && (identity.Chart == nil || len(identity.Chart.Values) != 8760) {
			t.Fatal("local scalar adapter lost exact chart frequency")
		}
	}

	t.Run("immutable required row registry", func(t *testing.T) {
		cases := []struct {
			name   string
			change func(*epathSQLModelChecks)
		}{
			{"proof missing", func(c *epathSQLModelChecks) { c.Rows[80].PVCogenerationSource = nil }},
			{"dictionary changed", func(c *epathSQLModelChecks) { c.Rows[80].PVCogenerationSource.DictionaryIndex++ }},
			{"spec changed", func(c *epathSQLModelChecks) { c.Rows[80].PVCogenerationSource.SpecID = "inverter.ancillary" }},
			{"system changed", func(c *epathSQLModelChecks) { c.Rows[80].PVCogenerationSource.SystemID = "foreign" }},
			{"foreign PV proof", func(c *epathSQLModelChecks) { c.Rows[80].PVSource = c.Rows[80].PVCogenerationSource }},
			{"target changed", func(c *epathSQLModelChecks) { c.Rows[80].Item.Target.Frequency = "Timestep" }},
			{"field changed", func(c *epathSQLModelChecks) {
				c.Rows[80].Item.Target.Field = "rawValue"
				c.Rows[80].PVCogenerationSource.Field = "rawValue"
			}},
			{"source key changed", func(c *epathSQLModelChecks) { c.Rows[80].Item.Target.SourceKey = "Heating:Electricity" }},
			{"period changed", func(c *epathSQLModelChecks) { c.Rows[80].Item.Period = "M01"; c.Rows[80].Want.Period = "M01" }},
			{"scope changed", func(c *epathSQLModelChecks) { c.Rows[80].Item.Scope = "zone"; c.Rows[80].Want.Scope = "zone" }},
			{"unit changed", func(c *epathSQLModelChecks) { c.Rows[80].Item.Unit = "J"; c.Rows[80].Want.Unit = "J" }},
			{"graph target", func(c *epathSQLModelChecks) {
				c.Rows[80].Item.Target.Collection = "nodes"
				c.Rows[80].Item.Target.Field = "value"
			}},
			{"item key changed", func(c *epathSQLModelChecks) { c.Rows[80].Item.Key += "/foreign" }},
			{"expected key changed", func(c *epathSQLModelChecks) { c.Rows[80].Want.Key += "/foreign" }},
			{"quantity changed", func(c *epathSQLModelChecks) { c.Rows[80].Quantity.Value++ }},
			{"unknown status", func(c *epathSQLModelChecks) { c.Rows[80].Want.Status = "unavailable" }},
			{"optional zero", func(c *epathSQLModelChecks) { c.Rows[80].OptionalPresentation = true }},
			{"remove row", func(c *epathSQLModelChecks) { c.Rows = c.Rows[:83] }},
			{"remove key", func(c *epathSQLModelChecks) { delete(c.Keys, c.Rows[80].Want.Key) }},
			{"remove both", func(c *epathSQLModelChecks) { delete(c.Keys, c.Rows[83].Want.Key); c.Rows = c.Rows[:83] }},
			{"remove all four with keys", func(c *epathSQLModelChecks) {
				for _, r := range c.Rows[80:] {
					delete(c.Keys, r.Want.Key)
				}
				c.Rows = c.Rows[:80]
			}},
			{"remove declaration", func(c *epathSQLModelChecks) { c.RequiredPVCogeneration = nil }},
			{"remove registry", func(c *epathSQLModelChecks) { c.PVCogenerationRegistry = nil }},
			{"remove parent", func(c *epathSQLModelChecks) { delete(c.PVCogenerationRegistry.Native.Parents, "Hourly") }},
			{"duplicate parent", func(c *epathSQLModelChecks) {
				c.PVCogenerationRegistry.Native.Parents["Hourly"] = c.PVCogenerationRegistry.Native.Parents["Monthly"]
			}},
			{"duplicate row", func(c *epathSQLModelChecks) { c.Rows = append(c.Rows, c.Rows[80]) }},
			{"false key", func(c *epathSQLModelChecks) { c.Keys[c.Rows[80].Want.Key] = false }},
			{"orphan key", func(c *epathSQLModelChecks) { c.Keys["endUses|building||annual|pv_cogeneration_source/foreign"] = true }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				changed := epathSQLPVCogenerationRegistryCopy(t, checks)
				tc.change(&changed)
				if err := epathSQLValidatePVAndCogenerationCompiledSourceChecks(changed, model); err == nil {
					t.Fatal("changed/missing required parent accepted by final census")
				}
				if _, _, err := epathSQLPreparePVAndCogenerationSourceChecks(changed); err == nil {
					t.Fatal("changed/missing required parent accepted by boundary preparation")
				}
			})
		}
	})

	t.Run("original source scalar shape and chart", func(t *testing.T) {
		cases := []struct {
			name   string
			change func(map[string]any, []any)
		}{
			{"raw omitted", func(_ map[string]any, s []any) { delete(s[0].(map[string]any), "rawValue") }},
			{"raw null", func(_ map[string]any, s []any) { s[0].(map[string]any)["rawValue"] = nil }},
			{"effective omitted", func(_ map[string]any, s []any) { delete(s[0].(map[string]any), "effectiveValue") }},
			{"effective null", func(_ map[string]any, s []any) { s[0].(map[string]any)["effectiveValue"] = nil }},
			{"wrong scalar", func(_ map[string]any, s []any) { s[0].(map[string]any)["rawValue"] = .001 }},
			{"multiplier missing", func(_ map[string]any, s []any) { delete(s[0].(map[string]any), "effectiveMultiplier") }},
			{"basis changed", func(_ map[string]any, s []any) { s[0].(map[string]any)["aggregationBasis"] = "representative_zone" }},
			{"wrong raw units", func(_ map[string]any, s []any) { s[0].(map[string]any)["units"] = "W" }},
			{"borrowed key", func(_ map[string]any, s []any) { s[0].(map[string]any)["keyValue"] = "Heating:Electricity" }},
			{"wrong index group", func(_ map[string]any, s []any) { s[0].(map[string]any)["indexGroup"] = "Plant" }},
			{"wrong output opener", func(_ map[string]any, s []any) { s[0].(map[string]any)["objectIndex"] = 0 }},
			{"false zone owner", func(_ map[string]any, s []any) { s[0].(map[string]any)["zoneName"] = "ZONE" }},
			{"scope detail", func(_ map[string]any, s []any) {
				s[0].(map[string]any)["scopeDetails"] = []any{map[string]any{"scope": map[string]any{"kind": "zone", "zoneName": "ZONE"}}}
			}},
			{"allocation flag", func(_ map[string]any, s []any) { s[0].(map[string]any)["allocationApplied"] = true }},
			{"allocation value", func(_ map[string]any, s []any) { s[0].(map[string]any)["allocatedValue"] = .001 }},
			{"related physical owner", func(_ map[string]any, s []any) { s[0].(map[string]any)["relatedEntityIds"] = []string{"component:7"} }},
			{"derived formula", func(_ map[string]any, s []any) { s[0].(map[string]any)["formula"] = "parent-member" }},
			{"derived input", func(_ map[string]any, s []any) { s[0].(map[string]any)["inputSourceIds"] = []string{"sql-rdd-2018"} }},
			{"source missing", func(r map[string]any, s []any) { r["sources"] = s[1:] }},
			{"duplicate ID", func(r map[string]any, s []any) { r["sources"] = append(s, s[0]) }},
			{"hourly chart missing", func(_ map[string]any, s []any) { delete(s[1].(map[string]any), "hourlyEnergy") }},
			{"monthly borrowed chart", func(_ map[string]any, s []any) {
				s[0].(map[string]any)["hourlyEnergy"] = s[1].(map[string]any)["hourlyEnergy"]
			}},
			{"wrong hourly value", func(_ map[string]any, s []any) {
				s[1].(map[string]any)["hourlyEnergy"].(map[string]any)["values"].([]any)[0] = .001
			}},
			{"hourly null sample", func(_ map[string]any, s []any) {
				s[1].(map[string]any)["hourlyEnergy"].(map[string]any)["values"].([]any)[0] = nil
			}},
			{"axis missing", func(r map[string]any, _ []any) { delete(r, "hourlyLabels") }},
			{"start versus end hour", func(r map[string]any, _ []any) { r["hourlyLabels"].([]string)[0] = "01-01 00:00" }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				wire := epathSQLPVCogenerationConsumerWire(literals, true)
				result := wire["energyExplanation"].(map[string]any)
				tc.change(result, result["sources"].([]any))
				data, err := json.Marshal(wire)
				if err != nil {
					t.Fatal(err)
				}
				changed, err := epathDecodeOriginalOracleCandidate(bytes.NewReader(data))
				if err != nil {
					return
				} // malformed raw sample is rejected before numeric decoding
				for _, row := range checks.Rows[80:] {
					if epathSQLCheckPVCogenerationSourceConsumer(changed, row, cg) != nil {
						return
					}
				}
				t.Fatal("altered parent observation/knownness/shape/chart accepted")
			})
		}
	})

	t.Run("external option and file anchor survive deletion", func(t *testing.T) {
		without := epathSQLPVCogenerationRegistryWithoutCG(checks)
		legacyModel := model
		legacyModel.PVCogeneration = nil
		if _, _, err := epathSQLPreparePVAndCogenerationSourceChecksForModel(without, &legacyModel, nil); err != nil {
			t.Fatal("legitimate core-only path changed", err)
		}
		if _, _, err := epathSQLPreparePVAndCogenerationSourceChecksForModel(without, &model, &observed); err == nil {
			t.Fatal("deleting all4 rows/keys/declaration/registry bypassed external requirement")
		}
		if _, _, err := epathSQLPreparePVAndCogenerationSourceChecksForModel(checks, &model, nil); err == nil {
			t.Fatal("CG claimed external provenance without caller evidence")
		}
		if _, _, err := epathSQLPreparePVAndCogenerationSourceChecksForModel(checks, nil, &observed); err == nil {
			t.Fatal("undeclared native supplement accepted")
		}
		orphan := epathSQLPVCogenerationRegistryCopy(t, without)
		orphan.Rows = append(orphan.Rows, checks.Rows[80])
		orphan.Keys[checks.Rows[80].Want.Key] = true
		if _, _, err := epathSQLPreparePVAndCogenerationSourceChecks(orphan); err == nil {
			t.Fatal("parent marker/proof accepted without the independent option")
		}
		changedObserved := observed
		changedObserved.originalText += "\n! unrelated original"
		if _, _, err := epathSQLPreparePVAndCogenerationSourceChecksForModel(checks, &model, &changedObserved); err == nil {
			t.Fatal("retained registry substituted another external original")
		}
		bad := epathSQLPVCogenerationRegistryCopy(t, checks)
		bad.PVCogenerationRegistry.Native.MTDPath = filepath.Join(t.TempDir(), "eplusout.mtd")
		if _, _, err := epathSQLPreparePVAndCogenerationSourceChecks(bad); err == nil {
			t.Fatal("retained check accepted an unrelated MTD path")
		}
		bad = epathSQLPVCogenerationRegistryCopy(t, checks)
		bad.PVCogenerationRegistry.SQLPath = filepath.Join(t.TempDir(), "forged.sql")
		if _, _, err := epathSQLPreparePVAndCogenerationSourceChecksForModel(bad, &model, &observed); err == nil {
			t.Fatal("forged registry SQL path bypassed external capture")
		}
		bad = epathSQLPVCogenerationRegistryCopy(t, checks)
		bad.PVCogenerationRegistry.Native.Membership.MTDText += "\nchanged"
		bad.PVCogenerationRegistry.Native.Membership.MTDSHA256 = epathSQLPVProofSHA(bad.PVCogenerationRegistry.Native.Membership.MTDText)
		if _, _, err := epathSQLPreparePVAndCogenerationSourceChecks(bad); err == nil {
			t.Fatal("retained forged bytes and recomputed hash replaced actual MTD")
		}
	})

	t.Run("binding and registration do not mutate callers", func(t *testing.T) {
		if checks.RequiredPVCogeneration == model.PVCogeneration || checks.PVCogenerationRegistry == nil || &checks.PVCogenerationRegistry.Native == frames.PVCogeneration {
			t.Fatal("registry aliases mutable input requirement/frame")
		}
		copyFrames := frames
		if err := epathSQLBindPVCogenerationSources(observed, model, &copyFrames); err == nil {
			t.Fatal("native parent rebound twice")
		}
		if err := epathSQLBindPVCogenerationSources(observed, model, nil); err == nil {
			t.Fatal("nil frame accepted")
		}
		if err := epathSQLBindPVCogenerationSources(observed, epathRealSQLModel{}, &copyFrames); err == nil {
			t.Fatal("unrequired CG frame accepted")
		}
		if err := epathSQLModelPVCogenerationSourceChecks(observed, frames, model, &checks); err == nil {
			t.Fatal("parent requirements registered twice")
		}
		if !reflect.DeepEqual(coreRows, checks.Rows[:80]) || len(checks.Rows) != 84 {
			t.Fatal("consumer or failed registration mutated core/check rows")
		}
		if _, _, err := epathSQLPreparePVAndCogenerationSourceChecksForModel(epathSQLModelChecks{}, nil, nil); err != nil {
			t.Fatal("legacy empty path changed", err)
		}
	})
	after, err := epathSQLPVFileSHA(path)
	if err != nil || before != after {
		t.Fatal("source registry/consumer changed the literal native SQL")
	}
}

// This second fixture proves all four mandatory scalars for actual native zero;
// the larger regression above proves positive native energy rounded to zero.
func TestEnergyPathSQLPVCogenerationRegistryMeasuredZeroFour(t *testing.T) {
	original, proof := epathSQLPVSourceOriginal(t)
	literals := epathSQLPVBalanceHandLiterals()
	for i := range literals {
		if literals[i].id == "inverter.ancillary" || literals[i].id == "cogeneration.electricity" {
			literals[i].power = 0
		}
	}
	path := epathSQLPVSourceSQL(t, literals, `CREATE INDEX cg_zero_dictionary_time ON ReportData(ReportDataDictionaryIndex,TimeIndex)`)
	mtdPath := filepath.Join(filepath.Dir(path), "eplusout.mtd")
	if err := os.WriteFile(mtdPath, []byte(epathSQLPVCogenerationHandMTD(proof)), 0600); err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	requests, plan := epathSQLPVSourceHandRequests(literals)
	observed.originalText, observed.executedText, observed.outputPlan = original, requests+original, &plan
	model := epathRealSQLModel{
		PVSystems:      []epathRealSQLPVSystem{proof.Declaration},
		PVCogeneration: &epathRealSQLPVCogeneration{SystemID: proof.Declaration.ID, Boundary: "shop-25.1-native-cogeneration", MTDFile: "eplusout.mtd"},
		Precision:      epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1},
	}
	frames := epathSQLFrames{}
	if err := epathSQLBindPVSources(observed, model, &frames); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLBindPVCogenerationSources(observed, model, &frames); err != nil {
		t.Fatal(err)
	}
	checks := epathSQLModelChecks{}
	if err := epathSQLModelPVSourceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelPVCogenerationSourceChecks(observed, frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	_, prepared, err := epathSQLPreparePVAndCogenerationSourceChecksForModel(checks, &model, &observed)
	if err != nil {
		t.Fatal(err)
	}
	bundle := epathSQLPVConsumerDecode(t, epathSQLPVCogenerationConsumerWire(literals, true))
	count := 0
	for _, row := range checks.Rows {
		if !epathSQLPVCogenerationHasSourceCheck(row) {
			continue
		}
		count++
		if row.Want.Value == nil || *row.Want.Value != 0 || row.Quantity == nil || row.OptionalPresentation {
			t.Fatal("measured zero lost a required numeric scalar")
		}
		if err := epathSQLCheckPVCogenerationSourceConsumer(bundle, row, prepared); err != nil {
			t.Fatal(err)
		}
	}
	if count != 4 || len(checks.Rows) != 84 || len(checks.Keys) != 84 {
		t.Fatal("measured zero lost the separate required four")
	}
	for _, frequency := range []string{"Monthly", "Hourly"} {
		native := frames.PVCogeneration.Parents[frequency]
		if native.NativeAnnualKWh != 0 || native.Source.RawSum == nil || *native.Source.RawSum != 0 || native.Source.EnergyKWh == nil || *native.Source.EnergyKWh != 0 {
			t.Fatal("native measured zero became missing or nonzero")
		}
	}
}
