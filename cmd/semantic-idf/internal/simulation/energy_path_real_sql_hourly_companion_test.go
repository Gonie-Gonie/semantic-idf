package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
)

type epathRealSQLHourlyCompanion struct {
	Name string   `json:"name"`
	Unit string   `json:"unit"`
	Keys []string `json:"keys"`
}

// The original Hourly observation is trace evidence for the same physical
// Monthly input, not another driver amount or an allocation denominator term.
type epathSQLHourlyCompanionIdentity struct {
	Source, Authority  epathRealSQLSource
	ZoneName           string
	Multiplier         float64
	ReportedScalar     float64
	HourlyLabels       []string
	HourlyValues       []float64
	NativeAbsoluteSums [12]float64
}

func epathSQLHourlyCompanionUnit(name string) string {
	switch name {
	case "Surface Inside Face Convection Heat Gain Energy", "Zone Total Internal Convective Heating Energy",
		"Zone Electric Equipment Convective Heating Energy", "Zone Electric Equipment Latent Gain Energy",
		"Zone Gas Equipment Convective Heating Energy", "Zone Gas Equipment Latent Gain Energy",
		"Zone Hot Water Equipment Convective Heating Energy", "Zone Hot Water Equipment Latent Gain Energy",
		"Zone Infiltration Latent Heat Gain Energy", "Zone Infiltration Latent Heat Loss Energy",
		"Zone Infiltration Sensible Heat Gain Energy", "Zone Infiltration Sensible Heat Loss Energy",
		"Zone Lights Convective Heating Energy", "Zone People Convective Heating Energy", "Zone People Latent Gain Energy":
		return "J"
	case "Zone Air Heat Balance Air Energy Storage Rate", "Zone Air Heat Balance Surface Convection Rate", "Zone Air Heat Balance Outdoor Air Transfer Rate":
		return "W"
	}
	return ""
}

func epathSQLValidateHourlyCompanion(p epathSQLHourlyCompanionIdentity) error {
	if p.Source.DictionaryIndex <= 0 || p.Authority.DictionaryIndex <= 0 || p.Source.DictionaryIndex == p.Authority.DictionaryIndex || p.Source.IsMeter || p.Authority.IsMeter || p.Source.Name != p.Authority.Name || !strings.EqualFold(p.Source.KeyValue, p.Authority.KeyValue) || p.Source.SourceUnit != p.Authority.SourceUnit || epathSQLHourlyCompanionUnit(p.Source.Name) != p.Source.SourceUnit || p.Source.ReportingFrequency != "Hourly" || p.Authority.ReportingFrequency != "Monthly" || p.Source.Rows != 8760 || p.Authority.Rows != 12 || p.Source.MissingRows != 0 || p.Authority.MissingRows != 0 || p.ZoneName == "" || !epathOracleFinite(p.Multiplier) || p.Multiplier <= 0 || len(p.Source.Months) != 12 || len(p.Authority.Months) != 12 || len(p.HourlyValues) != 8760 || len(p.HourlyLabels) != 8760 || !epathOracleFinite(p.ReportedScalar) {
		return fmt.Errorf("Hourly companion %d lacks exact original selected-Monthly identity/owner/calendar", p.Source.DictionaryIndex)
	}
	for i, h := range p.Source.Months {
		m := p.Authority.Months[i]
		hours := time.Date(2017, time.Month(i+2), 0, 0, 0, 0, 0, time.UTC).Day() * 24
		if h.Month != i+1 || m.Month != i+1 || h.Rows != hours || m.Rows != 1 || h.MissingRows != 0 || m.MissingRows != 0 || h.EnergyKWh == nil || m.EnergyKWh == nil || !epathOracleFinite(p.NativeAbsoluteSums[i]) || p.NativeAbsoluteSums[i] < 0 || !epathSQLHourlyCompanionNativeNear(*h.EnergyKWh, *m.EnergyKWh, p.NativeAbsoluteSums[i], hours) {
			return fmt.Errorf("Hourly companion %d M%d lost native equivalence to Monthly %d", p.Source.DictionaryIndex, i+1, p.Authority.DictionaryIndex)
		}
	}
	return nil
}

