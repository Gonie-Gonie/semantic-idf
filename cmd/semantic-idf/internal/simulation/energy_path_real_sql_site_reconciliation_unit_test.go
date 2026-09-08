package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func epathSQLSiteReconciliationUnitChecks(t *testing.T, expected, explained [12]float64) epathSQLModelChecks {
	t.Helper()
	frames := epathSQLFrames{Site: map[string][]*epathSQLQuantity{}}
	model := epathRealSQLModel{Site: []epathRealSQLSite{
		{ID: "facility.gas", Carrier: "natural_gas", Facility: true},
		{ID: "heating.gas", Carrier: "natural_gas", EndUse: "heating"},
	}}
	for month := range expected {
		frames.Site["facility.gas"] = append(frames.Site["facility.gas"], &epathSQLQuantity{Value: expected[month]})
		frames.Site["heating.gas"] = append(frames.Site["heating.gas"], &epathSQLQuantity{Value: explained[month]})
	}
	before, err := json.Marshal(frames)
	if err != nil {
		t.Fatal(err)
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelSiteChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(frames)
	if err != nil || string(before) != string(after) {
		t.Fatal("whole-row proof changed the independent source frames")
	}
	return checks
}

func epathSQLSiteReconciliationPeriodChecks(t *testing.T, checks epathSQLModelChecks, period string) epathSQLModelChecks {
	t.Helper()
	var selected epathSQLModelChecks
	for _, check := range checks.Rows {
		if check.Want.Group == "residuals" && check.Item.Period == period {
			selected.Rows = append(selected.Rows, check)
		}
	}
	if len(selected.Rows) != 3 {
		t.Fatalf("%s lost its three existing residual metric obligations: %d", period, len(selected.Rows))
	}
	return selected
}

func epathSQLSiteReconciliationUnitBundle(period string, expected, explained float64, present bool) PurposeResultBundle {
	var rows []EnergyReconciliation
	if present {
		rows = []EnergyReconciliation{{ID: "reconcile.energy.natural_gas." + period, Level: "energy", Period: period,
			Basis: "residual", Unit: "kWh", ExpectedValue: expected, ExplainedValue: explained, ResidualValue: expected - explained}}
	}
	result := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}
	kind := "monthly"
	if period == "annual" {
		kind = "annual"
		result.Reconciliation = append([]EnergyReconciliation(nil), rows...)
	}
	result.Periods = []EnergyPeriod{{ID: period, Kind: kind, Reconciliation: append([]EnergyReconciliation(nil), rows...)}}
	return PurposeResultBundle{EnergyExplanation: result}
}

