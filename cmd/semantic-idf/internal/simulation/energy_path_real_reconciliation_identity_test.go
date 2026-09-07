package simulation

import (
	"encoding/json"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Closed identity regression from the immutable LargeOffice 0350 snapshot
// (SHA256 c5368a4a472a0ebb2be46b231b6e92e2a06ed7f06f0c790f6013804dd86b64a0).
// These are observed reconciliation rows, NOT an independently accepted energy
// oracle or new expected manifest. No saved-loader/engine run is needed here.
func epathRealReconciliationFixture() EnergyExplanationResult {
	sources := []EnergyDataSource{
		{ID: "sql-rdd-12", DriverComponent: "internal.reconciliation.latent"},
		{ID: "sql-rdd-88", DriverComponent: "internal.people.latent.gain"},
		{ID: "sql-rdd-10", DriverComponent: "internal.reconciliation.convective"},
		{ID: "sql-rdd-1323", DriverComponent: "internal.equipment.sensible.gain"},
		{ID: "sql-rdd-827", DriverComponent: "internal.lighting.sensible.gain"},
		{ID: "sql-rdd-86", DriverComponent: "internal.people.sensible.gain"},
	}
	latent := []float64{888.564, 817.527, 1008.035, 915.494, 1025.169, 1131.182, 1061.735, 1194.738, 1038.401, 973.727, 920.184, 873.812}
	sensible := []float64{9756.799, 8837.863, 10213.968, 9396.633, 9931.575, 9712.828, 9436.885, 10083.276, 9306.764, 9699.102, 9522.256, 9566.514}
	base := EnergyReconciliation{ID: "reconcile.driver.internal.basement.annual", Level: "driver", Period: "annual", Label: "Internal gain reconciliation - Basement", Status: "balanced", ZoneName: "Basement", ExpectedValue: 127313.031, ExplainedValue: 127313.034, ResidualValue: -0.003, Unit: "kWh", Basis: "residual", Formula: "signed total internal aggregate - signed mapped internal source families", SourceIDs: []string{"sql-rdd-12", "sql-rdd-88", "sql-rdd-10", "sql-rdd-1323", "sql-rdd-827", "sql-rdd-86"}}
	unrelated := EnergyReconciliation{ID: "reconcile.driver.surface.basement.annual", Level: "driver", Period: "annual", ZoneName: "Basement", Label: "Surface reconciliation", ExpectedValue: 7, ExplainedValue: 7, Unit: "kWh", Basis: "residual"}
	result := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Sources: sources, Reconciliation: []EnergyReconciliation{base, unrelated}, Nodes: []EnergyExplanationNode{{ID: "unrelated", Level: "support", Value: 7}}, Links: []EnergyPathLink{{ID: "unrelated-link", FromValue: 3, ToValue: 4}}}
	result.Periods = []EnergyPeriod{{ID: "annual", Kind: "annual", Reconciliation: append([]EnergyReconciliation(nil), result.Reconciliation...)}}
	for index := range latent {
		period := "M" + strconv.Itoa(index+1)
		left, right := base, base
		left.ID, right.ID = "reconcile.driver.internal.basement."+period, "reconcile.driver.internal.basement."+period
		left.Period, right.Period = period, period
		left.ExpectedValue, left.ExplainedValue, left.ResidualValue = latent[index], latent[index], 0
		right.ExpectedValue, right.ExplainedValue, right.ResidualValue = sensible[index], sensible[index], 0
		if index == 3 || index == 4 || index == 11 {
			right.ExplainedValue = roundedEnergyNumber(right.ExplainedValue + 0.001)
			right.ResidualValue = -0.001
		}
		left.SourceIDs = []string{"sql-rdd-12", "sql-rdd-88"}
		right.SourceIDs = []string{"sql-rdd-10", "sql-rdd-1323", "sql-rdd-827", "sql-rdd-86"}
		result.Periods = append(result.Periods, EnergyPeriod{ID: period, Kind: "monthly", Reconciliation: []EnergyReconciliation{left, right}})
	}
	result.ZoneResults = []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Basement"}, Reconciliation: append([]EnergyReconciliation(nil), result.Reconciliation...), Periods: append([]EnergyPeriod(nil), result.Periods...)}}
	return result
}

