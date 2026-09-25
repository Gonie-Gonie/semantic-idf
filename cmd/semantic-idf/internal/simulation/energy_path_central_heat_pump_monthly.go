package simulation

import (
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func energyPathHasCentralHeatPump(doc idf.Document) bool {
	for _, object := range doc.Objects {
		if strings.EqualFold(strings.TrimSpace(object.Type), "CentralHeatPumpSystem") {
			return true
		}
	}
	return false
}

// Called after generic Hourly mirroring: this bounded source step needs only
// two native Monthly Energy budgets, not extra thermal/rate/module requests.
func (builder *purposePlanBuilder) addEnergyPathCentralHeatPumpMonthlyOutputs() {
	for _, target := range energyPathCentralHeatPumpOutputTargets(builder.doc) {
		if target.Definition.Role != energyPathCentralHeatPumpPurchased {
			continue
		}
		covers := func(output PurposeOutputObject) bool {
			key := strings.TrimSpace(output.KeyValue)
			return strings.EqualFold(output.ObjectType, "Output:Variable") &&
				strings.EqualFold(output.VariableName, target.Definition.EnergyName) &&
				strings.EqualFold(output.ReportingFrequency, "Monthly") && output.ScopeZoneName == "" &&
				strings.TrimSpace(purposeFieldValue(output.Fields, "Schedule Name")) == "" &&
				(key == "" || key == "*" || strings.EqualFold(key, target.System.ObjectName))
		}
		reused := false
		for i := range builder.objects {
			if covers(builder.objects[i]) {
				builder.objects[i].PurposeIDs = normalizePurposeIDs(append(builder.objects[i].PurposeIDs, SimulationPurposeBasicEnergy))
				reused = true
			}
		}
		if reused {
			continue
		}
		var existing []PurposeOutputObject
		for _, output := range builder.existing {
			if covers(output) {
				existing = append(existing, output)
			}
		}
		if len(existing) > 0 {
			sort.Slice(existing, func(i, j int) bool { return existing[i].Signature < existing[j].Signature })
			output := existing[0]
			output.PurposeIDs = []SimulationPurposeID{SimulationPurposeBasicEnergy}
			output.Reason = "Basic Energy Path"
			builder.addObject(output)
			continue
		}
		builder.addVariableWithReason(SimulationPurposeBasicEnergy, target.System.ObjectName, target.Definition.EnergyName, "Monthly", "medium",
			"Native system electricity constituent already included in its Cooling or Heating meter. Global model-total budget, never module-count scaled; unproved recipients remain unassigned.", "Basic Energy Path")
	}
}

// Preserve separate paid-service cohorts even when a requested observation is
// absent. No paths are authorized here: the still-required complete original
// recipient/controller/return proof is not replaced by a meter name or key.
// Both pools are appended before the existing single reservation call, so an
// independently measured sibling boiler/local cohort is not overwritten.
func readEnergyPathCentralHeatPumpMonthlyPools(db *sql.DB, sourceFile string, plan *PurposeRunPlan, context energyDriverBuildContext, series []energyExplanationSeries) ([]energyPathHVACConsumptionPool, []EnergyDataSource) {
	if !context.Enabled || !context.HasCentralHeatPump || !energyExplanationPlanUsesEnergyPath(plan) {
		return nil, nil
	}
	axis := energyPathHVACConsumptionMonthlyAxis(db)
	var pools []energyPathHVACConsumptionPool
	var sources []EnergyDataSource
	for _, service := range []string{"cooling", "heating"} {
		pool := energyPathHVACConsumptionPool{ID: "native.central_heat_pump." + service + ".electricity", ServiceKind: service, Carrier: "electricity", Valid: true}
		for _, item := range series {
			if item.Stage == "end_use" && item.ZoneName == "" && item.EndUse == service && item.Carrier == "electricity" && item.directComponentID == "" && item.Basis == "measured_meter" &&
				strings.EqualFold(firstNonEmpty(item.SourceName, item.sourceName), service+":Electricity") {
				pool.MeterSourceIDs = appendUniqueStrings(pool.MeterSourceIDs, item.SourceIDs...)
			}
		}
		seen := map[string]bool{}
		for _, target := range context.CentralHeatPumpTargets {
			if target.Definition.Role != energyPathCentralHeatPumpPurchased || target.Definition.ServiceKind != service {
				continue
			}
			member := energyPathHVACConsumptionMember{ID: target.Definition.ID + ":" + target.System.ID,
				ObjectType: target.System.ObjectType, ObjectName: target.System.ObjectName, OutputName: target.Definition.EnergyName}
			member.RelatedPathIDs = append([]string(nil), context.CentralHeatPumpPaths[member.ID]...)
			identity := normalizePurposeToken(member.ObjectName) + "|" + service
			if seen[identity] {
				pool.Valid = false
			}
			seen[identity] = true
			dictionaries, err := energyPathCentralHeatPumpMonthlyDictionaries(db, sourceFile, target)
			if err != nil || len(dictionaries) > 1 {
				pool.Valid = false
			}
			for _, dictionary := range dictionaries {
				source := energyDataSourceForDictionary(dictionary.dictionary)
				source.DriverRole, source.InspectorSection = energyDriverSourceRoleContext, energyDriverInspectorSectionContext
				source.AggregationBasis, source.EffectiveMultiplier, source.MultiplierApplication = "model_total", 1, "already_model_total"
				source.RelatedEntityIDs = appendUniqueStrings(source.RelatedEntityIDs, target.System.ID)
				source.Explanation = "Native system " + service + " electricity, already included in the broad service meter. Global source observation; recipient ownership is not established by this observation."
				var requested []PurposeOutputObject
				for _, output := range plan.OutputObjects {
					key := strings.TrimSpace(output.KeyValue)
					if strings.EqualFold(output.ObjectType, "Output:Variable") && strings.EqualFold(output.VariableName, member.OutputName) &&
						strings.EqualFold(output.ReportingFrequency, "Monthly") && output.ScopeZoneName == "" &&
						(key == "" || key == "*" || strings.EqualFold(key, member.ObjectName)) &&
						strings.TrimSpace(purposeFieldValue(output.Fields, "Schedule Name")) == "" && purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) {
						requested = append(requested, output)
					}
				}
				if len(requested) > 0 && len(dictionaries) == 1 && dictionary.valid {
					definition := energyPathDirectHVACComponentDefinition{Energy: energyMeterAliasDefinition{Aliases: []string{member.OutputName}}}
					// Native blank keys default to wildcard. Bind navigation only to
					// the unfiltered requests that authorized this observation; a
					// scheduled exact-key request cannot steal the blank owner.
					source.ObjectIndex = energyPathDirectHVACReportedObjectIndex(dictionary.dictionary, &PurposeRunPlan{OutputObjects: requested}, definition, "")
					source, member.Series = energyPathReadCentralHeatPumpMonthly(db, dictionary.dictionary, axis, source, service)
				}
				sources = append(sources, source)
			}
			pool.Members = append(pool.Members, member)
		}
		if len(pool.Members) == 0 {
			// An ambiguous/unsupported original reporting roster must not reopen
			// a generic broad-meter allocation under the same native family.
			pool.Valid = false
		}
		sort.Strings(pool.MeterSourceIDs)
		pools = append(pools, pool)
	}
	return pools, sources
}

