package simulation

import (
	"encoding/json"
	"fmt"
	"math"
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
		trace, temporal := frames.TraceSourceIdentities[id]
		if temporal {
			if err := epathSQLValidateTemporalTrace(trace); err != nil {
				return err
			}
			if trace.Source.DictionaryIndex != id || trace.Source.Name != source.Name || trace.Source.KeyValue != source.KeyValue || trace.Source.SourceUnit != source.SourceUnit || trace.Source.ReportingFrequency != source.ReportingFrequency {
				return fmt.Errorf("temporal trace differs from the original SQL observation")
			}
		}
		detail, nonAdditive := frames.LoadDetailIdentities[id]
		if nonAdditive {
			if err := epathSQLValidateLoadDetailIdentity(detail); err != nil {
				return err
			}
			if detail.Source.DictionaryIndex != id || detail.Source.Name != source.Name || detail.Source.KeyValue != source.KeyValue || detail.Source.SourceUnit != source.SourceUnit || detail.Source.ReportingFrequency != source.ReportingFrequency {
				return fmt.Errorf("non-additive load detail source differs from the observed SQL identity")
			}
			group = "loads"
		}
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
				if temporal {
					var err error
					q, err = epathSQLTemporalTraceAnnualQuantity(trace, field)
					if err != nil {
						return err
					}
					// Preserve every prior Monthly metric identity. Only these
					// explicitly declared additional observations gain frequency.
					key = "source/" + source.Name + "/" + source.KeyValue + "/Hourly/" + field
				}
				if err := checks.add(group, scope, zone, "annual", key, "kWh", &q, target, "", nil, nil); err != nil {
					return err
				}
				if nonAdditive {
					proof := detail
					checks.Rows[len(checks.Rows)-1].LoadDetail = &proof
				}
				if temporal {
					proof := trace
					checks.Rows[len(checks.Rows)-1].TraceSource = &proof
				}
			}
		}
	}
	for _, site := range model.Site {
		if site.Tabular == nil {
			continue
		}
		observation, ok := frames.SiteAnnual[site.ID]
		if !ok {
			return fmt.Errorf("annual source %s lacks its original observed cell", site.ID)
		}
		original, err := epathSQLOriginalTabular(site, observation)
		if err != nil {
			return err
		}
		q, err := epathSQLSitePeriod(frames, site.ID, "annual")
		if err != nil || q == nil {
			return fmt.Errorf("annual original source %s is unavailable: %v", site.ID, err)
		}
		group := "endUses"
		if site.Facility {
			group = "carriers"
		}
		for _, field := range []string{"rawValue", "effectiveValue"} {
			target := epathRealOracleTarget{Collection: "sources", Field: field, SourceName: original.Name, SourceKey: original.Key, SourceUnit: observation.Selector.Unit, Frequency: "Annual", Unit: "kWh"}
			key := "source/tabular/" + site.ID + "/" + field
			if err := checks.add(group, "building", "", "annual", key, "kWh", q, target, "", nil, nil); err != nil {
				return err
			}
			checks.Rows[len(checks.Rows)-1].OriginalSource = &original
		}
	}
	return nil
}