func epathAssertReconciliationComponents(t *testing.T, rows []EnergyReconciliation, period string) {
	t.Helper()
	seen := map[string]bool{}
	for _, row := range rows {
		if !strings.HasPrefix(row.ID, "reconcile.driver.internal.basement.") {
			continue
		}
		component := ""
		if strings.Contains(row.ID, ".component.latent.") {
			component = "latent"
		} else if strings.Contains(row.ID, ".component.sensible.") {
			component = "sensible"
		}
		if component == "" || seen[component] || !strings.HasSuffix(row.ID, "."+period) || row.Period != period {
			t.Fatalf("non-unique/non-semantic component identity: %#v", row)
		}
		seen[component] = true
		if !strings.HasSuffix(row.Label, " · "+strings.ToUpper(component[:1])+component[1:]) {
			t.Fatalf("component rows remain indistinguishable to readers: %q", row.Label)
		}
		if period == "annual" {
			wantExpected, wantExplained, wantResidual, sourceCount := 11848.568, 11848.568, 0.0, 2
			if component == "sensible" {
				wantExpected, wantExplained, wantResidual, sourceCount = 115464.463, 115464.466, -0.003, 4
			}
			if row.ExpectedValue != wantExpected || row.ExplainedValue != wantExplained || row.ResidualValue != wantResidual || len(row.SourceIDs) != sourceCount {
				t.Fatalf("annual %s values/provenance changed: %#v", component, row)
			}
		}
	}
	if len(seen) != 2 {
		t.Fatalf("%s lost sensible/latent accounting rows: %#v", period, rows)
	}
}

func TestEnergyPathRealReconciliationIdentityStoredRepair(t *testing.T) {
	original := epathRealReconciliationFixture()
	before, _ := json.Marshal(original)
	result := original
	if !canonicalizeEnergyPathDriverReconciliation(&result) {
		t.Fatal("actual stored component collision was not repaired")
	}
	after, _ := json.Marshal(original)
	if string(before) != string(after) {
		t.Fatal("canonical upgrade mutated the original shared bundle")
	}
	if !reflect.DeepEqual(result.Nodes, original.Nodes) || !reflect.DeepEqual(result.Links, original.Links) || !reflect.DeepEqual(result.Sources, original.Sources) || !reflect.DeepEqual(result.Reconciliation[2], original.Reconciliation[1]) {
		t.Fatal("reconciliation-only repair changed unrelated graph/data/identity")
	}
	epathAssertReconciliationComponents(t, result.Reconciliation, "annual")
	for _, period := range result.Periods {
		epathAssertReconciliationComponents(t, period.Reconciliation, period.ID)
	}
	epathAssertReconciliationComponents(t, result.ZoneResults[0].Reconciliation, "annual")
	for _, period := range result.ZoneResults[0].Periods {
		epathAssertReconciliationComponents(t, period.Reconciliation, period.ID)
	}
	stable, _ := json.Marshal(result)
	if canonicalizeEnergyPathDriverReconciliation(&result) {
		t.Fatal("component identity/label repair is not idempotent")
	}
	twice, _ := json.Marshal(result)
	if string(stable) != string(twice) {
		t.Fatal("second pass changed canonical identity")
	}
	// Exercise the actual canonical JSON read boundary, not only the helper.
	var reloaded EnergyExplanationResult
	if err := json.Unmarshal(before, &reloaded); err != nil {
		t.Fatal(err)
	}
	epathAssertReconciliationComponents(t, reloaded.Reconciliation, "annual")
	encoded, _ := json.Marshal(reloaded)
	var again EnergyExplanationResult
	if err := json.Unmarshal(encoded, &again); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded.Reconciliation, again.Reconciliation) {
		t.Fatal("canonical reconciliation does not survive JSON reload")
	}
}

func TestEnergyPathRealReconciliationIdentityPermutationAndPeriodFamily(t *testing.T) {
	result := epathRealReconciliationFixture()
	canonicalizeEnergyPathDriverReconciliation(&result)
	permuted := epathRealReconciliationFixture()
	for i, j := 0, len(permuted.Periods)-1; i < j; i, j = i+1, j-1 {
		permuted.Periods[i], permuted.Periods[j] = permuted.Periods[j], permuted.Periods[i]
	}
	for i := range permuted.Periods {
		rows := permuted.Periods[i].Reconciliation
		for a, b := 0, len(rows)-1; a < b; a, b = a+1, b-1 {
			rows[a], rows[b] = rows[b], rows[a]
		}
	}
	canonicalizeEnergyPathDriverReconciliation(&permuted)
	rowSet := func(rows []EnergyReconciliation) map[string]EnergyReconciliation {
		out := map[string]EnergyReconciliation{}
		for _, row := range rows {
			row.SourceIDs = append([]string(nil), row.SourceIDs...)
			sort.Strings(row.SourceIDs)
			out[row.ID] = row
		}
		return out
	}
	if !reflect.DeepEqual(rowSet(result.Reconciliation), rowSet(permuted.Reconciliation)) {
		t.Fatal("annual component identity or arithmetic depends on input order")
	}
	// M1 contains sensible only; another month proves the family's second
	// component. M1 must use the same semantic ID as the two-component months.
	sparse := epathRealReconciliationFixture()
	sparse.Periods[1].Reconciliation = sparse.Periods[1].Reconciliation[1:]
	canonicalizeEnergyPathDriverReconciliation(&sparse)
	if got := sparse.Periods[1].Reconciliation[0].ID; got != "reconcile.driver.internal.basement.component.sensible.M1" {
		t.Fatalf("single-component month did not inherit family identity: %q", got)
	}
	if !reflect.DeepEqual(sparse.Reconciliation, epathRealReconciliationFixture().Reconciliation) {
		t.Fatal("missing component month was invented to repair an incompatible annual aggregate")
	}
}

