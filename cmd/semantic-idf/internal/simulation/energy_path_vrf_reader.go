package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// These are private allocation observations, never additive ordinary energy
// series. Values retain the original SQL precision and modeled total basis.
type energyPathVRFConsumptionObservation struct {
	Target        energyPathVRFConsumptionTarget
	Source        EnergyDataSource
	Monthly       map[int]float64
	InvalidMonths map[int]bool
	Invalid       bool
	Requested     bool // This exact target, independent of missing sibling requests.
}

type energyPathVRFConsumptionCohort struct {
	System       energyPathVRFSystem
	Observations []energyPathVRFConsumptionObservation
	Months       map[int]bool
	Requested    bool
}

type energyPathVRFObservationPosition struct{ cohort, observation int }

type energyPathVRFConsumptionCollector struct {
	cohorts       []energyPathVRFConsumptionCohort
	plan          *PurposeRunPlan
	byDictionary  map[int][]energyPathVRFObservationPosition
	byTime        map[int64]int
	excludedTimes map[int64]bool
	axisInvalid   bool
	counts        map[energyPathVRFObservationPosition]map[int]int
}

func newEnergyPathVRFConsumptionCollector(systems []energyPathVRFSystem, plan *PurposeRunPlan) *energyPathVRFConsumptionCollector {
	if len(systems) == 0 {
		return nil
	}
	collector := &energyPathVRFConsumptionCollector{plan: plan, byDictionary: map[int][]energyPathVRFObservationPosition{},
		byTime: map[int64]int{}, excludedTimes: map[int64]bool{}, counts: map[energyPathVRFObservationPosition]map[int]int{}}
	for _, system := range systems {
		cohort := energyPathVRFConsumptionCohort{System: system, Months: map[int]bool{}, Requested: len(system.Targets) > 0}
		for _, target := range system.Targets {
			cohort.Requested = cohort.Requested && energyPathVRFConsumptionRequested(plan, target)
		}
		collector.cohorts = append(collector.cohorts, cohort)
	}
	return collector
}

func energyPathVRFConsumptionRequested(plan *PurposeRunPlan, target energyPathVRFConsumptionTarget) bool {
	_, requested := energyPathVRFConsumptionRequestIndex(plan, target)
	return requested
}

// Source.ObjectIndex identifies the exact Output request, not its equipment.
// Generated requests can have no original index; Target keeps physical identity.
func energyPathVRFConsumptionRequestIndex(plan *PurposeRunPlan, target energyPathVRFConsumptionTarget) (*int, bool) {
	if !energyExplanationPlanUsesEnergyPath(plan) {
		return nil, false
	}
	count, valid := 0, true
	var objectIndex *int
	for _, output := range plan.OutputObjects {
		if !strings.EqualFold(strings.TrimSpace(output.ObjectType), "Output:Variable") ||
			!strings.EqualFold(strings.TrimSpace(output.ReportingFrequency), "Monthly") ||
			!strings.EqualFold(strings.TrimSpace(output.KeyValue), strings.TrimSpace(target.KeyValue)) {
			continue
		}
		definition, recognized := energyPathVRFConsumptionDefinitionForName(output.VariableName)
		if !recognized || definition.ID != target.Definition.ID {
			continue
		}
		count++
		valid = valid && purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) &&
			strings.EqualFold(strings.TrimSpace(output.ScopeZoneName), strings.TrimSpace(target.ZoneName))
		if output.ObjectIndex != nil {
			index := *output.ObjectIndex
			objectIndex = &index
		}
	}
	if !valid || count != 1 {
		return nil, false
	}
	return objectIndex, true
}

