package simulation

// Independent acceptance support. Original topology plus hand native weather rows; no candidate,
// production chart builder or expected artifact supplies an expected value.
import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func epathSQLPoolChartHandBundle(t *testing.T, proof epathSQLPoolSourceProof) PurposeResultBundle {
	t.Helper()
	bundle := epathSQLPoolConsumerHandBundle(t, proof)
	chart, err := epathSQLPoolSourceChartExpectation(proof)
	if err != nil {
		t.Fatal(err)
	}
	if proof.Identity.Source.ReportingFrequency == "Hourly" {
		bundle.EnergyExplanation.HourlyLabels = append([]string(nil), chart.Labels...)
		bundle.EnergyExplanation.Sources[0].HourlyEnergy = &EnergySourceHourlyEnergy{Unit: "kWh", Basis: "reported_source", Values: append([]float64(nil), chart.Values...)}
	}
	return bundle
}

func TestEnergyPathRealSQLPoolSourceChartFourFrequencyRoles(t *testing.T) {
	frame := epathSQLPoolConsumerHandFrame(t)
	present, absent := 0, 0
	for _, id := range epathSQLPoolSourceIDs(frame) {
		proof := epathSQLPoolSourceProof{Identity: frame.Sources[id], Original: frame.Original, Weather: frame.Weather, Field: "rawValue"}
		bundle := epathSQLPoolChartHandBundle(t, proof)
		if err := epathSQLCheckPoolSourceChart(bundle, &proof); err != nil {
			t.Fatal(err)
		}
		if proof.Identity.Source.ReportingFrequency == "Monthly" {
			absent++
			bundle.EnergyExplanation.Sources[0].HourlyEnergy = &EnergySourceHourlyEnergy{Unit: "kWh", Basis: "reported_source", Values: make([]float64, 8760)}
			if err := epathSQLCheckPoolSourceChart(bundle, &proof); err == nil {
				t.Fatal("Monthly E/R borrowed a companion's chart")
			}
		} else {
			present++
			if bundle.EnergyExplanation.HourlyLabels[0] != "01-01 01:00" || bundle.EnergyExplanation.HourlyLabels[23] != "01-01 24:00" || bundle.EnergyExplanation.HourlyLabels[8760-1] != "12-31 24:00" {
				t.Fatal("native end-of-hour axis changed")
			}
			bundle.EnergyExplanation.Sources[0].HourlyEnergy = nil
			if err := epathSQLCheckPoolSourceChart(bundle, &proof); err == nil {
				t.Fatal("observed Hourly source lost its whole chart, including measured zeros")
			}
		}
	}
	if present != 14 || absent != 14 {
		t.Fatalf("native chart role census changed: present %d absent %d", present, absent)
	}
}

// Nonuniform native samples expose reordering, 3dp transport and wrongly
// deriving an annual raw scalar from rounded chart values. Recompute all source
// summaries independently from native rows so unchanged native proof remains
// mandatory, rather than mutating only the chart expectation.
func epathSQLPoolChartHandProfile(proof epathSQLPoolSourceProof) epathSQLPoolSourceProof {
	proof = epathSQLPoolConsumerPower(proof, 0.00049)
	proof.Identity.Source.RawSum, proof.Identity.Source.EnergyKWh = epathOracleNumber(0), epathOracleNumber(0)
	proof.Identity.Source.Months = append([]epathRealSQLMonth(nil), proof.Identity.Source.Months...)
	for m := range proof.Identity.Source.Months {
		proof.Identity.Source.Months[m].RawSum, proof.Identity.Source.Months[m].EnergyKWh = epathOracleNumber(0), epathOracleNumber(0)
	}
	profile := map[int]float64{0: 1.23456, 1: 0.00051, 23: 0.002001, 744: 0.001501, 8759: 0}
	for n := range proof.Identity.Rows {
		row := &proof.Identity.Rows[n]
		if energy, exists := profile[n]; exists {
			row.NativeValue = energy * 3600000
			if proof.Identity.Source.SourceUnit == "W" {
				row.NativeValue = energy * 1000
			}
		}
		row.EnergyKWh = row.NativeValue / 3600000
		if proof.Identity.Source.SourceUnit == "W" {
			row.EnergyKWh = row.NativeValue * (row.IntervalMinutes / 60) / 1000
		}
		bucket := &proof.Identity.Source.Months[row.Month-1]
		*bucket.RawSum += row.NativeValue
		*bucket.EnergyKWh += row.EnergyKWh
	}
	for m, month := range proof.Identity.Source.Months {
		proof.Identity.NativeRawMonthly[m], proof.Identity.NativeEnergyMonthly[m] = *month.RawSum, *month.EnergyKWh
		*proof.Identity.Source.RawSum += *month.RawSum
		*proof.Identity.Source.EnergyKWh += *month.EnergyKWh
	}
	return proof
}

