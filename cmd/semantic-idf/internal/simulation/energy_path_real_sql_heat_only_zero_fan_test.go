package simulation

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"
)

type epathSQLHeatOnlyNativeFanMonth struct {
	Hours, NonZero int
	RawJ, BroadJ   float64
}
type epathSQLHeatOnlyNativeFan struct {
	HourlyID, MonthlyMeterID int
	Months                   [12]epathSQLHeatOnlyNativeFanMonth
}

// Read both actual series once. An exact known-zero month is every original
// hourly J cell zero AND its one actual Monthly meter row zero, not a near-zero
// accumulated sum, absent dictionary, rounded chart, or synthetic denominator.
func epathSQLReadHeatOnlyNativeFan(path string, pool epathRealSQLFanPool) (epathSQLHeatOnlyNativeFan, error) {
	var out epathSQLHeatOnlyNativeFan
	db, err := epathOpenOracleSQL(path)
	if err != nil {
		return out, err
	}
	defer db.Close()
	weather, err := epathReadOracleWeather(db)
	if err != nil {
		return out, err
	}
	for _, spec := range []struct {
		name, key, frequency string
		meter                int
		id                   *int
	}{{"Air System Fan Electricity Energy", pool.Key, "Hourly", 0, &out.HourlyID}, {"Fans:Electricity", "", "Monthly", 1, &out.MonthlyMeterID}} {
		// The finite original has exactly one AirLoop fan and one broad meter.
		// No extra native key may hide outside the exact selected-key query.
		var familyCount int
		if e := db.QueryRow(`SELECT COUNT(*) FROM ReportDataDictionary WHERE Name=? COLLATE NOCASE AND ReportingFrequency=? COLLATE NOCASE`, spec.name, spec.frequency).Scan(&familyCount); e != nil {
			return out, e
		}
		if familyCount != 1 {
			return out, fmt.Errorf("HeatOnly native fan family census changed")
		}
		rows, e := db.Query(`SELECT ReportDataDictionaryIndex,Name,COALESCE(KeyValue,''),ReportingFrequency,Units,IsMeter,Type,TimestepType,IndexGroup,ScheduleName FROM ReportDataDictionary WHERE Name=? COLLATE NOCASE AND COALESCE(KeyValue,'')=? COLLATE NOCASE AND ReportingFrequency=? COLLATE NOCASE`, spec.name, spec.key, spec.frequency)
		if e != nil {
			return out, e
		}
		count := 0
		for rows.Next() {
			var id, meter int
			var name, key, frequency, unit, kind, step, group string
			var schedule sql.NullString
			if e = rows.Scan(&id, &name, &key, &frequency, &unit, &meter, &kind, &step, &group, &schedule); e != nil {
				rows.Close()
				return out, e
			}
			count++
			wantStep, wantGroup := "HVAC System", "System"
			if spec.meter == 1 {
				wantStep, wantGroup = "Zone", "Facility:Electricity:Fans"
			}
			if id <= 0 || name != spec.name || !strings.EqualFold(key, spec.key) || frequency != spec.frequency || unit != "J" || meter != spec.meter || kind != "Sum" || step != wantStep || group != wantGroup || schedule.Valid && schedule.String != "" {
				rows.Close()
				return out, fmt.Errorf("HeatOnly native fan dictionary identity changed")
			}
			*spec.id = id
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return out, e
		}
		if count != 1 {
			return out, fmt.Errorf("HeatOnly native fan dictionary missing/duplicate")
		}
	}
	rows, err := db.Query(`SELECT r.ReportDataDictionaryIndex,t.EnvironmentPeriodIndex,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,t.WarmupFlag,r.Value FROM ReportData r LEFT JOIN "Time" t USING(TimeIndex) WHERE r.ReportDataDictionaryIndex IN (?,?) ORDER BY r.ReportDataIndex`, out.HourlyID, out.MonthlyMeterID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	seen := [8760]bool{}
	broad := [12]bool{}
	count := 0
	for rows.Next() {
		var id, environment, year, month, day, hour, minute, intervalType int
		var interval, value sql.NullFloat64
		var warmup sql.NullInt64
		if err = rows.Scan(&id, &environment, &year, &month, &day, &hour, &minute, &interval, &intervalType, &warmup, &value); err != nil {
			return out, err
		}
		date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		if environment != weather.EnvironmentIndex || year != 2017 || month < 1 || month > 12 || day < 1 || date.Month() != time.Month(month) || date.Day() != day || minute != 0 || warmup.Valid && warmup.Int64 != 0 || !interval.Valid || !value.Valid || !epathOracleFinite(value.Float64) || value.Float64 < 0 {
			return out, fmt.Errorf("HeatOnly fan native row missing/negative/nonfinite/foreign")
		}
		m := &out.Months[month-1]
		if id == out.HourlyID {
			if hour < 1 || hour > 24 || interval.Float64 != 60 || intervalType != 1 {
				return out, fmt.Errorf("HeatOnly fan lacks exact native hourly interval")
			}
			slot := (date.YearDay()-1)*24 + hour - 1
			if seen[slot] {
				return out, fmt.Errorf("HeatOnly fan has duplicate native hour")
			}
			seen[slot] = true
			count++
			m.Hours++
			m.RawJ += value.Float64
			if value.Float64 != 0 {
				m.NonZero++
			}
		} else {
			last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
			if broad[month-1] || day != last || hour != 24 || intervalType != 3 || interval.Float64 != float64(last*1440) {
				return out, fmt.Errorf("HeatOnly broad fan meter lacks one exact native month")
			}
			broad[month-1] = true
			m.BroadJ = value.Float64
		}
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if count != 8760 {
		return out, fmt.Errorf("HeatOnly fan lacks 8760 actual rows")
	}
	for i, m := range out.Months {
		want := time.Date(2017, time.Month(i+2), 0, 0, 0, 0, 0, time.UTC).Day() * 24
		if m.Hours != want || !broad[i] || !epathOracleFinite(m.RawJ) {
			return out, fmt.Errorf("HeatOnly fan month incomplete")
		}
		if m.NonZero == 0 {
			if m.RawJ != 0 || m.BroadJ != 0 {
				return out, fmt.Errorf("HeatOnly zero hours disagree with native broad month")
			}
		} else if m.RawJ <= 0 || m.BroadJ <= 0 || !epathSQLFanPoolNear(m.RawJ/3600000, m.BroadJ/3600000) {
			return out, fmt.Errorf("HeatOnly positive fan month does not close broad meter")
		}
	}
	return out, nil
}

// Existing callers supply no binding and retain their positive-only contract.
// The full-model call supplies the original/executed/native proof, not a flag.
func epathSQLHeatOnlyFanZeroOptIn(observed epathRealOracleEvidence, frames epathSQLFrames, pools []epathRealSQLFanPool, bindings ...*epathSQLHeatOnlyBinding) ([12]bool, error) {
	var zero [12]bool
	if len(bindings) > 1 {
		return zero, fmt.Errorf("HeatOnly fan zero opt-in accepts at most one binding")
	}
	if len(bindings) == 0 || bindings[0] == nil {
		return zero, nil
	}
	b := bindings[0]
	if !reflect.DeepEqual(pools, b.Inputs.FanPools) || observed.sqlPath != b.SQLPath || observed.originalText != b.OriginalText || observed.executedText != b.ExecutedText || observed.outputPlan == nil || !reflect.DeepEqual(*observed.outputPlan, b.OutputPlan) {
		return zero, fmt.Errorf("HeatOnly fan proof differs from actual run/pool inputs")
	}
	if err := epathSQLValidateHeatOnlyBinding(b, nil); err != nil {
		return zero, err
	}
	items, err := epathSQLFanPoolObservations(observed.Sources, pools)
	if err != nil || len(items) != 1 {
		return zero, fmt.Errorf("HeatOnly opt-in lacks its native observed pool: %v", err)
	}
	meters, err := epathSQLSelect(observed.Sources, epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Fans:Electricity", Unit: "J"}}, Keys: []string{""}, IsMeter: true})
	if err != nil || len(meters) != 1 || items[0].Source.DictionaryIndex != b.Native.HourlyID || meters[0].DictionaryIndex != b.Native.MonthlyMeterID || meters[0].Rows != 12 || meters[0].MissingRows != 0 || len(meters[0].Months) != 12 {
		return zero, fmt.Errorf("HeatOnly native/observed source identity mismatch")
	}
	site := frames.Site[pools[0].SiteID]
	if len(site) != 12 {
		return zero, fmt.Errorf("HeatOnly fan site months missing")
	}
	for i, native := range b.Native.Months {
		if native.NonZero != 0 {
			continue
		}
		h, m := items[0].Source.Months[i], meters[0].Months[i]
		if h.Month != i+1 || h.Rows != native.Hours || h.MissingRows != 0 || h.RawSum == nil || h.EnergyKWh == nil || *h.RawSum != 0 || *h.EnergyKWh != 0 || m.Month != i+1 || m.Rows != 1 || m.MissingRows != 0 || m.RawSum == nil || m.EnergyKWh == nil || *m.RawSum != 0 || *m.EnergyKWh != 0 || site[i] == nil || !site[i].valid() {
			return zero, fmt.Errorf("HeatOnly zero month is not exact known native/site zero")
		}
		lo, hi := site[i].bounds()
		if site[i].Value != 0 || site[i].Error != 0 || lo != 0 || hi != 0 {
			return zero, fmt.Errorf("HeatOnly zero fan month has a positive/uncertain site budget")
		}
		zero[i] = true
	}
	return zero, nil
}

// Run only after boundary Prepare. Numeric zero never licenses an allocation
// edge, including an invented zero-valued edge on a pruned inactive period.
func epathSQLCheckHeatOnlyInactiveFanGraph(bundle PurposeResultBundle, checks epathSQLModelChecks) error {
	if err := epathSQLCheckHeatOnlyContextGraph(bundle, checks); err != nil {
		return err
	}
	if checks.HeatOnly == nil {
		return nil
	}
	var out epathSQLModelCoverageReport
	contexts := epathSQLCoverageContexts(bundle, &out)
	if len(out.Failures) != 0 {
		return fmt.Errorf("HeatOnly inactive graph wrappers invalid: %s", out.Failures[0].Message)
	}
	for _, c := range contexts {
		months := epathSQLPeriodMonths(c.period)
		inactive := len(months) > 0
		for _, m := range months {
			inactive = inactive && checks.HeatOnly.Native.Months[m-1].NonZero == 0
		}
		if !inactive {
			continue
		}
		fans := map[string]bool{}
		for _, n := range c.nodes {
			if n.Level == "end_use" && n.EndUse == "fans" {
				fans[n.ID] = true
				if n.Value != 0 || n.RawValue != 0 || n.EffectiveValue != 0 || n.AllocatedValue != 0 || n.AllocationApplied {
					return fmt.Errorf("HeatOnly inactive fan became numeric allocation")
				}
			}
		}
		fanID := fmt.Sprintf("sql-rdd-%d", checks.HeatOnly.Native.HourlyID)
		for _, link := range c.links {
			allocatedPool := false
			for _, id := range link.SourceIDs {
				allocatedPool = allocatedPool || id == fanID
			}
			if fans[link.FromID] || fans[link.ToID] || allocatedPool {
				return fmt.Errorf("HeatOnly inactive fan has an invented flow, even at zero")
			}
		}
	}
	return nil
}
