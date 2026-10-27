package simulation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Runtime bindings identify native observations, not inferred causal reasons.
// Samples live once in ControlSeries; graph records only reference their IDs.
type HVACControlSignalBinding struct {
	ID                 string                  `json:"id"`
	ComponentIndex     int                     `json:"componentIndex"`
	LoopID             string                  `json:"loopId,omitempty"`
	Graph              string                  `json:"graph"`
	NodeID             string                  `json:"nodeId,omitempty"`
	Role               string                  `json:"role"`
	Domain             string                  `json:"domain"`
	Source             idf.HVACOperatingSource `json:"source"`
	ParameterName      string                  `json:"parameterName,omitempty"`
	KeyValue           string                  `json:"keyValue"`
	VariableName       string                  `json:"variableName"`
	ReportingFrequency string                  `json:"reportingFrequency"`
	SeriesID           string                  `json:"seriesId,omitempty"`
	Required           bool                    `json:"required"`
	Status             string                  `json:"status"`
	Message            string                  `json:"message,omitempty"`
}

type HVACControlCoverage struct {
	Status    string `json:"status"`
	Required  int    `json:"required"`
	Collected int    `json:"collected"`
	Missing   int    `json:"missing"`
	Message   string `json:"message,omitempty"`
}

// Verified native 24.2 SetupOutputVariable names. Hybrid ventilation has a
// different key and status domain and is handled separately.
var hvacRuntimeManagerVariables = map[string]string{
	"availabilitymanager:scheduled":              "Availability Manager Scheduled Control Status",
	"availabilitymanager:scheduledon":            "Availability Manager Scheduled On Control Status",
	"availabilitymanager:scheduledoff":           "Availability Manager Scheduled Off Control Status",
	"availabilitymanager:nightcycle":             "Availability Manager Night Cycle Control Status",
	"availabilitymanager:differentialthermostat": "Availability Manager Differential Thermostat Control Status",
	"availabilitymanager:hightemperatureturnoff": "Availability Manager High Temperature Turn Off Control Status",
	"availabilitymanager:hightemperatureturnon":  "Availability Manager High Temperature Turn On Control Status",
	"availabilitymanager:lowtemperatureturnoff":  "Availability Manager Low Temperature Turn Off Control Status",
	"availabilitymanager:lowtemperatureturnon":   "Availability Manager Low Temperature Turn On Control Status",
	"availabilitymanager:nightventilation":       "Availability Manager Night Ventilation Control Status",
	"availabilitymanager:optimumstart":           "Availability Manager Optimum Start Control Status",
}

func hvacRuntimeStatusVariable(variable string) bool {
	if strings.EqualFold(variable, "Air System Simulation Cycle On Off Status") || strings.EqualFold(variable, "Availability Manager Hybrid Ventilation Control Status") {
		return true
	}
	for _, statusVariable := range hvacRuntimeManagerVariables {
		if strings.EqualFold(variable, statusVariable) {
			return true
		}
	}
	return false
}

func hvacRuntimeFrequency(variable string) string {
	if hvacRuntimeStatusVariable(variable) {
		return "Detailed"
	}
	return "Timestep"
}

func hvacRuntimeComponent(profile idf.HVACOperatingProfile) idf.HVACOperatingSource {
	for _, node := range profile.ParameterControl.Nodes {
		if node.Code == "equipment_response" {
			return node.Source
		}
	}
	for _, layer := range profile.AvailabilityLayers {
		if layer.Role == "equipment" {
			return layer.Owner
		}
	}
	return idf.HVACOperatingSource{ObjectIndex: profile.ComponentIndex}
}

func hvacRuntimeProfiles(report idf.HVACReport, loop idf.HVACLoop, scope SimulationPurposeScope) []idf.HVACOperatingProfile {
	selected := purposeComponentIDSet(scope.ComponentIDs)
	allowed := map[int]bool{}
	if len(selected) > 0 {
		for _, component := range hvacLoopScopedMeasurementComponents(loop, report, selected) {
			allowed[component.ObjectIndex] = true
		}
	}
	profiles := []idf.HVACOperatingProfile{}
	for _, profile := range report.OperatingProfiles {
		if profile.LoopID == loop.ID && (len(selected) == 0 || allowed[profile.ComponentIndex]) {
			profiles = append(profiles, profile)
		}
	}
	return profiles
}

