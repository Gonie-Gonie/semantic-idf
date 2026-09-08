package simulation

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var epathRealOracleGroups = []string{"drivers", "loads", "endUses", "carriers", "ratios", "completeness", "residuals", "zoneAllocation"}

type epathRealOracleMetric struct {
	Key    string   `json:"key"`
	Group  string   `json:"group"`
	Scope  string   `json:"scope"`
	Zone   string   `json:"zone,omitempty"`
	Period string   `json:"period"`
	Unit   string   `json:"unit"`
	Value  *float64 `json:"value"`
	Status string   `json:"status,omitempty"`
	Found  *int     `json:"found,omitempty"`
	Total  *int     `json:"total,omitempty"`
}

type epathRealOracleSourceRecipe struct {
	ID           string   `json:"id"`
	Names        []string `json:"names"`
	Keys         []string `json:"keys"`
	IsMeter      bool     `json:"isMeter"`
	Frequency    string   `json:"frequency"`
	SourceUnit   string   `json:"sourceUnit"`
	Multiplier   *float64 `json:"multiplier,omitempty"`
	Sign         *float64 `json:"sign,omitempty"`
	AllowMissing bool     `json:"allowMissing,omitempty"`
}

// The recipe is reviewed independently of the output candidate. Arithmetic
// names describe ordinary operations on raw SQL observations, not production
// graph rules. Constants cannot stand in for observed values.
type epathRealOracleMetricRecipe struct {
	Key               string                `json:"key"`
	Group             string                `json:"group"`
	Scope             string                `json:"scope"`
	Zone              string                `json:"zone,omitempty"`
	Period            string                `json:"period"`
	Unit              string                `json:"unit"`
	Operation         string                `json:"operation"`
	Inputs            []string              `json:"inputs"`
	Status            string                `json:"status,omitempty"`
	Target            epathRealOracleTarget `json:"target"`
	AbsoluteTolerance float64               `json:"absoluteTolerance,omitempty"`
	RelativeTolerance float64               `json:"relativeTolerance,omitempty"`
}

type epathRealOracleTarget struct {
	ID               string `json:"id,omitempty"`
	Collection       string `json:"collection"`
	Field            string `json:"field"`
	Level            string `json:"level,omitempty"`
	Category         string `json:"category,omitempty"`
	Service          string `json:"service,omitempty"`
	Carrier          string `json:"carrier,omitempty"`
	Basis            string `json:"basis,omitempty"`
	Relation         string `json:"relation,omitempty"`
	Kind             string `json:"kind,omitempty"`
	Unit             string `json:"unit,omitempty"`
	ScaleDomain      string `json:"scaleDomain,omitempty"`
	AggregationBasis string `json:"aggregationBasis,omitempty"`
	ThermalComponent string `json:"thermalComponent,omitempty"`
	Component        string `json:"component,omitempty"`
	SourceName       string `json:"sourceName,omitempty"`
	SourceKey        string `json:"sourceKey,omitempty"`
	SourceUnit       string `json:"sourceUnit,omitempty"`
	Frequency        string `json:"frequency,omitempty"`
	FromID           string `json:"fromId,omitempty"`
	ToID             string `json:"toId,omitempty"`
	FromUnit         string `json:"fromUnit,omitempty"`
	ToUnit           string `json:"toUnit,omitempty"`
	RatioKind        string `json:"ratioKind,omitempty"`
	AllocationMethod string `json:"allocationMethod,omitempty"`
	Status           string `json:"status,omitempty"`
	Aggregate        string `json:"aggregate,omitempty"`
	AllowPrunedZero  bool   `json:"allowPrunedZero,omitempty"`
}

type epathRealOracleRecipe struct {
	Schema   string                        `json:"schema"`
	Review   string                        `json:"review"`
	Sources  []epathRealOracleSourceRecipe `json:"sources"`
	Metrics  []epathRealOracleMetricRecipe `json:"metrics"`
	SQLModel *epathRealSQLModel            `json:"sqlModel,omitempty"`
}

