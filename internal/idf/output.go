package idf

import (
	"fmt"
	"sort"
	"strings"
)

const defaultOutputFrequency = "Hourly"

type OutputReport struct {
	ObjectCount          int                    `json:"objectCount"`
	VariableCount        int                    `json:"variableCount"`
	MeterCount           int                    `json:"meterCount"`
	ControlCount         int                    `json:"controlCount"`
	Existing             []OutputObjectSummary  `json:"existing"`
	Recommendations      []OutputRecommendation `json:"recommendations"`
	Warnings             []Diagnostic           `json:"warnings,omitempty"`
	ReportingFrequencies []string               `json:"reportingFrequencies"`
}

type OutputObjectSummary struct {
	ObjectIndex        int                  `json:"objectIndex"`
	ObjectType         string               `json:"objectType"`
	ObjectName         string               `json:"objectName,omitempty"`
	Category           string               `json:"category"`
	Summary            string               `json:"summary"`
	KeyValue           string               `json:"keyValue,omitempty"`
	VariableName       string               `json:"variableName,omitempty"`
	ReportingFrequency string               `json:"reportingFrequency,omitempty"`
	ScheduleName       string               `json:"scheduleName,omitempty"`
	Duplicate          bool                 `json:"duplicate,omitempty"`
	PurposeTags        []string             `json:"purposeTags,omitempty"`
	Fields             []OutputFieldSummary `json:"fields"`
}

type OutputFieldSummary struct {
	Index    int      `json:"index"`
	Name     string   `json:"name"`
	Value    string   `json:"value"`
	Editable bool     `json:"editable"`
	Choices  []string `json:"choices,omitempty"`
}

type OutputRecommendation struct {
	ID          string             `json:"id"`
	Label       string             `json:"label"`
	Category    string             `json:"category"`
	Description string             `json:"description"`
	ObjectType  string             `json:"objectType"`
	Fields      []OutputFieldValue `json:"fields"`
	Exists      bool               `json:"exists"`
	Tags        []string           `json:"tags,omitempty"`
	PurposeTags []string           `json:"purposeTags,omitempty"`
}

type OutputFieldValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func AnalyzeOutput(doc Document) OutputReport {
	report := OutputReport{
		ReportingFrequencies: outputReportingFrequencies(),
	}
	signatures := map[string]int{}
	for _, obj := range doc.Objects {
		if !isOutputManagementType(obj.Type) {
			continue
		}
		summary := summarizeOutputObject(obj)
		signature := outputObjectSignature(obj.Type, outputFieldValues(obj))
		if first, ok := signatures[signature]; ok {
			summary.Duplicate = true
			report.Warnings = append(report.Warnings, Diagnostic{
				Severity:    DiagnosticWarning,
				Category:    "Output",
				Code:        "duplicate_output_request",
				Message:     fmt.Sprintf("%s duplicates output object #%d.", objectLabel(obj), first+1),
				ObjectIndex: obj.Index,
				ObjectType:  obj.Type,
				ObjectName:  objectName(obj),
			})
		} else {
			signatures[signature] = obj.Index
		}
		report.Existing = append(report.Existing, summary)
		report.ObjectCount++
		switch strings.ToLower(obj.Type) {
		case "output:variable":
			report.VariableCount++
			if strings.EqualFold(summary.ReportingFrequency, "Detailed") || strings.EqualFold(summary.ReportingFrequency, "Timestep") {
				report.Warnings = append(report.Warnings, Diagnostic{
					Severity:    DiagnosticWarning,
					Category:    "Output",
					Code:        "high_volume_output",
					Message:     fmt.Sprintf("%s uses %s frequency and can create large output files.", objectLabel(obj), summary.ReportingFrequency),
					ObjectIndex: obj.Index,
					ObjectType:  obj.Type,
					ObjectName:  objectName(obj),
					Value:       summary.ReportingFrequency,
				})
			}
		case "output:meter", "output:meter:meterfileonly", "output:meter:cumulative", "output:meter:cumulativemeterfileonly":
			report.MeterCount++
		default:
			report.ControlCount++
		}
	}
	report.Recommendations = outputRecommendations(doc, report.Existing)
	sort.SliceStable(report.Existing, func(i, j int) bool {
		if report.Existing[i].Category != report.Existing[j].Category {
			return report.Existing[i].Category < report.Existing[j].Category
		}
		return report.Existing[i].ObjectIndex < report.Existing[j].ObjectIndex
	})
	return report
}

