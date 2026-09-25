package simulation

// Finite Furnace exception to the two-conversion-service contract. Missing
// Cooling consumption is absence, while delivered Cooling must be observed 0.
import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLHeatOnlySingleService(frames epathSQLFrames, model epathRealSQLModel, b *epathSQLHeatOnlyBinding) error {
	if b == nil {
		return fmt.Errorf("one Heating service requires the actual HeatOnly original/native binding")
	}
	if err := epathSQLValidateHeatOnlyBinding(b, &model); err != nil {
		return err
	}
	if len(model.Services) != 1 || model.Services[0].Service != "heating" || len(frames.DirectHVAC) != 0 || len(frames.DirectHVACSourceIdentities) != 0 {
		return fmt.Errorf("HeatOnly shared service cannot borrow direct HVAC observations")
	}
	zones, err := epathSQLAirLoopFanZones(b.Original.ServedZones)
	if err != nil || len(zones) != 3 || len(frames.Zones) != 3 {
		return fmt.Errorf("HeatOnly service lost its three original recipients")
	}
	seen := map[string]bool{}
	for _, load := range model.Loads {
		name := map[string]string{"cooling": "Zone Air System Sensible Cooling Energy", "heating": "Zone Air System Sensible Heating Energy"}[load.Service]
		keys, e := epathSQLAirLoopFanZones(load.Source.Keys)
		if name == "" || seen[load.Service] || load.Component != "sensible" || load.NativeRadiant != nil || load.Source.IsMeter || load.Source.AllowAbsent || !reflect.DeepEqual(load.Source.Alternatives, []epathRealSQLAlternative{{Name: name, Unit: "J"}}) || e != nil || !reflect.DeepEqual(keys, zones) {
			return fmt.Errorf("HeatOnly service requires both exact canonical Zone delivery observations")
		}
		seen[load.Service] = true
	}
	if len(seen) != 2 {
		return fmt.Errorf("HeatOnly missing canonical Heating/Cooling observations is not zero")
	}
	for _, site := range model.Site {
		if site.EndUse == "cooling" {
			return fmt.Errorf("HeatOnly cannot declare paid Cooling")
		}
		for _, a := range site.Source.Alternatives {
			if strings.HasPrefix(strings.ToLower(a.Name), "cooling:") {
				return fmt.Errorf("HeatOnly cannot relabel paid Cooling")
			}
		}
	}
	original, err := idf.Parse(b.OriginalText)
	if err != nil {
		return err
	}
	executed, err := idf.Parse(b.ExecutedText)
	if err != nil {
		return err
	}
	if _, err = epathSQLPVRequestBinding(original, executed, &b.OutputPlan, epathSQLPVNativeSpec{Name: "Cooling:Electricity", IsMeter: true}, "Monthly"); err != nil {
		return fmt.Errorf("HeatOnly absent paid Cooling lacks actual request: %w", err)
	}
	db, err := epathOpenOracleSQL(b.SQLPath)
	if err != nil {
		return err
	}
	defer db.Close()
	weather, err := epathReadOracleWeather(db)
	if err != nil {
		return err
	}
	if err := epathSQLHeatOnlyCombustionMeters(db, weather, frames, model, b, original, executed); err != nil {
		return err
	}
	var paid int
	if err = db.QueryRow(`SELECT COUNT(*) FROM ReportDataDictionary WHERE Name LIKE 'Cooling:%' COLLATE NOCASE`).Scan(&paid); err != nil {
		return err
	}
	if paid != 0 {
		return fmt.Errorf("HeatOnly paid Cooling is not absent in actual native SQL")
	}
	// Census the actual family before selecting rows: no hidden foreign key or
	// dictionary without data may disappear through the selected-row query.
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,Name,COALESCE(KeyValue,''),ReportingFrequency,Units,IsMeter,Type,TimestepType,IndexGroup,ScheduleName FROM ReportDataDictionary WHERE Name='Zone Air System Sensible Cooling Energy' COLLATE NOCASE AND ReportingFrequency='Monthly' COLLATE NOCASE`)
	if err != nil {
		return err
	}
	byID := map[int]string{}
	owners := map[string]bool{}
	exactZero := func(q epathSQLQuantity) bool {
		lo, hi := q.bounds()
		return q.valid() && q.Value == 0 && q.Error == 0 && lo == 0 && hi == 0
	}
	for rows.Next() {
		var id, meter int
		var name, key, frequency, unit, kind, step, group string
		var schedule sql.NullString
		if err = rows.Scan(&id, &name, &key, &frequency, &unit, &meter, &kind, &step, &group, &schedule); err != nil {
			rows.Close()
			return err
		}
		zone := strings.ToLower(key)
		owner := frames.Zones[zone]
		s, ok := frames.SourceIdentities[id]
		allowed := false
		for _, z := range zones {
			allowed = allowed || strings.EqualFold(z, key)
		}
		if id <= 0 || owners[zone] || !allowed || !ok || !strings.EqualFold(owner.Name, key) || owner.Multiplier != 1 || name != "Zone Air System Sensible Cooling Energy" || frequency != "Monthly" || unit != "J" || meter != 0 || kind != "Sum" || step != "HVAC System" || group != "System" || schedule.Valid && schedule.String != "" || s.DictionaryIndex != id || s.Name != name || !strings.EqualFold(s.KeyValue, key) || s.ReportingFrequency != frequency || s.SourceUnit != unit || s.IndexGroup != group || s.IsMeter || s.Rows != 12 || s.MissingRows != 0 || s.RawSum == nil || *s.RawSum != 0 || s.EnergyKWh == nil || *s.EnergyKWh != 0 {
			rows.Close()
			return fmt.Errorf("HeatOnly Cooling zero lost exact native dictionary/owner identity")
		}
		q, e := epathSQLMonthly(s, model.Precision)
		if e != nil || len(frames.SourceRaw[id]) != 12 || len(frames.SourceEffective[id]) != 12 || frames.SourceZone[id] != zone {
			rows.Close()
			return fmt.Errorf("HeatOnly Cooling has missing native/source months or owner")
		}
		for m, v := range q {
			key := epathSQLKey(zone, "cooling", m+1)
			load, known := frames.Loads[key]
			bucket := s.Months[m]
			if !known || !exactZero(v) || !exactZero(load) || !exactZero(frames.SourceRaw[id][m]) || !exactZero(frames.SourceEffective[id][m]) || bucket.MissingRows != 0 || bucket.RawSum == nil || *bucket.RawSum != 0 || !reflect.DeepEqual(frames.LoadSourceIDs[key], []int{id}) {
				rows.Close()
				return fmt.Errorf("HeatOnly Cooling is not exact known native/frame zero")
			}
		}
		owners[zone] = true
		byID[id] = zone
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(byID) != 3 {
		return fmt.Errorf("HeatOnly Cooling requires all three actual native dictionaries")
	}
	rows, err = db.Query(`SELECT r.ReportDataDictionaryIndex,t.EnvironmentPeriodIndex,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,t.WarmupFlag,r.Value FROM ReportData r JOIN ReportDataDictionary d USING(ReportDataDictionaryIndex) LEFT JOIN "Time" t USING(TimeIndex) WHERE d.Name='Zone Air System Sensible Cooling Energy' COLLATE NOCASE AND d.ReportingFrequency='Monthly' COLLATE NOCASE ORDER BY r.ReportDataIndex`)
	if err != nil {
		return err
	}
	defer rows.Close()
	calendar := map[string]bool{}
	for rows.Next() {
		var id, env, year, month, day, hour, minute, kind int
		var interval, value sql.NullFloat64
		var warmup sql.NullInt64
		if err = rows.Scan(&id, &env, &year, &month, &day, &hour, &minute, &interval, &kind, &warmup, &value); err != nil {
			return err
		}
		last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
		slot := fmt.Sprintf("%d/%d", id, month)
		if byID[id] == "" || calendar[slot] || env != weather.EnvironmentIndex || year != 2017 || month < 1 || month > 12 || day != last || hour != 24 || minute != 0 || kind != 3 || !interval.Valid || interval.Float64 != float64(last*1440) || warmup.Valid && warmup.Int64 != 0 || !value.Valid || value.Float64 != 0 {
			return fmt.Errorf("HeatOnly Cooling native month is missing/duplicate/nonzero/foreign")
		}
		calendar[slot] = true
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(calendar) != 36 {
		return fmt.Errorf("HeatOnly Cooling needs 36 actual native zero months")
	}
	if hash, e := epathReadRealFileHash(b.SQLPath); e != nil || hash != b.SQLSHA256 {
		return fmt.Errorf("HeatOnly native SQL changed during absent-service proof")
	}
	return nil
}

func epathSQLCompileHeatOnlyService(frames epathSQLFrames, model epathRealSQLModel, service epathRealSQLService, b *epathSQLHeatOnlyBinding) (*epathSQLDirectHVACServiceFrames, error) {
	if err := epathSQLHeatOnlySingleService(frames, model, b); err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(service, model.Services[0]) {
		return nil, fmt.Errorf("HeatOnly shared carrier constructor received another service")
	}
	// The constructor's default-false sharedOnly opt-in is supplied by the
	// separately frozen Central patch. It preserves all Monthly Site/Load math.
	// No DirectHVAC declaration, observed direct row or component is invented.
	return epathSQLCompileDirectHVACService(frames, model, service, true)
}