func TestEnergyPathRealSQLPoolSourceChartNativeRowsAndSingleSampleRounding(t *testing.T) {
	frame := epathSQLPoolConsumerHandFrame(t)
	for _, companion := range []int{1, 2} { // Hourly Energy and Hourly Rate.
		proof := epathSQLPoolChartHandProfile(epathSQLPoolConsumerHandProof(frame, "pool.water_heating", companion))
		bundle := epathSQLPoolChartHandBundle(t, proof)
		values := bundle.EnergyExplanation.Sources[0].HourlyEnergy.Values
		if !reflect.DeepEqual(values[:3], []float64{1.235, 0.001, 0}) || values[23] != 0.002 || values[744] != 0.002 || values[8759] != 0 {
			t.Fatalf("native sample formula or 3dp transport changed: %v", values[:3])
		}
		sum := 0.0
		for _, value := range values {
			sum += value
		}
		if math.Abs(sum-bundle.EnergyExplanation.Sources[0].RawValue) < 1 {
			t.Fatal("hand profile no longer distinguishes raw native annual quantity from rounded chart sample sum")
		}
		for reload := 0; reload < 2; reload++ {
			bundle.EnergyExplanation.Sources[0] = epathSQLPoolConsumerDecodeSource(t, bundle.EnergyExplanation.Sources[0], true)
			if err := epathSQLCheckPoolSourceChart(bundle, &proof); err != nil {
				t.Fatal(err)
			}
		}
		before := append([]epathSQLPoolNativeRow(nil), proof.Identity.Rows...)
		if err := epathSQLCheckPoolSourceChart(bundle, &proof); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, proof.Identity.Rows) {
			t.Fatal("chart consumer mutated native evidence")
		}
		for _, mutation := range []string{"wrong value", "raw unrounded", "value swap", "native W or J", "zone multiplier", "negative", "nonfinite", "truncated", "duplicate", "unit", "basis", "axis missing", "axis swap", "start-hour convention", "leap calendar", "annual from chart"} {
			copy := bundle
			copy.EnergyExplanation.Sources = append([]EnergyDataSource(nil), bundle.EnergyExplanation.Sources...)
			copy.EnergyExplanation.HourlyLabels = append([]string(nil), bundle.EnergyExplanation.HourlyLabels...)
			chart := *bundle.EnergyExplanation.Sources[0].HourlyEnergy
			chart.Values = append([]float64(nil), chart.Values...)
			copy.EnergyExplanation.Sources[0].HourlyEnergy = &chart
			switch mutation {
			case "wrong value":
				chart.Values[100] = 0.001
			case "raw unrounded":
				chart.Values[0] = 1.23456
			case "value swap":
				chart.Values[0], chart.Values[1] = chart.Values[1], chart.Values[0]
			case "native W or J":
				chart.Values[0] = proof.Identity.Rows[0].NativeValue
			case "zone multiplier":
				chart.Values[0] *= 10
			case "negative":
				chart.Values[0] = -1
			case "nonfinite":
				chart.Values[0] = math.NaN()
			case "truncated":
				chart.Values = chart.Values[:8759]
			case "duplicate":
				chart.Values = append(chart.Values, 0)
			case "unit":
				chart.Unit = "W"
			case "basis":
				chart.Basis = "allocated_source"
			case "axis missing":
				copy.EnergyExplanation.HourlyLabels = nil
			case "axis swap":
				copy.EnergyExplanation.HourlyLabels[0], copy.EnergyExplanation.HourlyLabels[1] = copy.EnergyExplanation.HourlyLabels[1], copy.EnergyExplanation.HourlyLabels[0]
			case "start-hour convention":
				copy.EnergyExplanation.HourlyLabels[0] = "01-01 00:00"
			case "leap calendar":
				copy.EnergyExplanation.HourlyLabels[1416] = "02-29 01:00"
			case "annual from chart":
				copy.EnergyExplanation.Sources[0].RawValue, copy.EnergyExplanation.Sources[0].EffectiveValue = math.Round(sum*1000)/1000, math.Round(sum*1000)/1000
			}
			if err := epathSQLCheckPoolSourceChart(copy, &proof); err == nil {
				t.Fatalf("accepted %s for native %s", mutation, proof.Identity.Source.SourceUnit)
			}
		}
	}
}