func summarizeOutputObject(obj Object) OutputObjectSummary {
	fields := outputFieldValues(obj)
	summary := OutputObjectSummary{
		ObjectIndex:  obj.Index,
		ObjectType:   obj.Type,
		ObjectName:   objectName(obj),
		Category:     outputCategory(obj.Type),
		KeyValue:     outputFieldValue(fields, "Key Value", "Key Name"),
		VariableName: outputFieldValue(fields, "Variable Name"),
		ScheduleName: outputFieldValue(fields, "Schedule Name"),
		PurposeTags:  outputPurposeTags(obj.Type, fields),
		Fields:       outputFieldSummaries(obj),
	}
	summary.ReportingFrequency = outputFieldValue(fields, "Reporting Frequency")
	if summary.ReportingFrequency == "" && outputTypeUsesFrequency(obj.Type) {
		summary.ReportingFrequency = defaultOutputFrequency
	}
	summary.Summary = outputObjectSummaryText(summary, fields)
	return summary
}

func outputObjectSummaryText(summary OutputObjectSummary, fields []OutputFieldValue) string {
	switch strings.ToLower(summary.ObjectType) {
	case "output:variable":
		return strings.TrimSpace(fmt.Sprintf("%s / %s / %s", blankAsWildcard(summary.KeyValue), summary.VariableName, summary.ReportingFrequency))
	case "output:meter", "output:meter:meterfileonly", "output:meter:cumulative", "output:meter:cumulativemeterfileonly":
		return strings.TrimSpace(fmt.Sprintf("%s / %s", summary.KeyValue, summary.ReportingFrequency))
	default:
		values := make([]string, 0, len(fields))
		for _, field := range fields {
			if strings.TrimSpace(field.Value) != "" {
				values = append(values, strings.TrimSpace(field.Value))
			}
		}
		return strings.Join(values, ", ")
	}
}

func outputFieldSummaries(obj Object) []OutputFieldSummary {
	out := make([]OutputFieldSummary, 0, len(obj.Fields))
	for index, field := range obj.Fields {
		name := outputFieldName(obj, index)
		choices := outputFieldChoices(obj.Type, index)
		out = append(out, OutputFieldSummary{
			Index:    index,
			Name:     name,
			Value:    strings.TrimSpace(field.Value),
			Editable: isOutputManagementType(obj.Type),
			Choices:  choices,
		})
	}
	return out
}

func outputFieldValues(obj Object) []OutputFieldValue {
	values := make([]OutputFieldValue, 0, len(obj.Fields))
	for index, field := range obj.Fields {
		values = append(values, OutputFieldValue{
			Name:  outputFieldName(obj, index),
			Value: strings.TrimSpace(field.Value),
		})
	}
	return values
}

func outputFieldName(obj Object, fieldIndex int) string {
	if name := catalogFieldName(obj, fieldIndex); name != "" {
		return name
	}
	return fmt.Sprintf("Field %d", fieldIndex+1)
}

func outputFieldChoices(objectType string, fieldIndex int) []string {
	if spec, ok := fieldSpecAt(objectType, fieldIndex); ok && len(spec.Choices) > 0 {
		return append([]string(nil), spec.Choices...)
	}
	if outputTypeUsesFrequency(objectType) && fieldIndex == 2 {
		return outputReportingFrequencies()
	}
	return nil
}

func outputFieldValue(fields []OutputFieldValue, names ...string) string {
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[normalizeFieldName(name)] = true
	}
	for _, field := range fields {
		if wanted[normalizeFieldName(field.Name)] {
			return strings.TrimSpace(field.Value)
		}
	}
	return ""
}