func TestEnergyPathRealReconciliationIdentityAnnualRepairGuards(t *testing.T) {
	cases := map[string]func(*EnergyExplanationResult){
		"missing month":      func(r *EnergyExplanationResult) { r.Periods = r.Periods[:12] },
		"duplicate month":    func(r *EnergyExplanationResult) { r.Periods[12] = r.Periods[11] },
		"expected mismatch":  func(r *EnergyExplanationResult) { r.Reconciliation[0].ExpectedValue += .001 },
		"explained mismatch": func(r *EnergyExplanationResult) { r.Reconciliation[0].ExplainedValue += .001 },
		"residual mismatch":  func(r *EnergyExplanationResult) { r.Reconciliation[0].ResidualValue += .001 },
		"source union mismatch": func(r *EnergyExplanationResult) {
			r.Reconciliation[0].SourceIDs = append(r.Reconciliation[0].SourceIDs, "not-observed")
		},
		"different unit":    func(r *EnergyExplanationResult) { r.Periods[1].Reconciliation[0].Unit = "J" },
		"different basis":   func(r *EnergyExplanationResult) { r.Periods[1].Reconciliation[0].Basis = "reported" },
		"different formula": func(r *EnergyExplanationResult) { r.Periods[1].Reconciliation[0].Formula = "different semantics" },
		"nonfinite month":   func(r *EnergyExplanationResult) { r.Periods[1].Reconciliation[0].ExpectedValue = math.Inf(1) },
		"nonfinite annual":  func(r *EnergyExplanationResult) { r.Reconciliation[0].ExpectedValue = math.Inf(1) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			result := epathRealReconciliationFixture()
			mutate(&result)
			original := append([]EnergyReconciliation(nil), result.Reconciliation...)
			canonicalizeEnergyPathDriverReconciliation(&result)
			if !reflect.DeepEqual(result.Reconciliation, original) {
				t.Fatalf("unproved annual split accepted: %#v", result.Reconciliation)
			}
		})
	}
}

func TestEnergyPathRealReconciliationIdentityUnknownTagsFailClosed(t *testing.T) {
	cases := map[string]func(*EnergyExplanationResult){
		"unknown exact tag": func(r *EnergyExplanationResult) {
			r.Sources[0].DriverComponent = "internal.reconciliation.latent.guessed"
		},
		"conflicting explicit tags": func(r *EnergyExplanationResult) { r.Sources[0].DriverComponent = "internal.people.sensible.gain" },
		"missing source":            func(r *EnergyExplanationResult) { r.Sources = r.Sources[1:] },
		"ambiguous source id":       func(r *EnergyExplanationResult) { r.Sources = append(r.Sources, r.Sources[0]) },
		"duplicate same component": func(r *EnergyExplanationResult) {
			r.Periods[1].Reconciliation = append(r.Periods[1].Reconciliation, r.Periods[1].Reconciliation[0])
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			result := epathRealReconciliationFixture()
			mutate(&result)
			before := append([]EnergyReconciliation(nil), result.Periods[1].Reconciliation...)
			canonicalizeEnergyPathDriverReconciliation(&result)
			if !reflect.DeepEqual(result.Periods[1].Reconciliation, before) {
				t.Fatal("unknown/conflicting observations were assigned component identity")
			}
		})
	}
}

