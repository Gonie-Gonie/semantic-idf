package simulation

import (
	"fmt"
	"strings"
	"testing"
)

func epathSQLRadiantCarrierFixture(t *testing.T) (epathSQLFrames, epathRealSQLModel, PurposeResultBundle) {
	t.Helper()
	frames, model, bundle := epathSQLHeatRejectionFixture(t)
	model.Site, model.Auxiliaries = nil, nil
	model.Services = []epathRealSQLService{
		{Service: "cooling", SiteIDs: []string{"cooling.electricity", "cooling.district_cooling"}, ServedZones: []string{"A", "B", "C"}, Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: "load_to_site_energy", FallbackRatioKind: "load_to_site_energy", CarrierReconciliationIDs: map[string]string{
			"electricity": "reconcile.zone_hvac_allocation.cooling.electricity.annual", "district_cooling": "reconcile.zone_hvac_allocation.cooling.district_cooling.annual"}},
		{Service: "heating", SiteIDs: []string{"heating.district_heating"}, ServedZones: []string{"A", "B", "C"}, Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: "load_to_site_energy", FallbackRatioKind: "load_to_site_energy", ReconciliationID: "reconcile.zone_hvac_allocation.heating.district_heating.annual"},
	}
	// The pre-existing literal native fixture has cooling only in M1/M2;
	// heating remains positive in M3. These independent meter constants make
	// electricity M2 genuinely zero despite positive cooling load, and M3
	// consumption unassigned despite positive heating load.
	for i, part := range []struct {
		service, carrier, name string
		values                 [12]float64
	}{
		{"cooling", "electricity", "Cooling:Electricity", [12]float64{60, 0, 30}},
		{"cooling", "district_cooling", "Cooling:DistrictCooling", [12]float64{90, 30, 45}},
		{"heating", "district_heating", "Heating:DistrictHeatingWater", [12]float64{}},
	} {
		index, id := 11+i, part.service+"."+part.carrier
		source := epathRealSQLSource{DictionaryIndex: index, Name: part.name, IsMeter: true, ReportingFrequency: "Monthly", SourceUnit: "J", Rows: 12}
		raw, total := 0.0, 0.0
		for month, value := range part.values {
			source.Months = append(source.Months, epathRealSQLMonth{Month: month + 1, Rows: 1, RawSum: epathOracleNumber(value * 3600000), EnergyKWh: epathOracleNumber(value)})
			raw, total = raw+value*3600000, total+value
		}
		source.RawSum, source.EnergyKWh = epathOracleNumber(raw), epathOracleNumber(total)
		frames.SourceIdentities[index] = source
		monthly, err := epathSQLMonthly(source, model.Precision)
		if err != nil {
			t.Fatal(err)
		}
		frames.SourceRaw[index], frames.SiteSources[id] = monthly, []int{index}
		frames.Site[id] = nil
		for _, q := range monthly {
			copy := q
			frames.Site[id] = append(frames.Site[id], &copy)
		}
		model.Site = append(model.Site, epathRealSQLSite{ID: id, EndUse: part.service, Carrier: part.carrier, Source: epathRealSQLSelector{IsMeter: true, Keys: []string{""}, Alternatives: []epathRealSQLAlternative{{Name: part.name, Unit: "J"}}}})
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", index), Name: part.name, IsMeter: true, SourceType: "sql_report_data", Units: "J", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", AggregationMethod: "sum", AggregationBasis: "model_total", RawValue: total, EffectiveValue: total, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total"})
	}
	for _, period := range epathSQLZoneCarrierPeriods() {
		row := EnergyPeriod{ID: period, Kind: "monthly"}
		if period == "annual" {
			row.Kind = "annual"
		}
		for i, carrier := range []string{"electricity", "district_cooling"} {
			expected, assigned, unassigned := 0.0, 0.0, 0.0
			switch period {
			case "M1":
				expected, assigned = []float64{60, 90}[i], []float64{60, 90}[i]
			case "M2":
				expected, assigned = []float64{0, 30}[i], []float64{0, 30}[i]
			case "M3":
				expected, unassigned = []float64{30, 45}[i], []float64{30, 45}[i]
			case "annual":
				expected, assigned, unassigned = []float64{90, 165}[i], []float64{60, 120}[i], []float64{30, 45}[i]
			}
			if expected == 0 {
				continue // Known all-zero carrier row is legitimately pruned.
			}
			method, sources := "unassigned", []string{fmt.Sprintf("sql-rdd-%d", 11+i)}
			if assigned > 0 {
				method = "service_path_load_share"
				sources = append(sources, "sql-rdd-101", "sql-rdd-103", "sql-rdd-105")
			}
			row.Reconciliation = append(row.Reconciliation, EnergyReconciliation{ID: "reconcile.zone_hvac_allocation.cooling." + carrier + "." + strings.ToLower(period), Level: "allocation", Period: period, ServiceKind: "cooling", Basis: "service_path_allocation", Unit: "kWh", AllocationMethod: method, ExpectedValue: expected, AllocatedValue: assigned, UnassignedValue: unassigned, SourceIDs: sources})
		}
		bundle.EnergyExplanation.Periods = append(bundle.EnergyExplanation.Periods, row)
		if period == "annual" {
			bundle.EnergyExplanation.Reconciliation = row.Reconciliation
		}
	}
	return frames, model, bundle
}

