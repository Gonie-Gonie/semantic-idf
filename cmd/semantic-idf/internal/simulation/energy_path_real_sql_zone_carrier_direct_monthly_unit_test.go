package simulation

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func epathSQLDirectMonthlyCarrierUnitInputs(t *testing.T) (epathSQLFrames, epathRealSQLModel, epathSQLModelChecks) {
	t.Helper()
	frames, model := epathSQLAnnualZoneUnitFrames(t)
	sources, directFrames, directModel := epathDirectUseUnitInputs()
	frames.Zones, model.DirectUses = directFrames.Zones, directModel.DirectUses
	for i := range sources {
		sources[i].DictionaryIndex += 100
	}
	// Independent original direct observations are A2 * multiplier10 and B1.
	// Their carrier has a monthly meter, while both HVAC carriers are annual.
	model.Site = append(model.Site, epathRealSQLSite{ID: "lighting.e", EndUse: "lighting", Carrier: "electricity"})
	for month := 1; month <= 12; month++ {
		frames.Site["lighting.e"] = append(frames.Site["lighting.e"], &epathSQLQuantity{Value: 21})
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelDirectUseChecks(sources, frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return frames, model, checks
}

func epathSQLDirectMonthlyCarrierUnitBundle() PurposeResultBundle {
	bundle := epathSQLAnnualZoneUnitBundle()
	// The shared hand fixture builds its source roster from a map. Give two
	// independently constructed fixtures the same order for immutability proof.
	sort.Slice(bundle.EnergyExplanation.Sources, func(i, j int) bool {
		return bundle.EnergyExplanation.Sources[i].ID < bundle.EnergyExplanation.Sources[j].ID
	})
	for i := range bundle.EnergyExplanation.ZoneResults {
		zone := &bundle.EnergyExplanation.ZoneResults[i]
		if zone.Scope.ZoneName == "Plenum" {
			continue
		}
		for p := range zone.Periods {
			period := &zone.Periods[p]
			value := 20.0
			if zone.Scope.ZoneName == "B" {
				value = 1
			}
			if period.ID == "annual" {
				value *= 12
			}
			period.Nodes = append(period.Nodes, EnergyExplanationNode{ID: "carrier.electricity", Level: "carrier", Carrier: "electricity", EndUse: "total", Value: value, AllocatedValue: value, AllocationApplied: true, Unit: "kWh", ScaleDomain: "site", Basis: "direct_zone_energy", AggregationBasis: "model_total", ZoneName: zone.Scope.ZoneName, Period: period.ID})
			period.Reconciliation = append(period.Reconciliation, EnergyReconciliation{ID: "reconcile.energy.electricity." + period.ID, Level: "energy", ZoneName: zone.Scope.ZoneName, Period: period.ID, Unit: "kWh", Basis: "direct_zone_energy", Status: "partial", ExpectedValue: value, ExplainedValue: value})
			if period.ID == "annual" {
				zone.Nodes = append([]EnergyExplanationNode(nil), period.Nodes...)
				zone.Reconciliation = append([]EnergyReconciliation(nil), period.Reconciliation...)
			}
		}
	}
	return bundle
}

func epathSQLDirectMonthlyCarrierUnitChecks(t *testing.T) epathSQLModelChecks {
	t.Helper()
	frames, model, checks := epathSQLDirectMonthlyCarrierUnitInputs(t)
	start := len(checks.Rows)
	if err := epathSQLModelZoneCarrierChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return epathSQLModelChecks{Rows: checks.Rows[start:]}
}

func TestEnergyPathRealSQLZoneCarrierDirectMonthlyPlainIDs(t *testing.T) {
	checks := epathSQLDirectMonthlyCarrierUnitChecks(t)
	if len(checks.Rows) != 3*13*3*5 {
		t.Fatalf("carrier identity mode changed dependency/field obligations: %d", len(checks.Rows))
	}
	bundle := epathSQLDirectMonthlyCarrierUnitBundle()
	before := epathSQLDirectMonthlyCarrierUnitBundle()
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) != 0 {
		t.Fatalf("independent annual HVAC plus direct monthly electricity: %v", failures[:min(8, len(failures))])
	}
	if !reflect.DeepEqual(bundle, before) {
		t.Fatal("plain-ID proof altered original candidate")
	}
	plainRows := 0
	for _, check := range checks.Rows {
		if check.Reconciliation == nil || !strings.Contains(check.Want.Key, "zone_subtotal/electricity/") {
			continue
		}
		plainRows++
		wantID := "reconcile.energy.electricity." + check.Item.Period
		if check.Reconciliation.ID != wantID || !reflect.DeepEqual(check.Reconciliation.AllowedIDs, []string{wantID}) {
			t.Fatalf("direct mode must permit exactly one independently derived ID: %+v", check.Reconciliation)
		}
		if check.Item.Target.Field == "expectedValue" && check.Item.Zone == "A" {
			value := 20.0
			if check.Item.Period == "annual" {
				value = 240
			}
			if check.Quantity == nil || check.Quantity.Value != value {
				t.Fatal("identity correction changed independent SQL multiplier/month-first quantity")
			}
		}
	}
	if plainRows != 3*13*3 {
		t.Fatalf("missing direct carrier reconciliation fields: %d", plainRows)
	}
}

func TestEnergyPathRealSQLZoneCarrierDirectMonthlyStillRejectsWrongIdentity(t *testing.T) {
	checks := epathSQLDirectMonthlyCarrierUnitChecks(t)
	for _, mutation := range []string{"qualified monthly", "qualified annual", "wrong Zone", "wrong period", "duplicate", "changed quantity", "plain annual district"} {
		t.Run(mutation, func(t *testing.T) {
			bundle := epathSQLDirectMonthlyCarrierUnitBundle()
			zone := &bundle.EnergyExplanation.ZoneResults[0]
			row := &zone.Periods[1].Reconciliation[0]
			switch mutation {
			case "qualified monthly":
				row.ID += ".a"
			case "qualified annual":
				zone.Periods[0].Reconciliation[2].ID = "reconcile.energy.electricity.M1.a.annual"
			case "wrong Zone":
				row.ZoneName = "B"
			case "wrong period":
				row.Period = "M2"
			case "duplicate":
				zone.Periods[1].Reconciliation = append(zone.Periods[1].Reconciliation, *row)
			case "changed quantity":
				row.ExpectedValue++
			case "plain annual district":
				zone.Periods[0].Reconciliation[0].ID = "reconcile.energy.district_cooling.annual"
			}
			if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) == 0 {
				t.Fatal("direct-only ID mode weakened exact context, quantity, or annual-source grammar")
			}
		})
	}
}

