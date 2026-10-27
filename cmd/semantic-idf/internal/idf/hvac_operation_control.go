package idf

import (
	"fmt"
	"strings"
)

type hvacOperatingControlBuilder struct {
	sequence HVACControlSequence
	nodes    map[string]bool
	edges    map[string]bool
}

func (g *hvacOperatingControlBuilder) node(id, kind, code string, source Object, parameters []HVACControlParameter) string {
	if !g.nodes[id] {
		g.nodes[id] = true
		g.sequence.Nodes = append(g.sequence.Nodes, HVACControlNode{ID: id, Kind: kind, Code: code, Source: hvacOperatingSource(source), Parameters: parameters})
	}
	return id
}

func (g *hvacOperatingControlBuilder) edge(from, to, relation string) {
	key := from + "|" + to + "|" + relation
	if from != to && !g.edges[key] {
		g.edges[key] = true
		g.sequence.Edges = append(g.sequence.Edges, HVACControlEdge{From: from, To: to, Relation: relation})
	}
}

func (b *hvacOperatingBuilder) control(occ hvacOperatingOccurrence, layers []HVACAvailabilityLayer) HVACControlSequence {
	g := hvacOperatingControlBuilder{sequence: HVACControlSequence{Nodes: []HVACControlNode{}, Edges: []HVACControlEdge{}}, nodes: map[string]bool{}, edges: map[string]bool{}}
	obj := occ.object
	family, inputCode, outputCode := hvacOperatingRecipe(obj.Type)
	if family == "passive" {
		input := g.node("flow", "input", inputCode, obj, nil)
		output := g.node("output", "actuator", outputCode, obj, nil)
		g.edge(input, output, "transfers")
		return g.sequence
	}
	permission := g.node("permission", "availability", "availability_permission", obj, nil)
	if family == "unresolved" {
		unknown := g.node("unresolved", "unknown", "unresolved_control", obj, hvacOperatingControlParameters(obj))
		g.edge(permission, unknown, "permits")
		return g.sequence
	}
	decision := g.node("decision", "decision", family+"_on_off", obj, hvacOperatingStageParameters(obj, true))
	modulation := g.node("modulation", "modulation", family+"_modulation", obj, hvacOperatingStageParameters(obj, false))
	output := g.node("output", "actuator", outputCode, obj, nil)
	off := g.node("off", "state", "off_command", obj, nil)
	g.edge(permission, decision, "permits")
	g.edge(permission, off, "blocked")
	g.edge(decision, modulation, "requested")
	g.edge(modulation, output, "modulates")
	stopsWithoutRequest := family != "pump"
	if family == "pump" {
		for index, field := range obj.Fields {
			if strings.EqualFold(hvacOperatingFieldName(obj, index), "Pump Control Type") && strings.EqualFold(strings.TrimSpace(field.Value), "Intermittent") {
				stopsWithoutRequest = true
			}
		}
	}
	if stopsWithoutRequest {
		g.edge(decision, off, "no_request")
	}
	input := g.node("demand", "input", inputCode, obj, nil)
	g.edge(input, decision, "requests")
	for _, parent := range occ.ancestors {
		if len(hvacOperatingControlParameters(parent)) == 0 {
			continue
		}
		id := g.node(fmt.Sprintf("parent:%d", parent.Index), "controller", "parent_control", parent, hvacOperatingControlParameters(parent))
		g.edge(id, modulation, "controls")
		g.addParameterInputs(b, parent, id)
	}
	g.addParameterInputs(b, obj, modulation)
	b.addLinkedControllers(&g, occ, modulation)
	b.addThermostats(&g, occ, decision)
	b.addZoneDispatch(&g, occ, decision)
	if dispatch, ok := b.dispatch(occ); ok {
		for _, choice := range dispatch.choices {
			parameters := hvacOperatingDispatchParameters(choice, occ.object)
			if occ.loop != nil {
				owner := b.objects[occ.loop.ObjectIndex]
				for _, name := range []string{"Load Distribution Scheme", "Plant Loop Demand Calculation Scheme"} {
					if parameter, found := hvacOperatingNamedParameter(owner, name); found {
						parameters = append(parameters, parameter)
					}
				}
			}
			id := g.node(fmt.Sprintf("dispatch:%d", choice.operation.Index), "dispatch", "plant_dispatch", choice.operation, parameters)
			g.edge(id, decision, "dispatches")
		}
	}
	return g.sequence
}

