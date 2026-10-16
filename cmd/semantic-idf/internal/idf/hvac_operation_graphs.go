package idf

import (
	"fmt"
	"strconv"
	"strings"
)

// The three presentation graphs intentionally have different contracts. Raw
// declarations never contain permission algorithms; command decisions never
// imply simulated status; signal flow distinguishes commands from responses.
// Legacy Control and availability expressions remain compatible API fields.
func newHVACOperatingGraph() hvacOperatingControlBuilder {
	return hvacOperatingControlBuilder{sequence: HVACControlSequence{Nodes: []HVACControlNode{}, Edges: []HVACControlEdge{}}, nodes: map[string]bool{}, edges: map[string]bool{}}
}

func (g *hvacOperatingControlBuilder) boundaryNode(id, kind, code, boundary string, source Object, parameters []HVACControlParameter) string {
	g.node(id, kind, code, source, parameters)
	for index := range g.sequence.Nodes {
		if g.sequence.Nodes[index].ID == id {
			g.sequence.Nodes[index].Boundary = boundary
			break
		}
	}
	return id
}

func (g *hvacOperatingControlBuilder) signal(from, to, relation, polarity, condition string) {
	key := from + "|" + to + "|" + relation + "|" + polarity + "|" + condition
	if from != to && !g.edges[key] {
		g.edges[key] = true
		g.sequence.Edges = append(g.sequence.Edges, HVACControlEdge{From: from, To: to, Relation: relation, Polarity: polarity, Condition: condition})
	}
}

func hvacOperatingNamedParameter(obj Object, name string) (HVACControlParameter, bool) {
	for index := range obj.Fields {
		if strings.EqualFold(hvacOperatingFieldName(obj, index), name) && hvacFieldValue(obj, index) != "" {
			return HVACControlParameter{Name: name, Value: hvacFieldValue(obj, index), ObjectIndex: obj.Index, FieldIndex: index}, true
		}
	}
	return HVACControlParameter{}, false
}

func (b *hvacOperatingBuilder) availabilityDefinitions(occ hvacOperatingOccurrence, layers []HVACAvailabilityLayer) HVACControlSequence {
	g := newHVACOperatingGraph()
	seenOwners := map[int]bool{}
	for _, layer := range layers {
		if layer.Role == "plant_dispatch" {
			b.dispatchAvailabilityDefinitions(&g, occ, layer)
			continue // Only its authored activation references, never load/range algorithms.
		}
		owner, exists := b.objects[layer.Owner.ObjectIndex]
		if !exists {
			continue
		}
		id := g.node(fmt.Sprintf("definition:%d", owner.Index), "definition", "availability_owner", owner, nil)
		if layer.Role == "unresolved_parent" {
			unknown := g.node("definition:unresolved_parent", "unknown", "multiple_parent_paths", owner, nil)
			g.edge(id, unknown, "unresolved_reference")
		}
		if seenOwners[owner.Index] {
			continue
		}
		seenOwners[owner.Index] = true
		if name := hvacOperatingAvailabilitySchedule(owner); name != "" {
			for index := range owner.Fields {
				if hvacOperatingAvailabilityFieldName(hvacOperatingFieldName(owner, index)) {
					for nodeIndex := range g.sequence.Nodes {
						if g.sequence.Nodes[nodeIndex].ID == id {
							g.sequence.Nodes[nodeIndex].Parameters = append(g.sequence.Nodes[nodeIndex].Parameters, HVACControlParameter{Name: hvacOperatingFieldName(owner, index), Value: name, ObjectIndex: owner.Index, FieldIndex: index})
						}
					}
				}
			}
			g.edge(id, b.definitionSchedule(&g, name, "annual", owner, map[int]bool{}), "declares_schedule")
		} else {
			code := "no_availability_schedule"
			if !hvacOperatingAvailabilityKnown(owner) {
				code = "unresolved_availability_schema"
			}
			g.edge(id, g.node(id+":schedule", "definition", code, owner, nil), "declares_schedule")
		}
		listName := hvacOperatingManagerListName(owner)
		if listName == "" {
			continue
		}
		list, found := b.exactObject("AvailabilityManagerAssignmentList", listName)
		if !found {
			missing := g.node(id+":manager_list", "unknown", "missing_manager_list", owner, []HVACControlParameter{{Name: "Availability Manager List Name", Value: listName, ObjectIndex: owner.Index, FieldIndex: -1}})
			g.edge(id, missing, "declares_manager_list")
			continue
		}
		listID := g.node(fmt.Sprintf("manager_list:%d", list.Index), "definition", "availability_manager_list", list, hvacOperatingAllParameters(list))
		g.edge(id, listID, "declares_manager_list")
		for index := 1; index+1 < len(list.Fields); index += 2 {
			typ, name := hvacFieldValue(list, index), hvacFieldValue(list, index+1)
			if typ == "" && name == "" {
				continue
			}
			manager, found := b.exactObject(typ, name)
			if !found {
				missing := g.node(fmt.Sprintf("manager_missing:%d:%d", list.Index, index), "unknown", "missing_manager", list, []HVACControlParameter{{Name: "Availability Manager Object Type", Value: typ, ObjectIndex: list.Index, FieldIndex: index}, {Name: "Availability Manager Name", Value: name, ObjectIndex: list.Index, FieldIndex: index + 1}})
				g.edge(listID, missing, "references_manager")
				continue
			}
			managerID := g.node(fmt.Sprintf("manager:%d", manager.Index), "definition", "availability_manager", manager, hvacOperatingAllParameters(manager))
			g.edge(listID, managerID, "references_manager")
			for _, parameter := range hvacOperatingAllParameters(manager) {
				if strings.Contains(strings.ToLower(parameter.Name), "schedule") {
					g.edge(managerID, b.definitionSchedule(&g, parameter.Value, "annual", manager, map[int]bool{}), "references_schedule")
				}
			}
		}
	}
	return g.sequence
}

