package idf

import "strings"

type hvacOperatingDispatch struct {
	owner      Object
	permission HVACOperatingExpression
	choices    []hvacOperatingDispatchChoice
}

type hvacOperatingDispatchChoice struct {
	operation Object
	schedule  string
	lists     []Object
}

// Show only the selected equipment's range. Multiple matching ranges remain
// a dispatch relationship, without presenting one arbitrary range as its limit.
func hvacOperatingDispatchParameters(choice hvacOperatingDispatchChoice, equipment Object) []HVACControlParameter {
	op := choice.operation
	parameter := func(index int, fallback string) HVACControlParameter {
		name := hvacOperatingFieldName(op, index)
		if name == "" {
			name = fallback
		}
		return HVACControlParameter{Name: name, Value: hvacFieldValue(op, index), ObjectIndex: op.Index, FieldIndex: index}
	}
	if strings.EqualFold(op.Type, "PlantEquipmentOperation:ComponentSetpoint") {
		for index := 1; index+5 < len(op.Fields); index += 6 {
			if strings.EqualFold(hvacFieldValue(op, index), equipment.Type) && strings.EqualFold(hvacFieldValue(op, index+1), objectName(equipment)) {
				return []HVACControlParameter{parameter(index+2, "Demand Calculation Node Name"), parameter(index+3, "Setpoint Node Name"), parameter(index+4, "Component Flow Rate"), parameter(index+5, "Operation Type")}
			}
		}
		return nil
	}
	if len(choice.lists) == 0 {
		return nil
	}
	if strings.EqualFold(op.Type, "PlantEquipmentOperation:Uncontrolled") {
		return []HVACControlParameter{parameter(1, "Equipment List Name")}
	}
	wanted := map[string]bool{}
	for _, list := range choice.lists {
		wanted[normalizeName(objectName(list))] = true
	}
	var matched []int
	for index := 1; index < len(op.Fields); index++ {
		if wanted[normalizeName(hvacFieldValue(op, index))] {
			matched = append(matched, index)
		}
	}
	if len(matched) != 1 || matched[0] < 3 {
		return nil
	}
	index := matched[0]
	quantity := "Range"
	if strings.EqualFold(op.Type, "PlantEquipmentOperation:HeatingLoad") || strings.EqualFold(op.Type, "PlantEquipmentOperation:CoolingLoad") {
		quantity = "Load Range"
	}
	return []HVACControlParameter{parameter(index-2, quantity+" Lower Limit"), parameter(index-1, quantity+" Upper Limit"), parameter(index, "Equipment List Name")}
}

func (b *hvacOperatingBuilder) dispatch(occ hvacOperatingOccurrence) (hvacOperatingDispatch, bool) {
	if occ.loop == nil || strings.EqualFold(occ.loop.Type, "AirLoopHVAC") || occ.loop.OperationScheme == "" || !hvacOperatingDispatchEquipment(occ.object.Type) {
		return hvacOperatingDispatch{}, false
	}
	onSupply := false
	for _, branch := range occ.loop.SupplySide.Branches {
		for _, component := range branch.Components {
			if component.ObjectIndex == occ.object.Index && strings.EqualFold(component.ObjectType, occ.object.Type) {
				onSupply = true
			}
		}
	}
	if !onSupply {
		return hvacOperatingDispatch{}, false
	}
	schemeType, listType := "PlantEquipmentOperationSchemes", "PlantEquipmentList"
	if strings.EqualFold(occ.loop.Type, "CondenserLoop") {
		schemeType, listType = "CondenserEquipmentOperationSchemes", "CondenserEquipmentList"
	}
	schemes, found := b.exactObject(schemeType, occ.loop.OperationScheme)
	if !found {
		return hvacOperatingDispatch{owner: b.objects[occ.loop.ObjectIndex], permission: HVACOperatingExpression{Op: "unknown", Reason: "missing_operation_schemes"}}, true
	}
	result := hvacOperatingDispatch{owner: schemes}
	schemeSource := hvacOperatingSource(schemes)
	alternatives := []HVACOperatingExpression{}
	for index := 1; index+2 < len(schemes.Fields); index += 3 {
		typeName, name, schedule := hvacFieldValue(schemes, index), hvacFieldValue(schemes, index+1), hvacFieldValue(schemes, index+2)
		if typeName == "" && name == "" {
			continue
		}
		operation, exists := b.exactObject(typeName, name)
		permission := hvacOperatingSchedule(schedule)
		permission.Source = &schemeSource
		if schedule == "" {
			permission = HVACOperatingExpression{Op: "unknown", Reason: "missing_dispatch_schedule", Source: &schemeSource}
		}
		if !exists {
			alternatives = append(alternatives, hvacOperatingCombine("all", permission, HVACOperatingExpression{Op: "unknown", Reason: "missing_operation_scheme"}))
			continue
		}
		lists, member, resolved := b.operationMembership(operation, occ.object, listType)
		if member {
			// Active schemes are considered in assignment order. Range-based
			// dispatch further depends on runtime load or environmental inputs.
			// Keep scheduled-off hours definite, but do not turn a mere list
			// membership into an assertion that this equipment can run now.
			if index > 1 || !strings.EqualFold(operation.Type, "PlantEquipmentOperation:Uncontrolled") {
				permission = hvacOperatingCombine("all", permission, HVACOperatingExpression{Op: "unknown", Reason: "runtime_dispatch"})
			}
			alternatives = append(alternatives, permission)
			result.choices = append(result.choices, hvacOperatingDispatchChoice{operation: operation, schedule: schedule, lists: lists})
		} else if !resolved {
			alternatives = append(alternatives, hvacOperatingCombine("all", permission, HVACOperatingExpression{Op: "unknown", Reason: "unresolved_dispatch_membership"}))
			result.choices = append(result.choices, hvacOperatingDispatchChoice{operation: operation, schedule: schedule})
		}
	}
	if len(alternatives) == 0 {
		zero := 0.0
		result.permission = HVACOperatingExpression{Op: "always", Value: &zero, Reason: "not_in_operation_scheme", Source: &schemeSource}
	} else {
		result.permission = hvacOperatingCombine("any", alternatives...)
	}
	return result, true
}