func (b *hvacOperatingBuilder) addZoneDispatch(g *hvacOperatingControlBuilder, occ hvacOperatingOccurrence, target string) {
	indices := map[int]bool{occ.object.Index: true}
	for _, owner := range occ.ancestors {
		indices[owner.Index] = true
	}
	for _, relation := range b.relations {
		for _, component := range append(append([]HVACComponent(nil), relation.ZoneEquipment...), relation.TerminalUnits...) {
			if !indices[component.ObjectIndex] || component.CoolingSequence == "" && component.HeatingSequence == "" {
				continue
			}
			list, found := b.objects[component.SourceOwnerObjectIndex]
			if !found || !strings.EqualFold(list.Type, "ZoneHVAC:EquipmentList") {
				continue
			}
			var params []HVACControlParameter
			for _, ref := range zoneEquipmentReferences(b.ctx, list) {
				if !strings.EqualFold(hvacFieldValue(list, ref.TypeIndex), component.ObjectType) || !strings.EqualFold(hvacFieldValue(list, ref.NameIndex), component.ObjectName) {
					continue
				}
				for _, field := range []struct {
					index int
					name  string
				}{{ref.CoolingSequenceIndex, "Cooling Sequence"}, {ref.HeatingSequenceIndex, "Heating Sequence"}, {ref.CoolingScheduleIndex, "Sequential Cooling Fraction Schedule Name"}, {ref.HeatingScheduleIndex, "Sequential Heating Fraction Schedule Name"}} {
					if value := hvacFieldValue(list, field.index); value != "" {
						params = append(params, HVACControlParameter{Name: field.name, Value: value, ObjectIndex: list.Index, FieldIndex: field.index})
					}
				}
			}
			id := g.node(fmt.Sprintf("zone_dispatch:%d:%d", list.Index, component.ObjectIndex), "dispatch", "zone_equipment_priority", list, params)
			g.edge(id, target, "dispatches")
			for _, param := range params {
				if strings.Contains(param.Name, "Schedule") {
					g.edge(g.scheduleNode(b, param.Value, list), id, "sets_fraction")
				}
			}
		}
	}
}

func hvacOperatingStageParameters(obj Object, onOff bool) []HVACControlParameter {
	var parameters []HVACControlParameter
	for _, parameter := range hvacOperatingControlParameters(obj) {
		name := strings.ToLower(parameter.Name)
		isDecision := strings.Contains(name, "control type") || strings.Contains(name, "operating mode") || strings.Contains(name, "deadband") || strings.Contains(name, "temperature") && (strings.Contains(name, "limit") || strings.Contains(name, "minimum outdoor") || strings.Contains(name, "maximum outdoor"))
		if isDecision == onOff {
			parameters = append(parameters, parameter)
		}
	}
	return parameters
}