func outputRecommendations(doc Document, existing []OutputObjectSummary) []OutputRecommendation {
	base := []OutputRecommendation{
		outputRecommendation("variable-dictionary-regular", "Report variable dictionary", "dictionary", "Adds the report-variable dictionary so available Output:Variable names can be reviewed after a run.", "Output:VariableDictionary",
			outputFields("Key Field", "Regular"), "dictionary"),
		outputRecommendation("sqlite-simple-tabular", "SQLite results database", "files", "Requests the SQLite output database with tabular data included.", "Output:SQLite",
			outputFields("Option Type", "SimpleAndTabular", "Unit Conversion for Tabular Data", "JtoKWH"), "database", "tabular"),
		outputRecommendation("summary-all", "All summary tables", "tabular", "Requests EnergyPlus summary tables for sizing, envelope, loads, HVAC, and economics where available.", "Output:Table:SummaryReports",
			outputFields("Report 1 Name", "AllSummary"), "tabular"),
		outputRecommendation("table-style-html", "HTML table style", "tabular", "Writes tabular reports in HTML with kWh-style energy units.", "OutputControl:Table:Style",
			outputFields("Column Separator", "HTML", "Unit Conversion", "JtoKWH"), "tabular"),
		outputVariableRecommendation("zone-air-temperature", "Zone air temperature", "*", "Zone Air Temperature", defaultOutputFrequency, "zone_conditions", "Tracks zone dry-bulb air temperature for all zones."),
		outputVariableRecommendation("zone-air-relative-humidity", "Zone air relative humidity", "*", "Zone Air Relative Humidity", defaultOutputFrequency, "zone_conditions", "Tracks relative humidity for all zones."),
		outputVariableRecommendation("outdoor-drybulb", "Outdoor dry-bulb temperature", "Environment", "Site Outdoor Air Drybulb Temperature", defaultOutputFrequency, "weather", "Tracks outdoor dry-bulb temperature."),
		outputVariableRecommendation("node-temperature", "System node temperature", "*", "System Node Temperature", defaultOutputFrequency, "hvac_nodes", "Tracks temperatures for all HVAC nodes."),
		outputVariableRecommendation("node-mass-flow", "System node mass flow", "*", "System Node Mass Flow Rate", defaultOutputFrequency, "hvac_nodes", "Tracks mass flow through HVAC nodes."),
		outputMeterRecommendation("electricity-facility", "Facility electricity meter", "Electricity:Facility", defaultOutputFrequency, "meters", "Tracks whole-facility electricity use."),
		outputMeterRecommendation("naturalgas-facility", "Facility natural gas meter", "NaturalGas:Facility", defaultOutputFrequency, "meters", "Tracks whole-facility natural gas use."),
		outputMeterRecommendation("district-cooling-facility", "District cooling meter", "DistrictCooling:Facility", defaultOutputFrequency, "meters", "Tracks whole-facility district cooling use."),
		outputMeterRecommendation("district-heating-facility", "District heating meter", "DistrictHeating:Facility", defaultOutputFrequency, "meters", "Tracks whole-facility district heating use."),
		standardOutputRecommendation("standard-variable-dictionary", "Standard: variable dictionary", "standard_controls", "Writes the output-variable dictionary so unavailable variable names can be reviewed after a standard run.", "Output:VariableDictionary",
			outputFields("Key Field", "Regular"), "dictionary"),
		standardOutputRecommendation("standard-sqlite-tabular", "Standard: SQLite tabular results", "standard_controls", "Requests SQLite output with tabular reports for consistent post-processing.", "Output:SQLite",
			outputFields("Option Type", "SimpleAndTabular", "Unit Conversion for Tabular Data", "JtoKWH"), "database", "tabular"),
		standardOutputRecommendation("standard-summary-all", "Standard: all summary tables", "standard_controls", "Requests the common EnergyPlus summary tables alongside monthly CSV series.", "Output:Table:SummaryReports",
			outputFields("Report 1 Name", "AllSummary"), "tabular"),
		standardOutputRecommendation("standard-table-style-html", "Standard: HTML table style", "standard_controls", "Writes tabular reports in HTML with kWh-style energy units.", "OutputControl:Table:Style",
			outputFields("Column Separator", "HTML", "Unit Conversion", "JtoKWH"), "tabular"),
		standardOutputVariableRecommendationWithFrequency("standard-zone-mean-air-temperature", "Standard: zone mean air temperature", "*", "Zone Mean Air Temperature", standardHeatFlowFrequency, "zone_conditions", "Hourly zone air temperature for thermal camera and heat-flow overlays."),
		standardOutputVariableRecommendationWithFrequency("standard-zone-air-hb-internal-convective", "Standard: heat-flow internal gains", "*", "Zone Air Heat Balance Internal Convective Heat Gain Rate", standardHeatFlowFrequency, "zone_heat_flow", "Hourly internal convective heat gains for zone heat-flow ledger."),
		standardOutputVariableRecommendationWithFrequency("standard-zone-air-hb-surface-convection", "Standard: heat-flow surface convection", "*", "Zone Air Heat Balance Surface Convection Rate", standardHeatFlowFrequency, "zone_heat_flow", "Hourly surface convection heat transfer for zone heat-flow ledger."),
		standardOutputVariableRecommendationWithFrequency("standard-zone-air-hb-interzone-air", "Standard: heat-flow interzone air", "*", "Zone Air Heat Balance Interzone Air Transfer Rate", standardHeatFlowFrequency, "zone_heat_flow", "Hourly interzone air transfer for zone heat-flow ledger."),
		standardOutputVariableRecommendationWithFrequency("standard-zone-air-hb-outdoor-air", "Standard: heat-flow outdoor air", "*", "Zone Air Heat Balance Outdoor Air Transfer Rate", standardHeatFlowFrequency, "zone_heat_flow", "Hourly outdoor-air and infiltration transfer for zone heat-flow ledger."),
		standardOutputVariableRecommendationWithFrequency("standard-zone-air-hb-system-air", "Standard: heat-flow HVAC air", "*", "Zone Air Heat Balance System Air Transfer Rate", standardHeatFlowFrequency, "zone_heat_flow", "Hourly HVAC system air transfer for zone heat-flow ledger."),
		standardOutputVariableRecommendationWithFrequency("standard-zone-air-hb-system-convective", "Standard: heat-flow system convective", "*", "Zone Air Heat Balance System Convective Heat Gain Rate", standardHeatFlowFrequency, "zone_heat_flow", "Hourly non-air-system convective heat gains for zone heat-flow ledger."),
		standardOutputVariableRecommendationWithFrequency("standard-zone-air-hb-air-storage", "Standard: heat-flow air storage", "*", "Zone Air Heat Balance Air Energy Storage Rate", standardHeatFlowFrequency, "zone_heat_flow", "Hourly air energy storage term for zone heat-flow ledger."),
		standardOutputVariableRecommendationWithFrequency("standard-zone-air-hb-deviation", "Standard: heat-flow deviation", "*", "Zone Air Heat Balance Deviation Rate", standardHeatFlowFrequency, "zone_heat_flow", "Hourly off-balance residual for zone heat-flow ledger."),
		standardOutputMeterRecommendation("standard-meter-electricity-facility", "Standard: facility electricity", "Electricity:Facility", "facility_energy", "Monthly whole-facility electricity use."),
		standardOutputMeterRecommendation("standard-meter-naturalgas-facility", "Standard: facility natural gas", "NaturalGas:Facility", "facility_energy", "Monthly whole-facility natural gas use."),
		standardOutputMeterRecommendation("standard-meter-district-cooling-facility", "Standard: facility district cooling", "DistrictCooling:Facility", "facility_energy", "Monthly whole-facility district cooling use."),
		standardOutputMeterRecommendation("standard-meter-district-heating-facility", "Standard: facility district heating", "DistrictHeating:Facility", "facility_energy", "Monthly whole-facility district heating use."),
		standardOutputMeterRecommendation("standard-meter-water-facility", "Standard: facility water", "Water:Facility", "facility_energy", "Monthly whole-facility water use."),
		standardOutputMeterRecommendation("standard-meter-electricity-cooling", "Standard: electricity cooling", "Electricity:Cooling", "end_use_energy", "Monthly electricity used for cooling."),
		standardOutputMeterRecommendation("standard-meter-electricity-heating", "Standard: electricity heating", "Electricity:Heating", "end_use_energy", "Monthly electricity used for heating."),
		standardOutputMeterRecommendation("standard-meter-electricity-interior-lights", "Standard: electricity interior lights", "Electricity:InteriorLights", "end_use_energy", "Monthly electricity used by interior lights."),
		standardOutputMeterRecommendation("standard-meter-electricity-interior-equipment", "Standard: electricity interior equipment", "Electricity:InteriorEquipment", "end_use_energy", "Monthly electricity used by interior equipment."),
		standardOutputMeterRecommendation("standard-meter-electricity-fans", "Standard: electricity fans", "Electricity:Fans", "end_use_energy", "Monthly electricity used by fans."),
		standardOutputMeterRecommendation("standard-meter-electricity-pumps", "Standard: electricity pumps", "Electricity:Pumps", "end_use_energy", "Monthly electricity used by pumps."),
		standardOutputMeterRecommendation("standard-meter-electricity-heat-rejection", "Standard: electricity heat rejection", "Electricity:HeatRejection", "end_use_energy", "Monthly electricity used by heat rejection equipment."),
		standardOutputMeterRecommendation("standard-meter-electricity-water-systems", "Standard: electricity water systems", "Electricity:WaterSystems", "end_use_energy", "Monthly electricity used by water systems."),
		standardOutputMeterRecommendation("standard-meter-naturalgas-heating", "Standard: natural gas heating", "NaturalGas:Heating", "end_use_energy", "Monthly natural gas used for heating."),
		standardOutputMeterRecommendation("standard-meter-naturalgas-water-systems", "Standard: natural gas water systems", "NaturalGas:WaterSystems", "end_use_energy", "Monthly natural gas used by water systems."),
		standardOutputVariableRecommendation("standard-zone-lights-electricity", "Standard: zone lights electricity", "*", "Zone Lights Electricity Energy", "zone_energy", "Monthly zone-level lighting electricity where available."),
		standardOutputVariableRecommendation("standard-zone-electric-equipment", "Standard: zone electric equipment", "*", "Zone Electric Equipment Electricity Energy", "zone_energy", "Monthly zone-level electric equipment energy where available."),
		standardOutputVariableRecommendation("standard-zone-gas-equipment", "Standard: zone gas equipment", "*", "Zone Gas Equipment Gas Energy", "zone_energy", "Monthly zone-level gas equipment energy where available."),
		standardOutputVariableRecommendation("standard-zone-sensible-heating", "Standard: zone sensible heating", "*", "Zone Air System Sensible Heating Energy", "zone_energy", "Monthly zone sensible heating energy where available."),
		standardOutputVariableRecommendation("standard-zone-sensible-cooling", "Standard: zone sensible cooling", "*", "Zone Air System Sensible Cooling Energy", "zone_energy", "Monthly zone sensible cooling energy where available."),
	}
	hasNodes := false
	hasZones := false
	features := detectOutputFeatures(doc)
	for _, obj := range doc.Objects {
		if strings.EqualFold(obj.Type, "Zone") {
			hasZones = true
		}
		for index, field := range obj.Fields {
			if fieldLooksLikeNodeRef(obj, index, field) && strings.TrimSpace(field.Value) != "" {
				hasNodes = true
			}
		}
	}
	var out []OutputRecommendation
	for _, item := range base {
		if outputRecommendationHasTag(item, standardOutputTag) && !standardOutputRecommendationApplies(item, features) {
			continue
		}
		if (item.Category == "zone_conditions" || item.Category == "zone_energy") && !hasZones {
			continue
		}
		if item.Category == "hvac_nodes" && !hasNodes {
			continue
		}
		item.Exists = outputRecommendationExists(item, existing)
		out = append(out, item)
	}
	return out
}