type epathRealExpectedManifest struct {
	Schema        string                          `json:"schema"`
	FixtureID     string                          `json:"fixtureId"`
	Version       string                          `json:"version"`
	ModelSHA256   string                          `json:"modelSHA256"`
	WeatherSHA256 string                          `json:"weatherSHA256"`
	Review        string                          `json:"review"`
	Metrics       []epathRealOracleMetric         `json:"metrics,omitempty"`
	MetricPayload *epathRealExpectedMetricPayload `json:"metricPayload,omitempty"`
}

func epathLoadRealOracleRecipe(path string) (epathRealOracleRecipe, error) {
	var recipe epathRealOracleRecipe
	if err := epathDecodeOracleFile(path, &recipe); err != nil {
		return recipe, err
	}
	if recipe.Schema != "semantic-idf.energy-path-sql-oracle-recipe/v1" || strings.TrimSpace(recipe.Review) == "" || recipe.SQLModel == nil && (len(recipe.Sources) == 0 || len(recipe.Metrics) == 0) {
		return recipe, fmt.Errorf("%s: reviewed, nonempty SQL oracle recipe required", path)
	}
	return recipe, nil
}

func epathLoadRealExpectedManifest(t *testing.T, path string) epathRealExpectedManifest {
	t.Helper()
	manifest, err := epathReadRealExpectedManifest(path)
	if err != nil {
		t.Fatalf("approved expected manifest required (never auto-created): %v", err)
	}
	return manifest
}

func epathValidateOracleMetricGroups(metrics []epathRealOracleMetric) error {
	groups := map[string]bool{}
	keys := map[string]bool{}
	for _, metric := range metrics {
		if metric.Key == "" || keys[metric.Key] {
			return fmt.Errorf("empty/duplicate expected metric key %q", metric.Key)
		}
		keys[metric.Key] = true
		if !epathContainsOracleGroup(metric.Group) {
			return fmt.Errorf("unknown oracle group %q", metric.Group)
		}
		groups[metric.Group] = true
		if err := epathValidateOracleMetricIdentity(metric); err != nil {
			return err
		}
		if metric.Value != nil && !epathOracleFinite(*metric.Value) {
			return fmt.Errorf("nonfinite metric %s", metric.Key)
		}
		if metric.Value == nil && metric.Status == "" {
			return fmt.Errorf("unknown metric %s requires explicit status", metric.Key)
		}
	}
	for _, group := range epathRealOracleGroups {
		if !groups[group] {
			return fmt.Errorf("all eight oracle groups are required; missing %s", group)
		}
	}
	return nil
}

func epathContainsOracleGroup(value string) bool {
	for _, group := range epathRealOracleGroups {
		if value == group {
			return true
		}
	}
	return false
}

func epathOracleSourceValue(source epathRealSQLSource, period string) *float64 {
	if period == "annual" {
		if strings.EqualFold(source.ReportingFrequency, "Monthly") && len(source.Months) != 12 {
			return nil
		}
		return source.EnergyKWh
	}
	var month int
	if _, err := fmt.Sscanf(period, "M%d", &month); err != nil || fmt.Sprintf("M%d", month) != period || month < 1 || month > 12 {
		return nil
	}
	for _, bucket := range source.Months {
		if bucket.Month == month {
			return bucket.EnergyKWh
		}
	}
	return nil
}