// Small family recipes label what the EnergyPlus component regulates. Exact
// parameter values, schedules, controllers and setpoints remain source-backed;
// the recipe never invents a numerical threshold or a controller connection.
func hvacOperatingRecipe(objectType string) (string, string, string) {
	lower := strings.ToLower(objectType)
	switch {
	case strings.HasPrefix(lower, "boiler:"):
		return "boiler", "heating_load_and_setpoint", "heating_rate_and_water_flow"
	case strings.HasPrefix(lower, "chiller:"):
		return "chiller", "cooling_load_and_setpoint", "cooling_rate_and_water_flow"
	case strings.HasPrefix(lower, "pump:") || strings.HasPrefix(lower, "headeredpumps:"):
		return "pump", "loop_flow_request", "pump_flow_and_power"
	case strings.HasPrefix(lower, "fan:"):
		return "fan", "airflow_request", "fan_airflow_and_power"
	case isWaterCoilType(objectType):
		return "water_coil", "air_load_or_outlet_setpoint", "coil_water_flow_and_heat_transfer"
	case strings.HasPrefix(lower, "coil:"):
		return "coil", "air_load_or_outlet_setpoint", "coil_capacity_and_runtime"
	case strings.HasPrefix(lower, "coilsystem:") || strings.Contains(lower, "unitary") || strings.Contains(lower, "furnace"):
		return "unitary", "zone_load_or_supply_setpoint", "fan_and_coil_commands"
	case strings.HasPrefix(lower, "airterminal:"):
		return "terminal", "zone_load_and_airflow_limits", "terminal_airflow_and_reheat"
	case isRefrigerantSystemType(objectType):
		return "vrf", "terminal_load_and_operating_mode", "compressor_and_terminal_capacity"
	case strings.HasPrefix(lower, "zonehvac:"):
		if lower == "zonehvac:energyrecoveryventilator" {
			return "ventilation", "ventilation_schedule_and_economizer", "ventilation_airflow_and_heat_recovery"
		}
		if lower == "zonehvac:outdoorairunit" {
			return "outdoor_air", "outdoor_air_temperature_control", "outdoor_airflow_and_conditioning"
		}
		if strings.Contains(lower, "dehumidifier") {
			return "dehumidifier", "zone_moisture_load", "moisture_removal"
		}
		return "zone_equipment", "zone_heating_cooling_load", "zone_equipment_capacity_and_flow"
	case strings.HasPrefix(lower, "coolingtower:") || strings.HasPrefix(lower, "fluidcooler:") || strings.HasPrefix(lower, "evaporativefluidcooler:"):
		return "heat_rejection", "water_outlet_setpoint", "fan_speed_and_heat_rejection"
	case strings.HasPrefix(lower, "heatexchanger:"):
		return "heat_exchanger", "temperature_difference_and_control_mode", "heat_transfer_and_bypass"
	case strings.HasPrefix(lower, "waterheater:"):
		return "water_heater", "tank_temperature_and_setpoint", "heater_capacity_and_flow"
	case strings.HasPrefix(lower, "districtheating") || strings.HasPrefix(lower, "districtcooling"):
		return "district", "plant_load_request", "delivered_capacity_and_flow"
	case strings.HasPrefix(lower, "heatpump:") || lower == "centralheatpumpsystem":
		return "heat_pump", "plant_load_and_operating_mode", "heat_pump_capacity_and_flow"
	case strings.HasPrefix(lower, "pipe:") || lower == "duct" || strings.HasPrefix(lower, "groundheatexchanger:"):
		return "passive", "upstream_flow", "passive_transfer"
	case strings.HasPrefix(lower, "evaporativecooler:"):
		return "evaporative_cooler", "cooling_request_and_limits", "cooling_and_airflow"
	case strings.HasPrefix(lower, "humidifier:"):
		return "humidifier", "humidity_setpoint_and_airflow", "humidification_rate"
	default:
		return "unresolved", "unresolved_control_input", "unresolved_control_output"
	}
}

func hvacOperatingControlParameters(obj Object) []HVACControlParameter {
	var result []HVACControlParameter
	for index, field := range obj.Fields {
		name := hvacOperatingFieldName(obj, index)
		lower := strings.ToLower(name)
		if index == 0 || strings.TrimSpace(field.Value) == "" || strings.Contains(lower, "availability") {
			continue
		}
		control := strings.Contains(lower, "control") || strings.Contains(lower, "setpoint") || strings.Contains(lower, "schedule") || strings.Contains(lower, "part load") || strings.Contains(lower, "flow mode") || strings.Contains(lower, "operating mode") || strings.Contains(lower, "cycling") || strings.Contains(lower, "minimum") || strings.Contains(lower, "maximum") || strings.Contains(lower, "limit") || strings.Contains(lower, "deadband") || strings.Contains(lower, "speed") || strings.Contains(lower, "curve") || strings.Contains(lower, "capacity") || strings.Contains(lower, "pressure rise")
		if control {
			result = append(result, HVACControlParameter{Name: name, Value: strings.TrimSpace(field.Value), ObjectIndex: obj.Index, FieldIndex: index})
		}
	}
	return result
}

func hvacOperatingAllParameters(obj Object) []HVACControlParameter {
	var result []HVACControlParameter
	for index, field := range obj.Fields {
		if index == 0 || strings.TrimSpace(field.Value) == "" {
			continue
		}
		name := hvacOperatingFieldName(obj, index)
		if name == "" {
			name = hvacOperatingFallbackFieldName(obj, index)
		}
		result = append(result, HVACControlParameter{Name: name, Value: strings.TrimSpace(field.Value), ObjectIndex: obj.Index, FieldIndex: index})
	}
	return result
}