type energyPathCentralHeatPumpMonthlyDictionary struct {
	dictionary energyExplanationDictionary
	valid      bool
}

func energyPathCentralHeatPumpMonthlyDictionaries(db *sql.DB, sourceFile string, target energyPathCentralHeatPumpOutputTarget) ([]energyPathCentralHeatPumpMonthlyDictionary, error) {
	// Do not prefilter IsMeter/units/type: a contradictory exact dictionary
	// remains an unknown source rather than silently becoming reported zero.
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,KeyValue,Name,Units,ReportingFrequency,IsMeter,Type,TimestepType,IndexGroup,ScheduleName
FROM ReportDataDictionary WHERE LOWER(TRIM(KeyValue))=LOWER(TRIM(?)) AND LOWER(TRIM(Name))=LOWER(TRIM(?)) AND LOWER(TRIM(ReportingFrequency))='monthly'
ORDER BY ReportDataDictionaryIndex`, target.System.ObjectName, target.Definition.EnergyName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []energyPathCentralHeatPumpMonthlyDictionary
	for rows.Next() {
		d := energyExplanationDictionary{sourceFile: sourceFile}
		var meter sql.NullInt64
		var kind, step, group, schedule sql.NullString
		if err := rows.Scan(&d.row.index, &d.row.keyValue, &d.row.name, &d.row.units, &d.reportingFrequency, &meter, &kind, &step, &group, &schedule); err != nil {
			return nil, err
		}
		d.isMeter, d.indexGroup = meter.Valid && meter.Int64 != 0, group.String
		valid := d.row.index > 0 && meter.Valid && meter.Int64 == 0 && kind.Valid && kind.String == "Sum" && step.Valid && step.String == "HVAC System" && group.Valid && group.String == "System" && strings.TrimSpace(schedule.String) == "" &&
			energyPathCentralHeatPumpMonthlyBudgetIdentity(target, d.row.name, d.row.keyValue, d.row.units, d.reportingFrequency, meter.Int64 != 0)
		out = append(out, energyPathCentralHeatPumpMonthlyDictionary{dictionary: d, valid: valid})
	}
	return out, rows.Err()
}

func energyPathReadCentralHeatPumpMonthly(db *sql.DB, dictionary energyExplanationDictionary, axis map[int64]int, source EnergyDataSource, service string) (EnergyDataSource, energyExplanationSeries) {
	item := energyExplanationSeries{Stage: "context", Level: "context", Kind: "energy." + service, EndUse: service, ServiceKind: service, Carrier: "electricity", Unit: "kWh",
		SourceName: dictionary.row.name, SourceKey: dictionary.row.keyValue, sourceName: dictionary.row.name, sourceKeyValue: dictionary.row.keyValue, sourceFrequency: "Monthly",
		SourceIDs: []string{source.ID}, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", multiplierApplied: true}
	if len(axis) == 0 {
		return source, item
	}
	rows, err := db.Query(`SELECT r.ReportDataIndex,r.TimeIndex,r.Value,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,e.EnvironmentType,t.WarmupFlag
FROM ReportData r LEFT JOIN "Time" t ON t.TimeIndex=r.TimeIndex LEFT JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex
WHERE r.ReportDataDictionaryIndex=? ORDER BY r.ReportDataIndex`, dictionary.row.index)
	if err != nil {
		return source, item
	}
	defer rows.Close()
	values, counts, bad, seenRows := map[int]float64{}, map[int]int{}, map[int]bool{}, map[int64]bool{}
	for rows.Next() {
		var row, index, year, month, day, hour, minute, intervalType, environment, warmup sql.NullInt64
		var value, interval sql.NullFloat64
		if err := rows.Scan(&row, &index, &value, &year, &month, &day, &hour, &minute, &interval, &intervalType, &environment, &warmup); err != nil {
			return source, item
		}
		if !environment.Valid || !index.Valid || !row.Valid || row.Int64 <= 0 || seenRows[row.Int64] {
			return source, item
		}
		seenRows[row.Int64] = true
		if environment.Int64 != 3 || warmup.Valid && warmup.Int64 == 1 {
			continue
		}
		m, exists := axis[index.Int64]
		if !exists {
			return source, item // Never borrow another weather frequency or orphan.
		}
		counts[m]++
		if counts[m] != 1 || !value.Valid || !energyPathFinite(value.Float64) || value.Float64 < 0 || !year.Valid || year.Int64 < 1 || year.Int64 > 9999 ||
			!month.Valid || month.Int64 != int64(m) || !day.Valid || !hour.Valid || hour.Int64 != 24 || !minute.Valid || minute.Int64 != 0 ||
			!intervalType.Valid || intervalType.Int64 != 3 || warmup.Valid && warmup.Int64 != 0 || !interval.Valid || !energyPathFinite(interval.Float64) {
			bad[m] = true
			continue
		}
		last := time.Date(int(year.Int64), time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if day.Int64 != int64(last) || interval.Float64 != float64(last*1440) {
			bad[m] = true // Full native Monthly cells only; no extrapolation.
			continue
		}
		values[m] = value.Float64 / 3600000
	}
	if rows.Err() != nil {
		return source, item
	}
	item.Monthly, item.RawMonthly = map[int]float64{}, map[int]float64{}
	for month := 1; month <= 12; month++ {
		value, exists := values[month]
		if !exists || bad[month] || counts[month] != 1 || !energyPathFinite(value) {
			continue
		}
		item.Monthly[month], item.RawMonthly[month] = value, value
		item.Total += value
	}
	item.RawTotal = item.Total
	source.NormalizedUnit = "kWh"
	if len(item.Monthly) > 0 {
		item.MonthlySourceIDs = []string{source.ID}
	}
	if len(item.Monthly) == 12 && len(axis) == 12 && energyPathFinite(item.Total) {
		item.AnnualSourceIDs = []string{source.ID}
		source.RawValue, source.EffectiveValue = item.Total, item.Total
		source.observedValuePresence = energySourceObservedRaw | energySourceObservedEffective
	}
	return source, item
}
