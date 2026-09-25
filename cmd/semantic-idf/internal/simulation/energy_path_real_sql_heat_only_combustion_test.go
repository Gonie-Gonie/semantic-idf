package simulation

import (
	"database/sql"
	"fmt"
	"reflect"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// This finite full-service qualification is not part of the source-only fan
// binding. The delivered-load/fuel ratio is the existing efficiency policy
// (or load_to_fuel above one), never the nominal burner efficiency.
func epathSQLHeatOnlyCombustionMeters(db *sql.DB, weather epathRealSQLWeather, frames epathSQLFrames, model epathRealSQLModel, b *epathSQLHeatOnlyBinding, original, executed idf.Document) error {
	sites := map[string]string{"Heating:Electricity": "heating.electricity", "Heating:NaturalGas": "heating.natural_gas"}
	groups := map[string]string{"Heating:Electricity": "Facility:Electricity:Heating", "Heating:NaturalGas": "Facility:NaturalGas:Heating"}
	for _, name := range []string{"Heating:Electricity", "Heating:NaturalGas"} {
		if _, err := epathSQLPVRequestBinding(original, executed, &b.OutputPlan, epathSQLPVNativeSpec{Name: name, IsMeter: true}, "Monthly"); err != nil {
			return fmt.Errorf("HeatOnly combustion boundary lacks actual %s request: %w", name, err)
		}
	}
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,Name,COALESCE(KeyValue,''),ReportingFrequency,Units,IsMeter,Type,TimestepType,IndexGroup,ScheduleName FROM ReportDataDictionary WHERE Name COLLATE NOCASE IN ('Heating:Electricity','Heating:NaturalGas') AND ReportingFrequency='Monthly' COLLATE NOCASE`)
	if err != nil {
		return err
	}
	native := map[int]*epathRealSQLSource{}
	seen := map[string]bool{}
	for rows.Next() {
		var id, meter int
		var name, key, frequency, unit, kind, step, group string
		var schedule sql.NullString
		if err = rows.Scan(&id, &name, &key, &frequency, &unit, &meter, &kind, &step, &group, &schedule); err != nil {
			rows.Close()
			return err
		}
		if id <= 0 || sites[name] == "" || seen[name] || key != "" || frequency != "Monthly" || unit != "J" || meter != 1 || kind != "Sum" || step != "Zone" || group != groups[name] || schedule.Valid && schedule.String != "" {
			rows.Close()
			return fmt.Errorf("HeatOnly combustion meter dictionary is missing/duplicate/foreign")
		}
		seen[name] = true
		native[id] = &epathRealSQLSource{DictionaryIndex: id, Name: name, KeyValue: key, IsMeter: true, ReportingFrequency: frequency, SourceUnit: unit, IndexGroup: group, RawSum: epathOracleNumber(0), EnergyKWh: epathOracleNumber(0)}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(native) != 2 {
		return fmt.Errorf("HeatOnly combustion requires both exact native meters; missing is not zero")
	}
	// LEFT JOIN and no weather-row filter: orphan/foreign/warmup/NULL rows must
	// be rejected, not removed before the native twelve-month census.
	rows, err = db.Query(`SELECT r.ReportDataDictionaryIndex,t.EnvironmentPeriodIndex,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,t.WarmupFlag,r.Value FROM ReportData r JOIN ReportDataDictionary d USING(ReportDataDictionaryIndex) LEFT JOIN "Time" t USING(TimeIndex) WHERE d.Name COLLATE NOCASE IN ('Heating:Electricity','Heating:NaturalGas') AND d.ReportingFrequency='Monthly' COLLATE NOCASE ORDER BY d.ReportDataDictionaryIndex,t.TimeIndex,r.ReportDataIndex`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, env, year, month, day, hour, minute, kind int
		var interval, value sql.NullFloat64
		var warmup sql.NullInt64
		if err = rows.Scan(&id, &env, &year, &month, &day, &hour, &minute, &interval, &kind, &warmup, &value); err != nil {
			rows.Close()
			return err
		}
		s := native[id]
		last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
		if s == nil || month != len(s.Months)+1 || env != weather.EnvironmentIndex || year != 2017 || month < 1 || month > 12 || day != last || hour != 24 || minute != 0 || kind != 3 || !interval.Valid || interval.Float64 != float64(last*1440) || warmup.Valid && warmup.Int64 != 0 || !value.Valid || !epathOracleFinite(value.Float64) || value.Float64 < 0 || s.Name == "Heating:Electricity" && value.Float64 != 0 {
			rows.Close()
			return fmt.Errorf("HeatOnly combustion month is missing/duplicate/foreign or not single-fuel")
		}
		energy := value.Float64 * (1.0 / 3600000)
		s.Rows++
		*s.RawSum += value.Float64
		*s.EnergyKWh += energy
		s.Months = append(s.Months, epathRealSQLMonth{Month: month, Rows: 1, RawSum: epathOracleNumber(value.Float64), EnergyKWh: epathOracleNumber(energy)})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for id, source := range native {
		site := sites[source.Name]
		if source.Rows != 12 || source.Name == "Heating:NaturalGas" && *source.RawSum <= 0 || !reflect.DeepEqual(frames.SourceIdentities[id], *source) || !reflect.DeepEqual(frames.SiteSources[site], []int{id}) || len(frames.Site[site]) != 12 || len(frames.SourceRaw[id]) != 12 || len(frames.SourceEffective[id]) != 12 || frames.SourceZone[id] != "" {
			return fmt.Errorf("HeatOnly combustion native/source/site identity or positive annual gas is missing")
		}
		if _, annual := frames.SiteAnnual[site]; annual {
			return fmt.Errorf("HeatOnly combustion cannot replace native monthly meters with annual cells")
		}
		quantities, err := epathSQLMonthly(*source, model.Precision)
		if err != nil {
			return err
		}
		for month, nativeQ := range quantities {
			if frames.Site[site][month] == nil {
				return fmt.Errorf("HeatOnly combustion site month is absent")
			}
			for _, actual := range []epathSQLQuantity{frames.SourceRaw[id][month], frames.SourceEffective[id][month], *frames.Site[site][month]} {
				lo, hi := actual.bounds()
				wantLo, wantHi := nativeQ.bounds()
				if !actual.valid() || actual.Value != nativeQ.Value || lo != wantLo || hi != wantHi {
					return fmt.Errorf("HeatOnly combustion frame changed the native amount or precision interval")
				}
			}
		}
	}
	return nil
}

func epathSQLHeatOnlyExactZero(q epathSQLQuantity) bool {
	lo, hi := q.bounds()
	return q.valid() && q.Value == 0 && q.Error == 0 && lo == 0 && hi == 0
}

// Compute once per carrier compiler, after validating the detached original,
// requests, full native fan calendar, combustion meters and source/frame math.
// One inactive month may have a fresh direct-only accounting row even though
// the annual graph and other months retain allocated HVAC accounting.
func epathSQLHeatOnlyPlainMonthlyIDs(frames epathSQLFrames, model epathRealSQLModel, b *epathSQLHeatOnlyBinding, parts map[string]map[string]epathSQLZoneCarrierPart) (map[string]bool, error) {
	out := map[string]bool{}
	if b == nil && len(model.HeatOnlyFurnaces) == 0 {
		return out, nil
	}
	if err := epathSQLHeatOnlySingleService(frames, model, b); err != nil {
		return nil, err
	}
	for month, fan := range b.Native.Months {
		inactive := fan.NonZero == 0 && fan.RawJ == 0 && fan.BroadJ == 0
		for _, site := range []string{"heating.electricity", "heating.natural_gas", "fans.electricity"} {
			values := frames.Site[site]
			if len(values) != 12 || values[month] == nil || !values[month].valid() {
				return nil, fmt.Errorf("HeatOnly inactive-period proof lost a known native site month")
			}
			if site == "fans.electricity" && fan.NonZero == 0 && fan.RawJ == 0 && fan.BroadJ == 0 && !epathSQLHeatOnlyExactZero(*values[month]) {
				return nil, fmt.Errorf("HeatOnly native zero fan month has a nonzero/uncertain frame budget")
			}
			inactive = inactive && epathSQLHeatOnlyExactZero(*values[month])
		}
		if !inactive {
			continue
		}
		for zone := range frames.Zones {
			key := epathSQLZoneCarrierContext(zone, fmt.Sprintf("M%d", month+1))
			context := parts[key]
			heating, heatOK := context["service/heating"]
			fanPart, fanOK := context["fan"]
			_, electricOK := heating.ByCarrier["electricity"]
			_, gasOK := heating.ByCarrier["natural_gas"]
			_, fanElectricOK := fanPart.ByCarrier["electricity"]
			if !heatOK || !fanOK || !electricOK || !gasOK || !fanElectricOK || heating.Direct || fanPart.Direct || len(heating.ByCarrier) != 2 || len(fanPart.ByCarrier) != 1 {
				return nil, fmt.Errorf("HeatOnly inactive month lost required non-direct carrier components")
			}
			for _, part := range context {
				if part.Direct {
					continue
				}
				if part.Unavailable || part.AnnualTabular || len(part.ByCarrier) == 0 {
					return nil, fmt.Errorf("HeatOnly inactive allocation is absent, not known zero")
				}
				for _, carriers := range []map[string]epathSQLQuantity{part.ByCarrier, part.DirectByCarrier} {
					for _, q := range carriers {
						if !epathSQLHeatOnlyExactZero(q) {
							return nil, fmt.Errorf("HeatOnly inactive allocation has a nonzero/uncertain component in some carrier")
						}
					}
				}
			}
			out[key] = true
		}
	}
	return out, nil
}
