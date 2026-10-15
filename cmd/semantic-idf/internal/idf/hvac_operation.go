package idf

import (
	"fmt"
	"sort"
	"strings"
)

type hvacOperatingBuilder struct {
	ctx       *hvacContext
	objects   map[int]Object
	children  map[int][]int
	parents   map[int][]int
	loops     []HVACLoop
	relations []HVACZoneChain
}

type hvacOperatingOccurrence struct {
	object    Object
	ancestors []Object
	loop      *HVACLoop
	ambiguous bool
}

func buildHVACOperatingProfiles(ctx *hvacContext, loops []HVACLoop, relations []HVACZoneChain) []HVACOperatingProfile {
	b := hvacOperatingBuilder{ctx: ctx, objects: map[int]Object{}, children: map[int][]int{}, parents: map[int][]int{}, loops: loops, relations: relations}
	for _, obj := range ctx.doc.Objects {
		b.objects[obj.Index] = obj
	}
	for _, ref := range ctx.componentReferences {
		if !hvacOperatingContainment(ref) {
			continue
		}
		b.children[ref.FromObjectIndex] = appendUniqueInt(b.children[ref.FromObjectIndex], ref.TargetObjectIndex)
		b.parents[ref.TargetObjectIndex] = appendUniqueInt(b.parents[ref.TargetObjectIndex], ref.FromObjectIndex)
	}
	occurrences := map[string]hvacOperatingOccurrence{}
	var visit func(Object, []Object, *HVACLoop, map[int]bool)
	visit = func(obj Object, ancestors []Object, loop *HVACLoop, path map[int]bool) {
		if path[obj.Index] {
			return
		}
		path[obj.Index] = true
		defer delete(path, obj.Index)
		if hvacOperatingEquipment(obj.Type) {
			key := fmt.Sprintf("%d/", obj.Index)
			if loop != nil {
				key += loop.ID
			}
			candidate := hvacOperatingOccurrence{object: obj, ancestors: append([]Object(nil), ancestors...), loop: loop, ambiguous: len(b.parents[obj.Index]) > 1}
			if previous, ok := occurrences[key]; ok {
				candidate.ambiguous = candidate.ambiguous || previous.ambiguous || hvacOperatingAncestorKey(previous.ancestors) != hvacOperatingAncestorKey(ancestors)
				// A direct occurrence is authoritative for its own circuit; an
				// unrelated wrapper route cannot replace its source context.
				if len(previous.ancestors) <= len(ancestors) {
					previous.ambiguous = candidate.ambiguous
					candidate = previous
				}
			}
			occurrences[key] = candidate
		}
		for _, child := range b.children[obj.Index] {
			if target, ok := b.objects[child]; ok {
				visit(target, append(append([]Object(nil), ancestors...), obj), loop, path)
			}
		}
	}
	for index := range loops {
		loop := &loops[index]
		for _, component := range loopComponents(*loop) {
			if obj, ok := b.componentObject(component); ok {
				visit(obj, b.containingAncestors(obj.Index), loop, map[int]bool{})
			}
		}
		if strings.EqualFold(loop.Type, "AirLoopHVAC") {
			for _, relation := range relations {
				if !stringSliceContainsFold(relation.AirLoopNames, loop.Name) {
					continue
				}
				for _, component := range relation.TerminalUnits {
					if obj, ok := b.componentObject(component); ok {
						visit(obj, b.containingAncestors(obj.Index), loop, map[int]bool{})
					}
				}
			}
		}
	}
	// Zone/local equipment without a loop gets the same presentation. Start at
	// true containers, so internal fans and coils inherit only actual references.
	for _, obj := range ctx.doc.Objects {
		if !hvacOperatingEquipment(obj.Type) || len(b.parents[obj.Index]) != 0 {
			continue
		}
		found := false
		for _, occurrence := range occurrences {
			if occurrence.object.Index == obj.Index {
				found = true
				break
			}
		}
		if !found {
			visit(obj, nil, nil, map[int]bool{})
		}
	}
	for _, obj := range ctx.doc.Objects {
		if !hvacOperatingEquipment(obj.Type) {
			continue
		}
		found := false
		for _, occurrence := range occurrences {
			if occurrence.object.Index == obj.Index {
				found = true
				break
			}
		}
		if !found {
			visit(obj, nil, nil, map[int]bool{})
		}
	}
	profiles := make([]HVACOperatingProfile, 0, len(occurrences))
	for _, occurrence := range occurrences {
		profiles = append(profiles, b.profile(occurrence))
	}
	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].ComponentIndex != profiles[j].ComponentIndex {
			return profiles[i].ComponentIndex < profiles[j].ComponentIndex
		}
		return profiles[i].LoopID < profiles[j].LoopID
	})
	return profiles
}