func epathResolveOracleSource(observed []epathRealSQLSource, recipe epathRealOracleSourceRecipe, period string) (*float64, error) {
	if recipe.ID == "" || len(recipe.Names) == 0 || len(recipe.Keys) == 0 || recipe.Frequency == "" || recipe.SourceUnit == "" {
		return nil, fmt.Errorf("source recipe requires exact name/key/frequency/unit: %q", recipe.ID)
	}
	names := map[string]bool{}
	matchedKeys := map[string]bool{}
	sum := 0.0
	unknown := false
	count := 0
	for _, source := range observed {
		nameOK, keyOK := false, false
		for _, name := range recipe.Names {
			if strings.EqualFold(source.Name, name) {
				nameOK = true
			}
		}
		for _, key := range recipe.Keys {
			if key == "*" || strings.EqualFold(source.KeyValue, key) {
				keyOK = true
			}
		}
		if !nameOK || !keyOK || source.IsMeter != recipe.IsMeter || !strings.EqualFold(source.ReportingFrequency, recipe.Frequency) || !strings.EqualFold(source.SourceUnit, recipe.SourceUnit) {
			continue
		}
		key := strings.ToLower(source.KeyValue)
		if matchedKeys[key] {
			return nil, fmt.Errorf("ambiguous source %s: duplicate aliases/dictionaries for key %q", recipe.ID, key)
		}
		matchedKeys[key] = true
		names[strings.ToLower(source.Name)] = true
		count++
		value := epathOracleSourceValue(source, period)
		if value == nil {
			unknown = true
		} else {
			sum += *value
		}
	}
	if len(names) > 1 {
		return nil, fmt.Errorf("source %s has multiple observed alias alternatives; explicit reviewed preference required", recipe.ID)
	}
	if count == 0 {
		if recipe.AllowMissing {
			return nil, nil
		}
		return nil, fmt.Errorf("required exact SQL source missing: %s", recipe.ID)
	}
	for _, key := range recipe.Keys {
		if key != "*" && !matchedKeys[strings.ToLower(key)] {
			if recipe.AllowMissing {
				return nil, nil
			}
			return nil, fmt.Errorf("source %s missing key %q", recipe.ID, key)
		}
	}
	if unknown {
		return nil, nil
	}
	for _, factor := range []*float64{recipe.Multiplier, recipe.Sign} {
		if factor != nil {
			if !epathOracleFinite(*factor) {
				return nil, fmt.Errorf("invalid factor %s", recipe.ID)
			}
			sum *= *factor
		}
	}
	if !epathOracleFinite(sum) {
		return nil, fmt.Errorf("nonfinite source aggregate %s", recipe.ID)
	}
	return epathOracleNumber(sum), nil
}

func epathOracleArithmetic(operation string, inputs []*float64) (*float64, error) {
	if operation == "unavailable" {
		if len(inputs) != 0 {
			return nil, fmt.Errorf("unavailable must not disguise supplied observations")
		}
		return nil, nil
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("%s requires observed inputs", operation)
	}
	if operation == "count_found" {
		count := 0.0
		for _, value := range inputs {
			if value != nil {
				count++
			}
		}
		return epathOracleNumber(count), nil
	}
	for _, value := range inputs {
		if value == nil {
			return nil, nil
		}
	}
	result := *inputs[0]
	switch operation {
	case "source", "sum":
		for _, value := range inputs[1:] {
			result += *value
		}
	case "difference":
		for _, value := range inputs[1:] {
			result -= *value
		}
	case "positive":
		if len(inputs) != 1 {
			return nil, fmt.Errorf("positive requires one input")
		}
		result = math.Max(result, 0)
	case "negative":
		if len(inputs) != 1 {
			return nil, fmt.Errorf("negative requires one input")
		}
		result = math.Max(-result, 0)
	case "ratio", "percent":
		if len(inputs) != 2 {
			return nil, fmt.Errorf("%s requires numerator and denominator", operation)
		}
		if *inputs[1] <= 0 {
			return nil, nil
		}
		result /= *inputs[1]
		if operation == "percent" {
			result *= 100
		}
	case "share":
		if len(inputs) != 3 {
			return nil, fmt.Errorf("share requires total, weight and weight sum")
		}
		if *inputs[2] <= 0 {
			return nil, nil
		}
		result = result * *inputs[1] / *inputs[2]
	default:
		return nil, fmt.Errorf("unknown independent arithmetic operation %q", operation)
	}
	if !epathOracleFinite(result) {
		return nil, fmt.Errorf("nonfinite independent arithmetic result")
	}
	return epathOracleNumber(result), nil
}

