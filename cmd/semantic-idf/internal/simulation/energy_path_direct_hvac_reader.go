package simulation

import "strings"

func energyPathDirectHVACCohortKey(zone, endUse, carrier string) string {
	return strings.ToLower(strings.TrimSpace(zone)) + "\x00" + endUse + "\x00" + carrier
}

// A surviving crankcase/ancillary source cannot stand in for a missing main
// consumption source. The direct-first allocator needs a complete owned
// Zone/service/carrier subtotal. Keep every original source as inspector
// evidence, but leave incomplete cohorts to the normal explicit fallback.
func energyPathCompleteDirectHVACComponentSeries(series []energyExplanationSeries, targets []energyPathDirectHVACComponentTarget) []energyExplanationSeries {
	if len(targets) == 0 {
		return series
	}
	expected := map[string]map[string]bool{}
	for _, target := range targets {
		key := energyPathDirectHVACCohortKey(target.ZoneName, target.Definition.Energy.EndUse, target.Definition.Energy.Carrier)
		if expected[key] == nil {
			expected[key] = map[string]bool{}
		}
		identity := strings.ToLower(strings.TrimSpace(target.KeyValue)) + "\x00" + target.Definition.ID
		expected[key][identity] = false
	}
	ambiguous := map[string]bool{}
	observations := map[string]bool{}
	for _, item := range series {
		if item.directComponentID == "" {
			continue
		}
		key := energyPathDirectHVACCohortKey(item.ZoneName, item.EndUse, item.Carrier)
		identity := strings.ToLower(strings.TrimSpace(item.sourceKeyValue)) + "\x00" + item.directComponentID
		if _, exists := expected[key][identity]; !exists {
			ambiguous[key] = true
			continue
		}
		observation := key + "\x00" + identity + "\x00" + normalizeEnergyOutputName(item.sourceName) + "\x00" + strings.ToLower(strings.TrimSpace(item.sourceFrequency))
		if observations[observation] {
			ambiguous[key] = true
		}
		observations[observation] = true
		expected[key][identity] = true
	}
	complete := map[string]bool{}
	for key, members := range expected {
		complete[key] = !ambiguous[key]
		for _, present := range members {
			complete[key] = complete[key] && present
		}
	}
	out := make([]energyExplanationSeries, 0, len(series))
	for _, item := range series {
		if item.directComponentID == "" || complete[energyPathDirectHVACCohortKey(item.ZoneName, item.EndUse, item.Carrier)] {
			out = append(out, item)
		}
	}
	return out
}

// A reporting key is a component, not a Zone. Resolve its definition only from
// the unique original typed target: Heating Coil Electricity Energy is reported
// by both DX and Fuel coils. The output name alone cannot choose between them,
// and an executed request cannot disambiguate two original objects with the
// same name/key reporting identity.
func energyPathDirectHVACComponentDictionaryScope(dictionary energyExplanationDictionary, plan *PurposeRunPlan, targets []energyPathDirectHVACComponentTarget) (energyPathDirectHVACComponentDefinition, string, *int, bool) {
	if !energyExplanationPlanUsesEnergyPath(plan) || dictionary.isMeter || !strings.EqualFold(strings.TrimSpace(dictionary.row.units), "J") || !strings.EqualFold(dictionary.reportingFrequency, "Monthly") {
		return energyPathDirectHVACComponentDefinition{}, "", nil, false
	}
	owner := ""
	count := 0
	var definition energyPathDirectHVACComponentDefinition
	for _, target := range targets {
		if !energyPathDirectHVACComponentNameMatches(target.Definition, dictionary.row.name) || !strings.EqualFold(strings.TrimSpace(target.KeyValue), strings.TrimSpace(dictionary.row.keyValue)) {
			continue
		}
		count++
		definition = target.Definition
		owner = strings.TrimSpace(target.ZoneName)
	}
	if count != 1 || owner == "" {
		return energyPathDirectHVACComponentDefinition{}, "", nil, false
	}
	requested := false
	var objectIndex *int
	ambiguousIndex := false
	for _, output := range plan.OutputObjects {
		if !strings.EqualFold(strings.TrimSpace(output.ObjectType), "Output:Variable") || !strings.EqualFold(strings.TrimSpace(output.KeyValue), strings.TrimSpace(dictionary.row.keyValue)) {
			continue
		}
		if !energyPathDirectHVACComponentNameMatches(definition, output.VariableName) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(output.ReportingFrequency), dictionary.reportingFrequency) {
			continue
		}
		if !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) || !strings.EqualFold(strings.TrimSpace(output.ScopeZoneName), owner) {
			return energyPathDirectHVACComponentDefinition{}, "", nil, false
		}
		requested = true
		if output.ObjectIndex != nil {
			if objectIndex != nil && *objectIndex != *output.ObjectIndex {
				ambiguousIndex = true
			} else {
				value := *output.ObjectIndex
				objectIndex = &value
			}
		}
	}
	if ambiguousIndex {
		objectIndex = nil
	}
	return definition, owner, objectIndex, requested
}