// This metadata-only axis is deliberately not a fixed annual calendar. Three
// observed weather months are three required months, not nine invented zeros.
// Monthly energy records may describe a partial ending month, so the valid day
// need not be the calendar's last day. Impossible dates/intervals, repeated
// months/years, multiple weather runs and duplicate Time IDs are ambiguous.
func (collector *energyPathVRFConsumptionCollector) readAxis(db *sql.DB) {
	rows, err := db.Query(`SELECT t.TimeIndex,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",
		t.EnvironmentPeriodIndex,t.WarmupFlag,e.EnvironmentType
		FROM "Time" t LEFT JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex
		WHERE t.IntervalType=3 ORDER BY t.TimeIndex`)
	if err != nil {
		collector.axisInvalid = true
		return
	}
	defer rows.Close()
	seenIDs, months := map[int64]bool{}, map[int]bool{}
	var observedYear, environment int64
	for rows.Next() {
		var id, year, month, day, hour, minute, env, warmup, envType sql.NullInt64
		var interval sql.NullFloat64
		if err := rows.Scan(&id, &year, &month, &day, &hour, &minute, &interval, &env, &warmup, &envType); err != nil {
			collector.axisInvalid = true
			continue
		}
		if !id.Valid || id.Int64 <= 0 || seenIDs[id.Int64] {
			collector.axisInvalid = true
			continue
		}
		seenIDs[id.Int64] = true
		if envType.Valid && envType.Int64 != 3 || warmup.Valid && warmup.Int64 != 0 {
			collector.excludedTimes[id.Int64] = true
			continue
		}
		if !envType.Valid || envType.Int64 != 3 || !env.Valid || env.Int64 <= 0 || !year.Valid || year.Int64 < 1 || year.Int64 > 9999 ||
			!month.Valid || month.Int64 < 1 || month.Int64 > 12 || !day.Valid || !hour.Valid || hour.Int64 != 24 || !minute.Valid || minute.Int64 != 0 ||
			!interval.Valid || !energyPathFinite(interval.Float64) || interval.Float64 <= 0 {
			collector.axisInvalid = true
			continue
		}
		lastDay := time.Date(int(year.Int64), time.Month(month.Int64)+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if day.Int64 < 1 || day.Int64 > int64(lastDay) || interval.Float64 > float64(lastDay*1440) || months[int(month.Int64)] ||
			observedYear != 0 && observedYear != year.Int64 || environment != 0 && environment != env.Int64 {
			collector.axisInvalid = true
			continue
		}
		observedYear, environment = year.Int64, env.Int64
		months[int(month.Int64)] = true
		collector.byTime[id.Int64] = int(month.Int64)
	}
	if rows.Err() != nil || len(collector.byTime) == 0 {
		collector.axisInvalid = true
	}
	for i := range collector.cohorts {
		for month := range months {
			collector.cohorts[i].Months[month] = true
		}
	}
}

// Read dictionary metadata, including identities with no ReportData rows. The
// values themselves are consumed only by the existing canonical SQL walker.
func (collector *energyPathVRFConsumptionCollector) dictionaries(db *sql.DB, sourceFile string) []energyExplanationDictionary {
	if collector == nil {
		return nil
	}
	collector.readAxis(db)
	columns, err := sqlTableColumns(db, "ReportDataDictionary")
	if err != nil || !sqlHasColumns(columns, "ReportDataDictionaryIndex", "KeyValue", "Name", "Units", "IsMeter", "ReportingFrequency") {
		return nil
	}
	rows, err := db.Query(fmt.Sprintf(`SELECT ReportDataDictionaryIndex,%s,%s,%s,%s,%s,%s
		FROM ReportDataDictionary ORDER BY ReportDataDictionaryIndex`,
		sqlTextColumnExpr(columns, "KeyValue", "''"), sqlTextColumnExpr(columns, "Name", "''"), sqlTextColumnExpr(columns, "Units", "''"),
		sqlCastTextColumnExpr(columns, "IsMeter", "''"), sqlTextColumnExpr(columns, "ReportingFrequency", "''"), sqlTextColumnExpr(columns, "IndexGroup", "''")))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []energyExplanationDictionary
	idCounts, identityCounts := map[int]int{}, map[string]int{}
	for rows.Next() {
		var dictionary energyExplanationDictionary
		var meter string
		if err := rows.Scan(&dictionary.row.index, &dictionary.row.keyValue, &dictionary.row.name, &dictionary.row.units, &meter, &dictionary.reportingFrequency, &dictionary.indexGroup); err != nil {
			collector.axisInvalid = true
			continue
		}
		idCounts[dictionary.row.index]++
		definition, recognized := energyPathVRFConsumptionDefinitionForName(dictionary.row.name)
		if !recognized || !strings.EqualFold(strings.TrimSpace(dictionary.reportingFrequency), "Monthly") {
			continue
		}
		dictionary.isMeter = parseSQLBool(meter)
		if dictionary.row.index <= 0 || dictionary.isMeter || strings.TrimSpace(meter) != "0" || !strings.EqualFold(strings.TrimSpace(dictionary.row.units), "J") {
			continue
		}
		identity := definition.ID + "\x00" + energyPathVRFName(dictionary.row.keyValue)
		identityCounts[identity]++
		cohortIndex, targetIndex, count := -1, -1, 0
		for i, cohort := range collector.cohorts {
			for j, target := range cohort.System.Targets {
				if target.Definition.ID == definition.ID && strings.EqualFold(strings.TrimSpace(target.KeyValue), strings.TrimSpace(dictionary.row.keyValue)) {
					cohortIndex, targetIndex, count = i, j, count+1
				}
			}
		}
		if count != 1 {
			continue
		}
		target := collector.cohorts[cohortIndex].System.Targets[targetIndex]
		requestIndex, requested := energyPathVRFConsumptionRequestIndex(collector.plan, target)
		if !requested {
			continue
		}
		dictionary.sourceFile = sourceFile
		source := energyDataSourceForDictionary(dictionary)
		source.NormalizedUnit, source.AggregationMethod, source.AggregationBasis = "kWh", "sum", "model_total"
		source.EffectiveMultiplier, source.MultiplierApplication = 1, energyMultiplierAlreadyModelTotal
		source.ZoneName = target.ZoneName
		source.ObjectIndex = requestIndex
		source.Explanation = "Reported native VRF consumption constituent; original model total and private allocation evidence, not an additional Building end use."
		position := energyPathVRFObservationPosition{cohortIndex, len(collector.cohorts[cohortIndex].Observations)}
		collector.cohorts[cohortIndex].Observations = append(collector.cohorts[cohortIndex].Observations, energyPathVRFConsumptionObservation{
			Target: target, Source: source, Monthly: map[int]float64{}, InvalidMonths: map[int]bool{}, Requested: true})
		collector.byDictionary[dictionary.row.index] = append(collector.byDictionary[dictionary.row.index], position)
		collector.counts[position] = map[int]int{}
		out = append(out, dictionary)
	}
	if rows.Err() != nil {
		collector.axisInvalid = true
	}
	for id, positions := range collector.byDictionary {
		for _, position := range positions {
			observation := &collector.cohorts[position.cohort].Observations[position.observation]
			identity := observation.Target.Definition.ID + "\x00" + energyPathVRFName(observation.Source.KeyValue)
			// The source ID also must be unique across the original dictionary,
			// not only within this VRF family's selected metadata.
			observation.Invalid = collector.axisInvalid || identityCounts[identity] != 1 || idCounts[id] != 1
		}
	}
	return out
}

// Return true only for this collector's dictionaries. Even a NULL row is
// consumed here before the ordinary series builder can discard its presence.
func (collector *energyPathVRFConsumptionCollector) observe(row SQLSeriesRow) bool {
	if collector == nil {
		return false
	}
	positions := collector.byDictionary[row.DictionaryIndex]
	if len(positions) == 0 {
		return false
	}
	if collector.excludedTimes[row.TimeIndex] {
		return true
	}
	month, validTime := collector.byTime[row.TimeIndex]
	for _, position := range positions {
		observation := &collector.cohorts[position.cohort].Observations[position.observation]
		if !validTime || !row.Month.Valid || int(row.Month.Int64) != month || strings.TrimSpace(row.IntervalType) != "3" {
			observation.Invalid = true
			if row.Month.Valid && row.Month.Int64 >= 1 && row.Month.Int64 <= 12 {
				observation.InvalidMonths[int(row.Month.Int64)] = true
				delete(observation.Monthly, int(row.Month.Int64))
			}
			continue
		}
		collector.counts[position][month]++
		if collector.counts[position][month] != 1 || !row.Value.Valid || math.IsNaN(row.Value.Float64) || math.IsInf(row.Value.Float64, 0) || row.Value.Float64 < 0 {
			observation.InvalidMonths[month] = true
			delete(observation.Monthly, month)
			continue
		}
		if observation.InvalidMonths[month] {
			continue
		}
		number := row.Value.Float64 / 3600000
		if !energyPathFinite(number) {
			observation.InvalidMonths[month] = true
			continue
		}
		observation.Monthly[month] = number
	}
	return true
}

func (collector *energyPathVRFConsumptionCollector) result() ([]energyPathVRFConsumptionCohort, []EnergyDataSource) {
	if collector == nil {
		return nil, nil
	}
	var sources []EnergyDataSource
	for cohortIndex := range collector.cohorts {
		cohort := &collector.cohorts[cohortIndex]
		for observationIndex := range cohort.Observations {
			observation := &cohort.Observations[observationIndex]
			position := energyPathVRFObservationPosition{cohortIndex, observationIndex}
			complete := !collector.axisInvalid && !observation.Invalid && len(cohort.Months) > 0
			months := make([]int, 0, len(cohort.Months))
			for month := range cohort.Months {
				months = append(months, month)
			}
			sort.Ints(months)
			total := 0.0
			for _, month := range months {
				value, present := observation.Monthly[month]
				if collector.counts[position][month] != 1 || !present || observation.InvalidMonths[month] {
					observation.InvalidMonths[month] = true
					delete(observation.Monthly, month)
					complete = false
					continue
				}
				total += value
			}
			if complete && energyPathFinite(total) {
				observation.Source.RawValue = roundedEnergyNumber(total)
				observation.Source.EffectiveValue = observation.Source.RawValue
				observation.Source.observedValuePresence |= energySourceObservedRaw | energySourceObservedEffective
			}
			sources = append(sources, observation.Source)
		}
	}
	return collector.cohorts, sources
}