func TestEnergyPathRealSQLSiteReconciliationPreservesMetricIdentityAndValues(t *testing.T) {
	var expected, explained [12]float64
	for month := range expected {
		expected[month], explained[month] = float64(month+1)+.125, float64(month+1)+.25
	}
	checks := epathSQLSiteReconciliationUnitChecks(t, expected, explained)
	if len(checks.Rows) != 65 || len(checks.Keys) != 65 {
		t.Fatalf("site checks changed from (end use + carrier + 3 residuals)*13: %d/%d", len(checks.Rows), len(checks.Keys))
	}
	count, seen := 0, map[string]bool{}
	for _, check := range checks.Rows {
		if check.Want.Group != "residuals" {
			if check.Reconciliation != nil {
				t.Fatal("whole-row proof escaped its reconciliation collection")
			}
			continue
		}
		count++
		e, x := 0.0, 0.0
		if check.Item.Period == "annual" {
			for month := range expected {
				e, x = e+expected[month], x+explained[month]
			}
		} else {
			var month int
			if _, err := fmt.Sscanf(check.Item.Period, "M%d", &month); err != nil || month < 1 || month > 12 {
				t.Fatal("unexpected compiler period")
			}
			e, x = expected[month-1], explained[month-1]
		}
		values := map[string]float64{"expectedValue": e, "explainedValue": x, "residualValue": e - x}
		value, ok := values[check.Item.Target.Field]
		if !ok {
			t.Fatal("unexpected fourth reconciliation metric")
		}
		key := "residuals|building||" + check.Item.Period + "|natural_gas/" + check.Item.Target.Field
		want := epathRealOracleMetric{Key: key, Group: "residuals", Scope: "building", Period: check.Item.Period, Unit: "kWh", Value: &value}
		if seen[key] || !reflect.DeepEqual(check.Want, want) || check.Quantity == nil || check.Quantity.Value != value || check.Quantity.Error != 0 {
			t.Fatalf("existing approved metric identity/precision changed: %+v", check)
		}
		seen[key] = true
		p := check.Reconciliation
		if p == nil || p.ID != "reconcile.energy.natural_gas."+check.Item.Period || p.Level != "energy" || p.Period != check.Item.Period || p.Basis != "residual" || p.Unit != "kWh" || p.ZoneName != "" || p.Service != "" || p.Expected.Value != e || p.Explained.Value != x || p.Residual.Value != e-x || check.Item.Target.Basis != p.Basis {
			t.Fatalf("known observations lack an exact whole-row proof: %+v", p)
		}
	}
	if count != 39 || len(seen) != 39 {
		t.Fatalf("three residuals per period changed: %d/%d", count, len(seen))
	}
}

func TestEnergyPathRealSQLSiteReconciliationWholeZeroVersusBalancedPositive(t *testing.T) {
	for _, period := range []string{"M7", "annual"} {
		for _, value := range []float64{0, 2, .000001} {
			var monthly [12]float64
			if period == "M7" {
				// The zero month is independently observed, not an annual zero
				// projected over months. Other months remain positive.
				for month := range monthly {
					monthly[month] = 3
				}
			}
			monthly[6] = value
			checks := epathSQLSiteReconciliationPeriodChecks(t, epathSQLSiteReconciliationUnitChecks(t, monthly, monthly), period)
			for _, present := range []bool{true, false} {
				bundle := epathSQLSiteReconciliationUnitBundle(period, value, value, present)
				failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks)
				if (len(failures) == 0) != (present || value == 0) {
					t.Fatalf("%s value=%g present=%v: %v", period, value, present, failures)
				}
			}
		}
	}
}

func TestEnergyPathRealSQLSiteReconciliationPresentRowRemainsStrict(t *testing.T) {
	checks := epathSQLSiteReconciliationPeriodChecks(t, epathSQLSiteReconciliationUnitChecks(t, [12]float64{}, [12]float64{}), "M7")
	for name, edit := range map[string]func(*EnergyReconciliation){
		"expected nonzero":  func(r *EnergyReconciliation) { r.ExpectedValue = .000001 },
		"explained nonzero": func(r *EnergyReconciliation) { r.ExplainedValue = .000001 },
		"residual nonzero":  func(r *EnergyReconciliation) { r.ResidualValue = -.000001 },
		"NaN":               func(r *EnergyReconciliation) { r.ExpectedValue = math.NaN() },
		"infinity":          func(r *EnergyReconciliation) { r.ResidualValue = math.Inf(1) },
		"wrong Zone":        func(r *EnergyReconciliation) { r.ZoneName = "SPACE1-1" },
		"wrong period":      func(r *EnergyReconciliation) { r.Period = "M6" },
		"wrong service":     func(r *EnergyReconciliation) { r.ServiceKind = "heating" },
		"wrong level":       func(r *EnergyReconciliation) { r.Level = "heat" },
		"wrong basis":       func(r *EnergyReconciliation) { r.Basis = "reported_meter" },
		"wrong unit":        func(r *EnergyReconciliation) { r.Unit = "J" },
	} {
		t.Run(name, func(t *testing.T) {
			bundle := epathSQLSiteReconciliationUnitBundle("M7", 0, 0, true)
			edit(&bundle.EnergyExplanation.Periods[0].Reconciliation[0])
			if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) != 3 {
				t.Fatalf("each selected field must validate the entire present row: %v", failures)
			}
		})
	}
	bundle := epathSQLSiteReconciliationUnitBundle("M7", 0, 0, true)
	bundle.EnergyExplanation.Periods[0].Reconciliation = append(bundle.EnergyExplanation.Periods[0].Reconciliation, bundle.EnergyExplanation.Periods[0].Reconciliation[0])
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) != 3 {
		t.Fatalf("duplicate exact rows bypassed zero pruning: %v", failures)
	}
}