func hvacOperatingFallbackFieldName(obj Object, index int) string {
	lower := strings.ToLower(obj.Type)
	switch {
	case strings.HasPrefix(lower, "availabilitymanager:scheduled") && index == 1:
		return "Schedule Name"
	case lower == "plantequipmentlist" || lower == "condenserequipmentlist":
		if index%2 == 1 {
			return "Equipment Object Type"
		}
		return "Equipment Name"
	case lower == "plantequipmentoperation:uncontrolled" && index == 1:
		return "Equipment List Name"
	case lower == "plantequipmentoperationschemes" || lower == "condenserequipmentoperationschemes":
		number := (index-1)/3 + 1
		switch index % 3 {
		case 1:
			return fmt.Sprintf("Control Scheme %d Object Type", number)
		case 2:
			return fmt.Sprintf("Control Scheme %d Name", number)
		default:
			return fmt.Sprintf("Control Scheme %d Schedule Name", number)
		}
	case lower == "plantequipmentoperation:heatingload" || lower == "plantequipmentoperation:coolingload":
		switch index % 3 {
		case 1:
			return "Load Range Lower Limit"
		case 2:
			return "Load Range Upper Limit"
		default:
			return "Equipment List Name"
		}
	}
	return fmt.Sprintf("Field %d", index+1)
}

func (g *hvacOperatingControlBuilder) scheduleNode(b *hvacOperatingBuilder, name string, fallback Object) string {
	source := fallback
	for _, candidate := range b.ctx.objectsByName[normalizeName(name)] {
		if isScheduleType(candidate.Type) {
			source = candidate
			break
		}
	}
	return g.node("schedule:"+normalizeName(name), "schedule", "schedule_value", source, []HVACControlParameter{{Name: "Schedule Name", Value: name, ObjectIndex: fallback.Index, FieldIndex: -1}})
}

func (g *hvacOperatingControlBuilder) addParameterInputs(b *hvacOperatingBuilder, obj Object, target string) {
	for _, parameter := range hvacOperatingControlParameters(obj) {
		lower := strings.ToLower(parameter.Name)
		if strings.Contains(lower, "schedule") {
			g.edge(g.scheduleNode(b, parameter.Value, obj), target, "sets_parameter")
		} else if strings.Contains(lower, "curve") {
			for _, curve := range b.ctx.objectsByName[normalizeName(parameter.Value)] {
				if strings.HasPrefix(strings.ToLower(curve.Type), "curve:") {
					id := g.node(fmt.Sprintf("curve:%d", curve.Index), "parameter", "performance_curve", curve, hvacOperatingAllParameters(curve))
					g.edge(id, target, "modifies_performance")
				}
			}
		}
	}
}

func (b *hvacOperatingBuilder) operatingNodeSet(occ hvacOperatingOccurrence) map[string]bool {
	nodes := map[string]bool{}
	// A shared chiller or HX has separate circuits. Its occurrence ports are
	// authoritative; setpoints on the other circuit must not leak into this view.
	if occ.loop != nil {
		for _, component := range loopComponents(*occ.loop) {
			if component.ObjectIndex == occ.object.Index && strings.EqualFold(component.ObjectType, occ.object.Type) {
				for _, value := range []string{component.InletNode, component.OutletNode} {
					if value != "" {
						nodes[normalizeName(value)] = true
					}
				}
			}
		}
	}
	hasCircuitPorts := len(nodes) > 0
	for _, usage := range b.ctx.nodeUsages {
		if usage.ObjectIndex == occ.object.Index && !hasCircuitPorts {
			if occ.loop != nil && strings.EqualFold(occ.loop.Type, "AirLoopHVAC") && (strings.HasPrefix(usage.Role, "water_") || strings.HasPrefix(usage.Role, "plant_") || strings.HasPrefix(usage.Role, "condenser_")) {
				continue
			}
			nodes[normalizeName(usage.NodeName)] = true
		}
	}
	family, _, _ := hvacOperatingRecipe(occ.object.Type)
	if occ.loop != nil && !strings.EqualFold(occ.loop.Type, "AirLoopHVAC") && occ.loop.SetpointNode != "" && family != "pump" && family != "passive" && family != "unresolved" {
		nodes[normalizeName(occ.loop.SetpointNode)] = true
	}
	return nodes
}