func epathEvaluateRealOracle(out *epathRealOracleEvidence, recipe epathRealOracleRecipe, bundle PurposeResultBundle) error {
	if recipe.SQLModel != nil {
		if len(recipe.Sources) != 0 || len(recipe.Metrics) != 0 {
			return fmt.Errorf("a SQL model must not hide additional uncompiled legacy metrics")
		}
		checks, err := epathCompileSQLModelChecks(*out, *recipe.SQLModel)
		if err != nil {
			return err
		}
		failures := epathEvaluateSQLModelChecks(out, bundle, checks)
		if len(failures) > 0 {
			return fmt.Errorf("%d independent SQL candidate mismatches; first: %s", len(failures), failures[0].Message)
		}
		return nil
	}
	sources := map[string]epathRealOracleSourceRecipe{}
	metrics := map[string]epathRealOracleMetric{}
	for _, source := range recipe.Sources {
		if _, ok := sources[source.ID]; ok {
			return fmt.Errorf("duplicate source recipe %s", source.ID)
		}
		sources[source.ID] = source
	}
	groups := map[string]bool{}
	for _, item := range recipe.Metrics {
		if item.Key == "" || !epathContainsOracleGroup(item.Group) {
			return fmt.Errorf("invalid oracle metric key/group %q/%q", item.Key, item.Group)
		}
		if _, exists := metrics[item.Key]; exists {
			return fmt.Errorf("duplicate metric recipe %s", item.Key)
		}
		inputs := []*float64{}
		for _, input := range item.Inputs {
			if source, ok := sources[input]; ok {
				value, err := epathResolveOracleSource(out.Sources, source, item.Period)
				if err != nil {
					return err
				}
				inputs = append(inputs, value)
				continue
			}
			if value, ok := metrics[input]; ok {
				inputs = append(inputs, value.Value)
				continue
			}
			return fmt.Errorf("metric %s refers to missing/forward input %s", item.Key, input)
		}
		value, err := epathOracleArithmetic(item.Operation, inputs)
		if err != nil {
			return fmt.Errorf("metric %s: %w", item.Key, err)
		}
		metric := epathRealOracleMetric{Key: item.Key, Group: item.Group, Scope: item.Scope, Zone: item.Zone, Period: item.Period, Unit: item.Unit, Value: value, Status: item.Status}
		if value == nil && metric.Status == "" {
			metric.Status = "unavailable"
		}
		if item.Operation == "count_found" {
			found, total := int(*value), len(inputs)
			metric.Found = &found
			metric.Total = &total
		}
		if item.Target.Collection != "" {
			actual, err := epathReadOracleCandidate(bundle, item, metric)
			if err != nil {
				return err
			}
			if err := epathCompareOracleNumber(actual, value, item.AbsoluteTolerance, item.RelativeTolerance); err != nil {
				return fmt.Errorf("independent SQL vs candidate %s: %w", item.Key, err)
			}
			groups[item.Group] = true
		}
		metrics[item.Key] = metric
		out.Metrics = append(out.Metrics, metric)
	}
	for _, group := range epathRealOracleGroups {
		if groups[group] {
			out.CheckedGroups = append(out.CheckedGroups, group)
		}
	}
	return nil
}

