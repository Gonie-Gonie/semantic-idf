package simulation

import (
	"fmt"
	"strings"
)

// Availability is observation coverage, not graph use. Signed/zero context
// remains found; dictionary-only, NULL and incomplete calendars remain missing.
func applyEnergyPathPVElectricalAvailability(result *EnergyCompleteness, evidence energyPathPVElectricalEvidence) {
	if result == nil || len(evidence.Observations) == 0 {
		return
	}
	entries := append([]EnergySourceAvailabilityEntry(nil), result.SourceAvailability...)
	byName := map[string]int{}
	for index, entry := range entries {
		byName[strings.ToLower(entry.Name)] = index
	}
	observedNames := map[string]bool{}
	allNames := map[string]bool{}
	for _, observation := range evidence.Observations {
		name := observation.Intent.Target.Definition.Name
		allNames[strings.ToLower(name)] = true
		if observation.Status == "observed" {
			observedNames[strings.ToLower(name)] = true
		}
	}
	// Unqualified legacy entries cannot claim that an unknown native source
	// was found merely because a matching dictionary or alias existed.
	for index := range entries {
		name := strings.ToLower(entries[index].Name)
		if allNames[name] {
			entries[index].Status = "missing"
			if observedNames[name] {
				entries[index].Status = "found"
			}
		}
	}
	for _, observation := range evidence.Observations {
		key := observation.Intent.Target.Key
		if key == "" {
			key = "meter"
		}
		entry := EnergySourceAvailabilityEntry{Name: fmt.Sprintf("%s [%s; %s]", observation.Intent.Target.Definition.Name, key, observation.Intent.Frequency), Level: "context", Status: "missing", SourceIDs: append([]string(nil), observation.SourceIDs...)}
		if observation.Status == "observed" {
			entry.Status = "found"
		}
		if index, exists := byName[strings.ToLower(entry.Name)]; exists {
			entries[index] = entry
		} else {
			byName[strings.ToLower(entry.Name)] = len(entries)
			entries = append(entries, entry)
		}
	}
	result.SourceAvailability = entries
	result.MissingCategories = missingEnergySourceCategories(entries)
}

func energyPathPVElectricalObservationWarning(evidence energyPathPVElectricalEvidence) []EnergyWarning {
	missing := 0
	for _, observation := range evidence.Observations {
		if observation.Status != "observed" {
			missing++
		}
	}
	if missing == 0 {
		return nil
	}
	return []EnergyWarning{{Severity: "warning", Code: "native_electrical_context_incomplete", Message: fmt.Sprintf("%d of %d native Monthly/Hourly electrical observations are missing or unverified. Their context is retained without assuming zero or assigning graph energy.", missing, len(evidence.Observations))}}
}