func (b *hvacOperatingBuilder) containingAncestors(index int) []Object {
	var ancestors []Object
	seen := map[int]bool{index: true}
	for len(b.parents[index]) == 1 {
		index = b.parents[index][0]
		if seen[index] {
			break
		}
		seen[index] = true
		if obj, found := b.objects[index]; found {
			ancestors = append(ancestors, obj)
		}
	}
	for left, right := 0, len(ancestors)-1; left < right; left, right = left+1, right-1 {
		ancestors[left], ancestors[right] = ancestors[right], ancestors[left]
	}
	return ancestors
}

func hvacOperatingContainment(ref HVACComponentReference) bool {
	if ref.TargetExists && ref.RelationRole == "refrigerant_terminal_unit" && isRefrigerantSystemType(ref.FromObjectType) {
		return true
	}
	if !ref.TargetExists || ref.RelationRole != "internal_component_reference" {
		return false
	}
	if ref.RelationshipType != "" && ref.RelationshipType != "references" && ref.RelationshipType != "contains" {
		return false
	}
	// The catalog also uses reference pairs for controllers, companion devices,
	// plant dispatch lists and similar associations. They are not containers.
	from := strings.ToLower(ref.FromObjectType)
	// A heat-pump water heater availability schedule governs its compressor
	// and fan; the associated tank's backup heater keeps its own thermostat.
	if strings.HasPrefix(from, "waterheater:heatpump") && strings.HasPrefix(strings.ToLower(ref.TargetObjectType), "waterheater:") {
		return false
	}
	if !strings.HasPrefix(from, "zonehvac:") && !strings.HasPrefix(from, "airterminal:") && !strings.HasPrefix(from, "airloophvac:") && !strings.HasPrefix(from, "coilsystem:") && !strings.HasPrefix(from, "waterheater:heatpump") {
		return false
	}
	return !isHVACControlType(ref.TargetObjectType) && !strings.Contains(strings.ToLower(ref.NameFieldName), "companion")
}

func hvacOperatingEquipment(objectType string) bool {
	lower := strings.ToLower(objectType)
	if !isHVACComponentType(objectType) && lower != "duct" && !strings.HasPrefix(lower, "headeredpumps:") && !strings.HasPrefix(lower, "fluidcooler:") && !strings.HasPrefix(lower, "evaporativefluidcooler:") && !strings.HasPrefix(lower, "evaporativecooler:") && !strings.HasPrefix(lower, "humidifier:") {
		return false
	}
	return !isHVACControlType(objectType) && !strings.Contains(lower, "equipmentlist") && !strings.HasSuffix(lower, ":controllerlist") && !strings.HasSuffix(lower, ":equipmentconnections") && !strings.HasSuffix(lower, ":surfacegroup") && !strings.HasSuffix(lower, ":design") && !strings.HasSuffix(lower, ":supplypath") && !strings.HasSuffix(lower, ":returnpath") && !strings.Contains(lower, "nodelist")
}

func (b *hvacOperatingBuilder) componentObject(component HVACComponent) (Object, bool) {
	obj, ok := b.objects[component.ObjectIndex]
	return obj, ok && strings.EqualFold(obj.Type, component.ObjectType) && strings.EqualFold(objectName(obj), component.ObjectName)
}

func hvacOperatingAncestorKey(objects []Object) string {
	var key strings.Builder
	for _, obj := range objects {
		fmt.Fprintf(&key, "%d/", obj.Index)
	}
	return key.String()
}