func hvacRuntimeLocalProfiles(report idf.HVACReport, scope SimulationPurposeScope) []idf.HVACOperatingProfile {
	if len(scope.AirLoopNames)+len(scope.PlantLoopNames)+len(scope.CondenserLoopNames) > 0 {
		return nil
	}
	selected := purposeComponentIDSet(scope.ComponentIDs)
	var out []idf.HVACOperatingProfile
	for _, profile := range report.OperatingProfiles {
		if profile.LoopID != "" {
			continue
		}
		source := hvacRuntimeComponent(profile)
		if len(selected) > 0 && !purposeHVACComponentSelected(idf.HVACComponent{ObjectIndex: source.ObjectIndex, ObjectType: source.ObjectType, ObjectName: source.ObjectName}, selected) {
			continue
		}
		out = append(out, profile)
	}
	return out
}

func hvacRuntimeParameter(parameters []idf.HVACControlParameter, names ...string) (idf.HVACControlParameter, bool) {
	for _, name := range names {
		for _, p := range parameters {
			if strings.EqualFold(strings.TrimSpace(p.Name), name) && strings.TrimSpace(p.Value) != "" {
				return p, true
			}
		}
	}
	return idf.HVACControlParameter{}, false
}

func hvacRuntimeNodeParameter(parameters []idf.HVACControlParameter) (idf.HVACControlParameter, bool) {
	for _, p := range parameters {
		if strings.Contains(strings.ToLower(p.Name), "node") && strings.TrimSpace(p.Value) != "" {
			return p, true
		}
	}
	return idf.HVACControlParameter{}, false
}

func hvacRuntimeZones(doc *idf.Document, name string) []string {
	if doc == nil || strings.TrimSpace(name) == "" {
		return nil
	}
	var zones, lists []idf.Object
	for _, obj := range doc.Objects {
		if len(obj.Fields) > 0 && strings.EqualFold(strings.TrimSpace(obj.Fields[0].Value), strings.TrimSpace(name)) {
			if strings.EqualFold(obj.Type, "Zone") {
				zones = append(zones, obj)
			}
			if strings.EqualFold(obj.Type, "ZoneList") {
				lists = append(lists, obj)
			}
		}
	}
	if len(zones) == 1 && len(lists) == 0 {
		return []string{strings.TrimSpace(zones[0].Fields[0].Value)}
	}
	if len(lists) != 1 || len(zones) > 0 {
		return nil
	}
	var out []string
	for _, field := range lists[0].Fields[1:] {
		for _, obj := range doc.Objects {
			if strings.EqualFold(obj.Type, "Zone") && len(obj.Fields) > 0 && strings.EqualFold(strings.TrimSpace(obj.Fields[0].Value), strings.TrimSpace(field.Value)) {
				out = appendUniquePurposeString(out, strings.TrimSpace(obj.Fields[0].Value))
			}
		}
	}
	return out
}

func hvacRuntimeSensorVariables(quantity string) []string {
	switch normalizePurposeToken(quantity) {
	case "humidityratio", "minimumhumidityratio", "maximumhumidityratio":
		return []string{"System Node Humidity Ratio"}
	case "temperatureandhumidityratio":
		return []string{"System Node Temperature", "System Node Humidity Ratio"}
	case "massflowrate", "flow":
		return []string{"System Node Mass Flow Rate"}
	case "temperature", "minimumtemperature", "maximumtemperature", "":
		return []string{"System Node Temperature"}
	default:
		return []string{""}
	}
}
func hvacRuntimeSetpointVariables(quantity string) []string {
	switch normalizePurposeToken(quantity) {
	case "maximumhumidityratio":
		return []string{"System Node Setpoint Maximum Humidity Ratio"}
	case "minimumhumidityratio":
		return []string{"System Node Setpoint Minimum Humidity Ratio"}
	case "humidityratio":
		return []string{"System Node Setpoint Humidity Ratio"}
	case "temperatureandhumidityratio":
		return []string{"System Node Setpoint Temperature", "System Node Setpoint Maximum Humidity Ratio"}
	case "massflowrate", "flow":
		return []string{"System Node Setpoint Mass Flow Rate"}
	case "minimumtemperature":
		return []string{"System Node Setpoint Low Temperature"}
	case "maximumtemperature":
		return []string{"System Node Setpoint High Temperature"}
	case "temperature", "":
		return []string{"System Node Setpoint Temperature"}
	default:
		return []string{""}
	}
}

