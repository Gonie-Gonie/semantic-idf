package simulation

import (
	"math"
	"testing"
)

func TestEnergyPathRealSQLQuantitySource3972SymmetricBounds(t *testing.T) {
	// Exact, independently read Monthly/J Cooling:Electricity observations
	// from the preserved 25.1 LargeOffice SQL dictionary 3972. These are a
	// validation-regression fixture, not approved model expected results.
	joules := []float64{0, 850239496.4437542, 5687321978.308133, 76073341849.55403, 148685425860.78113, 343452046774.0382, 504929632307.4874, 424395870307.23035, 245704758773.76755, 45295216690.90316, 44699674805.76154, 1795.6336609251157}
	source := epathRealSQLSource{DictionaryIndex: 3972, Name: "Cooling:Electricity", IsMeter: true, SourceUnit: "J", ReportingFrequency: "Monthly"}
	for month, value := range joules {
		source.Months = append(source.Months, epathRealSQLMonth{Month: month + 1, Rows: 1, RawSum: epathOracleNumber(value), EnergyKWh: epathOracleNumber(value / 3600000)})
	}
	values, err := epathSQLMonthly(source, epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2})
	if err != nil {
		t.Fatal(err)
	}
	roundingCounterexamples := 0
	for month, q := range values {
		before := q
		low, high := q.bounds()
		if q.Error+1e-12 < math.Max(q.Value-low, high-q.Value) {
			roundingCounterexamples++
		}
		if !q.valid() {
			t.Errorf("actual SQL3972 M%d is known nonnegative: center=%.17g error=%.17g bounds=[%.17g,%.17g]", month+1, q.Value, q.Error, low, high)
		}
		afterLow, afterHigh := q.bounds()
		wantError := .001
		if joules[month] == 0 {
			wantError = 0
		}
		if q != before || q.Value != joules[month]/3600000 || q.Error != wantError || q.Bounds != nil || math.Float64bits(low) != math.Float64bits(afterLow) || math.Float64bits(high) != math.Float64bits(afterHigh) {
			t.Fatal("validation changed the observed center, error budget or actual bounds")
		}
	}
	if roundingCounterexamples != 3 {
		t.Fatalf("fixture must reproduce M6/M8/M9 subtraction-rounding failures, got %d", roundingCounterexamples)
	}
}

func TestEnergyPathRealSQLQuantityExplicitBoundsRemainStrict(t *testing.T) {
	for _, value := range []float64{1e6, -1e6, 1e12, -1e12} {
		q := epathSQLQuantity{Value: value, Error: .001}
		if !q.valid() {
			t.Fatalf("finite symmetric bounds defined by the existing error budget rejected: %+v", q)
		}
		low, high := q.bounds()
		bounded := epathSQLBounded(value, low, high)
		if !bounded.valid() {
			t.Fatal("explicit interval with correctly derived error is invalid")
		}
	}
	for _, q := range []epathSQLQuantity{
		{Value: 1e6, Error: .001, Bounds: &[2]float64{1e6 - .01, 1e6 + .01}},
		{Value: 1e6, Error: .001, Bounds: &[2]float64{1e6 - .001, 1e6 + .001}},
		{Value: 10, Error: 1, Bounds: &[2]float64{11, 9}},
		{Value: 10, Error: 1, Bounds: &[2]float64{11, 12}},
		{Value: 0, Error: -1},
		{Value: math.Inf(1), Error: .001},
		{Value: 0, Error: math.NaN()},
		{Value: 0, Error: 1, Bounds: &[2]float64{0, math.Inf(1)}},
	} {
		if q.valid() {
			t.Fatalf("inconsistent explicit bounds or invalid numeric input accepted: %+v", q)
		}
	}
}