func hvacOperatingDispatchEquipment(objectType string) bool {
	lower := strings.ToLower(objectType)
	return isPlantSourceEquipmentType(objectType) && !strings.HasPrefix(lower, "pipe:") || strings.HasPrefix(lower, "fluidcooler:") || strings.HasPrefix(lower, "evaporativefluidcooler:")
}

func (b *hvacOperatingBuilder) operationMembership(operation, equipment Object, listType string) ([]Object, bool, bool) {
	lower := strings.ToLower(operation.Type)
	lists := []Object{}
	member, resolved := false, true
	var listNames []string
	switch lower {
	case "plantequipmentoperation:uncontrolled":
		listNames = append(listNames, hvacFieldValue(operation, 1))
	case "plantequipmentoperation:heatingload", "plantequipmentoperation:coolingload", "plantequipmentoperation:outdoordrybulb", "plantequipmentoperation:outdoorwetbulb", "plantequipmentoperation:outdoorrelativehumidity", "plantequipmentoperation:outdoordewpoint":
		for index := 3; index < len(operation.Fields); index += 3 {
			listNames = append(listNames, hvacFieldValue(operation, index))
		}
	case "plantequipmentoperation:outdoordrybulbdifference", "plantequipmentoperation:outdoorwetbulbdifference", "plantequipmentoperation:outdoordewpointdifference":
		for index := 4; index < len(operation.Fields); index += 3 {
			listNames = append(listNames, hvacFieldValue(operation, index))
		}
	case "plantequipmentoperation:componentsetpoint":
		// Component groups: type, name, demand node, setpoint node,
		// flow rate, operation type. No textual/name similarity matching.
		for index := 1; index+1 < len(operation.Fields); index += 6 {
			if strings.EqualFold(hvacFieldValue(operation, index), equipment.Type) && strings.EqualFold(hvacFieldValue(operation, index+1), objectName(equipment)) {
				member = true
			}
		}
		return nil, member, true
	default:
		// User-defined, TES and specialized operation types are kept visible
		// without fabricating dispatch eligibility from incidental strings.
		return nil, false, false
	}
	if len(listNames) == 0 {
		return nil, false, false
	}
	for _, name := range listNames {
		list, exists := b.exactObject(listType, name)
		if !exists {
			resolved = false
			continue
		}
		matched := false
		for index := 1; index+1 < len(list.Fields); index += 2 {
			if strings.EqualFold(hvacFieldValue(list, index), equipment.Type) && strings.EqualFold(hvacFieldValue(list, index+1), objectName(equipment)) {
				member, matched = true, true
			}
		}
		if matched {
			lists = append(lists, list)
		}
	}
	return lists, member, resolved
}
