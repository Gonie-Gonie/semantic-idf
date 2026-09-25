package simulation

import (
	"math"
	"strings"
	"testing"
)

func epathSQLPVBalanceHandPoint() map[string]epathSQLQuantity {
	out := map[string]epathSQLQuantity{}
	for _, literal := range epathSQLPVBalanceHandLiterals() {
		out[literal.id] = epathSQLPVBalanceNativeQuantity(literal.power)
	}
	return out
}

func TestEnergyPathSQLPVNativeNineBalancesAndNonEquations(t *testing.T) {
	positive := epathSQLPVBalanceHandPoint()
	if err := epathSQLPVCheckNativeBalancePeriod(positive, 3600); err != nil {
		t.Fatal(err)
	}
	if len(epathSQLPVNativeEquations()) != 9 {
		t.Fatal("finite equation census changed")
	}
	zeros := map[string]epathSQLQuantity{}
	for key := range positive {
		zeros[key] = epathSQLQuantity{}
	}
	if err := epathSQLPVCheckNativeBalancePeriod(zeros, 3600); err != nil {
		t.Fatal(err)
	}
	// State/thermal fields are deliberately unrelated to the electrical chain.
	positive["storage.thermal"] = epathSQLPVBalanceNativeQuantity(999)
	positive["distribution.thermal"] = epathSQLPVBalanceNativeQuantity(-999)
	positive["battery.state_Ah"] = epathSQLPVBalanceNativeQuantity(12345)
	if err := epathSQLPVCheckNativeBalancePeriod(positive, 3600); err != nil {
		t.Fatal("invented thermal or state equation:", err)
	}
	for _, key := range []string{"pv.dc.1", "distribution.electricity", "inverter.dc_input", "inverter.ac_output", "inverter.loss", "inverter.loss_decrement", "storage.charge", "storage.decrement", "storage.discharge", "facility.produced", "facility.demand", "facility.purchased", "facility.sold", "cogeneration.electricity", "inverter.ancillary"} {
		t.Run(key, func(t *testing.T) {
			bad := epathSQLPVBalanceHandPoint()
			q := bad[key]
			q.Value += 0.01
			bad[key] = q
			if err := epathSQLPVCheckNativeBalancePeriod(bad, 3600); err == nil {
				t.Fatal("independent native source mismatch accepted")
			}
			delete(bad, key)
			if err := epathSQLPVCheckNativeBalancePeriod(bad, 3600); err == nil {
				t.Fatal("absent native source inferred as0")
			}
		})
	}
	for _, key := range []string{"storage.decrement", "inverter.loss_decrement"} {
		bad := epathSQLPVBalanceHandPoint()
		bad[key] = epathSQLPVBalanceNativeQuantity(1)
		if err := epathSQLPVCheckNativeBalancePeriod(bad, 3600); err == nil {
			t.Fatal("positive decrement accepted")
		}
	}
	bad := epathSQLPVBalanceHandPoint()
	bad["inverter.ancillary"] = epathSQLQuantity{Value: math.NaN()}
	if err := epathSQLPVCheckNativeBalancePeriod(bad, 3600); err == nil {
		t.Fatal("unknown/nonfinite source inferred as0")
	}
}

func TestEnergyPathSQLPVFacilityDeadbandIsCalendarLocalOnly(t *testing.T) {
	for _, seconds := range []float64{3600, 31 * 86400, 365 * 86400} {
		limit := 0.0001 * seconds / 3600000
		for _, factor := range []float64{0.5, 2} {
			point := epathSQLPVBalanceHandPoint()
			point["facility.demand"] = epathSQLPVBalanceNativeQuantity(point["facility.demand"].Value + factor*limit)
			err := epathSQLPVCheckNativeBalancePeriod(point, seconds)
			if (err == nil) != (factor < 1) {
				t.Fatalf("native Facility deadband factor%g seconds%g: %v", factor, seconds, err)
			}
		}
		point := epathSQLPVBalanceHandPoint()
		point["cogeneration.electricity"] = epathSQLPVBalanceNativeQuantity(point["cogeneration.electricity"].Value + 0.5*limit)
		if err := epathSQLPVCheckNativeBalancePeriod(point, seconds); err == nil || !strings.Contains(err.Error(), "cogeneration_sole_member") {
			t.Fatal("Facility deadband leaked into consumed parent/member closure", err)
		}
	}
	for _, seconds := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if err := epathSQLPVCheckNativeBalancePeriod(epathSQLPVBalanceHandPoint(), seconds); err == nil {
			t.Fatal("unknown calendar acquired a deadband")
		}
	}
}

func TestEnergyPathSQLPVWrongMonthCannotHideInAnnualCancellation(t *testing.T) {
	months := []map[string]epathSQLQuantity{epathSQLPVBalanceHandPoint(), epathSQLPVBalanceHandPoint()}
	months[0]["cogeneration.electricity"] = epathSQLPVBalanceNativeQuantity(0.11)
	months[1]["cogeneration.electricity"] = epathSQLPVBalanceNativeQuantity(0.09)
	annual := map[string]epathSQLQuantity{}
	for _, month := range months {
		if err := epathSQLPVCheckNativeBalancePeriod(month, 3600); err == nil {
			t.Fatal("wrong native month accepted")
		}
		for key, q := range month {
			annual[key] = annual[key].add(q)
		}
	}
	if err := epathSQLPVCheckNativeBalancePeriod(annual, 7200); err != nil {
		t.Fatal("hand cancellation setup is not exact", err)
	}
	// The full frame validator tests every month/hour before annual; an annual
	// equality by itself is intentionally demonstrated to be insufficient.
}
