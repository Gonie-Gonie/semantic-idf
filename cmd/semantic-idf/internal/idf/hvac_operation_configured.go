package idf

import (
	"fmt"
	"strings"
)

// Configured availability is the minimum authored permission. It excludes
// runtime managers and demand/flow/range selection, not broken references.
// Legacy FinalAvailability remains the separate conservative execution gate.
func (b *hvacOperatingBuilder) configuredAvailability(occ hvacOperatingOccurrence, layers []HVACAvailabilityLayer) ([]HVACAvailabilityLayer, HVACOperatingExpression) {
	configured := make([]HVACAvailabilityLayer, 0, len(layers))
	conditions := make([]HVACOperatingExpression, 0, len(layers))
	for _, layer := range layers {
		copy := layer
		copy.Expression = b.configuredExpression(layer.Expression)
		if layer.Role == "plant_dispatch" {
			copy.Expression = b.configuredDispatchAvailability(occ, layer)
		}
		configured = append(configured, copy)
		conditions = append(conditions, copy.Expression)
	}
	if occ.loop != nil && strings.EqualFold(occ.loop.Type, "PlantLoop") && occ.loop.OperationScheme == "" && hvacOperatingDispatchEquipment(occ.object.Type) {
		for _, branch := range occ.loop.SupplySide.Branches {
			for _, component := range branch.Components {
				if component.ObjectIndex == occ.object.Index && strings.EqualFold(component.ObjectType, occ.object.Type) {
					source := hvacOperatingSource(b.objects[occ.loop.ObjectIndex])
					missing := HVACOperatingExpression{Op: "unknown", Reason: "missing_operation_schemes", Source: &source}
					configured = append(configured, HVACAvailabilityLayer{ID: fmt.Sprintf("%d:configured_missing_operation_schemes", source.ObjectIndex), Owner: source, Role: "plant_dispatch", Expression: missing})
					conditions = append(conditions, missing)
				}
			}
		}
	}
	return configured, hvacOperatingCombine("all", conditions...)
}

func (b *hvacOperatingBuilder) configuredExpression(expression HVACOperatingExpression) HVACOperatingExpression {
	copy := expression
	if expression.Op == "manager_unknown" && expression.Reason == "runtime_manager" {
		if expression.Source == nil {
			copy.Reason = "unresolved_manager_source"
			return copy
		}
		manager, found := b.objects[expression.Source.ObjectIndex]
		if !found || !strings.EqualFold(manager.Type, expression.Source.ObjectType) || objectName(manager) != expression.Source.ObjectName {
			copy.Reason = "unresolved_manager_source"
			return copy
		}
		if reason := b.runtimeManagerDefinitionError(manager, expression.AssignmentOwner); reason != "" {
			copy.Reason = reason
			return copy
		}
		zero := 0.0 // Manager status NoAction, not boolean blocked permission.
		return HVACOperatingExpression{Op: "always", Value: &zero, Source: expression.Source, Reason: "runtime_manager_excluded"}
	}
	if expression.Op == "unknown" && (expression.Reason == "runtime_dispatch" || expression.Reason == "airflow_path_requires_runtime") {
		return HVACOperatingExpression{Op: "always", Source: expression.Source, Reason: "runtime_condition_excluded"}
	}
	if expression.Children != nil {
		copy.Children = make([]HVACOperatingExpression, len(expression.Children))
		for index, child := range expression.Children {
			copy.Children[index] = b.configuredExpression(child)
		}
	}
	return copy
}

type hvacRuntimeAvailabilitySchema struct {
	requiredSchedules []int
	optionalSchedules []int
	requiredNodes     []int
	requiredNumbers   []int
	optionalNumbers   []int
}

