package simulation

// Independent native per-hour transport, separate from annual source rounding.
import (
	"fmt"
	"math"
	"sort"
	"time"
)

type epathSQLPVSourceChart struct {
	Labels []string
	Values []float64
}

// Called only after the native source frame passed its full metadata/calendar
// gate. The detached chart still proves TimeIndex ordering, not contiguous IDs.
func epathSQLPVSourceChartExpectation(identity epathSQLPVSourceIdentity, weather epathRealSQLWeather) (*epathSQLPVSourceChart, error) {
	if identity.Dictionary.Frequency == "Monthly" {
		return nil, nil
	}
	if identity.Dictionary.Frequency != "Hourly" || identity.Source.ReportingFrequency != "Hourly" || identity.Dictionary.Unit != "J" || weather.Year != 2017 || weather.EnvironmentIndex <= 0 || len(identity.Rows) != 8760 {
		return nil, fmt.Errorf("PV chart lacks independently observed native Hourly/J calendar")
	}
	rows := append([]epathSQLPVNativeRow(nil), identity.Rows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].TimeIndex < rows[j].TimeIndex })
	chart := &epathSQLPVSourceChart{Labels: make([]string, 8760), Values: make([]float64, 8760)}
	first := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)
	for n, row := range rows {
		stamp := first.Add(time.Duration(n) * time.Hour)
		if row.TimeIndex <= 0 || n > 0 && row.TimeIndex <= rows[n-1].TimeIndex || row.Year != 2017 || row.EnvironmentIndex != weather.EnvironmentIndex || row.Month != int(stamp.Month()) || row.Day != stamp.Day() || row.Hour != stamp.Hour()+1 || row.Minute != 0 || row.IntervalType != 1 || row.IntervalMinutes != 60 || row.SimulationDays != stamp.YearDay() || !epathOracleFinite(row.NativeJ) {
			return nil, fmt.Errorf("PV native TimeIndex order/calendar differs at Hourly sample %d", n)
		}
		chart.Labels[n] = fmt.Sprintf("%02d-%02d %02d:00", row.Month, row.Day, row.Hour)
		chart.Values[n] = math.Round(row.NativeJ/3600000*1000) / 1000
		if !epathOracleFinite(chart.Values[n]) {
			return nil, fmt.Errorf("PV native signed Hourly normalization overflow")
		}
	}
	return chart, nil
}

func epathSQLCheckPVSourceChart(bundle PurposeResultBundle, identity epathSQLPVValidatedIdentity) error {
	id := fmt.Sprintf("sql-rdd-%d", identity.DictionaryIndex)
	var source *EnergyDataSource
	for i := range bundle.EnergyExplanation.Sources {
		if bundle.EnergyExplanation.Sources[i].ID == id {
			if source != nil {
				return fmt.Errorf("PV chart duplicated its actual source identity")
			}
			source = &bundle.EnergyExplanation.Sources[i]
		}
	}
	if source == nil {
		return fmt.Errorf("PV chart lost its actual source")
	}
	if identity.Frequency == "Monthly" {
		if identity.Chart != nil || source.HourlyEnergy != nil {
			return fmt.Errorf("PV Monthly source borrowed an Hourly chart")
		}
		return nil
	}
	want, actual := identity.Chart, source.HourlyEnergy
	if identity.Frequency != "Hourly" || want == nil || len(want.Labels) != 8760 || len(want.Values) != 8760 || actual == nil || actual.Unit != "kWh" || actual.Basis != "reported_source" || len(actual.Values) != 8760 || len(bundle.EnergyExplanation.HourlyLabels) != 8760 {
		return fmt.Errorf("PV Hourly source lost complete native chart/shared axis")
	}
	for n, value := range want.Values {
		if !epathOracleFinite(value) || !epathOracleFinite(actual.Values[n]) || actual.Values[n] != value || bundle.EnergyExplanation.HourlyLabels[n] != want.Labels[n] {
			return fmt.Errorf("PV signed native Hourly sample/axis changed at %d", n)
		}
	}
	return nil
}

func epathSQLCheckPVSourceConsumerComplete(bundle PurposeResultBundle, check epathSQLModelCheck, prepared epathSQLPVValidatedSources) error {
	if err := epathSQLCheckPVSourceConsumerWithShape(bundle, check, prepared); err != nil {
		return err
	}
	return epathSQLCheckPVSourceChart(bundle, prepared.identities[check.PVSource.DictionaryIndex])
}