func hvacOperatingSource(obj Object) HVACOperatingSource {
	return HVACOperatingSource{ObjectIndex: obj.Index, ObjectType: obj.Type, ObjectName: objectName(obj)}
}

func hvacOperatingAlways() HVACOperatingExpression { return HVACOperatingExpression{Op: "always"} }

func hvacOperatingCombine(op string, values ...HVACOperatingExpression) HVACOperatingExpression {
	if len(values) == 0 {
		return hvacOperatingAlways()
	}
	if len(values) == 1 {
		return values[0]
	}
	return HVACOperatingExpression{Op: op, Children: values}
}

func hvacOperatingSchedule(name string) HVACOperatingExpression {
	if strings.TrimSpace(name) == "" {
		return hvacOperatingAlways()
	}
	return HVACOperatingExpression{Op: "schedule_positive", ScheduleName: name}
}

func (b *hvacOperatingBuilder) profile(occ hvacOperatingOccurrence) HVACOperatingProfile {
	profile := HVACOperatingProfile{ComponentIndex: occ.object.Index, AvailabilityLayers: []HVACAvailabilityLayer{}}
	var gates []HVACOperatingExpression
	var managers []HVACOperatingExpression
	add := func(owner Object, role string, expression HVACOperatingExpression) {
		profile.AvailabilityLayers = append(profile.AvailabilityLayers, HVACAvailabilityLayer{ID: fmt.Sprintf("%d:%s:%d", owner.Index, role, len(profile.AvailabilityLayers)), Owner: hvacOperatingSource(owner), Role: role, Expression: expression})
		gates = append(gates, expression)
	}
	if occ.loop != nil {
		profile.LoopID = occ.loop.ID
		owner := b.objects[occ.loop.ObjectIndex]
		manager, exists := b.managerExpression(owner, occ.loop.Type)
		gate := hvacOperatingAlways()
		if exists {
			managers = append(managers, manager)
			gate = HVACOperatingExpression{Op: "manager_gate", Children: []HVACOperatingExpression{manager}, Target: hvacOperatingFanTarget(occ)}
		}
		add(owner, "loop", gate)
		if strings.EqualFold(occ.loop.Type, "AirLoopHVAC") && !strings.HasPrefix(strings.ToLower(occ.object.Type), "fan:") {
			if source, permission, ok := b.airTransportPermission(occ, manager, exists); ok {
				add(source, "air_transport", permission)
			}
		}
	}
	if dispatch, ok := b.dispatch(occ); ok {
		add(dispatch.owner, "plant_dispatch", dispatch.permission)
	}
	for _, owner := range occ.ancestors {
		schedule := hvacOperatingOwnSchedule(owner)
		if manager, exists := b.managerExpression(owner, "AirLoopHVAC"); exists {
			managers = append(managers, manager)
			schedule = hvacOperatingEquipmentManagerGate(owner, manager, schedule)
		}
		add(owner, "parent", schedule)
	}
	selectedGate := hvacOperatingOwnSchedule(occ.object)
	if manager, exists := b.managerExpression(occ.object, "AirLoopHVAC"); exists {
		managers = append(managers, manager)
		selectedGate = hvacOperatingEquipmentManagerGate(occ.object, manager, selectedGate)
	}
	if strings.HasPrefix(strings.ToLower(occ.object.Type), "fan:") && len(managers) > 0 {
		selectedGate = HVACOperatingExpression{Op: "fan_gate", Children: []HVACOperatingExpression{{Op: "air_managers", Children: managers}, hvacOperatingOwnSchedule(occ.object)}, Target: hvacOperatingFanTarget(occ)}
	}
	add(occ.object, "equipment", selectedGate)
	if occ.ambiguous {
		add(occ.object, "unresolved_parent", HVACOperatingExpression{Op: "unknown", Reason: "multiple_parent_paths"})
	}
	profile.FinalAvailability = hvacOperatingCombine("all", gates...)
	profile.Control = b.control(occ, profile.AvailabilityLayers)
	return profile
}