// Literal native types only. A future/unrecognized manager must not become an
// unrestricted condition merely because the legacy evaluator needs runtime.
// Positions: EnergyPlus 22.1 IDD / 24.2 Group System Availability Managers.
var hvacRuntimeAvailabilitySchemas = map[string]hvacRuntimeAvailabilitySchema{
	"availabilitymanager:nightcycle":             {requiredSchedules: []int{1, 2}, optionalNumbers: []int{4, 6}},
	"availabilitymanager:optimumstart":           {requiredSchedules: []int{1, 2}, optionalNumbers: []int{6, 8, 9, 10, 11, 12, 13}},
	"availabilitymanager:nightventilation":       {requiredSchedules: []int{1, 2, 3}, optionalNumbers: []int{4, 5, 6}},
	"availabilitymanager:differentialthermostat": {requiredNodes: []int{1, 2}, requiredNumbers: []int{3}, optionalNumbers: []int{4}},
	"availabilitymanager:hightemperatureturnoff": {requiredNodes: []int{1}, requiredNumbers: []int{2}},
	"availabilitymanager:hightemperatureturnon":  {requiredNodes: []int{1}, requiredNumbers: []int{2}},
	"availabilitymanager:lowtemperatureturnoff":  {requiredNodes: []int{1}, requiredNumbers: []int{2}, optionalSchedules: []int{3}},
	"availabilitymanager:lowtemperatureturnon":   {requiredNodes: []int{1}, requiredNumbers: []int{2}},
}

func (b *hvacOperatingBuilder) runtimeManagerDefinitionError(manager Object, assignmentOwner *HVACOperatingSource) string {
	if strings.EqualFold(manager.Type, "AvailabilityManager:HybridVentilation") {
		return "invalid_manager_assignment"
	}
	schema, known := hvacRuntimeAvailabilitySchemas[normalizeName(manager.Type)]
	if !known {
		return "unsupported_manager_type"
	}
	if assignmentOwner == nil {
		return "unresolved_manager_assignment"
	}
	if (strings.EqualFold(assignmentOwner.ObjectType, "PlantLoop") || strings.EqualFold(assignmentOwner.ObjectType, "CondenserLoop")) && (strings.EqualFold(manager.Type, "AvailabilityManager:NightCycle") || strings.EqualFold(manager.Type, "AvailabilityManager:NightVentilation")) {
		return "invalid_manager_assignment"
	}
	for _, index := range schema.requiredSchedules {
		if name := hvacFieldValue(manager, index); name == "" {
			return "missing_runtime_manager_schedule"
		} else if reason := b.configuredScheduleDefinitionError(name, "annual", map[int]bool{}); reason != "" {
			return reason
		}
	}
	for _, index := range schema.optionalSchedules {
		if name := hvacFieldValue(manager, index); name != "" {
			if reason := b.configuredScheduleDefinitionError(name, "annual", map[int]bool{}); reason != "" {
				return reason
			}
		}
	}
	for _, index := range schema.requiredNodes {
		// Native node references create names; no separate Node object is required.
		if node := hvacFieldValue(manager, index); node == "" {
			return "missing_runtime_manager_node"
		}
	}
	for _, index := range schema.requiredNumbers {
		if _, valid := hvacOperationScheduleNumber(hvacFieldValue(manager, index)); !valid {
			return "invalid_runtime_manager_definition"
		}
	}
	for _, index := range schema.optionalNumbers {
		if value := hvacFieldValue(manager, index); value != "" {
			if _, valid := hvacOperationScheduleNumber(value); !valid {
				return "invalid_runtime_manager_definition"
			}
		}
	}
	return b.runtimeManagerZoneDefinitionError(manager)
}

