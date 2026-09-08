package simulation

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type epathOraclePendingUnitInput struct {
	root, destination, candidate string
	evidence                     epathRealRunEvidence
	recipe                       epathRealOracleRecipe
	observed                     epathRealOracleEvidence
	checks                       epathSQLModelChecks
	failures                     []epathSQLModelFailure
}

func (input epathOraclePendingUnitInput) write() error {
	return epathWriteOraclePending(input.root, input.destination, input.candidate, input.evidence, input.recipe, input.observed, input.checks, input.failures)
}

func epathOraclePendingUnitFixture(t *testing.T) epathOraclePendingUnitInput {
	t.Helper()
	// These are temporary identity/IO fixtures, not EnergyPlus observations or
	// approved expected data. Full SQL/graph proofs have their own regressions.
	input := epathOraclePendingUnitInput{root: t.TempDir()}
	write := func(path, content string) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		hash, err := epathOracleHashFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return hash
	}
	write(filepath.Join(input.root, "production.go"), "package fixture\n")
	input.destination = filepath.Join(input.root, ".runtime", "review", "pending.json")
	input.candidate = filepath.Join(input.root, ".runtime", "candidate.json")
	e := &input.evidence
	e.Version, e.Fixture.ID, e.Fixture.Version = "25.1", "unit-only", "25.1"
	e.CatalogDirectory = filepath.Join(input.root, "catalog")
	e.Fixture.OraclePath = "oracles/unit.json"
	e.OriginalInputPath = filepath.Join(e.CatalogDirectory, "model.idf")
	e.ModelSHA256 = write(e.OriginalInputPath, "original model fixture")
	e.Fixture.ModelSHA256 = e.ModelSHA256
	e.RunDirectory = filepath.Join(input.root, ".runtime", "capture")
	write(filepath.Join(e.RunDirectory, "run-evidence.json"), "immutable capture identity fixture")
	e.InputPath = filepath.Join(e.RunDirectory, "executed-model.idf")
	e.ExecutedSHA256 = write(e.InputPath, "executed input fixture")
	e.SQLPath = filepath.Join(e.RunDirectory, "eplusout.sql")
	e.SQLSHA256 = write(e.SQLPath, "immutable SQL identity fixture")
	e.EngineSHA256, e.WeatherSHA256 = strings.Repeat("a", 64), strings.Repeat("b", 64)
	e.Fixture.Weather.SHA256 = e.WeatherSHA256
	e.Run = &SimulationRunResult{Status: "succeeded", PurposeRunPlan: &PurposeRunPlan{}}
	input.recipe = epathRealOracleRecipe{Schema: "semantic-idf.energy-path-sql-oracle-recipe/v1", Review: "unit-only independently declared quantities", SQLModel: &epathRealSQLModel{}}
	recipeBytes, err := json.Marshal(input.recipe)
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(e.CatalogDirectory, e.Fixture.OraclePath), string(recipeBytes))
	// Deliberately unrelated content: exporting candidate scalars here would
	// corrupt the independently supplied metrics and fail the exact assertions.
	candidateHash := write(input.candidate, `{"candidateScalarNeverUsedAsExpected":999999}`)
	provenance, err := epathOracleSnapshotProvenanceFor(input.root, *e)
	if err != nil {
		t.Fatal(err)
	}
	provenance.CandidateSHA256 = candidateHash
	provenanceBytes, err := json.Marshal(provenance)
	if err != nil {
		t.Fatal(err)
	}
	write(input.candidate+".provenance.json", string(provenanceBytes))
	input.checks.RequireCoverage = true
	input.observed.sqlPath, input.observed.outputPlan = e.SQLPath, e.Run.PurposeRunPlan
	input.observed.CheckedGroups = append([]string(nil), epathRealOracleGroups...)
	input.observed.modelCoverage = &epathSQLModelCoverageReport{Records: []epathSQLModelCoverageRecord{{Scope: "building", Period: "annual", Collection: "nodes", ID: "unit-only", RequiredFields: []string{"value"}}}}
	for _, group := range epathRealOracleGroups {
		q, target, unit, status := &epathSQLQuantity{Value: 1.234567890123}, epathSQLNodeTarget("load", "", "cooling", "thermal"), "kWh", ""
		var found, total *int
		if group == "endUses" {
			q = &epathSQLQuantity{}
		}
		if group == "completeness" || group == "ratios" {
			q, unit, status = nil, "count", "unavailable"
			zero := 0
			found, total = &zero, &zero
			field := "drivers"
			if group == "ratios" {
				field = "ratios"
			}
			target = epathRealOracleTarget{Collection: "quality", Field: field}
		}
		if err := input.checks.add(group, "building", "", "annual", "literal", unit, q, target, status, found, total); err != nil {
			t.Fatal(err)
		}
		check := &input.checks.Rows[len(input.checks.Rows)-1]
		metric := check.Want
		if group == "ratios" {
			check.Quality = &epathSQLQualityProof{Field: "ratios"}
			one := 1
			metric.Found, metric.Total, metric.Value, metric.Status = &one, &one, epathOracleNumber(1), "complete"
		}
		input.observed.Metrics = append(input.observed.Metrics, metric)
	}
	return input
}