// Schedule names are only unique inside annual/week/day namespaces. Preserve
// complete raw definitions and their actual references, including unsupported
// external schedules, without silently choosing the first duplicate object.
func (b *hvacOperatingBuilder) definitionSchedule(g *hvacOperatingControlBuilder, name, namespace string, fallback Object, seen map[int]bool) string {
	var found Object
	count := 0
	for _, candidate := range b.ctx.objectsByName[normalizeName(name)] {
		typ := strings.ToLower(candidate.Type)
		if !isScheduleType(candidate.Type) && !(strings.HasPrefix(typ, "externalinterface:") && strings.HasSuffix(typ, ":schedule")) {
			continue
		}
		if hvacOperationScheduleNamespace(compileHVACOperationSchedule(candidate)) == namespace {
			found, count = candidate, count+1
		}
	}
	if count != 1 {
		return g.node("schedule_missing:"+namespace+":"+normalizeName(name), "unknown", "unresolved_schedule_reference", fallback, []HVACControlParameter{{Name: "Schedule Name", Value: name, ObjectIndex: fallback.Index, FieldIndex: -1}})
	}
	id := g.node(fmt.Sprintf("schedule:%d", found.Index), "schedule", "schedule_value", found, hvacOperatingAllParameters(found))
	if seen[found.Index] {
		return id
	}
	seen[found.Index] = true
	compiled := compileHVACOperationSchedule(found)
	for _, period := range compiled.Periods {
		g.edge(id, b.definitionSchedule(g, period.ScheduleName, "week", found, seen), "references_schedule")
	}
	for _, dayType := range hvacOperationScheduleDayTypes {
		if child := compiled.WeekDays[dayType]; child != "" {
			g.edge(id, b.definitionSchedule(g, child, "day", found, seen), "references_schedule")
		}
	}
	for _, rule := range compiled.Rules {
		if rule.ScheduleName != "" {
			g.edge(id, b.definitionSchedule(g, rule.ScheduleName, "day", found, seen), "references_schedule")
		}
	}
	return id
}

