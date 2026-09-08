package simulation

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// The alias has its own observations and precision ledger. Authority remains
// the original Monthly family term; neither alias values nor its rounding
// budget ever enter a numeric driver cell or allocation denominator.
type epathSQLTraceSourceIdentity struct {
	Family, ZoneName  string
	Source, Authority epathRealSQLSource
	Multiplier        float64
	NonzeroRows       [12]int
}

func epathSQLTraceEquivalentNames(family epathRealSQLFamily, trace epathRealSQLTraceSource) (epathRealSQLAlternative, error) {
	if trace.Frequency != "Hourly" || trace.Source.IsMeter || trace.Source.AllowAbsent || len(trace.Source.Alternatives) != 1 || len(family.Terms) != 1 || family.Terms[0].Sign != 1 || family.Terms[0].Source.IsMeter || len(family.Terms[0].Source.Alternatives) != 1 || family.Component != "sensible" || family.Role != "pressure" {
		return epathRealSQLAlternative{}, fmt.Errorf("temporal trace requires one explicit Hourly alias and one Monthly sensible authority")
	}
	alias, authority := trace.Source.Alternatives[0], family.Terms[0].Source.Alternatives[0]
	validSurface := family.ID == "surface.balance" && family.Category == "balance.storage_other" && alias.Name == "Zone Air Heat Balance Surface Convection Rate" && alias.Unit == "W" && authority == alias
	validInternal := family.ID == "internal.other.sensible" && family.Category == "internal.other" && alias.Name == "Zone Air Heat Balance Internal Convective Heat Gain Rate" && alias.Unit == "W" && authority.Name == "Zone Total Internal Convective Heating Energy" && authority.Unit == "J"
	if !validSurface && !validInternal {
		return epathRealSQLAlternative{}, fmt.Errorf("temporal trace is not one of the two independently reviewed physical equivalences")
	}
	return authority, nil
}

func epathSQLValidateTemporalTrace(proof epathSQLTraceSourceIdentity) error {
	name, category := "Zone Air Heat Balance Surface Convection Rate", "balance.storage_other"
	if proof.Family == "internal.other.sensible" {
		name, category = "Zone Air Heat Balance Internal Convective Heat Gain Rate", "internal.other"
	} else if proof.Family != "surface.balance" {
		return fmt.Errorf("unreviewed temporal trace family")
	}
	family := epathRealSQLFamily{ID: proof.Family, Category: category, Component: "sensible", Role: "pressure", Terms: []epathRealSQLTerm{{Sign: 1, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: proof.Authority.Name, Unit: proof.Authority.SourceUnit}}}}}}
	if _, err := epathSQLTraceEquivalentNames(family, epathRealSQLTraceSource{Frequency: proof.Source.ReportingFrequency, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: proof.Source.Name, Unit: proof.Source.SourceUnit}}, IsMeter: proof.Source.IsMeter}}); err != nil {
		return err
	}
	if proof.Source.Name != name || proof.Source.DictionaryIndex <= 0 || proof.Authority.DictionaryIndex <= 0 || proof.Source.DictionaryIndex == proof.Authority.DictionaryIndex || proof.Authority.IsMeter || proof.Authority.ReportingFrequency != "Monthly" || proof.ZoneName == "" || !strings.EqualFold(proof.Source.KeyValue, proof.ZoneName) || !strings.EqualFold(proof.Authority.KeyValue, proof.ZoneName) || !epathOracleFinite(proof.Multiplier) || proof.Multiplier <= 0 || len(proof.Source.Months) != 12 || len(proof.Authority.Months) != 12 || proof.Source.Rows != 8760 || proof.Source.MissingRows != 0 {
		return fmt.Errorf("temporal trace lacks exact owner/frequency/original identity")
	}
	for i, bucket := range proof.Source.Months {
		authority := proof.Authority.Months[i]
		hours := time.Date(2017, time.Month(i+2), 0, 0, 0, 0, 0, time.UTC).Day() * 24
		if bucket.Month != i+1 || bucket.Rows != hours || bucket.MissingRows != 0 || bucket.EnergyKWh == nil || !epathOracleFinite(*bucket.EnergyKWh) || authority.Month != i+1 || authority.Rows != 1 || authority.MissingRows != 0 || authority.EnergyKWh == nil || !epathSQLFanPoolNear(*bucket.EnergyKWh, *authority.EnergyKWh) || proof.NonzeroRows[i] < 0 || proof.NonzeroRows[i] > hours || proof.NonzeroRows[i] == 0 && *bucket.EnergyKWh != 0 {
			return fmt.Errorf("temporal trace M%d differs from its complete Monthly physical authority", i+1)
		}
	}
	return nil
}

