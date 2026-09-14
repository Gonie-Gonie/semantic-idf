package simulation

import (
	"database/sql"
	"sort"
	"strings"
)

const energyPathBoilerAncillaryElectricityEnergy = "Boiler Ancillary Electricity Energy"
const energyPathBoilerAncillaryElectricityRate = "Boiler Ancillary Electricity Rate"

// Native baseboard and hot-water-boiler ancillary electricity are constituents
// of Heating:Electricity, not additional Building end uses. The recognized
// roster is intentionally not a claim that all other consumers are absent.
// An unidentified remainder stays unassigned, including when no typed owner or
// component observation could be established in a saved run.
func readEnergyPathHVACConsumptionPools(db *sql.DB, sourceFile string, plan *PurposeRunPlan, context energyDriverBuildContext, series []energyExplanationSeries) ([]energyPathHVACConsumptionPool, []EnergyDataSource, []string) {
	if !context.Enabled || !context.HasNativeBaseboard || !energyExplanationPlanUsesEnergyPath(plan) {
		return nil, nil, nil
	}
	pool := energyPathHVACConsumptionPool{ID: "native.heating.electricity", ServiceKind: "heating", Carrier: "electricity", Valid: true}
	for _, item := range series {
		if item.Stage == "end_use" && item.ZoneName == "" && item.EndUse == "heating" && item.Carrier == "electricity" && item.directComponentID == "" {
			pool.MeterSourceIDs = appendUniqueStrings(pool.MeterSourceIDs, item.SourceIDs...)
		}
	}
	if len(pool.MeterSourceIDs) == 0 {
		return nil, nil, nil // No actual parent observation exists to allocate.
	}
	for _, target := range context.BaseboardTargets {
		member := energyPathHVACConsumptionMember{ID: "baseboard.electricity:" + normalizePurposeToken(target.KeyValue),
			ObjectType: target.Component.ObjectType, ObjectName: target.KeyValue,
			OutputName: "Baseboard Electricity Energy", ZoneName: target.ZoneName,
			Series: energyExplanationSeries{ZoneName: target.ZoneName}}
		count := 0
		for _, item := range series {
			if item.directComponentID != "" && strings.EqualFold(item.SourceKey, target.KeyValue) && strings.EqualFold(item.SourceName, member.OutputName) && strings.EqualFold(item.ZoneName, target.ZoneName) && strings.EqualFold(item.sourceFrequency, "Monthly") {
				member.Series = item
				count++
			}
		}
		if count > 1 {
			pool.Valid = false
		}
		pool.Members = append(pool.Members, member)
	}
	axis := energyPathHVACConsumptionMonthlyAxis(db)
	hourly := newEnergySourceHourlyCollector(db)
	var sources []EnergyDataSource
	for _, target := range context.SharedHeatingElectricTargets {
		member := energyPathHVACConsumptionMember{ID: "boiler.ancillary.electricity:" + normalizePurposeToken(target.Component.ObjectName),
			ObjectType: target.Component.ObjectType, ObjectName: target.Component.ObjectName,
			OutputName: energyPathBoilerAncillaryElectricityEnergy, RelatedPathIDs: append([]string(nil), target.RelatedPathIDs...)}
		dictionaries, err := energyPathBoilerConsumptionDictionaries(db, sourceFile, target.Component.ObjectName)
		if err != nil {
			pool.Valid = false
			pool.Members = append(pool.Members, member)
			continue
		}
		identities := map[string]int{}
		for _, dictionary := range dictionaries {
			identities[normalizePurposeToken(dictionary.row.name)+"|"+normalizePurposeToken(dictionary.reportingFrequency)]++
		}
		for _, dictionary := range dictionaries {
			identity := normalizePurposeToken(dictionary.row.name) + "|" + normalizePurposeToken(dictionary.reportingFrequency)
			source, observed := energyPathReadBoilerConsumptionObservation(db, dictionary, plan, axis, hourly)
			source.RelatedEntityIDs = appendUniqueStrings(source.RelatedEntityIDs, target.Component.ID)
			sources = append(sources, source)
			if strings.EqualFold(dictionary.reportingFrequency, "Monthly") && strings.EqualFold(dictionary.row.name, member.OutputName) {
				if identities[identity] != 1 {
					pool.Valid = false
					continue
				}
				member.Series = observed
			}
		}
		pool.Members = append(pool.Members, member)
	}
	sort.Strings(pool.MeterSourceIDs)
	var labels []string
	if hourly != nil {
		labels = hourly.labels
	}
	return []energyPathHVACConsumptionPool{pool}, sources, labels
}