func TestEnergyPathRealSQLZoneCarrierDirectMonthlyModeRequiresOriginalInputs(t *testing.T) {
	for _, mutation := range []string{"", "fan pool", "allocated auxiliary", "unassigned auxiliary", "monthly other-carrier service", "mixed reporting service", "missing monthly observation", "no direct declaration"} {
		t.Run(mutation, func(t *testing.T) {
			frames, model, _ := epathSQLDirectMonthlyCarrierUnitInputs(t)
			wantMode, wantError := true, false
			switch mutation {
			case "fan pool":
				model.FanPools = []epathRealSQLFanPool{{SiteID: "other.pool"}}
				wantMode = false
			case "allocated auxiliary":
				model.Auxiliaries = []epathRealSQLAuxiliary{{Weight: "cooling_plus_heating"}}
				wantMode = false
			case "unassigned auxiliary":
				model.Auxiliaries = []epathRealSQLAuxiliary{{Weight: "unassigned"}}
			case "monthly other-carrier service":
				delete(frames.SiteAnnual, "heating.district")
				frames.Site["heating.district"] = frames.Site["lighting.e"]
				wantMode = false
			case "mixed reporting service":
				model.Services[0].SiteIDs = append(model.Services[0].SiteIDs, "lighting.e")
				wantError = true
			case "missing monthly observation":
				frames.Site["lighting.e"][0] = nil
				wantError = true
			case "no direct declaration":
				model.DirectUses = nil
				wantMode = false
			}
			mode, err := epathSQLZoneCarrierPlainMonthlyIDs(frames, model)
			if (err != nil) != wantError || err == nil && (mode["electricity"] != wantMode || mode["district_cooling"] || mode["district_heating"]) {
				t.Fatalf("unproved direct-only identity mode %q: %v/%v", mutation, mode, err)
			}
		})
	}
}

func TestEnergyPathRealSQLZoneCarrierAllocatedModelsStillRejectPlainIDs(t *testing.T) {
	checks := epathSQLZoneCarrierUnitChecks(t)
	for _, periodIndex := range []int{0, 1} {
		bundle := epathSQLZoneCarrierUnitBundle()
		period := &bundle.EnergyExplanation.ZoneResults[0].Periods[periodIndex]
		period.Reconciliation[0].ID = "reconcile.energy.electricity." + period.ID
		if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) == 0 {
			t.Fatal("allocation-backed model accepted an unproved plain accounting identity")
		}
	}
}
