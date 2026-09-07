package simulation

// Fan expectations come from reviewed model membership and independent SQL
// observations, never from the production fan parser, allocator or candidate.
import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type epathRealSQLFanPool struct {
	SiteID      string   `json:"siteId"`
	Name        string   `json:"name"`
	Key         string   `json:"key"`
	Frequency   string   `json:"frequency"`
	Unit        string   `json:"unit"`
	ServedZones []string `json:"servedZones"`
}

type epathSQLFanPoolObservation struct {
	Declaration epathRealSQLFanPool
	Source      epathRealSQLSource
	Months      [12]float64
}

func epathSQLFanPoolNear(left, right float64) bool {
	// Floating summation only; this is not a presentation/rounding allowance.
	return epathOracleFinite(left) && epathOracleFinite(right) && math.Abs(left-right) <= 1e-10*math.Max(1, math.Max(math.Abs(left), math.Abs(right)))
}

func epathSQLFanPoolObservations(sources []epathRealSQLSource, pools []epathRealSQLFanPool) ([]epathSQLFanPoolObservation, error) {
	out := []epathSQLFanPoolObservation{}
	seen, zones := map[string]bool{}, map[string]bool{}
	for _, pool := range pools {
		if strings.TrimSpace(pool.SiteID) == "" || pool.Name != "Air System Fan Electricity Energy" || pool.Key == "" || strings.TrimSpace(pool.Key) != pool.Key || pool.Key == "*" || pool.Frequency != "Hourly" || pool.Unit != "J" || len(pool.ServedZones) == 0 {
			return nil, fmt.Errorf("fan pool requires exact reviewed Hourly/J energy identity and served Zones")
		}
		identity := strings.ToLower(pool.Name + "\x00" + pool.Key + "\x00" + pool.Frequency + "\x00" + pool.Unit)
		if seen[identity] {
			return nil, fmt.Errorf("duplicate fan pool identity %s/%s", pool.Name, pool.Key)
		}
		seen[identity] = true
		for _, zone := range pool.ServedZones {
			key := strings.ToLower(pool.SiteID + "\x00" + zone)
			if zone == "" || strings.TrimSpace(zone) != zone || zones[key] {
				return nil, fmt.Errorf("duplicate/overlapping/empty fan pool Zone %q; overlap requires separate reviewed proof", zone)
			}
			zones[key] = true
		}
		matches := []epathRealSQLSource{}
		for _, source := range sources {
			if strings.EqualFold(source.Name, pool.Name) && strings.EqualFold(source.KeyValue, pool.Key) && source.ReportingFrequency == pool.Frequency && source.SourceUnit == pool.Unit && !source.IsMeter {
				matches = append(matches, source)
			}
		}
		if len(matches) != 1 {
			return nil, fmt.Errorf("fan pool %s has %d exact SQL identities, require one", pool.Key, len(matches))
		}
		source := matches[0]
		if source.DictionaryIndex <= 0 || source.Rows != 8760 || source.MissingRows != 0 || source.RawSum == nil || source.EnergyKWh == nil || len(source.Months) != 12 {
			return nil, fmt.Errorf("fan pool %s lacks fully observed 8760-hour annual source", pool.Key)
		}
		item := epathSQLFanPoolObservation{Declaration: pool, Source: source}
		months := map[int]bool{}
		for _, month := range source.Months {
			if month.Month < 1 || month.Month > 12 || months[month.Month] || month.MissingRows != 0 || month.RawSum == nil || month.EnergyKWh == nil {
				return nil, fmt.Errorf("fan pool %s has duplicate/missing/unknown month", pool.Key)
			}
			months[month.Month] = true
			hours := time.Date(2017, time.Month(month.Month+1), 0, 0, 0, 0, 0, time.UTC).Day() * 24
			value := *month.RawSum / 3600000
			if month.Rows != hours || value < 0 || !epathSQLFanPoolNear(value, *month.EnergyKWh) {
				return nil, fmt.Errorf("fan pool %s M%d has incomplete/invalid Hourly J observations", pool.Key, month.Month)
			}
			item.Months[month.Month-1] = value
		}
		annual := 0.0
		for _, value := range item.Months {
			annual += value
		}
		if !epathSQLFanPoolNear(annual, *source.EnergyKWh) || !epathSQLFanPoolNear(annual, *source.RawSum/3600000) {
			return nil, fmt.Errorf("fan pool %s annual source does not equal its twelve observed months", pool.Key)
		}
		out = append(out, item)
	}
	return out, nil
}