func TestEnergyPathRealOraclePendingExportsIndependentMetricsOnly(t *testing.T) {
	input := epathOraclePendingUnitFixture(t)
	before, _ := json.Marshal(input.observed)
	if err := input.write(); err != nil {
		t.Fatal(err)
	}
	var pending epathOraclePendingMetrics
	if err := epathDecodeOracleFile(input.destination, &pending); err != nil {
		t.Fatal(err)
	}
	if pending.Schema != "semantic-idf.energy-path-oracle-pending/v1" || pending.Approved || pending.Acceptance || pending.Provenance.Acceptance || pending.ReviewStatus != "pending_independent_review" {
		t.Fatal("pending output claimed approval or used expected-manifest schema")
	}
	if pending.FixtureID != input.evidence.Fixture.ID || pending.Version != input.evidence.Version || pending.ModelSHA256 != input.evidence.ModelSHA256 || pending.Provenance.SQLSHA256 != input.evidence.SQLSHA256 || pending.Provenance.ExecutedSHA256 != input.evidence.ExecutedSHA256 || pending.Provenance.EngineSHA256 != input.evidence.EngineSHA256 || pending.Provenance.WeatherSHA256 != input.evidence.WeatherSHA256 || len(pending.RequiredKeysSHA256) != 64 || len(pending.RecipeSHA256) != 64 || len(pending.Provenance.ProductionSHA256) != 64 || len(pending.Provenance.CandidateSHA256) != 64 {
		t.Fatal("pending output lost original/candidate/recipe/production identities")
	}
	byKey := map[string]epathRealOracleMetric{}
	for _, metric := range input.observed.Metrics {
		byKey[metric.Key] = metric
	}
	last := ""
	for _, metric := range pending.Metrics {
		if metric.Key <= last || !reflect.DeepEqual(metric, byKey[metric.Key]) {
			t.Fatalf("independent precision/null/zero/status/count/order changed: %#v", metric)
		}
		last = metric.Key
	}
	if len(pending.Metrics) != len(input.checks.Keys) || !reflect.DeepEqual(pending.CheckedGroups, epathRealOracleGroups) {
		t.Fatal("required metric roster/groups changed")
	}
	after, _ := json.Marshal(input.observed)
	if string(before) != string(after) {
		t.Fatal("export mutated independent observations")
	}
	original, err := os.ReadFile(input.destination)
	if err != nil {
		t.Fatal(err)
	}
	if err := input.write(); err == nil {
		t.Fatal("second export overwrote pending review material")
	}
	unchanged, _ := os.ReadFile(input.destination)
	if string(original) != string(unchanged) {
		t.Fatal("exclusive-create failure modified existing bytes")
	}
}

