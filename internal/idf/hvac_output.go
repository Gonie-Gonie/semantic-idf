package idf

import "strings"

const defaultHVACOutputFrequency = "Hourly"

func HVACNodeOutputVariables() []HVACNodeOutputVariable {
	return []HVACNodeOutputVariable{
		{VariableName: "System Node Temperature", Units: "C", ReportType: "Average", Category: "core", Description: "Current dry-bulb temperature for air nodes, or fluid temperature for plant nodes."},
		{VariableName: "System Node Mass Flow Rate", Units: "kg/s", ReportType: "Average", Category: "core", Description: "Current mass flow through the node."},
		{VariableName: "System Node Standard Density Volume Flow Rate", Units: "m3/s", ReportType: "Average", Category: "core", Description: "Volume flow calculated with standardized density."},
		{VariableName: "System Node Enthalpy", Units: "J/kg", ReportType: "Average", Category: "core", Description: "Current specific enthalpy at the node."},
		{VariableName: "System Node Setpoint Temperature", Units: "C", ReportType: "Average", Category: "setpoint", Description: "Single temperature setpoint sensed by controllers or equipment."},
		{VariableName: "System Node Setpoint High Temperature", Units: "C", ReportType: "Average", Category: "setpoint", Description: "Upper temperature setpoint for dual-setpoint nodes."},
		{VariableName: "System Node Setpoint Low Temperature", Units: "C", ReportType: "Average", Category: "setpoint", Description: "Lower temperature setpoint for dual-setpoint nodes."},
		{VariableName: "System Node Humidity Ratio", Units: "kgWater/kgDryAir", ReportType: "Average", Category: "air", AppliesTo: "Air nodes", Description: "Current humidity ratio. Not applicable for liquid nodes."},
		{VariableName: "System Node Relative Humidity", Units: "%", ReportType: "Average", Category: "air", AppliesTo: "Air nodes", Description: "Relative humidity calculated from node temperature and humidity ratio."},
		{VariableName: "System Node Wetbulb Temperature", Units: "C", ReportType: "Average", Category: "air", AppliesTo: "Air nodes", Description: "Current wet-bulb temperature. Not applicable for liquid nodes."},
		{VariableName: "System Node Dewpoint Temperature", Units: "C", ReportType: "Average", Category: "air", AppliesTo: "Air nodes", Description: "Current dewpoint temperature. Not applicable for liquid nodes."},
		{VariableName: "System Node Current Density Volume Flow Rate", Units: "m3/s", ReportType: "Average", Category: "air", AppliesTo: "Air nodes", Description: "Air volume flow calculated with current moist-air density."},
		{VariableName: "System Node Current Density", Units: "kg/m3", ReportType: "Average", Category: "air", AppliesTo: "Air nodes", Description: "Current air density used for current-density volume flow."},
		{VariableName: "System Node Pressure", Units: "Pa", ReportType: "Average", Category: "state", Description: "Current pressure at the node."},
		{VariableName: "System Node Quality", Units: "", ReportType: "Average", Category: "state", Description: "Current vapor fraction, primarily for steam nodes."},
		{VariableName: "System Node Height", Units: "m", ReportType: "Average", Category: "state", AppliesTo: "Outdoor air nodes", Description: "Current node height, only applicable to outdoor air nodes."},
		{VariableName: "System Node Specific Heat", Units: "J/kg-K", ReportType: "Average", Category: "state", Description: "Current specific heat capacity of the fluid. Not available for steam nodes."},
		{VariableName: "System Node Setpoint Humidity Ratio", Units: "kgWater/kgDryAir", ReportType: "Average", Category: "setpoint", AppliesTo: "Air nodes", Description: "Single humidity-ratio setpoint."},
		{VariableName: "System Node Setpoint Minimum Humidity Ratio", Units: "kgWater/kgDryAir", ReportType: "Average", Category: "setpoint", AppliesTo: "Air nodes", Description: "Minimum desired humidity-ratio setpoint."},
		{VariableName: "System Node Setpoint Maximum Humidity Ratio", Units: "kgWater/kgDryAir", ReportType: "Average", Category: "setpoint", AppliesTo: "Air nodes", Description: "Maximum desired humidity-ratio setpoint."},
		{VariableName: "System Node Last Timestep Temperature", Units: "C", ReportType: "Average", Category: "history", Description: "Temperature at the previous timestep."},
		{VariableName: "System Node Last Timestep Enthalpy", Units: "J/kg", ReportType: "Average", Category: "history", Description: "Enthalpy at the previous timestep."},
		{VariableName: "System Node Minimum Temperature", Units: "C", ReportType: "Average", Category: "advanced", Description: "Advanced debugging variable for loop or branch limits.", Advanced: true},
		{VariableName: "System Node Maximum Temperature", Units: "C", ReportType: "Average", Category: "advanced", Description: "Advanced debugging variable for loop or branch limits.", Advanced: true},
		{VariableName: "System Node Minimum Limit Mass Flow Rate", Units: "kg/s", ReportType: "Average", Category: "advanced", Description: "Advanced debugging variable for component hardware flow limits.", Advanced: true},
		{VariableName: "System Node Maximum Limit Mass Flow Rate", Units: "kg/s", ReportType: "Average", Category: "advanced", Description: "Advanced debugging variable for component hardware flow limits.", Advanced: true},
		{VariableName: "System Node Minimum Available Mass Flow Rate", Units: "kg/s", ReportType: "Average", Category: "advanced", Description: "Advanced debugging variable for current loop or branch flow availability.", Advanced: true},
		{VariableName: "System Node Maximum Available Mass Flow Rate", Units: "kg/s", ReportType: "Average", Category: "advanced", Description: "Advanced debugging variable for current loop or branch flow availability.", Advanced: true},
		{VariableName: "System Node Setpoint Mass Flow Rate", Units: "kg/s", ReportType: "Average", Category: "advanced", Description: "Advanced debugging variable for mass-flow setpoint.", Advanced: true},
		{VariableName: "System Node Requested Mass Flow Rate", Units: "kg/s", ReportType: "Average", Category: "advanced", Description: "Advanced plant diagnostics variable showing requested flow.", Advanced: true},
		{VariableName: "System Node CO2 Concentration", Units: "ppm", ReportType: "Average", Category: "contaminant", AppliesTo: "Models with ZoneAirContaminantBalance CO2", Description: "Carbon dioxide concentration when CO2 simulation is enabled."},
		{VariableName: "System Node Generic Air Contaminant Concentration", Units: "ppm", ReportType: "Average", Category: "contaminant", AppliesTo: "Models with generic contaminant simulation", Description: "Generic contaminant concentration when generic contaminant simulation is enabled."},
	}
}