func (b *hvacOperatingBuilder) onOffControl(occ hvacOperatingOccurrence, profile HVACOperatingProfile) HVACControlSequence {
	g := newHVACOperatingGraph()
	family, inputCode, _ := hvacOperatingRecipe(occ.object.Type)
	if family == "passive" {
		input := g.boundaryNode("flow", "external_input", "upstream_flow", "input", occ.object, nil)
		g.edge(input, g.node("passive", "outcome", "passive_transfer", occ.object, nil), "transfers")
		return g.sequence
	}
	permission := g.node("permission", "decision", "availability_permission", occ.object, nil)
	for index, layer := range profile.AvailabilityLayers {
		owner := b.objects[layer.Owner.ObjectIndex]
		condition := b.onOffExpression(&g, fmt.Sprintf("condition:%d", index), layer.Expression, owner)
		g.edge(condition, permission, "requires_permission")
	}
	off := g.boundaryNode("off", "outcome", "off_command", "output", occ.object, nil)
	unknown := g.boundaryNode("unknown", "outcome", "runtime_status_unknown", "output", occ.object, nil)
	g.signal(permission, off, "blocked", "", "no")
	g.signal(permission, unknown, "unresolved", "", "unknown")
	if family == "unresolved" {
		g.signal(permission, unknown, "unresolved_control", "", "yes")
		return g.sequence
	}
	decision := g.node("decision", "decision", family+"_on_off", occ.object, hvacOperatingStageParameters(occ.object, true))
	g.signal(permission, decision, "permits", "", "yes")
	demand := g.boundaryNode("demand", "external_input", inputCode, "input", occ.object, nil)
	g.edge(demand, decision, "requests")
	on := g.boundaryNode("on", "outcome", "on_command", "output", occ.object, nil)
	g.signal(decision, on, "requested", "", "yes")
	g.signal(decision, unknown, "unresolved", "", "unknown")
	stops := family != "pump"
	if family == "pump" {
		parameter, found := hvacOperatingNamedParameter(occ.object, "Pump Control Type")
		stops = found && strings.EqualFold(parameter.Value, "Intermittent")
	}
	if stops {
		g.signal(decision, off, "no_request", "", "no")
	} else {
		// Continuous/default pump mode is not evidence for zero-flow OFF.
		g.signal(decision, unknown, "no_request_mode_dependent", "", "no")
	}
	// The old graph already resolves exact thermostat/dispatch ownership. Copy
	// only their incoming dependency closure, never modulation or outputs.
	needed := map[string]bool{"decision": true}
	for pass := 0; pass < len(profile.Control.Nodes); pass++ {
		for _, edge := range profile.Control.Edges {
			if needed[edge.To] && edge.From != "permission" && edge.From != "demand" {
				needed[edge.From] = true
			}
		}
	}
	ids := map[string]string{}
	for _, node := range profile.Control.Nodes {
		if needed[node.ID] && node.ID != "decision" {
			if node.Kind == "schedule" && len(node.Parameters) > 0 {
				// Legacy schedules chose the first same-named object. Resolve
				// their reference's annual namespace, including duplicates,
				// independently for the new decision graph.
				parameter := node.Parameters[0]
				ids[node.ID] = b.definitionSchedule(&g, parameter.Value, "annual", b.objects[parameter.ObjectIndex], map[int]bool{})
			} else {
				ids[node.ID] = g.node(node.ID, node.Kind, node.Code, b.objects[node.Source.ObjectIndex], node.Parameters)
			}
		}
	}
	for _, edge := range profile.Control.Edges {
		if needed[edge.From] && needed[edge.To] {
			from, to := edge.From, edge.To
			if replacement := ids[from]; replacement != "" {
				from = replacement
			}
			if replacement := ids[to]; replacement != "" {
				to = replacement
			}
			g.edge(from, to, edge.Relation)
		}
	}
	return g.sequence
}

func (b *hvacOperatingBuilder) onOffExpression(g *hvacOperatingControlBuilder, id string, expression HVACOperatingExpression, owner Object) string {
	if expression.Source != nil {
		if source, found := b.objects[expression.Source.ObjectIndex]; found {
			owner = source
		}
	}
	parameters := []HVACControlParameter{}
	if strings.HasPrefix(strings.ToLower(owner.Type), "availabilitymanager:") {
		parameters = hvacOperatingAllParameters(owner)
	}
	for _, value := range []struct{ name, value string }{{"Target", expression.Target}, {"Reason", expression.Reason}} {
		if value.value != "" {
			parameters = append(parameters, HVACControlParameter{Name: value.name, Value: value.value, ObjectIndex: owner.Index, FieldIndex: -1})
		}
	}
	if expression.Value != nil {
		parameters = append(parameters, HVACControlParameter{Name: "Value", Value: strconv.FormatFloat(*expression.Value, 'g', -1, 64), ObjectIndex: owner.Index, FieldIndex: -1})
	}
	if expression.ScheduleName != "" {
		parameters = append(parameters, HVACControlParameter{Name: "Schedule Name", Value: expression.ScheduleName, ObjectIndex: owner.Index, FieldIndex: -1})
		g.edge(b.definitionSchedule(g, expression.ScheduleName, "annual", owner, map[int]bool{}), id, "evaluates_schedule")
	}
	g.node(id, "decision", expression.Op, owner, parameters)
	var managerList Object
	if expression.Op == "air_managers" || expression.Op == "plant_managers" {
		managerList, _ = b.exactObject("AvailabilityManagerAssignmentList", hvacOperatingManagerListName(owner))
	}
	for index, child := range expression.Children {
		childOwner := owner
		if managerList.Type != "" {
			if manager, found := b.exactObject(hvacFieldValue(managerList, 1+2*index), hvacFieldValue(managerList, 2+2*index)); found {
				childOwner = manager
			}
		}
		childID := b.onOffExpression(g, fmt.Sprintf("%s:%d", id, index), child, childOwner)
		g.edge(childID, id, "evaluates_condition")
	}
	return id
}