func outputRecommendation(id, label, category, description, objectType string, fields []OutputFieldValue, tags ...string) OutputRecommendation {
	return OutputRecommendation{
		ID:          id,
		Label:       label,
		Category:    category,
		Description: description,
		ObjectType:  objectType,
		Fields:      fields,
		Tags:        tags,
		PurposeTags: outputPurposeTags(objectType, fields),
	}
}

func outputVariableRecommendation(id, label, keyValue, variableName, frequency, category, description string) OutputRecommendation {
	return outputRecommendation(id, label, category, description, "Output:Variable",
		[]OutputFieldValue{
			{Name: "Key Value", Value: keyValue},
			{Name: "Variable Name", Value: variableName},
			{Name: "Reporting Frequency", Value: frequency},
		}, "time-series")
}

func outputMeterRecommendation(id, label, keyName, frequency, category, description string) OutputRecommendation {
	return outputRecommendation(id, label, category, description, "Output:Meter",
		[]OutputFieldValue{
			{Name: "Key Name", Value: keyName},
			{Name: "Reporting Frequency", Value: frequency},
		}, "meter")
}

func standardOutputRecommendation(id, label, category, description, objectType string, fields []OutputFieldValue, tags ...string) OutputRecommendation {
	return outputRecommendation(id, label, category, description, objectType, fields, append([]string{standardOutputTag}, tags...)...)
}