func epathSQLAuditTemporalTrace(db *sql.DB, proof *epathSQLTraceSourceIdentity) error {
	// The observation reader intentionally omits dictionaries with no rows.
	// Such a dictionary still makes an exact physical identity ambiguous.
	for _, source := range []epathRealSQLSource{proof.Source, proof.Authority} {
		identities, err := db.Query(`SELECT ReportDataDictionaryIndex,Name,KeyValue,IsMeter,ReportingFrequency,Units FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=? OR Name=? COLLATE NOCASE`, source.DictionaryIndex, source.Name)
		if err != nil {
			return err
		}
		count := 0
		for identities.Next() {
			var id, meter int
			var name, key, frequency, unit string
			if err := identities.Scan(&id, &name, &key, &meter, &frequency, &unit); err != nil {
				identities.Close()
				return err
			}
			exact := strings.EqualFold(name, source.Name) && strings.EqualFold(key, source.KeyValue) && meter == 0 && strings.EqualFold(frequency, source.ReportingFrequency) && strings.EqualFold(unit, source.SourceUnit)
			if id != source.DictionaryIndex && !exact {
				continue
			}
			count++
			if id != source.DictionaryIndex || !exact {
				identities.Close()
				return fmt.Errorf("temporal source dictionary identity is duplicate or inconsistent")
			}
		}
		err = identities.Err()
		identities.Close()
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("temporal source requires exactly one original dictionary identity")
		}
	}
	weather, err := epathReadOracleWeather(db)
	if err != nil {
		return err
	}
	rows, err := db.Query(`SELECT t.TimeIndex,t.EnvironmentPeriodIndex,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,r.Value
FROM ReportData r JOIN "Time" t ON t.TimeIndex=r.TimeIndex JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex
WHERE r.ReportDataDictionaryIndex=? AND e.EnvironmentType=3 AND `+epathOracleNonWarmupSQL+` ORDER BY t.TimeIndex`, proof.Source.DictionaryIndex)
	if err != nil {
		return err
	}
	defer rows.Close()
	proof.NonzeroRows = [12]int{}
	seen, sums, count := [8760]bool{}, [12]float64{}, 0
	for rows.Next() {
		var index, environment, year, month, day, hour, minute, intervalType int
		var interval, value sql.NullFloat64
		if err := rows.Scan(&index, &environment, &year, &month, &day, &hour, &minute, &interval, &intervalType, &value); err != nil {
			return err
		}
		date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		if environment != weather.EnvironmentIndex || year != 2017 || month < 1 || month > 12 || day < 1 || date.Month() != time.Month(month) || date.Day() != day || hour < 1 || hour > 24 || minute != 0 || intervalType != 1 || !interval.Valid || interval.Float64 != 60 || !value.Valid || !epathOracleFinite(value.Float64) {
			return fmt.Errorf("temporal trace lacks a finite exact original 2017 hourly interval at %d", index)
		}
		slot := (date.YearDay()-1)*24 + hour - 1
		if seen[slot] {
			return fmt.Errorf("duplicate temporal trace hourly calendar slot")
		}
		seen[slot], count = true, count+1
		sums[month-1] += value.Float64 / 1000 // W * exactly one observed hour.
		if value.Float64 != 0 {
			proof.NonzeroRows[month-1]++
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != 8760 {
		return fmt.Errorf("temporal trace has %d observed hours, require8760", count)
	}
	for i, sum := range sums {
		if len(proof.Source.Months) != 12 || proof.Source.Months[i].EnergyKWh == nil || !epathSQLFanPoolNear(sum, *proof.Source.Months[i].EnergyKWh) {
			return fmt.Errorf("temporal trace original source/month sum mismatch")
		}
	}
	return epathSQLValidateTemporalTrace(*proof)
}

func epathCompileSQLTemporalTraceSources(sqlPath string, observed []epathRealSQLSource, model epathRealSQLModel, frames *epathSQLFrames) error {
	declared := false
	for _, family := range model.Families {
		declared = declared || len(family.TraceSources) > 0
	}
	if !declared {
		return nil
	}
	if frames == nil {
		return fmt.Errorf("temporal traces require independently completed frames")
	}
	db, err := epathOpenOracleSQL(sqlPath)
	if err != nil {
		return err
	}
	defer db.Close()
	proofs, cells := map[int]epathSQLTraceSourceIdentity{}, map[string][]int{}
	for _, family := range model.Families {
		for _, trace := range family.TraceSources {
			authorityName, err := epathSQLTraceEquivalentNames(family, trace)
			if err != nil {
				return err
			}
			if len(trace.Source.Keys) == 0 {
				return fmt.Errorf("temporal trace requires an explicit reviewed Zone roster")
			}
			familyZones, seen := map[string]bool{}, map[string]bool{}
			for _, zone := range family.Keys {
				familyZones[strings.ToLower(zone)] = true
			}
			for _, zone := range trace.Source.Keys {
				key := strings.ToLower(zone)
				if zone == "" || zone == "*" || seen[key] || !familyZones[key] || frames.Zones[key].Name == "" {
					return fmt.Errorf("unknown/duplicate temporal trace owner")
				}
				seen[key] = true
				var aliases, authorities []epathRealSQLSource
				for _, s := range observed {
					if !strings.EqualFold(s.KeyValue, zone) || s.IsMeter {
						continue
					}
					if s.Name == trace.Source.Alternatives[0].Name && s.ReportingFrequency == "Hourly" && s.SourceUnit == "W" {
						aliases = append(aliases, s)
					}
					if s.Name == authorityName.Name && s.ReportingFrequency == "Monthly" && s.SourceUnit == authorityName.Unit {
						authorities = append(authorities, s)
					}
				}
				if len(aliases) != 1 || len(authorities) != 1 {
					return fmt.Errorf("temporal trace requires exactly one Hourly and one Monthly physical source")
				}
				proof := epathSQLTraceSourceIdentity{Family: family.ID, ZoneName: frames.Zones[key].Name, Source: aliases[0], Authority: authorities[0], Multiplier: frames.Zones[key].Multiplier}
				if _, ok := proofs[proof.Source.DictionaryIndex]; ok {
					return fmt.Errorf("duplicate temporal trace declaration")
				}
				if _, ok := frames.SourceIdentities[proof.Source.DictionaryIndex]; ok {
					return fmt.Errorf("temporal trace overlaps canonical numeric authority")
				}
				if err := epathSQLAuditTemporalTrace(db, &proof); err != nil {
					return err
				}
				for month := 1; month <= 12; month++ {
					cellKey := epathSQLKey(key, family.ID, month)
					cell := frames.Cells[cellKey]
					if cell == nil {
						return fmt.Errorf("temporal trace lacks independently completed family cell")
					}
					found := false
					for _, id := range cell.SourceIDs {
						found = found || id == proof.Authority.DictionaryIndex
					}
					if !found {
						return fmt.Errorf("trace Monthly counterpart is not the selected family authority")
					}
					cells[cellKey] = epathSQLDictionaryUnion(cells[cellKey], []int{proof.Source.DictionaryIndex})
				}
				proofs[proof.Source.DictionaryIndex] = proof
			}
		}
	}
	if frames.TraceSourceIdentities == nil {
		frames.TraceSourceIdentities = map[int]epathSQLTraceSourceIdentity{}
	}
	if frames.CellTraceSourceIDs == nil {
		frames.CellTraceSourceIDs = map[string][]int{}
	}
	for id, p := range proofs {
		frames.TraceSourceIdentities[id] = p
		frames.SourceIdentities[id] = p.Source
		frames.SourceZone[id] = strings.ToLower(p.ZoneName)
		for i, bucket := range p.Source.Months {
			q := epathSQLQuantity{Value: *bucket.EnergyKWh, Error: float64(p.NonzeroRows[i]) * .0005}
			frames.SourceRaw[id] = append(frames.SourceRaw[id], q)
			frames.SourceEffective[id] = append(frames.SourceEffective[id], q.times(p.Multiplier))
		}
	}
	for key, ids := range cells {
		frames.CellTraceSourceIDs[key] = epathSQLDictionaryUnion(frames.CellTraceSourceIDs[key], ids)
	}
	return nil
}

func epathSQLTemporalTraceAnnualQuantity(proof epathSQLTraceSourceIdentity, field string) (epathSQLQuantity, error) {
	if err := epathSQLValidateTemporalTrace(proof); err != nil {
		return epathSQLQuantity{}, err
	}
	q, nonzero := epathSQLQuantity{}, 0
	for i, bucket := range proof.Source.Months {
		q = q.add(epathSQLQuantity{Value: *bucket.EnergyKWh, Error: float64(proof.NonzeroRows[i]) * .0005})
		nonzero += proof.NonzeroRows[i]
	}
	// Count only actual presentation stages: per-Hourly-point rounding above,
	// one original annual total round, then one effective-multiplier round.
	if nonzero > 0 {
		low, high := q.bounds()
		q = epathSQLBounded(q.Value, low-.0005, high+.0005)
	}
	if field == "effectiveValue" {
		q = q.times(proof.Multiplier)
		if nonzero > 0 {
			low, high := q.bounds()
			q = epathSQLBounded(q.Value, low-.0005, high+.0005)
		}
	} else if field != "rawValue" {
		return q, fmt.Errorf("unsupported original temporal trace field")
	}
	return q, nil
}

func epathSQLTemporalTraceSourceMatches(source EnergyDataSource, proof epathSQLTraceSourceIdentity) bool {
	if epathSQLValidateTemporalTrace(proof) != nil || !epathSQLOriginalSourceMatches(source, epathSQLOriginalRDD(proof.Source), "annual") {
		return false
	}
	category, component, direction := "balance.storage_other", "surface.reconciliation", "signed"
	if proof.Family == "internal.other.sensible" {
		category, component, direction = "internal.other", "internal.reconciliation.convective", "gain"
	}
	return source.ID == fmt.Sprintf("sql-rdd-%d", proof.Source.DictionaryIndex) && strings.EqualFold(source.ZoneName, proof.ZoneName) && source.Units == "W" && source.DriverRole == "reconciliation" && source.DriverCategory == category && source.DriverComponent == component && source.HeatDirection == direction && source.InspectorSection == "Context" && source.AggregationMethod == "integrate_rate_by_time_interval" && source.AggregationBasis == "model_total" && source.MultiplierApplication == "requires_zone_multiplier" && source.EffectiveMultiplier == proof.Multiplier && source.Formula == "" && len(source.InputSourceIDs) == 0
}

func epathCheckSQLTemporalTraceSource(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	p := check.TraceSource
	t := check.Item.Target
	if p == nil || check.Item.Period != "annual" || check.Item.Group != "drivers" || (check.Item.Scope != "building" && check.Item.Scope != "zone") || check.Item.Scope == "building" && check.Item.Zone != "" || check.Item.Scope == "zone" && !strings.EqualFold(check.Item.Zone, p.ZoneName) || check.Item.Unit != "kWh" || t.Collection != "sources" || t.SourceName != p.Source.Name || t.SourceKey != p.Source.KeyValue || t.SourceUnit != "W" || t.Frequency != "Hourly" || t.Unit != "kWh" {
		return fmt.Errorf("temporal source check contradicts its original identity")
	}
	q, err := epathSQLTemporalTraceAnnualQuantity(*p, t.Field)
	if err != nil {
		return err
	}
	if check.Quantity == nil || !epathSQLZoneCarrierQuantityEqual(q, *check.Quantity) {
		return fmt.Errorf("temporal source check changed independent precision ledger")
	}
	count := 0
	for _, source := range bundle.EnergyExplanation.Sources {
		if source.ID == fmt.Sprintf("sql-rdd-%d", p.Source.DictionaryIndex) || source.Name == p.Source.Name && source.KeyValue == p.Source.KeyValue && source.ReportingFrequency == "Hourly" {
			count++
			if !epathSQLTemporalTraceSourceMatches(source, *p) {
				return fmt.Errorf("temporal source lost exact original metadata")
			}
		}
	}
	if count != 1 {
		return fmt.Errorf("temporal source requires exactly one original observation")
	}
	return nil
}