func (b *hvacOperatingBuilder) expandedNodes(value string) []string {
	if nodes, found := b.ctx.nodeLists[normalizeName(value)]; found {
		return nodes
	}
	if value != "" {
		return []string{value}
	}
	return nil
}

func (b *hvacOperatingBuilder) addLinkedControllers(g *hvacOperatingControlBuilder, occ hvacOperatingOccurrence, target string) {
	nodes := b.operatingNodeSet(occ)
	actuatorNodes := map[string]bool{}
	for node := range nodes {
		actuatorNodes[node] = true
	}
	// A hydronic coil's controller actuates WATER flow even when this selected
	// occurrence is on the air circuit. Match that physical inlet explicitly.
	if isWaterCoilType(occ.object.Type) {
		for _, usage := range b.ctx.nodeUsages {
			if usage.ObjectIndex == occ.object.Index && strings.Contains(usage.Role, "inlet") {
				actuatorNodes[normalizeName(usage.NodeName)] = true
			}
		}
	}
	selected := map[int]Object{}
	for _, obj := range b.ctx.doc.Objects {
		if !strings.HasPrefix(strings.ToLower(obj.Type), "controller:") {
			continue
		}
		actuator := fieldValueByCatalogName(obj, "Actuator Node Name")
		if actuator != "" && actuatorNodes[normalizeName(actuator)] {
			selected[obj.Index] = obj
		}
	}
	owners := append(append([]Object(nil), occ.ancestors...), occ.object)
	if occ.loop != nil {
		owners = append(owners, b.objects[occ.loop.ObjectIndex])
	}
	for _, owner := range owners {
		listName := fieldValueByCatalogName(owner, "Controller List Name")
		list, found := b.exactObject("AirLoopHVAC:ControllerList", listName)
		if !found {
			continue
		}
		for index := 1; index+1 < len(list.Fields); index += 2 {
			if controller, exists := b.exactObject(hvacFieldValue(list, index), hvacFieldValue(list, index+1)); exists {
				// A loop list is shared by all coils. Its actuator must still
				// touch this component; wrapper-specific lists are inherited.
				actuator := fieldValueByCatalogName(controller, "Actuator Node Name")
				if owner.Index != occ.loopIndex() || actuator != "" && actuatorNodes[normalizeName(actuator)] {
					selected[controller.Index] = controller
				}
			}
		}
	}
	for _, obj := range b.ctx.doc.Objects {
		controller, ok := selected[obj.Index]
		if !ok {
			continue
		}
		id := g.node(fmt.Sprintf("controller:%d", controller.Index), "controller", "feedback_controller", controller, hvacOperatingAllParameters(controller))
		g.edge(id, target, "controls")
		for _, name := range []string{"Sensor Node Name", "Actuator Node Name"} {
			value := fieldValueByCatalogName(controller, name)
			if value == "" {
				continue
			}
			nodes[normalizeName(value)] = true
			if name == "Sensor Node Name" {
				sensor := g.node("sensor:"+normalizeName(value), "sensor", "node_measurement", controller, []HVACControlParameter{{Name: name, Value: value, ObjectIndex: controller.Index, FieldIndex: -1}})
				g.edge(sensor, id, "measures")
			}
		}
		g.addParameterInputs(b, controller, id)
	}
	// Follow target-node matches and then any reference setpoint nodes. This
	// captures MixedAir/Follow* chains without matching source object names.
	seen := map[int]bool{}
	for pass := 0; pass < len(b.ctx.doc.Objects); pass++ {
		changed := false
		for _, manager := range b.ctx.doc.Objects {
			if seen[manager.Index] || !strings.HasPrefix(strings.ToLower(manager.Type), "setpointmanager:") {
				continue
			}
			matched := false
			for index := range manager.Fields {
				name := strings.ToLower(hvacOperatingFieldName(manager, index))
				if !strings.Contains(name, "setpoint") || !strings.Contains(name, "node") || strings.Contains(name, "reference") {
					continue
				}
				for _, node := range b.expandedNodes(hvacFieldValue(manager, index)) {
					matched = matched || nodes[normalizeName(node)]
				}
			}
			if !matched {
				continue
			}
			seen[manager.Index], changed = true, true
			id := g.node(fmt.Sprintf("setpoint:%d", manager.Index), "setpoint", "setpoint_manager", manager, hvacOperatingAllParameters(manager))
			g.edge(id, target, "sets_target")
			for index := range manager.Fields {
				name := strings.ToLower(hvacOperatingFieldName(manager, index))
				if strings.Contains(name, "reference") && strings.Contains(name, "node") {
					nodes[normalizeName(hvacFieldValue(manager, index))] = true
				}
			}
			g.addParameterInputs(b, manager, id)
		}
		if !changed {
			break
		}
	}
}

