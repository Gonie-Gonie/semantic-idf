package simulation

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestEnergyPathRealSQLNativeFanLedgerOriginalSourcesAndMethod(t *testing.T) {
	path, model, _ := epathSQLNativeZoneEquipmentUnit(t)
	frames, _ := epathSQLDirectHVACSourceUnitFrames(t, path, model)
	var checks epathSQLModelChecks
	if err := epathSQLModelDirectFanChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Sources: []EnergyDataSource{
		epathSQLDirectHVACSourceUnitCandidate(frames.DirectHVACSourceIdentities[60]),
		{ID: "sql-rdd-62", Name: "Fans:Electricity", IsMeter: true, SourceType: "sql_report_data", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly"},
	}}}
	for _, period := range epathSQLZoneCarrierPeriods() {
		row := EnergyPeriod{ID: period, Kind: "monthly"}
		if period == "annual" {
			row.Kind = "annual"
		}
		if period == "M1" || period == "annual" {
			row.Reconciliation = []EnergyReconciliation{{ID: "reconcile.zone_auxiliary_allocation.fans.electricity.m1", Level: "allocation", Period: period, ServiceKind: "fans", Basis: "service_path_allocation", Unit: "kWh", AllocationMethod: "direct_only", ExpectedValue: 10, DirectValue: 10, SourceIDs: []string{"sql-rdd-60", "sql-rdd-62"}}}
			if period == "annual" {
				row.Reconciliation[0].ID = "reconcile.zone_auxiliary_allocation.fans.electricity.annual"
				bundle.EnergyExplanation.Reconciliation = row.Reconciliation
			}
		}
		bundle.EnergyExplanation.Periods = append(bundle.EnergyExplanation.Periods, row)
	}
	var selected, zero epathSQLModelCheck
	for _, check := range checks.Rows {
		if check.Allocation == nil {
			continue
		}
		if err := epathCheckSQLModelAllocation(bundle, check); err != nil {
			t.Fatalf("%s: %v", check.Want.Key, err)
		}
		if check.Item.Period == "M1" {
			selected = check
		}
		if check.Item.Period == "M2" {
			zero = check
		}
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"source-missing", "source-foreign", "source-duplicate", "wrong-method", "invented-shared", "overlap", "duplicate-row"} {
		var changed PurposeResultBundle
		if err := json.Unmarshal(encoded, &changed); err != nil {
			t.Fatal(err)
		}
		var row *EnergyReconciliation
		for i := range changed.EnergyExplanation.Periods {
			if changed.EnergyExplanation.Periods[i].ID == "M1" {
				row = &changed.EnergyExplanation.Periods[i].Reconciliation[0]
			}
		}
		switch mutation {
		case "source-missing":
			row.SourceIDs = []string{"sql-rdd-62"}
		case "source-foreign":
			row.SourceIDs = append(row.SourceIDs, "sql-rdd-40")
		case "source-duplicate":
			row.SourceIDs = append(row.SourceIDs, "sql-rdd-60")
		case "wrong-method":
			row.AllocationMethod = "service_path_load_share"
		case "invented-shared":
			row.DirectValue = 0
			row.AllocatedValue = 10
		case "overlap":
			row.OvermappedValue = 0.001
		case "duplicate-row":
			for i := range changed.EnergyExplanation.Periods {
				if changed.EnergyExplanation.Periods[i].ID == "M1" {
					changed.EnergyExplanation.Periods[i].Reconciliation = append(changed.EnergyExplanation.Periods[i].Reconciliation, *row)
				}
			}
		}
		if err := epathCheckSQLModelAllocation(changed, selected); err == nil {
			t.Fatalf("native fan ledger accepted %s", mutation)
		}
	}
	bundle.EnergyExplanation.Sources = bundle.EnergyExplanation.Sources[1:]
	if err := epathCheckSQLModelAllocation(bundle, zero); err == nil {
		t.Fatal("pruned zero ledger accepted absent native observation")
	}
}