func (b *hvacOperatingBuilder) parameterControl(occ hvacOperatingOccurrence, legacy HVACControlSequence) HVACControlSequence {
	g := newHVACOperatingGraph()
	obj := occ.object
	family, inputCode, outputCode := hvacOperatingRecipe(obj.Type)
	if family == "unitary" {
		outputCode = "supply_air_state_and_power"
	}
	equipment := g.node("equipment", "equipment", "equipment_response", obj, hvacOperatingModelParameters(obj))
	state := g.node("state", "state_variable", outputCode, obj, nil)
	output := g.boundaryNode("output", "external_output", outputCode, "output", obj, nil)
	g.edge(equipment, state, "determines_state")
	g.edge(state, output, "outputs")
	if family == "unresolved" {
		g.edge(g.node("unresolved", "unknown", "unresolved_control", obj, hvacOperatingControlParameters(obj)), equipment, "unresolved_control")
		return g.sequence
	}
	if family == "passive" {
		g.edge(g.boundaryNode("flow", "external_input", "upstream_flow", "input", obj, nil), equipment, "transfers")
		b.parameterPorts(&g, occ, equipment)
		return g.sequence
	}
	command, modulation := "", ""
	exactActuator := false
	for _, node := range legacy.Nodes {
		if node.Code == "feedback_controller" && b.waterCoilActuator(occ, b.objects[node.Source.ObjectIndex]) {
			exactActuator = true
		}
	}
	if !exactActuator {
		command = g.node("command", "control_variable", family+"_command", obj, nil)
		g.edge(command, equipment, "actuates")
		request := g.boundaryNode("request", "external_input", inputCode, "input", obj, nil)
		modulation = g.node("modulation", "controller", family+"_modulation", obj, hvacOperatingStageParameters(obj, false))
		g.edge(request, modulation, "requests")
		g.edge(modulation, command, "determines_command")
	}
	b.parameterInputs(&g, obj, modulation, equipment)
	// Keep source-backed configuration dependencies; decisions and dispatch
	// belong to On/Off. Setpoint/sensor wiring is rebuilt below by exact ports.
	for _, node := range legacy.Nodes {
		if node.Code != "parent_control" || exactActuator {
			continue
		}
		g.node(node.ID, node.Kind, node.Code, b.objects[node.Source.ObjectIndex], node.Parameters)
		b.parameterInputs(&g, b.objects[node.Source.ObjectIndex], node.ID, equipment)
		g.edge(node.ID, modulation, "controls")
	}
	ports := b.parameterPorts(&g, occ, equipment)
	controllers := b.parameterControllers(&g, occ, legacy, command, ports)
	b.parameterSetpoints(&g, occ, legacy, modulation, controllers, ports)
	if modulation != "" {
		b.parameterThermostats(&g, occ, legacy, modulation)
	}
	return g.sequence
}

func hvacOperatingModelParameters(obj Object) []HVACControlParameter {
	var parameters []HVACControlParameter
	for _, parameter := range hvacOperatingAllParameters(obj) {
		name := strings.ToLower(parameter.Name)
		if !strings.Contains(name, "node") && !strings.Contains(name, "availability") && !strings.Contains(name, "schedule") {
			parameters = append(parameters, parameter)
		}
	}
	return parameters
}

