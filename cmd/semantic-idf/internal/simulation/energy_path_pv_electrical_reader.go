package simulation

// Native observations only: no additive series, nodes,
// allocation, Zone recipient, electrical topology or accounting equation.
import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

type energyPathPVElectricalNativeDictionary struct {
	Index                                                                int
	IsMeter                                                              sql.NullInt64
	Name, Key, Unit, Frequency, Type, TimestepType, IndexGroup, Schedule sql.NullString
}

type energyPathPVElectricalObservation struct {
	Intent       energyPathPVElectricalIntent
	Status       string
	Dictionaries []energyPathPVElectricalNativeDictionary
	SourceIDs    []string
	// Native J / 3.6e6, no per-row display rounding and no Hourly fallback.
	// A map entry distinguishes an observed zero from an absent/invalid month.
	MonthlyValues                                         map[int]float64
	MissingTimes, NullTimes, InvalidTimes, DuplicateTimes []int64
	ExcludedDesignOrWarmupRows                            int
	HasNegativeValue                                      bool                                // Unrounded native weather cells, not display points.
	HourlyValues                                          []energyPathPVElectricalHourlyValue // Unrounded, individually validated weather cells only.
}

type energyPathPVElectricalHourlyValue struct {
	Month, Day, Hour int
	Value            float64
}

type energyPathPVElectricalEvidence struct {
	SourceFile            string                              // Caller-provided SQL provenance label, not a hash proof.
	Inventory             energyPathPVElectricalInventory     // Original physical refs ONLY here.
	Observations          []energyPathPVElectricalObservation // All M/H intents, including missing.
	UnmatchedDictionaries []energyPathPVElectricalNativeDictionary
	SourceSnapshots       []EnergyDataSource // One authoritative native snapshot per actual RDD.
	HourlyLabels          []string
	Issues                []string
}

func energyPathPVElectricalNativeIdentity(name, key, frequency string) string {
	return energyPathPVElectricalKey(name) + "\x00" + energyPathPVElectricalKey(key) + "\x00" + energyPathPVElectricalKey(frequency)
}

