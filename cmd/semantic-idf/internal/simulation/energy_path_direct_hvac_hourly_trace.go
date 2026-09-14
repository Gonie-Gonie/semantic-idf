package simulation

import "strings"

// Ownership is already established independently from the original typed
// equipment and its scoped Monthly request. Navigation needs a second proof:
// the actual-frequency request, including preserved native wildcard requests.
// Multiple different original indices stay unknown instead of picking one.
func energyPathDirectHVACReportedObjectIndex(dictionary energyExplanationDictionary, plan *PurposeRunPlan, definition energyPathDirectHVACComponentDefinition, owner string) *int {
	if plan == nil {
		return nil
	}
	type matches struct {
		present, ambiguous bool
		index              *int
	}
	exact, wildcard := matches{}, matches{}
	for _, output := range plan.OutputObjects {
		if !strings.EqualFold(strings.TrimSpace(output.ObjectType), "Output:Variable") ||
			!energyPathDirectHVACComponentNameMatches(definition, output.VariableName) ||
			!energyExplanationOutputFrequencyMatchesDictionary(output, dictionary) ||
			!purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) {
			continue
		}
		key := strings.TrimSpace(output.KeyValue)
		var target *matches
		if strings.EqualFold(key, strings.TrimSpace(dictionary.row.keyValue)) {
			target = &exact
		} else if key == "*" || key == "" {
			target = &wildcard
		} else {
			continue
		}
		target.present = true
		if zone := strings.TrimSpace(output.ScopeZoneName); zone != "" && !strings.EqualFold(zone, strings.TrimSpace(owner)) {
			target.ambiguous = true
		}
		if output.ObjectIndex == nil {
			continue
		}
		if target.index != nil && *target.index != *output.ObjectIndex {
			target.ambiguous = true
		} else {
			index := *output.ObjectIndex
			target.index = &index
		}
	}
	selected := wildcard
	if exact.present {
		selected = exact
	}
	if selected.ambiguous {
		return nil
	}
	return selected.index
}