// Exact object types and native families (selectors ending in ':') share one
// response registry, including pre-ExpandObjects wildcard acquisition. These
// reported responses are never substitutes for an ON command: a tower/HX can
// transfer heat without powered operation.
var hvacRuntimeActivityDefinitions = []struct {
	selectors []string
	variables []string
}{
	{[]string{"fan:"}, []string{"Fan Electricity Rate"}},
	{[]string{"pump:", "headeredpumps:"}, []string{"Pump Electricity Rate"}},
	{[]string{"coil:cooling:"}, []string{"Cooling Coil Total Cooling Rate"}},
	{[]string{"coil:heating:"}, []string{"Heating Coil Heating Rate"}},
	{[]string{"chiller:"}, []string{"Chiller Evaporator Cooling Rate"}},
	{[]string{"boiler:"}, []string{"Boiler Heating Rate"}},
	{[]string{"coolingtower:", "fluidcooler:", "evaporativefluidcooler:"}, []string{"Cooling Tower Heat Transfer Rate"}},
	{[]string{"heatexchanger:fluidtofluid"}, []string{"Fluid Heat Exchanger Heat Transfer Rate"}},
	{[]string{"heatpump:plantloop:eir:cooling", "heatpump:plantloop:eir:heating", "heatpump:watertowater:equationfit:cooling", "heatpump:watertowater:equationfit:heating"}, []string{"Heat Pump Load Side Heat Transfer Rate"}},
	{[]string{"waterheater:mixed", "waterheater:stratified"}, []string{"Water Heater Heating Rate"}},
	{[]string{"districtheating:water"}, []string{"District Heating Water Rate"}},
	{[]string{"districtheating:steam"}, []string{"District Heating Steam Rate"}},
	{[]string{"districtcooling"}, []string{"District Cooling Water Rate"}},
	{[]string{"zonehvac:baseboard:convective:water", "zonehvac:baseboard:convective:electric", "zonehvac:baseboard:radiantconvective:electric", "zonehvac:baseboard:radiantconvective:water", "zonehvac:baseboard:radiantconvective:steam"}, []string{"Baseboard Total Heating Rate"}},
	{[]string{"zonehvac:idealloadsairsystem"}, []string{"Zone Ideal Loads Zone Total Heating Rate", "Zone Ideal Loads Zone Total Cooling Rate"}},
	{[]string{"airloophvac:unitarysystem"}, []string{"Unitary System Total Heating Rate", "Unitary System Total Cooling Rate"}},
	{[]string{"zonehvac:packagedterminalairconditioner"}, []string{"Zone Packaged Terminal Air Conditioner Total Heating Rate", "Zone Packaged Terminal Air Conditioner Total Cooling Rate"}},
	{[]string{"zonehvac:packagedterminalheatpump"}, []string{"Zone Packaged Terminal Heat Pump Total Heating Rate", "Zone Packaged Terminal Heat Pump Total Cooling Rate"}},
	{[]string{"zonehvac:watertoairheatpump"}, []string{"Zone Water to Air Heat Pump Total Heating Rate", "Zone Water to Air Heat Pump Total Cooling Rate"}},
	{[]string{"zonehvac:fourpipefancoil"}, []string{"Fan Coil Heating Rate", "Fan Coil Total Cooling Rate"}},
	{[]string{"zonehvac:unitheater"}, []string{"Zone Unit Heater Heating Rate"}},
	{[]string{"zonehvac:unitventilator"}, []string{"Zone Unit Ventilator Heating Rate", "Zone Unit Ventilator Total Cooling Rate"}},
	{[]string{"zonehvac:outdoorairunit"}, []string{"Zone Outdoor Air Unit Total Heating Rate", "Zone Outdoor Air Unit Total Cooling Rate"}},
	{[]string{"zonehvac:windowairconditioner"}, []string{"Zone Window Air Conditioner Total Cooling Rate"}},
	{[]string{"zonehvac:lowtemperatureradiant:variableflow", "zonehvac:lowtemperatureradiant:constantflow"}, []string{"Zone Radiant HVAC Heating Rate", "Zone Radiant HVAC Cooling Rate"}},
	{[]string{"zonehvac:lowtemperatureradiant:electric"}, []string{"Zone Radiant HVAC Heating Rate"}},
	{[]string{"airconditioner:variablerefrigerantflow", "airconditioner:variablerefrigerantflow:fluidtemperaturecontrol", "airconditioner:variablerefrigerantflow:fluidtemperaturecontrol:hr"}, []string{"VRF Heat Pump Total Heating Rate", "VRF Heat Pump Total Cooling Rate"}},
	{[]string{"zonehvac:terminalunit:variablerefrigerantflow"}, []string{"Zone VRF Air Terminal Total Heating Rate", "Zone VRF Air Terminal Total Cooling Rate"}},
	{[]string{"zonehvac:dehumidifier:dx"}, []string{"Zone Dehumidifier Removed Water Mass Flow Rate"}},
	{[]string{"zonehvac:evaporativecoolerunit"}, []string{"Zone Evaporative Cooler Unit Total Cooling Rate"}},
	{[]string{"heatexchanger:airtoair:", "heatexchanger:desiccant:"}, []string{"Heat Exchanger Total Heating Rate", "Heat Exchanger Total Cooling Rate"}},
	{[]string{"humidifier:"}, []string{"Humidifier Water Volume Flow Rate"}},
	{[]string{"evaporativecooler:"}, []string{"Evaporative Cooler Electricity Rate"}},
}