func standardOutputVariableRecommendation(id, label, keyValue, variableName, category, description string) OutputRecommendation {
	return standardOutputVariableRecommendationWithFrequency(id, label, keyValue, variableName, standardOutputFrequency, category, description)
}

func standardOutputVariableRecommendationWithFrequency(id, label, keyValue, variableName, frequency, category, description string) OutputRecommendation {
	return standardOutputRecommendation(id, label, category, description, "Output:Variable",
		[]OutputFieldValue{
			{Name: "Key Value", Value: keyValue},
			{Name: "Variable Name", Value: variableName},
			{Name: "Reporting Frequency", Value: frequency},
		}, "time-series")
}

func standardOutputMeterRecommendation(id, label, keyName, category, description string) OutputRecommendation {
	return standardOutputRecommendation(id, label, category, description, "Output:Meter",
		[]OutputFieldValue{
			{Name: "Key Name", Value: keyName},
			{Name: "Reporting Frequency", Value: standardOutputFrequency},
		}, "meter")
}

func outputFields(values ...string) []OutputFieldValue {
	fields := make([]OutputFieldValue, 0, len(values)/2)
	for index := 0; index+1 < len(values); index += 2 {
		fields = append(fields, OutputFieldValue{Name: values[index], Value: values[index+1]})
	}
	return fields
}