// Port identities use the exact field/node name, not inferred names. Circuit
// occurrences restrict shared equipment to the selected circuit; a hydronic
// coil also exposes its proven water actuator connection on the air circuit.
func (b *hvacOperatingBuilder) parameterPorts(g *hvacOperatingControlBuilder, occ hvacOperatingOccurrence, equipment string) map[string]string {
	ports := map[string]string{}
	selected := map[string]bool{}
	if occ.loop != nil {
		direct := false
		for _, component := range loopComponents(*occ.loop) {
			if component.ObjectIndex == occ.object.Index && strings.EqualFold(component.ObjectType, occ.object.Type) {
				direct = true
				for _, name := range []string{component.InletNode, component.OutletNode} {
					if strings.TrimSpace(name) != "" {
						selected[normalizeName(name)] = true
					}
				}
			}
		}
		// A typed wrapper/terminal occurrence can contain equipment which is
		// not itself a Branch entry. Its proven air ports still belong to this
		// air occurrence; unrelated plant/condenser circuits do not.
		if !direct && strings.EqualFold(occ.loop.Type, "AirLoopHVAC") {
			for _, usage := range b.ctx.nodeUsages {
				if usage.ObjectIndex == occ.object.Index && !strings.HasPrefix(usage.Role, "water_") && !strings.HasPrefix(usage.Role, "plant_") && !strings.HasPrefix(usage.Role, "condenser_") {
					selected[normalizeName(usage.NodeName)] = true
				}
			}
		}
		if len(selected) == 0 {
			unknown := g.boundaryNode("port:unresolved", "external_input", "unresolved_circuit_ports", "input", occ.object, nil)
			g.signal(unknown, equipment, "inlet_conditions", "unknown", "")
		}
	}
	for _, usage := range b.ctx.nodeUsages {
		if usage.ObjectIndex != occ.object.Index || !strings.Contains(usage.Role, "inlet") && !strings.Contains(usage.Role, "outlet") {
			continue
		}
		if occ.loop != nil && !selected[normalizeName(usage.NodeName)] && !(isWaterCoilType(occ.object.Type) && strings.Contains(usage.Role, "water_")) {
			continue
		}
		parameter := HVACControlParameter{Name: usage.FieldName, Value: usage.NodeName, ObjectIndex: usage.ObjectIndex, FieldIndex: usage.FieldIndex}
		id := "port:" + normalizeName(usage.NodeName)
		ports[normalizeName(usage.NodeName)] = id
		if strings.Contains(usage.Role, "inlet") {
			g.boundaryNode(id, "external_input", "inlet_state", "input", occ.object, []HVACControlParameter{parameter})
			g.edge(id, equipment, "inlet_conditions")
		} else {
			g.boundaryNode(id, "external_output", "outlet_state", "output", occ.object, []HVACControlParameter{parameter})
			g.edge(equipment, id, "outlet_conditions")
		}
	}
	return ports
}

type hvacParameterController struct {
	id, errorID, sensor, variable string
	objectIndex                   int
	waterFlow                     bool
}

func (b *hvacOperatingBuilder) waterCoilActuator(occ hvacOperatingOccurrence, controller Object) bool {
	actuator, hasActuator := hvacOperatingNamedParameter(controller, "Actuator Node Name")
	variable, _ := hvacOperatingNamedParameter(controller, "Actuator Variable")
	if !strings.EqualFold(controller.Type, "Controller:WaterCoil") || !strings.EqualFold(variable.Value, "Flow") || !hasActuator || !isWaterCoilType(occ.object.Type) {
		return false
	}
	for _, usage := range b.ctx.nodeUsages {
		if usage.ObjectIndex == occ.object.Index && strings.Contains(usage.Role, "water_") && strings.Contains(usage.Role, "inlet") && strings.EqualFold(usage.NodeName, actuator.Value) {
			return true
		}
	}
	return false
}

func (b *hvacOperatingBuilder) parameterControllers(g *hvacOperatingControlBuilder, occ hvacOperatingOccurrence, legacy HVACControlSequence, fallbackCommand string, ports map[string]string) []hvacParameterController {
	controllers := []hvacParameterController{}
	for _, node := range legacy.Nodes {
		if node.Code != "feedback_controller" {
			continue
		}
		obj := b.objects[node.Source.ObjectIndex]
		id := g.node(node.ID, "controller", node.Code, obj, node.Parameters)
		sensor, hasSensor := hvacOperatingNamedParameter(obj, "Sensor Node Name")
		actuator, _ := hvacOperatingNamedParameter(obj, "Actuator Node Name")
		variable, _ := hvacOperatingNamedParameter(obj, "Control Variable")
		actuatorVariable, _ := hvacOperatingNamedParameter(obj, "Actuator Variable")
		isWaterFlow := b.waterCoilActuator(occ, obj)
		command := fallbackCommand
		if isWaterFlow {
			command = g.node(id+":command", "control_variable", "water_flow_command", obj, []HVACControlParameter{actuator, actuatorVariable})
			g.edge(id, command, "determines_command")
			g.edge(command, "equipment", "actuates")
		} else if command != "" {
			g.signal(id, command, "controls", "unknown", "")
		} else {
			g.signal(id, "equipment", "unresolved_control", "unknown", "")
		}
		entry := hvacParameterController{id: id, variable: variable.Value, objectIndex: obj.Index, waterFlow: isWaterFlow}
		if hasSensor {
			entry.sensor = normalizeName(sensor.Value)
			sensorID := ports[entry.sensor]
			if sensorID == "" {
				sensorID = g.boundaryNode(id+":sensor", "external_input", "node_measurement", "input", obj, []HVACControlParameter{sensor, variable})
			}
			// Exact native convention: DeltaSensed = SensedValue - SetPointValue.
			// This is a comparison sign, NOT the sign of actuator response.
			// EnergyPlus v24.2 HVACControllers.cc, InitController.
			if isWaterFlow && strings.EqualFold(variable.Value, "Temperature") {
				entry.errorID = g.node(id+":residual", "comparison", "sensed_minus_setpoint", obj, []HVACControlParameter{variable, {Name: "Formula", Value: "Sensed value - Setpoint value", ObjectIndex: obj.Index, FieldIndex: -1}})
				g.signal(sensorID, entry.errorID, "adds_measurement", "positive", "")
				g.edge(entry.errorID, id, "compares")
			} else {
				g.signal(sensorID, id, "feedback", "unknown", "")
			}
		}
		b.parameterInputs(g, obj, id, "equipment")
		controllers = append(controllers, entry)
	}
	return controllers
}

