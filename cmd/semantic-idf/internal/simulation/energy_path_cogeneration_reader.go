package simulation

// Native reader: graph-free, original-opt-in Cogeneration ingestion. Observed
// quantity is independent of predefined end-use membership. Only positively
// proved consumed inputs yield series; every other native resource is context.
import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

type energyPathCogenerationReadResult struct {
	Enabled               bool
	Sources               []EnergyDataSource        // Full merged roster, not append-only additions.
	SourceSnapshots       []EnergyDataSource        // Unrounded native observation truth.
	Series                []energyExplanationSeries // Monthly consumed or independent Annual-only TAB.
	HourlyLabels          []string
	Warnings              []EnergyWarning
	MonthlyParentPresence map[string]bool // Any M/H parent blocks missing-month fallback.
}

type energyPathCogenerationNativeParent struct {
	Resource, Carrier, Role string
	Dictionary              energyPathPVElectricalNativeDictionary
	Observation             energyPathPVElectricalObservation
	Source                  EnergyDataSource
	MembershipValid         bool
}

func energyPathCogenerationWarn(result *energyPathCogenerationReadResult, code, message string) {
	result.Warnings = appendEnergyDriverWarning(result.Warnings, EnergyWarning{Severity: "warning", Code: code, Message: message})
}

// Query actual dictionary metadata, including invalid identities. No name-only
// IsMeter inference, annual scalar borrowing, native-zero invention or Hourly
// budget fallback. The shared C reader owns calendar/NULL/duplicate-cell math.
func readEnergyPathCogenerationInputs(db *sql.DB, sourceFile string, plan *PurposeRunPlan, inventory energyPathCogenerationInventory, pv energyPathPVElectricalEvidence, existing []EnergyDataSource) (energyPathCogenerationReadResult, error) {
	result := energyPathCogenerationReadResult{Sources: append([]EnergyDataSource(nil), existing...)}
	if !inventory.HasOriginal || !energyExplanationPlanUsesEnergyPath(plan) {
		return result, nil
	}
	result.Enabled = true
	result.MonthlyParentPresence = map[string]bool{}
	dictionaries, err := energyPathCogenerationReadDictionaries(db)
	if err != nil {
		for _, source := range existing {
			if _, yes := energyPathCogenerationMeterResource(source.Name); yes {
				result.SourceSnapshots = append(result.SourceSnapshots, energyPathPVElectricalUnknownSource(source))
			}
		}
		energyPathCogenerationWarn(&result, "cogeneration_native_schema_unavailable", err.Error())
		result.Sources = applyEnergyPathCogenerationSourceObservations(result.Sources, result)
		return result, err
	}
	var monthlyAxis map[int64]int
	var hourly *energySourceHourlyCollector
	if len(dictionaries) > 0 {
		monthlyAxis = energyPathHVACConsumptionMonthlyAxis(db)
		hourly = newEnergySourceHourlyCollector(db)
	}
	identities := map[string]int{}
	for _, d := range dictionaries {
		identities[energyPathPVElectricalNativeIdentity(d.Name.String, d.Key.String, d.Frequency.String)]++
	}
	parents := []energyPathCogenerationNativeParent{}
	for _, d := range dictionaries {
		resource, _ := energyPathCogenerationMeterResource(d.Name.String)
		role, carrier := energyPathCogenerationResource(resource)
		result.MonthlyParentPresence[energyPathCogenerationKey(resource)] = true
		definition := energyPathPVElectricalDefinition{ID: "cogeneration.native_resource", Name: d.Name.String, Role: "cogeneration_resource_context", SignPolicy: "signed", IsMeter: true}
		// Measurement validity is not predefined-meter membership. A custom-name
		// collision can still report a known signed J quantity; it cannot grant
		// positive Facility-consumption authority. A signed-negative consumed
		// parent stays context: summing only its positive cells would invent a
		// different annual quantity. Its actual signed scalar remains visible.
		intent := energyPathPVElectricalIntent{Target: energyPathPVElectricalTarget{Definition: definition, IdentityValid: true}, Frequency: d.Frequency.String}
		observation := energyPathPVElectricalObservation{Intent: intent, Dictionaries: []energyPathPVElectricalNativeDictionary{d}, Status: "invalid_native_dictionary"}
		source := energyPathPVElectricalSource(d)
		source.ObjectIndex = energyPathPVElectricalOutputRequestIndex(intent, plan, pv.Inventory.OriginalOutputs)
		source.Explanation = "Native Cogeneration resource observation from " + sourceFile + ". Raw measurement does not prove predefined consumed-input membership or a Zone allocation."
		observation.SourceIDs = []string{source.ID}
		identity := energyPathPVElectricalNativeIdentity(d.Name.String, d.Key.String, d.Frequency.String)
		if identities[identity] == 1 && energyPathPVElectricalDictionaryValid(d, intent) {
			var readErr error
			source, readErr = energyPathPVElectricalReadObservation(db, d, observation, monthlyAxis, hourly, source, &observation)
			if readErr != nil {
				energyPathCogenerationWarn(&result, "cogeneration_native_rows_unavailable", fmt.Sprintf("%s: %v", source.ID, readErr))
			}
		} else if identities[identity] > 1 {
			observation.Status = "duplicate_native_dictionary"
		}
		parents = append(parents, energyPathCogenerationNativeParent{Resource: resource, Carrier: carrier, Role: role, Dictionary: d, Observation: observation, Source: source})
		result.SourceSnapshots = append(result.SourceSnapshots, source)
		if source.HourlyEnergy != nil && hourly != nil {
			result.HourlyLabels = append([]string(nil), hourly.labels...)
		}
	}
	// A previous generic source whose actual M/H dictionary cannot be found
	// remains unknown context; it must not survive as a guessed positive budget.
	actual := map[string]bool{}
	for _, source := range result.SourceSnapshots {
		actual[source.ID] = true
	}
	for _, source := range existing {
		if _, yes := energyPathCogenerationMeterResource(source.Name); yes && !actual[source.ID] {
			result.SourceSnapshots = append(result.SourceSnapshots, energyPathPVElectricalUnknownSource(source))
		}
	}
	for i := range parents {
		p := &parents[i]
		item := canonicalEnergyExplanationSeries(energyExplanationSeries{Level: "energy", Kind: "energy.generators", EndUse: "generators", Unit: "kWh", SourceIDs: []string{p.Source.ID}, sourceName: p.Source.Name, sourceFrequency: p.Source.ReportingFrequency})
		_, boundary := qualifyEnergyPathCogenerationMeter(item, p.Source, result.SourceSnapshots, inventory)
		p.MembershipValid = boundary != nil && boundary.Role == energyPathCogenerationConsumed && !boundary.NonFlow && !p.Observation.HasNegativeValue && p.Observation.Status != "invalid_native_dictionary" && p.Observation.Status != "duplicate_native_dictionary"
		if p.Role == energyPathCogenerationConsumed && p.Observation.HasNegativeValue {
			energyPathCogenerationWarn(&result, "cogeneration_native_negative_consumption_context", "A consumed-resource parent contains signed-negative native values. Its actual source is retained; no positive-only annual consumption is synthesized.")
		}
	}
	monthlyAuthorities := map[string]bool{}
	for _, p := range parents {
		if p.Dictionary.Frequency.String != "Monthly" || !p.MembershipValid {
			continue
		}
		values := energyPathCogenerationNonnegativeMonths(p.Observation.MonthlyValues)
		if len(values) == 0 {
			continue
		}
		item, ok := energyPathCogenerationConsumedSeries(p.Source, p.Carrier, values, false)
		if !ok {
			energyPathCogenerationWarn(&result, "cogeneration_native_sum_invalid", "Finite native Monthly values cannot form a finite annual consumed total.")
			continue
		}
		result.Series = append(result.Series, item)
		monthlyAuthorities[energyPathCogenerationKey(p.Resource)] = true
	}
	// Ancillary observations are read once by C's native owner/SQL path. They
	// remain protected snapshots whether selected, overlapping, or unresolved.
	memberObservations := []energyPathPVElectricalObservation{}
	for _, observation := range pv.Observations {
		if !strings.EqualFold(observation.Intent.Target.Definition.Name, "Inverter Ancillary AC Electricity Energy") {
			continue
		}
		for _, id := range observation.SourceIDs {
			for _, source := range pv.SourceSnapshots {
				if source.ID == id {
					result.SourceSnapshots = append(result.SourceSnapshots, energyPathPVElectricalCopySnapshot(source))
				}
			}
		}
		if observation.Intent.Frequency == "Monthly" {
			memberObservations = append(memberObservations, observation)
		}
	}
	if !result.MonthlyParentPresence["electricity"] && len(memberObservations) == 1 {
		observation := memberObservations[0]
		if len(observation.SourceIDs) == 1 && len(observation.Dictionaries) == 1 && energyPathPVElectricalDictionaryValid(observation.Dictionaries[0], observation.Intent) {
			for _, source := range pv.SourceSnapshots {
				if source.ID != observation.SourceIDs[0] || !energyPathCogenerationSoleInverterSource(source, pv.SourceSnapshots, inventory) {
					continue
				}
				values := energyPathCogenerationNonnegativeMonths(observation.MonthlyValues)
				if len(values) == 0 {
					continue
				}
				if item, ok := energyPathCogenerationConsumedSeries(source, "electricity", values, true); ok {
					result.Series = append(result.Series, item)
					monthlyAuthorities["electricity"] = true
				}
			}
		}
	}
	if !monthlyAuthorities["electricity"] {
		for _, observation := range memberObservations {
			if len(observation.SourceIDs) > 0 {
				energyPathCogenerationWarn(&result, "cogeneration_native_member_unassigned", "Observed inverter ancillary remains source context: an absent parent and complete sole-consumer proof, or valid Monthly parent cells, are required before consumption can be assigned.")
				break
			}
		}
	}
	// TAB is independent annual evidence. It cannot fill a missing/NULL native
	// parent month; any M/H parent presence suppresses its graph fallback.
	tables, tableSources, tableWarnings, tableErr := readEnergyPathCogenerationTabular(db, inventory, result.MonthlyParentPresence, monthlyAuthorities)
	result.Series = append(result.Series, tables...)
	result.SourceSnapshots = append(result.SourceSnapshots, tableSources...)
	for _, warning := range tableWarnings {
		result.Warnings = appendEnergyDriverWarning(result.Warnings, warning)
	}
	if tableErr != nil {
		energyPathCogenerationWarn(&result, "cogeneration_native_tabular_unavailable", tableErr.Error())
	}
	for _, p := range parents {
		if p.Role == energyPathCogenerationConsumed && (!p.MembershipValid || p.Observation.Status != "observed") {
			energyPathCogenerationWarn(&result, "cogeneration_native_budget_unresolved", fmt.Sprintf("%s (%s) retains its actual observation/knownness; only independently valid Monthly cells authorize consumed input. Missing months are not filled from ancillary or TAB.", p.Source.Name, p.Source.ReportingFrequency))
		}
	}
	result.Sources = applyEnergyPathCogenerationSourceObservations(result.Sources, result)
	return result, nil
}