func epathOracleGraph(bundle PurposeResultBundle, scope, zone, period string) ([]EnergyExplanationNode, []EnergyPathLink, []EnergyReconciliation, *EnergyPathQuality, error) {
	result := bundle.EnergyExplanation
	if result.Schema != energyExplanationSchema || result.Scope.Kind != "building" || result.Scope.ZoneName != "" {
		return nil, nil, nil, nil, fmt.Errorf("oracle requires the original canonical Building result, not a substituted scope/schema")
	}
	if (scope == "building" && zone != "") || (scope == "zone" && strings.TrimSpace(zone) == "") || !epathOracleValidPeriod(period) {
		return nil, nil, nil, nil, fmt.Errorf("invalid exact oracle scope/Zone/period %q/%q/%q", scope, zone, period)
	}
	nodes, links, rows, quality, periods := result.Nodes, result.Links, result.Reconciliation, result.Quality, result.Periods
	candidateZones := append([]string(nil), result.AvailableZones...)
	for _, candidate := range result.ZoneResults {
		if candidate.Scope.Kind == "zone" {
			candidateZones = append(candidateZones, candidate.Scope.ZoneName)
		}
	}
	if scope == "zone" {
		found := false
		for _, candidate := range result.ZoneResults {
			if strings.EqualFold(candidate.Scope.ZoneName, zone) {
				if candidate.Scope.Kind != "zone" {
					return nil, nil, nil, nil, fmt.Errorf("Zone wrapper has conflicting scope kind")
				}
				if found {
					return nil, nil, nil, nil, fmt.Errorf("ambiguous Zone %q", zone)
				}
				found = true
				nodes, links, rows, quality, periods = candidate.Nodes, candidate.Links, candidate.Reconciliation, candidate.Quality, candidate.Periods
			}
		}
		if !found {
			return nil, nil, nil, nil, fmt.Errorf("missing candidate Zone %q", zone)
		}
	} else if scope != "building" {
		return nil, nil, nil, nil, fmt.Errorf("invalid oracle scope %q", scope)
	}
	if period == "annual" {
		if err := epathValidateOracleGraphRecords(nodes, links, rows, scope, zone, period, candidateZones...); err != nil {
			return nil, nil, nil, nil, err
		}
		return nodes, links, rows, quality, nil
	}
	var selected *EnergyPeriod
	for _, candidate := range periods {
		if candidate.ID == period {
			if selected != nil {
				return nil, nil, nil, nil, fmt.Errorf("duplicate candidate period %q", period)
			}
			copy := candidate
			selected = &copy
		}
	}
	if selected != nil {
		if err := epathValidateOracleGraphRecords(selected.Nodes, selected.Links, selected.Reconciliation, scope, zone, period, candidateZones...); err != nil {
			return nil, nil, nil, nil, err
		}
		return selected.Nodes, selected.Links, selected.Reconciliation, selected.Quality, nil
	}
	return nil, nil, nil, nil, fmt.Errorf("missing candidate period %q; no annual fallback", period)
}