func (b *hvacOperatingBuilder) parameterInputs(g *hvacOperatingControlBuilder, obj Object, target, performance string) {
	for _, parameter := range hvacOperatingControlParameters(obj) {
		name := strings.ToLower(parameter.Name)
		if strings.Contains(name, "schedule") && target != "" {
			g.edge(b.definitionSchedule(g, parameter.Value, "annual", obj, map[int]bool{}), target, "sets_parameter")
		} else if strings.Contains(name, "curve") && strings.Contains(name, "name") {
			var curves []Object
			for _, candidate := range b.ctx.objectsByName[normalizeName(parameter.Value)] {
				if strings.HasPrefix(strings.ToLower(candidate.Type), "curve:") {
					curves = append(curves, candidate)
				}
			}
			if len(curves) == 1 {
				curve := curves[0]
				g.edge(g.node(fmt.Sprintf("curve:%d", curve.Index), "parameter", "performance_curve", curve, hvacOperatingAllParameters(curve)), performance, "modifies_performance")
			} else if _, numeric := strconv.ParseFloat(parameter.Value, 64); numeric != nil {
				g.edge(g.node(fmt.Sprintf("curve_missing:%d:%d", obj.Index, parameter.FieldIndex), "unknown", "unresolved_curve_reference", obj, []HVACControlParameter{parameter}), performance, "modifies_performance")
			}
		}
	}
}

func hvacOperatingControlQuantityMatches(controller, setpoint string) bool {
	controller, setpoint = normalizeName(controller), normalizeName(setpoint)
	if controller == "temperatureandhumidityratio" {
		return setpoint == "temperature" || setpoint == "maximumhumidityratio"
	}
	if controller == "humidityratio" {
		return setpoint == "maximumhumidityratio" || setpoint == "humidityratio"
	}
	return controller != "" && controller == setpoint
}

// TemperatureAndHumidityRatio has two separate reference channels at the same
// sensed node. Knowing one target does not establish the other target/provider.
func hvacOperatingWaterCoilTargetQuantities(variable string) []string {
	switch normalizeName(variable) {
	case "temperature":
		return []string{"Temperature"}
	case "humidityratio":
		return []string{"HumidityRatio"}
	case "temperatureandhumidityratio":
		return []string{"Temperature", "MaximumHumidityRatio"}
	default:
		return nil
	}
}

// These concrete managers encode their target quantity in the object type,
// without a Control Variable field. Do not borrow that meaning for unknown
// manager schemas. EnergyPlus 24.2 Group Setpoint Managers documents both forms.
func hvacOperatingSetpointQuantity(obj Object) HVACControlParameter {
	if parameter, found := hvacOperatingNamedParameter(obj, "Control Variable"); found {
		return parameter
	}
	quantity := ""
	switch normalizeName(obj.Type) {
	case "setpointmanager:singlezone:humidity:maximum", "setpointmanager:multizone:maximumhumidity:average", "setpointmanager:multizone:humidity:maximum":
		quantity = "MaximumHumidityRatio"
	case "setpointmanager:singlezone:humidity:minimum", "setpointmanager:multizone:minimumhumidity:average", "setpointmanager:multizone:humidity:minimum":
		quantity = "MinimumHumidityRatio"
	}
	return HVACControlParameter{Name: "Target quantity", Value: quantity, ObjectIndex: obj.Index, FieldIndex: -1}
}

