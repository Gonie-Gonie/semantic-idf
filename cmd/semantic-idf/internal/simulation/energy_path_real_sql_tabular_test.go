package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// A reviewed original annual site-energy cell, never a monthly substitute or
// an alias for SourceEnergy/Demand reports. DecimalPlaces describes the actual
// displayed source value, not the downstream graph's serialization precision.
type epathRealSQLTabularSelector struct {
	ReportName      string `json:"reportName"`
	ReportForString string `json:"reportForString"`
	TableName       string `json:"tableName"`
	RowName         string `json:"rowName"`
	ColumnName      string `json:"columnName"`
	Unit            string `json:"unit"`
	DecimalPlaces   int    `json:"decimalPlaces"`
}

type epathSQLTabularObservation struct {
	Selector         epathRealSQLTabularSelector
	TabularDataIndex int
	RawText          string
	RawValue         float64
	Quantity         epathSQLQuantity // Annual kWh only; no synthetic monthly frames.
	Weather          epathRealSQLWeather
}

func epathValidateSQLTabularSelector(selector epathRealSQLTabularSelector) (float64, error) {
	if selector.ReportName != "AnnualBuildingUtilityPerformanceSummary" || selector.ReportForString != "Entire Facility" || selector.TableName != "End Uses" {
		return 0, fmt.Errorf("annual site Tabular source requires the exact utility/Entire Facility/End Uses report")
	}
	for _, value := range []string{selector.RowName, selector.ColumnName} {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n\x00") {
			return 0, fmt.Errorf("annual Tabular source requires exact nonempty row/column identities")
		}
	}
	// Current reviewed tabular exports have explicit fractional precision.
	// Zero is rejected so an omitted JSON field cannot silently claim a unit
	// rounding interval. Supporting integer-only reports requires review.
	if selector.DecimalPlaces < 1 || selector.DecimalPlaces > 6 {
		return 0, fmt.Errorf("annual Tabular source requires explicit display precision from1 to6 decimal places")
	}
	factor, ok := map[string]float64{"J": 1.0 / 3600000, "MJ": 1.0 / 3.6, "GJ": 1000.0 / 3.6, "Wh": .001, "kWh": 1}[selector.Unit]
	if !ok {
		return 0, fmt.Errorf("unsupported exact annual Tabular energy unit %q", selector.Unit)
	}
	return factor, nil
}

func epathValidateSQLSiteSource(site epathRealSQLSite) error {
	if site.Tabular == nil {
		return nil
	}
	if len(site.Source.Alternatives) != 0 || len(site.Source.Keys) != 0 || site.Source.IsMeter || site.Source.AllowAbsent {
		return fmt.Errorf("site %s cannot mix time-series and annual Tabular source declarations", site.ID)
	}
	_, err := epathValidateSQLTabularSelector(*site.Tabular)
	return err
}

