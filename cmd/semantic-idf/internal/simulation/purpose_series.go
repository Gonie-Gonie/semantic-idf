package simulation

import (
	"database/sql"
	"strings"
)

// The general Series preview is bounded, but the HVAC and Comfort builders
// consume these same series. Keep every requested input to those builders even
// when EnergyPlus registers it after the preview columns.
type purposeSeriesSelection map[string]map[string]bool

func simulationEnvironmentPlotVariables() []string {
	return []string{
		"Site Outdoor Air Drybulb Temperature",
		"Site Outdoor Air Wetbulb Temperature",
		"Site Outdoor Air Relative Humidity",
		"Site Outdoor Air Barometric Pressure",
		"Site Direct Solar Radiation Rate per Area",
		"Site Diffuse Solar Radiation Rate per Area",
		"Site Wind Speed",
		"Site Wind Direction",
	}
}

func environmentPlotPurpose(purpose SimulationPurposeID) bool {
	return purpose == SimulationPurposeBasicEnergy || purpose == SimulationPurposeZoneHeatFlow || purpose == SimulationPurposeComfort
}

func purposeHasEnvironmentPlots(purposes []SimulationPurposeID) bool {
	for _, purpose := range purposes {
		if environmentPlotPurpose(purpose) {
			return true
		}
	}
	return false
}

func newPurposeSeriesSelection(plan PurposeRunPlan) purposeSeriesSelection {
	selection := purposeSeriesSelection{}
	variables := []string{}
	environmentVariables := map[string]bool{}
	if purposeHasEnvironmentPlots(plan.Purposes) {
		for _, variable := range simulationEnvironmentPlotVariables() {
			variables = append(variables, variable)
			environmentVariables[normalizePurposeToken(variable)] = true
		}
	}
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
			if comfortVariables[name] && !comfortPeopleVariable(name) && !comfortBuildingVariable(name) {
				scope := SimulationPurposeScope{ZoneMode: plan.ZoneMode, ZoneNames: plan.ZoneNames}
				if zones, scoped, _ := purposeZoneKeysForScope(scope); scoped {
					for _, zone := range zones {
						keys[normalizePurposeToken(zone)] = true
					}
				}
			}
			if environmentVariables[name] {
				keys[normalizePurposeToken("Environment")] = true
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

func newPurposePlotSeriesSelection(plan PurposeRunPlan) purposeSeriesSelection {
	if !purposeIDsContain(plan.Purposes, SimulationPurposeHVACLoopCheck) && !purposeHasEnvironmentPlots(plan.Purposes) {
		return nil
	}
	return newPurposeSeriesSelection(plan)
}

// Control diagnostics are a separate, source-specific cadence. The explicit
// HVAC output plan is authoritative; an unrelated or legacy preview must not
// retain every Timestep variable just because it happens to be in the file.
func newPurposeTimestepSeriesSelection(plan PurposeRunPlan) purposeSeriesSelection {
	return newPurposeControlSeriesSelection(plan, purposeTimestepFrequency)
}

func newPurposeDetailedSeriesSelection(plan PurposeRunPlan) purposeSeriesSelection {
	return newPurposeControlSeriesSelection(plan, purposeDetailedFrequency)
}

func newPurposeControlSeriesSelection(plan PurposeRunPlan, frequencyMatches func(string) bool) purposeSeriesSelection {
	selection := purposeSeriesSelection{}
	if !purposeIDsContain(plan.Purposes, SimulationPurposeHVACLoopCheck) {
		return selection
	}
	for _, output := range plan.OutputObjects {
		if !strings.EqualFold(output.ObjectType, "Output:Variable") ||
			!purposeIDsContain(output.PurposeIDs, SimulationPurposeHVACLoopCheck) || !frequencyMatches(output.ReportingFrequency) {
			continue
		}
		name, key := normalizePurposeToken(output.VariableName), normalizePurposeToken(output.KeyValue)
		if name == "" {
			continue
		}
		if key == "" {
			key = "*"
		}
		if selection[name] == nil {
			selection[name] = map[string]bool{}
		}
		selection[name][key] = true
	}
	return selection
}

func purposeTimestepFrequency(frequency string) bool {
	return strings.EqualFold(strings.TrimSpace(frequency), "Timestep") || strings.EqualFold(strings.TrimSpace(frequency), "Zone Timestep")
}

func purposeDetailedFrequency(frequency string) bool {
	return strings.EqualFold(strings.TrimSpace(frequency), "Detailed") || strings.EqualFold(strings.TrimSpace(frequency), "HVAC System Timestep")
}

func purposeControlFrequency(frequency string) string {
	if purposeTimestepFrequency(frequency) {
		return "timestep"
	}
	if purposeDetailedFrequency(frequency) {
		return "detailed"
	}
	return strings.ToLower(strings.TrimSpace(frequency))
}

func simulationPointAxisEqual(a, b []SimulationPoint) bool {
	if len(a) != len(b) {
		return false
	}
	for i, point := range a {
		if point.X != b[i].X || point.Label != b[i].Label {
			return false
		}
	}
	return true
}

func purposeSeriesFrequency(series SimulationSeries) string {
	if frequency := strings.TrimSpace(series.ReportingFrequency); frequency != "" {
		return frequency
	}
	if index := strings.LastIndex(series.Column, "("); index >= 0 && strings.HasSuffix(series.Column, ")") {
		return strings.TrimSpace(strings.TrimSuffix(series.Column[index+1:], ")"))
	}
	return ""
}

// SQL has explicit environment metadata; CSV does not. When weather-run rows
// exist, keep purpose plots on their reported cadence, excluding sizing
// days and warmup. Legacy schemas without that metadata retain their own axis.
func purposeWeatherPlotFrames(db *sql.DB, frequency string) map[int64]bool {
	interval := "t.IntervalType=1"
	if purposeTimestepFrequency(frequency) || purposeDetailedFrequency(frequency) {
		// Native zone-cadence output dictionaries may use either Zone or HVAC
		// Time records. Dictionary cadence is selected separately; retain each
		// native system record for explicitly requested Detailed diagnostics.
		interval = "t.IntervalType IN (-1,0)"
	} else if !strings.EqualFold(strings.TrimSpace(frequency), "Hourly") {
		return nil
	}
	var environments int
	if err := db.QueryRow(`SELECT COUNT(*) FROM EnvironmentPeriods WHERE EnvironmentType=3`).Scan(&environments); err != nil {
		return nil
	}
	environment := "e.EnvironmentType=3"
	if environments == 0 {
		if !purposeTimestepFrequency(frequency) && !purposeDetailedFrequency(frequency) {
			return nil
		}
		// A design/sizing-only run still has a real native axis. Keep its
		// observed non-warmup control steps without inventing a weather period.
		// Joining the native table also leaves an empty table explicitly empty.
		environment = "1=1"
	}
	rows, err := db.Query(`SELECT t.TimeIndex FROM "Time" t
JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex
WHERE ` + environment + ` AND ` + interval + ` AND (t.WarmupFlag=0 OR t.WarmupFlag IS NULL)`)
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