func hvacRuntimeActivityVariables(objectType string) []string {
	typ := strings.ToLower(strings.TrimSpace(objectType))
	for _, definition := range hvacRuntimeActivityDefinitions {
		for _, selector := range definition.selectors {
			if typ == selector || strings.HasSuffix(selector, ":") && strings.HasPrefix(typ, selector) {
				return definition.variables
			}
		}
	}
	return nil
}

func hvacRuntimeActivityVariableNames() []string {
	var variables []string
	for _, definition := range hvacRuntimeActivityDefinitions {
		for _, variable := range definition.variables {
			variables = appendUniquePurposeString(variables, variable)
		}
	}
	return variables
}

func hvacRuntimeSignals(profile idf.HVACOperatingProfile, loop idf.HVACLoop, doc *idf.Document) []HVACControlSignalBinding {
	var out []HVACControlSignalBinding
	seen := map[string]bool{}
	add := func(graph, node, role, domain string, source idf.HVACOperatingSource, field, key, variable string) {
		identity := fmt.Sprintf("%d/%s/%s/%s/%s/%s/%s", profile.ComponentIndex, profile.LoopID, graph, node, role, normalizePurposeToken(key), normalizePurposeToken(variable))
		if seen[identity] {
			return
		}
		seen[identity] = true
		status, message := "missing", "Required runtime observation was not collected at its specified source and reporting frequency."
		if key == "" || variable == "" {
			status, message = "unsupported", "The source control requires a runtime observation without a supported output binding."
		}
		out = append(out, HVACControlSignalBinding{ID: identity, ComponentIndex: profile.ComponentIndex, LoopID: profile.LoopID, Graph: graph, NodeID: node, Role: role, Domain: domain, Source: source, ParameterName: field, KeyValue: key, VariableName: variable, ReportingFrequency: hvacRuntimeFrequency(variable), Required: true, Status: status, Message: message})
	}
	component := hvacRuntimeComponent(profile)
	graphs := []struct {
		name  string
		graph idf.HVACControlSequence
	}{{"availabilityDefinitions", profile.AvailabilityDefinitions}, {"onOff", profile.OnOff}, {"parameterControl", profile.ParameterControl}}
	for _, current := range graphs {
		byID := map[string]idf.HVACControlNode{}
		for _, node := range current.graph.Nodes {
			byID[node.ID] = node
		}
		for _, node := range current.graph.Nodes {
			typ := strings.ToLower(node.Source.ObjectType)
			if node.Kind == "schedule" && !strings.HasPrefix(typ, "schedule:day:") && !strings.HasPrefix(typ, "schedule:week:") {
				domain := "boolean_schedule"
				if current.name != "availabilityDefinitions" {
					domain = "continuous"
				}
				add(current.name, node.ID, "schedule", domain, node.Source, "", node.Source.ObjectName, "Schedule Value")
			}
			if current.name == "availabilityDefinitions" && node.Code == "availability_manager" {
				variable := hvacRuntimeManagerVariables[typ]
				key, domain := node.Source.ObjectName, "manager_status"
				if typ == "availabilitymanager:hybridventilation" {
					variable, domain = "Availability Manager Hybrid Ventilation Control Status", "hybrid_ventilation_status"
					key = ""
					if p, ok := hvacRuntimeParameter(node.Parameters, "HVAC Air Loop Name", "Control Zone Name"); ok {
						key = p.Value
					}
				}
				add(current.name, node.ID, "manager_status", domain, node.Source, "", key, variable)
			}
			if current.name == "availabilityDefinitions" || current.name == "onOff" || current.name == "parameterControl" {
				for _, input := range hvacRuntimeReasonInputs(node, loop, doc) {
					add(current.name, node.ID, input.role, "continuous", node.Source, input.parameter, input.key, input.variable)
				}
			}
			if current.name != "parameterControl" {
				continue
			}
			quantity, _ := hvacRuntimeParameter(node.Parameters, "Target quantity", "Control Variable")
			p, hasNode := hvacRuntimeNodeParameter(node.Parameters)
			if node.Code == "node_setpoint" || node.Code == "unresolved_node_setpoint" || node.Code == "reference_setpoint" {
				if node.Code == "reference_setpoint" && typ == "setpointmanager:followsystemnodetemperature" {
					// This concrete native manager reads the reference node's
					// measured temperature, not a setpoint stored on that node.
					input, variable := hvacRuntimeFollowSystemNodeInput(node, current.graph, doc)
					add(current.name, node.ID, "sensor", "continuous", node.Source, input.Name, input.Value, variable)
				} else if hasNode {
					for _, variable := range hvacRuntimeSetpointVariables(quantity.Value) {
						add(current.name, node.ID, "setpoint", "continuous", node.Source, p.Name, p.Value, variable)
					}
				}
			}
			if node.Code == "inlet_state" || node.Code == "outlet_state" {
				if hasNode {
					add(current.name, node.ID, "sensor", "continuous", node.Source, p.Name, p.Value, "System Node Temperature")
					add(current.name, node.ID, "sensor", "continuous", node.Source, p.Name, p.Value, "System Node Mass Flow Rate")
				}
			}
			if hasNode {
				for _, edge := range current.graph.Edges {
					if edge.From != node.ID || (edge.Relation != "feedback" && edge.Relation != "adds_measurement" && edge.Relation != "measures") {
						continue
					}
					q := quantity.Value
					if q == "" {
						target := byID[edge.To]
						if qp, ok := hvacRuntimeParameter(target.Parameters, "Control Variable"); ok {
							q = qp.Value
						}
					}
					for _, variable := range hvacRuntimeSensorVariables(q) {
						add(current.name, node.ID, "sensor", "continuous", node.Source, p.Name, p.Value, variable)
					}
				}
			}
			if node.Kind == "control_variable" {
				if actuator, ok := hvacRuntimeParameter(node.Parameters, "Actuator Node Name"); ok {
					add(current.name, node.ID, "actuator", "continuous", node.Source, actuator.Name, actuator.Value, "System Node Mass Flow Rate")
					add(current.name, node.ID, "actuator_request", "continuous", node.Source, actuator.Name, actuator.Value, "System Node Requested Mass Flow Rate")
					add(current.name, node.ID, "sensor", "continuous", node.Source, "Minimum available actuator flow", actuator.Value, "System Node Minimum Available Mass Flow Rate")
					add(current.name, node.ID, "sensor", "continuous", node.Source, "Maximum available actuator flow", actuator.Value, "System Node Maximum Available Mass Flow Rate")
				}
			}
			if node.Code == "zone_temperature" {
				if zone, ok := hvacRuntimeParameter(node.Parameters, "Zone or ZoneList Name"); ok {
					for _, name := range hvacRuntimeZones(doc, zone.Value) {
						add(current.name, node.ID, "sensor", "continuous", node.Source, zone.Name, name, "Zone Thermostat Air Temperature")
						add(current.name, node.ID, "demand", "continuous", node.Source, zone.Name, name, "Zone Predicted Sensible Load to Setpoint Heat Transfer Rate")
					}
				}
			}
		}
	}
	// MyLoad is the supervisor's individual assignment, not whole-loop
	// demand or measured heat output. PlantManager registers it only for
	// non-demand components; a shared component's other loop must not borrow
	// this source-specific assignment.
	if hvacRuntimeHasPlantDispatch(profile, loop, component) {
		add("onOff", "decision", "dispatch", "continuous", component, "", component.ObjectName, "Plant Component Distributed Demand Rate")
	}
	if typ := strings.ToLower(component.ObjectType); strings.HasPrefix(typ, "fan:") || strings.HasPrefix(typ, "pump:") || strings.HasPrefix(typ, "headeredpumps:") {
		for _, node := range profile.ParameterControl.Nodes {
			if node.Code == "inlet_state" && node.Source == component {
				if p, ok := hvacRuntimeNodeParameter(node.Parameters); ok {
					// Actual inlet flow is an observed operating input, not a
					// fabricated native command or an availability schedule.
					add("onOff", "demand", "sensor", "continuous", component, p.Name, p.Value, "System Node Mass Flow Rate")
				}
			}
		}
	}
	if loop.Type == "AirLoopHVAC" {
		add("loop", "", "loop_status", "manager_status", idf.HVACOperatingSource{ObjectIndex: loop.ObjectIndex, ObjectType: loop.Type, ObjectName: loop.Name}, "", loop.Name, "Air System Simulation Cycle On Off Status")
	}
	if hvacWaterLoop(loop.Type) {
		for _, variable := range []string{"Plant Supply Side Cooling Demand Rate", "Plant Supply Side Heating Demand Rate"} {
			add("loop", "demand", "demand", "continuous", idf.HVACOperatingSource{ObjectIndex: loop.ObjectIndex, ObjectType: loop.Type, ObjectName: loop.Name}, "", loop.Name, variable)
		}
	}
	for _, variable := range hvacRuntimeActivityVariables(component.ObjectType) {
		add("parameterControl", "equipment", "activity", "continuous", component, "", component.ObjectName, variable)
	}
	return out
}