func epathReadOracleCandidate(bundle PurposeResultBundle, item epathRealOracleMetricRecipe, want epathRealOracleMetric) (*float64, error) {
	nodes, links, rows, quality, err := epathOracleGraph(bundle, item.Scope, item.Zone, item.Period)
	if err != nil {
		return nil, err
	}
	target := item.Target
	if err := epathValidateOracleTarget(target, item.Unit); err != nil {
		return nil, err
	}
	sum, count := 0.0, 0
	match := func(filter, value string) bool { return filter == "" || filter == value }
	switch target.Collection {
	case "nodes":
		for _, node := range nodes {
			if !epathOracleNodeMatches(node, target) {
				continue
			}
			if node.Unit != target.Unit || !match(target.ScaleDomain, node.ScaleDomain) || !match(target.AggregationBasis, node.AggregationBasis) {
				return nil, fmt.Errorf("selected node %s has wrong unit/domain/aggregation basis", node.ID)
			}
			value := node.Value
			bit := uint8(0)
			switch target.Field {
			case "value":
			case "rawValue":
				value = node.RawValue
				bit = 1
			case "effectiveValue":
				value = node.EffectiveValue
				bit = 2
			case "allocatedValue":
				value = node.AllocatedValue
				bit = 4
			case "loadBreakdown":
				found := false
				for _, component := range node.LoadBreakdown {
					if component.Component != target.Component {
						continue
					}
					if found {
						return nil, fmt.Errorf("duplicate load breakdown component")
					}
					found = true
					if component.Unit != target.Unit {
						return nil, fmt.Errorf("load breakdown unit mismatch")
					}
					value = component.Value
				}
				if !found {
					return nil, nil
				}
			default:
				return nil, fmt.Errorf("unknown node field %q", target.Field)
			}
			if bit != 0 && node.inspectorDecodedFromJSON && node.inspectorValuePresence&bit == 0 {
				return nil, nil
			}
			if bit != 0 && value == 0 && !node.inspectorDecodedFromJSON && !(node.Level == "driver" && node.AllocationApplied) {
				return nil, nil
			}
			if !epathOracleFinite(value) {
				return nil, fmt.Errorf("nonfinite candidate node %s", node.ID)
			}
			sum += value
			count++
		}
	case "sources":
		return epathReadOracleSourceCandidate(bundle.EnergyExplanation.Sources, item)
	case "links":
		if target.Field == "pairedRatio" {
			return epathReadOraclePairedRatio(nodes, links, bundle.EnergyExplanation.Sources, target)
		}
		for _, link := range links {
			if !match(target.ID, link.ID) || !match(target.FromID, link.FromID) || !match(target.ToID, link.ToID) || !match(target.Relation, link.Relation) || !match(target.Service, link.ServiceKind) || !match(target.Basis, link.Basis) {
				continue
			}
			if link.FromUnit != target.FromUnit || link.ToUnit != target.ToUnit || !match(target.RatioKind, link.RatioKind) {
				return nil, fmt.Errorf("selected link %s has wrong paired units/kind", link.ID)
			}
			if !epathOracleFinite(link.FromValue) || !epathOracleFinite(link.ToValue) {
				return nil, fmt.Errorf("nonfinite candidate link %s", link.ID)
			}
			switch target.Field {
			case "fromValue":
				sum += link.FromValue
			case "toValue":
				sum += link.ToValue
			default:
				return nil, fmt.Errorf("links require exact fromValue/toValue, not mean ratio")
			}
			count++
		}
	case "reconciliation":
		for _, row := range rows {
			if !match(target.ID, row.ID) || !match(target.Level, row.Level) || !match(target.Service, row.ServiceKind) || !match(target.Basis, row.Basis) {
				continue
			}
			if row.Unit != target.Unit || !match(target.AllocationMethod, row.AllocationMethod) || !match(target.Status, row.Status) {
				return nil, fmt.Errorf("selected reconciliation %s has wrong unit/method/status", row.ID)
			}
			switch target.Field {
			case "expectedValue":
				sum += row.ExpectedValue
			case "explainedValue":
				sum += row.ExplainedValue
			case "residualValue":
				sum += row.ResidualValue
			case "directValue":
				sum += row.DirectValue
			case "allocatedValue":
				sum += row.AllocatedValue
			case "unassignedValue":
				sum += row.UnassignedValue
			case "overmappedValue":
				sum += row.OvermappedValue
			default:
				return nil, fmt.Errorf("unknown reconciliation field %q", target.Field)
			}
			count++
		}
	case "quality":
		return epathReadStrictOracleQuality(nodes, rows, quality, target, want)
	default:
		return nil, fmt.Errorf("unknown candidate collection %q", target.Collection)
	}
	if count == 0 {
		if target.AllowPrunedZero && want.Value != nil && *want.Value == 0 {
			return epathOracleNumber(0), nil
		}
		return nil, nil
	}
	if count > 1 && (target.ID != "" || target.Aggregate != "sum") {
		return nil, fmt.Errorf("ambiguous candidate selection: %d records without an explicit sum", count)
	}
	return epathOracleNumber(sum), nil
}