func hvacOperatingEquipmentManagerGate(owner Object, manager, schedule HVACOperatingExpression) HVACOperatingExpression {
	// Zone equipment availability managers act on the equipment's fan. A
	// CycleOn request can override the wrapper's own fan availability schedule.
	if strings.HasPrefix(strings.ToLower(owner.Type), "zonehvac:") {
		return HVACOperatingExpression{Op: "fan_gate", Children: []HVACOperatingExpression{manager, schedule}, Target: "zone_fan"}
	}
	return hvacOperatingCombine("all", schedule, HVACOperatingExpression{Op: "manager_gate", Children: []HVACOperatingExpression{manager}, Target: "zone_fan"})
}

func (b *hvacOperatingBuilder) airTransportPermission(occ hvacOperatingOccurrence, loopManager HVACOperatingExpression, hasManager bool) (Object, HVACOperatingExpression, bool) {
	family, _, _ := hvacOperatingRecipe(occ.object.Type)
	if family == "passive" {
		return Object{}, HVACOperatingExpression{}, false
	}
	// Fan-powered zone terminals can recirculate air under ZoneFansOnly while
	// the central fan is off. Their local fan permission is evaluated below the
	// actual wrapper; do not impose the central fan schedule on that mode.
	if hvacOperatingFanTarget(occ) == "zone_fan" {
		owners := append(append([]Object(nil), occ.ancestors...), occ.object)
		for ownerIndex := len(owners) - 1; ownerIndex >= 0; ownerIndex-- {
			localFans := map[int]Object{}
			var collect func(int, map[int]bool)
			collect = func(index int, seen map[int]bool) {
				if seen[index] {
					return
				}
				seen[index] = true
				for _, child := range b.children[index] {
					if strings.HasPrefix(strings.ToLower(b.objects[child].Type), "fan:") {
						localFans[child] = b.objects[child]
					} else {
						collect(child, seen)
					}
				}
			}
			collect(owners[ownerIndex].Index, map[int]bool{})
			if len(localFans) == 0 {
				continue
			}
			if len(localFans) != 1 {
				return owners[ownerIndex], HVACOperatingExpression{Op: "unknown", Reason: "airflow_path_requires_runtime"}, true
			}
			var managers []HVACOperatingExpression
			if hasManager {
				managers = append(managers, loopManager)
			}
			for _, owner := range owners {
				if manager, exists := b.managerExpression(owner, "AirLoopHVAC"); exists {
					managers = append(managers, manager)
				}
			}
			for _, fan := range localFans {
				permission := hvacOperatingOwnSchedule(fan)
				if len(managers) > 0 {
					permission = HVACOperatingExpression{Op: "fan_gate", Children: []HVACOperatingExpression{{Op: "air_managers", Children: managers}, permission}, Target: "zone_fan"}
				}
				return fan, permission, true
			}
		}
	}
	type fanPath struct {
		fan       Object
		ancestors []Object
	}
	fans := map[int]fanPath{}
	var visit func(Object, []Object, map[int]bool)
	visit = func(obj Object, parents []Object, seen map[int]bool) {
		if seen[obj.Index] {
			return
		}
		seen[obj.Index] = true
		if strings.HasPrefix(strings.ToLower(obj.Type), "fan:") && !strings.EqualFold(obj.Type, "Fan:ZoneExhaust") {
			fans[obj.Index] = fanPath{obj, parents}
		}
		for _, index := range b.children[obj.Index] {
			if child, found := b.objects[index]; found {
				visit(child, append(append([]Object(nil), parents...), obj), seen)
			}
		}
	}
	for _, branch := range occ.loop.SupplySide.Branches {
		for _, component := range branch.Components {
			if obj, found := b.componentObject(component); found {
				visit(obj, nil, map[int]bool{})
			}
		}
	}
	if len(fans) != 1 {
		return b.objects[occ.loop.ObjectIndex], HVACOperatingExpression{Op: "unknown", Reason: "airflow_path_requires_runtime"}, true
	}
	for _, path := range fans {
		var gates, managers []HVACOperatingExpression
		if hasManager {
			managers = append(managers, loopManager)
		}
		for _, parent := range path.ancestors {
			gates = append(gates, hvacOperatingOwnSchedule(parent))
			if manager, exists := b.managerExpression(parent, "AirLoopHVAC"); exists {
				managers = append(managers, manager)
			}
		}
		permission := hvacOperatingOwnSchedule(path.fan)
		if len(managers) > 0 {
			permission = HVACOperatingExpression{Op: "fan_gate", Children: []HVACOperatingExpression{{Op: "air_managers", Children: managers}, permission}, Target: "central_fan"}
		}
		gates = append(gates, permission)
		return path.fan, hvacOperatingCombine("all", gates...), true
	}
	return Object{}, HVACOperatingExpression{}, false
}