func hvacNodeOutputMonitors(doc Document) []HVACNodeOutputMonitor {
	var monitors []HVACNodeOutputMonitor
	validVariables := hvacNodeOutputVariableSet()
	for _, obj := range doc.Objects {
		if !strings.EqualFold(obj.Type, "Output:Variable") {
			continue
		}
		keyValue := hvacFieldValue(obj, 0)
		variableName := hvacFieldValue(obj, 1)
		if !validVariables[normalizeFieldName(variableName)] {
			continue
		}
		frequency := hvacFieldValue(obj, 2)
		if frequency == "" {
			frequency = defaultHVACOutputFrequency
		}
		monitors = append(monitors, HVACNodeOutputMonitor{
			KeyValue:           keyValue,
			VariableName:       variableName,
			ReportingFrequency: frequency,
			ScheduleName:       hvacFieldValue(obj, 3),
			ObjectIndex:        obj.Index,
			Wildcard:           keyValue == "" || keyValue == "*",
		})
	}
	return monitors
}

func hvacNodeOutputVariableSet() map[string]bool {
	values := map[string]bool{}
	for _, variable := range HVACNodeOutputVariables() {
		values[normalizeFieldName(variable.VariableName)] = true
	}
	return values
}

func validHVACNodeOutputVariable(variableName string) bool {
	return hvacNodeOutputVariableSet()[normalizeFieldName(variableName)]
}

func hvacNodeOutputVariableByName(variableName string) (HVACNodeOutputVariable, bool) {
	wanted := normalizeFieldName(variableName)
	for _, variable := range HVACNodeOutputVariables() {
		if normalizeFieldName(variable.VariableName) == wanted {
			return variable, true
		}
	}
	return HVACNodeOutputVariable{}, false
}

func validHVACOutputFrequency(value string) bool {
	switch normalizeFieldName(value) {
	case "detailed", "timestep", "hourly", "daily", "monthly", "runperiod", "annual":
		return true
	default:
		return false
	}
}

func canonicalHVACOutputFrequency(value string) string {
	switch normalizeFieldName(value) {
	case "detailed":
		return "Detailed"
	case "timestep":
		return "Timestep"
	case "daily":
		return "Daily"
	case "monthly":
		return "Monthly"
	case "runperiod":
		return "RunPeriod"
	case "annual":
		return "Annual"
	default:
		return defaultHVACOutputFrequency
	}
}
