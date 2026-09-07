package simulation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Every compiled row is a candidate-bound assertion. Source rows separately
// prove all-month signed observations, including months pruned from main flow.
func epathSQLModelSourceChecks(frames epathSQLFrames, observed []epathRealSQLSource, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	byID := map[int]epathRealSQLSource{}
	for _, source := range observed {
		byID[source.DictionaryIndex] = source
	}
	ids := []int{}
	for id := range frames.SourceRaw {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		source, ok := byID[id]
		if !ok || len(frames.SourceRaw[id]) != 12 || len(frames.SourceEffective[id]) != 12 {
			return fmt.Errorf("incomplete compiled source proof %d", id)
		}
		group := "drivers"
		for _, load := range model.Loads {
			for _, alternative := range load.Source.Alternatives {
				if strings.EqualFold(alternative.Name, source.Name) {
					group = "loads"
				}
			}
		}
		if source.IsMeter {
			group = "endUses"
			for _, site := range model.Site {
				if site.Facility {
					for _, alternative := range site.Source.Alternatives {
						if strings.EqualFold(alternative.Name, source.Name) {
							group = "carriers"
						}
					}
				}
			}
		}
		contexts := []string{""}
		if key := frames.SourceZone[id]; key != "" {
			contexts = append(contexts, frames.Zones[key].Name)
		}
		for _, zone := range contexts {
			scope := "building"
			if zone != "" {
				scope = "zone"
			}
			for _, field := range []string{"rawValue", "effectiveValue"} {
				values := frames.SourceRaw[id]
				if field == "effectiveValue" {
					values = frames.SourceEffective[id]
				}
				q := epathSQLQuantity{}
				for _, value := range values {
					q = q.add(value)
				}
				target := epathRealOracleTarget{Collection: "sources", Field: field, SourceName: source.Name, SourceKey: source.KeyValue, SourceUnit: source.SourceUnit, Frequency: source.ReportingFrequency, Unit: "kWh"}
				key := "source/" + source.Name + "/" + source.KeyValue + "/" + field
				if err := checks.add(group, scope, zone, "annual", key, "kWh", &q, target, "", nil, nil); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func epathCompileSQLModelChecks(observed epathRealOracleEvidence, model epathRealSQLModel) (epathSQLModelChecks, error) {
	var checks epathSQLModelChecks
	frames, err := epathCompileSQLModelFrames(observed.sqlPath, observed.Sources, model)
	if err != nil {
		return checks, err
	}
	for _, build := range []func() error{
		func() error { return epathSQLModelLoadDriverChecks(frames, model, &checks) },
		func() error { return epathSQLModelSourceChecks(frames, observed.Sources, model, &checks) },
		func() error { return epathSQLModelSiteChecks(frames, model, &checks) },
		func() error { return epathSQLModelServiceChecks(frames, model, &checks) },
		func() error {
			return epathSQLModelAvailabilityChecks(observed.Sources, observed.outputPlan, model, &checks)
		},
	} {
		if err := build(); err != nil {
			return checks, err
		}
	}
	sort.Slice(checks.Rows, func(i, j int) bool { return checks.Rows[i].Want.Key < checks.Rows[j].Want.Key })
	metrics := make([]epathRealOracleMetric, 0, len(checks.Rows))
	for _, check := range checks.Rows {
		metrics = append(metrics, check.Want)
	}
	if err := epathValidateOracleMetricGroups(metrics); err != nil {
		return checks, err
	}
	return checks, nil
}

type epathSQLModelFailure struct{ Group, Key, Message string }

func epathEvaluateSQLModelChecks(out *epathRealOracleEvidence, bundle PurposeResultBundle, checks epathSQLModelChecks) []epathSQLModelFailure {
	failures := append([]epathSQLModelFailure{}, checks.Unresolved...)
	failedGroups := map[string]bool{}
	for _, failure := range failures {
		failedGroups[failure.Group] = true
	}
	for _, check := range checks.Rows {
		actual, err := epathReadOracleCandidate(bundle, check.Item, check.Want)
		if err == nil {
			err = epathCheckSQLModelQuantity(actual, check.Quantity)
		}
		out.Metrics = append(out.Metrics, check.Want)
		if err != nil {
			failedGroups[check.Want.Group] = true
			failures = append(failures, epathSQLModelFailure{check.Want.Group, check.Want.Key, check.Want.Key + ": " + err.Error()})
		}
	}
	for _, group := range epathRealOracleGroups {
		if !failedGroups[group] {
			out.CheckedGroups = append(out.CheckedGroups, group)
		}
	}
	return failures
}

// Opt-in diagnostic only. A mismatch FAILS even though all groups continue, so
// the first bad row cannot prevent inspection of the other physical stages.
// No engine, candidate rebuild, capture mutation, or expected-file write occurs.
func TestEnergyPathRealSQLModelSavedCandidate(t *testing.T) {
	directory, path := os.Getenv("EPATH_REAL_ORACLE_CAPTURE_DIR"), os.Getenv("EPATH_REAL_ORACLE_SNAPSHOT")
	if directory == "" && path == "" {
		t.Skip("explicit saved capture and SHA-bound candidate required; not acceptance")
	}
	if directory == "" || path == "" || os.Getenv("EPATH_REAL_RUN") == "1" || os.Getenv("EPATH_REAL_CAPTURE") == "1" {
		t.Fatal("read-only diagnostic requires both paths and cannot run engine")
	}
	root, catalog := epathRealDirectories(t)
	var evidence epathRealRunEvidence
	file, err := os.Open(filepath.Join(directory, "run-evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = json.NewDecoder(file).Decode(&evidence)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !epathRealSamePath(directory, evidence.RunDirectory) {
		t.Fatal("wrong saved capture directory")
	}
	if err := epathValidateSavedRealEvidence(root, catalog, evidence); err != nil {
		t.Fatal(err)
	}
	bundle, err := epathReadOracleSnapshot(root, path, evidence)
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := epathLoadRealOracleRecipe(filepath.Join(catalog, filepath.FromSlash(evidence.Fixture.OraclePath)))
	if err != nil {
		t.Fatal(err)
	}
	if recipe.SQLModel == nil || evidence.Run == nil {
		t.Fatal("reviewed SQL model and actual executed plan required")
	}
	observed, err := epathReadRealSQLOracle(evidence.SQLPath)
	if err != nil {
		t.Fatal(err)
	}
	observed.outputPlan = evidence.Run.PurposeRunPlan
	checks, err := epathCompileSQLModelChecks(observed, *recipe.SQLModel)
	if err != nil {
		t.Fatal(err)
	}
	failures := epathEvaluateSQLModelChecks(&observed, bundle, checks)
	encoded, err := json.Marshal(observed.Metrics)
	if err != nil {
		t.Fatal(err)
	}
	counts, failed := map[string]int{}, map[string]int{}
	for _, check := range checks.Rows {
		counts[check.Want.Group]++
	}
	for _, failure := range failures {
		failed[failure.Group]++
		if failed[failure.Group] <= 12 {
			t.Log(failure.Message)
		}
	}
	for _, group := range epathRealOracleGroups {
		t.Logf("%s: %d candidate-bound checks; %d mismatches", group, counts[group], failed[group])
	}
	t.Logf("NOT ACCEPTANCE: %d independent SQL metrics, %d JSON bytes; no expected or source capture writes", len(checks.Rows), len(encoded))
	if err := epathValidateSavedRealEvidence(root, catalog, evidence); err != nil {
		t.Fatal(err)
	}
	if len(failures) > 0 {
		t.Fatalf("independent SQL comparison rejected candidate: %d mismatches", len(failures))
	}
}