func TestEnergyPathRealSQLSiteReconciliationWireMissingIsNotWholePruning(t *testing.T) {
	for _, field := range []string{"expectedValue", "explainedValue", "residualValue"} {
		for _, mode := range []string{"missing", "null"} {
			fixture := epathOracleWireFixture()
			graph := epathOracleWireFixtureGraph(fixture, "building-period")
			row := map[string]any{"id": "reconcile.energy.natural_gas.M7", "level": "energy", "period": "M7", "basis": "residual", "unit": "kWh", "expectedValue": 0, "explainedValue": 0, "residualValue": 0}
			graph["reconciliation"] = []any{row}
			if err := epathOracleWireFixtureCheck(t, fixture); err != nil {
				t.Fatalf("explicit known zero rejected before mutation: %v", err)
			}
			if mode == "null" {
				row[field] = nil
			} else {
				delete(row, field)
			}
			if err := epathOracleWireFixtureCheck(t, fixture); err == nil {
				t.Fatalf("present %s/%s became a pruned whole row", field, mode)
			}
			delete(graph, "reconciliation")
			if err := epathOracleWireFixtureCheck(t, fixture); err != nil {
				t.Fatalf("wire boundary forbids genuine whole collection omission: %v", err)
			}
		}
	}
}

func TestEnergyPathRealSQLSiteReconciliationAnnualUnknownMonthsStayUnknown(t *testing.T) {
	frames, model, bundle := epathSQLAnnualSiteUnitInputs(t)
	var all epathSQLModelChecks
	if err := epathSQLModelSiteChecks(frames, model, &all); err != nil {
		t.Fatal(err)
	}
	annual, unknown := 0, 0
	for _, check := range all.Rows {
		if check.Want.Group != "residuals" {
			continue
		}
		if check.Item.Period == "annual" {
			annual++
			if check.Reconciliation == nil {
				t.Fatal("observed annual values lost their whole-row proof")
			}
			continue
		}
		unknown++
		if check.Reconciliation != nil || check.Quantity != nil || check.Want.Value != nil || check.Want.Status != "unavailable" || check.Item.Target.AllowPrunedZero {
			t.Fatal("unreported AnnualTabular month was healed into observed zero")
		}
	}
	if annual != 3 || unknown != 36 {
		t.Fatalf("annual/monthly residual obligations changed: %d/%d", annual, unknown)
	}
	for _, period := range []string{"M1", "M7", "M12"} {
		checks := epathSQLSiteReconciliationPeriodChecks(t, all, period)
		if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) != 0 {
			t.Fatalf("actual unavailable month rejected: %v", failures)
		}
		mutant := bundle
		mutant.EnergyExplanation.Periods = append([]EnergyPeriod(nil), bundle.EnergyExplanation.Periods...)
		for i := range mutant.EnergyExplanation.Periods {
			if mutant.EnergyExplanation.Periods[i].ID == period {
				mutant.EnergyExplanation.Periods[i].Reconciliation = []EnergyReconciliation{{ID: "reconcile.energy.district_cooling." + period, Level: "energy", Period: period, Basis: "residual", Unit: "kWh"}}
			}
		}
		if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, mutant, checks); len(failures) != 3 {
			t.Fatalf("invented zero reconciliation replaced unknown month: %v", failures)
		}
	}
}