func outputRecommendationExists(item OutputRecommendation, existing []OutputObjectSummary) bool {
	signature := outputObjectSignature(item.ObjectType, item.Fields)
	for _, obj := range existing {
		if outputObjectSignature(obj.ObjectType, summaryFieldsAsValues(obj.Fields)) == signature {
			return true
		}
	}
	return false
}

func summaryFieldsAsValues(fields []OutputFieldSummary) []OutputFieldValue {
	values := make([]OutputFieldValue, 0, len(fields))
	for _, field := range fields {
		values = append(values, OutputFieldValue{Name: field.Name, Value: field.Value})
	}
	return values
}

func outputObjectSignature(objectType string, fields []OutputFieldValue) string {
	parts := []string{normalizeFieldCatalogKey(objectType)}
	for _, field := range fields {
		value := strings.TrimSpace(field.Value)
		if outputTypeUsesFrequency(objectType) && normalizeFieldName(field.Name) == "reporting frequency" {
			value = canonicalOutputFrequency(value)
		}
		parts = append(parts, normalizeFieldName(field.Name)+"="+normalizeName(value))
	}
	return strings.Join(parts, "|")
}

func outputCategory(objectType string) string {
	switch strings.ToLower(strings.TrimSpace(objectType)) {
	case "output:variable":
		return "variables"
	case "output:meter", "output:meter:meterfileonly", "output:meter:cumulative", "output:meter:cumulativemeterfileonly":
		return "meters"
	case "output:variabledictionary":
		return "dictionary"
	case "output:sqlite", "output:json", "outputcontrol:files":
		return "files"
	case "output:table:summaryreports", "output:table:monthly", "output:table:annual", "output:table:timebins", "outputcontrol:table:style":
		return "tabular"
	case "output:diagnostics":
		return "diagnostics"
	default:
		if strings.HasPrefix(strings.ToLower(objectType), "outputcontrol:") {
			return "controls"
		}
		return "other"
	}
}

func outputPurposeTags(objectType string, fields []OutputFieldValue) []string {
	tags := map[string]bool{}
	lowerType := strings.ToLower(strings.TrimSpace(objectType))
	switch lowerType {
	case "output:sqlite":
		addOutputPurposeTags(tags, "basic_energy", "zone_heat_flow", "hvac_loop_check", "integrity_check", "comfort_check", "custom_outputs")
	case "output:variabledictionary":
		addOutputPurposeTags(tags, "custom_outputs")
	case "output:table:summaryreports":
		addOutputPurposeTags(tags, "integrity_check", "comfort_check")
	case "outputcontrol:table:style", "output:diagnostics":
		addOutputPurposeTags(tags, "integrity_check")
	case "output:meter", "output:meter:meterfileonly", "output:meter:cumulative", "output:meter:cumulativemeterfileonly":
		if outputMeterSupportsBasicEnergy(outputFieldValue(fields, "Key Name", "Key Value")) {
			addOutputPurposeTags(tags, "basic_energy")
		}
	case "output:variable":
		outputVariablePurposeTags(outputFieldValue(fields, "Variable Name"), tags)
	}
	return orderedOutputPurposeTags(tags)
}

