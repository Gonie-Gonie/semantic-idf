package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Private reported allocation evidence, never another graph end-use series.
// Monthly values retain SQL precision; rendering/accounting rounds only later.
type energyPathFanPool struct {
	Source      EnergyDataSource
	AirLoopName string
	Monthly     map[int]float64
	Invalid     bool
}

func readEnergyPathFanPools(db *sql.DB, sourceFile string, plan *PurposeRunPlan) ([]energyPathFanPool, error) {
	if !energyExplanationPlanUsesEnergyPath(plan) {
		return nil, nil
	}
	ready, err := sqlHasTables(db, "ReportDataDictionary", "ReportData", "Time", "EnvironmentPeriods")
	if err != nil || !ready {
		return nil, err
	}
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex, KeyValue, Name, Units, ReportingFrequency
FROM ReportDataDictionary WHERE IsMeter=0 AND LOWER(TRIM(Name)) IN
('air system fan electricity energy','air system fan electric energy')
AND LOWER(TRIM(ReportingFrequency))='hourly'
ORDER BY ReportDataDictionaryIndex`)
	if err != nil {
		return nil, err
	}
	pools := []energyPathFanPool{}
	indices := []int{}
	for rows.Next() {
		var id int
		var key, name, unit, frequency string
		if err := rows.Scan(&id, &key, &name, &unit, &frequency); err != nil {
			rows.Close()
			return nil, err
		}
		pool := energyPathFanPool{AirLoopName: strings.TrimSpace(key), Monthly: map[int]float64{}, Source: EnergyDataSource{
			ID: fmt.Sprintf("sql-rdd-%d", id), SourceType: "sql_report_data", KeyValue: key, Name: name,
			Units: unit, SourceUnit: unit, NormalizedUnit: "kWh", ReportingFrequency: frequency, AggregationMethod: "sum",
			TableName: "ReportData", Explanation: "Reported AirLoop fan energy; allocation context, not an additional Building end-use meter.",
		}}
		pool.Invalid = id <= 0 || pool.AirLoopName == "" || pool.AirLoopName == "*" || !strings.EqualFold(strings.TrimSpace(unit), "J") || !strings.EqualFold(strings.TrimSpace(frequency), "Hourly")
		pools, indices = append(pools, pool), append(indices, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(pools) == 0 {
		return nil, nil
	}
	byKey := map[string]int{}
	for _, pool := range pools {
		byKey[strings.ToLower(pool.AirLoopName)]++
	}
	for i := range pools {
		pools[i].Invalid = pools[i].Invalid || byKey[strings.ToLower(pools[i].AirLoopName)] != 1
	}
	var environment, count int
	if err := db.QueryRow(`SELECT COUNT(*), COALESCE(MIN(EnvironmentPeriodIndex),0) FROM EnvironmentPeriods WHERE EnvironmentType=3`).Scan(&count, &environment); err != nil {
		return nil, err
	}
	if count != 1 {
		for i := range pools {
			pools[i].Invalid = true
		}
		return pools, nil
	}
	// Establish complete, unique calendar-hour coverage before reading values.
	times, err := db.Query(`SELECT TimeIndex, Year, Month, Day, Hour, Minute, "Interval" FROM "Time"
WHERE EnvironmentPeriodIndex=? AND IntervalType=1 AND (WarmupFlag=0 OR WarmupFlag IS NULL) ORDER BY TimeIndex`, environment)
	if err != nil {
		return nil, err
	}
	monthByTime := map[int64]int{}
	monthCounts, expectedCounts := map[int]int{}, map[int]int{}
	monthYears := map[int]int{}
	badMonths, seenSlots := map[int]bool{}, map[string]bool{}
	for times.Next() {
		var id int64
		var year, month, day, hour, minute, interval int
		if err := times.Scan(&id, &year, &month, &day, &hour, &minute, &interval); err != nil {
			times.Close()
			return nil, err
		}
		if month < 1 || month > 12 || year < 1 {
			continue
		}
		expected := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day() * 24
		slot := fmt.Sprintf("%d/%d/%d", month, day, hour)
		if (monthYears[month] != 0 && monthYears[month] != year) || (expectedCounts[month] != 0 && expectedCounts[month] != expected) || seenSlots[slot] || interval != 60 || minute != 0 || hour < 1 || hour > 24 || day < 1 || day > expected/24 {
			badMonths[month] = true
		}
		seenSlots[slot] = true
		expectedCounts[month] = expected
		monthYears[month] = year
		monthCounts[month]++
		monthByTime[id] = month
	}
	if err := times.Err(); err != nil {
		times.Close()
		return nil, err
	}
	times.Close()
	type poolValues struct {
		index   int
		sums    map[int]float64
		counts  map[int]int
		invalid map[int]bool
		seen    map[int64]bool
	}
	accumulators := []*poolValues{}
	byDictionary := map[int][]*poolValues{}
	placeholders := []string{}
	args := []any{}
	for index := range pools {
		if pools[index].Invalid {
			continue
		}
		state := &poolValues{index: index, sums: map[int]float64{}, counts: map[int]int{}, invalid: map[int]bool{}, seen: map[int64]bool{}}
		accumulators = append(accumulators, state)
		dictionary := indices[index]
		if len(byDictionary[dictionary]) == 0 {
			placeholders = append(placeholders, "?")
			args = append(args, dictionary)
		}
		byDictionary[dictionary] = append(byDictionary[dictionary], state)
	}
	if len(accumulators) > 0 {
		// Native ReportData has no dictionary index. One filtered scan avoids
		// rescanning the annual table for every AirLoop. Time-first ordering
		// retains each pool's original floating-point accumulation sequence.
		args = append(args, environment)
		values, err := db.Query(`SELECT r.ReportDataDictionaryIndex, r.TimeIndex, r.Value FROM ReportData r JOIN "Time" t ON t.TimeIndex=r.TimeIndex
WHERE r.ReportDataDictionaryIndex IN (`+strings.Join(placeholders, ",")+`) AND t.EnvironmentPeriodIndex=? AND t.IntervalType=1
AND (t.WarmupFlag=0 OR t.WarmupFlag IS NULL) ORDER BY r.TimeIndex, r.ReportDataDictionaryIndex`, args...)
		if err != nil {
			return nil, err
		}
		for values.Next() {
			var dictionary int
			var id int64
			var value sql.NullFloat64
			if err := values.Scan(&dictionary, &id, &value); err != nil {
				values.Close()
				return nil, err
			}
			month, exists := monthByTime[id]
			if !exists {
				continue
			}
			for _, state := range byDictionary[dictionary] {
				state.counts[month]++
				if state.seen[id] || !value.Valid || math.IsNaN(value.Float64) || math.IsInf(value.Float64, 0) || value.Float64 < 0 {
					state.invalid[month] = true
				} else {
					state.sums[month] += value.Float64 / 3600000 // no per-hour rounding
				}
				state.seen[id] = true
			}
		}
		if err := values.Err(); err != nil {
			values.Close()
			return nil, err
		}
		values.Close()
	}
	for _, state := range accumulators {
		for month, expected := range expectedCounts {
			if !badMonths[month] && !state.invalid[month] && state.counts[month] == expected && monthCounts[month] == expected && energyPathFinite(state.sums[month]) {
				pools[state.index].Monthly[month] = state.sums[month]
			}
		}
	}
	sort.SliceStable(pools, func(i, j int) bool { return pools[i].Source.ID < pools[j].Source.ID })
	return pools, nil
}

func energyPathFanPoolValue(pool energyPathFanPool, period string) (float64, bool) {
	if pool.Invalid {
		return 0, false
	}
	if period == "annual" {
		if len(pool.Monthly) != 12 {
			return 0, false
		}
		total := 0.0
		for month := 1; month <= 12; month++ {
			value, ok := pool.Monthly[month]
			if !ok || !energyPathFinite(value) || value < 0 {
				return 0, false
			}
			total += value
		}
		return total, energyPathFinite(total)
	}
	for month := 1; month <= 12; month++ {
		if period == fmt.Sprintf("M%d", month) {
			value, ok := pool.Monthly[month]
			return value, ok && energyPathFinite(value) && value >= 0
		}
	}
	return 0, false
}
