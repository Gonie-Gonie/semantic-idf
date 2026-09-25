package simulation

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func epathSQLSimpleVentilationCarrierIDUnitInputs(t *testing.T) (epathRealOracleEvidence, epathRealSQLModel, epathSQLFrames, epathSQLModelChecks) {
	t.Helper()
	observed, model, frames := epathSQLSimpleVentilationZeroServiceUnit(t)
	var checks epathSQLModelChecks
	if err := epathSQLModelDirectFanChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return observed, model, frames, checks
}

func TestEnergyPathSQLSimpleVentilationCarrierPlainIDsAreNativeDirectAllPeriods(t *testing.T) {
	observed, model, frames, checks := epathSQLSimpleVentilationCarrierIDUnitInputs(t)
	before := len(checks.Rows)
	if err := epathSQLModelZoneCarrierChecks(frames, model, &checks, observed); err != nil {
		t.Fatal(err)
	}
	rows := checks.Rows[before:]
	if len(rows) != 3*13*5 {
		t.Fatalf("changed carrier field obligations: %d", len(rows))
	}
	reconciliations := 0
	for _, c := range rows {
		if c.Reconciliation == nil {
			continue
		}
		reconciliations++
		wantID := "reconcile.energy.electricity." + c.Item.Period
		if c.Reconciliation.ID != wantID || !reflect.DeepEqual(c.Reconciliation.AllowedIDs, []string{wantID}) {
			t.Fatal("finite identity must be exact, never unioned with guessed legacy IDs")
		}
		// Hand native fan fixture is 0, 2, 3 kWh each month, factor one.
		want := map[string]float64{"ZONE 1": 0, "ZONE 2": 2, "ZONE 3": 3}[c.Item.Zone]
		if c.Item.Period == "annual" {
			want *= 12
		}
		if c.Item.Target.Field == "residualValue" {
			want = 0
		}
		if c.Quantity == nil || math.Abs(c.Quantity.Value-want) > 1e-10 {
			t.Fatal("identity change altered independent native quantity")
		}
	}
	if reconciliations != 117 {
		t.Fatal("lost exact 39 Zone-period whole-row proofs")
	}
}

func TestEnergyPathSQLSimpleVentilationCarrierPlainIDsRequireActualZeroServiceEvidence(t *testing.T) {
	for _, mutation := range []string{"missing evidence", "duplicate evidence", "missing original", "missing executed", "missing load month", "uncertain zero", "paid meter even zero", "fan pool", "missing native fan"} {
		t.Run(mutation, func(t *testing.T) {
			observed, model, frames, checks := epathSQLSimpleVentilationCarrierIDUnitInputs(t)
			evidence := []epathRealOracleEvidence{observed}
			switch mutation {
			case "missing evidence":
				evidence = nil
			case "duplicate evidence":
				evidence = append(evidence, observed)
			case "missing original":
				evidence[0].originalText = ""
			case "missing executed":
				evidence[0].executedText = ""
			case "missing load month":
				delete(frames.Loads, epathSQLKey("zone 1", "heating", 4))
			case "uncertain zero":
				frames.Loads[epathSQLKey("zone 2", "cooling", 4)] = epathSQLQuantity{Error: 1e-12}
			case "paid meter even zero":
				epathOracleEditSQL(t, observed.sqlPath, "INSERT INTO ReportDataDictionary VALUES(91,'Heating:Electricity','',1,'Monthly','J','Facility','Sum','Zone',''); INSERT INTO ReportData SELECT 9100+TimeIndex,91,TimeIndex,0 FROM Time WHERE TimeIndex BETWEEN 1 AND 12")
			case "fan pool":
				model.FanPools = []epathRealSQLFanPool{{SiteID: "fans.electricity"}}
			case "missing native fan":
				epathOracleEditSQL(t, observed.sqlPath, "DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=40")
			}
			if err := epathSQLModelZoneCarrierChecks(frames, model, &checks, evidence...); err == nil {
				t.Fatal("unproved absence/zero/direct ownership selected plain IDs")
			}
		})
	}
	// Ordinary allocated models keep the old path without native evidence.
	frames, model, checks := epathSQLZoneCarrierUnitInputs(t)
	if err := epathSQLModelZoneCarrierChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	for _, c := range checks.Rows {
		if c.Reconciliation != nil && strings.Contains(c.Want.Key, "zone_subtotal/") && c.Reconciliation.ID == "reconcile.energy.electricity."+c.Item.Period {
			t.Fatal("ordinary allocation borrowed finite plain ID")
		}
	}
}