func TestEnergyPathRealOraclePendingRejectsIncompleteOrChangedProofs(t *testing.T) {
	for name, mutate := range map[string]func(*epathOraclePendingUnitInput){
		"numeric failure":        func(i *epathOraclePendingUnitInput) { i.failures = []epathSQLModelFailure{{Message: "failure"}} },
		"coverage not mandatory": func(i *epathOraclePendingUnitInput) { i.checks.RequireCoverage = false },
		"missing coverage":       func(i *epathOraclePendingUnitInput) { i.observed.modelCoverage = nil },
		"empty coverage":         func(i *epathOraclePendingUnitInput) { i.observed.modelCoverage.Records = nil },
		"coverage failure": func(i *epathOraclePendingUnitInput) {
			i.observed.modelCoverage.Failures = []epathSQLModelFailure{{Message: "gap"}}
		},
		"missing checked group":   func(i *epathOraclePendingUnitInput) { i.observed.CheckedGroups = i.observed.CheckedGroups[1:] },
		"duplicate checked group": func(i *epathOraclePendingUnitInput) { i.observed.CheckedGroups[0] = i.observed.CheckedGroups[1] },
		"unknown checked group":   func(i *epathOraclePendingUnitInput) { i.observed.CheckedGroups[0] = "fake" },
		"deleted selector":        func(i *epathOraclePendingUnitInput) { i.checks.Rows = i.checks.Rows[1:] },
		"missing registry":        func(i *epathOraclePendingUnitInput) { delete(i.checks.Keys, i.checks.Rows[0].Want.Key) },
		"disabled registry":       func(i *epathOraclePendingUnitInput) { i.checks.Keys[i.checks.Rows[0].Want.Key] = false },
		"extra registry":          func(i *epathOraclePendingUnitInput) { i.checks.Keys["ghost"] = true },
		"duplicate selector":      func(i *epathOraclePendingUnitInput) { i.checks.Rows = append(i.checks.Rows, i.checks.Rows[0]) },
		"selector identity":       func(i *epathOraclePendingUnitInput) { i.checks.Rows[0].Item.Period = "M1" },
		"missing metric":          func(i *epathOraclePendingUnitInput) { i.observed.Metrics = i.observed.Metrics[1:] },
		"duplicate metric": func(i *epathOraclePendingUnitInput) {
			i.observed.Metrics = append(i.observed.Metrics, i.observed.Metrics[0])
		},
		"metric identity":       func(i *epathOraclePendingUnitInput) { i.observed.Metrics[0].Period = "M1" },
		"metric source changed": func(i *epathOraclePendingUnitInput) { i.observed.Metrics[0].Value = epathOracleNumber(999999) },
		"nonfinite metric":      func(i *epathOraclePendingUnitInput) { i.observed.Metrics[0].Value = epathOracleNumber(math.NaN()) },
		"lost quality counts": func(i *epathOraclePendingUnitInput) {
			for n := range i.observed.Metrics {
				if i.observed.Metrics[n].Group == "ratios" {
					i.observed.Metrics[n].Found, i.observed.Metrics[n].Total = nil, nil
				}
			}
		},
		"quality unknown status": func(i *epathOraclePendingUnitInput) {
			for n := range i.observed.Metrics {
				if i.observed.Metrics[n].Group == "ratios" {
					i.observed.Metrics[n].Status = "invented"
				}
			}
		},
		"already approved":        func(i *epathOraclePendingUnitInput) { i.observed.Acceptance = true },
		"no exact output plan":    func(i *epathOraclePendingUnitInput) { i.observed.outputPlan = nil },
		"different SQL context":   func(i *epathOraclePendingUnitInput) { i.observed.sqlPath = filepath.Join(i.root, "another.sql") },
		"unsuccessful run":        func(i *epathOraclePendingUnitInput) { i.evidence.Run.Status = "failed" },
		"no review":               func(i *epathOraclePendingUnitInput) { i.recipe.Review = "" },
		"unbound model":           func(i *epathOraclePendingUnitInput) { i.evidence.ModelSHA256 = strings.Repeat("c", 64) },
		"no explicit destination": func(i *epathOraclePendingUnitInput) { i.destination = "" },
		"approved expected destination": func(i *epathOraclePendingUnitInput) {
			i.destination = filepath.Join(i.evidence.CatalogDirectory, "expected", "unit.json")
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := epathOraclePendingUnitFixture(t)
			mutate(&input)
			if err := input.write(); err == nil {
				t.Fatal("unreviewable material was exported")
			}
			if input.destination != "" {
				if _, err := os.Stat(input.destination); !os.IsNotExist(err) {
					t.Fatalf("failure created destination: %v", err)
				}
			}
		})
	}
}

func TestEnergyPathRealOraclePendingRejectsStaleIdentityAndPreservesFiles(t *testing.T) {
	for _, name := range []string{"candidate", "production", "capture", "recipe", "SQL", "original input", "executed input", "sidecar"} {
		t.Run(name, func(t *testing.T) {
			input := epathOraclePendingUnitFixture(t)
			paths := map[string]string{"candidate": input.candidate, "production": filepath.Join(input.root, "production.go"), "capture": filepath.Join(input.evidence.RunDirectory, "run-evidence.json"), "recipe": filepath.Join(input.evidence.CatalogDirectory, input.evidence.Fixture.OraclePath), "SQL": input.evidence.SQLPath, "original input": input.evidence.OriginalInputPath, "executed input": input.evidence.InputPath, "sidecar": input.candidate + ".provenance.json"}
			path := paths[name]
			if err := os.WriteFile(path, []byte("changed identity fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := input.write(); err == nil {
				t.Fatal("stale provenance exported")
			}
			if _, err := os.Stat(input.destination); !os.IsNotExist(err) {
				t.Fatal("stale provenance created pending file")
			}
			bytes, err := os.ReadFile(path)
			if err != nil || string(bytes) != "changed identity fixture" {
				t.Fatal("export repaired/overwrote an input artifact")
			}
		})
	}
}