func TestEnergyPathRealSQLNativeFanLedgerRetainsMonthlyRoundingOverlap(t *testing.T) {
	path, model, _ := epathSQLNativeZoneEquipmentUnit(t)
	frames, _ := epathSQLDirectHVACSourceUnitFrames(t, path, model)
	fans, err := epathSQLCompileDirectFans(frames, model)
	if err != nil {
		t.Fatal(err)
	}
	// Original literal readings: two native consumers each .0006/.0004;
	// broad meter .0012/.0008. Opposite monthly serialization debts must
	// remain overmapped=.001 AND unassigned=.001, not cancel annually.
	set := func(source epathRealSQLSource, values [12]float64) epathRealSQLSource {
		source.Months = nil
		raw, total := 0.0, 0.0
		for i, v := range values {
			source.Months = append(source.Months, epathRealSQLMonth{Month: i + 1, Rows: 1, RawSum: epathOracleNumber(v * 3600000), EnergyKWh: epathOracleNumber(v)})
			raw += v * 3600000
			total += v
		}
		source.RawSum, source.EnergyKWh = epathOracleNumber(raw), epathOracleNumber(total)
		return source
	}
	fans.MeterSource = set(fans.MeterSource, [12]float64{.0012, .0008})
	meterMonths, err := epathSQLMonthly(fans.MeterSource, model.Precision)
	if err != nil {
		t.Fatal(err)
	}
	for i, q := range meterMonths {
		fans.Meter[i] = q.positive()
	}
	fans.Sources = map[string]map[string]epathSQLDirectHVACSourceIdentity{}
	fans.Zones = map[string][12]epathSQLQuantity{}
	for i, zone := range []string{"office", "other"} {
		identity := frames.DirectHVACSourceIdentities[60]
		identity.Source = set(identity.Source, [12]float64{.0006, .0004})
		identity.Source.DictionaryIndex = 60 + i
		identity.Source.KeyValue = fmt.Sprintf("Fan %d", i)
		identity.Owner.KeyValue = identity.Source.KeyValue
		identity.Owner.ZoneName = zone
		identity.Owner.EquipmentName = fmt.Sprintf("Package %d", i)
		fans.Sources[zone] = map[string]epathSQLDirectHVACSourceIdentity{fmt.Sprintf("sql-rdd-%d", 60+i): identity}
		zoneMonths, err := epathSQLMonthly(identity.Source, identity.Precision)
		if err != nil {
			t.Fatal(err)
		}
		var months [12]epathSQLQuantity
		for i, q := range zoneMonths {
			months[i] = q.positive()
		}
		fans.Zones[zone] = months
	}
	want, _, err := epathSQLDirectFanLedgerPresentation(fans, "annual")
	if err != nil {
		t.Fatal(err)
	}
	if want.ExpectedValue != .002 || want.DirectValue != .002 || want.UnassignedValue != .001 || want.OvermappedValue != .001 || want.AllocationMethod != "mixed" {
		t.Fatalf("monthly rounding overlap was netted away: %+v", want)
	}
}

func TestEnergyPathRealSQLNativeFanCarrierProofBoundToOriginalRoster(t *testing.T) {
	path, model, _ := epathSQLNativeZoneEquipmentUnit(t)
	frames, _ := epathSQLDirectHVACSourceUnitFrames(t, path, model)
	// Isolate the fan consumer; unproved cooling/heating are not candidate
	// carrier components in this bounded mathematical test.
	model.Services = nil
	model.DirectUses = nil
	var checks epathSQLModelChecks
	if err := epathSQLModelDirectFanChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if _, _, err := epathSQLZoneCarrierInputs(frames, model, checks); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"quantity", "source-roster", "owner", "basis", "duplicate", "missing"} {
		changed := checks
		changed.Rows = append([]epathSQLModelCheck(nil), checks.Rows...)
		for i := range changed.Rows {
			row := &changed.Rows[i]
			if row.DirectFan == nil || row.Item.Period != "M1" || row.Item.Target.Field != "value" {
				continue
			}
			proof := *row.DirectFan
			row.DirectFan = &proof
			switch mutation {
			case "quantity":
				q := *row.Quantity
				q.Value *= 32
				row.Quantity = &q
			case "source-roster":
				proof.Sources = nil
				proof.Value = epathSQLQuantity{}
			case "owner":
				proof.Zone = "Other"
			case "basis":
				row.Item.Target.Basis = "service_path_allocation"
			case "duplicate":
				changed.Rows = append(changed.Rows, *row)
			case "missing":
				changed.Rows = append(changed.Rows[:i], changed.Rows[i+1:]...)
			}
			break
		}
		if _, _, err := epathSQLZoneCarrierInputs(frames, model, changed); err == nil {
			t.Fatalf("carrier accepted %s", mutation)
		}
	}
}