func energyPathHVACConsumptionMonthlyAxis(db *sql.DB) map[int64]int {
	known := energyPathObservedMonthlyTimeAxis(db)
	if len(known) == 0 {
		return nil
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM EnvironmentPeriods WHERE EnvironmentType=3`).Scan(&count); err != nil || count != 1 {
		return nil
	}
	rows, err := db.Query(`SELECT TimeIndex, Month FROM "Time"`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	axis, months := map[int64]int{}, map[int]bool{}
	for rows.Next() {
		var id int64
		var month int
		if err := rows.Scan(&id, &month); err != nil {
			return nil
		}
		if !known[id] {
			continue
		}
		if month < 1 || month > 12 || months[month] {
			return nil
		}
		axis[id], months[month] = month, true
	}
	if rows.Err() != nil || len(axis) != len(known) {
		return nil
	}
	return axis
}

func energyPathBoilerConsumptionDictionaries(db *sql.DB, sourceFile, key string) ([]energyExplanationDictionary, error) {
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,KeyValue,Name,Units,ReportingFrequency FROM ReportDataDictionary
WHERE IsMeter=0 AND LOWER(TRIM(KeyValue))=LOWER(TRIM(?)) AND LOWER(TRIM(Name)) IN
('boiler ancillary electricity energy','boiler ancillary electricity rate')
AND LOWER(TRIM(ReportingFrequency)) IN ('monthly','hourly') ORDER BY ReportDataDictionaryIndex`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []energyExplanationDictionary
	for rows.Next() {
		dictionary := energyExplanationDictionary{sourceFile: sourceFile}
		if err := rows.Scan(&dictionary.row.index, &dictionary.row.keyValue, &dictionary.row.name, &dictionary.row.units, &dictionary.reportingFrequency); err != nil {
			return nil, err
		}
		// A conversion-only definition permits native W integration; no series
		// from this reader is sent to the additive load/end-use classifier.
		if strings.EqualFold(dictionary.row.name, energyPathBoilerAncillaryElectricityRate) {
			dictionary.load = &energyLoadAliasDefinition{ServiceKind: "heating"}
		}
		out = append(out, dictionary)
	}
	return out, rows.Err()
}

func energyPathReadBoilerConsumptionObservation(db *sql.DB, dictionary energyExplanationDictionary, plan *PurposeRunPlan, axis map[int64]int, hourly *energySourceHourlyCollector) (EnergyDataSource, energyExplanationSeries) {
	source := energyDataSourceForDictionary(dictionary)
	source.DriverRole, source.InspectorSection = energyDriverSourceRoleContext, energyDriverInspectorSectionContext
	source.Explanation = "Native boiler ancillary electricity, included in Heating:Electricity. Allocation context only; this is not an additional Building end use or delivered heat."
	source.AggregationBasis, source.EffectiveMultiplier, source.MultiplierApplication = "model_total", 1, "already_model_total"
	definition := energyPathDirectHVACComponentDefinition{Energy: energyMeterAliasDefinition{Aliases: []string{dictionary.row.name}}}
	source.ObjectIndex = energyPathDirectHVACReportedObjectIndex(dictionary, plan, definition, "")
	item := energyExplanationSeries{Stage: "context", Level: "context", Kind: "energy.heating", EndUse: "heating", ServiceKind: "heating", Carrier: "electricity",
		SourceName: dictionary.row.name, SourceKey: dictionary.row.keyValue, sourceName: dictionary.row.name, sourceKeyValue: dictionary.row.keyValue,
		sourceFrequency: dictionary.reportingFrequency, Unit: "kWh", SourceIDs: []string{source.ID}, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", multiplierApplied: true}
	return energyPathReadNativeConstituentObservation(db, dictionary, axis, hourly, source, item)
}

// Shared observation arithmetic only. Callers supply independently reviewed
// physical roles and identities; this function cannot establish meter membership
// or a service allocation. No representative-Zone multiplier is applied here.
func energyPathReadNativeConstituentObservation(db *sql.DB, dictionary energyExplanationDictionary, axis map[int64]int, hourly *energySourceHourlyCollector, source EnergyDataSource, item energyExplanationSeries) (EnergyDataSource, energyExplanationSeries) {
	wantedUnit := "J"
	if energyExplanationIntegratesRate(dictionary) {
		wantedUnit = "W"
	}
	if dictionary.row.index <= 0 || !strings.EqualFold(strings.TrimSpace(dictionary.row.units), wantedUnit) {
		return source, item
	}
	source.NormalizedUnit = "kWh"
	rows, err := db.Query(`SELECT r.TimeIndex,r.Value,t."Interval" FROM ReportData r JOIN "Time" t ON t.TimeIndex=r.TimeIndex
JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex
WHERE r.ReportDataDictionaryIndex=? AND e.EnvironmentType=3 AND (t.WarmupFlag IS NULL OR t.WarmupFlag=0) ORDER BY r.TimeIndex`, dictionary.row.index)
	if err != nil {
		return source, item
	}
	defer rows.Close()
	values, counts, bad := map[int]float64{}, map[int]int{}, map[int]bool{}
	hourlyTotal := 0.0
	for rows.Next() {
		var id int64
		var value, minutes sql.NullFloat64
		if err := rows.Scan(&id, &value, &minutes); err != nil {
			return source, item
		}
		if strings.EqualFold(dictionary.reportingFrequency, "Hourly") {
			row := SQLSeriesRow{TimeIndex: id, DictionaryIndex: dictionary.row.index, Value: value}
			if value.Valid && value.Float64 < 0 {
				row.Value.Valid = false
			}
			hourly.observe(row, dictionary)
			if hourly != nil {
				if _, exists := hourly.times[id]; exists && row.Value.Valid && energyPathFinite(value.Float64) {
					if wantedUnit == "W" {
						hourlyTotal += value.Float64 / 1000
					} else {
						hourlyTotal += value.Float64 / 3600000
					}
				}
			}
			continue
		}
		month, exists := axis[id]
		if !exists {
			continue
		}
		counts[month]++
		if counts[month] != 1 || !value.Valid || !energyPathFinite(value.Float64) || value.Float64 < 0 || !minutes.Valid || !energyPathFinite(minutes.Float64) || minutes.Float64 <= 0 {
			bad[month] = true
			continue
		}
		// Retain native precision in the constituent budget. Only the final
		// allocation/display rounds; source charts retain their separate
		// existing 0.001 kWh transport contract.
		number := value.Float64 / 3600000
		if wantedUnit == "W" {
			number = value.Float64 * (minutes.Float64 / 60) / 1000
		}
		if !energyPathFinite(number) || number < 0 {
			bad[month] = true
			continue
		}
		values[month] = number
	}
	if rows.Err() != nil {
		return source, item
	}
	if strings.EqualFold(dictionary.reportingFrequency, "Hourly") {
		source.HourlyEnergy = hourly.energy(dictionary.row.index)
		if source.HourlyEnergy != nil && energyPathFinite(hourlyTotal) {
			source.RawValue = hourlyTotal
			source.EffectiveValue = source.RawValue
			source.observedValuePresence = energySourceObservedRaw | energySourceObservedEffective
		}
		return source, item
	}
	item.Monthly, item.RawMonthly = map[int]float64{}, map[int]float64{}
	for month := 1; month <= 12; month++ {
		value, exists := values[month]
		if !exists || bad[month] || counts[month] != 1 {
			continue
		}
		item.Monthly[month], item.RawMonthly[month] = value, value
		item.Total += value
	}
	item.RawTotal = item.Total
	if len(item.Monthly) > 0 {
		item.MonthlySourceIDs = []string{source.ID}
	}
	if len(axis) > 0 && len(item.Monthly) == len(axis) && energyPathFinite(item.Total) {
		source.RawValue, source.EffectiveValue = item.Total, item.Total
		source.observedValuePresence = energySourceObservedRaw | energySourceObservedEffective
		item.AnnualSourceIDs = []string{source.ID}
	}
	return source, item
}