func epathCompileSQLModelChecks(observed epathRealOracleEvidence, model epathRealSQLModel) (epathSQLModelChecks, error) {
	checks := epathSQLModelChecks{RequireCoverage: true}
	if err := epathSQLValidateDirectHVACRequests(observed.outputPlan, model); err != nil {
		return checks, err
	}
	frames, err := epathCompileSQLModelFrames(observed.sqlPath, observed.Sources, model)
	if err != nil {
		return checks, err
	}
	for _, build := range []func() error{
		func() error { return epathSQLModelLoadDriverChecks(frames, model, &checks) },
		func() error { return epathSQLModelDriverLinkChecks(frames, model, &checks) },
		func() error { return epathSQLModelThermalReconciliationChecks(frames, model, &checks) },
		func() error { return epathSQLModelSourceChecks(frames, observed.Sources, model, &checks) },
		func() error { return epathSQLModelDirectHVACSourceChecks(frames, &checks) },
		func() error { return epathSQLModelSiteChecks(frames, model, &checks) },
		func() error { return epathSQLModelSiteFlowChecks(frames, model, &checks) },
		func() error { return epathSQLModelSiteResidualChecks(frames, model, &checks) },
		func() error {
			return epathSQLModelFanPoolChecks(observed, frames, model.FanPools, model.Precision, &checks)
		},
		func() error { return epathSQLModelServiceChecks(frames, model, &checks) },
		func() error { return epathSQLModelZoneServiceChecks(frames, model, &checks) },
		func() error { return epathSQLModelDirectUseChecks(observed.Sources, frames, model, &checks) },
		func() error { return epathSQLModelFanFlowChecks(observed, frames, model, &checks) },
		func() error { return epathSQLModelZoneCarrierChecks(frames, model, &checks) },
		func() error {
			return epathSQLModelQualityChecks(observed, frames, model, &checks)
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

func epathCheckSQLModelPresentation(bundle PurposeResultBundle, check epathSQLModelCheck, actual *float64) error {
	q, target := check.Quantity, check.Item.Target
	if q != nil && !q.valid() {
		return fmt.Errorf("invalid presentation interval configuration")
	}
	if check.OptionalPresentation && target.Collection != "nodes" {
		return fmt.Errorf("pruning policy cannot change source/unknown SQL presence")
	}
	if actual != nil && target.Collection == "nodes" && *actual < 0 {
		return fmt.Errorf("negative directional presentation value")
	}
	allowAbsent := check.OptionalPresentation || target.Collection == "nodes" && target.AllowPrunedZero
	if allowAbsent && q == nil {
		return fmt.Errorf("unknown SQL cannot authorize presentation pruning")
	}
	if actual == nil && allowAbsent && q != nil && q.includesZero() {
		nodes, _, _, _, err := epathOracleGraph(bundle, check.Item.Scope, check.Item.Zone, check.Item.Period)
		if err != nil {
			return err
		}
		for _, node := range nodes {
			if epathOracleNodeMatches(node, target) {
				return fmt.Errorf("present node has missing numeric field; this is not presentation pruning")
			}
		}
		return nil
	}
	return epathCheckSQLModelQuantity(actual, q)
}

func epathCheckSQLModelConversion(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	proof := check.Conversion
	if proof == nil || !proof.From.valid() || !proof.To.valid() || proof.From.Value < 0 || proof.To.Value < 0 || check.Quantity != nil && !check.Quantity.valid() {
		return fmt.Errorf("invalid independently observed paired interval")
	}
	if err := epathValidateOracleTarget(check.Item.Target, "ratio"); err != nil {
		return err
	}
	nodes, links, _, _, err := epathOracleGraph(bundle, check.Item.Scope, check.Item.Zone, check.Item.Period)
	if err != nil {
		return err
	}
	actual, err := epathReadOraclePairedQuantities(nodes, links, bundle.EnergyExplanation.Sources, check.Item.Target)
	if err != nil {
		return err
	}
	if actual.Count == 0 {
		if proof.ExactPresentation != nil {
			return fmt.Errorf("missing conversion with independently proved positive displayed endpoints")
		}
		if proof.From.includesZero() || proof.To.includesZero() {
			return nil
		}
		return fmt.Errorf("missing conversion with two required non-prunable endpoints")
	}
	if actual.From == nil || actual.To == nil || *actual.From < 0 || *actual.To < 0 {
		return fmt.Errorf("missing/negative paired endpoint")
	}
	if err := epathCheckSQLModelQuantity(actual.From, &proof.From); err != nil {
		return fmt.Errorf("conversion thermal endpoint: %w", err)
	}
	if err := epathCheckSQLModelQuantity(actual.To, &proof.To); err != nil {
		return fmt.Errorf("conversion site endpoint: %w", err)
	}
	if exact := proof.ExactPresentation; exact != nil {
		if check.Item.Scope != "zone" || check.Item.Period == "annual" || check.Item.Target.Basis != "direct_zone_energy" || check.Item.Target.Service != "heating" || exact.ExactPresentation != nil || exact.From.Error != 0 || exact.To.Error != 0 || exact.From.Bounds != nil || exact.To.Bounds != nil || exact.From.Value <= 0 || exact.To.Value <= 0 {
			return fmt.Errorf("invalid exact independently quantized direct pair")
		}
		kind, err := epathSQLConversionRatioKind(check.Item.Target.RatioKind, exact.From, exact.To)
		if err != nil || kind != check.Item.Target.RatioKind {
			return fmt.Errorf("exact direct displayed pair contradicts its independent combustion classification")
		}
		if err := epathCheckSQLModelQuantity(actual.From, &exact.From); err != nil {
			return fmt.Errorf("exact direct thermal presentation: %w", err)
		}
		if err := epathCheckSQLModelQuantity(actual.To, &exact.To); err != nil {
			return fmt.Errorf("exact direct site presentation: %w", err)
		}
	}
	if proof.DirectHVACSources != nil {
		if err := epathCheckSQLDirectHVACBuildingSources(bundle, check); err != nil {
			return err
		}
	}
	if *actual.From > 0 && *actual.To > 0 && actual.Ratio == nil {
		return fmt.Errorf("positive paired quantities lost ratio availability")
	}
	if (*actual.From == 0 || *actual.To == 0) && actual.Ratio != nil {
		return fmt.Errorf("zero presented endpoint has a fabricated ratio")
	}
	return nil
}

func epathCheckSQLModelAllocation(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	target, proof := check.Item.Target, check.Allocation
	if proof == nil || target.Collection != "reconciliation" || target.Level != "allocation" || target.ID == "" || target.Unit != "kWh" || check.Item.Group != "zoneAllocation" {
		return fmt.Errorf("whole allocation pruning requires exact allocation identity")
	}
	if err := epathValidateOracleTarget(target, check.Item.Unit); err != nil {
		return err
	}
	canPrune := true
	for field, q := range proof.fields() {
		if q == nil || !q.valid() || q.Value < 0 {
			return fmt.Errorf("unknown/invalid observed allocation %s", field)
		}
		low, _ := q.bounds()
		if low < 0 {
			return fmt.Errorf("allocation interval must respect nonnegative presentation")
		}
		canPrune = canPrune && q.includesZero()
	}
	q, ok := proof.fields()[target.Field]
	if !ok || check.Quantity == nil || !check.Quantity.valid() {
		return fmt.Errorf("missing selected allocation quantity")
	}
	ql, qh := q.bounds()
	cl, ch := check.Quantity.bounds()
	if q.Value != check.Quantity.Value || ql != cl || qh != ch {
		return fmt.Errorf("selected quantity contradicts whole allocation proof")
	}
	accounted := proof.Direct.Value + proof.Allocated.Value + proof.Unassigned.Value
	if math.Abs(proof.Expected.Value-accounted) > 1e-10*math.Max(1, proof.Expected.Value) {
		return fmt.Errorf("independent allocation quantities do not conserve expected value")
	}
	_, _, rows, _, err := epathOracleGraph(bundle, check.Item.Scope, check.Item.Zone, check.Item.Period)
	if err != nil {
		return err
	}
	var selected *EnergyReconciliation
	for index := range rows {
		if rows[index].ID == target.ID {
			selected = &rows[index]
			break
		}
	}
	if selected == nil {
		if canPrune {
			return nil
		}
		return fmt.Errorf("missing allocation row with at least one required non-prunable quantity")
	}
	// An exact ID with wrong metadata is present and invalid, not absent.
	actual, err := epathReadOracleCandidate(bundle, check.Item, check.Want)
	if err != nil {
		return err
	}
	if actual == nil {
		return fmt.Errorf("present allocation row does not match required context")
	}
	values := map[string]float64{"expectedValue": selected.ExpectedValue, "directValue": selected.DirectValue, "allocatedValue": selected.AllocatedValue, "unassignedValue": selected.UnassignedValue}
	for field, number := range values {
		if !epathOracleFinite(number) || number < 0 {
			return fmt.Errorf("invalid present allocation %s", field)
		}
		if err := epathCheckSQLModelQuantity(epathOracleNumber(number), proof.fields()[field]); err != nil {
			return fmt.Errorf("present allocation %s: %w", field, err)
		}
	}
	return nil
}

func epathEvaluateSQLModelChecks(out *epathRealOracleEvidence, bundle PurposeResultBundle, checks epathSQLModelChecks) []epathSQLModelFailure {
	out.CheckedGroups = nil
	out.modelCoverage = nil
	failures := []epathSQLModelFailure{}
	failedGroups := map[string]bool{}
	for _, check := range checks.Rows {
		var err error
		metric := check.Want
		if check.Quality != nil {
			var derived epathRealOracleMetric
			derived, err = epathCheckSQLModelQuality(bundle, check)
			if err == nil {
				metric = derived
			}
		} else if check.SiteFlow != nil {
			err = epathCheckSQLSiteFlow(bundle, check)
		} else if check.SiteResidual != nil {
			err = epathCheckSQLSiteResidual(bundle, check)
		} else if check.Reconciliation != nil {
			err = epathCheckSQLModelReconciliation(bundle, check)
		} else if check.DriverLink != nil {
			err = epathCheckSQLModelDriverLink(bundle, check)
		} else if check.Conversion != nil {
			err = epathCheckSQLModelConversion(bundle, check)
		} else if check.Allocation != nil {
			err = epathCheckSQLModelAllocation(bundle, check)
		} else if check.DirectHVACSource != nil {
			err = epathCheckSQLDirectHVACSource(bundle, check)
		} else {
			var actual *float64
			actual, err = epathReadOracleCandidate(bundle, check.Item, check.Want)
			if err == nil {
				err = epathCheckSQLModelPresentation(bundle, check, actual)
			}
		}
		if err == nil && check.ZoneService != nil {
			err = epathCheckSQLZoneServiceEndpoints(bundle, check)
		}
		if err == nil && check.DirectUse != nil {
			err = epathCheckSQLDirectUseEndpoints(bundle, check)
		}
		if err == nil && check.ZoneCarrier != nil {
			err = epathCheckSQLZoneCarrier(bundle, check)
		}
		if err == nil && check.OriginalSource != nil {
			err = epathCheckSQLModelOriginalSource(bundle, check)
		}
		if err == nil && check.LoadDetail != nil {
			err = epathCheckSQLLoadDetailSource(bundle, check)
		}
		if err == nil && check.TraceSource != nil {
			err = epathCheckSQLTemporalTraceSource(bundle, check)
		}
		if err == nil && check.AnnualServiceAbsent {
			err = epathCheckSQLAnnualServiceAbsent(bundle, check)
		}
		out.Metrics = append(out.Metrics, metric)
		if err != nil {
			failedGroups[check.Want.Group] = true
			failures = append(failures, epathSQLModelFailure{check.Want.Group, check.Want.Key, check.Want.Key + ": " + err.Error()})
		}
	}
	if checks.RequireCoverage {
		coverage := epathSQLModelCoverage(bundle, checks)
		out.modelCoverage = &coverage
		for _, failure := range coverage.Failures {
			failures = append(failures, failure)
			failedGroups[failure.Group] = true
		}
	}
	for _, group := range epathRealOracleGroups {
		if !failedGroups["coverage"] && !failedGroups[group] {
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
	pendingPath := strings.TrimSpace(os.Getenv("EPATH_REAL_ORACLE_PENDING_NEW"))
	if pendingPath != "" && (directory == "" || path == "") {
		t.Fatal("pending review requires explicit saved capture and SHA-bound candidate")
	}
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
	if err := epathValidateRealSQLDirectHVACOriginal(evidence, recipe); err != nil {
		t.Fatal(err)
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
	coverageFailures := []epathSQLModelFailure{}
	if observed.modelCoverage != nil {
		coverageFailures = observed.modelCoverage.Failures
	}
	numericFailures := failures[:len(failures)-len(coverageFailures)]
	if reportPath := os.Getenv("EPATH_REAL_ORACLE_DIAGNOSTIC_NEW"); reportPath != "" {
		absolute, err := epathOracleSnapshotDestination(root, reportPath)
		if err != nil {
			t.Fatal(err)
		}
		candidateSHA, err := epathOracleHashFile(path)
		if err != nil {
			t.Fatal(err)
		}
		recipeSHA, err := epathOracleHashFile(filepath.Join(catalog, filepath.FromSlash(evidence.Fixture.OraclePath)))
		if err != nil {
			t.Fatal(err)
		}
		report := struct {
			Schema          string                       `json:"schema"`
			Acceptance      bool                         `json:"acceptance"`
			CandidatePath   string                       `json:"candidatePath"`
			CandidateSHA256 string                       `json:"candidateSHA256"`
			RecipeSHA256    string                       `json:"recipeSHA256"`
			SQLSHA256       string                       `json:"sqlSHA256"`
			Checks          int                          `json:"checks"`
			Failures        []epathSQLModelFailure       `json:"failures"`
			Coverage        *epathSQLModelCoverageReport `json:"coverage,omitempty"`
			NumericFailures []epathSQLModelFailure       `json:"numericAndContractFailures"`
		}{"semantic-idf.energy-path-sql-diagnostic/v1", false, path, candidateSHA, recipeSHA, evidence.SQLSHA256, len(checks.Rows), failures, observed.modelCoverage, numericFailures}
		file, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal(err)
		}
		err = json.NewEncoder(file).Encode(report)
		closeErr := file.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		t.Logf("NEW DIAGNOSTIC ONLY, NOT EXPECTED/ACCEPTANCE: %s", absolute)
	}
	encoded, err := json.Marshal(observed.Metrics)
	if err != nil {
		t.Fatal(err)
	}
	counts, failed, uncovered := map[string]int{}, map[string]int{}, map[string]int{}
	for _, check := range checks.Rows {
		counts[check.Want.Group]++
	}
	for _, failure := range numericFailures {
		failed[failure.Group]++
		if failed[failure.Group] <= 12 {
			t.Log(failure.Message)
		}
	}
	for _, failure := range coverageFailures {
		uncovered[failure.Group]++
		if uncovered[failure.Group] <= 3 {
			t.Logf("COVERAGE GAP: %s", failure.Message)
		}
	}
	for _, group := range epathRealOracleGroups {
		t.Logf("%s: %d candidate-bound checks; %d numeric/contract failures; %d coverage gaps", group, counts[group], failed[group], uncovered[group])
	}
	t.Logf("NOT ACCEPTANCE: %d independent SQL metrics, %d JSON bytes; no expected or source capture writes", len(checks.Rows), len(encoded))
	if err := epathValidateSavedRealEvidence(root, catalog, evidence); err != nil {
		t.Fatal(err)
	}
	if len(failures) > 0 {
		t.Fatalf("independent SQL comparison rejected candidate: %d mismatches", len(failures))
	}
	if pendingPath != "" {
		if err := epathWriteOraclePending(root, pendingPath, path, evidence, recipe, observed, checks, failures); err != nil {
			t.Fatal(err)
		}
		t.Logf("NEW PENDING REVIEW ONLY, NOT APPROVED EXPECTED/ACCEPTANCE: %s", pendingPath)
	}
}