func (builder *purposePlanBuilder) addHVACRuntimeOutputs() {
	report := idf.AnalyzeHVAC(builder.doc)
	unique := map[string]bool{}
	advanced := false
	add := func(binding HVACControlSignalBinding) {
		if binding.VariableName == "" || binding.KeyValue == "" {
			return
		}
		key := normalizePurposeToken(binding.KeyValue) + "\x00" + normalizePurposeToken(binding.VariableName)
		if unique[key] {
			return
		}
		unique[key] = true
		builder.addVariableWithReason(SimulationPurposeHVACLoopCheck, binding.KeyValue, binding.VariableName, hvacRuntimeFrequency(binding.VariableName), "heavy", "Native interval observation for source-backed HVAC availability and control diagnostics.", "HVAC control diagnostics")
		advanced = advanced || strings.Contains(binding.VariableName, "Requested Mass Flow") || strings.Contains(binding.VariableName, "Setpoint Mass Flow") || binding.VariableName == "Plant Component Distributed Demand Rate"
	}
	for _, loop := range report.Loops {
		if !hvacResultLoopInScope(loop, builder.request.Scope) {
			continue
		}
		for _, profile := range hvacRuntimeProfiles(report, loop, builder.request.Scope) {
			for _, binding := range hvacRuntimeSignals(profile, loop, &builder.doc) {
				add(binding)
			}
		}
	}
	for _, profile := range hvacRuntimeLocalProfiles(report, builder.request.Scope) {
		for _, binding := range hvacRuntimeSignals(profile, idf.HVACLoop{}, &builder.doc) {
			add(binding)
		}
	}
	for _, object := range builder.doc.Objects {
		if !strings.HasPrefix(strings.ToLower(object.Type), "hvactemplate:") {
			continue
		}
		variables := []string{"Schedule Value", "Air System Simulation Cycle On Off Status", "System Node Temperature", "System Node Mass Flow Rate", "System Node Setpoint Temperature", "System Node Setpoint High Temperature", "System Node Setpoint Low Temperature", "System Node Humidity Ratio", "System Node Setpoint Humidity Ratio", "System Node Setpoint Minimum Humidity Ratio", "System Node Setpoint Maximum Humidity Ratio", "System Node Requested Mass Flow Rate", "System Node Minimum Available Mass Flow Rate", "System Node Maximum Available Mass Flow Rate", "Zone Thermostat Air Temperature", "Zone Thermostat Control Type", "Zone Thermostat Heating Setpoint Temperature", "Zone Thermostat Cooling Setpoint Temperature", "Zone Predicted Sensible Load to Setpoint Heat Transfer Rate", "Zone Predicted Sensible Load to Heating Setpoint Heat Transfer Rate", "Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate", "Plant Supply Side Cooling Demand Rate", "Plant Supply Side Heating Demand Rate", "Site Outdoor Air Drybulb Temperature", "Site Outdoor Air Wetbulb Temperature", "Site Ground Temperature", "Site Surface Ground Temperature", "Site Deep Ground Temperature", "Site Simple Factor Model Ground Temperature"}
		variables = append(variables, "Plant Component Distributed Demand Rate", "VRF Heat Pump Terminal Unit Cooling Load Rate", "VRF Heat Pump Terminal Unit Heating Load Rate")
		for _, variable := range hvacRuntimeActivityVariableNames() {
			variables = appendUniquePurposeString(variables, variable)
		}
		for _, variable := range hvacRuntimeManagerVariables {
			variables = append(variables, variable)
		}
		variables = append(variables, "Site Outdoor Air Relative Humidity", "Site Outdoor Air Dewpoint Temperature", "System Node Wetbulb Temperature")
		for _, variable := range hvacLoopCheckComponentOutputs() {
			if strings.HasSuffix(variable.VariableName, " Rate") {
				variables = appendUniquePurposeString(variables, variable.VariableName)
			}
		}
		for _, variable := range variables {
			add(HVACControlSignalBinding{KeyValue: "*", VariableName: variable})
		}
		builder.warn("warning", "hvac_control_expansion_wildcard", "HVACTemplate generated control names are unknown before ExpandObjects; control diagnostics use wildcard outputs and bind only to the executed expanded model.", SimulationPurposeHVACLoopCheck, "")
		break
	}
	if advanced {
		builder.addObject(PurposeOutputObject{ObjectType: "Output:Diagnostics", Fields: []idf.OutputFieldValue{{Name: "Key 1", Value: "DisplayAdvancedReportVariables"}}, PurposeIDs: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}, Weight: "light", Description: "Enables native requested/setpoint flow and per-component plant dispatch observations for HVAC control diagnostics.", Reason: "HVAC control diagnostics"})
	}
	if len(unique) > 0 {
		builder.warn("info", "hvac_control_diagnostics", fmt.Sprintf("HVAC control diagnostics adds %d source-specific series: discrete availability actions at native Detailed intervals, other observations at Timestep cadence.", len(unique)), SimulationPurposeHVACLoopCheck, "")
	}
}