func TestEnergyPathRealSQLPoolSourceChartNativeAxisProof(t *testing.T) {
	frame := epathSQLPoolConsumerHandFrame(t)
	base := epathSQLPoolConsumerHandProof(frame, "pump.cw.electricity", 1)
	for _, mutation := range []string{"iteration order", "nonconsecutive IDs", "wrong duration", "duplicate calendar", "wrong year", "wrong environment", "negative native", "wrong native integration", "missing native row", "TimeIndex calendar inversion", "source request absent"} {
		proof := base
		proof.Identity.Rows = append([]epathSQLPoolNativeRow(nil), base.Identity.Rows...)
		switch mutation {
		case "iteration order":
			proof.Identity.Rows[0], proof.Identity.Rows[1] = proof.Identity.Rows[1], proof.Identity.Rows[0]
		case "nonconsecutive IDs":
			for n := range proof.Identity.Rows {
				proof.Identity.Rows[n].TimeIndex = 101 + 11*n
			}
		case "wrong duration":
			proof.Identity.Rows[0].IntervalMinutes = 30
		case "duplicate calendar":
			proof.Identity.Rows[1].Hour = proof.Identity.Rows[0].Hour
		case "wrong year":
			proof.Identity.Rows[0].Year = 2016
		case "wrong environment":
			proof.Identity.Rows[0].EnvironmentIndex++
		case "negative native":
			proof.Identity.Rows[0].NativeValue = -1
		case "wrong native integration":
			proof.Identity.Rows[0].EnergyKWh++
		case "missing native row":
			proof.Identity.Rows = proof.Identity.Rows[:8759]
		case "TimeIndex calendar inversion":
			proof.Identity.Rows[0].TimeIndex, proof.Identity.Rows[1].TimeIndex = proof.Identity.Rows[1].TimeIndex, proof.Identity.Rows[0].TimeIndex
		case "source request absent":
			proof.Identity.RequestBound = false
		}
		_, err := epathSQLPoolSourceChartExpectation(proof)
		valid := mutation == "iteration order" || mutation == "nonconsecutive IDs"
		if (err == nil) != valid {
			t.Fatalf("%s validity %t: %v", mutation, valid, err)
		}
	}
}

func TestEnergyPathRealSQLPoolSourceChartOriginalWireNumericPresence(t *testing.T) {
	for _, raw := range []string{
		`{"sources":[]}`, `{"sources":[{}]}`, `{"sources":[{"hourlyEnergy":null}]}`,
		`{"sources":[{"hourlyEnergy":{"values":[]}}]}`,
		`{"sources":[{"hourlyEnergy":{"values":[0,0.001,-1,1e3]}}]}`,
	} {
		if err := epathValidateOracleOriginalHourlyEnergy(json.RawMessage(raw)); err != nil {
			t.Fatalf("finite JSON values/absent object changed before family-specific validation: %v", err)
		}
	}
	for _, chart := range []string{`[]`, `1`, `"chart"`, `{}`, `{"values":null}`, `{"values":0}`, `{"values":{}}`, `{"values":[null]}`, `{"values":[0,null,0]}`, `{"values":["0"]}`, `{"values":[true]}`, `{"values":[{}]}`, `{"values":[[]]}`, `{"values":[1e999]}`} {
		raw := `{"sources":[{"hourlyEnergy":` + chart + `}]}`
		if err := epathValidateOracleOriginalHourlyEnergy(json.RawMessage(raw)); err == nil {
			t.Fatalf("original wire accepted non-observation chart %s", chart)
		}
	}
	// Demonstrate why the generic raw guard is required even when a complete
	// 8760-point native chart consists of observed zeros. Typed Go unmarshalling
	// accepts null as 0; the independent raw guard must reject before that loss.
	frame := epathSQLPoolConsumerHandFrame(t)
	proof := epathSQLPoolConsumerHandProof(frame, "boiler.ancillary_electricity", 1)
	bundle := epathSQLPoolChartHandBundle(t, proof)
	values := make([]any, 8760)
	for n := range values {
		values[n] = float64(0)
	}
	values[123] = nil
	raw, err := json.Marshal(map[string]any{"sources": []any{map[string]any{"hourlyEnergy": map[string]any{"values": values}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := epathValidateOracleOriginalHourlyEnergy(raw); err == nil {
		t.Fatal("a null chart sample was replaced by known zero")
	}
	var typed struct {
		Sources []struct {
			HourlyEnergy *EnergySourceHourlyEnergy `json:"hourlyEnergy"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(raw, &typed); err != nil {
		t.Fatal(err)
	}
	if typed.Sources[0].HourlyEnergy.Values[123] != 0 {
		t.Fatal("fixture no longer demonstrates Go null-to-zero conversion")
	}
	// Typed equality cannot recover the lost distinction. This deliberate
	// demonstration makes the root decoder wiring a mandatory integration gate.
	bundle.EnergyExplanation.Sources[0].HourlyEnergy.Values = typed.Sources[0].HourlyEnergy.Values
	if err := epathSQLCheckPoolSourceChart(bundle, &proof); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "NaN") {
		t.Fatal("negative fixture is not valid JSON null")
	}
}