func TestEnergyPathSQLSimpleVentilationCarrierRejectsHiddenAllocatedParts(t *testing.T) {
	for _, mutation := range []string{"allocated", "unavailable", "annual", "hidden family", "foreign carrier", "missing known-zero fan", "missing period"} {
		t.Run(mutation, func(t *testing.T) {
			observed, model, frames, checks := epathSQLSimpleVentilationCarrierIDUnitInputs(t)
			parts, _, err := epathSQLZoneCarrierInputs(frames, model, checks)
			if err != nil {
				t.Fatal(err)
			}
			key := epathSQLZoneCarrierContext("zone 1", "M1")
			context := parts[key]
			p := context["direct/fans"]
			switch mutation {
			case "allocated":
				p.Direct = false
			case "unavailable":
				p.Unavailable = true
			case "annual":
				p.AnnualTabular = true
			case "hidden family":
				context["service/heating"] = p
			case "foreign carrier":
				p.ByCarrier = map[string]epathSQLQuantity{"natural_gas": {}}
			case "missing known-zero fan":
				delete(context, "direct/fans")
			case "missing period":
				delete(parts, key)
			}
			if mutation != "missing known-zero fan" && mutation != "missing period" {
				context["direct/fans"] = p
			}
			if active, err := epathSQLSimpleVentilationPlainCarrierIDs(frames, model, parts, observed); active || err == nil {
				t.Fatal("unreviewed carrier component retained plain-ID eligibility")
			}
		})
	}
}

func epathSQLSimpleVentilationCarrierRowUnitBundle(period string) PurposeResultBundle {
	value := 2.0
	if period == "annual" {
		value = 24
	}
	row := EnergyReconciliation{ID: "reconcile.energy.electricity." + period, Level: "energy", ZoneName: "ZONE 2", Period: period, Unit: "kWh", Basis: "direct_zone_energy", Status: "partial", ExpectedValue: value, ExplainedValue: value}
	zone := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "ZONE 2"}, Periods: []EnergyPeriod{{ID: period, Kind: "monthly", Reconciliation: []EnergyReconciliation{row}}}}
	if period == "annual" {
		zone.Periods[0].Kind = "annual"
		zone.Reconciliation = []EnergyReconciliation{row}
	}
	return PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, ZoneResults: []EnergyExplanationZoneResult{zone}}}
}

func TestEnergyPathSQLSimpleVentilationCarrierPlainIDKeepsWholeRowAndAnnualCopiesStrict(t *testing.T) {
	observed, model, frames, checks := epathSQLSimpleVentilationCarrierIDUnitInputs(t)
	if err := epathSQLModelZoneCarrierChecks(frames, model, &checks, observed); err != nil {
		t.Fatal(err)
	}
	for _, period := range []string{"M1", "annual"} {
		var selected epathSQLModelCheck
		for _, c := range checks.Rows {
			if c.Reconciliation != nil && c.Item.Zone == "ZONE 2" && c.Item.Period == period && c.Item.Target.Field == "expectedValue" {
				selected = c
			}
		}
		for _, mutation := range []string{"", "legacy qualified ID", "wrong Zone", "wrong period", "wrong basis", "wrong carrier", "wrong status", "missing", "duplicate", "unselected scalar", "annual canonical only", "annual wrapper only"} {
			t.Run(period+"/"+mutation, func(t *testing.T) {
				if strings.HasPrefix(mutation, "annual ") && period != "annual" {
					t.Skip("annual-copy boundary")
				}
				b := epathSQLSimpleVentilationCarrierRowUnitBundle(period)
				z := &b.EnergyExplanation.ZoneResults[0]
				r := &z.Periods[0].Reconciliation[0]
				switch mutation {
				case "legacy qualified ID":
					r.ID += ".zone_2"
				case "wrong Zone":
					r.ZoneName = "ZONE 3"
				case "wrong period":
					r.Period = "M2"
				case "wrong basis":
					r.Basis = "service_path_allocation"
				case "wrong carrier":
					r.ID = strings.Replace(r.ID, "electricity", "natural_gas", 1)
				case "wrong status":
					r.Status = "balanced"
				case "missing":
					z.Periods[0].Reconciliation = nil
				case "duplicate":
					z.Periods[0].Reconciliation = append(z.Periods[0].Reconciliation, *r)
				case "unselected scalar":
					r.ExplainedValue++
				case "annual canonical only":
					z.Reconciliation[0].ExpectedValue++
				case "annual wrapper only":
					r.ResidualValue = 1
				}
				err := epathCheckSQLModelReconciliation(b, selected)
				if (err != nil) != (mutation != "") {
					t.Fatalf("whole-row/identity validation %q: %v", mutation, err)
				}
			})
		}
	}
}