func attachHVACRuntimeControls(result *HVACLoopRunResult, profiles []idf.HVACOperatingProfile, loop idf.HVACLoop, doc *idf.Document, series []SimulationSeries) {
	result.OperatingProfiles = profiles
	coverage := HVACControlCoverage{Status: "complete"}
	pooled := map[string]bool{}
	outputKey := func(key, name, frequency string) string {
		return normalizePurposeToken(key) + "\x00" + normalizePurposeToken(name) + "\x00" + purposeControlFrequency(frequency)
	}
	byOutput := map[string][]SimulationSeries{}
	byNativeID := map[string]SimulationSeries{}
	for _, item := range series {
		byNativeID[normalizePurposeToken(item.File)+"\x00"+item.SourceID] = item
	}
	for _, item := range series {
		frequency := purposeSeriesFrequency(item)
		if !purposeTimestepFrequency(frequency) && !purposeDetailedFrequency(frequency) || item.IsMeter != nil && *item.IsMeter || len(item.Points) == 0 {
			continue
		}
		key, name := hvacSeriesIdentity(item)
		identity := outputKey(key, name, frequency)
		byOutput[identity] = append(byOutput[identity], item)
	}
	poolIdentities := map[string]string{}
	for _, profile := range profiles {
		for _, binding := range hvacRuntimeSignals(profile, loop, doc) {
			coverage.Required++
			identity := outputKey(binding.KeyValue, binding.VariableName, binding.ReportingFrequency)
			matched := byOutput[identity]
			if len(matched) == 1 {
				item := matched[0]
				if item.NativeIntervalsSourceID != "" {
					owner, exists := byNativeID[normalizePurposeToken(item.File)+"\x00"+item.NativeIntervalsSourceID]
					if exists && simulationPointAxisEqual(item.Points, owner.Points) {
						item.NativeIntervals = owner.NativeIntervals
					}
					item.NativeIntervalsSourceID = "" // Rebase references inside this loop below.
				}
				if item.SourceID == "" {
					item.SourceID = "hvac-control:" + normalizePurposeToken(item.File) + ":" + normalizePurposeToken(binding.KeyValue) + ":" + normalizePurposeToken(binding.VariableName) + ":" + purposeControlFrequency(binding.ReportingFrequency)
				}
				if previous, exists := poolIdentities[item.SourceID]; exists && previous != identity {
					binding.Status, binding.Message = "ambiguous", "A reported source ID identifies conflicting output channels; no series was chosen."
					coverage.Missing++
					result.ControlSignals = append(result.ControlSignals, binding)
					continue
				}
				poolIdentities[item.SourceID] = identity
				binding.SeriesID, binding.Status, binding.Message = item.SourceID, "collected", ""
				coverage.Collected++
				if !pooled[item.SourceID] {
					pooled[item.SourceID] = true
					result.ControlSeries = append(result.ControlSeries, item)
				}
			} else {
				if binding.Status != "unsupported" {
					binding.Status = "missing"
					if len(matched) > 1 {
						binding.Status = "ambiguous"
						binding.Message = "Multiple reported series match this exact source and cadence; no series was chosen."
					}
				}
				coverage.Missing++
			}
			result.ControlSignals = append(result.ControlSignals, binding)
		}
	}
	validateHVACRuntimeObservations(result, &coverage)
	deduplicateHVACNativeIntervals(result.ControlSeries)
	if coverage.Missing > 0 {
		coverage.Status = "error"
		coverage.Message = fmt.Sprintf("%d required HVAC control observations were not acquired with valid coverage of every reported interval.", coverage.Missing)
	}
	if len(profiles) > 0 {
		result.ControlCoverage = &coverage
		if item := hvacRuntimeControlCompleteness([]HVACLoopRunResult{*result}); item != nil {
			result.Completeness = append(result.Completeness, *item)
		}
	}
	sort.SliceStable(result.ControlSeries, func(i, j int) bool { return result.ControlSeries[i].SourceID < result.ControlSeries[j].SourceID })
}

