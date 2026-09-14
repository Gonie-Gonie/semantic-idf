package simulation

// Independent acceptance support. Chart transport is separate from the one-stage annual source
// scalar. Native SQL rows, not chart sums or candidate values, are authority.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"
)

// The root wires this into the independent original-result decoder BEFORE its
// typed unmarshal. Go otherwise accepts null in []float64 as 0, destroying the
// distinction before a known-zero source/chart consumer can examine it.
// This validates every present chart, without interpreting native families or
// changing values. An absent/null chart object is handled by its typed proof.
func epathValidateOracleOriginalHourlyEnergy(data json.RawMessage) error {
	var result struct {
		Sources []struct {
			HourlyEnergy json.RawMessage `json:"hourlyEnergy"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}
	for sourceIndex, source := range result.Sources {
		chart := bytes.TrimSpace(source.HourlyEnergy)
		if len(chart) == 0 || bytes.Equal(chart, []byte("null")) {
			continue
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(chart, &object); err != nil {
			return fmt.Errorf("source %d HourlyEnergy is not an object: %w", sourceIndex, err)
		}
		values := bytes.TrimSpace(object["values"])
		if len(values) == 0 || bytes.Equal(values, []byte("null")) {
			return fmt.Errorf("source %d present HourlyEnergy has absent/null values, not observed zero", sourceIndex)
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(values, &rows); err != nil {
			return fmt.Errorf("source %d HourlyEnergy values are not an array: %w", sourceIndex, err)
		}
		for rowIndex, raw := range rows {
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return fmt.Errorf("source %d HourlyEnergy sample %d is null, not observed zero", sourceIndex, rowIndex)
			}
			var number float64
			if err := json.Unmarshal(raw, &number); err != nil || !epathOracleFinite(number) {
				return fmt.Errorf("source %d HourlyEnergy sample %d is not a finite JSON number", sourceIndex, rowIndex)
			}
		}
	}
	return nil
}

type epathSQLPoolSourceChart struct {
	Labels []string
	Values []float64
}

func epathSQLPoolSourceChartExpectation(proof epathSQLPoolSourceProof) (epathSQLPoolSourceChart, error) {
	if _, err := epathSQLPoolSourceProofQuantity(proof); err != nil {
		return epathSQLPoolSourceChart{}, err
	}
	if proof.Identity.Source.ReportingFrequency == "Monthly" {
		return epathSQLPoolSourceChart{}, nil
	}
	if proof.Identity.Source.ReportingFrequency != "Hourly" {
		return epathSQLPoolSourceChart{}, fmt.Errorf("Pool source chart needs an independently observed Hourly identity")
	}
	// SQL query iteration order is not physical evidence. Sort actual native
	// TimeIndex and then prove calendar order; do not assume consecutive IDs.
	rows := append([]epathSQLPoolNativeRow(nil), proof.Identity.Rows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].TimeIndex < rows[j].TimeIndex })
	out := epathSQLPoolSourceChart{Labels: make([]string, len(rows)), Values: make([]float64, len(rows))}
	for n, row := range rows {
		stamp := time.Date(proof.Weather.Year, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Hour)
		if row.Month != int(stamp.Month()) || row.Day != stamp.Day() || row.Hour != stamp.Hour()+1 || row.Year != proof.Weather.Year || row.Minute != 0 || row.IntervalMinutes != 60 || row.IntervalType != 1 || row.EnvironmentIndex != proof.Weather.EnvironmentIndex || row.SimulationDays != stamp.YearDay() {
			return epathSQLPoolSourceChart{}, fmt.Errorf("Pool native TimeIndex order does not prove the exact weather end-of-hour calendar at %d", n)
		}
		out.Labels[n] = fmt.Sprintf("%02d-%02d %02d:00", row.Month, row.Day, row.Hour)
		energy := row.NativeValue / 3600000
		if proof.Identity.Source.SourceUnit == "W" {
			energy = row.NativeValue * (row.IntervalMinutes / 60) / 1000
		}
		if !epathOracleFinite(energy) || energy < 0 {
			return epathSQLPoolSourceChart{}, fmt.Errorf("Pool native chart row cannot establish finite nonnegative energy")
		}
		out.Values[n] = math.Round(energy*1000) / 1000
	}
	return out, nil
}

// Call next to the existing source consumer; this does not introduce another
// required scalar or duplicate a Monthly consumption budget. The source
// consumer retains native identity, executed owner, knownness and role checks.
// Original-wire numeric-array presence must be retained/validated BEFORE Go's
// []float64 decoder, which otherwise turns JSON null into numeric zero.
func epathSQLCheckPoolSourceChart(bundle PurposeResultBundle, proof *epathSQLPoolSourceProof) error {
	if err := epathSQLCheckPoolSource(bundle, proof); err != nil {
		return err
	}
	want, err := epathSQLPoolSourceChartExpectation(*proof)
	if err != nil {
		return err
	}
	id := fmt.Sprintf("sql-rdd-%d", proof.Identity.Source.DictionaryIndex)
	var source EnergyDataSource
	for _, candidate := range bundle.EnergyExplanation.Sources {
		if candidate.ID == id {
			source = candidate
		}
	}
	// Only Hourly E and Hourly R own charts: 14 of the 28 native identities.
	// Monthly E and Monthly R must never borrow either companion's samples.
	if proof.Identity.Source.ReportingFrequency == "Monthly" {
		if source.HourlyEnergy != nil {
			return fmt.Errorf("Pool Monthly E/R source borrowed an Hourly companion chart")
		}
		return nil
	}
	chart := source.HourlyEnergy
	if len(want.Values) != 8760 || len(want.Labels) != 8760 || chart == nil || chart.Unit != "kWh" || chart.Basis != "reported_source" || len(chart.Values) != 8760 || len(bundle.EnergyExplanation.HourlyLabels) != 8760 {
		return fmt.Errorf("Pool Hourly source lost its complete native 8760-point chart/shared axis")
	}
	for n, value := range want.Values {
		if bundle.EnergyExplanation.HourlyLabels[n] != want.Labels[n] {
			return fmt.Errorf("Pool shared Hourly axis changed native end-hour/calendar at %d", n)
		}
		if !epathOracleFinite(chart.Values[n]) || chart.Values[n] < 0 || chart.Values[n] != value {
			return fmt.Errorf("Pool Hourly chart changed native row integration or single 3dp transport at %d", n)
		}
	}
	return nil
}