// This bound is only native binary accumulation error, never a 0.001 kWh
// presentation allowance. Signed cancellation uses original absolute row sums.
func epathSQLHourlyCompanionNativeNear(a, b, absoluteSum float64, rows int) bool {
	limit := 16 * (math.Nextafter(1, 2) - 1) * float64(rows+1) * math.Max(1, absoluteSum+math.Abs(b))
	return epathOracleFinite(a) && epathOracleFinite(b) && math.Abs(a-b) <= limit
}

func epathSQLReadHourlyCompanionRows(db *sql.DB, p *epathSQLHourlyCompanionIdentity) error {
	weather, err := epathReadOracleWeather(db)
	if err != nil {
		return err
	}
	var hourly, absolute [12]float64
	for _, source := range []epathRealSQLSource{p.Source, p.Authority} {
		var dictionaryCount, dictionaryID int
		if err := db.QueryRow(`SELECT COUNT(*),COALESCE(MIN(ReportDataDictionaryIndex),0) FROM ReportDataDictionary WHERE Name=? COLLATE NOCASE AND KeyValue=? COLLATE NOCASE AND ReportingFrequency=? COLLATE NOCASE`, source.Name, source.KeyValue, source.ReportingFrequency).Scan(&dictionaryCount, &dictionaryID); err != nil {
			return err
		}
		if dictionaryCount != 1 || dictionaryID != source.DictionaryIndex {
			return fmt.Errorf("Hourly companion %d has duplicate/missing dictionary identity", source.DictionaryIndex)
		}
		rows, err := db.Query(`SELECT t.TimeIndex,t.EnvironmentPeriodIndex,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,r.Value,d.Name,d.KeyValue,d.Units,d.IsMeter,d.ReportingFrequency FROM ReportData r JOIN ReportDataDictionary d USING(ReportDataDictionaryIndex) JOIN "Time" t USING(TimeIndex) JOIN EnvironmentPeriods e USING(EnvironmentPeriodIndex) WHERE r.ReportDataDictionaryIndex=? AND e.EnvironmentType=3 AND `+epathOracleNonWarmupSQL+` ORDER BY t.TimeIndex,r.ReportDataIndex`, source.DictionaryIndex)
		if err != nil {
			return err
		}
		count, last, rawSum, rawAbsoluteTotal, energySum, absoluteTotal, wireSum := 0, int64(0), 0.0, 0.0, 0.0, 0.0, 0.0
		var monthSums [12]float64
		for rows.Next() {
			var index int64
			var environment, year, month, day, hour, minute, intervalType, meter int
			var interval, value sql.NullFloat64
			var name, key, unit, frequency string
			if err := rows.Scan(&index, &environment, &year, &month, &day, &hour, &minute, &interval, &intervalType, &value, &name, &key, &unit, &meter, &frequency); err != nil {
				rows.Close()
				return err
			}
			if index <= last || environment != weather.EnvironmentIndex || year != 2017 || !interval.Valid || !value.Valid || !epathOracleFinite(value.Float64) || meter != 0 || name != source.Name || !strings.EqualFold(key, source.KeyValue) || unit != source.SourceUnit || frequency != source.ReportingFrequency {
				rows.Close()
				return fmt.Errorf("Hourly companion %d has duplicate/NULL/foreign original row %d", source.DictionaryIndex, index)
			}
			last = index
			stamp := time.Date(2017, time.Month(count+2), 0, 0, 0, 0, 0, time.UTC)
			if frequency == "Hourly" {
				stamp = time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(count) * time.Hour)
				if count >= 8760 || intervalType != 1 || interval.Float64 != 60 || month != int(stamp.Month()) || day != stamp.Day() || hour != stamp.Hour()+1 || minute != 0 {
					rows.Close()
					return fmt.Errorf("Hourly companion %d has an incomplete/shifted original hourly calendar at %d", source.DictionaryIndex, index)
				}
			} else if count >= 12 || intervalType != 3 || interval.Float64 != float64(stamp.Day()*1440) || month != count+1 || day != stamp.Day() || hour != 24 || minute != 0 {
				rows.Close()
				return fmt.Errorf("Hourly companion %d Monthly authority lacks an exact native calendar month", source.DictionaryIndex)
			}
			energy := value.Float64 * (1.0 / 3600000)
			if unit == "W" {
				energy = value.Float64 * (interval.Float64 / 60) / 1000
			}
			if !epathOracleFinite(energy) {
				rows.Close()
				return fmt.Errorf("Hourly companion %d has nonfinite native energy", source.DictionaryIndex)
			}
			monthSums[month-1] += energy
			rawSum += value.Float64
			rawAbsoluteTotal += math.Abs(value.Float64)
			energySum += energy
			absoluteTotal += math.Abs(energy)
			if frequency == "Hourly" {
				hourly[month-1] += energy
				absolute[month-1] += math.Abs(energy)
				wire := math.Round(energy*1000) / 1000
				p.HourlyValues = append(p.HourlyValues, wire)
				wireSum += wire
				p.HourlyLabels = append(p.HourlyLabels, fmt.Sprintf("%02d-%02d %02d:00", month, day, hour))
			} else if !epathSQLHourlyCompanionNativeNear(hourly[month-1], energy, absolute[month-1], stamp.Day()*24) {
				rows.Close()
				return fmt.Errorf("Hourly companion %d M%d native sum %.17g differs from selected Monthly %d %.17g", p.Source.DictionaryIndex, month, hourly[month-1], source.DictionaryIndex, energy)
			}
			count++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		want := 12
		if source.ReportingFrequency == "Hourly" {
			want = 8760
			p.ReportedScalar = math.Round(wireSum*1000) / 1000
		}
		if count != want || source.Rows != count || source.MissingRows != 0 || source.RawSum == nil || source.EnergyKWh == nil || len(source.Months) != 12 || !epathSQLHourlyCompanionNativeNear(energySum, *source.EnergyKWh, absoluteTotal, count) || !epathSQLHourlyCompanionNativeNear(rawSum, *source.RawSum, rawAbsoluteTotal, count) {
			return fmt.Errorf("Hourly companion %d lacks complete original observations", source.DictionaryIndex)
		}
		for i, bucket := range source.Months {
			if bucket.Month != i+1 || bucket.EnergyKWh == nil || bucket.MissingRows != 0 || !epathSQLHourlyCompanionNativeNear(monthSums[i], *bucket.EnergyKWh, absolute[i], time.Date(2017, time.Month(i+2), 0, 0, 0, 0, 0, time.UTC).Day()*24) {
				return fmt.Errorf("Hourly companion %d original observation M%d disagrees with row audit", source.DictionaryIndex, i+1)
			}
		}
	}
	p.NativeAbsoluteSums = absolute
	return epathSQLValidateHourlyCompanion(*p)
}

