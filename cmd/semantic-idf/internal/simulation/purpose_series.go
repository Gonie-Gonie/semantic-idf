package simulation

import "strings"

// The general Series preview is bounded, but the HVAC and Comfort builders
// consume these same series. Keep every requested input to those builders even
// when EnergyPlus registers it after the preview columns.
type purposeSeriesSelection map[string]map[string]bool

func newPurposeSeriesSelection(plan PurposeRunPlan) purposeSeriesSelection {
	selection := purposeSeriesSelection{}
	variables := []string{}
	if purposeIDsContain(plan.Purposes, SimulationPurposeHVACLoopCheck) {
		variables = append(variables, hvacLoopCheckNodeVariables()...)
		variables = append(variables, hvacLoopCheckComponentVariableNames()...)
	}
	comfortVariables := map[string]bool{}
	if purposeIDsContain(plan.Purposes, SimulationPurposeComfort) {
		for _, variable := range comfortCheckVariables() {
			variables = append(variables, variable)
			comfortVariables[normalizePurposeToken(variable)] = true
		}
	}
	for _, variable := range variables {
		name := normalizePurposeToken(variable)
		keys := map[string]bool{}
		for _, output := range plan.OutputObjects {
			if !strings.EqualFold(output.ObjectType, "Output:Variable") || normalizePurposeToken(output.VariableName) != name {
				continue
			}
			key := normalizePurposeToken(output.KeyValue)
			if key == "" {
				key = "*"
			}
			keys[key] = true
		}
		if len(keys) == 0 {
			if len(plan.OutputObjects) > 0 {
				// A populated plan is authoritative, including the absence of
				// component outputs outside a selected loop's equipment scope.
				continue
			}
			if comfortVariables[name] {
				scope := SimulationPurposeScope{ZoneMode: plan.ZoneMode, ZoneNames: plan.ZoneNames}
				if zones, scoped, _ := purposeZoneKeysForScope(scope); scoped {
					for _, zone := range zones {
						keys[normalizePurposeToken(zone)] = true
					}
				}
			}
			if len(keys) == 0 {
				keys["*"] = true
			}
		}
		selection[name] = keys
	}
	return selection
}

func (selection purposeSeriesSelection) matches(keyValue, variableName string) bool {
	keys := selection[normalizePurposeToken(variableName)]
	return keys["*"] || keys[normalizePurposeToken(keyValue)]
}