func epathSQLRadiantCarrierChecks(t *testing.T, frames epathSQLFrames, model epathRealSQLModel) epathSQLModelChecks {
	t.Helper()
	all, selected := epathSQLModelChecks{}, epathSQLModelChecks{}
	if err := epathSQLModelServiceChecks(frames, model, &all); err != nil {
		t.Fatal(err)
	}
	for _, check := range all.Rows {
		if check.Allocation != nil && check.Allocation.RadiantCarrier != nil {
			selected.Rows = append(selected.Rows, check)
		}
	}
	return selected
}

func TestEnergyPathRealSQLRadiantCarrierLedgerMonthlyPoolsAndConsumers(t *testing.T) {
	frames, model, bundle := epathSQLRadiantCarrierFixture(t)
	checks := epathSQLRadiantCarrierChecks(t, frames, model)
	if len(checks.Rows) != 2*13*4 {
		t.Fatalf("lost independent per-carrier monthly/annual fields: %d", len(checks.Rows))
	}
	for _, check := range checks.Rows {
		if check.Allocation.Direct.Value != 0 || check.Allocation.NativeVRF != nil {
			t.Fatal("broad shared service became measured direct or native VRF consumption")
		}
		if check.Item.Period == "annual" {
			want := map[string]float64{"electricity": 60, "district_cooling": 120}[check.Allocation.RadiantCarrier.Carrier]
			if check.Allocation.Allocated.Value != want {
				t.Fatal("annual allocation was recomputed from annual C/H weights or multiplied by 21")
			}
		}
	}
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) != 0 {
		t.Fatalf("literal independent shared carrier ledger failed: %v", failures[:min(5, len(failures))])
	}
}

func TestEnergyPathRealSQLRadiantCarrierLedgerRejectsDeclarationsAndUnknowns(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*epathSQLFrames, *epathRealSQLModel)
	}{
		{"wrong_carrier", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Site[1].Carrier = "natural_gas" }},
		{"wrong_meter", func(_ *epathSQLFrames, m *epathRealSQLModel) {
			m.Site[1].Source.Alternatives[0].Name = "Cooling:Electricity"
		}},
		{"wrong_service", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Site[1].EndUse = "heating" }},
		{"duplicate_site", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Services[0].SiteIDs[1] = m.Services[0].SiteIDs[0] }},
		{"swapped_row_identity", func(_ *epathSQLFrames, m *epathRealSQLModel) {
			m.Services[0].CarrierReconciliationIDs["electricity"] = m.Services[0].CarrierReconciliationIDs["district_cooling"]
		}},
		{"combined_row_identity", func(_ *epathSQLFrames, m *epathRealSQLModel) {
			m.Services[0].ReconciliationID = "allocation.cooling.annual"
		}},
		{"owner_subset", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Services[0].ServedZones = []string{"A", "B"} }},
		{"owner_unknown", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Services[0].ServedZones[2] = "Unknown" }},
		{"owner_duplicate", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Services[0].ServedZones[2] = "A" }},
		{"not_native", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Loads[0].NativeRadiant = nil }},
		{"missing_zero_site", func(f *epathSQLFrames, _ *epathRealSQLModel) { f.Site["cooling.electricity"][1] = nil }},
		{"missing_meter_month", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			s := f.SourceIdentities[11]
			s.Months[1].EnergyKWh = nil
			f.SourceIdentities[11] = s
		}},
		{"invalid_meter_raw", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			s := f.SourceIdentities[11]
			s.Months[1].RawSum = nil
			f.SourceIdentities[11] = s
		}},
		{"multiplied_pool", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			q := f.Site["cooling.electricity"][0].times(21)
			f.Site["cooling.electricity"][0] = &q
		}},
		{"heating_weight", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			f.LoadSourceIDs[epathSQLKey("A", "cooling", 1)] = []int{102}
		}},
		{"missing_zero_load", func(f *epathSQLFrames, _ *epathRealSQLModel) { delete(f.Loads, epathSQLKey("A", "cooling", 3)) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			frames, model, _ := epathSQLRadiantCarrierFixture(t)
			test.mutate(&frames, &model)
			if _, err := epathSQLCompileRadiantCarrierLedger(frames, model, model.Services[0]); err == nil {
				t.Fatal("mismatched carrier, owner, source or unknown monthly quantity passed")
			}
		})
	}
}