func (b *hvacOperatingBuilder) runtimeManagerZoneDefinitionError(manager Object) string {
	check := func(name string, listAllowed bool) bool {
		if _, found := b.exactObject("Zone", name); found {
			return true
		}
		if listAllowed {
			_, found := b.exactObject("ZoneList", name)
			return found
		}
		return false
	}
	switch normalizeName(manager.Type) {
	case "availabilitymanager:nightcycle":
		control := hvacFieldValue(manager, 3)
		validControl := control == "" || stringSliceContainsFold([]string{"StayOff", "CycleOnAny", "CycleOnControlZone", "CycleOnAnyZoneFansOnly", "CycleOnAnyCoolingOrHeatingZone", "CycleOnAnyCoolingZone", "CycleOnAnyHeatingZone", "CycleOnAnyHeatingZoneFansOnly"}, control)
		runControl := hvacFieldValue(manager, 5)
		if !validControl || runControl != "" && !stringSliceContainsFold([]string{"FixedRunTime", "Thermostat", "ThermostatWithMinimumRunTime"}, runControl) {
			return "invalid_runtime_manager_definition"
		}
		for index := 7; index <= 10; index++ {
			name := hvacFieldValue(manager, index)
			if name != "" && !check(name, true) || index == 7 && strings.EqualFold(control, "CycleOnControlZone") && name == "" {
				return "missing_runtime_manager_zone"
			}
		}
	case "availabilitymanager:nightventilation":
		if !check(hvacFieldValue(manager, 7), false) {
			return "missing_runtime_manager_zone"
		}
	case "availabilitymanager:optimumstart":
		control := hvacFieldValue(manager, 3)
		if control == "" {
			control = "ControlZone" // Native IDD default, not an unrestricted mode.
		}
		if control != "" && !stringSliceContainsFold([]string{"StayOff", "ControlZone", "MaximumofZoneList"}, control) {
			return "invalid_runtime_manager_definition"
		}
		algorithm := hvacFieldValue(manager, 7)
		if algorithm != "" && !stringSliceContainsFold([]string{"ConstantTemperatureGradient", "AdaptiveTemperatureGradient", "AdaptiveASHRAE", "ConstantStartTime"}, algorithm) {
			return "invalid_runtime_manager_definition"
		}
		if strings.EqualFold(control, "ControlZone") && !check(hvacFieldValue(manager, 4), false) {
			return "missing_runtime_manager_zone"
		}
		if strings.EqualFold(control, "MaximumofZoneList") {
			if _, found := b.exactObject("ZoneList", hvacFieldValue(manager, 5)); !found {
				return "missing_runtime_manager_zone"
			}
		}
	}
	return ""
}

func (b *hvacOperatingBuilder) configuredScheduleDefinitionError(name, namespace string, path map[int]bool) string {
	var selected Object
	count := 0
	for _, obj := range b.ctx.objectsByName[normalizeName(name)] {
		typ := normalizeName(obj.Type)
		if !isScheduleType(obj.Type) && !(strings.HasPrefix(typ, "externalinterface:") && strings.HasSuffix(typ, ":schedule")) {
			continue
		}
		if hvacOperationScheduleNamespace(HVACOperationSchedule{ObjectType: obj.Type}) == namespace {
			selected, count = obj, count+1
		}
	}
	if count != 1 {
		return "missing_or_ambiguous_runtime_manager_schedule"
	}
	if path[selected.Index] {
		return "invalid_runtime_manager_schedule_definition"
	}
	path[selected.Index] = true
	defer delete(path, selected.Index)
	// Interpolation values are runtime-only here, but their authored step
	// body still must parse. Validate an owned copy with interpolation disabled,
	// without changing the original schedule or substituting its runtime values.
	validationObject := selected
	validationObject.Fields = append([]Field(nil), selected.Fields...)
	typ := normalizeName(selected.Type)
	validInterpolation := func(value string) bool {
		return value == "" || stringSliceContainsFold([]string{"No", "Average", "Linear", "Yes"}, value)
	}
	if typ == "schedule:day:interval" || typ == "schedule:day:list" {
		if !validInterpolation(hvacFieldValue(selected, 2)) {
			return "invalid_runtime_manager_schedule_definition"
		}
		if len(validationObject.Fields) > 2 {
			validationObject.Fields[2].Value = "No"
		}
	}
	if typ == "schedule:compact" {
		for index := 2; index < len(validationObject.Fields); index++ {
			if value, isInterpolation := hvacOperationScheduleDirective(validationObject.Fields[index].Value, "interpolate"); isInterpolation {
				if !validInterpolation(value) {
					return "invalid_runtime_manager_schedule_definition"
				}
				validationObject.Fields[index].Value = "Interpolate: No"
			}
		}
	}
	compiled := compileHVACOperationSchedule(validationObject)
	if compiled.Status != "supported" {
		// Externally supplied/interpolated values are runtime data, not parse
		// failures. Their valid reference remains irrelevant to this view.
		if typ == "schedule:file" || stringSliceContainsFold([]string{"externalinterface:schedule", "externalinterface:functionalmockupunitimport:to:schedule", "externalinterface:functionalmockupunitexport:to:schedule"}, typ) {
			return ""
		}
		return "invalid_runtime_manager_schedule_definition"
	}
	for _, period := range compiled.Periods {
		if reason := b.configuredScheduleDefinitionError(period.ScheduleName, "week", path); reason != "" {
			return reason
		}
	}
	for _, child := range compiled.WeekDays {
		if reason := b.configuredScheduleDefinitionError(child, "day", path); reason != "" {
			return reason
		}
	}
	for _, rule := range compiled.Rules {
		if rule.ScheduleName != "" {
			if reason := b.configuredScheduleDefinitionError(rule.ScheduleName, "day", path); reason != "" {
				return reason
			}
		}
	}
	return ""
}

