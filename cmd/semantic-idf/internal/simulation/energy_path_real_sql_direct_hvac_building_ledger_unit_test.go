package simulation

import (
	"fmt"
	"testing"
)

func epathSQLDirectHVACBuildingLedgerFixture() (epathRealSQLService, *epathSQLDirectHVACServiceFrames) {
	service := epathRealSQLService{Service: "heating", CarrierReconciliationIDs: map[string]string{
		"electricity": "reconcile.zone_hvac_allocation.heating.electricity.annual",
		"natural_gas": "reconcile.zone_hvac_allocation.heating.natural_gas.annual",
	}}
	direct := &epathSQLDirectHVACServiceFrames{Carriers: []string{"electricity", "natural_gas"}}
	q := func(value float64) epathSQLQuantity { return epathSQLQuantity{Value: value} }
	for month := 0; month < 12; month++ {
		electric := epathSQLDirectHVACCarrierLedger{Site: q(10), Direct: q(4), Allocated: q(3), Unassigned: q(3)}
		gas := epathSQLDirectHVACCarrierLedger{Site: q(20), Direct: q(7), Allocated: q(5), Unassigned: q(8)}
		if month == 6 {
			gas = epathSQLDirectHVACCarrierLedger{} // Known zero remains an explicit partition.
		}
		direct.Monthly[month] = epathSQLDirectHVACServiceMonth{
			Site: electric.Site.add(gas.Site), Direct: electric.Direct.add(gas.Direct),
			Allocated: electric.Allocated.add(gas.Allocated), Unassigned: electric.Unassigned.add(gas.Unassigned),
			ByCarrier: map[string]epathSQLDirectHVACCarrierLedger{"electricity": electric, "natural_gas": gas},
			Zones: map[string]map[string]epathSQLDirectHVACShare{"owner": {
				"electricity": {Direct: electric.Direct, Allocated: electric.Allocated},
				"natural_gas": {Direct: gas.Direct, Allocated: gas.Allocated},
			}},
		}
	}
	return service, direct
}

func TestEnergyPathRealSQLDirectHVACBuildingLedgerPartitions(t *testing.T) {
	service, direct := epathSQLDirectHVACBuildingLedgerFixture()
	if err := epathSQLDirectHVACRequireCarrierLedger(service, direct); err != nil {
		t.Fatal(err)
	}
	legacy := service
	legacy.CarrierReconciliationIDs = nil
	legacy.ReconciliationID = "reconcile.zone_hvac_allocation.heating.natural_gas.annual"
	if err := epathSQLDirectHVACRequireCarrierLedger(legacy, direct); err == nil {
		t.Fatal("combined multi-carrier proof was accepted against a single fuel row")
	}
	if err := epathSQLDirectHVACRequireCarrierLedger(service, nil); err == nil {
		t.Fatal("declared carrier partitions accepted without original direct frames")
	}
	checks := epathSQLModelChecks{}
	for _, period := range []string{"M1", "M7", "annual"} {
		if err := epathSQLDirectHVACBuildingLedgerChecks(service, direct, period, &checks); err != nil {
			t.Fatal(err)
		}
	}
	if len(checks.Rows) != 24 {
		t.Fatalf("lost per-carrier whole-row proofs: %d", len(checks.Rows))
	}
	for _, check := range checks.Rows {
		if check.Allocation == nil || check.Quantity == nil {
			t.Fatal("partial numeric field cannot stand in for a whole carrier row")
		}
		carrier := "electricity"
		if check.Item.Target.ID == "reconcile.zone_hvac_allocation.heating.natural_gas."+map[string]string{"M1": "m1", "M7": "m7", "annual": "annual"}[check.Item.Period] {
			carrier = "natural_gas"
		}
		want := 10.0
		if carrier == "natural_gas" {
			want = 20
			if check.Item.Period == "M7" {
				want = 0
			}
		}
		if check.Item.Period == "annual" {
			want = 120
			if carrier == "natural_gas" {
				want = 220
			}
		}
		if check.Item.Target.Field == "expectedValue" && check.Quantity.Value != want {
			t.Fatalf("combined service total replaced %s/%s: %g != %g", carrier, check.Item.Period, check.Quantity.Value, want)
		}
	}
}

func TestEnergyPathRealSQLDirectHVACBuildingLedgerRejectsLostOrCrossedPartitions(t *testing.T) {
	for name, mutate := range map[string]func(*epathRealSQLService, *epathSQLDirectHVACServiceFrames){
		"missing ID": func(s *epathRealSQLService, d *epathSQLDirectHVACServiceFrames) {
			delete(s.CarrierReconciliationIDs, "natural_gas")
		},
		"crossed ID": func(s *epathRealSQLService, d *epathSQLDirectHVACServiceFrames) {
			s.CarrierReconciliationIDs["natural_gas"] = s.CarrierReconciliationIDs["electricity"]
		},
		"ambiguous combined ID": func(s *epathRealSQLService, d *epathSQLDirectHVACServiceFrames) {
			s.ReconciliationID = "heating.annual"
		},
		"missing known zero": func(s *epathRealSQLService, d *epathSQLDirectHVACServiceFrames) {
			delete(d.Monthly[6].ByCarrier, "natural_gas")
		},
		"missing Zone share": func(s *epathRealSQLService, d *epathSQLDirectHVACServiceFrames) {
			delete(d.Monthly[0].Zones["owner"], "natural_gas")
		},
		"wrong service total": func(s *epathRealSQLService, d *epathSQLDirectHVACServiceFrames) { d.Monthly[0].Site.Value++ },
		"unclosed carrier": func(s *epathRealSQLService, d *epathSQLDirectHVACServiceFrames) {
			row := &d.Monthly[0]
			e, g := row.ByCarrier["electricity"], row.ByCarrier["natural_gas"]
			e.Unassigned.Value, g.Unassigned.Value = 4, 7
			row.ByCarrier["electricity"], row.ByCarrier["natural_gas"] = e, g
		},
		"different direct owner": func(s *epathRealSQLService, d *epathSQLDirectHVACServiceFrames) {
			row := &d.Monthly[0]
			e, g := row.ByCarrier["electricity"], row.ByCarrier["natural_gas"]
			e.Direct, g.Direct = g.Direct, e.Direct
			row.ByCarrier["electricity"], row.ByCarrier["natural_gas"] = e, g
		},
	} {
		t.Run(name, func(t *testing.T) {
			service, direct := epathSQLDirectHVACBuildingLedgerFixture()
			mutate(&service, direct)
			if err := epathSQLDirectHVACBuildingLedgerChecks(service, direct, "annual", &epathSQLModelChecks{}); err == nil {
				t.Fatal(fmt.Sprintf("accepted %s", name))
			}
		})
	}
}