// A SQL result can share one native axis across hundreds of channels. Keep
// that axis once in each delivered pool, with references that never escape
// the current loop's ControlSeries (even for a component-only request).
func deduplicateHVACNativeIntervals(series []SimulationSeries) {
	axes := map[string][]int{}
	for i := range series {
		item := &series[i]
		if len(item.NativeIntervals) == 0 || len(item.NativeIntervals) != len(item.Points) {
			continue
		}
		key := fmt.Sprintf("%s/%d/%d/%d", normalizePurposeToken(item.File), len(item.Points), item.Points[0].X, item.Points[len(item.Points)-1].X)
		owner := -1
		for _, j := range axes[key] {
			other := series[j]
			if !simulationPointAxisEqual(item.Points, other.Points) {
				continue
			}
			equal := true
			if &item.NativeIntervals[0] != &other.NativeIntervals[0] {
				for k, interval := range item.NativeIntervals {
					if interval != other.NativeIntervals[k] {
						equal = false
						break
					}
				}
			}
			if equal {
				owner = j
				break
			}
		}
		if owner >= 0 {
			item.NativeIntervalsSourceID, item.NativeIntervals = series[owner].SourceID, nil
		} else {
			axes[key] = append(axes[key], i)
		}
	}
}

func hvacRuntimeControlCompleteness(loops []HVACLoopRunResult) *PurposeCompletenessItem {
	profiles, required, collected := 0, 0, 0
	var messages []string
	var observations []SimulationSeries
	for _, loop := range loops {
		if len(loop.OperatingProfiles) == 0 {
			continue
		}
		profiles += len(loop.OperatingProfiles)
		observations = append(observations, loop.ControlSeries...)
		if loop.ControlCoverage == nil {
			messages = append(messages, loop.Name+": executed control observation coverage is missing")
			continue
		}
		coverage := loop.ControlCoverage
		required += coverage.Required
		collected += coverage.Collected
		if coverage.Status != "complete" || coverage.Missing > 0 || coverage.Collected != coverage.Required {
			message := coverage.Message
			if message == "" {
				message = fmt.Sprintf("%d of %d required control observations were acquired", coverage.Collected, coverage.Required)
			}
			if loop.Name != "" {
				message = loop.Name + ": " + message
			}
			messages = append(messages, message)
		}
	}
	if profiles == 0 {
		return nil
	}
	item := purposeCompleteness(SimulationPurposeHVACLoopCheck, "HVAC control diagnostics", len(messages) == 0, seriesSource(observations))
	if len(messages) > 0 {
		item.Message = fmt.Sprintf("%d of %d required HVAC control observations were acquired. %s", collected, required, strings.Join(messages, "; "))
	}
	return &item
}

func hvacRuntimeLocalHourlySeries(profiles []idf.HVACOperatingProfile, doc *idf.Document, series []SimulationSeries) []SimulationSeries {
	nodes := map[string]bool{}
	components := map[string][]string{}
	for _, profile := range profiles {
		source := hvacRuntimeComponent(profile)
		components[normalizePurposeToken(source.ObjectName)] = append(components[normalizePurposeToken(source.ObjectName)], source.ObjectType)
		for _, binding := range hvacRuntimeSignals(profile, idf.HVACLoop{}, doc) {
			if strings.HasPrefix(binding.VariableName, "System Node ") {
				nodes[normalizePurposeToken(binding.KeyValue)] = true
			}
		}
	}
	var out []SimulationSeries
	for _, item := range series {
		if !hvacSeriesIsHourly(item) {
			continue
		}
		key, variable := hvacSeriesIdentity(item)
		if nodes[normalizePurposeToken(key)] && purposeSeriesMatchesVariables(item.Column, hvacLoopCheckNodeVariables()) {
			out = append(out, item)
			continue
		}
		for _, typ := range components[normalizePurposeToken(key)] {
			if hvacComponentTypeReportsVariable(typ, variable) {
				out = append(out, item)
				break
			}
		}
	}
	return out
}
