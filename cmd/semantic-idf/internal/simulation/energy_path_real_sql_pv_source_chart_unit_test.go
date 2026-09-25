package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"
)

// Literal constant hourly power for hand-wire fixtures; never compiled values.
func epathSQLPVLiteralChart(power float64) ([]string, []float64) {
	labels, values := make([]string, 0, 8760), make([]float64, 0, 8760)
	for day := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC); day.Year() == 2017; day = day.AddDate(0, 0, 1) {
		for hour := 1; hour <= 24; hour++ {
			labels = append(labels, fmt.Sprintf("%02d-%02d %02d:00", day.Month(), day.Day(), hour))
			values = append(values, math.Round(power*1000)/1000)
		}
	}
	return labels, values
}

func epathSQLPVChartHandIdentity() (epathSQLPVSourceIdentity, epathRealSQLWeather) {
	identity := epathSQLPVSourceIdentity{Source: epathRealSQLSource{DictionaryIndex: 77, ReportingFrequency: "Hourly"}, Dictionary: epathSQLPVNativeDictionary{Index: 77, Frequency: "Hourly", Unit: "J"}}
	first := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)
	for n := 0; n < 8760; n++ {
		stamp := first.Add(time.Duration(n) * time.Hour)
		value := 0.0
		switch n {
		case 0:
			value = -.0004
		case 1:
			value = -1.2346
		case 2:
			value = 1.2344
		}
		identity.Rows = append(identity.Rows, epathSQLPVNativeRow{TimeIndex: 11 + n*7, EnvironmentIndex: 3, Year: 2017, Month: int(stamp.Month()), Day: stamp.Day(), Hour: stamp.Hour() + 1, IntervalType: 1, IntervalMinutes: 60, SimulationDays: stamp.YearDay(), NativeJ: value * 3600000})
	}
	return identity, epathSQLPoolSourceHandWeather()
}

func TestEnergyPathSQLPVSourceChartSignedHoursAndDetachedNativeOrder(t *testing.T) {
	identity, weather := epathSQLPVChartHandIdentity()
	before := append([]epathSQLPVNativeRow(nil), identity.Rows...)
	chart, err := epathSQLPVSourceChartExpectation(identity, weather)
	if err != nil {
		t.Fatal(err)
	}
	labels, zeros := epathSQLPVLiteralChart(0)
	if len(chart.Values) != 8760 || !reflect.DeepEqual(chart.Labels, labels) || chart.Values[0] != 0 || chart.Values[1] != -1.235 || chart.Values[2] != 1.234 || chart.Values[8759] != zeros[8759] || !reflect.DeepEqual(before, identity.Rows) {
		t.Fatal("native signed per-sample transport, endpoints or input immutability changed")
	}
	identity.Rows[0], identity.Rows[8759] = identity.Rows[8759], identity.Rows[0]
	shuffled, err := epathSQLPVSourceChartExpectation(identity, weather)
	if err != nil || !reflect.DeepEqual(shuffled, chart) {
		t.Fatalf("query order was mistaken for native TimeIndex order: %v", err)
	}
	identity.Rows[1].NativeJ = 999
	if chart.Values[1] != -1.235 {
		t.Fatal("prepared chart aliased mutable native rows")
	}
}