func hvacOperatingFanTarget(occ hvacOperatingOccurrence) string {
	for _, owner := range append(append([]Object(nil), occ.ancestors...), occ.object) {
		lower := strings.ToLower(owner.Type)
		if strings.HasPrefix(lower, "zonehvac:") || strings.HasPrefix(lower, "airterminal:") || lower == "fan:zoneexhaust" {
			return "zone_fan"
		}
	}
	return "central_fan"
}

func hvacOperatingOwnSchedule(obj Object) HVACOperatingExpression {
	// Fan operating mode, pump flow, setpoint and performance schedules are
	// controls, not availability. Only the explicit availability field gates.
	if !hvacOperatingAvailabilityKnown(obj) {
		return HVACOperatingExpression{Op: "unknown", Reason: "unresolved_availability_schema"}
	}
	return hvacOperatingSchedule(hvacOperatingAvailabilitySchedule(obj))
}

func (b *hvacOperatingBuilder) managerExpression(owner Object, loopType string) (HVACOperatingExpression, bool) {
	listName := hvacOperatingManagerListName(owner)
	if listName == "" {
		return HVACOperatingExpression{}, false
	}
	list, exists := b.exactObject("AvailabilityManagerAssignmentList", listName)
	if !exists {
		return HVACOperatingExpression{Op: "manager_unknown", Reason: "missing_manager_list"}, true
	}
	op := "air_managers"
	if strings.EqualFold(loopType, "PlantLoop") || strings.EqualFold(loopType, "CondenserLoop") {
		op = "plant_managers"
	}
	result := HVACOperatingExpression{Op: op, Children: []HVACOperatingExpression{}}
	for index := 1; index+1 < len(list.Fields); index += 2 {
		objectType, name := hvacFieldValue(list, index), hvacFieldValue(list, index+1)
		if objectType == "" && name == "" {
			continue
		}
		manager, found := b.exactObject(objectType, name)
		expression := HVACOperatingExpression{Op: "manager_unknown", Reason: "missing_manager"}
		if found {
			switch strings.ToLower(manager.Type) {
			case "availabilitymanager:scheduled":
				expression.Op = "manager_scheduled"
			case "availabilitymanager:scheduledon":
				expression.Op = "manager_scheduled_on"
			case "availabilitymanager:scheduledoff":
				expression.Op = "manager_scheduled_off"
			default:
				expression.Reason = "runtime_manager"
			}
			if expression.Op != "manager_unknown" {
				expression.ScheduleName = fieldValueByCatalogName(manager, "Schedule Name")
				if expression.ScheduleName == "" {
					// These three manager types have the same stable two-field
					// schema, including inputs without an external IDD loaded.
					expression.ScheduleName = hvacFieldValue(manager, 1)
				}
				expression.Reason = ""
				if expression.ScheduleName == "" {
					expression = HVACOperatingExpression{Op: "manager_unknown", Reason: "missing_schedule"}
				}
			}
		}
		result.Children = append(result.Children, expression)
	}
	if len(result.Children) == 0 {
		return HVACOperatingExpression{Op: "manager_unknown", Reason: "empty_manager_list"}, true
	}
	return result, true
}

func (b *hvacOperatingBuilder) exactObject(objectType, name string) (Object, bool) {
	var result Object
	count := 0
	for _, obj := range b.ctx.objectsByName[normalizeName(name)] {
		if strings.EqualFold(obj.Type, objectType) {
			result, count = obj, count+1
		}
	}
	return result, count == 1
}
