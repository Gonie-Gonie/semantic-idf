package simulation

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Purchased conditioning boundaries only. Ventilation sensible/latent transfer
// and its separate paid fan are not a Heating or Cooling conversion service.
func epathSQLSimpleVentilationPaidConditioning(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(n, "heating:") || strings.HasPrefix(n, "cooling:") {
		return true
	}
	for _, family := range []string{"cooling.coil.electricity", "cooling.coil.crankcase_electricity", "heating.coil.natural_gas", "heating.coil.ancillary_natural_gas", "heating.coil.electricity", "heating.coil.dx_electricity", "heating.coil.defrost_electricity", "heating.coil.crankcase_electricity", "heating.baseboard.electricity"} {
		paid, _, _, _, ok := epathSQLDirectHVACTaxonomy(family)
		if ok && n == strings.ToLower(paid) {
			return true
		}
	}
	return false
}

// No retained flag: the existing service compiler supplies its actual evidence
// at this boundary. All ordinary two-service callers retain their old contract.
func epathSQLSimpleVentilationZeroServices(frames epathSQLFrames, model epathRealSQLModel, observed epathRealOracleEvidence) error {
	if len(model.Services) != 0 || len(model.DirectHVACComponents) != 1 || model.DirectHVACComponents[0].ID != epathSQLSimpleVentilationFanFamily || len(model.FanPools)+len(model.AirLoopFans)+len(model.HVACConsumptionPools)+len(model.NativeVRFSystems)+len(model.PoolSystems)+len(model.PVSystems)+len(model.HeatOnlyFurnaces) != 0 {
		return fmt.Errorf("zero services require only the finite native-direct ventilation cohort")
	}
	declaration := model.DirectHVACComponents[0]
	if observed.sqlPath == "" || strings.TrimSpace(observed.originalText) == "" || strings.TrimSpace(observed.executedText) == "" || observed.outputPlan == nil {
		return fmt.Errorf("zero services require actual original/executed/native evidence")
	}
	original, err := idf.Parse(observed.originalText)
	if err != nil {
		return err
	}
	executed, err := idf.Parse(observed.executedText)
	if err != nil {
		return err
	}
	for _, doc := range []idf.Document{original, executed} {
		if err = epathSQLValidateSimpleVentilationOriginal(doc, declaration); err != nil {
			return err
		}
	}
	a, b := epathSQLPVHVACPhysical(original), epathSQLPVHVACPhysical(executed)
	if len(a) != len(b) {
		return fmt.Errorf("zero-service executed physical census changed")
	}
	for i, o := range a {
		if o.ObjectType != b[i].ObjectType || o.ObjectName != b[i].ObjectName || !reflect.DeepEqual(o.Fields, b[i].Fields) {
			return fmt.Errorf("zero-service executed physical field changed")
		}
	}
	for _, name := range []string{"Heating:Electricity", "Cooling:Electricity", "Heating:NaturalGas"} {
		if _, err = epathSQLPVRequestBinding(original, executed, observed.outputPlan, epathSQLPVNativeSpec{Name: name, IsMeter: true}, "Monthly"); err != nil {
			return fmt.Errorf("zero-service paid absence lacks actual request: %w", err)
		}
	}
	if len(frames.Zones) != 3 || len(model.Loads) != 2 {
		return fmt.Errorf("zero-service proof needs all original Zones and both delivered-load families")
	}
	keys := []string{"zone 1", "zone 2", "zone 3"}
	seen := map[string]bool{}
	for _, load := range model.Loads {
		name := map[string]string{"cooling": "Zone Air System Sensible Cooling Energy", "heating": "Zone Air System Sensible Heating Energy"}[load.Service]
		actual, e := epathSQLAirLoopFanZones(load.Source.Keys)
		if name == "" || seen[load.Service] || load.Component != "sensible" || load.NativeRadiant != nil || load.Source.IsMeter || load.Source.AllowAbsent || !reflect.DeepEqual(load.Source.Alternatives, []epathRealSQLAlternative{{Name: name, Unit: "J"}}) || e != nil || !reflect.DeepEqual(actual, keys) {
			return fmt.Errorf("zero-service proof cannot substitute missing/noncanonical delivery for zero")
		}
		seen[load.Service] = true
	}
	for _, site := range model.Site {
		if site.EndUse == "heating" || site.EndUse == "cooling" {
			return fmt.Errorf("zero-service model declares purchased conditioning")
		}
		for _, alternative := range site.Source.Alternatives {
			if epathSQLSimpleVentilationPaidConditioning(alternative.Name) {
				return fmt.Errorf("zero-service model relabels purchased conditioning")
			}
		}
	}
	if len(model.Auxiliaries) != 1 || model.Auxiliaries[0].SiteID != declaration.SiteID || model.Auxiliaries[0].Weight != "native_direct" || model.Auxiliaries[0].AllocationMethod != "direct_only" {
		return fmt.Errorf("zero-service ventilation requires its actual native-direct fan consumer")
	}
	fans, err := epathSQLCompileDirectFans(frames, model)
	if err != nil || fans == nil || len(fans.Sources) != 3 || len(frames.DirectHVACSourceIdentities) != 3 || len(frames.DirectHVAC) != 36 {
		return fmt.Errorf("zero-service ventilation lost its nonempty complete native fan cohort: %v", err)
	}
	for _, identity := range frames.DirectHVACSourceIdentities {
		if identity.FamilyID != epathSQLSimpleVentilationFanFamily || identity.Service != "fans" {
			return fmt.Errorf("zero-service frame contains conditioning consumption")
		}
	}
	hash, err := epathReadRealFileHash(observed.sqlPath)
	if err != nil {
		return err
	}
	db, err := epathOpenOracleSQL(observed.sqlPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = epathSQLValidateOriginalZoneMultipliers(db, observed.originalText, epathSQLOriginalMultiplierContract); err != nil {
		return err
	}
	if err = epathSQLValidateSimpleVentilationDictionary(db, declaration); err != nil {
		return err
	}
	weather, err := epathReadOracleWeather(db)
	if err != nil {
		return err
	}
	rows, err := db.Query(`SELECT Name FROM ReportDataDictionary`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		if epathSQLSimpleVentilationPaidConditioning(name) {
			rows.Close()
			return fmt.Errorf("zero-service native SQL contains purchased conditioning identity %s", name)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = db.Query(`SELECT ReportDataDictionaryIndex,Name,KeyValue,ReportingFrequency,Units,IsMeter,Type,TimestepType,IndexGroup,ScheduleName FROM ReportDataDictionary WHERE Name COLLATE NOCASE IN ('Zone Air System Sensible Cooling Energy','Zone Air System Sensible Heating Energy') AND ReportingFrequency='Monthly' COLLATE NOCASE`)
	if err != nil {
		return err
	}
	exactZero := func(q epathSQLQuantity) bool {
		lo, hi := q.bounds()
		return q.valid() && q.Value == 0 && q.Error == 0 && lo == 0 && hi == 0
	}
	byID := map[int]string{}
	owners := map[string]bool{}
	for rows.Next() {
		var id, meter int
		var name, key, frequency, unit, kind, step, group string
		var schedule sql.NullString
		if err = rows.Scan(&id, &name, &key, &frequency, &unit, &meter, &kind, &step, &group, &schedule); err != nil {
			rows.Close()
			return err
		}
		service := map[string]string{"Zone Air System Sensible Cooling Energy": "cooling", "Zone Air System Sensible Heating Energy": "heating"}[name]
		zone := strings.ToLower(key)
		owner := frames.Zones[zone]
		s, ok := frames.SourceIdentities[id]
		ownerKey := zone + "|" + service
		if id <= 0 || service == "" || owners[ownerKey] || (zone != "zone 1" && zone != "zone 2" && zone != "zone 3") || !strings.EqualFold(owner.Name, key) || owner.Multiplier != 1 || !ok || frequency != "Monthly" || unit != "J" || meter != 0 || kind != "Sum" || step != "HVAC System" || group != "System" || schedule.Valid && schedule.String != "" || s.DictionaryIndex != id || s.Name != name || !strings.EqualFold(s.KeyValue, key) || s.ReportingFrequency != frequency || s.SourceUnit != unit || s.IndexGroup != group || s.IsMeter || s.Rows != 12 || s.MissingRows != 0 || s.RawSum == nil || *s.RawSum != 0 || s.EnergyKWh == nil || *s.EnergyKWh != 0 {
			rows.Close()
			return fmt.Errorf("zero-service delivery lost exact original/native identity")
		}
		q, e := epathSQLMonthly(s, model.Precision)
		if e != nil || len(frames.SourceRaw[id]) != 12 || len(frames.SourceEffective[id]) != 12 || frames.SourceZone[id] != zone {
			rows.Close()
			return fmt.Errorf("zero-service delivery has missing source months/owner")
		}
		for m, v := range q {
			k := epathSQLKey(zone, service, m+1)
			load, known := frames.Loads[k]
			bucket := s.Months[m]
			if !known || !exactZero(v) || !exactZero(load) || !exactZero(frames.SourceRaw[id][m]) || !exactZero(frames.SourceEffective[id][m]) || bucket.MissingRows != 0 || bucket.RawSum == nil || *bucket.RawSum != 0 || !reflect.DeepEqual(frames.LoadSourceIDs[k], []int{id}) {
				rows.Close()
				return fmt.Errorf("zero-service delivery must be exact known native/frame zero")
			}
		}
		owners[ownerKey] = true
		byID[id] = ownerKey
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(byID) != 6 {
		return fmt.Errorf("zero-service delivery requires six actual native dictionaries")
	}
	// Bind the existing three native fan observations and their one parent to
	// this same SQL, not just a plausible retained frame. This adds 48 bounded
	// rows to the 72 zero-load rows; no full-source rescan or new fan formula.
	native := map[int]epathRealSQLSource{}
	for id := range byID {
		native[id] = frames.SourceIdentities[id]
	}
	native[fans.MeterSource.DictionaryIndex] = fans.MeterSource
	for id, identity := range frames.DirectHVACSourceIdentities {
		native[id] = identity.Source
	}
	if len(native) != 10 {
		return fmt.Errorf("zero-service fan and delivery dictionaries overlap")
	}
	var fanMeters int
	if err = db.QueryRow(`SELECT COUNT(*) FROM ReportDataDictionary WHERE Name='Fans:Electricity' COLLATE NOCASE AND ReportingFrequency='Monthly' COLLATE NOCASE`).Scan(&fanMeters); err != nil || fanMeters != 1 {
		return fmt.Errorf("zero-service native fan parent must be unique: %v", err)
	}
	rows, err = db.Query(`SELECT r.ReportDataDictionaryIndex,d.Name,COALESCE(d.KeyValue,''),d.Units,d.IsMeter,d.IndexGroup,d.Type,d.TimestepType,d.ScheduleName,t.EnvironmentPeriodIndex,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,t.WarmupFlag,r.Value FROM ReportData r JOIN ReportDataDictionary d USING(ReportDataDictionaryIndex) LEFT JOIN "Time" t USING(TimeIndex) WHERE d.Name COLLATE NOCASE IN ('Zone Air System Sensible Cooling Energy','Zone Air System Sensible Heating Energy','Zone Ventilation Fan Electricity Energy','Fans:Electricity') AND d.ReportingFrequency='Monthly' COLLATE NOCASE ORDER BY r.ReportDataIndex`)
	if err != nil {
		return err
	}
	defer rows.Close()
	calendar := map[string]bool{}
	for rows.Next() {
		var id, meter, env, year, month, day, hour, minute, kind int
		var name, key, unit, group, reportType, step string
		var schedule sql.NullString
		var interval, value sql.NullFloat64
		var warmup sql.NullInt64
		if err = rows.Scan(&id, &name, &key, &unit, &meter, &group, &reportType, &step, &schedule, &env, &year, &month, &day, &hour, &minute, &interval, &kind, &warmup, &value); err != nil {
			return err
		}
		last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
		slot := fmt.Sprintf("%d/%d", id, month)
		source, ok := native[id]
		if !ok || source.DictionaryIndex != id || source.Name != name || !strings.EqualFold(source.KeyValue, key) || source.SourceUnit != unit || source.IndexGroup != group || meter != 0 && meter != 1 || source.IsMeter != (meter != 0) || reportType != "Sum" || meter == 0 && step != "HVAC System" || meter == 1 && step != "Zone" || schedule.Valid && schedule.String != "" || source.ReportingFrequency != "Monthly" || calendar[slot] || env != weather.EnvironmentIndex || year != 2017 || month < 1 || month > 12 || day != last || hour != 24 || minute != 0 || kind != 3 || !interval.Valid || interval.Float64 != float64(last*1440) || warmup.Valid && warmup.Int64 != 0 || !value.Valid || !epathOracleFinite(value.Float64) || value.Float64 < 0 || len(source.Months) != 12 {
			return fmt.Errorf("zero-service native source month is missing/duplicate/nonfinite/foreign")
		}
		bucket := source.Months[month-1]
		if bucket.Rows != 1 || bucket.MissingRows != 0 || bucket.RawSum == nil || value.Float64 != *bucket.RawSum || byID[id] != "" && value.Float64 != 0 {
			return fmt.Errorf("zero-service native fan/delivery month differs from independently observed source")
		}
		calendar[slot] = true
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(calendar) != 120 {
		return fmt.Errorf("zero-service proof needs 72 native zero-load and 48 native fan/parent months")
	}
	if current, e := epathReadRealFileHash(observed.sqlPath); e != nil || current != hash {
		return fmt.Errorf("zero-service SQL changed during native proof")
	}
	return nil
}