// Call while the canonical parser's read-only DB is open, after generic sources
// have been collected. Return merged sources, NOT an append-only additions list.
// The evidence snapshots can reapply observation truth after later source
// annotation; graph arithmetic must never reconstitute an unknown native scalar.
func readEnergyPathPVElectricalObservations(db *sql.DB, sourceFile string, plan *PurposeRunPlan, inventory energyPathPVElectricalInventory, existing []EnergyDataSource) (energyPathPVElectricalEvidence, []EnergyDataSource, []string, error) {
	evidence := energyPathPVElectricalEvidence{SourceFile: sourceFile, Inventory: inventory}
	if !inventory.HasOriginal || !inventory.SchemaReviewed || !energyExplanationPlanUsesEnergyPath(plan) {
		return evidence, append([]EnergyDataSource(nil), existing...), nil, nil
	}
	// Keep unresolved named owners visible without authorizing their quantities.
	byIdentity := map[string]int{}
	for _, target := range inventory.Targets {
		for _, frequency := range []string{"Monthly", "Hourly"} {
			intent := energyPathPVElectricalIntent{Target: target, Frequency: frequency}
			key := energyPathPVElectricalNativeIdentity(target.Definition.Name, target.Key, frequency)
			if _, duplicate := byIdentity[key]; duplicate {
				return evidence, nil, nil, fmt.Errorf("duplicate original PV electrical intent %q", key)
			}
			byIdentity[key] = len(evidence.Observations)
			evidence.Observations = append(evidence.Observations, energyPathPVElectricalObservation{Intent: intent, Status: "missing_dictionary"})
		}
	}
	if len(evidence.Observations) == 0 {
		return evidence, append([]EnergyDataSource(nil), existing...), nil, nil
	}
	dictionaries, err := energyPathPVElectricalReadDictionaries(db)
	if err != nil {
		// Do not leave a generic reader's guessed scalar known when this native
		// metadata could not be verified. Existing identities are not fabricated.
		for i := range evidence.Observations {
			evidence.Observations[i].Status = "native_schema_unavailable"
		}
		for _, source := range existing {
			if _, owned := byIdentity[energyPathPVElectricalNativeIdentity(source.Name, source.KeyValue, source.ReportingFrequency)]; owned {
				evidence.SourceSnapshots = append(evidence.SourceSnapshots, energyPathPVElectricalUnknownSource(source))
			}
		}
		evidence.Issues = append(evidence.Issues, err.Error())
		return evidence, applyEnergyPathPVElectricalSourceObservations(existing, evidence), nil, err
	}
	for _, dictionary := range dictionaries {
		identity := energyPathPVElectricalNativeIdentity(dictionary.Name.String, dictionary.Key.String, dictionary.Frequency.String)
		if index, found := byIdentity[identity]; found {
			evidence.Observations[index].Dictionaries = append(evidence.Observations[index].Dictionaries, dictionary)
		} else {
			// A same-family foreign/NULL equipment key is not borrowed for any
			// original target. Preserve the actual dictionary as unresolved.
			evidence.UnmatchedDictionaries = append(evidence.UnmatchedDictionaries, dictionary)
			source := energyPathPVElectricalSource(dictionary)
			source.Explanation = "Native electrical dictionary has no exact reviewed original reporting owner/key. Identity retained; no quantity or Zone/accounting authority inferred."
			evidence.SourceSnapshots = append(evidence.SourceSnapshots, source)
		}
	}
	actualIDs := map[string]bool{}
	for _, dictionary := range dictionaries {
		actualIDs[fmt.Sprintf("sql-rdd-%d", dictionary.Index)] = true
	}
	for _, source := range existing {
		if _, owned := byIdentity[energyPathPVElectricalNativeIdentity(source.Name, source.KeyValue, source.ReportingFrequency)]; owned && !actualIDs[source.ID] {
			unknown := energyPathPVElectricalUnknownSource(source)
			unknown.Explanation = "Prior source identity retained, but its exact native dictionary is missing or outside the reviewed Monthly/Hourly roster. No observed zero or graph-derived quantity is inferred."
			evidence.SourceSnapshots = append(evidence.SourceSnapshots, unknown)
		}
	}
	monthlyAxis := energyPathHVACConsumptionMonthlyAxis(db)
	hourly := newEnergySourceHourlyCollector(db)
	for i := range evidence.Observations {
		observation := &evidence.Observations[i]
		if len(observation.Dictionaries) == 0 {
			continue
		}
		observation.Status = "duplicate_dictionary"
		for _, dictionary := range observation.Dictionaries {
			source := energyPathPVElectricalSource(dictionary)
			observation.SourceIDs = append(observation.SourceIDs, source.ID)
			source.ObjectIndex = energyPathPVElectricalOutputRequestIndex(observation.Intent, plan, inventory.OriginalOutputs)
			source.Explanation = "Native model-total electrical observation; role=" + observation.Intent.Target.Definition.Role + ". Nonadditive source context, not a new consumption, supply or Zone flow. ObjectIndex, when present, identifies the unique original Output request at this frequency, not a physical owner or executed index."
			valid := len(observation.Dictionaries) == 1 && energyPathPVElectricalDictionaryValid(dictionary, observation.Intent)
			if len(observation.Dictionaries) == 1 && !valid {
				observation.Status = "invalid_native_dictionary_or_owner"
			}
			if valid {
				source, err = energyPathPVElectricalReadObservation(db, dictionary, *observation, monthlyAxis, hourly, source, observation)
				if err != nil {
					evidence.Issues = append(evidence.Issues, fmt.Sprintf("%s: %v", source.ID, err))
				}
			}
			evidence.SourceSnapshots = append(evidence.SourceSnapshots, source)
		}
	}
	for _, source := range evidence.SourceSnapshots {
		if source.HourlyEnergy != nil && hourly != nil {
			evidence.HourlyLabels = append([]string(nil), hourly.labels...)
			break
		}
	}
	return evidence, applyEnergyPathPVElectricalSourceObservations(existing, evidence), append([]string(nil), evidence.HourlyLabels...), nil
}