func hvacOperatingSetpointTargets(b *hvacOperatingBuilder, obj Object, reference bool) []HVACControlParameter {
	var result []HVACControlParameter
	for index := range obj.Fields {
		name := hvacOperatingFieldName(obj, index)
		lower := strings.ToLower(name)
		if !strings.Contains(lower, "node") || strings.Contains(lower, "reference") != reference || !reference && !strings.Contains(lower, "setpoint") {
			continue
		}
		for _, node := range b.expandedNodes(hvacFieldValue(obj, index)) {
			result = append(result, HVACControlParameter{Name: name, Value: node, ObjectIndex: obj.Index, FieldIndex: index})
		}
	}
	return result
}

func (b *hvacOperatingBuilder) parameterSetpoints(g *hvacOperatingControlBuilder, occ hvacOperatingOccurrence, legacy HVACControlSequence, modulation string, controllers []hvacParameterController, ports map[string]string) {
	candidates := []Object{}
	for _, node := range legacy.Nodes {
		if node.Code == "setpoint_manager" {
			candidates = append(candidates, b.objects[node.Source.ObjectIndex])
		}
	}
	// Recheck actual selected circuit ports rather than inheriting the legacy
	// broad node fallback when branch circuit identity itself is unresolved.
	wanted := map[string]bool{}
	for name := range ports {
		wanted[name] = true
	}
	for _, controller := range controllers {
		if controller.sensor != "" {
			wanted[controller.sensor] = true
		}
	}
	if occ.loop != nil && occ.loop.SetpointNode != "" {
		wanted[normalizeName(occ.loop.SetpointNode)] = true
	}
	managers := []Object{}
	seen := map[int]bool{}
	for pass := 0; pass < len(candidates); pass++ {
		for _, obj := range candidates {
			if seen[obj.Index] {
				continue
			}
			matched := false
			for _, target := range hvacOperatingSetpointTargets(b, obj, false) {
				matched = matched || wanted[normalizeName(target.Value)]
			}
			if matched {
				seen[obj.Index] = true
				managers = append(managers, obj)
				for _, reference := range hvacOperatingSetpointTargets(b, obj, true) {
					wanted[normalizeName(reference.Value)] = true
				}
			}
		}
	}
	type targetQuantity struct{ id, quantity string }
	targets := map[string][]targetQuantity{}
	resolvedTargets := map[string]map[string]bool{}
	for _, obj := range managers {
		id := g.node(fmt.Sprintf("setpoint:%d", obj.Index), "setpoint", "setpoint_manager", obj, hvacOperatingAllParameters(obj))
		b.parameterInputs(g, obj, id, id)
		quantity := hvacOperatingSetpointQuantity(obj)
		for _, target := range hvacOperatingSetpointTargets(b, obj, false) {
			setpoint := g.node(fmt.Sprintf("setpoint_value:%d:%s", obj.Index, normalizeName(target.Value)), "setpoint", "node_setpoint", obj, []HVACControlParameter{target, quantity})
			g.edge(id, setpoint, "sets_target")
			targets[normalizeName(target.Value)] = append(targets[normalizeName(target.Value)], targetQuantity{setpoint, quantity.Value})
			matched := false
			for _, controller := range controllers {
				if controller.sensor == normalizeName(target.Value) && hvacOperatingControlQuantityMatches(controller.variable, quantity.Value) {
					matched = true
					if resolvedTargets[controller.id] == nil {
						resolvedTargets[controller.id] = map[string]bool{}
					}
					resolvedTargets[controller.id][normalizeName(quantity.Value)] = true
					if strings.EqualFold(controller.variable, "HumidityRatio") {
						// The native humidity-only controller supports either
						// HumRatSetPoint or HumRatMax (chosen by provider type).
						resolvedTargets[controller.id]["humidityratio"] = true
					}
					if controller.errorID != "" {
						g.signal(setpoint, controller.errorID, "subtracts_target", "negative", "")
					} else {
						g.edge(setpoint, controller.id, "sets_target")
					}
				}
			}
			if !matched && len(controllers) == 0 && modulation != "" && (ports[normalizeName(target.Value)] != "" || occ.loop != nil && strings.EqualFold(occ.loop.SetpointNode, target.Value)) {
				g.edge(setpoint, modulation, "sets_target")
			}
		}
	}
	for _, controller := range controllers {
		if !controller.waterFlow || controller.sensor == "" {
			continue
		}
		obj := b.objects[controller.objectIndex]
		sensor, _ := hvacOperatingNamedParameter(obj, "Sensor Node Name")
		variable, _ := hvacOperatingNamedParameter(obj, "Control Variable")
		for _, quantity := range hvacOperatingWaterCoilTargetQuantities(controller.variable) {
			if resolvedTargets[controller.id][normalizeName(quantity)] {
				continue
			}
			id := controller.id + ":target"
			if controller.errorID == "" {
				id += ":" + normalizeName(quantity)
			}
			input := g.boundaryNode(id, "external_input", "unresolved_node_setpoint", "input", obj, []HVACControlParameter{sensor, variable, {Name: "Target quantity", Value: quantity, ObjectIndex: obj.Index, FieldIndex: -1}})
			if controller.errorID != "" {
				g.signal(input, controller.errorID, "subtracts_target", "negative", "")
			} else {
				// A known reference channel is not proof of its comparison or
				// combined-mode selection algorithm. Keep provider/value/sign
				// unresolved rather than omitting the external input entirely.
				g.signal(input, controller.id, "sets_target", "unknown", "")
			}
		}
	}
	for _, obj := range managers {
		id := fmt.Sprintf("setpoint:%d", obj.Index)
		quantity := hvacOperatingSetpointQuantity(obj)
		for _, reference := range hvacOperatingSetpointTargets(b, obj, true) {
			matched := false
			for _, source := range targets[normalizeName(reference.Value)] {
				if quantity.Value != "" && strings.EqualFold(quantity.Value, source.quantity) {
					g.edge(source.id, id, "references_setpoint")
					matched = true
				}
			}
			if !matched {
				source := g.boundaryNode("reference:"+normalizeName(reference.Value)+":"+normalizeName(quantity.Value), "external_input", "reference_setpoint", "input", obj, []HVACControlParameter{reference, quantity})
				g.edge(source, id, "references_setpoint")
			}
		}
		// Non-reference input nodes (e.g. MixedAir fan inlet/outlet) are
		// measured conditions, never additional manager output setpoints.
		for index := range obj.Fields {
			name := hvacOperatingFieldName(obj, index)
			lower := strings.ToLower(name)
			if !strings.Contains(lower, "node") || strings.Contains(lower, "setpoint") || strings.Contains(lower, "reference") || hvacFieldValue(obj, index) == "" {
				continue
			}
			parameter := HVACControlParameter{Name: name, Value: hvacFieldValue(obj, index), ObjectIndex: obj.Index, FieldIndex: index}
			source := ports[normalizeName(parameter.Value)]
			if source == "" {
				source = g.boundaryNode(fmt.Sprintf("setpoint_input:%d:%d", obj.Index, index), "external_input", "node_measurement", "input", obj, []HVACControlParameter{parameter})
			}
			g.signal(source, id, "measures", "unknown", "")
		}
	}
}