func outputMeterSupportsBasicEnergy(name string) bool {
	switch normalizeName(name) {
	case "electricity:facility", "naturalgas:facility", "districtcooling:facility", "districtheating:facility", "water:facility",
		"electricity:cooling", "electricity:heating", "electricity:interiorlights", "electricity:interiorequipment",
		"electricity:fans", "electricity:pumps", "electricity:heatrejection", "electricity:watersystems",
		"naturalgas:heating", "naturalgas:watersystems":
		return true
	default:
		return false
	}
}

func outputVariablePurposeTags(variableName string, tags map[string]bool) {
	switch normalizeName(variableName) {
	case "zone lights electricity energy", "zone electric equipment electricity energy", "zone gas equipment gas energy",
		"zone air system sensible heating energy", "zone air system sensible cooling energy":
		addOutputPurposeTags(tags, "basic_energy")
	case "zone mean air temperature":
		addOutputPurposeTags(tags, "zone_heat_flow", "comfort_check")
	case "zone air heat balance internal convective heat gain rate",
		"zone air heat balance surface convection rate",
		"zone air heat balance interzone air transfer rate",
		"zone air heat balance outdoor air transfer rate",
		"zone air heat balance system air transfer rate",
		"zone air heat balance system convective heat gain rate",
		"zone air heat balance air energy storage rate",
		"zone air heat balance deviation rate":
		addOutputPurposeTags(tags, "zone_heat_flow")
	case "system node temperature", "system node mass flow rate", "system node setpoint temperature", "system node humidity ratio", "system node enthalpy":
		addOutputPurposeTags(tags, "hvac_loop_check")
	case "fan electricity rate", "fan electricity energy",
		"pump electricity rate", "pump electricity energy",
		"cooling coil total cooling rate", "cooling coil total cooling energy",
		"heating coil heating rate", "heating coil heating energy",
		"chiller evaporator cooling rate", "chiller evaporator cooling energy",
		"chiller electricity rate", "chiller electricity energy",
		"boiler heating rate", "boiler heating energy",
		"cooling tower heat transfer rate", "cooling tower heat transfer energy":
		addOutputPurposeTags(tags, "hvac_loop_check")
	case "zone air relative humidity",
		"zone thermostat heating setpoint temperature", "zone thermostat cooling setpoint temperature",
		"zone air system sensible heating rate", "zone air system sensible cooling rate",
		"zone thermal comfort fanger model pmv", "zone thermal comfort fanger model ppd":
		addOutputPurposeTags(tags, "comfort_check")
	}
}

func addOutputPurposeTags(tags map[string]bool, values ...string) {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			tags[value] = true
		}
	}
}

func orderedOutputPurposeTags(tags map[string]bool) []string {
	order := []string{"basic_energy", "zone_heat_flow", "hvac_loop_check", "integrity_check", "comfort_check", "custom_outputs"}
	out := make([]string, 0, len(tags))
	for _, value := range order {
		if tags[value] {
			out = append(out, value)
			delete(tags, value)
		}
	}
	extra := make([]string, 0, len(tags))
	for value := range tags {
		extra = append(extra, value)
	}
	sort.Strings(extra)
	return append(out, extra...)
}

func isOutputManagementType(objectType string) bool {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	return strings.HasPrefix(lower, "output:") || strings.HasPrefix(lower, "outputcontrol:")
}

func outputTypeUsesFrequency(objectType string) bool {
	switch strings.ToLower(strings.TrimSpace(objectType)) {
	case "output:variable", "output:meter", "output:meter:meterfileonly", "output:meter:cumulative", "output:meter:cumulativemeterfileonly":
		return true
	default:
		return false
	}
}

func validOutputFrequency(value string) bool {
	for _, frequency := range outputReportingFrequencies() {
		if strings.EqualFold(strings.TrimSpace(value), frequency) {
			return true
		}
	}
	return false
}

func canonicalOutputFrequency(value string) string {
	if value == "" {
		return defaultOutputFrequency
	}
	for _, frequency := range outputReportingFrequencies() {
		if strings.EqualFold(strings.TrimSpace(value), frequency) {
			return frequency
		}
	}
	return strings.TrimSpace(value)
}

func outputReportingFrequencies() []string {
	return []string{"Detailed", "Timestep", "Hourly", "Daily", "Monthly", "RunPeriod", "Annual"}
}

func blankAsWildcard(value string) string {
	if strings.TrimSpace(value) == "" {
		return "*"
	}
	return value
}