func TestEnergyPathSQLPVSourceChartRejectsCalendarSubstitution(t *testing.T) {
	base, weather := epathSQLPVChartHandIdentity()
	cases := []struct {
		name   string
		change func(*epathSQLPVSourceIdentity)
	}{
		{"missing hour", func(s *epathSQLPVSourceIdentity) { s.Rows = s.Rows[1:] }},
		{"duplicate time", func(s *epathSQLPVSourceIdentity) { s.Rows[1].TimeIndex = s.Rows[0].TimeIndex }},
		{"time calendar reversal", func(s *epathSQLPVSourceIdentity) {
			s.Rows[0].TimeIndex, s.Rows[1].TimeIndex = s.Rows[1].TimeIndex, s.Rows[0].TimeIndex
		}},
		{"hour zero", func(s *epathSQLPVSourceIdentity) { s.Rows[0].Hour = 0 }},
		{"hour25", func(s *epathSQLPVSourceIdentity) { s.Rows[0].Hour = 25 }},
		{"leap day", func(s *epathSQLPVSourceIdentity) { s.Rows[0].Month, s.Rows[0].Day = 2, 29 }},
		{"wrong cumulative day", func(s *epathSQLPVSourceIdentity) { s.Rows[0].SimulationDays = 2 }},
		{"design environment", func(s *epathSQLPVSourceIdentity) { s.Rows[0].EnvironmentIndex = 1 }},
		{"wrong duration", func(s *epathSQLPVSourceIdentity) { s.Rows[0].IntervalMinutes = 10 }},
		{"wrong year", func(s *epathSQLPVSourceIdentity) { s.Rows[0].Year = 2020 }},
		{"nonfinite", func(s *epathSQLPVSourceIdentity) { s.Rows[0].NativeJ = math.NaN() }},
		{"wrong units", func(s *epathSQLPVSourceIdentity) { s.Dictionary.Unit = "W" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			s.Rows = append([]epathSQLPVNativeRow(nil), base.Rows...)
			tc.change(&s)
			if _, err := epathSQLPVSourceChartExpectation(s, weather); err == nil {
				t.Fatal("bad native chart accepted")
			}
		})
	}
}

func TestEnergyPathSQLPVSourceChartOriginalWireRequiresEverySignedSample(t *testing.T) {
	native, weather := epathSQLPVChartHandIdentity()
	chart, err := epathSQLPVSourceChartExpectation(native, weather)
	if err != nil {
		t.Fatal(err)
	}
	identity := epathSQLPVValidatedIdentity{DictionaryIndex: 77, Frequency: "Hourly", Chart: chart}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{HourlyLabels: append([]string(nil), chart.Labels...), Sources: []EnergyDataSource{{ID: "sql-rdd-77", HourlyEnergy: &EnergySourceHourlyEnergy{Unit: "kWh", Basis: "reported_source", Values: append([]float64(nil), chart.Values...)}}}}}
	if err := epathSQLCheckPVSourceChart(bundle, identity); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"nil", "missing", "absolute", "extra quantum", "reordered", "bad unit", "bad basis", "bad label", "short labels", "nan"} {
		t.Run(mode, func(t *testing.T) {
			bad := bundle
			bad.EnergyExplanation.Sources = append([]EnergyDataSource(nil), bundle.EnergyExplanation.Sources...)
			copy := *bundle.EnergyExplanation.Sources[0].HourlyEnergy
			copy.Values = append([]float64(nil), copy.Values...)
			bad.EnergyExplanation.Sources[0].HourlyEnergy = &copy
			bad.EnergyExplanation.HourlyLabels = append([]string(nil), bundle.EnergyExplanation.HourlyLabels...)
			switch mode {
			case "nil":
				bad.EnergyExplanation.Sources[0].HourlyEnergy = nil
			case "missing":
				copy.Values = copy.Values[1:]
			case "absolute":
				copy.Values[1] = math.Abs(copy.Values[1])
			case "extra quantum":
				copy.Values[2] += .001
			case "reordered":
				copy.Values[1], copy.Values[2] = copy.Values[2], copy.Values[1]
			case "bad unit":
				copy.Unit = "W"
			case "bad basis":
				copy.Basis = "allocated"
			case "bad label":
				bad.EnergyExplanation.HourlyLabels[0] = "01-01 00:00"
			case "short labels":
				bad.EnergyExplanation.HourlyLabels = bad.EnergyExplanation.HourlyLabels[1:]
			case "nan":
				copy.Values[0] = math.NaN()
			}
			if err := epathSQLCheckPVSourceChart(bad, identity); err == nil {
				t.Fatal("invalid persisted source chart passed")
			}
		})
	}
	monthly := identity
	monthly.Frequency, monthly.Chart = "Monthly", nil
	if err := epathSQLCheckPVSourceChart(bundle, monthly); err == nil {
		t.Fatal("Monthly source borrowed companion chart")
	}
	bundle.EnergyExplanation.Sources[0].HourlyEnergy = nil
	if err := epathSQLCheckPVSourceChart(bundle, monthly); err != nil {
		t.Fatal(err)
	}
	if err := epathValidateOracleOriginalHourlyEnergy(json.RawMessage(`{"sources":[{"hourlyEnergy":{"unit":"kWh","basis":"reported_source","values":[null]}}]}`)); err == nil {
		t.Fatal("raw null became a chart zero")
	}
}