func (b *hvacOperatingBuilder) configuredDispatchAvailability(occ hvacOperatingOccurrence, layer HVACAvailabilityLayer) HVACOperatingExpression {
	configured := b.configuredExpression(layer.Expression)
	owner, found := b.objects[layer.Owner.ObjectIndex]
	if !found || !hvacOperatingOperationSchemesType(owner.Type) {
		return configured // Original missing-scheme error remains explicit.
	}
	listType := "PlantEquipmentList"
	if strings.EqualFold(owner.Type, "CondenserEquipmentOperationSchemes") {
		listType = "CondenserEquipmentList"
	}
	var validation []HVACOperatingExpression
	for index := 1; index+2 < len(owner.Fields); index += 3 {
		typ, name := hvacFieldValue(owner, index), hvacFieldValue(owner, index+1)
		if typ == "" && name == "" {
			continue
		}
		operation, found := b.exactObject(typ, name)
		if !found {
			continue // Existing expression carries the exact missing-operation error.
		}
		_, _, resolved := b.operationMembership(operation, occ.object, listType)
		if !resolved {
			source := hvacOperatingSource(operation)
			validation = append(validation, HVACOperatingExpression{Op: "unknown", Reason: "unresolved_dispatch_membership", Source: &source})
		}
	}
	return hvacOperatingCombine("all", append([]HVACOperatingExpression{configured}, validation...)...)
}

func (b *hvacOperatingBuilder) dispatchAvailabilityDefinitions(g *hvacOperatingControlBuilder, occ hvacOperatingOccurrence, layer HVACAvailabilityLayer) {
	owner, found := b.objects[layer.Owner.ObjectIndex]
	if !found || !hvacOperatingOperationSchemesType(owner.Type) {
		return
	}
	id := g.node(fmt.Sprintf("definition:%d", owner.Index), "definition", "plant_operation_schemes", owner, hvacOperatingAllParameters(owner))
	for index := 1; index+2 < len(owner.Fields); index += 3 {
		typ, name, schedule := hvacFieldValue(owner, index), hvacFieldValue(owner, index+1), hvacFieldValue(owner, index+2)
		if typ == "" && name == "" {
			continue
		}
		if schedule != "" {
			g.edge(id, b.definitionSchedule(g, schedule, "annual", owner, map[int]bool{}), "declares_schedule")
		}
		operation, found := b.exactObject(typ, name)
		if found {
			g.edge(id, g.node(fmt.Sprintf("operation:%d", operation.Index), "definition", "plant_operation_definition", operation, hvacOperatingAllParameters(operation)), "references_operation")
		}
	}
}

func hvacOperatingOperationSchemesType(objectType string) bool {
	return strings.EqualFold(objectType, "PlantEquipmentOperationSchemes") || strings.EqualFold(objectType, "CondenserEquipmentOperationSchemes")
}