func (occ hvacOperatingOccurrence) loopIndex() int {
	if occ.loop != nil {
		return occ.loop.ObjectIndex
	}
	return -1
}

func (b *hvacOperatingBuilder) addThermostats(g *hvacOperatingControlBuilder, occ hvacOperatingOccurrence, target string) {
	lowerType := strings.ToLower(occ.object.Type)
	for _, excluded := range []string{"energyrecoveryventilator", "outdoorairunit", "dehumidifier"} {
		if strings.Contains(lowerType, excluded) {
			return
		}
		for _, parent := range occ.ancestors {
			if strings.Contains(strings.ToLower(parent.Type), excluded) {
				return
			}
		}
	}
	zones := map[string]bool{}
	indices := map[int]bool{occ.object.Index: true}
	for _, ancestor := range occ.ancestors {
		indices[ancestor.Index] = true
	}
	for _, relation := range b.relations {
		for _, equipment := range append(append([]HVACComponent(nil), relation.ZoneEquipment...), relation.TerminalUnits...) {
			if indices[equipment.ObjectIndex] {
				zones[normalizeName(relation.ZoneName)] = true
			}
		}
	}
	for _, owner := range append(append([]Object(nil), occ.ancestors...), occ.object) {
		name := fieldValueByCatalogName(owner, "Controlling Zone or Thermostat Location", "Controlling Zone or Thermostat Location Name", "Control Zone Name")
		if name != "" {
			zones[normalizeName(name)] = true
		}
	}
	for _, thermostat := range b.ctx.objectsByType[normalizeFieldCatalogKey("ZoneControl:Thermostat")] {
		zoneName := hvacFieldValue(thermostat, 1)
		matched := zones[normalizeName(zoneName)]
		if list, exists := b.exactObject("ZoneList", zoneName); exists {
			for index := 1; index < len(list.Fields); index++ {
				matched = matched || zones[normalizeName(hvacFieldValue(list, index))]
			}
		}
		if !matched {
			continue
		}
		id := g.node(fmt.Sprintf("thermostat:%d", thermostat.Index), "controller", "zone_thermostat", thermostat, hvacOperatingAllParameters(thermostat))
		g.edge(id, target, "requests")
		if schedule := hvacFieldValue(thermostat, 2); schedule != "" {
			g.edge(g.scheduleNode(b, schedule, thermostat), id, "selects_mode")
		}
		for index := 3; index+1 < len(thermostat.Fields); index += 2 {
			setpoint, exists := b.exactObject(hvacFieldValue(thermostat, index), hvacFieldValue(thermostat, index+1))
			if !exists || !strings.HasPrefix(strings.ToLower(setpoint.Type), "thermostatsetpoint:") {
				continue
			}
			setpointID := g.node(fmt.Sprintf("thermostat_setpoint:%d", setpoint.Index), "setpoint", "thermostat_setpoint", setpoint, hvacOperatingAllParameters(setpoint))
			g.edge(setpointID, id, "sets_target")
			for fieldIndex := 1; fieldIndex < len(setpoint.Fields); fieldIndex++ {
				name := hvacFieldValue(setpoint, fieldIndex)
				for _, candidate := range b.ctx.objectsByName[normalizeName(name)] {
					if isScheduleType(candidate.Type) {
						g.edge(g.scheduleNode(b, name, setpoint), setpointID, "sets_target")
						break
					}
				}
			}
		}
	}
}