func energyPathCogenerationReadDictionaries(db *sql.DB) ([]energyPathPVElectricalNativeDictionary, error) {
	if db == nil {
		return nil, fmt.Errorf("native Cogeneration SQL database unavailable")
	}
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,IsMeter,Name,KeyValue,Units,ReportingFrequency,Type,TimestepType,IndexGroup,ScheduleName FROM ReportDataDictionary WHERE LOWER(TRIM(Name)) LIKE 'cogeneration:%' AND LOWER(TRIM(ReportingFrequency)) IN ('monthly','hourly') ORDER BY ReportDataDictionaryIndex`)
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

func energyPathCogenerationNonnegativeMonths(input map[int]float64) map[int]float64 {
	if len(input) == 0 {
		return nil
	}
	out := map[int]float64{}
	for month, value := range input {
		if month >= 1 && month <= 12 && energyPathFinite(value) && value >= 0 {
			out[month] = value
		}
	}
	return out
}

func energyPathCogenerationConsumedSeries(source EnergyDataSource, carrier string, months map[int]float64, soleMember bool) (energyExplanationSeries, bool) {
	if len(months) == 0 {
		return energyExplanationSeries{}, false
	}
	keys := make([]int, 0, len(months))
	for month := range months {
		keys = append(keys, month)
	}
	sort.Ints(keys)
	total := 0.0
	raw, values := map[int]float64{}, map[int]float64{}
	for _, month := range keys {
		value := months[month]
		if !energyPathFinite(value) || value < 0 {
			return energyExplanationSeries{}, false
		}
		total += value
		raw[month], values[month] = value, value
	}
	if !energyPathFinite(total) {
		return energyExplanationSeries{}, false
	}
	basis, sourceClass := "measured_meter", "meter"
	if soleMember {
		basis, sourceClass = "measured_variable", "native_component"
	}
	item := canonicalEnergyExplanationSeries(energyExplanationSeries{Level: "energy", Kind: "energy.cogeneration_input", CanonicalKind: "energy.cogeneration_input", Stage: "end_use", Label: "Cogeneration input", Carrier: carrier, EndUse: "cogeneration_input", Unit: "kWh", MeterHierarchyLevel: "broad_end_use", Basis: basis, SourceClass: sourceClass, SourceIDs: []string{source.ID}, RawTotal: total, Total: total, RawMonthly: raw, Monthly: values, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", multiplierApplied: true, sourceName: source.Name, sourceKeyValue: source.KeyValue, sourceFrequency: "Monthly"})
	return item, true
}

type energyPathCogenerationTabularCell struct {
	Index    int
	Identity energyPathCogenerationTabularIdentity
	Value    sql.NullString
}

func readEnergyPathCogenerationTabular(db *sql.DB, inventory energyPathCogenerationInventory, parentPresent, monthlyAuthority map[string]bool) ([]energyExplanationSeries, []EnergyDataSource, []EnergyWarning, error) {
	has, err := sqlTableExists(db, "TabularDataWithStrings")
	if err != nil || !has {
		return nil, nil, nil, err
	}
	rows, err := db.Query(`SELECT TabularDataIndex,ReportName,ReportForString,TableName,RowName,ColumnName,Units,Value FROM TabularDataWithStrings WHERE LOWER(TRIM(ReportName))='annualbuildingutilityperformancesummary' AND LOWER(TRIM(ReportForString))='entire facility' AND LOWER(TRIM(TableName))='end uses' AND LOWER(TRIM(RowName))='generators' ORDER BY TabularDataIndex`)
	if err != nil {
		return nil, nil, nil, err
	}
	cells := []energyPathCogenerationTabularCell{}
	identities := []energyPathCogenerationTabularIdentity{}
	for rows.Next() {
		var cell energyPathCogenerationTabularCell
		var report, reportFor, table, row, column, unit sql.NullString
		if err := rows.Scan(&cell.Index, &report, &reportFor, &table, &row, &column, &unit, &cell.Value); err != nil {
			rows.Close()
			return nil, nil, nil, err
		}
		cell.Identity = energyPathCogenerationTabularIdentity{SourceID: fmt.Sprintf("sql-tabular-cogeneration-%d", cell.Index), ReportName: report.String, ReportForString: reportFor.String, TableName: table.String, RowName: row.String, ColumnName: column.String, Unit: unit.String}
		cells = append(cells, cell)
		identities = append(identities, cell.Identity)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, nil, err
	}
	var series []energyExplanationSeries
	var sources []EnergyDataSource
	var warnings []EnergyWarning
	for _, cell := range cells {
		identity := cell.Identity
		resource, energyColumn := energyExplanationTabularCarrier(identity.ColumnName)
		_, carrier := energyPathCogenerationResource(resource)
		if !energyColumn || strings.EqualFold(resource, "Water") {
			continue
		} // Water is not an energy source.
		source := EnergyDataSource{ID: identity.SourceID, SourceType: "sql_tabular", IsMeter: true, Name: identity.RowName, KeyValue: identity.RowName, Units: identity.Unit, SourceUnit: identity.Unit, ReportingFrequency: "Annual", AggregationMethod: "tabular_annual_value", TableName: identity.TableName, RowName: identity.RowName, ColumnName: identity.ColumnName, AggregationBasis: "model_total", EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", DriverRole: energyDriverSourceRoleContext, InspectorSection: energyDriverInspectorSectionContext, Explanation: "Actual native ABUPS Generators resource cell. Observation is annual only; no Monthly values or produced-electricity quantity are inferred."}
		source.ReportName, source.ReportForString = identity.ReportName, identity.ReportForString
		if energyPathCogenerationEnergyUnit(identity.Unit) {
			source.NormalizedUnit = "kWh"
		}
		known := false
		if cell.Index > 0 && cell.Value.Valid && energyPathCogenerationEnergyUnit(identity.Unit) {
			if native, valid := parseSQLTabularNumber(cell.Value.String); valid && energyPathFinite(native) {
				value, unit := convertEnergySQLValue(native, identity.Unit)
				if unit == "kWh" && energyPathFinite(value) {
					source.RawValue, source.EffectiveValue = value, value
					source.observedValuePresence = energySourceObservedRaw | energySourceObservedEffective
					known = true
				}
			}
		}
		sources = append(sources, source)
		_, boundary, handled := qualifyEnergyPathCogenerationTabular(identity, identities, inventory)
		if !handled || boundary == nil || boundary.Role != energyPathCogenerationConsumed || boundary.NonFlow || !known || source.RawValue < 0 {
			continue
		}
		key := energyPathCogenerationKey(resource)
		if parentPresent[key] || monthlyAuthority[key] {
			continue
		}
		item := canonicalEnergyExplanationSeries(energyExplanationSeries{Level: "energy", Kind: "energy.cogeneration_input", CanonicalKind: "energy.cogeneration_input", Stage: "end_use", Label: "Cogeneration input", Carrier: carrier, EndUse: "cogeneration_input", Unit: "kWh", Basis: "sql_tabular", SourceClass: "sql_tabular", MeterHierarchyLevel: "broad_end_use", SourceIDs: []string{source.ID}, RawTotal: source.RawValue, Total: source.RawValue, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", multiplierApplied: true, sourceName: "Cogeneration:" + resource, sourceFrequency: "Annual"})
		series = append(series, item)
	}
	return series, sources, warnings, nil
}

// Run AFTER the existing PV filter: its ancillary-family context filter must
// not remove a new, independently authorized sole-member consumption series.
// Root must also skip original-scoped native Generators rows BEFORE invoking
// its old TAB alias parser; this avoids constructing a bogus produced source.
func mergeEnergyPathCogenerationSeries(input []energyExplanationSeries, result energyPathCogenerationReadResult) []energyExplanationSeries {
	if !result.Enabled {
		return input
	}
	out := make([]energyExplanationSeries, 0, len(input)+len(result.Series))
	owned := map[string]bool{}
	for _, source := range result.SourceSnapshots {
		owned[source.ID] = true
	}
	for _, item := range input {
		ownedID := false
		for _, id := range item.SourceIDs {
			if owned[id] {
				ownedID = true
				break
			}
		}
		name := firstNonEmpty(item.SourceName, item.sourceName)
		_, parent := energyPathCogenerationMeterResource(name)
		if ownedID || parent || strings.EqualFold(strings.TrimSpace(name), "Inverter Ancillary AC Electricity Energy") {
			continue
		}
		out = append(out, item)
	}
	return append(out, result.Series...)
}

// The existing marker guards observations only, not consumption membership.
// Root carries these snapshots through parse -> V1 -> V2 final source merge.
func applyEnergyPathCogenerationSourceObservations(existing []EnergyDataSource, result energyPathCogenerationReadResult) []EnergyDataSource {
	return applyEnergyPathProtectedPVElectricalSources(existing, energyPathPVElectricalEvidence{SourceSnapshots: result.SourceSnapshots})
}