func epathCompareOracleNumber(actual, want *float64, absolute, relative float64) error {
	if !epathOracleFinite(absolute) || !epathOracleFinite(relative) || absolute < 0 || relative < 0 || absolute > 0.1 || relative > 0.001 {
		return fmt.Errorf("unreviewably broad/invalid tolerance")
	}
	if actual == nil || want == nil {
		if actual == nil && want == nil {
			return nil
		}
		return fmt.Errorf("known/unknown mismatch actual=%v expected=%v", actual, want)
	}
	if !epathOracleFinite(*actual) || !epathOracleFinite(*want) {
		return fmt.Errorf("nonfinite comparison")
	}
	if absolute == 0 {
		absolute = 0.0001
	}
	if relative == 0 {
		relative = 1e-8
	}
	if math.Abs(*actual-*want) > math.Max(absolute, relative*math.Max(math.Abs(*actual), math.Abs(*want))) {
		return fmt.Errorf("actual %.12g expected %.12g", *actual, *want)
	}
	return nil
}

func epathAssertRealSQLOracle(t *testing.T, evidence epathRealRunEvidence, manifest epathRealExpectedManifest) {
	t.Helper()
	if manifest.FixtureID != evidence.Fixture.ID || manifest.Version != evidence.Version || manifest.ModelSHA256 != evidence.ModelSHA256 || manifest.WeatherSHA256 != evidence.Fixture.Weather.SHA256 {
		t.Fatal("expected manifest provenance does not match the executed fixture")
	}
	if strings.TrimSpace(evidence.Fixture.OraclePath) == "" {
		t.Fatal("acceptance requires approved independent SQL oracle recipe")
	}
	recipe, err := epathLoadRealOracleRecipe(filepath.Join(evidence.CatalogDirectory, filepath.FromSlash(evidence.Fixture.OraclePath)))
	if err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(evidence.SQLPath)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Run != nil {
		observed.outputPlan = evidence.Run.PurposeRunPlan
	}
	if err := epathEvaluateRealOracle(&observed, recipe, evidence.Bundle); err != nil {
		t.Fatal(err)
	}
	if err := epathValidateOracleMetricGroups(observed.Metrics); err != nil {
		t.Fatal(err)
	}
	if len(observed.CheckedGroups) != len(epathRealOracleGroups) {
		t.Fatalf("acceptance requires candidate comparisons for all eight groups; checked %v", observed.CheckedGroups)
	}
	byKey := map[string]epathRealOracleMetric{}
	for _, metric := range observed.Metrics {
		byKey[metric.Key] = metric
	}
	if len(byKey) != len(manifest.Metrics) {
		t.Fatalf("oracle/expected metric sets differ: %d/%d", len(byKey), len(manifest.Metrics))
	}
	for _, want := range manifest.Metrics {
		actual, ok := byKey[want.Key]
		if !ok {
			t.Fatalf("expected metric not independently checked: %s", want.Key)
		}
		if actual.Group != want.Group || actual.Scope != want.Scope || actual.Zone != want.Zone || actual.Period != want.Period || actual.Unit != want.Unit || actual.Status != want.Status {
			t.Fatalf("metric identity/status mismatch %s: %#v / %#v", want.Key, actual, want)
		}
		if (actual.Found == nil) != (want.Found == nil) || (actual.Total == nil) != (want.Total == nil) {
			t.Fatalf("metric known-count mismatch %s", want.Key)
		}
		if actual.Found != nil && (*actual.Found != *want.Found || *actual.Total != *want.Total) {
			t.Fatalf("metric counts mismatch %s", want.Key)
		}
		if err := epathCompareOracleNumber(actual.Value, want.Value, 0, 0); err != nil {
			t.Errorf("approved expected %s: %v", want.Key, err)
		}
	}
	keys := []string{}
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	t.Logf("Independent SQL and approved expected values verified for all eight groups (%d semantic metrics)", len(keys))
}