func energyPathPVElectricalReadDictionaries(db *sql.DB) ([]energyPathPVElectricalNativeDictionary, error) {
	if db == nil {
		return nil, fmt.Errorf("native PV electrical SQL database is unavailable")
	}
	var names, placeholders []string
	for _, definition := range energyPathPVElectricalDefinitions() {
		names = append(names, definition.Name)
		placeholders = append(placeholders, "?")
	}
	args := make([]any, len(names))
	for i, name := range names {
		args[i] = name
	}
	// Do not filter invalid IsMeter/Units/Type away: retain then reject the
	// actual source identity. ScheduleName belongs to the native 25.1 schema.
	query := `SELECT ReportDataDictionaryIndex,IsMeter,Name,KeyValue,Units,ReportingFrequency,Type,TimestepType,IndexGroup,ScheduleName FROM ReportDataDictionary WHERE Name COLLATE NOCASE IN (` + strings.Join(placeholders, ",") + `) AND LOWER(TRIM(ReportingFrequency)) IN ('monthly','hourly') ORDER BY ReportDataDictionaryIndex`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []energyPathPVElectricalNativeDictionary
	for rows.Next() {
		var d energyPathPVElectricalNativeDictionary
		if err := rows.Scan(&d.Index, &d.IsMeter, &d.Name, &d.Key, &d.Unit, &d.Frequency, &d.Type, &d.TimestepType, &d.IndexGroup, &d.Schedule); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func energyPathPVElectricalDictionaryValid(d energyPathPVElectricalNativeDictionary, intent energyPathPVElectricalIntent) bool {
	if d.Index <= 0 || !d.IsMeter.Valid || d.IsMeter.Int64 != 0 && d.IsMeter.Int64 != 1 || !d.Name.Valid || !d.Unit.Valid || !d.Frequency.Valid || !d.Type.Valid || !d.TimestepType.Valid || d.Type.String != "Sum" || strings.TrimSpace(d.Schedule.String) != "" {
		return false
	}
	wantStep := "HVAC System"
	if intent.Target.Definition.IsMeter {
		wantStep = "Zone"
	} else if !d.Key.Valid || strings.TrimSpace(d.Key.String) == "" {
		return false
	}
	return d.TimestepType.String == wantStep && d.Frequency.String == intent.Frequency && intent.Target.matchesNativeDictionary(d.Name.String, d.Key.String, d.Unit.String, d.Frequency.String, d.IsMeter.Int64 == 1)
}

func energyPathPVElectricalDictionary(d energyPathPVElectricalNativeDictionary) energyExplanationDictionary {
	return energyExplanationDictionary{row: sqlOutputDictionaryRow{index: d.Index, keyValue: d.Key.String, name: d.Name.String, units: d.Unit.String, reportingFrequency: d.Frequency.String}, isMeter: d.IsMeter.Valid && d.IsMeter.Int64 == 1, reportingFrequency: d.Frequency.String, indexGroup: d.IndexGroup.String}
}

func energyPathPVElectricalSource(d energyPathPVElectricalNativeDictionary) EnergyDataSource {
	source := energyDataSourceForDictionary(energyPathPVElectricalDictionary(d))
	// Exact actual SQL spelling is retained, not replaced with original names.
	source.Name, source.KeyValue, source.Units, source.SourceUnit, source.ReportingFrequency = d.Name.String, d.Key.String, d.Unit.String, d.Unit.String, d.Frequency.String
	source.DriverRole, source.InspectorSection = energyDriverSourceRoleContext, energyDriverInspectorSectionContext
	source.AggregationBasis, source.EffectiveMultiplier, source.MultiplierApplication = "model_total", 1, "already_model_total"
	if d.Unit.Valid && d.Unit.String == "J" {
		source.NormalizedUnit = "kWh"
	}
	return source
}

// Only request navigation. Neither Target.OriginalOwners indices nor separately
// shifted executed physical indices can become this Output-request opener.
func energyPathPVElectricalOutputRequestIndex(intent energyPathPVElectricalIntent, plan *PurposeRunPlan, originals []energyPathPVElectricalOriginalOutput) *int {
	if plan == nil {
		return nil
	}
	// The normal builder deduplicates identical original signatures. Count
	// every covering raw original declaration BEFORE consulting that plan.
	originalCount, originalIndex := 0, -1
	for _, original := range originals {
		if !energyPathPVElectricalRequestCovers(original.request(), intent) {
			continue
		}
		if !original.NativeShapeValid || original.ObjectIndex < 0 {
			return nil
		}
		originalCount++
		originalIndex = original.ObjectIndex
	}
	if originalCount != 1 {
		return nil
	}
	seen := map[string]bool{}
	count := 0
	var index *int
	for _, output := range plan.OutputObjects {
		if !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) || output.ScopeZoneName != "" || !energyPathPVElectricalRequestCovers(output, intent) {
			continue
		}
		if output.ReportingFrequency != "" && !strings.EqualFold(canonicalPurposeFrequency(output.ReportingFrequency), intent.Frequency) {
			continue
		}
		key := PurposeOutputSignature(output.ObjectType, output.Fields)
		if output.ObjectIndex != nil {
			key += fmt.Sprintf("|original:%d", *output.ObjectIndex)
		} else {
			key += "|temporary"
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		count++
		if output.ObjectIndex != nil && *output.ObjectIndex >= 0 {
			value := *output.ObjectIndex
			index = &value
		}
	}
	if count != 1 || index == nil || *index != originalIndex {
		return nil
	}
	return index
}

func energyPathPVElectricalReadObservation(db *sql.DB, d energyPathPVElectricalNativeDictionary, input energyPathPVElectricalObservation, monthlyAxis map[int64]int, hourly *energySourceHourlyCollector, source EnergyDataSource, out *energyPathPVElectricalObservation) (EnergyDataSource, error) {
	axis := map[int64]bool{}
	if input.Intent.Frequency == "Monthly" {
		for id := range monthlyAxis {
			axis[id] = true
		}
	} else if hourly != nil {
		for id := range hourly.times {
			axis[id] = true
		}
	}
	if len(axis) == 0 {
		out.Status = "weather_axis_unavailable"
		return source, nil
	}
	rows, err := db.Query(`SELECT r.TimeIndex,r.Value,t.TimeIndex,e.EnvironmentType,t.WarmupFlag FROM ReportData r LEFT JOIN "Time" t ON t.TimeIndex=r.TimeIndex LEFT JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex WHERE r.ReportDataDictionaryIndex=? ORDER BY r.TimeIndex,r.ReportDataIndex`, d.Index)
	if err != nil {
		out.Status = "rows_unavailable"
		return source, err
	}
	defer rows.Close()
	counts := map[int64]int{}
	values := map[int64]float64{}
	bad := map[int64]bool{}
	offAxis := false
	dictionary := energyPathPVElectricalDictionary(d)
	for rows.Next() {
		var id, timeID, environment, warmup sql.NullInt64
		var value sql.NullFloat64
		if err := rows.Scan(&id, &value, &timeID, &environment, &warmup); err != nil {
			out.Status = "invalid_sql_row"
			return source, err
		}
		if !id.Valid || !timeID.Valid || !environment.Valid {
			offAxis = true
			out.InvalidTimes = append(out.InvalidTimes, id.Int64)
			continue
		}
		if environment.Int64 != 3 || warmup.Valid && warmup.Int64 != 0 {
			out.ExcludedDesignOrWarmupRows++
			continue
		}
		if !axis[id.Int64] {
			offAxis = true
			out.InvalidTimes = append(out.InvalidTimes, id.Int64)
			continue
		}
		counts[id.Int64]++
		if counts[id.Int64] > 1 {
			bad[id.Int64] = true
			out.DuplicateTimes = append(out.DuplicateTimes, id.Int64)
		}
		valid := value.Valid && input.Intent.Target.Definition.acceptsNativeEnergy(&value.Float64)
		if !value.Valid {
			bad[id.Int64] = true
			out.NullTimes = append(out.NullTimes, id.Int64)
		} else if !valid {
			bad[id.Int64] = true
			out.InvalidTimes = append(out.InvalidTimes, id.Int64)
		}
		if valid {
			number := value.Float64 / 3600000
			if energyPathFinite(number) {
				values[id.Int64] = number
				if number < 0 {
					out.HasNegativeValue = true
				}
			} else {
				valid = false
				bad[id.Int64] = true
				out.InvalidTimes = append(out.InvalidTimes, id.Int64)
			}
		}
		if input.Intent.Frequency == "Hourly" {
			// Sign policy is checked BEFORE the shared collector. It already
			// preserves signed J and rounds each chart point by the existing
			// 0.001-kWh transport convention. Pool positivity remains untouched.
			value.Valid = valid
			hourly.observe(SQLSeriesRow{TimeIndex: id.Int64, DictionaryIndex: d.Index, Value: value}, dictionary)
		}
	}
	if err := rows.Err(); err != nil {
		out.Status = "invalid_sql_row"
		return source, err
	}
	ids := make([]int64, 0, len(axis))
	for id := range axis {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	complete := !offAxis
	total := 0.0
	if input.Intent.Frequency == "Monthly" {
		out.MonthlyValues = map[int]float64{}
	}
	for _, id := range ids {
		if counts[id] == 0 {
			out.MissingTimes = append(out.MissingTimes, id)
		}
		value, exists := values[id]
		if counts[id] != 1 || bad[id] || !exists {
			complete = false
			continue
		}
		total += value
		if input.Intent.Frequency == "Monthly" {
			out.MonthlyValues[monthlyAxis[id]] = value
		} else if hourly != nil {
			// The collector already validated this exact calendar and 60-minute
			// weather axis. Keep unrounded observations separately from its
			// rounded display points; graph budgets must not reuse generic rows.
			var cell energyPathPVElectricalHourlyValue
			if _, err := fmt.Sscanf(hourly.labels[hourly.times[id]], "%02d-%02d %02d:00", &cell.Month, &cell.Day, &cell.Hour); err != nil {
				complete = false
				continue
			}
			cell.Value = value
			out.HourlyValues = append(out.HourlyValues, cell)
		}
	}
	if !energyPathFinite(total) {
		complete = false
	}
	if !complete {
		out.Status = "incomplete_or_invalid_observation"
		return source, nil
	}
	if input.Intent.Frequency == "Hourly" {
		source.HourlyEnergy = hourly.energy(d.Index)
		if source.HourlyEnergy == nil {
			out.Status = "incomplete_or_invalid_observation"
			return source, nil
		}
	}
	source.RawValue, source.EffectiveValue = total, total
	source.observedValuePresence = energySourceObservedRaw | energySourceObservedEffective
	out.Status = "observed"
	return source, nil
}

func energyPathPVElectricalUnknownSource(source EnergyDataSource) EnergyDataSource {
	source.RawValue, source.EffectiveValue = 0, 0
	source.observedValuePresence = 0
	source.inspectorDecodedFromJSON, source.inspectorValuePresence = false, 0
	source.HourlyEnergy = nil
	source.ObjectIndex = nil
	source.AllocatedValue, source.AllocationFactor, source.AllocationApplied = 0, 0, false
	source.AllocationExplanation, source.AllocationFormula = "", ""
	source.ScopeDetails = nil
	source.RelatedEntityIDs, source.InputSourceIDs = nil, nil
	source.ZoneName = ""
	source.DriverRole, source.DriverCategory = energyDriverSourceRoleContext, ""
	source.DriverComponent, source.HeatDirection, source.Formula = "", "", ""
	source.InspectorSection = energyDriverInspectorSectionContext
	source.AggregationBasis, source.EffectiveMultiplier, source.MultiplierApplication = "model_total", 1, "already_model_total"
	source.Explanation = "Native electrical observation could not be revalidated. Existing source identity is retained without an observed scalar, allocation or physical-owner claim."
	return source
}

func energyPathPVElectricalCopySnapshot(source EnergyDataSource) EnergyDataSource {
	source.pvObservationProtection = cloneEnergyPathPVObservationProtection(source.pvObservationProtection)
	if source.HourlyEnergy != nil {
		hourly := *source.HourlyEnergy
		hourly.Values = append([]float64(nil), hourly.Values...)
		source.HourlyEnergy = &hourly
	}
	source.InputSourceIDs = append([]string(nil), source.InputSourceIDs...)
	source.RelatedEntityIDs = append([]string(nil), source.RelatedEntityIDs...)
	source.ScopeDetails = append([]EnergyDataSourceScopeDetail(nil), source.ScopeDetails...)
	return source
}

// Exact RDD-ID replacement, never name-based sum/alias merge. Even a previously
// known generic scalar is replaced with the native revalidation result. All
// unrelated sources are untouched. Duplicate copies of one matching RDD are
// collapsed to the one source observation, not treated as separate energy.
func applyEnergyPathPVElectricalSourceObservations(existing []EnergyDataSource, evidence energyPathPVElectricalEvidence) []EnergyDataSource {
	snapshots := map[string]EnergyDataSource{}
	var order []string
	for _, source := range evidence.SourceSnapshots {
		if _, found := snapshots[source.ID]; !found {
			order = append(order, source.ID)
		}
		snapshots[source.ID] = source
	}
	out := make([]EnergyDataSource, 0, len(existing)+len(order))
	used := map[string]bool{}
	for _, source := range existing {
		if snapshot, owned := snapshots[source.ID]; owned {
			if !used[source.ID] {
				out = append(out, energyPathPVElectricalCopySnapshot(snapshot))
				used[source.ID] = true
			}
		} else {
			out = append(out, source)
		}
	}
	for _, id := range order {
		if !used[id] {
			out = append(out, energyPathPVElectricalCopySnapshot(snapshots[id]))
			used[id] = true
		}
	}
	return out
}
