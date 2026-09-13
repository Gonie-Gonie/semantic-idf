package simulation

import (
	"database/sql"
	"strings"
)

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

func newHVACPlotSeriesSelection(plan PurposeRunPlan) purposeSeriesSelection {
	if !purposeIDsContain(plan.Purposes, SimulationPurposeHVACLoopCheck) {
		return nil
	}
	plan.Purposes = []SimulationPurposeID{SimulationPurposeHVACLoopCheck}
	return newPurposeSeriesSelection(plan)
}

// SQL has explicit environment metadata; CSV does not. When weather-run rows
// exist, keep HVAC plots on those actual hourly observations, excluding sizing
// days and warmup. Legacy schemas without that metadata retain their own axis.
func hvacWeatherHourlyFrames(db *sql.DB) map[int64]bool {
	var environments int
	if err := db.QueryRow(`SELECT COUNT(*) FROM EnvironmentPeriods WHERE EnvironmentType=3`).Scan(&environments); err != nil || environments == 0 {
		return nil
	}
	rows, err := db.Query(`SELECT t.TimeIndex FROM "Time" t
JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex
WHERE e.EnvironmentType=3 AND t.IntervalType=1 AND (t.WarmupFlag=0 OR t.WarmupFlag IS NULL)`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	frames := map[int64]bool{}
	for rows.Next() {
		var index int64
		if err := rows.Scan(&index); err != nil {
			return nil
		}
		frames[index] = true
	}
	if rows.Err() != nil {
		return nil
	}
	return frames
}