func epathCompileSQLHourlyCompanions(sqlPath string, observed []epathRealSQLSource, model epathRealSQLModel, frames epathSQLFrames) (map[int]epathSQLHourlyCompanionIdentity, map[string][]int, error) {
	proofs, cells := map[int]epathSQLHourlyCompanionIdentity{}, map[string][]int{}
	if len(model.HourlyCompanions) == 0 {
		return proofs, cells, nil
	}
	db, err := epathOpenOracleSQL(sqlPath)
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	seen := map[string]bool{}
	for _, declaration := range model.HourlyCompanions {
		if declaration.Unit == "" || epathSQLHourlyCompanionUnit(declaration.Name) != declaration.Unit || len(declaration.Keys) == 0 {
			return nil, nil, fmt.Errorf("unreviewed/empty exact Hourly companion declaration %s", declaration.Name)
		}
		for _, key := range declaration.Keys {
			identity := declaration.Name + "|" + strings.ToLower(key)
			if strings.TrimSpace(key) == "" || key == "*" || seen[identity] {
				return nil, nil, fmt.Errorf("duplicate/wildcard Hourly companion key %s", identity)
			}
			seen[identity] = true
			var h, m epathRealSQLSource
			hc, mc := 0, 0
			for _, s := range observed {
				if s.Name != declaration.Name || !strings.EqualFold(s.KeyValue, key) {
					continue
				}
				if s.ReportingFrequency == "Hourly" {
					h = s
					hc++
				}
				if s.ReportingFrequency == "Monthly" {
					m = s
					mc++
				}
			}
			selected, ok := frames.SourceIdentities[m.DictionaryIndex]
			if hc != 1 || mc != 1 || !ok || !reflect.DeepEqual(selected, m) || h.IsMeter || m.IsMeter || h.SourceUnit != declaration.Unit || m.SourceUnit != declaration.Unit {
				return nil, nil, fmt.Errorf("Hourly companion %s lacks a unique actually selected Monthly authority", identity)
			}
			zoneKey := strings.ToLower(frames.SourceZone[m.DictionaryIndex])
			zone, ok := frames.Zones[zoneKey]
			if !ok || zone.Name == "" || zone.Multiplier <= 0 || !epathOracleFinite(zone.Multiplier) {
				return nil, nil, fmt.Errorf("Hourly companion %d has no independent Monthly Zone owner", h.DictionaryIndex)
			}
			query := `SELECT ZoneName,Multiplier,ListMultiplier FROM Zones WHERE ZoneName=? COLLATE NOCASE`
			if declaration.Name == "Surface Inside Face Convection Heat Gain Energy" {
				query = `SELECT z.ZoneName,z.Multiplier,z.ListMultiplier FROM Surfaces s JOIN Zones z USING(ZoneIndex) WHERE s.SurfaceName=? COLLATE NOCASE AND s.HeatTransferSurf=1`
			}
			ownerRows, err := db.Query(query, key)
			if err != nil {
				return nil, nil, err
			}
			ownerCount := 0
			for ownerRows.Next() {
				var name string
				var multiplier, listMultiplier float64
				if err := ownerRows.Scan(&name, &multiplier, &listMultiplier); err != nil {
					ownerRows.Close()
					return nil, nil, err
				}
				if !strings.EqualFold(name, zone.Name) || multiplier <= 0 || listMultiplier <= 0 || multiplier*listMultiplier != zone.Multiplier {
					ownerRows.Close()
					return nil, nil, fmt.Errorf("Hourly companion %d has a foreign native SQL Zone/surface owner or multiplier", h.DictionaryIndex)
				}
				ownerCount++
			}
			err = ownerRows.Err()
			ownerRows.Close()
			if err != nil {
				return nil, nil, err
			}
			if ownerCount != 1 {
				return nil, nil, fmt.Errorf("Hourly companion %d has duplicate/missing native SQL ownership", h.DictionaryIndex)
			}
			bound := []string{}
			for cellKey, cell := range frames.Cells {
				for _, id := range cell.SourceIDs {
					if id == m.DictionaryIndex {
						if !strings.EqualFold(cell.Zone, zone.Name) {
							return nil, nil, fmt.Errorf("Hourly companion %d crosses an original cell Zone", h.DictionaryIndex)
						}
						bound = append(bound, cellKey)
						break
					}
				}
			}
			if len(bound) == 0 {
				return nil, nil, fmt.Errorf("Hourly companion %d is not an input of any actual Monthly driver cell", h.DictionaryIndex)
			}
			proof := epathSQLHourlyCompanionIdentity{Source: h, Authority: m, ZoneName: zone.Name, Multiplier: zone.Multiplier}
			if err := epathSQLReadHourlyCompanionRows(db, &proof); err != nil {
				return nil, nil, err
			}
			proofs[h.DictionaryIndex] = proof
			for _, cellKey := range bound {
				cells[cellKey] = epathSQLDictionaryUnion(cells[cellKey], []int{h.DictionaryIndex})
			}
		}
	}
	return proofs, cells, nil
}