// Nil means that the exact cell is absent in a valid, full-annual SQL schema.
// Present zero remains a non-nil observed quantity. A wrong-unit match is an
// error, not absence; no alternate report or carrier column is ever selected.
func epathReadSQLModelTabular(path string, selector epathRealSQLTabularSelector) (*epathSQLTabularObservation, error) {
	_, err := epathValidateSQLTabularSelector(selector)
	if err != nil {
		return nil, err
	}
	db, err := epathOpenOracleSQL(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	weather, err := epathReadOracleWeather(db)
	if err != nil {
		return nil, fmt.Errorf("annual Tabular weather coverage: %w", err)
	}
	rows, err := db.Query(`SELECT TabularDataIndex,Units,Value FROM TabularDataWithStrings
WHERE ReportName=? COLLATE BINARY AND ReportForString=? COLLATE BINARY AND TableName=? COLLATE BINARY
AND RowName=? COLLATE BINARY AND ColumnName=? COLLATE BINARY`, selector.ReportName, selector.ReportForString, selector.TableName, selector.RowName, selector.ColumnName)
	if err != nil {
		return nil, fmt.Errorf("annual Tabular SQL schema: %w", err)
	}
	defer rows.Close()
	var out *epathSQLTabularObservation
	for rows.Next() {
		if out != nil {
			return nil, fmt.Errorf("duplicate exact annual Tabular cell")
		}
		var index sql.NullInt64
		var unit, raw sql.NullString
		if err := rows.Scan(&index, &unit, &raw); err != nil {
			return nil, fmt.Errorf("annual Tabular value: %w", err)
		}
		if !index.Valid || index.Int64 <= 0 || !unit.Valid || unit.String != selector.Unit {
			return nil, fmt.Errorf("annual Tabular cell has invalid original index or wrong exact unit")
		}
		if !raw.Valid {
			return nil, fmt.Errorf("annual Tabular cell is NULL, not an observed zero")
		}
		value, quantity, err := epathSQLTabularRawQuantity(selector, raw.String)
		if err != nil {
			return nil, err
		}
		out = &epathSQLTabularObservation{Selector: selector, TabularDataIndex: int(index.Int64), RawText: raw.String, RawValue: value, Quantity: quantity, Weather: weather}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func epathSQLTabularRawQuantity(selector epathRealSQLTabularSelector, raw string) (float64, epathSQLQuantity, error) {
	factor, err := epathValidateSQLTabularSelector(selector)
	if err != nil {
		return 0, epathSQLQuantity{}, err
	}
	text := strings.TrimSpace(raw)
	parts := strings.Split(text, ".")
	if len(parts) != 2 || len(parts[0]) == 0 || len(parts[1]) != selector.DecimalPlaces {
		return 0, epathSQLQuantity{}, fmt.Errorf("annual Tabular cell does not match its explicit source display precision")
	}
	for _, digit := range parts[0] + parts[1] {
		if digit < '0' || digit > '9' {
			return 0, epathSQLQuantity{}, fmt.Errorf("annual Tabular cell is not finite nonnegative fixed-decimal energy")
		}
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || !epathOracleFinite(value) || value < 0 {
		return 0, epathSQLQuantity{}, fmt.Errorf("invalid annual Tabular energy %q", text)
	}
	energy, precision := value*factor, .5*math.Pow10(-selector.DecimalPlaces)*factor
	quantity := epathSQLBounded(energy, math.Max(0, energy-precision), energy+precision)
	if !quantity.valid() {
		return 0, epathSQLQuantity{}, fmt.Errorf("nonfinite converted annual Tabular energy")
	}
	return value, quantity, nil
}

func epathValidateSQLTabularObservation(observation epathSQLTabularObservation) error {
	value, quantity, err := epathSQLTabularRawQuantity(observation.Selector, observation.RawText)
	if err != nil {
		return err
	}
	low, high := quantity.bounds()
	gotLow, gotHigh := observation.Quantity.bounds()
	if observation.TabularDataIndex <= 0 || !observation.Quantity.valid() || value != observation.RawValue || quantity.Value != observation.Quantity.Value || low != gotLow || high != gotHigh {
		return fmt.Errorf("annual Tabular observation lost its original cell/value/display precision")
	}
	w := observation.Weather
	if w.Year != 2017 || w.EnvironmentIndex <= 0 || w.EnvironmentName == "" || w.TimeRows < 12 || w.FirstMonth != 1 || w.LastMonth != 12 || len(w.Months) != 12 || w.CoverageBasis != "monthly_intervals_and_cumulative_simulation_days" && w.CoverageBasis != "365_observed_calendar_and_simulation_days" {
		return fmt.Errorf("annual Tabular observation lacks original full-weather proof")
	}
	for index, month := range w.Months {
		if month != index+1 {
			return fmt.Errorf("annual Tabular observation has incomplete/duplicate calendar months")
		}
	}
	return nil
}