func TestEnergyPathRealSQLRadiantCarrierLedgerRejectsCandidateAndProofMutants(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*PurposeResultBundle, *epathSQLModelChecks)
	}{
		{"carrier_id", func(b *PurposeResultBundle, _ *epathSQLModelChecks) {
			b.EnergyExplanation.Reconciliation[0].ID = "wrong.cooling.electricity.annual"
		}},
		{"heating_metadata", func(b *PurposeResultBundle, _ *epathSQLModelChecks) {
			b.EnergyExplanation.Reconciliation[0].ServiceKind = "heating"
		}},
		{"direct_method", func(b *PurposeResultBundle, _ *epathSQLModelChecks) {
			b.EnergyExplanation.Reconciliation[0].AllocationMethod = "direct_only"
		}},
		{"direct_basis", func(b *PurposeResultBundle, _ *epathSQLModelChecks) {
			b.EnergyExplanation.Reconciliation[0].Basis = "direct_zone_energy"
		}},
		{"wrong_zone", func(b *PurposeResultBundle, _ *epathSQLModelChecks) {
			b.EnergyExplanation.Reconciliation[0].ZoneName = "A"
		}},
		{"other_carrier_trace", func(b *PurposeResultBundle, _ *epathSQLModelChecks) {
			b.EnergyExplanation.Reconciliation[0].SourceIDs[0] = "sql-rdd-12"
		}},
		{"heating_weight_trace", func(b *PurposeResultBundle, _ *epathSQLModelChecks) {
			b.EnergyExplanation.Reconciliation[0].SourceIDs[1] = "sql-rdd-102"
		}},
		{"lost_weight_trace", func(b *PurposeResultBundle, _ *epathSQLModelChecks) {
			b.EnergyExplanation.Reconciliation[0].SourceIDs = b.EnergyExplanation.Reconciliation[0].SourceIDs[:1]
		}},
		{"unassigned_m3_to_allocated", func(b *PurposeResultBundle, _ *epathSQLModelChecks) {
			for i := range b.EnergyExplanation.Periods {
				if b.EnergyExplanation.Periods[i].ID == "M3" {
					row := &b.EnergyExplanation.Periods[i].Reconciliation[0]
					row.AllocatedValue, row.UnassignedValue = row.UnassignedValue, 0
				}
			}
		}},
		{"candidate_and_compiled_direct_laundering", func(b *PurposeResultBundle, c *epathSQLModelChecks) {
			b.EnergyExplanation.Reconciliation[0].DirectValue = 1
			b.EnergyExplanation.Reconciliation[0].AllocatedValue--
			for i := range c.Rows {
				check := &c.Rows[i]
				if check.Item.Period != "annual" || check.Allocation.RadiantCarrier.Carrier != "electricity" {
					continue
				}
				direct, allocated := epathSQLQuantity{Value: 1}, epathSQLQuantity{Value: 59}
				check.Allocation.Direct, check.Allocation.Allocated = &direct, &allocated
				if check.Item.Target.Field == "directValue" {
					check.Quantity = &direct
				}
				if check.Item.Target.Field == "allocatedValue" {
					check.Quantity = &allocated
				}
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			frames, model, bundle := epathSQLRadiantCarrierFixture(t)
			checks := epathSQLRadiantCarrierChecks(t, frames, model)
			test.mutate(&bundle, &checks)
			if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) == 0 {
				t.Fatal("shared carrier ledger numeric/source/context mutation passed")
			}
		})
	}
}

func TestEnergyPathRealSQLRadiantCarrierLedgerDoesNotReplaceDirectGuard(t *testing.T) {
	_, model, _ := epathSQLRadiantCarrierFixture(t)
	if err := epathSQLDirectHVACRequireCarrierLedger(model.Services[0], nil); err == nil {
		t.Fatal("ordinary direct-service guard was weakened for shared carrier declarations")
	}
	model.Services[0].CarrierReconciliationIDs = nil
	if err := epathSQLDirectHVACRequireCarrierLedger(model.Services[0], nil); err != nil {
		t.Fatal("ordinary one-carrier service behavior changed")
	}
}
