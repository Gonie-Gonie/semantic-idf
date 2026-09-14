package simulation

// Requested parent coverage remains independent of quantity and membership.
import (
	"math"
	"strings"
)

// A new native Cogeneration input can count only the exact requested consumed
// parent group with one complete Monthly native observation. TAB/sole-member
// quantities stay in accounting but do not fulfill a requested parent output.
func energyPathCogenerationCompletenessGroupKey(item energyExplanationSeries, expectedEnergy []string, sources []EnergyDataSource, plan *PurposeRunPlan) (string, bool) {
	if item.Kind != "energy.cogeneration_input" && item.EndUse != "cogeneration_input" {
		return "", false
	}
	if item.Kind != "energy.cogeneration_input" || item.EndUse != "cogeneration_input" || item.Level != "energy" || item.Stage != "end_use" || !strings.EqualFold(item.sourceFrequency, "Monthly") || len(item.SourceIDs) != 1 || len(item.Monthly) == 0 {
		return "", true
	}
	requests := energyPathCogenerationRequestedParentOutputs(plan)
	for _, name := range expectedEnergy {
		resource, relevant := energyPathCogenerationMeterResource(name)
		role, carrier := energyPathCogenerationResource(resource)
		if !relevant || role != energyPathCogenerationConsumed || canonicalEnergyPathCarrier(item.Carrier) != carrier {
			continue
		}
		if !strings.EqualFold(firstNonEmpty(item.SourceName, item.sourceName), name) || item.SourceName != "" && !strings.EqualFold(item.SourceName, name) || item.sourceName != "" && !strings.EqualFold(item.sourceName, name) || item.SourceKey != "" || item.sourceKeyValue != "" {
			continue
		}
		source, complete, _ := energyPathCogenerationCompleteParentObservation(name, "Monthly", sources)
		if !complete || source.ID != item.SourceIDs[0] {
			continue
		}
		for _, requested := range requests {
			if !strings.EqualFold(requested.name, name) {
				continue
			}
			allObserved := len(requested.frequencies) > 0
			for _, frequency := range requested.frequencies {
				_, observed, _ := energyPathCogenerationCompleteParentObservation(name, frequency, sources)
				allObserved = allObserved && observed
			}
			if allObserved {
				return expectedEnergyExplanationOutputGroupKey(name, "energy"), true
			}
		}
	}
	return "", true
}

// Full raw-knownness is independent of consumed-resource membership. A custom
// collision or signed observation can be observed without authorizing a flow.
// Exact frequency and all actual matching dictionary identities are considered.
func energyPathCogenerationCompleteParentObservation(name, frequency string, sources []EnergyDataSource) (EnergyDataSource, bool, []string) {
	count := 0
	var selected EnergyDataSource
	var ids []string
	for _, source := range sources {
		if !strings.EqualFold(source.Name, name) || !strings.EqualFold(source.ReportingFrequency, frequency) {
			continue
		}
		count++
		selected = source
		if source.ID != "" {
			ids = appendUniqueStrings(ids, source.ID)
		}
	}
	valid := count == 1 && (strings.EqualFold(frequency, "Monthly") || strings.EqualFold(frequency, "Hourly")) && selected.SourceType == "sql_report_data" && selected.IsMeter && selected.KeyValue == "" && selected.SourceUnit == "J" && (selected.Units == "" || selected.Units == "J") && energyPathCogenerationExactSource(selected, sources) && energyDataSourceValueKnown(selected, energySourceObservedRaw) && !math.IsNaN(selected.RawValue) && !math.IsInf(selected.RawValue, 0)
	return selected, valid, ids
}

// Existing name-level availability means all exact requested frequencies were
// fully observed. An Hourly parent cannot satisfy a missing Monthly request or
// vice versa. Source IDs of actual unknown rows remain traceable; none invented.
type energyPathCogenerationCoverageRequest struct {
	name        string
	frequencies []string
}

// Wildcards are retained as requests but are not expanded into a guessed native
// resource roster. They conservatively remain missing in this bounded helper.
func energyPathCogenerationRequestedParentOutputs(plan *PurposeRunPlan) []energyPathCogenerationCoverageRequest {
	if plan == nil {
		return nil
	}
	var requests []energyPathCogenerationCoverageRequest
	byName := map[string]int{}
	for _, output := range plan.OutputObjects {
		if !strings.EqualFold(output.ObjectType, "Output:Meter") || !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) {
			continue
		}
		fieldName := purposeOutputKeyValue(output.Fields)
		name := firstNonEmpty(output.KeyValue, fieldName)
		if _, relevant := energyPathCogenerationMeterResource(name); !relevant {
			continue
		}
		if fieldName != "" && !strings.EqualFold(fieldName, name) {
			continue
		}
		frequency := firstNonEmpty(output.ReportingFrequency, purposeOutputFrequency(output.ObjectType, output.Fields))
		if fieldFrequency := purposeFieldValue(output.Fields, "Reporting Frequency"); fieldFrequency != "" && !strings.EqualFold(canonicalPurposeFrequency(fieldFrequency), canonicalPurposeFrequency(frequency)) {
			continue
		}
		frequency = canonicalPurposeFrequency(frequency)
		key := energyPathCogenerationKey(name)
		index, present := byName[key]
		if !present {
			index = len(requests)
			byName[key] = index
			requests = append(requests, energyPathCogenerationCoverageRequest{name: name})
		}
		duplicate := false
		for _, prior := range requests[index].frequencies {
			duplicate = duplicate || strings.EqualFold(prior, frequency)
		}
		if !duplicate {
			requests[index].frequencies = append(requests[index].frequencies, frequency)
		}
	}
	return requests
}

func applyEnergyPathCogenerationAvailability(completeness *EnergyCompleteness, plan *PurposeRunPlan, result energyPathCogenerationReadResult) {
	if completeness == nil || plan == nil || !result.Enabled {
		return
	}
	requests := energyPathCogenerationRequestedParentOutputs(plan)
	if len(requests) == 0 {
		return
	}
	entries := append([]EnergySourceAvailabilityEntry(nil), completeness.SourceAvailability...)
	for _, requested := range requests {
		found := len(requested.frequencies) > 0
		var ids []string
		for _, frequency := range requested.frequencies {
			_, complete, actualIDs := energyPathCogenerationCompleteParentObservation(requested.name, frequency, result.SourceSnapshots)
			found = found && complete
			ids = appendUniqueStrings(ids, actualIDs...)
		}
		status := "missing"
		if found {
			status = "found"
		}
		updated := false
		for index := range entries {
			if strings.EqualFold(entries[index].Name, requested.name) {
				entries[index].Status = status
				entries[index].SourceIDs = append([]string(nil), ids...)
				updated = true
			}
		}
		if !updated {
			entries = append(entries, EnergySourceAvailabilityEntry{Name: requested.name, Level: "energy", Status: status, SourceIDs: ids})
		}
	}
	completeness.SourceAvailability = entries
	completeness.MissingCategories = missingEnergySourceCategories(entries)
}