func TestEnergyPathRealReconciliationIdentityConstructorAndFrozenV1(t *testing.T) {
	series := []energyExplanationSeries{
		reviewEnergyHeatSeries(t, "Basement", "Zone Total Internal Latent Gain Energy", 888.564, "latent-total"),
		reviewEnergyHeatSeries(t, "Basement", "Zone People Latent Gain Energy", 888.564, "latent-people"),
		reviewEnergyHeatSeries(t, "Basement", "Zone Total Internal Convective Heating Energy", 9756.799, "sensible-total"),
		reviewEnergyHeatSeries(t, "Basement", "Zone People Convective Heating Energy", 9756.799, "sensible-people"),
	}
	series, sources, _ := prepareEnergyDriverSeries(series, reviewEnergySources(series), energyDriverBuildContext{Enabled: true})
	rows, _ := appendEnergyDriverPeriodAccounting("M1", series, func(item energyExplanationSeries) float64 { return item.Total }, nil, nil)
	if len(rows) != 2 || rows[0].ID != rows[1].ID || rows[0].driverAccountingComponent != "latent" || rows[1].driverAccountingComponent != "sensible" {
		t.Fatalf("constructor lost component metadata or altered frozen v1 ID: %#v", rows)
	}
	wire, _ := json.Marshal(rows)
	if strings.Contains(string(wire), "component") || strings.Contains(string(wire), " · ") {
		t.Fatalf("new canonical metadata leaked into v1 JSON: %s", wire)
	}
	annual := aggregateEnergyExplanationMonthlyGraphs(map[int]energyExplanationGraph{1: {Reconciliation: rows}})
	if len(annual.Reconciliation) != 1 || annual.Reconciliation[0].ID != "reconcile.driver.internal.basement.annual" || annual.Reconciliation[0].ExpectedValue != 10645.363 || annual.Reconciliation[0].driverAccountingComponent != "" {
		t.Fatalf("frozen v1 annual aggregation changed or retained a false component: %#v", annual.Reconciliation)
	}
	input := EnergyExplanationV1{Schema: energyExplanationV1Schema, Reconciliation: annual.Reconciliation, Periods: []EnergyPeriod{{ID: "M1", Kind: "monthly", Reconciliation: rows}}, Sources: sources, canonicalMonthlyBasis: true}
	before, _ := json.Marshal(input)
	result := UpgradeEnergyExplanationV1(input)
	epathAssertReconciliationComponents(t, result.Periods[0].Reconciliation, "M1")
	if len(result.Reconciliation) != 2 {
		t.Fatalf("fresh runtime annual aggregation merged thermal components: %#v", result.Reconciliation)
	}
	for _, row := range result.Reconciliation {
		if !strings.HasSuffix(row.ID, ".annual") || !strings.Contains(row.ID, ".component.") {
			t.Fatalf("runtime annual ID lost component: %s", row.ID)
		}
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("UpgradeEnergyExplanationV1 mutated the frozen source payload")
	}
}

func TestEnergyPathRealReconciliationIdentityQualifierCollisionAndUniqueRows(t *testing.T) {
	result := epathRealReconciliationFixture()
	// Two real labels can normalize to the same legacy metricID. Detect the
	// resulting qualifier collision instead of making arbitrary ordinal IDs.
	result.Reconciliation = nil
	result.Periods = result.Periods[1:2]
	result.ZoneResults = nil
	for _, zone := range []string{"Office-A", "Office A"} {
		for _, original := range append([]EnergyReconciliation(nil), result.Periods[0].Reconciliation[:2]...) {
			original.ZoneName = zone
			original.Label = "Internal gain reconciliation - " + zone
			original.ID = "reconcile.driver.internal." + metricID(zone) + ".M1"
			result.Periods[0].Reconciliation = append(result.Periods[0].Reconciliation, original)
		}
	}
	before := append([]EnergyReconciliation(nil), result.Periods[0].Reconciliation[2:]...)
	canonicalizeEnergyPathDriverReconciliation(&result)
	if !reflect.DeepEqual(result.Periods[0].Reconciliation[2:], before) {
		t.Fatal("ambiguous Zone normalization was hidden with invented qualifiers")
	}
	unique := epathRealReconciliationFixture()
	unique.Reconciliation = nil
	unique.ZoneResults = nil
	unique.Periods = unique.Periods[1:2]
	unique.Periods[0].Reconciliation = unique.Periods[0].Reconciliation[1:]
	want := append([]EnergyReconciliation(nil), unique.Periods[0].Reconciliation...)
	if canonicalizeEnergyPathDriverReconciliation(&unique) || !reflect.DeepEqual(unique.Periods[0].Reconciliation, want) {
		t.Fatal("an already unique unrelated family received unnecessary ID/label changes")
	}
}
