package simulation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// This is deliberately not the approved expected-manifest schema. A passing
// candidate can produce review material, never approve or replace a golden.
type epathOraclePendingMetrics struct {
	Schema             string                        `json:"schema"`
	Approved           bool                          `json:"approved"`
	Acceptance         bool                          `json:"acceptance"`
	ReviewStatus       string                        `json:"reviewStatus"`
	FixtureID          string                        `json:"fixtureId"`
	Version            string                        `json:"version"`
	ModelSHA256        string                        `json:"modelSHA256"`
	RecipePath         string                        `json:"recipePath"`
	RecipeSHA256       string                        `json:"recipeSHA256"`
	CandidatePath      string                        `json:"candidatePath"`
	Provenance         epathOracleSnapshotProvenance `json:"provenance"`
	RequiredKeysSHA256 string                        `json:"requiredKeysSHA256"`
	CheckedGroups      []string                      `json:"checkedGroups"`
	Coverage           epathSQLModelCoverageReport   `json:"coverage"`
	Metrics            []epathRealOracleMetric       `json:"metrics"`
}

// The caller opts in with EPATH_REAL_ORACLE_PENDING_NEW after the normal saved
// candidate validation. No SQL/classifier is rerun and no candidate scalar is
// extracted here: only the independent evaluator's successful metric output is
// serialized, with the exact required-selector registry and current identities.
func epathWriteOraclePending(root, destination, candidatePath string, evidence epathRealRunEvidence, recipe epathRealOracleRecipe, observed epathRealOracleEvidence, checks epathSQLModelChecks, failures []epathSQLModelFailure) error {
	if len(failures) != 0 || !checks.RequireCoverage || observed.modelCoverage == nil || len(observed.modelCoverage.Failures) != 0 || len(observed.modelCoverage.Records) == 0 {
		return fmt.Errorf("pending review requires zero failures and a completed mandatory coverage ledger")
	}
	if observed.Acceptance || evidence.NumericalTrial != nil || recipe.SQLModel == nil || strings.TrimSpace(recipe.Review) == "" || !epathRealSamePath(observed.sqlPath, evidence.SQLPath) {
		return fmt.Errorf("pending review requires unapproved independent SQL-model evidence from this original run")
	}
	if evidence.Run == nil || evidence.Run.Status != "succeeded" || evidence.Run.ExitCode != 0 || evidence.Run.ERR.Severe != 0 || evidence.Run.ERR.Fatal != 0 || observed.outputPlan == nil || !reflect.DeepEqual(observed.outputPlan, evidence.Run.PurposeRunPlan) {
		return fmt.Errorf("pending review requires this successful run's exact executed output plan")
	}
	groups := map[string]bool{}
	for _, group := range observed.CheckedGroups {
		if !epathContainsOracleGroup(group) || groups[group] {
			return fmt.Errorf("invalid/duplicate checked group %q", group)
		}
		groups[group] = true
	}
	if len(groups) != len(epathRealOracleGroups) {
		return fmt.Errorf("all eight independently checked groups are required")
	}
	if err := epathValidateOracleMetricGroups(observed.Metrics); err != nil {
		return err
	}
	required := map[string]epathSQLModelCheck{}
	for _, check := range checks.Rows {
		key := check.Want.Key
		if key == "" || required[key].Want.Key != "" || !checks.Keys[key] || check.Item.Key != key || check.Item.Scope != check.Want.Scope || check.Item.Zone != check.Want.Zone || check.Item.Period != check.Want.Period || check.Item.Group != check.Want.Group || check.Item.Unit != check.Want.Unit {
			return fmt.Errorf("pending review has a missing/duplicate/contradictory required selector %q", key)
		}
		if err := epathValidateOracleMetricIdentity(check.Want); err != nil {
			return err
		}
		targetError := epathValidateOracleTarget(check.Item.Target, check.Item.Unit)
		if check.NativeVRFSource != nil {
			// The exported registry retains allocatedValue. Its knownness is
			// justified only by the same complete typed proof as field coverage.
			targetError = epathSQLVRFSourceTarget(check)
		}
		if err := targetError; err != nil {
			return err
		}
		required[key] = check
	}
	for key, enabled := range checks.Keys {
		if !enabled || required[key].Want.Key == "" {
			return fmt.Errorf("required selector registry differs from compiled rows: %s", key)
		}
	}
	if len(required) != len(observed.Metrics) {
		return fmt.Errorf("independent metrics do not contain the exact required selector set")
	}
	for _, metric := range observed.Metrics {
		check, found := required[metric.Key]
		want := check.Want
		if !found || metric.Group != want.Group || metric.Scope != want.Scope || metric.Zone != want.Zone || metric.Period != want.Period || metric.Unit != want.Unit {
			return fmt.Errorf("independent metric identity differs from required selector %s", metric.Key)
		}
		// Quality metrics are derived only after SQL/graph prerequisites pass;
		// other metrics must be the independently compiled expectations exactly.
		if check.Quality == nil && !reflect.DeepEqual(metric, want) {
			return fmt.Errorf("non-quality metric changed after independent compilation: %s", metric.Key)
		}
		if check.Quality != nil && (check.Quality.Field != check.Item.Target.Field || metric.Status == "" || check.Item.Target.Collection != "quality") {
			return fmt.Errorf("derived quality metric lacks its exact proof/status")
		}
		if check.Quality != nil {
			accounting := epathSQLQualityAccounting(check.Quality.Field)
			if accounting && (metric.Unit != "%" || metric.Found != nil || metric.Total != nil || !strings.Contains(" complete partial overmapped unavailable not_requested not_applicable ", " "+metric.Status+" ")) || !accounting && (metric.Unit != "count" || metric.Found == nil || metric.Total == nil || !strings.Contains(" complete partial missing unavailable not_requested not_applicable ", " "+metric.Status+" ")) {
				return fmt.Errorf("derived quality status/count shape is invalid")
			}
		}
	}
	path, err := epathOraclePendingDestination(root, destination)
	if err != nil {
		return err
	}
	if evidence.Fixture.ID == "" || evidence.Version != evidence.Fixture.Version || evidence.ModelSHA256 != evidence.Fixture.ModelSHA256 || evidence.WeatherSHA256 != evidence.Fixture.Weather.SHA256 || strings.TrimSpace(evidence.Fixture.OraclePath) == "" {
		return fmt.Errorf("pending fixture/model/weather identity is not bound")
	}
	// Verify the recipe has not changed since it generated these observations.
	recipePath := filepath.Join(evidence.CatalogDirectory, filepath.FromSlash(evidence.Fixture.OraclePath))
	currentRecipe, err := epathLoadRealOracleRecipe(recipePath)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(currentRecipe, recipe) {
		return fmt.Errorf("reviewed recipe changed during evaluation")
	}
	recipeSHA, err := epathOracleHashFile(recipePath)
	if err != nil {
		return err
	}
	provenance, err := epathOracleSnapshotProvenanceFor(root, evidence)
	if err != nil {
		return err
	}
	provenance.CandidateSHA256, err = epathOracleHashFile(candidatePath)
	if err != nil {
		return err
	}
	var recorded epathOracleSnapshotProvenance
	if err := epathDecodeOracleFile(candidatePath+".provenance.json", &recorded); err != nil {
		return err
	}
	if provenance != recorded {
		return fmt.Errorf("pending metrics refer to a stale/unbound candidate snapshot")
	}
	for _, digest := range []string{evidence.ModelSHA256, provenance.SQLSHA256, provenance.ExecutedSHA256, provenance.EngineSHA256, provenance.WeatherSHA256} {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("missing/invalid original run provenance digest")
		}
	}
	// These paths already passed the saved-run validator; rereading hashes
	// catches an original input/SQL change between evaluation and export too.
	for original, want := range map[string]string{evidence.OriginalInputPath: evidence.ModelSHA256, evidence.InputPath: evidence.ExecutedSHA256, evidence.SQLPath: evidence.SQLSHA256} {
		got, err := epathOracleHashFile(original)
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("original evidence changed before pending export: %s", original)
		}
	}
	metrics := append([]epathRealOracleMetric(nil), observed.Metrics...)
	sort.Slice(metrics, func(i, j int) bool { return metrics[i].Key < metrics[j].Key })
	keys := make([]string, len(metrics))
	for index := range metrics {
		keys[index] = metrics[index].Key
	}
	keyHash := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	candidatePath, err = filepath.Abs(candidatePath)
	if err != nil {
		return err
	}
	out := epathOraclePendingMetrics{Schema: "semantic-idf.energy-path-oracle-pending/v1", ReviewStatus: "pending_independent_review", FixtureID: evidence.Fixture.ID, Version: evidence.Version, ModelSHA256: evidence.ModelSHA256, RecipePath: recipePath, RecipeSHA256: recipeSHA, CandidatePath: candidatePath, Provenance: provenance, RequiredKeysSHA256: hex.EncodeToString(keyHash[:]), CheckedGroups: append([]string(nil), epathRealOracleGroups...), Coverage: *observed.modelCoverage, Metrics: metrics}
	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func epathOraclePendingDestination(root, destination string) (string, error) {
	if strings.TrimSpace(destination) == "" {
		return "", fmt.Errorf("explicit EPATH_REAL_ORACLE_PENDING_NEW destination required")
	}
	path, err := epathOracleSnapshotDestination(root, destination)
	if err != nil {
		return "", err
	}
	// Do not let a directory junction turn a lexical .runtime path into a
	// write to the checked-in expected directory (or another outside folder).
	runtimePath, err := filepath.EvalSymlinks(filepath.Join(root, ".runtime"))
	if err != nil {
		return "", err
	}
	ancestor := filepath.Dir(path)
	for {
		_, err = os.Lstat(ancestor)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) || filepath.Dir(ancestor) == ancestor {
			return "", fmt.Errorf("cannot resolve pending destination ancestor: %w", err)
		}
		ancestor = filepath.Dir(ancestor)
	}
	resolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(runtimePath, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("resolved pending destination must remain inside .runtime")
	}
	return path, nil
}