func (b *hvacOperatingBuilder) parameterThermostats(g *hvacOperatingControlBuilder, occ hvacOperatingOccurrence, legacy HVACControlSequence, modulation string) {
	for _, node := range legacy.Nodes {
		if node.Code != "zone_thermostat" {
			continue
		}
		obj := b.objects[node.Source.ObjectIndex]
		id := g.node(node.ID, "controller", node.Code, obj, node.Parameters)
		zone := hvacFieldValue(obj, 1)
		measurement := g.boundaryNode(id+":measurement", "external_input", "zone_temperature", "input", obj, []HVACControlParameter{{Name: "Zone or ZoneList Name", Value: zone, ObjectIndex: obj.Index, FieldIndex: 1}})
		g.signal(measurement, id, "feedback", "unknown", "")
		load := g.node(id+":request", "input", "zone_load_request", obj, nil)
		g.edge(id, load, "determines_request")
		g.edge(load, modulation, "requests")
		if mode := hvacFieldValue(obj, 2); mode != "" {
			g.edge(b.definitionSchedule(g, mode, "annual", obj, map[int]bool{}), id, "selects_mode")
		}
		for index := 3; index+1 < len(obj.Fields); index += 2 {
			setpoint, exists := b.exactObject(hvacFieldValue(obj, index), hvacFieldValue(obj, index+1))
			if !exists || !strings.HasPrefix(strings.ToLower(setpoint.Type), "thermostatsetpoint:") {
				continue
			}
			target := g.node(fmt.Sprintf("thermostat_setpoint:%d", setpoint.Index), "setpoint", "thermostat_setpoint", setpoint, hvacOperatingAllParameters(setpoint))
			g.edge(target, id, "sets_target")
			b.parameterInputs(g, setpoint, target, target)
		}
	}
}