func epathSQLMatchHourlyCompanionSource(source EnergyDataSource, p epathSQLHourlyCompanionIdentity) error {
	if err := epathSQLValidateHourlyCompanion(p); err != nil {
		return err
	}
	method := "sum_report_data"
	if p.Source.SourceUnit == "W" {
		method = "integrate_rate_by_time_interval"
	}
	if source.ID != fmt.Sprintf("sql-rdd-%d", p.Source.DictionaryIndex) || source.SourceType != "sql_report_data" || source.IsMeter || source.Name != p.Source.Name || !strings.EqualFold(source.KeyValue, p.Source.KeyValue) || !strings.EqualFold(source.ZoneName, p.ZoneName) || source.ReportingFrequency != "Hourly" || source.Units != p.Source.SourceUnit || source.SourceUnit != p.Source.SourceUnit || source.NormalizedUnit != "kWh" || source.AggregationMethod != method || source.AggregationBasis != "model_total" || source.EffectiveMultiplier != p.Multiplier || source.MultiplierApplication != "requires_zone_multiplier" || source.Formula != "" || len(source.InputSourceIDs) != 0 {
		return fmt.Errorf("Hourly companion %d lost exact native identity/Zone/multiplier/provenance", p.Source.DictionaryIndex)
	}
	if !source.inspectorDecodedFromJSON || source.inspectorValuePresence&3 != 3 || source.RawValue != p.ReportedScalar || source.EffectiveValue != math.Round(p.ReportedScalar*p.Multiplier*1000)/1000 {
		return fmt.Errorf("Hourly companion %d changed signed row-quantized native transport or multiplied twice", p.Source.DictionaryIndex)
	}
	if source.HourlyEnergy == nil || source.HourlyEnergy.Unit != "kWh" || source.HourlyEnergy.Basis != "reported_source" || !reflect.DeepEqual(source.HourlyEnergy.Values, p.HourlyValues) {
		return fmt.Errorf("Hourly companion %d lost the original 8760-row chart", p.Source.DictionaryIndex)
	}
	return nil
}