func epathSQLFanPoolAuditHours(path string, observations []epathSQLFanPoolObservation) error {
	db, err := epathOpenOracleSQL(path)
	if err != nil {
		return err
	}
	defer db.Close()
	weather, err := epathReadOracleWeather(db)
	if err != nil {
		return err
	}
	for _, item := range observations {
		var name, key, frequency, unit string
		var isMeter int
		if err := db.QueryRow(`SELECT Name,KeyValue,ReportingFrequency,Units,IsMeter FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=?`, item.Source.DictionaryIndex).Scan(&name, &key, &frequency, &unit, &isMeter); err != nil {
			return err
		}
		if !strings.EqualFold(name, item.Declaration.Name) || !strings.EqualFold(key, item.Declaration.Key) || frequency != "Hourly" || unit != "J" || isMeter != 0 {
			return fmt.Errorf("fan pool dictionary binding changed")
		}
		rows, err := db.Query(`SELECT t.TimeIndex,t.EnvironmentPeriodIndex,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,r.Value
FROM ReportData r JOIN "Time" t ON t.TimeIndex=r.TimeIndex
JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex
WHERE r.ReportDataDictionaryIndex=? AND e.EnvironmentType=3 AND `+epathOracleNonWarmupSQL+` ORDER BY t.TimeIndex`, item.Source.DictionaryIndex)
		if err != nil {
			return err
		}
		err = func() error {
			defer rows.Close()
			seen := [8760]bool{}
			sums, count := [12]float64{}, 0
			for rows.Next() {
				var timeIndex, environment, year, month, day, hour, minute, intervalType int
				var interval, value sql.NullFloat64
				if err := rows.Scan(&timeIndex, &environment, &year, &month, &day, &hour, &minute, &interval, &intervalType, &value); err != nil {
					return err
				}
				date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
				if environment != weather.EnvironmentIndex || year != 2017 || month < 1 || month > 12 || day < 1 || date.Month() != time.Month(month) || date.Day() != day || hour < 1 || hour > 24 || minute != 0 || intervalType != 1 || !interval.Valid || interval.Float64 != 60 || !value.Valid || !epathOracleFinite(value.Float64) || value.Float64 < 0 {
					return fmt.Errorf("fan pool %s TimeIndex %d lacks an exact finite nonnegative 2017 hourly observation", key, timeIndex)
				}
				slot := (date.YearDay()-1)*24 + hour - 1
				if seen[slot] {
					return fmt.Errorf("fan pool %s has duplicate hourly calendar slot at TimeIndex %d", key, timeIndex)
				}
				seen[slot], count = true, count+1
				sums[month-1] += value.Float64 / 3600000
			}
			if err := rows.Err(); err != nil {
				return err
			}
			if count != len(seen) {
				return fmt.Errorf("fan pool %s has %d unique hours, require 8760", key, count)
			}
			for month, sum := range sums {
				if !epathSQLFanPoolNear(sum, item.Months[month]) {
					return fmt.Errorf("fan pool %s M%d raw Hourly sum differs from independent source observation", key, month+1)
				}
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

func epathSQLModelFanPoolChecks(observed epathRealOracleEvidence, frames epathSQLFrames, pools []epathRealSQLFanPool, precision epathRealSQLPrecision, checks *epathSQLModelChecks) error {
	if len(pools) == 0 {
		return nil
	}
	if precision.DecimalPlaces != 3 || precision.SourceStages < 1 || precision.SourceStages > 3 || precision.ContributionStages < 1 || precision.ContributionStages > 3 {
		return fmt.Errorf("fan pool requires reviewed monthly precision budget")
	}
	items, err := epathSQLFanPoolObservations(observed.Sources, pools)
	if err != nil {
		return err
	}
	if err := epathSQLFanPoolAuditHours(observed.sqlPath, items); err != nil {
		return err
	}
	// The complete pool partition must independently close the broad meter;
	// neither a partial pool sum nor a zero sum licenses global rescaling.
	bySite := map[string][12]float64{}
	for _, item := range items {
		sums := bySite[item.Declaration.SiteID]
		for month, value := range item.Months {
			sums[month] += value
		}
		bySite[item.Declaration.SiteID] = sums
	}
	meters, err := epathSQLSelect(observed.Sources, epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Fans:Electricity", Unit: "J"}}, Keys: []string{"*"}, IsMeter: true})
	if err != nil || len(meters) != 1 || meters[0].Rows != 12 || meters[0].MissingRows != 0 || len(meters[0].Months) != 12 || len(bySite) != 1 {
		return fmt.Errorf("fan pool partition requires one exact reported Monthly/J Fans:Electricity meter")
	}
	broadMonths := [12]*float64{}
	for _, month := range meters[0].Months {
		if month.Month < 1 || month.Month > 12 || broadMonths[month.Month-1] != nil || month.Rows != 1 || month.MissingRows != 0 || month.RawSum == nil || month.EnergyKWh == nil || !epathSQLFanPoolNear(*month.RawSum/3600000, *month.EnergyKWh) {
			return fmt.Errorf("fan broad meter has duplicate/missing/unknown month")
		}
		broadMonths[month.Month-1] = month.EnergyKWh
	}
	for id, sums := range bySite {
		meter, ok := frames.Site[id]
		if !ok || len(meter) != 12 {
			return fmt.Errorf("fan pool lacks exact broad meter %s", id)
		}
		for month, sum := range sums {
			if meter[month] == nil || !meter[month].valid() || meter[month].Value < 0 || sum <= 0 || broadMonths[month] == nil || !epathSQLFanPoolNear(sum, *broadMonths[month]) || !epathSQLFanPoolNear(sum, meter[month].Value) {
				return fmt.Errorf("fan pools do not positively close broad meter %s M%d", id, month+1)
			}
		}
	}
	zoneValues := map[string][12]epathSQLQuantity{}
	for _, item := range items {
		served, err := epathSQLDeclaredZones(frames, item.Declaration.ServedZones)
		if err != nil {
			return err
		}
		for month, value := range item.Months {
			weights, denominator := map[string]epathSQLQuantity{}, epathSQLQuantity{}
			for zone := range served {
				weight := epathSQLQuantity{}
				for _, service := range []string{"cooling", "heating"} {
					load, ok := frames.Loads[epathSQLKey(zone, service, month+1)]
					if !ok || !load.valid() || load.Value < 0 {
						return fmt.Errorf("fan pool %s lacks known monthly %s load for %s", item.Declaration.Key, service, zone)
					}
					weight = weight.add(load)
				}
				weights[zone] = weight
				denominator = denominator.add(weight)
			}
			pool := epathSQLQuantity{Value: value, Error: float64(precision.SourceStages) * .0005}
			if value == 0 {
				pool = epathSQLQuantity{}
			}
			if value > 0 && denominator.Value <= 0 {
				return fmt.Errorf("positive fan pool %s M%d has zero served load: explicit unassigned-pool proof is required", item.Declaration.Key, month+1)
			}
			for zone, weight := range weights {
				share, err := epathSQLShare(pool, weight, denominator, precision)
				if err != nil {
					return err
				}
				values := zoneValues[zone]
				values[month] = values[month].add(share)
				zoneValues[zone] = values
			}
		}
		annual := epathSQLQuantity{Error: 12 * float64(precision.SourceStages) * .0005}
		for _, value := range item.Months {
			annual.Value += value
		}
		for _, field := range []string{"rawValue", "effectiveValue"} {
			target := epathRealOracleTarget{Collection: "sources", Field: field, SourceName: item.Source.Name, SourceKey: item.Source.KeyValue, SourceUnit: "J", Frequency: "Hourly", Unit: "kWh"}
			if err := checks.add("endUses", "building", "", "annual", "fan_pool/"+item.Declaration.Key+"/"+field, "kWh", &annual, target, "", nil, nil); err != nil {
				return err
			}
		}
	}
	zones := []string{}
	for zone := range frames.Zones {
		zones = append(zones, zone)
	}
	sort.Strings(zones)
	for _, zone := range zones {
		for _, period := range append([]string{"annual"}, "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12") {
			value := epathSQLQuantity{}
			for _, month := range epathSQLPeriodMonths(period) {
				value = value.add(zoneValues[zone][month-1])
			}
			target := epathSQLNodeTarget("end_use", "fans", "", "site")
			// End-use aggregates are carrier-agnostic in the canonical schema;
			// electricity is proven by the exact fan observations above, not a
			// redundant Carrier field on this node (carrier endpoints own it).
			target.Basis, target.AllowPrunedZero = "service_path_allocation", true
			if err := checks.add("zoneAllocation", "zone", frames.Zones[zone].Name, period, "fan_pools/value", "kWh", &value, target, "", nil, nil); err != nil {
				return err
			}
		}
	}
	return nil
}
