package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

type energySourceHourlyValues struct {
	values  []float64
	seen    []bool
	count   int
	invalid bool
}

type energySourceHourlyCollector struct {
	labels []string
	times  map[int64]int
	series map[int]*energySourceHourlyValues
}

func newEnergySourceHourlyCollector(db *sql.DB) *energySourceHourlyCollector {
	var environment, count int
	if err := db.QueryRow(`SELECT COUNT(*), COALESCE(MIN(EnvironmentPeriodIndex),0) FROM EnvironmentPeriods WHERE EnvironmentType=3`).Scan(&count, &environment); err != nil || count != 1 {
		return nil
	}
	rows, err := db.Query(`SELECT TimeIndex, Year, Month, Day, Hour, Minute, "Interval" FROM "Time"
WHERE EnvironmentPeriodIndex=? AND IntervalType=1 AND (WarmupFlag=0 OR WarmupFlag IS NULL) ORDER BY TimeIndex`, environment)
	if err != nil {
		return nil
	}
	defer rows.Close()
	collector := &energySourceHourlyCollector{times: map[int64]int{}, series: map[int]*energySourceHourlyValues{}}
	seen := map[string]bool{}
	for rows.Next() {
		var id int64
		var year, month, day, hour, minute, interval int
		if err := rows.Scan(&id, &year, &month, &day, &hour, &minute, &interval); err != nil {
			return nil
		}
		if year < 1 || month < 1 || month > 12 || day < 1 || day > time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day() || hour < 1 || hour > 24 || minute != 0 || interval != 60 {
			return nil
		}
		label := fmt.Sprintf("%02d-%02d %02d:00", month, day, hour)
		// Labels intentionally retain the EnergyPlus end-of-hour convention.
		// More than one year/calendar instance must not collapse onto that axis.
		if seen[label] {
			return nil
		}
		seen[label] = true
		collector.times[id] = len(collector.labels)
		collector.labels = append(collector.labels, label)
	}
	if rows.Err() != nil || len(collector.labels) == 0 {
		return nil
	}
	return collector
}

func (collector *energySourceHourlyCollector) observe(row SQLSeriesRow, dictionary energyExplanationDictionary) {
	if collector == nil || !strings.EqualFold(strings.TrimSpace(dictionary.reportingFrequency), "Hourly") {
		return
	}
	index, included := collector.times[row.TimeIndex]
	if !included {
		return
	}
	series := collector.series[row.DictionaryIndex]
	if series == nil {
		series = &energySourceHourlyValues{values: make([]float64, len(collector.labels)), seen: make([]bool, len(collector.labels))}
		collector.series[row.DictionaryIndex] = series
	}
	if series.seen[index] {
		series.invalid = true
	} else {
		series.seen[index] = true
		series.count++
	}
	// The axis above proves an actual reported sixty-minute weather-run interval.
	// This integrates W/kW with its real duration, never with downsampled gaps.
	value, unit := energyExplanationSQLValue(row.Value.Float64, dictionary, 1)
	if !row.Value.Valid || !strings.EqualFold(unit, "kWh") || math.IsNaN(value) || math.IsInf(value, 0) {
		series.invalid = true
	}
	series.values[index] = value
}

func (collector *energySourceHourlyCollector) energy(dictionaryIndex int) *EnergySourceHourlyEnergy {
	if collector == nil {
		return nil
	}
	series := collector.series[dictionaryIndex]
	if series == nil || series.invalid || series.count != len(collector.labels) {
		return nil
	}
	// The completed collector is no longer mutated. Retain its compact values
	// directly; all exported sources use the explanation's one shared label axis.
	return &EnergySourceHourlyEnergy{Unit: "kWh", Basis: "reported_source", Values: series.values}
}