func epathSQLBindHourlyCompanions(sqlPath string, observed []epathRealSQLSource, model epathRealSQLModel, frames *epathSQLFrames) error {
	if len(model.HourlyCompanions) == 0 {
		return nil
	}
	proofs, cells, err := epathCompileSQLHourlyCompanions(sqlPath, observed, model, *frames)
	if err != nil {
		return err
	}
	// Validate every collision before publishing any metadata. A failed opt-in
	// cannot leave a partially broadened source registry behind.
	for id := range proofs {
		_, original := frames.SourceIdentities[id]
		_, trace := frames.TraceSourceIdentities[id]
		_, raw := frames.SourceRaw[id]
		_, effective := frames.SourceEffective[id]
		if original || trace || raw || effective {
			return fmt.Errorf("Hourly companion %d overlaps an existing numeric/temporal authority", id)
		}
	}
	if frames.TraceSourceIdentities == nil {
		frames.TraceSourceIdentities = map[int]epathSQLTraceSourceIdentity{}
	}
	if frames.CellTraceSourceIDs == nil {
		frames.CellTraceSourceIDs = map[string][]int{}
	}
	for id, p := range proofs {
		copy := p
		frames.TraceSourceIdentities[id] = epathSQLTraceSourceIdentity{Source: p.Source, Authority: p.Authority, ZoneName: p.ZoneName, Multiplier: p.Multiplier, NativeCompanion: &copy}
		frames.SourceIdentities[id] = p.Source
	}
	for key, ids := range cells {
		frames.CellTraceSourceIDs[key] = epathSQLDictionaryUnion(frames.CellTraceSourceIDs[key], ids)
	}
	return nil
}
